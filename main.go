package main

import (
	"context"
	"errors"
	"fmt"
)

type State struct {
	Data map[string]any
}

type Reducer func(existing, update any) any

type NodeFunc func(context.Context, State) (State, error)

type RouterFunc func(State) string

type Graph struct {
	Nodes map[string]NodeFunc
	Edges map[string]string
	condEdges map[string]RouterFunc
	condEdgeMap map[string]map[string]string
	channels map[string]Reducer
	entry string
}

const START = "__start__"
const END = "__end__"

func NewGraph() *Graph {
	return &Graph{
		Nodes: make(map[string]NodeFunc),
		Edges: make(map[string]string),
		condEdges: make(map[string]RouterFunc),
		condEdgeMap: make(map[string]map[string]string),
		channels: make(map[string]Reducer),
		entry: START,
	}
}

func (g *Graph) AddNode(name string, fn NodeFunc) {
	g.Nodes[name] = fn
}

func (g *Graph) AddEdge(from, to string) {
	g.Edges[from] = to
}

func (g *Graph) SetEntry(name string) {
	g.entry = name
}

func (g *Graph) AddConditionalEdge(from string, router RouterFunc, routes map[string]string) {
	g.condEdges[from] = router
	g.condEdgeMap[from] = routes
}

func (g *Graph) AddChannel(key string, r Reducer) {
	g.channels[key] = r
}

func OverwriteReducer(existing, update any) any {
	return update
}

func AppendReducer(existing, update any) any {
    out, _ := existing.([]any)
    return append(out, update.([]any)...)
}

func (g *Graph) Compile() error {
	isNode := func(name string) bool {
		_, ok := g.Nodes[name]
		return ok
	}

	var errs []error

	entry := g.entry
	if entry == START {
		target, ok := g.Edges[START]
		if !ok {
			errs = append(errs, errors.New("no edge from START; call AddEdge(START, ...) to SetEntry(...)"))
		} else {
			entry = target
		}
	}

	if !isNode(entry) && entry != START {
		errs = append(errs, fmt.Errorf("entry node %s does not exist", entry))
	}

	for from, to := range g.Edges {
		if from == START {
			continue
		}
		if !isNode(from){
			errs = append(errs, fmt.Errorf("edge from non-existent node: %s", from))
		}
		if !isNode(to) && to != END {
			errs = append(errs, fmt.Errorf("edge to non-existent node: %s", to))
		}
	}

	for from := range g.condEdges {
		if !isNode(from) {
			errs = append(errs, fmt.Errorf("conditional edge from non-existent node: %s", from))
		}
		if _, ok := g.condEdgeMap[from]; !ok || len(g.condEdgeMap[from]) == 0 {
			errs = append(errs, fmt.Errorf("conditional edge from node %s has no valid routes", from))
		}
		if routes, ok := g.condEdgeMap[from]; ok {
			for _, to := range routes {
				if !isNode(to) && to != END {
					errs = append(errs, fmt.Errorf("conditional edge from node %s to non-existent node: %s", from, to))
				}
			}
		}
	}

	for name := range g.Nodes {
		_, staticEdge := g.Edges[name]
		_, condEdge := g.condEdges[name]
		if staticEdge && condEdge {
			errs = append(errs, fmt.Errorf("node %s has both static and conditional edges", name))
		}
		if !staticEdge && !condEdge {
			errs = append(errs, fmt.Errorf("node %s has no outgoing edges", name))
		}
	}

	return errors.Join(errs...)
}

func (g *Graph) Run(ctx context.Context, initial State) (State, error) {

	err := g.Compile()

	if err != nil {
		return initial, fmt.Errorf("graph compilation failed: %w", err)
	}

	var current string = g.entry

	if current == START {
		next, ok := g.Edges[START]
		if !ok {
			return initial, fmt.Errorf("no edge from START; call AddEdge(START, ...) to SetEntry(...)")
		}
		current = next
	}

	state := initial

	const maxSteps = 1000

	for steps := 0; current != END; steps++ {
		if err := ctx.Err(); err != nil {
			return state, err
		}

		if steps > maxSteps {
			return state, fmt.Errorf("exceeded maximum steps (%d), possible infinite loop", maxSteps)
		}

		node, ok := g.Nodes[current]
		if !ok {
			return state, fmt.Errorf("node not found: %s", current)
		}
		newState, err := node(ctx, state)
		if err != nil {
			return state, fmt.Errorf("error occurred while processing node %s: %w", current, err)
		}
		for key, update := range newState.Data {
			reducer, ok := g.channels[key]
			if !ok {
				reducer = OverwriteReducer
			}
			state.Data[key] = reducer(state.Data[key], update)
		}

		if router, ok := g.condEdges[current]; ok {
			label := router(state)
			target, ok := g.condEdgeMap[current][label]
			if !ok {
				return state, fmt.Errorf("conditional edge from node %s has no route for label: %s", current, label)
			}
			current = target
			continue
		}

		next, ok := g.Edges[current]
		if !ok {
			return state, fmt.Errorf("no outgoing edge from node: %s", current)
		}
		current = next
	}
	return state, nil
}





func main() {
	g := NewGraph()

	g.AddChannel("messages", AppendReducer)

	g.AddNode("greet", func(ctx context.Context, s State) (State, error) {
		return State{Data: map[string]any{
			"messages": []any{"greet: hello"},
			"step":     1,
		}}, nil
	})

	g.AddNode("ask", func(ctx context.Context, s State) (State, error) {
		return State{Data: map[string]any{
			"messages": []any{"ask: how are you?"},
			"step":     2,
		}}, nil
	})

	g.AddNode("farewell", func(ctx context.Context, s State) (State, error) {
		return State{Data: map[string]any{
			"messages": []any{"farewell: goodbye"},
			"step":     3,
		}}, nil
	})

	g.AddEdge(START, "greet")
	g.AddEdge("greet", "ask")
	g.AddEdge("ask", "farewell")
	g.AddEdge("farewell", END)

	final, err := g.Run(context.Background(), State{Data: map[string]any{}})
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Println("messages (append reducer accumulates every node's update):")
	for _, m := range final.Data["messages"].([]any) {
		fmt.Println("  -", m)
	}
	fmt.Println("step (overwrite reducer keeps only the last value):", final.Data["step"])
}
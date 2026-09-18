package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type State struct {
	Data map[string]any
}

type Reducer func(existing, update any) any

type NodeFunc func(context.Context, State) (State, error)

type nodeResult struct {
	name  string
	delta State
	err   error
}

type RouterFunc func(State) string

type Graph struct {
	Nodes       map[string]NodeFunc
	Edges       map[string][]string
	condEdges   map[string]RouterFunc
	condEdgeMap map[string]map[string]string
	channels    map[string]Reducer
	entry       string
}

const START = "__start__"
const END = "__end__"

func NewGraph() *Graph {
	return &Graph{
		Nodes:       make(map[string]NodeFunc),
		Edges:       make(map[string][]string),
		condEdges:   make(map[string]RouterFunc),
		condEdgeMap: make(map[string]map[string]string),
		channels:    make(map[string]Reducer),
		entry:       START,
	}
}

func (g *Graph) AddNode(name string, fn NodeFunc) {
	g.Nodes[name] = fn
}

func (g *Graph) AddEdge(from, to string) {
	g.Edges[from] = append(g.Edges[from], to)
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

func AddReducer(existing, update any) any {
	current, _ := existing.(int)
	increment, _ := update.(int)
	return current + increment
}

func (g *Graph) Compile() error {
	isNode := func(name string) bool {
		_, ok := g.Nodes[name]
		return ok
	}

	var errs []error

	entry := g.entry
	if entry == START {
		targets, ok := g.Edges[START]
		if !ok || len(targets) == 0 {
			errs = append(errs, errors.New("no edge from START; call AddEdge(START, ...) to SetEntry(...)"))
		} else {
			for _, target := range targets {
				if !isNode(target) {
					errs = append(errs, fmt.Errorf("entry node %s does not exist", target))
				}
			}
		}
	}

	if !isNode(entry) && entry != START {
		errs = append(errs, fmt.Errorf("entry node %s does not exist", entry))
	}

	for from, tos := range g.Edges {
		if from == START {
			continue
		}
		if !isNode(from) {
			errs = append(errs, fmt.Errorf("edge from non-existent node: %s", from))
		}
		for _, to := range tos {
			if !isNode(to) && to != END {
				errs = append(errs, fmt.Errorf("edge to non-existent node: %s", to))
			}
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

	state := initial
	if state.Data == nil {
		state.Data = make(map[string]any)
	}

	frontier := []string{}

	if g.entry == START {
		frontier = append(frontier, g.Edges[START]...)
	} else {
		frontier = append(frontier, g.entry)
	}

	const maxSteps = 1000
	for step := 0; len(frontier) > 0; step++ {
		if err := ctx.Err(); err != nil {
			return state, err
		}
		if step >= maxSteps {
			return state, fmt.Errorf("exceeded maximum steps (%d), possible infinite loop", maxSteps)
		}

		snapshot := State{Data: make(map[string]any, len(state.Data))}
		for key, value := range state.Data {
			snapshot.Data[key] = value
		}

		results := make([]nodeResult, len(frontier))
		var wg sync.WaitGroup
		wg.Add(len(frontier))

		for i, nodeName := range frontier {
			node, ok := g.Nodes[nodeName]
			if !ok {
				return state, fmt.Errorf("node not found: %s", nodeName)
			}

			go func(i int, nodeName string, node NodeFunc) {
				defer wg.Done()
				delta, err := node(ctx, snapshot)
				results[i] = nodeResult{name: nodeName, delta: delta, err: err}
			}(i, nodeName, node)
		}

		wg.Wait()

		for _, result := range results {
			if result.err != nil {
				return state, fmt.Errorf("error occurred while processing node %s: %w", result.name, result.err)
			}
		}

		for _, result := range results {
			for key, update := range result.delta.Data {
				reducer := g.channels[key]
				if reducer == nil {
					reducer = OverwriteReducer
				}
				state.Data[key] = reducer(state.Data[key], update)
			}
		}

		nextFrontier := make([]string, 0)
		seen := make(map[string]bool)
		for _, result := range results {
			destinations := g.Edges[result.name]
			if router, ok := g.condEdges[result.name]; ok {
				label := router(state)
				target, ok := g.condEdgeMap[result.name][label]
				if !ok {
					return state, fmt.Errorf("conditional edge from node %s has no route for label: %s", result.name, label)
				}
				destinations = []string{target}
			}

			for _, destination := range destinations {
				if destination == END || seen[destination] {
					continue
				}
				seen[destination] = true
				nextFrontier = append(nextFrontier, destination)
			}
		}

		frontier = nextFrontier
	}

	return state, nil
}

func main() {
	g := NewGraph()

	g.AddChannel("messages", AppendReducer)
	g.AddChannel("done", AddReducer)

	g.AddNode("dispatch", func(ctx context.Context, s State) (State, error) {
		return State{Data: map[string]any{
			"messages": []any{"dispatch: starting workers"},
		}}, nil
	})

	for _, workerName := range []string{"worker1", "worker2", "worker3"} {
		name := workerName
		g.AddNode(name, func(ctx context.Context, s State) (State, error) {
			return State{Data: map[string]any{
				"messages": []any{name + ": finished"},
				"done":     1,
			}}, nil
		})
	}

	g.AddNode("join", func(ctx context.Context, s State) (State, error) {
		return State{Data: map[string]any{
			"messages": []any{"join: all workers finished"},
		}}, nil
	})

	g.AddEdge(START, "dispatch")
	for _, workerName := range []string{"worker1", "worker2", "worker3"} {
		g.AddEdge("dispatch", workerName)
		g.AddEdge(workerName, "join")
	}
	g.AddEdge("join", END)

	final, err := g.Run(context.Background(), State{Data: map[string]any{}})
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Println("messages (append reducer; worker order may vary):")
	for _, m := range final.Data["messages"].([]any) {
		fmt.Println("  -", m)
	}
	fmt.Println("done (add reducer counts worker completions):", final.Data["done"])
}

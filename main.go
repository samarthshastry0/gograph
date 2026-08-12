package main

import (
	"fmt"
	"errors"
	"context"
)

type State struct {
	Data map[string]any
}

type NodeFunc func(context.Context, State) (State, error)

type RouterFunc func(State) string

type Graph struct {
	Nodes map[string]NodeFunc
	Edges map[string]string
	condEdges map[string]RouterFunc
	condEdgeMap map[string]map[string]string
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

func (g *Graph) compile() error {
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
			for to := range routes {
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

	err := g.compile()

	if err != nil {
		return initial, fmt.Errorf("graph compilation failed: %w", err)
	}

	current := g.entry

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
		state = newState

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

	g.AddNode("count", func(ctx context.Context, s State) (State, error) {
		n, _ := s.Data["n"].(int)
		n++
		s.Data["n"] = n
		fmt.Println("n =", n)
		return s, nil
	})

	g.AddConditionalEdge("count", func(s State) string {
		n, _ := s.Data["n"].(int)
		if n < 100 {
			return "count"
		}
		return END
	}, map[string]string{
		"count": "count",
		"__end__": END,
	})

	g.SetEntry("count")

	final, err := g.Run(context.Background(), State{Data: map[string]any{"n": 0}})
	if err != nil {
		fmt.Println("Error:", err)
	}
	fmt.Println("Final n:", final.Data["n"])
}
package main

import "fmt"

type State struct {
	Data map[string]any
}

type NodeFunc func(State) State

type RouterFunc func(State) string

type Graph struct {
	Nodes map[string]NodeFunc
	Edges map[string]string
	condEdges map[string]RouterFunc
	entry string
}

func NewGraph() *Graph {
	return &Graph{
		Nodes: make(map[string]NodeFunc),
		Edges: make(map[string]string),
		condEdges: make(map[string]RouterFunc),
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

func (g *Graph) AddConditionalEdge(from string, router RouterFunc) {
	g.condEdges[from] = router
}

const END = "__end__"

func (g *Graph) Run(initial State) (State, error) {
	current := g.entry
	state := initial

	const maxSteps = 1000

	for steps := 0; current != END; steps++ {
		if steps > maxSteps {
			return state, fmt.Errorf("exceeded maximum steps (%d), possible infinite loop", maxSteps)
		}
	
		node, ok := g.Nodes[current]
		if !ok {
			return state, fmt.Errorf("node not found: %s", current)
		}
		state = node(state)

		if router, ok := g.condEdges[current]; ok {
			current = router(state)
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

	g.AddNode("count", func(s State) State {
		n, _ := s.Data["n"].(int)
		n++
		s.Data["n"] = n
		fmt.Println("n =", n)
		return s
	})

	g.AddConditionalEdge("count", func(s State) string {
		n, _ := s.Data["n"].(int)
		if n < 100 {
			return "count"
		}
		return END
	})

	g.SetEntry("count")

	final, err := g.Run(State{Data: map[string]any{}})
	if err != nil {
		fmt.Println("Error:", err)
	}
	fmt.Println("Final n:", final.Data["n"])
}
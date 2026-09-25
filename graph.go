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
	return g.execute(ctx, initial, nil)
}

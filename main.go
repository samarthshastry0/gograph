package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

type NodeFactory func(config map[string]any) (NodeFunc, error)

var reducerRegistry = map[string]Reducer{
	"overwrite": OverwriteReducer,
	"append":    AppendReducer,
	"add":       AddReducer,
}

var nodeRegistry = map[string]NodeFactory{}
var routerRegistry = map[string]RouterFunc{}

type Graph struct {
	Nodes       map[string]NodeFunc
	Edges       map[string][]string
	condEdges   map[string]RouterFunc
	condEdgeMap map[string]map[string]string
	channels    map[string]Reducer
	entry       string
}

type GraphSpec struct {
	Entry     string        `json:"entry"`
	Nodes     []NodeSpec    `json:"nodes"`
	Edges     []EdgeSpec    `json:"edges"`
	CondEdges []CondSpec    `json:"conditional_edges"`
	Channels  []ChannelSpec `json:"channels"`
}

type NodeSpec struct {
	ID     string         `json:"id"`
	Type   string         `json:"type"`
	Config map[string]any `json:"config"`
}

type EdgeSpec struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type CondSpec struct {
	From   string            `json:"from"`
	Router string            `json:"router"`
	Routes map[string]string `json:"routes"`
}

type ChannelSpec struct {
	Key     string `json:"key"`
	Reducer string `json:"reducer"`
}

type StepEvent struct {
	Step   int      `json:"step"`
	Active []string `json:"active"`
	State  State    `json:"state"`
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

func RegisterNodeType(name string, factory NodeFactory) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("node type name cannot be empty")
	}
	if factory == nil {
		return fmt.Errorf("node type %q cannot be nil", name)
	}
	if _, exists := nodeRegistry[name]; exists {
		return fmt.Errorf("node type %q already registered", name)
	}

	nodeRegistry[name] = factory
	return nil
}

func RegisterRouter(name string, router RouterFunc) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("router name cannot be empty")
	}
	if router == nil {
		return fmt.Errorf("router for %q cannot be nil", name)
	}
	if _, exists := routerRegistry[name]; exists {
		return fmt.Errorf("router %q already registered", name)
	}
	routerRegistry[name] = router
	return nil
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

func (g *Graph) execute(ctx context.Context, initial State, emit func(StepEvent) error) (State, error) {
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

		if emit != nil {
			event := StepEvent{
				Step:   step,
				Active: append([]string(nil), frontier...),
				State:  cloneState(state),
			}
			if err := emit(event); err != nil {
				return state, err
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

func cloneState(state State) State {
	cloned := State{Data: make(map[string]any, len(state.Data))}
	for key, value := range state.Data {
		cloned.Data[key] = cloneValue(value)
	}
	return cloned
}

func cloneValue(value any) any {
	switch value := value.(type) {
	case []any:
		cloned := make([]any, len(value))
		for i, item := range value {
			cloned[i] = cloneValue(item)
		}
		return cloned
	case map[string]any:
		cloned := make(map[string]any, len(value))
		for key, item := range value {
			cloned[key] = cloneValue(item)
		}
		return cloned
	default:
		return value
	}
}

func BuildGraph(spec GraphSpec) (*Graph, error) {
	g := NewGraph()

	if spec.Entry != "" {
		g.SetEntry(spec.Entry)
	}

	seenChannels := make(map[string]bool)
	for _, channelSpec := range spec.Channels {
		if strings.TrimSpace(channelSpec.Key) == "" {
			return nil, errors.New("channel key cannot be empty")
		}
		if seenChannels[channelSpec.Key] {
			return nil, fmt.Errorf("duplicate channel key %q", channelSpec.Key)
		}
		seenChannels[channelSpec.Key] = true

		reducerName := channelSpec.Reducer
		if reducerName == "" {
			reducerName = "overwrite"
		}
		reducer, ok := reducerRegistry[reducerName]
		if !ok {
			return nil, fmt.Errorf(
				"channel %q references unknown reducer %q",
				channelSpec.Key,
				reducerName,
			)
		}
		g.AddChannel(channelSpec.Key, reducer)
	}

	seenNodeIDs := make(map[string]bool)
	for _, nodeSpec := range spec.Nodes {
		if strings.TrimSpace(nodeSpec.ID) == "" {
			return nil, errors.New("node id cannot be empty")
		}
		if seenNodeIDs[nodeSpec.ID] {
			return nil, fmt.Errorf("duplicate node id %q", nodeSpec.ID)
		}
		seenNodeIDs[nodeSpec.ID] = true
		if strings.TrimSpace(nodeSpec.Type) == "" {
			return nil, fmt.Errorf("node %q has an empty type", nodeSpec.ID)
		}

		factory, ok := nodeRegistry[nodeSpec.Type]
		if !ok {
			return nil, fmt.Errorf(
				"node %q references unknown node type %q",
				nodeSpec.ID,
				nodeSpec.Type,
			)
		}

		config := nodeSpec.Config
		if config == nil {
			config = make(map[string]any)
		}
		node, err := factory(config)
		if err != nil {
			return nil, fmt.Errorf(
				"building node %q of type %q: %w",
				nodeSpec.ID,
				nodeSpec.Type,
				err,
			)
		}
		if node == nil {
			return nil, fmt.Errorf(
				"node factory %q returned a nil node for %q",
				nodeSpec.Type,
				nodeSpec.ID,
			)
		}
		g.AddNode(nodeSpec.ID, node)
	}

	for _, edgeSpec := range spec.Edges {
		if strings.TrimSpace(edgeSpec.From) == "" || strings.TrimSpace(edgeSpec.To) == "" {
			return nil, errors.New("edge must have both from and to values")
		}
		g.AddEdge(edgeSpec.From, edgeSpec.To)
	}

	for _, condSpec := range spec.CondEdges {
		if strings.TrimSpace(condSpec.From) == "" {
			return nil, errors.New("conditional edge source cannot be empty")
		}
		if strings.TrimSpace(condSpec.Router) == "" {
			return nil, fmt.Errorf(
				"conditional edge from %q has an empty router",
				condSpec.From,
			)
		}

		router, ok := routerRegistry[condSpec.Router]
		if !ok {
			return nil, fmt.Errorf(
				"conditional edge from %q references unknown router %q",
				condSpec.From,
				condSpec.Router,
			)
		}
		if len(condSpec.Routes) == 0 {
			return nil, fmt.Errorf(
				"conditional edge from %q has no routes",
				condSpec.From,
			)
		}
		g.AddConditionalEdge(condSpec.From, router, condSpec.Routes)
	}

	if err := g.Compile(); err != nil {
		return nil, fmt.Errorf("invalid graph specification: %w", err)
	}

	return g, nil
}

func LoadGraph(data []byte) (*Graph, error) {
	var spec GraphSpec

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	if err := decoder.Decode(&spec); err != nil {
		return nil, fmt.Errorf("invalid graph JSON: %w", err)
	}

	graph, err := BuildGraph(spec)
	if err != nil {
		return nil, fmt.Errorf("invalid graph specification: %w", err)
	}

	return graph, nil
}

func (g *Graph) Stream(ctx context.Context, initial State) <-chan StepEvent {
	events := make(chan StepEvent)

	go func() {
		defer close(events)
		_, _ = g.execute(ctx, initial, func(event StepEvent) error {
			select {
			case events <- event:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()

	return events
}

func main() {
	g := NewGraph()

	g.AddChannel("messages", AppendReducer)
	g.AddChannel("done", AddReducer)

	g.AddNode("dispatch", func(ctx context.Context, s State) (State, error) {
		return State{Data: map[string]any{
			"messages": []any{"dispatch: starting race cars"},
		}}, nil
	})

	for i := 0; i < 3; i++ {
		RaceCar := fmt.Sprintf("Race Car%d", i+1)
		g.AddNode(RaceCar, func(ctx context.Context, s State) (State, error) {
			return State{Data: map[string]any{
				"messages": []any{RaceCar + ": finished"},
				"done":     1,
			}}, nil
		})
	}

	g.AddNode("join", func(ctx context.Context, s State) (State, error) {
		return State{Data: map[string]any{
			"messages": []any{"join: all race cars finished"},
		}}, nil
	})

	g.AddEdge(START, "dispatch")
	for i := 0; i < 3; i++ {
		RaceCar := fmt.Sprintf("Race Car%d", i+1)
		g.AddEdge("dispatch", RaceCar)
		g.AddEdge(RaceCar, "join")
	}
	g.AddEdge("join", END)

	final, err := g.Run(context.Background(), State{Data: map[string]any{}})
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Println("messages (append reducer; race car order may vary):")
	for _, m := range final.Data["messages"].([]any) {
		fmt.Println("  -", m)
	}
	fmt.Println("done (add reducer counts race car completions):", final.Data["done"])

}

package gograph

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

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
			return nil, fmt.Errorf("channel %q references unknown reducer %q", channelSpec.Key, reducerName)
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
			return nil, fmt.Errorf("node %q references unknown node type %q", nodeSpec.ID, nodeSpec.Type)
		}

		config := nodeSpec.Config
		if config == nil {
			config = make(map[string]any)
		}
		node, err := factory(config)
		if err != nil {
			return nil, fmt.Errorf("building node %q of type %q: %w", nodeSpec.ID, nodeSpec.Type, err)
		}
		if node == nil {
			return nil, fmt.Errorf("node factory %q returned a nil node for %q", nodeSpec.Type, nodeSpec.ID)
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
			return nil, fmt.Errorf("conditional edge from %q has an empty router", condSpec.From)
		}

		router, ok := routerRegistry[condSpec.Router]
		if !ok {
			return nil, fmt.Errorf("conditional edge from %q references unknown router %q", condSpec.From, condSpec.Router)
		}
		if len(condSpec.Routes) == 0 {
			return nil, fmt.Errorf("conditional edge from %q has no routes", condSpec.From)
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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func phase4Worker(name string) NodeFunc {
	return func(ctx context.Context, state State) (State, error) {
		return State{Data: map[string]any{
			"messages": []any{name + ": finished"},
			"done":     1,
		}}, nil
	}
}

func buildPhase4HandWrittenGraph() *Graph {
	graph := NewGraph()
	graph.AddChannel("messages", AppendReducer)
	graph.AddChannel("done", AddReducer)

	graph.AddNode("dispatch", func(ctx context.Context, state State) (State, error) {
		return State{Data: map[string]any{
			"messages": []any{"dispatch: starting race cars"},
		}}, nil
	})

	for i := 1; i <= 3; i++ {
		name := fmt.Sprintf("Race Car%d", i)
		graph.AddNode(name, phase4Worker(name))
	}

	graph.AddNode("join", func(ctx context.Context, state State) (State, error) {
		return State{Data: map[string]any{
			"messages": []any{"join: all race cars finished"},
		}}, nil
	})

	graph.AddEdge(START, "dispatch")
	for i := 1; i <= 3; i++ {
		name := fmt.Sprintf("Race Car%d", i)
		graph.AddEdge("dispatch", name)
		graph.AddEdge(name, "join")
	}
	graph.AddEdge("join", END)

	return graph
}

func sortedPhase4Messages(state State) []string {
	rawMessages := state.Data["messages"].([]any)
	messages := make([]string, 0, len(rawMessages))
	for _, rawMessage := range rawMessages {
		messages = append(messages, rawMessage.(string))
	}
	sort.Strings(messages)
	return messages
}

func registerPhase4DemoTypes(t *testing.T) {
	t.Helper()

	registrations := []struct {
		name    string
		factory NodeFactory
	}{
		{
			name: "phase4_dispatch_test",
			factory: func(config map[string]any) (NodeFunc, error) {
				return func(ctx context.Context, state State) (State, error) {
					return State{Data: map[string]any{
						"messages": []any{"dispatch: starting race cars"},
					}}, nil
				}, nil
			},
		},
		{
			name: "phase4_worker_test",
			factory: func(config map[string]any) (NodeFunc, error) {
				name, ok := config["name"].(string)
				if !ok || name == "" {
					return nil, fmt.Errorf("worker requires a string name")
				}
				return phase4Worker(name), nil
			},
		},
		{
			name: "phase4_join_test",
			factory: func(config map[string]any) (NodeFunc, error) {
				return func(ctx context.Context, state State) (State, error) {
					return State{Data: map[string]any{
						"messages": []any{"join: all race cars finished"},
					}}, nil
				}, nil
			},
		},
	}

	for _, registration := range registrations {
		if err := RegisterNodeType(registration.name, registration.factory); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadGraphMatchesHandWrittenDemo(t *testing.T) {
	registerPhase4DemoTypes(t)

	data := []byte(`{
		"entry": "__start__",
		"nodes": [
			{"id": "dispatch", "type": "phase4_dispatch_test", "config": {}},
			{"id": "Race Car1", "type": "phase4_worker_test", "config": {"name": "Race Car1"}},
			{"id": "Race Car2", "type": "phase4_worker_test", "config": {"name": "Race Car2"}},
			{"id": "Race Car3", "type": "phase4_worker_test", "config": {"name": "Race Car3"}},
			{"id": "join", "type": "phase4_join_test", "config": {}}
		],
		"edges": [
			{"from": "__start__", "to": "dispatch"},
			{"from": "dispatch", "to": "Race Car1"},
			{"from": "dispatch", "to": "Race Car2"},
			{"from": "dispatch", "to": "Race Car3"},
			{"from": "Race Car1", "to": "join"},
			{"from": "Race Car2", "to": "join"},
			{"from": "Race Car3", "to": "join"},
			{"from": "join", "to": "__end__"}
		],
		"channels": [
			{"key": "messages", "reducer": "append"},
			{"key": "done", "reducer": "add"}
		]
	}`)

	jsonGraph, err := LoadGraph(data)
	if err != nil {
		t.Fatal(err)
	}
	jsonState, err := jsonGraph.Run(context.Background(), State{})
	if err != nil {
		t.Fatal(err)
	}

	handWrittenState, err := buildPhase4HandWrittenGraph().Run(
		context.Background(),
		State{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if jsonState.Data["done"] != handWrittenState.Data["done"] {
		t.Fatalf("done count differs: JSON=%v hand-written=%v", jsonState.Data["done"], handWrittenState.Data["done"])
	}
	if strings.Join(sortedPhase4Messages(jsonState), "\n") != strings.Join(sortedPhase4Messages(handWrittenState), "\n") {
		t.Fatalf("messages differ: JSON=%v hand-written=%v", sortedPhase4Messages(jsonState), sortedPhase4Messages(handWrittenState))
	}
}

func TestLoadGraphPreservesJSONNumbers(t *testing.T) {
	err := RegisterNodeType("phase4_number_test", func(config map[string]any) (NodeFunc, error) {
		value, ok := config["count"].(json.Number)
		if !ok {
			return nil, fmt.Errorf("count has type %T, want json.Number", config["count"])
		}
		count, err := strconv.Atoi(value.String())
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, state State) (State, error) {
			return State{Data: map[string]any{"count": count}}, nil
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	graph, err := LoadGraph([]byte(`{
		"nodes": [{"id": "number", "type": "phase4_number_test", "config": {"count": 7}}],
		"edges": [
			{"from": "__start__", "to": "number"},
			{"from": "number", "to": "__end__"}
		],
		"channels": [{"key": "count", "reducer": "add"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}

	state, err := graph.Run(context.Background(), State{})
	if err != nil {
		t.Fatal(err)
	}
	if state.Data["count"] != 7 {
		t.Fatalf("count = %v, want 7", state.Data["count"])
	}
}

func TestStreamEmitsPhase3SuperSteps(t *testing.T) {
	graph := buildPhase4HandWrittenGraph()
	events := graph.Stream(context.Background(), State{})

	var received []StepEvent
	for event := range events {
		received = append(received, event)
	}

	if len(received) != 3 {
		t.Fatalf("received %d events, want 3", len(received))
	}

	wantActive := [][]string{
		{"dispatch"},
		{"Race Car1", "Race Car2", "Race Car3"},
		{"join"},
	}
	for i, event := range received {
		if event.Step != i {
			t.Errorf("event %d has step %d, want %d", i, event.Step, i)
		}
		if strings.Join(event.Active, "|") != strings.Join(wantActive[i], "|") {
			t.Errorf("event %d active = %v, want %v", i, event.Active, wantActive[i])
		}
	}

	if received[0].State.Data["messages"].([]any)[0] != "dispatch: starting race cars" {
		t.Fatalf("first event state = %v", received[0].State.Data)
	}
	if received[1].State.Data["done"] != 3 {
		t.Fatalf("second event done = %v, want 3", received[1].State.Data["done"])
	}
	if len(received[2].State.Data["messages"].([]any)) != 5 {
		t.Fatalf("final event messages = %v, want 5 messages", received[2].State.Data["messages"])
	}
}

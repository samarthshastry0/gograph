package main

import (
	"context"
	"fmt"
	"sync"
)

type StepEvent struct {
	Step   int      `json:"step"`
	Active []string `json:"active"`
	State  State    `json:"state"`
}

func (g *Graph) execute(ctx context.Context, initial State, emit func(StepEvent) error) (State, error) {
	if err := g.Compile(); err != nil {
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

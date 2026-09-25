package main

import (
	"context"
	"fmt"
)

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
		raceCar := fmt.Sprintf("Race Car%d", i+1)
		g.AddNode(raceCar, func(ctx context.Context, s State) (State, error) {
			return State{Data: map[string]any{
				"messages": []any{raceCar + ": finished"},
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
		raceCar := fmt.Sprintf("Race Car%d", i+1)
		g.AddEdge("dispatch", raceCar)
		g.AddEdge(raceCar, "join")
	}
	g.AddEdge("join", END)

	final, err := g.Run(context.Background(), State{Data: map[string]any{}})
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Println("messages (append reducer; race car order may vary):")
	for _, message := range final.Data["messages"].([]any) {
		fmt.Println("  -", message)
	}
	fmt.Println("done (add reducer counts race car completions):", final.Data["done"])
}

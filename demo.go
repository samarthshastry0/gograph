package gograph

import (
	"context"
	"fmt"
)

func RegisterDemoTypes() error {
	if err := RegisterNodeType("dispatch", func(config map[string]any) (NodeFunc, error) {
		return func(ctx context.Context, state State) (State, error) {
			return State{Data: map[string]any{
				"messages": []any{"dispatch: starting race cars"},
			}}, nil
		}, nil
	}); err != nil {
		return err
	}

	if err := RegisterNodeType("race_car", func(config map[string]any) (NodeFunc, error) {
		name, ok := config["name"].(string)
		if !ok || name == "" {
			return nil, fmt.Errorf("race_car requires a non-empty string name")
		}

		return func(ctx context.Context, state State) (State, error) {
			return State{Data: map[string]any{
				"messages": []any{name + ": finished"},
				"done":     1,
			}}, nil
		}, nil
	}); err != nil {
		return err
	}

	if err := RegisterNodeType("join", func(config map[string]any) (NodeFunc, error) {
		return func(ctx context.Context, state State) (State, error) {
			return State{Data: map[string]any{
				"messages": []any{"join: all race cars finished"},
			}}, nil
		}, nil
	}); err != nil {
		return err
	}

	return nil
}

package gograph

import (
	"errors"
	"fmt"
	"strings"
)

type NodeFactory func(config map[string]any) (NodeFunc, error)

var reducerRegistry = map[string]Reducer{
	"overwrite": OverwriteReducer,
	"append":    AppendReducer,
	"add":       AddReducer,
}

var nodeRegistry = map[string]NodeFactory{}
var routerRegistry = map[string]RouterFunc{}

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

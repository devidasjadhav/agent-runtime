package tool

import (
	"context"
	"encoding/json"
)

type ToolSchema struct {
	Type       string                        `json:"type"`
	Properties map[string]ToolPropertySchema `json:"properties"`
	Required   []string                      `json:"required"`
}

type ToolPropertySchema struct {
	Type        string               `json:"type"`
	Description string               `json:"description,omitempty"`
	Enum        []string             `json:"enum,omitempty"`
	Default     any                  `json:"default,omitempty"`
	Items       *ToolPropertySchema  `json:"items,omitempty"`
}

type ToolDefinition struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  ToolSchema `json:"parameters"`
}

type Tool interface {
	Name() string
	Description() string
	Parameters() ToolSchema
	Execute(ctx context.Context, args json.RawMessage) (Result, error)
}

type Result struct {
	Content string
	Error   bool
}

type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) All() []Tool {
	var result []Tool
	for _, t := range r.tools {
		result = append(result, t)
	}
	return result
}

func (r *Registry) Definitions() []ToolDefinition {
	var defs []ToolDefinition
	for _, t := range r.tools {
		defs = append(defs, ToolDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		})
	}
	return defs
}

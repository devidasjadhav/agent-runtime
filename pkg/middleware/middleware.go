package middleware

import (
	"context"
	"encoding/json"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
)

type Middleware interface {
	BeforeModel(ctx context.Context, state *State) (*State, error)
	AfterModel(ctx context.Context, state *State, resp *ModelResult) (*State, error)
	BeforeTool(ctx context.Context, call *ToolCall) (*ToolCall, error)
	AfterTool(ctx context.Context, call *ToolCall, result tool.Result) (tool.Result, error)
}

type State struct {
	Messages               []any
	Metadata               map[string]any
	ToolCalls              int
	SystemPromptExtensions []string // appended to system prompt by the agent loop
}

type ModelResult struct {
	Content    string
	ToolCalls  []ToolCall
	StopReason string
}

type ToolCall struct {
	ID   string
	Name string
	Args json.RawMessage
}

type Chain []Middleware

func NewChain(middlewares ...Middleware) Chain {
	return Chain(middlewares)
}

func (c Chain) BeforeModel(ctx context.Context, state *State) (*State, error) {
	var err error
	for _, m := range c {
		state, err = m.BeforeModel(ctx, state)
		if err != nil {
			return state, err
		}
	}
	return state, nil
}

func (c Chain) AfterModel(ctx context.Context, state *State, resp *ModelResult) (*State, error) {
	var err error
	for i := len(c) - 1; i >= 0; i-- {
		state, err = c[i].AfterModel(ctx, state, resp)
		if err != nil {
			return state, err
		}
	}
	return state, nil
}

func (c Chain) BeforeTool(ctx context.Context, call *ToolCall) (*ToolCall, error) {
	var err error
	for _, m := range c {
		call, err = m.BeforeTool(ctx, call)
		if err != nil {
			return call, err
		}
	}
	return call, nil
}

func (c Chain) AfterTool(ctx context.Context, call *ToolCall, result tool.Result) (tool.Result, error) {
	var err error
	for i := len(c) - 1; i >= 0; i-- {
		result, err = c[i].AfterTool(ctx, call, result)
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

type noopMiddleware struct{}

func (noopMiddleware) BeforeModel(_ context.Context, state *State) (*State, error) { return state, nil }
func (noopMiddleware) AfterModel(_ context.Context, state *State, _ *ModelResult) (*State, error) {
	return state, nil
}
func (noopMiddleware) BeforeTool(_ context.Context, call *ToolCall) (*ToolCall, error) {
	return call, nil
}
func (noopMiddleware) AfterTool(_ context.Context, _ *ToolCall, result tool.Result) (tool.Result, error) {
	return result, nil
}

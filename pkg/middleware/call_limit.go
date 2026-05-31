package middleware

import (
	"context"
	"encoding/json"
	"fmt"
)

type CallLimit struct {
	MaxCalls int
}

func NewCallLimit(maxCalls int) *CallLimit {
	return &CallLimit{MaxCalls: maxCalls}
}

func (m *CallLimit) BeforeModel(_ context.Context, state *State) (*State, error) {
	if state.ToolCalls >= m.MaxCalls {
		return nil, fmt.Errorf("model call limit reached (%d)", m.MaxCalls)
	}
	return state, nil
}

func (m *CallLimit) AfterModel(_ context.Context, state *State, _ *ModelResult) (*State, error) {
	return state, nil
}

func (m *CallLimit) BeforeTool(_ context.Context, call *ToolCall) (*ToolCall, error) {
	return call, nil
}

func (m *CallLimit) AfterTool(_ context.Context, _ *ToolCall, result json.RawMessage) (json.RawMessage, error) {
	return result, nil
}

type ErrorHandler struct{}

func NewErrorHandler() *ErrorHandler { return &ErrorHandler{} }

func (m *ErrorHandler) BeforeModel(_ context.Context, state *State) (*State, error) {
	return state, nil
}

func (m *ErrorHandler) AfterModel(_ context.Context, state *State, _ *ModelResult) (*State, error) {
	return state, nil
}

func (m *ErrorHandler) BeforeTool(_ context.Context, call *ToolCall) (*ToolCall, error) {
	return call, nil
}

func (m *ErrorHandler) AfterTool(_ context.Context, call *ToolCall, result json.RawMessage) (json.RawMessage, error) {
	return result, nil
}

type ToolErrorResult struct {
	Error string `json:"error"`
}

func WrapToolError(callName string, err error) json.RawMessage {
	b, _ := json.Marshal(map[string]any{
		"error": fmt.Sprintf("tool %s failed: %s", callName, err.Error()),
	})
	return b
}

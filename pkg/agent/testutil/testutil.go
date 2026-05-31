package testutil

import (
	"context"
	"encoding/json"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/model"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
)

type FakeProvider struct {
	Responses []FakeResponse
	callIndex int
}

type FakeResponse struct {
	Content   string
	ToolCalls []model.ToolCall
	Stop      bool
}

func (f *FakeProvider) Complete(_ context.Context, req model.ModelRequest) (*model.ModelResponse, error) {
	if f.callIndex >= len(f.Responses) {
		return &model.ModelResponse{
			Message:    model.Message{Role: model.RoleAssistant, Content: "done"},
			StopReason: "stop",
		}, nil
	}
	resp := f.Responses[f.callIndex]
	f.callIndex++

	msg := model.Message{
		Role:      model.RoleAssistant,
		Content:   resp.Content,
		ToolCalls: resp.ToolCalls,
	}

	stopReason := "stop"
	if len(resp.ToolCalls) > 0 {
		stopReason = "tool_call"
	}

	return &model.ModelResponse{
		Message:    msg,
		StopReason: stopReason,
		Usage:      model.Usage{InputTokens: 10, OutputTokens: 20},
	}, nil
}

func (f *FakeProvider) Stream(ctx context.Context, req model.ModelRequest) (<-chan model.ModelChunk, error) {
	resp, err := f.Complete(ctx, req)
	if err != nil {
		return nil, err
	}

	ch := make(chan model.ModelChunk, 64)
	go func() {
		defer close(ch)
		if resp.Message.Content != "" {
			ch <- model.ModelChunk{Type: "content", Content: resp.Message.Content}
		}
		for _, tc := range resp.Message.ToolCalls {
			ch <- model.ModelChunk{Type: "tool_call_start", ToolCallID: tc.ID, ToolName: tc.Name}
			ch <- model.ModelChunk{Type: "tool_call_args", ToolCallID: tc.ID, ToolArgs: tc.Arguments}
		}
		ch <- model.ModelChunk{Type: "done", Done: true}
	}()
	return ch, nil
}

type FakeTool struct {
	name        string
	description string
	params      json.RawMessage
	lastArgs    json.RawMessage
	callCount   int
	result      json.RawMessage
}

func NewFakeTool(name string, result json.RawMessage) *FakeTool {
	return &FakeTool{
		name:   name,
		result: result,
	}
}

func (t *FakeTool) Name() string        { return t.name }
func (t *FakeTool) Description() string { return t.description }
func (t *FakeTool) Parameters() tool.ToolSchema {
	return tool.ToolSchema{
		Type: "object",
		Properties: map[string]tool.ToolPropertySchema{
			"input": {Type: "string", Description: "test input"},
		},
		Required: []string{"input"},
	}
}
func (t *FakeTool) Execute(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
	t.lastArgs = args
	t.callCount++
	return t.result, nil
}

func (t *FakeTool) CallCount() int { return t.callCount }

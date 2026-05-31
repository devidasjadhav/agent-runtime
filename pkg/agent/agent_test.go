package agent_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/agent"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/agent/testutil"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/model"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
)

func TestAgent_SimpleCompletion(t *testing.T) {
	provider := &testutil.FakeProvider{
		Responses: []testutil.FakeResponse{
			{Content: "Hello! I can help with that.", Stop: true},
		},
	}

	registry := tool.NewRegistry()
	ag := agent.New(provider, registry,
		agent.WithMaxIterations(5),
	)

	events, err := ag.Run(context.Background(), agent.Input{
		Messages: []model.Message{
			{Role: model.RoleUser, Content: "Say hello"},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var completed bool
	for evt := range events {
		if evt.Type == "completed" {
			completed = true
			if evt.Content != "Hello! I can help with that." {
				t.Errorf("expected completion content, got: %s", evt.Content)
			}
		}
	}

	if !completed {
		t.Fatal("expected completed event")
	}
}

func TestAgent_ToolCallLoop(t *testing.T) {
	writeResult, _ := json.Marshal(map[string]string{"success": "true", "path": "/tmp/test.txt"})
	writeTool := testutil.NewFakeTool("write_file", writeResult)

	provider := &testutil.FakeProvider{
		Responses: []testutil.FakeResponse{
			{
				ToolCalls: []model.ToolCall{
					{ID: "call_1", Name: "write_file", Arguments: `{"file_path":"/tmp/test.txt","content":"hello world"}`},
				},
			},
			{Content: "I created the file.", Stop: true},
		},
	}

	registry := tool.NewRegistry()
	registry.Register(writeTool)

	ag := agent.New(provider, registry,
		agent.WithMaxIterations(5),
	)

	events, err := ag.Run(context.Background(), agent.Input{
		Messages: []model.Message{
			{Role: model.RoleUser, Content: "Create a file"},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var toolCalls, toolResults, completed int
	for evt := range events {
		switch evt.Type {
		case "tool_call":
			toolCalls++
			if evt.ToolCall.Name != "write_file" {
				t.Errorf("expected write_file, got: %s", evt.ToolCall.Name)
			}
		case "tool_result":
			toolResults++
		case "completed":
			completed++
		}
	}

	if toolCalls != 1 {
		t.Errorf("expected 1 tool call, got %d", toolCalls)
	}
	if toolResults != 1 {
		t.Errorf("expected 1 tool result, got %d", toolResults)
	}
	if completed != 1 {
		t.Errorf("expected 1 completed, got %d", completed)
	}
	if writeTool.CallCount() != 1 {
		t.Errorf("expected write_file called once, got %d", writeTool.CallCount())
	}
}

func TestAgent_MultipleToolCalls(t *testing.T) {
	result1, _ := json.Marshal(map[string]string{"success": "true"})
	result2, _ := json.Marshal(map[string]string{"success": "true"})
	tool1 := testutil.NewFakeTool("write_file", result1)
	tool2 := testutil.NewFakeTool("execute", result2)

	provider := &testutil.FakeProvider{
		Responses: []testutil.FakeResponse{
			{
				ToolCalls: []model.ToolCall{
					{ID: "call_1", Name: "write_file", Arguments: `{"file_path":"/tmp/a.txt","content":"a"}`},
					{ID: "call_2", Name: "execute", Arguments: `{"command":"cat /tmp/a.txt"}`},
				},
			},
			{Content: "Done!", Stop: true},
		},
	}

	registry := tool.NewRegistry()
	registry.Register(tool1)
	registry.Register(tool2)

	ag := agent.New(provider, registry,
		agent.WithMaxIterations(5),
	)

	events, err := ag.Run(context.Background(), agent.Input{
		Messages: []model.Message{
			{Role: model.RoleUser, Content: "Write a file and cat it"},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var toolCalls int
	for evt := range events {
		if evt.Type == "tool_call" {
			toolCalls++
		}
	}

	if toolCalls != 2 {
		t.Errorf("expected 2 tool calls, got %d", toolCalls)
	}
	if tool1.CallCount() != 1 {
		t.Errorf("expected write_file called once, got %d", tool1.CallCount())
	}
	if tool2.CallCount() != 1 {
		t.Errorf("expected execute called once, got %d", tool2.CallCount())
	}
}

func TestAgent_MaxIterations(t *testing.T) {
	result, _ := json.Marshal(map[string]string{"success": "true"})
	writeTool := testutil.NewFakeTool("write_file", result)

	provider := &testutil.FakeProvider{
		Responses: []testutil.FakeResponse{
			{ToolCalls: []model.ToolCall{{ID: "c1", Name: "write_file", Arguments: `{}`}}},
			{ToolCalls: []model.ToolCall{{ID: "c2", Name: "write_file", Arguments: `{}`}}},
			{ToolCalls: []model.ToolCall{{ID: "c3", Name: "write_file", Arguments: `{}`}}},
		},
	}

	registry := tool.NewRegistry()
	registry.Register(writeTool)

	ag := agent.New(provider, registry,
		agent.WithMaxIterations(2),
	)

	events, err := ag.Run(context.Background(), agent.Input{
		Messages: []model.Message{
			{Role: model.RoleUser, Content: "loop forever"},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var gotError bool
	for evt := range events {
		if evt.Type == "error" && evt.Content == "max iterations reached" {
			gotError = true
		}
	}

	if !gotError {
		t.Fatal("expected max iterations error")
	}
}

func TestAgent_UnknownTool(t *testing.T) {
	provider := &testutil.FakeProvider{
		Responses: []testutil.FakeResponse{
			{
				ToolCalls: []model.ToolCall{
					{ID: "c1", Name: "nonexistent_tool", Arguments: `{}`},
				},
			},
			{Content: "OK, the tool didn't work.", Stop: true},
		},
	}

	registry := tool.NewRegistry()
	ag := agent.New(provider, registry, agent.WithMaxIterations(5))

	events, err := ag.Run(context.Background(), agent.Input{
		Messages: []model.Message{
			{Role: model.RoleUser, Content: "use a bad tool"},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var toolResultContent string
	for evt := range events {
		if evt.Type == "tool_result" && evt.ToolResult != nil {
			toolResultContent = evt.ToolResult.Output
		}
	}

	if toolResultContent == "" {
		t.Fatal("expected tool result with error")
	}
	var parsed map[string]string
	json.Unmarshal([]byte(toolResultContent), &parsed)
	if parsed["error"] == "" {
		t.Fatal("expected error in tool result for unknown tool")
	}
}

func TestAgent_Streaming(t *testing.T) {
	result, _ := json.Marshal(map[string]string{"success": "true"})
	writeTool := testutil.NewFakeTool("write_file", result)

	provider := &testutil.FakeProvider{
		Responses: []testutil.FakeResponse{
			{Content: "Hello world", Stop: true},
		},
	}

	registry := tool.NewRegistry()
	registry.Register(writeTool)

	ag := agent.New(provider, registry, agent.WithMaxIterations(5))

	events, err := ag.RunStreaming(context.Background(), agent.Input{
		Messages: []model.Message{
			{Role: model.RoleUser, Content: "Say hello"},
		},
	})
	if err != nil {
		t.Fatalf("RunStreaming: %v", err)
	}

	var gotContent bool
	for evt := range events {
		if evt.Type == "content" && evt.Content == "Hello world" {
			gotContent = true
		}
		if evt.Type == "text_delta" && evt.Content != "" {
			gotContent = true
		}
	}

	if !gotContent {
		t.Fatal("expected content in streaming mode")
	}
}

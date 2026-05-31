package agent_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/agent"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/agent/testutil"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/model"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/sandbox/local"
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
	if toolResultContent != "Error: unknown tool: nonexistent_tool" {
		t.Fatal("expected error in tool result for unknown tool")
	}
}

func TestAgent_OffloadsLargeToolResult(t *testing.T) {
	dir := t.TempDir()
	sbx, err := local.New(dir)
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}

	largeContent := strings.Join([]string{
		"line 1",
		"line 2",
		"line 3",
		"line 4",
		"line 5",
		"line 6",
		"line 7",
		"line 8",
		"line 9",
		"line 10",
		"line 11",
	}, "\n")
	largeTool := testutil.NewFakeTool("execute", json.RawMessage(largeContent))

	provider := &testutil.FakeProvider{
		Responses: []testutil.FakeResponse{
			{ToolCalls: []model.ToolCall{{ID: "call/big", Name: "execute", Arguments: `{}`}}},
			{Content: "done", Stop: true},
		},
	}

	registry := tool.NewRegistry()
	registry.Register(largeTool)

	ag := agent.New(provider, registry,
		agent.WithMaxIterations(5),
		agent.WithResultOffload(sbx, 20),
	)

	events, err := ag.Run(context.Background(), agent.Input{
		Messages: []model.Message{{Role: model.RoleUser, Content: "run large tool"}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var toolResult string
	for evt := range events {
		if evt.Type == "tool_result" && evt.ToolResult != nil {
			toolResult = evt.ToolResult.Output
		}
	}

	if !strings.Contains(toolResult, "Tool result too large") {
		t.Fatalf("expected offload preview, got %q", toolResult)
	}
	if !strings.Contains(toolResult, "/large_tool_results/call_big") {
		t.Fatalf("expected sanitized offload path, got %q", toolResult)
	}
	if !strings.Contains(toolResult, "line 1") || !strings.Contains(toolResult, "line 11") {
		t.Fatalf("expected first and last preview lines, got %q", toolResult)
	}

	savedPath := filepath.Join(dir, "large_tool_results", "call_big")
	saved, err := os.ReadFile(savedPath)
	if err != nil {
		t.Fatalf("read saved offload file: %v", err)
	}
	if string(saved) != largeContent {
		t.Fatalf("saved content mismatch\nexpected: %q\nactual:   %q", largeContent, string(saved))
	}
}

func TestAgent_OffloadsLargeHumanMessage(t *testing.T) {
	dir := t.TempDir()
	sbx, err := local.New(dir)
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}

	provider := &testutil.FakeProvider{
		Responses: []testutil.FakeResponse{{Content: "done", Stop: true}},
	}
	registry := tool.NewRegistry()
	ag := agent.New(provider, registry,
		agent.WithResultOffload(sbx, 80_000),
		agent.WithHumanMessageOffloadLimit(20),
	)

	largeMessage := "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10\nline 11"
	events, err := ag.Run(context.Background(), agent.Input{
		Messages: []model.Message{{Role: model.RoleUser, Content: largeMessage}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for range events {
	}

	if len(provider.Requests) == 0 {
		t.Fatal("expected provider request")
	}
	content := provider.Requests[0].Messages[0].Content
	if !strings.Contains(content, "The user message was too large") || !strings.Contains(content, "/conversation_history/") {
		t.Fatalf("expected offloaded human message preview, got %q", content)
	}

	files, err := os.ReadDir(filepath.Join(dir, "conversation_history"))
	if err != nil {
		t.Fatalf("read conversation_history: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one offloaded message file, got %d", len(files))
	}
	saved, err := os.ReadFile(filepath.Join(dir, "conversation_history", files[0].Name()))
	if err != nil {
		t.Fatalf("read offloaded message: %v", err)
	}
	if string(saved) != largeMessage {
		t.Fatalf("saved human message mismatch")
	}
}

func TestAgent_CompleteFallsBackOnTransientProviderError(t *testing.T) {
	primary := &testutil.FakeProvider{
		Err: &model.ProviderError{Provider: "primary", Category: model.ErrorCategoryRateLimit, Message: "rate limited"},
	}
	fallback := &testutil.FakeProvider{
		Responses: []testutil.FakeResponse{{Content: "fallback response", Stop: true}},
	}

	ag := agent.New(primary, tool.NewRegistry(),
		agent.WithModelID("primary-model"),
		agent.WithFallbackProvider(fallback, "fallback-model"),
	)

	events, err := ag.Run(context.Background(), agent.Input{
		Messages: []model.Message{{Role: model.RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var gotFallbackEvent bool
	var completed string
	for evt := range events {
		if evt.Type == "model_fallback" {
			gotFallbackEvent = true
		}
		if evt.Type == "completed" {
			completed = evt.Content
		}
	}

	if !gotFallbackEvent {
		t.Fatal("expected model_fallback event")
	}
	if completed != "fallback response" {
		t.Fatalf("expected fallback response, got %q", completed)
	}
	if len(fallback.Requests) != 1 || fallback.Requests[0].Model != "fallback-model" {
		t.Fatalf("expected fallback request with fallback model, got %#v", fallback.Requests)
	}
}

func TestAgent_DoesNotFallbackOnValidationError(t *testing.T) {
	primary := &testutil.FakeProvider{
		Err: &model.ProviderError{Provider: "primary", Category: model.ErrorCategoryValidation, Message: "bad request"},
	}
	fallback := &testutil.FakeProvider{
		Responses: []testutil.FakeResponse{{Content: "fallback response", Stop: true}},
	}

	ag := agent.New(primary, tool.NewRegistry(), agent.WithFallbackProvider(fallback, "fallback-model"))
	events, err := ag.Run(context.Background(), agent.Input{
		Messages: []model.Message{{Role: model.RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var gotError bool
	for evt := range events {
		if evt.Type == "error" {
			gotError = true
		}
	}
	if !gotError {
		t.Fatal("expected error event")
	}
	if len(fallback.Requests) != 0 {
		t.Fatalf("fallback should not be called for validation errors")
	}
}

func TestAgent_StreamFallsBackOnTransientProviderError(t *testing.T) {
	primary := &testutil.FakeProvider{
		StreamErr: &model.ProviderError{Provider: "primary", Category: model.ErrorCategoryTimeout, Message: "timeout"},
	}
	fallback := &testutil.FakeProvider{
		StreamChunks: []model.ModelChunk{{Type: "content", Content: "fallback stream"}, {Type: "done", Done: true}},
	}

	ag := agent.New(primary, tool.NewRegistry(), agent.WithFallbackProvider(fallback, "fallback-model"))
	events, err := ag.RunStreaming(context.Background(), agent.Input{
		Messages: []model.Message{{Role: model.RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("RunStreaming: %v", err)
	}

	var gotFallbackEvent bool
	var completed string
	for evt := range events {
		if evt.Type == "model_fallback" {
			gotFallbackEvent = true
		}
		if evt.Type == "completed" {
			completed = evt.Content
		}
	}
	if !gotFallbackEvent {
		t.Fatal("expected model_fallback event")
	}
	if completed != "fallback stream" {
		t.Fatalf("expected fallback stream completion, got %q", completed)
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

func TestAgent_StreamingInterleavedToolCalls(t *testing.T) {
	writeResult, _ := json.Marshal(map[string]string{"ok": "write"})
	executeResult, _ := json.Marshal(map[string]string{"ok": "execute"})
	writeTool := testutil.NewFakeTool("write_file", writeResult)
	executeTool := testutil.NewFakeTool("execute", executeResult)

	provider := &testutil.FakeProvider{
		StreamChunks: []model.ModelChunk{
			{Type: "tool_call_start", ToolIndex: 0, ToolCallID: "call_write", ToolName: "write_file"},
			{Type: "tool_call_start", ToolIndex: 1, ToolCallID: "call_exec", ToolName: "execute"},
			{Type: "tool_call_args", ToolIndex: 0, ToolCallID: "call_write", ToolArgs: `{"file_path":"a.txt",`},
			{Type: "tool_call_args", ToolIndex: 1, ToolCallID: "call_exec", ToolArgs: `{"command":"`},
			{Type: "tool_call_args", ToolIndex: 0, ToolCallID: "call_write", ToolArgs: `"content":"hello"}`},
			{Type: "tool_call_args", ToolIndex: 1, ToolCallID: "call_exec", ToolArgs: `cat a.txt"}`},
			{Type: "done", Done: true},
		},
	}

	registry := tool.NewRegistry()
	registry.Register(writeTool)
	registry.Register(executeTool)

	ag := agent.New(provider, registry, agent.WithMaxIterations(1))
	events, err := ag.RunStreaming(context.Background(), agent.Input{
		Messages: []model.Message{{Role: model.RoleUser, Content: "test"}},
	})
	if err != nil {
		t.Fatalf("RunStreaming: %v", err)
	}

	var calls []agent.ToolCallEvent
	for evt := range events {
		if evt.Type == "tool_executing" && evt.ToolCall != nil {
			calls = append(calls, *evt.ToolCall)
		}
	}

	if len(calls) != 2 {
		t.Fatalf("expected 2 executed tool calls, got %d", len(calls))
	}
	if calls[0].Name != "write_file" || calls[0].Args != `{"file_path":"a.txt","content":"hello"}` {
		t.Fatalf("unexpected first call: %#v", calls[0])
	}
	if calls[1].Name != "execute" || calls[1].Args != `{"command":"cat a.txt"}` {
		t.Fatalf("unexpected second call: %#v", calls[1])
	}
	if writeTool.CallCount() != 1 || executeTool.CallCount() != 1 {
		t.Fatalf("expected both tools to run once, got write=%d execute=%d", writeTool.CallCount(), executeTool.CallCount())
	}
}

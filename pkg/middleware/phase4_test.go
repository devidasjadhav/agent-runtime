package middleware

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/model"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
)

func TestSanitizeInputs_RemovesControlChars(t *testing.T) {
	s := SanitizeInputs{}

	args := map[string]string{
		"file_path": "/tmp/test.py",
		"content":   "hello\x00world\x01test",
	}
	rawArgs, _ := json.Marshal(args)

	call := &ToolCall{
		ID:   "call_1",
		Name: "write_file",
		Args: rawArgs,
	}

	result, err := s.BeforeTool(context.Background(), call)
	if err != nil {
		t.Fatalf("BeforeTool error: %v", err)
	}

	var parsed map[string]string
	if err := json.Unmarshal(result.Args, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if strings.Contains(parsed["content"], "\x00") {
		t.Error("null byte not removed from content")
	}
	if strings.Contains(parsed["content"], "\x01") {
		t.Error("control char not removed from content")
	}
	if !strings.Contains(parsed["content"], "hello") || !strings.Contains(parsed["content"], "world") {
		t.Errorf("valid content removed: %q", parsed["content"])
	}
}

func TestSanitizeInputs_PreservesValidChars(t *testing.T) {
	s := SanitizeInputs{}

	args := map[string]string{
		"command": "echo 'hello\nworld'",
	}
	rawArgs, _ := json.Marshal(args)

	call := &ToolCall{
		ID:   "call_1",
		Name: "execute",
		Args: rawArgs,
	}

	result, err := s.BeforeTool(context.Background(), call)
	if err != nil {
		t.Fatalf("BeforeTool error: %v", err)
	}

	var parsed map[string]string
	if err := json.Unmarshal(result.Args, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if !strings.Contains(parsed["command"], "hello") || !strings.Contains(parsed["command"], "world") {
		t.Errorf("valid content removed: %q", parsed["command"])
	}
}

func TestSanitizeInputs_NilCall(t *testing.T) {
	s := SanitizeInputs{}
	result, err := s.BeforeTool(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeforeTool error: %v", err)
	}
	if result != nil {
		t.Error("expected nil result for nil call")
	}
}

func TestCircuitBreaker_ClosedState(t *testing.T) {
	cb := NewCircuitBreaker(3, 5*time.Second)

	for i := 0; i < 3; i++ {
		_, err := cb.AfterTool(context.Background(), &ToolCall{Name: "test"}, tool.Result{Content: "ok", Error: false})
		if err != nil {
			t.Fatalf("AfterTool error: %v", err)
		}
	}

	if cb.State() != "closed" {
		t.Errorf("State() = %q, want %q", cb.State(), "closed")
	}
}

func TestCircuitBreaker_OpensOnFailures(t *testing.T) {
	cb := NewCircuitBreaker(3, 5*time.Second)

	for i := 0; i < 3; i++ {
		_, _ = cb.AfterTool(context.Background(), &ToolCall{Name: "test"}, tool.Result{Content: "err", Error: true})
	}

	if cb.State() != "open" {
		t.Errorf("State() = %q after 3 failures, want %q", cb.State(), "open")
	}

	_, err := cb.BeforeModel(context.Background(), &State{})
	if err == nil {
		t.Error("expected error when circuit is open")
	}
}

func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	cb := NewCircuitBreaker(2, 10*time.Millisecond)

	for i := 0; i < 2; i++ {
		_, _ = cb.AfterTool(context.Background(), &ToolCall{Name: "test"}, tool.Result{Error: true})
	}

	if cb.State() != "open" {
		t.Fatalf("State() = %q, want open", cb.State())
	}

	time.Sleep(15 * time.Millisecond)

	_, err := cb.BeforeModel(context.Background(), &State{})
	if err != nil {
		t.Errorf("expected half-open to allow, got error: %v", err)
	}
}

func TestCircuitBreaker_ResetsOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker(2, 5*time.Second)

	_, _ = cb.AfterTool(context.Background(), &ToolCall{}, tool.Result{Error: true})
	if cb.FailureCount() != 1 {
		t.Errorf("FailureCount() = %d, want 1", cb.FailureCount())
	}

	_, _ = cb.AfterTool(context.Background(), &ToolCall{}, tool.Result{Content: "ok"})
	if cb.FailureCount() != 0 {
		t.Errorf("FailureCount() = %d after success, want 0", cb.FailureCount())
	}
}

func TestQueuedMessages_EnqueueAndInject(t *testing.T) {
	q := NewQueuedMessages()

	q.EnqueueMany([]model.Message{
		{Role: model.RoleUser, Content: "hello"},
		{Role: model.RoleAssistant, Content: "hi"},
	})

	if q.Pending() != 2 {
		t.Errorf("Pending() = %d, want 2", q.Pending())
	}

	state := &State{Messages: []any{}}
	result, err := q.BeforeModel(context.Background(), state)
	if err != nil {
		t.Fatalf("BeforeModel error: %v", err)
	}

	if len(result.Messages) != 2 {
		t.Errorf("len(Messages) = %d, want 2", len(result.Messages))
	}

	if q.Pending() != 0 {
		t.Errorf("Pending() after inject = %d, want 0", q.Pending())
	}
	if q.Injected() != 2 {
		t.Errorf("Injected() = %d, want 2", q.Injected())
	}
}

func TestQueuedMessages_EmptyQueue(t *testing.T) {
	q := NewQueuedMessages()
	state := &State{Messages: []any{model.Message{Role: model.RoleUser, Content: "existing"}}}

	result, err := q.BeforeModel(context.Background(), state)
	if err != nil {
		t.Fatalf("BeforeModel error: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Errorf("expected original messages preserved, got %d", len(result.Messages))
	}
}

func TestQueuedMessages_ParseMessage(t *testing.T) {
	msg, err := ParseQueuedMessage(json.RawMessage(`{"role": "user", "content": "test message"}`))
	if err != nil {
		t.Fatalf("ParseQueuedMessage error: %v", err)
	}
	if msg.Content != "test message" {
		t.Errorf("Content = %q, want %q", msg.Content, "test message")
	}
}

func TestQueuedMessages_ParseInvalidMessage(t *testing.T) {
	_, err := ParseQueuedMessage(json.RawMessage(`{"role": ""}`))
	if err == nil {
		t.Error("expected error for missing content")
	}
}

func TestThinkingBlockSanitizer_StripsBlocks(t *testing.T) {
	s := NewThinkingBlockSanitizer(true)
	resp := &ModelResult{Content: "Hello <thinking>internal thoughts here</thinking> World"}

	_, err := s.AfterModel(context.Background(), &State{}, resp)
	if err != nil {
		t.Fatalf("AfterModel error: %v", err)
	}

	if strings.Contains(resp.Content, "thinking") {
		t.Errorf("thinking block not stripped: %q", resp.Content)
	}
	if !strings.Contains(resp.Content, "Hello") || !strings.Contains(resp.Content, "World") {
		t.Errorf("non-thinking content removed: %q", resp.Content)
	}
}

func TestThinkingBlockSanitizer_PreservesWhenDisabled(t *testing.T) {
	s := NewThinkingBlockSanitizer(false)
	resp := &ModelResult{Content: "Hello <thinking>thoughts</thinking> World"}

	_, _ = s.AfterModel(context.Background(), &State{}, resp)

	if !strings.Contains(resp.Content, "<thinking>") {
		t.Error("thinking block stripped when disabled")
	}
}

func TestThinkingBlockSanitizer_NestedBlocks(t *testing.T) {
	input := "Before <thinking>outer <thinking>inner</thinking> more</thinking> After"
	result := stripThinkingBlocks(input)

	if strings.Contains(result, "thinking") {
		t.Errorf("nested blocks not fully stripped: %q", result)
	}
	if !strings.Contains(result, "Before") || !strings.Contains(result, "After") {
		t.Errorf("non-thinking content removed: %q", result)
	}
}

func TestSanitizeToolCallArgs(t *testing.T) {
	args := json.RawMessage(`{"content": "hello <thinking>bad</thinking> world"}`)
	result := SanitizeToolCallArgs(args)

	var parsed map[string]string
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if strings.Contains(parsed["content"], "thinking") {
		t.Errorf("thinking not stripped: %q", parsed["content"])
	}
}

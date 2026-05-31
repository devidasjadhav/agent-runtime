package builtin

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
)

func TestTodoToolSchema(t *testing.T) {
	todo := NewTodoTool(&TodoState{})
	def := tool.ToolDefinition{
		Name:        todo.Name(),
		Description: todo.Description(),
		Parameters:  todo.Parameters(),
	}

	if def.Name != "write_todos" {
		t.Errorf("Name() = %q, want %q", def.Name, "write_todos")
	}
	if def.Parameters.Type != "object" {
		t.Errorf("Parameters.Type = %q, want %q", def.Parameters.Type, "object")
	}
	if _, ok := def.Parameters.Properties["todos"]; !ok {
		t.Error("missing 'todos' property")
	}
	if len(def.Parameters.Required) != 1 || def.Parameters.Required[0] != "todos" {
		t.Errorf("Required = %v, want [todos]", def.Parameters.Required)
	}
	prop := def.Parameters.Properties["todos"]
	if prop.Type != "array" {
		t.Errorf("todos type = %q, want %q", prop.Type, "array")
	}
}

func TestTodoToolExecute(t *testing.T) {
	state := &TodoState{}
	todo := NewTodoTool(state)

	args, _ := json.Marshal(map[string]any{
		"todos": []map[string]string{
			{"content": "Write tests", "status": "completed"},
			{"content": "Fix bugs", "status": "in_progress"},
			{"content": "Ship it", "status": "pending"},
		},
	})

	result, err := todo.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.Error {
		t.Errorf("unexpected error: %s", result.Content)
	}

	items := state.Get()
	if len(items) != 3 {
		t.Fatalf("state has %d items, want 3", len(items))
	}
	if items[0].Content != "Write tests" {
		t.Errorf("items[0].Content = %q, want %q", items[0].Content, "Write tests")
	}
	if items[1].Status != "in_progress" {
		t.Errorf("items[1].Status = %q, want %q", items[1].Status, "in_progress")
	}

	emptyArgs, _ := json.Marshal(map[string]any{
		"todos": []map[string]string{
			{"content": "", "status": "pending"},
		},
	})
	result2, err := todo.Execute(context.Background(), emptyArgs)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !result2.Error {
		t.Error("expected error for empty content")
	}
}

func TestTodoToolEmptyList(t *testing.T) {
	state := &TodoState{}
	todo := NewTodoTool(state)

	args, _ := json.Marshal(map[string]any{
		"todos": []map[string]string{},
	})

	result, err := todo.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.Error {
		t.Errorf("unexpected error: %s", result.Content)
	}
	if len(state.Get()) != 0 {
		t.Error("expected empty state")
	}
}

func TestTodoToolInvalidStatus(t *testing.T) {
	state := &TodoState{}
	todo := NewTodoTool(state)

	args, _ := json.Marshal(map[string]any{
		"todos": []map[string]string{
			{"content": "test", "status": "unknown"},
		},
	})

	result, err := todo.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !result.Error {
		t.Error("expected error for invalid status")
	}
}

func TestTodoToolInvalidJSON(t *testing.T) {
	state := &TodoState{}
	todo := NewTodoTool(state)

	result, err := todo.Execute(context.Background(), json.RawMessage(`{invalid`))
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !result.Error {
		t.Error("expected error for invalid JSON")
	}
}

func TestFormatTodos(t *testing.T) {
	items := []TodoItem{
		{Content: "First", Status: "completed"},
		{Content: "Second", Status: "in_progress"},
		{Content: "Third", Status: "pending"},
	}

	formatted := FormatTodos(items)
	if len(formatted) == 0 {
		t.Error("FormatTodos returned empty string")
	}

	if !contains(formatted, "● First") {
		t.Errorf("expected completed marker, got: %s", formatted)
	}
	if !contains(formatted, "◉ Second") {
		t.Errorf("expected in_progress marker, got: %s", formatted)
	}
	if !contains(formatted, "○ Third") {
		t.Errorf("expected pending marker, got: %s", formatted)
	}
}

func TestFormatTodosEmpty(t *testing.T) {
	result := FormatTodos(nil)
	if result != "No todos." {
		t.Errorf("FormatTodos(nil) = %q, want %q", result, "No todos.")
	}
}

func TestTodoStateConcurrency(t *testing.T) {
	state := &TodoState{}
	done := make(chan bool)

	go func() {
		for i := 0; i < 100; i++ {
			state.Update([]TodoItem{{Content: "a", Status: "pending"}})
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			_ = state.Get()
		}
		done <- true
	}()

	<-done
	<-done

	items := state.Get()
	if len(items) != 1 || items[0].Content != "a" {
		t.Error("concurrent access corrupted state")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

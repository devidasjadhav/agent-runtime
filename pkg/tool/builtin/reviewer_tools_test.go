package builtin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestAddFindingTool(t *testing.T) {
	store := NewFindingStore()
	tool := NewAddFindingTool().WithStore(store)

	args, _ := json.Marshal(map[string]any{
		"title":     "Bug in auth",
		"body":      "The auth middleware has a null pointer dereference",
		"file_path": "/src/auth.go",
		"line_start": 42,
		"line_end":   45,
		"severity":  "error",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.Error {
		t.Errorf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "finding_1") {
		t.Errorf("expected finding ID in result: %s", result.Content)
	}

	findings := store.List()
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Title != "Bug in auth" {
		t.Errorf("title = %q", findings[0].Title)
	}
	if findings[0].Severity != "error" {
		t.Errorf("severity = %q", findings[0].Severity)
	}
}

func TestUpdateFindingTool(t *testing.T) {
	store := NewFindingStore()
	store.Add(&Finding{Title: "Original", Severity: "info"})

	tool := NewUpdateFindingTool().WithStore(store)
	args, _ := json.Marshal(map[string]any{
		"finding_id": "finding_1",
		"severity":   "warning",
		"title":      "Updated Title",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.Error {
		t.Errorf("unexpected error: %s", result.Content)
	}

	f, _ := store.Get("finding_1")
	if f.Title != "Updated Title" {
		t.Errorf("title = %q, want Updated Title", f.Title)
	}
	if f.Severity != "warning" {
		t.Errorf("severity = %q, want warning", f.Severity)
	}
}

func TestListFindingsTool(t *testing.T) {
	store := NewFindingStore()
	store.Add(&Finding{Title: "Finding A", Severity: "info"})
	store.Add(&Finding{Title: "Finding B", Severity: "error"})

	tool := NewListFindingsTool().WithStore(store)
	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.Error {
		t.Errorf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Finding A") || !strings.Contains(result.Content, "Finding B") {
		t.Errorf("expected both findings: %s", result.Content)
	}
}

func TestPublishReviewTool(t *testing.T) {
	store := NewFindingStore()
	store.Add(&Finding{Title: "Test", Severity: "info"})

	tool := NewPublishReviewTool().WithStore(store)
	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.Error {
		t.Errorf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "1 findings") {
		t.Errorf("expected count in result: %s", result.Content)
	}
}

func TestResolveFindingThreadTool(t *testing.T) {
	store := NewFindingStore()
	store.Add(&Finding{Title: "Test", Status: "open"})

	tool := NewResolveFindingThreadTool().WithStore(store)
	args, _ := json.Marshal(map[string]any{
		"finding_id":  "finding_1",
		"resolved_by": "author",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.Error {
		t.Errorf("unexpected error: %s", result.Content)
	}

	f, _ := store.Get("finding_1")
	if f.Status != "resolved" {
		t.Errorf("status = %q, want resolved", f.Status)
	}
}

func TestReplyToFindingThreadTool(t *testing.T) {
	tool := NewReplyToFindingThreadTool()
	args, _ := json.Marshal(map[string]any{
		"finding_id": "finding_1",
		"body":       "This is intentional",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.Error {
		t.Errorf("unexpected error: %s", result.Content)
	}
}

func TestSaveReviewStylePromptTool(t *testing.T) {
	tool := NewSaveReviewStylePromptTool()
	args, _ := json.Marshal(map[string]any{
		"prompt":    "Be concise and focus on bugs",
		"full_name": "owner/repo",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.Error {
		t.Errorf("unexpected error: %s", result.Content)
	}
}

func TestFindingStoreConcurrency(t *testing.T) {
	store := NewFindingStore()
	done := make(chan bool)

	go func() {
		for i := 0; i < 100; i++ {
			store.Add(&Finding{Title: "concurrent"})
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			_ = store.List()
		}
		done <- true
	}()

	<-done
	<-done

	if len(store.List()) != 100 {
		t.Errorf("expected 100 findings, got %d", len(store.List()))
	}
}

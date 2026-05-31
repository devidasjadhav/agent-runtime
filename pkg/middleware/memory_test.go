package middleware_test

import (
	"context"
	"errors"
	"testing"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/middleware"
)

func makeLoader(files map[string]string) middleware.FileLoaderFunc {
	return func(_ context.Context, path string) ([]byte, error) {
		if content, ok := files[path]; ok {
			return []byte(content), nil
		}
		return nil, errors.New("not found: " + path)
	}
}

func TestMemoryMiddleware_InjectsContent(t *testing.T) {
	loader := makeLoader(map[string]string{
		"/agents.md": "You are a helpful bot.",
	})
	m := middleware.NewMemoryMiddleware(loader, []string{"/agents.md"})

	state, err := m.BeforeModel(context.Background(), &middleware.State{Metadata: map[string]any{}})
	if err != nil {
		t.Fatalf("BeforeModel: %v", err)
	}
	if len(state.SystemPromptExtensions) != 1 {
		t.Fatalf("expected 1 extension, got %d", len(state.SystemPromptExtensions))
	}
	if state.SystemPromptExtensions[0] != "You are a helpful bot." {
		t.Fatalf("unexpected extension: %q", state.SystemPromptExtensions[0])
	}
}

func TestMemoryMiddleware_MultipleSourcesMerged(t *testing.T) {
	loader := makeLoader(map[string]string{
		"/a.md": "rule A",
		"/b.md": "rule B",
	})
	m := middleware.NewMemoryMiddleware(loader, []string{"/a.md", "/b.md"})

	state, _ := m.BeforeModel(context.Background(), &middleware.State{Metadata: map[string]any{}})
	if len(state.SystemPromptExtensions) != 2 {
		t.Fatalf("expected 2 extensions, got %d", len(state.SystemPromptExtensions))
	}
}

func TestMemoryMiddleware_MissingSourceSkipped(t *testing.T) {
	loader := makeLoader(map[string]string{
		"/exists.md": "real content",
	})
	m := middleware.NewMemoryMiddleware(loader, []string{"/missing.md", "/exists.md"})

	state, _ := m.BeforeModel(context.Background(), &middleware.State{Metadata: map[string]any{}})
	if len(state.SystemPromptExtensions) != 1 {
		t.Fatalf("expected 1 extension (missing skipped), got %d", len(state.SystemPromptExtensions))
	}
	if state.SystemPromptExtensions[0] != "real content" {
		t.Fatalf("unexpected extension: %q", state.SystemPromptExtensions[0])
	}
}

func TestMemoryMiddleware_HTMLCommentsStripped(t *testing.T) {
	loader := makeLoader(map[string]string{
		"/agents.md": "visible <!-- hidden --> text",
	})
	m := middleware.NewMemoryMiddleware(loader, []string{"/agents.md"})

	state, _ := m.BeforeModel(context.Background(), &middleware.State{Metadata: map[string]any{}})
	got := state.SystemPromptExtensions[0]
	if got != "visible  text" {
		t.Fatalf("HTML comment not stripped: %q", got)
	}
}

func TestMemoryMiddleware_HTMLCommentStrippingDisabled(t *testing.T) {
	loader := makeLoader(map[string]string{
		"/agents.md": "visible <!-- keep --> text",
	})
	m := middleware.NewMemoryMiddleware(loader, []string{"/agents.md"},
		middleware.WithHTMLCommentStripping(false))

	state, _ := m.BeforeModel(context.Background(), &middleware.State{Metadata: map[string]any{}})
	got := state.SystemPromptExtensions[0]
	if got != "visible <!-- keep --> text" {
		t.Fatalf("expected comment preserved, got: %q", got)
	}
}

func TestMemoryMiddleware_LoadsOnce(t *testing.T) {
	calls := 0
	loader := middleware.FileLoaderFunc(func(_ context.Context, _ string) ([]byte, error) {
		calls++
		return []byte("content"), nil
	})
	m := middleware.NewMemoryMiddleware(loader, []string{"/f.md"})
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		m.BeforeModel(ctx, &middleware.State{Metadata: map[string]any{}})
	}
	if calls != 1 {
		t.Fatalf("expected loader called once, got %d", calls)
	}
}

func TestMemoryMiddleware_EmptySourcesNoExtensions(t *testing.T) {
	m := middleware.NewMemoryMiddleware(makeLoader(nil), nil)
	state, _ := m.BeforeModel(context.Background(), &middleware.State{Metadata: map[string]any{}})
	if len(state.SystemPromptExtensions) != 0 {
		t.Fatalf("expected no extensions, got %d", len(state.SystemPromptExtensions))
	}
}

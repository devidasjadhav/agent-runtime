package agent_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/agent"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/sandbox"
)

type mockStore struct {
	files map[string][]byte
}

func (m *mockStore) WriteFile(_ context.Context, path string, content []byte) (sandbox.WriteResult, error) {
	if m.files == nil {
		m.files = make(map[string][]byte)
	}
	m.files[path] = content
	return sandbox.WriteResult{}, nil
}

func TestRetentionStore_EvictsOldestWhenOverLimit(t *testing.T) {
	inner := &mockStore{}
	var deleted []string
	store := agent.NewRetentionStore(inner, func(_ context.Context, p string) error {
		deleted = append(deleted, p)
		return nil
	}, 3)

	ctx := context.Background()
	for i := 1; i <= 5; i++ {
		store.WriteFile(ctx, fmt.Sprintf("/large_tool_results/r%d", i), []byte("x"))
	}

	if len(deleted) != 2 {
		t.Fatalf("expected 2 evictions, got %d: %v", len(deleted), deleted)
	}
	if deleted[0] != "/large_tool_results/r1" || deleted[1] != "/large_tool_results/r2" {
		t.Fatalf("wrong eviction order: %v", deleted)
	}
	if store.Len() != 3 {
		t.Fatalf("expected 3 tracked files, got %d", store.Len())
	}
}

func TestRetentionStore_NoEvictionUnderLimit(t *testing.T) {
	inner := &mockStore{}
	var deleted []string
	store := agent.NewRetentionStore(inner, func(_ context.Context, p string) error {
		deleted = append(deleted, p)
		return nil
	}, 10)

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		store.WriteFile(ctx, fmt.Sprintf("/r/%d", i), []byte("x"))
	}

	if len(deleted) != 0 {
		t.Fatalf("expected no evictions, got %d", len(deleted))
	}
}

func TestRetentionStore_ZeroMaxDisablesEviction(t *testing.T) {
	inner := &mockStore{}
	evicted := 0
	store := agent.NewRetentionStore(inner, func(_ context.Context, _ string) error {
		evicted++
		return nil
	}, 0)

	ctx := context.Background()
	for i := 0; i < 100; i++ {
		store.WriteFile(ctx, fmt.Sprintf("/r/%d", i), []byte("x"))
	}

	if evicted != 0 {
		t.Fatalf("expected no evictions with maxFiles=0, got %d", evicted)
	}
	if store.Len() != 0 {
		t.Fatalf("expected Len=0 when disabled, got %d", store.Len())
	}
}

func TestRetentionStore_ExactLimitNoEviction(t *testing.T) {
	inner := &mockStore{}
	var deleted []string
	store := agent.NewRetentionStore(inner, func(_ context.Context, p string) error {
		deleted = append(deleted, p)
		return nil
	}, 3)

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		store.WriteFile(ctx, fmt.Sprintf("/r/%d", i), []byte("x"))
	}

	if len(deleted) != 0 {
		t.Fatalf("at-limit should not evict; got %v", deleted)
	}
	if store.Len() != 3 {
		t.Fatalf("expected Len=3, got %d", store.Len())
	}
}

func TestRetentionStore_WriteErrorSkipsTracking(t *testing.T) {
	inner := &mockStore{}
	evicted := 0
	store := agent.NewRetentionStore(inner, func(_ context.Context, _ string) error {
		evicted++
		return nil
	}, 2)

	ctx := context.Background()
	// Fill to limit
	store.WriteFile(ctx, "/r/1", []byte("x"))
	store.WriteFile(ctx, "/r/2", []byte("x"))
	if store.Len() != 2 {
		t.Fatalf("expected 2, got %d", store.Len())
	}
	// One more triggers eviction
	store.WriteFile(ctx, "/r/3", []byte("x"))
	if evicted != 1 {
		t.Fatalf("expected 1 eviction, got %d", evicted)
	}
}

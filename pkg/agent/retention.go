package agent

import (
	"context"
	"sync"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/sandbox"
)

// RetentionStore wraps a ResultStore and evicts the oldest offloaded files
// once the tracked count exceeds MaxFiles. Eviction calls the injected deleter
// so callers can swap in a real sandbox exec or a test stub.
type RetentionStore struct {
	inner    ResultStore
	deleter  func(ctx context.Context, path string) error
	maxFiles int
	mu       sync.Mutex
	written  []string
}

// NewRetentionStore creates a RetentionStore with a custom deleter function.
// maxFiles=0 disables eviction entirely.
func NewRetentionStore(inner ResultStore, deleter func(ctx context.Context, path string) error, maxFiles int) *RetentionStore {
	return &RetentionStore{inner: inner, deleter: deleter, maxFiles: maxFiles}
}

// NewSandboxRetentionStore is the production constructor: eviction runs
// "rm -f <path>" via the sandbox Exec method.
func NewSandboxRetentionStore(inner ResultStore, sbx sandbox.Sandbox, maxFiles int) *RetentionStore {
	return NewRetentionStore(inner, func(ctx context.Context, path string) error {
		_, err := sbx.Exec(ctx, "rm -f "+path, nil)
		return err
	}, maxFiles)
}

func (r *RetentionStore) WriteFile(ctx context.Context, path string, content []byte) (sandbox.WriteResult, error) {
	result, err := r.inner.WriteFile(ctx, path, content)
	if err != nil {
		return result, err
	}
	if r.maxFiles > 0 {
		r.evictIfNeeded(ctx, path)
	}
	return result, nil
}

func (r *RetentionStore) evictIfNeeded(ctx context.Context, newPath string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.written = append(r.written, newPath)
	for len(r.written) > r.maxFiles {
		oldest := r.written[0]
		r.written = r.written[1:]
		_ = r.deleter(ctx, oldest)
	}
}

// Len returns the number of currently tracked (non-evicted) files.
func (r *RetentionStore) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.written)
}

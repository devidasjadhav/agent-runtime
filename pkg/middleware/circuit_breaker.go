package middleware

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
)

type CircuitBreaker struct {
	mu           sync.Mutex
	failureCount int
	threshold    int
	resetTimeout time.Duration
	lastFailure  time.Time
	state        string
}

func NewCircuitBreaker(threshold int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		threshold:    threshold,
		resetTimeout: resetTimeout,
		state:        "closed",
	}
}

func (cb *CircuitBreaker) BeforeModel(_ context.Context, state *State) (*State, error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == "open" {
		if time.Since(cb.lastFailure) > cb.resetTimeout {
			cb.state = "half-open"
		} else {
			return state, fmt.Errorf("circuit breaker is open: %d consecutive failures (resets after %s)", cb.failureCount, cb.resetTimeout)
		}
	}

	return state, nil
}

func (cb *CircuitBreaker) AfterModel(_ context.Context, state *State, _ *ModelResult) (*State, error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == "half-open" {
		cb.state = "closed"
		cb.failureCount = 0
	}

	return state, nil
}

func (cb *CircuitBreaker) BeforeTool(_ context.Context, call *ToolCall) (*ToolCall, error) {
	return call, nil
}

func (cb *CircuitBreaker) AfterTool(_ context.Context, _ *ToolCall, result tool.Result) (tool.Result, error) {
	if result.Error {
		cb.recordFailure()
	} else {
		cb.recordSuccess()
	}
	return result, nil
}

func (cb *CircuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failureCount++
	cb.lastFailure = time.Now()
	if cb.failureCount >= cb.threshold {
		cb.state = "open"
	}
}

func (cb *CircuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failureCount = 0
	cb.state = "closed"
}

func (cb *CircuitBreaker) State() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

func (cb *CircuitBreaker) FailureCount() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.failureCount
}

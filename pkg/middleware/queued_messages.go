package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/model"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
)

type QueuedMessages struct {
	mu       sync.Mutex
	queue    []model.Message
	injected int
}

func NewQueuedMessages() *QueuedMessages {
	return &QueuedMessages{}
}

func (q *QueuedMessages) Enqueue(msg model.Message) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queue = append(q.queue, msg)
}

func (q *QueuedMessages) EnqueueMany(msgs []model.Message) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queue = append(q.queue, msgs...)
}

func (q *QueuedMessages) Pending() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.queue)
}

func (q *QueuedMessages) Injected() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.injected
}

func (q *QueuedMessages) BeforeModel(_ context.Context, state *State) (*State, error) {
	q.mu.Lock()
	pending := q.queue
	q.queue = nil
	q.injected += len(pending)
	q.mu.Unlock()

	if len(pending) == 0 {
		return state, nil
	}

	injected := make([]any, 0, len(pending))
	for _, msg := range pending {
		injected = append(injected, msg)
	}

	state.Messages = append(state.Messages, injected...)
	if state.Metadata == nil {
		state.Metadata = make(map[string]any)
	}
	state.Metadata["queued_messages_injected"] = len(pending)

	return state, nil
}

func (q *QueuedMessages) AfterModel(_ context.Context, state *State, _ *ModelResult) (*State, error) {
	return state, nil
}

func (q *QueuedMessages) BeforeTool(_ context.Context, call *ToolCall) (*ToolCall, error) {
	return call, nil
}

func (q *QueuedMessages) AfterTool(_ context.Context, _ *ToolCall, result tool.Result) (tool.Result, error) {
	return result, nil
}

func ParseQueuedMessage(data json.RawMessage) (model.Message, error) {
	var raw struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return model.Message{}, fmt.Errorf("parse queued message: %w", err)
	}
	if raw.Role == "" || raw.Content == "" {
		return model.Message{}, fmt.Errorf("queued message missing role or content")
	}
	return model.Message{Role: model.Role(raw.Role), Content: raw.Content}, nil
}

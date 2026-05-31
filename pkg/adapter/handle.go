package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/agent"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/model"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/sandbox"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool/builtin"
)

type AgentHandle struct {
	Agent       *agent.Agent
	Sandbox     sandbox.Sandbox
	TodoState   *builtin.TodoState
	Checkpoints CheckpointStore
	EventSink   EventSink
}

type RunResult struct {
	ThreadID   string
	Steps      int
	FinalText  string
	ToolCalls  int
	Usage      model.Usage
	TodoItems  []builtin.TodoItem
}

func (h *AgentHandle) Run(ctx context.Context, input agent.Input) (*RunResult, error) {
	events, err := h.Agent.Run(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("agent run: %w", err)
	}

	result := &RunResult{}
	for evt := range events {
		if h.EventSink != nil {
			_ = h.EventSink.Send(ctx, evt.Type, evt)
		}

		if h.Checkpoints != nil {
			_ = h.maybeCheckpoint(ctx, evt)
		}

		switch evt.Type {
		case "completed":
			result.FinalText = evt.Content
		case "tool_result":
			result.ToolCalls++
		case "usage":
			if evt.Usage != nil {
				result.Usage = *evt.Usage
			}
		case "error":
			return result, fmt.Errorf("agent error: %s", evt.Content)
		}
		result.Steps++
	}

	if h.TodoState != nil {
		result.TodoItems = h.TodoState.Get()
	}

	return result, nil
}

func (h *AgentHandle) RunStreaming(ctx context.Context, input agent.Input) (<-chan SSEEvent, error) {
	events, err := h.Agent.RunStreaming(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("agent stream: %w", err)
	}

	out := make(chan SSEEvent, 64)
	go func() {
		defer close(out)
		for evt := range events {
			data, _ := json.Marshal(evt)
			out <- SSEEvent{
				Event: evt.Type,
				Data:  string(data),
			}

			if h.Checkpoints != nil {
				_ = h.maybeCheckpoint(ctx, evt)
			}
		}
	}()

	return out, nil
}

func (h *AgentHandle) maybeCheckpoint(ctx context.Context, evt agent.Event) error {
	if h.Checkpoints == nil {
		return nil
	}

	switch evt.Type {
	case "tool_result", "completed":
		state := CheckpointState{
			Step:      0,
			ToolCalls: 0,
			Metadata:  make(map[string]any),
		}
		if evt.ToolResult != nil {
			state.ToolCalls = 1
			state.Metadata["tool_name"] = evt.ToolResult.Name
		}
		return h.Checkpoints.Save(ctx, state)
	}
	return nil
}

func (h *AgentHandle) Close() error {
	if h.Sandbox != nil {
		return h.Sandbox.Close(context.Background())
	}
	return nil
}

func SSEStreamWriter(w io.Writer) EventSink {
	return &sseWriter{w: w}
}

type sseWriter struct {
	w io.Writer
}

func (s *sseWriter) Send(_ context.Context, eventType string, data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	timestamp := time.Now().UnixMilli()
	_, err = fmt.Fprintf(s.w, "event: %s\ndata: %s\nid: %d\n\n", eventType, string(jsonData), timestamp)
	return err
}

func (s *sseWriter) Close() error {
	if closer, ok := s.w.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

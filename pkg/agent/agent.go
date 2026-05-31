package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/middleware"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/model"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
)

type Agent struct {
	provider      model.Provider
	registry      *tool.Registry
	middlewares   middleware.Chain
	modelID       string
	maxTokens     int
	systemPrompt  string
	maxIterations int
}

type Option func(*Agent)

func WithModelID(id string) Option {
	return func(a *Agent) { a.modelID = id }
}

func WithMaxTokens(n int) Option {
	return func(a *Agent) { a.maxTokens = n }
}

func WithSystemPrompt(prompt string) Option {
	return func(a *Agent) { a.systemPrompt = prompt }
}

func WithMaxIterations(n int) Option {
	return func(a *Agent) { a.maxIterations = n }
}

func WithMiddleware(m middleware.Middleware) Option {
	return func(a *Agent) { a.middlewares = append(a.middlewares, m) }
}

func New(provider model.Provider, registry *tool.Registry, opts ...Option) *Agent {
	a := &Agent{
		provider:      provider,
		registry:      registry,
		middlewares:   middleware.NewChain(),
		modelID:       "gpt-4o",
		maxTokens:     4096,
		maxIterations: 50,
	}
	for _, o := range opts {
		o(a)
	}
	return a
}

type Input struct {
	Messages []model.Message
}

type Event struct {
	Type       string           `json:"type"`
	Timestamp  time.Time        `json:"timestamp"`
	Content    string           `json:"content,omitempty"`
	ToolCall   *ToolCallEvent   `json:"tool_call,omitempty"`
	ToolResult *ToolResultEvent `json:"tool_result,omitempty"`
	Usage      *model.Usage     `json:"usage,omitempty"`
}

type ToolCallEvent struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Args string `json:"args"`
}

type ToolResultEvent struct {
	ToolCallID string `json:"tool_call_id"`
	Name       string `json:"name"`
	Output     string `json:"output"`
	Error      string `json:"error,omitempty"`
}

func (a *Agent) Run(ctx context.Context, input Input) (<-chan Event, error) {
	ch := make(chan Event, 128)

	go func() {
		defer close(ch)

		messages := make([]model.Message, len(input.Messages))
		copy(messages, input.Messages)

		toolDefs := a.buildToolDefinitions()

		for i := 0; i < a.maxIterations; i++ {
			state := &middleware.State{
				Messages:  nil,
				Metadata:  make(map[string]any),
				ToolCalls: i,
			}

			state, err := a.middlewares.BeforeModel(ctx, state)
			if err != nil {
				a.emit(ch, Event{Type: "error", Content: err.Error()})
				return
			}

			req := model.ModelRequest{
				Model:        a.modelID,
				SystemPrompt: a.systemPrompt,
				Messages:     messages,
				Tools:        toolDefs,
				MaxTokens:    a.maxTokens,
			}

			a.emit(ch, Event{Type: "model_call_start"})

			resp, err := a.provider.Complete(ctx, req)
			if err != nil {
				a.emit(ch, Event{Type: "error", Content: fmt.Sprintf("model error: %s", err)})
				return
			}

			a.emit(ch, Event{
				Type:  "model_call_end",
				Usage: &resp.Usage,
			})

			_, err = a.middlewares.AfterModel(ctx, state, &middleware.ModelResult{
				Content:    resp.Message.Content,
				ToolCalls:  convertToolCalls(resp.Message.ToolCalls),
				StopReason: resp.StopReason,
			})
			if err != nil {
				a.emit(ch, Event{Type: "error", Content: err.Error()})
				return
			}

			if resp.Message.Content != "" {
				a.emit(ch, Event{Type: "text", Content: resp.Message.Content})
			}

			if len(resp.Message.ToolCalls) == 0 {
				a.emit(ch, Event{Type: "completed", Content: resp.Message.Content})
				return
			}

			messages = append(messages, resp.Message)

			for _, tc := range resp.Message.ToolCalls {
				a.emit(ch, Event{
					Type: "tool_call",
					ToolCall: &ToolCallEvent{
						ID:   tc.ID,
						Name: tc.Name,
						Args: tc.Arguments,
					},
				})

				result := a.executeTool(ctx, tc)

				a.emit(ch, Event{
					Type: "tool_result",
					ToolResult: &ToolResultEvent{
						ToolCallID: tc.ID,
						Name:       tc.Name,
						Output:     string(result),
					},
				})

				messages = append(messages, model.Message{
					Role:       model.RoleTool,
					Content:    string(result),
					ToolCallID: tc.ID,
				})
			}
		}

		a.emit(ch, Event{Type: "error", Content: "max iterations reached"})
	}()

	return ch, nil
}

func (a *Agent) RunStreaming(ctx context.Context, input Input) (<-chan Event, error) {
	ch := make(chan Event, 128)

	go func() {
		defer close(ch)

		messages := make([]model.Message, len(input.Messages))
		copy(messages, input.Messages)

		toolDefs := a.buildToolDefinitions()

		for i := 0; i < a.maxIterations; i++ {
			state := &middleware.State{
				Metadata:  make(map[string]any),
				ToolCalls: i,
			}

			state, err := a.middlewares.BeforeModel(ctx, state)
			if err != nil {
				a.emit(ch, Event{Type: "error", Content: err.Error()})
				return
			}

			req := model.ModelRequest{
				Model:        a.modelID,
				SystemPrompt: a.systemPrompt,
				Messages:     messages,
				Tools:        toolDefs,
				MaxTokens:    a.maxTokens,
			}

			stream, err := a.provider.Stream(ctx, req)
			if err != nil {
				a.emit(ch, Event{Type: "error", Content: fmt.Sprintf("stream error: %s", err)})
				return
			}

			var contentBuf string
			var toolCalls []model.ToolCall
			currentToolIdx := -1

			for chunk := range stream {
				switch chunk.Type {
				case "content":
					contentBuf += chunk.Content
					a.emit(ch, Event{Type: "text_delta", Content: chunk.Content})
				case "tool_call_start":
					toolCalls = append(toolCalls, model.ToolCall{
						ID:   chunk.ToolCallID,
						Name: chunk.ToolName,
					})
					currentToolIdx = len(toolCalls) - 1
					a.emit(ch, Event{
						Type: "tool_call",
						ToolCall: &ToolCallEvent{
							ID:   chunk.ToolCallID,
							Name: chunk.ToolName,
						},
					})
				case "tool_call_args":
					if currentToolIdx >= 0 {
						toolCalls[currentToolIdx].Arguments += chunk.ToolArgs
					}
				case "done":
				case "error":
					a.emit(ch, Event{Type: "error", Content: chunk.Content})
					return
				}
			}

			assistantMsg := model.Message{
				Role:      model.RoleAssistant,
				Content:   contentBuf,
				ToolCalls: toolCalls,
			}

			if len(toolCalls) == 0 {
				a.emit(ch, Event{Type: "completed", Content: contentBuf})
				return
			}

			messages = append(messages, assistantMsg)

			for _, tc := range toolCalls {
				a.emit(ch, Event{
					Type: "tool_executing",
					ToolCall: &ToolCallEvent{
						ID:   tc.ID,
						Name: tc.Name,
						Args: tc.Arguments,
					},
				})

				result := a.executeTool(ctx, tc)

				a.emit(ch, Event{
					Type: "tool_result",
					ToolResult: &ToolResultEvent{
						ToolCallID: tc.ID,
						Name:       tc.Name,
						Output:     truncate(string(result), 500),
					},
				})

				messages = append(messages, model.Message{
					Role:       model.RoleTool,
					Content:    string(result),
					ToolCallID: tc.ID,
				})
			}

		}

		a.emit(ch, Event{Type: "error", Content: "max iterations reached"})
	}()

	return ch, nil
}

func (a *Agent) executeTool(ctx context.Context, tc model.ToolCall) json.RawMessage {
	t, ok := a.registry.Get(tc.Name)
	if !ok {
		log.Printf("unknown tool: %s", tc.Name)
		errJSON, _ := json.Marshal(map[string]string{"error": fmt.Sprintf("unknown tool: %s", tc.Name)})
		return errJSON
	}

	call := &middleware.ToolCall{
		ID:   tc.ID,
		Name: tc.Name,
		Args: json.RawMessage(tc.Arguments),
	}

	call, err := a.middlewares.BeforeTool(ctx, call)
	if err != nil {
		return middleware.WrapToolError(tc.Name, err)
	}

	result, err := t.Execute(ctx, call.Args)
	if err != nil {
		log.Printf("tool %s error: %s", tc.Name, err)
		return middleware.WrapToolError(tc.Name, err)
	}

	result, err = a.middlewares.AfterTool(ctx, call, result)
	if err != nil {
		return middleware.WrapToolError(tc.Name, err)
	}

	return result
}

func (a *Agent) buildToolDefinitions() []model.ToolFuncDef {
	defs := a.registry.Definitions()
	result := make([]model.ToolFuncDef, len(defs))
	for i, d := range defs {
		result[i] = model.ToolFuncDef{
			Name:        d.Name,
			Description: d.Description,
			Parameters:  d.Parameters,
		}
	}
	return result
}

func (a *Agent) emit(ch chan Event, e Event) {
	e.Timestamp = time.Now()
	ch <- e
}

func convertToolCalls(tcs []model.ToolCall) []middleware.ToolCall {
	result := make([]middleware.ToolCall, len(tcs))
	for i, tc := range tcs {
		result[i] = middleware.ToolCall{
			ID:   tc.ID,
			Name: tc.Name,
			Args: json.RawMessage(tc.Arguments),
		}
	}
	return result
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

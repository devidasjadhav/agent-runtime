package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/middleware"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/model"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/sandbox"
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
	resultStore   ResultStore
	offloadLimit  int
	messageLimit  int
}

type ResultStore interface {
	WriteFile(ctx context.Context, path string, content []byte) (sandbox.WriteResult, error)
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

func WithResultOffload(store ResultStore, limit int) Option {
	return func(a *Agent) {
		a.resultStore = store
		a.offloadLimit = limit
	}
}

func WithHumanMessageOffloadLimit(limit int) Option {
	return func(a *Agent) { a.messageLimit = limit }
}

func New(provider model.Provider, registry *tool.Registry, opts ...Option) *Agent {
	a := &Agent{
		provider:      provider,
		registry:      registry,
		middlewares:   middleware.NewChain(),
		modelID:       "gpt-4o",
		maxTokens:     4096,
		maxIterations: 50,
		offloadLimit:  80_000,
		messageLimit:  200_000,
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

type streamToolCall struct {
	ID        string
	Name      string
	Arguments string
	announced bool
}

func (a *Agent) Run(ctx context.Context, input Input) (<-chan Event, error) {
	ch := make(chan Event, 128)

	go func() {
		defer close(ch)

		messages := make([]model.Message, len(input.Messages))
		copy(messages, input.Messages)
		messages = a.maybeOffloadHumanMessages(ctx, messages)

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
						Output:     result.Content,
					},
				})

				messages = append(messages, model.Message{
					Role:       model.RoleTool,
					Content:    result.Content,
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
		messages = a.maybeOffloadHumanMessages(ctx, messages)

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
			streamedTools := make(map[int]*streamToolCall)
			var toolOrder []int

			for chunk := range stream {
				switch chunk.Type {
				case "content":
					contentBuf += chunk.Content
					a.emit(ch, Event{Type: "text_delta", Content: chunk.Content})
				case "tool_call_start":
					call := ensureStreamToolCall(streamedTools, &toolOrder, chunk.ToolIndex)
					if chunk.ToolCallID != "" {
						call.ID = chunk.ToolCallID
					}
					if chunk.ToolName != "" {
						call.Name = chunk.ToolName
					}
					if !call.announced && call.Name != "" {
						call.announced = true
						a.emit(ch, Event{
							Type: "tool_call",
							ToolCall: &ToolCallEvent{
								ID:   call.ID,
								Name: call.Name,
							},
						})
					}
				case "tool_call_args":
					call := ensureStreamToolCall(streamedTools, &toolOrder, chunk.ToolIndex)
					if chunk.ToolCallID != "" {
						call.ID = chunk.ToolCallID
					}
					call.Arguments += chunk.ToolArgs
				case "done":
				case "error":
					a.emit(ch, Event{Type: "error", Content: chunk.Content})
					return
				}
			}

			toolCalls := orderedStreamToolCalls(streamedTools, toolOrder)

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
						Output:     truncate(result.Content, 500),
					},
				})

				messages = append(messages, model.Message{
					Role:       model.RoleTool,
					Content:    result.Content,
					ToolCallID: tc.ID,
				})
			}

		}

		a.emit(ch, Event{Type: "error", Content: "max iterations reached"})
	}()

	return ch, nil
}

func (a *Agent) executeTool(ctx context.Context, tc model.ToolCall) tool.Result {
	t, ok := a.registry.Get(tc.Name)
	if !ok {
		log.Printf("unknown tool: %s", tc.Name)
		return tool.Result{Content: fmt.Sprintf("Error: unknown tool: %s", tc.Name), Error: true}
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

	return a.maybeOffloadToolResult(ctx, tc, result)
}

func (a *Agent) maybeOffloadToolResult(ctx context.Context, tc model.ToolCall, result tool.Result) tool.Result {
	if result.Error || a.resultStore == nil || a.offloadLimit <= 0 || len(result.Content) <= a.offloadLimit || !shouldOffloadTool(tc.Name) {
		return result
	}

	toolCallID := tc.ID
	if toolCallID == "" {
		toolCallID = tc.Name
	}
	path := "/large_tool_results/" + sanitizeToolCallID(toolCallID)
	writeResult, err := a.resultStore.WriteFile(ctx, path, []byte(result.Content))
	if err != nil {
		return tool.Result{Content: result.Content + "\n\n[Large result offload failed: " + err.Error() + "]"}
	}
	if writeResult.Error != "" {
		return tool.Result{Content: result.Content + "\n\n[Large result offload failed: " + writeResult.Error + "]"}
	}

	return tool.Result{Content: largeResultPreview(toolCallID, path, result.Content)}
}

func (a *Agent) maybeOffloadHumanMessages(ctx context.Context, messages []model.Message) []model.Message {
	if a.resultStore == nil || a.messageLimit <= 0 {
		return messages
	}
	for i := range messages {
		if messages[i].Role != model.RoleUser || len(messages[i].Content) <= a.messageLimit {
			continue
		}
		path := fmt.Sprintf("/conversation_history/message_%d_%d.md", i, time.Now().UnixNano())
		writeResult, err := a.resultStore.WriteFile(ctx, path, []byte(messages[i].Content))
		if err != nil || writeResult.Error != "" {
			continue
		}
		messages[i].Content = largeHumanMessagePreview(path, messages[i].Content)
	}
	return messages
}

func largeHumanMessagePreview(path, content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	var b strings.Builder
	b.WriteString("The user message was too large and was saved in the filesystem at this path: ")
	b.WriteString(path)
	b.WriteString("\n\n")
	b.WriteString("You can read it with read_file, but only read part of it at a time.\n\n")
	b.WriteString("Preview:\n")
	writePreviewLines(&b, lines)
	return b.String()
}

func shouldOffloadTool(name string) bool {
	switch name {
	case "ls", "glob", "grep", "read_file", "edit_file", "write_file":
		return false
	default:
		return true
	}
}

func sanitizeToolCallID(id string) string {
	var b strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "tool_result"
	}
	return b.String()
}

func largeResultPreview(toolCallID, path, content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Tool result too large, the result of this tool call %s was saved in the filesystem at this path: %s\n\n", toolCallID, path)
	b.WriteString("You can read the result from the filesystem by using the read_file tool, but make sure to only read part of the result at a time.\n\n")
	b.WriteString("Preview:\n")

	writePreviewLines(&b, lines)
	return b.String()
}

func writePreviewLines(b *strings.Builder, lines []string) {
	if len(lines) <= 10 {
		for i, line := range lines {
			fmt.Fprintf(b, "%6d\t%s\n", i+1, line)
		}
		return
	}
	for i := 0; i < 5; i++ {
		fmt.Fprintf(b, "%6d\t%s\n", i+1, lines[i])
	}
	b.WriteString("...\n")
	for i := len(lines) - 5; i < len(lines); i++ {
		fmt.Fprintf(b, "%6d\t%s\n", i+1, lines[i])
	}
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

func ensureStreamToolCall(calls map[int]*streamToolCall, order *[]int, index int) *streamToolCall {
	if call, ok := calls[index]; ok {
		return call
	}
	call := &streamToolCall{}
	calls[index] = call
	*order = append(*order, index)
	return call
}

func orderedStreamToolCalls(calls map[int]*streamToolCall, order []int) []model.ToolCall {
	result := make([]model.ToolCall, 0, len(order))
	for _, index := range order {
		call := calls[index]
		if call == nil || call.Name == "" {
			continue
		}
		result = append(result, model.ToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: call.Arguments,
		})
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

package builtin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/agent"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/middleware"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/model"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
)

type TaskTool struct {
	provider       model.Provider
	registry       *tool.Registry
	middlewares    middleware.Chain
	modelID        string
	maxTokens      int
	maxIterations  int
	systemPrompt   string
	resultStore    agent.ResultStore
	offloadLimit   int
}

type TaskToolOption func(*TaskTool)

func WithTaskMaxIterations(n int) TaskToolOption {
	return func(t *TaskTool) { t.maxIterations = n }
}

func WithTaskSystemPrompt(prompt string) TaskToolOption {
	return func(t *TaskTool) { t.systemPrompt = prompt }
}

func WithTaskMiddlewares(m middleware.Chain) TaskToolOption {
	return func(t *TaskTool) { t.middlewares = m }
}

func WithTaskResultOffload(store agent.ResultStore, limit int) TaskToolOption {
	return func(t *TaskTool) {
		t.resultStore = store
		t.offloadLimit = limit
	}
}

func NewTaskTool(
	provider model.Provider,
	registry *tool.Registry,
	modelID string,
	maxTokens int,
	opts ...TaskToolOption,
) *TaskTool {
	t := &TaskTool{
		provider:      provider,
		registry:      registry,
		modelID:       modelID,
		maxTokens:     maxTokens,
		maxIterations: 20,
		offloadLimit:  80_000,
	}
	for _, o := range opts {
		o(t)
	}
	return t
}

func (t *TaskTool) Name() string { return "task" }

func (t *TaskTool) Description() string {
	return "Launch a new agent that has access to all tools to handle a complex, multi-step subtask. The agent will work autonomously. Provide a clear description of what the subagent should accomplish."
}

func (t *TaskTool) Parameters() tool.ToolSchema {
	return tool.ObjectSchema(
		[]string{"description"},
		map[string]tool.ToolPropertySchema{
			"description": {
				Type:        "string",
				Description: "The task description for the subagent. Be specific about what needs to be done.",
			},
			"prompt": {
				Type:        "string",
				Description: "Optional additional context or instructions to prepend to the subagent's system prompt.",
			},
		},
	)
}

func (t *TaskTool) Execute(ctx context.Context, args json.RawMessage) (tool.Result, error) {
	var input struct {
		Description string `json:"description"`
		Prompt      string `json:"prompt"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tool.Result{Content: fmt.Sprintf("Error: invalid arguments: %v", err), Error: true}, nil
	}

	if input.Description == "" {
		return tool.Result{Content: "Error: description is required", Error: true}, nil
	}

	childOpts := []agent.Option{
		agent.WithModelID(t.modelID),
		agent.WithMaxTokens(t.maxTokens),
		agent.WithMaxIterations(t.maxIterations),
	}

	sysPrompt := t.systemPrompt
	if input.Prompt != "" {
		sysPrompt = input.Prompt + "\n\n" + sysPrompt
	}
	if sysPrompt != "" {
		childOpts = append(childOpts, agent.WithSystemPrompt(sysPrompt))
	}

	if t.resultStore != nil {
		childOpts = append(childOpts, agent.WithResultOffload(t.resultStore, t.offloadLimit))
	}

	for _, m := range t.middlewares {
		childOpts = append(childOpts, agent.WithMiddleware(m))
	}

	childAgent := agent.New(t.provider, t.registry, childOpts...)

	childInput := agent.Input{
		Messages: []model.Message{
			{Role: "user", Content: input.Description},
		},
	}

	events, err := childAgent.Run(ctx, childInput)
	if err != nil {
		return tool.Result{Content: fmt.Sprintf("Error: failed to start subagent: %v", err), Error: true}, nil
	}

	var finalContent string
	var lastUsage *model.Usage
	for evt := range events {
		switch evt.Type {
		case "text", "completed":
			if evt.Content != "" {
				finalContent = evt.Content
			}
		case "usage":
			if evt.Usage != nil {
				lastUsage = evt.Usage
			}
		case "error":
			return tool.Result{Content: fmt.Sprintf("Subagent error: %s", evt.Content), Error: true}, nil
		}
	}

	result := finalContent
	if result == "" {
		result = "Subagent completed without output."
	}

	if lastUsage != nil {
		result += fmt.Sprintf("\n\n[Subagent usage: %d input, %d output tokens]", lastUsage.InputTokens, lastUsage.OutputTokens)
	}

	return tool.Result{Content: result}, nil
}

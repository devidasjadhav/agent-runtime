package builtin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/sandbox"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
)

type ExecuteTool struct {
	sbx sandbox.Sandbox
}

func NewExecuteTool(sbx sandbox.Sandbox) *ExecuteTool {
	return &ExecuteTool{sbx: sbx}
}

func (t *ExecuteTool) Name() string { return "execute" }

func (t *ExecuteTool) Description() string {
	return "Run a shell command in the sandbox."
}

func (t *ExecuteTool) Parameters() tool.ToolSchema {
	return tool.ToolSchema{
		Type: "object",
		Properties: map[string]tool.ToolPropertySchema{
			"command": {Type: "string", Description: "Shell command to execute."},
			"timeout": {Type: "integer", Description: "Optional timeout in seconds."},
		},
		Required: []string{"command"},
	}
}

type executeArgs struct {
	Command string `json:"command"`
	Timeout *int   `json:"timeout,omitempty"`
}

func (t *ExecuteTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a executeArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	result, err := t.sbx.Exec(ctx, a.Command, a.Timeout)
	if err != nil {
		return json.Marshal(map[string]any{"error": err.Error(), "exit_code": 1})
	}
	return json.Marshal(map[string]any{
		"output":    result.Output,
		"exit_code": result.ExitCode,
	})
}

package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
	return tool.ObjectSchema([]string{"command"}, map[string]tool.ToolPropertySchema{
		"command": tool.StringProperty("Shell command to execute."),
		"timeout": tool.IntegerProperty("Optional timeout in seconds.", nil),
	})
}

type executeArgs struct {
	Command string `json:"command"`
	Timeout *int   `json:"timeout,omitempty"`
}

func (t *ExecuteTool) Execute(ctx context.Context, args json.RawMessage) (tool.Result, error) {
	var a executeArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return tool.Result{}, fmt.Errorf("parse args: %w", err)
	}
	if a.Timeout != nil && *a.Timeout < 0 {
		return tool.Result{Content: "Error: timeout must be non-negative", Error: true}, nil
	}
	result, err := t.sbx.Exec(ctx, a.Command, a.Timeout)
	if err != nil {
		return tool.Result{Content: "Error: " + err.Error(), Error: true}, nil
	}

	status := fmt.Sprintf("[Command succeeded with exit code %d]", result.ExitCode)
	if result.ExitCode != 0 {
		status = fmt.Sprintf("[Command failed with exit code %d]", result.ExitCode)
	}

	content := strings.TrimRight(truncateExecuteOutput(result.Output), "\n")
	if content != "" {
		content += "\n"
	}
	content += status

	return tool.Result{Content: content}, nil
}

package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/sandbox"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
)

type ReadFileTool struct {
	sbx sandbox.Sandbox
}

func NewReadFileTool(sbx sandbox.Sandbox) *ReadFileTool {
	return &ReadFileTool{sbx: sbx}
}

func (t *ReadFileTool) Name() string { return "read_file" }

func (t *ReadFileTool) Description() string {
	return "Read the contents of a file."
}

func (t *ReadFileTool) Parameters() tool.ToolSchema {
	return tool.ObjectSchema([]string{"file_path"}, map[string]tool.ToolPropertySchema{
		"file_path": tool.StringProperty("Absolute path to the file."),
		"offset":    tool.IntegerProperty("Line number to start from (0-indexed).", 0),
		"limit":     tool.IntegerProperty("Maximum number of lines to read. Use for pagination of large files.", 100),
	})
}

type readFileArgs struct {
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

func (t *ReadFileTool) Execute(ctx context.Context, args json.RawMessage) (tool.Result, error) {
	var a readFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return tool.Result{}, fmt.Errorf("parse args: %w", err)
	}
	limit := a.Limit
	if limit <= 0 {
		limit = 100
	}
	result, err := t.sbx.ReadFile(ctx, a.FilePath, a.Offset, limit)
	if err != nil {
		return tool.Result{Content: "Error: " + err.Error(), Error: true}, nil
	}
	if result.Error != "" {
		return tool.Result{Content: "Error: " + result.Error, Error: true}, nil
	}
	if strings.TrimSpace(result.Content) == "" {
		return tool.Result{Content: "System reminder: File exists but has empty contents"}, nil
	}

	lines := strings.Split(result.Content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%6d\t%s", a.Offset+i+1, line)
	}
	return tool.Result{Content: truncateReadOutput(b.String())}, nil
}

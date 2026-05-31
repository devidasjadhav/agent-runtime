package builtin

import (
	"context"
	"encoding/json"
	"fmt"

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
	return tool.ToolSchema{
		Type: "object",
		Properties: map[string]tool.ToolPropertySchema{
			"file_path": {Type: "string", Description: "Absolute path to the file."},
			"offset":    {Type: "integer", Description: "Line number to start from (0-indexed).", Default: 0},
			"limit":     {Type: "integer", Description: "Max lines to read.", Default: 2000},
		},
		Required: []string{"file_path"},
	}
}

type readFileArgs struct {
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

func (t *ReadFileTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a readFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	limit := a.Limit
	if limit <= 0 {
		limit = 2000
	}
	result, err := t.sbx.ReadFile(ctx, a.FilePath, a.Offset, limit)
	if err != nil {
		return json.Marshal(map[string]any{"error": err.Error()})
	}
	if result.Error != "" {
		return json.Marshal(map[string]any{"error": result.Error})
	}
	return json.Marshal(map[string]any{"content": result.Content})
}

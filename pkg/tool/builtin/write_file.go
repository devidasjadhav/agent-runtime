package builtin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/sandbox"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
)

type WriteFileTool struct {
	sbx sandbox.Sandbox
}

func NewWriteFileTool(sbx sandbox.Sandbox) *WriteFileTool {
	return &WriteFileTool{sbx: sbx}
}

func (t *WriteFileTool) Name() string { return "write_file" }

func (t *WriteFileTool) Description() string {
	return "Write content to a file. Creates parent directories as needed."
}

func (t *WriteFileTool) Parameters() tool.ToolSchema {
	return tool.ToolSchema{
		Type: "object",
		Properties: map[string]tool.ToolPropertySchema{
			"file_path": {Type: "string", Description: "Absolute path where the file should be created."},
			"content":   {Type: "string", Description: "The text content to write."},
		},
		Required: []string{"file_path", "content"},
	}
}

type writeFileArgs struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

func (t *WriteFileTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a writeFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	result, err := t.sbx.WriteFile(ctx, a.FilePath, []byte(a.Content))
	if err != nil {
		return json.Marshal(map[string]any{"error": err.Error()})
	}
	if result.Error != "" {
		return json.Marshal(map[string]any{"error": result.Error})
	}
	return json.Marshal(map[string]any{"success": true, "path": result.Path})
}

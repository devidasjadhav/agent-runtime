package builtin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/sandbox"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
)

type EditFileTool struct {
	sbx sandbox.Sandbox
}

func NewEditFileTool(sbx sandbox.Sandbox) *EditFileTool {
	return &EditFileTool{sbx: sbx}
}

func (t *EditFileTool) Name() string { return "edit_file" }

func (t *EditFileTool) Description() string {
	return "Make a targeted edit to a file by replacing an exact string."
}

func (t *EditFileTool) Parameters() tool.ToolSchema {
	return tool.ToolSchema{
		Type: "object",
		Properties: map[string]tool.ToolPropertySchema{
			"file_path":   {Type: "string", Description: "Absolute path to the file to edit."},
			"old_string":  {Type: "string", Description: "The exact text to find."},
			"new_string":  {Type: "string", Description: "The text to replace it with."},
			"replace_all": {Type: "boolean", Description: "Replace all occurrences.", Default: false},
		},
		Required: []string{"file_path", "old_string", "new_string"},
	}
}

type editFileArgs struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

func (t *EditFileTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a editFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	result, err := t.sbx.EditFile(ctx, a.FilePath, a.OldString, a.NewString, a.ReplaceAll)
	if err != nil {
		return json.Marshal(map[string]any{"error": err.Error()})
	}
	if result.Error != "" {
		return json.Marshal(map[string]any{"error": result.Error})
	}
	return json.Marshal(map[string]any{"success": true, "path": result.Path, "occurrences": result.Occurrences})
}

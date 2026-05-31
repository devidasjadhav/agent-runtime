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
	return tool.ObjectSchema([]string{"file_path", "old_string", "new_string"}, map[string]tool.ToolPropertySchema{
		"file_path":   tool.StringProperty("Absolute path to the file to edit."),
		"old_string":  tool.StringProperty("The exact text to find."),
		"new_string":  tool.StringProperty("The text to replace it with."),
		"replace_all": tool.BooleanProperty("Replace all occurrences.", false),
	})
}

type editFileArgs struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

func (t *EditFileTool) Execute(ctx context.Context, args json.RawMessage) (tool.Result, error) {
	var a editFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return tool.Result{}, fmt.Errorf("parse args: %w", err)
	}
	result, err := t.sbx.EditFile(ctx, a.FilePath, a.OldString, a.NewString, a.ReplaceAll)
	if err != nil {
		return tool.Result{Content: "Error: " + err.Error(), Error: true}, nil
	}
	if result.Error != "" {
		return tool.Result{Content: result.Error, Error: true}, nil
	}
	return tool.Result{Content: fmt.Sprintf("Successfully replaced %d instance(s) of the string in '%s'", result.Occurrences, result.Path)}, nil
}

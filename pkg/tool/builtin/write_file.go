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
	return tool.ObjectSchema([]string{"file_path", "content"}, map[string]tool.ToolPropertySchema{
		"file_path": tool.StringProperty("Absolute path where the file should be created."),
		"content":   tool.StringProperty("The text content to write."),
	})
}

type writeFileArgs struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

func (t *WriteFileTool) Execute(ctx context.Context, args json.RawMessage) (tool.Result, error) {
	var a writeFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return tool.Result{}, fmt.Errorf("parse args: %w", err)
	}
	result, err := t.sbx.WriteFile(ctx, a.FilePath, []byte(a.Content))
	if err != nil {
		return tool.Result{Content: "Error: " + err.Error(), Error: true}, nil
	}
	if result.Error != "" {
		return tool.Result{Content: result.Error, Error: true}, nil
	}
	return tool.Result{Content: "Updated file " + result.Path}, nil
}

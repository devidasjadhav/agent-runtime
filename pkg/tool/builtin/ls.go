package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/sandbox"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
)

type LsTool struct {
	sbx sandbox.Sandbox
}

func NewLsTool(sbx sandbox.Sandbox) *LsTool {
	return &LsTool{sbx: sbx}
}

func (t *LsTool) Name() string { return "ls" }

func (t *LsTool) Description() string {
	return "List direct children of a directory."
}

func (t *LsTool) Parameters() tool.ToolSchema {
	return tool.ObjectSchema([]string{"path"}, map[string]tool.ToolPropertySchema{
		"path": tool.StringProperty("Absolute path to the directory to list."),
	})
}

type lsArgs struct {
	Path string `json:"path"`
}

func (t *LsTool) Execute(ctx context.Context, args json.RawMessage) (tool.Result, error) {
	var a lsArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return tool.Result{}, fmt.Errorf("parse args: %w", err)
	}
	result, err := t.sbx.Ls(ctx, a.Path)
	if err != nil {
		return tool.Result{Content: "Error: " + err.Error(), Error: true}, nil
	}
	if result.Error != "" {
		return tool.Result{Content: "Error: " + result.Error, Error: true}, nil
	}
	paths := make([]string, 0, len(result.Entries))
	for _, entry := range result.Entries {
		p := entry.Path
		if entry.IsDir && !strings.HasSuffix(p, "/") {
			p += "/"
		}
		paths = append(paths, p)
	}
	return tool.Result{Content: truncateListOutput(formatPythonList(paths))}, nil
}

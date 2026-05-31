package builtin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/sandbox"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
)

type GlobTool struct {
	sbx sandbox.Sandbox
}

func NewGlobTool(sbx sandbox.Sandbox) *GlobTool {
	return &GlobTool{sbx: sbx}
}

func (t *GlobTool) Name() string { return "glob" }

func (t *GlobTool) Description() string {
	return "Find files matching a glob pattern."
}

func (t *GlobTool) Parameters() tool.ToolSchema {
	return tool.ObjectSchema([]string{"pattern"}, map[string]tool.ToolPropertySchema{
		"pattern": tool.StringProperty("Glob pattern to match files."),
		"path":    tool.StringPropertyDefault("Base directory to search from.", "/"),
	})
}

type globArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}

func (t *GlobTool) Execute(ctx context.Context, args json.RawMessage) (tool.Result, error) {
	var a globArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return tool.Result{}, fmt.Errorf("parse args: %w", err)
	}
	basePath := a.Path
	if basePath == "" {
		basePath = "/"
	}
	result, err := t.sbx.Glob(ctx, a.Pattern, basePath)
	if err != nil {
		return tool.Result{Content: "Error: " + err.Error(), Error: true}, nil
	}
	if result.Error != "" {
		return tool.Result{Content: "Error: " + result.Error, Error: true}, nil
	}
	paths := make([]string, 0, len(result.Matches))
	for _, match := range result.Matches {
		paths = append(paths, match.Path)
	}
	return tool.Result{Content: truncateListOutput(formatPythonList(paths))}, nil
}

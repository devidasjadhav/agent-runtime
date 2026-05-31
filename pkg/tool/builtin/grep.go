package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/sandbox"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
)

type GrepTool struct {
	sbx sandbox.Sandbox
}

func NewGrepTool(sbx sandbox.Sandbox) *GrepTool {
	return &GrepTool{sbx: sbx}
}

func (t *GrepTool) Name() string { return "grep" }

func (t *GrepTool) Description() string {
	return "Search for literal text in files."
}

func (t *GrepTool) Parameters() tool.ToolSchema {
	return tool.ObjectSchema([]string{"pattern"}, map[string]tool.ToolPropertySchema{
		"pattern":     tool.StringProperty("Text pattern to search for (literal string, not regex)."),
		"path":        tool.StringProperty("Directory to search in. Defaults to current working directory."),
		"glob":        tool.StringProperty("Glob pattern to filter which files to search."),
		"output_mode": tool.StringEnumProperty("Output format.", []string{"files_with_matches", "content", "count"}, "files_with_matches"),
	})
}

type grepArgs struct {
	Pattern    string  `json:"pattern"`
	Path       *string `json:"path,omitempty"`
	Glob       *string `json:"glob,omitempty"`
	OutputMode string  `json:"output_mode,omitempty"`
}

func (t *GrepTool) Execute(ctx context.Context, args json.RawMessage) (tool.Result, error) {
	var a grepArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return tool.Result{}, fmt.Errorf("parse args: %w", err)
	}
	mode := a.OutputMode
	if mode == "" {
		mode = "files_with_matches"
	}
	if mode != "files_with_matches" && mode != "content" && mode != "count" {
		return tool.Result{Content: "Error: output_mode must be one of files_with_matches, content, count", Error: true}, nil
	}

	result, err := t.sbx.Grep(ctx, a.Pattern, a.Path, a.Glob)
	if err != nil {
		return tool.Result{Content: "Error: " + err.Error(), Error: true}, nil
	}
	if result.Error != "" {
		return tool.Result{Content: "Error: " + result.Error, Error: true}, nil
	}
	if len(result.Matches) == 0 {
		return tool.Result{Content: "No matches found"}, nil
	}

	switch mode {
	case "files_with_matches":
		seen := map[string]struct{}{}
		var paths []string
		for _, match := range result.Matches {
			if _, ok := seen[match.Path]; ok {
				continue
			}
			seen[match.Path] = struct{}{}
			paths = append(paths, match.Path)
		}
		sort.Strings(paths)
		return tool.Result{Content: truncateListOutput(strings.Join(paths, "\n"))}, nil
	case "count":
		counts := map[string]int{}
		for _, match := range result.Matches {
			counts[match.Path]++
		}
		var paths []string
		for p := range counts {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		var b strings.Builder
		for i, p := range paths {
			if i > 0 {
				b.WriteByte('\n')
			}
			fmt.Fprintf(&b, "%s: %d", p, counts[p])
		}
		return tool.Result{Content: truncateListOutput(b.String())}, nil
	default:
		return tool.Result{Content: truncateListOutput(formatGrepContent(result.Matches))}, nil
	}
}

func formatGrepContent(matches []sandbox.GrepMatch) string {
	byFile := map[string][]sandbox.GrepMatch{}
	for _, match := range matches {
		byFile[match.Path] = append(byFile[match.Path], match)
	}
	var paths []string
	for p := range byFile {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var b strings.Builder
	for fileIdx, p := range paths {
		if fileIdx > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(p)
		b.WriteString(":\n")
		for _, match := range byFile[p] {
			fmt.Fprintf(&b, "  %d: %s\n", match.Line, match.Text)
		}
		trimTrailingNewline(&b)
	}
	return b.String()
}

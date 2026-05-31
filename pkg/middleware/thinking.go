package middleware

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
)

type ThinkingBlockSanitizer struct {
	StripThinking bool
}

func NewThinkingBlockSanitizer(strip bool) *ThinkingBlockSanitizer {
	return &ThinkingBlockSanitizer{StripThinking: strip}
}

func (t *ThinkingBlockSanitizer) BeforeModel(_ context.Context, state *State) (*State, error) {
	return state, nil
}

func (t *ThinkingBlockSanitizer) AfterModel(_ context.Context, state *State, resp *ModelResult) (*State, error) {
	if resp == nil || !t.StripThinking {
		return state, nil
	}

	resp.Content = stripThinkingBlocks(resp.Content)
	return state, nil
}

func (t *ThinkingBlockSanitizer) BeforeTool(_ context.Context, call *ToolCall) (*ToolCall, error) {
	return call, nil
}

func (t *ThinkingBlockSanitizer) AfterTool(_ context.Context, _ *ToolCall, result tool.Result) (tool.Result, error) {
	return result, nil
}

func stripThinkingBlocks(content string) string {
	var b strings.Builder
	inBlock := false
	depth := 0

	for i := 0; i < len(content); {
		if !inBlock {
			idx := strings.Index(content[i:], "<thinking>")
			if idx == -1 {
				b.WriteString(content[i:])
				break
			}
			b.WriteString(content[i : i+idx])
			inBlock = true
			depth = 1
			i = i + idx + len("<thinking>")
			continue
		}

		closeIdx := strings.Index(content[i:], "</thinking>")
		openIdx := strings.Index(content[i:], "<thinking>")

		if closeIdx == -1 {
			break
		}

		if openIdx != -1 && openIdx < closeIdx {
			depth++
			i = i + openIdx + len("<thinking>")
			continue
		}

		depth--
		i = i + closeIdx + len("</thinking>")
		if depth <= 0 {
			inBlock = false
		}
	}

	return strings.TrimSpace(b.String())
}

func SanitizeToolCallArgs(args json.RawMessage) json.RawMessage {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(args, &raw); err != nil {
		return args
	}

	for key, val := range raw {
		if len(val) > 0 && val[0] == '"' {
			var s string
			if err := json.Unmarshal(val, &s); err == nil {
				s = stripThinkingBlocks(s)
				if cleaned, err := json.Marshal(s); err == nil {
					raw[key] = cleaned
				}
			}
		}
	}

	cleaned, err := json.Marshal(raw)
	if err != nil {
		return args
	}
	return cleaned
}

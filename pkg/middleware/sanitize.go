package middleware

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
)

type SanitizeInputs struct{}

func (SanitizeInputs) BeforeModel(_ context.Context, state *State) (*State, error) {
	return state, nil
}

func (SanitizeInputs) AfterModel(_ context.Context, state *State, _ *ModelResult) (*State, error) {
	return state, nil
}

func (SanitizeInputs) BeforeTool(_ context.Context, call *ToolCall) (*ToolCall, error) {
	if call == nil {
		return nil, nil
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(call.Args, &raw); err != nil {
		return call, nil
	}

	modified := false
	for key, val := range raw {
		cleaned := sanitizeStringValue(val)
		if cleaned != nil {
			raw[key] = cleaned
			modified = true
		}
	}

	if modified {
		cleaned, err := json.Marshal(raw)
		if err != nil {
			return call, nil
		}
		call.Args = cleaned
	}

	return call, nil
}

func (SanitizeInputs) AfterTool(_ context.Context, call *ToolCall, result tool.Result) (tool.Result, error) {
	return result, nil
}

func sanitizeStringValue(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	if raw[0] != '"' {
		return nil
	}

	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil
	}

	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return -1
		}
		return r
	}, s)

	if cleaned == s {
		return nil
	}

	out, err := json.Marshal(cleaned)
	if err != nil {
		return nil
	}
	return out
}

package model

import (
	"context"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolFuncDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type ModelRequest struct {
	Model           string        `json:"model"`
	SystemPrompt    string        `json:"system_prompt,omitempty"`
	Messages        []Message     `json:"messages"`
	Tools           []ToolFuncDef `json:"tools,omitempty"`
	MaxTokens       int           `json:"max_tokens,omitempty"`
	Temperature     *float64      `json:"temperature,omitempty"`
	ReasoningEffort string        `json:"reasoning_effort,omitempty"`
}

type ModelResponse struct {
	Message    Message `json:"message"`
	StopReason string  `json:"stop_reason"`
	Usage      Usage   `json:"usage"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type ModelChunk struct {
	Type       string `json:"type"`
	Content    string `json:"content,omitempty"`
	ToolIndex  int    `json:"tool_index,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`
	ToolArgs   string `json:"tool_args,omitempty"`
	Done       bool   `json:"done,omitempty"`
}

type Provider interface {
	Complete(ctx context.Context, req ModelRequest) (*ModelResponse, error)
	Stream(ctx context.Context, req ModelRequest) (<-chan ModelChunk, error)
}

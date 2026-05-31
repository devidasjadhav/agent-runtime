package middleware

import (
	"context"
	"encoding/json"
	"fmt"
)

// ApprovalGate decides whether a triggered tool call may proceed.
// RequestApproval blocks until the human responds or ctx is cancelled.
type ApprovalGate interface {
	RequestApproval(ctx context.Context, toolName string, args json.RawMessage) (approved bool, feedback string, err error)
}

// TriggerFunc returns true when a tool call should be submitted for approval.
type TriggerFunc func(toolName string, args json.RawMessage) bool

// HumanInTheLoopMiddleware pauses execution before tool calls that match the
// trigger condition and waits for human approval via the ApprovalGate.
type HumanInTheLoopMiddleware struct {
	noopMiddleware
	gate    ApprovalGate
	trigger TriggerFunc
}

// NewHumanInTheLoop creates a HumanInTheLoopMiddleware.
// Every tool call for which trigger returns true is submitted to gate.
func NewHumanInTheLoop(gate ApprovalGate, trigger TriggerFunc) *HumanInTheLoopMiddleware {
	return &HumanInTheLoopMiddleware{gate: gate, trigger: trigger}
}

func (m *HumanInTheLoopMiddleware) BeforeTool(ctx context.Context, call *ToolCall) (*ToolCall, error) {
	if !m.trigger(call.Name, call.Args) {
		return call, nil
	}
	approved, feedback, err := m.gate.RequestApproval(ctx, call.Name, call.Args)
	if err != nil {
		return nil, fmt.Errorf("approval gate error: %w", err)
	}
	if !approved {
		msg := fmt.Sprintf("tool call %q was rejected by human reviewer", call.Name)
		if feedback != "" {
			msg += ": " + feedback
		}
		return nil, fmt.Errorf("%s", msg)
	}
	return call, nil
}

var _ Middleware = (*HumanInTheLoopMiddleware)(nil)

// --- Built-in trigger helpers ---

// TriggerOnTools returns a TriggerFunc that fires when the tool name is in the given set.
func TriggerOnTools(names ...string) TriggerFunc {
	set := make(map[string]struct{}, len(names))
	for _, n := range names {
		set[n] = struct{}{}
	}
	return func(toolName string, _ json.RawMessage) bool {
		_, ok := set[toolName]
		return ok
	}
}

// TriggerOnWriteOps fires on write_file, edit_file, and execute.
func TriggerOnWriteOps() TriggerFunc {
	return TriggerOnTools("write_file", "edit_file", "execute")
}

// TriggerAny fires when any of the given triggers fires (logical OR).
func TriggerAny(triggers ...TriggerFunc) TriggerFunc {
	return func(toolName string, args json.RawMessage) bool {
		for _, t := range triggers {
			if t(toolName, args) {
				return true
			}
		}
		return false
	}
}

// TriggerAll fires only when every trigger fires (logical AND).
func TriggerAll(triggers ...TriggerFunc) TriggerFunc {
	return func(toolName string, args json.RawMessage) bool {
		for _, t := range triggers {
			if !t(toolName, args) {
				return false
			}
		}
		return true
	}
}

// TriggerNever is a no-op trigger — useful for disabling HITL without removing it.
func TriggerNever() TriggerFunc {
	return func(string, json.RawMessage) bool { return false }
}

// --- ChannelApprovalGate ---

// ApprovalRequest is sent to the Requests channel when a tool call needs approval.
type ApprovalRequest struct {
	ToolName string
	Args     json.RawMessage
	Response chan<- ApprovalResponse // caller sends exactly one response
}

// ApprovalResponse is sent back by the control plane.
type ApprovalResponse struct {
	Approved bool
	Feedback string // optional explanation shown to agent on rejection
}

// ChannelApprovalGate is a concrete ApprovalGate backed by Go channels.
// The control plane listens on Requests, inspects each ApprovalRequest, and
// sends an ApprovalResponse on request.Response.
type ChannelApprovalGate struct {
	Requests chan ApprovalRequest
}

// NewChannelApprovalGate creates a ChannelApprovalGate with a buffered request channel.
func NewChannelApprovalGate(bufSize int) *ChannelApprovalGate {
	return &ChannelApprovalGate{Requests: make(chan ApprovalRequest, bufSize)}
}

func (g *ChannelApprovalGate) RequestApproval(ctx context.Context, toolName string, args json.RawMessage) (bool, string, error) {
	resp := make(chan ApprovalResponse, 1)
	req := ApprovalRequest{ToolName: toolName, Args: args, Response: resp}

	select {
	case g.Requests <- req:
	case <-ctx.Done():
		return false, "", ctx.Err()
	}

	select {
	case r := <-resp:
		return r.Approved, r.Feedback, nil
	case <-ctx.Done():
		return false, "", ctx.Err()
	}
}

// AutoApproveGate approves every request immediately — useful for tests and dev mode.
type AutoApproveGate struct{}

func (AutoApproveGate) RequestApproval(_ context.Context, _ string, _ json.RawMessage) (bool, string, error) {
	return true, "", nil
}

// AutoRejectGate rejects every request with an optional fixed message.
type AutoRejectGate struct{ Feedback string }

func (g AutoRejectGate) RequestApproval(_ context.Context, _ string, _ json.RawMessage) (bool, string, error) {
	return false, g.Feedback, nil
}

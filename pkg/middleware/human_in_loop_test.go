package middleware_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/middleware"
)

func hitlCall(name string) *middleware.ToolCall {
	return &middleware.ToolCall{Name: name, Args: json.RawMessage(`{}`)}
}

// TestHITL_ApprovedCallProceeds verifies that an approved tool call passes through.
func TestHITL_ApprovedCallProceeds(t *testing.T) {
	m := middleware.NewHumanInTheLoop(
		middleware.AutoApproveGate{},
		middleware.TriggerOnTools("write_file"),
	)
	call, err := m.BeforeTool(context.Background(), hitlCall("write_file"))
	if err != nil {
		t.Fatalf("expected approval, got error: %v", err)
	}
	if call == nil || call.Name != "write_file" {
		t.Fatal("call should be returned unchanged on approval")
	}
}

// TestHITL_RejectedCallReturnsError verifies that a rejected tool call is blocked.
func TestHITL_RejectedCallReturnsError(t *testing.T) {
	m := middleware.NewHumanInTheLoop(
		middleware.AutoRejectGate{Feedback: "too dangerous"},
		middleware.TriggerOnTools("execute"),
	)
	_, err := m.BeforeTool(context.Background(), hitlCall("execute"))
	if err == nil {
		t.Fatal("expected rejection error, got nil")
	}
	if !strings.Contains(err.Error(), "too dangerous") {
		t.Errorf("feedback not in error: %v", err)
	}
}

// TestHITL_UntriggeredToolPassesThrough verifies non-matching tools bypass the gate.
func TestHITL_UntriggeredToolPassesThrough(t *testing.T) {
	m := middleware.NewHumanInTheLoop(
		middleware.AutoRejectGate{Feedback: "should not be called"},
		middleware.TriggerOnTools("execute"),
	)
	call, err := m.BeforeTool(context.Background(), hitlCall("read_file"))
	if err != nil {
		t.Fatalf("untriggered tool should pass through: %v", err)
	}
	if call.Name != "read_file" {
		t.Fatal("call should be unchanged")
	}
}

// TestHITL_TriggerOnWriteOps covers write_file, edit_file, execute.
func TestHITL_TriggerOnWriteOps(t *testing.T) {
	trigger := middleware.TriggerOnWriteOps()
	for _, name := range []string{"write_file", "edit_file", "execute"} {
		if !trigger(name, nil) {
			t.Errorf("TriggerOnWriteOps should fire for %q", name)
		}
	}
	for _, name := range []string{"read_file", "ls", "web_search"} {
		if trigger(name, nil) {
			t.Errorf("TriggerOnWriteOps should NOT fire for %q", name)
		}
	}
}

// TestHITL_TriggerAny fires when any sub-trigger matches.
func TestHITL_TriggerAny(t *testing.T) {
	t1 := middleware.TriggerOnTools("a")
	t2 := middleware.TriggerOnTools("b")
	any := middleware.TriggerAny(t1, t2)
	if !any("a", nil) || !any("b", nil) {
		t.Error("TriggerAny should fire for 'a' or 'b'")
	}
	if any("c", nil) {
		t.Error("TriggerAny should not fire for 'c'")
	}
}

// TestHITL_TriggerAll fires only when all sub-triggers match.
func TestHITL_TriggerAll(t *testing.T) {
	t1 := middleware.TriggerOnTools("write_file", "execute")
	t2 := middleware.TriggerOnTools("execute", "ls")
	all := middleware.TriggerAll(t1, t2)
	if !all("execute", nil) {
		t.Error("TriggerAll should fire when both match ('execute')")
	}
	if all("write_file", nil) {
		t.Error("TriggerAll should not fire when only t1 matches")
	}
}

// TestHITL_TriggerNever never fires.
func TestHITL_TriggerNever(t *testing.T) {
	trigger := middleware.TriggerNever()
	for _, name := range []string{"write_file", "execute", "anything"} {
		if trigger(name, nil) {
			t.Errorf("TriggerNever should never fire (got true for %q)", name)
		}
	}
}

// TestHITL_ChannelGate_ApproveFlow exercises the channel-based gate end-to-end.
func TestHITL_ChannelGate_ApproveFlow(t *testing.T) {
	gate := middleware.NewChannelApprovalGate(1)
	m := middleware.NewHumanInTheLoop(gate, middleware.TriggerOnTools("execute"))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := m.BeforeTool(ctx, hitlCall("execute"))
		done <- err
	}()

	// Simulate control plane approving the request
	req := <-gate.Requests
	req.Response <- middleware.ApprovalResponse{Approved: true}

	if err := <-done; err != nil {
		t.Fatalf("expected approval, got: %v", err)
	}
}

// TestHITL_ChannelGate_RejectFlow exercises the reject path.
func TestHITL_ChannelGate_RejectFlow(t *testing.T) {
	gate := middleware.NewChannelApprovalGate(1)
	m := middleware.NewHumanInTheLoop(gate, middleware.TriggerOnTools("execute"))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := m.BeforeTool(ctx, hitlCall("execute"))
		done <- err
	}()

	req := <-gate.Requests
	req.Response <- middleware.ApprovalResponse{Approved: false, Feedback: "not today"}

	err := <-done
	if err == nil {
		t.Fatal("expected rejection error")
	}
	if !strings.Contains(err.Error(), "not today") {
		t.Errorf("feedback not in error: %v", err)
	}
}

// TestHITL_ContextCancellation verifies gate unblocks on context cancel.
func TestHITL_ContextCancellation(t *testing.T) {
	gate := middleware.NewChannelApprovalGate(0) // unbuffered — will block
	m := middleware.NewHumanInTheLoop(gate, middleware.TriggerOnTools("execute"))

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := m.BeforeTool(ctx, hitlCall("execute"))
	if err == nil {
		t.Fatal("expected error on context cancellation")
	}
}

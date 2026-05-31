package middleware_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/middleware"
)

func aclCall(name string, args map[string]string) *middleware.ToolCall {
	raw, _ := json.Marshal(args)
	return &middleware.ToolCall{Name: name, Args: raw}
}

func TestACL_AllowByDefault(t *testing.T) {
	m := middleware.NewFilesystemACL(nil)
	_, err := m.BeforeTool(context.Background(), aclCall("read_file", map[string]string{"file_path": "/etc/passwd"}))
	if err != nil {
		t.Fatalf("expected allow by default, got: %v", err)
	}
}

func TestACL_DenyRuleBlocks(t *testing.T) {
	m := middleware.NewFilesystemACL([]middleware.Permission{
		{Pattern: "/etc/**", Operations: middleware.OpRead, Allow: false},
	})
	_, err := m.BeforeTool(context.Background(), aclCall("read_file", map[string]string{"file_path": "/etc/passwd"}))
	if err == nil {
		t.Fatal("expected deny, got nil")
	}
}

func TestACL_AllowRulePermits(t *testing.T) {
	m := middleware.NewFilesystemACL([]middleware.Permission{
		{Pattern: "/etc/**", Operations: middleware.OpRead, Allow: false},
		{Pattern: "/home/**", Operations: middleware.OpRead, Allow: true},
	})
	_, err := m.BeforeTool(context.Background(), aclCall("read_file", map[string]string{"file_path": "/home/dev/file.txt"}))
	if err != nil {
		t.Fatalf("expected allow, got: %v", err)
	}
}

func TestACL_FirstMatchWins(t *testing.T) {
	m := middleware.NewFilesystemACL([]middleware.Permission{
		{Pattern: "/home/**", Operations: middleware.OpAll, Allow: true},  // first: allow
		{Pattern: "/home/**", Operations: middleware.OpAll, Allow: false}, // second: deny (never reached)
	})
	_, err := m.BeforeTool(context.Background(), aclCall("write_file", map[string]string{"file_path": "/home/dev/out.txt"}))
	if err != nil {
		t.Fatalf("first match (allow) should win, got: %v", err)
	}
}

func TestACL_DefaultDeny(t *testing.T) {
	m := middleware.NewFilesystemACL([]middleware.Permission{
		{Pattern: "/workspace/**", Operations: middleware.OpAll, Allow: true},
	}, middleware.WithDefaultDeny())
	_, err := m.BeforeTool(context.Background(), aclCall("read_file", map[string]string{"file_path": "/etc/secret"}))
	if err == nil {
		t.Fatal("expected default deny, got nil")
	}
}

func TestACL_DefaultDenyAllowsMatchedPath(t *testing.T) {
	m := middleware.NewFilesystemACL([]middleware.Permission{
		{Pattern: "/workspace/**", Operations: middleware.OpAll, Allow: true},
	}, middleware.WithDefaultDeny())
	_, err := m.BeforeTool(context.Background(), aclCall("read_file", map[string]string{"file_path": "/workspace/main.go"}))
	if err != nil {
		t.Fatalf("expected allow for /workspace/**, got: %v", err)
	}
}

func TestACL_WriteToolCheckedSeparately(t *testing.T) {
	m := middleware.NewFilesystemACL([]middleware.Permission{
		{Pattern: "**", Operations: middleware.OpRead, Allow: true},
		{Pattern: "**", Operations: middleware.OpWrite, Allow: false},
	})
	// read allowed
	_, err := m.BeforeTool(context.Background(), aclCall("read_file", map[string]string{"file_path": "/any/file"}))
	if err != nil {
		t.Fatalf("read should be allowed: %v", err)
	}
	// write denied
	_, err = m.BeforeTool(context.Background(), aclCall("write_file", map[string]string{"file_path": "/any/file"}))
	if err == nil {
		t.Fatal("write should be denied")
	}
}

func TestACL_ExecuteBlocked(t *testing.T) {
	m := middleware.NewFilesystemACL([]middleware.Permission{
		{Pattern: "**", Operations: middleware.OpExecute, Allow: false},
	})
	_, err := m.BeforeTool(context.Background(), aclCall("execute", map[string]string{"command": "rm -rf /"}))
	if err == nil {
		t.Fatal("expected execute to be denied")
	}
}

func TestACL_UnknownToolPassesThrough(t *testing.T) {
	m := middleware.NewFilesystemACL([]middleware.Permission{
		{Pattern: "**", Operations: middleware.OpAll, Allow: false},
	})
	_, err := m.BeforeTool(context.Background(), aclCall("web_search", map[string]string{"query": "anything"}))
	if err != nil {
		t.Fatalf("unknown tools should pass through: %v", err)
	}
}

func TestACL_GlobPatternExtension(t *testing.T) {
	m := middleware.NewFilesystemACL([]middleware.Permission{
		{Pattern: "*.env", Operations: middleware.OpRead, Allow: false},
	})
	_, err := m.BeforeTool(context.Background(), aclCall("read_file", map[string]string{"file_path": "/app/.env"}))
	if err == nil {
		t.Fatal("*.env pattern should deny .env files")
	}
}

func TestACL_EditFileIsWrite(t *testing.T) {
	m := middleware.NewFilesystemACL([]middleware.Permission{
		{Pattern: "/protected/**", Operations: middleware.OpWrite, Allow: false},
	})
	_, err := m.BeforeTool(context.Background(), aclCall("edit_file", map[string]string{"file_path": "/protected/config.go"}))
	if err == nil {
		t.Fatal("edit_file should be treated as write and denied")
	}
}

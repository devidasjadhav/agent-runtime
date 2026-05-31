package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
)

// Operation represents a filesystem action that can be allowed or denied.
type Operation int

const (
	OpRead    Operation = 1 << iota // read_file, ls, glob, grep
	OpWrite                         // write_file, edit_file
	OpExecute                       // execute
	OpAll     = OpRead | OpWrite | OpExecute
)

// Permission is a single ACL rule. Rules are evaluated in order; the first
// matching rule wins. If no rule matches, the default policy applies.
type Permission struct {
	Pattern    string    // glob pattern matched against the tool's path argument
	Operations Operation // bitmask of affected operations
	Allow      bool      // true = allow, false = deny
}

// FilesystemACLMiddleware checks file/execute tool calls against an ordered
// list of allow/deny rules before they reach the tool.
type FilesystemACLMiddleware struct {
	noopMiddleware
	rules       []Permission
	defaultDeny bool
}

type ACLOption func(*FilesystemACLMiddleware)

// WithDefaultDeny makes unmatched paths denied instead of allowed.
func WithDefaultDeny() ACLOption {
	return func(m *FilesystemACLMiddleware) { m.defaultDeny = true }
}

// NewFilesystemACL creates a FilesystemACLMiddleware with the given ordered rules.
func NewFilesystemACL(rules []Permission, opts ...ACLOption) *FilesystemACLMiddleware {
	m := &FilesystemACLMiddleware{rules: rules}
	for _, o := range opts {
		o(m)
	}
	return m
}

func (m *FilesystemACLMiddleware) BeforeTool(ctx context.Context, call *ToolCall) (*ToolCall, error) {
	op, targetPath, ok := classifyTool(call.Name, call.Args)
	if !ok {
		return call, nil // unknown tool — pass through
	}
	if err := m.check(op, targetPath); err != nil {
		return nil, err
	}
	return call, nil
}

func (m *FilesystemACLMiddleware) check(op Operation, targetPath string) error {
	for _, rule := range m.rules {
		if rule.Operations&op == 0 {
			continue
		}
		if matchACLPattern(rule.Pattern, targetPath) {
			if rule.Allow {
				return nil
			}
			return fmt.Errorf("acl: %s denied by rule %q", opName(op), rule.Pattern)
		}
	}
	if m.defaultDeny {
		return fmt.Errorf("acl: %s denied (no matching allow rule for %q)", opName(op), targetPath)
	}
	return nil
}

// classifyTool maps a tool name + args to (operation, path, ok).
// Returns ok=false for tools we don't intercept.
func classifyTool(name string, args json.RawMessage) (Operation, string, bool) {
	switch name {
	case "read_file":
		return OpRead, extractStringArg(args, "file_path"), true
	case "write_file":
		return OpWrite, extractStringArg(args, "file_path"), true
	case "edit_file":
		return OpWrite, extractStringArg(args, "file_path"), true
	case "ls":
		return OpRead, extractStringArg(args, "path"), true
	case "glob":
		p := extractStringArg(args, "path")
		if p == "" {
			p = "."
		}
		return OpRead, p, true
	case "grep":
		p := extractStringArg(args, "path")
		if p == "" {
			p = "."
		}
		return OpRead, p, true
	case "execute":
		// For execute we check against the command string itself.
		cmd := extractStringArg(args, "command")
		return OpExecute, cmd, true
	}
	return 0, "", false
}

func extractStringArg(args json.RawMessage, key string) string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(args, &m); err != nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return ""
	}
	return s
}

// matchACLPattern matches a glob pattern against a target path.
// Supports * (single segment) and ** (any number of segments).
func matchACLPattern(pattern, target string) bool {
	if pattern == "" {
		return false
	}
	if pattern == "**" || pattern == "**/*" {
		return true
	}
	// Normalise separators
	pattern = filepath(pattern)
	target = filepath(target)

	if !strings.Contains(pattern, "**") {
		ok, _ := path.Match(pattern, target)
		if ok {
			return true
		}
		// Also try base name match for patterns without slashes (e.g. "*.env")
		if !strings.Contains(pattern, "/") {
			ok, _ = path.Match(pattern, path.Base(target))
			return ok
		}
		return false
	}

	// Split on ** and require prefix + suffix to match
	parts := strings.SplitN(pattern, "**", 2)
	prefix := strings.TrimSuffix(parts[0], "/")
	suffix := strings.TrimPrefix(parts[1], "/")

	if prefix != "" {
		if !strings.HasPrefix(target, prefix+"/") && target != prefix {
			return false
		}
	}
	if suffix == "" {
		return true
	}
	// suffix must match any trailing segment
	segs := strings.Split(strings.TrimPrefix(target, prefix+"/"), "/")
	for i := range segs {
		candidate := strings.Join(segs[i:], "/")
		ok, _ := path.Match(suffix, candidate)
		if ok {
			return true
		}
	}
	return false
}

func filepath(s string) string {
	return strings.ReplaceAll(s, "\\", "/")
}

func opName(op Operation) string {
	switch op {
	case OpRead:
		return "read"
	case OpWrite:
		return "write"
	case OpExecute:
		return "execute"
	}
	return "operation"
}

var _ Middleware = (*FilesystemACLMiddleware)(nil)
var _ tool.Result = tool.Result{} // keep tool import used via WrapToolError in other files

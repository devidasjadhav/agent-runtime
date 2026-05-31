package langsmith

import (
	"context"
	"testing"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/sandbox"
)

var _ sandbox.Sandbox = (*Sandbox)(nil)
var _ sandbox.ExecBackend = (*langsmithBackend)(nil)

func TestSandboxImplementsInterface(t *testing.T) {
	b := &langsmithBackend{sandboxName: "test-id"}
	s := &Sandbox{
		base:    sandbox.NewBaseSandbox(b),
		backend: b,
	}
	if s.ID() != "test-id" {
		t.Errorf("ID() = %q, want %q", s.ID(), "test-id")
	}
}

func TestBackendImplementsExecBackend(t *testing.T) {
	b := &langsmithBackend{
		sandboxName:  "test-sandbox",
		dataplaneURL: "https://dp.example.com",
	}
	if b.ID() != "test-sandbox" {
		t.Errorf("ID() = %q, want %q", b.ID(), "test-sandbox")
	}
}

func TestSandboxCloseReturnsNil(t *testing.T) {
	s := &Sandbox{
		base:    sandbox.NewBaseSandbox(&langsmithBackend{}),
		backend: &langsmithBackend{},
	}
	if err := s.Close(context.Background()); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}

func TestBackendCloseReturnsNil(t *testing.T) {
	b := &langsmithBackend{}
	if err := b.Close(context.Background()); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}

func TestConfigNewClientOpts(t *testing.T) {
	cfg := Config{
		APIKey:   "test-key",
		TenantID: "test-tenant",
		Endpoint: "https://custom.api.example.com",
	}
	client := newClient(cfg)
	if client == nil {
		t.Error("newClient returned nil")
	}
}

func TestWrapSandboxFields(t *testing.T) {
	b := &langsmithBackend{
		sandboxName:  "my-box",
		dataplaneURL: "https://dp.example.com",
	}
	s := &Sandbox{
		base:    sandbox.NewBaseSandbox(b),
		backend: b,
	}
	if s.ID() != "my-box" {
		t.Errorf("Sandbox.ID() = %q, want %q", s.ID(), "my-box")
	}
}

func TestSandboxDelegatesToBaseForEditLsGlobGrep(t *testing.T) {
	mock := &mockExecBackend{
		results: []sandbox.ExecResult{
			{Output: `{"path": "/tmp/test.py", "occurrences": 1}`, ExitCode: 0},
			{Output: `{"entries": [{"path": "/home/file.txt", "is_dir": false, "size": 100}]}`, ExitCode: 0},
			{Output: `{"matches": [{"path": "/home/a.txt", "is_dir": false, "size": 50}]}`, ExitCode: 0},
			{Output: `{"matches": [{"path": "/home/a.txt", "line": 1, "text": "hello"}]}`, ExitCode: 0},
		},
	}
	base := sandbox.NewBaseSandbox(mock)
	s := &Sandbox{
		base:    base,
		backend: &langsmithBackend{sandboxName: "test"},
	}

	edit, err := s.EditFile(context.Background(), "/tmp/test.py", "old", "new", false)
	if err != nil {
		t.Fatalf("EditFile error: %v", err)
	}
	if edit.Error != "" {
		t.Errorf("EditFile error: %s", edit.Error)
	}

	ls, err := s.Ls(context.Background(), "/home")
	if err != nil {
		t.Fatalf("Ls error: %v", err)
	}
	if ls.Error != "" {
		t.Errorf("Ls error: %s", ls.Error)
	}

	glob, err := s.Glob(context.Background(), "*.txt", "/home")
	if err != nil {
		t.Fatalf("Glob error: %v", err)
	}
	if glob.Error != "" {
		t.Errorf("Glob error: %s", glob.Error)
	}

	grep, err := s.Grep(context.Background(), "hello", nil, nil)
	if err != nil {
		t.Fatalf("Grep error: %v", err)
	}
	if grep.Error != "" {
		t.Errorf("Grep error: %s", grep.Error)
	}
}

type mockExecBackend struct {
	id      string
	results []sandbox.ExecResult
	calls   int
}

func (m *mockExecBackend) ID() string { return m.id }

func (m *mockExecBackend) Exec(_ context.Context, _ string, _ *int) (sandbox.ExecResult, error) {
	if m.calls >= len(m.results) {
		return sandbox.ExecResult{}, nil
	}
	r := m.results[m.calls]
	m.calls++
	return r, nil
}

func (m *mockExecBackend) Close(_ context.Context) error { return nil }

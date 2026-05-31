package ssh_test

import (
	"context"
	"os"
	"strings"
	"testing"

	sshsandbox "github.com/anomalyco/open-swe/agent-runtime/pkg/sandbox/ssh"
)

// Integration tests — only run when SSH_HOST is set.
// Example:
//
//	SSH_HOST=localhost SSH_USER=dev SSH_PASSWORD=secret SSH_DIR=/tmp go test ./pkg/sandbox/ssh/...

func requireEnv(t *testing.T) {
	t.Helper()
	if os.Getenv("SSH_HOST") == "" {
		t.Skip("SSH_HOST not set; skipping SSH sandbox integration tests")
	}
}

func newTestSandbox(t *testing.T) *sshsandbox.SSHSandbox {
	t.Helper()
	sbx, err := sshsandbox.NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv: %v", err)
	}
	t.Cleanup(func() { sbx.Close(context.Background()) })
	return sbx
}

func TestSSHExec(t *testing.T) {
	requireEnv(t)
	sbx := newTestSandbox(t)

	result, err := sbx.Exec(context.Background(), "echo hello", nil)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("unexpected exit code %d", result.ExitCode)
	}
	if !strings.Contains(result.Output, "hello") {
		t.Fatalf("unexpected output: %q", result.Output)
	}
}

func TestSSHExecFailure(t *testing.T) {
	requireEnv(t)
	sbx := newTestSandbox(t)

	result, err := sbx.Exec(context.Background(), "sh -c 'exit 42'", nil)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if result.ExitCode != 42 {
		t.Fatalf("expected exit code 42, got %d", result.ExitCode)
	}
}

func TestSSHWriteReadFile(t *testing.T) {
	requireEnv(t)
	sbx := newTestSandbox(t)
	ctx := context.Background()

	wr, err := sbx.WriteFile(ctx, "ssh_test_file.txt", []byte("hello from ssh\n"))
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if wr.Error != "" {
		t.Fatalf("WriteFile error: %s", wr.Error)
	}

	rr, err := sbx.ReadFile(ctx, "ssh_test_file.txt", 0, 0)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if rr.Error != "" {
		t.Fatalf("ReadFile error: %s", rr.Error)
	}
	if !strings.Contains(rr.Content, "hello from ssh") {
		t.Fatalf("unexpected content: %q", rr.Content)
	}

	// Cleanup
	sbx.Exec(ctx, "rm -f ssh_test_file.txt", nil)
}

func TestSSHLs(t *testing.T) {
	requireEnv(t)
	sbx := newTestSandbox(t)
	ctx := context.Background()

	sbx.WriteFile(ctx, "ls_test.txt", []byte("x"))
	defer sbx.Exec(ctx, "rm -f ls_test.txt", nil)

	lr, err := sbx.Ls(ctx, ".")
	if err != nil {
		t.Fatalf("Ls: %v", err)
	}
	if lr.Error != "" {
		t.Fatalf("Ls error: %s", lr.Error)
	}
	found := false
	for _, e := range lr.Entries {
		if strings.HasSuffix(e.Path, "ls_test.txt") {
			found = true
		}
	}
	if !found {
		t.Fatalf("ls_test.txt not found in Ls output")
	}
}

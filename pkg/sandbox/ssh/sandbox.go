// Package ssh provides a sandbox backed by a remote Linux host over SSH.
// All file operations are handled by BaseSandbox (Python helpers over Exec);
// only Exec itself uses the SSH session.
package ssh

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/sandbox"
)

// Config holds connection parameters for the SSH sandbox.
type Config struct {
	Host     string // hostname or IP
	Port     int    // default 22
	User     string
	Password string // optional; used if KeyPath is empty
	KeyPath  string // optional path to PEM private key file
	Dir      string // remote working directory (e.g. /home/user/workspace)
}

type sshBackend struct {
	id     string
	client *gossh.Client
	dir    string
}

// SSHSandbox is a Sandbox backed by a remote host.
// File operations delegate to BaseSandbox (Python helpers over Exec).
type SSHSandbox struct {
	*sandbox.BaseSandbox
	backend *sshBackend
}

// New dials the remote host and returns a ready SSHSandbox.
func New(cfg Config) (*SSHSandbox, error) {
	if cfg.Port == 0 {
		cfg.Port = 22
	}
	if cfg.Dir == "" {
		cfg.Dir = "."
	}

	auth, err := buildAuth(cfg)
	if err != nil {
		return nil, fmt.Errorf("ssh auth: %w", err)
	}

	clientCfg := &gossh.ClientConfig{
		User:            cfg.User,
		Auth:            auth,
		HostKeyCallback: gossh.InsecureIgnoreHostKey(), // acceptable for dev/test use
		Timeout:         15 * time.Second,
	}

	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))
	client, err := gossh.Dial("tcp", addr, clientCfg)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", addr, err)
	}

	be := &sshBackend{
		id:     fmt.Sprintf("ssh-%s-%d", cfg.Host, time.Now().UnixMilli()),
		client: client,
		dir:    cfg.Dir,
	}
	return &SSHSandbox{
		BaseSandbox: sandbox.NewBaseSandbox(be),
		backend:     be,
	}, nil
}

// NewFromEnv constructs an SSHSandbox from environment variables:
//
//	SSH_HOST, SSH_PORT (optional), SSH_USER, SSH_PASSWORD or SSH_KEY_PATH, SSH_DIR (optional)
func NewFromEnv() (*SSHSandbox, error) {
	host := os.Getenv("SSH_HOST")
	if host == "" {
		return nil, fmt.Errorf("SSH_HOST is required")
	}
	user := os.Getenv("SSH_USER")
	if user == "" {
		return nil, fmt.Errorf("SSH_USER is required")
	}
	port := 22
	if p := os.Getenv("SSH_PORT"); p != "" {
		fmt.Sscanf(p, "%d", &port)
	}
	return New(Config{
		Host:     host,
		Port:     port,
		User:     user,
		Password: os.Getenv("SSH_PASSWORD"),
		KeyPath:  os.Getenv("SSH_KEY_PATH"),
		Dir:      os.Getenv("SSH_DIR"),
	})
}

func buildAuth(cfg Config) ([]gossh.AuthMethod, error) {
	if cfg.KeyPath != "" {
		key, err := os.ReadFile(cfg.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("read key %s: %w", cfg.KeyPath, err)
		}
		signer, err := gossh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("parse key: %w", err)
		}
		return []gossh.AuthMethod{gossh.PublicKeys(signer)}, nil
	}
	if cfg.Password != "" {
		return []gossh.AuthMethod{gossh.Password(cfg.Password)}, nil
	}
	return nil, fmt.Errorf("either Password or KeyPath must be set")
}

// --- ExecBackend implementation ---

func (b *sshBackend) ID() string { return b.id }

func (b *sshBackend) Exec(ctx context.Context, cmd string, timeout *int) (sandbox.ExecResult, error) {
	if timeout != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(*timeout)*time.Second)
		defer cancel()
	}

	session, err := b.client.NewSession()
	if err != nil {
		return sandbox.ExecResult{}, fmt.Errorf("new session: %w", err)
	}
	defer session.Close()

	// Run in working directory; cd is cheap and avoids storing per-session state.
	fullCmd := fmt.Sprintf("cd %s && %s", shellQuote(b.dir), cmd)

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	done := make(chan error, 1)
	go func() { done <- session.Run(fullCmd) }()

	var runErr error
	select {
	case <-ctx.Done():
		session.Signal(gossh.SIGKILL)
		return sandbox.ExecResult{}, ctx.Err()
	case runErr = <-done:
	}

	out := stdout.String()
	if stderr.Len() > 0 {
		out += "\n" + stderr.String()
	}
	out = strings.TrimRight(out, "\n")

	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*gossh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
		} else {
			return sandbox.ExecResult{}, fmt.Errorf("run: %w", runErr)
		}
	}

	return sandbox.ExecResult{Output: out, ExitCode: exitCode}, nil
}

func (b *sshBackend) Close(_ context.Context) error {
	return b.client.Close()
}

// shellQuote wraps a path in single quotes and escapes embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

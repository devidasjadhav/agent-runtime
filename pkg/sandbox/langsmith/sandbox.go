package langsmith

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	ls "github.com/langchain-ai/langsmith-go"
	"github.com/langchain-ai/langsmith-go/option"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/sandbox"
)

type Config struct {
	APIKey       string
	TenantID     string
	Endpoint     string
	SnapshotID   string
	SnapshotName string
}

type Sandbox struct {
	base    *sandbox.BaseSandbox
	backend *langsmithBackend
	mu      sync.Mutex
}

type langsmithBackend struct {
	client      *ls.Client
	sandboxName string
	dataplaneURL string
	mu          sync.Mutex
}

func newClient(cfg Config) *ls.Client {
	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
		option.WithTenantID(cfg.TenantID),
	}
	if cfg.Endpoint != "" {
		opts = append(opts, option.WithBaseURL(cfg.Endpoint))
	}
	return ls.NewClient(opts...)
}

func Create(ctx context.Context, cfg Config, name string, opts ...option.RequestOption) (*Sandbox, error) {
	client := newClient(cfg)

	params := ls.SandboxBoxNewParams{
		Name: ls.F(name),
	}
	if cfg.SnapshotID != "" {
		params.SnapshotID = ls.F(cfg.SnapshotID)
	}
	if cfg.SnapshotName != "" {
		params.SnapshotName = ls.F(cfg.SnapshotName)
	}

	box, err := client.Sandboxes.Boxes.NewSandbox(ctx, params, opts...)
	if err != nil {
		return nil, fmt.Errorf("langsmith sandbox create: %w", err)
	}

	if box.Status != "ready" {
		waitParams := ls.SandboxWaitParams{
			Timeout:      120 * time.Second,
			PollInterval: 2 * time.Second,
		}
		box, err = client.Sandboxes.Boxes.WaitSandbox(ctx, box.Name, waitParams, opts...)
		if err != nil {
			return nil, fmt.Errorf("langsmith sandbox wait: %w", err)
		}
	}

	return wrapSandbox(client, box), nil
}

func Connect(ctx context.Context, cfg Config, name string, opts ...option.RequestOption) (*Sandbox, error) {
	client := newClient(cfg)

	box, err := client.Sandboxes.Boxes.GetSandbox(ctx, name, opts...)
	if err != nil {
		return nil, fmt.Errorf("langsmith sandbox connect: %w", err)
	}

	if box.Status == "stopped" {
		waitParams := ls.SandboxWaitParams{
			Timeout:      120 * time.Second,
			PollInterval: 2 * time.Second,
		}
		if err := box.Start(ctx, waitParams, opts...); err != nil {
			return nil, fmt.Errorf("langsmith sandbox start: %w", err)
		}
	}

	return wrapSandbox(client, box), nil
}

func wrapSandbox(client *ls.Client, box *ls.Sandbox) *Sandbox {
	backend := &langsmithBackend{
		client:      client,
		sandboxName: box.Name,
		dataplaneURL: box.DataplaneURL,
	}
	base := sandbox.NewBaseSandbox(backend)
	return &Sandbox{
		base:    base,
		backend: backend,
	}
}

func (s *Sandbox) ID() string { return s.backend.ID() }

func (s *Sandbox) Exec(ctx context.Context, cmd string, timeout *int) (sandbox.ExecResult, error) {
	return s.base.Exec(ctx, cmd, timeout)
}

func (s *Sandbox) ReadFile(ctx context.Context, path string, offset, limit int) (sandbox.ReadResult, error) {
	data, err := s.backend.readFile(ctx, path)
	if err != nil {
		return sandbox.ReadResult{Error: err.Error()}, nil
	}
	content := string(data)
	lines := strings.Split(content, "\n")
	start := offset
	if start > len(lines) {
		return sandbox.ReadResult{Error: fmt.Sprintf("Line offset %d exceeds file length (%d lines)", offset, len(lines))}, nil
	}
	end := len(lines)
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	return sandbox.ReadResult{Content: strings.Join(lines[start:end], "\n")}, nil
}

func (s *Sandbox) WriteFile(ctx context.Context, path string, content []byte) (sandbox.WriteResult, error) {
	existing, err := s.backend.readFile(ctx, path)
	if err == nil && len(existing) > 0 {
		return sandbox.WriteResult{Error: fmt.Sprintf("Cannot write to %s because it already exists. Read and then make an edit, or write to a new path.", path)}, nil
	}
	if err := s.backend.writeFile(ctx, path, content); err != nil {
		return sandbox.WriteResult{Error: err.Error()}, nil
	}
	return sandbox.WriteResult{Path: path}, nil
}

func (s *Sandbox) EditFile(ctx context.Context, path, oldStr, newStr string, replaceAll bool) (sandbox.EditResult, error) {
	return s.base.EditFile(ctx, path, oldStr, newStr, replaceAll)
}

func (s *Sandbox) Ls(ctx context.Context, path string) (sandbox.LsResult, error) {
	return s.base.Ls(ctx, path)
}

func (s *Sandbox) Glob(ctx context.Context, pattern, basePath string) (sandbox.GlobResult, error) {
	return s.base.Glob(ctx, pattern, basePath)
}

func (s *Sandbox) Grep(ctx context.Context, pattern string, path *string, glob *string) (sandbox.GrepResult, error) {
	return s.base.Grep(ctx, pattern, path, glob)
}

func (s *Sandbox) UploadFiles(ctx context.Context, files []sandbox.FileUpload) ([]sandbox.FileUploadResult, error) {
	var results []sandbox.FileUploadResult
	for _, f := range files {
		if err := s.backend.writeFile(ctx, f.Path, f.Content); err != nil {
			results = append(results, sandbox.FileUploadResult{Path: f.Path, Error: err.Error()})
		} else {
			results = append(results, sandbox.FileUploadResult{Path: f.Path})
		}
	}
	return results, nil
}

func (s *Sandbox) DownloadFiles(ctx context.Context, paths []string) ([]sandbox.FileDownloadResult, error) {
	results := make([]sandbox.FileDownloadResult, 0, len(paths))
	for _, p := range paths {
		data, err := s.backend.readFile(ctx, p)
		if err != nil {
			results = append(results, sandbox.FileDownloadResult{Path: p, Error: err.Error()})
		} else {
			results = append(results, sandbox.FileDownloadResult{Path: p, Content: data})
		}
	}
	return results, nil
}

func (s *Sandbox) Close(ctx context.Context) error {
	return s.backend.Close(ctx)
}

func (s *Sandbox) Refresh(ctx context.Context) error {
	box, err := s.backend.client.Sandboxes.Boxes.GetSandbox(ctx, s.backend.sandboxName)
	if err != nil {
		return fmt.Errorf("langsmith sandbox refresh: %w", err)
	}
	s.backend.mu.Lock()
	s.backend.dataplaneURL = box.DataplaneURL
	s.backend.mu.Unlock()
	return nil
}

func (s *Sandbox) ConfigureGitHubProxy(ctx context.Context, token string) error {
	s.backend.mu.Lock()
	name := s.backend.sandboxName
	s.backend.mu.Unlock()

	_, err := s.backend.client.Sandboxes.Boxes.Update(ctx, name, ls.SandboxBoxUpdateParams{
		ProxyConfig: ls.F(ls.SandboxBoxUpdateParamsProxyConfig{
			Rules: ls.F([]ls.SandboxBoxUpdateParamsProxyConfigRule{{
				MatchHosts: ls.F([]string{"github.com", "api.github.com", "*.github.com"}),
				Name:       ls.F("github-proxy"),
				Enabled:    ls.F(true),
				Headers: ls.F([]ls.SandboxBoxUpdateParamsProxyConfigRulesHeader{{
					Name:  ls.F("Authorization"),
					Type:  ls.F(ls.SandboxBoxUpdateParamsProxyConfigRulesHeadersTypePlaintext),
					IsSet: ls.F(true),
					Value: ls.F(fmt.Sprintf("Bearer %s", token)),
				}}),
			}}),
		}),
	})
	return err
}

func (b *langsmithBackend) ID() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sandboxName
}

func (b *langsmithBackend) Exec(ctx context.Context, cmd string, timeout *int) (sandbox.ExecResult, error) {
	params := ls.SandboxBoxRunParams{
		Command: ls.F(cmd),
	}
	if timeout != nil {
		params.Timeout = ls.F(int64(*timeout))
	}

	b.mu.Lock()
	dataplaneURL := b.dataplaneURL
	b.mu.Unlock()

	result, err := b.client.Sandboxes.Boxes.RunWithDataplaneURL(ctx, dataplaneURL, params)
	if err != nil {
		return sandbox.ExecResult{}, fmt.Errorf("langsmith exec: %w", err)
	}

	output := result.Stdout
	if result.Stderr != "" {
		output += "\n" + result.Stderr
	}
	return sandbox.ExecResult{
		Output:   strings.TrimRight(output, "\n"),
		ExitCode: int(result.ExitCode),
	}, nil
}

func (b *langsmithBackend) readFile(ctx context.Context, path string) ([]byte, error) {
	b.mu.Lock()
	dataplaneURL := b.dataplaneURL
	b.mu.Unlock()
	return b.client.Sandboxes.Boxes.ReadFileWithDataplaneURL(ctx, dataplaneURL, path)
}

func (b *langsmithBackend) writeFile(ctx context.Context, path string, content []byte) error {
	b.mu.Lock()
	dataplaneURL := b.dataplaneURL
	b.mu.Unlock()
	return b.client.Sandboxes.Boxes.WriteFileWithDataplaneURL(ctx, dataplaneURL, path, content)
}

func (b *langsmithBackend) Close(_ context.Context) error {
	return nil
}

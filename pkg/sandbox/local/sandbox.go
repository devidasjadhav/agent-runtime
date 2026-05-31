package local

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/sandbox"
)

type LocalSandbox struct {
	id  string
	dir string
}

func New(dir string) (*LocalSandbox, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving dir: %w", err)
	}
	return &LocalSandbox{
		id:  fmt.Sprintf("local-%d", time.Now().UnixMilli()),
		dir: abs,
	}, nil
}

func (s *LocalSandbox) ID() string { return s.id }

func (s *LocalSandbox) Exec(ctx context.Context, command string, timeout *int) (sandbox.ExecResult, error) {
	var cancel context.CancelFunc
	if timeout != nil {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(*timeout)*time.Second)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = s.dir
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n" + stderr.String()
	}

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return sandbox.ExecResult{}, fmt.Errorf("exec: %w", err)
		}
	}

	return sandbox.ExecResult{
		Output:   strings.TrimRight(output, "\n"),
		ExitCode: exitCode,
	}, nil
}

func (s *LocalSandbox) ReadFile(_ context.Context, path string, offset, limit int) (sandbox.ReadResult, error) {
	abs := s.resolve(path)
	data, err := os.ReadFile(abs)
	if err != nil {
		return sandbox.ReadResult{Error: err.Error()}, nil
	}
	lines := strings.Split(string(data), "\n")
	start := offset
	if start > len(lines) {
		start = len(lines)
	}
	end := len(lines)
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	content := strings.Join(lines[start:end], "\n")
	return sandbox.ReadResult{Content: content}, nil
}

func (s *LocalSandbox) WriteFile(_ context.Context, path string, content []byte) (sandbox.WriteResult, error) {
	abs := s.resolve(path)
	dir := filepath.Dir(abs)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return sandbox.WriteResult{Error: err.Error()}, nil
	}
	if err := os.WriteFile(abs, content, 0o644); err != nil {
		return sandbox.WriteResult{Error: err.Error()}, nil
	}
	return sandbox.WriteResult{Path: abs}, nil
}

func (s *LocalSandbox) EditFile(_ context.Context, path, oldStr, newStr string, replaceAll bool) (sandbox.EditResult, error) {
	abs := s.resolve(path)
	data, err := os.ReadFile(abs)
	if err != nil {
		return sandbox.EditResult{Error: err.Error()}, nil
	}
	content := string(data)
	count := strings.Count(content, oldStr)
	if count == 0 {
		return sandbox.EditResult{Error: "old_string not found in file"}, nil
	}
	if !replaceAll && count > 1 {
		return sandbox.EditResult{Error: fmt.Sprintf("old_string found %d times, set replace_all=true to replace all", count)}, nil
	}
	if replaceAll {
		content = strings.ReplaceAll(content, oldStr, newStr)
	} else {
		content = strings.Replace(content, oldStr, newStr, 1)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return sandbox.EditResult{Error: err.Error()}, nil
	}
	return sandbox.EditResult{Path: abs, Occurrences: count}, nil
}

func (s *LocalSandbox) Ls(_ context.Context, path string) (sandbox.LsResult, error) {
	abs := s.resolve(path)
	entries, err := os.ReadDir(abs)
	if err != nil {
		return sandbox.LsResult{Error: err.Error()}, nil
	}
	var infos []sandbox.FileInfo
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		infos = append(infos, sandbox.FileInfo{
			Path:  e.Name(),
			IsDir: e.IsDir(),
			Size:  info.Size(),
		})
	}
	return sandbox.LsResult{Entries: infos}, nil
}

func (s *LocalSandbox) Glob(_ context.Context, pattern, basePath string) (sandbox.GlobResult, error) {
	searchDir := s.resolve(basePath)
	matches, err := filepath.Glob(filepath.Join(searchDir, pattern))
	if err != nil {
		return sandbox.GlobResult{Error: err.Error()}, nil
	}
	var infos []sandbox.FileInfo
	for _, m := range matches {
		stat, err := os.Stat(m)
		if err != nil {
			continue
		}
		rel, _ := filepath.Rel(s.dir, m)
		infos = append(infos, sandbox.FileInfo{
			Path:  rel,
			IsDir: stat.IsDir(),
			Size:  stat.Size(),
		})
	}
	return sandbox.GlobResult{Matches: infos}, nil
}

func (s *LocalSandbox) Grep(_ context.Context, pattern string, path *string, _ *string) (sandbox.GrepResult, error) {
	searchDir := s.dir
	if path != nil {
		searchDir = s.resolve(*path)
	}
	cmd := exec.Command("grep", "-rn", "--binary-files=without-match", pattern, searchDir)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	_ = cmd.Run()

	var matches []sandbox.GrepMatch
	for _, line := range strings.Split(stdout.String(), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 {
			continue
		}
		var lineNum int
		fmt.Sscanf(parts[1], "%d", &lineNum)
		matches = append(matches, sandbox.GrepMatch{
			Path: parts[0],
			Line: lineNum,
			Text: parts[2],
		})
	}
	return sandbox.GrepResult{Matches: matches}, nil
}

func (s *LocalSandbox) UploadFiles(_ context.Context, files []sandbox.FileUpload) ([]sandbox.FileUploadResult, error) {
	var results []sandbox.FileUploadResult
	for _, f := range files {
		abs := s.resolve(f.Path)
		dir := filepath.Dir(abs)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			results = append(results, sandbox.FileUploadResult{Path: f.Path, Error: err.Error()})
			continue
		}
		if err := os.WriteFile(abs, f.Content, 0o644); err != nil {
			results = append(results, sandbox.FileUploadResult{Path: f.Path, Error: err.Error()})
			continue
		}
		results = append(results, sandbox.FileUploadResult{Path: f.Path})
	}
	return results, nil
}

func (s *LocalSandbox) DownloadFiles(_ context.Context, paths []string) ([]sandbox.FileDownloadResult, error) {
	var results []sandbox.FileDownloadResult
	for _, p := range paths {
		abs := s.resolve(p)
		data, err := os.ReadFile(abs)
		if err != nil {
			results = append(results, sandbox.FileDownloadResult{Path: p, Error: err.Error()})
			continue
		}
		results = append(results, sandbox.FileDownloadResult{Path: p, Content: data})
	}
	return results, nil
}

func (s *LocalSandbox) Close(_ context.Context) error {
	return nil
}

func (s *LocalSandbox) resolve(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(s.dir, path)
}

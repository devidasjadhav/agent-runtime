package local

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
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
	abs, err := s.resolve(path)
	if err != nil {
		return sandbox.ReadResult{Error: err.Error()}, nil
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return sandbox.ReadResult{Error: err.Error()}, nil
	}
	lines := strings.Split(string(data), "\n")
	start := offset
	if start > len(lines) {
		return sandbox.ReadResult{Error: fmt.Sprintf("Line offset %d exceeds file length (%d lines)", offset, len(lines))}, nil
	}
	end := len(lines)
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	content := strings.Join(lines[start:end], "\n")
	return sandbox.ReadResult{Content: content}, nil
}

func (s *LocalSandbox) WriteFile(_ context.Context, path string, content []byte) (sandbox.WriteResult, error) {
	abs, err := s.resolve(path)
	if err != nil {
		return sandbox.WriteResult{Error: err.Error()}, nil
	}
	if _, err := os.Stat(abs); err == nil {
		return sandbox.WriteResult{Error: fmt.Sprintf("Cannot write to %s because it already exists. Read and then make an edit, or write to a new path.", abs)}, nil
	} else if !os.IsNotExist(err) {
		return sandbox.WriteResult{Error: err.Error()}, nil
	}
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
	abs, err := s.resolve(path)
	if err != nil {
		return sandbox.EditResult{Error: err.Error()}, nil
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return sandbox.EditResult{Error: err.Error()}, nil
	}
	content := string(data)
	count := strings.Count(content, oldStr)
	if count == 0 {
		return sandbox.EditResult{Error: fmt.Sprintf("Error: String not found in file: '%s'", oldStr)}, nil
	}
	if !replaceAll && count > 1 {
		return sandbox.EditResult{Error: fmt.Sprintf("Error: String '%s' appears %d times in file. Use replace_all=True to replace all instances, or provide a more specific string with surrounding context.", oldStr, count)}, nil
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
	abs, err := s.resolve(path)
	if err != nil {
		return sandbox.LsResult{Error: err.Error()}, nil
	}
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
			Path:  filepath.Join(abs, e.Name()),
			IsDir: e.IsDir(),
			Size:  info.Size(),
		})
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Path < infos[j].Path })
	return sandbox.LsResult{Entries: infos}, nil
}

func (s *LocalSandbox) Glob(_ context.Context, pattern, basePath string) (sandbox.GlobResult, error) {
	searchDir, err := s.resolve(basePath)
	if err != nil {
		return sandbox.GlobResult{Error: err.Error()}, nil
	}
	cleanPattern := filepath.ToSlash(strings.TrimPrefix(pattern, "/"))
	var infos []sandbox.FileInfo
	err = filepath.WalkDir(searchDir, func(m string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		stat, err := os.Stat(m)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(searchDir, m)
		rel = filepath.ToSlash(rel)
		matched, err := matchGlob(cleanPattern, rel)
		if err != nil {
			return err
		}
		if !matched {
			return nil
		}
		infos = append(infos, sandbox.FileInfo{
			Path:  m,
			IsDir: stat.IsDir(),
			Size:  stat.Size(),
		})
		return nil
	})
	if err != nil {
		return sandbox.GlobResult{Error: err.Error()}, nil
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Path < infos[j].Path })
	return sandbox.GlobResult{Matches: infos}, nil
}

func (s *LocalSandbox) Grep(_ context.Context, pattern string, pathFilter *string, glob *string) (sandbox.GrepResult, error) {
	searchDir := s.dir
	if pathFilter != nil {
		var err error
		searchDir, err = s.resolve(*pathFilter)
		if err != nil {
			return sandbox.GrepResult{Error: err.Error()}, nil
		}
	}
	args := []string{"-rnF", "--binary-files=without-match", pattern, searchDir}
	cmd := exec.Command("grep", args...)
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
		if glob != nil && *glob != "" {
			rel, _ := filepath.Rel(searchDir, parts[0])
			matched, err := matchGlob(filepath.ToSlash(*glob), filepath.ToSlash(rel))
			if err != nil {
				return sandbox.GrepResult{Error: err.Error()}, nil
			}
			if !matched {
				continue
			}
		}
		matches = append(matches, sandbox.GrepMatch{
			Path: parts[0],
			Line: lineNum,
			Text: parts[2],
		})
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Path == matches[j].Path {
			return matches[i].Line < matches[j].Line
		}
		return matches[i].Path < matches[j].Path
	})
	return sandbox.GrepResult{Matches: matches}, nil
}

func matchGlob(pattern, candidate string) (bool, error) {
	if pattern == "" {
		return false, nil
	}
	if pattern == "**" || pattern == "**/*" {
		return true, nil
	}
	if !strings.Contains(pattern, "**") {
		matched, err := path.Match(pattern, candidate)
		if err != nil || matched || strings.Contains(pattern, "/") {
			return matched, err
		}
		return path.Match(pattern, path.Base(candidate))
	}
	parts := strings.Split(pattern, "**")
	if len(parts) != 2 {
		return false, fmt.Errorf("unsupported glob pattern %q", pattern)
	}
	prefix := strings.TrimSuffix(parts[0], "/")
	suffix := strings.TrimPrefix(parts[1], "/")
	if prefix != "" && !strings.HasPrefix(candidate, prefix+"/") && candidate != prefix {
		return false, nil
	}
	if suffix == "" {
		return true, nil
	}
	for _, start := range suffixCandidates(candidate, prefix) {
		matched, err := path.Match(suffix, start)
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

func suffixCandidates(candidate, prefix string) []string {
	trimmed := candidate
	if prefix != "" {
		trimmed = strings.TrimPrefix(candidate, strings.TrimSuffix(prefix, "/")+"/")
	}
	parts := strings.Split(trimmed, "/")
	var out []string
	for i := range parts {
		out = append(out, strings.Join(parts[i:], "/"))
	}
	return out
}

func (s *LocalSandbox) UploadFiles(_ context.Context, files []sandbox.FileUpload) ([]sandbox.FileUploadResult, error) {
	var results []sandbox.FileUploadResult
	for _, f := range files {
		abs, err := s.resolve(f.Path)
		if err != nil {
			results = append(results, sandbox.FileUploadResult{Path: f.Path, Error: err.Error()})
			continue
		}
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
		abs, err := s.resolve(p)
		if err != nil {
			results = append(results, sandbox.FileDownloadResult{Path: p, Error: err.Error()})
			continue
		}
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

func (s *LocalSandbox) resolve(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("invalid path: path cannot be empty")
	}

	path = strings.ReplaceAll(path, "\\", "/")
	if strings.HasPrefix(path, "~") {
		return "", fmt.Errorf("invalid path %q: home-directory paths are not allowed", path)
	}
	if hasWindowsDrivePrefix(path) {
		return "", fmt.Errorf("invalid path %q: Windows drive paths are not allowed", path)
	}
	if hasParentTraversal(path) {
		return "", fmt.Errorf("invalid path %q: parent directory traversal is not allowed", path)
	}

	normalized := filepath.FromSlash(path)
	normalized = filepath.Clean(normalized)
	var abs string
	if filepath.IsAbs(normalized) {
		if strings.HasPrefix(normalized, s.dir+string(filepath.Separator)) || normalized == s.dir {
			abs = normalized
		} else {
			abs = filepath.Join(s.dir, strings.TrimPrefix(normalized, string(filepath.Separator)))
		}
	} else {
		abs = filepath.Join(s.dir, normalized)
	}

	abs, err := filepath.Abs(abs)
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(s.dir, abs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("invalid path %q: path escapes sandbox root %s", path, s.dir)
	}

	return abs, nil
}

func hasWindowsDrivePrefix(path string) bool {
	return len(path) >= 2 && path[1] == ':' && ((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z'))
}

func hasParentTraversal(path string) bool {
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if part == ".." {
			return true
		}
	}
	return false
}

package builtin_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/sandbox/local"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool/builtin"
)

func TestLocalBuiltinTools(t *testing.T) {
	dir := t.TempDir()
	sbx, err := local.New(dir)
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}

	writeTool := builtin.NewWriteFileTool(sbx)
	writeArgs := json.RawMessage(`{"file_path":"hello.txt","content":"hello world"}`)
	writeResult, err := writeTool.Execute(context.Background(), writeArgs)
	if err != nil {
		t.Fatalf("write Execute: %v", err)
	}
	if !strings.HasPrefix(writeResult.Content, "Updated file ") {
		t.Fatalf("expected write success, got %s", writeResult.Content)
	}

	data, err := os.ReadFile(filepath.Join(dir, "hello.txt"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(data) != "hello world" {
		t.Fatalf("unexpected file content: %q", string(data))
	}

	readTool := builtin.NewReadFileTool(sbx)
	readResult, err := readTool.Execute(context.Background(), json.RawMessage(`{"file_path":"hello.txt"}`))
	if err != nil {
		t.Fatalf("read Execute: %v", err)
	}
	if readResult.Content != "     1\thello world" {
		t.Fatalf("unexpected read content: %q", readResult.Content)
	}

	executeTool := builtin.NewExecuteTool(sbx)
	execResult, err := executeTool.Execute(context.Background(), json.RawMessage(`{"command":"cat hello.txt"}`))
	if err != nil {
		t.Fatalf("execute Execute: %v", err)
	}
	if execResult.Content != "hello world\n[Command succeeded with exit code 0]" {
		t.Fatalf("unexpected exec output: %q", execResult.Content)
	}
}

func TestReadFilePaginationAndEmptyFile(t *testing.T) {
	dir := t.TempDir()
	sbx, err := local.New(dir)
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "lines.txt"), []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	readTool := builtin.NewReadFileTool(sbx)

	result, err := readTool.Execute(context.Background(), json.RawMessage(`{"file_path":"lines.txt","offset":1,"limit":2}`))
	if err != nil {
		t.Fatalf("read Execute: %v", err)
	}
	if result.Content != "     2\tb\n     3\tc" {
		t.Fatalf("unexpected paginated content: %q", result.Content)
	}

	if err := os.WriteFile(filepath.Join(dir, "empty.txt"), []byte(""), 0o644); err != nil {
		t.Fatalf("write empty fixture: %v", err)
	}
	empty, err := readTool.Execute(context.Background(), json.RawMessage(`{"file_path":"empty.txt"}`))
	if err != nil {
		t.Fatalf("read empty Execute: %v", err)
	}
	if empty.Content != "System reminder: File exists but has empty contents" {
		t.Fatalf("unexpected empty-file content: %q", empty.Content)
	}
}

func TestWriteFileRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	sbx, err := local.New(dir)
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}

	writeTool := builtin.NewWriteFileTool(sbx)
	args := json.RawMessage(`{"file_path":"same.txt","content":"first"}`)
	if _, err := writeTool.Execute(context.Background(), args); err != nil {
		t.Fatalf("initial write Execute: %v", err)
	}

	result, err := writeTool.Execute(context.Background(), json.RawMessage(`{"file_path":"same.txt","content":"second"}`))
	if err != nil {
		t.Fatalf("overwrite Execute: %v", err)
	}
	if !result.Error {
		t.Fatal("expected overwrite to be marked as error")
	}
	if !strings.Contains(result.Content, "because it already exists") {
		t.Fatalf("unexpected overwrite message: %q", result.Content)
	}
}

func TestWriteFileMapsVirtualAbsolutePathIntoSandbox(t *testing.T) {
	dir := t.TempDir()
	sbx, err := local.New(dir)
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}

	writeTool := builtin.NewWriteFileTool(sbx)
	result, err := writeTool.Execute(context.Background(), json.RawMessage(`{"file_path":"/tmp/outside-agent-runtime-test.txt","content":"bad"}`))
	if err != nil {
		t.Fatalf("write Execute: %v", err)
	}
	if result.Error {
		t.Fatalf("expected virtual absolute path to map into sandbox, got %q", result.Content)
	}
	expectedPath := filepath.Join(dir, "tmp", "outside-agent-runtime-test.txt")
	if result.Content != "Updated file "+expectedPath {
		t.Fatalf("unexpected virtual absolute path result: %q", result.Content)
	}
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("expected file inside sandbox: %v", err)
	}
}

func TestWriteFileRejectsDangerousPaths(t *testing.T) {
	dir := t.TempDir()
	sbx, err := local.New(dir)
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}

	writeTool := builtin.NewWriteFileTool(sbx)
	cases := []struct {
		name     string
		path     string
		contains string
	}{
		{name: "parent traversal", path: "../escape.txt", contains: "parent directory traversal"},
		{name: "nested parent traversal", path: "safe/../../escape.txt", contains: "parent directory traversal"},
		{name: "home path", path: "~/escape.txt", contains: "home-directory paths"},
		{name: "windows drive", path: `C:\\Users\\escape.txt`, contains: "Windows drive paths"},
		{name: "empty path", path: "", contains: "path cannot be empty"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args, _ := json.Marshal(map[string]string{"file_path": tc.path, "content": "bad"})
			result, err := writeTool.Execute(context.Background(), args)
			if err != nil {
				t.Fatalf("write Execute: %v", err)
			}
			if !result.Error {
				t.Fatalf("expected error for path %q", tc.path)
			}
			if !strings.Contains(result.Content, tc.contains) {
				t.Fatalf("expected %q in error, got %q", tc.contains, result.Content)
			}
		})
	}
}

func TestWriteFileNormalizesBackslashes(t *testing.T) {
	dir := t.TempDir()
	sbx, err := local.New(dir)
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}

	writeTool := builtin.NewWriteFileTool(sbx)
	result, err := writeTool.Execute(context.Background(), json.RawMessage(`{"file_path":"nested\\file.txt","content":"ok"}`))
	if err != nil {
		t.Fatalf("write Execute: %v", err)
	}
	if result.Error {
		t.Fatalf("expected write success, got %q", result.Content)
	}
	data, err := os.ReadFile(filepath.Join(dir, "nested", "file.txt"))
	if err != nil {
		t.Fatalf("read normalized path file: %v", err)
	}
	if string(data) != "ok" {
		t.Fatalf("unexpected content: %q", string(data))
	}
}

func TestEditFileDeepagentsStyleMessages(t *testing.T) {
	dir := t.TempDir()
	sbx, err := local.New(dir)
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.txt"), []byte("mode=dev\nmode=dev\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	editTool := builtin.NewEditFileTool(sbx)
	ambiguous, err := editTool.Execute(context.Background(), json.RawMessage(`{"file_path":"config.txt","old_string":"mode=dev","new_string":"mode=prod"}`))
	if err != nil {
		t.Fatalf("ambiguous edit Execute: %v", err)
	}
	if !ambiguous.Error || !strings.Contains(ambiguous.Content, "appears 2 times") {
		t.Fatalf("expected ambiguous edit error, got: %#v", ambiguous)
	}

	success, err := editTool.Execute(context.Background(), json.RawMessage(`{"file_path":"config.txt","old_string":"mode=dev","new_string":"mode=prod","replace_all":true}`))
	if err != nil {
		t.Fatalf("replace_all edit Execute: %v", err)
	}
	if success.Content != "Successfully replaced 2 instance(s) of the string in '"+filepath.Join(dir, "config.txt")+"'" {
		t.Fatalf("unexpected edit success: %q", success.Content)
	}
}

func TestExecuteFailureAnnotation(t *testing.T) {
	dir := t.TempDir()
	sbx, err := local.New(dir)
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}

	executeTool := builtin.NewExecuteTool(sbx)
	result, err := executeTool.Execute(context.Background(), json.RawMessage(`{"command":"sh -c 'echo nope; exit 7'"}`))
	if err != nil {
		t.Fatalf("execute Execute: %v", err)
	}
	if !strings.Contains(result.Content, "[Command failed with exit code 7]") {
		t.Fatalf("expected failure annotation, got: %q", result.Content)
	}
}

func TestExecuteOutputTruncation(t *testing.T) {
	dir := t.TempDir()
	sbx, err := local.New(dir)
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}

	executeTool := builtin.NewExecuteTool(sbx)
	result, err := executeTool.Execute(context.Background(), json.RawMessage(`{"command":"yes x | head -c 101000"}`))
	if err != nil {
		t.Fatalf("execute Execute: %v", err)
	}
	if !strings.Contains(result.Content, "[Output was truncated due to size limits]") {
		t.Fatalf("expected truncation message, got suffix: %q", result.Content[len(result.Content)-120:])
	}
	if !strings.Contains(result.Content, "[Command succeeded with exit code 0]") {
		t.Fatalf("expected success annotation")
	}
}

func TestLsGlobAndGrepTools(t *testing.T) {
	dir := t.TempDir()
	sbx, err := local.New(dir)
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	fixtures := map[string]string{
		"README.md":     "alpha\n",
		"src/main.go":   "package main\n// alpha\n",
		"src/other.txt": "beta\nalpha\n",
	}
	for name, content := range fixtures {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
	}

	lsTool := builtin.NewLsTool(sbx)
	lsResult, err := lsTool.Execute(context.Background(), json.RawMessage(`{"path":"."}`))
	if err != nil {
		t.Fatalf("ls Execute: %v", err)
	}
	if !strings.Contains(lsResult.Content, filepath.Join(dir, "README.md")) || !strings.Contains(lsResult.Content, filepath.Join(dir, "src")+"/") {
		t.Fatalf("unexpected ls output: %q", lsResult.Content)
	}

	globTool := builtin.NewGlobTool(sbx)
	globResult, err := globTool.Execute(context.Background(), json.RawMessage(`{"pattern":"*.go","path":"."}`))
	if err != nil {
		t.Fatalf("glob Execute: %v", err)
	}
	if !strings.Contains(globResult.Content, filepath.Join(dir, "src/main.go")) {
		t.Fatalf("unexpected glob output: %q", globResult.Content)
	}

	grepTool := builtin.NewGrepTool(sbx)
	filesResult, err := grepTool.Execute(context.Background(), json.RawMessage(`{"pattern":"alpha","path":".","output_mode":"files_with_matches"}`))
	if err != nil {
		t.Fatalf("grep files Execute: %v", err)
	}
	if !strings.Contains(filesResult.Content, filepath.Join(dir, "README.md")) || !strings.Contains(filesResult.Content, filepath.Join(dir, "src/main.go")) {
		t.Fatalf("unexpected grep files output: %q", filesResult.Content)
	}

	contentResult, err := grepTool.Execute(context.Background(), json.RawMessage(`{"pattern":"alpha","path":".","glob":"*.go","output_mode":"content"}`))
	if err != nil {
		t.Fatalf("grep content Execute: %v", err)
	}
	if !strings.Contains(contentResult.Content, "2: // alpha") || strings.Contains(contentResult.Content, "README.md") {
		t.Fatalf("unexpected grep content output: %q", contentResult.Content)
	}

	countResult, err := grepTool.Execute(context.Background(), json.RawMessage(`{"pattern":"alpha","path":".","output_mode":"count"}`))
	if err != nil {
		t.Fatalf("grep count Execute: %v", err)
	}
	if !strings.Contains(countResult.Content, filepath.Join(dir, "README.md")+": 1") {
		t.Fatalf("unexpected grep count output: %q", countResult.Content)
	}
}

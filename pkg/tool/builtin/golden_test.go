package builtin_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/sandbox/local"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool/builtin"
)

func TestGoldenReadFileOutput(t *testing.T) {
	dir := t.TempDir()
	sbx, err := local.New(dir)
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "example.txt"), []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	tool := builtin.NewReadFileTool(sbx)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"file_path":"example.txt","offset":1,"limit":2}`))
	if err != nil {
		t.Fatalf("read_file Execute: %v", err)
	}

	expected := "     2\tbeta\n     3\tgamma"
	if result.Content != expected {
		t.Fatalf("read_file golden mismatch\nexpected: %q\nactual:   %q", expected, result.Content)
	}
}

func TestGoldenWriteFileOutput(t *testing.T) {
	dir := t.TempDir()
	sbx, err := local.New(dir)
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}

	tool := builtin.NewWriteFileTool(sbx)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"file_path":"nested/out.txt","content":"hello"}`))
	if err != nil {
		t.Fatalf("write_file Execute: %v", err)
	}

	expected := "Updated file " + filepath.Join(dir, "nested", "out.txt")
	if result.Content != expected {
		t.Fatalf("write_file golden mismatch\nexpected: %q\nactual:   %q", expected, result.Content)
	}

	overwrite, err := tool.Execute(context.Background(), json.RawMessage(`{"file_path":"nested/out.txt","content":"again"}`))
	if err != nil {
		t.Fatalf("write_file overwrite Execute: %v", err)
	}
	expectedOverwrite := "Cannot write to " + filepath.Join(dir, "nested", "out.txt") + " because it already exists. Read and then make an edit, or write to a new path."
	if overwrite.Content != expectedOverwrite {
		t.Fatalf("write_file overwrite golden mismatch\nexpected: %q\nactual:   %q", expectedOverwrite, overwrite.Content)
	}
}

func TestGoldenEditFileOutput(t *testing.T) {
	dir := t.TempDir()
	sbx, err := local.New(dir)
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}
	path := filepath.Join(dir, "config.txt")
	if err := os.WriteFile(path, []byte("mode=dev\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	tool := builtin.NewEditFileTool(sbx)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"file_path":"config.txt","old_string":"mode=dev","new_string":"mode=prod"}`))
	if err != nil {
		t.Fatalf("edit_file Execute: %v", err)
	}

	expected := "Successfully replaced 1 instance(s) of the string in '" + path + "'"
	if result.Content != expected {
		t.Fatalf("edit_file golden mismatch\nexpected: %q\nactual:   %q", expected, result.Content)
	}

	missing, err := tool.Execute(context.Background(), json.RawMessage(`{"file_path":"config.txt","old_string":"not-there","new_string":"x"}`))
	if err != nil {
		t.Fatalf("edit_file missing Execute: %v", err)
	}
	expectedMissing := "Error: String not found in file: 'not-there'"
	if missing.Content != expectedMissing {
		t.Fatalf("edit_file missing golden mismatch\nexpected: %q\nactual:   %q", expectedMissing, missing.Content)
	}
}

func TestGoldenExecuteOutput(t *testing.T) {
	dir := t.TempDir()
	sbx, err := local.New(dir)
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}

	tool := builtin.NewExecuteTool(sbx)
	success, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"printf hello"}`))
	if err != nil {
		t.Fatalf("execute success Execute: %v", err)
	}
	expectedSuccess := "hello\n[Command succeeded with exit code 0]"
	if success.Content != expectedSuccess {
		t.Fatalf("execute success golden mismatch\nexpected: %q\nactual:   %q", expectedSuccess, success.Content)
	}

	failure, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"sh -c 'printf nope; exit 3'"}`))
	if err != nil {
		t.Fatalf("execute failure Execute: %v", err)
	}
	expectedFailure := "nope\n[Command failed with exit code 3]"
	if failure.Content != expectedFailure {
		t.Fatalf("execute failure golden mismatch\nexpected: %q\nactual:   %q", expectedFailure, failure.Content)
	}
}

func TestGoldenLsGlobGrepOutput(t *testing.T) {
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
	expectedLs := "['" + filepath.Join(dir, "README.md") + "', '" + filepath.Join(dir, "src") + "/']"
	if lsResult.Content != expectedLs {
		t.Fatalf("ls golden mismatch\nexpected: %q\nactual:   %q", expectedLs, lsResult.Content)
	}

	globTool := builtin.NewGlobTool(sbx)
	globResult, err := globTool.Execute(context.Background(), json.RawMessage(`{"pattern":"**/*.go","path":"."}`))
	if err != nil {
		t.Fatalf("glob Execute: %v", err)
	}
	expectedGlob := "['" + filepath.Join(dir, "src", "main.go") + "']"
	if globResult.Content != expectedGlob {
		t.Fatalf("glob golden mismatch\nexpected: %q\nactual:   %q", expectedGlob, globResult.Content)
	}

	grepTool := builtin.NewGrepTool(sbx)
	grepFiles, err := grepTool.Execute(context.Background(), json.RawMessage(`{"pattern":"alpha","path":".","glob":"*.go","output_mode":"files_with_matches"}`))
	if err != nil {
		t.Fatalf("grep files Execute: %v", err)
	}
	expectedGrepFiles := filepath.Join(dir, "src", "main.go")
	if grepFiles.Content != expectedGrepFiles {
		t.Fatalf("grep files golden mismatch\nexpected: %q\nactual:   %q", expectedGrepFiles, grepFiles.Content)
	}

	grepContent, err := grepTool.Execute(context.Background(), json.RawMessage(`{"pattern":"alpha","path":".","glob":"*.go","output_mode":"content"}`))
	if err != nil {
		t.Fatalf("grep content Execute: %v", err)
	}
	expectedGrepContent := filepath.Join(dir, "src", "main.go") + ":\n  2: // alpha"
	if grepContent.Content != expectedGrepContent {
		t.Fatalf("grep content golden mismatch\nexpected: %q\nactual:   %q", expectedGrepContent, grepContent.Content)
	}

	grepCount, err := grepTool.Execute(context.Background(), json.RawMessage(`{"pattern":"alpha","path":".","glob":"*.go","output_mode":"count"}`))
	if err != nil {
		t.Fatalf("grep count Execute: %v", err)
	}
	expectedGrepCount := filepath.Join(dir, "src", "main.go") + ": 1"
	if grepCount.Content != expectedGrepCount {
		t.Fatalf("grep count golden mismatch\nexpected: %q\nactual:   %q", expectedGrepCount, grepCount.Content)
	}
}

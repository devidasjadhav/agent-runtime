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
	var writeParsed map[string]any
	if err := json.Unmarshal(writeResult, &writeParsed); err != nil {
		t.Fatalf("unmarshal write result: %v", err)
	}
	if writeParsed["success"] != true {
		t.Fatalf("expected write success, got %s", string(writeResult))
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
	var readParsed map[string]string
	if err := json.Unmarshal(readResult, &readParsed); err != nil {
		t.Fatalf("unmarshal read result: %v", err)
	}
	if readParsed["content"] != "hello world" {
		t.Fatalf("unexpected read content: %q", readParsed["content"])
	}

	executeTool := builtin.NewExecuteTool(sbx)
	execResult, err := executeTool.Execute(context.Background(), json.RawMessage(`{"command":"cat hello.txt"}`))
	if err != nil {
		t.Fatalf("execute Execute: %v", err)
	}
	var execParsed map[string]any
	if err := json.Unmarshal(execResult, &execParsed); err != nil {
		t.Fatalf("unmarshal exec result: %v", err)
	}
	if execParsed["output"] != "hello world" {
		t.Fatalf("unexpected exec output: %q", execParsed["output"])
	}
}

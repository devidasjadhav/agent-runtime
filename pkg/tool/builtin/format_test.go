package builtin

import (
	"strings"
	"testing"
)

func TestTruncateListOutput(t *testing.T) {
	content := strings.Repeat("x", maxToolOutputChars+100)
	result := truncateListOutput(content)
	if len(result) > maxToolOutputChars {
		t.Fatalf("expected output length <= %d, got %d", maxToolOutputChars, len(result))
	}
	if !strings.Contains(result, listTruncationMessage) {
		t.Fatalf("expected list truncation message, got %q", result[len(result)-100:])
	}
}

func TestTruncateReadOutput(t *testing.T) {
	content := strings.Repeat("x", maxToolOutputChars+100)
	result := truncateReadOutput(content)
	if len(result) > maxToolOutputChars {
		t.Fatalf("expected output length <= %d, got %d", maxToolOutputChars, len(result))
	}
	if !strings.Contains(result, readTruncationMessage) {
		t.Fatalf("expected read truncation message")
	}
}

func TestTruncateExecuteOutput(t *testing.T) {
	content := strings.Repeat("x", maxExecuteOutputChars+100)
	result := truncateExecuteOutput(content)
	if len(result) > maxExecuteOutputChars {
		t.Fatalf("expected output length <= %d, got %d", maxExecuteOutputChars, len(result))
	}
	if !strings.Contains(result, execTruncationMessage) {
		t.Fatalf("expected execute truncation message")
	}
}

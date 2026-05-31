package sandbox_test

import (
	"context"
	"testing"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/sandbox"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/sandbox/local"
)

func TestBaseSandboxFileOperationsViaExec(t *testing.T) {
	localSandbox, err := local.New(t.TempDir())
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}
	sbx := sandbox.NewBaseSandbox(localSandbox)

	write, err := sbx.WriteFile(context.Background(), "example.txt", []byte("alpha\nbeta\n"))
	if err != nil || write.Error != "" {
		t.Fatalf("WriteFile: result=%#v err=%v", write, err)
	}

	read, err := sbx.ReadFile(context.Background(), "example.txt", 1, 1)
	if err != nil || read.Error != "" {
		t.Fatalf("ReadFile: result=%#v err=%v", read, err)
	}
	if read.Content != "beta" {
		t.Fatalf("unexpected read content: %q", read.Content)
	}

	edit, err := sbx.EditFile(context.Background(), "example.txt", "beta", "gamma", false)
	if err != nil || edit.Error != "" || edit.Occurrences != 1 {
		t.Fatalf("EditFile: result=%#v err=%v", edit, err)
	}

	grep, err := sbx.Grep(context.Background(), "gamma", strPtr("."), nil)
	if err != nil || grep.Error != "" || len(grep.Matches) != 1 {
		t.Fatalf("Grep: result=%#v err=%v", grep, err)
	}
}

func TestBaseSandboxUploadDownloadViaExec(t *testing.T) {
	localSandbox, err := local.New(t.TempDir())
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}
	sbx := sandbox.NewBaseSandbox(localSandbox)

	uploads, err := sbx.UploadFiles(context.Background(), []sandbox.FileUpload{
		{Path: "nested/a.txt", Content: []byte("hello")},
		{Path: "nested/b.bin", Content: []byte{0, 1, 2, 3}},
	})
	if err != nil {
		t.Fatalf("UploadFiles: %v", err)
	}
	for _, upload := range uploads {
		if upload.Error != "" {
			t.Fatalf("upload error: %#v", upload)
		}
	}

	downloads, err := sbx.DownloadFiles(context.Background(), []string{"nested/a.txt", "nested/b.bin", "missing.txt"})
	if err != nil {
		t.Fatalf("DownloadFiles: %v", err)
	}
	if string(downloads[0].Content) != "hello" {
		t.Fatalf("unexpected text download: %q", string(downloads[0].Content))
	}
	if string(downloads[1].Content) != string([]byte{0, 1, 2, 3}) {
		t.Fatalf("unexpected binary download: %#v", downloads[1].Content)
	}
	if downloads[2].Error == "" {
		t.Fatalf("expected missing file download error")
	}
}

func strPtr(s string) *string { return &s }

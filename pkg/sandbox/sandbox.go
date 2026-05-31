package sandbox

import (
	"context"
)

type Sandbox interface {
	ID() string
	Exec(ctx context.Context, cmd string, timeout *int) (ExecResult, error)
	ReadFile(ctx context.Context, path string, offset, limit int) (ReadResult, error)
	WriteFile(ctx context.Context, path string, content []byte) (WriteResult, error)
	EditFile(ctx context.Context, path, oldStr, newStr string, replaceAll bool) (EditResult, error)
	Ls(ctx context.Context, path string) (LsResult, error)
	Glob(ctx context.Context, pattern, basePath string) (GlobResult, error)
	Grep(ctx context.Context, pattern string, path *string, glob *string) (GrepResult, error)
	UploadFiles(ctx context.Context, files []FileUpload) ([]FileUploadResult, error)
	DownloadFiles(ctx context.Context, paths []string) ([]FileDownloadResult, error)
	Close(ctx context.Context) error
}

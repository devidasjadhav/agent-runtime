package sandbox

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type ExecBackend interface {
	ID() string
	Exec(ctx context.Context, cmd string, timeout *int) (ExecResult, error)
	Close(ctx context.Context) error
}

type BaseSandbox struct {
	Backend ExecBackend
}

func NewBaseSandbox(backend ExecBackend) *BaseSandbox {
	return &BaseSandbox{Backend: backend}
}

func (s *BaseSandbox) ID() string { return s.Backend.ID() }

func (s *BaseSandbox) Exec(ctx context.Context, cmd string, timeout *int) (ExecResult, error) {
	return s.Backend.Exec(ctx, cmd, timeout)
}

func (s *BaseSandbox) Close(ctx context.Context) error { return s.Backend.Close(ctx) }

func (s *BaseSandbox) ReadFile(ctx context.Context, path string, offset, limit int) (ReadResult, error) {
	args := map[string]any{"path": path, "offset": offset, "limit": limit}
	var out ReadResult
	if err := s.runPythonJSON(ctx, baseReadScript, args, &out); err != nil {
		return ReadResult{Error: err.Error()}, nil
	}
	return out, nil
}

func (s *BaseSandbox) WriteFile(ctx context.Context, path string, content []byte) (WriteResult, error) {
	args := map[string]any{"path": path, "content_b64": base64.StdEncoding.EncodeToString(content)}
	var out WriteResult
	if err := s.runPythonJSON(ctx, baseWriteScript, args, &out); err != nil {
		return WriteResult{Error: err.Error()}, nil
	}
	return out, nil
}

func (s *BaseSandbox) EditFile(ctx context.Context, path, oldStr, newStr string, replaceAll bool) (EditResult, error) {
	args := map[string]any{"path": path, "old": oldStr, "new": newStr, "replace_all": replaceAll}
	var out EditResult
	if err := s.runPythonJSON(ctx, baseEditScript, args, &out); err != nil {
		return EditResult{Error: err.Error()}, nil
	}
	return out, nil
}

func (s *BaseSandbox) Ls(ctx context.Context, path string) (LsResult, error) {
	args := map[string]any{"path": path}
	var out LsResult
	if err := s.runPythonJSON(ctx, baseLsScript, args, &out); err != nil {
		return LsResult{Error: err.Error()}, nil
	}
	return out, nil
}

func (s *BaseSandbox) Glob(ctx context.Context, pattern, basePath string) (GlobResult, error) {
	args := map[string]any{"pattern": pattern, "path": basePath}
	var out GlobResult
	if err := s.runPythonJSON(ctx, baseGlobScript, args, &out); err != nil {
		return GlobResult{Error: err.Error()}, nil
	}
	return out, nil
}

func (s *BaseSandbox) Grep(ctx context.Context, pattern string, path *string, glob *string) (GrepResult, error) {
	args := map[string]any{"pattern": pattern, "path": path, "glob": glob}
	var out GrepResult
	if err := s.runPythonJSON(ctx, baseGrepScript, args, &out); err != nil {
		return GrepResult{Error: err.Error()}, nil
	}
	return out, nil
}

func (s *BaseSandbox) UploadFiles(ctx context.Context, files []FileUpload) ([]FileUploadResult, error) {
	encoded := make([]map[string]string, 0, len(files))
	for _, file := range files {
		encoded = append(encoded, map[string]string{"path": file.Path, "content_b64": base64.StdEncoding.EncodeToString(file.Content)})
	}
	args := map[string]any{"files": encoded}
	var out []FileUploadResult
	if err := s.runPythonJSON(ctx, baseUploadScript, args, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *BaseSandbox) DownloadFiles(ctx context.Context, paths []string) ([]FileDownloadResult, error) {
	args := map[string]any{"paths": paths}
	var raw []struct {
		Path       string `json:"path"`
		ContentB64 string `json:"content_b64"`
		Error      string `json:"error,omitempty"`
	}
	if err := s.runPythonJSON(ctx, baseDownloadScript, args, &raw); err != nil {
		return nil, err
	}
	results := make([]FileDownloadResult, 0, len(raw))
	for _, item := range raw {
		var content []byte
		if item.ContentB64 != "" {
			decoded, err := base64.StdEncoding.DecodeString(item.ContentB64)
			if err != nil {
				results = append(results, FileDownloadResult{Path: item.Path, Error: err.Error()})
				continue
			}
			content = decoded
		}
		results = append(results, FileDownloadResult{Path: item.Path, Content: content, Error: item.Error})
	}
	return results, nil
}

func (s *BaseSandbox) runPythonJSON(ctx context.Context, script string, args map[string]any, out any) error {
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return err
	}
	fullScript := fmt.Sprintf("import json\nARGS = json.loads(%s)\n%s", strconv.Quote(string(argsJSON)), script)
	encoded := base64.StdEncoding.EncodeToString([]byte(fullScript))
	cmd := fmt.Sprintf("echo %s | base64 -d | python3", encoded)
	result, err := s.Exec(ctx, cmd, nil)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("python helper failed: %s", result.Output)
	}
	output := strings.TrimSpace(result.Output)
	if output == "" {
		return fmt.Errorf("python helper returned empty output")
	}
	return json.Unmarshal([]byte(output), out)
}

const baseReadScript = `
import json, os
path = ARGS["path"]
offset = int(ARGS.get("offset") or 0)
limit = int(ARGS.get("limit") or 0)
try:
    with open(path, "r", encoding="utf-8") as f:
        data = f.read()
    lines = data.split("\n")
    if offset > len(lines):
        print(json.dumps({"error": f"Line offset {offset} exceeds file length ({len(lines)} lines)"}))
    else:
        end = len(lines) if limit <= 0 else min(len(lines), offset + limit)
        print(json.dumps({"content": "\n".join(lines[offset:end])}))
except Exception as e:
    print(json.dumps({"error": str(e)}))
`

const baseWriteScript = `
import base64, json, os
path = ARGS["path"]
try:
    if os.path.exists(path):
        print(json.dumps({"error": f"Cannot write to {path} because it already exists. Read and then make an edit, or write to a new path."}))
    else:
        os.makedirs(os.path.dirname(path) or ".", exist_ok=True)
        with open(path, "wb") as f:
            f.write(base64.b64decode(ARGS["content_b64"]))
        print(json.dumps({"path": path}))
except Exception as e:
    print(json.dumps({"error": str(e)}))
`

const baseEditScript = `
import json, os
path = ARGS["path"]
old = ARGS["old"]
new = ARGS["new"]
replace_all = bool(ARGS.get("replace_all"))
try:
    with open(path, "r", encoding="utf-8") as f:
        data = f.read()
    count = data.count(old)
    if count == 0:
        print(json.dumps({"error": f"Error: String not found in file: '{old}'"}))
    elif count > 1 and not replace_all:
        print(json.dumps({"error": f"Error: String '{old}' appears {count} times in file. Use replace_all=True to replace all instances, or provide a more specific string with surrounding context."}))
    else:
        data = data.replace(old, new) if replace_all else data.replace(old, new, 1)
        with open(path, "w", encoding="utf-8") as f:
            f.write(data)
        print(json.dumps({"path": path, "occurrences": count}))
except Exception as e:
    print(json.dumps({"error": str(e)}))
`

const baseLsScript = `
import json, os
path = ARGS["path"]
try:
    entries = []
    for name in sorted(os.listdir(path)):
        p = os.path.join(path, name)
        st = os.stat(p)
        entries.append({"path": p, "is_dir": os.path.isdir(p), "size": st.st_size})
    print(json.dumps({"entries": entries}))
except Exception as e:
    print(json.dumps({"error": str(e)}))
`

const baseGlobScript = `
import glob, json, os
pattern = ARGS["pattern"].lstrip("/")
path = ARGS.get("path") or "/"
try:
    matches = []
    cwd = os.getcwd()
    os.chdir(path)
    for p in sorted(glob.glob(pattern, recursive=True)):
        if os.path.isfile(p):
            full = os.path.abspath(p)
            st = os.stat(full)
            matches.append({"path": full, "is_dir": False, "size": st.st_size})
    os.chdir(cwd)
    print(json.dumps({"matches": matches}))
except Exception as e:
    print(json.dumps({"error": str(e)}))
`

const baseGrepScript = `
import fnmatch, json, os
pattern = ARGS["pattern"]
root = ARGS.get("path") or "."
glob_pat = ARGS.get("glob")
try:
    matches = []
    for dirpath, _, files in os.walk(root):
        for name in files:
            full = os.path.join(dirpath, name)
            rel = os.path.relpath(full, root).replace(os.sep, "/")
            if glob_pat and not (fnmatch.fnmatch(rel, glob_pat) or fnmatch.fnmatch(name, glob_pat)):
                continue
            try:
                with open(full, "r", encoding="utf-8") as f:
                    for idx, line in enumerate(f, start=1):
                        if pattern in line:
                            matches.append({"path": full, "line": idx, "text": line.rstrip("\n")})
            except Exception:
                pass
    matches.sort(key=lambda m: (m["path"], m["line"]))
    print(json.dumps({"matches": matches}))
except Exception as e:
    print(json.dumps({"error": str(e)}))
`

const baseUploadScript = `
import base64, json, os
results = []
for item in ARGS["files"]:
    path = item["path"]
    try:
        os.makedirs(os.path.dirname(path) or ".", exist_ok=True)
        with open(path, "wb") as f:
            f.write(base64.b64decode(item["content_b64"]))
        results.append({"path": path})
    except Exception as e:
        results.append({"path": path, "error": str(e)})
print(json.dumps(results))
`

const baseDownloadScript = `
import base64, json
results = []
for path in ARGS["paths"]:
    try:
        with open(path, "rb") as f:
            results.append({"path": path, "content_b64": base64.b64encode(f.read()).decode("ascii")})
    except Exception as e:
        results.append({"path": path, "error": str(e)})
print(json.dumps(results))
`

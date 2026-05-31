# Phase 1: Deepagents-Compatible Tool Parity

## Goal

Make `agent-runtime` compatible with the practical behavior Open SWE currently gets from `deepagents` filesystem and shell tools, while preserving first-class OpenAI-compatible provider support as a differentiator.

Phase 0 proved the Go loop works. Phase 1 makes the runtime predictable enough for existing Open SWE prompts and workflows.

## Non-Goals

- Production durable orchestration.
- Reviewer-specific finding tools.
- Subagents / `task` tool.
- LangSmith sandbox production adapter.
- Full prompt migration.
- Temporal/River workflow integration.

## Compatibility Principle

Prefer deepagents-compatible tool names, arguments, output shapes, and failure semantics before inventing improved behavior.

Reason: Open SWE's prompts and agent habits were shaped around deepagents-style tools. Compatibility gives us safer A/B testing against the Python runtime.

## Tool Surface Target

Implement these tools with deepagents-compatible names and arguments:

```text
ls(path)
read_file(file_path, offset=0, limit=100)
write_file(file_path, content)
edit_file(file_path, old_string, new_string, replace_all=false)
glob(pattern, path="/")
grep(pattern, path=null, glob=null, output_mode="files_with_matches")
execute(command, timeout=null)
```

## Phase 1A: Model-Facing Tool Result Contract

### Current PoC

Tools return JSON objects such as:

```json
{"content":"hello world"}
```

### Target

Tools should return model-facing plain text content matching deepagents where possible.

Examples:

```text
Updated file /repo/hello.txt
```

```text
     1	hello
     2	world
```

```text
test output
[Command succeeded with exit code 0]
```

### Implementation

- Replace raw JSON tool result handling with a typed `tool.Result`:

```go
type Result struct {
    Content string
    Error   bool
}
```

- Agent tool messages should use `Result.Content` directly.
- Events should show `Result.Content`.
- Tool execution errors should still be converted into model-visible error content.

## Phase 1B: `read_file`

### Target Semantics

- Args: `file_path`, `offset=0`, `limit=100`.
- `offset` is 0-indexed.
- Rendered line numbers are 1-indexed.
- Default `limit` is 100.
- Output uses cat-style line numbering:

```text
     1	line one
     2	line two
```

- Empty files return:

```text
System reminder: File exists but has empty contents
```

- Offset past EOF returns an error-like result:

```text
Error: Line offset N exceeds file length (M lines)
```

### Later Enhancements

- Long-line chunking with continuation markers like `5.1`, `5.2`.
- Binary/multimodal handling.
- 80k-character truncation guidance.

## Phase 1C: `write_file`

### Target Semantics

- Args: `file_path`, `content`.
- Auto-create parent directories.
- Never overwrite existing files.
- On existing file, return:

```text
Cannot write to /path/file because it already exists. Read and then make an edit, or write to a new path.
```

- On success, return:

```text
Updated file /path/file
```

## Phase 1D: `edit_file`

### Target Semantics

- Args: `file_path`, `old_string`, `new_string`, `replace_all=false`.
- Exact match replacement.
- If `replace_all=false`, `old_string` must be unique.
- Multiple matches error:

```text
Error: String '...' appears N times in file. Use replace_all=True to replace all instances, or provide a more specific string with surrounding context.
```

- No match error:

```text
Error: String not found in file: '...'
```

- Success:

```text
Successfully replaced N instance(s) of the string in '/path/file'
```

### Later Enhancements

- CRLF/LF compatibility attempts.
- EOF newline mismatch diagnostics.

## Phase 1E: `execute`

### Target Semantics

- Args: `command`, `timeout=null`.
- Timeout must be non-negative.
- Default max timeout should be configurable.
- Model-facing output includes exit-code annotation:

```text
stdout/stderr here
[Command succeeded with exit code 0]
```

```text
stdout/stderr here
[Command failed with exit code 1]
```

- Large output should be truncated or offloaded in a later step.

## Phase 1F: `ls`, `glob`, `grep`

### `ls`

- Non-recursive directory listing.
- Deterministically sorted.
- Directories should include trailing `/`.
- Output should be a simple path list compatible with deepagents-style prompting.

### `glob`

- Args: `pattern`, `path="/"`.
- Deterministically sorted file matches.
- Add result cap and timeout.

### `grep`

- Args: `pattern`, `path=null`, `glob=null`, `output_mode="files_with_matches"`.
- Literal search, not regex.
- Output modes:
  - `files_with_matches`
  - `content`
  - `count`
- Sorted by file path.

## Phase 1G: Path Validation

Add deepagents-like path validation before filesystem access:

- Normalize `\` to `/`.
- Reject `..` components.
- Reject `~` prefixes.
- Reject Windows drive paths like `C:\...`.
- Normalize paths.
- For local dev sandbox, keep all non-absolute paths rooted in sandbox dir.
- Do not allow host absolute paths by default unless explicitly enabled.

## Phase 1H: Large Output Protection

Add safeguards before real Open SWE integration:

- Truncate `read_file`, `grep`, `glob`, and `execute` outputs around 80k chars.
- Add execute output cap around 100k bytes.
- Later: offload huge `execute` results to `/large_tool_results/{tool_call_id}` with a preview.

## Test Plan

### Unit Tests

- `read_file` line numbering.
- `read_file` offset and limit.
- `read_file` empty file.
- `read_file` offset past EOF.
- `write_file` creates parent dirs.
- `write_file` refuses overwrite.
- `edit_file` unique replacement.
- `edit_file` multiple occurrence error.
- `edit_file` replace_all.
- `execute` success annotation.
- `execute` failure annotation.
- `execute` timeout behavior.
- `grep` literal search.
- `glob` deterministic sorting.
- path validation rejects traversal.

### Live Scenarios

Run with DeepSeek:

```bash
go run ./cmd/demo -task "create hello.txt with hello world"
go run ./cmd/demo -task "create config.txt with mode=dev then edit it to mode=prod and read it"
go run ./cmd/demo -task "create numbers.txt with 1, 2, 3 on separate lines, then run wc -l numbers.txt"
go run ./cmd/demo -stream=false -task "create status.txt with ok and read it"
```

## Phase 1 Completion Criteria

- All seven deepagents filesystem/shell tools exist.
- Tool arg names match deepagents.
- Common output shapes match deepagents enough for existing prompts.
- Local sandbox is path-bounded by default.
- Large outputs cannot blow up context unboundedly.
- Go tests cover compatibility behavior.
- Live DeepSeek demo passes file/read/edit/shell/search scenarios.

## Ordered Remaining Work

### 1. Output Safety

Status: next.

Implement shared truncation utilities and apply them to every model-facing tool output.

Acceptance criteria:

- `execute` output cannot exceed the configured cap before annotation.
- `read_file`, `ls`, `glob`, and `grep` return bounded output.
- Truncation messages are explicit and tell the model to narrow the request.
- Unit tests cover truncation for command output and search/list output.

### 2. Streaming Tool-Call Assembly

Status: pending.

Harden streaming assembly for OpenAI-compatible providers.

Acceptance criteria:

- Tool calls are assembled by provider index/ID, not a single "current tool" pointer.
- Multiple tool calls in one streamed response are handled correctly.
- Interleaved argument chunks are handled correctly.
- Unit tests use a fake streaming provider with interleaved tool-call chunks.

### 3. Path Validation Cleanup

Status: partially implemented.

Local sandbox now blocks absolute paths outside the sandbox root. Finish deepagents-like validation.

Acceptance criteria:

- Reject `..` traversal before filesystem access.
- Reject `~` paths.
- Reject Windows drive paths.
- Normalize slash behavior.
- Tests cover accepted relative paths and rejected dangerous paths.

### 4. Tool Schema Hardening

Status: pending.

Reduce schema drift from manual JSON schema definitions.

Acceptance criteria:

- Add helper constructors for common schema properties.
- Prefer arg structs + schema helper when practical.
- Unit tests validate generated schemas have `type: object` and required fields.

### 5. Compatibility Golden Tests

Status: pending.

Pin exact deepagents-like output for common paths.

Acceptance criteria:

- Golden tests cover `read_file`, `write_file`, `edit_file`, `execute`, `ls`, `glob`, and `grep`.
- Failures are easy to inspect when output changes.

### 6. Commit Phase 1 Slice

Status: pending.

Commit the nested `agent-runtime` repo after output safety and search tool parity are verified.

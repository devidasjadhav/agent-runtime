# Architecture

---

## Overview

agent-runtime is built around three orthogonal concerns that compose cleanly:

```
┌──────────────────────────────────────────────────────────────────────┐
│                            Agent Loop                                │
│                                                                      │
│  Input ──► [ Middleware Chain ] ──► Model Provider ──► Tool Call    │
│               │            ▲              │               │          │
│               │            │              │               ▼          │
│               ▼            │         Stream/Complete  [ Tool Registry ]
│           State (msgs,      │                              │          │
│           extensions)       └──────── BeforeModel          ▼          │
│                                       AfterModel       [ Sandbox ]   │
│                             BeforeTool ◄── ──► AfterTool             │
└──────────────────────────────────────────────────────────────────────┘
```

- **Agent** — the loop that drives model calls and tool dispatch
- **Middleware** — composable hooks that intercept before/after each model call and tool call
- **Sandbox** — the execution environment where tools run

These three layers are independent. You can swap the sandbox without touching middleware, or add middleware without touching the agent loop.

---

## Package map

```
agent-runtime/
├── cmd/
│   └── demo/           CLI entry point — flags, sandbox wiring, streaming output
│
└── pkg/
    ├── agent/          Core agent loop, event streaming, result offloading
    ├── middleware/      Interceptor chain — Memory, ACL, Skills, HITL
    ├── model/          Provider interface, message types, profile resolution
    │   └── openai/     OpenAI-compatible provider (DeepSeek / OpenAI)
    ├── sandbox/        Sandbox interface + BaseSandbox (Python file helpers)
    │   ├── local/      Local bash execution
    │   ├── ssh/        Remote execution over SSH
    │   └── langsmith/  Cloud sandbox (LangSmith)
    ├── tool/           Tool interface + Registry
    │   └── builtin/    12+ production tools (files, exec, web, review)
    └── adapter/        Event type adapters (for external integrations)
```

---

## Agent loop

`pkg/agent/agent.go`

The agent runs a bounded loop (capped by `maxIterations`):

```
1. Apply middleware.BeforeModel(state)
2. Build ModelRequest from state.Messages + state.SystemPromptExtensions
3. Call provider.Complete() or provider.Stream()
4. Apply middleware.AfterModel(state, response)
5. If response has tool calls:
     For each call:
       a. Apply middleware.BeforeTool(toolCall)
       b. Dispatch to tool.Execute()
       c. Apply middleware.AfterTool(toolCall, result)
       d. Append tool result to messages
6. If stop_reason == "end_turn" and no tool calls → done
7. Else → go to 1
```

Tool calls within a single model response are dispatched sequentially in the order the model returns them.

### Result offloading

When a tool result exceeds `offloadLimit` characters, the full output is written to the sandbox (e.g. `tool_result_<id>.txt`) and replaced in the message history with a truncated preview and a path hint. This keeps long-running agents from exceeding context limits.

`SandboxRetentionStore` implements `ResultStore` using the sandbox's `WriteFile`/`ReadFile`:

```go
retentionStore := agent.NewSandboxRetentionStore(sbx, sbx, maxResults)
agent.WithResultOffload(retentionStore, 80_000)  // offload results > 80k chars
```

---

## Middleware

`pkg/middleware/middleware.go`

### Interface

```go
type Middleware interface {
    BeforeModel(ctx context.Context, state *State) (*State, error)
    AfterModel(ctx context.Context, state *State, resp *ModelResponse) (*State, error)
    BeforeTool(ctx context.Context, call *ToolCall) (*ToolCall, error)
    AfterTool(ctx context.Context, call *ToolCall, result *ToolResult) (*ToolResult, error)
}
```

All four hooks are optional — embed `noopMiddleware` to skip any you don't need.

### State

`State` is the shared structure passed through the `BeforeModel` / `AfterModel` chain:

```go
type State struct {
    Messages               []model.Message
    SystemPromptExtensions []string   // appended to system prompt by agent
}
```

Middleware can append to `SystemPromptExtensions` (Memory, Skills) or inspect/mutate `Messages` (queued messages injection, summarization).

### Chain

`Chain` is a slice of `Middleware` values, applied left-to-right for Before hooks and right-to-left for After hooks (standard onion model):

```
BeforeModel:  M1 → M2 → M3 → model → M3 → M2 → M1  :AfterModel
BeforeTool:   M1 → M2 → M3 → tool  → M3 → M2 → M1  :AfterTool
```

### Implemented middleware

| Middleware | Hooks used | What it does |
|---|---|---|
| `MemoryMiddleware` | `BeforeModel` | Loads AGENTS.md files once (sync.Once), appends to SystemPromptExtensions |
| `FilesystemACLMiddleware` | `BeforeTool` | Extracts path from tool args, walks rules, returns error on deny |
| `SkillsMiddleware` | `BeforeModel` | Parses SKILL.md YAML frontmatter once, injects skill directory listing |
| `HumanInTheLoopMiddleware` | `BeforeTool` | Calls ApprovalGate when trigger matches, blocks until approved/rejected |
| `CallLimit` | `BeforeModel` | Returns error when model call count exceeds limit |
| `ErrorHandler` | `AfterTool` | Catches panics/errors from tools, surfaces as tool error messages |

### Adding middleware

```go
// 1. Implement the interface (embed noopMiddleware for unused hooks)
type MyMiddleware struct {
    middleware.noopMiddleware
}

func (m *MyMiddleware) BeforeModel(ctx context.Context, state *middleware.State) (*middleware.State, error) {
    state.SystemPromptExtensions = append(state.SystemPromptExtensions, "Extra instruction.")
    return state, nil
}

// 2. Wire it in
agent.WithMiddleware(&MyMiddleware{})
```

---

## Sandbox

`pkg/sandbox/sandbox.go`

### Interface

```go
type Sandbox interface {
    ID() string
    Exec(ctx, cmd string, timeout *int) (ExecResult, error)
    ReadFile(ctx, path string, offset, limit int) (ReadResult, error)
    WriteFile(ctx, path string, content []byte) (WriteResult, error)
    EditFile(ctx, path, oldStr, newStr string, replaceAll bool) (EditResult, error)
    Ls(ctx, path string) (LsResult, error)
    Glob(ctx, pattern, basePath string) (GlobResult, error)
    Grep(ctx, pattern string, path, glob *string) (GrepResult, error)
    UploadFiles(ctx, files []FileUpload) ([]FileUploadResult, error)
    DownloadFiles(ctx, paths []string) ([]FileDownloadResult, error)
    Close(ctx) error
}
```

### BaseSandbox

`pkg/sandbox/base.go`

`BaseSandbox` implements the full `Sandbox` interface on top of a single `ExecBackend`:

```go
type ExecBackend interface {
    ID() string
    Exec(ctx context.Context, cmd string, timeout *int) (ExecResult, error)
    Close(ctx context.Context) error
}
```

File operations (`ReadFile`, `WriteFile`, `EditFile`, `Ls`, `Glob`, `Grep`) are implemented as **Python helper scripts** injected via `Exec`. The scripts accept JSON arguments via stdin and emit JSON results to stdout. This means any backend that can run `python3 -c "..."` automatically gets the full file operation surface — including remote SSH hosts.

### Sandbox implementations

```
BaseSandbox ◄── LocalSandbox    exec: bash -c in local directory
BaseSandbox ◄── SSHSandbox      exec: ssh session (golang.org/x/crypto/ssh)
BaseSandbox ◄── LangSmithSandbox  exec: LangSmith cloud API
```

### Adding a sandbox backend

```go
// 1. Implement ExecBackend
type DockerBackend struct{ containerID string }
func (b *DockerBackend) ID() string { return "docker-" + b.containerID }
func (b *DockerBackend) Exec(ctx context.Context, cmd string, timeout *int) (sandbox.ExecResult, error) {
    // docker exec b.containerID bash -c cmd
}
func (b *DockerBackend) Close(ctx context.Context) error {
    // docker stop b.containerID
}

// 2. Compose with BaseSandbox
type DockerSandbox struct{ *sandbox.BaseSandbox }

func NewDockerSandbox(containerID string) *DockerSandbox {
    return &DockerSandbox{
        BaseSandbox: sandbox.NewBaseSandbox(&DockerBackend{containerID: containerID}),
    }
}
```

---

## Model provider

`pkg/model/provider.go`

### Interface

```go
type Provider interface {
    Complete(ctx context.Context, req ModelRequest) (*ModelResponse, error)
    Stream(ctx context.Context, req ModelRequest) (<-chan ModelChunk, error)
}
```

### Request / Response types

```go
type ModelRequest struct {
    Model           string
    SystemPrompt    string
    Messages        []Message
    Tools           []ToolFuncDef
    MaxTokens       int
    Temperature     *float64
    ReasoningEffort string  // "low" | "medium" | "high" (provider-dependent)
}

type ModelResponse struct {
    Message    Message
    StopReason string  // "end_turn" | "tool_use" | "max_tokens"
    Usage      Usage
}

type Usage struct {
    InputTokens  int
    OutputTokens int
}
```

### Profile resolution

```
DEEPSEEK_API_KEY set? → DeepSeek profile (api.deepseek.com, deepseek-v4-flash)
OPENAI_API_KEY set?   → OpenAI profile (api.openai.com, gpt-4o)
Neither               → error
```

The OpenAI provider (`pkg/model/openai`) implements both `Complete` and `Stream` using the openai-go SDK, and works with any OpenAI-compatible endpoint via `WithBaseURL`.

---

## Tool system

`pkg/tool/tool.go`

### Interface

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() ToolSchema
    Execute(ctx context.Context, args json.RawMessage) (Result, error)
}
```

### Registry

`tool.Registry` holds a map of `name → Tool`. When the agent builds a `ModelRequest`, it calls `registry.Definitions()` to get the tool list in OpenAI function-calling format. When a tool call comes back, it dispatches via `registry.Execute(ctx, name, args)`.

### Built-in tools

All tools in `pkg/tool/builtin` take a `Sandbox` at construction time:

```go
builtin.NewExecuteTool(sbx)    // execute — run shell commands
builtin.NewReadFileTool(sbx)   // read_file
builtin.NewWriteFileTool(sbx)  // write_file
builtin.NewEditFileTool(sbx)   // edit_file
builtin.NewLsTool(sbx)         // ls
builtin.NewGlobTool(sbx)       // glob
builtin.NewGrepTool(sbx)       // grep
```

Web tools don't take a sandbox (they make outbound HTTP calls directly):

```go
builtin.NewFetchURLTool()
builtin.NewHTTPRequestTool()
builtin.NewWebSearchTool(apiKey)
```

---

## Data flow: streaming agent run

```
caller                agent loop                middleware           model          tool
  │                       │                         │                  │              │
  ├─RunStreaming()────────►│                         │                  │              │
  │                       ├─BeforeModel()───────────►│                  │              │
  │                       │◄────────────────────────┤                  │              │
  │                       ├─Stream(request)──────────────────────────►│              │
  │◄──text_delta──────────│◄─────────────────────────────────────────-│              │
  │◄──text_delta──────────│◄─────────────────────────────────────────-│              │
  │◄──text────────────────│◄─────────────────────────────────────────-│              │
  │◄──tool_call───────────│◄─────────────────────────────────────────-│              │
  │                       ├─AfterModel()────────────►│                  │              │
  │                       │◄────────────────────────┤                  │              │
  │                       ├─BeforeTool()────────────►│                  │              │
  │                       │◄────────────────────────┤                  │              │
  │◄──tool_executing──────│                         │                  │              │
  │                       ├─Execute()────────────────────────────────────────────────►│
  │                       │◄─────────────────────────────────────────────────────────┤
  │                       ├─AfterTool()─────────────►│                  │              │
  │                       │◄────────────────────────┤                  │              │
  │◄──tool_result─────────│                         │                  │              │
  │                       │   (loop back to BeforeModel if more calls) │              │
  │◄──completed───────────│                         │                  │              │
```

---

## Extension points summary

| To add... | Implement | Register via |
|---|---|---|
| New sandbox backend | `ExecBackend` | Compose with `BaseSandbox` |
| New tool | `tool.Tool` | `registry.Register(t)` |
| New middleware | `middleware.Middleware` | `agent.WithMiddleware(m)` |
| New model provider | `model.Provider` | `agent.New(provider, ...)` |
| New approval gate | `middleware.ApprovalGate` | `middleware.NewHumanInTheLoop(gate, trigger)` |
| New file loader | `middleware.FileLoader` | `middleware.NewMemoryMiddleware(loader, ...)` |

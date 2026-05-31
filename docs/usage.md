# Usage Guide

---

## Demo CLI reference

```
go run ./cmd/demo [flags]
```

| Flag | Type | Default | Description |
|---|---|---|---|
| `-task` | string | **required** | Task for the agent to perform |
| `-sandbox` | string | `local` | `local` or `ssh` |
| `-dir` | string | temp dir | Local working directory (local sandbox only) |
| `-model` | string | provider default | Model ID override |
| `-stream` | bool | `true` | Streaming mode (false = wait for full response) |
| `-max-iter` | int | `20` | Maximum agent loop iterations |
| `-memory-file` | string | — | Local AGENTS.md path (MemoryMiddleware) |
| `-acl-deny` | string | — | Glob to deny all file ops (FilesystemACLMiddleware) |
| `-skills-file` | string | — | Local SKILL.md path (SkillsMiddleware) |
| `-hitl` | bool | `false` | Human-in-the-loop approval for write/edit ops |

### SSH environment variables

When `-sandbox ssh` is used, the sandbox is configured from environment:

| Variable | Required | Description |
|---|---|---|
| `SSH_HOST` | yes | Hostname or IP of the remote machine |
| `SSH_USER` | yes | SSH username |
| `SSH_KEY_PATH` | one of | Path to PEM private key |
| `SSH_PASSWORD` | one of | Password (used if `SSH_KEY_PATH` is unset) |
| `SSH_PORT` | no | Port (default: 22) |
| `SSH_DIR` | no | Remote working directory (default: remote home) |

### Provider environment variables

| Variable | Provider | Default model |
|---|---|---|
| `DEEPSEEK_API_KEY` | DeepSeek | `deepseek-v4-flash` |
| `OPENAI_API_KEY` | OpenAI | `gpt-4o` |

---

## Common recipes

### Quick local task

```bash
DEEPSEEK_API_KEY=sk-... go run ./cmd/demo \
  -task "Explain the difference between a mutex and a channel in Go"
```

### SSH task with a fixed working directory

```bash
SSH_HOST=192.168.1.14 SSH_USER=dev SSH_KEY_PATH=~/.ssh/id_rsa SSH_DIR=/home/dev/myproject \
go run ./cmd/demo -sandbox ssh -task "Run the test suite and report failures"
```

### Non-streaming mode (wait for full response, with token counts)

```bash
go run ./cmd/demo -stream=false -task "Refactor main.go to split the HTTP handlers into separate files"
```

### All middleware combined

```bash
go run ./cmd/demo \
  -sandbox ssh \
  -task "Check GPU status, then write a summary to /home/dev/gpu-report.txt" \
  -memory-file ./AGENTS.md \
  -skills-file ./skills/gpu-info/SKILL.md \
  -acl-deny "/etc/**" \
  -hitl
```

---

## Go API

### Minimal agent

```go
package main

import (
    "context"
    "fmt"

    "github.com/anomalyco/open-swe/agent-runtime/pkg/agent"
    "github.com/anomalyco/open-swe/agent-runtime/pkg/middleware"
    "github.com/anomalyco/open-swe/agent-runtime/pkg/model"
    modelopenai "github.com/anomalyco/open-swe/agent-runtime/pkg/model/openai"
    "github.com/anomalyco/open-swe/agent-runtime/pkg/sandbox/local"
    "github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
    "github.com/anomalyco/open-swe/agent-runtime/pkg/tool/builtin"
)

func main() {
    sbx, _ := local.New("/tmp/workspace")
    defer sbx.Close(context.Background())

    registry := tool.NewRegistry()
    registry.Register(builtin.NewExecuteTool(sbx))
    registry.Register(builtin.NewReadFileTool(sbx))
    registry.Register(builtin.NewWriteFileTool(sbx))
    registry.Register(builtin.NewLsTool(sbx))

    provider := modelopenai.NewProvider(os.Getenv("DEEPSEEK_API_KEY"),
        option.WithBaseURL("https://api.deepseek.com"),
    )

    ag := agent.New(provider, registry,
        agent.WithModelID("deepseek-v4-flash"),
        agent.WithSystemPrompt("You are a helpful coding assistant."),
        agent.WithMaxIterations(30),
        agent.WithMiddleware(middleware.NewCallLimit(50)),
        agent.WithMiddleware(middleware.NewErrorHandler()),
    )

    events, _ := ag.RunStreaming(context.Background(), agent.Input{
        Messages: []model.Message{
            {Role: model.RoleUser, Content: "Write hello.py that prints 'Hello, World!'"},
        },
    })

    for e := range events {
        switch e.Type {
        case "text_delta":
            fmt.Print(e.Content)
        case "tool_call":
            fmt.Printf("\n[%s] %s\n", e.ToolCall.Name, e.ToolCall.Args)
        case "completed":
            fmt.Printf("\nDone: %s\n", e.Content)
        }
    }
}
```

### Agent options reference

```go
agent.New(provider, registry,
    agent.WithModelID("deepseek-v4-flash"),
    agent.WithFallbackProvider(fallbackProvider, "gpt-4o"),
    agent.WithSystemPrompt("..."),
    agent.WithMaxTokens(8192),
    agent.WithMaxIterations(50),
    agent.WithMiddleware(m),                          // add one middleware (call multiple times)
    agent.WithResultOffload(store, 80_000),           // offload results > 80k chars
    agent.WithHumanMessageOffloadLimit(200_000),      // offload human messages > 200k chars
)
```

### Event types

```go
for event := range events {
    switch event.Type {
    case "text_delta":      // streaming text chunk (stream mode only)
    case "text":            // complete assistant text message
    case "tool_call":       // agent decided to call a tool
    case "tool_executing":  // tool started running
    case "tool_result":     // tool finished; event.ToolResult has Output and Error
    case "completed":       // agent finished; event.Content is final summary
    case "error":           // fatal error; event.Content has the message
    case "model_call_start":// model call began (complete mode only)
    case "model_call_end":  // model call finished; event.Usage has token counts
    case "model_fallback":  // switched to fallback provider
    }
}
```

---

## Middleware API

### MemoryMiddleware

```go
// Local filesystem loader (reads from the machine running the agent process)
loader := middleware.LocalFileLoader{}

// Custom loader — e.g. read from SSH sandbox
loader := middleware.FileLoaderFunc(func(ctx context.Context, path string) ([]byte, error) {
    r, err := sbx.ReadFile(ctx, path, 0, 0)
    if err != nil || r.Error != "" {
        return nil, errors.New(r.Error)
    }
    return []byte(r.Content), nil
})

mw := middleware.NewMemoryMiddleware(
    loader,
    []string{"./AGENTS.md", "/org/AGENTS.md"},
    middleware.WithHTMLCommentStripping(true), // default true
)
```

### FilesystemACLMiddleware

```go
mw := middleware.NewFilesystemACL(
    []middleware.Permission{
        // Deny all access under /etc
        {Pattern: "/etc/**", Operations: middleware.OpAll, Allow: false},
        // Allow reads anywhere under /home/dev
        {Pattern: "/home/dev/**", Operations: middleware.OpRead, Allow: true},
        // Allow writes only in /tmp
        {Pattern: "/tmp/**", Operations: middleware.OpWrite, Allow: true},
    },
    middleware.WithDefaultDeny(), // optional: deny unmatched paths (default: allow)
)
```

Rules are matched in order. First match wins. Patterns support:
- `*` — matches within a single path segment
- `**` — matches any number of segments
- `?` — matches any single character

Operations:
- `middleware.OpRead` — `read_file`, `ls`, `glob`, `grep`
- `middleware.OpWrite` — `write_file`, `edit_file`
- `middleware.OpExecute` — `execute`
- `middleware.OpAll` — all of the above

### SkillsMiddleware

```go
mw := middleware.NewSkillsMiddleware(
    middleware.LocalFileLoader{},
    []middleware.SkillSource{
        {Path: "./skills/gpu-info/SKILL.md"},             // label defaults to "gpu-info"
        {Path: "/org/skills/deploy/SKILL.md", Label: "Org"},
    },
)
```

SKILL.md frontmatter fields:

```yaml
---
name: my-skill          # required; [a-z0-9-], max 64 chars, must match directory name
description: one line   # required; truncated at 1024 chars
allowed_tools:          # optional
  - execute
  - read_file
---
Full skill content here. The agent reads this on demand via read_file.
```

### HumanInTheLoopMiddleware

```go
// Channel-based gate for programmatic control
gate := middleware.NewChannelApprovalGate()

go func() {
    for req := range gate.C {
        log.Printf("HITL: %s(%s)", req.ToolName, req.Args)
        // approve
        req.Respond(true, "")
        // or reject with feedback
        // req.Respond(false, "this path is protected")
    }
}()

mw := middleware.NewHumanInTheLoop(gate, middleware.TriggerOnWriteOps())

// Close the gate when done to unblock any pending requests
defer gate.Close()
```

Trigger helpers:

```go
middleware.TriggerOnTools("write_file", "execute")      // named tools
middleware.TriggerOnWriteOps()                          // write_file, edit_file, execute
middleware.TriggerAny(t1, t2, t3)                       // logical OR
middleware.TriggerAll(t1, t2)                           // logical AND
middleware.TriggerNever()                               // disabled (useful for feature flags)
```

---

## Custom tools

Implement the `tool.Tool` interface:

```go
type MyTool struct{}

func (t *MyTool) Name() string { return "my_tool" }
func (t *MyTool) Description() string { return "Does something useful." }
func (t *MyTool) Parameters() tool.ToolSchema {
    return tool.ToolSchema{
        Type: "object",
        Properties: map[string]tool.Property{
            "input": {Type: "string", Description: "The input value"},
        },
        Required: []string{"input"},
    }
}
func (t *MyTool) Execute(ctx context.Context, args json.RawMessage) (tool.Result, error) {
    var params struct{ Input string `json:"input"` }
    json.Unmarshal(args, &params)
    return tool.Result{Output: "processed: " + params.Input}, nil
}

// Register it
registry.Register(&MyTool{})
```

---

## Custom sandboxes

Implement the `sandbox.ExecBackend` interface and embed `*sandbox.BaseSandbox` to get all file operations for free:

```go
type MyBackend struct{ id string }

func (b *MyBackend) ID() string { return b.id }
func (b *MyBackend) Exec(ctx context.Context, cmd string, timeout *int) (sandbox.ExecResult, error) {
    // run cmd on your target system
}
func (b *MyBackend) Close(ctx context.Context) error { return nil }

type MySandbox struct{ *sandbox.BaseSandbox }

func NewMySandbox() *MySandbox {
    return &MySandbox{BaseSandbox: sandbox.NewBaseSandbox(&MyBackend{id: "my-backend"})}
}
```

---

## Custom providers

Implement `model.Provider` to connect any LLM:

```go
type MyProvider struct{}

func (p *MyProvider) Complete(ctx context.Context, req model.ModelRequest) (*model.ModelResponse, error) {
    // call your LLM API
}
func (p *MyProvider) Stream(ctx context.Context, req model.ModelRequest) (<-chan model.ModelChunk, error) {
    // return a channel of streaming chunks
}
```

# agent-runtime

> A composable, sandbox-native AI agent runtime for Go — run coding agents locally, over SSH, or in the cloud with a single binary.

```
┌─────────────────────────────────────────────────────────────┐
│                        agent-runtime                        │
│                                                             │
│   Task ──► Agent Loop ──► Middleware Chain ──► Tools        │
│               │                                  │          │
│               │         ┌──────────────┐         │          │
│               └────────►│   Sandbox    │◄────────┘          │
│                         │  local│ssh  │                     │
│                         └──────────────┘                    │
└─────────────────────────────────────────────────────────────┘
```

---

## Features

| | |
|---|---|
| **Streaming agent loop** | Real-time event stream — text deltas, tool calls, results, completions |
| **Dual sandbox backends** | Local bash or remote SSH — same agent code, zero changes |
| **Composable middleware** | Memory injection, filesystem ACLs, skills, human-in-the-loop — all opt-in |
| **OpenAI-compatible models** | DeepSeek, OpenAI, or any compatible provider via env vars |
| **Fallback model support** | Automatic secondary provider on primary failure |
| **Result offloading** | Large tool outputs stored in sandbox, previews kept in context |
| **Rich tool surface** | 12+ built-in tools: files, exec, grep, glob, HTTP, web search |
| **Human-in-the-loop** | Pause before destructive tool calls, approve/reject via channel or stdin |
| **Skills system** | YAML-frontmatter SKILL.md files injected as a skill directory prompt |
| **Memory injection** | AGENTS.md files loaded and appended to system prompt each run |

---

## Quick Start

```bash
# DeepSeek (recommended — fast and cheap)
export DEEPSEEK_API_KEY=sk-...

# Run a task in a local sandbox
go run ./cmd/demo -task "Write a Python script that prints fibonacci numbers"

# Run against a remote machine over SSH
export SSH_HOST=192.168.1.14
export SSH_USER=dev
export SSH_KEY_PATH=~/.ssh/id_rsa
go run ./cmd/demo -sandbox ssh -task "What GPU is in this machine? Use nvidia-smi."
```

---

## Middleware

All middleware is opt-in via flags (demo) or `agent.WithMiddleware(...)` in code.

### MemoryMiddleware — persistent agent instructions

Loads AGENTS.md files and injects their content into the system prompt before every model call. HTML comments are stripped by default.

```bash
go run ./cmd/demo \
  -task "..." \
  -memory-file ./AGENTS.md
```

```go
agent.WithMiddleware(
    middleware.NewMemoryMiddleware(
        middleware.LocalFileLoader{},
        []string{"./AGENTS.md", "/org/AGENTS.md"},
    ),
)
```

**AGENTS.md example:**
```markdown
# Agent Memory
Always prefer Python 3 for scripting.
When querying GPU info, use `nvidia-smi`.
<!-- This comment is stripped automatically -->
```

---

### FilesystemACLMiddleware — path-level access control

Intercepts file and execute tool calls before they run. Rules are evaluated in order; first match wins.

```bash
go run ./cmd/demo \
  -task "Try to read /etc/passwd, then list /home/dev." \
  -acl-deny "/etc/**"
```

```go
agent.WithMiddleware(
    middleware.NewFilesystemACL([]middleware.Permission{
        {Pattern: "/etc/**",        Operations: middleware.OpAll,   Allow: false},
        {Pattern: "/home/dev/**",   Operations: middleware.OpRead,  Allow: true},
        {Pattern: "/tmp/**",        Operations: middleware.OpWrite, Allow: true},
    }),
)
```

Operations: `OpRead` · `OpWrite` · `OpExecute` · `OpAll`

When a rule denies a call, the tool receives: `acl: read denied by rule "/etc/**"`

---

### SkillsMiddleware — progressive skill disclosure

Parses SKILL.md files with YAML frontmatter, injects a skill directory into the system prompt. The agent reads full skill content on demand via `read_file`.

```bash
go run ./cmd/demo \
  -task "List available skills, then use the gpu-info skill." \
  -skills-file ./skills/gpu-info/SKILL.md
```

```go
agent.WithMiddleware(
    middleware.NewSkillsMiddleware(
        middleware.LocalFileLoader{},
        []middleware.SkillSource{
            {Path: "./skills/gpu-info/SKILL.md", Label: "Project"},
            {Path: "/org/skills/deploy/SKILL.md", Label: "Org"},
        },
    ),
)
```

**SKILL.md format:**
```markdown
---
name: gpu-info
description: Query GPU status using nvidia-smi. Returns device name, memory, temperature.
allowed_tools:
  - execute
---

Run `nvidia-smi --query-gpu=name,memory.used,temperature.gpu --format=csv,noheader`
```

Constraints: name must be `[a-z0-9-]`, max 64 chars · description truncated at 1024 chars · files > 10 MB skipped · later sources win on name collision.

---

### HumanInTheLoopMiddleware — approval gate before tool calls

Pauses the agent before trigger-matched tool calls and waits for human approval. On rejection, returns a descriptive error; the agent handles it gracefully.

```bash
# Approve writes interactively via stdin
go run ./cmd/demo \
  -task "Write 'hello world' to /home/dev/test.txt" \
  -hitl
```

```go
// Channel-based gate for programmatic control
gate := middleware.NewChannelApprovalGate()

agent.WithMiddleware(
    middleware.NewHumanInTheLoop(
        gate,
        middleware.TriggerOnWriteOps(),  // write_file, edit_file, execute
    ),
)

// In your control loop:
req := <-gate.C
fmt.Printf("Approve %s? ", req.ToolName)
req.Respond(true, "")
```

**Trigger helpers:**

| Helper | Fires on |
|---|---|
| `TriggerOnTools("write_file", "execute")` | Named tools |
| `TriggerOnWriteOps()` | `write_file`, `edit_file`, `execute` |
| `TriggerAny(t1, t2)` | Any trigger matches |
| `TriggerAll(t1, t2)` | All triggers match |
| `TriggerNever()` | Disabled |

---

## Sandboxes

### Local sandbox

Executes commands via `bash -c` in a working directory on the local machine.

```go
sbx, _ := local.New("/path/to/workspace")
defer sbx.Close(ctx)
```

### SSH sandbox

Executes commands on a remote machine over SSH. File operations use Python helpers injected via `Exec` — no scp or sftp required.

```go
sbx, _ := sshsandbox.New(ssh.Config{
    Host:    "192.168.1.14",
    User:    "dev",
    KeyPath: "~/.ssh/id_rsa",
    Dir:     "/home/dev/workspace",
})
```

Or from environment variables:

```bash
SSH_HOST=192.168.1.14
SSH_USER=dev
SSH_KEY_PATH=~/.ssh/id_rsa
SSH_DIR=/home/dev/workspace
```

```go
sbx, _ := sshsandbox.NewFromEnv()
```

---

## Providers

Set one environment variable — the runtime auto-selects the provider and default model.

| Env var | Provider | Default model |
|---|---|---|
| `DEEPSEEK_API_KEY` | DeepSeek | `deepseek-v4-flash` |
| `OPENAI_API_KEY` | OpenAI | `gpt-4o` |

Override the model with `-model <id>`. Use any OpenAI-compatible provider with `CustomOpenAICompatibleProfile`.

---

## Built-in Tools

| Tool | Description |
|---|---|
| `execute` | Run shell commands (optional timeout) |
| `read_file` | Read files with line-numbered output (offset/limit) |
| `write_file` | Write or overwrite files |
| `edit_file` | Find-and-replace within files |
| `ls` | List directory contents |
| `glob` | Pattern-match file paths |
| `grep` | Regex search across files |
| `fetch_url` | HTTP GET with HTML → text parsing |
| `http_request` | Full HTTP (GET/POST/PUT/DELETE, headers, auth) |
| `web_search` | Search engine queries |
| `write_todos` | Write a TODO list to `/todos.md` |
| `task` | Spawn a sub-agent for parallel work |

---

## Installation

**Requirements:** Go 1.25+

```bash
git clone https://github.com/anomalyco/open-swe
cd open-swe/agent-runtime
go build ./...
```

See [docs/getting-started.md](docs/getting-started.md) for a step-by-step guide.

---

## Documentation

| | |
|---|---|
| [Getting Started](docs/getting-started.md) | Install, configure, run your first agent |
| [Usage Guide](docs/usage.md) | All flags, code examples, middleware recipes |
| [Architecture](docs/architecture.md) | Package map, data flows, extension points |

---

## License

MIT

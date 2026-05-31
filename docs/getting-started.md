# Getting Started

This guide gets you from zero to a running agent in under five minutes.

---

## Prerequisites

| Requirement | Version | Notes |
|---|---|---|
| Go | 1.25+ | `go version` to check |
| Git | any | for cloning |
| SSH access | optional | only for SSH sandbox |
| API key | required | DeepSeek or OpenAI |

---

## 1. Clone and build

```bash
git clone https://github.com/anomalyco/open-swe
cd open-swe/agent-runtime
go build ./...
```

Verify the build:

```bash
go test ./...
# ok  github.com/anomalyco/open-swe/agent-runtime/pkg/agent
# ok  github.com/anomalyco/open-swe/agent-runtime/pkg/middleware
# ok  ...
```

---

## 2. Set your API key

**DeepSeek** (recommended — faster and cheaper for coding tasks):

```bash
export DEEPSEEK_API_KEY=sk-...
```

**OpenAI:**

```bash
export OPENAI_API_KEY=sk-...
```

The runtime auto-detects which key is present and selects the matching provider and default model. You never need to specify the provider name explicitly.

---

## 3. Run your first task (local sandbox)

```bash
go run ./cmd/demo -task "Write a Python script that prints the first 10 Fibonacci numbers, then run it."
```

The agent will:
1. Write `fibonacci.py` in a temporary directory
2. Execute it with `python3`
3. Report the output

You should see streaming output like:

```
=== Agent Demo ===
Task:      Write a Python script that prints the first 10 Fibonacci numbers, then run it.
Sandbox:   local (/tmp/agent-demo-abc123)
Provider:  deepseek
Model:     deepseek-v4-flash
Mode:      streaming
Middleware:

[tool call] write_file({"file_path": "fibonacci.py", ...})
[executing] write_file...
[result] write_file: Updated file fibonacci.py

[tool call] execute({"command": "python3 fibonacci.py"})
[executing] execute...
[result] execute: 0 1 1 2 3 5 8 13 21 34

=== Completed ===
The script prints the first 10 Fibonacci numbers: 0, 1, 1, 2, 3, 5, 8, 13, 21, 34.
```

---

## 4. Run against a remote machine (SSH sandbox)

If you have a remote Linux machine:

```bash
export SSH_HOST=192.168.1.14
export SSH_USER=dev
export SSH_KEY_PATH=~/.ssh/id_rsa    # or SSH_PASSWORD=...
export SSH_DIR=/home/dev             # remote working directory

go run ./cmd/demo \
  -sandbox ssh \
  -task "What is the OS version and how much free disk space is there?"
```

The agent connects over SSH, runs commands remotely, and streams results back. No special setup is needed on the remote host — only a working Python 3 interpreter (used by the file helper scripts).

---

## 5. Add memory (AGENTS.md)

Create an `AGENTS.md` file with instructions that persist across every run:

```markdown
# Agent Instructions

- Always use Python 3 for scripting tasks.
- Prefer `rg` (ripgrep) over `grep` when available.
- Confirm the working directory before writing files.
<!-- This comment is automatically stripped -->
```

Run with memory injection:

```bash
go run ./cmd/demo \
  -task "Search for all TODO comments in the codebase" \
  -memory-file ./AGENTS.md
```

---

## 6. Protect sensitive paths (FilesystemACL)

Block access to `/etc` while allowing everything else:

```bash
go run ./cmd/demo \
  -task "Try to read /etc/passwd, then list the home directory." \
  -acl-deny "/etc/**"
```

The agent will see: `acl: read denied by rule "/etc/**"` and gracefully report the restriction.

---

## 7. Add a skill (SkillsMiddleware)

Create a `SKILL.md` for a reusable capability:

```markdown
---
name: gpu-info
description: Query GPU status and utilization using nvidia-smi.
allowed_tools:
  - execute
---

Run `nvidia-smi --query-gpu=name,memory.used,temperature.gpu --format=csv,noheader`
```

Run with skill injection:

```bash
go run ./cmd/demo \
  -task "Use the gpu-info skill to report GPU status." \
  -skills-file ./skills/gpu-info/SKILL.md
```

The skill directory is injected into the system prompt. The agent reads full skill content on demand via `read_file`.

---

## 8. Enable human-in-the-loop approval

Pause the agent before any write operation and approve via stdin:

```bash
go run ./cmd/demo \
  -task "Write a summary report to /home/dev/report.txt" \
  -hitl
```

When the agent tries to write, you'll see:

```
[HITL] write_file({"file_path": "/home/dev/report.txt", "content": "..."})
Approve? [y/N]:
```

Type `y` to allow or `n` to reject. On rejection the agent receives a clear error and handles it gracefully.

---

## Next steps

- [Usage Guide](usage.md) — all flags, combining middleware, using the Go API
- [Architecture](architecture.md) — how the pieces fit together and how to extend them

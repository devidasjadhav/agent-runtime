# Middleware Implementation Plan

Tracks the remaining middleware phases after Phase 4 parity work.
Ordered simple → complex. Each phase must leave the suite green before the next starts.

Reference: deepagents source at `/tmp/deepagents_pkg/` (local copy only).

---

## Phase A — MemoryMiddleware

**Status:** pending

**Goal:** Inject AGENTS.md file content into the system prompt before every model call,
giving the agent persistent knowledge that survives sandbox restarts.

**Deepagents reference:** `deepagents/middleware/memory.py` (~441 lines)

### Deliverables

- `pkg/middleware/memory.go` — `MemoryMiddleware` struct
  - Constructor: `NewMemoryMiddleware(loader FileLoader, sources []string, opts ...MemoryOption)`
  - `FileLoader` interface: `Load(ctx, path) ([]byte, error)` — accepts sandbox or local fs impl
  - `BeforeModel` hook: load files lazily on first call, strip HTML comments, append to system prompt
  - Option: `WithStripHTMLComments(bool)` (default true)
- Infra change: add `SystemPromptExtensions []string` to `middleware.State`
- `agent/agent.go`: concatenate `state.SystemPromptExtensions` onto system prompt when building request
- Unit tests: mock loader, verify content injected exactly once per run, HTML comment stripping

### Exit criteria

- `go test ./...` passes
- Running demo with an AGENTS.md file in the working dir causes its content to appear in the model request

---

## Phase B — FilesystemACLMiddleware

**Status:** pending

**Goal:** Block file/execute tool calls that violate caller-supplied path rules before they reach the tool.

**Deepagents reference:** `FilesystemPermission` rules inside `deepagents/middleware/filesystem.py`

### Deliverables

- `pkg/middleware/filesystem_acl.go` — `FilesystemACLMiddleware` struct
  - `Permission` struct: `{Pattern string, Operations []Operation, Allow bool}`
  - `Operation` type: `OpRead | OpWrite | OpExecute`
  - Constructor: `NewFilesystemACL(rules []Permission)`
  - `BeforeTool` hook: extract path arg from tool JSON, walk rules (first match wins), return error if denied
  - Tools checked: `read_file`, `write_file`, `edit_file`, `ls`, `glob`, `grep`, `execute` (working dir)
  - Default when no rule matches: allow (configurable via `WithDefaultDeny()`)
- Unit tests: allow/deny on exact paths, glob patterns, default behaviour, unknown tools pass through

### Exit criteria

- `go test ./...` passes
- Deny rule on `/etc/**` blocks a `read_file /etc/passwd` attempt with a clear error message

---

## Phase C — SkillsMiddleware

**Status:** pending  
**Depends on:** Phase A infra (`SystemPromptExtensions`)

**Goal:** Load SKILL.md files, parse YAML frontmatter, inject a skill directory into the system prompt.
Full skill content is read on-demand by the agent via `read_file` (progressive disclosure).

**Deepagents reference:** `deepagents/middleware/skills.py` (~1069 lines)

### Deliverables

- `pkg/middleware/skills.go` — `SkillsMiddleware` struct
  - `SkillMeta` struct: `{Name, Description, AllowedTools []string, FilePath string}`
  - `SkillSource` struct: `{Path string, Label string}`
  - Constructor: `NewSkillsMiddleware(loader FileLoader, sources []SkillSource, opts ...SkillOption)`
  - YAML frontmatter parser (inline, no external YAML lib): `---\nkey: val\n---`
  - `BeforeModel` hook: load skills once, inject directory listing into system prompt via `SystemPromptExtensions`
  - Validation: name must be lowercase alphanumeric + hyphens, max 64 chars, match directory name
  - Truncation: skip skills >10 MB, truncate descriptions >1024 chars
- Unit tests: frontmatter parsing, name validation, multi-source last-wins override, size limits

### Exit criteria

- `go test ./...` passes
- A SKILL.md in a test dir appears as a skill entry in the injected system prompt

---

## Phase D — HumanInTheLoopMiddleware

**Status:** pending

**Goal:** Pause agent execution before configurable trigger conditions (e.g., destructive tool calls),
signal out to a human-facing channel, resume or abort based on the response.

**Deepagents reference:** Concept only — wired at graph level in `graph.py` / `subagents.py`,
not a concrete middleware. Our implementation is original.

### Deliverables

- `pkg/middleware/human_in_loop.go` — `HumanInTheLoopMiddleware` struct
  - `ApprovalGate` interface: `RequestApproval(ctx, toolName, args) (approved bool, feedback string, err error)`
  - `TriggerFunc` type: `func(toolName string, args json.RawMessage) bool`
  - Constructor: `NewHumanInTheLoop(gate ApprovalGate, trigger TriggerFunc)`
  - `BeforeTool` hook: if `trigger(name, args)` returns true, call gate; on reject return error with feedback
  - Built-in trigger helpers: `TriggerOnTools(names ...string)`, `TriggerOnWriteOps()`
  - `ChannelApprovalGate`: concrete gate backed by `chan ApprovalRequest` for control-plane integration
- Unit tests: trigger matching, approve path, reject path, context cancellation

### Exit criteria

- `go test ./...` passes
- `write_file` to a protected path blocks until approval; approval resumes; rejection returns error

---

## Phase E — RubricMiddleware

**Status:** deferred — requires structured output (`ResponseFormat[T]`) first

**Goal:** After agent completes, run a grader sub-agent that evaluates the transcript against
caller-supplied pass/fail criteria. On `needs_revision`, inject feedback and re-run (up to N times).

**Deepagents reference:** `deepagents/middleware/rubric.py` (~814 lines)

### Prerequisite

Structured output support in `model.Provider` (`ResponseFormat[T]` or equivalent JSON schema constraint).

### Deliverables (planned, not yet scoped)

- `pkg/middleware/rubric.go` — `RubricMiddleware` struct
- `GraderResponse` struct: `{Verdict: satisfied|needs_revision|failed, Feedback string, CriteriaResults []CriterionResult}`
- Grader sub-agent: reuses `agent.New` with structured output and a truncated transcript (30 msgs, 4k chars/msg)
- `AfterModel` hook: on `completed` stop-reason, run grader; on `needs_revision` inject feedback message and continue
- Config: `maxIterations int` (default 3), `onEvaluation func(GraderResponse)` callback

### Exit criteria

- `go test ./...` passes
- A rubric that requires "must print fibonacci numbers" causes the agent to retry if it doesn't

---

## Summary Table

| Phase | Middleware | Status | Depends on | Complexity |
|---|---|---|---|---|
| A | MemoryMiddleware | pending | — | Simple |
| B | FilesystemACLMiddleware | pending | — | Medium |
| C | SkillsMiddleware | pending | Phase A infra | Medium |
| D | HumanInTheLoopMiddleware | pending | — | Medium |
| E | RubricMiddleware | deferred | Structured output | Complex |

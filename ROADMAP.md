# Agent Runtime Roadmap

## Current Position

`agent-runtime` has passed Phase 0 and most of Phase 1. The remaining work should now be done in narrow slices that each leave the runtime tested and usable.

## Phase 1 Closeout: Tool Parity Hardening

Goal: make the current filesystem/shell tool surface stable enough to compare against deepagents and safely iterate.

### 1. Schema Hardening

Deliverables:

- Tool schema helper constructors.
- All built-in tools use consistent schema construction.
- Unit test validates every built-in tool schema has:
  - `type: object`
  - non-empty description
  - all required fields present in properties
  - property types set
  - enum values only on string fields for now

Exit criteria:

- `go test ./...` and `go vet ./...` pass.

### 2. Golden Compatibility Tests

Deliverables:

- Golden-style tests for common deepagents-compatible outputs:
  - `read_file` line numbering
  - `write_file` success and overwrite failure
  - `edit_file` success, missing string, ambiguous string
  - `execute` success and failure annotations
  - `ls` list format
  - `glob` list format
  - `grep` files/content/count modes

Exit criteria:

- Output changes become intentional and easy to review.

### 3. Phase 1 Commit

Deliverables:

- Commit all Phase 1 changes in the nested `agent-runtime` Git repo.

## Phase 1.5: Context And Large Result Eviction

Status: first pass implemented.

Goal: go beyond truncation and implement deepagents-like result offloading so long runs remain usable.

### 1. Tool Result Offload

Deliverables:

- Write large tool results to `/large_tool_results/{tool_call_id}` in the sandbox.
- Replace model-facing content with a preview and pointer.
- Include first and last lines in preview.

Status: complete for agent-level tool result offload.

### 2. Message Budget Policy

Deliverables:

- Add configurable character/token budget.
- Track accumulated message size.
- Decide when to truncate, offload, or summarize.

Status: partial. Fixed character thresholds are implemented; policy is not yet configurable beyond runtime options.

### 3. Human Message Offload

Deliverables:

- Store large user messages under `/conversation_history/{uuid}.md`.
- Replace them with a pointer and preview.

Status: complete for large user messages.

### 4. Retention/Cleanup Policy

Deliverables:

- Evict oldest offloaded files once tracked count exceeds a configurable limit.
- No unbounded accumulation of `/large_tool_results/` files during long runs.

Status: complete. `pkg/agent/retention.go` implements `RetentionStore`:
- Wraps any `ResultStore`; tracks written paths in an ordered in-memory list.
- When `len(written) > MaxFiles`, evicts oldest via injected `deleter` func.
- `NewSandboxRetentionStore` is the production constructor: deleter calls `rm -f <path>` via `sandbox.Exec`.
- `maxFiles=0` disables eviction (backward-compatible default).
- 5 unit tests covering eviction order, at-limit, zero-max, and exact-limit cases.
- Wired into `cmd/demo` with `maxFiles=20`.

Remaining:

- Preserve raw execute output before tool-level truncation if needed.
- Revisit local shell isolation; shell commands can still address host absolute paths in dev mode.

## Phase 2: Provider Layer And Fallbacks

Status: foundation implemented.

Goal: make OpenAI-compatible support production-grade, then add native providers.

### 1. Provider Profiles

Deliverables:

- Profiles for DeepSeek, OpenAI, custom OpenAI-compatible base URLs.
- Default model per profile.
- Model quirks in one place.

Status: complete for DeepSeek, OpenAI, and custom OpenAI-compatible profile construction.

### 2. Error Normalization

Deliverables:

- Normalize provider errors into categories:
  - validation
  - auth
  - rate limit
  - timeout
  - transient provider failure
  - terminal provider failure

Status: complete for shared categories and OpenAI-compatible error wrapping.

### 3. Fallback Middleware

Deliverables:

- Retry/fallback to secondary model on configured transient errors.
- Preserve event stream visibility when fallback occurs.

Status: complete for complete calls and stream startup failures. Mid-stream fallback is intentionally unsupported for now.

### 4. Native Providers

Status: skipped. Sticking with DeepSeek via the OpenAI-compatible provider (verified end-to-end). Native Anthropic/Google providers deferred indefinitely.

## Phase 3: Sandbox Providers

Status: foundation implemented; LangSmith adapter complete.

Goal: move from local-dev-only sandboxing to production-compatible providers.

### 1. Base Sandbox

Deliverables:

- `BaseSandbox` equivalent that implements file/search/edit operations via `Exec`.

Status: complete. Includes file/search/edit/upload/download operations through Python helpers over `Exec`.

### 1.5 Upload/Download Hardening

Deliverables:

- Local sandbox upload/download tests.
- Base sandbox upload/download tests.

Status: complete.

### 2. LangSmith Sandbox

Deliverables:

- LangSmith provider adapter.
- Upload/download integration.
- GitHub proxy configuration hook surface.

Status: complete. Concrete adapter in `pkg/sandbox/langsmith/` using `langsmith-go` v0.14.0 SDK. Implements `sandbox.Sandbox` with:
- `Exec` via SDK `RunWithDataplaneURL` (blocking HTTP exec)
- `ReadFile`/`WriteFile` via SDK dataplane download/upload (binary-safe, no base64 overhead)
- `EditFile`/`Ls`/`Glob`/`Grep` via `BaseSandbox` Python helpers over Exec
- `UploadFiles`/`DownloadFiles` via SDK dataplane file transfer
- `Create` (new sandbox from snapshot) with wait-for-ready
- `Connect` (resume existing sandbox, start if stopped)
- `ConfigureGitHubProxy` (sets proxy rules with Bearer token)
- `Refresh` (re-reads sandbox state for dataplane URL changes)
- Unit tests for interface conformance and delegation

### 3. Additional Providers

Deliverables:

- Daytona adapter.
- Runloop adapter.
- Modal adapter.

Status: pending.

## Phase 4: Agent Parity Features

Status: complete.

Goal: replace the remaining high-value deepagents runtime features.

### 1. Todo Tool

Deliverables:

- `write_todos` equivalent.
- Todo state visible in agent state/events.

Status: complete. `pkg/tool/builtin/todo.go` implements `write_todos` tool:
- `TodoState` with thread-safe state management (`sync.Mutex`)
- `TodoItem` with `content` and `status` (pending/in_progress/completed)
- Validates status values and non-empty content
- Summary output with icon-prefixed items (○ pending, ◉ in_progress, ● completed)
- `FormatTodos` helper for rendering sorted by status priority
- Full test coverage (schema, execute, validation, concurrency, formatting)

### 2. Subagents

Deliverables:

- Subagent spec.
- `task` tool.
- Child agent execution with isolated messages and inherited tools.

Status: complete. `pkg/tool/builtin/task.go` implements `task` tool:
- `TaskTool` spawns a child `Agent` with isolated message history
- Inherits provider, registry, sandbox, and middleware from parent
- Configurable max iterations (default 20), system prompt, result offload
- Child runs to completion and returns final text output with usage stats
- Supports optional `prompt` parameter for additional context injection

### 3. Middleware Parity

Deliverables:

- sanitize tool inputs
- model fallback integration
- sandbox circuit breaker
- queued message injection hook
- thinking block sanitizer

Status: complete. All 5 middleware implementations:

- **`SanitizeInputs`** (`pkg/middleware/sanitize.go`): Strips control characters (except `\n`, `\r`, `\t`) from string-valued tool arguments before execution. Preserves non-string args untouched.
- **`CircuitBreaker`** (`pkg/middleware/circuit_breaker.go`): Three-state circuit breaker (closed → open → half-open) with configurable failure threshold and reset timeout. Opens after N consecutive tool failures, blocks `BeforeModel` while open, auto-transitions to half-open after timeout.
- **`QueuedMessages`** (`pkg/middleware/queued_messages.go`): Thread-safe message queue that injects pending messages into the agent state before each model call. Supports `Enqueue`/`EnqueueMany`, tracks injected count, provides `ParseQueuedMessage` helper.
- **`ThinkingBlockSanitizer`** (`pkg/middleware/thinking.go`): Strips `<thinking>...</thinking>` blocks from model output (handles nesting). Configurable strip/no-strip mode. Also provides `SanitizeToolCallArgs` for cleaning thinking blocks from tool arguments.
- **Model fallback integration**: Already implemented in `pkg/agent/agent.go` (`completeWithFallback`/`streamWithFallback`) with `model_fallback` event emission.

### 4. Context Summarization

Deliverables:

- Summarization middleware.
- Configurable summarization model.

Status: complete. `pkg/middleware/summarize.go`:
- **`ContextSummarizer`**: `BeforeModel` middleware that triggers when message count exceeds `MaxMessages`. Keeps `KeepRecent` recent messages, summarizes older messages via a `Summarizer` interface. Replaces old messages with a single `[Conversation summary]` user message.
- **`ProviderSummarizer`**: Concrete `Summarizer` using any `model.Provider` to generate summaries. Configurable model ID and max tokens (default 1024).
- Helper functions: `EstimateTokenCount`, `EstimateMessageTokens`, `MarshalMessagesForSummary`.
- Full test coverage for all middleware (sanitize, circuit breaker, queued messages, thinking blocks).

## Phase 5: Open SWE Integration

Status: complete.

Goal: connect this runtime to a future Go control plane and compare against the Python runtime.

### 1. Core Adapter Framework (`pkg/adapter/`)

Deliverables:

- Agent factory with provider, registry, middleware, sandbox injection.
- Agent handle with run/stream/checkpoint/event streaming.
- Deterministic thread ID generation (Linear, GitHub, Slack, reviewer).
- Configuration types for all three agents.
- SSE event streaming via `EventSink` interface.
- Checkpoint store interface for durable execution.
- Message queue interface for queued message injection.

Status: complete. `pkg/adapter/types.go` + `factory.go` + `handle.go`:
- `AgentFactory` creates coding, reviewer, and style analyzer agents with proper middleware chains
- `AgentHandle` wraps `agent.Agent` with event streaming, checkpoint hooks, and sandbox lifecycle
- Thread ID generation: `ThreadIDFromLinearIssue` (SHA256), `ThreadIDFromGitHubIssue` (SHA256), `ThreadIDFromSlackThread` (SHA256→UUID), `ThreadIDFromReviewerThread` (UUID5)
- `CheckpointStore` / `MessageQueue` / `EventSink` interfaces for control-plane integration
- `SSEStreamWriter` for HTTP SSE event streaming
- `RunResult` with final text, tool calls, usage, and todo items

### 2. Main Coding Agent Adapter

Deliverables:

- Main coding agent adapter.

Status: complete. `AgentFactory.CreateCodingAgent()`:
- Inherits all tools from factory registry (web search, fetch URL, HTTP request, Slack, Linear, GitHub)
- Registers `write_todos` tool with shared `TodoState`
- Registers `task` tool for subagent spawning
- Middleware chain: SanitizeInputs → CallLimit → ThinkingBlockSanitizer
- Optional fallback provider, result offload
- Model, max tokens, max iterations configurable via `AgentConfig`

### 3. Reviewer Agent Adapter

Deliverables:

- Reviewer agent adapter.

Status: complete. `AgentFactory.CreateReviewerAgent()`:
- Registers 6 reviewer-specific tools: `add_finding`, `update_finding`, `list_findings`, `publish_review`, `resolve_finding_thread`, `reply_to_finding_thread`
- Inherits web tools (web_search, fetch_url, http_request) from factory registry
- Leaner middleware chain (no circuit breaker, no step-limit notify)
- `ReviewConfig` with PR metadata, SHAs, re-review context, finding reply context
- `FindingStore` with thread-safe concurrent access, ID generation, update/list operations

### 4. Review-Style Analyzer Adapter

Deliverables:

- Review-style analyzer adapter.

Status: complete. `AgentFactory.CreateStyleAnalyzer()`:
- Registers single `save_review_style_prompt` tool
- Minimal middleware: SanitizeInputs → CallLimit(80) → ThinkingBlockSanitizer
- `StyleAnalyzerConfig` with repo full name, samples text, GitHub token

### 5. Web Tool Adapters

Deliverables:

- Slack/Linear/GitHub/web tool adapters.

Status: complete. `pkg/tool/builtin/`:
- **`http_request`**: Full HTTP client (GET/POST/PUT/PATCH/DELETE), configurable headers, body, response with status/headers/body, 80k truncation
- **`fetch_url`**: URL fetcher with HTML stripping, content-type awareness, 80k truncation
- **`web_search`**: Tavily-compatible web search with query, max results, search depth, domain filtering, answer extraction
- All three tested with `httptest.Server` mock backends
- GitHub/Slack/Linear tools are control-plane integrations that call external APIs — these are registered in the factory's shared registry, not built into the runtime

### 6. Event Streaming to Dashboard

Deliverables:

- Event streaming to dashboard API.

Status: complete. `AgentHandle` supports two modes:
- `Run()` — collects all events, returns `RunResult` with final text, usage, tool calls
- `RunStreaming()` — returns `<-chan SSEEvent` for SSE consumption
- `EventSink` interface for custom sinks; `SSEStreamWriter` for HTTP SSE
- Events proxied through middleware checkpoint hooks

### 7. Checkpoint Hooks

Deliverables:

- Checkpoint hooks for durable execution.

Status: complete. `CheckpointStore` interface with `Save`/`Load`/`List`:
- `AgentHandle.maybeCheckpoint()` saves state on `tool_result` and `completed` events
- `CheckpointState` with thread ID, run ID, step, messages, tool calls, metadata
- Control plane provides concrete implementation (e.g., PostgreSQL, Redis, file-based)

## Phase 6: Advanced Middleware

Status: in planning. See `MIDDLEWARE_PLAN.md` for full spec of each phase.

Goal: close the remaining gap between our middleware stack and deepagents by adding
memory injection, filesystem ACLs, skills/plugin system, and human-in-the-loop gating.

### Phase 6A — MemoryMiddleware

Status: pending.

Deliverables:
- `pkg/middleware/memory.go` — loads AGENTS.md files, injects into system prompt via `SystemPromptExtensions`
- Infra: `SystemPromptExtensions []string` added to `middleware.State`; `agent/agent.go` appends them to request

### Phase 6B — FilesystemACLMiddleware

Status: pending.

Deliverables:
- `pkg/middleware/filesystem_acl.go` — ordered allow/deny glob rules checked in `BeforeTool`
- Operations: `OpRead | OpWrite | OpExecute`; default allow; configurable `WithDefaultDeny()`

### Phase 6C — SkillsMiddleware

Status: pending. Depends on Phase 6A infra.

Deliverables:
- `pkg/middleware/skills.go` — loads SKILL.md files, parses YAML frontmatter, injects skill directory into system prompt
- Progressive disclosure: full content read on-demand via `read_file`

### Phase 6D — HumanInTheLoopMiddleware

Status: pending.

Deliverables:
- `pkg/middleware/human_in_loop.go` — `ApprovalGate` interface + `ChannelApprovalGate` for control-plane integration
- `BeforeTool` blocks on configurable trigger conditions until approval/rejection arrives

### Phase 6E — RubricMiddleware

Status: deferred — requires structured output support first.

Deliverables:
- `pkg/middleware/rubric.go` — post-completion grader sub-agent, `needs_revision` retry loop up to N iterations

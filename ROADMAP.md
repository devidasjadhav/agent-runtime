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

Goal: go beyond truncation and implement deepagents-like result offloading so long runs remain usable.

### 1. Tool Result Offload

Deliverables:

- Write large tool results to `/large_tool_results/{tool_call_id}` in the sandbox.
- Replace model-facing content with a preview and pointer.
- Include first and last lines in preview.

### 2. Message Budget Policy

Deliverables:

- Add configurable character/token budget.
- Track accumulated message size.
- Decide when to truncate, offload, or summarize.

### 3. Human Message Offload

Deliverables:

- Store large user messages under `/conversation_history/{uuid}.md`.
- Replace them with a pointer and preview.

## Phase 2: Provider Layer And Fallbacks

Goal: make OpenAI-compatible support production-grade, then add native providers.

### 1. Provider Profiles

Deliverables:

- Profiles for DeepSeek, OpenAI, custom OpenAI-compatible base URLs.
- Default model per profile.
- Model quirks in one place.

### 2. Error Normalization

Deliverables:

- Normalize provider errors into categories:
  - validation
  - auth
  - rate limit
  - timeout
  - transient provider failure
  - terminal provider failure

### 3. Fallback Middleware

Deliverables:

- Retry/fallback to secondary model on configured transient errors.
- Preserve event stream visibility when fallback occurs.

### 4. Native Providers

Deliverables:

- Anthropic provider.
- Google provider.
- Provider-specific reasoning knobs.

## Phase 3: Sandbox Providers

Goal: move from local-dev-only sandboxing to production-compatible providers.

### 1. Base Sandbox

Deliverables:

- `BaseSandbox` equivalent that implements file/search/edit operations via `Exec`.

### 2. LangSmith Sandbox

Deliverables:

- LangSmith provider adapter.
- Upload/download integration.
- GitHub proxy configuration hook surface.

### 3. Additional Providers

Deliverables:

- Daytona adapter.
- Runloop adapter.
- Modal adapter.

## Phase 4: Agent Parity Features

Goal: replace the remaining high-value deepagents runtime features.

### 1. Todo Tool

Deliverables:

- `write_todos` equivalent.
- Todo state visible in agent state/events.

### 2. Subagents

Deliverables:

- Subagent spec.
- `task` tool.
- Child agent execution with isolated messages and inherited tools.

### 3. Middleware Parity

Deliverables:

- sanitize tool inputs
- model fallback integration
- sandbox circuit breaker
- queued message injection hook
- thinking block sanitizer

### 4. Context Summarization

Deliverables:

- Summarization middleware.
- Configurable summarization model.

## Phase 5: Open SWE Integration

Goal: connect this runtime to a future Go control plane and compare against the Python runtime.

Deliverables:

- Main coding agent adapter.
- Reviewer agent adapter.
- Review-style analyzer adapter.
- Slack/Linear/GitHub/web tool adapters.
- Event streaming to dashboard API.
- Checkpoint hooks for durable execution.
- A/B harness against current Python/deepagents runtime.

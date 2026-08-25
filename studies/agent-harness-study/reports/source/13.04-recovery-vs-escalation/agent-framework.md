# Source Analysis: agent-framework

## Dimension 13.04 — Recovery vs Escalation

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (primary, `python/packages/*`) and .NET/C# (`dotnet/src/*`); `go/` is a README-only placeholder |
| Analyzed | 2026-08-25 |

All citations below are workspace-relative paths under `studies/agent-harness-study/sources/agent-framework/`.

## Summary

Agent Framework implements recovery-vs-escalation as a **layered decision ladder** rather than a single policy engine. At the innermost layer, tool exceptions are converted into model-visible error results so the LLM itself decides whether to retry with corrected arguments (`python/packages/core/agent_framework/_tools.py:1412-1434`). A configurable cap on consecutive errors stops the tool loop gracefully by forcing a final text-only response instead of crashing (`python/packages/core/agent_framework/_tools.py:2718-2733`, `_tools.py:3299-3313`). Escalation to humans happens through an explicit approval-content protocol: tools requiring approval pause the loop, emit `function_approval_request` content, and return control to the caller (`python/packages/core/agent_framework/_tools.py:1796-1832`), which workflows surface as `request_info` events that park the run in `IDLE_WITH_PENDING_REQUESTS` (`python/packages/core/agent_framework/_workflows/_workflow.py:588-598`). Transport-level transient failures get bounded automatic retry: MCP connections reconnect once and replay exactly-once-unsafe calls only when provably safe (`python/packages/core/agent_framework/_mcp.py:2096-2160`, `test_mcp.py:7148-7177`); long-running MCP tasks cancel remote work on abandonment rather than leaving zombies (`_mcp.py:2286-2311`). Fail-closed enforcement is separated from recoverable failure via `MiddlewareFailure` vs ordinary exceptions (`python/packages/core/agent_framework/_middleware.py:85-114`). Policy violations expose a three-way configurable outcome — auto-approve-if-bound, request human approval, or block — each audited (`python/packages/core/agent_framework/security.py:2054-2095`). Recovery state survives restarts via superstep checkpoints (`python/packages/core/agent_framework/_workflows/_checkpoint.py:129-200`) including pending human-input requests (`_workflows/_agent_executor.py:320-358`). The .NET side mirrors this model (`LoopAgent` HITL escape hatch at `dotnet/src/Microsoft.Agents.AI/Harness/Loop/LoopAgent.cs:196-210`; `MaxConsecutiveStuckPolls=60` for MCP tasks at `dotnet/src/Microsoft.Agents.AI.Mcp/McpTaskOptions.cs:58`).

What is *not* built in: automatic backoff/retry for transient model-provider errors (429/timeouts) lives in samples and user middleware (`python/samples/02-agents/auto_retry.py:63-100`), not in the core loop.

## Rating

**Score: 8 / 10**

Rationale against the rubric:

- **Clear model (7–8 band):** every failure class has an explicit disposition — model-recoverable tool errors, capped retries with graceful stop, one-shot transport reconnect, HITL escalation, fail-closed abort, workflow pause/resume. Each is implemented at a cited location and covered by tests.
- **Explicit interfaces:** `FunctionInvocationConfiguration` knobs are validated TypedDict fields (`python/packages/core/agent_framework/_tools.py:1332-1409`); escalation outcomes are first-class content types (`function_approval_request`/`user_input_request`) rather than stringly-typed flags; workflow run states are an enum (`_workflows/_events.py:58-67`).
- **Operational safeguards:** budget caps (`max_iterations`, `max_function_calls`), fail-closed batch cancellation (`_tools.py:1863-1876`), duplicate-task prevention on lost responses (`test_mcp.py:7148`).
- **Not 9–10 because:** (a) there is no built-in default retry/backoff for provider-rate-limit failures — the flagship sample explicitly notes streaming retry "requires more delicate handling" and falls back to no retry (`python/samples/02-agents/auto_retry.py:82-84`); (b) the non-streaming consecutive-error failsafe test is skipped with "behavior needs investigation" (`python/packages/core/tests/core/test_function_invocation_logic.py:2125-2126`); (c) policy audit logs are in-memory per middleware instance with no durable sink (`security.py:1698`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Tool-error → model self-correction | Exceptions become `function_result` contents with `exception=` set, logged WARNING; detail gated by `include_detailed_errors` | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_tools.py:1412-1434` |
| Consecutive-error failsafe | `_update_consecutive_error_count` returns `reached_error_limit`; loop logs "Stopping further function calls" and sets `tool_choice="none"` | `python/packages/core/agent_framework/_tools.py:2718-2733`, `3480-3481` |
| Graceful stop after iteration budget | Phase 3 requests one final response with tools disabled ("Maximum iterations reached… Requesting final response without tools") | `python/packages/core/agent_framework/_tools.py:3307-3313` (non-streaming), `3489-3495` (streaming) |
| Defaults & config validation | `DEFAULT_MAX_ITERATIONS=40`, `DEFAULT_MAX_CONSECUTIVE_ERRORS_PER_REQUEST=3`; negative values raise `ValueError` | `python/packages/core/agent_framework/_tools.py:95-96`, `1403-1408` |
| Function-call budget stop | `_disable_tools_at_function_call_limit` logs and forces `tool_choice="none"`; documented best-effort semantics | `python/packages/core/agent_framework/_tools.py:2736-2748`, docstring `1342-1351` |
| Human escalation (approval) | Batch classification pauses execution when any call requires approval; visible requests returned to caller, session-backed siblings hidden | `python/packages/core/agent_framework/_tools.py:1775-1832` |
| Approval return-to-caller action | `_handle_function_call_results` returns `action="return"` when approval/user-input content present — loop hands control back instead of continuing | `python/packages/core/agent_framework/_tools.py:2829-2854` |
| Approval resume settlement | Approved decisions executed once; rejected converted to results; dangling-call settlement on `MiddlewareFailure` during replay | `python/packages/core/agent_framework/_tools.py:2873-2968`, `2927-2933` |
| Session-backed approval coordination | `ToolApprovalMiddleware` queues requests, supports `auto_approval_rules`, persists rules/state in `AgentSession` | `python/packages/core/agent_framework/_harness/_tool_approval.py:343-394`, `351-379` |
| Loop HITL escape hatch | Agent-loop middleware stops before `should_continue` when a pending `function_approval_request` is present (both stream/non-stream) | `python/packages/core/agent_framework/_harness/_loop.py:443-456`, `543`, `647` |
| .NET equivalent | `LoopAgent.HasPendingApprovalRequests` checked before continue/max-iteration stop; `DefaultMaxIterations=10` | `dotnet/src/Microsoft.Agents.AI/Harness/Loop/LoopAgent.cs:65-68`, `196-210`, `464-484` |
| Workflow human-input events | Executor emits `ctx.request_info(...)` per pending input; workflow parks in `IN_PROGRESS_PENDING_REQUESTS` / `IDLE_WITH_PENDING_REQUESTS` | `python/packages/core/agent_framework/_workflows/_agent_executor.py:434-452`; `python/packages/core/agent_framework/_workflows/_workflow.py:576-598` |
| Workflow terminal-failure path | On exception: drain pending events → yield `WorkflowEvent.failed(WorkflowErrorDetails)` → status `FAILED` → re-raise | `python/packages/core/agent_framework/_workflows/_workflow.py:601-623`; details dataclass `_workflows/_events.py:71-88` |
| Run-state taxonomy | `STARTED/IN_PROGRESS(+PENDING_REQUESTS)/IDLE(+PENDING)/FAILED/CANCELLED` | `python/packages/core/agent_framework/_workflows/_events.py:58-67` |
| Checkpoint storage interface | `CheckpointStorage` protocol (save/load/list/delete/get_latest) + `InMemoryCheckpointStorage`, `FileCheckpointStorage`; restore validated against graph fingerprint | `python/packages/core/agent_framework/_workflows/_checkpoint.py:129-247`; `_workflow.py:333`, `642-651` |
| Pending HITL survives restarts | `on_checkpoint_save/on_checkpoint_restore` persist cache, session, `pending_agent_requests`; failed session restore degrades to fresh session with warning | `python/packages/core/agent_framework/_workflows/_agent_executor.py:320-358` |
| MCP transient reconnect-and-retry | One reconnect attempt on `ClosedResourceError`/"session terminated"; second failure raises wrapped `ToolExecutionException` (INFO then ERROR logged) | `python/packages/core/agent_framework/_mcp.py:2108-2160` |
| Unsafe-replay protection | Connection loss before `task_id` known does NOT retry (would duplicate server-side task); tested explicitly | `python/packages/core/agent_framework/_mcp.py:2313-2354` (fallback only for METHOD_NOT_FOUND/INVALID_PARAMS); test `python/packages/core/tests/core/test_mcp.py:7148-7177` |
| Task abandonment cancels remote work | max_task_wait exceeded / pre-terminal abandonment → best-effort `tasks/cancel` before raising; terminal failures skip cancel | `python/packages/core/agent_framework/_mcp.py:2286-2311` |
| .NET MCP poll bound | `MaxConsecutiveStuckPolls = 60` (validated > 0) | `dotnet/src/Microsoft.Agents.AI.Mcp/McpTaskOptions.cs:51-58`; validation `dotnet/src/Microsoft.Agents.AI.Mcp/McpClientTaskExtensions.cs:51-56` |
| Sub-agent user-input propagation | `UserInputRequiredException` carries child `user_input_request` contents up instead of swallowing them as tool errors | `python/packages/core/agent_framework/exceptions.py:184-209` |
| Fail-closed abort signal | `MiddlewareFailure` never becomes a tool result; cancels in-flight sibling calls; settles service-managed conversations before propagating | `python/packages/core/agent_framework/_middleware.py:85-114`; batch cancellation `_tools.py:1863-1876`; settlement `_tools.py:3457-3471` |
| Graceful middleware termination | `MiddlewareTermination` stops early optionally substituting a result | `python/packages/core/agent_framework/_middleware.py:75-82`, contrast `112-114` |
| Configurable escalation policy (security) | `PolicyEnforcementFunctionMiddleware(approval_on_violation=False, block_on_violation=True)`; approval wins over block; bound single-use approvals | `python/packages/core/agent_framework/security.py:1674-1704`, decision ladder `2061-2095` |
| Human notification content | `_request_policy_violation_approval` emits `Content.from_function_approval_request` + `MiddlewareTermination("Policy approval required")` | `python/packages/core/agent_framework/security.py:1919-1924` |
| Block outcome | `_block_policy_violation` returns structured error dict + terminates | `python/packages/core/agent_framework/security.py:1926-1945` |
| Audit of escalations | Every violation appended to `audit_log` before decision; `get_audit_log()` accessor; also logged WARNING | `python/packages/core/agent_framework/security.py:2054-2055`, `2163-2180` |
| Compaction degradation | Summarization failures absorbed (`return False` = skip compaction); consecutive failures escalate log WARNING→ERROR at threshold 3; success resets | `python/packages/core/agent_framework/_compaction.py:1273-1288`, `1359-1371`, constant `1194` |
| Container cleanup retry→human | Docker `rm -f` retried once with shorter timeout, then ERROR log asks operator for manual cleanup | `python/packages/tools/agent_framework_tools/shell/_docker.py:657-676` |
| Corrupt-state quarantine | Malformed session snapshot quarantined via atomic rename; error message instructs "retry to create a new session" | `python/packages/core/agent_framework/_sessions.py:1959-1972`, `2055-2065` |
| Provider-retry pattern (sample, not core) | tenacity `AsyncRetrying(stop_after_attempt(3), wait_exponential, retry_if_exception_type(RateLimitError))` as class decorator or chat middleware; streaming excluded | `python/samples/02-agents/auto_retry.py:26-28`, `63`, `73-100`, `117-173` |
| Tests: error-counter reset | `test_error_recovery_resets_counter` proves one error does not trip the limit | `python/packages/core/tests/core/test_function_invocation_logic.py:3798-3850` |
| Tests: streaming failsafe | `test_streaming_function_invocation_config_max_consecutive_errors` | `python/packages/core/tests/core/test_function_invocation_logic.py:4465+` |
| Test gap (skipped failsafe) | Non-streaming counterpart skipped: "Error handling and failsafe behavior needs investigation in unified API" | `python/packages/core/tests/core/test_function_invocation_logic.py:2125-2126` |
| Tests: reconnect semantics | Reconnect-once-and-retry on session termination; no-retry on create-disconnect; no silent retry on malformed success | `python/packages/core/tests/core/test_mcp.py:1348`, `7148`, `7255` |
| Tests: compaction escalation | Escalates repeated summary failures; resets after success | `python/packages/core/tests/core/test_compaction.py:997`, `1020` |
| Hosted UI escalation surface | DevUI tracks outgoing approval requests server-side and logs `request_info` HIL events | `python/packages/devui/agent_framework_devui/_executor.py:67-69`, `126-136`, `305-308` |
| AG-UI approval lifecycle | Eight-state occurrence machine (`PENDING…EXPIRED/INDETERMINATE`), idempotency keys, retention windows, safe-to-retry claims | `python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:96-119`, `131-134`, `186`, `244-258` |
| Spec-driven auditability | Function-loop spec maps scenarios to named tests incl. cancellation-retry idempotency and indeterminate-outcome reclamation | `docs/specs/004-python-function-calling-loop.md:403-406`, `510-514` |

*(All paths are workspace-relative under the source root `studies/agent-harness-study/sources/agent-framework/`.)*

## Answers to Dimension Questions

### 1. When does the system retry vs escalate?

Four distinct dispositions, chosen by failure class:

- **Retry via the model:** tool exceptions are folded into `function_result` contents with `exception` set and returned into the transcript so the LLM can correct arguments and try again (`python/packages/core/agent_framework/_tools.py:1412-1434`). This is deliberate design — the sample docs say "The LLM decides whether to retry the call or to respond with something else, based on the exception" (`python/samples/02-agents/tools/function_tool_recover_from_failures.py:17`). A single error never trips anything; the counter resets on success (test at `python/packages/core/tests/core/test_function_invocation_logic.py:3798-3850`).
- **Bounded automatic retry (transport only):** MCP `tools/call` reconnects exactly once on connection loss/session termination and replays (`python/packages/core/agent_framework/_mcp.py:2136-2146`); MCP long-running tasks retry `408 REQUEST_TIMEOUT` polls within `max_task_wait`. Retries are suppressed when unsafe — a lost augmented `tools/call` before a task id exists is *never* re-sent (`python/packages/core/tests/core/test_mcp.py:7148-7177`).
- **Escalate to human:** any tool with `approval_mode="always_require"` (or hosted/declaration-only calls needing input) pauses the batch and returns `function_approval_request`/`user_input_request` content to the caller (`_tools.py:1775-1843`); workflows translate these into `request_info` events and park in `IDLE_WITH_PENDING_REQUESTS` (`_workflow.py:582-593`). Security-policy violations can be routed to the same channel via `approval_on_violation=True` (`security.py:2077-2083`).
- **Stop:** three independent stops, all graceful — consecutive-error cap (default 3) sets `tool_choice="none"` and lets the model write a final answer (`_tools.py:2718-2733`, `3299-3300`); iteration cap (default 40) triggers one final tool-free response (`_tools.py:3307-3313`); function-call budget disables tools mid-run (`_tools.py:2736-2748`). Only enforcement-layer failures crash the run, via `MiddlewareFailure` (`_middleware.py:85-98`).

### 2. Are escalation thresholds configurable?

Yes, extensively, though split across layers rather than centralized:

- Consecutive errors, iterations, total calls, unknown-call termination, detailed-error verbosity: `FunctionInvocationConfiguration` fields with constructor-time validation (`_tools.py:1380-1409`).
- Auto-approval heuristics: `ToolApprovalMiddleware(auto_approval_rules=[...])` callbacks receive the full call content for argument-aware matching, with an explicit security warning about name collisions bypassing approval (`python/packages/core/agent_framework/_harness/_tool_approval.py:355-379`). .NET adds `AutoApprovalRules` plus `MaxAutoApprovalIterations` (`dotnet/src/Microsoft.Agents.AI/Harness/ToolApproval/ToolApprovalAgentOptions.cs:48,68`).
- Violation outcome: `allow_untrusted_tools`, `block_on_violation`, `approval_on_violation` (`security.py:1674-1696`).
- MCP task bounds: `max_task_wait`, `default_ttl`, `cancel_remote_task_on_local_cancellation` (frozen `MCPTaskOptions`, `python/packages/core/agent_framework/_mcp.py:348`, fields at `378-380`; .NET `MaxConsecutiveStuckPolls` at `McpTaskOptions.cs:58`).
- AG-UI retention windows: `indeterminate_retention_seconds` default 7 days (`_approval_lifecycle.py:244-258`).
- Loop caps: Python `max_iterations` (default 10, `None`=unbounded) and judge variant (5) at `_loop.py:122,127`; .NET `LoopAgentOptions.MaxIterations` default 10 (`LoopAgent.cs:68`).
- What is *not* configurable: the MCP reconnect count is hard-coded at one attempt (`for attempt in range(2)`, `_mcp.py:2108`), and the compaction error-log threshold is a module constant (`SUMMARY_FAILURE_ERROR_THRESHOLD = 3`, `_compaction.py:1194`).

### 3. Can the system stop gracefully?

Yes — stopping is the default degradation path, not an exception:

- Error/iteration/budget limits convert into a final text-only generation with tools disabled (`_tools.py:3299-3313`, `3489-3495`), and unexecutable tool content is dropped from the final stream (`_drop_unexecutable_tool_contents_from_update`, used at `_tools.py:3417-3422`).
- `MiddlewareTermination` ends a pipeline early while still delivering a result (`_middleware.py:75-82`).
- Workflows emit structured status transitions to `IDLE_WITH_PENDING_REQUESTS` rather than dying while awaiting humans (`_workflow.py:588-598`), and even the failure path drains pending events and yields a typed `failed` event with stack-trace details before re-raising (`_workflow.py:601-623`).
- Cancellation is cooperative but honest about its limits: a sync tool already running in a worker thread cannot be interrupted and may complete side effects, though its result is discarded and never reaches history (`_middleware.py:95-98`, `_tools.py:1868-1872`).

### 4. Are recovery decisions auditable?

Partially — strong inside components, weak across them:

- Security escalations keep a queryable audit trail: every violation is recorded (type, function, label, turn, canonical reason) before the approve/block decision, exposed via `get_audit_log()` (`security.py:2163-2180`).
- Retry attempts are logged at INFO/WARNING/ERROR with structured OTel span errors attached (`set_mcp_span_error` at `_mcp.py:2119-2151`); docker cleanup names the leaked container for operators (`_docker.py:668-676`).
- Approval control-plane contents are retained in history until resolved, so the transcript itself records what was asked and answered (documented contract in `python/packages/core/AGENTS.md`, "Tool Approval Harness" section; filtering behavior implemented in the function-invocation layer, `_tools.py:2909-2911` hides unresolved batches from model input while keeping them resumable).
- The function-calling loop maintains a written specification whose scenario-to-test mapping enumerates retry-idempotency cases (`docs/specs/004-python-function-calling-loop.md:510-514`).
- Gaps: the policy `audit_log` is a plain in-memory list on the middleware instance (`security.py:1698`) with no persistence/export hook; workflow checkpoint writes carry no built-in decision journal beyond state snapshots; compaction escalates only log severity, with no counter exposed programmatically.

## Architectural Decisions

1. **Recovery is delegated upward to the model first.** Rather than framework-level blind retries of arbitrary tools (which risks duplicating side effects), tool failures become first-class transcript data and the LLM chooses the next move (`_tools.py:1412-1434`; stated intent at `python/samples/02-agents/tools/function_tool_recover_from_failures.py:17`). Automatic retry is reserved for the layer that can prove safety (transport reconnect with known task id).
2. **Two exception classes encode the retry-vs-abort philosophy.** Ordinary function-middleware/tool exceptions fail open into model-visible results; `MiddlewareFailure` is the explicit fail-closed escape that cancels the batch and kills the run (`_middleware.py:86-110`). The docstring contrasts the two explicitly (`112-114`).
3. **Escalation is a content type, not a callback contract.** `function_approval_request` / `user_input_request` flow through the same `Content` pipeline as normal data, which lets chat clients, agents, workflows (`request_info` events, `_agent_executor.py:449`), DevUI (`_executor.py:126-136`), and AG-UI interrupts all consume the same signal without bespoke wiring.
4. **Graceful stop over crash.** All three loop budgets end in "one more model call with `tool_choice='none'`" so the user gets an answer plus an explanation instead of a stack trace (`_tools.py:3307-3324`).
5. **Exactly-once paranoia around side effects.** Lost-before-acknowledged MCP task creates are not retried; ambiguous post-forward failures become *indeterminate* occurrences that are retained (not purgeable) for a safety window (`_approval_lifecycle.py:103-119`, spec rows at `docs/specs/004-python-function-calling-loop.md:512-514`).
6. **Checkpoints include the escalation state.** Pending human requests are checkpointed alongside conversation state so a process restart does not lose an open question to the human (`_agent_executor.py:320-337`).

## Notable Patterns

- **Counter-with-reset:** consecutive-error counters reset on any success (`_update_consecutive_error_count`, `_tools.py:2718-2733`), avoiding cumulative-failure false trips across a long healthy run.
- **Severity-laddered logging as lightweight escalation telemetry:** compaction upgrades WARNING→ERROR after 3 consecutive summary failures and emits it exactly once until success resets the latch (`_compaction.py:1273-1284`).
- **Quarantine-don't-delete:** corrupt session files are atomically renamed aside (`.corrupt` suffix) and the operator is told to retry (`_sessions.py:2055-2065`) — recovery preserves forensic evidence.
- **Batch-level classification before execution:** approval/user-input needs pause the whole parallel batch before any call runs, preventing partial side effects ahead of a human veto (`_tools.py:1775-1795`).
- **Single-use, multi-dimension-bound approvals:** security approvals bind to function+arguments signature, security label, session, and disclosed violation set, and are consumed on use — a granted approval cannot be replayed against changed risk (`security.py:1624-1639`, `1824-1870`).
- **Cross-language symmetry:** the Python `AgentLoopMiddleware` HITL escape hatch and .NET `LoopAgent.HasPendingApprovalRequests` implement the same rule independently (`_loop.py:443-456`; `LoopAgent.cs:464-480`), suggesting a shared internal spec.

## Tradeoffs

- **Model-driven recovery trades determinism for flexibility.** Letting the LLM decide to retry means no guaranteed backoff, no jitter, and no cap other than the consecutive-error counter; a model stuck in argument loops burns up to `max_iterations` (default 40) paid round-trips before the graceful stop.
- **Fail-open default for tools maximizes agent autonomy at the cost of surprise:** an exception from a tool silently becomes "Error: Function failed." unless `include_detailed_errors=True` (`_tools.py:1426-1428`) — safer against prompt-injection via error text, but harder to debug.
- **Hard-coded reconnect depth (one)** keeps MCP semantics simple and exactly-once-safe, but a flaky network needing two reconnects escalates straight to a raised `ToolExecutionException`.
- **In-memory audit structures** (`audit_log`, queued approvals in session state) are cheap and testable but assume a long-lived process; durable audit requires hosts to add their own sinks.
- **Streaming retry asymmetry:** rate-limit retry works non-streaming only; streaming callers fall back to no retry (`auto_retry.py:82-84`), so production streaming agents must implement their own strategy.

## Failure Modes / Edge Cases

- **Skipped failsafe test:** the non-streaming `max_consecutive_errors_per_request` scenario is `@pytest.mark.skip("Error handling and failsafe behavior needs investigation in unified API")` (`test_function_invocation_logic.py:2125-2126`) — the headline recovery knob's primary path lacks a live regression test, though the streaming twin runs and `test_error_recovery_resets_counter` covers reset semantics.
- **Uninterruptible sync tools during fail-closed abort:** cancellation is cooperative; a synchronous tool body may finish its side effects after the batch is abandoned, with results discarded (`_middleware.py:95-98`) — the framework documents rather than solves this.
- **Best-effort remote cancel:** abandonment fires `tasks/cancel` best-effort; if that also fails the server keeps running the task with no client-side record (`_mcp.py:2292`, `2307`).
- **Auto-approval rule collision:** a name-matching auto-approval rule intended for one feature can silently approve an unrelated same-named local tool, bypassing the human gate — called out as a security warning on the constructor (`_tool_approval.py:365-376`).
- **Checkpoint/restore mismatch:** restoring without checkpoint storage raises guidance-bearing errors (`_workflow.py:649-651`), and a failed agent-session restore downgrades to a fresh session with only a WARNING — continuity loss is possible and quiet (`_agent_executor.py:356-358`).
- **Unknown context label fail-open-ish:** if `LabelTrackingFunctionMiddleware` didn't run, policy checks are skipped entirely with just a warning (`security.py:1969-1976`) — misconfiguration disables escalation rather than failing closed.

## Future Considerations

- Add an opt-in, framework-native retry policy (backoff/jitter/predicate) for chat-client transport errors, closing the gap the `auto_retry` sample fills by hand (`python/samples/02-agents/auto_retry.py:73-100`).
- Promote the skipped non-streaming failsafe scenario to a working test or document its deviation from the spec's scenario map (`docs/specs/004-python-function-calling-loop.md`).
- Offer a durable audit-log sink interface on `PolicyEnforcementFunctionMiddleware` (the pieces — canonical violation dicts — already exist at `security.py:2017-2023`).
- Make MCP reconnect depth and compaction thresholds constructor parameters for parity with the other knobs.
- Surface a unified "escalation ledger" across layers (approvals, policy blocks, compaction failures, transport reconnects) since today each subsystem logs in its own vocabulary.

## Questions / Gaps

- No evidence found of push-based human notification channels (email/chat/pager) in framework code; the closest implementations live in sample business logic (`python/samples/03-workflows/declarative/customer_support/ticketing_plugin.py:69` "Send an email notification to escalate ticket engagement"). Searched `python/packages/**` for notification/mail/page patterns; only DevUI logging and approval-request content emission were found.
- No evidence found of a central registry ranking failure severity across subsystems (13.01-style taxonomy applied at runtime); each layer owns its own constants and enums. Searched for shared `severity`/`failure_class` symbols in `python/packages/core`.
- The Go implementation could not be evaluated: `go/` contains only `README.md` (placeholder), so all findings are Python/.NET.
- Whether hosted-provider approval flows (e.g., OpenAI hosted tools returning their own approval requests) honor the same resume/settlement machinery was verified only indirectly via the mixed-batch bypass documentation and code at `_tools.py:1796-1832`; a dedicated end-to-end test was not located within the isolation boundary searched (`packages/core/tests`, `packages/openai/tests`).

---

Generated by dimension `13.04-recovery-vs-escalation` against `agent-framework`.

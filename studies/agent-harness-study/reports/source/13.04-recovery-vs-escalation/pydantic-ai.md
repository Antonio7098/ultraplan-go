# Source Analysis: pydantic-ai

## Dimension 13.04: Recovery vs Escalation

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (pydantic-core, httpx/httpx2, tenacity, anyio; durable-exec adapters for Temporal/DBOS/Prefect) |
| Analyzed | 2026-08-25 |

> All citation paths below are relative to the source root (`studies/agent-harness-study/sources/pydantic-ai/`).

## Summary

Pydantic AI treats "recovery vs escalation" as a layered decision taxonomy rather than a single policy engine. `docs/retries.md:3-15` explicitly maps five retry layers — transport, model fallback, tool, output, and model-request hooks — each with an independent budget and a different escalation surface ("they don't share budgets"). The core control-flow signals are exceptions with distinct semantics: `ModelRetry` asks the model to correct itself (`pydantic_ai_slim/pydantic_ai/exceptions.py:57`), `ToolFailed` reports a terminal failure for the model to adapt to without consuming budget (`pydantic_ai_slim/pydantic_ai/exceptions.py:100`), `ApprovalRequired`/`CallDeferred` escalate to a human or external executor by ending the run with a `DeferredToolRequests` output (`pydantic_ai_slim/pydantic_ai/exceptions.py:150`, `pydantic_ai_slim/pydantic_ai/exceptions.py:168`; `pydantic_ai_slim/pydantic_ai/_deferred.py:27`), and `UsageLimitExceeded`/`UnexpectedModelBehavior` stop the run when recovery budgets are exhausted (`pydantic_ai_slim/pydantic_ai/exceptions.py:459`, `pydantic_ai_slim/pydantic_ai/exceptions.py:478`). Human-in-the-loop is implemented as pause/resume: the run ends with pending approvals, and a follow-up run resumes from message history plus `DeferredToolResults` (`pydantic_ai_slim/pydantic_ai/_deferred.py:155`). Graceful stop is first-class via first-party cancellation that preserves resumable partial history (`pydantic_ai_slim/pydantic_ai/run.py:555`). Every retry/failure/denial decision is persisted in message history as typed parts (`RetryPromptPart`, `ToolReturnPart.outcome`), streamed as events, and serialized into OTel spans — so the audit trail is the transcript itself.

## Rating

**9 / 10.**

Rationale against the rubric:

- **Clear model (7–8 bar met):** the five-layer taxonomy is explicit in docs (`docs/retries.md:5-15`) and enforced in code by separate counters — per-tool retries keyed by tool name (`pydantic_ai_slim/pydantic_ai/tool_manager.py:256-265`) vs the run-scoped output-retry counter (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:361-378`). Exhaustion has a defined terminal outcome (`UnexpectedModelBehavior`) rather than silent looping.
- **Explicit interfaces:** recovery vs escalation vs termination are distinct exception types with docstrings stating intent (`pydantic_ai_slim/pydantic_ai/exceptions.py:57-147`); capability hooks define exactly where recovery can intercept (`on_tool_execute_error`, `on_model_request_error`, `on_run_error`, `handle_deferred_tool_calls` — `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:905`, `:724`, `:537`, `:1099`).
- **Operational safeguards:** usage limits bound runaway recovery loops across requests/tokens/cost/tool-calls (`pydantic_ai_slim/pydantic_ai/usage.py:418-472`); cancellation is deliberately non-recoverable even by hooks (`pydantic_ai_slim/pydantic_ai/run.py:568-569`); Temporal policies classify deterministic failures as non-retryable (`pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_toolset.py:156-170`).
- **Proven under failure:** extensive tests exercise failure paths through the public API — fallback failover and all-fail groups (`tests/models/test_fallback.py:141`, `:540`, `:1527`), HITL approval round-trips (`tests/test_agent.py:10658`), empty-response retry exhaustion (`tests/test_agent.py:3808`), deferred-result error handling (`tests/test_agent.py:10868`).
- Why not 10: there is no dedicated persistent audit store for recovery decisions beyond message history + event stream + OTel spans (acceptable for a library, but consumers must assemble their own audit log); escalation policy (who approves, notification channels) is entirely delegated to the application; and a few recovery surfaces are asymmetric between modalities (realtime sessions cannot pause on deferral and instead answer the model inline — `docs/retries.md:131`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Retry signal (model-directed recovery) | `ModelRetry` exception → becomes `RetryPromptPart` back to the model; serializable for durable runs | `pydantic_ai_slim/pydantic_ai/exceptions.py:57-97` |
| Terminal failure without retry budget | `ToolFailed` produces a failed tool result the model sees but does not count against retries; bounded by `UsageLimits` instead | `pydantic_ai_slim/pydantic_ai/exceptions.py:100-112` |
| Human escalation triggers | `CallDeferred` / `ApprovalRequired` control-flow exceptions | `pydantic_ai_slim/pydantic_ai/exceptions.py:150-183` |
| Escalation payload types | `DeferredToolRequests` (calls/approvals/metadata), `DeferredToolResults`, `ToolApproved` (with `override_args`), `ToolDenied` (with custom denial message) | `pydantic_ai_slim/pydantic_ai/_deferred.py:27-96`, `:100-118`, `:154-197` |
| Approval gate wrapper | `ApprovalRequiredToolset.call_tool` raises `ApprovalRequired` unless `ctx.tool_call_approved`; predicate configurable via `approval_required_func` | `pydantic_ai_slim/pydantic_ai/toolsets/approval_required.py:22-32` |
| Per-tool retry budget enforcement | `_check_max_retries` raises `UnexpectedModelBehavior('Tool ... exceeded max retries count ...')` when `ctx.retries[name] >= max_retries` (`>=` guards negative budgets) | `pydantic_ai_slim/pydantic_ai/tool_manager.py:256-265` |
| Unknown-tool recovery | Hallucinated tool name → `ModelRetry` listing available tools, budgeted under the invented name | `pydantic_ai_slim/pydantic_ai/tool_manager.py:496-517` |
| Hook errors wrapped as retries | Capability-hook `ValidationError`/`ModelRetry` converted to `ToolRetryError` after budget check | `pydantic_ai_slim/pydantic_ai/tool_manager.py:477-490` |
| Output-retry budget | `GraphAgentState.output_retries_used` + `consume_output_retry` raising `UnexpectedModelBehavior` on exhaustion | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:304`, `:361-378` |
| Output retry node construction | `_build_retry_node` increments budget then emits a `ModelRequest` whose only part is a `RetryPromptPart` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1797-1810` |
| Non-actionable response handling | Empty/blank/thinking-only responses: token-limit → hard stop; content filter → `ContentFilterError`; `None`-allowed outputs succeed; otherwise retry prompt enumerating valid alternatives | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1909-1970`, `:2052-2061` |
| Runaway-loop bounds | `UsageLimits`: request/token/cost/tool-call/per-request-input limits checked before each request (`usage.py:492`), after each response (`:537`), and before tool calls (`:553`) | `pydantic_ai_slim/pydantic_ai/usage.py:417-569` |
| Model fallback (recovery by substitution) | `FallbackModel.request` walks models while `_should_fallback(exc)` holds; collects all failures into `FallbackExceptionGroup`; rejected-response costs still counted toward limits | `pydantic_ai_slim/pydantic_ai/models/fallback.py:229-318`, `:174-182`, `:544-554` |
| Fallback trigger configurability | `fallback_on` accepts exception types, sync/async exception handlers, response handlers, or mixed sequences; empty config rejected with guidance | `pydantic_ai_slim/pydantic_ai/models/fallback.py:50-57`, `:104-165` |
| Transport-level retries (below agent) | Tenacity transports + `wait_retry_after` respecting HTTP `Retry-After`; invisible to agent history | `pydantic_ai_slim/pydantic_ai/retries.py:140-236`, `:514-590` |
| Provider hint for backoff | `ModelHTTPError.retry_after` parses `Retry-After` into seconds; lowercased headers | `pydantic_ai_slim/pydantic_ai/exceptions.py:534-609` |
| Graceful stop (first-party cancel) | `AgentRun.cancel()` tears down request/tools/suspended job; surfaces as `RunCancelled` with complete resumable history; explicitly terminal — hooks may observe but not recover | `pydantic_ai_slim/pydantic_ai/run.py:555-584` |
| Cancel state recovery | `RunCancelled.all_messages()` detached snapshot; `RunCancelled.from_cancellation` traverses exception chains for external cancels | `pydantic_ai_slim/pydantic_ai/exceptions.py:268-362` |
| Cancellation machinery | `CancellationToken` / `RunCancellation` with bind/issue/deliver semantics | `pydantic_ai_slim/pydantic_ai/_cancel.py:42-258` |
| Recovery extension points | `on_tool_execute_error` (return replacement result, raise `ModelRetry`, or propagate); `on_run_error` (return `AgentRunResult` to suppress); deferral excluded from both | `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:905-935`, `:537-563` |
| Deferred resolution pipeline | Batch collection, duplicate-ID rejection, inline handler resolution via `HandleDeferredToolCalls`, re-deferral loop, remaining-requests bubble-up | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:910-1041` |
| Resume-path integrity checks | `UserError` if results reference missing/executed tool calls; already-executed calls marked `'skip'` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:660-705` |
| Audit: retry prompts in history | `RetryPromptPart` carries content (string or Pydantic error details), `tool_name`, `tool_call_id`, timestamp; rendered deterministically by `from_error`/`model_response` | `pydantic_ai_slim/pydantic_ai/messages.py:1636-1721` |
| Audit: outcome classification | `ToolReturnPart.outcome ∈ {'success','failed','denied','interrupted'}` with documented semantics per value | `pydantic_ai_slim/pydantic_ai/messages.py:1335-1351` |
| Audit: event stream | `FunctionToolCallEvent`/`FunctionToolResultEvent` (incl. args validity) and `DeferredToolRequestsEvent`/`DeferredToolResultsEvent` | `pydantic_ai_slim/pydantic_ai/messages.py:3977`, `:4012`, `:4060`, `:4086` |
| Audit: telemetry | `RetryPromptPart.otel_message_parts` renders retries into GenAI-conventional span messages | `pydantic_ai_slim/pydantic_ai/messages.py:1723-1740` |
| Durable-execution escalation policy | Temporal activities get `non_retryable_error_types` for `UserError`, `UnexpectedModelBehavior`, `FallbackExceptionGroup`, payload-size errors (deterministic failures never retried) | `pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_toolset.py:156-170`; applied at `pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_durability.py:255-280` |
| MCP error-behavior policy | `tool_error_behavior: 'retry' \| 'error' \| 'failed'` selects ModelRetry vs raise vs ToolFailed per toolset | `pydantic_ai_slim/pydantic_ai/mcp.py:764`, `:875`, `:1387-1459` |
| Budget configuration surface | `AgentRetries` TypedDict (`tools`/`output`) accepted at construction, per-run, override blocks, and specs | `pydantic_ai_slim/pydantic_ai/agent/abstract.py:109-129` |
| Documented design map | Five-layer table incl. what each layer adds to history; "What is never retried" section | `docs/retries.md:5-15`, `:127-132` |
| Failure-decision table (docs) | What a tool raise does: retry / failed result / defer-to-human / propagate | `docs/timeouts.md:34-49` |

## Answers to Dimension Questions

**1. When does the system retry vs escalate?**
The decision is made by which exception a failure site raises, not by a central policy evaluator:
- *Retry (ask the model again):* `ModelRetry` raised from tools, args validators, output validators, or capability hooks (`pydantic_ai_slim/pydantic_ai/exceptions.py:57-66`); argument-validation failures wrapped into `RetryPromptPart` (`pydantic_ai_slim/pydantic_ai/tool_manager.py:268-271`); unknown/hallucinated tool names (`pydantic_ai_slim/pydantic_ai/tool_manager.py:514`); responses with nothing actionable (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2052-2061`); MCP server tool errors when `tool_error_behavior='retry'` (default — `pydantic_ai_slim/pydantic_ai/mcp.py:875`). Each retry consumes one unit of a named budget.
- *Escalate to human/external:* only `ApprovalRequired` and `CallDeferred`. They end the run with a `DeferredToolRequests` output unless a registered `HandleDeferredToolCalls` handler resolves them inline (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:964-1041`; `docs/timeouts.md:42`). Denial returns `ToolDenied` as the tool's message to the model (`pydantic_ai_slim/pydantic_ai/messages.py:1341-1343`).
- *Stop:* retry-budget exhaustion (`UnexpectedModelBehavior` — `pydantic_ai_slim/pydantic_ai/tool_manager.py:256-265`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:374-378`), usage-limit breach (`UsageLimitExceeded` — `pydantic_ai_slim/pydantic_ai/usage.py:492-560`), token-limit-truncated responses (`_agent_graph.py:1928-1931`), and any other exception, which propagates out of `agent.run()` unless a capability's `on_tool_execute_error`/`on_model_request_error`/`on_run_error` suppresses or converts it (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:905-935`, `:537-563`).
- *Recover by substitution:* transport retries below the agent (`pydantic_ai_slim/pydantic_ai/retries.py:195-236`) and model fallback across providers (`pydantic_ai_slim/pydantic_ai/models/fallback.py:282-294`) happen without model-visible history changes.

**2. Are escalation thresholds configurable?**
Yes, at seven layers. Retry budgets resolve through a documented precedence ladder — per-tool → per-toolset → override block → per-run arg → per-run spec → agent-wide default → built-in default of `1` (`docs/tools-advanced.md:562-576`; `pydantic_ai_slim/pydantic_ai/agent/abstract.py:109-129`; default in `pydantic_ai_slim/pydantic_ai/tool_manager.py:167`). Usage/cost/request/tool-call ceilings are all fields on `UsageLimits` (`pydantic_ai_slim/pydantic_ai/usage.py:427-459`). Fallback triggers accept exception types and sync/async handlers, including response-quality handlers that treat a *successful* response as grounds for failover (`pydantic_ai_slim/pydantic_ai/models/fallback.py:104-133`, `tests/models/test_fallback.py:1413-1586`). Which tools need human approval is configurable per call via `approval_required_func` (`pydantic_ai_slim/pydantic_ai/toolsets/approval_required.py:22-24`) or dynamic raises inside tools. MCP servers choose among three error behaviors (`pydantic_ai_slim/pydantic_ai/mcp.py:875`). Transport retry behavior is fully user-supplied via `RetryConfig` (`pydantic_ai_slim/pydantic_ai/retries.py:72-134`), and per-activity Temporal `RetryPolicy`s are user-overridable (`pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_durability.py:221-280`).

**3. Can the system stop gracefully?**
Yes, in three graded ways. (a) First-party cancellation: `AgentRun.cancel()` (or `RunContext.cancel()` from inside a tool — `docs/timeouts.md:47`) stops the run, drains in-flight tool tasks, best-effort cancels suspended provider jobs, and yields `RunCancelled` carrying a detached snapshot of everything completed, ready to pass as `message_history` to a resume run (`pydantic_ai_slim/pydantic_ai/run.py:555-584`; `pydantic_ai_slim/pydantic_ai/exceptions.py:279-286`, `:364-373`). Cancellation is deliberately unrecoverable by hooks (`run.py:568-569`). External `asyncio` cancellation keeps its own semantics while attaching recoverable state discoverable via `RunCancelled.from_cancellation` (`pydantic_ai_slim/pydantic_ai/exceptions.py:321-362`). (b) Pause-for-human: the deferred-tools flow ends the run cleanly with `DeferredToolRequests`, and the conversation resumes in a new run keyed by `conversation_id` (`docs/deferred-tools.md:14-21`). (c) Mid-turn suspension: provider-side continuation (`state='suspended'`, e.g. Anthropic `pause_turn`, OpenAI background mode) checkpoints between segments, with usage re-checked provisionally per segment (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:875-893`).

**4. Are recovery decisions auditable?**
Substantially, via the transcript-as-audit-log design: every retry is persisted as a `RetryPromptPart` with timestamp, tool name, and tool call id (`pydantic_ai_slim/pydantic_ai/messages.py:1636-1674`), and stays in history so later runs replay prior failures to the model unless filtered (`docs/retries.md:95-97`). Failures/denials/interruptions persist as `ToolReturnPart.outcome ∈ {'success','failed','denied','interrupted'}` with explicit semantics distinguishing deliberate denials from transient errors (`pydantic_ai_slim/pydantic_ai/messages.py:1335-1351`). Consumers additionally get a live event stream (`FunctionToolCallEvent` with `args_valid`, `FunctionToolResultEvent`, `DeferredToolRequestsEvent`, `DeferredToolResultsEvent` — `pydantic_ai_slim/pydantic_ai/messages.py:3977-4086`) and OTel spans that serialize retry prompts (`messages.py:1723-1740`) and record which fallback model actually served (`pydantic_ai_slim/pydantic_ai/models/fallback.py:477-489`). Gap: there is no separate append-only recovery-audit store; auditing beyond the current process requires the application to persist history/events/spans itself.

## Architectural Decisions

- **Exceptions as typed control flow.** Recovery (`ModelRetry`), terminal reporting (`ToolFailed`), human escalation (`ApprovalRequired`/`CallDeferred`), and interception skips (`SkipToolExecution`, etc.) are distinct exception classes, and the tool-execution pipeline routes them differently (`pydantic_ai_slim/pydantic_ai/tool_manager.py:466-490` treats control-flow signals as neither errors nor retries). This makes the retry-vs-escalate decision local and inspectable at the raise site.
- **Budget separation.** Tool retries are per-tool-name counters that reset on success; output retries share a separate run-scoped counter; neither is shared with transport retries or fallback attempts (`docs/retries.md:33-39`; `pydantic_ai_slim/pydantic_ai/_agent_graph.py:361-378`). A hallucinated tool name gets its own budget keyed under the invented name, preventing one bad name from exhausting another tool's allowance (implemented at `pydantic_ai_slim/pydantic_ai/tool_manager.py:503-514`; behavior documented at `docs/retries.md:37`).
- **Retry-wins invariant.** Under `graceful`/`exhaustive` end strategies, a function-tool `RetryPromptPart` suppresses an otherwise-valid final output so the model addresses the correction first; the winning output's status part is rewritten in place (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:881-908`).
- **Human escalation as pause/resume, not blocking.** Instead of holding the process open awaiting approval, the run terminates with a structured request object; resolution happens in a subsequent run fed `DeferredToolResults` matched by `tool_call_id` (`pydantic_ai_slim/pydantic_ai/_deferred.py:44-96`; resume validation at `pydantic_ai_slim/pydantic_ai/_agent_graph.py:660-705`).
- **Cancellation is terminal by contract.** Hooks may observe cancellation for cleanup but cannot convert it into success (`pydantic_ai_slim/pydantic_ai/run.py:568-569`; `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:555-556`), avoiding the classic "zombie recovery" failure mode.
- **Deterministic failures are non-retryable under durable execution.** The Temporal integration injects `UserError`, `UnexpectedModelBehavior`, `FallbackExceptionGroup`, and payload-size errors into `non_retryable_error_types` so engine-level retries don't mask permanent failures (`pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_toolset.py:156-170`).

## Notable Patterns

- **Layered budgets with a precedence ladder** (`docs/tools-advanced.md:564-576`) mirrored by `AgentRetries` dict-vs-int ergonomics (`pydantic_ai_slim/pydantic_ai/agent/abstract.py:109-129`) — the same shape at every call site.
- **Wrapper-toolset composition for escalation policy**: `ApprovalRequiredToolset` adds approval gating to any inner toolset without modifying it (`pydantic_ai_slim/pydantic_ai/toolsets/approval_required.py:16-32`), matching the repo rule that cross-cutting behavior lives in wrappers.
- **Capability hooks as recovery seams**: `on_tool_execute_error` can return a replacement result, raise `ModelRetry`, or re-raise; `on_run_error` can return a synthetic `AgentRunResult` to suppress a failed run (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:905-935`, `:537-563`). This is the sanctioned way to add automatic recovery beyond built-ins.
- **Guided self-correction prompts**: unknown-tool retries enumerate available tools (`pydantic_ai_slim/pydantic_ai/tool_manager.py:507-513`); non-actionable-response retries enumerate valid output alternatives built from the actual output schema (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2026-2060`); unavailable deferred-capability tools say "*not available yet*" and point at search/load rather than implying nonexistence (`tool_manager.py:519-539`).
- **Cost accounting survives rejection**: responses rejected by fallback handlers have their cost accumulated onto the eventual winner so spend stays observable (`pydantic_ai_slim/pydantic_ai/models/fallback.py:296-307`).
- **Docs-as-taxonomy discipline**: `docs/retries.md:127-132` documents what is *never* retried (prepare callbacks, `before_model_request`, non-signal exceptions, whole runs) — closing the gap between intuition and implementation.

## Tradeoffs

- **Transcript-based audit vs dedicated audit log.** Persisting decisions in message history keeps one source of truth and round-trips through every provider adapter, but there is no queryable audit store; high-volume deployments must export events/spans themselves.
- **Pause/resume escalation vs synchronous approval.** Ending the run scales across processes/UIs and composes with durable engines, but callers must manage `message_history` + results correlation themselves (duplicate-ID and already-executed checks guard this — `pydantic_ai_slim/pydantic_ai/_agent_graph.py:687-697`).
- **Local exception-driven policy vs central policy engine.** Any tool author decides retry-vs-terminal by choosing `ModelRetry` vs `ToolFailed`; flexible, but correctness depends on developer judgment, and repeated `ToolFailed`s are bounded only by run-level `UsageLimits` (`pydantic_ai_slim/pydantic_ai/exceptions.py:110-111`).
- **Per-tool-name reset-on-success counting** prevents false exhaustion for alternating patterns but means a flaky tool can fail unboundedly many times within one run if it succeeds intermittently — mitigated by `request_limit=50` default (`pydantic_ai_slim/pydantic_ai/usage.py:429`) rather than a tool-wide cap (`docs/retries.md:35`).
- **Timeout ambiguity**: a user-raised `TimeoutError` inside a timed tool is indistinguishable from deadline expiry and both become the same retry prompt, potentially reporting a deadline that never passed (`docs/timeouts.md:29-30`).

## Failure Modes / Edge Cases

- **Budget exhaustion is terminal, not degrading:** exceeding tool retries raises `UnexpectedModelBehavior` with remediation text (`pydantic_ai_slim/pydantic_ai/tool_manager.py:262-264`); exceeding output retries raises after checking for a truncated tool call to give a better diagnostic (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:374-378`, `check_incomplete_tool_call` at `:345-359`).
- **All-models-failed aggregation:** `FallbackModel` raises `FallbackExceptionGroup` containing every underlying exception plus a `ResponseRejected` marker when handlers rejected otherwise-valid responses (`pydantic_ai_slim/pydantic_ai/models/fallback.py:544-554`, `ResponseRejected` at `:60-64`); non-fallback errors propagate immediately, recording the failing model on the span (`tests/models/test_fallback.py:699-731`).
- **Content filtering:** empty responses with `finish_reason='content_filter'` become `ContentFilterError` with the raw body attached for inspection; an opt-in `RaiseContentFilterError` capability generalizes this to any response (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1934-1947`; `pydantic_ai_slim/pydantic_ai/capabilities/content_filter.py:17-47`).
- **Token-limited thinking-only responses** are not retried — they hard-stop, because a retry would just hit the limit again (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1927-1931`); conversely, `None`-allowing output types treat empty responses as success to avoid pushing chatty models into filler (`:1949-1970`, `tests/test_agent.py:3808` covers the retry side).
- **Suspended-response misuse:** handing a suspended turn to `CallToolsNode` raises `UserError` instructing how to resume instead of silently treating partial parts as final output (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1898-1907`).
- **Stale suspended jobs:** resuming past the provider retention window raises `SuspendedResponseExpired` advising restart-from-history rather than an opaque HTTP error (`pydantic_ai_slim/pydantic_ai/exceptions.py:449-455`); abandoning a pinned continuation best-effort cancels the orphaned server-side job to avoid double billing (`pydantic_ai_slim/pydantic_ai/models/fallback.py:266-275`).
- **Duplicate escalation IDs:** deferred batches with duplicate `tool_call_id`s are rejected up front because resume matching would be ambiguous (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:967-973`, tested at `tests/test_agent.py:10965`).
- **Negative retry budgets** raise immediately instead of looping forever (`>=` comparison — `pydantic_ai_slim/pydantic_ai/tool_manager.py:259-261`).

## Future Considerations

- A pluggable sink for recovery/escalation decisions (beyond events + spans) would give operators a first-class audit trail without screen-scraping message history; today the pieces exist (`DeferredToolRequestsEvent`, `outcome` fields, OTel rendering) but assembly is left to users.
- Approval remains a per-tool/per-toolset concern; there is no declarative global policy (e.g. role-based "who may approve what") — reasonable for a library, but multi-tenant adopters build their own layer atop `HandleDeferredToolCalls`.
- Realtime sessions cannot pause for deferral and instead tell the model the tool can't complete mid-session (`docs/retries.md:131`); unifying that asymmetry with the pause/resume flow could simplify cross-modal reasoning.
- Timeout-origin ambiguity (user `TimeoutError` vs deadline expiry) inside timed tools could be resolved by distinct exception types.

## Questions / Gaps

- No evidence found of a persistent, framework-owned recovery audit log (searched `pydantic_ai_slim/pydantic_ai/` for audit/log stores around retry and deferral paths; only history parts, in-memory event streams, and OTel spans carry the record). If longer-term audit exists, it lives in consuming applications, outside this source.
- Escalation *notification* (email/chat/pager) is absent by design — searched for notification mechanisms around `ApprovalRequired`/`DeferredToolRequests` and found only the structured-output handoff; human contact is fully delegated to the caller.
- Whether fallback chains are intended to be tried more than once per run (circuit-breaker style) is not addressed: `FallbackModel` walks its chain once per request and never revisits a failed model within that request (`pydantic_ai_slim/pydantic_ai/models/fallback.py:282-318`); repeated failover across steps relies on re-entering the chain each step, which is observable but undocumented as a policy.

---

Generated by dimension `13.04-recovery-vs-escalation` against `pydantic-ai`.

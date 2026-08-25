# Source Analysis: pydantic-ai

## Dimension 07.08: Tool Failure Escalation

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (pydantic-core, anyio, OTel; agent graph on pydantic-graph) |
| Analyzed | 2026-08-25 |

All file paths below are workspace-relative and rooted at the selected source directory `studies/agent-harness-study/sources/pydantic-ai/`.

## Summary

pydantic-ai treats a tool failure as a *message*, not a dead end. Every failure is classified into one of two model-facing envelopes — a `RetryPromptPart` ("try again, here's why") or a failed `ToolReturnPart` (`outcome='failed'`, "this happened, adapt") — plus three control-flow outcomes (`CallDeferred`, `ApprovalRequired`, denial) that escalate to an external resolver rather than to the model. Failures are normalized inside `ToolManager` (`pydantic_ai_slim/pydantic_ai/tool_manager.py`), which owns the per-tool retry budget, converts raw exceptions into envelopes (`_wrap_error_as_retry` / `_wrap_error_as_failed`, tool_manager.py:267-282), and terminates the run with an actionable `UnexpectedModelBehavior` when the budget exhausts (tool_manager.py:256-265).

Escalation audiences are explicitly separated:

- **Model**: envelopes are appended as history parts and rendered per provider, using native error channels where they exist (Anthropic `is_error`, Bedrock `status='error'`) and JSON `{"error": ...}` framing elsewhere.
- **User/application**: typed exceptions propagate out of `agent.run()`; stream consumers see `FunctionToolResultEvent` carrying the exact part sent to the model; UI adapters translate `outcome='failed'|'denied'` into protocol-level `error_text`.
- **Operator**: the instrumentation capability opens OTel tool spans that record exceptions, set `ERROR` status, and — unlike failures — treat deferrals as non-error control flow recorded via span attributes.

Retry budgets are split across five documented layers (transport, fallback, tool, output, model-request hooks) that do not share counters (docs/retries.md:5-15). Retry exhaustion raises; terminal failure does not consume budget and relies on run-level `UsageLimits`. The only human escalation path is explicit opt-in approval (`requires_approval=True` / `ApprovalRequired`), which ends the run with a `DeferredToolRequests` output or resolves inline through a handler.

## Rating

**9 / 10.** A mature, layered failure-escalation model: a small typed exception surface (`exceptions.py:29-54`) maps onto exactly two wire envelopes; every conversion point is centralized in `ToolManager`/`_tool_execution`; provider wire framing for failures is pinned by dedicated cross-provider tests (`tests/test_tool_failed_wire.py`); retry-counter semantics, exhaustion messages, timeout handling, and unknown-tool guidance each have tests and docs. Operational safeguards go beyond the minimum: availability refusals get a free pass so one act of model disobedience isn't fatal (tool_manager.py:587-595), duplicate `tool_call_id`s fail closed before results can mis-bind (_tool_execution.py:405-420), and sub-agent self-cancellation is isolated into a recoverable failed return rather than tearing down the caller (_tool_execution.py:41-62). It falls short of 10 only because there is no first-class operator alerting beyond standard OTel spans, and one acknowledged budget-semantics inconsistency between output validators and output tools (tool_manager.py:855-862, tracking issue #5238).

## Evidence Collected

Every entry cites a file path with line numbers, relative to `studies/agent-harness-study/sources/pydantic-ai/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Failure taxonomy | `ModelRetry` (retryable correction request) vs `ToolFailed` (terminal failure the model sees without retry instructions) | `pydantic_ai_slim/pydantic_ai/exceptions.py:57-97`, `pydantic_ai_slim/pydantic_ai/exceptions.py:100-147` |
| Human escalation exceptions | `CallDeferred` and `ApprovalRequired` carry optional metadata keyed by `tool_call_id` | `pydantic_ai_slim/pydantic_ai/exceptions.py:150-183` |
| Envelope carriers | `ToolRetryError` wraps a `RetryPromptPart`; `ToolFailedError` wraps a failed `ToolReturnPart` | `pydantic_ai_slim/pydantic_ai/exceptions.py:616-648`, `pydantic_ai_slim/pydantic_ai/exceptions.py:651-661` |
| Outcome status enum | `outcome: Literal['success','failed','denied','interrupted']` with semantics doc; only `'failed'` maps to native provider error channels | `pydantic_ai_slim/pydantic_ai/messages.py:1335-1351` |
| Error envelope framing | Failed returns wrapped as `{"error": ...}` JSON for providers without a native error channel | `pydantic_ai_slim/pydantic_ai/messages.py:1450-1469` |
| Retry prompt construction | `RetryPromptPart.from_error` builds "the exact message the model receives"; content is error-details list (ValidationError) or string (ModelRetry) | `pydantic_ai_slim/pydantic_ai/messages.py:1637-1697` |
| Model-facing retry text | `model_response()` appends "Fix the errors and try again." | `pydantic_ai_slim/pydantic_ai/messages.py:1699-1721` |
| Retry budget check | `_check_max_retries` raises `UnexpectedModelBehavior` naming the tool, count, and docs URL | `pydantic_ai_slim/pydantic_ai/tool_manager.py:256-265` |
| Budget default & tracking | `default_max_retries=1`; `failed_tools` / `succeeded_tools` sets keyed by tool name | `pydantic_ai_slim/pydantic_ai/tool_manager.py:155-167` |
| Cross-step counter carry-over | `for_run_step` carries retries across steps, resets a tool's counter on success | `pydantic_ai_slim/pydantic_ai/tool_manager.py:187-220` |
| Unknown-tool escalation | `_resolve_tool` raises `ModelRetry('Unknown tool name…')` listing currently available tools | `pydantic_ai_slim/pydantic_ai/tool_manager.py:496-517` |
| Unavailable-tool guidance | Message says "not available yet: search/load capability first" instead of "unknown", so the model takes a recovery action | `pydantic_ai_slim/pydantic_ai/tool_manager.py:519-550` |
| Free first availability refusal | `_make_validation_failure` exempts the first `_ToolUnavailable` per tool from the retry budget | `pydantic_ai_slim/pydantic_ai/tool_manager.py:583-607` |
| Timeout → retry | Tool timeout converts to `ModelRetry('Timed out after {timeout} seconds.')` | `pydantic_ai_slim/pydantic_ai/toolsets/function.py:684-691` |
| Deferred-result dispatch | Handler-supplied `ToolFailed`/`ModelRetry`/`RetryPromptPart`/denial converted to the same envelopes as direct execution | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:668-702` |
| Denial envelope | `ToolDenied` becomes `ToolReturnPart(outcome='denied')` — policy outcome, not runtime error | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:76-86` |
| Retry-wins invariant | A function-tool retry suppresses an otherwise-valid final result so the model addresses the retry next round | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:881-908` |
| Output-retry exhaustion | Output-tool max retries wrapped into `UnexpectedModelBehavior('Exceeded maximum output retries (N)')`; absorbed as skip if another output won | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:483-527`, `pydantic_ai_slim/pydantic_ai/_tool_execution.py:570-578` |
| Output budget enforcement | `consume_output_retry` raises `UnexpectedModelBehavior` past `max_output_retries`; checks truncated tool calls first | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:361-378` |
| Truncated-call guard | `check_incomplete_tool_call` raises `IncompleteToolCall` when token limit cut a tool call mid-args | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:345-359` |
| Run-level bound | `UsageLimits.check_before_tool_call` projects batch size against `tool_calls_limit` before executing | `pydantic_ai_slim/pydantic_ai/usage.py:553-559`, `pydantic_ai_slim/pydantic_ai/_tool_execution.py:444-448` |
| Provider native error channel (Anthropic) | `is_error=request_part.outcome == 'failed'`; tool-bound `RetryPromptPart` rendered as `is_error=True` tool_result | `pydantic_ai_slim/pydantic_ai/models/anthropic.py:1857-1878` |
| Provider native error channel (Bedrock) | `success_result['status'] = 'error' if part.outcome == 'failed' else 'success'`; framed-text fallback when unsupported | `pydantic_ai_slim/pydantic_ai/models/bedrock.py:1209-1271` |
| Provider native error channel (Google) | `part.outcome == 'failed'` branch in function-response mapping | `pydantic_ai_slim/pydantic_ai/models/google.py:1197` |
| Wire-framing tests | Parametrized tests pin `is_error`/`status` mapping and `{"error": ...}` framing across providers, including no-double-wrap of legit `error` keys | `tests/test_tool_failed_wire.py:291-316`, `tests/test_tool_failed_wire.py:61`, `tests/test_tool_failed_wire.py:269-284` |
| User stream events | `FunctionToolResultEvent.part` carries the exact `ToolReturnPart | RetryPromptPart` sent to the model; call events expose `args_valid` | `pydantic_ai_slim/pydantic_ai/messages.py:3984-4021`, `pydantic_ai_slim/pydantic_ai/messages.py:3959-3966` |
| UI error surfacing | Vercel AI adapter maps `outcome == 'denied'/'failed'` to protocol `error_text` | `pydantic_ai_slim/pydantic_ai/ui/vercel_ai/_event_stream.py:374-378`, `pydantic_ai_slim/pydantic_ai/ui/vercel_ai/_event_stream.py:456-458` |
| Interrupted-run closeout | Pending tool calls closed with `outcome='interrupted'` (cancelled) or `'failed'` ("Tool execution was interrupted by an error.") before `on_error` | `pydantic_ai_slim/pydantic_ai/ui/_event_stream.py:330-369` |
| Operator spans | `_run_tool_span` records exceptions + `ERROR` status; deferrals recorded as span attributes (not errors) unless old instrumentation version; retry/failed prompts recorded as tool result attribute | `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:418-497` |
| Validation-failure span | `on_tool_validate_error` emits an escaped error span containing the same message the model will receive | `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:325-381` |
| Human-in-the-loop flow | Approval/denial ends run with `DeferredToolRequests` output; resume via `DeferredToolResults`; missing output type raises instructive `UserError` | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:964-1050`, `docs/deferred-tools.md:91-106` |
| Declarative approval | `requires_approval=True` read off `ToolDefinition.defer` so graph and single-call paths cannot drift | `pydantic_ai_slim/pydantic_ai/tool_manager.py:1099-1129` |
| Inline handler resolution | `_resolve_single_deferred` supports approve (+`override_args`), deny with message, handler-forced retry/failure | `pydantic_ai_slim/pydantic_ai/tool_manager.py:1148-1247` |
| MCP cause grouping | `tool_error_behavior` config ('retry'/'failed'/'error'); bare protocol `McpError` always stays retryable even under `'failed'`; ExceptionGroup split keeps concurrent cancellation unswallowed | `pydantic_ai_slim/pydantic_ai/mcp.py:764`, `pydantic_ai_slim/pydantic_ai/mcp.py:1407-1447` |
| Sub-agent isolation | Sub-agent self-cancellation surfaces as a failed tool return the caller's model can react to | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:41-62` |
| Exhaustion tests | Counter snapshots incl. `ctx.last_attempt`; ToolFailed never triggers exhaustion; timeout and validator exhaustion raise `UnexpectedModelBehavior` | `tests/test_tools.py:1510-1553`, `tests/test_tools.py:1615`, `tests/test_tools.py:3287-3305`, `tests/test_tools.py:3979-3995` |
| Layered-budget doc | Five retry layers table; what a retry looks like in history; what is never retried | `docs/retries.md:5-15`, `docs/retries.md:29-99`, `docs/retries.md:127-132` |

## Answers to Dimension Questions

1. **Who sees tool failure?** All three audiences, by design. The model sees either a `RetryPromptPart` (retryable) or a `ToolReturnPart(outcome='failed')` (terminal), both bound to the original `tool_call_id` (messages.py:1637-1697, messages.py:1335-1351). The application developer sees typed exceptions at the `agent.run()` boundary — `UnexpectedModelBehavior` for exhaustion, `UserError` for misuse — and, when streaming, `FunctionToolResultEvent`s carrying the identical part the model receives (messages.py:4012-4021). End users of UI adapters see protocol-level errors (`error_text`) derived from those parts (ui/vercel_ai/_event_stream.py:456-458). Operators see OTel tool spans with recorded exceptions and `ERROR` status (capabilities/instrumentation.py:444-489).
2. **Is the error actionable?** Yes, consistently. The max-retries message names the tool, the limit, and links remediation docs ("Consider raising the retry limit, or see the docs on tool retries: …") (tool_manager.py:262-265). Unknown-tool errors enumerate available tools (tool_manager.py:507-514); unavailable-tool errors name the recovery step (search vs `load_capability`) rather than saying "unknown" (tool_manager.py:539-550). `UsageLimitExceeded` appends a docs hint idempotently (exceptions.py:462-471).
3. **Can the model recover?** Yes — that is the primary path. Retry prompts render with "Fix the errors and try again." (messages.py:1721); the per-tool counter resets on success so transient failures don't accumulate across steps (tool_manager.py:193-204); and the retry-wins invariant guarantees a retried function tool is not silently overridden by a concurrent final result (_tool_execution.py:892-908). For terminal `ToolFailed` cases, recovery is deliberately handed to the model's judgment without spending retry budget (exceptions.py:100-112).
4. **When is failure escalated to a human?** Only by explicit opt-in: `requires_approval=True`, a raised `ApprovalRequired`, or external `CallDeferred` end the run with a `DeferredToolRequests` output (or resolve inline via a `HandleDeferredToolCalls` handler) (exceptions.py:150-183; _tool_execution.py:964-1050; docs/deferred-tools.md:91-106). Ordinary tool failures are never auto-escalated to humans — there is no built-in pager/webhook hook; operators must watch OTel signals.
5. **Are failures grouped by cause?** Yes, along several axes: the four-value `outcome` enum separates execution failure from policy denial and interruption (messages.py:1335-1351); the five-layer retry map keeps transport/fallback/tool/output/hook budgets separate (docs/retries.md:5-15); MCP distinguishes completed tool errors from protocol-level `McpError`s and even splits ExceptionGroups so protocol errors stay retryable under `'failed'` configuration (mcp.py:1411-1447); and availability refusals are tracked apart from real failures so a refusal can't starve the tool's failure budget (tool_manager.py:159-166, 587-595).

## Architectural Decisions

- **Errors-as-messages over errors-as-exceptions across the loop boundary.** Raw exceptions (`ValidationError`, `ModelRetry`, `ToolFailed`) exist only inside `ToolManager`; everything crossing to the model request builder has been normalized into a `RetryPromptPart` or `ToolReturnPart`. The internal exception types `ToolRetryError`/`ToolFailedError` are pure carriers between validate/execute and the graph (_tool_execution.py:697-702; tool_manager.py:267-282).
- **Two model-visible failure channels with different budget semantics.** `ModelRetry` = correction request, consumes the per-tool budget, prepends retry instructions; `ToolFailed` = terminal report, no budget consumption, bounding left to `UsageLimits` (exceptions.py:100-112; docs/tools-advanced.md:597-599). This gives developers a first-class way to say "don't retry this."
- **One `outcome` field as the provider-abstraction seam.** Each adapter decides how `'failed'` renders natively (Anthropic `is_error`, Bedrock `status`, Google branch) while `'denied'`/`'interrupted'` deliberately stay on the success channel because they aren't transient tool errors (messages.py:1347-1350).
- **Budget separation and precedence.** Tool and output budgets are distinct (`retries={'tools': N, 'output': N}`) with a documented six-step precedence ladder (per-tool → per-toolset → override → per-run → spec → agent-wide) (docs/tools-advanced.md:562-573).
- **Control-flow ≠ failure.** `CallDeferred`/`ApprovalRequired` bypass retry accounting entirely, are reported `args_valid=True` when raised after validation (tool_manager.py:85-94), and instrumentation records them as attributes rather than span errors (capabilities/instrumentation.py:452-471).

## Notable Patterns

- **Envelope normalization funnels.** Both direct execution (`_call_tool`) and deferred-handler results dispatch through the same branch set converting `ToolDenied`/`ToolFailed`/`ModelRetry`/`RetryPromptPart` into history parts, with an explicit sync-note requiring the two paths stay aligned (_tool_execution.py:674-702; tool_manager.py:1163-1165).
- **Free-first-refusal budgeting.** The first time a model calls a not-yet-available tool, the correction prompt doesn't charge the tool's budget — charging it would make one act of disobedience fatal on the default budget of 1; subsequent refusals charge normally (tool_manager.py:587-595).
- **Fail-closed ID matching.** Duplicate `tool_call_id`s in resume results raise `UserError` before any result can bind ambiguously (_tool_execution.py:405-420, 966-973).
- **Raw-error escape hatch for nested callers.** `wrap_validation_errors=False` lets sandboxed/nested dispatch get raw exceptions while leaving retry-budget state untouched — preventing double-counting (tool_manager.py:640-647, 987-991).
- **Timeouts as retries.** Per-tool timeouts convert to `ModelRetry` so they ride the existing budget/instructions machinery instead of a parallel failure path (toolsets/function.py:686-691).
- **Raise-to-propagate, return-to-recover hooks.** Capability error hooks can convert third-party exceptions into `ModelRetry`/`ToolFailed` centrally instead of try/except in every tool (`pydantic_ai_slim/pydantic_ai/tool_manager.py:462-471`; docs/hooks.md:398-430).

## Tradeoffs

- **Hallucinated tool names get fresh budgets.** An invented name consumes a budget keyed under that name, so a model hallucinating a *different* name each round never exhausts the tool budget; only run-level limits bound it. Documented as intended behavior (docs/retries.md:37) but it shifts the safety burden to `request_limit`.
- **`ToolFailed` loops are unbounded by default.** Since terminal failures don't consume the retry budget, a tool that always fails requires the developer to configure `UsageLimits` (docs/retries.md:99; docs/tools-advanced.md:597).
- **JSON error framing is a heuristic.** Providers without native error channels receive `{"error": ...}`-wrapped content; tests pin that legitimate outputs containing an `error` key are never mistaken for prior wrapping (tests/test_tool_failed_wire.py:269-284), but the extra nesting costs the model a level of indirection.
- **Retry-wins can revoke work under graceful/exhaustive strategies.** Suppressing a valid output because a sibling function tool retried avoids lost corrections but means already-executed side effects may be discarded from consideration; streaming commits are exempted since they can't be revoked (_tool_execution.py:358-365, 892-908).
- **Output-vs-tool budget mismatch acknowledged.** When `ToolOutput(max_retries=N)` exceeds the global output budget, validator `ctx.last_attempt` can fire before actual termination — tracked in issue #5238 rather than fixed ad hoc (tool_manager.py:855-862).

## Failure Modes / Edge Cases

- **Truncated tool calls**: a response hitting the token limit mid-args raises `IncompleteToolCall` with the effective `max_tokens` in the message rather than a confusing validation spiral (_agent_graph.py:345-359).
- **Sub-agent self-cancellation**: isolated into a failed return ("The sub-agent run was cancelled: …") so the caller's model can react instead of the whole run/session tearing down (_tool_execution.py:41-62).
- **External vs first-party cancellation**: external `CancelledError` never becomes `RunCancelled`; partial results survive in `RunCancelled.all_messages()` for resumable replay with unfinished calls synthesized as `outcome='interrupted'` (exceptions.py:268-290).
- **MCP ExceptionGroups**: a tool error racing session teardown surfaces wrapped in a group; only pure tool/protocol groups are unwrapped — anything grouped with cancellation re-raises unchanged (mcp.py:1420-1447).
- **Absorbed output failures**: when multiple output tools race, one loser's max-retries error is downgraded to a skip-status part (`_OUTPUT_EXECUTION_FAILED` / `_OUTPUT_VALIDATION_FAILED`) and only raised if nothing won (_tool_execution.py:570-578, 1224-1231).
- **Stream interruption closeout**: pending UI tool calls are closed as failed/interrupted before the error event, so clients never show them as still running (ui/_event_stream.py:318-358).

## Future Considerations

- Unify tool-path and validator-path output retry accounting once #5238's semantics question is settled (tool_manager.py:855-862).
- Consider structured machine-readable error codes alongside the `{"error": ...}` text envelope for programmatic model-side branching.
- A first-class alerting hook (e.g., an `on_tool_failure` operator callback) would complement the OTel-only observability story for teams not yet running a tracing backend.
- Bound repeated unknown-tool-name hallucinations more tightly than whole-run request limits (docs/retries.md:37).

## Questions / Gaps

- No evidence found of aggregate failure-rate metrics or alert rules shipped in-repo; operator escalation relies entirely on standard OTel spans/logs (searched `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py` and `docs/logfire.md`; only span/status recording exists).
- No evidence found of per-attempt backoff for *tool* retries (unlike HTTP transport retries via tenacity, `retries.py`): tool retries re-prompt the model immediately within the same run; whether pacing matters is left to the model/provider.
- The realtime path shares the envelope machinery (`_unsettled_call_return`, _tool_execution.py:53-55), but I did not audit `realtime/_session.py` line-by-line; claims about realtime escalation are limited to the shared helper.

---

Generated by dimension `07.08-tool-failure-escalation` against `pydantic-ai`.

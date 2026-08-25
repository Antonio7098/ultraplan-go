# Source Analysis: agent-framework

## 07.08 Tool Failure Escalation

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework (Microsoft Agent Framework) |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (`python/packages/core`), .NET (`dotnet/src/Microsoft.Agents.AI`); `go/` is a README-only placeholder |
| Analyzed | 2026-08-25 |

## Summary

Microsoft Agent Framework treats a failed tool primarily as a **recovery path for the model, not a dead end**. In the Python function-invocation loop, every ordinary exception raised by a tool or function middleware is absorbed into a synthetic terminal `function_result` content carrying the text `"Error: Function failed."` plus an `exception` metadata field, and the loop continues so the model can react (`python/packages/core/agent_framework/_tools.py:1412-1434`, `1573-1574`). Detail disclosure to the model is a deliberate, config-gated decision: `include_detailed_errors=False` by default keeps the envelope generic and logs the full exception operator-side with a hint to enable detail (`python/packages/core/agent_framework/_tools.py:1420-1428`).

Failure escalation is layered by audience. The **model** sees error envelopes and can retry with different arguments; a consecutive-error circuit breaker (`max_consecutive_errors_per_request`, default 3) stops the loop after repeated failing batches by forcing `tool_choice="none"` and one final no-tool model call (`python/packages/core/agent_framework/_tools.py:96`, `2718-2733`, `3299-3300`). The **caller/human** is escalated via control-flow exceptions that are explicitly *not* converted to results — `MiddlewareTermination` (graceful stop), `MiddlewareFailure` (fail-closed abort that cancels in-flight sibling calls), and `UserInputRequiredException` (sub-agent input requests surfaced to the parent response) (`python/packages/core/agent_framework/_middleware.py:85-118`; `python/packages/core/agent_framework/_tools.py:1569-1572`, `1706-1722`). Approval-required tools pause before execution and surface `function_approval_request` contents to the host (`python/packages/core/agent_framework/_tools.py:1796-1832`). The **client/user** sees failures through hosting surfaces: Foundry hosting converts MCP consent errors into `oauth_consent_request` stream events and everything else into a terminal `response.failed` SSE event (`python/packages/foundry_hosting/agent_framework_foundry_hosting/_responses.py:508-539`), while DevUI emits raw error events (`python/packages/devui/agent_framework_devui/_executor.py:324-326`). The **operator** gets Python logging per failure plus OpenTelemetry spans marked with `error.type` attributes and ERROR status (`python/packages/core/agent_framework/observability.py:2787-2791`, `2361-2374`).

The whole contract is codified in a normative specification with an explicit scenario-to-test mapping (`docs/specs/004-python-function-calling-loop.md:317-343`, `516-560`), which is unusually strong documentation discipline. The .NET stack delegates its tool loop to the external `Microsoft.Extensions.AI` `FunctionInvokingChatClient` (declared as an external dependency in `dotnet/AGENTS.md`) and layers approval/bypass decorators around it, so .NET failure semantics are largely inherited rather than re-implemented.

## Rating

**Score: 8 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale: failure taxonomy is typed (`python/packages/core/agent_framework/exceptions.py:169-212`), model-facing envelopes are pinned by tests in both generic and detailed modes (`python/packages/core/tests/core/test_function_invocation_logic.py:3274-3408`), retry exhaustion is bounded and tested (`test_function_invocation_logic.py:2126-2192`), fail-closed vs fail-open semantics are explicit and spec-mapped (`_middleware.py:85-137`; `docs/specs/004-python-function-calling-loop.md:523-529`), and observability hooks mark spans per OTel semconv. It falls short of 9–10 because ordinary tools have no configurable retry/backoff policy (only MCP has a fixed single-reconnect policy), there are no alerting/webhook hooks for operators beyond logging + OTel export, DevUI leaks full exception strings to the UI contrary to the core's redaction posture, and .NET behavior is outsourced to an external library not visible in this source.

## Evidence Collected

Every entry includes a file path with line numbers relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Typed failure taxonomy | `ToolException` / `ToolExecutionException` / `UserInputRequiredException`; base class auto-logs at debug with inner exception | `python/packages/core/agent_framework/exceptions.py:172-212`, `21-38` |
| Error envelope (model-facing) | `_function_execution_error_result` builds `Content.from_function_result(result="Error: Function failed.", exception=str(exc))`; warning log hints at `include_detailed_errors` | `python/packages/core/agent_framework/_tools.py:1412-1434` |
| Envelope carrier | `Content.from_function_result` accepts `exception` keyword stored on the result content; `had_errors` detects failures via `content.exception is not None` | `python/packages/core/agent_framework/_types.py:855-911`; `python/packages/core/agent_framework/_tools.py:1894-1902` |
| Unknown tool call | Default returns `'Error: Requested function "X" not found.'` result; `terminate_on_unknown_calls=True` raises `KeyError` before execution | `python/packages/core/agent_framework/_tools.py:1484-1491`, `1794-1795` |
| Argument validation failure | `"Error: Argument parsing failed."` result returned without invoking the tool body | `python/packages/core/agent_framework/_tools.py:1534-1543` |
| Config knobs | `FunctionInvocationConfiguration`: `max_iterations`, `max_function_calls`, `max_consecutive_errors_per_request`, `terminate_on_unknown_calls`, `include_detailed_errors`; defaults normalized with validation | `python/packages/core/agent_framework/_tools.py:1332-1409` |
| Fail-open default, fail-closed escape | Ordinary exceptions → error result, loop continues; `MiddlewareFailure` re-raised, batch cancelled, siblings awaited; `MiddlewareTermination`/`UserInputRequiredException` re-raised | `python/packages/core/agent_framework/_tools.py:1549-1574`, `1606-1641`, `1863-1876` |
| `MiddlewareFailure` contract | Docstring: never converted to a tool result; cooperative cancellation caveat for sync tools in worker threads; service-conversation settlement of dangling calls | `python/packages/core/agent_framework/_middleware.py:85-137` |
| Consecutive-error circuit breaker | Counter increments per failing batch, resets on success; limit logs warning; loop sets `tool_choice="none"` and makes one final no-tool call | `python/packages/core/agent_framework/_tools.py:2718-2733`, `2860-2870`, `3299-3300`, `3307-3313` |
| Streaming parity | Streaming loop applies identical stop-on-error-limit handling | `python/packages/core/agent_framework/_tools.py:3393-3396`, `3447-3456` |
| Iteration/function-call budgets | `DEFAULT_MAX_ITERATIONS=40`, `DEFAULT_MAX_CONSECUTIVE_ERRORS_PER_REQUEST=3`; fallback text `"Function invocation limit reached..."` when no final answer produced | `python/packages/core/agent_framework/_tools.py:95-104`, `2013-2036` |
| MCP transport retry & exhaustion | `_call_tool_with_retries`: 2 attempts, reconnect once on `ClosedResourceError`/"session terminated"; wraps all failures in `ToolExecutionException` carrying server error message; final exhaustion raises `"Failed to call tool 'X' after retries."` | `python/packages/core/agent_framework/_mcp.py:2096-2160` |
| MCP isError handling | `result.isError` parsed to text, span marked `error.type="tool_error"` per OTel semconv, then raised as `ToolExecutionException` | `python/packages/core/agent_framework/_mcp.py:2111-2121` |
| Long-running task abandonment | `_MCPTaskAbandoned(ToolExecutionException)` marker; max-wait expiry raises and fires best-effort `tasks/cancel`; no resubmit before `task_id` known (prevents double side effects) | `python/packages/core/agent_framework/_mcp.py:330-343`, `372-375`, `2243-2256` |
| HITL escalation (approvals) | Approval-required tools return `function_approval_request` contents; pending requests snapshotted in session state | `python/packages/core/agent_framework/_tools.py:1775-1832`, `2109-2140` |
| HITL escalation (sub-agents) | Sub-agent user-input requests raise `UserInputRequiredException(contents=...)`; invocation layer propagates them bound to the parent `call_id` instead of a generic error | `python/packages/core/agent_framework/_agents.py:710-712`; `python/packages/core/agent_framework/_tools.py:1706-1722` |
| Client-facing failure events | Foundry hosting: consent errors → `OAuthConsentRequestOutputItem` stream events + `emit_incomplete`; other exceptions → terminal `response.failed` via `_emit_failure`; lazy agent lifecycle lets OAuth retry succeed later | `python/packages/foundry_hosting/agent_framework_foundry_hosting/_responses.py:260-320`, `448-479`, `508-567` |
| DevUI user surface | Executor yields `{"type": "error", "message": str(e)}` and `AgentFailedEvent(error=e)` with full exception text | `python/packages/devui/agent_framework_devui/_executor.py:324-326`, `415-422` |
| Operator logs | Per-failure WARNING with tool name; limit-exceeded WARNING; MCP reconnect INFO / exhaustion ERROR; MCP server log levels mapped to Python levels ("alert"→CRITICAL) | `python/packages/core/agent_framework/_tools.py:1420-1425`, `2729-2732`; `python/packages/core/agent_framework/_mcp.py:2138`, `2149`, `148-157` |
| OTel span errors | `capture_exception` sets `error.type`, records exception, ERROR status; `set_mcp_span_error` for MCP spans | `python/packages/core/agent_framework/observability.py:2787-2791`, `2361-2374` |
| Hook boundary redaction | On tool exception, `post_tool_call` hook emitted with `is_error=True` and only the exception *type name* crosses the boundary (spec §6.3/§14) | `python/packages/core/agent_framework/_agent_hooks.py:1448-1465`; test `python/packages/core/tests/core/test_agent_hooks.py:661-690` |
| Normative spec + test mapping | Contract: one terminal error result per failing call; `MiddlewareFailure` fail-closed; consecutive-error cap; unknown-call modes; settlement on service-managed conversations — each mapped to named tests | `docs/specs/004-python-function-calling-loop.md:317-343`, `380-387`, `516-560` |
| Tests: envelope modes | Generic vs detailed error assertions pin exact model-visible strings | `python/packages/core/tests/core/test_function_invocation_logic.py:3274-3340`, `3342-3408` |
| Tests: retry exhaustion & unknown calls | `test_function_invocation_config_max_consecutive_errors` (stops after limit), `terminate_on_unknown_calls_false/true` | `python/packages/core/tests/core/test_function_invocation_logic.py:2126-2192`, `2225+` |
| Tests: fail-closed middleware | `TestMiddlewareFailure`: aborts run, cause chain preserved, ordinary exceptions still become tool errors | `python/packages/core/tests/core/test_middleware_with_agent.py:2506` (see mapping in spec) |
| MCP error propagation tests | `isError=True` raises `ToolExecutionException`; middleware observes it | `python/packages/core/tests/core/test_mcp.py:1385-1420`, `1465-1473` |
| .NET delegation | Tool loop owned by external M.E.AI `FunctionInvokingChatClient`; framework decorators positioned "between/above" it for approvals/bypass | `dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientBuilderExtensions.cs:94-237`; `dotnet/AGENTS.md` (External Dependencies) |
| .NET repo-owned envelope | Skills script executor returns `"Error: ... Exception: {ex.Message}"` only when `IncludeDetailedErrors=true`, otherwise rethrows | `dotnet/src/Microsoft.Agents.AI/Skills/AgentSkillsProvider.cs:446-456` |

## Answers to Dimension Questions

1. **Who sees tool failure?** All four audiences, in layers: the *model* always receives the error `function_result` in the same turn (`python/packages/core/agent_framework/_tools.py:1429-1434`); the *operator* gets a WARNING log and OTel span status per failure (`_tools.py:1420-1425`; `observability.py:2787-2791`); the *user/caller* sees failures only when they are control-plane matters — approval requests (`_tools.py:1800-1832`), sub-agent input requests (`_tools.py:1706-1715`), or hosting-level conversions to consent/failed stream events (`foundry_hosting/..._responses.py:513-539`). DevUI surfaces raw error strings to the developer UI (`devui/..._executor.py:324-326`).
2. **Is the error actionable?** Deliberately split. The default model-facing message `"Error: Function failed."` is intentionally non-actionable (detail suppressed to avoid leaking internals to the model); actionable detail is opt-in via `include_detailed_errors=True` and always present in operator logs (`_tools.py:1426-1428`, test assertion `test_function_invocation_logic.py:3338-3339`). MCP errors forward the server's own message text, which is usually actionable (`_mcp.py:2131-2134`). Hook consumers get only the exception class name (`_agent_hooks.py:1450-1454`).
3. **Can the model recover?** Yes — this is the design center. Error results feed back into the conversation so the model can adjust arguments or try another tool; recovery attempts are bounded by `max_consecutive_errors_per_request` (default 3 consecutive failing iterations), after which the loop forces `tool_choice="none"` and asks the model for a final answer without tools (`_tools.py:96`, `2718-2733`, `3299-3300`, tested at `test_function_invocation_logic.py:2126-2192`).
4. **When is failure escalated to a human?** (a) Before execution for approval-required tools (`function_approval_request` pause/resume, `_tools.py:1787-1832`); (b) when a wrapped sub-agent needs user input (`UserInputRequiredException` propagates request contents, `exceptions.py:184-209`); (c) on OAuth consent required for hosted MCP tools (consent link streamed to client, `foundry_hosting/..._responses.py:516-538`); (d) immediately and fatally when enforcement middleware fails (`MiddlewareFailure` propagates to the caller of `run()`, cancelling siblings, `_middleware.py:85-105`). There is no asynchronous human-notification channel (no email/pager/ticket integration).
5. **Are failures grouped by cause?** Yes, three ways: a typed exception hierarchy with documented per-failure-class mapping (`exceptions.py:15-263`; `python/CODING_STANDARD.md:270,295` maps "Tool execution failure → ToolExecutionException"); MCP-level cause classification distinguishing connection-lost vs session-terminated vs protocol `McpError` vs `isError` vs task-abandonment (`_mcp.py:2111-2159`, `335-343`); and OTel `error.type` attribute values (`"tool_error"` for isError results, exception class name otherwise) enabling cause-based aggregation in telemetry backends (`observability.py:2373`, `_mcp.py:2118-2120`). The core loop itself does not aggregate or bucket failures beyond the consecutive-error counter — grouping is left to log/trace analysis.

## Architectural Decisions

1. **Fail-open for tools, fail-closed for enforcement.** Ordinary tool exceptions become results and the loop continues (recovery-first), but `MiddlewareFailure` exists precisely because guardrails must not be absorbed — it is never converted to a tool result, cancels the parallel batch, and aborts the run (`_tools.py:1569-1574`; `_middleware.py:85-118`). This two-mode design is normatively specified (`docs/specs/004-python-function-calling-loop.md:323-339`).
2. **Information gating over information elimination.** The envelope is uniform (`function_result` with `exception` metadata); what varies by config is how much text reaches the model vs the log (`_tools.py:1412-1434`). The hook layer applies a second, stricter gate: type names only (`_agent_hooks.py:1448-1454`).
3. **Bounded loops instead of unbounded retries.** Rather than retrying failing tools, the framework bounds total work (`max_iterations`=40, `max_function_calls`, `max_consecutive_errors_per_request`=3) and degrades gracefully to a no-tool final answer, including a synthesized fallback assistant message when the budget expires without any visible content (`_tools.py:95-104`, `2013-2036`, `3307-3313`).
4. **Transport-aware retry only where safe.** MCP retries exactly once on connection loss, and long-running-task submission never re-issues the augmented call before a `task_id` is known, because a lost response after acceptance could double-execute a side-effecting tool; abandonment paths fire best-effort `tasks/cancel` (`_mcp.py:2108-2160`, `2243-2246`, doc `python/packages/core/AGENTS.md` §MCP submit-vs-track policy). Retry policy is correctness-driven, not blanket.
5. **Service-managed conversation settlement.** On a persisted/hosted conversation, a fatal abort first settles dangling calls by submitting one error `function_result` per call with `tool_choice="none"`, so the hosted thread is not left in a state the service would reject next turn (`_tools.py:2883-2933`, `3275-3290`; `_middleware.py:99-105`).
6. **.NET reuses the platform loop.** Function-invocation error semantics (including M.E.AI's own exception-to-result options) come from the external `FunctionInvokingChatClient`; the repo contributes orchestration around it (approval binding, bypass, persistence) rather than reimplementing the loop (`ChatClientBuilderExtensions.cs:94-237`).

## Notable Patterns

- **Error-as-content**: failures are first-class `Content` items (`exception` field on `function_result`, `python/packages/core/agent_framework/_types.py:855-911`), so they survive serialization, history replay, compaction, and streaming under the same rules as successful results.
- **Control-flow exception trio**: `MiddlewareTermination` (graceful, may substitute a result), `MiddlewareFailure` (fatal, produces no result), `UserInputRequiredException` (carries payload contents for HITL) — each documented with contrast to the others (`_middleware.py:112-118`; `exceptions.py:184-195`).
- **Occurrence-aware correlation**: reused `call_id`s are matched by ordered occurrence so an error result can never attach to the wrong logical call after retries/replays (`docs/specs/004-python-function-calling-loop.md:341`; `_tools.py:2503+`).
- **Spec-with-test-mapping documentation**: every error behavior row in the spec names the exact tests that pin it (`docs/specs/004-python-function-calling-loop.md:520-557`), keeping docs and tests synchronized.
- **Redaction boundaries differ by consumer**: model (config-gated), hooks (type name only), DevUI (full string) — implemented independently in each layer (`_tools.py:1426-1428`; `_agent_hooks.py:1450-1454`; `_executor.py:326`).

## Tradeoffs

- **Generic-by-default hides root causes from the model.** With `include_detailed_errors=False`, the model cannot self-correct from `"Error: Function failed."` alone (e.g., a typo'd argument name is invisible), potentially burning the 3-attempt error budget on blind retries; the safer default trades recovery speed against leakage risk (`_tools.py:1426-1428`).
- **Fail-open absorption can mask systemic breakage.** A persistently failing tool just yields error results until the circuit breaker trips; nothing distinguishes "model keeps choosing a broken tool" from "deployment incident" except log volume — there is no health signal surfaced programmatically (`_tools.py:2718-2733`).
- **Cooperative cancellation is incomplete.** A synchronous tool body running in a worker thread cannot be interrupted and may complete side effects after a fatal abort; its result is discarded but the effect happened (`_middleware.py:95-98`; `_tools.py:1865-1876`).
- **Cross-language inconsistency.** Python has the full taxonomy, circuit breaker, and settlement logic; .NET inherits whatever M.E.AI does, so identical tool code can escalate differently per language, and the .NET-side behavior cannot be verified from this repository (`dotnet/AGENTS.md` External Dependencies).
- **DevUI contradicts the redaction posture** by streaming `str(e)` to the browser UI (`_executor.py:324-326`) — acceptable for a dev tool, but it means the "type name only" discipline holds only outside developer surfaces.

## Failure Modes / Edge Cases

- **Error-budget burn by repetition**: the model repeating a failing call consumes consecutive-error slots; mixed batches count one slot per failing *batch* (`errors_in_a_row += 1` per batch, not per call — `_tools.py:2718-2733`), so a large failing batch counts as one step toward the cap.
- **Unknown-call ambiguity between local and hosted tools**: a name missing from the tool map is assumed to be a hosted tool in the approved path and passed through (`_tools.py:1502-1505`), while in the direct path it becomes a "not found" error — misconfigured `allowed_tools` filters can therefore produce confusing model-facing messages rather than loud configuration errors.
- **MCP unparseable success responses do not fall back** to plain call mode, deliberately raising `ToolExecutionException` to avoid double-executing a side-effecting tool (`python/packages/core/AGENTS.md` §Permissive fallback; `_MCPTaskAbandoned` design `_mcp.py:335-343`).
- **Settlement best-effort**: if the settlement request itself fails during a `MiddlewareFailure` abort on a hosted conversation, the abort still propagates and the thread may remain unsettled — the failure is logged, not masked (`_tools.py:3275-3290`; spec line 334).
- **Streaming partial visibility**: in streaming runs, error results are yielded as tool-role updates as they occur (`_tools.py:2841-2869`), so clients see failures mid-stream even though the loop may still recover afterward.

## Future Considerations

- Add a configurable retry policy (attempts/backoff/retryable-exception predicates) for ordinary tools, generalizing the fixed MCP single-reconnect pattern (`_mcp.py:2108-2146`).
- Expose an operator-facing failure aggregation or webhook hook (e.g., on circuit-breaker trip) instead of relying solely on log scraping; the consecutive-error warning at `_tools.py:2729-2732` is the natural trigger point.
- Align DevUI's error payloads with the core redaction discipline (type name + sanitized message) to keep developer surfaces from leaking secrets embedded in exception texts (`_executor.py:326`; contrast `_agent_hooks.py:1450-1454`).
- Document/verify .NET parity for the error-limit and envelope behaviors in-repo (e.g., conformance tests against the external `FunctionInvokingChatClient` defaults) since the framework advertises cross-language equivalence.
- Consider surfacing `error.type` classification on the `function_result` metadata itself (not only on OTel spans) so downstream hosts can group failures without a telemetry backend.

## Questions / Gaps

- No evidence found of operator *alerting* integrations (webhooks, metrics-based alerts, paging): searches for `on_error`/`alert` across `python/packages/core` found only the MCP log-level mapping and unrelated settings (`_mcp.py:155`). Operator notification appears fully delegated to logging/OTel consumers.
- No evidence found of per-tool retry configuration in Python (retryable vs non-retryable classification beyond the MCP transport special cases); if desired, it lives only in external M.E.AI for .NET, which is out of this source's tree.
- The .NET `FunctionInvokingChatClient`'s actual exception-handling options are not implemented in this source; the repo references M.E.AI's `IncludeDetailedErrors` policy by name when explaining its own skills-level equivalent (`dotnet/src/Microsoft.Agents.AI/Skills/AgentSkillsProviderOptions.cs:24,33`), but the loop's implementation is external and its behavior here is inferred from wiring comments (`ChatClientBuilderExtensions.cs:94-237`); it cannot be cited line-by-line.
- Whether the `exception` metadata field on error results reaches providers verbatim (vs only `result` text being sent) varies per provider adapter and was not exhaustively traced across all ~30 provider packages; evidence shown covers the core loop and OpenAI-family defaults.

---

Generated by `07.08-tool-failure-escalation` against `agent-framework`.

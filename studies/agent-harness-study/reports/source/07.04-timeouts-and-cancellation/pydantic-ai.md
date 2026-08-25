# Source Analysis: pydantic-ai

## 07.04: Timeouts and Cancellation

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python 3.10-3.13, asyncio + anyio, OpenTelemetry-instrumented |
| Analyzed | 2026-08-23 |

## Summary

Pydantic AI ships a deliberately layered model for timeouts and cancellation that is among the more mature in the agent-framework space. Tool-level and agent-level timeouts are first-class via `FunctionToolset.call_tool` (`pydantic_ai_slim/pydantic_ai/toolsets/function.py:679-693`), enforced with `anyio.fail_after(timeout)` and translated into a `ModelRetry` so the model can adapt. Hook timeouts use the same primitive (`pydantic_ai_slim/pydantic_ai/capabilities/hooks.py:239-252`). Per-request HTTP timeouts are layered on top (`pydantic_ai_slim/pydantic_ai/_http.py:24`, `pydantic_ai_slim/pydantic_ai/settings.py:209`), and MCP/realtime timeouts each have their own dedicated knobs (`pydantic_ai_slim/pydantic_ai/mcp.py:893-1043`, `docs/timeouts.md:14-16`).

Cancellation is the most developed part. A first-party cancellation controller (`pydantic_ai_slim/pydantic_ai/_cancel.py`) — `CancellationToken` + `RunCancellation` — distinguishes itself from external asyncio cancellation by issuing its own `Task.cancel()` and then consuming that issuance at the run's outer edge via `Task.uncancel()` (`pydantic_ai_slim/pydantic_ai/_cancel.py:203-242`). External `asyncio.CancelledError` is *never* swallowed; the run state is attached to the propagating exception for `RunCancelled.from_cancellation()` (`pydantic_ai_slim/pydantic_ai/exceptions.py:318-362`). Parallel tool tasks are explicitly drained with `cancel_and_drain` under an `anyio.CancelScope(shield=True)` to avoid re-delivery storms (`pydantic_ai_slim/pydantic_ai/_utils.py:312-329`, `pydantic_ai_slim/pydantic_ai/_tool_execution.py:828-836`). A run-level `release_issued()` finalizer prevents stale `Task.cancelling()` counts from contaminating outer scopes (`pydantic_ai_slim/pydantic_ai/_cancel.py:244-257`, `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1790-1796`). Streamed model responses have their own `cancel()`/`close_stream()` model with explicit `'interrupted'` state semantics (`pydantic_ai_slim/pydantic_ai/models/__init__.py:1082-1126`, `pydantic_ai_slim/pydantic_ai/messages.py:126-147`). The whole system has comprehensive regression coverage (`tests/test_run_cancellation.py`, 68 dedicated tests, 1879 lines; `tests/test_tools.py` timeout tests at lines 3076-3300).

The model is, however, *cooperative* rather than forced: synchronous `def` tools run on a worker thread that Python cannot terminate, so their worker thread is abandoned via `_utils.abandon_threads_on_cancel` (`pydantic_ai_slim/pydantic_ai/_utils.py:162-180`, `pydantic_ai_slim/pydantic_ai/toolsets/function.py:688`), and a tool body that catches `CancelledError` without `Task.uncancel()` can defeat the backstop (documented at `pydantic_ai_slim/pydantic_ai/_utils.py:332-368`). Python 3.10 lacks `Task.cancelling()`/`Task.uncancel()`, so the first-party/external distinction is documented as degraded on 3.10 (`pydantic_ai_slim/pydantic_ai/_cancel.py:18-20`).

## Rating

**9 / 10 — Mature, durable, observable, extensible, and proven under failure.**

Rationale against the rubric:

- **Clear model with tests, explicit interfaces, and operational safeguards** is achieved. The cancellation controller is a separate object with a documented state machine (`pydantic_ai_slim/pydantic_ai/_cancel.py:1-23`). There are dedicated tests for: token-from-sibling-task, pre-cancelled token, run queued behind concurrency limiter, post-end cancel, cancel outside a run, sub-agent self-cancel isolation, sub-agent cancel propagation, absorbed-cancellation handoff, partial-request capture, parallel-sibling cancellation, hook-timeout, tool-timeout retry counting, and Python 3.10 pinned degraded behavior (`tests/test_run_cancellation.py` and `tests/test_tools.py:3076-3300`). The tests load-bear the documented semantics.
- **Forced cancellation is not provided.** Sync worker threads and tool bodies that swallow `CancelledError` are explicit documented failure modes (`docs/agent.md:737-738`, `pydantic_ai_slim/pydantic_ai/_utils.py:332-368`). A synchronous tool's side effects are not rolled back even after the run is cancelled (`docs/agent.md:737-738`). Hitting "10" would require a true forced-stop with rollback, which Python cannot generally provide.
- **Provider-side model generation cancel is best-effort** (`docs/agent.md:737-738`). Each provider has its own `close_stream()` override (`pydantic_ai_slim/pydantic_ai/models/openai.py:3838-3839`, `pydantic_ai_slim/pydantic_ai/models/anthropic.py:3106`, `pydantic_ai_slim/pydantic_ai/models/google.py:1364`, etc.), but cancellation depends on what the SDK exposes.
- **A first-party cancellation is communicated through a shared `CancellationToken` only.** Sub-agent delegation needs to opt in (`docs/agent.md:843-847`, `pydantic_ai_slim/pydantic_ai/_tool_execution.py:41-62`). A user who forgets that gets isolated-to-tool-failure by default, which is correct but not always what they want — a UX gap, not a safety gap.
- **Tool-timeout boundaries are uneven.** `Agent(tool_timeout=...)` only reaches tools registered on the agent itself; externally constructed `FunctionToolset` instances need their own timeout (`docs/timeouts.md:25`). MCP and external toolsets don't read the timeout either (`docs/timeouts.md:25`).

Score reflects the rubric ceiling: the design is intentional, tested, and observable (Otel spans carry `gen_ai.response.finish_reasons`, `messages.py:2589-2590`, `pydantic_ai_slim/pydantic_ai/_instrumentation.py:418-435`), but the cooperative-cancellation ceiling and uneven toolset-level timeout propagation stop it short of "durable under arbitrary failure."

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Cancellation controller class | `RunCancellation` owns the run-driving task, tracks issued cancellations, resolves at outer edge | `pydantic_ai_slim/pydantic_ai/_cancel.py:92-242` |
| CancellationToken API | `cancel()`, idempotent, thread-safe, may govern multiple runs | `pydantic_ai_slim/pydantic_ai/_cancel.py:42-89` |
| First-party cancellation request | `RunCancellation.cancel()` schedules `_deliver()` on the run's loop via `call_soon_threadsafe` | `pydantic_ai_slim/pydantic_ai/_cancel.py:154-175` |
| Cancellation attribution | `resolve()` consumes its own `Task.uncancel()` count and only translates to `RunCancelled` when no external cancel remains | `pydantic_ai_slim/pydantic_ai/_cancel.py:203-242` |
| Finalizer to clean issued counts | `release_issued()` runs in `_translate_cancellation.finally` so `Task.cancelling()` does not contaminate outer scopes | `pydantic_ai_slim/pydantic_ai/_cancel.py:244-257`, `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1790-1796` |
| Absorbed-cancel backstop | `raise_if_cancelling()` re-raises if the task's `cancelling()` count is positive after a step | `pydantic_ai_slim/pydantic_ai/_utils.py:332-368` |
| Cancel-and-drain helper | `cancel_and_drain(tasks, msg=...)` cancels and shields the drain to prevent AnyIO level cancellation storms | `pydantic_ai_slim/pydantic_ai/_utils.py:312-329` |
| Parallel tool-task cancel | `_call_tools` cancels sibling tasks on `CancelledError` and any other exception | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:828-836` |
| Parallel tool-task cancel (graph) | Same pattern in the graph's tool execution path | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:1209-1214` |
| Sub-agent self-cancel isolation | `cancelled_sub_agent_return` converts a nested `RunCancelled` into a failed tool return | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:41-62`, `pydantic_ai_slim/pydantic_ai/_tool_execution.py:701-702` |
| Tool timeout (per-tool > toolset > None) | Precedence check + `anyio.fail_after(timeout)` + `_utils.abandon_threads_on_cancel()` | `pydantic_ai_slim/pydantic_ai/toolsets/function.py:684-693` |
| Timeout → ModelRetry mapping | A `TimeoutError` from `anyio.fail_after` becomes `ModelRetry(f'Timed out after {timeout} seconds.')` | `pydantic_ai_slim/pydantic_ai/toolsets/function.py:686-691` |
| Agent-level tool_timeout | Default timeout applied to `_AgentFunctionToolset`, validated >0 | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:610-612`, `pydantic_ai_slim/pydantic_ai/agent/__init__.py:661-680` |
| Hook timeout | `_call_entry` wraps hook function in `anyio.fail_after(entry.timeout)`, raises `HookTimeoutError` | `pydantic_ai_slim/pydantic_ai/capabilities/hooks.py:239-252`, `pydantic_ai_slim/pydantic_ai/capabilities/hooks.py:64-71` |
| HTTP request default timeout | `DEFAULT_HTTP_TIMEOUT = 600` with `connect=5` | `pydantic_ai_slim/pydantic_ai/_http.py:24-63` |
| ModelSettings.timeout field | Per-request override, accepts numeric or legacy `httpx.Timeout` | `pydantic_ai_slim/pydantic_ai/settings.py:209-222` |
| MCP per-tool/request timeout | `init_timeout`, `read_timeout` defaults 5 / 300 | `pydantic_ai_slim/pydantic_ai/mcp.py:893-1043` |
| SSRF client timeout | `_DEFAULT_TIMEOUT = 30` for SSRF-safe downloads | `pydantic_ai_slim/pydantic_ai/_ssrf.py:128,541-601` |
| AgentRun.cancel() | First-party cancellation: `self.ctx.deps.cancellation.cancel()` | `pydantic_ai_slim/pydantic_ai/run.py:555-584` |
| AgentRunEvents.cancel() | Pre-start cancel yields `RunCancelled` with empty history | `pydantic_ai_slim/pydantic_ai/agent/abstract.py:180-208` |
| RunContext.cancel() | First-party cancel from a tool/hook, raises `UserError` if not backed by a live run | `pydantic_ai_slim/pydantic_ai/_run_context.py:467-498` |
| RunCancelled exception | `AgentRunError` subclass with `all_messages()`, `new_messages()`, `from_cancellation(exc)` for external-cancel state recovery | `pydantic_ai_slim/pydantic_ai/exceptions.py:268-446` |
| External-cancel state attachment | `_RUN_CANCELLED_ATTR` attached to propagating `CancelledError` so `from_cancellation()` recovers state | `pydantic_ai_slim/pydantic_ai/exceptions.py:265,318-362` |
| Iter outer-edge translation | `_translate_cancellation` translates `CancelledError` to `RunCancelled` only when first-party and re-stamps nested cancellations | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1763-1796` |
| Token registration before concurrency limiter | Token attached before the concurrency context so a pre-cancelled token never blocks waiting for a slot | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1802-1815` |
| StreamedResponse.cancel() / close_stream() | Sets `_cancelled`, calls `close_stream()` (provider-specific override); transport errors from cancellation are suppressed via `get_stream_cancel_errors()` | `pydantic_ai_slim/pydantic_ai/models/__init__.py:1046-1126` |
| Cancel-guard for transport teardown errors | `iterator_with_cancel_guard` suppresses `httpx.StreamError`/`TransportError` when the stream was cancelled | `pydantic_ai_slim/pydantic_ai/models/__init__.py:1046-1080` |
| StreamedResponse state derivation | `get()` resolves `state` from `_finished` vs `_cancelled`; cancel wins → `'interrupted'` | `pydantic_ai_slim/pydantic_ai/models/__init__.py:1142-1170` |
| Provider close_stream overrides | OpenAI, Anthropic, Google, Mistral, Groq, Bedrock, HuggingFace, xAI, FunctionModel, ContinuationModel each override | `pydantic_ai_slim/pydantic_ai/models/openai.py:3838,4055,4115`, `pydantic_ai_slim/pydantic_ai/models/anthropic.py:3106`, `pydantic_ai_slim/pydantic_ai/models/google.py:1364`, `pydantic_ai_slim/pydantic_ai/models/mistral.py:780`, `pydantic_ai_slim/pydantic_ai/models/groq.py:710`, `pydantic_ai_slim/pydantic_ai/models/bedrock.py:1717`, `pydantic_ai_slim/pydantic_ai/models/huggingface.py:555`, `pydantic_ai_slim/pydantic_ai/models/xai.py:971`, `pydantic_ai_slim/pydantic_ai/models/function.py:394`, `pydantic_ai_slim/pydantic_ai/models/_continuation.py:555` |
| Suspended-job cancel | `Model.cancel_suspended_response` no-op default; `_continuation.py` cancels via `cancel_suspended_job` on error / over-limit / merge-failure paths | `pydantic_ai_slim/pydantic_ai/models/__init__.py:557-563`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:983-1039` |
| StreamedRunResult.cancel() | Stops current response only; run continues | `pydantic_ai_slim/pydantic_ai/result.py:158-167`, `pydantic_ai_slim/pydantic_ai/result.py:782-809` |
| Interrupted tool return | `outcome='interrupted'` synthesized via `INTERRUPTED_TOOL_RETURN_CONTENT` | `pydantic_ai_slim/pydantic_ai/messages.py:1292-1295,1335-1351`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2880-2887` |
| Durable-execution: token rejection | Same-process tokens rejected at a Temporal/Prefect/DBOS boundary with `UserError` | `pydantic_ai_slim/pydantic_ai/durable_exec/_runtime_toolsets.py:38-49`, `pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_agent.py`, `pydantic_ai_slim/pydantic_ai/durable_exec/prefect/_agent.py`, `pydantic_ai_slim/pydantic_ai/durable_exec/dbos/_agent.py` |
| Temporal: heartbeat timeout default | `_DEFAULT_MODEL_HEARTBEAT_TIMEOUT` for model-request activities | `pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_durability.py:97-263` |
| Temporal: activity shield | `_run_in_activity` shields our task from AnyIO level cancellation, forwards one cancel to Temporal graceful machinery | `pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_activity_execution.py:1-46` |
| Test: token cancels sibling task | `test_cancellation_token_from_sibling_task` | `tests/test_run_cancellation.py:165-181` |
| Test: pre-cancelled token prevents start | `test_pre_cancelled_token_does_not_start_run` | `tests/test_run_cancellation.py:184-198` |
| Test: cancel after completion is no-op | `test_cancel_after_completion_is_a_noop` | `tests/test_run_cancellation.py:1708-1721` |
| Test: cancel queued behind concurrency limiter | `test_token_cancels_run_queued_behind_concurrency_limiter` | `tests/test_run_cancellation.py:1741-1767` |
| Test: sub-agent self-cancel isolation | `test_sub_agent_self_cancel_is_isolated_as_tool_failure` | `tests/test_run_cancellation.py:1786-1807` |
| Test: sub-agent propagation opt-in | `test_sub_agent_cancel_can_be_propagated_by_delegate` | `tests/test_run_cancellation.py:1810-1828` |
| Test: external cancel keeps propagating | Multiple tests assert `asyncio.CancelledError` raises through and `from_cancellation` recovers state | `tests/test_run_cancellation.py` (search `CancelledError`) |
| Test: parallel-tool cancel drains siblings | `test_exhaustive_outer_cancellation_cancels_pending_tools`, `test_parallel_tool_exception_cancels_sibling_tasks`, `test_parallel_tool_outer_cancellation_only_cancels_pending_tool_tasks` | `tests/test_agent.py:7346-7390`, `tests/test_agent.py:9850-9946` |
| Test: tool timeout retry | `test_tool_timeout_triggers_retry`, `test_tool_timeout_retry_counts_as_failed`, `test_tool_timeout_message_format`, `test_tool_timeout_definition`, `test_tool_timeout_default_none`, `test_tool_timeout_exceeds_retry_limit`, `test_agent_level_tool_timeout` | `tests/test_tools.py:3076-3319` |
| Test: sync tool timeout abandons thread | `test_sync_tool_timeout_triggers_retry` (asserts the worker thread runs to completion while only its result is discarded) | `tests/test_tools.py:3111-3140` |
| Test: cancel during model request | `test_cancel_mid_continuation_cancels_job_and_stops`, `test_cancel_stops_loop_and_cancels_suspended_response`, `test_cancel_teardown_survives_scope_cancellation` | `tests/models/test_streamed_continuation.py:488`, `tests/models/test_continuation_stream.py:544,996` |
| Test: absorbed-cancel handoff | `test_run_handoff_survives_absorbed_cancellation`, `test_streaming_handoff_survives_absorbed_cancellation`, `test_run_stream_events_aclose_survives_absorbed_cancellation` | `tests/test_agent.py:9981-10130` |
| Test: timeout scope consumed via Task.uncancel | `test_consumed_cancellation_is_not_a_false_positive` | `tests/test_run_cancellation.py:146-159` |
| Test: cancel outside a run raises UserError | `test_cancel_outside_a_run_raises_user_error` | `tests/test_run_cancellation.py:1850-1854` |
| Test: Python 3.10 degraded behavior pinned | `test_absorbed_cancellation_completes_on_py310` | `tests/test_run_cancellation.py:1857-1879` |

## Answers to Dimension Questions

### 1. Can a tool hang forever?

Only if the developer does not set a timeout. By default, `Agent(tool_timeout=...)` is `None` (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:610-612`) and `FunctionToolset.timeout` is `None` (`pydantic_ai_slim/pydantic_ai/toolsets/function.py:72,93-94`); in that case `anyio.fail_after` is not entered and the tool may run indefinitely (`pydantic_ai_slim/pydantic_ai/toolsets/function.py:684-693`). The HTTP layer has its own default — `DEFAULT_HTTP_TIMEOUT = 600` seconds — but a long-running tool body that does not call out to an HTTP client has no built-in timeout. A user who sets `Agent(tool_timeout=...)` or `@tool_plain(timeout=...)` gets `anyio.fail_after`-bound execution with `ModelRetry` on expiry (`pydantic_ai_slim/pydantic_ai/toolsets/function.py:686-691`). Note: even with a timeout, sync `def` tools are not actually stopped — their worker thread is abandoned but keeps running (`pydantic_ai_slim/pydantic_ai/_utils.py:162-180`).

### 2. Are timeouts configurable?

Yes, per layer:

- **Tool-level**: `@agent.tool_plain(timeout=N)` / `@agent.tool(timeout=N)` (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:2414-2635`); per-toolset `FunctionToolset(timeout=...)` (`pydantic_ai_slim/pydantic_ai/toolsets/function.py:72`).
- **Agent-level default**: `Agent(tool_timeout=N)` propagated to the function toolset (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:610-680`).
- **Hook-level**: `@hooks.on.<hook>(timeout=N)` decorators on every hook entry (`pydantic_ai_slim/pydantic_ai/capabilities/hooks.py:281-460`).
- **Per-request model HTTP**: `ModelSettings.timeout` — accepts numeric seconds or `httpx.Timeout` (`pydantic_ai_slim/pydantic_ai/settings.py:209-222`).
- **MCP**: `MCPToolset(init_timeout=...)` and `MCPToolset(read_timeout=...)` (`pydantic_ai_slim/pydantic_ai/mcp.py:893-1043`).
- **SSRF-protected downloads**: `_ssrf.create_async_httpx2_client(timeout=...)` (`pydantic_ai_slim/pydantic_ai/_ssrf.py:541-601`).
- **Web fetch tool**: `WebFetchTool(timeout=...)` (`pydantic_ai_slim/pydantic_ai/common_tools/web_fetch.py:59,159`).
- **Realtime session handshake**: `RealtimeModelSettings.handshake_timeout` (OpenAI, Azure OpenAI, xAI; default 30 s) (`docs/timeouts.md:16`).

Documented as "no built-in wall-clock duration for a whole run" (`docs/timeouts.md:18`) — users wrap `agent.run()` in `asyncio.timeout()` / `anyio.fail_after()` or `CancellationToken`.

### 3. Can users cancel?

Yes, four distinct surfaces:

- **`CancellationToken`** passed to `agent.run(..., cancellation_token=...)`, cancelled from any thread (`pydantic_ai_slim/pydantic_ai/_cancel.py:42-89`). Works against `run_sync` (the only way to interrupt a sync run from outside it) (`docs/agent.md:634`).
- **`RunContext.cancel()`** from inside a tool, `event_stream_handler`, or capability hook (`pydantic_ai_slim/pydantic_ai/_run_context.py:467-498`).
- **`AgentRunEvents.cancel()`** on the handle yielded by `agent.run_stream_events()` (`pydantic_ai_slim/pydantic_ai/agent/abstract.py:197-208`).
- **`AgentRun.cancel()`** on the handle from `agent.iter()` (`pydantic_ai_slim/pydantic_ai/run.py:555-584`).
- **`StreamedRunResult.cancel()` / `StreamedRunResultSync`** stops only the current model response (`pydantic_ai_slim/pydantic_ai/result.py:158-167,782-809`).

All four first-party surfaces are thread-safe (`pydantic_ai_slim/pydantic_ai/_cancel.py:50-89`) and idempotent (a no-op once the run has finished).

### 4. Is cancellation cooperative or forced?

**Cooperative, with a strong backstop.** Pydantic AI does not forcibly kill code. The mechanisms are:

- **Cooperative**: `Task.cancel()` is delivered to whatever the run is awaiting; tools receive `CancelledError` at a suspension point (`docs/agent.md:737-738`).
- **Documented exception**: synchronous `def` tools run on a worker thread Python cannot terminate, so the worker is abandoned (result discarded, side effects may persist) (`pydantic_ai_slim/pydantic_ai/_utils.py:162-180`, `docs/agent.md:737-738`).
- **Backstop**: `raise_if_cancelling()` re-asserts a pending cancellation if a step's `await` absorbed the `CancelledError` without `Task.uncancel()` (`pydantic_ai_slim/pydantic_ai/_utils.py:332-368`); `_finalize_result` raises `CancelledError` if a first-party cancel was still requested (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:1838-1843`).
- **Known degraded mode**: user code that absorbs `CancelledError` and does not call `Task.uncancel()` is misread as a still-pending external cancellation, so the backstop will treat the run as cancelled even though it is the user's internal wake-up — documented as deliberate policy at `pydantic_ai_slim/pydantic_ai/_utils.py:332-368`. Cancellation identity (not count) would fix this — tracked as #7240 (`pydantic_ai_slim/pydantic_ai/_cancel.py:144,222-223`).

### 5. Does cancellation leave resources dirty?

The design works to keep them clean, with documented limits:

- **Tool tasks**: `_call_tools` cancels and drains via `cancel_and_drain(tasks, msg=e.args[0] if len(e.args) != 0 else None)` under a `with anyio.CancelScope(shield=True):` so AnyIO cannot re-cancel during teardown (`pydantic_ai_slim/pydantic_ai/_utils.py:312-329`, `pydantic_ai_slim/pydantic_ai/_tool_execution.py:828-836`). Both `CancelledError` and any other exception in the parallel segment triggers drain (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:828-836`).
- **Wrap-task handoff**: `_RunStreamEventsIterator.aclose()` cancels the background run and closes the receive stream, with explicit handling for the case where the run absorbs the cancel and resumes (`pydantic_ai_slim/pydantic_ai/agent/abstract.py:263-279`).
- **Model stream**: `StreamedResponse.cancel()` sets `_cancelled = True` *before* delegating to `close_stream()` so a transport-error on teardown still leaves the flag visible (`pydantic_ai_slim/pydantic_ai/models/__init__.py:1082-1097`). `iterator_with_cancel_guard` suppresses `httpx.StreamError`/`TransportError` from cancellation tearing down the stream (`pydantic_ai_slim/pydantic_ai/models/__init__.py:1046-1080`).
- **Suspended/background jobs**: `_continuation.request_with_continuation` and its error/over-limit paths call `await cancel_suspended_job(model, response)` *before* propagating so server-side jobs are not leaked (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:981-1039`).
- **Worker threads**: documented as not stoppable; result discarded, side effects may persist (`docs/agent.md:737-738`).
- **Run state**: `_translate_cancellation.finally` calls `graph_deps.cancellation.release_issued()` so a leaked `Task.cancelling()` count does not poison outer scopes like `asyncio.timeout()` or AnyIO cancel scopes (`pydantic_ai_slim/pydantic_ai/_cancel.py:244-257`, `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1790-1796`).
- **Message history**: cancellation is *non-destructive*: `RunCancelled.all_messages()` returns the messages produced before cancel, and `_repair_dangling_tool_calls` synthesizes `outcome='interrupted'` returns for tool calls that never produced a result so the history is provider-valid for resumption (`pydantic_ai_slim/pydantic_ai/exceptions.py:281-286`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2816-2887`, `pydantic_ai_slim/pydantic_ai/messages.py:1292-1295`).

## Architectural Decisions

- **CancellationToken is a *same-process* handle** that holds a live task reference and is never serialized (`pydantic_ai_slim/pydantic_ai/_cancel.py:22-23`). Durable execution rejects it at the activity boundary with an explicit `UserError` (`pydantic_ai_slim/pydantic_ai/durable_exec/_runtime_toolsets.py:38-49`, `pydantic_ai_slim/pydantic_ai/durable_exec/_base.py:164-176`), because a token that crossed an activity boundary would either be ignored or silently cancel a replay.
- **First-party vs external cancellation is distinguished by counting, not identity.** `RunCancellation._issued` is a `dict[Task, int]`; `resolve()` calls `Task.uncancel()` for each issued count and only translates if no external count remains (`pydantic_ai_slim/pydantic_ai/_cancel.py:140-148,225-242`). The known failure mode (user uncancels → external cancel arrives with matching count) is tracked at `pydantic_ai_slim/pydantic_ai/_cancel.py:144,222-223` (#7240).
- **External `CancelledError` is never translated.** Pydantic AI attaches the run state to the propagating exception via `_RUN_CANCELLED_ATTR` (`pydantic_ai_slim/pydantic_ai/exceptions.py:265,318-362`) so callers can recover state with `RunCancelled.from_cancellation(exc)` while still letting `asyncio.timeout()` produce `TimeoutError`, `TaskGroup` tear down cleanly, and Temporal end the workflow *Cancelled* (`docs/agent.md:647,777-782`).
- **The agent iter outer edge is the sole translation point.** `_translate_cancellation` does the `RunCancelled` conversion in `agent.iter()` (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:1763-1796`), which is why `agent.run()` (which wraps `iter()`) also raises `RunCancelled` and why raw external cancel still propagates as `CancelledError` from `agent.run()`.
- **Streamed response cancel is two-tier**: `_cancelled` is set immediately so the iterator's transport-error guard sees it, *then* `close_stream()` runs (`pydantic_ai_slim/pydantic_ai/models/__init__.py:1082-1097`). This is essential because `close_stream()` itself may raise when the transport is torn down.
- **Tool timeouts translate to `ModelRetry`, not a terminal failure.** A timeout consumes one retry budget of the tool (`pydantic_ai_slim/pydantic_ai/toolsets/function.py:686-691`, `tests/test_tools.py:3202-3221`), which means a slow tool can keep failing across retries — confirmed by `test_tool_timeout_exceeds_retry_limit` (`tests/test_tools.py:3285-3306`). `ToolFailed` is the alternative for terminal failures that should *not* consume the retry budget (`docs/timeouts.md:41`).
- **`Agent(tool_timeout=...)` only reaches agent-direct tools.** A user-constructed `FunctionToolset` passed via `toolsets=[...]` reads its own `timeout` and not the agent's (`docs/timeouts.md:25`). MCP and external toolsets don't read the agent timeout either.
- **Cancelled stream usage is best-effort and explicitly not cost-reliable** (`docs/agent.md:837-839`). `openai_continuous_usage_stats` improves in-stream usage but is still partial on cancel.

## Notable Patterns

- **`anyio.fail_after(timeout)` + `_utils.abandon_threads_on_cancel()`** is the canonical deadline idiom, used identically for tools and hooks (`pydantic_ai_slim/pydantic_ai/toolsets/function.py:686-691`, `pydantic_ai_slim/pydantic_ai/capabilities/hooks.py:239-252`). The abandon-on-cancel is required because `anyio.to_thread.run_sync` shields sync functions from cancellation by default — without it, `anyio.fail_after` would only fire after the thread returns.
- **Driven-task rebind at step boundaries.** `RunCancellation.bind()` is called at the start of every node so a manual `AgentRun.next()` driver from a different task still gets cancelled (`pydantic_ai_slim/pydantic_ai/_cancel.py:121-152`, `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1807`). A request that arrived before any task was bound is re-delivered on bind.
- **Token-before-limiter attachment.** `cancellation_token` is registered before entering the concurrency limiter so a pre-cancelled token prevents the run from blocking on a slot (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:1802-1815`); covered by `test_token_cancels_run_queued_behind_concurrency_limiter`.
- **Cancellation-finish ordering.** `graph_deps.cancellation.finish()` is registered as a `stack.callback` so it runs even on exception, neutralizing the controller after the run is over (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:1808`). Tested by `test_cancel_after_completion_is_a_noop` (`tests/test_run_cancellation.py:1708-1721`).
- **Sub-agent cancellation isolation is intentionally opt-in.** A sub-agent that cancels itself is *not* a parent cancellation by default — the delegate tool sees a `RunCancelled`, which if uncaught surfaces as a failed tool return the parent can react to (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:41-62,701-702`). To cancel a whole tree, share one `CancellationToken` (`docs/agent.md:843-847`).
- **`release_issued()` is a `finally` invariant.** `_translate_cancellation.finally` ensures an issued-but-unresolved cancellation does not contaminate outer scopes — covered by `test_cancel_followed_by_other_error_releases_cancellation` (`tests/test_run_cancellation.py:552`).
- **Per-provider `close_stream()` override** is the seam where transport-specific teardown lives (`pydantic_ai_slim/pydantic_ai/models/__init__.py:1115-1126`). Providers on gRPC/botocore are expected to override `get_stream_cancel_errors()` (`pydantic_ai_slim/pydantic_ai/models/__init__.py:1099-1113`).
- **Interruption is its own outcome**, distinct from `'success'`, `'failed'`, `'denied'` (`pydantic_ai_slim/pydantic_ai/messages.py:1335-1351`). `'interrupted'` is never mapped to a provider's native error channel — only `'failed'` is — so interruption is a neutral result the model sees without it being treated as a transient failure.

## Tradeoffs

- **Cooperative, not forced.** A synchronous tool body cannot be force-stopped; its worker thread runs to completion with no rollback of side effects (`docs/agent.md:737-738`). This is a hard ceiling of Python, not an oversight.
- **CancellationToken is single-use and same-process.** A reused token cancels every later run passed to it (`docs/agent.md:627`). The recommended pattern is a fresh token per run or per stop gesture.
- **Sub-agent cancellation does not propagate by default.** It is intentional and safer (avoids surprise parent cancellation), but a user who wants propagation has to opt in by catching `RunCancelled` and calling `ctx.cancel()` on the parent (`docs/agent.md:843-847`).
- **Tool-timeout propagation stops at agent boundaries.** `Agent(tool_timeout=...)` does not reach a `FunctionToolset` you constructed yourself, MCP, or external toolsets (`docs/timeouts.md:25`). Users have to know to configure each layer.
- **Synchronous tool timeout is bounded but not enforced.** The deadline fires after `anyio.fail_after`, but the underlying `def` body finishes in a thread; side effects persist (`pydantic_ai_slim/pydantic_ai/_utils.py:162-180`, `tests/test_tools.py:3111-3140`).
- **Provider-side model cancel is best-effort.** Whether a `cancel()` actually stops remote generation depends on the SDK; some keep generating server-side after the local stream closes (`docs/agent.md:837-839`).
- **Python 3.10 has degraded cancellation guarantees.** No `Task.cancelling()`/`Task.uncancel()`, so first-party/external attribution cannot be disambiguated; documented and pinned (`pydantic_ai_slim/pydantic_ai/_cancel.py:18-20`, `tests/test_run_cancellation.py:1857-1879`).
- **Cancellation attribution is count-based, not identity-based.** A user `Task.uncancel()` followed by an external cancel with matching count can be mis-credited — tracked as #7240 (`pydantic_ai_slim/pydantic_ai/_cancel.py:144,222-223`).
- **`StreamedRunResult.cancel()` does not cancel the run, only the response** (`pydantic_ai_slim/pydantic_ai/result.py:158-167`). Distinct from `AgentRun.cancel()` which ends the whole run. This is a frequent foot-gun.
- **`CancelledError` raised by user code inside a tool body is indistinguishable from a timeout.** Both produce `'Timed out after N seconds.'` retry prompts (`docs/timeouts.md:27-32`).

## Failure Modes / Edge Cases

- **Absorbed cancellation during handoff.** Documented as #6422: under Temporal's `WAIT_CANCELLATION_COMPLETED` a step can swallow the injected `CancelledError` and complete normally. Pydantic AI handles this by setting `stream_done` before draining, so the run can exit cleanly even when the wrap_task absorbs the cancel (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1240-1257`). Covered by `test_run_handoff_survives_absorbed_cancellation`, `test_streaming_handoff_survives_absorbed_cancellation`, `test_run_stream_events_aclose_survives_absorbed_cancellation` (`tests/test_agent.py:9981-10130`).
- **Consumed cancellation is not a false positive.** A timeout scope inside a tool calls `Task.uncancel()` (e.g. `anyio.move_on_after`), so `raise_if_cancelling()` does not re-raise — covered by `test_consumed_cancellation_is_not_a_false_positive` (`tests/test_run_cancellation.py:146-159`).
- **`cancel()` issued before `bind()` is delivered later.** The controller stores the request and re-issues it on the next `bind()` (`pydantic_ai_slim/pydantic_ai/_cancel.py:121-152`). Covered by `test_cancel_before_bind_delivers_on_bind` (`tests/test_run_cancellation.py:737`).
- **`cancel()` after the run finished is a no-op.** Covered by `test_cancel_after_completion_is_a_noop` (`tests/test_run_cancellation.py:1708-1721`).
- **`cancel()` after `End` inside `iter()` surfaces as `RunCancelled` on context exit.** Trailing `__anext__` raises `StopAsyncIteration`, not `CancelledError`, when cancel was requested as the run finished — covered by `test_cancel_after_end_within_context_stops_iteration_cleanly` (`tests/test_run_cancellation.py:1724-1738`).
- **Nested `RunCancelled` reaching the outer edge is re-stamped** with the parent's history, keeping the nested one as `__cause__` (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:1770-1779`).
- **Suspended-job leak on failure paths.** Each `_continuation` failure branch calls `cancel_suspended_job` before re-raising, including the inter-poll sleep path (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:982-1039`). Tested by `test_cancel_mid_continuation_cancels_job_and_stops` and the merge-failure guards above.
- **Parallel-tool exception cancel-on-non-CancelledError.** Originally only `CancelledError` triggered sibling drain; extended to any exception (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:831-837`). Tested by `test_parallel_tool_exception_cancels_sibling_tasks` (`tests/test_agent.py:9850-9897`).
- **Suspended-job retry-after-cancel must re-cancel.** OpenAI Responses cancellation does not re-cancel on resumption; Pydantic AI cancels the *background* segment of any resumed response (`pydantic_ai_slim/pydantic_ai/models/openai.py:2016`). Tested by `test_cancel_suspended_response_only_cancels_background_jobs` (`tests/models/test_openai_responses.py:12998`).
- **Cancellation during between-segment sleep skips the next request.** Without this, a server-side job could burn tokens after cancel (`tests/models/test_continuation_stream.py:1165`).
- **Transport-error from cancel is not a real error.** When the user cancels mid-stream and the connection tears down, `iterator_with_cancel_guard` suppresses the transport error and stamps the stream `'interrupted'` (`pydantic_ai_slim/pydantic_ai/models/__init__.py:1046-1080`). Tested by `test_cancel_suppresses_segment_transport_error` (`tests/models/test_continuation_stream.py:697`).
- **`CancellationToken` inside durable execution** raises `UserError` rather than silently no-op'ing — covered in `_runtime_toolsets.py:38-49`.
- **A run cancelled via `asyncio.timeout()` keeps the `TimeoutError` surface.** `from_cancellation()` recovers state via the `__context__` chain, but the caller must re-raise after capturing (`pydantic_ai_slim/pydantic_ai/exceptions.py:322-362`).

## Future Considerations

- **Issue #7240 — cancellation-identity tracking.** Replacing the count-based attribution in `_issued` with identity tracking would eliminate the user-uncancel mis-attribute (`pydantic_ai_slim/pydantic_ai/_cancel.py:144,222-223`). This is the only documented correctness gap in the cancellation controller.
- **Wrapping a whole run in `asyncio.timeout()`** is the recommended wall-clock bound (`docs/timeouts.md:18`), but the framework could provide it directly as `Agent.run(timeout=...)` to match the symmetry of `tool_timeout` / hook timeout.
- **Tool-timeout propagation to user-constructed toolsets** is asymmetric today (`docs/timeouts.md:25`). A `WrapperToolset(timeout=...)` would compose cleanly with the existing `WrapperToolset` pattern (`pydantic_ai_slim/pydantic_ai/toolsets/AGENTS.md` guidance) and apply to MCP/external toolsets uniformly.
- **Provider-side cancel reliability** depends on each SDK; could be tracked per-provider in `docs/models/{provider}.md` so users know which providers' cancels are best-effort vs reliable.
- **Cancellation token durability**: Temporal/DBOS/Prefect reject same-process tokens at the boundary (`pydantic_ai_slim/pydantic_ai/durable_exec/_runtime_toolsets.py:38-49`). A durable equivalent that survives an activity boundary — e.g. an engine-side cancel signal — would unify cancellation across the durable and in-process runtimes.
- **A `cancel()` reaping hook on finished runs** could surface late arrivals of `RunCancelled` for tests/observability, but currently `finish()` makes it a true no-op (`pydantic_ai_slim/pydantic_ai/_cancel.py:194-201`).

## Questions / Gaps

- **Is there a global timeout for a whole run?** No — the framework explicitly defers this to `asyncio.timeout()` / `anyio.fail_after()` / `CancellationToken` from a timer (`docs/timeouts.md:18`). A built-in option would close a paper cut but the design is documented and intentional.
- **`StreamedRunResult.cancel()` vs `AgentRun.cancel()` distinction** is documented (`pydantic_ai_slim/pydantic_ai/result.py:158-167`, `pydantic_ai_slim/pydantic_ai/run.py:555-584`) but is the most likely user-confusion point — no proactive discoverability beyond the type names.
- **Tools that already use `asyncio.shield` around critical work** may behave oddly under cancel because `shield` suppresses cancellation inside that block. This is general asyncio behavior, not a Pydantic-AI quirk.
- **Provider-side remote generation that continues after a local stream close** (`docs/agent.md:837-839`) cannot be observed beyond usage, and usage itself is unreliable on cancel. There is no public API for hard-stopping provider-side generation.
- **`StreamedRunResultSync` lifecycle** — cancellation depends on the underlying bridge loop (`pydantic_ai_slim/pydantic_ai/result.py:812-836`), and the wrapper "must be used and closed on the thread where it was created." This is documented but is a subtle thread-affinity requirement.
- **`CancellationToken.cancel()` semantics during `iter()` after `End`** — the cancel is honored on context exit (`tests/test_run_cancellation.py:1724-1738`) but the contract is non-obvious from the type signature.
- **Sync tool side effects after cancel** are explicitly *not* rolled back (`docs/agent.md:737-738`). There is no "best-effort undo" surface — a user needing that must build their own compensation into the tool body.
- **Realtime session interruption** has its own semantics (`pydantic_ai_slim/pydantic_ai/realtime/openai.py:391-836`, `pydantic_ai_slim/pydantic_ai/realtime/google.py:1060-1322`). The interaction between `CancellationToken` and a realtime session is not fully explored in the dimension's surface; deferred to a dedicated dimension.

# Source Analysis: openai-agents-sdk

## Dimension 13.04: Recovery vs Escalation

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (OpenAI Agents SDK; asyncio, httpx/openai client, MCP) |
| Analyzed | 2026-08-25 |

All file citations below are relative to the source root (`studies/agent-harness-study/sources/openai-agents-sdk/`).

## Summary

The OpenAI Agents SDK separates recovery into three explicit tiers with a fail-closed default. (1) **Automatic retry** exists only for model transport failures: a runner-managed retry loop (`src/agents/run_internal/model_retry.py:574-681`) driven by composable, application-supplied retry policies (`src/agents/retry.py:209-230`, `src/agents/retry.py:304-464`) and provider "retry advice", with hard vetoes for aborts, streamed output already emitted, and replay-unsafe requests (`src/agents/run_internal/model_retry.py:419-479`). MCP transports have their own budgeted exponential-backoff retries plus isolated-session reconnection (`src/agents/mcp/server.py:874-944`, `src/agents/mcp/server.py:2359-2456`), and the sandbox layer has a generic `retry_async` decorator (`src/agents/sandbox/util/retry.py:65-127`). (2) **Model-visible self-recovery** for tool failures: instead of retrying tools, the SDK converts exceptions into messages sent back to the model via `failure_error_function` (`src/agents/tool.py:1863-1872`, `src/agents/tool.py:1964-1981`). (3) **Escalation to humans** is implemented as structured HITL interruptions: tools declare `needs_approval`, the run pauses into `NextStepInterruption`, state is serializable, and the human decides through `RunState.approve/reject` before resume (`src/agents/run_state.py:1255-1298`). Graceful stopping is first-class: `max_turns` limits (`src/agents/run_config.py:45`), streaming cancel modes (`immediate`/`after_turn`, `src/agents/result.py:647`), and pluggable error handlers that can convert terminal errors (`max_turns`, `model_refusal`, `invalid_final_output`) into validated fallback final outputs (`src/agents/run_error_handlers.py:50-55`). Audit trails combine per-exception `RunErrorDetails` snapshots (`src/agents/exceptions.py:413-441`, attached at `src/agents/run.py:2132-2144`), tracing span errors (`src/agents/run_internal/error_handlers.py:75-110`), debug retry logs, and zero-token usage entries that make failed attempts observable in usage counts (`src/agents/run_internal/model_retry.py:338-350`).

## Rating

**Score: 9/10**

Rationale against the rubric:

- **Clear model**: retry-vs-stop decisions are centralized in `_evaluate_retry` (`src/agents/run_internal/model_retry.py:389-493`) with three explicitly documented, deliberately separate replay vetoes (`src/agents/run_internal/model_retry.py:442-479`); escalation is a distinct mechanism (interruptions), not an overloaded retry.
- **Tests**: extensive failure-path coverage — ~60 tests in `tests/models/test_model_retry.py` covering timeouts, parent cancellation preservation, provider vetoes, budget exhaustion, replay safety (`tests/models/test_model_retry.py:150`, `:1481`, `:1601`); MCP retry/isolated-session tests (`tests/mcp/test_client_session_retries.py:448-580`); sandbox retry strategy tests (`tests/sandbox/test_retry.py:92`); max-turns, error-handler, and HITL-resume tests (`tests/test_max_turns.py`, `tests/test_run_internal_error_handlers.py:87`, `tests/test_hitl_error_scenarios.py:240-583`).
- **Operational safeguards**: fail-closed defaults for stateful replays (`src/agents/run_internal/model_retry.py:454-469`), explicit opt-in for unsafe replay approval (`src/agents/retry.py:123-129`), redaction of exception payloads at public boundaries (`src/agents/exceptions.py:181-215`).
- **Configurable**: retry budgets/backoff/policies per model call (`src/agents/model_settings.py:188-189`), MCP retry knobs (`src/agents/mcp/server.py:874-881`), approval policies per tool (`needs_approval` as bool or callable, `docs/human_in_the_loop.md:43`), custom rejection text (`src/agents/run_config.py:448`).
- Not a 10 because there is no built-in out-of-band human notification or escalation channel (HITL assumes the app polls `result.interruptions`), retry events are only debug-level logs rather than structured public events, and three coexisting model-retry layers (provider-managed, runner-managed, conversation-locked compatibility) form intricate interactions documented mostly in code comments (`src/agents/run_internal/model_retry.py:516-554`).

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Runner-managed model retry loop | `get_response_with_retry` / `stream_response_with_retry`: while-loop over attempts, policy evaluation, rewind + sleep between attempts | `src/agents/run_internal/model_retry.py:574-681`, `src/agents/run_internal/model_retry.py:684-891` |
| Retry decision gate | `_evaluate_retry`: budget check (`attempt > max_retries` → stop), abort veto, emitted-output veto, replay-safety vetoes | `src/agents/run_internal/model_retry.py:403-404`, `src/agents/run_internal/model_retry.py:419-433`, `src/agents/run_internal/model_retry.py:442-479` |
| Default backoff | constants 0.25 s initial / 2.0 s max / ×2 / jitter on; `_default_retry_delay` computes capped exponential backoff | `src/agents/run_internal/model_retry.py:46-49`, `src/agents/run_internal/model_retry.py:175-204` |
| Compatibility conversation-lock retry | legacy path retries `conversation_locked` up to 3 times, delay `1.0 * 2**(n-1)`, disabled by explicit `max_retries=0` | `src/agents/run_internal/model_retry.py:50`, `src/agents/run_internal/model_retry.py:619-640`, `src/agents/run_internal/model_retry.py:504-513` |
| Retry policy API (configurable) | `ModelRetrySettings(max_retries, backoff, policy)`; `RetryPolicy = Callable[[RetryPolicyContext], MaybeAwaitable[bool \| RetryDecision]]`; `approve_unsafe_replay` field documented as deliberate separate approval | `src/agents/retry.py:209-230`, `src/agents/retry.py:185`, `src/agents/retry.py:116-133` |
| Built-in composable policies | `retry_policies.never/provider_suggested/network_error/retry_after/http_status/all/any`, incl. hard-veto vs delegable-veto semantics | `src/agents/retry.py:304-464` |
| Normalized error facts exposed to policies | `ModelRetryNormalizedError(status_code, error_code, request_id, retry_after, is_abort, is_network_error, is_timeout)`; context exposes attempt/max_retries/stream/stateful_request | `src/agents/retry.py:49-89`, `src/agents/retry.py:142-183` |
| Provider retry advice | `get_openai_retry_advice`: honors `unsafe_to_replay`, `x-should-retry` header, network/timeout classes, 408/409/429/5xx; marks stateful requests replay-safe | `src/agents/models/_openai_retry.py:41-112` |
| Provider advice plumbing | `get_retry_after` parses `retry-after-ms`/`retry-after` headers across the exception chain; context vars disable hidden provider/websocket retries when the runner owns replay | `src/agents/models/_retry_runtime.py:88-97`, `src/agents/models/_retry_runtime.py:138-171` |
| Provider/runner retry coordination | `_should_disable_provider_managed_retries` decides per attempt whether hidden SDK retries run, so budgets don't stack silently | `src/agents/run_internal/model_retry.py:516-554` |
| Config attachment point | `ModelSettings.retry: ModelRetrySettings \| None` ("Opt-in runner-managed retry settings"); merged in `ModelSettings.resolve` | `src/agents/model_settings.py:188-189`, `src/agents/model_settings.py:284-287` |
| Tool failure → model self-recovery | `default_tool_error_function` returns "Please try again…" message to the model; per-tool override via `failure_error_function` (None raises); cancellation coerced to Exception contract | `src/agents/tool.py:1863-1872`, `src/agents/tool.py:1886-1911`, `src/agents/tool.py:1955-1961`, `src/agents/tool.py:1964-1981` |
| Tool timeout behavior options | `error_as_result` vs `raise_exception`; `ToolTimeoutError` carries tool name and seconds | `src/agents/tool.py:1875-1883`, `src/agents/exceptions.py:507-516` |
| MCP transport retries (configurable) | `max_retry_attempts=0`, `retry_backoff_seconds_base=1.0`, `retry_backoff_seconds_max=None`; exponential backoff calc; unlimited mode (-1) | `src/agents/mcp/server.py:874-881`, `src/agents/mcp/server.py:942-944`, `src/agents/mcp/server.py:1219-1228` |
| MCP isolated-session recovery | 5xx/408/closed-resource retried on a fresh session; mixed exception groups not retried; outer cancellation preserved; isolated retry charges the shared budget | `src/agents/mcp/server.py:2325-2336`, `src/agents/mcp/server.py:2359-2456` |
| MCP manager-level reconnect | `retry_failed_servers(failed_only=...)` cleans up and reconnects failed servers | `src/agents/mcp/manager.py:319-345` |
| Sandbox generic retry | `retry_async` decorator: FIXED/LINEAR/EXPONENTIAL backoff, `retry_if` predicate, `on_retry` hook, transient status set {500, 502, 503, 504} | `src/agents/sandbox/util/retry.py:14-26`, `src/agents/sandbox/util/retry.py:65-127` |
| Human escalation trigger | Tools declare `needs_approval` (bool or callable); shell/custom/apply-patch/function paths evaluate it and pause the run | `src/agents/agent.py:600-630`, `src/agents/run_internal/tool_execution.py:1300-1318`, `src/agents/run_internal/turn_resolution.py:2560` |
| Escalation surface to humans | Run pauses as `NextStepInterruption(interruptions=[ToolApprovalItem…])`; surfaced on `RunResult.interruptions` / `RunResultStreaming.interruptions` | `src/agents/run_internal/turn_resolution.py:1822-1833`, `src/agents/items.py:556`, `src/agents/run.py:1206` |
| Human decision API | `RunState.approve(approval_item, always_approve)` and `RunState.reject(..., always_reject, rejection_message)`; nested agent-tool approvals routed to nested state; sticky decisions serialized | `src/agents/run_state.py:1255-1298`, `src/agents/run_state.py:1300-1314` |
| Programmatic (no-human) approval callbacks | `on_approval` callbacks for hosted MCP/shell/apply_patch auto-approve/auto-reject without surfacing interruptions | `docs/human_in_the_loop.md:43` |
| Custom rejection message config | run-wide `RunConfig.tool_error_formatter`; per-call `rejection_message` precedence documented and implemented | `src/agents/run_config.py:84`, `src/agents/run_config.py:448`, `src/agents/run_state.py:1276-1282` |
| Graceful stop: turn limit | `DEFAULT_MAX_TURNS = 10`; `max_turns=None` disables; exceeded → `MaxTurnsExceeded` with span error recorded | `src/agents/run_config.py:45`, `docs/running_agents.md:41`, `src/agents/run_loop.py:1542-1550` |
| Graceful stop: error handlers | `Runner.run(..., error_handlers={"max_turns"|"model_refusal"|"invalid_final_output"})`; handler returns fallback `final_output` (+`include_in_history`) validated against `output_type`; returning None declines recovery and re-raises | `src/agents/run_error_handlers.py:36-55`, `src/agents/run_internal/error_handlers.py:226-262`, `src/agents/run.py:1482-1490`, `src/agents/run_internal/turn_resolution.py:426-492` |
| Graceful stop: streaming cancel modes | `_cancel_mode ∈ {none, immediate, after_turn}`; `after_turn` completes current turn, persists items, then completes queue cleanly | `src/agents/result.py:647`, `src/agents/result.py:845`, `src/agents/run_loop.py:356-366` |
| Pause/resume persistence | `RunState.to_json/to_string/from_string`; resumed interruption state restored from result (`state._current_step = NextStepInterruption(...)`) | `src/agents/run_state.py:1704`, `src/agents/run_state.py:2042`, `src/agents/run_state.py:2100`, `src/agents/result.py:158-160` |
| Audit: exception run data | Every `AgentsException` gets `run_data: RunErrorDetails` snapshot (input, new_items, raw_responses, last_agent, guardrail results) attached on failure | `src/agents/exceptions.py:413-441`, `src/agents/run.py:2132-2144` |
| Audit: tracing | Generic agent-span error attachment policy shared by streaming/non-streaming; redacted variant when sensitive tracing disabled; `SpanError` payload | `src/agents/run_internal/error_handlers.py:75-110`, `src/agents/tracing/spans.py:19` |
| Audit: retry logging & usage | Debug logs of retry delays/attempts; failed attempts added to `usage.requests` as zero-token entries so retry counts are observable | `src/agents/run_internal/model_retry.py:631-636`, `src/agents/run_internal/model_retry.py:669-676`, `src/agents/run_internal/model_retry.py:338-350` |

## Answers to Dimension Questions

**1. When does the system retry vs escalate?**

Retry is confined to transient infrastructure faults on outbound calls. For model calls, `_evaluate_retry` (`src/agents/run_internal/model_retry.py:389-493`) stops immediately on exhausted budget (`attempt > max_retries`, line 403-404), on abort-like errors (`asyncio.CancelledError` chains, lines 74-87), or if user-visible stream events were already emitted (only `response.created`/`response.in_progress` are retry-safe, lines 51, 384-386). Otherwise the configured `RetryPolicy` decides, with three fail-closed vetoes for replay hazards: request-level side-effect veto, stateful-request veto (`previous_response_id`/`conversation_id` fails closed unless provider marks safe or the app passes an explicit replay approval), and provider-marked-unsafe veto (lines 442-479). MCP calls retry on transient transport conditions including 5xx/408/closed resources via isolated sessions (`src/agents/mcp/server.py:2325-2359`). Function tools are never automatically retried — their failures are formatted into model-visible messages so the model decides whether to try again (`src/agents/tool.py:1872`). Escalation happens for *authorization*, not failure: any tool whose `needs_approval` evaluates true pauses the whole run into an interruption regardless of error state (`src/agents/run_internal/turn_resolution.py:2560`). Terminal semantic failures (max turns, refusal, invalid structured output) do not retry; they raise unless an error handler supplies a fallback output (`src/agents/run_error_handlers.py:50-55`).

**2. Are escalation thresholds configurable?**

Yes, extensively. Model retries: `ModelSettings.retry.max_retries`, `backoff{initial_delay, max_delay, multiplier, jitter}`, and a full custom `policy` callback receiving normalized error facts and provider advice (`src/agents/model_settings.py:188-189`, `src/agents/retry.py:209-230`, `src/agents/retry.py:142-183`); built-ins cover provider-suggested, network-error, retry-after, and arbitrary HTTP-status sets, combinable via `all`/`any` (`src/agents/retry.py:315-374`, `src/agents/retry.py:376-461`). MCP: `max_retry_attempts` (including `-1` unlimited), base and cap of exponential backoff (`src/agents/mcp/server.py:874-881`, `:1219-1228`). Approval thresholds are configurable per tool as bool *or* callable evaluated at call time (`src/agents/util/_approvals.py:32-43`), plus programmatic `on_approval` callbacks that bypass pausing entirely (`docs/human_in_the_loop.md:43`). The turn limit itself is configurable and disableable (`src/agents/run_config.py:45`, `docs/running_agents.md:41`). Even the hidden provider-side retry layer can be suppressed so callers get deterministic single-shot behavior (`src/agents/run_internal/model_retry.py:527-531`).

**3. Can the system stop gracefully?**

Yes, through several mechanisms. A max-turns overrun becomes a recoverable event: without a handler it raises `MaxTurnsExceeded` carrying full `run_data`; with a registered handler the run converts into a validated fallback `final_output`, runs output guardrails and hooks, persists history, and returns normally (`src/agents/run.py:1466-1563`, `src/agents/run_internal/run_loop.py:1542-1652`). Streaming supports cooperative cancellation where `after_turn` finishes the in-flight turn and persists items before completing (`src/agents/run_loop.py:356-366`, `src/agents/result.py:647`). HITL pauses are fully serializable: `result.to_state()` → `to_json/to_string` → approve/reject → resume via `Runner.run(agent, state)`, with sticky approval decisions surviving serialization (`src/agents/run_state.py:1704`, `:2042`, `:2100`, `:1300-1314`; flow documented at `docs/human_in_the_loop.md:50-53`). Cancellation hygiene is unusually careful: timed-out model tasks are cancelled, drained, and scrubbed from tracebacks so no provider task leaks (`src/agents/run_internal/model_retry.py:234-255`, tested at `tests/models/test_model_retry.py:207-345`).

**4. Are recovery decisions auditable?**

Partially, with good but not complete coverage. On failure, every `AgentsException` receives a `RunErrorDetails` snapshot (input, generated items, raw responses, last agent, all guardrail results) accessible post-mortem (`src/agents/exceptions.py:434-441`, attached at `src/agents/run.py:2132-2144`; tested in `tests/test_run_error_details.py`). Tracing records span errors for failed runs and max-turns breaches (`src/agents/run_internal/error_handlers.py:88-102`, `src/agents/run_loop.py:1543-1549`). Retry activity is visible through debug logs naming delay and attempt index (`src/agents/run_internal/model_retry.py:669-676`) and through `usage.requests` inflation with zero-token entries per failed attempt (`src/agents/run_internal/model_retry.py:338-350`). MCP tracks consumed retry budget precisely, even charging isolated-session setup failures (`src/agents/mcp/server.py:2420-2437`, tested at `tests/mcp/test_client_session_retries.py:523-537`). What is missing: there is no structured public event/span emitted per retry decision (reason, chosen delay, veto type) — reconstructing *why* a retry happened requires enabling debug logs or inspecting usage deltas.

## Architectural Decisions

1. **Policy objects instead of flags.** Retry behavior is a callable `RetryPolicy` receiving rich context, composed via `all`/`any` combinators with explicit veto semantics (`src/agents/retry.py:185`, `:376-461`) rather than a matrix of booleans. This makes retry-vs-stop an application-owned decision with SDK-provided building blocks.
2. **Provider authority + application approval split.** Providers emit `ModelRetryAdvice` (including `replay_safety`), and the runtime treats provider "unsafe"/abort/emitted-output signals as hard vetoes that application code cannot override, while `approve_unsafe_replay` is a distinct, explicit second consent channel (`src/agents/retry.py:116-133`, `src/agents/run_internal/model_retry.py:419-479`). This prevents well-meaning blanket-retry policies from duplicating non-idempotent provider work.
3. **Single retry authority per layer.** When the runner manages retries, hidden provider-managed retries and websocket pre-event retries are switched off via context vars so budgets don't multiply invisibly (`src/agents/models/_retry_runtime.py:138-171`, decision table at `src/agents/run_internal/model_retry.py:516-554`).
4. **Escalation as interruption, not exception.** Human escalation pauses the run and yields a serializable state object rather than raising, keeping the control flow resumable and audit-friendly (`src/agents/result.py:158-160`, `src/agents/run_state.py:1255-1298`).
5. **Errors become conversation content.** Tool failures and approval rejections are formatted into model-visible messages (with configurable formatting and custom rejection text), making the model the recovery engine for tool-level faults (`src/agents/tool.py:1863-1872`, `src/agents/run_config.py:448`).
6. **Recovery outputs pass the same gates.** Error-handler fallback outputs must validate against the agent's `output_type`, go through output guardrails, hooks, and session persistence like normal finals (`src/agents/run_internal/error_handlers.py:170-205`, `finalize_max_turns_handler_output` at `src/agents/run_internal/run_loop.py:742-786`).

## Notable Patterns

- **Normalized error taxonomy**: both retry stacks normalize arbitrary exception chains into a fact record (`status_code`, `error_code`, `is_network_error`, `is_timeout`, `retry_after`) by walking `__cause__`/`__context__` with cycle protection (`src/agents/models/_retry_runtime.py:16-35`, `src/agents/sandbox/util/retry.py:29-47`) — the same pattern duplicated at two layers.
- **Fail-closed statefulness**: stateful (`previous_response_id`/`conversation_id`) requests get stricter replay rules than stateless ones, with the compatibility-only `conversation_locked` path preserved but explicitly escapable via `max_retries=0` (`src/agents/run_internal/model_retry.py:504-513`).
- **Redaction-aware auditing**: error payloads crossing public boundaries are replaced with detached, traceback-cleared placeholders when sensitive-data tracing is off (`src/agents/exceptions.py:159-215`, `src/agents/run_internal/error_handlers.py:94-98`).
- **Documentation tied to implementation**: the runner-managed retries section (`docs/models/index.md:512+`) and error-handler guide (`docs/running_agents.md:471-561`) describe exactly the code paths cited above, including the "does not retry the model call or replay any tool side effects" contract for `invalid_final_output` handlers (`docs/running_agents.md:500`).

## Tradeoffs

- **Safety vs simplicity**: the three-layer retry coordination (provider-managed / runner-managed / conversation-locked compatibility) is carefully reasoned but hard to hold in one's head; correctness depends on dense comments like the "three separate vetoes, deliberately not folded together" block (`src/agents/run_internal/model_retry.py:442-479`) and a large test matrix (`tests/models/test_model_retry.py`).
- **No automatic tool retry**: converting tool errors to model messages leverages model intelligence but means flaky tools burn tokens and turns instead of getting a cheap programmatic retry; applications wanting tool-level retry must implement it inside the tool.
- **Debug-level retry observability**: cheap to ship, but production users relying on structured telemetry must infer retries from usage entries or traces rather than subscribing to retry events.
- **Escalation latency**: HITL pauses require the host process (or serialized state) to survive until human decision; the SDK mitigates this with full state serialization but does not provide durable queues or notification delivery itself.

## Failure Modes / Edge Cases

Handled explicitly (with tests):

- Timeout mid-stream cancels and drains the owner task, discards its exception graph, and raises `ModelTimeoutError` from a clean frame (`src/agents/run_internal/model_retry.py:288-315`, `tests/models/test_model_retry.py:207-280`).
- Parent cancellation during retry cleanup is preserved rather than swallowed (`src/agents/run_internal/model_retry.py:301-303`, tests at `tests/models/test_model_retry.py:318-408`).
- Malformed/persisted approvals that no longer match queued tool calls re-pause the run instead of executing unverified work (`src/agents/run_internal/turn_resolution.py:1784-1833`, `tests/test_hitl_error_scenarios.py:536-583`).
- Ambiguous nested-agent approval identities raise `UserError` rather than guessing (`src/agents/run_state.py:1241-1252`).
- Mixed exception groups are not retried blindly in MCP isolated sessions; outer cancellation always wins (`tests/mcp/test_client_session_retries.py:553-580`).
- Handler-produced fallbacks that fail schema validation raise `UserError` instead of emitting bad output (`src/agents/run_internal/error_handlers.py:198-205`).
- Unhandled handler errors under redaction surfaces a payload-free `UserError` (`src/agents/run_internal/turn_resolution.py:474-492`).

Residual risks:

- Retry storms are bounded by budget but the unlimited MCP mode (`max_retry_attempts=-1`) relies solely on uncapped-by-default exponential backoff (`retry_backoff_seconds_max=None`, `src/agents/mcp/server.py:881`, `:1219-1228`).
- The `conversation_locked` compatibility retry runs *even when the caller configured policies for unrelated failures* (`src/agents/run_internal/model_retry.py:621-627`) — historical behavior retained for compatibility, which can surprise budget accounting.

## Future Considerations

- Emit structured, public retry-decision events (or tracing spans with reason/veto/delay) so recovery decisions are auditable without debug logs.
- Provide optional escalation adapters (webhook/callback notification on interruption) since human notification currently rests entirely on the embedding application reading `result.interruptions`.
- Consider promoting the provider-advice/replay-safety machinery behind a stable public seam for non-OpenAI model providers, which currently rely on the generic normalized-error path.

## Questions / Gaps

- No evidence found of any automatic human-notification mechanism (email/chat/pager) anywhere in `src/agents/`; searches for escalation/notification infrastructure returned only HITL approval surfaces. Escalation-to-human outside tool approvals is entirely the application's responsibility.
- No evidence found of retry-policy configuration via environment/config files; all retry configurability flows through Python constructors (`ModelSettings.retry`, `MCPServer*` kwargs). `src/agents/_config.py` was inspected for related keys and none exist.
- Guardrail tripwires intentionally do not retry or escalate: they terminate the run and persist accumulated results (`src/agents/run_loop.py:1214-1232`); if tripwire-triggered re-prompting counts as "recovery," the SDK delegates that to the caller.

---

Generated by `Dimension 13.04: Recovery vs Escalation` against `openai-agents-sdk`.

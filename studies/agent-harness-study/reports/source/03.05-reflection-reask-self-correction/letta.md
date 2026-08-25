# Source Analysis: letta

## Reflection, ReAsk, and Self-Correction Loops

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `sources/letta` |
| Language / Stack | Python (FastAPI + SQLAlchemy + Pydantic), LLM-agnostic agent platform |
| Analyzed | 2026-07-14 |

## Summary

Letta's agent loop drives "self-correction" through three orthogonal mechanisms rather than a dedicated ReAsk/Reflect judge primitive:

1. **Compaction as forced reflection**: An LLM-backed "summarizer / compactor" rewrites the in-context message history when the running token estimate crosses `SUMMARIZATION_TRIGGER_MULTIPLIER * context_window` (default 0.9) (`letta/services/summarizer/thresholds.py:27-41`, `letta/constants.py:83`). On the way down, if the compactor fails to shrink under `trigger_threshold`, a chain of fallback modes (`self_compact_sliding_window` → `all`) fires before a non-recoverable error is raised (`letta/services/summarizer/compact.py:220-413`).
2. **Per-step retry on context-window overflow**: A bounded `for llm_request_attempt in range(summarizer_settings.max_summarizer_retries + 1)` loop catches `ContextWindowExceededError` between LLM calls; each attempt compacts the prompt and re-issues the request, with current attempt + cap logged (`letta/agents/letta_agent_v3.py:1093-1299`, `letta/agents/letta_agent_v2.py:518-573`, `letta/agent.py:1037-1067`). Defaults to 3 attempts (`letta/settings.py:96`).
3. **Tool-rule driven corrective feedback**: The `ToolRulesSolver` rejects LLM tool choices outside the allowed set and emits a synthesized `[ToolConstraintError]` plus a "Continuing: tool rule violation" heartbeat so the next turn re-enters the loop with explicit, structured feedback (`letta/agents/helpers.py:501-505`, `letta/agents/letta_agent_v3.py:1987-2036`).

Validation in the loop runs in three places: (a) tool-arg JSON parsing via `_safe_load_tool_call_str` (`letta/agents/helpers.py:378-393`); (b) approval tool_call_id round-trip with idempotency check (`letta/agents/helpers.py:101-145, 230-294`); (c) post-execution `_step_checkpoint_*` plumbing that fails safe by rolling back `step_progression` (`letta/agents/letta_agent_v3.py:1511-1592`, `letta/agents/letta_agent_v2.py:648-728`). Errors are surfaced as a closed `StopReasonType` enum (`end_turn | error | llm_api_error | invalid_llm_response | invalid_tool_call | max_steps | max_tokens_exceeded | no_tool_call | tool_rule | cancelled | insufficient_credits | requires_approval | context_window_overflow_in_system_prompt`) (`letta/schemas/letta_stop_reason.py:9-49`). The streaming service surfaces mid-stream errors as `event: error` SSE chunks with a typed `error_type` and never silently closes a successful stream without a `[DONE]` (`letta/services/streaming_service.py:600-700`).

A LLM-routing "circuit breaker" exists in the Redis-backed agent deployment (`letta/services/llm_router/llm_router_client_base.py:42-93`); OSS installs get a `RuntimeError` from `resolve_auto_mode_config`/`get_fallback_config_for_handle` so fallback is opt-in. There is **no auto-mode-style task-level "reflect on past answers" pattern** in OSS build — corrective feedback is structural, not LLM-as-judge.

`max_summarizer_retries` (3) is the primary retry budget; legacy code also exposes `max_summarization_retries`, `empty_response_retry_limit=3`, and an exponential-backoff `get_ai_reply` block on the deprecated `letta/agent.py` (`letta/agent.py:354-430`). Repeated failures on context overflow become `SystemPromptTokenExceededError` → `RunStatus.failed` (`letta/errors.py:364-371`, `letta/schemas/letta_stop_reason.py:36-43`), so the system never claims success when context could not be reclaimed.

## Rating

**6 / 10 — Present but inconsistent across generations, weakly documented externally, structurally correct.**

Rationale:
- + Strong: every retry is bounded by an explicit configuration constant (`summarizer_settings.max_summarizer_retries`, `part_iMessageEvictEviction` from `message_buffer_limit/min`), and the v3 loop categorically stops stepping on any unhandled exception rather than spinning (`letta/agents/letta_agent_v3.py:1519-1525`). The `StepProgression` enum and the post-step `update_step_error_async` + `update_step_stop_reason` give a stable, idempotent failure record (`letta/agents/letta_agent_v2.py:1034`, `letta/agents/letta_agent_v3.py:1297`).
- + Strong: tool-rule violations produce a structured heartbeat that is **injected into the next turn** (`letta/agents/helpers.py:501-505`, `letta/server/rest_api/utils.py:630-655`), not just logged, so the LLM sees the correction on the next pass.
- + Strong: idempotency on retry of an approval message after summarization evicted it (`letta/agents/helpers.py:230-294`, integration test `tests/integration_test_human_in_the_loop.py:1456-1568`).
- + Strong: stop-reason taxonomy is rich (13 enum members), and the streaming service guarantees terminal events so dropped streams are not mistaken for success (`letta/services/streaming_service.py:609-636`).
- - Weak: the **two coexisting agent loops** (`LettaAgent` legacy + `LettaAgentV2` + `LettaAgentV3`) implement reask/retry with overlapping-but-distinct semantics. v2 retries only on `ContextWindowExceededError`, v3 adds a LLM-router fallback for `LLMRateLimitError`/`LLMServerError`/`LLMProviderOverloaded` and a circuit-breaker success record (`letta/agents/letta_agent_v3.py:1171-1213`). Picking the wrong one in `letta.agent_loop.AgentLoopFactory` (`letta/agents/agent_loop.py:16-63`) yields different failure surfaces.
- - Weak: no dedicated **critic/reflection agent** primitive. There is no built-in "LlmAsJudge" pattern nor a loop that asks a second model whether the prior answer is good. Compaction summarizer is the only LLM-in-the-loop for self-evaluation.
- - Weak: the `summarizer_settings.max_summarizer_retries = 3` retry-on-context-overflow path is documented in a TODO comment ("can be broken by bad configs") (`letta/services/summarizer/compact.py:158-160`).
- - Weak: when the retry budget is exhausted, the v3 exception handler asserts the agent "is not in a bad state" but the only safeguard against further retries is `self.should_continue = False`; there is no per-run attempt counter visible to callers (`letta/agents/letta_agent_v3.py:1519-1521`).
- - Weak: in legacy `letta/agent.py` `_get_ai_reply`, retries use `time.sleep(delay)` with `warnings.warn` rather than `asyncio.sleep`, blocking the event loop (`letta/agent.py:354-430, 422-424`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Stop-reason taxonomy | `StopReasonType` enum mapping every failure to a typed reason and RunStatus | `letta/schemas/letta_stop_reason.py:9-49` |
| Validation error → next turn heartbeat | `[ToolConstraintError] Cannot call X, valid tools include: ...` built and persisted | `letta/agents/helpers.py:501-505` |
| Per-step retry on `ContextWindowExceededError` | `for llm_request_attempt in range(summarizer_settings.max_summarizer_retries + 1): …` | `letta/agents/letta_agent_v3.py:1093-1299`, `letta/agents/letta_agent_v2.py:518-573` |
| Retry → compact → re-issue | `if isinstance(e, ContextWindowExceededError) and llm_request_attempt < summarizer_settings.max_summarizer_retries: # Retry case` calls `self.compact(...)` then `continue` | `letta/agents/letta_agent_v3.py:1217-1284` |
| Fallback to `all` summarizer when first fails | If `sliding_window`/`self_compact_*` compactor fails, model is `model_copy(update={"mode": "all", ...})` and re-summarized | `letta/services/summarizer/compact.py:220-296` |
| Compaction trigger threshold | `trigger_threshold = int(llm_config.context_window * SUMMARIZATION_TRIGGER_MULTIPLIER)`; default `SUMMARIZATION_TRIGGER_MULTIPLIER = 0.9` | `letta/services/summarizer/thresholds.py:27-41`, `letta/constants.py:83` |
| Proactive post-step compactor | After successful step `if self.context_token_estimate > compaction_trigger_threshold: …await self.compact(..., trigger="post_step_context_check")` | `letta/agents/letta_agent_v3.py:1439-1505` |
| Hard-stop on system-prompt overflow | If compactor cannot shrink under threshold and system prompt alone exceeds context, raise `SystemPromptTokenExceededError` → `StopReasonType.context_window_overflow_in_system_prompt` | `letta/services/summarizer/compact.py:391-410`, `letta/errors.py:364-371`, `letta/schemas/letta_stop_reason.py:39-41` |
| Step-loop kill on any exception | `self.should_continue = False` and structured `update_step_error_async` | `letta/agents/letta_agent_v3.py:1511-1592`, `letta/agents/letta_agent_v2.py:648-728` |
| Tool-rule continuation reason | `_decide_continuation` builds feedback string `f"{NON_USER_MSG_PREFIX}Continuing: tool rule violation."` etc. and converts it to a heartbeat user message | `letta/agents/letta_agent_v3.py:1987-2036`, `letta/agents/letta_agent_v2.py:1250-1285`, `letta/server/rest_api/utils.py:630-655` |
| Required-before-exit enforcement | When LLM omits required tool, fed `ToolRuleViolated: You must call {uncalled} at least once to exit the loop` heartbeat | `letta/agents/letta_agent_v3.py:1622-1641`, `letta/agents/letta_agent_v2.py:1175-1197` |
| Approval rejection feedback | Denied approvals converted to `ToolExecutionResult(status="error")` + heartbeat `"Continuing: user denied request to call tool"` | `letta/agents/letta_agent_v2.py:1087-1106`, `letta/agents/letta_agent.py:1747-1754` |
| LLM-router fallback (v3 only) | On `LLMRateLimitError/LLMServerError/LLMProviderOverloaded`: record failure, switch `active_llm_config = fallback_config`, `continue` | `letta/agents/letta_agent_v3.py:1183-1213` |
| Circuit breaker success/failure hooks | `record_success`/`record_failure` available on OSS base (no-op) and Redis-backed routing client | `letta/services/llm_router/llm_router_client_base.py:42-93` |
| Approval idempotency across compaction | If `tool_call_ids` already present in recent tool-return messages, drop duplicate approval and inject keep-alive | `letta/agents/helpers.py:230-294` |
| Parallel-tool + missing-tool fallback path | If LLM returns N tool calls but `parallel_tool_calls=False`, truncate to first; if tool violates rule build `[ToolConstraintError]` and persist as a tool message | `letta/agents/letta_agent_v3.py:1336-1342`, `letta/agents/letta_agent_v3.py:1822-1827` |
| Test: approval retry after summarization evicts context | `tests/integration_test_human_in_the_loop.py:1456-1568` proves the idempotency path | `tests/integration_test_human_in_the_loop.py:1456-1568` |
| Stream terminal-event guarantee | If `[DONE]` or `event: error` not seen, emit synthetic stop_reason=error + SSE error + `[DONE]` | `letta/services/streaming_service.py:609-636` |
| Stream error categorization | Five typed SSE errors: `llm_timeout`, `llm_rate_limit`, `llm_authentication`, `llm_empty_response`, `stream_incomplete` | `letta/services/streaming_service.py:639-700` |
| Legacy empty-response retry | `empty_response_retry_limit=3` with `backoff_factor=0.5`, `max_delay=10.0`, exponential `time.sleep` | `letta/agent.py:306-442` |
| Lock-contention retries (deadlock retry) | ORM helpers retry deadlocks with `2**attempt` backoff up to `_DEADLOCK_MAX_RETRIES` | `letta/orm/sqlalchemy_base.py:591-789` |
| Mass-add approval-tool | `requires_approval` is a `ToolRuleType`; tool is forced through a structured approve/deny flow | `letta/schemas/tool_rule.py:353`, `letta/agents/letta_agent_v3.py:1697-1709` |

## Answers to Dimension Questions

1. **When does the agent self-correct?**
   - **Provider / context overflow**: When the streaming layer raises `ContextWindowExceededError` (mapped from provider errors via `letta/llm_api/error_utils.py:8-23` and `llm_api/openai_client.py:1301-1394`), the v3 step catches it inside the bounded retry loop, calls `self.compact(...)`, and re-issues the same request (`letta/agents/letta_agent_v3.py:1217-1284`).
   - **Tool-rule violation**: When the LLM proposes a tool not in the allowed set, `_build_rule_violation_result` returns a `ToolExecutionResult(status="error")` with `[ToolConstraintError] Cannot call X, valid tools include: …` (`letta/agents/helpers.py:501-505`); `_decide_continuation` flips `continue_stepping=True` and emits a heartbeat (`letta/agents/letta_agent_v3.py:1991-2036`).
   - **Required-before-exit**: When required tools haven't been called, the next turn's heartbeat says `ToolRuleViolated: You must call {names} at least once to exit the loop` (`letta/agents/letta_agent_v3.py:1622-1641`).
   - **Provider outage** (Redis-backed deployments only): `LLMRateLimitError`/`LLMServerError`/`LLMProviderOverloaded` triggers `record_failure` + `active_llm_config = fallback_config` (`letta/agents/letta_agent_v3.py:1183-1213`).
   - **Mid-stream protocol failure**: Empty/overloaded streaming yields `LLMEmptyResponseError` and surfaces `error_type=llm_empty_response` to the client (`letta/services/streaming_service.py:686-700`).
   - **Inactive approval retry**: When an approval message arrives without a matching pending request, an idempotency probe scans recent tool-return messages; if already processed the agent is restored to a no-op state with a keep-alive ping (`letta/agents/helpers.py:230-294`).

2. **Is correction bounded?**
   - Yes, on three independent axes:
     - **Per-step**: `for llm_request_attempt in range(summarizer_settings.max_summarizer_retries + 1)` plus the `except` arm's `if isinstance(e, ContextWindowExceededError) and llm_request_attempt < summarizer_settings.max_summarizer_retries` guard means at most `max_summarizer_retries + 1` attempts (default 4) before re-raising (`letta/agents/letta_agent_v3.py:1093-1299`, `letta/settings.py:96`).
     - **Per-run**: `max_steps: int = DEFAULT_MAX_STEPS` (default `50` at `letta/constants.py:75`) wrapped in `for i in range(max_steps)`; the iterator hard-stops with `StopReasonType.max_steps` on the final iteration (`letta/agents/letta_agent_v3.py:569-629`).
     - **Step-level failure**: `self.should_continue = False` set immediately on any uncaught `Exception` to prevent runaway retry chains (`letta/agents/letta_agent_v3.py:1519-1525`).
   - Edge: there is **no logged per-run attempt counter** that callers can read. The only durability point is `summarize_attempt_count` parameter threaded through `letta/agent.py:1037-1067`, and that parameter is only mutated on context overflow — not for every tool failure.

3. **What evidence is shown during correction?**
   - Tool-rule violations: structured message text including the `valid tools include: [...]` list and an optional `Hint: Possible rules that were violated: …` from `ToolRulesSolver.guess_rule_violation()` (`letta/helpers/tool_rule_solver.py:239-271`, `letta/agents/helpers.py:501-505`).
   - Compaction failures: the count of messages evicted vs. retained (`letta/system.py:207-235`); on a clean retry, a structured `SummaryMessage` carrying `compaction_stats: {trigger, context_tokens_before, context_tokens_after, ...}` is yielded (with the `SummaryMessage.message_type = "summary_message"` channel) (`letta/schemas/letta_message.py:442-450`, `letta/services/summarizer/compact.py:414-449`).
   - LLM-routing fallback: warning `[LLM ROUTER]: {handle} failed ({TypeName}), falling back to {fallback.handle}` plus an OpenTelemetry span event (`letta/agents/letta_agent_v3.py:1197-1199`).
   - Step-level failures: `update_step_error_async(error_type=type(e).__name__, error_message=str(e), error_traceback=traceback.format_exc(), stop_reason=self.stop_reason)` produces a durable audit row (`letta/agents/letta_agent_v3.py:1559-1567`).
   - Stream-level failures: typed `LettaErrorMessage(run_id, error_type, message, detail)` SSE event, followed by `[DONE]` and a `RunStatus.failed` snapshot (`letta/services/streaming_service.py:603-700`).

4. **Does correction improve outputs or hide errors?**
   - Improve: yes for compaction — `compaction_stats` is round-tripped into the conversation (`letta/system.py:233-235`), and the `SummaryMessage` carries `context_tokens_before/after` so the post-run audit can verify the compactor actually shrunk the prompt (used by `tests/integration_test_summarizer.py:1969-2008`).
   - Doesn't hide:
     - The step rollback is explicit: "NOTE: message persistence does not happen in the case of an exception (rollback to previous state)" (`letta/agents/letta_agent_v3.py:1513`).
     - Every reject path produces a `LettaStopReason` value persisted on the run (`letta/services/run_manager.py:673-775`); there is no silent path that flips `end_turn` after a failure.
     - Streaming client always gets a terminal marker: even when the inner loop dies, the wrapper synthesizes `stop_reason=error` + `error_type=stream_incomplete` rather than closing cleanly (`letta/services/streaming_service.py:609-636`).
   - Hides: the legacy `letta/agent.py` `_get_ai_reply` retries on `ValueError` only (`finish_reason` bad and empty-choice cases) and uses `time.sleep` + `warnings.warn` not surfaced as `StopReasonType`; that path mutates `agent_state` only after success (`letta/agent.py:300-442, 1022-1024`).

5. **Are repeated failures escalated?**
   - **Up to caller**: When the retry budget is exhausted, the exception bubbles up to the streaming service, which translates `LLMTimeoutError`, `LLMRateLimitError`, `LLMAuthenticationError`, `LLMEmptyResponseError` into typed SSE error events and writes a `RunUpdate(status=RunStatus.failed, stop_reason=StopReasonType.llm_api_error, metadata={"error":...})` (`letta/services/streaming_service.py:639-700`).
   - **Human-in-the-loop**: `requires_approval` tool-rule forces a structured approval request and the agent loop stops with `StopReasonType.requires_approval`; the user must re-issue an `ApprovalCreate` payload, validated by `validate_approval_tool_call_ids` (`letta/agents/helpers.py:121-145`, `letta/agents/letta_agent_v3.py:1697-1709`, `letta/schemas/letta_stop_reason.py:21, 30`).
   - **Credits**: `StopReasonType.insufficient_credits` from `_check_credits()` (`letta/agents/letta_agent_v2.py:738-747`, `letta/agents/letta_agent_v3.py:626-579`) is `RunStatus.failed` and stops the loop without retrying (`letta/schemas/letta_stop_reason.py:46-47`).
   - **Cancellation**: `_check_run_cancellation(run_id)` returns `RunStatus.cancelled` and stops without raising (`letta/agents/letta_agent_v2.py:750-757`, `letta/agents/letta_agent_v3.py:1031-1035`).
   - **Not covered**: there is no automatic escalation beyond `max_steps` exhaustion (no paging, no alert, no `RunUpdate` of severity). The `event_loop_watchdog` is a process-level watchdog, not an agent loop one (`letta/monitoring/event_loop_watchdog.py:192-207`).

## Architectural Decisions

- **Compaction, not replanning**: Self-correction in Letta targets *context pressure*, not *output quality*. The "reflection" is fundamentally an LLM-driven compression of preceding turns, not a critic pass on the assistant's last answer. Trade-off: this is robust for unbounded conversations but does not catch reasoning mistakes that the compactor's summary also inherits.
- **Two-loop split (`LettaAgent` legacy vs `LettaAgentV2/V3`)**: The legacy path exists for direct chat completion in `letta/agent.py` and re-implements retry, summarization, and tool-rule handling in a single class (~1758 LOC). The v3 loop (`letta_agent_v3.py`, 2134 LOC) decomposes the same concerns into `_step` / `_handle_ai_response` / `_decide_continuation` / `compact`, separated per concern. `AgentLoopFactory` (`letta/agents/agent_loop.py:16-63`) routes by `AgentType` and `enable_sleeptime`, so different agents get different self-correction surfaces.
- **Stop-reason first-class enum**: Every retry/decide branch sets a typed `LettaStopReason`, and `RunStatus` is derived (`completed | failed | cancelled`) rather than tracked separately (`letta/schemas/letta_stop_reason.py:24-49`). This guarantees no codepath can omit a terminal reason — including the streaming service's synthetic `error` injection.
- **Tool rules as state machine**: `ToolRulesSolver` is the authority on what tools are legal at any moment given `tool_call_history` and the last `function_response` (`letta/helpers/tool_rule_solver.py:46-271`). The loop never has to ask the LLM "what should I do next" — `_decide_continuation` (`letta/agents/letta_agent_v3.py:1967-2036`) is deterministic and produces the heartbeat text the LLM will see.
- **Step roll-back via `StepProgression`**: The six-state enum (`START | STEP_LOGGED | LOGGED_TRACE | STREAM_RECEIVED | RESPONSE_RECEIVED | FINISHED`) gives the `finally` block enough context to write the right telemetry row without needing a try/retry tree (`letta/schemas/step.py:68`, `letta/agents/letta_agent_v3.py:1511-1592`).
- **Idempotent approvals across compaction**: When summarization evicts the message containing the approval request, the next approval payload is intercepted by `validate_persisted_tool_call_ids` and short-circuited to a keep-alive turn (`letta/agents/helpers.py:230-294`). This keeps the resume flow from re-running the tool or hallucinating state.
- **Per-mode fallbacks for compaction**: Four compaction modes (`all | sliding_window | self_compact_all | self_compact_sliding_window`) with chained fallback (`self_compact_*` → `all` on exception; `sliding_window` mode retry after insufficient shrink) (`letta/services/summarizer/compact.py:220-413`). The cascading design means a single bad prompt doesn't permanently brick the agent — only a fully-stuck `sliding_window+all` pair escalates to `SystemPromptTokenExceededError`.

## Notable Patterns

- **Heartbeat user-message injection**: Tool-rule, denial, and required-tool feedback are encoded as a `Message(role=MessageRole.user, content=[TextContent(text=get_heartbeat(timezone, reason))])` carrying a `[This is an automated system message hidden from the user]` prefix (`letta/constants.py:244`, `letta/server/rest_api/utils.py:630-655`). The model itself treats the heartbeat as a tool return, which is simple but couples correctness to the model's prompt-side "ignore instructions" compliance.
- **Step-ID binding to messages**: Every `Message` carries `step_id`/`run_id`/`agent_id` so debugging and rollback can be re-keyed (`letta/schemas/message.py:137-143`, `letta/agents/letta_agent_v3.py:937-1042`).
- **`summarize_attempt_count`-style recursion**: The legacy path threads an explicit counter through the recursive `inner_step` call when context overflows, terminating with `ContextWindowExceededError` only after `summarizer_settings.max_summarizer_retries` (`letta/agent.py:1025-1067`). v3 drops that recursion in favour of an inline loop.
- **Streaming edge handling** (downstream of the agent loop): `StreamingService` translates inner-stop reasons into SSE, never lets the client sit waiting, and includes a `stream_incomplete` fallback for missing terminals (`letta/services/streaming_service.py:600-700`).

## Tradeoffs

- **Compaction can lose signal**: If the compactor emits a summary that drops a key detail, the next turn never recovers it. There is no critic that detects "the compactor just lied" — only another compactor attempt (`letta/services/summarizer/compact.py:391-413`).
- **Reask without reflection**: The system reissues prompts after `ContextWindowExceededError`, but doesn't change prompt content beyond compaction. There is no analog of `letta = retry + small-talk-with-the-LLM-about-its-mistake`. When the LLM produces a bad answer, the only recovery is user re-prompt.
- **Two-loop drift**: Different retry semantics per loop class make operational metrics tricky. For example, v3 records `compaction_trigger_threshold` reactive triggers, v2 only fires on a 400-style `ContextWindowExceededError` thrown by the LLM client.
- **OSS has no auto-mode fallback**: `resolve_auto_mode_config` raises `RuntimeError` unless Redis is configured (`letta/services/llm_router/llm_router_client_base.py:36-39`). Self-hosted installs get only the per-step compaction/retry path, not the circuit-breaker fallback path.
- **Stop-reason explosion**: 13 enum members plus an unmatched `raise ValueError("Unknown StopReasonType")` in the run-status mapping (`letta/schemas/letta_stop_reason.py:48`) means a new reason can silently break the mapping until the dict is updated.
- **Legacy retry uses `time.sleep`**: `letta/agent.py:354-424` blocks the event loop on retry; if a heavy agent is asked to do this from inside `step()` it can stall the server thread.

## Failure Modes / Edge Cases

- **Compactor self-defense**: `letta/services/summarizer/compact.py:391-413` raises `SystemPromptTokenExceededError` if even after fallback the system prompt alone exceeds context window. The run becomes `RunStatus.failed` with `context_window_overflow_in_system_prompt`.
- **Approval without pending request**: After eviction, a stale approval raises `ValueError("Cannot process approval response: No tool call is currently awaiting approval…")` unless the idempotency check finds the matching tool return (`letta/agents/helpers.py:287-293`).
- **Empty LLM response**: `LLMEmptyResponseError` (a subclass of `LLMServerError`) forces an `invalid_llm_response` stop with `error_type=llm_empty_response` (`letta/errors.py:300-306`, `letta/services/streaming_service.py:686-700`).
- **Streaming interruption without terminal**: `streaming_service.py:609-636` synthesizes an `error` event so the client never hangs on an unterminated stream.
- **Permanent failure on LLM unprocessable entity**: `LLMUnprocessableEntityError` (422) is not retried — it bubbles up as `letta/llm_api/openai_client.py:1380-1387`.
- **Credit exhaustion**: `StopReasonType.insufficient_credits` stops the loop *before* `max_steps` and propagates as `RunStatus.failed` (`letta/schemas/letta_stop_reason.py:46-47`, `letta/agents/letta_agent_v2.py:743-747`).
- **Parallel-tool + tool-rule conflict**: If a tool-rule requires `parallel_tool_calls=False` and the LLM ignores it, v3 truncates to the first call (`letta/agents/letta_agent_v3.py:1336-1342`) and persists the rules-violation result for the dropped call as a `ToolConstraintError`. No retry is triggered on this — the call is just lost.
- **Prefilled-args schema mismatch**: `merge_and_validate_prefilled_args` raises `ValueError` which is caught and surfaced as an `error` ToolExecutionResult (`letta/agents/helpers.py:465-493`, `letta/agents/letta_agent_v3.py:1789-1809`).
- **Misbehaving parallel-tool JSON**: `_safe_load_tool_call_str` shaves `}{` to take only the first JSON object (`letta/agents/helpers.py:378-393`); subsequent calls are silently dropped — no retry, no warning surfaced to the model.

## Future Considerations

- **`LoopEvaluator` / critic agent**: A primary self-improvement target would be a built-in `Evaluator` interface that runs after each `_step` and either continues the loop (with feedback) or ends it, similar to agent-framework's `LoopAgent`. The current `compaction_trigger_threshold` mechanism reacts to *token* pressure, not *answer quality* pressure.
- **Per-run attempt counter**: The retry budget is in-band; making `summarize_attempt_count` and `llm_request_attempt` durable on the `Run`/`Step` rows (currently only `RunStatus` and `StopReasonType` are stored) would let dashboards see "this run took 4 retries" without reading logs.
- **Unify agent loops**: `LettaAgent`, `LettaAgentV2`, and `LettaAgentV3` have materially different retry semantics. Consolidating behind a single class with strategy switches would reduce "did I get the right loop?" risk surfaced by `AgentLoopFactory` (`letta/agents/agent_loop.py:16-63`).
- **Streaming graceful close on cancellation**: The current cancellation path emits stop_reason=`cancelled` mid-stream without resetting partial-message state, which can leave partially streamed tool-call messages. A `cancel_checkpoint` analogue to `_step_checkpoint_finish` would let the next turn resume more cleanly.
- **Compactor quality controls**: A simple validity check (e.g., does the compaction preserve every required tool-call reference?) would convert today's "compactor fails to shrink"→silent retry into a more principled "compactor loses data"→escalation path.
- **Pluggable fallback routes for OSS**: Right now the circuit-breaker is gated behind Redis; an OSS-default strategy (e.g., try once, switch to a static fallback model) would let self-hosted users get partial protection.

## Questions / Gaps

- Is there a deliberate decision **not** to expose an LLM-as-judge primitive in OSS, given the heavy investment in tool-rule solvers but absence of `Evaluator`-style classes? No evidence found in `letta/agents/` or `letta/services/summarizer/` for "judge", "critic", "evaluator", "reflection".
- Why are there **two parallel `max_summarizer_retries` and `max_summarization_retries`** settings, with v2 using `max_summarizer_retries` and v1/`StepStreamNoTokens` using `max_summarization_retries`? No migration note found.
- What is the intended user-visible signal after a retry exhausts the budget? `streaming_service.py:639-700` emits `error_type=llm_rate_limit`, but no `attempts` count or `agent_attempt_count` is exposed in the metadata.
- The `helpers.py:230-294` approval idempotency check asks the database once for the most-recent tool-return message, but only when `len(non_system_summary_messages) == 0`. What's the behaviour for partial eviction (e.g., 1 user message survives)? No evidence found in `letta/agents/helpers.py` covering this boundary case explicitly.
- `agent.py:1037-1067` re-throws the `ContextWindowExceededError` once the cap is exhausted, but the embedded `details` ("num_in_context_messages", "in_context_messages_text", "token_counts") could grow unbounded if the agent history is large — is there a size cap before persisting? No evidence found in the on-call schema or the log handler.
- The `letta` package exposes `non_system_summary_messages` filter only in `agents/helpers.py:249-250`; parallel-agent (`sleeptime_*`) loops have their own buffered `_step` methods. Whether the approval idempotency check is consistently applied across `SleeptimeMultiAgentV3`/`V4`/`sleeptime_agent` was not verifiable without reading each group's `_step` (`letta/groups/sleeptime_multi_agent_v3.py`, `letta/groups/sleeptime_multi_agent_v4.py`).

---

Generated by `studies/agent-harness-study/reports/source/03.05-reflection-reask-self-correction/letta.md` against `letta`.

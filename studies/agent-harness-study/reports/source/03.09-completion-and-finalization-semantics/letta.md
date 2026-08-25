# Source Analysis: letta

## Completion and Finalization Semantics

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI, Pydantic, SQLAlchemy, Redis streaming) |
| Analyzed | 2026-07-17 |

## Summary

Letta has a richly modeled completion story organized around three orthogonal dimensions: (1) an in-loop `StopReasonType` enum on each step/response, (2) a run-level `RunStatus` lifecycle that maps every `StopReasonType` to a terminal `RunStatus`, and (3) a step-failure `StepStatus` set on each `Step` row. A run's `stop_reason` and `completed_at` are persisted in `Run` (`letta/schemas/run.py:14-58`), and tool-call denials, missing required tools, and `tool_rule` (terminal / continue / required-before-exit) semantics gate whether the agent can declare done (`letta/agents/letta_agent_v3.py:1622-2052`). Usage is finalized both per-step (`Step.completion_tokens`, `Step.prompt_tokens`, `Step.cached_input_tokens`, etc.) and aggregated at run level through `RunManager.get_run_usage` (`letta/services/run_manager.py:512-540`). Finalization can fire an optional HTTP `callback_url` with the full `LettaResponse` payload (`letta/services/run_manager.py:484-510`), and the Redis-backed stream manager synthesizes a terminal `[DONE]` marker if a stream ends without one (`letta/server/rest_api/redis_stream_manager.py:283-315`). The same code path also exposes a "stream_incomplete" error so a run that disappears mid-flight is never silently left in a non-terminal state.

## Rating

**8 / 10 (Clear model with tests, explicit interfaces, and operational safeguards — but a few rough edges around validation, "warning" completions, and warning coverage.)**

Rationale: there is a strong, deduplicated stop-reason enum mapped bi-directionally to `RunStatus` (`letta/schemas/letta_stop_reason.py:9-49`) plus extensive test coverage for terminal updates (`tests/managers/test_run_manager.py:160-723`, `tests/test_run_status_conversion.py`). Per-step token totals + a final usage rollup (`letta/services/run_manager.py:512-540`) and nanosecond-level timing (`letta/schemas/run_metrics.py:7-19`, `letta/schemas/step_metrics.py:9-21`) keep observability excellent. Mid-flight cancellation is a first-class path with idempotent semantics (`letta/services/run_manager.py:649-781`). However, there is no generic "final-output validator" — content validation is delegated to the LLM adapter / `validate_function_response` (`letta/utils.py:898-940`) and to user-supplied `response_format`, and there is no "completed-with-warnings" status distinct from `completed`. Lifecycle enforcement is logging-only (not blocking) for already-terminal runs (`letta/services/run_manager.py:342-356`).

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Final output schema | `LettaResponse(messages, stop_reason, usage, logprobs?, turns?)` | `letta/schemas/letta_response.py:55-86` |
| Streaming output schema | `LettaStreamingResponse` root model with discriminated `message_type` union (SystemMessage, ..., LettaStopReason, LettaUsageStatistics, LettaErrorMessage) | `letta/schemas/letta_response.py:147-175` |
| Run lifecycle schema | `Run(status: RunStatus, completed_at, stop_reason, callback_url, callback_sent_at, callback_status_code, callback_error, ttft_ns, total_duration_ns)` | `letta/schemas/run.py:14-58` |
| Run update payload | `RunUpdate(status?, completed_at?, stop_reason?, metadata?, total_duration_ns?)` | `letta/schemas/run.py:62-72` |
| StopReasonType enum | 12 reasons: `end_turn`, `error`, `llm_api_error`, `invalid_llm_response`, `invalid_tool_call`, `max_steps`, `max_tokens_exceeded`, `no_tool_call`, `tool_rule`, `cancelled`, `insufficient_credits`, `requires_approval`, `context_window_overflow_in_system_prompt` | `letta/schemas/letta_stop_reason.py:9-22` |
| StopReason → RunStatus mapping | `.run_status` property maps `end_turn|max_steps|tool_rule|requires_approval → completed`, `error|...|invalid_*|context_window_overflow → failed`, `cancelled → cancelled`, `insufficient_credits → failed` | `letta/schemas/letta_stop_reason.py:24-49` |
| LettaStopReason SSE envelope | `{message_type: "stop_reason", stop_reason: StopReasonType}` | `letta/schemas/letta_stop_reason.py:52-58` |
| RunStatus enum | `created`, `running`, `completed`, `failed`, `cancelled` | `letta/schemas/enums.py:158-164` |
| StepStatus enum | `PENDING`, `SUCCESS`, `FAILED`, `CANCELLED` | `letta/schemas/enums.py:225-230` |
| Step.record statuses | Per-step `status=StepStatus.PENDING` on creation (v2) and `step_manager.update_step_*` mutators | `letta/agents/letta_agent_v2.py:941-966`, `letta/services/step_manager.py:402-547` |
| StepProgression state machine | `START → STREAM_RECEIVED → RESPONSE_RECEIVED → STEP_LOGGED → LOGGED_TRACE → FINISHED` used to decide what to rollback / finalize | `letta/schemas/step.py:55-62` |
| Usage schema | `LettaUsageStatistics(completion_tokens, prompt_tokens, total_tokens, step_count, run_ids?, cached_input_tokens?, cache_write_tokens?, reasoning_tokens?, context_tokens?)` | `letta/schemas/usage.py:79-118` |
| RunMetrics (timing) | `run_start_ns`, `run_ns`, `num_steps`, `tools_used`, `template_id`, `project_id`, `base_template_id` | `letta/schemas/run_metrics.py:11-19` |
| StepMetrics (timing) | `step_start_ns`, `llm_request_start_ns`, `llm_request_ns`, `tool_execution_ns`, `step_ns` | `letta/schemas/step_metrics.py:11-19` |
| `max_steps` default | `DEFAULT_MAX_STEPS = 50` and router default `max_steps: int = Query(100, deprecated=True)` | `letta/constants.py:75`, `letta/server/rest_api/routers/v1/agents.py:298` |
| `max_steps` hard stop | After loop: `if i == max_steps - 1 and self.stop_reason is None: self.stop_reason = LettaStopReason(stop_reason=StopReasonType.max_steps.value)` | `letta/agents/letta_agent_v3.py:628-629`, `letta/agents/letta_agent_v3.py:394-395` |
| Default `end_turn` finalization | `if self.stop_reason is None: self.stop_reason = LettaStopReason(stop_reason=StopReasonType.end_turn.value)` | `letta/agents/letta_agent_v3.py:412-413`, `letta/agents/letta_agent_v3.py:646-647` |
| Continuation policy | `_decide_continuation(...)` — no tool call + uncalled required tools + not final step → continue w/heartbeat; otherwise `end_turn`/`max_steps`/`tool_rule` | `letta/agents/letta_agent_v3.py:1971-2052` |
| Tool-rule completion gate | `ToolRulesSolver.is_terminal_tool` ⇒ `StopReasonType.tool_rule` + stop stepping | `letta/agents/letta_agent_v3.py:2010-2019`, `letta/helpers/tool_rule_solver.py:174-186` |
| Required-before-exit tool rules | `RequiredBeforeExitToolRule` + `get_uncalled_required_tools(...)` blocks end-of-loop unless called | `letta/schemas/tool_rule.py:298-312`, `letta/helpers/tool_rule_solver.py:198-223`, `letta/agents/letta_agent_v3.py:1622-1637` |
| Approval as terminal state | Requires-approval tool → loop returns `StopReasonType.requires_approval` (mapped to `RunStatus.completed`) with approval messages persisted | `letta/agents/letta_agent_v3.py:1610-1618`, `letta/schemas/letta_stop_reason.py:29-32` |
| Parallel tool + finalization | For multi-tool calls force `aggregate_continue=True` unless a terminal tool was called or `max_steps` | `letta/agents/letta_agent_v3.py:1952-1962` |
| Mid-stream error handling | `try/except` in `stream` classifies `SystemPromptTokenExceededError → context_window_overflow_in_system_prompt`, `LLMError → llm_api_error`, else `error`; yields `LettaErrorMessage` then returns early (no `end_turn` finish chunks) | `letta/agents/letta_agent_v3.py:672-736` |
| Cleanup-finally error path | Errors during finalization are caught and a `cleanup_error` `LettaErrorMessage` is emitted | `letta/agents/letta_agent_v3.py:717-732` |
| Stream final chunk contract | `get_finish_chunks_for_stream` emits `[stop_reason, usage_statistics, "[DONE]"]` | `letta/agents/base_agent.py:188-195` |
| Run usage aggregation | `get_run_usage` sums `prompt/completion/total` across steps and uses `normalize_cache_tokens` / `normalize_reasoning_tokens` | `letta/services/run_manager.py:512-540` |
| Run num-steps + tools_used | On every update, count steps and scan messages for distinct tool IDs | `letta/services/run_manager.py:413-445` |
| Run finalization hygiene | On terminal update: missing `stop_reason` is logged as error; missing `completed_at` is auto-stamped at `get_utc_time()` | `letta/services/run_manager.py:358-364` |
| Lifecycle guardrails | Already-`completed` runs only transition to `cancelled` with `StopReasonType.requires_approval`; updates to terminal runs are logged as errors (not blocked) | `letta/services/run_manager.py:341-356` |
| Agent-side `last_stop_reason` | Successful terminal run updates `agent.last_stop_reason` via `UpdateAgent` | `letta/services/run_manager.py:398-410` |
| Run callback dispatch | If terminal update is the first completion AND `callback_url` is set, POST `{run_id, status, completed_at, metadata}` with the `LettaResponse` embedded in `metadata.result` | `letta/services/run_manager.py:332-339`, `letta/services/run_manager.py:451-477`, `letta/services/run_manager.py:484-510` |
| Callback idempotency | Belt-and-suspenders test: non-terminal `running` update must not set `completed_at` or dispatch; the subsequent `completed` call dispatches exactly once | `tests/managers/test_run_manager.py:655-723` |
| Cancellation-aware stream | `cancellation_aware_stream_wrapper` polls Redis-backed run status, injects `stop_reason=cancelled` and `RunCancelledException` into the generator | `letta/server/rest_api/streaming_response.py:160-229` |
| Cancel run | `RunManager.cancel_run` is idempotent (no-op if already terminated except for `requires_approval`); rolls back any pending tool-call approvals | `letta/services/run_manager.py:618-781` |
| Streaming Redis manager — synthesize terminal | If a stream ends without `[DONE]` and without `error_event`, write a synthesized `[DONE]` or a `stream_incomplete` error | `letta/server/rest_api/redis_stream_manager.py:283-315` |
| Streaming Redis manager — task cancellation classification | Distinguish user-cancelled (`status=cancelled`) from infrastructure `asyncio.CancelledError` → `stream_task_cancelled` | `letta/server/rest_api/redis_stream_manager.py:324-377` |
| Streaming run status mapping | Use `StopReasonType(stop_reason).run_status` as canonical mapper; default unknown values to `completed` with warning | `letta/server/rest_api/redis_stream_manager.py:419-445` |
| Background task completion path | `_process_message_background` calls `AgentLoop.load(...).step(...)`, derives `RunStatus` from `result.stop_reason` (cancelled → cancelled, anything else → completed), persists via `update_run_by_id_async`; errors fall through to `failed / StopReasonType.error` | `letta/server/rest_api/routers/v1/agents.py:2062-2160` |
| Per-step finalization | `update_step_success_async` (sets `StepStatus.SUCCESS` + per-step usage + cached/reasoning tokens) and `update_step_error_async` (sets `FAILED` + `error_type/error_data`) | `letta/services/step_manager.py:368-414`, `letta/services/step_manager.py:416-476`, `letta/services/step_manager.py:516-555` |
| `track_stop_reason` setting | `settings.track_stop_reason: bool = True` controls whether `_log_request` is invoked on failure paths | `letta/settings.py:385`, `letta/agents/letta_agent_v3.py:1578-1586` |
| Output validator | `validate_function_response` truncates tool returns to `return_char_limit`, raises on `strict=True`; per-step LLM response parsing in `_step` raises `ValueError/LLMEmptyResponseError` → `StopReasonType.invalid_llm_response` | `letta/utils.py:898-940`, `letta/agents/letta_agent_v3.py:1167-1183` |
| Approval tool | Per-step return-truncation cap (`5000` chars) applied to client-side tool returns | `letta/agents/letta_agent_v3.py:1600-1649` |
| Provider response finish_reason | Adapter exposes `finish_reason` → mapped to `StopReasonType.max_tokens_exceeded` for `"length"`, otherwise `end_turn` | `letta/adapters/letta_llm_adapter.py:91-103`, `letta/agents/letta_agent_v3.py:1989-1998` |
| Streaming incomplete_details | OpenAI Responses adapter converts `incomplete_details.reason` into `_finish_reason` (`"length"`, `"content_filter"`, unknown passthrough) | `letta/adapters/simple_llm_stream_adapter.py:237-251` |
| Run completion property test | `wait_for_run_completion` polls `client.runs.retrieve(run_id)` until `status == "completed"` (treats `"failed"` as a hard error) | `tests/integration_test_send_message.py:1853-1864` |
| Run callback E2E test | Asserts callback receives `run_id`, `status=="completed"`, `completed_at`, `metadata.result.messages` matching `Run.messages` | `tests/integration_test_send_message.py:2085-2154` |
| Stop-reason routing on cancel | `test_stop_reason_set_correctly_on_cancellation` ensures `stop_reason != "end_turn"` and equals `"cancelled"` after cancellation | `tests/managers/test_cancellation.py:660-695` |
| Stop-reason enum conversion test | 10-case `convert_statuses_to_enum` test (case-sensitive, dedupe, ordering, `ValueError` on unknown) | `tests/test_run_status_conversion.py` |
| Step failure recording on partial | When `step_progression < FINISHED` and the step fails, compute `step_ns = get_utc_timestamp_ns() - step_metrics.step_start_ns` and record partial metrics | `letta/agents/letta_agent_v3.py:1549-1592` |
| Inflight tool-call denial | Tool-rule violation creates `ToolReturnMessage` with status=`error`, sets `continue=True`, stop_reason=None | `letta/agents/letta_agent_v3.py:1692-1718` |
| Conditional heartbeat | `tool_rule_violated` ⇒ heartbeat continues; `uncalled required tools` ⇒ continues; `_decide_continuation` returns `(continue_stepping, heartbeat_reason, stop_reason)` | `letta/agents/letta_agent_v3.py:2000-2049` |

## Answers to Dimension Questions

1. **What counts as done?**
   A response is terminal when `LettaStopReason.stop_reason` is in `{end_turn, max_steps, tool_rule, requires_approval}` (mapped to `RunStatus.completed`) **or** `{cancelled}` (mapped to `RunStatus.cancelled`) **or** any error category mapped to `RunStatus.failed` (`letta/schemas/letta_stop_reason.py:24-49`). At the loop level, the loop ends when `not self.should_continue` after `_handle_ai_response` returns; if no reason was set during the loop, `LettaAgentV3.step` defaults to `end_turn` (`letta/agents/letta_agent_v3.py:412-413`, `letta/agents/letta_agent_v3.py:646-647`). The agent-side rule "no tool called" → `end_turn` is overridden by tool rules: if any `required_before_exit` tool has not been called and we're not on the final step, the loop continues with a heartbeat (`letta/agents/letta_agent_v3.py:1989-2002`).

2. **Can the model falsely declare done?**
   No, not in any meaningful sense. The loop does not trust a model-emitted "done" signal — completion is computed from (a) tool-call presence + tool-rule state, (b) `max_steps` counter, (c) cancellation flag, (d) LLM errors. The model's choice of `finish_reason` only enters through `_decide_continuation` which uses `finish_reason == "length"` to map to `StopReasonType.max_tokens_exceeded` (`letta/agents/letta_agent_v3.py:1989-1998`); all other paths use `end_turn` regardless of what the model said. Token limits cannot mark done — they only set `StopReasonType.max_tokens_exceeded` which is mapped to `failed`, not `completed` (`letta/schemas/letta_stop_reason.py:39-43`).

3. **Are final outputs validated?**
   Validation is **partial and tool/format scoped**, not a generic "did we succeed" check. (a) Tool returns are coerced and optionally truncated via `validate_function_response(..., return_char_limit)` (`letta/utils.py:898-940`). (b) LLM responses are validated inside `_step`: empty responses and `ValueError` map to `StopReasonType.invalid_llm_response`; exceptions thrown by the LLM client map to `StopReasonType.llm_api_error`; cancellation maps to `cancelled` (`letta/agents/letta_agent_v3.py:1156-1183`). (c) `response_format` is enforced upstream by the LLM provider via `runtime_override_tool_json_schema` (`letta/agents/letta_agent_v3.py:2056-2068`). There is no schema validator on the final `LettaResponse.messages` itself — content correctness is the LLM's responsibility.

4. **Is usage/cost finalized?**
   Yes, in two layers. (a) Per-step: each `Step` row stores `prompt_tokens/completion_tokens/total_tokens/cached_input_tokens/cache_write_tokens/reasoning_tokens` via `step_manager.update_step_success_async` (`letta/services/step_manager.py:416-476`). (b) Per-run: `RunManager.get_run_usage` aggregates across steps and uses normalized cache/reasoning helpers (`letta/services/run_manager.py:512-540`). `RunMetrics.run_ns` is computed either from the caller-supplied `total_duration_ns` or `now() - run_start_ns` (`letta/services/run_manager.py:434-445`). On terminal updates, missing `total_duration_ns` falls back to wall-clock timing. There is no separate "cost" field — cost is derived from `usage` and provider pricing.

5. **Can a run complete with warnings?**
   No dedicated status. A run reaches `RunStatus.completed` regardless of internal warnings (truncations, retried LLM calls). Warnings live in `Run.metadata["error"]` (e.g. set in `_process_message_background` on `Exception` → `RunUpdate(metadata={"error": str(e)})`, `letta/server/rest_api/routers/v1/agents.py:2153-2157`). The run lifecycle guardrail *logs but does not block* transitions into already-terminal states (`letta/services/run_manager.py:341-356`). Compaction metadata is preserved via `message.metadata.compaction_stats` (`letta/schemas/letta_message.py:410-422`), not a status field. LLM retries triggered by `LLMRateLimitError`, `LLMServerError`, or `LLMProviderOverloaded` will silently switch to a fallback route before failing (`letta/agents/letta_agent_v3.py:1185-1213`).

## Architectural Decisions

- **Two-level completion model.** `StopReasonType` (granular, 12 values, exposed in every `LettaResponse` and SSE stream) is mapped bidirectionally to `RunStatus` (5 values for persistence and observability) via the `StopReasonType.run_status` property (`letta/schemas/letta_stop_reason.py:24-49`). Streams emit `StopReasonType`; the database stores `RunStatus`. Redis stream manager maps back via `StopReasonType(...).run_status` at the end (`letta/server/rest_api/redis_stream_manager.py:430-445`).
- **StepProgression state machine for atomicity.** Each `_step` tracks START → STREAM_RECEIVED → RESPONSE_RECEIVED → STEP_LOGGED → LOGGED_TRACE → FINISHED and decides rollback vs. finalize based on where the failure happened (`letta/schemas/step.py:55-62`, `letta/agents/letta_agent_v3.py:1537-1593`).
- **Run is durable; Step is best-effort during exception.** If `_step` raises, message persistence is intentionally **not** performed (rollback semantics), but metrics and stop-reason are still recorded (`letta/agents/letta_agent_v3.py:1511-1517`).
- **Callback is part of the terminal transition.** `RunUpdate` to a terminal `RunStatus` is the single trigger for `_dispatch_callback_async` (`letta/services/run_manager.py:332-339`, `letta/services/run_manager.py:451-477`). Non-terminal updates (e.g. `running`) never set `completed_at` or fire the callback — enforced by `not_completed_before and is_terminal_update` (`letta/services/run_manager.py:335-339`, regression test in `tests/managers/test_run_manager.py:655-723`).
- **Stop-reason–first, status-second.** `_process_message_background` decodes the termination state from `result.stop_reason` (`if stop_reason == "cancelled": cancelled, else: completed`), so a `requires_approval` run is treated as `completed` and remains resumable on the next call (`letta/server/rest_api/routers/v1/agents.py:2112-2130`).
- **Mid-stream vs. pre-chunk error handling.** Errors before any chunk is sent are re-raised; errors after a chunk is sent emit an `event: error` SSE and return early without `end_turn` finish chunks — preventing the `end_turn` mask over a real failure (`letta/agents/letta_agent_v3.py:691-733`).
- **Two complementary timer sources.** Request-level timing lives in `Run.ttft_ns`/`total_duration_ns` (set in update payloads); step-level timing lives in `StepMetrics.step_start_ns/step_ns` (recorded by `_record_step_metrics`). Both are persisted independently.
- **Stream synthesis for hung streams.** The Redis SSE layer *invents* a terminal event when one never arrives, treating a missing terminal marker as a `stream_incomplete` failure (`letta/server/rest_api/redis_stream_manager.py:283-315`). This is a deliberate safety net against silently incomplete background jobs.

## Notable Patterns

- **Decoupled stop-reason enum and `run_status` derivation** — one source of truth, no parallel mapping tables (`letta/schemas/letta_stop_reason.py:24-49`).
- **Discriminator-driven streaming schema** — `LettaStreamingResponse` is a RootModel union keyed on `message_type` so clients can `match` on type (`letta/schemas/letta_response.py:147-175`).
- **Idempotent callbacks via "only-on-terminal" gating**, plus belt-and-suspenders `[DONE]` appending (`letta/services/run_manager.py:335-339`, `letta/server/rest_api/redis_stream_manager.py:447-449`).
- **Cancellation as a co-routine-level concern** — `RunCancelledException` is injected into the inner generator via `athrow` so the generator's own `except` blocks can finalize cleanly (`letta/server/rest_api/streaming_response.py:201-206`).
- **Auto-mode rerouting with persisted resolved model** — the resolved model is written to `step_manager.update_step_resolved_model_async` in a `finally` so billing gets the right rates even if resolution fails partway (`letta/agents/letta_agent_v3.py:1057-1081`).
- **Parallel tool-call continuation forced** — calling more than one tool in a step forces `aggregate_continue=True` to let the agent process the results, unless a terminal tool was called or the loop is at `max_steps` (`letta/agents/letta_agent_v3.py:1952-1962`).
- **Compaction as a first-class step event** — `EventMessage(event_type="compaction", ...)` is yielded before compaction, and a `SummaryMessage` is yielded after, so clients can render progress (`letta/agents/letta_agent_v3.py:769-794`, `letta/agents/letta_agent_v3.py:1303-1339`).
- **Token error classification ladder** — `_decide_continuation` maps `finish_reason="length"` to `max_tokens_exceeded` (mapped to `failed`), all other finish reasons to `end_turn` (`letta/agents/letta_agent_v3.py:1989-1998`).

## Tradeoffs

- **No dedicated "completed-with-warnings" status.** Truncations, retries, and validation warnings are not surfaced as a terminal-state modifier; only `metadata["error"]` is persisted for soft failures (`letta/server/rest_api/routers/v1/agents.py:2153-2157`). An opaque `completed` may mask non-fatal issues.
- **Lifecycle guards are logs only.** Re-updating an already-terminal run logs an `error` but mutates the row anyway (`letta/services/run_manager.py:341-356`). This keeps tooling flexible but lets clients corrupt history if they retry.
- **Reliance on the LLM adapter's `finish_reason` mapping.** `_decide_continuation` only special-cases `"length"`; other provider-specific termination reasons fall through to `end_turn`, which can misclassify runs hit by content filters or tool-call validation errors (`letta/agents/letta_agent_v3.py:1989-1998`, mitigated by `simple_llm_stream_adapter.py:237-251`).
- **Callback POST is fire-and-forget at 5-second timeout.** A callback failure does **not** roll back the terminal state — stored in `callback_error` and `callback_status_code` (`letta/services/run_manager.py:498-509`).
- **Streaming finalization is string-sniffed from SSE chunks** (`letta/server/rest_api/redis_stream_manager.py:244-281`) — robust to most streams but slightly fragile vs. structured parsing.
- **Run `get_run_usage` reads every step's rows per call.** No memoization of accumulated token totals; the aggregation cost grows linearly with steps (`letta/services/run_manager.py:512-540`).
- **`track_stop_reason` flag exists but is hardcoded `True`** (no observability toggle observed in tests; setting exists for configuration symmetry, `letta/settings.py:385`).

## Failure Modes / Edge Cases

| Edge case | Handling | File:Line |
|-----------|----------|-----------|
| Stream ends before any chunk | `if first_chunk: raise` so callers see real exception | `letta/agents/letta_agent_v3.py:691-694` |
| Stream ends mid-stream | Emit `event: error` with `LettaErrorMessage` then return early (no `end_turn`) | `letta/agents/letta_agent_v3.py:703-732` |
| Cleanup error during finalization | Catch, set `error` `StopReasonType`, emit dedicated `cleanup_error` event | `letta/agents/letta_agent_v3.py:717-732` |
| Pending approval at cancel time | Inject `ApprovalReturn(approve=False)` denials and synthetic `ToolReturn` for all pending tool calls | `letta/services/run_manager.py:669-781` |
| Task cancellation on infrastructure | Classify as `StopReasonType.error` with `stream_task_cancelled` `error_metadata` | `letta/server/rest_api/redis_stream_manager.py:324-376` |
| Run cancel after terminal | Idempotent no-op (logs `debug`) | `letta/services/run_manager.py:649-653` |
| Re-cancel a `requires_approval` run | Allowed (treated as still-active) | `letta/services/run_manager.py:649-653` |
| Stream without `[DONE]` and without `stop_reason` | Synthesize `stream_incomplete` error and write `[DONE]` to close clients | `letta/server/rest_api/redis_stream_manager.py:283-315` |
| System prompt larger than context | `_check_for_system_prompt_overflow` raises `SystemPromptTokenExceededError`, mapped to `StopReasonType.context_window_overflow_in_system_prompt` | `letta/agents/letta_agent_v3.py:740-755` |
| `LLMEmptyResponseError` or `ValueError` from LLM | Classified to `StopReasonType.invalid_llm_response`, persists via `update_step_error_async` | `letta/agents/letta_agent_v3.py:1167-1183` |
| Insufficient credits before next step | Loop terminates with `StopReasonType.insufficient_credits` (→ `failed`) | `letta/agents/letta_agent_v3.py:332-337`, `letta/schemas/letta_stop_reason.py:46-47` |
| Callback 5xx or timeout | Log error, persist `callback_error`, run stays `completed` | `letta/services/run_manager.py:498-509` |
| Auto-mode routing failure mid-flight | `update_step_resolved_model_async` in `finally` ensures partial billing | `letta/agents/letta_agent_v3.py:1055-1081` |
| LLM rate-limit / server error | If fallback handle exists, switch model and retry; else `StopReasonType.llm_api_error` | `letta/agents/letta_agent_v3.py:1185-1213` |

## Future Considerations

- **Warning state**: Add `RunStatus.partial` (or a `warning` flag on `Run`) so truncated retries, fallbacks, and validation warnings are observable without parsing metadata.
- **Strict terminal-state enforcement**: Convert the lifecycle log warnings at `letta/services/run_manager.py:341-356` to explicit raises (or 409 responses) once the API surface is stable.
- **Unified final output schema validation**: Add a `validate_final_output` step (analogue of `validate_function_response`) that runs over the `LettaResponse.messages` list before persistence, particularly when `response_format` is configured.
- **Memoize run usage**: Cache `get_run_usage` results per `run_id` so large-step runs don't re-sum on every retrieval (`letta/services/run_manager.py:512-540`).
- **Structured finish_reason coverage**: Map `content_filter`, `tool_calls`, and provider-specific reasons to dedicated `StopReasonType` values instead of collapsing to `end_turn`/`max_tokens_exceeded` (`letta/agents/letta_agent_v3.py:1989-1998`, `letta/adapters/simple_llm_stream_adapter.py:237-251`).
- **Webhook signing/retries**: Callback POST only retries on connection failure (not on 5xx) and has no signature; consider HMAC + dead-letter semantics.

## Questions / Gaps

- **Per-step finalization on `requires_approval`**: The `Step.status` after a `requires_approval` stop is not explicit in the inspected code; only `letta/services/step_manager.py:339-363` shows `update_step_stop_reason` writes the reason without changing `status`. Need to verify step is committed as `SUCCESS` vs left `PENDING`. — **No clear evidence found**.
- **Cost rollup**: There is per-step/per-run token accounting but no calculated cost field anywhere inspected. Cost is presumably external. — **No clear evidence found**.
- **Blocking of "completed → completed" transitions**: Documented as a log-only warning; needs confirmation of intended behavior. — Found evidence (logs only) at `letta/services/run_manager.py:341-356`.
- **`requires_approval` cancellation interplay**: Confirmed allowed via `run.stop_reason not in [StopReasonType.requires_approval]` (`letta/services/run_manager.py:649-651`), but the explicit interaction with newly issued denials via synthetic tool-return messages needs deeper multi-agent coverage.
- **`step_progression` for streaming path**: Streaming reuses the same `_step`, but the `finally` block's `step_metrics.step_ns` fallback to wall-clock may overestimate network waits; cannot verify without measuring.

---

Generated by `dimensions/03.09-completion-and-finalization-semantics.md` against `letta`.

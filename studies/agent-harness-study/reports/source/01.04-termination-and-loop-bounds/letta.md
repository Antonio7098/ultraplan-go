# Source Analysis: letta

## 01.04 — Termination and Loop Bounds

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python 3, FastAPI, asyncio, SQLAlchemy, Pydantic |
| Analyzed | 2026-07-02 |

## Summary

Letta implements a step-bounded agent loop driven by `max_steps` and a
rich, enumerated `StopReasonType`. There are three live agent
implementations (`LettaAgent` legacy `letta/agents/letta_agent.py`,
`LettaAgentV2` `letta/agents/letta_agent_v2.py`, `LettaAgentV3`
`letta/agents/letta_agent_v3.py`), each with its own `step()` /
`stream()` bounded by `for i in range(max_steps)`. Termination is
*runtime-driven*: the loop watches `should_continue`, an externally
pollable `_check_run_cancellation`, periodic
`_check_credits()`, and a per-step `_decide_continuation` that maps
tool-rule outcomes into continue / stop decisions. There is **no
model-side "exhaustion" signal and no stuck-loop detector** beyond
`max_steps`. Termination reasons are persisted at three layers: the
`Step.stop_reason` (`letta/schemas/step.py:47`), the `Run.stop_reason`
(`letta/schemas/run.py:40`), and the agent's `last_stop_reason`
(`letta/schemas/agent.py:155`), with `RunStatus` derived from
`StopReasonType` via `run_status` (`letta/schemas/letta_stop_reason.py:24-49`).
Concurrency is bounded indirectly through a Redis
`conversation_lock` (`letta/constants.py:475`) with a 5-minute TTL, an
event-loop watchdog (`letta/monitoring/event_loop_watchdog.py:32`), and
multi-agent send-message timeouts
(`letta/settings.py:351`). No wall-clock or per-step timeout exists for
the agent loop itself; a hard `max_steps` is the only global bound.

## Rating

**7 / 10**

The taxonomy of stop reasons is mature, well-typed, and persisted at
three storage layers with a clear `RunStatus` mapping. Limits are
configurable per request, `max_steps` is enforced explicitly with a
final-step override, cancellation is pollable mid-step, and there is a
dedicated test suite (`tests/managers/test_cancellation.py`) covering
cancelled / max_steps race conditions
(`tests/managers/test_cancellation.py:1354-1397`). The model falls short
of an 8 because it lacks (a) any wall-clock timeout on the loop, (b) any
heuristic to detect a stuck loop before `max_steps` is hit, (c) a
dedicated loop-detection metric in `StepMetrics`, and (d) explicit
public observability for the `StepProgression` state machine that
`finally` blocks rely on (`letta/agents/letta_agent_v3.py:1546-1592`).

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Default max steps | `DEFAULT_MAX_STEPS = 50` constant | `letta/constants.py:75` |
| Max steps request schema | `max_steps: int = Field(default=DEFAULT_MAX_STEPS, ...)` | `letta/schemas/letta_request.py:43-46` |
| Stop reason enum (13 values) | `StopReasonType` enum with mapping to `RunStatus` | `letta/schemas/letta_stop_reason.py:9-49` |
| Loop body — V3 step() | `for i in range(max_steps):` with final-step override | `letta/agents/letta_agent_v3.py:328, 394-395` |
| Loop body — V2 step() | `for i in range(max_steps):` with `remaining_turns` | `letta/agents/letta_agent_v2.py:233-234` |
| Loop body — V2 stream() | Final-step `max_steps` stop-reason assignment | `letta/agents/letta_agent_v2.py:375-409` |
| Loop body — V3 stream() | Final-step `max_steps` assignment | `letta/agents/letta_agent_v3.py:569-629` |
| Legacy loop body — letta_agent.py | `for i in range(max_steps):` in three paths | `letta/agents/letta_agent.py:247, 597, 949` |
| Voice agent loop | `for _ in range(max_steps):` (no max_steps stop reason) | `letta/agents/voice_agent.py:169-194` |
| Step-level stop-reason persistence | `step.stop_reason = stop_reason` | `letta/services/step_manager.py:339-363` |
| Step-level error persistence | `step.status = StepStatus.FAILED` + `stop_reason` | `letta/services/step_manager.py:368-414` |
| Run-level stop-reason persistence | `Run.stop_reason` field | `letta/schemas/run.py:40` |
| Run status derived from stop reason | `StopReasonType.run_status` property | `letta/schemas/letta_stop_reason.py:24-49` |
| Run status updated on terminal transitions | `update.stop_reason` writes `last_stop_reason` to agent | `letta/services/run_manager.py:398-410` |
| Per-step termination decision (v3) | `_decide_continuation` returns `(continue, reason, stop_reason)` | `letta/agents/letta_agent_v3.py:1967-2036` |
| Tool-rule → stop mapping (v3) | `is_terminal_tool` → `tool_rule`, `is_continue_tool` → continue, `has_children_tools` → continue | `letta/agents/letta_agent_v3.py:2004-2036`; `letta/helpers/tool_rule_solver.py:174-184` |
| Required-before-exit tools force continuation | `get_uncalled_required_tools` heartbeat injection | `letta/agents/letta_agent_v3.py:1626-1642, 1994-1997`; `letta/helpers/tool_rule_solver.py:198-207` |
| Cancellation check (v3) | `_check_run_cancellation(run_id)` polled at start of each step | `letta/agents/letta_agent_v3.py:1031-1035`; `letta/agents/letta_agent_v2.py:506-509, 750-757` |
| Cancellation propagation to loop break | `if not self.should_continue and self.stop_reason.stop_reason == "cancelled": break` | `letta/agents/letta_agent_v3.py:359-360, 616-617` |
| Credit check between steps | `_check_credits` → `StopReasonType.insufficient_credits` | `letta/agents/letta_agent_v3.py:334-338, 574-579`; `letta/agents/letta_agent_v2.py:238-242, 377-381, 738-747` |
| Context-window retry / compaction | `for llm_request_attempt in range(summarizer_settings.max_summarizer_retries + 1)` | `letta/agents/letta_agent_v3.py:1093-1218`; `letta/agents/letta_agent_v2.py:518-573` |
| Final-step force `max_steps` (v3) | `if i == max_steps - 1 and self.stop_reason is None: self.stop_reason = ...` | `letta/agents/letta_agent_v3.py:394-395, 628-629` |
| Final-step force `max_steps` (v2 stream) | `if self.stop_reason is None: ... max_steps.value` | `letta/agents/letta_agent_v2.py:407-409` |
| Exception → `error` stop reason | `if not self.stop_reason: ... = error.value` | `letta/agents/letta_agent_v3.py:1528-1530`; `letta/agents/letta_agent_v2.py:654-655`; `letta/agents/letta_agent.py:457-458` |
| `requires_approval` stop reason | `StopReasonType.requires_approval` set when tool needs approval | `letta/agents/letta_agent_v3.py:1709`; `letta/agents/letta_agent_v2.py:1153` |
| `no_tool_call` stop reason (v2 / legacy) | `raise LLMError("No tool calls found ...")` after setting `no_tool_call` | `letta/agents/letta_agent_v2.py:582-584`; `letta/agents/letta_agent.py:361-362` |
| `max_tokens_exceeded` stop reason | Triggered when `finish_reason == "length"` | `letta/agents/letta_agent_v3.py:2000-2001` |
| `context_window_overflow_in_system_prompt` | `_check_for_system_prompt_overflow` raises `SystemPromptTokenExceededError` | `letta/agents/letta_agent_v3.py:741-756, 1285-1292, 1506-1509` |
| Multi-agent timeout | `multi_agent_send_message_timeout: int = 20 * 60` | `letta/settings.py:351` |
| Tool sandbox timeout | `tool_sandbox_timeout: float = 180` | `letta/settings.py:36` |
| LLM request / stream timeouts | `llm_request_timeout_seconds`, `llm_stream_timeout_seconds` | `letta/settings.py:429-431` |
| LLM provider per-provider retries | `anthropic_max_retries: int = 3`, `gemini_max_retries: int = 5` | `letta/settings.py:181, 218` |
| Summarizer retries (LLM-call level) | `max_summarizer_retries: int = 3` | `letta/settings.py:96` |
| Conversation lock TTL | `CONVERSATION_LOCK_TTL_SECONDS = 300` (5 minutes) | `letta/constants.py:475` |
| Conversation lock acquire / release | `acquire_conversation_lock`, `release_conversation_lock` | `letta/data_sources/redis_client.py:194-231`; `letta/services/streaming_service.py:299` |
| Event loop watchdog | `EventLoopWatchdog` with `timeout_threshold: float = 15.0` | `letta/monitoring/event_loop_watchdog.py:32-44, 191-207` |
| Step metrics for run-loop bookkeeping | `StepMetrics` with `step_start_ns`, `step_ns`, `tool_execution_ns`, `llm_request_ns` | `letta/schemas/step_metrics.py:13-26` |
| StepProgression state machine (6 states) | `START → STREAM_RECEIVED → RESPONSE_RECEIVED → STEP_LOGGED → LOGGED_TRACE → FINISHED` | `letta/schemas/step.py:68-74` |
| Step-level stop-reason update paths (v3) | Multiple update points in `finally` based on `step_progression` | `letta/agents/letta_agent_v3.py:1546-1592` |
| Cancellation idempotency | Reject double-cancel if already terminated except `requires_approval` | `letta/services/run_manager.py:649-667` |
| Run completion → agent last_stop_reason | `UpdateAgent(last_stop_reason=update.stop_reason)` | `letta/services/run_manager.py:398-410` |
| Cancellation tests | Dedicated test suite | `tests/managers/test_cancellation.py` |
| Cancellation + max_steps race test | `test_cancellation_with_max_steps_reached` | `tests/managers/test_cancellation.py:1354-1397` |
| Streaming cancellation tests | `test_token_streaming_cancellation`, `test_step_streaming_cancellation` | `tests/managers/test_cancellation.py:449-575` |
| Tool-call-noop test | `integration_test_client_side_tools.py` asserts allowed stop reasons | `tests/integration_test_client_side_tools.py:140-162` |

## Answers to Dimension Questions

1. **What stops the loop?**
   - A bounded `for i in range(max_steps)` (`letta/agents/letta_agent_v3.py:328`,
     `letta/agents/letta_agent_v2.py:233`,
     `letta/agents/letta_agent.py:597, 949`).
   - `self.should_continue == False` set by per-step
     `_decide_continuation` (`letta/agents/letta_agent_v3.py:1967-2036`),
     `_check_run_cancellation` (`letta/agents/letta_agent_v3.py:1031-1035`),
     `_check_credits` (`letta/agents/letta_agent_v3.py:334-338`),
     or `_handle_ai_response` outcomes
     (`letta/agents/letta_agent_v3.py:1016, 1709, 2001`).
   - Exceptions caught in the `_step` `try / except / finally`
     (`letta/agents/letta_agent_v3.py:1511-1540`).
   - Terminal tool calls (`StopReasonType.tool_rule`,
     `letta/agents/letta_agent_v3.py:2011`).
   - Hard override on the last iteration:
     `if i == max_steps - 1 and self.stop_reason is None: ... max_steps.value`
     (`letta/agents/letta_agent_v3.py:394-395, 628-629`).

2. **Are limits configurable?**
   - `max_steps` defaults to `DEFAULT_MAX_STEPS = 50`
     (`letta/constants.py:75`) and is overridable per-request via
     `LettaRequest.max_steps` (`letta/schemas/letta_request.py:43-46`)
     or the `/agents/.../export` query parameter
     (`letta/server/rest_api/routers/v1/agents.py:298`, marked
     `deprecated=True`).
   - Summarizer retry cap is `max_summarizer_retries = 3`
     (`letta/settings.py:96`); Anthropic retries = 3 (`letta/settings.py:181`);
     Gemini retries = 5 (`letta/settings.py:218`).
   - Concurrency lock TTL is `CONVERSATION_LOCK_TTL_SECONDS = 300`
     (`letta/constants.py:475`); multi-agent send-message timeout is
     `20 * 60` seconds (`letta/settings.py:351`).
   - There is **no per-step wall-clock timeout**, no per-loop wall-clock
     timeout, and no public knob for `max_steps` on the `agent_type`
     schema (it lives only on the request object).

3. **Is exhaustion treated differently from success?**
   - Yes, in two layers. The `StopReasonType` distinguishes
     `max_steps` from `end_turn`, `tool_rule`, `requires_approval`
     (all "completed" terminal reasons) from `error`,
     `invalid_tool_call`, `invalid_llm_response`, `llm_api_error`,
     `max_tokens_exceeded`, `no_tool_call`,
     `context_window_overflow_in_system_prompt`,
     `insufficient_credits` (all "failed" terminal reasons)
     (`letta/schemas/letta_stop_reason.py:24-49`).
   - `RunStatus` is derived from this enum
     (`letta/schemas/letta_stop_reason.py:25-49`), so `Run.status`
     is `completed`, `failed`, or `cancelled`
     (`letta/schemas/enums.py:152-161`).
   - The `_process_message_background` wrapper explicitly maps
     `cancelled` → `RunStatus.cancelled`, fallback → `RunStatus.completed`
     (`letta/server/rest_api/routers/v1/agents.py:2118-2128`).
   - Cancellation runs are guarded as idempotent: a completed run can
     only be re-marked as cancelled when its stop reason is
     `requires_approval` (`letta/services/run_manager.py:649-667`).

4. **Are stuck loops detected before the hard limit?**
   - **No dedicated detector.** There is no repetition counter, no
     n-gram diff on assistant messages, and no consecutive-tool-call
     heuristic. The only pre-limit signal is the
     `ToolRulesSolver.get_uncalled_required_tools` heartbeat that
     forces continuation when required-before-exit tools are missing
     (`letta/agents/letta_agent_v3.py:1626-1642, 1994-1997`).
   - The `if last_function_failed` filter strips the previously failed
     tool name from the allowed set
     (`letta/agent.py:325-326`), which prevents the *next* step from
     re-trying the same failing tool, but does not detect broader
     oscillation.
   - The event-loop watchdog (`letta/monitoring/event_loop_watchdog.py:32`)
     is global infrastructure that flips readiness to `degraded`
     after sustained lag; it does not interrupt an individual agent
     loop. It is verified by `test_watchdog_hang.py:1-97`.

5. **Does the user get a useful final state?**
   - The blocking `step()` always returns `LettaResponse` with
     `stop_reason`, `usage`, and `messages`
     (`letta/agents/letta_agent_v3.py:426-441`).
   - Streaming responses emit `[DONE]` plus a serialized stop reason
     as the final SSE chunk
     (`letta/agents/base_agent.py:188-195`).
   - Each step writes a `Step` row with `stop_reason`, `error_type`,
     `error_data`, and `status`
     (`letta/schemas/step.py:47, 63-65`; `letta/services/step_manager.py:339-414`).
   - The run row is updated at terminal transitions with `status`,
     `completed_at`, `stop_reason`, `total_duration_ns`
     (`letta/services/run_manager.py:359-446`).
   - The agent's `last_stop_reason` mirrors the terminal run stop
     reason (`letta/services/run_manager.py:398-410`,
     `letta/schemas/agent.py:155`).
   - Webhook callbacks fire on terminal updates
     (`letta/services/run_manager.py:336-339, 451-468`).
   - Failure cleanup uses `StepProgression` to decide whether to
     `update_step_error_async` (when pre-`STEP_LOGGED`) or
     `update_step_stop_reason` (when post-`RESPONSE_RECEIVED`)
     (`letta/agents/letta_agent_v3.py:1546-1592`).

## Architectural Decisions

- **Steps-as-atoms, max_steps-as-budget.** Every loop runs
  `for i in range(max_steps)` and stamps `StopReasonType.max_steps`
  on the final iteration if no earlier signal fired
  (`letta/agents/letta_agent_v3.py:394-395, 628-629`,
  `letta/agents/letta_agent_v2.py:407-409`). This makes the hard limit
  observable rather than silent.
- **Single source of truth for stop taxonomy.** `StopReasonType` is the
  single enum shared by LLM adapters, step tables, run rows, and agent
  metadata (`letta/schemas/letta_stop_reason.py:9-22`). Its
  `run_status` property centralizes the success/failure mapping
  (`letta/schemas/letta_stop_reason.py:24-49`).
- **Tool-rule driven continuation, not free-form LLM decisions.**
  `ToolRulesSolver` answers `is_terminal_tool`, `is_continue_tool`,
  `has_children_tools`, `is_requires_approval_tool`, and
  `get_uncalled_required_tools` (`letta/helpers/tool_rule_solver.py:174-207`)
  and is consulted in `_decide_continuation`
  (`letta/agents/letta_agent_v3.py:1967-2036`). This makes termination
  predictable for users who define explicit tool rules.
- **Three storage layers, one direction of write.** Steps persist
  `stop_reason` first; runs pick it up at terminal transition and
  propagate it to `agent.last_stop_reason`
  (`letta/services/run_manager.py:398-410`). The `last_stop_reason`
  field powers agent-listing filters
  (`letta/services/helpers/agent_manager_helper.py:743-788`,
  `letta/server/rest_api/routers/v1/agents.py:176-283`).
- **Cancellation is pollable, not interrupt-based.** Each step
  re-checks `_check_run_cancellation(run_id)` against the run row's
  status (`letta/agents/letta_agent_v2.py:750-757`,
  `letta/agents/letta_agent_v3.py:1031-1035`). The Redis
  conversation-lock TTL (`letta/constants.py:475`) acts as the upper
  bound if the worker crashes mid-step.
- **Stop-reason persistence is gated by a feature flag.**
  `track_stop_reason: bool = Field(default=True, ...)`
  (`letta/settings.py:385`) and `track_agent_run`
  (`letta/settings.py:386`) gate whether the system writes the
  `RunUpdate` payload, providing a kill switch for the persistence
  path.

## Notable Patterns

- **Per-step checkpointing via `StepProgression` enum.** The 6-state
  `START → STREAM_RECEIVED → RESPONSE_RECEIVED → STEP_LOGGED →
  LOGGED_TRACE → FINISHED` enum (`letta/schemas/step.py:68-74`) is
  updated at each phase, and the `finally` block dispatches on the
  final state to choose between `update_step_error_async`,
  `update_step_stop_reason`, or no-op
  (`letta/agents/letta_agent_v3.py:1546-1592`).
- **Tool-rule prompts compiled into the system block.**
  `compile_tool_rule_prompts()` builds a `Block` that is inserted into
  the system prompt at request build time
  (`letta/helpers/tool_rule_solver.py:209-237`,
  `letta/agents/base_agent.py:109-110`); this lets the model know
  *before* a step that calling `send_message` will end the loop,
  reducing wasted continuation work.
- **Required-before-exit + heartbeat pattern.** When a tool is
  configured as `required_before_exit`, the loop refuses to end until
  it has been called. If the model returns no tool call, the
  controller injects a `NON_USER_MSG_PREFIX` heartbeat that names the
  uncalled tools and forces another step
  (`letta/agents/letta_agent_v3.py:1628-1642`). This is a soft
  stuck-loop mitigator for one specific pattern (missing required
  tool).
- **Asynchronous credit verification between steps.** The credit check
  is fired as a `safe_create_task_with_return` after each step so it
  overlaps with the next step's setup
  (`letta/agents/letta_agent_v3.py:334-339, 574-580`); the awaited
  result on the next iteration either permits continuation or sets
  `StopReasonType.insufficient_credits`. This is a runtime-driven
  budget enforcement without a hard wall-clock.
- **Streaming-friendly error reporting.** The V3 stream loop buffers
  `response_letta_messages` and, on mid-stream exception, only emits
  an SSE error event if at least one chunk has been sent; if no chunk
  has been sent it re-raises so the caller gets an HTTP error status
  (`letta/agents/letta_agent_v3.py:649-692`). This keeps the
  client-side observable state consistent regardless of where
  failure occurred.
- **Cancellation idempotency at the run layer.** `cancel_run` checks
  the existing `stop_reason` and only proceeds when not already
  terminated (with a special exemption for `requires_approval`)
  (`letta/services/run_manager.py:649-667`), eliminating the common
  race where cancellation arrives after the loop completes.

## Tradeoffs

- **`max_steps` is the only hard bound.** Without a wall-clock or
  per-step timeout, a step that hangs in a synchronous tool call
  cannot be cut short. The conversation lock TTL of 300 s
  (`letta/constants.py:475`) is the only fall-back release, and it
  requires the worker to die to take effect. The event-loop
  watchdog (`letta/monitoring/event_loop_watchdog.py:32-44`) detects
  hangs but does not interrupt the agent loop directly.
- **`max_steps` is hard-coded into the request schema, not the
  agent.** A caller that sets `max_steps=1000` can keep the worker
  occupied far longer than the agent's intended workload
  (`letta/schemas/letta_request.py:43-46`). There is no per-agent
  cap enforced by `AgentState`.
- **No repetition / oscillation detector.** The system relies on the
  LLM to vary its tool calls. A model that returns
  `archival_memory_search` forever will exhaust `max_steps` even if
  the same result keeps being returned. The only anti-loop measure
  is `last_function_failed`-based suppression
  (`letta/agent.py:325-326`), which is narrow.
- **Stop-reason semantics for `requires_approval` are ambiguous.**
  The enum classifies it as `RunStatus.completed`
  (`letta/schemas/letta_stop_reason.py:30`), but the run is paused
  awaiting user input, not finished. The cancellation idempotency
  check allows it to be overwritten by `cancelled`
  (`letta/services/run_manager.py:651`), indicating a partial
  workaround.
- **Three concurrent agent implementations.** V1 (`letta_agent.py`),
  V2 (`letta_agent_v2.py`), and V3 (`letta_agent_v3.py`) each carry
  their own stop-reason copy-paste logic with subtle differences
  (V3 catches `LLMEmptyResponseError`, V2 doesn't). Drift between
  them is a maintenance hazard.
- **Per-step streaming cancellation relies on cooperative polling.**
  The cancellation check only fires at the start of each step
  (`letta/agents/letta_agent_v3.py:1031`). A step stuck inside a
  slow LLM request will not observe cancellation until the LLM
  responds.
- **`track_stop_reason` is disabled by default.** The default is
  `True` (`letta/settings.py:385`), but any environment that flips it
  off loses the `Step.stop_reason` write entirely
  (`letta/agents/letta_agent_v3.py:1578-1579`).

## Failure Modes / Edge Cases

- **Concurrent request to same agent.** The agent loop assumes
  single-tenant access; the `/agents/.../messages` route documents
  this with a warning about interleaving
  (`letta/server/rest_api/routers/v1/agents.py:1854-1858`). The
  Redis conversation lock mitigates this only at the conversation
  granularity.
- **`run_id=None` outside `_check_run_cancellation`.** When a caller
  invokes `step()` without a `run_id`, the cancellation check is
  skipped (`letta/agents/letta_agent_v3.py:1031`). The agent loop has
  no other way to be told to stop.
- **`max_steps=0` and `max_steps=-1`.** The schema accepts both
  (`tests/test_letta_request_schema.py:33-48`); with `max_steps=0`,
  the loop body never executes and the post-loop assignment sets
  `end_turn` directly
  (`letta/agents/letta_agent_v3.py:412-413`). With negative values
  the `range()` is empty and the same fallback applies. Both are
  supported but not user-friendly.
- **Cancellation and max_steps race.** A cancellation that arrives
  between iterations may lose the race to a natural `max_steps`
  finish; the test suite allows both outcomes
  (`tests/managers/test_cancellation.py:1394-1397`), meaning the user
  may receive `cancelled` or `max_steps` interchangeably.
- **Mid-stream exception after partial yield.** A thrown exception
  mid-stream yields the stop-reason chunk and an `event: error` SSE
  chunk before closing, but the `response_letta_messages` may
  contain partial data (`letta/agents/letta_agent_v3.py:664-692`).
  Clients must distinguish between a clean stop and an error event.
- **Compaction retries exhausted.** `ContextWindowExceededError` is
  retried up to `max_summarizer_retries + 1` times
  (`letta/agents/letta_agent_v3.py:1093, 1218`,
  `letta/agents/letta_agent_v2.py:518, 563`). When retries are
  exhausted, the loop propagates a `StopReasonType.error` and re-raises
  (`letta/agents/letta_agent_v3.py:1521-1529`).
- **Stuck on tool rule violation.** If a model repeatedly calls a
  tool not in `valid_tool_names`, the v3 loop treats it as a
  rule-violation result and continues
  (`letta/agents/letta_agent_v3.py:1800-1808, 1825-1827`), only
  hitting `max_steps` after `DEFAULT_MAX_STEPS` iterations.

## Future Considerations

- Add a per-loop `max_total_runtime_seconds` knob on `LettaRequest`
  and a per-step `max_step_runtime_seconds`, with an
  `asyncio.timeout` that converts the timeout into
  `StopReasonType.error` (or a new `timeout` enum value) instead of
  relying on cooperative polling.
- Add a repetition detector that hashes consecutive assistant
  messages (or consecutive tool-call signatures) and trips
  `StopReasonType.no_tool_call` or a new `stuck` reason before
  `max_steps`. Surface the hash count in `StepMetrics`.
- Unify V1 / V2 / V3 stop-reason copy-paste. Either retire V1 or
  factor the `except / finally` block into a shared `_finalize_step`
  helper to eliminate drift (`letta/agents/letta_agent_v2.py:648-726`,
  `letta/agents/letta_agent_v3.py:1511-1592`).
- Promote `track_stop_reason` to be non-toggleable, or move the
  persistence path out of `settings` entirely
  (`letta/settings.py:385`). Silent loss of termination telemetry is
  an observability regression waiting to happen.
- Add a `StopReasonType.timed_out` value distinct from
  `max_tokens_exceeded` so that wall-clock exhaustion is reported
  without conflating with provider-side length limits
  (`letta/schemas/letta_stop_reason.py:16`).
- Expose `max_steps` as an `AgentState` field with a per-agent
  ceiling enforced server-side, so clients cannot request unbounded
  steps regardless of what the request body says.

## Questions / Gaps

- Is there an internal SLO for `p99` step duration that the system
  relies on, but no code path enforces? No evidence of a global
  step-runtime cap beyond per-LLM-request timeouts
  (`letta/settings.py:429-431`).
- Does the `last_stop_reason` propagation handle the case where
  the agent was deleted between the run completing and the agent
  update (`letta/services/run_manager.py:402-410`)? The path swallows
  exceptions and logs them, but no test exercises it.
- Are there any callers that bypass `AgentLoop.load` and call
  `_step()` directly, bypassing cancellation? No evidence in the
  inspected code, but the abstract `_step()` is `async` and could
  be invoked elsewhere.
- What happens when `client_skills` or `client_tools` are mutated
  mid-stream? The V3 `_initialize_state` resets these once per `step`
  call (`letta/agents/letta_agent_v3.py:122-141`), but a streaming
  response that persists across runs may carry stale state — no
  evidence was found that this is tested.
- The voice agent (`letta/agents/voice_agent.py:169`) does not set
  `max_steps` stop reason; it simply breaks out of the loop and
  emits `[DONE]`. This means voice runs never observe
  `StopReasonType.max_steps`, only whatever `_handle_ai_response`
  decided. No voice-specific test for max-step exhaustion was found.

---

Generated by `dimensions/01.04-termination-and-loop-bounds.md` against `letta`.
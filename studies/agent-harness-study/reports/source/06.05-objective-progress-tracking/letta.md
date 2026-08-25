# Source Analysis: letta

## Objective and Progress Tracking

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI server, SQLAlchemy ORM, Pydantic schemas, SSE streaming, OTEL/ClickHouse observability) |
| Analyzed | 2026-08-25 |

## Summary

Letta does not model "the goal" as a first-class object. Goals exist implicitly in user messages and in the agent's memory (including a summarizer prompt that explicitly asks the summarizer to carry forward "high level goals" as prose, `letta/prompts/summarizer_prompt.py:7`). What Letta instead builds — and builds well — is a **harness-owned progress ledger**: every unit of activity is recorded as a `Step` with a lifecycle state machine (`letta/schemas/step.py:16-74`), grouped under a `Run` whose `status` transitions are driven by an explicit, enumerated set of stop reasons (`letta/schemas/run.py:23-40`, `letta/schemas/letta_stop_reason.py:9-49`). Progress is measured mechanically by the loop itself: whether a tool call was made and executed, whether tool rules demand continuation or termination (`letta/agents/letta_agent_v3.py:1967-2036`), whether approvals block progress (`letta/agents/letta_agent_v3.py:1682-1709`), and hard bounds like `max_steps=50` (`letta/constants.py:75`). The model cannot declare success; only the harness's `_decide_continuation` / stop-reason machinery can end a run. Completion is surfaced everywhere — SSE terminal events (`stop_reason`, usage, `[DONE]`; `letta/agents/letta_agent_v2.py:1476-1487`), REST endpoints for run status/steps/metrics/usage/traces (`letta/server/rest_api/routers/v1/runs.py:143-307`), webhooks on step completion (`letta/services/step_manager.py:473-476`), and per-run callbacks (`letta/schemas/run.py:43-46`). The main weakness is semantic, not mechanical: `end_turn`, `max_steps`, `tool_rule`, and `requires_approval` all map to `RunStatus.completed` (`letta/schemas/letta_stop_reason.py:24-32`), so "the loop stopped" is conflated with "the objective was achieved", and nothing independently verifies task success.

## Rating

**7 / 10** — Clear progress-tracking model with explicit interfaces (Step/StepMetrics/Run/RunMetrics), tests for the continuation rules (`tests/test_tool_rule_solver.py:55-64`, `tests/test_tool_rule_solver.py:487-576`) and run-status conversion (`tests/test_run_status_conversion.py:13-80`), plus operational safeguards (terminal-state guards, stream-finalizer watchdogs, cancellation support). It falls short of 8–10 because: (a) there is no structured objective/goal representation to measure progress *against*; (b) final success is never independently checked; (c) run-lifecycle transition guards only log violations rather than rejecting them (`letta/services/run_manager.py:341-356`); and (d) truncation by `max_steps` is reported as `completed`.

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Goal representation | No goal/task primitive found (searched `goal|todo|objective` across `letta/schemas/`, `letta/orm/`). Only prose-level goal preservation in summarizer prompts ("High level goals…") | `letta/prompts/summarizer_prompt.py:7` |
| Goal record (run) | Run schema: `status` (created/running/completed/failed/cancelled), `stop_reason`, `completed_at`, callback fields, TTFT/duration metrics | `letta/schemas/run.py:17-51` |
| Run persistence | `runs` table with FKs to agent/conversation, cascade relationships to steps and messages | `letta/orm/run.py:22-77` |
| Progress unit (step) | Step schema: token counts, `stop_reason`, `status` (pending/success/failed/cancelled), error_type/error_data, trace_id, feedback | `letta/schemas/step.py:16-65` |
| Step progression state machine | `StepProgression` enum: START → STREAM_RECEIVED → RESPONSE_RECEIVED → STEP_LOGGED → LOGGED_TRACE → FINISHED | `letta/schemas/step.py:68-74` |
| Step lifecycle writes | Step created early with `PENDING` at checkpoint start | `letta/agents/letta_agent_v2.py:941-966` |
| Step success finalize | `update_step_success_async` sets `SUCCESS` + per-step token details, fires step-complete webhook | `letta/services/step_manager.py:419-476` |
| Step failure recording | `update_step_error_async` sets `FAILED` + error_type/error_data(message/traceback/details) | `letta/services/step_manager.py:368-414` |
| Step cancellation recording | `update_step_cancelled_async` sets `CANCELLED` + optional stop reason | `letta/services/step_manager.py:519-555` |
| Timing metrics per step | StepMetrics: llm_request_ns, tool_execution_ns, step_ns (nanosecond precision) | `letta/schemas/step_metrics.py:13-26`, `letta/services/step_manager.py:562-652` |
| Loop bound | `DEFAULT_MAX_STEPS = 50`; enforced via `for i in range(max_steps)` in both blocking and streaming loops | `letta/constants.py:75`, `letta/agents/letta_agent_v3.py:328`, `letta/agents/letta_agent_v3.py:569` |
| max_steps stop reason | On last iteration without another stop reason → `StopReasonType.max_steps` | `letta/agents/letta_agent_v3.py:394-395`, `letta/agents/letta_agent_v3.py:628-629` |
| Continuation decision | `_decide_continuation`: no tool call → end_turn; tool call → continue unless terminal tool rule; required-before-exit tools force continuation; is_final_step overrides as max_steps | `letta/agents/letta_agent_v3.py:1967-2036` |
| Terminal tool rule ends loop | `is_terminal_tool` → `continue_stepping=False`, stop_reason=`tool_rule` | `letta/agents/letta_agent_v3.py:2010-2012`, `letta/helpers/tool_rule_solver.py:174-176` |
| Required-before-exit guard | Uncalled required tools keep the loop alive with explicit feedback injected into context | `letta/agents/letta_agent_v3.py:1994-1997`, `letta/agents/letta_agent_v3.py:2027-2034` |
| Tool rule definitions | `TerminalToolRule` (exit_loop), `ContinueToolRule` (continue_loop), `RequiredBeforeExitToolRule` with rendered `<tool_rule>` prompts | `letta/schemas/tool_rule.py:275-312` |
| Stop reason taxonomy | 12 enumerated reasons incl. end_turn, error, llm_api_error, invalid_tool_call, max_steps, cancelled, insufficient_credits, requires_approval, context overflow | `letta/schemas/letta_stop_reason.py:9-22` |
| Canonical status mapping | `StopReasonType.run_status` property maps stop reasons → completed/failed/cancelled | `letta/schemas/letta_stop_reason.py:24-49` |
| Model cannot self-declare completion (v3) | v3 pops `request_heartbeat` from tool args and disables heartbeat injection (`request_heartbeat=False`); continuation decided only by harness rules | `letta/agents/letta_agent_v3.py:1776`, `letta/agents/letta_agent_v3.py:2068-2073` |
| Legacy model-driven continuation (v1/v2) | `REQUEST_HEARTBEAT_PARAM = "request_heartbeat"` still exists for older loops; heartbeat messages inject "returning control" notices | `letta/constants.py:217`, `letta/server/rest_api/utils.py:630-655` |
| Approval blocker | Tool calls requiring approval produce persisted `ApprovalRequestMessage`; loop stops with `requires_approval`; agent exposes `pending_approval` | `letta/agents/letta_agent_v3.py:1682-1709`, `letta/schemas/letta_message.py:306-325`, `letta/schemas/agent.py:134` |
| Approval tested | Integration tests assert approval request/response round trip and `pending_approval` clearing | `tests/integration_test_human_in_the_loop.py:185-292` |
| Credit-based halt | Pre-step credit check breaks loop with `insufficient_credits` stop reason | `letta/agents/letta_agent_v3.py:333-339`, `letta/agents/letta_agent_v3.py:574-580` |
| Error classification on exception | Exceptions classify into `context_window_overflow_in_system_prompt` / `llm_api_error` / generic `error` stop reasons | `letta/agents/letta_agent_v3.py:649-662` |
| Stream terminal protocol | Finish chunks always emit `stop_reason`, usage stats, `[DONE]` sentinel | `letta/agents/letta_agent_v2.py:1476-1487`, `letta/schemas/enums.py:175-179` |
| Stuck-stream watchdog | If stream ends without `[DONE]`, run force-marked failed with `stream_incomplete` error metadata (background path delegated to processor) | `letta/services/streaming_service.py:777-800` |
| Background processor finalization | Derives final stop reason if unobserved and resolves run status via canonical `StopReasonType(...).run_status` mapping | `letta/server/rest_api/redis_stream_manager.py:419-445` |
| Run lifecycle guards | `update_run_by_id_async` detects illegal re-terminal transitions but only logs them ("validate run lifecycle (only log the errors)") before applying updates | `letta/services/run_manager.py:341-356` |
| Agent-level last-run rollup | On terminal update, agent's `last_stop_reason` updated; `last_run_completion`/`last_run_duration_ms` maintained | `letta/services/run_manager.py:398-410`, `letta/agents/letta_agent_v2.py:1464-1474`, `letta/schemas/agent.py:153-155` |
| Run metrics rollup | `num_steps` and `tools_used` computed at run completion | `letta/services/run_manager.py:412-439`, `letta/schemas/run_metrics.py:13-23` |
| UI/API status surface | REST: list runs (filter by status/stop_reason/active), retrieve run status, run messages/usage/metrics/steps, OTEL trace spans | `letta/server/rest_api/routers/v1/runs.py:46-115`, `letta/server/rest_api/routers/v1/runs.py:143-307` |
| Observability correlation | Steps persist `trace_id`/`request_id` from OTEL tracer and API middleware; `/runs/{id}/trace` returns filtered spans (agent_step, tools, root, TTFT) | `letta/services/step_manager.py:157-158`, `letta/schemas/step.py:50-51`, `letta/server/rest_api/routers/v1/runs.py:267-307` |
| Keepalive liveness | Optional `LettaPing` keepalive every 20s on long-running SSE streams (opt-in via `include_pings`) | `letta/settings.py:290-291`, `letta/schemas/letta_message.py:371-379`, `letta/server/rest_api/routers/v1/runs.py:405-406` |
| Webhook/callback delivery | Step-complete webhooks; per-run `callback_url` POSTed once on first terminal transition with sent_at/status_code/error tracking | `letta/services/webhook_service.py:16`, `letta/services/run_manager.py:332-339`, `letta/schemas/run.py:43-46` |
| Cancellation | `cancel_run` refuses to cancel already-terminal runs except pending approvals; cancellation-aware stream wrappers | `letta/services/run_manager.py:619-664`, `letta/services/streaming_service.py:417-426` |
| Tests | Tool-rule solver behavior incl. terminal/required-before-exit semantics; run-status string conversion | `tests/test_tool_rule_solver.py:55-64`, `tests/test_tool_rule_solver.py:487-576`, `tests/test_run_status_conversion.py:13-80` |

## Answers to Dimension Questions

**1. What is the goal?**
There is no explicit goal object. The closest artifacts are: (a) the user's input message(s), which seed the conversation (`letta/agents/letta_agent_v3.py:274-287`); (b) the agent's memory blocks, which the model edits as it works; and (c) the summarizer prompt, which instructs the compaction LLM to preserve "high level goals and long-term progress" as narrative text (`letta/prompts/summarizer_prompt.py:7`). The `Run` record (`letta/schemas/run.py:17-51`) represents *execution intent* (process these messages), not a declarative objective with success criteria. Search note: queries for `goal|todo|objective` across `letta/schemas/` and `letta/orm/` returned no schema-level hits.

**2. How is progress measured?**
Mechanically, by the harness, at three granularities:
- **Per-step**: a Step row created `PENDING` before each LLM call (`letta/agents/letta_agent_v2.py:941-966`) and finalized `SUCCESS`/`FAILED`/`CANCELLED` with token and timing metrics (`letta/services/step_manager.py:419-476`, `letta/services/step_manager.py:562-652`). An in-process `StepProgression` enum tracks START→FINISHED checkpoints (`letta/schemas/step.py:68-74`).
- **Per-run**: aggregate `RunMetrics` with `num_steps` and `tools_used` computed at completion (`letta/services/run_manager.py:412-439`), plus TTFT/duration (`letta/schemas/run.py:48-50`).
- **Per-agent**: `last_run_completion`, `last_run_duration_ms`, `last_stop_reason` rollups (`letta/schemas/agent.py:153-155`).
Progress toward the *user's actual objective* is not measured anywhere; there is no test, rubric, or verifier that scores outcomes.

**3. Can the model fake progress?**
Largely no, with one caveat. In the current v3 loop the model cannot assert completion: `request_heartbeat` is stripped from its tool arguments (`letta/agents/letta_agent_v3.py:1776`), heartbeats are not injected into schemas (`request_heartbeat=False`, `letta/agents/letta_agent_v3.py:2071`), and the loop continues/stops based on structural facts (tool called or not, tool rules, step budget). Run status changes flow exclusively through harness code paths (`letta/services/streaming_service.py:630-637`, `letta/server/rest_api/routers/v1/agents.py:2113-2134`). The caveats: (a) if a developer marks a tool as `TerminalToolRule`, then the model *calling that tool* ends the run as `tool_rule`→`completed` (`letta/agents/letta_agent_v3.py:2010-2012`), so a model can trigger premature "completion" by invoking the designated exit tool — the trust boundary is the tool-rule configuration, not runtime verification; (b) legacy v1/v2 loops still honor model-supplied `request_heartbeat=true` for continuation control (`letta/constants.py:217`), though this controls pacing rather than outcome claims.

**4. Are blockers recorded?**
Yes, in several channels. Approval-gated tools and client-side tools pause the loop with a persisted `ApprovalRequestMessage` and stop reason `requires_approval` (`letta/agents/letta_agent_v3.py:1682-1709`); the pending request is queryable via `agent.pending_approval` (`letta/schemas/agent.py:134`) and exercised by integration tests (`tests/integration_test_human_in_the_loop.py:280-292`). Failures are recorded on steps as `error_type` + structured `error_data` (message, traceback, details) (`letta/services/step_manager.py:368-414`) and on runs as metadata `{"error": ...}` with typed SSE `error_type`s like `llm_timeout`, `stream_incomplete` (`letta/services/streaming_service.py:639-651`, `letta/services/streaming_service.py:793-799`). Resource blockers surface as `insufficient_credits` (`letta/agents/letta_agent_v3.py:333-339`). There is no dedicated "blocker" entity; they are encoded as message types + stop reasons + error payloads.

**5. Is final success independently checked?**
No. There is no verification layer: `end_turn` simply means the model produced content without a tool call and no required-before-exit tools remain uncalled (`letta/agents/letta_agent_v3.py:1998-2002`). More significantly, `max_steps` (truncation) and `tool_rule` (exit-tool invoked) also map to `RunStatus.completed` (`letta/schemas/letta_stop_reason.py:26-32`), so clients cannot distinguish "finished the job" from "ran out of budget" or "called the exit tool" without inspecting `stop_reason` themselves. The only independent signals are operational (webhooks/callbacks confirm delivery of the result, not its correctness).

## Architectural Decisions

1. **Harness-owned outcome arbitration.** Outcome lives in an enumerable `StopReasonType` with a single canonical mapping to `RunStatus` (`letta/schemas/letta_stop_reason.py:24-49`). All writers (foreground stream, background Redis processor, background tasks, sleeptime groups) route through this mapping, e.g. `letta/server/rest_api/redis_stream_manager.py:430-435` explicitly resolves via `StopReasonType(final_stop_reason).run_status` "to avoid drift".

2. **Write-ahead step records.** Steps are inserted as `PENDING` *before* the LLM request (`letta/agents/letta_agent_v2.py:946-962`) so that crashes leave an auditable trail; the `finally` block downgrades partial failures using the `StepProgression` watermark (`letta/agents/letta_agent_v3.py:1541-1590`).

3. **Structural continuation rules over prompt-based control.** v3 replaced MemGPT-style heartbeats with `ToolRulesSolver`-driven decisions (`letta/helpers/tool_rule_solver.py:174-207`) and renders rule explanations back into context when forcing continuation (`letta/agents/letta_agent_v3.py:1994-1997`).

4. **Layered notification fan-out.** Terminal states trigger up to four observers: SSE terminal chunks, run callbacks (`callback_url`, one-shot guarded by `not_completed_before`, `letta/services/run_manager.py:332-339`), step webhooks (`letta/services/step_manager.py:473-476`), and agent rollup fields (`letta/services/run_manager.py:398-410`).

5. **Advisory lifecycle guards.** Illegal transitions (e.g., updating a completed run) are logged loudly but not rejected (`letta/services/run_manager.py:341-356`), favoring availability over strictness.

## Notable Patterns

- **Sentinel-delimited streams**: every stream terminates with `stop_reason` → usage → `[DONE]` (`letta/agents/letta_agent_v2.py:1476-1487`), and a belt-and-suspenders finalizer appends a forced `[DONE]` if the producer died (`letta/server/rest_api/redis_stream_manager.py:447-459`).
- **Watchdog defaults-fail-closed**: a stream that ends without `[DONE]` marks the run `failed` with `stream_incomplete` rather than leaving it `running` forever (`letta/services/streaming_service.py:777-800`) — progress cannot silently hang.
- **Milestone rollups**: `num_steps`/`tools_used` aggregated at completion (`letta/services/run_manager.py:412-439`).
- **Correlation IDs threaded end-to-end**: step rows capture `trace_id` and `request_id` (`letta/services/step_manager.py:157-158`), enabling `/runs/{id}/trace` span retrieval filtered for UI consumption (`letta/server/rest_api/routers/v1/runs.py:296-307`).
- **Goal continuity via summarizer contract**: compaction prompts require carrying forward high-level goals so multi-session work isn't lost (`letta/prompts/summarizer_prompt.py:7,30,52`).

## Tradeoffs

- **Loop-completion vs. goal-completion conflation**: mapping `max_steps`/`tool_rule` to `RunStatus.completed` keeps the client API simple but destroys the distinction between activity and achievement (`letta/schemas/letta_stop_reason.py:26-32`).
- **Availability over integrity**: logging-not-blocking invalid run transitions prevents stuck runs but permits history rewriting after terminal states (`letta/services/run_manager.py:341-356`).
- **NoopStepManager escape hatch**: telemetry can be fully disabled via a no-op implementation (`letta/services/step_manager.py:716-818`) and settings flags (`track_stop_reason`, `track_agent_run`, `letta/settings.py:385-386`) — operationally flexible, but progress visibility becomes deployment-dependent.
- **Approval-as-pause**: stopping the run entirely on `requires_approval` (mapped to `completed`) simplifies resumption but means a "blocked" run looks externally identical to a "done" run apart from the stop reason.

## Failure Modes / Edge Cases

- **Truncation masked as success**: a 50-step run cut off mid-task reports `completed` with `stop_reason=max_steps`; naive clients polling status will treat it as achieved.
- **Premature exit via terminal tool**: any model invocation of a `TerminalToolRule` tool ends the run successfully regardless of actual task state (`letta/agents/letta_agent_v3.py:2010-2012`).
- **Stuck-run containment is best-effort**: foreground streams force-fail on missing `[DONE]` (`letta/services/streaming_service.py:783-799`), but background runs defer to the processor; if that processor dies before writing terminal markers, the run could remain `running` until external reconciliation (none found in-source; `test_watchdog_hang.py` at repo root hints at known hang concerns).
- **Duplicate route registration**: `GET /{run_id}/metrics` is declared twice in the runs router (`letta/server/rest_api/routers/v1/runs.py:203-214` vs `letta/server/rest_api/routers/v1/runs.py:217-231`); the second handler's 404 handling is shadowed dead code — minor, but shows metric-surface maintenance drift.
- **Legacy divergence**: v1 (`letta_agent.py`) and v2 loops retain model-controlled `request_heartbeat` pacing while v3 removes it, so progress semantics differ by `AgentType` (`letta/schemas/enums.py:81-94`).
- **Status enum sprawl**: `JobStatus`, `RunStatus`, `AgentStepStatus`, and `StepStatus` coexist with acknowledged consolidation debt ("TODO (cliandy): consolidate this with job status", `letta/schemas/enums.py:164-173`).

## Future Considerations

- Add an explicit objective/success-criteria field on `Run` (or a Task entity) so completion can be evaluated against something rather than inferred from loop exit.
- Split `RunStatus.completed` into `completed` vs. `truncated` (or expose a first-class derived flag from `stop_reason`) to prevent false-positive polling clients.
- Enforce run lifecycle transitions (reject invalid updates) now that all writers share the canonical stop-reason mapping; the advisory-only guard undermines otherwise strong invariants.
- Add post-hoc verification hooks (e.g., optional evaluator tool or webhook-side validation contract) to independently check goal attainment.
- Deduplicate the `/runs/{id}/metrics` registration and consolidate Job/Run/Step status enums.

## Questions / Gaps

- No evidence found of any automated evaluation of whether a run accomplished its stated objective; search covered `letta/schemas/`, `letta/orm/`, `letta/services/run_manager.py`, and the summarizer prompts.
- Whether the cloud offering layers goal tracking onto these primitives cannot be verified from this source; several fields are annotated "(cloud only)" (`letta/schemas/step.py:60`, `letta/schemas/run_metrics.py:17`) with no local implementation.
- Reconciliation behavior for orphaned `running` background runs (processor crash between stream end and DB write) was not found in-source beyond the foreground-path watchdog; the root-level `test_watchdog_hang.py` suggests awareness but contains no server-side recovery mechanism.
- No evidence found that `feedback` on steps (positive/negative, `letta/schemas/step.py:57-59`) feeds back into any execution decision; it appears to be purely a data-collection affordance exposed via `list_steps_async` filters (`letta/services/step_manager.py:57`).

---

Generated by dimension `06.05-objective-and-progress-tracking` against `letta`.

# Source Analysis: agent-framework

## Dimension 06.05 — Objective and Progress Tracking

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (primary, `python/packages/*`), C#/.NET (`dotnet/src/*`), Go (early `go/`) |
| Analyzed | 2026-08-25 |

## Summary

Microsoft Agent Framework does not have a single global "goal object"; instead it offers several layered mechanisms that each represent the objective and measure progress toward it:

1. **Magentic multi-agent orchestration** — an explicit two-tier ledger model. A `_MagenticTaskLedger` (facts + plan) captures *what the goal is* (`python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:271-286`), while a `MagenticProgressLedger` with five structured slots (`is_request_satisfied`, `is_in_loop`, `is_progress_being_made`, `next_speaker`, `instruction_or_question`) captures *where we are* per round (`_magentic.py:307-334`). The ledger is produced by an LLM manager from a strict JSON prompt (`_magentic.py:194-243`), evaluated every inner-loop round (`_magentic.py:1086-1103`), and drives completion, stall-triggered replanning, and next-speaker selection.
2. **Agent loop harness** — a single-agent re-run loop (`AgentLoopMiddleware`) whose continuation is decided by a user-supplied `should_continue` predicate or by an LLM judge returning a structured `JudgeVerdict.answered` boolean (`python/packages/core/agent_framework/_harness/_loop.py:104-116, 265-347`). It maintains a per-iteration **progress log** injected back into the model's input (`_loop.py:731-745, 798-842`).
3. **Todo provider** — a persisted, tool-managed checklist (`TodoItem.is_complete`) that doubles as both goal decomposition and progress marker; `todos_remaining()` turns open items into a loop predicate (`_harness/_todo.py:51-64`; `_loop.py:925-986`).
4. **Background task registry** — framework-owned status tracking (`RUNNING/COMPLETED/FAILED/LOST`) derived from actual `asyncio.Task` state, not model claims (`_harness/_background_agents.py:46-53, 214-259`).
5. **Offline evaluation** — a provider-agnostic eval framework (`_evaluation.py`) with pass/fail checks, score thresholds, CI gates (`EvalResults.raise_for_status`), providing post-hoc independent success checking.

The dominant pattern: **progress is measured by LLM self-assessment inside bounded loops**, with numeric caps (rounds/stalls/resets/iterations) as operational safeguards, plus event streams (`output`, `superstep_completed`, `magentic_orchestrator` events), checkpoints, DevUI mapping, and OTel telemetry for observability.

## Rating

**Score: 7 / 10**

Rationale against the rubric (7–8 = clear model with tests, explicit interfaces, operational safeguards):

- **Clear model**: explicit typed goal/progress structures (`MagenticProgressLedger` `_magentic.py:307-334`, `TodoItem` `_todo.py:51-64`, `JudgeVerdict` `_loop.py:104-116`, `BackgroundTaskInfo` `_background_agents.py:55-111`) with serialization for durability (`DictConvertible` implementations at `_magentic.py:278-334, 348-384`).
- **Tests**: dedicated suites verify ledger parsing, stall/reset limits, round limits, judge behavior, feedback injection (`python/packages/orchestrations/tests/test_magentic.py:400, 537, 820`; `python/packages/core/tests/core/test_harness_loop.py:344, 760-862`).
- **Operational safeguards**: default iteration caps (`DEFAULT_MAX_ITERATIONS=10` `_loop.py:122`, judge `DEFAULT_JUDGE_MAX_ITERATIONS=5` `_loop.py:127`, stall→replan `_magentic.py:1116-1125`, reset limit termination `_magentic.py:1229-1265`).
- **Why not 8–9**: every in-loop completion decision is ultimately **model-judged without independent verification** — Magentic's `is_request_satisfied` comes from the same LLM family managing the run (`_magentic.py:699-739`); todo completion is self-reported by the model via a plain tool call with no verification beyond a required free-text reason (`_todo.py:144-176, 528-561`). The evaluation framework that *does* independently check success is offline-only, experimental (`@experimental(feature_id=ExperimentalFeature.EVALS)` `_evaluation.py:68-70`), and not wired into any in-loop gate. There is no built-in notion of test-execution-based progress inside the loop itself.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Goal object (task ledger) | `_MagenticTaskLedger` dataclass holds facts + plan messages; serialized to/from dict for checkpoints | python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:271-286 |
| Goal facts extraction prompt | Pre-survey prompt forces GIVEN/LOOK-UP/DERIVE/GUESSES fact taxonomy before planning | _magentic.py:108-136 |
| Plan creation prompt | Team-aware bullet plan derived from facts | _magentic.py:138-145 |
| Progress marker structure | `MagenticProgressLedger` with `is_request_satisfied`, `is_in_loop`, `is_progress_being_made`, `next_speaker`, `instruction_or_question` (each reason+answer) | _magentic.py:307-334 |
| Progress measurement prompt | JSON-schema progress prompt asks model to judge satisfaction, loops, forward progress | _magentic.py:194-243 |
| Progress evaluation cadence | Ledger created every inner-loop round; failure triggers reset+replan | _magentic.py:1086-1092 |
| Progress event emission | `PROGRESS_LEDGER_UPDATED` / `PLAN_CREATED` / `REPLANNED` events surfaced as workflow events | _magentic.py:786-800, 1094-1103 |
| Stall counter | `stall_count` incremented when no progress or loop detected; decremented on progress; > max_stall_count → reset+replan | _magentic.py:344-346, 1116-1125 |
| Completion criterion (Magentic) | `is_request_satisfied.answer == True` → `prepare_final_answer` → yield + terminate | _magentic.py:1110-1114, 1213-1227 |
| Hard limits | `max_round_count` / `max_reset_count` produce termination message and mark workflow terminated | _magentic.py:479-481, 1229-1265 |
| Checkpointed progress state | `on_checkpoint_save/restore` persists context, task ledger, progress ledger, manager state | _magentic.py:1273-1329 |
| User-defined termination | `TerminationCondition: Callable[[list[Message]], bool]` checked each round in group chat patterns | orchestrations/_base_group_chat_orchestrator.py:59, 338-372 |
| Round-limit guard (shared) | `_check_round_limit` forces completion at max rounds | _base_group_chat_orchestrator.py:499-518 |
| Human approval of plan | `require_plan_signoff` pauses workflow for human approve/revise; revision loop until approved | _magentic.py:805-858, 993-1055 |
| Loop stop predicate | `should_continue` callable receives iteration/result/messages/session/progress/feedback kwargs | core/agent_framework/_harness/_loop.py:130-143, 265-347 |
| Judge verdict type | `JudgeVerdict(answered: bool, reasoning: str)` structured output decides loop exit | _loop.py:104-116 |
| Judge fallback parsing | Non-overlapping `VERDICT: DONE`/`VERDICT: MORE` markers; ambiguity keeps looping (fail-safe) | _loop.py:71-87, 192-201 |
| Iteration safety cap | Default 10 iterations (judge: 5); cap short-circuits before expensive predicates | _loop.py:119-127, 747-758 |
| Progress log injection | Per-iteration entries recorded and rendered as "Progress so far:" user message into next input | _loop.py:731-745, 798-802, 824-842 |
| Todo item model | `TodoItem(id, title, description, is_complete)` persisted in session state or JSON file | _harness/_todo.py:51-106 |
| Todo completion requires reason | `todos_complete` demands non-empty `reason` string per completed id | _todo.py:144-176, 528-561 |
| Todo-driven loop predicate | `todos_remaining()` resolves `TodoProvider` from running agent; loops while any `not is_complete` | _loop.py:925-986 |
| Open-todo nudge message | `todos_remaining_message()` lists open titles and instructs agent to finish them | _loop.py:989-1021 |
| Background task statuses | `RUNNING/COMPLETED/FAILED/LOST` enum on `BackgroundTaskInfo` incl. error_text | _harness/_background_agents.py:46-53, 55-111 |
| Ground-truth status refresh | Status updated from actual `asyncio.Task.done()/exception()`, not model claims; missing task marked LOST | _background_agents.py:214-259 |
| Offline success checks | `LocalEvaluator` checks (`keyword_check`, `tool_called_check`, arg matching) return pass/reason | core/agent_framework/_evaluation.py:1035-1148, 1213-1276 |
| CI gate on results | `EvalResults.raise_for_status()` / `assert_score_at_least()` raise `EvalNotPassedError` on failures | _evaluation.py:470-500, 502-543 |
| Workflow-level progress events | Event types: started/status/superstep_started/superstep_completed/executor_invoked/completed/failed/bypassed/output/intermediate/request_info/warning/error | _workflows/_events.py:104-130 |
| Superstep events emitted | Runner emits superstep_started/completed per Pregel iteration | _workflows/_runner.py:123, 168 |
| Run result accessors | `WorkflowRunResult.get_outputs/get_intermediate_outputs/get_final_state/status_timeline` | _workflows/_workflow.py:103-166 |
| Run states | STARTED/IN_PROGRESS(+PENDING_REQUESTS)/IDLE(+PENDING_REQUESTS)/FAILED/CANCELLED | _events.py:58-67 |
| Structured blocker record | `WorkflowErrorDetails(error_type, message, traceback, executor_id)` attached to failed/error events | _events.py:70-99, 334-337 |
| UI status mapping | DevUI converts workflow events to OpenAI Responses events (`response.created/in_progress`); tags outputs terminal vs intermediate | packages/devui/agent_framework_devui/_mapper.py:75-81, 901-954 |
| Event forwarding boundary | Only output/intermediate/data/request_info cross the `as_agent()` boundary; internal orchestration events stay inside | _events.py:133-143 |
| Function-loop budgets | `max_iterations` / `max_function_calls` bound tool-call progress; spec maps scenarios to tests | docs/specs/004-python-function-calling-loop.md:524-533 |
| .NET parity | `MagenticProgressLedger` mirrors same five slots incl. `IsRequestSatisfied`/`IsProgressBeingMade` | dotnet/src/Microsoft.Agents.AI.Workflows/MagenticProgressLedger.cs:17-128 |
| Test: ledger parse failure | Invalid ledger JSON raises after retries (orchestrator then resets) | orchestrations/tests/test_magentic.py:537-562 |
| Test: stall/reset limits | Not-progressing manager terminates with "maximum reset count" message; IDLE status asserted | tests/test_magentic.py:820-838 |
| Test: round limit partial result | Round-limited run still yields output event | tests/test_magentic.py:400 |
| Tests: judge loop semantics | Judge stops when answered, continues otherwise, ambiguous text keeps looping, criteria injection verified | core/tests/core/test_harness_loop.py:760-931 |
| Tests: todo tools | Provider tools create/complete/remove/query todos under session-state persistence | core/tests/core/test_harness_todo.py:242-367 |

## Answers to Dimension Questions

**1. What is the goal?**
There is no universal goal primitive. In Magentic, the goal is the user task string captured in `MagenticContext.task` (`_magentic.py:337-356`) and elaborated into a task ledger (facts + plan) by the manager (`_magentic.py:621-652`). In single-agent harnesses, the goal lives outside the framework as the original input messages (`original_messages` preserved across iterations, `_loop.py:431`) optionally sharpened by user-declared `criteria` injected into both agent instructions and judge instructions (`_loop.py:90-101, 387-394`). With todos, the goal is decomposed into persisted `TodoItem`s created by the model itself (`_todo.py:506-526`).

**2. How is progress measured?**
Four complementary signals, all explicitly implemented:
- **Model judgment**: the Magentic manager classifies each round along five dimensions via forced-JSON output with retries and backoff (`_magentic.py:699-739`); a judge chat client scores "was the request fully addressed?" in the loop middleware (`_loop.py:153-213`).
- **Tool/state success**: `todos_remaining` reads persisted todo completeness (`_loop.py:959-984`); `background_tasks_running` reads framework-refreshed asyncio task status (`_loop.py:845-884`).
- **Counters**: round/stall/reset counts (`_magentic.py:344-346`) and iteration caps (`_loop.py:119-127`) convert qualitative judgments into quantitative bounds.
- **Offline tests**: `LocalEvaluator` checks and Foundry cloud evaluators score final artifacts (`_evaluation.py:1060-1148`), intended for CI (`raise_for_status` `_evaluation.py:470-500`). Tool-success *inside* the function-calling loop is measured only by budget accounting (`max_function_calls`, spec `docs/specs/004-python-function-calling-loop.md:532`), not by outcome quality.

**3. Can the model fake progress?**
Partially yes, by design tradeoff:
- A model can mark its own todos complete — `todos_complete` only validates that ids exist and reasons are non-empty strings; there is no verification that work happened (`_todo.py:540-561`). Mitigation: the loop predicate will then stop looping, so faking completion ends the run rather than extending it; the recorded reason provides an audit trail but nothing checks it.
- Magentic's `is_request_satisfied` is emitted by the same LLM that coordinates agents; a hallucinated "satisfied" ends the workflow with a synthesized answer (`_magentic.py:1110-1114, 1213-1227`). No second-opinion mechanism exists inside Magentic itself (human plan review gates the *plan*, not the completion claim, `_magentic.py:885-903`).
- What cannot be faked: background-task status (derived from real `asyncio.Task` state, `_background_agents.py:234-259`), superstep/executor events (emitted by the runner, `_runner.py:123, 168`), and budget counters (middleware-enforced). The judge pattern adds independence at the *process* level (separate client, `_loop.py:159-170`) but not at the *ground-truth* level — it is still an LLM reading the agent's claims, and the docstring explicitly warns about untrusted judges steering the loop via manipulated verdicts (`_loop.py:369-384`).

**4. Are blockers recorded?**
Yes, in several forms: progress-ledger items carry a mandatory `reason` per judgment including loop/no-progress diagnoses (`_magentic.py:220-242`); replanning prompts ask the model to explain the root cause of the last failure (`_magentic.py:186-192`); `WorkflowErrorDetails` captures type/message/traceback/executor for failures (`_events.py:70-99`); background tasks persist `error_text` and a distinct `LOST` status for vanished tasks (`_background_agents.py:46-53, 220-231`); eval items carry `error_code`/`error_message` distinguishing infrastructure errors from quality failures (`_evaluation.py:326-369`). Human intervention on stall can inject guidance messages into history (`with_human_input_on_stall`, documented at builder level `_magentic.py:1391-1400`).

**5. Is final success independently checked?**
Not within the execution path. Termination is declared by the model (Magentic satisfied flag) or by user predicates (termination condition `_base_group_chat_orchestrator.py:338-372`; `should_continue` `_loop.py:747-763`). Independent verification exists only as an optional offline layer: `evaluate_agent(...)` with `LocalEvaluator` or Foundry evaluators, gated via `raise_for_status()`/`assert_score_at_least()` for CI (`_evaluation.py:470-543`), and it is flagged experimental (`_evaluation.py:68`). The judge loop is the closest in-loop analogue, but its independence is limited to being a separate LLM call with fresh messages (`_loop.py:158-186`).

## Architectural Decisions

1. **Ledger-as-message, not ledger-as-database**: Magentic represents goals/plans as `Message` objects inside chat history (`_MagenticTaskLedger.facts/plan` are `Message`s, `_magentic.py:275-276`), so progress reasoning stays grounded in the conversation the participants see, at the cost of structured querying.
2. **Judgment centralized in a manager role**: all progress assessment flows through `MagenticManagerBase.create_progress_ledger` (`_magentic.py:495-498`), making the measurement strategy swappable (custom managers supported via `manager_factory`, `_magentic.py:1604-1721`) but concentrating trust in one model.
3. **Progress as injectable context**: the loop's progress log and the todo list are fed back to the model as user messages ("Progress so far:", "Current todo list") rather than hidden bookkeeping (`_loop.py:798-802`; `_todo.py:596-614`) — making the agent aware of its own trajectory.
4. **Bounded autonomy everywhere**: every loop has a numeric escape (iterations, rounds, stalls, resets); defaults are conservative and opt-out is explicit (`max_iterations=None`, `_loop.py:119-127`).
5. **Framework-owned vs model-owned status split**: background-task truth is owned by the runtime (`asyncio.Task` inspection), while task-completion truth is owned by the model (todos, ledger) — a deliberate line between what can and cannot be trusted to self-report.
6. **Event-sourced observability**: one generic `WorkflowEvent` class with a type discriminator covers lifecycle, data, requests, diagnostics, iteration, and orchestration events (`_events.py:104-146`), consumed uniformly by streaming callers, `WorkflowRunResult`, DevUI, and checkpointing.

## Notable Patterns

- **Fail-safe verdict parsing**: when the judge's structured output is unavailable, ambiguous text falls back to "keep looping" (`MORE wins`, `_loop.py:192-201`) — biasing toward continued work over premature stop. Conversely, when the Magentic progress ledger cannot be parsed after retries, the orchestrator treats it as a stall signal and resets+replans instead of guessing (`_magentic.py:1086-1092`; test `test_magentic.py:556-562` documents the raise-to-reset contract).
- **Stall hysteresis**: `stall_count` decays when progress resumes (`stall_count = max(0, stall_count - 1)`, `_magentic.py:1117-1120`) so isolated bad rounds don't trigger replans.
- **Session snapshot for fresh-context loops**: `fresh_context=True` snapshots the session pre-loop and restores it between iterations, so continuity is carried *only* by the progress log — an elegant way to prevent stale history from masking lack of progress (`_loop.py:431-436, 583-586`; tests `test_harness_loop.py:454-533`).
- **Mode-gated looping**: `todos_remaining(looping_modes=[...])` lets teams restrict auto-continuation to an execution mode so planning phases don't spin (`_loop.py:925-957`).
- **Output designation discipline**: caller-visible progress is curated — `output_from`/`intermediate_output_from` decide which executor emissions are terminal vs intermediate vs hidden (`_workflow.py:172-205`; DevUI tagging `_mapper.py:75-81`).

## Tradeoffs

- **Self-reported progress is cheap but spoofable**: requiring only a reason string for todo completion keeps latency low but means completion claims are unverifiable without external checks; the alternative (framework-verified completion) doesn't exist for arbitrary tasks.
- **LLM-judged progress costs tokens and adds variance**: the progress ledger runs an extra model call per round with up to 3 parse retries (`_magentic.py:588-590, 722-735`); judge loops add another client per iteration.
- **Termination message conflates failure with completion**: hitting round/reset limits yields a normal-looking assistant message ("Workflow terminated due to reaching maximum reset count.", `_magentic.py:1255-1261`) through the same output channel as genuine answers — callers must inspect text or events to distinguish exhausted-budget from success (tests assert exact wording, `test_magentic.py:838`).
- **Rich observability surface, filtered by default**: `as_agent()` hides orchestration-internal events (including `magentic_orchestrator` progress events) from agent consumers (`_events.py:133-143`), which simplifies the surface but hides exactly the progress evidence this dimension cares about unless callers drop to workflow level.

## Failure Modes / Edge Cases

- **Progress-ledger JSON drift**: model output that isn't clean JSON exhausts retries and raises; handled by converting to reset+replan (`_magentic.py:737-739`, `1086-1092`), consuming reset budget. A helper even patches Python-style `True/False/None` literals (`_magentic.py:437-441`), acknowledging real-world parser fragility.
- **Invalid next speaker**: if the ledger names a non-participant, the orchestrator short-circuits to final-answer synthesis rather than deadlocking (`_magentic.py:1128-1138`); non-string answers fall back to first participant (`1130-1132`).
- **Runaway loops**: possible if users pass `max_iterations=None` and write a never-false predicate — the docstring warns the responsibility shifts entirely to `should_continue` (`_loop.py:259-262`).
- **Stuck background tasks**: a persisted RUNNING task whose asyncio task vanished becomes LOST only when refreshed (`_background_agents.py:245-249`); the loop helper explicitly advises pairing with `max_iterations` because "a task's persisted status [may] never [be] refreshed" (`_loop.py:871-874`).
- **Checkpoint restore degradation**: ledger/context restore failures are swallowed to warnings and nulled (`_magentic.py:1299-1328`), meaning a resumed run may silently lose its progress baseline rather than fail loudly.
- **Duplicate-history regression**: manager sessions previously duplicated task/plan context each round; fixed by throwaway sessions per manager call (regression #4371 documented at `_magentic.py:600-611`), showing how easily progress context can compound incorrectly.

## Future Considerations

- Wire the evaluation layer into in-loop gating (e.g., evaluator-backed `should_continue`) so final-success checking is not purely offline; today `LocalEvaluator` and `raise_for_status()` are CI-oriented only (`_evaluation.py:22-32, 470-500`).
- Add verification hooks to `todos_complete` (e.g., require linked artifact/tool evidence) to harden self-reported completion (`_todo.py:528-561`).
- Surface a machine-readable distinction between exhausted-budget terminations and satisfied completions (typed result payload instead of prose message, cf. `_magentic.py:1251-1261`).
- Promote progress-ledger/judge telemetry to first-class OTel spans; current tracing centers on chat calls (`ChatTelemetryLayer`, `observability.py:1528+`), while progress assessments are only visible as workflow events/logs.

## Questions / Gaps

- **Go implementation maturity**: `go/` contains only a README (`go/README.md`); no Go progress-tracking code was found, so parity claims rest on Python/.NET only (.NET evidence: `dotnet/src/Microsoft.Agents.AI.Workflows/MagenticProgressLedger.cs:17-128`).
- **No evidence found** for milestone records beyond the ledger/todo/task trio — searched for `milestone`, `percent`, and completion-percentage concepts across `python/packages/core` and `packages/orchestrations`; nothing matched.
- **No evidence found** that Magentic progress judgments incorporate tool-result outcomes directly (e.g., "the file now exists"); assessment is conversational only — grounded in chat history (`_magentic.py:719-723`), not in side-effect verification.
- Whether DevUI renders `magentic_orchestrator` progress-ledger payloads visually could not be confirmed from code read (`_mapper.py:885-954` handles lifecycle/output conversion; no magentic-specific rendering located) — stated design boundary: mapper converts workflow events generically.

---

Generated by dimension `06.05-objective-and-progress-tracking` against `agent-framework`.

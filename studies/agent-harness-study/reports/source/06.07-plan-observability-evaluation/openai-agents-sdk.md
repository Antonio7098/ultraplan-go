# Source Analysis: openai-agents-sdk

## Dimension 06.07: Plan Observability and Evaluation

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (Agents SDK) |
| Analyzed | 2026-08-27 |

## Summary

`openai-agents-sdk` is a reactive loop-based agent harness, not a planner-based harness. There is no user-visible `Plan`, `PlanItem`, or `PlanStep` abstraction. The only object named "plan" is the internal, ephemeral `ToolExecutionPlan` (`src/agents/run_internal/tool_planning.py:557-574`) that partitions a single model turn's tool calls (function runs, computer actions, shell calls, approvals) before parallel execution. That plan is not logged, not traced, not persisted to `RunState`, and not evaluated. Observability exists for execution ( `RunItem` history, `RunState` turn snapshot, OpenTelemetry-like tracing with `trace`/`span` ), but none of it reifies a plan to compare against execution. No plan traces, plan item IDs, execution links, eval datasets, planning metrics, or planning regression tests were found.

## Rating

**2 / 10 — Absent / Implicit**

Rationale: The dimension expects explicit plan artifacts that are logged, linked to executed actions, and evaluated. This source has no plan artifact. Tracing covers tasks/turns/agents/generations/functions but not plans. `ToolExecutionPlan` is an internal scheduler, invisible to callers, with zero persistence or export. No planning evaluation harness or regression fixture exists in the isolated `src/` snapshot.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Plan trace — absent | No `PlanSpanData` or `plan` span type exists; enumerated span types are `agent, task, turn, function, generation, response, handoff, custom, guardrail, transcription, speech, speech_group, mcp_tools` | `src/agents/tracing/span_data.py:28-451` |
| Plan trace — task/turn coverage only | `TaskSpanData` exports `sdk_span_type=task` with `name, usage`; `TurnSpanData` exports `turn, agent_name, usage`; neither carries plan items | `src/agents/tracing/span_data.py:64-133` |
| Execution plan definition | Internal `@dataclass ToolExecutionPlan` with fields `function_runs, computer_actions, custom_tool_calls, shell_calls, apply_patch_calls, local_shell_calls, pending_interruptions, approved_mcp_responses, mcp_requests_with_callback` | `src/agents/run_internal/tool_planning.py:557-574` |
| Plan construction (fresh turn) | `_build_plan_for_fresh_turn()` builds plan from `ProcessedResponse` + MCP approval plan; returns `ToolExecutionPlan(...)` | `src/agents/run_internal/tool_planning.py:619-646` |
| Plan construction (resume) | `_build_plan_for_resume_turn()` builds plan for resumed HITL turn | `src/agents/run_internal/tool_planning.py:649-682` |
| Plan execution | `_execute_tool_plan(plan: ToolExecutionPlan, ...)` fans out to `execute_function_tool_calls`, `execute_computer_actions`, etc., via `gather_with_cancel`; no span emission or logging of the plan itself | `src/agents/run_internal/tool_planning.py:944-1101` |
| Plan consumption in turn loop | `plan = _build_plan_for_fresh_turn(...)` then `await _execute_tool_plan(plan=plan, ...)`; plan fields are burst into step items but plan object is discarded | `src/agents/run_internal/turn_resolution.py:828-894` |
| Plan consumption (resume) | `plan = _build_plan_for_resume_turn(...)` then `_execute_tool_plan(plan=plan, ...)` same discard pattern | `src/agents/run_internal/turn_resolution.py:2403-2530` |
| Result observability — no plan items | `RunResultBase.new_items: list[RunItem]` and `RunResultBase.to_input_list()` expose execution history only; no `plan`/`planItems` field | `src/agents/result.py:308-320` |
| RunState persistence — no plan | `RunState` persists `generated_items, session_items, model_responses, input_guardrail_results, pending_input, tool_use_tracker_snapshot, trace_state, sandbox` but no `plan` snapshot | `src/agents/run_state.py:761-878` |
| Tracing span hierarchy — no plan | `SpanImpl` stores `trace_id, span_id, parent_id, started_at, ended_at, span_data, error, trace_metadata`; plan ID is not a field | `src/agents/tracing/spans.py:289-423` |
| Trace export — no plan | `TraceImpl.export()` emits `object:trace, id, workflow_name, group_id, metadata`; `SpanImpl.export()` emits `object:trace.span, id, trace_id, parent_id, started_at, ended_at, span_data, error, metadata` — neither includes plan linkage | `src/agents/tracing/traces.py:568-575` / `src/agents/tracing/spans.py:396-423` |
| Trace creation API | Public tracing helpers are `trace, agent_span, task_span, turn_span, function_span, generation_span, response_span, handoff_span, custom_span, guardrail_span`; no `plan_span` | `src/agents/tracing/create.py:31-491` |
| Item identity — tool call, not plan item | `ToolCallItem.call_id` and `ToolCallOutputItem.call_id` provide tool invocation linkage, but no `planItemId` | `src/agents/items.py:412-461` |
| Handoff linkage — not plan | `HandoffCallItem`, `HandoffOutputItem` track agent-to-agent transfers with `source_agent/target_agent`; no plan step | `src/agents/items.py:299-365` |
| Execution link via call_id | `tool_invocation_identity` / `tool_output_identity` correlate calls to outputs; used for deduplication, not plan-item linking | `src/agents/run_internal/tool_planning.py:102-116` |
| Usage metrics — not plan metrics | `AgentSpanData.tools, output_type`, `TaskSpanData.usage`, `TurnSpanData.usage`, `GenerationSpanData.usage` carry token/latency, not plan quality (adherence, completeness, depth) | `src/agents/tracing/span_data.py:34-62,64-95,98-133,169-210` |
| Eval / harness search — no planning eval | `grep eval|dataset|metric` across `src/` returns only `harness_id` (OpenAI harness registration), `tool_use_tracker`, and retry `_evaluate_policy`; no planning eval harness, dataset loader, or metric registry | `src/agents/models/openai_agent_registration.py:10,45-86` / `src/agents/run_internal/tool_use_tracker.py` (via grep) |
| Visualization — no plan view | `extensions/visualization.py` renders agent graph, not plan traces | `src/agents/extensions/visualization.py` (glob) |
| Sandbox prompts — planning instruction, not observability | Sandbox system prompt advises progress updates for multi-step plans ("plan with multiple steps") but no structured plan artifact or logging | `src/agents/sandbox/instructions/prompt.md:102` |

## Answers to Dimension Questions

### 1. Are plans observable?

**No.** The SDK has no first-class `Plan` type. The model free-forms reasoning and tool calls each turn; the runner iterates `turn -> model -> ProcessedResponse -> ToolExecutionPlan -> execution -> next turn` (`src/agents/run_internal/turn_resolution.py:828-894`, `src/agents/run.py:966+`). `ToolExecutionPlan` (`src/agents/run_internal/tool_planning.py:557`) is an internal scheduler that partitions one turn's tool calls; it is never traced (`src/agents/tracing/span_data.py:28-451`), never appended to `RunResult.new_items` (`src/agents/result.py:315`), never serialized into `RunState` (`src/agents/run_state.py:761-878`), and never exported via `ConsoleSpanExporter`/`BackendSpanExporter` (`src/agents/tracing/processors.py:27-245`). Execution is observable (every `RunItem` and model response is retained), but there is no durable plan artifact to observe.

### 2. Can each action be linked to a plan item?

**No — partially via tool `call_id`, but not to a plan item.** Tool calls and outputs are correlated by `call_id`/`id` (`src/agents/items.py:412-461`, `src/agents/run_internal/tool_planning.py:102-116`) and weak agent references (`src/agents/items.py:98-133`). Turns are grouped under `TurnSpanData.turn` (`src/agents/tracing/span_data.py:98-108`) nested inside `TaskSpanData`/`AgentSpanData`. However, there are no `planId`/`planItemId` fields on `RunItem`, no `PlanItem` type, and no `plan_span` parent. The only grouping is per-turn `ToolExecutionPlan` which is destroyed after `_execute_tool_plan` (`src/agents/run_internal/turn_resolution.py:858`). Callers cannot answer "which planned step did this tool call satisfy?" from the snapshot alone.

### 3. Are plans evaluated?

**No.** No evaluation harness, dataset, or metric for planning was found in the isolated `src/` directory. The codebase contains no `eval/`, `datasets/`, or `benchmarks/` tree, no `planning_metrics`, no `plan_score`, and no scorer validating plan completeness, feasibility, or adherence. The closest mechanisms are `Guardrail` (`src/agents/guardrail.py` via `src/agents/tracing/span_data.py:292-315`) and `ToolGuardrail` for input/output validation — those are safety checks, not plan quality evaluators. `AgentToolUseTracker` tracks which tools an agent used, but is not a plan evaluator.

### 4. Can poor planning be diagnosed?

**Only indirectly, by comparing execution traces to model reasoning.** A failed run can be debugged by replaying `RunResult.raw_responses` (`src/agents/result.py:320`), `new_items` sequences, and timing in `TaskSpan`/`TurnSpan`/`GenerationSpan`/`FunctionSpan` (`src/agents/tracing/span_data.py:28-210`), plus span errors (`src/agents/tracing/spans.py:18-29`). `MaxTurnsExceeded` and guardrail tripwires surface as `RunErrorDetails` (`src/agents/result.py:1027`). What is missing: no snapshot of the intended plan vs actual execution, no `planned_steps` vs `executed_steps` diff, no plan adherence or backtracking metric, no reason code for plan deviation. The sandbox instructions even encourage informal textual progress updates for long tasks (`src/agents/sandbox/instructions/prompt.md:102`), confirming that planning is prompt-based, not instrumented.

### 5. Does planning improve success rate?

**No evidence found.** No experiment, cohort study, A/B harness, or metric ties an explicit planning step (e.g., `plan-before-act`, decomposition) to success rate. The loop's success is measured by reaching `NextStepFinalOutput` or guardrail pass/fail, not by plan quality. The isolated source contains no evaluation tying planning interventions to task success.

## Architectural Decisions

- **Reactive turn loop over explicit planner.** The harness models execution as `ProcessedResponse -> ToolExecutionPlan -> parallel tool executors` (`src/agents/run_internal/tool_planning.py:944`) rather than `GeneratePlan -> ExecutePlan -> Replan`. This keeps the model autonomous and the runner simple, at the cost of zero plan observability.
- **Internal-only execution plan.** Making `ToolExecutionPlan` private (`_build_plan_for_fresh_turn` / `_build_plan_for_resume_turn`) avoids committing to a public planning contract. Tradeoff: callers and tracers cannot reconstruct or audit the plan.
- **Tracing focused on model/tool boundaries.** Span taxonomy mirrors model calls (`GenerationSpanData`, `ResponseSpanData`), agent invocations (`AgentSpanData`), and tool invocations (`FunctionSpanData`, `ToolSearch*`), not on planning. Task and turn spans provide coarse time-series grouping (`src/agents/tracing/span_data.py:64-133`) but no plan semantics.
- **Durable `RunState` without plan.** `RunState` is designed to survive HITL approval gaps and server-managed conversation tracking (`src/agents/run_state.py:191-230` schema policies), persisting `generated_items`, `context`, `trace_state`, and `pending_session_write`. Plans are excluded from durability, so resumed runs cannot compare resumed execution to a prior plan snapshot.

## Notable Patterns

- **Per-turn fan-out planner:** Pattern: collect finished tool calls -> build plan -> `gather_with_cancel` parallel executors -> collect results/interruptions. Seen at `src/agents/run_internal/tool_planning.py:984-1036` and `src/agents/run_internal/turn_resolution.py:828-894`. Applies to all tool modalities (function, computer, shell, MCP approvals).
- **Call-ID identity as the only execution link:** `tool_invocation_identity` + `call_id` deduplicates and correlates calls across streaming/history (`src/agents/run_internal/tool_planning.py:102-116`, `src/agents/items.py:412-461`). This is the sole stable join key; no higher-level plan key overlays it.
- **Tracing Processor interface for pluggable observability:** `TracingProcessor`/`BackendSpanExporter`/`BatchTraceProcessor` (`src/agents/tracing/processors.py:541+`) allow custom plan tracing to be added via `custom_span`, but the SDK itself does not use it for plans.

## Tradeoffs

- **Simplicity vs auditability.** Not reifying plans keeps the agent protocol minimal (model outputs tool calls directly) and keeps the runner model-agnostic. The price is that compliance, safety, and debugging use cases that require "did the agent follow the approved plan?" have no native answer.
- **Performance vs fidelity.** `_execute_tool_plan` isolates parallel failures (`sibling_category_failure`, `isolate_function_tool_failures`) (`src/agents/run_internal/tool_planning.py:965-976`) to avoid cascading failures cheaply, but discards the structured ordering/dependency that an explicit DAG plan would provide.
- **Backend tracing coupling.** `BackendSpanExporter` (`src/agents/tracing/processors.py:44-320`) forwards spans to `https://api.openai.com/v1/traces/ingest`. Any plan spans would inherit that vendor lock unless routed through `custom_span` + custom processor. Today the SDK avoids that commitment by not spanning plans at all.

## Failure Modes / Edge Cases

- **Silent re-planning.** Because there is no plan artifact, the model can change its decomposition each turn without detection; reviewers see only the final `new_items` history and cannot distinguish intentional replanning from failure.
- **Approval gaps break assumed ordering.** `_collect_mcp_approval_plan` (`src/agents/run_internal/tool_planning.py:593`) splits approvals into callback vs manual interruptions; pending `ToolApprovalItem` is stored in `RunState._current_step` (`src/agents/run_state.py:846`) but without a plan snapshot to show what was deferred, making it hard to tell whether later tool calls were planned pre-approval or opportunistic.
- **Duplicate `call_id` hazards are execution-level only.** `_dedupe_processed_response_invocations` (`src/agents/run_internal/tool_planning.py:339-554`) guards against reuse of completed invocations, but there is no plan-level guard against executing the same planned step twice or skipping a planned step.
- **Max-turns exhaustion masks planning debt.** `MaxTurnsExceeded` (`src/agents/result.py:1052`) fires when `current_turn > max_turns` with no attribution to which plan steps consumed turns; poor decomposition that wastes turns is indistinguishable from legitimately hard tasks.
- **Trace export loss.** `BatchTraceProcessor` drops spans when its queue is full (`src/agents/tracing/processors.py:602-621`); even if callers instrument private plan spans via `custom_span`, they inherit the same lossy guarantee, and the SDK's own batcher does not prioritize plan-critical spans.
- **Session persistence replay ambiguity.** `RunState._pending_session_write` (`src/agents/run_state.py:873`) holds the last canonical append, but without a plan reference, reconciling duplicate writes after a failure cannot verify whether the re-executed batch still satisfies the original plan.

## Future Considerations

- **Introduce a public `Plan`/`PlanItem` type with IDs and attach it to traces.** Minimal: `PlanSpanData(id, items: [{id, description, tool_name, status, dependencies}])` + `plan_item_id` on `ToolCallItem` + `FunctionSpanData`. Emit a `plan_span` per turn (or per run) before `_execute_tool_plan` and link each `function_span` as a child via `parent_id`.
- **Persist `plan_snapshot` in `RunState`** alongside `generated_items` and `trace_state` so that `Runner.run(state)` can diff planned vs executed steps after an interruption, and `to_state().to_json()` retains plan lineage (extend `CURRENT_SCHEMA_VERSION` at `src/agents/run_state.py:191`).
- **Add plan evaluation harness.** Even a lightweight harness that scores `plan_completeness` (planned steps covered), `plan_adherence` (executed steps were planned), and `plan_efficiency` (turns per successful step) from `RunResult.new_items` + `plan_snapshot` would satisfy the dimension without external datasets.
- **Consider `custom_span("plan", data={items})` as interim** if a public plan type is out of scope; document it as the plan trace contract and add a `TracingProcessor` example that exports it alongside existing spans.

## Questions / Gaps

- Is planning intentionally out-of-scope for this SDK (left to caller prompts/orchestrators), or is an explicit planner on the roadmap? No README or `ARCHITECTURE.md` is present in the isolated `src/` snapshot to confirm intent.
- The current distribution ships only `src/`; tests, examples, and eval fixtures (if any) live outside the isolation boundary, so absence here means "no evidence in the isolated source," not proof of absence in the full repository.
- No plan quality metrics (e.g., time-to-first-successful-tool, replan count, wasted turn rate) are exported from `Usage` or tracing today; it is unclear whether callers are expected to compute those from `new_items` themselves.

---
Generated by `Dimension 06.07: Plan Observability and Evaluation` against `openai-agents-sdk`.

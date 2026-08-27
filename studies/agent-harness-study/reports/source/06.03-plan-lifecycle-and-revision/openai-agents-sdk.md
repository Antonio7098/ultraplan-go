# Source Analysis: openai-agents-sdk

## 06.03 Plan Lifecycle and Revision

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python / OpenAI Responses & ChatCompletions SDK, asyncio |
| Analyzed | 2026-08-27 |

## Summary

`openai-agents-sdk` has **no first-class `Plan` artifact**. Task decomposition, step sequencing, and revision are delegated entirely to the LLM. The framework's durable lifecycle is a **turn loop** (`Runner` → `run_single_turn` → `SingleStepResult.next_step`) checkpointed as `RunState`. Creation, update, interruption, completion, and persistence are modeled around model responses and tool executions, not an explicit plan object that can be authored, diffed, revised, or validated. A `ToolExecutionPlan` exists but is an ephemeral per-turn tool-dispatch plan (`sources/openai-agents-sdk/src/agents/run_internal/tool_planning.py:558`), not a user-visible task plan. `RunState.to_json()/from_json()` provides durable history via append-only lists (`_generated_items`, `_session_items`, `_model_responses`), not versioned plan revisions, and there is no drift detector or justification field for why the trajectory changed.

## Rating

**Score: 3 / 10 — Absent, implicit, ad-hoc**

Rationale: The dimension requires creation → update → replanning → completion → persisted revision history → drift detection. All five are implicit in the model turn loop. There is no `Plan` type, no `update_plan` API, no revision counter, no `replan` trigger, and no `plan drift` metric. Durable `RunState` serializes *execution history* well (schema 1.17, 17 versions tracked in `sources/openai-agents-sdk/src/agents/run_state.py:191-229`), but does not capture plan semantics. Changes are model-justified only via generated text/traces, not explicit metadata. Completion is delegated to `AgentOutputSchema` validation and `ToolsToFinalOutputResult` triage. This is by design — the SDK is a tool-orchestration harness, not a planning harness — but against the rubric it is "absent/implicit".

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Plan creation (implicit) | `RunState.__init__` initializes durable snapshot (`_current_turn=0`, `_original_input`, `_current_agent`) — the run's "plan" is the turn-0 state that will be expanded turn-by-turn. Also `AgentRunner._run_impl` creates fresh `RunState` when `input` is not a state. | `sources/openai-agents-sdk/src/agents/run_state.py:879-924` , `sources/openai-agents-sdk/src/agents/run.py:775-783` |
| Plan creation (turn) | `Runner.run`/`AgentRunner.run` doc describes loop 1-4: call model, check final_output, handoff, else run tools and loop again. No explicit plan object constructed before loop. | `sources/openai-agents-sdk/src/agents/run.py:275-283` , `sources/openai-agents-sdk/src/agents/run.py:543-578` |
| Plan update / next-step | `ProcessedResponse` → `SingleStepResult.next_step` union (`NextStepHandoff`, `NextStepFinalOutput`, `NextStepRunAgain`, `NextStepInterruption`) is the plan-transition decision per turn. Mutations occur in `turn_resolution.py` and `run_loop.py`. | `sources/openai-agents-sdk/src/agents/run_internal/run_steps.py:156-232` , `sources/openai-agents-sdk/src/agents/run_internal/turn_resolution.py:790-1100` |
| Tool execution plan | `ToolExecutionPlan` dataclass (function_runs, computer_actions, interruptions, etc.) plus `_build_plan_for_fresh_turn` / `_build_plan_for_resume_turn` and `_execute_tool_plan`. Ephemeral, not persisted beyond turn. | `sources/openai-agents-sdk/src/agents/run_internal/tool_planning.py:558-680` , `sources/openai-agents-sdk/src/agents/run_internal/tool_planning.py:944-985` |
| Replanning trigger (tool output) | `_maybe_finalize_from_tool_results` / `check_for_final_output_from_tools` consults `Agent.tool_use_behavior` (`run_llm_again`, `stop_on_first_tool`, `StopAtTools`, callable returning `ToolsToFinalOutputResult`) to decide if tool output becomes final output or loop continues. | `sources/openai-agents-sdk/src/agents/run_internal/turn_resolution.py:753-773` , `sources/openai-agents-sdk/src/agents/agent.py:373-388` |
| Replanning trigger (handoff) | `HandoffInputFilter`, `nest_handoff_history`, and `_nest_handoff_history_with_provenance` rewrite `original_input` / `generated_items` on handoff; handoff creates new agent and optionally filters/nests history, effectively replanning via agent switch. | `sources/openai-agents-sdk/src/agents/handoffs/__init__.py:161-174` , `sources/openai-agents-sdk/src/agents/handoffs/history.py:97-156` , `sources/openai-agents-sdk/src/agents/run_internal/turn_resolution.py:540-745` |
| Interruption / HITL plan pause | `NextStepInterruption` holds `interruptions: list[ToolApprovalItem]` and flags `response_accepted`/`llm_end_hooks_started`; `RunState.get_interruptions()`, `approve()`/`reject()`-style methods on `RunContextWrapper`, and `RunState.add_input()` allow human to mutate trajectory before next model call. | `sources/openai-agents-sdk/src/agents/run_internal/run_steps.py:171-182` , `sources/openai-agents-sdk/src/agents/run_state.py:1015-1047` , `sources/openai-agents-sdk/src/agents/run_state.py:971-1010` , `sources/openai-agents-sdk/src/agents/run_context.py:1043-1071` |
| Status transitions | Loop in `AgentRunner._run_impl` handles `NextStepRunAgain`→continue, `NextStepHandoff`→swap agent, `NextStepInterruption`→build `RunResult.interruptions` via `build_interruption_result` → `RunState._current_step`, `NextStepFinalOutput`→finalize via `execute_final_output` / `finalize_conversation_tracking`. | `sources/openai-agents-sdk/src/agents/run.py:966-1150` , `sources/openai-agents-sdk/src/agents/run_internal/agent_runner_helpers.py:369-376` , `sources/openai-agents-sdk/src/agents/run_internal/blocked_output.py:333-371` |
| Completion validator | `AgentOutputSchemaBase.validate_json` called on `potential_final_output_text` in `turn_resolution._resolve_turn`; on failure `_resolve_invalid_final_output` invokes `RunErrorHandlers["invalid_final_output"]` then synthesizes `MessageOutputItem` or error; `validate_handler_final_output` + `format_final_output_text` + `run_final_output_hooks` enforce schema before `NextStepFinalOutput`. | `sources/openai-agents-sdk/src/agents/run_internal/turn_resolution.py:1000-1065` , `sources/openai-agents-sdk/src/agents/run_internal/turn_resolution.py:426-473` , `sources/openai-agents-sdk/src/agents/run_internal/turn_resolution.py:335-422` |
| Max-turns / abandon | `current_turn` incremented each loop; `if max_turns is not None and current_turn > max_turns` raises `MaxTurnsExceeded` with optional `error_handlers["max_turns"]` that can synthesize final output via `finalize_max_turns_handler_output`. Resume preserves `run_state._max_turns`. `RunResultStreaming.cancel(mode)` with `immediate` vs `after_turn`. | `sources/openai-agents-sdk/src/agents/run.py:1477-1502` , `sources/openai-agents-sdk/src/agents/run.py:1508-1582` , `sources/openai-agents-sdk/src/agents/run_internal/run_loop.py:1559-1671` , `sources/openai-agents-sdk/src/agents/result.py:1048-1057` , `sources/openai-agents-sdk/src/agents/result.py:819-866` |
| Persistence / revision history | `RunState.to_json` merges `_generated_items` with `_last_processed_response.new_items` (deduplicating by id/call_id), persists `model_responses`, `generated_items`, `session_items`, `context`, `tool_use_tracker`, `current_turn`, `current_step`, `last_processed_response`, pending Session write; `from_json` restores via schema-gated validation. 17 schema versions summarized in `SCHEMA_VERSION_SUMMARIES`; `current_response_generated_item_ownership` tracks interrupted current-response slice. No plan-diff field. | `sources/openai-agents-sdk/src/agents/run_state.py:191-232` , `sources/openai-agents-sdk/src/agents/run_state.py:1774-1940` , `sources/openai-agents-sdk/src/agents/run_state.py:1707-1772` , `sources/openai-agents-sdk/src/agents/run_state.py:1491-1528` |
| Drift / observability | `AgentToolUseTracker` records `tools_used` per agent via `record_processed_response`; `serialize_tool_use_tracker` snapshot stored in state (`_tool_use_tracker_snapshot`). Tracing via `TraceState`, `agent_span`, `turn_span`, `task_span`. No comparison of execution against an upfront plan; no drift threshold or alert. | `sources/openai-agents-sdk/src/agents/run_internal/tool_use_tracker.py:76-100` , `sources/openai-agents-sdk/src/agents/run_state.py:858-862` , `sources/openai-agents-sdk/src/agents/run.py:738-746` |
| Add-input as informal plan edit | `RunState.add_input` stages `TResponseInputItem` for admission before next resumed model call, gated: rejects if terminal, if `response_accepted`, if tool_use_behavior would stop, or if max_turns exhausted. Persists in `pending_input`. | `sources/openai-agents-sdk/src/agents/run_state.py:971-1013` |

## Answers to Dimension Questions

### 1. Can plans change?

**Yes, but implicitly and continuously.** There is no `Plan` object with an `update()` method. The "plan" is the LLM's next token. Each `run_single_turn` produces a `ProcessedResponse` that is triaged into `SingleStepResult.next_step` (`sources/openai-agents-sdk/src/agents/run_internal/run_steps.py:185-199`). Change mechanisms:

- Model elects new tool calls/handoffs/final output each turn (`sources/openai-agents-sdk/src/agents/run_internal/turn_resolution.py:937-950`).
- `HandoffInputFilter` / `nest_handoff_history` rewrites history and swaps the executing agent (`sources/openai-agents-sdk/src/agents/handoffs/history.py:97-135`).
- `RunState.add_input()` (`sources/openai-agents-sdk/src/agents/run_state.py:971-1009`) lets a human inject input before the next model call, altering trajectory.
- `ToolExecutionPlan` is rebuilt fresh each turn (`sources/openai-agents-sdk/src/agents/run_internal/tool_planning.py:619-680`), reflecting new tool invocations; prior plan is discarded, not revised.

No explicit `plan_version++` or `replan(reason)` call.

### 2. Are changes justified?

**Partially, via unstructured narrative and traces, not explicit justification metadata.** When a handoff occurs a `HandoffOutputItem` and nested transcript are recorded (`sources/openai-agents-sdk/src/agents/handoffs/history.py:83-95`); tool results and model reasoning items appear in `generated_items` / `model_responses` (`sources/openai-agents-sdk/src/agents/run.py:1085-1206`). Tracing spans (`turn_span`, `agent_span`, `sources/openai-agents-sdk/src/agents/run.py:1628-1650`) and `output_guardrail_results` capture why a guardrail blocked. However there is no `plan_change_reason` field, no `previous_plan → new_plan` diff, and `ToolExecutionPlan` carries no justification string. `_resolve_invalid_final_output` routes to error handlers but records the handler's synthesized output, not a rationale (`sources/openai-agents-sdk/src/agents/run_internal/turn_resolution.py:426-473`). So "why did the plan change?" can only be inferred from conversation history, not queried explicitly.

### 3. Is old plan history preserved?

**Execution history is preserved append-only; plan revision history is not.** `RunState.to_json` persists `model_responses`, `generated_items` (`_merge_generated_items_with_processed` deduping by `id`/`call_id`, `sources/openai-agents-sdk/src/agents/run_state.py:1707-1772`), `session_items`, `original_input`, `current_turn`, and `last_processed_response` (`sources/openai-agents-sdk/src/agents/run_state.py:1837-1906`). `SCHEMA_VERSION_SUMMARIES` documents durable migrations (`sources/openai-agents-sdk/src/agents/run_state.py:196-229`). For handoffs, `nested_history_owned_session_item_refs` and `current_response_generated_item_ownership` preserve which items were moved into nested history (`sources/openai-agents-sdk/src/agents/run_state.py:1452-1528`). This gives a full *execution* audit log, but there is no `plan_revisions[]` with diffs, timestamps, or actor that changed the plan. Previous "intended steps" not yet executed were never reified, so they cannot be retained.

### 4. Can the agent abandon a plan?

**Yes, via several abandonment paths, none labeled "abandon plan":**

- **Handoff:** `NextStepHandoff` abandons the current agent's trajectory and delegates to another agent, optionally filtering the input history (`sources/openai-agents-sdk/src/agents/run_internal/run_steps.py:156-157` , `sources/openai-agents-sdk/src/agents/run_internal/turn_resolution.py:540-745`).
- **Final output shortcut:** `ToolExecutionPlan` → `check_for_final_output_from_tools` with `stop_on_first_tool` or custom `ToolsToFinalOutputFunction` returns `NextStepFinalOutput` immediately, skipping remaining steps (`sources/openai-agents-sdk/src/agents/run_internal/turn_resolution.py:753-773`).
- **Max-turns / invalid output handlers:** `MaxTurnsExceeded` and `invalid_final_output` can be handled by `RunErrorHandlers` that synthesize a final output instead of continuing the loop (`sources/openai-agents-sdk/src/agents/run.py:1496-1520`, `sources/openai-agents-sdk/src/agents/run_internal/turn_resolution.py:426-473`).
- **Interrupt + never resume:** A `NextStepInterruption` result can be converted to `RunState` (`sources/openai-agents-sdk/src/agents/result.py:542-589`); the caller may simply drop it — effectively abandoning.
- **Streaming cancel:** `RunResultStreaming.cancel(mode="immediate"|"after_turn")` (`sources/openai-agents-sdk/src/agents/result.py:819-866`) abandons remaining turns.

None of these emit a `plan_abandoned` event.

### 5. Can plan drift be detected?

**No.** Drift presupposes a declared plan to compare against; the SDK never asks the model to emit one. `AgentToolUseTracker` (`sources/openai-agents-sdk/src/agents/run_internal/tool_use_tracker.py:76-100`) records which tools were actually called, and `TraceState` preserves spans, but there is no `expected_plan` vs `actual_trajectory` comparator, no drift score, and no threshold alert. Input/output guardrails validate individual items, not cumulative deviation from an initial plan. Session persistence can detect *history* drift (`_pending_session_write` reconciliation in `sources/openai-agents-sdk/src/agents/run_state.py:872-878` warns about ambiguous history requiring application repair), but not *plan* drift.

## Architectural Decisions

| Decision | Location | Implication for Plan Lifecycle |
|----------|----------|-------------------------------|
| **Delegate planning to LLM** — no `Plan` type, `Runner` loops over model outputs. | `sources/openai-agents-sdk/src/agents/run.py:275-283, 967-968` | Simplest integration; plan is prompt-dependent and not inspectable/versioned. |
| **Turn-level `SingleStepResult.next_step` as state machine** (`RunAgain`/`Handoff`/`FinalOutput`/`Interruption`). | `sources/openai-agents-sdk/src/agents/run_internal/run_steps.py:171-199` | Gives clear status transitions without reifying a multi-step plan. |
| **Ephemeral `ToolExecutionPlan` per turn** rather than persistent task plan. | `sources/openai-agents-sdk/src/agents/run_internal/tool_planning.py:558-680` | Allows clean retry/resume tool dispatch but discards intent across turns. |
| **Append-only `RunState` with id/call_id dedup** instead of revisioned plan snapshots. | `sources/openai-agents-sdk/src/agents/run_state.py:1707-1772, 1837-1906` , `sources/openai-agents-sdk/src/agents/run_state.py:191-232` | Durable HITL resume and replay; audit trail is execution history, not plan versions. |
| **`HandoffInputFilter` + `nest_handoff_history` rewrites input history on agent switch.** | `sources/openai-agents-sdk/src/agents/handoffs/history.py:97-156` | Handoff is the de facto replanning primitive; provenance via `NestedHistoryOwnedItemRef`. |
| **Guardrails + `RunErrorHandlers` as completion validators** (`invalid_final_output`, `max_turns`). | `sources/openai-agents-sdk/src/agents/run_internal/turn_resolution.py:426-473` , `sources/openai-agents-sdk/src/agents/run.py:1496-1520` | Completion is policy-validated; handlers can synthesize substitute outputs to force termination. |
| **`RunState.add_input` as the only user-facing trajectory mutation API before next model call.** | `sources/openai-agents-sdk/src/agents/run_state.py:971-1009` | Enables human steering without an explicit plan-edit vocabulary; gated by terminal/accepted-response/stop-behavior checks. |

## Notable Patterns

- **State-machine over planner:** `NextStep*` union + `SingleStepResult` is a compact workflow engine; planning emerges rather than being declared (`sources/openai-agents-sdk/src/agents/run_internal/run_steps.py:171-232`).
- **Durable interruption as checkpoint:** `NextStepInterruption` + `RunState._current_step` + `ToolApprovalItem` with copysafe serialization (`_copy_tool_approval_raw_item` `sources/openai-agents-sdk/src/agents/run_state.py:696-758`) gives HITL pause/resume with approval identity matching (`_approval_items_match` `sources/openai-agents-sdk/src/agents/run_state.py:1049-1085`).
- **Nested history ownership:** `NestedHistoryOwnedItemRef` + `rebase_nested_history_owned_item_refs` tracks which session items were moved verbatim into nested handoff history, preventing double replay (`sources/openai-agents-sdk/src/agents/result.py:82-105`, `sources/openai-agents-sdk/src/agents/run_state.py:1452-1530`).
- **Canonical pending session write:** `_PendingSessionWrite` / `_pending_session_write` must settle before next model call, reconciled via `resume_pending_session_write` (`sources/openai-agents-sdk/src/agents/run_state.py:872-878`).
- **Tool-identity aware planning:** `ToolExecutionPlan` building consults `FunctionToolLookupKey`, `tool_invocation_identity`, and `ToolOrigin` for routing and de-duplication (`sources/openai-agents-sdk/src/agents/run_internal/tool_planning.py:315-339`).
- **Schema-versioned durability:** `CURRENT_SCHEMA_VERSION="1.17"` with per-version summaries and forward-compatibility fail-fast (`sources/openai-agents-sdk/src/agents/run_state.py:191-236`).

## Tradeoffs

| Tradeoff | Pro | Con |
|----------|-----|-----|
| LLM-native planning (no Plan DSL) | Zero planning boilerplate; any decomposition prompt works. | No plan validation, revision tracking, or drift detection; auditing requires transcript parsing. |
| Ephemeral `ToolExecutionPlan` | Simple, race-free tool dispatch; easy resume with `_build_plan_for_resume_turn`. | No cross-turn optimization, no persistent tool schedule to diff. |
| Handoff-based replanning | Clean agent specialization; `HandoffInputFilter` isolates concerns. | Replanning is scattered across handoff impls and LLM outputs, not central. |
| Append-only history vs plan versions | Strong replay guarantees; id/call_id dedup prevents duplication after interruption. | Cannot answer "what was step 3 of the original plan?" — history is execution, not intent. |
| `add_input` gating (`stop_on_first_tool`, `response_accepted`) | Prevents adding input when semantics would be ambiguous. | Friction for human steering of tool-result-driven final-output flows. |
| `RunErrorHandlers` synthesize output to handle completion failures | Graceful degradation for `max_turns`/`invalid_final_output` without crashing. | May mask underlying model or schema drift behind a synthesized "completed" result. |

## Failure Modes / Edge Cases

- **Lost interrupted turn if `RunState` dropped:** No plan persists outside `RunState`; abandoning the serialized state loses the only record of pending tool approvals (`sources/openai-agents-sdk/src/agents/run_state.py:762-783`).
- **Duplicate call_id across approvals:** `_approval_items_match` / `_find_current_approval_item` raise `UserError` when multiple pending approvals share the same invocation identity (`sources/openai-agents-sdk/src/agents/run_state.py:1104-1123`), blocking `approve()` — callers must ensure unique `call_id`s.
- **Adding input to `stop_on_first_tool` interruption:** `RunState.add_input` raises `UserError` when tool result may end the run (`sources/openai-agents-sdk/src/agents/run_state.py:991-1006`); steering requires rejecting approvals first.
- **`response_accepted` blocks mutation:** After server accepts a response but local processing is incomplete, adding input is forbidden (`sources/openai-agents-sdk/src/agents/run_state.py:984-988`).
- **Max-turns silently handled:** If `error_handlers["max_turns"]` synthesizes output, caller sees a `RunResult` with `final_output` but `_current_turn == max_turns` (`sources/openai-agents-sdk/src/agents/run.py:1564-1573`); original plan truncation is not flagged as `plan_abandoned`.
- **Invalid final output loop:** Invalid JSON final output that repeatedly fails handler validation can keep returning `NextStepRunAgain` with an error message item (`sources/openai-agents-sdk/src/agents/run_internal/turn_resolution.py:1007-1064`), never surfacing a distinct `plan_failed`.
- **History-ownership mismatch on resume:** `current_response_generated_item_ownership` returns `None` if `generated_items` and `last_processed_response.new_items` cannot be uniquely aligned (`sources/openai-agents-sdk/src/agents/run_state.py:1506-1528`), causing `to_json` to omit the ownership marker and risking replay anomalies.
- **Schema drift:** Forward compatibility is fail-fast — older SDK rejects newer `RunState` versions (`sources/openai-agents-sdk/src/agents/run_state.py:2295-2302`); rolling deployments that persist states must coordinate upgrades.
- **Context loss on serialize:** Non-mapping contexts without `context_serializer` are warned and stored as `{}` with `context_meta.requires_deserializer=true` (`sources/openai-agents-sdk/src/agents/run_state.py:1638-1653`); resuming without deserializer silently runs with degraded context, changing plan-relevant state.

## Future Considerations

- Introduce an opt-in `Plan` artifact (e.g., `Agent.plan_schema` + `plan: list[PlanStep] {id, goal, status, depends_on}`) emitted via structured output before tool loop, with `RunState.plan_revisions: list[PlanRevision {version, diff, reason, actor, ts}]` persisted alongside `generated_items`.
- Add `RunState.replan(reason: str, new_plan: Plan)` that validates against `plan_schema`, bumps `plan_version`, and emits a `plan_revised` span/event for observability ("can the system explain why the plan changed?").
- Add `plan_drift_metrics` (steps skipped/added, tool deviation from planned tool set) computed from `AgentToolUseTracker` versus declared plan, with configurable thresholds and guardrail hooks.
- Persist `ToolExecutionPlan` lineage in `RunState` as `tool_plan_history` so tool-level revision can be audited even without a task-level plan.
- Expose `RunState.abandon(reason)` that transitions `_current_step` to a terminal abandoned sentinel and records `abandon_reason`, distinct from `MaxTurnsExceeded` synthesis, to make abandonment queryable.
- Centralize handoff replanning rules (when to rewrite history vs preserve) behind a `ReplanningPolicy` to make plan-change justification uniform.

## Questions / Gaps

- No evidence of a user-authored plan prompt lifecycle — do typical consumers provide a "plan" in `Agent.instructions` versus relying on model autonomy? Search of `src/agents/agent.py:183-296` shows no `plan` field on `Agent`.
- `ToolExecutionPlan` is internal and unlabeled for plan lifecycle purposes; no tests were inspected — does test suite treat it as "plan lifecycle" or purely as concurrency dispatch? No evidence found in sampled files.
- Drift detection for training-data improvements (evaluators) — does the tracing/export pipeline in `src/agents/tracing/` contain plan-level evaluators? No evidence found in scope-checked files.
- How does `advanced_sqlite_session` branch/turn anchoring interact with plan revision branching? `sources/openai-agents-sdk/src/agents/extensions/memory/advanced_sqlite_session.py:637-686` tracks `current_turn`/`branch_id` but not plan versions — is there hidden plan-like branching? Needs deeper inspection.

---

Generated by `06.03-plan-lifecycle-and-revision` against `openai-agents-sdk`.

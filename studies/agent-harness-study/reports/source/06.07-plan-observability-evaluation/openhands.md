# Source Analysis: openhands

## 06.07 Plan Observability and Evaluation

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | Python (Pydantic, Litellm, Laminar/OTel) — SDK under `_sdk_inspect/sdk`, server under `openhands/app_server` |
| Analyzed | 2026-08-27 |

## Summary

OpenHands implements two disjoint planning surfaces — a runtime `TaskTracker` task list and a dedicated planning-agent `PLAN.md` file — both observable via the file-backed `EventLog` and filesystem, but with no explicit link between plan items and subsequent tool actions, no plan-quality metrics, no planning eval harness, and no planning regression tests. Observability is generic (event persistence, Laminar/OTel tracing) rather than planning-aware, so `plan vs execution` debugging is manual and forensic rather than first-class.

## Rating

**4 / 10 — Present but inconsistent, weakly documented, fragile**

Rationale: Plans exist and persist (TaskTracker `view`/`plan` commands + `TASKS.json`, planning agent `PLAN.md`), and all actions/observations are durably logged in `EventLog` with OTEL traces. However (a) actions carry no `plan_item_id`, (b) there is no plan-quality metric or scorecard, (c) no eval dataset or benchmark exercises plan generation, and (d) no regression tests assert planning behavior. Debugging a failed run by comparing plan vs execution requires manual correlation of timestamps/titles — the question that defines a 7+ score cannot be answered satisfactorily.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Plan traces — TaskTracker action/observation schema | `TaskTrackerAction` with `command: Literal["view","plan"]` and `task_list: list[TaskItem]`; `TaskTrackerObservation` echoes `task_list` with status counts and Rich `visualize` | `_sdk_inspect/tools/task_tracker/definition.py:42-84` |
| Plan traces — TaskItem model | `TaskItem(title, notes, status: todo|in_progress|done)` — no stable ID, ordering is positional index | `_sdk_inspect/tools/task_tracker/definition.py:32-39` |
| Plan traces — persistence to file | `TaskTrackerExecutor._save_tasks` writes `TASKS.json` under `save_dir` (conversation `persistence_dir`); `_load_tasks` restores on init | `_sdk_inspect/tools/task_tracker/definition.py:252-266` |
| Plan traces — creation wiring | `TaskTrackerTool.create` instantiates `TaskTrackerExecutor(save_dir=conv_state.persistence_dir)` and registers via `register_tool` | `_sdk_inspect/tools/task_tracker/definition.py:406-435` |
| Plan traces — planning-agent file | `PlanningFileEditorTool.create` resolves `plan_path` to `{workspace}/.agents_tmp/PLAN.md` (legacy `{workspace}/PLAN.md`), auto-initializes with `get_plan_headers()` if missing | `_sdk_inspect/tools/planning_file_editor/definition.py:67-125` |
| Plan traces — PLAN.md structure | `PLAN_STRUCTURE` defines 5 mandatory sections: OBJECTIVE, CONTEXT SUMMARY, APPROACH OVERVIEW, IMPLEMENTATION STEPS, TESTING AND VALIDATION | `_sdk_inspect/tools/preset/planning.py:14-53` |
| Plan traces — plan headers formatting | `get_plan_headers()` emits `# 1. OBJECTIVE` etc.; `format_plan_structure()` injects into system prompt | `_sdk_inspect/tools/preset/planning.py:56-88` |
| Plan traces — planning agent prompt | `system_prompt_planning.j2` enforces four-phase workflow (Understanding → Planning → Synthesis → Refinement) and `Write the initial plan to PLAN.md` | `_sdk_inspect/sdk/agent/prompts/system_prompt_planning.j2:26-84` |
| Plan traces — long-horizon prompt instructs task_tracker usage | `system_prompt_long_horizon.j2` mandates regular `task_tracker` use for multi-phase work | `_sdk_inspect/sdk/agent/prompts/system_prompt_long_horizon.j2:4-7` |
| Plan item IDs | **No evidence found** — `TaskItem` has no `id` field; `TaskTrackerObservation` lists tasks by enumerative index (`for i, task in enumerate(self.task_list, 1)`); `ActionEvent` has no `plan_item_id` | `_sdk_inspect/tools/task_tracker/definition.py:122-130`, `_sdk_inspect/sdk/agent/agent.py:879-903` |
| Execution links — generic event log | `EventLog.append` persists every `Event` (including `TaskTrackerAction/Observation`, `PlanningFileEditorAction/Observation`) with UUID, timestamp, and file-backed locking; `LocalConversation` default callback `_default_callback` appends all events and tracks `last_user_message_id` | `_sdk_inspect/sdk/conversation/event_store.py:119-152`, `_sdk_inspect/sdk/conversation/impl/local_conversation.py:195-206` |
| Execution links — action/observation pairing | `ConversationState.get_unmatched_actions` matches `ObservationEvent.action_id` / `AgentErrorEvent.tool_call_id` to `ActionEvent.id`; agent loop `_ActionBatch.prepare/emit` preserves ordering but does not carry plan references | `_sdk_inspect/sdk/conversation/state.py:473-513`, `_sdk_inspect/sdk/agent/agent.py:156-206` |
| Execution links — Task delegation IDs | `TaskManager._generate_ids` creates `task_id=task_XXXXXXXX` for sub-agent delegation, unrelated to TaskTracker plan items | `_sdk_inspect/tools/task/manager.py:133-135` |
| Observability — Laminar/OTel span per conversation | `BaseConversation._start_observability_span` / `RootSpan` via `Laminar.start_span` + `Laminar.use_span`; `maybe_init_laminar` gated on `LMNR_PROJECT_API_KEY`/`OTEL_*` env vars; `@observe` on `conversation.send_message`, `conversation.run`, `agent.step` | `_sdk_inspect/sdk/observability/laminar.py:57-90`, `_sdk_inspect/sdk/observability/laminar.py:115-196`, `_sdk_inspect/sdk/conversation/base.py:130-151`, `_sdk_inspect/sdk/conversation/impl/local_conversation.py:678,744`, `_sdk_inspect/sdk/agent/agent.py:475` |
| Observability — event service for server persistence | `EventServiceBase.search_events` / `iter_events_for_export` supports filtering by `EventKind` and timestamp + pagination; `save_event` writes `{event_id.hex}.json` under `V1_CONVERSATIONS_DIR/{hex}` | `openhands/app_server/event/event_service_base.py:94-157` |
| Execution metrics (not plan-specific) | `ConversationStats` merges per-LLM `Metrics` (tokens, cost, latency); `Metrics.get_snapshot()` / `get_combined_metrics()` exposed to UI | `_sdk_inspect/sdk/conversation/conversation_stats.py:13-63`, `_sdk_inspect/sdk/llm/utils/metrics.py:96-243` |
| Planning metrics | **No evidence found** — grep for `plan.*metric`, `planning.*quality`, `plan.*evaluat` returned 0 hits | `_sdk_inspect` grep 2026-08-27 |
| Eval harnesses — critic (not planning-specific) | `CriticBase` / `APIBasedCritic` / `AgentFinishedCritic` / `EmptyPatchCritic` evaluate final agent outcome (patch non-empty, finished signal, LLM-judged score) — never inspects `TaskTracker` or `PLAN.md` | `_sdk_inspect/sdk/critic/base.py:58-114`, `_sdk_inspect/sdk/critic/impl/agent_finished.py:1-28`, `_sdk_inspect/sdk/critic/impl/api/critic.py:31-160` |
| Eval harnesses — planning | **No evidence found** — no `eval`, benchmark, or dataset directory under `_sdk_inspect`; `tools/preset/planning.py` provides `get_planning_agent` but no harness invokes it for scoring | `_sdk_inspect/tools/preset/planning.py:154-179` |
| Regression tests — planning | **No evidence found** — source checkout contains no `tests/` mirror for `_sdk_inspect` planning; grep for `plan.*test`, `planning.*regress` empty | `_sdk_inspect` grep 2026-08-27 |
| System prompt — task_tracker examples | `fn_call_examples.py` registers `TASK_TRACKER_TOOL_NAME` and injects `tool_tracker` `view`+`plan` examples into LLM few-shot when tool is available | `_sdk_inspect/sdk/llm/mixins/fn_call_examples.py:21,297-394` |

## Answers to Dimension Questions

### 1. Are plans observable?

**Partially — yes for existence, no for quality.**

- **Observable:** `TaskTracker` plans are observable through three channels: (a) `TaskTrackerObservation.visualize` rich text (`_sdk_inspect/tools/task_tracker/definition.py:85-145`), (b) persisted `TASKS.json` under `save_dir` / `persistence_dir` (`_sdk_inspect/tools/task_tracker/definition.py:252-266`), and (c) event stream (`ActionEvent`→`ObservationEvent` in `EventLog` at `_sdk_inspect/sdk/conversation/event_store.py:119-151` + server-side `EventServiceBase.search_events` at `openhands/app_server/event/event_service_base.py:94-143`). Planning-agent plans are observable as a markdown file at `.agents_tmp/PLAN.md` (`_sdk_inspect/tools/planning_file_editor/definition.py:29-31,116-125`) edited only via `PlanningFileEditorTool` (`_sdk_inspect/tools/planning_file_editor/impl.py:18-66`) and also logged as file-editor events.
- **Not observable:** No telemetry summarizes plan health (e.g., count of `todo` vs `done`, staleness, drift). `ConversationStats` (`_sdk_inspect/sdk/conversation/conversation_stats.py:13-63`) tracks only LLM token/cost metrics. Laminar spans (`_sdk_inspect/sdk/observability/laminar.py:231-265`) are per-conversation, not per-plan-item.
- **Gap:** A user inspecting `TASKS.json` or `PLAN.md` can read the plan, but there is no dashboard, no plan-version diff, and no event linking plan edits to later execution.

### 2. Can each action be linked to a plan item?

**No.**

- `TaskItem` (`_sdk_inspect/tools/task_tracker/definition.py:32-39`) has no stable identifier — only `title`/`notes`/`status`. The list is positional; reordering or re-`plan`ing shifts indices without migration.
- `ActionEvent` (`_sdk_inspect/sdk/agent/agent.py:879-903`) stores `thought`, `tool_call`, `tool_name`, `llm_response_id`, `security_risk`, `summary`, `critic_result` but no `plan_item_id`, `task_index`, or `plan_version` field. `ObservationEvent` (`_sdk_inspect/sdk/agent/agent.py:955-961`) stores `action_id`/`tool_call_id` back to `ActionEvent`, not to a plan item.
- `PlanningFileEditorAction` (`_sdk_inspect/tools/planning_file_editor/definition.py:33-38`) inherits `FileEditorAction` (path + edit op) with no structured plan-item decomposition — `PLAN.md` IMPLEMENTATION STEPS are free-form markdown, not addressable entities.
- **Forensic workaround only:** An analyst can temporally interleave `TaskTrackerObservation` events (when `command=="plan"`) with subsequent `ActionEvent`s in `EventLog` order, but must infer linkage by string matching `title` vs `summary`. No first-class join exists.

### 3. Are plans evaluated?

**Not as plans. Only final outcomes are evaluated.**

- `CriticBase` evaluation (`_sdk_inspect/sdk/critic/base.py:58-88`) and iterative refinement (`agent/agent.py:893-900`, `sdk/agent/critic_mixin.py:34-73`) score the final trajectory (e.g., `AgentFinishedCritic` checks patch non-empty and finish signal at `_sdk_inspect/sdk/critic/impl/agent_finished.py:28`; `APIBasedCritic` calls a remote scoring service at `_sdk_inspect/sdk/critic/impl/api/critic.py:31-160`). No critic inspects `task_list` completeness, `PLAN.md` conformance, or step feasibility.
- `VerificationSettings` (`_sdk_inspect/sdk/settings/model.py:143-228,810-849`) configures `critic_enabled`, `critic_mode`, `critic_threshold`, `enable_iterative_refinement` but has no `plan_evaluation` toggle.
- No eval harness for planning: `tools/preset/planning.py:154-179` constructs a planning agent, but no runner scores multiple generated `PLAN.md` variants against a gold set, nor measures TaskTracker plan quality (coverage, ordering, decomposability).

### 4. Can poor planning be diagnosed?

**Barely — manual, inferential, not instrumented.**

- **What helps:** The full event history is reconstructible (`EventLog.__iter__` at `_sdk_inspect/sdk/conversation/event_store.py:107-117` and server export `iter_events_for_export` at `openhands/app_server/event/event_service_base.py:145-157`), so an engineer can diff the final TaskTracker state (e.g., tasks stuck in `todo`/`in_progress`) vs executed `ActionEvent.summary` values. `StuckDetector` (`_sdk_inspect/sdk/conversation/impl/local_conversation.py:311-319`) can flag loops, indirectly indicating plan-following failure. `AgentErrorEvent` (`_sdk_inspect/sdk/agent/agent.py:746-765`) surfaces tool validation/runtime failures that may stem from bad plan sequencing.
- **What hinders:** No plan diff view, no plan-vs-action coverage metric, no "unplanned action" flag, no "planned but not executed" detector. `TASKS.json` has no version history (overwritten on each `plan` command at `_sdk_inspect/tools/task_tracker/definition.py:174-179`). `PLAN.md` edits are not versioned either (FileEditorExecutor overwrites file; only event log preserves prior `file_text`). Laminar traces do not tag spans with plan item IDs, so trace analysis cannot surface which plan step inflated latency/cost.

### 5. Does planning improve success rate?

**No measured evidence in this source.**

- `TaskTracker` and planning-agent docs claim organizational benefit (`TASK_TRACKER_DESCRIPTION` at `_sdk_inspect/tools/task_tracker/definition.py:269-400`, `system_prompt_long_horizon.j2:4-30`), but the codebase contains no A/B experiment, no success-rate telemetry segmented by `task_tracker` usage, and no citation of controlled evaluation.
- `ConversationStats` + `LLMRegistry` metrics could support such a study, but they are not aggregated or sliced by planning usage. The critic's iterative-refinement loop (`_sdk_inspect/sdk/critic/base.py:21-39`) measures post-hoc correction, not planning uplift. No documentation ties `PLAN.md` adoption to outcome improvement.

## Architectural Decisions

| Decision | Evidence | Consequence for Observability/Evaluation |
|----------|----------|------------------------------------------|
| **Dual planning surfaces: lightweight `TaskTracker` vs heavyweight planning agent** | `TaskTrackerTool` at `_sdk_inspect/tools/task_tracker/definition.py:403-435` vs `PlanningFileEditorTool` at `_sdk_inspect/tools/planning_file_editor/definition.py:62-155` + `get_planning_agent` at `_sdk_inspect/tools/preset/planning.py:154-179` | Users choose inconsistent formalism; neither is canonical, so evaluation must cover both or misses half the plans. Telemetry is split across `TASKS.json` and `.agents_tmp/PLAN.md`. |
| **TaskTracker stored as mutable array without stable IDs** | `TaskItem` without `id` at `_sdk_inspect/tools/task_tracker/definition.py:32-39`; executor holds `self._task_list: list[TaskItem]` at `...:162`; serialization is positional JSON at `...:263` | Prevents durable linking from actions → plan items; re-planning invalidates any ad-hoc external join. History is lossy (last write wins). |
| **Planning agent restricted to PLAN.md but otherwise free-form markdown** | `PlanningFileEditorExecutor` with `allowed_edits_files=[plan_path]` at `_sdk_inspect/tools/planning_file_editor/impl.py:28-31`; `PLAN_STRUCTURE` at `_sdk_inspect/tools/preset/planning.py:14-53` defines 5 prose sections | Machine-evaluable structure is absent — IMPLEMENTATION STEPS embed `goal/method/reference` as bullet guidance, not schema, so automated validation/coverage scoring is infeasible without NLP parsing. |
| **Generic EventLog as sole ground truth** | `EventLog.append` with file locking at `_sdk_inspect/sdk/conversation/event_store.py:119-152`; `EventServiceBase._search_paths` + `_load_event` at `openhands/app_server/event/event_service_base.py:45-64` | Provides durable audit trail but no plan-aware indexing; querying "show executions of plan item 3" requires custom scanning. Benefits observability infrastructure reuse but misses domain-specific observability. |
| **Laminar/OTEL as optional, conversation-scoped tracing** | `maybe_init_laminar` gated on env vars at `_sdk_inspect/sdk/observability/laminar.py:57-90`; `RootSpan` per conversation at `...:231-265`; `@observe` on `agent.step` at `_sdk_inspect/sdk/agent/agent.py:475` | Enables distributed tracing with minimal code change (pass-through when disabled), but spans are not annotated with plan metadata, so plan-evaluation signals cannot be derived from traces. |
| **Critic evaluated on final outcome, not plan fidelity** | `AgentFinishedCritic` / `APIBasedCritic` at `_sdk_inspect/sdk/critic/impl/*.py`; `CriticMixin._evaluate_with_critic` at `_sdk_inspect/sdk/agent/critic_mixin.py:49-73` invoked only for `FinishAction` or `all_actions` per `_should_evaluate_with_critic` | Decouples outcome quality from planning quality — good plan + bad execution and bad plan + lucky execution are indistinguishable to the evaluator. |

## Notable Patterns

- **Event-sourced, file-backed audit log:** All plan mutations and tool executions are `Event` subclasses with UUID + timestamp, persisted via `EventLog` (file-per-event under `EVENTS_DIR`) and server-side `EventServiceBase` pagination — a strong foundation for retroactive plan↔execution analysis, even though plan links are not currently stored.
- **Prompt-engineered planning discipline:** `system_prompt_planning.j2` (four-phase workflow) and `system_prompt_long_horizon.j2` (task_tracker mandate) encode planning methodology as LLM instructions rather than code — cheap to evolve, but fragile and unevaluated.
- **Lazy, cache-friendly observability:** `observe` decorator at `_sdk_inspect/sdk/observability/laminar.py:115-196` lazy-imports `lmnr` only when OTEL env vars are present, and `RootSpan` uses `Laminar.start_span` + `Laminar.use_span` to survive cross-task context propagation — avoids overhead when tracing is off.
- **Plan persistence tied to conversation persistence:** `TaskTrackerExecutor(save_dir=conv_state.persistence_dir)` at `_sdk_inspect/tools/task_tracker/definition.py:415` and `PLAN.md` under `workspace_root/.agents_tmp` at `_sdk_inspect/tools/planning_file_editor/definition.py:96-113` both live alongside `BASE_STATE` / `EVENTS_DIR` — plan lifetime is conversation lifetime, enabling replay/fork (`LocalConversation.fork` at `_sdk_inspect/sdk/conversation/impl/local_conversation.py:314-415` copies events but note it does not explicitly copy `TASKS.json`).

## Tradeoffs

| Tradeoff | Pro | Con | Evidence |
|----------|-----|-----|----------|
| Generic event log vs plan-aware store | Zero extra infra; every plan mutation is already an event; fork/replay come for free | No indexed plan-item lookup, no plan version history, no coverage metric; every analysis is O(n) scan with string matching | `_sdk_inspect/sdk/conversation/event_store.py:25-52`, `openhands/app_server/event/event_service_base.py:94-143` |
| Schema-less `TaskItem` (title/notes/status) | Fast for LLM to emit; no rigid planning ontology to maintain | Cannot validate completeness, detect duplicates, enforce dependencies, or link actions; re-planning loses identity | `_sdk_inspect/tools/task_tracker/definition.py:32-39,174-179` |
| Free-form `PLAN.md` vs structured plan DSL | Human-readable, prompt-easy, leverages existing file-editor tooling | Unparseable for automated eval; `IMPLEMENTATION STEPS` bullet convention not enforced in code | `_sdk_inspect/tools/preset/planning.py:37-45`, `_sdk_inspect/tools/planning_file_editor/impl.py:46-55` |
| Optional OTEL (opt-in via env) | No overhead/cost when disabled; `should_enable_observability()` pass-through preserves perf | Plan observability is off by default; teams needing plan diagnostics must configure Laminar separately | `_sdk_inspect/sdk/observability/laminar.py:199-216`, `...:57-87` |
| Outcome critic vs plan critic | Outcome is ultimately what matters; critic service is pluggable | Cannot attribute failures to planning vs execution; no signal to improve planning specifically | `_sdk_inspect/sdk/critic/base.py:58-88`, `_sdk_inspect/sdk/settings/model.py:810-849` |

## Failure Modes / Edge Cases

- **Plan Overwrite Without History:** `TaskTrackerExecutor.__call__` at `_sdk_inspect/tools/task_tracker/definition.py:174-179` does `self._task_list = action.task_list` and overwrites `TASKS.json` atomically — prior plan version is only recoverable by scanning `EventLog` for prior `TaskTrackerObservation` events; concurrent `plan` calls race via file lock but last writer wins silently.
- **Index Fragility on Re-Plan:** Because `TaskItem` has no ID, inserting a task at index 1 shifts all downstream indices; any external system that referenced "task 2" by position now points to the wrong item.
- **PLAN.md Legacy Path Ambiguity:** `PlanningFileEditorTool.create` at `_sdk_inspect/tools/planning_file_editor/definition.py:98-110` prefers legacy `{workspace}/PLAN.md` if it exists, otherwise `.agents_tmp/PLAN.md` — a repo containing both will silently use the legacy file, causing confusion about which plan is canonical.
- **Fork Does Not Carry Task State Explicitly:** `LocalConversation.fork` at `_sdk_inspect/sdk/conversation/impl/local_conversation.py:383-407` copies `EventLog` events and `agent_state` but has no explicit handling for `TASKS.json` file copy — if fork uses a new `persistence_dir`, the task list may start empty until next `plan` event, breaking plan continuity.
- **Observability Silent Failure:** Any exception in `Laminar.initialize` or `use_span` is swallowed with `logger.debug` at `_sdk_inspect/sdk/observability/laminar.py:288-290,319-321` — plan execution can appear untraced with no visible error, masking observability regressions.
- **Critic Gating Misses Planning Failures:** `CriticMixin._should_evaluate_with_critic` at `_sdk_inspect/sdk/agent/critic_mixin.py:34-46` only runs on `FinishAction` (in `finish_and_message` mode) — a run that never finishes (error/stuck) yields no critic score, so planning failures that prevent finishing are never scored.
- **500-iteration Cutoff Masks Plan Loops:** `LocalConversation.run` at `_sdk_inspect/sdk/conversation/impl/local_conversation.py:850-864` caps at `max_iteration_per_run=500` and emits `ConversationErrorEvent` — a plan that induces looping will be truncated with `ERROR` status indistinguishable from other errors.
- **Security Risk Swallowing Breaks Plan→Execution Trace:** `_extract_security_risk` at `_sdk_inspect/sdk/agent/agent.py:648-684` returns `UNKNOWN` when no analyzer is configured, even if LLM emitted `security_risk`; forensic analysis of whether risky plan steps were flagged is impossible without analyzer.

## Future Considerations

- **Add stable `id` + `parent_plan_version` to `TaskItem`** (`_sdk_inspect/tools/task_tracker/definition.py:32`): generate `uuid` per item on `plan`, persist full version history (append-only `TASKS.{version}.json` or event-only), and add optional `plan_item_id` / `plan_version` fields to `ActionEvent` (`_sdk_inspect/sdk/agent/agent.py:746-765`) so every tool call can declare the plan item it fulfills.
- **Introduce plan-coverage metrics:** After each `EventLog` append, compute `% plan items with ≥1 linked action`, `% actions with no plan link` (unplanned work), and `plan churn` (items added/removed between versions); expose via `ConversationStats` (`_sdk_inspect/sdk/conversation/conversation_stats.py:13`) and `EventServiceBase` filters.
- **Structured `PLAN.md` frontmatter:** Keep markdown for readability but add YAML frontmatter with machine-evaluable `steps: [{id, title, status, produced_files}]`, validated on `PlanningFileEditorExecutor` write — enables automated plan-conformance checks without sacrificing prose.
- **Plan-aware OTEL attributes:** In `observe` wrapper (`_sdk_inspect/sdk/observability/laminar.py:115`) and `RootSpan` (`...:253`), attach `plan.item_id` / `plan.version` as span attributes when an `ActionEvent` is linked, so Laminar traces support "which plan step was most expensive?" queries.
- **Planning eval harness:** Sample `get_planning_agent` runs (`_sdk_inspect/tools/preset/planning.py:154`) against a curated dataset of tasks with gold `PLAN.md` sketches; score with heuristics (section presence, step count, file reference precision) + LLM judge; run in CI to catch prompt regressions.
- **Plan diff + replay UI:** In `frontend/` and `EventServiceBase.search_events` (`openhands/app_server/event/event_service_base.py:94`), add a plan timeline view that interleaves `TaskTrackerObservation` / `PlanningFileEditorObservation` with `ActionEvent`/`ObservationEvent` in timestamp order, highlighting orphaned actions and unexecuted plan items — answers `plan vs execution` directly.

## Questions / Gaps

- **No evidence of planning eval dataset:** Searched `_sdk_inspect/sdk`, `tools`, and `openhands/app_server` for `eval`, `benchmark`, `dataset` directories; none contain planning-specific fixtures. If an external eval repo exists, it is out-of-scope per source-isolation rules.
- **No planning regression tests located:** The inspected checkout has no `tests/` directory mirroring planning code; grep for `test.*plan`, `TaskTracker`, `PlanningFileEditor` under `_sdk_inspect` returned only implementation files, not tests.
- **Unclear whether `PLAN.md` is consumed by execution agent:** `get_planning_agent` (`_sdk_inspect/tools/preset/planning.py:154`) creates a read-only planning agent; no evidence that the main `Agent` (`_sdk_inspect/sdk/agent/agent.py`) ingests `PLAN.md` content into its context or enforces plan adherence — plan may be purely advisory.
- **Unknown retention policy for `TASKS.json` / `.agents_tmp/PLAN.md`:** Not specified whether these files are included in conversation export (`iter_events_for_export`) or deleted on `delete_on_close=True` (`_sdk_inspect/sdk/conversation/impl/local_conversation.py:276-277`).
- **Laminar session linkage unverified:** `RootSpan` sets `trace_session_id` at `_sdk_inspect/sdk/observability/laminar.py:259-264`, but no code was found that queries traces by plan lifetime — end-to-end verification requires a live Laminar/OTEL deployment not available in static analysis.

---

Generated by `06.07-plan-observability-and-evaluation` against `openhands`.

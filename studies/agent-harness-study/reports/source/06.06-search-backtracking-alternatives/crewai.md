# Source Analysis: crewai

## Dimension 06.06: Search, Backtracking, and Alternative Plans

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python / Pydantic, OpenTelemetry, LiteLLM, CrewAI Flow/Crew |
| Analyzed | 2026-08-27 |

## Summary

CrewAI does not implement a search tree or explicit alternative-plan ranking. Its "search" model is checkpoint-branching: a persisted `RuntimeState` can be restored and forked onto a new `branch` label, creating an isolated copy-on-restore lineage that can be re-executed with different inputs. Concurrently, the Flow DSL provides deterministic conditional branching via `@router`/`or_`/`and_` listeners with racing `or_` semantics, and agents/crews provide bounded local retries (`max_retry_limit`, `guardrail_max_retries`, `max_execution_time`). Planning is single-plan generation (`CrewPlanner`, `AgentReasoning`) with replanning/refinement of the remaining `TodoList` but no parallel alternatives kept and no scoring function selecting among branches. Cost is bounded per-attempt (retry caps, RPM, `max_method_calls`, `max_checkpoints` pruning) but not globally across forks.

## Rating

**Score: 4 / 10 — Present but inconsistent, weakly documented, fragile**

Rationale: Checkpoint `fork` + `branch` + branch-aware providers supply an explicit, tested mechanism for trying alternatives in isolation (`lib/crewai/src/crewai/state/runtime.py:352`, `lib/crewai/src/crewai/crew.py:454`, `lib/crewai/tests/test_checkpoint.py:285`). Flow routing covers deterministic branching (`lib/crewai/src/crewai/flow/dsl/_router.py:97`, `lib/crewai/src/crewai/flow/dsl/_conditions.py:22`) with racing semantics tested (`lib/crewai/tests/test_flow.py:1402`). However, there is no multi-plan coexistence, no ranking/scoring of alternative branches (evaluation `quality: 0-10` in `lib/crewai/src/crewai/utilities/evaluators/task_evaluator.py:32` is post-hoc and unused for selection), no explicit backtracking primitive beyond manual `from_checkpoint`, and cost controls are per-run rather than per-search budget.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Branching logic — Crew/Flow fork | `Crew.fork(config, branch)` restores checkpoint then calls `state.fork(branch)`; `Flow.fork` does same and regenerates state `id` | `lib/crewai/src/crewai/crew.py:453-476`, `lib/crewai/src/crewai/flow/runtime/__init__.py:672-704` |
| Branching logic — RuntimeState fork | `RuntimeState.fork(branch)` generates `fork/{checkpoint_id}_{uuid6}` or `fork/{uuid8}`, emits `CheckpointForkStartedEvent`/`CheckpointForkCompletedEvent`, mutates only `self._branch` | `lib/crewai/src/crewai/state/runtime.py:352-389` |
| Branching logic — Flow router | `@router` decorator marks method `router=True` with declared `emit` events; return value becomes next event name. Conditional routing via `or_`/`and_` combinators builds `FlowDefinitionCondition` dicts | `lib/crewai/src/crewai/flow/dsl/_router.py:97-162`, `lib/crewai/src/crewai/flow/dsl/_conditions.py:22-86` |
| Branching logic — Flow racing OR | `_build_racing_groups` detects multi-event `or_` listeners whose events exclusively feed one OR listener; `_execute_racing_listeners` runs racers in parallel, first to complete wins, others cancelled | `lib/crewai/src/crewai/flow/runtime/__init__.py:1098-1221` |
| Forked sessions — Isolation | `JsonProvider.checkpoint` writes to `location/branch/{ts}_{uuid}_p-{parent}.json`; `SqliteProvider` stores `branch` + `parent_id` per row; `prune` is branch-aware; `_safe_branch` prevents directory escape | `lib/crewai/src/crewai/state/provider/json_provider.py:42-111`, `lib/crewai/src/crewai/state/provider/sqlite_provider.py:114-118`, `lib/crewai/src/crewai/state/provider/core.py:64-70` |
| Forked sessions — Lineage | `RuntimeState` serializes `crewai_version`, `parent_id`, `branch`, `entities`, `event_record`; `_chain_lineage` chains `parent_id` after each write; `from_checkpoint` restores `_checkpoint_id`/`_parent_id`/`_branch` and emits restore events | `lib/crewai/src/crewai/state/runtime.py:190-220`, `lib/crewai/src/crewai/state/runtime.py:216-227`, `lib/crewai/src/crewai/state/runtime.py:391-442` |
| Alternative plans — CrewPlanner | `CrewPlanner._handle_crew_planning` creates a single planning agent + one `Task` with `PlannerTaskPydanticOutput`; output is `list_of_plans_per_task: list[PlanPerTask]` (one plan per task, not multiple alternatives per task) | `lib/crewai/src/crewai/utilities/planning_handler.py:37-79`, `lib/crewai/src/crewai/utilities/planning_handler.py:15-34` |
| Alternative plans — Agent reasoning single plan | `AgentReasoning._execute_planning` creates one `ReasoningPlan(plan, steps, ready)`; `_refine_plan_if_needed` iterates until `ready` or `max_attempts`, no parallel candidates | `lib/crewai/src/crewai/utilities/reasoning_handler.py:244-349` |
| Alternative plans — TodoList replanning | `TodoList.replace_pending_todos(new_items)` preserves completed/failed/running, replaces only pending; `StepObservation.needs_full_replan`/`goal_already_achieved` decide continue/refine/replan but do not keep failed branch | `lib/crewai/src/crewai/utilities/planning_types.py:185-196`, `lib/crewai/src/crewai/utilities/planning_types.py:212-278` |
| Alternative plans — Hierarchical vs Sequential | `Crew.process` enum `Process.sequential`/`hierarchical` selects linear task loop vs manager-agent delegation; no parallel alternative execution | `lib/crewai/src/crewai/crew.py:251`, `lib/crewai/src/crewai/crew.py:1051-1054` |
| Scoring / Ranking — Task evaluation | `TaskEvaluation.quality: float 0-10` + `suggestions` produced by LLM via `Converter` to `TaskEvaluation`; `TrainingTaskEvaluation` likewise. Used only for `Crew.train()` human-feedback loop, not for branch selection | `lib/crewai/src/crewai/utilities/evaluators/task_evaluator.py:28-37`, `lib/crewai/src/crewai/utilities/evaluators/task_evaluator.py:69-113` |
| Scoring — Absence of ranking | No call site ranks or compares multiple concurrent plans/branches by `quality` score; grep of `quality` shows no comparator, sorter, or selector over alternative branches | `lib/crewai/src/crewai/utilities/evaluators/task_evaluator.py:32` (only definition) |
| Branch traces — EventRecord DAG | `EventRecord` stores `EventNode{event, edges: parent|child|trigger|next|previous|started|completed_by}`; `add` wires edges from `parent_event_id`, `triggered_by_event_id`, `previous_event_id`, `started_event_id` | `lib/crewai/src/crewai/state/event_record.py:64-146` |
| Branch traces — Checkpoint events | Types `CheckpointStartedEvent`, `CheckpointCompletedEvent`, `CheckpointFailedEvent`, `CheckpointForkStartedEvent`, `CheckpointForkCompletedEvent`, `CheckpointRestore*`, `CheckpointPrunedEvent` emitted on bus with `location`, `branch`, `parent_id`, `checkpoint_id`, `duration_ms` | `lib/crewai/src/crewai/events/types/checkpoint_events.py:1-40` (via `lib/crewai/src/crewai/state/runtime.py:229-284`), `lib/crewai/src/crewai/state/checkpoint_listener.py:129-216` |
| Cost limits — Retry caps | `Agent.max_retry_limit=2` (re-enter `execute_task` on non-litellm errors), `Agent.guardrail_max_retries=3`, `Task.guardrail_max_retries=3` (deprecated `max_retries`), `LiteAgent.guardrail_max_retries=3` | `lib/crewai/src/crewai/agent/core.py:288-291`, `lib/crewai/src/crewai/agent/core.py:359-361`, `lib/crewai/src/crewai/task.py:279-282`, `lib/crewai/src/crewai/lite_agent.py:276` |
| Cost limits — Execution time & RPM | `Agent.max_execution_time` validated positive int, enforced via `ThreadPoolExecutor` timeout; `RPMController.max_rpm` throttles and tracks current RPM | `lib/crewai/src/crewai/agent/core.py:244`, `lib/crewai/src/crewai/agent/utils.py:304-314`, `lib/crewai/src/crewai/utilities/rpm_controller.py:15-48`, `lib/crewai/src/crewai/agent/core.py:897-950` |
| Cost limits — Flow & checkpoint pruning | `Flow.max_method_calls=100` prevents infinite loops; `CheckpointConfig.max_checkpoints` triggers `provider.prune(location, max_keep, branch)` keeping newest N per branch | `lib/crewai/src/crewai/flow/runtime/__init__.py:614`, `lib/crewai/src/crewai/state/checkpoint_config.py:184-186`, `lib/crewai/src/crewai/state/checkpoint_listener.py:191-216` |
| Cost limits — Planning attempts | `PlanningConfig.max_attempts` bounds refine loop; `AgentReasoning._refine_plan_if_needed` warns and proceeds when exceeded | `lib/crewai/src/crewai/agent/planning_config.py:1-30` (via `lib/crewai/src/crewai/utilities/reasoning_handler.py:292-347`) |
| Retry — Tool & LLM passthrough | `_check_execution_error` ignores `litellm` module errors and passthrough exceptions (no retry), else increments `_times_executed` and re-calls `execute_task` recursively | `lib/crewai/src/crewai/agent/core.py:764-822` |
| Auto-checkpoint trigger — Selective | `_on_any_event` checks `is_replaying()` guard (no checkpoint on replay), ignores `CheckpointBaseEvent` family to avoid recursion, then checks `cfg.trigger_all` or `event.type in cfg.trigger_events` | `lib/crewai/src/crewai/state/checkpoint_listener.py:229-245`, `lib/crewai/src/crewai/state/checkpoint_config.py:207-212` |

## Answers to Dimension Questions

1. **Can the system try alternatives?**  
   Partial. Alternatives are not generated as a search tree. The framework can (a) walk conditional branches via Flow routers and `or_`/`and_` listeners with racing `or_` first-wins (`lib/crewai/src/crewai/flow/runtime/__init__.py:1098-1221`), and (b) create manual alternatives by restoring a checkpoint and forking to a new branch (`lib/crewai/src/crewai/crew.py:453-476`, `lib/crewai/src/crewai/state/runtime.py:352-389`) then re-kicking off with different `inputs`. There is no API to enumerate N candidate plans at a task and execute them in parallel; `CrewPlanner` returns exactly one `PlanPerTask` per task (`lib/crewai/src/crewai/utilities/planning_handler.py:57-78`). Retries (`max_retry_limit`, `guardrail_max_retries`) re-execute the *same* path, not an alternative path.

2. **Are alternatives isolated?**  
   Forked runs are isolated per branch. `JsonProvider` writes to `location/branch/*.json` and `SqliteProvider` partitions by `branch` column; `prune` operates per branch (`lib/crewai/src/crewai/state/provider/json_provider.py:98-111`). `RuntimeState` carries `branch` and `parent_id` lineage (`lib/crewai/src/crewai/state/runtime.py:177-184`), validated for directory traversal (`lib/crewai/src/crewai/state/provider/json_provider.py:22-34`). Flow fork also regenerates `state.id` to avoid ID collision (`lib/crewai/src/crewai/flow/runtime/__init__.py:699-703`). However, in-memory `Memory` rebind is lossy — restored `MemoryScope`/`MemorySlice` views rebind to a fresh `Memory()` if nothing persisted (`lib/crewai/src/crewai/crew.py:540-571`), so alternative branches do not share accumulated memories.

3. **How are alternatives compared?**  
   They are not formally compared. There is no scoring/ranking function that selects among branches. `TaskEvaluator` and `CrewEvaluator` produce a `quality 0-10` and `suggestions` via LLM (`lib/crewai/src/crewai/utilities/evaluators/task_evaluator.py:28-37`), but these are only used in the offline `Crew.train()` loop to aggregate training feedback (`lib/crewai/src/crewai/crew.py:965-972`), not during search. Flow's `or_` racing picks the first completed racer, not the highest-scored (`lib/crewai/src/crewai/flow/runtime/__init__.py:1206-1217`). No evidence of pairwise comparison, tournament, or cost-weighted selection.

4. **Is backtracking explicit?**  
   Brittle-explicit. `Crew.from_checkpoint`, `Flow.from_checkpoint`, `Agent.from_checkpoint` restore a `RuntimeState` snapshot (`lib/crewai/src/crewai/crew.py:428-451`, `lib/crewai/src/crewai/flow/runtime/__init__.py:622-670`), and `fork` explicitly sets a new branch label before next write. Resumption uses `checkpoint_kickoff_event_id` to skip completed tasks (`lib/crewai/src/crewai/crews/utils.py:143-152`, `lib/crewai/src/crewai/crew.py:530-534`). There is no `backtrack()` rollback primitive inside a running agent executor; `prepare_kickoff` only resets emission counters when not resuming (`lib/crewai/src/crewai/crews/utils.py:269-273`). Failed `TodoItem` status is retained as `failed` (`lib/crewai/src/crewai/utilities/planning_types.py:104-110`), but `replace_pending_todos` discards pending plan context without retaining the failed branch's trace for replay (`lib/crewai/src/crewai/utilities/planning_types.py:185-196`).

5. **Are costs bounded?**  
   Per-attempt costs are bounded, global search cost is not. Bounds include: `Agent.max_retry_limit=2` (`lib/crewai/src/crewai/agent/core.py:288`), `guardrail_max_retries=3` (`lib/crewai/src/crewai/task.py:279`), `Agent.max_execution_time` timeout (`lib/crewai/src/crewai/agent/core.py:897-950`), `RPMController.max_rpm` throttling (`lib/crewai/src/crewai/utilities/rpm_controller.py:15-48`), `PlanningConfig.max_attempts` (`lib/crewai/src/crewai/utilities/reasoning_handler.py:292-347`), `Flow.max_method_calls=100` (`lib/crewai/src/crewai/flow/runtime/__init__.py:614`), and `CheckpointConfig.max_checkpoints` pruning per branch (`lib/crewai/src/crewai/state/checkpoint_listener.py:191-216`). No budget tracks tokens/cost across multiple forks; a caller could `fork` in a loop and exhaust disk/API quota without a global limiter.

## Architectural Decisions

- **Branch-as-namespace checkpointing rather than search tree** — Chosen via `RuntimeState.branch` + `parent_id` lineage (`lib/crewai/src/crewai/state/runtime.py:181-183`) and branch-partitioned providers (`lib/crewai/src/crewai/state/provider/json_provider.py:37-68`). This enables manual A/B experimentation but avoids implementing graph search.
- **Event-sourced trace with edge-typed EventRecord** — All execution events append to `EventRecord.nodes` with typed edges (`parent`, `trigger`, `next`, `started`) (`lib/crewai/src/crewai/state/event_record.py:64-146`). This is the durable trace for alternatives; restored state carries the full `EventRecord` so prior failed branches remain queryable via `descendants()` (`lib/crewai/src/crewai/state/event_record.py:160-191`).
- **Declarative Flow routing over imperative branching** — `FlowDefinition` compiles `@start`/`@listen`/`@router` + `or_`/`and_` into static conditions (`lib/crewai/src/crewai/flow/dsl/_router.py:142-162`). Racing `or_` is handled centrally in runtime (`lib/crewai/src/crewai/flow/runtime/__init__.py:1098-1221`) rather than in user code, giving deterministic branching but limited to event-name equality, not scoring.
- **Single-plan planning with observation-driven replanning** — `AgentReasoning` and `TodoList`/`StepObservation` follow PLAN-AND-ACT: plan once, then after each step observe and optionally refine pending steps or trigger `needs_full_replan` (`lib/crewai/src/crewai/utilities/planning_types.py:212-278`). This is iterative refinement, not parallel alternatives.
- **Lazy auto-checkpoint registration** — `_ensure_handlers_registered` registers handlers on first `CheckpointConfig` occurrence, and `_on_any_event` short-circuits on `is_replaying()` and on `Checkpoint*` events to avoid infinite loops (`lib/crewai/src/crewai/state/checkpoint_listener.py:42-245`).

## Notable Patterns

- **Fork-then-kickoff pattern** — Documented fork tests show `Crew.fork`/`Flow.fork`/`Agent.fork` immediately after `from_checkpoint`, asserting `rt._branch == "experiment"` (`lib/crewai/tests/test_checkpoint.py:285-308`, `lib/crewai/tests/test_checkpoint.py:673-700`). Callers must manually provide divergent inputs; no sugar to fork N times with sweep params.
- **Guardrail retry as local backtrack** — Both `Task` (`lib/crewai/src/crewai/task.py:1339-1410`) and `LiteAgent` (`lib/crewai/src/crewai/lite_agent.py:746-760`) accumulate failures via `tool_failure_collector` and re-enter the guardrail check up to `guardrail_max_retries`, resetting the agent's failure list each time but preserving via `merge_tool_failures`.
- **Branch-aware pruning** — `max_checkpoints` keeps newest N per branch (`lib/crewai/src/crewai/state/provider/json_provider.py:98-111`, `lib/crewai/src/crewai/state/provider/sqlite_provider.py:114`), so experiments on one branch do not evict checkpoints on `main`.

## Tradeoffs

- **Durability vs complexity**: Persisting the entire `RuntimeState` (entities + `EventRecord` + execution context) on every `task_completed` (`lib/crewai/src/crewai/state/checkpoint_config.py:172`) gives reliable resume but writes large JSON blobs (includes LLM model dump, memory views). Pruning mitigates disk but not per-write latency (`duration_ms` tracked in `CheckpointCompletedEvent` at `lib/crewai/src/crewai/state/runtime.py:272-284`).
- **Simplicity vs search power**: Avoiding a search abstraction keeps the API surface small (fork/branch as strings) but forces callers to orchestrate alternatives externally; there is no built-in beam search, Monte-Carlo rollouts, or cost-aware planner.
- **Determinism vs flexibility**: Flow's racing `or_` guarantees exactly one branch fires for competing triggers (`lib/crewai/src/crewai/flow/runtime/__init__.py:1206-1217`), preventing nondeterministic double-firing, but it arbitrarily picks fastest completion, not best result.
- **Isolation vs sharing**: Fresh `Memory()` fallback on restore (`lib/crewai/src/crewai/crew.py:565-567`) guarantees isolation yet loses learned context that could benefit alternative branches.

## Failure Modes / Edge Cases

- **Fork without checkpoint_id collision fallback is weak** — `fork` auto-branch uses `uuid4().hex[:8]` when `_checkpoint_id` is None (`lib/crewai/src/crewai/state/runtime.py:368`), but tests show two successive forks produce different branches only by chance of UUID (`lib/crewai/tests/test_checkpoint.py:300-308`); no monotonic guarantee.
- **Branch traversal rejected late** — `_safe_branch` validates `branch` resolves inside base dir, but only at write/prune time (`lib/crewai/src/crewai/state/provider/json_provider.py:22-34`, `:100`, `:162`); a malicious `branch` stored in serialized state could be deserialized (`_branch = data.get("branch", "main")` at `lib/crewai/src/crewai/state/runtime.py:211`) before validation on next write.
- **Stale checkpoint field leakage** — `_sync_checkpoint_fields` copies `task._original_description` into `task.checkpoint_original_description` on every write (`lib/crewai/src/crewai/state/runtime.py:84-86`); if a caller mutated a task description post-fork, the original is overwritten on next checkpoint, losing the divergence point.
- **Agent executor reused across tasks stamped as completion** — Checkpoint serialization stamps every live Flow's `completed_methods` (`lib/crewai/src/crewai/state/runtime.py:65-79`), which for the reused `AgentExecutor` flow would falsely signal restore on next task; mitigated by `_restored_from_checkpoint` guard in task loop (`lib/crewai/tests/test_checkpoint.py:755-781`), but fragile.
- **No global budget across forks** — `max_retry_limit`, `max_method_calls`, `max_execution_time` apply per-run; forking in a tight loop can generate unbounded checkpoint files and LLM calls until `max_checkpoints` pruning throttles only storage, not API cost.
- **Event replay skips checkpoint but still mutates state** — `is_replaying()` suppresses checkpoint writes (`lib/crewai/src/crewai/state/checkpoint_listener.py:231-232`) but does not suppress `usage_metrics` or `memory` side effects, so a replayed branch could double-count usage (`lib/crewai/src/crewai/flow/runtime/__init__.py:918-925`).

## Future Considerations

- **Explicit alternative-plans API**: Add `crew.branch(inputs=...)` or `Flow.branch(params)` that atomically forks, applies divergent inputs/state patches, and returns a typed `BranchHandle` with `parent_id` tracking, reducing manual `from_checkpoint` + `fork` boilerplate (`lib/crewai/src/crewai/crew.py:453-476`).
- **Scoring & selection layer**: Reuse `TaskEvaluation.quality` (`lib/crewai/src/crewai/utilities/evaluators/task_evaluator.py:32`) as a pluggable `ScoringFunction` over a batch of branches; store `BranchEvaluation` alongside `EventRecord` to enable `select_best(branch_ids)` without caller-side LLM orchestration.
- **Bounded search budget**: Extend `CheckpointConfig` with `max_branches` / `max_cost` that the listener enforces per lineage tree, emitting `CheckpointPrunedEvent` or `BranchBudgetExceededEvent` when exceeded.
- **Retain failed branch traces for backtracking**: Persist `TodoList` failed items and associated `EventNode` subgraph when `replace_pending_todos` is called, enabling "recover from bad path without forgetting what it learned" audits.
- **Deterministic fork IDs**: Use `parent_id` + monotonic counter instead of truncated UUID for auto-branch names to guarantee uniqueness and sortability (`lib/crewai/src/crewai/state/runtime.py:365-368`).

## Questions / Gaps

- No evidence found that `knowledge_sources` or `memory` embeddings are fork-aware; search did not reveal per-branch vector-store partitioning. Searched `lib/crewai/src/crewai/memory` and found `bind(Memory())` fallback only (`lib/crewai/src/crewai/flow/runtime/__init__.py:731-735`).
- No evidence that `Crew.kickoff_for_each` / `akickoff_for_each` evaluates or ranks results across inputs; it aggregates `UsageMetrics` but returns raw list (`lib/crewai/src/crewai/crew.py:1091-1136`).
- No evidence of declarative `retry`/`backtrack` DSL beyond `guardrail_max_retries` and `@router` conditionals; `Flow` does not expose a `backtrack(to_method)` primitive.
- Missing load test: how large can `EventRecord.nodes` grow before `model_dump_json()` on every `task_completed` becomes a bottleneck? No benchmark found.

---
Generated by `Dimension 06.06: Search, Backtracking, and Alternative Plans` against `crewai`.

# Source Analysis: langgraph

## Dimension 06.07: Plan Observability and Evaluation

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (monorepo: libs/langgraph, libs/checkpoint, libs/prebuilt, libs/cli) |
| Analyzed | 2026-08-27 |

## Summary

LangGraph is a stateful graph execution framework, not a planning agent. It provides **no first-class Plan / PlanItem model, no plan log, and no planning eval harness**. Planning is user-implemented as ordinary state (e.g., `state["plan"] = [...]`) and routing via `Command(goto=...)` or `Send`. The framework compensates with mature **generic execution observability**: every super-step produces a `Checkpoint`/`CheckpointMetadata` (step, source, parents), every node execution is a `PregelExecutableTask` with deterministic `task.id`/`name`/`triggers`/`writes`, and `StateSnapshot` + `stream_mode="debug|checkpoints|tasks|values|updates"` + `get_state_history()` provide full time-travel replay. This is sufficient to reconstruct a user-defined plan stored in state and compare plan vs. execution, but requires manual wiring — there are no plan quality metrics, no per-action plan-item linking, no eval datasets, and no planning regression tests. The old `examples/plan-and-execute` tutorial has been moved/redirected.

## Rating

**3/10 — Absent / ad-hoc for planning; mature for generic execution**

Rationale: Core implements durable execution tracing at production grade, but planning observability is entirely implicit. Plans are only observable iff the developer persists them in state; there is no `plan_id`/`plan_item_id`, no plan trace schema, no evaluation harness to score plan quality or measure planning-vs-success correlation, and no automated regression for planning. The `test_multistep_plan` test validates mechanical routing — not plan quality. For the dimension question *“Can you debug a failed run by comparing plan vs execution?”* the answer is **yes, but only by repurposing generic state/trace infra**, not via a planning-aware API.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Plan traces (generic execution trace) | `TASK_NAMESPACE` and `map_debug_tasks` emit task start payloads `{id, name, input, triggers, metadata}` | `libs/langgraph/langgraph/pregel/debug.py:38-71` |
| Plan traces | `map_debug_task_results` yields `TaskResultPayload {id, name, error, result: channel->writes, interrupts}` | `libs/langgraph/langgraph/pregel/debug.py:106-128` |
| Plan traces | `map_debug_checkpoint` yields `CheckpointPayload {config, parent_config, values, metadata, next, tasks}` | `libs/langgraph/langgraph/pregel/debug.py:144-206` |
| Plan traces | `tasks_w_writes()` merges `pending_writes` + subgraph states into `PregelTask` list surfaced in `StateSnapshot.tasks` | `libs/langgraph/langgraph/pregel/debug.py:209-279` |
| Stream IDs / execution links | `PregelExecutableTask` defined with `id: str, name, path, triggers, writes, config, cache_key` | `libs/langgraph/langgraph/types.py:666-681` |
| Stream IDs / execution links | `CheckpointTask` + `TaskPayload`/`TaskResultPayload` TypedDicts carry stable `id` for task-level correlation | `libs/langgraph/langgraph/types.py:144-204` |
| Stream IDs / execution links | `StateSnapshot {values, next, config, metadata, created_at, parent_config, tasks: tuple[PregelTask], interrupts}` | `libs/langgraph/langgraph/types.py:683-701` |
| Stream IDs / execution links | `PregelTask NamedTuple {id, name, path, error, interrupts, state, result}` | `libs/langgraph/langgraph/types.py:637-647` |
| Checkpoint linkage | `CheckpointMetadata {source: input|loop|update|fork, step, parents: dict[ns->id], run_id, counters_since_delta_snapshot}` | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:38-86` |
| Checkpoint linkage | `Checkpoint {v, id, ts, channel_values, channel_versions, versions_seen, pending_sends, updated_channels}` | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:92-123` |
| Checkpoint linkage | `BaseCheckpointSaver.get_tuple/list/put/put_writes/delete_thread/copy_thread/prune` + `get_delta_channel_history` ancestor walk | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:227-690` |
| Execution observability | `PregelLoop.tick()` prepares `self.tasks` via `prepare_next_tasks`; emits `tasks`/`checkpoints` via `_emit`+`map_debug_*` | `libs/langgraph/langgraph/pregel/_loop.py:599-674` |
| Execution observability | `PregelLoop._put_checkpoint()` creates `create_checkpoint()` with `channel_versions`, `updated_channels`, `channels_to_snapshot` | `libs/langgraph/langgraph/pregel/_loop.py:1081-1219` |
| State inspection (plan-in-state) | `_prepare_state_snapshot()` hydrates channels, computes `next_tasks`, builds `StateSnapshot` with `tasks_w_writes` | `libs/langgraph/langgraph/pregel/main.py:1145-1266` |
| State inspection | `Pregel.get_state()` / `aget_state()` / `get_state_history()` delegate to snapshot preparation | `libs/langgraph/langgraph/pregel/main.py:1392-1505` |
| Stream modes | `StreamMode = "values"|"updates"|"checkpoints"|"tasks"|"debug"|"messages"|"custom"` + `StreamPart` union | `libs/langgraph/langgraph/types.py:122-367` |
| Plan emulation (test) | `test_multistep_plan` implements plan as `state["plan"]: list[str|list[str]]` with `planner` node returning `Command(goto=next_step, update={"plan": next_plan})` | `libs/langgraph/tests/test_pregel.py:5165-5219` |
| Plan emulation (async) | `test_multistep_plan` async variant | `libs/langgraph/tests/test_pregel_async.py:6420-6474` |
| Planning deprecation | `/concepts/plans` and `/tutorials/plan-and-execute/plan-and-execute` redirects removed; plan-and-execute now points to `langchain/middleware/built-in#to-do-list` | `docs/redirects.json:125,190` |
| Plan absence (prebuilt) | `create_react_agent` wires `agent`→`tools` loop via `StateGraph`; tool dispatch uses `Send("tools", ToolCallWithContext)` v2, no plan/todo state | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:830-991` |
| Lifecycle observability | `GraphCallbackHandler` with `on_interrupt(GraphInterruptEvent{checkpoint_id, checkpoint_ns, interrupts})` / `on_resume(GraphResumeEvent)` | `libs/langgraph/langgraph/callbacks.py:42-76,87-112` |
| Benchmarks (perf, not planning) | `bench/__main__.py:99-515`, `test_delta_channel_benchmark.py:276-320` — throughput/latency benchmarks, no planning quality benchmark | `libs/langgraph/bench/__main__.py:99` |
| No plan eval harness | `grep eval|metric|benchmark` across `libs/langgraph` returns only perf benchmarks; zero hits for plan eval datasets | `libs/langgraph` (search boundary, no file) |

## Answers to Dimension Questions

**1. Are plans observable?**
Partially, via generic state observability. There is no `Plan` object. If a developer stores a plan in state (as in `test_multistep_plan`), the plan becomes observable because every checkpoint persists `channel_values` and every `StateSnapshot.values` and `CheckpointPayload.values` exposes it (`libs/langgraph/langgraph/pregel/main.py:1257`, `libs/langgraph/langgraph/pregel/debug.py:179`). Trace policy can redact/transform per-node inputs/outputs (`libs/langgraph/langgraph/types.py:532-567`) but there is no plan-specific log stream. Without user discipline, no evidence found of plan capture.

**2. Can each action be linked to a plan item?**
No native link. Each action is a `PregelExecutableTask` with `id` (`libs/langgraph/langgraph/types.py:676`), `path` tuple, and `triggers` (`libs/langgraph/langgraph/types.py:154`), and result writes are keyed by channel. Linking to a logical plan item requires the node to publish `plan_item_id` explicitly in its write (e.g., updating `state["plan"]` + `Send` arg). The `Command(goto, update)` plumbing (`libs/langgraph/langgraph/types.py:798-831`) can carry arbitrary `update` payloads, so a convention could be built, but no framework enforcement or index. `pending_writes` are `(task_id, channel, value)` tuples (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:31`), giving technical lineage but not semantic plan-step lineage.

**3. Are plans evaluated?**
No. No eval harness, dataset, or scorer for planning found. `libs/langgraph/tests/` contains execution correctness tests (e.g., time-travel `test_time_travel.py:15`, delta-channel exit mode `test_delta_channel_exit_mode.py:50`), but none measure plan completeness, feasibility, or optimality. `bench/` measures tokens/step and channel throughput (`libs/langgraph/bench/__main__.py:99`), not plan quality. LangSmith integration (`libs/langgraph/langgraph/callbacks.py:258`) provides external tracing but not in-repo evaluation.

**4. Can poor planning be diagnosed?**
Yes — via generic debug infra, not planning-aware tooling. A failed run can be diagnosed by: `graph.get_state_history(config)` → walk `StateSnapshot` chain (parent_config, metadata.step, tasks with error/interrupts); `stream(..., stream_mode="debug")` which emits interleaved `checkpoint` + `task` + `task_result` events (`libs/langgraph/langgraph/pregel/main.py:2691`); and time-travel re-invoke from a prior `checkpoint_id`. Differences between planned steps (values["plan"]) and executed `tasks[i].path`/`name` are visible by diffing snapshots, as demonstrated by the plan-in-state pattern. No automated “plan deviation” detector.

**5. Does planning improve success rate?**
No evidence in-repo. There are no A/B planning experiments, no metrics tying plan presence to task success, and no documentation claiming measured improvement. ReAct agent (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:278`) loops without an explicit planner; `deepagents` reference in `README.md:31,54` is external. Any improvement claim would be user-supplied.

## Architectural Decisions

- **Generic graph over specialized planner:** LangGraph deliberately models execution as `Pregel` with channels and nodes (`libs/langgraph/langgraph/pregel/main.py:1`). Planning is delegated to application State + `Command` routing (`libs/langgraph/langgraph/types.py:798`). Tradeoff: maximal flexibility, zero opinion on plan representation.
- **Checkpoint-centric observability:** All observability derives from the checkpointer (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:176`) and `StateSnapshot` (`libs/langgraph/langgraph/types.py:683`). `PregelLoop` checkpoints per superstep (`libs/langgraph/langgraph/pregel/_loop.py:1081`), and debug streams are views over checkpoints/tasks. This makes “plan trace” a materialized view of state history.
- **Task identity via stable `task.id` + `checkpoint_ns`:** `TASK_NAMESPACE = UUID("6ba7b831-...")` (`libs/langgraph/langgraph/pregel/debug.py:38`) + `PregelTask.path` gives deterministic lineage across subgraph hierarchies. Enables execution linking even without plan IDs.
- **Deprecated plan-and-execute:** Redirects in `docs/redirects.json:190` retire the dedicated plan-and-execute tutorial in favor of `langchain/middleware/to-do-list`. Aligns with moving planning out of `langgraph` core (now covered by `deepagents` / middleware).
- **Tracing delegated to LangSmith / callbacks:** Graph lifecycle callbacks are limited to `interrupt`/`resume` (`libs/langgraph/langgraph/callbacks.py:87`), while detailed LLM/tool tracing flows through `langchain_core` callbacks + `TracePolicy`.

## Notable Patterns

- **Plan-as-state pattern:** `test_multistep_plan` (`libs/langgraph/tests/test_pregel.py:5165`) is the canonical idiom — a `planner` node stores `plan: list[str|list[str]]` and drives execution via `Command(goto=...)`. Validates that any plan semantics must be re-implemented per graph.
- **Stream mode multiplexing:** Consumers select `stream_mode` (`libs/langgraph/langgraph/types.py:122`) to get incremental `values`/`updates` vs. full `checkpoints`/`tasks`/`debug`. Debug mode unifies checkpoint + task streams for plan-vs-execution diffing.
- **Pending-writes + versions_seen protocol:** Determines `next` nodes without re-executing and underpins accurate `get_state` replay (`libs/langgraph/langgraph/pregel/main.py:1178-1195`, `libs/langgraph/langgraph/pregel/_loop.py:599-629`).
- **Subgraph state rehydration:** `task_states: dict[str, RunnableConfig|StateSnapshot]` in `map_debug_checkpoint` and `_prepare_state_snapshot` (`libs/langgraph/langgraph/pregel/debug.py:157`, `libs/langgraph/langgraph/pregel/main.py:1199`) lets parent traces drill into nested plan executors.

## Tradeoffs

- **Flexibility vs. planning guarantees:** No schema validation for plans; no compile-time check that `goto` targets exist (`libs/langgraph/langgraph/pregel/_validate.py:1` validates nodes but not semantic plan steps). Corrupt plans fail late as `GraphInterrupted`/`error` in `task_result` (`libs/langgraph/langgraph/pregel/debug.py:115-128`).
- **Durability modes (`sync`/`async`/`exit`):** `PregelLoop.durability` (`libs/langgraph/langgraph/types.py:89`) and `_exit_delta_writes` handling (`libs/langgraph/langgraph/pregel/_loop.py:215-223`) trade trace completeness for latency. `exit` may lose intermediate task granularity needed for fine-grained plan audit.
- **Observability cost:** Full checkpoint + debug streams persist full `channel_values` per step; `DeltaChannel` + snapshot frequency mitigates but requires tuning (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:63-86`).
- **Externalized evaluation:** By omitting in-repo eval datasets/metrics, LangGraph avoids opinionated plan scoring but forces teams to build their own harness (often via LangSmith), risking inconsistent planning QA.

## Failure Modes / Edge Cases

- **No plan schema → silent drift:** A planner returning `plan = ["step1", ["step2", "step3"]]` is trusted; typos or missing nodes surface only as `PregelTask.error` at runtime. No pre-execution plan validation.
- **Implicit plan linkage fragile:** Without conventional `plan_item_id`, correlating a tool call (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1`) to its originating plan step relies on ad-hoc state updates; async `Send` fan-out multiplies this risk.
- **Checkpoint history pruning breaks plan replay:** `prune`/`delete_thread` docs in `libs/checkpoint/langgraph/checkpoint/base/__init__.py:374-415` warn `DeltaChannel` walks break if ancestors are dropped. Post-pruning, `get_state_history()` may silently return truncated plan lineage.
- **Time-travel resume ambiguity:** `PregelLoop._first()` resuming logic + `ReplayState` (`libs/langgraph/langgraph/pregel/_loop.py:848-1079`) drops stale `RESUME` writes to re-fire interrupts; mis-identifying a fork as a resume can lose plan progress.
- **Version skew on `channel_versions`:** `StateSnapshot` reconstruction via `prepare_next_tasks` depends on monotonic `get_next_version` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:692-711`). Custom checkpointer versioning bugs can hide pending tasks, so a plan appears “stuck” with empty `next`.

## Future Considerations

- **Add optional `Plan` channel type:** Define a first-class `PlanChannel` / middleware `TodoList` schema with `plan_id`, `items: [{id, description, status}]`, and automatic `task.id → plan_item_id` annotation (e.g., via `TracePolicy` or `ToolCallWithContext`). Migrated from `deepagents` TODO middleware (hinted by `docs/redirects.json:190`).
- **Emit plan events on dedicated stream mode:** Extend `StreamMode` (`libs/langgraph/langgraph/types.py:122`) with `plan`/`plan_items` and `PlanStreamPart` so dashboards can subscribe without parsing generic `values`.
- **Ship planning eval harness:** Add `libs/langgraph/tests/eval_planning/` with golden plan datasets + scorers (coverage, ordering, feasibility) and wire into CI, mirroring perf `bench/__main__.py:99`.
- **Track plan-quality metrics per checkpoint:** Extend `CheckpointMetadata` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:38`) with optional `plan_metrics: {coverage, steps_planned, steps_executed, replan_count}` populated by a `PlanningMetricsCallback`.
- **Formalize plan-vs-execution diff tool:** Build a CLI/debug helper that diffs `values["plan"]` across `get_state_history()` and flags deviations (skipped/reordered steps).

## Questions / Gaps

- No evidence found of plan item IDs, plan traces, or plan quality metrics in `libs/langgraph`, `libs/prebuilt`, or `libs/checkpoint` (searched `plan`, `todo`, `TodoList`, `eval`, `metric`, `benchmark` boundaries; only perf benchmarks surfaced). State-of-the-art planning patterns now live outside core (referenced `deepagents`, `langchain.agents` middleware) — not inspected per isolation rules.
- Unclear whether `remaining_steps: RemainingSteps` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:62`) should be interpreted as a minimal planning budget/metric — not instrumented as evaluation signal.
- No docs within source directory confirm intended success-rate methodology for planning; would require inspecting external `langsmith` / `deepagents` repos.

---
Generated by `studies/agent-harness-study/dimensions/06.07-plan-observability-and-evaluation.md` against `langgraph`.

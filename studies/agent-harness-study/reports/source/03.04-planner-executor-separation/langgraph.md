# Source Analysis: langgraph

## 03.04 Planner/Executor Separation

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python 3.10+ (core `langgraph` package, `langgraph-prebuilt`); monorepo with parallel `checkpoint`, `checkpoint-postgres`, `checkpoint-sqlite`, `cli`, `sdk-py`, `sdk-js` (TypeScript) libraries |
| Analyzed | 2026-07-14 |

## Summary

LangGraph ships **no first-class planner/executor architecture**. It is a low-level orchestration framework that compiles a `StateGraph` (or a `@entrypoint` functional API) into the `Pregel` execution model — a Bulk Synchronous Parallel (BSP) superstep loop. Every reference to "Plan" inside the core engine refers to the BSP algorithm's per-superstep scheduling phase (`libs/langgraph/langgraph/pregel/main.py:465-468`), not to an LLM-driven plan artifact. Plans, when they exist at all, are **invented and persisted by user code** — most commonly as a list held in the graph state and consumed via `Command(goto=...)` or conditional edges. The only multistep-plan example in the test suite (`libs/langgraph/tests/test_pregel.py:5179-5234`) builds the planner by hand, stores the remaining plan in a `plan` channel, and uses `Command(goto=...)` to dispatch the next step.

The framework exposes primitives that can be assembled into a planner pattern:

- **`Command(goto=...)` and `Command.update=...`** — `libs/langgraph/langgraph/types.py:759-808` lets a node return a routing + state-update instruction. It is processed by `map_command` (`libs/langgraph/langgraph/pregel/_io.py:56-78`) and emitted as pending writes on the `TASKS` / branch channels.
- **`Send` (PUSH tasks)** — `libs/langgraph/langgraph/types.py:664+` schedules a fan-out task in the next superstep (`libs/langgraph/langgraph/pregel/_algo.py:938-1107`).
- **`interrupt_before` / `interrupt_after`** — `libs/langgraph/langgraph/pregel/main.py:720-722,767-768` lets the user pin pause points, which the loop honours in `should_interrupt` (`libs/langgraph/langgraph/pregel/_algo.py:155-185`).
- **Node-level error handler** — `libs/langgraph/langgraph/graph/_node.py:91-92` and `libs/langgraph/langgraph/graph/state.py:857-879` lets a node specify `error_handler=`; the compiled map `node_error_handler_map` is fed into `PregelRunner` (`libs/langgraph/langgraph/pregel/_runner.py:147-174`) and routed via `prepare_node_error_handler_task` (`libs/langgraph/langgraph/pregel/_algo.py:1110-1248`).

But the framework itself owns **no plan schema, no plan-update tool, no plan-prompt template, no plan-revision event, and no notion of a "replan"** — the word does not appear anywhere in the source (`grep` of `replan` returns 0 hits across `libs/`). Plan observability reduces to the standard BSP trajectory metadata (`langgraph_step`, `langgraph_node`, `langgraph_path`, `langgraph_triggers` on each `PregelExecutableTask` — `libs/langgraph/langgraph/pregel/_algo.py:654-660`).

## Rating

**2 / 10 — Absent.** No plan schema, no planner prompt, no plan-update tool, no replanning event. Planning exists only as a **user convention** ("hold a list in state, branch on it"). The "Plan" the engine knows about is the BSP algorithm's per-superstep actor-selection phase, not an LLM-generated artifact. Because planning is purely user-defined, the system **cannot compare what it planned to what it actually did** unless the user also writes the bookkeeping.

## Evidence Collected

Every entry includes file path and line numbers. Format: `path/to/file.py:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| BSP "Plan" phase (algorithm only, not LLM) | "Plan: Determine which actors to execute in this step" | `libs/langgraph/langgraph/pregel/main.py:465-468` |
| Per-superstep actor selection | `prepare_next_tasks` selects PUSH + PULL tasks from trigger_to_nodes + updated_channels | `libs/langgraph/langgraph/pregel/_algo.py:392-513` |
| `Command` dataclass with `goto` + `update` | `class Command(Generic[N], ToolOutputMixin)` | `libs/langgraph/langgraph/types.py:758-808` |
| `Command.goto` → TASKS / branch-channel writes | `map_command` translates `Command` into `(task_id, channel, value)` triples | `libs/langgraph/langgraph/pregel/_io.py:56-78` |
| `Send` → PUSH task in next superstep | `prepare_push_task_send` | `libs/langgraph/langgraph/pregel/_algo.py:938-1107` |
| State snapshot exposing "next" (would-run, not planned) | `class StateSnapshot.next: tuple[str, ...]` = name of node to execute in each task | `libs/langgraph/langgraph/types.py:643-661` |
| Snapshot population of `next` | `tuple(t.name for t in next_tasks.values() if not t.writes)` | `libs/langgraph/langgraph/pregel/main.py:1258` |
| Multistep plan test (user-defined planner via `Command.goto`) | `test_multistep_plan` — planner node holds `plan: list` in state, picks first step, returns `Command(goto=..., update={"plan": ...})` | `libs/langgraph/tests/test_pregel.py:5179-5234` |
| Async twin of the plan test | `test_command_with_static_breakpoints` (async) with same `planner` shape | `libs/langgraph/tests/test_pregel_async.py:6387-6478` |
| `examples/plan-and-execute/` directory is a stub | Notebook is a redirect to moved docs | `examples/plan-and-execute/plan-and-execute.ipynb:8-9` |
| No built-in planner agent / `Plan` class | `grep -rn "PlanExecute\|plan_executor\|PlanStep" libs/` returns 0 hits | (search boundary) |
| No replanning mechanism | `grep -rn "replan" libs/` returns 0 hits | (search boundary) |
| No todo system | `grep -rn "todo" libs/langgraph/langgraph/` returns 0 hits | (search boundary) |
| Node-level error handler (not a planner) | `StateNodeSpec.error_handler_node` + auto-named handler node | `libs/langgraph/langgraph/graph/_node.py:84-95`, `libs/langgraph/langgraph/graph/state.py:857-904` |
| Handler-task preparation | `prepare_node_error_handler_task` injects `NodeError(node, error)` into config | `libs/langgraph/langgraph/pregel/_algo.py:1110-1248` |
| Runner routes to handler | `PregelRunner.schedule_error_handler` / `_should_route_to_error_handler` | `libs/langgraph/langgraph/pregel/_runner.py:147-174,171-174,1549-1584,1803-1838` |
| Handler wiring from graph compile | `node_error_handler_map` populated and passed to Pregel | `libs/langgraph/langgraph/graph/state.py:1327-1331,1354` |
| No plan-persistence schema in checkpoint | `Checkpoint` has `channel_values`, `channel_versions`, `versions_seen`, `updated_channels` — no plan field | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:92-136` |
| Checkpoint source enum distinguishes trajectory, not plan | `CheckpointMetadata.source` ∈ {"input", "loop", "update", "fork"} | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:38-48` |
| Step counter (superstep) is the only "plan-like" identifier | `CheckpointMetadata.step: int` | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:49-55` |
| Per-task observability (BSP trajectory only) | `metadata["langgraph_step"|"langgraph_node"|"langgraph_path"|"langgraph_triggers"]` | `libs/langgraph/langgraph/pregel/_algo.py:654-660` |
| Interrupt pause-points | `interrupt_before_nodes` / `interrupt_after_nodes` on Pregel | `libs/langgraph/langgraph/pregel/main.py:720-722,767-768` |
| Interrupt evaluation | `should_interrupt` raises `GraphInterrupt` if any triggered node is in the list | `libs/langgraph/langgraph/pregel/_algo.py:155-185` |
| Interrupt enforcement inside the loop | `tick()` raises `GraphInterrupt`; `after_tick()` raises on `interrupt_after` | `libs/langgraph/langgraph/pregel/_loop.py:660-664,708-712` |
| Retry policy (intra-node, not plan revision) | `RetryPolicy` with `initial_interval`, `backoff_factor`, `max_interval`, `max_attempts`, `jitter`, `retry_on` | `libs/langgraph/langgraph/types.py:416-435` |
| Remaining-steps managed value (advisory budget) | `RemainingSteps = Annotated[int, RemainingStepsManager]` derived from scratchpad `step`/`stop` | `libs/langgraph/langgraph/managed/is_last_step.py:1-24` |
| Scratchpad (per-run, not per-plan) | `PregelScratchpad(step, stop, call_counter, interrupt_counter, get_null_resume, resume, subgraph_counter)` | `libs/langgraph/langgraph/_internal/_scratchpad.py:1-19` |
| Main superstep driver (BSP loop, no plan phase) | `while loop.tick(): ... runner.tick(...) ... loop.after_tick()` | `libs/langgraph/langgraph/pregel/main.py:2979-2999` |

## Answers to Dimension Questions

1. **Is there an explicit plan?**
   No. LangGraph's `Checkpoint` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:92-123`) holds `channel_values` / `channel_versions` / `versions_seen` / `updated_channels` plus a per-task `task_path`. There is no plan field, no plan record, no plan schema, no `Plan` dataclass. The only "Plan" mention in the engine is the BSP algorithm's per-superstep actor-selection phase at `libs/langgraph/langgraph/pregel/main.py:465-468`, which is mechanical (pick actors whose trigger channels were updated), not LLM-driven.

2. **Does the plan control execution?**
   N/A — there is no framework plan. Execution is controlled by **(a) the static graph topology** (edges, conditional edges, destinations), **(b) `Command(goto=...)` and `Send` returned by user code** (`libs/langgraph/langgraph/types.py:758-808`, `libs/langgraph/langgraph/pregel/_io.py:56-78`), and **(c) interrupt points** (`libs/langgraph/langgraph/pregel/main.py:720-768`). When a user builds a multistep plan, they encode it as a list in state and return `Command(goto=next_step, update={"plan": remaining})` themselves (`libs/langgraph/tests/test_pregel.py:5186-5212`). The framework does not enforce that the plan drives the execution — the planner is just another node.

3. **Can the plan change?**
   Yes, but only because the user mutates the state-held plan and returns a new `Command`. There is no plan-revision event, no `plan_update` tool, no prompt template, no diff mechanism. The only structured "change" the framework provides for free is **error routing**: when a node fails, the runner consults `node_error_handler_map` (`libs/langgraph/langgraph/pregel/_runner.py:147-174`) and schedules `prepare_node_error_handler_task` (`libs/langgraph/langgraph/pregel/_algo.py:1110-1248`) with a `NodeError(node, error)` injected into config (`libs/langgraph/langgraph/pregel/_algo.py:1236-1238`). This is a node-level fallback, not a plan revision.

4. **Are plan changes traceable?**
   Only at the granularity of the BSP trajectory. Each `PregelExecutableTask` carries `langgraph_step`, `langgraph_node`, `langgraph_path`, `langgraph_triggers` (`libs/langgraph/langgraph/pregel/_algo.py:654-660`), and each checkpoint records `CheckpointMetadata.source ∈ {"input","loop","update","fork"}` plus `step` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:38-55`). This lets you replay "what ran, in what order" but does not let you replay "what was the plan when this node ran". A user can reconstruct a planned-vs-actual diff only by reading user-authored state (e.g., a `plan` channel) at each checkpoint, which the framework itself does not do.

5. **Is planning useful or decorative?**
   Decorative. The framework neither provides a planner nor benefits from one being present. A planner node, if added, is just another node in the BSP superstep loop. Its only "levers" are the same levers every other node has: write to state, return `Command`, raise `interrupt()`. There is no validation that the plan is consistent, that the plan was followed, that the plan was updated when it should have been, or that the plan survives errors.

**Bonus question — Can the system compare what it planned to what it actually did?**
   No. The system records what nodes ran (task trajectory via `langgraph_step/node/path/triggers`) and what state they wrote (channel writes in `Checkpoint.channel_values` + `pending_writes`). It does not record what the plan was at any given step unless the user stores the plan in state. Even when the user does, comparing it to the actual trajectory is a manual exercise.

## Architectural Decisions

- **Pregel/BSP as the execution substrate** — `libs/langgraph/langgraph/pregel/main.py:455-477` (class docstring) describes the model as: Plan → Execution → Update per superstep, repeat until no actors selected or `recursion_limit` reached (`libs/langgraph/langgraph/pregel/main.py:2578-2579`). This is an actor-reactor pattern, not a planner/executor pattern. It is sound for deterministic graph execution but does not model LLM-generated plans.
- **User-defined state holds any "plan"** — the schema-less `StateGraph` state (`libs/langgraph/langgraph/graph/state.py:1801-1821`, `_get_channels`) is the only place a plan can live. The framework neither adds a `plan` slot nor constrains its shape.
- **`Command.goto` as the planner's tool** — `libs/langgraph/langgraph/types.py:758-808` lets a node pick the next node(s) (`str | Sequence[Send | N] | Send`). When processed (`libs/langgraph/langgraph/pregel/_io.py:56-78`), `goto` becomes writes to the `TASKS` channel or `branch:to:<name>` channels (`libs/langgraph/langgraph/_internal/_constants.py:20,82-83`). This is the only "planner primitive" the framework exposes.
- **No separate planner process** — there is no manager agent, no orchestrator class, no Magentic-style pattern. The orchestrator and the actors all live in the same BSP loop. Compare to Microsoft Agent Framework's Magentic (`MagenticOrchestrator` in `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:859-876`), which the sibling report (03.04 for `agent-framework`) rates 9/10 because of explicit plan/progress ledgers — LangGraph has none of that surface area.
- **Error handlers are graph-level fallbacks, not replanning** — `libs/langgraph/langgraph/graph/state.py:857-879` lets a node be paired with an `error_handler=` callback that becomes a sibling node with name `__error_handler__<node>`. `libs/langgraph/langgraph/pregel/_runner.py:147-174` reads `node_error_handler_map` to route failures. This is the only structured "reroute" the framework supports, and it does not touch any plan.

## Notable Patterns

- **BSP superstep loop** — `libs/langgraph/langgraph/pregel/main.py:2979-2999`: `while loop.tick(): runner.tick(...); loop.after_tick()`. Inside `tick`, `prepare_next_tasks` (`libs/langgraph/langgraph/pregel/_algo.py:392-513`) computes the next task set from `(checkpoint, pending_writes, processes, channels, ...)`. The framework's "plan" is literally `prepare_next_tasks`.
- **Trigger-based scheduling** — `libs/langgraph/langgraph/pregel/_algo.py:475-512`: if `updated_channels` is known and `trigger_to_nodes` is provided, candidate nodes are restricted to those triggered by the updated channels (an O(updated-channels) optimization). Otherwise all nodes are considered. This is the BSP planner in its purest form.
- **`Send` as fan-out / PUSH** — `libs/langgraph/langgraph/pregel/_algo.py:938-1107`: when a node writes a `Send` to the `TASKS` channel, the next superstep reads pending sends (`libs/langgraph/langgraph/pregel/_algo.py:442-466`) and prepares a PUSH task for each. This is how a planner node fans out to parallel workers without explicit state plumbing.
- **`Command(goto)` as ad-hoc routing** — `libs/langgraph/langgraph/pregel/_io.py:56-78`: a node can return `Command(goto="next", update={"x": 1})` and the next superstep's `prepare_next_tasks` will include `next`. This is the natural planner-to-executor handoff in user code (`libs/langgraph/tests/test_pregel.py:5186-5212`).
- **Per-task scratchpad** — `libs/langgraph/langgraph/_internal/_scratchpad.py:1-19`: `step`, `stop`, `call_counter`, `interrupt_counter`, `resume`, `subgraph_counter`. Surfaces via `RemainingSteps` (`libs/langgraph/langgraph/managed/is_last_step.py:18-24`). This is the closest thing to a "plan execution budget" but it is mechanical (superstep counter), not semantic (plan progress).
- **Stateful `StateSnapshot.next`** — `libs/langgraph/langgraph/types.py:648-649`: `next: tuple[str, ...]` reports the node names that would run at this step. Useful for a user to inspect "what is about to happen", but again it is computed from trigger analysis, not from a plan.

## Tradeoffs

- **Pro: Flexibility.** Because planning is user-defined, LangGraph does not lock users into a planner schema. Power users can implement any planner strategy (plan-and-execute, ReWOO, LATS, Reflexion — all of which exist as `examples/` subdirectories or test cases).
- **Pro: No magic.** The execution model is straightforward BSP; tracing a run is mechanical (walk checkpoints, replay supersteps). The plan-vs-actual distinction collapses to "the plan was whatever your planner node wrote to state".
- **Pro: Testability.** `libs/langgraph/tests/test_pregel.py:5179-5234` shows that a planner can be tested as a normal graph — invoke, assert state. No planner infrastructure to mock.
- **Con: No plan-vs-actual accounting.** Without a built-in plan schema, you cannot ask LangGraph "did this run follow the plan?"; you have to compare `plan` channel history yourself.
- **Con: No replanning primitives.** When the world changes mid-run, the planner node must re-execute and re-emit a `Command(goto=...)`. There is no signal that "the plan is stale; revise it now" — the user code is responsible.
- **Con: Plan can drift silently.** If the planner returns `Command(goto="X")` but `X` raises and is silently routed to `__error_handler__X` (`libs/langgraph/langgraph/pregel/_runner.py:1549-1584`), the original planner state is unaware. The plan was for `X`; the actual execution was `__error_handler__X`. The framework records the routing but does not surface a "deviation" event.
- **Con: `interrupt_before/after` is the only built-in checkpoint.** A user building a planner must use `interrupt()` (dynamic, from inside a node) or `interrupt_before/after` (static, at compile time) to pause for human approval. Both are framework pause points, not plan-revision hooks.

## Failure Modes / Edge Cases

- **Planner node crashes mid-plan** — handled like any other node failure: `RetryPolicy` retries (`libs/langgraph/langgraph/types.py:416-435`), then `node_error_handler_map` routes to a fallback (`libs/langgraph/langgraph/pregel/_runner.py:147-174`). If neither is configured, the exception propagates and the run fails. The remaining plan in state is orphaned.
- **`Command(goto=...)` to a non-existent node** — `libs/langgraph/langgraph/pregel/_algo.py:977-979` (`prepare_push_task_send`): "Ignoring unknown node name ... in pending sends" — silently dropped. The plan looks valid but a step is skipped without any error event.
- **`Send` to a node with no path in the graph** — same silent drop (`libs/langgraph/langgraph/pregel/_algo.py:972-975`). No exception, no event.
- **Planner returns `Command(goto=...)` referencing a deleted/stale plan entry** — the framework will route to whatever the planner said; there is no validation that `goto` is "still in the plan".
- **Plan survives `interrupt()` but the planner node itself was the interrupt** — the interrupt writes `INTERRUPT` to `checkpoint_pending_writes` (`libs/langgraph/langgraph/pregel/_loop.py:1393-1437`), and on resume the next superstep reruns the planner (because `task.writes` was filtered to skip `INTERRUPT/ERROR/RESUME` in `_reapply_writes_to_succeeded_nodes`, `libs/langgraph/langgraph/pregel/_loop.py:724-737`). This works but only because the planner is just a node.
- **`recursion_limit` reached before plan completes** — `libs/langgraph/langgraph/pregel/main.py:2578-2579,3017-3022,3501-3503` raises `GraphRecursionError`. The plan in state is partially executed; no automatic continuation.
- **Checkpoint durability** — `durability ∈ {"sync", "async", "exit"}` (`libs/langgraph/langgraph/pregel/_loop.py:301,313`); the plan-in-state is durable only to the extent the user-configured checkpointer is durable. DeltaChannel captures plans-as-deltas for replay (`libs/langgraph/langgraph/pregel/_loop.py:696-700,996-1003`) but only if the `plan` channel is annotated as `DeltaChannel`.

## Future Considerations

- **Magentic-style plan ledger is absent** — Microsoft Agent Framework's Magentic orchestrator emits `MagenticPlanCreatedEvent` / `MagenticReplannedEvent` and persists `MagenticTaskLedger`. LangGraph has no equivalent; users wanting that have to roll their own (and the `examples/plan-and-execute/` notebook was retired to docs).
- **No plan-validation step** — there is no built-in way to assert "the next step is in the plan" or "the plan was followed". A user could write a guard node, but the framework does not provide one.
- **No plan-replay primitive** — `ReplayState` (`libs/langgraph/langgraph/_internal/_replay.py`) supports time-travel to a checkpoint, but you cannot say "replay from checkpoint X but with the plan that was current at Y".
- **Prebuilt agents do not ship planners** — `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py` (create_react_agent / create_agent) and `tool_node.py` (ToolNode) are ReAct-style; `grep -n "plan\|planner\|todo" libs/prebuilt/` returns 0 hits. The prebuilt layer does not fill the gap.
- **Future additions likely via Deep Agents** — `README.md:31` points users to Deep Agents ("a higher-level package built on LangGraph for agents that can plan, use subagents, and leverage file systems for complex tasks"). Planning is delegated to an external higher-level package, not absorbed into LangGraph itself.

## Questions / Gaps

- Is there an undocumented plan feature in the TypeScript sibling (`libs/sdk-js/`) or the LangGraph Platform server (CLI)? Not investigated — outside the source boundary.
- Are there `examples/llm-compiler/` or `examples/reflexion/` patterns that effectively implement a planner? Those directories exist (`examples/llm-compiler/`, `examples/reflexion/`, `examples/lats/`, `examples/rewoo/`) but their planner implementations are user code, not framework features. Not deep-dived here.
- Does the LangGraph Platform / Studio tooling surface a "plan" view? No evidence found in this source — those live in a separate server repo.
- Is `RemainingSteps` ever used as a "plan budget" semantic (vs. superstep budget)? Search shows it is only ever derived from `scratchpad.stop - scratchpad.step` (`libs/langgraph/langgraph/managed/is_last_step.py:18-22`). No user-facing documentation inside the source treats it as a plan-progress counter.

---

Generated by `reports/source/03.04-planner-executor-separation` against `langgraph`.
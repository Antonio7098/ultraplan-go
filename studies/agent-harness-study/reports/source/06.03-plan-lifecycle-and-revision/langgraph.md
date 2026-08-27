# Source Analysis: langgraph

## 06.03 Plan Lifecycle and Revision

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (Pregel engine `libs/langgraph`, `libs/prebuilt`, `libs/checkpoint`) |
| Analyzed | 2026-08-27 |

## Summary

LangGraph has **no first-class `Plan` object**. The "plan" is a composite of three layers: (1) **static topology** — `StateGraph` compiled to `Pregel` (`libs/langgraph/langgraph/graph/state.py:1231`, `libs/langgraph/langgraph/pregel/main.py:758`); (2) **dynamic schedule** — per-superstep task set computed by `prepare_next_tasks` from `channel_versions`/`versions_seen`/`trigger_to_nodes` (`libs/langgraph/langgraph/pregel/_algo.py:392`, `libs/langgraph/langgraph/pregel/_loop.py:599`); (3) **user-state plan** — any todo/plan field stored as a channel (e.g., `messages`, custom `plan: list`) mutated via reducers. Lifecycle is therefore **BSP-superstep lifecycle**, not plan-object lifecycle: creation = graph compile + first checkpoint (`source: input`), update = `Command(update, goto, Send)` or `bulk_update_state` (`libs/langgraph/langgraph/pregel/main.py:1590`, `libs/langgraph/langgraph/types.py:799`), invalidation = version bump + `apply_writes` (`libs/langgraph/langgraph/pregel/_algo.py:232`), completion = `tick()` returns `False` when no tasks schedulable → `status="done"` (`libs/langgraph/langgraph/pregel/_loop.py:652`), optionally gated by `RemainingSteps`/`IsLastStep` or model-judged `END` routing (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:831`, `libs/langgraph/langgraph/managed/is_last_step.py:18`). Revisions are **checkpoint revisions**, not plan revisions: every superstep optionally persists `Checkpoint(id=uuid6(step), channel_values, channel_versions, versions_seen)` with `CheckpointMetadata{source, step, parents, counters_since_delta_snapshot}` (`libs/langgraph/langgraph/pregel/_checkpoint.py:149`, `libs/checkpoint/langgraph/checkpoint/base/__init__.py:38`), retrievable via `get_state`/`get_state_history`/`list` (`libs/langgraph/langgraph/pregel/main.py:1392`, `1480`). No justification field, no diff/drift API, no plan-specific validators exist. The legacy `plan-and-execute` has been redirected (`docs/redirects.json:190`) and delegated to external `deepagents` (`README.md:31`), confirming plan lifecycle is intentionally user-owned.

## Rating

**Score: 5 / 10**

**Rationale:** Mechanical lifecycle (creation → scheduling → checkpoint → completion) is explicit, typed, tested, and — when a checkpointer is configured — durable and time-travelable via `uuid6`-ordered checkpoints and ancestor walks (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:854`, `libs/langgraph/langgraph/pregel/_checkpoint.py:229`). This exceeds ad-hoc. However *plan*-level lifecycle is inconsistent/fragile: no typed plan schema, topology is immutable post-compile, updates carry no `reason`/`justification`, revision history is checkpoint-generic (no plan diff, no `revision_id`, overwritten `state.plan` string), abandonment is implicit via `END`/state-clear, and drift detection is absent (no validator, no numeric threshold). Maps to rubric "Present but inconsistent, weakly documented, or fragile" — strong engine safeguards, weak plan semantics.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Pregel Plan phase definition | Docstring: three phases `Plan → Execution → Update` — Plan selects actors subscribing to updated channels | `libs/langgraph/langgraph/pregel/main.py:464-474` |
| Scheduler — plan creation per superstep | `prepare_next_tasks(checkpoint, pending_writes, processes, channels, managed, config, step, stop, ...)` merges PUSH (`TASKS` Topic `Send`) + PULL (channel-triggered) into `dict[id, task]` | `libs/langgraph/langgraph/pregel/_algo.py:392-513` |
| Trigger predicate — replanning gate | `_triggers()` compares `channel_versions[chan] > seen[chan]` or `chan.is_available()` on first superstep | `libs/langgraph/langgraph/pregel/_algo.py:1260-1277` |
| Tick — plan → exec → update orchestrator | `PregelLoop.tick()` prepares tasks, checks `should_interrupt(before)`, emits `checkpoints`/`tasks` debug payloads | `libs/langgraph/langgraph/pregel/_loop.py:599-682` |
| After-tick — apply writes & checkpoint | `after_tick()` calls `apply_writes()`, emits `values` if output channel updated, captures delta writes, clears pending, `._put_checkpoint({"source":"loop"})`, checks `should_interrupt(after)` | `libs/langgraph/langgraph/pregel/_loop.py:683-726` |
| Write application — deterministic plan execution | `apply_writes()` sorts tasks by `task_path_str(path[:3])`, bumps `versions_seen`, consumes triggers, groups writes, bumps `channel_versions` | `libs/langgraph/langgraph/pregel/_algo.py:232-345` |
| Static plan surface — StateGraph DSL | `StateGraph.add_node/add_edge/add_conditional_edges/add_sequence/validate/compile` → validates and builds `trigger_to_nodes` | `libs/langgraph/langgraph/graph/state.py:667-1030`, `1131-1390` |
| Compiled plan artifact | `Pregel.__init__` builds `channels[TASKS]=Topic(Send)` (reserved), stores `nodes`, `trigger_to_nodes`, `interrupt_before/after` | `libs/langgraph/langgraph/pregel/main.py:758-809` |
| Dynamic plan mutation — Send | `class Send(node, arg, timeout)` hashable fan-out primitive; documented map-reduce example | `libs/langgraph/langgraph/types.py:704-792` |
| Dynamic plan mutation — Command | `class Command(graph, update, resume, goto=Send|Sequence[Send|N]|N)` with `_update_as_tuples()` and `update` patching | `libs/langgraph/langgraph/types.py:799-849` |
| Command write mapping | `map_command(cmd)` yields `(NULL_TASK_ID, TASKS, send)` and `(NULL_TASK_ID, RESUME, ...)` tuples for loop ingestion | `libs/langgraph/langgraph/pregel/_io.py:42-61` |
| Loop ingestion of Command/Send | `_first()` maps `Command` to writes via `put_writes(NULL_TASK_ID,...)`; handles `resume_map` for multi-interrupt | `libs/langgraph/langgraph/pregel/_loop.py:848-932` |
| Manual plan revision — update_state | `Pregel.bulk_update_state(config, [StateUpdate(values, as_node, task_id)])` → `create_checkpoint_plan_for_update_state_api` → `create_checkpoint` + `put` | `libs/langgraph/langgraph/pregel/main.py:1590-1809` |
| Manual plan revision convenience | `Pregel.update_state(config, values, as_node)` wraps `bulk_update_state` with single `StateUpdate` | `libs/langgraph/langgraph/pregel/main.py:2515-2520` |
| Revision plan helper | `create_checkpoint_plan_for_update_state_api(channels, updated_channels, step, parents, saved_metadata, is_fresh_thread)` computes `channels_to_snapshot` and `metadata{source:"update", step, parents}` | `libs/langgraph/langgraph/pregel/_checkpoint.py:117-146` |
| Revision durability — checkpoint build | `create_checkpoint(prev, channels, step, id=uuid6(step), updated_channels, channels_to_snapshot)` snapshots `channel_values`/`channel_versions`/`versions_seen` | `libs/langgraph/langgraph/pregel/_checkpoint.py:149-214` |
| Metadata schema — revision attribution | `CheckpointMetadata.source: Literal["input","loop","update","fork"]`, `step:int`, `parents: dict`, `counters_since_delta_snapshot` | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:38-86` |
| History retrieval — point query | `Pregel.get_state(config, subgraphs=False)` → validates checkpointer, resolves `recast_checkpoint_ns`, hydrates via `channels_from_checkpoint`, calls `_prepare_state_snapshot` | `libs/langgraph/langgraph/pregel/main.py:1392-1434` |
| History retrieval — range query | `Pregel.get_state_history(config, filter, before, limit)` iterates `checkpointer.list` | `libs/langgraph/langgraph/pregel/main.py:1480-1520` |
| Checkpoint ordering guarantee | `Checkpoint.id = str(uuid6(clock_seq=step))` — monotonic, sortable first→last (`id` drives `ORDER BY`) | `libs/langgraph/langgraph/pregel/_checkpoint.py:209`, `libs/checkpoint/langgraph/checkpoint/base/__init__.py:854` |
| Old-plan preservation — parent chain | `get_delta_channel_history(config, channels)` walks `parent_config` chain accumulating `PendingWrite`s until `seed` `_DeltaSnapshot` found | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:582-649` |
| Delta snapshot cadence | `delta_channels_to_snapshot(channels, counters)` fires when `updates >= snapshot_frequency` or `supersteps >= 5000` | `libs/langgraph/langgraph/pregel/_checkpoint.py:50-71` |
| Completion — schedulability check | `if not self.tasks: self.status="done"; return False` — no tasks = plan satisfied | `libs/langgraph/langgraph/pregel/_loop.py:652-655` |
| Completion — recursion bound | `if self.step > self.stop: self.status="out_of_steps"; return False` with default `stop = start + recursion_limit` (`recursion_limit` default 25/10007) | `libs/langgraph/langgraph/pregel/_loop.py:606-609`, `libs/langgraph/langgraph/_internal/_config.py:32` |
| Completion — error raise | `GraphRecursionError("Recursion limit of N reached...")` instead of success | `libs/langgraph/langgraph/pregel/main.py:3002-3011` |
| Prebuilt completion — model-judged END | `should_continue(state)` returns `END`/`post_model_hook`/`generate_structured_response` when `last AIMessage.tool_calls` empty, else `Send("tools", ToolCallWithContext)` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:831-859` |
| Budget awareness — managed values | `RemainingSteps = stop - step`, `IsLastStep` injected via `ManagedValueSpec` to allow graceful degradation near limit | `libs/langgraph/langgraph/managed/is_last_step.py:18-21` |
| Time-travel fork | `is_time_traveling = is_replaying and not resuming` → clears `RESUME` writes, forks checkpoint `source:"fork"` | `libs/langgraph/langgraph/pregel/_loop.py:874-972`, `libs/langgraph/langgraph/pregel/main.py:1819`, `2282` |
| Plan invalidation — update_state fork | `_first` on `update_state` path calls `_put_checkpoint({"source":"update"})` and sets `versions_seen[INTERRUPT]` to current versions so pending interrupts are not re-fired incorrectly | `libs/langgraph/langgraph/pregel/_loop.py:946-1020` |
| Interrupts as plan blockers | `should_interrupt(checkpoint, interrupt_nodes, tasks)` checks `any_updates_since_prev_interrupt` + node in list | `libs/langgraph/langgraph/pregel/_algo.py:155-185` |
| Interrupt persistence | `INTERRUPT`/`RESUME` are reserved write keys persisted via `put_writes` and surfaced in `StateSnapshot.interrupts` | `libs/langgraph/langgraph/_internal/_constants.py:9-12`, `libs/langgraph/langgraph/types.py:700` |
| Explicit plan absence — no Planner | `grep -r "class.*Plan" libs/langgraph` yields only `create_checkpoint_plan_for_update_state_api` (checkpoint write planning, not LLM task planning) | `libs/langgraph/langgraph/pregel/_checkpoint.py:117` |
| Externalized planning | `redirects.json` rewrites `/tutorials/plan-and-execute/plan-and-execute` → `langchain/middleware/built-in#to-do-list`; `README.md` recommends `Deep Agents` for planning | `docs/redirects.json:190`, `README.md:31` |
| No drift validator | `grep -R "drift\|validator\|replan\|revision" libs/langgraph/langgraph/pregel` yields no plan-drift or validator primitives | `No evidence found` |
| Async parity for lifecycle | `AsyncPregelLoop`, `abulk_update_state`, `aget_state`, `achannels_from_checkpoint` mirror sync paths | `libs/langgraph/langgraph/pregel/_loop.py:140`, `libs/langgraph/langgraph/pregel/main.py:1731`, `libs/langgraph/langgraph/pregel/_checkpoint.py:280` |

## Answers to Dimension Questions

### 1. Can plans change?
**Yes — at three granularities, but topology itself is immutable without recompile.**

* **State-level plan** (the user-modelled todo/plan field) is freely mutable: any node can `return {"plan": new_plan}` or `{"messages": [...]}` via channel reducers (e.g., `add_messages` for `messages`, `operator.add` for lists, `BinaryOperatorAggregate` with `Overwrite` to bypass reducer `libs/langgraph/langgraph/types.py:978`). The Pregel loop applies these via `apply_writes` (`libs/langgraph/langgraph/pregel/_algo.py:315`) and bumps `channel_versions` so dependents refire.
* **Dynamic control flow** mutates the *schedule*: returning `Command(update={"plan": ...}, goto="planner")` or `Command(goto=[Send("worker", arg), ...])` (`libs/langgraph/langgraph/types.py:798`) injects `(TASKS, Send)` writes that `prepare_next_tasks` consumes as `PUSH` tasks next superstep (`libs/langgraph/langgraph/pregel/_algo.py:441`).
* **External mutation** via `bulk_update_state`/`update_state` lets an operator patch `channel_values` mid-thread with optional `as_node` attribution (`libs/langgraph/langgraph/pregel/main.py:1590`, `libs/langgraph/langgraph/pregel/_checkpoint.py:117`). This is the primary replanning API for HITL editing.
* **What cannot change:** `Pregel.nodes`, `trigger_to_nodes`, channel specs. Compiled graph validates once (`libs/langgraph/langgraph/pregel/main.py:933`); adding/removing nodes requires new `StateGraph.compile()` and new thread.

### 2. Are changes justified?
**No — no required `reason`/`justification` field exists in any mutation path.**

* `StateUpdate = NamedTuple(values, as_node, task_id)` (`libs/langgraph/langgraph/types.py:631`) carries only new values and optional write-origin, not a rationale. `bulk_update_state` doc and impl never log a reason (`libs/langgraph/langgraph/pregel/main.py:1590`).
* `Command(update, goto, resume)` fields (`libs/langgraph/langgraph/types.py:821`) similarly carry no `reason`; `map_command` (`libs/langgraph/langgraph/pregel/_io.py:42`) yields bare `(channel, value)` tuples.
* `CheckpointMetadata` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:38`) records `source`/`step`/`parents`/`counters_since_delta_snapshot` but no `author`, `reason`, or semantic diff. Stream `TasksStreamPart`/`CheckpointPayload` (`libs/langgraph/langgraph/types.py:144`, `206`) carry `id`/`name`/`error`/`result` but not plan-change justification.
* Only indirect justification surfaces via `messages` history or `ToolMessage` tool outputs, if the user prompts the LLM to explain its `goto` decision — framework does not enforce or persist it.

### 3. Is old plan history preserved?
**Conditionally — checkpoint history is preserved durably when a checkpointer is configured; otherwise not. It is generic checkpoint history, not typed plan revision history.**

* **What is preserved:** Every superstep with `durability="sync"`/`"async"` calls `_put_checkpoint` (`libs/langgraph/langgraph/pregel/_loop.py:111-1219`) which `put`s `Checkpoint` + `CheckpointMetadata` via the `BaseCheckpointSaver` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:277`). IDs are monotonic `uuid6(step)` (`libs/langgraph/langgraph/pregel/_checkpoint.py:209`), so `list`/`get_state_history` returns ordered lineage (`libs/langgraph/langgraph/pregel/main.py:1480`). `parent_config` chain plus `pending_writes` enables point-in-time replay (`libs/langgraph/langgraph/pregel/_checkpoint.py:229`, `libs/checkpoint/langgraph/checkpoint/base/__init__.py:582`).
* **What is not preserved as plan history:** There is no `PlanRevision{version, parent_version, diff, reason, timestamp}` log. Overwriting a `plan` channel with a new list replaces the prior snapshot; the prior value survives only as an ancestor `Checkpoint.channel_values["plan"]` reachable by ancestor walk, with no diff packaging. `state.plan` string overwritten on replanning in examples is not version-stamped.
* **When history is absent:** Without `checkpointer` (`checkpointer=None`), `_put_checkpoint` is no-op (`libs/langgraph/langgraph/pregel/_loop.py:1133`), `get_state` raises `ValueError("No checkpointer set")` (`libs/langgraph/langgraph/pregel/main.py:1402`), and revision history is lost. `durability="exit"` defers writes until exit and stages via `_exit_delta_writes` (`libs/langgraph/langgraph/pregel/_loop.py:215`, `1221`), so intermediate plan revisions are not queryable until termination.
* **Coarse retention:** `prune`/`delete_thread` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:374`) and `DeltaChannel` ancestry caveats (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:378`) show history can be purged; some `DeltaChannel` walks silently reconstruct as empty if ancestors deleted, with no error.

### 4. Can the agent abandon a plan?
**Yes — implicitly via routing or state patching; no explicit `abandon_plan` primitive.**

* **Graph-level abandonment:** Any node or conditional edge can return `END` (`libs/langgraph/langgraph/constants.py:28`) or `Command(goto=END)` (`libs/langgraph/langgraph/types.py:824`). `should_continue` in prebuilt (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:841`) returns `END` when no tool calls remain, terminating the loop even with pending logical todos.
* **State-level abandonment:** Patching the plan channel to empty/cleared via `update_state({"plan": []}, as_node="planner")` (`libs/langgraph/langgraph/pregel/main.py:2515`) or returning `{"plan": Overwrite([])}` effectively abandons remaining steps; no framework abort signal is required because unscheduled nodes simply stop triggering (`libs/langgraph/langgraph/pregel/_algo.py:605`).
* **HITL abandonment:** An operator can fork a new thread via `copy_thread` or abandon by `delete_thread` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:320`, `350`).
* **What is missing:** No `PlanStatus=abandoned` enum, no `plan_abandoned` event on the lifecycle stream, no cancellation token for in-flight `PUSH` `Send` tasks (once queued in `TASKS` they run next superstep and cannot be retracted `libs/langgraph/langgraph/pregel/_algo.py:977` only warns on unknown node, not cancel).

### 5. Can plan drift be detected?
**No — no drift metric, threshold, or validator exists.**

* **Search boundary:** Grep across `libs/langgraph`, `libs/checkpoint`, `libs/prebuilt` for `drift`, `validator`, `replan_reason`, `stale`, `diverge` yields only false positives (`create_checkpoint_plan_*`). No `PlanValidator`, `DriftDetector`, or `remaining_plan_still_valid` analogue (CrewAI has `StepObservation.remaining_plan_still_valid`; langgraph has no equivalent).
* **Closest proxies:** `RemainingSteps`/`IsLastStep` (`libs/langgraph/langgraph/managed/is_last_step.py:18`) expose budget pressure, not semantic drift. `stream_mode="debug"`/`"tasks"`/`"checkpoints"` (`libs/langgraph/langgraph/types.py:122`) expose every task input/output/error, so a human or external evaluator could *manually* compare `channel_values["plan"]` across checkpoints retrieved via `get_state_history`, but framework does not compute diff.
* **No automatic trigger:** There is no stall-counter, no `is_in_loop`/`is_progress_being_made` ledger, and no periodic `should_replan` predicate. Conditional edges decide next node locally (`libs/langgraph/langgraph/graph/state.py:982`) but have no view of global plan coherence. Multi-step LLM planning (plan-and-execute) was externalized to `deepagents` (`README.md:31`) and its drift handling, if any, lives outside this repo.

## Architectural Decisions

| Decision | Description | Tradeoff |
|----------|-------------|----------|
| **Pregel BSP as plan engine** — single `tick() → apply_writes → checkpoint` loop with versioned `channel_versions`/`versions_seen` (`libs/langgraph/langgraph/pregel/main.py:464`, `libs/langgraph/langgraph/pregel/_loop.py:599`) | Deterministic replay, durable per-superstep, trivial completion = empty task set | No explicit plan object; lifecycle is step lifecycle; long-horizon plans must be encoded as cycles + state |
| **StateGraph compiles to Pregel with `trigger_to_nodes` index** — `add_node(node, retry_policy, cache_policy, timeout)` + `add_edge`/`add_conditional_edges` compile to `Pregel(nodes, channels, trigger_to_nodes)` (`libs/langgraph/langgraph/graph/state.py:131`, `libs/langgraph/langgraph/pregel/main.py:933`) | Compile-time validation, O(|updated_channels|) scheduling fast path (`libs/langgraph/langgraph/pregel/_algo.py:475`) | Plan topology frozen post-compile; cannot invent new nodes at runtime beyond `Send` fan-out |
| **`Send` + `Command` as dual mutation vectors** — `Send(node, arg)` via `TASKS` Topic and `Command(update, goto, resume)` via `map_command` (`libs/langgraph/langgraph/types.py:704`, `799`, `libs/langgraph/langgraph/pregel/_io.py:42`) | Flexible data-parallel and imperative control without graph mutation | Two overlapping APIs; static `ends` inference for `Command` is advisory (`libs/langgraph/langgraph/graph/state.py:838`), runtime can still send arbitrary `goto` with only a warning |
| **Checkpoint as plan revision store** — `Checkpoint` + `CheckpointMetadata{source, step, parents, counters}` persisted via `BaseCheckpointSaver` (`libs/langgraph/langgraph/pregel/_checkpoint.py:149`, `libs/checkpoint/langgraph/checkpoint/base/__init__.py:38`) | Full history, monotonic `uuid6(step)` ordering, ancestor walks for `DeltaChannel`, time-travel forks | History is checkpoint-generic, not plan-diff-aware; query requires manual diff; `DeltaChannel` walks break silently if ancestors pruned |
| **Externalize high-level planning** — redirect `plan-and-execute` (`docs/redirects.json:190`), recommend `deepagents` (`README.md:31`), keep core minimal | Keeps core dependency-light and unopinionated | Dimension gap: no planner tests, no reuse story, no safeguard for decomposition |
| **Managed `RemainingSteps`/`IsLastStep`** (`libs/langgraph/langgraph/managed/is_last_step.py:9`) | Gives nodes budget awareness instead of discovering limit via `GraphRecursionError` | Advisory only; prebuilt `_are_more_steps_needed` returns synthetic message rather than error (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:684`) |

## Notable Patterns

* **Channel-triggered planning:** Nodes declare `channels` + `triggers` (`libs/langgraph/langgraph/pregel/_read.py:97`), `PregelNode` metadata drives version comparison (`libs/langgraph/langgraph/pregel/_algo.py:1260`), cleanly separating data plane (channel values) from control plane (versions).
* **Deterministic ordering:** `apply_writes` sorts by `task_path_str(path[:3])` (`libs/langgraph/langgraph/pregel/_algo.py:256`); trigger candidates sorted alphabetically (`libs/langgraph/langgraph/pregel/_algo.py:482`) — ensures replay-identical plan execution without priority knob.
* **Scratchpad-scoped resume:** `_scratchpad()` (`libs/langgraph/langgraph/pregel/_algo.py:1280`) injects `CONFIG_KEY_RESUME_MAP`/`get_null_resume` so interrupts/resume participate in planning without mutating graph.
* **Exit-mode delta accumulator:** `_exit_delta_writes: list[tuple[step, task_id, channel, value]]` (`libs/langgraph/langgraph/pregel/_loop.py:215`, `708`) stages delta writes until exit, closing visibility gap for `durability="exit"` threads.
* **Runnables coercion for planning primitives:** `coerce_to_runnable(action, trace=False)` in `add_node` (`libs/langgraph/langgraph/graph/state.py:883`) and `BranchSpec.from_path` (`libs/langgraph/langgraph/graph/state.py:1019`) make any callable a planning primitive without planner interface.
* **`Topic(TASKS, accumulate=False)` isolation:** Dedicated `TASKS` channel (`libs/langgraph/langgraph/pregel/main.py:809`) isolates `Send` queue so user state channels remain unaffected by plan dispatch.

## Tradeoffs

* **Explicit topology vs emergent planning:** Developer gets reproducibility and checkpoint fidelity; LLM cannot author new graph nodes at runtime. Good for governance, poor for open-ended research needing dynamic decomposition.
* **Durability vs flexibility:** Versioned channels + checks make plan revisions crash-recoverable and forkable (`libs/langgraph/langgraph/pregel/_loop.py:956`), but every plan signal must be serializable via `JsonPlusSerializer`/`StrictMsgPack` (`libs/langgraph/langgraph/_internal/_serde.py`) and belongs to a declared channel.
* **Minimal core vs batteries-included planning:** Core stays small, but users re-implement similar ReAct/planner loops; planning reuse is limited to subgraph embedding (`get_subgraphs` at `libs/langgraph/langgraph/pregel/main.py:1076`) rather than a `Plan` artifact.
* **Sync/async parity tax:** `PregelLoop`/`AsyncPregelLoop`, sync/async `bulk_update_state`/`get_state`, duplicate `channels_from_checkpoint`/`achannels_from_checkpoint` (`libs/langgraph/langgraph/pregel/_checkpoint.py:229`, `280`) double maintenance surface (`_V3_INVARIANT_KWARGS` at `libs/langgraph/langgraph/pregel/main.py:384`).
* **Performance optimization via fast-path:** `updated_channels ∩ trigger_to_nodes` avoids scanning all nodes (`libs/langgraph/langgraph/pregel/_algo.py:475`), but pathological fan-in graphs fall back to linear scan.

## Failure Modes / Edge Cases

| Mode | Symptom | Evidence |
|------|---------|----------|
| **Silent Send drop** | `Send(node="unknown", arg=...)` logs warning `Ignoring unknown node ...` and returns `None`, proceeding without error — plan step lost | `libs/langgraph/langgraph/pregel/_algo.py:977-978` |
| **Orphan trigger** | Node whose `triggers` channel never written never schedules; `prepare_next_tasks` returns no `PULL` task with no warning — plan stalls silently | `libs/langgraph/langgraph/pregel/_algo.py:605-612`; `StateGraph.validate` only checks `START` reachable (`libs/langgraph/langgraph/graph/state.py:1141`) |
| **Recursion limit masquerading as plan end** | `RemainingSteps < 2` with pending `tool_calls` returns synthetic `AIMessage("Sorry, need more steps...")` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:684`) instead of `GraphRecursionError`, silently capping plan length | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:620-692` |
| **Update_state wipes pending interrupts** | `is_time_traveling` branch drops `RESUME` writes and forks checkpoint to preserve plan, but if `counters_since_delta_snapshot` or `_delta_channels_with_overwrite` missed, `DeltaChannel` state drifts post-fork | `libs/langgraph/langgraph/pregel/_loop.py:897-972`, `libs/langgraph/langgraph/pregel/_algo.py:33` |
| **UntrackedValue loss in plan dispatch** | `TASKS` `Send` packets containing `UntrackedValue` channels are sanitized (`sanitize_untracked_values_in_send`) dropping planning signals silently | `libs/langgraph/langgraph/pregel/_algo.py:109ff`, `libs/langgraph/langgraph/pregel/_loop.py:439-452` |
| **No justification → unauditable revision** | `bulk_update_state` accepts arbitrary `dict` without `reason`; subsequent `CheckpointMetadata` shows only `source:"update"` — "why did plan change?" unanswerable from checkpoint alone | `libs/langgraph/langgraph/pregel/main.py:1590`, `libs/checkpoint/langgraph/checkpoint/base/__init__.py:38` |
| **Drift invisible** | No `remaining_plan_still_valid` or drift score; a series of small description tweaks via `Command(update=...)` is indistinguishable from no drift | `No evidence found` |
| **Multiple writes to same channel race** | Later writes overwrite unless `BinaryOperatorAggregate`/`Topic(accumulate=True)`; concurrent `Send` tool calls writing same key resolve via `task_path_str` sort, not plan priority | `libs/langgraph/langgraph/pregel/_algo.py:253-256`, `315-324` |
| **Durability=exit hides intermediate plan** | Intermediate `._put_checkpoint` is skipped (`do_checkpoint = durability != "exit"`) so plan revisions during run are not queryable until exit; crash before exit loses plan entirely | `libs/langgraph/langgraph/pregel/_loop.py:1133-1134` |
| **Pruning corrupts delta plan** | `prune(keep_latest)` that drops ancestors between head and `_DeltaSnapshot` makes `get_delta_channel_history` reconstruct as empty with no error — plan channels silently empty | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:378-414` |

## Future Considerations

* **First-class `Plan` channel/type** — introduce `Plan` TypedDict/Pydantic with `steps: list[Step{id, description, status, depends_on}]` stored as `Annotated[list, add_plan]` or `DeltaChannel` so plan becomes diffable, checkpoint-replayable payload rather than ad-hoc `messages` history; leverage existing `DeltaChannel` `snapshot_frequency` for sparse snapshots.
* **Typed `plan_update` reducer with reason** — extend `StateUpdate`/`Command` to require `reason: str` (or `author: Literal[user, model, operator]`) and persist it in `CheckpointMetadata` (new `plan_reason` key) so "why did the plan change?" becomes queryable without scraping transcript; mirror `TodoCompleteInput.reason` pattern from other frameworks.
* **Plan-aware interrupts** — type `Interrupt[PlanStep]` with per-step resume map keyed by stable `id` (`_xxhash_str` at `libs/langgraph/langgraph/pregel/_algo.py:1404`) so HITL can approve/edit single todo rather than whole step.
* **Observable `plan` stream mode** — add `StreamMode="plan"` via `StreamTransformer` pattern (`libs/langgraph/langgraph/stream/transformers.py`) emitting `PlanDiffPayload{added, removed, changed}` each superstep, making decomposition visible without reading full checkpoint.
* **Drift validator hook** — pluggable `plan_validator(state, checkpoint_history) -> DriftReport{score, stale_steps, missing_deps}` invoked before `tick()` or via `pre_model_hook`; threshold auto-triggers `Command(goto="replan")` or `update_state` remediation, analogous to `RemainingSteps` budget guard.

## Questions / Gaps

* No planner prompt, planner agent, or `Plan` node exists in `libs/langgraph` or `libs/prebuilt` beyond ReAct routing — `grep "planner" libs/langgraph` yields only test mocks (`libs/langgraph/tests/test_pregel.py:5171`, `...test_pregel_async.py:6426`).
* `examples/plan-and-execute/plan-and-execute.ipynb` is a redirect stub — prior durability/failure handling for decomposition cannot be verified.
* No dedicated planning-revision tests: `libs/langgraph/tests/test_pregel.py` verifies orchestration (edges, `Send`, `remaining_steps`) but not decomposition quality, replanning after failure, or justification persistence.
* Visibility of dynamic plan via `get_state(subgraphs=True)` and interrupt/resume (`libs/langgraph/langgraph/pregel/_loop.py:1375`) is documented for HITL but not evaluated for long-horizon plan editing workloads.
* Reusability limited to subgraph embedding; no evidence of cross-graph plan templates or serialized `Plan` JSON surviving `libs/prebuilt` version upgrades (`AgentState` deprecation notices at `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:53` suggest churn).
* Unclear whether `update_state` with `as_node` correctly propagates `versions_seen` for barrier `waiting_edges` — `_first` sets `versions_seen[INTERRUPT]` on resume but `bulk_update_state` path does not visibly bump dependent triggers.

---

Generated by `06.03-plan-lifecycle-and-revision` against `langgraph`.

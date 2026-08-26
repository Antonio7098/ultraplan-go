# Source Analysis: langgraph

## 06.05 — Objective and Progress Tracking

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (core: `libs/langgraph`, `libs/checkpoint`, `libs/prebuilt`; plus JS/TS REST SDKs) |
| Analyzed | 2026-08-26 |

All file paths below are relative to the source root `studies/agent-harness-study/sources/langgraph/`.

## Summary

LangGraph does not track *objectives* natively; it tracks *mechanical progress* with unusual rigor, and deliberately delegates goal semantics to the application layer. The goal representation is the declarative graph itself: a shared state (channels with reducers) plus a topology of nodes and conditional edges whose terminal state is structural — the loop ends when no next task is schedulable (`libs/langgraph/langgraph/pregel/_loop.py:652-655`) or a conditional edge returns `END`. Progress is measured in **supersteps**: a monotonic step counter drives execution (`libs/langgraph/langgraph/pregel/_loop.py:599-681`), is stamped into every checkpoint's metadata (`libs/langgraph/langgraph/pregel/_loop.py:1125`, `libs/checkpoint/langgraph/checkpoint/base/__init__.py:49-55`), and is surfaced to observers through seven typed stream modes (`values`, `updates`, `tasks`, `checkpoints`, `debug`, `messages`, `custom` — `libs/langgraph/langgraph/types.py:122-136`), the `get_state`/`get_state_history` query APIs (`libs/langgraph/langgraph/pregel/main.py:1392-1434`, `main.py:1480`), and managed values that expose remaining step budget to nodes (`libs/langgraph/langgraph/managed/is_last_step.py:18-21`). Completion is judged either structurally (no schedulable tasks) or by model-authored routing logic such as the prebuilt agent's `should_continue`, which stops when the last AI message has no tool calls (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:831-841`). There is no independent verification that reaching "done" means the goal was achieved; the sole operational safeguards are the recursion limit (`libs/langgraph/langgraph/pregel/main.py:3002-3011`, default 10007 at `libs/langgraph/langgraph/_internal/_config.py:32`) and opt-in human gating via `interrupt()` (`libs/langgraph/langgraph/types.py:851-970`). Blockers (interrupts) and failures (errors) are recorded as first-class persisted writes and surfaced in snapshots and stream payloads.

**Answer to the dimension's framing question ("does the agent know the difference between activity and progress?"):** LangGraph mechanically distinguishes activity (task started → task finished, with results/errors) but has no native notion of *meaningful* progress toward an objective. Whether a superstep moved toward the goal is unknowable to the framework; it is encoded only in application state and model judgment.

## Rating

**7 / 10**

Rationale against the rubric:

- **Clear model + explicit interfaces (+):** The progress model is unambiguous — BSP supersteps, per-step checkpoints with `step`/`source` metadata (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:41-55`), typed stream payloads discriminated on `type` (`libs/langgraph/langgraph/types.py:223-261`), and queryable history (`get_state_history`, `libs/langgraph/langgraph/pregel/main.py:1480`). These are documented public interfaces, not incidental internals.
- **Tests (+):** Recursion-limit behavior is directly tested (`with pytest.raises(GraphRecursionError)` at `libs/langgraph/tests/test_pregel.py:588-589`); interrupt/resume semantics have dozens of dedicated tests (`test_pregel.py:3326`, `:4839`, `:4909`, `:7577`); debug-stream payload shaping is unit-tested (`libs/langgraph/tests/test_pregel_debug.py:27-175`).
- **Operational safeguards (+):** Enforced recursion bound validated at config time (`recursion_limit must be at least 1`, `libs/langgraph/langgraph/pregel/main.py:2563-2564`), durable checkpoint ordering (`_put_checkpoint_fut` chaining, `libs/langgraph/langgraph/pregel/_loop.py:1202-1209`), durability modes (`sync`/`async`/`exit`, `libs/langgraph/langgraph/_internal/_constants.py:68-69`).
- **Not 8+:** The framework cannot answer "are we closer to the goal?" — there is no goal object, no success criterion type, no milestone abstraction, and no independent success verification. Goal semantics exist only implicitly in user-defined conditional edges and model behavior, making objective tracking ad hoc at the framework level despite excellent progress mechanics.
- **Not 6 or lower:** What it does measure is durable (checkpoints survive crashes and enable resume/time-travel), observable (streams + snapshot API), and proven under failure (interrupt persistence and multi-interrupt resume tested extensively).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Loop termination criterion | `tick()` returns False when no tasks remain → `status = "done"`; also stops on `step > stop` → `"out_of_steps"` | `libs/langgraph/langgraph/pregel/_loop.py:606-609`, `libs/langgraph/langgraph/pregel/_loop.py:652-655` |
| Step counter as progress unit | `self.step += 1` per superstep; stop = start + `recursion_limit + 1` | `libs/langgraph/langgraph/pregel/_loop.py:1217-1219`, `libs/langgraph/langgraph/pregel/_loop.py:1701` |
| Checkpoint = milestone record | `_put_checkpoint({"source": "loop"})` after every applied-superstep; metadata carries `step` and `parents` | `libs/langgraph/langgraph/pregel/_loop.py:718`, `libs/langgraph/langgraph/pregel/_loop.py:1125-1126` |
| CheckpointMetadata schema | `source: Literal["input","loop","update","fork"]`, `step: int` (-1 input, 0 first loop), `parents` | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:38-86` |
| Monotonic checkpoint IDs encode order | `id=id or str(uuid6(clock_seq=step))` — IDs sortable first→last | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:854` |
| Stream modes (progress visibility) | `StreamMode = Literal["values","updates","checkpoints","tasks","debug","messages","custom"]` with per-mode semantics | `libs/langgraph/langgraph/types.py:122-136` |
| Task lifecycle events | `TaskPayload` (start: id/name/input/triggers), `TaskResultPayload` (error/interrupts/result) | `libs/langgraph/langgraph/types.py:144-179` |
| Checkpoint event shows what's next | `CheckpointPayload.next`: names of nodes scheduled to execute next; per-task `error`/`result`/`interrupts` | `libs/langgraph/langgraph/types.py:206-220`, `libs/langgraph/langgraph/pregel/debug.py:176-206` |
| Emission points for updates/values/tasks | `output_writes()` emits `updates` (per-task channel writes), `values` (state incl. interrupts), `tasks` result events | `libs/langgraph/langgraph/pregel/_loop.py:1416-1466` |
| External progress query | `get_state()` returns `StateSnapshot` (values, next nodes, tasks, interrupts); `get_state_history()` iterates milestones | `libs/langgraph/langgraph/pregel/main.py:1392-1434`, `libs/langgraph/langgraph/pregel/main.py:1480` |
| Model-judged completion (prebuilt agent) | `should_continue`: END when last AIMessage has no `tool_calls`, else route/Send to tools | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:831-859` |
| Application-defined completion edges | `add_conditional_edges(path)` — returning `'END'` stops execution | `libs/langgraph/langgraph/graph/state.py:982-1030` |
| Remaining-budget signal to nodes | `RemainingSteps = stop - step`, `IsLastStep` managed values injected into node state | `libs/langgraph/langgraph/managed/is_last_step.py:9-21` |
| Recursion-limit safeguard | `GraphRecursionError("Recursion limit of N reached without hitting a stop condition")`, error code `GRAPH_RECURSION_LIMIT`; default 10007 | `libs/langgraph/langgraph/pregel/main.py:3002-3011`, `libs/langgraph/langgraph/errors.py:35`, `libs/langgraph/langgraph/_internal/_config.py:32` |
| Blockers recorded (interrupts) | Reserved `INTERRUPT`/`RESUME` write keys persisted via pending writes; `StateSnapshot.interrupts`; multi-interrupt resume requires interrupt-id map | `libs/langgraph/langgraph/_internal/_constants.py:9-12`, `libs/langgraph/langgraph/types.py:700-701`, `libs/langgraph/langgraph/pregel/_loop.py:903-920` |
| Errors recorded per task | Reserved `ERROR` key; `map_debug_task_results` includes `error` field in task_result events | `libs/langgraph/langgraph/_internal/_constants.py:13-14`, `libs/langgraph/langgraph/pregel/debug.py:106-128` |
| Human approval gate | `interrupt(value)` raises `GraphInterrupt`, surfaces value to client, resumes via `Command(resume=...)`; requires checkpointer | `libs/langgraph/langgraph/types.py:851-969` |
| UI status surface | `push_ui_message` / `ui_message_reducer` maintain a `ui` state key streamed to clients | `libs/langgraph/langgraph/graph/ui.py:61-165` |
| Tests of intended behavior | Recursion error test; interrupt loop/multi-interrupt tests; debug-payload unit tests | `libs/langgraph/tests/test_pregel.py:588-589`, `libs/langgraph/tests/test_pregel.py:4909`, `libs/langgraph/tests/test_pregel.py:7577`, `libs/langgraph/tests/test_pregel_debug.py:27-175` |

## Answers to Dimension Questions

**1. What is the goal?**
There is no goal object anywhere in the core runtime (searched `libs/langgraph/langgraph/`, `libs/checkpoint/`, `libs/prebuilt/` for goal/objective/target abstractions — no evidence found). The goal exists implicitly as (a) the graph's terminal structure — `END` sentinel constant (`libs/langgraph/langgraph/constants.py:28-29`) reached via conditional edges, and (b) application state conventions (e.g., the prebuilt agent treats "model produced a message with no tool calls" as done, `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:834-841`). Goal semantics are therefore authored per-application in Python routing functions, not declared as data the runtime can inspect.

**2. How is progress measured?**
Mechanically and durably, in four layers: (i) a superstep counter bounding and indexing execution (`libs/langgraph/langgraph/pregel/_loop.py:606-609`, `1125`, `1217-1219`); (ii) per-step checkpoints recording `source`/`step`/`parents` metadata (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:41-60`) with monotonic UUIDv6 IDs ordered by step (`__init__.py:854`); (iii) typed stream events derived from actual task scheduling/completion — `values` emitted only when output channels were updated in that superstep (`_loop.py:700-707`), `updates` from real task writes (`_loop.py:1452-1459`), `tasks` start/result pairs (`debug.py:41-71`, `106-128`); (iv) pull-based observation via `get_state`/`get_state_history` (`pregel/main.py:1392`, `1480`). Notably, progress is measured by *task execution and channel writes*, not by tool success or test outcomes — a failed task records its error but does not advance state.

**3. Can the model fake progress?**
Partially. **Content-level: yes.** Nodes (typically LLM-driven code) can write arbitrary values into channels; reducers enforce merge mechanics, not truthfulness — nothing validates that a written `plan: [...all complete]` corresponds to reality (no schema-level assertions found in `channels/` reducers, e.g. `libs/langgraph/langgraph/channels/last_value.py`, `binop.py`). **Event-level: no.** Observable progress events are mechanically derived from the scheduler: an `updates` chunk requires a scheduled task actually writing channels (`output_writes`, `_loop.py:1416-1466`), and hidden internal work can be excluded via `TAG_HIDDEN` (`debug.py:44-45`) but cannot be *invented*. So an agent can mislabel its state as achieved, but cannot fabricate stream-visible execution that didn't happen.

**4. Are blockers recorded?**
Yes, as durable first-class data. Interrupts raised via `interrupt()` become reserved-key writes (`INTERRUPT`, `libs/langgraph/langgraph/_internal/_constants.py:9-10`) persisted through pending writes; they surface in `StateSnapshot.interrupts` ("Interrupts ... pending resolution", `types.py:700-701`), in `TaskResultPayload.interrupts` (`types.py:176-177`), and in checkpoint debug payloads (`debug.py:122-127`). Resumption is explicit and addressable: `Command(resume=...)` requires a checkpointer (`_loop.py:904-908`), and with multiple pending interrupts the caller *must* supply an interrupt-id→value map or the run fails with a RuntimeError pointing at docs (`_loop.py:916-920`). Node errors are likewise recorded under the reserved `ERROR` key with the failing node name (`_internal/_constants.py:13-17`) and reported in task-result events (`debug.py:118`).

**5. Is final success independently checked?**
No evidence of any independent success verification was found. Completion means one of: no schedulable tasks (`_loop.py:652-655`), a drain request (`_loop.py:657-659`), or exhaustion of the recursion budget, which is *raised as an exception* rather than treated as success (`pregel/main.py:3002-3011`). The closest mechanisms to independent checks are (a) human approval gates via `interrupt()` (opt-in, application-placed) and (b) static graph validation before compile-time (`pregel/_validate.py`). Neither verifies that the produced output satisfies the original objective; correctness at termination rests entirely on the application's routing logic and the model's calibration.

## Architectural Decisions

1. **BSP supersteps as the atomic unit of progress.** All tasks in a step run against immutable channel values; writes apply at the boundary (`after_tick`, `libs/langgraph/langgraph/pregel/_loop.py:683-714`). This makes "progress" well-defined and checkpointable at every step, at the cost of latency (barrier waits).
2. **Progress ledger externalized to pluggable storage.** The checkpointer interface (`put`, `put_writes`, `list`, `prune` — `libs/checkpoint/langgraph/checkpoint/base/__init__.py:277-389`) makes the milestone history a swappable backend (memory/postgres/sqlite/redis), separating progress accounting from execution.
3. **Observability as typed streams, not logging.** Seven discriminated stream modes with structured payloads (`types.py:122-261`) let UIs/tests/tracers subscribe to exactly the progress granularity they need; a beta v3 protocol consolidates this into projections on a `GraphRunStream` (`libs/langgraph/langgraph/stream/run_stream.py:36-65`).
4. **Goal-checking pushed to the edges (literally).** By making conditional edges ordinary callables over state (`graph/state.py:982-1030`), the framework avoids encoding any notion of success while enabling patterns like plan-and-execute or reflexion to be built entirely in user code.
5. **Budget-awareness injected via managed values.** `RemainingSteps`/`IsLastStep` (`managed/is_last_step.py:18-21`) let nodes degrade gracefully near the recursion ceiling instead of discovering it via exception.
6. **Blockers as state, not control flow side-channels.** Persisting interrupts/resumes/errors through the same write pipeline as normal state (`_internal/_constants.py:7-24`) means crash recovery replays blockers deterministically — the reason multi-interrupt resume works across processes (`tests/test_pregel.py:5710`, `:7577`).

## Notable Patterns

- **Reserved-channel vocabulary:** a tiny set of interned keys (`INTERRUPT`, `RESUME`, `ERROR`, `RETURN`, `NO_WRITES`) forms a protocol layer over plain channel writes (`_internal/_constants.py:7-24`).
- **Discriminated-union payloads:** debug events wrap payloads with `type: Literal["checkpoint"|"task"|"task_result"]` plus `step` and timestamp (`types.py:223-261`), giving consumers a stable progress timeline.
- **Monotonic identifiers everywhere:** uuid6 checkpoint IDs keyed on step (`checkpoint/base/__init__.py:854`) and xxh3-derived stable interrupt IDs from task namespace (`types.py:612-618`) make ordering/addressing deterministic across processes.
- **Hidden-work tagging:** `TAG_HIDDEN` (`langsmith:hidden`, `constants.py:26-27`) excludes internal plumbing from streams and debug output so observers see user-meaningful progress only (`debug.py:44-45`, `_loop.py:1420-1423`).
- **Write-ordering guarantees:** checkpoint puts are chained futures ensuring the saver observes checkpoints in order even when saving off-thread (`_loop.py:1199-1209`).

## Tradeoffs

- **Mechanical ≠ semantic progress.** The runtime can tell you precisely *that* 42 steps ran and what each wrote, but not whether any step reduced distance-to-goal; graphs can spin indefinitely on useless cycles until the guard trips. The high default ceiling (10007, `_internal/_config.py:32`) favors long-running agents but permits large wasted spend before intervention.
- **Model-calibrated termination.** In the flagship prebuilt agent, "done" is whatever the model decides by omitting tool calls (`chat_agent_executor.py:834-841`); premature stopping and endless tool loops are both unguarded beyond the global step cap.
- **Observer complexity.** Seven stream modes plus a parallel experimental v3 protocol (`run_stream.py:36-55`) mean integrators must track evolving surfaces; `stream_events(version="v3")` is explicitly beta.
- **Interruption ergonomics vs safety:** requiring interrupt IDs when multiple interrupts pend (`_loop.py:916-920`) prevents wrong-slot resumes but pushes bookkeeping onto clients.

## Failure Modes / Edge Cases

- **Out-of-steps as hard failure:** hitting `recursion_limit` mid-run raises `GraphRecursionError` (`pregel/main.py:3002-3011`) rather than returning partial state; recovery relies on the caller inspecting checkpoint history (`get_state_history`) to diagnose where work stalled.
- **Replay vs resume ambiguity:** distinguishing time-travel (re-execute interrupts) from live resume (return cached resume values) requires subtle heuristics over run metadata and checkpoint maps (`_loop.py:860-900`) — a known-complex edge the code comments themselves flag.
- **Observer blind spots by design:** `TAG_HIDDEN` tasks and `UntrackedValue` channels are excluded from streams and checkpoint persistence (`debug.py:44-45`, `_loop.py:439-454`), so externally observed progress can legitimately undercount internal work.
- **Multi-writer same-step races on progress signals:** multiple tasks writing the same channel fold into `$writes` lists in task-result payloads (`debug.py:83-103`), and null-resume with multiple interrupts is explicitly disallowed (`tests/test_pregel.py:8906`).

## Future Considerations

- `DeltaChannel` counters (`counters_since_delta_snapshot` tracking per-channel update/superstep ratios, `checkpoint/base/__init__.py:63-86`) introduce per-channel *activity frequency* metrics — a foundation for richer progress analytics if exposed.
- The v3 `GraphRunStream` projection model (`stream/run_stream.py:36-65`) could become the single canonical progress surface once stabilized.
- Exposing `RemainingSteps`-style budgets more broadly (e.g., to conditional-edge functions) would let routing logic trade ambition against remaining budget explicitly.

## Questions / Gaps

- **No in-repo exemplars of model-scored goal tracking:** the `examples/` tree is archived stubs pointing to external docs (e.g., `examples/lats/lats.ipynb` contains only a move notice), so classic objective-tracking patterns (LATS reflection scores, plan-and-execute task lists) could not be verified against current code — searched all notebooks; content moved out of repo.
- **No evidence found** of any independent final-success checker, output-quality validator, or goal-attainment assertion in core, prebuilt, or checkpoint libraries (searched for `goal`, `objective`, `success`, `verify` across `libs/langgraph/langgraph/` and `libs/prebuilt/`).
- The JS/TS SDK (`libs/sdk-js`) wraps the REST API and was out of scope for runtime-behavior analysis; server-side progress endpoints (LangGraph Platform) are not part of this source.
- Whether LangGraph Platform/UI layers build higher-level objective dashboards atop these primitives cannot be answered from this repository.

---

Generated by `06.05-objective-and-progress-tracking` against `langgraph`.

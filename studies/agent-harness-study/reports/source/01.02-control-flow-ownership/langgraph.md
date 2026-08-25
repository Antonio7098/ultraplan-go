# Source Analysis: langgraph

## Control-Flow Ownership

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (monorepo: langgraph core + prebuilt + checkpoint + sdk) |
| Analyzed | 2026-07-02 |

## Summary

LangGraph is a Pregel/BSP-inspired orchestration runtime that puts control flow squarely on the framework, not the model. The orchestrator owns the superstep loop and decides what runs next based on (a) deterministic PregelNode triggers (`libs/langgraph/langgraph/pregel/_algo.py:606-613`, `_algo.py:1260-1277`) and (b) tasks pushed onto a reserved topic channel (`__pregel_tasks`) by user-defined branches (`libs/langgraph/langgraph/pregel/_io.py:67` writes to TASKS, drained by `prepare_next_tasks` at `_algo.py:441-466`). The LLM never directly chooses the next node; it produces either (i) writes to state channels (handled by an automatically-attached `ChannelWrite` writer, `libs/langgraph/graph/state.py:1482-1492`, `libs/langgraph/graph/_branch.py:122-144`) or (ii) a `Send`/`Command(goto=…)` that the framework queues for the *next* superstep. The runtime can override the loop at three checkpoints: a `RunControl.request_drain()` (read at `_loop.py:650-652`, raised as `GraphDrained` at `main.py:3027-3030`/`3510-3511`), a static `recursion_limit` (enforced at `_loop.py:600-602` and surfaced as `GraphRecursionError` at `main.py:3017-3026`), and node-level error handlers attached via `node_error_handler_map` (`_runner.py:148-174`, called at `_runner.py:222-232` and `_runner.py:304-323` for parallel failure paths). Transitions are explicit: a single `PregelLoop.tick()` returns a boolean that the outer `while loop.tick(): ...` consumes (`main.py:2979-2999`), and the next-step computation is centralized in `prepare_next_tasks`/`prepare_single_task`/`prepare_node_error_handler_task`. Control flow is testable without an LLM because `prepare_next_tasks` is exposed publicly with two overloads (planning vs. execution, `_algo.py:348-410`) and exercised in `libs/langgraph/tests/test_algo.py:6-43` and across `tests/test_pregel.py`/`tests/test_runtime.py` without any model involvement.

## Rating

**8 / 10** — Clear, explicit, and uniform control model with strong runtime override authority and a public test surface that does not require an LLM. The runtime can reject a runaway loop (`recursion_limit` -> `GraphRecursionError`, `main.py:3017-3026`) and can stop a run mid-stride via `RunControl.request_drain()` (`_loop.py:650-652`, `runtime.py:79-104`). Transitions are not scattered: every transition is either (a) a static edge compiled into a `PregelNode` (`main.py:241-289`), (b) a static conditional mapped through `BranchSpec.run` (`graph/_branch.py:122-167`), or (c) a `Send`/Command routed into the `TASKS` topic at the start of the next superstep. It does not reach 9–10 because (i) the public `_internal` module path remains the source of truth for many transition primitives (`_internal/_constants.py:6-93`), (ii) the `goto`/static-endpoints path has a small inconsistency where runtime routing (`map_command`, `_io.py:56-78`) and the writer fallback (`_control_branch`, `state.py:1735-1761`) both encode routing logic in two places, and (iii) cooperative drain is single-bit with no priority/reason prioritization beyond the first caller.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Next-step decision (the loop's "what runs next") | `PregelLoop.tick` calls `prepare_next_tasks` (with `for_execution=True`) and exits with `status="done"` / `"out_of_steps"` / `"interrupt_before"` / `"draining"` | `libs/langgraph/langgraph/pregel/_loop.py:592-674` |
| Next-step selection (algorithm) | `prepare_next_tasks` enumerates PUSH tasks from `channels[TASKS]` and PULL tasks from nodes whose trigger channels advanced | `libs/langgraph/langgraph/pregel/_algo.py:392-513` |
| Channel-version trigger check | `_triggers` returns True if any of the node's `triggers` channels is available and its version is greater than what the node has "seen" | `libs/langgraph/langgraph/pregel/_algo.py:1260-1277` |
| Outer driver loop | `while loop.tick():` in sync `Pregel.stream` (and the async twin) — model never picks the next node itself; the framework iterates | `libs/langgraph/langgraph/pregel/main.py:2979-2999`, `main.py:3447-3456` |
| Top-level termination statuses | `Literal["input","pending","done","draining","interrupt_before","interrupt_after","out_of_steps"]` set on every superstep | `libs/langgraph/langgraph/pregel/_loop.py:249-257` |
| PUSH (Send) routing into next superstep | `map_command` writes `TASKS` or `branch:to:<node>` to NULL_TASK_ID; TASKS is a topic channel drained at the top of `prepare_next_tasks` | `libs/langgraph/langgraph/pregel/_io.py:56-78`, `_algo.py:441-466` |
| Static branch routing | `BranchSpec.run` -> `ChannelWrite.register_writer(RunnableCallable(_route))`, with a `_route`/`_aroute`/`_finish` triplet that turns the path's return value into channel writes | `libs/langgraph/langgraph/graph/_branch.py:83-224` |
| END + node-controlled destination | `_control_branch` rewrites `Command.goto` into `(branch:to:<node>, None)` or `(__pregel_tasks, Send)` writes; END is dropped from routing | `libs/langgraph/langgraph/graph/state.py:1735-1761` |
| StateGraph -> compiled triggers/edges | `attach_edge` wires a `branch:to:<end>` write to the start node and registers the destination's trigger; `attach_branch` wires `BranchSpec` similarly | `libs/langgraph/langgraph/graph/state.py:1537-1586` |
| Runtime override authority: drain | `RunControl.request_drain(reason)` checked at the top of `tick()`; the loop raises no error but flips status to `draining`; outer code translates to `GraphDrained` | `libs/langgraph/langgraph/runtime.py:79-104`, `libs/langgraph/langgraph/pregel/_loop.py:650-652`, `libs/langgraph/langgraph/pregel/main.py:3027-3030` |
| Runtime override authority: recursion | `tick()` starts with `if self.step > self.stop: status="out_of_steps"` (stop = `step + recursion_limit + 1`); outer raises `GraphRecursionError` | `libs/langgraph/langgraph/pregel/_loop.py:599-602`, `_loop.py:1677`, `libs/langgraph/langgraph/pregel/main.py:3017-3026` |
| Runtime override authority: interrupt | `interrupt_before`/`interrupt_after` checked at `tick()` (before execution) and `after_tick()` (after) via `should_interrupt` | `libs/langgraph/langgraph/pregel/_loop.py:659-664`, `_loop.py:707-712`, `libs/langgraph/langgraph/pregel/_algo.py:155-185` |
| Runtime override authority: dynamic Interrupt | `langgraph.types.interrupt()` raises `GraphInterrupt` inside the user node; `_runner.commit` and `_panic_or_proceed` collect interrupts and bubble them up as `GraphInterrupt` | `libs/langgraph/langgraph/types.py:811-934`, `libs/langgraph/langgraph/pregel/_runner.py:574-613`, `libs/langgraph/langgraph/pregel/_runner.py:650-697` |
| Task scheduler (single superstep) | `PregelRunner.tick` / `atick` runs tasks concurrently, applies `run_with_retry`/`arun_with_retry`, handles errors, schedules error handlers | `libs/langgraph/langgraph/pregel/_runner.py:176-358`, `libs/langgraph/langgraph/pregel/_runner.py:360-572` |
| Pause / cancel other tasks on failure | `_should_stop_others` is consulted from `FuturesDict.should_stop` (wakes the event when any tracked future fails) and from the main runner loop; GraphInterrupts are explicitly excluded | `libs/langgraph/langgraph/pregel/_runner.py:616-634`, `libs/langgraph/langgraph/pregel/_runner.py:75-132` |
| Node-level error handler routing | `_should_route_to_error_handler(task)` looks up `node_error_handler_map`; runner calls `schedule_error_handler` (sync/async) before raising | `libs/langgraph/langgraph/pregel/_runner.py:171-174`, `_runner.py:222-232`, `_runner.py:304-323`, `_runner.py:417-422`, `_runner.py:496-503` |
| Error handler task fabrication | `prepare_node_error_handler_task` builds a fresh executable task against the same checkpoint; loop stores it under `self.tasks[handler_task.id]` | `libs/langgraph/langgraph/pregel/_loop.py:739-804`, `libs/langgraph/langgraph/pregel/_algo.py:1110-1249` (function definition) |
| Subgraph control routing | `map_command` raises nothing special; `attach_branch`/`attach_edge` route `Command(graph=PARENT)` upward via the reserved `ParentCommand` exception | `libs/langgraph/langgraph/pregel/_io.py:58-59`, `libs/langgraph/langgraph/graph/state.py:1747-1748` |
| `_panic_or_proceed` semantics | Mixed interrupts are aggregated into one `GraphInterrupt`; panics are re-raised unless they fall in `SKIP_RERAISE_SET` (handled) or `_handled_exception_ids` (routed) | `libs/langgraph/langgraph/pregel/_runner.py:650-697` |
| Testability without an LLM | `prepare_next_tasks(..., for_execution=False, ...)` overload exercised in `tests/test_algo.py:6-43`; `RunControl` interactions tested in `tests/test_runtime.py:100-246` | `libs/langgraph/langgraph/pregel/_algo.py:348-389`, `libs/langgraph/tests/test_algo.py:6-43`, `libs/langgraph/tests/test_runtime.py:100-246` |
| Static routing index | `_trigger_to_nodes` inverts `node.triggers` so `prepare_next_tasks` can use `updated_channels & trigger_to_nodes` to skip the full pass when channels are unchanged | `libs/langgraph/langgraph/pregel/main.py:4190-4196`, `libs/langgraph/langgraph/pregel/_algo.py:475-512` |

## Answers to Dimension Questions

1. **Who decides what happens next?**
   The framework, via `PregelLoop.tick()` → `prepare_next_tasks()` (`libs/langgraph/langgraph/pregel/_loop.py:605-622`, `_algo.py:392-513`). Each "next" is computed from channel-version deltas (`_triggers`, `_algo.py:1260-1277`) plus the topic channel `TASKS` (`_algo.py:441-466`). The LLM does **not** name the next node; it can only contribute writes — one of which may be a `Send` (PUSH) or `Command(goto=…)` that the next `tick()` consumes (`_io.py:56-78`). The outer `while loop.tick():` driver lives in `Pregel.stream`/`astream` (`main.py:2979-2999`, `main.py:3447-3456`).

2. **Can the LLM bypass runtime control?**
   No in the sense of choosing an arbitrary "next step" — the model can only (a) write to declared state channels, (b) write `__interrupt__` (triggers a runtime pause), (c) write a `Send`/`Command` that the *next* superstep will execute, (d) raise inside its body which is captured by `_runner.commit` (`_runner.py:574-613`). What it cannot do: skip the `recursion_limit` check (`_loop.py:600-602`), ignore a `RunControl.drain` (`_loop.py:650-652`), or smuggle writes into a reserved `TASKS` channel from user code — `ChannelWrite.do_write` rejects that explicitly (`_write.py:113-117`).

3. **Can the runtime reject or rewrite the next action?**
   Yes. (a) `tick()` computes the task set but immediately checks `should_interrupt` *before* running tasks (`_loop.py:659-664`); if the conditional edge returned a destination gated by `interrupt_before`, the loop raises `GraphInterrupt` instead of executing. (b) `_should_stop_others` cancels sibling tasks as soon as any unhandled exception is observed (`_runner.py:616-634`). (c) `_should_route_to_error_handler` redirects an exception into a fresh error-handler task rather than raising it (`_runner.py:171-174`, called at `_runner.py:222-232` and `_runner.py:304-323`). (d) The runner's "panic" path (`_panic_or_proceed`, `_runner.py:650-697`) is the sole gate between "task failed" and "raise to caller". (e) `ChannelWrite.do_write` rejects writes to `TASKS` and un-substituted `PASSTHROUGH` values (`_write.py:111-122`).

4. **Are transitions explicit or scattered?**
   Mostly explicit, but with two known seams: (a) static transitions are entirely explicit (`attach_edge`/`attach_branch` wire each one to a `branch:to:<name>` channel write, `state.py:1537-1586`); (b) dynamic transitions are explicit at compile time via the writers attached to every node (`state.py:1482-1492`) and at runtime via `map_command` (`_io.py:56-78`) and `_control_branch` (`state.py:1735-1761`). The two functions encode essentially the same routing logic in two places — `_control_branch` for values returning from a node body, `map_command` for values arriving as the next user input. A `Command(graph=PARENT)` is intercepted early (`_io.py:58-59`) and a sub-graph `Command(graph=PARENT)` raises `ParentCommand` at the writer (`state.py:1747-1748`). `END` is consistently dropped from routing (`state.py:1757`, `state.py:1774`, `_branch.py:207-210`).

5. **Is control flow testable without calling an LLM?**
   Yes. The `prepare_next_tasks` API has two overloads — planning (`for_execution=False`) and execution (`for_execution=True`) (`_algo.py:348-389`), and the planning overload is exercised directly in `tests/test_algo.py:6-43`. The drain behavior is exercised in `tests/test_runtime.py:100-246` using only synthetic `RunControl`/`TypedDict` graphs. Routing logic is exercised across the whole `tests/test_state.py`/`tests/test_pregel.py` corpus with no `ChatModel` involved — the model's calls are abstracted behind `Runnable` so that the routing harness runs identically with or without one.

## Architectural Decisions

- **BSP / Pregel "Bulk Synchronous Parallel" loop.** The loop body is `tick` → `apply_writes` → `_put_checkpoint` → `after_tick`, with the explicit comment at `main.py:2974-2978`: "Similarly to Bulk Synchronous Parallel / Pregel model / computation proceeds in steps, while there are channel updates. Channel updates from step N are only visible in step N+1".

- **Channels as the single source of truth for next-step semantics.** Each node subscribes to "trigger" channels (`NodeBuilder._triggers` filled in `subscribe_to` at `main.py:257-290`; `attach_node` at `state.py:1512-1533`). The next-step algorithm is "for each node, if any trigger channel advanced past its last seen version, prepare a task" (`_algo.py:606-612`, `_algo.py:1260-1277`).

- **Two layer routing.** Static edges compile down to direct `ChannelWrite` writes (`attach_edge`, `state.py:1537-1561`). Conditional edges compile to a `BranchSpec` whose `_route` evaluates the path call under `config[CONF][CONFIG_KEY_READ]` — i.e. the path callable is run inline with the node, *not* deferred into the next superstep (`_branch.py:146-167`). Push-style routing (`Send`, `Command.goto`) is deferred: it goes into the `TASKS` topic and is consumed only at the next tick (`_algo.py:441-466`).

- **Single point of override.** `RunControl` is the only mechanism by which the runtime can stop a run mid-graph without an exception (`runtime.py:79-104`). It's checked once per tick at `_loop.py:650-652` and surfaced as `GraphDrained` (`errors.py:54-64`) at `main.py:3027-3030`/`main.py:3507-3511`. Notably: drains are *cooperative*, evaluated at superstep boundaries, not preemptive.

- **Error model is rich and type-discriminated.** `GraphBubbleUp` is the umbrella for `GraphInterrupt`, `GraphDrained`, `ParentCommand` (`errors.py:50-65`, `errors.py:102-135`). The runtime can choose, per exception type: to bubble (`GraphBubbleUp`), to handle via a node error handler (user-attached, `_runner.py:171-174`), or to panic via `_panic_or_proceed` (`_runner.py:650-697`).

- **`interrupt_before`/`interrupt_after` are first-class.** They are passed to the loop at construction (`_loop.py:283-284`), evaluated by `should_interrupt` (`_algo.py:155-185`), and gate execution before/after every superstep (`_loop.py:659-664`, `_loop.py:707-712`).

- **No implicit escalation.** A node that exits without writing anything is fine — `_runner.commit` adds a `NO_WRITES` marker rather than raising (`_runner.py:609-611`).

## Notable Patterns

- **Channel versioning (Pregel-style).** `checkpoint["channel_versions"]` is a `dict[str, str|int]` advanced by `GetNextVersion` on each write. `_triggers` compares `versions[chan] > seen.get(chan, null_version)` (`_algo.py:1260-1277`). This is the substrate of "what should fire next" — without it the next-step decision would be scattered across edges.

- **Trigger index.** `_trigger_to_nodes` builds a static `trigger -> [nodes]` map (`main.py:4190-4196`) so `prepare_next_tasks` can short-circuit when no relevant channel changed (`_algo.py:475-486`).

- **Writer-attached routing.** Every node's `writers` list (`state.py:1482-1492`, `main.py:241-289`) includes a `ChannelWriteTupleEntry(mapper=_control_branch, ...)`; this is how `Command`/`Send` returns are silently re-routed into writes without an explicit "router step" existing in the graph.

- **Scratchpad routing of state into a node.** `CONFIG_KEY_SCRATCHPAD` carries a `PregelScratchpad` with `interrupt_counter`, `call_counter`, `subgraph_counter`, and a `get_null_resume` closure (`_scratchpad.py`, used in `_algo.py:1280-1345`). Allows `interrupt()` (the public API, `types.py:811-934`) to "consume" a resume value and resume execution without leaking the resume data into node signatures.

- **ChannelWrite is the universal writer.** Both user nodes and conditional branches produce writes via `ChannelWrite.register_writer` + `RunnableCallable` (`_write.py:46-169`, `_branch.py:122-144`). `ChannelWrite.do_write` is the single funnel that validates and pushes writes into `CONFIG_KEY_SEND` (`_write.py:106-126`).

- **Cooperative drain.** `RunControl` is a single-bit, lock-free cooperative stop primitive (`runtime.py:90-104`). It is *not* preemptive — it is read at the start of each `tick()`. The model does not have authority to override it.

## Tradeoffs

- **Model has no "go to" primitive, only "post for next step".** This is intentional: it gives the runtime the only authority over superstep boundaries and makes durable execution straightforward (every transition happens between checkpointed states). The tradeoff is that fan-out / map-reduce requires `Send`/`Command(goto=)` instead of an `await Task.next()`, which is slightly more ceremony but is exactly what makes durability tractable.

- **Two routing paths share the same language.** `map_command` (`_io.py:56-78`) and `_control_branch` (`state.py:1735-1761`) both convert `Command`/`Send` -> channel writes but live in different layers (input vs. node-output). They are duplicated (and have very slightly different vocabularies — `map_command` writes `branch:to:<name>` to NULL_TASK_ID, while `_control_branch` writes `_CHANNEL_BRANCH_TO.format(go)`). A future refactor could unify them under one `_route_command` helper.

- **Drain is superstep-bounded, not preemptive.** The runtime cannot abort a node mid-execution; it must wait for the next tick. That's safer (no partial writes) but means a long-running node (e.g. an LLM with a slow upstream call) cannot be canceled by `RunControl.request_drain()` — only by the per-node `TimeoutPolicy` (`_retry.py:111-186`, raised as `NodeTimeoutError`).

- **Routing is **declared at compile time**.** Static routes (add_edge, add_conditional_edges) become channel writes at compile; dynamic routes (Command, Send) are deferred into the next superstep. This keeps the data-flow graph static and analyzable (X-ray visualization, `_draw.py`), but means a "graph that changes itself at runtime" requires re-compilation rather than imperatively mutating the running loop.

- **External control surface is small.** `RunControl` is the only async-friendly runtime override visible to user code. There is no public "pause", "resume by id", "inject task" — all of those flow through `Command(resume=...)`, `Command(goto=...)`, or via the checkpointer (`get_state`/`update_state`).

- **`tests/test_algo.py` is very thin** (only 67 lines; `test_algo.py:1-67`). The promise of testability without an LLM is real, but the dedicated unit tests of `prepare_next_tasks` are sparse; most coverage is indirect via `test_pregel.py`.

## Failure Modes / Edge Cases

- **Self-loop until `recursion_limit`.** A node that always produces a write (e.g., appends to a list it reads) can grow the loop monotonically; `_loop.py:600-602` is the only backstop. See `tests/test_pregel.py:8366-8491` and `main.py:3017-3026`.

- **Unhandled exception in a non-error-handler task.** The runner cancels siblings (`_should_stop_others`, `_runner.py:616-634`) and re-raises via `_panic_or_proceed` (`_runner.py:650-697`). `GraphBubbleUp` (i.e. `GraphInterrupt`, `GraphDrained`, `ParentCommand`) does not trigger this cancellation by design — see `_runner.py:629-631`.

- **Concurrency: the futures dict is keyed on the future itself, not the task.** Two retries for the same task get distinct futures (`_runner.py:114-114`); `SKIP_RERAISE_SET` and `_handled_exception_ids` track which futures are deliberately swallowed (`_runner.py:70-72`, `_runner.py:166-169`, `_runner.py:235-239`).

- **`interrupt()` called outside a runtime.** `interrupt()` (in `types.py:811-934`) reads from `get_config()[CONF]`; calling it without `langgraph.pregel`'s surrounding context will raise `KeyError`. There is no explicit "no-runtime" guard.

- **Drain request *before* the run starts.** `control` is accepted as a parameter on `stream`/`astream` (`main.py:2643`, `main.py:3527`). If a user constructs `RunControl()`, calls `request_drain()`, then passes it in, the first `tick()` immediately flips to `"draining"` (`_loop.py:650-652`). See `tests/test_runtime.py:880-905`.

- **Subgraphs and `drain_requested`.** The control surface is propagated from parent to subgraph via `parent_runtime.control` precedence (`main.py:2894`, `main.py:3337`), but `control or parent_runtime.control or RunControl()` means a parent drain *replaces* a child drain — they are the same object. This is a runtime-driven override of any prior child state. See `tests/test_runtime.py:203-246`.

- **Reserved channels cannot be written by users.** `ChannelWrite.do_write` rejects writes to `TASKS` (`_write.py:113-117`), and the loop fills the TASKS topic only from `Send`/`Command.goto` writes produced via `_io.py`/`_control_branch`. Mishandling these paths bypasses the BSP synchronization.

- **Timer-driven interrupts.** `interrupt_before`/`interrupt_after` are evaluated by `should_interrupt` against *channel* updates (`_algo.py:155-185`), not "node just executed"; this is fine for static gating but means a state-channel-driven loop without channel-version advancement is not detected by `interrupt_after`.

## Future Considerations

- **Extend `RunControl` with non-cooperative signals.** Currently a single bit; a richer signal (priority + reason) could let the runtime prioritize shutdown vs. user cancel, and would let tooling like LangSmith Studio cancel mid-step rather than waiting for the next tick.

- **Unify `map_command` and `_control_branch`.** Both interpret `Command`/`Send` into writes; the duplication is small but non-zero. A single `_route_command(cmd, *, target_node_re=None)` helper that both call sites share would shrink the surface.

- **Move reserved-channel constants out of `_internal`.** Constants like `PUSH`, `PULL`, `TASKS`, `__pregel_*` config keys are scattered across `_internal/_constants.py:1-140`, but their semantics (which writes are special) are part of the public control-flow contract. A thin public re-export would make the control model less reliant on `_internal`.

- **Drain parity with `step_timeout`.** `step_timeout` is enforced inside `runner.tick`/`atick` at every wait (`main.py:2980-2987`, `main.py:3452-3457`); the drain signal could similarly be checked *between* futures completing, not only at superstep boundaries, to give operators a faster SIGTERM response.

- **Direct state-graph reachability for Send/Command.goto.** Currently the `TASKS` topic channel and the `branch:to:<node>` channels are the only way to drive the next step; exposing them as first-class "edges" in `_draw.py` and `_validate.py` would close the loop between visual graph inspection and runtime routing.

- **More direct tests of `prepare_next_tasks` behavior.** `tests/test_algo.py:1-67` covers only the empty-case. Covering `PUSH` from `TASKS`, `PULL` triggers, and `_triggers` correctness would harden the test surface that already claims to be LLM-free.

## Questions / Gaps

- **What happens if the same node appears in multiple parallel branches?** Not explicitly answered in `_algo.py`'s `prepare_next_tasks`; task IDs are derived from `(checkpoint_id, namespace, step, name, PULL, *triggers)` (`_algo.py:614-623`) — the *triggers tuple* is part of the task ID, so a node subscribed to different triggers would get distinct tasks, but the contract for "two PULL tasks for the same node name in the same step" is implicit.

- **Can user code trigger an interrupt mid-iteration, not just at the configured `interrupt_before`/`interrupt_after`?** Yes, via `interrupt()` (`types.py:811-934`); but there is no documented "interrupt while a Send is in flight" contract.

- **Is `RunControl` thread-safe across nested subgraphs?** The implementation note at `runtime.py:80-87` says "Safe to call from any thread: the drain request is represented by a single attribute write, so no lock is needed for this signal. If more mutable state is added here, add synchronization." That is a forward-looking caveat, not a current limitation.

- **Does `Command(goto=...)` survive across `Command(graph=PARENT)` boundaries?** Routed via `_io.py:58-59` and `state.py:1747-1748`, but no explicit unit test in `tests/test_parent_command.py` was inspected for this combination.

- **`_panic_or_proceed` with `handled_futures`** is recent (referenced at `_runner.py:288-289` inline comment); coverage is implied via `tests/test_pregel.py`'s larger suites but not enumerated here.

---

Generated by `dimension-01.02-control-flow-ownership` against `langgraph`.

# Source Analysis: langgraph

## 03.01 LLM Turn Loop Structure

### Source Info

| Field | Value |
|-------|-------|
| Name | `langgraph` (monorepo, focus on `libs/langgraph` core + `libs/prebuilt`) |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (3.10+), core in `libs/langgraph/langgraph`, prebuilt agents in `libs/prebuilt/langgraph/prebuilt` |
| Analyzed | 2026-07-13 |

## Summary

LangGraph does **not** ship a hand-written LLM turn loop. The model-call loop is the **Pregel Bulk Synchronous Parallel (BSP) engine** itself, and a ReAct-style "turn" is emergent: it spans 2+ Pregel supersteps across the nodes of a `StateGraph`. The outer driver is a plain Python `while loop.tick(): for _ in runner.tick(...): ... loop.after_tick()` in `libs/langgraph/langgraph/pregel/main.py:2979` (sync) and `libs/langgraph/langgraph/pregel/main.py:3452` (async).

Three layers cooperate:

1. `PregelLoop.tick()` / `after_tick()` (`libs/langgraph/langgraph/pregel/_loop.py:592-674`, `libs/langgraph/langgraph/pregel/_loop.py:676-714`) — picks the next batch of `PregelExecutableTask`s via `prepare_next_tasks` (`libs/langgraph/langgraph/pregel/_algo.py:392`), checks `interrupt_before/after`, applies writes via `apply_writes` (`libs/langgraph/langgraph/pregel/_algo.py:232`), and commits a checkpoint via `_put_checkpoint` (`libs/langgraph/langgraph/pregel/_loop.py:1064-1199`).
2. `PregelRunner.tick()` / `PregelRunner.atick()` (`libs/langgraph/langgraph/pregel/_runner.py:176-358`, `libs/langgraph/langgraph/pregel/_runner.py:360-572`) — executes the prepared tasks concurrently under a `BackgroundExecutor` / `AsyncBackgroundExecutor` (`libs/langgraph/langgraph/pregel/_executor.py:40`, `libs/langgraph/langgraph/pregel/_executor.py:122`), yields control back to the caller between completions for streaming, commits via `commit()` (`libs/langgraph/langgraph/pregel/_runner.py:574-613`).
3. The "turn" is the agent graph that `create_react_agent` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:278-1002`) compiles: `pre_model_hook? → agent → (should_continue) → tools? → generate_structured_response? → END`. The model call itself is `call_model`/`acall_model` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:661-694` and `:696-721`), which invokes `static_model.invoke(state, config)` where `static_model = _get_prompt_runnable(prompt) | model.bind_tools(...)` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:586-590`).

The loop body phase contract is fixed across every agent, subgraph, and re-entry: tick → runner → after_tick. Termination is **two-layered**: the agent graph routes to `END` when `should_continue` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:831-859`) sees no `tool_calls`, and the outer Pregel loop terminates when `tick()` returns False (no tasks left, all graphs drained, or `out_of_steps` at `libs/langgraph/langgraph/pregel/_loop.py:600-602`). Persisted state is **per superstep**: every `after_tick` invokes `_put_checkpoint`, and writes are also flushed via `put_writes` (`libs/langgraph/langgraph/pregel/_loop.py:408-501`). The message reducer `add_messages` (`libs/langgraph/langgraph/graph/message.py:61`) is the state-mutation primitive between supersteps.

## Rating

**9 / 10** — Mature, durable, observable, extensible. Two-loop separation (Pregel superstep loop over graph + agent conditional-edge loop over nodes) makes the basic turn → conditional-routing → tool-execution → model-call shape inferable from a single read of `main.py:2974-3003` plus `chat_agent_executor.py:830-870`. All four mechanisms the dimension asks about (runner loop, turn records, model-call wrapper, message assembly, final-output condition) are first-class and live in well-named symbols. Deductions: "turn" is not a first-class language concept — only "superstep", "step counter", `PregelExecutableTask`, and the channel write-set are; the conditional-edge loop the *user perceives* as "one turn" actually crosses two Pregel supersteps (model node → tool node), so an LLM-driven agent does not have a single observable turn boundary in the loop, only in routing. The `recursion_limit` knob stops at the outer loop only (`libs/langgraph/langgraph/pregel/_loop.py:1677`), and the agent-level `remaining_steps` (`libs/langgraph/langgraph/managed/is_last_step.py:18-22`) is advisory.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Outer driver loop (sync) | `while loop.tick(): for _ in runner.tick(...): ... loop.after_tick()` with comment "Similarly to Bulk Synchronous Parallel / Pregel model" | `libs/langgraph/langgraph/pregel/main.py:2974-3003` |
| Outer driver loop (async) | same shape as `async for _ in runner.atick(...)` inside `while loop.tick()` | `libs/langgraph/langgraph/pregel/main.py:3446-3477` |
| Runner construction | `PregelRunner(submit=..., put_writes=..., node_finished=..., schedule_error_handler=loop.schedule_error_handler, ...)` | `libs/langgraph/langgraph/pregel/main.py:2938-2946`, `libs/langgraph/langgraph/pregel/main.py:3392-3401` |
| Loop body — tick | `PregelLoop.tick()` prepares next tasks via `prepare_next_tasks`, checks `interrupt_before`, emits "tasks"/"checkpoints" debug | `libs/langgraph/langgraph/pregel/_loop.py:592-674` |
| Loop body — runner | `PregelRunner.tick` (and `PregelRunner.atick`) is a generator yielding after every `concurrent.futures.wait(FIRST_COMPLETED)` batch | `libs/langgraph/langgraph/pregel/_runner.py:176-358`, `libs/langgraph/langgraph/pregel/_runner.py:360-572` |
| Loop body — after_tick | `apply_writes` → emit "values" → capture delta writes for exit mode → clear pending writes → `_put_checkpoint({"source":"loop"})` → `interrupt_after` | `libs/langgraph/langgraph/pregel/_loop.py:676-714` |
| Turn record (per-superstep state) | `PregelExecutableTask` with `name`, `input`, `writes: deque[tuple[str,Any]]`, `triggers`, `id`, `path` | `libs/langgraph/langgraph/types.py:627-641` |
| Per-step record (read-side) | `PregelTask` NamedTuple (id, name, path, error, interrupts, state, result) | `libs/langgraph/langgraph/types.py:597-606` |
| Model call wrapper (sync) | `call_model(state, runtime, config)` resolves dynamic model, calls `static_model.invoke(model_input, config)`, returns `{"messages": [response]}` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:661-694` |
| Model call wrapper (async) | `acall_model` mirrors sync with `ainvoke` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:696-721` |
| Model call wrapper (dynamic) | `_resolve_model` / `_aresolve_model` build the runnable per-state; supports `(state, runtime) -> BaseChatModel` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:599-618` |
| Tool binding | `model = cast(BaseChatModel, model).bind_tools(tool_classes + llm_builtin_tools)` after a sanity check that names match | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:582-589` |
| Message assembly — prompt prefix | `_get_prompt_runnable` wraps `State["messages"]` with optional system-message prefix (str / SystemMessage / callable / Runnable) | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:137-170` |
| Message assembly — pre-Model hook | Reads `state["llm_input_messages"]` or `state["messages"]`; `CallModelInputSchema` adds `llm_input_messages` key | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:636-742` |
| Message assembly — final LLM → state | Returns `{"messages": [response]}` where response is `AIMessage`; reducer `add_messages` extends the channel | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:693-694`, `libs/langgraph/langgraph/graph/message.py:61` |
| Message assembly — structured response | Separate node `generate_structured_response` runs once after the loop ends; uses `with_structured_output` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:744-785`, `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:899-915` |
| Turn boundary — tool-decision edge | `should_continue(state)` returns `"tools"` (v1), list of `Send("tools", ToolCallWithContext)` (v2), or END | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:830-859` |
| Turn boundary — step-budget edge | `route_tool_responses` returns END early if any tool has `return_direct=True` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:970-988` |
| Turn boundary — `recursion_limit` | `self.stop = self.step + self.config["recursion_limit"] + 1` set in `__enter__`; `tick()` flips status to `out_of_steps` on overrun | `libs/langgraph/langgraph/pregel/_loop.py:1677`, `libs/langgraph/langgraph/pregel/_loop.py:600-602` |
| Turn boundary — `remaining_steps` | `RemainingStepsManager.get(scratchpad) = stop - step`; `_are_more_steps_needed` injects "Sorry, need more steps…" message | `libs/langgraph/langgraph/managed/is_last_step.py:18-22`, `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:620-634`, `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:684-692` |
| Multiple tool calls per turn (v1) | `ToolNode._run_one` runs each tool call in the same node; `executor.map(self._run_one, ...)` parallelizes | `libs/prebuilt/langgraph/prebuilt/tool_node.py:793-826`, `libs/prebuilt/langgraph/prebuilt/tool_node.py:1014-1067` |
| Multiple tool calls per turn (v2) | `Send("tools", ToolCallWithContext(...))` produces one Pregel task per tool call; uses `ToolCallWithContext.__type` discriminator | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:846-859`, `libs/prebuilt/langgraph/prebuilt/tool_node.py:286-306`, `libs/prebuilt/langgraph/prebuilt/tool_node.py:1237-1245` |
| Persist per superstep | `_put_checkpoint` runs on every `after_tick`; writes go through `put_writes` to `BaseCheckpointSaver` | `libs/langgraph/langgraph/pregel/_loop.py:706`, `libs/langgraph/langgraph/pregel/_loop.py:408-501`, `libs/langgraph/langgraph/pregel/_loop.py:1064-1199` |
| Persist durability modes | `do_checkpoint = self._checkpointer_put_after_previous is not None and (exiting or self.durability != "exit")` | `libs/langgraph/langgraph/pregel/_loop.py:1116-1118` |
| Persist writes synchronisation | `SyncPregelLoop._checkpointer_put_after_previous` waits on `self._delta_write_futs` then on `prev` future before `put` | `libs/langgraph/langgraph/pregel/_loop.py:1507-1524` |
| Persist writes synchronisation (async) | `AsyncPregelLoop._checkpointer_put_after_previous` awaits `asyncio.gather(*futs)` then `prev` before `aput` | `libs/langgraph/langgraph/pregel/_loop.py:1759-1778` |
| Generality — same loop across agents | `_first` + `tick` + `after_tick` is reused for subgraphs (not-nested path); `Loop.__exit__` pushes `_suppress_interrupt` once | `libs/langgraph/langgraph/pregel/_loop.py:836-1062`, `libs/langgraph/langgraph/pregel/_loop.py:1674` |
| Generality — task runner reused | `PregelRunner` constructed with `WeakMethod(loop.put_writes)` etc., shared across all subgraphs; nested graphs reuse the same submit path | `libs/langgraph/langgraph/pregel/main.py:2938-2946` |
| Final output condition — sync | `if loop.status == "out_of_steps": raise GraphRecursionError(...)` after `SyncPregelLoop` context exits | `libs/langgraph/langgraph/pregel/main.py:3017-3030` |
| Final output condition — async | `if loop.status == "out_of_steps": ...` after the `AsyncPregelLoop` block | `libs/langgraph/langgraph/pregel/main.py:3498-3510` |
| Test — recurring react patterns | `test_react_agent_graph_structure` snapshots the generated graph (4 nodes × 4 options × 2 versions) | `libs/prebuilt/tests/test_react_agent_graph.py:38-52` |
| Test — recursion_limit/out_of_steps | Imported & exercised in `tests/test_pregel*.py`; surfaced via `create_error_message(ErrorCode.GRAPH_RECURSION_LIMIT)` | `libs/langgraph/langgraph/pregel/main.py:3017`, `libs/langgraph/langgraph/errors.py` |
| Test — parallel tool calls | `test_react_agent_parallel_tool_calls` parametrised over `version` (v1/v2) | `libs/prebuilt/tests/test_react_agent.py:599` |

## Answers to Dimension Questions

1. **What happens during one turn?**
   A user-perceived "turn" in a ReAct agent is one full `agent → tools → agent` cycle. Each half is a Pregel superstep: `agent` superstep runs `call_model`/`acall_model` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:661-721`), which uses `static_model.invoke` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:679`); the state-machine then routes via `should_continue` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:831-859`); the `tools` superstep runs `ToolNode._run_one` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1014-1067`); writes apply in `after_tick` (`libs/langgraph/langgraph/pregel/_loop.py:676-714`), and a checkpoint is put via `_put_checkpoint` (`libs/langgraph/langgraph/pregel/_loop.py:1064-1199`). Two Pregel supersteps per "turn".

2. **Does every turn call the model?**
   Yes — the `agent` node is the only node that calls the model. The `tools` node only runs tool bodies, and `pre_model_hook` / `post_model_hook` are user-provided. The loop terminates when the last `AIMessage` carries no `tool_calls` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:835`), so the final superstep ends on a model call that produces a text-only answer; then `generate_structured_response` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:744-785`) may make one additional structured-output model call if `response_format` is set.

3. **Can a turn include multiple tool calls?**
   Yes, both `v1` and `v2`. v1 runs them sequentially inside one `ToolNode._func` invocation (`libs/prebuilt/langgraph/prebuilt/tool_node.py:821-826`); v2 fans one `Send("tools", ToolCallWithContext)` per `tool_call` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:849-859`), which the runner then schedules as parallel tasks (`libs/langgraph/langgraph/pregel/_runner.py:282-339`). Either way, all of the model's tool calls in one assistant turn execute before the next `agent` superstep.

4. **Is turn state persisted?**
   Yes, but the persistence unit is the **Pregel superstep**, not the agent-level turn. Every `after_tick` calls `_put_checkpoint` (`libs/langgraph/langgraph/pregel/_loop.py:706`), and `put_writes` persists channel writes per task (`libs/langgraph/langgraph/pregel/_loop.py:408-501`). The `_delta_write_futs` list is drained before the next checkpoint put (`libs/langgraph/langgraph/pregel/_loop.py:1515-1517`, `:1769-1771`) to guarantee writes are durable before the snapshot they produced. Durability modes (`"sync"`, `"async"`, `"exit"`) gate whether intermediate supersteps become durable at all (`libs/langgraph/langgraph/pregel/_loop.py:1116-1118`). There is no user-facing "turn record" — only channel snapshots + `checkpoint_pending_writes`.

5. **Is the loop generic or agent-specific?**
   Fully generic. `PregelLoop` (`libs/langgraph/langgraph/pregel/_loop.py:156-1444`) is reused for **any** `StateGraph`, **any** number of nested subgraphs, and **any** node topology. The agent-specific logic lives entirely in compiled graph structure built by `create_react_agent` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:861-1002`) and the `call_model` closure (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:661-694`). The prebuilt `ToolNode` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:622-`) is also used by other entry points (subgraph tool wrappers, custom agent stacks).

## Architectural Decisions

- **"Loop" = BSP superstep driver, not LLM turn driver.** LangGraph treats the unit of forward progress as a Pregel superstep, not a model call. This is the central decision: it turns every agent into just another graph, and gives durable, recoverable execution for free.
- **Single generic loop, agents composed as graphs.** The same outer driver (`libs/langgraph/langgraph/pregel/main.py:2979-3003`) executes any Pregel graph; per-agent differences (system prompt, tools, structured response, pre/post-model hooks) live in node functions and edge routing, not in specialised code.
- **Two-loop nesting for the ReAct pattern.** The model call lives one layer inside a graph loop. `should_continue` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:831-859`) converts a one-shot Pregel superstep (the model) into a turn loop via conditional edges + `Send` fan-out. This makes the same machinery run `route-tool-responses` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:970-988`) and `post_model_hook_router` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:919-962`) without bespoke loops.
- **Synchronous writes via `_delta_write_futs` + `_checkpointer_put_after_previous`.** Writes are submitted to the executor first, the checkpoint put is enqueued after them, and the put future awaits both prior write futures and the previous checkpoint future (`libs/langgraph/langgraph/pregel/_loop.py:1507-1524`). Async mirrors with `asyncio.gather` (`libs/langgraph/langgraph/pregel/_loop.py:1759-1778`).
- **Three durability tiers (`sync`, `async`, `exit`).** `async` (default) does not block on per-superstep checkpoints; `exit` defers the final put until `__exit__` (`libs/langgraph/langgraph/pregel/_loop.py:1294-1352`); `sync` blocks between supersteps (`libs/langgraph/langgraph/pregel/main.py:3002-3003`, `:3476-3477`). Delta channels (`DeltaChannel`) drive a separate periodic snapshot path (`libs/langgraph/langgraph/pregel/_checkpoint.py:37-58`, `_loop.py:1086-1142`) so high-frequency sub-freq writes remain recoverable.
- **Two Send shapes for fan-out.** v1 packs all tool calls into one `ToolNode` invocation; v2 splits per-call via `Send("tools", ToolCallWithContext(...))` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:849-859`) with a discriminated payload shape `__type="tool_call_with_context"` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:286-306`).
- **Errors modelled in-channel.** Node errors are persisted as `(ERROR, exc)` and `(ERROR_SOURCE_NODE, name)` writes (`libs/langgraph/langgraph/pregel/_runner.py:597-603`); the runner tracks `_handled_exception_ids` so error-handler tasks don't panic the superstep (`libs/langgraph/langgraph/pregel/_runner.py:166-169`, `:225-248`).

## Notable Patterns

- **"Tick / runner / after_tick" trinity.** Each superstep is `tick()` (prepare) → `runner.tick(...)` (execute with streaming yields) → `after_tick()` (commit). The user controls only the inner of the three. Pattern anchors: `libs/langgraph/langgraph/pregel/main.py:2979-3003` and `libs/langgraph/langgraph/pregel/main.py:3452-3477`.
- **`PregelExecutableTask` as the unit of work.** Carries `name`, `input`, `writes: deque`, `triggers`, `path` (`libs/langgraph/langgraph/types.py:627-641`). The same shape represents a PULL (graph-edge) task and a PUSH (`Send`/`call`) task — distinguishes at runtime.
- **`TaskId` deterministic from checkpoint + step + path.** `_triggers` and a hash of `(checkpoint_ns, step, name, PULL, *triggers)` identify tasks for replay dedup (`libs/langgraph/langgraph/pregel/_algo.py:596-643`).
- **Background executor + "next-tick" routing for `call()`.** User `call(func)` resolves through `_call_with_options` → `CONFIG_KEY_CALL` → `_call`/`_acall` (`libs/langgraph/langgraph/pregel/_call.py:276-298`, `_runner.py:700-786`). Subtasks can opt in to `__next_tick__=True` so streaming emits parent's writes first (`libs/langgraph/langgraph/pregel/_runner.py:777-782`, `:927-934`).
- **Generated graphs over hard-coded flows.** `create_react_agent` builds a `StateGraph` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:862-1002`) rather than encoding the ReAct loop imperatively. The graph structure itself is a `syrupy` snapshot in `libs/prebuilt/tests/test_react_agent_graph.py:38-52`.
- **Managed values for state-injected counters.** `RemainingSteps = Annotated[int, RemainingStepsManager]` (`libs/langgraph/langgraph/managed/is_last_step.py:18-22`) is computed lazily from `scratchpad.stop - scratchpad.step`.

## Tradeoffs

- **"Superstep" ≠ "turn".** A user looking for a single turn boundary must reason across two Pregel supersteps and the conditional edge. LangSmith / observability tooling (`libs/langgraph/langgraph/pregel/_messages.py:StreamMessagesHandler`) re-stitches the two into one "model invocation" via `langgraph_step` metadata. The unit is **not** surfaced at the Python API in a struct.
- **In-memory "turn record" framework.** `create_react_agent`'s graph compiles down to a few nodes (`agent`, `tools`, optional `pre_model_hook`, optional `post_model_hook`, optional `generate_structured_response`), so the per-turn record is implicit in two messages appended via the channel reducer (`libs/langgraph/langgraph/graph/message.py:61`). Good for streaming; bad for non-LLM turn-style agents that want a `Turn` row.
- **Both sync (`graph.stream()`) and async (`graph.astream()`) carry weight.** Every public surface is duplicated (`PregelRunner.tick` / `PregelRunner.atick`, `SyncPregelLoop.__enter__` / `AsyncPregelLoop.__aenter__`, `_checkpointer_put_after_previous` sync/async in `libs/langgraph/langgraph/pregel/_loop.py:1507-1524` and `:1759-1778`). Reduces runner reuse to `weakref.WeakMethod` plumbing — borderline fragile when the bound method's owning object is GC'd before the future.
- **Channel-mode vs delta-mode writes.** `LastValue`/`Topic` channels snapshot on every checkpoint; `DeltaChannel` (`libs/langgraph/langgraph/channels/delta.py` referenced at `libs/langgraph/langgraph/pregel/_checkpoint.py:37-58`) keeps high-frequency messages as WAL-style writes under `task_path`. Tradeoff: implementation complexity vs I/O cost for long agent traces. LangGraph solves this with the dual-mode `delta_channels_to_snapshot` rule (`libs/langgraph/langgraph/pregel/_checkpoint.py:37-58`).
- **`recursion_limit` is global, `remaining_steps` is per-state.** Agents must choose between letting the framework stop them (`recursion_limit: 25` default, set in `libs/langgraph/langgraph/pregel/_loop.py:1677`) and reading `RemainingSteps` from state for advisory budgets.
- **Durability="exit" drops intermediate checkpoints on crash.** `do_checkpoint = self._checkpointer_put_after_previous is not None and (exiting or self.durability != "exit")` (`libs/langgraph/langgraph/pregel/_loop.py:1116-1118`) suppresses intermediate puts in `exit` mode; the exit-mode delta accumulator in `_exit_delta_writes` (`libs/langgraph/langgraph/pregel/_loop.py:213-221`, `:680-700`) preserves delta-channel writes; `LastValue`-channel writes are lost on crash in this mode.
- **`Send` payload shape coupling.** v2's `ToolCallWithContext` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:286-306`) inlines state to keep message routing exact; this creates a serialization risk and forces `__type` discriminator on every routing branch. v1 sidesteps this entirely.

## Failure Modes / Edge Cases

- **`out_of_steps`.** `tick()` flips to status `out_of_steps` when `self.step > self.stop` (`libs/langgraph/langgraph/pregel/_loop.py:600-602`); the driver surfaces `GraphRecursionError` at `libs/langgraph/langgraph/pregel/main.py:3017` and `:3498`. The agent variant converts this into a soft "Sorry, need more steps..." `AIMessage` when the step budget is exhausted (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:620-634`, `:684-692`).
- **Tool raise.** `ToolNode._run_one` / `_arun_one` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1014-1067`, `:1161-1222`) catch exceptions classified by `handle_tool_errors`; non-handled exceptions propagate (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1002`, `:1149`). Propagated exceptions land in `PregelRunner.commit` as `(ERROR, exc)` writes (`libs/langgraph/langgraph/pregel/_runner.py:595-603`); error-handler tasks may then be scheduled (`libs/langgraph/langgraph/pregel/_runner.py:171-174`, `:300-323`).
- **GraphInterrupt.** Raised by `interrupt(...)` calls inside nodes. Persisted under `(INTERRUPT, value)` per task (`libs/langgraph/langgraph/pregel/_runner.py:585-591`); converted to public `GraphInterrupt` at `PregelLoop.__exit__` after one last `apply_writes` (`libs/langgraph/langgraph/pregel/_loop.py:1313-1352`).
- **Multi-interrupt resume.** `_pending_interrupts` (`libs/langgraph/langgraph/pregel/_loop.py:806-834`) computes the unresolved set; `Command(resume=dict[interrupt_id]=value)` flows through `map_command` (`libs/langgraph/langgraph/pregel/_io.py:56`) and `_first` (`libs/langgraph/langgraph/pregel/_loop.py:890-919`).
- **Time-travel replay.** `is_replaying / is_time_traveling` (`libs/langgraph/langgraph/pregel/_loop.py:308`, `:866-884`) drop stale `RESUME` writes; forks `{"source":"fork"}` checkpoint before re-execution (`libs/langgraph/langgraph/pregel/_loop.py:948-959`).
- **Drain mode.** `RunControl.drain_requested` returns False from `tick()` (`libs/langgraph/langgraph/pregel/_loop.py:650-652`); the loop honours `out_of_steps`, `interrupt_after`, and `draining` as the only non-error exit paths.
- **Background-executor reaping.** `_checkpointer_put_after_previous` swaps `_delta_write_futs` to `[]` then `concurrent.futures.wait(futs)` (`libs/langgraph/langgraph/pregel/_loop.py:1515-1517`); if a write future raises an exception, it surfaces in the next `tick`'s `commit()` callback chain.
- **Async cancellation.** `asyncio.create_task(self.stack.__aexit__(...))` in `AsyncPregelLoop.__aexit__` (`libs/langgraph/langgraph/pregel/_loop.py:1953-1962`) bubbles cancellation up by setting `e.args = (*e.args, exit_task)` so consumers can await the teardown explicitly.
- **Tool-call ID mismatch.** `_validate_tool_call` returns a `ToolMessage(status="error")` for unknown tools (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1268-1279`); `_validate_chat_history` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:243-271`) raises at start of every `call_model` if any `AIMessage.tool_calls` lacks a `ToolMessage`.

## Future Considerations

- **First-class turn record.** A `Turn` object capturing `(superstep_range, prompt, tool_calls, response, checkpoint_ids, latency)` would align with what users see in LangSmith today (`libs/langgraph/langgraph/pregel/_messages.py:StreamMessagesHandler`).
- **Migrating off parallel tick/atick.** Today there are two near-mirror impls (`PregelRunner.tick` / `.atick`); the runner could become a single async core with a sync adapter that drops down to `concurrent.futures` only at I/O boundaries.
- **Combine `recursion_limit` and `remaining_steps`.** Currently `recursion_limit` is per-invocation (`libs/langgraph/langgraph/pregel/_loop.py:1677`) and `remaining_steps` (`libs/langgraph/langgraph/managed/is_last_step.py:18-22`) is per-state; the latter is advisory only — a unified budget API would clarify per-agent ceilings.
- **Stop conditions for draining.** `status == "draining"` (`libs/langgraph/langgraph/pregel/_loop.py:650-652`) lacks a public signal channel; consumers must infer from `RunControl.drain_requested`.
- **Resume robustness.** `Command(resume=dict)` flow at `libs/langgraph/langgraph/pregel/_loop.py:890-919` doesn't revalidate that every interrupt id is still pending before applying; race with parallel interrupts could become ambiguous.
- **ToolNode input-shape drift.** `tool_node.py` accepts three input shapes (`list`, `dict`, `tool_calls`) with `ToolCallWithContext` discriminator (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1224-1266`); consolidating behind a single `parse_input` interface would shrink the `_parse_input`/`_extract_state` surface.

## Questions / Gaps

- **Where does the agent-level "turn counter" live?** There is no explicit `turn` integer in the framework; the closest analog is `PregelScratchpad.call_counter` referenced at `libs/langgraph/langgraph/pregel/_runner.py:720`. **No evidence found** for a single object that says "this was turn #N".
- **What does a "no tool calls" final AIMessage look like in the persisted record?** The last superstep produces an `AIMessage` write to the `messages` channel; but I did not find a dedicated "final message" hook distinct from `_put_pending_writes` (`libs/langgraph/langgraph/pregel/_loop.py:503-541`). The cleanest signal is the absence of new `tool_call` writes after `after_tick` (`libs/langgraph/langgraph/pregel/_loop.py:670-672`) and `loop.tasks` becoming empty on the next `tick` (`libs/langgraph/langgraph/pregel/_loop.py:646-648`).
- **How does `recursion_limit` interact with subgraphs?** Each subgraph owns its own `SyncPregelLoop`/`AsyncPregelLoop` (`libs/langgraph/langgraph/pregel/_loop.py:1446`, `:1698`) with its own step counter; nested subgraph limits are not unified. **No clear evidence found** for cross-subgraph budget enforcement.

---

Generated by `03.01-llm-turn-loop-structure` against `langgraph`.

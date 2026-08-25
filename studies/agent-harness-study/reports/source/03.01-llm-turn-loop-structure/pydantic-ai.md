# Source Analysis: pydantic-ai

## 03.01 LLM Turn Loop Structure

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai (Pydantic AI) |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (3.10–3.13); `uv` workspace; packages: `pydantic_ai_slim/`, `pydantic_graph/`, `pydantic_evals/`, `clai/` |
| Analyzed | 2026-07-13 |

## Summary

Pydantic AI models the agent loop as a typed graph, not a `while` loop. The loop is a `pydantic_graph.Graph` with four node types — `UserPromptNode`, `ModelRequestNode`, `CallToolsNode`, and `SetFinalResult` — declared in `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2130-2138` and driven by the generic `_GraphIterator` runtime in `pydantic_graph/pydantic_graph/graph_builder.py:646-825`. One agent turn is a `ModelRequestNode → CallToolsNode` hop; the loop terminates when `CallToolsNode` returns `End(FinalResult)` (`_agent_graph.py:1359-1379`) or `SetFinalResult` is dispatched from the streaming path (`agent/abstract.py:868`). The same 4-node graph is built for every `Agent` via `build_agent_graph` (`_agent_graph.py:2110-2139`), so the loop is reusable across agents and models — customization happens via `GraphAgentDeps` and `AbstractCapability` middleware, not by forking the loop.

A turn is persisted only as a message-history append: `_prepare_request` appends the `ModelRequest` (`_agent_graph.py:854`) and `_append_response` appends the `ModelResponse` (`_agent_graph.py:1015-1025`) into `GraphAgentState.message_history` (`_agent_graph.py:125`). Per-message `run_id`/`conversation_id` are stamped by `fill_run_metadata` (`_agent_graph.py:912, 969, 1021`) and the step counter lives at `state.run_step` (`_agent_graph.py:856`). There is no turn-level snapshot protocol for non-durable runs — durable turn persistence lives outside the slim package in `pydantic_ai_slim/pydantic_ai/durable_exec/{temporal,dbos,prefect}/`.

The loop can be drawn in three lines:

```
UserPromptNode ──▶ ModelRequestNode ──▶ CallToolsNode
                       ▲                       │
                       └──────── (ModelRequest)┘   (loop until final result)
                                                 │
                                                 └──▶ End(FinalResult)
```

…with `SetFinalResult` as a streaming-only terminal node (`_agent_graph.py:1385-1393`).

## Rating

**9 / 10** — A mature, generic, observable loop. The graph runner is a dedicated library (`pydantic_graph`) with a clear dispatcher (`_GraphIterator`, `graph_builder.py:646-825`), the agent loop is a single, named node set (`_agent_graph.py:2130-2138`), every turn is observable through structured state (`GraphAgentState` at `_agent_graph.py:122-144`) and message-history stamping, and the loop is reusable across every agent without copying (`build_agent_graph`, `_agent_graph.py:2110-2139`). Deductions: the loop's "turn" is a `ModelRequestNode` only — there is no built-in turn snapshot protocol for non-durable runs (only message history), the streaming and non-streaming code paths in `ModelRequestNode` are large parallel implementations (`_agent_graph.py:606-757` vs. `778-839`), and the wrapper around `wrap_model_request` (`_agent_graph.py:640-723`) carries hand-rolled async event hand-off that would benefit from being extracted into the `pydantic_graph` core.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Graph structure (4-node agent loop) | `g.add(g.edge_from(g.start_node).to(UserPromptNode), g.node(UserPromptNode), g.node(ModelRequestNode), g.node(CallToolsNode), g.node(SetFinalResult))` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2130-2138` |
| Graph builder entry | `def build_agent_graph(name, deps_type, output_type)` returns a generic `Graph[...]` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2110-2139` |
| Per-run state (turn identity, history, usage, retries, step) | `class GraphAgentState` dataclass | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:122-180` |
| Turn model call wrapper (non-streaming) | `ModelRequestNode._make_request` builds `ModelRequestContext`, calls `ctx.deps.root_capability.wrap_model_request(...)` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:778-839` |
| Turn model call wrapper (streaming) | `ModelRequestNode.stream` async-contextmanager; `_streaming_handler` wraps `req_ctx.model.request_stream(...)` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:606-757` |
| Step counter increment per turn | `ctx.state.run_step += 1` (after request append, before wrap) | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:856` |
| Message assembly (system + user + instructions) | `UserPromptNode.run`, `_sys_parts`, `_get_instructions`, `_prepare_request_parameters` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:270-447, 473-537` |
| Per-turn history append (request) | `ctx.state.message_history.append(self.request)` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:854` |
| Per-turn history append (response) | `_append_response(ctx, response)` appends to `message_history` and updates `usage` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1015-1025` |
| Run/conversation ID stamping | `fill_run_metadata(messages[-1], run_id=..., conversation_id=...)`; defaults to `state.run_id`, `state.conversation_id` (UUID7) | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:912, 969, 1021` |
| Final output condition | `CallToolsNode._handle_final_result` returns `End(final_result)` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1359-1379` |
| Streaming finalization node | `class SetFinalResult(AgentNode): async def run(...) -> End(self.final_result)` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1384-1393` |
| Retry node construction | `ModelRequestNode._build_retry_node` consumes `output_retries`, builds a fresh `ModelRequestNode(parts=[RetryPromptPart])` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1027-1040` |
| Multi-tool calls per turn (parallel / sequential) | `_call_tools` with `parallel_execution_mode ∈ {sequential, parallel, parallel_ordered_events}` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1862-1965` |
| Output tool dispatch | `process_tool_calls` validates + executes output tools, tracks `output_retries_used`, yields events | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1537-1859` |
| Loop driver (graph iterator) | `_GraphIterator.iter_graph` schedules tasks via `task_group.start_soon`, yields `EndMarker` / `Sequence[GraphTask]` | `pydantic_graph/pydantic_graph/graph_builder.py:646-825` |
| Public async iteration | `AgentRun.__anext__` advances via `await anext(self._graph_run)` | `pydantic_ai_slim/pydantic_ai/run.py:203-233` |
| Public manual stepping | `AgentRun.next(node)` runs `before_node_run → wrap_node_run → after_node_run` per node | `pydantic_ai_slim/pydantic_ai/run.py:330-404` |
| High-level run loop driver | `Agent.run` iterates `while not isinstance(node, End): node = await agent_run.next(node)` | `pydantic_ai_slim/pydantic_ai/agent/abstract.py:438-446` |
| Run-start graph entry point | `graph = _agent_graph.build_agent_graph(...)`; `graph_run = await graph.iter(inputs=user_prompt_node, state=state, deps=graph_deps)` | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1273, 1536-1543` |
| Turn persistence (graph state only — no per-turn snapshot in slim) | `state.message_history` mutated in place; `GraphAgentState` is the only persistence boundary | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:122-180, 854, 1025` |
| Durable persistence (external) | `pydantic_ai_slim/pydantic_ai/durable_exec/{temporal,dbos,prefect}/_agent.py` wrap `Agent.iter` for replay | `pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_agent.py:912-971`, `.../dbos/_agent.py:887-943`, `.../prefect/_agent.py:801-859` |
| Graph-level snapshot API (deprecated for new code) | `pydantic_graph.persistence.{FileStatePersistence,FullStatePersistence,SimpleStatePersistence}` | `pydantic_graph/pydantic_graph/persistence/file.py:31-66`, `pydantic_graph/pydantic_graph/persistence/in_mem.py:33-176` |
| Model call protocol | `Model.request(...)` and `Model.request_stream(...)` abstract methods | `pydantic_ai_slim/pydantic_ai/models/__init__.py:266-315` |
| End-to-end node sequence in docstring (worked example) | `UserPromptNode → ModelRequestNode → CallToolsNode → End(FinalResult)` | `pydantic_ai_slim/pydantic_ai/run.py:57-87` |
| End-to-end node sequence in docstring (worked example, `Agent.iter`) | Same node sequence | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1101-1133` |
| Graph iteration tests (turn-by-turn stepping) | `tests/graph/beta/test_graph_iteration.py::test_iter_basic` | `tests/graph/beta/test_graph_iteration.py:23-54` |
| Agent iter streaming tests | `tests/test_streaming.py::test_iter_stream_response`, `test_iter_stream_structured_output` | `tests/test_streaming.py:3007, 3407` |
| Architecture declaration | "`_agent_graph.py` owns loop orchestration: prompt assembly, model requests, tool/output processing, retries, usage checks, and finalization." | `agent_docs/pydantic-ai-slim.md:8` |

## Answers to Dimension Questions

1. **What happens during one turn?**
   A turn is one `ModelRequestNode → CallToolsNode` pair. `ModelRequestNode.run()` (`_agent_graph.py:593-604`) calls `_make_request` (`_agent_graph.py:778-839`), which (a) appends the assembled `ModelRequest` to `state.message_history`, (b) bumps `state.run_step`, (c) refreshes capability/tool-search state, (d) resolves tools via `ToolManager.for_run_step`, (e) resolves instructions via `_get_instructions`, (f) builds `ModelRequestParameters` via `_prepare_request_parameters`, (g) calls `Model.request()` (or `Model.request_stream()` for streaming) through the `wrap_model_request` middleware, (h) appends the `ModelResponse` via `_append_response`. `CallToolsNode.run()` (`_agent_graph.py:1067-1079`) then consumes the response: dispatches output tools, dispatches function tools in parallel (or sequentially), recovers text on empty responses, decides whether to return a `ModelRequestNode` (next turn) or `End(FinalResult)` (terminal).

2. **Does every turn call the model?**
   Yes — every `ModelRequestNode` calls the model exactly once through `Model.request()` / `Model.request_stream()`. The exceptions are documented short-circuits: `SkipModelRequest` (`_agent_graph.py:788-794, 617-632`), `ModelRetry` retry-node construction (`_agent_graph.py:1027-1040`), and the streaming-side `_SkipStreamedResponse` for the same short-circuit (`_agent_graph.py:540-577`). `usage.requests` is incremented only on real handler calls (`_agent_graph.py:624, 655, 716, 793, 837`).

3. **Can a turn include multiple tool calls?**
   Yes. `CallToolsNode._run_stream` calls `_handle_tool_calls` (`_agent_graph.py:1260-1298`) which iterates `process_tool_calls` (`_agent_graph.py:1537-1859`). Tool calls are partitioned by `kind` (`output`, `function`, `external`, `unapproved`, `unknown`) and run in parallel by default via `_call_tools` (`_agent_graph.py:1862-1965`) using `asyncio.create_task` and `asyncio.wait`. Sequential execution is selected when `parallel_execution_mode == 'sequential'` (`_agent_graph.py:1902-1913`), and `parallel_ordered_events` collects all events before yielding (`_agent_graph.py:1928-1933`).

4. **Is turn state persisted?**
   In the slim package, turn state is persisted only as the cumulative `GraphAgentState.message_history`. Every `ModelRequest` and `ModelResponse` is appended with `run_id` and `conversation_id` stamped (`_agent_graph.py:854, 1025, 912, 969, 1021`). `state.run_step` is incremented each turn (`_agent_graph.py:856`) and `state.usage` accumulates per turn (`_agent_graph.py:1022`). For durable per-turn snapshots, callers must integrate via `durable_exec/{temporal,dbos,prefect}/_agent.py` (e.g., `tests/test_temporal.py`, `tests/test_dbos.py`, `tests/test_prefect.py`). The legacy graph-level snapshot API at `pydantic_graph/persistence/` is still present but is described in `pydantic_graph/__init__.py:80-86` as no longer the recommended path for new code.

5. **Is the loop generic or agent-specific?**
   The loop is fully generic. `build_agent_graph` (`_agent_graph.py:2110-2139`) returns a `Graph[GraphAgentState, GraphAgentDeps[DepsT, OutputT], UserPromptNode, FinalResult[OutputT]]` parameterized only on `DepsT` and `OutputT`; every `Agent.iter()` call constructs one such graph and drives it (`agent/__init__.py:1273, 1536-1543`). Agent-specific behavior is injected through `GraphAgentDeps` (model, tool manager, output schema, validators, capabilities, native tools, model settings resolver, instruction resolver) — never by subclassing the loop. The graph runtime itself (`_GraphIterator`, `graph_builder.py:646-825`) is shared with all non-agent graphs built on `pydantic_graph` and is not agent-specific.

## Architectural Decisions

- **Graph-based loop instead of imperative `while`.** Choosing `pydantic_graph` (`pydantic_graph/graph_builder.py:646-825`) over a state machine baked into `_agent_graph.py` gives the loop declarative shape, debuggable `AgentRun.next_node`, free visualization via `Graph.render()` (`graph_builder.py:336-359`), and a generic fork/join engine for advanced flows.
- **Four-node turn, not two.** Splitting `ModelRequestNode` and `CallToolsNode` instead of folding them into one node lets the loop reuse the model call (streaming and non-streaming paths share `ModelRequestNode`; output validation lives in `CallToolsNode`) and exposes two natural insertion points for capability middleware (`wrap_model_request` vs. `after_model_request`).
- **State on a single dataclass.** `GraphAgentState` (`_agent_graph.py:122-180`) is the only mutable carry-over between nodes; `GraphAgentDeps` (`_agent_graph.py:183-221`) holds configuration. This keeps the graph runtime generic and the turn snapshot equivalent to "serialize `state`."
- **Capability middleware around the model call.** `wrap_model_request` (`_agent_graph.py:671-676, 816-820`) wraps the model call with a hand-off protocol (`stream_ready`/`stream_done` events at `_agent_graph.py:640-692`) so any capability (caching, retries, instrumentation, fallbacks) can intercept without modifying `_make_request`.
- **Tools as parallel fan-out, output as sequential validator.** `process_tool_calls` (`_agent_graph.py:1537-1859`) processes output tools strictly before function tools and dispatches function tools concurrently via `_call_tools` (`_agent_graph.py:1862-1965`) with three named execution modes.
- **Streaming as an async-contextmanager, not a side channel.** `ModelRequestNode.stream()` (`_agent_graph.py:606-757`) and `CallToolsNode.stream()` (`_agent_graph.py:1081-1091`) yield `AgentStream` / `HandleResponseEvent` iterators that the caller consumes inside the graph node's run; the cooperative hand-off with `_wrap_task` and `stream_ready`/`stream_done` events keeps cancellation safe.
- **Retry is a new node, not a flag.** `ModelRetry` produces a fresh `ModelRequestNode(parts=[RetryPromptPart])` via `_build_retry_node` (`_agent_graph.py:1027-1040`), and `output_retries_used` is checked against `max_output_retries` in `GraphAgentState.consume_output_retry` (`_agent_graph.py:162-179`).

## Notable Patterns

- **Graph as state machine.** `StartNode → UserPromptNode → ModelRequestNode ⇄ CallToolsNode → SetFinalResult → EndNode`. Edges are constructed with `g.edge_from(g.start_node).to(...)` (`_agent_graph.py:2130-2138`).
- **`_result` field on a node for "can't `return` from async iterator."** `ModelRequestNode._result` (`_agent_graph.py:587-589, 988-990, 1039`) caches the next node so streaming and non-streaming paths converge on the same continuation; `CallToolsNode._next_node` (`_agent_graph.py:1062-1064`) does the same.
- **History is the message store, not a log.** `_clean_message_history` (`_agent_graph.py:2220-2269`) merges consecutive same-role messages, so the model sees a normalized history even though the framework appends per-turn.
- **`new_message_index` delta tracking.** `_first_new_message_index` (`_agent_graph.py:2184-2202`) and `_is_same_request` (`_agent_graph.py:2205-2217`) make `AgentRun.new_messages()` (`run.py:174-179`) return only what this run produced, regardless of how many `ModelRequest`s got merged.
- **Cooperative hand-off between `wrap_model_request` and `stream()`.** `stream_ready` / `stream_done` events with `asyncio.wait` (`_agent_graph.py:640-692`) replace a generator-based protocol so cancellation drains both sides cleanly.
- **In-place mutation of shared sets.** `GraphAgentDeps.loaded_capability_ids` and `discovered_tool_names` are mutated in place (`_agent_graph.py:1431-1456`) rather than reassigned, to preserve identity across `replace(ctx, ...)`.
- **End-state terminator separation.** Two terminal paths: `End(FinalResult)` from `CallToolsNode._handle_final_result` (`_agent_graph.py:1359-1379`) for the non-streaming run path; `SetFinalResult` node (`_agent_graph.py:1384-1393`) for `Agent.run_stream`, which discovers the final result mid-stream (`agent/abstract.py:868`).

## Tradeoffs

- **No built-in turn snapshot inside the slim package.** Persistence is delegated to external adapters (`durable_exec/`); a caller using only `pydantic_ai_slim` and crashing between turns loses everything beyond the in-memory `message_history`.
- **Large parallel implementations for streaming vs. non-streaming.** `ModelRequestNode.stream()` (≈150 lines, `_agent_graph.py:606-757`) and `_make_request` (≈60 lines, `_agent_graph.py:778-839`) duplicate request preparation, hand-off, and error handling. A bug fix to one path may not flow to the other.
- **Hand-rolled async hand-off.** `stream_ready` / `stream_done` events (`_agent_graph.py:640-692`) are bespoke and require careful cancellation handling (`_cancel_task` at `_agent_graph.py:78-86`); this is harder to reason about than a single async generator.
- **Wrapper-heavy model call path.** `wrap_model_request` → `wrap_request_context` → `asyncio.create_task(...)` → `_streaming_handler` (`_agent_graph.py:646-676`) introduces three layers of indirection before the actual `req_ctx.model.request_stream(...)` call, trading simplicity for capability composability.
- **Graph runner is generic but low-overhead.** `_GraphIterator` (`graph_builder.py:646-825`) handles fork/join/reducers that the agent loop never uses; the agent graph is declared with `validate_graph_structure=False` (`_agent_graph.py:2139`), which trades structural validation for speed.

## Failure Modes / Edge Cases

- **Empty / thinking-only model response.** `CallToolsNode._run_stream` (`_agent_graph.py:1102-1173`) detects `is_empty` or `is_thinking_only`, checks `finish_reason == 'length'` (`_agent_graph.py:1111-1114`), checks content-filter rejection (`_agent_graph.py:1117-1130`), allows `None` when `output_schema.allows_none` (`_agent_graph.py:1132-1150`), attempts text recovery from history (`_agent_graph.py:1156-1164`), and otherwise resubmits with an empty `ModelRequest` to retry.
- **Truncated tool calls.** `GraphAgentState.check_incomplete_tool_call` (`_agent_graph.py:146-161`) raises `IncompleteToolCall` if the last response was `finish_reason='length'` and ended mid-tool-call.
- **Output-tool validation failure.** `process_tool_calls` (`_agent_graph.py:1583-1625`) records a `RetryPromptPart`, increments `output_retries_used`, and re-enters the loop with a fresh `ModelRequestNode`.
- **Per-tool retries vs. output retries.** `ToolManager._check_max_retries` is invoked inside `process_tool_calls`, while `output_retries_used` is consumed via `GraphAgentState.consume_output_retry` (`_agent_graph.py:162-179`) — both budgets must be exhausted for `UnexpectedModelBehavior` to fire.
- **Deferred / unapproved tool calls.** If `output_schema.allows_deferred_tools`, `process_tool_calls` (`_agent_graph.py:1849-1856`) terminates the run with `End(FinalResult(DeferredToolRequests(...)))`; the caller resumes by re-entering with `deferred_tool_results`.
- **Cancellation safety.** `ModelRequestNode.stream()` (`_agent_graph.py:729-756`) and `_call_tools` (`_agent_graph.py:1947-1956`) cancel sibling tasks via `cancel_and_drain` on `CancelledError` and any other `BaseException`.
- **Bare `async for node in agent_run` skips capability hooks.** `AgentRun.__anext__` (`run.py:203-233`) uses the graph's internal iterator without firing `wrap_node_run`; `Agent.run()` (`agent/abstract.py:438-446`) explicitly uses `agent_run.next(node)` to drive through the hooks.
- **Undrained pending messages.** Reaching `End` with non-empty `pending_messages` raises `UndrainedPendingMessagesError` (`run.py:221-232`).
- **User-prompt over unprocessed tool calls.** `UserPromptNode.run` rejects this with `UserError` (`_agent_graph.py:342-345`).

## Future Considerations

- **Unify streaming and non-streaming `ModelRequestNode` paths.** The duplicated prepare / wrap / finish logic between `_make_request` (`_agent_graph.py:778-839`) and `stream()` (`_agent_graph.py:606-757`) is the single largest source of accidental divergence; a unified async-contextmanager-with-optional-stream could halve the file.
- **Promote the cooperative hand-off to `pydantic_graph`.** `stream_ready` / `stream_done` events (`_agent_graph.py:640-692`) are essentially a sub-protocol for "task produces a value before it finishes"; making this a first-class primitive would let other graphs stream without re-implementing it.
- **Per-turn snapshot primitive for non-durable runs.** A `StateSnapshot` protocol on `GraphAgentState` (mirroring `durable_exec/{temporal,dbos,prefect}`) would let callers persist turn boundaries without external integration.
- **Graph-structure validation.** Disabling `validate_graph_structure` (`_agent_graph.py:2139`) is safe today because the agent graph is hardcoded, but if `capabilities/` ever adds runtime graph mutation the validation should be re-enabled or moved to a build-time check.
- **Trace boundaries.** OpenTelemetry spans appear to be created at the graph / node level (`graph_builder.py:881-883`), but `ModelRequestNode` and `CallToolsNode` each set `usage.requests` separately; ensure `requests` accounting lines up with span boundaries (e.g., `SkipModelRequest` must not double-count — see `_agent_graph.py:624, 655, 716, 793, 837` for current accounting).

## Questions / Gaps

- **Turn-level persistence boundary is not clearly documented.** The slim package's `GraphAgentState` is mutated in place; no public API exposes "snapshot of state at the end of turn N." Durable persistence is documented only as "see `durable_exec/`." (No clear evidence found in `_agent_graph.py` or `run.py` for a turn-snapshot protocol.)
- **`run_step` semantics under `SkipModelRequest`.** `state.run_step += 1` is unconditional in `_prepare_request` (`_agent_graph.py:856`), but `SkipModelRequest` short-circuits before the actual model call. It is unclear from the slim code alone whether `run_step` counts "turns attempted" or "turns completed" — `tests/test_agent.py` snapshots both but does not assert the relationship.
- **Concurrent agent runs sharing state.** `GraphAgentDeps` declares `loaded_capability_ids` and `discovered_tool_names` as identity-shared mutable sets (`_agent_graph.py:212-213, 1431-1456`). Two concurrent `Agent.iter()` calls on the same `Agent` would share these sets via `state`; the framework rejects this implicitly by constructing a fresh `GraphAgentDeps` per `iter()` (`agent/__init__.py:1480-1517`), but the invariant is enforced only by caller discipline, not by a type-level isolation guarantee.
- **Where the graph node hook system lives.** `before_node_run` / `wrap_node_run` / `after_node_run` are documented as part of `AgentRun.next` (`run.py:330-404`) but the implementation of the hook registry is in `pydantic_ai/capabilities/` and not surfaced in the search above; the agent-loop study cannot fully answer "how does a capability intercept the loop?" without inspecting `AbstractCapability` (out of scope for this dimension).

---

Generated by `dimension-03.01-llm-turn-loop-structure` against `pydantic-ai`.

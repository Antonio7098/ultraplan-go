# Source Analysis: pydantic-ai

## 01.02 - Control-Flow Ownership

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `sources/pydantic-ai` |
| Language / Stack | Python (>= 3.10) on a `pydantic_graph` graph-runtime foundation |
| Analyzed | 2026-07-02 |

## Summary

Pydantic AI runs a fixed four-node state machine on top of a third-party graph
runtime (`pydantic_graph`). The framework — not the LLM and not the user —
owns control flow. The "graph" is a static schema whose vertices are
`UserPromptNode`, `ModelRequestNode`, `CallToolsNode`, and `SetFinalNode`
(`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2130-2139`) and whose "edges" are
typed `BaseNode`/`End[...]` return unions that the framework inspects at
`Graph` construction (`pydantic_graph/pydantic_graph/basenode.py:104-136`). The
model never picks a next vertex by name; it emits a `ModelResponse` (text or
tool calls), and the framework's `CallToolsNode` decides between
`End[FinalResult]` (terminate), a fresh `ModelRequestNode` (retry / loop), or
`SetFinalResult` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1045-1381`).

The runtime can override, pause, reroute, and terminate the model via:

- A `Capability` middleware chain (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:486-536`,
  `pydantic_ai_slim/pydantic_ai/capabilities/combined.py:300-331`) that
  exposes `wrap_run`, `wrap_node_run`, `wrap_model_request`,
  `after_model_request`, `on_model_request_error`, `wrap_tool_validate`,
  `wrap_tool_execute`, etc. — every step in agent execution is hookable.
- Control-flow exceptions (`pydantic_ai_slim/pydantic_ai/exceptions.py:116-178`):
  `SkipModelRequest`, `ModelRetry`, `ToolRetryError`, `CallDeferred`,
  `ApprovalRequired`, `SkipToolValidation`, `SkipToolExecution`,
  `UndrainedPendingMessagesError`.
- An `end_strategy` literal (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:73`)
  in `{'early', 'graceful', 'exhaustive'}` that the runtime enforces inside
  `process_tool_calls` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1571-1576`,
  `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1666-1675`).
- Usage-limit, retry-budget, and concurrency gates
  (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:162-180`,
  `pydantic_ai_slim/pydantic_ai/tool_manager.py:177-181`,
  `pydantic_ai_slim/pydantic_ai/concurrency.py:79-220`).

Control flow is testable without an LLM via `TestModel`
(`pydantic_ai_slim/pydantic_ai/models/test.py:62`) and `FunctionModel`
(`pydantic_ai_slim/pydantic_ai/models/function.py:46`). Transitions are explicit
but the surface is large (four nodes × many state-machine branches inside each),
which is why the codebase carries a layered capability middleware plus
exception-driven control to keep behaviour configurable.

## Rating

**8/10 — Clear model with tests, explicit interfaces, and operational
safeguards.**

Rationale:

- The control surface is fully explicit at the type level (`BaseNode.run`
  return-type union is read by the framework, see
  `pydantic_graph/pydantic_graph/basenode.py:104-136`).
- There is a single source-of-truth graph for the agent loop, with explicit
  next-step selection in `CallToolsNode._run_stream`
  (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1093-1251`) and
  `ModelRequestNode._build_retry_node`
  (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1027-1040`).
- The framework can override model output at multiple layers
  (`after_model_request`, `wrap_model_request`, `on_model_request_error`,
  `on_tool_execute_error`, `on_output_validate_error`,
  `on_output_process_error`).
- Tests can drive the same loop without calling an LLM (`TestModel`, unit
  tests in `tests/graph/` and `tests/graph/beta/`).
- Weak points preventing a 9/10:
  - **`CallToolsNode._run_stream`** carries the heaviest control-flow
    branching in the framework and combines `_handle_tool_calls`,
    `_handle_text_response`, `_handle_image_response`, and a 19-arm
    `_handle_final_result` close (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1093-1380`).
    It is one of two functions explicitly suppressed from C901 complexity
    warnings (`# noqa: C901` at lines 1093 and 1537).
  - State-machine branches depend on `end_strategy` and on
    `ctx.deps.output_schema.allows_deferred_tools`, which are runtime-readable
    configuration rather than statically checked.
  - The newer graph-builder (`pydantic_graph/pydantic_graph/graph_builder.py`)
    ships in **beta** alongside the v1 `Graph`/`BaseNode` runtime
    (`tests/graph/beta/`). Tests assert `v1_v2_integration`, but the beta
    builder is not the runtime of the agent loop (the agent uses the v1
    schema — see `_agent_graph.py:2121-2139`).

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Static graph schema | `GraphBuilder` registers `UserPromptNode`, `ModelRequestNode`, `CallToolsNode`, `SetFinalResult` with `auto_instrument=False` and `validate_graph_structure=False`. The declared return types of each node's `run()` define the legal next vertices — the framework reads them via reflection. | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2110-2139`; `pydantic_graph/pydantic_graph/basenode.py:104-136`; `pydantic_graph/pydantic_graph/graph.py:509-526` |
| NextStep selector | `CallToolsNode._run_stream` decides termination via `End(final_result)`, retry via `ModelRequestNode(_messages.ModelRequest(parts=[...]))`, or fallback `RetryPromptPart` via `ToolRetryError`. | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1093-1251`; `_handle_final_result` at `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1359-1381`; `_build_retry_node` at `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1027-1040` |
| Tool-call dispatch | `process_tool_calls` partitions `ToolCallPart`s by `kind` (output / function / external / unapproved / unknown), routes them through `ToolManager` hooks, and merges outcomes via `output_parts` and a `deque(maxlen=1)` of `FinalResult`. Parallel execution mode (`'parallel' | 'sequential' | 'parallel_ordered_events'`) is resolved inside the call. | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1537-1860`; `pydantic_ai_slim/pydantic_ai/tool_manager.py:38-117`; `_call_tools` at `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1862-1966` |
| Termination evaluator | `_handle_final_result` returns `End(final_result)`; `SetFinalResult.run` returns `End(self.final_result)`; `_narrow_tool_call_parts` adapts the typed response to the schema. | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1359-1381`; `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1384-1393`; `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2142-2173` |
| End-strategy enforcement | Literal `EndStrategy = Literal['early', 'graceful', 'exhaustive']`; logic in `process_tool_calls` short-circuits remaining output / function tool calls once a final result exists. | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:73`; `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1571-1576`; `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1666-1674` |
| Model retry / override | `wrap_model_request` / `after_model_request` raise `ModelRetry`; the framework converts that into a fresh `ModelRequestNode` via `_build_retry_node`. The handler is called inside an `asyncio` task using a `stream_ready` / `stream_done` cooperative hand-off. | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:638-757`; `_build_retry_node` at `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1027-1040` |
| Capability middleware (control authority) | `AbstractCapability` exposes `wrap_run`, `wrap_node_run`, `before_node_run`, `after_node_run`, `on_node_run_error`, `wrap_model_request`, `before_model_request`, `after_model_request`, `on_model_request_error`, `wrap_tool_validate`, `wrap_tool_execute`, `after_tool_execute`, `on_tool_execute_error`, `wrap_output_validate`, `on_output_validate_error`, `wrap_output_process`, `on_output_process_error`, `handle_deferred_tool_calls`. `CombinedCapability.wrap_node_run` builds a middleware chain from the list of capabilities (outermost wraps innermost). | `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:405-947`; middleware composition at `pydantic_ai_slim/pydantic_ai/capabilities/combined.py:247-401`; `pydantic_ai_slim/pydantic_ai/capabilities/combined.py:690-747` |
| Control-flow exceptions | `ModelRetry`, `SkipModelRequest`, `SkipToolValidation`, `SkipToolExecution`, `ToolRetryError`, `CallDeferred`, `ApprovalRequired`, `UndrainedPendingMessagesError` are caught in the call site and translated into graph transitions (next node / retry / End). | `pydantic_ai_slim/pydantic_ai/exceptions.py:40-178`; surface area in `pydantic_ai_slim/pydantic_ai/_agent_graph.py:603-839`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1100-1251`, `pydantic_ai_slim/pydantic_ai/tool_manager.py:177-181` |
| Retry budgets | Per-tool retries tracked on `RunContext.retries[name]` with `_check_max_retries`; output retries tracked on `GraphAgentState.output_retries_used` via `consume_output_retry` which raises `UnexpectedModelBehavior` once a budget is exceeded. `max_output_retries` is a separate gate from per-tool `max_retries`. | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:162-180`; `pydantic_ai_slim/pydantic_ai/tool_manager.py:177-181`; `pydantic_ai_slim/pydantic_ai/_run_context.py:60-76` |
| Concurrency gate | `Agent._concurrency_limiter` is acquired in `iter` before any node runs; `ConcurrencyLimiter.acquire` uses `anyio.Lock` with waiting-task tracking and backpressure on `max_queued`. Model-side concurrency is layered on top. | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1531-1534`; `pydantic_ai_slim/pydantic_ai/concurrency.py:79-263`; `pydantic_ai_slim/pydantic_ai/models/concurrency.py:85-107` |
| Handoff to streaming | Inside `run_stream`, after `FinalResultEvent` is yielded, `agent_run.next(_agent_graph.SetFinalResult(final_result))` drives the graph to `End[FinalResult]` — `SetFinalResult.run` returns `End(self.final_result)` directly. | `pydantic_ai_slim/pydantic_ai/agent/abstract.py:838-880`; `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1384-1393` |
| Graph-level termination objects | v1 graph: `pydantic_graph.End[RunEndT]` is the only terminal value, set inside `GraphRun.next`; `next_task` is the runtime's view. v2 beta graph: `EndMarker[OutputT]`, `ErrorMarker`, `JoinItem` distinguish completion, error, and fork-join data flow. | `pydantic_graph/pydantic_graph/graph.py:632-753`; `pydantic_graph/pydantic_graph/graph_builder.py:111-164` |
| Recovery / override | v2 `GraphRun.override_next(Sequence[GraphTaskRequest] | EndMarker[OutputT])` lets a hook redirect after a node errors or returns `End`. After-call sites reach it via `AgentRun._sync_graph_state`. The yield-based `_GraphIterator.iter_graph` echoes `ErrorMarker` errors back so the caller can override. | `pydantic_graph/pydantic_graph/graph_builder.py:559-577`, `pydantic_graph/pydantic_graph/graph_builder.py:646-820`; `pydantic_ai_slim/pydantic_ai/run.py:255-310` |
| Async cancellation authority | Tasks in `_CallToolsNode._call_tools` are drained via `cancel_and_drain` if the loop is cancelled or any sibling raises; the streaming path in `ModelRequestNode.stream` similarly drains `wrap_task` and `_streaming_handler` on outer cancellation. | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1947-1956`; `pydantic_ai_slim/pydantic_ai/_agent_graph.py:681-723`; `pydantic_ai_slim/pydantic_ai/_utils.py` (cancel_and_drain) |
| Test hooks | `TestModel` (no LLM) and `FunctionModel` (user-driven) are first-class `Model` implementations; `TestModel.call_tools='all'` exercises the tool-dispatch path. Tests in `tests/graph/beta/` exercise error-recovery via `override_next` directly. | `pydantic_ai_slim/pydantic_ai/models/test.py:62-554`; `pydantic_ai_slim/pydantic_ai/models/function.py:46`; `tests/graph/beta/test_graph_iteration.py:469-577` |
| Persistence / durable execution | `GraphRun` accepts a `BaseStatePersistence` for snapshot/restore; `durable_exec/{prefect,temporal,dbos}/` keep their hooks at the toolset / tool-call layer (not the graph layer) so the run can be re-driven externally. | `pydantic_graph/pydantic_graph/persistence/__init__.py:113-136`; `pydantic_graph/pydantic_graph/graph.py:594-609`; `pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_function_toolset.py:43-69` |
| Pending-message queue | `ctx.state.pending_messages` is drained at deterministic points: `'asap'` before model request, `'when_idle'` at end of run. `Enqueue` allows user-side injection that is read by capability middleware. | `pydantic_ai_slim/pydantic_ai/_enqueue.py`; `pydantic_ai_slim/pydantic_ai/run.py:436-469`; `pydantic_ai_slim/pydantic_ai/_agent_graph.py:128:142` |

## Answers to Dimension Questions

1. **Who decides what happens next?**
   The framework. `BaseNode.run` returns a typed union whose members are the only
   legal next vertices (`pydantic_graph/pydantic_graph/basenode.py:104-136`).
   The model's output never names the next vertex — `ModelRequestNode.run`
   always returns a `CallToolsNode` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:988-990`),
   and `CallToolsNode._run_stream` picks the next vertex from a 4-way
   decision (tool calls / image output / text output / empty / thinking-only)
   plus the closure state
   (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1093-1251`).
   `End[FinalResult]` is the only termination symbol
   (`pydantic_graph/pydantic_graph/basenode.py:143-167`).

2. **Can the LLM bypass runtime control?**
   No. `Model.request` / `Model.request_stream` runs inside
   `wrap_model_request`, which capabilities can short-circuit
   (`pydantic_ai_slim/pydantic_ai/capabilities/combined.py:373-384`). The
   response is fed through `after_model_request` before being treated as a
   `ModelResponse` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:966-990`),
   `ModelRetry` is converted into a `RetryPromptPart` and a fresh
   `ModelRequestNode` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1027-1040`),
   and unknown tool names are converted into a `RetryPromptPart`
   (`pydantic_ai_slim/pydantic_ai/tool_manager.py:356-370`). Tool calls for
   tools that don't exist never run; they retry until the per-tool or
   output budget is exhausted
   (`pydantic_ai_slim/pydantic_ai/tool_manager.py:177-181`,
   `pydantic_ai_slim/pydantic_ai/_agent_graph.py:162-180`).

3. **Can the runtime reject or rewrite the next action?**
   Yes. Capabilities may rewrite `before_model_request` results, raise
   `SkipModelRequest`, raise `ModelRetry`, replace the response in
   `after_model_request`, and rewrite tool args in `wrap_tool_validate` /
   `wrap_tool_execute`. `wrap_run` / `wrap_node_run` may also redirect
   control flow: returning a different node from `after_node_run` rewrites
   the transition
   (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:467-484`,
   `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:486-536`).
   v2 graph exposes `override_next` so a hook can redirect after an
   error or `End` (`pydantic_graph/pydantic_graph/graph_builder.py:559-571`).

4. **Are transitions explicit or scattered?**
   Mostly explicit. The graph schema is explicit
   (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2110-2139`); node return
   types pin transitions. Within `CallToolsNode._run_stream`, however, the
   transition logic is scattered across a `_run_stream` async generator with
   inline branches for empty / thinking-only / text / image / output tools
   (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1093-1251`). The
   `process_tool_calls` function is the explicit dispatcher
   (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1537-1860`) — it is the
   single, named place where the kinds of tool calls (output / function /
   external / unapproved / unknown) are partitioned and routed.

5. **Is control flow testable without calling an LLM?**
   Yes. `TestModel` and `FunctionModel` are real `Model` subclasses and the
   graph runtime never needs an LLM. `tests/graph/beta/test_graph_iteration.py`
   exercises `next_task`, `override_next`, and error recovery directly
   (`tests/graph/beta/test_graph_iteration.py:147-180`,
   `tests/graph/beta/test_graph_iteration.py:469-577`). The repo's testing
   conventions document `TestModel` as the canonical non-LLM harness
   (`tests/AGENTS.md`).

## Architectural Decisions

- **Static, type-checked graph schema.** Control-flow vertices are dataclass
  `BaseNode` subclasses with declared `run` return unions; the framework
  parses the return-type union at build time
  (`pydantic_graph/pydantic_graph/basenode.py:104-136`,
  `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2121-2139`). Adding a node or
  a new branch requires updating the type, so misrouted transitions are a
  type-check error rather than a runtime assertion.
- **One runtime loop, many hooks.** The agent loop is fixed
  (`UserPromptNode → ModelRequestNode ⇄ CallToolsNode → SetFinalResult |
  End[FinalResult]`). All cross-cutting concern — retries, telemetry,
  moderation, caching, capability loading — is implemented as a `Capability`
  in the middleware chain
  (`pydantic_ai_slim/pydantic_ai/capabilities/combined.py:247-401`).
- **Async generator as the dispatch primitive.** `CallToolsNode._run_stream`
  is an async generator that yields events and assigns `_next_node` as a
  side effect; `set_final_result` is achieved by `End` return
  (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1045-1080`,
  `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1384-1393`). Async
  generators cannot return values, so the framework writes to instance
  attributes (`_next_node`, `_stream_error`) and unwinds through the
  caller.
- **Cooperative hand-off for streaming.** `ModelRequestNode.stream` runs the
  model on a task and waits for a `stream_ready` event before yielding the
  `AgentStream`. Outer cancellation drains both tasks before re-raising
  (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:638-757`).
- **Two parallel budget gates.** Per-tool retries track
  `RunContext.retries[name]` and terminate via
  `ToolManager._check_max_retries`
  (`pydantic_ai_slim/pydantic_ai/tool_manager.py:177-181`). Output-validation
  retries track `GraphAgentState.output_retries_used` and terminate via
  `consume_output_retry` / `UnexpectedModelBehavior`
  (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:162-180`).

## Notable Patterns

- **Middleware-with-handler pattern.** Every layer that runs code on behalf
  of the agent exposes `wrap_<stage>` methods that take a `handler`
  callable; the capability chain builds handler-passing closures
  (`pydantic_ai_slim/pydantic_ai/capabilities/combined.py:247-401`,
  `pydantic_ai_slim/pydantic_ai/capabilities/combined.py:690-747`,
  `pydantic_ai_slim/pydantic_ai/capabilities/hooks.py:882-987`). This pattern
  matches Python `asyncio`-style decorator stacks and is reusable across
  run / node / model / output / tool stages.
- **Exception-driven control.** The framework recognises a closed set of
  control-flow exceptions and translates them into graph transitions
  (`pydantic_ai_slim/pydantic_ai/exceptions.py:40-178`,
  `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1100-1251`,
  `pydantic_ai_slim/pydantic_ai/tool_manager.py:330-352`).
- **`End`-typed graph schemas.** Both the v1 `Graph.run` API
  (`pydantic_graph/pydantic_graph/basenode.py:143-167`) and the v2 builder
  (`pydantic_graph/pydantic_graph/graph_builder.py:111-141`) use a
  discriminated terminal value. v2 adds `ErrorMarker` and `JoinItem` for
  recovery and fork-join data flow, so error recovery and parallel flow are
  also type-named rather than ad-hoc.
- **`RunContext` as the unified per-step view.** Every hook and every node
  receives a `RunContext[DepsT]` constructed from the graph runner's
  state (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1396-1428`). This is
  the single place where deps, messages, usage, model settings, tool
  manager, capabilities, loaded-capability ids, and pending-message queue
  are visible — making the loop testable in isolation.
- **Tool call classification drives routing.** `process_tool_calls`
  classifies each `ToolCallPart` by kind — output / function / external /
  unapproved / unknown (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1553-1560`)
  — before deciding what to do with it. This is a co-located, explicit
  router with no implicit fallbacks.

## Tradeoffs

- **Explicit schema over flexibility.** The fixed four-node graph minimises
  control-flow surprise but means that branching outside the graph must
  route through capability middleware or persistence. A user who wants a
  dual-loop "plan → critique → refine" pattern must do it inside the LLM
  message history, via tool return content, or via a custom graph (the
  README documents `pydantic_graph` as the lower-level primitive for
  this — `pydantic_graph/README.md`).
- **Heavy function-level complexity.** `CallToolsNode._run_stream` and
  `process_tool_calls` carry dozens of branches and are suppressed from
  C901 warnings (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1093`,
  `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1537`). These are the only
  places where the framework's control-flow decisions are concentrated; any
  optimisation or feature change has to thread through them.
- **Capability ordering is configuration.** `CapabilityOrdering` accepts
  `outermost` / `innermost` / `wraps` / `wrapped_by` / `requires`
  (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:100-142`). It is
  observed via reflection at `CombinedCapability` build time
  (`pydantic_ai_slim/pydantic_ai/capabilities/combined.py:62-63`,
  `pydantic_ai_slim/pydantic_ai/capabilities/_ordering.py:collect_leaves`),
  which means control flow can silently reorder if capability ordering
  metadata changes incompatibly. Mismatched ordering raises
  `Conflicting positions` (see comment at
  `pydantic_ai_slim/pydantic_ai/capabilities/combined.py:55-60`).
- **Two retry budgets at the same boundary.** Per-tool retries and
  output-validation retries are tracked separately and have separate
  boundaries. The interplay between them is documented and constrained
  (`pydantic_ai_slim/pydantic_ai/tool_manager.py:622-630`), but it is not
  always intuitive: e.g. `ToolOutput(max_retries=N)` exceeding
  `max_output_retries` can let `ctx.last_attempt` fire before the run
  terminates (`pydantic_ai_slim/pydantic_ai/tool_manager.py:622-630`).
- **Async-iterator-as-fsm.** Because Python async generators cannot return
  values, control state is written to instance attributes (`_next_node`,
  `_stream_error`, `_result_override`). Disassembling the state machine
  from this representation is non-trivial — the
  `agent_run.next(node)`/`async for` split in the code makes the two paths
  diverge semantically (`pydantic_ai_slim/pydantic_ai/run.py:189-233`).

## Failure Modes / Edge Cases

- **Model emits no actionable output.** `CallToolsNode._run_stream` raises
  `UnexpectedModelBehavior` for finish_reason='length' on an empty
  response, `ContentFilterError` for content-filter refusals, and otherwise
  attempts text recovery or a `ModelRequestNode(parts=[])` re-prompt
  (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1107-1173`).
- **Tool-call truncated mid-emit.** Detected by
  `GraphAgentState.check_incomplete_tool_call`
  (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:146-161`) and re-raised as
  `IncompleteToolCall` with a hint to raise `max_tokens`
  (`pydantic_ai_slim/pydantic_ai/exceptions.py:308-309`).
- **Tool call for an unknown tool.** `ToolManager._resolve_tool` raises
  `ModelRetry`, which is wrapped into `ToolRetryError` and counted against
  `ToolManager._check_max_retries`
  (`pydantic_ai_slim/pydantic_ai/tool_manager.py:356-370`,
  `pydantic_ai_slim/pydantic_ai/tool_manager.py:177-181`).
- **Concurrent cancellation.** Outer cancellation during streaming drains
  both the `wrap_task` and the readiness waiter before re-raising. Parallel
  tool execution drains via `cancel_and_drain` on `CancelledError` or any
  sibling exception
  (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1947-1956`).
- **`TestModel`/`FunctionModel` token models are not safe for budgeting.**
  The integration tests rely on VCR cassettes; token counts are estimates
  (`pydantic_ai_slim/pydantic_ai/models/function.py:_estimate_usage`). For
  production usage limits, real provider responses are required.
- **`undrained pending_messages`.** If a user iterates with bare `async for`
  while `'when_idle'` messages are queued, the run reaches `End` and raises
  `UndrainedPendingMessagesError`
  (`pydantic_ai_slim/pydantic_ai/exceptions.py:170-178`,
  `pydantic_ai_slim/pydantic_ai/run.py:221-232`).
- **`end_strategy='early'` short-circuits later outputs.** If
  `end_strategy='early'`, pending function tool calls after the first
  successful output tool are converted into synthetic "Final result
  processed" return parts and never run
  (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1666-1675`).
- **Streaming vs. node-hooks asymmetry.** Bare `async for agent_run` does
  not fire `wrap_node_run`; `agent.run` and `agent_run.next(...)` do
  (`pydantic_ai_slim/pydantic_ai/run.py:189-210`). Users who want hook
  coverage with streaming must use `agent.run(..., event_stream_handler=...)`
  or `AgentRun.next`.
- **Errors raised inside `wrap_run_event_stream`.** These propagate out of
  `agent_run.next(...)` via `_sync_graph_state` and re-raise on the next
  `__anext__` (v2 graph) or are surfaced by `ErrorMarker`
  (`pydantic_graph/pydantic_graph/graph_builder.py:646-820`,
  `pydantic_ai_slim/pydantic_ai/run.py:255-310`).

## Future Considerations

- **v2 (`graph_builder.py`) adoption.** The new graph builder has typed
  `Step`/`Join`/`Decision`/`Fork` primitives that make next-step control
  even more explicit (`pydantic_graph/pydantic_graph/graph_builder.py:382-396`,
  `pydantic_graph/pydantic_graph/decision.py`, `pydantic_graph/pydantic_graph/join.py`).
  The agent loop has not migrated; tests show v1/v2 integration is exercised
  but the runtime still uses the v1 schema
  (`tests/graph/beta/test_v1_v2_integration.py`,
  `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2121-2139`). A future
  migration would tighten the type-checkable surface and decouple tool-call
  classification from `process_tool_calls`.
- **Capability middleware ordering.** As capability graphs grow
  (precedences may conflict), ordering metadata (`outermost` /
  `innermost` / `wraps` / `wrapped_by`) will be load-bearing. Today, a
  conflict raises `Conflicting positions` during capability construction
  (`pydantic_ai_slim/pydantic_ai/capabilities/combined.py:55-60`); the
  ordering module could expose more ergonomic diagnostics.
- **Retry-budget unification.** `ToolOutput(max_retries=N)` overstepping
  `max_output_retries` already flagged as a tracked issue (#5238 in the
  comment at `pydantic_ai_slim/pydantic_ai/tool_manager.py:622-630`). A
  unified budget model would simplify a class of corner cases.
- **Cancel scopes for tool execution.** `_call_tools` cancels siblings only
  on caught exceptions (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1947-1956`)
  — successful-but-running siblings complete. This is current behaviour but
  is a candidate for stricter "all or nothing" semantics on a per-call-call
  basis.

## Questions / Gaps

- **Adapter parity between `request_stream` and `request`.** The dimension
  study calls out identical processing for streaming and non-streaming
  (`pydantic_ai_slim/pydantic_ai/models/AGENTS.md: rule 81`). Does
  `_narrow_tool_call_parts` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2142-2173`)
  ensure parity for every provider? The base appears to enforce it; provider
  adapters live in `pydantic_ai_slim/pydantic_ai/models/` (out of scope for
  this dimension).
- **Hook-firing under `AgentRun.override_next`.** v1 graph exposes
  `override_next` only in v2. The current agent loop runs on v1, so
  per-step overrides happen via `after_node_run` rewriting a different node
  (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:476-484`,
  `pydantic_ai_slim/pydantic_ai/run.py:255-310`). Future migration to v2
  may add a more granular override surface.
- **Latent bugs in `_run_stream`'s multi-arm dispatch.** The function is
  `# noqa: C901`; without a state-machine diagram it is hard to follow every
  path. A future version could split output / tool / text / image arms into
  separate helper functions while keeping the public behaviour identical.
- **Static guarantees on `end_strategy`.** `end_strategy` and
  `output_schema.allows_deferred_tools` are not statically enforced against
  the chosen output type; a runtime `UserError` is raised inside
  `process_tool_calls`
  (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1850-1856`). A future
  version could move this validation to graph build time.

---

Generated by `01.02-control-flow-ownership` against `pydantic-ai`.

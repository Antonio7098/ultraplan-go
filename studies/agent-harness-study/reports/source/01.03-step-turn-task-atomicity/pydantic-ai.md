# Source Analysis: pydantic-ai

## 01.03 Step, Turn, and Task Atomicity

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (Pydantic AI + pydantic_graph, async) |
| Analyzed | 2026-07-02 |

## Summary

Pydantic AI decomposes an agent run into three cleanly-layered, but differently-shaped, atomic units. The **graph node** (`pydantic_graph.BaseNode`) is the durable, traced, retry-able, and persisted unit; the **run step** (`ctx.state.run_step`, `GraphAgentState.run_step`) is a monotonically incrementing counter that scopes per-step tool lifecycle (`for_run_step`, retry budgets, capability refresh); and the **model request** (`ModelRequestNode`) plus the **tool call** (`ToolCallPart` / `_call_tool` in `CallToolsNode`) are sub-`step` units that have their own retry budgets, span/span hierarchy, and concurrency semantics. The system says exactly what completed and what did not: `NodeSnapshot.status` ∈ `{'created','pending','running','success','error'}`, `usage.requests`/`usage.tool_calls` counts, and per-tool `ctx.retries[name]` are the recorded partial-completion markers. Snapshots (`pydantic_graph.persistence`) only target graph *nodes*, not tool calls or model requests — those are observable through OpenTelemetry spans (`pydantic_ai._instrumentation`, `InstrumentationCap`), per-request `RequestUsage`, and the `HandleResponseEvent`/`ModelResponseStreamEvent` stream. Durable execution engines (Temporal, Prefect, DBOS) further map the atomic units to engine-specific constructs: Temporal activities wrap the model request (`TemporalModel.request_activity`), tool calls (`TemporalFunctionToolset.call_tool_activity`), and MCP I/O (`pydantic_ai/durable_exec/temporal/_*.py`); Prefect wraps them as Prefect `task`s (`pydantic_ai/durable_exec/prefect/_model.py:37-71`).

The model is **mature for `pydantic_graph`-native graphs** (8–9/10): `BaseStatePersistence` is an ABC with three concrete impls (`SimpleStatePersistence`, `FullStatePersistence`, `FileStatePersistence`) plus `SnapshotStatus` lifecycle, atomic `snapshot_node_if_new`, an explicit `record_run` context manager that sets status to `running`/`success`/`error` with `start_ts`/`duration`, and `load_next` for crash recovery. For the agent loop, the unit model is **clear, well-tested, and operationally rich** (8/10): graph nodes are snapshotted, every tool call is bounded by `ToolDefinition.max_retries`, output validation is bounded by `max_output_retries`, model requests are bounded by `usage_limits` and `tool_calls_limit`, and `GraphAgentState.check_incomplete_tool_call` (`_agent_graph.py:146-160`) detects a half-built `ToolCallPart` from a `finish_reason='length'` truncation. The remaining gap is that Pydantic AI does *not* persist `BaseStatePersistence`-style snapshots for the **agent** graph — `agent.run()`/`agent.iter()` does not call `snapshot_node` at all; only `pydantic_graph.Graph` users who pass their own `persistence=` get crash-recovery. The agent's recovery path is to re-derive state from `message_history`, which works but is not atomic in the snapshot sense.

## Rating

**8/10** — Clear model with explicit interfaces, two-layer atomicity (graph node + tool call), explicit retry budgets, lifecycle status, durable-engine wrappers, and observable partial-completion markers. Not 9–10 because (a) agent-loop nodes are *not* snapshotted via `BaseStatePersistence` by default; recovery relies on `message_history` reconstruction rather than node snapshot replay; (b) the three atomic units are coupled to one another and can drift — e.g. a mid-`ModelRequestNode` crash leaves the `ModelRequest` already appended to `state.message_history` (`_agent_graph.py:854`) but no `ModelResponse` and no tool-call result; (c) `run_step` is only on `GraphAgentState` and `RunContext`, not part of the snapshot schema.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Graph node atomic unit | `BaseNode` with `async run()` returning next `BaseNode` or `End`; abstract base for all nodes | `pydantic_graph/pydantic_graph/basenode.py:37-65` |
| Node classes (the four agent node types) | `AgentNode`, `UserPromptNode`, `ModelRequestNode`, `CallToolsNode`, `SetFinalResult` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:224-272, 540-604, 1045-1100, 1383-1394` |
| Agent graph definition | `build_agent_graph` joins UserPrompt → ModelRequest → CallTools → SetFinalResult | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2110-2139` |
| Per-step state & retry counter | `GraphAgentState.run_step`, `output_retries_used`, `usage`, `run_id`, `conversation_id` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:122-145` |
| Per-tool retry map | `RunContext.retries: dict[str, int]`, `max_retries`, `retry` (output-validation counter) | `pydantic_ai_slim/pydantic_ai/_run_context.py:60-77` |
| Per-step tool resolution | `ctx.state.run_step += 1` then `tool_manager = await tool_manager.for_run_step(run_context)` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:856, 872` |
| Per-step retries carried across steps | `ToolManager.for_run_step` propagates failed-tool counters | `pydantic_ai_slim/pydantic_ai/tool_manager.py:120-145` |
| Tool max-retries enforcement | `ToolManager._check_max_retries` raises `UnexpectedModelBehavior` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:177-181` |
| Output retry budget | `GraphAgentState.consume_output_retry`, called from retry nodes, empty-response paths, validator paths | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:162-179, 1036, 1146, 1171, 1248` |
| Incomplete-tool-call detection | `GraphAgentState.check_incomplete_tool_call` on `finish_reason='length'` truncated `ToolCallPart` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:146-160` |
| Tool-call concurrency within a step | `_call_tools` with `'parallel'`, `'sequential'`, `'parallel_ordered_events'` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1862-1965` |
| Per-call validation / execution split | `ValidatedToolCall`, `validate_tool_call`, `execute_tool_call` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:46-75, 419-465, 467-501` |
| Streamed event types (UI atomic unit) | `HandleResponseEvent = FunctionToolCallEvent \| FunctionToolResultEvent \| OutputToolCallEvent \| OutputToolResultEvent` discriminated by `event_kind` | `pydantic_ai_slim/pydantic_ai/messages.py:2806-2967` |
| Run-context propagation for the live node | `_CURRENT_RUN_CONTEXT` ContextVar + `set_current_run_context` so hooks/tools can read the current `RunContext` | `pydantic_ai_slim/pydantic_ai/_run_context.py:275-305` |
| Captured message history (partial-completion view) | `capture_run_messages` context manager + `_RunMessages` contextvar | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2055-2107` |
| Node-snapshot lifecycle statuses | `SnapshotStatus = 'created' \| 'pending' \| 'running' \| 'success' \| 'error'` | `pydantic_graph/pydantic_graph/persistence/__init__.py:32-41` |
| Snapshot dataclasses | `NodeSnapshot` (with `state`, `node`, `start_ts`, `duration`, `status`, `id`) and `EndSnapshot` | `pydantic_graph/pydantic_graph/persistence/__init__.py:44-103` |
| Persistence ABC | `BaseStatePersistence.snapshot_node`, `snapshot_node_if_new`, `snapshot_end`, `record_run`, `load_next`, `load_all` | `pydantic_graph/pydantic_graph/persistence/__init__.py:106-203` |
| Simple persistence | `SimpleStatePersistence.record_run` sets `running` then `success`/`error` with `perf_counter` duration | `pydantic_graph/pydantic_graph/persistence/in_mem.py:55-74` |
| File persistence | `FileStatePersistence.record_run` with file-based lock | `pydantic_graph/pydantic_graph/persistence/file.py:68-94` |
| Snapshot uniqueness | `generate_snapshot_id = f'{node_id}:{uuid4().hex}'` | `pydantic_graph/pydantic_graph/basenode.py:170-172` |
| `record_run` integration in `GraphRun.next` | Drives `node.run(ctx)` inside the `record_run` context manager and snapshots `End`/`BaseNode` afterwards | `pydantic_graph/pydantic_graph/graph.py:738-751` |
| `iter_from_persistence` for resume | Restores the next pending snapshot and resumes | `pydantic_graph/pydantic_graph/graph.py:260-309` |
| Agent hook-driven step iterator | `AgentRun.next()` → `_run_node_with_hooks` → `_wrap_and_advance` fires `before_node_run → wrap_node_run → after_node_run / on_node_run_error` | `pydantic_ai_slim/pydantic_ai/run.py:312-328, 267-310` |
| Capability node hooks | `before_node_run`, `wrap_node_run`, `after_node_run`, `on_node_run_error` | `pydantic_ai_slim/pydantic_ai/capabilities/hooks.py:115-128, 899-937` |
| Concurrency limit (per-run gate) | `ConcurrencyLimiter` via `get_concurrency_context` wrapping the whole agent run | `pydantic_ai_slim/pydantic_ai/concurrency.py:79-127, 247-273` |
| Usage limits (token / request / tool-call gates) | `UsageLimits` with `request_limit`, `tool_calls_limit`, `input_token_limit`, etc. | `pydantic_ai_slim/pydantic_ai/usage.py:275-419` |
| Per-request usage increments | `RunUsage.requests`, `RunUsage.tool_calls`, `RequestUsage` per call | `pydantic_ai_slim/pydantic_ai/usage.py:181-273` |
| Tool-call limit pre-flight | `_agent_graph.py:1705-1708` `projected_usage = deepcopy(usage); projected_usage.tool_calls += len(calls_to_run); check_before_tool_call(projected_usage)` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1705-1708` |
| Before-request usage check | `_prepare_request` calls `usage_limits.check_before_request(usage)` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:960` |
| Temporal model-request atomicity | `TemporalModel.request_activity` wraps `model.request(...)` in a Temporal activity; `request_stream_activity` wraps the stream | `pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_model.py:84-129, 131-166, 168-205` |
| Temporal tool-call atomicity | `TemporalFunctionToolset.call_tool_activity` wraps `self._call_tool_in_activity` per tool; configurable `tool_activity_config` | `pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_function_toolset.py:43-105` |
| Temporal MCP-call atomicity | `TemporalMCPServer.call_tool_activity` and `get_tools_activity` | `pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_mcp.py:87-184` |
| Temporal dynamic-toolset atomicity | `TemporalDynamicToolset.get_tools_activity` + `call_tool_activity` | `pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_dynamic_toolset.py:81-148` |
| Prefect model-request atomicity | `PrefectModel.request_task` and `request_stream_task` | `pydantic_ai_slim/pydantic_ai/durable_exec/prefect/_model.py:37-110` |
| Prefect function-toolset atomicity | `PrefectFunctionToolset._call_tool_task` | `pydantic_ai_slim/pydantic_ai/durable_exec/prefect/_function_toolset.py:28-60` |
| Retry caches on the run context | `RunContext.cache_key_for_tool_call` style fields via cache policies in Prefect | `pydantic_ai_slim/pydantic_ai/durable_exec/prefect/_cache_policies.py:39-41` |
| Tests: graph persistence round-trip | `test_next_from_persistence`, `test_full_state_persistence_snapshot_state_stability` | `tests/graph/test_persistence.py:297-328, 349-378` |
| Tests: per-tool retry propagation across steps | `tests/test_agent.py:8125` "Per-tool retry counts populated by `for_run_step` propagate too" | `tests/test_agent.py:8125-8135` |
| Tests: `run_step` counter behavior | `tests/test_agent.py:7881` `assert run_result._state.run_step == 3` | `tests/test_agent.py:7873-7884` |
| Tests: incomplete tool-call detection | `tests/test_agent.py:11072` "Enforcement runs through `ToolManager._check_max_retries` (per-tool), not `consume_output_retry`" | `tests/test_agent.py:11072` |
| Tests: per-step dynamic toolset re-evaluation | `tests/test_agent.py:7873-7884` | `tests/test_agent.py:7873-7884` |
| Tests: Temporal retry round-trip | `tests/test_temporal.py:2508-2668` `test_temporal_agent_with_model_retry` | `tests/test_temporal.py:2508-2668` |

## Answers to Dimension Questions

### 1. What is the atomic unit of execution?

There are **four co-existing atomic units**, each with different responsibilities:

- **Graph node** (`pydantic_graph.basenode.BaseNode` subclasses, including the four agent nodes `UserPromptNode`, `ModelRequestNode`, `CallToolsNode`, `SetFinalResult` at `_agent_graph.py:247-272, 540-604, 1045-1100, 1383-1394`). This is the durable, traced, and persistence-target unit. Every node has a stable `snapshot_id` (`get_snapshot_id`, `basenode.py:67-76`) and is wrapped in `BaseStatePersistence.record_run` (`graph.py:738-751`).
- **Run step** (`GraphAgentState.run_step`, `_agent_graph.py:128`). A monotonic counter, incremented in `ModelRequestNode._prepare_request` (`_agent_graph.py:856`). It scopes per-step tool lifecycle: `ToolManager.for_run_step` is a no-op for the same step (`tool_manager.py:122-124`) and only re-evaluates dynamic toolsets and propagates per-tool retry counters when `run_step` advances.
- **Model request** (`ModelRequestNode.run` → `_make_request` / `stream`, `_agent_graph.py:778-839, 606-757`). One model API round-trip (streaming or non-streaming). Bounded by `usage_limits.request_limit` (`usage.py:415-419`) and counted by `ctx.state.usage.requests += 1` (`_agent_graph.py:624, 716, 793, 837`).
- **Tool call** (`ToolCallPart`, executed by `_call_tool` at `_agent_graph.py:1984-2052`). Bounded by per-tool `max_retries` (`ToolDefinition.max_retries`, `_agent_graph.py:177-181`); counted by `usage.tool_calls += 1` (`tool_manager.py:735, 767`); validated and executed as separate atomic steps via `ValidatedToolCall` (`tool_manager.py:46-75`).

Tool calls within one `CallToolsNode` can run **concurrently** via `_call_tools` (`_agent_graph.py:1862-1965`), which is itself an explicit atomic scope — its `_call_tools` function-level yields `FunctionToolCallEvent`/`FunctionToolResultEvent` pairs (`messages.py:2836-2921`) to signal start/end.

### 2. Is the atomic unit the same for persistence, tracing, retry, and UI?

**No — the units differ by concern, and they are deliberately layered:**

| Concern | Atomic unit | Where defined |
|---|---|---|
| Persistence (graph) | Graph node | `BaseStatePersistence.snapshot_node` / `record_run` |
| Persistence (durable engine) | Temporal **activity** or Prefect **task** (model request, tool call, MCP call, get_tools) | `durable_exec/temporal/_*.py`, `durable_exec/prefect/_*.py` |
| Tracing (OTel spans) | Model request → tool call (nested). `InstrumentationCap` wraps `wrap_model_request`/`wrap_tool_execute`; per-tool-call spans live inside the request | `_instrumentation.py`, capabilities/hooks |
| Retry (model side) | Model request — `max_output_retries` budget on `GraphAgentState.output_retries_used` | `_agent_graph.py:162-179` |
| Retry (tool side) | Per-tool — `ctx.retries[name]`, `max_retries` | `tool_manager.py:177-181, 348-350` |
| UI / event stream | `HandleResponseEvent` (tool start/end) + `ModelResponseStreamEvent` (part start/delta/end/final result) | `messages.py:2806-2967` |
| Concurrency gating | Whole agent run | `ConcurrencyLimiter` |
| `for_run_step` lifecycle | Run step | `ToolManager.for_run_step`, `DynamicToolset.per_run_step` |

The agent-loop drives the graph **node by node**, but inside a `CallToolsNode` it dispatches tool calls **per call**. So a node is atomic at the graph/persistence level, but at the tool-execution level each tool call is its own atomic unit that runs in parallel and counts against its own retry budget.

### 3. Can partially completed steps exist?

**Yes, by design.** The system is explicit about partial completion:

- A `ModelRequestNode` may complete streaming the response and crash before `_finish_handling` appends it. The `ModelRequest` is *already* appended to `state.message_history` in `_prepare_request` (`_agent_graph.py:854`), but no `ModelResponse`. Recovery has to detect this — `check_incomplete_tool_call` (`_agent_graph.py:146-160`) catches the case where a `finish_reason='length'` truncates a `ToolCallPart` and refuses to call `args_as_dict`.
- A `CallToolsNode` may complete some tool calls but not others; the function is split into a validate-phase and an execute-phase (`_agent_graph.py:1715-1734, 1736-1745`), and `ValidatedToolCall.args_valid` (`tool_manager.py:46-75`) is the recorded marker. Within `_call_tools`, `parallel_execution_mode='parallel'` keeps a pending set and lets some tasks complete while others fail; on `BaseException`, sibling tasks are `cancel_and_drain`'d (`_agent_graph.py:1947-1956`).
- `CallToolsNode.stream` keeps `_stream_error` separately (`_agent_graph.py:1065, 1077-1079`) so that an exception caught by an external consumer (e.g. `UIEventStream.transform_stream`) doesn't get masked by an assertion about `_next_node` not being set.
- Streaming-mode cancellation: `CancelAndDrain` (`_agent_graph.py:686-693`) cancels the in-flight `wrap_task` plus the readiness waiter, drains them, and re-raises — explicitly documented to "drain both tasks so cleanup runs before we re-raise."
- `_did_stream` flag on `ModelRequestNode` (`_agent_graph.py:590, 601-603`) prevents a re-entrant `run()` after `stream()` was started.

The system **does** say exactly what completed: `NodeSnapshot.status`, `RunUsage.requests`/`tool_calls`, per-tool `ctx.retries[name]`, and per-step `output_retries_used`. It does *not*, however, snapshot the agent-graph node state by default — see "Failure Modes" below.

### 4. What happens if a crash occurs mid-step?

Three layers of behavior:

1. **`pydantic_graph.Graph` with a `BaseStatePersistence`**: `record_run` is wrapped in a `try/except` that sets `NodeSnapshot.status='error'` and records `duration` (`in_mem.py:67-71`). On recovery, `load_next` returns the next `'created'` snapshot, and `iter_from_persistence` resumes from it (`graph.py:260-309`). The state field on the snapshot is deep-copied at snapshot time (`in_mem.py:101-105, 167-171`).
2. **Agent loop without persistence**: a mid-`ModelRequestNode` crash leaves the partial `ModelRequest` already in `state.message_history`. The user can recover by calling `agent.run(message_history=...)` with the saved messages; `GraphAgentState.run_id` and `conversation_id` are stamped on every request/response (`_agent_graph.py:912, 969` and `fill_run_metadata`), so `AgentRun.new_messages()` correctly partitions new from prior via `_first_new_message_index` (`_agent_graph.py:2184-2202`). This is recovery by *message-history reconstruction*, not by node-snapshot replay.
3. **Temporal/DBOS/Prefect**: each model request and each tool call is a separate activity/task with its own retry policy (`TemporalAgent.activity_config['retry_policy']`, `_agent.py:130-136`), so a crashed activity is retried in-place by the engine. The agent-graph `run_step`/`run_id` survives the activity because it's serialized into the `TemporalRunContext` payload (`durable_exec/temporal/_run_context.py:20-64`).

### 5. Are tool calls their own atomic units?

**Yes — at the concurrency, retry, and durable-execution levels.** Specifically:

- `_call_tools` (`_agent_graph.py:1862-1965`) treats each `ToolCallPart` as its own unit of execution with three concurrency modes (`parallel`, `sequential`, `parallel_ordered_events`).
- `validate_tool_call` and `execute_tool_call` are split (`tool_manager.py:419-501, 467-501`) so that a caller can decide whether a validated-but-not-executed call counts as "started" for retry budget purposes.
- Each tool call is bounded by `ToolDefinition.max_retries` and tracked by `ctx.retries[name]`; `failed_tools` is the per-step failed set carried to the next step (`tool_manager.py:90, 127-129`).
- Each tool call yields a `FunctionToolCallEvent` on entry and a `FunctionToolResultEvent` on exit (`_agent_graph.py:1718-1734, 1900`), giving the UI a per-call start/end marker.
- Temporal wraps each tool call in its own activity (`durable_exec/temporal/_function_toolset.py:43-104`) with per-tool `ActivityConfig`, so a single tool-call crash retried independently of the model request that produced it. Prefected and DBOS equivalents match this pattern (`durable_exec/prefect/_function_toolset.py:28-60`, `durable_exec/dbos/`).

So: at the **graph node** level the atomic unit is one node; at the **tool execution** level the atomic unit is one call; at the **durable-engine** level those collapse back together (or are collapsed, by config, to whole-node activities). The `for_run_step` hook is the per-step bridge that lets toolsets evaluate once per request step.

## Architectural Decisions

- **Graph nodes are the first-class durable unit; agent runs above the graph.** Pydantic AI builds an agent graph out of `pydantic_graph.Graph` (`build_agent_graph`, `_agent_graph.py:2110-2139`) and reuses `pydantic_graph`'s persistence, tracing, and node-snapshot machinery. Custom agent graphs (built by users with `BaseNode` subclasses) automatically inherit `BaseStatePersistence`, `record_run`, `iter_from_persistence`, snapshot IDs, and OpenTelemetry span wrapping.
- **The agent adds a separate `run_step` counter.** It is not the graph node; it scopes `ToolManager.for_run_step` and capability hooks (`ctx.run_step > 0` gating in `capabilities/prepare_tools.py:65`). This lets the framework re-evaluate dynamic toolsets and toolset-level per-step state without conflating that with graph traversal.
- **Two retry budgets, distinct and explicit.** Output retry budget (`max_output_retries`) lives on `GraphAgentState.output_retries_used` and is global per-run; tool retry budget (`ToolDefinition.max_retries`) lives on each tool, defaults to `Agent.default_max_retries`, and is per-tool per-step. The distinction is enforced and documented (`agent/abstract.py:106-130`, `tool_manager.py:177-181`, `_agent_graph.py:162-179`).
- **Per-tool retries propagate across steps via `for_run_step`.** When `ctx.run_step` advances, the toolset manager carries `failed_tools` into the next step's `ctx.retries` (`tool_manager.py:120-145`).
- **Tool validation and execution are split.** `ValidatedToolCall` (`tool_manager.py:46-75`) separates `args_valid` from execution, so the framework can emit `FunctionToolCallEvent(args_valid=...)` before the body runs and decide whether to retry, skip, or defer.
- **Output validation is wrapped in output hooks, not tool hooks.** `ToolManager.validate_output_tool_call` and `execute_output_tool_call` (`tool_manager.py:505-671`) use a separate hook chain (`run_output_validate_hooks`, `run_output_process_hooks`) so that output-tool failures don't fan out to user-facing tool hooks (`before_tool_validate`, etc.). Output validators see a global retry context, while output-tool functions see per-tool `max_retries` (`tool_manager.py:622-631`, with a comment acknowledging a known mismatch tracked in #5238).
- **Agent concurrency is per-run, not per-step.** `ConcurrencyLimiter` gates the entire `agent.run` block (`agent/__init__.py:1532-1534`, `concurrency.py:79-127`). Within a run, model requests and tool calls execute without run-level locking (tool calls have their own internal `parallel_execution_mode`).
- **Three durability models share one base.** Temporal, DBOS, and Prefect all implement the same `AbstractToolset` interface (`toolsets/abstract.py:110-180`) plus a `TemporalWrapperToolset` / DBOS / Prefect wrapper that moves IO methods into activities/tasks. The wrapper only intercepts `get_tools` / `call_tool`; non-IO methods stay on the tool itself. This means the framework's atomic model is unchanged for the agent — only the IO calls become engine-native units.
- **`run_id` and `conversation_id` are stamped onto every message and reused across crashes.** `_agent_graph.py:912, 969, fill_run_metadata` ensure every `ModelRequest`/`ModelResponse` carries both IDs, so message-history replay works.
- **`capture_run_messages` is a dedicated context manager** so that even if a run raises, the user can inspect what messages exchanged before the failure (`_agent_graph.py:2064-2107`).

## Notable Patterns

- **Capability hook chains** (`capabilities/hooks.py`) — `before_node_run` → `wrap_node_run(handler)` → `on_node_run_error` → `after_node_run` give a stable, observable per-step lifecycle that survives the move from v1 to v2 graph.
- **Context-var based "current run context"** (`_run_context.py:275-305`) — lets hooks, capability middleware, and Temporal run-context serialization find the active `RunContext` without it being passed through every function signature.
- **In-place mutation of shared `loaded_capability_ids` and `discovered_tool_names`** sets (`_agent_graph.py:1431-1456`, with explicit invariant comments) — kept as references across `replace(ctx, ...)` so capability loads in one tool body are visible to the next capability tool body in the same step.
- **SkipModelRequest as an explicit short-circuit** (`_agent_graph.py:617-632, 788-794`) — `SkipModelRequest` lets a capability handler inject a synthetic `ModelResponse` without actually calling the model; counted as a request (`ctx.state.usage.requests += 1`) and routed through the same `_finish_handling` path.
- **Cooperative hand-off for streaming**: `stream_ready` / `stream_done` events + `wrap_task` (`_agent_graph.py:640-693`) — give `wrap_model_request` capability middleware a way to wrap the stream without orphaning the request task on cancellation.
- **`HandleResponseEvent` and `ModelResponseStreamEvent` discriminated unions** with `event_kind` literal tag (`messages.py:2806-2967`) — both event families share one `AgentStreamEvent` union, so consumers can match either kind.

## Tradeoffs

- **Three atomic units are coupled by hand.** The graph node, the run step, and the model request are all incremented/reset in different places, and the relationships (step ⊃ model-request ⊇ tool-call) are documented in comments but not enforced by types. Concretely: `ctx.state.run_step += 1` lives in `_prepare_request` (`_agent_graph.py:856`); `usage.requests += 1` lives in `_make_request` and `stream` (`_agent_graph.py:624, 716, 793, 837`); `usage.tool_calls += 1` lives in `_raw_execute` and `_execute_tool_call_impl` (`tool_manager.py:735, 767`). A refactor that moves one but not the others will desync them.
- **`run_step` lives only on the agent's `GraphAgentState`, not on the graph's snapshot.** So the run-step counter is part of in-memory state but not part of `BaseStatePersistence`. A pure-`pydantic_graph` user who wants `run_step` semantics has to model it themselves in their `StateT`.
- **The agent loop does not snapthost graph nodes by default.** No `BaseStatePersistence` is wired into `Agent.run()` (`agent/__init__.py:1535-1543`). Recovery is by `message_history` reconstruction. This is by design (agent users typically want to re-derive rather than re-replay) but means there's no first-class "did the previous `CallToolsNode` complete?" answer — only the message history and `RunUsage`.
- **Two retry budgets (output vs tool) require per-tool dispatch to track separately.** Output validation uses one counter; each tool has its own counter. The user has to configure both. Mismatch is possible and documented as known (`tool_manager.py:622-631`, #5238).
- **Tool-call retries inside a step are not isolated per call in the message history.** A tool that retries 3 times emits three `FunctionToolResultEvent`s in a single step's event stream, but the message history only retains the final `ToolReturnPart`/`RetryPromptPart` (`_agent_graph.py:1960-1961`). Tracing the per-attempt behaviour requires OTel or the event stream, not the message history alone.
- **`SimpleStatePersistence` is not durable** (`in_mem.py:32-50`); only `FullStatePersistence` (in-memory) and `FileStatePersistence` (`file.py:30-179`) keep a history. A user who wants to use `record_run`'s status transitions but doesn't pass `persistence=` falls back to `SimpleStatePersistence` (`graph.py:235-237`).
- **The `event_kind` discriminator is a literal string** (`messages.py:2840, 2854, 2884, 2918, 2935, 2951`). Renaming an event_kind is a breaking change with no static check, only runtime `Discriminator` resolution.

## Failure Modes / Edge Cases

- **Mid-`ModelRequestNode` crash with truncated tool call**: `check_incomplete_tool_call` (`_agent_graph.py:146-160`) raises `IncompleteToolCall` with the diagnostic message about increasing `max_tokens`. Without this, the next step would try to validate a half-built `ToolCallPart.args`.
- **Mid-`ModelRequestNode` crash with full response**: `_append_response` (`_agent_graph.py:1015-1025`) appends to history *and* increments usage. On retry the message is already present and the framework re-uses it (`_agent_graph.py:752-753, 831-834`).
- **Mid-`CallToolsNode` parallel-execution crash**: `_call_tools` (`_agent_graph.py:1947-1956`) cancels and drains sibling tasks on `BaseException` (not just `CancelledError`) so the asyncio task pool doesn't leak.
- **`run_stream_events()` flow-control mismatch**: `run_stream_events` skips `wrap_node_run` / `after_node_run` for the streaming model-request node, but compensates by routing through `agent_run.next(SetFinalResult(...))` (`agent/abstract.py:876-879`). The comment explicitly acknowledges the skip is intentional.
- **`Bare async for` over `agent_run`**: `__anext__` (`run.py:203-233`) explicitly does *not* fire capability hooks; if the run ends with undrained `when_idle` enqueued messages, it raises `UndrainedPendingMessagesError` so messages don't get silently dropped.
- **`_did_stream` flag**: `ModelRequestNode.run` (`_agent_graph.py:596-602`) raises `AgentRunError` if `run()` is called after `stream()` was started — prevents the same node from being double-entered.
- **Concurrency-limiter held during the entire agent run**: if the limiter's slot is released only by an unhandled exception, the slot can be orphaned. The `AsyncExitStack` pattern at `agent/__init__.py:1531-1543` should release it on normal exit; needs a careful read of `concurrency.py` to confirm leak-free release.
- **In-flight tool-set changes mid-run**: `tests/test_agent.py:7888-7960` shows that adding tools during a run is supported (it round-trips via `_add_function` then a follow-up tool call), but only via mutation on a *shared* toolset — `for_run_step` re-evaluates but doesn't snapshot, so a concurrent agent using the same toolset would race.

## Future Considerations

- **v2 step graph** (`pydantic_graph/step.py:119-253`) introduces `Step`, `StepNode`, and `NodeStep` so v1 `BaseNode` graphs can interop with v2 step functions. The NodeStep wraps a node type and the agent graph's iterator already calls into this (`AgentRun._node_to_task`, `run.py:252-253`). This is forward-compatible but not yet a full step-native runtime; the agent loop is still v1 node-by-node.
- **Persistent agent runs**: the existence of `SnapshotStatus` and `record_run` in `pydantic_graph.persistence` strongly suggests that wiring `BaseStatePersistence` into `Agent.run` (to snapshot the agent's `GraphAgentState` per node) would be a natural extension. Currently absent (see "Tradeoffs"), but the shape is in place.
- **Tool-call idempotency at the activity level**: Temporal's `TemporalFunctionToolset.call_tool_activity` (`durable_exec/temporal/_function_toolset.py:43-104`) re-derives `tool` from `await toolset.get_tools(ctx)` inside the activity body, so a tool added between attempts inside an activity can be picked up. This is fragile if the toolset is mutated between attempts; a stable per-call tool identity (e.g. via the `ToolsetTool` returned by `get_tools`) would harden it.
- **Partial completion reconciliation**: `CaptureRunMessages` (`_agent_graph.py:2064-2107`) gives a single-shot view; for long-running runs, a structured checkpoint (e.g. by step) would be a useful addition.
- **Granularity of cancellation**: per-call `cancel_and_drain` is implemented; per-step cancellation (stop the graph at the next node boundary and discard partial in-flight tool calls) is not explicitly modeled. The `BaseException` drain in `_call_tools` (`_agent_graph.py:1950-1956`) approaches it but doesn't roll back tool side-effects.
- **Activity / task naming**: Temporal and Prefect use deterministic, descriptive activity/task names (`call tool: <id>:<name>`, `Model Request: <model_name>`); if the durable-execution engines move to a "step" model, the current activity-per-tool design needs translation.

## Questions / Gaps

- **Is there a way to tell from a finished `AgentRunResult` whether the run actually completed the final `CallToolsNode`?** `GraphAgentState.run_step` is exposed via the private `_state` attribute on `AgentRunResult` (`agent/__init__.py:485-487`, exposed by tests at `tests/test_agent.py:7881`). It's not part of the public surface, and a user looking at `AgentRunResult` cannot reliably distinguish "model produced final output and graph ended" from "tool produced final output and graph ended" without inspecting the message history.
- **Is there a way to get the partial `ModelResponse` from a crashed `ModelRequestNode.stream`?** `_SkipStreamedResponse` (`_agent_graph.py:540-577`) exists for the skip path, but a real partial-stream failure is handled by `stream_error` (`_agent_graph.py:1065, 1077-1079`) which only stores the exception, not the partial response. OTel spans are the canonical place to recover partial responses.
- **What is the public, durable-stable schema for `run_step`?** It is a `RunContext.run_step: int` (`_run_context.py:78-79`) and a `GraphAgentState.run_step: int` (`_agent_graph.py:128`), but neither is part of `BaseStatePersistence.Snapshot` schemas. There is no documented schema versioning of these fields.
- **Does the `enqueue` queue form a partial-completion marker?** `_pending_messages: list[PendingMessage]` lives on `GraphAgentState` (`_agent_graph.py:142`), but it is *not* part of the message-history snapshot and would not survive a `pydantic_graph` persistence-based resume (only the in-memory state). For durable execution, the queue must be drained before the run ends or it surfaces as `UndrainedPendingMessagesError` (`run.py:221-232`).
- **For the v2 step graph**: there is no explicit `StepStatus` yet; the step runtime relies on `BaseNode`'s `SnapshotStatus`. A unified status enum across v1 and v2 would be needed if the two are to coexist.
- **No clear evidence found** for an explicit "turn" abstraction. Pydantic AI uses `run_id` (one run = one turn) and `conversation_id` (multi-run conversation), but no `Turn` class. `_run_id_index` (`_agent_graph.py:2176-2181`) is the closest analog.

---

Generated by `01.03-step-turn-and-task-atomicity` against `pydantic-ai`.
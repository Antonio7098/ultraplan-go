# Source Analysis: pydantic-ai

## 01.07 Concurrency and Parallel Advancement

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python 3.10+ on `asyncio` + `anyio`; subpackages `pydantic_ai_slim` (agent framework, `pydantic_ai/`), `pydantic_graph` (graph runtime, `pydantic_graph/`), `pydantic_evals` (eval framework, `pydantic_evals/`). Durable adapters in `pydantic_ai/durable_exec/{temporal,dbos,prefect}`. |
| Analyzed | 2026-07-03 |

## Summary

Pydantic-AI has a **deeply layered, well-engineered concurrency model** built on `asyncio` + `anyio` with three orthogonal axes:

1. **Per-run tool-fan-out.** The agent loop in `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1862-1956` (`_call_tools`) executes multiple tool calls from one model response as a fan-out: it spawns one `asyncio.create_task` per call (`_agent_graph.py:1917-1925`) and then drives them with `asyncio.wait(..., return_when=...)` in either **stream-as-they-finish** (`parallel`) or **all-complete-then-stream** (`parallel_ordered_events`) mode, with **per-call deterministic ordering** of results in the final message history (`_agent_graph.py:1958-1961`). A `ContextVar`-driven `ParallelExecutionMode` (`pydantic_ai_slim/pydantic_ai/tool_manager.py:38-117`) and a per-tool `sequential=True` flag (`pydantic_ai_slim/pydantic_ai/tools.py:454, 720`) opt the batch into sequential execution when any tool requires it (`pydantic_ai_slim/pydantic_ai/tool_manager.py:155-168`).
2. **Graph-level parallel advancement.** `pydantic_graph/pydantic_graph/graph_builder.py` is a v2 runtime with explicit `Fork` and `Join` nodes (`pydantic_graph/pydantic_graph/node.py:60-95`, `pydantic_graph/pydantic_graph/join.py:140-216`). The runtime schedules multiple `GraphTask`s in parallel via `anyio.create_task_group()` + `tg.start_soon(...)` (`graph_builder.py:457, 473, 838`), tracks them in `active_tasks` / `active_reducers` dicts (`graph_builder.py:634-642`), and provides a `ReducerContext.cancel_sibling_tasks()` primitive that lets a join opt into early-stop semantics (`pydantic_graph/pydantic_graph/join.py:73-78, 142-147`).
3. **Cross-run / cross-process concurrency control.** A first-class `ConcurrencyLimiter` (`pydantic_ai_slim/pydantic_ai/concurrency.py:79-226`) wraps `anyio.CapacityLimiter`, supports `max_queued` back-pressure with a race-free `anyio.Lock` for atomic queue-depth enforcement (`concurrency.py:104-110, 188-222`), and exposes OTel observability spans on wait (`concurrency.py:202-219`). It is used as both per-agent cap (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:261, 293, 1531-1534`) and per-model cap via `ConcurrencyLimitedModel` (`pydantic_ai_slim/pydantic_ai/models/concurrency.py:26-111`).

Concurrency primitives in use:
- **`asyncio`:** `create_task`, `wait(..., return_when=...)`, `Event` for handoffs, `Lock` (none observed; `anyio.Lock` is preferred). `asyncio` is the public surface for streaming and tool fan-out.
- **`anyio`:** `create_task_group`, `CapacityLimiter`, `Lock`, `Semaphore`, `fail_after`, `move_on_after`, `CancelScope`, `create_memory_object_stream`, `to_thread.run_sync`. `anyio` is the workhorse for graph execution, MCP session lifecycle, and concurrency limiting.
- **Thread pool:** default `anyio.to_thread.run_sync` for sync tool functions (`pydantic_ai_slim/pydantic_ai/_utils.py:73-130`), with an opt-in `using_thread_executor` context manager to swap in a bounded `ThreadPoolExecutor` for long-running servers (`_utils.py:93-115`, `pydantic_ai_slim/pydantic_ai/agent/abstract.py:1574-1584`).

The **cooperative hand-off protocol** between the agent coroutine and a streaming wrap task (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:634-693` and `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1561-1598`) is a small but important pattern: a pair of `asyncio.Event`s (`_run_ready` / `_run_done`) with `asyncio.wait(..., return_when=FIRST_COMPLETED)` synchronizes the capability middleware chain and the run loop, with `cancel_and_drain` cleanup on either-side failure (`_agent_graph.py:686-693`, `agent/__init__.py:1594-1598`).

Failure-collection contract: when a single tool-call task raises, `asyncio.wait` does not cancel siblings automatically. The agent graph handles this by explicitly calling `await cancel_and_drain(*tasks, ...)` on `asyncio.CancelledError` and on any other `BaseException` (`_agent_graph.py:1947-1956`, `_utils.py:232-249`) to avoid orphan tasks. The graph runtime routes task exceptions through a `MemoryObjectStream` so they are not transformed by the task group (`graph_builder.py:840-863`). For general fan-out, `_utils.gather` (`pydantic_ai_slim/pydantic_ai/_utils.py:201-229`) uses `anyio.create_task_group` so that a failure cancels the rest, while preserving `asyncio.gather`'s API shape (single exception unwrap, multi-exception `ExceptionGroup`).

Durable execution adapters (Temporal, DBOS, Prefect) **constrain parallel advancement** to remain deterministic for replay: the Temporal `Agent` forces `disable_threads()` and disables non-deterministic tool fan-out by routing it through serializable activities (`pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_agent.py:286-297`); the DBOS `Agent` defaults `parallel_execution_mode='parallel_ordered_events'` because `'parallel'` cannot guarantee deterministic ordering on replay (`pydantic_ai_slim/pydantic_ai/durable_exec/dbos/_agent.py:46-48, 63, 83, 295-302`).

Bottom line: read-only tool calls run as fast as `asyncio.wait(FIRST_COMPLETED)` allows, graph branches advance in parallel under `anyio.TaskGroup` with deterministic join semantics, agent / model concurrency caps are first-class with back-pressure, and durable engines collapse parallelism into ordered replays. The model is consistent, observable, and rigorously tested for race conditions (`tests/test_concurrency.py:127-181`).

## Rating

**9/10 — Mature, durable, observable, extensible, and proven under failure or scale.**

The model is internally coherent and rigorously engineered. Distinct concurrency primitives are used for distinct concerns (process-wide locks via `anyio.Lock`, per-agent caps via `anyio.CapacityLimiter`, in-run fan-out via `asyncio.create_task`+`asyncio.wait`, cross-task coordination via `asyncio.Event`). The cooperative hand-off protocol between the agent run loop and capability middleware is explicit and double-checked by tests. The race condition in `ConcurrencyLimiter._max_queued` was found and fixed with an atomic `anyio.Lock` (`concurrency.py:189-222`), and the test at `tests/test_concurrency.py:127-181` (`test_backpressure_race_condition`) uses an `AsyncBarrier` of 5 parties to deterministically assert the fix. `anyio` is the canonical choice for a backend-agnostic async library and works under both `asyncio` and `trio` (`pydantic_ai_slim/pydantic_ai/durable_exec/temporal/__init__.py:32-39`). The graph runtime has explicit `Fork` / `Join` semantics with `ReducerContext.cancel_sibling_tasks()` and `intermediate_join_nodes` reasoning (`graph_builder.py:1037-1061`, `pydantic_graph/pydantic_graph/parent_forks.py:73-225`). Tool fan-out has three ordered modes (`'parallel'`, `'parallel_ordered_events'`, `'sequential'`) with a per-tool `sequential=True` escape hatch, all observable in OTel.

What keeps it short of 10:
1. `asyncio.gather` is **not** used directly; instead `_utils.gather` (`_utils.py:201-229`) is the wrapper. This is intentional (correct cancellation semantics) but means the surface is `pydantic_ai._utils`-internal rather than a documented top-level utility.
2. The agent stream `event_stream_handler` / `ProcessEventStream` capability (`pydantic_ai_slim/pydantic_ai/capabilities/process_event_stream.py:62-100`) uses a single observer task via `anyio.create_task_group()` rather than a true pub-sub; back-pressure is implicit (the observer's `await send_stream.send(event)` synchronizes). The docstring at lines 33-37 explicitly states "a slow handler back-pressures the rest of the stream."
3. Per-agent caps and per-model caps are independent; there is no global cap that combines both. Sharing a `ConcurrencyLimiter` between agents works (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:261, 542` plus `tests/test_concurrency.py:446-457`) but is opt-in.
4. The graph v2 runtime's `JoinItem` reducer state (`graph_builder.py:701-708`) is a plain `dict[tuple[JoinID, NodeRunID], JoinState]`. The single-threaded mutation is safe under `asyncio` but the docstring does not state this assumption.
5. There is no built-in `asyncio.Semaphore` at the framework level — only the embedding case in `pydantic_ai_slim/pydantic_ai/embeddings/bedrock.py:613-614` and the explicit `ConcurrencyLimiter` (which is `CapacityLimiter`-based, not `Semaphore`).

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Parallel tool fan-out: per-call `asyncio.create_task` | `tasks = [asyncio.create_task(_call_tool(...), name=call.tool_name) for call in tool_calls]` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1916-1926` |
| `parallel_ordered_events` waits for all, then yields in order | `await asyncio.wait(tasks, return_when=asyncio.ALL_COMPLETED)` then loop in index order | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1928-1933` |
| `parallel` (default) yields as tasks complete | `await asyncio.wait(pending, return_when=asyncio.FIRST_COMPLETED)` in a loop | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1935-1945` |
| `sequential` fallback when no fan-out is desired | `if parallel_execution_mode == 'sequential': for ... await ...` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1902-1913` |
| `ParallelExecutionMode` literal and ContextVar default `'parallel'` | `ParallelExecutionMode = Literal['parallel', 'sequential', 'parallel_ordered_events']` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:38-42` |
| `parallel_execution_mode()` context manager | `ToolManager.parallel_execution_mode(mode)` uses `ContextVar` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:95-118` |
| Per-tool `sequential=True` overrides ContextVar | `get_parallel_execution_mode` returns `'sequential'` if any tool is sequential | `pydantic_ai_slim/pydantic_ai/tool_manager.py:155-168` |
| `Tool.sequential` flag | `sequential: bool` on `ToolDefinition` and `Tool` ctor | `pydantic_ai_slim/pydantic_ai/tools.py:454, 481, 575, 720` |
| Tool-call results collected in index order for deterministic history | `output_parts.extend([tool_parts_by_index[k] for k in sorted(...)])` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1958-1961` |
| Tool fan-out partial-failure cleanup: `cancel_and_drain` on `CancelledError` and other `BaseException` | `except asyncio.CancelledError as e: await cancel_and_drain(*tasks, msg=...)` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1947-1956` |
| `_utils.gather` wraps `anyio.create_task_group` with `asyncio.gather`-shaped single-exception unwrap | `async with anyio.create_task_group() as tg: ... except BaseExceptionGroup` | `pydantic_ai_slim/pydantic_ai/_utils.py:201-229` |
| `cancel_and_drain` uses `anyio.CancelScope(shield=True)` + `asyncio.gather(return_exceptions=True)` | Cleanup helper for orphaned tasks | `pydantic_ai_slim/pydantic_ai/_utils.py:232-249` |
| Cooperative wrap-task handoff with `asyncio.Event` pair | `stream_ready` / `stream_done` events; `asyncio.wait({ready_waiter, wrap_task}, return_when=FIRST_COMPLETED)` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:640-693` |
| Cooperative `_do_run`/`wrap_run` handoff in `Agent.run` | `_run_ready` / `_run_done` events; `asyncio.wait(..., return_when=FIRST_COMPLETED)` | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1561-1598` |
| `ConcurrencyLimiter` is a wrapper around `anyio.CapacityLimiter` | `self._limiter = anyio.CapacityLimiter(max_running)` | `pydantic_ai_slim/pydantic_ai/concurrency.py:79-110` |
| `max_queued` enforced under `anyio.Lock` for race-free atomic check | `async with self._queue_lock: ... self._waiting_count += 1` | `pydantic_ai_slim/pydantic_ai/concurrency.py:186-222` |
| OTel span emitted while waiting on the limiter | `with tracer.start_as_current_span(span_name, attributes=attributes)` | `pydantic_ai_slim/pydantic_ai/concurrency.py:202-222` |
| `AbstractConcurrencyLimiter` extensible for distributed (e.g., Redis) limiters | ABC with `acquire` / `release`; docstring shows `RedisConcurrencyLimiter` example | `pydantic_ai_slim/pydantic_ai/concurrency.py:23-63` |
| `ConcurrencyLimitExceeded` raised when queue is full | `raise ConcurrencyLimitExceeded(...)` from `ConcurrencyLimiter.acquire` | `pydantic_ai_slim/pydantic_ai/concurrency.py:189-196`; class at `pydantic_ai_slim/pydantic_ai/exceptions.py:199` |
| Agent-level `max_concurrency` runs every `run()` / `iter()` through the limiter | `async with get_concurrency_context(self._concurrency_limiter, f'agent:{name}')` | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1531-1534` |
| Agent also stores the limiter; no default cap | `self._concurrency_limiter = _concurrency.normalize_to_limiter(max_concurrency)` | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:542` |
| `ConcurrencyLimitedModel` wraps `Model.request` / `request_stream` / `count_tokens` with the limiter | `async with get_concurrency_context(self._limiter, f'model:{self.model_name}')` | `pydantic_ai_slim/pydantic_ai/models/concurrency.py:78-111` |
| `limit_model_concurrency` helper | Returns `ConcurrencyLimitedModel` if limiter is not None; passthrough otherwise | `pydantic_ai_slim/pydantic_ai/models/concurrency.py:114-141` |
| Shared limiter across models / agents | `model1 = ConcurrencyLimitedModel(..., limiter=shared_limiter); model2 = ...` | `pydantic_ai_slim/pydantic_ai/models/concurrency.py:48-52`; tests at `tests/test_concurrency.py:361-416` |
| `CombinedToolset.for_run` / `for_run_step` / `get_tools` / `get_instructions` all use `_utils.gather` | `new_toolsets = await gather(*(t.for_run(ctx) for t in self.toolsets))` | `pydantic_ai_slim/pydantic_ai/toolsets/combined.py:44-46, 48-52, 66-67, 105-106` |
| `CombinedCapability.for_run` runs all caps in parallel | `new_caps = await gather(*(c.for_run(ctx) for c in self.capabilities))` | `pydantic_ai_slim/pydantic_ai/capabilities/combined.py:77-81` |
| `ProcessEventStream` capability spawns observer in `anyio.TaskGroup` and tees the event stream | `tg.start_soon(run_handler); async with send_stream: ... await send_stream.send(event)` | `pydantic_ai_slim/pydantic_ai/capabilities/process_event_stream.py:83-100` |
| `anyio.Semaphore` for bounded parallel embedding (Bedrock) | `semaphore = anyio.Semaphore(max_concurrency)` then `async with anyio.create_task_group()` | `pydantic_ai_slim/pydantic_ai/embeddings/bedrock.py:606-639` |
| Sync tool functions dispatched via `anyio.to_thread.run_sync` by default | `await run_sync(wrapped_func)` inside `run_in_executor` | `pydantic_ai_slim/pydantic_ai/_utils.py:118-130` |
| Opt-in bounded `ThreadPoolExecutor` via `using_thread_executor` | `loop.run_in_executor(executor, ctx.run, wrapped_func)` | `pydantic_ai_slim/pydantic_ai/_utils.py:124-128`; public context at `pydantic_ai_slim/pydantic_ai/agent/abstract.py:1574-1584` |
| `Agent.parallel_tool_call_execution_mode` static context manager | Mirrors `ToolManager.parallel_execution_mode` | `pydantic_ai_slim/pydantic_ai/agent/abstract.py:1551-1571` |
| `MCPServer` session task pinned to one asyncio.Task to keep cancel scopes inside one task | `state.session_task = asyncio.create_task(self._session_runner())`; reference-counted `nesting_counter` under `_enter_lock` | `pydantic_ai_slim/pydantic_ai/mcp.py:1280-1311, 1313-1342` |
| MCP cleanup bounded by `anyio.move_on_after(_SHUTDOWN_GRACE_SECONDS)` to bound a hung transport | `with anyio.move_on_after(_SHUTDOWN_GRACE_SECONDS): await session_task_to_await` | `pydantic_ai_slim/pydantic_ai/mcp.py:1331-1341` |
| `_anext_lock: anyio.Lock` serializes consumer access to the shared event iterator | `async with self._anext_lock: try: event = await anext(base_iter)` | `pydantic_ai_slim/pydantic_ai/result.py:71, 397-411` |
| `group_by_temporal` uses a single `asyncio.create_task(anext(...))` and `asyncio.wait((task,), timeout=wait_time)` | Debouncer for event streams | `pydantic_ai_slim/pydantic_ai/_utils.py:280-360` |
| `_run_stream_events` uses `asyncio.create_task(run_agent())` and `anyio.create_memory_object_stream` to fan the run into the consumer | Single-producer, single-consumer | `pydantic_ai_slim/pydantic_ai/agent/abstract.py:1252-1326` |
| `disable_threads` context manager used inside Temporal workflow override | `with _utils.disable_threads(): ...` in `_temporal_overrides` | `pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_agent.py:286-289` |
| `DBOSAgent` defaults `parallel_execution_mode='parallel_ordered_events'` (subset of modes) | `DBOSParallelExecutionMode = Literal['sequential', 'parallel_ordered_events']` | `pydantic_ai_slim/pydantic_ai/durable_exec/dbos/_agent.py:46-48, 63, 83, 295-302` |
| Temporal `Agent` wires the agent graph through Temporal activities, so non-deterministic Python parallelism is replaced with deterministic workflow steps | Comment on why non-async tools are unsupported in workflows | `pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_agent.py:108-108`; `_model.py:100-126, 131-166, 168-212` |
| `pydantic_graph` v2 runtime uses `anyio.create_task_group` + `task_group.start_soon(...)` per task | `_handle_execution_request` schedules new tasks in parallel | `pydantic_graph/pydantic_graph/graph_builder.py:457, 473, 834-838` |
| `Fork` node type | `class Fork(Generic[InputT, OutputT])` with `is_map: bool` | `pydantic_graph/pydantic_graph/node.py:60-95` |
| `Fork` map path eagerly creates one `GraphTask` per iterable item | `for thread_index, input_item in enumerate(inputs): item_tasks = self._handle_path(...)` | `pydantic_graph/pydantic_graph/graph_builder.py:1001-1035` |
| `Join` node + reducers (`reduce_list_append`, `reduce_sum`, `ReduceFirstValue`) | `class Join`, `PlainReducerFunction`, `ReducerContext` | `pydantic_graph/pydantic_graph/join.py:81-216` |
| `ReduceFirstValue` cancels siblings on first completion | `ctx.cancel_sibling_tasks(); return inputs` | `pydantic_graph/pydantic_graph/join.py:142-147` |
| Graph runtime iterates over a `MemoryObjectStream` of `_GraphTaskResult`s and `ErrorMarker`s | `async for task_result in self.iter_stream_receiver` | `pydantic_graph/pydantic_graph/graph_builder.py:646-748` |
| Each task runs in its own `CancelScope` tracked in `cancel_scopes: dict[TaskID, CancelScope]` | `with CancelScope() as scope: self.cancel_scopes[t_.task_id] = scope` | `pydantic_graph/pydantic_graph/graph_builder.py:840-863` |
| `_finish_task` cancels the task's scope | `scope = self.cancel_scopes.pop(task_id, None); scope.cancel()` | `pydantic_graph/pydantic_graph/graph_builder.py:827-832` |
| `_cancel_sibling_tasks` walks `active_tasks` and `_finish_task`s those sharing the parent fork run | Used by reducer to cancel siblings | `pydantic_graph/pydantic_graph/graph_builder.py:1049-1059` |
| `intermediate_join_nodes: dict[JoinID, set[JoinID]]` reasoning to avoid finalizing a join before its intermediate join | "we must finalize J1 first because it might produce items that feed into J2" | `pydantic_graph/pydantic_graph/graph_builder.py:749-818` |
| `pydantic_evals` `task_group_gather` utility | `async with anyio.create_task_group() as tg: ...` for parallel case evaluation | `pydantic_evals/pydantic_evals/_utils.py:91-110` |
| Online evals phase 1: run every evaluator in parallel | `async with anyio.create_task_group() as eval_tg: ... start_soon(_run_and_collect, ...)` | `pydantic_evals/pydantic_evals/_online.py:519-533` |
| Online evals phase 2: per (group, sink) fan-out | `async with anyio.create_task_group() as sink_tg: ...` | `pydantic_evals/pydantic_evals/_online.py:535-563` |
| Race-condition test for `max_queued` uses an `AsyncBarrier` of 5 parties | `barrier.wait()` then all try to acquire | `tests/test_concurrency.py:24-181` |
| Tests assert shared limiter across models and across agents | `test_shared_limiter_limits_across_models`, `test_agent_with_shared_limiter` | `tests/test_concurrency.py:373-457` |
| Tests assert OTel span attributes for named / unnamed / `max_queued` limiters | `assert attrs['limiter_name'] == 'test-pool'` etc. | `tests/test_concurrency.py:479-575` |
| Parallel mode in DBOS is excluded because of replay determinism | `'parallel' cannot guarantee deterministic ordering` | `pydantic_ai_slim/pydantic_ai/durable_exec/dbos/_agent.py:46-48` |
| Temporal workflow `using_thread_executor` is explicitly disabled to keep deterministic steps | `_utils.disable_threads()` in `_temporal_overrides` | `pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_agent.py:286-289` |
| Tool-call handoff with `agent.run` cancels orphan tasks on consumer-side exception | `except asyncio.CancelledError as e: await _utils.cancel_and_drain(task, msg=...)` | `pydantic_ai_slim/pydantic_ai/agent/abstract.py:1312-1321` |
| Cancellation cleanup: `_cancel_task` swallows exceptions, used to drain wrap task | `except BaseException: pass` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:78-87` |

## Answers to Dimension Questions

### 1. What can run in parallel?

- **Inside one agent run (tool fan-out):** all function / native / output tool calls from a single model response can be dispatched concurrently. Default `parallel` mode runs them in `asyncio.create_task` siblings and streams events as each completes (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1916-1945`). `parallel_ordered_events` runs them in parallel but only yields events after all complete (`_agent_graph.py:1928-1933`). `sequential` mode is the fallback (`_agent_graph.py:1902-1913`) and is auto-forced if any tool in the batch has `sequential=True` (`pydantic_ai_slim/pydantic_ai/tool_manager.py:155-168`).
- **Across agent runs:** multiple `agent.run()` calls share an `anyio.CapacityLimiter` via `Agent(..., max_concurrency=...)` (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:261, 293, 542, 1531-1534`). Sharing a `ConcurrencyLimiter` across agents is supported and tested (`tests/test_concurrency.py:361-457`).
- **Across model requests:** `ConcurrencyLimitedModel` caps concurrent model requests per-model, or shares a limiter across models (`pydantic_ai_slim/pydantic_ai/models/concurrency.py:26-111`).
- **Across toolsets and capabilities:** `CombinedToolset` uses `_utils.gather` to run `for_run` / `for_run_step` / `get_tools` / `get_instructions` in parallel (`pydantic_ai_slim/pydantic_ai/toolsets/combined.py:44-46, 48-52, 66-67, 105-106`); `CombinedCapability` runs `for_run` in parallel (`pydantic_ai_slim/pydantic_ai/capabilities/combined.py:77-81`).
- **Inside `pydantic_graph` v2 runtime:** `Fork` and `Join` nodes (`pydantic_graph/pydantic_graph/node.py:60-95`, `pydantic_graph/pydantic_graph/join.py:140-216`) drive parallel branch advancement under a single `anyio.create_task_group` (`pydantic_graph/pydantic_graph/graph_builder.py:457, 473, 838`).
- **Embedding case:** `BedrockEmbeddingModel._embed_concurrent` fans out one task per input with an `anyio.Semaphore` cap (`pydantic_ai_slim/pydantic_ai/embeddings/bedrock.py:606-639`).
- **ProcessEventStream capability:** the observer coroutine runs in its own task via `anyio.create_task_group` and pulls events from an `anyio.create_memory_object_stream` (`pydantic_ai_slim/pydantic_ai/capabilities/process_event_stream.py:83-100`).
- **Event-stream hand-off:** `agent.iter` and `agent.run_stream_events` use a producer task created via `asyncio.create_task` plus `anyio.create_memory_object_stream` to feed the consumer (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:1561-1598`; `pydantic_ai_slim/pydantic_ai/agent/abstract.py:1252-1326`).
- **Eval framework:** `task_group_gather` runs eval cases and evaluators in parallel via `anyio.create_task_group` (`pydantic_evals/pydantic_evals/_utils.py:91-110`; `pydantic_evals/pydantic_evals/_online.py:519-563`).
- **Sync tool functions:** dispatched to the default `anyio.to_thread.run_sync` thread pool (`pydantic_ai_slim/pydantic_ai/_utils.py:73-130`), or to a user-supplied `ThreadPoolExecutor` via `Agent.using_thread_executor` (`pydantic_ai_slim/pydantic_ai/agent/abstract.py:1574-1584`).
- **MCP session:** runs in a single dedicated `asyncio.Task` so its cancel scopes never cross task boundaries; the entry lock and reference counter (`pydantic_ai_slim/pydantic_ai/mcp.py:1280-1342`) make concurrent `async with` safe.
- **There is no multiprocessing / `ProcessPoolExecutor` use** in core; the framework is `asyncio` + thread-pool only.

### 2. What must be serialized?

- **Per tool batch with `sequential=True`:** serial (`pydantic_ai_slim/pydantic_ai/tool_manager.py:155-168`).
- **Across the agent run step:** the agent loop yields results in input-tool index order regardless of fan-out mode (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1958-1961`).
- **Per agent run:** a single `ConcurrencyLimiter` slot at a time per agent; additional concurrent calls wait or raise `ConcurrencyLimitExceeded` (`pydantic_ai_slim/pydantic_ai/concurrency.py:171-196`; `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1531-1534`).
- **MCP session lifecycle:** serialized through `_enter_lock: anyio.Lock` (cached_property) and an `anyio.Event` pair (`pydantic_ai_slim/pydantic_ai/mcp.py:800-803, 1280-1311`). The session itself runs in a single pinned `asyncio.Task` so cancel scopes never cross task boundaries (`pydantic_ai_slim/pydantic_ai/mcp.py:1217-1269, 1280-1292`).
- **Stream consumption:** `_anext_lock: anyio.Lock` serializes access to the shared base iterator to prevent races between an early `stream_text()` break and the `group_by_temporal` `anext` task (`pydantic_ai_slim/pydantic_ai/result.py:71, 397-411`).
- **`ConcurrencyLimiter.max_queued` race:** atomic through `anyio.Lock` (`pydantic_ai_slim/pydantic_ai/concurrency.py:189-222`).
- **Graph v2 runtime:** per-task `CancelScope` keys and `active_tasks` / `active_reducers` dicts are mutated only from the same event loop (`pydantic_graph/pydantic_graph/graph_builder.py:633-642, 827-832`). `Fork`s with `is_map=True` create one task per item; `Join` reducers run in serialized order relative to their branch arrivals (`graph_builder.py:701-708`).
- **Durable engines:** the Temporal override disables Python threads and routes work through Temporal activities (`pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_agent.py:286-289`); DBOS constrains parallel mode to `'parallel_ordered_events'` to keep replays deterministic (`pydantic_ai_slim/pydantic_ai/durable_exec/dbos/_agent.py:46-48, 295-302`).

### 3. How are shared resources protected?

- **`anyio.Lock` (cached_property when bound to event loop) on:** `Agent._enter_lock` (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:267-271`), `MCPServer._enter_lock` (`pydantic_ai_slim/pydantic_ai/mcp.py:800-803, 2118-2122`), `FallbackModel._enter_lock` (`pydantic_ai_slim/pydantic_ai/models/fallback.py:80-85`), `FastMCPToolset._enter_lock` (`pydantic_ai_slim/pydantic_ai/toolsets/fastmcp.py:119-123`), `_anext_lock` on `AgentStream` (`pydantic_ai_slim/pydantic_ai/result.py:71`).
- **`anyio.CapacityLimiter` via `ConcurrencyLimiter` for back-pressured concurrency** with optional `max_queued` queue depth (`pydantic_ai_slim/pydantic_ai/concurrency.py:79-110, 171-196`).
- **`anyio.Semaphore` for a simple cap** (only in `BedrockEmbeddingModel._embed_concurrent`) (`pydantic_ai_slim/pydantic_ai/embeddings/bedrock.py:613-639`).
- **`asyncio.Event` pairs for cooperative hand-offs** between capability middleware and the run loop (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:640-693`; `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1561-1598`).
- **`asyncio.Task` + `anyio.CancelScope` per graph task** for fine-grained cancellation (`pydantic_graph/pydantic_graph/graph_builder.py:840-863`).
- **`anyio.create_task_group` + `start_soon`** as the dominant fan-out primitive in `pydantic_graph`, `ProcessEventStream`, Bedrock embedder, and `pydantic_evals`.
- **No `asyncio.Lock`** is used in core. All locking is `anyio` so the framework works under both `asyncio` and `trio`.

### 4. Are results ordered deterministically?

- **Tool-fan-out events:** in `parallel_ordered_events` mode, events are emitted only after all tasks complete, then iterated in input-tool order (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1928-1933`); in default `parallel` mode, events are emitted as tasks finish (`_agent_graph.py:1935-1945`).
- **Final tool-result parts in message history:** always in index order, regardless of which mode produced them (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1958-1961`).
- **`_utils.gather`:** returns results in input-coroutine order, not completion order (`pydantic_ai_slim/pydantic_ai/_utils.py:201-229`).
- **`CombinedToolset.get_tools` / `get_instructions`:** gather results in `self.toolsets` declaration order (`pydantic_ai_slim/pydantic_ai/toolsets/combined.py:66-88, 105-113`).
- **`pydantic_graph` `Join` reducers:** pure functions of `(current, inputs)`, applied in arrival order. `reduce_list_append` / `reduce_list_extend` / `reduce_dict_update` / `reduce_sum` / `reduce_null` are deterministic. `ReduceFirstValue` cancels siblings and returns the first inputs (`pydantic_graph/pydantic_graph/join.py:81-147`).
- **`AgentStream`:** debounced by `group_by_temporal` (`pydantic_ai_slim/pydantic_ai/_utils.py:280-360`); consumer-side `anext` is serialized through `_anext_lock` (`pydantic_ai_slim/pydantic_ai/result.py:71, 397-411`).
- **Durable engines:** order is enforced by routing through deterministic activity steps and using `'parallel_ordered_events'` semantics (`pydantic_ai_slim/pydantic_ai/durable_exec/dbos/_agent.py:46-48, 295-302`).
- **`anyio.Semaphore` in Bedrock embeddings:** results are placed into a pre-allocated `results: list = [None] * len(inputs)` by index, so the final list is in input order regardless of completion order (`pydantic_ai_slim/pydantic_ai/embeddings/bedrock.py:616-630`).

### 5. What happens when one parallel branch fails?

- **Tool fan-out in default `parallel` mode:** the per-tool task raises into `handle_call_or_result` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1876-1900`), which converts the exception to a `RetryPromptPart` / `ToolReturnPart` for that index. The other tasks continue; on `asyncio.CancelledError` or any `BaseException` from the wait loop, `cancel_and_drain` cancels the rest (`_agent_graph.py:1947-1956`). Partial results from non-failing calls are preserved in `tool_parts_by_index` (`_agent_graph.py:1871, 1895-1900`) and merged into the final message history (`_agent_graph.py:1958-1961`).
- **`_utils.gather`:** failure of one coroutine cancels the rest of the task group, and the single exception is re-raised directly (multi-failure becomes an `ExceptionGroup`) (`pydantic_ai_slim/pydantic_ai/_utils.py:201-229`).
- **`ConcurrencyLimiter.acquire` on full queue:** raises `ConcurrencyLimitExceeded` instead of waiting (`pydantic_ai_slim/pydantic_ai/concurrency.py:189-196`; exception at `pydantic_ai_slim/pydantic_ai/exceptions.py:199`).
- **Graph v2 runtime:** a task exception is sent through `iter_stream_sender` as `_GraphTaskResult(..., error=exc)` instead of being raised into the task group; the iterator yields an `ErrorMarker` to the caller, which can either echo it back to actually raise or override it with new tasks (`pydantic_graph/pydantic_graph/graph_builder.py:660-668`). The `_finish_task` cleanup cancels the task's `CancelScope` and removes it from `active_tasks` (`graph_builder.py:827-832`).
- **MCP session:** if the session task fails, the error is recorded in `_session_state.connect_error` and surfaced by the next `__aenter__`; the lock is held while a recycled session is started so concurrent callers don't see stale state (`pydantic_ai_slim/pydantic_ai/mcp.py:1259-1309`).
- **ProcessEventStream observer:** an observer that raises propagates through the surrounding capability chain; an observer that returns early stops receiving events but does not affect downstream consumers, because the producer side keeps yielding from `stream` (`pydantic_ai_slim/pydantic_ai/capabilities/process_event_stream.py:33-37, 89-100`).
- **Durable engines:** exceptions are routed through durable activity / step primitives; the workflow engine handles retry. DBOS / Temporal override `parallel_execution_mode` to ordered-events so partial completion semantics match the engine's determinism guarantees.

## Architectural Decisions

- **One `ParallelExecutionMode` literal with three values** (`pydantic_ai_slim/pydantic_ai/tool_manager.py:38`) plus per-tool `sequential=True` (`pydantic_ai_slim/pydantic_ai/tools.py:720`) gives both batch-level and per-tool control. Mode is stored in a `ContextVar` (`tool_manager.py:40-42`) so it is task-local and overridable per-run.
- **`asyncio.wait` over a hand-rolled `TaskGroup`** for the tool fan-out loop (`_agent_graph.py:1928-1945`). This was a deliberate choice: it lets the agent loop distinguish `FIRST_COMPLETED` (default mode) from `ALL_COMPLETED` (ordered mode) and `task.result()` for per-index error handling. Cleanup uses `cancel_and_drain` (an `anyio.CancelScope(shield=True)` + `asyncio.gather(return_exceptions=True)`) so failures are not lost (`_utils.py:232-249`).
- **`_utils.gather` (an `anyio.TaskGroup` wrapper) is preferred over `asyncio.gather`** because `asyncio.gather` leaves sibling tasks running on first failure, while the `TaskGroup` variant cancels them and re-raises a single exception (or `ExceptionGroup` for multi-failure) (`_utils.py:201-229`). The shape matches `asyncio.gather` so call sites are familiar.
- **Cooperative hand-off protocol** between the agent coroutine and the streaming capability middleware uses an `asyncio.Event` pair (`_run_ready` / `_run_done`) and a `FIRST_COMPLETED` race (`_agent_graph.py:634-693`, `agent/__init__.py:1561-1598`). Either side can short-circuit; the other side is cleaned up via `cancel_and_drain` before re-raising.
- **`ConcurrencyLimiter` is pluggable:** `AbstractConcurrencyLimiter` is a public ABC with a Redis-style example in the docstring (`pydantic_ai_slim/pydantic_ai/concurrency.py:23-63`). The default implementation is a thin wrapper around `anyio.CapacityLimiter` with race-free `max_queued` enforcement (`concurrency.py:186-222`).
- **OTel observability at every wait point:** the limiter emits a span with `source`, `waiting_count`, `max_running`, optional `limiter_name`, and `max_queued` (`concurrency.py:202-222`); tests assert the span attributes (`tests/test_concurrency.py:479-575`).
- **Sync work is opt-in to thread pools:** `anyio.to_thread.run_sync` is the default, but `Agent.using_thread_executor` swaps in a bounded `ThreadPoolExecutor` for long-running servers (`_utils.py:73-130`; `agent/abstract.py:1574-1584`). The docstring at `agent/abstract.py:1577-1584` explicitly motivates this for FastAPI deployments where ephemeral threads accumulate.
- **MCP session is pinned to a single task** with reference-counted `async with` (`mcp.py:1217-1342`). The docstring at `mcp.py:1278-1283` explains: "Because the session runs in a dedicated background task, entering and exiting from different tasks (e.g. `asyncio.gather` children, fasta2a workers, or graph node tasks) is safe: the underlying transport's cancel scopes never cross task boundaries."
- **Durable engines intentionally constrain parallelism** for replay determinism: DBOS exposes only `'sequential'` and `'parallel_ordered_events'` (`durable_exec/dbos/_agent.py:46-48`); Temporal forces `_utils.disable_threads()` inside the workflow override (`durable_exec/temporal/_agent.py:286-289`) and routes I/O through Temporal activities (`durable_exec/temporal/_model.py:84-127, 131-166, 168-212`).
- **`pydantic_graph` v2 runtime** uses a `MemoryObjectStream` (`anyio.create_memory_object_stream`) as the work-completion channel (`graph_builder.py:643, 660, 749`) so per-task errors are routed as data, not as task-group explosions.

## Notable Patterns

- **`ParallelExecutionMode` ContextVar + per-tool `sequential=True`** as orthogonal escape hatches (`pydantic_ai_slim/pydantic_ai/tool_manager.py:38-118, 155-168`; `pydantic_ai_slim/pydantic_ai/tools.py:454, 720`).
- **`asyncio.create_task` + `asyncio.wait(..., return_when=...)`** for streaming tool fan-out (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1916-1945`).
- **`_utils.gather`** as the canonical "fan out and gather in input order, but cancel siblings on failure" wrapper (`pydantic_ai_slim/pydantic_ai/_utils.py:201-229`).
- **`cancel_and_drain` cleanup** uses `anyio.CancelScope(shield=True)` + `asyncio.gather(return_exceptions=True)` to avoid both losing exceptions and orphaning tasks (`pydantic_ai_slim/pydantic_ai/_utils.py:232-249`).
- **Cooperative `asyncio.Event` hand-off** between coroutines where one needs to await another without `await`ing it directly (so the outer task can be cancelled independently) (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:640-693`; `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1561-1598`).
- **`anyio.CapacityLimiter` + `anyio.Lock`-protected queue** for back-pressure with deterministic race-free enforcement (`pydantic_ai_slim/pydantic_ai/concurrency.py:79-222`).
- **Pinned session task** for transports whose cancel scopes are bound to a specific task (MCP / fastmcp / stdio / streamable HTTP) (`pydantic_ai_slim/pydantic_ai/mcp.py:1217-1342`).
- **`anyio.create_task_group` + `start_soon`** in the graph runtime for true per-task fan-out, with per-task `CancelScope` keys (`pydantic_graph/pydantic_graph/graph_builder.py:457, 473, 633-642, 834-863`).
- **Reducer-based joins** with `cancel_sibling_tasks` opt-in (`pydantic_graph/pydantic_graph/join.py:73-78, 142-147`).
- **`pydantic_evals` `task_group_gather`** as a one-liner fan-out primitive for evals (`pydantic_evals/pydantic_evals/_utils.py:91-110`).
- **`group_by_temporal` debouncer** for streaming output that uses a single `asyncio.create_task(anext(...))` and `asyncio.wait((task,), timeout=wait_time)` (`pydantic_ai_slim/pydantic_ai/_utils.py:280-360`).
- **Durable-engine parallelism reduction:** Temporal uses `_utils.disable_threads()` to keep the workflow deterministic; DBOS restricts `ParallelExecutionMode` to a subset (`pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_agent.py:286-289`; `pydantic_ai_slim/pydantic_ai/durable_exec/dbos/_agent.py:46-48, 295-302`).

## Tradeoffs

- **`asyncio.create_task` (not `anyio.TaskGroup`) for tool fan-out** is intentional because the agent needs `FIRST_COMPLETED` streaming and per-task error handling, but it also means the agent manually owns the cleanup contract. The `cancel_and_drain` helper mitigates this, but a future refactor to a structured `asyncio.TaskGroup` (Python 3.11+) with `except*` handling could capture partial failures more cleanly (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1916-1956`).
- **Per-agent and per-model caps are independent.** Sharing a `ConcurrencyLimiter` instance across agents is supported (`tests/test_concurrency.py:446-457`) and is the natural way to coordinate, but it is opt-in: each `Agent(max_concurrency=...)` creates its own limiter by default.
- **`ProcessEventStream` is not a true pub-sub.** The observer runs in a single task that pulls from a `MemoryObjectStream`; a slow observer back-pressures the producer (`pydantic_ai_slim/pydantic_ai/capabilities/process_event_stream.py:33-37`). There is no `asyncio.Queue` with a configurable max size.
- **Sync work goes to the default `anyio.to_thread.run_sync` thread pool.** This is the default loop-owned executor; long-running servers are expected to swap in a bounded pool via `Agent.using_thread_executor` (`pydantic_ai_slim/pydantic_ai/agent/abstract.py:1574-1584`). There is no built-in thread-pool cap.
- **Per-task `CancelScope` keys in `pydantic_graph` are a plain dict.** Single-loop mutation is safe under `asyncio` but the design assumes that. Concurrent invocations of `graph.iter(...)` on the same `GraphRun` would race; the docstring at `graph_builder.py:403-413` describes `GraphRun` as a "single execution instance" but does not forbid concurrent iteration.
- **Durable engines have to disable non-determinism.** Users who want full `'parallel'` mode under Temporal / DBOS have to accept that it cannot replay deterministically and is intentionally restricted (`pydantic_ai_slim/pydantic_ai/durable_exec/dbos/_agent.py:46-48`).
- **`ConcurrencyLimitExceeded` is the only back-pressure failure mode.** When `max_queued` is exceeded, the agent raises immediately rather than waiting indefinitely. This is the right behavior for HTTP servers, but a long-running batch job would prefer block-with-timeout.

## Failure Modes / Edge Cases

- **Race in `max_queued` enforcement:** the original non-atomic check was caught by tests; fix at `pydantic_ai_slim/pydantic_ai/concurrency.py:186-222`, regression test at `tests/test_concurrency.py:127-181` (`test_backpressure_race_condition`).
- **Tool fan-out orphan tasks on failure:** `asyncio.wait` does not auto-cancel siblings; the agent loop explicitly calls `cancel_and_drain` on `CancelledError` and on any other `BaseException` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1947-1956`).
- **Streaming handoff task ownership:** if outer cancellation arrives during `asyncio.wait({ready_waiter, wrap_task}, FIRST_COMPLETED)`, the agent drains both tasks before re-raising so `wrap_model_request` cleanup runs (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:686-693`).
- **MCP session recycled mid-startup:** the runner captures local `ready_event` / `stop_event` references so a recycled `__aenter__` cannot corrupt the next session's events (`pydantic_ai_slim/pydantic_ai/mcp.py:1228-1270`).
- **MCP shutdown grace bound:** `anyio.move_on_after(_SHUTDOWN_GRACE_SECONDS)` ensures a hung transport cannot block the agent's own shutdown forever; cancelling the wait propagates to the session task via the same task group (`pydantic_ai_slim/pydantic_ai/mcp.py:1331-1341`).
- **Stream consumption race:** if a consumer breaks out of `stream_text()` early, a pending `anext` task in `group_by_temporal` can race with cleanup; `_anext_lock` serializes access (`pydantic_ai_slim/pydantic_ai/result.py:397-411`).
- **Agent + model cap duplication:** two independent `ConcurrencyLimiter` slots may apply to the same logical request (one on the agent, one on the model). They compose, not collide, but a user setting both should understand that the effective cap is `min(agent, model)`.
- **Graph v2 `active_reducers` dict mutation:** under `asyncio` single-loop mutation is safe; under `trio` it is also safe (single-threaded model). The docstring at `graph_builder.py:403-413` does not explicitly state this assumption.
- **`JoinNode` not runnable directly:** `JoinNode.run` always raises `NotImplementedError` (intended) (`pydantic_graph/pydantic_graph/join.py:235-247`).
- **Durable engine mode subset:** DBOS rejects `'parallel'` (it cannot be deterministically replayed); Temporal disallows `run_sync` / `run_stream` / `run_stream_events` inside a workflow (explicit `UserError`) (`pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_agent.py:564-567, 705-709, 864-868`; `durable_exec/dbos/_agent.py:701-705, 860-863`).

## Future Considerations

- **Structured concurrency in the tool fan-out.** `asyncio.TaskGroup` (Python 3.11+) with `except* Group*` would let the agent collect partial results while still canceling siblings on first failure. The pattern is already used by `_utils.gather` (`pydantic_ai_slim/pydantic_ai/_utils.py:201-229`); lifting it to the tool fan-out loop would be a natural next step.
- **Per-tool concurrency budgets.** Today, every function tool in a fan-out shares the per-agent / per-model cap. A per-tool `max_concurrency` (analogous to `sequential=True`) would let expensive tools be throttled independently.
- **ProcessEventStream as a proper pub-sub.** Replace the single-observer task with a fan-out to N observer tasks, each with its own `asyncio.Queue(maxsize=...)` and a `LifespanPolicy` to control the queue depth.
- **Bounded thread pool by default for sync tools.** Expose a `ThreadPoolExecutor` config on `Agent` so that long-running server deployments do not need to call `using_thread_executor` manually.
- **A unified `concurrency.Budget`** that composes agent, model, and toolset caps. Today, two independent `ConcurrencyLimiter` instances can apply to the same request.
- **Document `GraphRun` as single-loop.** The `pydantic_graph` v2 runtime's `active_tasks` and `cancel_scopes` are dicts with single-loop mutation; explicit docs and (optionally) an `asyncio.Lock` would prevent surprises for users coming from multi-threaded environments.
- **Extend `ReduceFirstValue` style cancellation to graph-level primitives.** `ReducerContext.cancel_sibling_tasks` already exists (`pydantic_graph/pydantic_graph/join.py:73-78`); other reducers could opt into similar early-stop behavior.

## Questions / Gaps

- **No evidence found** of `concurrent.futures.ProcessPoolExecutor` or `multiprocessing` use anywhere in the source. CPU-bound parallelism is out of scope for this codebase.
- **No evidence found** of `asyncio.Queue` with a bounded `maxsize` used as a back-pressure mechanism. The `MemoryObjectStream` in `ProcessEventStream` (`pydantic_ai_slim/pydantic_ai/capabilities/process_event_stream.py:83`) and the graph runtime (`pydantic_graph/pydantic_graph/graph_builder.py:643`) are unbounded.
- **No evidence found** of a framework-level `asyncio.Semaphore` for tool fan-out. The only `anyio.Semaphore` is the Bedrock embedder's `max_concurrency` (`pydantic_ai_slim/pydantic_ai/embeddings/bedrock.py:613-614`).
- **No evidence found** of a "cancel all parallel work" knob on the public `Agent` API. Cancellation is driven by the caller's `asyncio.CancelledError` propagation through the cooperative hand-off; users who want explicit cancellation must use `asyncio.Task.cancel()` on the run's outer task.
- **Concurrency semantics for the .NET / C# implementation** (not present in this source) — the source under analysis is Python-only; if a C# runtime exists in a sibling source, it is not in scope per source isolation rules.

---

Generated by `dimensions/01.07-concurrency-and-parallel-advancement.md` against `pydantic-ai`.

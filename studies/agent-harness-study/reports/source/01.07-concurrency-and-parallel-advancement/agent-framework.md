# Source Analysis: agent-framework

## 01.07 Concurrency and Parallel Advancement

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `sources/agent-framework` |
| Language / Stack | Python 3.11+ (asyncio) + .NET 9 (C#). Python is the primary analyzed surface; the .NET implementation is in `sources/agent-framework/dotnet` and mirrors the Python design. |
| Analyzed | 2026-07-03 |

## Summary

The framework exposes a **two-layered** concurrency model that is internally coherent and well documented:

1. **Per-superstep intra-workflow concurrency** in the **Pregel-style graph runner** (`sources/agent-framework/python/packages/core/agent_framework/_workflows/_runner.py`). One iteration drains all queued messages from `_ctx.drain_messages()`, then fans out across all source executors with `asyncio.gather` (`_runner.py:202-203`), and within each source executor across all outgoing edge runners also with `asyncio.gather` (`_runner.py:210`). Edge runners of the same source are serialized in order so per-edge message ordering is preserved (`_runner.py:182-203`), but edges to different targets execute concurrently.
2. **Whole-workflow isolation** for sub-workflows via `WorkflowExecutor` (`_workflows/_workflow_executor.py`). Each invocation gets its own `ExecutionContext` keyed by UUID (`_workflow_executor.py:299-301, 376-383`); pending requests/responses are routed per-execution (`_workflow_executor.py:653-676`) and the docstring explicitly markets "concurrent executions are fully isolated" (`_workflow_executor.py:197-264`). A test (`packages/core/tests/workflow/test_sub_workflow.py:361-464`) verifies 5 concurrent sub-workflow executions complete with all results collected.

Concurrency primitives in use:
- `asyncio.gather` is the dominant fan-out primitive (no `TaskGroup` adoption observed).
- `asyncio.Lock` is used to serialize lifecycle/connect-close transitions in MCP (`_mcp.py:445-447, 769-770`), file-memory (`_harness/_file_memory.py:205`), todo mutations (`_harness/_todo.py:484-490`), sessions (`_sessions.py:1103-1108`, striped-lock set of size `_FILE_LOCK_STRIPE_COUNT`), file-access harness (`_harness/_file_access.py:514`), and memory stores (`_harness/_memory.py:933-1043`).
- `asyncio.Queue` carries events from producers to the consumer-side runner (`_runner_context.py:289, 411, 769`).
- `asyncio.create_task` is used for: a single "iteration vs event-stream" race per superstep (`_runner.py:105-120`), MCP lifecycle-owner workers (`_mcp.py:770`), background sub-agent dispatch in `BackgroundAgentsProvider` (`_harness/_background_agents.py:341, 466`), and MCP tool reloads (`_mcp.py:1297, 1532-1539`).
- `asyncio.Semaphore` is used exactly once, in the experimental GAIA benchmark runner (`packages/lab/gaia/agent_framework_lab_gaia/gaia.py:432, 574`) — `parallel=N` is the cap on simultaneously running benchmark tasks.
- `concurrent.futures.ThreadPoolExecutor` is used **only** by the experimental `hyperlight` code-actor (one single-thread executor per VM, `packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:126`) and by `asyncio.to_thread` adapters elsewhere (sync function executors, sync filesystem ops, sync checkpoint I/O). There are **no multi-thread pools inside the core framework**.

Concurrency *invariants* enforced by the design:
- **A single workflow instance is single-run at a time.** `_workflow.py:361-383` and `_functional.py:1228-1231` both raise `"Workflow is already running. Concurrent executions are not allowed."` Three dedicated tests assert this (`packages/core/tests/workflow/test_workflow.py:744-829` and `test_functional_workflow.py:820-847`). This is enforced by a plain boolean flag (`_is_running`), not a lock — the comment at `_workflow.py:368-377` justifies it because a workflow owns "a single enum reference whose assignment is atomic under the CPython GIL".
- **Multiple sub-workflow executions on the same `WorkflowExecutor` are explicitly allowed** (`_workflow_executor.py:299`, `test_sub_workflow.py:361-464`) but rely on dict mutation which is *not* locked. Concurrent sub-workflow execution is only safe if the wrapped workflow and its executors are stateless; the docstring warns: *"For proper isolation, ensure that the wrapped workflow and its executors are stateless"* (`_workflow_executor.py:226-235`).
- **Per-edge message order is preserved.** The runner explicitly documents this contract at `_runner.py:160-179`: "Multiple messages from a source to the same target are delivered in the order they were sent." Implemented by iterating sequentially through `source_messages` for each edge runner (`_runner.py:189-193`).
- **Fan-in is all-or-nothing per superstep.** `FanInEdgeRunner._is_ready_to_send()` (`_edge_runner.py:399-401`) buffers messages keyed by source executor ID and only emits when every source has produced at least one. The aggregated message preserves source order (`_edge_runner.py:351`).
- **Function-call batches are concurrent by default.** The function-invocation loop runs all function calls from a single model response in one `asyncio.gather` (`_tools.py:1821-1823`). `FunctionInvocationConfiguration.max_function_calls` is *post-batch* (`_tools.py:1355-1364`), so an over-limit batch still runs to completion.

Failure-collection contract: `asyncio.gather` is used with default behavior everywhere except `_mcp.py:1539` (cleanup, `return_exceptions=True`) and `_functional.py:546-548` (uses synchronous result caching, not `gather`). When a branch in a `FanOutEdgeRunner` raises, the gather propagates the first exception; the runner swallows nothing. The `ExecutorFailedEvent` is emitted (`_executor.py:281-285`) before re-raising. `WorkflowExecutor._handle_response` (`_workflow_executor.py:647-697`) accumulates responses and waits for the full batch before resuming the sub-workflow, so partial success within a batch is held until the last arrives (no early-fire semantics).

`ConcurrentBuilder` (`packages/orchestrations/agent_framework_orchestrations/_concurrent.py:177-431`) is the high-level fan-out / fan-in orchestrator: it wires dispatcher -> `add_fan_out_edges` -> participants -> `add_fan_in_edges` -> aggregator (`_concurrent.py:425-429`). Participants can be either agents (`AgentExecutor`) or arbitrary `Executor`s. There is **no built-in cap on fan-out width** — concurrency is bounded only by the number of participants.

The **durable orchestration host** (`packages/durabletask/agent_framework_durabletask/_workflows/orchestrator.py:631-797`) translates the same graph topology into Azure Durable Functions primitives: `_prepare_all_tasks` collapses per-agent messages so each agent runs at most once per superstep (`orchestrator.py:663-679`), then `ctx.task_all(all_tasks)` schedules them in one fan-out call (`orchestrator.py:796`). Remaining messages for an agent are processed sequentially after the parallel batch (`orchestrator.py:809-818`).

Bottom line: read-only work in `Workflow.run()` and `FunctionalWorkflow.run()` runs as fast as Python's event loop permits (true I/O concurrency); stateful work is serialized at the workflow-instance level and at per-resource Locks. The model is small, observable, and explicitly bounded — there is no global thread pool, no multi-process, and no semaphore at the workflow level. It is mature on the asyncio axis but explicit on the threading/process axis.

## Rating

**8/10 — Clear model with tests, explicit interfaces, and operational safeguards.**

The model is internally coherent and well documented: the ordering rules, the per-superstep gather pattern, the per-edge serialization, the sub-workflow isolation, and the "no concurrent workflow runs" guard are all stated in code comments, docstrings, and have dedicated tests. The `WorkflowExecutor` documentation explicitly markets concurrent sub-workflow support with isolation guarantees (`_workflow_executor.py:197-264`). Failure events (`ExecutorFailedEvent`) are emitted *before* exception propagation (`_executor.py:281-285`).

What keeps it short of 9–10:
1. The `_is_running` flag is a plain boolean with no `asyncio.Lock` (`_workflow.py:361, 379-383`), relying on a GIL comment (`_workflow.py:368-377`) — a multi-thread caller could in principle race the flag.
2. `WorkflowExecutor` documents the concurrent-execution contract but warns callers to keep the wrapped workflow stateless (`_workflow_executor.py:226-235`). The bookkeeping dicts (`_execution_contexts`, `_request_to_execution`) are not synchronized.
3. `asyncio.gather` is used without `return_exceptions=True` in the runner and edge runners, so the first failing branch cancels the rest of the gather but does not collect partial results.
4. There is no built-in back-pressure: fan-out is unbounded by participant count, and `max_function_calls` is enforced *after* a batch, not *before* (`_tools.py:1355-1364`).
5. `asyncio.TaskGroup` (Python 3.11+) is not adopted; `asyncio.gather` everywhere means a single bad exception in a batch can mask others.

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Per-superstep gather across sources and edges | `await asyncio.gather(*tasks)` for source fan-out and edge fan-out | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_runner.py:202-210` |
| Iteration task raced against event streaming via `asyncio.create_task` and `wait_for` | Live event polling while superstep runs | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_runner.py:105-120` |
| Per-edge message ordering preserved (sequential await within edge runner, parallel across edges) | Comment + sequential loop | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_runner.py:160-203` |
| Fan-out runner uses `asyncio.gather(*[send_to_edge(edge) for edge in deliverable_edges])` | Parallel delivery to selected targets | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_edge_runner.py:281-287` |
| Fan-in buffers per-source and only emits when every source has data | Aggregated list of `messages_to_send`; order preserved | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_edge_runner.py:305-401` |
| Sub-workflow output / intermediate fan-out uses `asyncio.gather` | `ctx.yield_output` and `ctx.send_message` fanned out | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_workflow_executor.py:532-583` |
| WorkflowExecutor per-execution context (UUID keyed) for concurrent sub-workflow invocations | `_execution_contexts` and `_request_to_execution` dicts | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_workflow_executor.py:298-302, 376-383, 647-697` |
| WorkflowExecutor concurrent-execution support and isolation contract | Docstring section "Concurrent Execution Support" | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_workflow_executor.py:197-235` |
| Concurrent sub-workflow execution test (5 simultaneous emails) | `test_concurrent_sub_workflow_execution` | `sources/agent-framework/python/packages/core/tests/workflow/test_sub_workflow.py:361-464` |
| Function-call batch in a single model response runs concurrently | `asyncio.gather` over `_auto_invoke_function` | `sources/agent-framework/python/packages/core/agent_framework/_tools.py:1767-1830` |
| `max_function_calls` enforced after a batch, not before | Docstring note on best-effort semantics | `sources/agent-framework/python/packages/core/agent_framework/_tools.py:1355-1364` |
| Event queue used to interleave iteration with live event streaming | `asyncio.Queue[WorkflowEvent]` | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_runner_context.py:289, 411` |
| Single-instance concurrent-run guard (boolean flag, no lock) | `_is_running` / `_ensure_not_running` | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_workflow.py:361-388, 750` |
| Functional workflow's identical concurrent-run guard | `@workflow` decorator runtime guard | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_functional.py:683, 832, 1228-1234` |
| Concurrent execution-prevention tests (non-stream, streaming, mixed) | Three dedicated tests | `sources/agent-framework/python/packages/core/tests/workflow/test_workflow.py:744-829` and `sources/agent-framework/python/packages/core/tests/workflow/test_functional_workflow.py:820-847` |
| `asyncio.Lock` on MCP lifecycle / function load | Three named locks | `sources/agent-framework/python/packages/core/agent_framework/_mcp.py:445-448` |
| MCP lifecycle owner via `asyncio.Queue` + `asyncio.create_task` worker task | `_lifecycle_owner_task` | `sources/agent-framework/python/packages/core/agent_framework/_mcp.py:764-819` |
| MCP tool-reload cleanup uses `asyncio.gather(..., return_exceptions=True)` | Cancellation cleanup | `sources/agent-framework/python/packages/core/agent_framework/_mcp.py:1532-1539` |
| `asyncio.Lock` in memory and todo mutation paths | `_mutation_lock`, `_store_async_lock`, `_topic_async_lock`, `_state_async_lock` | `sources/agent-framework/python/packages/core/agent_framework/_harness/_todo.py:484-490`, `sources/agent-framework/python/packages/core/agent_framework/_harness/_memory.py:933-1043`, `sources/agent-framework/python/packages/core/agent_framework/_harness/_file_memory.py:205`, `sources/agent-framework/python/packages/core/agent_framework/_harness/_file_access.py:514` |
| Session stripe lock set (fixed-size asyncio.Lock pool) | `_FILE_LOCK_STRIPE_COUNT` | `sources/agent-framework/python/packages/core/agent_framework/_sessions.py:1103-1108` |
| Background-agents provider uses `asyncio.create_task` per sub-agent run | Two start paths | `sources/agent-framework/python/packages/core/agent_framework/_harness/_background_agents.py:341, 466` |
| `asyncio.wait(..., return_when=FIRST_COMPLETED)` to wait on multiple background tasks | First-completion semantics | `sources/agent-framework/python/packages/core/agent_framework/_harness/_background_agents.py:376-379` |
| Sync function executors dispatched via `asyncio.to_thread` (default thread pool) | FunctionExecutor wraps sync funcs | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_function_executor.py:137-151` |
| Sync checkpoint I/O dispatched via `asyncio.to_thread` | `_write_atomic`, `_read`, `_list_checkpoints`, `_delete`, `_list_ids` | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_checkpoint.py:323, 350, 390, 410, 450` |
| Sync filesystem harness dispatched via `asyncio.to_thread` | `write/read/delete/list/list_directories/file_exists/mkdir` | `sources/agent-framework/python/packages/core/agent_framework/_harness/_file_access.py:793-995` |
| `ConcurrentBuilder` wires fan-out → fan-in → aggregator | Builder pattern | `sources/agent-framework/python/packages/orchestrations/agent_framework_orchestrations/_concurrent.py:177-431` |
| `ConcurrentBuilder` accepts both `SupportsAgentRun` agents and arbitrary `Executor`s as participants | No per-participant cap, no shared semaphores | `sources/agent-framework/python/packages/orchestrations/agent_framework_orchestrations/_concurrent.py:211-265, 357-379` |
| Aggregator fan-out (gather over outputs / intermediate outputs) | `asyncio.gather(*[ctx.yield_output/send_message(...)])` | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_workflow_executor.py:565-583` |
| `SequentialBuilder` is sequential (no gather); uses `add_edge` in order | Sequential chain wiring | `sources/agent-framework/python/packages/orchestrations/agent_framework_orchestrations/_sequential.py:63-269` |
| Group-chat broadcast to participants uses `asyncio.gather` | `_broadcast_messages_to_participants` | `sources/agent-framework/python/packages/orchestrations/agent_framework_orchestrations/_base_group_chat_orchestrator.py:411-441` |
| Durable orchestrator schedules prepared tasks in one fan-out call (`ctx.task_all`) | Phase 2 parallel batch | `sources/agent-framework/python/packages/durabletask/agent_framework_durabletask/_workflows/orchestrator.py:780-797` |
| Durable orchestrator collapses per-agent messages so each agent runs once per superstep | `_prepare_all_tasks` | `sources/agent-framework/python/packages/durabletask/agent_framework_durabletask/_workflows/orchestrator.py:625-681` |
| `asyncio.Semaphore` used only in the experimental GAIA benchmark runner | `semaphore = asyncio.Semaphore(parallel)` | `sources/agent-framework/python/packages/lab/gaia/agent_framework_lab_gaia/gaia.py:574` |
| GAIA per-task run acquires semaphore, then runs `task_runner(task)` under `wait_for` timeout | Bounded parallel task execution | `sources/agent-framework/python/packages/lab/gaia/agent_framework_lab_gaia/gaia.py:431-458` |
| Handoff orchestrator disables parallel tool calls to avoid race conditions | Single-tool-call semantics | `sources/agent-framework/python/packages/orchestrations/agent_framework_orchestrations/_handoff.py:285` |
| Bounded batch dispatch pattern (sample-validation coordinator) | `BatchCoordinatorExecutor` caps active workers, dispatches via superstep-batched messages | `sources/agent-framework/python/scripts/sample_validation/create_dynamic_workflow_executor.py:190-235` |
| `concurrent.futures.ThreadPoolExecutor` (single-thread, named) only in hyperlight | `max_workers=1` for sandboxed code execution | `sources/agent-framework/python/packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:126` |

## Answers to Dimension Questions

### 1. What can run in parallel?

- **Inside one workflow run (per superstep):** all messages from different source executors run concurrently (`_runner.py:202-210`), and within a single source executor all edge runners run concurrently (`_runner.py:189-203`). Inside a `FanOutEdgeRunner`, all selected edges are dispatched concurrently with `asyncio.gather` (`_edge_runner.py:281-287`). Function calls in a single model response are dispatched concurrently (`_tools.py:1767-1830`). Group-chat participant broadcasts run concurrently (`_base_group_chat_orchestrator.py:411-441`).
- **Across workflows:** multiple `WorkflowExecutor` invocations of the same wrapped workflow run concurrently and are isolated by per-execution `ExecutionContext` (`_workflow_executor.py:299-301, 647-697`).
- **Background sub-agents:** the `BackgroundAgentsProvider` spawns each sub-agent as an independent `asyncio.Task` (`_background_agents.py:341, 466`) and supports `asyncio.wait(..., return_when=FIRST_COMPLETED)` for early completion (`_background_agents.py:376-379`).
- **Durable host:** all tasks prepared in a superstep are scheduled in a single fan-out call (`orchestrator.py:796`).
- **Outside the workflow:** sync function executors and sync I/O (checkpoints, file harness) are offloaded to the default thread pool via `asyncio.to_thread` (`_function_executor.py:137-151`, `_checkpoint.py:323-450`, `_file_access.py:793-995`). There is **no** process-level parallelism in the core framework.

### 2. What must be serialized?

- **Per edge runner:** messages from one source to one target are delivered in send order. Implementation: sequential `for message in source_messages: await _deliver_message_inner(...)` (`_runner.py:189-193`).
- **Per fan-in target:** aggregation only fires when every source has produced at least one message. Implementation: `FanInEdgeRunner._is_ready_to_send` checks `all(self._buffer[edge.source_id] for edge in self._edges)` (`_edge_runner.py:399-401`).
- **Per workflow instance:** `_ensure_not_running` rejects overlapping runs (`_workflow.py:361-388, 750` and `_functional.py:1228-1234`).
- **MCP lifecycle, file-memory, todo mutations, file-access writes, sessions:** protected by `asyncio.Lock` (`_mcp.py:445-448`, `_file_memory.py:205`, `_todo.py:484-490`, `_file_access.py:514`, `_sessions.py:1103-1108`).
- **Durable orchestrator:** per-agent messages are coalesced — only the first runs in the parallel batch, the rest are processed sequentially (`orchestrator.py:663-679, 809-818`).
- **Handoff orchestrator:** parallel tool calls are explicitly disabled to prevent the agent from invoking multiple handoff tools at once (`_handoff.py:285`).

### 3. How are shared resources protected?

- `asyncio.Lock` for in-process asyncio-mutated state: MCP lifecycle and reload (`_mcp.py:445-448`), file memory (`_file_memory.py:205`), file-access writes (`_file_access.py:514`), todo mutations (`_todo.py:484-490`), memory stores per (session, kind, key) (`_memory.py:933-1043`).
- Striped lock set for sessions to avoid one lock bottleneck for all sessions: `_FILE_LOCK_STRIPE_COUNT` asyncio.Lock objects hashed by file path (`_sessions.py:1103-1108`).
- WeakKeyDictionary of locks keyed by event loop and `AgentSession`/`MemoryStore` (`_memory.py:1024-1036`).
- Plain boolean `_is_running` flag (no lock) for workflow-level concurrent-run rejection; the comment explicitly justifies this on the CPython GIL (`_workflow.py:368-377`).
- Dict mutation in `WorkflowExecutor._execution_contexts`/`_request_to_execution` is **not** explicitly locked; the documented contract is "ensure that the wrapped workflow and its executors are stateless" (`_workflow_executor.py:226-235`).
- The `InProcRunnerContext` does **not** lock its `_messages` dict or its `asyncio.Queue`. Mutation is only performed from coroutines scheduled on the same event loop.
- `asyncio.Semaphore` is the only bounded-cap primitive, used exactly once (GAIA benchmark: `semaphore = asyncio.Semaphore(parallel)` in `gaia.py:574`).

### 4. Are results ordered deterministically?

- **Per-edge delivery is deterministic in send order** (`_runner.py:160-179`). Within a single superstep, an executor that sends multiple messages to the same edge receives them at the target in order.
- **Fan-in aggregated list is deterministic in source order**, because the iteration is `[msg for edge in self._edges for msg in self._buffer[edge.source_id]]` where `self._edges` is the `FanInEdgeGroup.edges` list (`_edge_runner.py:351`).
- **Fan-out target order is not guaranteed when targets are independent.** `asyncio.gather` returns results in argument order, but executor side effects (LLM calls, I/O) may finish in any order. The FanOutEdgeRunner returns `any(results)` (`_edge_runner.py:287`), discarding per-target completion order.
- **ConcurrentBuilder aggregator order is deterministic in participant order** (matches the `sources` configuration of the fan-in edge group). The comment at `_concurrent.py:84-92` says: *"in deterministic participant order matching the fan-in `sources` configuration."*
- **Workflow outputs from a multi-executor run are not guaranteed in source order** — they are ordered by `yield_output` invocation time, which depends on executor scheduling. The OutputDesignation pattern (`_workflow.py:170-203`) explicitly hides / labels outputs rather than ordering them.
- **Sub-workflow responses are correlated by request_id**, not by delivery order (`_workflow_executor.py:443-448, 647-697`).

### 5. What happens when one parallel branch fails?

- **In `asyncio.gather` (default) in the runner/edge runner:** the first exception propagates and the gather is cancelled. Partial completion is *not* preserved in a structured way; only the executor that raised produced an `ExecutorFailedEvent` (`_executor.py:281-285`). The runner surfaces pending events before re-raising (`_runner.py:122-130`).
- **In the function-call batch:** `_try_execute_function_calls` catches `MiddlewareTermination` and `UserInputRequiredException` per-call and returns `(Content, should_terminate)` tuples; `should_terminate = any(...)` triggers loop exit (`_tools.py:1772-1830`). A normal exception inside a function call surfaces as a `function_result` `Content` with `exception=` set.
- **In `WorkflowExecutor`:** a sub-workflow `failed` event triggers a `WorkflowEvent.error` to the parent and an exception with the sub-workflow ID (`_workflow_executor.py:607-621`). `WorkflowExecutor` does **not** short-circuit parallel sub-workflows on one failing; other pending executions continue (they are tracked in `_execution_contexts`).
- **Background agents:** a background task's failure is captured into the `BackgroundTaskInfo` record (`status=FAILED`, `error_text=...`) and surfaced via `background_agents_get_task_results` rather than raised into the parent agent (`_background_agents.py:410-418`).
- **Partial success is preserved structurally:** even when one branch fails, the runner drains pending events and yields `ExecutorFailedEvent` *before* propagating the exception (`_runner.py:122-130, _executor.py:281-285`). The `WorkflowEvent.failed` is emitted in `_run_workflow_with_tracing` before `raise` (`_workflow.py:606-628`).
- **`asyncio.gather(..., return_exceptions=True)` is used only in two places:** MCP reload cleanup (`_mcp.py:1539`) and tests (`tests/core/test_mcp.py:1831, 3588, 3619`). The core runner does not adopt this pattern.

## Architectural Decisions

- **Two-layer concurrency**: graph-level concurrency inside a workflow is real (`asyncio.gather`); workflow-instance-level concurrency is explicitly forbidden (`_is_running` flag). The flag is intentionally lock-free, justified by the CPython GIL (`_workflow.py:368-377`). This is a deliberate, documented trade — keep workflow instances single-threaded, rely on `WorkflowExecutor` for parallel composition instead.
- **Per-superstep Pregel model**: the runner processes all queued messages in one iteration with bounded fan-out, drains events for live streaming, commits state, and creates checkpoints at the boundary (`_runner.py:99-153`). This collapses otherwise-N concurrent branches into a single execution step, which is what makes ordering deterministic across iterations.
- **Per-edge message ordering**: `EdgeRunner.send_message` is called sequentially for messages from one source (`_runner.py:189-193`); the gather is over edge runners, not messages. This preserves the common "send A then B to the same target" semantics.
- **All-or-nothing fan-in**: `FanInEdgeRunner._is_ready_to_send` buffers per source (`_edge_runner.py:399-401`). Partial fan-in results are not exposed.
- **Sub-workflow isolation via UUID execution_id** (`_workflow_executor.py:376-383`): dict-keyed isolation avoids cross-talk between concurrent sub-workflow invocations. The isolation contract is documented as "wrapped workflow and its executors must be stateless" (`_workflow_executor.py:226-235`).
- **Lock-based vs flag-based synchronization**: `asyncio.Lock` is used wherever shared mutable state crosses coroutines in a way the GIL cannot trivially protect (MCP lifecycle, file writes, sessions). The `_is_running` flag is a deliberate exception justified by the GIL.
- **Default thread pool only for blocking I/O**: sync function executors and sync filesystem/checkpoint operations go through `asyncio.to_thread` (`_function_executor.py:137-151`, `_checkpoint.py:323-450`, `_file_access.py:793-995`). The framework never creates a custom `ThreadPoolExecutor` in core (`packages/core/agent_framework/_tools.py:552` for `self.__call__`); the only multi-thread pool in the entire repo is the single-thread executor in the hyperlight sandbox (`packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:126`).
- **No back-pressure at the framework level**: fan-out width is bounded only by the participant count the caller declares. The function-call batch is bounded by `max_function_calls`, but only *after* the batch runs (`_tools.py:1355-1364`).

## Notable Patterns

- **Pregel-style supersteps** with `asyncio.gather` per edge runner and per source (`_runner.py:189-210`).
- **Live event streaming via `asyncio.Queue`** interweaved with iteration via `asyncio.wait_for(timeout=0.05)` (`_runner.py:103-120`, `_runner_context.py:336-341`).
- **Striped asyncio.Lock pool** to scale session-level locking (`_sessions.py:1103-1108`).
- **Per-event-loop WeakKeyDictionary of locks** so that locks created for one loop don't leak to another (`_memory.py:933-1043`).
- **Single-task lifecycle owner** with `asyncio.Queue` for serializing MCP connect/close requests, while accepting concurrent callers (`_mcp.py:445-448, 764-819`).
- **`asyncio.create_task` + first-completion polling** for "wait on any of N background tasks" (`_background_agents.py:341, 376-379`).
- **Durable-function fan-out** via `ctx.task_all(all_tasks)` after coalescing per-agent messages (`orchestrator.py:625-797`).
- **`@workflow` functional API** that explicitly delegates parallelism to user code: *"Native Python control flow (`if`/`else`, `for`, `asyncio.gather`) is used for branching and parallelism"* (`_functional.py:12, 647`).
- **`ConcurrentBuilder`** as the public, agent-friendly parallel composition surface: dispatcher → fan-out → participants → fan-in → aggregator, with optional HITL between participants and aggregation (`_concurrent.py:177-431`).

## Tradeoffs

- **GIL-bound for CPU work.** Parallelism is real for I/O-bound (LLM calls, MCP, file I/O) but Python CPU-bound code does not speed up under gather. The framework does not address this with `ProcessPoolExecutor` or `multiprocessing` anywhere in core.
- **First-failure-wins in `asyncio.gather`.** No `return_exceptions=True` in the runner or edge runner, so a single failed branch cancels the rest of the gather. The structured `ExecutorFailedEvent` is emitted, but partial completion is only preserved through the event stream, not as data.
- **No built-in concurrency limit.** Fan-out is unbounded by participant count, and the only `asyncio.Semaphore` is in the experimental GAIA benchmark runner. A caller who creates 1000 concurrent sub-workflow invocations on one `WorkflowExecutor` will queue 1000 dict mutations.
- **Documented stateless contract for sub-workflows.** `WorkflowExecutor`'s concurrent-execution safety hinges on the wrapped workflow being stateless (`_workflow_executor.py:226-235`). The dict-mutation bookkeeping is not lock-protected.
- **`max_function_calls` is best-effort post-batch** (`_tools.py:1355-1364`). A model that requests 20 parallel calls when the cap is 10 will execute all 20 before the loop notices.
- **Lock-free `_is_running` flag.** Documented and tested, but a multi-threaded caller could in principle race it. The risk is low because the flag is only flipped from coroutines on the same event loop.
- **Sync function executors run on the default loop-owned executor** with no isolated pool. Concurrent sync calls share the default thread pool's threads.

## Failure Modes / Edge Cases

- **Concurrent `Workflow.run` calls** on the same instance raise `RuntimeError("Workflow is already running...")` (`_workflow.py:381-382`, `_functional.py:1230`). This is asserted in three tests (`test_workflow.py:744-829`).
- **One branch raising in a gather** propagates as the runner error; pending events are drained first (`_runner.py:122-130`). The `ExecutorFailedEvent` is emitted from inside the failing executor (`_executor.py:281-285`), so consumers see the structured event even on raise.
- **Sub-workflow failed event** is re-emitted to the parent as `WorkflowEvent.error` with the sub-workflow ID and error type/message (`_workflow_executor.py:607-621`).
- **Background agent task exception** is captured into `BackgroundTaskInfo(status=FAILED, error_text=...)` and surfaced via `background_agents_get_task_results` (`_background_agents.py:410-418`). It does not raise into the parent.
- **Function-call batch exception** in `UserInputRequiredException` is converted into a `function_result` Content with an explanatory message (`_tools.py:1800-1819`). `MiddlewareTermination` short-circuits the batch (`_tools.py:1790-1799`).
- **MCP lifecycle owner stop**: pending queued items in `_lifecycle_queue` get `RuntimeError("MCP lifecycle owner stopped unexpectedly.")` set on their futures during cleanup (`_mcp.py:809-818`).
- **MCP reload cleanup cancellation**: pending reload tasks are cancelled and awaited with `return_exceptions=True` so one cancellation failure doesn't prevent others (`_mcp.py:1532-1539`).
- **Edge runner `_is_ready_to_send` short-circuits**: if any source in a `FanInEdgeGroup` never produces a message, the fan-in target is never invoked. There is no timeout.
- **`WorkflowExecutor._handle_response` accumulates a full batch** before resuming the sub-workflow; partial responses are stored but not delivered (`_workflow_executor.py:677-683`).
- **GAIA benchmark timeout** uses `asyncio.wait_for` per task (`gaia.py:458`), bounded by the `parallel` semaphore — both axes are configurable.

## Future Considerations

- **Adopt `asyncio.TaskGroup` (Python 3.11+)** in the runner to get structured concurrency: `async with asyncio.TaskGroup() as tg: tg.create_task(...)` would surface per-branch exceptions while preserving partial completion and avoiding the manual `asyncio.gather` cancellation semantics.
- **Back-pressure on fan-out**: add an optional `asyncio.Semaphore` to `ConcurrentBuilder` (or per-edge config) so callers can cap concurrent participant invocations. The GAIA implementation (`gaia.py:574`) already shows the pattern.
- **Partial-success collection**: when one branch in a `FanOutEdgeRunner` raises, collect the others' outputs before propagating. Currently the gather only surfaces one exception and discards the others.
- **Lock-protect `WorkflowExecutor._execution_contexts`** if concurrent sub-workflow execution becomes a hot path. Today the contract is "wrapped workflow must be stateless" (`_workflow_executor.py:226-235`); a lock would let stateful workflows participate.
- **`max_function_calls` enforcement before the batch**: convert the post-batch cap (`_tools.py:1355-1364`) into a pre-batch gate that splits or truncates the batch.
- **`_is_running` flag**: replace with `asyncio.Lock` if multi-thread callers are ever supported; today the GIL comment (`_workflow.py:368-377`) is the only safety net.
- **Durable-task fan-out width**: `ctx.task_all` is unbounded — durable orchestrations may want a sub-batch limit to stay within Durable Functions task hub quotas.

## Questions / Gaps

- **No evidence found** of `concurrent.futures.ProcessPoolExecutor` use anywhere in the source. CPU-bound parallelism is not in scope of this codebase.
- **No evidence found** of `multiprocessing` use anywhere in the source.
- **No evidence found** of `asyncio.TaskGroup` adoption (the framework uses `asyncio.gather` everywhere).
- **No evidence found** of framework-level back-pressure (e.g., bounded queues between supersteps). The only `asyncio.Semaphore` is the GAIA benchmark cap (`gaia.py:574`).
- **No evidence found** of cancellation tokens in user-facing parallel APIs. Background-agent tasks use `asyncio.create_task` without explicit cancellation tokens (`_background_agents.py:341, 466`).
- **Concurrency semantics for `.NET` implementation** not analyzed in depth (only Python was searched per source isolation). The same superstep / edge runner / WorkflowExecutor architecture is mirrored in `sources/agent-framework/dotnet/` per the `wf-source-gen-plan.md` and `agent-framework-dotnet.slnx` files, but specific `Task.Run`/`Parallel.ForEachAsync` patterns in C# were not searched in this analysis.

---

Generated by `dimensions/01.07-concurrency-and-parallel-advancement.md` against `agent-framework`.
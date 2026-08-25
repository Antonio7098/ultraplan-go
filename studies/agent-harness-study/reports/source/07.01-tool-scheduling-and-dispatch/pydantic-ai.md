# Source Analysis: pydantic-ai

## 07.01 Tool Scheduling and Dispatch

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python 3.10+; asyncio + anyio; agent loop as a `pydantic_graph` state machine |
| Analyzed | 2026-08-24 |

## Summary

pydantic-ai centralizes tool dispatch in a single module, `_tool_execution.py`, invoked from the `CallToolsNode` of the graph-based agent loop. A model response's tool calls are classified by kind (`function` / `output` / `external` / `unapproved` / `unknown`) and executed under one of three end-strategies — `'early'`, `'graceful'` (default), `'exhaustive'` — each implemented as a processor subclass sharing one base class. Function tools run concurrently via `asyncio.create_task` within *segments* split around per-tool `sequential=True` barriers; message-history parts are always re-assembled deterministically in emission order, while streaming events can surface in completion order (`'parallel'`) or emission order (`'parallel_ordered_events'`). A run-scoped mode ContextVar offers fully serial execution. Deferred tools (`external`, `unapproved`) are collected during the walk and resolved as a single batch at the end of the step, either inline by a capability handler or returned to the caller as a `DeferredToolRequests` result (the run's queue-like handoff). Synchronous tool bodies are offloaded to worker threads via anyio (or a user-supplied bounded executor). Dispatch is observable twice over: typed events on the run stream (`FunctionToolCallEvent`/`FunctionToolResultEvent`/...) and OpenTelemetry spans (`execute_tool {name}`) emitted by an instrumentation capability.

## Rating

**9 / 10** — The dispatch model is explicit and layered: strategy processors with documented semantics (`_tool_execution.py:265-296`), an execution-mode enum (`tool_manager.py:40-42`), per-tool barrier flags (`tools.py:583-591`), and deterministic history assembly whose rationale is written into the code (`_tool_execution.py:204-217`). It is proven under failure: sibling-task cancellation is drained explicitly (`_tool_execution.py:828-837`), partial results survive interruptions (`_agent_graph.py:2158-2172`), duplicate `tool_call_id`s fail closed (`_tool_execution.py:405-420`), and barrier/serialization behavior is directly tested (`tests/test_agent.py:7017`, `tests/test_toolsets.py:1317-1334`, `tests/realtime/test_session.py:2252`). It loses the last point because scheduling is bound to the caller's asyncio loop — there is no built-in worker pool, per-tool concurrency limiter, or persistent queue inside the process (cross-process deferral and durable engines are opt-in integrations), and both parallel paths carry complexity flagged `# noqa: C901`.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Graph entry point to dispatch | `ModelRequestNode.run()` returns `CallToolsNode`, which owns tool handling | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1127-1138` |
| Dispatch trigger | Response parts walked; `ToolCallPart`s collected and routed to `_handle_tool_calls` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1982-1986,2027-2031` |
| Dispatcher entry | `CallToolsNode._handle_tool_calls` prepares the step's `ToolManager` then calls `process_tool_calls` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2075-2157` |
| Strategy selection | `end_strategy` picks `_ExhaustiveProcessor`/`_EarlyProcessor`/`_GracefulProcessor` | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:300-308` |
| End-strategy type & default | `EndStrategy = Literal['early', 'graceful', 'exhaustive']`; Agent stores it at construction | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:99`; `pydantic_ai_slim/pydantic_ai/agent/__init__.py:503,626` |
| Call classification | Calls classified once by kind in `__post_init__`, preserving emission order | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:374-430` |
| Pre-dispatch usage gate | Projected tool-call count checked against `usage_limits` before any execution | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:445-448`; `pydantic_ai_slim/pydantic_ai/usage.py:553-560` |
| Validate-then-execute split | `validate_tool_call` produces `ValidatedToolCall`; execution consumes it separately | `pydantic_ai_slim/pydantic_ai/tool_manager.py:609-692,694-700` |
| Executor chain | `_call_tool` → `tool_manager.execute_tool_call` → hooks → `_raw_execute` → `toolset.call_tool` | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:654-734`; `pydantic_ai_slim/pydantic_ai/tool_manager.py:979-1027` |
| Parallel task launch (graceful path) | Segment tasks created with `asyncio.create_task(..., name=tool_name)`; `asyncio.wait` ALL/FIRST_COMPLETED for ordered vs completion-order events | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:806-827` |
| Parallel task launch (exhaustive path) | One task per executable index per segment; winner picked by emission order | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:1159-1222` |
| Barrier segmentation | `_segment_by_barriers` splits indices so `sequential=True` tools run alone | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:232-251,791-797,1141-1149` |
| Execution modes | `ParallelExecutionMode = Literal['parallel','sequential','parallel_ordered_events']` via ContextVar, default `'parallel'`; context manager + getter | `pydantic_ai_slim/pydantic_ai/tool_manager.py:40-46,170-185,230-238` |
| Per-tool sequential flag | `ToolDefinition.sequential: bool` documents barrier semantics | `pydantic_ai_slim/pydantic_ai/tools.py:583-591` |
| Deterministic history assembly | Parts appended sorted by index in `finally` even on exception; reveal dedupe pruned "at assembly ... in model call (index) order" | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:204-229,838-847,1233-1264` |
| Sibling cancellation cleanup | `cancel_and_drain(*tasks)` on CancelledError and BaseException so no orphaned tasks | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:828-837,1209-1214` |
| Sync tools in threads | `run_in_executor` branches: disabled-inline / custom executor / `anyio.to_thread.run_sync` | `pydantic_ai_slim/pydantic_ai/_utils.py:183-200`; `pydantic_ai_slim/pydantic_ai/_function_schema.py:83-89` |
| Tool timeout → retry | `anyio.fail_after(timeout)` wraps the call; `TimeoutError` becomes `ModelRetry('Timed out...')` | `pydantic_ai_slim/pydantic_ai/toolsets/function.py:684-693` |
| Bounded custom executor | `UseThreadExecutor` capability swaps ephemeral threads for a supplied `Executor` | `pydantic_ai_slim/pydantic_ai/capabilities/thread_executor.py:17-55` |
| Deferred-call batching | Deferred (`external`/`unapproved`) calls collected during the walk, resolved as one batch; run can end with `DeferredToolRequests` | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:910-1050`; `pydantic_ai_slim/pydantic_ai/tools.py:593-602` |
| Retry-wins invariant | A function-tool retry suppresses an otherwise-valid final output next round | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:285-290,881-908` |
| Dispatch events | `FunctionToolCallEvent`, `OutputToolCallEvent`, `FunctionToolResultEvent`, `OutputToolResultEvent`, `ToolAvailabilityDeltaEvent`, `DeferredToolRequestsEvent` | `pydantic_ai_slim/pydantic_ai/messages.py:3976-4069` |
| OTel tracing of execution | Instrumentation capability creates span `execute_tool {name}` around `wrap_tool_execute`; deferral/failure-stage attributes defined | `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:322-393,499-515`; `pydantic_ai_slim/pydantic_ai/_instrumentation.py:665-707` |
| Event-stream wrapping hook | Capabilities wrap the whole node event stream (`wrap_run_event_stream`), memoized once per node | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1874-1889` |
| Partial capture on failure | On exception, completed tool returns are appended to history with request state `'interrupted'` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2158-2172` |
| Barrier tests | `test_sequential_tool_is_a_per_tool_barrier` asserts barrier runs alone; exhaustive output-barrier ordering test; streaming-path variant | `tests/test_agent.py:7017-7100`; `tests/test_streaming.py:4124` |
| Mode tests | Per-tool flag doesn't change run-scoped mode; `parallel_execution_mode('sequential')` read back; realtime serialization test | `tests/test_toolsets.py:1317-1334`; `tests/realtime/test_session.py:2252-2269` |
| Durable-execution boundary | Durable engines durably wrap executing toolsets (tools become steps/tasks/activities); same-process cancellation tokens rejected at that boundary | `pydantic_ai_slim/pydantic_ai/durable_exec/_runtime_toolsets.py:1-18,38-49` |
| Adjacent concurrency control | `ConcurrencyLimiter` (anyio `CapacityLimiter` + OTel waiting spans) limits whole runs/model requests, not individual tools | `pydantic_ai_slim/pydantic_ai/concurrency.py:77-247`; `pydantic_ai_slim/pydantic_ai/models/concurrency.py:85-107`; `pydantic_ai_slim/pydantic_ai/agent/__init__.py:694,1814` |

## Answers to Dimension Questions

**1. How does a tool call start?**
A model response lands in `CallToolsNode` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1816`), whose stream handler collects `ToolCallPart`s from the response parts (`_agent_graph.py:1982-1986`) and calls `_handle_tool_calls` (`_agent_graph.py:2075`). That refreshes discovered tools, prepares the step-scoped `ToolManager` via `for_run_step` (`_agent_graph.py:2109`; `pydantic_ai_slim/pydantic_ai/tool_manager.py:187-220`), projects tool-call usage limits (`_tool_execution.py:445-448`), and hands everything to `process_tool_calls` (`_agent_graph.py:2147-2157`; `_tool_execution.py:254-321`), which instantiates the strategy processor. Each call is first validated (schema + hooks, `tool_manager.py:609-692`) before its body ever runs.

**2. Is tool execution inline or queued?**
Inline within the graph step's async iteration — there is no internal job queue. Parallelism is achieved with `asyncio.create_task` per call inside barrier segments (`_tool_execution.py:806-818`, exhaustive variant `1180`); single-call/barrier segments run awaited inline (`_tool_execution.py:800-805`). Synchronous tool bodies are offloaded to worker threads (`anyio.to_thread.run_sync`, a custom `Executor`, or inline under `disable_threads` — `_utils.py:183-200`). The only cross-step handoff is deliberate deferral: `external`/`unapproved` calls are stubbed/collected and either resolved inline by a capability handler or returned to the application as a `DeferredToolRequests` final result (`_tool_execution.py:910-1050`) — an out-of-process continuation rather than an in-process queue.

**3. Are tool calls ordered?**
Three distinct orders are managed explicitly. *Start order* follows emission order (calls are zipped against their index lists). *Completion order* is scheduler-dependent for parallel tasks; under `'parallel'` result events yield as tasks complete (`FIRST_COMPLETED`, `_tool_execution.py:820-827`), while `'parallel_ordered_events'` waits for the segment and replays events in emission order (`ALL_COMPLETED`, `_tool_execution.py:813-818`; exhaustive buffering at `1153-1155,1205-1208`). *Message-history order* is always emission order regardless of scheduling: parts are keyed by index and appended `for index in sorted(...)` in a `finally` block (`_tool_execution.py:838-847`), and the exhaustive path appends in `executable_indices` order (`1233-1264`). Tool-reveal deltas are pruned in history order precisely so parallel siblings cannot race for ownership (`_tool_execution.py:204-217,725-728`). Full determinism is available via `sequential=True` barriers, `ToolManager.parallel_execution_mode('sequential')` (`tool_manager.py:170-185`), or durable-engine workflows where tools become checkpointed activities (`durable_exec/_runtime_toolsets.py:1-8`).

**4. Can tools be batched?**
Yes, at three levels. (a) Function/unknown calls are validated as a batch upfront before any executes (`_tool_execution.py:582-608,625-652`), then executed together within their barrier-free segment — the natural batch unit. (b) Under `'graceful'`, function calls accumulate into `pending_functions` batches flushed before each output tool (`_tool_execution.py:1085-1106`). (c) Deferred calls are grouped by kind and resolved as a single announced batch (`DeferredToolRequestsEvent`, `messages.py:4060-4069`; collection/resolution at `_tool_execution.py:921-1050`). There is no user-facing API to coalesce multiple model turns into one dispatch batch.

**5. Is dispatch observable?**
Strongly. Every call emits a typed pre-event with `args_valid` status and a typed result event (`messages.py:3976-4047`), plus `ToolAvailabilityDeltaEvent` and `DeferredToolRequestsEvent` for mid-run reveals and pending external work. The whole event stream is wrapped by capabilities (`wrap_run_event_stream`, `_agent_graph.py:1884-1889`), and an instrumentation capability opens an OTel child span per execution named `execute_tool {tool_name}` with GenAI attributes (`gen_ai.tool.call.arguments`/`.result`) plus deferral and failure-stage attributes (`capabilities/instrumentation.py:322-393,499-515`; `_instrumentation.py:700-707`). Waiting-for-slot time is traced too when the adjacent run/model-level `ConcurrencyLimiter` engages (`concurrency.py:192-243`).

## Architectural Decisions

1. **One dispatcher module, three strategy processors.** All scheduling lives behind `process_tool_calls` (`_tool_execution.py:254-321`) with a template-method base class (`_ToolCallProcessor`, `_tool_execution.py:324-455`) and thin subclasses `_EarlyProcessor`/`_GracefulProcessor`/`_ExhaustiveProcessor` (`_tool_execution.py:1053-1107,1110+`). Behavior differences between strategies are data-independent and localized.
2. **Barriers instead of global serialization.** Rather than forcing all-or-nothing serial execution, `_segment_by_barriers` (`_tool_execution.py:232-251`) lets independent tools overlap around `sequential=True` islands — a finer-grained contract than v1's "one sequential tool serializes the batch" (documented at `tool_manager.py:230-238`).
3. **Deterministic history, optional event order.** The team accepted nondeterministic *completion* but refused nondeterministic *history*: index-keyed assembly plus assembly-time reveal pruning make the next model request reproducible irrespective of task scheduling (`_tool_execution.py:204-229,838-847`).
4. **Validate/execute separation.** `ValidatedToolCall` (`tool_manager.py:56-94`) lets the pipeline emit accurate `args_valid` on call events, short-circuit deferred kinds before execution, and share validation across resume paths.
5. **Defer outward, don't queue inward.** Long-running/human-gated work leaves the process as `DeferredToolRequests` matched back by `tool_call_id` (`_tool_execution.py:964-1050`); durable engines integrate at the toolset boundary (`durable_exec/_runtime_toolsets.py:1-18`) instead of pydantic-ai embedding a broker.

## Notable Patterns

- **ContextVar-scoped run configuration**: `parallel_execution_mode` is ambient per-run state settable around any `run` call (`tool_manager.py:44-46,170-185`), read at dispatch time via `get_parallel_execution_mode` (`_tool_execution.py:787-789,1143-1145`).
- **Async-generator outputs via mutable arguments**: `output_parts` and `output_final_result` deque are in/out parameters because async iterators cannot return values (`_tool_execution.py:295-299`).
- **Partial-capture on interruption**: completed returns are appended to history even when the step throws, tagged `state='interrupted'` (`_agent_graph.py:2158-2172`); the exhaustive processor tracks per-index appends so a consumer closing the stream mid-loop cannot double-append (`_tool_execution.py:1171-1177,1265-1282`).
- **Cancellation hygiene**: sibling tasks are cancelled and drained on any exception escaping the wait loop (`_tool_execution.py:828-837`); timed-out sync tools are abandoned rather than blocking cancellation (`_utils.py:161-180`; `toolsets/function.py:688-691`); sub-agent self-cancellation is isolated into a failed tool return instead of tearing down the caller (`_tool_execution.py:41-62,701-702`).
- **Retry-wins invariant**: function-tool retries outrank a concurrent winning output, implemented by identity-tracked replacement of the winner's status part (`_tool_execution.py:892-908`).
- **Fail-closed ID integrity**: duplicate `tool_call_id`s are rejected wherever matching would otherwise be ambiguous — resume inputs (`_tool_execution.py:405-420`) and deferred batches (`967-973`).

## Tradeoffs

- **Complexity concentration**: `_call_tools` and the exhaustive `_run_strategy` are long, multi-concern generators carrying `# noqa: C901` (`_tool_execution.py:736,1121`); correctness relies on subtle `try/finally` interplay (non-idempotent prune guarded by a `pruned` flag, `_tool_execution.py:1236,1271-1276`).
- **Two parallel implementations**: graceful and exhaustive paths duplicate the segment/task/wait skeleton with different payload types; a docstring explicitly warns the deferred-result dispatch must be kept in sync between graph and non-graph callers (`tool_manager.py:1163-1165`).
- **Loop-bound scaling**: dispatch inherits the caller's event loop; there is no built-in per-tool/per-toolset concurrency cap (limits exist only at run level, `agent/__init__.py:694`, and model level, `models/concurrency.py:85-107`), so N unbounded tool tasks can start simultaneously.
- **Ephemeral thread defaults**: sync tools use anyio's thread pool, acknowledged as prone to accumulation in servers — hence the opt-in `UseThreadExecutor` (`capabilities/thread_executor.py:20-25`).
- **Sequential-mode cost**: `'sequential'` trades latency for determinism globally; per-tool barriers mitigate but require annotating tools (`tools.py:583-591`).

## Failure Modes / Edge Cases

- Orphaned asyncio tasks if a sibling raises while others run — mitigated by `cancel_and_drain` on both exception shapes (`_tool_execution.py:828-837,1209-1214`).
- Stream consumer aborts mid-dispatch — memoized wrapper and per-index append tracking prevent double-appends and stranded capability state (`_agent_graph.py:1865-1889`; `_tool_execution.py:1265-1282`).
- Duplicate `tool_call_id`s in history or deferred batches — hard `UserError`/`UnexpectedModelBehavior` instead of silent mis-binding (`_tool_execution.py:405-420,967-973`).
- Tool overrun past deadline — `TimeoutError` converted to `ModelRetry` so the model self-corrects (`toolsets/function.py:686-691`); retry budgets tracked per tool name across steps with success reset (`tool_manager.py:187-220`).
- Max-retries exhaustion inside a parallel output race — captured into `_OutputCallResult.raise_exc` and only raised if no other output won (`_tool_execution.py:483-503,573-578,1224-1231`).
- Run-level budget breach before dispatch — projected tool-call counts checked up front (`_tool_execution.py:445-448`; `usage.py:553-560`).
- Sub-agent cancellation leaking — normalized to a failed `ToolReturnPart` (`_tool_execution.py:41-62`).

## Future Considerations

- Introduce an optional per-toolset `AbstractConcurrencyLimiter` for tool dispatch, mirroring the existing run/model limiters (`concurrency.py:35-74` defines the extension point shape already).
- Unify the two segment-execution skeletons (graceful `_call_tools` and exhaustive `run_one`) around one shared scheduler helper to shrink the highest-complexity code in the module.
- Surface scheduling decisions (segment membership, barrier waits, mode) as structured events/spans — currently derivable only by reading code, not from the trace.

## Questions / Gaps

- No evidence of a persistent or distributed tool-execution queue *inside* a run; searched for queue/worker/scheduler abstractions across `pydantic_ai_slim/pydantic_ai` and found only (a) out-of-process `DeferredToolRequests` handoff (`_tool_execution.py:910-1050`) and (b) external durable engines wrapping toolsets as activities/steps (`durable_exec/_runtime_toolsets.py:1-18`). In-process scheduling is exclusively asyncio-task-based.
- No evidence of priority schemes beyond emission order or of tool-level timeouts enforced above the per-tool `timeout` field (`toolsets/function.py:684-693`); MCP toolsets were not audited for additional dispatch behavior in this pass.
- Whether the `'early'` strategy's skip-stubbing of function tools (`_tool_execution.py:1064-1074`) is observable to consumers purely via events was not verified end-to-end; the stub content constants exist (`_tool_execution.py:32-38`) but no dedicated event test was located for that specific branch.

---

Generated by `07.01-tool-scheduling-and-dispatch` against `pydantic-ai`.

# Source Analysis: pydantic-ai

## Dimension 07.02: Sequential vs Parallel Tool Execution

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (asyncio + anyio; agent loop built on `pydantic-graph`) |
| Analyzed | 2026-08-26 |

All citations below are relative to the source root `studies/agent-harness-study/sources/pydantic-ai/`.

## Summary

pydantic-ai executes the tool calls of a single model response **in parallel by default**, using `asyncio.create_task` per call, with a deterministic, developer-controlled escape hatch at two levels: a per-tool `sequential=True` barrier flag and a run-scoped `parallel_execution_mode('sequential')` context. The core engine is `_ToolCallProcessor._call_tools` (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:736-860`): calls are split into *segments* around barrier tools via `_segment_by_barriers` (`_tool_execution.py:232-251`) — non-barrier tools in a segment launch concurrently as tasks, while each barrier runs alone inline between segments. Three `end_strategy` processors (`_EarlyProcessor`, `_GracefulProcessor`, `_ExhaustiveProcessor`, `_tool_execution.py:1053-1291) control how output tools interleave with function tools, with `'graceful'` (default) batching function tools between sequential output-tool runs and `'exhaustive'` running everything in parallel.

Two ordering guarantees coexist deliberately: **message-history parts are always assembled in emission order** regardless of completion order (results collected into per-index dicts and appended by sorted index in `_tool_execution.py:838-847`), while **streamed events** are emitted in completion order under the default `'parallel'` mode or buffered into emission order under `'parallel_ordered_events'` (`_tool_execution.py:813-827`, `1155`, `1205-1208`). Failure handling is two-tier: model-visible failures (`ModelRetry`, `ToolFailed`) are isolated per tool as retry/failed parts so siblings continue (`_call_tool`, `_tool_execution.py:697-702`), but an unexpected exception cancels and drains remaining sibling tasks before propagating, with already-completed returns preserved as an `interrupted` history request (`_tool_execution.py:831-847`; `_agent_graph.py:2158-2172`). There is no semaphore bounding per-step fan-out — concurrency control is declarative (flags/modes), not resource-based.

## Rating

**9 / 10.**

The mechanism is explicit, tested, observable, and documented end-to-end: a public three-value `ParallelExecutionMode` literal (`tool_manager.py:40-46`), a documented per-tool barrier on `ToolDefinition.sequential` (`tools.py:583-591`), strategy processors with precisely specified winner/skip semantics (`_tool_execution.py:254-297`), deterministic history assembly even when siblings complete out of order (`_tool_execution.py:204-229`, `845-847`), cancellation that drains pending tasks (`_utils.py:312-330`; test `tests/test_agent.py:7346-7385`), partial-result capture under mid-batch failure (`tests/test_agent.py:7141-7179`), and docs whose claims match implementation (`docs/tools-advanced.md:736-782`). It is proven under failure: dedicated tests pin barrier isolation, outer-cancellation of pending tasks, ordered-event determinism for durable replay (`tests/test_agent.py:7387-7426`), and realtime parity (`tests/realtime/test_session.py:2252-2284`). It falls short of 10 only because a single step's batch fans out unbounded (no per-step concurrency cap or queue backpressure for tools — the existing limiter applies to concurrent *runs*, `agent/__init__.py:694`, `1814`), write-conflict safety rests entirely on developer-declared flags rather than any side-effect taxonomy or lock infrastructure, and execution uses raw asyncio primitives inside an otherwise anyio-based graph.

## Evidence Collected

Every entry cites paths relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Execution modes | `ParallelExecutionMode = Literal['parallel', 'sequential', 'parallel_ordered_events']`; ContextVar defaults to `'parallel'` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:40-46` |
| Run-scoped mode switch | `ToolManager.parallel_execution_mode()` contextmanager sets/resets the ContextVar | `pydantic_ai_slim/pydantic_ai/tool_manager.py:170-185` |
| Agent-facing API | `Agent.parallel_tool_call_execution_mode(mode)` delegates to `ToolManager.parallel_execution_mode` | `pydantic_ai_slim/pydantic_ai/agent/abstract.py:1881-1893` |
| Mode read at execution | `_call_tools` reads mode; `global_sequential` forces every index to be its own barrier | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:787-797` |
| Per-tool side-effect flag | `Tool(sequential=...)` stored on the tool; propagated into `ToolDefinition.sequential` | `pydantic_ai_slim/pydantic_ai/tools.py:332`, `430`, `502`; field doc `tools.py:583-591` |
| Output-tool barrier flag | `ToolOutput(sequential=...)`: "output tool runs alone, so function tools … emitted before it complete first" | `pydantic_ai_slim/pydantic_ai/output.py:125-150`; registration `agent/__init__.py:2491`, `2520` |
| Barrier predicate | `is_sequential(call)` returns `tool_def.sequential` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:240-247` |
| Barrier segmentation | `_segment_by_barriers`: barriers become single-element segments; others group into parallel segments | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:232-251` |
| Parallel task fan-out | `asyncio.create_task(call_tool(index))` per segment member; `asyncio.wait(FIRST_COMPLETED)` streams events on completion | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:807-827` |
| Ordered-events variant | `ALL_COMPLETED` then yields events per emission-order index; exhaustive variant buffers `function_events[index]` and replays in order | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:813-818`, `1155`, `1205-1208`, `1254-1257` |
| Exhaustive all-parallel strategy | Output + function tools launch together, segmented only by barriers; first valid output *by emission order* wins | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:1110-1222` |
| Graceful default strategy | Function tools accumulate into batches flushed before each output tool runs sequentially | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:1081-1107` |
| Deterministic history assembly | Results keyed by index; appended in `sorted(...)` order in `finally`, even on exception; reveal dedupe pruned in history order because "parallel siblings … race" | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:204-229`, `838-852` |
| Failure isolation (model-visible) | `ToolRetryError`/`ToolFailedError`/sub-agent `RunCancelled` converted to per-call parts; siblings unaffected | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:697-702`, `41-62` |
| Failure isolation (unexpected exceptions) | Sibling tasks cancelled & drained via `cancel_and_drain`; completed returns appended to `output_parts` in `finally` | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:828-847`; helper `_utils.py:312-330` |
| Partial capture surface | `CallToolsNode._handle_tool_calls` records partial `output_parts` as `state='interrupted'` request on `BaseException` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2158-2172` |
| Usage budget pre-check | Projected `tool_calls` checked against `UsageLimits.tool_calls_limit` before executing the batch | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:444-448`; limit `usage.py:431`, `553-559` |
| Run-level (not tool-level) limiter | `ConcurrencyLimiter` (anyio `CapacityLimiter`) wraps whole agent runs via `max_concurrency`, not individual tools | `pydantic_ai_slim/pydantic_ai/concurrency.py:95-128`; applied `agent/__init__.py:694`, `1814` |
| Provider-side switch | `ModelSettings.parallel_tool_calls` forwarded to providers (e.g. Mistral, Anthropic) | `pydantic_ai_slim/pydantic_ai/settings.py:232`; `models/mistral.py:311`; `models/anthropic.py:1651` |
| Sync tools off-loop | Sync functions run via thread executor (`run_in_executor`), so they parallelize across threads; optional bounded executor | `pydantic_ai_slim/pydantic_ai/_function_schema.py:81-89`; `_utils.py:200`; `agent/abstract.py:1895-1929` |
| Realtime second surface | Session dispatches each tool call as its own task honoring same mode/barriers via completion-event prerequisites | `pydantic_ai_slim/pydantic_ai/realtime/_session.py:2389-2457` |
| Barrier tests | `test_sequential_tool_is_a_per_tool_barrier` asserts barrier runs with zero in-flight siblings; output-tool barrier under exhaustive | `tests/test_agent.py:7017-7065`, `7067-7100` |
| Cancellation/failure tests | Outer cancel cancels pending tool tasks; mid-batch exception still surfaces completed returns; ordered events for DBOS replay | `tests/test_agent.py:7346-7385`, `7141-7179`, `7387-7426` |
| Docs alignment | "schedules them concurrently using `asyncio.create_task`"; barrier semantics; limiting note points at `UsageLimits(tool_calls_limit=...)` | `docs/tools-advanced.md:736-740`, `782`, `815-816` |

## Answers to Dimension Questions

1. **Can multiple tools run in one turn?** Yes — this is the default. The mode ContextVar defaults to `'parallel'` (`tool_manager.py:44-46`); every non-barrier call in a response's segment becomes its own `asyncio.Task` (`_tool_execution.py:807-810`). Under `'exhaustive'`, even output tools join the parallel batch (`_tool_execution.py:1131-1149`).
2. **Which tools are safe to parallelize?** Everything not marked `sequential=True` is treated as safe by the harness; safety itself is delegated to the developer. The flag is described as "a barrier that runs alone, not overlapping with other tool calls" (`tools.py:397-398`, `583-591`) and can be attached to function tools, schema tools (`tools.py:445`), and output tools via `ToolOutput(sequential=True)` (`output.py:125-150`; test `tests/test_agent.py:7067-7100`). There is no inferred read/write classification.
3. **Are write tools serialized?** Only if declared. A `sequential=True` tool serializes against its neighbors (before-tools finish, it runs alone, after-tools wait — `_tool_execution.py:232-251`, verified by the active-counter assertion in `tests/test_agent.py:7017-7065`), and `parallel_execution_mode('sequential')` serializes the entire run (`_tool_execution.py:787-797`; migration from v1's `sequential_tool_calls()` recorded in `docs/migration.md:23`). Notably, one barrier no longer forces the whole batch serial (the v1 behavior) — pinned by `tests/test_toolsets.py:1314-1334`. Undeclared writers get no protection.
4. **How are failures aggregated?** Three tiers. (a) Model-visible failures are fully isolated: `ModelRetry` → `RetryPromptPart`, `ToolFailed` → failed `ToolReturnPart`, sub-agent cancellation → failed return (`_tool_execution.py:697-702`, `41-62`); a function-tool retry additionally triggers the retry-wins invariant suppressing an otherwise-valid output (`_tool_execution.py:881-908`; tests `tests/test_agent.py:6730`, `6836`). (b) Unexpected exceptions abort the batch: sibling tasks are cancelled and drained (`_tool_execution.py:831-837`), yet completed returns survive as an `interrupted` request (`_agent_graph.py:2158-2172`; test `tests/test_agent.py:7141-7179`). (c) Deferred calls (`CallDeferred`/`ApprovalRequired`) raised anywhere in the batch are collected into a single end-of-step `DeferredToolRequests` result (`_tool_execution.py:759-764`, `862-877`, `964-1050`; test `tests/test_agent.py:7330-7344`).
5. **Is result order deterministic?** Split by channel. Message history (`output_parts`) is deterministic — always emission order, assembled from per-index maps (`_tool_execution.py:845-847`, `1233-1264`), including cross-call dedupe of tool reveals pruned "at assembly … in model call (index) order" specifically because parallel completion order would otherwise race (`_tool_execution.py:204-229`). Streamed events are completion-ordered by default; `'parallel_ordered_events'` restores emission order for consumers needing determinism (explicitly motivated by DBOS durable replay, `tests/test_agent.py:7387-7390`). The exhaustive winner is "first valid output by emission order", never first-to-finish (`_tool_execution.py:1216-1222`).

## Architectural Decisions

- **Parallel-by-default with opt-out serialization.** Concurrency is the default (`tool_manager.py:45`); both escape hatches (`sequential=True`, `'sequential'` mode) are additive flags rather than configuration of a sequential baseline. This mirrors provider behavior (models emit parallel calls) and is documented as such (`docs/tools-advanced.md:736-740`).
- **Barrier segmentation instead of global locking.** Rather than locks around resources, ordering constraints are expressed structurally: `_segment_by_barriers` partitions the emission sequence so each barrier is a single-call segment executed inline between parallel segments (`_tool_execution.py:232-251`, `800-805`). This gives happens-before edges without lock bookkeeping.
- **Separation of execution order from observation order.** Execution is concurrent; history assembly is index-ordered; event streaming is mode-dependent (`'parallel'` vs `'parallel_ordered_events'`). The split exists because different consumers need different guarantees — UI streaming tolerates completion order, durable executors (DBOS/Temporal) require replayable order (`_tool_execution.py:1153-1156`; `docs/changelog.md:166-172`).
- **Strategy pattern over output/function interleaving.** `end_strategy` selects among `_EarlyProcessor`/`_GracefulProcessor`/`_ExhaustiveProcessor` subclasses sharing validation, deferred resolution, and retry-wins machinery (`_tool_execution.py:300-335`). The default changed from `'early'` (skip side-effecting function tools once output succeeds) to `'graceful'` (let them run) precisely because tools have side effects that should happen (`docs/changelog.md:125`, `166-172`).
- **Deterministic assembly as a correctness invariant, not an afterthought.** Non-idempotent reveal-pruning runs exactly once per pass with guards against re-entry (`pruned` flag, `appended_*_index` sets, `_tool_execution.py:1175-1282`), showing the design treats scheduler-nondeterminism as a first-class hazard.
- **One policy core, two surfaces.** The graph path and the realtime session share `ToolManager` semantics; realtime reimplements only scheduling (background task per call with completion-event prerequisites implementing the same barrier math, `realtime/_session.py:2417-2451`).

## Notable Patterns

- **ContextVar-scoped execution policy**: `ParallelExecutionMode` rides a `ContextVar` so the mode follows async context rather than object state, letting a `with` block scope it to one run (`tool_manager.py:44-46`, `170-185`).
- **Indexed mailbox collection**: parallel tasks write results into dicts keyed by emission index (`tool_parts_by_index`, `user_parts_by_index`, `function_parts`), and a single consumer drains them in sorted order (`_tool_execution.py:745-748`, `838-852`) — a lightweight reorder buffer.
- **Completion-driven event pump**: `while pending: done, pending = await asyncio.wait(pending, FIRST_COMPLETED)` yields each result as it lands (`_tool_execution.py:819-827`, `1182-1208`).
- **Cancel-and-drain discipline**: on any exit path, siblings are cancelled with a message and awaited to completion so cancellations don't leak into unrelated scopes (`_utils.py:312-330`; invoked at `_tool_execution.py:828-837`, `1209-1214`); verified by `tests/test_agent.py:7346-7385`.
- **Budget projection before fan-out**: `check_before_tool_call` validates the *projected* post-batch usage up front so a batch that would exceed `tool_calls_limit` executes nothing (`_tool_execution.py:444-448`; `usage.py:553-559`; documented at `docs/agent.md:956`).
- **Per-tool timeout as failure conversion**: `FunctionToolset.call_tool` wraps execution in `anyio.fail_after` and converts `TimeoutError` to `ModelRetry` (`toolsets/function.py:684-691`), keeping slow siblings from stalling a parallel segment forever.

## Tradeoffs

- **Latency vs. corruption safety**: parallelism maximizes throughput, but conflict avoidance depends wholly on developers remembering `sequential=True`. The harness provides no static analysis or runtime detection of conflicting writes; the cost of forgetting is silent corruption the framework cannot warn about. Docs frame the flag as the remedy without enforcing it (`docs/tools-advanced.md:736-740`).
- **Unbounded fan-out vs. simplicity**: a model emitting N calls spawns N tasks immediately; there is no semaphore/queue for tool bodies within a step. Backpressure exists only at the run level (`max_concurrency`, `agent/__init__.py:694`) or via provider-side `parallel_tool_calls=False` (`settings.py:232`) — neither bounds intra-step concurrency.
- **Completion-ordered streaming vs. determinism**: default event order is cheapest and most responsive, but consumers needing stable order must buffer everything until segment completion (`parallel_ordered_events` trades latency for replayability, `_tool_execution.py:813-818`).
- **Isolation asymmetry**: retry-able failures keep siblings alive; unexpected exceptions tear down the batch. This is defensible (unknown state ⇒ stop), but means a single buggy tool converts a parallel step into an aborted step even when its siblings were independent and already succeeded (their results are preserved as interrupted history rather than fed to the model).
- **Async-first bias**: sync tools execute on threads (`_function_schema.py:87-89`), which keeps them parallel but introduces thread-accumulation concerns and ContextVar invisibility inside sync tools, both documented with a bounded-executor mitigation (`docs/tools-advanced.md:784-813`).

## Failure Modes / Edge Cases

- **Mid-batch exception**: handled by cancelling/draining siblings, then surfacing completed returns as a `state='interrupted'` request so message history stays valid for resume (`_agent_graph.py:2158-2172`; `tools-advanced.md:641` documents that cancelled tools' side effects may remain while results are discarded).
- **Outer cancellation during a batch**: pending tasks receive explicit cancel + drain; completed results persist; history repair handles tool calls that never produced returns (`tests/test_agent.py:7346-7385`; `tests/test_run_cancellation.py:362-406` builds a `_parallel_tools_agent` to prove resumability).
- **Duplicate `tool_call_id`s across a batch**: rejected outright on the resume path because supplied results could bind ambiguously (`_tool_execution.py:119-127`, `405-420`).
- **Racing tool reveals**: two parallel tools revealing the same tool name — resolved deterministically by pruning at assembly time, with the code comment explicitly citing the race (`_tool_execution.py:209-216`).
- **Output-tool max-retries in a parallel batch**: captured into `_OutputCallResult.raise_exc` instead of raised inline so the caller can absorb it if another output won (`_tool_execution.py:483-527`, `572-578`); retry counters accumulate in `output_retries_increment` to avoid interleaved parallel writes (`_tool_execution.py:360-367`).
- **Realtime-specific**: a provider-side `ToolCallCancelled` cancels the in-flight task and writes an `outcome='interrupted'` return so history never dangles (`realtime/_session.py:2470-2486`).

## Future Considerations

- An optional per-step concurrency cap (semaphore/queue depth) for tool bodies would complement the count-based `tool_calls_limit` and give operators I/O-boundary control analogous to the embeddings path, which does use `anyio.Semaphore(max_concurrency)` (`embeddings/bedrock.py:618-629`).
- Richer side-effect metadata (e.g., read/write resource hints on `ToolDefinition`) could let the scheduler derive barriers instead of relying solely on manual `sequential=True` declarations.
- Migrating task management onto anyio primitives would align tool execution with the anyio-based graph runner (`agent/__init__.py:1848` references "GraphRun → anyio TaskGroup") and future-proof trio support.

## Questions / Gaps

- No evidence found of resource-conflict detection beyond the declarative barrier flag: searches for locks/semaphores in the tool path (`Semaphore`, `Lock`, `CapacityLimiter` usage across `pydantic_ai_slim/pydantic_ai`) surfaced only the run-level limiter (`agent/__init__.py:694`), the embeddings client (`embeddings/bedrock.py:618`), and the model-request wrapper (`models/concurrency.py`). Nothing mediates concurrent access between tool bodies.
- Whether `'exhaustive'`'s full-parallel output execution has production-scale validation is not visible in-repo beyond unit/integration tests; no benchmark or load evidence exists in the source tree.
- The interaction of `sequential=True` with deferred (external/approval) tools appears untested in the corpus reviewed here: deferred calls bypass `_call_tools` execution and are batched separately (`_tool_execution.py:292-293`, `921-962`), so a barrier declared on a deferred tool has no execution-time meaning in the graph path. No test pins this; it may be intentionally moot but is undocumented.

---

Generated by `07.02-sequential-vs-parallel-tool-execution` against `pydantic-ai`.

# Source Analysis: crewai

## 07.02: Sequential vs Parallel Tool Execution

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (uv monorepo: `lib/crewai`, `lib/crewai-tools`; version 1.15.17 per `lib/crewai/src/crewai/__init__.py:51`) |
| Analyzed | 2026-08-26 |

## Summary

CrewAI is fundamentally a **one-tool-per-turn harness** whose newer native tool-calling paths add **opt-out batch parallelism** guarded by per-tool flags rather than side-effect metadata. There are at least four distinct tool-execution stacks, and their concurrency models disagree:

1. **ReAct/prompt-parsed path**: strictly sequential. Each LLM response yields one `AgentAction`, executed by `execute_tool_and_check_finality` (`lib/crewai/src/crewai/utilities/tool_utils.py:200-360`) which delegates to a single `ToolUsage.use` call (`lib/crewai/src/crewai/tools/tool_usage.py:148-185`).
2. **LLM-internal fallback loop**: when an LLM provider object handles tool calls itself, `LLM._handle_tool_call` executes **only the first** call of a returned batch (`tool_call = tool_calls[0]`, `lib/crewai/src/crewai/llm.py:1755`). The Anthropic provider iterates multi-tool batches sequentially in a plain `for` loop (`lib/crewai/src/crewai/llms/providers/anthropic/completion.py:1330-1353`).
3. **Native tool-calling executors** (`CrewAgentExecutor._handle_native_tool_calls`, `lib/crewai/src/crewai/agents/crew_agent_executor.py:667-807`; experimental executor `execute_native_tool`, `lib/crewai/src/crewai/experimental/agent_executor.py:1693-1878`): when the model emits several calls in one turn and **none** carries `result_as_answer` or `max_usage_count`, the whole batch runs concurrently in a `ThreadPoolExecutor(max_workers=min(8, n))`, with `contextvars.copy_context()` propagated into worker threads. Results are collected out-of-order (`as_completed`) but replayed into conversation history in original call order.
4. **Plan-and-execute StepExecutor**: sequential loop over a todo's tool calls (`lib/crewai/src/crewai/agents/step_executor.py:591-659`), while *independent todos themselves* fan out via `asyncio.gather` over thread-offloaded step executions (`lib/crewai/src/crewai/experimental/agent_executor.py:1220-1254`).

Safety rails are narrow but real: a thread-safe usage-limit claim (`BaseTool._claim_usage` under `_usage_lock`, `lib/crewai/src/crewai/tools/base_tool.py:302-324`), a read-write-locked result cache (`lib/crewai/src/crewai/agents/cache/cache_handler.py:20-44`), sibling cancellation on deliberate aborts (`experimental/agent_executor.py:1771-1779`), and a large regression-test surface spanning five LLM providers. What is missing is any notion of *which* tools are safe to run together: there is no read-only/write side-effect flag, no resource lock, and the worker cap of 8 is hardcoded. Task-level parallelism (`async_execution=True`) exists above the tool layer and produces deterministic output ordering by draining futures in declaration order (`lib/crewai/src/crewai/crew.py:2005-2018`).

## Rating

**6 / 10 — Present but inconsistent.**

Rationale against the rubric:

- **Why not 7-8**: The concurrency model differs by code path (first-call-only in `llm.py:1755`, sequential loop in `step_executor.py:608`, provider-sequential in `anthropic/completion.py:1332`, thread-pool parallel in two executors). The parallelism gate reuses flags designed for other purposes (`result_as_answer`, `max_usage_count`) instead of declaring side effects; the 8-worker cap is hardcoded with no configuration knob (`experimental/agent_executor.py:1754`, `agents/crew_agent_executor.py:742`); and the parallel batching behavior itself is not described in current user docs (only changelog fragments such as "Propagate contextvars context to parallel tool call threads", `docs/edge/en/changelog.mdx:2425`).
- **Why not lower**: Deterministic result ordering is enforced and tested (`tests/agents/test_agent_executor.py:1274-1277`), failure isolation converts individual tool errors into per-call tool messages (`experimental/agent_executor.py:1780-1791`), usage limits are race-safe under threads (`tests/tools/test_tool_failure.py:1350-1408`), and provider-integration tests exercise 3-way parallel calls across OpenAI/Anthropic/Gemini/Azure/Bedrock (`tests/agents/test_native_tool_calling.py:284-891`).

## Evidence Collected

Every entry cites workspace-relative paths from the source root `studies/agent-harness-study/sources/crewai`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| ReAct path: one tool per action | `execute_tool_and_check_finality` takes a single `AgentAction` and runs one `tool_usage.use(...)` | `lib/crewai/src/crewai/utilities/tool_utils.py:200-308` |
| Single-tool ReAct executor | `ToolUsage.use` / `ause` resolve and execute exactly one parsed calling | `lib/crewai/src/crewai/tools/tool_usage.py:148-185, 187-207` |
| First-call-only fallback in LLM layer | `tool_call = tool_calls[0]` inside `_handle_tool_call` | `lib/crewai/src/crewai/llm.py:1752-1755` |
| Provider-sequential multi-tool handling | Anthropic `_execute_tools_and_collect_results` loops tool_uses serially | `lib/crewai/src/crewai/llms/providers/anthropic/completion.py:1312-1353` |
| Parallel native batch executor (production) | `_handle_native_tool_calls`: >1 parsed call → `ThreadPoolExecutor(max_workers=min(8, len(execution_plan)))`, futures via `contextvars.copy_context().run` | `lib/crewai/src/crewai/agents/crew_agent_executor.py:726-767` |
| Parallelism gate (production executor) | Batch skipped to sequential if any tool has `result_as_answer` or `max_usage_count` | `lib/crewai/src/crewai/agents/crew_agent_executor.py:698-724` |
| Parallel native batch executor (experimental) | `execute_native_tool` + `_should_parallelize_native_tool_calls` gate; `max_workers = min(8, len(runnable_tool_calls))` | `lib/crewai/src/crewai/experimental/agent_executor.py:1748-1755, 1880-1911` |
| Deterministic result ordering under parallelism | Preallocated `ordered_results[idx]` filled from `as_completed`, then appended in order; `asyncio.gather preserves input order` comment | `lib/crewai/src/crewai/experimental/agent_executor.py:1764-1794`; `1251-1260` |
| Failure isolation in parallel batch | Generic exceptions become per-call error tool results; `ToolExecutionFailedError` triggers `pool.shutdown(wait=False, cancel_futures=True)` with explicit note that in-flight threads cannot be interrupted | `lib/crewai/src/crewai/experimental/agent_executor.py:1767-1791` |
| Sequential short-circuit for final-answer tools | Sequential branch returns `"tool_result_is_final"` without running remaining calls | `lib/crewai/src/crewai/experimental/agent_executor.py:1795-1837` |
| Side-effect-ish tool flags (the only gate inputs) | `result_as_answer` field; `max_usage_count` field; `cache_function` field | `lib/crewai/src/crewai/tools/base_tool.py:176-198` |
| Thread-safe usage limiting (resource guard) | `_usage_lock: threading.Lock` + atomic `_claim_usage` check-and-increment used by both `run` and `arun` | `lib/crewai/src/crewai/tools/base_tool.py:199, 302-324, 334-341, 362-366` |
| Thread-safe tool-result cache | `CacheHandler` uses `RWLock`: concurrent reads (`r_locked`), exclusive writes (`w_locked`); declared failures never cached | `lib/crewai/src/crewai/agents/cache/cache_handler.py:20-44, 46-60`; `lib/crewai/src/crewai/utilities/rw_lock.py:13` |
| Shared native-call lifecycle helper (used by StepExecutor) | `execute_single_native_tool_call`: cache read, hooks, events, usage-limit detection, unknown-tool `ToolFailure` | `lib/crewai/src/crewai/utilities/agent_utils.py:1602-1794` |
| Plan-todo parallelism primitive | `TodoList.can_parallelize` (>1 ready todo) and dependency-satisfied readiness incl. failed deps | `lib/crewai/src/crewai/utilities/planning_types.py:112-154` |
| Parallel todo execution | `execute_todos_parallel`: `asyncio.to_thread(step_executor.execute)` fanned out with `asyncio.gather(..., return_exceptions=True)`; observations afterwards run sequentially | `lib/crewai/src/crewai/experimental/agent_executor.py:1220-1254, 1305-1308` |
| Task-level parallelism (above tools) | `Task.execute_async` daemon-thread Future; crew starts async tasks and drains futures at sync-task boundary or end | `lib/crewai/src/crewai/task.py:609-623`; `lib/crewai/src/crewai/crew.py:1597-1625` |
| Deterministic task-output ordering | `_process_async_tasks` awaits `future.result()` in declaration order | `lib/crewai/src/crewai/crew.py:2005-2018` |
| Hardcoded limits elsewhere (contrast) | Event bus `ThreadPoolExecutor(max_workers=10)`; A2A card fetch `min(len(a2a_agents), 10)`; memory save pool `max_workers=1` | `lib/crewai/src/crewai/events/event_bus.py:180`; `lib/crewai/src/crewai/a2a/wrapper.py:279-280`; `lib/crewai/src/crewai/memory/unified_memory.py:165-167` |
| Parallel-execution unit test | `test_execute_native_tool_runs_parallel_for_multiple_calls`: two 0.2s tools finish in <0.5s, tool messages ordered `call_1`, `call_2` | `lib/crewai/tests/agents/test_agent_executor.py:1243-1277` |
| Fallback-to-sequential tests | `...falls_back_to_sequential_for_result_as_answer` (elapsed >= 0.2s) and `...short_circuits_remaining_calls` (second tool count == 0) | `lib/crewai/tests/agents/test_agent_executor.py:1279-1319, 1321-1369` |
| Cross-provider parallel integration tests | 3 parallel local search tools asserted across OpenAI/Anthropic/Gemini/Azure/Bedrock crews and agent kickoffs; hook parity asserts >=3 before-hooks | `lib/crewai/tests/agents/test_native_tool_calling.py:103-136, 284-330, 520-555, 631-666, 752-787, 855-891` |
| Concurrency-corruption regression tests | Threads do not cross-contaminate failure accumulation; barrier-synchronized shared-agent kickoffs each hold only their own record | `lib/crewai/tests/tools/test_tool_failure.py:1350-1369, 1371-1408` |
| Parallel MCP tool tests | Same tool and different tools called from 2 worker threads must not interfere | `lib/crewai/tests/mcp/test_mcp_config.py:218-261, 264-305` |
| Order-preserving async utility test | `test_parallel_results_preserve_order` for chunk summarization via `asyncio.gather` | `lib/crewai/tests/utilities/test_agent_utils.py:951` |
| Changelog evidence of parallel-path hardening | "Resolve issues with grouping parallel tool results…"; "Propagate contextvars context to parallel tool call threads" | `docs/edge/en/changelog.mdx:2421, 2425` |
| Documented task-level async | `async_execution` task attribute documented for users | `docs/edge/en/concepts/tasks.mdx:56` |
| Flow-level parallelism (adjacent layer) | Routers run sequentially; normal listeners run in parallel; `@start` methods "often in parallel" | `lib/crewai/src/crewai/flow/runtime/__init__.py:3048-3071`; `docs/edge/en/concepts/flows.mdx:108` |
| Naming trap (not framework parallelism) | `ParallelSearchTool` wraps the Parallel.ai Search HTTP API; it is a vendored web-search tool, not an intra-harness parallel executor | `lib/crewai-tools/src/crewai_tools/tools/parallel_tools/parallel_search_tool.py:47-66` |

## Answers to Dimension Questions

1. **Can multiple tools run in one turn?**
   Yes — but only on the native tool-calling executors. When the model returns a batch, `CrewAgentExecutor` runs them concurrently (`lib/crewai/src/crewai/agents/crew_agent_executor.py:742-767`) and the experimental executor does the same (`lib/crewai/src/crewai/experimental/agent_executor.py:1753-1794`). The ReAct path never can (single `AgentAction`, `lib/crewai/src/crewai/utilities/tool_utils.py:255-308`), the StepExecutor runs its batch sequentially (`lib/crewai/src/crewai/agents/step_executor.py:608`), and the LLM-layer fallback silently drops all but the first call (`lib/crewai/src/crewai/llm.py:1755`).

2. **Which tools are safe to parallelize?**
   Any tool *not* flagged `result_as_answer=True` and *not* given a `max_usage_count`. These two flags are checked per batch in `_should_parallelize_native_tool_calls` (`lib/crewai/src/crewai/experimental/agent_executor.py:1880-1911`) and duplicated inline in the production executor (`lib/crewai/src/crewai/agents/crew_agent_executor.py:698-724`). There is **no read-only/write side-effect declaration**, so a pair of file-writing tools would be parallelized unless they happen to carry those unrelated flags.

3. **Are write tools serialized?**
   No. Nothing distinguishes writes from reads. The only serialization mechanisms are incidental: `result_as_answer` forces the sequential branch (where remaining calls are skipped after the answer, `experimental/agent_executor.py:1822-1835`), and `max_usage_count` both disables batching and enforces a lock-guarded global cap (`lib/crewai/src/crewai/tools/base_tool.py:311-324`). Delegation tools and human-input flows are not special-cased for concurrency either.

4. **How are failures aggregated?**
   Per-call isolation: in the parallel branch each future's generic exception is converted into an `"Error executing tool: …"` result dict that becomes that call's tool message (`lib/crewai/src/crewai/experimental/agent_executor.py:1780-1791`); the shared helper likewise turns parse errors and unknown tools into structured `ToolFailure`s rather than raising past the batch (`lib/crewai/src/crewai/utilities/agent_utils.py:1659-1681, 1767-1794`). Only a deliberate `ToolExecutionFailedError` aborts the batch, cancelling not-yet-started siblings while acknowledging in-flight threads complete anyway (`experimental/agent_executor.py:1771-1779`). For plan todos, `asyncio.gather(return_exceptions=True)` keeps sibling steps alive and marks only the failed one (`experimental/agent_executor.py:1251-1268`); at task level, the first failing future's exception re-raises through `future.result()` (`lib/crewai/src/crewai/crew.py:2011-2012`).

5. **Is result order deterministic?**
   Yes, deliberately. Both thread-pool paths map futures to indices and fill a preallocated list, so tool messages enter history in the model's emission order regardless of completion order (`experimental/agent_executor.py:1764-1794`; `crew_agent_executor.py:743-769`); a unit test pins this ordering (`lib/crewai/tests/agents/test_agent_executor.py:1274-1277`). Async paths rely on documented `asyncio.gather` input-order preservation (`experimental/agent_executor.py:1256-1260`, tested at `tests/utilities/test_agent_utils.py:951`) and on draining task futures in declaration order (`crew.py:2011-2013`).

## Architectural Decisions

- **Thread pools, not asyncio, for parallel tool calls.** Native batches use `ThreadPoolExecutor` with `contextvars.copy_context().run` submissions (`crew_agent_executor.py:748-757`, `experimental/agent_executor.py:1756-1763`), keeping sync `_run` tools first-class; async support exists per-tool via optional `_arun` overrides (`base_tool.py:368-377`) but is not required for parallelism.
- **Flags-as-gates instead of a concurrency contract.** Rather than introducing `parallel_safe`/side-effect metadata, reuse of `result_as_answer` and `max_usage_count` as implicit "do not batch" markers couples control-flow policy to unrelated feature flags (`experimental/agent_executor.py:1903-1909`). This is conservative (both flags signal stateful or flow-controlling tools) but invisible to users reading either flag's description (`base_tool.py:180-187`).
- **Deterministic replay over completion-order streaming.** Results are buffered and appended in call order, trading a small latency win (could stream each result as finished) for stable conversation-history semantics and simpler prompt equivalence between sequential and parallel modes.
- **Layered fallback ladder.** Native tool calling degrades gracefully: unsupported-provider errors flip the executor back to ReAct mode mid-run (`is_native_tool_conversation_unsupported_error` handling at `crew_agent_executor.py:577`; `_use_native_tools = False` reset at `agents/step_executor.py:190-192`), which also silently changes the concurrency model from parallel-batch to strictly sequential.
- **Plan-level parallelism isolated from tool-level parallelism.** Independent plan todos fan out via `asyncio.to_thread` around the same `StepExecutor` used sequentially (`experimental/agent_executor.py:1238-1249`), but observations are deliberately serialized afterward to protect shared planning state (`1305-1308`) — a read-mostly/write-serial compromise applied ad hoc at the orchestration layer rather than via a general mechanism.

## Notable Patterns

- **Index-keyed result collection** (`ordered_results[idx] = future.result()` keyed off submission order) appears identically in both executors (`experimental/agent_executor.py:1756-1794`; `crew_agent_executor.py:742-769`) — duplicated logic rather than a shared abstraction.
- **RWLock-protected memoization cache**: identical `(tool, input)` hits skip execution entirely, including inside parallel batches (`agent_utils.py:1706-1712`), with failures excluded from caching so a transient error is never replayed permanently (`cache_handler.py:38-41`).
- **Barrier-based race reproduction tests**: concurrency bugs are pinned with deterministic `threading.Barrier` setups (`tests/tools/test_tool_failure.py:1384-1391`) and timing assertions (`elapsed < 0.5` for parallel, `>= 0.2` for sequential, `tests/agents/test_agent_executor.py:1273, 1316`).
- **Single-flight style guards elsewhere in the repo** show the team knows the pattern: memory saves serialize through a dedicated `ThreadPoolExecutor(max_workers=1)` pool (`unified_memory.py:165-167, 297-312`) even though searches fan out 4-wide (`recall_flow.py:147`).

## Tradeoffs

- **Latency vs corruption safety**: batching up to 8 tools cuts wall-clock time (validated by the <0.5s test), but because there is no conflict detection, two mutating calls in one batch race freely; safety is delegated to whoever remembers to set `max_usage_count`.
- **Simplicity vs expressiveness**: the two-flag gate avoids schema/API surface growth, yet it conflates "this tool ends the turn" and "this tool is scarce" with "this tool must not run concurrently" — three distinct concerns sharing two booleans.
- **Graceful degradation vs behavioral consistency**: falling back from native parallel batches to first-call-only or ReAct sequential paths keeps agents working everywhere, but the same prompt can execute with different concurrency (and different reflection granularity — see the docstring admitting first-call-only enables "reflection after each tool", `crew_agent_executor.py:672-684`) depending on provider capability detection.
- **Hardcoded cap vs tunability**: `min(8, n)` bounds thread explosion with zero configuration burden, but heavy-I/O fleets (e.g., scraping) cannot raise it and CPU-bound tools cannot document why they want 1.

## Failure Modes / Edge Cases

- **Uninterruptible stragglers**: on abort, already-running threads complete and may still emit events/mutate caches after the batch was cancelled; the code comments acknowledge this window (`experimental/agent_executor.py:1775-1777`).
- **Silent truncation in the fallback path**: `LLM._handle_tool_call` drops every call after the first (`llm.py:1755`) without emitting any event for the dropped calls — the model's remaining intents vanish from history on that path.
- **Cache-hit asymmetry under parallelism**: two identical calls in one batch both miss the cache (submitted simultaneously) and both execute; the second write wins in the cache. Benign for pure functions, surprising for counted or metered APIs.
- **Shared-agent concurrent kickoffs**: `agent.kickoff()` has no instance-reuse guard, historically causing cross-contaminated failure records; fixed and regression-tested, but the guard asymmetry versus `Crew` remains documented only in a test docstring (`tests/tools/test_tool_failure.py:1371-1380`).
- **Observation serialization bottleneck**: parallel todo execution still observes results one-by-one with LLM calls (`experimental/agent_executor.py:1308-1334`), so wall-clock gains shrink as observation cost grows relative to execution.

## Future Considerations

- Introduce an explicit per-tool execution-safety declaration (e.g., `parallel_safe: bool` or a `SideEffects` enum) consumed by a single shared `_should_parallelize` implementation, replacing the duplicated flag-sniffing in both executors.
- Promote the worker cap to configuration (agent- or crew-level), mirroring how `concurrency_limit` is already exposed to users in adjacent tooling docs (`docs/edge/en/tools/ai-ml/ragtool.mdx:607`).
- Unify the four execution stacks' semantics (or at least emit an event when the effective concurrency model changes due to native-tool-support fallback) so downstream telemetry can distinguish batch-parallel from sequential turns.
- Extend per-resource locking (the `RWLock` pattern already proven in `CacheHandler`) to user-declared resource keys, enabling safe batching of mixed read/write tools.

## Questions / Gaps

- No evidence found of resource-conflict detection: searched the source tree for `Semaphore`, resource locks keyed by file/path/URL, and any `parallel_safe`-style metadata; only the usage-count `threading.Lock` (`base_tool.py:199`) and the cache `RWLock` exist.
- No evidence found of user-facing documentation describing native multi-tool-call batching or its gating rules; current docs discuss only task-level `async_execution` (`docs/edge/en/concepts/tasks.mdx:56`) and flow-level listener parallelism (`docs/edge/en/concepts/flows.mdx:108`). The strongest doc traces are changelog fix entries (`docs/edge/en/changelog.mdx:2421-2425`).
- No evidence found that the Gemini/OpenAI Responses-style providers aggregate parallel calls differently from the Anthropic provider at the `BaseLLM` layer beyond what is cited here; the OpenAI Responses loop test (`tests/llms/openai/test_responses_tool_loop.py:139`) shows parallel calls becoming separate items, but the corresponding provider-side executor code was not individually audited within this dimension's scope.
- Whether `LiteAgent` (imported for typing in `utilities/tool_utils.py:29`) has its own tool loop was not traced; the study focused on the four executors listed in the Summary.

---

Generated by `dimensions/07.02-sequential-vs-parallel-tool-execution` against `crewai`.

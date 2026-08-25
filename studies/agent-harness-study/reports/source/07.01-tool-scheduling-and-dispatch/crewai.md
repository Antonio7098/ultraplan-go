# Source Analysis: crewai

## 07.01 Tool Scheduling and Dispatch

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (pydantic, asyncio, concurrent.futures; monorepo with `lib/crewai` as the main package) |
| Analyzed | 2026-08-24 |

## Summary

CrewAI dispatches validated tool calls **inline, inside the agent loop**, with no durable queue, broker, or remote execution tier. A tool call starts as either (a) a native function-calling list returned by the LLM or (b) a text-parsed ReAct `AgentAction`; both are parsed/validated, then executed synchronously by the calling thread. There is one deliberate exception: when a single LLM response carries multiple native tool calls, `CrewAgentExecutor` fans the batch out to an ephemeral `ThreadPoolExecutor` capped at 8 workers (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/agents/crew_agent_executor.py:742-767`) and then reassembles results in submission order so message history stays deterministic. Two other executors — the Plan-and-Act `StepExecutor` and the text-parsing loops in `LiteAgent`/`CrewAgentExecutor._invoke_loop_react` — execute strictly one call at a time.

Dispatch is heavily instrumented: every path emits `ToolUsageStartedEvent` / `ToolUsageFinishedEvent` / `ToolUsageErrorEvent` on a central event bus (which itself runs handlers on a 10-thread pool plus a dedicated event-loop thread), attaches timing and cache-provenance to finished events, records structured failures under an `ignore/warn/raise` policy, and reports usage counts to PostHog telemetry. Operational guardrails around scheduling include RPM limiting before each LLM turn, atomic per-tool `max_usage_count` claims under a lock, a thread-safe result cache consulted before any execution, and pre/post tool-call hooks that can block or rewrite calls. The main weakness is that the dispatch logic exists in three parallel implementations (ReAct `ToolUsage`, native executor path, StepExecutor path) with duplicated lifecycle code and inconsistent batching semantics between them.

## Rating

**7 / 10** — Clear model with explicit interfaces, real concurrency tests, and operational safeguards (rate limits, usage limits, failure policy, hooks, timeout). Not higher because: dispatch logic is duplicated across three code paths with subtly different batch semantics (parallel batch vs sequential batch vs first-call-only); the async native loop still schedules through OS threads instead of true asyncio scheduling; and the repeated-usage guard only exists on the ReAct path, so identical consecutive calls behave differently depending on which executor dispatched them.

## Evidence Collected

Every entry cites paths relative to the workspace root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Dispatcher (ReAct/text path) | `execute_tool_and_check_finality` builds a `ToolUsage`, parses the calling, runs hooks, executes, applies failure policy | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/tool_utils.py:200-360` |
| Dispatcher (async variant) | `aexecute_tool_and_check_finality` mirrors sync flow with `await tool_usage.ause(...)` | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/tool_utils.py:35-197` |
| ToolUsage orchestrator | `use()` selects tool then delegates to `_use()`; async twins `ause()`/`_ause()` | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/tools/tool_usage.py:148-185`, `187-236`, `238-501`, `503-758` |
| Executor (native batch, parallel) | `_handle_native_tool_calls`: ThreadPoolExecutor `max_workers=min(8, len(plan))`, futures keyed by index into `ordered_results` | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/agents/crew_agent_executor.py:726-767` |
| Executor (native single) | When batch is degraded, only `parsed_calls[0]` executes, then a `post_tool_reasoning` user message is appended for reflection | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/agents/crew_agent_executor.py:786-807` |
| Executor (Plan-and-Act step) | `StepExecutor._execute_native_tool_calls` iterates the LLM's tool-call list sequentially, short-circuits on `result_as_answer` | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/agents/step_executor.py:591-659` |
| Executor (LiteAgent loop) | Text-parsed actions dispatched inline via `execute_tool_and_check_finality` inside the while-loop | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/lite_agent.py:906-1024` |
| Batch degradation rule | Parallelism skipped when any call targets a `result_as_answer` or `max_usage_count` tool (debug log names the reason) | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/agents/crew_agent_executor.py:698-724` |
| Final tool invocation | `CrewStructuredTool.invoke` validates args against schema, enforces max-usage, runs coroutine funcs via `asyncio.run`; `ainvoke` offloads sync funcs via default executor | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/tools/structured_tool.py:380-448` |
| Atomic usage claim | `BaseTool._claim_usage` checks/increments `current_usage_count` under `_usage_lock`, returning a `ToolFailure(USAGE_LIMIT)` instead of raising | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/tools/base_tool.py:302-343` |
| Cache-before-execute | Cache read keyed by sanitized name + JSON args before invoking; write-back gated by optional `cache_function` | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/agent_utils.py:1700-1763`; `tools/tool_usage.py:557-569, 633-648` |
| Cache handler | In-memory dict guarded by RWLock; declared `ToolFailure` results are never cached | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/agents/cache/cache_handler.py:14-60` |
| Scheduling trace events | `ToolUsageStartedEvent`/`FinishedEvent`(with `started_at`,`finished_at`,`from_cache`,`failure`)/`ErrorEvent`/`ToolFailureDetectedEvent` types | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/events/types/tool_usage_events.py:57-113` |
| Trace emission points | Started emitted before execute; Finished in `finally` unless an error event already fired | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/agent_utils.py:1714-1726, 1810-1845`; `tools/tool_usage.py:527-549, 744-752` |
| Event bus scheduling | Sync handlers run on a lazy `ThreadPoolExecutor(max_workers=10)`; async handlers on a dedicated daemon event loop; stream-chunk events forced synchronous to preserve ordering | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/events/event_bus.py:166-191, 572-647` |
| Rate limiting (pre-dispatch gate) | `enforce_rpm_limit` before every LLM turn; `RPMController.check_or_wait` blocks until next minute under a lock/timer | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/agents/crew_agent_executor.py:515`; `lite_agent.py:931`; `utilities/rpm_controller.py:38-55` |
| Whole-task timeout | `max_execution_time` runs the entire agent invocation on a copied context in a throwaway thread pool, cancels on timeout | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/agent/core.py:888-926` |
| Failure policy | `ToolFailurePolicy.IGNORE/WARN/RAISE` resolved most-specific-wins (tool → task → agent → crew); RAISE raises `ToolExecutionFailedError` to abort the loop | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/tools/tool_failure.py:57-68, 177-208, 324-382` |
| Hooks as dispatch gates | `run_before_tool_call_hooks` can block execution ("Tool execution blocked by hook"); after-hooks may rewrite results | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/hooks/tool_hooks.py:173-206`; `utilities/agent_utils.py:1730-1746` |
| Retry budget | `_max_parsing_attempts` (3, or 2 for big OpenAI models); recursive retry re-entered only after the finished event is emitted | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/tools/tool_usage.py:109-135, 451-479, 497-499, 708-736, 754-756` |
| Telemetry | PostHog `tool_usage`, `tool_repeated_usage`, `tool_usage_error` captures with attempt counts | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/telemetry/telemetry.py:611-666` |
| Concurrency tests | Tests assert overlapping execution windows (`max concurrency >= 2`) computed from `ToolUsageFinishedEvent` timestamps | `studies/agent-harness-study/sources/crewai/lib/crewai/tests/agents/test_native_tool_calling.py:103-141, 209-218, 282-320` |
| Async native loop | `_ainvoke_loop_native_tools` awaits the LLM but still calls the threaded/synchronous `_handle_native_tool_calls` for batches | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/agents/crew_agent_executor.py:1285, 1335-1339` |

## Answers to Dimension Questions

1. **How does a tool call start?** From model output, never from a timer or external producer. Two ingress formats: (a) native function-calling — the executor detects a list of tool-call objects (`crew_agent_executor.py:531-541`, detection at `634-665`) and maps names to callables registered up-front via `convert_tools_to_openai_schema` (`crew_agent_executor.py:497-499`); (b) text-parsed ReAct — `process_llm_response` yields an `AgentAction`, and `execute_tool_and_check_finality` parses it into a `ToolCalling` object, falling back to a dedicated function-calling LLM when direct parsing fails (`tool_utils.py:255`, `tools/tool_usage.py:889-937`). Fuzzy tool selection tolerates near-miss names with a SequenceMatcher ratio > 0.85 (`tool_usage.py:810-825`).

2. **Is tool execution inline or queued?** Inline. Every dispatcher executes the call on the current call stack within the agent's iteration loop (`lite_agent.py:958-969`, `step_executor.py:608-623`). The only off-thread execution is the native multi-call batch fanning out to an ephemeral `ThreadPoolExecutor(max_workers=min(8, n))` whose lifetime is one batch (`crew_agent_executor.py:742-746`), plus per-tool `ainvoke` offloading sync functions to the default executor (`structured_tool.py:408-412`). There are no persistent worker queues, job stores, or remote tool servers anywhere in the package; the only other pools serve observability (event bus, `event_bus.py:179-190`).

3. **Are tool calls ordered?** Message-history ordering is deterministic even when execution is not: futures submit into `ordered_results[idx]` keyed by the plan index, and results append to the conversation in that order regardless of completion order (`crew_agent_executor.py:743-776`). Sequential paths preserve the LLM's emission order trivially (`step_executor.py:607-657`). One caveat: if a batch contains a `result_as_answer` or `max_usage_count` tool, CrewAI degrades to executing *only the first* call and discards the rest of the batch for that turn (`crew_agent_executor.py:698-724, 786`), trading throughput for predictable side-effect control.

4. **Can tools be batched?** Yes, for native function calling: all tool calls in one assistant response form an execution plan appended as a single assistant message, dispatched together, then followed by a shared `post_tool_reasoning` prompt (`crew_agent_executor.py:726-784`). Batching is concurrent in `CrewAgentExecutor` (verified by overlap-asserting tests, `tests/agents/test_native_tool_calling.py:131-140, 282-304`) but strictly sequential in `StepExecutor` (`step_executor.py:591-659`) — an inconsistency, not a config switch. The ReAct/text path never batches: exactly one action per iteration.

5. **Is dispatch observable?** Yes, on three layers. (a) Lifecycle events: started/finished/error/failure-detected events carry timestamps, `from_cache`, args, agent/task correlation ids, plan-step numbers (`tool_usage_events.py:57-113`; emission at `agent_utils.py:1715-1726, 1810-1832`; `step_executor.py:387-452`); the finished event is emitted in a `finally` block so it fires even on failure paths (`tool_usage.py:744-752`). (b) Structured failure records land on `TaskOutput.tool_failures` and a ContextVar-scoped collector safe under concurrent tasks (`tool_failure.py:111-151, 260-301`). (c) Product telemetry (`telemetry.py:611-666`) and verbose printer logs including cache-hit annotations (`agent_utils.py:1854-1859`). The project's own concurrency tests consume these events to measure overlap, demonstrating the trace is sufficient to answer "why did this run now".

## Architectural Decisions

- **Inline dispatch over a scheduler service.** Tools run in-process on the agent loop (or a short-lived thread pool), keeping latency low and the mental model simple; no persistence, acknowledgement, or replay semantics exist for tool jobs.
- **Deterministic history over maximal concurrency.** The index-keyed future map (`crew_agent_executor.py:743-767`) decouples completion order from conversation order — the LLM always sees tool messages in its own emission order.
- **Safety-valve degradation instead of partial parallelism.** Rather than running a mixed batch partially parallel, presence of `result_as_answer`/`max_usage_count` tools forces first-call-only sequential mode (`crew_agent_executor.py:698-724`), prioritizing side-effect predictability.
- **Cache-first execution.** A lookup precedes any invocation on all three paths (`agent_utils.py:1706-1712`; `tool_usage.py:558-569`), with per-tool `cache_function` opt-outs and a rule that declared failures are never cached (`cache_handler.py:30-36`) so transient errors cannot be pinned permanently.
- **Declarative failure signalling.** Tools return `ToolFailure` objects rather than magic error strings (`tool_failure.py:71-108`), and a resolution chain (tool → task → agent → crew → WARN default) decides record/emit/abort behavior (`tool_failure.py:177-208`).
- **Events as the tracing contract.** Dispatch tracing is built on a typed pub/sub bus with dependency-ordered handlers rather than log lines, enabling programmatic consumers (including internal tests) to reconstruct timelines.

## Notable Patterns

- **Twin sync/async APIs everywhere**: `use`/`ause`, `invoke`/`ainvoke`, `run`/`arun`, `execute_tool_and_check_finality`/`aexecute_tool_and_check_finality` — though "async" often means thread-offloading, e.g. `ainvoke` wraps sync functions with `run_in_executor(None, ...)` (`structured_tool.py:406-412`) and `kickoff_async` wraps the whole sync loop in `asyncio.to_thread` (`lite_agent.py:796-816`).
- **Lifecycle event bracketing**: `started_at` captured before dispatch, `ToolUsageStartedEvent` before execution, `ToolUsageFinishedEvent` in `finally` with elapsed timestamps, deliberately suppressed when an error event already fired to avoid double-reporting (`tool_usage.py:270-298, 487-495`).
- **ContextVar-scoped collectors for concurrency safety**: tool-failure accumulation uses a `ContextVar` so a shared agent serving concurrent tasks doesn't cross-contaminate failure lists (`tool_failure.py:265-285`); the parallel pool propagates context via `contextvars.copy_context().run` (`crew_agent_executor.py:748-749`).
- **Atomic claim pattern**: usage-limit enforcement is check-and-increment under a private lock returning a typed failure (`base_tool.py:302-324`), avoiding TOCTOU races when batches run concurrently.
- **Retry-after-event discipline**: parse/exec retries recurse only after the finished event has been emitted, so subscribers see complete lifecycles for failed attempts too (`tool_usage.py:497-499, 754-756`).

## Tradeoffs

- **Simplicity of inline dispatch** means a long-running tool blocks its whole agent loop (mitigated only by the coarse whole-task `max_execution_time` thread wrapper, `agent/core.py:888-926`); there is no per-tool cancellation or timeout knob.
- **Parallel batching improves latency** but widens race surface: `ToolsHandler.last_used_tool` and the repeated-usage check (`tool_usage.py:779-789`, `tools_handler.py:39`) exist only on the ReAct path, so duplicate consecutive calls are caught there but not in native batches; cache writes from concurrent same-key calls last-writer-wins (guarded only by RWLock).
- **Three dispatch implementations** (`ToolUsage._use`, `agent_utils.execute_single_native_tool_call`, executor-local `CrewAgentExecutor._execute_single_native_tool_call` at `crew_agent_executor.py:868+`) duplicate cache/hook/event logic with small behavioral drift — e.g., different unknown-tool reporting (`tool_usage.py:835-853` raises vs `agent_utils.py:1787-1794` returns a structured `UNKNOWN_TOOL` failure).
- **Ephemeral thread pools per batch** (`crew_agent_executor.py:742-746`, also `agent/core.py:904`) cost thread creation per turn/batch and cap parallelism at 8 without configurability.

## Failure Modes / Edge Cases

- **Unknown tool in a native batch** becomes a structured `UNKNOWN_TOOL` failure feeding the model "Tool not found" text (`agent_utils.py:1702, 1787-1794`), while the ReAct path raises and counts a tool error (`tool_usage.py:826-853`).
- **Unparseable JSON args** return a ready-made error tool-message with `INVALID_INPUT` instead of silently running the tool with empty args (`agent_utils.py:1659-1682`, comment documents the old bug).
- **Spent usage limit** short-circuits before invocation with a `USAGE_LIMIT` failure surfaced on the finished event (`tool_usage.py:581-595`; `structured_tool.py:398-401, 433-436`).
- **Hook veto** replaces the result with "Tool execution blocked by hook" and clears any cached-failure attribution, while still firing post-hooks for monitoring symmetry (`agent_utils.py:1741-1746`; `tool_utils.py:123-143`).
- **Native-function-calling unsupported** triggers mid-run fallback to text-parsed tooling, keeping accumulated conversation state so completed calls aren't re-executed (`step_executor.py:189-226`; `crew_agent_executor.py:576-579`).
- **Policy RAISE** converts a declared failure into `ToolExecutionFailedError` raised *after* the finished event so traces stay complete, and is re-raised past generic handlers in both step and lite loops (`agent_utils.py:1834-1845`; `step_executor.py:184-187`; `lite_agent.py:980-982`).
- **Timeout** cancels the future but cannot interrupt a stuck tool thread — the abandoned worker keeps running detached (`agent/core.py:912-918`).
- **Repeated identical calls** are rejected once on the ReAct path via last-tool comparison (`tool_usage.py:509-525`), but nothing equivalent guards native or StepExecutor batches.

## Future Considerations

- Consolidate the three dispatch implementations behind one lifecycle component (cache → limits → hooks → invoke → events → policy) to remove drift; the newest, most complete implementation is `agent_utils.execute_single_native_tool_call`.
- Make batching semantics configurable (concurrency limit, sequential fallback) instead of hardcoding `min(8, n)` and implicit per-executor differences; expose it next to existing knobs like `max_usage_count` (`base_tool.py:184`).
- Add per-tool timeouts/cancellation for the inline path; the current whole-task thread timeout leaks workers on expiry.
- Extend the repeated-usage guard to native and StepExecutor paths, ideally keyed on (name, args) hashes stored on `ToolsHandler`.
- Offer true asyncio-native dispatch for `kickoff_async` so async users don't pay thread-pool overhead (`crew_agent_executor.py:1335-1339`).

## Questions / Gaps

- No evidence found of priority schemes, fairness policies, or backpressure between tool dispatch and the LLM polling loop beyond the RPM limiter; searches across `lib/crewai/src/crewai/{tools,agents,utilities}` for queue/worker/priority constructs returned only the event-bus and batch pools cited above.
- Whether `Flow` runtime-level parallelism (e.g., `asyncio.gather` over methods in `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/flow/runtime/__init__.py:2354, 2395`) composes safely with parallel tool batches was not tested directly in the source tree; the ContextVar failure collector suggests it is considered, but no test demonstrates nested concurrency of flows × tool batches.
- No documentation file states the intended batching contract; the docstring "Executes only the FIRST tool call" (`crew_agent_executor.py:672-679`) contradicts the general expectation set by the parallel branch, and no design note explains when each mode should be preferred.

---

Generated by `07.01-tool-scheduling-and-dispatch` against `crewai`.

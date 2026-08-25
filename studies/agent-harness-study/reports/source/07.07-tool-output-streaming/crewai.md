# Source Analysis: crewai

## 07.07 Tool Output Streaming

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (pydantic-based monorepo: `lib/crewai`, `lib/crewai-tools`, `lib/crewai-core`) |
| Analyzed | 2026-08-25 |

## Summary

CrewAI has **no framework-level mechanism for a tool to stream progress or partial output**. The tool contract is strictly batch: `BaseTool._run`/`_arun` return one final value (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/tools/base_tool.py:387-403`), and `CrewStructuredTool.invoke`/`ainvoke` block until the wrapped function completes (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/tools/structured_tool.py:380-414,424-448`). There is no generator/iterator protocol for tools, no progress-callback parameter (searches for `progress_callback|on_progress|report_progress` across `lib/` return nothing), and no cancellation token passed to tool bodies.

What CrewAI does have is a mature **event-streaming infrastructure around tool activity**: the internal event bus emits `ToolUsageStartedEvent` before execution and `ToolUsageFinishedEvent` after it (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/tools/tool_usage.py:291,488-495`; event types at `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/events/types/tool_usage_events.py:57-75`). A newer public "frame streaming" contract re-broadcasts these lifecycle events on a dedicated `"tools"` channel of an ordered `StreamFrame` timeline (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/streaming.py:143-154,174-190`), with channel projections, cancellation, ordering guarantees, and tests. But the frames carry start/finish/error *facts*, never incremental tool output.

The model itself only ever sees the final, fully-formatted tool result appended as a `role:"tool"` message (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/agents/crew_agent_executor.py:1059-1065`). One bundled tool (`TavilyResearchTool`) can return a raw SSE byte generator, demonstrating that per-tool streaming is possible but entirely unintegrated — the framework would stringify the generator object rather than consume it.

Net assessment: excellent streaming plumbing for LLM tokens and runtime events; absent (and in one case ad-hoc/unsafe) support for tool-emitted progress and partial results.

## Rating

**3 / 10 — Absent as a first-class capability, with adjacent infrastructure that does not cover the dimension's core question.**

Rationale against the rubric:

- Tool output streaming per se is **absent**: no streaming API on `BaseTool` (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/tools/base_tool.py:103-403`), no progress event type among the seven tool-usage events (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/events/types/tool_usage_events.py:11-132`), no partial-result buffer or persistence.
- The one streaming-capable tool is **ad-hoc and effectively unsafe** end-to-end: `TavilyResearchTool._run` returns `Generator[bytes]` when `stream=True` (`studies/agent-harness-study/sources/crewai/lib/crewai-tools/src/crewai_tools/tools/tavily_research_tool/tavily_research_tool.py:145-169`), which `_format_tool_output_for_agent` reduces to `str(raw_result)` — i.e., the model would receive the Python repr of a generator object (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/tools/structured_tool.py:59-93`).
- It does not score lower because the surrounding observability layer (started/finished/error events → console panels, trace spans, public `"tools"` frame channel) is real, tested, and documented, so users get coarse liveness signal for long tools — just not progress or partial output.

## Evidence Collected

Every entry includes a workspace-relative file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Batch-only tool contract | `_run` abstract method documented to return "The result"; single-value return type | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/tools/base_tool.py:387-403` |
| Async tool contract | `_arun` raises `NotImplementedError` by default; returns one awaited value | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/tools/base_tool.py:368-377` |
| Blocking invocation | `invoke()` runs func to completion; even awaits coroutines inline via `asyncio.run` | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/tools/structured_tool.py:424-448` |
| No progress callback | Grep across `lib/` for `progress_callback|on_progress|report_progress` returns no hits; `ToolCallHookContext` exposes only pre/post hooks | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/hooks/tool_hooks.py:31-52` |
| Lifecycle-only events | Event set: Started / Finished / Error / FailureDetected / ValidateInputError / SelectionError / ExecutionError — no Progress/Partial type | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/events/types/tool_usage_events.py:57-132` |
| Start/finish emission points | `ToolUsageStartedEvent` emitted before invoke; `on_tool_use_finished` in `finally` after completion | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/tools/tool_usage.py:291,487-495` |
| Public stream frame mapping | `_stream_channel` maps `ToolUsageEvent \| ToolExecutionErrorEvent` to the `"tools"` channel | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/streaming.py:143-154` |
| Frame conversion | `stream_frame_from_event` wraps any bus event (incl. tool events) into ordered `StreamFrame(id, seq, channel, data)` | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/streaming.py:174-190` |
| Legacy chunk stream is LLM-only | Stream handler drops everything except `LLMStreamChunkEvent` | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/streaming.py:354-356` |
| Chunk types lack tool-output kind | `StreamChunkType` = TEXT \| TOOL_CALL only; TOOL_CALL carries streamed *arguments* from the LLM | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/types/streaming.py:277-281,284-297` |
| Model sees final result only | Native path: blocking call then formatted result appended as `role:"tool"` message | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/agents/crew_agent_executor.py:975-994,1059-1065` |
| Final-result caching only | `ToolsHandler.on_tool_use` docstring: "Run when tool ends running"; caches complete outputs keyed by tool+input | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/agents/tools_handler.py:26-52` |
| Pre-execution interrupt (not mid-run) | `pre_tool_call` hooks can block a call before it starts; blocked message returned instead | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/hooks/tool_hooks.py:142-190`; `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/tool_utils.py:123-143` |
| Human gate during hook phase | `request_human_input` pauses console live-updates and blocks on stdin inside a hook | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/hooks/tool_hooks.py:86-128` |
| Hard timeout for MCP tools only | `MCP_TOOL_EXECUTION_TIMEOUT = 60`; `asyncio.wait_for` cancels and returns error string | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/tools/mcp_tool_wrapper.py:10-12,156-159` |
| Session-level cancellation (sync) | `StreamSession.close()` marks cancelled, closes iterator, runs cleanup | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/types/streaming.py:182-192` |
| Session-level cancellation (async) | `aclose()` closes async iterator; async frame generator cancels its background task in `finally` | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/types/streaming.py:264-274`; `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/streaming.py:320-331` |
| Sync close still joins worker | `create_frame_generator`'s `finally: thread.join()` runs when iterator closed — close detaches output but waits for run completion | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/streaming.py:276-287` |
| UI emitters | ConsoleFormatter renders "Tool Execution Started (#n)" and completion panels; wired via `@crewai_event_bus.on(ToolUsageStartedEvent)` | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/events/utils/console_formatter.py:404-467`; `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/events/event_listener.py:531-556` |
| Trace integration | Trace listener records `tool_usage_started/finished/error/failure_detected` action spans | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/events/listeners/tracing/trace_listener.py:394-424` |
| Ad-hoc streaming tool | `TavilyResearchTool` supports `stream=True` returning SSE `Generator[bytes]` / `AsyncGenerator[bytes]` | `studies/agent-harness-study/sources/crewai/lib/crewai-tools/src/crewai_tools/tools/tavily_research_tool/tavily_research_tool.py:40-43,138-169,199-200` |
| Generator not consumed by harness | Output formatting falls through to `str(raw_result)` for schema-less tools | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/tools/structured_tool.py:59-93` |
| LLM-side partial handling (contrast) | On mid-stream error, accumulated partial text is returned with warning — exists for LLM text, no analogue for tools | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/llm.py:1098-1114` |
| Tests: session semantics & cancellation | `test_aclose_cancels_async_streaming`, `test_close_cancels_sync_streaming`, concurrency isolation tests | `studies/agent-harness-study/sources/crewai/lib/crewai/tests/test_streaming.py:760-930` |
| Tests: frame ordering/cancellation cleanup | Channel projection order-preservation; `test_astream_cancellation_cleans_up_task` | `studies/agent-harness-study/sources/crewai/lib/crewai/tests/test_stream_frames.py:98-300` |
| Tests: streamed tool-call arguments | Provider-level accumulation of partial tool-call JSON deltas | `studies/agent-harness-study/sources/crewai/lib/crewai/tests/llms/test_tool_call_streaming.py:79-181` |
| Documented design intent | Docs define `"tools"` channel as "Tool usage start, finish, and error events" | `studies/agent-harness-study/sources/crewai/docs/edge/en/concepts/streaming.mdx:53-62`; `studies/agent-harness-study/sources/crewai/docs/edge/en/learn/streaming-runtime-contract.mdx:36-45` |

## Answers to Dimension Questions

**1. Can tools stream progress?**
No. The tool interface accepts no sink/callback and returns a single value (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/tools/base_tool.py:326-343,387-403`; `structured_tool.py:424-448`). The event vocabulary contains no progress event (`tool_usage_events.py:11-132`). A tool author's only workaround is emitting custom events onto `crewai_event_bus` themselves, which would surface on the generic `"custom"` frame channel (`utilities/streaming.py:152-154`) — supported by the bus but undefined as a tool contract.

**2. Are partial outputs durable?**
No. Nothing is written until a tool finishes; `ToolsHandler.on_tool_use` caches the complete output keyed by sanitized tool name + serialized input (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/agents/tools_handler.py:26-52`). There are no partial-output buffers anywhere in the tool path (the only buffered partials in the codebase are LLM token accumulators, e.g. `llm.py:1098-1114`).

**3. Does the model act on partial output?**
No. In both the native function-calling path and the ReAct `ToolUsage` path, execution is awaited to completion and only the final formatted string enters the conversation (`crew_agent_executor.py:1050-1065`; `tool_usage.py:185,234-236,400-402`). What *is* streamed toward the model side is the reverse direction: the LLM's own tool-call arguments arrive as partial JSON deltas and are accumulated before execution (`events/types/llm_events.py:136-144`; provider accumulation at `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/llms/providers/anthropic/completion.py:1207-1211`).

**4. Can users interrupt?**
Only at boundaries, not mid-stream:
- Before launch: `pre_tool_call` hooks can veto a call (`hooks/tool_hooks.py:173-190`), and hook code can solicit human approval interactively via `request_human_input` (`hooks/tool_hooks.py:86-128`).
- Of the observation stream: `StreamSession.close()` / `AsyncStreamSession.aclose()` cancel consumption; the async variant also cancels the background task (`types/streaming.py:182-192,264-274`; `utilities/streaming.py:320-331`).
- A long-running non-MCP tool body cannot be interrupted once started: there is no timeout or cancellation propagation in `CrewStructuredTool.invoke`/`BaseTool.run`. Only MCP-wrapped tools enforce a hard 60s `asyncio.wait_for` cutoff (`mcp_tool_wrapper.py:156-159`). Note that sync `close()` on a frame session still `thread.join()`s the worker (`utilities/streaming.py:284-285`), so closing the stream does not stop the underlying crew.

**5. Are partial outputs clearly marked?**
Moot for tools (there are none). For the artifacts that do stream, marking is explicit: `StreamFrame.channel="tools"` + preserved `frame.type` such as `tool_usage_started` (`utilities/streaming.py:174-190`; docs at `docs/edge/en/learn/streaming-runtime-contract.mdx:67-68`), and legacy `StreamChunk.chunk_type` distinguishes `TEXT` vs `TOOL_CALL` (`types/streaming.py:277-281`). The one ad-hoc case is unmarked and misleading: a Tavily SSE generator would reach the model stringified as `<generator object ...>` with no marker (`structured_tool.py:69-71`).

## Architectural Decisions

1. **Batch tool functions over streaming handlers.** `BaseTool` models a tool as `(args) -> result` with pydantic-validated schemas (`base_tool.py:139-158,326-366`). This keeps the authoring model simple but forecloses incremental protocols; contrast with the LLM layer where chunk events were designed in from the start.
2. **Observability via bus events, not tool callbacks.** All tool visibility flows through `crewai_event_bus` emissions at two fixed points (start/finish) plus error paths (`tool_usage.py:291,1020-1028,1058`). Adding progress reporting would require new event types and emission points inside the invocation path — the bus itself (`event_bus.py`, threaded executor + background loop, `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/events/event_bus.py:102-190`) could already carry them.
3. **Two-generation public streaming API.** The legacy `StreamChunk`/`CrewStreamingOutput` API forwards only LLM tokens/tool-call deltas (`utilities/streaming.py:354-356`); the newer `StreamFrame`/`StreamSession` contract routes every event, including tool lifecycle events, into typed channels (`utilities/streaming.py:143-190`). The design intent — a stable envelope for runtime/UI consumers — is documented (`docs/edge/en/learn/streaming-runtime-contract.mdx:10-45`).
4. **Scoped sinks via contextvars.** Stream sinks attach to the current execution context so concurrent streams don't cross-contaminate (`stream_context.py:12-30`; isolation test at `tests/test_streaming.py:931`).

## Notable Patterns

- **Lifecycle bracketing with `finally`**: finished-events fire from a `finally` block and are suppressed if an error event already fired, avoiding double-reporting (`tool_usage.py:487-495,744-752`).
- **Channel projections over one ordered timeline** rather than separate streams per concern: `stream.tools`, `stream.llm`, `interleave([...])` preserve global order (`types/streaming.py:135-157`; test at `tests/test_stream_frames.py:122-131`).
- **Hook veto with post-fire symmetry**: even a blocked call runs POST_TOOL_CALL hooks so monitoring hooks stay consistent (`utilities/tool_utils.py:127-143`).
- **Provider-agnostic argument streaming**: partial tool-call JSON is normalized into `LLMStreamChunkEvent.tool_call` regardless of provider (`llm_events.py:124-144`).

## Tradeoffs

- **Simplicity vs. liveness**: the batch tool contract makes tools trivially portable and cacheable, but long operations (research, scraping, MCP calls) present as silent gaps between the started and finished panels.
- **Session cancel ≠ work cancel**: cancelling a stream stops observation immediately, but sync paths join the worker thread (`utilities/streaming.py:284-285`) and nothing signals the tool; this avoids partial-write corruption yet means users cannot actually stop a runaway tool.
- **Timeouts only where wrapped**: MCP gets defensive timeouts (`mcp_tool_wrapper.py:80-81,156-159`) while native tools get none — inconsistent operational safeguards.
- **Dual streaming APIs**: maintaining both `StreamChunk` and `StreamFrame` contracts doubles the surface; the docs steer new integrations to frames (`concepts/streaming.mdx:16-19`).

## Failure Modes / Edge Cases

- **Generator-as-result hazard**: any tool returning an iterator/generator (as `TavilyResearchTool` can) yields a useless `str(<generator ...>)` to the model, because formatting falls through to `str(raw_result)` when no `result_schema` matches (`structured_tool.py:59-93`; tool at `tavily_research_tool.py:145-167`). This is the clearest evidence that tool-level streaming exists outside the framework's assumptions.
- **Silent hang**: a non-MCP tool that blocks indefinitely stalls the agent with no timeout and no intermediate feedback; only the started panel marks it "In Progress" (`console_formatter.py:404-413`).
- **Close-then-block**: calling `close()` on a sync stream session during a long run blocks on `thread.join()` until the crew finishes (`utilities/streaming.py:276-287`), which can surprise consumers expecting prompt cancellation.
- **Cache poisoning risk avoided, but at cost**: since only completed outputs are cached (`tools_handler.py:26-52`), interrupted runs leave no resumable partial state — restart repeats the whole tool call.

## Future Considerations

- Introduce an optional streaming protocol on `BaseTool` (e.g., `_run_stream` yielding typed updates, or a `progress` emitter injected like the existing `fingerprint_config`), mapped onto a new `tool_progress` event and the existing `"tools"` frame channel — the bus, frame envelope, channel projections, and ordering tests already exist to carry it (`utilities/streaming.py:143-190`; `tests/test_stream_frames.py:98-131`).
- Add opt-in per-tool timeouts mirroring the MCP wrapper, with the elapsed time surfaced on the started/finished events (`started_at`/`finished_at` already exist on `ToolUsageFinishedEvent`, `tool_usage_events.py:66-67`).
- Guard `_format_tool_output_for_agent` against non-stringifiable results (generators, iterators) with an explicit error or auto-consumption, closing the Tavily gap.
- Propagate async cancellation into tool bodies (e.g., `anyio` scopes) so `aclose()` can optionally stop work, not just observation.

## Questions / Gaps

- No evidence found of any first-party tool (outside Tavily's passthrough) emitting progress; search boundary: all of `lib/crewai-tools/src` and `lib/crewai/src/crewai/tools` for `yield`, `Iterator[`, `Generator[`, `progress_callback`.
- No evidence found of UI consumers rendering intra-tool progress; the devtools package (`studies/agent-harness-study/sources/crewai/lib/devtools`) contains CLI/docs utilities only, and the console formatter handles exactly started/finished/error states (`console_formatter.py:404-489`).
- Whether the Plus/enterprise hosted runtime streams tool progress could not be assessed from this source: `plus_api.py` concerns deployment telemetry, and no progress-related endpoints appear in-tree.

---

Generated by `dimensions/07.07-tool-output-streaming` against `crewai`.

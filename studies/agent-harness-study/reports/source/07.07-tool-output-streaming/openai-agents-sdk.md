# Source Analysis: openai-agents-sdk

## Dimension 07.07: Tool Output Streaming

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+ (asyncio; OpenAI Responses API; Pydantic) |
| Analyzed | 2026-08-25 |

## Summary

The OpenAI Agents SDK (Python) has a mature, well-tested **run-level streaming** architecture, but it deliberately has **no generic streaming tool API**: a function tool runs to completion and produces exactly one output. Streaming reaches the UI through three typed event channels (`src/agents/stream_events.py:10-61`): raw provider token deltas (`RawResponsesStreamEvent`), semantic item-completion events (`RunItemStreamEvent` with names such as `tool_called` and `tool_output`), and agent-change events (`AgentUpdatedStreamEvent`). Intra-tool progress exists in exactly two opt-in forms: `Agent.as_tool(on_stream=...)` forwards the nested run's full event stream to a user callback while the tool still resolves to one final value (`src/agents/agent.py:592,887-958`), and the experimental Codex tool always streams its thread internally and aggregates events into a single `CodexToolResult` (`src/agents/extensions/experimental/codex/codex_tool.py:374-401`).

Partial outputs are never persisted mid-tool and never shown to the model. Session persistence happens at turn boundaries via `_save_stream_items` (`src/agents/run_internal/run_loop.py:409-438`), tracing records a tool's input/output once at completion (`src/agents/tracing/span_data.py:135-166`), and the model receives a single `function_call_output` item built only after invocation plus output guardrails finish (`src/agents/items.py:845-923`, `src/agents/run_internal/tool_execution.py:2117-2131`). Cancellation during streaming is a first-class concern with two modes — `cancel("immediate")` tears down tasks and unblocks consumers, `cancel(mode="after_turn")` lets the current turn finish cleanly (`src/agents/result.py:818-864`) — and a cancelled tool's `CancelledError` is converted into a model-visible error string rather than crashing the run (`src/agents/run_internal/tool_execution.py:2069-2095`).

Net effect on the dimension question ("Can a long-running tool keep the user informed without confusing the model?"): yes for users, by construction for the model — the design keeps partials strictly on the UI side of the boundary.

## Rating

**7 / 10** — Clear model with explicit interfaces, tests, and operational safeguards for what it supports.

Rationale:
- The streaming event taxonomy is explicit and documented (`docs/streaming.md:74-96` lists all eleven `RunItemStreamEvent.name` values), the producer/consumer machinery has back-pressure (the background loop pauses between turns until consumers acknowledge items, `src/agents/result.py:866-880`), cancellation has two well-defined modes plus an extensive test suite (`tests/test_cancel_streaming.py`, 13 tests; `tests/test_soft_cancel.py`, 23 tests).
- The `on_stream` callback path for nested agents is carefully engineered (off-loop dispatch so slow handlers do not block consumption, handler exceptions logged but non-fatal; `src/agents/agent.py:905-938`, tested across ~1,300 lines in `tests/test_agent_as_tool.py:2334-3641`).
- It stops short of 9-10 because ordinary function tools cannot stream at all — there is no yielding/generator tool protocol, no per-chunk progress callback parameter on `function_tool` (verified: no `progress_callback`/`on_progress` symbol anywhere under `src/agents`), partial outputs are neither durable nor model-visible, and the only intra-tool streaming paths are opt-in callbacks (one of which, Codex, lives under `extensions/experimental/`).

## Evidence Collected

Every entry includes a file path with line numbers. Paths are relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Stream event types | `RawResponsesStreamEvent` wraps raw provider events; `RunItemStreamEvent` carries fixed names incl. `tool_called`, `tool_output`; `AgentUpdatedStreamEvent`; union `StreamEvent` | `src/agents/stream_events.py:10-20,23-48,51-61` |
| Event emission mapping | `stream_step_items_to_queue` maps `ToolCallItem → "tool_called"`, `ToolCallOutputItem → "tool_output"`; approval and compaction items are deliberately not streamed | `src/agents/run_internal/streaming.py:28-65` (exclusions at 56-59) |
| Raw event pump | Every provider stream event is put on the result queue as `RawResponsesStreamEvent(data=event)`; terminal `response.incomplete/failed/error` raise out | `src/agents/run_internal/run_loop.py:2249-2266` |
| Function tools are single-shot | `FunctionTool.on_invoke_tool: Callable[[ToolContext[Any], str], Awaitable[Any]]`; docstring requires "one of the structured tool output types … or a string representation"; no streaming variant exists | `src/agents/tool.py:455-466` |
| No streaming option in decorator | `function_tool(...)` params cover guardrails, timeout, approval, custom data extractor — nothing for streaming | `src/agents/tool.py:2454-2509` (impl `_on_invoke_tool_impl` awaits once at 2612-2666) |
| Agent-as-tool streaming callback | `as_tool(on_stream: Callable[[AgentToolStreamEvent], MaybeAwaitable[None]] \| None)`; when set, nested run uses `Runner.run_streamed` and forwards all events via producer/consumer queue | `src/agents/agent.py:592`, `887-958` |
| Slow-handler safety | Comment: dispatch callbacks off-loop so slow handlers do not block event consumption; exceptions logged, not fatal | `src/agents/agent.py:905-925` |
| Callback payload type | `AgentToolStreamEvent = {"event": StreamEvent, "agent": Agent, "tool_call": ResponseFunctionToolCall \| None}`; exported from package root | `src/agents/agent.py:130-140`; `src/agents/__init__.py:12,369` |
| Nested tool still resolves to one value | After streaming, tool returns `run_result.final_output` or extracts last message/output — parent conversation gets only this | `src/agents/agent.py:988-1013` |
| Codex tool streams internally | Comment "Always stream and aggregate locally to enable on_stream callbacks"; consumes `thread.run_streamed(...)` events then returns aggregated `CodexToolResult` | `src/agents/extensions/experimental/codex/codex_tool.py:374-401` (options field `on_stream` at 218, param at 254) |
| Lifecycle progress hooks | `RunHooksBase.on_tool_start(ctx, agent, tool)` / `on_tool_end(ctx, agent, tool, result)` fire around the whole invocation, not within it | `src/agents/lifecycle.py:70-105`; invoked at `src/agents/run_internal/tool_execution.py:2023,2158-2165` |
| Session persistence granularity | `_save_stream_items` writes completed `RunItem`s via `save_result_to_session`; called from turn-boundary helpers inside `start_streaming`, never during tool execution | `src/agents/run_internal/run_loop.py:409-438`, `1121-1147`, call sites `1857,1884,1914,1933` |
| Tracing records final state only | `FunctionSpanData(name, input, output, mcp_data).export()` emits one record; no incremental span updates | `src/agents/tracing/span_data.py:135-166` |
| Cancellation recorded in trace | On cancel, span gets `SpanError(message="Tool execution cancelled", data={tool_name, error})` | `src/agents/run_internal/tool_execution.py:2085-2094` |
| Run-level cancel modes | `RunResultStreaming.cancel(mode="immediate")` cleans up tasks and enqueues `QueueCompleteSentinel`; `mode="after_turn"` sets flag checked between turns | `src/agents/result.py:818-864` |
| Consumer/back-pressure protocol | `stream_events()` registers consumers; `_wait_for_turn_event_consumption` joins queue or waits for consumer stop before next turn | `src/agents/result.py:866-880,882-1024` |
| Cancelled tool → model-visible error | `except asyncio.CancelledError` routes through `failure_error_function`; if it yields a message the run continues instead of crashing | `src/agents/run_internal/tool_execution.py:2069-2095` |
| Mid-batch tool cancellation | Batch executor cancels sibling tasks, drains them bounded (`_drain_cancelled_function_tool_tasks`), arbitrates real failures over `CancelledError` | `src/agents/run_internal/tool_execution.py:1552-1783` (drain at ~307-548) |
| MCP cancellation propagation | `MCPServerUtil.invoke_mcp_tool` cancels child task on outer cancel; raises `MCPToolCancellationError`, formatted as model-visible error downstream | `src/agents/mcp/util.py:703-759`; `src/agents/exceptions.py:497-505` |
| Model sees exactly one output item | `ItemHelpers.tool_call_output_item` builds a single `{"call_id", "output", "type": "function_call_output"}` from the fully-completed value | `src/agents/items.py:845-923`; assembled after guardrails at `src/agents/run_internal/tool_execution.py:2106-2131` |
| SDK-only side channel on finished outputs | `ToolCallOutputItem.custom_data` populated from `custom_data_extractor` after invocation — attached to complete outputs, not model-visible | `src/agents/items.py:447-452`; `src/agents/run_internal/tool_execution.py:2143-2156` |
| UI consumption surface | `Runner.run_streamed` → `RunResultStreaming.stream_events()`; REPL prints `[tool called]` / `[tool output: …]` from item events | `src/agents/run.py:464-540`; `src/agents/repl.py:56-67` |
| Documented semantics | Event-name table ("progress updates at the level of 'message generated', 'tool ran'"), approvals+streaming flow, `cancel(mode="after_turn")` guidance | `docs/streaming.md:58-96` |
| Worked example | `on_stream=handle_stream` prints every nested event while the outer run stays non-streamed | `examples/agent_patterns/agents_as_tools_streaming.py:25-62` |
| Tests: nested streaming | on_stream suite: sync handlers, non-blocking dispatch, handler-exception logging, BaseException propagation | `tests/test_agent_as_tool.py:2334-3641` (e.g., 2334, 3069, 3150, 3530) |
| Tests: cancellation | 13 cancel-streaming tests incl. immediate-cancel unblocking waiting consumers; 23 soft-cancel tests | `tests/test_cancel_streaming.py:31,183`; `tests/test_soft_cancel.py` |
| Tests: streamed tool-call args | Regression test that streamed function-call argument deltas carry populated arguments at `tool_called` time | `tests/test_streaming_tool_call_arguments.py` |

## Answers to Dimension Questions

**1. Can tools stream progress?**
Ordinary function tools cannot: `FunctionTool.on_invoke_tool` must return a single awaited value (`src/agents/tool.py:455-466`), and the `function_tool` decorator exposes no streaming parameter (`src/agents/tool.py:2454-2509`). Two opt-in mechanisms exist. `Agent.as_tool(on_stream=...)` (`src/agents/agent.py:592`) runs the nested agent via `Runner.run_streamed` and forwards every event to the callback through an internal queue (`src/agents/agent.py:907-958`); the experimental Codex tool always streams its thread and invokes `on_stream` per event while aggregating locally (`src/agents/extensions/experimental/codex/codex_tool.py:374-401,1041-1075`). Coarse-grained progress for any tool is available externally via `RunItemStreamEvent("tool_called"/"tool_output")` (`src/agents/run_internal/streaming.py:40-47`) and `on_tool_start`/`on_tool_end` hooks (`src/agents/lifecycle.py:70-105`).

**2. Are partial outputs durable?**
No. There are no durable partial outputs. Session persistence writes completed `RunItem`s at turn boundaries (`src/agents/run_internal/run_loop.py:409-438`, `1121-1147`), and the trace span exports input/output as one record at completion (`src/agents/tracing/span_data.py:159-166`); a cancelled tool leaves a `"Tool execution cancelled"` span error instead of partial data (`src/agents/run_internal/tool_execution.py:2089-2094`). What *is* durably captured mid-run is interruption/approval state for resume: pending approvals land on `RunResultStreaming.interruptions` and convert to resumable `RunState` (`docs/streaming.md:38-56`).

**3. Does the model act on partial output?**
No. The model receives exactly one `function_call_output` per call, constructed only after invocation and output guardrails complete (`src/agents/run_internal/tool_execution.py:2106-2131` → `src/agents/items.py:918-923`). For agent-as-tools, nested partials go exclusively to the `on_stream` callback; the parent conversation sees only the extracted final output (`src/agents/agent.py:988-1013`). The closest thing to incremental model-side visibility is streaming of the model's own tool-call *arguments* as raw deltas (`src/agents/run_internal/run_loop.py:2251` passthrough), which is generation-side, not tool-output-side.

**4. Can users interrupt?**
Yes, at two levels. Run level: `result.cancel()` with `"immediate"` (tears down tasks, unblocks waiting stream consumers with a sentinel) or `"after_turn"` (finishes the current turn, persists session state, stops cleanly) (`src/agents/result.py:818-864`; documented at `docs/streaming.md:58-64`). Tool level: cancelling the outer task cancels in-flight tool tasks, drains siblings with bounded grace, converts `CancelledError` into a formatted model-visible error via `failure_error_function` (`src/agents/run_internal/tool_execution.py:2069-2095`), and propagates through MCP wrappers as `MCPToolCancellationError` (`src/agents/mcp/util.py:703-759`). Additionally, approval-required tools pause the run entirely before execution, exposed as `interruptions` (`docs/streaming.md:38-56`) — pre-execution gating rather than mid-stream interruption.

**5. Are partial outputs clearly marked?**
Partial outputs do not exist as artifacts to mark. At the event level, the taxonomy is explicit and self-describing: raw deltas vs. completed-item events are distinct dataclass types with literal `type` discriminators (`raw_response_event` / `run_item_stream_event` / `agent_updated_stream_event`, `src/agents/stream_events.py:19,48,58`), so a UI can never mistake a delta for a finished tool output. The SDK additionally keeps non-output bookkeeping out of the stream — approval placeholders and compaction items emit no events (`src/agents/run_internal/streaming.py:56-59`). SDK-only metadata on finished outputs is quarantined in `ToolCallOutputItem.custom_data` (`src/agents/items.py:447-452`), which never enters the model payload.

## Architectural Decisions

1. **Single-shot tool contract.** Tools resolve once; the streaming boundary sits around the whole run, not inside tools. This is visible in the `Awaitable[Any]` executor signature (`src/agents/tool.py:455`) and the one-item output builder (`src/agents/items.py:845-923`). Consequence: the model's conversation stays append-only and consistent; long-running tools simply block until done.
2. **Three-tier event model.** Raw provider deltas, semantic item events, and agent transitions are separate typed events (`src/agents/stream_events.py:10-61`), letting UIs choose fidelity without parsing provider payloads.
3. **Callback-based intra-tool streaming, not protocol-based.** Rather than adding a generator/yielding tool protocol, nested-agent and Codex tools expose an optional `on_stream` callback (`src/agents/agent.py:592`; `codex_tool.py:218`) — observability is decoupled from the tool-result contract.
4. **Producer/consumer with back-pressure everywhere.** The run loop pauses between turns until consumers acknowledge (`src/agents/result.py:866-880`), and the agent-as-tool callback runs on a side queue so slow handlers cannot stall the nested stream (`src/agents/agent.py:905-958`).
5. **Two-mode cancellation.** Immediate teardown versus graceful turn completion is a public, documented choice (`src/agents/result.py:818-864`) rather than a single kill switch, preserving session-state consistency on soft cancel.
6. **Failure-as-data for cancelled tools.** Cancellation is funneled through `failure_error_function` into a model-visible string (`src/agents/run_internal/tool_execution.py:2069-2095`), keeping the conversation valid even after interruption.

## Notable Patterns

- **Sentinel-unblocking:** immediate cancel pushes `QueueCompleteSentinel` onto the event queue so awaiting consumers wake up (`src/agents/result.py:855-858`).
- **Deliberate event exclusion list:** approvals and compaction items return `event = None` so internal bookkeeping never leaks to UIs (`src/agents/run_internal/streaming.py:56-59`).
- **Non-fatal callback errors:** `on_stream` handler exceptions are logged with diagnostic context and swallowed (`src/agents/agent.py:909-925`), preventing a buggy UI callback from failing an agent run.
- **Aggregating streamer:** the Codex tool streams purely to enable callbacks and reassembles the thread into `CodexToolResult(thread_id, response, usage)` (`codex_tool.py:374-401`) — streaming as an implementation detail behind a single-shot interface.
- **SDK-only metadata channel:** `custom_data_extractor` attaches JSON-safe data to finished `ToolCallOutputItem`s for host applications, explicitly outside the model-visible payload (`src/agents/run_internal/tool_execution.py:2143-2156`).

## Tradeoffs

- **Simplicity vs. liveness:** because arbitrary tools cannot report progress, a 10-minute tool shows the user nothing between `tool_called` and `tool_output` unless the developer wraps it as a nested agent or Codex tool. The escape hatches exist but are asymmetric in maturity (agent-as-tool is stable; Codex is under `extensions/experimental/`).
- **UI-only partials:** partials never reach the model, eliminating confusion/prompt-pollution risk, but also meaning a crashed tool's progress is unrecoverable — the model must retry from scratch with no partial-state memory.
- **Turn-boundary persistence vs. crash granularity:** persisting only completed items (`run_loop.py:409-438`) keeps sessions clean, at the cost of losing all in-flight work on process death.
- **Back-pressure vs. throughput:** requiring consumer acknowledgment between turns (`result.py:866-880`) guarantees no dropped events for slow UIs but couples run progress to consumption speed.
- **Compatibility debt kept visible:** the misspelled `handoff_occured` event name is frozen for backward compatibility, with the comment preserved in both code and docs (`src/agents/stream_events.py:32-33`; `docs/streaming.md:90`).

## Failure Modes / Edge Cases

- **Cancelled tool mid-batch:** sibling tasks are cancelled and drained with a bounded wait; a real exception in a cancelled task wins over bare `CancelledError` arbitration (`src/agents/run_internal/tool_execution.py:1552-1783`), so genuine failures aren't masked as cancellations.
- **Slow/broken `on_stream` handlers:** dispatched off-loop; exceptions logged as `"Error while handling an agent tool on_stream event"` and ignored (`src/agents/agent.py:915-925`; regression-tested at `tests/test_agent_as_tool.py:3069,3150`).
- **Terminal stream errors:** `response.incomplete/failed/error` events raise out of the raw pump, surfacing as run-loop exceptions through the iterator (`src/agents/run_internal/run_loop.py:2258-2266`).
- **Consumer abandonment:** `stream_events()` cleanup drains queues and unregisters consumers in `finally` blocks, so dropping the iterator doesn't deadlock the background loop (`src/agents/result.py:1012`, `1119-1124`).
- **Post-token tail work:** docs warn the stream isn't complete when the last token arrives — persistence, approval bookkeeping, and compaction can still be running (`docs/streaming.md:7,62`).

## Future Considerations

- A generic generator/yielding function-tool protocol would let ordinary Python tools report progress without wrapping them as nested agents; today the only sanctioned route is `as_tool(on_stream=...)`.
- Promoting the Codex-style "always stream + aggregate + callback" pattern out of `experimental/` into a stable tool base class would unify the two ad-hoc callback shapes (`AgentToolStreamEvent` dict vs. `CodexToolStreamEvent` dataclass).
- Durability hooks (e.g., checkpointing partial tool output into session or tracing) would let resumed runs benefit from interrupted work instead of discarding it.
- A first-class `progress` event kind in `RunItemStreamEvent` (or a fourth event tier) would standardize what currently requires custom callbacks.

## Questions / Gaps

- **No evidence found for per-chunk progress callbacks on plain function tools:** searched `src/agents` for `progress_callback`, `on_progress`, `AsyncIterator`-returning executors, and yielding tool protocols; none exist. This is an absence by design, not a missed feature.
- **Partial-output marking N/A:** since no partial outputs are persisted or emitted, the "clearly marked" question could only be evaluated at the event-taxonomy level (answered above).
- The exact drain bounds for cancelled sibling tasks (`_FUNCTION_TOOL_CANCELLED_DRAIN_SECONDS=0.25`) were located in `src/agents/run_internal/tool_execution.py` (~lines 167-169) but their precise tuning rationale is not documented in-code beyond comments; behavior is covered by tests rather than prose.

---

Generated by `07.07-tool-output-streaming` against `openai-agents-sdk`.

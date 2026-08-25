# Source Analysis: pydantic-ai

## Dimension 07.07: Tool Output Streaming

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (asyncio, pydantic, pydantic-graph; Starlette-based UI adapters) |
| Analyzed | 2026-08-25 |

> All citations below are relative to the selected source root `studies/agent-harness-study/sources/pydantic-ai/`.

## Summary

pydantic-ai deliberately does **not** let a running tool stream partial output to the model. A tool call is an awaited coroutine that returns one settled value (`AbstractToolset.call_tool(...) -> Any`, `pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:170-173`); there is no async-generator tool API and no progress-reporting method on `RunContext`. The framework's answer to long-running work is *deferral* (suspend the run, resolve outside it) rather than streaming: tools can be `kind='external'` or `'unapproved'` (`pydantic_ai_slim/pydantic_ai/tools.py:593-602`), pause the graph, and be resolved later via `DeferredToolResults`.

What pydantic-ai streams instead is **events around tool execution**: call events when a tool starts, result events as parallel tool tasks settle (`FunctionToolCallEvent` / `FunctionToolResultEvent`, `pydantic_ai_slim/pydantic_ai/messages.py:3977-4021`), batched deferred-request/result events for paused calls (`pydantic_ai_slim/pydantic_ai/messages.py:4060-4109`), and — at the UI layer — full protocol-level tool lifecycles (streamed argument deltas, input-available/error, output-available/error/denied) for AG-UI and Vercel AI frontends. Tools that want to push custom UI events can attach protocol event objects to their settled `ToolReturn.metadata`, but those are emitted only once the tool returns.

The model never sees partial tool output mid-flight; feedback channels are complete `ToolReturnPart`s, `RetryPromptPart`s, and synthesized `outcome='interrupted'` returns on repair. Cancellation during in-flight tool tasks is robustly handled (cancel-and-drain, first-party vs external cancellation arbitration). Net: excellent adjacent streaming infrastructure with a clear "settled-only" contract toward the model, but no first-class intra-tool progress channel — MCP progress notifications, the one place progress could flow from a long-running server-side tool, are a user-owned pass-through that never reaches agent events or UIs.

## Rating

**6 / 10** — Present but one-sided. Event-level streaming (call/result, deferred batches, UI protocol lifecycles) is explicit, tested, and operationally safeguarded; cancellation during streaming is mature. But the dimension's core capability — a long-running tool streaming progress or partial output — has no first-class API: tools cannot yield, there is no progress callback on `RunContext`, MCP `progress_handler` is forwarded to FastMCP without integration into the event stream (`pydantic_ai_slim/pydantic_ai/mcp.py:890,942,982`), and MCP task-augmented execution blocks until the final result (`pydantic_ai_slim/pydantic_ai/mcp.py:1341-1364`). Partial-output durability exists only at step boundaries (interrupted-return synthesis), not mid-tool.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Tool execution API is settle-only | `AbstractToolset.call_tool()` is an awaited coroutine returning `Any`; no iterator/generator return type anywhere in the toolset hierarchy | `pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:170-186` |
| Long-running answer is deferral, not streaming | Tool `kind` docs: `'external'` = result produced outside the run because it "could take longer to generate than it's reasonable to keep the agent process running"; `'unapproved'` = needs human approval | `pydantic_ai_slim/pydantic_ai/tools.py:593-602` |
| Per-tool timeout instead of progress | `Tool.timeout`: on overrun "a retry prompt is returned to the model" — binary failure feedback, not incremental progress | `pydantic_ai_slim/pydantic_ai/tools.py:610-615` |
| Structured tool return | `ToolReturn` separates model-visible `return_value`, app-only `metadata`, extra `content`, deferred-tool reveals; single-shot dataclass (no stream semantics) | `pydantic_ai_slim/pydantic_ai/messages.py:970-1006` |
| Tool call/result events | `ToolCallEvent` (with `args_valid`) → `FunctionToolCallEvent` / `OutputToolCallEvent`; `ToolResultEvent` → `FunctionToolResultEvent` / `OutputToolResultEvent` | `pydantic_ai_slim/pydantic_ai/messages.py:3946-4048` |
| Events streamed as parallel tasks complete | `_ExhaustiveProcessor._run_strategy` uses `asyncio.wait(..., FIRST_COMPLETED)` and yields each `FunctionToolResultEvent` on task completion; `cancel_and_drain` on error/cancellation | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:1179-1214` |
| Ordering modes for completion events | `parallel_execution_mode`: `'parallel'` (events as they complete), `'sequential'`, `'parallel_ordered_events'` (buffered results re-emitted in emission order) | `pydantic_ai_slim/pydantic_ai/tool_manager.py:42-45,171-189`; applied at `pydantic_ai_slim/pydantic_ai/_tool_execution.py:1143-1155,1251-1257` |
| Deferred-call visibility in streams | `DeferredToolRequestsEvent` emitted per batch before handlers run; `DeferredToolResultsEvent` on resolution — documented contract for frontend notification while a call is paused | `pydantic_ai_slim/pydantic_ai/messages.py:4059-4109`; `docs/deferred-tools.md:487-515` |
| No progress callback on RunContext | `RunContext` offers `cancel()`, `enqueue()`, private `_emit_event` (used only by framework capabilities), nothing for progress reporting | `pydantic_ai_slim/pydantic_ai/_run_context.py:280-287,413,467-494` |
| MCP progress pass-through gap | `progress_handler` param accepted and forwarded verbatim to FastMCP client config; never surfaced as agent/UI events | `pydantic_ai_slim/pydantic_ai/mcp.py:890,942,982` |
| MCP long-running tools block until done | Task-augmented execution (SEP-1686/SEP-2663): `_call_tool_as_task` awaits `tool_task.result()` — durable/pollable server-side, but caller sees only the completed result; tests confirm transparent drive-to-completion | `pydantic_ai_slim/pydantic_ai/mcp.py:1341-1364,1463-1477`; `tests/test_mcp.py:1915-2025`; `docs/mcp/client.md:358-395` |
| Partial args buffering (model→harness direction) | `ModelResponsePartsManager.handle_tool_call_delta` buffers string deltas of in-flight tool-call args; these deltas stream out to UIs but are harness-internal, not tool output | `pydantic_ai_slim/pydantic_ai/_parts_manager.py:358-380` |
| Boundary-level partial persistence | On exception/mid-stream close, completed function-tool returns still appended to history so interrupted requests record them (`finally` capture) | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:1265-1282` |
| Interrupted-return synthesis | Dangling tool calls repaired deterministically with `INTERRUPTED_TOOL_RETURN_CONTENT`, `outcome='interrupted'`, marker metadata, derived timestamps | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2830-2898`; content/outcome defined at `pydantic_ai_slim/pydantic_ai/messages.py:1290-1341` |
| Cancellation during tool streaming | In-flight tasks cancelled+drained with message (`cancel_and_drain`); first-party vs external cancellation arbitrated via `Task.uncancel()` bookkeeping; `RunContext.cancel()` callable from inside tools; sub-agent self-cancel isolated as failed return | `pydantic_ai_slim/pydantic_ai/_utils.py:312-326`; `pydantic_ai_slim/pydantic_ai/_tool_execution.py:1209-1214`; `pydantic_ai_slim/pydantic_ai/_cancel.py:92-242`; `pydantic_ai_slim/pydantic_ai/_run_context.py:467-494`; `pydantic_ai_slim/pydantic_ai/_tool_execution.py:41-62` |
| UI integration: base stream tracker | `UIEventStream` keeps `_pending_tool_calls` and `_open_part`(+deltas) state; error/cancel closeout synthesizes `interrupted`/`failed` returns for pending calls "so the UI doesn't show them as still running" | `pydantic_ai_slim/pydantic_ai/ui/_event_stream.py:124-142,318-358` |
| UI integration: AG-UI lifecycle | `ToolCallStartEvent` → raw-partial-JSON `ToolCallArgsEvent` deltas → `ToolCallEndEvent` → `ToolCallResultEvent`; comment documents emitting args fragments that are not yet valid JSON | `pydantic_ai_slim/pydantic_ai/ui/ag_ui/_event_stream.py:333-371` |
| Tool-driven UI events (on return only) | Tools return AG-UI `BaseEvent`s (e.g. `StateSnapshotEvent`, `CustomEvent`) inside `ToolReturn.metadata`; adapter forwards them after the result settles | `pydantic_ai_slim/pydantic_ai/ui/ag_ui/_event_stream.py:451-461`; `docs/ui/ag-ui.md:367-430` |
| UI integration: Vercel AI lifecycle | `ToolInputDeltaChunk` (args streaming), `tool-input-available`, v6 `tool-input-error` suppression, backfill of missed input announcements on interrupted runs, `ToolOutputAvailable/Error/DeniedChunk`, data chunks from `ToolReturnPart` metadata | `pydantic_ai_slim/pydantic_ai/ui/vercel_ai/_event_stream.py:292-474` |
| Model never sees partial results | Settled results normalized by `build_tool_return_part` into complete `ToolReturnPart` (+ optional user content); only complete parts enter `output_parts`/history | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:65-116` |
| Tests | Deferred request/result events (`test_run_stream_deferred_tool_requests_and_results`), cancelled runs close tools as interrupted, raw arg-delta emission, dict-arg compaction, MCP task routing | `tests/test_ui.py:652-761,856`; `tests/test_ag_ui.py:4788,4870`; `tests/test_mcp.py:1962-2025` |

## Answers to Dimension Questions

1. **Can tools stream progress?**
   Not first-class. Function/MCP tools execute to settlement with no API to emit progress (`call_tool -> Any`, `pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:170-173`; no progress member on `RunContext`, `pydantic_ai_slim/pydantic_ai/_run_context.py:280-287,467-494`). Progress-like observability exists only at call granularity: `FunctionToolCallEvent` on start, `FunctionToolResultEvent` as each parallel task completes (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:1183-1208`), plus `DeferredToolRequestsEvent`/`DeferredToolResultsEvent` framing paused calls (`pydantic_ai_slim/pydantic_ai/messages.py:4059-4109`). MCP `notifications/progress` reach a user-supplied `progress_handler` directly and never enter the agent's event stream (`pydantic_ai_slim/pydantic_ai/mcp.py:890-942,982`).

2. **Are partial outputs durable?**
   Only at boundaries. If a step fails or the consumer closes mid-stream, already-completed tool returns are flushed into message history (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:1265-1282`), and dangling calls get deterministic synthesized `outcome='interrupted'` returns during history repair (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2844-2898`). There is no persistence of intra-tool partial output — none is produced. For whole-run durability, Temporal/Prefect/DBOS integrations checkpoint the run rather than stream tool output.

3. **Does the model act on partial output?**
   No — by design. The model receives only settled, normalized results (`build_tool_return_part`, `pydantic_ai_slim/pydantic_ai/_tool_execution.py:65-116`) or retry prompts/timeouts (`pydantic_ai_slim/pydantic_ai/tools.py:610-615`). This is a deliberate confusion-prevention contract: the model's next turn always sees consistent, complete tool messages; interrupted turns see explicitly-marked `'interrupted'` outcomes rather than fabricated partials (`pydantic_ai_slim/pydantic_ai/messages.py:1290-1341`).

4. **Can users interrupt?**
   Yes, comprehensively. First-party cancellation (`AgentRun.cancel()` / `RunContext.cancel()`, thread-safe `CancellationToken`) cancels the driving task; in-flight tool tasks are cancelled and drained with a message (`cancel_and_drain`, `pydantic_ai_slim/pydantic_ai/_utils.py:312-326`; applied at `pydantic_ai_slim/pydantic_ai/_tool_execution.py:1209-1214`); first-party vs external cancellation is disambiguated via `Task.cancelling()/uncancel()` accounting with documented degraded behavior on Python 3.10 (`pydantic_ai_slim/pydantic_ai/_cancel.py:11-20,203-242`). Model-stream cancellation additionally requires models to implement `close_stream()` (`pydantic_ai_slim/pydantic_ai/models/__init__.py:1120-1127`).

5. **Are partial outputs clearly marked?**
   Where they exist, yes. Interrupted calls are marked three ways consistently: `outcome='interrupted'` on the part (`pydantic_ai_slim/pydantic_ai/messages.py:1335-1341`), shared `INTERRUPTED_TOOL_RETURN_CONTENT` text used identically by graph repair, realtime sessions, and UI closeout (`pydantic_ai_slim/pydantic_ai/messages.py:1290-1295`; consumers at `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2881`, `pydantic_ai_slim/pydantic_ai/ui/_event_stream.py:344-351`), and `SYNTHESIZED_TOOL_RETURN_METADATA_KEY` metadata for inspection (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2848,2883`). Vercel AI adapters distinguish `input-streaming → input-available → output-*` states and even suppress/backfill announcements to keep frontends out of stuck states (`pydantic_ai_slim/pydantic_ai/ui/vercel_ai/_event_stream.py:334-357,420-453`). However, genuine partial tool *output* never exists to be marked.

## Architectural Decisions

- **Settled-result contract toward the model.** Tools are coroutines returning one value; normalization into `ToolReturnPart`s happens once, post-settlement (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:65-116`). This trades intra-tool liveness for guaranteed consistency of what the model reasons over.
- **Deferral over streaming for long-running work.** Instead of keeping a process alive to trickle results, `kind='external'`/`'unapproved'` tools suspend the graph and resume on supplied results (`pydantic_ai_slim/pydantic_ai/tools.py:593-602`; `docs/deferred-tools.md:23-91,321-486`), which composes with durable-execution backends.
- **Events as the streaming surface.** All liveness flows through typed `AgentStreamEvent`s consumed uniformly by `run_stream_events`, `agent.iter` node streaming, capabilities, and UI adapters (`pydantic_ai_slim/pydantic_ai/messages.py:3946-4109`; `pydantic_ai_slim/pydantic_ai/agent/abstract.py:1307`).
- **Parallelism with selectable event ordering.** `'parallel'` vs `'parallel_ordered_events'` lets consumers choose between earliest completion signal and deterministic emission order — an explicit tradeoff knob (`pydantic_ai_slim/pydantic_ai/tool_manager.py:171-189`; `pydantic_ai_slim/pydantic_ai/_tool_execution.py:1153-1155`).
- **Protocol adapters own wire semantics.** Each UI protocol gets a dedicated `UIEventStream` subclass translating internal events, including version-gated behavior (AG-UI reasoning events, Vercel SDK v5/v6 differences) (`pydantic_ai_slim/pydantic_ai/ui/ag_ui/_event_stream.py:303-320`; `pydantic_ai_slim/pydantic_ai/ui/vercel_ai/_event_stream.py:347-357`).

## Notable Patterns

- **Closeout honesty:** both the graph and every UI stream classify interruption vs error and mark pending tool calls accordingly (`interrupted` vs `failed`), so neither the model nor a reloading frontend misreads a cancelled call as a tool bug (`pydantic_ai_slim/pydantic_ai/ui/_event_stream.py:330-358`).
- **Raw-fragment passthrough for streamed args:** AG-UI emits partial JSON args verbatim because concatenation with later deltas is what makes them valid — avoiding lossy re-encoding (`pydantic_ai_slim/pydantic_ai/ui/ag_ui/_event_stream.py:350-358`).
- **Backfill/suppression bookkeeping:** the Vercel adapter stashes streamed call parts and suppresses/re-emits `tool-input-available` so validation failures and interrupted runs still produce a well-formed chunk sequence (`pydantic_ai_slim/pydantic_ai/ui/vercel_ai/_event_stream.py:300-308,420-453`).
- **Tools as UI producers:** a settled `ToolReturn.metadata` carrying protocol events gives tools an escape hatch to update UI state or emit customs without any new framework API (`docs/ui/ag-ui.md:367-430`).
- **Cancellation hygiene as a library invariant:** `cancel_and_drain` + controller-counted cancellations ensure child tool tasks never leak across run teardown (`pydantic_ai_slim/pydantic_ai/_utils.py:312-326`; `pydantic_ai_slim/pydantic_ai/_cancel.py:1-23`).

## Tradeoffs

- **Consistency vs liveness for the model:** the model can't course-correct based on early partial data; a slow tool simply stalls its turn until timeout produces a retry prompt (`pydantic_ai_slim/pydantic_ai/tools.py:610-615`) or the run defers.
- **User-informed vs integration cost:** keeping users informed mid-tool requires either building a custom toolset whose implementation surfaces progress through side channels, or using MCP progress handlers outside the framework's event model — both bypass the unified event stream.
- **Early-completion events vs ordering:** `'parallel'` mode yields results immediately but out of emission order; ordered consumers must opt into buffered delivery and accept delayed signals (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:1153-1155`).
- **Protocol richness vs forward compatibility:** adapters gate new protocol features behind installed-version checks, so newer capabilities (e.g. AG-UI activity snapshots for tool-availability deltas) silently no-op on older clients (`pydantic_ai_slim/pydantic_ai/ui/ag_ui/_event_stream.py:397-408`).

## Failure Modes / Edge Cases

- **Mid-stream consumer close:** the exhaustive processor's `finally` re-appends only un-appended parts, guarding against duplicate history appends when interruption lands between a yield suspension points (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:1166-1180,1265-1282`).
- **Error racing tool completion:** concurrent tool errors arriving inside an MCP `ExceptionGroup` are split so a grouped cancellation is never swallowed as a tool error (`pydantic_ai_slim/pydantic_ai/mcp.py:1420-1447`).
- **Stuck-spinner prevention:** UI closeout synthesizes terminal results for every dispatched-but-unsettled call on error/cancel (`pydantic_ai_slim/pydantic_ai/ui/_event_stream.py:318-358`); tests pin this (`tests/test_ui.py:856`, `tests/test_ag_ui.py:641-687`).
- **Cancellation attribution races:** documented residual window where user code calling `Task.uncancel()` can make a first-party cancel resolve as external (or vice versa) — acknowledged limitation with issue reference (#7240) (`pydantic_ai_slim/pydantic_ai/_cancel.py:213-223`).
- **Python 3.10 degraded disambiguation:** first-party cancellation wins attribution when external races cannot be distinguished (`pydantic_ai_slim/pydantic_ai/_cancel.py:18-20,227-230`).

## Future Considerations

- A first-class progress channel would fit the existing event architecture naturally: e.g. a `RunContext.report_progress()` that emits into `_event_stream_buffer` (`pydantic_ai_slim/pydantic_ai/_run_context.py:280-287`) and maps to MCP progress notifications and/or UI data chunks.
- Surfacing MCP task-augmented polls as observable events (rather than blocking on `await tool_task.result()`, `pydantic_ai_slim/pydantic_ai/mcp.py:1357-1364`) would let UIs show server-side task status.
- Bridging the MCP `progress_handler` into the agent event stream would unify progress observability for the most common source of genuinely long-running tools.

## Questions / Gaps

- **No evidence found** for any public API letting a user tool yield intermediate values to the model or the event stream; searches covered `tools.py`, `toolsets/*`, `tool_manager.py`, `_function_schema.py`, and `RunContext` for generator/iterator signatures and "progress"/"partial" naming.
- **No evidence found** that MCP progress notifications are recorded, logged into runs, or forwarded beyond the user-supplied handler; the handler is configured and forgotten (`pydantic_ai_slim/pydantic_ai/mcp.py:982,1039`).
- Whether the realtime session exposes richer mid-call tool updates was not exhaustively traced beyond its reuse of `INTERRUPTED_TOOL_RETURN_CONTENT` closeout (`pydantic_ai_slim/pydantic_ai/realtime/_session.py:31,1961,2481`).
- CLI behavior: the bundled chat loop streams model text only and does not render tool-call events, so terminal users get no per-tool progress (`pydantic_ai_slim/pydantic_ai/_cli/__init__.py:440-461`).

---

Generated by `07.07-tool-output-streaming` against `pydantic-ai`.

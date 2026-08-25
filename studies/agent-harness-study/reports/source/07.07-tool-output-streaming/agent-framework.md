# Source Analysis: agent-framework

## 07.07 Tool Output Streaming

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | C#/.NET (`dotnet/src/*`), Python (`python/packages/*`); `go/` contains only a README placeholder (`go/README.md`) |
| Analyzed | 2026-08-25 |

## Summary

The repository (Microsoft agent-framework) implements tool output streaming in a deliberately asymmetric way: **local tools cannot stream**. A local tool executes to completion and returns one whole result, which is emitted as a single update chunk. In Python, `FunctionTool.invoke()` returns `list[Content] | Any` with no async-generator variant (`python/packages/core/agent_framework/_tools.py:587-595`), and the streaming function-invocation loop executes tools only *after* the model turn finalizes, then yields the finished result as one post-hoc `ChatResponseUpdate` (`python/packages/core/agent_framework/_tools.py:3424-3479`). In .NET, all tool paths are single-shot `ValueTask<object?> InvokeAsync(...)`-shaped; nothing in-repo returns `IAsyncEnumerable` from a tool, and the framework's own docs state that `FunctionInvokingChatClient` "emits each call as a complete FunctionCallContent (it never splits a call across updates)" (`dotnet/src/Microsoft.Agents.AI/ChatClient/InvocableFunctionBypassingChatClient.cs:121-131`).

What *does* stream incrementally falls into four categories:

1. **Model-side deltas**: function-call argument deltas stream as chunks and are merged at finalize (`python/packages/core/tests/core/test_function_invocation_logic.py:623-662` proves the canonical shape: two argument-delta updates → one whole-result update → final text).
2. **Provider-executed / hosted tools**: the OpenAI Responses client maps `response.code_interpreter_call_code.delta` into incremental code-interpreter content (`python/packages/openai/agent_framework_openai/_chat_client.py:2977-3023`) and partial image-generation output (`python/packages/openai/agent_framework_openai/_chat_client.py:3203-3230`).
3. **MCP SEP-2663 long-running tasks**: polling-based progress (`WorkingTaskResult` states) rather than streamed partials, with best-effort remote cancellation (`dotnet/src/Microsoft.Agents.AI.Mcp/TaskAwareMcpClientAIFunction.cs:215-233`; Python equivalent at `python/packages/core/agent_framework/_mcp.py:2299-2302`).
4. **Background-agent tools**: harness tools return a task id immediately ("Background task {taskId} started...") that the model polls via status/result tools (`dotnet/src/Microsoft.Agents.AI/Harness/BackgroundAgents/BackgroundTaskStatus.cs:12-33`).

Progress visibility for long-running local work therefore comes only from side doors — call-boundary middleware/hooks (`pre_tool_call`/`post_tool_call`, `python/packages/core/agent_framework/_agent_hooks.py:1420-1481`; `FunctionInvocationContext.result`, `python/packages/core/agent_framework/_middleware.py:270-351`) or poll/patterns — never from intra-tool progress events. There is no progress-callback API for tool authors in either stack.

Compensating strengths: partial-output **durability** is strong (continuation tokens serialize accumulated `ResponseUpdates` plus input messages — `dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientAgentContinuationToken.cs:32-59,164-169`; per-service-call history persistence inside the function loop — `dotnet/src/Microsoft.Agents.AI/ChatClient/PerServiceCallChatHistoryPersistingChatClient.cs:15-19` and Python `python/packages/core/agent_framework/_agents.py:1244-1315`), and **cancellation** is plumbed end-to-end including remote MCP task cancellation. UI integration is real but coarse-grained: AG-UI maps `TOOL_CALL_START/ARGS/END/RESULT` to streamed function-call contents (`python/packages/ag-ui/agent_framework_ag_ui/_event_converters.py:82-204`).

## Rating

**4 / 10 — Present but inconsistent.**

Rationale against the rubric:
- The central capability — a long-running *local* tool streaming progress/partial output to user and model — is absent by design in both stacks (no yield/generator tool protocol; single-shot invoke signatures cited above). This alone caps the score.
- What exists is inconsistent across surfaces: provider-executed tools stream richly (code deltas, partial images) while local tools produce exactly one result chunk; .NET marks executed calls via an in-place `InformationalOnly` flip (`dotnet/src/Microsoft.Agents.AI/ChatClient/MessageInjectingChatClient.cs:333-361`) whereas Python emits metadata-only copies with `arguments=None` (`python/packages/core/agent_framework/_tools.py:2843-2846`) — different models for the same concept.
- It is not rated 1–3 because the surrounding machinery is explicitly engineered, tested, and safe: fail-closed buffered streaming gates (`python/packages/core/agent_framework/_types.py:3233-3283`), durable continuation tokens proven under consumer abandonment (`dotnet/tests/Microsoft.Agents.AI.UnitTests/ChatClient/ChatClientAgent_BackgroundResponsesTests.cs:526-813`), cooperative batch cancellation with sibling-task cleanup (`python/packages/core/agent_framework/_tools.py:1863-1876`), and a documented function-loop specification (`docs/specs/004-python-function-calling-loop.md` referenced from `python/AGENTS.md`).
- Not 7–8 because there is no explicit interface for a tool author to emit progress, no partial-result persistence during execution (only after each completed model turn), and no test anywhere exercising intra-tool progress because the capability does not exist.

## Evidence Collected

Every entry includes a file path with line numbers. Paths are relative to the source root `studies/agent-harness-study/sources/agent-framework`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Tool invocation signature (Python) | `async def invoke(...) -> list[Content] \| Any` — whole-result return type; docstring says raw value is "automatically parsed into a ``list[Content]``" | python/packages/core/agent_framework/_tools.py:587-609 |
| Sync tool execution path | `_invoke_function` runs sync tools via `await asyncio.to_thread(self.__call__, ...)` — no per-chunk consumption | python/packages/core/agent_framework/_tools.py:555-563 |
| Whole-result parsing | `parse_result(result)` converts the entire return value post-execution | python/packages/core/agent_framework/_tools.py:849 |
| Streaming function-invocation loop | `_stream_response_with_function_invocation`: streams model turn live (`_tools.py:3417-3422`), awaits `get_final_response()`, then executes calls and yields finished results (`_tools.py:3424`, `_tools.py:3448-3479`) | python/packages/core/agent_framework/_tools.py:3337-3486 |
| Tool results as one chunk | `_handle_function_call_results` builds `ChatResponseUpdate(contents=execution_results, role="tool")` — complete batch in a single update | python/packages/core/agent_framework/_tools.py:2862-2870 |
| Canonical streaming-shape test | `test_base_client_with_streaming_function_calling`: asserts `len(updates) == 4 # two updates with the function call, the function result and the final text` | python/packages/core/tests/core/test_function_invocation_logic.py:657-662 |
| Parallel batch execution & cancellation | Tasks created via `contextvars.copy_context().run(asyncio.create_task, ...)`; on abort siblings cancelled and awaited; comment notes sync tools in threads cannot be interrupted | python/packages/core/agent_framework/_tools.py:1848-1876 |
| Call-boundary hooks only | `pre_tool_call` emitted before invoke, `post_tool_call` after — no mid-execution events exist | python/packages/core/agent_framework/_agent_hooks.py:1420-1422,1479-1481 |
| Middleware result-level observation | `FunctionInvocationContext.result` "Can be observed after calling ``call_next()``" — boundary-only visibility | python/packages/core/agent_framework/_middleware.py:270-351 |
| Fail-closed gated streaming | `ResponseStream.buffered_and_gated`: "fully buffered stream... Nothing has egressed yet"; sealed `_GatedResponseStream` raises if hooks registered late | python/packages/core/agent_framework/_types.py:3233-3283,3543-3577 |
| Hosted tool code streaming (provider side) | `case "response.code_interpreter_call_code.delta":` yields incremental code-interpreter content per delta | python/packages/openai/agent_framework_openai/_chat_client.py:2977-2998 |
| Partial image streaming | `response.image_generation_call.partial_image` → content with `partial_image_index` / `is_partial_image` properties | python/packages/openai/agent_framework_openai/_chat_client.py:3203-3230 |
| Argument-delta streaming | `response.function_call_arguments.delta` → incremental `Content.from_function_call(arguments=event.delta)` chunks | python/packages/openai/agent_framework_openai/_chat_client.py:3188-3202 |
| In-progress status content | `Content.from_shell_tool_call(status=...)` documented as `"in_progress", "completed", "incomplete"` | python/packages/core/agent_framework/_types.py:1100-1137 |
| Continuation-token semantics | `ChatResponseUpdate.continuation_token`: "token for resuming a long-running background operation. When present, indicates the operation is still in progress." | python/packages/core/agent_framework/_types.py:2506,2583-2584 |
| Persistence timing (finalize-only) | Streaming `_post_hook` persists history only after stream completes; per-service-call history persisted inside the loop per turn, not per chunk | python/packages/core/agent_framework/_agents.py:1279-1280,1308-1312 |
| MCP long-running task polling | `MCPTaskOptions.cancel_remote_task_on_local_cancellation=True` triggers best-effort `tasks/cancel` then re-raises `CancelledError` | python/packages/core/agent_framework/_mcp.py:346-386 |
| MCP cancel-on-abandonment | `except asyncio.CancelledError: self._spawn_best_effort_cancel(task_id); raise` inside `call_tool_as_task` polling lifecycle | python/packages/core/agent_framework/_mcp.py:2209-2311 |
| .NET tool shape (single-shot) | Agent-as-tool wraps `InvokeAgentAsync` into `AIFunctionFactory.Create` returning `Task<string>` — non-streaming despite agents supporting streaming | dotnet/src/Microsoft.Agents.AI/AgentExtensions.cs:67-89 |
| Custom tool extension pattern | `MiddlewareEnabledFunction : DelegatingAIFunction` overriding `InvokeCoreAsync(AIFunctionArguments, CancellationToken) : ValueTask<object?>` — no streaming override exists | dotnet/src/Microsoft.Agents.AI/FunctionInvocationDelegatingAgent.cs:70-86 |
| No intra-call splitting (authoritative comment) | "FunctionInvokingChatClient emits each call as a complete FunctionCallContent (it never splits a call across updates)... buffers per iteration... streams the next iteration live" | dotnet/src/Microsoft.Agents.AI/ChatClient/InvocableFunctionBypassingChatClient.cs:121-131 |
| InformationalOnly in-place flip | FICC flips `FunctionCallContent.InformationalOnly = true` once executed — the only mid-stream state marker for tool calls | dotnet/src/Microsoft.Agents.AI/ChatClient/MessageInjectingChatClient.cs:333-361 |
| Streaming API surface | `RunStreamingAsync` overloads + core iterator with `[EnumeratorCancellation] CancellationToken`; abstract `RunCoreStreamingAsync` | dotnet/src/Microsoft.Agents.AI.Abstractions/AIAgent.cs:383-503 |
| Explicit dispose for abandoned streams | `ChatClientAgent.RunCoreStreamingAsync` disposes enumerator in `finally` so in-flight `FunctionResultContent`/`FunctionCallContent` state persists when consumers break early | dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientAgent.cs:330-406 |
| Durable partial updates | `ChatClientAgentContinuationToken` serializes `InputMessages` + `ResponseUpdates` ("response updates received so far") | dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientAgentContinuationToken.cs:32-59,164-169 |
| Resume-from-partial tests | Background-response resume tests restore streaming from saved `InputMessages`/`ResponseUpdates` | dotnet/tests/Microsoft.Agents.AI.UnitTests/ChatClient/ChatClientAgent_BackgroundResponsesTests.cs:526-813 |
| Per-service-call checkpointing | `PerServiceCallChatHistoryPersistingChatClient` persists "after each service call within the FunctionInvokingChatClient loop"; fixups prevent orphaned in-flight results | dotnet/src/Microsoft.Agents.AI/ChatClient/PerServiceCallChatHistoryPersistingChatClient.cs:15-19,236-237,288-289 |
| Bounded partial-output capture | `HeadTailBuffer`: bounded accumulator keeping head+tail halves with `[... truncated N bytes ...]` marker, used "when streaming stdout/stderr from a long-running subprocess" — but released only at completion | dotnet/src/Microsoft.Agents.AI.Tools.Shell/HeadTailBuffer.cs:9-29 |
| Polling-based long-running tasks (.NET) | `PollTaskToCompletionAsync` handles `WorkingTaskResult`/`InputRequiredTaskResult`/terminal states; local cancellation triggers `TryCancelTaskAsync` (best-effort, timeout-guarded) | dotnet/src/Microsoft.Agents.AI.Mcp/TaskAwareMcpClientAIFunction.cs:132-212,215-233,270-281 |
| Background task status enum | `enum BackgroundTaskStatus { Running, Completed, Failed, Lost }` — polled by the model, not streamed | dotnet/src/Microsoft.Agents.AI/Harness/BackgroundAgents/BackgroundTaskStatus.cs:12-33 |
| CT propagation tests | `RunAsync_CancellationToken_PropagatedToMiddlewareAsync`; builder-level propagation test | dotnet/tests/Microsoft.Agents.AI.UnitTests/FunctionInvocationDelegatingAgentTests.cs:860; dotnet/tests/Microsoft.Agents.AI.UnitTests/AnonymousDelegatingAIAgentTests.cs:932 |
| Streamed tool events over SSE (UI) | Integration test asserts updates contain `FunctionCallContent` and matching `FunctionResultContent` over the AG-UI SSE protocol | dotnet/tests/Microsoft.Agents.AI.Hosting.AGUI.AspNetCore.IntegrationTests/ToolCallingTests.cs:50-70 |
| AG-UI event mapping (UI emitters) | `TOOL_CALL_START`/`TOOL_CALL_ARGS`/`TOOL_CALL_END`/`TOOL_CALL_RESULT` ↔ `ChatResponseUpdate(contents=[from_function_call(...)])` | python/packages/ag-ui/agent_framework_ag_ui/_event_converters.py:82-204 |
| DevUI streaming executor | `execute_streaming(request)` — "Execute request and stream results in OpenAI format" | python/packages/devui/agent_framework_devui/_executor.py:222 |
| Model-waits-for-completion sample | MCP long-running sample README: under `RunStreamingAsync` "the model still waits for the tool's terminal result before it can begin producing the final answer" | dotnet/samples/02-agents/ModelContextProtocol/Agent_MCP_LongRunningTask_Client/README.md:25 |
| User interrupt of stream | Sample cancels in-flight request via `task.cancel()` catching `asyncio.CancelledError` | python/samples/02-agents/chat_client/chat_response_cancellation.py:36-42 |

## Answers to Dimension Questions

### 1. Can tools stream progress?

**No for local tools, yes for provider-executed tools.** Local tools are single-shot in both stacks: Python `FunctionTool.invoke(...) -> list[Content] | Any` (`python/packages/core/agent_framework/_tools.py:587-595`) and .NET `InvokeCoreAsync(...) -> ValueTask<object?>` (`dotnet/src/Microsoft.Agents.AI/FunctionInvocationDelegatingAgent.cs:72`). Neither stack offers an async-generator/yield tool API; a grep for generator-returning tool signatures found none. Provider-executed hosted tools do stream genuine incremental output: OpenAI Responses code-interpreter code deltas become incremental `code_interpreter_tool_call` contents (`python/packages/openai/agent_framework_openai/_chat_client.py:2977-2998`) and partial images arrive as flagged partial-image contents (`python/packages/openai/agent_framework_openai/_chat_client.py:3203-3230`). Progress for slow local work is approximated by polling designs (MCP tasks: `dotnet/src/Microsoft.Agents.AI.Mcp/TaskAwareMcpClientAIFunction.cs:132-212`; background-agent tools: `dotnet/src/Microsoft.Agents.AI/Harness/BackgroundAgents/BackgroundTaskStatus.cs:12-33`).

### 2. Are partial outputs durable?

**Partially.** Two distinct answers:
- *Within a run*: yes and well-engineered. .NET continuation tokens serialize accumulated `ResponseUpdates` so a dropped connection resumes from the last received update (`dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientAgentContinuationToken.cs:164-169`, resume tests at `dotnet/tests/Microsoft.Agents.AI.UnitTests/ChatClient/ChatClientAgent_BackgroundResponsesTests.cs:526-813`). History is persisted after each service call inside the function loop (`dotnet/src/Microsoft.Agents.AI/ChatClient/PerServiceCallChatHistoryPersistingChatClient.cs:15-19`), and Python persists per completed provider turn inside its loop (`python/packages/core/agent_framework/_agents.py:1308-1312`).
- *Within a tool execution*: no. Nothing a tool computes before returning is captured; the shell executor accumulates subprocess stdout/stderr in memory only (`dotnet/src/Microsoft.Agents.AI.Tools.Shell/HeadTailBuffer.cs:17-19`) and discards context on truncation boundaries. If a process dies mid-command, intermediate output is lost unless the caller re-runs.

### 3. Does the model act on partial output?

**No — the model sees only terminal results.** Tool execution happens strictly between finalized model turns: Python awaits `get_final_response()` before executing calls (`python/packages/core/agent_framework/_tools.py:3424`), and .NET's own architecture comment confirms calls are never split across updates (`dotnet/src/Microsoft.Agents.AI/ChatClient/InvocableFunctionBypassingChatClient.cs:123-124`). An explicit sample README documents that even with a 15-second MCP task under streaming, "the model still waits for the tool's terminal result before it can begin producing the final answer" (`dotnet/samples/02-agents/ModelContextProtocol/Agent_MCP_LongRunningTask_Client/README.md:25`). The closest the model gets to acting on incomplete information is the background-task pattern: receive a task id now, poll status later (`dotnet/src/Microsoft.Agents.AI/Harness/BackgroundAgents/BackgroundTaskStatus.cs:12-33`), which converts waiting into explicit model-driven polling rather than streamed feedback.

### 4. Can users interrupt?

**Yes, cooperatively.** `CancellationToken`s flow from public streaming APIs down to tool bodies (`dotnet/src/Microsoft.Agents.AI.Abstractions/AIAgent.cs:465-480`; propagation tested at `dotnet/tests/Microsoft.Agents.AI.UnitTests/FunctionInvocationDelegatingAgentTests.cs:860`). Users can cancel an in-flight stream directly (`python/samples/02-agents/chat_client/chat_response_cancellation.py:36-42`). Parallel tool batches cancel sibling tasks on failure and guarantee discarded results never reach the transcript (`python/packages/core/agent_framework/_tools.py:1863-1876`). For MCP long-running tasks, local cancellation propagates remotely via best-effort `tasks/cancel` (`dotnet/src/Microsoft.Agents.AI.Mcp/TaskAwareMcpClientAIFunction.cs:215-233`; `python/packages/core/agent_framework/_mcp.py:2299-2302`). The documented limit: synchronous Python tools running in worker threads cannot be interrupted mid-body — their results are simply discarded (`python/packages/core/agent_framework/_tools.py:1868-1872`).

### 5. Are partial outputs clearly marked?

**Yes where they exist, though marking schemes differ per stack.** Python flags in-progress hosted operations via content `status` fields (`Content.from_shell_tool_call(status="in_progress")`, `python/packages/core/agent_framework/_types.py:1120-1122`), `additional_properties={"is_partial_image": True}` (`python/packages/openai/agent_framework_openai/_chat_client.py:3203-3230`), and `continuation_token` presence signaling an unfinished operation (`python/packages/core/agent_framework/_types.py:2583-2584`). .NET marks already-executed calls via in-place `InformationalOnly=true` mutation (`dotnet/src/Microsoft.Agents.AI/ChatClient/MessageInjectingChatClient.cs:333-361`) and truncation via `ShellResult.Truncated` plus the `[... truncated N bytes ...]` marker text (`dotnet/src/Microsoft.Agents.AI.Tools.Shell/HeadTailBuffer.cs:13`). Tests verify the semantics (`python/packages/openai/tests/openai/test_openai_chat_client.py:2807-2818` asserts `status == "in_progress"` mapping). However, there is no unified "this content is partial" contract spanning both stacks — consumers must know which marker applies to which content type.

## Architectural Decisions

1. **Terminal-result-only tool contract.** Both stacks commit tools to a request→whole-result model. Python's `parse_result` runs on the complete return value (`python/packages/core/agent_framework/_tools.py:849`); .NET delegates entirely to Microsoft.Extensions.AI's `AIFunction` (external NuGet dependency pinned at `dotnet/Directory.Packages.props:84-85`). This keeps transcript semantics simple — every `function_result` is atomic — which the Python function-loop spec treats as load-bearing (`docs/specs/004-python-function-calling-loop.md`, referenced from `python/AGENTS.md`): small changes risk duplicating side effects or orphaning call/result pairs.
2. **Streaming lives at the response layer, not the tool layer.** `ChatResponseUpdate`/`AgentResponseUpdate` are the universal streaming currency (`python/packages/core/agent_framework/_types.py:2506,2913`; `dotnet/src/Microsoft.Agents.AI.Abstractions/AgentResponseUpdate.cs:35-100`), and tool activity surfaces as discrete content items inside those updates — never as a separate progress channel.
3. **Iteration-boundary observability instead of intra-tool observability.** FICC buffers per iteration and releases updates at iteration ends (`dotnet/src/Microsoft.Agents.AI/ChatClient/InvocableFunctionBypassingChatClient.cs:126-131`); Python yields finished result batches between turns (`python/packages/core/agent_framework/_tools.py:3478-3479`). Consumers can observe *that* tools ran, never *how far along* one is.
4. **Long-running work is decomposed into poll loops, not streams.** MCP SEP-2663 tasks (`WorkingTaskResult` states, `dotnet/src/Microsoft.Agents.AI.Mcp/TaskAwareMcpClientAIFunction.cs:132-212`) and harness background-agent tools (`background_agents_start_task` returning ids, `dotnet/src/Microsoft.Agents.AI/Harness/BackgroundAgents/BackgroundAgentsProvider.cs:590-635`) convert latency into model-visible state machines the model queries deliberately.
5. **Durability is invested where streams break, not where tools compute.** Continuation tokens carrying accumulated updates (`dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientAgentContinuationToken.cs:32-59`), explicit enumerator disposal on early consumer exit (`dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientAgent.cs:340-406`), and fail-closed gating that prevents any egress until hooks finalize (`python/packages/core/agent_framework/_types.py:3233-3283`) show the design priority is crash/connection resilience over live progress.

## Notable Patterns

- **Argument-delta merging**: consecutive streamed `function_call` contents merge via `__add__` when building the final response (`python/packages/core/agent_framework/_types.py:1984-1988,1479`), letting argument deltas stream freely while preserving a single logical call.
- **Metadata-only echo for declaration-only calls**: streamed declaration-only function calls get a follow-up update copy with `arguments=None` so call ids survive aggregation without duplicated arguments (`python/packages/core/agent_framework/_tools.py:2843-2846`).
- **Fail-closed buffered gating**: agent-hook middleware uses `buffered_and_gated` so streaming works without leaking unobserved content past hooks ("Streaming is supported fail-closed by buffering", `python/packages/core/agent_framework/_agent_hooks.py:72-78`).
- **In-place instance mutation as state signal**: flipping `InformationalOnly` on the exact `FunctionCallContent` instances flowing through the pipeline lets downstream clients distinguish executed vs. bypassed calls without extra bookkeeping (`dotnet/src/Microsoft.Agents.AI/ChatClient/InvocableFunctionBypassingChatClient.cs:127-131`).
- **Bounded-memory capture with lossy markers**: `HeadTailBuffer` guarantees O(cap) memory for unbounded subprocess output while explicitly marking what was dropped (`dotnet/src/Microsoft.Agents.AI.Tools.Shell/HeadTailBuffer.cs:10-27`).
- **UI protocols consume the same update stream**: AG-UI converters map streamed function-call contents to `TOOL_CALL_*` events (`python/packages/ag-ui/agent_framework_ag_ui/_event_converters.py:82-204`); the .NET AG-UI integration test asserts tool-call/result contents arrive over SSE (`dotnet/tests/Microsoft.Agents.AI.Hosting.AGUI.AspNetCore.IntegrationTests/ToolCallingTests.cs:50-70`). One stream serves model, host, and UI.

## Tradeoffs

- **Atomicity vs. liveness**: the terminal-result-only contract makes transcripts trivially consistent and replay-safe (a stated concern of the function-loop spec), at the cost of zero intra-tool feedback — users stare at silence during a 15-second tool call except for whatever the host renders from call-start events.
- **Simplicity for tool authors vs. capability ceiling**: writing a tool is easy in both stacks (plain function/delegate), but there is no sanctioned escape hatch to report progress; authors needing liveness must restructure into multiple model-visible calls or external task stores themselves.
- **Provider asymmetry**: hosted/provider-executed tools get first-class streaming (because providers emit deltas), while locally-executed tools cannot — the same conceptual operation behaves differently depending on who executes it.
- **Cooperative cancellation vs. hard interrupts**: asyncio/CancellationToken plumbing is thorough and even reaches remote MCP servers, but thread-bound sync tools remain uninterruptible (`python/packages/core/agent_framework/_tools.py:1868-1872`) — a deliberate tradeoff of Python's `to_thread` execution model.
- **Durable resumption vs. payload size**: serializing every `ResponseUpdate` into continuation tokens (`dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientAgentContinuationToken.cs:169`) buys resumability at the cost of token size growth proportional to stream length.

## Failure Modes / Edge Cases

- **Silent stalls**: since no intra-tool progress exists, a hung local tool produces an indefinitely silent stream until cancellation; detection depends entirely on host-level timeouts (e.g., shell session timeout + interrupt-grace linked CTS at `dotnet/src/Microsoft.Agents.AI.Tools.Shell/ShellSession.cs:354,363-368`).
- **Uninterruptible sync tools**: cancelling a batch abandons but does not stop running sync tool bodies; their side effects continue even though results are discarded (`python/packages/core/agent_framework/_tools.py:1868-1876`).
- **Remote-cancel ambiguity**: if best-effort `tasks/cancel` fails silently (it swallows exceptions, `dotnet/src/Microsoft.Agents.AI.Mcp/TaskAwareMcpClientAIFunction.cs:277-280`), the server-side task keeps running with no client-visible indication.
- **Abandonment-vs-terminal distinction**: Python carefully separates paths where the remote task may still be running (always cancels) from terminal failures (never cancels) using the `_MCPTaskAbandoned` marker (`python/packages/core/agent_framework/_mcp.py:335` region, documented behavior) — mishandling this would either double-execute side-effecting tools or leak orphaned remote work.
- **Partial-egress safety under hooks**: registering hooks after a gate seals raises rather than silently skipping them (`python/packages/core/agent_framework/_types.py:3543-3577`) — fail-loud instead of fail-open.
- **Consumer-abandoned streams**: breaking out of `.NET` streaming mid-function-loop could orphan paired call/result contents; mitigated by explicit disposal and per-service-call persistence fixups (`dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientAgent.cs:340-345`; `dotnet/src/Microsoft.Agents.AI/ChatClient/PerServiceCallChatHistoryPersistingChatClient.cs:236-237`), with a dedicated test for abandoning enumeration (`dotnet/tests/Microsoft.Agents.AI.UnitTests/ChatClient/PerServiceCallChatHistoryPersistingChatClientTests.cs:1395`).
- **Delta accumulation divergence**: code-interpreter deltas are intentionally *not* accumulated downstream like reasoning deltas, so done-events always re-emit full content (`python/packages/openai/agent_framework_openai/_chat_client.py:2999-3001`) — a consumer assuming uniform accumulation will duplicate code text.

## Future Considerations

- A generator-style tool API (e.g., `AsyncIterable[Content]` invoke variants) would let long-running local tools push progress through the existing `ChatResponseUpdate` pipeline with minimal architectural change, since the transport already supports arbitrary content chunks; the open problem is transcript semantics (what a partially-complete `function_result` means for replay, compaction, and approval pairing — all specified as atomic groups today, see `python/packages/core/agent_framework/_compaction.py:916-969` strategies operating on completed call groups).
- Unifying partial-state marking (Python `status` strings, .NET `InformationalOnly`, `is_partial_image` properties) behind one cross-stack contract would reduce consumer complexity.
- Extending the background-agent poll pattern (`BackgroundTaskStatus`) into a general "job tool" primitive would give model-driven progress a standard shape without violating the atomic-result invariant.

## Questions / Gaps

- **No evidence found for any progress-callback or on-progress event parameter on tool invocation in either stack.** Searched Python core for generator/`AsyncIterable` tool signatures and .NET src for `IAsyncEnumerable`-returning tool members; none exist. The absence appears intentional but is not discussed in any design doc within the source (no ADR/design note located; searches covered `docs/`, `*.md` adjacent to tool code).
- **No evidence found of intra-tool partial output reaching the model.** All sampled and tested flows show the model receiving terminal results only (`dotnet/samples/02-agents/ModelContextProtocol/Agent_MCP_LongRunningTask_Client/README.md:25` is explicit). If any provider adapter forwards mid-tool content to the model, it was not discoverable in this checkout.
- **Microsoft.Extensions.AI internals are external.** `FunctionInvokingChatClient`'s own buffering behavior is observable here only through comments and integration effects (`dotnet/src/Microsoft.Agents.AI/ChatClient/InvocableFunctionBypassingChatClient.cs:121-131`), not its source; claims about its internals are inferred from these authoritative in-repo comments plus observed test assertions.
- The `go/` directory contains only `go/README.md` and contributes no evidence to this dimension.

---

Generated by dimension 07.07 (Tool Output Streaming) against `agent-framework`.

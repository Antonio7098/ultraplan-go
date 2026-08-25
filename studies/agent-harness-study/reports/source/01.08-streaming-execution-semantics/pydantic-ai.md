# Source Analysis: pydantic-ai

## Streaming Execution Semantics

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python 3.10+ / asyncio-first, pydantic-ai-slim + pydantic-graph, multi-provider streaming models |
| Analyzed | 2026-07-03 |

## Summary

Pydantic AI exposes a layered streaming surface: model-level token streaming (`Model.request_stream()` yielding `StreamedResponse` events of type `PartStartEvent`, `PartDeltaEvent`, `PartEndEvent`, `FinalResultEvent`), agent-level node streaming inside the graph (`ModelRequestNode.stream()` / `CallToolsNode.stream()` exposed through `AgentStream`), and an optional run-level event handler pipeline (`EventStreamHandler` + the `ProcessEventStream` capability's `wrap_run_event_stream` hook). Public entry points are `agent.run_stream(...)` (yields `StreamedRunResult`), `agent.run_stream_events(...)` (yields `AgentEventStream`), `agent.iter(...)` (graph-level async iteration yielding `AgentNode`s and `End`), and `agent.run(..., event_stream_handler=...)` for non-streaming runs that still surface events.

Partial tool calls are reconstructed by the `ModelResponsePartsManager` which keeps `ToolCallPartDelta` items in the internal `_parts` list until a name is known, then promotes them to a typed `ToolCallPart` (or further to `NativeToolCallPart`, `ToolSearchCallPart` via `ToolDefinition.tool_kind`) via `handle_tool_call_delta()` and `handle_tool_call_part()` (`pydantic_ai_slim/pydantic_ai/_parts_manager.py:292-440`). `PartDeltaEvent` is then emitted each time a delta is applied, so consumers see text/thinking/tool-call drips in real time.

Streaming output **is** persisted into the message history at completion (via `ModelRequestNode._append_response`, `_finish_handling`, and `StreamedRunResult._record_response`/`_marked_completed`), but a truncated response (e.g. `finish_reason='length'` mid-tool-call) is explicitly labeled with `state='incomplete'` on `ModelResponse`. Cancellation is cooperative and well-supported: `StreamedResponse.cancel()` closes the underlying connection, flips `_cancelled=True`, suppresses transport errors via the `iterator_with_cancel_guard` wrapper so iteration ends gracefully, and the truncated response is recorded as `state='interrupted'`. User-facing cancellation from outside the stream is plumbed through `_run_stream_events`'s `CancelledError` handler that calls `_utils.cancel_and_drain(task)`, and the `ModelRequestNode.stream()` readiness wait explicitly drains the `wrap_model_request` capability task on outer cancellation so user cleanup runs.

Retry behavior is layered: HTTP-level retries are handled outside the stream by `AsyncTenacityTransport` + `wait_retry_after` (`pydantic_ai_slim/pydantic_ai/retries.py:215-381`); model-level retries on `ModelRetry` produce a fresh `ModelRequestNode` via `_build_retry_node` but the partial prior response is preserved in history with `state='incomplete'` so the next request sees the truncated context (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1027-1040`); output-tool retries **are not** streamed-replayable (error: `Output validation failed during streaming, and retries are not supported in run_stream()`) at `pydantic_ai_slim/pydantic_ai/result.py:304-308`. The streaming model validation path runs through `AgentStream.validate_response_output(allow_partial=True)` so partial structured outputs are tolerated, but final `allow_partial=False` validation is reserved for the terminal call.

## Rating

**9 / 10 — Mature, observable, extensible streaming surface, with explicit lifecycle states (`complete` / `incomplete` / `interrupted`), per-vendor cancellation, and a tested capability-based wrap hook.**

Rationale:
- Token-level streaming is the default for every `Model` subclass via the `StreamedResponse` ABC + `_parts_manager` (`pydantic_ai_slim/pydantic_ai/models/__init__.py:718-939`), with four discriminated event types and a `PartEndEvent` synthesized per-part for UI consumers (`iterator_with_part_end` at `pydantic_ai_slim/pydantic_ai/models/__init__.py:769-804`).
- Partial tool calls are first-class: kept as `ToolCallPartDelta` until a name arrives, then promoted via `narrow_type` so `isinstance(part, ToolSearchCallPart)` is true from the first `PartStartEvent` (`pydantic_ai_slim/pydantic_ai/_parts_manager.py:64-97`, `pydantic_ai_slim/pydantic_ai/messages.py:2547-2648`).
- Cancellation is per-vendor (overridable `close_stream()` + `get_stream_cancel_errors()`), streams are guarded by an `iterator_with_cancel_guard` that suppresses `httpx.StreamError`/`httpx.TransportError` (`pydantic_ai_slim/pydantic_ai/models/__init__.py:806-868`), and tests explicitly cover partial-state persistence (`tests/test_streaming.py:4624-4958`).
- The output stream enters `state='incomplete'` whenever the response was truncated (`_cancelled=False`, `_finished=False`) and `state='interrupted'` after `cancel()` (`pydantic_ai_slim/pydantic_ai/models/__init__.py:884-903`, `messages.py:2159-2161`).
- A backwards-incompatible legacy `_event_stream_handler` slot still exists on `Agent` (removal is gated on the deprecation handler `consume_deprecated_event_stream_handler` at `pydantic_ai_slim/pydantic_ai/agent/__init__.py:436-441`), giving two coexisting event-delivery surfaces (legacy direct hook + new capability `wrap_run_event_stream`).
- Deducted points: cancelled `ModelRequestNode.stream()` requires `cancel()` to be called via `StreamedResponse`, not exposed at the `AgentStream` boundary for arbitrary mid-iteration `break` (the `AgentEventStream.__aexit__` propagates and `_run_stream_events` drains the producer task in `BaseException`/cancel paths at `pydantic_ai_slim/pydantic_ai/agent/abstract.py:1305-1321`); stream retry after partial failure is not supported for output-tool validation (`result.py:304-308`).

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| `StreamedResponse` ABC (model streaming primitive) | `class StreamedResponse(ABC)` with `_event_iterator`, `_cancelled`, `_finished`, `_parts_manager` | `pydantic_ai_slim/pydantic_ai/models/__init__.py:717-741` |
| Event union (4 discriminated types) | `ModelResponseStreamEvent = PartStartEvent \| PartDeltaEvent \| PartEndEvent \| FinalResultEvent` | `pydantic_ai_slim/pydantic_ai/messages.py:2800-2803` |
| `PartStartEvent` / `PartDeltaEvent` / `PartEndEvent` dataclasses | Discriminator `event_kind`, `index`, `previous_part_kind`, `next_part_kind` for UI grouping | `pydantic_ai_slim/pydantic_ai/messages.py:2716-2783` |
| `FinalResultEvent` | Carries `tool_name`, `tool_call_id`; first matching event stops consumption | `pydantic_ai_slim/pydantic_ai/messages.py:2786-2797` |
| `iterator_with_final_event` (first-match-wins wrapper) | Breaks out after final result, then drains remaining events | `pydantic_ai_slim/pydantic_ai/models/__init__.py:751-767` |
| `iterator_with_part_end` (PartEndEvent synthesis) | Synthesizes `PartEndEvent` between consecutive parts so UI sees run lifecycle | `pydantic_ai_slim/pydantic_ai/models/__init__.py:769-804` |
| `iterator_with_cancel_guard` (cancel safety net) | `try/except self.get_stream_cancel_errors()`, sets `_finished` only on natural completion | `pydantic_ai_slim/pydantic_ai/models/__init__.py:806-830` |
| `StreamedResponse.cancel()` + overridable `close_stream()` | `cancel()` flips `_cancelled=True` before delegating; default `close_stream()` raises `NotImplementedError` to fail clearly on unsupported transports | `pydantic_ai_slim/pydantic_ai/models/__init__.py:833-868` |
| `get_stream_cancel_errors()` default | `tuple[httpx.StreamError, httpx.TransportError]` — overridable per provider transport | `pydantic_ai_slim/pydantic_ai/models/__init__.py:846-855` |
| `StreamedResponse.get()` (state inference) | Returns ModelResponse with `state='interrupted'`, `'complete'`, or `'incomplete'` based on `_cancelled`/`_finished` | `pydantic_ai_slim/pydantic_ai/models/__init__.py:884-903` |
| `ModelResponseState` typing | `Literal['complete', 'incomplete', 'interrupted']` with `state: ModelResponseState = 'complete'` on `ModelResponse` | `pydantic_ai_slim/pydantic_ai/messages.py:123`, `2159-2161` |
| `ModelResponsePartsManager` (partial tool-call buffer) | Tracks `_parts`, `_vendor_id_to_part_index`, `_tool_kind_by_name` so streamed `ToolCallPart`s auto-promote via `ToolDefinition.tool_kind` | `pydantic_ai_slim/pydantic_ai/_parts_manager.py:57-97` |
| `handle_text_delta` | Creates `TextPart` on first delta or appends via `TextPartDelta.apply`, yielding `PartStartEvent`/`PartDeltaEvent` | `pydantic_ai_slim/pydantic_ai/_parts_manager.py:121-207` |
| `handle_tool_call_delta` (partial tool call accumulation) | Managed items remain `ToolCallPartDelta` until they have a tool_name, then upgraded to `ToolCallPart`/`NativeToolCallPart`; emits `PartStartEvent` on upgrade, `PartDeltaEvent` on subsequent update | `pydantic_ai_slim/pydantic_ai/_parts_manager.py:292-388` |
| `handle_tool_call_part` (full overwrite for non-delta providers) | Used by providers that emit complete tool calls in one chunk (Anthropic) | `pydantic_ai_slim/pydantic_ai/_parts_manager.py:390-440` |
| `ToolCallPartDelta.as_part()` | Returns `ToolCallPart` only when `tool_name_delta` is set; otherwise returns `None` | `pydantic_ai_slim/pydantic_ai/messages.py:2583-2598` |
| `ToolCallPartDelta._apply_to_delta` | Merges successive string-deltas into running `args_delta` until promotion | `pydantic_ai_slim/pydantic_ai/messages.py:2633-2646` |
| `narrow_type` for typed tool subclasses | `ToolCallPart.narrow_type` promotes to `ToolSearchCallPart` etc., based on `ToolDefinition.tool_kind` | `pydantic_ai_slim/pydantic_ai/messages.py:1877-1895`, `pydantic_ai_slim/pydantic_ai/_parts_manager.py:86-97` |
| `AgentStream` (run-level stream wrapper) | Holds `_raw_stream_response`, `drain()`, `cancel()`, `stream_output(allow_partial=True)`, `stream_text(delta=...)`, `validate_response_output(allow_partial=...)` | `pydantic_ai_slim/pydantic_ai/result.py:55-243` |
| `stream_response()` (incomplete-snapshot iteration) | Yields `state='incomplete'` while streaming, final `state='complete'/'interrupted'` snapshot | `pydantic_ai_slim/pydantic_ai/result.py:105-124` |
| `validate_response_output(allow_partial=True)` | Lets partial structured outputs be yielded; `allow_partial=False` only on the final call | `pydantic_ai_slim/pydantic_ai/result.py:245-309` |
| `_get_usage_checking_stream_response` (enforces usage limits mid-stream) | Wraps `StreamedResponse` with `limits.check_tokens(get_usage())` per event | `pydantic_ai_slim/pydantic_ai/result.py:1070-1084` |
| `StreamedRunResult.cancel()` records interrupted response | Sets `is_complete=True` and appends `_record_response(self.response)` so `all_messages()` includes the interrupted `state='interrupted'` `ModelResponse` | `pydantic_ai_slim/pydantic_ai/result.py:759-779` |
| `AgentEventStream` async context manager | `__aenter__/__aexit__/aclose()` ensures generator cleanup on early exit; standalone iteration deprecated | `pydantic_ai_slim/pydantic_ai/result.py:995-1051` |
| `agent.run_stream_events` (public event stream entry) | Builds an `async with anyio.create_memory_object_stream` and a producer task via `self.run(..., event_stream_handler=...)` | `pydantic_ai_slim/pydantic_ai/agent/abstract.py:1252-1325` |
| `_run_stream_events` CancelledError handling | Drains producer task via `_utils.cancel_and_drain(task, msg=...)` and re-raises | `pydantic_ai_slim/pydantic_ai/agent/abstract.py:1307-1321` |
| `agent.run` with `event_stream_handler` | On `_needs_streaming`, custom `_stream_and_advance` invokes `n.stream(...)` and routes through `wrap_run_event_stream` | `pydantic_ai_slim/pydantic_ai/agent/abstract.py:407-444` |
| `agent.run_stream` (final-result-driven streaming) | Per-node `node.stream(...)` + `wrap_run_event_stream`; drains `wrapped` after handler returns | `pydantic_ai_slim/pydantic_ai/agent/abstract.py:770-911` |
| `ModelRequestNode.stream()` | Async context manager yielding `AgentStream`; cooperative readiness hand-off with `wrap_model_request` via `stream_ready`/`stream_done` events | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:606-758` |
| `ModelRequestNode.stream()` outer cancellation handling | Drains `ready_waiter` and `wrap_task` so user `wrap_model_request` cleanup runs before re-raise | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:678-693` |
| `CallToolsNode.stream()` | Streams `HandleResponseEvent`s (`FunctionToolCallEvent`, `FunctionToolResultEvent`, `OutputToolCallEvent`, `OutputToolResultEvent`, `BuiltinToolCallEvent`/`BuiltinToolResultEvent`) | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1067-1300` |
| `ModelRequestNode._finish_handling` (persists streamed response) | Appends to `state.message_history` via `_append_response`, sets `_result = CallToolsNode(response)` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:964-990`, `1016-1025` |
| `_build_retry_node` (stream-aware retry path) | Preserves `_handler_response` in history so the retry sees the truncated context | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:752`, `828-835`, `1027-1040` |
| `GraphAgentState.check_incomplete_tool_call` | Detects `finish_reason='length'` mid-tool-call and raises `IncompleteToolCall` with mitigation hint | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:146-160` |
| `consume_output_retry` | Output-validation budget tracker; raises `UnexpectedModelBehavior` once `output_retries_used > max_output_retries` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:162-179` |
| `EventStreamHandler`/`EventStreamProcessor` type aliases | Function/async-generator hook for events from the model's streaming response and tool execution | `pydantic_ai_slim/pydantic_ai/agent/abstract.py:80-91` |
| `Agent._event_stream_handler` (legacy slot) | Stored on every agent; deprecated, auto-remapped via `consume_deprecated_event_stream_handler` | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:259`, `436-441`, `540` |
| `ProcessEventStream` capability | Replaces `wrap_run_event_stream`; runs on top of the model stream, supports `EventStreamHandler` or `EventStreamProcessor` | `pydantic_ai_slim/pydantic_ai/capabilities/process_event_stream.py:16-63` |
| `wrap_run_event_stream` hook (capabilities) | Composable hook on `AbstractCapability`; `CombinedCapability` chains child wrappers in reverse order | `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:247-249`, `540`; `pydantic_ai_slim/pydantic_ai/capabilities/combined.py:335-343` |
| `OpenAIStreamedResponse.close_stream()` | `await self._response.source.close()` to drop the HTTP connection | `pydantic_ai_slim/pydantic_ai/models/openai.py:3261-3262` |
| `OpenAIResponsesStreamedResponse.close_stream()` | Override closing the Responses-API source | `pydantic_ai_slim/pydantic_ai/models/openai.py:3461` |
| `AnthropicStreamedResponse.close_stream()` | Implement Claude-specific stream tear-down | `pydantic_ai_slim/pydantic_ai/models/anthropic.py:2382` |
| `Mistral.close_stream()` | Mistral SDK-specific cancellation | `pydantic_ai_slim/pydantic_ai/models/mistral.py:698` |
| `GoogleStreamedResponse.close_stream()` | Gemini SDK cancellation | `pydantic_ai_slim/pydantic_ai/models/google.py:1311` |
| `GroqStreamedResponse.close_stream()` | Groq SDK cancellation | `pydantic_ai_slim/pydantic_ai/models/groq.py:645` |
| `TestStreamedResponse._cancelled` simulation | Raises `httpx.StreamClosed` mid-iteration when `_cancelled` is True to mimic real-provider transport behavior | `pydantic_ai_slim/pydantic_ai/models/test.py:339-342` |
| `TestModel.request_stream` | Async context manager yielding a `TestStreamedResponse` that streams word-by-word | `pydantic_ai_slim/pydantic_ai/models/test.py:131-153` |
| `FunctionModel.request_stream` | Wraps a user-provided async iterator (`stream_function`) in a `PeekableAsyncStream` | `pydantic_ai_slim/pydantic_ai/models/function.py:160-188` |
| `AsyncTenacityTransport` (HTTP-level retries) | `Tenacity`-based wrapper for httpx that retries idempotent requests and honors `Retry-After` | `pydantic_ai_slim/pydantic_ai/retries.py:215-294` |
| `wait_retry_after` (per-header wait strategy) | Parses integer-seconds or HTTP-date `Retry-After` headers; falls back to exponential | `pydantic_ai_slim/pydantic_ai/retries.py:312-381` |
| `UIEventStream` (UI protocol base class) | Transforms native events into protocol-specific events for AG-UI / Vercel AI / Web adapters | `pydantic_ai_slim/pydantic_ai/ui/_event_stream.py:80-339` |
| `UIEventStream.transform_stream` cancel/error cleanup | On mid-stream error, closes pending tool calls with `ToolReturnPart(outcome='failed')` before emitting `on_error()` events | `pydantic_ai_slim/pydantic_ai/ui/_event_stream.py:231-269` |
| `UIEventStream.handle_event` dispatcher | `match event` over `PartStartEvent`, `PartDeltaEvent`, `PartEndEvent`, `FinalResultEvent`, `FunctionTool*`, `OutputTool*`, `AgentRunResultEvent` | `pydantic_ai_slim/pydantic_ai/ui/_event_stream.py:292-339` |
| Cancellation test - `state='interrupted'` visibility | `assert result.response.state == 'interrupted'` and snapshot of `all_messages()` | `tests/test_streaming.py:4638-4675` |
| Cancellation test - guard suppresses transport error | Validates `iterator_with_cancel_guard` swallows `httpx.StreamClosed` when `cancel()` was called | `tests/test_streaming.py:4678-4714` |
| Cancellation test - external task cancellation | Cancels outer task; expects consumer to raise `CancelledError` and producer cleanup to finish | `tests/test_streaming.py:4846-4865` |
| Cancellation test - managed cancellation waits for cleanup | `SlowCleanupTestModel`'s stream `finally` runs `asyncio.sleep(0.2)` before yielding; `__aexit__` waits | `tests/test_streaming.py:4868-4909` |
| Capability cancellation test - readiness wait drains wrapper | Outer cancellation during `asyncio.wait({ready_waiter, wrap_task})` drains the user `wrap_model_request` so its `finally` runs | `tests/test_streaming.py:4912-4955` |
| Stream error tests catalog | `test_anthropic_midstream_status_error`, `test_openai_midstream_connection_error`, `test_bedrock_midstream_error`, etc. cover per-provider failure modes | `tests/test_streaming_errors.py:217-967` |
| Fallback streaming triggers fallback | `test_fallback_model_streaming_error_triggers_fallback` ensures `FallbackModel` re-routes on mid-stream errors | `tests/test_streaming_errors.py:937` |

## Answers to Dimension Questions

1. **What is emitted while execution is still running?**  
   `PartStartEvent` / `PartDeltaEvent` / `PartEndEvent` for each streamed text/thinking/tool-call part (`pydantic_ai_slim/pydantic_ai/messages.py:2716-2803`), `FinalResultEvent` once an output-tool match is observed (`models/__init__.py:752-762`), and tool-side events (`FunctionToolCallEvent`, `FunctionToolResultEvent`, `OutputToolCallEvent`, `OutputToolResultEvent`, plus deprecated `BuiltinTool*Event`) from `CallToolsNode._run_stream` (`_agent_graph.py:1093-1300`). `AgentStream` exposes `stream_output(debounce_by=...)` / `stream_text(delta=True)` / `stream_response()` for typed slices (`result.py:76-244`).

2. **Are partial outputs trusted?**  
   Yes, but conditionally. `AgentStream.validate_response_output(allow_partial=True)` allows partial structured outputs to flow through; the final `allow_partial=False` validation is performed exactly once, on terminal `await result.get_output()` or first iteration of `stream_output()` after the final result (`result.py:245-309`). Truncated streams are persisted with `state='incomplete'` so the next model request can see the truncated context (`models/__init__.py:884-903`).

3. **Can a failed stream resume?**  
   Not within `run_stream()` for output-tool validation (`result.py:304-308` raises `Output validation failed during streaming, and retries are not supported in run_stream()`). For the model layer, HTTP-level errors are auto-retried by `AsyncTenacityTransport`/`TenacityTransport` (`retries.py:215-294`), and `ModelRetry` rebuilds a `ModelRequestNode` reusing the prior (truncated) response in history (`_agent_graph.py:1027-1040`). Application-level "resume" is achieved by passing `message_history=` to a fresh `agent.run(...)`, where the `state` field on prior `ModelResponse`s preserves the canonical cause.

4. **Can a user stop the stream safely?**  
   Yes, three layers deep. (a) `StreamedResponse.cancel()` / `AgentStream.cancel()` set `_cancelled=True` and close the underlying connection; `iterator_with_cancel_guard` swallows transport errors so iteration ends gracefully (`models/__init__.py:806-868`). (b) `StreamedRunResult.cancel()` records the truncated response as `state='interrupted'` so it appears in `all_messages()` (`result.py:759-779`). (c) External task cancellation against `agent.run_stream_events(...)` is plumbed through `CancelledError → _utils.cancel_and_drain(task)` (`agent/abstract.py:1312-1314`). Tests for all three are at `tests/test_streaming.py:4624-4955`.

5. **Does streaming weaken guardrails?**  
   No. Output validation has its own retry budget (`output_retries_used`/`max_output_retries`, `_agent_graph.py:162-179`); incomplete tool calls at token-limit raise `IncompleteToolCall` (`_agent_graph.py:146-160`); capability hooks (`wrap_model_request`, `on_model_request_error`, `before_model_request`, `after_model_request`) still fire through the `ModelRequestNode.stream()` cooperative hand-off (`_agent_graph.py:670-723`). Wrapping happens _before_ the user observes any events (`agent/abstract.py:786-790`), so capability-side inspection isn't bypassed by streaming. Streaming is purely additive to the existing model+guardrail pipeline.

## Architectural Decisions

- **Stream as part of graph, not above it.** `ModelRequestNode.stream()` is an async context manager that hands a stream back to graph drivers, with capability hooks running alongside (`_agent_graph.py:606-758`); the graph loop drives both streaming and non-streaming via the same step function pattern (`agent/abstract.py:407-444`). This makes streaming a peer to non-streaming rather than a separate execution mode.
- **Cooperative hand-off with readiness signaling.** `stream_ready` / `stream_done` events ensure the user-facing `async with node.stream(...)` waits for the capability middleware (`wrap_model_request`) to actually start streaming before yielding (`_agent_graph.py:640-692`). Outer cancellation explicitly drains both ready-waiter and wrap-task so user cleanup runs (`_agent_graph.py:684-693`).
- **Part classes as discriminated union.** `AgentStreamEvent = ModelResponseStreamEvent | HandleResponseEvent` (`messages.py:2965-2966`) and the four `ModelResponseStreamEvent` subtypes use string discriminators (`event_kind`) for both Python `match` and Pydantic JSON serialization (`messages.py:2716-2803`).
- **Partial-state surface in `ModelResponse.state`.** Three states (`complete`, `incomplete`, `interrupted`) are first-class on `ModelResponse` (`messages.py:2159-2161`) and inferred in `StreamedResponse.get()` (`models/__init__.py:884-903`) so consumers can distinguish "stream ended cleanly", "truncated", and "cancelled" without inspecting flags.
- **Wrap hook over event sink.** `wrap_run_event_stream` lets capabilities _transform_ the live event stream (including drop, replace, inject) instead of just observing it (`capabilities/abstract.py:540`; `capabilities/combined.py:335-343`). The legacy `EventStreamHandler` is a callable sink, retained for backward compatibility (`agent/__init__.py:436-441`).
- **Provider-specific cancellation.** `StreamedResponse.close_stream()` is abstract; providers override with SDK-specific teardown (`openai.py:3261`, `anthropic.py:2382`, `mistral.py:698`, `google.py:1311`, `groq.py:645`). `get_stream_cancel_errors()` defaults to `(httpx.StreamError, httpx.TransportError)` and can be overridden for non-HTTP transports (`models/__init__.py:846-855`).
- **Token-streaming ≠ library responsibility.** Pydantic AI doesn't add its own tokenization layer on top of vendors; it consumes each provider's SSE/WebSocket and translates into the unified `ModelResponseStreamEvent` set via per-model `_get_event_iterator()` (`models/openai.py:3264`, `models/anthropic.py:2173`).
- **HTTP retries separate from run retries.** `AsyncTenacityTransport`/`TenacityTransport` handle transport-level retries (`retries.py:215-294`); `ModelRetry` produces a fresh `ModelRequestNode` (`_agent_graph.py:1027-1040`); output-tool retry is explicitly unsupported in `run_stream()` (`result.py:304-308`).
- **Tool call "typed subclass on the stream".** `_parts_manager._typed_call_part` (`_parts_manager.py:86-97`) plus `ToolCallPart.narrow_type` (`messages.py:1880-1895`) promote streamed `ToolCallPart`s into `NativeToolCallPart`, `ToolSearchCallPart`, etc. as soon as the name is known, so consumers see typed parts throughout the stream — not only after a post-stream pass.

## Notable Patterns

- **Delta application pattern**: `XxxPartDelta.apply(part_or_delta)` extends both full parts (`TextPart`, `ThinkingPart`, `ToolCallPart`) and prior deltas (`messages.py:2413-2432`, `2476-2545`, `2608-2648`). This allows incremental merging across many small deltas until promotion to a full part.
- **Lifecycle stream wrapper composition**: `iterator_with_cancel_guard(iterator_with_part_end(iterator_with_final_event(self._get_event_iterator())))` (`models/__init__.py:828-830`) is the canonical streaming pipeline layered cleanly with pure functions, easy to add new wrappers without touching `_get_event_iterator`.
- **Async-context-manager-driven producer/consumer**: `agent.run_stream_events(...)` (`abstract.py:1252-1325`) builds an `anyio.create_memory_object_stream`, runs the agent in a separate task, and routes events through an `EventStreamHandler` that `await`s `send_stream.send(event)`. The consumer side observes events as they happen while the producer advances the graph.
- **Final-event-driven early termination**: `iterator_with_final_event` (`models/__init__.py:752-767`) breaks out of the inner loop after the first `FinalResultEvent`, drains remaining events, and the framework's `end_strategy` decides whether to continue executing tool calls after the final result (`agent/abstract.py:840-854`).
- **Cancel-safe iterator guard**: `iterator_with_cancel_guard` suppresses transport errors when `cancel()` already flipped `_cancelled`, ensuring natural completion still flips `_finished` (`models/__init__.py:806-826`).
- **Typed tool-call promotion**: `narrow_type` in `_parts_manager._typed_call_part` (`_parts_manager.py:86-97`) plus `ToolCallPart.narrow_type` (`messages.py:1880-1895`) provides isinstance-stable parts during streaming.
- **Testing with simulated transport errors**: `TestStreamedResponse` raises `httpx.StreamClosed` mid-iteration when `_cancelled` (`test.py:339-342`) to faithfully reproduce real-provider cancellation behavior in unit tests without flakiness.
- **Capability-driven stream wrapping**: `ProcessEventStream` capability (`capabilities/process_event_stream.py:16-63`) lets users register either an observer (`EventStreamHandler`) or a transformer (`EventStreamProcessor`) without touching `agent.py`. `CombinedCapability` chains them in reverse order (`capabilities/combined.py:335-343`).

## Tradeoffs

- **Streaming requires capability-readiness hand-off.** The `stream_ready`/`stream_done` pattern (`_agent_graph.py:640-758`) is correct but heavyweight, and an outer-cancellation bug means the framework must explicitly drain both events-or-tasks (`_agent_graph.py:684-693`, tested in `tests/test_streaming.py:4912-4955`). Adding a new side-channel into `ModelRequestNode.stream()` requires understanding this protocol.
- **Partial output validation differs from full validation.** `validate_response_output(allow_partial=True)` is best-effort: validation errors are caught and skipped, so a partial structured output might be discarded between iterations before the terminal `allow_partial=False` call (`result.py:91-93`, `245-309`). Consumers must not assume each yielded `stream_output(...)` value will pass final validation.
- **No per-delta interruption of structured output.** If the run is `run_stream()` with structured output, a mid-stream `ValidationError` cannot be retried (`result.py:304-308`). Failures bubble up on the final `get_output()` only.
- **Two coexisting event surfaces (legacy + capability).** Both `Agent._event_stream_handler` (deprecated `EventStreamHandler` slot at `agent/__init__.py:259`) and the new `wrap_run_event_stream` capability hook run the same `handle_event` semantics, leading to subtle precedence (`agent/__init__.py:436-441`). The dual surface invites confusion.
- **Cancellation granularity is per `ModelRequestNode`, not per `AgentStream`.** `AgentStream.cancel()` delegates to `StreamedResponse.cancel()`; once the node's stream is closed the framework still needs to coordinate cancellation with the `CallToolsNode` (which has its own `_run_stream` — `agent/abstract.py:881-890`). Cross-node cancellation relies on the `wrap_run_event_stream` chain finishing naturally.
- **`finish_reason='length'` mid-tool-call is fatal by default.** `check_incomplete_tool_call` raises `IncompleteToolCall` rather than guessing partial args, which is safer but requires clients to bump `max_tokens` (`_agent_graph.py:146-160`).

## Failure Modes / Edge Cases

- **Truncated text streams mid-tool-call:** `_append_response` records the truncated `ModelResponse` in history with `state='incomplete'`; subsequent `_build_retry_node` builds a `RetryPromptPart`-bearing request (`_agent_graph.py:752-754`, `828-835`, `1027-1040`). `GraphAgentState.check_incomplete_tool_call` surfaces `IncompleteToolCall` only when `max_output_retries` is exhausted and `finish_reason='length'` (`_agent_graph.py:146-177`).
- **Provider-side mid-stream errors:** `test_streaming_errors.py` covers status errors, connection errors, peek errors, and non-HTTP errors per provider (Anthropic, OpenAI, OpenAI Responses, Groq, Bedrock, HuggingFace, Mistral, xAI). `FallbackModel` is exercised via `test_fallback_model_streaming_error_triggers_fallback` (`tests/test_streaming_errors.py:937`).
- **Cancel-during-readiness bug:** Outer `CancelledError` while awaiting `asyncio.wait({ready_waiter, wrap_task})` is handled by `cancel_and_drain(ready_waiter, wrap_task)` so the user's `wrap_model_request` `finally` clause runs (`_agent_graph.py:684-693`, `tests/test_streaming.py:4912-4955`).
- **Cancel-during-iteration bug:** `iterator_with_cancel_guard` must suppress `httpx.StreamClosed`; tests explicitly check `state='interrupted'` and verify the interrupted response is included in `all_messages()` (`tests/test_streaming.py:4624-4714`).
- **Cancellation via `break`/`aclose()` does not flip `_finished`:** Docs and code explicitly note that early termination leaves `_finished=False` so `StreamedResponse.get()` reports `state='incomplete'` unless `cancel()` was called (`models/__init__.py:819-826`).
- **Iterating `AgentEventStream` standalone without `async with`** logs a `DeprecationWarning` and routes through a `_standalone_iterator` that still calls `aclose()` on break so cleanup is preserved (`result.py:1013-1033`).
- **`BaseException`-on-consumer-exit:** `_run_stream_events` cancels and drains the producer task without replacing the in-flight exception (`agent/abstract.py:1316-1321`), so the original error bubbles up untouched.
- **Stream-output throws mid-iteration:** `stream_output(...)` swallows `ValidationError`/`ModelRetry` during partial validation but re-raises on the terminal `allow_partial=False` call (`result.py:91-93`, `304-309`).
- **Retry-after infrastructure:** `wait_retry_after` handles both integer-seconds and HTTP-date `Retry-After` headers, with `max_wait=300s` ceiling (default) and exponential fallback (`retries.py:312-381`). Per-provider transport wrapping is opt-in.

## Future Considerations

- **Unify legacy `_event_stream_handler` slot with `wrap_run_event_stream` capability.** The current dual-surface approach (legacy callback vs. capability hook at `agent/__init__.py:436-441`) is deprecated but not gone; a v2 cleanup could remove the slot entirely.
- **Extend `run_stream()` output-tool retry support.** Currently partial structured outputs cannot trigger a retry path (`result.py:304-308`); a future redesign could allow `_build_retry_node`-style continuation from truncated structured responses without re-running tool calls.
- **Standardize per-provider `close_stream()` adoption.** A handful of providers override `close_stream()` correctly; gRPC/botocore transports may need to override `get_stream_cancel_errors()` instead (`models/__init__.py:846-855`). A typed `StreamCancelError` protocol would let providers contribute typed errors per transport.
- **Telemetry span around streaming iteration.** `StreamedResponse` doesn't expose hooks for OTel spans around iteration lifecycle (only `current_otel_traceparent()` at `run.py:114`). With growing cancel/error coverage, a `span_iteration_start/iteration_end` API would help consumers observe streams.
- **Detached streaming / SSE reconnection.** `UIEventStream` provides SSE encoding (`ui/_event_stream.py:139-153`), but no reconnect logic for mid-stream client drops; a future addition could buffer events server-side and re-attach clients after disconnect.

## Questions / Gaps

- **WebSocket transports / non-HTTP providers.** `get_stream_cancel_errors()` defaults to `httpx.StreamError`/`TransportError` (`models/__init__.py:846-855`); no concrete examples in the code for gRPC cancel behavior on streaming, only the docstring note.
- **Structured-output retries inside `run_stream()`.** With retries explicitly disabled (`result.py:304-308`), how should downstream consumers detect a recoverable partial structured failure? Docstring suggests passing a fresh `message_history=` manually — but no concrete example in this study boundary.
- **Cross-process durability of streaming state.** The Temporal integration (`durable_exec/temporal/_model.py`) lifts `event_stream_handler` to an activity (`_model.py:100-124`), but the streamed events themselves are not checkpointed — only the final `ModelResponse` is via `streamed_response.get()`. Resumability across restarts would need either event replay or final-result tolerance.
- **`ToolCallPart.narrow_type` for unrecognized tool kinds.** `_typed_call_part` returns the base `ToolCallPart` if `kind` is `None` (`_parts_manager.py:93-96`); whether unknown-but-valid tool types should fall back gracefully vs. raise is not explicit.
- **No clear evidence found** for stream-credit/exponential backoff within `AsyncTenacityTransport` applied to streaming responses specifically; the available evidence is HTTP-level and pre-stream.

---

Generated by `01.08-streaming-execution-semantics` against `pydantic-ai`.

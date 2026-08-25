# Source Analysis: crewai

## 01.08: Streaming Execution Semantics

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (3.10+), with provider adapters for OpenAI, Anthropic, Gemini, Azure OpenAI, Bedrock, LiteLLM (any provider), and an A2A networking stack |
| Analyzed | 2026-07-03 |

## Summary

CrewAI implements streaming as a **per-token / per-tool-delta event protocol** wrapped by an iterator façade, *not* as a streaming protocol of the conversation itself. Every LLM provider emits `LLMStreamChunkEvent` (or `LLMThinkingChunkEvent`) on the singleton event bus (`crewai_event_bus`) whenever the upstream SDK yields a delta (`lib/crewai/src/crewai/llms/providers/openai/completion.py:1987-2031`, `lib/crewai/src/crewai/llms/providers/anthropic/completion.py:1084-1143`, `lib/crewai/src/crewai/llms/providers/gemini/completion.py:1001-1026`, `lib/crewai/src/crewai/llms/base_llm.py:595-625`). A consumer-side `stream_handler` registered when `crew.stream=True` (`lib/crewai/src/crewai/crews/utils.py:47` toggles each agent's `llm.stream = True`) pushes each chunk onto a `queue.Queue` or `asyncio.Queue` (`lib/crewai/src/crewai/utilities/streaming.py:130-144`, `188-226`) and a generator (`create_chunk_generator` / `create_async_chunk_generator` at `utilities/streaming.py:258-345`) drains that queue into a `CrewStreamingOutput` / `FlowStreamingOutput` (`lib/crewai/src/crewai/types/streaming.py:240-351`).

The implementation is explicit and tested but **stream-id scope is one-shot per crew/flow kickoff, partial output is not rolled back on failure, there is no in-band resume of a broken stream, and `LiteAgent` has no streaming path at all** (verified by `grep`-ing `lib/crewai/src/crewai/lite_agent.py`). Stream chunks are *not* persisted by default; checkpointing is opt-in via `CheckpointConfig(events=...)` and the type list does include `llm_stream_chunk` (`lib/crewai/src/crewai/state/checkpoint_config.py:53`) and `a2a_streaming_chunk` (`lib/crewai/src/crewai/state/checkpoint_config.py:114-115`). Cancellation is supported via `StreamingOutputBase.aclose()` / `close()` (`lib/crewai/src/crewai/types/streaming.py:155-185`), which sets `is_cancelled`, propagates to the underlying iterator and unregisters the event handler; this is exercised by `TestStreamingCancellation` (`lib/crewai/tests/test_streaming.py:710-860`) and the concurrent-stream regression test (`lib/crewai/tests/test_streaming.py:882-967`).

> Does streaming give responsiveness without corrupting state? **Partially.** The model is clear and the synchronous event-bus dispatch prevents reordering (`events/event_bus.py:509-510,627-628`), but the framework treats chunks as fire-and-forget: no rollback, no resumption, no transactional guarantee that every chunk corresponds to a finalised tool call, and `LiteAgent` users get nothing.

## Rating

**6/10** — Clear model (typed `StreamChunk` + `StreamChunkType`, dedicated `CrewStreamingOutput` / `FlowStreamingOutput`, contextvar-scoped isolation that is regression-tested) with a few real safeguards (synchronous handler dispatch on `LLMStreamChunkEvent` to preserve ordering, idempotent close/aclose, stream_id ContextVar for concurrent crews). Below "good" because: (1) `current_task_info` is set once at `StreamingContext.__init__` and never refreshed (`lib/crewai/src/crewai/crews/utils.py:381-388`), so chunks from task N carry task-0 metadata — the per-task context advertised by `StreamChunk.task_index` / `task_name` is broken; (2) `LiteAgent.kickoff` and `Agent.kickoff` have no streaming wrapper, so the lightest-weight execution path is missing; (3) partial output is *not* rolled back on stream failure — what was emitted stays emitted; (4) there is no automatic resume of a broken stream (the only resubscribe logic lives in the A2A handler, `lib/crewai/src/crewai/a2a/updates/streaming/handler.py:57-149`); (5) guardrails and `before_llm_call` / `after_llm_call` hooks run only on the final response, not on streamed chunks (`lib/crewai/src/crewai/llms/providers/openai/completion.py:2044-2048`, `2044` returns the final string into the hook), so per-chunk guardrail enforcement is impossible today.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Stream-chunk Pydantic model | `StreamChunk(content, chunk_type, task_index, task_name, task_id, agent_role, agent_id, tool_call)`; `__str__` returns content | `lib/crewai/src/crewai/types/streaming.py:43-72` |
| Stream chunk enum | `StreamChunkType.TEXT` / `StreamChunkType.TOOL_CALL` | `lib/crewai/src/crewai/types/streaming.py:20-24` |
| Tool-call partial chunk type | `ToolCallChunk(tool_id, tool_name, arguments: str, index)` — note `arguments` is a raw JSON *string*, not a parsed object, signalling partial-byte accumulation | `lib/crewai/src/crewai/types/streaming.py:27-40` |
| Streaming output base (sync/async iter, close/aclose) | `StreamingOutputBase[T]` with `__iter__`, `_async_iterate`, `__aenter__/__aexit__`, idempotent `close()` / `aclose()`, `result`, `is_completed`, `is_cancelled`, `chunks`, `get_full_text()` | `lib/crewai/src/crewai/types/streaming.py:75-237` |
| Crew streaming output (single + for-each) | `CrewStreamingOutput` adds `.results` for `kickoff_for_each_async`; `_set_result` / `_set_results` setters | `lib/crewai/src/crewai/types/streaming.py:240-319` |
| Flow streaming output | `FlowStreamingOutput(StreamingOutputBase[Any])` | `lib/crewai/src/crewai/types/streaming.py:322-351` |
| Per-provider event emission | `_emit_stream_chunk_event` builds `LLMStreamChunkEvent(chunk, tool_call, from_task, from_agent, call_type, response_id, call_id)` and emits synchronously on the bus | `lib/crewai/src/crewai/llms/base_llm.py:595-625` |
| Thinking-chunk variant | `LLMThinkingChunkEvent(chunk, response_id, …)` emitted by providers with reasoning support (e.g. Anthropic extended thinking via `_emit_thinking_chunk_event`) | `lib/crewai/src/crewai/llms/base_llm.py:627-651` |
| Event baseclass sets agent/task metadata on the chunk | `LLMEventBase.__init__` writes `agent_id`, `agent_role`, `task_id`, `task_name` from the source `from_task`/`from_agent` | `lib/crewai/src/crewai/events/types/llm_events.py:9-28` |
| OpenAI text-delta chunk emission | `if chunk_delta.content: self._emit_stream_chunk_event(chunk=chunk_delta.content, …)` | `lib/crewai/src/crewai/llms/providers/openai/completion.py:1985-1992` |
| OpenAI tool-call partial accumulation | delta-by-delta: opens `tool_calls[index]` on first hit, then accumulates `arguments` strings, emitting a chunk for *every* delta with the current accumulated state | `lib/crewai/src/crewai/llms/providers/openai/completion.py:1994-2031` |
| OpenAI async equivalent | identical accumulation pattern | `lib/crewai/src/crewai/llms/providers/openai/completion.py:2329-2366` |
| OpenAI Responses API streaming (text only via emit) | emits `response.output_text.delta`; tool-call deltas are explicitly swallowed (`pass`) and only complete tool calls are pulled from `response.output_item.done` | `lib/crewai/src/crewai/llms/providers/openai/completion.py:1115-1137, 1252-1274` |
| Anthropic tool-use block tracking | `content_block_start` (tool_use) creates entry; `content_block_delta` (input_json_delta) appends `partial_json` to `arguments` and emits a chunk each time | `lib/crewai/src/crewai/llms/providers/anthropic/completion.py:1094-1143` |
| Anthropic async equivalent | mirrors the sync accumulation | `lib/crewai/src/crewai/llms/providers/anthropic/completion.py:1620-1662` |
| Gemini streaming emit | `delta.text` → chunk; `function_call.args` partials tracked and emitted | `lib/crewai/src/crewai/llms/providers/gemini/completion.py:1001-1026` |
| Azure streaming (uses OpenAI base) | delegates to the same `OpenAICompletion._emit_stream_chunk_event` | `lib/crewai/src/crewai/llms/providers/azure/completion.py:975-998` |
| Bedrock streaming | emits chunk per delta and per accumulated tool-args | `lib/crewai/src/crewai/llms/providers/bedrock/completion.py:980-1017, 1577-1614` |
| LiteLLM streaming path | `_handle_streaming_response` builds a `LLMStreamChunkEvent` per text/tool delta, sets `include_usage=True` so usage comes through the *final* `usage` chunk | `lib/crewai/src/crewai/llm.py:760-1086` |
| Event-bus ordering guarantee for stream chunks | sync handlers for `LLMStreamChunkEvent` are invoked on the emitting thread (no thread pool, no async hop) — explicitly to preserve ordering | `lib/crewai/src/crewai/events/event_bus.py:508-510, 626-628` |
| Per-crew context | `StreamingContext(result_holder, current_task_info, state, output_holder)` builds a `StreamingState` with a UUID `stream_id` | `lib/crewai/src/crewai/crews/utils.py:372-392` |
| Per-flow context | identical pattern, inlined in `kickoff`/`kickoff_async` | `lib/crewai/src/crewai/flow/runtime/__init__.py:2148-2188, 2248-2290` |
| Agent-level opt-in | `enable_agent_streaming(agents)` flips `agent.llm.stream = True` | `lib/crewai/src/crewai/crews/utils.py:47-55` |
| Stream handler push to queue | filters by `stream_id in _current_stream_ids` to avoid cross-stream contamination; calls `loop.call_soon_threadsafe(async_queue.put_nowait, chunk)` for async or `sync_queue.put(chunk)` otherwise | `lib/crewai/src/crewai/utilities/streaming.py:110-144` |
| Concurrent stream isolation | `_current_stream_ids: ContextVar[tuple[str, …]]` plus `stream_id` UUID; covered by regression test | `lib/crewai/src/crewai/utilities/streaming.py:29-31, 134, 273-280`; `lib/crewai/tests/test_streaming.py:882-967` |
| Stream registration | `create_streaming_state` registers the handler on the bus and returns the `StreamingState` | `lib/crewai/src/crewai/utilities/streaming.py:188-226` |
| Stream teardown | `_unregister_handler` removes the handler from `_sync_handlers[LLMStreamChunkEvent]` under a write lock | `lib/crewai/src/crewai/utilities/streaming.py:147-157` |
| Result finalisation | `_finalize_streaming` unsubscribes the handler and writes `state.result_holder[0]` onto the streaming output | `lib/crewai/src/crewai/utilities/streaming.py:160-173` |
| Sync chunk generator | spawns a daemon thread, propagates `_current_stream_ids` via `ContextVar.copy_context()` to isolate the LLM producer from the consumer | `lib/crewai/src/crewai/utilities/streaming.py:258-295` |
| Async chunk generator | spawns `asyncio.create_task`; cancellation on cleanup calls `task.cancel()` and awaits it with `CancelledError` swallowed | `lib/crewai/src/crewai/utilities/streaming.py:298-345` |
| Error signalling on the stream | `signal_error(state, exc[, is_async])` pushes the exception onto the queue; the generator re-raises it (`raise item`) | `lib/crewai/src/crewai/utilities/streaming.py:242-255, 287-289, 330-332` |
| Crew kickoff streaming path (sync) | `if self.stream:` calls `enable_agent_streaming(self.agents)`, creates `StreamingContext`, defines `run_crew` that flips `self.stream=False`, calls `self.kickoff(...)`, stores `CrewOutput` in `result_holder`, then on completion calls `signal_end`. Returns a `CrewStreamingOutput` wrapping `create_chunk_generator`. | `lib/crewai/src/crewai/crew.py:1002-1026` |
| Crew kickoff streaming path (async) | same pattern via `create_async_chunk_generator`, kickoff runs inside `asyncio.to_thread` | `lib/crewai/src/crewai/crew.py:1136-1160` |
| Crew akickoff streaming | identical to async wrapper but awaits `self.akickoff` instead of `asyncio.to_thread` | `lib/crewai/src/crewai/crew.py:1214-1238` |
| for-each streaming (async, fused) | `run_for_each_async` creates `ForEachStreamingContext`, runs each crew copy with `stream=True`, consumes each per-crew `CrewStreamingOutput`, multiplexes chunks onto a single outer output, and stores all final results via `streaming_output._set_results(...)` | `lib/crewai/src/crewai/crews/utils.py:395-488` |
| Flow kickoff streaming (sync + async) | mirrors crew pattern, uses `FlowStreamingOutput` | `lib/crewai/src/crewai/flow/runtime/__init__.py:2148-2188, 2248-2290` |
| Stream chunk cancellation | `StreamingOutputBase.close()`/`aclose()` set `_cancelled`/`_completed`, close the iterator, invoke `_on_cleanup` which unregisters the bus handler | `lib/crewai/src/crewai/types/streaming.py:155-185` |
| Cancellation tests | `TestStreamingCancellation.test_aclose_cancels_async_streaming`, `test_close_cancels_sync_streaming`, idempotency, async-context-manager | `lib/crewai/tests/test_streaming.py:710-859` |
| `current_task_info` is set once, never refreshed | `StreamingContext.__init__` writes empty task info; nothing in `utilities/streaming.py` updates `ctx.current_task_info` between tasks — meaning `StreamChunk.task_index`/`task_name`/`task_id` are zero/empty unless overridden by event-side metadata | `lib/crewai/src/crewai/crews/utils.py:381-391` |
| Stream-level metadata comes from the event | `agent_role` / `agent_id` are populated by `LLMEventBase.__init__` and back-fill `current_task_info["agent_role"]` if missing — but `task_index`/`task_name`/`task_id` are NOT similarly back-filled | `lib/crewai/src/crewai/utilities/streaming.py:98-107`, `lib/crewai/src/crewai/events/types/llm_events.py:9-28` |
| LiteAgent does NOT support streaming | `lite_agent.py` has no `enable_agent_streaming` call and no `StreamingContext`/`CrewStreamingOutput`; `kickoff` returns a `LiteAgentOutput` directly | `lib/crewai/src/crewai/lite_agent.py:477-546` |
| Agent (Crew's old `Agent`) has no streaming kickoff | same — `Agent.kickoff` returns `LiteAgentOutput` | `lib/crewai/src/crewai/agent/core.py:1568-1690` |
| Memory analysis LLM forced non-streaming | `unified_memory._non_streaming_analysis_llm` sets `analysis_llm.stream = False`; used for memory extraction only | `lib/crewai/src/crewai/memory/unified_memory.py:58-72, 222-226` |
| OpenAI SDK-level connection retries (provider-level, not stream-level) | `OpenAICompletion.max_retries: int = 2` passed through to the OpenAI SDK | `lib/crewai/src/crewai/llms/providers/openai/completion.py:209` |
| OpenAI non-streaming fallback | if zero chunks arrive, `_handle_streaming_response` re-invokes `_handle_non_streaming_response` with `stream=False` | `lib/crewai/src/crewai/llms/providers/openai/completion.py:890-903` |
| A2A streaming has explicit resubscribe | `StreamingHandler._try_recover_from_interruption` calls `client.get_task(...)` and `client.resubscribe(TaskIdParams(id=task_id))` up to `MAX_RESUBSCRIBE_ATTEMPTS=3` | `lib/crewai/src/crewai/a2a/updates/streaming/handler.py:49-50, 57-149` |
| A2A stream chunk event type | `A2AStreamingChunkEvent(task_id, context_id, chunk, chunk_index, final, …)` plus `A2AStreamingStartedEvent` | `lib/crewai/src/crewai/events/types/a2a_events.py:410-449` |
| Checkpoint event-type list includes stream events | `"llm_stream_chunk"`, `"a2a_streaming_started"`, `"a2a_streaming_chunk"` are valid `CheckpointEventType` values | `lib/crewai/src/crewai/state/checkpoint_config.py:53, 114-115` |
| Stream chunks NOT persisted by default | no built-in handler in `_do_checkpoint` triggers on stream events; persistence requires the user to attach a `CheckpointConfig(events=[...])` | `lib/crewai/src/crewai/state/checkpoint_listener.py:113-194` |
| Guardrails / hooks run on final string only | `_invoke_after_llm_call_hooks` is called on the assembled `full_response` after the stream completes, not per chunk | `lib/crewai/src/crewai/llms/providers/openai/completion.py:2044-2048` (sync), `lib/crewai/src/crewai/llms/providers/anthropic/completion.py:1226-1228` |
| Console formatter streaming UI | `ConsoleFormatter.handle_llm_stream_chunk` updates a Rich `Live` panel | `lib/crewai/src/crewai/events/utils/console_formatter.py:530-602` |

## Answers to Dimension Questions

1. **What is emitted while execution is still running?**
   - `LLMStreamChunkEvent` (text or tool-call partials) for every provider delta, and `LLMThinkingChunkEvent` for reasoning chunks (`lib/crewai/src/crewai/events/types/llm_events.py:136-151`). The bus dispatches these synchronously to preserve order (`lib/crewai/src/crewai/events/event_bus.py:509-510,627-628`).
   - These are converted to `StreamChunk` Pydantic objects (`lib/crewai/src/crewai/utilities/streaming.py:83-107`) and drained from a `queue.Queue`/`asyncio.Queue` by `create_chunk_generator` / `create_async_chunk_generator` (`utilities/streaming.py:258-345`).
   - A2A runs a separate stream layer: `A2AStreamingStartedEvent` and `A2AStreamingChunkEvent` are emitted by the `StreamingHandler` (`lib/crewai/src/crewai/a2a/updates/streaming/handler.py:243-340`) and consumed via the A2A chunk list, not via `CrewStreamingOutput`.

2. **Are partial outputs trusted?**
   - Partial text is trusted at face value but the *full text* is only rebuilt after the loop ends: `get_full_text()` filters `StreamChunkType.TEXT` chunks (`lib/crewai/src/crewai/types/streaming.py:135-145`).
   - Tool-call output is **not trusted** until the entire JSON `arguments` string is assembled; intermediate states are emitted as `StreamChunkType.TOOL_CALL` chunks (`utilities/streaming.py:56-80`), but execution only happens in `_finalize_streaming_response` / `_execute_first_tool` *after* the loop finishes (`openai/completion.py:2033-2048`, `anthropic/completion.py:1145-1228`).
   - Guardrails (`before_llm_call` / `after_llm_call`) only see the final assembled string (`openai/completion.py:2044-2048`); per-chunk guardrail enforcement is impossible in the current implementation.

3. **Can a failed stream resume?**
   - **No.** There is no in-band resume of a partial text/tool stream. Once the generator raises, the caller must restart the kickoff from scratch. The OpenAI Responses API has `previous_response_id` (`lib/crewai/src/crewai/llms/providers/openai/completion.py:694-698`) but the streaming code path does not pass it through in a way that re-enters mid-response.
   - The only resume path is the **A2A** `StreamingHandler._try_recover_from_interruption`, which calls `client.get_task(...)` and `client.resubscribe(...)` up to three times with exponential backoff (`lib/crewai/src/crewai/a2a/updates/streaming/handler.py:49-50, 57-149`). That is a *transport-level* resume, not a model-stream resume.
   - Provider-level retries: `OpenAICompletion.max_retries = 2` is handed to the OpenAI SDK for connection retries (`openai/completion.py:209`). OpenAI also falls back to non-streaming when zero chunks arrive (`openai/completion.py:890-903`).

4. **Can a user stop the stream safely?**
   - **Yes**, both sync and async. `StreamingOutputBase.close()` / `aclose()` are idempotent, set `is_cancelled` / `is_completed`, close the underlying iterator, and call `_on_cleanup` which unregisters the bus handler (`lib/crewai/src/crewai/types/streaming.py:155-185`, `utilities/streaming.py:176-186`).
   - The async cleanup also cancels the background `run_crew` task with `task.cancel()` and swallows `CancelledError` (`utilities/streaming.py:333-341`).
   - The async context manager exits via `aclose()` (`types/streaming.py:147-153`), so `async with streaming:` on early `break` still unregisters the handler.
   - Concurrency regression test (`lib/crewai/tests/test_streaming.py:882-967`) plus `TestStreamingCancellation` (`test_streaming.py:710-859`) cover happy and adversarial paths.

5. **Does streaming weaken guardrails?**
   - **Yes, materially.** Per-chunk guardrails do not exist: `before_llm_call` and `after_llm_call` hooks run on the final assembled string only (`openai/completion.py:2044-2048`, `anthropic/completion.py:1226-1228`). Content the model emits token-by-token is delivered to the consumer before any guardrail validates it.
   - Task-level guardrails still run, because they wrap `agent_executor.execute_core`, which is called after the LLM call completes — i.e. after streaming is finalised (`lib/crewai/src/crewai/task.py:1258-1316, 1368-1426`).
   - Structured-output parsing happens *during* the stream only when `response_model` is provided (e.g. `openai/completion.py:1908-1953`, `2226-2293`); on failure the partial JSON is discarded and the raw text is returned, weakening the guardrail guarantee for partial JSON.

## Architectural Decisions

- **Event-bus mediated streaming** instead of callbacks per provider. `_emit_stream_chunk_event` is the single chokepoint (`llms/base_llm.py:595-625`); all providers (OpenAI, Anthropic, Gemini, Bedrock, LiteLLM, Azure) emit the same `LLMStreamChunkEvent` Pydantic model. Trade-off: ordering is preserved by synchronous dispatch (`events/event_bus.py:509-510`), but per-chunk user callbacks are not part of the public API — the user-facing API is the iterator on `CrewStreamingOutput` / `FlowStreamingOutput`.
- **Per-crew ContextVar-scoped isolation.** `_current_stream_ids: ContextVar[tuple[str, …]]` plus a UUID `stream_id` per `StreamingState` (`utilities/streaming.py:29-31, 211, 273-280`) prevents cross-contamination when multiple crews run concurrently. The producer thread is wrapped via `contextvars.copy_context()` so `stream_id` propagates into the LLM-emitting thread.
- **Background-thread kickoff.** `create_chunk_generator` spawns a daemon `threading.Thread` (or `asyncio.create_task` for async) that flips `self.stream=False` and re-invokes `self.kickoff(...)` so the streaming wrapper is not recursive. (`utilities/streaming.py:279-280`, `crew.py:1006-1017, 1140-1150`).
- **`StreamingState` is immutable** (a `NamedTuple`); the only mutable piece is `result_holder: list[Any]`. The `StreamingContext.current_task_info` is *not* refreshed between tasks, so the per-task metadata on streamed chunks is effectively frozen at task-0 (`crews/utils.py:381-391`).
- **Iterator façade encapsulates lifecycle.** `StreamingOutputBase.__iter__` / `__aiter__` set `is_completed` in `finally` (`types/streaming.py:198-208, 228-237`), so the user doesn't need to call `close()` on normal exhaustion. Idempotency of `close()`/`aclose()` makes double-cleanup safe.
- **Two streaming layers, two protocols.** LLM streaming (`LLMStreamChunkEvent` → `CrewStreamingOutput`) is for the model. A2A streaming (`A2AStreamingStartedEvent` / `A2AStreamingChunkEvent` → `StreamingHandler` with resubscribe) is for agent-to-agent over SSE (`lib/crewai/src/crewai/a2a/updates/streaming/handler.py`). They do not share an iterator type.

## Notable Patterns

- **Streaming output is a context manager and an async context manager** (`types/streaming.py:147-153, 209-237`), so `with`/`async with` cleanly tears down handlers.
- **Concurrent-stream isolation is regression-tested** (`tests/test_streaming.py:882-967`) with a note referencing issue #5376.
- **Cancellation is idempotent and observable** via `is_cancelled` / `is_completed` properties (`types/streaming.py:120-128`).
- **`ToolCallChunk.arguments` is the raw JSON string** of accumulated deltas (`types/streaming.py:39`), so consumers know to validate JSON before trusting it. Some providers (OpenAI Responses API) explicitly *suppress* per-delta function-call deltas and only emit completed tool calls (`openai/completion.py:1125-1137`) — partial state is not exposed at all for that path.
- **Stream-chunk event names are listed in the checkpoint event-type union** (`state/checkpoint_config.py:53, 114-115`), so a user can opt in to persist stream chunks via `CheckpointConfig(events=["llm_stream_chunk"])`. Default persistence is off.
- **Backwards recursion guard**: each streaming kickoff flips `self.stream = False` before re-entering (`crew.py:1009, 1142, 1220`) and restores it in `finally`, so nested kickoffs do not double-wrap.
- **Tool-call name/id back-fill**: when only the first chunk of a tool call carries the name/id (OpenAI), subsequent deltas reuse the same `tool_index` dict entry (`openai/completion.py:1994-2031`).
- **OpenAI Responses API emits text deltas but suppresses tool-argument deltas** — the streaming wire for that API is text-only, with tool calls delivered whole via `response.output_item.done` (`openai/completion.py:1115-1137`).

## Tradeoffs

- **Synchronous event-bus dispatch for stream chunks** preserves ordering (`events/event_bus.py:509-510,627-628`) at the cost of throughput: if a user installs a slow custom handler on `LLMStreamChunkEvent`, it blocks the LLM thread. The system makes the same trade for `LLMCallFailedEvent` to keep error reporting in order.
- **Partial output is *append-only*, not transactional.** Whatever chunks the consumer has already rendered are not rolled back if the stream later errors (`utilities/streaming.py:287-289` simply raises `item`, leaving the consumer's `_chunks` intact — see `types/streaming.py:198-205`). Compare with agent-framework's documented "append-and-stop" choice (`reports/source/01.08-streaming-execution-semantics/agent-framework.md:22-28`).
- **`current_task_info` is set once and never updated.** `StreamingContext.__init__` writes empty placeholders (`crews/utils.py:381-388`), and nothing else writes to it during a multi-task kickoff. The advertised `chunk.task_index` / `chunk.task_name` / `chunk.task_id` on a multi-task crew will be `0`/`""`/`""` for every chunk unless the chunk's own `LLMStreamChunkEvent.from_task` is non-null, which the OpenAI path does pass — but the OpenAI provider emits `task_id` / `task_name` *only on the first event of the call*, not per chunk (`llm_events.py:16-21`). This is a real correctness gap; in practice consumers get the *current* task's metadata only on chunks where the provider was kind enough to send `from_task`.
- **No LiteAgent/Agent streaming.** `lite_agent.py` and `agent/core.py` return `LiteAgentOutput` directly; only `Crew` and `Flow` have `stream=True`. This is the lightest-weight execution path — and it does not support streaming at all (`lib/crewai/src/crewai/lite_agent.py:477-546`, `lib/crewai/src/crewai/agent/core.py:1568-1690`).
- **Tool-call partial bytes are exposed to consumers** (`StreamChunk.tool_call.arguments` is the accumulated string). Providers differ on whether they emit deltas at all: OpenAI Responses API chooses not to (`openai/completion.py:1125-1137`), Anthropic does (`anthropic/completion.py:1094-1143`).
- **One event type, two layers.** A2A streaming is implemented as a parallel stream of `A2AStreamingChunkEvent`s consumed by `StreamingHandler` — not via `CrewStreamingOutput` — so the user-facing API for agent-to-agent streams is bespoke.
- **Memory/LLM calls for memory analysis are forced non-streaming** (`unified_memory._non_streaming_analysis_llm`) so they don't leak into the user-visible stream. This is a small correctness choice that protects the user-facing model.

## Failure Modes / Edge Cases

- **Mid-stream exception**: `signal_error` puts the exception on the queue; the generator re-raises (`utilities/streaming.py:242-255, 287-289`). Background `run_crew` exception is caught and pushed via `signal_error`; async cleanup cancels the task and swallows `CancelledError` only (`utilities/streaming.py:333-341`). Other exceptions raised by the background task are swallowed silently after `signal_error` is signalled (`utilities/streaming.py:340-341` — `logger.debug("Background streaming task failed", exc_info=True)`).
- **`result_holder` empty on error**: if the kickoff raises before writing to `result_holder`, `_finalize_streaming` writes `state.result_holder[0]` (None) into `_set_result` — but the `_completed=True` is set, and `result` raises `RuntimeError("No result available")` (`types/streaming.py:99-118`). So callers who iterate to completion and then read `.result` will not silently get a stale value.
- **Streaming with zero chunks**: OpenAI provider falls back to non-streaming (`openai/completion.py:890-903`); LiteLLM warns and falls back to non-streaming when `chunk_count == 0` (`llm.py:890-903`). No chunks are emitted to the consumer in this case, so the iterator receives an empty stream and `result` is set from the non-streaming call.
- **Structured-output stream failures**: when `response_model` validation fails mid-stream, providers fall back to returning the raw text and warn (`openai/completion.py:2280-2292`, `anthropic/completion.py:1166-1194`). Consumer sees the raw text rather than a structured object — guardrail guarantee weakened.
- **Concurrent cancellation races**: `_unregister_handler` and `_register_handler` both take `crewai_event_bus._rwlock.w_locked()` (`utilities/streaming.py:147-157`), so concurrent register/unregister is safe; the iteration consumer may still see one extra chunk after `aclose()` returns due to in-flight queue items, but `_cancelled=True` is set and `aclose` is idempotent.
- **HumanFeedbackPending exception in Flow streaming**: this is *not* an error; `run_flow` catches it and appends to `result_holder` (`flow/runtime/__init__.py:2173-2176, 2274-2277`). Good — `HumanFeedbackPending` is control flow, not a failure.
- **`is_completed` set even on `aclose()` after exhaustion**: `aclose` is a no-op once `is_completed`/`is_cancelled`/`_exhausted` is set (`types/streaming.py:161-162`), so calling `aclose` on a stream that completed normally is safe and does not double-clean.
- **`stream_id` reuse after context reset**: the producer thread copies contextvars (`utilities/streaming.py:274-276, 319-321`), so a `stream_id` set on the consumer's context is not visible to handlers invoked from outside the kickoff. A custom handler installed *before* kickoff will receive ALL stream chunks regardless of stream_id — the filter only excludes chunks for streams whose stream_id is in the current context.

## Future Considerations

- **Refresh `current_task_info` per task.** `StreamingContext` should expose `set_task_info(index, name, id, agent_role, agent_id)` and `_create_stream_chunk` should read it via a mutable container (or pass it through the `from_task`/`from_agent` objects the way OpenAI already does). Without this, `chunk.task_index` is unreliable across tasks.
- **Native async streaming for LiteAgent/Agent.** `Agent.kickoff` and `LiteAgent.kickoff` should accept a `stream=True` flag and return a streaming output that mirrors `CrewStreamingOutput`. The infrastructure (`create_streaming_state`, the queue) is already provider-agnostic.
- **In-band stream resumption.** Either through a provider-issued `response_id` (OpenAI Responses API already has `previous_response_id`, `openai/completion.py:694-698`) or by caching partial tool-call state into the `CheckpointConfig` events list. Currently the only resume path lives in A2A.
- **Per-chunk guardrails.** Add a `StreamingGuardrailConfig` that runs the guardrail on the rolling `get_full_text()` window (e.g. every N tokens or every M seconds), with the option to call `signal_error` and tear down the stream.
- **Per-chunk usage accounting.** Today usage is only available on `LLMCallCompletedEvent` (`llm.py:977`), so streaming consumers cannot show token-by-token cost. Some providers (LiteLLM with `stream_options.include_usage`) emit a final usage-only chunk (`llm.py:832-835`) that could be promoted into the stream.
- **Hook a `persistent` flag on `StreamChunk`** so callers can choose to persist chunks via the existing `CheckpointConfig(events=["llm_stream_chunk"])` mechanism without having to wire their own listener.
- **Replace `_unregister_handler`'s direct frozenset mutation** with a public `off(event_type, handler)` to stop reaching into `crewai_event_bus._sync_handlers` directly (`utilities/streaming.py:147-157`). The current code relies on a private attribute that could break with future bus refactors.
- **Distinguish "stream timeout" from "stream error"** in `signal_error`. Today both raise the same Exception to the consumer (`utilities/streaming.py:242-255`), so retry policies are coarse.

## Questions / Gaps

- **Per-chunk ordering across providers**: OpenAI Responses API emits text deltas but suppresses tool-argument deltas (`openai/completion.py:1125-1137`), so a stream can contain a `tool_call` chunk followed by *no further `arguments` delta chunks*. The current `_finalize_streaming_response` still works because it pulls the final `tool_calls` from the last chunk (`openai/completion.py:2033-2043`), but it depends on the provider-specific finish behaviour. No evidence found in this codebase that this is documented for callers.
- **Streaming inside hierarchical / async fan-out crews**: `_run_hierarchical_process` and `_arun_sequential_process` are not in this study's read; it's unclear whether the manager agent's LLM calls in hierarchical mode also emit `LLMStreamChunkEvent` to the consumer's queue. No clear evidence found.
- **`StreamChunk.task_index` semantics**: how is this expected to evolve? It is set from `current_task_info["index"]` which is never updated, so the documented behaviour (`docs/edge/en/learn/streaming-crew-execution.mdx:67-83`) — "Task: chunk.task_name (index chunk.task_index)" — is broken in practice for multi-task crews. Either the docs or the code is wrong.
- **Resume semantics after `from_checkpoint`**: `apply_checkpoint(self, from_checkpoint)` (`state/checkpoint_config.py:214-232`) restores a previous Crew but does not mention streaming. If the previous run was streaming and was killed mid-task, the resume restarts the task from scratch. No evidence found of replayed stream chunks.
- **`flow_kickoff_for_each_async`**: not in the read window; unclear whether it has a streaming wrapper analogous to `crew.kickoff_for_each_async`. No clear evidence found.
- **Telemetry spans around stream chunks**: `event_listener.py:449-460` subscribes a console-formatter handler, but tracing spans for individual chunks are not in the read window. No evidence found that streamed chunks produce individual OTel spans (one span per call vs. one per chunk is unknown).
- **Sync vs async streaming consistency for `kickoff_for_each` (sync variant)**: `crew.py:1074-1108` returns a `list[CrewStreamingOutput]` rather than a fused stream — sync consumers cannot iterate a single stream across multiple crews. Asymmetry vs. the async variant (`crew.py:1164-1188`).

---

Generated by `dimensions/01.08-streaming-execution-semantics.md` against `crewai`.
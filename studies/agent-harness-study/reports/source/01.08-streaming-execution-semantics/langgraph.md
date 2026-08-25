# Source Analysis: langgraph

## Streaming Execution Semantics

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python 3.11+ / Async-first, LangChain Core + langgraph-sdk |
| Analyzed | 2026-07-03 |

## Summary

LangGraph exposes streaming at three layered levels: a low-level "raw stream mode" surface (`graph.stream(stream_mode=...)` / `astream(...)`, default v1), a typed v2 `StreamPart` envelope (`{"type","ns","data","interrupts"}`), and an experimental v3 protocol (`stream_events(version="v3")` / `astream_events(version="v3")`) that fronts the v2 stream with a `StreamMux`+`StreamTransformer` pipeline driven by caller iteration (`GraphRunStream` / `AsyncGraphRunStream`). It does **not** tokenize LLM responses itself — token-level streaming comes from LangChain's chat-model hook (`on_llm_new_token` in v1, `on_stream_event` in v2/v3). Tool-call output is partially streamed through a `StreamToolCallHandler` ContextVar bridge and surfaced on a dedicated `tools` channel (typed `tool_output_deltas` in v3 via prebuilt's `ToolCallStream`). Cancellation is cooperative and explicit (`abort()` on v3 run streams; `RunControl.request_drain()` to cooperatively stop; `durability` controls when writes become durable). Partial tool calls and partial state are *not* rolled back — the framework persists whatever is durable at the configured durability mode and exposes a `Recursion limit` exit. Retries do not restore streamed output: retry policy reruns the node; downstream consumers must dedupe by `message_id` / tool-call id (`on_llm_end`'s `dedupe=True`). Streaming never weakens the underlying checkpoint guardrails (interrupts, durability, checkpointer allowlist) — those are evaluated independently of the stream.

## Rating

**8 / 10 — Clear model with explicit interfaces, broad test coverage, and operational safeguards, but with an experimental v3 protocol that is not yet durable under failure and some legacy v1 sharp edges around nested message streams.**

Rationale:
- Multiple named stream modes (`values`, `updates`, `messages`, `checkpoints`, `tasks`, `custom`, `debug`, `tools`) with documented semantics (`libs/langgraph/langgraph/pregel/main.py:2690-2710`).
- Two protocol generations (v1 and v2) with backward-compatible default and typed v2 `StreamPart` envelope (`libs/langgraph/langgraph/pregel/main.py:4234-4258`).
- v3 introduces a unifying `StreamMux` + `StreamTransformer` pipeline with auto-forwarding, lifecycle hooks, scheduled async tasks, mini-mux nesting, and an explicit abort protocol (`libs/langgraph/langgraph/stream/_mux.py`).
- Safety rails: caller-driven pumps give memory bounding, root vs. child distinction prevents seq-number drift, async drain is graceful under cancel (`run.abort()` cancels in-flight `_anext_task`) (`libs/langgraph/langgraph/stream/run_stream.py:458-498`).
- Deducted points: v3 is marked `@beta` ("experimental and may change") and doesn't fully support nested v1 streams (`libs/langgraph/langgraph/pregel/main.py:3683-3691`). Per-stream-mode filtering has subtle defaults — e.g. `stream_mode` for `RemoteGraph` always appends `updates` even if not requested (`libs/langgraph/langgraph/pregel/remote.py:715-721`).

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Stream modes enumeration | "values", "updates", "custom", "messages", "checkpoints", "tasks", "debug" | `libs/langgraph/langgraph/pregel/main.py:2697-2710` |
| `stream_mode` attribute default | `stream_mode: StreamMode = "values"` | `libs/langgraph/langgraph/pregel/main.py:708-709` |
| Eager streaming flag | `stream_eager: bool = False` (auto on for messages/custom) | `libs/langgraph/langgraph/pregel/main.py:711-713` |
| Streaming sync queue | `SyncQueue()` with `put`/`get`/`wait`+`Semaphore` | `libs/langgraph/langgraph/_internal/_queue.py:70-122` |
| Streaming async queue | `AsyncQueue` (asyncio.Queue subclass with `wait`) | `libs/langgraph/langgraph/_internal/_queue.py:12-42` |
| Sync PregelLoop entry & concurrent streaming waiter | `if self.stream_eager or subgraphs or "messages" in stream_modes or "custom" in stream_modes:` | `libs/langgraph/langgraph/pregel/main.py:2952-2973` |
| Async concurrent streaming waiter | `waiter: asyncio.Task[None] | None = None` + cleanup | `libs/langgraph/langgraph/pregel/main.py:3407-3444` |
| Internal emit on superstep transitions | `_emit(mode, mapper, …)` for `values`, `updates`, `tasks`, `checkpoints`, `debug` | `libs/langgraph/langgraph/pregel/_loop.py:1357-1391` |
| Values emission on interrupt | `self._emit("values", map_output_values, …)` after pending writes applied | `libs/langgraph/langgraph/pregel/_loop.py:1317-1341` |
| Drain status check | `if self.control is not None and self.control.drain_requested: self.status = "draining"` | `libs/langgraph/langgraph/pregel/_loop.py:650-652` |
| Drain reason surfaced to API | `raise GraphDrained(loop.control.drain_reason or "shutdown")` | `libs/langgraph/langgraph/pregel/main.py:3027-3030`, `3508-3511` |
| `RunControl.request_drain()` | Run-scoped control surface for cooperative draining | `libs/langgraph/langgraph/runtime.py:79-104` |
| Recursion-limit exit branch | `GraphRecursionError(msg)` if `loop.status == "out_of_steps"` | `libs/langgraph/langgraph/pregel/main.py:3017-3026`, `3497-3507` |
| Tool call ContextVar bridge | `_tool_call_writer: ContextVar[ToolCallWriter]` for `ToolRuntime.emit_output_delta` | `libs/langgraph/langgraph/pregel/_tools.py:25-32` |
| `tool-started` / `tool-output-delta` / `tool-finished` / `tool-error` lifecycle | `_start` / `_end` / `_error` (sync via `on_tool_*` callbacks) | `libs/langgraph/langgraph/pregel/_tools.py:121-201`, `228-268` |
| v1 token streaming hook | `on_llm_new_token` -> `_emit(meta, chunk.message)` | `libs/langgraph/langgraph/pregel/_messages.py:151-165` |
| v2 message events hook | `on_stream_event` re-routes chat-model invoke to `_stream_chat_model_events` | `libs/langgraph/langgraph/pregel/_messages.py:259-408` |
| Dedupe on resumption / replay | `dedupe=True` and `_streamed_run_ids` to skip finalized messages | `libs/langgraph/langgraph/pregel/_messages.py:97-104`, `305-355` |
| `StreamProtocol` type | Callable `__call__` + `modes: set[StreamMode]`; `DuplexStream` combines streams | `libs/langgraph/langgraph/pregel/protocol.py:272-288`; `libs/langgraph/langgraph/pregel/_loop.py:147-153` |
| Custom stream writer function | `stream_writer` closure installed when `"custom"` in stream_modes | `libs/langgraph/langgraph/pregel/main.py:2855-2871`, `3283-3318` |
| Message dedupe via message_id on `message-start` | `if event.get("event") == "message-start": self._streamed_run_ids.add(run_id)` | `libs/langgraph/langgraph/pregel/_messages.py:402-406` |
| v3 entry — public dispatch | `_reject_v3_invariant_kwargs` rejects `stream_mode`/`subgraphs` | `libs/langgraph/langgraph/pregel/main.py:386-394` |
| v3 stream-mode union selector | `_collect_stream_modes(mux)` from transformers' `required_stream_modes` | `libs/langgraph/langgraph/pregel/main.py:397-413` |
| v3 native factories | `ValuesTransformer, MessagesTransformer, LifecycleTransformer, SubgraphTransformer` | `libs/langgraph/langgraph/pregel/main.py:3548-3559`, `3603-3612` |
| `ProtocolEvent` envelope with monotonic `seq` | Root mux assigns `seq`; child muxes share forwarded events | `libs/langgraph/langgraph/stream/_types.py:11-58`; `libs/langgraph/langgraph/stream/_mux.py:269-296` |
| `StreamTransformer` extension surface | `init`, `process`, `aprocess`, `finalize`, `afinalize`, `fail`, `afail`, `schedule`, `requires_async`, `supports_sync`, `required_stream_modes`, `before_builtins`, `_native` | `libs/langgraph/langgraph/stream/_types.py:60-330` |
| Sync `push` -> sync transformer pipeline | `for transformer in self._transformers: if not transformer.process(event): keep = False` | `libs/langgraph/langgraph/stream/_mux.py:269-296` |
| Async `apush` awaits each transformer sequentially | Comment notes: "slow `aprocess` serializes the pipeline by design" | `libs/langgraph/langgraph/stream/_mux.py:351-378` |
| Mini-mux cloning for subgraphs | `StreamMux._make_child` clones factories into a child mux | `libs/langgraph/langgraph/stream/_mux.py:193-225` |
| Pumping model | Caller iteration drives `_pump_next` (sync) / `_apump_next` (async) | `libs/langgraph/langgraph/stream/run_stream.py:107-130`, `391-456` |
| `abort()` cancels in-flight pull | `anext_task.cancel(); await anext_task; await self._mux.aclose()` | `libs/langgraph/langgraph/stream/run_stream.py:458-498` |
| Sync `abort()` closes graph iterator | `graph_iter.close()` then `mux.close()` | `libs/langgraph/langgraph/stream/run_stream.py:132-155` |
| Root-supplied pump via `_wire_request_more` | `mux.bind_pump(self._pump_next)` | `libs/langgraph/langgraph/stream/run_stream.py:80-92` |
| Async "take-a-number" pump serialization | `asyncio.Condition` + `_pumping` flag | `libs/langgraph/langgraph/stream/run_stream.py:359-456` |
| Auto-close/fail via StreamMux.finalize/afail/fail | `for ch in self._channels: ch.close()` after transformers | `libs/langgraph/langgraph/stream/_mux.py:298-345`, `380-449` |
| StreamChannel single-subscriber constraint | `_subscribed` flag + lazy-subscribe on push | `libs/langgraph/langgraph/stream/stream_channel.py:120-180` |
| StreamChannel backpressure | Pumps are caller-driven so each cursor advance produces one event; pump-side memory bounded | `libs/langgraph/langgraph/stream/stream_channel.py:38-46`, `182-239` |
| Channel tee/atee fan-out | `tee(n)` / `atee(n)` per-channel | `libs/langgraph/langgraph/stream/stream_channel.py:245-340` |
| `ValuesTransformer` projection | Captures `values` events into drainable channel | `libs/langgraph/langgraph/stream/transformers.py:28-83` |
| `MessagesTransformer` ignores v1 chunks on v2/v3 | "Legacy `AIMessageChunk` tuples (from `on_llm_new_token`) are not streamed" | `libs/langgraph/langgraph/stream/transformers.py:265-290` |
| `MessagesTransformer.process` routes protocol events by run_id | `if isinstance(payload, dict) and "event" in payload: self._route_protocol_event(...)` | `libs/langgraph/langgraph/stream/transformers.py:265-322` |
| `SubgraphTransformer` discovers subgraph handles | Mini-mux per direct child, mini-muxes driven by parent pump | `libs/langgraph/langgraph/stream/transformers.py:670-925` |
| `_TasksLifecycleBase` infers lifecycle from `tasks` events | Tasks events suppressed (`return False`) | `libs/langgraph/langgraph/stream/transformers.py:458-473` |
| `RunControl.drain_requested` shapes tick loop drain status | `if self.control.drain_requested: self.status = "draining"` | `libs/langgraph/langgraph/pregel/_loop.py:650-652`, `runtime.py:79-104` |
| Async stream single-flight condition | `_pump_cond.wait_for…` | `libs/langgraph/langgraph/stream/run_stream.py:411-417` |
| Test: abort cancels running subgraph | `test_abort_cancels_running_subgraph` | `libs/langgraph/tests/test_pregel_stream_events_v3.py:610-655` |
| Test: abort cancels deeply nested subgraph | `test_abort_cancels_deeply_nested_subgraph` | `libs/langgraph/tests/test_pregel_stream_events_v3.py:657-706` |
| Test: abort cancels during in-flight pump | `test_abort_cancels_subgraph_during_inflight_pump` | `libs/langgraph/tests/test_pregel_stream_events_v3.py:708-766` |
| Test: drain request in invoke flow | `test_request_drain_allows_inflight_call_scheduling` | `libs/langgraph/tests/test_pregel.py:125-145` |
| Test: afail cancels scheduled tasks | `test_afail_cancels_pending_scheduled_tasks` | `libs/langgraph/tests/test_pregel_stream_events_v3.py:1570-1595` |
| Test: stream events v3 partial state on interrupt | `test_interrupt_values_snapshot_has_partial_state` | `libs/langgraph/tests/test_stream_events_v3_e2e.py:483` |
| Test: tool-call ContextVar writer reset | `_tool_call_writer.get() is None` after cancel | `libs/langgraph/tests/test_tool_stream_handler.py:152` |
| Test: context manager calls abort on exit | `test_context_manager_calls_abort_on_exit` (sync and async) | `libs/langgraph/tests/test_pregel_stream_events_v3.py:493-498`, `602-608` |
| Tool-call transformer partial output | `ToolCallStream` with `output_deltas` StreamChannel | `libs/prebuilt/langgraph/prebuilt/_tool_call_stream.py:17-67` |
| RemoteGraph v3 abort path | `client.runs.cancel(...)` then `sdk.close()` | `libs/langgraph/langgraph/pregel/_remote_run_stream.py:221-233`, `349-363` |
| RemoteGraph v3 always adds `updates` even if not requested | `if "updates" not in updated_stream_modes: updated_stream_modes.append("updates")` | `libs/langgraph/langgraph/pregel/remote.py:715-721` |
| `_StreamingCallbackHandler` not always present — guarded | `try: from langchain_core.tracers._streaming import _StreamingCallbackHandler` | `libs/langgraph/langgraph/pregel/main.py:195-198` |

## Answers to Dimension Questions

### Q1 — What is emitted while execution is still running?

Streaming emits at three granularities depending on the mode:

- **Per-superstep boundary** (default v1 `values` / `updates`): after all tasks in a step finish and `apply_writes` runs (`libs/langgraph/langgraph/pregel/_loop.py:1357-1391`, `687-695`). Channel updates from step N are visible only in step N+1 — this is documented as the BSP/Pregel model guarantee (`libs/langgraph/langgraph/pregel/main.py:2974-2978`).
- **Per-task** with `stream_mode="tasks"` (task start, task result) (`libs/langgraph/langgraph/pregel/_loop.py:573`, `667`, `1438-1443`).
- **Token-by-token** with `stream_mode="messages"` — the chat model drives it via `on_llm_new_token` in v1 / `on_stream_event` in v2/v3 (`libs/langgraph/langgraph/pregel/_messages.py:151-165`, `373-408`). Stream_eager is auto-enabled when `messages` or `custom` is requested (`libs/langgraph/langgraph/pregel/main.py:2952-2958`).
- **Tool call output** — partial chunks via `tool-output-delta` events on the `tools` channel, keyed by `tool_call_id` (`libs/langgraph/langgraph/pregel/_tools.py:142-153`).
- **Per-event** in v3 — every protocol event after the corresponding callback fires is dispatched through the transformer pipeline into matching projections (`libs/langgraph/langgraph/stream/_mux.py:269-296`, `transformers.py:70-82`).

Items may be emitted mid-flight in v3 because the graph generator yields one chunk per loop tick; a generator yield from `runner.atick` flushes everything currently buffered. In v3, the consumer's iteration is the pump that pulls the graph forward — there is no background task producing data that the consumer just samples from (verified by `_pump_next` pulling `next(self._graph_iter)` in `libs/langgraph/langgraph/stream/run_stream.py:107-130`).

### Q2 — Are partial outputs trusted?

Partial outputs are trusted by **type** (a `StreamChannel` may carry incomplete items; a chat-model chunk may not have a finalized `message_id`), but each consumer is responsible for its own dedupe:

- v1 messages dedupes by `message.id` via `dedupe=True` (`libs/langgraph/langgraph/pregel/_messages.py:97-104`, `151-178`).
- v2 messages handler tracks `_streamed_run_ids` and on `message-start` records `message_id` so that the chain-finalized replay doesn't double-emit (`libs/langgraph/langgraph/pregel/_messages.py:402-406`, `348-355`).
- v3 `MessagesTransformer.process` does NOT trust v1 `AIMessageChunk` tuples — the comment is explicit: "Legacy `AIMessageChunk` tuples (from `on_llm_new_token`) are not streamed into this projection: chat models that want to populate `run.messages` with content-block streaming must use `stream_events(version="v3")`" (`libs/langgraph/langgraph/stream/transformers.py:286-290`).
- Tool calls use `ToolMessage` shape on the messages channel only for legacy v1 streams; v2/v3 handlers skip ToolMessage from chain outputs to avoid double-counting (`libs/langgraph/langgraph/pregel/_messages.py:307-334`).

Guardrails are not weakened: streaming is purely a projection layer; durability (`libs/langgraph/langgraph/runtime.py` and the `Durability` literal `"sync"|"async"|"exit"` plumbed through `config[CONF][CONFIG_KEY_DURABILITY]` at `libs/langgraph/langgraph/pregel/main.py:2878-2879`, `3321-3322`), checkpointer allowlist (`libs/langgraph/langgraph/pregel/main.py:837-842`), and interrupts (`libs/langgraph/langgraph/pregel/_loop.py:660-664`) operate on the channel/checkpoint layer regardless of whether a stream is attached.

### Q3 — Can a failed stream resume?

There is no in-band stream "resume" — a stream ends when its generator raises `StopIteration`/`StopAsyncIteration`. v3 handles exhaustion gracefully: `mux.close()`/`mux.fail(err)` finalize all transformers and channels with the first transformer error as the failure marker (`libs/langgraph/langgraph/stream/_mux.py:312-345`, `410-449`).

What **does** survive a stream failure is the underlying graph state, because the Pregel loop manages `durability` independently from the stream (`libs/langgraph/langgraph/pregel/_loop.py:1300-1312`):

- `durability="sync"` flushes pending writes before the next tick (`libs/langgraph/langgraph/pregel/main.py:3002-3004`, `3475-3477`).
- `durability="exit"` flushes only on loop exit (`libs/langgraph/langgraph/pregel/_loop.py:1301-1312`).
- Failures during `put_writes` raise into the run as a run-level error (`libs/langgraph/langgraph/pregel/main.py:3033-3036`); the loop still records a checkpoint before propagating when `durability=="exit"` and the run is rooted (`libs/langgraph/langgraph/pregel/_loop.py:1305-1312`).

If a stream-aborted task was persistent, the next invocation can resume — `update_state` re-enters at the last durable checkpoint (`libs/langgraph/langgraph/pregel/main.py:2530-2556`).

For an `astream_events(version="v3")` run, "resume" is supported via `Command(resume=...)` when the user has a checkpointer — see `libs/langgraph/langgraph/pregel/_loop.py:878-921` — and the v3 lifecycle surfaces `interrupted`/`interrupts` so the caller can drive a resume (`libs/langgraph/langgraph/stream/run_stream.py:531-552`).

`RemoteGraph` v3 aborts with `client.runs.cancel(thread_id, run_id, wait=False)` and disconnects the SDK (`libs/langgraph/langgraph/pregel/_remote_run_stream.py:221-233`, `349-363`); the server-side run is the unit of recovery, not the stream.

### Q4 — Can a user stop the stream safely?

Yes, with three layers:

- **Per-projection pause/break**: an event consumer simply breaks out of the `for`/`async for` loop; nothing crashes, channels are not closed. The generator returned by `StreamChannel.__iter__` / `__aiter__` closes the subscription when GC happens (`libs/langgraph/langgraph/stream/stream_channel.py:159-223`).
- **Run abort**: `GraphRunStream.abort()` (sync, `run_stream.py:132-155`) closes the underlying graph iterator, closing the generator — which propagates `GeneratorExit` into any still-running node. `AsyncGraphRunStream.abort()` cancels in-flight `_anext_task`, waits for the cancelled child task, closes the async iterator, and gracefully closes the mux (`libs/langgraph/langgraph/stream/run_stream.py:458-498`).
- **Context manager**: `with run` / `async with run` calls `abort()` on exit (`libs/langgraph/langgraph/stream/run_stream.py:157-166`, `500-509`). Tests cover both sync and async (`libs/langgraph/tests/test_pregel_stream_events_v3.py:493-498`, `602-608`).

A **cooperative drain** is also exposed: `RunControl.request_drain()` flips `drain_requested` and the next `tick()` short-circuits to `status = "draining"`, which the API then re-raises as `GraphDrained` (`libs/langgraph/langgraph/pregel/_loop.py:650-652`, `runtime.py:79-104`, `main.py:3027-3030`). Tests prove this works without losing in-flight calls: `test_request_drain_allows_inflight_call_scheduling` (`libs/langgraph/tests/test_pregel.py:125-145`).

A timeout-cap signal is also exposed via `step_timeout` (per superstep, `libs/langgraph/langgraph/pregel/main.py:726-727`, `2984`, `3457`).

The v3 `StreamMux.afail` path waits for any `schedule()`'d tasks before failing transformers; user opt-in is via `on_error="raise"` (`libs/langgraph/langgraph/stream/_mux.py:380-449`).

### Q5 — Does streaming weaken guardrails?

No evidence found that streaming weakens guardrails. Specifically:

- Durability decisions are made on the loop (`durability` field of `Runtime.control`, `libs/langgraph/langgraph/runtime.py:79-104`) and explicit `"sync"|"async"|"exit"` mode (`libs/langgraph/langgraph/pregel/main.py:3170-3168` for arg → runtime mapping). A graph with `durability="sync"` still flushes writes before the next tick even when `astream` is attached.
- Interrupts (`interrupt_before`, `interrupt_after` and `interrupt()` from inside nodes) are evaluated in `tick()` *before* the tasks run, independent of the stream (`libs/langgraph/langgraph/pregel/_loop.py:660-664`).
- Graph-callback lifecycle events still flow through `run_manager.on_chain_start` / `on_chain_end` / `on_chain_error` / `on_resume` / `on_interrupt` even when the user is in `astream_events(version="v3")` (`libs/langgraph/langgraph/pregel/main.py:2807-2816`, `2903-2912`, `3220-3228`, `3346-3366`).
- The `messages` stream handler strips `ToolMessage`s from chain-end replays in v2 (`libs/langgraph/langgraph/pregel/_messages.py:312-334`), and the v3 layer filters incomplete legacy v1 chunks (`libs/langgraph/langgraph/stream/transformers.py:286-290`) — both reduce double-count rather than weaken guardrails.

Where streaming can subtly change semantics: in `RemoteGraph._get_stream_modes`, `"updates"` is *always* appended if not present (`libs/langgraph/langgraph/pregel/remote.py:715-716`). This is a behavior users should be aware of but it does not weaken a checkpointer guardrail — it changes what the server is asked to stream, not the durability model.

## Architectural Decisions

1. **Two streaming protocols stacked on the same graph generator.** v1 (raw `dict[str, Any]`) and v2 (typed `StreamPart`) come from `Pregel.stream/astream`; v3 wraps v2 with a `StreamMux` and a transformer pipeline and is reached via `stream_events(version="v3")` (`libs/langgraph/langgraph/pregel/main.py:2670-2687`, `3519-3627`). The dispatcher hard-rejects `stream_mode` and `subgraphs` for v3 because it owns them (`libs/langgraph/langgraph/pregel/main.py:386-394`).

2. **Caller-driven pump instead of background tasks.** `GraphRunStream` is explicitly described as "No background thread is used — the caller's `for` loop is the pump" (`libs/langgraph/langgraph/stream/run_stream.py:34-37`). The async lane uses an `asyncio.Condition` to give "take-a-number" semantics: at most one task advances the generator at a time, others wait, then everyone wakes once the pumper signals (`libs/langgraph/langgraph/stream/run_stream.py:411-456`).

3. **Monotonic `seq` numbers from the root mux only.** Child mini-muxes receive forwarded events without mutating envelopes (`libs/langgraph/langgraph/stream/_mux.py:86-89`, `193-225`, `273-296`). Ordering is preserved across the entire run stream with no copying cost for forwarded events.

4. **Lazy-subscribe with single-subscriber semantics.** A `StreamChannel.push` only buffers if subscribed, but auto-forwards to the mux main log always (`libs/langgraph/langgraph/stream/stream_channel.py:120-141`). This bounds memory when projections go unread while keeping wire-level events visible.

5. **Token-level streaming defers to LangChain's callback contract.** LangGraph never tokenizes itself — `StreamMessagesHandler.on_llm_new_token` (v1) and `StreamMessagesHandlerV2.on_stream_event` (v2/v3) are the bridges (`libs/langgraph/langgraph/pregel/_messages.py:151-165`, `373-408`). For v3 to populate `run.messages` with content-block streaming, the underlying chat model must use the content-block event protocol (`libs/langgraph/langgraph/pregel/main.py:3683-3691`).

6. **Tool-call partial outputs go through a ContextVar bridge.** `StreamToolCallHandler._start` sets `_tool_call_writer` to a closure bound to a single `tool_call_id`; `ToolRuntime.emit_output_delta` reads it (`libs/langgraph/langgraph/pregel/_tools.py:25-32`, `142-156`). On async detach (worker thread without context copy) the reset is best-effort (`libs/langgraph/langgraph/pregel/_tools.py:213-222`).

7. **`tasks` events are absorbed, not forwarded.** `LifecycleTransformer` and `SubgraphTransformer` `return False` on `tasks` events so they are folded into the synthesized `lifecycle` channel and `subgraphs` handle log; raw `tasks` are reachable via the explicit `TasksTransformer` (`libs/langgraph/langgraph/stream/transformers.py:458-473`, `1002-1039`).

8. **Auto-close / auto-fail wiring on channels.** The mux shuts down channels after `finalize`/`afinalize` (sync/async) or `fail`/`afail`; transformers don't close channels themselves unless they need ordering overrides (`libs/langgraph/langgraph/stream/_mux.py:298-345`, `380-449`). This keeps teardown atomic.

9. **`RunControl` is a dedicated control surface, not a config dict.** A single attribute write is sufficient because there's no other mutable state on the control plane (`libs/langgraph/langgraph/runtime.py:79-104`). The drain check is `len(self._value)==0`, see drain check loop in `_loop.py:650`.

## Notable Patterns

- **Visitor-pattern transformer pipeline** (`StreamTransformer.process`/`aprocess`) that can suppress events (`return False`) before they hit the main log (`libs/langgraph/langgraph/stream/_mux.py:269-296`).
- **Two-lane sync/async with shared `StreamChannel` core**: `_bind(is_async=...)` flips the cursor factory; mismatched iteration raises `TypeError` (`libs/langgraph/langgraph/stream/stream_channel.py:159-223`).
- **Mini-mux nesting via factory cloning** — child muxes inherit the parent's pump binding so cursors on a subgraph handle transparently drive the root run (`libs/langgraph/langgraph/stream/_mux.py:193-225`, `run_stream.py:613-662`).
- **Tag-suppression pattern**: handlers respect `TAG_NOSTREAM` to opt out a chain from streaming without changing API surface (`libs/langgraph/langgraph/pregel/_tools.py:108-119`, `_messages.py:141`).
- **Single-consumer constraint with explicit fan-out API** (`tee(n)` / `atee(n)`) so multiple readers must opt-in by name rather than racing on the same buffer (`libs/langgraph/langgraph/stream/stream_channel.py:245-340`).
- **Drain via single flag**: one signal (`RunControl._drain_reason`) is enough because the next `tick()` picks it up cleanly (`libs/langgraph/langgraph/runtime.py:79-104`).
- **`_Collect_scheduled_tasks` snapshot pattern** to make async fan-out cancel correctly on `afail` (`libs/langgraph/langgraph/stream/_mux.py:451-458`).

## Tradeoffs

- **Caller-driven pumps are safer for memory but mean stalling when no one iterates.** If a user starts `run.output` and walks away, no pump drives the graph; whoever iterates a projection does. This is a deliberate design — memory bounded by caller pace — but means slow consumers slow the whole graph (`libs/langgraph/langgraph/stream/stream_channel.py:38-46`).
- **v2's `StreamMessagesHandlerV2` only delivers content blocks if the underlying chat model emits them.** A regular `langchain_core.language_models.BaseChatModel.invoke` invoked from inside a node will not token-stream through v3 unless it goes through the v2 event protocol — leading to the explicit caveat in `stream_events(version="v3")` docstring (`libs/langgraph/langgraph/pregel/main.py:3683-3691`). This is an awkward integration boundary.
- **Scheduled async transformer tasks** can choose between decoupled (`schedule()`) and serialised (`aprocess`) modes — but the choice has failure semantics. `schedule(on_error="raise")` re-raises into `aclose()`, while the default `"log"` swallows errors (`libs/langgraph/langgraph/stream/_types.py:230-261`).
- **v3 is `@beta`-tagged** with multiple "experimental" markers in the protocol class and entry points (`libs/langgraph/langgraph/pregel/main.py:3519`, `3674`, `3778`); users adopting it should expect breaking changes.
- **`stream_mode="messages"` swallows ToolMessage in v2 but not v1** — the asymmetry is documented but easy to miss (`libs/langgraph/langgraph/pregel/_messages.py:307-334`).
- **`RemoteGraph` always appends `updates` and re-maps `messages` to `messages-tuple`** on the wire (`libs/langgraph/langgraph/pregel/remote.py:703-721`); SDK users don't see the same shape they would locally.
- **The pump is single-flight even with `tee(n)`** — fan-out happens at the channel layer, not the pump, so readers still share the upstream event order (`libs/langgraph/langgraph/stream/stream_channel.py:245-340`).

## Failure Modes / Edge Cases

- **Run abort during in-flight pull**: the `AsyncGraphRunStream._apump_next` runs `_graph_aiter.__anext__()` in a child task so `abort()` can `cancel()` it and let the cancellation propagate into the running node (test: `test_abort_cancels_subgraph_during_inflight_pump`, `libs/langgraph/tests/test_pregel_stream_events_v3.py:708-766`).
- **`StopAsyncIteration` is converted into `aclose()`**; an exception is converted into `afail(err)`; a transformer error during `afail` is logged, not raised (`libs/langgraph/langgraph/stream/_mux.py:380-449`).
- **`put_nowait` may fail on `asyncio.QueueFull`** if the queue is full; the stream does not handle backpressure on the underlying event queue — the AsyncQueue passed through to v3 is unbounded (`libs/langgraph/langgraph/_internal/_queue.py:12-42`).
- **`recursion_limit` exceeded** → `GraphRecursionError`, the loop emits `status == "out_of_steps"` before raising (`libs/langgraph/langgraph/pregel/main.py:3017-3026`).
- **`drain_requested` after a partial step** → `GraphDrained` (`libs/langgraph/langgraph/pregel/main.py:3027-3030`). Tested to allow in-flight scheduling to complete (`libs/langgraph/tests/test_pregel.py:125-145`).
- **Subgraph transformer routing** depends on the `task_id` segment in the namespace; without a `task_id`, `LifecycleTransformer` skips the started payload and only the terminal sweep emits (`libs/langgraph/langgraph/stream/transformers.py:644-655`).
- **Two transformers declaring the same projection key**: the mux raises `ValueError` (`libs/langgraph/langgraph/stream/_mux.py:246-256`).
- **Async transformer used under sync `stream()`**: the mux raises at registration time — fail-fast, not at first event (`libs/langgraph/langgraph/stream/_mux.py:233-239`).
- **Second subscriber on a channel without `tee(n)`**: raises `RuntimeError` (`libs/langgraph/langgraph/stream/stream_channel.py:175-180`, `217-223`).
- **Mis-wired subscription order**: `interleave()` raises if a channel name does not exist, or if it is async-only / not yet bound / already subscribed (`libs/langgraph/langgraph/stream/run_stream.py:241-264`).
- **Stream closed before consumer subscribed**: `_subscribed=False` means pushes are dropped silently; auto-forward to the main log still happens, so the subscription is not needed for visibility, but a `tee(n)` after `close()` will yield nothing (`libs/langgraph/langgraph/stream/stream_channel.py:120-141`).
- **Step timeout**: `step_timeout` raises in `runner.tick`/`atick` (`libs/langgraph/langgraph/pregel/main.py:2984`, `3457`); manifests as a run error → `_emit("tasks", ...)` is the last visible event with the current state's pending writes (`libs/langgraph/langgraph/pregel/_loop.py:1317-1341`).
- **If the underlying ChatModel chain emits duplicates**: dedupe by `message.id` is the user's contract on v1; v2's `_streamed_run_ids` tracks session-level dedupe, and v3 sees the same dedupe via `MessagesTransformer._route_protocol_event` (`libs/langgraph/langgraph/pregel/_messages.py:97-178`, `libs/langgraph/langgraph/stream/transformers.py:300-322`).

## Future Considerations

- **v3 is experimental.** The `@beta` markers (`libs/langgraph/langgraph/pregel/main.py:3519`, `3758`, `3772-3780`) and the existence of `_V3_INVARIANT_KWARGS` (`libs/langgraph/langgraph/pregel/main.py:383-394`) signal that internal `stream_mode` derivation and `subgraphs=True` injection may move. Tooling that relies on v3 should be prepared to migrate.
- **Drain-reason handling for nested subgraphs**: only the top graph raises `GraphDrained` explicitly (`libs/langgraph/langgraph/pregel/main.py:3027-3030`); nested subgraphs rely on `Runtime.drain_requested` plumbing. There is no current hook in `_mux.fail` to differentiate "drained" from "failed" — `SubgraphTransformer._status_from_exception` maps to one of `"drained"|"interrupted"|"failed"` (`libs/langgraph/langgraph/stream/transformers.py:582-588`).
- **Tool streaming durability**: a `tool-output-delta` is not stored to the checkpointer — only `tool-finished` writes the terminal payload. If a run aborts mid-tool, the persisted output is empty.
- **v2 message shape on v3**: the `_route_whole_message` replay path turns a finalized `AIMessage` into synthetic protocol events via `message_to_events` (`libs/langgraph/langgraph/stream/transformers.py:324-328`); if `langchain-core` adds new content-block types they may need transformer attention.
- **`tee()` memory**: each branch keeps its own buffer; backpressure is naturally bounded because the source cursor is single-consumer (`libs/langgraph/langgraph/stream/stream_channel.py:265-286`), but `atee(n)` uses a lock and `asyncio.Lock` does not guarantee ordering across consumers (`libs/langgraph/langgraph/stream/stream_channel.py:288-340`).
- **`maxlen` on `StreamChannel`** is accepted but currently unused; future plans may enable bounded buffers and back-pressure (`libs/langgraph/langgraph/stream/stream_channel.py:49-68`).

## Questions / Gaps

- **What happens to v1 messages inside a v3-bound subgraph?** The docstring warns that v1 messages inside a v3 caller are not streamed; users opting in to v3 must switch every nested chat model to v2-aware invocation. No built-in compatibility shim was found (`libs/langgraph/langgraph/pregel/main.py:3683-3691`).
- **What happens if a transformer raises during `process()` mid-run?** `StreamMux.push` does not currently `try/except` around the for-loop, so a transformer exception propagates out of `push()` (`libs/langgraph/langgraph/stream/_mux.py:288-296`). No clear evidence found for a documented contract on partial transformer errors leaving the rest of the pipeline consistent.
- **No clear evidence found** for a hard cap on `messages` projection size for very long chat responses — caller-paced pump is the only bound.
- **No clear evidence found** for an audit log of `tool-output-delta`s that weren't followed by `tool-finished`. A reader cannot distinguish "still in flight" from "lost forever" until either `_finish` or `_fail` lands.
- **Persistent run endpoints** in `RemoteGraph` abort via `runs.cancel(...)`, but no clear evidence was found of how the server's `_RemoteGraphRunStream._run_id` is recreated if the SDK reconnects mid-stream.

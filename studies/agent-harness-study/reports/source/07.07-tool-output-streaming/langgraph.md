# Source Analysis: langgraph

## 07.07 — Tool Output Streaming

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (core `libs/langgraph`, prebuilt agents `libs/prebuilt`), Python SDK (`libs/sdk-py`); JS SDK moved out of repo (`libs/sdk-js/README.md:1-9`) |
| Analyzed | 2026-08-25 |

## Summary

LangGraph gives long-running tools three distinct, composable streaming channels, each with a different audience and lifecycle:

1. **Dedicated tool-output channel (`tools` stream mode).** A callback handler (`StreamToolCallHandler`, `libs/langgraph/langgraph/pregel/_tools.py:35-51`) attached by `Pregel.stream`/`astream` when `"tools"` is requested (`libs/langgraph/langgraph/pregel/main.py:2830-2838`) emits a per-tool-call event protocol — `tool-started`, `tool-output-delta`, `tool-finished`, `tool-error` — keyed by `tool_call_id`. Tool bodies push partial chunks via `ToolRuntime.emit_output_delta()` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1732-1750`), which is a deliberate silent no-op when the run was not started with the `tools` mode.
2. **Free-form custom events (`custom` stream mode).** Nodes and tasks obtain a `StreamWriter` either as an injected keyword argument (`libs/langgraph/langgraph/types.py:138-141`) or at runtime via `get_stream_writer()` (`libs/langgraph/langgraph/config.py:126-196`), emitting arbitrary progress payloads to consumers only.
3. **LLM token streaming (`messages` stream mode)** via `StreamMessagesHandler` (`libs/langgraph/langgraph/pregel/_messages.py:49-104`).

On top of these raw modes sits a v3 (beta) projection architecture: a `StreamMux` dispatches uniform `ProtocolEvent`s through `StreamTransformer`s into typed, single-consumer `StreamChannel`s (`run.custom`, `run.messages`, `run.tool_calls`, ...) (`libs/langgraph/langgraph/stream/__init__.py:1-45`). A prebuilt `ToolCallTransformer` reconstructs per-call `ToolCallStream` handles with a deltas channel plus terminal output/error fields (`libs/prebuilt/langgraph/prebuilt/_tool_call_transformer.py:44-78`), and the Python SDK ships wire-level decoders mirroring that state machine for remote clients (`libs/sdk-py/langgraph_sdk/stream/decoders.py:199-254`).

The design keeps partial output strictly consumer-facing: deltas are ephemeral in-memory channels ("no retention beyond what's currently queued", `libs/langgraph/langgraph/stream/stream_channel.py:24-27`) and are never fed back into model context; the model only ever sees the tool's final result as a `ToolMessage` in state. Durability of partial output is opt-in via state writes (e.g. UI messages are dual-written to stream *and* a checkpointed state key). Cancellation during streaming is well handled on both lanes: aborts cancel in-flight pulls and propagate into running nodes, open tool streams are failed/closed deterministically, and cancelled tasks still flush their partial writes to the checkpointer.

## Rating

**8 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale:
- Explicit, documented interfaces at every layer: `StreamWriter` (`libs/langgraph/langgraph/types.py:138-141`), `get_stream_writer()` (`libs/langgraph/langgraph/config.py:126`), `ToolRuntime.emit_output_delta` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1732`), `ToolCallStream` handle contract (`libs/prebuilt/langgraph/prebuilt/_tool_call_stream.py:17-35`), and the `StreamTransformer` extension point (`libs/langgraph/langgraph/stream/_types.py:44-115`).
- Strong test coverage of intended behavior and edge cases: delta ordering bracketed by start/finish (`libs/langgraph/tests/test_tool_stream_handler.py:102-126`), error events (`libs/langgraph/tests/test_tool_stream_handler.py:128-147`), concurrent parallel calls not bleeding across `tool_call_id`s (`libs/langgraph/tests/test_tool_stream_handler.py:200-237`), subgraph namespace propagation (`libs/langgraph/tests/test_tool_stream_handler.py:240-290`), and no-op behavior without the mode (`libs/langgraph/tests/test_tool_stream_handler.py:154-173`).
- Operational safeguards: silent no-op writers when unconfigured, mux-wide fail/close sweeps for in-flight streams, cancellation propagation into nodes, and caller-driven-pump backpressure.

Why not higher:
- Partial output is not durable by default — a crash loses everything streamed since the last checkpoint unless authors mirror data into state themselves.
- The v3 protocol is self-declared beta/experimental (`libs/langgraph/langgraph/pregel/main.py:3504`, `libs/langgraph/langgraph/stream/run_stream.py:36`).
- Typing/doc gap: the runtime supports `"tools"` in `stream_mode` (`libs/langgraph/langgraph/pregel/main.py:2830`) but the public `StreamMode` literal does not include it (`libs/langgraph/langgraph/types.py:122-124`), and `_defaults` performs no mode validation (`libs/langgraph/langgraph/pregel/main.py:2571-2578`).
- Cross-surface asymmetry: core deltas accept any JSON value, but the SDK decoder drops non-string deltas (`libs/sdk-py/langgraph_sdk/stream/decoders.py:239-243` vs `libs/prebuilt/langgraph/prebuilt/tool_node.py:1743-1745`).

## Evidence Collected

Every entry cites file paths relative to `studies/agent-harness-study/sources/langgraph`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Stream modes | `StreamMode` Literal: values/updates/checkpoints/tasks/debug/messages/custom; documented semantics | libs/langgraph/langgraph/types.py:122-136 |
| Writer API | `StreamWriter = Callable[[Any], None]`; injected into nodes on request; no-op unless `custom` mode | libs/langgraph/langgraph/types.py:138-141 |
| Runtime writer access | `get_stream_writer()` reads `runtime.stream_writer` from config contextvar | libs/langgraph/langgraph/config.py:126,195-196 |
| No-op default | `_no_op_stream_writer` default; `Runtime.stream_writer` field defaults to it | libs/langgraph/langgraph/config.py:13-14; libs/langgraph/langgraph/runtime.py:206 |
| Tools stream handler | `StreamToolCallHandler` emits `tool-started`/`tool-output-delta`/`tool-finished`/`tool-error`; `run_inline = True` for deterministic ordering | libs/langgraph/langgraph/pregel/_tools.py:35-53 |
| Delta writer plumbing | Handler sets `_tool_call_writer` ContextVar per executing tool call; reset on end/error (tolerating cross-thread token invalidation) | libs/langgraph/langgraph/pregel/_tools.py:25-32,142-156,213-222 |
| Tool-side emit API | `ToolRuntime.emit_output_delta(delta)` forwards to ContextVar; silent no-op if unset | libs/prebuilt/langgraph/prebuilt/tool_node.py:1732-1750 |
| Mode wiring | `Pregel.stream` appends `StreamMessagesHandler` for `messages` and `StreamToolCallHandler` for `tools`; defines inline `stream_writer` closure for `custom` | libs/langgraph/langgraph/pregel/main.py:2811-2860 |
| Event payload shapes | Delta payload `{event, tool_call_id, delta}`; finished `{event, tool_call_id, output}`; error `{event, tool_call_id, message}`; started carries `input` | libs/langgraph/langgraph/pregel/_tools.py:142-165,167-201 |
| Namespace scoping | `_ns_for_emit` derives namespace from `langgraph_checkpoint_ns`, honors `TAG_NOSTREAM` opt-out (`constants.py:24`) and subgraphs filter | libs/langgraph/langgraph/pregel/_tools.py:87-119 |
| Message streaming | `StreamMessagesHandler._emit` pushes `(message, metadata)` tuples with dedupe; `run_inline` for ordering | libs/langgraph/langgraph/pregel/_messages.py:49-58,97-104 |
| v3 protocol envelope | `ProtocolEvent` with monotonic `seq`, wall-clock timestamp caveat, method discriminator | libs/langgraph/langgraph/stream/_types.py:14-41 |
| Transformer pipeline | `StreamMux.push/apush` route events through transformers into main log; seq assigned at log append | libs/langgraph/langgraph/stream/_mux.py:269-296,351-378 |
| Custom projection | `CustomTransformer` surfaces `get_stream_writer()` payloads on `run.custom` scoped to own namespace | libs/langgraph/langgraph/stream/transformers.py:85-117 |
| Tool-call projection | `ToolCallTransformer` builds `ToolCallStream` per call; deltas appended; finalize/fail close or fail still-active streams | libs/prebuilt/langgraph/prebuilt/_tool_call_transformer.py:109-165 |
| Per-call handle | `ToolCallStream.output_deltas` StreamChannel + terminal `output`/`error`/`completed` fields | libs/prebuilt/langgraph/prebuilt/_tool_call_stream.py:17-35,59-88 |
| Partial-output buffer semantics | StreamChannel: single consumer, items popped as consumed, "no retention beyond what's currently queued"; `tee`/`atee` fan-out | libs/langgraph/langgraph/stream/stream_channel.py:14-47,245-341 |
| Backpressure | Caller-driven pump: each cursor advance produces ≤1 event; async backpressure via subscriber drain pacing graph | libs/langgraph/langgraph/stream/stream_channel.py:33-43; libs/langgraph/langgraph/stream/run_stream.py:320-333 |
| Cancellation (v3) | `AsyncGraphRunStream.abort()` cancels in-flight pull so CancelledError propagates into nodes/subgraphs; idempotent; context manager guarantees shutdown | libs/langgraph/langgraph/stream/run_stream.py:484-524,529-535 |
| Cancellation persistence | Runner commits `(ERROR, exception)` write for cancelled tasks via `put_writes`; interrupts saved to checkpointer | libs/langgraph/langgraph/pregel/_runner.py:579-591 |
| Interrupt emission during streaming | On interrupt, loop emits final `values` event with pending writes applied and `updates` with `__interrupt__`; persists exit-delta checkpoint | libs/langgraph/langgraph/pregel/_loop.py:1320-1375 |
| User-raised cancel surfacing | `NodeCancelledError` converts user-thrown `CancelledError` into node failure instead of silent teardown | libs/langgraph/langgraph/errors.py:168-188 |
| Output marking (v1/v2) | `_output` yields `(mode, payload)` or `(ns, mode, payload)` tuples; v2 emits typed `{type, ns, data[, interrupts]}` parts | libs/langgraph/langgraph/pregel/main.py:4184-4243 |
| Model feedback path | Final tool result enters state as `ToolMessage` (updates mode shows node output); deltas never injected into messages channel | libs/prebuilt/tests/test_tool_node.py:1600-1618 |
| UI integration | `push_ui_message` dual-writes: stream writer + durable state key `ui` via CONFIG_KEY_SEND; merge semantics in metadata | libs/langgraph/langgraph/graph/ui.py:99-130 |
| UI reducer durability | `ui_message_reducer` merges/removes by id incl. prop merging — replayable through checkpoints | libs/langgraph/langgraph/graph/ui.py:165-227 |
| Remote/wire support | SDK `ToolCallsDecoder` reconstructs handles from `tools` events; reserved channels rejected up front | libs/sdk-py/langgraph_sdk/stream/decoders.py:14-31,199-254 |
| Subagent correlation | Lifecycle inference harvests triggering `tool_call_id` from task inputs to link subagent starts to tool calls | libs/langgraph/langgraph/stream/transformers.py:417-426,488-516 |
| Tests: happy path & errors | started→finished cycle, delta order, tool-error message, no-op outside tools mode, concurrency isolation, subgraph ns | libs/langgraph/tests/test_tool_stream_handler.py:76-290 |
| Tests: transformer unit | Delta interleaving across two active calls; error propagation; finalize closes incomplete streams | libs/prebuilt/tests/test_tool_call_transformer.py:97-265 |
| Tests: custom writer from tools | `get_stream_writer` inside a tool streams `("custom", {...})` chunks alongside final updates | libs/prebuilt/tests/test_tool_node.py:1567-1618 |

## Answers to Dimension Questions

### 1. Can tools stream progress?

Yes, through two first-class mechanisms. (a) Structured per-call output streaming: attach nothing, just request `stream_mode=["tools", ...]` and call `runtime.emit_output_delta(chunk)` inside any `@tool` body (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1732-1750`); the framework installs a ContextVar-bound writer at `on_tool_start` (`libs/langgraph/langgraph/pregel/_tools.py:142-156`) and brackets the call with `tool-started`/`tool-finished` (or `tool-error`) events. (b) Unstructured progress: `writer(...)` / `get_stream_writer()(...)` inside nodes or tasks lands on the `custom` channel (`libs/langgraph/langgraph/config.py:126-196`; wiring at `libs/langgraph/langgraph/pregel/main.py:2841-2854`). LLM token-by-token progress is separately available via `messages` mode. Parallel tool calls are isolated by `tool_call_id`, verified by test `test_parallel_tool_calls_do_not_bleed` (`libs/langgraph/tests/test_tool_stream_handler.py:200-237`).

### 2. Are partial outputs durable?

No — by design they are transient. `StreamChannel` retains items only until consumed ("no retention beyond what's currently queued", `libs/langgraph/langgraph/stream/stream_channel.py:24-27`); nothing in the loop or runner persists `custom` or `tools` payloads — checkpoints store only task writes/channel values (`libs/langgraph/langgraph/pregel/_runner.py:604-613`). Two durable escape hatches exist: (a) write-through into state, exactly what `push_ui_message` does by sending the event both to the stream writer and to the `ui` state key via `CONFIG_KEY_SEND` (`libs/langgraph/langgraph/graph/ui.py:126-128`), making it replayable through `ui_message_reducer` (`libs/langgraph/langgraph/graph/ui.py:165-227`); (b) checkpoint/task-write machinery itself, which persists interrupts, errors, and results even on cancellation (`libs/langgraph/langgraph/pregel/_runner.py:579-603`). So durability is possible and demonstrated, but it is the author's responsibility per-event.

### 3. Does the model act on partial output?

No. There is no mechanism that folds `tool-output-delta` or `custom` payloads back into model context. The model-visible record is produced solely by the tool's return value becoming a `ToolMessage` written to the `messages` channel after execution (observable in `test_tool_node_stream_writer`: the `updates` chunk contains only the final `ToolMessage`, while custom chunks flow separately — `libs/prebuilt/tests/test_tool_node.py:1595-1618`). `ToolCallStream.output` is populated only by the terminal `tool-finished` event (`libs/prebuilt/langgraph/prebuilt/_tool_call_stream.py:80-83`). This is a clean separation: partial output informs humans/UIs; completed results inform the agent. The closest thing to "the system acting on streaming signals" is control-plane: `LifecycleTransformer` correlates a task's `tool_call_id` to a spawned named subagent (`cause: {"type": "toolCall", ...}`, `libs/langgraph/langgraph/stream/transformers.py:488-546`), enabling observers to react mid-flight.

### 4. Can users interrupt?

Yes, at several levels. (a) Cooperative human-in-the-loop interrupts pause the graph, are persisted to the checkpointer, and surface as clearly marked stream events: on interrupt the loop applies pending writes, emits one last `values` event, and emits `updates` containing the `__interrupt__` key (`libs/langgraph/langgraph/pregel/_loop.py:1335-1375`, `1440-1451`). (b) Hard cancellation of a v3 stream: `AsyncGraphRunStream.abort()` cancels the in-flight pull task so `CancelledError` propagates through the Pregel loop into nested subgraph nodes, then closes everything (`libs/langgraph/langgraph/stream/run_stream.py:484-524`); the sync lane propagates `GeneratorExit` by closing the generator (`run_stream.py:148-171`). Even then, the runner records `(ERROR, CancelledError)` in task writes and flushes them to the checkpointer so the super-step can finish coherently (`libs/langgraph/langgraph/pregel/_runner.py:579-583`). (c) Per-run cooperative drain exists via `RunControl` (`main.py:2712`). A user node that raises `CancelledError` itself is deliberately converted to `NodeCancelledError` so it reports as failure rather than silent teardown (`libs/langgraph/langgraph/errors.py:168-188`).

### 5. Are partial outputs clearly marked?

Yes. Every raw chunk is tagged with its mode: multi-mode sync/async streams yield `(mode, payload)` or `(ns, mode, payload)` tuples, and v2 emits typed parts `{type: "custom"|"messages"|..., ns, data}` (`libs/langgraph/langgraph/pregel/main.py:4219-4243`). In v3 every event is a `ProtocolEvent` with `method` (e.g. `"tools"`), `namespace`, monotonic `seq`, and timestamp (`libs/langgraph/langgraph/stream/_types.py:28-41`). Within the `tools` channel each payload carries its own `event` discriminator and `tool_call_id` (`_tools.py:147-153,158-165`). Terminal states are explicit (`completed`, `error`, `message`), and transformers deterministically close/fail any stream still open at run end (`libs/prebuilt/langgraph/prebuilt/_tool_call_transformer.py:152-165`; `libs/langgraph/langgraph/stream/transformers.py:335-340`), so consumers cannot hang on a half-open delta channel.

## Architectural Decisions

1. **Callback-handler ingestion, channel-agnostic emission.** Tool streaming piggybacks on LangChain's `on_tool_*` callback hooks (`StreamToolCallHandler` implements `BaseCallbackHandler` + `_StreamingCallbackHandler`, `libs/langgraph/langgraph/pregel/_tools.py:35`) rather than requiring tools to know about streams. This means any LangChain tool — including third-party ones — gets lifecycle events for free; only intra-body deltas require the explicit `emit_output_delta` call. `run_inline = True` keeps handlers on the main thread to preserve event ordering (`_tools.py:49-51`; same choice for messages, `libs/langgraph/langgraph/pregel/_messages.py:57-58`).
2. **ContextVar indirection between handler and tool body.** Rather than threading a writer through signatures, the handler publishes a per-call closure on `_tool_call_writer` and `emit_output_delta` reads it (`_tools.py:25-32`; `tool_node.py:1747-1750`). Reset tolerates invalid tokens when callbacks hop threads (`_tools.py:213-222`). Consequence: emission works deep inside helper functions without plumbing, and is silently inert outside a streamed tool run — chosen over failing loud, explicitly documented (`tool_node.py:1737-1740`) and tested (`test_tool_stream_handler.py:149-152`).
3. **Pull-based (caller-driven) streaming with no background pump.** In v3, iterating any projection drives the graph one event at a time (`GraphRunStream._pump_next`, `libs/langgraph/langgraph/stream/run_stream.py:123-146`); memory is bounded by consumer pace (`stream_channel.py:38-43`). Async uses take-a-number semantics with a single-flight pump lock so multiple projections can be consumed concurrently (`run_stream.py:417-482`).
4. **Transformers as the extension seam.** New projections (e.g. `ToolCallTransformer`) plug into the mux without touching the engine; `required_stream_modes` lets the mux compute which raw modes a v3 run must request (`main.py:398-411`, `_collect_stream_modes` used at `main.py:3549`); `before_builtins` orders content-mutating transformers ahead of eager built-ins (`stream/_types.py:94-109`).
5. **Dual-write for durable UI data instead of persisting the stream.** The framework refuses to make ephemeral streams durable and instead offers the reducer+state-key pattern (`graph/ui.py:126-128`), keeping checkpoint payloads deterministic while allowing arbitrary live decoration.
6. **Wire parity via mirrored decoders.** The SDK reimplements the transformer state machines client-side (`ToolCallsDecoder`, `decoders.py:199-254`) against the same `ProtocolEvent` vocabulary, and reserves conflicting channel names to fail closed (`decoders.py:25-31`).

## Notable Patterns

- **Bracketed event protocol**: every tool call produces `tool-started … tool-finished|tool-error` with deltas strictly inside the bracket — asserted directly in tests (`test_tool_stream_handler.py:121-126`).
- **Silent-degradation writers everywhere**: no-op stream writer default (`config.py:13-14`, `runtime.py:206`), no-op `emit_output_delta` without the mode (`tool_node.py:1748-1750`), suppressed emission under `TAG_NOSTREAM` tag (`_tools.py:110-111`, `constants.py:24`). Tool code stays stream-mode agnostic.
- **Scope-filtered fan-in/out**: all transformers drop events whose `namespace != scope` (e.g. `transformers.py:110-116`), while `SubgraphTransformer` clones mini-muxes per child scope so grandchildren project onto child handles (`transformers.py:670-711`); the tools handler mirrors this with `subgraphs=`/`parent_ns=` filtering (`_tools.py:87-119`).
- **Terminal-state sweeps**: `finalize`/`fail` hooks close or fail every in-flight projection (`_mux.py:298-345`; `transformers.py:330-340`; `_tool_call_transformer.py:152-165`), preventing leaked channels on early exit or error.
- **Correlation keys over nesting**: `tool_call_id` (deltas), `run_id` (message routing, `transformers.py:203-205`), and task-id↔tool_call_id joins for subagents (`transformers.py:488-516`) let flat event streams be reconstructed into trees by consumers.

## Tradeoffs

- **Ephemeral vs. durable**: keeping deltas out of checkpoints makes the hot path cheap and schema-free, but crash recovery replays state only — anything shown to a user since the last checkpoint vanishes unless dual-written like UI messages (`graph/ui.py:99-130`).
- **Human-facing vs. model-facing separation**: the model never sees partial output, which avoids confusing completions with half-formed data, but also means a tool cannot "think out loud" to the model mid-execution; that requires interrupt/resume instead.
- **Single-consumer channels**: simplicity and bounded memory come at the cost of explicit `.tee(n)`/`.atee(n)` for fan-out, with a second subscribe raising (`stream_channel.py:24-27,175-179`).
- **Inline callback handlers** avoid reordering/locking bugs but execute user-visible work on the main thread of the run.
- **Experimental v3**: the most ergonomic surfaces (`run.tool_calls`, `run.messages`, abort semantics) are gated behind `@beta` (`run_stream.py:36`, `main.py:3504,3560`), while production-stable usage remains tuple-based v1/v2.

## Failure Modes / Edge Cases

- **Tool raises mid-stream**: `tool-error` event carries `str(exception)`; the transformer fails the handle, closing the deltas channel (`_tools.py:185-201`; tested `test_tool_stream_handler.py:128-147`).
- **Run fails or exits with streams open**: `ToolCallTransformer.fail/finalize` force-terminalizes them (`_tool_call_transformer.py:152-165`); `MessagesTransformer.fail` fails open chat streams (`transformers.py:335-340`); the mux auto-fails channels even if transformer hooks raise (`_mux.py:326-345,424-449`).
- **Cancellation mid-pull**: async abort cancels the `__anext__` task because bare `aclose()` cannot interrupt a running generator (`run_stream.py:446-464,503-512`); cancelled tasks still persist their partial writes (`_runner.py:579-583`).
- **Cross-context callback execution**: ContextVar token may be invalid if `on_tool_end` runs in a different context; reset swallows `ValueError` and lifetime is bounded by the enclosing task (`_tools.py:213-222`).
- **Unsubscribed channels**: `push` skips buffering when nobody subscribed, but wired forwarding still fires — remote consumers see events even if no local cursor reads them (`stream_channel.py:120-140`).
- **Sync/async mismatch**: iterating a channel bound to the other lane raises `TypeError` immediately (`stream_channel.py:166-176,209-217`).
- **Timestamps are not ordering**: `ProtocolEvent.timestamp` is wall-clock and can go backwards (NTP); consumers must use `seq` (`stream/_types.py:15-20,32-35`) — a documented trap for cross-source merging.

## Future Considerations

- Promote `"tools"` into the public `StreamMode` literal and validate modes in `_defaults`, closing the typing/documentation gap between `types.py:122-124` and the runtime branch at `main.py:2830`.
- Reconcile delta type expectations across surfaces: core documents "any JSON-serializable value" (`tool_node.py:1743-1745`) while the SDK decoder silently requires `isinstance(delta, str)` (`decoders.py:241-243`) — either broaden the decoder or narrow the contract.
- Graduate v3 out of beta once the transformer/mux API stabilizes; the abort/backpressure semantics (`run_stream.py:417-535`) are the strongest part of the design and deserve a stable label.
- Consider an optional durable ring-buffer for tool deltas (per `StreamChannel.maxlen` parameter, currently accepted-but-unused, `stream_channel.py:60-63`) for post-hoc inspection of interrupted runs.
- Document a recommended pattern for "model-visible progress" (interrupt-based checkpointing of intermediate results) since streaming alone deliberately never reaches the model.

## Questions / Gaps

- **No evidence found** for any mechanism that feeds streamed partial output back into model context within this repository; searched `libs/langgraph` and `libs/prebuilt` for delta accumulation/aggregation into `ToolMessage` or prompt construction (grep for `output_deltas`, `tool-output-delta` consumers) and found only consumer-side projections/decoders.
- **Server-side emitter for the `tools` mode over REST**: the SDK decoders consume `tools` events (`decoders.py:216-254`) and local Pregel emits them, but the actual HTTP server implementation (langgraph-server) is not part of this repository, so end-to-end wire behavior for remote tool streaming could not be verified here beyond the SDK's decoder contract and `remote.py`'s mode mapping (`libs/langgraph/langgraph/pregel/remote.py:704-712` maps only `messages` ↔ `messages-tuple`).
- Whether `stream_eager` interacts differently with `tools` mode than with `messages`/`custom` (it gates the waiter setup at `main.py:2936-2942`, where `tools` is absent from the eager-start condition) could not be confirmed as intentional vs. oversight from code alone.
- `sdk-js` is a relocation stub in this snapshot (`libs/sdk-js/README.md:1-9`), so JS client capabilities were out of scope.

---

Generated by `dimensions/07.07-tool-output-streaming.md` (dimension 07.07: Tool Output Streaming) against `langgraph`.

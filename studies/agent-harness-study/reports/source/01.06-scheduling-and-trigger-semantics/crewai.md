# Source Analysis: crewai

## 01.06: Scheduling and Trigger Semantics

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python 3.10+ / Pydantic v2 / asyncio / a2a SDK |
| Analyzed | 2026-07-03 |

## Summary

CrewAI is a **library, not a hosted runtime.** Execution always originates from a Python entry point that the caller invokes directly; there is no in-process scheduler, no cron surface, no webhook receiver, no queue/worker adapter, and no background daemon. The trigger vocabulary is therefore narrow but well-defined:

- **Direct `kickoff()` / `kickoff_async()` / `kickoff_for_each()` calls** on `Crew` (`lib/crewai/src/crewai/crew.py:980`, `:1074`, `:1110`, `:1164`) and on `LiteAgent` (`lib/crewai/src/crewai/lite_agent.py:477`, `:750`); on `Flow` (`lib/crewai/src/crewai/flow/runtime/__init__.py:2110`, `:2209`); and on the metaclass-wrapped `Agent.kickoff_with_a2a` (`lib/crewai/src/crewai/a2a/wrapper.py:175`, `:202`).
- **Turn-by-turn conversational entry** via `Flow.handle_turn()` and `Flow.chat()` (`lib/crewai/src/crewai/experimental/conversational_mixin.py:259`, `:311`), which stash a pending user message and run one `kickoff` per turn with state restored from `@persist`.
- **Durable resumption** via `Crew.from_checkpoint()` (`lib/crewai/src/crewai/crew.py:403`), `Flow.from_checkpoint()` (`lib/crewai/src/crewai/flow/runtime/__init__.py:866`), `Flow.from_pending()` (`:1432`), and `Flow.fork()` (`:913`); checkpointing itself is event-driven via the lazy listener at `lib/crewai/src/crewai/state/checkpoint_listener.py:43`.
- **In-process event subscriptions** through the singleton `CrewAIEventsBus` (`lib/crewai/src/crewai/events/event_bus.py:94`), which dispatches to sync handlers on a `ThreadPoolExecutor` and async handlers on a daemon-thread-owned asyncio loop (`event_bus.py:165-189`, `:570-645`).
- **OS-signal triggers** registered by `Telemetry._register_shutdown_handlers` (`lib/crewai/src/crewai/telemetry/telemetry.py:176-234`) that re-emit as events on SIGTERM / SIGINT / SIGHUP / SIGTSTP / SIGCONT.
- **Polling-based A2A updates** inside `PollingHandler.execute` (`lib/crewai/src/crewai/a2a/updates/polling/handler.py:123-200`) and the cancellable A2A task wrapper (`lib/crewai/src/crewai/a2a/utils/task.py:115-195`).

Every trigger runs the same async/sync code paths the caller would invoke interactively — there is exactly one execution engine, and "scheduled/background" work is just a `kickoff` running on a different actor (a polling coroutine, a deferred `run_coroutine_threadsafe`, or a fresh process picking up a checkpoint). Durability is provided entirely by the explicit `CheckpointConfig` / `FlowPersistence` opt-ins; without them, a restart loses in-flight runs.

The CLI surface itself was extracted to `crewai-cli` (`lib/crewai/src/crewai/cli/__init__.py:19-27` — `crewai_cli` shim only), so the in-source CLI no longer adds a triggering surface.

## Rating

**6/10 — Present but inconsistent, weakly documented at the trigger layer, and missing a first-class scheduler.**

Strengths: `crewai_event_bus.emit()` is the canonical in-process trigger mechanism and is well-isolated, with sync/async dispatch and replay support (`event_bus.py:67-80`, `:570-645`, `:671-730`); the checkpoint / `from_checkpoint` / `fork` API provides genuine durability across restarts (`state/checkpoint_config.py:159-233`, `state/runtime.py:286-498`, `flow/runtime/__init__.py:866-945`); the `HumanFeedbackPending` exception is treated as control-flow rather than error and triggers an auto-persist + return-value pattern (`flow/async_feedback/types.py:141-211`, `flow/runtime/__init__.py:2503-2557`); the conversational mixin cleanly models turn-by-turn triggers with message persistence (`conversational_mixin.py:259-309`, `:707-733`); A2A server-side cancellation uses pub-sub + polling fallback with a 0.1s sleep (`a2a/utils/task.py:147-189`).

Weaknesses: no native cron / timer / queue / webhook trigger is exposed — "scheduled" means "another Python process calls `kickoff`"; triggers live across `crew.py`, `flow/runtime/__init__.py`, `a2a/wrapper.py`, `a2a/utils/task.py`, `state/checkpoint_listener.py`, and `hooks/{llm,tool}_hooks.py` with no shared `BaseTrigger` abstraction; idempotency is partial — `SQLiteFlowPersistence.save_pending_feedback` uses `INSERT OR REPLACE` keyed on `flow_uuid` (`flow/persistence/sqlite.py:229-244`) but `flow_states` rows are append-only and the checkpoint config has no built-in de-dup key; observed behavior in `restore_from_state_id` silently falls back when the source state is missing (`flow/runtime/__init__.py:2374-2379`) which is unsafe for a queue-like trigger surface; async `kickoff_async` is mostly a `asyncio.to_thread` shim around `kickoff` (`crew.py:1143`, `:1162`), so background tasks inherit the same OpenTelemetry contextvars but no separate runtime.

## Evidence Collected

Every entry cites file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Crew sync kickoff | `Crew.kickoff(inputs, input_files, from_checkpoint)` and routing into `_run_sequential_process` / `_run_hierarchical_process` | `lib/crewai/src/crewai/crew.py:980`–`:1069`, `:1485`, `:1489` |
| Crew async kickoff | `kickoff_async` delegates to `asyncio.to_thread(self.kickoff, ...)`; per-input fan-out is `kickoff_for_each_async` using `asyncio.gather` over a generator of coroutines | `lib/crewai/src/crewai/crew.py:1110`–`:1185`, `:1338`–`:1417` |
| Bulk / fan-out trigger | `kickoff_for_each` copies the crew per input; `kickoff_for_each_async` iterates `asyncio.gather` over per-input `kickoff_async` coroutines | `lib/crewai/src/crewai/crew.py:1074`–`:1108`, `:1164`–`:1464` |
| LiteAgent sync/async entry | `LiteAgent.kickoff(messages)` and `kickoff_async` calling `asyncio.to_thread(self.kickoff, ...)` — same shared `_execute_core` path | `lib/crewai/src/crewai/lite_agent.py:477`–`:546`, `:750`–`:770` |
| A2A-wrapped agent kickoff | `wrap_agent_with_a2a_instance` patches `kickoff` and `kickoff_async` to insert A2A delegation when `agent.a2a` is set | `lib/crewai/src/crewai/a2a/wrapper.py:97`–`:235` |
| A2A server-side task execution | `_execute_impl` is decorated with `@cancellable` which runs the task and a Redis pub-sub / in-process cancel watcher via `asyncio.wait(FIRST_COMPLETED)` | `lib/crewai/src/crewai/a2a/utils/task.py:115`–`:195`, `:290`–`:399` |
| A2A cancel pub-sub / polling fallback | `pubsub.subscribe(f"cancel:{task_id}")` with `aiocache` configured from `REDIS_URL` env or `SimpleMemoryCache`; 0.1s polling fallback at line 152 | `lib/crewai/src/crewai/a2a/utils/task.py:39`–`:172`, `:262-278` |
| A2A polling update trigger | `_poll_task_until_complete` sleeps `polling_interval` between `client.get_task` calls; timeout/max_polls both raise `A2APollingTimeoutError` | `lib/crewai/src/crewai/a2a/updates/polling/handler.py:45`–`:200` |
| A2A push / streaming update variants | `class PushNotificationHandler` and `class StreamingHandler` siblings of `PollingHandler` in `crewai.a2a.updates` | `lib/crewai/src/crewai/a2a/updates/push_notifications/handler.py`, `lib/crewai/src/crewai/a2a/updates/streaming/handler.py` |
| Flow sync / async entry | `Flow.kickoff` wraps `kickoff_async`, runs in a `ThreadPoolExecutor` when an asyncio loop is already running; `kickoff_async` is the canonical implementation | `lib/crewai/src/crewai/flow/runtime/__init__.py:2110`–`:2207`, `:2209`–`:2323` |
| Flow fork / checkpoint restore | `Flow.fork` and `Crew.fork` create a new execution branch; `Flow.from_checkpoint` rebuilds `_completed_methods`, `_method_outputs`, `_method_execution_counts`, `_state` | `lib/crewai/src/crewai/flow/runtime/__init__.py:866`–`:945`, `lib/crewai/src/crewai/crew.py:403`–`:450` |
| Flow paused-for-feedback | `Flow.from_pending(flow_id, persistence)` loads `pending_feedback` row and rehydrates a flow that returns `HumanFeedbackPending`; `resume` / `resume_async` deliver feedback and continue | `lib/crewai/src/crewai/flow/runtime/__init__.py:1432`–`:1571`, `lib/crewai/src/crewai/flow/async_feedback/types.py:141`–`:211` |
| Flow input provider trigger | `Flow.ask(message, timeout, metadata)` emits `FlowInputRequestedEvent`, checkpoints state, and runs the provider synchronously inside a `ThreadPoolExecutor` with cancellation | `lib/crewai/src/crewai/flow/runtime/__init__.py:3298`–`:3499` |
| `HumanFeedbackPending` is control flow not error | Caught in `kickoff_async` exception block, persisted to `pending_feedback` table, returned as value via `return e` | `lib/crewai/src/crewai/flow/runtime/__init__.py:2503`–`:2557` |
| Conversational turn trigger | `Flow.handle_turn(message, session_id, ...)` stashes pending state, calls `kickoff`, then promotes assistant message; `Flow.chat()` loops over `input_fn` | `lib/crewai/src/crewai/experimental/conversational_mixin.py:259`–`:362` |
| Deferred trace finalization across turns | `defer_trace_finalization=True` + `finalize_session_traces()` so multiple turns share one trace; `_deferred_flow_started_event_id` is stashed/restored | `lib/crewai/src/crewai/experimental/conversational_mixin.py:681`–`:696`, `:1012`–`:1072`, `lib/crewai/src/crewai/flow/runtime/__init__.py:2414`–`:2461` |
| Event bus dispatch | `CrewAIEventsBus.emit(source, event)` splits sync handlers (thread pool) vs async handlers (asyncio loop via `run_coroutine_threadsafe`) and supports `Depends` ordering + `replay` | `lib/crewai/src/crewai/events/event_bus.py:570`–`:730` |
| Singleton daemon loop | `_ensure_executor_initialized` lazily spawns `_sync_executor` (10 workers) and a daemon `CrewAIEventsLoop` thread running `loop.run_forever()` | `lib/crewai/src/crewai/events/event_bus.py:165`–`:214` |
| `flow.id` lookup on restore | `kickoff(inputs={"id": sid})` triggers `self.persistence.load_state(restore_uuid)` to hydrate `_state` before running start methods | `lib/crewai/src/crewai/flow/runtime/__init__.py:2381`–`:2407` |
| Fork hydration | `restore_from_state_id` mints a new `state.id`, hydration failure falls through "silently to baseline" | `lib/crewai/src/crewai/flow/runtime/__init__.py:2349`–`:2379` |
| Restore guard / single-shot | `is_restoring` flag clears only when `_completed_methods` is non-empty so a pure state reload does not enter resumption mode; `_restored_from_checkpoint` resets after one kickoff | `lib/crewai/src/crewai/flow/runtime/__init__.py:2325`–`:2347` |
| Checkpoint config + provider discriminator | `CheckpointConfig` with `JsonProvider | SqliteProvider` discriminated union; `location` is dir-path vs db-file path; `max_checkpoints` triggers `provider.prune` | `lib/crewai/src/crewai/state/checkpoint_config.py:159`–`:211` |
| Provider base | `BaseProvider.checkpoint` / `from_checkpoint` / `prune` / `extract_id` abstract surface | `lib/crewai/src/crewai/state/provider/core.py:10`–`:111` |
| Provider prune + lineage | `RuntimeState._chain_lineage` / `fork` write parent_id + branch on each checkpoint; `prune` removes oldest | `lib/crewai/src/crewai/state/runtime.py:216`–`:389`, `lib/crewai/src/crewai/state/provider/utils.py` |
| Lazy checkpoint listener | `_ensure_handlers_registered` registers handlers on first `CheckpointConfig` resolution; `_find_checkpoint` walks Task → Agent → Crew | `lib/crewai/src/crewai/state/checkpoint_listener.py:43`–`:111` |
| Trigger-event map | Full list of `CheckpointEventType` literals shows every event that may auto-checkpoint; signal events (`SIGTERM/SIGINT/...`) are first-class | `lib/crewai/src/crewai/state/checkpoint_config.py:14`–`:142` |
| `@persist` decorator | Class- or method-level metadata stamper; runtime calls `persistence.save_state(flow_uuid, method_name, state_data)` after each method | `lib/crewai/src/crewai/flow/persistence/decorators.py:67`–`:191` |
| SQLite state writes | `SQLiteFlowPersistence` uses `PRAGMA journal_mode=WAL`, two tables (`flow_states`, `pending_feedback`), `store_lock` for writer, `INSERT OR REPLACE` for the pending row keyed on `flow_uuid` | `lib/crewai/src/crewai/persistence/sqlite.py:71`–`:296` |
| Per-execution checkpoint hook | `_checkpoint_state_for_ask` writes a `_ask_checkpoint` row before each `ask()` | `lib/crewai/src/crewai/flow/runtime/__init__.py:3273`–`:3296` |
| Crew trigger-payload injection | `_set_allow_crewai_trigger_context_for_first_task` flips `task.allow_crewai_trigger_context=True` when `crewai_trigger_payload` is in inputs | `lib/crewai/src/crewai/crew.py:2325`–`:2338` |
| OS-signal-to-event bridging | `Telemetry._register_signal_handler` maps `SIGTERM/SIGINT/SIGHUP/SIGTSTP/SIGCONT` to event classes and emits on the bus; `shutdown=True` triggers `self._shutdown()` | `lib/crewai/src/crewai/telemetry/telemetry.py:176`–`:234` |
| Event-bus dependency graph | `Depends` class + `build_execution_plan` enforce handler DAG with parallelism where independent; `is_replaying` lets listeners skip side effects during replay | `lib/crewai/src/crewai/events/depends.py:43`–`:105`, `lib/crewai/src/crewai/events/handler_graph.py`, `lib/crewai/src/crewai/events/event_bus.py:647`–`:730` |
| Global hook registry | `_before_llm_call_hooks`, `_after_llm_call_hooks`, `_before_tool_call_hooks`, `_after_tool_call_hooks` are global lists; `register_*_hook` appends, `clear_*_hooks` resets | `lib/crewai/src/crewai/hooks/llm_hooks.py:153`–`:341`, `lib/crewai/src/crewai/hooks/tool_hooks.py:1`–`:330` |
| Hook request_human_input | `LLMCallHookContext.request_human_input` and `ToolCallHookContext.request_human_input` block on stdin (synchronous) — a deliberate blocking hook, not async | `lib/crewai/src/crewai/hooks/llm_hooks.py:109`–`:151`, `lib/crewai/src/crewai/hooks/tool_hooks.py:79`–`:127` |
| `before_kickoff_callbacks` / `after_kickoff_callbacks` | Crew-level callback lists executed serially around the kickoff process | `lib/crewai/src/crewai/crew.py:278`–`:289`, `:1046`–`:1049` |
| Runtime scope tracking | Event bus opens `_enter_runtime_scope` / `_exit_runtime_scope` around kickoff so nested kickoffs compose | `lib/crewai/src/crewai/events/event_bus.py:317`–`:335`, `lib/crewai/src/crewai/crew.py:1033`–`:1069`, `lib/crewai/src/crewai/flow/runtime/__init__.py:2197`–`:2207`, `:2313`–`:2318` |
| Rate limiter (timer, not a scheduler) | `RPMController` uses `threading.Timer(60.0, ...)` to reset the per-minute counter — not a general trigger surface | `lib/crewai/src/crewai/utilities/rpm_controller.py:1`–`:89` |
| Event bus flush at kickoff end | `kickoff` drains pending handlers via `await asyncio.gather(*self._event_futures)` so logs/handlers complete before returning | `lib/crewai/src/crewai/flow/runtime/__init__.py:2542`–`:2550`, `:2565`–`:2570`, `lib/crewai/src/crewai/events/event_bus.py:732`–`:790` |
| Replay safety | `_replaying` context var is set during `bus.replay` so listeners (e.g., checkpoint writer) can opt out via `is_replaying()` | `lib/crewai/src/crewai/events/event_bus.py:67`–`:80`, `:671`–`:730` |
| Async background saves | `memory.drain_writes()` runs in the `kickoff` finally block; `MemoryScope.background_save` defers writes | `lib/crewai/src/crewai/crew.py:1065`–`:1069`, `lib/crewai/src/crewai/memory/unified_memory.py` |

## Answers to Dimension Questions

1. **What starts execution?**
   Five orthogonal triggers, all in-process and synchronous at the entry-point level (unless the caller wraps them):
   - Explicit user/caller `kickoff()` / `kickoff_async()` / `kickoff_for_each()` on `Crew`, `LiteAgent`, or `Flow` (`crew.py:980`, `:1074`, `:1110`, `:1164`; `lite_agent.py:477`, `:750`; `flow/runtime/__init__.py:2110`, `:2209`).
   - A2A server execute endpoint `a2a.utils.task.execute` reached via the A2A request handler (`a2a/utils/task.py:290-399`); inside crewAI the local-leg trigger is `_execute_impl` decorated with `@cancellable` (`:115-195`).
   - A polling loop in `PollingHandler.execute` that re-enters A2A `client.get_task` every `polling_interval` seconds until terminal or `polling_timeout`/`max_polls` (`a2a/updates/polling/handler.py:45-120`, `:123-200`).
   - `Flow.handle_turn(message)` / `Flow.chat()` for conversational flows — each turn is a full kickoff with persisted state reload (`experimental/conversational_mixin.py:259-309`, `:311-362`).
   - OS-signal-to-event conversion in `Telemetry._register_shutdown_handlers` for SIGTERM / SIGINT / SIGHUP / SIGTSTP / SIGCONT (`telemetry/telemetry.py:176-234`). These emit events on the bus rather than starting runs directly.
   - The checkpoint listener (`state/checkpoint_listener.py:113-269`) and event bus handlers are *reactive* triggers — they fire on existing events, not new runs.

2. **Are triggers durable?**
   Yes for the flows that opt in via `CheckpointConfig` or `@persist` / `FlowPersistence`; no otherwise.
   - `CheckpointConfig` writes `RuntimeState.model_dump_json()` (a snapshot of the entire `crewai_event_bus.runtime_state.root`, including every Agent/Flow/Crew with its `execution_context`) via `JsonProvider` or `SqliteProvider` (`state/runtime.py:286-350`, `state/checkpoint_config.py:159-233`).
   - `SQLiteFlowPersistence` uses `PRAGMA journal_mode=WAL`, a file lock, and persists per-method rows plus a dedicated `pending_feedback` row for human-in-the-loop suspension (`flow/persistence/sqlite.py:71-296`).
   - `Flow.from_pending()` rebuilds the flow from the `pending_feedback` row + last flow_states row; `Crew.from_checkpoint()` / `Flow.from_checkpoint()` rebuild from a full `RuntimeState` snapshot, with version migrations applied by `_migrate` (`state/runtime.py:89-119`, `:391-498`).
   - Without an explicit opt-in, in-flight `kickoff` calls have no durability — process crash loses them entirely.

3. **Are duplicate triggers safe?**
   Mixed.
   - `SQLiteFlowPersistence.save_pending_feedback` is idempotent for the same flow UUID thanks to `INSERT OR REPLACE` and the `UNIQUE` index on `flow_uuid` (`flow/persistence/sqlite.py:96-112`, `:229-244`).
   - `RuntimeState.checkpoint` always writes a new row keyed off the provider's extracted id; `_chain_lineage` updates `_checkpoint_id` / `_parent_id` so duplicate writes produce duplicate rows (`state/runtime.py:216-228`, `:286-318`).
   - `flow_states` rows are append-only (no per-event dedup key). Restoring the same flow twice on the same `flow_uuid` will replay the last write by `load_state(...)` (`flow/persistence/sqlite.py:178-203`) — that part is safe.
   - `kickoff(inputs={"id": sid})` is *idempotent at the state layer* when `inputs["id"]` matches an existing flow: `load_state` returns the latest snapshot, and `_restore_state` is applied (`flow/runtime/__init__.py:2390-2407`). But the executor does not detect duplicate *kickoff invocations* with the same id and no `from_checkpoint`; it simply runs from scratch and overwrites.
   - `kickoff_for_each` *copies the crew per input* and runs serially, so each input gets a fresh internal state (`crew.py:1095-1103`).
   - `restore_from_state_id` for a missing UUID is a **silent fallback** to baseline — this is unsafe for queue-style callers that expect the operation to actually be a fork (`flow/runtime/__init__.py:2374-2379`).

4. **Can scheduled work be observed like interactive work?**
   Yes, with the same vocabulary.
   - Every `kickoff` emits `CrewKickoffStartedEvent` / `CrewKickoffCompletedEvent` / `CrewKickoffFailedEvent` from the synchronous method (`crew.py:1055-1062`, `:1873+`).
   - Every flow kickoff emits `FlowCreatedEvent` / `FlowStartedEvent` / `FlowPausedEvent` / `FlowFinishedEvent`, with `triggered_by_scope` linking each listener to its triggering event id (`flow/runtime/__init__.py:1069-1076`, `:2434-2461`, `:2505-2554`).
   - Conversational turns emit `ConversationMessageAddedEvent` and `ConversationRouteSelectedEvent` (`experimental/conversational_mixin.py:488-528`).
   - Trace batch finalization can be deferred across turns (`defer_trace_finalization`) so a multi-turn session lands as one trace; `finalize_session_traces()` closes the batch (`conversational_mixin.py:1012-1072`).
   - `_triggering_event_id` contextvar propagates causal chain ID, settable via `set_triggering_event_id()` and inspectable via `get_triggering_event_id()` (`events/event_context.py:53`, `:100-110`, `context.py:95-134`).
   - For `HumanFeedbackPending`-paused flows, the framework auto-creates a default persistence on the first pause and persists both `pending_feedback` and the prior flow_states row (`flow/runtime/__init__.py:2506-2523`).

5. **Do background and foreground execution share semantics?**
   Almost entirely yes — there is one execution engine.
   - `kickoff_async()` is largely `asyncio.to_thread(self.kickoff, ...)` for crew (`crew.py:1143`, `:1162`) and likewise for `LiteAgent.kickoff_async` (`lite_agent.py:750-770`).
   - `Flow.kickoff` runs the async implementation inside a one-worker `ThreadPoolExecutor` when an event loop already exists, else `asyncio.run` (`flow/runtime/__init__.py:2197-2205`).
   - `Crew.a2a`-wrapped `kickoff` and `kickoff_async` reuse the same `_execute_task_with_a2a` / `_aexecute_task_with_a2a` bodies — only the dispatch path differs (`a2a/wrapper.py:118-235`).
   - The A2A server path (`a2a/utils/task.py:305-399`) calls the same `Agent.aexecute_task` synchronously, so semantics are identical to a remote kickoff that ran locally.
   - Polling- and streaming-driven A2A updates call into the same `send_message_and_get_task_id` helper — they produce the same `A2AResponseReceivedEvent` payload (`a2a/task_helpers.py`, `a2a/updates/polling/handler.py:160-200`).
   - Background memory writes (`Memory.remember_many`, `MemoryScope.background_save`) *do* differ: they fire-and-forget to a background asyncio loop and drain in the kickoff `finally` block (`crew.py:1065-1069`, `memory/unified_memory.py`).
   - Async event-bus handlers always run on the daemon `CrewAIEventsLoop` thread, distinct from the caller thread; this is the only true background path and is documented in `event_bus.py:178-189`.

## Architectural Decisions

- **Single-engine runtime, not a hosted scheduler.** Every trigger in CrewAI ultimately funnels into one of `Crew.kickoff`, `Flow.kickoff`, `LiteAgent.kickoff`, or an `Agent` method decorated with the A2A wrapper. There is no scheduler, broker, queue, or webhook surface baked in. Adoption is library-style: bring your own host.
- **Explicit durability opt-in.** Durability is conditional on `CheckpointConfig`, `FlowPersistence`, `@persist`, or `from_pending`. Nothing happens automatically besides the lazy checkpoint listener registration on first checkpoint resolution (`state/checkpoint_listener.py:43-52`) — so non-durable runs are still cheap and casual users never pay for persistence.
- **Two parallel state systems inside flows.** `@persist` writes per-method rows into `SQLiteFlowPersistence` (or any registered `FlowPersistence`) keyed by `flow_uuid`; `CheckpointConfig` writes the whole `RuntimeState.root` snapshot. These are deliberately segregated (`flow/runtime/__init__.py:2139-2144`, `state/checkpoint_config.py:14-142`). Mixing the two raises `ValueError("Cannot combine `from_checkpoint` and `restore_from_state_id`...")`.
- **Sync/async duality via `asyncio.to_thread`.** The runtime mirrors sync and async surfaces but the body of the sync path runs the same code as the async path. Crew's `kickoff_async` literally wraps `self.kickoff`; Flow's `kickoff` runs `kickoff_async` in a worker when an event loop is already running (`crew.py:1143`, `flow/runtime/__init__.py:2199-2205`).
- **`HumanFeedbackPending` as control flow, not failure.** The framework catches it at the outermost level, persists pending state with auto-created default persistence, emits `FlowPausedEvent`, and returns the exception object instead of raising. Documentation explicitly notes this is non-error semantics (`flow/async_feedback/types.py:141-211`, `flow/runtime/__init__.py:2503-2557`).
- **Event bus with dependency DAG and replay.** `CrewAIEventsBus` separates sync / async handlers and tracks `Depends(...)` ordering for deterministic execution plans. A `_replaying` contextvar lets listeners opt out of side effects when their event is replayed (`events/event_bus.py:67-80`, `:570-730`, `events/depends.py:43-105`).
- **Runtime scope nesting.** `_enter_runtime_scope` / `_exit_runtime_scope` are opened around every `kickoff` so nested crew executions are isolated by `crewai_runtime_state` and `crewai_registered_entity_ids` contextvars (`events/event_bus.py:317-335`).
- **Lazy initialization of expensive resources.** The async event loop, sync executor, and checkpoint listener all wait for first use — `CrewAIEventsBus._ensure_executor_initialized` (`event_bus.py:165-189`), `CheckpointListener._ensure_handlers_registered` (`checkpoint_listener.py:43-52`).
- **Versioned checkpoints.** `RuntimeState._migrate` runs migrations on restore, including discriminator backfill for `memory_kind` and `source_type` introduced in 1.14.6 (`state/runtime.py:89-119`). Unknown legacy values raise with an actionable error.
- **Two output-update channels in A2A.** `PollingHandler`, `PushNotificationHandler`, and `StreamingHandler` are sibling modules under `a2a/updates/`. Each encapsulates its own waiting loop, timeouts, and back-off (`a2a/updates/polling/handler.py:45-200`, `a2a/updates/push_notifications/handler.py`, `a2a/updates/streaming/handler.py`).
- **A2A cancellable task contract.** A `@cancellable` decorator races an execution task against a Redis pub-sub subscriber (or a 100ms in-process poll) and raises `asyncio.CancelledError` cleanly when an external cancel request arrives (`a2a/utils/task.py:115-195`).

## Notable Patterns

- **Provider abstraction with discriminator union** (`flow/persistence/base.py` + `state/provider/core.py:10-111`, `JsonProvider` / `SqliteProvider` registered via discriminator in `CheckpointConfig`, `state/checkpoint_config.py:176-182`). Adding a new backend is a one-class change.
- **State lineage tracking** — `parent_id` and `branch` are first-class fields on every `RuntimeState` checkpoint (`state/runtime.py:216-228`, `:352-389`). `fork()` mints a new branch line and bumps `_branch`.
- **Reactive durability listener** registered lazily against the singleton event bus (`state/checkpoint_listener.py:43-269`). Pattern: app code declares `checkpoint=True`, framework wires handler, every event that matches `cfg.on_events` triggers a write.
- **Flow-driven conversational turn pattern** — `_ConversationalMixin.handle_turn` stashes pending turn, calls `kickoff`, promotes assistant message if no listener did (`experimental/conversational_mixin.py:259-309`). Removes the need for a bespoke message-router state machine.
- **Dual-channel A2A update** — push notifications vs polling vs streaming split into sibling modules with consistent `handler.execute(...)` API surface.
- **Resumable human-in-the-loop with auto-persist** — `HumanFeedbackPending` is caught, persisted via auto-created SQLite, returned as a value (`flow/runtime/__init__.py:2503-2557`). Provider authors raise, framework handles persistence.
- **Endpoint key derivation and dedup** — `flow_uuid`-keyed `INSERT OR REPLACE` for the pending feedback row gives free idempotency for re-issuing the same `kickoff` while paused (`flow/persistence/sqlite.py:229-244`).
- **Pre-ask checkpoint** — `Flow.ask()` calls `_checkpoint_state_for_ask` before resolving the input provider so a crash mid-`ask` leaves recoverable state (`flow/runtime/__init__.py:3273-3296`).
- **Lazy import and resolve** — `kickoff_event_id` propagation via private attribute `_kickoff_event_id` and public `checkpoint_kickoff_event_id` field; the restart path restores both (`crew.py:216`, `:506-507`, `state/runtime.py:52-87`).
- **`is_replaying()` opt-out for side-effect listeners** so re-emitting recorded events during entity rehydration does not double-write checkpoints (`events/event_bus.py:67-80`, `:671-730`).

## Tradeoffs

- **No scheduler at all.** A team wanting cron or delayed triggers must run their own scheduler (Celery beat, k8s CronJob, systemd timer) and have it `subprocess.run(["python", "-c", "crew.kickoff()"])` or `import` and call. There is no in-process delay, no per-second or per-minute granularity, no DAG-aware scheduler. (`grep -rn 'cron\|apscheduler\|aiocron'` in `lib/crewai/src/crewai` returns no matches beyond `kickoff_event_id` style string usage.)
- **Sync/async duality via `to_thread` is cheap but loses stack trace continuity.** Errors thrown inside `asyncio.to_thread(self.kickoff, ...)` lose the original asyncio context (`crew.py:1143`, `:1162`).
- **Silent fallback for missing state on `restore_from_state_id`** trades safety for ergonomics. A queue-driven caller cannot tell whether a job actually forked or ran from scratch (`flow/runtime/__init__.py:2374-2379`).
- **Append-only checkpoint rows in SQLite.** Without `max_checkpoints` the DB grows unbounded. `max_checkpoints` only prunes per-branch, so a runaway branch can still fill disk (`state/provider/sqlite_provider.py`, `state/checkpoint_config.py:183-187`).
- **`@persist` decorator is metadata-only.** It stamps `__flow_persistence_config__` on the class/method; runtime support for the actual write happens because the Flow engine reads the metadata during method execution. If the engine ever changes the way methods are executed, every `@persist` site silently stops persisting (`flow/persistence/decorators.py:147-191`, `flow/runtime/__init__.py:2935-2965`).
- **`depends.py` is named like FastAPI's dependency system but is unrelated.** Search hits return a handler-ordering tool, not an HTTP dependency (`events/depends.py:1-105`).
- **A2A cancellation relies on a shared cache key** — `cancel:{task_id}`. Two parallel requests sharing `task_id` could cross-cancel each other; the cache call uses a 0.1s polling fallback which adds latency for local-only deployments (`a2a/utils/task.py:147-189`).
- **Hooks are global, not per-instance.** `register_before_llm_call_hook` and `register_before_tool_call_hook` mutate module-level lists and affect every agent, every run, until cleared (`hooks/llm_hooks.py:153-218`, `hooks/tool_hooks.py:1-330`). Multi-tenant code needs to clean up explicitly.
- **`kickoff_for_each` is sequential.** It calls `crew.copy()` then `kickoff` per input in a loop; parallel fan-out requires `kickoff_for_each_async` and the runtime configuration to actually use `asyncio.gather` (`crew.py:1095-1108`, `:1338-1417`).
- **Conversational traces rely on opt-in `defer_trace_finalization`.** If a developer forgets, each `handle_turn()` closes its own trace batch and the conversation appears as N separate runs in the trace UI (`experimental/conversational_mixin.py:681-696`, `conversational_mixin.py:1012-1072`).
- **CLI was extracted.** The framework itself does not own a CLI — calling `crewai run` or scheduling from the shell requires the deprecated `crewai_cli` shim or a custom entry point (`lib/crewai/src/crewai/cli/__init__.py:1-74`).
- **`Flow.from_checkpoint` re-imports the user's flow class.** Tools with non-deterministic `__init__` or constructor side-effects may re-run during restoration (`flow/runtime/__init__.py:866-911`).

## Failure Modes / Edge Cases

- **Process crash during `kickoff` without checkpoint** loses the in-flight run. There is no implicit journaling of progress without `CheckpointConfig` or `@persist`.
- **`HumanFeedbackPending` without `persistence` configured** auto-creates a default `SQLiteFlowPersistence` (`flow/runtime/__init__.py:2506-2512`). This means a flow author who never opted in can still get persistence behavior as a side effect — surprising for tests that expect ephemeral state.
- **`restore_from_state_id` target absent** — the framework silently continues. A retry-from-queue pattern that depends on fork semantics will silently run from scratch (`flow/runtime/__init__.py:2374-2379`).
- **Polling timeout bursts** — `PollingHandler` raises `A2APollingTimeoutError` after `polling_timeout` OR `max_polls`; tests/users must tune both to avoid spurious failures (`a2a/updates/polling/handler.py:110-118`).
- **State ID collision across `from_pending`** — there is no namespace separation across flow classes. Two different Flow classes using the same `flow_uuid` collide in `pending_feedback` (`flow/persistence/sqlite.py:96-112`).
- **`Telemetry` signal handlers** emit events but `kickoff` does not wait for them — fast `SIGTERM` may race against in-flight runs that have not yet checkpointed (`telemetry/telemetry.py:176-234`).
- **Nested kickoff scope** — re-entering `kickoff` on the same `Flow` instance opens a new runtime scope; `_usage_aggregation_handler` is gated by `owns_usage_aggregation` so only the outermost call aggregates, but sibling parallel kickoffs under the same parent context share `current_flow_id` and can double-count (`flow/runtime/__init__.py:2313-2322`, `:1162-1191`).
- **`Cyclic flow` resumptions** — the framework explicitly clears `_completed_methods` and `_clear_or_listeners()` so listeners re-fire on resume; stale cached `_racing_groups_cache` is invalidated but `max_method_calls` is enforced per-flow-class, not per-execution (`flow/runtime/__init__.py:3204-3211`).
- **Two consumers compete for the same `kickoff_event_id`** — the `_kickoff_event_id` is shared on a `Crew` instance, not per-restore; the latest `Checkpoint` snapshot wins on `from_checkpoint` restoration (`crew.py:216`, `:506-507`).
- **A2A server cancel** — Redis pubsub + polling fallback can mask cancel for ~0.1s; a task that completes in that window gets `cache.delete` cleanup but the cancellation signal arrives too late (`a2a/utils/task.py:147-193`).
- **`@persist`-without-persistence-configured** flow silently no-ops in `_checkpoint_state_for_ask` and `__flow_persistence_config__` resolution; the lack of an explicit warning makes configuration drift easy to miss (`flow/runtime/__init__.py:3273-3296`).
- **Event bus shutdown** during `emit` returns `None` and prints a console warning; callers that do not null-check may silently miss checkpoints (`events/event_bus.py:600-606`).
- **`kickoff_event_id` from older versions** triggers discriminator backfill paths that can silently coerce memory kind / source type; legacy payloads without `source_type` raise with a manual remediation message (`state/runtime.py:115-150`).
- **Hooks that throw** are not isolated; global `_before_llm_call_hooks` raise into the executor and surface as `LiteAgentExecutionErrorEvent` (`lite_agent.py:539-545`, `hooks/llm_hooks.py:170-218`).
- **Streaming paths** in `kickoff` race cleanup with `signal_end`; on exceptions the streaming_output falls back to a `signal_error` then continues to deliver chunks from `output_holder`, which can leak partial state if iterated after failure (`flow/runtime/__init__.py:2163-2188`).

## Future Considerations

- The library can grow a `BaseTrigger` abstraction (mirroring `BaseProvider`) so cron, queue, webhook, and signal triggers compose with the same `kickoff` body. The current scattered code across `crew.py`, `flow/runtime/__init__.py`, `a2a/wrapper.py`, `a2a/utils/task.py`, `hooks/`, `state/checkpoint_listener.py`, `telemetry/telemetry.py`, and `conversational_mixin.py` makes this hard.
- A first-class idempotency key per kickoff — combined with the `pending_feedback` insert-or-replace pattern — would let queue retries dedupe. Today, callers must hand-construct `flow_uuid` and guard against double-paused states.
- An explicit durable queue / broker abstraction (similar to `agent-framework-durabletask`) could replace the current reliance on the caller to thread checkpoint state across processes.
- A scheduler surface — `Flow.schedule(cron=*/5 * * * *)` or `@cron` / `@interval` decorators — would close the largest gap. The CLI is already external (`lib/crewai/src/crewai/cli/__init__.py:19-27`); doing the scheduler as a port of that extraction would be consistent.
- `restore_from_state_id` should return a `Result` or raise on miss instead of falling back silently — the safety profile changes once callers trust it for queue semantics.
- Hook isolation per `Flow` / `Agent` / `Crew` instance would let multi-tenant processes register without polluting global state (`hooks/llm_hooks.py:153-218`).
- A formal outbox/transactional pattern for "kickoff + checkpoint" so that a kickoff failure between dispatch and checkpoint does not strand the caller. `RuntimeState.checkpoint` writes synchronously after the kickoff starts emitting, but there is no two-phase commit with a downstream call.
- Extend `RuntimeState._migrate` to warn when restoring across major versions, not just patch. Today it only debug-logs (`state/runtime.py:108-114`).

## Questions / Gaps

- **Is there a recommended production cron trigger pattern?** No evidence found inside the selected source — only the `crewai_cli` shim remains. Search boundary: `lib/crewai/src/crewai/**`. The framework defers to the host.
- **Is there any queue/worker integration (Celery, RQ, SQS, PubSub)?** No evidence found inside the selected source. Search boundary: `lib/crewai/src/crewai/**`. External integrations live in companion packages outside the scope.
- **Is there a webhook receiver / HTTP server for incoming triggers?** No evidence found inside the selected source. A2A executes when *another* service calls `a2a.utils.task.execute`, but no `app.post("/kickoff")` route is exposed.
- **Is there idempotency on duplicate kickoffs with the same `inputs["id"]`?** Partial — `load_state` returns the latest snapshot for the id, but the executor does not detect duplicate invocations. Search boundary: `lib/crewai/src/crewai/flow/runtime/__init__.py` lines `2381-2407` and `2952-2968`.
- **Are global hooks safe to use in long-lived processes?** No process-level isolation is enforced; `clear_all_llm_call_hooks` / `clear_all_tool_call_hooks` returns counts but does not document ownership (`hooks/llm_hooks.py:293-341`, `hooks/tool_hooks.py:270-330`).
- **How does `Flow.ask` behave under signal handlers?** The framework returns `None` on timeout, but provider code is run inside `ThreadPoolExecutor` with `wait=False` and `cancel_futures=True`, so a stuck provider call may leak a background thread (`flow/runtime/__init__.py:3382-3420`). No clear evidence of a bounded shutdown.
- **Where do deferred pending writes drain on process exit?** Evidence inside `kickoff` finally blocks for `memory.drain_writes()` (`:1065-1069`) and `_pending_futures` flush (`event_bus.py:732-790`), but no evidence of an `atexit` hook that drains a partial execution.
- **Can `Flow.fork` co-exist with `restore_from_state_id`?** The code explicitly forbids combining `from_checkpoint` and `restore_from_state_id` (`:2139-2144`, `:2239-2244`), but a manual fork followed by `restore_from_state_id` is not documented.
- **Is there a Tenant / Caller-id / Auth surface on trigger entry?** `set_platform_integration_token` carries a single string but the kickoff entry points never validate it (`:30-48`, `:120-134`). Authentication is delegated to the surrounding shell.

---

Generated by `lib/crewai/src/crewai/__init__.py`-relative dimension `01.06-scheduling-and-trigger-semantics` against `crewai`.

# Source Analysis: letta

## 02.08 Crash Recovery and Reconstruction

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python 3.11+ (FastAPI + Uvicorn/Granian, SQLAlchemy 2.x async, asyncpg/SQLite, APScheduler, Redis optional, Anthropic/OpenAI clients, Lettuce external executor optional) |
| Analyzed | 2026-07-10 |

## Summary

Letta's recovery model is split across three layers and the boundaries between them are uneven:

1. **In-process run lifecycle** is the cleanest piece. Every foreground/background run is materialized as a row in `runs` (RunModel ORM, created at `letta/services/run_manager.py:48-90`) plus a parallel `run_metrics` row, and the agent loop "checkpoints" every successful step by persisting new messages and updating in-context message IDs in the same transaction (`letta/agents/letta_agent_v3.py:758-816`). On a hard crash the row stays at status `created` or `running`, but the message table already holds everything that was successfully produced. There is **no periodic sweeper** that reaps "orphaned" `running` rows; an operator is expected to call `cancel_run` on stuck runs (`letta/services/run_manager.py:619-783`, exposed at `letta/server/rest_api/routers/v1/agents.py:1948`), and the cancel path itself is idempotent (`run_manager.py:649-654`). The streaming layer is explicit about treating "stream ended without [DONE]" as a failure and synthesizes a `stream_incomplete` error + sets the run to `failed` (`letta/services/streaming_service.py:610-629`, `letta/server/rest_api/redis_stream_manager.py:284-315`), and treats asyncio task cancellation as either an explicit cancellation (run status was already `cancelled`) or an interrupt (`stream_task_cancelled`, `redis_stream_manager.py:317-377`). The run lifecycle in `update_run_by_id_async` is carefully guarded against re-entering a completed run (`run_manager.py:342-356`) and forces `completed_at` to be set on the first terminal transition (`run_manager.py:358-364`), making double-completion impossible from the manager layer.

2. **Background/async path** uses asyncio tasks wrapped with `safe_create_shielded_task` (`letta/utils.py:1247-1279`) so a parent cancellation does not interrupt processing, and registers a `task.add_done_callback` (`letta/server/rest_api/routers/v1/agents.py:2286-2315`) that, on unhandled exception, schedules a `update_failed_run()` so the DB row still terminates. The `StreamingService`/`RedisSSEStreamWriter` pair decouples client lifetime from background work: chunks are flushed into a Redis stream keyed `sse:run:{run_id}` with TTL ~3 h (`letta/server/rest_api/redis_stream_manager.py:42-58`, `redis_stream_manager.py:132-157`) and the original HTTP request can die without losing the run's state. Cursor-based recovery from Redis is supported (`redis_stream_manager.py:463-526`) and exposed at `/v1/conversations/{cid}/stream` (`letta/server/rest_api/routers/v1/conversations.py:690-820`), with OTID -> run_id mapping kept in Redis for "duplicate request recovery" (`letta/services/streaming_service.py:83-117`, `letta/server/rest_api/routers/v1/conversations.py:366-368`). Conversation locks have a 300 s TTL (`letta/constants.py:475`) so a crashed lock holder eventually unblocks.

3. **Cross-pod / scheduler recovery** uses PostgreSQL advisory locks at key `0x12345678ABCDEF00` (`letta/jobs/scheduler.py:18`) for the LLM-batch poller. A single leader is elected at lifespan startup (`letta/jobs/scheduler.py:25-99`); non-leaders run a retry task (`scheduler.py:108-133`). On PostgreSQL this is safe; the code falls back to "everyone is leader" on SQLite (`scheduler.py:41-43`) and warns about it. The poller itself reads `JobStatus.running` rows from the DB (`letta/services/llm_batch_manager.py:211-229`), queries Anthropic for status, updates job rows in bulk, and resumes agent execution for newly completed batches via `LettaAgentBatch.resume_step_after_request` (`letta/jobs/llm_batch_job_polling.py:170-247`, `letta/agents/letta_agent_batch.py:233-270`). This is the only explicitly modeled resume flow in the codebase — and it only covers Anthropic Message Batches, not normal runs.

What Letta does **not** do: there is no periodic sweeper that detects `status='running'` rows whose worker is gone; no auto-retry/requeue of failed runs; no idempotency token for tool calls beyond Lettuce/external executor design (the in-process tool-execution helpers in `letta/services/tool_executor/` are not consulted here); no dead-letter queue. The Lettuce external executor (`letta/services/lettuce/lettuce_client_base.py:9-101`) is a stub in OSS that returns `None`, but cloud `get_status` is the recovery boundary when Lettuce is enabled (`letta/services/run_manager.py:103-130`).

## Rating

**6 / 10** — Clear, test-covered model for in-process cancellation, stream replay, and Anthropic batch resumption; explicit, logged treatment of stream-incomplete and stream-task-cancelled paths; observability hooks (readiness state, event-loop watchdog, Sentry). Missing: any periodic sweeper for orphaned `running` rows on Postgres, no DLQ, no automatic requeue of failed runs, batch-resume logic only covers Anthropic and only covers LLM batch jobs (not regular runs), and Lettuce-based recovery is cloud-only.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Run ORM model | `status` enum + terminal-state lifecycle; `completed_at` is nullable | `letta/schemas/enums.py:152-161` |
| Run schema | `status`, `created_at`, `completed_at`, `callback_url`, `total_duration_ns` | `letta/schemas/run.py:17-61` |
| Run creation | creates RunModel + RunMetricsModel in one transaction | `letta/services/run_manager.py:48-90` |
| Run terminal guard | refuses to overwrite `completed`/`failed`/`cancelled` | `letta/services/run_manager.py:342-356` |
| Run terminal housekeeping | auto-fills `completed_at`, dispatches callback once | `letta/services/run_manager.py:358-481` |
| Run cancel (idempotent) | early-exits if already terminal; cleans up approval-pending tool calls | `letta/services/run_manager.py:619-783` |
| Cancel API path | HTTP route falls back from Redis `agent:send_message:run_id` to DB query | `letta/server/rest_api/routers/v1/agents.py:1900-1956` |
| In-flight cancellation check (per step) | `_check_run_cancellation` polled at step boundary | `letta/agents/letta_agent_v3.py:1030-1035` |
| Same, v2 path | `_check_run_cancellation` from `run_manager` | `letta/agents/letta_agent_v2.py:506-512`, `letta/agents/letta_agent_v2.py:750-757` |
| Same, legacy v1 | `_check_run_cancellation` (no arg, relies on instance state) | `letta/agents/letta_agent.py:155-160`, `letta/agents/letta_agent.py:285-286`, `letta/agents/letta_agent.py:634-635`, `letta/agents/letta_agent.py:984-985` |
| Background task wrapping | `safe_create_shielded_task` prevents parent cancellation killing the run | `letta/utils.py:1247-1279` |
| Async send-message background task | shielded task + `add_done_callback` that marks `failed` on unhandled exception | `letta/server/rest_api/routers/v1/agents.py:2265-2315` |
| Stream "ended without DONE" detection | synthesizes `stream_incomplete` error + `[DONE]`, marks run failed | `letta/services/streaming_service.py:610-629`, `letta/services/streaming_service.py:777-808` |
| Background stream processor | explicit handling of `RunCancelledException` vs `asyncio.CancelledError`; synthesizes `stream_incomplete` / `stream_task_cancelled` | `letta/server/rest_api/redis_stream_manager.py:284-460` |
| Streaming cancellation wrapper | polls `run_manager.get_run_by_id` every 0.5 s; sets asyncio.Event on `cancelled` status | `letta/server/rest_api/streaming_response.py:33-229` |
| Per-run cancellation event registry | `_cancellation_events` dict; intentionally never cleaned up | `letta/server/rest_api/streaming_response.py:31-40` |
| StreamingResponse shield | `asyncio.shield` keeps background task alive across client disconnect | `letta/server/rest_api/streaming_response.py:244-264` |
| Redis SSE stream TTL | 10 800 s default + max length cap | `letta/server/rest_api/redis_stream_manager.py:42-58` |
| Cursor-based stream recovery | `redis_sse_stream_generator(starting_after=...)` | `letta/server/rest_api/redis_stream_manager.py:463-526` |
| Duplicate-request recovery via OTID | `try_recover_duplicate_request` returns existing run stream | `letta/services/streaming_service.py:83-117` |
| Duplicate-request recovery in second-chance lock failure | polled up to 3× with backoff | `letta/services/streaming_service.py:283-319` |
| `/conversations/{cid}/stream` resume endpoint | priority 1 direct run_id, priority 2 OTID lookup, priority 3 active-run lookup | `letta/server/rest_api/routers/v1/conversations.py:690-820` |
| Conversation lock TTL | `CONVERSATION_LOCK_TTL_SECONDS = 300` | `letta/constants.py:474-475` |
| Run active endpoint | filters `RunStatus.created/running` | `letta/server/rest_api/routers/v1/runs.py:117-140` |
| Anthropic batch leader election | `pg_try_advisory_lock` on `0x12345678ABCDEF00` | `letta/jobs/scheduler.py:18-99` |
| Non-leader retry loop | polls every `poll_lock_retry_interval_seconds` | `letta/jobs/scheduler.py:108-133` |
| Poller shutdown | releases advisory lock and shuts down APScheduler | `letta/jobs/scheduler.py:185-228` |
| Polling job | polls Anthropic batches, fetches item results, resumes agents | `letta/jobs/llm_batch_job_polling.py:170-247` |
| Resume step after batch | `LettaAgentBatch.resume_step_after_request` | `letta/agents/letta_agent_batch.py:233-270` |
| `JobStatus` enum | `pending/running/completed/failed/cancelled/expired` + `is_terminal` property | `letta/schemas/enums.py:131-149` |
| Batch-status mapping (failure as expired) | unknown result types collapse to `expired` | `letta/agents/letta_agent_batch.py:356-365` |
| Database session hardening | rollback on `asyncio.CancelledError`; closes idle transactions | `letta/server/db.py:73-117` |
| Async session factory | `expire_on_commit=False` for post-commit attribute access | `letta/server/db.py:61-67` |
| Lettuce cloud recovery (status passthrough) | `run_manager.get_run_with_status` queries Lettuce for non-terminal runs | `letta/services/run_manager.py:103-130` |
| Lettuce cancellation | separate API path so Lettuce-side state is cleared | `letta/server/rest_api/routers/v1/agents.py:1940-1947` |
| Lifespan startup hooks | readiness_state = warming → ready, event-loop watchdog starts | `letta/server/rest_api/app.py:175-258` |
| Lifespan shutdown hooks | draining → scheduler shutdown → DB teardown | `letta/server/rest_api/app.py:260-291` |
| Init provider sync (idempotent) | safe for multi-pod startup | `letta/server/server.py:374-502` |
| Readiness state machine | warming/ready/draining/degraded/manual_disable; multi-source gating | `letta/monitoring/readiness_state.py:8-84` |
| Event-loop watchdog | separate thread; toggles `degraded` after `degraded_stabilization_seconds` (30 s), recovers after `recovery_stabilization_seconds` (15 s) | `letta/monitoring/event_loop_watchdog.py:26-100`, `letta/monitoring/event_loop_watchdog.py:188-261` |
| Load gates | FG/BG/admission counters + event-loop lag drive readiness | `letta/monitoring/load_gate.py:9-230` |
| Settings: readiness tuning | stabilization windows default to 30 s / 15 s | `letta/settings.py:600-637` |
| Global exception handlers | `sys.excepthook`, `threading.excepthook`, `loop.set_exception_handler` | `letta/server/global_exception_handler.py:14-108` |
| Database lifecycle (graceful close) | `close_db()` for shutdown | `letta/server/db.py:136-139` |
| Container startup script | waits for Postgres/Redis, runs alembic upgrade head, traps EXIT cleanup | `letta/server/startup.sh:1-104` |
| Container startup hook comments | `init_async` is called from lifespan after migrations | `letta/server/server.py:374-426` |
| Test: run lifecycle (create/get/list/update/delete) | 55 tests including `test_update_run_auto_complete`, `test_run_callback_only_on_terminal_status`, `test_get_run_with_status_*` | `tests/managers/test_run_manager.py:32-2085` |
| Test: cancellation | `tests/managers/test_cancellation.py:1-1416`, `tests/integration_test_cancellation.py:1-208` |
| Test: duplicate-request / OTID stream recovery | `tests/integration_test_conversations_sdk.py:1796-1960` |
| Test: batch resume (some stop / all continue) | `tests/test_letta_agent_batch.py:615-790` |
| Test: event-loop watchdog | `tests/test_watchdog_hang.py` (module-level scenario file) |
| Pending tool-call handling | `_maybe_get_pending_tool_call_message` walks last 3 messages to find assistant tool_calls not requiring approval (HITL parallel-tool case) | `letta/agents/helpers.py:530-543`, `letta/agents/letta_agent_v3.py:990-993` |
| Cancel cleans up approval pending state | writes denials + tool-return messages for all pending tool calls | `letta/services/run_manager.py:671-781` |
| Approval-request step_id fallback | old approval messages without `step_id` get a fresh step | `letta/agents/letta_agent_v3.py:1019-1027` |
| Stream finalizer belt-and-suspenders | appends terminal `[DONE]` even if prior chunk set `complete` | `letta/server/rest_api/redis_stream_manager.py:447-460` |
| Callback only on first terminal | guards callback_sent_at to fire exactly once | `letta/services/run_manager.py:332-340`, `letta/services/run_manager.py:451-481` |
| DB error mapping | OperationalError → 503, DeadlockDetected → 409 with `Retry-After: 1` | `letta/server/rest_api/app.py:609-649` |
| Manual disable flag | runs can be marked terminal by external code; SSE cleanup hook | `letta/server/rest_api/redis_stream_manager.py:156-191` |

## Answers to Dimension Questions

1. **What happens to in-progress work after restart?**
   - The DB row remains at `created` or `running` forever — there is no startup-time scan that resolves these states. The agent loop checkpoints per-step message persistence (`letta/agents/letta_agent_v3.py:758-816`), so any messages from completed steps survive. Stream chunks buffered in Redis (`sse:run:{run_id}`) survive for ~3 h (`letta/server/rest_api/redis_stream_manager.py:42-58`), so a client reconnecting via `/v1/conversations/{cid}/stream` (`letta/server/rest_api/routers/v1/conversations.py:690-820`) can resume reading. Conversation locks auto-expire at 300 s (`letta/constants.py:475`). The Anthropic batch poller will detect newly-completed batches and call `LettaAgentBatch.resume_step_after_request` (`letta/agents/letta_agent_batch.py:233-270`), which is the only built-in agent-execution resume path.

2. **Are orphaned runs detected?**
   - Not automatically. There is no scheduled job that selects `runs WHERE status IN ('created','running') AND updated_at < now() - threshold AND not in lettuce`. The only detection surface is the `/v1/runs/active` endpoint (`letta/server/rest_api/routers/v1/runs.py:117-140`) which lists them, and the Lettuce external executor which can return a definitive status via `LettuceClient.get_status` (`letta/services/run_manager.py:103-130`). Operators must call `cancel_run` (`letta/server/rest_api/routers/v1/agents.py:1948`) on stuck rows; that path is idempotent and safe to call repeatedly (`run_manager.py:649-654`).

3. **Is recovery automatic?**
   - Partially. Stream replay, OTID duplicate-request recovery, Anthropic batch resumption, and Lettuce status passthrough are automatic. In-process crash recovery of a normal agent run is **not** automatic: the shielded asyncio task is killed when the worker dies, the DB row stays `running`, and the only path back is operator intervention or the next caller's `/conversations/{cid}/stream` reconnect.

4. **Is recovery idempotent?**
   - `cancel_run` is idempotent — second call on a terminal run no-ops (`run_manager.py:649-654`). `update_run_by_id_async` is guarded against re-entering a terminal state (`run_manager.py:342-356`). The Anthropic batch poller re-tries transient errors as if still `running` (`letta/jobs/llm_batch_job_polling.py:60-62`). `_dispatch_callback_async` only fires when transitioning from non-terminal to terminal (`run_manager.py:332-340`). The `try_recover_duplicate_request` is safe to call repeatedly (`letta/services/streaming_service.py:83-117`). The stream finalizer's forced `[DONE]` write is explicitly documented as safe (`letta/server/rest_api/redis_stream_manager.py:447-450`).

5. **Are users/operators notified?**
   - Callback URL is POSTed once per run terminal transition with status / completed_at / metadata (`letta/services/run_manager.py:484-510`). Sentry captures unhandled exceptions and `stream_task_cancelled` events (`letta/services/streaming_service.py:776`, `letta/server/rest_api/streaming_response.py:362`). Logs include `Worker {worker_id}` prefixes for every lifespan transition (`letta/server/rest_api/app.py:175-291`). Metrics: `readiness_state_gauge`, `in_flight_background_counter`, `event_loop_lag_ms_histogram` are emitted to OTLP / Datadog (`letta/otel/metric_registry.py:248-253`, `letta/monitoring/event_loop_watchdog.py:97-100`). There is **no built-in alerting** for orphaned `running` rows — operators must wire that to the metrics/logs themselves.

## Architectural Decisions

- **Two-state run lifecycle (`running` ↔ terminal)** with `created` as a transient pre-execute state. Terminal guards live in `RunManager.update_run_by_id_async` (`run_manager.py:342-356`) rather than in the ORM. This prevents accidental terminal writes but means the DB itself does not enforce state machines (the schema at `letta/orm/run.py` does not declare a CHECK or trigger).
- **Per-step message checkpoint inside a single SQLAlchemy session**, gated by `_checkpoint_messages` (`letta/agents/letta_agent_v3.py:758-816`). The only place messages are persisted — guarantees that after every successful step the message state is durable even if the next step crashes.
- **Redis-backed SSE buffer** as the cursor-recovery substrate: chunks are written to a Redis stream with TTL (`letta/server/rest_api/redis_stream_manager.py:42-58`, `redis_stream_manager.py:132-157`), and `redis_sse_stream_generator(starting_after=...)` allows clients to resume from any seq id (`redis_stream_manager.py:463-526`).
- **PostgreSQL advisory lock for batch-poll leader election** (`letta/jobs/scheduler.py:18-99`). Single-leader pattern via `pg_try_advisory_lock`; non-leaders loop on `poll_lock_retry_interval_seconds` (`scheduler.py:108-133`). On SQLite the code intentionally logs a warning and runs scheduler on every instance (`scheduler.py:41-43`).
- **Shielded asyncio tasks** (`letta/utils.py:1247-1279`) for background runs so client disconnects don't kill in-flight work. `done_callback` (`letta/server/rest_api/routers/v1/agents.py:2286-2315`) ensures failed shielded tasks still mark the run `failed`.
- **Cancellation as DB-flag polling** (`letta/agents/letta_agent_v2.py:750-757`) plus asyncio.Event propagation (`letta/server/rest_api/streaming_response.py:33-194`). Two paths exist for legacy reasons; both ultimately rely on `run_manager.get_run_by_id` to detect the cancellation.
- **Readiness state machine with multi-source gating** (`letta/monitoring/readiness_state.py:8-84`) so the same pod can be marked `degraded` by event-loop lag (watchdog), load (FG/BG counters), or admission wait (`letta/monitoring/load_gate.py:9-230`).
- **Graceful shutdown drains Redis SSE buffers, releases advisory lock, disposes DB engine** (`letta/server/rest_api/app.py:260-291`, `letta/jobs/scheduler.py:185-228`, `letta/server/db.py:136-139`).

## Notable Patterns

- **Belt-and-suspenders [DONE] finalizer** in the background stream processor (`letta/server/rest_api/redis_stream_manager.py:447-460`) so SDKs that wait for an explicit `[DONE]` always see one.
- **Status synthesis on abnormal stream termination**: when the upstream async generator exits without `[DONE]` and without `event: error`, the code writes a synthetic `LettaStopReason(stop_reason=StopReasonType.error)` followed by an error event and `[DONE]` (`letta/services/streaming_service.py:610-629`, `letta/server/rest_api/redis_stream_manager.py:284-315`).
- **Background task metadata embedding** (`letta/server/rest_api/routers/v1/agents.py:2230-2235`, `letta/services/streaming_service.py:336-347`): `run_type` and `lettuce` flag are written into `metadata` at run creation so the cancel path and external-executor status passthrough can branch.
- **Idempotent Lettuce cancel + best-effort error swallowing** (`letta/server/rest_api/routers/v1/agents.py:1940-1952`): cancel failures are logged but never propagated to the client.
- **Per-run asyncio.Event registry without cleanup** (`letta/server/rest_api/streaming_response.py:31-40`): events are intentionally kept forever ("we don't bother cleaning them up"), trading memory for simplicity.
- **Stream-task-cancelled vs explicit-cancellation disambiguation** (`letta/server/rest_api/redis_stream_manager.py:317-377`): the processor inspects `run.status` after `asyncio.CancelledError` to decide whether to write a `cancelled` terminal or a `stream_task_cancelled` error.

## Tradeoffs

- **No orphan sweeper** simplifies the design but means a worker crash leaves rows in `running` indefinitely until cancelled. With Lettuce disabled, a restarted pod will never pick those runs back up.
- **Shielded tasks** decouple processing from HTTP lifetime at the cost of potential resource leaks (the `_cancellation_events` dict, the global `_background_tasks` set in `letta/utils.py:1131-1162`).
- **Cursor-based stream replay via Redis** is lightweight and observable but couples Letta to Redis availability. When Redis is down, background streaming raises `LettaServiceUnavailableError` (`letta/services/streaming_service.py:381-388`); foreground streaming falls back to non-buffered mode.
- **Anthropic batch resume covers only Anthropic** (`letta/jobs/llm_batch_job_polling.py:196-197`); OpenAI Batch, Gemini Batch, etc. have no equivalent `resume_*` flow in OSS.
- **Stream-incomplete as `failed`** (`letta/services/streaming_service.py:628-629`) is conservative — a transient client-side disconnect with a recoverable run is marked failed unless explicit cancellation was already recorded (`streaming_service.py:737-748`).
- **One global asyncio.Event per run** (`letta/server/rest_api/streaming_response.py:33-40`) is the simplest cross-task signaling mechanism; multi-pod scenarios need Redis-backed events instead, but Redis is not used for cancellation signaling here.

## Failure Modes / Edge Cases

- **Worker SIGKILL during agent loop step**: step's message checkpoint already persisted (good); the run row stays `running`. There is no automatic requeue. Operator must cancel and resubmit. Lettuce can recover status if enabled (`run_manager.py:103-130`).
- **Worker SIGKILL during tool execution**: tool result is not persisted; the run row stays `running`. The agent loop's `_step_checkpoint_start` writes a `Step` row before tool execution (`letta/agents/letta_agent_v2.py:941-1015`) so the operator can see at least where it stopped.
- **Client disconnect during foreground streaming**: `StreamingResponse.stream_response` shields the task (`letta/server/rest_api/streaming_response.py:244-264`), so the run completes in the background and updates the DB.
- **Client disconnect during background streaming**: chunks continue to flush to Redis (`letta/server/rest_api/redis_stream_manager.py:72-130`); client can reconnect via `/v1/conversations/{cid}/stream` (`letta/server/rest_api/routers/v1/conversations.py:690-820`).
- **Redis outage during background streaming**: `LettaServiceUnavailableError` (`letta/services/streaming_service.py:381-388`); foreground streaming still works because it doesn't depend on Redis for chunk buffering.
- **Duplicate request with same OTID**: returns the existing stream from Redis (`letta/services/streaming_service.py:283-295`) — verified by `tests/integration_test_conversations_sdk.py:1796-1900`.
- **PostgreSQL restart while leader holds advisory lock**: `pg_try_advisory_lock` is session-scoped; on Postgres restart the lock is released. The new pod's `start_scheduler_with_leader_election` will acquire it (`letta/jobs/scheduler.py:25-80`). No requeue needed for the poller because `list_running_llm_batches_async` queries the DB fresh (`llm_batch_manager.py:211-229`).
- **Anthropic batch expires mid-poll**: `JobStatus.running` → `JobStatus.expired` mapping (`letta/agents/letta_agent_batch.py:356-365`); the poller keeps polling but no item results are produced (`letta/jobs/llm_batch_job_polling.py:60-62`).
- **asyncio.CancelledError mid-stream**: distinguished from explicit cancellation via `run.status` lookup (`letta/server/rest_api/redis_stream_manager.py:332-352`).
- **Approval pending at cancel time**: cancel writes denial messages + tool-return messages for ALL pending tool calls (not just approval-required ones) (`letta/services/run_manager.py:671-781`), preventing an agent from being "stuck" with unanswered tool calls.
- **Old approval messages with no `step_id`**: a fresh `step_id` is generated and the step is checkpointed (`letta/agents/letta_agent_v3.py:1019-1027`), preventing a crash on stale approval data.
- **HITL with parallel tool calling**: `_maybe_get_pending_tool_call_message` walks the last three messages to find assistant tool calls that don't require approval, so they can be re-attached to the next approval response (`letta/agents/helpers.py:530-543`, `letta/agents/letta_agent_v3.py:990-993`).
- **Database deadlock during run update**: mapped to HTTP 409 with `Retry-After: 1` (`letta/server/rest_api/app.py:622-649`); the run row is not corrupted because every `RunManager.update` commits through `db_registry.async_session` (`letta/server/db.py:73-117`).
- **Database statement timeout**: mapped to HTTP 503 (`letta/server/rest_api/app.py:666-675`); background stream finalizer still appends `[DONE]` (`letta/server/rest_api/redis_stream_manager.py:447-460`).
- **Pod restart mid-poll**: APScheduler state is in-memory only; after restart, `start_scheduler_with_leader_election` re-acquires the advisory lock and the new poll resumes from the current DB state (`scheduler.py:25-99`).

## Future Considerations

- **Periodic orphan-run reaper**: a scheduled job that selects `runs WHERE status='running' AND updated_at < now() - interval '1 hour'` and either reaps or notifies. Currently absent.
- **Generic batch poller for non-Anthropic providers**: OpenAI Batch / Gemini Batch are not in the leader-elected poller loop (`letta/jobs/llm_batch_job_polling.py:196-197` is hardcoded to Anthropic).
- **Resumable agent step (not just LLM batch step)**: a checkpoint-and-replay pattern modeled on `_checkpoint_messages` plus a `pending_writes`-style table would let regular runs survive a pod restart. The Lettuce external executor exists to provide this in cloud (`letta/services/lettuce/lettuce_client_base.py:9-101`), but is a no-op stub in OSS.
- **DLQ for terminal-failed runs**: a `run_failure_queue` table or Sentry breadcrumb on every `stop_reason=error` would give operators a single view.
- **Cancellation event cleanup**: the `_cancellation_events` dict at `letta/server/rest_api/streaming_response.py:31-40` grows unboundedly across the process lifetime.
- **Cross-pod cancellation signal**: today the cancellation flag is read from the DB (`letta/agents/letta_agent_v2.py:750-757`) which is correct but adds DB round-trips; a Redis pub/sub channel could speed it up for high-frequency polling loops.
- **Idempotency keys for tool calls**: tool execution is not idempotent — re-running a partially-failed run could double-execute side-effecting tools. The Lettuce design is the obvious place to add this.

## Questions / Gaps

- Is there a documented operational procedure for dealing with `running` runs older than X hours? No code path does this automatically.
- What happens to a run when Lettuce is enabled but unreachable? `get_status` falls back to DB status on exception (`letta/services/run_manager.py:126-129`), so the run stays `running` indefinitely until cancelled.
- Does the Anthropic poller have a max retry count? `poll_running_llm_batches` retries "still running" on retrieval error (`letta/jobs/llm_batch_job_polling.py:60-62`) without any backoff cap or max-attempt limit.
- Are `expires_at` timestamps on `MCPOAuth` enforced server-side? `letta/orm/mcp_oauth.py:48` declares the column but no scheduler sweeps expired tokens — `cleanup_expired_oauth_sessions` exists as a function but is not invoked from any lifespan hook (`letta/services/mcp_manager.py:1071-1087`).
- Is the Lettuce external executor source open? The OSS base client at `letta/services/lettuce/lettuce_client_base.py` is a stub. Cloud recovery semantics are not visible from this repo.
- Does the event-loop watchdog test (`tests/test_watchdog_hang.py`) actually run in CI, or is it manual-only? No `pytest` test invocation was found referencing it.

---

Generated by `02.08-crash-recovery-and-reconstruction` against `letta`.
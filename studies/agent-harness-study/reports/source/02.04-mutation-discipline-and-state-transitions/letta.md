# Source Analysis: letta

## 02.04 — Mutation Discipline and State Transitions

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python 3.10+, SQLAlchemy 2.x (async), PostgreSQL/SQLite, Redis (optional distributed locks), OpenTelemetry |
| Analyzed | 2026-07-06 |

## Summary

Letta's mutation discipline is layered and uneven. The base ORM (`SqlalchemyBase`) routes every create/update/delete through centralized, retry-decorated async methods (`letta/orm/sqlalchemy_base.py:560`, `:747`, `:684`) that automatically populate `created_at`, `updated_at`, `created_by_id`, `last_updated_by_id`, and the soft-delete `is_deleted` flag (`letta/orm/base.py:13-85`). All access predicates are forced through `apply_access_predicate` (`letta/orm/sqlalchemy_base.py:872`) which scopes every query to the actor's `organization_id`. Mutation visibility is observed via OpenTelemetry: the `@trace_method` decorator (`letta/otel/tracing.py:228`) and `log_event` (`letta/otel/tracing.py:484`) wrap nearly every manager method, and run/step completions additionally fire HTTP callbacks and webhooks (`letta/services/run_manager.py:483-510`, `letta/services/webhook_service.py:16`).

State-transition enforcement, however, is partial and inconsistent. `RunStatus` is a `StrEnum` with `is_terminal` defined (`letta/schemas/enums.py:133-149`), but `RunManager.update_run_by_id_async` only **logs** illegal transitions (e.g., re-opening a completed run) instead of raising — illegal-status updates proceed silently (`letta/services/run_manager.py:342-356`). `JobManager.safe_update_job_status_async` does enforce `Created → Pending → Running → Terminal` (`letta/services/job_manager.py:84-94`), and `FileManager.update_file_status` enforces the PENDING/PARSING/EMBEDDING/COMPLETED/ERROR machine inside a single SQL `WHERE` clause (`letta/services/file_manager.py:213-231`). Optimistic locking is implemented for the `Block` ORM only, via SQLAlchemy's `version_id_col` (`letta/orm/block.py:56-61`); everywhere else, conflicts are surfaced only as deadlock retries and `StaleDataError → ConcurrentUpdateError` mapping (`letta/orm/sqlalchemy_base.py:779-780`). Concurrent execution is serialized per-conversation through a Redis distributed lock with TTL (`letta/data_sources/redis_client.py:194-229`), and per-memory-repo through a separate Redis lock (`letta/data_sources/redis_client.py:311-348`). The human-in-the-loop approval state is implicit and reconstructed by reading `agent_state.message_ids[-1]` and calling `is_approval_request()` (`letta/schemas/message.py:2450`); the only protection is `PendingApprovalError` raised in `helpers.py:309`, not a stored status field.

## Rating

**6 / 10** — Present but inconsistent, weakly documented, and fragile. Some flows have hard guards (`safe_update_job_status_async`, file state machine, conversation lock); the most critical write paths (Run.status, Agent.message_ids, Step.status) are either logged-but-not-raised or rely on soft invariants maintained outside the database.

Rationale:
- **+** Centralized mutation funnel (`SqlalchemyBase.create_async/update_async/delete_async`, `letta/orm/sqlalchemy_base.py:560`, `:747`, `:684`) with retry/backoff for deadlocks (`:591-610`), automatic actor tracking (`:758-759`), and per-method access predicates (`:872`).
- **+** A real state machine exists for files with WHERE-clause-enforced transitions and tested invariants (`letta/services/file_manager.py:200-274`, `tests/managers/test_source_manager.py:1111-1495`).
- **+** Conversation concurrency serialized via Redis lock with `ConversationBusyError` mapped to HTTP 409 (`letta/data_sources/redis_client.py:194`, `letta/errors.py:81`, `letta/server/rest_api/app.py:584`).
- **+** Optimistic locking on `Block` via SQLAlchemy `version_id_col` plus dedicated `ConcurrentUpdateError` and 409 mapping (`letta/orm/block.py:56-61`, `letta/errors.py:72`, `letta/server/rest_api/app.py:584`).
- **+** Mutations are observable: OTel `@trace_method` plus `log_event` (`letta/otel/tracing.py:228`, `:484`) and run/step webhooks (`letta/services/run_manager.py:500-503`, `letta/services/webhook_service.py:16`).
- **−** `RunManager.update_run_by_id_async` does not enforce state transitions — illegal updates (e.g., `completed → running`) are logged as errors but still committed (`letta/services/run_manager.py:342-356`).
- **−** `Agent.message_ids` is mutated via repeated read-modify-write (`agent_state.message_ids = …`, then `update_message_ids_async`, `letta/services/agent_manager.py:999-1019`) with no optimistic version check; a desync between persisted messages and the agent's pointer is a known failure mode explicitly tested for in `test_message_state_consistency_after_cancellation` (`tests/managers/test_cancellation.py:63-142`).
- **−** `StepStatus` (PENDING/SUCCESS/FAILED/CANCELLED) has no transition guard; `step_manager.update_step_error_async` blindly sets `FAILED` (`letta/services/step_manager.py:402`); `update_step_success_async` sets `SUCCESS` (`:447`) without checking prior status.
- **−** Approval state is implicit in message ordering (`is_approval_request()` reads the tail of `agent.message_ids`, `letta/schemas/message.py:2450`); a missing message at index -1 leaves the system believing no approval is pending.
- **−** Conversation lock is opt-out via `NoopAsyncRedisClient` (`letta/data_sources/redis_client.py:583-625`), so deployments without Redis lose serializability entirely; the `test_concurrent_messages_to_same_conversation` test is skipped when Redis is not configured (`tests/integration_test_conversations_sdk.py:572`).
- **−** TODO at `letta/services/file_processor/file_processor.py:318` admits "the file state machine here is kind of out of date".
- **−** Commented-out race-condition tests at `tests/test_managers.py:12411-12532` indicate that high-concurrency block updates still surface deadlocks (`INSERT INTO blocks_agents`) — the deadlock-retry path is a backstop, not a model.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Centralized mutation funnel (create) | `SqlalchemyBase.create_async` populates actor + retry on deadlock | `letta/orm/sqlalchemy_base.py:560-611` |
| Centralized mutation funnel (update) | `update_async` maps `StaleDataError → ConcurrentUpdateError`, retries on deadlock | `letta/orm/sqlalchemy_base.py:747-791` |
| Centralized mutation funnel (delete) | Soft-delete via `is_deleted=True`, hard-delete with deadlock retry | `letta/orm/sqlalchemy_base.py:673-706` |
| Base mixin metadata | `created_at`, `updated_at`, `is_deleted`, `created_by_id`, `last_updated_by_id` | `letta/orm/base.py:13-85` |
| Organization-scoped access predicate | `apply_access_predicate` enforces `organization_id == actor.organization_id` | `letta/orm/sqlalchemy_base.py:872-902` |
| Run status enum + terminal predicate | `RunStatus` (StrEnum) with `is_terminal` property | `letta/schemas/enums.py:133-149` |
| Run ORM column | `status: Mapped[RunStatus]` with default `RunStatus.created` | `letta/orm/run.py:40` |
| Run state transition guard (only logged, not raised) | `update_run_by_id_async` logs errors for completed→non-cancelled and terminal→any | `letta/services/run_manager.py:342-356` |
| Run auto-completion timestamp | `completed_at` set automatically when terminal status set without one | `letta/services/run_manager.py:362-364` |
| Run status from stop_reason mapping | `StopReasonType.run_status` property translates stop reason to RunStatus | `letta/schemas/letta_stop_reason.py:24-49` |
| Run creation, metrics, project_id wiring | `RunManager.create_run` | `letta/services/run_manager.py:47-90` |
| Run callback on terminal transition | `_dispatch_callback_async` posts only on first terminal update | `letta/services/run_manager.py:333-339`, `:450-479`, `:483-510` |
| Cancel idempotency | `cancel_run` returns early if `run.stop_reason not in [None, requires_approval]` | `letta/services/run_manager.py:649-653` |
| Job status enum + terminal predicate | `JobStatus.is_terminal` returns True for completed/failed/cancelled/expired | `letta/schemas/enums.py:147-149` |
| Job status transition guard (real) | `safe_update_job_status_async` rejects illegal Created→notCreated etc.; `safe_update` flag | `letta/services/job_manager.py:84-94`, `:153-189` |
| Job auto-completion timestamp | `completed_at` set on terminal | `letta/services/job_manager.py:113-115` |
| File processing state machine (real) | `update_file_status` enforces PENDING/PARSING/EMBEDDING/COMPLETED/ERROR with WHERE-clause guard | `letta/services/file_manager.py:200-274` |
| File terminal-state lock | `processing_status.is_terminal_state()` used to skip status transitions | `letta/schemas/enums.py:200-209`, `letta/services/file_manager.py:311-336` |
| Block optimistic locking | `version: Mapped[int]` with SQLAlchemy `version_id_col` | `letta/orm/block.py:56-61` |
| Block history pointer | `current_history_entry_id`, `BlockHistory` table for undo/redo | `letta/orm/block.py:53-55`, `alembic/versions/bff040379479_add_block_history_tables.py:29-58` |
| Optimistic-lock exception type | `ConcurrentUpdateError(LettaError)` with CONFLICT code | `letta/errors.py:72-78` |
| ConcurrentUpdateError HTTP mapping | `_error_handler_409` registered for FastAPI app | `letta/server/rest_api/app.py:584` |
| Conversation distributed lock | `acquire_conversation_lock` raises `ConversationBusyError` if held | `letta/data_sources/redis_client.py:194-229` |
| Conversation lock TTL | 300 s | `letta/constants.py:474-475` |
| Memory repo distributed lock | `acquire_memory_repo_lock` (TTL 60 s) prevents git-memory concurrent writes | `letta/data_sources/redis_client.py:311-348`, `letta/constants.py:482-483` |
| Conversation lock release after run | `release_conversation_lock` called in `update_run_by_id_async` and after stream error | `letta/services/run_manager.py:390-396`, `letta/server/rest_api/redis_stream_manager.py:413-417` |
| Step status enum | `StepStatus` (PENDING/SUCCESS/FAILED/CANCELLED) — no terminal predicate | `letta/schemas/enums.py:268-274` |
| Step status update (no transition guard) | `update_step_error_async` sets `FAILED`; `update_step_success_async` sets `SUCCESS`; `update_step_cancelled_async` sets `CANCELLED` | `letta/services/step_manager.py:402`, `:447`, `:545` |
| Step status initial | `log_step` defaults to `PENDING` if not provided | `letta/services/step_manager.py:160`, `:222` |
| Step completion webhook | `WebhookService.notify_step_complete` fires after success/error/cancelled | `letta/services/step_manager.py:413`, `:475`, `:554`, `letta/services/webhook_service.py:16` |
| Approval state implicit in messages | `is_approval_request` checks `role == "approval" and tool_calls is not None` | `letta/schemas/message.py:2450-2454` |
| Pending-approval guard | `PendingApprovalError` raised when last in-context message is an approval request | `letta/agents/helpers.py:309-310`, `letta/errors.py:48-56` |
| Agent message_ids mutation (read-modify-write, no version) | `update_message_ids_async` uses plain `update_async` without optimistic guard | `letta/services/agent_manager.py:999-1019` |
| Agent update funnel | `update_agent_async` updates scalar fields via direct `setattr`, replaces pivot rows | `letta/services/agent_manager.py:759-989` |
| Conversation-level message pointer | `ConversationMessage.in_context` flag + `position` ordering managed centrally | `letta/services/conversation_manager.py:773-797` |
| Conversation soft-delete cascade | `delete_conversation` soft-deletes messages, hard-deletes isolated blocks | `letta/services/conversation_manager.py:556-609` |
| Run metadata merge-on-update | `update_run_by_id_async` merges metadata_ dicts rather than overwriting | `letta/services/run_manager.py:369-374` |
| OTel tracing decorator | `@trace_method` records spans + parameters (with skip list) | `letta/otel/tracing.py:228-...` |
| OTel event emitter | `log_event(name, attributes)` adds to current span | `letta/otel/tracing.py:484-496` |
| Run callback log events | `log_event("POST callback dispatched" / "finished")` | `letta/services/run_manager.py:500-503` |
| Approve/deny mutation via Cancellation cleanup | Cancel of a pending-approval run inserts denial ApprovalReturn + tool returns | `letta/services/run_manager.py:670-783` |
| TODO on file state machine | "kind of out of date, we need to match with the correct one above" | `letta/services/file_processor/file_processor.py:318` |
| Race-condition regression (commented test) | `test_concurrent_block_updates_race_condition` left disabled with deadlock stack trace in comment | `tests/test_managers.py:12402-12532` |
| File state-transition tests (positive coverage) | `test_file_status_invalid_transitions`, `test_same_state_transitions_allowed` | `tests/managers/test_source_manager.py:1111-1495` |
| Message-state consistency test | `test_message_state_consistency_after_cancellation` checks DB ↔ agent.message_ids desync | `tests/managers/test_cancellation.py:63-142` |
| Conversation lock tests (Redis-dependent) | `test_concurrent_messages_to_same_conversation`, `test_concurrent_conversation_requests_return_409` — both skip when Redis missing | `tests/integration_test_conversations_sdk.py:559`, `:1712` |
| Run cancel idempotency | `cancel_run` no-op when `stop_reason not in [None, requires_approval]` | `letta/services/run_manager.py:649-653` |

## Answers to Dimension Questions

1. **Can any module mutate state?**
   - No direct raw SQL outside of helpers; all writes flow through `SqlalchemyBase.create_async`/`update_async`/`delete_async` (`letta/orm/sqlalchemy_base.py:560-791`) which enforce actor tracking, `updated_at`, `is_deleted`, deadlock retry, and `StaleDataError` mapping. Bypasses exist: the `BatchCreateAsync` helper and bulk updates via raw `update(...)` statements (e.g., `letta/services/file_manager.py:239-244`) skip the version + retry plumbing.
   - Manager methods are wrappers but enforce type signatures via `@enforce_types` (`letta/utils.py:533`) and validation via `@raise_on_invalid_id` (`letta/validators.py`).

2. **Are illegal transitions prevented?**
   - **File processing:** Yes — illegal transitions raise `ValueError("Invalid state transition ...")` and never persist (`letta/services/file_manager.py:200-274`).
   - **Job:** Yes — `safe_update_job_status_async` rejects transitions not matching `Created → Pending → Running → Terminal` (`letta/services/job_manager.py:84-94`); bypassable when caller omits `safe_update=True` flag.
   - **Run:** No — `RunManager.update_run_by_id_async` logs but does not raise on illegal transitions (`letta/services/run_manager.py:342-356`). The terminal-state guard relies on `not_completed_before` to decide whether to fire callbacks (`:333-339`); subsequent updates overwrite.
   - **Step:** No — `update_step_error_async` / `_success_async` / `_cancelled_async` overwrite status unconditionally (`letta/services/step_manager.py:402`, `:447`, `:545`).
   - **Conversation / message-position:** Yes at the `ConversationMessage.in_context` + `position` level (`letta/services/conversation_manager.py:773-797`); the agent-side `message_ids` list has no transition guard.
   - **Block (memory):** Partially — optimistic locking raises `ConcurrentUpdateError` on version mismatch (`letta/orm/sqlalchemy_base.py:779-780`), but the version only protects row-level updates; checkpoint creation and undo can still produce duplicated sequence numbers if not wrapped in a transaction.

3. **Are concurrent writes safe?**
   - **Per-conversation writes:** Serialized via Redis lock (`letta/data_sources/redis_client.py:194`) with 5-minute TTL and idempotent release (`:231`). When Redis is unavailable, `NoopAsyncRedisClient` no-ops locking (`letta/data_sources/redis_client.py:593-625`), and tests skip concurrency coverage (`tests/integration_test_conversations_sdk.py:572`).
   - **Per-agent memory repo:** Separate Redis lock (`letta/data_sources/redis_client.py:311`) prevents concurrent git operations.
   - **Database-level:** PostgreSQL deadlocks are detected (`letta/orm/sqlalchemy_base.py:56`) and retried up to 3 times with exponential backoff (`:40-41`, `:591-610`, `:691-705`). Statement timeouts and lock timeouts are mapped to `DatabaseTimeoutError` and `DatabaseLockNotAvailableError` (`letta/orm/sqlalchemy_base.py:69-108`, `:912-928`).
   - **Block writes:** Optimistic locking via `version_id_col` raises `ConcurrentUpdateError` on race (mapped to HTTP 409, `letta/errors.py:72`, `letta/server/rest_api/app.py:584`).
   - **Known weakness:** Bulk-update of `blocks_agents` pivot rows can deadlock under high concurrency — the regression test is commented out with a captured stack trace showing `DeadlockDetectedError` on `INSERT INTO blocks_agents` (`tests/test_managers.py:12411-12432`).

4. **Are mutations observable?**
   - Yes, two channels:
     - **OpenTelemetry:** `@trace_method` decorator (`letta/otel/tracing.py:228`) wraps essentially every manager method; `log_event` is called inside hot paths (`letta/services/run_manager.py:500-503`).
     - **HTTP webhooks:** `_dispatch_callback_async` fires only on the first terminal `Run` update (`letta/services/run_manager.py:450-510`); `WebhookService.notify_step_complete` fires after every step terminal transition (`letta/services/step_manager.py:413`, `:475`, `:554`).
   - **State changes are not surfaced as first-class domain events.** There is no `RunStatusChanged` event consumer or `StepStatusChanged` event consumer; downstream systems learn about state only via polled reads or the callback URL round-trip.

5. **Can state become internally inconsistent?**
   - **Run ↔ Agent.last_stop_reason:** `update_run_by_id_async` writes the run, then writes `agent.last_stop_reason` in a separate `update_agent_async` call (`letta/services/run_manager.py:400-410`). A failure between the two leaves `agent.last_stop_reason` stale.
   - **Persisted messages ↔ agent.message_ids:** The standard pattern is `create_many_messages_async` then `update_message_ids_async` (e.g., `letta/agents/letta_agent.py:1612-1614`). The cancellation test specifically guards against this desync (`tests/managers/test_cancellation.py:121-142`).
   - **Run ↔ Steps:** `RunMetricsModel.num_steps` and `tools_used` are recomputed inside `update_run_by_id_async` by re-listing steps (`letta/services/run_manager.py:412-446`); they can drift from the true step count between commit and re-read.
   - **Step status vs. Run terminal status:** A step can be marked FAILED while the parent run stays `running` (no transaction wraps both). The streaming finalizer (`letta/server/rest_api/redis_stream_manager.py:428-445`) handles this by re-reading stop_reason and pushing a terminal status, but the two writes are not atomic.
   - **Block version + history pointer:** `checkpoint_block_async` truncates future `BlockHistory` rows, inserts a new history entry, updates `current_history_entry_id`, and `update_async`s the block — within one transaction (`letta/services/block_manager.py:842-911`), so this is safe.

## Architectural Decisions

- **Centralized ORM base.** Every business-object write goes through `SqlalchemyBase.create_async`/`update_async`/`delete_async`/`hard_delete_async` (`letta/orm/sqlalchemy_base.py:560-791`). This gives a single chokepoint for actor attribution, deadlock retry, `StaleDataError` mapping, and timestamps.
- **Soft-delete via `is_deleted`.** Mixed across models — most inherit it from `CommonSqlalchemyMetaMixins` (`letta/orm/base.py:18`) but some tables (e.g., `BlocksTags`, `letta/orm/blocks_tags.py:33`) re-declare; isolated `Block` rows in conversations still rely on hard delete (`letta/services/conversation_manager.py:602-609`).
- **StrEnum + `is_terminal` property pattern.** Applied to `JobStatus`, `RunStatus`, `FileProcessingStatus` (`letta/schemas/enums.py:133-209`), enabling `is_terminal` checks in routing and listing code without scattering string comparisons.
- **Conversation-level serialization via Redis.** A single 5-minute TTL lock key per `conversation_id` (`letta/constants.py:474-475`) makes per-conversation concurrency linear. The TTL bounds the damage if a worker crashes mid-flight, at the cost of 5-minute tail latency for recovery.
- **Optimistic locking on memory blocks.** Block value edits and checkpoint operations are the only place that uses SQLAlchemy `version_id_col` (`letta/orm/block.py:56-61`). The justification — multiple agents share a block, and "last write wins" would silently overwrite another agent's edits.
- **Run-level state decoupled from agent-loop execution.** The run row is the authoritative record of an execution session. Agent loops create it (`letta/services/run_manager.py:48-90`), update it at terminal points (`:319-481`), and the streaming finalizer re-reads stop_reason to derive the terminal status (`letta/server/rest_api/redis_stream_manager.py:428-445`).
- **StopReasonType → RunStatus mapping.** A property on `StopReasonType` (`letta/schemas/letta_stop_reason.py:24-49`) defines the canonical mapping; clients that bypass `RunManager` (e.g., routers in `letta/server/rest_api/routers/v1/agents.py:1799-1821`) can compute the same target status.
- **Callback-on-terminal-only.** Run callbacks fire exactly once, on the first transition into a terminal state (`letta/services/run_manager.py:333-339`, `:656-706`). Step webhooks fire on every terminal step (`letta/services/step_manager.py:413`, `:475`, `:554`).
- **Approval as message type, not state field.** Human-in-the-loop approvals are encoded as `MessageRole.approval` rows in the message log (`letta/schemas/message.py:178-197`, `:303-307`); the only "state" is the existence and ordering of those rows. `PendingApprovalError` (`letta/errors.py:48-56`) and `is_approval_request()` (`letta/schemas/message.py:2450`) form the implicit guard.
- **MongoDB-style OTID for client-side retry dedup.** The streaming service stores `otid → run_id` in Redis (`letta/data_sources/redis_client.py:250`) so duplicate requests with the same OTID recover the in-flight run rather than starting a new one (`letta/services/streaming_service.py:285-317`, `letta/server/rest_api/routers/v1/conversations.py:330-373`).

## Notable Patterns

- **Decorator stack on every manager method.** `@enforce_types` (`letta/utils.py:533`), `@raise_on_invalid_id` (`letta/validators.py`), and `@trace_method` (`letta/otel/tracing.py:228`) compose to validate inputs, normalize IDs, and emit telemetry.
- **Audit metadata on every row.** `created_at`, `updated_at`, `created_by_id`, `last_updated_by_id`, `is_deleted` (`letta/orm/base.py:13-85`) populated automatically on every `create_async`/`update_async`/`delete_async`.
- **Per-step and per-run metrics side-tables.** `StepMetricsModel` and `RunMetricsModel` (`letta/orm/run_metrics.py`, `letta/orm/step_metrics.py`) are populated outside the main transaction to avoid blocking the LLM streaming pipeline (`letta/services/run_manager.py:412-446`).
- **Single-session `async with db_registry.async_session() as session` pattern.** Used everywhere; commits happen on context exit. The codebase has largely moved away from explicit `await session.commit()` (visible as commented-out lines: `letta/services/run_manager.py:88-89`, `:386-388`).
- **State-of-pending-approval reconstructed from message tail.** Saves a column and a write, but moves consistency into the message-creation code path (`letta/agents/helpers.py:309`).
- **Soft-cancel with conversation-state repair.** When cancelling a run that was waiting on approval, the manager constructs denial `ApprovalReturn`s and tool returns and checkpoints them in a single agent-loop call (`letta/services/run_manager.py:670-783`).
- **Distributed lock with `raise_on_release_error=False`.** `letta/data_sources/redis_client.py:219` — failures during release are logged, not raised, so cleanup paths don't crash.

## Tradeoffs

- **Soft invariants over hard validation.** Illegal Run transitions are logged but committed (`letta/services/run_manager.py:342-356`); this gives operators visibility but lets clients corrupt state. Pragmatic for streaming flows where the producer and consumer of `Run.status` are decoupled, but creates the inconsistency surfaced in cancellation tests.
- **Optimistic locking only on `Block`.** Choosing memory blocks as the only versioned table trades row-update cost for cross-agent safety. Everywhere else, deadlock retry is the backstop — cheap when contention is rare, but the high-concurrency regression test shows it is not free.
- **Conversation lock via Redis with TTL.** Linear-per-conversation execution simplifies message ordering and `message_ids` desync handling, but couples the system to Redis being healthy; the `NoopAsyncRedisClient` path is an undocumented silent degradation.
- **Implicit approval state.** Reconstructing "is the agent awaiting approval" from the tail of `agent.message_ids` (`letta/schemas/message.py:2450`) avoids a new column, but couples approval detection to message-write correctness — a misordered `append_to_in_context_messages` call breaks the guard.
- **Mutable `agent.message_ids`.** Treated as a free-form list with read-modify-write semantics (`letta/services/agent_manager.py:1650-1667`). The advantage is flexibility (summarization can rewrite history), the cost is the desync tested for in `tests/managers/test_cancellation.py:121-142`.
- **`StepStatus` lacks an `is_terminal` predicate.** `StepStatus` enum (`letta/schemas/enums.py:268-274`) has no terminal flag, even though SUCCESS/FAILED/CANCELLED are effectively terminal. Webhook code checks the concrete values inline (`letta/services/step_manager.py:413`, `:475`, `:554`).
- **Separate writes for run + metrics + last_stop_reason.** `update_run_by_id_async` issues three sequential DB sessions (`letta/services/run_manager.py:319-481`); partial failures can leave metrics stale.
- **Mixed Soft-delete vs. hard-delete.** `delete_conversation` soft-deletes messages but hard-deletes isolated blocks (`letta/services/conversation_manager.py:556-609`) — the pattern is intentional ("Block model doesn't support soft-delete" comment at `:602`) but increases blast radius.

## Failure Modes / Edge Cases

- **Run status desync from agent loop.** If a streaming run crashes after `release_conversation_lock` but before `update_run_by_id_async` (`letta/server/rest_api/redis_stream_manager.py:413-445`), the run row can stay `running` indefinitely; the `finally` block attempts the update, but a `try/except` swallows lock-release errors (`:416`) and a separate `try/except` swallows the booking call (`:441-445`).
- **message_ids desync after cancellation.** Tested explicitly (`tests/managers/test_cancellation.py:121-142`); the `cancel_run` path reconstructs `agent.message_ids` only inside `agent_loop._checkpoint_messages` (`letta/services/run_manager.py:751-760`), and the commented-out code path that called `update_message_ids_async` was removed (`:769-771`).
- **High-concurrency block pivot deadlock.** `INSERT INTO blocks_agents ... ON CONFLICT DO NOTHING` can deadlock under contention (`tests/test_managers.py:12402-12532`). The deadlock-retry path catches it but the test is disabled.
- **Redis-down silent degradation.** When `NoopAsyncRedisClient` is in use, conversation locks are no-ops, allowing concurrent requests to mutate `agent.message_ids` simultaneously; the system has no fallback "admission control" layer.
- **Approval request with empty `tool_calls`.** `is_approval_request()` requires `tool_calls is not None and len(tool_calls) > 0` (`letta/schemas/message.py:2450`), so an approval message that lost its tool calls is silently treated as no approval pending. Cancellation code logs a warning (`letta/services/run_manager.py:777-781`) but does not recover.
- **Run created with a stale `agent_id`.** If the agent is deleted between `create_run` and `update_run_by_id_async`, the second update's `agent_manager.update_agent_async` for `last_stop_reason` (`letta/services/run_manager.py:404-408`) catches the exception and logs — the run state is preserved but agent state is silently stale.
- **`FileProcessingStatus.PENDING` cannot be re-entered after initial create.** Enforced (`letta/services/file_manager.py:201-203`), but a worker that crashes after marking `EMBEDDING` cannot simply resume — it must continue to `COMPLETED` or fail.
- **Stale `Block.version` after a long-running agent checkpoint.** Optimistic locking raises `ConcurrentUpdateError` and surfaces 409 (`letta/orm/sqlalchemy_base.py:779-780`, `letta/server/rest_api/app.py:584`); clients that retry without re-reading the block version enter a loop.
- **ConversationMessage.position reordering race.** `update_in_context_messages` reorders positions in-memory then commits (`letta/services/conversation_manager.py:773-797`); two concurrent updaters can produce non-monotonic positions.

## Future Considerations

- **Promote Run/Step transition guards to the ORM layer.** Adding `is_terminal` to `StepStatus` and a `_validate_transition` helper on `Run`/`Step` models would close the gap currently papered over with `logger.error` (`letta/services/run_manager.py:342-356`).
- **Add optimistic locking to Agent, Run, Conversation.** Currently only `Block` carries a version column (`letta/orm/block.py:56-61`); the agent and run rows are mutated by multiple async tasks, and the existing read-modify-write `agent.message_ids` pattern (`letta/services/agent_manager.py:1016`) would benefit from the same guarantee.
- **Promote approval state to a real field.** Replace the `is_approval_request()` heuristic (`letta/schemas/message.py:2450`) with an explicit `agent_state.pending_approval_id` column set/cleared atomically with message writes; this would survive partial-message-write failures.
- **Recover concurrent-state invariants at the DB layer.** Wrap the three sequential writes in `update_run_by_id_async` (`letta/services/run_manager.py:319-481`) — Run row, agent `last_stop_reason`, `RunMetricsModel` — into one transaction with `session.begin()`.
- **Make the conversation lock mandatory.** `NoopAsyncRedisClient` (`letta/data_sources/redis_client.py:593-625`) silently degrades; instead, fail fast at startup if Redis is missing in a multi-replica deployment.
- **Re-enable the high-concurrency block tests.** `tests/test_managers.py:12411-12532` is commented out with a captured deadlock; either fix the deadlock (e.g., by acquiring `blocks_agents` rows in a deterministic order) or accept the deadlock-retry path and prove it with active tests.
- **Domain events.** A first-class `RunStatusChanged` / `StepStatusChanged` event bus would let the webhook and last_stop_reason logic subscribe to writes rather than re-reading, and would let downstream consumers receive guaranteed, ordered notifications.
- **Merge `AgentStepStatus` and `StepStatus`.** The TODO at `letta/schemas/enums.py:167` ("consolidate this with job status") hints at drift; today there are two parallel enums with overlapping values (`letta/orm/llm_batch_items.py:46`, `letta/services/step_manager.py:160`).

## Questions / Gaps

- Is the lack of a `Run.status` transition guard intentional for streaming flows where the producer may not know the final stop_reason, or a missing feature? The current behavior lets a client POST `RunUpdate(status=RunStatus.running)` after `RunUpdate(status=RunStatus.completed)` succeed silently (`letta/services/run_manager.py:342-356`).
- How does the system behave when `agent_manager.update_agent_async` raises inside the "do this after run update is committed" block (`letta/services/run_manager.py:400-410`)? The exception is logged but the run update is already committed — is this intentional or a hole?
- Are there any production traces showing deadlocks on `blocks_agents` outside the disabled test (`tests/test_managers.py:12411`)? The current answer is: deadlock-retry handles it for low-contention workloads, no evidence for high-contention.
- Is `agent_state.message_ids` ever read while a write is in flight (e.g., between `create_many_messages_async` and `update_message_ids_async`)? No clear evidence found in the manager paths, but the cancellation cleanup at `letta/services/run_manager.py:751-760` does checkpoint inside `agent_loop._checkpoint_messages` which does not necessarily call `update_message_ids_async`.
- Does the `NoopAsyncRedisClient` path emit a metric or warning so operators notice the silent degradation? No clear evidence found.
- Are there any places outside `FileManager.update_file_status` that use WHERE-clause-enforced state transitions? The pattern is unique in the codebase; `RunManager` and `StepManager` rely on application-level checks.
- What is the behavior when `Block.version` increments but `Block.current_history_entry_id` fails to update? Optimistic locking covers the block row but the `BlockHistory` insert + `current_history_entry_id` update happens in the same `checkpoint_block_async` transaction (`letta/services/block_manager.py:842-911`) — this is safe today, but adding a second writer of `current_history_entry_id` could regress it.

---

Generated by `02.04-mutation-discipline-and-state-transitions` against `letta`.
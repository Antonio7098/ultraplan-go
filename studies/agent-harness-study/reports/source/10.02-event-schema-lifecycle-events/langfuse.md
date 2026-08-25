# Source Analysis: langfuse

## 10.02 — Event Schema and Lifecycle Events

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript / Next.js (web) + Express/BullMQ (worker) + Postgres + ClickHouse |
| Analyzed | 2026-08-23 |

## Summary

Langfuse has two distinct event systems in the source tree: the canonical AG-UI event stream that drives the in-app agent runtime (`packages/shared/src/in-app-agent/*` + `worker/src/features/in-app-agent/*`), and the OpenTelemetry-shaped `events_core` ClickHouse table that backs public observability (`packages/shared/src/eventsTable.ts`, ClickHouse migrations `0039`–`0044`).

The AG-UI stream is the closest analogue to "lifecycle events". It uses a typed, discriminated event enum from `@ag-ui/core` (`EventType`) with a per-conversation `sequenceNumber` cursor for ordering, a per-row `createdAt` timestamp, and an explicit `runId` parent pointer on every row (`packages/shared/prisma/schema.prisma:263-278`). Events are linked to runtime objects via the `InAppAgentRun` model (`status`, `claimedAt`, `heartbeatAt`, `finishedAt`, `errorCode`, `cancelRequestedAt`), and the lifecycle is governed by Postgres CAS transitions in `runLifecycle.ts`. Run lifecycle states (`QUEUED`, `RUNNING`, `AWAITING_APPROVAL`, `SUCCEEDED`, `FAILED`, `CANCELLED`) and terminal error codes (`APPROVAL_EXPIRED`, `WORKER_LOST`, `RUN_TIMEOUT`, `STEP_LIMIT`, etc.) are versioned as a typed enum (`packages/shared/src/features/inAppAgent/types.ts:12-69`) that admits "unknown string" readers and "missing status" readers.

The runtime can reconstruct every run: durable append-only event stream with monotonic cursor, run row with timestamps + heartbeats + error codes, terminal reconciliation with deadline detection (`classifyStaleRun`), and approval continuations threaded via typed `parentRunId` / `rootRunId` / `traceStartedAt` fields. Event typing is enforced via an explicit whitelist in `toPersistableAgentEvent` (`persistence.ts:739-932`); unknown event types return `null` rather than being silently dropped in some readers.

## Rating

**8 / 10 — Clear model with tests, explicit interfaces, and operational safeguards, with a few minor weaknesses around cursor decoupling and durability of run-level timestamps.**

Rationale: the event schema is explicit and well-tested, sequence numbers are monotonic per conversation, parent IDs are present on every event row, and lifecycle states cover creation/completion/failure/cancellation/timeout. We deduct points because (1) the AG-UI `EventType` is borrowed from a third-party library that duplicates schemas due to Zod v3/v4 mismatch (`packages/shared/src/in-app-agent/schema.ts:9-14`), (2) `createdAt` lives only on Postgres — the `InAppAgentEvent.event` JSON body does not carry the producer-side `timestamp` field that `BaseEventSchema` allows (`@ag-ui/core`), so cross-process replay relies on the database column rather than the wire event, and (3) run-level state (`createdAt`/`claimedAt`/`heartbeatAt`) and event-level state (`createdAt`/`sequenceNumber`) are not bound by a single foreign-key constraint tying an event's `createdAt` to its `run.createdAt`/`run.claimedAt` ordering.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Event type enum (AG-UI) | `EventType` enum with text/tool/reasoning/run/step/state/activity/custom variants | `packages/shared/src/in-app-agent/schema.ts:208-219` (re-export) + `@ag-ui/core` `EventType` |
| Persisted event table | `InAppAgentEvent` model with `sequenceNumber`, `runId`, `createdAt`, `event JsonB`, `type String` | `packages/shared/prisma/schema.prisma:263-278` |
| Composite PK on event | `@@id([projectId, conversationId, sequenceNumber])` enforces uniqueness per conversation | `packages/shared/prisma/schema.prisma:275` |
| Run-level timestamps | `createdAt`, `claimedAt`, `heartbeatAt`, `cancelRequestedAt`, `finishedAt`, `updatedAt` | `packages/shared/prisma/schema.prisma:296-302` |
| Run status enum | `InAppAgentRunStatus` (QUEUED, RUNNING, AWAITING_APPROVAL, SUCCEEDED, FAILED, CANCELLED) | `packages/shared/src/features/inAppAgent/types.ts:12-19` |
| Run error codes (lifecycle) | `InAppAgentRunErrorCode` covers STALE, CANCELLED, AGENT_ERROR, INIT_FAILED, WORKER_SHUTDOWN, OUTCOME_UNKNOWN, ENQUEUE_FAILED, QUEUE_TIMEOUT, WORKER_LOST, RUN_TIMEOUT, APPROVAL_EXPIRED, APPROVAL_SUPERSEDED, APPROVAL_CANCELLED, STEP_LIMIT | `packages/shared/src/features/inAppAgent/types.ts:30-69` |
| Sequence number assignment | Last-`sequenceNumber` lookup + `(latestEvent?.sequenceNumber ?? -1) + 1` | `packages/shared/src/in-app-agent/server/runLifecycle.ts:850-882` |
| Sequence number append (run events) | `appendRunEvents` extends tail via `createMany` after lockConversation | `packages/shared/src/in-app-agent/server/persistence.ts:213-237` |
| Sequence-number watch cursor | `PersistedConversationEvent` carries `sequenceNumber`; comment marks it as the watch cursor | `packages/shared/src/in-app-agent/server/persistence.ts:72-83` |
| Ordering read | `getConversationEvents` orders by `sequenceNumber: "asc"` | `packages/shared/src/in-app-agent/server/persistence.ts:269-294` |
| Lifecycle CAS: claim QUEUED→RUNNING | `claimQueuedRun` | `packages/shared/src/in-app-agent/server/runLifecycle.ts:37-58` |
| Lifecycle CAS: heartbeat RUNNING | `heartbeatClaimedRun` with `cancelRequestedAt` pickup | `packages/shared/src/in-app-agent/server/runLifecycle.ts:64-84` |
| Lifecycle CAS: finish RUNNING→terminal | `finishClaimedRun` with terminal `errorCode`/`errorMessage` | `packages/shared/src/in-app-agent/server/runLifecycle.ts:93-124` |
| Approval continuation lineage | `parentRunId`, `rootRunId`, `traceStartedAt`, `approvalRequestedAt`, `continuationNumber` in request | `packages/shared/src/features/inAppAgent/types.ts:85-101` |
| Approval decision event | `InAppAgentApprovalDecisionSchema` + `buildInAppAgentApprovalDecisionEvent` | `packages/shared/src/in-app-agent/approvalEvents.ts:10-28` |
| Run status strict enum readers | `InAppAgentRunStatusSchema = z.enum(InAppAgentRunStatus)` plus `safeParse` calls | `packages/shared/src/features/inAppAgent/types.ts:21` and `runLifecycle.ts:431-435,483-486` |
| Settled vs unsettled | `IN_APP_AGENT_UNSETTLED_RUN_STATUSES`, `isSettledInAppAgentRunStatus`, `isUnsettledInAppAgentRunStatus` | `packages/shared/src/in-app-agent/constants.ts:12-39` |
| Reconciliation: classify stale | `classifyStaleRun` returns `{errorCode, errorMessage}` for queue_timeout, worker_lost, run_timeout, approval_expired | `packages/shared/src/in-app-agent/server/runLifecycle.ts:696-747` |
| Reconciliation CAS | `reconcileConversationRunsInTransaction` writes failure with status + heartbeatAt guard | `packages/shared/src/in-app-agent/server/runLifecycle.ts:627-685` |
| Tunables (timeouts) | `IN_APP_AGENT_HEARTBEAT_STALE_MS=60s`, `QUEUE_TIMEOUT_MS=5m`, `RUN_MAX_DURATION_MS=15m`, `APPROVAL_TTL_MS=24h`, `HEARTBEAT_INTERVAL_MS=5s`, `MAX_STEPS=20` | `packages/shared/src/in-app-agent/server/tunables.ts:1-23` |
| Run cancellation | `requestRunCancellation` returns `CancelRunResult` with cooperative cancel (`cancelRequestedAt`) vs immediate CAS | `packages/shared/src/in-app-agent/server/runLifecycle.ts:465-507` |
| Immediate-cancel statuses | `QUEUED`→CANCELLED, `AWAITING_APPROVAL`→CANCELLED; `RUNNING` sets `cancelRequestedAt` | `packages/shared/src/in-app-agent/server/runLifecycle.ts:516-573` |
| Cancellation races | `signalled` recheck guards QUEUED→RUNNING transition | `packages/shared/src/in-app-agent/server/runLifecycle.ts:558-573` |
| Event-type whitelist (persistence) | `toPersistableAgentEvent` exhaustive switch returning compact event per `EventType` | `packages/shared/src/in-app-agent/server/persistence.ts:739-932` |
| Drop-redirect events | `dropRedirectToolCallEvents` collapses redirect tool-call scaffolding | `packages/shared/src/in-app-agent/server/persistence.ts:1346-1396` |
| Flush triggers (run-finish, error, snapshot, tool-end) | `shouldFlushPersistedEvent` — TEXT_MESSAGE_END, TOOL_CALL_END, TOOL_CALL_RESULT, ACTIVITY_SNAPSHOT, REASONING_END, RUN_FINISHED, RUN_ERROR | `packages/shared/src/in-app-agent/server/persistence.ts:640-650` |
| Compaction: text deltas | `compactTextMessageChunks` collapses consecutive TEXT_MESSAGE_CHUNK for same messageId | `packages/shared/src/in-app-agent/server/eventCompaction.ts:5-33` |
| Compaction: reasoning deltas | `compactPersistedEventDeltas` collapses reasoning content/chunk events | `packages/shared/src/in-app-agent/server/eventCompaction.ts:35-62` |
| Run input / start event | `createQueuedRun` writes a `RUN_STARTED` event under the conversation lock | `packages/shared/src/in-app-agent/server/runLifecycle.ts:142-219` |
| `RUN_STARTED` normalization | `normalizeAdapterEvent` strips `input` from `RUN_STARTED`, propagates `parentRunId` | `worker/src/features/in-app-agent/runtime/agent.ts:1512-1526` |
| `RUN_ERROR` synthesis | `createRunErrorEvent` and `getRunErrorMessage` wrap errors into AG-UI shape | `worker/src/features/in-app-agent/runtime/agent.ts:1544-1579` |
| Event stream subscription | `MastraAgent` subscription drives `next/ error / complete` lifecycle on AG-UI stream | `worker/src/features/in-app-agent/runtime/agent.ts:700-836` |
| Persist hook | `onEvent` callback in `executeInAppAgentRun` feeds `pendingPersistedEvents` then `flushPendingRunEvents` | `worker/src/features/in-app-agent/executeInAppAgentRun.ts:446-489` |
| Persist hook filtering | Custom interrupt events written raw; everything else routed through `toPersistableAgentEvent`; `RUN_STARTED` skipped (already persisted in `createQueuedRun`) | `worker/src/features/in-app-agent/executeInAppAgentRun.ts:446-463` |
| Terminal callbacks | `onComplete` (SUCCEEDED/AWAITING_APPROVAL/STEP_LIMIT), `onAbort` (CANCELLED/WORKER_SHUTDOWN/OUTCOME_UNKNOWN), `onError` (AGENT_ERROR) | `worker/src/features/in-app-agent/executeInAppAgentRun.ts:491-561` |
| `OUTCOME_UNKNOWN` window | `approvedToolResultPersisted` flag toggled when TOOL_CALL_RESULT for the approved toolCallId is queued for persist | `worker/src/features/in-app-agent/executeInAppAgentRun.ts:160-167, 466-475` |
| Heartbeat timer | `IN_APP_AGENT_HEARTBEAT_INTERVAL_MS=5000ms` in worker loop | `worker/src/features/in-app-agent/executeInAppAgentRun.ts:412-424` |
| `InAppAgentRunStatus` schema validation in termination | `InAppAgentRunStatusSchema.safeParse` before cancel | `packages/shared/src/in-app-agent/server/runLifecycle.ts:431-435` |
| Tool-call typed wrapper | `parseInAppAgentInterruptEvent` parses CUSTOM event `on_interrupt` into `InAppAgentToolApprovalRequest` | `packages/shared/src/in-app-agent/interrupts.ts:16-37` |
| Approval request schema | `InAppAgentToolApprovalRequestSchema` with toolCallId/toolName/args/runId | `packages/shared/src/in-app-agent/schema.ts:221-227` |
| Test: lifecycle races | `runLifecycle.test.ts` covers reconciliation races, claim races, cancel races | `packages/shared/src/in-app-agent/server/runLifecycle.test.ts:30-342` |
| Test: persistence | `persistence.test.ts` covers silent MCP redaction and sandbox file accumulation | `packages/shared/src/in-app-agent/server/persistence.test.ts:1-109` |
| AG-UI base event schema | `BaseEventSchema` includes optional `timestamp`, `rawEvent`, `metadata` | `@ag-ui/core` `events.ts` `BaseEventSchema` |
| AG-UI `RUN_STARTED` schema | `RunStartedEventSchema` carries `threadId`, `runId`, `parentRunId?`, `input?` | `@ag-ui/core` `events.ts` `RunStartedEventSchema` |
| AG-UI `RUN_FINISHED` schema | `RunFinishedEventSchema` carries `threadId`, `runId`, optional `result`, `outcome` (interrupt), `usage[]` | `@ag-ui/core` `events.ts` `RunFinishedEventSchema` |
| AG-UI `RUN_ERROR` schema | `RunErrorEventSchema` carries `message`, optional `code`, optional `usage[]` | `@ag-ui/core` `events.ts` `RunErrorEventSchema` |
| ClickHouse observability events | `events_core` table with `start_time`, `end_time`, `event_ts`, `is_deleted`, `is_app_root` | `packages/shared/clickhouse/migrations/clustered/0040_create_events_core.up.sql:1-123` |
| ClickHouse events_full (raw) | Underlying full-fidelity table; MV `events_core_mv` projects it | `packages/shared/clickhouse/migrations/clustered/0039_create_events_full.up.sql:1-124`, `0041_create_events_core_mv.up.sql:1-69` |
| Event-log retirement | `event_log` dropped in favor of `blob_storage_file_log` (migration `0044`) | `packages/shared/clickhouse/migrations/clustered/0044_drop_event_log.up.sql:1-5` |
| Public events table schema | `eventsTableCols` defines typed columns exposed to tRPC/UI/export | `packages/shared/src/eventsTable.ts:65-481` |
| Event-table has-parent helper | `eventsTableHasParentObservationSql = "e.parent_span_id != ''"` | `packages/shared/src/eventsTable.ts:4` |
| Event-table root helper | `eventsTableIsRootObservationSql` (`parent_span_id=''` OR `is_app_root`) | `packages/shared/src/eventsTable.ts:5-10` |
| Span/trace IDs in events | `span_id`, `parent_span_id`, `trace_id`, `start_time`, `end_time` on every event row | `packages/shared/clickhouse/migrations/clustered/0040_create_events_core.up.sql:5-9` |
| Event timestamps | `created_at`, `updated_at`, `event_ts` with default `now()`, `is_deleted` flag | `packages/shared/clickhouse/migrations/clustered/0040_create_events_core.up.sql:95-98` |

## Answers to Dimension Questions

### 1. Are events typed and versioned?

**Yes.** The in-app agent runtime types every event through the `EventType` enum imported from `@ag-ui/core` (referenced at `packages/shared/src/in-app-agent/schema.ts:7` and `runLifecycle.ts:64`). Langfuse ships an exhaustive whitelist `toPersistableAgentEvent` that projects each known `EventType` into a compact persistable shape (`packages/shared/src/in-app-agent/server/persistence.ts:739-932`) and returns `null` for unrecognized types so they are silently dropped at persistence (with a typed passthrough for raw CUSTOM interrupt events at `executeInAppAgentRun.ts:447-457`). Status is a typed enum at the writer side (`packages/shared/src/features/inAppAgent/types.ts:12-19`) but stored as a plain `String` on Postgres (`schema.prisma:296`) so new states can ship without `ALTER TYPE`; readers tolerate unknown values via `safeParse` (`runLifecycle.ts:431-435,483-486,584-590`). The `error_code` column stays a free-form string for forward compatibility (`features/inAppAgent/types.ts:23-29`). AG-UI events also have an optional `metadata` and an optional `timestamp` on every base event (`@ag-ui/core` `BaseEventSchema`), though Langfuse does not surface a `version` field on persisted events; the closest thing is the `InAppAgentRunStatus` + `InAppAgentRunErrorCode` enums which constitute a versioned lifecycle vocabulary.

### 2. Are events ordered and timestamped?

**Yes for both.** Per-conversation monotonic ordering is enforced by a `sequenceNumber` integer assigned at persist time via `(latestEvent?.sequenceNumber ?? -1) + 1` (`packages/shared/src/in-app-agent/server/runLifecycle.ts:859`) or its `appendRunEvents` analogue (`persistence.ts:222-231`). The watch-cursor contract is explicit: the snapshot's high-water mark is the maximum over the events it returned, so subscribing with `> cursor` is gap-free (`persistence.ts:75-83`). Reads order by `sequenceNumber: "asc"` (`persistence.ts:279`). Each row carries a database-assigned `createdAt DateTime @default(now())` (`schema.prisma:273`), and the composite primary key `(projectId, conversationId, sequenceNumber)` (`schema.prisma:275`) prevents collisions. The optional `timestamp` field on AG-UI `BaseEventSchema` is not relied upon at the DB layer — the database clock is authoritative — which means replay across processes is consistent but the producer-side timestamp is not first-class in persistence.

### 3. Do events carry sufficient context?

**Yes for parent linkage.** Every persisted `InAppAgentEvent` row carries `projectId`, `conversationId`, `runId`, and the full JSON `event` body which embeds AG-UI fields like `toolCallId`, `messageId`, `threadId`, `runId`, `parentRunId` (`schema.prisma:264-272`). Approval decision events carry `toolCallId`, `approved`, `decidedByUserId` (`packages/shared/src/in-app-agent/approvalEvents.ts:10-14`). The run row carries `triggeredByUserId`, `model`, `mcpApiKeyId`, `cancelRequestedAt`, `finishedAt`, `errorCode`, `errorMessage`, `claimedAt`, `heartbeatAt`, and a JSON `request` payload (`schema.prisma:280-303`). Approval continuations thread a typed `parentRunId` / `rootRunId` / `traceStartedAt` / `approvalRequestedAt` / `continuationNumber` lineage (`packages/shared/src/features/inAppAgent/types.ts:85-101`) so `resolveInAppAgentRootRunId` can roll an approval-continuation chain back to its user-message root. Telemetry-side events go further with `parent_span_id`, `is_app_root`, `trace_id` so a Langfuse trace can be reconstructed from events alone.

### 4. Are lifecycle events comprehensive?

**Yes, with explicit fan-out for failure modes.** The lifecycle covers creation (`RUN_STARTED` event + `createdAt`), enqueue (`QUEUED`), claim (`claimedAt` + status→`RUNNING`), steady-state (`heartbeatAt`), parking (`AWAITING_APPROVAL` + `finishedAt` set), approval decision (`langfuse_approval_decision` CUSTOM event), success (`SUCCEEDED`), failure (`FAILED`), cancellation (`CANCELLED` with `errorCode`), timeout via reconciliation (`QUEUE_TIMEOUT`, `WORKER_LOST`, `RUN_TIMEOUT`, `APPROVAL_EXPIRED`), step exhaustion (`STEP_LIMIT`), and durable continuation via the approval chain. Heartbeat-based liveness (`HEARTBEAT_INTERVAL_MS=5s`, stale after `60s` — `tunables.ts:2,5`) plus run-duration cap (`RUN_MAX_DURATION_MS=15m` — `tunables.ts:14`) give the runtime three independent deadline detectors. `classifyStaleRun` is pure (`runLifecycle.ts:696-747`) so reconcilers and read paths cannot diverge.

## Architectural Decisions

- **Two-tier event model**: AG-UI in-app agent events (typed, durable, append-only) + ClickHouse `events_core` (columnar, public observability). The two are deliberately separate: AG-UI is the canonical conversation history, ClickHouse is the analytical surface.
- **Postgres-owned CAS over a queue-side state machine**: every run status transition is a Postgres CAS (`updateMany` + `count`-based fencing) under a per-conversation `FOR UPDATE` lock (`runLifecycle.ts:1280-1311`). The worker is a single subscriber; Postgres serializes correctness.
- **Cursor-by-max-not-timestamp**: the in-app agent uses `sequenceNumber` rather than `createdAt` for the watch cursor (`persistence.ts:75-83`) to avoid clock skew between DB and producer. Database `createdAt` is recorded for human display but not used for ordering.
- **Append-only event stream under a row lock**: every append takes `lockConversation` (`persistence.ts:1280-1311`), then bumps the conversation `updatedAt` (`runLifecycle.ts:872-880`), guaranteeing a single writer per conversation.
- **Lifecycle enums stored as plain strings, not PG enums**: new states and error codes can ship without migrations, with `safeParse` readers that tolerate unknown values (`features/inAppAgent/types.ts:5-11,23-29`).
- **Reconciliation as the source of truth for timeouts**: `reconcileConversationRuns` is the only authority that persists a stale→failed transition (`runLifecycle.ts:688-695`); read paths can apply `classifyStaleRun` without writing.
- **Outcome-aware cancellation**: instead of always recording `CANCELLED`, the worker distinguishes `OUTCOME_UNKNOWN` (approved mutation may have started but its result was not persisted) from plain `CANCELLED` / `WORKER_SHUTDOWN` (`executeInAppAgentRun.ts:160-167,548-561`).
- **Compaction at the persistence seam, not at the producer**: text and reasoning deltas are merged by `compactPersistedEventDeltas` (`eventCompaction.ts:35-62`) before they hit `InAppAgentEvent`, which keeps the runtime simple and the storage shape bounded.
- **Schema duplication from `@ag-ui/core`**: the in-app agent maintains its own Zod copies of AG-UI base schemas (`packages/shared/src/in-app-agent/schema.ts:9-30, 158-197`) because the upstream package publishes Zod v3 shapes against Langfuse's Zod v4. A tracked upgrade path exists.

## Notable Patterns

- **Run-input typed union**: `InAppAgentRunRequestSchema` is a `z.discriminatedUnion("kind", …)` with `userMessage` and `approvalDecision` variants; the latter carries the durable lineage fields (`features/inAppAgent/types.ts:79-102`). This is the single typed channel between web mutation and worker claim.
- **Watch-cursor semantics over sequence numbers**: the comment block at `persistence.ts:75-83` documents that the snapshot high-water mark is the max over the returned rows, so `> cursor` is gap-free by construction. This is a load-bearing design choice for the SSE/streaming surface.
- **Tool-call-result error stamping**: `normalizeAdapterEvent` augments the bridge's `TOOL_CALL_RESULT` with an `error` field when the unwrapped content is a tool failure message (`worker/src/features/in-app-agent/runtime/agent.ts:1528-1539`), but only when no top-level error is already present. This avoids both false negatives and double-stamping.
- **Fenced CAS returns count**: `claimQueuedRun`, `heartbeatClaimedRun`, `finishClaimedRun`, and approval-decision CAS all return `{count: 0}` semantics so the worker can detect a lost-ownership state without inspecting the row (`runLifecycle.ts:37-58,64-84,93-124,283-297`).
- **Redirect-tool-call event compression**: `dropRedirectToolCallEvents` retains only the successful `TOOL_CALL_RESULT` for the `langfuse_proposeRedirect` tool and drops its START/ARGS scaffolding (`persistence.ts:1346-1396`). The companion `dropRedirectActionToolResults` strips the corresponding tool messages from the rendered transcript (`persistence.ts:1398-1409`).
- **Event-type whitelist as a single audit point**: `toPersistableAgentEvent` lists every persisted event type by hand and returns `null` for explicit non-persisted types (`persistence.ts:907-928`) including the deprecated `THINKING_*` types and `STATE_SNAPSHOT`/`STATE_DELTA`/`ACTIVITY_DELTA`/`RAW`/`STEP_*`/`TOOL_CALL_CHUNK`/`REASONING_ENCRYPTED_VALUE`.

## Tradeoffs

- **Database-clock timestamp, not producer-clock**: replay across processes sees the database `createdAt` rather than the AG-UI optional `timestamp`. This removes skew between writers but makes it impossible to detect clock-frozen producers from the event stream.
- **`String` columns for status / error_code instead of PG enums**: forward compatibility is great, but readers cannot rely on `enum_in` filters at the SQL level — every consumer must round-trip through `safeParse`.
- **Out-of-band `mcpApiKeyId` cleanup**: the lifecycle state machine and the API-key cleanup are decoupled (`cleanupTerminalRunMcpApiKeys`), so a worker crash can leave orphan keys until the next reconcile (`runLifecycle.ts:757-803`). This is acceptable because the cleanup is idempotent and tolerates P2025, but it means run termination and key destruction are not atomic.
- **`RUN_STARTED` skipped in the runtime `onEvent` path** because `createQueuedRun` already wrote it under the lock (`executeInAppAgentRun.ts:461-463`). Two writers competing for the start event would be a bug, so the duplication-prevention is type-level, not constraint-level.
- **Per-conversation lock serializes throughput**: every event append and every cancellation holds `lockConversation` (`runLifecycle.ts:1280-1311`), so a single hot conversation can bottleneck. The read-side cursor approach still streams without locking.
- **Approval TTL is generous (24h)**: the timeout vocabulary is comprehensive, but the values skew long (`tunables.ts:5-17`), trading user-visible "approval lost" responsiveness for fewer false-positive expirations.

## Failure Modes / Edge Cases

- **Worker crash mid-run**: heartbeat stale after 60s → reconciler fails the run as `WORKER_LOST` (`runLifecycle.ts:715-734`). If a tool mutation already started but the result did not persist before crash, the next continuation records `OUTCOME_UNKNOWN` so the user verifies before retrying (`executeInAppAgentRun.ts:160-167`).
- **Run claimed between cancel-read and cancel-write**: `signalled` recheck in `runLifecycle.ts:558-573` re-runs the CAS against `RUNNING` so a worker that beat the cancel can still be cooperatively signalled.
- **Approval superseded by a new user message**: `createQueuedRun` immediately marks any parked `AWAITING_APPROVAL` run as `CANCELLED` with `APPROVAL_SUPERSEDED` and increments the cancelled counter (`runLifecycle.ts:162-174, 211-216`).
- **Approval race / replay**: `decideToolApproval` performs the parent CAS exactly once; a second decision returns `LangfuseConflictError("This approval was already decided…")` (`runLifecycle.ts:283-297`).
- **Approval expiry race**: if `parkedAt + TTL` has passed, the function CAS-transitions the parent to `FAILED/approval_expired` instead of creating a continuation (`runLifecycle.ts:262-281`).
- **Heartbeat-renew race**: the reconciler's CAS guards the prior `heartbeatAt` value so a worker that renewed its heartbeat between read and write cannot be killed (`runLifecycle.test.ts:128-170`).
- **Reconciled outcome after commit, before metric record**: the metric is recorded only after the transaction commits and only on the writer that won the CAS, so rolled-back flushes do not inflate counters (`persistence.ts:252-264`; `runLifecycle.ts:618-625`).
- **Dropped unknown event types**: an `EventType` value the runtime has never heard of returns `null` from `toPersistableAgentEvent` (`persistence.ts:931`) and is silently dropped from persistence but logged. Tests guard the silent path only via the typed cases.
- **Worker shutdown with in-flight approved mutation**: SIGTERM aborts all loops at their next step boundary (`executeInAppAgentRun.ts:73-77, 540-547`) and records `WORKER_SHUTDOWN` or `OUTCOME_UNKNOWN` depending on whether the approved tool result landed.
- **Event-log table dropped in v4**: `event_log` was retired in favor of `blob_storage_file_log` (migration `0044`); any v3→v4 upgrade assumes the `20250417_1737_migrate_event_log_to_blob_storage` background migration completed (`0044_drop_event_log.up.sql:1-4`).

## Future Considerations

- **Restore direct `@ag-ui/core` Zod schemas** once https://github.com/ag-ui-protocol/ag-ui/pull/1637 ships (`schema.ts:10-14`). The current duplication is a debt against the runtime.
- **Tighten event/run FK constraints**: there is no DB constraint linking `InAppAgentEvent.createdAt >= InAppAgentRun.createdAt` or `event.runId = run.id` at insert time. A unique key on `runId` + per-run sequence would close the gap if cross-process writers ever appear.
- **Surface AG-UI `timestamp`**: producer-side timestamps could be persisted in the JSON `event` body to allow detection of producer clock anomalies; today they are silently dropped by `toPersistableAgentEvent`.
- **Promote `pendingStateRunStatus` / `errorCode` reads to the column level**: a future migration could add a typed enum once the lifecycle stabilizes.
- **Observability of cursor gaps**: there is no test that detects a missing `sequenceNumber` in the persisted stream; a regression test for the watch-cursor contract would harden the streaming surface.
- **Eventual compaction of `InAppAgentEvent`**: text/reasoning deltas are merged before persist (`eventCompaction.ts:35-62`), but full-message snapshots and activity deltas are not. If the event stream grows linearly with turns, archival compaction may be needed.

## Questions / Gaps

- **Is there a documented retention / archival policy for `InAppAgentEvent`?** No retention migrations for the table were found under `packages/shared/prisma/migrations/`. The runtime has no built-in eviction.
- **Is there an event-versioning contract for AG-UI types beyond the discriminated `EventType`?** No `version` or `schema_version` field appears on `InAppAgentEvent` or on `BaseEventSchema`. Compatibility is achieved by exhaustive switches in `toPersistableAgentEvent` and `createConversationMessageAccumulator` rather than by an explicit version.
- **Are CUSTOM events other than approval decisions persisted?** `toPersistableAgentEvent` returns `null` for `EventType.CUSTOM` (`persistence.ts:912`), but `executeInAppAgentRun.ts:447-457` short-circuits to push raw CUSTOM interrupt events into the buffer before that filter. It is unclear whether other CUSTOM events from upstream adapters would land.
- **How does the worker know the high-water mark across process restarts?** `getConversationEvents` reads in ascending `sequenceNumber` (`persistence.ts:279`), and the client presumably tracks its cursor out of band. No in-repo SSE/Subscription cursor was located beyond the comment at `persistence.ts:75-83`.
- **Can a worker produce events for two conversations concurrently?** Yes — each `executeInAppAgentRun` is a separate BullMQ job with its own `lockConversation`, but the in-memory `pendingPersistedEvents` array (`executeInAppAgentRun.ts:427-438`) is per-run, not per-conversation, so a single worker can interleave flushes across jobs without violating per-conversation ordering.

---

Generated by `10.02-event-schema-and-lifecycle-events` against `langfuse`.
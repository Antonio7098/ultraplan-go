# Source Analysis: temporal

## Objective and Progress Tracking

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (distributed workflow orchestration server; gRPC APIs; pluggable DB/Elasticsearch persistence) |
| Analyzed | 2026-08-26 |

## Summary

Temporal does not track progress against an LLM-style plan; it treats each *workflow execution* as the unit of goal-bearing work and turns "progress" into a durable, verifiable ledger. The goal is represented by a `WorkflowExecutionState` (a coarse state machine plus a terminal `WorkflowExecutionStatus`) held inside per-execution mutable state (`service/history/workflow/mutable_state_state_status.go:16`). Every incremental step toward the goal is materialized as an immutable history event appended by a `HistoryBuilder` (`service/history/historybuilder/history_builder.go:424-432`), so progress is *reconstructable* rather than merely observable. Fine-grained liveness signals come from three mechanisms: activity heartbeats persisted into `ActivityInfo` (`service/history/workflow/mutable_state_impl.go:2105-2135`), custom `RecordMarker` commands (`service/history/api/respondworkflowtaskcompleted/workflow_task_completed_handler.go:1018-1040`), and visibility tasks that project execution status into an indexed store consumed by UI/listing APIs (`service/history/tasks/start_visibility_task.go:53-55`, `service/history/visibility_queue_task_executor.go:180,289`).

Completion is an explicit, single-shot, server-arbitrated event: the SDK emits a `CompleteWorkflowExecutionCommand`, the handler validates it, refuses duplicates after the first completion wins (`workflow_task_completed_handler.go:821-830`), appends the terminal event, flips state/status through a guarded transition function that makes `COMPLETED` terminal (`mutable_state_impl.go:4966-4982`, `mutable_state_state_status.go:74-92`), and emits completion metrics keyed by close status (`service/history/workflow/metrics.go:124-139`). The critical boundary: Temporal structurally verifies *every* progress claim (tokens, IDs, sequence validity, running-state checks) but deliberately does **not** semantically verify final success — `ValidateCompleteWorkflowExecutionAttributes` only checks the result payload exists (`service/history/api/command_attr_validator.go:219-228`). Correctness of "did we achieve the goal?" remains the workflow author's responsibility; the server guarantees the *record* of what happened cannot be forged or lost.

## Rating

**9 / 10**

Rationale: This is a mature, durable, observable progress-tracking model proven under failure and scale. Strengths: append-only event-sourced progress log with rebuild-from-history (`service/history/workflow/mutable_state_rebuilder.go:72-128`); enforced state-transition matrix making terminal states irreversible (`mutable_state_state_status.go:16-125`); duplicate-completion suppression with metrics (`workflow_task_completed_handler.go:821-830`); server-enforced liveness via heartbeat timeouts computed from persisted heartbeat times (`service/history/workflow/timer_sequence.go:405-444`); full observability surface (history API, Describe API with per-activity heartbeat data, visibility store, per-status completion metrics). It loses one point because final success is trusted, not independently checked — a worker can complete a workflow with an arbitrary result payload, and visibility projections are eventually consistent (async queue tasks), so the UI can briefly disagree with authoritative state.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Goal representation (execution state machine) | `setStateStatus` validates every state/status transition; `COMPLETED` rejects all moves except identical re-set | service/history/workflow/mutable_state_state_status.go:16-125 |
| Goal identity | Workflow key = NamespaceID + WorkflowId + RunId used to scope all progress updates | service/history/api/recordactivitytaskheartbeat/api.go:44-48 |
| Running check | `IsWorkflowExecutionRunning()` gates mutations on CREATED/RUNNING states | service/history/workflow/mutable_state_impl.go:2493-2498 |
| Progress as events | Completion/failure recorded by appending immutable events via HistoryBuilder | service/history/historybuilder/history_builder.go:424-446 |
| Heartbeat progress marker | `UpdateActivityProgress` stores `LastHeartbeatDetails` + `LastHeartbeatUpdateTime`, emits `ActivityPayloadSize`/`ActivityHeartbeatCount` metrics | service/history/workflow/mutable_state_impl.go:2105-2135 |
| Heartbeat validation | Heartbeat requires deserializable task token, active namespace, running workflow, matching pending activity; stale-state detected | service/history/api/recordactivitytaskheartbeat/api.go:25-75 |
| Heartbeat → cancel feedback loop | Response returns `CancelRequested`/`ActivityPaused` so workers learn to stop | service/history/api/recordactivitytaskheartbeat/api.go:97-101 |
| Liveness enforcement | `getActivityHeartbeatTimeout` derives a HEARTBEAT timeout timer from max(StartedTime, LastHeartbeatUpdateTime)+timeout | service/history/workflow/timer_sequence.go:405-444 |
| Custom progress markers | `handleCommandRecordMarker` → validated marker name → `AddRecordMarkerEvent` | service/history/api/respondworkflowtaskcompleted/workflow_task_completed_handler.go:1018-1040 |
| Command-sequence validation | `ValidateCommandSequence` runs before any SDK command applied | service/history/api/respondworkflowtaskcompleted/workflow_task_completed_handler.go:176-179; service/history/api/command_attr_validator.go:637 |
| Completion criteria (success path) | `handleCommandCompleteWorkflow`: buffered-events guard, attr validation, size check, first-wins duplicate guard, then `AddCompletedWorkflowEvent` | service/history/api/respondworkflowtaskcompleted/workflow_task_completed_handler.go:797-850 |
| Terminal event application | `ApplyWorkflowExecutionCompletedEvent` sets STATE_COMPLETED/STATUS_COMPLETED, records CloseTime, clears sticky queue | service/history/workflow/mutable_state_impl.go:4966-4982 |
| Failure/retry criteria | `handleCommandFailWorkflow` computes retry/cron backoff; failure becomes new run instead of reopened goal | service/history/api/respondworkflowtaskcompleted/workflow_task_completed_handler.go:852-920 |
| Invalid-command consequence | Bad commands fail the workflow task with typed causes (e.g. duplicate timer ID), not silently accepted | service/history/api/respondworkflowtaskcompleted/workflow_task_completed_handler.go:790-793 |
| Success semantics NOT verified server-side | `ValidateCompleteWorkflowExecutionAttributes` only rejects nil attributes | service/history/api/command_attr_validator.go:219-228 |
| Visibility projection (start) | `StartExecutionVisibilityTask` generated on start; executed via `RecordWorkflowExecutionStarted` | service/history/workflow/task_generator.go:410-420; service/history/visibility_queue_task_executor.go:129-186 |
| Visibility projection (close) | `CloseExecutionVisibilityTask` generated at close; executor reloads mutable state and records close with status | service/history/workflow/task_generator.go:240-246; service/history/visibility_queue_task_executor.go:236-303 |
| Searchable custom status | `GenerateUpsertVisibilityTask` projects memo/search-attribute updates for listing queries | service/history/workflow/task_generator.go:707-713 |
| UI trace API | `GetWorkflowExecutionHistory` returns paginated event stream; archived-history fallback | service/frontend/workflow_handler.go:958-1023,6700-6723 |
| UI status API | `DescribeWorkflowExecution` surfaces execution info, pending activities/children, pending workflow task | service/frontend/workflow_handler.go:3320-3372 |
| Per-activity progress in UI | `GetPendingActivityInfo` exports `LastHeartbeatTime`, `HeartbeatDetails`, attempt, retry timing | service/history/workflow/activity.go:92-149 |
| Listing APIs (open/closed) | `ListOpenWorkflowExecutions`/`ListClosedWorkflowExecutions` read the visibility store | service/frontend/workflow_handler.go:2537-2538,2638-2639 |
| Completion metrics | `workflow_success/cancel/failed/timeout/terminate/continued_as_new` counters + schedule-to-close latency | common/metrics/metric_defs.go:1092-1103; service/history/workflow/metrics.go:106-146 |
| Heartbeat metric | `activity_heartbeat_count` counter with `has_details` tag | common/metrics/metric_defs.go:992 |
| Rebuild from history | Mutable state can be rebuilt by replaying events (reset/replication) — progress log is authoritative over snapshots | service/history/workflow/mutable_state_rebuilder.go:72-135 |
| Tests: heartbeat contract | `TestHeartbeat` covers invalid/stale tokens rejected, details round-trip | tests/activity_standalone_test.go:5516-5560+ |
| Tests: heartbeat across retry | `TestActivityHeartbeatDetailsDuringRetry` verifies last heartbeat details survive retry | tests/activity_test.go:1180 |

## Answers to Dimension Questions

**1. What is the goal?**
A workflow execution. The goal itself is application-defined (workflow code + input), but the server represents its achievement state explicitly as `WorkflowExecutionState{State, Status}` (`service/history/workflow/mutable_state_state_status.go:16-19`) scoped by `definition.WorkflowKey` (`service/history/api/recordactivitytaskheartbeat/api.go:44-48`). There is no server-side decomposition of the goal into sub-goals; sub-steps exist only as activities/child workflows/events within the history.

**2. How is progress measured?**
Three channels: (a) *structural* — appended history events for every scheduled/started/completed step (`service/history/historybuilder/history_builder.go:424-446`); (b) *liveness* — activity heartbeats persisted with timestamps and details (`service/history/workflow/mutable_state_impl.go:2105-2135`) and enforced via heartbeat-timeout timers (`service/history/workflow/timer_sequence.go:405-444`); (c) *outcome* — terminal status set once via validated completion commands and counted per-status in metrics (`service/history/workflow/metrics.go:124-146`). Progress is thus measured by tool/task success plus time-based enforcement, not by model self-judgment alone.

**3. Can the model fake progress?**
Structurally, no; semantically, yes. A worker cannot fabricate progress without a valid task token bound to a live pending activity (`service/history/api/recordactivitytaskheartbeat/api.go:25-75`), cannot issue out-of-order commands (`command_attr_validator.go:637` sequence validation), and cannot double-complete (first-wins guard, `workflow_task_completed_handler.go:821-830`). But the *content* of success is self-reported: the server accepts any non-nil result payload for completion (`command_attr_validator.go:219-228`), and heartbeats carry arbitrary caller-supplied details — a busy-looping worker can emit heartbeats while doing nothing useful unless the author configures meaningful heartbeat semantics. Deterministic replay and the immutable history make any such claim permanently auditable after the fact, which is Temporal's chosen mitigation.

**4. Are blockers recorded?**
Yes, as typed failures and pending-state. Invalid commands produce typed `WORKFLOW_TASK_FAILED_CAUSE_*` events (`workflow_task_completed_handler.go:790-793`); activity failures are captured in retry state with `RetryLastFailure`/`RetryLastWorkerIdentity` (`service/history/workflow/activity.go:74-75`) and surfaced as pending-activity fields including next-attempt scheduling (`activity.go:116-134`); pauses are explicit flags (`ai.Paused`) visible in Describe output (`activity.go:136`). There is no dedicated "blocked reason" object beyond these.

**5. Is final success independently checked?**
No. Final success is whatever the workflow code reports via `CompleteWorkflowExecution`; the server verifies structural validity only (`workflow_task_completed_handler.go:805-811`) and arbitrates uniqueness/ordering (state machine + first-wins). Independent checks exist for *consistency*, not *correctness*: state can be rebuilt from history (`service/history/workflow/mutable_state_rebuilder.go:72-135`) and close-time visibility tasks re-read mutable state before recording closure (`service/history/visibility_queue_task_executor.go:255-289`). Semantic verification is delegated upward to callers reading the result, or sideways to cron/retry policies that re-run the goal.

## Architectural Decisions

1. **Event-sourced progress over status polling.** All progress is an append-only event stream built by `HistoryBuilder` (`service/history/historybuilder/history_builder.go:424-446`), enabling replay-based reconstruction (`mutable_state_rebuilder.go:72`) rather than trusting snapshots.
2. **Terminality enforced by a total state-transition function.** Instead of scattered checks, one function (`setStateStatus`) enumerates legal transitions, making completed runs immutable and zombie states explicit (`mutable_state_state_status.go:21-120`).
3. **Async visibility projection.** The authoritative store and the query/UI store are decoupled via queued visibility tasks (`task_generator.go:410-420,707-713` → `visibility_queue_task_executor.go:129-303`), trading read-your-writes consistency for isolation of expensive indexing from the critical write path.
4. **Server as arbiter of structure, not meaning.** Command handlers validate references, ordering, and size (`workflow_task_completed_handler.go:797-850`) while leaving outcome semantics to workflow authors (`command_attr_validator.go:219-228`).
5. **Liveness as a configurable timeout, not a heuristic.** Stalls are caught only if the author declares `HeartbeatTimeout`; the server mechanically converts heartbeat timestamps into timer tasks (`timer_sequence.go:405-444`) rather than inferring stuck-ness.

## Notable Patterns

- **First-wins completion arbitration:** extra completion/cancel commands after the first are logged with `MultipleCompletionCommandsCounter` and dropped (`workflow_task_completed_handler.go:821-830,876-885`).
- **Two-way progress channel:** heartbeat responses return `CancelRequested`/`ActivityPaused`/`ActivityReset`, so progress reporting doubles as cancellation polling (`recordactivitytaskheartbeat/api.go:38-40,97-101`).
- **Retry-as-new-goal:** failure with retry/cron policy closes the current run with `CONTINUED_AS_NEW`-style new run ID instead of mutating the closed run (`workflow_task_completed_handler.go:887-916`).
- **Per-attempt heartbeat reset:** activity resets can clear heartbeat details for the new attempt while still accepting beats from the old one (`service/history/workflow/activity.go:76-85`).
- **Status-tagged completion metrics emitted only when events were actually written**, avoiding metric inflation from no-op transactions (`transaction_impl.go:784-806`).

## Tradeoffs

- **Durability vs. semantic safety:** the event ledger makes progress unfakeable and auditable, but provides zero help deciding whether the reported result is *correct*; garbage-in success is durably recorded as success.
- **Consistency vs. scalability in visibility:** list/search results are eventually consistent because they flow through async queue tasks (`visibility_queue_task_executor.go:236-303`); a just-closed workflow may still appear open in listings.
- **Explicitness vs. burden:** heartbeat timeouts, markers, and search-attribute upserts are opt-in; teams that skip them get weaker stall detection and poorer UI progress (`timer_sequence.go:420-423` bails when no heartbeat timeout configured).
- **Immutability vs. convenience:** correcting a wrong terminal outcome requires reset/new-run mechanics rather than editing history.

## Failure Modes / Edge Cases

- **Stale cache on heartbeat:** extreme persistence failures can leave stale workflow caches; detected via `scheduledEventID >= GetNextEventID()` and retried with `ErrStaleState` (`recordactivitytaskheartbeat/api.go:64-71`).
- **Duplicate completion races:** concurrent tasks emitting multiple completions are suppressed, but only after the first lands; the loser is silently dropped with a warn log (`workflow_task_completed_handler.go:821-830`).
- **Visibility resurrection ordering:** a close task executing after a delete task would resurrect the visibility record; handled with explicit ordering logic (`visibility_queue_task_executor.go:341-342`).
- **Zombie executions:** state machine includes ZOMBIE transitions so lost leadership cannot silently mutate dead runs (`mutable_state_state_status.go:93-117`).
- **Unbounded "progress" without heartbeat timeouts:** long-running activities with no configured timeout never fail regardless of heartbeat staleness (`timer_sequence.go:420-423`).

## Future Considerations

- Surface a first-class "progress percentage"/milestone primitive instead of relying on conventions like markers and searchable memo (`task_generator.go:707-713`).
- Optionally verify completion payloads against declared schemas or acceptance predicates at completion time, closing the self-reported-success gap (`command_attr_validator.go:219-228`).
- Provide a tunable consistency mode for visibility reads where callers need read-your-writes on close status (`visibility_queue_task_executor.go:294-301`).

## Questions / Gaps

- No evidence found in this source for server-side evaluation of workflow *results* against expected outcomes (searched `service/history/api/command_attr_validator.go` and respond-workflow-task-completed handlers; validation is structural only).
- The web UI itself is not part of this repository; UI-progress claims here rest on the server-side APIs that feed it (`service/frontend/workflow_handler.go:2537,2638,3320`) — actual rendering behavior could not be inspected within the source boundary.
- Whether archival (`archival_queue_task_executor.go`) preserves heartbeat details long-term was not traced end-to-end; only task plumbing was confirmed.

---

Generated by dimension 06.05 (Objective and Progress Tracking) against `temporal`.

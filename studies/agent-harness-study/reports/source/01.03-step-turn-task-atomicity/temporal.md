# Source Analysis: temporal

## 01.03 Step, Turn, and Task Atomicity

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (Temporal Server — distributed workflow engine; ~10k+ LoC for MutableState alone) |
| Analyzed | 2026-07-02 |

## Summary

Temporal is a *workflow orchestration* system, not an "agent harness," so the vocabulary is biased toward durable workflow execution rather than the LLM-style "step / turn / tool call / observation." Nevertheless, the codebase reveals **several nested layers of atomic units** with different semantics for persistence, retry, tracing, and user visibility:

- **Task** (`service/history/tasks/task.go:14`) — the smallest *persisted* unit, identified by `(TaskID, Category)`.
- **Executable** (`service/history/queues/executable.go:44`) — the *in-memory, retried, traced* unit wrapping a Task; carries state, attempt counter, span, and DLQ terminal-failure flag.
- **Workflow Task "turn"** — a *logical* unit representing one worker round-trip: scheduled → started → completed/failed/timed-out (`service/history/workflow/workflow_task_state_machine.go:39`).
- **HSM `MachineTransition`** (`service/history/hsm/tree.go:579`) — the explicit "atomic state transition" of a single state-machine node within a workflow execution; has rollback semantics.
- **CHASM Transition** (`chasm/engine.go:16`, `docs/architecture/chasm.md:216`) — the explicitly named "atomic unit of state change in CHASM" that operates on the entire Execution tree, written as one DB write.
- **MutableState transaction** (`service/history/workflow/mutable_state_impl.go:7451`, `service/history/workflow/context.go:241`) — the *persistence* atomic unit for a workflow; pairs `StartTransaction` → `CloseTransactionAsMutation|Snapshot` and commits mutable state + history events + queued tasks in one DB write.
- **Workflow Execution** (shard-owned, `service/history/workflow/context.go:35`) — the *concurrency* atomic unit; one in-memory `ContextImpl` per `(namespace, workflowID, runID)` guarded by a `locks.PrioritySemaphore(1)` (`service/history/workflow/context.go:73`).

The atomic units are **not unified** across the four concerns (persistence, retry, trace, UI). The persistence boundary is the DB transaction (Mutation/Snapshot), the retry boundary is the `Executable` lifecycle (`Ack`/`Nack`/`Reschedule`), the trace boundary is the OTEL span wrapped around `Executable.Execute()`, and the UI boundary is the Workflow Task "turn" observed via `RespondWorkflowTaskCompleted`. Tool calls (in the LLM sense) are *not* a first-class concept here — the closest analog is `internalTask` in matching (`service/matching/task.go:51`), which represents an in-flight workflow/activity task being matched to a poller, with its own state and eviction semantics.

## Rating

**8/10 — Clear, multi-layered, durable, observable, extensible model with explicit interfaces, tests, and operational safeguards.** Below 9 because the four concerns are not unified into a single "step" abstraction (different sub-systems use slightly different units), and because the HSM/CHASM subsystems are still evolving and have transitional code paths (e.g., `TODO` comments at `service/history/tasks/task.go:27-30` and `service/history/hsm/tree.go:55-58`).

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Task interface (smallest persisted unit) | `Task` interface with `GetTaskID`, `GetCategory`, `GetType`, `GetVisibilityTime` | `service/history/tasks/task.go:14-31` |
| Task Category persistence IDs (DB-persisted) | Persisted in DB; "WARNING: These IDS are persisted in the database. Do not change them." | `service/history/tasks/category.go:18-28` |
| Executable interface (in-memory retried unit) | `Executable` embeds both `ctasks.Task` and `tasks.Task`; carries Attempt, Priority, ScheduledTime | `service/history/queues/executable.go:43-53` |
| Executable task states (5-state FSM) | `TaskStatePending`, `TaskStateAborted`, `TaskStateCancelled`, `TaskStateAcked`, `TaskStateNacked` | `common/tasks/state.go:6-17` |
| Workflow Task turn (logical round-trip) | `workflowTaskStateMachine` orchestrates Scheduled→Started→Completed/Failed/TimedOut | `service/history/workflow/workflow_task_state_machine.go:39-60`, `761-859` |
| HSM atomic state transition | `MachineTransition` increments `TransitionCount`, captures `LastUpdateVersionedTransition`, **rollback on error** | `service/history/hsm/tree.go:579-629` |
| HSM OperationLog (operations persisted with state changes) | `OperationLog`, `TransitionOperation`, `DeleteOperation` types | `service/history/hsm/tree.go:96-148` |
| CHASM transition is documented as atomic unit | "A Transition is the atomic unit of state change in CHASM. ... All Field writes and Task schedules from a transition commit together as a single database write, or roll back entirely." | `docs/architecture/chasm.md:216-238` |
| CHASM `Engine.UpdateComponent` transition wrapper | Public Engine entry points for atomic transitions | `chasm/engine.go:16-59`, `service/history/chasm_engine.go:520-546` |
| MutableState transaction (persistence boundary) | `StartTransaction` checks for dirty tx; `CloseTransactionAsMutation`/`CloseTransactionAsSnapshot` flushes | `service/history/workflow/mutable_state_impl.go:7451-7482`, `7484-7528`, `7530-7573` |
| Workflow lock (concurrency atomic boundary) | `ContextImpl.lock` is `locks.NewPrioritySemaphore(1)` — single permit | `service/history/workflow/context.go:73` |
| Tracing atomic unit | OTEL span wraps the entire `Executable.Execute()`; tag includes `queue.task.id` and `queue.task.type` | `service/history/queues/executable.go:259-300` |
| Retry atomic unit | `Nack`/`Reschedule` on Executable; rescheduler holds backoff; `IsRetryableError` always false (never retry holding the goroutine) | `service/history/queues/executable.go:623-636`, `680-715` |
| Activity retry (separate Task unit) | `ActivityRetryTimer` is its own task type; activity retries are not inline retry of the original Executable | `service/history/tasks/activity_retry_timer.go` (file header) |
| Speculative WT: zero-DB-write "turn" | Speculative Workflow Task is *never written to DB*, lives in memory only, special timeout queue | `docs/architecture/speculative-workflow-task.md:33-48`, `service/history/queues/speculative_workflow_task_timeout_executable.go:30-55` |
| Transactional Outbox pattern | "Consistency of Mutable State and History Tasks is achieved via use of database transactions. ... Transfer Task queue processor ... ensures Transfer Tasks ... result in the required Workflow or Activity Task being created in Matching Service (Transactional Outbox Pattern)" | `docs/architecture/history-service.md:294-322` |
| VersionedTransition as global ordering of changes | "Provides a total ordering of all state changes, even across data centers." (FailoverVersion + TransitionCount) | `docs/architecture/chasm.md:277-296` |
| Panic recovery during atomic task execution | `defer recover()` inside `executableImpl.Execute()` converts panic into error so the task can still be Acked/Nacked | `service/history/queues/executable.go:302-315` |
| DLQ terminal-failure (drop atomic unit) | `terminalFailureCause` causes subsequent `Execute()` call to write to DLQ instead of re-running | `service/history/queues/executable.go:344-396`, `service/history/queues/dlq_writer.go:64-143` |
| `internalTask` (matching) — closest to "tool call" analog | `internalTask` is a union type with `event`/`query`/`nexus`/`started` arms, carries priority, response channel, eviction marker | `service/matching/task.go:51-103` |
| Update registry (sub-turn atomic unit) | Per-Update state machine: `stateCreated → stateAdmitted → stateAccepted → stateCompleted`, with explicit lifecycle | `service/history/workflow/update/update.go:69-130` |
| Task ack/nack path (commit vs. retry at Executable level) | `Ack` flips state to `TaskStateAcked`, emits TaskAttempt/TaskLatency; `Nack` re-submits or reschedules | `service/history/queues/executable.go:656-715` |
| Activity lifecycle (sub-task FSM) | `PENDING_ACTIVITY_STATE_CANCEL_REQUESTED`, `_STARTED`, `_SCHEDULED` | `service/history/workflow/activity.go:53-61` |
| `WorkflowExecutionState` atomicity guards | ContextImpl rejects dirty transactions: `MutableState encountered dirty transaction` error | `service/history/workflow/mutable_state_impl.go:7454-7463` |
| HSM rollback semantics on transition failure | Defer restores `TransitionCount`, `LastUpdateVersionedTransition`, and forces reload | `service/history/hsm/tree.go:599-607` |
| CHASM task validator/executor pair | Each task has `Validator` (discard stale) + `Executor` (carry out work) | `docs/architecture/chasm.md:262-267` |
| `IsRetryableError` deliberately returns false | "never retry task while holding the goroutine, and rely on shouldResubmitOnNack" | `service/history/queues/executable.go:623-628` |
| `backoffDuration` retry policy by error class | Different reschedule policies for busy workflow / dependency / resource exhausted / internal errors | `service/history/queues/executable.go:796-825` |
| Events notification (post-transaction visibility) | `NotifyNewHistoryEvent` channels new committed events to subscribers (queries, UI) | `service/history/events/notifier.go:28-67` |
| `MutableState.IsDirty` (in-memory dirty tracking) | Used to decide whether a transaction needs flushing | `service/history/historybuilder/history_builder.go:155-157` |
| Persistence error handling (`OperationPossiblySucceeded`) | On ambiguous error (e.g., timeout) still notifies the snapshot, treating op as possibly succeeded | `service/history/workflow/transaction_impl.go:78-82` |
| `stateMachineRef` token for callback staleness checks | `initialVT` and `lastUpdateVT` guard against stale callback updating wrong node | `docs/architecture/chasm.md:316-321` |
| CHASM validateAccess traversal (intent-based gating) | `OperationIntentProgress` enables writes; `OperationIntentObserve` is read-only | `chasm/tree.go:486-563` |

## Answers to Dimension Questions

1. **What is the atomic unit of execution?**
   There is no single atomic unit. The system has at least six nested units, each with explicit semantics:
   - **DB Transaction** (MutableState `CloseTransactionAsMutation|Snapshot`) — persistence atomic unit (`service/history/workflow/mutable_state_impl.go:7484`, `7530`).
   - **Executable** (`service/history/queues/executable.go:44`) — retry + tracing + queueing unit.
   - **Task** (`service/history/tasks/task.go:14`) — DB row + scheduler routing unit (category + type + ID).
   - **Workflow Task "turn"** (`service/history/workflow/workflow_task_state_machine.go:39`) — user-visible progress unit.
   - **HSM `MachineTransition`** (`service/history/hsm/tree.go:579`) — per-node state-change unit with rollback.
   - **CHASM Transition** (`chasm/engine.go:16`, `docs/architecture/chasm.md:216`) — officially named "atomic unit of state change in CHASM."

2. **Is the atomic unit the same for persistence, tracing, retry, and UI?**
   **No**, but the relationships are well-defined:
   - *Persistence* = DB transaction (MutableState `Snapshot`/`Mutation` + tasks in outbox).
   - *Tracing* = OTEL span wrapped around one Executable's `Execute()` call (`service/history/queues/executable.go:276-298`).
   - *Retry* = Executable-level `Nack`/`Reschedule` lifecycle, with extra `ActivityRetryTimer` and `WorkflowTaskTimer` for higher-level retries (`service/history/queues/executable.go:680-715`, `service/history/tasks/activity_retry_timer.go`).
   - *UI / user observation* = Workflow Task turn (Scheduled→Started→Completed); plus history events via `events.Notifier` (`service/history/events/notifier.go:28`).
   The mapping is "**one Executable ≈ one Task ≈ one DB transaction's queued-task-emission; multiple Executables (across many transactions) → one Workflow Task turn**."

3. **Can partially completed steps exist?**
   At the **DB level**: no — `StartTransaction` rejects dirty state (`service/history/workflow/mutable_state_impl.go:7454-7463`); `CloseTransaction*` either flushes everything or errors before flush. The CHASM doc explicitly states "All Field writes and Task schedules from a transition commit together as a single database write, or roll back entirely" (`docs/architecture/chasm.md:234`).
   At the **executable level**: yes — a panicking task is caught and the Executable still goes through `Nack`/`Ack` (see `service/history/queues/executable.go:302-315`).
   At the **workflow turn level**: yes — `WORKFLOW_TASK_TYPE_TRANSIENT` and `WORKFLOW_TASK_TYPE_SPECULATIVE` turns exist transiently (in memory only) and can be discarded without DB writes (`docs/architecture/speculative-workflow-task.md:33-48`).

4. **What happens if a crash occurs mid-step?**
   - **In Executable.Execute**: panic is recovered, metric emitted, error returned (`service/history/queues/executable.go:302-342`). The Executable is then Nacked → rescheduled.
   - **In HSM MachineTransition**: deferred rollback restores `TransitionCount` and `LastUpdateVersionedTransition`, forces cache reload (`service/history/hsm/tree.go:599-607`).
   - **In CHASM Transition**: "Tasks are written atomically with the component state (transactional outbox pattern), guaranteeing no work is silently lost on crash" (`docs/architecture/chasm.md:244`). Errors propagate; `MutableState` `cleanupTransaction()` resets in-memory buffers (`service/history/workflow/mutable_state_impl.go:8327-8387`).
   - **In Matching `internalTask`**: `setEvicted()` atomically swaps the matcher remove-func to `removeFuncEvicted` (`service/matching/task.go:338-342`); a duplicate `RecordTaskStarted` returns `serviceerror.NewNotFound`.
   - **In Workflow `ContextImpl`**: `Clear()` is called in defer on any error during `CreateWorkflowExecution` (`service/history/workflow/context.go:270-274`); mutable state is reloaded from DB on next access.

5. **Are tool calls their own atomic units?**
   In the LLM-agents sense, **no** — Temporal's design predates that vocabulary. The closest analog is **`internalTask`** in matching (`service/matching/task.go:51-103`), which is a per-poll unit carrying priority, response channel, eviction marker, and source. Its atomicity properties:
   - *Eviction* is atomic via `compare-and-swap` on `removeFromMatcher` (`service/matching/task.go:333-342`).
   - *Finish* (sync-match) sends a `taskResponse` over the buffered channel and may recycle a rate-limit token if dispatch was invalid (`service/matching/task.go:344-375`).
   - *Lifecycle*: a task moves Pending → Started → Completed/Failed/Timed-out; "in-flight" is held by the poller but the *matcher* releases the slot on `setEvicted`.
   Other sub-task units: **Update** (`service/history/workflow/update/update.go:33-66`) with `stateCreated → stateAdmitted → stateAccepted → stateCompleted/Rejected/Aborted`, and **Activity** (`service/history/workflow/activity.go:53-61`) with `SCHEDULED → STARTED → CANCEL_REQUESTED`.

## Architectural Decisions

- **Task is the persisted unit; Executable is the runtime unit.** (`service/history/queues/executable.go:44-53`, `service/history/tasks/task.go:14`) — decouples DB schema from in-memory lifecycle.
- **Transactional outbox for state-and-task durability.** (`docs/architecture/history-service.md:322`, `docs/architecture/chasm.md:244`) — Mutable State, History Events, and queued Tasks commit in one DB transaction; Transfer Tasks later move work to Matching Service. Eliminates "lost task" or "applied state without task" inconsistencies.
- **Workflow Task is the user-visible progress unit, with three flavors** (Normal, Transient, Speculative) (`docs/architecture/speculative-workflow-task.md:1-48`) — Normal persists events; Transient emits Scheduled+Started in response without DB write; Speculative is wholly in-memory and is used by Update to achieve zero-write rejection.
- **HSM introduces per-node transitions with explicit rollback.** (`service/history/hsm/tree.go:579-629`) — single-machine transitions are atomic; multi-node changes via the same MutableState transaction get the same DB atomicity.
- **CHASM makes Transition a first-class public API.** (`chasm/engine.go:16-59`, `docs/architecture/chasm.md:216-238`) — replacing the legacy "MutableState + EventStore" pattern with a typed, generic framework; has clear outbox semantics and `VersionedTransition` logical clock.
- **Speculative WT deliberately sacrifices DB durability for latency.** (`docs/architecture/speculative-workflow-task.md:33-87`) — used only for Update; falls back to Normal WT after a 5-second timeout. Special `speculative_workflow_task_timeout_queue` (`service/history/queues/speculative_workflow_task_timeout_queue.go`) handles the timeout.
- **Per-shard Workflow Context with single-permit semaphore.** (`service/history/workflow/context.go:73`) — guarantees one writer per `(ns, workflowID, runID)` at a time; allows priority preemption via `locks.PrioritySemaphore`.
- **DLQ as terminal failure destination.** (`service/history/queues/dlq_writer.go:64`, `service/history/queues/executable.go:344-396`) — terminal failures are routed to a per-category DLQ; otherwise dropped (no infinite retry loop).
- **Panic-recover at Executable boundary.** (`service/history/queues/executable.go:302-315`) — protects scheduler goroutine from user-implementation panics; metrics still emitted.
- **`OperationPossiblySucceeded` recovery semantics.** (`service/history/workflow/transaction_impl.go:78`) — on ambiguous errors (timeouts), callers notify snapshot anyway because the operation may have succeeded at the DB.
- **HSM TODO acknowledges legacy versioning.** (`service/history/tasks/task.go:27-30`, `service/history/hsm/tree.go:55-58`) — `HasVersion` and `CompareState` are transitional; long-term goal is transition-history-only.
- **CHASM access is intent-gated.** (`chasm/tree.go:486-563`) — `OperationIntentProgress` enables writes; `OperationIntentObserve` is read-only; ancestors are walked for closed/paused state checks.

## Notable Patterns

- **5-state Task FSM** (`common/tasks/state.go:6-17`) — clean separation between pending, aborted, cancelled, acked, nacked; transitions are guarded by a Mutex (`service/history/queues/executable.go:236-240`, `638-654`).
- **Operation log + compact** (`service/history/hsm/tree.go:711-805`) — `TransitionOperation`/`DeleteOperation` are stored chronologically; `compact()` filters operations that fall under deleted subtrees, so a deleted component's history is correctly excluded.
- **ExecutionLease pattern** (`service/history/chasm_engine.go:531-546`) — explicit acquire/lease/release with deferred cleanup (`defer func() { executionLease.GetReleaseFn()(retError) }`).
- **Builder pattern for history events** (`service/history/historybuilder/history_builder.go:32-94`) — events are accumulated in memory, then flushed via `Finish()` into `HistoryMutation` with DB buffer/clear flags.
- **Mutable state visitor with deferred flush** (`service/history/workflow/context.go:241-308`) — `CreateWorkflowExecution` defers `c.Clear()` on error to guarantee in-memory state is not left dirty.
- **Per-category task queue family** (`service/history/tasks/category.go:21-44`) — Transfer, Timer, Replication, Visibility, Archival, MemoryTimer, Outbound; each has its own queue type (Immediate vs. Scheduled) and persistence semantics.
- **OTEL span conditional on telemetry enabled** (`service/history/queues/executable.go:259-300`) — avoids allocation overhead when OTEL is off; span name includes task label with CHASM archetype fallback.
- **Speculative-task special-case Executable** (`service/history/queues/speculative_workflow_task_timeout_executable.go`) — overrides Ack/Nack/Abort to no-op because the underlying state is in-memory only.
- **VersionedTransition logical clock** (`docs/architecture/chasm.md:277-296`) — `(FailoverVersion, TransitionCount)` provides total ordering across data centers.
- **CHASM ref-with-staleness** (`docs/architecture/chasm.md:316-321`) — `initialVT` and `lastUpdateVT` guards on `ComponentRef` prevent stale callbacks from updating wrong nodes.

## Tradeoffs

- **Multiple atomic-unit abstractions** increases cognitive load but enables per-sub-system optimization (e.g., Speculative WT saves a DB round-trip; HSM rollback is cheaper than a full DB transaction).
- **One Executable = one Task = one DB write's emitted task** is the dominant pattern, but **Activity retry** uses a separate `ActivityRetryTimer` task, breaking the 1:1 mapping. (`service/history/tasks/activity_retry_timer.go`.)
- **`IsRetryableError` always returns `false`** (`service/history/queues/executable.go:623-628`) — explicit design choice: "never retry task while holding the goroutine, and rely on shouldResubmitOnNack." Trades simplicity for slightly higher reschedule latency.
- **Speculative WT can fail with a transient WorkflowTaskFailed event** if the worker sees inconsistent history (`docs/architecture/speculative-workflow-task.md:49-59`) — known minor tradeoff documented in TODO.
- **CHASM speculation** (`chasm/engine.go:146-150`) defers persistence until the next non-speculative transition — saves writes but risks more complex recovery if the speculative chain never gets committed (mitigated by timeouts).
- **Operation log compaction** (`service/history/hsm/tree.go:713-727`) requires walking the entire log on every close — O(operations) overhead at close time but constant memory at rest.
- **Event-notifier channel is bounded** (`service/history/events/notifier.go:25` — `eventsChanSize = 1000`) — back-pressure falls on the producer; risk of dropped notifications if subscribers are slow.
- **DLQ terminal failure is opt-in via dynamic config** (`service/history/queues/executable.go:175-181`, `:586-595`) — without DLQ, terminal errors are silently dropped (`return nil`), which can mask data corruption. The matchDLQErrorPattern path (`service/history/queues/executable.go:600-621`) lets ops route specific error patterns to DLQ.
- **MutableState `StartTransaction` rejects dirty state with `Unavailable`** (`service/history/workflow/mutable_state_impl.go:7454-7463`) — caller must retry; potential availability issue under heavy contention.
- **`OperationPossiblySucceeded` recovery** (`service/history/workflow/transaction_impl.go:78-82`) — on ambiguous errors, snapshot is still notified, accepting the risk of double-notification.

## Failure Modes / Edge Cases

- **Dirty MutableState** → `Unavailable` error on next transaction (`service/history/workflow/mutable_state_impl.go:7454-7463`); CHASM's `IsDirty()` recursively walks the tree (`service/history/hsm/tree.go:230-240`).
- **Panic in Executable.Execute** → captured in `defer recover()`, error returned, task Nacked for retry (`service/history/queues/executable.go:302-315`, `:680-715`).
- **Mid-execution shard ownership loss** → `shouldResubmitOnNack` returns false (`service/history/queues/executable.go:783-785`), task is rescheduled with longer backoff.
- **Mid-transition HSM error** → rollback defers restore `TransitionCount` and `LastUpdateVersionedTransition`, forces data reload (`service/history/hsm/tree.go:599-607`).
- **CHASM task validation after success** — TODO note: "a task validator must succeed validation after a task executes successfully (without error), otherwise it will generate an infinite loop." (`chasm/tree.go:3307-3312`.)
- **Stale callback via ComponentRef** → guarded by `initialVT` and `lastUpdateVT` (`docs/architecture/chasm.md:316-321`).
- **Delete operation on a deleted node** → `ErrStateMachineInvalidState` (`service/history/hsm/tree.go:407-409`).
- **Speculative WT timeout** → convert to normal WT, persist events (`docs/architecture/speculative-workflow-task.md:81-83`).
- **CHASM `Access` on paused/closed ancestor** → `errAccessCheckFailed` (`chasm/tree.go:486-563`).
- **WorkflowTaskStateMachine transient attempt** — TODO comment notes `WORKFLOW_TASK_TYPE_TRANSIENT` is unused; transient detection is via `IsTransientWorkflowTask()` + attempt count (`docs/architecture/speculative-workflow-task.md:30-32`).
- **Concurrent `RecordTaskStarted`** in matching → `serviceerror.NewNotFound` returned for the duplicate; `internalTask.setEvicted()` evicts the prior dispatch (`service/matching/fair_task_reader.go:139`, `service/matching/task.go:338-342`).
- **DLQ write failure** → metric `TaskDLQFailures` recorded; error returned; terminal task likely surfaces as retry-on-failure or drop (`service/history/queues/executable.go:391-393`).

## Future Considerations

- **Unified "step" abstraction** — currently persistence, retry, trace, and UI use different units. A higher-level "Step" primitive that ties `Executable + Transaction + OTEL span + Workflow Task turn` together could simplify cross-system reasoning. The CHASM Transition is a step in that direction (`chasm/engine.go:16`).
- **HSM legacy version/compare cleanup** — TODO comments indicate `HasVersion` and `CompareState` should be removed once transition history is fully enabled (`service/history/hsm/tree.go:55-58`).
- **All tasks should carry a versioned transition** — TODO at `service/history/tasks/task.go:27-30` would unify task staleness checks with the MutableState versioned-transition mechanism.
- **CHASM speculation persistence model** — TODO note at `chasm/engine.go:144-146`: "we need to figure out a way to run the tasks generated in a speculative transition." Speculative writes currently rely on timeouts to convert to normal transitions.
- **Speculative WT for Queries** — TODO at `docs/architecture/speculative-workflow-task.md:65-66`: "The task processing for Queries could be replaced by using speculative Workflow Tasks under the hood."
- **CHASM per-component observability** — `validateAccess` traversal is O(ancestor depth) per access; can be costly for deep trees.
- **Task retry taxonomy** — only `ActivityRetryTimer` exists as a separate retry task; future task types (e.g., CHASM callbacks) may benefit from a unified retry-via-new-task pattern.
- **Event-notifier channel overflow** — bounded at 1000 (`service/history/events/notifier.go:25`); future may need back-pressure-aware drop or coalescing.

## Questions / Gaps

- **How does the system answer "what completed and what didn't" for a multi-task transaction?** The DB transaction is atomic, so all-or-nothing. But across transactions (e.g., a Workflow Task that schedules 5 activities, then fails), each activity is its own atomic unit. The system relies on history events + tasks to reconstruct progress; explicit "transaction receipt" is not exposed beyond `StateTransitionCount` on the MutableState.
- **Is there a single "step ID" exposed to users/clients?** No direct analog of an LLM step ID. The nearest are: `WorkflowTaskInfo.ScheduledEventID` (turn ID), `TaskID` (queue task ID), `TransitionCount` (HSM transition counter).
- **What does the system do if a Task is generated in a transaction that never commits?** Tasks are committed only via `CloseTransactionAsMutation|Snapshot` (`service/history/workflow/mutable_state_impl.go:7484-7573`); abandoned MutableStates drop their queued tasks via `Clear()` (`service/history/workflow/context.go:102-113`).
- **Is there a global ordering across Executables?** Within one shard, yes (by `LastUpdateVersionedTransition`); across shards, no global ordering but per-namespace failover-version + transition-count provides cross-DC total order for state changes (`docs/architecture/chasm.md:279-296`).
- **Is there a public "atomic step" API for SDKs?** The SDK-visible APIs are RPCs (StartWorkflow, SignalWorkflow, RespondWorkflowTaskCompleted, etc.); the atomic unit visible to the SDK is the Workflow Task turn — `RespondWorkflowTaskCompleted` returns success only after the transaction commits.
- **HSM/CHASM overlap** — both provide atomic transitions; HSM is the older framework, CHASM is the new one. Migration is in progress; some components (e.g., activity callback) exist in both (`chasm/lib/callback/`, `service/history/hsm/`).

---

Generated by `dimension 01.03` against `temporal`.
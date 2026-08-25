# Source Analysis: temporal

## 01.01 - Execution Model Taxonomy

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `sources/temporal` |
| Language / Stack | Go (server), protobuf, fx, ringpop/membership, gRPC |
| Analyzed | 2026-07-02 |

## Summary

Temporal's server is **not** one execution model. It is a **deliberate, layered composition of at least four distinct execution models**, each chosen to fit one slice of the durable-execution problem:

1. **Event-sourced workflow state machine** in `service/history/` (the "classic" engine) — every Workflow Execution has a `MutableStateImpl` (`service/history/workflow/mutable_state_impl.go:104`) whose state transitions are validated by a status state machine (`service/history/workflow/mutable_state_state_status.go:21-125`) and driven by a per-workflow-task state machine (`service/history/workflow/workflow_task_state_machine.go:62`).
2. **Hierarchical State Machine (HSM) framework** in `service/history/hsm/sm.go:18-66` — a generic, typed `Transition[S, SM, E]` used by callbacks, Nexus operations, versioning, etc. with an HSM tree (`service/history/hsm/tree.go`).
3. **CHASM (Coordinated Heterogeneous Application State Machines) engine** in `chasm/engine.go:16-59` and `chasm/statemachine.go:18-79` — the typed successor to HSM, with `PureTaskHandler` and `SideEffectTaskHandler` task types (`chasm/task.go:30-74`).
4. **Persistent, sharded task-queue processor** in `service/history/queues/` — category-based pipelines (`service/history/tasks/category.go:20-88`) with two distinct event loops: `immediateQueue.processEventLoop` (`service/history/queues/queue_immediate.go:132-161`) and `scheduledQueue.processEventLoop` (`service/history/queues/queue_scheduled.go:175-201`).
5. **Long-poll matcher** in `service/matching/matching_engine.go:629-664` and `service/matching/pri_task_reader.go` — the user-facing `AddWorkflowTask` / `AddActivityTask` / `PollWorkflowTaskQueue` API (`service/matching/matching_engine_interfaces.go:12-52`).
6. **Membership-driven shard ownership loop** in `service/history/shard/ownership.go` — a `eventLoop` selecting on `acquireTicker.C` and `membershipUpdateCh`.

The one-sentence answer to "what advances execution?" is:

> A Workflow Execution advances when an event (RPC, timer, or task response) triggers an atomic state transition over `MutableStateImpl`, appending history events and outboxing follow-up tasks in one database write; those tasks are then drained by per-shard, per-category queue processors whose `processEventLoop` goroutines feed a reader → scheduler → executor pipeline, while parallel CHASM transitions drive non-workflow "Application State Machines" through the same outbox.

The model is **explicit, well-documented, and layered** rather than emergent. Each layer has a named interface (`Queue`, `Scheduler`, `Engine`, `Controller`, `chasm.Engine`), `Start`/`Stop` lifecycle methods guarded by `common.DaemonStatus*` CAS, and a published architecture document (`docs/architecture/history-service.md:1`, `docs/architecture/chasm.md:1`).

## Rating

**9 / 10** — Mature, durable, observable, extensible, proven under failure and scale.

Rationale: every layer has an explicit interface, lifecycle, tests, DLQ plumbing, monitor/mitigator back-pressure (`service/history/queues/mitigator.go`), and a published architecture document. The system cleanly composes state-machine, queue, long-poll, and ownership models. The only structural risk is **pluralism**: a contributor must learn the workflow state machine, the HSM framework, the CHASM engine, the queue processor, the matcher, and the shard ownership model to be effective. The classic workflow engine and CHASM workflow archetype currently co-exist (`chasm/lib/workflow/workflow.go:28-51`).

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Runtime entrypoint | CLI builds server, dispatches `start` action, calls `temporal.NewServer` then `s.Start()` | `cmd/server/main.go`, `temporal/server.go:44-46` |
| Top-level server | `ServerImpl.Start` runs `initSystemNamespaces` then `startServices` in `initOrder` | `temporal/server_impl.go:88-146` |
| Services topology | Default services: Frontend, History, Matching, Worker | `temporal/server.go:25-41` |
| fx composition | `TopLevelModule` provides service graphs and invokes `ServerLifetimeHooks` | `temporal/fx.go` |
| History engine structure | `historyEngineImpl` holds `queueProcessors map[tasks.Category]queues.Queue`, `chasmEngine chasm.Engine` | `service/history/history_engine.go:104-152` |
| History engine lifecycle | `Start()` starts every `queueProcessor` and the `replicationProcessorMgr` | `service/history/history_engine.go:344-362` |
| Workflow state machine | `workflowTaskStateMachine` is the per-workflow-task state machine with `ApplyWorkflowTaskScheduledEvent` etc. | `service/history/workflow/workflow_task_state_machine.go:40-62` |
| Workflow status state machine | `setStateStatus` enumerates valid transitions between `WORKFLOW_EXECUTION_STATE_*` × `WORKFLOW_EXECUTION_STATUS_*` | `service/history/workflow/mutable_state_state_status.go:21-125` |
| HSM Transition (typed) | `Transition[S, SM, E]` checks `Sources`, calls `apply`, sets `Destination` | `service/history/hsm/sm.go:32-66` |
| HSM tree | `Node.MachineTransition` rolls forward the typed HSM, marks dirty, regenerates tasks | `service/history/hsm/tree.go` |
| HSM registry | `Registry.execute` / `executeTimer` are the HSM task executors | `service/history/hsm/registry.go:165,238` |
| CHASM Engine interface | `Engine.StartExecution/UpdateWithStartExecution/UpdateComponent/ReadComponent/PollComponent/DeleteExecution/NotifyExecution` | `chasm/engine.go:16-59` |
| CHASM Transition (typed) | `Transition[S, SM, E].Apply` checks `Sources`, runs `apply`, sets `Destination` with telemetry on error | `chasm/statemachine.go:24-79` |
| CHASM component lifecycle | `LifecycleState` enum: `Running`, `Paused`, `Completed`, `Failed` | `chasm/component.go:62-83` |
| CHASM task types | `SideEffectTaskHandler` and `PureTaskHandler` interfaces with `Validate`/`Execute`/`Discard` | `chasm/task.go:27-74` |
| CHASM task attributes | `TaskAttributes{ScheduledTime, Destination}`; `TaskScheduledTimeImmediate` sentinel | `chasm/task.go:17-25,77` |
| Scheduler ASMs (built on CHASM) | `workflow`, `activity`, `scheduler`, `callback`, `nexusoperation` libraries | `chasm/lib/workflow/`, `chasm/lib/activity/`, `chasm/lib/scheduler/`, `chasm/lib/callback/`, `chasm/lib/nexusoperation/` |
| Activity state machine | `Activity` implements `chasm.StateMachine[ActivityExecutionStatus]` with `TransitionScheduled` / `TransitionRescheduled` / `TransitionStarted` | `chasm/lib/activity/statemachine.go:21-150` |
| Activity task handlers | `activityDispatchTaskHandler`, `scheduleToStartTimeoutTaskHandler`, `scheduleToCloseTimeoutTaskHandler`, `startToCloseTimeoutTaskHandler` | `chasm/lib/activity/activity_tasks.go:21-200` |
| Scheduler component tree | `Scheduler` root with `Generator`, `Invoker`, `Backfillers` `chasm.Field`/`Map` children | `chasm/lib/scheduler/scheduler.go:38-61` |
| Scheduler task scheduling | `Invoker.addTasks` schedules `InvokerProcessBufferTask` (pure) and `InvokerExecuteTask` (side effect) | `chasm/lib/scheduler/invoker.go:294-320` |
| Callback state machine | `TransitionScheduled`, `TransitionRescheduled`, `TransitionAttemptFailed`, `TransitionFailed` | `chasm/lib/callback/statemachine.go:19-100` |
| Nexus operation state machine | `TransitionScheduled` with `InvocationTask`, `ScheduleToStartTimeoutTask`, `ScheduleToCloseTimeoutTask` | `chasm/lib/nexusoperation/operation_statemachine.go:18-47` |
| Workflow CHASM archetype (bridged) | `Workflow.MSPointer` delegates to legacy `MutableStateImpl`; `LifecycleState` hard-coded to `Running` | `chasm/lib/workflow/workflow.go:20-73` |
| Task category taxonomy | `CategoryTransfer` (Immediate), `CategoryTimer` (Scheduled), `CategoryReplication`, `CategoryVisibility`, `CategoryArchival`, `CategoryMemoryTimer`, `CategoryOutbound` | `service/history/tasks/category.go:20-88` |
| Category type enum | `CategoryTypeImmediate` vs `CategoryTypeScheduled` | `service/history/tasks/category.go:30-34` |
| Task category registry | `TaskCategoryRegistry` registers all categories by ID | `service/history/tasks/task_category_registry.go:31-36` |
| Queue interface | `Queue` interface with `Start`, `Stop`, `NotifyNewTasks` | `service/history/queues/queue.go:9-16` |
| Immediate queue event loop | `immediateQueue.processEventLoop` selects on `notifyCh`, `pollTimer.C`, `checkpointTimer.C`, `alertCh`, `shutdownCh` | `service/history/queues/queue_immediate.go:132-161` |
| Scheduled queue event loop | `scheduledQueue.processEventLoop` selects on `newTimerCh`, `lookAheadCh`, `timerGate.FireCh()`, `checkpointTimer.C`, `alertCh`, `shutdownCh` | `service/history/queues/queue_scheduled.go:175-201` |
| Queue base shared logic | `queueBase` composes reader, scheduler, rescheduler, executable factory, and host rate limiter | `service/history/queues/queue_base.go:104` |
| Queue factory registration | `QueueModule` provides `NewTransferQueueFactory`, `NewTimerQueueFactory`, `NewVisibilityQueueFactory`, `NewMemoryScheduledQueueFactory`, `NewArchivalQueueFactory`, `NewOutboundQueueFactory` | `service/history/queue_factory_base.go:77-104,126-140` |
| FIFO scheduler worker pool | `FIFOScheduler` is a buffered `tasksChan` drained by N goroutines that call `task.Execute()` then `Ack/Nack` | `common/tasks/fifo_scheduler.go:51-53,92-97,190-215` |
| Executable wrapper | `executableImpl.Execute` adds tracing, priority check, `HandleErr`, Nack/Ack, DLQ on terminal errors, busy-workflow reroute | `service/history/queues/executable.go` |
| Queue executors | Type-switches on task type within each category's `*ActiveTaskExecutor` / `*StandbyTaskExecutor` | `service/history/timer_queue_active_task_executor.go:90-138`, `service/history/transfer_queue_active_task_executor.go:104-217`, `service/history/visibility_queue_task_executor.go:73` |
| Mitigator + monitor | `queueBase.handleAlert` invokes mitigator and notifies readers | `service/history/queues/queue_base.go:425-437`, `service/history/queues/mitigator.go` |
| Chasm Engine integration | `chasm_engine.go` provides the chasm.Engine implementation for History service | `service/history/chasm_engine.go` |
| Chasm task types | `chasm_task.go` defines `ChasmTask` with `CategoryTimer` (scheduled) | `service/history/tasks/chasm_task.go:42,50,89` |
| Matching engine interface | `Engine` exposes `AddWorkflowTask`, `AddActivityTask`, `PollWorkflowTaskQueue`, `PollActivityTaskQueue`, `QueryWorkflow`, `DispatchNexusTask`, `PollNexusTaskQueue` etc. | `service/matching/matching_engine_interfaces.go:12-52` |
| Matching engine entrypoints | `matchingEngineImpl.AddActivityTask` → `pm.AddTask` to per-partition manager | `service/matching/matching_engine.go:629-664` |
| Matching engine lifecycle | `matchingEngineImpl.Start` subscribes to membership changes via `membershipChangedCh` | `service/matching/matching_engine.go:342-352` |
| Backlog reader pump | `priTaskReader.getTasksPump` driven by `notifyC` and look-ahead goroutines | `service/matching/pri_task_reader.go:94-198` |
| Frontend gRPC handlers | `WorkflowHandler.StartWorkflowExecution` calls `wh.historyClient.StartWorkflowExecution` | `service/frontend/workflow_handler.go:538-574` |
| WorkflowHandler to History | `SignalWorkflowExecution` and `UpdateWorkflowExecution` also delegate to `historyClient` | `service/frontend/workflow_handler.go:2258,5298` |
| Architecture doc (history) | Explicit sequence: RPC → state transition → outbox tasks → queue processor → Matching | `docs/architecture/history-service.md:13-63,176-216,319-321` |
| Architecture doc (CHASM) | Explicit definition of Transitions, Pure Tasks, Side-Effect Tasks, `VersionedTransition` | `docs/architecture/chasm.md:214-297` |
| Architecture doc (matching) | Task queues, partitions, forwarder pattern | `docs/architecture/matching-service.md:1-19` |
| Architecture doc (lifecycle) | Sequence diagrams for start, signal, response | `docs/architecture/workflow-lifecycle.md:1-80` |
| Architecture doc (in-memory queue) | Speculative `WorkflowTaskTimeoutTask` lives in memory | `docs/architecture/in-memory-queue.md:1-23` |

## Answers to Dimension Questions

1. **What is the primary execution model?**
   A **layered composition**: an event-sourced workflow state machine + HSM + CHASM + persistent sharded task queues + a long-poll matcher. The most fundamental loop is:
   - An event arrives at a History Shard gRPC handler (`service/history/handler.go`).
   - `MutableState` is loaded, the `mutableState_state_status` state machine validates the requested state transition (`service/history/workflow/mutable_state_state_status.go:21`), the per-workflow-task state machine applies it (`service/history/workflow/workflow_task_state_machine.go:62`).
   - History events are appended and follow-up tasks are generated in the same persistence transaction (transactional outbox, `docs/architecture/history-service.md:319-321`).
   - Per-category queue processors read those tasks via `processEventLoop` → readers → scheduler → executor (`service/history/queues/queue_immediate.go:132`, `service/history/queues/queue_scheduled.go:175`).
   - For matching/worker delivery, Transfer Tasks RPC into the Matching Service (`service/matching/matching_engine.go:622-664`), which buffers them in partitions until a worker long-polls.

2. **Is it explicit or emergent?**
   **Explicit.** Each layer has a named interface and `Start`/`Stop` lifecycle:
   - `Queue` interface (`service/history/queues/queue.go:9-16`).
   - `Scheduler` interface (`common/tasks/scheduler.go:25`).
   - `Executable` / `Executor` (`service/history/queues/executable.go`).
   - `chasm.Engine` interface (`chasm/engine.go:16-59`).
   - `Controller` / `Ownership` (`service/history/shard/`).
   - `matching.Engine` interface (`service/matching/matching_engine_interfaces.go:12-52`).
   The architecture documents (`docs/architecture/history-service.md:1`, `docs/architecture/chasm.md:1`, `docs/architecture/matching-service.md:1`) spell out the design intent.

3. **Does the model match the product shape?**
   **Yes.** A durable workflow engine naturally fits:
   - durable state machines (Workflow),
   - timer-driven continuations (Timer Task queue),
   - long-poll-style task delivery to remote workers (Matching Service),
   - sharded ownership of large fan-out (Shard Controller + `RangeID`),
   - pluggable "Application State Machines" (CHASM).
   Each of those is implemented as a distinct, named model rather than twisted into one. The transactional outbox is the explicit pattern connecting them (`docs/architecture/history-service.md:319-321`).

4. **Is the model easy to explain to a new contributor?**
   **Partially.** The headline story (Event → State Machine → Mutable State → Outbox Tasks → Queue Processor → Matching → Worker) is clean. But a new contributor must learn:
   1. The workflow task state machine in `service/history/workflow/workflow_task_state_machine.go` plus the status state machine in `mutable_state_state_status.go`.
   2. The queue processor abstraction in `service/history/queues/`.
   3. The HSM framework in `service/history/hsm/`.
   4. The CHASM engine in `chasm/` and at least one ASM in `chasm/lib/`.
   5. The shard controller + ownership model in `service/history/shard/`.
   6. The Matching Service backlog/matcher/forwarder split in `service/matching/`.
   These are documented but represent real surface area.

5. **Does the system mix models cleanly or accidentally?**
   **Mostly cleanly, with one actively evolving seam.** Cleanly: queues, scheduler, and executor are well-separated; HSM is an isolated framework; CHASM has a published doc. The seam: the **Workflow itself** is implemented in two engines. The classic engine (`MutableStateImpl` + `workflowTaskStateMachine`) is being complemented by CHASM's `Workflow` archetype (`chasm/lib/workflow/workflow.go:20`), whose `LifecycleState` is hard-coded to `Running` and which delegates work through `MSPointer` (`chasm/lib/workflow/workflow.go:28-51,53-61`). The TODO at `chasm/engine.go:284-288` and the explicit gap noted in `service/history/chasm_engine.go` show that the boundary is intentionally being migrated, not abandoned.

## Architectural Decisions

- **Transactional outbox over async RPC**: `MutableState.CloseTransaction*` writes History Events + Mutable State + Tasks in one persistence call (`docs/architecture/history-service.md:319-321`).
- **Per-shard ownership with `RangeID` fencing**: a `ShardController` and `ownership.eventLoop` move shards between hosts (`service/history/shard/ownership.go`).
- **Two queue categories, one shared base**: `queueBase` is composed into `immediateQueue` and `scheduledQueue`, with `category.Type()` choosing the executor (`service/history/queues/queue_base.go`, `service/history/tasks/category.go:30-34`).
- **Queue readers are leases, not locks**: readers fetch slices from persistence under a single-mutex `ReaderImpl`, then submit to a scheduler that runs in a separate goroutine (`service/history/queues/reader.go`).
- **FIFO over lock-free stack**: the scheduler is intentionally a buffered channel + worker pool (`common/tasks/fifo_scheduler.go:51-53`), not lock-free MPMC.
- **Two-tier scheduler stack**: `InterleavedWeightedRoundRobin` over `ExecutionAwareScheduler` over `FIFO`, allowing namespace priority, busy-workflow reroute to per-execution queues, and rate limiting without changing the worker pool (`service/history/queues/scheduler.go`, `common/tasks/execution_aware_scheduler.go`).
- **Speculative workflow tasks**: a separate `MemoryScheduledQueue` provides sub-second latency for speculative tasks (`service/history/queues/memory_scheduled_queue.go`, `docs/architecture/in-memory-queue.md:1`).
- **HSM as a generic transition framework**: HSM tasks (callbacks, Nexus operations, versioning) all reuse the same `Transition` mechanism (`service/history/hsm/sm.go:32-66`) and the same outbox semantics as workflow tasks.
- **CHASM engine fronted by `chasm.Engine` interface** to keep a stable, typed API for the future where workflows themselves become just one Application State Machine (`chasm/engine.go:16-59`, `docs/architecture/chasm.md:1-211`).
- **Pure vs Side-Effect task split** in CHASM: pure tasks run within the state lock and may not do I/O; side-effect tasks run after commit and may do I/O but must call back via `Engine.UpdateComponent` (`chasm/task.go:27-74`).
- **Long-poll matcher rather than push**: workers poll, the matching engine responds from in-memory match data or persists and re-loads (`service/matching/matching_engine.go:342-352`, `service/matching/pri_task_reader.go`).
- **Membership-change-driven hot reload**: matching engine watches membership via `membershipChangedCh` and unloads partitions it no longer owns (`service/matching/matching_engine.go:342-433`).
- **Per-ASM task registry**: each CHASM ASM registers tasks with a `Library` (`chasm/library.go`) and provides `PureTaskHandler` and `SideEffectTaskHandler` implementations (e.g. `chasm/lib/scheduler/invoker_tasks.go:48-64`).

## Notable Patterns

- **`processEventLoop` as the canonical server goroutine shape**: select on `shutdownCh`, one or more notification channels, one timer channel, and an alert channel. Reused literally in `service/history/queues/queue_scheduled.go:175-200`, `service/history/queues/queue_immediate.go:132-161`, and the matching service's `getTasksPump` (`service/matching/pri_task_reader.go`).
- **Daemon status CAS guards**: every `Start()` / `Stop()` uses `atomic.CompareAndSwapInt32` against `common.DaemonStatus*` constants to make double-start safe (`service/history/queues/queue_immediate.go:91-93`, `service/history/queues/queue_scheduled.go:125-128`, `service/history/history_engine.go:344-352`, `common/tasks/fifo_scheduler.go:55-69`).
- **Transactional outbox** for every state change: timers/transfer/replication tasks are written in the same persistence op as the events that generated them (`docs/architecture/history-service.md:319-321`).
- **Generator + State Machine split**: `MutableStateImpl` mutates in-memory state via `WorkflowTaskStateMachine`, while `TaskGenerator` (`service/history/workflow/task_generator.go`) converts that mutation into queued tasks. Pure separation of "what changed" from "what to do next."
- **Bridge interface `chasm.MSPointer`**: lets the CHASM workflow component delegate state ownership to the legacy `MutableStateImpl` while still being a CHASM node (`chasm/lib/workflow/workflow.go:28-51`).
- **Validator/Executor/Discard triad** for tasks: every task handler has a `Validate(ctx, component, attrs, task) (bool, error)` to gate execution and a `Discard` for standby-cluster spillover (`chasm/task.go:52-72`, `chasm/lib/activity/activity_tasks.go:32-80`).
- **Operation log compaction** in HSM: keeps an `OperationLog` that is compacted using an op-tree to handle subtree deletions (`service/history/hsm/tree.go`).
- **Alert / monitor / mitigator triad**: queues expose a `Monitor`, a Mitigator reacts to `Alert`s emitted by the monitor (`service/history/queues/queue_base.go:425-437`, `service/history/queues/mitigator.go`).
- **VersionedTransition as the logical clock of CHASM**: `{FailoverVersion, TransitionCount}` provides total ordering of state changes even across data centers (`docs/architecture/chasm.md:277-297`).
- **TaskScheduledTimeImmediate sentinel**: zero-value `time.Time` signals "execute immediately" (`chasm/task.go:77-83`).
- **Pure task + side-effect task split for transactional outbox** in CHASM tasks (`chasm/task.go:30-48`, `docs/architecture/chasm.md:242-273`).
- **CHASM as a meta-state-machine for ASMs**: the chasm tree is a Hierarchical State Machine that any library can register against (`chasm/registrable_component.go`, `chasm/registrable_task.go`).

## Tradeoffs

- **Layered composition**: very strong separation of concerns; a queue bug rarely affects HSM or CHASM. But it adds conceptual overhead: every "what advances execution?" answer requires naming which layer.
- **Single mutable state, multiple engines**: the workflow execution has a `MutableStateImpl` *and* a CHASM tree pointing at it via `MSPointer` (`chasm/lib/workflow/workflow.go:24-28`). This keeps existing semantics intact during the CHASM migration but duplicates "what is the source of truth" in two places.
- **Polling matchers**: chosen over push to avoid fan-out push storms at the cost of needing `notifyC` and look-ahead goroutines.
- **CAS-guarded `Start/Stop`**: explicit but verbose; an fx-based daemon abstraction would be smaller but harder to follow.
- **Per-shard scheduling**: keeps back-pressure local to a shard but means rebalancing shards during rolling upgrades requires queue-aware handover logic (`service/history/shard/ownership.go`).
- **Memory Scheduled Queue** is specialised to one task type (`WorkflowTaskTimeoutTask`); the comment at `docs/architecture/in-memory-queue.md:21-23` explicitly admits the design is not generalised yet.
- **`executableImpl.Execute`** carries an explicit branch for `BusyWorkflowHandler` (`service/history/queues/executable.go`). This is a runtime detour around contention rather than a structural feature.
- **Two Transition structs with the same shape**: `chasm/statemachine.go:24-79` and `service/history/hsm/sm.go:32-66` are near-duplicates, indicating an in-flight consolidation.
- **CHASM workflow archetype is a stub**: `chasm/lib/workflow/workflow.go:53-61` returns `LifecycleStateRunning` unconditionally because the legacy engine still owns state; the new engine cannot yet terminate the workflow.

## Failure Modes / Edge Cases

- **Shard lost mid-task**: `HandleBusyWorkflow` reroutes the task to a per-execution queue (`service/history/queues/executable.go`).
- **Persistence error during task load**: `ReaderImpl.loadAndSubmitTasks` does exponential backoff via `r.retrier.NextBackOff(err)` and pauses on `ResourceExhausted`.
- **Reader stuck**: `actionReaderStuck` and `alertCh` → `handleAlert` triggers a reader-side mitigation (`service/history/queues/queue_base.go:425-437`, `service/history/queues/action_reader_stuck.go`).
- **Pending task overload**: `MaxPendingTasksCount` triggers `pauseLocked` in the reader.
- **Terminal errors / DLQ**: `executableImpl.HandleErr` writes to a `DLQWriter` when a task is deemed terminal via `MaybeTerminalTaskError` (`service/history/queues/executable.go`).
- **Membership churn**: `ownership.eventLoop` debounces via `scheduleAcquire` writing to a `acquireCh` of capacity 1 (`service/history/shard/ownership.go`).
- **Crash before checkpoint**: `forceNewSliceDuration = 5 * time.Minute` ensures at least one new slice gets created, bounding re-scan cost on shard reload (`service/history/queues/queue_base.go`).
- **Shutdown timeout**: every `Stop()` waits up to one minute via `common.AwaitWaitGroup` (`service/history/queues/queue_immediate.go:117-118`, `service/history/queues/queue_scheduled.go:152-153`); the history service enforces a `serviceStopTimeout = 5 * time.Minute` (`temporal/server.go:13`).
- **Stale reference / replication lag**: explicitly handled in CHASM (`service/history/chasm_engine.go`) by classifying `ErrStaleState` as retryable rather than fatal.
- **Speculative workflow task superseded**: `CheckSpeculativeWorkflowTaskTimeoutTask` discards the old timer (`docs/architecture/in-memory-queue.md:14-19`).
- **CHASM speculative transition**: not persisted (`chasm/engine.go:139-145`); recovery depends on the next non-speculative transition persisting both.
- **Invalid state transition**: `setStateStatus` returns `invalidStateTransitionErr` for undeclared source/target combinations (`service/history/workflow/mutable_state_state_status.go:127-138`).
- **Invalid HSM/CHASM transition**: `Transition.Apply` returns `ErrInvalidTransition` from `Possible(sm) == false` (`service/history/hsm/sm.go:60-61`, `chasm/statemachine.go:70-72`).
- **Standby task past discard delay**: `SideEffectTaskHandler.Discard` is invoked instead of silently dropping (e.g. activity dispatch spills to matching, `chasm/lib/activity/activity_tasks.go:52-61`).

## Future Considerations

- **CHASM-first workflow**: the explicit migration intent in `docs/architecture/chasm.md:11` plus the `MSPointer` bridge (`chasm/lib/workflow/workflow.go:24-28`) suggest the long-term plan is to move the workflow itself into the CHASM engine.
- **In-memory queue generalization**: the in-memory queue doc explicitly notes it is currently for speculative Workflow Task timeouts only (`docs/architecture/in-memory-queue.md:21-23`). A more general "in-process timer scheduler" could reuse this.
- **State-based replication for CHASM**: an open issue is replicating a chain of runs created via `BusinessIDConflictPolicyTerminateExisting` (`service/history/chasm_engine.go`).
- **Scheduler shadow mode**: `RateLimitedSchedulerOptions.EnableShadowMode` lets new policies be exercised without affecting throughput, useful for safe rollouts.
- **Speculative transitions in CHASM**: still missing the ability to run tasks generated inside a speculative transition (`chasm/engine.go:139-145,284-288`).
- **CHASM Task type consolidation**: the duplicated `Transition` shape between `chasm/statemachine.go` and `service/history/hsm/sm.go` will need to converge.

## Questions / Gaps

- No clear evidence found for a single canonical "execution model" doc; the architecture documents (`docs/architecture/history-service.md:1`, `docs/architecture/chasm.md:1`, `docs/architecture/matching-service.md:1`, `docs/architecture/in-memory-queue.md:1`) describe slices but the cross-cutting narrative ("workflow advances via state machine → outbox → queue processor → matcher → worker") is left implicit.
- The matching service's matcher/scheduler layering is described as "Additional Documentation of Matching Service internals is not yet available" (`docs/architecture/matching-service.md:18`). The `pri_matcher.go`/`pri_backlog_manager.go`/`pri_task_reader.go` files are real, but a newcomer has no single entry point.
- The relationship between `MutableStateImpl`'s state machine and the CHASM tree is exercised through `MSPointer` but there is no single test or doc showing both engines applying the *same* transition in lockstep — confidence here relies on inspection of `chasm/lib/workflow/workflow.go` plus `service/history/chasm_engine.go`.
- Whether the queue model is the intended replacement for, or augmentation of, the workflow task state machine for non-Workflow ASMs (callbacks, Nexus operations, scheduler) is documented in `docs/architecture/chasm.md:25-51` but the runtime ownership between "executor in queue" and "executor in HSM registry" is not explicitly contrasted.
- Why two near-identical `Transition` types exist (`chasm/statemachine.go:24` vs `service/history/hsm/sm.go:32`) is not explained in code or docs — likely a pre-consolidation artefact.

---

Generated by `01.01-execution-model-taxonomy` against `temporal`.

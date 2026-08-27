# Source Analysis: temporal

## Plan Lifecycle and Revision

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (distributed workflow orchestration server; gRPC APIs; pluggable DB/Elasticsearch persistence) |
| Analyzed | 2026-08-27 |

## Summary

Temporal has no LLM-style "plan object" — the plan *is* the workflow code (a deterministic program that schedules activities, timers, child workflows, and signals). Plan lifecycle is therefore mapped onto **workflow execution lifecycle**: creation is a `StartWorkflowExecution` request, revision is any in-flight mutation or code-version change, and completion is a terminal event that flips a guarded state machine. The system makes that lifecycle durable and auditable rather than autonomous: every transition is validated against a total `setStateStatus` function, appended as an immutable history event via `HistoryBuilder`, replicated through `VersionHistories` branch tokens, and reconstructable by replay (`MutableStateRebuilder`). 

Changes are possible but narrowly channeled: intra-execution replanning via `UpdateWorkflowExecution`, `SignalWorkflowExecution`, `RequestCancel`, and `ContinueAsNew`; inter-execution revision via code patching + `TemporalChangeVersion` search attributes, worker deployment versioning, and explicit `ResetWorkflowExecution` that forks a new run from an earlier workflow-task boundary. Abandonment is explicit (`Cancel`, `Terminate`, `Timeout`, `Fail` without retry). Drift is detected as **non-determinism**: any divergence between replayed history and newly emitted commands fails the workflow task with `WORKFLOW_TASK_FAILED_CAUSE_NON_DETERMINISTIC_ERROR` rather than silently advancing. History is never mutated — old plan history is preserved as an append-only event log with version-history branching, archival, and deterministic rebuild. The gap versus an agentic planner is intentional: Temporal explains *why a revision was applied* (reason, patch marker, reset point, update payload) but does not generate or evaluate alternative plans itself; plan reasoning lives in worker code.

## Rating

**8 / 10**

Rationale: Plan lifecycle is explicit, durable, and mechanically enforced with strong tests and operational safeguards — event-sourced history (`service/history/historybuilder/history_builder.go:163-187,424-446`), irreversible terminal state matrix (`service/history/workflow/mutable_state_state_status.go:16-125`), validated command sequencing (`service/history/api/respondworkflowtaskcompleted/workflow_task_completed_handler.go:173-179` + `service/history/api/command_attr_validator.go`), and full reset/update audit trails. It loses two points because (a) there is no first-class "plan" artifact distinct from workflow code/history — no structured plan diff, planner/executor contract, or autonomous replanning engine to score — and (b) final semantic success is trusted, not independently verified, and plan-change observability depends on opt-in `TemporalChangeVersion` indexing and async visibility tasks.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Plan creation – start execution | `Starter` struct + `StartWorkflowExecutionRequest` handling creates new execution; validates workflow ID reuse, builds first `WorkflowExecutionStarted` event | `studies/agent-harness-study/sources/temporal/service/history/api/startworkflow/api.go:32-60,50-200` |
| Plan creation – deterministic start event | `AddWorkflowExecutionStartedEventWithOptions` enforces `FirstEventID` guard, creates event via `hBuilder.AddWorkflowExecutionStartedEvent`, then applies it to mutable state | `studies/agent-harness-study/sources/temporal/service/history/workflow/mutable_state_impl.go:3011-3049` |
| Plan creation – child/scheduled/continued paths | `handleCommandStartChildWorkflow` validates then schedules child; `handleCommandContinueAsNewWorkflow` creates new run with `uuid.NewString()` + `AddContinueAsNewEvent`; schedule/cron handled via `GetCronBackoffDuration`/`GetRetryBackoffDuration` | `studies/agent-harness-study/sources/temporal/service/history/api/respondworkflowtaskcompleted/workflow_task_completed_handler.go:1151-1280,1042-1149,831-846` |
| Plan update – in-flight mutation (Update) | `UpdateWorkflowExecution` API + `AddWorkflowExecutionUpdateAdmittedEvent` appends `WorkflowExecutionUpdateAdmitted` event; updates carry request payload, acceptance/rejection lifecycle | `studies/agent-harness-study/sources/temporal/service/history/workflow/mutable_state_impl.go:5676-5686` ; `studies/agent-harness-study/sources/temporal/service/history/api/updateworkflow/api.go:41-76` |
| Plan update – signal / cancel | `SignalWorkflowExecution`/`RequestCancelWorkflowExecution` add `WorkflowExecutionSignaled`/`CancelRequested` events; `UpdateWorkflowWithNew` pattern mutates lease then generates workflow task | `studies/agent-harness-study/sources/temporal/service/history/api/requestcancelworkflow/api.go:36-86` |
| Plan update validation (revision guard) | `ValidateCommandSequence` + per-command `Validate*Attributes` (e.g. `ValidateCompleteWorkflowExecutionAttributes`, `ValidateContinueAsNewWorkflowExecutionAttributes`) run before any command applied; invalid args fail workflow task | `studies/agent-harness-study/sources/temporal/service/history/api/respondworkflowtaskcompleted/workflow_task_completed_handler.go:173-179,804-810,1068-1078` ; `studies/agent-harness-study/sources/temporal/service/history/api/command_attr_validator.go:219-228,391-452` |
| Plan revision – code versioning / patch | `TemporalChangeVersion` predefined search attribute (`sadefs.TemporalChangeVersion`) tags executions by patch/version; worker deployment versioning (`VersioningOverride`, `WorkerVersionStamp`) routed via `recordworkflowtaskstarted/api.go`, `recordactivitytaskstarted/api.go` | `studies/agent-harness-study/sources/temporal/common/searchattribute/sadefs/constants.go:26,155` ; `studies/agent-harness-study/sources/temporal/service/history/api/recordworkflowtaskstarted/api.go:129-168` |
| Plan revision – reset (time-travel fork) | `ResetWorkflowExecutionRequest` validates `WorkflowTaskFinishEventId` bounds, dedups by `RequestId`, rebuilds to `baseRebuildLastEventID-1` via `versionhistory.GetVersionHistoryEventVersion`, then calls `WorkflowResetter.ResetWorkflow` to fork new `resetRunID` branch; `PostResetOperations` can pin versioning override | `studies/agent-harness-study/sources/temporal/service/history/api/resetworkflow/api.go:28-216,136-201` |
| Plan revision persisted – event sourcing | All mutations go through `HistoryBuilder` (`AddCompletedWorkflowEvent`, `AddWorkflowExecutionStartedEvent`, `AddContinuedAsNewEvent`, etc.) whose `add` appends to immutable `historyEvents` buffer; state reconstructed by replay | `studies/agent-harness-study/sources/temporal/service/history/historybuilder/history_builder.go:163-187,424-432,458-468,603-620` |
| Revision history preserved – version histories | Per-execution `VersionHistories` / `VersionHistoryItem` (version + eventID) maintained; `CurrentVersionHistory` branch token carried through reset/reapply, enabling causal lineage across forks | `studies/agent-harness-study/sources/temporal/service/history/api/get_workflow_util.go:63-75,322-331` ; `studies/agent-harness-study/sources/temporal/service/history/workflow/mutable_state_rebuilder.go:101,137-142` |
| Revision history rebuild – replayer | `MutableStateRebuilder.ApplyEvents` re-hydrates mutable state from `[][]*HistoryEvent` + `newRunHistory` via `NewImmutable` builder and `chasm` event definitions, proving log is authoritative over snapshot | `studies/agent-harness-study/sources/temporal/service/history/workflow/mutable_state_rebuilder.go:72-142,769-780` |
| Plan completion – state transition matrix | `setStateStatus` enumerates legal transitions; `COMPLETED` with any status rejects moves except identical re-set (`status != e.GetStatus()` → error); zombie/completed isolation explicit | `studies/agent-harness-study/sources/temporal/service/history/workflow/mutable_state_state_status.go:16-125` |
| Plan completion – running guard & first-wins | `IsWorkflowExecutionRunning()` (`CREATED/RUNNING=true`, else false) gates completion; `handleCommandCompleteWorkflow/FailWorkflow/ContinueAsNew` check `IsWorkflowExecutionRunning()` and emit `MultipleCompletionCommandsCounter` + warn then drop extra completions | `studies/agent-harness-study/sources/temporal/service/history/workflow/mutable_state_impl.go:2493-2508` ; `studies/agent-harness-study/sources/temporal/service/history/api/respondworkflowtaskcompleted/workflow_task_completed_handler.go:820-830,875-885,1115-1124` |
| Plan completion – terminal events | `AddCompletedWorkflowEvent`→`ApplyWorkflowExecutionCompletedEvent` (`STATE_COMPLETED/STATUS_COMPLETED`, sets `CloseTime`, clears sticky queue); symmetric `AddFailWorkflowEvent`/`AddTimeoutWorkflowEvent`/`AddWorkflowExecutionCanceledEvent`/`AddWorkflowExecutionTerminatedEvent` each call `UpdateWorkflowStateStatus` + `GenerateWorkflowCloseTasks` | `studies/agent-harness-study/sources/temporal/service/history/workflow/mutable_state_impl.go:4941-4982,4984-5033,5035-5082,5120-5160,5649-5674` |
| Plan abandonment | Cancel (`ApplyWorkflowExecutionCanceledEvent`), Terminate (`AddWorkflowExecutionTerminatedEvent`), Timeout (`AddTimeoutWorkflowEvent`), and Fail without retry all transition to `COMPLETED` terminal; `DeleteAfterTerminate` flag propagates to close tasks | `studies/agent-harness-study/sources/temporal/service/history/workflow/mutable_state_impl.go:5120-5160,5649-5674,5035-5060` |
| Replanning triggers – retry/cron/continue-as-new | `handleCommandFailWorkflow` computes `GetRetryBackoffDuration`/`GetCronBackoffDuration` and, if backoff != NoBackoff, generates `newExecutionRunID` and delegates to `handleRetry`/`handleCron`; `ContinueAsNew` always forks new mutable state via `NewMutableStateInChain` | `studies/agent-harness-study/sources/temporal/service/history/api/respondworkflowtaskcompleted/workflow_task_completed_handler.go:886-918,831-846,6300-6350` |
| Plan drift detection – determinism enforcement | Non-deterministic replay diverges → workflow task failed with `WORKFLOW_TASK_FAILED_CAUSE_NON_DETERMINISTIC_ERROR` (checked in `workflow_handler.go:1313`); `failWorkflowTask` sets `stopProcessing=true` and emits `WorkflowTaskFailed` event instead of advancing | `studies/agent-harness-study/sources/temporal/service/frontend/workflow_handler.go:1313` ; `studies/agent-harness-study/sources/temporal/service/history/api/respondworkflowtaskcompleted/workflow_task_completed_handler.go:790-794,1571-1585` |
| Plan drift – mutability guard | Every state mutation starts with `checkMutability(opTag)` which rejects writes to completed/zombie/corrupted executions | `studies/agent-harness-study/sources/temporal/service/history/workflow/mutable_state_impl.go:4947,3019,5125,5657,6308` |
| Tests – lifecycle | `TestAddCompletedWorkflowEvent_CompletionEventBatchID`, `TestAddCompletedWorkflowEvent_ArchivalConvertsVirtualToWall`, history builder categorization tests for terminal events | `studies/agent-harness-study/sources/temporal/service/history/workflow/mutable_state_impl_test.go:7625-7685,8958-8965` ; `studies/agent-harness-study/sources/temporal/service/history/historybuilder/history_builder_categorization_test.go:563,795,1340` |

## Answers to Dimension Questions

**1. Can plans change?**
Yes. Temporal's "plan" is the workflow's deterministic code and its scheduled steps. It can change intra-execution (signal, update, request-cancel, patch via `TemporalChangeVersion`/`GetVersion` markers, child workflow spawns) and inter-execution (new code deployment under deployment-versioning, worker build-ID routing `studies/agent-harness-study/sources/temporal/service/history/api/recordworkflowtaskstarted/api.go:129-168`, or explicit forks via `ContinueAsNew` `studies/agent-harness-study/sources/temporal/service/history/workflow/mutable_state_impl.go:6300-6350` and `ResetWorkflowExecution` `studies/agent-harness-study/sources/temporal/service/history/api/resetworkflow/api.go:28-216`). All changes funnel through validated workflow-task commands (`ValidateCommandSequence` at `workflow_task_completed_handler.go:173-179`).

**2. Are changes justified?**
Every change is justified in the durable log. Command validators enforce structural justification (`ValidateCompleteWorkflowExecutionAttributes` `service/history/api/command_attr_validator.go:219-228`, `ValidateContinueAsNewWorkflowExecutionAttributes` `391-452`); `HistoryBuilder` records the resulting event with user metadata/links (`history_builder.go:179-184`); resets require a `Reason` string and explicit `WorkflowTaskFinishEventId` plus `ResetReapplyExcludeTypes` (`resetworkflow/api.go:43-44,237-259`); updates carry request payload/origin (`mutable_state_impl.go:5676-5682`). Completion/cancel/terminate events also record identity, reason, details, and close time. There is no LLM planner justification — justification is the event payload and its validation chain.

**3. Is old plan history preserved?**
Yes, completely. History is append-only: `HistoryBuilder.add` never mutates prior events (`historybuilder/event_store.go:260-286`), version histories branch rather than overwrite (`versionhistory.GetCurrentVersionHistory` `get_workflow_util.go:71`), and `MutableStateRebuilder.ApplyEvents` can rebuild any execution from `[][]*HistoryEvent` (`mutable_state_rebuilder.go:72-142`). Resets fork a new run (`resetRunID = uuid.NewString()` `resetworkflow/api.go:136`) while retaining the base run's history; `ContinueAsNew` chains via `PrevRunID`/`FirstRunId` (`mutable_state_impl.go:3011-3049`). No compaction deletes history within the retention window; archival preserves it beyond.

**4. Can the agent abandon a plan?**
Yes, via explicit terminal transitions that the state machine makes irreversible (`mutable_state_state_status.go:74-85`). Worker-initiated `CancelWorkflowExecution` (`AddWorkflowExecutionCanceledEvent` `5120-5160`), operator `TerminateWorkflowExecution` (`AddWorkflowExecutionTerminatedEvent` `5649-5674`), system `Timeout` (`AddTimeoutWorkflowEvent` `5035-5060`), or `Fail` without retry (`AddFailWorkflowEvent` `4984-5033`) all move to `WORKFLOW_EXECUTION_STATE_COMPLETED` with distinct `WORKFLOW_EXECUTION_STATUS_*`. `checkMutability` then forbids further mutations. There is no silent abandonment — even abandonment is an event.

**5. Can plan drift be detected?**
Yes, as non-determinism. Temporal replays history deterministically; any mismatch between history and newly generated commands fails the workflow task with `WORKFLOW_TASK_FAILED_CAUSE_NON_DETERMINISTIC_ERROR` (`service/frontend/workflow_handler.go:1313`, `workflow_task_completed_handler.go:1571-1585`) instead of committing drift. `checkMutability` (`mutable_state_impl.go:4947` et al.) also rejects writes to completed/zombie executions, preventing state drift from stale workers. Semantic drift (wrong but deterministic output) is **not** detected — the server trusts payload content, only ordering/identity/determinism are enforced. `TemporalChangeVersion` (`sadefs/constants.go:26`) plus `VersionHistories` provide observability of intentional drift (patches).

## Architectural Decisions

1. **Code-is-plan, history-is-truth.** No separate plan artifact; the workflow function and its event log are the plan (`mutable_state_impl.go:3011-3049` + `history_builder.go:163-187`). Tradeoff: unmatched durability/replay vs. no declarative plan inspection/editing outside code.
2. **Total state-transition function for completion.** `setStateStatus` (`mutable_state_state_status.go:16-125`) centralizes terminality, making `COMPLETED` irreversible and zombie states explicit, rather than scattering checks.
3. **Event-sourced branching, not mutation.** Resets and retries fork new runs with new `RunId`/`VersionHistories` branch tokens (`resetworkflow/api.go:136-148`, `mutable_state_impl.go:6300-6320`) instead of editing the existing run — old plan history is never overwritten.
4. **Fail-workflow-task, not fail-workflow, on bad revisions.** Invalid commands or non-deterministic changes produce `WorkflowTaskFailed` events (`workflow_task_completed_handler.go:790-794,1571-1585`) and block progress, while `terminateWorkflow` (`1587-1601`) is reserved for unrecoverable payload/size violations — preserving the ability to fix and replay.
5. **Opt-in version observability.** Patch/version changes are surfaced only if authors emit `GetVersion` markers indexed into `TemporalChangeVersion` (`sadefs/constants.go:26`) and deployment version stamps (`recordworkflowtaskstarted/api.go:129-168`), decoupling versioning from the critical write path.

## Notable Patterns

- **First-wins completion arbitration:** multiple completions in one task silently drop after the first with `MultipleCompletionCommandsCounter` + warn (`workflow_task_completed_handler.go:820-830`).
- **Continue-as-new as bounded replanning:** long-running plans periodically atomically close and re-instantiate with new run ID, preserving parent/root lineage (`mutable_state_impl.go:6312-6347`).
- **Reset-as-fork:** `ResetWorkflowExecution` rebuilds state to `WorkflowTaskFinishEventId-1` via `GetVersionHistoryEventVersion` and re-applies exclude-filtered signals/updates (`resetworkflow/api.go:137-144,237-259`) — a git-like `git reset --hard` for workflows.
- **Two-phase command validation:** sequence check (`ValidateCommandSequence`) then per-attribute check (`Validate*Attributes`) before any `HistoryBuilder.add`, ensuring revision attempts are structurally sound.
- **Retry/cron as automatic replanning:** failure/timeout with retry or cron policy auto-generates a new run ID and chains execution (`workflow_task_completed_handler.go:886-918`).

## Tradeoffs

- **Durability vs. flexibility:** Immutable event log + `VersionHistories` makes every revision auditable and replayable, but correcting a bad terminal decision requires a new run/reset rather than in-place edit.
- **Determinism safety vs. author burden:** Strict non-determinism detection catches drift early (`WORKFLOW_TASK_FAILED_CAUSE_NON_DETERMINISTIC_ERROR`), but forces authors to use `GetVersion`/`Patch` APIs and avoid non-deterministic code.
- **Explicit completion vs. semantic verification:** Server guarantees *structural* validity (running check, size limits, state matrix) but trusts result payloads (`ValidateCompleteWorkflowExecutionAttributes` only checks existence `command_attr_validator.go:219-228`) — garbage-in success is durably recorded as success.
- **Branching history vs. query complexity:** Preserving every revision as a branched version history improves auditability but requires callers to traverse `VersionHistories`/`BranchToken` rather than reading a single mutable plan field.

## Failure Modes / Edge Cases

- **Duplicate completion race:** Two concurrent completions — first wins, second dropped with metrics (`workflow_task_completed_handler.go:820-830`), caller gets no error but log/metric records the conflict.
- **Reset to invalid boundary:** `WorkflowTaskFinishEventId <= FirstEventID` or `>= NextEventID` rejected with `InvalidArgument` (`resetworkflow/api.go:74-77`); missing base run tolerated only if explicit `baseRunID` provided (`shouldTolerateMissingCurrentExecution` `218-231`).
- **Non-deterministic patch:** Workflow code changed without a `GetVersion` guard → replay mismatch → workflow task failed with `NON_DETERMINISTIC_ERROR`, workflow stalls until code reverted or patched correctly (`workflow_handler.go:1313`).
- **Buffered-events guard blocks completion:** `hasBufferedEventsOrMessages` check fails completion/cancel/continue-as-new with `WORKFLOW_TASK_FAILED_CAUSE_UNHANDLED_COMMAND` (`workflow_task_completed_handler.go:800-802,1046-1048`), preventing lost signals.
- **Zombie resurrection:** State machine permits `ZOMBIE → RUNNING` only via explicit valid paths (`mutable_state_state_status.go:93-117`); otherwise `invalidStateTransitionErr` surfaces as internal error, preventing ghost writes from stale shards.
- **Unbounded plan growth without ContinueAsNew:** History length unbounded within retention; operators must explicitly use `ContinueAsNew` to truncate — forgetting causes large histories and replay cost.

## Future Considerations

- Expose a declarative plan view derived from history (ordered list of scheduled/completed activities mapped from `AddActivityTaskScheduled/Started/Completed` events) so UI can diff revisions without parsing proto.
- Enforce payload schemas on `CompleteWorkflowExecution` result (optional JSON-schema check in `ValidateCompleteWorkflowExecutionAttributes`) to catch semantic drift that determinism cannot.
- Make `TemporalChangeVersion` auto-populated from deployment versioning metadata rather than requiring manual `GetVersion` calls, closing the opt-in observability gap.
- Add plan-level reset preview API (dry-run `ResetWorkflow` returning forked history without committing) to let operators validate revisions before forking.

## Questions / Gaps

- No first-class plan DSL or LLM planner artifact was found (searched `service/history`, `api`, `common/searchattribute` for `Plan`, `Planner` — only deployment/schedule terminology exists) — plan intent lives solely in worker code, so intent-vs-execution drift beyond determinism cannot be scored.
- SDK-side `GetVersion`/`Patch` marker implementation is outside this server repository; server evidence rests on `TemporalChangeVersion` indexing (`sadefs/constants.go:26`) and version-stamp propagation (`recordworkflowtaskstarted/api.go:129-168`) — exact marker event mapping was not traced end-to-end.
- Visibility projection for plan revisions flows through async queue tasks (not exhaustively audited here); read-your-writes on `ListClosedWorkflowExecutions` after reset/continue-as-new was not verified within source boundary.

---

Generated by `06.03-plan-lifecycle-and-revision` against `temporal`.

# Source Analysis: agent-framework

## 02.04 — Mutation Discipline and State Transitions

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python 3.10+ (core, providers, workflows) and C# / .NET (Microsoft.Agents.AI.*, Microsoft.Agents.AI.Workflows) |
| Analyzed | 2026-07-06 |

## Summary

Mutation discipline in the agent-framework is built on a **two-tier staging + commit** pattern that is essentially identical across the Python and .NET implementations of the workflow runtime. Writes are queued in a per-executor "pending" buffer (`State._pending` in Python, `StateManager._queuedUpdates` in .NET) and become visible to other executors only after an explicit `commit()` (Python) or `PublishUpdatesAsync()` (.NET) at the superstep boundary. The runner calls the commit exactly once per iteration, so a failing superstep leaves committed state untouched and the next iteration starts from the previous consistent snapshot.

The agent-session layer (outside of workflows) uses a plain mutable dictionary (`AgentSession.state` in Python, `AgentSessionStateBag` in .NET) without staging. The .NET `AgentSessionStateBag` wraps the dictionary in a `ConcurrentDictionary` plus per-value locks (`AgentSessionStateBagValue._lock`) and a typed-deserialization cache; the Python equivalent has no thread-safety primitives at all and relies on the asyncio single-threaded model. Both layers intentionally separate *visibility* (read returns last write) from *atomicity* (a transaction either fully applies or is discarded) — but only the workflow layer actually implements the second property.

Run-level state is modeled with explicit enums (`RunStatus` in .NET, `WorkflowRunState` in Python), but neither language uses a formal finite-state-machine guard. Transitions are encoded as ordinary field assignments that are observed through event emissions (`SuperStepStartedEvent`, `SuperStepCompletedEvent`, `status` events), so transitions are *observable* and *typed* but not *validated*. The system explicitly embraces last-write-wins semantics within a superstep and documents this in code comments.

Concurrency safety is achieved through different mechanisms on each side: Python locks individual file-history files with a 64-stripe lock table (`FileHistoryProvider._FILE_LOCK_STRIPE_COUNT`) and per-event-loop `asyncio.Lock`s; .NET uses `ConcurrentDictionary` for the state bag, `Interlocked.CompareExchange` to gate single-enumerator event-stream consumption (`AsyncRunHandle._isEventStreamTaken`), and a `Volatile.Read`/`Interlocked.Exchange` pair on `_runEnded` and `_needsRepublish` to coordinate disposal and event republish. There is **no version column or optimistic-concurrency check** on user state in either implementation.

## Rating

**8 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale:
- **+** Mutations are routed through a small, well-typed API (`State.set/get/delete/commit/discard` in Python, `QueueStateUpdateAsync`/`PublishUpdatesAsync` in .NET) rather than scattered setters (`python/packages/core/agent_framework/_workflows/_state.py:30-104`; `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateManager.cs:177-233`).
- **+** Staging is explicit and documented; failure cases have dedicated tests (`python/packages/core/tests/workflow/test_state.py:210-267` covers discard semantics, no-partial-commits, and isolated supersteps; `dotnet/tests/Microsoft.Agents.AI.Abstractions.UnitTests/AgentSessionStateBagTests.cs:840` lines cover thread-safety edge cases).
- **+** Mutations emit observable events (`superstep_started`, `superstep_completed`, `executor_invoked`, `executor_completed`, `executor_failed`, `status`, and the .NET `SuperStepCompletedEvent.CompletionInfo.StateUpdated` flag — `dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcStepTracer.cs:25-70`).
- **+** Some impossible states are *unrepresentable*: a key cannot be both "set" and "absent" once committed because pending writes either land in the buffer or in the committed store, never both with conflicting semantics (`python/packages/core/agent_framework/_workflows/_state.py:78-83`, `90-100`).
- **−** No optimistic-concurrency version column; two concurrent writers to the same key in the same superstep silently lose one update (`python/packages/core/agent_framework/_workflows/_state.py:36-42` documents "last write wins").
- **−** `RunStatus` (.NET) and `WorkflowRunState` (Python) are plain enums with no transition guard; an out-of-order assignment compiles and runs (e.g. assigning `Ended` and then `Running` is syntactically valid — `dotnet/src/Microsoft.Agents.AI.Workflows/RunStatus.cs:8-34`, `python/packages/core/agent_framework/_workflows/_events.py:58-67`).
- **−** Python `State` is not thread-safe; it relies entirely on asyncio's single-threaded model and has no internal locks (`python/packages/core/agent_framework/_workflows/_state.py:25-28`). The Python `AgentSession.state` dict is also unsynchronized (`python/packages/core/agent_framework/_sessions.py:772`).
- **−** Some invariants are only enforced by runtime exceptions rather than the type system (e.g. .NET `InvalidOperationException` for "Expected exactly one update for key" — `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateScope.cs:78-82`).

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| State container (Python) | `State` class with `_committed` + `_pending` two-tier dict | `python/packages/core/agent_framework/_workflows/_state.py:6-127` |
| Mutation API (Python) | `set/get/has/delete/clear/commit/discard/export/import` | `python/packages/core/agent_framework/_workflows/_state.py:30-118` |
| Delete sentinel (Python) | `_DeleteSentinel` type + `State.delete` to mark deletion without immediate effect | `python/packages/core/agent_framework/_workflows/_state.py:68-83`, `121-127` |
| Last-write-wins doc (Python) | Comment acknowledging concurrent writes lose data inside a superstep | `python/packages/core/agent_framework/_workflows/_state.py:36-42` |
| Superstep commit (Python) | `Runner.run_until_convergence` calls `self._state.commit()` after each iteration | `python/packages/core/agent_framework/_workflows/_runner.py:140-146` |
| Pre-checkpoint commit (Python) | `_create_checkpoint_if_enabled` commits so pending `on_checkpoint_save` state lands in the snapshot | `python/packages/core/agent_framework/_workflows/_runner.py:218-233` |
| Restore clears stale state (Python) | `restore_from_checkpoint` calls `self._state.clear()` before `import_state` to avoid leaking stale keys | `python/packages/core/agent_framework/_workflows/_runner.py:281-294` |
| Workflow state guard (Python) | `Workflow.run` short-circuits when `is_continuation` and explicitly commits run kwargs | `python/packages/core/agent_framework/_workflows/_workflow.py:540-570` |
| Functional workflow state API | `RunContext.get_state` / `set_state` reject underscore-prefixed keys | `python/packages/core/agent_framework/_workflows/_functional.py:263-302` |
| Reserved key prefix (Python) | Underscore-prefixed keys disallowed for users because framework owns them | `python/packages/core/agent_framework/_workflows/_state.py:20-23`, `_functional.py:294-301` |
| Checkpoint state shape (Python) | `WorkflowCheckpoint` captures committed `state`, `pending_request_info_events`, `iteration_count` | `python/packages/core/agent_framework/_workflows/_checkpoint.py:30-88` |
| Checkpoint encoding (Python) | `FileCheckpointStorage` uses JSON + pickle hybrid, allowlist for safe types | `python/packages/core/agent_framework/_workflows/_checkpoint.py:239-326` |
| Session state (Python) | `AgentSession.state` is a plain `dict[str, Any]`, no thread safety | `python/packages/core/agent_framework/_sessions.py:758-777` |
| Provider state scoping (Python) | `provider_session.state.setdefault(provider.source_id, {})` gives each provider its own sub-dict | `python/packages/core/agent_framework/_sessions.py:625, 645, 482` |
| Agent session serialization (Python) | `_serialize_state` / `_deserialize_state` for round-trip with type registry | `python/packages/core/agent_framework/_sessions.py:42-152`, `779-811` |
| Lock-stripe file writes (Python) | `FileHistoryProvider` 64-stripe lock table + per-loop `asyncio.Lock` | `python/packages/core/agent_framework/_sessions.py:916-996`, `1048-1110` |
| State manager (.NET) | `StateManager` with per-`ScopeId` `StateScope` instances and shared `_queuedUpdates` | `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateManager.cs:13-273` |
| Staging writes (.NET) | `WriteStateAsync` writes to `_queuedUpdates`, not the live `StateScope` | `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateManager.cs:177-188` |
| Single-update invariant (.NET) | `StateScope.WriteStateAsync` throws `InvalidOperationException` if more than one update is queued for a key | `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateScope.cs:67-95` |
| Delete update (.NET) | `StateUpdate.Delete` factory + `ClearStateAsync` flip from update to delete | `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateManager.cs:38-66`, `190-199`; `Execution/StateUpdate.cs:7-26` |
| Commit at superstep boundary (.NET) | `PublishUpdatesAsync` aggregates `_queuedUpdates` by scope and applies them | `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateManager.cs:201-233` |
| Export guard (.NET) | `ExportStateAsync` refuses to run while `_queuedUpdates` is non-empty | `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateManager.cs:243-251` |
| Import guard (.NET) | `ImportStateAsync` clears queued updates + scopes and repopulates from checkpoint | `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateManager.cs:253-273` |
| Read API (.NET) | `BoundWorkflowContext` exposes `ReadStateAsync`, `ReadOrInitStateAsync`, `ReadStateKeysAsync`, `QueueStateUpdateAsync`, `QueueClearScopeAsync` | `dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcessRunnerContext.cs:366-380` |
| RunStatus enum (.NET) | `NotStarted`, `Idle`, `PendingRequests`, `Ended`, `Running` — no transition guard | `dotnet/src/Microsoft.Agents.AI.Workflows/RunStatus.cs:8-34` |
| Run state reads (.NET) | `Run.GetStatusAsync` delegates to `AsyncRunHandle._eventStream.GetStatusAsync` | `dotnet/src/Microsoft.Agents.AI.Workflows/Run.cs:46-47` |
| Event stream gating (.NET) | `Interlocked.CompareExchange` on `_isEventStreamTaken` allows exactly one enumerator | `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/AsyncRunHandle.cs:60-93` |
| Run end flag (.NET) | `Volatile.Read(ref _runEnded)` in `CheckEnded` and `Interlocked.Exchange` to set | `dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcessRunnerContext.cs:491-497`, `499-524` |
| Pending-request flag (.NET) | `_needsRepublish` `Volatile.Write` / `Interlocked.Exchange` for atomic event replay after restore | `dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcessRunner.cs:91-91`, `187-206` |
| Single-active-runner check (.NET) | `ConcurrentDictionary<string, Task<Executor>>` ensures each executor is instantiated exactly once | `dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcessRunnerContext.cs:85-119` |
| Workflow ownership (.NET) | `TakeOwnership` / `ReleaseOwnership` and `enableConcurrentRuns` gate | `dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcessRunnerContext.cs:41-72`; `InProc/InProcessRunner.cs:45-52` |
| Stateful executor cache (.NET) | `StatefulExecutor._stateCache` skipped when `ConcurrentRunsEnabled == true` | `dotnet/src/Microsoft.Agents.AI.Workflows/StatefulExecutor.cs:60-94`, `108-143` |
| AgentSession (.NET) | Abstract base class with `StateBag` property | `dotnet/src/Microsoft.Agents.AI.Abstractions/AgentSession.cs:84-85` |
| Concurrent state bag (.NET) | `AgentSessionStateBag` backed by `ConcurrentDictionary` | `dotnet/src/Microsoft.Agents.AI.Abstractions/AgentSessionStateBag.cs:21-145` |
| Per-value locking (.NET) | `AgentSessionStateBagValue._lock` around `JsonValue` getter/setter and deserialization cache | `dotnet/src/Microsoft.Agents.AI.Abstractions/AgentSessionStateBagValue.cs:14-66`, `74-165` |
| Type mismatch enforcement (.NET) | Throws `InvalidOperationException` when requested type ≠ cached type | `dotnet/src/Microsoft.Agents.AI.Abstractions/AgentSessionStateBagValue.cs:89-91`, `134-135` |
| Provider state (.NET) | `ProviderSessionState<T>` for typed get/save with key isolation | `dotnet/src/Microsoft.Agents.AI.Abstractions/ProviderSessionState{TState}.cs:26-87` |
| InMemory chat history (.NET) | `InMemoryChatHistoryProvider` stores `List<ChatMessage>` in `StateBag` and supports reducer-based mutation | `dotnet/src/Microsoft.Agents.AI.Abstractions/InMemoryChatHistoryProvider.cs:27-134` |
| Workflow session (.NET) | `WorkflowSession` tracks `_pendingRequests` dictionary for response correlation | `dotnet/src/Microsoft.Agents.AI.Workflows/WorkflowSession.cs:52`, `214-288` |
| Response port validation (.NET) | `AddExternalResponseAsync` rejects mismatched `PortId` to prevent forged routing | `dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcessRunnerContext.cs:147-182` |
| External request registry (.NET) | `ConcurrentDictionary<string, ExternalRequest> _externalRequests` with `TryAdd` uniqueness check | `dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcessRunnerContext.cs:39`, `313-322` |
| Workflow run state (Python) | `WorkflowRunState` enum: `STARTED, IN_PROGRESS, IN_PROGRESS_PENDING_REQUESTS, IDLE, IDLE_WITH_PENDING_REQUESTS, FAILED, CANCELLED` | `python/packages/core/agent_framework/_workflows/_events.py:58-67` |
| Run state assignment (Python) | `Workflow` mutates `_status` directly during run with no validation | `python/packages/core/agent_framework/_workflows/_workflow.py:589`, `595`, `600`, `616` |
| Workflow events emitted (Python) | `started`, `status`, `failed`, `superstep_started`, `superstep_completed`, `executor_invoked/completed/failed/bypassed`, `request_info` | `python/packages/core/agent_framework/_workflows/_events.py:104-130`, `255-342` |
| Event origin guard (Python) | `_framework_event_origin` context var + `_OUTPUT_SELECTION_EVENT_TYPES` / `_FRAMEWORK_LIFECYCLE_EVENT_TYPES` reservations | `python/packages/core/agent_framework/_workflows/_events.py:38-56`, `_workflow_context.py:202-205` |
| State event types | `WorkflowContext.add_event` rejects `output/intermediate/started/status/failed` from executor origin | `python/packages/core/agent_framework/_workflows/_workflow_context.py:372-391` |
| Tracer flag (.NET) | `InProcStepTracer.TraceStatePublished` sets `StateUpdated` exposed via `SuperStepCompletedEvent` | `dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcStepTracer.cs:17-26`, `64-70` |
| Checkpoint tracer hook (.NET) | `TraceCheckpointCreated` records the latest checkpoint info into `SuperStepCompletionInfo` | `dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcStepTracer.cs:26`; `InProc/InProcessRunner.cs:367` |
| Tests for staging (Python) | `tests/workflow/test_state.py` covers commit, discard, delete-with-sentinel, failure rollback | `python/packages/core/tests/workflow/test_state.py:56-303` |
| Tests for state bag (.NET) | `AgentSessionStateBagTests` (840 lines) cover keys, values, serialization, concurrency | `dotnet/tests/Microsoft.Agents.AI.Abstractions.UnitTests/AgentSessionStateBagTests.cs:12-840` |
| Tests for checkpoint restore | `CheckpointResumeTests.cs` and `CheckpointParentTests.cs` cover restore + parent chains | `dotnet/tests/Microsoft.Agents.AI.Workflows.UnitTests/CheckpointResumeTests.cs`, `CheckpointParentTests.cs` |
| Tests for export-during-pending (.NET) | `ExportStateAsync` throws if `_queuedUpdates.Count != 0` (impossible state impossible by construction) | `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateManager.cs:245-251` |
| Reserved `_executor_state` (Python) | Framework writes executor snapshots to `State.set(EXECUTOR_STATE_KEY, ...)` during checkpoint | `python/packages/core/agent_framework/_workflows/_runner.py:367-379` |

## Answers to Dimension Questions

### 1. Can any module mutate state?
**Yes — but only through the staged API for workflow state.**

- *Workflow state* in Python flows exclusively through `WorkflowContext.set_state` / `WorkflowContext.get_state` (`python/packages/core/agent_framework/_workflows/_workflow_context.py:426-432`) and the underlying `State.set`/`get`/`delete` methods. There is no public `State._pending` / `_committed` direct-access API, but `_state` attributes are accessible to subclasses and tests intentionally poke at the private attributes to verify staging (`python/packages/core/tests/workflow/test_state.py:64-66`).
- *Workflow state* in .NET flows through `IWorkflowContext.QueueStateUpdateAsync` / `ReadStateAsync` / `ReadOrInitStateAsync` (`dotnet/src/Microsoft.Agents.AI.Workflows/IWorkflowContext.cs:68-186`), and the only mutation entry point is `StateManager.WriteStateAsync` (`dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateManager.cs:177-188`). Other modules cannot mutate state directly — `StateManager._scopes` and `_queuedUpdates` are private.
- *Agent session state* is mutable by any code that holds the `AgentSession` instance. Python's `AgentSession.state` is a plain `dict` (`python/packages/core/agent_framework/_sessions.py:772`); .NET's `AgentSessionStateBag` exposes `GetValue<T>` / `SetValue<T>` / `TryGetValue<T>` / `TryRemoveValue` (`dotnet/src/Microsoft.Agents.AI.Abstractions/AgentSessionStateBag.cs:55-117`), with concurrency safety via the underlying `ConcurrentDictionary`.

### 2. Are illegal transitions prevented?
**Partially.** Illegal workflow transitions *cannot be prevented by the type system* in either language, but the framework uses reservations and runtime checks to discourage them:

- Workflow output events (`output`, `intermediate`) and lifecycle events (`started`, `status`, `failed`) are reserved for the framework and rejected from executor origin with a warning + a synthetic `warning` event (`python/packages/core/agent_framework/_workflows/_workflow_context.py:372-391`).
- `_OUTPUT_SELECTION_EVENT_TYPES` and `_FRAMEWORK_LIFECYCLE_EVENT_TYPES` are declared as `frozenset` constants so the reserved set is immutable (`python/packages/core/agent_framework/_workflows/_workflow_context.py:202-205`).
- Reserved keys in Python `State` (`_`-prefixed) are silently overwritten by the framework and **explicitly rejected at runtime** by `FunctionalWorkflow.RunContext.set_state` (`python/packages/core/agent_framework/_workflows/_functional.py:294-301`) but **not** by the core `State.set`. The mismatch is a documented footgun: user keys in the underscore namespace will be clobbered by checkpoint save and dropped on restore.
- `.NET StateScope.WriteStateAsync` throws `InvalidOperationException` if a single superstep produces more than one update for the same key (`dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateScope.cs:78-82`); this prevents a logically-illegal state ("two writers, one slot") only if every author follows the staging convention.
- `RunStatus` (.NET) and `WorkflowRunState` (Python) are plain enums; any-to-any assignment is allowed by the type system.

### 3. Are concurrent writes safe?
**Conditionally safe.**

- *Within a single Python asyncio loop*: yes. `State` has no locks but the model is single-threaded; the framework documents that "When multiple executors run concurrently within the same superstep, each executor's writes go to the same pending buffer. The last write for a given key wins when commit() is called" (`python/packages/core/agent_framework/_workflows/_state.py:36-42`). This is documented behavior, not a failure mode.
- *Across Python processes / event loops*: `FileHistoryProvider` uses a 64-stripe `threading.Lock` table keyed by `hash(file_path)` plus per-event-loop `asyncio.Lock`s for the same path (`python/packages/core/agent_framework/_sessions.py:916-996`, `1048-1110`).
- *.NET* uses `ConcurrentDictionary` for `AgentSessionStateBag`, per-value `lock` blocks on `AgentSessionStateBagValue`, `ConcurrentDictionary<string, Task<Executor>>` to ensure single executor instantiation (`dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcessRunnerContext.cs:35`), `Interlocked.CompareExchange` to gate event-stream enumeration (`dotnet/src/Microsoft.Agents.AI.Workflows/Execution/AsyncRunHandle.cs:60-93`), and `Volatile.Write`/`Interlocked.Exchange` for the `_needsRepublish` and `_runEnded` flags.
- *Workflow runs*: the .NET `StatefulExecutor` opts out of caching when `ConcurrentRunsEnabled == true`, forcing a re-read of state from the manager (`dotnet/src/Microsoft.Agents.AI.Workflows/StatefulExecutor.cs:70-73`, `88-91`, `114-142`). This is the only place where the framework acknowledges and adapts to concurrent runs.
- **No version column / ETag / OCC retry loop** in either implementation. There is no way to detect a stale write.

### 4. Are mutations observable?
**Yes — through events.**

- Python emits `superstep_started`, `superstep_completed`, `executor_invoked`, `executor_completed`, `executor_failed`, `executor_bypassed`, `status`, `started`, `failed`, `request_info`, `output`, `intermediate`, `warning`, `error` (`python/packages/core/agent_framework/_workflows/_events.py:104-130`, factory methods at `255-342`).
- The `WorkflowRunState` value is broadcast via `WorkflowEvent.status(...)` events on each transition (`python/packages/core/agent_framework/_workflows/_workflow.py:589-619`).
- `.NET` emits `SuperStepStartedEvent` and `SuperStepCompletedEvent` carrying `SuperStepStartInfo` and `SuperStepCompletionInfo`. `SuperStepCompletionInfo.StateUpdated` is set whenever `TraceStatePublished` was called in the step (`dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcStepTracer.cs:17-26`, `64-70`). The completion info also exposes `Checkpoint` and the activated/instantiated executor sets.
- OpenTelemetry: the runner wraps message sends and workflow execution in OTel activities (`dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcessRunnerContext.cs:213-251`; `python/packages/core/agent_framework/_workflows/_workflow_context.py:323-336`), so state mutations are observable through tracing.
- **No mutation-level audit log**: per-key state writes are not individually emitted as events. Observability is at superstep granularity, not per-key granularity.

### 5. Can state become internally inconsistent?
**Mostly no — by construction.**

- The staging buffer means that within a superstep, partial updates cannot be observed by other executors; only committed state is visible cross-superstep (`python/packages/core/agent_framework/_workflows/_state.py:45-60`, `90-100`; `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateManager.cs:201-233`).
- Deletion is staged via sentinel / delete-update, so a "delete" cannot race a "set" — whichever update was applied last to the same key wins (`python/packages/core/agent_framework/_workflows/_state.py:68-83`; `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateUpdate.cs:22-26`).
- The .NET `StateScope.WriteStateAsync` rejects multiple updates for the same key within a single superstep with `InvalidOperationException`, preventing partial-commit ambiguity (`dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateScope.cs:78-82`).
- `StateManager.ExportStateAsync` refuses to export while `_queuedUpdates` is non-empty, preventing checkpoint snapshots from including uncommitted data (`dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateManager.cs:243-251`).
- `StateManager.ImportStateAsync` clears any pending updates first, ensuring restore never mixes live and checkpointed data (`dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateManager.cs:253-273`).
- **Possible inconsistent states**:
  - Run-status can be set to any enum value at any time (no FSM guard).
  - `_pendingRequests` in `WorkflowSession` is a regular `Dictionary` (not `ConcurrentDictionary`) and only mutated from the session's own invocation path, so concurrent cross-session calls could corrupt it (`dotnet/src/Microsoft.Agents.AI.Workflows/WorkflowSession.cs:52`).
  - `RunContext._events` / `_step_cache` / `_pending_requests` in Python `_functional.py` are plain `dict`s and `list`s with no synchronization, but they are owned by a single execution (`python/packages/core/agent_framework/_workflows/_functional.py:166-184`).

## Architectural Decisions

1. **Two-tier staging is the universal pattern.** Both languages implement the same shape: pending writes are queued, then applied atomically at a synchronization point. This was chosen because it makes superstep semantics verifiable: every "what did the world look like at step N" question has a clean answer drawn from committed state. Evidence: `python/packages/core/agent_framework/_workflows/_state.py:25-104`; `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateManager.cs:13-273`.
2. **Reserved key prefix as a soft contract.** Underscore-prefixed keys are framework-owned in both languages. Python's `FunctionalWorkflow.RunContext.set_state` enforces this with a `ValueError`; Python's `State.set` and .NET's `StateManager` do not. This is a deliberately inconsistent enforcement level — the contract is "don't" with a runtime check at the user-facing layer only.
3. **Last-write-wins is documented, not prevented.** The Python `State` docstring explicitly acknowledges that concurrent executors overwrite each other's writes inside a superstep (`python/packages/core/agent_framework/_workflows/_state.py:36-42`). The .NET equivalent rejects the case at write time per scope/key. The asymmetry is significant: Python is permissive + documented, .NET is strict + exception.
4. **No formal FSM for run state.** `RunStatus` (.NET) and `WorkflowRunState` (Python) are enums with comments, not transition tables. The author intentionally relies on the runner to do the right thing — there is no guard rejecting `RunStatus.Ended → RunStatus.Running`.
5. **Per-value locking in the .NET state bag.** `AgentSessionStateBagValue` carries its own `_lock` (`dotnet/src/Microsoft.Agents.AI.Abstractions/AgentSessionStateBagValue.cs:15`) so that JsonValue serialization can run concurrently with reads on different keys, while reads on the same key serialize. The design optimizes for the common case (many keys, infrequent contention) rather than full serialization.
6. **Single-enumerator event stream.** `AsyncRunHandle._isEventStreamTaken` uses `Interlocked.CompareExchange` to allow exactly one consumer of the event stream at a time (`dotnet/src/Microsoft.Agents.AI.Workflows/Execution/AsyncRunHandle.cs:64-67`). This avoids the cost of a full lock for a property that only flips at most once per run.
7. **Checkpoints are committed-state-only.** The Python `WorkflowCheckpoint.state` is built from `state.export_state()` which returns the committed dict only (`python/packages/core/agent_framework/_workflows/_state.py:106-111`). The .NET `StateData` is similarly built post-`PublishUpdatesAsync` (`dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcessRunner.cs:341-369`). This is the most important consistency guarantee in the whole system — there is no way to capture a snapshot of partially-applied writes.
8. **Reserved event types and `_framework_event_origin` context var.** Python uses a `ContextVar` to mark events emitted from framework code so that downstream consumers can distinguish framework bookkeeping from executor-originated events (`python/packages/core/agent_framework/_workflows/_events.py:38-56`). This shapes observability — the origin field is the transition guard.

## Notable Patterns

- **Sentinel object for deletion**: `python/packages/core/agent_framework/_workflows/_state.py:121-127` defines `_DeleteSentinel` as a unique object instance, not a string or boolean, so a `delete` of a pending key cannot be confused with a normal write.
- **Update-vs-delete factory on the value type**: `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateUpdate.cs:20` — `Update<T>(key, null)` is *also* a delete because `value is null` becomes `isDelete = true`. This is a quiet source of false deletes; a user explicitly storing `null` will see their key vanish.
- **Causal context for message delivery**: `python/packages/core/agent_framework/_workflows/_runner_context.py:36-94` carries `trace_contexts: list[dict[str, str]]` and `source_span_ids: list[str]` on `WorkflowMessage`, so fan-in scenarios preserve provenance of every contributing message.
- **Lock-stripe hashing for file writes**: `python/packages/core/agent_framework/_sessions.py:1118-1120` — `hash(file_path) % _FILE_LOCK_STRIPE_COUNT` maps each session file to one of 64 stripes, avoiding a single contended lock across all sessions.
- **Pending-request bookkeeping at the session boundary**: `dotnet/src/Microsoft.Agents.AI.Workflows/WorkflowSession.cs:218-288` correlates incoming response `ContentId`s to pending requests by *workflow-facing* ID, not by call id, with a per-call `HashSet<string>` of matched content ids to deduplicate double responses.
- **Failure drains events before propagating**: `python/packages/core/agent_framework/_workflows/_runner.py:122-130` and `dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcessRunner.cs:230-235` ensure that `ExecutorFailedEvent` / `WorkflowErrorEvent` reach the consumer even when the underlying exception propagates out of the iteration loop.

## Tradeoffs

- **Staging is great for consistency, but it hides live edits from the writer's own subsequent code in the same superstep.** The Python `State.get` checks pending first (`python/packages/core/agent_framework/_workflows/_state.py:55-60`) so within the same executor this works correctly; across executors, however, an executor that writes a key in iteration N will not see that write when it runs in iteration N+1 if the iteration does not yet call `commit()` — and the commit is owned by the runner, not the writer.
- **Python's permissive last-write-wins + .NET's strict single-update-per-key exception** create asymmetric authoring experiences. A workflow written in Python that happens to have two executors writing the same key will silently lose one update; the same workflow ported to .NET will throw.
- **No optimistic concurrency** means that a stale restore + new run sequence is only caught by `graph_signature_hash` mismatch at restore time (`python/packages/core/agent_framework/_workflows/_runner.py:275-279`). If the graph topology changed, restore fails; if only data drifted, restore silently applies stale data.
- **`ConcurrentDictionary` in .NET gives parallelism for *reading* the state bag, but every per-value write serializes on `_lock`** (`dotnet/src/Microsoft.Agents.AI.Abstractions/AgentSessionStateBagValue.cs:15`). For hot paths with frequent writes to the same key, contention is real.
- **`AgentSessionStateBag.SetValue` uses `GetOrAdd`** so two concurrent `SetValue` calls on a fresh key produce *one* `AgentSessionStateBagValue` that is then mutated by both writers via `SetDeserialized`. The lock on `_lock` prevents corruption but the order of the last `SetDeserialized` is undefined.
- **Workflow checkpoint is committed-state-only**, which is correct, but it also means a checkpoint created in the middle of a superstep will not capture in-flight writes — the runner's `_create_checkpoint_if_enabled` therefore commits explicitly *before* snapshotting (`python/packages/core/agent_framework/_workflows/_runner.py:218-233`). This adds a hidden ordering constraint: callers must not rely on "pending after last commit" semantics across checkpoint boundaries.
- **The .NET `StateScope.WriteStateAsync` requires "exactly one update per key" but `StateManager.WriteStateAsync` allows the same executor to call it multiple times in a single superstep**. The single-update invariant is enforced only on commit, not on queue — meaning a misbehaving writer can stage writes that throw at the next `PublishUpdatesAsync` rather than at write time.

## Failure Modes / Edge Cases

- **`StateUpdate.Update(key, null)` is a delete**: `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateUpdate.cs:20` interprets `value is null` as `isDelete: true`, so a user explicitly storing `null` will lose their data on next `PublishUpdatesAsync`. The Python equivalent uses an explicit `_DeleteSentinel`, which is clearer and avoids this trap.
- **Multiple updates to the same scope+key within one superstep throw**: `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateScope.cs:78-82`. This is desirable, but the throw happens *after* the iteration completed, so the writer has no way to recover mid-iteration.
- **`ExportStateAsync` rejects mid-superstep export**: `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateManager.cs:245-251` throws `InvalidOperationException` if any pending updates exist. The runner avoids this by always committing before checkpointing, but a third-party caller of `ExportStateAsync` can easily trip this.
- **Restore does not validate executor-set vs checkpoint-set identity**: `_restore_executor_states` checks that the executor exists and that its state is a `dict[str, Any]`, but does not check schema (`python/packages/core/agent_framework/_workflows/_runner.py:320-340`). A checkpoint from a workflow version that used different executor state shapes will silently restore a malformed state.
- **Functional workflow `set_state` raises `ValueError` on underscore keys but core `State.set` does not**: `python/packages/core/agent_framework/_workflows/_functional.py:294-301` vs. `python/packages/core/agent_framework/_workflows/_state.py:30-43`. A user who bypasses the functional workflow and writes directly to `WorkflowContext.set_state` (or `State.set`) can still write `_foo` and have it clobbered.
- **`_pendingRequests` is a plain `Dictionary`**: `dotnet/src/Microsoft.Agents.AI.Workflows/WorkflowSession.cs:52` is not concurrent; the design assumes the session is driven from one consumer at a time, but the workflow layer does not enforce this.
- **`AgentSessionStateBag.SetValue` type-safety is per-call**: A user can `SetValue("key", new Foo())` and later `GetValue<Bar>("key")` and the deserializer will silently return `null` rather than throw, because `TryReadDeserializedValue` returns false on cache-type mismatch and callers are expected to check (`dotnet/src/Microsoft.Agents.AI.Abstractions/AgentSessionStateBagValue.cs:89-91`).
- **`State.delete` raises `KeyError` for missing keys**: `python/packages/core/agent_framework/_workflows/_state.py:75-77`. This makes "delete-if-exists" not idempotent — a user who wants idempotent delete must first `has()`.
- **Python `WorkflowCheckPoint.version` is a static `"1.0"` string**: `python/packages/core/agent_framework/_workflows/_checkpoint.py:88`. There is no version negotiation on load; the field is descriptive only.

## Future Considerations

1. **Promote run-status to a transition table.** Today `RunStatus` (.NET) and `WorkflowRunState` (Python) are plain enums. A guard helper (e.g. `CanTransition(from, to)`) backed by a unit test would prevent `Ended → Running` and other illegal moves without changing the wire format.
2. **Add an optional optimistic-concurrency check to `State.set`.** A monotonic version (per-executor or per-key) could be carried in the `WorkflowCheckpoint` and validated on restore, catching the case where the workflow topology changed between save and load in ways the `graph_signature_hash` doesn't capture.
3. **Tighten the underscore-key contract.** Either enforce at the `State.set` level too (rejecting any write whose key starts with `_` from non-framework callers), or remove the prefix reservation and use a separate `framework_state: dict` field. The current split-brain approach is the source of the "silently clobbered" footgun.
4. **Normalize the Python / .NET last-write-wins divergence.** Either align Python on the .NET exception model (catch conflicts at commit) or align .NET on the Python last-write-wins model. Today, a workflow that works in one language may behave differently in the other.
5. **Surface per-key state writes as observability events.** Today mutations are only visible at superstep granularity (`SuperStepCompletedEvent.StateUpdated`). Per-key audit events would make it possible to trace exactly which executor changed which key.
6. **Add thread-safety to Python `AgentSession.state`** if the framework ever wants to expose the session to multiple concurrent callers (e.g. parallel sub-agent invocations). Today the only safety net is `await`-based serialization.
7. **Replace the `_lock` pattern in `AgentSessionStateBagValue` with a `ReaderWriterLockSlim` or `[MethodImpl(MethodImplOptions.Synchronized)]` upgrade**, depending on profile. The current per-value lock is simple but blocks all readers during writes, even for unrelated keys (the lock is per-value, but `JsonValue.get` always writes to `_jsonValue`).

## Questions / Gaps

- **No evidence found** for: a formal finite-state machine for `WorkflowRunState` transitions. The enum is used as a marker, not a guard. The framework relies on the runner code being correct.
- **No evidence found** for: optimistic concurrency control (ETag, version column) on user state in either language.
- **No evidence found** for: per-key mutation observability events. Only superstep-level observability exists.
- **No evidence found** for: a "lock-free" `State` implementation in Python. All safety rests on the asyncio single-threaded assumption, which is fine but should be documented at every public entry point that mutates `state`.
- **Gap**: `WorkflowSession._pendingRequests` is a plain `Dictionary`, not `ConcurrentDictionary`. If the agent framework ever exposes a session to parallel callers, this will be a contention hot spot. (`dotnet/src/Microsoft.Agents.AI.Workflows/WorkflowSession.cs:52`).
- **Gap**: the `WorkflowCheckpoint.version` field is decorative — there is no schema versioning code that reads it on load (`python/packages/core/agent_framework/_workflows/_checkpoint.py:88`). Loading a "1.0" checkpoint works; loading a "2.0" checkpoint with a different shape would deserialize without complaint.
- **Gap**: `_executor_state` is the only reserved key explicitly documented, but `WORKFLOW_RUN_KWARGS_KEY` and `_step_cache` are also reserved and only mentioned in source comments (`python/packages/core/agent_framework/_workflows/_workflow.py:567-569`; `python/packages/core/agent_framework/_workflows/_functional.py:118-119`). Users have to read the source to know which keys are off-limits.

---

Generated by `02.04-mutation-discipline-and-state-transitions.md` against `agent-framework`.
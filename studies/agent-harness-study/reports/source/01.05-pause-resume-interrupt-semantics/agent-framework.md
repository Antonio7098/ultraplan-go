# Source Analysis: agent-framework

## Pause, Resume, and Interrupt Semantics

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (and .NET in parallel) – asyncio workflows, Durable Task Scheduler integration |
| Analyzed | 2026-07-02 |

## Summary

Microsoft Agent Framework (MAF) implements a deliberately multi-tier pause/resume model built on the
Pregel-style superstep execution loop defined in
`python/packages/core/agent_framework/_workflows/_runner.py:78` and the request/response mechanism
defined in `python/packages/core/agent_framework/_workflows/_workflow_context.py:393` /
`python/packages/core/agent_framework/_workflows/_request_info_mixin.py:29`. The framework exposes
three distinct ways for an execution to stop safely and resume later:

1. **Human-in-the-loop pause** via `WorkflowContext.request_info` /
   `RunContext.request_info` — emits a `WorkflowEvent` of type `"request_info"`
   (`_events.py:297`) and settles the workflow into `WorkflowRunState.IDLE_WITH_PENDING_REQUESTS`
   (`_events.py:65`, `workflow.py:595`). Resumption happens through the same `run()` call with
   `responses={request_id: value}` (`workflow.py:936`, `functional.py:813`).
2. **Persistent checkpointing** via the `WorkflowCheckpoint` dataclass
   (`_checkpoint.py:30`) and `CheckpointStorage` Protocol (`_checkpoint.py:119`). Checkpoints are
   created automatically at superstep boundaries
   (`_runner.py:96-144`) and can be replayed across process restarts
   (`workflow.py:660`, `workflow.py:912`).
3. **Cross-process durable orchestration** via the
   `agent_framework_durabletask._workflows` package, which translates the same pause/resume
   model onto Azure Durable Functions
   (`durabletask/agent_framework_durabletask/_workflows/orchestrator.py:840-900`,
   `client.py:243-310`). The .NET mirror at
   `dotnet/src/Microsoft.Agents.AI.DurableTask/Workflows/DurableExecutorDispatcher.cs:104-130`
   uses `TaskOrchestrationContext.WaitForExternalEvent` to implement the same semantics.

In addition, the `@workflow`/`@step` functional API
(`_functional.py:108-250`) provides a single-thread replayable pause model: the
`WorkflowInterrupted` `BaseException` (`_functional.py:87`) is the framework's interrupt signal,
captured by `_run_core` (`_functional.py:1027`) so that authors do not see it. Per-step result
caching (`_functional.py:170`) plus per-step checkpointing
(`_functional.py:956-964`) make replay deterministic. Recovery, however, is single-branch: the
checkpoint chain (`_functional.py:1131`, `workflow.py:330`) is consumed by the same workflow
instance — there is no API to fork a checkpoint into two distinct runs.

Resume is *deterministic* against the workflow's `graph_signature_hash`
(`_workflow.py:1015-1080`) and against auto-generated `request_id`s
(`_functional.py:229-234`) — the framework refuses to restore a checkpoint when the graph
topology has changed (`_runner.py:275-279`). What the framework does not implement is
multi-party resume arbitration: pending `request_info` events are keyed by `request_id`, so any
caller holding a pending `request_id` can answer it, but there is no first-responder
serialization primitive. The reusable `Workflow._state` is deliberately shared across
`run()` calls (`workflow.py:541-547`), so a workflow instance behaves like a single execution
branch unless callers use checkpoint IDs to start a new process from a known point.

## Rating

**9 / 10** — Mature, durable, observable, extensible, and proven under failure or scale.

Rationale: MAF offers an explicit, typed pause model with three layers (HITL, durable
checkpoint, durable orchestration), automatic checkpointing at every superstep boundary,
graph-topology validation on restore, a documented resume command (`run(checkpoint_id=...)` /
`run(responses=...)`), explicit `WorkflowRunState` semantics
(`_events.py:58-67`), and a fully-tested `DurableWorkflowClient` integration
(`client.py:243-310`, `test_09_dt_workflow_hitl.py`). Multiple backends (in-memory, file,
Durable Task) and a clear, layered protocol (`CheckpointStorage`,
`RunnerContext`) make this dimension durable and extensible. The .5 deduction reflects
absent multi-party resume arbitration and a known limitation in
`WorkflowExecutor` cross-workflow checkpoint restoration
(`_workflow_executor.py:525-526`).

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Workflow run state machine | `WorkflowRunState` enum with `IDLE_WITH_PENDING_REQUESTS` and `IN_PROGRESS_PENDING_REQUESTS` | `python/packages/core/agent_framework/_workflows/_events.py:58-67` |
| Lifecycle/status events | `started`/`status`/`failed`/`superstep_started`/`superstep_completed` factories on `WorkflowEvent` | `python/packages/core/agent_framework/_workflows/_events.py:255-322` |
| HITL pause API | `WorkflowContext.request_info(request_data, response_type, *, request_id=None)` emits a `request_info` event via the runner context | `python/packages/core/agent_framework/_workflows/_workflow_context.py:393-424` |
| Functional HITL API | `RunContext.request_info` raises `WorkflowInterrupted` to suspend the workflow; deterministic `auto::<index>` request_id fallback | `python/packages/core/agent_framework/_workflows/_functional.py:193-250` |
| Interrupt signal | `WorkflowInterrupted` defined as `BaseException` so user `except Exception:` clauses cannot swallow it | `python/packages/core/agent_framework/_workflows/_functional.py:83-99` |
| Response routing | `InProcRunnerContext.send_request_info_response` validates type and converts pending `request_info` events into `WorkflowMessage(type=MessageType.RESPONSE)` | `python/packages/core/agent_framework/_workflows/_runner_context.py:244-486` |
| Pending request tracking | `_pending_request_info_events: dict[str, WorkflowEvent[Any]]` on `InProcRunnerContext` | `python/packages/core/agent_framework/_workflows/_runner_context.py:292` |
| Request handler contract | `RequestInfoMixin` + `@response_handler` decorator discover handlers via `_discover_response_handlers` | `python/packages/core/agent_framework/_workflows/_request_info_mixin.py:29-105` |
| Checkpoint data class | `WorkflowCheckpoint` captures workflow name, graph signature, messages, state, pending request info events, iteration count | `python/packages/core/agent_framework/_workflows/_checkpoint.py:30-89` |
| Checkpoint schema docstring | Lists reserved keys, "captures the full execution state of a workflow at a specific point" | `python/packages/core/agent_framework/_workflows/_checkpoint.py:31-69` |
| Checkpoint storage Protocol | `CheckpointStorage` defines async save/load/list/delete/get_latest/list_checkpoint_ids contract | `python/packages/core/agent_framework/_workflows/_checkpoint.py:119-189` |
| In-memory backend | `InMemoryCheckpointStorage` for tests/local dev (deep-copies on save) | `python/packages/core/agent_framework/_workflows/_checkpoint.py:192-236` |
| File backend | `FileCheckpointStorage` writes JSON, pickles opaque payloads, path-traversal-safe, supports `allowed_checkpoint_types` allowlist | `python/packages/core/agent_framework/_workflows/_checkpoint.py:239-450` |
| Checkpoint creation in runner | `_create_checkpoint_if_enabled` calls `on_checkpoint_save` on every executor and persists the new checkpoint at superstep boundary | `python/packages/core/agent_framework/_workflows/_runner.py:212-238` |
| Checkpoint chain | `previous_checkpoint_id` is threaded across supersteps to form a checkpoint history | `python/packages/core/agent_framework/_workflows/_runner.py:84, 97, 144` |
| First-checkpoint semantics | Special-case checkpoint before the iteration loop captures superstep-0 state | `python/packages/core/agent_framework/_workflows/_runner.py:92-97` |
| Restore entry point | `Runner.restore_from_checkpoint` validates graph hash, clears state, imports checkpoint state, applies checkpoint to context, marks runner as resumed | `python/packages/core/agent_framework/_workflows/_runner.py:240-300` |
| Resume flag | `_resumed_from_checkpoint` is set on restore and consulted to skip the first-checkpoint creation | `python/packages/core/agent_framework/_workflows/_runner.py:67, 96, 156, 364` |
| Graph signature | SHA-256 of canonical JSON form of executors, edges, fanout selection, etc. | `python/packages/core/agent_framework/_workflows/_workflow.py:1015-1080` |
| Graph signature mismatch error | Restore raises `WorkflowCheckpointException("Workflow graph has changed ...")` when hash differs | `python/packages/core/agent_framework/_workflows/_runner.py:274-279` |
| Resume command (graph API) | `Workflow.run(checkpoint_id=..., checkpoint_storage=..., responses=...)` overloaded with `stream: Literal[True/False]` | `python/packages/core/agent_framework/_workflows/_workflow.py:701-770` |
| Resume execution mode | `_resolve_execution_mode` chooses restore, restore+send responses, or responses-only path | `python/packages/core/agent_framework/_workflows/_workflow.py:891-910` |
| Restore + send responses | `_restore_and_send_responses` and `_send_responses_internal` enforce request_id match and type coercion | `python/packages/core/agent_framework/_workflows/_workflow.py:912-960` |
| Resume validation | `_validate_run_params` rejects illegal combos (message+responses, message+checkpoint_id) | `python/packages/core/agent_framework/_workflows/_workflow.py:866-889` |
| Run state transitions | `_run_workflow_with_tracing` updates `WorkflowRunState` to `IN_PROGRESS`, `IN_PROGRESS_PENDING_REQUESTS`, `IDLE_WITH_PENDING_REQUESTS`, `IDLE`, `FAILED` | `python/packages/core/agent_framework/_workflows/_workflow.py:532-618` |
| Per-run reset semantics | State, messages, events, iteration counter handled distinctly so multi-turn input works; only iteration + kwargs are per-run | `python/packages/core/agent_framework/_workflows/_workflow.py:537-574` |
| Concurrent-run guard | `_ensure_not_running` prevents two `run()` calls on the same instance from interleaving | `python/packages/core/agent_framework/_workflows/_workflow.py:379-383` |
| `ResponseStream` cleanup | `_run_cleanup` clears runtime checkpoint storage override after each run | `python/packages/core/agent_framework/_workflows/_workflow.py:832-836` |
| `responses`-only suppress | When resuming with `responses=...`, request_info events whose id is answered are dropped from the visible stream | `python/packages/core/agent_framework/_workflows/_workflow.py:821-829` |
| `In-flight message guard` | If a prior run left executor messages undrained, a fresh `run(message=...)` is rejected with a "resume from checkpoint" hint | `python/packages/core/agent_framework/_workflows/_workflow.py:803-810` |
| Functional API run signature | `FunctionalWorkflow.run(message, *, stream, responses, checkpoint_id, checkpoint_storage)` | `python/packages/core/agent_framework/_workflows/_functional.py:761-849` |
| Functional run-time validation | Requires `responses` keys to match currently-pending request_ids; rejects stale replay | `python/packages/core/agent_framework/_workflows/_functional.py:814-831` |
| Functional checkpoint save | `_save_checkpoint` persists `state`, `_step_cache`, `_step_cache_auto_request_info_counts`, `_original_message`, and `pending_request_info_events` | `python/packages/core/agent_framework/_workflows/_functional.py:1124-1142` |
| Per-step checkpoint hook | `_on_step_completed` closure is invoked after each `@step` completes to chain checkpoints | `python/packages/core/agent_framework/_workflows/_functional.py:956-964` |
| Functional restore | `_run_core` imports cached step results, auto-request-info counts, user state, pending requests, and original message from a checkpoint | `python/packages/core/agent_framework/_workflows/_functional.py:912-954` |
| Response-only replay state | When no checkpoint, last-run state is kept in `self._last_message`, `self._last_step_cache`, `self._last_pending_request_ids` | `python/packages/core/agent_framework/_workflows/_functional.py:687-695, 941-946` |
| Functional interruption flow | `WorkflowInterrupted` caught at top of `_run_core`; events emitted, then status `IDLE_WITH_PENDING_REQUESTS` | `python/packages/core/agent_framework/_workflows/_functional.py:1027-1049` |
| Cross-workflow HITL | `WorkflowExecutor` propagates sub-workflow request_info events to the parent context | `python/packages/core/agent_framework/_workflows/_workflow_executor.py:256, 300-301, 354-356, 524-606` |
| Cross-workflow checkpoint limitation | `TODO(@taochen): Issue #1614 - how to handle the case when the parent workflow has checkpointing` | `python/packages/core/agent_framework/_workflows/_workflow_executor.py:525-526` |
| `WorkflowExecutor` checkpoint hooks | `on_checkpoint_save` serializes per-execution `execution_contexts`; `on_checkpoint_restore` rebuilds them from the checkpoint | `python/packages/core/agent_framework/_workflows/_workflow_executor.py:474-497` |
| Executor checkpoint hooks | `Executor.on_checkpoint_save` (returns dict) and `on_checkpoint_restore` (applies dict) overridable for custom state | `python/packages/core/agent_framework/_workflows/_executor.py:493-517` |
| WorkflowBuilder checkpoint plumbing | `WorkflowBuilder(checkpoint_storage=...)` is forwarded to `InProcRunnerContext` | `python/packages/core/agent_framework/_workflows/_workflow_builder.py:96-150, 861` |
| Runtime checkpoint override | `set_runtime_checkpoint_storage` overrides the build-time storage for one `run()` call | `python/packages/core/agent_framework/_workflows/_runner_context.py:166-176, 351-368` |
| Checkpoint encoding safety | `decode_checkpoint_value` uses a safe type allowlist; pickle markers restricted via `allowed_checkpoint_types` | `python/packages/core/agent_framework/_workflows/_checkpoint_encoding.py:72-148` |
| Checkpoint atomic file write | `FileCheckpointStorage.save` writes to `*.json.tmp` then `os.replace` for atomicity | `python/packages/core/agent_framework/_checkpoint.py:317-321` |
| Checkpoint exception type | `WorkflowCheckpointException` for all checkpoint failures | `python/packages/core/agent_framework/exceptions.py` and `_checkpoint.py:111-116, 211, 272, 299-300` |
| Functional signature hash | `_compute_signature_hash` mixes step names + `__code__.co_code`/`co_names` digest; raises on restore if different | `python/packages/core/agent_framework/_workflows/_functional.py:1144+` |
| Durable orchestration HITL | `ctx.wait_for_external_event(request_id)` blocks orchestration; sanitization re-checks before consuming | `python/packages/durabletask/agent_framework_durabletask/_workflows/orchestrator.py:840-900` |
| Durable workflow client | `DurableWorkflowClient.get_pending_hitl_requests` / `send_hitl_response` / `await_workflow_output` | `python/packages/durabletask/agent_framework_durabletask/_workflows/client.py:243-310` |
| .NET durable HITL mirror | `DurableExecutorDispatcher.ExecuteRequestPortAsync` uses `context.WaitForExternalEvent<string>(eventName)` | `dotnet/src/Microsoft.Agents.AI.DurableTask/Workflows/DurableExecutorDispatcher.cs:104-130` |
| .NET resume API | `InProcessExecution.ResumeAsync(Workflow, CheckpointInfo, CheckpointManager)` and `ResumeStreamingAsync` | `dotnet/src/Microsoft.Agents.AI.Workflows/InProcessExecution.cs:53-74` |
| .NET checkpoint manager | `InMemoryCheckpointManager`, `JsonCheckpointManager`, `FileSystemJsonCheckpointStore` | `dotnet/src/Microsoft.Agents.AI.Workflows/Checkpointing/InMemoryCheckpointManager.cs`, `JsonCheckpointManager.cs`, `FileSystemJsonCheckpointStore.cs` |
| .NET request port | `RequestPort` / `RequestInfoEvent` and `_pendingRequestInfoEvents` | `dotnet/src/Microsoft.Agents.AI.Workflows/WorkflowSession.cs:42-58` |
| .NET WorkflowSession resume | `CreateOrResumeRunAsync` branches on `LastCheckpoint` to call `ResumeStreamingInternalAsync` | `dotnet/src/Microsoft.Agents.AI.Workflows/WorkflowSession.cs:175-204` |
| .NET halt request | `DurableHaltRequestedEvent` and `DurableWorkflowContext.HaltRequested` permit explicit workflow halt | `dotnet/src/Microsoft.Agents.AI.DurableTask/Workflows/DurableHaltRequestedEvent.cs:10-16`, `DurableWorkflowContext.cs:65, 125-126` |
| Approval tests | `test_approval_workflow` / `test_workflow_state_with_pending_requests` / `test_denied_approval_workflow` round-trip the pause/resume contract | `python/packages/core/tests/workflow/test_request_info_and_response.py:177-309` |
| Checkpoint tests | `test_resume_succeeds_when_graph_matches` / `test_resume_fails_when_graph_mismatch` / sub-workflow variants | `python/packages/core/tests/workflow/test_checkpoint_validation.py:40-168` |
| Checkpoint rehydration test | `test_checkpoint_with_pending_request_info_events` proves end-to-end pause+restart via file storage | `python/packages/core/tests/workflow/test_request_info_event_rehydrate.py:127-220` |
| Crash-recovery test | `test_per_step_checkpoint_enables_crash_recovery` proves per-step cache + restore skips re-execution of step1 | `python/packages/core/tests/workflow/test_functional_workflow.py:592-635` |
| HITL-only replay | `test_request_info_resume` and `test_checkpoint_hitl_resume` | `python/packages/core/tests/workflow/test_functional_workflow.py:244-258, 543-561` |
| Durable HITL integration test | `test_09_dt_workflow_hitl.py` — durable worker pauses for human, sends response, awaits output | `python/packages/durabletask/tests/integration_tests/test_09_dt_workflow_hitl.py` |
| Single-agent orchestration HITL | `test_07_dt_single_agent_orchestration_hitl.py` covers immediate approval, iterative refinement, and timeout behavior | `python/packages/durabletask/tests/integration_tests/test_07_dt_single_agent_orchestration_hitl.py` |

## Answers to Dimension Questions

1. **Can execution pause safely?** Yes. Two safe pause mechanisms exist:
   - **In-process HITL**: `WorkflowContext.request_info` (`_workflow_context.py:393-424`) and
     `RunContext.request_info` (`_functional.py:193-250`) emit a `WorkflowEvent(type="request_info")`
     and a) for the graph API, leave the workflow in `WorkflowRunState.IDLE_WITH_PENDING_REQUESTS`
     (`_events.py:65`, `workflow.py:589-595`); b) for the functional API, raise
     `WorkflowInterrupted` (`_functional.py:83-99, 1027-1049`), which is caught by `_run_core` and
     persisted via the `state` and `_pending_requests` snapshot.
   - **Persisted pause**: every superstep boundary writes a `WorkflowCheckpoint` via
     `_create_checkpoint_if_enabled` (`_runner.py:212-238`). The first checkpoint before the
     superstep loop captures superstep-0 state (`_runner.py:92-97`).

2. **Can it resume after a crash?** Yes. `FileCheckpointStorage` writes JSON files atomically
   (`_checkpoint.py:317-321`) and reads them back through `decode_checkpoint_value`
   (`_checkpoint_encoding.py:165`). `Runner.restore_from_checkpoint` (`_runner.py:240-300`) clears
   the in-memory state, imports the checkpoint state, restores each executor via
   `on_checkpoint_restore`, applies the checkpoint to the runner context (restoring pending
   `request_info` events), and continues. The functional API additionally caches per-step results
   (`_functional.py:170-180, 320-340`) so resume does not re-execute completed steps —
   demonstrated by `test_per_step_checkpoint_enables_crash_recovery` at
   `test_functional_workflow.py:592-635` and the durable task integration test
   `test_09_dt_workflow_hitl.py`. The cross-process durable path also resumes after a
   task-hub restart because the orchestration history is persisted by the Durable Task Scheduler.

3. **Is the resume point deterministic?** Yes, with explicit guards:
   - **Graph-shape guard**: `Workflow._hash_graph_signature` (`workflow.py:1077-1080`) and the
     `_create_checkpoint_if_enabled` / `restore_from_checkpoint` pair
     (`_runner.py:274-279`) reject any restore where the graph topology has changed. Tested by
     `test_resume_fails_when_graph_mismatch` (`test_checkpoint_validation.py:40-62`) and
     `test_resume_fails_when_sub_workflow_changes` (`test_checkpoint_validation.py:147-167`).
   - **Functional guard**: `_compute_signature_hash` (`_functional.py:1144+`) mixes
     `__code__.co_code` and `co_names` so that a workflow body edit invalidates
     checkpoints.
   - **Functional auto-id determinism**: `RunContext.request_info` derives a deterministic
     `auto::<index>` request_id so resume works without echoing an explicit id
     (`_functional.py:229-234`).

4. **What happens if the world changed while paused?** Two distinct answers, depending on which
   pause layer is in use:
   - **In-process HITL** (graph API): no automatic re-check. Pending `request_info` events
     remain in `InProcRunnerContext._pending_request_info_events` (`_runner_context.py:292`)
     and the caller decides how to handle them. The response is type-validated
     (`_runner_context.py:469-473`) but the underlying request payload is *not* automatically
     re-validated against current external state. Sub-workflow pending requests are
     re-emitted on restore (see `test_request_info_event_rehydrate.py:223-303`), so stale
     external state can be picked up if the workflow is re-run from the same checkpoint.
   - **Checkpointed pause**: on restore, the runner imports the saved state at superstep
     boundary and re-runs from that point. External resources (DB rows, file paths, etc.)
     that the original step touched are *not* re-fetched unless the executor explicitly
     does so on `on_checkpoint_restore` (`_executor.py:508-517`). This is the standard
     durability contract: the *internal* state is replayed exactly; the *external* world
     is the executor's responsibility.

5. **Can multiple people or systems resume the same run?** Yes for HITL, with caveats.
   - The pending `request_info` event is keyed by `request_id` (`_runner_context.py:454,
     _workflow_context.py:393-408`). Any caller that knows a pending `request_id` can call
     `Workflow.run(responses={request_id: value})` and the validation in
     `_send_responses_internal` (`workflow.py:936-960`) checks that the id matches an
     actually-pending request before routing.
   - The framework serializes per-workflow-instance: `_is_running` blocks concurrent
     `run()` calls on the same instance (`workflow.py:379-383`), so a race between two
     callers attempting to answer the same `request_id` is funneled through the event loop
     and the first response wins (the second call would either find the request gone
     and raise `ValueError("No pending requests found")` or fail validation). There is
     **no** explicit "first-responder-wins" abstraction; concurrency safety comes from
     the event loop serialization, not from a multi-party lock.
   - For checkpoints, the storage is shared. Multiple processes can load the same
     `checkpoint_id`, but only the first to start a run will make progress; the second
     process will see a different `_resumed_from_checkpoint`/iteration state when
     restoring. There is no API to *fork* a checkpoint into two independent runs.
   - The .NET parallel uses `TaskOrchestrationContext.WaitForExternalEvent` and the
     Durable Task Scheduler's external-event routing (`DurableExecutorDispatcher.cs:121`),
     which is also keyed by event name; the same first-responder semantics apply.

## Architectural Decisions

- **Pregel superstep loop with explicit state commit at every boundary**
  (`_runner.py:78-156`) is the unit of durability. A "superstep" is the visible unit of
  progress, and the framework only ever writes a checkpoint at a superstep boundary, so
  replay determinism is enforced by the architecture rather than by ad-hoc calls.
- **Layered pause model**: in-memory HITL → file/in-memory checkpoints → Durable Task
  Scheduler. Each layer is an explicit, separately-tested mechanism
  (`test_request_info_and_response.py`, `test_checkpoint_validation.py`,
  `test_09_dt_workflow_hitl.py`).
- **Graph signature is the schema validator** (`_workflow.py:1015-1080`). Resume refuses to
  silently re-bind a checkpoint to a different workflow topology. This is the analog of a
  database schema check: cheap to compute, strict to apply.
- **`request_id` as the universal correlation key**. Whether a `request_info` event
  originated from a graph executor or a functional `@step`, the id is what lets a caller
  resume. The functional API supplies a deterministic fallback (`auto::<index>`,
  `_functional.py:229-234`) so explicit ids are optional.
- **`WorkflowInterrupted` is a `BaseException`** (`_functional.py:87-99`). The framework
  explicitly chose not to let user `except Exception:` blocks swallow the interrupt signal —
  this is a deliberate authoring contract.
- **Per-step caching + per-step checkpointing** for the functional API
  (`_functional.py:170, 956-964`). A workflow `@step` that has already completed is
  replayed from cache on resume, so resumption is *fast* (no re-execution) and
  *idempotent* (the same inputs return the same cached output).
- **State + messages + pending request events are the three checkpoint pillars**
  (`_checkpoint.py:78-81`). The runner explicitly comments that only *committed* state
  is serialized (`_runner.py:222-224`), so uncommitted mutations at the time of crash
  are not partially captured.

## Notable Patterns

- **Single, typed event object (`WorkflowEvent`)** with a string `type` discriminator
  (`_events.py:104-130, 146-280`). Frameworks that model pause/resume as a separate event
  hierarchy tend to grow special cases; using one class with semantic types keeps the
  rehydration code path uniform.
- **`CheckpointStorage` Protocol with two reference implementations** (`_checkpoint.py:119-450`).
  The Protocol keeps the door open for third-party backends (Cosmos, Blob) without
  changing the runner; the file backend includes a path-traversal check
  (`_checkpoint.py:282-300`) and an `allowed_checkpoint_types` allowlist for the pickle
  deserialization layer (`_checkpoint.py:266-279, _checkpoint_encoding.py:148-194`).
- **Runtime checkpoint override**: `set_runtime_checkpoint_storage`
  (`_runner_context.py:166-176, 351-368`) lets a caller pass a *different* `CheckpointStorage`
  per `run()` call. This is the bridge between "build-time" and "per-run" storage and is
  what enables injecting a remote storage handle without rebuilding the workflow.
- **`auto::<index>` deterministic request_id** for the functional API
  (`_functional.py:229-234`). Mirrors the same determinism contract as `@step` caching.
- **State keys that start with `_` are framework-reserved** (`_functional.py:283-298`).
  The framework writes its own bookkeeping (`_step_cache`, `_original_message`) into
  the same dict the user writes into, but the leading-underscore rule prevents user code
  from clobbering framework state and silently losing it on checkpoint restore.
- **`DurableHaltRequestedEvent`** in the .NET parallel
  (`DurableHaltRequestedEvent.cs:10-16` and `DurableWorkflowContext.cs:65, 125-126`)
  is an explicit *halt* primitive — different from a HITL pause in that it does not
  expect a response. The Python surface does not yet expose an equivalent
  `ctx.halt()` API; pause is exclusively HITL-shaped today.

## Tradeoffs

- **Pausing is HITL-shaped**: the only in-process pause mechanism is `request_info`. There
  is no plain "sleep until timestamp" or "wait for a callback" primitive for the
  graph API — to pause without external input, you must use the checkpoint mechanism.
  This is intentional (a pause without an expected response has no obvious
  deterministic resume) but it does limit simple "wait for a queue" patterns to the
  durable-task host (`DurableExecutorDispatcher.cs:104-130`).
- **`Workflow._state` is shared across `run()` calls on the same instance**
  (`workflow.py:541-547`). This is a deliberate authoring choice for multi-turn
  conversation, but it means *if* a caller reuses a `Workflow` instance after a
  successful run, state from the prior run is still in memory until the next
  `checkpoint_id`-based restore explicitly clears it (`_runner.py:286-287`).
- **Auto request_id determinism only works inside a single workflow function**:
  the `auto::<index>` counter (`_functional.py:229-234`) is per-`RunContext`
  instance, not global. Reusing the same workflow function for two concurrent
  workflows with HITL will give each its own counter — that is correct, but it
  is worth knowing that the id is *only* stable across replay of the same
  run, not across separate invocations.
- **Cross-workflow checkpoint restore has a known limitation**:
  `WorkflowExecutor` sub-workflow checkpoints are persisted but the framework
  flags `TODO(@taochen): Issue #1614` for restoring the *parent* workflow
  from a checkpoint that embeds sub-workflow state
  (`_workflow_executor.py:525-526`). The unit test
  `test_sub_workflow_checkpoint_restore_no_duplicate_requests` works around the
  current gap by re-emitting requests instead of restoring them.
- **File checkpointing uses pickle for opaque payloads** (`_checkpoint_encoding.py:148-194`)
  and relies on an explicit `allowed_checkpoint_types` allowlist. This is a
  deliberate opt-in for user dataclasses, but it does mean that *unlisted*
  dataclasses are silently rejected on load — a feature, not a bug, but a
  sharp edge.
- **No multi-party resume arbitration primitive.** A pending `request_info` can be
  answered by any process holding the id; the framework relies on the event loop
  serializing per-instance and the storage layer for cross-process. This is
  adequate for most HITL patterns (UI button, approval workflow) but is not a
  hard lock — a long-running pause without a single-source-of-truth approver
  can race.

## Failure Modes / Edge Cases

- **In-flight executor messages left behind by a crash are not automatically drained.**
  Calling `Workflow.run(message=...)` after a crash without `checkpoint_id` raises
  `RuntimeError` with a hint to resume from a checkpoint
  (`workflow.py:803-810`). This is correct behavior but is a hard error — there is
  no implicit recovery path.
- **Graph-shape change between checkpoint and restore** is rejected with
  `WorkflowCheckpointException("Workflow graph has changed")`
  (`_runner.py:274-279`). The framework will not "auto-adapt" — the caller must
  rebuild the original workflow before resuming.
- **State-key prefix collision**: user code that writes into `state["_foo"]` is
  rejected (`_functional.py:295-300`) precisely because such keys would be
  silently clobbered by framework bookkeeping and dropped on checkpoint
  restore. This is a constraint, not a failure mode.
- **HITL request that arrives after timeout or process death**: pending
  `request_info` events are stored in the checkpoint and re-emitted on
  restore (`_runner_context.py:414-426`), so a downstream caller that lost
  its memory can still answer by querying
  `Workflow.get_request_info_events()`. The durable-task host escalates this
  further: a stale external event that doesn't match any current pending
  request is sanitized and the request stays open
  (`orchestrator.py:860-892`).
- **Stale `_state` after restore**: because `Workflow._state` is shared
  across `run()` calls, a caller that mutates state in run A and then runs
  the workflow with `checkpoint_id=` from a prior checkpoint that does not
  have those mutations must remember that `restore_from_checkpoint` calls
  `self._state.clear()` first (`_runner.py:286-287`) to avoid leakage.
- **Process restart during per-step checkpoint write**: the
  `FileCheckpointStorage.save` is atomic via `tmp + os.replace`
  (`_checkpoint.py:317-321`), so a partial write is impossible; the worst
  case is a lost final checkpoint.
- **Durable task HITL: pickle-marker injection through bypassed client
  sanitization** is caught and the request is not consumed
  (`orchestrator.py:881-892`). The pending request is re-armed and a
  different response can be retried.

## Future Considerations

- **Explicit `halt()` primitive on the Python side** to mirror the .NET
  `DurableHaltRequestedEvent`. Useful for "cancel from the outside without
  waiting for an answer" workflows.
- **Sub-workflow checkpoint restoration across `WorkflowExecutor` boundary**
  (open issue #1614, `_workflow_executor.py:525-526`). Today the framework
  can checkpoint a sub-workflow, but restoring the parent from that
  checkpoint is not fully supported.
- **Multi-party resume arbitration**: a "claim this pending request_id"
  primitive so a manager can hand a `request_id` to a specific approver and
  the framework rejects other approvers. Not present today.
- **First-class Cosmos / Azure Blob / Redis `CheckpointStorage`
  implementations** (the durable-task package already exists for
  Azure-hosted orchestrations; the pure core package has only
  in-memory and file backends).
- **Checkpoint TTL/garbage collection**: there is no built-in retention
  policy in `InMemoryCheckpointStorage` or `FileCheckpointStorage`. The
  chain structure (`previous_checkpoint_id`) supports deletion
  (`_checkpoint.py:217-223, 392-410`) but the caller is responsible for
  compaction.

## Questions / Gaps

- **What is the operational guarantee when the world changes during a multi-day
  HITL pause?** The framework rehydrates internal state and pending requests
  deterministically, but it does not call any executor "freshen" hook. Any
  external resource the executor consulted before pause is *not* automatically
  re-validated. The contract is: "the framework remembers what it knew; you
  remember what the world knows." Documented implicitly, would benefit from an
  explicit "`on_world_changed`" hook in `Executor` for callers that need to
  recompute.
- **How does the framework decide which request to answer first when multiple
  pending `request_info` events are answered in a single `responses={...}` call?**
  `Workflow._send_responses_internal` uses `asyncio.gather`
  (`workflow.py:957-960`), so order is the dict-iteration order. The
  functional API orders by `request_id` for validation but does not guarantee
  a deterministic execution order to the user. No test in
  `test_functional_workflow.py` exercises multi-`request_id` resume ordering
  explicitly.
- **Is there a documented way to *fork* a checkpoint into two independent
  runs?** No clear evidence found. The `previous_checkpoint_id` chain
  structure could in principle support a "create a new checkpoint chain
  branching from id X" API, but `CheckpointStorage` has no `fork` or
  `branch` method (`_checkpoint.py:119-189`). A caller wanting
  parallel exploration of a paused run must restore once, then manually
  re-pause.
- **What is the maximum size of a checkpoint payload?** No limit is
  documented. `FileCheckpointStorage` writes a single JSON file per
  checkpoint (`_checkpoint.py:317-321`), and `pending_request_info_events`
  are pickled in full (`_checkpoint_encoding.py:148-194`). Large state
  payloads (LLM context, embeddings) will linearly increase checkpoint
  size and save/load latency; no batching or sharding is implemented.
- **Pause semantics for the non-functional `@tool` approval flow** are
  different from workflow pause. The `ToolApprovalMiddleware`
  (`_harness/_tool_approval.py` per `core/AGENTS.md`) pauses a *single
  agent run* using session state, not a `WorkflowCheckpoint`. Resuming a
  paused tool approval is therefore tied to a `session_id` rather than a
  checkpoint_id, and survives a process restart only when the session
  itself is persisted. No test under `tests/workflow/` covers this
  cross-cutting interaction.

---

Generated by `01.05-pause-resume-and-interrupt-semantics` against `agent-framework`.

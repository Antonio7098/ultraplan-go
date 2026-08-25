# Source Analysis: agent-framework

## 02.08 Crash Recovery and Reconstruction

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (and .NET in `dotnet/`) — primary surface is Python under `python/packages/`. Core abstractions live in `python/packages/core/agent_framework/`. Crash recovery is split between the in-process workflow runner (`_workflows/_checkpoint.py`, `_runner.py`, `_runner_context.py`) and the Azure Durable Task integration (`python/packages/durabletask/`). |
| Analyzed | 2026-07-10 |

## Summary

Crash recovery in agent-framework is built around two complementary layers, both opt-in:

1. **In-process workflow checkpoints** capture workflow state at the end of each superstep and let a `Workflow` resume across process restarts via `Workflow.run(checkpoint_id=...)`. Checkpoint contents are versioned, graph-fingerprinted, and stored in pluggable `CheckpointStorage` backends (in-memory, file, Azure Cosmos DB).
2. **Durable Task integration** (`agent-framework-durabletask` and `agent-framework-azurefunctions`) wraps agents as Durable Entities and workflows as orchestrations, delegating durability to Azure's Durable Task Framework. Conversation state persists in a versioned schema (`DurableAgentState`, schema `1.1.0`), and HITL / agent invocations survive process restarts via the orchestration host's history replay.

Recovery is explicit, not implicit: there is no background reaper for orphaned in-flight runs in the in-process model. A workflow that crashes mid-iteration leaves state in `_state` and uncommitted messages in `_runner_context`; the next `run()` call on the same instance refuses a fresh `message=` until those drain and surfaces an explicit error: "Workflows that need to recover from a mid-run failure must use checkpointing; there is no in-process recovery path." (`python/packages/core/agent_framework/_workflows/_workflow.py:803-810`). When a checkpoint exists, restoration is automatic via `Workflow.run(checkpoint_id=..., checkpoint_storage=...)` (`python/packages/core/agent_framework/_workflows/_workflow.py:651-660`).

The reconstruction model is built on three primitives:

- **`WorkflowCheckpoint`** dataclass with `previous_checkpoint_id` chaining, `graph_signature_hash` validation, `messages`, `state`, `pending_request_info_events`, `iteration_count`, and an explicit `version` field (`python/packages/core/agent_framework/_workflows/_checkpoint.py:30-88`).
- **`Runner.restore_from_checkpoint()`** which validates the graph hash, clears stale shared state, imports the snapshot, replays executor state via `on_checkpoint_restore` hooks, then rehydrates the runner context and resumes the iteration counter (`python/packages/core/agent_framework/_workflows/_runner.py:240-300`).
- **Executor-level `on_checkpoint_save` / `on_checkpoint_restore` hooks** that let each executor serialize its own resumable state (`python/packages/core/agent_framework/_workflows/_executor.py:493-517`).

For the Durable Task path, reconstruction happens through orchestration history replay: `DurableTaskWorkflowContext.is_replaying` (`python/packages/durabletask/agent_framework_durabletask/_workflows/dt_context.py:44-46`) suppresses side effects during replay and `run_workflow_orchestrator` (`python/packages/durabletask/agent_framework_durabletask/_workflows/orchestrator.py:689-911`) re-issues agent and activity tasks, accumulating outputs and waiting for HITL responses via `ctx.wait_for_external_event` with a per-request re-entry loop (`orchestrator.py:857-900`).

Operator visibility is reasonable but not rich. The `Workflow.status` property mirrors the most recent emitted `WorkflowRunState` (`STARTED`, `IN_PROGRESS`, `IN_PROGRESS_PENDING_REQUESTS`, `IDLE`, `IDLE_WITH_PENDING_REQUESTS`, `FAILED`, `CANCELLED`) (`python/packages/core/agent_framework/_workflows/_events.py:58-67`, `_workflow.py:368-377`). The durable host exposes runtime status and live custom status via `DurableWorkflowClient.get_runtime_status` / `stream_workflow` (`python/packages/durabletask/agent_framework_durabletask/_workflows/client.py:159-241`).

Idempotency is partial. The runner marks itself resumed (`_resumed_from_checkpoint = True`) and skips the initial "superstep 0" checkpoint on resume (`python/packages/core/agent_framework/_workflows/_runner.py:96, 155-156, 359-365`). The `version` field on `WorkflowCheckpoint` lets storage backends refuse mismatched versions, and `graph_signature_hash` refuses resumes against a topology change (`_runner.py:275-279`, `_checkpoint.py:48-49`). However, application-level functions invoked before the last successful checkpoint may have already produced external side effects — there is no built-in at-least-once guarantee for tool calls or chat client invocations between checkpoints.

## Rating

**8 / 10 — Clear model with explicit interfaces, opt-in durability, and extensive test coverage; missing only automatic orphan-detection, dead-letter handling, and operator alerting.**

Justification: the primitives are well-defined (`WorkflowCheckpoint`, `CheckpointStorage` protocol, `on_checkpoint_save` / `on_checkpoint_restore` hooks) and exercised by ~1454-line `test_checkpoint.py` + ~168-line `test_checkpoint_validation.py` + dedicated runner tests. The Durable Task integration adds a second, host-managed durability path with `is_replaying`-aware orchestrator and `set_custom_status` for live observation. It loses points for: (a) the in-process model refusing fresh `message=` runs after a crash unless checkpointing is enabled — there is no automatic fallback (explicitly documented at `_workflow.py:808-809`); (b) no dead-letter queue or operator notification when restore fails or graphs mismatch; (c) the `Workflow.run(message=...)` while in-flight-messages-remain guard (`_workflow.py:803-810`) silently rejects recovery attempts without checkpointing.

## Evidence Collected

Every entry cites `path:line`. Where evidence spans multiple lines, the entry uses inclusive ranges.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Checkpoint dataclass with chaining, graph fingerprint, version, iteration count | `WorkflowCheckpoint` fields and docstring | `python/packages/core/agent_framework/_workflows/_checkpoint.py:30-88` |
| Pluggable storage protocol (save/load/list/delete/get_latest/list_ids) | `CheckpointStorage` Protocol class | `python/packages/core/agent_framework/_workflows/_checkpoint.py:119-189` |
| In-memory storage implementation | `InMemoryCheckpointStorage` | `python/packages/core/agent_framework/_workflows/_checkpoint.py:192-236` |
| File-based storage with atomic write (`.tmp` + `os.replace`), restricted pickle allowlist | `FileCheckpointStorage.__init__`, `_validate_file_path`, `save`, `_write_atomic` | `python/packages/core/agent_framework/_workflows/_checkpoint.py:239-326` |
| Path-traversal defense on checkpoint IDs | `_validate_file_path` resolves and rejects non-relative paths | `python/packages/core/agent_framework/_workflows/_checkpoint.py:282-300` |
| Corrupted-checkpoint tolerance | `list_checkpoints` catches and logs malformed files | `python/packages/core/agent_framework/_workflows/_checkpoint.py:386-389` |
| Hybrid JSON + pickle encoding (security model) | `RestrictedUnpickler`, allowlist, `allowed_checkpoint_types` opt-in | `python/packages/core/agent_framework/_workflows/_checkpoint_encoding.py:1-45` |
| Run-state taxonomy used by recovery logic | `WorkflowRunState` enum (STARTED, IN_PROGRESS, IDLE, FAILED, CANCELLED, ...) | `python/packages/core/agent_framework/_workflows/_events.py:58-67` |
| Runner checkpoint creation on superstep boundaries | `_create_checkpoint_if_enabled` | `python/packages/core/agent_framework/_workflows/_runner.py:212-238` |
| Resume-from-checkpoint with validation, clear+import of state, executor-state replay | `Runner.restore_from_checkpoint` | `python/packages/core/agent_framework/_workflows/_runner.py:240-300` |
| Graph-signature mismatch refusal on resume | `if self._graph_signature_hash != checkpoint.graph_signature_hash` | `python/packages/core/agent_framework/_workflows/_runner.py:275-279` |
| `_resumed_from_checkpoint` flag suppresses initial "superstep 0" checkpoint on resume | `reset_iteration_count`, `_mark_resumed`, post-run reset | `python/packages/core/agent_framework/_workflows/_runner.py:74-76, 96, 155-156, 359-365` |
| Cancellation propagates to in-flight iteration task to avoid orphaned work | `except asyncio.CancelledError` branch | `python/packages/core/agent_framework/_workflows/_runner.py:115-120` |
| Per-executor save/restore hooks (default no-op) | `Executor.on_checkpoint_save`, `on_checkpoint_restore` | `python/packages/core/agent_framework/_workflows/_executor.py:493-517` |
| AgentExecutor restores session, cache, full_conversation, pending requests | `AgentExecutor.on_checkpoint_save` / `on_checkpoint_restore` | `python/packages/core/agent_framework/_workflows/_agent_executor.py:320-367` |
| WorkflowExecutor restores nested execution_contexts and request_to_execution map | `WorkflowExecutor.on_checkpoint_save` / `on_checkpoint_restore` | `python/packages/core/agent_framework/_workflows/_workflow_executor.py:474-519` |
| RunnerContext protocol for messaging, events, checkpointing | `RunnerContext` Protocol | `python/packages/core/agent_framework/_workflows/_runner_context.py:97-275` |
| In-process context: pending request_info events persist with messages | `InProcRunnerContext.apply_checkpoint` | `python/packages/core/agent_framework/_workflows/_runner_context.py:414-426` |
| Workflow.run validates checkpoint_id-only call paths | `_validate_run_params` (message/responses/checkpoint_id mutex rules) | `python/packages/core/agent_framework/_workflows/_workflow.py:865-889` |
| Rejects fresh `message=` run when in-flight messages remain (recovery requires checkpoint) | Guard in `_run_core` | `python/packages/core/agent_framework/_workflows/_workflow.py:803-810` |
| Run-state machine wiring around resume (IDLE_WITH_PENDING_REQUESTS, IN_PROGRESS_PENDING_REQUESTS) | `_run_workflow_with_tracing` status emissions | `python/packages/core/agent_framework/_workflows/_workflow.py:481-628` |
| Custom exception class for checkpoint errors | `WorkflowCheckpointException` | `python/packages/core/agent_framework/exceptions.py:257-260` |
| Public status API for operators | `Workflow.status` property + status events | `python/packages/core/agent_framework/_workflows/_workflow.py:368-377` |
| Durable Task: host-managed recovery via orchestration replay | `DurableTaskWorkflowContext` adapter | `python/packages/durabletask/agent_framework_durabletask/_workflows/dt_context.py:30-107` |
| `is_replaying` flag prevents re-emitting side effects during orchestrator replay | `DurableTaskWorkflowContext.is_replaying`, `publish_live_status` guard | `python/packages/durabletask/agent_framework_durabletask/_workflows/dt_context.py:44-46`, `python/packages/durabletask/agent_framework_durabletask/_workflows/orchestrator.py:754-768` |
| Workflow orchestrator with HITL pause/resume via external events | `run_workflow_orchestrator` main loop and HITL block | `python/packages/durabletask/agent_framework_durabletask/_workflows/orchestrator.py:689-911` |
| Per-request HITL wait loop: rejects poisoned payloads without consuming the request | HITL `while True:` block with `strip_pickle_markers` | `python/packages/durabletask/agent_framework_durabletask/_workflows/orchestrator.py:857-900` |
| Durable agent conversation state schema (versioned) | `DurableAgentState.SCHEMA_VERSION = "1.1.0"`, `DurableAgentState.to_dict` / `from_dict` | `python/packages/durabletask/agent_framework_durabletask/_durable_agent_state.py:395-434` |
| Entity-backed state persistence (durable entities keep state across invocations) | `DurableTaskEntityStateProvider` (`self.set_state(state)`) | `python/packages/durabletask/agent_framework_durabletask/_entities.py:335-353` |
| Agent conversation history replayed into chat messages on next invocation | `AgentEntity.run` filters non-replayable reasoning, replays history | `python/packages/durabletask/agent_framework_durabletask/_entities.py:153-193, 195-208` |
| In-flight agent-execution exception captured as `error_state_response` and persisted | `AgentEntity.run` `except Exception` branch | `python/packages/durabletask/agent_framework_durabletask/_entities.py:177-193` |
| Sanitization of untrusted pickle markers at every external trust boundary | `strip_pickle_markers` + comment block | `python/packages/durabletask/agent_framework_durabletask/_workflows/serialization.py:79-107` |
| Initial workflow input sanitized against pickle-marker injection | `_coerce_initial_input` calls `strip_pickle_markers` | `python/packages/durabletask/agent_framework_durabletask/_workflows/orchestrator.py:500-533` |
| CapturingRunnerContext explicitly disables checkpointing (orchestrator owns state) | `create_checkpoint` raises `NotImplementedError` | `python/packages/durabletask/agent_framework_durabletask/_workflows/runner_context.py:90-105` |
| `WorkflowCheckpoint` schema validation tests | `test_resume_fails_when_graph_mismatch`, `test_resume_succeeds_when_graph_matches`, sub-workflow parity | `python/packages/core/tests/workflow/test_checkpoint_validation.py:40-168` |
| `_resumed_from_checkpoint` prevents initial checkpoint on resume | `test_runner_checkpoint_with_resumed_flag` | `python/packages/core/tests/workflow/test_runner.py:846-883` |
| Restore from external storage when context has no checkpointing | `test_runner_restore_from_checkpoint_with_external_storage` | `python/packages/core/tests/workflow/test_runner.py:589-624` |
| Restore rejects no-storage, missing-id, graph-mismatch, generic-exception cases | `test_runner_restore_from_checkpoint_*` (4 cases) | `python/packages/core/tests/workflow/test_runner.py:626-682` |
| Executor-state restore type validation (5 negative tests) | `test_runner_restore_executor_states_invalid_*` | `python/packages/core/tests/workflow/test_runner.py:685-764` |
| Sub-workflow resume does not duplicate RequestInfoEvents | `test_sub_workflow_checkpoint_restore_no_duplicate_requests` | `python/packages/core/tests/workflow/test_sub_workflow.py:574-619` |
| Atomic write semantics for file storage | `_write_atomic` uses `tmp_path` + `os.replace` | `python/packages/core/agent_framework/_workflows/_checkpoint.py:317-323` |
| Corrupted JSON checkpoint file is skipped during listing | `test_file_checkpoint_storage_corrupted_file` | `python/packages/core/tests/workflow/test_checkpoint.py:886-897` |
| Status transitions to FAILED after exception; surfaces to operator | `test_status_is_failed_after_failure`, `_build_failing_workflow` | `python/packages/core/tests/workflow/test_workflow_status.py:31-37, 95-99` |
| CosmosCheckpointStorage respects `allowed_checkpoint_types` allowlist | init signature and `__init__` docstring | `python/packages/azure-cosmos/agent_framework_azure_cosmos/_checkpoint_storage.py:46-71` |
| Cosmos allowlist enforcement tests | `test_init_accepts_allowed_checkpoint_types`, `_APP_STATE_TYPE_KEY` cases | `python/packages/azure-cosmos/tests/test_cosmos_checkpoint_storage.py:626-705` |
| Documented public Status API and InProcRunnerContext.reset_for_new_run clears in-flight messages on context (not on workflow instance) | `InProcRunnerContext.reset_for_new_run` | `python/packages/core/agent_framework/_workflows/_runner_context.py:403-412` |
| Workflow.run preserves in-flight messages across calls (multi-turn input) — explicit comment that this is intentional | `_run_workflow_with_tracing` comment block | `python/packages/core/agent_framework/_workflows/_workflow.py:537-544` |
| Azure Functions HITL endpoint scopes `raise_event` to the app's workflow orchestrator, sanitizes pickle markers | `send_hitl_response` HTTP trigger | `python/packages/azurefunctions/agent_framework_azurefunctions/_app.py:441-477` |
| File storage path-traversal defense test (Foundry Hosting) | "Reject path-traversal context IDs in checkpoint storage" | `python/CHANGELOG.md:177` |
| Restricted pickle by default with opt-in `allowed_checkpoint_types` allowlist | CHANGELOG entries | `python/CHANGELOG.md:308, 310, 339, 346, 705` |
| Sub-workflow structure captured in graph signature (rejects resume when child workflow topology changes) | `Workflow._compute_graph_signature` includes sub-workflow signature | `python/packages/core/agent_framework/_workflows/_workflow.py:1015-1075` |
| Cancellation cascade propagates from runner → iteration task → pending coroutines (no orphaned tasks) | `Runner.run_until_convergence` `CancelledError` branch | `python/packages/core/agent_framework/_workflows/_runner.py:115-120` |

## Answers to Dimension Questions

### 1. What happens to in-progress work after restart?

There is no implicit "resume from RAM" path. Behavior depends on whether checkpointing was enabled:

- **With checkpointing**: each superstep writes a `WorkflowCheckpoint` (end-of-superstep) plus an initial "superstep 0" capture (`python/packages/core/agent_framework/_workflows/_runner.py:96-97, 144`). After restart, `Workflow.run(checkpoint_id=..., checkpoint_storage=...)` calls `Runner.restore_from_checkpoint` (`_runner.py:240-300`), which clears `_state`, imports the snapshot (`_state.import_state`), invokes each executor's `on_checkpoint_restore` hook (`_runner.py:314-340`), rehydrates messages and pending request-info events (`_runner_context.py:414-426`), and resumes the iteration counter.
- **Without checkpointing**: the framework refuses to recover. `Workflow.run` rejects a fresh `message=` when in-flight executor messages remain from a prior run with an explicit error: "Workflows that need to recover from a mid-run failure must use checkpointing; there is no in-process recovery path." (`python/packages/core/agent_framework/_workflows/_workflow.py:803-810`). Operators must implement their own retry logic around `Workflow.run`.
- **Durable Task path**: the orchestrator is replayed by the host. `DurableTaskWorkflowContext.is_replaying` (`python/packages/durabletask/agent_framework_durabletask/_workflows/dt_context.py:44-46`) is consulted before publishing custom status (`orchestrator.py:754-768`); non-agent executors run inside durable activities whose result is captured and replayed (`orchestrator.py:801-807`, `_workflows/activity.py:89-183`); the orchestrator's `while` loop and HITL `while True:` waits (`orchestrator.py:775, 857-900`) are replayed until the orchestration reaches a deterministic state.

### 2. Are orphaned runs detected?

Not in the in-process model. There is no background reaper for `Workflow` instances that died mid-iteration. Detection is operator-driven: callers must observe the workflow `status` property (`_workflow.py:368-377`) or subscribe to status events to know a run left behind state. On the next call, `Workflow.run` actively detects orphaned in-flight messages via `_runner.context.has_messages()` and raises `_workflow.py:803-810`. The durable host detects orchestrations stuck in a non-terminal state via `DurableWorkflowClient.get_runtime_status` (`python/packages/durabletask/agent_framework_durabletask/_workflows/client.py:159-177`), but it does not auto-clean them — termination/cleanup is the host's responsibility.

### 3. Is recovery automatic?

Conditional yes. With checkpointing enabled, restoration is driven by the user passing `checkpoint_id` to `Workflow.run` (`_workflow.py:701-712`), so it is operator-initiated, not transparent. The durable path is fully automatic: the durable-task host re-runs the orchestrator against its history. The runner auto-skips the initial "superstep 0" checkpoint on resume via `_resumed_from_checkpoint` (`_runner.py:96, 359-365`), confirmed by `test_runner_checkpoint_with_resumed_flag` (`python/packages/core/tests/workflow/test_runner.py:846-883`).

### 4. Is recovery idempotent?

Partially. The framework guarantees the **state graph** is replayed deterministically: shared state is cleared before reimport (`_runner.py:286-287`), the iteration counter is restored (`_runner.py:293, 359-365`), and `apply_checkpoint` re-queues pending request_info events (`_runner_context.py:424-426`). The `version` field on `WorkflowCheckpoint` (`_checkpoint.py:88`) is intended for forward-compat checks but currently has no validation gate in `WorkflowCheckpoint.from_dict` (`_checkpoint.py:100-117`) — version mismatches are not rejected.

Application-level idempotency is the developer's responsibility. There is no built-in deduplication of tool calls or chat client invocations that may have completed before the crash but after the last checkpoint. Sub-workflow request_info rehydration has a regression guard that prevents duplicate RequestInfoEvents (`test_sub_workflow_checkpoint_restore_no_duplicate_requests`, `python/packages/core/tests/workflow/test_sub_workflow.py:574-619`).

### 5. Are users/operators notified?

Yes, with caveats. The runner emits `WorkflowEvent.failed` plus a `status=FAILED` event when an exception escapes an iteration (`_workflow.py:606-628`, `_events.py:267`). The `Workflow.status` property always reflects the most recent state (`_workflow.py:368-377`, `test_status_is_failed_after_failure` at `python/packages/core/tests/workflow/test_workflow_status.py:95-99`). The durable host publishes live state, pending_requests, and (where supported) an event timeline to the orchestration's custom status, surfaced through `DurableWorkflowClient.get_runtime_status` / `get_pending_hitl_requests` / `stream_workflow` (`python/packages/durabletask/agent_framework_durabletask/_workflows/client.py:159-241`). There is no built-in alerting (webhook, email, log alert) for failed recovery or graph-mismatch errors — operators must wire their own.

## Architectural Decisions

- **Checkpoint-at-superstep-boundary, not at every message**: `Runner._create_checkpoint_if_enabled` runs after `self._state.commit()` and `superstep_completed` is yielded (`_runner.py:140-146`). This trades durability granularity for write volume; between-checkpoint side effects are not replay-protected.
- **Two-layer durability**: in-process `CheckpointStorage` protocol + Durable Task host. The durable host's `CapturingRunnerContext.create_checkpoint` explicitly raises `NotImplementedError` because the orchestrator owns state (`python/packages/durabletask/agent_framework_durabletask/_workflows/runner_context.py:90-105`).
- **Graph-fingerprint validation on resume**: `Workflow._compute_graph_signature` and `_hash_graph_signature` (`_workflow.py:1015-1080`) capture executor class identities, sub-workflow signatures, edge topology, and edge conditions; `restore_from_checkpoint` rejects mismatches (`_runner.py:275-279`).
- **Default-deny pickle deserialization**: `FileCheckpointStorage` and `CosmosCheckpointStorage` accept an `allowed_checkpoint_types` allowlist (`_checkpoint.py:262-280`, `_checkpoint_encoding.py:7-45`); the durable host further sanitizes external payloads with `strip_pickle_markers` at every trust boundary (`orchestrator.py:500-533, 881-893`, `_app.py:467-469`).
- **Per-executor save/restore hooks**: `Executor.on_checkpoint_save` and `on_checkpoint_restore` (`_executor.py:493-517`) let each executor capture its own resumable state. Used by `AgentExecutor` (cache, session, full_conversation, pending requests — `_agent_executor.py:320-367`) and `WorkflowExecutor` (execution_contexts map, request_to_execution — `_workflow_executor.py:474-519`).
- **HITL via durable external events**: `run_workflow_orchestrator` uses `ctx.wait_for_external_event(request_id)` with a re-entry `while True:` (`orchestrator.py:857-900`) so a poisoned payload does not consume the request. Mirrors the in-process `Workflow.run(responses=...)` continuation (`_workflow.py:912-934`).
- **Multi-turn preservation**: `_run_workflow_with_tracing` deliberately does not clear `_state` or `_runner_context` between `run()` calls (`_workflow.py:537-544`), so a `WorkflowAgent` can deliver multi-turn input on the same instance.

## Notable Patterns

- **Chained checkpoints**: `previous_checkpoint_id` links checkpoints into a history chain (`_checkpoint.py:52, 75`), enabling lineage inspection and incremental pruning.
- **Atomic file writes**: `FileCheckpointStorage.save` writes to `<id>.json.tmp` then `os.replace` (`_checkpoint.py:317-323`) to avoid torn writes on crash.
- **Restricted unpickler**: defense-in-depth allowlist via `RestrictedUnpickler` (`_checkpoint_encoding.py`) plus explicit `strip_pickle_markers` at trust boundaries (orchestrator + activity + HTTP endpoints).
- **Status-event-as-truth-source**: `Workflow.status` is updated in lockstep with `WorkflowEvent.status(...)` emissions, with the explicit guarantee that callers observing a status event have already observed the property change (`_workflow.py:368-377`, `test_status_transitions_during_streaming_run` at `python/packages/core/tests/workflow/test_workflow_status.py:102-117`).
- **Replay-safe clock**: `DurableTaskWorkflowContext.current_utc_datetime` delegates to the host's replay-safe clock (`dt_context.py:55-56`); `is_replaying` guards side-effect publishing (`orchestrator.py:754-768`).
- **Cancelled status distinct from FAILED**: `WorkflowRunState.CANCELLED` is separate from `FAILED` (`_events.py:67`), enabling caller distinction.
- **Event draining on iteration exception**: the runner drains queued events before propagating exceptions (`_runner.py:122-136`) so failure-related events are surfaced to observers.

## Tradeoffs

- **Granularity vs volume**: superstep-boundary checkpoints mean side effects between the last checkpoint and a crash are lost; finer granularity (per-executor-call) would increase durability at the cost of write amplification.
- **Resume is opt-in, not transparent**: `Workflow.run(checkpoint_id=...)` requires explicit action; the framework refuses to silently attempt a fresh run after a crash (`_workflow.py:803-810`). This is safer (no surprise replays) but pushes orchestration onto the operator.
- **Two-track model**: in-process checkpoints and Durable Task are separate surfaces. A workflow built with `WorkflowBuilder` will not magically gain durability by being deployed to Azure Functions — `configure_workflow` is required (`python/packages/durabletask/agent_framework_durabletask/_worker.py:172-213`).
- **No dead-letter or failed-restore alerting**: `WorkflowCheckpointException` is raised but not translated into a state event or log alert; recovery failures require operator inspection (`_runner.py:296-300`).
- **Application-level idempotency is unowned**: tool calls, HTTP requests, and chat client invocations between checkpoints are not deduplicated; the framework provides state replay but not at-least-once execution guarantees for external effects.

## Failure Modes / Edge Cases

- **Topology change after checkpoint**: `restore_from_checkpoint` raises `WorkflowCheckpointException("Workflow graph has changed since the checkpoint was created")` (`_runner.py:275-279`). Sub-workflow signature is included in the graph fingerprint (`_workflow.py:1029-1033`) so child-workflow changes also trigger this guard.
- **Checkpoint file corruption**: `FileCheckpointStorage.list_checkpoints` and `list_checkpoint_ids` log a warning and skip malformed files (`_checkpoint.py:386-389, 446-448`); `load` raises `WorkflowCheckpointException` (`_checkpoint.py:343-344`). Tested by `test_file_checkpoint_storage_corrupted_file` (`python/packages/core/tests/workflow/test_checkpoint.py:886-897`).
- **Path-traversal via checkpoint ID**: `_validate_file_path` rejects IDs that resolve outside the storage directory (`_checkpoint.py:282-300`).
- **Executor-state corruption**: restore validates that the executor-state map is a `dict[str, dict[str, Any]]` with string keys (`_runner.py:316-340`); five negative tests at `python/packages/core/tests/workflow/test_runner.py:685-764`.
- **Restore with no storage configured**: `_execute_with_message_or_checkpoint` raises `ValueError("Cannot restore from checkpoint...")` if neither runtime nor build-time checkpointing is available (`_workflow.py:651-660`); also tested at `python/packages/core/tests/workflow/test_workflow.py:284-322`.
- **Pickle RCE in restored checkpoints**: `RestrictedUnpickler` (`_checkpoint_encoding.py`) is the allowlist defense; `CosmosCheckpointStorage` enables it by default per CHANGELOG entry `[#5200]`. `strip_pickle_markers` at every HTTP/orchestrator boundary prevents untrusted payloads from reaching `pickle.loads` (`orchestrator.py:500-533, 881-893`, `_app.py:467-469`).
- **Cancellation without cleanup**: `Runner.run_until_convergence` cancels the iteration task and awaits it via `contextlib.suppress(asyncio.CancelledError)` (`_runner.py:115-120`), preventing orphaned tasks but leaving state in `_state` until the next `run()` call.
- **HITL payload poisoning**: orchestrator's HITL loop sanitizes with `strip_pickle_markers` and `continue`s without consuming the request (`orchestrator.py:881-893`).
- **Agent execution failure captured as state, not crash**: `AgentEntity.run` records the error as a `DurableAgentStateResponse(is_error=True)` and persists (`_entities.py:177-193`), so the entity keeps responding to subsequent invocations. Tested by `test_run_agent_handles_agent_exception` etc. (`python/packages/durabletask/tests/test_durable_entities.py:520-567`).
- **`max_iterations` exhaustion**: `_runner.py:152-153` raises `WorkflowConvergenceException`; orchestrator's `while iteration < workflow.max_iterations` raises the same on max-out (`orchestrator.py:775, 909`).

## Future Considerations

- **Automatic orphan detection and reconciliation**: introduce a background reaper that detects `Workflow` instances with in-flight messages older than a threshold, or wire the durable host's runtime status into the in-process model.
- **Operator alerting hooks**: add an opt-in callback (`on_recovery_failure`, `on_status_change`) so failures (graph mismatch, restore error, FAILED state) can drive external alerts without subclassing.
- **Dead-letter storage**: provide a `CheckpointStorage` decorator or sibling API for capturing irrecoverable checkpoints (graph mismatch, missing executor) so operators can inspect them post-mortem.
- **At-least-once tool invocation**: optional integration with `BaseChatClient` / tool dispatch to dedupe by call_id across checkpoint boundaries.
- **Version-aware checkpoint migration**: `WorkflowCheckpoint.version` is set but not checked in `from_dict` (`_checkpoint.py:100-117`). Adding a `_checkpoint_version_supported(version)` gate would let operators migrate schema safely.
- **Idempotency keys for HITL responses**: durable-side `send_hitl_response` (`python/packages/durabletask/agent_framework_durabletask/_workflows/client.py:290-308`) already returns synchronously; adding at-most-once semantics would simplify client retries.
- **Per-iteration checkpoint opt-in**: let workflows with low iteration counts opt into per-iteration checkpointing for tighter durability.

## Questions / Gaps

- **Operator alert for graph-mismatch during restore?** No callback is registered for `WorkflowCheckpointException` (`_runner.py:296-300`). It bubbles to the caller of `Workflow.run` but is not translated into a `WorkflowEvent`.
- **Per-executor idempotency hook?** The framework captures executor state for replay but does not provide an `on_replay(previous_state)` hook for executors to know their prior state at replay time (e.g., to deduplicate tool calls).
- **Cross-workflow checkpoint lineage?** `WorkflowCheckpoint` carries `workflow_name` and `graph_signature_hash` but no link back to the `Workflow` instance ID (intentionally — `_checkpoint.py:38-41`). Without instance-ID tracking, post-mortem analysis requires correlating against the caller's logs.
- **What is the durability floor?** With checkpointing on, the worst-case loss is the work performed in the current superstep up to the last committed checkpoint. The framework does not document this SLA. The runner's "atomic" boundary is `self._state.commit()` followed by `_create_checkpoint_if_enabled` (`_runner.py:140-146`), but checkpoint-save failure is swallowed (`_runner.py:236-238`): `logger.warning("Failed to create checkpoint: %e")` and returns `None`. There is no built-in metric or alert for checkpoint-save failures.
- **Is `WorkflowCheckpoint.version` used as a compatibility gate?** Searched in `_checkpoint.py:100-117` and `_runner.py:240-300`: it is stored on save, copied on load, but never compared. Future versions may break round-trips silently.

---

Generated by `02.08-crash-recovery-and-reconstruction` against `agent-framework`.
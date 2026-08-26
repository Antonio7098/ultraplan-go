# Source Analysis: agent-framework

## Dimension 15.03: Shared State and Conflict Resolution

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | C#/.NET (`dotnet/src/Microsoft.Agents.AI.*`) and Python (`python/packages/*`); dual-language framework (Microsoft Agent Framework) |
| Analyzed | 2026-08-26 |

## Summary

Microsoft Agent Framework shares state between agents through three distinct layers, each with its own coordination model:

1. **Workflow shared state (Pregel-style superstep model).** Both languages implement a shared key/value state that executors (which include agent executors) read and write during workflow execution, with writes staged in a pending buffer and committed only at superstep boundaries. Python: `State` class (`python/packages/core/agent_framework/_workflows/_state.py:6-127`) committed by the runner (`python/packages/core/agent_framework/_workflows/_runner.py:162-163`). .NET: scoped `StateManager` keyed by `(ExecutorId, ScopeName)` with queued updates published at step boundaries (`dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateManager.cs:13-16`, `StateManager.cs:201-233`; publish called from `dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcessRunner.cs:347,360`). Conflict resolution here is explicitly **last-write-wins within a superstep** — documented in both implementations (`_state.py:36-41`; the .NET `WriteStateAsync` overwrites the queued update for a key, `StateManager.cs:180-188`).

2. **Orchestration-level shared conversation.** Group chat / handoff / magentic patterns share one conversation history object among participant agents. Python keeps a `_full_conversation` list inside the orchestrator executor (`python/packages/orchestrations/agent_framework_orchestrations/_base_group_chat_orchestrator.py:169-170`), serialized for checkpoints via a unified `OrchestrationState` dataclass (`python/packages/orchestrations/agent_framework_orchestrations/_orchestration_state.py:33-54`). The .NET handoff orchestration uses a mutex-protected `MultiPartyConversation` with integer bookmarks so an agent can collect only messages appended since its last turn (`dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/MultiPartyConversation.cs:11-69`, bookmarks consumed in `Specialized/HandoffAgentExecutor.cs:363-378`).

3. **Session stores and checkpoint storage.** Sessions are stored per opaque ID; in-memory reads return deep copies to prevent cross-continuation mutation (`python/packages/core/agent_framework/_sessions.py:1826-1841`), and the file-backed store uses atomic `os.replace` plus process-local striped locks while explicitly declaring cross-process writers to be last-writer-wins (`_sessions.py:1878-1898`, locks at `_sessions.py:1902-1905`).

Conflict *detection* is strongest where durability is at stake: the .NET Foundry checkpoint store implements genuine optimistic concurrency (ETag precondition checks, classifying 412/409 as "lost race", bounded retry with reread) so two instances committing checkpoints for the same session cannot lose entries (`dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/FoundryJsonCheckpointStore.cs:45-49, 220-258, 490-496, 536-553`). Inside a single run, conflicts are mostly prevented rather than detected: per-executor serialization locks (Python `Executor._execution_lock`, `python/packages/core/agent_framework/_workflows/_executor.py:202-244`), fan-in edge state locks (.NET `Execution/FanInEdgeState.cs:12,39-52`), and an explicit cross-run shareability contract that makes concurrent execution of non-thread-safe executors impossible by construction (`dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcessRunner.cs:47-51` throws if concurrent runs are requested but any executor lacks `declareCrossRunShareable`; gate defined at `dotnet/src/Microsoft.Agents.AI.Workflows/Workflow.cs:89-92`, `ExecutorInstanceBinding.cs:20`).

The answer to the dimension's core question — *"Can two agents update the same resource without corrupting it?"* — is **yes within one process/run** (locks + superstep commit discipline prevent corruption, though simultaneous writers to the same key silently lose all but the last write), and **partially across processes**: durable checkpoint indexes get optimistic-concurrency retries, but file-based session/checkpoint stores fall back to last-writer-wins.

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale against the rubric:

- **Clear model + explicit interfaces (7-8 band):** The superstep pending/committed state semantics are a precisely documented contract (`_state.py:14-22`), implemented symmetrically in Python and .NET, exposed through typed public APIs (`WorkflowContext.get_state/set_state`, `python/packages/core/agent_framework/_workflows/_workflow_context.py:436-442`; `IWorkflowContext.ReadStateAsync/QueueStateUpdateAsync`, `dotnet/src/Microsoft.Agents.AI.Workflows/IWorkflowContext.cs:68,139`). The cross-run shareability flag is an explicit, opt-in concurrency contract (`Executor.cs:178-199`).
- **Tests:** Superstep caching, failure isolation (`discard()` restores committed state), atomicity of commit, and export/import behavior all have dedicated unit tests (`python/packages/core/tests/workflow/test_state.py:56-303`); AG-UI shared-state round-trips have integration tests on both sides (`dotnet/tests/Microsoft.Agents.AI.Hosting.AGUI.AspNetCore.IntegrationTests/SharedStateTests.cs:31-238`; `python/packages/ag-ui/tests/ag_ui/golden/test_scenario_shared_state.py:53-91`).
- **Operational safeguards:** Per-executor locks, mutex-protected shared conversation, striped file locks with atomic replace, ETag-retried checkpoint indexing.
- **Why not 9-10:** Same-key concurrent writes resolve via silent last-write-wins with no detection, merge functions, versioning, or warning surfaced to users (`_state.py:37-41`); lost-race logging is Debug-level only (`FoundryJsonCheckpointStore.cs:245-254`); file-based stores have no cross-process conflict detection beyond LWW (`_sessions.py:1894-1896`); there is no dedicated conflict log or observable metric for overwritten writes. Not proven under adversarial multi-host scale in-repo.

## Evidence Collected

Every entry includes a file path with line numbers relative to the source root `studies/agent-harness-study/sources/agent-framework`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Shared state API (Python) | `State` class: pending buffer + committed dict, `set/get/has/delete/clear/commit/discard/export_state/import_state`; reserved `_`-prefixed keys | `python/packages/core/agent_framework/_workflows/_state.py:6-127` |
| Last-write-wins semantics (Python) | Docstring: "each executor's writes go to the same pending buffer. The last write for a given key wins when commit() is called... consistent with the .NET behavior" | `python/packages/core/agent_framework/_workflows/_state.py:36-42` |
| Commit at superstep boundary (Python) | Runner calls `self._state.commit()` after each superstep iteration; checkpoint created after commit | `python/packages/core/agent_framework/_workflows/_runner.py:160-168` |
| Concurrent delivery within superstep (Python) | `asyncio.gather` across edge runners/sources; order preserved per edge runner | `python/packages/core/agent_framework/_workflows/_runner.py:179-229` |
| Executor serialization lock (Python) | Per-executor `asyncio.Lock` created lazily under running loop; guarantees one-at-a-time processing per executor within a superstep | `python/packages/core/agent_framework/_workflows/_executor.py:202-212, 232-244` |
| User-facing state access (Python) | `WorkflowContext.get_state/set_state` delegating to shared `State` | `python/packages/core/agent_framework/_workflows/_workflow_context.py:436-442` |
| Shared state API (.NET) | `StateManager`: scopes keyed by `ScopeId(ExecutorId, ScopeName)`, queued `UpdateKey -> StateUpdate` dictionary | `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateManager.cs:13-16` |
| Queued update overwrite (.NET) | `WriteStateAsync` assigns `_queuedUpdates[stateKey] = StateUpdate.Update(...)` — later writer replaces earlier queued update for same key/scope | `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateManager.cs:180-188` |
| Publish-at-step-boundary (.NET) | `PublishUpdatesAsync` aggregates updates per scope and writes them after all receivers complete (`Task.WhenAll` in `RunSuperstepAsync`) | `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateManager.cs:201-233`; `dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcessRunner.cs:305-336, 347, 360` |
| Typed read-modify-write helper (.NET) | `InvokeWithStateAsync<TState>` extension: read → invoke → queue update (single logical transaction per call site) | `dotnet/src/Microsoft.Agents.AI.Workflows/IWorkflowContextExtensions.cs:26-60` |
| Type-mismatch detection (.NET) | Reading a key as wrong type throws `InvalidOperationException("State for key ... is not of type ...")` | `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateManager.cs:135-138` |
| Quiescent export guard (.NET) | `ExportStateAsync`/`ImportStateAsync` throw if queued updates exist — checkpoints never capture half-applied state | `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StateManager.cs:243-259` |
| Cross-run shareability contract (.NET) | `declareCrossRunShareable` opt-in; `SupportsConcurrentSharedExecution` derived from it; `Workflow.AllowConcurrent` requires ALL bindings shareable | `dotnet/src/Microsoft.Agents.AI.Workflows/Executor.cs:178-199`; `dotnet/src/Microsoft.Agents.AI.Workflows/ExecutorInstanceBinding.cs:20`; `dotnet/src/Microsoft.Agents.AI.Workflows/Workflow.cs:89-92` |
| Concurrent-run enforcement (.NET) | Constructor throws listing non-concurrent executors when `enableConcurrentRuns && !workflow.AllowConcurrent` | `dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcessRunner.cs:47-51` |
| Stale-cache safeguard (.NET) | `StatefulExecutor` skips its local `_stateCache` whenever `context.ConcurrentRunsEnabled` is true — every read/write hits shared state | `dotnet/src/Microsoft.Agents.AI.Workflows/StatefulExecutor.cs:60-76, 86-94, 108-143` |
| Mutex-protected shared conversation (.NET) | `MultiPartyConversation` with `_mutex`; `CloneHistory`, `CollectNewMessages(bookmark)`, `AddMessages` all lock; stale bookmark throws | `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/MultiPartyConversation.cs:11-69` |
| Handoff shared-state usage (.NET) | `HandoffAgentExecutor` holds `StateRef<HandoffSharedState>`, appends messages and snapshots conversation inside `InvokeWithStateAsync` closure; turn reentrancy rejected ("Cannot have multiple simultaneous conversations") | `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/HandoffAgentExecutor.cs:100, 262-321, 354-357` |
| Fan-in coordination lock (.NET) | `FanInEdgeState.ProcessMessage` serializes concurrent arrivals from parallel executor tasks under `_syncLock`; snapshot taken under lock for checkpoint safety | `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/FanInEdgeState.cs:35-76` |
| Orchestrator conversation state (Python) | `BaseGroupChatOrchestrator._full_conversation` single shared history; participant registry rejects duplicate/conflicting IDs | `python/packages/orchestrations/agent_framework_orchestrations/_base_group_chat_orchestrator.py:99-115, 169-170, 304-312` |
| Unified orchestration checkpoint state (Python) | `OrchestrationState` dataclass standardizes conversation/round/metadata persistence across GroupChat, Handoff, Magentic; save/restore hooks | `python/packages/orchestrations/agent_framework_orchestrations/_orchestration_state.py:33-93`; `_base_group_chat_orchestrator.py:545-595` |
| Magentic manager state (Python) | Manager persists ledger/cache via `on_checkpoint_save`/`on_checkpoint_restore` into orchestrator state | `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:505-509, 1287-1326` |
| Session store copy-on-read (Python) | `SessionStore.get` returns `copy.deepcopy(session)`; `set` deep-copies on write — callers cannot mutate the stored snapshot | `python/packages/core/agent_framework/_sessions.py:1826-1855` |
| File session store consistency (Python) | Atomic temp-file + `os.replace`; 64-stripe process-local `threading.Lock`s; docs declare cross-process writers use last-writer-wins and recommend OS-level locking for more | `python/packages/core/agent_framework/_sessions.py:1878-1905, 1996-2069` |
| Optimistic concurrency for checkpoints (.NET) | Index update loop: read index → append → `SetItemAsync(ifMatch: etag)`; 412/409 classified as lost race; up to `DefaultMaxIndexUpdateAttempts=8` retries with fresh read; final failure throws with attempt count | `dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/FoundryJsonCheckpointStore.cs:45-49, 69, 218-261, 490-496, 536-553` |
| Collision detection on checkpoint IDs (.NET) | Duplicate checkpoint id already present in index → `InvalidOperationException` (guards identifier reuse) | `dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/FoundryJsonCheckpointStore.cs:225-231` |
| Retention protects concurrent runs (.NET) | Checkpoint retrieval deletes superseded ancestors but retains sibling branches/later checkpoints "so another persisted or concurrent run cannot lose its live state" | `dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/FoundryJsonCheckpointStore.cs:38-43, 264-279` |
| Chat-history provider conflict policy (.NET) | Service-managed conversation id vs local `ChatHistoryProvider`: configurable warn/throw/clear resolution (`ClearOnChatHistoryProviderConflict` default true, `ThrowOn...` default true) with Warning log | `dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientAgent.cs:816-856`; `dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientAgentOptions.cs:87-105`; log helper `ChatClientLogMessages.cs:61-66` (`ChatClientAgentLogMessages.cs:61-66`) |
| AG-UI client-owned shared state | Thread state travels in `RunAgentInput.State`, agent emits full `STATE_SNAPSHOT`; round-trip verified (counter 1→2→3) | `dotnet/tests/Microsoft.Agents.AI.Hosting.AGUI.AspNetCore.IntegrationTests/SharedStateTests.cs:31-163`; Python emission path `python/packages/ag-ui/agent_framework_ag_ui/_agent_run.py:2313-2333, 2480-2500, 2586-2596` |
| Snapshot-mutation isolation test (Python) | `test_endpoint_provider_mutation_does_not_change_shared_state_snapshot` — mutating provider output must not alter emitted snapshot | `python/packages/ag-ui/tests/ag_ui/test_endpoint.py:976` |
| State failure-isolation tests (Python) | Failure before commit preserves committed state; commit atomic; repeated supersteps isolated; delete sentinel semantics | `python/packages/core/tests/workflow/test_state.py:210-267, 126-208` |
| Checkpoint storage (Python) | `FileCheckpointStorage.save` uses atomic `_write_atomic` path validation; safe-type allowlist for deserialization | `python/packages/core/agent_framework/_workflows/_checkpoint.py:249-338` |
| Executor checkpoint hooks (Python) | Runner saves/restores each executor's state into shared state under `EXECUTOR_STATE_KEY`; restore validates types and raises `WorkflowCheckpointException` on mismatch | `python/packages/core/agent_framework/_workflows/_runner.py:391-429` |
| Concurrent orchestration avoidance (Python) | Parallel agents each own isolated conversations; `_AggregateAgentConversations` merges deterministically in participant order — no shared mutable resource to conflict over | `python/packages/orchestrations/agent_framework_orchestrations/_concurrent.py:83-136, 55-81` |

## Answers to Dimension Questions

**1. What state is shared between agents?**

Four categories, all evidenced above:

- *Workflow key/value state* shared by all executors in a graph: untyped dict in Python (`State`, `_state.py:25-28`), typed-and-scoped entries in .NET (`StateManager` scopes `(ExecutorId, ScopeName)`, `StateManager.cs:15`). Orchestration patterns build on this: the .NET handoff pattern shares a `HandoffSharedState` object (conversation + per-agent autonomous-turn counters, `HandoffStartExecutor.cs:73-90`, `HandoffAgentExecutor.cs:300-315`) under a well-known key (`HandoffConstants.HandoffSharedStateKey`).
- *Shared conversation history*: Python group-chat/handoff/magentic keep one `_full_conversation` owned by the orchestrator executor (`_base_group_chat_orchestrator.py:170`); .NET handoff shares `MultiPartyConversation`.
- *Sessions*: per-session `AgentSession` snapshots behind `SessionStore`/`FileSessionStore` (`_sessions.py:1795-1868, 1872+`).
- *Durable artifacts*: workflow checkpoints (superstep-granular, containing committed shared state + executor states + edge data — `InProcessRunner.cs:341-369`, `python .../_checkpoint.py:31-127`) and AG-UI thread state owned by the client and round-tripped as full snapshots (`SharedStateTests.cs:39-59`).

**2. How are conflicts detected?**

- *Optimistic concurrency (durable layer, .NET only):* ETag `ifMatch` preconditions on the Foundry state store; HTTP-style 412/409 mapped to `IsLostRace` and retried (`FoundryJsonCheckpointStore.cs:235-257, 490-496`).
- *Structural guards:* wrong-type reads throw (`StateManager.cs:135-138`); stale conversation bookmarks throw (`MultiPartyConversation.cs:40-44`); exporting/importing checkpoint state with pending updates throws (`StateManager.cs:245-259`); duplicate participant IDs and colliding tool names raise at build time (`_base_group_chat_orchestrator.py:109`; `python/packages/orchestrations/agent_framework_orchestrations/_handoff.py:325-326`).
- *Configuration conflict:* service-managed history vs local history provider detected at runtime and resolved per policy (`ChatClientAgent.cs:827-852`).
- *Not detected:* two executors writing the same state key in the same superstep produce no error, warning, or trace — the collision is invisible by design (LWW).

**3. How are conflicts resolved?**

- **Prevention first:** per-executor serialization locks (Python `_executor.py:202-211`), fan-in message locks (.NET `FanInEdgeState.cs:39-52`), mutex around shared conversation (.NET `MultiPartyConversation.cs`), and the cross-run-shareability gate that refuses to enable concurrent runs unless every executor opts in (`InProcessRunner.cs:47-51`).
- **Last-write-wins:** within a superstep's pending buffer (both languages) and across processes for file-backed session/checkpoint stores (`_sessions.py:1878-1880`).
- **Retry-with-reread:** bounded (8 attempts) retry loop for checkpoint index updates, re-reading the index each attempt so concurrent committers converge without losing entries (`FoundryJsonCheckpointStore.cs:218-258`).
- **Policy-driven resolution:** warn/throw/clear options for chat-history-provider conflicts (`ChatClientAgentOptions.cs:87-105`).
- **Avoidance by architecture:** the concurrent orchestrator gives each agent an isolated conversation and merges outputs deterministically instead of sharing mutable state (`_concurrent.py:83-136`).

**4. Is shared state consistent?**

Within a run, yes in a well-defined sense: all executors observe the same *committed* state at superstep start; intra-superstep writes are visible to the writing executor through the pending buffer but not to other executors until commit (`_state.py:14-18, 45-60`; .NET queued-update read-through `StateManager.cs:119-139`). Failures roll back cleanly because `discard()` drops pending changes and tests verify committed-state preservation (`test_state.py:213-232`). Durability is consistent-by-construction: checkpoints are only exported when no updates are queued (`StateManager.cs:245-248`) and Python checkpoints are written post-commit (`_runner.py:162-166`). Across processes/hosts, consistency weakens to last-writer-wins except where the Foundry optimistic-concurrency store is used; the file stores document this limitation themselves (`_sessions.py:1894-1896`, `python/packages/core/agent_framework/_workflows/_checkpoint.py` has no inter-process locking).

## Architectural Decisions

1. **Pregel/BSP superstep model as the consistency boundary.** Both languages stage writes and commit atomically at superstep end (`_runner.py:162-163`; `InProcessRunner.cs:305-360`). This trades immediate cross-executor visibility for deterministic, restartable state transitions and makes checkpointing trivially consistent.

2. **Conflict prevention via construction over conflict resolution at runtime.** Rather than merging concurrent writes, the framework constrains *who may run concurrently*: executors must declare `declareCrossRunShareable`, and enabling concurrent runs on a workflow containing non-shareable executors is a hard constructor error (`InProcessRunner.cs:47-51`). Executors that hold instance state disable local caches under concurrent runs (`StatefulExecutor.cs:62-73, 114`).

3. **Explicit LWW as documented semantics, not an accident.** The Python docstring states the last-write-wins rule and asserts parity with .NET (`_state.py:36-42`) — a deliberate design decision aligned with the superstep execution model, accepting silent overwrite in exchange for simplicity.

4. **Optimistic concurrency pushed to the durable boundary.** In-process coordination uses cheap locks; the only place independent writers genuinely race (shared checkpoint index in Foundry storage) gets ETags + bounded retry (`FoundryJsonCheckpointStore.cs:45-49`), including retention logic that deliberately preserves sibling branches belonging to concurrent runs (`FoundryJsonCheckpointStore.cs:38-43`).

5. **Copy-on-read session snapshots.** `SessionStore.get` deep-copies (`_sessions.py:1840-1841`) so multiple continuations cannot alias one stored snapshot — corruption prevention at the API-contract level.

## Notable Patterns

- **Bookmark-based incremental consumption** of a shared conversation (`CollectNewMessages(bookmark)` returning new bookmark, `MultiPartyConversation.cs:36-48`): agents track their own position in shared history instead of copying it, with a hard error on stale bookmarks.
- **Read-modify-write closures** (`InvokeWithStateAsync<TState>(key, invocation)`): the .NET API shapes user code into short read→mutate→queue transactions (`IWorkflowContextExtensions.cs:26-60`), used pervasively by handoff/group-chat internals (`GroupChatHost.cs:135-150`, `MagenticOrchestrator.cs:361-401`).
- **Striped file locks** (64 lock stripes keyed per resolved file) to bound lock contention while serializing per-file operations (`_sessions.py:1901-1905, 2068-2069`).
- **Delete sentinel** for tombstoning keys in the pending buffer so deletes commit correctly (`_state.py:68-83, 121-127`), fully tested (`test_state.py:143-207`).
- **Unified orchestration state envelope** (`OrchestrationState`) giving GroupChat/Handoff/Magentic one checkpoint schema with a pattern-specific metadata escape hatch (`_orchestration_state.py:33-54`).

## Tradeoffs

- **Simplicity vs. write conflation:** LWW keeps the engine trivial but means two executors updating the same key in one superstep lose data silently; users must partition keys by convention — nothing enforces it.
- **Concurrency gating vs. flexibility:** requiring `declareCrossRunShareable` on *every* binding before concurrent runs are allowed (`Workflow.cs:89`) is safe but coarse — one conservative executor disables concurrency workflow-wide.
- **Snapshot-style AG-UI state vs. mergeability:** whole-snapshot STATE_SNAPSHOT round-trips are simple and testable but make concurrent frontends clobber each other's state (no delta/patch events observed in the studied code).
- **File-store portability vs. multi-host safety:** atomic `os.replace` + process-local locks give crash-safe single-process behavior, but the docs explicitly punt cross-process/host coordination to OS tooling (`_sessions.py:1894-1898`) — portable yet not multi-writer safe.
- **Debug-level lost-race logging:** quiet by default; operators get little signal that contention occurred until the terminal failure (`FoundryJsonCheckpointStore.cs:245-254, 260-261`).

## Failure Modes / Edge Cases

- **Silent overwrite (documented):** same-key concurrent writes within a superstep — last writer wins, earlier values unrecoverable (`_state.py:36-42`).
- **Stale executor cache under concurrency (guarded):** would occur if `StatefulExecutor` cached during concurrent runs; mitigated by skipping cache when `ConcurrentRunsEnabled` (`StatefulExecutor.cs:70-73`).
- **Partial writes:** prevented for sessions/checkpoints via temp-file + `os.replace` (`_sessions.py:1996-2018`; `_checkpoint.py:328` `_write_atomic`) and for in-memory checkpoint stores by snapshotting fan-in buffers under lock (`FanInEdgeState.cs:64-76`).
- **Lost index updates under contention (bounded):** exhausted retries raise a descriptive `InvalidOperationException` after 8 attempts rather than dropping the checkpoint item (the checkpoint itself was already stored under its own key, `FoundryJsonCheckpointStore.cs:240-261`).
- **Reentrant turns:** attempting a second conversation while a handoff turn is active throws ("Cannot have multiple simultaneous conversations", `HandoffAgentExecutor.cs:354-357`); requesting a handoff while holding pending requests also throws (`HandoffAgentExecutor.cs:257-260`).
- **Type drift in shared state:** reading a key as the wrong type surfaces immediately as `InvalidOperationException` instead of corrupting downstream logic (`StateManager.cs:135-138`); Python restore validates executor-state shape and fails with `WorkflowCheckpointException` (`_runner.py:409-419`).
- **Cross-loop executor reuse:** Python recreates the executor's `asyncio.Lock` when the event loop changes to avoid "bound to a different event loop" failures (`_executor.py:207-212, 232-244`).

## Future Considerations

- Add optional per-key conflict callbacks or warnings when multiple distinct executors queue writes to the same key in one superstep (the information exists in `_pending`/`_queuedUpdates` before commit).
- Expose lost-race retry metrics/counters (e.g., OTel counters alongside existing workflow events `superstep_started/completed`, `_events.py:118-121, 315-322`) so storage contention is observable in production.
- Consider ETag/version support in `FileSessionStore`/`FileCheckpointStorage` (or documenting a recommended advisory-lock pattern) for multi-process deployments, mirroring what `FoundryJsonCheckpointStore` already does.
- Generalize the bookmark pattern (`MultiPartyConversation`) into the public Python orchestrations, which currently rely on plain lists copied wholesale (`_base_group_chat_orchestrator.py:304-312`).

## Questions / Gaps

- **No evidence found** of any distributed lock service, lease, or consensus mechanism anywhere in the source tree; searches for `lock`/`conflict` in coordination contexts surfaced only in-process locks, file stripes, and the Foundry ETag path. Multi-host coordination is delegated to backend stores.
- **No evidence found** of merge functions, CRDTs, or per-key version vectors for workflow shared state; LWW appears to be the only resolution strategy for same-key writes (searched `merge`, `last.write`, `wins`, `version` across both trees).
- No dedicated persistent conflict log exists; conflict visibility is limited to Debug logs (lost races), Warning logs (history-provider conflict), and thrown exceptions.
- The Go SDK is out of tree (README points to `microsoft/agent-framework-go`, `README.md:30-31`), so its shared-state semantics could not be inspected here.
- Whether `enableConcurrentRuns` is exercised by hosted samples could not be confirmed within this source; the enforcement path itself is tested indirectly via constructor validation (`InProcessRunner.cs:47-51`), but a dedicated concurrency stress test for two runs mutating shared state was not located in the studied tests directories.

---

Generated by dimension `dimensions/15.03-shared-state-conflict-resolution.md` (Dimension 15.03: Shared State and Conflict Resolution) against `agent-framework`.

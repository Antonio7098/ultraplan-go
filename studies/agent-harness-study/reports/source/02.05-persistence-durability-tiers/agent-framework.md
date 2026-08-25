# Source Analysis: agent-framework

## 02.05: Persistence Durability Tiers

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | C# / .NET (Microsoft.Extensions.AI, DurableTask, Azure Functions, Cosmos DB, Valkey/Redis, Mem0, Vector stores) |
| Analyzed | 2026-07-10 |

## Summary

The framework offers a layered persistence model organized around three abstractions: `AgentSession` (in-process state bag per conversation), `ChatHistoryProvider` (per-session chat-history persistence with pluggable backends), and workflow `ICheckpointStore` / `TaskEntity` state (durable, replay-safe state for orchestrations and long-running agents). Durability tiers span (a) in-process state inside `AgentSessionStateBag` (`dotnet/src/Microsoft.Agents.AI.Abstractions/AgentSessionStateBag.cs:23`), (b) file-backed session file stores (`dotnet/src/Microsoft.Agents.AI/Harness/FileStore/FileSystemAgentFileStore.cs:33`), (c) in-memory workflow checkpoint manager (`dotnet/src/Microsoft.Agents.AI.Workflows/Checkpointing/InMemoryCheckpointManager.cs:13`), (d) file-system JSON checkpoint store (`dotnet/src/Microsoft.Agents.AI.Workflows/Checkpointing/FileSystemJsonCheckpointStore.cs:30`), (e) Cosmos DB NoSQL for both chat history and workflow checkpoints (`dotnet/src/Microsoft.Agents.AI.CosmosNoSql/CosmosChatHistoryProvider.cs:41`, `dotnet/src/Microsoft.Agents.AI.CosmosNoSql/CosmosCheckpointStore.cs:24`), (f) Valkey (Redis-compatible) list storage (`dotnet/src/Microsoft.Agents.AI.Valkey/ValkeyChatHistoryProvider.cs:42`), (g) vector-store backed memory via `Microsoft.Extensions.VectorData` (`dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProvider.cs:55`), (h) external Mem0 service (`dotnet/src/Microsoft.Agents.AI.Mem0/Mem0Provider.cs:47`), and (i) Durable Task entity/orchestration state for `Microsoft.Agents.AI.DurableTask` (`dotnet/src/Microsoft.Agents.AI.DurableTask/AgentEntity.cs:13`, `dotnet/src/Microsoft.Agents.AI.DurableTask/State/DurableAgentState.cs:11`).

Operators can tell which tier a given piece of state lives in: the framework separates per-session state from durable infrastructure state, exposes `SerializeSessionAsync`/`DeserializeSessionAsync` for `AgentSession` (so the caller chooses storage; nothing is auto-persisted), and `IConversationStorage` (`dotnet/src/Microsoft.Agents.AI.Hosting.OpenAI/Conversations/InMemoryConversationStorage.cs:19`) plus `IDistributedCache` (`dotnet/src/Microsoft.Agents.AI.Purview/CacheProvider.cs:25`) for derived packages. However, several components silently drop state if misconfigured (notably the default `InMemoryChatHistoryProvider` and the default `FileSystemAgentFileStore` rooted at `Path.Combine(Directory.GetCurrentDirectory(), "working")` in `dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgent.cs:316`), and there is no built-in auto-save of `AgentSession` to any store — callers must wire `SerializeSessionAsync` themselves. Durability is configurable for most backends (Cosmos TTL `MessageTtlSeconds`, Valkey `MaxMessages`, Durable Task TTL `DefaultTimeToLive` of 14 days, durable checkpoint interval).

## Rating

**6/10 — Present but inconsistent, with clear pluggable interfaces, but several silent defaults, no automatic session save, and heterogeneous durability semantics.**

The framework exposes well-named stores and concrete implementations covering the full durability spectrum (in-memory → file → Cosmos → Redis → Durable Entities). The abstractions are layered cleanly (`ChatHistoryProvider`, `ICheckpointStore`, `AgentFileStore`, `IDistributedCache`), explicit lifecycle hooks exist (TTL, schema versioning, hierarchical partitioning, transactional batch operations), and tests cover the major stores. However, durability is opt-in: `HarnessAgent` defaults to `InMemoryChatHistoryProvider` (`dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgent.cs:199`) and to per-CWD `FileSystemAgentFileStore` (`dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgent.cs:316`); `AgentSession` is never auto-persisted; the in-memory `ConversationStorage` is described as "not persisted across application restarts" (`dotnet/src/Microsoft.Agents.AI.Hosting.OpenAI/Conversations/InMemoryConversationStorage.cs:18`); Purview cache uses whichever `IDistributedCache` is registered but the default DI registration is not provided in this package; and the `HarnessAgent` itself is annotated `[Experimental]` (`dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgent.cs:83`), signaling that the most batteries-included durability story is still under active change.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Session state bag abstraction | `AgentSessionStateBag` keyed concurrent dictionary with JSON serialization | `dotnet/src/Microsoft.Agents.AI.Abstractions/AgentSessionStateBag.cs:21-40` |
| `AgentSession` serialization contract | `SerializeSessionAsync` / `DeserializeSessionAsync` on `AIAgent` | `dotnet/src/Microsoft.Agents.AI.Abstractions/AIAgent.cs:187,222` |
| In-memory chat history (silent no-op for restart) | `InMemoryChatHistoryProvider` stores messages only in `AgentSession.StateBag` | `dotnet/src/Microsoft.Agents.AI.Abstractions/InMemoryChatHistoryProvider.cs:27-134` |
| In-memory file store (silent no-op) | `InMemoryAgentFileStore` uses `ConcurrentDictionary` only | `dotnet/src/Microsoft.Agents.AI/Harness/FileStore/InMemoryAgentFileStore.cs:24-199` |
| File-system file store (machine-scoped) | `FileSystemAgentFileStore` writes to a configured root, no cross-process safety guarantees beyond file-share `None` not used | `dotnet/src/Microsoft.Agents.AI/Harness/FileStore/FileSystemAgentFileStore.cs:33-384` |
| Harness default cwd-relative store | `working` dir under `Directory.GetCurrentDirectory()` is default working dir | `dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgent.cs:316` |
| Harness default file-memory store | `{cwd}/agent-file-memory/{timestamp}_{guid}` per session | `dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgent.cs:300-309` |
| Harness default chat history | `InMemoryChatHistoryProvider` unless `ChatHistoryProvider` is supplied | `dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgent.cs:193-199` |
| Per-service-call persistence | `PerServiceCallChatHistoryPersistingChatClient` flushes after every inner service call, enabling crash recovery | `dotnet/src/Microsoft.Agents.AI/ChatClient/PerServiceCallChatHistoryPersistingChatClient.cs:57-358` |
| `RequirePerServiceCallChatHistoryPersistence` opt-in | `ChatClientAgentOptions` flag | `dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientAgentOptions.cs:152` |
| Compaction (destructive on reducer) | `InMemoryChatHistoryProviderOptions.ReducerTriggerEvent` and `ChatReducer` interface | `dotnet/src/Microsoft.Agents.AI.Abstractions/InMemoryChatHistoryProviderOptions.cs:25-36` |
| Compaction strategies | `SlidingWindowCompactionStrategy`, `SummarizationCompactionStrategy`, `ContextWindowCompactionStrategy`, `TruncationCompactionStrategy` | `dotnet/src/Microsoft.Agents.AI/Compaction/SlidingWindowCompactionStrategy.cs`, `dotnet/src/Microsoft.Agents.AI/Compaction/SummarizationCompactionStrategy.cs:37`, `dotnet/src/Microsoft.Agents.AI/Compaction/ContextWindowCompactionStrategy.cs`, `dotnet/src/Microsoft.Agents.AI/Compaction/TruncationCompactionStrategy.cs` |
| Cosmos DB chat history provider | `CosmosChatHistoryProvider` with hierarchical partitioning, transactional batch, TTL | `dotnet/src/Microsoft.Agents.AI.CosmosNoSql/CosmosChatHistoryProvider.cs:41-631` |
| Cosmos DB TTL config | `MessageTtlSeconds` default 86400s (24h) | `dotnet/src/Microsoft.Agents.AI.CosmosNoSql/CosmosChatHistoryProvider.cs:88` |
| Cosmos DB hierarchical partition keys | `BuildPartitionKey` supports tenantId/userId/conversationId | `dotnet/src/Microsoft.Agents.AI.CosmosNoSql/CosmosChatHistoryProvider.cs:197-215` |
| Cosmos DB checkpoint store | `CosmosCheckpointStore<T>` for workflow checkpoints | `dotnet/src/Microsoft.Agents.AI.CosmosNoSql/CosmosCheckpointStore.cs:24-251` |
| Cosmos DB partition key per session | `CosmosCheckpointDocument.Id = $"{sessionId}_{checkpointId}"` | `dotnet/src/Microsoft.Agents.AI.CosmosNoSql/CosmosCheckpointStore.cs:115-125` |
| Valkey (Redis-compatible) chat history | `ValkeyChatHistoryProvider` using `ListRightPushAsync` + `ListTrimAsync` | `dotnet/src/Microsoft.Agents.AI.Valkey/ValkeyChatHistoryProvider.cs:42-225` |
| Valkey TTL/retention policy | "Stored messages have no TTL and persist indefinitely" — caller responsibility | `dotnet/src/Microsoft.Agents.AI.Valkey/ValkeyChatHistoryProvider.cs:25-29` |
| Valkey key naming | `BuildKey` prefixes by `KeyPrefix` | `dotnet/src/Microsoft.Agents.AI.Valkey/ValkeyChatHistoryProvider.cs:72,203` |
| Vector-store chat memory | `ChatHistoryMemoryProvider` over `Microsoft.Extensions.VectorData` | `dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProvider.cs:55-538` |
| External Mem0 provider | HTTP client to Mem0 service for memory storage/search | `dotnet/src/Microsoft.Agents.AI.Mem0/Mem0Provider.cs:47-302` |
| In-memory workflow checkpoint manager | `InMemoryCheckpointManager` keyed by sessionId, process-lifetime | `dotnet/src/Microsoft.Agents.AI.Workflows/Checkpointing/InMemoryCheckpointManager.cs:13-66` |
| File-system JSON workflow checkpoints | `FileSystemJsonCheckpointStore` with exclusive file lock and JSONL index | `dotnet/src/Microsoft.Agents.AI.Workflows/Checkpointing/FileSystemJsonCheckpointStore.cs:30-213` |
| FileSystem store cross-process exclusion | Throws "already in use by another process" via `FileShare.None` | `dotnet/src/Microsoft.Agents.AI.Workflows/Checkpointing/FileSystemJsonCheckpointStore.cs:60-65` |
| Checkpoint factory | `CheckpointManager.CreateInMemory` / `CheckpointManager.CreateJson` / `CheckpointManager.Default` | `dotnet/src/Microsoft.Agents.AI.Workflows/CheckpointManager.cs:33-51` |
| Durable Task entity state | `TaskEntity<DurableAgentState>` managed by DTFx | `dotnet/src/Microsoft.Agents.AI.DurableTask/AgentEntity.cs:13` |
| Durable Task state schema | `DurableAgentState` carries `schemaVersion` (currently `1.1.0`) and `data` | `dotnet/src/Microsoft.Agents.AI.DurableTask/State/DurableAgentState.cs:11-27` |
| Durable Task state schema enforcement | `DurableAgentStateJsonConverter` rejects non-major-1 schemas | `dotnet/src/Microsoft.Agents.AI.DurableTask/State/DurableAgentStateJsonConverter.cs:38-41` |
| Durable Task conversation history | `DurableAgentStateData.ConversationHistory` is ordered list of request/response entries | `dotnet/src/Microsoft.Agents.AI.DurableTask/State/DurableAgentStateData.cs:11-31` |
| Durable Task TTL | `DefaultTimeToLive` defaults to 14 days, `MinimumTimeToLiveSignalDelay` 5 min | `dotnet/src/Microsoft.Agents.AI.DurableTask/DurableAgentsOptions.cs:25-53` |
| Durable Task TTL self-signal | `ScheduleDeletionCheck` uses `SignalEntity` with `SignalTime` | `dotnet/src/Microsoft.Agents.AI.DurableTask/AgentEntity.cs:197-215` |
| Durable Task TTL cleanup | `CheckAndDeleteIfExpired` clears `this.State = null!` | `dotnet/src/Microsoft.Agents.AI.DurableTask/AgentEntity.cs:166-195` |
| Durable Agent proxy | `DurableAIAgentProxy` plus `DurableAIAgent` for client/worker | `dotnet/src/Microsoft.Agents.AI.DurableTask/DurableAIAgent.cs:15-296`, `dotnet/src/Microsoft.Agents.AI.DurableTask/DurableAIAgentProxy.cs` |
| Durable Task client signalling | `DurableTaskClient.Entities.SignalEntityAsync` for cross-process invocation | `dotnet/src/Microsoft.Agents.AI.DurableTask/DefaultDurableAgentClient.cs:23-27` |
| Purview distributed cache | `CacheProvider` depends on caller-provided `IDistributedCache` | `dotnet/src/Microsoft.Agents.AI.Purview/CacheProvider.cs:15-89` |
| OpenAI hosting in-memory conversation storage | `InMemoryConversationStorage` `MemoryCache`-backed | `dotnet/src/Microsoft.Agents.AI.Hosting.OpenAI/Conversations/InMemoryConversationStorage.cs:19-348` |
| Conversation storage explicitly non-durable | "data is not persisted across application restarts" | `dotnet/src/Microsoft.Agents.AI.Hosting.OpenAI/Conversations/InMemoryConversationStorage.cs:18` |
| ToolApproval state persistence | Persisted in `AgentSessionStateBag` only via `ToolApprovalState` | `dotnet/src/Microsoft.Agents.AI/Harness/ToolApproval/ToolApprovalState.cs:16-53` |
| Todo state persistence | Persisted in `AgentSessionStateBag` only via `TodoState` | `dotnet/src/Microsoft.Agents.AI/Harness/Todo/TodoProvider.cs:80-83` |
| Agent-mode state persistence | `AgentModeState` in `AgentSessionStateBag` | `dotnet/src/Microsoft.Agents.AI/Harness/AgentMode/AgentModeProvider.cs` |
| File-memory state | `FileMemoryState.WorkingFolder` stored in `AgentSessionStateBag` only | `dotnet/src/Microsoft.Agents.AI/Harness/FileMemory/FileMemoryProvider.cs:89-92`, `dotnet/src/Microsoft.Agents.AI/Harness/FileMemory/FileMemoryState.cs` |
| Workflow Session → checkpoint manager | `WorkflowSession.EnsureExternalizedInMemoryCheckpointing` constructs `CheckpointManager` from store | `dotnet/src/Microsoft.Agents.AI.Workflows/WorkflowSession.cs:81,113` |
| InProc execution checkpoint wiring | `InProcessExecution.RunAsync` etc. attach `WithCheckpointing` | `dotnet/src/Microsoft.Agents.AI.Workflows/InProcessExecution.cs:54-74` |
| Workflow checkpoint store DI | Sample harness uses `FileSystemJsonCheckpointStore` if `UseJsonCheckpoints` else in-memory | `dotnet/src/Shared/Workflows/Execution/WorkflowRunner.cs:59-69` |
| Cosmos DB chat history tests | `CosmosChatHistoryProviderTests`, `CosmosCheckpointStoreTests` | `dotnet/tests/Microsoft.Agents.AI.CosmosNoSql.UnitTests/CosmosChatHistoryProviderTests.cs`, `dotnet/tests/Microsoft.Agents.AI.CosmosNoSql.UnitTests/CosmosCheckpointStoreTests.cs` |
| Valkey tests | `ValkeyChatHistoryProviderTests` | `dotnet/tests/Microsoft.Agents.AI.Valkey.UnitTests/ValkeyChatHistoryProviderTests.cs` |
| Durable Task state tests | `DurableAgentStateTests`, `DurableAgentStateMessageTests` | `dotnet/tests/Microsoft.Agents.AI.DurableTask.UnitTests/State/` |
| File-system checkpoint tests | `FileSystemJsonCheckpointStoreTests` covers reopen-after-crash semantics | `dotnet/tests/Microsoft.Agents.AI.Workflows.UnitTests/FileSystemJsonCheckpointStoreTests.cs:82-322` |
| `AgentSession` security note on serialization | "treat restoring a session from an untrusted source as equivalent to accepting untrusted input" | `dotnet/src/Microsoft.Agents.AI.Abstractions/AgentSession.cs:46-52` |

## Answers to Dimension Questions

1. **What survives a restart?**
   - Anything written to Durable Task entities (entity state, `DurableAgentState`) survives restart and machine death because DTFx owns persistence (`dotnet/src/Microsoft.Agents.AI.DurableTask/AgentEntity.cs:13`).
   - Anything written to Cosmos DB (chat history via `CosmosChatHistoryProvider` and workflow checkpoints via `CosmosCheckpointStore<T>`) survives restart (`dotnet/src/Microsoft.Agents.AI.CosmosNoSql/CosmosChatHistoryProvider.cs:218-308`, `dotnet/src/Microsoft.Agents.AI.CosmosNoSql/CosmosCheckpointStore.cs:98-161`).
   - Anything written to Valkey survives restart (`dotnet/src/Microsoft.Agents.AI.Valkey/ValkeyChatHistoryProvider.cs:83-171`).
   - `FileSystemJsonCheckpointStore` JSON files survive restart, but a single process at a time (`dotnet/src/Microsoft.Agents.AI.Workflows/Checkpointing/FileSystemJsonCheckpointStore.cs:60-65`).
   - `FileSystemAgentFileStore` files survive restart but are machine-local (`dotnet/src/Microsoft.Agents.AI/Harness/FileStore/FileSystemAgentFileStore.cs:46-60`).
   - **`InMemoryChatHistoryProvider`, `InMemoryAgentFileStore`, `InMemoryConversationStorage`, `InMemoryCheckpointManager` do NOT survive restart** (`dotnet/src/Microsoft.Agents.AI.Abstractions/InMemoryChatHistoryProvider.cs:17-26`, `dotnet/src/Microsoft.Agents.AI/Harness/FileStore/InMemoryAgentFileStore.cs:18-22`, `dotnet/src/Microsoft.Agents.AI.Hosting.OpenAI/Conversations/InMemoryConversationStorage.cs:17-18`, `dotnet/src/Microsoft.Agents.AI.Workflows/Checkpointing/InMemoryCheckpointManager.cs:13`).
   - The default `HarnessAgent` chat-history and file-memory stores are in-memory and cwd-relative file system respectively, so neither survives a redeploy with no persisted volume (`dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgent.cs:193-199,298-318`).

2. **What survives redeploy?**
   - Only state written to external durable stores (Durable Task backend, Cosmos DB, Valkey, distributed cache) survives redeploy.
   - State in `AgentSessionStateBag` survives *only* if the caller invokes `agent.SerializeSessionAsync` and saves the JSON themselves — there is no framework auto-save (`dotnet/src/Microsoft.Agents.AI.Abstractions/AIAgent.cs:187,222`, `dotnet/src/Microsoft.Agents.AI.Abstractions/AgentSession.cs:39-44`).
   - `FileSystemAgentFileStore` files survive redeploy only if the deployment volume persists across instances; this is unspecified by the framework.
   - `FileSystemJsonCheckpointStore` content survives redeploy only on a shared/persistent volume and only when the directory is exclusive (`dotnet/src/Microsoft.Agents.AI.Workflows/Checkpointing/FileSystemJsonCheckpointStore.cs:60-65`).

3. **What survives multi-instance operation?**
   - Durable Task entities are the canonical multi-instance story: any worker process can route messages/signals to the same entity, and entity state is shared (`dotnet/src/Microsoft.Agents.AI.DurableTask/DefaultDurableAgentClient.cs:14-30`).
   - Cosmos DB / Valkey are external stores and trivially multi-instance safe (`dotnet/src/Microsoft.Agents.AI.CosmosNoSql/CosmosChatHistoryProvider.cs:114-135`, `dotnet/src/Microsoft.Agents.AI.Valkey/ValkeyChatHistoryProvider.cs:60-77`).
   - `FileSystemJsonCheckpointStore` is explicitly single-process via `FileShare.None` (`dotnet/src/Microsoft.Agents.AI.Workflows/Checkpointing/FileSystemJsonCheckpointStore.cs:60-65`).
   - `FileSystemAgentFileStore` has no concurrency control beyond `File.WriteAllTextAsync` semantics — concurrent writers will race (`dotnet/src/Microsoft.Agents.AI/Harness/FileStore/FileSystemAgentFileStore.cs:63-80`).
   - In-memory stores (`InMemoryChatHistoryProvider`, `InMemoryAgentFileStore`, `InMemoryConversationStorage`, `InMemoryCheckpointManager`) are per-process and will diverge across instances.

4. **What is silently dropped?**
   - If a caller forgets to wire `ChatHistoryProvider`, agent messages stay only in `AgentSession.StateBag` and disappear with the process (`dotnet/src/Microsoft.Agents.AI.Abstractions/InMemoryChatHistoryProvider.cs:23-26`).
   - `HarnessAgent` defaults to `InMemoryChatHistoryProvider` and cwd-relative file stores without warning (`dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgent.cs:193-199,300-318`).
   - `ValkeyChatHistoryProvider` does not enforce a TTL — caller must use `ClearMessagesAsync` (`dotnet/src/Microsoft.Agents.AI.Valkey/ValkeyChatHistoryProvider.cs:25-29`).
   - Per-service-call persistence only takes effect when `RequirePerServiceCallChatHistoryPersistence=true` AND a `PerServiceCallChatHistoryPersistingChatClient` is present in the pipeline; otherwise the framework emits a warning log and falls back to end-of-run persistence (`dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientAgentLogMessages.cs:75-95`).
   - Purview cache silently no-ops if no `IDistributedCache` is registered in DI (the package does not register one by default) (`dotnet/src/Microsoft.Agents.AI.Purview/CacheProvider.cs:25`, `dotnet/src/Microsoft.Agents.AI.Purview/PurviewExtensions.cs`).
   - Tool approval rules and todos persist only as long as `AgentSession.StateBag` does — no auto-save (`dotnet/src/Microsoft.Agents.AI/Harness/ToolApproval/ToolApprovalState.cs:17-22`, `dotnet/src/Microsoft.Agents.AI/Harness/Todo/TodoProvider.cs:80-83`).
   - Malformed Valkey entries are logged at warning and skipped, not surfaced (`dotnet/src/Microsoft.Agents.AI.Valkey/ValkeyChatHistoryProvider.cs:122-126`).

5. **Is durability configurable?**
   - Yes for most backends:
     - Cosmos DB TTL: `MessageTtlSeconds` (default 86400), hierarchical partitioning (`dotnet/src/Microsoft.Agents.AI.CosmosNoSql/CosmosChatHistoryProvider.cs:82-88,197-215`).
     - Valkey: `MaxMessages`, `KeyPrefix`, `MaxMessagesToRetrieve`; no TTL (`dotnet/src/Microsoft.Agents.AI.Valkey/ValkeyChatHistoryProviderOptions.cs:14-30`).
     - Durable Task: `DefaultTimeToLive`, per-agent `timeToLive`, `MinimumTimeToLiveSignalDelay` (`dotnet/src/Microsoft.Agents.AI.DurableTask/DurableAgentsOptions.cs:25-74`).
     - `InMemoryChatHistoryProvider`: `StateKey`, JSON options, `ChatReducer`, `ReducerTriggerEvent` (`dotnet/src/Microsoft.Agents.AI.Abstractions/InMemoryChatHistoryProviderOptions.cs:14-49`).
     - Harness: `ChatHistoryProvider`, `FileMemoryStore`, `FileAccessStore`, `DisableFileMemory`, `DisableFileAccess` (`dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgentOptions.cs:130-257`).
     - Workflow checkpointing: `CheckpointManager.CreateInMemory`/`CreateJson` pluggable (`dotnet/src/Microsoft.Agents.AI.Workflows/CheckpointManager.cs:33-51`).
   - **No framework-level toggle for "make the default session durable"** — durability of `AgentSession` is caller-controlled through `SerializeSessionAsync`.

## Architectural Decisions

- **Two-tier state model**: lightweight, in-process per-session state in `AgentSessionStateBag` (`dotnet/src/Microsoft.Agents.AI.Abstractions/AgentSessionStateBag.cs:21`) vs. durable state in external stores. The framework deliberately does not auto-promote one to the other; callers serialize sessions explicitly (`dotnet/src/Microsoft.Agents.AI.Abstractions/AIAgent.cs:178-188`).
- **Pluggable `ChatHistoryProvider` contract** with three lifecycle hooks (`InvokingAsync`, `InvokedAsync`, filters) and a default storage backend (`InMemoryChatHistoryProvider`) (`dotnet/src/Microsoft.Agents.AI.Abstractions/ChatHistoryProvider.cs:51-477`).
- **`ICheckpointStore<T>` abstraction for workflows** with three reference implementations (in-memory, file system, Cosmos DB) chosen via `CheckpointManager.CreateJson(store)` (`dotnet/src/Microsoft.Agents.AI.Workflows/Checkpointing/ICheckpointStore.cs:12-46`, `dotnet/src/Microsoft.Agents.AI.Workflows/CheckpointManager.cs:47-51`).
- **Durable Task entities as the multi-instance durable agent substrate**, including schema-versioned state (`DurableAgentState.schemaVersion = "1.1.0"`) and TTL-based cleanup via self-signalling (`dotnet/src/Microsoft.Agents.AI.DurableTask/State/DurableAgentState.cs:26`, `dotnet/src/Microsoft.Agents.AI.DurableTask/AgentEntity.cs:121-149,197-215`).
- **Per-service-call persistence as opt-in for crash recovery** between LLM iterations within a function-invocation loop (`dotnet/src/Microsoft.Agents.AI/ChatClient/PerServiceCallChatHistoryPersistingChatClient.cs:57-358`).
- **Schema versioning for `DurableAgentState`** with hard-fail on major version mismatch (`dotnet/src/Microsoft.Agents.AI.DurableTask/State/DurableAgentStateJsonConverter.cs:38-41`); workflow JSON checkpoint marshalling is wired through `JsonMarshaller` (`dotnet/src/Microsoft.Agents.AI.Workflows/CheckpointManager.cs:47-51`).
- **Default-then-opt-out model for the `HarnessAgent`**: batteries-included but defaults are non-durable; durability is achieved by passing a real `ChatHistoryProvider` and `FileMemoryStore`/`FileAccessStore` (`dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgent.cs:193-318`).
- **Single-purpose vector-store integration** via `Microsoft.Extensions.VectorData` for semantic retrieval memories; durability is delegated to whichever vector store is plugged in (`dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProvider.cs:55-148`).

## Notable Patterns

- **Provider-session-state helper**: `ProviderSessionState<TState>` encapsulates the `StateBag.GetValue/SetValue` dance for each provider and avoids per-provider reinvention (`dotnet/src/Microsoft.Agents.AI.Abstractions/ProviderSessionState{TState}.cs:26-87`).
- **Transaction batch + hierarchical partition keys for Cosmos**: `CosmosChatHistoryProvider` validates that all items in a batch share the same partition key (`dotnet/src/Microsoft.Agents.AI.CosmosNoSql/CosmosChatHistoryProvider.cs:341-360`) and splits batches that exceed 2 MB (`dotnet/src/Microsoft.Agents.AI.CosmosNoSql/CosmosChatHistoryProvider.cs:380-397`).
- **Tombstoning-on-TTL for Durable Task entities**: rather than relying on a side-channel sweeper, the agent schedules a self-signal to delete itself when TTL elapses (`dotnet/src/Microsoft.Agents.AI.DurableTask/AgentEntity.cs:197-215`).
- **SessionStateBag JSON persistence**: all per-session state is JSON-serializable via `AgentSessionStateBagJsonConverter`, so `SerializeSessionAsync` can be safely applied even when the session contains arbitrary provider state (`dotnet/src/Microsoft.Agents.AI.Abstractions/AgentSessionStateBag.cs:124-144`, `dotnet/src/Microsoft.Agents.AI.Abstractions/AgentSessionStateBagJsonConverter.cs`).
- **FileSystem checkpoint store atomicity via temp + flush**: index updates are line-appended and flushed before the write returns (`dotnet/src/Microsoft.Agents.AI.Workflows/Checkpointing/FileSystemJsonCheckpointStore.cs:148-160`).
- **Fail-soft Mem0 and vector-store searches**: `Mem0Provider` and `ChatHistoryMemoryProvider` catch and log search failures instead of crashing the agent (`dotnet/src/Microsoft.Agents.AI.Mem0/Mem0Provider.cs:175-191`, `dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProvider.cs:234-249,287-300`).
- **Filter pipelines in providers**: every persistent provider exposes `ProvideOutputMessageFilter`, `StorageInputRequestMessageFilter`, `StorageInputResponseMessageFilter` to bound what is persisted (`dotnet/src/Microsoft.Agents.AI.Abstractions/ChatHistoryProvider.cs:53-77`).

## Tradeoffs

- **No automatic `AgentSession` persistence** — durable session is a caller responsibility. This keeps the core framework light but pushes durability mistakes to application code.
- **Harness defaults are non-durable** to favor developer ergonomics on a developer machine. Production deployments must swap stores (`dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgent.cs:193-318`).
- **File-system checkpoint store is single-process** (`FileShare.None`) — durable across restarts on a persistent volume, but not horizontally scalable (`dotnet/src/Microsoft.Agents.AI.Workflows/Checkpointing/FileSystemJsonCheckpointStore.cs:60-65`).
- **Valkey has no TTL** — operators must run their own eviction or use `MaxMessages` (`dotnet/src/Microsoft.Agents.AI.Valkey/ValkeyChatHistoryProvider.cs:25-29`).
- **Cosmos DB chat history doc size cap (2 MB)**: messages larger than 2 MB throw with an actionable error message rather than truncating (`dotnet/src/Microsoft.Agents.AI.CosmosNoSql/CosmosChatHistoryProvider.cs:411-418`).
- **DTFx entity-state size limits** are inherited from the durable backend; the framework does not chunk conversation history beyond TTL.
- **`ChatHistoryMemoryProvider` accepts untrusted data as-is** — store compromise ⇒ indirect prompt injection (`dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProvider.cs:39-43`).
- **`Mem0Provider` requires HTTPS / authenticated `HttpClient`** — security-by-configuration, not enforced (`dotnet/src/Microsoft.Agents.AI.Mem0/Mem0Provider.cs:30-44`).
- **Compaction is destructive**: strategies like `SummarizationCompactionStrategy` (`dotnet/src/Microsoft.Agents.AI/Compaction/SummarizationCompactionStrategy.cs:37`) and `TruncationCompactionStrategy` mutate the in-memory list, but the persisted store still holds the originals — durability of compaction is by-storage, not by-framework.

## Failure Modes / Edge Cases

- **Process crash before `ChatHistoryProvider` flush**: with `RequirePerServiceCallChatHistoryPersistence=false`, messages from a half-completed run may not be persisted (`dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientAgentOptions.cs:139-152`).
- **Concurrent file-store writers**: `FileSystemAgentFileStore` lacks cross-process locking; two pods writing to the same volume will race (`dotnet/src/Microsoft.Agents.AI/Harness/FileStore/FileSystemAgentFileStore.cs:63-80`).
- **Cosmos 429 / throttling**: SDK retries transparently, but `CosmosChatHistoryProvider` raises `InvalidOperationException` on non-success batch statuses and propagates exceptions (`dotnet/src/Microsoft.Agents.AI.CosmosNoSql/CosmosChatHistoryProvider.cs:374-378`).
- **Cosmos 2 MB item size**: explicit error message but no automatic chunking (`dotnet/src/Microsoft.Agents.AI.CosmosNoSql/CosmosChatHistoryProvider.cs:411-418`).
- **TTL race in Durable Task entities**: between TTL expiry and the scheduled deletion signal, callers can still signal the entity. The deletion simply clears state on next operation; in-flight calls are not cancelled (`dotnet/src/Microsoft.Agents.AI.DurableTask/AgentEntity.cs:166-195`).
- **Schema-version bump**: major version change rejects deserialization outright — no automatic migration (`dotnet/src/Microsoft.Agents.AI.DurableTask/State/DurableAgentStateJsonConverter.cs:38-41`).
- **Valkey malformed JSON**: logged at warning and skipped, so the message is silently lost (`dotnet/src/Microsoft.Agents.AI.Valkey/ValkeyChatHistoryProvider.cs:122-126`).
- **`FileSystemJsonCheckpointStore` index corruption**: throws `InvalidOperationException("Index corrupted.")` rather than recovering (`dotnet/src/Microsoft.Agents.AI.Workflows/Checkpointing/FileSystemJsonCheckpointStore.cs:92-95`).
- **`HarnessAgent` is `[Experimental]`**: surface area and durability semantics may shift (`dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgent.cs:83`).
- **Compaction + multi-instance**: a non-durable `ChatHistoryProvider` (e.g. `InMemoryChatHistoryProvider`) combined with compaction means compaction is lost on restart because the reducer input never persisted (`dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgent.cs:193-199`).

## Future Considerations

- **Auto-save hook for `AgentSession`**: an opt-in `ISessionStore` would close the "caller must remember to serialize" gap and let `HarnessAgent` survive restarts transparently.
- **Valkey TTL**: a `MessageTtlSeconds`-style option on `ValkeyChatHistoryProviderOptions` would remove the "no TTL and persist indefinitely" caveat (`dotnet/src/Microsoft.Agents.AI.Valkey/ValkeyChatHistoryProvider.cs:25-29`).
- **File-store cross-process locking**: a `FileShare.None` or advisory-lock layer on `FileSystemAgentFileStore` would make multi-pod deployments safer (`dotnet/src/Microsoft.Agents.AI/Harness/FileStore/FileSystemAgentFileStore.cs:63-80`).
- **Durable compaction**: persist summarization events so resumed agents don't re-summarize from scratch.
- **Distributed cache default**: register a default in-memory `IDistributedCache` for Purview when no other is configured, or fail-fast at startup.
- **Migration story for `DurableAgentState`**: a major-version bump currently throws — adding a version registry with `TryUpgrade` would avoid invalidating existing state (`dotnet/src/Microsoft.Agents.AI.DurableTask/State/DurableAgentStateJsonConverter.cs:38-41`).
- **Observability**: expose durability-tier counters (e.g., `cosmos_messages_persisted`, `durable_entity_state_bytes`) so operators can verify production durability.

## Questions / Gaps

- **Production durability for `HarnessAgent` defaults** — explicit documentation of how to swap defaults for Cosmos/Valkey/Durable stores when promoting a `HarnessAgent` from dev to prod is not present in the source files inspected; only the option properties exist (`dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgentOptions.cs:130-257`).
- **`AgentSession` lifecycle durability in ASP.NET hosting** — no evidence found in this source of automatic session save/load on HTTP request boundaries; the `Microsoft.Agents.AI.Hosting.AspNetCore` package uses `ClaimsIdentitySessionIsolationKeyProvider` for routing isolation but does not specify persistence behavior (`dotnet/src/Microsoft.Agents.AI.Hosting.AspNetCore/ClaimsIdentitySessionIsolationKeyProvider.cs`).
- **`FileSystemAgentFileStore` concurrent write semantics** — no evidence of file locking beyond `File.WriteAllTextAsync`; multi-pod deployments on a shared PVC will need external coordination (`dotnet/src/Microsoft.Agents.AI/Harness/FileStore/FileSystemAgentFileStore.cs:63-80`).
- **Cosmos DB provisioning** — no evidence that the framework auto-creates databases/containers/indexes; callers must provision them, otherwise queries will throw (`dotnet/src/Microsoft.Agents.AI.CosmosNoSql/CosmosChatHistoryProvider.cs:114-135`).
- **Mem0 transport encryption** — explicitly delegated to caller; no built-in HTTPS enforcement (`dotnet/src/Microsoft.Agents.AI.Mem0/Mem0Provider.cs:30-44`).
- **`FileSystemJsonCheckpointStore` durability on shared PVC** — exclusive lock precludes multi-instance workflows on shared volumes; no Redis-style store provided out of the box (`dotnet/src/Microsoft.Agents.AI.Workflows/Checkpointing/FileSystemJsonCheckpointStore.cs:60-65`).
- **Compaction + `ChatHistoryProvider` durability interaction** — no evidence that compaction strategy state is itself persisted; resumption relies on the underlying provider retaining full or summarized history (`dotnet/src/Microsoft.Agents.AI/Compaction/SummarizationCompactionStrategy.cs:37-89`).

---

Generated by `studies/agent-harness-study` against `agent-framework`.
# Source Analysis: crewai

## Dimension 15.03 — Shared State and Conflict Resolution

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (pydantic, SQLite, LanceDB, Qdrant, Redis/portalocker; monorepo with `lib/crewai`, `lib/crewai-core`, `lib/crewai-tools`, `lib/crewai-files`) |
| Analyzed | 2026-08-26 |

All citations below are workspace-relative paths into the selected source directory (`studies/agent-harness-study/sources/crewai/...`).

## Summary

CrewAI agents share state through four main channels: (1) a unified vector `Memory` that is namespaced per crew (`/crew/{name}`) and per agent role (`.../agent/{role}`), (2) task-output context passing inside crews (sequential/hierarchical processes, plus async tasks sharing the `task_outputs` list), (3) flow-level mutable state objects mutated by concurrently scheduled listeners, and (4) shared infrastructure stores: a crew-wide tool-result cache, kickoff task-output SQLite storage, flow-state persistence, and file attachments keyed by execution ID.

Conflict handling is layered. Within one process, memory writes are serialized through a single-worker background save pool with a read barrier (`drain_writes`) before recall and an RLock-guarded reset; shared registries (event record, tool cache, event-bus handler map) use a custom reader-writer lock. Across processes, a centralized lock factory (`crewai_core.lock_store`) provides Redis-distributed or file-based `portalocker` locks around every persistent store (LanceDB memory, kickoff-output DB, flow-state DB, log files, project-ID minting). Semantic conflicts in memory are detected via cosine-similarity consolidation thresholds and resolved by an LLM that emits a typed keep/update/delete plan, with intra-batch deduplication and first-wins dedup of actions targeting the same record to avoid commit conflicts. LanceDB writes additionally use optimistic concurrency with exponential-backoff retries on "Commit conflict" errors.

The weakest area is Flow state: listeners execute concurrently against one shared mutable `self._state` object with no locking or copy-on-write, so two parallel listeners can interleave updates. Also notable: the dedicated multi-process memory stress test exists but is skipped.

## Rating

**7 / 10** — Clear model with explicit interfaces, tests, and operational safeguards for shared memory and persistent stores (serialized save pool + read barrier, distributed/file locks, optimistic-retry writes, typed failure events). Not higher because: concurrent flow listeners mutate shared flow state without synchronization (`lib/crewai/src/crewai/flow/runtime/__init__.py:3157-3165`), LLM-based consolidation silently degrades to insert-on-failure which allows duplicates (`lib/crewai/src/crewai/memory/analyze.py:369-375`), and the multi-process concurrency stress test is disabled (`lib/crewai/tests/memory/test_concurrent_storage.py:13`). On the rubric question "Can two agents update the same resource without corrupting it?" — yes for memory and all lock-guarded persistent stores within documented limits; only ad-hoc mutation of live Flow state remains unsafe.

## Evidence Collected

Every entry cites a workspace-relative path from the selected source directory with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Shared state API — unified Memory | `Memory` class: single intelligent memory w/ LLM analysis, scopes, pluggable storage (default LanceDB) | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/unified_memory.py:76-99` |
| Crew namespace for shared memory | Crew validator auto-creates `Memory(root_scope=f"/crew/{crew_name}")` when `memory=True` | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/crew.py:652-688` |
| Per-agent sub-namespace | Executor saves agent results under `{root}/agent/{sanitized_role}` root scope; delegation outputs skipped | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:31-65` |
| Scoped/sliced views over shared memory | `MemoryScope` (root-path view, `bind()` after restore) and `MemorySlice` (multi-scope read-only by default, merged+deduped recall) | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/memory_scope.py:38-75, 227-324` |
| Task-context sharing in crews | `_get_context` aggregates prior task outputs into a task's context string | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/crew.py:1865-1874` |
| Context-conflict validation | Validators reject async tasks including sequential async context tasks and context referencing future tasks | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/crew.py:840-862, 864-880` |
| Hierarchical delegation as shared channel | Manager gets `AgentTools(agents=...)` (delegate work / ask question tools); manager shares crew cache handler | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/crew.py:1518-1548`; `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/tools/agent_tools/delegate_work_tool.py:16-30` |
| Crew-wide tool cache (shared) | `CacheHandler` dict + RWLock; declared `ToolFailure`s never cached | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/agents/cache/cache_handler.py:10-60` |
| File attachments keyed by execution | Global in-memory aiocache store; task files override crew files on name conflict (last-wins merge) | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/file_store.py:58-61, 154-175` |
| Kickoff task-output store (shared, cross-process) | SQLite `latest_kickoff_task_outputs` with `INSERT OR REPLACE` per task_id and `was_replayed` flag | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/kickoff_task_outputs_storage.py:46-58, 92-106` |
| Conflict detector — semantic overlap | Consolidation triggered when similarity ≥ `consolidation_threshold` (0.85); batch dedup at 0.98 | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/types.py:185-213`; `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/encoding_flow.py:121-140` |
| Conflict resolver — LLM plan | `ConsolidationAction`/`ConsolidationPlan` (keep/update/delete + insert_new); safe default = insert on LLM failure | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/analyze.py:103-141, 318, 321-375` |
| Cross-item action dedup (first wins) | `execute_plans` collects one action per record_id ("first wins") to prevent LanceDB commit conflicts between two pipeline items | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/encoding_flow.py:371-412` |
| Write serialization (in-process) | Memory saves run through single-worker ThreadPoolExecutor (`max_workers=1`) tracked as pending futures | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/unified_memory.py:165-171, 297-322` |
| Read barrier | `recall()` calls `drain_writes()` before searching so reads see all pending saves | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/unified_memory.py:350-363, 711-713` |
| Reset vs save race protection | `_reset_lock` (RLock) held across drain+reset; test proves no save can be submitted mid-reset | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/unified_memory.py:172, 1022-1035`; `studies/agent-harness-study/sources/crewai/tests/memory/test_unified_memory.py:990-1035` |
| Coordination lock factory | `lock(name)` → Redis lock if `REDIS_URL`+redis installed, else portalocker file lock in temp dir; 120s default timeout; pluggable backend | `studies/agent-harness-study/sources/crewai/lib/crewai-core/src/crewai_core/lock_store.py:33-35, 45-54, 79-121` |
| Lock consumers | LanceDB storage, kickoff-output DB, flow-state DB, file logs, ChromaDB client/factory, RAG adapters, tracing, project-ID minting | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/lancedb_storage.py:92, 100, 305`; `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/flow/persistence/sqlite.py:67, 73-76, 172-176`; `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/file_handler.py:88, 150, 163`; `studies/agent-harness-study/sources/crewai/lib/crewai-core/src/crewai_core/project.py:305-313` |
| Optimistic concurrency control | LanceDB writes retry on "Commit conflict" OSError with 0.2s doubling backoff, 5 attempts, table re-open between tries | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/lancedb_storage.py:34-39, 128-153` |
| Schema-conflict detection | `EmbeddingDimensionMismatchError` raised on dim mismatch instead of zero-filling corrupt vectors | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/backend.py:11-43`; `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/lancedb_storage.py:289-304` |
| RWLock primitive | Reader-writer lock, writer-priority condition variable; used by EventRecord, CacheHandler, event bus registry | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/rw_lock.py:12-81`; `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/state/event_record.py:108, 119-146, 157-158`; `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/events/event_bus.py:118-120, 153` |
| Checkpoint lineage (branch/fork) | `RuntimeState` serializes entities + event record with `parent_id`/`branch`; `fork()` creates unique branch names; version migrations on restore | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/state/runtime.py:177-198, 286-317, 352-389, 89-119` |
| First-wins racing listeners | or_() listener groups race concurrently; first completion cancels the rest | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/flow/runtime/__init__.py:1147-1196` |
| Concurrent listeners share live state | Parallel listeners dispatched with `asyncio.gather` while methods mutate the same `self._state` (copies made only for events/checkpoints) | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/flow/runtime/__init__.py:3157-3165, 1743-1763, 3038-3046` |
| Conflict logging — memory failures | `MemorySaveFailedEvent`/`MemoryQueryFailedEvent` emitted on bus, incl. background-save failures surfaced at drain | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/unified_memory.py:324-348, 505-521, 805-816` |
| Conflict logging — checkpoints | Typed checkpoint started/completed/failed/restore events with provider, branch, parent_id, duration | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/state/runtime.py:29-38, 245-263, 408-441` |
| Tests — lock & concurrency behavior | RWLock suite incl. stress test; lock-store backend selection tests; memory reset-race, recall-drain, and failure-reporting tests | `studies/agent-harness-study/sources/crewai/lib/crewai/tests/utilities/events/test_rw_lock.py:12-241`; `studies/agent-harness-study/sources/crewai/lib/crewai/tests/utilities/test_lock_store.py:55-107`; `studies/agent-harness-study/sources/crewai/lib/crewai/tests/memory/test_unified_memory.py:990-1092` |
| Gap — skipped multiprocess stress test | Dedicated N-process storage stress suite is entirely skipped (`pytestmark = pytest.mark.skip(...)`) | `studies/agent-harness-study/sources/crewai/lib/crewai/tests/memory/test_concurrent_storage.py:11-13` |

## Answers to Dimension Questions

**1. What state is shared between agents?**
- Unified `Memory`: crew-scoped instance created by the crew validator (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/crew.py:665-686`); agents without their own memory fall back to `crew._memory` when saving (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:31-39`), each writing under `/agent/{role}` subtrees (`base_agent_executor.py:51-61`). Flows get their own `Memory(root_scope=f"/flow/{flow_name}")` (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/flow/runtime/__init__.py:836`).
- Task outputs: downstream tasks consume upstream outputs via explicit `context` lists aggregated by `Crew._get_context` (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/crew.py:1865-1874`).
- Crew tool-result cache: one `CacheHandler` offered to every agent and the manager (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/crew.py:631-632, 1547-1548`).
- Kickoff task-output SQLite DB shared by replays across processes (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/kickoff_task_outputs_storage.py:24-29`).
- File attachments keyed by crew execution/task ID (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/file_store.py:64-175`).
- Flow `state`: one mutable object visible to all methods/listeners of a flow instance (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/flow/runtime/__init__.py:1765-1767`).

**2. How are conflicts detected?**
- Semantic overlap in memory: cosine similarity search above `consolidation_threshold=0.85` finds candidate records before insert; near-exact duplicates within a batch dropped at 0.98 (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/encoding_flow.py:121-140, 154-221`; `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/types.py:185-213`).
- Storage-level write contention: LanceDB raises OSError containing "Commit conflict"; retried with backoff (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/lancedb_storage.py:137-152`). Incompatible embedding dimensions raise `EmbeddingDimensionMismatchError` rather than corrupting search (`lancedb_storage.py:289-304`).
- Structural conflicts in crews: validators detect illegal async/context combinations and future-task references before kickoff (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/crew.py:840-880`).
- Loop conflicts in flows: per-method call counting raises `RecursionError` past `max_method_calls` (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/flow/runtime/__init__.py:3248-3256`).

**3. How are conflicts resolved?**
- Memory semantic conflicts: an LLM produces a typed `ConsolidationPlan` choosing keep/update/delete per overlapping record plus whether to insert the new content (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/analyze.py:103-141, 321-375`). When several batch items target the same existing record, only the first action applies ("first wins") to prevent interleaved delete/update conflicts (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/encoding_flow.py:374-412`).
- Concurrent writers: single-worker save pool serializes encoding pipelines per `Memory` instance (`unified_memory.py:165-169`); cross-process writers serialize via `store_lock`; residual LanceDB commit races absorbed by retry loop.
- Racing alternative branches: or_() racing listeners resolve first-wins, losers cancelled (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/flow/runtime/__init__.py:1154-1193`).
- Naming collisions for files: task-scoped entries override crew-scoped entries with identical keys (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/file_store.py:169-175`).
- Replay overwrites: `INSERT OR REPLACE` keyed on task_id records the replayed output with `was_replayed=True` (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/kickoff_task_outputs_storage.py:54, 92-106`).
- Divergent histories: checkpoint fork/branch lineage gives each branch unique labels and parent chains rather than merging (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/state/runtime.py:352-389`).

**4. Is shared state consistent?**
- Yes for memory within a process: writes are pooled and ordered, recalls drain pending writes (`test: studies/agent-harness-study/sources/crewai/lib/crewai/tests/memory/test_unified_memory.py:1038-1056`), resets cannot interleave with submissions (`unified_memory.py:1022-1035`), and record updates are atomic delete+add under the store lock (`lancedb_storage.py:330-337`).
- Mostly yes across processes for persistent stores: named locks guard SQLite/LanceDB/log writes, WAL mode enabled (`kickoff_task_outputs_storage.py:42-44`; `flow/persistence/sqlite.py:77`), and project-ID minting re-reads under the lock so a concurrent minter's ID wins (`studies/agent-harness-study/sources/crewai/lib/crewai-core/src/crewai_core/project.py:305-326`). Caveat: if the lock backend fails, project-ID minting degrades to best-effort read ("a torn write is worse than no id", `project.py:311-313`).
- Not guaranteed for live Flow state under parallel listeners: no copy-on-write or lock protects `self._state` during `asyncio.gather` fan-out (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/flow/runtime/__init__.py:3157-3165`); consistency is only ensured at persistence boundaries after each method completes (`__init__.py:2931`).
- Read paths rely on SQLite WAL rather than locks in some places (e.g., `load()` without lock, `kickoff_task_outputs_storage.py:173-180`; `SqliteProvider.checkpoint` uses plain sqlite connection without `store_lock`, `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/state/provider/sqlite_provider.py:79-84`).

## Architectural Decisions

1. **Namespace-partitioned shared memory instead of free-for-all writes.** One backing `Memory` per crew with `root_scope="/crew/{name}"` and per-agent `/agent/{role}` prefixes gives isolation-by-convention while keeping a single physical store (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/crew.py:662-680`; `.../agents/agent_builder/base_agent_executor.py:51-61`). Read-only `MemorySlice` views default to non-writable multi-scope access (`.../memory/memory_scope.py:234-236, 279-290`).
2. **Serialize-then-barrier write model.** All encodes go through a 1-worker pool with future tracking; any read drains pending futures first. This trades throughput for strict ordering and eliminates most in-process write races by construction (`.../memory/unified_memory.py:165-171, 350-363, 711-713`).
3. **Centralized, replaceable lock service.** A single `lock(name)` factory fronts Redis/file backends and is injectable via `set_lock_backend`, so tests can swap in in-process locks and deployments can scale out (`studies/agent-harness-study/sources/crewai/lib/crewai-core/src/crewai_core/lock_store.py:39-54`). MD5-namespaced channels avoid collisions (`lock_store.py:97`).
4. **Optimistic concurrency at the storage layer.** Rather than holding coarse locks for everything, LanceDB commits optimistically and retries on conflict — locks protect init/compaction/reset, retries absorb hot-write windows (`.../memory/storage/lancedb_storage.py:34-39, 128-153, 223-231`).
5. **LLM-as-arbiter for semantic conflicts.** Overlap resolution is delegated to the model with a constrained schema (`ConsolidationPlan`), falling back to "insert" so user data is never lost on LLM failure (`.../memory/analyze.py:321-375`).
6. **Copy-at-the-boundary flow state.** State copies are produced only where snapshots are needed (events, checkpoints, human-feedback pause) via deep copy + JSON dump; live mutation stays shared (`.../flow/runtime/__init__.py:1743-1763, 3038-3046, 3354-3356`).

## Notable Patterns

- **Read barrier / drain pattern**: `recall()` waits for background saves; crew teardown also drains crew, manager, and per-agent memories so late `MemorySaveCompletedEvent`s are not lost (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/unified_memory.py:711-713`; `.../crew.py:1887-1909`).
- **Deepcopy hardening for unshared internals**: `Memory.__deepcopy__` recreates executors/locks in the copy instead of copying OS resources (`.../memory/unified_memory.py:174-204`).
- **First-wins dedup maps** appear twice: cross-item consolidation actions (`.../memory/encoding_flow.py:384-412`) and racing listeners (`.../flow/runtime/__init__.py:1182-1193`).
- **Rebinding dependency views after restore**: `MemoryScope/MemorySlice` are persisted without the live `Memory` and re-bound post-checkpoint so all views share one backing store (`.../crew.py:540-571`; `.../memory/memory_scope.py:68-84`).
- **Typed lifecycle observability**: every checkpoint and memory save/query emits started/completed/failed events carrying provider, branch, parent_id, durations, and error strings — usable as a conflict/audit log (`.../state/runtime.py:245-263, 408-441`; `.../events/types/memory_events.py` referenced from `unified_memory.py:17-24`).
- **Writer-priority RWLock** reused across cache, event record, and handler registry keeps read-heavy paths parallel while making registry mutations exclusive (`.../utilities/rw_lock.py:57-62`).

## Tradeoffs

- **Single-worker save pool** guarantees ordering but caps memory write throughput per process; heavy `remember_many` workloads serialize behind one thread (`unified_memory.py:165-169`).
- **LLM arbitration** yields high-quality merges but adds latency and nondeterminism; failure mode is duplicate accumulation rather than data loss (`analyze.py:369-375`).
- **File-lock fallback** works out-of-the-box but is host-local; genuinely distributed crews must set `REDIS_URL` or provide a custom backend, otherwise cross-host mutual exclusion does not exist (`lock_store.py:1-10, 79-121`).
- **Shared live Flow state** simplifies authoring (methods just mutate `self.state`) but pushes correctness responsibility onto users who use concurrent listeners (`flow/runtime/__init__.py:3157-3165`).
- **Skip-listed stress tests** keep CI fast/xdist-compatible but leave multi-process memory behavior formally unverified (`tests/memory/test_concurrent_storage.py:11-13`).

## Failure Modes / Edge Cases

- **Lost updates between concurrent flow listeners**: two listeners mutating the same state key during `gather` can clobber each other; last writer silently wins (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/flow/runtime/__init__.py:3157-3165`).
- **Duplicate memories on analysis failure**: consolidation/query/save analyses each degrade to defaults on exception — inserts proceed, so conflicting facts can coexist undetected (`.../memory/analyze.py:244-256, 309-315, 369-375`).
- **Lock-backend swap unsynchronized**: `set_lock_backend` while other threads hold/acquire locks is explicitly unsupported (`lock_store.py:49-51`).
- **Lock timeout surfaces as exception**: 120s default then `portalocker.exceptions.LockException` with remediation hint — callers must decide on retry semantics (`lock_store.py:112-117`).
- **Zero-vector placeholder rows**: records saved without embeddings get zero vectors, matching dimension but semantically inert; dim mismatches are rejected loudly instead (`lancedb_storage.py:308-310`; `backend.py:11-43`).
- **Background compaction races**: compaction runs on a daemon thread under the same store lock and absorbs all exceptions, so repeated failures are silent (`lancedb_storage.py:213-231`).
- **Checkpoint restore of legacy formats** relies on migration/backfill heuristics; unrecognized legacy knowledge sources raise instead of guessing (`state/runtime.py:89-150`).
- **Update miss is only a warning**: updating kickoff outputs by nonexistent `task_index` logs a warning and succeeds vacuously (`kickoff_task_outputs_storage.py:153-156`).

## Future Considerations

- Protect live Flow state for concurrent listeners (e.g., per-key merge hooks, copy-on-write with deterministic merge, or documenting listener exclusivity contracts) — concrete work item in `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/flow/runtime/__init__.py`.
- Re-enable or port the skipped multi-process storage stress suite (`tests/memory/test_concurrent_storage.py`) using temp-file IPC as its docstring already sketches, so cross-process claims stay regression-tested.
- Add CAS/version columns to `StorageBackend.update` to make record updates optimistic-concurrency-safe independent of the storage engine (`.../memory/storage/backend.py:102-104`).
- Consider a durable conflict log (append-only record of consolidation decisions with reasons already present in `ConsolidationAction.reason`) for post-hoc auditing (`.../memory/analyze.py:103-121`).
- Wrap `SqliteProvider.checkpoint` writes in `store_lock` for parity with other SQLite stores (`state/provider/sqlite_provider.py:79-84`).

## Questions / Gaps

- No evidence found of versioning or compare-and-swap on individual memory-record updates beyond whole-lock exclusion; searched `update|version|cas` across `lib/crewai/src/crewai/memory/`.
- No evidence found of user-facing documentation of the consolidation/conflict behavior; `docs/edge/en/concepts/memory.mdx` documents scoping and private memories (lines 110-122, 165-194) but not the keep/update/delete arbitration or its fallback-to-insert behavior.
- Whether `REDIS_URL`-backed locking is exercised in production-like tests is unverifiable in-repo: redis-path tests mock the connection (`tests/utilities/test_lock_store.py:64-80`), and true multi-process coverage is skipped.
- The exact concurrency contract for flows with multiple simultaneous listeners (allowed? discouraged? merged how?) is not stated in code or docs reviewed; only the first-wins special case for `or_()` racing groups is specified (`flow/runtime/__init__.py:1154-1158`).

---

Generated by `Dimension 15.03: Shared State and Conflict Resolution` against `crewai`.

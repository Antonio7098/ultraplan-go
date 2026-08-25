# Source Analysis: crewai

## Dimension 05.03: Long-Term User, Project, and Domain Memory

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (pydantic, LanceDB, Qdrant Edge, SQLite; uv workspace with `lib/crewai`, `lib/cli`, `lib/crewai-core`) |
| Analyzed | 2026-08-25 |

## Summary

CrewAI implements long-term memory as a **single unified `Memory` class** that replaced the legacy short-term/long-term/entity/external split (the old types survive only as deprecated CLI aliases — `lib/crewai/src/crewai/crew.py:2294-2296`, `lib/cli/src/crewai_cli/cli.py:430-451`). Persistence is a pluggable vector store: a `StorageBackend` protocol (`lib/crewai/src/crewai/memory/storage/backend.py:45-212`) with two built-in durable backends — LanceDB on local disk (default, `lib/crewai/src/crewai/memory/storage/lancedb_storage.py:42-78`) and Qdrant Edge with a write-local/flush-central shard pattern (`lib/crewai/src/crewai/memory/storage/qdrant_edge_storage.py:81-144`) — plus a process-wide storage factory for custom backends (`lib/crewai/src/crewai/memory/storage/factory.py:33-55`).

Scoping is hierarchical path-based (`MemoryRecord.scope`, e.g. `/company/team/user`, `lib/crewai/src/crewai/memory/types.py:28-31`). Crews auto-namespace under `/crew/{name}` (`lib/crewai/src/crewai/crew.py:652-688`), flows under `/flow/{name}` (`lib/crewai/src/crewai/flow/runtime/__init__.py:830-836`), and per-agent saves nest under `/crew/{crew}/agent/{role}` (`lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:50-61`). There is no first-class "user" or "organization" scope primitive; user identity is only a caller-supplied provenance string (`MemoryRecord.source`, `types.py:60-66`) paired with an opt-in `private` flag enforced as client-side recall filtering.

Write policy: LLM-analyzed encoding (scope/category/importance inference plus consolidation keep/update/delete decisions) run through a serialized background save pool (`lib/crewai/src/crewai/memory/unified_memory.py:297-322`, `lib/crewai/src/crewai/memory/encoding_flow.py:75-87`). Retrieval policy: adaptive-depth recall with LLM query distillation, multi-scope parallel search, and composite semantic+recency+importance scoring (`lib/crewai/src/crewai/memory/recall_flow.py:58-68`, `types.py:345-380`). Deletion exists at three levels (`forget()`, scoped `reset()`, CLI `reset-memories`), and freshness is managed via exponential recency decay, access touching, and LLM-driven consolidation of near-duplicate records.

The model is clear, tested (six dedicated test modules including a ~1,200-line root-scope suite), and operationally hardened (file locks, commit-conflict retries, read barriers over async writes, embedding-dimension migration errors). Its main gaps are privacy (opt-in flag, no identity binding or erasure-by-subject) and freshness guarantees that depend on best-effort LLM consolidation.

## Rating

**8 / 10.** Clear scoping model with explicit interfaces (`StorageBackend` protocol, `MemoryScope`/`MemorySlice` views), extensive tests (root scope, concurrency, dimension mismatch, storage factory), and real operational safeguards (cross-process locks with retry, drain-before-read barrier, actionable dimension-mismatch error guiding reset vs pin). It falls short of 9–10 because privacy controls are shallow and lightly tested, there are no user/organization scope primitives beyond naming conventions, and scale behavior relies on full-table scans capped at 50k rows (`lancedb_storage.py:29-32, 466-492`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Unified memory API | `class Memory(BaseModel)` with `remember`/`remember_many`/`recall`/`forget`/`update`/`reset` | lib/crewai/src/crewai/memory/unified_memory.py:76, 430, 523, 681, 818, 852, 1015 |
| Memory record schema | id, content, hierarchical `scope`, categories, metadata, importance, created_at, last_accessed, source, private | lib/crewai/src/crewai/memory/types.py:20-73 |
| Durable store (default) | LanceDB table persisted to `$CREWAI_STORAGE_DIR/memory` or platform data dir; dim auto-detect (3072 default) | lib/crewai/src/crewai/memory/storage/lancedb_storage.py:27, 42-114 |
| Platform data location | `db_storage_path()` → appdirs user data dir keyed by project name ("CrewAI") | lib/crewai-core/src/crewai_core/paths.py:16-26 |
| Alternative vector store | Qdrant Edge: local per-PID shard + central shard, flush on close/atexit | lib/crewai/src/crewai/memory/storage/qdrant_edge_storage.py:81-144 |
| Pluggable backend contract | `StorageBackend` Protocol: save/search/delete/update/get_record/list/reset (+async) | lib/crewai/src/crewai/memory/storage/backend.py:44-212 |
| Backend factory | `set_memory_storage_factory()` process-wide override for custom/remote stores | lib/crewai/src/crewai/memory/storage/factory.py:33-55 |
| Scope views | `MemoryScope` subtree view; `MemorySlice` multi-scope view with read-only writes | lib/crewai/src/crewai/memory/memory_scope.py:38-45, 227-236 |
| Crew root scope | Auto `root_scope=f"/crew/{sanitized_name}"`; user-supplied instances keep their own config | lib/crewai/src/crewai/crew.py:652-688 |
| Flow root scope | Flows auto-create `Memory(root_scope="/flow/{flow_name}")` when unset | lib/crewai/src/crewai/flow/runtime/__init__.py:830-836 |
| Agent-level nesting | Executor saves extracted memories under `{crew_root}/agent/{role}` | lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:50-63 |
| Write pipeline | EncodingFlow: batch embed → intra-batch dedup (≥0.98 cosine dropped) → parallel similar-search → parallel LLM analyze → execute plans | lib/crewai/src/crewai/memory/encoding_flow.py:112-222, 372-501 |
| Consolidation policy | Similarity ≥ `consolidation_threshold` (0.85) triggers LLM keep/update/delete plan; failure defaults to insert | lib/crewai/src/crewai/memory/types.py:185-202; lib/crewai/src/crewai/memory/analyze.py:318-375 |
| Serialized writes | Single-worker `ThreadPoolExecutor("memory-save")`; `read_only=True` makes writes silent no-ops | lib/crewai/src/crewai/memory/unified_memory.py:148-151, 165-171, 466-467 |
| Read-your-writes barrier | `recall()` calls `drain_writes()` before searching; failures surfaced via events, not raised | lib/crewai/src/crewai/memory/unified_memory.py:350-363, 711-713 |
| Retrieval flow | RecallFlow: LLM query distillation into ≤3 sub-queries, ≤20 candidate scopes, parallel search, confidence routing with exploration budget | lib/crewai/src/crewai/memory/recall_flow.py:180-291 |
| Ranking model | composite = 0.5·semantic + 0.3·recency-decay + 0.2·importance; decay half-life 30 days; oversampling factor 2 | lib/crewai/src/crewai/memory/types.py:17, 144-183, 345-380 |
| Freshness touchpoints | Recalled records get `last_accessed=now` via storage `touch_records` (best-effort) | lib/crewai/src/crewai/memory/unified_memory.py:784-790; lib/crewai/src/crewai/memory/storage/lancedb_storage.py:339-359 |
| Privacy filter | Private records visible only to same-source recall or `include_private=True` (shallow + deep paths) | lib/crewai/src/crewai/memory/unified_memory.py:746-751; lib/crewai/src/crewai/memory/recall_flow.py:109-114 |
| Deletion API | `forget(scope, categories, older_than, metadata_filter, record_ids)` returning deleted count | lib/crewai/src/crewai/memory/unified_memory.py:818-850; lib/crewai/src/crewai/memory/storage/lancedb_storage.py:411-464 |
| Reset flows | `reset(scope)` respects root_scope; `reset_all()` wipes whole table under RLock | lib/crewai/src/crewai/memory/unified_memory.py:1015-1035 |
| CLI deletion UX | `crewai reset-memories -m/-kn/-akn/-k/-a` with hidden deprecated `-l/-s/-e` flags | lib/cli/src/crewai_cli/cli.py:424-480; lib/crewai/src/crewai/utilities/reset_memories.py:63-139 |
| Correction API | `update(record_id, ...)` re-embeds new content; raises if record missing | lib/crewai/src/crewai/memory/unified_memory.py:852-896 |
| Agent-facing memory tools | `RecallMemoryTool` (limit 20/query) and `RememberTool`; Remember omitted when read-only | lib/crewai/src/crewai/tools/memory_tools.py:25-60, 75-101, 104-130 |
| Auto context injection | Task prompt augmented with top-5 recalled memories; standalone agent kickoff uses limit 20 | lib/crewai/src/crewai/agent/core.py:619-682, 1540-1580 |
| Lite-agent injection | Last user message used as query; matches appended to system message; output saved post-run | lib/crewai/src/crewai/lite_agent.py:599-660 |
| Memory opt-in wiring | `Agent(memory=True)` resolves to default `Memory()`; Crew builds embedder-aware instance | lib/crewai/src/crewai/lite_agent.py:403-417; lib/crewai/src/crewai/crew.py:665-680 |
| Observability | Save/query/retrieval started/completed/failed event types on the event bus | lib/crewai/src/crewai/events/types/memory_events.py:23-100 |
| Telemetry attribute | Span attribute `crew_memory` reports whether memory is enabled | lib/crewai/src/crewai/telemetry/telemetry.py:312-313 |
| Migration safeguard | `EmbeddingDimensionMismatchError` (not RuntimeError so background saves don't swallow it) with reset-or-pin remediation text | lib/crewai/src/crewai/memory/storage/backend.py:11-41 |
| Concurrency tests | Commit-conflict retries (5× exponential), file locks via `lock_store`, concurrent save/search tests | lib/crewai/src/crewai/memory/storage/lancedb_storage.py:34-39, 128-153; lib/crewai/tests/memory/test_concurrent_storage.py |
| Root-scope test suite | ~40 tests covering crew/flow/agent scoping, recall isolation, scoped reset | lib/crewai/tests/memory/test_memory_root_scope.py:384, 552, 617, 844, 1022, 1091, 1126 |
| Behavior tests | forget counts, slice read-only no-op, drain-on-recall, close semantics, dedup, consolidation | lib/crewai/tests/memory/test_unified_memory.py:155, 221, 283, 763-935, 1038-1095 |
| Docs match implementation | Memory doc documents unified model, scope tree conventions, slices, forget | docs/edge/en/concepts/memory.mdx:8-36, 161-320 |
| Inspection tooling | CLI TUI browses scopes/records and runs deep recall against the live store | lib/cli/src/crewai_cli/memory_tui.py:31, 124-135, 279-358 |

## Answers to Dimension Questions

**1. What persists across sessions?**
Three durable stores persist on local disk keyed by project directory: (a) the unified LanceDB/Qdrant memory table at `$CREWAI_STORAGE_DIR/memory` or the platform user-data dir (`lancedb_storage.py:67-74`, `qdrant_edge_storage.py:103-110`, `paths.py:16-26`); (b) latest kickoff task outputs in SQLite (`kickoff_task_outputs_storage.py:24-29`); (c) knowledge collections (separate system, out of scope here but reset alongside memory via `reset_memories_command`, `utilities/reset_memories.py:70-77`). Memory content itself is discrete fact statements extracted by the LLM from task/kickoff results (`analyze.py:155-197`), stored as embedded `MemoryRecord`s.

**2. Who can write memory?**
Any code holding a `Memory` instance: crews (auto-created when `memory=True`, `crew.py:665-680`), agents (`memory=True` → default instance, `lite_agent.py:403-417`), flows (auto-created, `flow/runtime/__init__.py:832-836`), and standalone users. At runtime, agents gain write ability only through the `RememberTool`, which is withheld when `read_only=True` (`memory_tools.py:104-130`). Automatic post-task saves skip delegation outputs and read-only memories (`base_agent_executor.py:31-41`). There is **no authentication or ACL layer**: whoever constructs the object can write wherever scopes allow.

**3. Who can read memory?**
Readers are bounded by scope views rather than identities: `MemoryScope` restricts to one subtree (`memory_scope.py:148-168`), `MemorySlice` merges several subtrees with optional read-only enforcement (`memory_scope.py:292-324`), and `root_scope` silently prefixes every recall/list/info call (`unified_memory.py:715-719`). The `private` flag hides records from recall callers whose `source` differs (`unified_memory.py:746-751`). Beyond that, any reader of the same backing store sees everything — e.g., the CLI TUI opens the raw LanceDB path and can recall across all scopes (`memory_tui.py:124-135`).

**4. Can memory be corrected?**
Yes, through three mechanisms: explicit `update(record_id, ...)` re-embeds corrected content (`unified_memory.py:852-896`); save-time consolidation lets the LLM update/delete overlapping stale records when similarity ≥ 0.85, with action de-duplication to avoid conflicting double-writes (`encoding_flow.py:224-347, 372-474`); and bulk correction via `forget()` filters or `reset(scope)` (`unified_memory.py:818-850, 1015-1029`). Correction quality is LLM-dependent: all analysis paths fall back to safe defaults (insert-new) on failure (`analyze.py:309-315, 369-375`), so wrong memories persist rather than get lost — a deliberate availability-over-accuracy tradeoff documented in code comments.

**5. Can memory become stale?**
Yes, by design it decays gracefully rather than expiring. Recency decay halves a record's recency contribution every 30 days (configurable, `types.py:175-183, 364-372`), so stale facts sink in ranking but never disappear automatically — there is no TTL; removal requires manual `forget(older_than=...)`. Two mitigations exist: consolidation merges superseded facts when near-duplicates are saved again (`encoding_flow.py:154-222, 344-347`), and LLM query analysis can apply temporal cutoffs from phrases like "last week" (`analyze.py:83-91`, `recall_flow.py:107-108, 222-228`). Notably, ranking uses `created_at`, not `last_accessed`, so frequently-recalled old facts still decay — `touch_records` maintains `last_accessed` (`lancedb_storage.py:339-359`) but nothing consumes it for scoring (no evidence found of `last_accessed` use in scoring; searched `compute_composite_score`, `recall_flow.py`, `unified_memory.py`). Embedder-model upgrades make old stores unreadable rather than stale; this is handled with an explicit migration error (`backend.py:11-41`, tested in `test_dimension_mismatch.py:28-166`).

## Architectural Decisions

1. **One unified memory instead of typed memories.** The legacy long/short/entity/external taxonomy was collapsed into a single `Memory` with LLM-inferred structure; CLI flags alias to `--memory` (`cli.py:430-480`, `crew.py:2294-2296`). This trades specialized retrieval behavior per memory type for one consistent, introspectable API.
2. **LLM-in-the-loop persistence.** Saves are not raw inserts: the EncodingFlow infers scope/categories/importance/metadata and runs consolidation planning via concurrent LLM calls grouped into fast/slow paths (`encoding_flow.py:224-310`). Writes therefore cost tokens and latency, mitigated by running them on a background single-worker pool (`unified_memory.py:297-322`) with a drain-before-read barrier (`unified_memory.py:711-713`).
3. **Hierarchical string scopes as the tenancy model.** Scopes are path strings with prefix search (`LIKE 'prefix%'`, `lancedb_storage.py:387-390`) and Qdrant ancestor arrays (`qdrant_edge_storage.py:65-78`). Crew/flow/agent namespaces are conventions layered on this mechanism (`crew.py:662-663`, `flow/runtime/__init__.py:836`, `base_agent_executor.py:56`) rather than enforced boundaries.
4. **Pluggable storage behind a runtime-checkable Protocol.** Backends are duck-typed (`storage/backend.py:44-212`) with a process-wide factory hook mirroring `set_lock_backend` (`factory.py:12-14, 33-45`), enabling remote-memory deployments without subclassing `Memory`.
5. **Failure isolation via events, not exceptions.** Background save failures emit `MemorySaveFailedEvent` and never fail the producing task/crew/flow (`unified_memory.py:324-363`); recall failures likewise degrade with safe-default query analyses (`analyze.py:244-256`).
6. **Local-first durability.** Default persistence is a project-keyed on-disk store (`paths.py:16-26`), with Qdrant Edge offering a write-local/sync-central pattern for multi-process workers flushed at exit (`qdrant_edge_storage.py:82-87, 143-144`).

## Notable Patterns

- **Flow-as-pipeline reuse:** both encoding and retrieval are implemented as internal CrewAI `Flow`s (`EncodingFlow`, `RecallFlow`) marked `_skip_auto_memory = True` to prevent recursive self-memory creation (`encoding_flow.py:85-87`, `recall_flow.py:66-68`, `flow/runtime/__init__.py:830-832`).
- **Oversample-then-rank:** vector searches fetch 2–3× candidates, then apply category/metadata filters and composite scoring before trimming (`types.py:12-17`, `lancedb_storage.py:391-393`, `recall_flow.py:100-106`).
- **Confidence-routed adaptive depth:** deep recall returns early when confidence ≥ 0.8, explores deeper (budgeted) below 0.5, with a separate threshold for complex queries (`recall_flow.py:273-291`).
- **Evidence-gap propagation:** failed deep explorations record "what's missing" onto returned matches (`recall_flow.py:305-333`, `types.py:87-90`) — an unusual honesty signal in retrieval APIs.
- **Migration-aware error typing:** the dimension-mismatch error deliberately subclasses `ValueError`, not `RuntimeError`, because the background-save plumbing treats `RuntimeError` as interpreter shutdown and would silently swallow it (`storage/backend.py:16-21`, regression-tested in `test_dimension_mismatch.py:138-166`).
- **Cross-process coordination:** file locks namespaced by resolved DB path (`lancedb:{path}`, `lancedb_storage.py:92`) plus optimistic-concurrency retries with exponential backoff (`lancedb_storage.py:34-39, 128-153`).

## Tradeoffs

- **Availability over accuracy on write:** every LLM-analysis failure falls back to persisting anyway with defaults (`analyze.py:159-162, 309-315`), and extraction failure stores the full raw blob as one memory (`analyze.py:191-197`). Memory never blocks agent work, but mis-scoped or bloated records accumulate.
- **Convention-based isolation:** `/crew/{name}` and `/customer/{id}` scoping depends on sanitized names (`memory/utils.py:8-36`) sharing one physical table; two crews with colliding names share a namespace, and nothing prevents an agent from recalling outside its root if handed the parent `Memory`.
- **Privacy is filter-only:** `private` records remain fully present in the store and serialization-visible to anyone reading the DB; filtering happens after fetch (`unified_memory.py:746-751`). No encryption-at-rest, redaction, or subject-based erasure.
- **Background-write complexity:** the single-worker pool, pending-future tracking, shutdown fallbacks, and event-bus teardown guards (`unified_memory.py:174-204, 297-363, 641-650`) buy non-blocking saves at the cost of subtle lifecycle behavior, much of which is defensively coded and explicitly tested (`test_unified_memory.py:957-1095`).
- **Scan-based administration:** scope listing/info/deletion do filtered full scans capped at 50k rows (`lancedb_storage.py:29-32, 466-492`) — simple and correct, but O(table) for large stores despite the BTREE scope index optimization attempt (`lancedb_storage.py:183-199`).

## Failure Modes / Edge Cases

- **Embedder drift across upgrades:** reopening a pre-upgrade store raises `EmbeddingDimensionMismatchError` on save/search/update with remediation guidance (`storage/backend.py:23-41`, `test_dimension_mismatch.py:87-128`). Mixed-dimension batches within one save also raise (`lancedb_storage.py:292-304`).
- **Lost saves during interpreter shutdown:** background saves racing process exit are silently abandoned when asyncio's executor is closed (`unified_memory.py:641-650`), and event emission is guarded against bus shutdown (`unified_memory.py:616-626, 652-664`); `close()`/`drain_writes()` exist but rely on callers (flows drain before `FlowFinishedEvent`, `flow/runtime/__init__.py:990-1004`).
- **Consolidation races between items:** overlapping similar-record sets could double-delete/double-update; actions are de-duplicated first-wins across batch items (`encoding_flow.py:380-412`).
- **Commit conflicts under concurrency:** LanceDB version advancement triggers up-to-5 retries with reopen-and-backoff (`lancedb_storage.py:128-153`), exercised by `test_concurrent_storage.py`.
- **Qdrant central-shard flush timing:** local shards flush to central on `close()` registered via `atexit` (`qdrant_edge_storage.py:143-144`); abnormal termination without `close()` risks losing local-only writes (no WAL evidence found for edge shards).
- **Naive UTC timestamps:** `datetime.utcnow()` throughout (`types.py:46-53`, `unified_memory.py:880`, `lancedb_storage.py:255-256`) — timezone-less comparisons and Python deprecation risk; parsed rows assume ISO strings (`lancedb_storage.py:265-271`).
- **Injection surface:** recalled memory content is concatenated directly into prompts/system messages (`lite_agent.py:616-625`, `agent/core.py:652-657`); no sanitization layer between stored text and prompt context.

## Future Considerations

- Add identity-bound scopes (e.g., first-class `user:`/`org:` namespace segments with enforcement in `join_scope_paths` and recall defaults) instead of documentation-level conventions (`docs/edge/en/concepts/memory.mdx:250-320`).
- Strengthen privacy: encrypt or tokenize `private` records at rest, add subject-based erasure (delete-all-for-source), and cover the private-filter path with dedicated tests (currently only incidental coverage found — `test_qdrant_edge_storage.py:275`).
- Use `last_accessed` in scoring or TTL policies so frequently-useful old facts aren't purely decay-penalized (`lancedb_storage.py:339-359` currently writes a field nothing reads for ranking).
- Replace scan-based `get_scope_info`/`list_scopes`/`list_categories` with indexed aggregates to stay healthy past the 50k-row cap (`lancedb_storage.py:466-492`).
- Make consolidation outcomes observable (record which memories were merged/deleted and why) beyond the transient flow state counters (`encoding_flow.py:64-72`).

## Questions / Gaps

- **No evidence found for cross-session user identity:** searched `source`, `user`, `identity` across `lib/crewai/src/crewai/memory/` — `source` is free-text provenance with no authn binding or validation (`types.py:60-66`). Any caller can claim any source.
- **No evidence found for organization/global memory tiers:** scope taxonomy examples mention `/company/*` (`docs/edge/en/concepts/memory.mdx:244-248`) but no code distinguishes org-level storage from ordinary scopes.
- **Private-flag recall behavior is untested:** no test in `lib/crewai/tests/memory/` asserts `private=True` filtering on `recall` (searched `private|include_private` across the six memory test modules; only a payload literal matched). The enforcement code exists (`unified_memory.py:746-751`, `recall_flow.py:109-114`) but its guarantee is unverified by CI.
- **Scale/failure evidence is thin:** concurrency is covered (`test_concurrent_storage.py`) but there are no benchmarks or tests at row counts approaching `_SCAN_ROWS_LIMIT`, and no chaos tests for Qdrant central-flush loss scenarios.
- **Knowledge vs memory boundary** (both persist locally and are resettable) was noted but not analyzed; it belongs to a separate knowledge dimension.

---

Generated by `05.03-long-term-user-project-and-domain-memory` against `crewai`.

# Source Analysis: crewai

## Dimension 05.04: Retrieval-Augmented Memory

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (Pydantic, LanceDB, Qdrant Edge, ChromaDB, Qdrant client) |
| Analyzed | 2026-08-25 |

All citations below are relative to the source root `studies/agent-harness-study/sources/crewai/`.

## Summary

CrewAI implements retrieval-augmented memory as **two parallel stacks plus a shared RAG infrastructure layer**:

1. **Unified Memory** (`lib/crewai/src/crewai/memory/unified_memory.py:76`) — an LLM-analyzed episodic memory store with vector search over pluggable backends (LanceDB default, `unified_memory.py:242-249`; Qdrant Edge alternative, `unified_memory.py:238-241`). Writes go through a batch-native `EncodingFlow` (`lib/crewai/src/crewai/memory/encoding_flow.py:75`) that embeds in one call, dedups intra-batch, and runs LLM consolidation; reads go through either a direct vector search ("shallow") or an LLM-orchestrated adaptive-depth `RecallFlow` (`lib/crewai/src/crewai/memory/recall_flow.py:58`) that distills queries into sub-queries, searches candidate scopes in parallel, and routes on confidence.

2. **Knowledge** (`lib/crewai/src/crewai/knowledge/knowledge.py:88`) — document RAG over strings and files (PDF, CSV, Excel, JSON, text, Docling) with naive fixed-size chunking (`lib/crewai/src/crewai/knowledge/source/base_knowledge_source.py:19-20,43-48`) persisted to ChromaDB via the shared RAG client (`lib/crewai/src/crewai/knowledge/storage/knowledge_storage.py:105-136`). Retrieved snippets are appended verbatim into the task prompt (`lib/crewai/src/crewai/agent/utils.py:119-198`).

3. **Shared `rag` package** (`lib/crewai/src/crewai/rag/`) — a `BaseClient` protocol with ChromaDB (`lib/crewai/src/crewai/rag/chromadb/client.py:39`) and Qdrant (`lib/crewai/src/crewai/rag/qdrant/client.py:32`) implementations plus an embedding factory registering ~18 providers (`lib/crewai/src/crewai/rag/embeddings/factory.py:90-110`).

Retrieval is scoped by hierarchical scope paths (memory), collection names (knowledge), and privacy filters on memory records. Provenance is strong for memory records (`source`/`private` fields, `lib/crewai/src/crewai/memory/types.py:60-73`) but **absent for knowledge chunks**, which are stored as bare content strings with no origin metadata. There is no dedicated reranker model anywhere in the core pipeline; ranking is a weighted composite of semantic similarity, recency decay, and importance (`lib/crewai/src/crewai/memory/types.py:345-380`). Staleness is handled by LLM-driven consolidation at write time rather than TTL/expiry at read time.

## Rating

**7 / 10** — The unified memory stack matches the "clear model with tests, explicit interfaces, and operational safeguards" band precisely: a runtime-checkable `StorageBackend` protocol (`lib/crewai/src/crewai/memory/storage/backend.py:44-212`), extensive behavioral tests (`lib/crewai/tests/memory/test_unified_memory.py`, >1,100 lines), concurrency safeguards (file locks, commit-conflict retries, background-save drain barriers), and event-bus observability. It falls short of 8+ because the knowledge/document RAG path is comparatively weak (naive character chunking, no source provenance or citation of retrieved chunks, silent exception swallowing), there is no reranker beyond score-based re-sorting, and the memory vs. knowledge stacks duplicate storage concerns across two different vector-store integrations without a unified retrieval interface.

## Evidence Collected

Every entry cites file paths relative to `studies/agent-harness-study/sources/crewai/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Retriever (memory, shallow/deep) | `Memory.recall()` with `depth="shallow"` (direct vector search) or `"deep"` (RecallFlow); read barrier drains pending writes first | lib/crewai/src/crewai/memory/unified_memory.py:681-782 |
| Retriever (adaptive recall flow) | LLM query distillation → parallel multi-scope search → confidence router → exploration loop → synthesis | lib/crewai/src/crewai/memory/recall_flow.py:58-380 |
| Retriever (knowledge) | `Knowledge.query()` delegates to `KnowledgeStorage.search()` against ChromaDB | lib/crewai/src/crewai/knowledge/knowledge.py:135-152; lib/crewai/src/crewai/knowledge/storage/knowledge_storage.py:59-89 |
| Retriever (agent tool) | `RecallMemoryTool` exposes multi-query memory search to agents; omitted when read-only | lib/crewai/src/crewai/tools/memory_tools.py:25-60,104-130 |
| Retriever (flow API) | `Flow.recall()/remember()` delegate to the flow's Memory instance | lib/crewai/src/crewai/flow/runtime/__init__.py:944-988 |
| Vector store (memory) | `LanceDBStorage`: ANN search, scope-prefix LIKE filter, category/metadata post-filters, oversampled fetch | lib/crewai/src/crewai/memory/storage/lancedb_storage.py:371-409 |
| Vector store (memory, alt) | `QdrantEdgeStorage`: write-local/sync-central shards, reads merge local + central, ancestor-scope filters | lib/crewai/src/crewai/memory/storage/qdrant_edge_storage.py:1-7,65-78,314-354 |
| Vector store (knowledge) | `ChromaDBClient.search()` with metadata filter, score threshold, HNSW cosine space | lib/crewai/src/crewai/rag/chromadb/client.py:134-137,410-471 |
| Indexing pipeline (knowledge) | Chunked docs upserted in batches of 100 with auto-generated IDs | lib/crewai/src/crewai/rag/chromadb/client.py:309-358 |
| Indexing pipeline (memory) | `EncodingFlow`: batch embed → intra-batch cosine dedup → parallel find-similar → parallel LLM analysis → batched writes | lib/crewai/src/crewai/memory/encoding_flow.py:112-501 |
| Chunker | Fixed-size sliding window: `chunk_size=4000`, `chunk_overlap=200`, pure character slicing | lib/crewai/src/crewai/knowledge/source/base_knowledge_source.py:19-20,43-48; lib/crewai/src/crewai/knowledge/source/string_knowledge_source.py:36-41 |
| Embedding config | Default OpenAI embedder (`text-embedding-3-large`, 3072-dim default); dim auto-detected from table schema or first embedding | lib/crewai/src/crewai/memory/unified_memory.py:52-55; lib/crewai/src/crewai/memory/storage/lancedb_storage.py:24-27,94-114 |
| Embedding providers | Factory registry mapping ~18 provider names (openai, google, vertex, bedrock, cohere, ollama, voyageai, watsonx, sentence-transformer, …) to provider classes | lib/crewai/src/crewai/rag/embeddings/factory.py:90-110,352-375 |
| Batch embedding helper | `embed_texts()` single-call batch embedding used by both flows | lib/crewai/src/crewai/memory/types.py:289-342 |
| Retrieval filters (scope) | Scope prefix `scope LIKE 'prefix%'` SQL filter; BTREE scalar index maintained for the hot path | lib/crewai/src/crewai/memory/storage/lancedb_storage.py:183-199,387-390 |
| Retrieval filters (categories/metadata) | Post-search filtering with `limit*3` oversampling so filters don't starve results | lib/crewai/src/crewai/memory/storage/lancedb_storage.py:391-402 |
| Retrieval filters (privacy) | Private records visible only when caller's `source` matches record's `source`, unless `include_private=True` | lib/crewai/src/crewai/memory/unified_memory.py:746-751; lib/crewai/src/crewai/memory/recall_flow.py:109-114; field defs lib/crewai/src/crewai/memory/types.py:60-73 |
| Retrieval filters (temporal) | LLM extracts ISO date from temporal hints ("last week") into a `time_cutoff` post-filter | lib/crewai/src/crewai/memory/analyze.py:83-91; lib/crewai/src/crewai/memory/recall_flow.py:107-108,222-228 |
| Score threshold | ChromaDB results filtered by `score_threshold` (default 0.6); distance→score conversion per metric | lib/crewai/src/crewai/rag/chromadb/utils.py:167-189,239-240; defaults lib/crewai/src/crewai/rag/chromadb/client.py:56-57 |
| Reranking (composite scoring) | No reranker model; composite = `w_sem*sim + w_rec*decay + w_imp*importance` with exponential recency half-life | lib/crewai/src/crewai/memory/types.py:144-146,175-183,345-380 |
| Reranking (multi-scope merge) | `MemorySlice.recall()` merges per-scope results and re-ranks by composite score with dedup | lib/crewai/src/crewai/memory/memory_scope.py:292-324 |
| Reranking (optional tool) | ContextualAI rerank exists only as an agent-invocable tool in crewai-tools, not wired into retrieval pipeline | lib/crewai-tools/src/crewai_tools/tools/contextualai_rerank_tool/contextual_rerank_tool.py:22-81 |
| Source citations (provenance, memory) | `MemoryRecord.source` documented as "provenance tracking"; `MemoryMatch.format()` surfaces content/categories/metadata but not source path | lib/crewai/src/crewai/memory/types.py:60-73,92-106 |
| Source citations (provenance, knowledge) | Chunks saved as bare `[{"content": doc}]`; source-level `metadata` field marked "Currently unused" — no origin preserved | lib/crewai/src/crewai/knowledge/storage/knowledge_storage.py:118; lib/crewai/src/crewai/knowledge/source/base_knowledge_source.py:28 |
| Context injection | Knowledge snippets appended to task prompt; memory block injected into system message / kickoff messages | lib/crewai/src/crewai/agent/utils.py:154-177; lib/crewai/src/crewai/lite_agent.py:600-643; lib/crewai/src/crewai/agent/core.py:1540-1580 |
| Query rewriting | Agent LLM rewrites task prompt into a knowledge search query before retrieval | lib/crewai/src/crewai/agent/core.py:1364-1417 |
| Consolidation (staleness) | On save, similarity ≥ 0.85 triggers LLM merge/update/delete plan; actions deduped before execution | lib/crewai/src/crewai/memory/encoding_flow.py:263-310,371-501; threshold config lib/crewai/src/crewai/memory/types.py:185-202 |
| Recency maintenance | `touch_records()` updates `last_accessed` after successful recall | lib/crewai/src/crewai/memory/unified_memory.py:784-790; lib/crewai/src/crewai/memory/storage/lancedb_storage.py:339-359 |
| Operational safeguards | Cross-process locks, optimistic-concurrency retries with exponential backoff, background compaction every N saves, 50k scan cap, read barrier (`drain_writes()`) | lib/crewai/src/crewai/memory/storage/lancedb_storage.py:29-39,128-153,201-231,314-317; lib/crewai/src/crewai/memory/unified_memory.py:297-370 |
| Embedder-drift detection | `EmbeddingDimensionMismatchError` with actionable migration message when store dims ≠ embedder dims | lib/crewai/src/crewai/memory/storage/backend.py:11-41 |
| Observability | `MemoryQueryStarted/Completed/FailedEvent`, `MemorySaveStarted/Completed/FailedEvent`, knowledge retrieval events with timings | lib/crewai/src/crewai/memory/unified_memory.py:474-521,723-816; lib/crewai/src/crewai/agent/utils.py:146-197 |
| Tests (memory) | save/search roundtrip, composite-score re-ranking, dedup, drain-on-recall, background-failure reporting, LLM-failure fallbacks | lib/crewai/tests/memory/test_unified_memory.py:131,520,763,1038,1059,635,653 |
| Tests (dimension drift) | Mismatch raised on save/search/update/reopen; error deliberately not RuntimeError so background saves surface it | lib/crewai/tests/memory/test_dimension_mismatch.py:28-166,138 |
| Tests (qdrant edge) | Save/search, scope prefix filter, category filter, metadata filter | lib/crewai/tests/memory/test_qdrant_edge_storage.py:57,105,120,132 |
| Tests (knowledge) | Chunking behavior for strings/files/PDF/CSV/JSON/Excel, hash-based doc-id generation | lib/crewai/tests/knowledge/test_knowledge.py:37,396,567 |
| Tests (chromadb client) | Client search/add/reset behaviors | lib/crewai/tests/rag/chromadb/test_client.py |

## Answers to Dimension Questions

**1. What can be retrieved?**
Three classes of content: (a) episodic memories extracted from agent/task outputs — the executor extracts discrete statements from "Task/Agent/Expected result/Result" dumps and stores them (`lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:31-63`; same pattern for lite agents at `lib/crewai/src/crewai/lite_agent.py:645-654`); (b) user-curated documents via `Knowledge` sources — string, text file, PDF, CSV, JSON, Excel, Docling (`lib/crewai/src/crewai/knowledge/knowledge.py:23-31`); (c) anything an agent explicitly saves through the `RememberTool` (`lib/crewai/src/crewai/tools/memory_tools.py:75-101`). Prior conversation traces are *not* stored verbatim as retrievable messages — only LLM-extracted memory statements from them.

**2. How is it indexed?**
Memories: each statement is embedded (batched via `embed_texts`, `lib/crewai/src/crewai/memory/types.py:312-342`) and stored as a flat row/vector in LanceDB (or Qdrant Edge point), one table/collection for all scopes; scope hierarchy is a string column filtered by LIKE/prefix, not separate namespaces (`lib/crewai/src/crewai/memory/storage/lancedb_storage.py:247-262,387-390`). Knowledge: documents are split by fixed-size sliding window (4000 chars / 200 overlap), each chunk upserted into a ChromaDB collection named `knowledge_{collection_name}` with SHA256(content)-derived IDs enabling idempotent re-adds (`lib/crewai/src/crewai/rag/chromadb/client.py:349-358`; `lib/crewai/src/crewai/rag/chromadb/utils.py:81-85`). Embeddings default to OpenAI text-embedding-3-large (3072-dim) with auto-detected dimensionality on reopen (`lib/crewai/src/crewai/memory/storage/lancedb_storage.py:24-27,94-126`).

**3. Are retrieval results scoped correctly?**
Largely yes, with layered enforcement: hierarchical scope prefixes constrain both write-side consolidation search and read-side recall (`lib/crewai/src/crewai/memory/encoding_flow.py:170-185`; `lib/crewai/src/crewai/memory/unified_memory.py:715-719`), with `root_scope` nesting so crews/agents can't escape their subtree (`lib/crewai/src/crewai/memory/unified_memory.py:152-159`). Privacy scoping is explicit: private records require matching `source` or `include_private=True` (`lib/crewai/src/crewai/memory/recall_flow.py:109-114`). Knowledge scoping is coarser — collections partition agent-knowledge from crew-knowledge only by collection name (`knowledge` vs `knowledge_{name}`, `lib/crewai/src/crewai/knowledge/storage/knowledge_storage.py:71-75`), and the deep-recall LLM can suggest any of up to 20 candidate scopes listed from storage, which is a soft boundary dependent on prompt compliance (`lib/crewai/src/crewai/memory/recall_flow.py:206-220,264`). Caveat: the LanceDB scope filter is a substring/LIKE predicate on a column, so correctness relies on consistent scope-path formatting (`join_scope_paths`, `lib/crewai/src/crewai/memory/utils.py`).

**4. Are sources preserved?**
Split answer. For **memory records**: yes — `source` and `private` fields capture origin (user/session ID) and drive visibility filtering (`lib/crewai/src/crewai/memory/types.py:60-73`); human-feedback learning uses `source` to tag lessons (`lib/crewai/src/crewai/flow/human_feedback.py:261,346`). However, provenance granularity stops there: no pointer to which task/run produced the memory beyond what fits in free-form `metadata`. For **knowledge chunks**: no — chunks are saved as bare content strings (`lib/crewai/src/crewai/knowledge/storage/knowledge_storage.py:118`), the source-level `metadata` field is explicitly "Currently unused" (`lib/crewai/src/crewai/knowledge/source/base_knowledge_source.py:28`), and context extraction discards id/score/metadata, keeping only raw text joined into "Additional Information: …" (`lib/crewai/src/crewai/knowledge/utils/knowledge_utils.py:4-12`). An agent consuming knowledge snippets therefore cannot cite which file or page a fact came from.

**5. Can stale or low-quality retrieval be detected?**
Partially, by construction rather than inspection. Low quality is attacked at write time: near-exact duplicates within a batch are dropped at cosine ≥ 0.98 (`lib/crewai/src/crewai/memory/encoding_flow.py:121-140`), and overlapping records trigger LLM consolidation (merge/update/delete) above 0.85 similarity (`lib/crewai/src/crewai/memory/encoding_flow.py:272-310`). At read time, confidence-based routing detects weak retrieval — below thresholds it triggers deeper exploration and records `evidence_gaps` ("what's missing" lines propagated onto the top match, `lib/crewai/src/crewai/memory/recall_flow.py:273-343,377-378`). Recency decay de-prioritizes stale records numerically but nothing expires them (`lib/crewai/src/crewai/memory/types.py:364-372`). Embedder-model drift is detected loudly via `EmbeddingDimensionMismatchError` (`lib/crewai/src/crewai/memory/storage/backend.py:11-41`). What's missing: no retrieval-quality metrics, no hit-rate feedback loop, no way to detect a poisoned/incorrect memory once it passes consolidation, and knowledge search silently swallows all exceptions returning `[]` (`lib/crewai/src/crewai/knowledge/storage/knowledge_storage.py:85-89`), making empty-but-broken indistinguishable from legitimately-empty.

## Architectural Decisions

1. **Two stacks, not one.** Memory (LanceDB/Qdrant Edge, scope trees, LLM-in-the-loop encode/decode) and Knowledge (ChromaDB/Qdrant clients, collections) never share code despite overlapping needs. The memory stack predates/generalizes poorly onto documents; the knowledge stack lacks memory's provenance and scoring. Decision visible in directory layout: `lib/crewai/src/crewai/memory/` vs `lib/crewai/src/crewai/knowledge/` vs shared `lib/crewai/src/crewai/rag/`.

2. **Flows as retrieval pipelines.** Both encoding and recall are implemented as CrewAI `Flow` state machines with typed Pydantic state (`EncodingState`, `RecallState`) — `lib/crewai/src/crewai/memory/encoding_flow.py:64-89`, `lib/crewai/src/crewai/memory/recall_flow.py:37-70`. This gives stepwise observability but couples core memory to the Flow framework (both mark `_skip_auto_memory = True` to avoid recursion, `recall_flow.py:68`, `encoding_flow.py:87`).

3. **LLM in the hot path, gated by budgets.** Deep recall spends LLM calls on query distillation and optional exploration rounds bounded by `exploration_budget` (default 1) and skips analysis entirely for short queries (`lib/crewai/src/crewai/memory/recall_flow.py:192-204`; `lib/crewai/src/crewai/memory/unified_memory.py:140-147`). Encoding resolves scope/category/importance via structured LLM output (`QueryAnalysis`/`MemoryAnalysis` schemas, `lib/crewai/src/crewai/memory/analyze.py:37-91`) with graceful degradation to defaults on LLM failure (`lib/crewai/tests/memory/test_unified_memory.py:602-676`).

4. **Composite scoring instead of a reranker.** Ranking fuses semantic similarity, exponential recency decay (30-day half-life), and importance weights — all user-tunable (`lib/crewai/src/crewai/memory/unified_memory.py:100-115`). This is cheap and explainable (match reasons returned, `types.py:374-378`) but cannot fix semantic-search misses the way a cross-encoder would.

5. **Background saves with a read barrier.** `remember_many()` returns immediately; saves run on a single-worker pool and `recall()` blocks until pending writes complete, preventing read-your-writes anomalies (`lib/crewai/src/crewai/memory/unified_memory.py:297-370,711-713`). Failures surface as events, not exceptions, so memory problems don't fail tasks (`lib/crewai/tests/memory/test_unified_memory.py:1059`).

## Notable Patterns

- **Oversample-then-filter**: fetch `limit * _RECALL_OVERSAMPLE_FACTOR` (×2 in RecallFlow, ×3 in LanceDB when filters active) candidates, then apply category/metadata/privacy/time filters and trim — avoids ANN-under-filter starvation (`lib/crewai/src/crewai/memory/types.py:12-17`; `lancedb_storage.py:391-393`; `recall_flow.py:100-115`).
- **Evidence-gap tracking**: deep recall asks the LLM what's missing from retrieved excerpts and attaches gaps to the top `MemoryMatch` — an unusual, honest signal that retrieval was insufficient (`lib/crewai/src/crewai/memory/recall_flow.py:305-332,377-378`; field at `lib/crewai/src/crewai/memory/types.py:87-90`).
- **Content-hash IDs for idempotent indexing**: SHA256(content|metadata) doc IDs make knowledge re-ingestion an upsert rather than a duplicate generator (`lib/crewai/src/crewai/rag/chromadb/utils.py:74-105`).
- **Write-local/sync-central sharding** for multi-process safety in Qdrant Edge: each PID writes its own shard, reads fan out and merge, close() flushes to central (`lib/crewai/src/crewai/memory/storage/qdrant_edge_storage.py:1-7`).
- **Actionable failure taxonomy**: dimension-mismatch errors name the exact old/new embedder models and give two remediation commands; the class deliberately subclasses `ValueError` not `RuntimeError` because the background-save path interprets RuntimeError as interpreter shutdown and would swallow it (`lib/crewai/src/crewai/memory/storage/backend.py:11-41`; test at `lib/crewai/tests/memory/test_dimension_mismatch.py:138`).
- **Prompt injection of retrieval results**: knowledge goes into the task prompt tail; memory goes into the system message under an i18n "memory" template — retrieval is ambient context, never cited inline (`lib/crewai/src/crewai/agent/utils.py:167,177`; `lib/crewai/src/crewai/lite_agent.py:620-625`).

## Tradeoffs

- **Simplicity of chunking vs. retrieval quality**: fixed 4000-char character slicing ignores sentence/paragraph boundaries and token limits, and applies the same strategy to CSV rows, JSON, and PDF pages alike (`lib/crewai/src/crewai/knowledge/source/base_knowledge_source.py:43-48`, `pdf_knowledge_source.py:58-63`). Cheap and predictable; poor boundary preservation for structured formats.
- **LLM-in-loop richness vs. latency/cost**: deep recall can cost multiple LLM calls (analysis + N explorations) per query; mitigated by short-query bypass and budget=1 default (`lib/crewai/src/crewai/memory/recall_flow.py:192-204`), but the fast path still requires an embedder and the slow path requires a working LLM.
- **Flat vector table + LIKE scoping vs. namespace isolation**: one table keeps operations simple (reset by prefix range scan, `lancedb_storage.py:603-616`) but means a formatting bug in scope strings leaks records across logical boundaries.
- **Silent-degradation philosophy vs. debuggability**: knowledge search returns `[]` on any error and memory background saves emit events instead of raising — good for agent uptime, bad for noticing broken embedding credentials early (`knowledge_storage.py:85-89`; contrast: memory failures do emit `MemorySaveFailedEvent`, `unified_memory.py:337-348`).
- **Consolidation-at-write vs. garbage collection**: merging similar records keeps the store tidy but trusts the LLM's delete decisions; a bad consolidation irreversibly drops information (delete actions executed directly, `encoding_flow.py:452-455`) with no soft-delete/tombstone mechanism.

## Failure Modes / Edge Cases

- **No knowledge provenance → uncitable answers**: since chunk origin is dropped at index time (`knowledge_storage.py:118`), hallucinated facts cannot be traced to a source document even by the framework itself.
- **Multi-query knowledge search mangles semantics**: multiple query strings are concatenated with spaces into one query (`" ".join(query)`, `knowledge_storage.py:76`) rather than searched separately — distinct intents blur into one embedding.
- **Zero-vector placeholders**: records saved without embeddings get zero vectors inserted (`lancedb_storage.py:308-310`), which sit in the table and could surface in scans; dimension validation prevents cross-model corruption but not this degenerate case.
- **String-built filter expressions**: WHERE clauses are f-assembled SQL with only single-quote escaping (`safe_id = str(record.id).replace("'", "''")`, `lancedb_storage.py:332,365,390`); scope names come from LLM inference (`analyze_for_save` suggests scopes, `analyze.py:40-42`), so adversarial/odd content in scope paths is only lightly sanitized (`sanitize_scope_name` applied to agent roles, `base_agent_executor.py:55`).
- **Deep-recall scope suggestion is prompt-bounded**: the LLM chooses among `list_scopes()` output capped at 20 candidates (`recall_flow.py:264`); with more than 20 scopes, relevant ones may be invisible to the planner.
- **Score-scale mismatch between stacks**: LanceDB maps distance via `1/(1+d)` (`lancedb_storage.py:403-405`) while ChromaDB cosine uses `1 - 0.5*d` assuming unit-normalized embeddings (`chromadb/utils.py:174,183-185`) — thresholds tuned on one backend don't transfer to the other.
- **Shutdown races handled explicitly but narrowly**: encoding detects "cannot schedule new futures" during process exit and abandons the save silently (`unified_memory.py:641-650`); event emission wrapped against bus shutdown (`recall_flow.py` n/a, `unified_memory.py:616-626,652-664`).
- **Recursion guard**: internal flows suppress auto-memory to prevent memory-about-memory loops (`_skip_auto_memory`, `recall_flow.py:68`, `encoding_flow.py:87`) — a subtle invariant that new callers must respect.

## Future Considerations

1. **Preserve knowledge provenance end-to-end**: thread `source_id`/file path/page through `BaseKnowledgeSource.add()` → `BaseRecord.metadata` → `SearchResult` → `extract_knowledge_context()`. All extension points already exist (`BaseRecord.metadata`, `rag/types.py:21-24`; unused source `metadata` field, `base_knowledge_source.py:28`); only plumbing is missing.
2. **Add a real reranking stage**: the composite score reorders but cannot rescue semantic misses. A cross-encoder pass over the oversampled candidate set (already fetched at ×2–×3) would fit the existing pipeline shape; the ContextualAI tool shows demand but lives outside the pipeline (`contextual_rerank_tool.py:22-81`).
3. **Per-query knowledge search**: replace the space-join of multiple queries with independent searches merged like `MemorySlice.recall()` does (`knowledge_storage.py:76` vs `memory_scope.py:292-324`).
4. **Unify memory and knowledge behind one retrieval interface**: a shared `StorageBackend`-style protocol for document stores would let knowledge gain memory's consolidation, scoring, and privacy machinery.
5. **Retrieval-quality telemetry**: aggregate `MemoryQueryCompletedEvent` scores/hit rates (`unified_memory.py:793-803`) into dashboards; currently events exist but no built-in consumer computes quality trends.
6. **Soft deletes for consolidation**: tombstones instead of hard `storage.delete(record_ids=...)` (`encoding_flow.py:452-455`) would make LLM consolidation mistakes recoverable.

## Questions / Gaps

- **How does the global RAG client interact with per-instance configs in practice?** `KnowledgeStorage._get_client()` prefers instance config over the contextvar-global client (`knowledge_storage.py:55-57`; global setup at `rag/config/utils.py:62-81`). No evidence found of lifecycle documentation for mixing both; searched `docs/edge/en` knowledge pages and `tests/knowledge/test_storage_factory.py` — coverage exists for resolution order but not for concurrent-mix behavior.
- **Is Qdrant (`rag/qdrant/`) actually used by any default path?** The `BaseClient` implementation and factory exist (`rag/qdrant/factory.py:9-20`), and `RagConfigType` accepts it (`rag/config/types.py:15-27`), but the shipped default config constant points at ChromaDB (`DEFAULT_RAG_CONFIG_CLASS`, `rag/config/constants.py`). No evidence found of knowledge storage defaulting to Qdrant; it appears opt-in only.
- **What happens to consolidated-away memories' provenance?** When consolidation updates a record, the new record inherits the existing record's scope/categories/metadata (`encoding_flow.py:461-471`) but the contributing item's own `source` is not merged — potential privacy leak if a private-source item triggers a public-record update. No test found covering this combination (searched `test_unified_memory.py` consolidation tests at lines 821-956).
- **Chunk-size rationale**: no evidence found (comments, docs, or tests) justifying the 4000/200 defaults relative to common embedding-context windows; values appear inherited rather than measured.
- **Legacy memory migration**: the classic ShortTerm/LongTerm/Entity/User memory classes are absent from this tree (searched `class ShortTermMemory|LongTermMemory|EntityMemory|UserMemory` across `src/` — no matches), implying this snapshot post-dates the unification; migration guidance for existing stores exists only for the embedder-dimension case (`backend.py:11-41`).

---

Generated by `05.04-retrieval-augmented-memory` against `crewai`.

# Source Analysis: letta

## Dimension 05.04 — Retrieval-Augmented Memory

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI server, SQLAlchemy + Postgres/pgvector, Turbopuffer, Pinecone, LlamaIndex chunking) |
| Analyzed | 2026-08-25 |

> Citation convention: all paths below are workspace-relative under the source root `studies/agent-harness-study/sources/letta/` (e.g., `letta/helpers/tpuf_client.py:1489` = `studies/agent-harness-study/sources/letta/letta/helpers/tpuf_client.py:1489`).

## Summary

Letta implements retrieval-augmented memory as a first-class subsystem with four distinct retrievable corpora: (1) **archival memory** passages (agentic long-term memory), (2) **conversation history** messages, (3) **file/source passages** (documents attached via folders/sources), and (4) **tools** (embedded tool descriptions). Retrieval is exposed to the agent itself as callable tools (`conversation_search`, `archival_memory_search`, file search) and to developers via REST endpoints.

The storage layer is pluggable across three backends selected per-archive or by settings flags: **Turbopuffer** (primary cloud vector DB, `letta/helpers/tpuf_client.py:223`), **Postgres pgvector / SQLite** ("NATIVE", `letta/orm/passage.py:34-40`, `letta/services/helpers/agent_manager_helper.py:1242-1258`), and **Pinecone** for legacy source-file search (`letta/helpers/pinecone_utils.py:273`). Turbopuffer supports four search modes — vector ANN, BM25 full-text, hybrid multi-query, and timestamp-recency fallback (`letta/helpers/tpuf_client.py:791-906`) — fused client-side with a textbook Reciprocal Rank Fusion (k=60, weighted) that returns per-result rank metadata (`letta/helpers/tpuf_client.py:1489-1559`).

The indexing pipeline for documents is parse (Mistral OCR / markitdown) → type-aware chunking (LlamaIndex CodeSplitter/MarkdownNodeParser/HTMLNodeParser/JSONNodeParser/SentenceSplitter with layered fallbacks) → batched embedding with token-limit split retries and global rate limiting → dual-write to SQL plus the vector store (`letta/services/file_processor/file_processor.py:157-300`). Agentic writes go through `PassageManager.insert_passage`, which writes Postgres first and then mirrors to Turbopuffer with matching IDs, treating TPUF failures as non-fatal unless strict mode (`letta/services/passage_manager.py:586-632`).

Provenance is structurally preserved: every passage carries `archive_id`, `source_id`/`file_id`, `file_name`, tags, timestamps and organization scoping (`letta/schemas/passage.py:14-32`, `letta/orm/passage.py:48-73`), and search results expose relevance metadata (`rrf_score`, `vector_rank`, `fts_rank`, `search_mode`) so both the model and callers can see *how* a result was found (`letta/services/agent_manager.py:2651-2663`, `letta/services/tool_executor/core_tool_executor.py:231-246`). The main gaps are a weak reranking story outside Pinecone (no cross-encoder on the TPUF/NATIVE paths), tag filtering applied after `LIMIT` on the SQL path, dropped message IDs in `conversation_search` output, and a comment/code mismatch around soft-deleted messages in TPUF search.

## Rating

**8 / 10**

Rationale against the rubric:

- **Clear model with explicit interfaces (7-8 bar met):** retrieval is organized into named managers (`PassageManager`, `MessageManager`, `AgentManager.search_agent_archival_memory_async`) behind one vector-client abstraction (`TurbopufferClient`), with declared search modes (`letta/helpers/tpuf_client.py:830-831`), typed filters, and an `EmbeddingConfig` schema (`letta/schemas/embedding_config.py:8-37`).
- **Tests:** extensive coverage including real-backend integration tests gated on API keys — RRF unit test (`tests/integration_test_turbopuffer.py:984-1080`), hybrid search (:390), tag filtering (:496), temporal filtering (:623), SQL fallback (:1342), and failure isolation proving "Turbopuffer failure does not break Postgres" (:1586); manager-level tests cover pagination, text/vector/date search and tags with TPUF disabled (`tests/managers/test_passage_manager.py:96-285`).
- **Operational safeguards:** dual-write with graceful degradation, retry-with-backoff on writes (`letta/helpers/tpuf_client.py:79-160`), transient-error classification (:37), Redis-cached query embeddings (`letta/services/passage_manager.py:34-40`), global embedding semaphore (`letta/services/file_processor/embedder/openai_embedder.py:20`).
- **Why not 9-10:** no reranker on primary paths; SQL fallback has no scoring/hybrid and applies tag filters post-`LIMIT` (`letta/services/agent_manager.py:2494-2527`); soft-delete handling in TPUF message search is documented but not visibly implemented (`letta/helpers/tpuf_client.py:1052-1055` vs `letta/services/message_manager.py:1201-1216`); three parallel agent-loop implementations duplicate retrieval-tool plumbing (`letta/agents/letta_agent.py:1823`, `letta_agent_v2.py:1184`, `letta_agent_v3.py:1878`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Retriever tools exposed to agent | `conversation_search`, `archival_memory_search`, `archival_memory_insert` wired into core tool executor map | `letta/services/tool_executor/core_tool_executor.py:43-45` |
| Conversation search implementation | Hybrid search over messages w/ role+date filters, recursive-result suppression | `letta/services/tool_executor/core_tool_executor.py:81-276` |
| Archival search implementation | Delegates to shared service method used by tool and REST API | `letta/services/tool_executor/core_tool_executor.py:278-305`; `letta/server/rest_api/routers/v1/agents.py:1510-1551` |
| Vector store: Turbopuffer client | `TurbopufferClient` class managing archival/messages/tools/file-passage namespaces | `letta/helpers/tpuf_client.py:223-232` |
| Feature gates | `use_tpuf`, `tpuf_api_key`, `embed_all_messages`, `embed_tools` flags and gate functions | `letta/settings.py:442-446`; `letta/helpers/tpuf_client.py:208-220` |
| Native vector column | pgvector `Vector(MAX_EMBEDDING_DIM)` for Postgres else SQLite `CommonVector`; MAX_EMBEDDING_DIM=4096 | `letta/orm/passage.py:34-40`; `letta/constants.py:93` |
| SQL/pgvector retrieval queries | cosine-distance ordering (Postgres) / custom cosine_distance (SQLite), ILIKE contains fallback | `letta/services/helpers/agent_manager_helper.py:1124-1140,1242-1258` |
| Search modes | vector ANN, FTS BM25, hybrid `multi_query`, timestamp recency; input validation per mode | `letta/helpers/tpuf_client.py:791-906` |
| Hybrid fusion (rerank) | RRF k=60 with vector/fts weights, returns `combined_score`, `vector_rank`, `fts_rank` | `letta/helpers/tpuf_client.py:1489-1559` |
| Archival hybrid default | Archive-backed search forces `search_mode="hybrid"` | `letta/services/agent_manager.py:2460-2476` |
| Indexer pipeline | FileProcessor: parse→chunk→embed→persist with status machine (PARSING/EMBEDDING/COMPLETED/ERROR) | `letta/services/file_processor/file_processor.py:157-300` |
| Chunker strategy | LlamaIndex splitters chosen by MIME registry; conservative fallback 384/25; default 512/50 | `letta/services/file_processor/chunker/llama_index_chunker.py:16-17,31-85` |
| Chunk fallback chain | file-specific chunker → default SentenceSplitter retry inside processor | `letta/services/file_processor/file_processor.py:49-153` |
| Embedding batching | Batch embed with token-limit halving retry; empty-chunk filtering; global semaphore(3) | `letta/services/file_processor/embedder/openai_embedder.py:20,49-91,110-127` |
| Embedding config schema | Endpoint-type literal union, dim/chunk-size/batch-size fields; OpenAI & Letta defaults | `letta/schemas/embedding_config.py:8-81` |
| Query-time embedding | `_generate_embeddings` via LLMClient using fixed default config (text-embedding-3-small, 1536d) | `letta/helpers/tpuf_client.py:226-232,285-309` |
| Embedding padding invariant | Zero-pad embeddings to MAX_EMBEDDING_DIM at write/query time | `letta/schemas/passage.py:49-77`; `letta/services/helpers/agent_manager_helper.py:1102-1103` |
| Dual-write archival insert | SQL write first, TPUF mirror with same IDs; non-fatal TPUF errors unless `strict_mode` | `letta/services/passage_manager.py:586-632` |
| Message index write | `insert_messages` to org-scoped TPUF namespace when `embed_all_messages` | `letta/services/message_manager.py:647,777`; `letta/helpers/tpuf_client.py:317-330,642` |
| TPUF delete sync on message delete | Deletes mirrored from Postgres to TPUF with strict-mode opt-out | `letta/services/message_manager.py:1120-1138` |
| Retrieval filters (tags) | TPUF `ContainsAny`/`Contains`+And for ANY/ALL modes | `letta/helpers/tpuf_client.py:954-968` |
| Retrieval filters (time) | UTC normalization + midnight end-of-day expansion for inclusive dates | `letta/helpers/tpuf_client.py:970-988` |
| Retrieval filters (messages) | Mandatory `agent_id` Eq filter plus role/project/template/conversation filters | `letta/helpers/tpuf_client.py:1110-1183` |
| Scoping isolation | Org-scoped namespaces incl. environment suffix; archive-scoped namespaces; source_id In-filter mandatory | `letta/helpers/tpuf_client.py:1677-1693,312-335,1852-1858` |
| Multi-tenant guard in SQL | All passage queries constrain `organization_id == actor.organization_id` | `letta/services/helpers/agent_manager_helper.py:1220-1233` |
| Reranker (Pinecone only) | Cross-encoder `bge-reranker-v2-m3` rerank on Pinecone file search | `letta/helpers/pinecone_utils.py:287-296` |
| Provenance schema | Passage carries `archive_id`, `source_id`, `file_id`, `file_name`, tags, metadata | `letta/schemas/passage.py:19-32`; `letta/orm/passage.py:48-73` |
| Provenance returned to model | Results include id/timestamp/tags/content plus rrf/rank metadata | `letta/services/agent_manager.py:2632-2663` |
| File-search result formatting | Per-file headers with score, grouped matches; marks accessed files | `letta/services/tool_executor/files_tool_executor.py:593-689` |
| Result hygiene | Filters out tool-role messages and prior `conversation_search` calls from results | `letta/services/tool_executor/core_tool_executor.py:151-165` |
| Failure isolation tests | `test_turbopuffer_failure_does_not_break_postgres`, `test_message_search_fallback_to_sql` | `tests/integration_test_turbopuffer.py:1586,1342` |
| RRF unit test | `test_generic_reciprocal_rank_fusion` covers mixed types, weights, empty inputs | `tests/integration_test_turbopuffer.py:984-1080` |
| Manager tests (TPUF off) | pagination, text search, vector ordering, date-only, tag functionality | `tests/managers/test_passage_manager.py:27-55,96,143-185,204-285,814-880` |
| Default page size | `RETRIEVAL_QUERY_DEFAULT_PAGE_SIZE = 5` used when top_k omitted | `letta/constants.py:458`; `letta/functions/function_sets/base.py:135-140` |

## Answers to Dimension Questions

### 1. What can be retrieved?

Four corpora: (a) **archival memory passages** written by agents (`archival_memory_insert` → `letta/services/tool_executor/core_tool_executor.py:307-317`) or APIs; (b) **prior conversation messages**, embedded only if `settings.embed_all_messages` is enabled (`letta/settings.py:445`, gate at `letta/helpers/tpuf_client.py:213-215`); (c) **document chunks** ingested from files attached to sources/folders (`letta/services/file_processor/file_processor.py:157-237`), searchable through the agent-facing file-search tool (`letta/services/tool_executor/files_tool_executor.py:570-591`); (d) **tool schemas** if `embed_tools` is set (`letta/helpers/tpuf_client.py:218-220`, `query_tools` at :2036). Additionally, files can be injected directly into the context window at attach time rather than retrieved on demand (`letta/services/file_processor/file_processor.py:208-213`).

### 2. How is it indexed?

Documents: OCR/markdown parsing → MIME-aware LlamaIndex chunking (Code/Markdown/HTML/JSON/Sentence splitters, `letta/services/file_processor/chunker/llama_index_chunker.py:31-85`) with a two-tier fallback chain (`letta/services/file_processor/file_processor.py:98-141`) → batched embeddings (`openai_embedder.py:156-199`) → persisted as `SourcePassage` rows (NATIVE) or TPUF upserts with a full-text-search-enabled `text` column schema (`letta/helpers/tpuf_client.py:606-629`). Archival memories: inserted verbatim without chunking (`text_chunks = [text]`, `letta/services/passage_manager.py:566-567`), embedded with the agent's own `embedding_config` when present, else stored text-only (`passage_manager.py:574-582`). Messages: text extracted and mirrored to TPUF with role/conversation attributes. Namespacing: one TPUF namespace per archive, org-scoped namespaces (environment-suffixed) for messages, tools, and file passages (`letta/helpers/tpuf_client.py:312-335,1677-1693`). Index layout uses B-tree-style composite indexes on `(organization_id, archive_id)`, `(created_at, id)`, `file_id` (`letta/orm/passage.py:60-103`).

### 3. Are retrieval results scoped correctly?

Yes, with defense-in-depth. Every SQL passage query constrains `organization_id == actor.organization_id` and joins through agent/archive membership tables (`letta/services/helpers/agent_manager_helper.py:1215-1233`); TPUF message queries always AND a mandatory `agent_id` equality filter into the filter tree (`letta/helpers/tpuf_client.py:1166`), and file-passage queries require `source_id In (...)` plus optional `file_id` (:1852-1865). Namespaces themselves are org- or archive-scoped and environment-suffixed to avoid cross-env bleed (:326-333,1683-1692). One deliberate limitation surfaces here: an agent attached to multiple archives raises an error instead of merging searches ("multiple archives... not yet supported for vector search", `letta/services/agent_manager.py:2444-2446`) — safe, but a functional constraint.

### 4. Are sources preserved?

Yes, structurally. The passage schema carries origin fields (`source_id`, `file_id`, `file_name`, `tags`, `created_at`: `letta/schemas/passage.py:19-32`) and the ORM persists `file_name` as a first-class column on source passages (`letta/orm/passage.py:53`). TPUF stores these as row attributes and requests them back via `include_attributes=["text","organization_id","archive_id","created_at","tags"]` / `["text","organization_id","source_id","file_id","created_at"]` (`letta/helpers/tpuf_client.py:1012,1875`). At presentation time, file-search results are grouped under their file name with scores (`files_tool_executor.py:654-673`), and archival results return id/timestamp/tags/content (`agent_manager.py:2651`). Caveat: `conversation_search` returns timestamp/role/content/relevance but **omits message IDs** (`core_tool_executor.py:225-246`), so the model cannot cite a specific prior message; and the formatted file-search output drops `passage_id`, keeping only file names.

### 5. Can stale or low-quality retrieval be detected?

Partially. Every result tuple carries a relevance metadata dict exposing `search_mode` — including the honest `"sql_fallback"` marker when the vector path degraded (`letta/services/message_manager.py:1232-1239`) — plus `rrf_score`/ranks (`core_tool_executor.py:231-246`). Timestamps are preserved and surfaced with human-readable deltas, and date-range filters allow temporal scoping (`tpuf_client.py:970-988`). However, there is no freshness signal *within* content (no re-index checksum/versioning of documents observed), no deduplication of near-identical passages, and the recency fallback (`timestamp` mode when no query is given, `tpuf_client.py:947-950`) silently changes semantics from relevance to recency without flagging it in result metadata beyond the mode key.

## Architectural Decisions

1. **SQL as source of truth, vector DB as index.** Writes always hit Postgres first; TPUF/Pinecone are mirrors with same-ID writes (`letta/services/passage_manager.py:586-628`). This makes deletion/recovery tractable and enables the SQL fallback path, at the cost of dual-write consistency management.
2. **Per-corpus namespacing instead of single-index-plus-filter for archival data.** Archives get dedicated TPUF namespaces (`letta/helpers/tpuf_client.py:312-314`) while high-volume message data shares an org namespace filtered by `agent_id` (:1110-1116) — a scale/isolation tradeoff made per corpus.
3. **Hybrid-by-default retrieval with rank fusion, not score fusion.** Vector ANN and BM25 run as a `multi_query`, merged by pure rank-based RRF (k=60) rather than mixing heterogeneous scores (`letta/helpers/tpuf_client.py:870-894,1489-1559`).
4. **Graceful degradation over hard failure.** TPUF query errors fall back to SQL with a labeled `sql_fallback` mode (`message_manager.py:1220-1240`); TPUF write errors are logged and swallowed unless `strict_mode` (`passage_manager.py:629-632`).
5. **Fixed embedding config on the hot path.** TPUF search always embeds queries with its own default config (text-embedding-3-small/1536, `tpuf_client.py:226-232`) regardless of agent config, with dimension-mismatch regeneration on ingest (:527-548) — simplifies consistency but silently overrides user-configured models for message search.
6. **Padding-to-max-dimension storage invariant.** All native-store embeddings zero-padded to 4096 dims so mixed-model corpora coexist in one column (`letta/constants.py:93`, `letta/schemas/passage.py:49-77`).
7. **Tool-first retrieval UX.** Retrieval is primarily agent-invoked (function tools whose docstrings teach search strategy, `letta/functions/function_sets/base.py:87-243`), with REST parity via shared service methods (`agents.py:1523-1546` reuses `search_agent_archival_memory_async`).

## Notable Patterns

- **Recursive-contamination guard:** `conversation_search` filters out tool-role messages and earlier search calls so retrieved results cannot recursively nest or echo previous queries (`core_tool_executor.py:151-165`).
- **Layered fallback chains:** chunker → default chunker (`file_processor.py:98-141`); specialized splitter → SentenceSplitter inside the chunker (`llama_index_chunker.py:128-146`); TPUF → SQL at query time (`message_manager.py:1220-1240`); token-limit error → batch halving recursion (`openai_embedder.py:64-91`).
- **Metadata-honest results:** the fusion step attaches provenance-of-ranking (which list ranked it where) to each hit (`tpuf_client.py:1548-1552`).
- **Redis-cached embeddings for repeated query texts** keyed `model:endpoint:text` (`passage_manager.py:34-40`).
- **Cache warming hints** sent to TPUF before latency-sensitive search traffic (`tpuf_client.py:248-282`).
- **Access tracking as a side effect of search:** matched files get `mark_access_bulk` updates (`files_tool_executor.py:676-679`).
- **Test-mode determinism:** a `disable_turbopuffer` fixture (`tests/conftest.py:149`) forces the NATIVE path in manager tests (`tests/managers/test_passage_manager.py:27`), keeping unit tests backend-free.

## Tradeoffs

- **Dual-write eventual consistency:** SQL-first mirroring means a failed TPUF write leaves an invisible-to-vector-search passage until re-sync; the code accepts this deliberately (`passage_manager.py:629-632`) and even documents pending backfills (`tpuf_client.py:1052-1055`).
- **Hybrid latency vs. recall:** every hybrid query issues two backend queries plus a query embedding call (`tpuf_client.py:870-894`); acceptable at top_k≤10 defaults but costly at scale.
- **Rank fusion discards magnitudes:** RRF ignores raw similarity/BM25 scores, which stabilizes ranking across metrics but cannot express "very confident" vs "marginal" hits beyond order (`tpuf_client.py:1500-1503`).
- **Simplicity of SQL fallback vs. quality:** fallback search is substring `contains` with no scores (`agent_manager_helper.py:1256-1258`) — available but markedly weaker than the primary path.
- **Chunking quality vs. robustness:** aggressive multi-tier fallbacks guarantee output but can silently degrade document structure fidelity (e.g., falling back from MarkdownNodeParser to SentenceSplitter, `llama_index_chunker.py:81-85`).
- **Agentic inserts bypass chunking:** `archival_memory_insert` indexes the whole string as one passage (`passage_manager.py:566-567`), relying on the model to write self-contained snippets (as its docstring instructs, `base.py:172-176`); oversized entries become poor retrieval units.

## Failure Modes / Edge Cases

- **Tag filter applied after LIMIT on the SQL path:** `query_agent_passages_async` limits the query (:2494-2496) *then* post-filters by tags in Python (:2507-2527), acknowledged as inefficient — a tagged search can return fewer than `top_k` (or zero) results despite matches existing just past the limit window.
- **Soft-deleted messages may resurface from TPUF:** the TODO says deleted messages will be excluded once `is_deleted` is backfilled, claiming DB post-filtering meanwhile (`tpuf_client.py:1052-1055`), but `search_messages_async`'s TPUF branch constructs messages directly from TPUF dicts without a liveness check (`message_manager.py:1201-1216`) — comment and implementation diverge.
- **Silent semantic switch to recency:** omitting `query_text` downgrades search to `timestamp` mode (`tpuf_client.py:947-950,1103-1106`) — reasonable, but callers get "most recent," not "most relevant," with little surface warning.
- **Multi-archive agents cannot vector-search at all** (`agent_manager.py:2444-2446`).
- **Empty-query archival search returns empty list early** (`agent_manager.py:2563-2565`) rather than recency results — inconsistent with the message path's behavior above.
- **Brittle token-limit detection** via string-matching error text, self-admittedly brittle (`openai_embedder.py:93-107`).
- **Namespace-not-found mapped to friendly ValueError** only by sniffing error strings (`tpuf_client.py:897-904`).
- **Mixed-provider sources split the limit:** searching across TPUF and Pinecone sources halves `limit` for each backend and concatenates without cross-reranking, flagged "hacky" in-code (`files_tool_executor.py:570-588`).

## Future Considerations

- Push tag (and other attribute) filtering into the SQL `WHERE`/JOIN clause to fix the limit-before-filter defect (`agent_manager.py:2507-2509` already sketches this).
- Add a cross-encoder reranker stage to the TPUF/NATIVE paths (parity with the Pinecone `bge-reranker-v2-m3` setup, `pinecone_utils.py:295`), especially for hybrid candidates pre-top_k.
- Complete the `is_deleted` attribute backfill and enforce liveness filtering in TPUF message queries (`tpuf_client.py:1052-1055`).
- Surface message IDs and passage IDs in agent-facing search outputs to enable precise citation loops.
- Unify the three agent-loop truncation/search plumbing variants (`letta/agents/letta_agent.py:1823`, `letta_agent_v2.py:1184`, `letta_agent_v3.py:1878`) into one retrieval-tool middleware.
- Consider honoring per-agent `embedding_config` for message query embedding, or documenting that message search always uses the platform default.

## Questions / Gaps

- **No evidence found of any evaluation harness measuring retrieval quality** (recall@k, nDCG) or golden-dataset tests for the chunkers; searched `tests/` for such fixtures and found only behavioral/integration tests (`tests/integration_test_turbopuffer.py`, `tests/managers/test_passage_manager.py`).
- **No evidence of incremental document re-indexing** on file update (checksum/diff-based); only delete-and-reinsert paths were observed (`tpuf_client.py:1952-2001`), though message updates do reindex (`tests/integration_test_turbopuffer.py:1391`).
- **No deduplication or near-duplicate collapse** mechanism was found anywhere in the ingestion or query paths.
- Whether the deprecated `source_id` field (`letta/schemas/passage.py:23-26`) still participates in any active write path alongside `folder_id` migration was not fully traced; the TPUF file-passage namespace still keys on `source_id` (`tpuf_client.py:1852-1858`), suggesting incomplete migration.
- The exact semantics of `include_attributes=True` (bare boolean, `tpuf_client.py:1193`) versus the explicit attribute lists used elsewhere (:1012,:1875) could not be confirmed against the vendored turbopuffer SDK from this repository alone.

---

Generated by `Dimension 05.04: Retrieval-Augmented Memory` against `letta`.

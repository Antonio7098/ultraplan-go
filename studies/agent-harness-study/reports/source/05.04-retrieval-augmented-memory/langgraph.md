# Source Analysis: langgraph

## Dimension 05.04 — Retrieval-Augmented Memory

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (monorepo: `libs/checkpoint`, `libs/checkpoint-postgres`, `libs/checkpoint-sqlite`, `libs/langgraph`, `libs/prebuilt`, `libs/sdk-py`); JS/TS SDK in `libs/sdk-js` |
| Analyzed | 2026-08-25 |

## Summary

LangGraph implements retrieval-augmented memory as a first-class, pluggable **Store** subsystem rather than a monolithic RAG pipeline. The abstract `BaseStore` (`libs/checkpoint/langgraph/store/base/__init__.py:708`) defines namespace-scoped key-value operations (`get/put/search/list_namespaces`) with an optional semantic-search layer enabled by an `IndexConfig` (`dims`, `embed`, `fields`) at construction time (`libs/checkpoint/langgraph/store/base/__init__.py:578-705`). Three concrete backends ship: `InMemoryStore` (exact cosine similarity over stored vectors, numpy-optional), `PostgresStore` (pgvector with HNSW/IVFFlat/flat index options), and `SqliteStore` (sqlite-vec). There is no built-in document loader or text chunker; "chunking" is JSON-path field selection where each array element becomes its own vector. Retrieval is scoped by hierarchical namespace prefixes plus JSONB filter operators (`$eq`, `$gt`, ...), results carry a similarity `score`, and staleness is managed via TTLs with a background sweeper. Notably absent: rerankers, relevance thresholds, embedding-model versioning/migration, and any enforced citation/provenance schema beyond `(namespace, key)` identity.

## Rating

**7 / 10** — Clear model with explicit interfaces, multiple tested implementations, and real operational safeguards (TTL sweeping, expired-item filtering on reads, SQL-injection-safe DDL sanitization, LIKE escaping for namespace scoping). Falls short of 8–10 because retrieval quality is unobservable (no metrics/thresholds), score semantics are inconsistent across backends (one apparent bug in the SQLite inner-product path), ANN indexes are effectively disabled whenever filters are present in Postgres, and there is no reranking or provenance metadata model.

## Evidence Collected

Every entry cites workspace-relative paths from `sources/langgraph`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Retriever interface | `BaseStore.search()` / `asearch()` take `namespace_prefix`, optional natural-language `query`, `filter`, `limit`, `offset` | `libs/checkpoint/langgraph/store/base/__init__.py:779-854`, `:1029-1107` |
| Retrieval op model | `SearchOp(namespace_prefix, filter, limit, offset, query, refresh_ttl)`; `GetOp`; `ListNamespacesOp` | `libs/checkpoint/langgraph/store/base/__init__.py:203-307`, `:157-200`, `:368-428` |
| Vector store: Postgres/pgvector | `store_vectors` table keyed `(prefix, key, field_name)` with FK cascade; pgvector extension created by migration | `libs/checkpoint-postgres/langgraph/store/postgres/base.py:92-145` |
| ANN index config | `ANNIndexConfig` kind ∈ {hnsw, ivfflat, flat}, vector_type ∈ {vector, halfvec}; distance_type ∈ {l2, inner_product, cosine} | `libs/checkpoint-postgres/langgraph/store/postgres/base.py:178-233` |
| Vector store: SQLite | `sqlite_vec` extension loaded; distances via `vec_distance_cosine/L2/L1` | `libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:15`, `:506-518`, `:1098` |
| Vector store: In-memory | `_cosine_similarity` with lazy numpy import and pure-Python fallback | `libs/checkpoint/langgraph/store/memory/__init__.py:479-522` |
| Embedding config | `IndexConfig(dims, embed, fields)`; `embed` accepts LangChain `Embeddings`, sync/async callables, or provider string `"openai:text-embedding-3-small"` | `libs/checkpoint/langgraph/store/base/__init__.py:585-668` |
| Embedding normalization | `ensure_embeddings()` wraps arbitrary functions into LangChain `Embeddings` (`EmbeddingsLambda`); provider strings require `langchain>=0.3.9` | `libs/checkpoint/langgraph/store/base/embed.py:34-106`, `:419-426` |
| Chunking = JSON-path fields | Per-item `index=[...]` overrides store defaults; array notation `context[*].content` embeds each element as a separate vector named `path.i` | `libs/checkpoint/langgraph/store/base/__init__.py:490-525`; `libs/checkpoint/langgraph/store/memory/__init__.py:430-440`; `libs/checkpoint-postgres/langgraph/store/postgres/base.py:386-398` |
| Path tokenizer | `tokenize_path` / `get_text_at_path` handle dot paths, `[0]`, `[-1]`, `[*]`, `{f1,f2}` multi-field selection; missing fields skipped | `libs/checkpoint/langgraph/store/base/embed.py:233-327`, `:333-396` |
| Write-time indexing pipeline | PUT queries extract texts per path and defer one batched `embed_documents` call; vectors upserted `ON CONFLICT (prefix,key,field_name) DO UPDATE` | `libs/checkpoint-postgres/langgraph/store/postgres/base.py:377-423`, `:1039-1053` |
| Query-time embedding | Search queries embedded in batch (`embed_documents([q for ...])`) before SQL execution | `libs/checkpoint-postgres/langgraph/store/postgres/base.py:1076-1089` (sync); `libs/checkpoint-postgres/langgraph/store/postgres/aio.py:507-513` (async) |
| Retrieval filters | `$eq/$ne/$gt/$gte/$lt/$lte` operators documented and implemented per backend (JSONB in PG, `json_extract` in SQLite, dict comparison in memory) | `libs/checkpoint/langgraph/store/base/__init__.py:250-285`; `libs/checkpoint-postgres/langgraph/store/postgres/base.py:451-462`, `:649-664`; `libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:453-499`; `libs/checkpoint/langgraph/store/memory/__init__.py:551-592` |
| Namespace scoping | Prefix condition matches exact or `.`-separated descendants; LIKE metacharacters escaped so `("foo",)` does not match `("foobar",)` | `libs/checkpoint-postgres/langgraph/store/postgres/base.py:1283-1308` |
| Namespace listing | Wildcard prefix/suffix matching via POSIX regex mirroring InMemoryStore semantics | `libs/checkpoint-postgres/langgraph/store/postgres/base.py:1311-1328`; `libs/checkpoint/langgraph/store/memory/__init__.py:525-548` |
| Score/provenance on results | `SearchItem` carries `score` plus full identity (`namespace`, `key`, `value`, timestamps); invalid scores logged and nulled | `libs/checkpoint/langgraph/store/base/__init__.py:118-154`; `libs/checkpoint-postgres/langgraph/store/postgres/base.py:1359-1382` |
| Max-pooling per item | SQL `DISTINCT ON (prefix, key)` picks best-scoring vector per item; InMemoryStore dedupes by `(namespace, key)` after sorting | `libs/checkpoint-postgres/langgraph/store/postgres/base.py:521-536`; `libs/checkpoint/langgraph/store/memory/__init__.py:330-344` |
| TTL / stale handling | `TTLConfig(refresh_on_read, omit_expired, default_ttl, sweep_interval_minutes)`; expired-but-unswept rows excluded from reads when `omit_expired`; sweeper thread deletes expired items | `libs/checkpoint/langgraph/store/base/__init__.py:526-575`; `libs/checkpoint-postgres/langgraph/store/postgres/base.py:244-247`, `:269-271`, `:439-444`, `:850-939` |
| TTL capability gate | `BaseStore.supports_ttl = False` default; Postgres/SQLite set True; passing `ttl` to an unsupported store raises `NotImplementedError` | `libs/checkpoint/langgraph/store/base/__init__.py:727`, `:920-924`; `libs/checkpoint-postgres/langgraph/store/postgres/base.py:755`; `libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:278`, `:853` |
| Runtime injection | Graph compile accepts `store=`; nodes/tools access via `Runtime.store`, `get_store()`, or `Annotated[BaseStore, InjectedStore()]` tool args | `libs/langgraph/langgraph/graph/state.py:1182`; `libs/langgraph/langgraph/runtime.py:115`, `:172-203`; `libs/langgraph/langgraph/config.py:32-56`; `libs/prebuilt/langgraph/prebuilt/tool_node.py:1829-1900`, injection at `:1407-1415` |
| Prebuilt agent wiring | `create_react_agent(..., store=...)` forwards store to compiled graph | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:301`, `:823`, `:997` |
| Async op batching | `AsyncBatchedBaseStore` coalesces concurrent ops per event-loop tick and dedupes identical Get/Search/Put ops before hitting storage/embeddings | `libs/checkpoint/langgraph/store/base/batch.py:58-101`, `:283-371` |
| Server API surface | SDK `client.store.search_items(namespace_prefix, filter, limit, offset, query)` exposes semantic search over HTTP; auth layer models semantic-search query | `libs/sdk-py/langgraph_sdk/_async/store.py:180-239`; `libs/sdk-py/langgraph_sdk/auth/types.py:902` |
| Tests: filters + vectors | `test_vector_search_with_filters`, `test_async_vector_search_with_filters` assert combined query+filter behavior incl. operator filters | `libs/checkpoint/tests/test_store.py:719-794` |
| Tests: pagination | `test_vector_search_pagination` verifies stable limit/offset paging over ranked results | `libs/checkpoint/tests/test_store.py:879-916` |
| Tests: auto-embedding + update | `test_vector_insert_with_auto_embedding`, `test_vector_update_with_embedding`, `test_embed_with_path` (multi-vector docs) | `libs/checkpoint/tests/test_store.py:597-682`, `:919-948`; `libs/checkpoint-postgres/tests/test_store.py:589-630` |
| Tests: TTL lifecycle | Sync/async TTL basic/refresh/sweeper/search-with-TTL suites | `libs/checkpoint-sqlite/tests/test_ttl.py:25-370` |
| Reranker search | Searched `rerank`, `re-rank`, `cross-encoder`, `BM25`, `bm25` across all libs — no evidence found | (grep across `libs/`, 0 hits) |

## Answers to Dimension Questions

1. **What can be retrieved?** Arbitrary JSON-dict items ("memories") stored under hierarchical string namespaces, e.g. `("documents", "user123")` (`libs/checkpoint/langgraph/store/base/__init__.py:51-115`). Retrieval is either exact by `(namespace, key)` or similarity/attribute-based within a namespace prefix (`SearchOp`, `__init__.py:203-307`). There is no retrieval over conversation history, code, or traces through this subsystem — those live in separate checkpoint/channel machinery; the store is explicitly "long-term memory that persists across threads" (`__init__.py:1-10`). RAG notebooks in `examples/rag/` wire external vector stores instead.
2. **How is it indexed?** Opt-in per store instance via `IndexConfig(dims, embed, fields)` (`__init__.py:578-705`). On write, configured (or per-item overridden) JSON paths are extracted and each extracted text is embedded once, then upserted keyed by `(namespace, key, field_name)` (`libs/checkpoint-postgres/langgraph/store/postgres/base.py:377-423`). Postgres additionally creates a pgvector ANN index (HNSW default) during migrations (`base.py:125-144`).
3. **Are retrieval results scoped correctly?** Yes, this is a strength. Namespace prefixes match whole segments only, with LIKE-metacharacter escaping to prevent `user_1` matching `userX1` (`base.py:1283-1308`); wildcard namespace matching uses POSIX regex anchored per segment (`base.py:1311-1328`). Structured filters compose with vector search in all three backends, and `omit_expired` TTL filtering excludes stale rows at read time (`base.py:439-444`). One caveat: because of pgvector's filter+index restrictions, filtered searches cannot use the ANN index (see Tradeoffs).
4. **Are sources preserved?** Partially. Every result carries its `(namespace, key)` coordinates, timestamps, and similarity score (`SearchItem`, `__init__.py:118-154`) — enough for callers to trace a hit back to its stored item. But there is no schema for original-source provenance (URL, document ID, chunk origin): whatever provenance exists must be manually placed inside the user-defined value dict. No evidence found of any citation-generation or source-attribution mechanism in library code (searched `provenance`, `citation` in libs; only notebook/example hits).
5. **Can stale or low-quality retrieval be detected?** Stale: yes, operationally — TTLs expire items, `omit_expired=True` hides them from reads before the sweep runs, and a background sweeper thread hard-deletes them (`base.py:850-939`; tests `libs/checkpoint-sqlite/tests/test_ttl.py:201`, `:340`). Low quality: only partially — a similarity `score` is returned with every hit, but there is no minimum-score threshold, no reranking stage, no retrieval-quality metrics, and no observability beyond logging invalid scores (`base.py:1369-1374`). Callers cannot tell whether a low score indicates a bad hit without building that logic themselves.

## Architectural Decisions

- **Store as infrastructure, not pipeline.** LangGraph deliberately ships a generic namespaced KV store with optional vector search instead of a document-RAG framework; document ingestion/chunking stays outside the harness (external vectorstores in `examples/rag/*.ipynb`). This keeps the runtime small and makes memory a graph-level resource injected at compile time (`libs/langgraph/langgraph/graph/state.py:1182`).
- **Opt-in semantic search.** Without `IndexConfig`, `search(query=...)` silently degrades to recency-ordered filtering (`libs/checkpoint-postgres/langgraph/store/postgres/base.py:548-563`: `ORDER BY store.updated_at DESC`), and `put(index=...)` is ignored (`libs/checkpoint/langgraph/store/base/__init__.py:716-725`). Explicit non-goal coupling of embeddings to every deployment.
- **Embeddings behind a normalization seam.** `ensure_embeddings` unifies LangChain `Embeddings` objects, plain sync/async functions, and provider strings (`libs/checkpoint/langgraph/store/base/embed.py:34-106`), decoupling backend SQL from embedding providers.
- **Batch-first operation model.** All public methods lower to `batch([op])`/`abatch([op])`; async stores coalesce concurrent calls in one tick and dedupe repeated puts/gets before executing, amortizing embedding calls (`libs/checkpoint/langgraph/store/base/batch.py:326-369`, `_dedupe_ops` at `:283-323`).
- **Per-backend SQL generation with shared op semantics.** Filter operators and scoring are re-implemented per backend against native features (JSONB vs `json_extract` vs Python dicts), keeping the interface contract in docs/tests rather than code (`libs/checkpoint-postgres/langgraph/store/postgres/base.py:649-664` vs `libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:453-499`).

## Notable Patterns

- **JSON-path pseudo-chunking.** Instead of fixed-size text chunking, indexing granularity is declared structurally: `fields=["messages[*].content"]` produces one vector per message, with the element index encoded in `field_name` as `path.i` (`libs/checkpoint/langgraph/store/memory/__init__.py:430-440`; tokenizer at `libs/checkpoint/langgraph/store/base/embed.py:333-396`).
- **Max-pooling across multi-vector documents.** When one item yields several vectors, both SQL (`DISTINCT ON (prefix, key)` after ordering by distance, `libs/checkpoint-postgres/langgraph/store/postgres/base.py:521-536`) and the in-memory path (dedupe-by-key after sorting, `libs/checkpoint/langgraph/store/memory/__init__.py:330-344`) reduce to the item's best-matching vector.
- **Over-fetch heuristic for pagination correctness.** Postgres fetches `limit * estimated_vectors_per_doc * 2 + 1` candidate rows before de-duplicating to keep limit/offset stable (`base.py:503-546`); SQLite uses `limit * 2` (`libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:561-567`).
- **Capability flags over silent failure.** `supports_ttl` gates TTL usage with a raised error rather than ignoring arguments (`libs/checkpoint/langgraph/store/base/__init__.py:920-924`), while semantic search is documented as degrade-silently — two different policies for optional capabilities.
- **Reserved root namespace.** Namespaces may not start with `"langgraph"`, reserving a root label for internal data (`__init__.py:1280-1283`).

## Tradeoffs

- **Correctness vs speed in Postgres filtered search.** `get_distance_operator` documents that pgvector refuses ANN traversal when non-vector WHERE clauses are mixed in, so today's queries accept sequential scans for filter-correctness (`libs/checkpoint-postgres/langgraph/store/postgres/base.py:1414-1424`, citing pgvector issue #216). The HNSW/IVFFlat migration still creates the index, but it goes unused for the common "semantic search + metadata filter" case.
- **Simplicity vs recall guarantees.** Over-fetch heuristics (`limit*est*2+1`, `limit*2`) bound result truncation for multi-vector items but provide no completeness guarantee when a single doc embeds into many vectors (`base.py:503-506`).
- **Backend parity vs backend-native performance.** Re-implementing filters/scoring per engine maximizes portability but has already produced divergence: SQLite's `inner_product` distance maps to `-1 * vec_distance_L1` (`libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:512-515`), which is not inner product at all and will rank differently than Postgres's `<#>` operator (`base.py:1445-1447`).
- **Zero-config defaults vs footguns.** Default `fields=["$"]` serializes and embeds the entire JSON value (`libs/checkpoint/langgraph/store/base/__init__.py:670-705`); convenient, but semantically noisy for structured memories compared to curated fields.

## Failure Modes / Edge Cases

- **Silent degradation without index config:** `query` passed to an unconfigured store returns recent-filtered rows, indistinguishable from semantic results to naive callers (`base.py:548-556`).
- **Unembedded items leak into semantic results:** if fewer items have vectors than `limit`, InMemoryStore pads results with `score=None` items (`libs/checkpoint/langgraph/store/memory/__init__.py:345-350`) — unscored entries can occupy result slots meant for ranked hits.
- **Cross-backend score drift:** cosine similarity (memory/PG default) vs L2-based expressions (SQLite `inner_product` mislabel above) mean identical data can produce differently ordered results per backend.
- **Embedding model evolution is unmigrated:** `dims` and vector type are baked into DDL at setup time (`base.py:105-124`); changing the embedding model requires out-of-band table rebuild — no versioning/re-embedding path exists in code.
- **Pagination instability under concurrent writes:** offset-based paging over a live store (no snapshot/token) can skip or repeat items between pages (`__init__.py:290-291`).
- **Async-store sync-call deadlock guard:** calling sync methods on `AsyncBatchedBaseStore` from the running loop raises immediately rather than deadlocking (`libs/checkpoint/langgraph/store/base/batch.py:33-55`) — good failure, but a sharp edge for users mixing APIs.

## Future Considerations

- Add a reranking hook (post-vector, pre-return) and/or configurable score threshold on `search()`; the `DISTINCT ON`/max-pool sites are the natural interception points (`base.py:521-536`).
- Introduce embedding-space versioning (model id + dims stamped per vector row) to make model migration and mixed-space detection possible.
- Expose retrieval-quality observability: log/histogram scores, counts scanned vs returned, and whether the ANN index was used.
- Fix or document the SQLite `inner_product` mapping (`libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:512-515`) and align expanded-limit heuristics between backends.
- Optional hybrid retrieval (keyword/BM25 alongside vectors) — currently no lexical index exists anywhere in the repo.

## Questions / Gaps

- **Reranking:** No evidence found. Searched `rerank|re-rank|cross-encoder|BM25` across `libs/`; zero hits. Ordering is raw similarity only.
- **Provenance/citation model:** No evidence found of a first-class citation type; searched `provenance|citation` in `libs/` — hits only in example notebooks and prompt text (`examples/rag/langgraph_adaptive_rag_cohere.ipynb`, `libs/cli/examples/graphs/storm.py:444-454`), i.e., application-level, not harness-level.
- **Chunking of long texts:** No evidence found of length-aware splitting; long values embedded whole at `"$"` default. What happens to oversized inputs depends entirely on the chosen embedding provider.
- **Retrieval evaluation:** No benchmark or eval suite measures retrieval precision/recall for the store backends; tests verify mechanics (filters, pagination, TTL) not quality. The conformance package (`libs/checkpoint-conformance/`) targets checkpointers, not stores — searched `semantic|vector|score` there with no store-retrieval coverage found.
- **Multi-tenant isolation:** Namespacing is convention-based; nothing in the store enforces that a tenant cannot read another tenant's prefix. Whether isolation is delegated entirely to server-side auth could not be confirmed from library code alone (auth types exist at `libs/sdk-py/langgraph_sdk/auth/types.py:902` but enforcement lives outside this source tree).

---

Generated by `05.04-retrieval-augmented-memory` against `langgraph`.

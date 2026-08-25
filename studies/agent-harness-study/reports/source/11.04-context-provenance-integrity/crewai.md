# Source Analysis: crewai

## 11.04 Context Provenance and Integrity

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (pydantic models, uv workspace monorepo: `lib/crewai`, `lib/crewai-tools`, `lib/crewai-files`, `lib/crewai-core`, `lib/cli`, `lib/devtools`) |
| Analyzed | 2026-08-24 |

## Summary

CrewAI's context provenance story is split sharply between two context channels. The **unified memory system** has an explicit, typed provenance model: every `MemoryRecord` carries a `source` identifier ("Used for provenance tracking and privacy filtering", `sources/crewai/lib/crewai/src/crewai/memory/types.py:60-66`), a `private` visibility flag (`types.py:67-73`), and `created_at` / `last_accessed` timestamps (`types.py:46-53`). This model survives persistence in both storage backends (LanceDB, `sources/crewai/lib/crewai/src/crewai/memory/storage/lancedb_storage.py:247-287`; Qdrant edge, `sources/crewai/lib/crewai/src/crewai/memory/storage/qdrant_edge_storage.py:206-240`) and is documented as a design goal with access-control semantics (`sources/crewai/docs/edge/en/concepts/memory.mdx:446-477`).

The **knowledge/RAG channel**, by contrast, strips provenance entirely: knowledge sources have a metadata field explicitly marked "Currently unused" (`sources/crewai/lib/crewai/src/crewai/knowledge/source/base_knowledge_source.py:28`), chunks are persisted as bare content strings (`sources/crewai/lib/crewai/src/crewai/knowledge/storage/knowledge_storage.py:118`), and retrieved snippets are injected into prompts with all metadata discarded (`sources/crewai/lib/crewai/src/crewai/knowledge/utils/knowledge_utils.py:4-12`).

Framework-internal callers undermine the memory model in practice: automatic agent memory saves never populate `source` (`sources/crewai/lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:59-63`), and the LLM consolidation pipeline silently **destroys** `source` and `private` on merged records by rebuilding records without them (`sources/crewai/lib/crewai/src/crewai/memory/encoding_flow.py:461-471`). No trust/authority level exists anywhere for context items. Transformations (LLM extraction, consolidation merges, chunking) are performed but never logged. Only the HITL learning flow uses provenance end-to-end (`sources/crewai/lib/crewai/src/crewai/flow/human_feedback.py:181,261,346`), and no test exercises source/private filtering.

## Rating

**5 / 10 — Present but inconsistent and fragile.**

Rationale against the rubric:
- The memory schema is a genuine design (typed pydantic fields, two backends round-trip provenance, documented semantics) — clearly above "absent, implicit, ad-hoc".
- But it fails the 7-8 bar ("clear model with tests, explicit interfaces, and operational safeguards") because: (a) the flagship auto-memory path never writes `source`, so most stored memories have no origin; (b) the consolidation pipeline actively resets `source`/`private` to defaults on update (`encoding_flow.py:461-471`) — a correctness defect that can silently make private memories public; (c) zero test coverage of provenance filtering; (d) the knowledge system, the largest context channel, has no provenance at all; (e) no trust dimension exists; (f) transformations are untraceable.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Source annotation (memory) | `MemoryRecord.source: str \| None` described as "Origin of this memory... Used for provenance tracking and privacy filtering." | sources/crewai/lib/crewai/src/crewai/memory/types.py:60-66 |
| Privacy/access flag | `MemoryRecord.private` — visible only to recall from same source or `include_private=True` | sources/crewai/lib/crewai/src/crewai/memory/types.py:67-73 |
| Provenance-aware API | `remember(..., source=..., private=...)` and `recall(..., source=..., include_private=...)` params | sources/crewai/lib/crewai/src/crewai/memory/unified_memory.py:430-441,681-690 |
| Private-record filtering at recall | Shallow recall filters `not r.private or r.source == source` | sources/crewai/lib/crewai/src/crewai/memory/unified_memory.py:746-751 |
| Freshness fields | `created_at` and `last_accessed` datetimes default to utcnow | sources/crewai/lib/crewai/src/crewai/memory/types.py:46-53 |
| Recency scoring | Composite score decays with `0.5^(age_days / half_life)`; half-life configurable | sources/crewai/lib/crewai/src/crewai/memory/types.py:364-366,175-183 |
| Access-time freshness touch | Recall calls `touch_records()` updating `last_accessed` | sources/crewai/lib/crewai/src/crewai/memory/unified_memory.py:784-790; lancedb_storage.py:339-359 |
| Scope freshness info | `ScopeInfo.oldest_record` / `newest_record` timestamps | sources/crewai/lib/crewai/src/crewai/memory/types.py:121-128 |
| Provenance survives serialization (LanceDB) | `_record_to_row`/`_row_to_record` round-trip source, private, created_at, last_accessed, metadata JSON | sources/crewai/lib/crewai/src/crewai/memory/storage/lancedb_storage.py:247-287 |
| Provenance survives serialization (Qdrant) | Payload stores/rehydrates source, private, created_at, last_accessed, metadata | sources/crewai/lib/crewai/src/crewai/memory/storage/qdrant_edge_storage.py:206-240 |
| Consolidation drops provenance (defect) | Updated record rebuilt without `source=`/`private=`, resetting both to defaults | sources/crewai/lib/crewai/src/crewai/memory/encoding_flow.py:461-471 |
| Unrecorded transformation (extraction) | Task output distilled by LLM into memory strings; no link to origin blob kept | sources/crewai/lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:43-49; memory/analyze.py:159 |
| Unrecorded transformation (merge) | `ConsolidationPlan` keep/update/delete actions applied; merged record copies metadata verbatim, no merge log | sources/crewai/lib/crewai/src/crewai/memory/analyze.py:124-136; encoding_flow.py:372-501 |
| Auto-memory saves omit source | `remember_many(extracted, agent_role=...)`; `agent_role` only decorates events | sources/crewai/lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:59-63; unified_memory.py:455-456 |
| Only provenance-complete flow | HITL learning tags lessons `source="hitl"` and recalls with matching filter | sources/crewai/lib/crewai/src/crewai/flow/human_feedback.py:181,261,346 |
| Knowledge metadata unused | `metadata: dict[str, Any] = Field(default_factory=dict)  # Currently unused` | sources/crewai/lib/crewai/src/crewai/knowledge/source/base_knowledge_source.py:28 |
| Knowledge chunks saved bare | `rag_documents: list[BaseRecord] = [{"content": doc} for doc in documents]` — no metadata/doc_id/source | sources/crewai/lib/crewai/src/crewai/knowledge/storage/knowledge_storage.py:118,199 |
| File-to-chunk mapping lost | File sources track `content: dict[Path, str]` but `_save_documents` saves only flat chunk strings | sources/crewai/lib/crewai/src/crewai/knowledge/source/base_file_knowledge_source.py:24,71-76 |
| Retrieved knowledge stripped | `extract_knowledge_context` keeps only `result["content"]`; metadata dropped before prompt injection | sources/crewai/lib/crewai/src/crewai/knowledge/utils/knowledge_utils.py:4-12 |
| RAG record supports provenance (unused) | `BaseRecord` TypedDict allows `doc_id` + `metadata`; `SearchResult` returns metadata | sources/crewai/lib/crewai/src/crewai/rag/types.py:9-24,32-49 |
| Content-addressable IDs | ChromaDB prep generates SHA256 doc ids from content+metadata (dedup identity, not provenance) | sources/crewai/lib/crewai/src/crewai/rag/chromadb/utils.py:81-85 |
| Inter-task context unannotated | Context = raw task outputs joined by dividers, no task id/timestamp/trust | sources/crewai/lib/crewai/src/crewai/utilities/formatter.py:16-26; crew.py:1866-1874 |
| Memory context formatting | `MemoryMatch.format()` includes score, categories, arbitrary metadata — but not `source` or `created_at` | sources/crewai/lib/crewai/src/crewai/memory/types.py:92-106; agent/core.py:651-655 |
| File delivery provenance | `FileReference` carries provider `file_id`, `provider`, `expires_at`, `file_uri` for uploads | sources/crewai/lib/crewai-files/src/crewai_files/core/resolved.py:51-67 |
| Task-output freshness | SQLite kickoff outputs store `timestamp DATETIME DEFAULT CURRENT_TIMESTAMP`, task_id, was_replayed | sources/crewai/lib/crewai/src/crewai/memory/storage/kickoff_task_outputs_storage.py:46-58 |
| Documented design goal | Docs: "Every memory record can carry a `source` tag for provenance tracking and a `private` flag for access control" with user:alice/admin examples | sources/crewai/docs/edge/en/concepts/memory.mdx:446-477 |
| No tests for provenance | Grep across `lib/crewai/tests/memory/*` finds no exercise of `source=`/`.private` recall filtering; only schema placeholder `"source": ""` | sources/crewai/lib/crewai/tests/memory/test_qdrant_edge_storage.py:274 |

## Answers to Dimension Questions

### 1. Does each context item know where it came from?
**Only memory records do — partially.** `MemoryRecord.source` exists and round-trips through both backends (`types.py:60-66`, `lancedb_storage.py:257,285`). However, the framework's own auto-memory writer never sets it (`base_agent_executor.py:59-63`, `lite_agent.py:654`, `agent/core.py:1751`), so memories produced by normal crew runs have `source=None`; only HITL learning tags origins (`flow/human_feedback.py:346`). Knowledge chunks know nothing about their file/URL/string origin once ingested (`knowledge_storage.py:118`); the per-file mapping exists only transiently in the source object (`base_file_knowledge_source.py:24`). Inter-task context strings carry no origin markers (`formatter.py:16-26`).

### 2. Is freshness tracked?
**Yes for memory; no for knowledge.** Memory has creation/access timestamps feeding recency-decay ranking with configurable half-life (`types.py:46-53,364-366,175-183`) plus `touch_records` on recall (`unified_memory.py:784-790`) and scope-level oldest/newest stats (`types.py:121-128`). Knowledge/RAG documents and inter-task context have zero timestamp fields. One robustness gap: missing timestamps are silently fabricated as `utcnow()` during deserialization (`lancedb_storage.py:265-271`) rather than flagged.

### 3. Is trust level indicated?
**No.** There is no trust, authority, confidence-in-origin, or verification field anywhere in the memory, knowledge, RAG, or context-aggregation code paths (repo-wide grep for `trust|authority` returns only unrelated hits: A2A completion-status trust `a2a/config.py:383-413`, IBM auth profiles, checkpoint deserialization guards). `private` is an access-control bit, not a trust grade; `importance` weights retrieval relevance, not reliability (`types.py:40-45`). All recalled items are presented to the LLM as equally trustworthy text.

### 4. Are transformations traceable?
**No.** Three transformations occur routinely, none recorded: (a) LLM distillation of task outputs into memory statements (`base_agent_executor.py:43-49`) leaves no link between stored fact and originating task run; (b) consolidation merges/updates/deletes rewrite records (`encoding_flow.py:372-501`, plan schema `analyze.py:124-136`) copying metadata verbatim with no merged-from annotation; (c) knowledge chunking (`base_knowledge_source.py:43-48`) discards chunk boundaries and offsets. Worse, the update path actively corrupts existing provenance: rebuilt records omit `source` and `private`, so any consolidated record loses its origin tag and privacy flag (`encoding_flow.py:461-471`).

## Architectural Decisions

- **Typed provenance in the record schema, not a side table**: provenance lives directly on `MemoryRecord` as first-class pydantic fields (`types.py:60-73`), making it enforceable by validation and automatically serialized by every backend implementing the `StorageBackend` protocol (`memory/storage/backend.py:44-185`).
- **Provenance doubles as an access-control mechanism**: `source` + `private` implement per-origin visibility filtering at recall time (`unified_memory.py:746-751`), an unusual pairing that gives provenance operational teeth beyond documentation.
- **Content-addressable document identity in RAG**: SHA256(content+metadata) doc ids (`chromadb/utils.py:81-85`) provide dedup integrity but conflate identity with provenance — identical text from different origins collapses to one id.
- **LLM-mediated encoding pipeline**: scope/categories/importance/metadata are inferred by LLM when not supplied (`encoding_flow.py:223-347`), meaning even the *classification* metadata on a memory is itself a machine transformation — and is stored indistinguishably from caller-provided values.
- **Knowledge ingestion decoupled from retrieval typing**: the RAG layer defines provenance-capable containers (`BaseRecord.metadata`, `SearchResult.metadata`) but the knowledge ingestion path opts out by sending bare strings (`knowledge_storage.py:118`).

## Notable Patterns

- **Round-trip discipline via explicit row mappers**: both backends use symmetric `_record_to_row`/`_row_to_record` pairs with a documented empty-string sentinel for `None` source (`lancedb_storage.py:257,285`; `qdrant_edge_storage.py:210,239`).
- **Match-reason transparency**: `MemoryMatch.match_reasons` explains why a memory surfaced ("semantic", "recency", "importance") (`types.py:83-86,374-378`) — a cousin of provenance explaining retrieval cause if not data origin.
- **Structured events around memory I/O**: save/query lifecycle events carry metadata payloads (`events/types/memory_events.py:53-80`), giving observability hooks where transformation logs could attach.
- **Read-barrier write coalescing**: background saves are drained before reads (`unified_memory.py:350-363,711-713`), ensuring provenance-filtered queries see consistent state.
- **Delivery-level file lineage**: the separate `crewai-files` library tracks which provider holds each uploaded file, its id, URI, and expiry (`crewai_files/core/resolved.py:51-67`) — provenance for attachments, though disconnected from memory/knowledge provenance.

## Tradeoffs

- **Privacy filtering vs. recall completeness**: private records are excluded pre-scoring unless source matches (`unified_memory.py:746-751`); simple and safe, but a mismatched `source` string silently hides data with no diagnostic.
- **Simplicity of bare-string knowledge ingestion**: saving `[{"content": doc}]` keeps collections portable and cheap, at the cost of being unable to answer "which document did this come from?" or refresh stale sources — the tradeoff is made implicitly (the unused `metadata` field suggests provenance was planned, `base_knowledge_source.py:28`).
- **Empty-string sentinel for optional source**: avoids nullable columns in LanceDB/Qdrant but means `""` and `None` must be carefully normalized at both boundaries (`lancedb_storage.py:257,285`).
- **utcnow() fallbacks**: defaulting missing timestamps to "now" (`lancedb_storage.py:267`) keeps old rows usable but fabricates freshness, inflating recency scores for legacy data.

## Failure Modes / Edge Cases

- **Provenance reset on consolidation (highest severity)**: when the LLM decides to update an existing memory, the replacement record omits `source` and `private` (`encoding_flow.py:461-471`). A memory saved as `source="user:alice", private=True` becomes `source=None, private=False` after one merge — now visible to every recall, violating the documented access-control contract (`docs/edge/en/concepts/memory.mdx:446-477`).
- **Cross-source contamination in shared collections**: knowledge collection naming is per-crew/agent instance (`knowledge_{collection_name}`, `knowledge_storage.py:71-75`), but chunks within a collection are anonymous; two crews sharing a collection cannot attribute or purge a specific source's documents except by full reset.
- **Unattributed auto-memories pollute scoped recall**: since auto-saves pass no `source` (`base_agent_executor.py:59-63`), HITL-private filtering cannot distinguish agent-derived memories from human-derived ones; a later `recall(source="user:x")` sees agent memories regardless.
- **Silent timestamp fabrication**: deserialized rows missing `created_at` get `utcnow()` (`lancedb_storage.py:265-271`), masking data corruption as fresh memories.
- **Metadata collision on LLM extraction**: caller metadata is dict-merged with LLM-extracted metadata with caller precedence (`encoding_flow.py:330-337`); overlapping keys from different origins overwrite silently with no audit trail.
- **No integrity verification on embeddings**: zero-vector placeholders replace failed/missing embeddings at write time (`lancedb_storage.py:308-310`), so such records remain retrievable with meaningless similarity — no marker distinguishes them.

## Future Considerations

- Fix the consolidation update path to preserve `existing.source` and `existing.private` in `execute_plans` (`encoding_flow.py:461-471`), and add a regression test.
- Populate `source` on framework auto-saves (e.g., `source=f"task:{task.id}"` or `agent:{role}`) so provenance filtering covers machine-generated memories (`base_agent_executor.py:59-63`).
- Extend knowledge ingestion to attach `doc_id`/metadata per chunk — the plumbing already exists in `BaseRecord` and ChromaDB prep (`rag/types.py:19-24`; `chromadb/utils.py:87-104`) — and include source labels in `extract_knowledge_context` output (`knowledge_utils.py:4-12`).
- Add a transformation-history field (e.g., `origin_refs: list[str]`, `transformed_from: str | None`) to `MemoryRecord` and append entries on extraction/consolidation in `EncodingFlow`.
- Replace `utcnow()` fallbacks with explicit nullability or a migration marker (`lancedb_storage.py:265-271`).
- Introduce a trust/authority enum (even a coarse `user > tool > llm-inferred` ordering) consumable by recall ranking, given importance already feeds composite scoring (`types.py:345-380`).

## Questions / Gaps

- No evidence found of any trust/authority classification for context items; searched `trust`, `authority`, `confidence` (context-origin sense), `credib*` across `lib/crewai/src` — all hits were unrelated (A2A status trust, auth profiles, checkpoint guards).
- No evidence found that provenance survives CrewAI's checkpointing/serialization of crews themselves (only memory backends were verified); `crew.py` checkpoint code (`crew.py:439-448`) was not traced to memory-record fidelity.
- Test coverage for `include_private`/source-match recall appears absent; searched all files under `lib/crewai/tests/memory/` for `private`, `include_private`, `source=` — only a storage-schema placeholder matched (`tests/memory/test_qdrant_edge_storage.py:274`). If coverage exists elsewhere (e.g., flow tests), it was outside this search boundary.
- Whether the older entity/short-term memory subsystems (pre-unified-memory) retain provenance was not assessed: they no longer appear under `lib/crewai/src/crewai/memory/` (only unified memory, storage, flows), suggesting removal, but no migration note was located within the source tree.
- The English edge docs reference implementation faithfully for memory, but no equivalent documentation states a position on knowledge-chunk provenance; intent there remains inferred from the unused `metadata` field, not stated.

---

Generated by `dimensions/11.04-context-provenance-and-integrity` against `crewai`.

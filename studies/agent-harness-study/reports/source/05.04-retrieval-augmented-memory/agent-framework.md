# Source Analysis: agent-framework

## Dimension 05.04: Retrieval-Augmented Memory

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (packages/*) and .NET (dotnet/src/*); multi-language monorepo |
| Analyzed | 2026-08-25 |

> **Path convention:** all citations below are relative to the source root
> `studies/agent-harness-study/sources/agent-framework/` (e.g. `python/packages/core/...`,
> `dotnet/src/...`).

## Summary

Retrieval-augmented memory in agent-framework is organized around a **context-provider hook abstraction** rather than a single retriever interface. On the Python side, `ContextProvider.before_run/after_run` (`python/packages/core/agent_framework/_sessions.py:793-870`) is the universal extension point: every retrieval implementation injects results as messages/instructions/tools into the invocation context before the model runs, and persists or extracts memory afterwards. On the .NET side the mirror abstraction is `AIContextProvider` with `ProvideAIContextAsync`/`StoreAIContextAsync` (`dotnet/src/Microsoft.Agents.AI.Abstractions/AIContextProvider.cs:42,137`).

The framework ships several distinct retrieval families:

1. **File-backed conversational memory (Python core harness)** — `MemoryContextProvider` maintains an always-loaded `MEMORY.md` index plus topic files and a raw transcript archive; selection is keyword-overlap scoring over index entries (`python/packages/core/agent_framework/_harness/_memory.py:1079-1098`), with LLM-driven extraction and consolidation pipelines that write the "index".
2. **Vector-store-backed chat history (.NET)** — `ChatHistoryMemoryProvider` upserts every message into any `Microsoft.Extensions.VectorData` store and retrieves by similarity search combined with scope equality filters (`dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProvider.cs:362-440`).
3. **Delegate-based text search (.NET)** — `TextSearchProvider` performs search-before-invoke or exposes search as an on-demand tool (`dotnet/src/Microsoft.Agents.AI/TextSearchProvider.cs:110-131`).
4. **Service-backed RAG/memory integrations (both languages)** — Azure AI Search (semantic + agentic Knowledge Base retrieval), Mem0, Foundry Memory, Redis (BM25/hybrid), and Azure Cosmos DB semantic memory.
5. **Hosted-retrieval delegation** — `get_file_search_tool` / `Content.from_hosted_vector_store` hand indexing, chunking, embedding, and ranking entirely to the model provider.

The unifying design stance is that **retrieval scope (who may see what) is treated as a first-class safety concern**: Mem0 explicitly separates storage scope from retrieval scope so nothing is inherited implicitly (`python/packages/mem0/agent_framework_mem0/_context_provider.py:50-61`); Redis requires at least one scope filter (`python/packages/redis/agent_framework_redis/_context_provider.py:409-412`); the .NET vector provider indexes ApplicationId/AgentId/UserId/SessionId as filterable fields (`dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProvider.cs:128-145`). Provenance is preserved where the backend supplies it (Azure AI Search citation annotations including reranker scores and sensitivity labels, `python/packages/azure-ai-search/agent_framework_azure_ai_search/_context_provider.py:996-1048`) and in the core harness via cross-session origin attribution (`python/packages/core/agent_framework/_harness/_memory.py:1321-1346`). What the framework does **not** provide is its own chunking pipeline or local reranker; those are delegated to backends or absent entirely.

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale:
- The `ContextProvider` contract is small, explicit, and consistently implemented across ~7 Python packages and the .NET side, each with dedicated unit-test suites (e.g. 20 tests for the core memory harness in `python/packages/core/tests/core/test_harness_memory.py`, 48 for Mem0, 43+10 for Redis, 44 for Cosmos; 22 Fact/Theory cases for `ChatHistoryMemoryProviderTests.cs` and 25 for `TextSearchProviderTests.cs`).
- Operational safeguards are concrete: per-topic asyncio locks (`python/packages/core/agent_framework/_harness/_memory.py:1046-1050`), atomic writes (`_memory.py:123-136`), consolidation windows that do not advance on transient failure (`_memory.py:1541-1549`), SDK-preview capability gating (`python/packages/azure-ai-search/agent_framework_azure_ai_search/_context_provider.py:559-584`), log redaction of PII (`TextSearchProvider.cs:94,339`).
- It stops short of 8+: there is no shared retriever/chunker/reranker abstraction (each provider re-implements query construction and result formatting), the flagship core-memory retriever uses naive lexical scoring rather than embeddings, and most providers silently degrade to empty context on retrieval failure.

## Evidence Collected

Every entry cites a path relative to `studies/agent-harness-study/sources/agent-framework/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Context-provider hook (retrieval entry point, Py) | `ContextProvider.before_run/after_run`; provider-scoped state persisted in session state | python/packages/core/agent_framework/_sessions.py:793-870 |
| History provider base + file history store | `HistoryProvider` flags (`load_messages`, `store_inputs`, ...); `InMemoryHistoryProvider`; `FileHistoryProvider` JSONL/msgspec archive | python/packages/core/agent_framework/_sessions.py:943-973, 2087, 2172 |
| Core memory store ABC + file store | `MemoryStore` abstract ops (list/get/write/delete topic, rebuild index, search transcripts); `MemoryFileStore` layout `MEMORY.md` + `topics/` + `transcripts/` | python/packages/core/agent_framework/_harness/_memory.py:559-651, 655-698 |
| Memory index format | Pointer lines `- [topic](topics/<slug>.md): summary`, capped at 200 lines × 150 chars | python/packages/core/agent_framework/_harness/_memory.py:32-38, 322-332 |
| Retriever (core memory) | Keyword-overlap scorer `_topic_score` over topic title+summary vs. input-message word tokens; `_select_topics` keeps top `selection_limit=3` entries with score > 0 | python/packages/core/agent_framework/_harness/_memory.py:1079-1098 |
| Transcript retrieval | `search_transcripts`: casefolded substring scan over JSONL transcript lines, optional per-session filter, limit 20 | python/packages/core/agent_framework/_harness/_memory.py:884-927 |
| Indexing pipeline (memory write path) | LLM extraction prompt → JSON parse → merge into topic records → rebuild index after each turn | python/packages/core/agent_framework/_harness/_memory.py:1398-1470, 1472-1503, 1368-1389 |
| Consolidation (staleness control) | Gated by `consolidation_min_sessions=5` and 24 h interval; window not advanced if all topics fail | python/packages/core/agent_framework/_harness/_memory.py:1505-1573 |
| Provenance (core memory) | Injected memory message carries `origin_session_ids` from contributing sessions; topic records persist `session_ids` | python/packages/core/agent_framework/_harness/_memory.py:1317-1346, 357-386 |
| File-memory harness retriever | `FileMemoryProvider` auto-built capped 50-entry `memories.md` index injected each turn; regex `file_memory_grep` tool | python/packages/core/agent_framework/_harness/_file_memory.py:77-78, 280-298, 472-495 |
| Embedding protocol (Py) | `SupportsGetEmbeddings.get_embeddings(...)` protocol; `BaseEmbeddingClient` ABC | python/packages/core/agent_framework/_clients.py:871-913, 926 |
| Vector-store RAG provider (Azure AI Search) | Hybrid search: text + `VectorizableTextQuery` (server-side vectorizer) or `VectorizedQuery` (client embeddings); semantic config w/ captions; `vector_k = max(top_k, 50)` when semantic config set | python/packages/azure-ai-search/agent_framework_azure_ai_search/_context_provider.py:755-797 |
| Vector-field auto-discovery | Reads index schema, picks single vectorizable field, degrades to keyword-only otherwise | python/packages/azure-ai-search/agent_framework_azure_ai_search/_context_provider.py:676-753 |
| Agentic (multi-hop) retrieval | Knowledge Base creation/reuse, reasoning-effort & output-mode options, preview-SDK gating, per-query `x-ms-query-source-authorization` token header | python/packages/azure-ai-search/agent_framework_azure_ai_search/_context_provider.py:891-956, 559-584, 900-906 |
| Citations/provenance (Azure AI Search) | KB references → `Annotation(type="citation")` with url/title/reranker_score/source_data/sensitivity label/raw repr; semantic mode prefixes `[Source: <doc_id>]` | python/packages/azure-ai-search/agent_framework_azure_ai_search/_context_provider.py:996-1048, 1090-1107 |
| Citation content type (core) | `Annotation` TypedDict (`type="citation"`, title/url/file_id/snippet/annotated_regions) attachable to any Content | python/packages/core/agent_framework/_types.py:387-398 |
| Hosted vector stores (delegated retrieval) | `Content.from_hosted_vector_store`; `get_file_search_tool(vector_store_ids=...)` protocol; Foundry impl validates non-empty `vector_store_ids` | python/packages/core/agent_framework/_types.py:1003-1019, python/packages/core/agent_framework/_clients.py:806-817, python/packages/foundry/agent_framework_foundry/_chat_client.py:395-418 |
| Mem0 scoping model | Storage scope vs. retrieval scope never inherit; empty retrieval scope → warning + no retrieval | python/packages/mem0/agent_framework_mem0/_context_provider.py:44-62, 146-154 |
| Mem0 fan-out retrieval | Independent per-partition searches gathered concurrently; dedupe by memory id; partial-failure tolerance | python/packages/mem0/agent_framework_mem0/_context_provider.py:160-228 |
| Foundry hosted memory | `FoundryMemoryProvider(scope=...)` with debounced `update_delay=300s` writes | python/packages/foundry/agent_framework_foundry/_memory_provider.py:51-141, 156-270 |
| Redis retrieval | BM25 text or hybrid vector-text (`AggregateHybridQuery`, `alpha=0.7`); mandatory scope-filter validation; index schema-compatibility check | python/packages/redis/agent_framework_redis/_context_provider.py:343-412, 244-303 |
| Cosmos DB semantic memory | `search_cosmos(search_terms, user_id, top_k, memory_types, min_confidence)`; user summary injected as *untrusted* user-role context to block stored prompt injection | python/packages/azure-cosmos-memory/agent_framework_azure_cosmos_memory/_context_provider.py:334-412 |
| .NET context provider base | `AIContextProvider.ProvideAIContextAsync/StoreAIContextAsync`; default input filters include only `External`-source messages | dotnet/src/Microsoft.Agents.AI.Abstractions/AIContextProvider.cs:42-78 |
| Message attribution (.NET) | `AgentRequestMessageSourceAttribution` stamped under `_attribution` additional-properties key | dotnet/src/Microsoft.Agents.AI.Abstractions/AgentRequestMessageSourceAttribution.cs:9-22 |
| .NET text-search RAG | `TextSearchProvider`: `BeforeAIInvoke` injection vs `OnDemandFunctionCalling` tool; recent-message memory window; SourceName/SourceLink formatting + citations prompt | dotnet/src/Microsoft.Agents.AI/TextSearchProvider.cs:23-34, 110-131, 206-243, 275-306 |
| .NET vector chat-history memory | Schema with indexed scope fields (ApplicationId/AgentId/UserId/SessionId); embedding field populated from raw text; `SearchAsync(query, top, Filter)` with AND-combined equality predicates | dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProvider.cs:63-81, 128-145, 272-293, 380-420 |
| .NET Mem0 scope enforcement | Init throws unless both storage and search scopes carry ≥1 identifier | dotnet/src/Microsoft.Agents.AI.Mem0/Mem0Provider.cs:107-112 |
| Prompt-injection threat modeling | Explicit security remarks for injected retrieval content in both .NET providers | dotnet/src/Microsoft.Agents.AI/TextSearchProvider.cs:36-46, dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProvider.cs:21-46 |
| Tests (Py, core memory) | e.g. `test_memory_context_provider_marks_cross_session_origins`, `test_memory_file_store_rejects_owner_path_traversal`, `test_memory_consolidation_transient_failure_preserves_state` | python/packages/core/tests/core/test_harness_memory.py:504, 216, 799 |
| Tests (Mem0 scoping) | `test_no_search_scope_skips_retrieval`, `test_search_scope_does_not_inherit_storage_scope` | python/packages/mem0/tests/test_mem0_context_provider.py:345, 372 |
| Tests (.NET retrieval) | `dotnet/tests/Microsoft.Agents.AI.UnitTests/Data/TextSearchProviderTests.cs` (964 lines, 25 cases); `dotnet/tests/Microsoft.Agents.AI.UnitTests/Memory/ChatHistoryMemoryProviderTests.cs` (959 lines, 22 cases) | same paths |

## Answers to Dimension Questions

**1. What can be retrieved?**
Four classes of content: (a) durable conversational memory — curated topic files + `MEMORY.md` index + raw transcripts in the Python harness (`python/packages/core/agent_framework/_harness/_memory.py:31-35`); (b) prior chat messages embedded into an external vector store (.NET `ChatHistoryMemoryProvider`, `dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProvider.cs:16-20`); (c) external documents/knowledge — via Azure AI Search indices/Knowledge Bases, delegate-supplied text search, Mem0, Foundry Memory, Redis, Cosmos DB; (d) provider-hosted corpora through file-search tools bound to hosted vector stores (`python/packages/foundry/agent_framework_foundry/_chat_client.py:395-418`).

**2. How is it indexed?**
Three strategies coexist. (i) *Self-managed markdown*: the harness writes topic records as markdown with slug filenames and rebuilds the pointer index from directory listing on every turn (`_memory.py:814-836`) — no vectors involved. (ii) *LLM extraction*: after each run, an extraction prompt converts the transcript delta into ≤5 JSON memory items merged into topics (`_memory.py:44-56, 1398-1470`); Cosmos DB delegates fact extraction to its toolkit with customizable `.prompty` templates (`python/packages/azure-cosmos-memory/agent_framework_azure_cosmos_memory/_context_provider.py:140-142`). (iii) *Backend-owned embeddings*: Azure AI Search relies on server-side vectorizers (`VectorizableTextQuery`, `_context_provider.py:762-765`) or a client `embedding_function`/`SupportsGetEmbeddings`; the .NET vector provider delegates embedding generation to the `Microsoft.Extensions.VectorData` store configuration while storing raw text in the embedding field (`ChatHistoryMemoryProvider.cs:286`). **No evidence found** of a framework-owned chunker: searched `chunk*` across `python/packages/core/agent_framework` and found only unrelated matches; chunking happens upstream in the search services/hosted stores.

**3. Are retrieval results scoped correctly?**
This is the strongest area. Mem0's split storage/retrieval scope with no implicit inheritance prevents an agent-shared memory from leaking to all users (`python/packages/mem0/agent_framework_mem0/_context_provider.py:50-61`; enforced by tests `python/packages/mem0/tests/test_mem0_context_provider.py:372`). The .NET chat-history provider ANDs equality filters on indexed scope columns before vector search (`ChatHistoryMemoryProvider.cs:380-420`), and Mem0/.NET throws if scopes are missing (`Mem0Provider.cs:107-112`). The Python harness namespaces storage by encoded owner+source components under a resolved base root with traversal rejection (`_memory.py:700-745`; test at `test_harness_memory.py:216`). Residual gaps: the Azure AI Search provider has **no row-level filter parameter at all** — scoping must be baked into the index itself — and the core harness's transcript search is scoped only when the caller passes `session_id` (`_memory.py:903-906`).

**4. Are sources preserved?**
Partially, and inconsistently. Best: agentic retrieval converts KB references into typed citation annotations carrying URL, doc key, reranker score, source data, and even sensitivity labels (`_context_provider.py:1024-1048`), attached to every injected content item (`:1076-1081`); semantic mode embeds `[Source: <doc_id>]` inline (`:1090-1107`); the .NET formatter emits `SourceDocName:`/`SourceDocLink:` per result plus a citations instruction (`TextSearchProvider.cs:292-304`); the core harness attributes injected memory to originating session IDs (`_memory.py:1329-1346`). Weakest: Mem0 joins bare `memory` strings with newlines and discards record ids/metadata at injection time (`python/packages/mem0/agent_framework_mem0/_context_provider.py:230-235`), and Redis/Cosmos inject formatted blobs without per-item provenance.

**5. Can stale or low-quality retrieval be detected?**
Mechanisms exist but are shallow. The harness consolidates topic memories on a min-sessions/time cadence with prompts instructing removal of stale/transient items (`_memory.py:57-68, 1558-1573`), keeps `updated_at` per topic, and refuses to advance the consolidation window when all LLM calls fail (`:1541-1549`). Redis validates live-index schema compatibility against its declared schema before querying (`python/packages/redis/agent_framework_redis/_context_provider.py:254-303`). Mem0 logs an error when *all* partition searches fail ("unable to verify memory state", `:227-228`). However, no provider computes relevance thresholds on injected results (only Cosmos exposes `min_confidence`, applied service-side, `python/packages/azure-cosmos-memory/agent_framework_azure_cosmos_memory/_context_provider.py:370`), and retrieval exceptions are typically swallowed into "return empty context" (e.g. `TextSearchProvider.cs:199-203`), so silent degradation rather than detection is the norm.

## Architectural Decisions

1. **Hook-based composition over a retriever interface.** Instead of a `Retriever` abstraction, the framework standardizes on lifecycle hooks (`before_run`/`after_run`) that may add messages, instructions, tools, or middleware (`python/packages/core/agent_framework/_sessions.py:830-870`). This lets one interface cover pre-fetch RAG (Azure AI Search), on-demand tools (`.NET OnDemandFunctionCalling`), and write-behind memory (all providers). Cost: every provider re-implements query building, formatting, and error policy, so behavior varies across packages.
2. **Index-as-markdown with pointer indirection.** The core harness caps the always-present index (200 lines × 150 chars, `DEFAULT_MEMORY_INDEX_LINE_LIMIT/LINE_LENGTH`, `_memory.py:36-37`) and loads at most 3 topic files per turn (`:38`), bounding token cost deterministically instead of trusting a ranker.
3. **Separate cheap-model consolidation channel.** `consolidation_client` allows running cleanup with a different model than the agent (`_memory.py:957-981`), acknowledging that maintenance quality and interactive latency have different budgets.
4. **Scope-isolation by explicit configuration, not inference.** Both Mem0 implementations make retrieval scope strictly opt-in (Python: warn-and-return; .NET: constructor throw). This is a deliberate cross-language decision documented in package AGENTS.md docs and covered by tests.
5. **Capability negotiation with the retrieval backend.** The Azure AI Search provider detects stable vs preview `azure-search-documents` builds and rejects unsupported options with actionable errors citing the installed version (`_context_provider.py:559-584, 893-898`), avoiding silent server-side failure.
6. **Untrusted-channel framing of retrieved content.** Cosmos DB injects LLM-generated user summaries as user-role messages explicitly labeled "untrusted reference information" to avoid stored-prompt-injection escalation (`python/packages/azure-cosmos-memory/agent_framework_azure_cosmos_memory/_context_provider.py:392-408`), and the .NET providers document indirect-prompt-injection risk in their XML remarks (`TextSearchProvider.cs:36-46`).
7. **Attribution plumbing at the request level (.NET).** `AgentRequestMessageSourceType.External` defaults mean providers only see/store externally-sourced messages unless configured otherwise (`AIContextProvider.cs:54-56`), keeping provider-generated context out of downstream stores by default.

## Notable Patterns

- **Auto-discovery with graceful degradation**: the Azure provider introspects the index schema to find vector/vectorizable fields and falls back to keyword-only search with warnings rather than failing (`_context_provider.py:698-753`).
- **Fan-out + dedupe**: Mem0 queries user/agent partitions independently and merges by id, sidestepping strict-AND filter limitations of the backing API (`_context_provider.py:160-226`).
- **Dual behavior toggle**: the same .NET class serves eager injection (`BeforeAIInvoke`) and agentic tool-call retrieval (`OnDemandFunctionCalling`), refusing invalid combinations loudly (`TextSearchProvider.cs:116-123, 138-141`).
- **Concurrency-safe memory mutation**: per-topic asyncio locks keyed by loop/store/topic (`_memory.py:1025-1050`) plus atomic temp-file writes (`:123-136`); tested under concurrent writers (`test_harness_memory.py:640`) and simulated crash mid-write (`:695`).
- **Unicode-aware lexical matching**: the word regex treats CJK/Cyrillic as keywords so non-English input still scores topics (`_memory.py:71-74`; test `:862`).
- **Markdown hardening**: summary/memory lines are escaped so persisted content cannot forge section delimiters on later parses (`_memory.py:107-120, 457-461`).

## Tradeoffs

- **Lexical vs semantic retrieval in core memory**: keyword overlap is free, debuggable, and dependency-free, but misses paraphrases and ranks only against the 150-char pointer summaries, not file bodies (`_memory.py:1079-1098`).
- **Whole-input concatenation as query**: Azure semantic mode joins all input messages into one query string (`_context_provider.py:663`); simple for single-turn use, diluting multi-turn queries. The agentic mode mitigates this with recent-history windows (`:666`).
- **Silent empty-context fallback**: catching all retrieval errors and proceeding without context maximizes availability but hides broken configurations (e.g. `TextSearchProvider.cs:199-203`); only logs distinguish the states.
- **Delegated vs owned infrastructure**: relying on hosted vector stores removes embedding/chunk maintenance from the framework but means index quality, staleness, and cost controls live outside the repo's testability.
- **Strict scope defaults vs convenience**: requiring explicit `search_*` scopes (Mem0) eliminates a whole leak class but adds setup friction that manifests as a runtime warning the first time retrieval silently does nothing.

## Failure Modes / Edge Cases

- **Extraction poisoning**: memory content originates from LLM output parsed from JSON; malformed payloads are dropped with logged previews (`_memory.py:1427-1459`), but semantically-wrong yet well-formed memories are accepted verbatim — quality control is deferred to consolidation.
- **Consolidation partial failure**: per-topic failures keep the old record and the window stays open for retry (`_memory.py:1541-1549`, test `:799`), but a permanently failing topic retries every turn, adding latency to `after_run`.
- **Owner-ID requirement**: file-backed memory raises at runtime if the session lacks the owner state key, and rejects absolute/traversal owner ids (`_memory.py:700-710`) — misconfigured sessions fail loudly on first write/read.
- **Preview-SDK drift**: agentic options valid on one SDK build raise `ValueError` on another; the provider encodes this boundary explicitly (`_context_provider.py:566-584`) rather than detecting it at call time.
- **PII in traces**: both .NET providers redact queries/results in logs by default via `Redactor`, with opt-in `EnableSensitiveTelemetryData` (`ChatHistoryMemoryProvider.cs:94`; `TextSearchProvider.cs:339`).
- **Cross-session origin correctness**: origin attribution is omitted when all contributors belong to the current session, avoiding spurious provenance (`_memory.py:1321-1327`; test `:550`).

## Future Considerations

- Introduce a minimal shared retriever/result contract (query construction, top-k, filter, result-with-provenance) so per-package formatting and error policies converge; today only the annotation type (`python/packages/core/agent_framework/_types.py:387-398`) is shared.
- Add an optional embedding-based selector for the core harness memory (the `SupportsGetEmbeddings` protocol already exists at `python/packages/core/agent_framework/_clients.py:871`) while retaining lexical fallback.
- Surface retrieval telemetry (result counts, score distributions, empty-result rates) through the existing observability layer; currently only ad-hoc loggers exist (`TextSearchProvider.cs:179-195`).
- Propagate Mem0 record metadata (ids, scores, timestamps) into `Annotation`s so injected memories carry provenance parity with Azure AI Search.
- Define a staleness/TTL story for harness topic files beyond periodic consolidation (e.g., last-access timestamps already exist but are unused for eviction; index truncation at 200 pointers silently drops overflow, `_memory.py:825-836`).

## Questions / Gaps

- **Chunking**: No evidence found of any framework-owned document chunker. Searches for `chunk`/splitter patterns across `python/packages/**` (excluding tests/samples) and `dotnet/src` returned only incidental matches; chunking appears fully delegated to Azure AI Search indexers, hosted vector stores, and the Mem0/Cosmos services.
- **Local reranking**: No evidence found of an in-framework reranker; the only rerank signal is the passthrough `reranker_score` captured from Azure KB references (`_context_provider.py:1029-1030`) and the Redis hybrid `alpha` weight (`python/packages/redis/agent_framework_redis/_context_provider.py:352`).
- **Filter authoring for Azure AI Search**: whether enterprises rely on index-level security trimming (no OData `$filter` parameter exists on the provider's search calls) could not be confirmed from code alone; samples were not exhaustively audited.
- **Evaluation of retrieval quality**: no benchmark/eval harness for retrieval accuracy was located inside the source (the `_evaluation.py` module targets responses, not retrieval); if one exists it lives outside this repository.

---

Generated by `05.04-retrieval-augmented-memory` against `agent-framework`.

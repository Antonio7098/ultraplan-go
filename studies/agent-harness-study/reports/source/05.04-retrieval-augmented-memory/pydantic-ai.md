# Source Analysis: pydantic-ai

## Dimension 05.04: Retrieval-Augmented Memory

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (uv workspace: `pydantic_ai_slim`, `pydantic_graph`, `pydantic_evals`; pytest + VCR + inline-snapshot) |
| Analyzed | 2026-08-25 |

All citations below are workspace-relative paths into the selected source directory `studies/agent-harness-study/sources/pydantic-ai/`.

## Summary

pydantic-ai treats retrieval-augmented memory as three deliberately separated surfaces rather than one integrated RAG subsystem:

1. **Retrieval over the agent's own tool corpus ("tool search").** The only retrieval subsystem implemented end-to-end inside the framework. A deferred-tool corpus is searched by a keyword-overlap algorithm (`pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:451-483`), a provider-native server-side strategy (Anthropic BM25/regex, OpenAI Responses — `pydantic_ai_slim/pydantic_ai/native_tools/_tool_search.py:46-63,102-156`), or a user-supplied callable (`pydantic_ai_slim/pydantic_ai/capabilities/_tool_search.py:104-121`). Results are provenance-tracked as typed message parts that survive serialization, cross-provider replay, and compaction boundaries (`pydantic_ai_slim/pydantic_ai/_tool_search.py:121-362,471-575`).

2. **An embeddings abstraction with no store.** `Embedder` (`pydantic_ai_slim/pydantic_ai/embeddings/__init__.py:143`) plus an `EmbeddingModel` ABC (`pydantic_ai_slim/pydantic_ai/embeddings/base.py:8`) cover six providers (OpenAI, Cohere, Google, Bedrock, VoyageAI, SentenceTransformers; listed at `pydantic_ai_slim/pydantic_ai/embeddings/base.py:12-19`). It produces vectors and rich per-request metadata but no vector index, chunker, or reranker: "Pydantic AI does not prescribe a vector database" (`docs/embeddings.md:133-135`) and "Pydantic AI does not ship a reranker provider class" (`docs/embeddings.md:873`).

3. **Delegation of managed RAG to providers.** `FileSearchTool` hands chunking, embedding, storage, and retrieval to OpenAI Responses / Gemini / xAI collections (`pydantic_ai_slim/pydantic_ai/native_tools/__init__.py:630-676`). Web retrieval (`WebSearchTool`, domain allow/block filters at `pydantic_ai_slim/pydantic_ai/native_tools/__init__.py:167-183`) and its local fallbacks in `pydantic_ai_slim/pydantic_ai/common_tools/` (duckduckgo.py, exa.py, tavily.py) are retrievers over external knowledge rather than memory.

The design stance is consistent with the repo's stated philosophy of "strong primitives ... over every single possible battery included" (`AGENTS.md`, Philosophy section): the framework owns retrieval *plumbing* (embedding calls, search dispatch, history/provenance), while indexing strategy stays application-side, documented in `docs/embeddings.md:96-149` and demonstrated in `examples/pydantic_ai_examples/rag.py`.

## Rating

**7/10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale:
- The surfaces that exist are precise and well-guarded. Tool search has typed interfaces (`ToolSearchArgs`/`ToolSearchReturnContent` at `pydantic_ai_slim/pydantic_ai/_tool_search.py:77-118`), provider-adaptive strategy selection with fail-fast semantics for unsupported strategies (`pydantic_ai_slim/pydantic_ai/capabilities/_tool_search.py:168-177`), retry budgets (`pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:314-324`), and ~199 tests in `tests/test_tool_search.py` plus a pydantic-evals-based quality evaluation across three models (`tests/test_tool_search.py:503-537`). Embeddings have 117 tests in `tests/test_embeddings.py`, OTel instrumentation with cost metrics (`pydantic_ai_slim/pydantic_ai/embeddings/instrumented.py:59-180`), and a deterministic test model (`pydantic_ai_slim/pydantic_ai/embeddings/test.py:22`).
- It does not reach 8+ because document-level retrieval — the heart of classic RAG — has no framework implementation to safeguard: no chunker, no vector store interface, no similarity search, no reranker, and no freshness/staleness tracking over retrieved documents. Those exist only as documentation guidance (`docs/embeddings.md:112-149`) and example code (`examples/pydantic_ai_examples/rag.py`), so the harness cannot itself guarantee scoped, provenance-preserving retrieval over documents.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Retriever: tool search toolset | `ToolSearchToolset` wraps any toolset; splits visible vs deferred vs searchable corpus | `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:276-381` |
| Retriever: default algorithm | Keyword-overlap scoring over tokenized name+description; descending sort, `max_results=10` cap | `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:95-129,451-483` |
| Retriever: custom callable contract | `ToolSearchFunc(ctx, queries, tools) -> names`, sync or async | `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/native_tools/_tool_search.py:65-77` |
| Retriever: native server-side strategies | Anthropic `'bm25'`/`'regex'`, OpenAI Responses `tool_search`; adapter mapping | `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/native_tools/_tool_search.py:46-52,102-156`; `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/models/anthropic.py:1426-1439,1541-1542` |
| Embedding abstraction | `EmbeddingModel` ABC with abstract `embed(inputs, input_type)` and shared `prepare_embed` normalization | `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/embeddings/base.py:8-93` |
| High-level embedder | `Embedder` with model inference from string ids, settings merge, override context manager, sync wrappers | `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/embeddings/__init__.py:143-391,83-139` |
| Query/document asymmetry | `embed_query()` vs `embed_documents()` set `EmbedInputType='query'\|'document'` because some models optimize differently per role | `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/embeddings/result.py:11-18`; `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/embeddings/__init__.py:266-300` |
| Embedding config | `EmbeddingSettings` TypedDict (`dimensions`, `truncate`) plus provider-prefixed subclasses; known-model-name Literal list | `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/embeddings/settings.py:4-50`; `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/embeddings/__init__.py:36-76` |
| Task conditioning (Google) | Symmetric/asymmetric task types (`clustering`, `sentence similarity` vs retrieval tasks) mapped onto Gemini embedding requests | `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/embeddings/google.py:59-84` |
| Vector normalization (Bedrock) | Titan `bedrock_titan_normalize` normalizes vectors "for direct cosine similarity calculations"; batch concurrency limits for single-text Nova models | `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/embeddings/bedrock.py:108-114,194-199,231-273` |
| Indexing pipeline (none in-framework; documented) | Explicit guidance: pick a vector DB (FAISS/LanceDB/pgvector/managed), dimension must match embedding output, re-embed on config change | `studies/agent-harness-study/sources/pydantic-ai/docs/embeddings.md:133-143` |
| Chunking (application-side) | Chunking strategy table (structure / fixed-size windows / semantic / LLM-assisted) with tradeoffs; recommendation to keep source metadata with each chunk | `studies/agent-harness-study/sources/pydantic-ai/docs/embeddings.md:112-131` |
| Managed-RAG delegation | `FileSearchTool(file_store_ids, max_num_results, instructions, retrieval_mode='hybrid'\|'semantic'\|'keyword')` — provider handles chunking/embeddings/storage/retrieval | `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/native_tools/__init__.py:630-676` |
| Retrieval filters (web) | `WebSearchTool.allowed_domains` / `blocked_domains` (mutually exclusive on Anthropic); same filters + `enable_citations` on web fetch | `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/native_tools/__init__.py:167-183,392-415` |
| Corpus scoping rules | Deferred tools gated by on-demand capabilities are excluded from the searchable corpus; reserved name `search_tools` rejected; capability-gated tools load via capability, never via query | `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:9-16,340-343,354-360` |
| Reranking (in-framework) | Score = \|query tokens ∩ tool tokens\|; undiscovered-first primary key, relevance tiebreak before `max_results` trim | `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:465-482` |
| Reranking (documented pattern, bring-your-own) | Two-stage retrieval with cross-encoder rerankers; "Pydantic AI does not ship a reranker provider class" | `studies/agent-harness-study/sources/pydantic-ai/docs/embeddings.md:869-903` |
| Provenance: typed discovery parts | `ToolSearchCallPart`/`ToolSearchReturnPart` and `NativeToolSearch*Part` carry typed args/content with `tool_kind='tool-search'` discriminator | `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/_tool_search.py:65-118,121-229,242-362` |
| Provenance: cross-provider replay | `synthesize_local_tool_search_messages` translates native parts into local-shape parts when replaying history on another provider, preserving call ids and content | `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/_tool_search.py:442-575` |
| Provenance: legacy histories | Pre-typed histories read via validated `metadata['discovered_tools']` sideband so old discoveries still surface | `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:77-92,243-252` |
| Provenance vs compaction | Derived discovery state recomputed only from `post_compaction_window(messages)` — compaction resets what the model can be said to have "seen" | `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:196-230`; `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/messages.py:2774-2814` |
| Embedding result metadata | `EmbeddingResult` carries inputs, input_type, model/provider names, timestamp, usage, `provider_response_id`, and cost via genai-prices | `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/embeddings/result.py:53-120` |
| Observability | `InstrumentedEmbeddingModel` emits OTel spans/metrics incl. dimensions count, tokens, cost; optional content capture | `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/embeddings/instrumented.py:59-180` |
| Retrieval-quality evaluation | `test_tool_search_eval` runs 4 scenarios × 3 models through pydantic-evals with assertions `used_search_tools`, `found_expected_tools`, `reasonable_usage`, `keyword_count` | `studies/agent-harness-study/sources/pydantic-ai/tests/test_tool_search.py:432-537` |
| Behavioral tests | Case-insensitivity, description matching, specificity preference, substring exclusion, empty-query `ModelRetry`, pagination via repeated searches, undiscovered-first trimming | `studies/agent-harness-study/sources/pydantic-ai/tests/test_tool_search.py:699-984` |
| Portability tests | Record/replay matrix across anthropic-native/openai-native/local-fallback shapes onto different target providers | `studies/agent-harness-study/sources/pydantic-ai/tests/test_tool_availability_portability.py:156-295,460-560` |
| End-to-end RAG reference implementation | pgvector HNSW index, section-level chunks, embeddings incl. path+title in embedded content, retrieve tool returns title+URL+content | `studies/agent-harness-study/sources/pydantic-ai/examples/pydantic_ai_examples/rag.py:55-83,113-164,218-230` |
| Empty-result semantics | Shaped `{discovered_tools: [], message}` return tells the model not to retry identical keywords without spending retries | `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:508-521`; `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/_tool_search.py:56-62` |

## Answers to Dimension Questions

### 1. What can be retrieved?

Three kinds of corpora, with sharply different ownership:

- **Tool definitions**: deferred tools (`defer_loading=True`) form a searchable corpus discovered via `search_tools` or provider-native search (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:277-287,326-360`). This is the framework's flagship retrieval feature.
- **Arbitrary text via embeddings**: any string corpus the application embeds with `Embedder.embed_documents()` (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/embeddings/__init__.py:284-300`); the framework stops at vector production.
- **Provider-managed files and the web**: `FileSearchTool` over uploaded file stores (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/native_tools/__init__.py:630-649`) and `WebSearchTool` over the open web with domain filters (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/native_tools/__init__.py:167-183`).

Notably absent: retrieval over prior messages beyond the compaction window, over traces/spans, or over code. No evidence found of a document-memory retriever in-framework (searched `pydantic_ai_slim/` for `memory`, `retriev`, `vector_store`, `VectorStore`; only tool-search, embeddings, and native tools matched).

### 2. How is it indexed?

- Tool search has **no persistent index**: queries and tool name/descriptions are tokenized on the fly into lowercase alphanumeric-token sets (`_SEARCH_TOKEN_RE`, `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:95-106,431-436`). Native strategies delegate indexing to providers (Anthropic BM25/regex server-side; `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/native_tools/_tool_search.py:115-117`).
- For embeddings, indexing is entirely out of scope by design; the docs specify the compatibility constraint (index dimension must match embedding output; re-embed after config change) instead of an indexer (`studies/agent-harness-study/sources/pydantic-ai/docs/embeddings.md:141`).
- The reference RAG example indexes pre-split doc sections into pgvector with an HNSW L2 index, embedding `path + title + content` per section (`studies/agent-harness-study/sources/pydantic-ai/examples/pydantic_ai_examples/rag.py:182-183,218-230`).

### 3. Are retrieval results scoped correctly?

Yes, and scoping is unusually carefully engineered:

- Only non-capability-gated deferred tools enter the corpus; capability-owned tools must be loaded by id, never found by query (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:347-360`).
- Discovery state derives exclusively from the post-compaction message window, so results never claim visibility the model lacks after compaction (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/messages.py:2781-2787`).
- Custom callables' outputs are validated against known corpus names and trimmed to `max_results` (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:485-506`).
- Web retrieval is scopeable via allow/block domain lists (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/native_tools/__init__.py:167-183`); xAI collections search adds `retrieval_mode='hybrid'|'semantic'|'keyword'` (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/native_tools/__init__.py:667-673`).

For application-built vector stores, scoping (permissions/metadata filters) is the user's responsibility — the docs explicitly recommend storing permissions metadata with chunks (`studies/agent-harness-study/sources/pydantic-ai/docs/embeddings.md:106`), but nothing enforces it.

### 4. Are sources preserved?

Strongly on the tool-search surface, partially elsewhere:

- Every discovery round-trip is persisted as typed parts in history (`ToolSearchCallPart` → `ToolSearchReturnPart` with `discovered_tools` lists), tagged `tool_kind='tool-search'` so UIs and adapters can identify them regardless of execution path (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/_tool_search.py:130-132,252-254,315-331`).
- Discoveries survive provider handoff: `synthesize_local_tool_search_messages` rewrites foreign-provider native parts into local shape, preserving tool_call_id, content, and metadata (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/_tool_search.py:454-468,471-502`).
- Embedding results keep full request provenance (model, provider, timestamp, usage, response id — `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/embeddings/result.py:61-83`), though there is no store to attach it to.
- Document retrieval provenance is convention-based: the RAG example returns title + URL with each retrieved chunk (`studies/agent-harness-study/sources/pydantic-ai/examples/pydantic_ai_examples/rag.py:80-83`), and docs instruct keeping "a link or identifier for the full document with every chunk" (`studies/agent-harness-study/sources/pydantic-ai/docs/embeddings.md:129`). Anthropic web fetch can attach citations to fetched content (`enable_citations`, `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/native_tools/__init__.py:412-415`).

### 5. Can stale or low-quality retrieval be detected?

Partially, with real investment in the tool-search case:

- **Quality evaluation exists**: `test_tool_search_eval` runs scenario-based pydantic-evals against recorded sessions for three models, asserting expected tools were found and usage was reasonable (`studies/agent-harness-study/sources/pydantic-ai/tests/test_tool_search.py:503-537`).
- **Empty-result feedback loops are closed**: a shaped empty return with a canonical "no matching tools" message prevents same-keyword retries (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:508-521`); blank queries raise `ModelRetry` (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:442-444`).
- **History corruption self-heals**: a stripped reveal exchange is recoverable by re-searching (`studies/agent-harness-study/sources/pydantic-ai/tests/test_tool_search.py:986-1050`), and legacy metadata shapes are validated before use (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:243-252`).
- **Embedding calls are observable**: spans carry input counts, dimensions, token usage, and cost (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/embeddings/instrumented.py:80-107,157-174`).
- **No detection mechanism exists for document-retrieval staleness/quality**: no freshness metadata schema, no drift detection when embedding models change beyond a docs admonition to re-embed (`studies/agent-harness-study/sources/pydantic-ai/docs/embeddings.md:93-94,141`), and retrieval evaluation is recommended but delegated to Pydantic Evals application-side (`studies/agent-harness-study/sources/pydantic-ai/docs/embeddings.md:145-149`).

## Architectural Decisions

1. **Retrieval over tools is a first-class framework feature; retrieval over documents is not.** The `ToolSearch` capability is auto-injected into every agent with zero overhead when no deferred tools exist (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/capabilities/_tool_search.py:39-40`), while document RAG is explicitly left to applications (`studies/agent-harness-study/sources/pydantic-ai/docs/embeddings.md:100`).

2. **Provider-adaptive strategy lattice with explicit commitment semantics.** `None` picks the best available strategy per provider, `'keywords'` forces local, `'bm25'`/`'regex'` force named-native and hard-fail where unsupported (`optional=False` so `_resolve_request_tools` raises rather than silently substituting an algorithm — `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/capabilities/_tool_search.py:155-177`; emission suppression via `enable_fallback=False` at `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/capabilities/_tool_search.py:179-203`).

3. **Provenance lives in the message history, not in instance state.** Derived state (discovered tool names) is recomputed from typed history parts within the compaction window rather than remembered on objects, which makes it survive serialization, failover, and mid-run model switches (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/messages.py:2781-2800`).

4. **Query/document input-type split exposed at the API level.** `embed_query` vs `embed_documents` encode the asymmetric-embedding-model reality (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/embeddings/result.py:11-18`), and Google's task conditioning maps retrieval vs clustering/similarity intents onto provider parameters (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/embeddings/google.py:59-84`).

5. **Cache-aware retrieval wire design.** The local `search_tools` tool stays registered even after everything is discovered so the prompt-cache prefix is not invalidated mid-conversation (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:362-367`), and client-executed native modes keep the tool list byte-stable across discovery rounds (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/capabilities/_tool_search.py:110-114`).

## Notable Patterns

- **Typed-part narrowing registries**: base `ToolCallPart`s are promoted to typed tool-search subclasses via `tool_kind`-keyed narrowers and serializer tags, avoiding name-based dispatch collisions with user tools (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/_tool_search.py:401-420`).
- **Undiscovered-first ranking**: relevance is only a tiebreak under an "is it new?" primary key, so already-available tools can't crowd new matches out of the `max_results` window (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:478-482`); repeated searches paginate a large corpus (test at `studies/agent-harness-study/sources/pydantic-ai/tests/test_tool_search.py:923-957`).
- **Schema derivation reuse**: the `search_tools` function's JSON schema comes from a real `Tool(...)` signature so it matches how all other tools are declared; custom parameter descriptions splice into the cached schema (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:147-193`).
- **Backward-compatible readers**: legacy `metadata['discovered_tools']` sidebands validated through Pydantic before being trusted (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:80-92`).
- **Provider quirk isolation**: e.g., legacy Bedrock clients reject BM25, so strategy resolution downgrades/errors explicitly (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/models/anthropic.py:288-293,1426-1439`), and Anthropic's empty `tool_result.content` rejection is handled by substituting the canonical no-matches message (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/_tool_search.py:56-62`).

## Tradeoffs

- **Lexical-only local search**: the built-in keyword-overlap algorithm cannot match synonyms or paraphrase; word-boundary tokenization deliberately refuses substring hits (`me` ≠ `comment`), trading recall for precision (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:100-105`). Semantic fallback requires either native BM25 or a user-supplied embedding-backed callable.
- **No vector store abstraction**: maximum flexibility for users, but every RAG app re-implements store/retrieve plumbing and none of it gets framework safeguards (tests, instrumentation) — the framework's own quality bar applies only to embeddings generation.
- **Named-native strictness vs availability**: forcing `'bm25'` fails hard on OpenAI and legacy Bedrock rather than degrading — correctness of intent over convenience (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/capabilities/_tool_search.py:168-177`).
- **Compaction resets discovery**: after compaction, previously discovered tools may need re-discovery (an admitted under-count once permitted only a redundant idempotent search; now availability gating makes the boundary exact but lossy — `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:196-205`).
- **Managed RAG portability**: `FileSearchTool` fields like `retrieval_mode` are xAI-only (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/native_tools/__init__.py:651-673`), so behavior differs per provider despite one type.

## Failure Modes / Edge Cases

- **Malformed search arguments** consume the tool retry budget (bare string instead of list, blank queries), with precedence `tool.max_retries → toolset.max_retries → ctx.max_retries` (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:314-324`); a no-match search never spends retries.
- **Reserved-name collision**: a user tool named `search_tools` raises `UserError` instead of silently shadowing the retriever (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:340-343`).
- **Cross-provider history replay mismatches**: bm25 emits `{"query": "..."}` while regex emits different keys; adapters normalize both into the canonical `queries` list and rebuild on emit, including replaying `regex` for clients that can't support `bm25` (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/models/anthropic.py:2007-2022`; `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/_tool_search.py:88-96`).
- **Streaming-partial states**: typed accessors return `None`/`[]` for unparsed streaming args instead of throwing (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/_tool_search.py:154-189`).
- **Dimension mismatch on embedding-config change**: documented as requiring full re-index; undetectable by the library since it never sees the store (`studies/agent-harness-study/sources/pydantic-ai/docs/embeddings.md:141`).
- **Custom search functions returning unknown names**: silently filtered rather than erroring, which could mask a misconfigured retriever (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:499-506`).

## Future Considerations

- Local BM25/TF-IDF/regex strategies are anticipated by the forward-compat `Literal` scaffolding (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/native_tools/_tool_search.py:61-63`); if ported locally, named strategies could honor user intent on all providers (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/capabilities/_tool_search.py:172-174`).
- `post_compaction_window` tracking improvements are flagged in-docstring as needed to fix rediscovery refusal semantics precisely (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:201-205`).
- A first-party reranker or vector-store interface would close the largest gap between the embeddings module and an actual retrieval pipeline; today both ends of that bridge are documented patterns (`studies/agent-harness-study/sources/pydantic-ai/docs/embeddings.md:869-903`).

## Questions / Gaps

- **No long-term/document memory retriever**: searches for `memory`, `vector_store`, `VectorStore`, `chunker`, `rerank` implementations in `pydantic_ai_slim/` returned only embeddings, tool-search, and provider-delegation code; retrieval over prior messages beyond the compaction window is not implemented (compaction summarizes rather than indexes). Dimension step 3's "chunking/embedding strategy" therefore has no in-framework answer beyond docs.
- **No similarity metric or top-k search utility ships with the embeddings module**: cosine/L2 choices live in user-selected stores (`studies/agent-harness-study/sources/pydantic-ai/examples/pydantic_ai_examples/rag.py:77,229` uses pgvector `<->` L2).
- **No staleness/freshness metadata standard** for retrieved content; nothing marks when an indexed fact was captured or whether the embedding model that produced stored vectors still matches current config.
- **Uncited claim boundary**: whether the keyword-overlap default will remain indefinitely or be replaced by local BM25 is explicitly marked as changeable (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/native_tools/_tool_search.py:57-59`); downstream harnesses should pin `'keywords'` if they depend on its behavior.

---

Generated by `dimensions/05.04-retrieval-augmented-memory` against `pydantic-ai`.

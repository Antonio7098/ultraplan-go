# Source Analysis: crewai

## Dimension 20.02 — Caching, Batching, and Reuse

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (pydantic-based multi-agent framework; monorepo under `lib/` with `lib/crewai`, `lib/crewai-tools`, `lib/crewai-files`, etc.) |
| Analyzed | 2026-08-26 |

## Summary

CrewAI implements caching at two layers of the agent loop and batching in its memory pipeline, but has no application-level model-response cache, no embedding-reuse cache, and no retrieval cache.

1. **Tool-result cache (opt-in).** A thread-safe in-memory `CacheHandler` (`lib/crewai/src/crewai/agents/cache/cache_handler.py:10-60`) stores tool outputs keyed by `"{tool}-{input}"`. It is wired through a shared `ToolsHandler` (`lib/crewai/src/crewai/agents/tools_handler.py:26-52`) into every tool-execution path. Since EPD-180 it is deliberately **opt-in** via `Crew(cache=True)` or `Agent(cache=True)` (`lib/crewai/src/crewai/crew.py:229-238`, `lib/crewai/src/crewai/agent/core.py:411-434`), and declared failures are never cached (`cache_handler.py:38-41`).
2. **Provider prompt caching (explicit markers).** A provider-agnostic `mark_cache_breakpoint` flag (`lib/crewai/src/crewai/llms/cache.py:24-37`) is stamped on the stable system/user prompts by both agent executors; the Anthropic adapter translates the marker into `cache_control: {"type": "ephemeral"}` blocks (`lib/crewai/src/crewai/llms/providers/anthropic/completion.py:758-953`) while other providers strip it. Cached-token accounting is propagated into usage metrics across all five first-party providers.
3. **Batch-native memory pipeline.** `EncodingFlow` embeds all items with ONE batched embedder call, dedups intra-batch duplicates via cosine similarity, runs consolidation LLM calls concurrently, then re-embeds updates in one batch and bulk-inserts (`lib/crewai/src/crewai/memory/encoding_flow.py:112-152`, `414-481`). Recall batch-embeds distilled sub-queries (`lib/crewai/src/crewai/memory/recall_flow.py:189-242`).

The design philosophy is correctness-first: repeated identical requests do **not** avoid full cost by default — tool calls re-execute unless caching is opted in, and identical completions are never served from an application cache. Where caching exists it is well-tested and guarded; where it is absent the absence is mostly deliberate rather than accidental.

## Rating

**Score: 6 / 10**

Rationale: The implemented mechanisms are high quality — explicit interfaces (`CacheHandler`, `mark_cache_breakpoint`), operational safeguards (failures never cached, opt-in default for live-data safety, non-mutating message formatting), and strong regression tests (`lib/crewai/tests/test_tool_cache_default.py:130-140`, `lib/crewai/tests/llms/test_prompt_cache.py:14-191`). But coverage is inconsistent across the dimension's scope: three of five areas (model-response cache, embedding reuse, retrieval cache) have no evidence of implementation, the tool cache is unbounded and process-local, cache keys are order-sensitive raw JSON strings, prompt-cache stamping is missing on the `LiteAgent` path, and user-facing docs contradict the code default (`docs/edge/en/concepts/crews.mdx:25` claims "Defaults to `True`" while `lib/crewai/src/crewai/crew.py:229-230` sets `default=False`). This matches "present but inconsistent, weakly documented, or fragile."

## Evidence Collected

All paths are workspace-relative from the selected source directory root `studies/agent-harness-study/sources/crewai`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Tool-result cache store | `CacheHandler`: in-memory dict keyed `f"{tool}-{input}"`, guarded by `RWLock`; refuses to store `ToolFailure` results | `lib/crewai/src/crewai/agents/cache/cache_handler.py:20-44` |
| Thread-safety primitive | Reader-writer lock allowing concurrent reads, exclusive writes, writer priority | `lib/crewai/src/crewai/utilities/rw_lock.py:12-81` |
| Cache write path | `ToolsHandler.on_tool_use` serializes call arguments (JSON or str) as key part; skips caching the `Hit Cache` tool itself | `lib/crewai/src/crewai/agents/tools_handler.py:40-52` |
| Cache read paths (ReAct loop) | Sync + async `ToolUsage.execute` consult `tools_handler.cache.read(...)` before executing; `from_cache` tracked | `lib/crewai/src/crewai/tools/tool_usage.py:301-312`, `lib/crewai/src/crewai/tools/tool_usage.py:558-569` |
| Cache read paths (native tool calling) | Experimental executor + native-call helper read cached result and format `from_cache` into events | `lib/crewai/src/crewai/experimental/agent_executor.py:1985-1998`, `lib/crewai/src/crewai/utilities/agent_utils.py:1706-1712` |
| Per-tool write gate | Tools expose `cache_function` (default always-True); result consulted before `on_tool_use(..., should_cache=...)` | `lib/crewai/src/crewai/tools/base_tool.py:81-83`, `lib/crewai/src/crewai/tools/base_tool.py:176-177`, `lib/crewai/src/crewai/tools/tool_usage.py:376-391` |
| Opt-in default (safety) | `Crew.cache` defaults `False` with live-data warning in description; handler distributed to agents at validation | `lib/crewai/src/crewai/crew.py:229-238`, `lib/crewai/src/crewai/crew.py:758-763` |
| Agent-level opt-in wiring | Standalone agent creates handler only when constructed with `cache=True`/`cache_handler`; copies never inherit caching | `lib/crewai/src/crewai/agent/core.py:411-434`, `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:280-287`, `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:804-813` |
| Manager-agent parity | Hierarchical manager receives the same shared handler when `Crew(cache=True)` | `lib/crewai/src/crewai/crew.py:1546-1548` |
| Failure-not-cached safeguard + tests | `add()` early-returns on `ToolFailure`; dedicated tests assert failures are refused and successes stored | `lib/crewai/src/crewai/agents/cache/cache_handler.py:38-41`, `lib/crewai/tests/tools/test_tool_failure.py:915-930` |
| Opt-in regression tests (EPD-180) | Default re-executes identical tool calls; `Crew(cache=True)` dedupes to one execution (offline scripted LLM) | `lib/crewai/tests/test_tool_cache_default.py:130-140` |
| Cross-task reuse test | `test_crew.py` asserts exactly one `CacheHandler.add` across 4 tasks sharing identical tool args | `lib/crewai/tests/test_crew.py:2250-2267` |
| Agent-facing cache reader tool | `CacheTools.hit_cache` lets the model read prior results via `"tool:<name>|input:<args>"` keys | `lib/crewai/src/crewai/tools/cache_tools/cache_tools.py:8-28` |
| Cache observability | `from_cache: bool` field on tool usage events surfaced to the event bus | `lib/crewai/src/crewai/events/types/tool_usage_events.py:68`, `lib/crewai/src/crewai/utilities/agent_utils.py:1855` |
| Prompt-cache marker API | Provider-neutral `CACHE_BREAKPOINT_KEY` flag; `mark_cache_breakpoint` returns new dict, `strip_cache_breakpoint` idempotent | `lib/crewai/src/crewai/llms/cache.py:24-37` |
| Breakpoint stamping sites | Both executors mark end-of-system (per-agent stable prefix) and end-of-user (per-task prefix) messages | `lib/crewai/src/crewai/agents/crew_agent_executor.py:180-204`, `lib/crewai/src/crewai/experimental/agent_executor.py:310-335` |
| Anthropic adapter translation | Reads flags pre-strip, matches marked content after role-coalescing, stamps `cache_control: ephemeral` on system block + matched user message (max 4 breakpoints noted) | `lib/crewai/src/crewai/llms/providers/anthropic/completion.py:758-796`, `lib/crewai/src/crewai/llms/providers/anthropic/completion.py:898-934`, `lib/crewai/src/crewai/llms/providers/anthropic/completion.py:936-953` |
| Marker stripping without mutation | `BaseLLM._format_messages` drops the flag in a copy only — executor message buffers keep markers across ReAct iterations | `lib/crewai/src/crewai/llms/base_llm.py:825-850` |
| Prompt-stability engineering | Date injection rendered at prompt tail to preserve a usable cache anchor; metadata-only skills kept stable as anchor | `lib/crewai/src/crewai/utilities/prompts.py:143-163`, `lib/crewai/src/crewai/utilities/prompts.py:165-190` |
| Cached-token accounting | All providers parse cached/cache-creation token counts; aggregated per LLM instance and cost process | `lib/crewai/src/crewai/llms/providers/anthropic/completion.py:1971-1983`, `lib/crewai/src/crewai/llms/providers/openai/completion.py:1544-1545`, `lib/crewai/src/crewai/llms/providers/gemini/completion.py:1412-1421`, `lib/crewai/src/crewai/llms/providers/bedrock/completion.py:2072-2083`, `lib/crewai/src/crewai/llms/providers/azure/completion.py:1342-1356`, `lib/crewai/src/crewai/llms/base_llm.py:969-971`, `lib/crewai/src/crewai/utilities/token_counter_callback.py:62-65` |
| Azure cache-key passthrough | Optional `prompt_cache_key` forwarded as model extra | `lib/crewai/src/crewai/llms/providers/azure/completion.py:689-691` |
| Prompt-cache tests | Marker purity, no-mutation across repeated formats, stable-user vs volatile-tool-result stamping, assistant markers ignored, unmarked → no `cache_control`, OpenAI strips marker | `lib/crewai/tests/llms/test_prompt_cache.py:14-27`, `lib/crewai/tests/llms/test_prompt_cache.py:30-50`, `lib/crewai/tests/llms/test_prompt_cache.py:53-117`, `lib/crewai/tests/llms/test_prompt_cache.py:119-179`, `lib/crewai/tests/llms/test_prompt_cache.py:182-191` |
| Batched embedding helper | `embed_texts(embedder, texts)` performs ONE embedder call for all valid texts and normalizes output | `lib/crewai/src/crewai/memory/types.py:312-339` |
| Batch-native encoding pipeline | Step 1 single batch embed; step 5 batch re-embed of updates + bulk insert; documented "ONE embedder call ... ONE bulk storage write" | `lib/crewai/src/crewai/memory/encoding_flow.py:112-119`, `lib/crewai/src/crewai/memory/encoding_flow.py:371-481`, `lib/crewai/src/crewai/memory/encoding_flow.py:76-83` |
| Intra-batch deduplication | Pairwise cosine similarity against configurable `batch_dedup_threshold` drops near-exact duplicates before any LLM/storage work | `lib/crewai/src/crewai/memory/encoding_flow.py:121-152`, `lib/crewai/src/crewai/memory/types.py:203-210` |
| Consolidation action dedup | Multiple items targeting the same existing record collapse to first-wins delete/update to prevent write conflicts | `lib/crewai/src/crewai/memory/encoding_flow.py:371-412` |
| Recall-side batching + fast path | Sub-queries embedded in single batch call; short queries skip LLM query analysis entirely (~1-3 s saved per recall) | `lib/crewai/src/crewai/memory/recall_flow.py:180-242`, `lib/crewai/src/crewai/memory/types.py:278-286` |
| Concurrent (not batched) memory LLM calls | Field-resolution + consolidation LLM calls run individually but concurrently via `ThreadPoolExecutor(max_workers=10)` | `lib/crewai/src/crewai/memory/encoding_flow.py:223-261` |
| Skill artifact cache | `SkillCacheManager` caches downloaded skill archives at `~/.crewai/skills/{org}/{name}/`; version-pinned lookups miss instead of loading wrong version; corrupted entries read as misses | `lib/crewai/src/crewai/skills/cache.py:35-66`, `lib/crewai/src/crewai/skills/cache.py:68-82` |
| Adjacent infra TTL caches (non-model) | aiocache-backed file store (TTL 3600); MCP tool-schema caches (300 s); A2A agent-card TTL memoization; JWKS cache | `lib/crewai/src/crewai/utilities/file_store.py:20-31`, `lib/crewai/src/crewai/mcp/client.py:51-52`, `lib/crewai/src/crewai/mcp/tool_resolver.py:42`, `lib/crewai/src/crewai/a2a/utils/agent_card.py:107-141`, `lib/crewai/src/crewai/a2a/utils/agent_card.py:226`, `lib/crewai/src/crewai/a2a/auth/server_schemes.py:238-259` |
| Telemetry event batching | `TraceBatch` aggregates trace events per execution session and flushes/finalizes as one payload (not model calls) | `lib/crewai/src/crewai/events/listeners/tracing/trace_batch_manager.py:40-60`, `lib/crewai/src/crewai/events/listeners/tracing/trace_batch_manager.py:327` |

## Answers to Dimension Questions

**1. Are model responses cached?**
No application-level response cache exists. Searching `response_cache|llm_cache|cache_response|@cached|lru_cache` across `lib/crewai/src` returned only infrastructure caches (file store, MCP schemas, agent cards, JWKS, i18n prompt files) — nothing keyed on completion inputs. What CrewAI does instead is *provider-side* prompt-prefix caching: executors mark stable prefixes (`lib/crewai/src/crewai/agents/crew_agent_executor.py:189-199`) that the Anthropic adapter turns into explicit `cache_control` breakpoints, while OpenAI/Gemini rely on implicit provider caching after marker stripping (`lib/crewai/src/crewai/llms/cache.py:4-6`). Repeated *identical completions* therefore always pay full generation cost; only repeated *prompt prefixes* are discounted. No evidence found of any replay/serving of stored completions.

**2. Are embeddings reused?**
Not explicitly. There is no embedding memoization layer: every `EncodingFlow` run computes fresh vectors via `embed_texts` (`lib/crewai/src/crewai/memory/encoding_flow.py:113-119`), and recall re-embeds queries each time (`lib/crewai/src/crewai/memory/recall_flow.py:230-242`). Indirect reuse exists only through persistence: knowledge storage keeps embeddings inside ChromaDB collections (`lib/crewai/src/crewai/knowledge/storage/knowledge_storage.py:45-49`), so previously stored chunks are not re-embedded at query time, and updated records carry forward their old embedding when re-embedding fails or is skipped (`lib/crewai/src/crewai/memory/encoding_flow.py:468-470`). Embedding providers are thin wrappers over ChromaDB embedding functions with no cache hooks (`lib/crewai/src/crewai/rag/embeddings/providers/openai/openai_provider.py:24-27`).

**3. Is retrieval cached?**
No. Every recall executes fresh vector searches against the configured backend (e.g., concurrent scope searches in `lib/crewai/src/crewai/memory/recall_flow.py:147-178`); there is no query→result cache anywhere under `memory/` or `rag/` (a targeted search for `cach` in `rag/` returned zero matches). Cost avoidance is achieved by *skipping work*, not caching results: short queries bypass the LLM query-analysis step entirely (`recall_flow.py:194-204`). The nearest analogues are artifact caches — downloaded skills on disk (`lib/crewai/src/crewai/skills/cache.py:35-66`) and MCP tool schemas with a 300-second TTL (`lib/crewai/src/crewai/mcp/client.py:51-52`) — which avoid network refetches, not retrieval computations.

**4. Are model calls batched efficiently?**
Chat-model calls are never batched — each agent step sends one message list, and memory-consolidation LLM calls are parallelized (up to 10 threads) rather than batched (`lib/crewai/src/crewai/memory/encoding_flow.py:223-261`); no provider adapter references a batch/inference API. Embedding calls, by contrast, are efficiently batched by design: `embed_texts` issues a single API call per pipeline stage (`lib/crewai/src/crewai/memory/types.py:312-339`), used both for initial item embedding and for update re-embedding plus bulk insert (`encoding_flow.py:414-424`, `476-481`), with recall sub-queries batched too (`recall_flow.py:189-234`). Telemetry is separately batched into `TraceBatch` payloads per execution session (`lib/crewai/src/crewai/events/listeners/tracing/trace_batch_manager.py:40-56`).

## Architectural Decisions

1. **Provider-agnostic cache-marker protocol.** Application code marks intent (`mark_cache_breakpoint`), adapters translate or strip (`lib/crewai/src/crewai/llms/cache.py:1-17` docstring; Anthropic translation at `completion.py:898-934`). This decouples prompt construction from provider cache semantics and degrades gracefully to no-op on providers without explicit controls.
2. **Correctness-first default: tool caching is opt-in.** EPD-180 flipped the default off because replaying stale tool output was "silently wrong for live-data tools, and silently dropped actions for stateful tools" (`lib/crewai/tests/test_tool_cache_default.py:1-15`). Consent is recorded at construction and never inherited by `copy()` (`lib/crewai/src/crewai/agent/core.py:421-430`), so runtime crew wiring cannot silently turn copies into cachers.
3. **Failures are never cached.** A declared failure would make a transient error permanent for the rest of the run, so `add()` refuses them (`cache_handler.py:23-41`), with a dedicated regression test (`lib/crewai/tests/tools/test_tool_failure.py:918-930`).
4. **The crew is the cache domain.** One `_cache_handler` per `Crew` (`lib/crewai/src/crewai/crew.py:213`) is distributed to all agents and the hierarchical manager at kickoff (`crew.py:758-763`, `1546-1548`), making dedup effective across agents/tasks within a run while keeping process isolation between crews.
5. **Batch-native memory pipeline.** The save path is designed around batch primitives end-to-end — one embed call, intra-batch dedup before any expensive work, concurrent searches/calls, batch re-embed, single bulk write (`encoding_flow.py:1-9`) — minimizing per-item API calls.
6. **Write gating delegated to tools.** Each tool can veto caching of its own results via `cache_function(args, result)` (`base_tool.py:176-177`), acknowledging that only the tool author knows whether outputs are safe to replay.

## Notable Patterns

- **Content-matching over positional indexing for cache stamping:** because Anthropic's formatter rewrites tool results into user messages, the adapter records the *content* of marked messages pre-conversion and re-finds them afterward, ensuring the stable initial task prompt — not the volatile trailing tool-result carrier — gets the breakpoint (`completion.py:759-796`, `898-921`).
- **Non-mutating strip to survive iteration loops:** `BaseLLM._format_messages` copies messages minus the marker so executor buffers retain flags across many LLM calls; a test pins this exact hazard (`base_llm.py:831-836`; `test_prompt_cache.py:30-50`).
- **Prompt-layout discipline for cacheability:** volatile data (current date) renders at the prompt tail so everything ahead stays a stable anchor; metadata-only skills keep the catalog block byte-stable (`prompts.py:143-163`, `165-190`).
- **Corrupted-entry-as-miss:** the skill cache treats unreadable/malformed metadata as a cache miss and re-downloads rather than raising (`skills/cache.py:68-82`).
- **Observable cache hits:** `from_cache` propagates into tool-usage events (`tool_usage_events.py:68`) and even into the textual observation ("...executed with result (from cache)") shown to the model (`agent_utils.py:1855`).
- **First-action-wins conflict avoidance:** consolidation plans dedup deletes/updates per record id to prevent LanceDB commit conflicts (`encoding_flow.py:383-412`).

## Tradeoffs

- **Exact-string cache keys.** Keys are `f"{tool}-{input}"` with `json.dumps(calling.arguments)` (`tools_handler.py:43-44`, `cache_handler.py:44`); semantically identical arguments with different key insertion order produce false misses, and no normalization/TTL exists.
- **Unbounded, process-local cache.** `CacheHandler._cache` is a plain dict with no eviction, size cap, or persistence (`cache_handler.py:20`); long-running crews with caching enabled grow memory monotonically, and nothing survives process restarts.
- **Opt-in default trades savings for safety.** Users who don't set `Crew(cache=True)` pay full tool-execution cost for duplicate calls; the code comments accept this explicitly (`crew.py:232-237`).
- **Miss-vs-none ambiguity.** `read()` returns `Any | None` and callers treat `None` as a miss (`tool_usage.py:309-312`), so a tool legitimately returning `None` re-executes every time.
- **Stamping coverage is uneven.** Only `CrewAgentExecutor` and experimental `AgentExecutor` mark breakpoints; the `LiteAgent` path contains no `mark_cache_breakpoint` usage (searched all of `lib/`), leaving lite-agent traffic on implicit provider caching only.
- **Dedup is O(n²) pairwise cosine within a batch** (`encoding_flow.py:130-140`) — fine for small batches, a scaling limit for very large `remember_many` calls.

## Failure Modes / Edge Cases

- **Stale-result replay within a run:** once opted in, an identical tool call returns the first result forever, even if the world changed; mitigations are per-tool `cache_function` vetoes and the docstring warning (`crew.py:232-237`).
- **Successes can rot, failures can't be frozen:** a transient-success (e.g., partial output) is cached permanently, while a failed call correctly re-executes — asymmetric by design (`cache_handler.py:26-29`, `tests/tools/test_tool_failure.py:915-930`).
- **Anthropic breakpoint budget:** the adapter notes a maximum of 4 cache breakpoints per request (`completion.py:900-901`); today's usage (system + up to a few marked user messages) stays under it, but additional marking call sites could exceed the cap silently.
- **Marker loss disables caching invisibly:** if any layer mutated the caller's message list, every post-first iteration would lose breakpoints; guarded only by regression tests (`test_prompt_cache.py:36-50`).
- **Silent length mismatch in batch embedding:** `zip(items, embeddings, strict=False)` leaves items unembedded if the embedder returns fewer vectors (`encoding_flow.py:118`); downstream checks treat empty embeddings as inactive rather than erroring.
- **Docs drift misleads users:** `docs/edge/en/concepts/crews.mdx:25` states cache "Defaults to `True`", contradicting `crew.py:229-230` (`default=False`) — users may assume dedup behavior that isn't active.
- **Background saves during shutdown:** background batch encodes detect executor shutdown and abandon the save rather than crash (`lib/crewai/src/crewai/memory/unified_memory.py:641-650`) — durability edge case acknowledged in-code.

## Future Considerations

1. Bound the tool cache: add max-size/TTL options and optional persistence to `CacheHandler`, and canonicalize JSON keys (sorted keys) to remove argument-order false misses.
2. Extend `mark_cache_breakpoint` stamping to the LiteAgent execution path so all agent loops benefit from provider prefix caching.
3. Add an embedding memoization layer keyed by (provider, model, text hash) shared between `EncodingFlow` and `RecallFlow`.
4. Consider a response/replay cache scoped to evaluation or testing modes, where determinism outweighs freshness.
5. Resolve the documentation/code drift on the `cache` default (`docs/edge/en/concepts/crews.mdx:25` vs `crew.py:229-230`).
6. Replace pairwise-cosine dedup with a vectorized (numpy/chromadb) similarity computation if batch sizes grow.
7. Surface prompt-cache effectiveness (hit ratio from `cached_prompt_tokens` vs total prompt tokens) as a first-class metric alongside existing usage aggregation (`base_llm.py:969-971`).

## Questions / Gaps

- **Model-response caching:** No evidence found. Searched `src` for `response_cache`, `llm_cache`, `cache_response`, `@cached`, `lru_cache`, and litellm `Cache(` integration patterns; all hits were infrastructural (files, MCP schemas, agent cards, i18n), none completion-keyed.
- **Embedding cache:** No evidence found in `rag/embeddings/**` (zero matches for `cach`) or in the memory flows; providers delegate directly to ChromaDB embedding functions.
- **Retrieval cache:** No evidence found under `memory/` or `rag/`; every recall performs live storage searches.
- **LLM request batching:** No evidence found of any batch-completions API usage in any provider adapter under `lib/crewai/src/crewai/llms/providers/`; concurrency exists (ThreadPoolExecutor in memory flows) but not request batching.
- Whether the deprecated `CrewAgentExecutor` will keep parity with the experimental executor's cache features during migration is unstated beyond its deprecation warning (`lib/crewai/src/crewai/agents/crew_agent_executor.py:145-151`).

---

Generated by dimension 20.02 (Caching, Batching, and Reuse) against `crewai`.

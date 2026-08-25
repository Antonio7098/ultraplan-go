# Source Analysis: pydantic-ai

## Dimension 20.02 — Caching, Batching, and Reuse

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (pydantic_ai_slim package; provider adapters over httpx/SDK clients; VCR-based test suite) |
| Analyzed | 2026-08-25 |

## Summary

Pydantic AI's caching strategy is **provider-side prompt caching as a first-class, explicitly engineered feature**, not client-side response memoization. There is no client-side cache of model responses in the core library: repeated identical requests pay full provider cost each time, by design — cost avoidance is delegated to provider prompt caches, which the framework makes easy to hit and hard to accidentally invalidate. The machinery spans:

1. A portable explicit cache-boundary marker (`CachePoint`, `pydantic_ai_slim/pydantic_ai/messages.py:761-788`) honored by Anthropic, Bedrock, OpenAI GPT-5.6, and OpenRouter.
2. Provider-namespaced settings for Anthropic automatic/per-block caching (`pydantic_ai_slim/pydantic_ai/models/anthropic.py:409-461`), OpenAI cache keys/options (`pydantic_ai_slim/pydantic_ai/models/openai.py:585-609,677-698`), Google `cached_content` (`pydantic_ai_slim/pydantic_ai/models/google.py:263`), and Mistral cache keys (`pydantic_ai_slim/pydantic_ai/models/mistral.py:175-176`).
3. Cache-aware request assembly: static/dynamic instruction sorting so the cache boundary lands after the static prefix (`pydantic_ai_slim/pydantic_ai/models/__init__.py:222-230`, `models/anthropic.py:2230-2254`).
4. Observability of cache economics via `cache_read_tokens`/`cache_write_tokens` and a computed cache-hit ratio (`pydantic_ai_slim/pydantic_ai/usage.py:96-119,204-216`).
5. An operational safeguard unusual for an agent library: a suite-wide test invariant that fails any recorded cassette whose consecutive requests move the provider-cache wire prefix without an explicit exemption marker (`tests/cassette_utils.py:54-76`).

Secondary caches exist for framework-internal work only: per-run OTel serialization caching (`pydantic_ai_slim/pydantic_ai/_instrumentation.py:91-120`), validated-output caching on stream objects (`pydantic_ai_slim/pydantic_ai/result.py:66-101`), and memoized schema builders (`pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:172-193`). Embeddings support batching but have zero caching/reuse. LLM calls are never batched; concurrency limiting is offered instead (`pydantic_ai_slim/pydantic_ai/models/concurrency.py:27-54`). Cross-run response reuse exists only in the Prefect durable-execution integration via a carefully projected task-result cache policy (`pydantic_ai_slim/pydantic_ai/durable_exec/prefect/_cache_policies.py:199-224`).

## Rating

**8 / 10**

Rationale against the rubric:

- **Prompt caching (the dimension's core)** reaches the 9-band bar locally: explicit public interfaces (`CachePoint`, typed settings fields), unified retention resolution across four providers with tests (`pydantic_ai_slim/pydantic_ai/models/__init__.py:446-465`, `tests/models/test_prompt_cache_retention.py:30-204`), documented invalidation semantics down to per-provider tool-filtering tables (`docs/tools-advanced.md:510-529`), usage-level observability, and a CI-enforced prefix-stability net over the entire recorded-cassette corpus (`tests/cassette_utils.py:35,54-76`). This is "proven under scale" in the sense that the framework continuously proves it does not silently bust provider caches.
- What keeps the overall score at 8 rather than 9–10: model responses are not cached client-side anywhere in core (repeated identical requests always re-execute); embeddings are batched but never cached or reused (`grep -rn cache sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/embeddings/` → 0 matches); there is no retrieval cache because there is no built-in retrieval subsystem; LLM calls are never batched. The cross-run response-reuse story is confined to one optional integration (Prefect).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Explicit prompt-cache marker | `CachePoint` dataclass with `ttl: Literal['5m','1h']`; unsupported models filter it out | `pydantic_ai_slim/pydantic_ai/messages.py:761-788` |
| CachePoint exported publicly | Re-exported from package root | `pydantic_ai_slim/pydantic_ai/__init__.py:65,243` |
| Anthropic settings: tools/instructions/messages/auto | `anthropic_cache_tool_definitions`, `anthropic_cache_instructions`, `anthropic_cache_messages`, `anthropic_cache` (server-applied moving breakpoint; counts 1 of 4 slots) | `pydantic_ai_slim/pydantic_ai/models/anthropic.py:409-416,424-431,433-442,444-461` |
| Anthropic cache-point budget enforcement | `_limit_cache_points` trims excess message breakpoints newest-first, raises `UserError` if system+tools exceed budget | `pydantic_ai_slim/pydantic_ai/models/anthropic.py:2262-2333` |
| Smart instruction caching | Static instructions sorted before dynamic; boundary placed after last static block | `pydantic_ai_slim/pydantic_ai/models/anthropic.py:2230-2254`; design note `pydantic_ai_slim/pydantic_ai/models/__init__.py:222-230`; docs `docs/models/anthropic.md:327-330` |
| Deferred-tool cache-point relocation | `deferred_cache_points` collected then re-attached after deferred blocks are appended | `pydantic_ai_slim/pydantic_ai/models/anthropic.py:1736-1750,1885-1902` |
| OpenAI explicit breakpoints | `_add_openai_prompt_cache_breakpoint` attaches `prompt_cache_breakpoint` to prior content; gated on profile flag `openai_supports_prompt_cache_breakpoints` | `pydantic_ai_slim/pydantic_ai/models/openai.py:599-609,1897-1898` |
| OpenAI cache settings | `openai_prompt_cache_key`, `openai_prompt_cache_retention ('in_memory'\|'24h')`, `OpenAIPromptCacheOptions {mode, ttl}` | `pydantic_ai_slim/pydantic_ai/models/openai.py:677-698,585-592` |
| Unified retention resolution | `Model.resolve_prompt_cache_retention` base + `_max_prompt_cache_retention` longest-wins helper; implemented by anthropic/openai/openrouter/bedrock | `pydantic_ai_slim/pydantic_ai/models/__init__.py:446-465`; `models/openai.py:914-919,984-986`; `models/anthropic.py:649-652` |
| Google cached content | `google_cached_content` strips `tools`/`system_instruction`/`tool_config` which the cached resource owns; warns if caller populated them | `pydantic_ai_slim/pydantic_ai/models/google.py:263-272,908-976` |
| Mistral cache key | `mistral_prompt_cache_key` mirroring `openai_prompt_cache_key` | `pydantic_ai_slim/pydantic_ai/models/mistral.py:175-176,312,363` |
| Tool-choice vs cache tradeoff table | API-level filtering preserves cache; client-side trimming breaks it; per-provider matrix | `docs/tools-advanced.md:510-529` |
| Tool-search cache preservation | Native search keeps discovered tools out of the cached prefix; local fallback invalidates from tools onward; search tool deliberately retained across discovery steps to keep prefix stable | `docs/tools-advanced.md:982-1010`; `pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:366-371` |
| Cache observability | `RequestUsage.cache_write_tokens/cache_read_tokens`, audio-cache subfield, OTel attributes `gen_ai.usage.cache_creation.input_tokens`/`..._read...`, `cache_hit_ratio()` property | `pydantic_ai_slim/pydantic_ai/usage.py:96-119,204-216,227-234` |
| OpenAI cache-write mapping incl. streaming | Nested `cache_write_tokens` extracted into usage (with TODO pending genai-prices); stream-mapping tests | `pydantic_ai_slim/pydantic_ai/models/openai.py:5028-5035`; tests at `tests/models/test_openai_prompt_cache.py:535,566,595` |
| Suite-wide prefix-stability invariant | `check_cache_prefix_stability` fails cassettes whose consecutive requests move the wire prefix unless `@pytest.mark.moves_cache_prefix(reason=...)` | `tests/cassette_utils.py:54-76`; enforcement fixture `tests/conftest.py` (`fail_cache_prefix_violations`) exercised in `tests/test_cache_prefix_invariant.py:64-100` |
| Prefix model per provider shape | `canonical_prefix_blocks` flattens anthropic/openai-chat/openai-responses/google bodies into cache-ordered blocks (`_CACHE_ORDER = tools→system→messages`); deferred Anthropic tools excluded from key (measured behavior) | `tests/cassette_utils.py:33-35,79-130` |
| Prompt-cache test coverage | 27 tests incl. history-prefix-stability, adjacency collapse, unsupported-model filtering, first-content error, e2e VCR replay asserting cache reads | `tests/models/test_openai_prompt_cache.py:90-808` (e.g. `:170,:194,:261,:733,:761,:785`) |
| Retention resolution tests | Longest-wins bias tests for openai/anthropic/bedrock/openrouter | `tests/models/test_prompt_cache_retention.py:30-204` |
| Per-provider cache tests | Bedrock, Google, OpenRouter cache test modules | `tests/models/bedrock/test_cache.py`; `tests/models/google/test_cache.py`; `tests/models/openrouter/test_cache.py` |
| OTel serialization cache | `CachedMessageJson`/`MessageJsonCache`: per-run dict keyed by `id(message)`, identity-pinned entry holds the message alive, evicted each request; makes span attribute O(new messages) not O(history); staleness detected by re-serialization compare | `pydantic_ai_slim/pydantic_ai/_instrumentation.py:91-120,256-274`; `models/instrumented.py:226-250`; run-scoped field `capabilities/instrumentation.py:101,224-225,294` |
| Stream output reuse | `_cached_output` memoizes validated output on the stream object; deepcopied on every yield to protect against caller mutation | `pydantic_ai_slim/pydantic_ai/result.py:66,74-101,142-146,231-242` |
| Schema-builder memoization | `@cache` on `_build_search_args_schema`; module-level prebuilt schema/validator constants | `pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:167-193` |
| Misc `@cache`/`cached_property` hot-path reuse | `get_user_agent()`, usage serializer lookup, `ModelRequestParameters.tool_defs` and friends | `pydantic_ai_slim/pydantic_ai/models/__init__.py:112,1778-1783,253-278`; `usage.py:34`; `tools.py:717` |
| Embedding batching (Bedrock) | `supports_batch` handler property; single-request `_embed_batch` for Cohere-style models vs `_embed_concurrent` with `anyio.Semaphore(max_concurrency)` (default 5) for Titan-style models | `pydantic_ai_slim/pydantic_ai/embeddings/bedrock.py:244-245,580-585,587-606,608-643` |
| Embedding batch size (local) | `sentence_transformers_batch_size` setting forwarded to `encode()` | `pydantic_ai_slim/pydantic_ai/embeddings/sentence_transformers.py:47-50,132-147` |
| No embedding cache | `grep -rn "cache" .../embeddings/` returns 0 matches across all 13 embedding modules | `pydantic_ai_slim/pydantic_ai/embeddings/*` |
| No LLM call batching | Only concurrency limiting wrappers exist; no batch request builder for chat models | `pydantic_ai_slim/pydantic_ai/models/concurrency.py:27-111,114-142` |
| Cross-run result reuse (Prefect) | `DEFAULT_PYDANTIC_AI_CACHE_POLICY = PrefectAgentInputs() + TASK_SOURCE + RUN_ID` default on all durable tasks; cache key strips per-run fields (`timestamp`,`run_id`,`conversation_id`), normalizes framework tool-call IDs, value-addresses tools by `(toolset.id, tool_def)` | `pydantic_ai_slim/pydantic_ai/durable_exec/prefect/_cache_policies.py:132-133,152-160,171-196,199-224`; applied in `durable_exec/prefect/_types.py:42-48` |
| RunContext projection exhaustiveness guard | Hand-authored hashable projection of `RunContext`; dedicated test fails when a new field is uncategorized | `pydantic_ai_slim/pydantic_ai/durable_exec/prefect/_cache_policies.py:62-129` |
| Docs: OpenAI prompt caching guide | Breakpoint semantics, 1024-token minimum, TTL partitioning via cache keys, billing notes | `docs/models/openai.md:125-148` |
| Docs: Anthropic prompt caching guide | Automatic caching, Bedrock/Vertex fallback, speed-switch invalidation warning | `docs/models/anthropic.md:222-255,544-545` |
| Docs: instruction placement & cache | Mid-conversation instructions preserve reusable prefix; repaired history is deterministic so reuse doesn't invalidate caches | `docs/message-history.md:231,258` |

## Answers to Dimension Questions

**1. Are model responses cached?**
No, not in core. There is no memoization layer between `Agent.run` and the provider adapter; identical requests re-execute. The deliberate substitute is provider-side prompt caching (evidence throughout above). The one true response cache is the durable-execution integration: Prefect tasks persist results under `DEFAULT_PYDANTIC_AI_CACHE_POLICY` (`pydantic_ai_slim/pydantic_ai/durable_exec/prefect/_cache_policies.py:199-224`), so a flow retry replays a prior model-response task's persisted result instead of re-invoking the provider. This is keyed by projected inputs, not by prompt content hash, and is scoped to Prefect users.

**2. Are embeddings reused?**
No. The embeddings subsystem (`pydantic_ai_slim/pydantic_ai/embeddings/base.py:57-72`) defines a stateless `embed()` protocol with no cache hooks; zero occurrences of caching across all embedding modules. Identical strings are re-embedded every call. Batching exists (see Q4), but no fingerprint/dedup layer.

**3. Is retrieval cached?**
There is no built-in retrieval/RAG pipeline to cache. The nearest analogues: (a) tool discovery results are *derived from history* via `parse_discovered_tools` (`pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:196-209`) rather than re-executed searches; (b) the search-tool JSON schema is memoized per-description (`_tool_search.py:172-193`). Users doing RAG must bring their own retrieval cache.

**4. Are model calls batched efficiently?**
LLM calls are never batched — the agent loop issues one request per step, sequentially through the graph. Efficiency levers instead are: concurrency limiting with shareable limiter pools (`models/concurrency.py:44-53`), streaming debouncing to coalesce chunks (`result.py:118,135-137`), and provider prompt caching. Embedding inputs *are* batched where the provider supports it (single-request batch for Cohere-on-Bedrock, `embeddings/bedrock.py:580-582`; semaphore-bounded parallel fan-out otherwise `:608-643`; configurable local batch size `sentence_transformers.py:132-147`).

**Rating question — can repeated identical requests avoid paying full cost?**
Within a conversation turn sequence: yes, via prompt caching when enabled (explicit `CachePoint` or provider settings). Across independent identical requests: no — nothing in core detects repetition.

## Architectural Decisions

1. **Cache boundaries as message content, not configuration-only.** `CachePoint` is a part inside `UserPromptPart.content` (`messages.py:760-776`), so breakpoints travel with serialized history and survive persistence — unlike settings that would need re-application per request. Unsupported adapters filter markers out (`vercel_ai/_adapter.py:1111-1112` shows the same skip pattern in UI conversion).
2. **Provider namespacing over a unified cache setting.** All cache knobs are `{provider}_`-prefixed settings fields (`anthropic.py:409-461`, `openai.py:677-698`); the base class explicitly notes "a future unified cache setting is not yet an input" (`models/__init__.py:450`). Only the *retention* is unified, as a read-only resolution API used by durable engines/tests.
3. **Prefix-stability as a repo-wide invariant, not per-feature tests.** The AGENTS.md testing doctrine states cassettes "double as a suite-wide prompt-cache prefix regression net" (`tests/AGENTS.md`, testing philosophy section), enforced by `check_cache_prefix_stability` (`tests/cassette_utils.py:54-76`) with an opt-out marker requiring a written reason. This converts cache correctness from a review concern into a mechanical gate.
4. **Static-before-dynamic instruction ordering as a caching enabler.** Instruction classification (`models/__init__.py:222-230`) exists specifically so granular-caching providers can place the boundary after the stable prefix (`anthropic.py:2230-2254`); documented at `docs/agent.md:1364`.
5. **Durable-execution caching keyed by projected semantics, not raw payloads.** The Prefect policy hand-authors what enters the hash (deps, model id, messages minus per-run fields, sorted capability/tool sets) and documents why each exclusion is safe (`_cache_policies.py:62-133`), plus a regression test forcing future fields to be consciously categorized.

## Notable Patterns

- **Measured-behavior encoding:** the prefix checker excludes Anthropic deferred tools from its cache-key model with the comment "(measured)" (`tests/cassette_utils.py:100-107`) — provider cache behavior was empirically probed and codified into the invariant.
- **Budget-aware breakpoint management:** `_limit_cache_points` prioritizes system > tools > newest-messages when trimming to Anthropic's 4-slot limit, raising `UserError` only for unfixable configurations (`anthropic.py:2277-2293`).
- **Identity-pinned cache entries:** OTel's `CachedMessageJson` stores the message object itself so an `id(message)` collision with a garbage-collected replacement cannot serve a stale fragment (`_instrumentation.py:94-98`), with eviction keeping the cache bounded by current history (`:110-113`) and a staleness detector for in-place mutation (`:256-274`, surfaced as `MessageHistoryMutatedWarning`).
- **Deepcopy-on-read for cached outputs:** `_cached_output` is deepcopied on every yield/access so callers cannot poison the memoized validated output (`result.py:76-77,231-242`).
- **Anti-pattern guidance in rules files:** `pydantic_ai_slim/pydantic_ai/models/AGENTS.md` (API Design section, token-counting rule) mandates that per-request injections anchor to positions that do not move with history length — "anchoring to a moving position shifts the cacheable prefix every turn … a cost/latency regression that surfaces no error."
- **Tool retention for prefix stability:** the local `search_tools` function is kept registered even after everything is discovered, because removing it mid-run would invalidate the request prefix (`toolsets/_tool_search.py:366-371`).

## Tradeoffs

- **Provider-side caching vs portability:** cost savings depend entirely on provider features; on providers without prefix caching (or below OpenAI's 1024-token minimum, `docs/models/openai.md:146`) identical requests pay fully. The framework mitigates but cannot remove this dependency.
- **Explicitness vs friction:** users must opt into nearly all caching (`anthropic_cache=False` by default; explicit `CachePoint` insertion). Safer defaults, but many multi-turn users likely run uncached without realizing it.
- **Prefix-invariant strictness vs legitimate dynamism:** any test whose behavior legitimately moves the prefix (compaction, dynamic tool disclosure) must carry `@pytest.mark.moves_cache_prefix(reason=...)` (`tests/cassette_utils.py:56-63`); the corpus-wide check adds maintenance overhead on every request-shape change.
- **Prefect cache-key projection vs completeness:** excluding live resources with sentinels means two runs differing only in an unhashable dep can collide on the same key (`_cache_policies.py:36-43` acknowledges the sentinel approach trades precision for robustness).
- **Batching asymmetry:** embeddings get real batching; chat calls get none — fine for interactive agents, suboptimal for offline bulk evaluation workloads (pydantic_evals inherits this).

## Failure Modes / Edge Cases

Handled explicitly:
- Exceeding Anthropic's cache-point budget → newest message breakpoints silently dropped; unfixable overflow raises `UserError` (`anthropic.py:2310-2333`).
- `CachePoint` as first content → `UserError` with actionable message (`openai.py:602-606`; tested `test_openai_prompt_cache.py:251,475`).
- Adjacent cache points collapsed (`test_openai_prompt_cache.py:170`); unsupported content types filtered (`:261,485`); TTL ignored where providers don't accept it (`messages.py:786-787`).
- In-place mutation of cached history → staleness detection and warning path rather than corrupt telemetry (`_instrumentation.py:256-274`).
- Speed-tier switch silently invalidating Anthropic cache → documented warning (`docs/models/anthropic.md:544-545`).

Residual risks:
- Client-side tool filtering breaks cache invisibly on affected providers; the docs table (`docs/tools-advanced.md:515-529`) is advisory only — no runtime signal tells a user their `tool_choice` restriction just busted the prefix (only post-hoc `cache_read_tokens == 0` in usage hints at it, `usage.py:204-216`).
- Prefect sentinel projection can theoretically merge distinct runs (above).
- No client-side dedup means accidental duplicate concurrent requests (e.g., retry storms) pay repeatedly; `ConcurrencyLimitedModel` queues but does not coalesce (`concurrency.py:85-86`).

## Future Considerations

- The base class already anticipates "a future unified cache setting" spanning providers (`models/__init__.py:450-452`) — a single `prompt_cache=…` knob would remove per-provider settings burden.
- An opt-in client-side embedding cache (hash-of-text → vector, LRU/TTL) would complement the new embeddings subsystem at near-zero API cost.
- Surfacing a warning or metric when `cache_read_tokens == 0` on a multi-turn run with caching enabled would convert silent prefix-busts into diagnosable events.
- Generalizing the Prefect result-cache concept (value-addressed, projection-based) behind a provider-agnostic interface could give non-Prefect users cross-run reuse for expensive tool/model steps.

## Questions / Gaps

- Where is `resolve_prompt_cache_retention` consumed in production code paths? It is defined on the base `Model` (`models/__init__.py:446-455`), implemented by four adapters, and tested (`tests/models/test_prompt_cache_retention.py`), but no call site outside tests was found (`grep -rn "\.resolve_prompt_cache_retention(" --include="*.py"` → tests only). Its docstring and the `wrapper.py:130-136` comment imply durable-execution timeout computation; the consumer may live in user code or a surface this study did not locate within the source tree.
- No evidence found of any HTTP-transport-layer response caching (e.g., conditional requests): `create_async_http_client` builds a plain client (`models/__init__.py:1653`); searched docs/changelog for response-caching features with no hits beyond the renamed `cached_async_http_client` utility (a client-lifetime helper, not a response cache — `docs/migration.md:39`).
- Whether the prefix-stability fixture runs in all CI lanes or only some could not be confirmed from the source alone (`fail_cache_prefix_violations` wiring lives in `tests/conftest.py`; CI workflow files were outside the selected-source boundary for this dimension).

---

Generated by `dimensions/20.02-caching-batching-and-reuse` against `pydantic-ai`.

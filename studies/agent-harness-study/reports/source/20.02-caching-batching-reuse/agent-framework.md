# Source Analysis: agent-framework

## Caching, Batching, and Reuse

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python + .NET (C#) monorepo (Microsoft Agent Framework); Go implementation is external (`go/README.md:1-3`) |
| Analyzed | 2026-08-25 |

## Summary

Agent Framework takes a deliberate **delegate-to-provider** posture on prompt caching rather than building its own model-response cache. There is no application-level cache of chat responses, no embedding memoization, and no batching of chat completions anywhere in either stack; a repo-wide search for `CachingChatClient` / `ResponseCache` / `response_cache` across `dotnet/src` and `python/packages` returns zero hits. What the framework does instead is threefold:

1. **Pass-through surface for provider-managed prompt caching.** The Python OpenAI Responses client exposes `prompt_cache_key`, `prompt_cache_retention`, and `prompt_cache_options` (explicit GPT-5.6+ cache breakpoints with an SDK-version guard), and the Anthropic client supports structured system blocks carrying prompt-cache `cache_control`. The .NET hosting models expose OpenAI's `prompt_cache_key` on both the Responses and ChatCompletions wire types.
2. **First-class observability of cache economics.** Cache token counts are normalized into core usage types (`cache_creation_input_token_count` / `cache_read_input_token_count`) and OTel GenAI attributes from every provider (OpenAI, Anthropic, Bedrock, Claude SDK, Mistral, Gemini, GitHub Copilot), and are summed correctly through usage aggregation on both stacks.
3. **A well-engineered caching layer where the framework itself pays a repeated cost: skills discovery.** Both Python (`CachingSkillsSource`) and .NET (`CachingAgentSkillsSource`) implement single-flight fetches per isolation key, optional TTL-based refresh using a monotonic clock, fail-open semantics (a failed or cancelled fetch is never cached), and automatic wrapping only for built-in context-independent leaf sources — all backed by extensive test suites on both stacks.

Batching exists only at the edges: embedding inputs can be sent as one batched API call, tool calls within a turn execute concurrently via `asyncio.gather`, and telemetry export uses OTel batch processors. Chat model calls themselves are never batched or deduplicated.

**Answer to the dimension's driving question** — *Can repeated identical requests avoid paying full cost each time?* — Only partially: identical prompts can hit provider-side prompt caches (via pass-through keys/breakpoints), and repeated skills discovery hits the framework's own caches, but an identical chat request always re-executes end-to-end; there is no response memoization or embedding reuse layer.

## Rating

**5 / 10**

Rationale against the rubric:

- Where caching exists it is high quality — explicit interfaces, documented concurrency/failure semantics, isolation keys, monotonic-clock TTLs, and dense test coverage on both stacks (7–8 territory for the skills-cache subsystem alone).
- But the dimension's core surfaces are absent by design: model responses are never cached (Q1: no), embeddings are never reused (Q2: no), retrieval results are not cached beyond storage providers doubling as history stores (Q3: no), and model calls are not batched — only embeddings and telemetry exports are batched (Q4: partially).
- The pass-through prompt-caching options are thin wrappers over vendor features with no framework-level guidance, defaults, or automatic prefix shaping (e.g., nothing stabilizes system-prompt ordering to improve cache hit rates). This places the overall story in "present but inconsistent / partial coverage" (middle band).

## Evidence Collected

Every entry uses workspace-relative paths into the selected source.

| Area | Evidence | File:Line |
|------|----------|-----------|
| No model-response cache | Searches for `CachingChatClient`, `ResponseCache`, `response_cache`, `cached response` across `dotnet/src` and `python/packages` return no implementation hits | studies/agent-harness-study/sources/agent-framework/dotnet/src; .../python/packages (search boundary noted below) |
| Provider-managed prompt-cache keys (Python) | `OpenAIChatOptions.prompt_cache_key` ("Used by OpenAI to cache responses for similar requests"), `prompt_cache_retention: Literal["24h"]`, `prompt_cache_options` with `mode: implicit/explicit` breakpoints; option flows into request via generic options passthrough with SDK>=2.45 guard | studies/agent-harness-study/sources/agent-framework/python/packages/openai/agent_framework_openai/_chat_client.py:227-239, :1404-1407 |
| PromptCacheOptions fallback typing | Fallback TypedDict (`mode`, `ttl: "30m"`) when installed openai predates the feature; runtime guard raises `ChatClientInvalidRequestException` | studies/agent-harness-study/sources/agent-framework/python/packages/openai/agent_framework_openai/_chat_client.py:116-132 |
| Anthropic prompt-cache blocks | Docs state structured `instructions` system blocks exist "when you need prompt-cache ``cache_control``"; tests assert `cache_control: {"type": "ephemeral", "ttl": "1h"}` blocks round-trip | studies/agent-harness-study/sources/agent-framework/python/packages/anthropic/agent_framework_anthropic/_chat_client.py:132-134; python/packages/anthropic/tests/test_anthropic_client.py:1061, :1092, :1143 |
| OpenAI prompt_cache_key (.NET) | `[JsonPropertyName("prompt_cache_key")]` on CreateResponse/CreateChatCompletion; deprecated `user` field comment says "Use prompt_cache_key instead to maintain caching optimizations" | studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Hosting.OpenAI/Responses/Models/CreateResponse.cs:145, :176-180; .../ChatCompletions/Models/CreateChatCompletion.cs:134 |
| Core usage cache-token fields | `UsageDetails.cache_creation_input_token_count` / `cache_read_input_token_count` defined as core usage counters | studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_types.py:416-426 |
| OTel cache-token attributes | `CACHE_CREATION_INPUT_TOKENS = "gen_ai.usage.cache_creation.input_tokens"`, `CACHE_READ_INPUT_TOKENS`; alias mapping normalizes anthropic/openai/mistral legacy key names onto them | studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/observability.py:251-252, :384-400 |
| Per-provider cache-token mapping (Python) | OpenAI Responses: `openai.cached_input_tokens` → `cache_read_input_token_count`; ChatCompletions: `prompt/cached_tokens`; Mistral: `cached_tokens` → both keys; Bedrock: `cacheReadInputTokens`; Claude SDK: `cache_creation/read_input_tokens`; Gemini: cached count; GitHub Copilot: `cache_read_tokens`/`cache_write_tokens` | studies/agent-harness-study/sources/agent-framework/python/packages/openai/agent_framework_openai/_chat_client.py:3409-3413; .../_chat_completion_client.py:951-954; python/packages/mistral/.../_chat_client.py:858-861; python/packages/bedrock/.../_chat_client.py:697-700; python/packages/claude/.../_agent.py:971-972; python/packages/gemini/.../_chat_client.py:1258; python/packages/github_copilot/.../_agent.py:829-830 |
| DevUI surfaces cache tokens | Mapper builds `cached_tokens=` / `cache_write_tokens=` from normalized usage details | studies/agent-harness-study/sources/agent-framework/python/packages/devui/agent_framework_devui/_mapper.py:96-97 |
| Usage aggregation sums cached tokens (.NET) | `UsageAggregator.Combine` sums `CachedInputTokenCount` plus per-key additional counts "such as cached, reasoning, or cost counters"; Copilot maps `CacheReadTokens` → `CachedInputTokenCount` and writes `CacheWriteTokens` to additional counts | studies/agent-harness-study/sources/agent-framework/dotnet/src/Shared/Usage/UsageAggregator.cs:38, :52, :60-64; dotnet/src/Microsoft.Agents.AI.GitHub.Copilot/GitHubCopilotAgent.cs:474, :499-502 |
| Skills result cache (Python) | `CachingSkillsSource`: per-key dict caches + monotonic timestamps; `_get_fresh_cached` expiry check uses `time.monotonic()`; per-key asyncio locks created under a guard; double-checked locking around inner fetch; failed/cancelled fetch leaves cache untouched | studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_skills.py:3810-3984 (expiry :3935-3938, locks :3941-3948, single-flight get_skills :3966-3984) |
| Skills auto-wrap policy (Python) | Built-in leaf sources wrapped as `DeduplicatingSkillsSource(CachingSkillsSource(leaf, refresh_interval=cache_refresh_interval))`; caller-supplied sources intentionally never auto-cached because shared-bucket caching would replay one context's skills for later contexts (cross-agent/tenant leak) | studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_skills.py:2140-2147, :2159, :2274-2275; dedup class :3703 |
| Skills result cache (.NET) | `CachingAgentSkillsSource` mirrors Python: `SemaphoreSlim(1,1)` gate per key serializes fetches, fast-path freshness check, refresh-interval staleness check, cancelled/failed fetches not cached; `CachingAgentSkillsSourceOptions.CacheIsolationKeySelector` + `RefreshInterval`; auto-wrapping in provider and builder | studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI/Skills/Decorators/CachingAgentSkillsSource.cs:32-148 (:56 isolation key, :67 gate, :77-82 no-cache-on-failure, :100-104 interval); .../CachingAgentSkillsSourceOptions.cs:26, :38; .../Skills/AgentSkillsProvider.cs:211, :243; .../AgentSkillsProviderBuilder.cs:235-243, :278 |
| Cache tests (both stacks) | Python tests: inner results cached, refresh-interval serving, custom source not auto-cached, refresh interval threading/defaults; .NET facts: calls-inner-once (sequential + concurrent), error not cached, isolation-key separation, empty-string key isolated, cancellation matrix (first/all/single caller cancel, no poisoning), zero/negative interval always refetches | studies/agent-harness-study/sources/agent-framework/python/packages/core/tests/core/test_skills.py:5567, :5843, :6171, :6249-6274; dotnet/tests/Microsoft.Agents.AI.UnitTests/AgentSkills/CachingAgentSkillsSourceTests.cs:16-370 (17 `[Fact]`s) |
| Embedding batching, no reuse (Python) | `get_embeddings(values)` sends all inputs in a single API call (`kwargs = {"input": list(values), ...}`) — input batching yes, but no memoization of vectors for repeated strings | studies/agent-harness-study/sources/agent-framework/python/packages/openai/agent_framework_openai/_embedding_client.py:254-290 (:280 batched input), :290-317 fresh call every time |
| Redis vectorization without local reuse | Query-time vectorization via redisvl `aembed_many` (ingest) and `aembed` (query) each invocation; no embedding cache layer | studies/agent-harness-study/sources/agent-framework/python/packages/redis/agent_framework_redis/_context_provider.py:331-337, :380-390 |
| Retrieval/history layers are stores, not caches | Redis history provider contains no cache logic; Valkey chat history provider (.NET) is persistence; workflow checkpoints use an internal in-memory `SessionCheckpointCache` keyed by `CheckpointInfo` | studies/agent-harness-study/sources/agent-framework/python/packages/redis/agent_framework_redis/_history_provider.py (no `cache` matches); dotnet/src/Microsoft.Agents.AI.Valkey/ValkeyChatHistoryProvider.cs; dotnet/src/Microsoft.Agents.AI.Workflows/Checkpointing/SessionCheckpointCache.cs:9-68 used by InMemoryCheckpointManager.cs:16-38 |
| Tool-call concurrency (not chat batching) | Executable function-call batch runs concurrently preserving per-call result groups: `asyncio.gather(*execution_tasks)`; `_FunctionExecutionBatch` groups results; batch classification happens before execution | studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_tools.py:1733, :1750-1883, :1864-1875, :1883-1925 |
| Telemetry export batching | `BatchSpanProcessor` / `BatchLogRecordProcessor` wrap exporters when OTel is enabled | studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/observability.py:1021, :1030, :1057, :1063 |
| Computed-artifact micro-caches (Python) | WeakKeyDictionary cache of serialized OTel tool JSON per tool object identity (with `_CACHE_MISS` sentinel and TypeError fallback for unhashable specs); FunctionTool lazily caches JSON schema (`_input_schema_cached`) and parameters (`_cached_parameters`); middleware pipeline instances cached and rebuilt only when composition changes; `lru_cache(maxsize=128)` on serialization-protocol structural type check | studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/observability.py:2485-2551; _tools.py:395-407, :806-825, :3053, :3166-3172; _middleware.py:1277-1291; _serialization.py:141-144 |
| User-space caching pattern documented | FunctionMiddleware docstring example demonstrates tool-result caching keyed by `function.name:arguments` — presented as a pattern users implement, not a shipped cache | studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_middleware.py:598-635 |

## Answers to Dimension Questions

**1. Are model responses cached?**
No. There is no response cache at any layer of either stack: chat clients (`python/packages/core/agent_framework/_clients.py`), agents (`_agents.py`), and .NET `ChatClientAgent` (`dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientAgent.cs`) all issue a live provider call per run, and no decorator resembling `CachingChatClient` exists in-repo (search returned zero implementation hits; Microsoft.Extensions.AI's abstract `CachingChatClient` is an external dependency that this repo never subclasses or wires). Cost avoidance for repeated prefixes is delegated entirely to provider-managed prompt caches via the pass-through options listed above.

**2. Are embeddings reused?**
No. `OpenAIEmbeddingClient.get_embeddings` re-invokes the API on every call (`python/packages/openai/agent_framework_openai/_embedding_client.py:290`), and the Redis context provider re-vectorizes queries on demand (`python/packages/redis/agent_framework_redis/_context_provider.py:380-381`). No content-addressed vector cache exists anywhere in the repo. The only "reuse" is structural: multiple values submitted together share one HTTP round-trip.

**3. Is retrieval cached?**
Not as such. Retrieval-adjacent components are storage/persistence layers (Redis history provider, Valkey chat history provider, Cosmos memory context provider) with no read-through caching. The closest thing to retrieval caching is the skills-discovery pipeline: filesystem/MCP skill enumeration is expensive and static, so built-in leaf sources are auto-wrapped in `DeduplicatingSkillsSource(CachingSkillsSource(...))` (`python/packages/core/agent_framework/_skills.py:2140-2141`; .NET equivalent `dotnet/src/Microsoft.Agents.AI/Skills/AgentSkillsProviderBuilder.cs:278`), with deliberate refusal to auto-cache caller-supplied sources to avoid cross-context leakage (`_skills.py:2143-2147`). Workflow checkpoints also maintain an in-memory session-scoped checkpoint cache (`SessionCheckpointCache.cs:9-68`).

**4. Are model calls batched efficiently?**
Chat calls: no batching mechanism exists — one request per turn, no queueing/coalescing of concurrent identical requests. Partial batching exists elsewhere: embedding requests accept value arrays in a single call (`_embedding_client.py:280`), tool calls within one turn run concurrently (`_tools.py:1864`), and telemetry export is batched via OTel processors (`observability.py:1057,1063`). Multi-agent fan-out concurrency exists at the orchestration level but is parallelism, not request batching.

## Architectural Decisions

- **Delegate cost reduction to providers, keep the framework neutral.** Rather than owning a cache with invalidation risk, the framework exposes provider-native knobs (`prompt_cache_key` / `prompt_cache_retention` / `prompt_cache_options` at `python/packages/openai/agent_framework_openai/_chat_client.py:227-239`; Anthropic structured system blocks at `python/packages/anthropic/agent_framework_anthropic/_chat_client.py:132-154`). This avoids stale-response hazards entirely but means hit-rate optimization is the application's job.
- **Normalize cache economics in the usage contract, not the execution path.** Cache token counts are part of core `UsageDetails` (`python/packages/core/agent_framework/_types.py:416-426`) with a dedicated OTel alias-mapping table reconciling seven vendors' naming schemes (`observability.py:384-400`), so cache effectiveness is observable everywhere even though caching itself is external.
- **Make caching a composable decorator, not baked-in behavior.** Skills caching is a `DelegatingSkillsSource` wrapper (`_skills.py:3810`), mirroring .NET's delegating decorator (`CachingAgentSkillsSource.cs:32`). Composition over inheritance lets users opt in/out per node.
- **Fail-open caching semantics.** Both implementations refuse to cache failures or cancellations (Python docstring contract at `_skills.py:3826-3828` and implementation at :3981-3984; .NET remarks at `CachingAgentSkillsSource.cs:27-30` and code at :77-82), so a transient MCP outage degrades to latency, not stale data.
- **Safety-gated auto-wrapping.** Only built-in context-independent leaf sources are auto-cached; context-aware user sources must opt in with an isolation-key selector to prevent tenant leakage (`_skills.py:2143-2147`, `CachingAgentSkillsSourceOptions.cs:22-23` warns against high-cardinality keys).

## Notable Patterns

- **Single-flight fetch under double-checked locking**: fast path checks the cache, acquire a per-key lock, re-check, then fetch — identical shape in asyncio locks (`_skills.py:3966-3984`) and `SemaphoreSlim` (`CachingAgentSkillsSource.cs:60-88`), showing cross-language port discipline.
- **Monotonic-clock TTLs**: Python measures freshness with `time.monotonic()` (`_skills.py:3936`) rather than wall time, immune to clock jumps.
- **Weak-identity micro-caches**: the OTel tool-JSON cache uses `WeakKeyDictionary` keyed by object identity so entries die with their tools, with a sentinel distinguishing "cached None" from "never computed" (`observability.py:2493-2551`).
- **Version-adaptive option surfaces**: the OpenAI client imports `PromptCacheOptions` from newer SDKs and installs a typed fallback + runtime guard on older ones (`_chat_client.py:116-132, :1404-1407`).
- **Pipeline instance reuse**: middleware pipelines are cached and invalidated by membership comparison (`_middleware.py:1284-1291`, `_tools.py:3166-3172`), avoiding per-call allocation without risking stale composition.

## Tradeoffs

- **No response/embedding cache means guaranteed correctness but full repeat cost.** Identical requests always pay full inference price unless the provider's own prompt cache engages; output tokens are never avoidable.
- **Prompt-caching support is shallow pass-through.** Nothing in the framework shapes prompts for better hit rates (stable prefix ordering, breakpoint placement automation); users must know vendor-specific options (`prompt_cache_options` even hard-requires `openai>=2.45.0`, raising otherwise — `_chat_client.py:1404-1407`).
- **Unbounded-by-default skills caches**: with no `refresh_interval`, cached skills live forever within the process (`_skills.py:3830-3833`); long-lived hosts see stale skill sets until restart unless TTL is configured. Mitigated by documented low-cardinality key guidance, but the cache has no size bound.
- **Embedding batching without dedup**: sending N values in one call amortizes transport but still re-embeds duplicate strings repeatedly across turns.
- **Cross-language parity burden**: every caching semantic is implemented twice (Python asyncio vs .NET SemaphoreSlim); divergence risk is real but currently well controlled (docstrings reference matching .NET PR ports, e.g., `_skills.py` AGENTS notes and refresh_interval port).

## Failure Modes / Edge Cases

- **Failed fetch never poisons the cache**: initial failure leaves the bucket empty (retry next call); refresh failure retains the previous list and keeps retrying (`_skills.py:3838-3840`; tested in `test_skills.py` and `CachingAgentSkillsSourceTests.cs:61-79`).
- **Cancellation isolation**: a cancelling caller neither caches a partial result nor breaks queued callers — first-caller-cancel, all-cancel, and no-poisoning scenarios are explicitly tested (`CachingAgentSkillsSourceTests.cs:198-325`).
- **Isolation-key edge cases**: `null` selector → shared bucket; empty-string key is deliberately distinct from shared bucket (`CachingAgentSkillsSourceTests.cs:152-197`).
- **Degenerate intervals**: zero/negative `refresh_interval` disables caching by making everything instantly stale (`_skills.py:3836-3838`; `CachingAgentSkillsSourceTests.cs:348-370`).
- **Unhashable/unweakreferenceable tools bypass the OTel cache gracefully** via `TypeError` suppression paths (`observability.py:2540-2550`) rather than failing telemetry.
- **Known gap**: no eviction/size cap on the skills caches; a pathological high-cardinality isolation key grows memory without bound — acknowledged in docs (`_skills.py:3846-3849`, `CachingAgentSkillsSourceOptions.cs:22-23`) but not enforced.

## Future Considerations

- An opt-in `CachingChatClient`-style decorator (TTL + key selector, reusing the proven single-flight pattern) would complete the dimension without changing default semantics.
- Content-hash embedding memoization in front of embedding clients would make RAG re-indexing and repeated-query workloads substantially cheaper; the `BaseEmbeddingClient` seam (`python/packages/core/agent_framework/_clients.py:855+`) is a natural insertion point.
- Automatic prompt-prefix stabilization (deterministic ordering of instructions/tools before serialization) would raise provider prompt-cache hit rates with zero API changes; today tool-definition serialization order follows insertion order.
- Bounded-size eviction (LRU) for `CachingSkillsSource` buckets to convert the documented cardinality guidance into an enforced invariant.
- Surfacing cache-hit ratios (creation vs read tokens) as first-class metrics in DevUI beyond the raw mapper passthrough (`devui/_mapper.py:96-97`).

## Questions / Gaps

- **No evidence found** for any retrieval-result caching in `TextSearchProvider` (`dotnet/src/Microsoft.Agents.AI/TextSearchProvider.cs` — no `cache` matches), mem0 context provider (`python/packages/mem0/agent_framework_mem0/_context_provider.py` — no `cache` matches), or the Cosmos memory provider; if caching exists, it lives inside the external mem0/Cosmos services, not this repo.
- **Go implementation out of scope of this source**: `go/README.md:3` points to `microsoft/agent-framework-go`; no caching surface could be inspected here (source-isolation rule observed).
- Whether .NET's `FunctionInvokingChatClient` performs parallel function execution could not be confirmed in-repo: the type comes from the external `Microsoft.Extensions.AI` package; only the Python loop's concurrency (`_tools.py:1864`) is directly evidenced.
- No benchmark or load-test evidence demonstrating cache effectiveness under scale was found in the repo; the 7–8-quality judgment for the skills cache rests on unit tests, not production telemetry.

---

Generated by `dimensions/20.02-caching-batching-and-reuse` against `agent-framework`.

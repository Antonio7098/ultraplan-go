# Source Analysis: openai-agents-sdk

## 20.02 Caching, Batching, and Reuse

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+ (asyncio, OpenAI Python SDK, pydantic, anyio) |
| Analyzed | 2026-08-25 |

## Summary

The SDK deliberately delegates model-response caching to the OpenAI platform instead of building a client-side response cache. Its own contribution to cost reduction is a well-engineered **prompt-cache-key subsystem**: the runner generates one stable `prompt_cache_key` per invocation, scoped by a grouping hierarchy (server conversation → SDK session → trace group → per-run UUID), injects it into every model request, persists it across turns, resumes, and streamed/non-streamed paths, and gates generation on models that actually support it (official api.openai.com endpoints only). Explicit prompt-cache controls (`prompt_cache_retention`, `prompt_cache_options`, content-part cache breakpoints) are surfaced as typed `ModelSettings` fields and forwarded by both Responses and Chat Completions adapters.

For retrieval-adjacent work, MCP tool listings can be cached per server (`cache_tools_list`) with an explicit invalidation API and deep-copy snapshot semantics that protect cached schemas from caller mutation; this is opt-in with no TTL. Cached-token consumption is observable via aggregated usage (`cached_tokens`, `cache_write_tokens`).

There is **no client-side model-response cache** (identical repeated requests pay full provider cost again apart from server-side prompt-prefix hits), **no embedding cache or reuse** (the SDK has no embedding surface at all), and **no batching of model inference calls** (one provider request per agent turn). The only true batch builder is for trace export (`BatchTraceProcessor`). Parallelism exists (function-tool fan-out, parallel MCP connects) but is concurrency, not request batching.

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards for what it implements: prompt-cache keys have a documented grouping contract, opt-out precedence, provider gating, resume persistence, and ~15 dedicated tests; the MCP tool-list cache is tested including mutation-corruption and pagination edge cases; trace export batching has queue/backpressure handling. It falls short of 8–9 because the tool-list cache has no TTL/expiry, there is no client-side response deduplication (repeated identical requests cannot avoid full cost), no embeddings/retrieval-result caching whatsoever, and prompt-cache effectiveness is entirely dependent on unverifiable server-side behavior — observable only through usage counters.

## Evidence Collected

Every entry includes a file path with line numbers relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Prompt cache key generator | `PromptCacheKeyResolver` returns one generated key per runner invocation, caches it in memory, and persists to `RunState`; skips when user supplied `prompt_cache_key` via `extra_args`/`extra_body` | `src/agents/run_internal/prompt_cache_key.py:17-66`, `src/agents/run_internal/prompt_cache_key.py:75-88` |
| Cache-key field name & injection | `PROMPT_CACHE_KEY_FIELD = "prompt_cache_key"`; `model_settings_with_prompt_cache_key` writes the key into `ModelSettings.extra_args` unless already present | `src/agents/run_internal/prompt_cache_key.py:13`, `src/agents/run_internal/prompt_cache_key.py:97-107` |
| Grouping hierarchy for key scoping | `resolve_run_grouping` orders: conversation_id → session_id → group_id → fresh per-run uuid4; docstring states "order matches prompt-cache grouping" | `src/agents/run_internal/run_grouping.py:12-34` |
| Key format/hashing | Run-scoped key is `agents-sdk:run:{uuid}`; session/group keys are sha256-derived 32-hex digests prefixed `agents-sdk:{kind}:` so raw IDs never leak upstream | `src/agents/run_internal/prompt_cache_key.py:119-130` |
| Provider gating | `_supports_default_prompt_cache_key()` on `OpenAIResponsesModel` requires `is_official_openai_client` (https + api.openai.com host check); Chat Completions equivalent uses `ChatCmplHelpers.is_openai` | `src/agents/models/openai_responses.py:505-506`, `src/agents/models/openai_client_utils.py:8-18`, `src/agents/models/openai_chatcompletions.py:82-83` |
| Runner wiring (streaming turn) | Resolver created once per run in `run.py`; per-turn resolve + settings merge before `model.stream_response` | `src/agents/run.py:804-806`, `src/agents/run_internal/run_loop.py:2201-2212` |
| Runner wiring (non-streaming turn) | Same resolver passed through `_run_streamed_impl`-style helpers and applied at second call site | `src/agents/run_internal/run_loop.py:984`, `src/agents/run_internal/run_loop.py:2578-2589` |
| Resume persistence of generated key | `_generated_prompt_cache_key` stored on `RunState`, serialized into state JSON (`generated_prompt_cache_key`) and restored on deserialize; propagated onto finalized results | `src/agents/run_state.py:808-809`, `src/agents/run_state.py:1795`, `src/agents/run_state.py:4266-4268`, `src/agents/run.py:840-842`, `src/agents/result.py:149-153` |
| Prompt-cache tests | Dedicated suite: default generation, reuse across turns, skip when model disables, respect of pre-set `extra_args`/`extra_body`, session/group boundaries, streaming parity, resume preservation | `tests/test_prompt_cache_key.py:39-269` |
| Versioned state fixtures include the key | Feature fixture `v1_8_prompt_cache_key.json` sets `"generated_prompt_cache_key"`; all version snapshots carry the field | `tests/fixtures/run_state/features/v1_8_prompt_cache_key.json:39`, `tests/fixtures/run_state/minimal/v1_15.json:41` |
| Explicit prompt-cache settings | `ModelSettings.prompt_cache_retention` (`in_memory`/`24h`) and `prompt_cache_options` (`{"mode": "explicit", "ttl": "30m"}`), validated against `PromptCacheOptions` annotations | `src/agents/model_settings.py:150-154`, `src/agents/model_settings.py:198-203`, `src/agents/model_settings.py:370-374` |
| Settings forwarding by adapters | Both Responses and Chat Completions request builders pass `prompt_cache_retention`/`prompt_cache_options` through (omitted when None) | `src/agents/models/openai_responses.py:993-994`, `src/agents/models/openai_chatcompletions.py:729-730` |
| Cache breakpoints preserved on conversion | Chat Completions converter copies `prompt_cache_breakpoint` from text/image/audio/file content parts | `src/agents/models/chatcmpl_converter.py:401-427`, `src/agents/models/chatcmpl_converter.py:445-527` |
| Prompt-caching documentation | Docs describe `prompt_cache_options`, explicit-mode breakpoints, and retention; internal module reference page exists for `run_internal.prompt_cache_key` | `docs/models/index.md:418-467`, `docs/ref/run_internal/prompt_cache_key.md:3` |
| Response caching | No client-side response cache found. Closest mechanisms are server-side storage knobs (`store`) and `previous_response_id` continuation — both delegate reuse to the server. Searched `cache|memo` across `src/agents/models/**`: only websocket-model instance caching and token-detail plumbing matched | `src/agents/model_settings.py:144-148` (search boundary noted) |
| Websocket model/connection reuse | Provider keeps a per-event-loop `WeakKeyDictionary` cache of websocket model wrappers keyed by `(client, transport, options)` so persistent connections are reused instead of rebuilt | `src/agents/models/openai_provider.py:125-129`, `src/agents/models/openai_provider.py:246-283` |
| Cached-token observability | `Usage` aggregates `cached_tokens` and `cache_write_tokens` across requests, normalizes missing fields to 0, serializes them for spans and state | `src/agents/usage.py:71-108`, `src/agents/usage.py:231-289`, `src/agents/usage.py:449-476` |
| MCP retrieval/tool-list cache | Opt-in `cache_tools_list` ("drastically improve latency"); dirty-flag cache `_cache_dirty`/`_tools_list` seeded dirty at startup; served from `list_tools()` only when enabled, non-dirty, and populated; manual `invalidate_tools_cache()` | `src/agents/mcp/server.py:885-890`, `src/agents/mcp/server.py:934`, `src/agents/mcp/server.py:947-949`, `src/agents/mcp/server.py:1427-1429`, `src/agents/mcp/server.py:1479-1480`, `src/agents/mcp/server.py:1083-1085` |
| Tool-cache mutation safety | All returned tools deep-copied via `_snapshot_tools` (callers cannot corrupt cached schemas); dynamic filters inspect detached copies; required-parameter validation reads the internal cache so schema tampering cannot bypass checks; `cached_tools` property returns snapshot or `None` sentinel | `src/agents/mcp/server.py:131-133`, `src/agents/mcp/server.py:861-866`, `src/agents/mcp/server.py:1042-1045`, `src/agents/mcp/server.py:1486-1489`, `src/agents/mcp/server.py:1576-1589` |
| Tool-cache tests | Suite covers hit/miss counts, invalidate, pagination cached before filtering, returned-list mutation not leaking, filter mutation not corrupting schemas, `None` sentinel before first list | `tests/mcp/test_caching.py:18-65`, `tests/mcp/test_caching.py:71-97`, `tests/mcp/test_caching.py:104-152`, `tests/mcp/test_caching.py:159-196`, `tests/mcp/test_caching.py:242-272`, `tests/mcp/test_caching.py:280-329`, `tests/mcp/test_caching.py:336-344` |
| Tool-cache docs | "Caching" section: set True only if tool definitions are stable; call `invalidate_tools_cache()` to refresh; partial pages never cached on failure | `docs/mcp.md:481-487` |
| Embedding caches | No evidence found. Searched `embedding|lru_cache|TTLCache|cachetools` across `src/`: the only `lru_cache` memoizes sandbox instructions text (`get_default_sandbox_instructions`); "embedding" appears only as prose. The SDK has no embedding API surface | `src/agents/sandbox/runtime_agent_preparation.py:7`, `src/agents/sandbox/runtime_agent_preparation.py:28-39` |
| Model-call batching | No batch-request builder for inference. Each agent turn issues exactly one provider request (`stream_response`/`response`); no dedupe of identical concurrent requests found | search boundary: `batch|Batch` matches in `src/agents/` are tracing/session/tool-execution/sandbox related, none build multi-prompt inference batches |
| Trace-export batching (non-model) | `BatchTraceProcessor` batches spans/traces: queue cap 8192, `max_batch_size=128`, 5s schedule delay, export trigger at 70% queue, drop-on-full with warnings, deadline-aware drain, exporter exceptions isolated | `src/agents/tracing/processors.py:541-582`, `src/agents/tracing/processors.py:597-621`, `src/agents/tracing/processors.py:653-718` |
| Concurrency (not batching) within turns | Function-tool calls from one model turn execute concurrently under `_FunctionToolBatchExecutor`; MCP servers optionally connect in parallel via worker tasks | `src/agents/run_internal/tool_execution.py:1552-1553`, `src/agents/run_internal/tool_execution.py:2322`, `src/agents/mcp/manager.py:498-503` |

## Answers to Dimension Questions

1. **Are model responses cached?**
   No client-side response cache exists. Every turn performs a live provider call (`src/agents/run_internal/run_loop.py:2220-2245`). Cost avoidance for repeated prefixes is delegated to OpenAI's server-side prompt cache, which the SDK steers via generated `prompt_cache_key`s (`src/agents/run_internal/prompt_cache_key.py:97-107`) and explicit options (`src/agents/model_settings.py:150-154`, `src/agents/model_settings.py:198-203`). Server-side response *storage* is configurable via `store` (`src/agents/model_settings.py:144-148`) and continued via `previous_response_id`, but that is conversation-state management, not local response reuse. Connection-level reuse exists for websocket transports (`src/agents/models/openai_provider.py:246-283`).

2. **Are embeddings reused?**
   No evidence found. The SDK exposes no embedding computation or vector-store client in `src/agents/`. Searches for `embedding`, `lru_cache`, `TTLCache`, and `cachetools` surfaced only an unrelated `lru_cache(maxsize=1)` around sandbox instruction loading (`src/agents/sandbox/runtime_agent_preparation.py:28-39`). Any embedding reuse would have to be implemented by applications or external retrieval layers.

3. **Is retrieval cached?**
   Partially — the nearest analog is the MCP tool-listing cache. With `cache_tools_list=True`, `list_tools()` results are fetched once per connection and served thereafter until `invalidate_tools_cache()` flips the dirty flag (`src/agents/mcp/server.py:1427-1429`, `src/agents/mcp/server.py:1083-1085`). It defaults to False, has no TTL, and caches only tool metadata (schemas), not retrieval results/content. Pagination failures never persist partial lists (`docs/mcp.md:481`; enforced by clearing tools and re-raising at `src/agents/mcp/server.py:1464-1474`).

4. **Are model calls batched efficiently?**
   No. There is no batch-request builder for inference; each model turn maps to one provider request, matching the chat/agent loop shape rather than offline batch APIs. Batching exists elsewhere: trace spans are exported in batches of up to 128 with time/ratio triggers (`src/agents/tracing/processors.py:548-554`, `src/agents/tracing/processors.py:670-694`), and function tools within a single turn run concurrently (`src/agents/run_internal/tool_execution.py:1552`). These reduce overhead around model calls but do not coalesce model requests.

## Architectural Decisions

- **Delegate prefix caching to the provider, steer it locally.** Rather than caching responses, the SDK generates stable routing hints (`prompt_cache_key`) that maximize server-side cache hits, with a strict grouping hierarchy so unrelated traffic does not share cache groups (`src/agents/run_internal/run_grouping.py:12-34`). Run-scoped fallback keys use an un-hashed `agents-sdk:run:` prefix because they are unique per run anyway (`src/agents/run_internal/prompt_cache_key.py:124-129`).
- **Conservative gating over blanket injection.** Default keys are generated only for models declaring support, which today means official api.openai.com clients (`src/agents/models/openai_responses.py:505-506`, `src/agents/models/openai_client_utils.py:8-11`). Proxied/self-hosted deployments must opt in explicitly, avoiding garbage parameters on incompatible providers.
- **User intent wins.** Any user-supplied `prompt_cache_key` in `extra_args` or `extra_body` suppresses generation entirely (`src/agents/run_internal/prompt_cache_key.py:54-57`, tested at `tests/test_prompt_cache_key.py:95-126`).
- **Cache identity survives interruption.** The generated key lives on `RunState` and serialized state JSON so a resumed run rejoins the same cache group (`src/agents/run_state.py:808-809`, `src/agents/run_state.py:1795`, `src/agents/run_state.py:4266-4268`).
- **Immutable-by-contract caches.** The MCP tool cache is never exposed directly; every read path hands out deep copies, and validation consults the pristine internal list (`src/agents/mcp/server.py:131-133`, `src/agents/mcp/server.py:1486-1489`, `src/agents/mcp/server.py:1576-1589`).

## Notable Patterns

- **Resolver object per run** encapsulating lazy key creation, memoization, and state write-back — a small stateful policy object threaded through both execution paths (`src/agents/run_internal/prompt_cache_key.py:16-88`).
- **Dirty-flag cache with seed-dirty initialization**, guaranteeing at least one real fetch after connect (`src/agents/mcp/server.py:947-949`).
- **Snapshot-on-read discipline**: `cached_tools` preserves the `None` "never fetched" sentinel distinct from an empty list (`tests/mcp/test_caching.py:336-344`).
- **Observability of cache economics**: `cached_tokens`/`cache_write_tokens` are normalized, aggregated per run, and emitted into trace span data (`src/agents/usage.py:286-289`, `src/agents/usage.py:449-458`), giving operators the signal needed to confirm caching works.
- **Backpressured background batching** for traces with drop-oldest-on-full and exporter fault isolation (`src/agents/tracing/processors.py:601-604`, `src/agents/tracing/processors.py:696-717`).

## Tradeoffs

- **Server-side dependence vs local control**: prompt-cache savings depend on OpenAI honoring the key; the SDK cannot guarantee or verify hits locally beyond usage counters. This keeps the client simple but makes cost behavior opaque.
- **Opt-in tool cache vs latency**: `cache_tools_list=False` by default means every run pays a `list_tools` round-trip per server unless users accept staleness risk (`docs/mcp.md:487`).
- **No TTL anywhere**: both the tool cache and the generated run-scoped key rely on explicit invalidation/lifecycle rather than expiry — simple and predictable, but stale tool schemas persist until manually invalidated.
- **Hashed keys trade debuggability for hygiene**: session/group IDs are hashed before leaving the process (`src/agents/run_internal/prompt_cache_key.py:119-121`), preventing ID leakage but making cross-referencing sent keys back to sessions harder without keeping the mapping.
- **Deep-copy snapshots cost CPU** on large tool catalogs each listing, in exchange for corruption-proof caches (`src/agents/mcp/server.py:1486-1489`).

## Failure Modes / Edge Cases

- **Cache poisoning attempts are contained**: mutating a returned tool's `input_schema.required` does not bypass required-parameter validation, verified end-to-end in tests (`tests/mcp/test_caching.py:242-272`, `tests/mcp/test_caching.py:280-329`).
- **Partial pagination never cached**: if a later page fails or a cursor repeats, accumulated tools are cleared and an error raised, so the dirty flag stays set (`src/agents/mcp/server.py:1448-1474`).
- **Resume correctness**: the generated key round-trips through serialized state; a missing/corrupt value degrades to `None` rather than breaking deserialization (`src/agents/run_state.py:4266-4268`, fixture test `tests/test_run_state_compatibility_corpus.py:233-253`).
- **Queue saturation drops telemetry**: full trace queues drop items with warnings rather than blocking the run loop (`src/agents/tracing/processors.py:601-604`) — availability prioritized over completeness.
- **Edge case not handled**: two agents sharing one `Session` concurrently generate the same session-scoped cache key by design; this maximizes hits but couples their cache eviction behavior — no isolation knob exists between them.

## Future Considerations

- Add optional TTL or change-detection (e.g., ETag/hash comparison) to the MCP tool cache to reduce stale-schema risk while keeping the latency win.
- Expose cache-hit observability at the run level (e.g., summary of cached vs uncached input tokens per run result) so applications can act on the existing usage data without parsing span payloads (`src/agents/usage.py:449-458`).
- Consider a lightweight in-process response memoization hook for deterministic replay/testing workloads, clearly segregated from production paths where provider-side caching remains authoritative.
- Document the generated-key scheme publicly (currently only an auto-generated API reference page exists at `docs/ref/run_internal/prompt_cache_key.md:3`); the behavioral spec lives in tests (`tests/test_prompt_cache_key.py`).

## Questions / Gaps

- No evidence was found for embedding or vector-retrieval caching; confirmed by searches across `src/` (boundary: whole `src/agents` tree, terms `embedding`, `lru_cache`, `TTLCache`, `cachetools`).
- Whether the OpenAI backend's implicit prompt caching actually keys off the injected `prompt_cache_key` is outside this repository; the SDK-side contract ends at placing the field in `extra_args` (`src/agents/run_internal/prompt_cache_key.py:105-107`).
- The exact semantics of `ChatCmplHelpers.is_openai` vs base-URL checking for Chat Completions gating were not traced further than `src/agents/models/openai_chatcompletions.py:82-83`; tests confirm custom base URLs receive no generated key (`tests/models/test_openai_chatcompletions.py:1278-1285`).

---

Generated by `Dimension 20.02: Caching, Batching, and Reuse` against `openai-agents-sdk`.

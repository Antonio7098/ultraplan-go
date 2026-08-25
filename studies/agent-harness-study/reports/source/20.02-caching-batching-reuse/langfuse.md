# Source Analysis: langfuse

## 20.02 Caching, Batching, and Reuse

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript monorepo: Next.js (`web`), BullMQ worker (`worker`), shared package (`packages/shared`); Postgres, ClickHouse, Redis, S3 |
| Analyzed | 2026-08-25 |

## Summary

Langfuse is an observability platform, not an inference harness, so "caching model responses" manifests differently than in an agent runtime. The codebase implements a deliberate, layered caching and batching architecture around its own expensive operations rather than around LLM calls:

1. **Two-tier caches (Redis + in-process LRU)** for hot read paths on the ingestion pipeline — model/pricing matching (`packages/shared/src/server/ingestion/modelMatch.ts:31-42`), API-key authentication (`web/src/features/public-api/server/apiAuth.ts:320-413`), and prompt resolution (`packages/shared/src/server/services/PromptService/index.ts:149-177`). All are env-gated, emit hit/miss metrics, include negative caching, and have explicit invalidation paths.
2. **A buffered batch writer for ClickHouse** (`worker/src/services/ClickhouseWriter/index.ts:35-99`) that coalesces ingestion writes by size (default 1,000 records) or time (default 1s), with retry, batch splitting, truncation, and drop-after-max-attempts safeguards.
3. **Deduplication/reuse layers**: S3 event grouping per entity id (`packages/shared/src/server/ingestion/processEventBatch.ts:203-273`), Redis-backed OTel seen-trace suppression (`packages/shared/src/server/otel/OtelIngestionProcessor.ts:3380-3427`), media content-hash dedup (`web/src/features/media/server/mediaService.ts:67-91`), tokenizer instance reuse (`worker/src/features/tokenisation/usage.ts:157-185`), and in-memory trace reuse across eval configs (`worker/src/features/evaluation/evalService.ts:356-448`).

What is absent is equally clear: LLM calls made *by* Langfuse (evals, playground, experiments) are never cached and never batched — one `generateLLMText` call per job/item. Provider-side prompt caching is only observed as usage metadata (`input_cached_tokens`), not exploited. Embeddings exist only as ingested data types; there is no embedding computation or embedding/retrieval cache to assess.

## Rating

**8 / 10**

Rationale against the rubric:

- **Clear model with tests**: every major cache has a dedicated implementation with an explicit interface — `LocalCache<V>` (`packages/shared/src/server/cache/localCache.ts:18-49`) wrapping `lru-cache`, plus direct Redis cache services (`PromptService`, `apiAuth`). Each is covered by tests: prompt cache behavior including epoch invalidation and Redis-failure fallbacks (`web/src/__tests__/server/promptCache.servertest.ts:62-280`), two-tier model match including local-cache serving after Redis deletion and negative caching (`worker/src/__tests__/modelMatch.test.ts:61-240`), and ClickHouseWriter batching/retry/drop semantics (`worker/src/services/ClickhouseWriter/ClickhouseWriter.unit.test.ts:66-428`).
- **Operational safeguards**: epoch-token rotation instead of key scans for prompt invalidation (`packages/shared/src/server/services/PromptService/index.ts:179-192`), a distributed lock guarding full model-cache clears (`packages/shared/src/server/ingestion/modelMatch.ts:430-470`), zod validation of cached payloads with self-healing eviction of malformed entries (`web/src/features/public-api/server/apiAuth.ts:395-408`), batch-splitting and record truncation on oversized ClickHouse writes (`worker/src/services/ClickhouseWriter/index.ts:180-286`).
- **Observability**: hit/miss counters, queue-length gauges, wait-time histograms across all mechanisms (`packages/shared/src/server/cache/localCache.ts:131-141`, `packages/shared/src/server/ingestion/modelMatch.ts:197-201`, `worker/src/services/ClickhouseWriter/index.ts:374-388`, 500-531).
- **Why not 9–10**: the ClickhouseWriter silently drops rows after max attempts with only a TODO for a dead-letter queue (`worker/src/services/ClickhouseWriter/index.ts:544-548`); the Redis recently-processed dedup and the local model-match L1 cache default to off (`worker/src/env.ts:211-214`, `packages/shared/src/env.ts:106-108`); LLM calls themselves get no batching or response caching; and several caches rely on TTL-only consistency by documented design tradeoff (`packages/shared/src/server/ingestion/modelMatch.ts:29-30`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Generic local cache abstraction (LRU) | `LocalCache<V>` wraps `lru-cache` with namespace, enabled flag, TTL, max size; emits `langfuse.local_cache.*` hit/miss/evict/size metrics; `getOrLoad()` read-through helper | packages/shared/src/server/cache/localCache.ts:18-43, 100-129, 131-141 |
| Prompt cache (Redis) | `PromptService.getPrompt` checks Redis before Postgres, caches resolved prompts with TTL; enabled via `LANGFUSE_CACHE_PROMPT_ENABLED` (default true), TTL 3600s | packages/shared/src/server/services/PromptService/index.ts:49-81, 166-177; packages/shared/src/env.ts:118-119 |
| Prompt cache invalidation via epoch rotation | `invalidateCache` writes a fresh project-scoped epoch token (48-bit random, 7-day TTL) so old keys expire naturally instead of being scanned/deleted | packages/shared/src/server/services/PromptService/index.ts:179-192, 216-240 |
| Model-match two-tier cache | L1 `LocalCache` (10s TTL, 20k entries, off by default) → L2 Redis (24h TTL, on by default) → Postgres regex lookup | packages/shared/src/server/ingestion/modelMatch.ts:26-42, 63-104; packages/shared/src/env.ts:104-117 |
| Negative caching (model match) | `NOT_FOUND_TOKEN` stored in Redis/local cache for unmatched models to avoid repeated Postgres misses | packages/shared/src/server/ingestion/modelMatch.ts:95-102, 203-205, 308-325 |
| Model-match invalidation | Project-scoped scan+delete (`clearModelCacheForProject`), full clear guarded by `LOCK:model-match-clear` Redis lock on worker startup | packages/shared/src/server/ingestion/modelMatch.ts:396-418, 430-470 |
| API-key auth cache | Redis cache keyed on fast hash, sliding TTL via `getex` (300s default), negative token `API_KEY_NON_EXISTENT`, zod-validated payload with malformed-entry deletion, hit/miss metrics | web/src/features/public-api/server/apiAuth.ts:320-413; packages/shared/src/server/auth/types.ts:38-40; web/src/env.mjs:355-356 |
| API-key cache invalidation | `invalidateCachedApiKeys` deletes keys on CRUD; `invalidateAllCachedApiKeys` scan+delete; cluster-safe del helpers | packages/shared/src/server/auth/invalidateApiKeys.ts:24-50, 136-150; packages/shared/src/server/redis/redis.ts:349-379 |
| Eval job-config negative cache | "No executable configs" marker cached in Redis per project/type, 10-minute TTL, cleared when configs become executable | packages/shared/src/server/evalJobConfigCache.ts:21-99 |
| ClickHouse write batching | `ClickhouseWriter` singleton buffers per-table queues; flush at `LANGFUSE_INGESTION_CLICKHOUSE_WRITE_BATCH_SIZE` (1000) or interval (1000ms); single `insert` call per flush | worker/src/services/ClickhouseWriter/index.ts:46-65, 364-371, 576-594, 596-626; worker/src/env.ts:113-120 |
| Batch failure safeguards | Exponential-backoff retry; split-batch-on-string-length-error; truncate-oversized-fields-on-size-error; requeue-with-attempt-cap then drop with `rows_dropped` metric | worker/src/services/ClickhouseWriter/index.ts:137-171, 406-498, 532-573 |
| Ingestion dedup cache | Redis "recently processed" seen-key check before processing a fileKey (5-minute EX), gated by `LANGFUSE_ENABLE_REDIS_SEEN_EVENT_CACHE` (default false) | worker/src/queues/ingestionQueue.ts:84-109, 241-264; worker/src/env.ts:211-214 |
| S3 event batching at producer | Events grouped by `eventBodyId` into one S3 object ("reduces infra interactions per event"); batch sorted updates-last; concurrent S3 reads chunked by `LANGFUSE_S3_CONCURRENT_READS` (50) | packages/shared/src/server/ingestion/processEventBatch.ts:201-294, 441-460; worker/src/queues/ingestionQueue.ts:201-208; worker/src/env.ts:431 |
| OTel trace dedup | Per-trace Redis `SET NX EX 600` builds a seen-set to suppress redundant shallow trace-create events; fail-safe empty set on Redis error | packages/shared/src/server/otel/OtelIngestionProcessor.ts:3380-3427, 683-688, 929-949 |
| Org-created-at routing cache | `LocalCache` for immutable org signup dates (1h TTL, 10k entries) used as routing hint for v4 OTel direct-write decision | worker/src/queues/otelIngestionQueue.ts:216-228, 250-286 |
| Media content-hash dedup | Upload-URL issuance short-circuits when a project's media with identical sha256 already exists (uploadUrl: null) | web/src/features/media/server/mediaService.ts:67-91 |
| Tokenizer instance reuse | Tiktoken encodings memoized per model in module-level map with explicit `freeAllTokenizers()` cleanup | worker/src/features/tokenisation/usage.ts:157-185 |
| Request-scoped reuse in evals | Trace and dataset-item ids fetched once per batch and reused in memory across all matching eval configs, with in-memory filter evaluation ("cache" decision source) | worker/src/features/evaluation/evalService.ts:356-448, 538-552 |
| No LLM response caching / no LLM call batching | Evals execute one `generateLLMText` per job; experiments loop items sequentially, one call each; playground handler has no cache layer | worker/src/features/evaluation/evalExecutionDeps.ts:241; worker/src/features/experiments/experimentServiceClickhouse.ts:352-365; web/src/features/playground/server/chatCompletionHandler.ts |
| Provider prompt-cache tokens observed, not exploited | AI SDK telemetry maps `cacheRead`/`cacheWrite` to `gen_ai.usage.cache_read.input_tokens` etc.; OTel mapper folds `cachedInputTokens`/Anthropic cache metadata into `usageDetails` cost keys | packages/shared/src/server/llm/ai-sdk/telemetry.ts:373-376; packages/shared/src/server/otel/OtelIngestionProcessor.ts:2699-2770 |
| ClickHouse query condition cache | Optional server-side `use_query_condition_cache` setting toggled by env for dashboard/query-executor queries | packages/shared/src/features/query/server/queryExecutor.ts:57-65; packages/shared/src/env.ts:157 |
| Tests: prompt cache | Covers hit/miss, version-vs-label key separation, bypass on `resolve:false`, disabled cache, null Redis, Redis.get/set failure fallback, epoch rotation | web/src/__tests__/server/promptCache.servertest.ts:62-280 |
| Tests: model match | Redis hit, Postgres fallback, not-found caching, local-cache serving after Redis delete, short-TTL local negative cache, project-scope clearing incl. not-found tokens | worker/src/__tests__/modelMatch.test.ts:61-360 |
| Tests: ClickhouseWriter | Singleton, batch-size flush, interval flush, error retry, drop-after-max-attempts, timeout retry within flush, graceful shutdown, high-load concurrency, metrics, Decimal64 clamping | worker/src/services/ClickhouseWriter/ClickhouseWriter.unit.test.ts:66-530 |
| Tests: API-key cache | Read-from-cache, Prisma-not-called-on-hit, cache deletion on key delete | web/src/__tests__/server/api-auth.servertest.ts:422, 690, 805 |

## Answers to Dimension Questions

**1. Are model responses cached?**
No. Langfuse makes outbound LLM calls for LLM-as-judge evaluations, dataset experiments, and the playground; none of these paths consult or populate any response cache — each job issues its own `generateLLMText` call (`worker/src/features/evaluation/evalExecutionDeps.ts:241`; `worker/src/features/experiments/experimentServiceClickhouse.ts:354-365`), and the playground completion handler contains no caching (`web/src/features/playground/server/chatCompletionHandler.ts`). This is coherent for an observability product: repeated identical requests are user-driven evaluations where stale cached scores would be wrong, not free. The platform does *observe* provider-side prompt caching — cache-read/cache-write tokens from the AI SDK and OTel GenAI attributes are normalized into usage/cost details (`packages/shared/src/server/llm/ai-sdk/telemetry.ts:373-376`; `packages/shared/src/server/otel/OtelIngestionProcessor.ts:2703-2770`) — so customers can see cache savings, but Langfuse itself does not influence them.

**2. Are embeddings reused?**
No evidence found for embedding computation or embedding-vector caching inside this repository. Embeddings appear only as an ingested observation type (`EMBEDDING_CREATE` schema at `packages/shared/src/server/ingestion/types.ts:292`, OTel span mapping at `packages/shared/src/server/otel/ObservationTypeMapper.ts:225`) and as seeded demo data (`packages/shared/scripts/seeder/scenarios/timeline-shapes.ts:43`). There is no vector store, no retrieval component, and hence no embedding-reuse mechanism. Search boundary: repo-wide grep for `embedding` (100+ matches, all ingest/display/tokenizer-pricing contexts).

**3. Is retrieval cached?**
There is no RAG/retrieval subsystem to cache. What exists instead are read-path caches serving the platform's own hot lookups: prompt resolution (Redis, `packages/shared/src/server/services/PromptService/index.ts:54-76`), model+pricing matching (two-tier, `packages/shared/src/server/ingestion/modelMatch.ts:63-104`), API-key auth (Redis with sliding TTL, `web/src/features/public-api/server/apiAuth.ts:320-413`), eval-config presence markers (Redis, `packages/shared/src/server/evalJobConfigCache.ts:27-68`), and org-signup-date routing hints (local LRU, `worker/src/queues/otelIngestionQueue.ts:223-228`). Additionally, ClickHouse query execution can enable the server-side query condition cache (`packages/shared/src/features/query/server/queryExecutor.ts:59-61`), which caches per-block condition evaluation results inside ClickHouse.

**4. Are model calls batched efficiently?**
No. Every LLM invocation path is one-call-per-unit-of-work: LLM-as-judge jobs run individually through their queue, experiment items are processed sequentially with one generation each (`worker/src/features/experiments/experimentServiceClickhouse.ts:354-365`), and there is no batch-request builder that packs multiple prompts into a single model request anywhere in `packages/shared/src/server/llm/*`. The efficient batching in this codebase targets storage, not models: the `ClickhouseWriter` coalesces up to 1,000 rows or 1 second of writes into single inserts across eight tables (`worker/src/services/ClickhouseWriter/index.ts:46-65, 114-135`), and the producer groups same-entity events into one S3 object per batch to cut write amplification (`packages/shared/src/server/ingestion/processEventBatch.ts:203-210, 283-294`). Within a single eval batch job, DB reads are shared across configs in memory (`worker/src/features/evaluation/evalService.ts:356-448`), which reduces load but does not reduce model-call count.

## Architectural Decisions

1. **Layered cache topology (L1 memory → L2 Redis → source of truth).** Model matching composes a process-local LRU in front of Redis in front of Postgres (`packages/shared/src/server/ingestion/modelMatch.ts:63-104`). The L1 is deliberately TTL-only (10s) because cross-container correctness comes from Redis invalidation plus short local expiry — an explicit, commented tradeoff (`packages/shared/src/server/ingestion/modelMatch.ts:29-30`).
2. **Invalidation strategy varies with data mutability.** Prompts use epoch-token rotation (project-scoped because resolved prompts can pull transitive dependencies; O(1) write, zero scans, old keys expire naturally — `packages/shared/src/server/services/PromptService/index.ts:184-220`). Model-match uses targeted scan+delete per project plus a lock-guarded global clear at worker startup (`packages/shared/src/server/ingestion/modelMatch.ts:396-470`). API keys use precise per-hash deletes on CRUD events (`packages/shared/src/server/auth/invalidateApiKeys.ts:41,87,123`).
3. **Negative caching everywhere misses are common.** Non-existent API keys (`API_KEY_NON_EXISTENT`, `packages/shared/src/server/auth/types.ts:38`), unmatched models (`NOT_FOUND_TOKEN`, `packages/shared/src/server/ingestion/modelMatch.ts:308`), and config-less projects (`setNoEvalConfigsCache`, `packages/shared/src/server/evalJobConfigCache.ts:51-68`) all cache the absence result — protecting Postgres from hot-miss storms on the ingestion path.
4. **Buffered micro-batching at the storage boundary.** Rather than writing per event, the worker funnels all ClickHouse inserts through a singleton buffered writer with size/time triggers, isolating latency-sensitive queue processing from write throughput (`worker/src/services/ClickhouseWriter/index.ts:83-99, 576-594`).
5. **Env-flag gating for every cache/batch knob**, with validated zod schemas and operational defaults: prompt cache on (3600s), model-match Redis on (24h), local L1 off, seen-event dedup off, CH batch 1000×1000ms (`packages/shared/src/env.ts:104-119`; `worker/src/env.ts:113-124, 211-214`). This lets operators shed cache-consistency risk at the cost of DB load.
6. **Fail-open caches.** All cache reads catch errors and fall through to the source of truth; prompt reads fall back to DB on Redis errors (`packages/shared/src/server/services/PromptService/index.ts:159-163`), OTel dedup returns an empty set on Redis failure (`packages/shared/src/server/otel/OtelIngestionProcessor.ts:3421-3426`). Caching accelerates but is never load-bearing for correctness.

## Notable Patterns

- **Reusable `LocalCache<V>` primitive** with namespace-tagged metrics, dispose-hook eviction counting, and a `getOrLoad` read-through helper returning `{ value, ttlMs, source }` so callers can attribute provenance (`packages/shared/src/server/cache/localCache.ts:5-9, 100-129`) — reused for model match and OTel routing hints rather than ad-hoc Maps.
- **Self-healing deserialization**: cached API-key payloads are re-validated with zod on every read, and unparseable entries are deleted so poisoning cannot persist beyond one request (`web/src/features/public-api/server/apiAuth.ts:395-408`).
- **Sliding-TTL hot keys**: API-key cache uses `GETEX ... EX` so continuously-used credentials never expire mid-traffic while idle ones do (`web/src/features/public-api/server/apiAuth.ts:385-389`).
- **Producer-side batch shaping**: the ingestion producer sorts update-events last (`sortBatch`, `packages/shared/src/server/ingestion/processEventBatch.ts:441-460`) and precomputes entityType/bucketPrefix once per entity id so downstream consumers share one source of truth (`processEventBatch.ts:203-217`) — batching decisions encode ordering-correctness, not just throughput.
- **Adaptive degradation under provider throttling**: S3 SlowDown errors flip projects onto a secondary ingestion queue via a Redis flag (`worker/src/queues/ingestionQueue.ts:111-136, 330-343`) — a batching-adjacent backpressure pattern.
- **Cluster-aware cache plumbing**: hash-tagged Redis keys for model-match (`packages/shared/src/server/ingestion/modelMatch.ts:363-370`), `safeMultiDel`/`safeMultiGet` falling back from MGET/DEL to per-key ops under cluster mode (`packages/shared/src/server/redis/redis.ts:349-379`).

## Tradeoffs

- **Freshness vs. load on the prompt cache**: prompts resolve label selectors (e.g. `production`) whose meaning changes on deploy; epoch rotation means up to `LANGFUSE_CACHE_PROMPT_TTL_SECONDS` (default 1h) of staleness window is closed only when writers call `invalidateCache` (`web/src/features/prompts/server/actions/createPrompt.ts:242`; `deletePrompt.ts:126`; `updatePrompts.ts:137`) — correct within one deployment, but readers in other containers serve stale content until the epoch write propagates.
- **Memory vs. cross-container consistency in L1**: the model-match LRU serves up to 10s-stale pricing after a Redis delete; accepted explicitly because pricing drift is low-harm (`packages/shared/src/server/ingestion/modelMatch.ts:29-30`) — but it ships disabled by default, suggesting the team did not yet consider it proven enough to be default-on.
- **Throughput vs. durability in ClickhouseWriter**: batching buys massive insert efficiency but a hard crash loses the in-memory buffer (no WAL), and exhausted retries drop rows with only metrics and logs — an acknowledged TODO (`worker/src/services/ClickhouseWriter/index.ts:532-573`).
- **Negative-cache TTL vs. setup friction**: a freshly created model or eval config can be invisible to the ingestion path until the 10-minute eval-config marker or 24-hour model-match entry expires/is invalidated (`packages/shared/src/server/evalJobConfigCache.ts:21`; `packages/shared/src/env.ts:105`); both have explicit clear functions, so the tradeoff was taken knowingly.
- **S3-grouping vs. list-cost**: coalescing per-entity events into one object minimizes writes but forces the consumer to list+download sibling files unless the newer skip-list fast path applies (`worker/src/queues/ingestionQueue.ts:160-209`), trading consumer I/O for producer efficiency.

## Failure Modes / Edge Cases

- **Partial batch failure isolation**: ClickHouse string-length errors split the batch in half and retry recursively, falling back to field truncation at batch-size 1 (`worker/src/services/ClickhouseWriter/index.ts:180-214, 187-203`); oversize JSON errors trigger one-time record truncation with a visible `[TRUNCATED]` marker (`index.ts:216-286, 463-484`).
- **Silent data loss path**: after `LANGFUSE_INGESTION_CLICKHOUSE_MAX_ATTEMPTS` (default 3) failed flushes, records are dropped and counted (`langfuse.queue.clickhouse_writer.rows_dropped`) with logged ids, but not persisted anywhere (`worker/src/services/ClickhouseWriter/index.ts:536-572`) — recovery requires manual replay from S3 raw events.
- **Cache-poisoning resistance**: malformed cached API keys are detected by zod and evicted (`web/src/features/public-api/server/apiAuth.ts:401-407`); unknown model-match cache formats are warned and treated as miss (`packages/shared/src/server/ingestion/modelMatch.ts:224-228`).
- **Concurrent initialization races**: epoch tokens use SET NX with a re-read of the winner value (`packages/shared/src/server/services/PromptService/index.ts:235-239`); full model-cache clears use a 10-minute Redis lock to prevent N workers from stampeding scans at boot (`packages/shared/src/server/ingestion/modelMatch.ts:437-452`).
- **Redis outage behavior**: every cache degrades to source-of-truth queries (prompt: DB; model match: Postgres; auth: Prisma; OTel dedup: reprocessing duplicates for ≤10 min windows) rather than failing requests — verified by dedicated tests (`web/src/__tests__/server/promptCache.servertest.ts:237-280`; `worker/src/__tests__/modelMatch.test.ts:146-166`).
- **Dedupe gaps by design**: the recently-processed fileKey cache defaults off (`worker/src/env.ts:211-214`), so without it, duplicate BullMQ deliveries rely on downstream idempotent merges; the OTel seen-trace window is fixed at 600s, so traces resuming after a 10-minute silence re-emit a shallow create event (`packages/shared/src/server/otel/OtelIngestionProcessor.ts:3389-3391`).
- **Decimal64 overflow clamping** before batch insert prevents one extreme cost value from poisoning a whole flushed batch (`worker/src/services/ClickhouseWriter/index.ts:288-362`, tested at `ClickhouseWriter.unit.test.ts:428-530`).

## Future Considerations

- **Persist failed batches**: implement the noted dead-letter sink for dropped ClickhouseWriter rows (`worker/src/services/ClickhouseWriter/index.ts:544`) — the S3 raw-event log already provides replay material (`worker/src/scripts/replayIngestionEvents/s3-ingestion-event-replay.ts` exists), so wiring automatic DLQ→replay would close the loss gap.
- **Turn the L1 model-match cache on by default** once observed safe; it currently requires opt-in despite being tested (`worker/src/__tests__/modelMatch.test.ts:195-240`), leaving most deployments paying Redis RTTs on every observation.
- **Reuse across sequential eval items**: experiment execution could share fetched dataset-item IO and template compilation across items (`worker/src/features/experiments/experimentServiceClickhouse.ts:354-365`) even if model calls stay unbatched.
- **Expose provider prompt-cache economics**: cache-read/write tokens are captured in usage details (`packages/shared/src/server/otel/OtelIngestionProcessor.ts:2703-2770`); surfacing them as first-class cost-line analytics would let users act on caching without Langfuse caching anything itself.
- **Re-evaluate seen-event cache default**: enabling `LANGFUSE_ENABLE_REDIS_SEEN_EVENT_CACHE` by default would make at-least-once queue delivery cheaply idempotent for the 5-minute window at trivial Redis cost.

## Questions / Gaps

- **Are model responses cached?** No — answered definitively by absence: no cache lookup surrounds `generateLLMText`/`streamLLMText` call sites in web, worker, or shared (`worker/src/features/evaluation/evalExecutionDeps.ts:241`; `web/src/features/playground/server/chatCompletionHandler.ts`). No evidence found of any HTTP-level response caching on public API GET routes either (no `Cache-Control`/`s-maxage` headers in `web/src/pages/api/public/**`).
- **Embedding/retrieval caching**: not applicable — searched all of `packages/shared`, `web`, `worker`, `ee` for embedding/vector/similarity infrastructure; only ingestion typing, OTel mapping, and pricing seeders matched. Stated as "No evidence found" per rules.
- **Unresolved quantitatives**: production hit rates, buffer-loss frequency (`rows_dropped`), and staleness impact of the 1h prompt TTL are observable via the emitted metrics (`langfuse.local_cache.*`, `langfuse.model_match.cache_*`, `ingestion_clickhouse_insert_queue_length`) but no dashboards/SLO definitions live in this repository, so operational maturity under scale could not be verified from code alone.
- **Client-side (SDK) prompt caching**: the JS/Python SDKs that also consume Langfuse prompt management are separate repositories and out of scope here; their client-side caching behavior is unassessed from this source.

---

Generated by dimension 20.02 (Caching, Batching, and Reuse) against `langfuse`.

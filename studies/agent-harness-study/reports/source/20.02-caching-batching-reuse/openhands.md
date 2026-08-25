# Source Analysis: openhands

## Dimension 20.02 — Caching, Batching, and Reuse

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19 (Vite, TanStack Query, Zustand) — the "Agent Canvas" frontend; LLM inference is delegated to a separate Python agent-server backend |
| Analyzed | 2026-08-25 |

## Summary

This source is the **OpenHands Agent Canvas frontend** (`AGENTS.md`, repo map table: "This repo (`OpenHands/OpenHands`) is **only the agent-canvas frontend**"). It makes **no direct LLM calls** — all inference goes through the agent-server backend via the sanctioned `@openhands/typescript-client` (enforced by CI guard `src/api/no-direct-agent-server-calls.test.ts`). Consequently, the classic harness-level caching subjects (model-response caches, prompt-cache construction, embedding reuse, retrieval caches) are either **absent by architecture** or implemented server-side outside this source.

What this repo *does* implement is a disciplined **client-side data-caching and batching layer**:

1. **Provider prompt-cache observability**: the UI ingests and displays `cache_read_tokens` / `cache_write_tokens` from the backend's accumulated token usage, making provider-side prompt caching visible to users.
2. **A schema-driven `prompt_cache_retention` LLM setting label**, indicating the backend exposes a prompt-cache retention knob that this frontend renders.
3. **Streaming delta batching**: a dedicated, well-tested `StreamingDeltaBatcher` coalesces WebSocket token deltas into at most one store commit/render per animation frame.
4. **Request batching**: batch REST endpoints for conversation lookups (`batchGetAppConversations`), fixed-concurrency file-upload batches, and delegated telemetry batching in the PostHog SDK.
5. **Layered HTTP/data caches**: TanStack Query defaults, a module-scope settings cache with explicit TTL + invalidation, a host-keyed `/server_info` cache, an in-memory TTL + negative cache with in-flight dedup for automation SDK versions, and session-long caches for expensive subprocess probes.

On the dimension's core question — *"Can repeated identical requests avoid paying full cost each time?"* — the answer inside this source is **yes for API/data requests (client-side), but only indirectly for model calls** (provider-side prompt caching is observed and surfaced, not constructed or controlled here).

## Rating

**Score: 4 / 10**

Rationale against the dimension rubric:

- The dimension's primary subjects are largely **absent from this source**: no model-response cache, no client-side prompt-cache construction, no embeddings, no retrieval pipeline (search evidence below). That alone caps the score in the 1–3 band for those areas.
- However, what exists is far from ad-hoc: the streaming-delta batcher has explicit interfaces and strong tests (`__tests__/utils/streaming-delta-batcher.test.ts:53-238`), caches have explicit TTLs, scoped keys, invalidation hooks, and negative-caching/dedup semantics. This lifts the overall rating into the low end of the **"present but inconsistent" (4–6)** band: the mechanisms are real but cover only the client-data layer, while the LLM-cost reuse story is delegated to the backend/provider and merely observed here.
- If judged purely as a web-frontend data layer it would score ~7 (clear model, tests, safeguards); judged on this dimension's actual scope (LLM/prompt/embedding/retrieval reuse), the effective score within this source is **4**.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Frontend-only architecture (no direct LLM calls) | Repo map states this repo is only the frontend; API access rules ban raw axios/fetch to the agent-server, enforced by a CI guard test | `AGENTS.md` (Repository Map section); `src/api/no-direct-agent-server-calls.test.ts:1` |
| Model-response cache | No `/chat/completions` call anywhere in `src/`; the only completion endpoint is the mock-LLM test fake | `tests/e2e/mock-llm/scripts/mock-llm-server.py:3,156` |
| Prompt-cache metrics types | `cache_read_tokens` / `cache_write_tokens` fields on the conversation-state usage payload | `src/types/agent-server/core/events/conversation-state-event.ts:14-15` |
| Prompt-cache metric normalization | Backend stats mapped into typed usage with `numberOrZero(cache_read_tokens/cache_write_tokens)` | `src/api/conversation-service/agent-server-conversation-service.api.ts:123-124` |
| Prompt-cache accumulation across turns | Metrics reducer sums `cache_read_tokens`/`cache_write_tokens` into combined token usage | `src/utils/conversation-metrics.ts:49-54` |
| Prompt-cache display (UI) | Usage panel renders "Cache hit"/"Cache write" rows from live metrics | `src/components/features/conversation/metrics-modal/usage-section.tsx:24-37`; i18n keys `CONVERSATION$CACHE_HIT`/`CONVERSATION$CACHE_WRITE` in `src/i18n/translation.json:21915,21932` |
| Live vs REST metrics merge | Hook prefers live WS store, falls back to 30s REST poll including cache token fields | `src/hooks/use-live-conversation-metrics.ts:53-76` |
| Prompt-cache retention setting surface | Convention-based i18n key generation renders backend schema field `llm.prompt_cache_retention` as "Prompt Cache Retention" | `src/utils/sdk-settings-field-metadata.ts:25-31`; `SCHEMA$LLM$PROMPT_CACHE_RETENTION$LABEL/$DESCRIPTION` in `src/i18n/translation.json:3861,3878` |
| Embedding cache / retrieval cache | Searched `src/`, `docs/`, `specs/` for `embedding|vector|retriev|semantic|rag` — zero relevant matches (only CSS "semantic tokens", PATCH "semantics") | search boundary noted in Questions/Gaps |
| Streaming delta batching | `createStreamingDeltaBatcher` buffers deltas and flushes ≤1 commit per animation frame via injectable scheduler | `src/utils/streaming-delta-batcher.ts:36-75` |
| Delta batcher wiring | Separate batchers for main and planning WS streams commit through the event store | `src/contexts/conversation-websocket-context.tsx:162-180` |
| Delta batcher tests | Coalescing per frame, ordered content/reasoning merges, byte-exactness over 5000 sub-frame deltas, flush-before-durable-event ordering | `__tests__/utils/streaming-delta-batcher.test.ts:54-77,79-95,97-115,145-172,185-238` |
| Batch REST reads (conversations) | `batchGetAppConversations(ids)` fans out to one typed-client call (or cloud proxy) | `src/api/conversation-service/agent-server-conversation-service.api.ts:599-615` |
| Batch read consumers | Sub-conversations fetched as one batch query; single conversation also resolved through the batch endpoint; cloud mirror of the same interface | `src/hooks/query/use-sub-conversations.ts:35-45`; `src/hooks/query/use-user-conversation.ts:55-57`; `src/api/cloud/conversation-service.api.ts:115-126` |
| Cache-patching instead of refetch | Mutations patch cached `AppConversation` objects in both single-item and paginated list caches rather than re-downloading | `src/hooks/mutation/conversation-mutation-utils.ts:126-171` |
| File upload batching | Fixed batches of 5 concurrent uploads (`FILE_UPLOAD_CONCURRENCY = 5`) via sliced `Promise.all` waves | `src/api/conversation-file-upload.api.ts:13,158-162` |
| Telemetry batching/delegation | PostHog SDK chosen explicitly for batching + retry; session-start event deduplicated via sessionStorage | `src/services/telemetry.ts:5,761-781` |
| Global data-fetch cache | Single TanStack `QueryClient` with QueryCache/MutationCache error routing; dev-only inspection handle | `src/query-client-config.ts:31-81,86-105` |
| Shared cache timing policy | `CONFIG_CACHE_OPTIONS` = staleTime 5 min / gcTime 15 min reused by config-class queries | `src/hooks/query/query-keys.ts:68-71`; consumed at `src/hooks/query/use-config.ts:22` |
| Settings response cache | Module-scope cache holding redacted+encrypted settings with 5-min TTL, cleared on every save/MCP mutation and via public `invalidateCache()` | `src/api/settings-service/settings-service.api.ts:158-181,462-470,491-512,552,569,586,599,697,706-708` |
| Cross-layer invalidation discipline | Every LLM-profile/agent-profile mutation hook explicitly calls `SettingsService.invalidateCache()` after saves/switches | `src/hooks/mutation/use-save-llm-profile.ts:23-25`; `src/hooks/mutation/use-activate-llm-profile.ts:15-17`; `src/hooks/mutation/use-switch-llm-profile.ts:94-97` |
| `/server_info` module cache | Host-keyed singleton cache with explicit `clearCachedAgentServerInfo()` on failure/version rejection; version getters read the cache | `src/api/agent-server-compatibility.ts:31-32,135-147,235-241,293-318,397-398` |
| Automation SDK version cache | In-memory Map cache: 1-h TTL, negative caching of failures, in-flight request coalescing (`sdkVersionRequests` map), backend/org-scoped keys, test reset hook | `src/hooks/query/use-automation-sdk-version.ts:8,16-17,36-41,43-69,74-80` |
| Expensive probe caching | ACP auth-status probe (spawns a subprocess on the agent-server) cached with `staleTime: Infinity` + 15-min gcTime, keyed by backend+provider, no refetch on focus/mount | `src/hooks/query/use-acp-auth-status.ts:56-89` |
| Conversation history hybrid reuse | REST tail page cached 30-min gcTime, `refetchOnMount: 'always'` deliberately batches missed events into one page instead of socket-replaying them one-by-one | `src/hooks/query/use-conversation-history.ts:73-97` |
| Static asset HTTP caching | Hashed assets served immutable for 1 year; HTML/no-hash responses `no-cache` | `scripts/static-server.mjs:532-535,436`; `vite.config.ts:179` |

## Answers to Dimension Questions

### 1. Are model responses cached?

**Not in this source — no evidence of any model-response cache.** All inference happens in the separate agent-server backend; the frontend's only sanctioned path to it is typed clients (`AGENTS.md`, API Access Rules Rule 1). A full-text search of `src/` found `/chat/completions` references only in the mock-LLM test server (`tests/e2e/mock-llm/scripts/mock-llm-server.py:156`), which is a scripted test fake, not a production cache. There is no request-signature → response memoization, no disk cache, and no replay layer for completions. The nearest related behaviors are cost-side: budget/cost accounting surfaces (`src/components/features/conversation/usage-panel/usage-panel.tsx:64-82`) and context compaction (`CompactContextButton`, `usage-panel.tsx:53-59`), which reduce spend but do not reuse prior responses.

### 2. Are embeddings reused?

**No evidence found.** Searches across `src/`, `docs/`, and `specs/` for `embedding`, `vector`, `retrieval`, `semantic search`, and `RAG` returned no embedding-related code — the app has no embedding pipeline at all (the only "semantic" hits are CSS design tokens and PATCH-merge semantics). Skills are loaded statically at build time via `SKILLS_CATALOG` from `@openhands/extensions` (`AGENTS.md`, General section), which avoids runtime fetch but is catalog distribution, not embedding reuse.

### 3. Is retrieval cached?

**There is no retrieval subsystem to cache.** No RAG/vector store exists in this frontend. The closest functional analogues are ordinary data caches over server-provided resources: the skills service merging build-time public skills with user/project skills from the agent-server (`AGENTS.md`, General section), and React Query caches over git/file/conversation APIs (e.g., `CONFIG_CACHE_OPTIONS` consumers at `src/hooks/query/use-agent-profiles.ts:27-30`). These prevent repeated network fetches but involve no relevance-ranked retrieval.

### 4. Are model calls batched efficiently?

**Model calls cannot be batched here (they happen server-side), but adjacent I/O is batched deliberately:**

- **Streaming output**: `StreamingDeltaBatcher` (`src/utils/streaming-delta-batcher.ts:30-75`) coalesces adjacent token deltas into at most one store commit + render per animation frame, with separate batchers for main/planning streams (`src/contexts/conversation-websocket-context.tsx:162-180`). Tests prove frame-bounded commits and byte-exact reconstruction under 5000 faster-than-frame deltas (`__tests__/utils/streaming-delta-batcher.test.ts:145-172`). Note this batches *rendering*, not network requests.
- **Reads**: `batchGetAppConversations(ids)` collapses N conversation lookups into one REST call locally or through the cloud proxy (`src/api/conversation-service/agent-server-conversation-service.api.ts:599-615`), used by sub-conversation queries (`src/hooks/query/use-sub-conversations.ts:39`) and even single-id resolution (`src/hooks/query/use-user-conversation.ts:55-57`).
- **Writes**: file uploads run in fixed waves of 5 (`src/api/conversation-file-upload.api.ts:159-162`).
- **Telemetry**: event delivery batching/retry is delegated to the PostHog SDK (`src/services/telemetry.ts:5`).

## Architectural Decisions

1. **Delegate LLM-cost optimization to the backend/provider boundary.** The frontend treats the agent-server as the sole inference gateway (`AGENTS.md`, API Access Rules) and limits itself to observing cache economics (`cache_read_tokens`/`cache_write_tokens` end-to-end: type `src/types/agent-server/core/events/conversation-state-event.ts:14-15` → normalization `src/api/conversation-service/agent-server-conversation-service.api.ts:123-124` → accumulation `src/utils/conversation-metrics.ts:49-54` → display `src/components/features/conversation/metrics-modal/usage-section.tsx:24-37`). This keeps a single place (server) responsible for prompt-cache construction, at the cost of the frontend having no lever besides the schema-exposed `prompt_cache_retention` knob.

2. **Render schema-driven settings generically so backend cache controls flow through without frontend code.** Field labels are derived by convention (`toSchemaTranslationKey`, `src/utils/sdk-settings-field-metadata.ts:25-31`), which is how the backend-owned `llm.prompt_cache_retention` setting gets a localized label (`src/i18n/translation.json:3861,3878`) without any bespoke frontend logic.

3. **Batch at the presentation boundary, not the wire, for streaming.** Rather than buffering WebSocket frames at the network layer, the design commits merged deltas once per animation frame into Zustand (`src/contexts/conversation-websocket-context.tsx:166-180`), directly attacking the re-render cost identified in the docstring ("a fast model can't force a store commit + re-render per token", `src/utils/streaming-delta-batcher.ts:31-34`).

4. **Prefer cache patching over invalidation-refetch cycles where feasible.** Conversation mutations patch cached objects in place across single-item and paginated list caches (`patchConversationInCache`, `src/hooks/mutation/conversation-mutation-utils.ts:131-171`), avoiding redundant downloads after status/model changes.

5. **TTL-plus-explicit-invalidation for cross-cutting module caches.** Both the settings cache (5-min TTL, `settings-service.api.ts:175-181`) and the automation SDK version cache (1-h TTL with negative entries, `use-automation-sdk-version.ts:8,60-63`) pair time bounds with forced invalidation hooks invoked from every mutating code path (`use-save-llm-profile.ts:23-25` et al.).

## Notable Patterns

- **In-flight request coalescing (single-flight)** beyond React Query: the automation SDK version cache keeps a `Map<key, Promise>` so concurrent callers share one request (`sdkVersionRequests`, `src/hooks/query/use-automation-sdk-version.ts:47-48,64-68`).
- **Negative caching**: failed SDK-version probes write `null` entries that still satisfy the TTL (`writeCachedSdkVersion(cacheKey, null)` on catch, `src/hooks/query/use-automation-sdk-version.ts:60-63`), preventing hammering a broken backend.
- **Cost-aware cache scoping**: the ACP auth probe is cached for the whole session because each probe spawns a subprocess on the agent-server; `staleTime: Infinity` is bounded by `gcTime: 15 min` after unmount so stale logins eventually re-probe (`src/hooks/query/use-acp-auth-status.ts:74-88`).
- **Hybrid REST-tail + WebSocket-since history loading**: initial history arrives as one batched REST page; the socket connects with `resend_mode='since'` to avoid re-receiving known events; returns to a conversation refetch one tail page instead of replaying potentially long socket backfills (`src/hooks/query/use-conversation-history.ts:21-29,73-97`).
- **Deterministic-testable batching**: the batcher accepts an injected `DeltaFlushScheduler` (`streaming-delta-batcher.ts:4-19`), letting tests tick virtual frames manually (`__tests__/utils/streaming-delta-batcher.test.ts:28-51`).
- **Event-level dedup guards**: UI event handling prevents duplicate messages under out-of-order arrival and React batching (`src/utils/handle-event-for-ui.ts:427`), and telemetry milestones use deterministic `$insert_id`s instead of process-local caches (`AGENTS.md`, Tracking section; session-start sessionStorage dedup at `src/services/telemetry.ts:764-781`).

## Tradeoffs

- **Observability without control (prompt caching)**: users can see cache hit/write tokens and set a retention policy exposed by the backend schema, but the frontend cannot influence prompt prefixing or cache breakpoints — all leverage lives in the out-of-tree agent-server. A user paying non-cached rates due to a backend misconfiguration can diagnose it from the usage panel but not fix it from here.
- **Module-scope caches vs. multi-backend reality**: the settings cache is a plain module singleton (`settings-service.api.ts:162-173`) without backend-scoped keys; correctness relies entirely on the many explicit `invalidateCache()` calls sprinkled through mutation hooks. A new write path that forgets the call serves up to 5 minutes of stale settings — a fragile-by-convention invariant, unlike the backend/org-keyed version cache (`use-automation-sdk-version.ts:74-80`).
- **Frame-based delta batching adds latency ordering constraints**: buffered deltas must be flushed before any durable event lands, otherwise a message could render ahead of its own streamed text (`streaming-delta-batcher.ts:33-34`); the design handles this contractually (callers must `flush()`), verified by tests (`streaming-delta-batcher.test.ts:206-237`).
- **Fixed upload concurrency (5)** trades throughput simplicity for adaptivity — no back-off on slow links or scale-up on fast ones (`conversation-file-upload.api.ts:13`).
- **Cloud/local divergence**: cloud settings bypass the local cache entirely (`getSettings` early-return branch, `settings-service.api.ts:451-459`), trading cache reuse for shape compatibility — repeated cloud settings reads pay full round-trips.

## Failure Modes / Edge Cases

- **Stale-settings window after external changes**: if settings change outside the frontend (another tab, CLI), the local cache serves stale values until TTL expiry unless some hook calls `invalidateCache()` (`settings-service.api.ts:177,702-708`).
- **Backend-switch bleed-through is guarded, not guaranteed everywhere**: newer queries explicitly include backend identity in their cache keys to prevent cross-backend leakage (`use-sub-conversations.ts:29-34`; history key includes host+session key at `use-conversation-history.ts:37-40`), but the settings singleton achieves isolation only through invalidation discipline.
- **Version-cache poisoning risk is contained**: `/server_info` cache is cleared on unavailability, auth failure, unknown version, and unsupported version (`agent-server-compatibility.ts:135-138,297-317,354`), and `getCachedAgentServerInfo({host})` refuses mismatches (`140-147`).
- **Unmount mid-probe**: the automation-version effect uses mounted-guards and shared promises so late resolutions don't setState on unmounted components (`use-automation-sdk-version.ts:98-108`).
- **Dropped deltas on conversation switch**: `reset()` discards buffered deltas without committing (`streaming-delta-batcher.ts:26-27,70-73`), correct for switches but would silently truncate text if invoked mid-stream on the same conversation.
- **Offline/focus flapping**: history refetch on window focus/reconnect is disabled to avoid loops; recovery relies on the socket `since` replay (`use-conversation-history.ts:83-96`) — a failed initial load downgrades the socket to `resend_mode='all'` (`use-conversation-history.ts:86-90`).

## Future Considerations

- Move settings caching behind backend-scoped keys (as already done for SDK versions and sub-conversations) so correctness stops depending on hand-placed invalidation calls.
- Expose prompt-cache effectiveness (hit ratio computed from existing `cache_read_tokens` vs `prompt_tokens`) in the usage panel — the raw inputs already exist at `src/components/features/conversation/metrics-modal/usage-section.tsx:19-42`.
- Adaptive upload concurrency with failure back-off could replace the fixed wave size in `conversation-file-upload.api.ts:159-162`.
- A generic single-flight/TTL cache utility could unify three hand-rolled variants (automation SDK version, settings cache, `/server_info` cache).

## Questions / Gaps

- **Where is prompt caching actually implemented?** Out of scope for this source: it lives in the `OpenHands/software-agent-sdk` agent-server (per the AGENTS.md repo map). This study inspected only the openhands directory, so the server-side mechanism could not be verified here.
- **Embeddings/retrieval**: confirmed absent within this source's boundaries (searched `src/**`, `docs/**`, `specs/**` for embedding/vector/retrieval/RAG terms). Any retrieval caching in the wider system was therefore not assessable.
- **Server-side response caching for repeated identical conversations**: not observable from the frontend; no client behavior depends on it either way.
- **Tool-call result caching** (e.g., repeating identical bash commands): handled by execution semantics, not caching; no evidence of result memoization in the tool/event plumbing reviewed (`src/utils/handle-event-for-ui.ts`).

---

Generated by Dimension 20.02 (`20.02-caching-batching-and-reuse`) against `openhands` (studies/agent-harness-study/sources/openhands).

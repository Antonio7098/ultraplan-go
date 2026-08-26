# Source Analysis: openhands

## 05.04 Retrieval-Augmented Memory

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (OpenHands `agent-canvas` frontend) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19 + React Router 7 + Vite + TanStack Query + Zustand; Electron desktop shell; agent-server access via `@openhands/typescript-client` (`package.json:22-176`) |
| Analyzed | 2026-08-26 |

**Citation convention:** all paths below are relative to the studied source root `studies/agent-harness-study/sources/openhands/`. This repository is explicitly only the frontend of a multi-repo system (`AGENTS.md`, "Repository Map"): the Python SDK/agent-server that would own semantic retrieval lives in a sibling repo and was not inspected (source-isolation rule).

## Summary

Within this source boundary, retrieval-augmented memory in the classic sense (vector stores, embeddings, chunking, rerankers) is **absent**. Verified by case-insensitive search across the tree for `embedding|vector|faiss|pinecone|chroma|qdrant|weaviate|pgvector|lancedb|milvus|rerank|\brag\b`: the only hits are unrelated comments about XSS attack vectors (`src/components/features/mcp-page/install-server-modal.tsx:38`) and link `rel` vectors (`src/components/features/markdown/markdown-renderer.tsx:56`). There are no RAG/vector dependencies in `package.json`.

What exists instead are three non-semantic mechanisms that approximate the dimension:

1. **Structured event-history retrieval** — keyset-paginated REST reads over prior conversation messages/events with timestamp filters (`src/api/event-service/event-service.api.ts:102-181`), consumed by an initial-tail seed hook (`src/hooks/query/use-conversation-history.ts:48-71`) and scroll-up backfill (`src/hooks/use-load-older-events.ts:125-161`). Deterministic log access, no relevance scoring.
2. **Keyword-triggered skill/knowledge injection** — the closest thing to domain-knowledge retrieval. The frontend assembles the candidate knowledge set (build-time bundled public catalog merged with server-loaded user/project skills, filtered by `disabled_skills`, scoped `project | personal | public`), but trigger matching itself is delegated to the server-side SDK via `{type: "keyword", keywords}` payloads (`src/api/agent-server-adapter.ts:697-747, 749-788`).
3. **LLM condensation (context compaction)** — memory *management* rather than retrieval: user-triggered condense endpoint (`src/hooks/mutation/use-condense-conversation.ts:15-31`; `src/api/conversation-service/agent-server-conversation-service.api.ts:723-745`), typed condensation event taxonomy (`src/types/agent-server/core/events/condensation-event.ts:5-52`), and an observable outcome-detection hook that verifies a token drop or times out (`src/hooks/use-await-context-compaction.ts:57-163`). A persistent-notes toggle (`agent_context.load_memory`, storage under `.openhands/memory/`) is stamped server-side (`src/mocks/settings-handlers.ts:349-372`; `src/api/agent-server-adapter.ts:1100-1106`).

Provenance handling exists only for skills: `SkillInfo.source` records where each knowledge document came from (`src/types/settings.ts:72-93`) and drives scope classification (`src/utils/skill-scope.ts:79-106`). There is no citation rendering for injected content anywhere in the chat UI.

## Rating

**3 / 10** — Absent or implicit within this source.

Rationale against the rubric:

- Retrievers: no semantic retriever exists; the only retriever-shaped code is REST pagination over structured logs (`src/api/event-service/event-service.api.ts:102-181`). Not scored up because it has no query-text relevance model.
- Indexing pipeline: absent entirely — no indexer, vector store, embedding generation, or document chunker anywhere (the only "chunk" code is base64 byte-chunking to avoid call-stack limits, `src/hooks/query/use-workspace-file-content.ts:114-127`).
- Embedding config: absent; `search_api_key` is a web-search tool credential, not an embedding key (`src/services/settings.ts:12,24`).
- Reranking: absent.
- Scoping/provenance: present and deliberate for skills (path-marker taxonomy, source field, disabled-skills filter) — this is what keeps the score at 3 rather than 1–2.
- The actual retrieval semantics (keyword trigger matching, condenser summarization, persistent-memory loading) live in the sibling `software-agent-sdk` repo per the documented repo boundary (`AGENTS.md`, Repository Map table); from inside this source they are configuration-only, i.e., implicit.

## Evidence Collected

Every entry cites files relative to the studied source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Retriever (events) | `EventService.searchEvents()` — keyset pagination over prior conversation events: `limit` (default 100), `sort_order=TIMESTAMP_DESC`, `page_id` cursor, `timestamp__gte`/`timestamp__lt` filters; cloud path proxies `/api/v1/conversation/{id}/events/search`, local path uses typed `RemoteEventsList` client | `src/api/event-service/event-service.api.ts:102-181` |
| Retriever consumer (initial tail) | Initial history = latest 50 events (`INITIAL_HISTORY_PAGE_SIZE`), fetched newest-first then reversed; `hasMore` derived from `next_page_id` or full page | `src/hooks/query/use-conversation-history.ts:10, 43-72` |
| Retriever consumer (backfill) | Scroll-up pagination anchored on oldest timestamp (`timestampLt`); exhaustion when short page or missing `next_page_id` | `src/hooks/use-load-older-events.ts:125-161` |
| Other paginators | Bash terminal-event search; conversation listing sorted `UPDATED_AT_DESC` | `src/api/bash-service/bash-service.api.ts:58-77`; `src/api/conversation-service/agent-server-conversation-service.api.ts:747-764` |
| Indexing pipeline | None found — searched whole tree for faiss/pinecone/chroma/qdrant/weaviate/pgvector/lancedb/milvus/langchain/rag: zero matches in source and deps; only false-positive "vector" comments | search over `src/` + `package.json` (hits only at `src/components/features/mcp-page/install-server-modal.tsx:38`, `src/components/features/markdown/markdown-renderer.tsx:56`) |
| Chunking | Only base64 byte-chunking under call-stack limits (`CHUNK = 0x8000`), not document chunking | `src/hooks/query/use-workspace-file-content.ts:114-127` |
| Embedding config | No embedding model/base-url fields; `llm_api_key`/`search_api_key_set` are generation-LLM and web-search credentials | `src/services/settings.ts:10-12,24`; `src/types/settings.ts:127,139` |
| Knowledge corpus assembly | Public skills bundled at build time from `@openhands/extensions` (`SKILLS_CATALOG`, immutable snapshot); user/project skills loaded from agent-server with `load_public:false, load_user:true, load_project:true` | `src/api/skills-service.ts:26-34, 37-64` |
| Trigger shape (delegated retrieval) | Bundled skills serialized into SDK `Skill` JSON with `{type:"keyword", keywords:[...]}` triggers or `trigger:null` (always-active); comment states the agent-server performs trigger matching, activation, and system-prompt injection | `src/api/agent-server-adapter.ts:697-747` |
| Injection config & filtering | `buildAgentContext()` merges bundled skills into `agent_context.skills`, drops `disabled_skills`, sets `load_public_skills:false` | `src/api/agent-server-adapter.ts:749-788` |
| Scope classification | `getSkillScope()` maps skill provenance to `project/personal/public` via path markers incl. legacy `.openhands/microagents/` compat marker | `src/utils/skill-scope.ts:11-19, 25-52, 79-106` |
| Conversation-level scoping | Skill catalog scoped to active conversation's attached workspace | `src/hooks/query/use-conversation-skills.ts:4-13` |
| Provenance field | `SkillInfo.source: string \| null` records origin path or `"public"`; `BundledSkill.source` carries absolute `SKILL.md` path so the server can resolve bundled resources | `src/types/settings.ts:72-93`; `src/api/agent-server-adapter.ts:703-712, 729-734` |
| Condenser defaults & UI | Defaults `enable_default_condenser:true`, `condenser_max_size:240`; settings page renders schema section `condenser`; nav entry | `src/services/settings.ts:18-19, 42-45`; `src/routes/condenser-settings.tsx:3-14`; `src/constants/settings-nav.tsx:32-49` |
| Compaction observability | Outcome detection watches for new `Condensation` events and verifies `per_turn_token` drop; classifies `compacted/no_change/timeout` with 90s timeout; comment notes HTTP ack only means work started | `src/hooks/use-await-context-compaction.ts:6-24, 57-61, 96-110, 150-154` |
| Condensation event taxonomy | `Condensation {forgotten_event_ids, summary?, summary_offset?}`, `CondensationRequestEvent`, `CondensationSummaryEvent {summary}` | `src/types/agent-server/core/events/condensation-event.ts:5-52` |
| Persistent memory toggle | Schema exposes single boolean `agent_context.load_memory` ("Persistent memory"), description documents `.openhands/memory/` note storage and load-at-conversation-start; profile-launch stamping handled server-side | `src/mocks/settings-handlers.ts:349-372`; `src/api/agent-server-adapter.ts:1100-1106`; nav comment `src/constants/settings-nav.tsx:38-49` |
| Keyword filtering (trivial) | Slash-command menu filters items by substring `includes()` on command/name/content — the only client-side text matching | `src/hooks/chat/use-slash-command.ts:100-112` |
| Legacy recall leftover | i18n key `OBSERVATION_MESSAGE$RECALL` has zero referencing code — remnant of removed microagent-recall observation UI | `src/i18n/translation.json:16866` |
| Fake semantic-search fixture | Test-only slash item "Search code semantically." proves no shipped feature behind it | `__tests__/components/features/chat/slash-command-menu.test.tsx:33,107` |

## Answers to Dimension Questions

**1. What can be retrieved?**
Prior conversation events/messages (per-conversation, timestamp-windowed pages: `src/api/event-service/event-service.api.ts:102-181`), terminal/bash events (`src/api/bash-service/bash-service.api.ts:58-77`), conversation metadata listings (`src/api/conversation-service/agent-server-conversation-service.api.ts:747-764`), and knowledge documents in the form of skills (bundled public catalog + server-side user/project skills, `src/api/skills-service.ts:37-64`). Code, traces, and external docs are *not* retrievable through any indexed mechanism in this repo; workspace file access is direct fetch, not retrieval (`src/hooks/query/use-workspace-file-content.ts`).

**2. How is it indexed?**
No index is built. Events are ordered by server timestamps with a keyset cursor (`sort_order=TIMESTAMP_DESC` + `page_id`, `src/api/event-service/event-service.api.ts:128-134, 166-175`). The public skills corpus is a build-time baked catalog — an immutable snapshot updated only by bumping the `@openhands/extensions` dependency and rebuilding (`src/api/skills-service.ts:26-34`). There is no inverted index, embedding index, or vector store.

**3. Are retrieval results scoped correctly?**
Largely yes, with caveats. Event retrieval is strictly scoped per conversation ID and routed per backend (cloud App API vs. runtime sandbox vs. local typed client), with auth mode switching between bearer and session-key (`src/api/event-service/event-service.api.ts:108-175`; docstring 18-38). Skills are scoped three ways: path-marker scope classification (`project/personal/public`, `src/utils/skill-scope.ts:79-106`), load flags sent to the server (`load_user/load_project/load_org`, `src/api/skills-service.ts:50-56`), and conversation-level workspace binding (`src/hooks/query/use-conversation-skills.ts:10-13`). Caveat: scope classification is heuristic regex/path-prefix matching on `/Users/<name>/` and `/home/<name>/` layouts (`skill-scope.ts:35-52`) and would misfile skills in unusual home-directory layouts.

**4. Are sources preserved?**
Partially. Every skill carries a `source` string (origin file path, or `"public"`), which the UI uses for scope grouping (`src/types/settings.ts:77`; `src/utils/skill-scope.ts:83-105`), and bundled skills pass their absolute `SKILL.md` path to the server so it can resolve companion resources (`src/api/agent-server-adapter.ts:729-734`). However, there is no mechanism that attributes injected knowledge back to its source in the agent's context or the chat transcript — no citations, footnotes, or provenance metadata accompany activated skills. For event history, provenance is trivially first-party (the conversation's own log). The orphaned `OBSERVATION_MESSAGE$RECALL` translation key (`src/i18n/translation.json:16866`) suggests an earlier architecture rendered recalled-source observations in the UI; that machinery no longer exists here.

**5. Can stale or low-quality retrieval be detected?**
Mixed. For event history, yes: malformed pages throw instead of silently rendering (`src/hooks/query/use-conversation-history.ts:58-62`; `src/hooks/use-load-older-events.ts:136-140`), and unsupported server-side pagination filters degrade loudly with a console warning plus a deliberate stop rather than infinite retry loops (`src/api/event-service/event-service.api.ts:143-163`). For condensation, quality is verified behaviorally: compaction counts as succeeded only if a `Condensation` event lands *and* measured tokens drop; otherwise `no_change` or `timeout` failure outcomes are surfaced (`src/hooks/use-await-context-compaction.ts:11-24, 150-154`). For the skills corpus, no staleness or quality signal exists: the public catalog can silently drift out of date until rebuild, and a failed server skills fetch is swallowed by an empty catch, silently degrading to public-only knowledge (`src/api/skills-service.ts:58-63`).

## Architectural Decisions

1. **Delegate retrieval semantics to the backend.** The frontend never decides *what* knowledge enters the prompt; it ships candidate skills with keyword triggers and lets the agent-server SDK match triggers, activate skills, inject them into the system prompt, run the condenser, and load `.openhands/memory/` notes (`src/api/agent-server-adapter.ts:714-721`; `src/mocks/settings-handlers.ts:349-352`). This follows the documented multi-repo boundary where agent/tool/server logic belongs to `software-agent-sdk` (`AGENTS.md`, Repository Map).
2. **REST-first lazy history with WebSocket `since` replay instead of full replay.** Only the latest ~50 events are retrieved up front; older history loads on scroll, and the socket connects with `resend_mode='since'` anchored at the preloaded tail to avoid re-receiving history (`src/hooks/query/use-conversation-history.ts:21-29` and cache-policy rationale at 73-96).
3. **Build-time bundling of public knowledge.** Replacing server-side extensions-repo cloning with a bundled catalog removes clone latency and makes `load_public_skills:false` authoritative — the frontend is the sole source of public skills (`src/api/agent-server-adapter.ts:771-781`; `src/api/skills-service.ts:42-45`).
4. **Compaction as an observable operation, not fire-and-forget.** The HTTP ack is treated as "work started"; success requires an observed `Condensation` event plus token-drop verification with explicit timeout semantics (`src/hooks/use-await-context-compaction.ts:57-61, 150-154`).
5. **Persistent memory reduced to one boolean.** Rather than exposing the raw `AgentContext` model, only `load_memory` is surfaced, deliberately kept off the client-owned enrichment boundary so profile-launched agents inherit it via server stamping (`src/api/agent-server-adapter.ts:1100-1106`; `src/mocks/settings-handlers.ts:349-350`).

## Notable Patterns

- **Graceful degradation ladders**: the cloud events path attempts full filter params, then falls back to limit-only on server errors, with a `strictPagination` escape hatch for callers that must not lose filters (`src/api/event-service/event-service.api.ts:120-163`).
- **Legacy-compat markers as data**: the skills scope module keeps the pre-rename `.openhands/microagents/` marker precisely because dropping it would misfile legacy skills as public (`src/utils/skill-scope.ts:11-14`) — a small, well-commented example of migration-aware scoping.
- **Schema-driven settings surfaces**: both condenser and memory UIs render from server-provided field schemas (`SdkSectionPage` with `sectionKeys:["condenser"]`, `src/routes/condenser-settings.tsx:5-10`; curated `fields_opt_in` section in `src/mocks/settings-handlers.ts:349-372`), keeping frontend and SDK contract-aligned.
- **Ref-guarded pagination hooks**: dual ref/state guards prevent duplicate page fetches when scroll/wheel effects fire in the same tick (documented convention; guards visible at `src/hooks/use-load-older-events.ts:106-123`).

## Tradeoffs

- **Determinism over relevance**: timestamp/keyset retrieval and keyword triggers are cheap, debuggable, and dependency-free, but cannot surface relevant context from deep history beyond the tail page; long conversations depend entirely on the (out-of-repo) condenser's summarization quality. There is no client-side notion of "find similar past events."
- **Freshness vs. latency for public knowledge**: the build-time catalog eliminates clone latency but freezes knowledge until the next dependency bump + rebuild (`src/api/skills-service.ts:26-34`).
- **Thin-client simplicity vs. opacity**: because condensation and memory loading happen server-side, this repo offers users a toggle and a token meter but no inspection of *what* was summarized away, forgotten (`forgotten_event_ids` exist on the wire type, `src/types/agent-server/core/events/condensation-event.ts:16`, but no UI consumes them for display), or remembered.
- **Heuristic scoping vs. correctness**: path-based scope inference avoids needing server-side scope metadata but bakes OS home-layout assumptions into the client (`src/utils/skill-scope.ts:35-52`).

## Failure Modes / Edge Cases

- **Cloud pagination-filter gap**: if the deployed cloud backend lacks timestamp-filter support (tracked upstream as OpenHands#14399), older-events backfill stops after the initial tail with only a console warning; users see truncated history without a UI error (`src/api/event-service/event-service.api.ts:115-119, 149-163`).
- **Silent knowledge degradation**: if the agent-server skills endpoint is unreachable or unsupported, user/project skills silently vanish from the assembled corpus (empty catch → public-only fallback, `src/api/skills-service.ts:58-63`).
- **Malformed history anchors**: an oldest event lacking a timestamp flips `hasMore` permanently false rather than erroring — safe for brand-new conversations but could mask corruption mid-history (`src/hooks/use-load-older-events.ts:113-120`).
- **Compaction ambiguity**: `no_change` (event landed, no token drop) vs. `timeout` (no event at all) are distinguished, but both leave the user with unshrunk context after up to 90s (`src/hooks/use-await-context-compaction.ts:17-23, 150-154`).
- **Profile-path enrichment loss**: profile-launched agents rely on the server restoring the exec toolset and public skills themselves; the known upstream gap (software-agent-sdk#3967) is acknowledged in-code (`src/api/agent-server-adapter.ts:1089-1098`) — a concrete case where knowledge-injection continuity across launch paths is fragile.
- **Stale bundled knowledge**: nothing detects that the baked public-skill snapshot predates available fixes/features.

## Future Considerations

- If any semantic retrieval ever moves client-side, the groundwork is absent today: no embedding provider abstraction, no vector store interface, no chunking utilities — introducing RAG here would mean new infrastructure, not extension of existing code.
- Surfacing provenance for activated skills (which skill fired, why, and its `source`) in the chat transcript would directly answer the dimension's trust question ("knows where it came from") using data already present on `SkillInfo` (`src/types/settings.ts:77`).
- Rendering `forgotten_event_ids` / summaries from `Condensation` events would give users visibility into what compaction discarded (`src/types/agent-server/core/events/condensation-event.ts:16-26`).
- A health signal replacing the silent skills-fetch catch would make knowledge-corpus degradation observable (`src/api/skills-service.ts:58-63`).

## Questions / Gaps

- **Where does keyword trigger matching actually rank/select?** Out of this source: the matching lives in `software-agent-sdk` (stated at `src/api/agent-server-adapter.ts:714-721` and `AGENTS.md`); no evidence of its precision/recall behavior exists here. Searched `src/` for trigger-matching logic — only payload construction found.
- **Is there any deduplication between bundled skills and server-reported skills with identical names?** `buildAgentContext` concatenates existing + bundled lists and filters only by `disabled_skills` (`src/api/agent-server-adapter.ts:758-767`); no name-based dedup was found. Potential duplicate-injection edge case, unverifiable from this boundary.
- **What did `OBSERVATION_MESSAGE$RECALL` originally render?** Only the orphaned i18n entry remains (`src/i18n/translation.json:16866`); the consuming component was removed before this study's snapshot.
- **No clear evidence found** for any stale-content detection on the bundled skills catalog, or for any citation/attribution rendering of retrieved knowledge in the chat UI; searches across `src/components/` for citation/provenance patterns returned only automation-manifest and npm supply-chain uses of the word "provenance" (e.g., `src/manifests/types.ts:102,157`).

---

Generated by `05.04-retrieval-augmented-memory` against `openhands`.

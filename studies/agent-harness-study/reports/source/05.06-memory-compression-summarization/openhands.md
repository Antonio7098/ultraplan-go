# Source Analysis: openhands

## Dimension 05.06 — Memory Compression and Summarization

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React (agent-canvas frontend; Vite, Zustand, TanStack Query, react-router) |
| Analyzed | 2026-08-25 |

> Citation note: all `path:line` references below are relative to the source root `studies/agent-harness-study/sources/openhands/`.

## Summary

This repository is only the OpenHands **frontend** (agent-canvas); per its own architecture notes (`AGENTS.md`, repo map section), conversation memory/condensation mechanics live in the sibling Python SDK (`OpenHands/software-agent-sdk`), which is outside this study boundary. Within this repo, memory compression appears as four frontend concerns:

1. **Configuration plumbing** for a default LLM condenser (`enable_default_condenser`, `condenser_max_size`), stored in nested `agent_settings.condenser` and edited through a schema-driven settings page at `/settings/condenser`.
2. **A manual compaction trigger** ("Compact context") that POSTs `/api/conversations/{id}/condense` from two surfaces (Usage panel CTA and composer context-window popover) through the shared `useCompactContextAction` hook.
3. **Observation machinery**: a typed `Condensation` event (`forgotten_event_ids`, optional `summary`, `summary_offset`) consumed solely by `useAwaitContextCompaction`, which correlates the event with a drop in live `per_turn_token` metrics to classify the outcome as `compacted` / `no_change` / `timeout`.
4. **Token-budget visualization**: a context-fill meter (warning >70%, danger >90%) computed from `per_turn_token` vs `context_window`, fed by WebSocket stats combined across LLM usage ids (including a dedicated `"condenser"` usage id).

Notably, the summary *content* itself is invisible in this frontend: condensation events are deliberately filtered out of the chat render path, the `summary`/`forgotten_event_ids` payload fields have zero consumers, and no drift detection or summary-quality evaluation exists. Compression fidelity ("does it preserve decisions, facts, uncertainty?") is neither surfaced nor verifiable from this codebase — only token savings are measured and toasted to the user.

## Rating

**Score: 5 / 10**

Rationale against the rubric: this sits squarely in the "present but inconsistent" band (4–6), leaning high. On the positive side: the trigger/config interface is fully typed end-to-end (settings types `src/types/settings.ts:133-134`, flat↔nested API mapping `src/api/settings-service/settings-service.api.ts:395-400`, cloud shape mapping `src/api/cloud/settings-service.api.ts:33-34`), the manual trigger has explicit operational safeguards (disabled while the agent is RUNNING/LOADING, race-safe pre-request event baselines, 90 s timeout, clamped token math) with unit/integration tests (`src/hooks/use-await-context-compaction.test.ts:58,92,119,141`). But the substance of the dimension — what the summary covers, whether decisions/facts/uncertainty survive — cannot be inspected or evaluated here: summary text is never rendered (`shouldRenderEvent` returns `false` for unhandled kinds, `src/components/conversation-events/chat/event-content-helpers/should-render-event.ts:143-144`), coverage fields (`forgotten_event_ids`) are parsed but unused, and the only success criterion is a token-count drop. The actual summary prompt is server-side; "No evidence found" within this source boundary despite searching `src/` for `condens|summariz|compress`.

## Evidence Collected

Every entry includes a file path with line numbers, relative to `studies/agent-harness-study/sources/openhands/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Condenser settings keys | `enable_default_condenser: boolean` and `condenser_max_size: number \| null` on the flat `Settings` type | `src/types/settings.ts:133-134` |
| Condenser defaults | Default ON with `max_size: 240`; nested default `agent_settings.condenser = { enabled: true, max_size: 240 }` | `src/services/settings.ts:18-19,42-45` |
| Settings page | `/settings/condenser` route renders `SdkSectionPage` for `agent_settings` section `condenser` | `src/routes/condenser-settings.tsx:3-12`; nav entry `src/constants/settings-nav.tsx:34` |
| Schema-driven field definitions | Mocked agent-schema section `condenser` with fields `condenser.enable_default_condenser` (critical, boolean) and `condenser.condenser_max_size` (major, integer) | `src/mocks/settings-handlers.ts:312-346` |
| Save path (diff-based) | Dirty-only payloads emitted as `agent_settings_diff` via `PAYLOAD_DIFF_KEY` and merged in `handleSave` | `src/components/features/settings/sdk-settings/sdk-section-page.tsx:94-97,536-556` |
| Flat ↔ nested settings sync | Read side hoists `condenser.enabled`→`enable_default_condenser`, `condenser.max_size`→`condenser_max_size` | `src/api/settings-service/settings-service.api.ts:395-400` |
| Cloud backend shape | Cloud API nests the same values back under `agent.condenser.{enabled,max_size}` | `src/api/cloud/settings-service.api.ts:33-34,84-91` |
| Client-side default resolution | `use-settings.ts` falls back to `DEFAULT_SETTINGS.enable_default_condenser` / `.condenser_max_size` | `src/hooks/query/use-settings.ts:98-103` |
| Manual trigger endpoint | `POST /api/conversations/{id}/condense`, routed via cloud proxy or direct typed `ConversationClient` | `src/api/conversation-service/agent-server-conversation-service.api.ts:717-745` |
| Trigger mutation | `useCondenseConversation` mutation invalidates `["conversation-metrics", id]` on settle | `src/hooks/mutation/use-condense-conversation.ts:15-31` |
| Shared compact action | `useCompactContextAction`: busy/disabled guards (agent RUNNING/LOADING), pre-request baseline snapshot of event ids, toasts for started/completed/no-change/failed | `src/hooks/use-compact-context-action.ts:26-111` |
| Compaction awaiter | Outcome taxonomy `compacted`/`no_change`/`timeout`; requires Condensation event **and** `per_turn_token` drop; 2.5 s metrics-settle window; 90 s timeout; `savedToken` clamped ≥ 0 | `src/hooks/use-await-context-compaction.ts:6-37,96-110,150-154` |
| Condensation wire contract | `CondensationEvent { kind: "Condensation"; forgotten_event_ids; summary?; summary_offset? }`; also `CondensationRequestEvent` and `CondensationSummaryEvent { summary }` | `src/types/agent-server/core/events/condensation-event.ts:5-52` |
| Type guard | `isCondensationEvent` matches only `kind === "Condensation"` — payload fields ignored | `src/types/agent-server/type-guards.ts:286-289` |
| Event union membership | All three condensation kinds are members of `OpenHandsEvent` | `src/types/agent-server/core/openhands-event.ts:11-12,39-40` |
| UI suppression of summaries | `shouldRenderEvent` returns `false` for every kind not explicitly listed (incl. Condensation*) | `src/components/conversation-events/chat/event-content-helpers/should-render-event.ts:139-145` |
| Token budget inputs | Metrics store holds `per_turn_token` and `context_window` | `src/stores/metrics-store.ts:3-14` |
| WS metrics combination | Stats events combined across `usage_to_metrics` keyed by arbitrary ids ("default", `"condenser"`, "profile:<name>:<uuid>"); `per_turn_token` taken as max across usages | `src/contexts/conversation-websocket-context.tsx:216-276` |
| REST metrics combination | `combineUsageMetrics` mirrors the SDK's `get_combined_metrics`; `per_turn_token` max-combined | `src/utils/conversation-metrics.ts:12-73` |
| Live-over-REST preference | `useLiveConversationMetrics` prefers the live WS store, falls back to a 30 s REST poll | `src/hooks/use-live-conversation-metrics.ts:22-85` |
| Budget thresholds & display | Warning >70%, danger >90%; percentage capped at 100; unknown-window guard (≤0 → raw count only) | `src/components/features/conversation/usage-panel/context-meter.tsx:6-21,41-43`; `src/utils/format-token-count.ts:22-31` |
| Compact CTA surfaces | Usage panel button (promoted to primary CTA above warning threshold) and composer context-window popover entry point | `src/components/features/conversation/usage-panel/compact-context-button.tsx:24-79`; `src/components/features/chat/components/context-window-meter.tsx:47-49,134-161` |
| Panel wiring | Meter + CTA rendered together in Usage tab | `src/components/features/conversation/usage-panel/usage-panel.tsx:46-61` |
| User-facing copy | "Summarizes older messages to free context space while keeping recent details."; failure/no-change/start strings | `src/i18n/translation.json:37997-38065,38133` |
| Condenser trigger copy | Tooltip: "After this many events, the condenser will summarize history. Minimum 20." (vs. schema description saying "tokens") | `src/i18n/translation.json:2110-2112`; contrast `src/mocks/settings-handlers.ts:332-333` |
| Tests — awaiter | Tokens-freed happy path; settle-window `no_change`; timeout; early-landing condensation (baseline captured pre-request) | `src/hooks/use-await-context-compaction.test.ts:58,92,119,141` |
| Tests — integration | Toast counts when metrics drop; error toast on timeout | `src/hooks/use-compact-context-action.test.tsx:78,103` |
| Tests — CTA rendering | Info-control layout; high-fill warning above threshold | `src/components/features/conversation/usage-panel/compact-context-button.test.tsx:57,78` |
| ACP carve-out | Condenser settings documented inert for `acp` agent kind | `src/types/settings.ts:100-108` |

## Answers to Dimension Questions

1. **When does summarization happen?**
   Two paths. (a) Automatic, server-side: whenever `agent_settings.condenser.enabled` is true, the backend condenser summarizes long histories per `condenser_max_size`; the frontend only persists this config (`src/services/settings.ts:42-45`, saved as `agent_settings_diff` from `src/components/features/settings/sdk-settings/sdk-section-page.tsx:536-556`). The exact threshold semantics differ between sources inside the repo — the settings tooltip says "after this many events … Minimum 20" (`src/i18n/translation.json:2111`) while the schema description says "maximum number of tokens the condenser keeps" (`src/mocks/settings-handlers.ts:332-333`); the authoritative definition is server-side and out of boundary. (b) Manual: user-triggered `POST /api/conversations/{id}/condense` from the Usage panel or composer popover (`src/hooks/use-compact-context-action.ts:80-111`, `src/api/conversation-service/agent-server-conversation-service.api.ts:723-745`).

2. **What evidence does the summary cover?**
   The wire contract declares coverage explicitly: `forgotten_event_ids` lists which events were removed from the LLM View, with an optional `summary` and `summary_offset` marking where the summary is inserted into the resulting view (`src/types/agent-server/core/events/condensation-event.ts:13-26`). However, the frontend reads none of these fields — the only consumer matches on `kind` alone (`src/types/agent-server/type-guards.ts:286-289`). Whether the preserved summary captures decisions, facts, or uncertainty is unverifiable here: no summary prompt exists in this repo (No evidence found; searched `src/` for `condens|summariz|compress` — the only summarization prompt-like artifact is user-facing marketing copy at `src/i18n/translation.json:37997-37999`).

3. **Can summary drift be detected?**
   No. Drift/content-fidelity checking does not exist in the frontend. The nearest proxy is outcome classification in `useAwaitContextCompaction`: a condensation counts as effective only if live `per_turn_token` drops below the pre-request snapshot within a 2.5 s settle window after the Condensation event (`src/hooks/use-await-context-compaction.ts:96-109`); otherwise `no_change`, and `timeout` after 90 s (`src/hooks/use-await-context-compaction.ts:150-154`). This measures freed tokens, never whether the summary preserved the right information.

4. **Is raw history retained?**
   Yes, from the UI's perspective. Condensation shrinks what the *LLM* sees (per the event-type docstring: "removed from the View given to the LLM", `src/types/agent-server/core/events/condensation-event.ts:13-16`); the user-facing transcript is unaffected — condensation events are filtered out of chat rendering (`src/components/conversation-events/chat/event-content-helpers/should-render-event.ts:143-145`), while the full raw event stream remains loaded via REST-first pagination (`AGENTS.md` conversation-history section; store seeding in `src/contexts/conversation-websocket-context.tsx:278-324`). So raw history supplements rather than disappears; replacement applies only server-side.

5. **Can summaries be regenerated?**
   Partially. Re-running compaction is always available via the manual `/condense` endpoint (the button re-enables whenever the agent is idle, `src/hooks/use-compact-context-action.ts:35-39`), and the backend may produce a new summary. But there is no UI to *inspect* a specific historical summary, no targeted regeneration of a single summary record, and no API surface in this repo addressing prior condensations. `CondensationRequestEvent` (`src/types/agent-server/core/events/condensation-event.ts:30-37`) exists in the union but has no frontend producer — requests go through REST instead.

## Architectural Decisions

- **Thin-client split**: all summarization intelligence (prompts, LLM calls, view construction) is delegated to the agent-server SDK; the frontend owns config, triggering, and observation. Enforced structurally — API access must go through `@openhands/typescript-client` (CI guard test referenced in `AGENTS.md` API Access Rules; condense call implemented at `src/api/conversation-service/agent-server-conversation-service.api.ts:742-744`).
- **Schema-driven settings**: the condenser page renders whatever the backend advertises for the `condenser` section rather than hardcoding a form (`src/routes/condenser-settings.tsx:5-10`, generic engine at `src/components/features/settings/sdk-settings/sdk-section-page.tsx:180+`), saving dirty-only diffs (`agent_settings_diff`) so unrelated agent settings are untouched (`sdk-section-page.tsx:94-97,547-555`).
- **Ack ≠ done**: the team explicitly models that the HTTP condense response only means work started; completion is inferred from the event stream plus metric movement (`src/hooks/use-await-context-compaction.ts:57-60`), with the baseline captured *before* the request fires to avoid missing fast condensations (`src/hooks/use-compact-context-action.ts:85-88`).
- **Metrics keyed by usage id**: token/cost accounting treats the condenser as its own LLM usage ("condenser" appears as a named example usage id) and combines across ids with additive costs and max-token semantics (`src/contexts/conversation-websocket-context.tsx:224-262`, `src/utils/conversation-metrics.ts:28-66`).
- **UI silence about compression**: summaries and forgotten-event lists are intentionally kept out of the chat stream; users learn about compaction only through toasts and the context meter.

## Notable Patterns

- **Race-aware event baselining**: `baselineEventIdsRef` snapshots known event ids pre-POST; the awaiter scans only newer events for `kind === "Condensation"` and even handles a condensation that landed before the post-ack effect mounted (`src/hooks/use-compact-context-action.ts:88`, `src/hooks/use-await-context-compaction.ts:112-130`, regression test `src/hooks/use-await-context-compaction.test.ts:141`).
- **Three-outcome taxonomy** (`compacted` / `no_change` / `timeout`) with distinct user messaging, including an honest failure distinction: no Condensation event in time is a failure, not a no-op (`src/hooks/use-await-context-compaction.ts:150-153`).
- **Progressive urgency UI**: the compact control escalates from secondary button to primary CTA with an amber warning once context fill passes 70% (`src/components/features/conversation/usage-panel/compact-context-button.tsx:31,38-47`), mirroring the meter tones at 70%/90% (`src/components/features/conversation/usage-panel/context-meter.tsx:6-21`).
- **Dual-surface single-hook reuse**: one `useCompactContextAction` powers both the Usage panel and the composer popover, guaranteeing identical guards/toasts (`src/components/features/chat/components/context-window-meter.tsx:47-49`).
- **Graceful unknown-window handling**: when a model reports no context window, the meter degrades to raw token counts instead of dividing by zero (`src/components/features/conversation/usage-panel/context-meter.tsx:41-43`, `src/utils/format-token-count.ts:26-28`).

## Tradeoffs

- **Observability gap by design**: hiding summaries keeps the chat clean but means users (and reviewers) cannot audit what was forgotten or what the summary claims — a deliberate UX tradeoff that sacrifices transparency (`should-render-event.ts:143-145` vs. rich unused payload at `condensation-event.ts:16-26`).
- **Token-proxy verification**: measuring success purely by `per_turn_token` reduction is cheap and robust, but a lossy summary that saves many tokens scores identically to a faithful one; conversely a faithful no-op compaction reports `no_change` after the settle window (`use-await-context-compaction.ts:105-110`).
- **Client-side defaults duplication**: `condenser_max_size: 240` default is hardcoded in the frontend (`src/services/settings.ts:19`) while the backend schema advertises `default: null` (`src/mocks/settings-handlers.ts:339`); whichever wins depends on save order, creating potential config skew.
- **Manual trigger concurrency guard is coarse**: disabling compaction during RUNNING/LOADING avoids server races (`use-compact-context-action.ts:35-39`) but blocks users from reclaiming context mid-long-run — precisely when they may need it most.

## Failure Modes / Edge Cases

- **Timeout (90 s)**: no Condensation event arrives → error toast, metrics invalidated, state reset (`use-await-context-compaction.ts:150-154`, `use-compact-context-action.ts:52-54`).
- **HTTP failure**: axios error message surfaced verbatim with localized fallback (`use-compact-context-action.ts:101-108`).
- **Fast condensation race**: POST ack returning after the server already emitted the Condensation event — mitigated by pre-request baseline and covered by a dedicated test (`use-await-context-compaction.test.ts:141-152`).
- **Duplicate/replayed WS events**: dedup by id protects side-effect-free storage, but the awaiter also guards via `knownEventIds` membership (`src/contexts/conversation-websocket-context.tsx:556-568`, `use-await-context-compaction.ts:116-119`).
- **Negative savings impossible**: `savedToken` floored at 0, tested (`use-await-context-compaction.ts:34`, test at `use-await-context-compaction.test.ts:29-31`).
- **Unmeasurable improvement**: condensation lands but metrics don't drop within 2.5 s → reported as `no_change` success toast, which could mask a failed compaction whose metrics lag (`use-await-context-compaction.ts:105-110`).
- **ACP agents**: condenser config is inert for external ACP agents (`src/types/settings.ts:103-106`); the compact button remains driven by the same metrics path, so behavior depends on the ACP host reporting `per_turn_token`.

## Future Considerations

- Surface condensation outcomes in-band: render a collapsed "N earlier events summarized" card using `forgotten_event_ids.count` and expose the `summary` text on demand (`condensation-event.ts:13-26` already carries everything needed).
- Add drift/fidelity signals: e.g., let users flag "the agent lost context after compacting", feeding evaluation of server-side summaries — currently impossible since nothing downstream distinguishes summary quality.
- Reconcile `condenser_max_size` semantics (events vs tokens) between the tooltip copy, schema descriptions, and the backend definition; today three artifacts disagree (`src/i18n/translation.json:2111` vs `src/mocks/settings-handlers.ts:332-333`).
- Consider auto-inviting compaction (or auto-triggering it behind confirmation) when the danger threshold (>90%) is crossed instead of only recoloring the meter (`context-meter.tsx:8-9`).
- Emit or consume `CondensationRequestEvent` for optimistic UI, or remove dead union members to reduce contract ambiguity (`condensation-event.ts:29-37`).

## Questions / Gaps

- **Summary prompt contents**: not in this repo (server-side SDK). Searched `src/`, `docs/`, `specs/`, `README.md` — no evidence found beyond behavioral copy. Whether compression preserves decisions/facts/uncertainty cannot be answered from this source.
- **Automatic trigger threshold truth**: the real trigger condition (token-based vs event-count-based) is defined by the out-of-boundary agent-server; the repo contains contradictory hints (see Tradeoffs).
- **Coverage-range persistence**: whether `forgotten_event_ids` are queryable later (e.g., to reconstruct which spans of history each summary covers) is unknowable here; the frontend discards them.
- **Refresh logic**: no evidence of scheduled re-summarization or summary TTL handling on the client; if the server refreshes summaries, the frontend would only observe it indirectly via new Condensation events and metric shifts.
- **Multi-summary chains**: no handling for repeated condensations accumulating multiple summaries (`summary_offset` implies insertion position, but no client logic composes chained views).

---

Generated by `05.06-memory-compression-and-summarization` against `openhands`.

# Source Analysis: openhands

## 11.02 Token Budgeting and Compression

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (`@openhands/agent-canvas`) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19, React Router 7, Zustand, TanStack Query, Vite; agent-server accessed via `@openhands/typescript-client` |
| Analyzed | 2026-08-25 |

> Citation convention: all `src/...` paths below are relative to the source root `studies/agent-harness-study/sources/openhands/`. Per repo docs (AGENTS.md), this repo is **only the frontend** of a multi-repo system — token counting, condensation execution, and context assembly live in the sibling `software-agent-sdk` agent-server, which is outside this study's isolation boundary. This analysis therefore grades what the frontend itself implements: usage observation, budget configuration surfacing, and user-triggered compression.

## Summary

The frontend implements token budgeting as an **observer + manual relief valve**, not as a client-side enforcement mechanism. There is no client-side token counter at all (searches for `tiktoken`, token estimation, or pre-send measurement return nothing); instead the server reports `per_turn_token` and `context_window` inside `TokenUsage` (`src/types/agent-server/core/events/conversation-state-event.ts:10-20`), which the client merges from WebSocket stats events (`src/contexts/conversation-websocket-context.tsx:216-274`) and a 30-second REST poll (`src/hooks/query/use-conversation-metrics.ts:38-41`). Two UI surfaces render context fill with 70%/90% warning/danger thresholds (`src/components/features/conversation/usage-panel/context-meter.tsx:7-21`, `src/components/features/chat/components/context-window-meter.tsx:37-63`). Compression ("condensation") is executed entirely server-side; the client triggers it manually via `POST /api/conversations/{id}/condense` (`src/api/conversation-service/agent-server-conversation-service.api.ts:717-745`) and then *event-sources* confirmation: it waits for a `Condensation` event plus a measured drop in `per_turn_token`, distinguishing "compacted", "no_change", and "timeout" outcomes with race-condition handling for fast servers (`src/hooks/use-await-context-compaction.ts:62-164`). Condenser budget configuration (`enable_default_condenser`, `condenser_max_size`) is schema-driven in settings and forwarded verbatim in conversation-start payloads (`src/api/agent-server-adapter.ts:891-941`). Display truncation exists only for chat rendering (1000-char cap), never for LLM input.

## Rating

**6 / 10** — Within its self-declared scope (usage observability, config surfacing, manual compaction UX), the model is clear, typed, tested, and defensively engineered: threshold constants are exported and tested (`context-meter.test.tsx:31-45`), the compaction await hook has dedicated tests including the pre-request baseline race (`src/hooks/use-await-context-compaction.test.ts:141-177`), and failure is explicitly distinguished from no-op (`use-await-context-compaction.ts:150-154`). It loses points because: overflow protection is never autonomous (a danger-tone meter is cosmetic if the user ignores it), summarization faithfulness is unverifiable from this repo, the `max()` merge heuristic can mask per-component spikes, and half of the dimension checklist (counting before model calls, truncation strategy for LLM input, priority ranking) has **no client-side evidence by architectural delegation**.

## Evidence Collected

Every entry cites a path relative to `studies/agent-harness-study/sources/openhands/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Token counters (server-reported) | `TokenUsage` carries `prompt_tokens`, `completion_tokens`, cache read/write, `reasoning_tokens`, `context_window`, `per_turn_token`; no client-side counting exists | `src/types/agent-server/core/events/conversation-state-event.ts:10-20` |
| Live metrics ingestion | WS `ConversationStateUpdateEventStats` handler folds `usage_to_metrics` across arbitrary usage ids ("default", "condenser", …): costs summed, `context_window`/`per_turn_token` take `Math.max` | `src/contexts/conversation-websocket-context.tsx:216-274` |
| Metrics store | Zustand store holding cost, `max_budget_per_task`, and per-field usage; dev-only `window.__OH_METRICS_STORE__` debug handle | `src/stores/metrics-store.ts:3-42` |
| REST snapshot fallback | `combineUsageMetrics` documented as "TypeScript equivalent of the get_combined_metrics method from the Python SDK"; same sum/max semantics; 30s `refetchInterval` + `staleTime` | `src/utils/conversation-metrics.ts:8-12,55-62`; `src/hooks/query/use-conversation-metrics.ts:38-41` |
| Live-first selection | `useLiveConversationMetrics` prefers WS store over REST snapshot to avoid up-to-30s display lag; store reset on conversation switch prevents cross-conversation leakage | `src/hooks/use-live-conversation-metrics.ts:6-21,42-84` |
| Budget thresholds | `CONTEXT_FILL_WARNING_PERCENT = 70`, `CONTEXT_FILL_DANGER_PERCENT = 90`; tone classifier neutral/warning/danger | `src/components/features/conversation/usage-panel/context-meter.tsx:7-21` |
| Percentage math guards | Zero-window guard and hard 100% cap; compact formatter (198.5k / 1.0M); unit-tested | `src/utils/format-token-count.ts:2-31`; `__tests__/utils/format-token-count.test.ts:5-32` |
| Unknown-window edge case | Models reporting no window show raw tokens plus "unknown" label instead of misleading "x / 0" | `src/components/features/conversation/usage-panel/context-meter.tsx:41-58` |
| Usage panel assembly | ContextMeter + CompactContextButton + per-token-type breakdown (cache hit/write rows) + budget progress | `src/components/features/conversation/usage-panel/usage-panel.tsx:46-84`; `src/components/features/conversation/metrics-modal/usage-section.tsx:15-44` |
| Composer ring meter | Second surface: popover with percentage, compact action, token summary, deep link to Usage tab | `src/components/features/chat/components/context-window-meter.tsx:55-63,94-181` |
| Manual compaction trigger | `POST /api/conversations/{id}/condense` via typed `ConversationClient` (cloud path proxied); comment notes ack means work *started* | `src/api/conversation-service/agent-server-conversation-service.api.ts:717-745` |
| Compaction lifecycle hook | Waits for new `Condensation` event + lower `per_turn_token`; settle window 2500 ms, timeout 90 s; outcomes `compacted`/`no_change`/`timeout`; saved tokens floored at 0 | `src/hooks/use-await-context-compaction.ts:7-11,26-37,96-110,150-154` |
| Race-safe baseline | Event-id baseline captured **before** the POST fires because the server may emit Condensation before the HTTP response lands | `src/hooks/use-compact-context-action.ts:83-88`; test `src/hooks/use-await-context-compaction.test.ts:141-177` |
| Operational safeguard | Compact disabled while agent RUNNING/LOADING ("the server would race the active step") or while a compaction is pending | `src/components/features/conversation/usage-panel/compact-context-button.tsx:16-23`; `src/hooks/use-compact-context-action.ts:35-39` |
| Promoted CTA on high fill | Above 70% fill the button switches to primary variant and shows an amber warning line | `src/components/features/conversation/usage-panel/compact-context-button.tsx:31,38-47` |
| Post-compaction feedback | Success toast reports freed/before/after via `formatCompactTokenCount`; timeout surfaces error toast; metrics query invalidated | `src/hooks/use-compact-context-action.ts:46-71`; `src/hooks/mutation/use-condense-conversation.ts:25-30` |
| Condensation event contract | `CondensationEvent.forgotten_event_ids` (events removed "from the View given to the LLM"), optional `summary`, `summary_offset`; `CondensationSummaryEvent.summary` | `src/types/agent-server/core/events/condensation-event.ts:5-52` |
| Type guard | `isCondensationEvent` discriminates `kind === "Condensation"` | `src/types/agent-server/type-guards.ts:284-289` |
| Budget config types | Settings expose `enable_default_condenser`, `condenser_max_size`, `max_budget_per_task` | `src/types/settings.ts:133-134,143` |
| Config defaults | Default condenser enabled with `max_size: 240`; mirrored into nested `agent_settings.condenser` sent to server | `src/services/settings.ts:18-19,42-45` |
| Config persistence round-trip | Local service flattens `agent_settings.condenser.{enabled,max_size}` onto top-level keys; cloud service rebuilds nested shape; save uses `agent_settings_diff` deep-merge | `src/api/settings-service/settings-service.api.ts:380-400`; `src/api/cloud/settings-service.api.ts:84-91` |
| Schema-driven condenser page | `/settings/condenser` renders backend-provided section schema (`sectionKeys: ["condenser"]`), including "Maximum number of tokens the condenser keeps after summarization" field | `src/routes/condenser-settings.tsx:3-13`; mock schema mirror `src/mocks/settings-handlers.ts:315-341` |
| Start-payload forwarding | `buildConfiguredOpenHandsAgentSettings` spreads persisted `agent_settings` (including `condenser`) into the conversation-start payload | `src/api/agent-server-adapter.ts:891-941` |
| Dollar-budget parsing | `parseMaxBudgetPerTask` accepts only finite values ≥ $1 | `src/utils/settings-utils.ts:18-28` |
| Budget visualization | `BudgetDisplay` renders progress bar + usage text when `max_budget_per_task > 0`, else "no budget limit" note | `src/components/features/conversation-panel/budget-display.tsx:16-29` |
| Display-only truncation | Chat content helpers cap rendered observation/action/tool text at `MAX_CONTENT_LENGTH = 1000` chars with `...(truncated)` suffix; file-search observations carry server `truncated` flag ("results were truncated to 100 files") | `src/components/conversation-events/chat/event-content-helpers/shared.ts:3`; `.../get-observation-content.ts:77-78,120-121,173,312-317`; `.../get-action-content.ts:79-80`; `src/types/agent-server/core/base/observation.ts:254-287` |
| Server-side condenser corroboration | Repo's own E2E framework documents that the agent-server makes "an internal LLM call (condenser/skill-analysis) before the agent's main loop starts"; mock trajectory must pad for it | `AGENTS.md` (Mock-LLM E2E Test Framework section) |

## Answers to Dimension Questions

### 1. Is token usage measured before calling the model?

**Not in this source.** No token counting or estimation code exists client-side — searches for `tiktoken`, token-count utilities, and draft-size estimation return nothing relevant. Measurement happens in the agent-server, which reports accumulated `TokenUsage` (`src/types/agent-server/core/events/conversation-state-event.ts:10-20`) through two channels: streamed `stats` state events merged live into the Zustand store (`src/contexts/conversation-websocket-context.tsx:216-274`) and a REST snapshot polled every 30 s (`src/hooks/query/use-conversation-metrics.ts:38-41`). The client is therefore strictly post-hoc: it observes what the server already spent and cannot gate a send on projected size. Notably, the composer does not warn about oversized drafts before submission.

### 2. What gets dropped when budget is exceeded?

Nothing is dropped by the frontend. Three mechanisms exist, all delegated or cosmetic:

- **Server-side condenser** configured via `enable_default_condenser` / `condenser_max_size` (default: enabled, 240 — `src/services/settings.ts:18-19`), which the server applies autonomously.
- **Manual compaction**: the user clicks "Compact context", which POSTs `/condense` (`src/api/conversation-service/agent-server-conversation-service.api.ts:718-745`). The resulting `CondensationEvent.forgotten_event_ids` names what was evicted from the LLM view (`src/types/agent-server/core/events/condensation-event.ts:14-16`), but the frontend only counts these events for completion detection — it never renders forgotten items or the summary specially in chat (no non-test consumers of `forgotten_event_ids` exist).
- **Display truncation**: chat rendering slices event content at 1000 characters (`shared.ts:3`); this affects only what humans see, not LLM input.

At >90% fill the only autonomous behavior is visual (red bar/text). Nothing auto-triggers compaction, blocks sends, or prunes history client-side.

### 3. Is summarization faithful?

**Unverifiable from this repo.** Summaries arrive as opaque strings on `CondensationEvent.summary` / `CondensationSummaryEvent.summary` (`condensation-event.ts:19-27,49-52`); the frontend performs zero content validation — its entire fidelity check is the numeric delta `savedToken = max(0, before − after)` (`use-await-context-compaction.ts:26-37`). To its credit, the design does not overclaim: a condensation that produces no measured token drop yields outcome `"no_change"` after a 2.5 s settle window rather than a success toast claiming savings, and absence of any Condensation event within 90 s is reported as `"timeout"` = failure, explicitly documented as "a failure, not a 'nothing to compact' no-op" (`use-await-context-compaction.ts:8-9,105-110,150-154`, tested at `use-await-context-compaction.test.ts:92-139`). Faithfulness of the *text* remains a server-side property outside this boundary.

### 4. Is budget configurable?

Yes, at three levels, all surfaced through the UI:

1. **Compression budget**: `condenser.enable_default_condenser` (boolean) and `condenser.condenser_max_size` (integer tokens kept after summarization) are edited on a schema-driven settings page (`src/routes/condenser-settings.tsx:7`), persisted via `agent_settings_diff` deep-merge (`settings-service.api.ts:380-400`), and forwarded in every conversation-start payload (`agent-server-adapter.ts:891-941`). Configuration is global-per-agent-settings, not per-message; there is no per-model UI knob (the model comes from the LLM profile; window size is whatever the server reports per call).
2. **Dollar budget**: `max_budget_per_task` parsed with a ≥-$1 sanity floor (`src/utils/settings-utils.ts:18-28`) and rendered as a progress bar against accumulated cost (`budget-display.tsx:16-29`). Enforcement is again server-side; the client only visualizes.
3. **Observation thresholds**: the 70/90% warning/danger constants are exported module-level values (`context-meter.tsx:7-9`) but are compile-time, not user-configurable.

## Architectural Decisions

- **Client-as-observer, server-as-enforcer.** All counting, enforcement, and summarization live in the agent-server (sibling SDK repo, per AGENTS.md repo map). The frontend specializes in real-time observability (WS-first with REST fallback, `use-live-conversation-metrics.ts:6-21`) and a manual relief valve. This keeps one source of truth for budgets but means the UI cannot act preemptively.
- **Event-sourced completion semantics.** The HTTP `/condense` acknowledgment is treated as "work started", not "work done"; actual completion requires observing a `Condensation` event in the event stream *plus* a metrics drop (`use-await-context-compaction.ts:57-61`). This correctly models the asynchronous reality and handles the inverted race where the server finishes before the POST returns (baseline ids captured pre-request, `use-compact-context-action.ts:85-88`).
- **Cross-component metric merging heuristic.** Because the server keys metrics by arbitrary LLM usage ids ("default", "condenser", profiles), the client sums additive quantities (cost, prompt/completion/cache tokens) but takes `Math.max` for `context_window` and `per_turn_token` (`conversation-websocket-context.tsx:244-260`), mirroring the SDK's `get_combined_metrics` (`conversation-metrics.ts:9`).
- **Schema-driven settings mirroring.** Condenser fields are not hardcoded forms; they render the backend's `agent_settings_schema` condenser section (`sdk-section-page.tsx:122,301`), so budget knobs track server capabilities without frontend releases.

## Notable Patterns

- **Threshold-toned progressive disclosure**: neutral → amber (>70%) → red (>90%), with the compact CTA simultaneously escalating to primary variant (`context-meter.tsx:13-21`, `compact-context-button.tsx:47`) — the same tone classification is reused by both the panel meter and the composer ring (`context-window-meter.tsx:25-35` imports `getContextFillTone`).
- **Degraded-data honesty**: when a model reports no context window (e.g., some OpenRouter models), the meter shows raw counts and an "unknown" label instead of fabricating percentages (`context-meter.tsx:41-58`, tested at `context-meter.test.tsx:59-68`).
- **Outcome taxonomy over booleans**: compaction results distinguish `compacted` / `no_change` / `timeout` with distinct toasts, avoiding false success reporting (`use-await-context-compaction.ts:11-24`, `use-compact-context-action.ts:52-70`).
- **Dev-only observability escape hatch**: the metrics store exposes `window.__OH_METRICS_STORE__` gated on `import.meta.env.DEV` (`metrics-store.ts:33-40`).

## Tradeoffs

- **Autonomy vs. safety**: disabling compaction while the agent runs (`use-compact-context-action.ts:35-39`) avoids racing the active step but means long-running tasks can sail past 90% with no recourse except pausing; nothing auto-condenses.
- **`max()` merge can under-report**: if a condenser sub-call momentarily holds more context than the main agent, the combined `per_turn_token` reflects the largest component, not the total; conversely summing prompt tokens across components can double-count shared prefixes. Both heuristics are inherited from the SDK contract (`conversation-metrics.ts:55-62`).
- **Token-delta as success proxy**: `savedToken` measures shrinkage, not information retention; a condenser could drop critical context and still report a healthy positive delta.
- **Static thresholds**: 70/90% fit large windows well but fire late on small-window models where a single turn can jump tens of percent.
- **Delegated configurability ceiling**: because knobs are schema-forwarded, the frontend cannot offer budget features (e.g., auto-compact-at-X%) the server hasn't exposed.

## Failure Modes / Edge Cases

- **Fast-server race**: Condensation event may arrive before the POST resolves; handled by capturing the event-id baseline before dispatch and treating pre-effect condensations as new (`use-await-context-compaction.test.ts:141-177`).
- **Silent no-op condensation**: server emits Condensation but metrics don't drop (e.g., cache effects keep `per_turn_token` flat); surfaced honestly as `no_change` rather than fabricated savings (`use-await-context-compaction.ts:105-110`).
- **Stuck compaction**: no event within 90 s ⇒ `timeout` error toast; the arbitrary 90 s constant could mislabel very slow summarizations as failures.
- **Unknown context window**: guarded division and alternate rendering prevent "x / 0" and NaN percentages (`format-token-count.ts:26-30`, `context-meter.tsx:43`).
- **Cross-conversation leakage**: metrics store reset on conversation switch so live figures always belong to the active conversation (`use-live-conversation-metrics.ts:13-17`).
- **Overshoot rendering**: percentages and bar widths capped at 100 even when `per_turn_token > context_window` (tested: `context-meter.test.tsx:53-56`).

## Future Considerations

- Add an opt-in auto-compaction policy at the danger threshold (the plumbing — threshold constants, compaction action, outcome handling — already exists; only the trigger is missing).
- Render `forgotten_event_ids` / `summary` in the chat timeline (e.g., a collapsible "history condensed" marker) so users see what the LLM lost, closing the observability gap the event types already support (`condensation-event.ts:14-27`).
- Pre-send draft-size awareness in the composer (even a rough character-based estimate) would give the "measure before calling" question a partial client-side answer without duplicating server logic.
- Make warning/danger thresholds proportional or configurable for small-window models.

## Questions / Gaps

- **No evidence found** for client-side token counting, per-model budget tables, truncation of LLM-bound input, or priority ranking of context sections. Searched all `src/**` for `token`, `budget`, `truncat*`, `summariz*`, `condens*`, `tiktoken`, `estimate`, `context_window`, `max_input`; the only hits are the observability/config/display paths cited above. These mechanisms belong to the `software-agent-sdk` agent-server (out-of-bounds per source isolation rules), so their maturity cannot be graded here.
- Whether automatic (non-user-triggered) condensation fires mid-task, and how the server chooses eviction candidates, is invisible from this repo; the E2E framework note about a pre-loop condenser LLM call (`AGENTS.md`, Mock-LLM section) confirms the mechanism exists but reveals none of its policy.
- The relationship between `condenser_max_size: 240` (default) and modern 200k+ token windows is unexplained in-repo; whether 240 means turns, events, or thousands of tokens cannot be determined from frontend code (backend schema description says "tokens ... after summarization", `mocks/settings-handlers.ts:333`, but the default magnitude suggests a different unit).

---

Generated by `dimensions/11.02-token-budgeting-and-compression.md` against `openhands`.

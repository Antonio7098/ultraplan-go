# Source Analysis: openhands

## Dimension 20.01: Token and Cost Accounting

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React (OpenHands "agent-canvas" frontend; Vite, Zustand, TanStack Query, `@openhands/typescript-client`) |
| Analyzed | 2026-08-24 |

## Summary

This repository is the OpenHands **frontend** (per `AGENTS.md` repo map, the backend agent-server lives in a sibling `software-agent-sdk` repo, which is outside this study's boundary). Token and cost accounting is therefore implemented here as a **consumption, aggregation, and display layer**: all counting and dollar computation happens server-side, and this repo ingests the numbers through two redundant channels — live WebSocket `stats` events and a 30-second REST poll — merges them across per-usage-id metric buckets, and renders per-run summaries in a dedicated Usage tab.

The core accounting model is explicit and typed. A `TokenUsage` record carries prompt/completion/cache-read/cache-write/context-window/per-turn tokens (`src/api/conversation-service/agent-server-conversation-service.types.ts:29-36`), and a `MetricsSnapshot` carries `accumulated_cost` (USD), `max_budget_per_task`, and accumulated token usage (`src/api/conversation-service/agent-server-conversation-service.types.ts:38-42`). The server keys metrics by arbitrary LLM usage ids (`"default"`, `"condenser"`, `"profile:<name>:<uuid>"`; `src/types/agent-server/core/events/conversation-state-event.ts:44-47`), and the frontend sums them with `combineUsageMetrics` — documented as a TypeScript port of the Python SDK's `get_combined_metrics` (`src/utils/conversation-metrics.ts:8-12`). Costs are summed additively; context-window and per-turn token fields combine via `Math.max` rather than addition, which is semantically correct for those fields (`src/utils/conversation-metrics.ts:55-62`).

Answering "what did this run cost?" takes well under a minute: open the conversation's Usage tab, which shows total USD to 4 decimals, budget progress against `max_budget_per_task`, an input/output/cache token breakdown, a context-fill meter with warning (70%) and danger (90%) thresholds, and — for OpenRouter-backed providers — remaining provider credits (`src/components/features/conversation/usage-panel/usage-panel.tsx:22-86`). Automation runs additionally persist a per-run USD cost reported by the SDK completion callback and export it to CSV/JSON (`src/utils/automation-activity-log-export.ts:24-27`).

Notable gaps: the rich per-call data the server already provides — per-model `costs` ledger entries with timestamps, `response_latencies`, and raw `token_usages` — is typed but never consumed anywhere in the UI (`src/api/conversation-service/agent-server-conversation-service.types.ts:268-288`); tool-execution costs and retry costs have no surface at all; and one settings parser for max budget is dead code (`src/utils/settings-utils.ts:18-27`).

## Rating

**7 / 10.**

Rationale against the rubric ("clear model with tests, explicit interfaces, operational safeguards"):

- **Clear model**: typed wire shapes (`MetricsSnapshot`, `TokenUsage`, `LLMMetrics` at `src/api/conversation-service/agent-server-conversation-service.types.ts:29-42` and `src/types/agent-server/core/events/conversation-state-event.ts:25-47`), a single aggregation function with SDK-mirroring semantics (`src/utils/conversation-metrics.ts:12-73`), and documented ingestion precedence — live WebSocket store first, REST snapshot fallback (`src/hooks/use-live-conversation-metrics.ts:6-21, 42-84`).
- **Tests**: the aggregation behavior is pinned through the adapter (combining agent + condenser usage ids sums costs/tokens correctly, prefers backend-provided metrics over stats, defaults to zero-cost snapshot — `__tests__/api/agent-server-adapter.test.ts:921-1006`); the live hook pins precedence over the REST fallback, null-field coercion from the wire, missing-usage mapping, and polling-flag threading (`__tests__/hooks/use-live-conversation-metrics.test.ts:64-159`); the cross-conversation stale-figure hazard has a dedicated regression test asserting the metrics store resets on conversation switch (`__tests__/contexts/conversation-websocket-context.test.tsx:707-738`).
- **Operational safeguards**: metrics store reset on conversation switch prevents leaking the previous conversation's cost into the new meter (`src/contexts/conversation-websocket-context.tsx:303-318`); malformed wire values coerce to zero instead of `NaN` (`src/api/conversation-service/agent-server-conversation-service.api.ts:96-98, 115-128`); unknown context windows render raw counts instead of a misleading "% of 0" (`src/components/features/conversation/usage-panel/context-meter.tsx:41-43, 56-58`); ACP conversations label the dollar figure as an API-equivalent estimate rather than a real bill (`src/components/features/conversation/usage-panel/usage-panel.tsx:26-28, 71-75`); balance UI hides gracefully on 404/error while distinguishing timeout-as-error from absent-endpoint-as-null (`src/api/llm-balance-service.ts:57-69`).

Not scored higher because: it is display-only accounting with no client-side verification or drill-down (the server's per-call `costs`/`token_usages`/latency arrays go unused), there is no tool-level, retry-level, or historical trend visibility, and the budget input path contains dead code (see Gaps). The harness cannot answer "which model call cost what" or "what did that retry cost me" from anything surfaced here.

## Evidence Collected

Every entry includes a file path with line numbers (paths relative to the source root `studies/agent-harness-study/sources/openhands`).

| Area | Evidence | File:Line |
|------|----------|-----------|
| Token counters (wire shape) | `TokenUsage`: prompt/completion tokens, cache_read_tokens, cache_write_tokens, context_window, per_turn_token | `src/api/conversation-service/agent-server-conversation-service.types.ts:29-36` |
| Richer per-call token record | Per-call `TokenUsage` adds `model`, `reasoning_tokens`, `response_id` — typed but not consumed downstream | `src/types/agent-server/core/events/conversation-state-event.ts:10-20` |
| Cost snapshot type | `MetricsSnapshot { accumulated_cost, max_budget_per_task, accumulated_token_usage }` | `src/api/conversation-service/agent-server-conversation-service.types.ts:38-42` |
| Per-usage-id metric buckets | `UsageToMetrics = Record<string, LLMMetrics>` keyed by arbitrary ids ("default", "condenser", "profile:<name>:<uuid>") | `src/types/agent-server/core/events/conversation-state-event.ts:44-47` |
| Cost calculator (aggregation) | `combineUsageMetrics` sums `accumulated_cost` across usage ids, combines token fields, keeps first non-null budget; doc comment says it mirrors the Python SDK `get_combined_metrics` | `src/utils/conversation-metrics.ts:12-73` |
| Correct field-specific combination | `context_window`/`per_turn_token` merge via `Math.max`, additive fields sum | `src/utils/conversation-metrics.ts:55-62` |
| Live ingestion channel | WS `ConversationStateUpdateEvent` with `key: "stats"` carries full `ConversationStats` | `src/types/agent-server/core/events/conversation-state-event.ts:135-138` |
| Live metrics update handler | `updateMetricsFromStats` reduces `usage_to_metrics` into `{cost, maxBudgetPerTask, usage}` and writes the store | `src/contexts/conversation-websocket-context.tsx:217-276` |
| Handler dispatch points | Stats branch invoked in main WS handler (line 646-648) and duplicated in planning-agent handler (line 879) | `src/contexts/conversation-websocket-context.tsx:637-648, 879` |
| Metrics store | Zustand `useMetricsStore` with `setMetrics`/`resetMetrics`; dev-only `window.__OH_METRICS_STORE__` debug handle | `src/stores/metrics-store.ts:16-40` |
| Stale-cost safeguard | Store reset on conversation switch so the previous conversation's cost cannot render in the new meter | `src/contexts/conversation-websocket-context.tsx:303-318` |
| Fallback ingestion channel | `useConversationMetrics` polls `getRuntimeConversation` every 30s, `retry: false` | `src/hooks/query/use-conversation-metrics.ts:16-42` |
| REST fetch + normalization | `getRuntimeConversation` normalizes `metrics` and passes `stats ?? {usage_to_metrics: {}}` | `src/api/conversation-service/agent-server-conversation-service.api.ts:690-715` |
| Wire-coercion safeguards | `numberOrZero`/`normalizeTokenUsage`/`normalizeMetrics` coerce non-numeric wire values to 0/null | `src/api/conversation-service/agent-server-conversation-service.api.ts:96-148` |
| Ingestion precedence | `useLiveConversationMetrics` prefers live store when any field non-null, else REST snapshot mapped to flat shape | `src/hooks/use-live-conversation-metrics.ts:42-84` |
| Adapter-level fallback | `toAppConversation` uses `info.metrics` if present, else `combineUsageMetrics(info.stats)` (#16480) | `src/api/agent-server-adapter.ts:365-386` |
| Per-run cost summary UI | Usage tab renders `CostSection` (total `$X.XXXX` to 4 decimals) plus token breakdown rows | `src/components/features/conversation/usage-panel/usage-panel.tsx:76-80`; `src/components/features/conversation/metrics-modal/cost-section.tsx:17-27`; `src/components/features/conversation/metrics-modal/usage-section.tsx:19-50` |
| Budget tracking display | `BudgetDisplay` → progress bar + usage text when `max_budget_per_task > 0`, else "No budget limit"; percentage capped at 100 | `src/components/features/conversation-panel/budget-display.tsx:12-34`; `src/components/features/conversation-panel/budget-progress-bar.tsx:12`; `src/i18n/translation.json:21899` |
| Budget configuration field | `max_budget_per_task` in persisted `Settings` (default null) | `src/types/settings.ts:143`; `src/services/settings.ts:28` |
| Context-window observability | Context meter with 70% warning / 90% danger tones driven by `per_turn_token` vs `context_window` | `src/components/features/conversation/usage-panel/context-meter.tsx:7-21` |
| Tokens as compaction feedback loop | `useAwaitContextCompaction` measures before/after `per_turn_token` from the live metrics store to report tokens saved, with settle window and timeout outcome taxonomy | `src/hooks/use-await-context-compaction.ts:6-9, 83-103, 150-154` |
| Provider-level spend | `LLMBalanceService.getBalance()` fetches provider credit limit/remaining/used + daily/weekly/monthly usage (OpenRouter today) | `src/api/llm-balance-service.ts:12-24, 70-110` |
| Provider balance UI | `ProviderBalanceCard` hidden entirely on missing endpoint/error; manual refresh affordance | `src/components/features/conversation/usage-panel/provider-balance-card.tsx:13-29, 63-77` |
| Balance query gating | Cloud backends excluded (no `/api/llm/balance` there), `staleTime: Infinity`, `retry: false` | `src/hooks/query/use-llm-balance.ts:14-28` |
| Automation per-run cost | `AutomationRun.cost` = accumulated LLM cost USD from SDK completion callback; null means unknown/cancelled/predates tracking | `src/types/automation.ts:88-95` |
| Automation cost rendering | Genuine $0 rendered as `$0.0000` (not hidden); matches 4-decimal convention | `src/components/features/automations/detail/activity-log-item.tsx:41-52` |
| Automation cost export | CSV column `cost` appended last (position-compatible), JSON export normalizes missing cost to null | `src/utils/automation-activity-log-export.ts:12-27, 104-106`; `src/types/automation.ts:120-126` |
| Unused rich ledger | `RuntimeMetrics.costs[]` (`{model, cost, timestamp}`), `response_latencies[]`, `token_usages[]` typed but no consumer found | `src/api/conversation-service/agent-server-conversation-service.types.ts:268-288` |
| Tests: aggregation | Combines agent+condenser ids (cost 1.5+0.5→2; tokens summed; budget taken from any non-null), prefers direct metrics, zero-default | `__tests__/api/agent-server-adapter.test.ts:921-1006` |
| Tests: REST fallback path | `useConversationMetrics` test exercises `usage_to_metrics` combination via query hook | `__tests__/hooks/query/use-conversation-metrics.test.tsx:18, 121` |
| Tests: live precedence & coercion | Live-store-over-REST preference, null wire fields coerced to 0, missing usage → null, enabled-flag threading | `__tests__/hooks/use-live-conversation-metrics.test.ts:64-159` |
| Tests: stale-state reset | Switching conversations resets cost/budget/usage to null | `__tests__/contexts/conversation-websocket-context.test.tsx:707-738` |

## Answers to Dimension Questions

1. **Are tokens counted per run?**
   Yes — but counting itself happens in the agent-server, not here. This repo accumulates and displays per-conversation totals: `accumulated_token_usage` across all LLM usage ids, refreshed live via WS stats events (`src/contexts/conversation-websocket-context.tsx:217-276`) and polled via REST every 30s (`src/hooks/query/use-conversation-metrics.ts:38-40`). Six token dimensions are tracked including cache reads/writes and current-turn context fill (`src/api/conversation-service/agent-server-conversation-service.types.ts:29-36`). There is no local tokenization anywhere in the app code (searched for `tiktoken`/`tokenizer`/`countToken`; only hits are an argv *tokenizer* for shell parsing in `src/utils/acp-command.ts:22` and i18n copy describing the backend's optional custom tokenizer setting).

2. **Are costs attributed per model call?**
   Partially, and invisibly. The server attributes costs per LLM usage id (agent vs condenser vs profile) and even records a per-call `costs` ledger with model name and timestamp (`src/types/agent-server/core/events/conversation-state-event.ts:30-34`), but the frontend collapses everything into a single `accumulated_cost` scalar (`src/utils/conversation-metrics.ts:23, 30`). No UI renders the per-id, per-model, or per-call breakdown; grep for `.costs\b`, `token_usages`, and `response_latencies` finds only the type declarations, no consumers.

3. **Are tool execution costs tracked?**
   No evidence found. Searched `src/` for tool-cost patterns (tool+cost/retry+cost co-occurrence, per-tool ledgers). Tools execute inside the sandbox; their cost footprint is implicit in the LLM token usage of surrounding turns and is not broken out. The closest proxy is the condenser usage id being separately bucketable server-side, but the UI merges it away.

4. **Are retry costs accounted for?**
   No evidence found for explicit retry-cost accounting. Any retry cost is implicitly folded into server-side `accumulated_cost`. On the frontend, the metrics REST query disables retries outright (`retry: false`, `src/hooks/query/use-conversation-metrics.ts:41`) and the balance query likewise (`src/hooks/query/use-llm-balance.ts:25`), so transport retries do not double-count displays — but there is no mechanism to see what LLM-level retries cost.

5. **Are per-run cost summaries available?**
   Yes, in three forms: (a) the interactive Usage tab showing total USD, budget progress, token breakdown, context meter, and provider credits (`src/components/features/conversation/usage-panel/usage-panel.tsx:41-86`); (b) automation runs carrying `AutomationRun.cost` rendered per row and exportable to CSV/JSON (`src/utils/automation-activity-log-export.ts:12-27, 143-171`); (c) a dev-only debug handle exposing the raw live store (`window.__OH_METRICS_STORE__`, `src/stores/metrics-store.ts:33-40`). There is no aggregate summary across conversations/runs.

## Architectural Decisions

1. **Server-authoritative accounting, client-side aggregation.** The frontend performs no counting or pricing; it trusts `accumulated_cost`/token figures from the agent-server and focuses on correct merging across usage-id buckets (`src/utils/conversation-metrics.ts:8-12` explicitly ports the SDK's `get_combined_metrics`). This avoids drift between counted and priced tokens but makes the client unable to detect or explain server-side accounting errors.

2. **Dual-channel ingestion with documented precedence.** Live WS stats updates win over the 30s REST snapshot; the rationale (real-time display, post-reset scoping to the active conversation, REST lagging a poll interval behind) is written out at `src/hooks/use-live-conversation-metrics.ts:6-21`. The same duplication exists between `updateMetricsFromStats` in the WS context (`src/contexts/conversation-websocket-context.tsx:227-267`) and `combineUsageMetrics` (`src/utils/conversation-metrics.ts:28-66`) — two hand-written implementations of the identical reduce.

3. **Per-usage-id bucketing preserved end-to-end until display.** The wire keeps `Record<string, LLMMetrics>` keyed by arbitrary ids (`src/types/agent-server/core/events/conversation-state-event.ts:44-47`); tests pin that agent and condenser buckets both contribute to the total (`__tests__/api/agent-server-adapter.test.ts:921-974`). Attribution granularity exists on the wire but is deliberately flattened for presentation.

4. **Conversation-scoped ephemeral state with reset discipline.** Metrics live in a global Zustand store that is reset whenever `conversationId` changes, ordered before history re-seeding to avoid stale-dollar flash (`src/contexts/conversation-websocket-context.tsx:303-318`); a regression test locks this (`__tests__/contexts/conversation-websocket-context.test.tsx:707-738`).

5. **Graceful degradation as a stated design goal.** Missing endpoints hide their UI (balance card returns null on 404/error, `provider-balance-card.tsx:29`), older servers without `metrics` fall back to `stats` combination (#16480, `src/api/agent-server-adapter.ts:365-386`), unknown context windows render raw counts (`context-meter.tsx:41-43`).

## Notable Patterns

- **Semantics-aware merging**: additive fields (tokens, dollars) sum; state-like fields (`context_window`, `per_turn_token`) take `Math.max` because they describe the current turn, not cumulative volume — consistently applied in both the REST combiner (`src/utils/conversation-metrics.ts:55-62`) and the WS reducer (`src/contexts/conversation-websocket-context.tsx:254-261`).
- **Null-vs-zero honesty**: the automation cost formatter distinguishes genuine `$0` from unknown-null, rendering `$0.0000` only when the SDK actually reported zero (`activity-log-item.tsx:41-52`), and the CSV export preserves nullability rather than coercing (`automation-activity-log-export.ts:53-60`).
- **Tokens as a control-loop signal**, not just display: `useAwaitContextCompaction` subscribes to the metrics store and event stream to confirm a condensation landed by observing `per_turn_token` drop below a pre-request snapshot, with a 2.5s settle window and 90s timeout classified into `compacted`/`no_change`/`timeout` outcomes (`src/hooks/use-await-context-compaction.ts:6-9, 96-109, 150-154`).
- **Honest labeling of estimates**: ACP conversations (external CLI agents like Claude Code billing their own subscriptions) show an explicit note that the dollar figure is an API-equivalent estimate, not a bill (`usage-panel.tsx:26-28, 71-75`).
- **Dev-only observability escape hatch**: `window.__OH_METRICS_STORE__` gated on `import.meta.env.DEV` (`src/stores/metrics-store.ts:33-40`).

## Tradeoffs

- **Trust-the-server simplicity vs auditability.** No client-side recomputation means a mispriced or dropped server-side cost event propagates silently; conversely, the client stays free of tokenizer/pricing-table maintenance.
- **Aggregation for clarity vs attribution depth.** Summing across usage ids gives users one honest number fast, but discards the per-model/per-call ledger the server already ships (`conversation-state-event.ts:30-34` unused), making cost debugging ("why did this conversation cost $5?") impossible from the UI.
- **Live-first freshness vs duplicate logic.** Keeping a second hand-rolled reducer in the WS context duplicates `combineUsageMetrics`; the two could diverge silently (they currently agree).
- **Global store + reset vs scoped stores.** One metrics store simplifies hooks but relies on disciplined reset ordering at conversation switch; the code documents exactly why ordering matters (`conversation-websocket-context.tsx:308-317`).
- **Polling cadence vs load.** A fixed 30s interval (`use-conversation-metrics.ts:40`) with an opt-out `enabled` flag balances freshness against request volume; hidden consumers can pause polling (`use-live-conversation-metrics.ts:19-20`).

## Failure Modes / Edge Cases

- **Stale cost across conversations**: mitigated by reset-on-switch (`conversation-websocket-context.tsx:313-317`), regression-tested (`conversation-websocket-context.test.tsx:707-738`).
- **Malformed wire metrics**: coerced to zero/null by `normalizeTokenUsage`/`normalizeMetrics` (`agent-server-conversation-service.api.ts:115-138`) and again at the hook layer (`use-live-conversation-metrics.ts:57-74`); the hook tests pin null-token coercion (`use-live-conversation-metrics.test.ts:85-106`).
- **Backend variants omitting `metrics` but populating `stats`**: handled by the #16480 adapter fallback, tested (`agent-server-adapter.test.ts:921-996`).
- **Unknown context window** (`context_window <= 0`, e.g. some OpenRouter models): percentage readout replaced with raw count (`context-meter.tsx:41-58`).
- **Balance endpoint absent (older servers) vs timing out**: 404 maps to null (hide UI) while timeouts stay errors to avoid permanently caching a wrong absence under `staleTime: Infinity` (`llm-balance-service.ts:57-69`); cloud backends never probe (`use-llm-balance.ts:20-22`).
- **Cancelled/pre-cost automation runs**: `cost` is null rather than fabricated; export rows normalize shape (`types/automation.ts:88-95, 120-126`).
- **Unbounded budget**: budget UI degrades to "No budget limit" text when `max_budget_per_task` is null/zero (`budget-display.tsx:22-31`); progress percentage clamped at 100 (`i18n/translation.json:21899`, clamp in `format-token-count.ts:26-30` for context; budget bar computes `(currentCost/maxBudget)*100` unclamped at `budget-progress-bar.tsx:12`).
- **Residual risk**: the WS-path reducer writes the store only when `usage_to_metrics` is truthy (`conversation-websocket-context.tsx:219-222`); a stats event with an empty map leaves prior figures displayed rather than zeroing them — acceptable mid-stream but could show stale numbers after server-side metric resets.

## Future Considerations

- Surface the existing per-model/per-call `costs`, `token_usages`, and `response_latencies` arrays (already on the wire, `agent-server-conversation-service.types.ts:268-288`) as a drill-down view — the highest-leverage improvement given zero new backend work.
- Extract the duplicated WS reducer to reuse `combineUsageMetrics` (map `usage_to_metrics` into `RuntimeConversationStats` shape) so the two paths cannot diverge.
- Add a direct unit suite for `combineUsageMetrics` edge cases (empty map, single id, conflicting budgets) — it currently has only indirect coverage through the adapter.
- Expose retry/failure cost deltas once the server distinguishes them; nothing client-side blocks this.
- Remove or rewire `parseMaxBudgetPerTask` (`settings-utils.ts:18-27`), which has no importer.
- Cross-run aggregates (spend per day/workspace/automation) would build naturally on the automation CSV pipeline (`automation-activity-log-export.ts:113-138`).

## Questions / Gaps

- Where exactly does the agent-server compute `accumulated_cost` and how does it treat LLM retries, streaming partial failures, and cache pricing? Out of scope here (server lives in the sibling `software-agent-sdk` repo; this study was restricted to `sources/openhands`). The frontend's doc comments assert the SDK reports automation costs in a "completion callback" (`src/types/automation.ts:89-90`) and that `combineUsageMetrics` mirrors `get_combined_metrics` (`src/utils/conversation-metrics.ts:9-11`), but the server implementation was not inspected.
- Are tool executions ever billed independently (e.g., paid MCP servers)? No evidence found in this repo either way; searched tool/cost co-occurrences across `src/`.
- Is `max_budget_per_task` enforced (run halted at cap) server-side, and does the frontend receive any near-budget warning event? Only display-side evidence found; no enforcement or threshold-event handling exists in `src/`.
- The `"Input Cost Per Token"` / `"Output Cost Per Token"` i18n strings (`src/i18n/translation.json:3335, 3845`) appear orphaned — no component referencing corresponding declaration keys was found; they may be leftovers from an older custom-LLM pricing form.

---

Generated by `Dimension 20.01: Token and Cost Accounting` against `openhands`.

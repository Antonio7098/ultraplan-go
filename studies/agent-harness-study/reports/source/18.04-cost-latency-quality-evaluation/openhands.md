# Source Analysis: openhands

## Dimension 18.04: Cost, Latency, and Quality Evaluation

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React (Vite), Vitest + Playwright (mock-LLM and live E2E); the OpenHands "agent-canvas" frontend |
| Analyzed | 2026-08-25 |

## Summary

This repository is the OpenHands **frontend** (agent-canvas). Per its own architecture note (`sources/openhands/AGENTS.md:60-70`), benchmarking and agent-loop behavior belong to sibling repos (`software-agent-sdk`, etc.), and a repo-wide search confirms **no evaluation harness exists here**: searches for `benchmark`, `SWE-bench`, `eval harness`, and `evaluation` surface only a PR-review skill doc warning reviewers about "benchmark/evaluation performance" risk (`sources/openhands/.agents/skills/custom-codereview-guide.md:18-24`) and test fixtures. Consequently the dimension's questions must be answered against what the frontend actually builds: **runtime cost/token observability, automation-run success-rate analytics, a critic quality signal, and latency data that is transported but never consumed**.

What exists is substantial but split:

1. **Token cost is measured and surfaced end-to-end** — typed metrics schemas (`accumulated_cost`, per-model `costs[]`), multi-usage-id combination logic, WebSocket + 30s REST polling, a usage panel showing dollar totals, budget caps with progress bars, and provider credit balances — all with unit tests.
2. **Success rate is tracked for automations**, computed as COMPLETED/(COMPLETED+FAILED) over terminal runs with average duration, rendered on cards/dashboard tiles, and unit-tested.
3. **Quality is signaled, not evaluated** — a server-side critic emits a predicted success probability (0–1) per event, rendered as stars with categorized issue breakdowns, verified by a dedicated live LLM E2E test.
4. **Latency is recorded only in the wire schema** (`response_latencies[]`); no frontend code reads, renders, or analyzes it.
5. **Model choice is switchable at runtime** (`switchLLM`, `/model` slash command E2E) and costs carry per-model attribution, but **no artifact compares two models on cost vs quality** — the answer to the dimension's headline question is **no**.

The one explicit cost-conscious decision is operational, not analytical: live E2E pins a cheap Haiku-class model with a written "stay cheap" policy (`AGENTS.md:211-212`).

## Rating

**4 / 10 — Present but inconsistent.**

Rationale against the rubric: token-cost measurement (rubric step 1) is a clear, tested model (would score 7–8 alone as observability); success-rate tracking (step 3) has a tested implementation for automations; but the dimension's frame is *evals*, and this repo contains no eval harness at all. Latency (step 2) exists only as dead schema fields. Model choice (step 4) is factored into switching and attribution but never into comparison. Cost/quality tradeoff analysis (step 5) has no evidence whatsoever — cost displays and critic scores are never joined. The result is real, quality-built machinery covering parts of the dimension with large structural gaps, which lands at the top of "present but inconsistent."

## Evidence Collected

Every entry cites `sources/openhands/<path>:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Cost schema (REST) | `RuntimeMetrics` carries `accumulated_cost`, `max_budget_per_task`, `accumulated_token_usage`, per-model `costs[]` (`{model, cost, timestamp}`) | `sources/openhands/src/api/conversation-service/agent-server-conversation-service.types.ts:268-288` |
| Cost schema (WebSocket events) | `LLMMetrics` with `accumulated_cost`, `max_budget_per_task`, `costs[]`, `token_usages[]`; keyed by arbitrary usage ids ("default", "condenser", "profile:<name>:<uuid>") | `sources/openhands/src/types/agent-server/core/events/conversation-state-event.ts:25-47` |
| Multi-source cost combination | `combineUsageMetrics()` sums `accumulated_cost` across all usage ids, keeps first non-null budget, merges token counters (mirrors Python SDK `get_combined_metrics`) | `sources/openhands/src/utils/conversation-metrics.ts:12-79` |
| Live WS accumulation | Stats events folded into Zustand store: `acc.cost += metrics.accumulated_cost`, per-field token merge | `sources/openhands/src/contexts/conversation-websocket-context.tsx:217-276` |
| REST polling hook | `useConversationMetrics` polls combined metrics every 30s (`staleTime`/`refetchInterval` 30s) | `sources/openhands/src/hooks/query/use-conversation-metrics.ts:37-41` |
| Store-vs-REST precedence | `useLiveConversationMetrics` prefers live store, falls back to REST snapshot; documented leak-safety rationale | `sources/openhands/src/hooks/use-live-conversation-metrics.ts:22-84` |
| Cost UI | Usage panel renders total cost `$${cost.toFixed(4)}` plus token rows (input/output/cache hit/cache write/total) | `sources/openhands/src/components/features/conversation/metrics-modal/cost-section.tsx:20-25`; `sources/openhands/src/components/features/conversation/metrics-modal/usage-section.tsx:14-52` |
| Budget cap display | `BudgetDisplay` renders progress bar + usage text when `max_budget_per_task > 0`, else "No budget limit"; setting defined in defaults | `sources/openhands/src/components/features/conversation-panel/budget-display.tsx:12-34`; `sources/openhands/src/services/settings.ts:28` |
| Panel composition | UsagePanel tab wires ContextMeter + CompactContextButton + UsageSection + CostSection + ProviderBalanceCard; ACP runs labeled "plan usage note" (dollar figure is estimate, not bill) | `sources/openhands/src/components/features/conversation/usage-panel/usage-panel.tsx:22-87` |
| Provider balance | Credits used/remaining fetched from `/api/llm/balance`, graceful 404 degradation | `sources/openhands/src/components/features/conversation/usage-panel/provider-balance-card.tsx:13-78` |
| Latency schema (unused) | `ResponseLatency {model, latency, response_id}` and `response_latencies[]` exist in both wire types; zero consumers render them | `sources/openhands/src/api/conversation-service/agent-server-conversation-service.types.ts:284-288`; `sources/openhands/src/types/agent-server/core/events/conversation-state-event.ts:35-39` |
| Latency never exercised | All test fixtures pass `response_latencies: []`; grep over `src/` finds no reader | `sources/openhands/__tests__/hooks/query/use-conversation-metrics.test.tsx:25,135,151` |
| Success-rate computation | `summarizeAutomationRuns()`: `recentSuccessRate = completed / terminal` over newest page; excludes cancelled/skipped; `averageDurationMs` mean of `completed_at − started_at` | `sources/openhands/src/manifests/automation-insights.ts:51-105` |
| Success-rate UI | AutomationRunStats 3-column footer (runs / recent success % / average duration); dashboard tiles "needs-attention", "total-runs", "average-duration" | `sources/openhands/src/components/features/automations/automation-run-insights.tsx:33-71`; `sources/openhands/src/manifests/automation-insights.ts:252-301` |
| Success-rate tests | Unit tests for summary math (rate 0.5, null without terminal runs, duration averaging) and component assertion on `automation-run-stats` | `sources/openhands/__tests__/manifests/automation-insights.test.ts:134-181`; `sources/openhands/__tests__/components/automations/automation-card.test.tsx:169` |
| Critic quality signal | `CriticResult.score` = "Predicted probability of success (0-1)" with categorized features (agent behavioral issues, infra, user follow-ups); `GoalVerdict.score` for `/goal` loops | `sources/openhands/src/types/agent-server/core/base/critic.ts:37-50`; `sources/openhands/src/types/agent-server/core/events/conversation-state-event.ts:68-75` |
| Critic rendering | Star rating + color thresholds (≥0.6 green, ≥0.4 yellow) + expandable feature breakdown + iterative-refinement hint when disabled | `sources/openhands/src/components/conversation-events/chat/event-message-components/critic-result-display.tsx:16-48,174-247` |
| Critic verified live | Dedicated live-LLM E2E: configures `critic_enabled/critic_model_name`, polls events API for critic result, asserts UI display | `sources/openhands/tests/e2e/live/real-agent-server-conversation.spec.ts:142-192`; `sources/openhands/tests/e2e/live/utils/agent-server-conversation.ts:200-215,648-676` |
| Runtime model switching | `switchProfile()` resolves profile model and calls `ConversationClient.switchLLM` with fresh `usage_id: profile:<name>:<uuid>`; `switchAcpModel()` switches live ACP session model preserving context | `sources/openhands/src/api/conversation-service/agent-server-conversation-service.api.ts:891-909,925-929` |
| Model-switch E2E | Mock-LLM spec exercises `/model` slash-command profile switch and verifies the switch landed | `sources/openhands/tests/e2e/mock-llm/settings/mock-llm-model-switch.spec.ts:50,92,137` |
| Cheap-model CI policy | Live E2E defaults to `openhands/claude-haiku-4-5-20251001`; AGENTS.md mandates the live test "should stay cheap and as deterministic as possible" | `sources/openhands/AGENTS.md:211-212`; `sources/openhands/tests/e2e/live/utils/agent-server-conversation.ts:42-45` |
| Mocked-token E2E economy | Mock-LLM harness fabricates fixed usage chunks (`prompt_tokens: 10, completion_tokens: 5`) so full-stack E2E costs $0 | `sources/openhands/tests/e2e/mock-llm/scripts/mock-llm-server.py:300-310` |
| Adapter-level cost tests | Adapter tests assert merged `accumulated_cost` across usage ids (1.5+0.5=2, override 3) | `sources/openhands/__tests__/api/agent-server-adapter.test.ts:928-1002` |

## Answers to Dimension Questions

**1. Is token cost measured in evals?**
Not in *evals* — there is no eval harness here (searches for `benchmark|SWE-bench|eval harness|evaluation` returned only `.agents/skills/custom-codereview-guide.md:18-24`). Token cost **is** measured comprehensively as runtime telemetry: `accumulated_cost` and per-call `costs[]` flow through both REST (`sources/openhands/src/api/conversation-service/agent-server-conversation-service.types.ts:268-282`) and WebSocket event schemas (`sources/openhands/src/types/agent-server/core/events/conversation-state-event.ts:25-41`), are summed across all LLM usage ids including condenser calls (`sources/openhands/src/utils/conversation-metrics.ts:28-66`), and are user-visible to four decimal places (`sources/openhands/src/components/features/conversation/metrics-modal/cost-section.tsx:24`). Budget caps (`max_budget_per_task`) render progress bars (`sources/openhands/src/components/features/conversation-panel/budget-display.tsx:22-31`). This is per-conversation accounting, not aggregate eval reporting.

**2. Is latency measured?**
Only at the schema boundary. `RuntimeMetrics.response_latencies: ResponseLatency[]` (`{model, latency, response_id}`) is declared in both wire contracts (`sources/openhands/src/api/conversation-service/agent-server-conversation-service.types.ts:284-288`, `sources/openhands/src/types/agent-server/core/events/conversation-state-event.ts:35-39`), but a repo-wide grep finds **zero** readers in application code — every fixture hardcodes `response_latencies: []` (`sources/openhands/__tests__/hooks/query/use-conversation-metrics.test.tsx:25`). Duration is only derived outside the LLM path: automation wall-clock averages (`completed_at − started_at`, `sources/openhands/src/manifests/automation-insights.ts:87-103`). Verdict: latency is transported, never consumed.

**3. Is success rate tracked?**
Yes, for automations — the strongest tested piece of this dimension. `summarizeAutomationRuns()` computes `recentSuccessRate` as COMPLETED/(COMPLETED+FAILED) over the newest runs page, explicitly excluding cancelled/skipped runs as uninformative (`sources/openhands/src/manifests/automation-insights.ts:57-99`), alongside lifetime totals and mean durations. It drives card footers (`sources/openhands/src/components/features/automations/automation-run-insights.tsx:33-56`), health derivation ("failing" when latest run FAILED), dashboard tiles, and status filters, with unit tests covering the rate math and edge cases (`sources/openhands/__tests__/manifests/automation-insights.test.ts:134-181`). Separately, per-conversation *predicted* success comes from the critic score (0–1 probability, `sources/openhands/src/types/agent-server/core/base/critic.ts:43-46`) — a quality estimate, not a measured outcome.

**4. Are model tradeoffs analyzed?**
No. Models can be switched mid-conversation with per-profile usage-id attribution (`sources/openhands/src/api/conversation-service/agent-server-conversation-service.api.ts:897-909`), and every cost/latency/token record carries a `model` field (`sources/openhands/src/api/conversation-service/agent-server-conversation-service.types.ts:278-288`), so the raw ingredients for comparison exist. But no view, script, report, or test joins cost with quality across models; the critic score and the cost panel never meet in code. The only tradeoff decision found is implicit and operational: live E2E pins Claude Haiku because the suite "should stay cheap" (`sources/openhands/AGENTS.md:211-212`). You cannot currently compare two model choices on cost vs quality using anything in this repo.

## Architectural Decisions

- **Observability lives in the client, aggregation mirrors the SDK.** `combineUsageMetrics()` is documented as "the TypeScript equivalent of the get_combined_metrics method from the Python SDK" (`sources/openhands/src/utils/conversation-metrics.ts:8-11`), deliberately re-implemented so the thin frontend can combine per-usage-id metrics (agent vs condenser vs profiles) without a new server endpoint.
- **Dual-path metric ingestion with live preference.** Metrics arrive via WebSocket stats events into a Zustand store and via a 30s REST poll; the consumer prefers the store because post-switch resets guarantee conversation ownership, avoiding up-to-one-poll-interval lag (`sources/openhands/src/hooks/use-live-conversation-metrics.ts:6-21`).
- **Server-owned economics.** Cost computation, budgets, provider balances, critic scoring, and success/failure classification all come from backend APIs; the frontend only transports and renders them. This is consistent with the repo map in `sources/openhands/AGENTS.md:60-70`.
- **Test-economy as policy.** Three tiers encode cost discipline: mocked unit tests (free), mock-LLM E2E with scripted trajectories and fabricated token counts (`sources/openhands/tests/e2e/mock-llm/scripts/mock-llm-server.py:300-310`), and gated live E2E pinned to a cheap model, label-gated in CI and fork-skipped before credential checkout (`sources/openhands/AGENTS.md:215`).

## Notable Patterns

- **Per-usage-id namespacing**: metrics are keyed by arbitrary ids ("default", "condenser", "profile:<name>:<uuid>", `sources/openhands/src/types/agent-server/core/events/conversation-state-event.ts:44-47`), giving natural attribution of cost to agent vs condenser vs switched profiles — the seam where future per-model comparison would plug in.
- **Cache-aware token accounting**: prompt/completion tokens are decomposed into cache-read and cache-write components throughout schema, combination logic, and UI rows (`sources/openhands/src/components/features/conversation/metrics-modal/usage-section.tsx:19-37`) — economically meaningful since cached tokens price differently.
- **Defensive rendering of probabilistic scores**: critic scores are clamped to [0,1] before star mapping (`sources/openhands/src/components/conversation-events/chat/event-message-components/critic-result-display.tsx:16-19`).
- **Honest labeling of estimated spend**: ACP conversations annotate the dollar figure as an API-equivalent estimate rather than a bill (`sources/openhands/src/components/features/conversation/usage-panel/usage-panel.tsx:26-28`).
- **Sample-scoped statistics**: automation success rates are explicitly documented as derived from the newest fetched page, with lifetime count taken from the response — an honest sample-vs-population distinction (`sources/openhands/src/manifests/automation-insights.ts:10-14,52-53`).

## Tradeoffs

- **Rich per-conversation cost visibility vs zero aggregate analysis.** A user sees exact spend per conversation, but nothing accumulates across conversations or models; the data needed for fleet-level cost/quality curves is displayed once and dropped.
- **Latency shipped but unread.** Carrying `response_latencies` in two type definitions costs maintenance surface with no consumer; either it awaits a UI (e.g., slow-turn warnings) or it is dead weight.
- **Predicted success vs measured success.** The critic gives an instant quality proxy per conversation, while automations measure realized success only for recurring jobs; one-off interactive work has no outcome ground truth beyond the critic's guess.
- **Cheap-CI discipline vs coverage realism.** Pinning live E2E to Haiku with one retry (`sources/openhands/AGENTS.md:211-212`) controls spend but means the flagship live test validates behavior on a small model only; regressions specific to stronger models' behavior would go unseen.

## Failure Modes / Edge Cases

- **Null-tolerant metric combination**: missing `usage_to_metrics` yields a zeroed snapshot rather than throwing (`sources/openhands/src/utils/conversation-metrics.ts:15-21`); string-coerced `"1.23"` costs are normalized via `numberOrNull` (`sources/openhands/__tests__/api/agent-server-conversation-service.test.ts:735-758`).
- **Negative/non-finite durations filtered** before averaging automation durations (`ms >= 0 && Number.isFinite(ms)` guard, `sources/openhands/src/manifests/automation-insights.ts:87-92`).
- **Division-by-zero avoided**: success rate returns `null` when no terminal runs exist, rendered as "—" (`sources/openhands/src/manifests/automation-insights.ts:98-99`; `sources/openhands/src/components/features/automations/automation-run-insights.tsx:33-37`).
- **Budget-less display path**: `max_budget_per_task === null` degrades to "No budget limit" text instead of a broken bar (`sources/openhands/src/components/features/conversation-panel/budget-display.tsx:22-31`).
- **Balance endpoint absence**: `/api/llm/balance` 404s hide the provider card entirely (`sources/openhands/src/components/features/conversation/usage-panel/provider-balance-card.tsx:13-29`).
- **Gap observed**: because the WS reducer takes the first non-null `max_budget_per_task` across usage ids (`sources/openhands/src/contexts/conversation-websocket-context.tsx:234-239`), multiple concurrent LLM entries with different caps resolve arbitrarily to whichever id iterates first; same first-wins rule in the REST combiner (`sources/openhands/src/utils/conversation-metrics.ts:32-35`).

## Future Considerations

- Join critic scores with accumulated cost per conversation to enable the missing cost-vs-quality view; all inputs already coexist on the same event stream and stats objects.
- Consume `response_latencies` (per-model latency already attributed) for a latency column in the usage panel or slow-turn diagnostics.
- Extend automation-style `recentSuccessRate`/`averageDurationMs` summaries to interactive conversations by adopting terminal-status semantics from the execution-status stream.
- Persist per-usage-id metrics history server-side so cross-conversation and cross-model comparisons become possible without frontend-side accumulation.

## Questions / Gaps

- **Where do evals actually run?** Not in this repo. The review skill references "evaluation harness code" as a risk area (`sources/openhands/.agents/skills/custom-codereview-guide.md:18-24`) and a test fixture mentions `OpenHands/evaluation` as an external repo (`sources/openhands/__tests__/components/conversation-events/chat/event-content-helpers/get-acp-tool-call-content.test.ts:16`), implying the harness lives elsewhere. Studying it would require those sources, which are outside this task's isolation boundary.
- **Is `response_latencies` populated by the current agent-server?** Undeterminable from the frontend alone; fixtures always send `[]`. No evidence found in this source.
- **Does any dashboard aggregate cost across conversations?** Searches over `src/`, `docs/`, and `specs/` (only four spec files exist: `backend-management.md`, `llm-defaults.md`, `mcp-settings.md`, `workspace-upload-path.md`) found none. The cloud conversation list carries per-item metrics (`sources/openhands/__tests__/api/agent-server-conversation-service.test.ts:650-679`) but no roll-up was found.
- **What thresholds drive critic color bands?** They are hardcoded presentation choices (0.6/0.4 in `sources/openhands/src/components/conversation-events/chat/event-message-components/critic-result-display.tsx:35-39`); whether they were calibrated against any labeled outcomes is not evidenced here.

---

Generated by `Dimension 18.04: Cost, Latency, and Quality Evaluation` against `openhands`.

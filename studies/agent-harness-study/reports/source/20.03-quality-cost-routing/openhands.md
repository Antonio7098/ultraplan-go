# Source Analysis: openhands

## 20.03 Quality-Cost Routing

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React (Vite, React Router, Zustand, TanStack Query) — **frontend only** |
| Analyzed | 2026-08-26 |

## Summary

OpenHands (this repository) is explicitly the **agent-canvas frontend** (`AGENTS.md:10-14` — "This repository is the OpenHands frontend"). All LLM routing, execution, and fallback logic lives in the sibling `software-agent-sdk` (agent-server). The frontend's role is confined to: (a) persisting a single LLM configuration per conversation, (b) letting the user manually switch it, and (c) surfacing cost/budget as read-only telemetry. There is no in-tree router, no quality/cost/latency criteria, no fallback chain, and no routing policy. Multi-model choice exists only as a flat catalog of named **LLM profiles** from which the user picks one — a manual selector, not an automatic tier.

## Rating

**2/10 — Absent.**

Justification per rubric (1-3 = Absent, implicit, ad-hoc, or unsafe): quality-cost routing is architecturally absent from this repository by design (delegated to `software-agent-sdk`). The frontend implements only single-model selection with user-initiated switching and passive cost display. No routing criteria, fallback chain, tracing, or configurable policy exists in-tree. Score 2 rather than 1 because the delegation is explicit and documented (`AGENTS.md:31-36` repo map), cost observability is present, and profile switching has correct error handling — the absence is intentional isolation, not an unsafe ad-hoc attempt.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Multi-model config | Single default model constant — no tiers, no cost/quality metadata: `DEFAULT_SETTINGS.llm_model = "openhands/kimi-k3"` | `src/services/settings.ts:6` |
| Multi-model config | `agent_settings.llm.model` defaults to same single model: `llm: { model: "openhands/kimi-k3" }` | `src/services/settings.ts:39-41` |
| Multi-model config | Frontend never relies on backend default — always injects `DEFAULT_SETTINGS.llm_model` if `llm.model` is absent/whitespace (LLD-001). This is a correctness guard, not routing. | `src/api/agent-server-adapter.ts:890-893` |
| Multi-model config | LLM profile catalog is a flat list fetched via `ProfilesService.listProfiles()` backed by `ProfilesClient.listProfiles()`. Profiles hold `config.model` + `api_key_set` but no tier, cost, latency, or quality labels. | `src/api/profiles-service/profiles-service.api.ts:91-93` |
| Multi-model config | Free-model display map is a static 4-entry label map (`kimi-k3`, `glm-5.2`, `deepseek-v4-flash`, `minimax-m2.7`) — display-only, not a routing tier. | `src/utils/format-model-name.ts:3-8` |
| Multi-model config | `LLM_PROFILES_QUERY_KEYS` + `useLlmProfiles` caches the flat profile list per backend; no tier filter or ranking. | `src/hooks/query/use-llm-profiles.ts:15-22` |
| Router / routing criteria | No file matches `*router*` / `*routing*` except React Router navigation provider (`react-router-navigation-provider.tsx`) and test scaffold (`__tests__/router.md`) — unrelated to model routing. Grep for `routing|quality.*cost|cost.*rout|model.*tier|tier.*model` returned 0 hits in `src/`. | `src/routes/react-router-navigation-provider.tsx` (only router match) |
| Routing criteria | Grep for `cost|latency|quality.*rout` in `src/` returned only `accumulated_cost` metric plumbing and `cost` translation keys — no routing predicate. | `src/types/settings.ts:143`, `src/stores/metrics-store.ts:4` |
| Fallback chain | `useSwitchLlmProfile` calls `AgentServerConversationService.switchProfile(conversationId, profileName)` exactly once — no retry, no fallback model, no second attempt on failure. `onError` only shows a toast. | `src/hooks/mutation/use-switch-llm-profile.ts:57-68` |
| Fallback chain | `useSwitchAcpModel` calls `switchAcpModel` once; home-page persist path writes a single profile or `agent_settings_diff` — no fallback chain, failure propagates to global toast. | `src/hooks/mutation/use-switch-acp-model.ts:53-111` |
| Fallback chain | `AgentServerConversationService.switchProfile` for local backends: fetches encrypted profile, validates subscription auth, then calls `conversationClient.switchLLM(..., {model, stream:true, usage_id:...})` once. No fallback chain, no cost-aware re-try. | `src/api/conversation-service/agent-server-conversation-service.api.ts:891-909` |
| Fallback chain | `AgentServerConversationService.switchAcpModel` — single `switchAcpModel` call per invocation, cloud vs local branch only, no fallback. | `src/api/conversation-service/agent-server-conversation-service.api.ts:925-944` |
| Fallback chain | `fallback` string occurrences in `src/` are all for encrypted-settings or fallback UI (`React.Suspense fallback`, `fallbackSchema`, dead-profile fallback to `agent_settings`), not model quality-cost fallback. | `src/api/agent-server-adapter.ts:84`, `src/hooks/query/use-agent-settings-schema.ts:54` |
| Routing decision traces | `useModelStore` records only user-initiated switches (`recordSwitch`, `seedSwitches`) keyed by `profileName` + `anchorEventId` — no automatic routing decision, no cost/latency reason, no model-comparison log. | `src/stores/model-store.ts:92-104`, `src/hooks/chat/record-model-switch-message.ts:7-15` |
| Routing decision traces | `seedModelSwitchesFromHistory` replays `SwitchLLMObservationEvent` history into `ModelStore` for display after reload — trace is of manual switches, not policy decisions. | `src/hooks/chat/record-model-switch-message.ts:39-62` |
| Routing decision traces | `conversation-websocket-context.tsx` handles `SwitchLLMObservation` as a metadata update (`active_profile`), not a routing trace — carries only `profile_name` + `active_model`, no cost/quality rationale. | `src/contexts/conversation-websocket-context.tsx:695-718` |
| Routing policy config | `max_budget_per_task: number \| null` exists on `Settings` and `MetricsState` but is a single hard cap (`max_iterations`, `max_budget_per_task`) — it stops execution when exceeded, does not steer model choice. | `src/types/settings.ts:143`, `src/stores/metrics-store.ts:5` |
| Routing policy config | Budget display is read-only: `BudgetDisplay` / `BudgetProgressBar` / `BudgetUsageText` render `cost / maxBudget` with percentage. No slider or rule that downgrades/upgrades model based on budget. | `src/components/features/conversation-panel/budget-display.tsx:12-25`, `src/components/features/conversation-panel/budget-progress-bar.tsx:10-12` |
| Routing policy config | `specs/llm-defaults.md:5-8` LLD-001 mandates always sending explicit `DEFAULT_SETTINGS.llm_model` — policy is about default injection, not tier selection. | `specs/llm-defaults.md:5-8` |
| Cost observability | `accumulated_cost` + `max_budget_per_task` + token usage are normalized from `DirectConversationInfo.metrics` and combined via `combineUsageMetrics` for display — observability exists but is not wired to routing. | `src/api/conversation-service/agent-server-conversation-service.api.ts:130-138`, `src/utils/conversation-metrics.ts:17-34` |
| Configurability | LLM auth type exposes `api_key` vs `subscription` choice (`LLM_AUTH_TYPE_SUBSCRIPTION`) but this is a credential mode, not a quality-cost routing rule. | `src/constants/llm-subscription.ts:3-27` |

## Answers to Dimension Questions

**1. Are multiple model tiers available?**

No. The frontend exposes a flat catalog of named LLM profiles via `ProfilesService.listProfiles()` (`src/api/profiles-service/profiles-service.api.ts:91-93`). Each profile holds a `config.model` string (e.g., `openhands/kimi-k3`, `anthropic/claude-sonnet-*`) plus credential state. There is no tier concept (no `cheap/standard/premium`, no quality score, no latency SLA). The only model set constant is `FREE_OPENHANDS_MODELS` (`src/utils/format-model-name.ts:3-8`), a 4-entry display-label map, not a tier definition. The canonical default is a single value `DEFAULT_SETTINGS.llm_model = "openhands/kimi-k3"` (`src/services/settings.ts:6`). Users can create arbitrarily many profiles, but they are peer options requiring manual selection — the system never classifies them into cost/quality tiers.

**2. What criteria drive model selection?**

No automatic criteria. Selection is 100% manual and user-driven:
- At conversation creation, `useCreateConversation` resolves the active AgentProfile's `llm_profile_ref` (or falls back to the active LLM profile / `agent_settings`) — this is a pointer chase, not a criteria evaluation (`src/hooks/mutation/use-create-conversation.ts:109-214`). Active profile is chosen by the user in the LLM profile pill / profile manager.
- During a conversation, `useSwitchLlmProfile` swaps to whatever `profileName` the user clicked (`src/hooks/mutation/use-switch-llm-profile.ts:57-58`).
- ACP model switching (`src/hooks/mutation/use-switch-acp-model.ts:53-58`) likewise takes an explicit `model` string.
No latency, cost, risk, or quality heuristic is consulted. The `max_budget_per_task` setting (`src/types/settings.ts:143`) is a stop condition surfaced in `BudgetDisplay` (`src/components/features/conversation-panel/budget-display.tsx:22-25`), not a routing knob. There is no request-complexity estimator, no token-budget allocator, and no risk classifier.

**3. Are fallback chains defined?**

No. Grep for `fallback` in `src/` returned only unrelated uses (encrypted-settings fallback, `React.Suspense fallback`, ADK fallback schema — `src/api/agent-server-adapter.ts:84`, `src/hooks/query/use-agent-settings-schema.ts:54`). Both switching paths execute a single RPC:
- `ConversationClient.switchLLM()` with a generated `usage_id` (`src/api/conversation-service/agent-server-conversation-service.api.ts:901-908`) — no retry, no second model, no degraded-model ladder.
- `ConversationClient.switchAcpModel()` (`src/api/conversation-service/agent-server-conversation-service.api.ts:940-943`) — same shape.
Failures hit `onError` toast (`src/hooks/mutation/use-switch-llm-profile.ts:65-68`) or the global mutation error toast for ACP and do not trigger an automatic downgrade. The default-model fallback in `buildConfiguredOpenHandsAgentSettings` (`src/api/agent-server-adapter.ts:890-893`) is a null-guard injecting `DEFAULT_SETTINGS.llm_model`, not a quality-cost fallback chain. The `software-agent-sdk` may implement server-side fallback, but this frontend has no chain definition, no ordering, and no configuration for one.

**4. Are routing decisions observable?**

Only manual switch events are observable, and not as routing decisions. `ModelStore` (`src/stores/model-store.ts:29-61`) persists `ModelListEntry` / `SeededSwitch` records containing `anchorEventId`, `profileName`, and a timestamp implicit via event order. `recordModelSwitchMessage` (`src/hooks/chat/record-model-switch-message.ts:7-15`) and `seedModelSwitchesFromHistory` (`src/hooks/chat/record-model-switch-message.ts:39-62`) replay `SwitchLLMObservationEvent` entries into inline "Switched to {profile}" messages. These are traces of user actions, not of an autonomous router — they carry no routing rationale, no cost comparison, no latency measurement, and no rejected-alternative log. Cost itself is observable as `accumulated_cost` time-series on `MetricsState` (`src/stores/metrics-store.ts:4`) and via `CostSection` (`src/components/features/conversation/metrics-modal/cost-section.tsx:10`), but is never joined to a routing decision because no such decision exists here.

## Architectural Decisions

| Decision | Evidence | Consequence |
|----------|----------|-------------|
| Frontend/backend split — frontend owns UI, backend (software-agent-sdk) owns LLM execution & routing | `AGENTS.md:31-36` repo map: SDK owns "agents, tools, conversations, events, and the REST/WebSocket API surface"; frontend "only the agent-canvas frontend" | Quality-cost routing cannot be evaluated in this repo; any router lives out of scope. Frontend deliberately has no router code. |
| Single-model per conversation, injected explicitly (LLD-001) | `src/services/settings.ts:6`, `src/api/agent-server-adapter.ts:890-893`, `specs/llm-defaults.md:5-8` | Deterministic launch behavior; no implicit model resolution. But also no notion of "cheap for simple turns / expensive for hard turns" within one conversation. |
| Profile-as-selector pattern — profiles are named credential+model bundles, active profile is a pointer | `src/api/profiles-service/profiles-service.api.ts:91-93`, `src/hooks/mutation/use-create-conversation.ts:109-214`, `src/hooks/use-llm-configured.ts:70-102` | Gives users multi-model capability without in-tree tier logic. Downgrade for dangling `default` profile ref to `agent_settings` (`src/hooks/mutation/use-create-conversation.ts:185-192`) shows the indirection has edge cases but no fallback chain. |
| Budget as hard cap, not routing signal | `src/types/settings.ts:143`, `src/stores/metrics-store.ts:5`, `src/components/features/conversation-panel/budget-display.tsx:22-25`, `src/api/agent-server-adapter.ts:1122-1125` (`max_iterations:500`) | Cost observability exists; `max_budget_per_task` can abort or limit, but never triggers a model downgrade. |
| ACPAgent kind separation — `agent_kind: "openhands" \| "acp"` branches all LLM logic | `src/types/settings.ts:110`, `src/api/agent-server-adapter.ts:790-793`, `src/hooks/use-llm-configured.ts:67-68` | Correctly isolates ACP (CLI subprocess) from litellm routing; prevents cross-contamination but doubles switching surfaces with no shared routing abstraction. |
| `stream: true` forced on every litellm request | `src/api/agent-server-adapter.ts:897`, `src/api/conversation-service/agent-server-conversation-service.api.ts:905-906` | Uniform streaming behavior; no streaming-vs-non-streaming cost/latency tradeoff exposed to routing. |

## Notable Patterns

- **Thin-client trigger pattern** — The frontend never runs agent tools itself; it only sends `initial_message: {run:true}` and profile/model identifiers. All LLM calls, tool dispatches, and any cost-aware decisions happen server-side. Pattern is visible in `src/api/agent-server-adapter.ts:679-695` (`buildInitialMessage`) and `src/api/conversation-service/agent-server-conversation-service.api.ts:358-398` (`sendMessage` delegates to `ConversationClient` or `callCloudProxy`).
- **Encrypted-settings pass-through** — Secrets are never read client-side at conversation start; they are sent as `LookupSecret` URLs the server resolves (`src/api/agent-server-adapter.ts:1202-1228`). This correctly keeps credential routing out of frontend routing logic but also means the frontend cannot implement per-request cost-based credential switching even if it wanted to.
- **Store + seed replay for switch observability** — `ModelStore` + `seedModelSwitchesFromHistory` (`src/hooks/chat/record-model-switch-message.ts:39-62`) is the closest thing to routing traces: it replays hidden `SwitchLLMObservationEvent` entries into visible messages on reload. Pattern is purpose-built for manual switches, not for autonomous routing audit.
- **No router, no policy DSL, no interceptor** — Unlike sources that expose `FallbackModel`, `wrap_model_request`, or `tool_search` primitives, this codebase has zero routing types, predicates, or middleware. The `useSwitchLlmProfile` / `useSwitchAcpModel` hooks are plain CRUD mutations with no capability hook.

## Tradeoffs

| Tradeoff | Benefit | Cost |
|----------|---------|------|
| Hard frontend/backend boundary (routing lives in SDK) | Keeps frontend small, testable, and release-decoupled; avoids duplicating litellm logic | Quality-cost routing is unobservable and unevaluable in this repo; study must record absence and defer to sibling repo |
| Flat profile list vs tiered catalog | Users get unlimited model choices without framework-imposed categorization; `FREE_OPENHANDS_MODELS` (`src/utils/format-model-name.ts:3-8`) is a minimal affordance | No cheap-for-simple / expensive-for-hard optimization; users must manually know which model suits which task |
| Manual switch with single RPC, no fallback | Simple mental model; failure is loud (toast) rather than silent downgrade to an unexpected model | A bad model name or revoked key fails the switch with no graceful degrade to a cheaper/fallback model; conversation may stall until user retries |
| Budget cap decoupled from model choice | Prevents surprising model changes mid-conversation; cost control is explicit (`max_budget_per_task`) | Budget exhaustion has only one response (stop / stuck detection `stuck_detection:true` at `src/api/agent-server-adapter.ts:1126`) — no automatic cost-saving downgrade |
| Always-inject default model (LLD-001) | Prevents reliance on server-side default (`gpt-5.5`) shifting under users | Hides the case where a cheaper default would have been preferable for a trivial query |

## Failure Modes / Edge Cases

- **Dangling profile reference on launch** — The seeded `default` AgentProfile's `llm_profile_ref` can point at a deleted LLM profile. `useCreateConversation` detects this and silently downgrades to `agent_settings` launch (`src/hooks/mutation/use-create-conversation.ts:185-192` — `console.warn` + `effectiveAgentProfileId = undefined`). No user-visible error, but the conversation runs a different model/profile config than the user may expect. This is the sole fallback-like path in the routing surface, and it is a correctness workaround, not a quality-cost ladder.
- **Active LLM mismatch vs displayed pill** — When the home LLM pill's active profile differs from the AgentProfile's pinned ref, launch silently downgrades to `agent_settings` to keep display and execution consistent (`src/hooks/mutation/use-create-conversation.ts:197-214`). Again, correct but silent; the named profile's non-LLM config is dropped.
- **Switch failure is terminal for that turn** — `switchProfile` throws on missing model (`src/api/conversation-service/agent-server-conversation-service.api.ts:899` — `has no model`), failed subscription auth (`src/api/agent-server-adapter.ts:1248-1251`), or transport error. No retry, no fallback model. The `displayErrorToast` (`src/hooks/mutation/use-switch-llm-profile.ts:66-67`) is the only recovery signal.
- **Budget exhaustion has no degrade path** — `accumulated_cost` exceeding `max_budget_per_task` surfaces via `MetricsState` (`src/stores/metrics-store.ts:4`) and `BudgetProgressBar` (`src/components/features/conversation-panel/budget-progress-bar.tsx:12`) but triggers no model switch; the agent loop presumably halts (governed by `max_iterations:500` at `src/api/agent-server-adapter.ts:1122-1125` and `stuck_detection:true`).
- **ACP profile discovery failure propagates** — `useSwitchAcpModel` home-page path calls `ensureQueryData` for the active ACP profile; on failure it propagates rather than downgrading to `agent_settings` (`src/hooks/mutation/use-switch-acp-model.ts:69-74` comment — "#16523: an active-profile launch ignores agent_settings, so the pick would be silently dropped"). Correct but leaves the user with no switch at all.
- **No routing loop guard needed, but none exists** — Because there is no automatic router, there is also no routing loop, no cost runaway loop, and no routing circuit breaker. The `max_iterations` and budget caps are the only loop bounds.

## Future Considerations

- **If cost-aware routing is desired, it belongs in software-agent-sdk, not here** — This frontend's sanctioned extension point is `AgentSettingsPayload` + `agent_profile_id` at `src/api/agent-server-adapter.ts:1002-1010`. Adding a `routing_policy` field (e.g., `{ strategy: "cost_aware", tiers: [...], fallback_chain: [...] }`) to `agent_settings` would let the frontend configure a backend router without owning the router. No such field exists today; design must avoid front-running the SDK's litellm integration.
- **Frontend could surface tier metadata without owning routing** — Adding `tier`, `estimated_cost_per_1k`, or `quality_label` to `ProfileInfo` (`src/api/profiles-service/profiles-service.api.ts:50-54`) and rendering it in the profile picker / `formatModelNameForDisplay` (`src/utils/format-model-name.ts:27-36`) would let users make informed manual choices even before automatic routing exists. Requires contract change in `typescript-client`.
- **Phase-appropriate model hints** — Conversation-phase awareness (planning vs execution) could be a frontend hint (e.g., a directive tool or tag) without building a full router; assess whether `conversationInstructions` (`src/api/agent-server-adapter.ts:964`) is the right surface or whether a dedicated `routing_hint` tag is cleaner.
- **Observability gap to close when a router lands** — Any future router must emit structured traces joinable to `ModelStore` entries and `MetricsState.cost` (`src/stores/metrics-store.ts:21-25`, `src/stores/model-store.ts:29-38`) so cost/quality decisions are auditable. Today the switch messages are the only trace surface.
- **Determinism of default-model injection** — LLD-001 (`specs/llm-defaults.md:5-8`) is a de-facto routing decision ("always this default"). If defaults become tier-aware (e.g., different default per task complexity), the fallback at `src/api/agent-server-adapter.ts:890-893` needs a complexity signal or it will mask the intended tier.

## Questions / Gaps

- **No evidence for latency- or risk-based routing** — Grep across `src/` for latency, risk, quality-derived routing criteria returned 0 hits beyond cost metric plumbing. If the SDK implements such policies, they are not surfaced or configured through this frontend's `AgentSettingsPayload` (`src/api/agent-server-adapter.ts:421-425`).
- **Cannot verify fallback chains without inspecting `software-agent-sdk`** — Isolation rule forbids cross-source access. Whether the agent-server implements `FallbackModel`, provider retry, or litellm fallback is out of scope for this report. Frontend's `switchLLM` call (`src/api/conversation-service/agent-server-conversation-service.api.ts:901-908`) passes a single model config, implying no client-side chain.
- **Cost tracing granularity unknown server-side** — Frontend normalizes `accumulated_cost` (`src/api/conversation-service/agent-server-conversation-service.api.ts:130-138`) and `per_turn_token` but does not know if the SDK emits per-request cost breakdowns that could inform a router. Requires SDK inspection.
- **Profile validation does not assess cost/quality fitness** — `ProfilesService.validateProfile` (`src/api/profiles-service/profiles-service.api.ts:158-188`) checks only connectivity (204/404/429/5xx gating) via `client.validateProfile`, not cost efficiency or quality suitability of the model for a task class.
- **Is budget-aware downgrade on the roadmap?** — `max_budget_per_task` (`src/types/settings.ts:143`) and `max_iterations` (`src/services/settings.ts:15`) are the only resource bounds. No ADR or TODO was found describing a future "downgrade to cheaper model when budget low" policy in this repo.

---

Generated by `dimensions/20.03-quality-cost-routing.md` against `openhands`.

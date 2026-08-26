# Source Analysis: openhands

## Dimension 09.02 — Risk Taxonomy and Control Mapping

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (OpenHands agent-canvas frontend) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19, Vite, TanStack Query; talks to a Python agent-server via `@openhands/typescript-client` |
| Analyzed | 2026-08-26 |

## Summary

OpenHands' risk model is a **four-level action-risk taxonomy** (`UNKNOWN | LOW | MEDIUM | HIGH`, `src/types/agent-server/core/base/common.ts:58-64`) that is **assessed per-action** (per tool call), not per-tool or per-agent: every `ActionEvent` carries an LLM-predicted `security_risk` field (`src/types/agent-server/core/events/action-event.ts:46-61`). The control side is a **two-axis user setting** — a boolean `confirmation_mode` plus a `security_analyzer` selector (`src/types/settings.ts:128-129`) — which the frontend compiles into a server-enforced confirmation policy at conversation start (`getConversationConfirmationPolicy`, `src/api/agent-server-adapter.ts:593-605`): off → `NeverConfirm`; on + LLM analyzer → `ConfirmRisky` with threshold `HIGH` and `confirm_unknown: true`; on otherwise → `AlwaysConfirm`. The agent-server performs the actual gating and pauses the conversation (`ExecutionStatus.WAITING_FOR_CONFIRMATION`, `src/types/agent-server/core/base/common.ts:71`); the frontend surfaces the pending action's risk (HIGH-risk alert banner, per-action badges), blocks chat input while paused, and relays the user's accept/reject decision to `/api/conversations/{id}/events/respond_to_confirmation`. The mapping function is unit-tested, but the taxonomy has no documentation beyond code, the confirm threshold is hardcoded to HIGH with no per-risk-category controls for MEDIUM/LOW, and the adapter supports two analyzers (`pattern`, `policy_rail`) that the settings schema never exposes.

## Rating

**6 / 10** — A clear, typed, end-to-end risk taxonomy exists and the risk→control mapping is explicit and tested (`__tests__/api/agent-server-adapter.test.ts:349-376`), but it is coarse: one hardcoded threshold (HIGH), no distinct control for MEDIUM/LOW outcomes, adapter capabilities exceed what the settings schema offers, and actual enforcement lives in a separate backend repo, so this repo alone cannot demonstrate durable policy enforcement under failure.

## Evidence Collected

Every entry includes a file path with line numbers relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Risk enum (taxonomy) | `enum SecurityRisk { UNKNOWN, LOW, MEDIUM, HIGH }` under comment "Security risk levels" | `src/types/agent-server/core/base/common.ts:58-64` |
| Per-action risk assessment | `ActionEvent.security_risk`: "The LLM's assessment of the safety risk of this action"; doc note says `tool_call.security_risk` is "predicted by LLM when LLM risk analyzer is enabled" | `src/types/agent-server/core/events/action-event.ts:42-49, 58-61` |
| Risk assessed at tool-call level, not tool level | Risk lives on the event/tool_call copy, not on any tool definition; client tools instead declare MCP-style hints (`readOnlyHint/destructiveHint/idempotentHint`) noting "the agent is asked to predict a security risk before every call" | `src/api/launch-child-conversation-client-tool.ts:103-111` |
| Control setting #1 | `confirmation_mode: boolean` on `Settings` | `src/types/settings.ts:128` |
| Control setting #2 | `security_analyzer: string \| null` on `Settings` | `src/types/settings.ts:129` |
| Defaults | `confirmation_mode: false`, `security_analyzer: "llm"` (both flat and nested `conversation_settings`) | `src/services/settings.ts:13-14, 54-58` |
| Risk→control mapping (core) | `getConversationConfirmationPolicy()`: not enabled → `{kind:"NeverConfirm"}`; `security_analyzer==="llm"` → `{kind:"ConfirmRisky", threshold:"HIGH", confirm_unknown:true}`; else → `{kind:"AlwaysConfirm"}` | `src/api/agent-server-adapter.ts:593-605` |
| Analyzer kind mapping | `getConversationSecurityAnalyzer()`: `"llm"→LLMSecurityAnalyzer`, `"pattern"→PatternSecurityAnalyzer`, `"policy_rail"→PolicyRailSecurityAnalyzer`, default `undefined` | `src/api/agent-server-adapter.ts:607-618` |
| Policy enforcement point (handoff) | Conversation-start payload always carries `confirmation_policy:` from the mapper; `security_analyzer` attached only when non-null | `src/api/agent-server-adapter.ts:1120-1121, 1169-1173` |
| Runtime state signal | Server-side pause surfaces as `ExecutionStatus.WAITING_FOR_CONFIRMATION` | `src/types/agent-server/core/base/common.ts:71` |
| Status→UI-state bridge | `WAITING_FOR_CONFIRMATION → AgentState.AWAITING_USER_CONFIRMATION` | `src/hooks/use-agent-state.ts:24-25` |
| Runtime exposure of risk metadata | Confirmation buttons read last agent event's `security_risk` (`isActionEvent` guard, UNKNOWN fallback) and render a red alert only when `risk === SecurityRisk.HIGH` | `src/components/shared/buttons/conversation-confirmation-buttons.tsx:102-118` |
| Alert component scope | `RiskAlert` renders only `severity === "high"` ("Currently, we are only supporting the high risk alert") | `src/components/shared/risk-alert.tsx:19-35` |
| Action-card risk badge | Bash visualizer shows "Risk: High"/"Risk: Medium" labels for `HIGH`/`MEDIUM` actions | `src/components/features/chat/tool-visualizers/bash/bash.tsx:24-37` |
| Transcript risk text | `getExecuteBashActionContent()` appends risk text for HIGH/MEDIUM only; full LOW/MEDIUM/HIGH/UNKNOWN label switch in `getRiskText` | `src/components/conversation-events/chat/event-content-helpers/get-action-content.ts:30-42, 92-98` |
| Client-side guardrail while gated | Chat input disabled when `AgentState.AWAITING_USER_CONFIRMATION` | `src/components/features/chat/interactive-chat-box.tsx:62-65` |
| Decision API | `EventService.respondToConfirmation` POSTs `{accept, reason?}` to `/api/conversations/{id}/events/respond_to_confirmation` (cloud path via `callCloudProxy`, local via typed client) | `src/api/event-service/event-service.api.ts:40-68`; request/response types `src/api/event-service/event-service.types.ts:1-8` |
| Decision mutation hook | `useRespondToConfirmation()` builds the request; duplicate submissions guarded by `submittedEventIds` store; keyboard shortcuts Cmd+Enter accept / Shift+Cmd+Backspace reject | `src/hooks/mutation/use-respond-to-confirmation.ts:12-32`; `src/components/shared/buttons/conversation-confirmation-buttons.tsx:44-47, 60-90` |
| Settings UI surface | Verification page renders conversation-owned `verification` section and de-dupes deprecated agent-owned `verification.confirmation_mode`/`verification.security_analyzer` fields | `src/routes/verification-settings.tsx:4-12, 23-40` |
| Schema shape of controls | Mock schema: `confirmation_mode` boolean default false; `security_analyzer` string default `"llm"` with choices `llm`/`none` and `depends_on: ["confirmation_mode"]` | `src/mocks/settings-handlers.ts:404-441` |
| Settings persistence merge | Local and cloud settings services round-trip `confirmation_mode`/`security_analyzer` through diffs | `src/api/settings-service/settings-service.api.ts:401-409`; `src/api/cloud/settings-service.api.ts:110-117` |
| Mapping test | "derives confirmation and security settings the same way as OpenHands" asserts `ConfirmRisky`+`LLMSecurityAnalyzer` payload | `__tests__/api/agent-server-adapter.test.ts:349-376` |
| UI tests for controls | Advanced-view visibility of both controls; legacy-field de-duplication test | `__tests__/routes/verification-settings.test.tsx:126-149, 151-230` |
| Adjacent resource controls | `max_iterations` (default 500) and unconditional `stuck_detection: true` in start payload; `max_budget_per_task` surfaced via metrics store | `src/api/agent-server-adapter.ts:1122-1126`; `src/stores/metrics-store.ts:5`; `src/types/settings.ts:143` |

## Answers to Dimension Questions

**1. Are risks named and categorized?**
Yes. A single four-value enum `SecurityRisk { UNKNOWN, LOW, MEDIUM, HIGH }` (`src/types/agent-server/core/base/common.ts:58-64`) is the canonical taxonomy. It is typed end-to-end: server event type (`src/types/agent-server/core/events/action-event.ts:61`), UI badge rendering (`src/components/features/chat/tool-visualizers/bash/bash.tsx:29-37`), transcript text (`src/components/conversation-events/chat/event-content-helpers/get-action-content.ts:30-42`), and localized labels for all four levels exist (`SECURITY$LOW_RISK`…`SECURITY$UNKNOWN_RISK` in `src/i18n/translation.json:563-626`). There is no sub-categorization (no e.g. "filesystem vs network vs secret-exfiltration" axes); severity is the only dimension.

**2. Is every risk mapped to a control?**
Partially. The control mapping is expressed as a single compile-time table inside `getConversationConfirmationPolicy` (`src/api/agent-server-adapter.ts:593-605`): enabling verification maps *any* risk ≥ HIGH (and UNKNOWN, via `confirm_unknown: true`) to the "pause for human confirmation" control; everything below the threshold gets presentation-only treatment (badges/transcript text). MEDIUM actions receive a visual label but no distinct control; LOW and UNKNOWN receive nothing beyond optional transcript text. So the mapping covers all named levels, but three of four levels map to the same trivial control (display), and the threshold itself is hardcoded rather than configurable.

**3. Can risks be assessed at runtime?**
Yes — but by the server/LLM, not the frontend. Risk is predicted per action at runtime by the LLM security analyzer and travels on each `ActionEvent` (`src/types/agent-server/core/events/action-event.ts:46-61`); the frontend consumes it live from the event stream to render badges and gate the confirmation prompt (`conversation-confirmation-buttons.tsx:102-107`). The analyzer *kind* is selected once per conversation at creation time (`src/api/agent-server-adapter.ts:1169-1173`), so assessment strategy is fixed for a conversation's lifetime. No evidence was found of frontend-side risk re-evaluation or of mid-conversation analyzer changes.

**4. Can controls be bypassed?**
The enforcement itself cannot be bypassed *from this repo* because it is server-side: the frontend never executes tools and can only relay accept/reject to `respond_to_confirmation` (`src/api/event-service/event-service.api.ts:40-68`). However, **policy selection is client-authored**: `buildStartConversationRequest` serializes whatever `confirmation_mode`/`security_analyzer` values the client holds into `confirmation_policy`/`security_analyzer` payload fields (`src/api/agent-server-adapter.ts:1120-1121, 1169-1173`), so a modified client (or direct API caller, as the live-test helper does at `tests/e2e/live/utils/agent-server-conversation.ts:243-245`) can request `{"kind":"NeverConfirm"}` outright. Whether the server validates these fields against user permissions is unobservable here (backend lives in `software-agent-sdk`, outside this source). Two softer weaknesses: the awaiting-action lookup selects the last agent event purely from global agent state rather than verifying the event actually awaits confirmation (`conversation-confirmation-buttons.tsx:30-36`), and duplicate-submission protection is client-local only (`submittedEventIds`, lines 17-22).

## Architectural Decisions

- **Per-action risk over per-tool risk.** Risk is attached to each `ActionEvent`/tool_call predicted by an LLM analyzer, rather than a static classification of tools. Tool definitions carry only MCP-style behavioral hints (`readOnlyHint`, `destructiveHint`, `idempotentHint`) that feed the prediction requirement (`src/api/launch-child-conversation-client-tool.ts:103-111`). Tradeoff: context-sensitive accuracy at the cost of no upfront guarantee about what a given tool may do.
- **Policy compiled client-side, enforced server-side.** The frontend owns the UX-facing boolean/analyzer settings and translates them into the SDK's discriminated-union policy objects at conversation start (`src/api/agent-server-adapter.ts:593-618`); the agent-server owns execution gating and pause semantics (`common.ts:71`). This keeps the harness thin but means the trust boundary sits at payload construction.
- **UNKNOWN treated as risky.** `confirm_unknown: true` (`agent-server-adapter.ts:601`) makes fail-safe the default posture when the LLM analyzer declines to classify an action.
- **Canonical settings ownership migrated to conversation scope.** The SDK deprecated agent-owned `verification.confirmation_mode`/`verification.security_analyzer`; the UI deliberately renders only conversation-settings copies and filters legacy duplicates (`src/routes/verification-settings.tsx:4-12`).
- **Presentation tiering by severity.** HIGH gets a dedicated red alert banner component; MEDIUM gets inline text badges; the component explicitly documents that only high is supported today (`src/components/shared/risk-alert.tsx:19`).

## Notable Patterns

- **Discriminated-union policy objects** (`{kind: "NeverConfirm"|"ConfirmRisky"|"AlwaysConfirm"}` and analyzer kinds) mirror the Python SDK's tagged unions — consistent with the repo-wide Rule 3 style of literal-typed unions (AGENTS.md "No Magic Strings").
- **Single-source mapping functions**: both mappings are pure functions of a `SettingsRecord`, making them trivially unit-testable; the test name even pins parity intent — "derives confirmation and security settings the same way as OpenHands" (`__tests__/api/agent-server-adapter.test.ts:349`).
- **Schema-driven settings UI** with dependency ordering (`depends_on: ["confirmation_mode"]` gates the analyzer selector, `src/mocks/settings-handlers.ts:437`) and prominence-based visibility, verified in `__tests__/routes/verification-settings.test.tsx:126-149`.
- **State-machine-driven gating**: the same `AgentState.AWAITING_USER_CONFIRMATION` value simultaneously disables chat input (`interactive-chat-box.tsx:62-65`), triggers notifications, and reveals confirmation buttons — one signal, multiple consumers.

## Tradeoffs

- **Simplicity vs granularity.** One threshold (HIGH) and one control (pause-and-confirm) keep the model explainable, but MEDIUM-risk actions execute without any interactive control, and there is no way to express "confirm only for terminal commands" or other scoped rules.
- **Client-authored policy vs tamper resistance.** Putting `confirmation_policy` assembly in the browser simplifies the server contract but means the effective safety posture is whatever the requesting client serialized; no integrity mechanism is visible in this repo.
- **LLM-judged risk vs determinism.** The default analyzer outsources risk judgment to the same class of model performing the action; `confirm_unknown: true` mitigates silent misses but the repo contains no pattern-based fallback logic (the `PatternSecurityAnalyzer` kind exists in the adapter yet nothing in this repo implements or configures patterns).
- **Frontend-only observability.** Risk visibility (badges, alerts) exists only where events render; exports/transcripts include risk text for bash commands (`get-action-content.ts:92-98`) but there is no audit log surface dedicated to risk decisions in this repo.

## Failure Modes / Edge Cases

- **Analyzer/schema drift.** The adapter recognizes `pattern` and `policy_rail` analyzers (`src/api/agent-server-adapter.ts:611-614`), but the settings schema choices exposed to users are only `llm`/`none` (`src/mocks/settings-handlers.ts:433-436`). Values arriving from older/cloud settings (`src/api/cloud/settings-service.api.ts:31,114-117`) could select analyzers the current schema doesn't advertise — behavior depends entirely on server acceptance.
- **Mis-targeted confirmation prompt.** While paused, the "awaiting action" is found as simply the last agent-sourced event in the store (`conversation-confirmation-buttons.tsx:30-36`); if the trailing event isn't the gated action (e.g., interleaved hook/error events), accept/reject still submits for it. Duplicate-click races are handled only client-side via `submittedEventIds` (lines 44-47).
- **Non-action events during confirmation** degrade to `SecurityRisk.UNKNOWN` (lines 103-105): buttons render without the high-risk banner, which is correct visually but silently loses the real risk if the event shape is unexpected.
- **Silent no-op alerts.** `RiskAlert` returns `null` for medium/low severities (`risk-alert.tsx:35`); callers passing lower severities get no feedback that the alert was dropped.
- **Stuck/error states collapse.** `STUCK` maps to `ERROR` in the UI bridge (`use-agent-state.ts:31`), so a stuck loop near a risky action presents identically to a hard failure — obscuring whether a confirmation gate was involved.

## Future Considerations

- Expose the confirm threshold and per-analyzer configuration through the settings schema so `ConfirmRisky.threshold` stops being a hardcoded literal (`agent-server-adapter.ts:601`), or document why HIGH-only is intentional.
- Reconcile adapter analyzer support (`pattern`, `policy_rail`) with the schema choices (`llm`, `none`) or delete dead branches.
- Add a distinct control for MEDIUM (e.g., batched review or annotation requirement) so the taxonomy levels map to differentiated responses rather than display-only.
- Consider server-side validation/authorization of `confirmation_policy` downgrades, since the current architecture lets any API caller send `NeverConfirm` (as the live-test helper legitimately does at `tests/e2e/live/utils/agent-server-conversation.ts:243-245`).
- Track pending-confirmation explicitly on the event (e.g., a flag) instead of inferring from global agent state + last-event heuristic.

## Questions / Gaps

- **Where does the server enforce the policy?** No answer possible within this source: the agent-server implementation lives in the separate `software-agent-sdk` repository (per `AGENTS.md` repo map). This report can only certify the handoff contract (`agent-server-adapter.ts:1120-1121`).
- **Is `confirm_unknown` surfaced anywhere in the UI?** Searched `src/` for `confirm_unknown`: only the payload construction site matches. Users are not told that UNKNOWN actions also prompt.
- **What do `PatternSecurityAnalyzer`/`PolicyRailSecurityAnalyzer` do?** No implementation, tests, or docs in this repo beyond the two `kind` strings (`agent-server-adapter.ts:611-614`); searched `docs/` (no risk/confirmation content in `docs/architecture.md`) and the whole tree for `PolicyRail` — no further evidence found.
- **Was the confirmation flow covered by E2E tests?** Searched `tests/e2e/mock-llm/` and `tests/e2e/live/`: no spec exercises the AWAITING_USER_CONFIRMATION flow or the respond_to_confirmation endpoint; coverage is limited to unit/component tests cited above.

---

Generated by `Dimension 09.02: Risk Taxonomy and Control Mapping` against `openhands`.

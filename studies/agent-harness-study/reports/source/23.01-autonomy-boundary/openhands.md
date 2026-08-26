# Source Analysis: openhands

## Dimension 23.01 — Autonomy Boundary

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19 (Vite), "OpenHands agent-canvas" frontend; talks to a Python `software-agent-sdk` agent-server via `@openhands/typescript-client` |
| Analyzed | 2026-08-26 |

## Summary

This source is the OpenHands **frontend** (`AGENTS.md` repo map: this repo owns only the React/TypeScript agent-canvas; the agent loop and its enforcement live in the sibling `software-agent-sdk`). Within that boundary, the autonomy model is a **user-selected gating policy compiled into an explicit wire contract at conversation start**, plus a reactive UI escalation surface when the backend pauses for review:

1. Two user-facing knobs — `confirmation_mode: boolean` and `security_analyzer: string | null` — are declared on the `Settings` type (`src/types/settings.ts:128-129`) with defaults `confirmation_mode: false`, `security_analyzer: "llm"` (`src/services/settings.ts:13-14`). The shipped default is therefore **fully autonomous execution**.
2. At conversation start the frontend compiles those knobs into an explicit `confirmation_policy` object on the request payload (`src/api/agent-server-adapter.ts:1120-1121`): `NeverConfirm` when confirmation mode is off, `ConfirmRisky { threshold: "HIGH", confirm_unknown: true }` when the LLM analyzer is selected, and `AlwaysConfirm` otherwise (`src/api/agent-server-adapter.ts:593-605`). A matching `security_analyzer` field maps to `LLMSecurityAnalyzer` / `PatternSecurityAnalyzer` / `PolicyRailSecurityAnalyzer` (`src/api/agent-server-adapter.ts:607-618`). Enforcement of these policies is server-side (out of this source's scope by design — `AGENTS.md` repo map).
3. When the backend pauses, the frontend escalates through several channels: a confirm/reject pair bound to the `AWAITING_USER_CONFIRMATION` state with a high-risk banner (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:93-135`), chat-input lockout while confirmation is pending (`src/components/features/chat/interactive-chat-box.tsx:62-65`), a status line (`src/utils/status.ts:107-108`), a tab-title emoji (`src/utils/agent-state-emoji.ts:26-28`), and an optional sound notification (`src/hooks/use-agent-notification.ts:6-10`).
4. Risk metadata travels on every action event as `security_risk: SecurityRisk` (`UNKNOWN | LOW | MEDIUM | HIGH`, `src/types/agent-server/core/base/common.ts:59-64`; field on `ActionEvent`, `src/types/agent-server/core/events/action-event.ts:58-61`), so the UI can differentiate *what* it is asking the user to approve.

The autonomy boundary is configurable and typed, with tests covering the policy compilation, but documentation is thin (behavioral descriptions live mostly in i18n strings and mock schema data), automations expose no gating surface at all, and enforcement correctness cannot be observed inside this source.

## Rating

**6 / 10** — Present and reasonably well-engineered, but not yet durable.

Why not higher (7–8 requires "clear model with tests, explicit interfaces, and operational safeguards" throughout):

- There *is* a clear model (three named policy kinds, four risk levels) with explicit interfaces (`confirmation_policy` payload field, `respond_to_confirmation` endpoint) and tests pinning the mapping (`__tests__/api/agent-server-adapter.test.ts:349-376`).
- But autonomy decisions are weakly documented: no README/docs coverage of confirmation mode (`grep "confirmation|Confirmation" README.md` → no matches); behavioral copy exists only in i18n strings (`SETTINGS$CONFIRMATION_MODE_TOOLTIP`, `src/i18n/translation.json:11477`) and MSW mock schema descriptions (`src/mocks/settings-handlers.ts:408-411`). Legacy `INVARIANT$ASK_CONFIRMATION_RISK_SEVERITY_LABEL` i18n keys describing per-severity thresholds are dead code (no usage outside `src/i18n/translation.json:8944-8961`).
- Automations — the most autonomous mode in the product (cron/event-triggered headless runs) — carry no confirmation-policy or gating field in their frontend contract (`src/types/automation.ts:24-46`), so the strongest autonomy surface has the weakest boundary.
- Enforcement lives in another repo, so "does the system respect autonomy boundaries?" cannot be fully verified from this source alone; the frontend evidence shows intent and transport, not guarantees.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Autonomy settings knobs | `Settings.confirmation_mode: boolean`, `security_analyzer: string \| null` on the user-settings contract | `src/types/settings.ts:128-129` |
| Defaults (autonomous-by-default) | `DEFAULT_SETTINGS` sets `confirmation_mode: false`, `security_analyzer: "llm"` | `src/services/settings.ts:13-14,56-57` |
| Policy compilation → wire format | `getConversationConfirmationPolicy()` returns `{kind:"NeverConfirm"}` / `{kind:"ConfirmRisky", threshold:"HIGH", confirm_unknown:true}` / `{kind:"AlwaysConfirm"}` | `src/api/agent-server-adapter.ts:593-605` |
| Policy attached to start payload | `confirmation_policy:` set in `buildStartConversationRequest` payload | `src/api/agent-server-adapter.ts:1120-1121` |
| Analyzer selection | `getConversationSecurityAnalyzer()` maps `llm`→LLMSecurityAnalyzer, `pattern`→PatternSecurityAnalyzer, `policy_rail`→PolicyRailSecurityAnalyzer | `src/api/agent-server-adapter.ts:607-618`; attached at `1169-1173` |
| Payload type declares boundary fields | `StartConversationPayload.confirmation_policy: SettingsRecord`, optional `security_analyzer` | `src/api/agent-server-adapter.ts:1002-1023` |
| Risk-level vocabulary | `SecurityRisk` enum UNKNOWN/LOW/MEDIUM/HIGH | `src/types/agent-server/core/base/common.ts:59-64` |
| Risk carried per action | `ActionEvent.security_risk` ("The LLM's assessment of the safety risk of this action"), predicted when LLM risk analyzer enabled | `src/types/agent-server/core/events/action-event.ts:46-61` |
| Paused-for-review states | `ExecutionStatus.WAITING_FOR_CONFIRMATION`; `AgentState.AWAITING_USER_CONFIRMATION/USER_CONFIRMED/USER_REJECTED` | `src/types/agent-server/core/base/common.ts:71`; `src/types/agent-state.tsx:12-14` |
| Status→state mapping | `WAITING_FOR_CONFIRMATION` → `AgentState.AWAITING_USER_CONFIRMATION` | `src/hooks/use-agent-state.ts:24-25` |
| Human gate UI | Confirm/reject buttons rendered only in `AWAITING_USER_CONFIRMATION`, dedup via `submittedEventIds`, keyboard shortcuts (⌘↩ accept, ⇧⌘⌫ reject) | `src/components/shared/buttons/conversation-confirmation-buttons.tsx:38-100,61-90` |
| High-risk escalation banner | `RiskAlert` rendered only when `security_risk === SecurityRisk.HIGH` | `src/components/shared/buttons/conversation-confirmation-buttons.tsx:102-118`; component only implements severity "high": `src/components/shared/risk-alert.tsx` |
| Chat input locked during gate | `isDisabled` includes `curAgentState === AgentState.AWAITING_USER_CONFIRMATION` | `src/components/features/chat/interactive-chat-box.tsx:62-65` |
| Response endpoint | `EventService.respondToConfirmation()` POSTs `/api/conversations/{id}/events/respond_to_confirmation` (cloud via proxy; local via typed client) | `src/api/event-service/event-service.api.ts:40-69` |
| Request/response shape | `ConfirmationResponseRequest { accept: boolean; reason?: string }` | `src/api/event-service/event-service.types.ts:1-8` |
| Mutation hook | `useRespondToConfirmation` wraps the endpoint | `src/hooks/mutation/use-respond-to-confirmation.ts` |
| Escalation: attention signals | Sound notification on entering `AWAITING_USER_CONFIRMATION`; ✅ emoji prefix for `WAITING_FOR_CONFIRMATION` tab title | `src/hooks/use-agent-notification.ts:6-10`; `src/utils/agent-state-emoji.ts:26-28`; status label `AGENT_STATUS$WAITING_FOR_USER_CONFIRMATION` at `src/utils/status.ts:107-108` |
| Inline risk labeling in transcript | Bash visualizer flags HIGH/MEDIUM commands; bash content appends risk text for MEDIUM/HIGH | `src/components/features/chat/tool-visualizers/bash/bash.tsx:29-37`; `src/components/conversation-events/chat/event-content-helpers/get-action-content.ts:30-42,87-101` |
| Configurability: schema-driven UI | Verification page renders backend-provided `conversation_settings_schema["verification"]` fields; legacy agent-owned duplicates excluded | `src/routes/verification-settings.tsx:9-40` |
| Schema field definitions (mock of backend schema) | `confirmation_mode` boolean (default false, prominence major); `security_analyzer` choice llm/none (default llm, depends_on confirmation_mode) | `src/mocks/settings-handlers.ts:404-443` |
| Settings persistence merge | `confirmation_mode`/`security_analyzer` hoisted from conversation_settings into flat Settings | `src/api/settings-service/settings-service.api.ts:401-409`; cloud variant `src/api/cloud/settings-service.api.ts:30-31,110-117` |
| Tool-level capability gating | Browser tools gated by env flag + server-advertised `usable_tools`; sub-agent tool gated by `enable_sub_agents === true` | `src/api/agent-server-adapter.ts:631-644` |
| Tests: policy mapping pinned | Test asserts ConfirmRisky/HIGH/confirm_unknown + LLMSecurityAnalyzer payload | `__tests__/api/agent-server-adapter.test.ts:349-376` |
| Tests: settings-page behavior | Confirmation controls hidden in basic view, shown in advanced view; legacy field dedup asserted | `__tests__/routes/verification-settings.test.tsx:98-149,151-229` |
| Headless automations have no gate | `Automation` interface has trigger/enabled/timeout/prompt but no confirmation-policy field | `src/types/automation.ts:24-46` |
| Documentation posture | Only generic workspace-access risk note in docs; nothing about confirmation mode | `docs/architecture.md:80-82` |

## Answers to Dimension Questions

**1. What determines agent autonomy?**
A single user-facing boolean (`settings.confirmation_mode`, `src/types/settings.ts:128`) refined by an analyzer choice (`settings.security_analyzer`, `src/types/settings.ts:129`). The combination is compiled deterministically into one of three policy kinds at conversation start (`src/api/agent-server-adapter.ts:593-605`). Secondary, coarser autonomy bounds exist at tool level: whether the browser toolset or sub-agent delegation tool is offered at all depends on env flags, server-advertised capabilities, and the `enable_sub_agents` setting (`src/api/agent-server-adapter.ts:631-644`). Runtime autonomy is decided server-side against the transmitted policy; the frontend's role ends at declaring and transmitting it, then reacting to `WAITING_FOR_CONFIRMATION` (`src/hooks/use-agent-state.ts:24-25`).

**2. Are autonomy levels configurable?**
Yes, but coarsely — effectively three discrete levels: never-confirm (default), always-confirm, and risk-threshold-confirm at HIGH with unknown-risk actions also gated (`confirm_unknown: true`, `src/api/agent-server-adapter.ts:601`). Configuration is persisted server-side and merged back into flat settings (`src/api/settings-service/settings-service.api.ts:401-409`). The UI is schema-driven from the backend (`conversation_settings_schema`, consumed at `src/routes/verification-settings.tsx:23-39`); the adapter supports `pattern` and `policy_rail` analyzers (`src/api/agent-server-adapter.ts:611-614`) but the exposed schema choices are only `llm`/`none` (`src/mocks/settings-handlers.ts:433-436`). Notably there is no per-tool, per-conversation (at launch time), or numeric-threshold configurability in this source, and the MEDIUM threshold seen in transcript rendering (`get-action-content.ts:94-95`) is display-only, not a selectable gate.

**3. Are boundaries documented?**
Weakly. User-visible copy exists: "Pause for confirmation before the agent performs high-risk actions." (mock schema description, `src/mocks/settings-handlers.ts:410-411`) and tooltip keys (`SETTINGS$CONFIRMATION_MODE_TOOLTIP`, `src/i18n/translation.json:11477`). But neither `README.md` nor `docs/` explains the confirmation model (`docs/architecture.md:82` covers only general workspace-access hardening). Developer-facing rationale survives mainly in code comments (e.g., the deprecation note that `confirmation_mode`/`security_analyzer` moved from agent_settings to conversation_settings, `src/routes/verification-settings.tsx:4-8`). Legacy unused i18n keys referencing per-risk-severity confirmation thresholds (`INVARIANT$…`, `src/i18n/translation.json:8944-8985`) suggest an earlier, richer autonomy UI whose documentation was removed with the feature.

**4. Does the system respect autonomy boundaries?**
Within this source: the frontend respects them structurally — it cannot bypass the gate because approval flows exclusively through one mutation hitting the backend's `respond_to_confirmation` endpoint (`src/api/event-service/event-service.api.ts:40-69`), chat input is disabled while the gate is active (`interactive-chat-box.tsx:62-65`), and duplicate submissions are guarded (`conversation-confirmation-buttons.tsx:44-47,92-100`). Whether the agent-server actually halts before risky actions cannot be verified here — enforcement is in `software-agent-sdk` (per the repo-boundary rules in `AGENTS.md`). One structural exception: headless automations run on cron/event triggers with no confirmation-policy surface anywhere in the frontend contract (`src/types/automation.ts:24-46`), so the most autonomous execution mode operates entirely outside this gating system as far as this source shows.

## Architectural Decisions

- **Policy-as-data over client-side enforcement.** The frontend never decides which actions to block; it transmits a declarative `confirmation_policy` (`NeverConfirm`/`ConfirmRisky`/`AlwaysConfirm`, `src/api/agent-server-adapter.ts:593-605`) and treats the backend as the single enforcer. This keeps the trust boundary at the server but means frontend tests verify serialization, not safety (`__tests__/api/agent-server-adapter.test.ts:368-375`).
- **Conservative default under LLM analysis.** With the default analyzer, even UNKNOWN-risk actions require confirmation (`confirm_unknown: true`, `src/api/agent-server-adapter.ts:601`) — the design assumes the risk classifier can be wrong. Yet the top-level default remains `confirmation_mode: false` (`src/services/settings.ts:13`), i.e., the product ships fully autonomous and users must opt into oversight.
- **Schema-driven settings ownership migration.** Confirmation controls moved from agent-owned to conversation-owned settings, with explicit de-duplication of legacy agent-side copies (`CONVERSATION_OWNED_AGENT_VERIFICATION_FIELD_KEYS`, `src/routes/verification-settings.tsx:9-12`), showing the boundary definition itself has versioned across releases.
- **Multi-channel escalation rather than modal blocking.** Instead of a modal dialog, pending confirmation manifests as inline buttons plus passive channels (title emoji, sound, status text) — chosen to keep long-running sessions observable without forcing interaction (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:109-134`; `src/utils/agent-state-emoji.ts:26-28`).

## Notable Patterns

- **Discriminated-union policy kinds** (`{ kind: "NeverConfirm" }` etc.) mirroring the SDK's Python models — same pattern used for analyzers (`src/api/agent-server-adapter.ts:597-604,610-614`).
- **Risk surfaced contextually**: the same `security_risk` field drives three independent renderings — pre-execution banner (HIGH only), inline transcript labels (MEDIUM/HIGH), and the confirmation card (`bash.tsx:29-37`; `get-action-content.ts:93-98`; `conversation-confirmation-buttons.tsx:107-118`).
- **Idempotency guard on human decisions**: `submittedEventIds` in the event-message store prevents double-firing accept/reject for the same awaiting event (`conversation-confirmation-buttons.tsx:17-22,44-47`).
- **Keyboard-first approval**: ⌘↩ to accept, ⇧⌘⌫ to reject (`conversation-confirmation-buttons.tsx:66-78`) — deliberate ergonomics for a gate users hit often.
- **Capability-gated toolsets**: autonomy scope narrowed before launch by omitting tools entirely (browser tools off unless advertised and enabled; sub-agents off unless `enable_sub_agents === true`, `src/api/agent-server-adapter.ts:631-659`).

## Tradeoffs

- **Autonomous-by-default vs safe-by-default.** Shipping with `confirmation_mode: false` (`src/services/settings.ts:13`) maximizes flow but means unreviewed command execution is the default posture; the alternative (opt-out gating) would trade convenience for safety.
- **Single global toggle, coarse granularity.** One boolean + one analyzer choice covers all conversations and all tools; there is no per-project or per-tool override in this source. Users wanting "always confirm bash but not file edits" have no lever.
- **Progressive disclosure hides the gate.** `confirmation_mode` is major-prominence and hidden in the basic verification view (`__tests__/routes/verification-settings.test.tsx:98-101`), reducing discoverability for exactly the users least likely to know it exists.
- **Speed vs deliberation in approvals.** One-keystroke accept (⌘↩) makes confirming frictionless; combined with a heuristic that any last agent event counts as "the action awaiting confirmation," rapid approval could rubber-stamp unintended commands.

## Failure Modes / Edge Cases

- **Loose awaiting-action heuristic.** The "action being confirmed" is found by scanning reversed events and returning the last agent-sourced event whenever the state is `AWAITING_USER_CONFIRMATION`; the predicate ignores the event itself (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:30-36`). If the newest agent event is not the gated action (e.g., interleaved events after a reconnect/replay), the wrong event's id/risk is displayed and submitted.
- **Risk shown only at the moment of the gate.** If the user misses the transient banner, post-hoc transcript cards still show MEDIUM/HIGH labels (`bash.tsx:29-37`) but LOW/UNKNOWN actions execute with no visible annotation beyond default cards.
- **Reject-without-reason.** The request type accepts an optional `reason` (`event-service.types.ts:1-4`) but the UI never sends one (`useRespondToConfirmation` passes only `accept`), so agents receive rejection with no explanation channel from this client.
- **Stuck ≠ distinct handling.** `ExecutionStatus.STUCK` is folded into ERROR for both state mapping and notifications (`use-agent-state.ts:30-31`), so a wedged loop escalates identically to a crash rather than prompting intervention earlier.
- **Automations bypass the gate entirely** (as far as this source exposes): scheduled runs carry no confirmation-policy field (`src/types/automation.ts:24-46`) and no automation UI references gating (`grep "confirm" src/routes/automation-detail.tsx` → only delete-confirm modals). A cron-triggered agent's blast radius is bounded only by timeout (`automation.timeout`, `src/types/automation.ts:32-36`).
- **Cloud/local divergence.** Confirmation responses take different transports per backend (cloud proxy with session-key auth vs direct typed client, `event-service.api.ts:48-68`); a misconfigured cloud conversation URL would silently target the wrong host for the approval call.

## Future Considerations

- Expose a per-automation confirmation policy (or at minimum a "require first-run review" flag) so the headless mode inherits the product's own gating philosophy.
- Replace the last-agent-event heuristic with a server-declared pointer to the specific event awaiting confirmation, making the approval target authoritative instead of inferred (`conversation-confirmation-buttons.tsx:30-36`).
- Surface MEDIUM as a configurable `ConfirmRisky.threshold` option — the enum already distinguishes it (`common.ts:59-64`) and the transcript already renders it (`get-action-content.ts:94-95`); only the policy compiler lacks the lever.
- Wire the unused `reason` field into the reject button so rejections feed the agent's retry strategy.
- Document the three policy levels in `docs/architecture.md` and remove the dead `INVARIANT$*` i18n keys to prevent stale-model confusion.

## Questions / Gaps

- **Enforcement fidelity unverifiable in-source.** Nothing here proves the agent-server pauses *before* executing each risky action under `ConfirmRisky`/`AlwaysConfirm`; that logic resides in `software-agent-sdk`, outside this study's isolation boundary (searches were confined to `studies/agent-harness-study/sources/openhands`).
- **No evidence of per-action allowlists/denylists** in the frontend contract (searched `allowlist|denylist|permission` patterns across `src/api/` and `src/types/`); if they exist they are backend-only.
- **`policy_rail` analyzer provenance unknown.** `PolicyRailSecurityAnalyzer` appears only as an adapter mapping case (`src/api/agent-server-adapter.ts:613-614`) with no schema choices, UI, or docs in this repo; its semantics could not be determined from this source.
- **Whether `ConfirmRisky` also gates non-bash actions equally** (e.g., MCP tool calls) cannot be confirmed; risk is computed per `ActionEvent` (`action-event.ts:61`) but no frontend code differentiates policy application per action kind.

---

Generated by `Dimension 23.01: Autonomy Boundary` against `openhands`.

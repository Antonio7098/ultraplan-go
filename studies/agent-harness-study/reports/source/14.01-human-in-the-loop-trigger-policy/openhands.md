# Source Analysis: openhands

## Dimension 14.01: Human-in-the-Loop Trigger Policy

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React (Vite, React Router, Zustand, TanStack Query); the "agent-canvas" frontend of the OpenHands multi-repo system |
| Analyzed | 2026-08-26 |

## Summary

This repository is the OpenHands **frontend** (per `AGENTS.md`, "this repository is only the agent-canvas frontend"; the agent loop and tool execution live in the sibling Python `software-agent-sdk`). Its HITL story therefore has three parts, all of which are implemented here:

1. **Policy configuration**: two user-facing settings — `confirmation_mode` (boolean) and `security_analyzer` (`llm` | `pattern` | `policy_rail` | `none`) — stored as conversation settings (`src/types/settings.ts:128-129`).
2. **Trigger derivation**: at conversation start, those settings are compiled into an explicit wire-level `confirmation_policy` object sent to the agent server: `NeverConfirm`, or `ConfirmRisky` with `threshold: "HIGH"` + `confirm_unknown: true`, or `AlwaysConfirm` (`src/api/agent-server-adapter.ts:593-605`). A matching `security_analyzer` object is attached to the same payload (`src/api/agent-server-adapter.ts:607-618`, `1169-1173`).
3. **Review surface**: when the agent reports `ExecutionStatus.WAITING_FOR_CONFIRMATION`, the UI maps it to `AgentState.AWAITING_USER_CONFIRMATION` (`src/hooks/use-agent-state.ts:24-25`) and renders accept/reject buttons with a HIGH-risk warning banner, keyboard shortcuts, and a locked chat input; decisions are POSTed back to `/api/conversations/{id}/events/respond_to_confirmation` (`src/api/event-service/event-service.api.ts:40-68`).

Two additional human-intervention triggers exist outside the confirmation policy: a **budget/limit pause** (max-iterations error flips the agent into `PAUSED`, requiring the user to resume — `src/hooks/use-handle-ws-events.ts:55-61`) and an **always-available explicit user pause/interrupt** (`src/hooks/mutation/conversation-mutation-utils.ts:41-61`), which is recorded as a user-sourced `PauseEvent` in the event stream (`src/types/agent-server/core/events/pause-event.ts:3-8`).

The core question — *does the system know when to ask for help?* — is answered affirmatively but narrowly: the trigger is **action risk severity** (as assessed by a security analyzer) gated by a single on/off mode. Uncertainty is handled only as "unknown risk counts as risky when the LLM analyzer is active". There is no failed-validation trigger, no policy-violation trigger, and no budget-based *confirmation* prompt (the budget path just pauses).

## Rating

**7 / 10** — Clear model with tests and an explicit interface: the settings→`confirmation_policy` translation is pure, unit-tested code (`__tests__/api/agent-server-adapter.test.ts:349-376`), the trigger taxonomy is visible in one function, and the review UX has operational safeguards (input locking, risk banners, sound notification, keyboard shortcuts). It falls short of 8–10 because the accept/reject flow itself has no dedicated test coverage, confirmation *decisions* are not modeled as auditable events on the frontend side (dedup is in-memory only), the `HIGH` threshold is hardcoded, and no documentation ties the design together.

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Trigger condition code | `getConversationConfirmationPolicy()` returns `{kind:"NeverConfirm"}` when `confirmation_mode !== true`; `{kind:"ConfirmRisky", threshold:"HIGH", confirm_unknown:true}` when analyzer is `llm`; else `{kind:"AlwaysConfirm"}` | `src/api/agent-server-adapter.ts:593-605` |
| Analyzer selection | `llm→LLMSecurityAnalyzer`, `pattern→PatternSecurityAnalyzer`, `policy_rail→PolicyRailSecurityAnalyzer`, default omits field | `src/api/agent-server-adapter.ts:607-618` |
| Payload wiring | `confirmation_policy` and `security_analyzer` are required/optional fields of the conversation-start payload | `src/api/agent-server-adapter.ts:1008-1009`, `1120-1121`, `1169-1173` |
| Risk signal source | Every `ActionEvent` carries `security_risk` ("The LLM's assessment of the safety risk of this action") | `src/types/agent-server/core/events/action-event.ts:58-61` |
| Risk enum | `SecurityRisk = UNKNOWN \| LOW \| MEDIUM \| HIGH` | `src/types/agent-server/core/base/common.ts:59-64` |
| Waiting state mapping | `ExecutionStatus.WAITING_FOR_CONFIRMATION → AgentState.AWAITING_USER_CONFIRMATION` | `src/hooks/use-agent-state.ts:24-25`; enums at `src/types/agent-server/core/base/common.ts:71`, `src/types/agent-state.tsx:12` |
| Review UI | Accept/reject buttons render only while awaiting confirmation; HIGH risk shows warning banner; risk read from pending `ActionEvent.security_risk` | `src/components/shared/buttons/conversation-confirmation-buttons.tsx:30-36`, `102-118` |
| Decision submission | `useRespondToConfirmation` mutation → `EventService.respondToConfirmation(accept)` → `POST /api/conversations/{id}/events/respond_to_confirmation` (cloud via proxy) | `src/hooks/mutation/use-respond-to-confirmation.ts:12-31`; `src/api/event-service/event-service.api.ts:40-68` |
| Input locking during review | Chat input disabled while `AWAITING_USER_CONFIRMATION` | `src/components/features/chat/interactive-chat-box.tsx:62-65` |
| Configurable settings | `confirmation_mode: boolean` (default false), `security_analyzer: string\|null` (default `"llm"`) in defaults and nested conversation settings | `src/services/settings.ts:13-14`, `56-57` |
| Schema-driven config UI | Verification section rendered by `SdkSectionPage` from backend-provided schemas; de-dupes deprecated agent-settings copies | `src/routes/verification-settings.tsx:4-12`, `23-40` |
| Field dependency | Mock schema metadata: `security_analyzer.depends_on = ["confirmation_mode"]`, description "Pause for confirmation before the agent performs high-risk actions" | `src/mocks/settings-handlers.ts:407-441` |
| Settings persistence | `confirmation_mode`/`security_analyzer` merged from `conversation_settings` and saved via PATCH diffs | `src/api/settings-service/settings-service.api.ts:401-411` |
| Wire contract (e2e) | Live helper sends `confirmation_mode: false` diff and `confirmation_policy: {kind:"NeverConfirm"}` in `POST /api/conversations` | `tests/e2e/live/utils/agent-server-conversation.ts:126-128`, `243-245` |
| Budget-triggered pause | Error message starting "Agent reached maximum" sends `AgentState.PAUSED` so the user must intervene/resume | `src/hooks/use-handle-ws-events.ts:55-61` |
| Explicit user request | Pause button → `pauseConversation()`: cloud sandbox pause or local `/interrupt` | `src/components/features/chat/components/chat-input-actions.tsx:162-165`; `src/hooks/mutation/conversation-mutation-utils.ts:41-61` |
| User pause audit event | `PauseEvent` with `source: "user"` ("paused by user request") exists in the event stream types | `src/types/agent-server/core/events/pause-event.ts:3-8`; listed in `src/types/agent-server/core/events/index.ts:9` |
| Status surfacing | `WAITING_FOR_CONFIRMATION` maps to localized status text `AGENT_STATUS$WAITING_FOR_USER_CONFIRMATION` | `src/utils/status.ts:107-108`; key at `src/i18n/translation.json:25655` |
| Attention notification | Sound plays on transition into `AWAITING_USER_CONFIRMATION` (opt-in via `enable_sound_notifications`) | `src/hooks/use-agent-notification.ts:6-10`, `36-48` |
| Mode indicator | Lock-icon tooltip shown near chat input when confirmation mode enabled | `src/components/features/chat/confirmation-mode-enabled.tsx:7-27`; mounted at `src/components/features/chat/chat-interface.tsx:627` |
| Inline risk disclosure | Bash visualizer flags MEDIUM/HIGH commands; action content appends risk text for MEDIUM/HIGH | `src/components/features/chat/tool-visualizers/bash/bash.tsx:24-37`; `src/components/conversation-events/chat/event-content-helpers/get-action-content.ts:92-98` |
| Policy derivation test | Asserts payload yields `{ConfirmRisky, threshold HIGH, confirm_unknown true}` + `LLMSecurityAnalyzer` for `confirmation_mode:true, security_analyzer:"llm"` | `__tests__/api/agent-server-adapter.test.ts:349-376` |
| State-mapping & settings tests | WAITING_FOR_CONFIRMATION mapping; `confirmation_mode` persisted/read as boolean | `src/hooks/use-agent-state.test.tsx:48-55`; `__tests__/api/settings-service.test.ts:57-86` |
| Notification test | "plays notification sound when agent reaches AWAITING_USER_CONFIRMATION state" | `__tests__/hooks/use-agent-notification.test.ts:64-70` |
| Double-submit guard | Client-side dedup of already-answered confirmation events | `src/stores/event-message-store.ts:14-26`; used at `conversation-confirmation-buttons.tsx:44-47`, `93-100` |

## Answers to Dimension Questions

### 1. What triggers human review?

Three distinct mechanisms:

- **Risk-gated confirmation (primary).** With `confirmation_mode` on, each proposed action's `security_risk` decides: under the LLM analyzer the policy is `ConfirmRisky` with `threshold: "HIGH"` and `confirm_unknown: true` (high-risk *and* unknown-risk actions ask; `src/api/agent-server-adapter.ts:600-601`); with any other analyzer it is `AlwaysConfirm` — every tool call asks (`src/api/agent-server-adapter.ts:604`). With the mode off, `NeverConfirm` — nothing asks (`src/api/agent-server-adapter.ts:596-597`). The actual halt happens server-side; the frontend learns of it via `ExecutionStatus.WAITING_FOR_CONFIRMATION` over WebSocket (`src/contexts/conversation-websocket-context.tsx:639-645`).
- **Limit/budget pause.** When an error event reports "Agent reached maximum [iterations]", the frontend forces the agent into `PAUSED`, making the run stop until a human resumes (`src/hooks/use-handle-ws-events.ts:55-61`). This is an intervention point, not a confirmation dialog.
- **Explicit user interruption.** The user can always hit pause/stop, which pauses the cloud sandbox or interrupts the local run immediately (`src/hooks/mutation/conversation-mutation-utils.ts:41-61`).

Not implemented as triggers in this source: failed validation, policy violation, cost-budget confirmation prompts (`max_budget_per_task` is display-only here — see `src/components/features/conversation-panel/budget-display.tsx:4-5` and metrics stores such as `src/stores/metrics-store.ts:5`), and uncertainty signals beyond the analyzer's `UNKNOWN` bucket.

### 2. Are triggers configurable?

Partially, and deliberately split across layers. Users can toggle `confirmation_mode` and choose the `security_analyzer` through a schema-driven settings page whose fields come from the backend (`verification.confirmation_mode`, `verification.security_analyzer`; `src/routes/verification-settings.tsx:9-12`, `24-36`). The analyzer choice is constrained by schema metadata (`choices: llm|none`, `depends_on: ["confirmation_mode"]`; `src/mocks/settings-handlers.ts:433-437`) even though the adapter understands a richer set (`pattern`, `policy_rail`; `src/api/agent-server-adapter.ts:611-614`). What is *not* configurable from this client: the risk threshold is hardcoded to `"HIGH"` inside `getConversationConfirmationPolicy` (`src/api/agent-server-adapter.ts:601`), and the three-policy taxonomy itself is fixed. Defaults are safe-ish but asymmetric: confirmation off by default (`src/services/settings.ts:13`, `56`) while the analyzer defaults to `"llm"` (`src/services/settings.ts:14`).

### 3. Can users request human review?

Yes, in two directions. (a) **Pull the agent back**: the chat toolbar exposes pause, wired to `pauseConversationMutation` (`src/components/features/chat/components/chat-input-actions.tsx:162-165`), which pauses the cloud sandbox or sends `/interrupt` locally (`src/hooks/mutation/conversation-mutation-utils.ts:41-61`); the resulting `PauseEvent` is explicitly typed as user-sourced (`src/types/agent-server/core/events/pause-event.ts:3-8`). (b) **Answer the agent's ask**: while `AWAITING_USER_CONFIRMATION`, dedicated Confirm/Reject buttons appear next to the last message (`generic-event-message-wrapper.tsx:126`, `user-assistant-event-message.tsx:127`), with keyboard shortcuts Cmd+Enter (accept) and Shift+Cmd+Backspace (reject) (`conversation-confirmation-buttons.tsx:66-90`); the free-text input is locked until the decision is made (`interactive-chat-box.tsx:62-65`).

### 4. Are trigger decisions auditable?

Weakly, from this repo's perspective. The **trigger context** is durable: every action carries its assessed `security_risk` on the persisted `ActionEvent` (`src/types/agent-server/core/events/action-event.ts:58-61`), surfaced inline in the transcript and bash cards (`get-action-content.ts:92-98`, `bash.tsx:24-37`). **User-initiated pauses** become `PauseEvent`s in the event stream. But the **confirmation decision itself** (accept/reject) is sent to a runtime endpoint (`respond_to_confirmation`; `src/api/event-service/event-service.api.ts:40-68`) and has no corresponding event type among the ten exported frontend event kinds (`src/types/agent-server/core/events/index.ts:1-11`) — it is observable only indirectly through subsequent `ConversationStateUpdateEvent` execution-status transitions and whichever observation follows. Client-side double-submit protection lives in a non-persisted Zustand array (`src/stores/event-message-store.ts:14-26`), so after a reload the UI relies purely on agent state. Whether the server persists the decision cannot be verified from this source.

## Architectural Decisions

1. **Compile settings into an explicit server contract, not client-side gating.** The frontend does not intercept tool calls; it translates `confirmation_mode`/`security_analyzer` into a declarative `confirmation_policy` + `security_analyzer` object on the conversation-start payload (`src/api/agent-server-adapter.ts:1120-1121`, `1169-1173`). Enforcement belongs to the SDK agent-server; the client owns configuration and interaction.
2. **Three named policies instead of arbitrary parameters** (`NeverConfirm` / `AlwaysConfirm` / `ConfirmRisky`) keep the wire vocabulary small and mirror the upstream OpenHands semantics — asserted verbatim by the adapter test "derives confirmation and security settings the same way as OpenHands" (`__tests__/api/agent-server-adapter.test.ts:349-376`).
3. **Server-owned state machine, client-rendered states.** Confirmation wait is an `ExecutionStatus` streamed over WebSocket and mapped to a local `AgentState` union (`src/hooks/use-agent-state.ts:10-35`), so any tab/device reconnecting sees the same pending-review truth without extra protocol.
4. **Modal-free, in-stream review.** Confirm/Reject renders inline beside the pending action card rather than as a blocking modal (`generic-event-message-wrapper.tsx:126`), combined with hard input-locking (`interactive-chat-box.tsx:62-65`) to prevent the user typing past a pending gate.
5. **Schema-driven settings UI with backend authority.** The verification page renders whatever fields the backend schemas expose, including dependency ordering (`depends_on`), and strips deprecated duplicates (`src/routes/verification-settings.tsx:4-12`).

## Notable Patterns

- **Unknown-risk conservatism**: `confirm_unknown: true` means an unclassifiable action still triggers review when the LLM analyzer is active (`src/api/agent-server-adapter.ts:601`) — a deliberate fail-closed bias.
- **Multi-sensory attention routing**: tab-title emoji (`src/utils/agent-state-emoji.ts:22`), status label (`src/utils/status.ts:107-108`), optional sound (`src/hooks/use-agent-notification.ts:6-10`), lock icon for the enabled mode (`confirmation-mode-enabled.tsx:16-26`).
- **Risk made legible before the click**: HIGH-risk pending actions get a red warning banner above the buttons (`conversation-confirmation-buttons.tsx:107-118`), and executed MEDIUM/HIGH bash commands stay flagged afterward (`bash.tsx:29-37`).
- **Idempotency-by-store**: answered confirmation IDs are tracked to avoid duplicate submissions on re-render (`conversation-confirmation-buttons.tsx:44-47`, `93-100`).
- **Test-tagged spec discipline**: behavior specs are grep-able via `@spec` comments per `AGENTS.md`, though the confirmation components carry none — a coverage-signaling gap in itself.

## Tradeoffs

- **Simplicity vs. expressiveness of triggers**: one boolean + one enum covers most user needs and keeps the wire contract tiny, but forecloses per-tool rules, path-based rules, threshold tuning, or "ask only for writes" policies without an SDK change (`src/api/agent-server-adapter.ts:593-605`).
- **AlwaysConfirm as fallback**: choosing a non-LLM analyzer silently upgrades to confirm-everything (`src/api/agent-server-adapter.ts:604`) — safest possible default, but potentially noisy enough that users disable confirmation mode entirely.
- **Frontend trust boundary**: because enforcement is server-side, the client can only *display* the wait state; a stale socket means a user could believe they are reviewing when the run already proceeded (or vice versa). Mitigated by REST-fallback execution status (`use-agent-state.ts:49-52`) but never fully closed.
- **In-memory dedup**: `submittedEventIds` prevents double POSTs cheaply but resets on navigation/reload (`event-message-store.ts:15`); correctness then depends on the server treating repeat responses idempotently, which is unverifiable here.
- **Pause semantics differ by backend kind**: cloud pauses gracefully after the in-flight LLM call; local `/interrupt` cancels mid-flight (`conversation-mutation-utils.ts:36-61`) — same button, different durability guarantees.

## Failure Modes / Edge Cases

- **Decision submitted twice** (double-click, shortcut + button race): guarded client-side by `submittedEventIds` (`conversation-confirmation-buttons.tsx:44-47`) — but the store is volatile (`src/stores/event-message-store.ts:14-26`).
- **Awaiting-action detection is loose**: `awaitingAction` is computed as "the last agent event whenever the current state is AWAITING_USER_CONFIRMATION" (`conversation-confirmation-buttons.tsx:30-36`); if the trailing event is not the gated action (e.g., interleaved system/system-error event), the accept/reject would attach to the wrong event ID for dedup purposes (submission itself goes to the conversation-level endpoint, so the server-side effect is likely still correct).
- **Non-ActionEvent pending events degrade to `SecurityRisk.UNKNOWN`**, showing the generic prompt without the high-risk banner (`conversation-confirmation-buttons.tsx:102-107`) — acceptable, but means risk warnings depend on event typing discipline.
- **Max-iterations pause depends on string matching** an error message prefix ("Agent reached maximum", `use-handle-ws-events.ts:57-60`) — fragile against server copy changes; there is even a `STUCK` status that is silently remapped to `ERROR` (`use-agent-state.ts:30-31`).
- **No E2E coverage of the confirmation loop**: mock-LLM suites exercise settings and conversations, and the live helper explicitly sets `confirmation_mode: false` and `NeverConfirm` (`tests/e2e/live/utils/agent-server-conversation.ts:127`, `243-245`) — so the full ask→answer→resume cycle ships without automated verification at either layer of this repo.
- **Reload during review**: since dedup state is lost, the UI must rely solely on `execution_status`; if the socket reconnect misses the status event, buttons may briefly render for an already-answered action.

## Future Considerations

- Make the risk threshold and unknown-handling configurable end-to-end (parameterize `ConfirmRisky` instead of hardcoding `threshold: "HIGH"` at `src/api/agent-server-adapter.ts:601`) and expose richer analyzers (`pattern`, `policy_rail`) in the schema choices beyond the mocked `llm|none` set (`src/mocks/settings-handlers.ts:433-436`).
- Model the confirmation decision as a first-class event type (mirroring `PauseEvent`) so accept/reject becomes part of the replayable stream and transcript export, rather than a side-effect POST.
- Add dedicated unit tests for `ConversationConfirmationButtons` (render gating, shortcut handling, duplicate suppression) and one mock-LLM E2E trajectory that drives `AWAITING_USER_CONFIRMATION` → accept and → reject paths.
- Replace the max-iterations string-prefix match (`use-handle-ws-events.ts:57`) with a typed error code once the server provides one.
- Persist or server-authoritize response dedup so reload-during-review cannot double-answer.

## Questions / Gaps

- **Where is enforcement actually implemented?** The gate, analyzer execution, and decision persistence live in the sibling Python `software-agent-sdk` agent-server, outside this source. Searched this repo for `ConfirmationResponseEvent`, `respond_to_confirmation` producers, and analyzer implementations — only the client call sites exist (`src/api/event-service/event-service.api.ts:40-68`); no evidence of server-side audit storage is available here. **No evidence found within this source's boundary.**
- **Does the server treat repeated `respond_to_confirmation` calls idempotently?** Unverifiable from the frontend code alone.
- **Is `PatternSecurityAnalyzer` / `PolicyRailSecurityAnalyzer` reachable from real (non-mock) schemas?** The adapter supports both kinds (`src/api/agent-server-adapter.ts:611-614`), but the only schema fixture exposing choices lists `llm|none` (`src/mocks/settings-handlers.ts:670`); no UI path selects the other two was found.
- **Budget-triggered review**: `max_budget_per_task` exists in settings and metrics (`src/types/settings.ts:143`, `src/stores/metrics-store.ts:5`) but no code path converts budget exhaustion into a confirmation request; only display widgets reference it. Searched `src/` for budget-conditioned mutations — none found beyond rendering.

---

Generated by `14.01-human-in-the-loop-trigger-policy` against `openhands`.

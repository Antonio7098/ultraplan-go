# Source Analysis: openhands

## Governance UX and Operator Workflow

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React (Vite, React Router, Zustand, TanStack Query, @openhands/typescript-client) |
| Analyzed | 2026-08-26 |

## Summary

OpenHands (agent-canvas frontend) is a single-operator, single-conversation developer agent UI — not a multi-tenant governance platform. Its only governance primitive is a per-conversation, human-in-the-loop confirmation gate for risky tool actions. When `confirmation_mode=true` the agent-server sets `execution_status=waiting_for_confirmation`, the frontend surfaces a Confirm/Reject affordance inline in the active chat, and the operator's decision is posted to `POST /api/conversations/{id}/events/respond_to_confirmation`. There is no approval dashboard, no cross-conversation review queue, no bulk operation, and no evidence-pack artifact. Exception states (ERROR/STUCK/PAUSED/WAITING_FOR_CONFIRMATION) are surfaced via chat banners, status dots, sound, and title emoji, but governance decisions leave no audit trail in the UI beyond the event stream and optional transcript/zip exports.

## Rating

**3 / 10 — Absent / ad-hoc**

Rationale: A functional single-action approve/reject exists and is reachable without reading code, which lifts the score above 1. However every capability the dimension asks for at operator scale — dashboard, queue with pending/approved/rejected views, exception workflow, evidence packs, bulk actions — is absent or implicit. The implementation is suitable for a solo developer approving one risky bash at a time, not for an operator triaging many agents under pressure.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Confirmation policy model | `getConversationConfirmationPolicy()` maps `confirmation_mode !== true → NeverConfirm`, `security_analyzer==="llm" → ConfirmRisky(threshold HIGH, confirm_unknown true)`, else `AlwaysConfirm` | `studies/agent-harness-study/sources/openhands/src/api/agent-server-adapter.ts:593-605` |
| Confirmation payload wiring | `buildStartConversationRequest()` attaches `confirmation_policy` and optional `security_analyzer` to `StartConversationPayload` | `studies/agent-harness-study/sources/openhands/src/api/agent-server-adapter.ts:1120-1173` |
| Settings type + defaults | `Settings.confirmation_mode: boolean`, `security_analyzer: string \| null`; default `confirmation_mode: false` | `studies/agent-harness-study/sources/openhands/src/types/settings.ts:128-129` and `studies/agent-harness-study/sources/openhands/src/services/settings.ts:13,56` |
| Verification settings UI | `VerificationSettingsScreen` renders `conversation_settings.verification` (deduped from `agent_settings.verification`) via `SdkSectionPage` — the only place an operator configures confirmation/security_analyzer | `studies/agent-harness-study/sources/openhands/src/routes/verification-settings.tsx:1-43` |
| Pending-action affordance | `ConversationConfirmationButtons` finds the last `source==="agent"` event when `curAgentState===AWAITING_USER_CONFIRMATION`, extracts `security_risk`, shows `RiskAlert` if HIGH, renders Confirm/Reject via `ActionTooltip` | `studies/agent-harness-study/sources/openhands/src/components/shared/buttons/conversation-confirmation-buttons.tsx:30-136` |
| Approval/rejection handler | `useRespondToConfirmation` mutation → `EventService.respondToConfirmation(conversationId, conversationUrl, {accept}, sessionApiKey)`; prevents duplicate submit via `submittedEventIds` | `studies/agent-harness-study/sources/openhands/src/hooks/mutation/use-respond-to-confirmation.ts:12-32` and `studies/agent-harness-study/sources/openhands/src/components/shared/buttons/conversation-confirmation-buttons.tsx:38-58` |
| Approval API (local+cloud) | `EventService.respondToConfirmation` branches: cloud via `callCloudProxy` with `hostOverride=buildHttpBaseUrl(conversationUrl)` + `session-api-key`, local via `ConversationClient.respondToConfirmation` | `studies/agent-harness-study/sources/openhands/src/api/event-service/event-service.api.ts:40-69` |
| Risk signal | `SecurityRisk` enum `UNKNOWN/LOW/MEDIUM/HIGH`; `RiskAlert` renders only `severity==="high"` (red alert) | `studies/agent-harness-study/sources/openhands/src/types/agent-server/core/base/common.ts:58-64` and `studies/agent-harness-study/sources/openhands/src/components/shared/risk-alert.tsx:12-36` |
| Risk surfacing in content | `getActionContent` appends `getRiskText(event.security_risk)` when `MEDIUM/HIGH` | `studies/agent-harness-study/sources/openhands/src/components/conversation-events/chat/event-content-helpers/get-action-content.ts:30-97` |
| Chat blocked while awaiting | `InteractiveChatBox` disables `CustomChatInput` when `curAgentState===AWAITING_USER_CONFIRMATION` | `studies/agent-harness-study/sources/openhands/src/components/features/chat/interactive-chat-box.tsx:62-65` |
| State mapping | `ExecutionStatus.WAITING_FOR_CONFIRMATION → AgentState.AWAITING_USER_CONFIRMATION`; `STUCK → ERROR` lossy mapping | `studies/agent-harness-study/sources/openhands/src/hooks/use-agent-state.ts:24-25,30-31` and `studies/agent-harness-study/sources/openhands/src/types/agent-state.tsx:12` |
| Execution status source | `ExecutionStatus.WAITING_FOR_CONFIRMATION="waiting_for_confirmation"` | `studies/agent-harness-study/sources/openhands/src/types/agent-server/core/base/common.ts:71` |
| Confirmation mode indicator | `ConfirmationModeEnabled` shows lock icon tooltip when `settings.confirmation_mode` true | `studies/agent-harness-study/sources/openhands/src/components/features/chat/confirmation-mode-enabled.tsx:7-27` |
| Shortcuts | `ConversationConfirmationButtons` binds `Cmd+Enter → accept`, `Shift+Cmd+Backspace → reject` | `studies/agent-harness-study/sources/openhands/src/components/shared/buttons/conversation-confirmation-buttons.tsx:60-90` |
| Action button detail | `ActionTooltip` renders `Confirm: "Continue ⌘↩" / Reject: "Cancel ⇧⌘⌫"` with tooltips `USER_CONFIRMED/USER_REJECTED` | `studies/agent-harness-study/sources/openhands/src/components/shared/action-tooltip.tsx:11-44` |
| Status dot | `ConversationStatusDot` maps `WAITING_FOR_CONFIRMATION → active (green static dot)`, not a distinct "needs review" visual; `FINISHED=check, RUNNING=pulse, ERROR/STUCK=red` | `studies/agent-harness-study/sources/openhands/src/components/features/conversation-panel/conversation-status-dot.tsx:26-43` |
| Exception surfacing — banner | `ChatInterface` renders `ErrorMessageBanner` over composer when `errorMessage` present, with copy/expand/retry/reauth affordances | `studies/agent-harness-study/sources/openhands/src/components/features/chat/chat-interface.tsx:583-600` and `studies/agent-harness-study/sources/openhands/src/components/features/chat/error-message-banner.tsx:95-210` |
| Exception surfacing — sound | `useAgentNotification` plays sound on transition into `AWAITING_USER_INPUT / FINISHED / AWAITING_USER_CONFIRMATION` if `enable_sound_notifications` | `studies/agent-harness-study/sources/openhands/src/hooks/use-agent-notification.ts:6-10,18-48` |
| Exception surfacing — title emoji | `ExecutionStatus.WAITING_FOR_CONFIRMATION` → `✅` in app title (via `useAppTitle`) | `studies/agent-harness-study/sources/openhands/__tests__/hooks/use-app-title.test.tsx:73` (exercises `src/utils/agent-state-emoji.ts:22`) |
| Exception routing — websocket | `ConversationWebSocketContext` forwards displayable errors to banner store + telemetry; `AgentErrorEvent` rendered inline, not as banner | `studies/agent-harness-study/sources/openhands/src/contexts/conversation-websocket-context.tsx:570-804` |
| Evidence pack / export — transcript | `eventsToMarkdown`/`eventsToHtml` in transcript-export builds full conversation export (tool details, timestamps, sanitized markdown) | `studies/agent-harness-study/sources/openhands/src/utils/transcript-export/index.ts:514-677` |
| Evidence pack / export — zip | `useDownloadConversation` downloads `AgentServerConversationService.downloadConversation(id)` as `conversation_{id}.zip` | `studies/agent-harness-study/sources/openhands/src/hooks/use-download-conversation.ts:9-27` |
| Dashboard / review queue — no evidence | Glob `**/*govern*`, `**/*approv*`, `**/*queue*` returned zero governance files; `grep approval` found only workflow/pr artifacts and role `permissions` unrelated to governance; automations dashboard is unrelated | `studies/agent-harness-study/sources/openhands/src/components/features/automations/dashboard/automations-dashboard-controls.tsx:1` (no approval UI) — exhaustive search boundary documented in Gaps |
| Bulk operations — no evidence | `grep bulk\|evidence` found zero bulk-approval code; no `select all`, multi-select, or batch `respond_to_confirmation` endpoint usage | `studies/agent-harness-study/sources/openhands/src/api/event-service/event-service.api.ts:1-184` (single-id `respondToConfirmation` only) |

## Answers to Dimension Questions

**1. Can operators see what needs review?**

Partially, only inside the active conversation. When `confirmation_mode` is enabled and the security analyzer flags an action, the agent pauses with `execution_status=waiting_for_confirmation` (`src/api/agent-server-adapter.ts:593-605`, `src/types/agent-server/core/base/common.ts:71`). `ChatInterface` then renders `ConfirmationModeEnabled` lock icon (`src/components/features/chat/confirmation-mode-enabled.tsx:12-26`) and `ConversationConfirmationButtons` surfaces the pending action (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:30-36,92-100`) with the i18n string `CHAT_INTERFACE$AGENT_AWAITING_USER_CONFIRMATION_MESSAGE` (`src/i18n/translation.json:10866`). There is no central dashboard or review queue listing pending / approved / rejected across conversations. `ConversationStatusDot` does not visually distinguish `WAITING_FOR_CONFIRMATION` from `IDLE` (both map to `active` green dot, `src/components/features/conversation-panel/conversation-status-dot.tsx:32-34`), so the conversation list conveys no priority signal. Operators must open each conversation to discover it needs review.

**2. Can they act on approvals efficiently?**

For a single pending action, yes — the interaction is lightweight: two buttons (Confirm `⌘↩`, Reject `⇧⌘⌫`) with keyboard shortcuts and dedup via `submittedEventIds` (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:60-90,44-47`). `ActionTooltip` labels the actions with localized hints (`src/components/shared/action-tooltip.tsx:24-27`). `InteractiveChatBox` blocks further chat input until the decision is made (`src/components/features/chat/interactive-chat-box.tsx:62-65`), preventing accidental queueing behind the gate. However efficiency collapses beyond one: no bulk approve/reject, no queue keyboard navigation, no "approve all high-risk" policy override in the UI. The mutation is strictly per-conversation single-id (`src/api/event-service/event-service.api.ts:40-69`, `src/hooks/mutation/use-respond-to-confirmation.ts:14-32`).

**3. Are exceptions surfaced?**

Generically, yes; governance-specifically, weakly. Generic agent/transport errors surface through three channels: `ErrorMessageBanner` with copy/expand/retry/reauth (`src/components/features/chat/error-message-banner.tsx:95-210`) rendered in `ChatInterface` (`src/components/features/chat/chat-interface.tsx:583-600`), browser title emoji, and optional sound (`src/hooks/use-agent-notification.ts:6-10`). `ConversationStatusDot` maps `ERROR/STUCK → red`, `PAUSED → gray` (`src/components/features/conversation-panel/conversation-status-dot.tsx:37-39`), and sandbox `MISSING/ERROR` overrides execution status for cloud (`src/components/features/conversation-panel/conversation-status-dot.tsx:128-133`). What is missing: no structured exception taxonomy for governance (e.g., "approval timed out", "policy violation", "overrode analyzer"), no severity triage, no retry/escalate workflow, and `STUCK` is lossily collapsed into `ERROR` (`src/hooks/use-agent-state.ts:30-31`). A rejected confirmation yields no post-rejection guidance in the UI — the agent simply resumes.

**4. Is the governance UI usable under pressure?**

No. Under pressure (many concurrent runs, high-risk bursts, or page reload mid-confirmation) the UI degrades: (a) no filtered/sorted queue, so triage requires manual tab hunting; (b) no persistent "N pending approvals" badge or notification center — only transient in-conversation prompt and optional sound that fires on state transition (`src/hooks/use-agent-notification.ts:36-48`); (c) no bulk or "approve all LOW" action; (d) pending state is not durable in a list — `submittedEventIds` dedup is in-memory Zustand (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:17-22`), so refresh behavior depends on re-derived `curAgentState` from websocket/REST, not an explicit persisted queue; (e) evidence is limited to a red `RiskAlert` (`src/components/shared/risk-alert.tsx:20-33`) plus whatever the action's `summary/thought` contains — no attached policy rationale, diff preview, or audit evidence pack at approval time (transcript/zip exports exist but are post-hoc, `src/utils/transcript-export/index.ts:514-677` and `src/hooks/use-download-conversation.ts:15-21`).

## Architectural Decisions

* **Opt-in NeverConfirm default.** `confirmation_mode` defaults `false` (`src/services/settings.ts:13`), so out-of-box agents auto-execute without human gates. Governance is additive and per-conversation (`src/api/agent-server-adapter.ts:596-605`), not a deployment-wide policy.
* **Risk threshold is server-evaluated, client-rendered.** The agent-server's security analyzer assigns `security_risk` (`LOW/MEDIUM/HIGH/UNKNOWN` in `src/types/agent-server/core/base/common.ts:58-64`); the frontend only decides whether to prompt based on `confirmation_policy` derived at conversation start. This avoids duplicating analyzer logic in the client but leaves the client unable to explain *why* an action was flagged beyond the generic `HIGH_RISK_WARNING`.
* **Single-conversation confirmation state.** `execution_status` is a conversation-level string (`src/types/agent-server/core/base/common.ts:67-75`); the UI maps it to a single `AgentState.AWAITING_USER_CONFIRMATION` (`src/hooks/use-agent-state.ts:24-25`). No multi-conversation governance state exists in the frontend stores.
* **Snapshot, not queue, persistence.** Transcript exports and zip downloads are pull-on-demand artifacts built from the full event log (`src/utils/transcript-export/index.ts:264-495`, `src/hooks/use-download-conversation.ts:17-21`), not pre-built evidence packs attached to the approval prompt.
* **Cloud/local branch for confirmation RPC.** `EventService.respondToConfirmation` handles both planes (cloud via `callCloudProxy` with `session-api-key` to runtime sandbox, local via `ConversationClient`) (`src/api/event-service/event-service.api.ts:40-69`), keeping the approval path functional regardless of deployment.

## Notable Patterns

* **Inline human-in-the-loop gate:** `ConversationConfirmationButtons` reverses the event log to locate the pending agent action (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:30-36`), an efficient pattern for single-pending-action semantics but not a queue.
* **Keyboard-first approval under focus:** `Cmd+Enter` / `Shift+Cmd+Backspace` shortcuts (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:66-77`) improve throughput for power users even without bulk.
* **Graceful dedup:** `submittedEventIds` in `event-message-store` prevents double-submit on impatient clicks/keys (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:44-47`).
* **Verification as code:** `VerificationSettingsScreen` is a thin `SdkSectionPage` over `conversation_settings.verification` (`src/routes/verification-settings.tsx:1-43`), so governance knobs track the backend schema automatically rather than being hardcoded.
* **Export sanitization:** `sanitizeMarkdownText` and `escapeHtml` plus fenced markdown generation hardens transcript exports against injection (`src/utils/transcript-export/index.ts:138-150,505-512`).

## Tradeoffs

* **Simplicity vs. observability.** Defaulting `confirmation_mode=false` and rendering `WAITING_FOR_CONFIRMATION` as ordinary `active` in the conversation list (`src/components/features/conversation-panel/conversation-status-dot.tsx:32-34`) keeps the UI uncluttered for solo devs but makes pending approvals invisible at fleet scale.
* **Generic error UX vs. governance semantics.** Reusing `ErrorMessageBanner` for all failures (`src/components/features/chat/error-message-banner.tsx:1-211`) avoids bespoke governance error UI, at the cost of no differentiated "approval rejected / policy violation / analyzer failure" workflow.
* **Client minimalism vs. operator power.** One RPC, two buttons, no server-side bulk endpoint consumed — minimal code to maintain, but operators cannot triage under pressure.
* **Sound as the only out-of-band signal.** `useAgentNotification` (`src/hooks/use-agent-notification.ts:32-48`) respects `enable_sound_notifications` and transition-only firing, which is polite for single-session use but insufficient as a pager for waiting approvals when the tab is backgrounded.

## Failure Modes / Edge Cases

* **Analyzer silent failure.** If `security_analyzer` is misconfigured or the LLM risk predictor returns `UNKNOWN`, policy is `ConfirmRisky(confirm_unknown: true)` only when `security_analyzer==="llm"` (`src/api/agent-server-adapter.ts:600-602`); otherwise `AlwaysConfirm` when `confirmation_mode=true` (`src/api/agent-server-adapter.ts:604`). No UI explains which path produced the current `confirmation_policy`.
* **No evidence on reject.** Rejecting posts `{accept:false}` via `EventService.respondToConfirmation` (`src/hooks/mutation/use-respond-to-confirmation.ts:21-23`). The UI does not capture a reason, nor show what the agent did with the rejection (retry, skip, abort) — the transcript just continues.
* **State mapping collapse.** `STUCK → ERROR` (`src/hooks/use-agent-state.ts:30-31`) discards the distinct stuck signal; operator playbooks cannot distinguish a planner loop stall from an infrastructure error at the status-dot level.
* **Race on refresh.** `ConversationConfirmationButtons` derives `awaitingAction` from `useEventStore.events.slice().reverse().find(...)` gated on `curAgentState` (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:30-36`). If websocket history is still loading or the pending action sits beyond the initial `INITIAL_HISTORY_PAGE_SIZE=50` REST window, the button may momentarily not render until WS/backfill completes.
* **Dedup is ephemeral.** Duplicate-submit protection lives in Zustand `submittedEventIds` (in-memory); after a hard reload the same pending ID could be resubmitted if the server did not yet clear `WAITING_FOR_CONFIRMATION`.
* **Bulk absence as failure amplifier.** Without bulk or queue, a burst of concurrent `WAITING_FOR_CONFIRMATION` conversations forces serial tab visits; missed approvals stall agents with no SLA timer or escalation.
* **Export staleness.** Transcript exports are built from the local event store snapshot (`src/utils/transcript-export/index.ts:264-280`); if the UI is paginating or websocket is lagging, the export may omit the most recent evidence at approval time.

## Future Considerations

* Add a **Review Queue** route (`/review` or `/governance`) listing all `WAITING_FOR_CONFIRMATION` conversations with filters (risk, age, repo, automation), backed by a paginated `search` on `execution_status` — the current per-conversation reverse-scan does not scale.
* Promote **`WAITING_FOR_CONFIRMATION` to a distinct status-dot visual** (e.g., amber pulsing badge) and a global badge count in the sidebar, reusing `ConversationStatusDot` and header notification plumbing.
* **Attach evidence packs to the confirmation prompt**: include the action summary, `security_risk` + `security_analyzer` rationale, working-dir/git diff preview, and previous-approval history; reuse `eventsToMarkdown` sanitization but render inline as a collapsible `<details>` block at decision time.
* **Bulk operations**: introduce a server endpoint `POST /api/conversations/batch/respond_to_confirmation` and a `useRespondToConfirmationBatch` hook; surface as `Approve all LOW` / `Reject selected` with audit-trail toast and undo window.
* **Approval audit log**: persist per-decision records (who/what/when/why, policy snapshot, risk level) queryable from conversations and from the review queue; expose via existing `downloadConversation` zip or a dedicated `/audit` export.
* **Exception workflow for governance**: dedicated handling for `approval timeout`, `analyzer error`, and `operator override` with escalation (reassign, auto-reject after N minutes, require second approver when `HIGH` risk).
* **Strengthen failure semantics**: stop collapsing `STUCK` into `ERROR` in `useAgentState`; add `AgentState.STUCK` and distinct banner copy with suggested remediation.

## Questions / Gaps

* **No cross-conversation pending view found.** Exhaustive globs for `*govern*`, `*approv*`, `*queue*`, `*bulk*`, and grep for `pending.*action|awaiting approval|require.*confirm` returned no review-queue implementation; `automations-dashboard*` files relate to automations scheduling, not governance. Search boundary: `src/`, `__tests__/`, `tests/`, `config/`.
* **No evidence-pack builder at approval time.** `transcript-export` (`src/utils/transcript-export/index.ts:1-678`) and `useDownloadConversation` (`src/hooks/use-download-conversation.ts:1-27`) are general post-hoc exports. No code assembles a pre-approval pack (risk rationale, diff, tool preview) for the confirmation UI; `RiskAlert` content is generic (`HIGH_RISK_WARNING`) (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:113`).
* **No approval persistence/audit query.** `EventService.searchEvents` (`src/api/event-service/event-service.api.ts:102-181`) paginates events, but there is no frontend consumer that aggregates `accepted/rejected` decisions into an approved/rejected list.
* **No bulk mutation.** Only single-id `respondToConfirmation` exists (`src/api/event-service/event-service.api.ts:40-69`); no client or service code references a batch path.
* **Analyzer policy visibility gap.** Whether the current conversation is `NeverConfirm / ConfirmRisky(threshold HIGH) / AlwaysConfirm` is not shown in the UI beyond the lock icon (`src/components/features/chat/confirmation-mode-enabled.tsx:7-27`); operators cannot confirm which threshold is active without inspecting settings payloads.

---

Generated by `Dimension 09.03: Governance UX and Operator Workflow` against `openhands`.

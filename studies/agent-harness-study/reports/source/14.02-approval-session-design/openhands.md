# Source Analysis: openhands

## 14.02 Approval Session Design

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React (agent-canvas frontend; agent-server lives in a sibling repo) |
| Analyzed | 2026-08-26 |

## Summary

This source is the OpenHands **frontend** ("agent-canvas"); per the repo's own boundary notes (`AGENTS.md`, "Repository Map" table), the agent loop, confirmation-policy enforcement, and pending-action state machine live in the sibling `software-agent-sdk` (Python), which is outside this study's isolation boundary. What this repository owns is the **approval request/response surface**: a settings-driven confirmation policy sent at conversation start, a server-authoritative "waiting for confirmation" execution status delivered over WebSocket/REST, an approve/reject UI wired to keyboard shortcuts, and a typed POST to `/api/conversations/{id}/events/respond_to_confirmation`.

The model is: the client *declares* a policy (`NeverConfirm` / `AlwaysConfirm` / `ConfirmRisky`) when creating a conversation (`src/api/agent-server-adapter.ts:593-605`, attached at `src/api/agent-server-adapter.ts:1120-1122`), the *server* suspends the agent with `execution_status = "waiting_for_confirmation"`, and the frontend renders approve/reject controls derived purely from that status plus the latest agent action event (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:30-36`). Because approval state is never stored client-side as truth — it is re-derived from the conversation's execution status on every load — an approval session survives a browser refresh as long as the server keeps the conversation in the waiting state.

Notable gaps: there is no timeout/expiry mechanism anywhere in the frontend for pending approvals; no per-tool or "approve all for session" scoping (scope granularity is whole-conversation policy only); and approval decisions are not audited by any frontend-specific log — auditability rests entirely on the server-side event store. The deduplication guard against double-submission is memory-only and resets on refresh.

## Rating

**6 / 10** — Present but inconsistent, weakly documented, fragile in places.

Rationale against the rubric:

- The core model is clear and testable: policy derivation has a named function with a dedicated unit test asserting parity with upstream OpenHands behavior (`__tests__/api/agent-server-adapter.test.ts:349-378`), and the request flow is a thin typed mutation over a single endpoint (`src/hooks/mutation/use-respond-to-confirmation.ts:12-32`). That earns the "clear model" credit.
- But operational safeguards are thin: no timeout on pending approvals (no TTL/expiry code exists — searched `expire|expiry|ttl` across `src/`; only unrelated LLM-token hits such as `src/utils/mcp-credential-validation.ts:38`), no optimistic-error recovery or retry on the respond mutation, a memory-only double-submit guard (`src/stores/event-message-store.ts:6-27`), and the optional `reason` field on rejection is defined but never populated by the UI (`src/api/event-service/event-service.types.ts:1-4` vs `src/hooks/mutation/use-respond-to-confirmation.ts:21-23`).
- There are no unit tests for the confirmation buttons component itself (searched `__tests__/` for `ConversationConfirmationButtons|respond-to-confirmation`; no matches), so the most safety-relevant UI path is exercised only indirectly.
- Durability is good but borrowed: it works because the server owns the state, not because the frontend engineers persistence deliberately.

It does not reach 7–8 because of the missing tests on the approval UI, the unused rejection-reason field, and the absence of any timeout/observability story; it stays well above 4 because the policy derivation is explicit, typed, tested, and consistently server-authoritative.

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Agent states for confirmation | `AgentState.AWAITING_USER_CONFIRMATION`, `USER_CONFIRMED`, `USER_REJECTED` enum members | `src/types/agent-state.tsx:12-14` |
| Server execution status vocabulary | `ExecutionStatus.WAITING_FOR_CONFIRMATION` among idle/running/paused/finished/error/stuck | `src/types/agent-server/core/base/common.ts:67-75` |
| Status → AgentState mapping | `WAITING_FOR_CONFIRMATION → AWAITING_USER_CONFIRMATION` in `mapExecutionStatusToAgentState` | `src/hooks/use-agent-state.ts:24-25` |
| State source (live + REST fallback) | Hook reads live zustand `execution_status`, falling back to polled conversation data | `src/hooks/use-agent-state.ts:45-59` |
| Live status updates over WebSocket | `setExecutionStatus(event.value.execution_status)` on full-state update; agent-status variant below | `src/contexts/conversation-websocket-context.tsx:638-645` (duplicated for planning socket at 869-876) |
| Client-side live-status store | In-memory zustand `execution_status` + setter/reset | `src/stores/conversation-state-store.ts:20-25` |
| Pending-action detection | Reverse scan of event store for latest agent event while state is awaiting confirmation | `src/components/shared/buttons/conversation-confirmation-buttons.tsx:30-36` |
| Approve/reject handler | `handleConfirmation(accept)` marks submitted id then calls mutation | `src/components/shared/buttons/conversation-confirmation-buttons.tsx:38-58` |
| Double-submit guard | `addSubmittedEventId` before send; render suppressed if already submitted | `src/components/shared/buttons/conversation-confirmation-buttons.tsx:44-47,92-100` |
| Dedup store is memory-only | `useEventMessageStore` plain zustand, no `persist` middleware | `src/stores/event-message-store.ts:1-27` |
| Keyboard shortcuts | Reject ⇧⌘⌫, Confirm ⌘↩ bound on document while pending | `src/components/shared/buttons/conversation-confirmation-buttons.tsx:60-90` |
| Risk-gated warning UI | `security_risk === HIGH` renders `RiskAlert` before buttons | `src/components/shared/buttons/conversation-confirmation-buttons.tsx:102-118` |
| Risk enum | `SecurityRisk { UNKNOWN, LOW, MEDIUM, HIGH }` | `src/types/agent-server/core/base/common.ts:59-64` |
| Risk provenance | `ActionEvent.security_risk` — "LLM's assessment of the safety risk of this action" | `src/types/agent-server/core/events/action-event.ts:58-61` |
| Respond mutation | Builds `{ accept }` request, delegates to `EventService.respondToConfirmation` | `src/hooks/mutation/use-respond-to-confirmation.ts:12-32` |
| Request/response contract | `ConfirmationResponseRequest { accept: boolean; reason?: string }`; response `{ success }` | `src/api/event-service/event-service.types.ts:1-8` |
| Endpoint wiring (local vs cloud) | Local: `ConversationClient.respondToConfirmation`; cloud: proxy POST to runtime sandbox `/api/conversations/{id}/events/respond_to_confirmation` with session-key auth | `src/api/event-service/event-service.api.ts:40-69` (path literal at :53) |
| Policy scoping at conversation start | `getConversationConfirmationPolicy`: `NeverConfirm` / `ConfirmRisky(threshold HIGH, confirm_unknown)` / `AlwaysConfirm` | `src/api/agent-server-adapter.ts:593-605` |
| Analyzer selection | `LLMSecurityAnalyzer` / `PatternSecurityAnalyzer` / `PolicyRailSecurityAnalyzer` from settings | `src/api/agent-server-adapter.ts:607-618` |
| Payload fields | `confirmation_policy` required, `security_analyzer` optional in start-conversation payload type | `src/api/agent-server-adapter.ts:1008-1009` |
| Payload assembly | `confirmation_policy:` stamped into every start request; analyzer stamped conditionally | `src/api/agent-server-adapter.ts:1120-1122,1168-1172` |
| Parity test for policy derivation | Test asserts `ConfirmRisky` + `LLMSecurityAnalyzer` payloads | `__tests__/api/agent-server-adapter.test.ts:349-378` (expects at :364-373) |
| Settings surface | `confirmation_mode: boolean` and `security_analyzer: string \| null` on flat Settings | `src/types/settings.ts:128-129` |
| Default off | `confirmation_mode: false` in DEFAULT_SETTINGS (two occurrences) | `src/services/settings.ts:13,56` |
| Settings resolution | `pickFirstBoolean(conversationSettings.confirmation_mode) ?? DEFAULT_SETTINGS.confirmation_mode` | `src/hooks/query/use-settings.ts:89-91` |
| Server-side settings merge | `conversation_settings.confirmation_mode` merged into flat shape | `src/api/settings-service/settings-service.api.ts:401-403` (cloud twin at `src/api/cloud/settings-service.api.ts:110-112`) |
| Verification settings page | Renders SDK-schema `verification` section; deprecates agent-owned duplicates via exclusion set | `src/routes/verification-settings.tsx:9-12,23-41` |
| Schema field metadata (mock) | `confirmation_mode` major prominence, default false; `security_analyzer` depends_on `confirmation_mode` | `src/mocks/settings-handlers.ts:407-441` |
| Enabled indicator | Lock tooltip shown only when `settings.confirmation_mode` true | `src/components/features/chat/confirmation-mode-enabled.tsx:12-14` |
| Input lock during pending | Chat input disabled while `AWAITING_USER_CONFIRMATION` | `src/components/features/chat/interactive-chat-box.tsx:63-66` |
| Attention notification | Sound played on transition into `AWAITING_USER_CONFIRMATION` | `src/hooks/use-agent-notification.ts:8-13,32-46` |
| Refresh survival (REST rehydrate) | Conversation query polls; `execution_status` fallback feeds `useAgentState`; history seeded REST-first then WS | `src/hooks/query/use-active-conversation.ts:19-31`; `src/hooks/query/use-conversation-history.ts:53-54` |
| Waiting counts as "active" | `WAITING_FOR_CONFIRMATION` in ACTIVE_EXECUTION_STATUSES; human-readable status text key | `src/utils/status.ts:6-11,107` |
| Passive indicators | ✅ emoji and "active" dot for waiting conversations | `src/utils/agent-state-emoji.ts:22`; `src/components/features/conversation-panel/conversation-status-dot.tsx:33` |
| Runtime status validation | `"waiting_for_confirmation"` accepted in RUNTIME_STATUSES set | `src/api/conversation-service/agent-server-conversation-service.api.ts:308-315` |
| Failure surfacing | Global MutationCache onError toast covers failed confirm responses | `src/query-client-config.ts:62-73` |
| Mock fixture uses waiting state | MSW conversation #1 seeded `execution_status: "waiting_for_confirmation"` | `src/mocks/conversation-handlers.ts:49` |
| Risk text in transcripts | HIGH/MEDIUM risk appended to bash action card content | `src/components/conversation-events/chat/event-content-helpers/get-action-content.ts:93-98` |
| Per-tool risk label | Bash visualizer shows SECURITY$HIGH_RISK/MEDIUM_RISK tag | `src/components/features/chat/tool-visualizers/bash/bash.tsx:24-39` |

## Answers to Dimension Questions

**1. How is approval requested?**
The agent-server suspends execution and reports `execution_status = "waiting_for_confirmation"`. The frontend derives `AgentState.AWAITING_USER_CONFIRMATION` from that status (`src/hooks/use-agent-state.ts:24-25`), finds the latest agent action event in the event store (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:30-36`), disables the chat input (`src/components/features/chat/interactive-chat-box.tsx:63-66`), optionally plays a sound (`src/hooks/use-agent-notification.ts:8-13`), and renders Confirm/Reject buttons with keyboard shortcuts ⌘↩ / ⇧⌘⌫ (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:109-133`, shortcuts at :61-90). If the pending `ActionEvent.security_risk` is `HIGH`, a red `RiskAlert` banner precedes the buttons (:102-118, `src/components/shared/risk-alert.tsx:18-33`). The decision is sent as `{ accept: boolean }` via `EventService.respondToConfirmation` to `/api/conversations/{id}/events/respond_to_confirmation` (`src/api/event-service/event-service.api.ts:40-69`).

**2. Are approval sessions durable?**
Effectively yes, by construction: the frontend keeps no authoritative approval state. Pending state is re-derived after any reload from (a) the polled conversation record's `execution_status` fallback (`src/hooks/use-agent-state.ts:49-52`, polling configured at `src/hooks/query/use-active-conversation.ts:19-31`) and (b) WebSocket state-update events (`src/contexts/conversation-websocket-context.tsx:638-645`), with the pending action itself recovered through the REST-first event history seed (`src/hooks/query/use-conversation-history.ts:53-54`). So the answer to the dimension's driving question — *can an approval session survive a browser refresh?* — is yes: refresh, reload the conversation, and the approve/reject prompt re-renders if the server still holds the decision. Caveats: the double-submit guard is memory-only (`src/stores/event-message-store.ts:6-27`), so after a refresh the same event could be responded to again if the user clicks twice around a reload; whether the server deduplicates is not observable from this repo. Final durability authority (does the server persist a queued confirmation across restart?) lives in `software-agent-sdk`, out of boundary.

**3. Can approvals be scoped?**
Only at whole-conversation granularity, fixed at creation time. `getConversationConfirmationPolicy` maps two settings booleans into one of three policies: `NeverConfirm` (default), `AlwaysConfirm`, or `ConfirmRisky` with `threshold: "HIGH"` and `confirm_unknown: true` when the LLM analyzer is selected (`src/api/agent-server-adapter.ts:593-605`). There is no per-tool, per-command-pattern, "allow once vs allow always for this tool," or mid-conversation scope change visible in the frontend; once a conversation starts with `AlwaysConfirm`, every actionable tool call pauses (the UI has no "don't ask again" affordance). The only intra-scope differentiation is *informational*: risk labels on cards (`src/components/features/chat/tool-visualizers/bash/bash.tsx:24-39`) and the high-risk alert. The `security_analyzer` choice (`llm` | `pattern` | `policy_rail`) tunes which actions are deemed risky under `ConfirmRisky` (`src/api/agent-server-adapter.ts:607-618`), which is the closest thing to scoped approval.

**4. Do approvals time out?**
No evidence found. Searched all of `src/` for `timeout` in the confirmation paths (`use-respond-to-confirmation.ts`, `event-service/`) and for `expire|expiry|ttl` globally — the only expiry code concerns MCP credentials and LLM subscriptions (`src/utils/mcp-credential-validation.ts:38`, `src/api/llm-subscription-service.ts:19-148`), not approvals. The waiting status is even treated as a stable "active" state for list dots, tab-title emoji, and polling (`src/utils/status.ts:6-11`, `src/utils/agent-state-emoji.ts:22`), implying the design expects a pending approval to sit indefinitely until answered or until the process is stopped/paused through other means. Any server-side deadline would be invisible here.

**5. Are approvals audited?**
Not within this repository. There is no approval-specific audit log, telemetry event, or decision-history UI; `use-tracking.ts` contains no confirmation events (searched `approval|confirm` in hooks — none). Auditability is implicit and indirect: decisions change server-side conversation state and the eventual observation/error events land in the persisted event stream that the UI replays (`src/api/event-service/event-service.api.ts:102-181`), and rejected/confirmed outcomes surface only as generic state transitions (`USER_REJECTED` exists as an enum member but is rendered nowhere except a button tooltip string, `src/components/shared/action-tooltip.tsx:20-22`). Whether the server records who approved what and when cannot be verified from this source.

## Architectural Decisions

- **Server-authoritative approval state, client-derived UI.** The frontend never stores "an approval is pending"; it projects `execution_status` + the latest agent event into UI (`src/hooks/use-agent-state.ts:45-59`, `src/components/shared/buttons/conversation-confirmation-buttons.tsx:30-36`). This makes refresh-resilience free but couples correctness entirely to server semantics this repo doesn't control.
- **Policy declared at conversation creation, not per action.** `confirmation_policy` and `security_analyzer` are fields of the start-conversation payload (`src/api/agent-server-adapter.ts:1008-1009,1120-1122`), meaning scope changes require a new conversation — a deliberate simplification that trades flexibility for predictability.
- **Risk assessment pushed to the LLM/analyzer side.** `ActionEvent.security_risk` is annotated as the "LLM's assessment" (`src/types/agent-server/core/events/action-event.ts:58-61`), and the frontend merely visualizes it; the `ConfirmRisky` threshold logic runs server-side against analyzer output.
- **Single endpoint, dual transports.** The same logical call routes through the typed `ConversationClient` locally or the cloud-proxy envelope to the runtime sandbox with session-key auth (`src/api/event-service/event-service.api.ts:46-68`), keeping auth topology out of components.
- **Settings schema-driven configuration UI.** Confirmation knobs render generically from the backend-provided schema (`SdkSectionPage`, `src/routes/verification-settings.tsx:23-41`), including a dependency edge making the analyzer choice conditional on confirmation mode (mocked shape at `src/mocks/settings-handlers.ts:407-441`); legacy agent-owned duplicates are explicitly excluded (:9-12).

## Notable Patterns

- **Keyboard-first confirmation UX**: document-level shortcuts with `preventDefault` and cleanup (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:61-90`) mirror terminal-style approve flows.
- **Optimistic dedup via submitted-id set**: the component records the pending event id before the mutation resolves to prevent double clicks (`:44-47`), a lightweight pattern that unfortunately doesn't survive reloads.
- **State projection layer**: a pure mapping function from wire `ExecutionStatus` to UI `AgentState` (`src/hooks/use-agent-state.ts:10-35`) keeps rendering logic decoupled from API enums; the same waiting status also drives emoji/tab-title/dot indicators (`src/utils/agent-state-emoji.ts:22`, `src/components/features/conversation-panel/conversation-status-dot.tsx:33`).
- **Parity testing against upstream semantics**: the adapter test is named "derives confirmation and security settings the same way as OpenHands" and pins exact policy payloads (`__tests__/api/agent-server-adapter.test.ts:349-378`), treating cross-repo consistency as a contract.
- **Attention orchestration**: sound notification on entering the waiting state (`src/hooks/use-agent-notification.ts:32-46`) plus input locking (`src/components/features/chat/interactive-chat-box.tsx:63-66`) form a small "modal-ish" focus pattern without an actual modal.

## Tradeoffs

- **Durability by delegation**: deriving everything from server state yields refresh-survival without persistence code, but means the frontend can't answer basic questions locally (e.g., how long has this been pending? will it expire?) and can show stale prompts if the WS drops and the poll lags up to 30s (`src/hooks/query/use-active-conversation.ts:19-31`).
- **Coarse scoping vs. friction**: `AlwaysConfirm` maximizes safety but forces a round-trip per action; `NeverConfirm` (the default, `src/services/settings.ts:13`) removes all friction and all protection. No middle ground like per-tool allowlists exists in this surface.
- **Memory-only dedup**: cheap and reactive, but a refresh clears the guard, reintroducing duplicate-response risk around exactly the moment (reload while pending) users are likely to retry.
- **Generic schema UI vs. discoverability**: rendering confirmation settings from backend schemas keeps the client thin, but the mock shows `confirmation_mode` hidden in the "basic" view (`__tests__/routes/verification-settings.test.tsx:98-100`), so many users may never find the safety switch.
- **Unused rejection reason**: the wire contract supports `reason` (`src/api/event-service/event-service.types.ts:3`) but the UI hardcodes `{ accept }` (`src/hooks/mutation/use-respond-to-confirmation.ts:21-23`) — foregone signal for both agent feedback and audit trails.

## Failure Modes / Edge Cases

- **Lost response**: if the POST fails, the global MutationCache shows a toast (`src/query-client-config.ts:62-73`) and the buttons remain (render gate only suppresses after local submission, and only while the store remembers), but there is no targeted retry/backoff; TanStack mutations do not retry by default.
- **Duplicate response across refresh**: `submittedEventIds` resets on reload (`src/stores/event-message-store.ts:15`), so a user can submit a second response for the same pending event; downstream handling depends on unverified server idempotency.
- **Ambiguous pending-action detection**: the "awaiting action" finder returns the newest agent event regardless of kind whenever the state flag is set (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:30-36`) — it checks `source === "agent"`, not that the event actually requires confirmation; correctness relies on the server only entering the waiting state for confirmable actions.
- **Stuck-looking conversations**: since waiting is classified as "active" (`src/utils/status.ts:6-11`) and rendered with a green ✅ (`src/utils/agent-state-emoji.ts:22`), an abandoned pending approval is indistinguishable from success in conversation lists — no escalation, reminder, or stale marker exists.
- **Cloud pagination degradation near approvals**: if the cloud backend lacks timestamp-filter support, history backfill silently stops (`src/api/event-service/event-service.api.ts:149-163`); a pending action older than the first page might fail to rehydrate its card after refresh, leaving status "waiting" without visible buttons (buttons require both state and a found action event).
- **Legacy schema duplication**: agent-owned `verification.confirmation_mode` still exists for back-compat and must be actively excluded to avoid double-rendering controls (`src/routes/verification-settings.tsx:4-12`); a backend reverting that deprecation would resurface conflicting toggles.

## Future Considerations

- Add a pending-approval age indicator and optional timeout/expiry UX (the status pipeline already streams updates needed to drive a countdown; nothing in `src/` models time-in-waiting today).
- Persist or server-mirror the submitted-event set (or rely on explicit server idempotency guarantees) to close the refresh double-submit window.
- Wire the existing `reason` field to an optional reject-with-feedback input, improving both agent steering and future auditing.
- Introduce finer scoping: per-tool or per-session "always allow" grants derived from `security_risk` thresholds, which the current policy kinds could accommodate without protocol changes.
- Add direct unit tests for `ConversationConfirmationButtons` (submit gating, shortcut handling, high-risk banner) — currently the approval UI has no dedicated coverage in `__tests__/`.
- Emit a consent-style analytics event for approve/reject decisions through the sanctioned `useTracking` pattern if product needs adoption metrics for confirmation mode.

## Questions / Gaps

- Does the agent-server persist a queued confirmation decision durably (survive server restart), and does it deduplicate multiple `respond_to_confirmation` calls? Not answerable from this repo — the enforcement lives in `software-agent-sdk` (per `AGENTS.md` repository map).
- Is there any server-side deadline after which a waiting conversation errors or auto-rejects? No evidence found in this source; searched for timeout/TTL/expiry semantics across `src/` with no confirmation-related hits.
- Who approved what, when: no audit trail exists client-side; whether the server's persisted event stream records the decision as a distinct event type is unverifiable within this isolation boundary.
- What happens visually after a *rejection* is accepted by the server (which event replaces the pending action card)? `USER_REJECTED` (`src/types/agent-state.tsx:14`) is declared but never consumed as a state in this codebase; the post-rejection transcript shape is server-defined.
- The `ConfirmRisky` `threshold` is hardcoded to `"HIGH"` with `confirm_unknown: true` (`src/api/agent-server-adapter.ts:600-602`); whether the SDK exposes configurable thresholds is unknown from here.

---

Generated by `dimensions/14.02-approval-session-design` against `openhands`.

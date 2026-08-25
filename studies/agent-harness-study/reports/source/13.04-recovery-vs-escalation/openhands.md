# Source Analysis: openhands

## Dimension 13.04: Recovery vs Escalation

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (OpenHands "agent-canvas" frontend) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19, Zustand stores, TanStack Query, WebSocket, `@openhands/typescript-client` |
| Analyzed | 2026-08-25 |

> All citations below use paths relative to the selected source root `studies/agent-harness-study/sources/openhands/`.

## Summary

This source is the OpenHands agent-canvas **frontend**, so recovery-vs-escalation is implemented client-side: the harness retries transient transport failures automatically within bounded budgets, and escalates anything "decided" or exhausted to the human user through typed error surfaces with explicit recovery actions.

The core model is a two-tier error taxonomy. Connection errors (`errorType: "connection"`) auto-clear when connectivity recovers and are backed by automatic retry: WebSocket reconnection with exponential backoff + jitter capped at 30s (`src/hooks/use-websocket.ts:18-20,125-136`), a generic `withRetry` helper with exponential backoff for settings/secrets API calls (`src/api/with-retry.ts:4-26`), and quick-retried health probes (`src/hooks/query/use-backends-health.ts:140-186`). Conversation errors are sticky by design — they clear only on an explicit user action such as dismiss/retry/re-auth (`src/stores/error-message-store.ts:4-9,62-65`).

Escalation to the human happens through several purpose-built channels: an error banner that picks code-specific recovery actions (`ACPAuthRequired` → "update credentials" button navigating to agent settings, `src/utils/acp-error-codes.ts:8-28` + `src/components/features/chat/chat-interface.tsx:583-599`); a full backend-recovery modal screen when the active backend is unreachable (`src/root.tsx:127-159`); and an opt-in human-in-the-loop confirmation gate where the agent pauses in `WAITING_FOR_CONFIRMATION` state until the user confirms or rejects a pending action with keyboard shortcuts and a high-risk alert (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:38-118`). The escalation policy itself is configurable at the settings level: `confirmation_mode` plus `security_analyzer` map onto server policies `NeverConfirm`, `AlwaysConfirm`, or `ConfirmRisky(threshold: HIGH)` (`src/api/agent-server-adapter.ts:593-605`).

Graceful stopping is first-class: a pause mutation patches both `execution_status` and `sandbox_status` optimistically and navigates away (`src/hooks/mutation/use-unified-stop-conversation.ts:52-71`), cloud sandboxes auto-resume from `PAUSED` via a dedicated endpoint with duplicate-trigger guards (`src/routes/conversation.tsx:143-178`), and archived/errored sandboxes degrade to a read-only banner instead of a dead chat input (`src/components/features/chat/chat-interface.tsx:604-621`).

## Rating

**7 / 10**

Rationale against the rubric:

- **Why not lower:** The retry-vs-escalate boundary is explicit and tested. Non-retryable failures are deliberately classified (auth errors skip probe retries so the recovery UI appears faster, `src/hooks/query/use-backends-health.ts:160-171`); connection errors only surface after a previously successful connect to avoid bootstrap noise (`src/contexts/conversation-websocket-context.tsx:987-993`). Watchdogs convert hangs into visible errors (10s handshake watchdog `src/utils/websocket-handshake.ts:17-25`; 150s pending-message watchdog `src/stores/optimistic-user-message-store.ts:14,131-139`). A persisted circuit breaker stops polling after 5 consecutive backend-probe failures (`src/api/backend-registry/health-storage.ts:10`). Test coverage is direct: backoff spacing (`__tests__/hooks/use-websocket.test.ts:662`), watchdog flip/consumption races (`__tests__/stores/optimistic-user-message-store.test.ts:201-247`), transient-failure recovery and disabled-state persistence (`__tests__/hooks/query/use-backends-health.test.tsx:135,270,302,371`), and sticky-vs-transient error semantics (`__tests__/stores/error-message-store.test.ts:34-48`).
- **Why not higher:** Retry/escalation thresholds are scattered hardcoded constants rather than one policy module; there are two byte-identical `withRetry` implementations (`src/api/with-retry.ts:4` vs `src/api/settings-service/settings-service.api.ts:132-156`) that can drift; `STUCK` execution status is silently collapsed into `ERROR` ("Map STUCK to ERROR for now", `src/hooks/use-agent-state.ts:30-31`) with no distinct recovery path; and auditing of recovery decisions relies on consent-gated PostHog telemetry plus localStorage health entries rather than a durable audit log.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Generic bounded retry helper (default 3 attempts, 500ms exponential base) | `withRetry<T>` loops `maxRetries`, delays `baseDelayMs * 2**attempt`, rethrows last error | `src/api/with-retry.ts:4-26` |
| Duplicated retry helper (settings service keeps its own copy) | identical implementation used for all settings CRUD calls | `src/api/settings-service/settings-service.api.ts:132-156` |
| Secrets service retries then logs "Failed to fetch secrets after retries" | `console.error` after `withRetry` exhaustion | `src/api/secrets-service.ts:11,38` |
| WebSocket reconnect policy: enabled per-consumer, `maxAttempts` option defaults to Infinity | options interface + fallback logic | `src/hooks/use-websocket.ts:12-15,112-114` |
| WS exponential backoff 1s→30s cap with ≤30% jitter to de-synchronize parallel sockets | comment explains anti-lockstep rationale | `src/hooks/use-websocket.ts:18-20,125-136` |
| Handshake watchdog aborts sockets stuck CONNECTING (10s) so retries can proceed | `startHandshakeWatchdog` closes socket; close flows into normal reconnect path | `src/utils/websocket-handshake.ts:5,17-25` |
| Error taxonomy: "connection" auto-clears on recovery; "conversation" sticky until dismiss/retry/new message | store doc comment + `clearConnectionError` guard | `src/stores/error-message-store.ts:4-9,62-65` |
| Pending user message watchdog: 150s in "sending" flips to "error" with retry link | `PENDING_MESSAGE_TIMEOUT_MS` + timeout closure | `src/stores/optimistic-user-message-store.ts:14,131-139` |
| Connection-error banner only shown if a previous connect succeeded | `hasConnectedRefMain.current` gate in `onError` | `src/contexts/conversation-websocket-context.tsx:987-993` |
| ConversationErrorEvent/ServerErrorEvent → sticky banner + structured code/classification | `setErrorMessage(detail, "conversation", code, classification)` | `src/contexts/conversation-websocket-context.tsx:570-591` |
| LLM/tool errors are NOT escalated to a banner; rendered inline in chat and tracked for analytics | `isAgentErrorEvent` branch keeps them out of the banner | `src/contexts/conversation-websocket-context.tsx:596-608` |
| Send fallback: when WS is closed, messages queue via REST `sendEvent(..., { run: true })` instead of failing | REST queue path returns `{ queued: true }` | `src/contexts/conversation-websocket-context.tsx:1100-1128` |
| Banner wiring: Retry button only for connection errors (reconnect); Reauth button for `ACPAuthRequired` codes | conditional `onRetry` / `onReauth` props | `src/components/features/chat/chat-interface.tsx:583-599` |
| Error banner UI actions: retry, copy, dismiss, view-more, re-auth | buttons with test ids `error-message-banner-*` | `src/components/features/chat/error-message-banner.tsx:141-207` |
| Structured ACP error codes mapped to headers; credential failure flagged for re-auth | `ACP_AUTH_REQUIRED_CODE = "ACPAuthRequired"` | `src/utils/acp-error-codes.ts:8-29` |
| Escalation-policy mapping sent to the server: `NeverConfirm` / `AlwaysConfirm` / `ConfirmRisky(HIGH, confirm_unknown)` | `getConversationConfirmationPolicy` keyed off `confirmation_mode` + `security_analyzer === "llm"` | `src/api/agent-server-adapter.ts:593-605` |
| `confirmation_mode` default false in frontend settings defaults | `DEFAULT_SETTINGS` | `src/services/settings.ts:13` |
| Lock-icon indicator when confirmation mode is active | `ConfirmationModeEnabled` reads `settings.confirmation_mode` | `src/components/features/chat/confirmation-mode-enabled.tsx:12-25` |
| Execution status enum includes `WAITING_FOR_CONFIRMATION`, `PAUSED`, `ERROR`, `STUCK` | `ExecutionStatus` enum | `src/types/agent-server/core/base/common.ts:67-75` |
| Security risk levels drive confirmation UX severity | `SecurityRisk` enum (UNKNOWN…HIGH) | `src/types/agent-server/core/base/common.ts:59-64` |
| Status→AgentState mapping; STUCK collapsed to ERROR | switch in `mapExecutionStatusToAgentState` | `src/hooks/use-agent-state.ts:24-31` |
| Human confirm/reject UI with ⇧⌘⌫ reject and ⌘↩ confirm shortcuts, duplicate-submit guard, high-risk alert | `ConversationConfirmationButtons` | `src/components/shared/buttons/conversation-confirmation-buttons.tsx:38-118` |
| Confirmation decision POSTed to runtime sandbox (`respond_to_confirmation`), cloud-aware routing | `EventService.respondToConfirmation` local/cloud branches | `src/api/event-service/event-service.api.ts:40-69` |
| Mutation wrapper for the confirmation response | `useRespondToConfirmation` | `src/hooks/mutation/use-respond-to-confirmation.ts:12-32` |
| Sound + tab-title notification when entering attention states incl. awaiting confirmation | `NOTIFICATION_STATES` array | `src/hooks/use-agent-notification.ts:6-10` |
| Graceful stop: pause mutation with loading toast, rollback on error, optimistic cache patch of both status fields | `useUnifiedPauseConversation` | `src/hooks/mutation/use-unified-stop-conversation.ts:16-73` |
| Cloud sandbox auto-resume on `PAUSED` via `POST .../sandboxes/{id}/resume`, ref-guarded per conversation id | resume effect replacing wrong start-task approach | `src/routes/conversation.tsx:143-178` |
| WS suppressed while `sandbox_status === "PAUSED"` to avoid stale-host connects; active conversation fast-polls 3s | wrapper gate + refetch interval predicate | `src/contexts/websocket-provider-wrapper.tsx:31`, `src/hooks/query/use-active-conversation.ts:28-29` |
| Backend health probes: 2 quick retries @300ms; auth failures explicitly non-retryable so recovery UI shows sooner | `probeBackendWithQuickRetry` + `isRetryableProbeError` | `src/hooks/query/use-backends-health.ts:140-186` |
| Circuit breaker: polling stops after 5 consecutive failures; state persists across refresh; one-shot re-probe option | `MAX_CONSECUTIVE_FAILURES` + validated localStorage entries | `src/api/backend-registry/health-storage.ts:10,24-39`, `src/hooks/query/use-backends-health.ts:203-210` |
| Full-screen backend recovery screen (Manage Backends modal in `recoveryMode`, unclosable) when `/server_info` fails | `MissingAgentServerScreen` re-probes on backend change | `src/root.tsx:127-159` |
| Launch-time fallback: git-worktree creation failure retries child launch with shared isolation + explanatory note | catch-and-degrade around `createChild(isolation)` | `src/services/child-conversation-launch.ts:303-323` |
| Bounded poll for async Cloud provisioning (deadline-based), ERROR surfaced instead of hanging | `waitForCloudConversationId` | `src/services/child-conversation-launch.ts:357-384` |
| Failed cloud provisioning escalates with actionable guidance: report to user, retry, or fall back to `target="local"` | failure message text | `src/services/child-conversation-launch.ts:425-430` |
| Bootstrap query retry suppressed for unavailable/auth errors (no blind retries against a dead server) | custom `retry` predicate | `src/hooks/query/use-config.ts:17-21` |
| Archived / sandbox-error conversations stop accepting input; read-only banner replaces composer, history preserved | `archived-conversation-banner` branch | `src/components/features/chat/chat-interface.tsx:604-621` |
| Recovery-outcome telemetry: `error_outcome` events with `error_kind`, correlatable `error_id`, no raw messages | `trackError` reserved-key handling | `src/utils/error-handler.ts:17-47` |
| Tests: backoff spacing capped at 30s | `spaces reconnect attempts with exponential backoff capped at 30s` | `__tests__/hooks/use-websocket.test.ts:662` |
| Tests: watchdog flips stuck "sending" to error; echo/failure races handled | three watchdog cases | `__tests__/stores/optimistic-user-message-store.test.ts:201-247` |
| Tests: transient probe failure recovers; failure count persisted; refresh does not re-arm disabled backend; edit re-arms | four scenarios | `__tests__/hooks/query/use-backends-health.test.tsx:135,270,302,336,371` |
| Tests: connection vs conversation error stickiness semantics | `clearConnectionError preserves a sticky conversation error` etc. | `__tests__/stores/error-message-store.test.ts:34-48` |

## Answers to Dimension Questions

### 1. When does the system retry vs escalate?

Retry is reserved for **transient transport-level failures** and is always bounded:

- WebSocket drops reconnect with exponential backoff (1s→30s cap, ≤30% jitter) indefinitely while the component is mounted (`reconnect.maxAttempts` defaults to `Infinity`, `src/hooks/use-websocket.ts:113-114`; main/planning providers enable it without a cap, `src/contexts/conversation-websocket-context.tsx:978,1017`). A handshake hung in `CONNECTING` is force-closed after 10s so its close event flows into the same reconnect path (`src/utils/websocket-handshake.ts:20-23`).
- Settings/secrets REST calls retry up to 3 times with 500ms-base exponential delay (`src/api/with-retry.ts:4-26`).
- Backend health probes retry twice quickly (300ms) but **skip retries for decided auth failures** (invalid key, missing key, logged out) because "they are a decided server response, not a transient miss" and retrying would only delay the correct disconnected verdict (`src/hooks/query/use-backends-health.ts:160-186`).

Escalation to the human happens when the failure is decided or automatic attempts are exhausted: sticky conversation-error banners with per-code recovery actions (`src/components/features/chat/chat-interface.tsx:583-599`), the unclosable backend-recovery modal (`src/root.tsx:133-159`), a 150s send watchdog flipping a stuck bubble to an error with a manual retry link (`src/stores/optimistic-user-message-store.ts:131-139`), and read-only degradation for archived/errored sandboxes (`src/components/features/chat/chat-interface.tsx:604-621`). Notably, LLM/tool-call errors inside the loop are *not* escalated as banners — they render inline in chat and the frontend leaves recovery to the agent loop, only tracking them for analytics (`src/contexts/conversation-websocket-context.tsx:596-608`).

### 2. Are escalation thresholds configurable?

Partially, at two distinct levels:

- **User-configurable (settings-driven):** the action-escalation policy is genuinely configurable. `confirmation_mode` (default `false`, `src/services/settings.ts:13`) combined with `security_analyzer` produces `NeverConfirm`, `AlwaysConfirm`, or `ConfirmRisky` with `threshold: "HIGH"` and `confirm_unknown: true` sent to the agent server (`src/api/agent-server-adapter.ts:593-605`). Sound notifications for attention states are also opt-in (`src/hooks/use-agent-notification.ts:32`).
- **Developer-configurable only:** transport thresholds are function parameters or module constants, not operational config — `maxRetries`/`baseDelayMs` args of `withRetry` (`src/api/with-retry.ts:6-7`), `reconnect.maxAttempts` hook option (`src/hooks/use-websocket.ts:12-15`, unset by production consumers), `PROBE_RETRY_ATTEMPTS = 2` / `PROBE_RETRY_DELAY_MS = 300` (`src/hooks/query/use-backends-health.ts:140-141`), `PENDING_MESSAGE_TIMEOUT_MS = 150_000` (`src/stores/optimistic-user-message-store.ts:14`), `MAX_CONSECUTIVE_FAILURES = 5` (`src/api/backend-registry/health-storage.ts:10`). No evidence of these being exposed via env vars or settings schemas (searched `retry`, `maxAttempt`, `threshold` across `src/`; hits were all code constants or React Query options).

### 3. Can the system stop gracefully?

Yes, at multiple layers. The pause mutation cancels in-flight queries, shows progress, rolls back the cache on error, and optimistically patches both `execution_status: PAUSED` and `sandbox_status: "PAUSED"` so downstream consumers react immediately (`src/hooks/mutation/use-unified-stop-conversation.ts:25-70`). While paused, the WebSocket provider suppresses connections to the stale sandbox host (`src/contexts/websocket-provider-wrapper.tsx:31`) and the active-conversation poll switches to a fast 3s interval to detect resume (`src/hooks/query/use-active-conversation.ts:28-29`); reopening triggers an idempotent, deduplicated resume call (`src/routes/conversation.tsx:158-170`). Deliberate teardown paths exist in the socket layer itself: `disconnect()` removes the socket from the reconnect allow-list before closing so unmount never triggers reconnection (`src/hooks/use-websocket.ts:200-212,164-188`). Provisioning polls are deadline-bounded and report a still-provisioning task instead of hanging forever (`src/services/child-conversation-launch.ts:357-384`).

### 4. Are recovery decisions auditable?

Weakly, and only client-side. Three mechanisms exist: (a) structured `error_outcome` telemetry events carrying `error_source`, `error_kind`, and a correlation `error_id` promoted from the server classification, deliberately excluding raw messages (`src/utils/error-handler.ts:27-46`, wired for every conversation/planning error at `src/contexts/conversation-websocket-context.tsx:578-585,599-607,811-818,832-840`); (b) a persisted per-backend health record (`consecutiveFailures`, `lastError` truncated to 500 chars, `lastFailureAt`, `disabled`) in localStorage under `openhands-backend-health`, validated on read against tampering (`src/api/backend-registry/health-storage.ts:12-20,24-39`); (c) `console.error` after secrets-retry exhaustion (`src/api/secrets-service.ts:38`). There is **no dedicated append-only audit log of recovery attempts or escalation decisions**, and the telemetry channel is consent-gated (see AGENTS.md tracking architecture), so auditability is best-effort rather than guaranteed. No evidence found of any server-side recovery audit consumption in this repo (search boundary: `audit`, `log.*recovery`, `trackError` usages within `src/`).

## Architectural Decisions

1. **Two-tier error lifecycle instead of one error flag.** Splitting `connection` (transient, auto-clearing) from `conversation` (sticky, user-dismissed) errors (`src/stores/error-message-store.ts:9,62-65`) lets the UI retry silently in the first case and demand human action in the second, which is the backbone of the retry-vs-escalate split.
2. **Classify-before-retrying.** Auth/version failures are excluded from retry paths so the escalation surface appears immediately (`src/hooks/query/use-config.ts:17-21`, `src/hooks/query/use-backends-health.ts:160-171`) — an explicit trade-off favoring fast, truthful escalation over hopeful retries.
3. **Escalation-to-human modeled as execution status, not a modal interrupt.** `WAITING_FOR_CONFIRMATION` arrives as a conversation-state event, maps to `AgentState.AWAITING_USER_CONFIRMATION` (`src/hooks/use-agent-state.ts:24-25`), and renders inline confirm/reject controls attached to the last message (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:92-135`). The decision round-trips through a dedicated runtime endpoint (`src/api/event-service/event-service.api.ts:40-69`).
4. **Policy computed on the client, enforced on the server.** The frontend translates user settings into a declarative confirmation policy payload (`NeverConfirm`/`AlwaysConfirm`/`ConfirmRisky`) rather than gating locally (`src/api/agent-server-adapter.ts:593-605`), keeping enforcement with the agent loop.
5. **Circuit breaker with persistence for unreachable infrastructure.** After 5 consecutive failed probes, polling stops and stays stopped across refreshes until user action or an explicit one-shot re-probe (`src/api/backend-registry/health-storage.ts:5-10`, `src/hooks/query/use-backends-health.ts:203-210`).
6. **Hang-to-error conversion via watchdogs.** Both the socket handshake (10s) and optimistic message sends (150s) have timeouts that convert indefinite waits into visible, actionable errors (`src/utils/websocket-handshake.ts:17-25`, `src/stores/optimistic-user-message-store.ts:131-139`).

## Notable Patterns

- **Backoff with jitter to protect struggling servers**: the jitter exists specifically "so parallel sockets (main + planning) don't retry in lockstep and hammer an already-struggling server every few seconds forever" (`src/hooks/use-websocket.ts:125-132`).
- **Fallback degradation over failure**: worktree-isolation launch failure silently downgrades to shared workspace with an explanatory note rather than losing the launch (`src/services/child-conversation-launch.ts:303-323`); cloud provisioning failure tells the agent/user to retry or fall back to a local target (`src/services/child-conversation-launch.ts:425-430`); cloud event pagination falls back from filtered to limit-only requests on server 500s (`src/api/event-service/event-service.api.ts:115-126`).
- **Queue-instead-of-fail message delivery**: with the socket closed, sends fall back to a REST queue endpoint so messages survive reconnection windows (`src/contexts/conversation-websocket-context.tsx:1100-1120`).
- **Echo-based optimistic reconciliation**: pending bubbles clear by matching echoed content with FIFO fallback, scoped per conversation, so out-of-order or munged echoes cannot strand UI state (`src/stores/optimistic-user-message-store.ts:169-198`).
- **Replaced-socket suppression**: late close/error events from deliberately replaced sockets are ignored so stale failures don't clobber a healthy replacement's state (`src/hooks/use-websocket.ts:101-108,142-148`).
- **Keyboard-first human escalation**: confirmation supports ⇧⌘⌫ (reject) and ⌘↩ (confirm) shortcuts alongside buttons, with a submitted-event-id set preventing double submission (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:44-90`).

## Tradeoffs

- **Unbounded WS reconnects vs bounded everything else**: main/planning sockets pass `reconnect: { enabled: true }` without `maxAttempts` (`src/contexts/conversation-websocket-context.tsx:978`), relying on the 30s cap to bound load; the option exists but no consumer sets it, so the "give up and escalate" threshold for sockets is effectively never.
- **Duplicated retry helpers**: `src/api/with-retry.ts:4-26` and `src/api/settings-service/settings-service.api.ts:132-156` are byte-identical; a future policy change (e.g., adding jitter or retry-after honoring) must be made twice.
- **Silent status collapse**: mapping `STUCK` → `ERROR` loses the distinction between "the loop reports being stuck" and "hard failure", acknowledged only by an inline comment (`src/hooks/use-agent-state.ts:30-31`).
- **Telemetry-dependent auditability**: recovery analytics are meaningful only when the user consents to telemetry, and `error_outcome` intentionally omits raw messages (`src/utils/error-handler.ts:42-46`), trading diagnosability for privacy.
- **Client-side fallbacks can mask server bugs**: the cloud pagination downgrade on 500s (`src/api/event-service/event-service.api.ts:115-126`) keeps the UI working but hides the underlying defect unless separately monitored.

## Failure Modes / Edge Cases

- **Send-timeout false positive**: a message whose echo is delayed past 150s flips to "error" even though the server may still process it; a manual retry could then double-deliver, mitigated only by echo-content dedup (`consumeMatchingPendingMessage`, `src/stores/optimistic-user-message-store.ts:169-198`) — the oldest-entry FIFO fallback means a munged echo pops the wrong bubble in multi-pending scenarios.
- **Circuit-breaker staleness**: a backend that was disabled after 5 failures stays disabled across sessions until the user edits config or opens Manage Backends (`probeDisabledOnce`), so self-healed servers remain red until human action (`src/hooks/query/use-backends-health.ts:203-210,302-334` tests).
- **Planning-agent event-count failure**: a failed count fetch falls back to marking history as loaded to avoid infinite skeletons (`src/contexts/conversation-websocket-context.tsx:1040-1043`), which can silently truncate visible planning history.
- **Handshake watchdog vs slow networks**: sockets legitimately taking >10s to handshake are closed and counted as reconnect attempts; combined with Infinity max attempts this recovers, but each abort consumes a backoff cycle.
- **Sticky-error lockout**: a sticky `conversation` error is never cleared by connectivity recovery (`clearConnectionError` is type-guarded, `src/stores/error-message-store.ts:62-65`); if the user misses the banner, the session appears broken even after the underlying issue resolves.
- **Duplicate-submission guard is memory-only**: `submittedEventIds` lives in a Zustand store (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:17-22`); a remount mid-wait resets it, allowing a repeated confirm click.

## Future Considerations

- Consolidate the two `withRetry` copies into a single shared module with named retry policies (transport vs probe vs save) so thresholds live in one auditable place.
- Give `STUCK` its own recovery affordance instead of aliasing `ERROR` (`src/hooks/use-agent-state.ts:30-31`) — e.g., a "resume/restart loop" prompt.
- Plumb `reconnect.maxAttempts` (and an on-exhaustion callback) through the conversation providers so socket give-up behavior matches the deliberate bounds used elsewhere.
- Add a durable, exportable local log of recovery/escalation decisions (probe outcomes, confirmation accept/reject, watchdog firings) to make question 4 answerable without telemetry consent.
- Surface the health circuit-breaker state in the recovery UI timeline (currently only `lastError`/counts in localStorage, invisible to users).

## Questions / Gaps

- No evidence found of escalation beyond the local user (email/Slack/webhook notification of failures) — search for `escalat`, `notify`, `webhook` in `src/` returned only UI-local notification sound/toast paths; such behavior would live in the sibling agent-server repo, outside this source's boundary.
- Whether the server honors `ConfirmRisky.confirm_unknown` and how risk levels are assigned (`security_risk` on ActionEvents, `src/components/shared/buttons/conversation-confirmation-buttons.tsx:102-105`) is determined in `software-agent-sdk`, not observable here.
- No dedicated recovery audit log was found (see answer 4). Searched `audit`, `recovery`, `history` across `src/` and `__tests__/`; nearest matches were the localStorage health store and PostHog `error_outcome` events.
- Retry behavior of the automation-backend health queries is intentionally none (`retry: false` to show degraded state immediately, `src/hooks/query/use-automation-health.ts:13`) — whether automations retry server-side is out of this repo's visibility.

---

Generated by `13.04-recovery-vs-escalation` against `openhands`.

# Source Analysis: openhands

## Dimension 07.08: Tool Failure Escalation

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React (OpenHands "agent-canvas" frontend; tool execution itself lives in the separate `software-agent-sdk` agent-server) |
| Analyzed | 2026-08-25 |

## Summary

This repository is the OpenHands frontend, so its role in tool-failure escalation is threefold: (1) it **observes and renders** failures the backend emits as events (`AgentErrorEvent`, `ConversationErrorEvent`, `ServerErrorEvent`, and per-observation error flags), (2) it **executes two client-side tools** (`canvas_ui` acknowledgement and `launch_child_conversation`) whose failures must be converted into model-facing corrective envelopes, and (3) it owns **escalation UI**: inline chat error cards, a banner with retry/reauth/copy/dismiss actions, toasts, a human confirmation loop (`respond_to_confirmation`), WebSocket reconnection, and a persisted per-backend health circuit breaker.

The failure taxonomy is explicit at every layer. Observation types carry per-kind error signals (`is_error`, `error` strings, exit codes/timeout flags — `src/types/agent-server/core/base/observation.ts:9-362`), which the UI normalizes into a single `"success" | "error" | "timeout"` verdict (`src/components/conversation-events/chat/event-content-helpers/get-observation-result.ts:22-69`). Conversation-level errors carry a structured `code` plus optional `ErrorClassification` (`kind`, `retryable`, `user_action`, `error_id`) that drives both recovery actions and telemetry grouping. Client-executed tools never reject: every failure becomes a `{status: "error", error, guidance}` envelope handed back to the model as a message (`src/services/child-conversation-launch.ts:68-89, 459-497`). A failed tool is generally a recovery path here — the model gets actionable guidance, the user gets retry/reauth affordances, and only connectivity exhaustion degrades into a disabled, persisted circuit breaker.

Scope caveat: the harness's core escalation decision — whether a failed tool result is fed back to the LLM for self-correction — happens server-side in `software-agent-sdk` and is out of this repo's boundary. This analysis covers everything visible from the frontend.

## Rating

**Score: 7/10** — Clear, typed failure model with tests, explicit escalation interfaces (banner actions, confirmation loop, classification-driven telemetry), and operational safeguards (jittered reconnect backoff, probe circuit breaker persisted in localStorage). It falls short of 8-9 because: retries are never automatic for failed tool calls (delegated entirely to the model/user), the generic API helper throws an unstructured `Error("Retry attempts exhausted")` (`src/api/with-retry.ts:25`), `classification.retryable` exists but is not consumed by any UI retry action, and operator alerting beyond PostHog analytics is absent (expected for a frontend, but unprovable from this source alone).

## Evidence Collected

Every entry cites file paths with line numbers relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Per-kind error flags on observations | `MCPToolObservation.is_error`; `FinishObservation.is_error`; `TerminalObservation.is_error` + `timeout`; `ExecuteBashObservation.error`/`timeout`/`exit_code` (-1 = soft timeout); `FileEditorObservation.error`; `BrowserObservation.error` | src/types/agent-server/core/base/observation.ts:17, 32, 100-104, 69-77, 145, 50 |
| Normalized failure status mapping | `getObservationResult()` maps all observation kinds to `"success" \| "error" \| "timeout"` (bash exit code −1 → timeout; non-zero → error; editor `observation.error` → error) | src/components/conversation-events/chat/event-content-helpers/get-observation-result.ts:22-69 |
| Agent (LLM/tool-call) error event | Type guard `isAgentErrorEvent`: `source === "agent"` with `tool_name`, `tool_call_id`, `error` string | src/types/agent-server/type-guards.ts:80-89 |
| Conversation/server error events | `ServerErrorEvent { kind, code ("e.g. MCPError"), detail }`; `ConversationErrorEvent` re-exported from generated client | src/types/agent-server/core/events/conversation-state-event.ts:154-174, 3-5 |
| Structured classification | `ErrorClassification` fields `kind`, `retryable`, `user_action`, `error_id` exercised in tests of the tracking path | __tests__/utils/error-handler.test.ts:64-69, 102-108 |
| ACP error-code taxonomy | `ACPAuthRequired`, `ACPSpawnError`, `ACPInitError`, `ACPPromptError`, `UsagePolicyRefusal` mapped to localized banner headers; auth code triggers re-auth action | src/utils/acp-error-codes.ts:8-29 |
| MCP failure kinds | `ExtendedMCPTestFailureKind = MCPTestFailureKind \| "credentials"`; kind-specific localized guidance (timeout/connection/credentials/unknown) | src/types/mcp-server.ts:34, src/utils/mcp-test-error-message.ts:10-25 |
| MCP health state machine | `unchecked → checking → healthy(verified\|connectivity-only) → failed{kind, redacted error}`; conservative client-side auth sniffing upgrades `connection`+401/403 text to `credentials` | src/types/mcp-health.ts:10-27, src/api/mcp-health/probe-mcp-server-health.ts:22-58 |
| Model-facing client-tool envelope | `LaunchFailure { status: "error", error, guidance }`; validation failures return corrective guidance ("call launch_child_conversation again with a valid target") | src/services/child-conversation-launch.ts:68-89, 110-160 |
| Result hand-back to model | `reportLaunchResult()` posts `CHILD_CONVERSATION_RESULT_PREFIX + JSON(result)` as a user message so the agent relays/follows guidance | src/services/child-conversation-launch.ts:459-497 |
| Guidance instructs retry policy | Cloud sandbox failure: "you can retry, or fall back to target=\"local\""; transport failure: "retry only if the cause looks transient" | src/services/child-conversation-launch.ts:426-429, 522-527 |
| User-facing inline rendering of failed tools | Failed observations render `**Error:**\n…` bodies for editor/browser/MCP/skill/switch-LLM/finish/glob/grep; failed SwitchLLM stays visible while successful ones are hidden via `ModelMessages` | src/components/conversation-events/chat/event-content-helpers/get-observation-content.ts:30-31, 112-114, 140-144, 167-169, 197-201, 283-284, 307-308, 342-343; should-render-event.ts:90-102 |
| Inline agent-error card | `ErrorEventMessage` renders `AgentErrorEvent` via `ErrorMessage`; i18n key lookup on `event.id` with fallback `CHAT_INTERFACE$AGENT_ERROR_MESSAGE`, details collapsed behind expand toggle | src/components/conversation-events/chat/event-message-components/error-event-message.tsx:10-16, src/components/features/chat/error-message.tsx:13-41 |
| Banner above composer | `ErrorMessageBanner` shows header by error code, truncated content (>220 chars), copy button, dismiss, conditional Retry and Re-auth buttons; warning vs error icon chosen by `classification.kind` | src/components/features/chat/chat-interface.tsx:583-600, src/components/features/chat/error-message-banner.tsx:22, 103-120, 141-150, 172-207 |
| Error store semantics | `"connection"` errors auto-clear when socket reopens; `"conversation"` errors are sticky until explicit dismiss/retry/new message; carries `errorCode` + `classification` | src/stores/error-message-store.ts:4-9, 47-65 |
| Toast channel & dedupe | Global QueryCache/MutationCache onError toasts via `retrieveAxiosErrorMessage`, 3-second duplicate suppression set; opt-out via query `meta.disableToast` | src/query-client-config.ts:41-76, src/utils/retrieve-axios-error-message.ts:21-49 |
| Connection-error normalization | Cause-chain walker (depth ≤ 4, cycle-safe) classifies CORS/network vs timeout vs raw message into fixed actionable copy | src/utils/user-facing-error.ts:1-92 |
| Operator/analytics alerts | `trackError` → PostHog `error_outcome` with `error_source`, `error_kind`, correlatable `error_id`, no raw messages; reserved keys not spoofable; `internal`/`unknown` flagged as `diagnostic` telemetry | src/utils/error-handler.ts:10-46, src/contexts/conversation-websocket-context.tsx:570-608, 803-841 |
| Human confirmation loop | `ExecutionStatus.WAITING_FOR_CONFIRMATION`; `AgentState.AWAITING_USER_CONFIRMATION`; accept/reject buttons + ⌘↩ / ⇧⌘⌫ shortcuts; HIGH-risk alert; duplicate-submission guard; POSTs `/events/respond_to_confirmation` | src/types/agent-server/core/base/common.ts:67-75, src/types/agent-state.tsx:12, src/components/shared/buttons/conversation-confirmation-buttons.tsx:38-136, src/api/event-service/event-service.api.ts:40-69 |
| Rejection feedback | `UserRejectObservation { rejection_reason, action_id }` — a rejected action becomes an observation the agent can see | src/types/agent-server/core/events/observation-event.ts:42-52 |
| Runaway-loop guard | Typed WS error starting "Agent reached maximum…" flips agent state to PAUSED so resume auto-bumps max iterations | src/hooks/use-handle-ws-events.ts:55-61 |
| WS retry exhaustion behavior | Exponential backoff 1 s → capped 30 s with ≤30 % jitter; `maxAttempts` option (default Infinity); attempt counter resets on success; handshake watchdog aborts sockets stuck in CONNECTING | src/hooks/use-websocket.ts:18-20, 110-140, 61-64 |
| Reconnect replay safety | Post-reconnect replayed events are deduped by id before non-idempotent side effects (#1656) | src/contexts/conversation-websocket-context.tsx:556-568 |
| Backend probe circuit breaker | Quick in-query retry (2 attempts, 300 ms) skipping definitive auth errors; after `MAX_CONSECUTIVE_FAILURES` the backend is marked `disabled` in localStorage and polling stops; Manage Backends can one-shot re-probe | src/hooks/query/use-backends-health.ts:136-186, src/api/backend-registry/health-store.ts:40-63, use-backends-health.ts:203-210 |
| Generic API retry helper | `withRetry(fn, maxRetries=3, baseDelayMs=500)` exponential backoff; exhaustion surfaces as generic `throw new Error("Retry attempts exhausted")` | src/api/with-retry.ts:4-26 |
| Isolation fallback retry | Worktree creation failure retried once as shared isolation with an explanatory note rather than losing the launch | src/services/child-conversation-launch.ts:308-323 |

## Answers to Dimension Questions

1. **Who sees tool failure?** All four audiences, deliberately separated. The **model** sees failed client-tool results as JSON envelopes with corrective `guidance` posted back through the conversation message endpoint (`src/services/child-conversation-launch.ts:488-496`), and server-side observations carry error payloads to the LLM (frontend renders them but the feed originates in the SDK). The **user** sees three tiers: inline error cards in the chat stream (`src/components/conversation-events/chat/event-message-components/error-event-message.tsx:15`), a banner above the composer for conversation/server errors (`src/components/features/chat/chat-interface.tsx:583-600`), and toasts for WS server errors and REST query/mutation failures (`src/hooks/use-handle-ws-events.ts:41-53`, `src/query-client-config.ts:51-76`). The **operator** gets consent-gated PostHog `error_outcome` events with kind/source/error-id dimensions (`src/utils/error-handler.ts:37-46`) and a persisted per-backend health state rendered as a connectivity dot (`src/api/backend-registry/health-store.ts:55-62`). No evidence found of server-side paging/alerting hooks inside this repo (searched `alert`, `notify`, `datadog` patterns; AGENTS.md mentions Datadog facets only for ingress headers).

2. **Is the error actionable?** Mostly yes. Connection errors get fixed, plain-language copy ("Disconnected (check URL or network). Check that the backend URL is correct…" — `src/utils/user-facing-error.ts:1-5`) plus a Retry button that reconnects the socket (`chat-interface.tsx:589-593`). ACP credential failures get a code-specific header and an "Update credentials" button navigating to settings (`src/utils/acp-error-codes.ts:26-29`, `error-message-banner.tsx:141-150`). MCP test failures give kind-specific localized guidance (`mcp-test-error-message.ts:15-24`). Model-facing envelopes always pair `error` with `guidance` naming the next action (`child-conversation-launch.ts:119-123`). Weak spot: `classification.retryable` is recorded but never read to enable the banner Retry button — only the literal connection-error string does.

3. **Can the model recover?** Yes for client-executed tools: the launcher "never rejects" and converts every failure into guidance the agent can act on, including target fallback (`local` ↔ `cloud`) and parameter correction (`src/services/child-conversation-launch.ts:499-528`). The user-rejection path also feeds back: rejected confirmations produce a `UserRejectObservation` with a reason (`src/types/agent-server/core/events/observation-event.ts:42-52`). For backend-executed tools, the recovery loop belongs to the SDK (out of boundary); the frontend preserves the full error text in chat history so the model's next turn and the user both see the same evidence.

4. **When is failure escalated to a human?** Three triggers: (a) explicit confirmation mode pauses execution at `WAITING_FOR_CONFIRMATION` and requires accept/reject, with a high-risk alert for `SecurityRisk.HIGH` actions (`conversation-confirmation-buttons.tsx:92-118`); (b) max-iteration exhaustion auto-pauses the agent and waits for the user to resume (`use-handle-ws-events.ts:57-60`); (c) sticky conversation errors persist until a human dismisses/retries (`error-message-store.ts:4-9`). Additionally, repeated backend probe failures escalate to "disabled" state surfaced through the Manage Backends modal (`health-store.ts:40-63`).

5. **Are failures grouped by cause?** Yes, along several axes: `ErrorClassification.kind` groups errors and switches telemetry between `outcome` and `diagnostic` classes (`error-handler.ts:44-45`); ACP codes group spawn/init/prompt/usage-policy failures under one generic header while keeping auth distinct (`acp-error-codes.ts:12-17`); MCP failures are grouped by `timeout/connection/credentials/unknown` (`mcp-test-error-message.ts:15-24`); observation results normalize into success/error/timeout buckets including the soft-timeout sentinel (`get-observation-result.ts:33`). There is no cross-source aggregation or clustering beyond the `error_kind` dimension in analytics.

## Architectural Decisions

- **Errors are events, not exceptions.** Every failure mode the agent produces arrives over the same WebSocket event stream as normal work (`isDisplayableErrorEvent` routing in `src/contexts/conversation-websocket-context.tsx:570-594`), which means failures replay, dedupe, and persist exactly like successes — escalation survives reloads because it is part of history.
- **Dual-channel display policy.** `AgentErrorEvent`s render *inline* in the transcript while `ConversationErrorEvent`/`ServerErrorEvent`s render *banners*; a deliberate early return suppresses double-notification by toast (`use-handle-ws-events.ts:35-39`). This keeps per-tool noise attached to the failing action card and conversation-wide failures pinned to the composer.
- **Client tools fail closed-but-guiding.** Because the agent-server pre-acknowledges client tool calls, the browser treats any outcome other than success as data for the model, never a thrown error (`handleLaunchChildConversationAction` docstring, `child-conversation-launch.ts:499-504`). The envelope shape `{status:"error", error, guidance}` is the repo's canonical model-facing error contract.
- **Sticky vs transient error memory.** The error store distinguishes auto-healing `"connection"` errors from sticky `"conversation"` errors requiring human action (`error-message-store.ts:4-9`), preventing both flapping banners and permanently stuck ones.
- **Circuit breaker over infinite polling.** Rather than hammering dead backends forever, consecutive failures trip a persisted `disabled` flag; recovery is explicit (config edit or modal re-probe) rather than time-based (`health-store.ts:40-85`).
- **Privacy-preserving telemetry.** Raw error messages never reach analytics — only classification kind and correlatable IDs do; reserved outcome fields cannot be overridden by caller metadata (`error-handler.ts:22-45`).

## Notable Patterns

- **Normalized verdict function**: one pure function maps 16 observation kinds onto a three-value status consumed by success indicators, grouping logic, and transcript export (`get-observation-result.ts:22-69`, mirrored for ACP at :14-20).
- **Redaction-before-display**: MCP health errors pass through `redactMcpSecrets` before storage (`probe-mcp-server-health.ts:29-35`), and the health type documents the field as "Redacted, display-safe error detail" (`src/types/mcp-health.ts:22`).
- **Jittered reconnect**: parallel main/planning sockets add up-to-30 % random jitter to avoid lockstep retries against a struggling server (`use-websocket.ts:125-132`).
- **Retry placement rationale**: backend probes retry *inside* the query function so success/failure recording runs exactly once per logical probe, avoiding premature circuit-breaker trips (`use-backends-health.ts:154-162`).
- **Cause-chain walking**: connection-error detection traverses `error.cause` up to depth 4 with cycle protection before classifying (`user-facing-error.ts:13-41`).
- **Keyboard-first confirmation UX**: ⌘↩ accept and ⇧⌘⌫ reject shortcuts mirror desktop idioms, with submitted-event guards against double submission (`conversation-confirmation-buttons.tsx:60-97`).

## Tradeoffs

- **No automatic tool retry**: the frontend never re-issues a failed backend tool call; recovery is delegated to the model (via observations) or the human (via resume/retry buttons). This keeps side-effect control server-side but means transient tool blips cost a full model turn.
- **String-matching heuristics**: connection/timeout/auth classification relies on substring matching over error text (`user-facing-error.ts:53-79`, `probe-mcp-server-health.ts:22-23`). Robust across locales of underlying libraries but fragile if upstream wording changes.
- **Default-infinity WS retries**: `reconnect.maxAttempts ?? Infinity` (`use-websocket.ts:113-114`) combined with the 30 s cap means a permanently down backend yields endless low-rate retries; the circuit breaker covers REST probes but not the socket path.
- **Inline-vs-banner split depends on event typing**: mis-typed or unknown error events fall through to generic handling (`getDefaultEventContent`, default `"success"` verdict at `get-observation-result.ts:67-68`) — an unrecognized observation kind silently reads as success.
- **Toast dedupe window is time-based (3 s)**: identical errors more than 3 seconds apart toast again (`query-client-config.ts:54-61`), which can spam during systematic failures.

## Failure Modes / Edge Cases

- **Soft-timeout ambiguity**: bash commands that hit the soft timeout report `exit_code: -1` and map to `"timeout"` status, distinct from real failure (`observation.ts:67`, `get-observation-result.ts:33`) — the UI must handle the follow-up completion observation replacing it.
- **Replay duplication**: post-reconnect event replay would double-fire non-idempotent side effects (toasts, telemetry); guarded by event-id dedup (`conversation-websocket-context.tsx:556-568`).
- **Malformed oldest event during pagination**: the store throws, flips `hasMore` off, and the failure surfaces via the shared banner instead of failing silently (documented in `AGENTS.md`, implemented via `useLoadOlderEvents`).
- **Cloud pagination fallback**: unsupported timestamp filters trigger a warn-and-stop (empty page) so the UI does not retry indefinitely against a known-bad backend (`event-service.api.ts:149-163`).
- **Duplicate child launches**: `claimToolCall` idempotency ledger prevents a replayed action from launching twice (`child-conversation-launch.ts:510`).
- **Unparseable WS frames**: JSON parse failures degrade to `console.warn` without user notification (`conversation-websocket-context.tsx:742-744`) — silent from the user's perspective.

## Future Considerations

- Consume `classification.retryable` (and `user_action`) to drive banner actions generically instead of string-comparing against `SERVER_CONNECTION_ERROR_MESSAGE` (`chat-interface.tsx:589-593`), making classified errors self-describing in the UI.
- Replace substring-based network/timeout/auth sniffers with structured codes end-to-end as the SDK expands its classification coverage (`user-facing-error.ts:47-79`).
- Give `withRetry` a structured exhaustion record (attempt count, last error) instead of a bare `"Retry attempts exhausted"` string (`with-retry.ts:25`) so callers can log/distinguish exhaustion causes.
- Bound WS reconnect attempts (or reuse the backend circuit breaker) so a dead host stops socket churn entirely.
- Consider surfacing `error_id` correlation in the copy-to-clipboard payload of the error banner so user-reported bugs join to telemetry without raw message capture.

## Questions / Gaps

- **Where does the model actually see `is_error` observations?** The conversion of observations into the next LLM prompt lives in `software-agent-sdk`, outside this source. No evidence found in-repo; searched `src/` for prompt assembly and found none (frontend only forwards events).
- **Operator alerting beyond PostHog**: no PagerDuty/Datadog/log-sink hooks exist in this repo. Whether the agent-server or cloud layers alert on `error_outcome`-adjacent signals cannot be answered from this source.
- **`ErrorEventMessage` translation keying**: passing `event.id` as an i18n key (`error-event-message.tsx:15`) will almost always miss `i18n.exists()` and fall back to the generic message; whether any SDK populates meaningful ids here is unverifiable from this repo.
- **Confirmation-mode enforcement**: `confirmation_mode` is a settings flag (`src/types/settings.ts:128`, defaults in `src/services/settings.ts:13`), but the analyzer that decides *which* actions require confirmation is server-side; the risk-level taxonomy (`SecurityRisk`, `common.ts:59-64`) reaches the UI only as a label on the awaiting action.

---

Generated by `07.08-tool-failure-escalation` against `openhands`.

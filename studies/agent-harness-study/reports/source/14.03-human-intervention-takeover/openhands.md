# Source Analysis: openhands

## Human Intervention and Takeover (Dimension 14.03)

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (`@openhands/agent-canvas`) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19, React Router 7, Zustand, TanStack Query, `@openhands/typescript-client`, xterm.js, Monaco |
| Analyzed | 2026-08-26 |

## Summary

This source is the OpenHands **agent-canvas frontend** — the React UI that drives an external `openhands-agent-server` (Python, separate `software-agent-sdk` repo). Human intervention is therefore implemented as a client-side control surface over the server's conversation/event APIs, and it is unusually rich for a frontend:

- **Mid-run feedback is first-class.** The composer stays enabled while the agent is `RUNNING` (`src/components/features/chat/interactive-chat-box.tsx:62-65`); messages go out over the live WebSocket with `run: true` or are queued via REST when the socket is down (`src/contexts/conversation-websocket-context.tsx:1094-1144`). An optimistic pending-message queue with a timeout watchdog and server-echo matching keeps delivery honest (`src/stores/optimistic-user-message-store.ts:14,115-141`). A `/btw <question>` side-channel asks the agent a question through a dedicated `askAgent` endpoint without appending to the main history (`src/hooks/chat/use-btw-interceptor.ts:21-40`).
- **Pause/interrupt/resume are explicit, backend-aware operations.** Local mode interrupts immediately (cancelling in-flight LLM calls); cloud mode pauses the whole sandbox (`src/hooks/mutation/conversation-mutation-utils.ts:36-61`, `src/api/cloud/conversation-service.api.ts:226-255`). Resume re-runs the conversation (`conversation-mutation-utils.ts:117-123`) or auto-unpauses a PAUSED cloud sandbox on navigation (`src/routes/conversation.tsx:158-178`).
- **Human approval gates** exist as a first-class execution status (`WAITING_FOR_CONFIRMATION`, `src/api/conversation-service/agent-server-conversation-service.api.ts:306-314`) with accept/reject UI, high-risk warnings, and keyboard shortcuts (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:16-136`).
- **"Edit state" is modeled as fork-and-continue, not in-place mutation.** Every user/assistant message carries a "Branch from here" action; for user messages this forks at the message's parent (excluding it) and restores its text into the new conversation's composer (`src/components/conversation-events/chat/event-message-components/user-assistant-event-message.tsx:64-113`, `src/hooks/mutation/use-fork-conversation.ts:22-71`). Forking is local-backend-only.
- **Sandbox takeover is deliberately restricted**: the terminal tab is a read-only mirror of agent bash activity (`disableStdin: true`, `src/hooks/use-terminal.ts:107`); humans take over editing by opening VSCode in the browser via a per-conversation URL (`src/hooks/query/use-unified-vscode-url.ts:24-211`). The diff viewer's Monaco instance is also read-only (`src/components/features/diff-viewer/file-diff-viewer.tsx:36`).

The answer to the dimension's guiding question — "Can a human correct the agent mid-flight without restarting?" — is **yes**: send a message while running, interrupt locally without losing the sandbox, respond to confirmation prompts, switch LLM profile/model in place, compact context, branch from any past message, or stop/resume goal loops.

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale: pause/interrupt, resume, mid-run messaging, confirmation gates, and forking each have concrete API paths, UI entry points, and unit tests (`__tests__/components/features/chat/user-assistant-event-message.test.tsx:83-269`, `__tests__/hooks/mutation/pause-conversation-local.test.ts:68-91`, `__tests__/api/cloud/conversation-pause.test.ts:66-90`, `__tests__/hooks/mutation/use-resume-conversation.test.tsx:38-82`). Operational safeguards include optimistic-send watchdogs, echo-based reconciliation, duplicate-submission guards, version-compatibility fallbacks for older servers, and deliberate WS suppression while a cloud sandbox resumes. It falls short of 8+ because: forking is unsupported on cloud backends, terminal takeover is absent by design, confirmation rejection cannot carry a reason from the UI despite wire support, and intervention lineage (forks) is tracked only through a "(branch)" title convention rather than a durable parent link.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Pause (local = interrupt) | `pauseConversation` branches: cloud → `pauseCloudSandbox`; local → `ConversationClient.interruptConversation` "so in-flight LLM requests are cancelled immediately" | `src/hooks/mutation/conversation-mutation-utils.ts:41-61` |
| Cloud sandbox pause/resume endpoints | `POST /api/v1/sandboxes/{id}/pause` and `/resume` via cloud proxy | `src/api/cloud/conversation-service.api.ts:226-255` |
| Stop UX + cache patch | Toast-driven unified pause; patches `execution_status: PAUSED` + `sandbox_status: "PAUSED"` atomically | `src/hooks/mutation/use-unified-stop-conversation.ts:21-71` |
| Resume run | `resumeConversation` → `ConversationClient.runConversation` | `src/hooks/mutation/conversation-mutation-utils.ts:117-123` |
| Stop/Resume buttons driven by state | `shouldShownAgentStop` when RUNNING; `shouldShownAgentResume` when STOPPED/PAUSED | `src/components/features/controls/agent-status.tsx:78-86,149-157` |
| Wire-up of handlers | `handlePauseAgent` / `handleResumeAgentClick` call the mutations | `src/components/features/chat/components/chat-input-actions.tsx:162-170,493-500` |
| Execution-status mapping | IDLE→AWAITING_USER_INPUT, RUNNING→RUNNING, PAUSED→PAUSED, WAITING_FOR_CONFIRMATION→AWAITING_USER_CONFIRMATION | `src/hooks/use-agent-state.ts:10-35` |
| Runtime status set includes paused/waiting | `RUNTIME_STATUSES` = {idle, running, paused, waiting_for_confirmation, finished, error, stuck} | `src/api/conversation-service/agent-server-conversation-service.api.ts:306-314` |
| Mid-run input not disabled | `isDisabled` only for AWAITING_USER_CONFIRMATION / task polling / provisioning / no-LLM — NOT while RUNNING | `src/components/features/chat/interactive-chat-box.tsx:62-65` |
| Send while running | WS path sends `{...message, run: true}`; REST fallback queues via `sendEvent(..., { run: true })` when socket closed | `src/contexts/conversation-websocket-context.tsx:1094-1144` |
| Optimistic send queue | `PENDING_MESSAGE_TIMEOUT_MS = 150_000` watchdog flips stuck "sending" bubbles to retryable errors | `src/stores/optimistic-user-message-store.ts:14,131-141` |
| Echo reconciliation | `consumeMatchingPendingMessage(conversationId, echoedText)` on UserMessageEvent; FIFO fallback for munged bodies | `src/contexts/conversation-websocket-context.tsx:610-624`; `src/stores/optimistic-user-message-store.ts:169-198` |
| Side-channel Q&A | `/btw` interceptor routes to `askAgent` (ask-side-question endpoint), bypassing the main stream | `src/hooks/chat/use-btw-interceptor.ts:21-40`; `src/hooks/mutation/conversation-mutation-utils.ts:63-75` |
| Confirmation gate UI | Reject/confirm rendered only in AWAITING_USER_CONFIRMATION; high-risk `RiskAlert`; ⇧⌘⌫ reject, ⌘↩ confirm shortcuts | `src/components/shared/buttons/conversation-confirmation-buttons.tsx:60-135` |
| Confirmation endpoint | `respondToConfirmation` → runtime `POST /api/conversations/{id}/events/respond_to_confirmation` | `src/api/event-service/event-service.api.ts:40-69` |
| Confirmation payload type | `{ accept: boolean; reason?: string }` — `reason` exists on the wire but the hook sends only `{ accept }` | `src/api/event-service/event-service.types.ts:1-4`; `src/hooks/mutation/use-respond-to-confirmation.ts:21-23` |
| Duplicate-submission guard | `submittedEventIds` store prevents double responses to one awaiting action | `src/stores/event-message-store.ts:14-26`; `conversation-confirmation-buttons.tsx:44-47,92-100` |
| Fork ("Branch from here") | Action on hover of any user/assistant message; local + inside-conversation only | `src/components/conversation-events/chat/event-message-components/user-assistant-event-message.tsx:64-113` |
| Fork semantics | Edit-mode resolves `getEventParentId` and branches *before* the message, excluding it; otherwise inclusive | `src/hooks/mutation/use-fork-conversation.ts:22-71` |
| Fork API | `forkConversation(sourceId, fromEventId, title)` casts `from_event_id` into the request; requires agent-server ≥ 1.31.0 | `src/api/conversation-service/agent-server-conversation-service.api.ts:792-827` |
| Older-server fallback | If `leaf_event_id !== fromEventId`, the backend ignored `from_event_id` (copied everything) → skip composer prefill | `src/hooks/mutation/use-fork-conversation.ts:59-68` |
| Cloud fork prohibition | Throws "Branching a conversation isn't supported on the cloud backend yet." | `src/api/conversation-service/agent-server-conversation-service.api.ts:802-806` |
| Read-only terminal mirror | xterm created with `disableStdin: true` ("Make terminal read-only"); fed by ExecuteBash action/observation events | `src/hooks/use-terminal.ts:100-115`; `src/contexts/conversation-websocket-context.tsx:657-670` |
| Editor takeover (VSCode) | Per-conversation VSCode URL, capability-probed via `/api/vscode/status`; cloud reads exposed_urls | `src/hooks/query/use-unified-vscode-url.ts:40-115,128-194`; `src/api/conversation-service/agent-server-conversation-service.api.ts:541-588` |
| Read-only diffs | Diff-viewer Monaco options set `readOnly: true` — no human file edits from the UI | `src/components/features/diff-viewer/file-diff-viewer.tsx:36` |
| Internal bash socket (not user-facing) | `useBashCommandRunner` speaks `/sockets/bash-events` but is consumed only by local git-info detection | `src/hooks/use-bash-command-runner.ts:49-72`; `src/hooks/query/use-local-git-info.ts:122` |
| In-place model/profile swap | `switchProfile` / `switchAcpModel` swap LLM on the live session; SwitchLLM observation recorded as inline "Switched to" message | `src/api/conversation-service/agent-server-conversation-service.api.ts:861-944`; `src/contexts/conversation-websocket-context.tsx:688-722` |
| Human-triggered compaction | Compact action POSTs `/api/conversations/{id}/condense`; disabled while agent busy | `src/hooks/use-compact-context-action.ts:35-39,80-111`; `agent-server-conversation-service.api.ts:717-745` |
| Goal-loop control | startGoal/stopGoal/resumeGoal; stop also interrupts because backend leaves the turn running; resume continues an interrupted loop | `src/hooks/mutation/conversation-mutation-utils.ts:77-115`; `src/components/features/chat/goal-status-content.tsx:29-95` |
| Client-driven state change (legacy) | Max-iterations error triggers `CHANGE_AGENT_STATE → PAUSED` over the socket so resume bumps iterations | `src/hooks/use-handle-ws-events.ts:55-61`; `src/services/agent-state-service.ts:4-7`; `src/types/action-type.tsx:43` |
| Cloud auto-resume gate | On entering a PAUSED cloud conversation, `resumeCloudSandbox` fires once per mount; WS URL suppressed until RUNNING | `src/routes/conversation.tsx:143-178`; `src/contexts/websocket-provider-wrapper.tsx:24-33` |
| Trajectory export | Download conversation trajectory zip (audit artifact) | `src/hooks/use-download-conversation.ts:13-27`; `agent-server-conversation-service.api.ts:673-681` |
| Tests — fork/edit | 10 cases: inclusive assistant branch, edit-mode prefill, root fallback, older-backend leaf check, image-only, double-click guard, cloud hidden, off-conversation hidden | `__tests__/components/features/chat/user-assistant-event-message.test.tsx:83-269` |
| Tests — pause | Local interrupt asserted; missing-sandbox cloud case asserted | `__tests__/hooks/mutation/pause-conversation-local.test.ts:68-91`; `__tests__/api/cloud/conversation-pause.test.ts:66-90` |
| Tests — resume | Cache invalidation after `runConversation` settle | `__tests__/hooks/mutation/use-resume-conversation.test.tsx:38-82` |

## Answers to Dimension Questions

1. **Can humans edit agent state?**
   Partially — through control transitions rather than raw mutation. Humans can pause/interrupt (`conversation-mutation-utils.ts:41-61`), resume (`:117-123`), condense history (`use-compact-context-action.ts:80-111`), swap the live model/profile (`agent-server-conversation-service.api.ts:861-944`), rename conversations (`:777-790`), and drive goal loops (`conversation-mutation-utils.ts:83-115`). There is **no API in this repo to edit or delete individual events** in a conversation's history; correction is achieved by branching (see #4). A legacy client-initiated `CHANGE_AGENT_STATE` action still exists for the max-iterations auto-pause path (`use-handle-ws-events.ts:55-61`).

2. **Can humans provide mid-run feedback?**
   Yes, explicitly. The composer remains active during `RUNNING` (`interactive-chat-box.tsx:62-65`); messages are delivered over the same conversation WebSocket with `run: true` (`conversation-websocket-context.tsx:1131-1135`), or queued server-side over REST if the socket dropped (`:1100-1128`). Delivery integrity is protected by the optimistic queue + echo matching + timeout watchdog (`optimistic-user-message-store.ts:131-141,169-198`). The `/btw` command provides an out-of-band question channel (`use-btw-interceptor.ts:21-40`).

3. **Can humans take over execution?**
   Only indirectly. The in-app terminal is intentionally read-only (`use-terminal.ts:107`), diffs are read-only (`file-diff-viewer.tsx:36`), and the direct bash-events socket is reserved for internal git probing (`use-bash-command-runner.ts:49-72`). Takeover means: (a) interrupt/pause the run (`conversation-mutation-utils.ts:41-61`), (b) approve/reject gated actions (`conversation-confirmation-buttons.tsx:38-58`), or (c) open the workspace in browser VSCode for hands-on editing (`use-unified-vscode-url.ts:85-115`). There is no "human types into the agent's shell" path.

4. **Are human interventions traceable?**
   Mostly, via the event stream itself: every user message becomes a persisted `UserMessageEvent` echoed over the socket (`conversation-websocket-context.tsx:615-624`); model switches render inline "Switched to" entries reconstructed even from REST history (`:688-722`, `seedModelSwitchesFromHistory`); confirmation responses hit a dedicated endpoint whose request schema supports an optional `reason` (`event-service.types.ts:1-4`) — though the UI never sends one (`use-respond-to-confirmation.ts:21-23`). The weak point is **fork lineage**: forks are distinguished only by a `"<title> (branch)"` suffix (`user-assistant-event-message.tsx:69-75`) plus copied client-side metadata (`agent-server-conversation-service.api.ts:819-824`); no `parent_conversation_id` is recorded for forks (that field exists solely for agent-delegated sub-conversations, `child-conversation-launch.ts:236`). Pause/fork actions leave no dedicated audit records beyond resulting state changes; trajectory export (`use-download-conversation.ts:13-27`) is the offline audit artifact.

## Architectural Decisions

- **Interventions are backend commands, not local state writes.** All takeover verbs (interrupt, run, fork, condense, respond_to_confirmation, switch_llm) map to typed `ConversationClient` methods routed through `src/api/*` services, honoring the repo's API-access rules (AGENTS.md "API Access Rules"; enforced by `src/api/no-direct-agent-server-calls.test.ts`). The frontend owns UX integrity (optimism, dedupe, retries) but never invents state.
- **Edit-as-fork instead of edit-in-place.** Correcting a past message creates a derived conversation at `parent(event)` with the text restored into the composer (`use-fork-conversation.ts:22-51`). This preserves append-only event-stream semantics on the server at the cost of conversation proliferation and weak lineage.
- **Backend-kind branching is pervasive.** Every intervention has distinct local vs cloud behavior — local favors immediacy (interrupt cancels in-flight LLM calls), cloud favors lifecycle (pause/resume entire sandboxes) — e.g. `conversation-mutation-utils.ts:45-61` and `websocket-provider-wrapper.tsx:24-33`.
- **Version-tolerant degradation.** Forks detect pre-1.31.0 servers that ignore `from_event_id` by comparing returned `leaf_event_id` and suppress composer prefill to avoid duplicated prompts (`use-fork-conversation.ts:59-68`).
- **Confirmation as an execution status, not a modal.** `WAITING_FOR_CONFIRMATION` is a peer of RUNNING/PAUSED (`agent-server-conversation-service.api.ts:306-314`), so gating composes with pause/resume machinery instead of being a separate overlay system.

## Notable Patterns

- **Optimistic UI with server-echo reconciliation**: exact content match first, FIFO fallback within the same conversation, cross-conversation scoping to prevent stale acks popping wrong bubbles (`optimistic-user-message-store.ts:169-198`).
- **Watchdog timers for liveness**: a 150 s budget converts a lost send into a visible retry affordance rather than an eternal spinner (`optimistic-user-message-store.ts:131-141`).
- **Capability probing before takeover surfaces**: VSCode availability is probed via `/api/vscode/status` so a deliberately editor-less deployment renders nothing instead of a dead button (`use-unified-vscode-url.ts:49-83`).
- **Cache-first status propagation**: pause patches both `execution_status` and `sandbox_status` in query caches immediately so reconnect logic doesn't race a stale poll (`use-unified-stop-conversation.ts:52-70`).
- **Double-submit guards at two layers**: ref-based same-tick click guard for forks (`user-assistant-event-message.tsx:42,66-67`) and store-level submitted-id guard for confirmations (`event-message-store.ts:16-25`).

## Tradeoffs

- **Fork-based editing multiplies conversations** and scatters lineage; there is no tree view linking branches to sources, and the `(branch)` marker can be renamed away.
- **Read-only terminal reduces operator power**: diagnosing a wedged sandbox requires leaving the app for VSCode or waiting for the agent; the capable bash-events socket is withheld from users (`use-bash-command-runner.ts` used only by `use-local-git-info.ts:122`).
- **Local-first feature skew**: forking and immediate interrupt work only on local backends; cloud users get coarser tools (sandbox pause) and no branching (`agent-server-conversation-service.api.ts:802-806`).
- **Interrupt semantics differ by backend** (cancel-now vs wait-for-call-then-pause), which is documented in code (`conversation-mutation-utils.ts:36-40`) but invisible to users beyond a toast.

## Failure Modes / Edge Cases

- **Echo mismatch after server normalization**: if the server trims/munges a message body, the FIFO fallback pops the oldest sending bubble in that conversation — correct for single-message flows but potentially mislabeled under rapid concurrent sends (`optimistic-user-message-store.ts:174-188`).
- **Old backend silently copying full history on fork** is detected and worked around, but the fork still exists with the wrong shape; only the prefill is suppressed (`use-fork-conversation.ts:59-68`).
- **Cloud resume races**: navigating into a PAUSED cloud conversation must not touch the stale `conversation_url`; the wrapper nulls it and fast-polls until RUNNING, else the WS hammers a dead host (`websocket-provider-wrapper.tsx:24-33`, `routes/conversation.tsx:143-157`).
- **Duplicate confirmation submission**: guarded by `submittedEventIds`, but the store is session-memory only — a reload before the response lands could allow a second response to the same awaiting action (`event-message-store.ts:14-26`).
- **Max-iterations auto-pause depends on parsing English error text** (`message.startsWith("Agent reached maximum")`), which is brittle against copy changes (`use-handle-ws-events.ts:57-59`).

## Future Considerations

- Record durable fork parentage (surface a `parent_conversation_id`-like field for forks) and render a branch tree, turning the title suffix convention into real lineage.
- Pass the already-supported `reason` field on confirmation rejection to enrich audit trails (`event-service.types.ts:2`; `use-respond-to-confirmation.ts:21-23`).
- Expose a guarded interactive console (reusing the existing bash-events socket with policy checks) for sandbox takeover without leaving the app.
- Unify local-interrupt and cloud-pause semantics behind a single documented contract so downstream automation gets consistent timing guarantees.
- Replace the string-match max-iterations detection with a structured event from the server.

## Questions / Gaps

- No evidence in this repository of server-side enforcement details for `respond_to_confirmation` or `fork` — those live in the sibling `software-agent-sdk` repo (out of scope per source-isolation rules); this analysis covers the client contracts only.
- No evidence found of any UI to edit/delete historical events in place; searched `src/api/event-service/`, `src/hooks/mutation/`, and components for edit/delete-event paths — none exist.
- Whether confirmation responses or pauses are persisted as first-class events (rather than status transitions) could not be verified from the frontend alone; only their request/response shapes are visible here (`event-service.types.ts:1-8`).

---

Generated by `Dimension 14.03: Human Intervention and Takeover` against `openhands (@openhands/agent-canvas)`.

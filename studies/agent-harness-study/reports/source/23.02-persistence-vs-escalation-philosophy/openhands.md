# Source Analysis: openhands

## Dimension 23.02: Persistence vs Escalation Philosophy

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React (Vite, React Router, TanStack Query, Zustand) — the "Agent Canvas" frontend of the OpenHands multi-repo system |
| Analyzed | 2026-08-24 |

## Summary

This source is **not** the agent loop itself. `AGENTS.md` (repo map table) states explicitly that this repo owns only the React/TypeScript frontend and that agent/tool/conversation behavior lives in the sibling `OpenHands/software-agent-sdk`. Consequently, this analysis studies how the frontend **shapes, observes, and escalates** persistence rather than executing it.

The philosophy that emerges is: **the client persists aggressively at the transport layer but never silently on behalf of the user; every bounded-retry give-up escalates to an explicit human decision surface.** Concretely:

1. **Bounded retry everywhere, with deliberate non-retry paths.** A shared exponential-backoff helper (`src/api/with-retry.ts:4-25`) backs settings/secrets calls; WebSocket reconnects use exponential backoff capped at 30 s with 30% jitter (`src/hooks/use-websocket.ts:18-20`, `src/hooks/use-websocket.ts:125-136`); React Query hooks tune `retry` per query — `false` for decided failures (401/404), capped at 1 where a slow load would block the socket gate (`src/hooks/query/use-conversation-history.ts:86-96`), conditional for auth-dependent queries (`src/hooks/query/use-settings.ts:145`). Backend health probes retry twice inside the query function specifically so a transient cold-start miss does not flip the indicator red, while definitive auth failures are never retried (`src/hooks/query/use-backends-health.ts:140-186`).
2. **Persistence of the *agent* is a server-side concern configured by the client.** The client caps the loop via `max_iterations` (default fallback 500) sent at conversation start (`src/api/agent-server-adapter.ts:1122-1125`), enables server-side `stuck_detection` (`src/api/agent-server-adapter.ts:1126`), and maps the resulting `STUCK` execution status to an error state for display (`src/hooks/use-agent-state.ts:30-31`).
3. **Escalation is rich and code-aware.** Error banners distinguish sticky conversation errors from auto-clearing connection errors (`src/stores/error-message-store.ts:4-9`) and offer context-specific recovery actions: Retry (reconnect), Re-authenticate for `ACPAuthRequired`, dismiss, copy (`src/components/features/chat/chat-interface.tsx:583-599`, `src/components/features/chat/error-message-banner.tsx:141-207`).
4. **Delegation failures are converted into corrective guidance instead of hard errors** — the `launch_child_conversation` tool handler "Never rejects" (`src/services/child-conversation-launch.ts:499-503`), telling the agent when to retry or fall back from cloud to local.
5. **A circuit breaker bounds futile persistence**: after 5 consecutive failed health probes, polling is disabled and persisted across refreshes until the user intervenes (`src/hooks/query/use-backends-health.ts:220-227`, `src/api/backend-registry/health-storage.ts:10`).

The `/goal` loop feature is the clearest window into the harness's own persistence model as seen from the client: rounds stream live with iteration/max counts, terminal states include `capped` (gave up at the iteration budget) and `interrupted` (resumable), and the UI exposes Stop/Resume controls (`src/types/agent-server/core/events/conversation-state-event.ts:82-97`, `src/components/features/chat/goal-status-content.tsx:22-27`, `goal-status-content.tsx:87-128`).

## Rating

**7 / 10**

Rationale against the rubric:

- **Why not lower:** There is a clear, documented model — bounded retries with explicit non-retryable classifications, escalation surfaces tied to structured error codes, a persisted circuit breaker, and a resumable goal loop. Behavior is backed by targeted tests (retry-then-recover probes: `__tests__/hooks/query/use-backends-health.test.tsx:135`; replay-idempotent delegation: `__tests__/services/child-conversation-launch.test.ts:488-490`; stop/resume semantics: `__tests__/components/features/chat/goal-status-content.test.tsx:44-73`).
- **Why not higher:** (a) The core persistence decisions — LLM-call retries, replanning strategy, judge cadence — happen in a sibling repo and are only pass-through-configurable here, so the dimension's central question can only be partially answered from this source; (b) retry policy is scattered per-hook with no single declarative layer (dozens of one-off `retry:` settings across `src/hooks/query/*`); (c) WebSocket reconnect defaults to unbounded attempts (`maxAttempts ?? Infinity`, `src/hooks/use-websocket.ts:113-114`) which conflicts with the otherwise strict bounded-retry discipline; (d) `STUCK → ERROR` mapping is acknowledged as a placeholder ("Map STUCK to ERROR for now", `src/hooks/use-agent-state.ts:31`), losing a distinct stuck-vs-failed signal in the UI.

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Agent state taxonomy incl. PAUSED/STOPPED/ERROR/RATE_LIMITED/AWAITING_USER_CONFIRMATION | `AgentState` enum defines all terminal + waiting states used across the UI | `src/types/agent-state.tsx:1-15` |
| Execution-status → state mapping; STUCK collapsed into ERROR | `mapExecutionStatusToAgentState()` maps `ExecutionStatus.STUCK` to `AgentState.ERROR` with a "for now" comment | `src/hooks/use-agent-state.ts:17-35` |
| Shared API retry helper | `withRetry(fn, maxRetries=3, baseDelayMs=500)` with exponential backoff, rethrows after exhaustion | `src/api/with-retry.ts:4-25` |
| WS reconnect backoff + jitter | Bounds 1 s→30 s cap; jitter added "so parallel sockets (main + planning) don't retry in lockstep"; attempt count reset on success | `src/hooks/use-websocket.ts:18-20`, `use-websocket.ts:112-136` |
| WS reconnect configurability | `reconnect: { enabled?, maxAttempts? }` options, default unbounded (`Infinity`) | `src/hooks/use-websocket.ts:12-15`, `use-websocket.ts:113-114` |
| Per-query retry discipline | `retry: 1` cap so a slow history load cannot hold the WebSocket gate closed; missed events replay over WS `since` instead | `src/hooks/query/use-conversation-history.ts:86-96` |
| Conditional retry on settings fetch | `retry: (_, error) => getErrorStatus(error) !== 404` — decided auth failures not retried | `src/hooks/query/use-settings.ts:145` |
| Health-probe quick retry with non-retryable classification | `PROBE_RETRY_ATTEMPTS = 2`, 300 ms delay; `isRetryableProbeError()` excludes invalid-key/logged-out errors; rationale documents why retry lives inside the query fn | `src/hooks/query/use-backends-health.ts:140-186` |
| Circuit breaker / give-up | `MAX_CONSECUTIVE_FAILURES = 5` disables polling per backend; disabled state persisted in localStorage so refresh doesn't re-arm; manual one-shot re-probe supported | `src/hooks/query/use-backends-health.ts:220-227`, `src/api/backend-registry/health-storage.ts:10`, `use-backends-health.ts:203-210` |
| Loop bound config (max_iterations) | Sent on conversation start with fallback 500; typed on Settings and on the wire payload | `src/api/agent-server-adapter.ts:1122-1125`, `src/types/settings.ts:130`, `src/services/settings.ts:15` |
| Stuck detection enabled client-side | `stuck_detection: true` in the start-conversation payload | `src/api/agent-server-adapter.ts:1126` |
| Autonomy level: confirmation policy | `confirmation_mode=false → NeverConfirm`; `AlwaysConfirm`; or `ConfirmRisky(threshold HIGH)` when the LLM security analyzer is on | `src/api/agent-server-adapter.ts:593-605` |
| Autonomy level: security analyzers | `llm` / `pattern` / `policy_rail` analyzer selection forwarded to the server | `src/api/agent-server-adapter.ts:607-618` |
| Confirmation gate UI (escalate-to-human mid-run) | Confirm/reject buttons rendered only in `AWAITING_USER_CONFIRMATION`, keyboard shortcuts, duplicate-submission guard, high-risk warning banner | `src/components/shared/buttons/conversation-confirmation-buttons.tsx:16-135` |
| Delegation tool with guidance-not-failure escalation | `handleLaunchChildConversationAction` "Never rejects"; failures return `{status:"error", error, guidance}` instructing the agent to retry or fall back to local | `src/services/child-conversation-launch.ts:68-89`, `child-conversation-launch.ts:499-536` |
| Worktree-launch fallback retry | If worktree isolation fails on launch, retry once without it rather than losing the launch; consequence explained in `isolation_note` | `src/services/child-conversation-launch.ts:308-323` |
| Delegation replay idempotency ledger | `claimToolCall()` marks tool calls handled before network work so socket-reconnect replays cannot double-launch billable Cloud conversations | `src/services/child-conversation-launch.ts:196-227` |
| Goal-loop statuses: running/complete/capped/interrupted | `GoalStatus` type streamed as ConversationStateUpdateEvent with iteration/max_iterations/verdict | `src/types/agent-server/core/events/conversation-state-event.ts:66-97` |
| Goal-loop Stop = cancel loop + interrupt turn | Backend `stopGoal` deliberately leaves the in-flight turn running, so the UI also calls `pauseConversation`; Resume continues an interrupted loop | `src/hooks/mutation/conversation-mutation-utils.ts:94-115`, `src/components/features/chat/goal-status-content.tsx:87-95` |
| Per-goal iteration-cap override | `/goal --max N <objective>` interceptor parses the flag into `StartGoalRequest.max_iterations` | `src/hooks/chat/use-goal-interceptor.ts:11-13`, `use-goal-interceptor.ts:38-54` |
| Pause/stop semantics differ by backend | Cloud pause waits for the current LLM call; local `/interrupt` cancels in-flight requests immediately | `src/hooks/mutation/conversation-mutation-utils.ts:36-61` |
| Sticky vs self-healing errors | Connection errors auto-clear on recovery; conversation errors clear only on explicit dismiss/retry/new message | `src/stores/error-message-store.ts:4-9`, `error-message-store.ts:62-65` |
| Code-specific escalation actions | `ACPAuthRequired` maps to a dedicated header plus a Re-auth button navigating to `/settings/agents` | `src/utils/acp-error-codes.ts:8-29`, `src/components/features/chat/chat-interface.tsx:594-598` |
| Error classification drives severity rendering | Classified non-internal errors render a warning icon vs hard-error icon | `src/components/features/chat/error-message-banner.tsx:103-120` |
| Persistence-decision telemetry | `trackError()` emits `error_outcome` events with `error_kind` / `error_id` correlation (no raw messages) for both conversation and agent error events | `src/utils/error-handler.ts:17-47`, `src/contexts/conversation-websocket-context.tsx:570-608` |
| Optimistic-send watchdog with retry link | `PENDING_MESSAGE_TIMEOUT_MS = 150_000` flips a stuck "sending" bubble to "error … with a retry link" | `src/stores/optimistic-user-message-store.ts:14`, `optimistic-user-message-store.ts:131-139` |
| Anti-infinite-retry pagination fallback | Cloud event search returns an empty page when timestamp filters are unsupported "so the UI doesn't retry indefinitely" | `src/api/event-service/event-service.api.ts:149-163` |
| Sandbox resume escalation path | On `sandbox_status === "PAUSED"` the route calls `resumeCloudSandbox` once per mount (ref-guarded); fast-poll (3 s) until RUNNING, then the WS connects | `src/routes/conversation.tsx:143-178`, `src/hooks/query/use-active-conversation.ts:19-34` |
| User-attention notification on handoff | Sound plays on transitions into `AWAITING_USER_INPUT` / `FINISHED` / `AWAITING_USER_CONFIRMATION` | `src/hooks/use-agent-notification.ts:6-10` |
| Tests: probe recovers after transient failure | "recovers when a transient first probe fails, then succeeds on retry" | `__tests__/hooks/query/use-backends-health.test.tsx:135` |
| Tests: breaker state persisted and recoverable | Refresh keeps disabled backend unprobed; editing the backend re-arms polling | `__tests__/hooks/query/use-backends-health.test.tsx:302`, `:336` |
| Tests: delegation replay ignored | "ignores a replayed tool call" (socket reconnect / REST-WS race scenario) | `__tests__/services/child-conversation-launch.test.ts:488-490` |
| Tests: goal stop/resume controls | Stop cancels loop AND interrupts agent; Resume continues interrupted goal; no controls when complete | `__tests__/components/features/chat/goal-status-content.test.tsx:44-73` |
| Tests: execution-status mapping precedence | Live WS status preferred over cached status; cached fallback when empty | `src/hooks/use-agent-state.test.tsx:26-45` |

## Answers to Dimension Questions

### 1. Does the agent persist or escalate on failure?

Both, at strictly separated layers. Transport-layer persistence is automatic but bounded: `withRetry` (3 attempts, exponential backoff, `src/api/with-retry.ts:4-25`), WS reconnect with capped-backoff+jitter (`src/hooks/use-websocket.ts:125-136`), and probe quick-retries (`src/hooks/query/use-backends-health.ts:173-186`). When retries exhaust — or the failure is classified as decided (invalid key, logged out, unsupported filter) — the system stops trying and escalates to the human: sticky error banners with recovery actions (`src/stores/error-message-store.ts:4-9`, `chat-interface.tsx:583-599`), confirmation prompts that freeze the input until answered (`interactive-chat-box.tsx:62-66`), toasts from the global query/mutation error handlers (`src/query-client-config.ts:41-77`), and a persisted circuit breaker that halts probing entirely (`use-backends-health.ts:220-227`). For the *agent's own* loop, the client neither persists nor replans; it configures the server's budget (`max_iterations`, `stuck_detection`, `agent-server-adapter.ts:1122-1126`) and renders the outcome (`capped`/`interrupted`, `goal-status-content.tsx:22-27`).

### 2. Is persistence configurable?

Yes, at three levels:

- **Loop budget**: `max_iterations` is a first-class setting surfaced through schema-driven UI (label/description i18n keys `SCHEMA$MAX_ITERATIONS$LABEL`/`DESCRIPTION`, `src/i18n/translation.json:4507`, `:4490`), defaulted/fallback to 500 (`src/api/agent-server-adapter.ts:1122-1125`), and overridable per-goal via a chat command flag `/goal --max N` (`src/hooks/chat/use-goal-interceptor.ts:52-54`).
- **Autonomy level**: `confirmation_mode` selects `NeverConfirm` / `AlwaysConfirm` / risk-threshold-gated `ConfirmRisky` (`src/api/agent-server-adapter.ts:593-605`); `enable_sub_agents === true` gates whether the agent may delegate at all by attaching `task_tool_set` (`src/api/agent-server-adapter.ts:631-644`); sound notifications fire when the agent hands control back (`src/hooks/use-agent-notification.ts:6-10`).
- **Transport persistence**: `reconnect.enabled` / `reconnect.maxAttempts` options on the WS hook (`src/hooks/use-websocket.ts:12-15`); per-hook React Query `retry` values (e.g., `src/hooks/query/use-conversation-history.ts:96`, `use-git-sync.ts:30-31`); probe attempts/delay constants (`use-backends-health.ts:140-141`). Note these transport knobs are code-level constants/options, not end-user settings — configurability here is developer-facing only.

### 3. Are escalation paths clear?

Yes — unusually so. Escalation is keyed off structured data rather than string matching: server error events carry `code` + `classification` (`src/contexts/conversation-websocket-context.tsx:572-591`), which map to code-specific headers and a dedicated re-auth action for `ACPAuthRequired` (`src/utils/acp-error-codes.ts:8-29`; `error-message-banner.tsx:141-150`). Mid-task escalation to confirmation has its own state (`AWAITING_USER_CONFIRMATION`), dedicated UI, shortcuts, and a high-risk warning (`conversation-confirmation-buttons.tsx:92-135`). Delegation failures carry machine-readable `guidance` telling the agent exactly how to recover ("retry, or fall back to target=local", `child-conversation-launch.ts:428`, `:525`). The one muddy spot: `STUCK` is folded into `ERROR` (`src/hooks/use-agent-state.ts:31`), so a stuck-but-alive agent presents identically to a failed one.

### 4. Are persistence decisions observable?

Largely. Goal-loop round-by-round progress (iteration, max, judge verdict/score) streams into the event store and stays in the timeline after termination (`src/components/features/chat/goal-status-banner.tsx:8-14`; `goal-status-content.tsx:130-168`). Client-side error handling emits `error_outcome` analytics with `error_kind` and correlatable `error_id` while deliberately excluding raw messages (`src/utils/error-handler.ts:37-46`). Health state — failure counts and last error per backend — is persisted to localStorage and rendered as connectivity dots (`use-backends-health.ts:220-227`). Gaps: retry attempts themselves (backoff timers, exhausted `withRetry` loops) are not individually logged or instrumented; you see outcomes, not attempts.

## Architectural Decisions

1. **Ownership split by design.** Persistence-of-execution lives in `software-agent-sdk`; the canvas owns configuration, observation, and human escalation (`AGENTS.md` repo-map table). This study's score therefore reflects the frontend half of a two-repo contract.
2. **Retry classification over blanket retry.** Every retry site distinguishes transient from decided failures — auth errors, 404s, and unsupported-feature responses are excluded from retry (`use-settings.ts:145`; `use-backends-health.ts:160-171`; `event-service.api.ts:149-163`). The comments record operational scar tissue (e.g., git-sync retries once only because "three retries plus backoff sat the page in its skeleton for about two minutes", `use-git-sync.ts:24-29`).
3. **Circuit breaker with durable state.** Rather than retrying forever against a dead backend, five consecutive failures disable polling; the disabled flag survives refresh in localStorage and requires either a config edit or an explicit one-shot re-probe to clear (`use-backends-health.ts:203-227`).
4. **Client tools acknowledge-first, then report asynchronously.** The agent-server acks `launch_child_conversation` before the browser acts, so all outcomes — success or corrective guidance — must flow back as a message to the agent (`child-conversation-launch.ts:451-458`). This forces every failure mode into an actionable instruction, an unusual but effective escalation design.
5. **Idempotency ledger instead of optimistic retries.** Non-idempotent launches are claimed by `toolCallId` before any network work, trading "never launching" risk for "never double-launching" safety, with a corrupt-ledger fail-open (`child-conversation-launch.ts:196-227`).
6. **Backend-aware stop semantics.** One `pauseConversation` API compiles to graceful sandbox pause on Cloud vs immediate interrupt locally, matching each platform's cost/latency profile (`conversation-mutation-utils.ts:36-61`).

## Notable Patterns

- **Jittered, lockstep-breaking reconnects**: parallel sockets deliberately desynchronize their retries so they don't "hammer an already-struggling server" (`src/hooks/use-websocket.ts:125-127`).
- **Watchdog-to-retry-link conversion**: an optimistic send that never gets echoed flips from "Sending…" to an explicit error bubble with a retry affordance after 150 s (`optimistic-user-message-store.ts:131-139`) — turning a silent hang into a user decision.
- **Fallback-with-explanation**: degraded launches succeed with `isolation_note` / `parent_link_note` fields explaining what was lost, rather than failing outright (`child-conversation-launch.ts:300-323`, `:241-250`).
- **Resume-on-navigate**: PAUSED cloud sandboxes trigger exactly one resume call per mount (ref-guarded) and rely on fast-polling rather than retry storms (`routes/conversation.tsx:156-170`; `use-active-conversation.ts:19-34`).
- **Terminal-goal-inlining**: active goals pin a live banner; finished/capped goals settle inline into the transcript so the record persists in the conversation (`goal-status-banner.tsx:8-14`).
- **Spec-tagged tests**: tests encode the persistence contract (probe recovery `__tests__/hooks/query/use-backends-health.test.tsx:135`, replay immunity `__tests__/services/child-conversation-launch.test.ts:488-490`).

## Tradeoffs

- **Frontend simplicity vs observability of the loop.** Because replanning/judging lives server-side, the client shows verdicts and round counters but cannot explain *why* the judge scored 60% or why a round was cut short beyond the `missing` note (`conversation-state-event.ts:68-75`).
- **Unbounded WS reconnect vs bounded everything else.** Defaulting `maxAttempts` to Infinity (`use-websocket.ts:113-114`) maximizes session survival but contradicts the repo's own circuit-breaker instincts; the mitigation is jitter+cap-on-delay, not attempt caps.
- **Fail-open ledger.** A full localStorage drops the idempotency claim and proceeds, "accepting replay risk over never launching at all" (`child-conversation-launch.ts:222-225`) — a cost-safety tradeoff resolved in favor of availability.
- **Per-hook retry tuning vs coherence.** Dozens of individual `retry:` decisions give precise control but make global questions ("how often do we retry GETs?") unanswerable without auditing each hook.
- **STUCK→ERROR conflation** trades UI nuance for implementation speed today (`use-agent-state.ts:31`).

## Failure Modes / Edge Cases

- **Replay storms**: socket reconnects with `resend_mode: 'all'` can replay ActionEvents; side-effectful handlers guard via event-id dedup (`conversation-websocket-context.tsx:556-568`) and the delegation ledger (`child-conversation-launch.ts:196-227`).
- **Stuck handshake**: sockets wedged in CONNECTING are aborted by a watchdog so Chrome's per-host handshake lock isn't held forever (`use-websocket.ts:61-64`).
- **Echo mismatch**: if the server munges a message body, FIFO fallback pops the oldest pending bubble so none pins forever (`optimistic-user-message-store.ts:169-198`).
- **Unsupported pagination filters**: older Cloud backends cause an intentional empty page + console warning instead of infinite refetch (`event-service.api.ts:153-162`).
- **Cold-start provisioning**: Cloud start-task polling is bounded at 180 s × 3 s interval; on timeout the still-provisioning task is reported rather than hanging (`child-conversation-launch.ts:36-38`, `:357-384`).
- **Duplicate confirmations**: submitted confirmation event ids are tracked to prevent double-submission (`conversation-confirmation-buttons.tsx:44-47`).
- **Stale sandbox URL**: the WS suppresses a paused sandbox's stale URL to avoid connecting to a dead host before wake-up (`AGENTS.md` cloud-resume note; implemented via `websocket-provider-wrapper.tsx:31`).

## Future Considerations

- Surface `STUCK` as a distinct UI state (with its own recovery suggestion) instead of aliasing `ERROR` (`src/hooks/use-agent-state.ts:31`).
- Consolidate the ~30 ad-hoc React Query `retry` configurations into a shared policy helper with named classifications (transient / decided / unsupported), making the retry posture auditable in one place.
- Cap or make observable the default-unbounded WS reconnect (`src/hooks/use-websocket.ts:113-114`), e.g., emit `trackError` when reconnect attempts exceed a threshold.
- Instrument individual retry attempts (currently invisible between outcome events) so persistence decisions become fully auditable from telemetry.
- Expose transport-level persistence knobs (probe attempts, poll intervals like the 3 s/30 s split in `use-active-conversation.ts:23-34`) through configuration rather than compiled constants, for embedding hosts.

## Questions / Gaps

- **What happens inside the loop when an LLM call fails?** Not answerable from this source. The client enables `stuck_detection` and forwards errors, but retry/backoff of inference calls lives in `OpenHands/software-agent-sdk` (per `AGENTS.md`'s ownership table). Searched: `retry`, `max_iterations`, `stuck`, `rate_limit` across `src/`; only pass-through config and display were found.
- **Does the condenser or budget cap interact with `max_iterations`?** `LLMMetrics.max_budget_per_task` exists on the type (`src/types/agent-server/core/events/conversation-state-event.ts:28`) but no client logic ties budget exhaustion to persistence decisions. No evidence found.
- **Is there a rate-limit recovery strategy?** `AgentState.RATE_LIMITED` is defined (`src/types/agent-state.tsx:11`) and referenced only in status gating (`src/components/features/controls/agent-status.tsx:73-74`); no automatic wait-and-resume logic was found client-side.
- **Who decides "capped" vs "complete"?** The judge runs server-side; the client renders `GoalVerdict.score/complete/missing` (`conversation-state-event.ts:66-75`) but the acceptance threshold is not visible here. No evidence found in this repository.
- **Are there other autonomy levels beyond confirmation_mode/sub-agents?** The settings schema mock shows only these two autonomy-shaped knobs (`src/mocks/settings-handlers.ts:380-445`); deeper ladders (full-auto vs supervised tiers) would live server-side. No evidence found in this source.

---

Generated by `dimensions/23.02-persistence-vs-escalation-philosophy` against `openhands`.

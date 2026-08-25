# Source Analysis: openhands

## Dimension 15.02 — Message Routing and Termination

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (OpenHands "agent-canvas" frontend) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19, Zustand, TanStack Query, Vite; talks to the Python `openhands-agent-server` (separate repo) via REST + WebSocket through `@openhands/typescript-client` |
| Analyzed | 2026-08-25 |

**Scope caveat (important for interpreting this study).** This source is the OpenHands *frontend*. Per the repo's own architecture note (`AGENTS.md`, repository map table), the agent execution loop, tool dispatch, and server-side termination enforcement live in the sibling `software-agent-sdk` repo, which is outside this source's isolation boundary. Consequently this report studies what this source actually implements and governs: client-side message routing over WebSocket/REST, event classification, handoff contracts exposed to agents via client-defined tools, termination *surfacing and control* (pause/interrupt, confirmation gates, goal-loop caps), and client-side deadlock-avoidance machinery. Where a behavior is only mirrored from the server, that is stated explicitly rather than inferred.

## Summary

OpenHands routes all conversation traffic through a single provider, `ConversationWebSocketProvider` (`src/contexts/conversation-websocket-context.tsx:121`), which maintains up to two parallel WebSocket connections per route — one for the main agent conversation and one for the planning sub-conversation — and demultiplexes every incoming frame through a chain of runtime type guards (`src/types/agent-server/type-guards.ts:45-301`) into store writes, cache invalidations, error banners, terminal/browser panels, or client-tool side effects (`src/contexts/conversation-websocket-context.tsx:538-758`, `760-961`). There is **no speaker-selection mechanism**: the harness is one user ↔ one agent per conversation (plus exactly one planning sub-conversation), not a group chat; turn-taking is implicit in `ExecutionStatus` transitions streamed by the server (`src/types/agent-server/core/base/common.ts:67-75`).

Handoffs are implemented as *client-defined tools*: the server acknowledges a `launch_child_conversation` call immediately, and the browser executes it — validating parameters the server's schema cannot express (`src/services/child-conversation-launch.ts:110-194`), guarding against replayed launches with a localStorage ledger (`src/services/child-conversation-launch.ts:205-227`), then posting a machine-readable result message back into the parent conversation so the agent learns the child's id (`src/services/child-conversation-launch.ts:459-497`; contract at `src/constants/child-conversation.ts:20`).

Termination is multi-layered: an explicit `ExecutionStatus` state machine including terminal `FINISHED`, `ERROR`, and a distinct `STUCK` state (`src/types/agent-server/core/base/common.ts:67-75`); user-initiated pause/interrupt with cloud-vs-local semantics (`src/hooks/mutation/conversation-mutation-utils.ts:41-61`); a confirmation gate that blocks the loop on `WAITING_FOR_CONFIRMATION` until the human accepts/rejects (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:92-100`); and `/goal` auto-termination loops bounded by judge verdicts and a user-settable `max_iterations` cap (`src/types/agent-server/core/events/conversation-state-event.ts:82-97`, `src/hooks/chat/use-goal-interceptor.ts:51-54`, `src/api/agent-server-adapter.ts:1122-1126`). Deadlock prevention is a mix of server-requested stuck detection (`stuck_detection: true` always sent, `src/api/agent-server-adapter.ts:1012,1126`), handshake watchdogs (`src/hooks/use-websocket.ts:61-64`), jittered exponential reconnect backoff (`src/hooks/use-websocket.ts:18-20,125-132`), dedup of replayed events (`src/stores/use-event-store.ts:95-107`, context handler `#1656` guard), and bounded polling loops (`src/services/child-conversation-launch.ts:36-38`).

**Can a multi-agent conversation terminate without human intervention?** Within this source's visibility: yes, partially — runs reach `FINISHED`/`ERROR`/`STUCK` statuses on their own, and a `/goal` loop self-terminates when its judge reports `complete` or when the iteration counter hits `max_iterations` ("capped") with no human input (`src/components/features/chat/goal-status-content.tsx:22-27`). However, the actual enforcement of those terminations happens server-side in `software-agent-sdk`; this repo faithfully renders the outcomes and provides manual Stop/Resume controls.

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale against the rubric:

- **Clear model**: routing has one canonical entry point and a documented dispatch taxonomy (`src/contexts/conversation-websocket-context.tsx:547-741`); termination states are an explicit enum (`src/types/agent-server/core/base/common.ts:67-75`).
- **Tests**: substantial coverage exists for the highest-risk paths — launch validation/fallback/replay (`__tests__/services/child-conversation-launch.test.ts:122-560`), replayed-event idempotency (`__tests__/contexts/conversation-websocket-context.test.tsx:512-534`), socket keep-alive across refetches (`__tests__/contexts/conversation-websocket-context.test.tsx:222`), goal interception (`__tests__/hooks/chat/use-goal-interceptor.test.ts`), and raw WebSocket lifecycle (`__tests__/hooks/use-websocket.test.ts`).
- **Explicit interfaces**: typed guards (`isAgentServerEvent`, `src/types/agent-server/type-guards.ts:45`), a `ClientToolSpec` contract with MCP-style annotations (`src/api/canvas-ui-client-tool.ts:9-20`), and versioned feature gating (`MIN_AGENT_SERVER_VERSION_FOR_PARENT_LINK = "1.37.1"`, `src/constants/child-conversation.ts:37`).
- **Operational safeguards**: dedup ledgers, delta batching with flush-before-non-delta ordering (`src/contexts/conversation-websocket-context.tsx:549-554`), bounded cloud polls, backoff+jitter reconnect.

Why not higher: the `STUCK` state is lossily collapsed into `ERROR` on the client with an acknowledged TODO (`src/hooks/use-agent-state.ts:30-31`); the main/planning WS handlers are ~200 lines of intentionally duplicated logic (`src/contexts/conversation-websocket-context.tsx:649-651` comment admits the duplication); reconnect attempts are uncapped by default (`Infinity`, `src/hooks/use-websocket.ts:113-114`) with no consumer setting `maxAttempts`; there is no application-level heartbeat to detect a silently dead-but-OPEN socket; and true loop termination/deadlock enforcement is delegated to a sibling repo, so this source alone cannot guarantee termination.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Routing hub | Single provider owns both main + planning sockets and all inbound dispatch | `src/contexts/conversation-websocket-context.tsx:121-135` |
| Inbound dispatch chain | Type-guard cascade over every WS frame (deltas, errors, messages, actions, state updates, bash, browser, canvas UI, child launch) | `src/contexts/conversation-websocket-context.tsx:547-741` |
| Event taxonomy | Runtime type guards classify events by structural shape + `source` ∈ {agent, user, environment, hook} | `src/types/agent-server/type-guards.ts:45-62,104-120` |
| Streaming order safety | Deltas buffered per-frame and flushed before any non-delta event so text cannot overtake the message that closes it | `src/contexts/conversation-websocket-context.tsx:549-554` |
| Replay protection | Duplicate event ids skip non-idempotent side effects (#1656) | `src/contexts/conversation-websocket-context.tsx:556-568`; `src/stores/use-event-store.ts:100-107` |
| Outbound routing | `sendMessage` picks socket by `conversationMode === "plan"`, falls back to REST queue `{run: true}` when socket closed | `src/contexts/conversation-websocket-context.tsx:1094-1144` |
| Optimistic echo matching | Pending "Sending…" bubbles consumed by exact echoed content, FIFO fallback within a conversation | `src/stores/optimistic-user-message-store.ts:169-189` |
| History anchor | WS subscribes `resend_mode='since'&after_timestamp=<latest REST event>`; falls back to `'all'` | `src/contexts/conversation-websocket-context.tsx:368-400,963-973` |
| Speaker selection | No speaker/group-chat code found (searches for `speaker`, `group_chat`, `select_speaker` returned nothing); single agent + one planning sub-conversation | search boundary noted in Questions/Gaps; dual-socket design at `src/contexts/conversation-websocket-context.tsx:136-140,402-421` |
| Execution state machine | `ExecutionStatus = IDLE/RUNNING/PAUSED/WAITING_FOR_CONFIRMATION/FINISHED/ERROR/STUCK` | `src/types/agent-server/core/base/common.ts:67-75` |
| Status → UI mapping | Live WS status preferred over polled REST status; STUNK mapped to ERROR "for now" | `src/hooks/use-agent-state.ts:10-35` |
| Handoff contract (client tool) | `launch_child_conversation` spec: local/cloud target, worktree/shared isolation, self-contained task brief, `idempotentHint: false` | `src/api/launch-child-conversation-client-tool.ts:14-112` |
| ClientToolSpec interface | Shared tool-spec shape incl. readOnly/destructive/idempotent/openWorld hints | `src/api/canvas-ui-client-tool.ts:9-20` |
| Handoff executor | Client validates enum/cross-target rules the server schema can't express; never rejects — failures become corrective guidance | `src/services/child-conversation-launch.ts:99-194,499-504` |
| Handoff replay ledger | `claimToolCall()` claims tool_call_id in localStorage before network work; replays dropped mid-flight too | `src/services/child-conversation-launch.ts:196-227,510` |
| Result reporting back | Outcome posted as `[child-conversation] {...}` user message; hidden from chat UI, relayed by agent; suppressed while goal loop active | `src/constants/child-conversation.ts:11-20`; `src/services/child-conversation-launch.ts:481-497`; `src/components/conversation-events/chat/event-content-helpers/should-render-event.ts:51` |
| Isolation fallback | Worktree launch degrades to shared directory (with consequence note) when parent workspace can't host a worktree or creation fails | `src/services/child-conversation-launch.ts:265-323` |
| Versioned link contract | Parent links require agent-server ≥ 1.37.1; older servers get honest "not linked" note instead of silent assumption | `src/constants/child-conversation.ts:30-37`; `src/services/child-conversation-launch.ts:241-250` |
| Manual stop | Cloud pauses sandbox (drains current LLM call); local uses `/interrupt` to cancel in-flight requests immediately | `src/hooks/mutation/conversation-mutation-utils.ts:36-61` |
| Stop UX consistency | Cache patched with `execution_status=PAUSED` AND `sandbox_status=PAUSED` together so stale-host WS attempts are gated | `src/hooks/mutation/use-unified-stop-conversation.ts:58-70` |
| Confirmation gate | Buttons render only in `AWAITING_USER_CONFIRMATION`; duplicate-submission guard; high-risk alert from `security_risk`; keyboard shortcuts | `src/components/shared/buttons/conversation-confirmation-buttons.tsx:30-58,92-118` |
| Confirmation endpoint | POST `/api/conversations/{id}/events/respond_to_confirmation` (cloud proxied vs direct) | `src/api/event-service/event-service.api.ts:40-69`; `src/hooks/mutation/use-respond-to-confirmation.ts:12-32` |
| Goal loop termination | `GoalStatus {active, running\|complete\|capped\|interrupted, iteration, max_iterations}` streamed as ConversationStateUpdateEvents | `src/types/agent-server/core/events/conversation-state-event.ts:79-97`; consumed at `src/contexts/conversation-websocket-context.tsx:652-654` |
| Goal iteration cap | `/goal --max N` parsed client-side; start payload sends `max_iterations` (default 500) and `stuck_detection: true` | `src/hooks/chat/use-goal-interceptor.ts:11-13,51-54`; `src/api/agent-server-adapter.ts:1011-1013,1121-1127` |
| Goal stop composition | Stop = `stopGoal` + `pauseConversation` because backend deliberately leaves the in-flight turn running | `src/components/features/chat/goal-status-content.tsx:87-93`; `src/hooks/mutation/conversation-mutation-utils.ts:94-106` |
| Paused-sandbox routing gate | WS URL suppressed to `null` while `sandbox_status === "PAUSED"`; fast 3 s poll detects wake-up | `src/contexts/websocket-provider-wrapper.tsx:24-33`; `src/hooks/query/use-active-conversation.ts:17-35` |
| Reconnect policy | Exponential backoff 1 s→30 s cap with ≤30 % random jitter so parallel sockets don't retry in lockstep | `src/hooks/use-websocket.ts:18-20,125-132` |
| Handshake watchdog | Sockets stuck in CONNECTING are aborted (browsers never time out CONNECTING) | `src/hooks/use-websocket.ts:61-64`; `src/utils/websocket-handshake.ts:1` |
| Stuck surfacing | `STUCK` treated as errored for activity checks and falls through to AGENT_ERROR_MESSAGE label | `src/utils/status.ts:25-29,101-116` |
| Tests: handoff | Validation, worktree fallbacks, replay ignore, bounded cloud wait, goal suppression | `__tests__/services/child-conversation-launch.test.ts:122-560` |
| Tests: routing idempotency | Replayed bash events not re-appended; dismissed error banner not re-raised | `__tests__/contexts/conversation-websocket-context.test.tsx:512-548` |

## Answers to Dimension Questions

### 1. How are messages routed?

Inbound: every WebSocket frame is JSON-parsed inside `handleMainMessage` / `handlePlanningMessage` (`src/contexts/conversation-websocket-context.tsx:538-758`, `760-961`). Valid events (`isAgentServerEvent`) first pass through streaming-delta batchers — deltas enqueue into a per-frame coalescer and are force-flushed before any non-delta event so ordering is preserved (`src/contexts/conversation-websocket-context.tsx:549-554`). Then a guard cascade routes each event to exactly the stores/effects it concerns: error banners (`isDisplayableErrorEvent`), inline LLM-error tracking (`isAgentErrorEvent`), optimistic-bubble consumption (`isUserMessageEvent` matched by echoed content, `src/contexts/conversation-websocket-context.tsx:610-624`), query-cache invalidation (`isActionEvent` → `handleActionEventCacheInvalidation`), execution-status/goal/stats updates (`isConversationStateUpdateEvent` narrowed by `key`: `full_state`/`execution_status`/`stats`/`goal`, lines `639-655`), terminal/browser panel feeds, model-switch bookkeeping, and two *client tools* executed browser-side (`isCanvasUIActionEvent`, `isLaunchChildConversationActionEvent`, lines `727-740`). The event store itself dedups by id before appending (`src/stores/use-event-store.ts:100-107`), and handlers re-check duplicates before non-idempotent side effects because a reconnect replay is not idempotent end-to-end (`src/contexts/conversation-websocket-context.tsx:556-568`, regression-tested at `__tests__/contexts/conversation-websocket-context.test.tsx:512-548`).

Outbound: `sendMessage` selects the socket by UI mode — `plan` mode targets the planning sub-conversation's socket, otherwise the main one (`src/contexts/conversation-websocket-context.tsx:1096-1098`). If the chosen socket is not OPEN, the message is queued via REST `ConversationClient.sendEvent(..., { run: true })` and delivered by the server when the conversation becomes ready; the caller is told `{queued: true}` so it suppresses optimistic UI (`src/contexts/conversation-websocket-context.tsx:1100-1128`). The planning connection still replays full history (`resend_all=true`, line `1006-1008`) while the main connection anchors to the REST-preloaded tail with `resend_mode='since'&after_timestamp` (lines `963-973`).

### 2. How is the next speaker selected?

There is none — by design. Searches for `speaker`, `group_chat`, and `select_speaker` across `src/` return nothing; the harness is strictly one user ↔ one primary agent per conversation, with at most one hard-coded planning sub-conversation ("Currently, there is only one sub-conversation and it uses the planning agent", `src/contexts/conversation-websocket-context.tsx:407`). Turn-taking is expressed as server-streamed `ExecutionStatus` values (`IDLE` = awaiting user, `RUNNING` = agent's turn, `WAITING_FOR_CONFIRMATION` = blocked on human, mapped to UI states in `src/hooks/use-agent-state.ts:17-31`). The closest thing to multi-party arbitration is *message-direction discipline* between the two conversations: each socket tags its events (`isFromPlanningAgent`, `src/contexts/conversation-websocket-context.tsx:793-797`) and the planning handler scopes optimistic-bubble consumption to the main conversation id so sub-agent echoes can never consume the wrong pending entry (lines `843-854`). Any claim about weighted/round-robin/model-based speaker election would have no evidence in this source.

### 3. How are handoffs managed?

Handoffs use the *client-tool pattern*, which inverts the usual trust flow: the agent-server validates and acknowledges the call before the browser does any work, so the browser must report outcomes back conversationally. Two client tools exist:

- `launch_child_conversation` — the real inter-agent handoff. Its contract is a long prompt-level spec embedded in the tool description: children are independent (no shared history), `task` must be self-contained, one call per task, parameter applicability rules per target (`src/api/launch-child-conversation-client-tool.ts:14-59`), plus JSON Schema with enums and required fields and honesty annotations (`readOnlyHint: false`, `idempotentHint: false`, lines `61-112`).
  - **Validation gap-filling**: the SDK drops JSON-Schema `enum`s when building the pydantic model, so misspelled `target`/`isolation` values arrive intact and are rejected client-side with corrective guidance telling the agent exactly how to retry (`src/services/child-conversation-launch.ts:99-124,173-181`).
  - **Replay safety**: launches are not idempotent (on Cloud, billable), so `claimToolCall` claims the `tool_call_id` in a localStorage ledger *before* any network work, dropping mid-flight replays from socket resend races (`src/services/child-conversation-launch.ts:196-227`; test `__tests__/services/child-conversation-launch.test.ts:490`).
  - **Graceful degradation**: a requested git-worktree isolation falls back to a shared directory — with an explicit consequence note that siblings may conflict over files — both when the parent workspace provably can't host a worktree and when creation throws (`src/services/child-conversation-launch.ts:265-323`; tests `299-395`).
  - **Bounded async**: Cloud children provision asynchronously; polling for the conversation id is capped at 180 s × 3 s interval and surfaces the still-provisioning task rather than hanging (`src/services/child-conversation-launch.ts:36-38,365-384`).
  - **Result channel**: the outcome is posted back as a user message prefixed `[child-conversation] ` carrying id/URL/status or error+guidance (`src/services/child-conversation-launch.ts:459-497`); the chat hides these messages because the agent relays them (`src/components/conversation-events/chat/event-content-helpers/should-render-event.ts:51`). Reporting is suppressed while a goal loop is active, since any inbound message cancels that loop server-side (lines `481-486`; test `510`).
  - **Version honesty**: if the connected agent-server predates parent-link support (< 1.37.1), the result explicitly says the child was created but not linked, instead of letting the agent assume a relationship (`src/constants/child-conversation.ts:30-37`; `src/services/child-conversation-launch.ts:241-250`).
- `canvas_ui` — a same-process "handoff" from agent to UI panel; the server acknowledges immediately and the client applies the panel mutation (`src/contexts/conversation-websocket-context.tsx:724-729`; spec `src/api/canvas-ui-client-tool.ts:60+`; tested in `__tests__/services/canvas-ui.test.ts`).

The planning-agent relationship is *not* a negotiated handoff: `WebSocketProviderWrapper` simply fans out a second socket keyed off `sub_conversation_ids` returned by the server (`src/contexts/websocket-provider-wrapper.tsx:16-44`), using the sub-conversation's own session key when provided (`src/contexts/conversation-websocket-context.tsx:1010-1016`, test at `__tests__/contexts/conversation-websocket-context.test.tsx:299`).

### 4. When does a group conversation terminate?

Termination is observable in this source at four layers:

1. **Natural completion**: the server streams `execution_status: FINISHED` (or `IDLE` back to awaiting-input) via `ConversationStateUpdateEvent`; the UI maps these to localized terminal labels ("Agent finished", "Waiting for task") in `getStatusCode` (`src/utils/status.ts:101-114`).
2. **Human-initiated stop**: the unified stop mutation calls `pauseConversation`, which branches by backend — Cloud pauses the whole sandbox (waits for the current LLM call to drain); local issues `/interrupt` to cancel in-flight LLM requests immediately (`src/hooks/mutation/conversation-mutation-utils.ts:36-61`). On success the cache patches `execution_status` *and* `sandbox_status` to PAUSED atomically so downstream gates fire without waiting for the poll (`src/hooks/mutation/use-unified-stop-conversation.ts:58-70`), and the user is navigated out of the conversation.
3. **Confirmation-gated suspension**: while status is `WAITING_FOR_CONFIRMATION`, the loop is paused pending a human decision rendered as accept/reject buttons with duplicate-submission protection and keyboard shortcuts (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:30-58,92-100`); the decision posts to `events/respond_to_confirmation` (`src/api/event-service/event-service.api.ts:40-69`). This is the harness's built-in "human stays in the loop" checkpoint.
4. **Goal-loop auto-termination**: `/goal` runs are capped by `max_iterations` (user-suppliable via `--max N`, else the conversation-start default of 500) and judged after each round; the streamed `GoalStatus.status` takes exactly one of `running | complete | capped | interrupted` (`src/types/agent-server/core/events/conversation-state-event.ts:82-97`; cap parsing `src/hooks/chat/use-goal-interceptor.ts:11-13,51-54`; defaults `src/api/agent-server-adapter.ts:1121-1127`). A `complete` or `capped` loop terminates **without human intervention**; `interrupted` leaves a Resume affordance guarded against stale-button 409s (`src/components/features/chat/goal-status-content.tsx:46-56,113-128`). Notably, stopping a goal requires *two* calls — `stopGoal` only cancels the background loop while deliberately leaving the in-flight agent turn running, so the UI composes it with `pauseConversation` to truly halt (`src/components/features/chat/goal-status-content.tsx:87-93`; documented at `src/hooks/mutation/conversation-mutation-utils.ts:94-106`).

So: a *conversation* reaches terminal states autonomously (FINISHED/ERROR/STUCK) and a *goal loop* self-terminates on verdict or cap, but the authoritative loop control is server-side; the frontend's role is faithful projection plus manual override.

### 5. Is deadlock possible?

Full deadlock prevention is split across repos, but this source contributes several concrete mechanisms and retains identifiable gaps:

**Mechanisms present:**
- **Stuck detection requested unconditionally**: every conversation-start payload sets `stuck_detection: true` (`src/api/agent-server-adapter.ts:1012,1126`), and the resulting `ExecutionStatus.STUCK` is surfaced distinctly in status dots/emojis and forced into the error path of status text (`src/components/features/conversation-panel/conversation-status-dot.tsx:38`; `src/utils/status.ts:28,101-116`). Note the server-side detector itself lives outside this source.
- **Transport-level liveness**: a handshake watchdog aborts sockets wedged in CONNECTING (which browsers never time out) so they can't hold Chrome's per-host lock forever (`src/hooks/use-websocket.ts:61-64`; `src/utils/websocket-handshake.ts:1`). Reconnects use exponential backoff capped at 30 s with ≤30% random jitter specifically so the main and planning sockets don't retry in lockstep against a struggling server (`src/hooks/use-websocket.ts:18-20,125-132`).
- **History-loading anti-wedge**: if the planning history count fetch fails, loading is force-cleared "to avoid infinite loading state" (`src/contexts/conversation-websocket-context.tsx:1040-1043`); the main path degrades from `since` anchoring to `resend_mode='all'` when no anchor exists so live events still flow (`src/contexts/conversation-websocket-context.tsx:389-391,969-973`); malformed pagination pages are rejected defensively rather than looping (`AGENTS.md`, older-events pagination notes).
- **Idempotency ledgers**: duplicate suppression at store level (`src/stores/use-event-store.ts:100-107`) plus side-effect-level replay guards (#1656) and the launch ledger prevent livelocks caused by redelivery.
- **Poll bounds**: cloud provisioning polls are deadline-bounded (180 s) instead of infinite (`src/services/child-conversation-launch.ts:365-384`).
- **Paused-state routing gate**: suppressing the WS URL while a cloud sandbox is PAUSED prevents a connect→reject→retry churn loop against a stale host; recovery relies on a fast 3 s poll (`src/contexts/websocket-provider-wrapper.tsx:24-33`; `src/hooks/query/use-active-conversation.ts:23-34`).

**Residual risks visible in this source:**
- An OPEN-but-dead socket has no application-level heartbeat/ping; liveness is inferred only from events arriving (`handleNonErrorEvent` clears connection errors on any normal event, `src/contexts/conversation-websocket-context.tsx:210-214`) — silence looks like health.
- Reconnect attempts default to `Infinity` and no consumer passes `maxAttempts` (`src/hooks/use-websocket.ts:112-121`); backoff bounds the rate, not the total.
- Optimistic bubbles acknowledge their own failure mode: if exact content matching fails repeatedly the FIFO fallback can leave "a permanently-stuck bubble" in edge cases (`src/stores/optimistic-user-message-store.ts:174-177`).
- The duplicated main/planning handlers mean a fix applied to one dispatch chain (e.g., a new guard) can silently miss the other — the code comments admit the duplication is intentional, not abstracted (`src/contexts/conversation-websocket-context.tsx:649-651`).

Verdict: classic two-agent deadlock (both sides waiting on each other forever) is structurally prevented from the client side by confirmation gates and status-driven rendering, and runaway loops are bounded server-side via `max_iterations` + `stuck_detection`; but within this repo alone, silent transport death and uncapped reconnects mean "stalled forever with a green-looking socket" remains conceivable.

## Architectural Decisions

1. **One provider, N sockets, guard-based demux.** All routing funnels through `ConversationWebSocketProvider` with per-connection handlers rather than per-component subscriptions; components consume stores, never sockets. Decision evidenced by the provider's ownership of both sockets and the store-write dispatch (`src/contexts/conversation-websocket-context.tsx:121-135,537-961`). Tradeoff: a single 1200-line file concentrates routing logic (and duplicates ~200 lines between handlers), but it gives one auditable place where every event kind is handled.

2. **Client-defined tools with conversational results.** Instead of synchronous tool RPC, client tools are acknowledged server-side and the browser reports outcomes as tagged user messages (`[child-conversation] `, `src/constants/child-conversation.ts:11-20`). This decouples slow/billable browser work from the server's ack path and survives reloads (the ledger), at the cost of a second round-trip through the LLM to deliver results (`src/services/child-conversation-launch.ts:459-497`).

3. **REST-first history, anchored WS replay.** Loading the tail over REST and subscribing with `resend_mode='since'&after_timestamp` eliminates full-history re-streaming on every reconnect; the deliberate gate on `isPending` (not `isFetching`) was specifically introduced to kill a refetch→teardown→reconnect loop that stranded conversations at "Connecting" (`src/contexts/conversation-websocket-context.tsx:376-400` comment; keep-alive test `__tests__/contexts/conversation-websocket-context.test.tsx:222`).

4. **Server-owned state machine, thin client projection.** The client holds no authority over run lifecycle: it maps `ExecutionStatus` to UI states (`src/hooks/use-agent-state.ts:10-35`) and patches caches optimistically only to mirror known server outcomes (`src/hooks/mutation/use-unified-stop-conversation.ts:63-66`). This avoids split-brain but makes fidelity dependent on the mapping — hence the acknowledged STUCK→ERROR conflation debt (`src/hooks/use-agent-state.ts:30-31`).

5. **Honesty-oriented handoff errors.** Every handoff failure mode returns structured `{status:"error", error, guidance}` designed as corrective prompts for the agent rather than thrown exceptions, because "the agent-server has already told it the call succeeded" (`src/services/child-conversation-launch.ts:68-89,499-504`). Same philosophy drives the parent-link version notes (`src/services/child-conversation-launch.ts:241-250`).

## Notable Patterns

- **Flush-before-transition batching**: buffered token deltas are flushed whenever any non-delta event arrives, guaranteeing a closing MessageEvent can never render before the tokens it terminates (`src/contexts/conversation-websocket-context.tsx:549-554`); batchers also double as connectivity probes, clearing connection errors on commit (lines `166-171`).
- **Claim-before-act idempotency ledger**: localStorage-backed claim of `tool_call_id` taken before any async work, tolerant of corrupt/full storage (proceeds accepting replay risk rather than never launching) (`src/services/child-conversation-launch.ts:205-227`).
- **Content-addressed optimistic reconciliation**: pending "Sending…" bubbles are popped by exact echoed-text match with FIFO fallback, safe under out-of-order delivery across conversations/sub-agents (`src/stores/optimistic-user-message-store.ts:169-189`; `src/contexts/conversation-websocket-context.tsx:610-614`).
- **Mode-scoped socket selection**: one `conversationMode` flag decides which of two live sockets carries outbound traffic and which gets reconnected (`src/contexts/conversation-websocket-context.tsx:1077-1090,1096-1098`).
- **Jittered exponential backoff as multi-tenant politeness**: randomness added explicitly because two parallel sockets share fate (`src/hooks/use-websocket.ts:125-132`).
- **Spec-as-prompt tool descriptions**: behavioral contracts (one-call-per-task, wait-for-result-prefix, parameter applicability matrices) live inside tool descriptions consumed by the LLM, i.e., documentation positioned exactly where the model reads it (`src/api/launch-child-conversation-client-tool.ts:14-59`).

## Tradeoffs

- **Centralization vs. modularity**: the monolithic provider makes routing auditable but yields near-duplicate main/planning handlers (~200 lines each) whose divergence risk is mitigated only by comments (`src/contexts/conversation-websocket-context.tsx:649-651,881-883`).
- **Conversational result delivery vs. structured channels**: tagging results into the chat stream is resilient and agent-legible, but hides real data from the UI (messages filtered out, `should-render-event.ts:51`) and depends on the model relaying correctly.
- **Infinite reconnect vs. finite budget**: unlimited attempts maximize eventual reconnection for interactive users but would burn resources unattended; the code exposes `maxAttempts` yet nothing sets it (`src/hooks/use-websocket.ts:12-15,112-121`).
- **Optimistic UX vs. truthfulness**: immediate bubbles plus content-matched reconciliation give snappy feedback but carry the acknowledged stuck-bubble edge case (`src/stores/optimistic-user-message-store.ts:174-177`).
- **Worktree isolation vs. availability**: defaulting to isolated worktrees protects files but fails on scratch workspaces; automatic shared-directory fallback keeps launches succeeding at the cost of introducing potential write conflicts, disclosed via `isolation_note` (`src/services/child-conversation-launch.ts:265-323,269-270`).

## Failure Modes / Edge Cases

- **Replayed ActionEvents** after reconnect/resend could double-execute side effects; mitigated by store dedup + per-handler `isDuplicateEvent` skips (regression tests: `__tests__/contexts/conversation-websocket-context.test.tsx:512-548`; fix reference #1656 at `src/contexts/conversation-websocket-context.tsx:556-558`).
- **Duplicate billable launches** from resend races; prevented by the pre-network ledger claim (`src/services/child-conversation-launch.ts:199-203`), tested at `__tests__/services/child-conversation-launch.test.ts:490`.
- **Cloud child provisioning stall**; bounded by 180 s poll deadline after which the task is reported as still-provisioning instead of hanging the agent (`src/services/child-conversation-launch.ts:357-364`).
- **Stale sandbox host post-pause**; the wrapper nulls the URL during PAUSED and a 3 s fast poll restores it on wake (`src/contexts/websocket-provider-wrapper.tsx:24-33`).
- **Handshake wedge holding the per-host socket lock**; watchdog aborts CONNECTING sockets (`src/utils/websocket-handshake.ts:1`, wired at `src/hooks/use-websocket.ts:61-64`).
- **Malformed/unparseable WS frames**; caught and logged without killing the handler loop (`src/contexts/conversation-websocket-context.tsx:742-744,940-942`).
- **Older servers silently dropping `parent_conversation_id`** (pydantic extra-ignore); surfaced as explicit `parent_link: false` + note instead of a phantom relationship (`src/constants/child-conversation.ts:30-37`).
- **Goal loop cancellation as a side effect**: any inbound message (including an automated launch-result report) cancels an active goal loop, so result reporting is suppressed while goals run (`src/services/child-conversation-launch.ts:481-486`).
- **`STUCK` mislabeled as `ERROR`** in the UI state mapping, losing the distinction for downstream consumers of `curAgentState` (`src/hooks/use-agent-state.ts:30-31`).

## Future Considerations

- Extract the shared skeleton of `handleMainMessage`/`handlePlanningMessage` into a parameterized dispatcher (conversation-id scope, planning flag, plan-file branch) to eliminate the admitted duplication while keeping the two sockets' differences declarative.
- Replace the localStorage ledger with a server-visible idempotency key once the agent-server supports it, removing the client-only replay window (the current ledger already accepts residual risk when storage is unavailable, `src/services/child-conversation-launch.ts:220-225`).
- Add an application-level heartbeat/idle-timeout so a dead-but-OPEN socket converges to the error banner instead of relying on user activity to reveal it; wire `maxAttempts` for background/embedded scenarios.
- Preserve `STUCK` as a first-class UI state (distinct copy, emoji, recovery guidance) instead of collapsing to `ERROR`, now that the enum already models it (`src/hooks/use-agent-state.ts:30-31`).
- Migrate the planning sub-conversation to the REST-then-`since` history pattern used by the main connection (it still streams `resend_all`, `src/contexts/conversation-websocket-context.tsx:183-186`), eliminating the count-based loading heuristic and its force-clear fallback.
- Generalize `subConversations[0]` hard-coding if more than one sub-conversation ever ships (`src/contexts/conversation-websocket-context.tsx:407-409`).

## Questions / Gaps

- **Speaker selection**: not applicable/not present in this source — searches for `speaker`, `group_chat`, `select_speaker` across `src/` found no matches. Any group-conversation arbitration logic would have to live in `software-agent-sdk`, which is outside this study's isolation boundary. No evidence found here.
- **Loop enforcement internals**: how `max_iterations`, `stuck_detection`, and the goal judge actually halt the agent loop is invisible from this repo; only the request flags (`src/api/agent-server-adapter.ts:1011-1013`) and streamed outcomes (`GoalStatus`, `ExecutionStatus`) are observable. Claims about those internals would be unsupported.
- **Confirmation-policy origin**: the start payload includes `confirmation_policy` (`src/api/agent-server-adapter.ts:1008`), populated from `getConversationConfirmationPolicy(conversationSettings)` (`src/api/agent-server-adapter.ts:1121`); the policy's evaluation semantics (what triggers `WAITING_FOR_CONFIRMATION`, e.g., the `security_risk` values seen at `src/components/shared/buttons/conversation-confirmation-buttons.tsx:103-105`) are defined server-side and were not verifiable here.
- **Heartbeat absence**: I found no ping/keepalive frames in `src/hooks/use-websocket.ts` or the handshake utilities; if the server emits protocol-level pings handled by the browser transparently, that would be external evidence — none exists in this source.
- **Multi-child orchestration**: the frontend renders child launches/toasts but does not track or schedule multiple concurrent children (each is fire-and-forget with a URL toast); whether richer parent↔child supervision exists elsewhere is unknown from this source.

---

Generated by dimension `15.02-message-routing-and-termination` against `openhands` (studies/agent-harness-study/sources/openhands).

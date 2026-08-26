# Source Analysis: openhands

## 07.01 Tool Scheduling and Dispatch

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (`@openhands/agent-canvas`) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19, Zustand, TanStack Query, socket.io-style native WebSockets, Vite (frontend-only repo; agent loop lives in the sibling `software-agent-sdk` Python server) |
| Analyzed | 2026-08-26 |

## Summary

This source is the OpenHands "Agent Canvas" **frontend** (`package.json:2-4`, confirmed by `AGENTS.md` "Repository Map": agent/tool behaviour belongs in the sibling `software-agent-sdk` repo). Consequently, tool scheduling and dispatch here is a **remote-server** architecture: the browser never runs agent tools itself. The client's dispatch role decomposes into four concrete mechanisms:

1. **Run triggering over a transport with fallback.** A user message is dispatched either through the conversation WebSocket with `run: true` (so "the agent loop starts automatically in async mode", `src/contexts/conversation-websocket-context.tsx:1131-1135`), or — when the socket is not OPEN — queued via REST `ConversationClient.sendEvent(..., { run: true })` for delivery when the conversation becomes ready (`src/contexts/conversation-websocket-context.tsx:1100-1120`). Actual tool execution then happens on the agent-server ("Tool calls (terminal, file_editor, browser, etc.) execute here" per the `<RUNTIME_SERVICES>` contract documented in `AGENTS.md`).

2. **Client-side execution of two registered "client tools".** `canvas_ui_control` and `launch_child_conversation` are advertised to the server at conversation start as `client_tools` (`src/api/agent-server-adapter.ts:1116-1119`). The server acknowledges them immediately; the work executes in the browser when the corresponding `ActionEvent` arrives on the WebSocket (`src/contexts/conversation-websocket-context.tsx:724-740`), dispatched to `handleCanvasUIAction` (`src/services/canvas-ui.ts:32-64`) or `handleLaunchChildConversationAction` (`src/services/child-conversation-launch.ts:505-536`). This is an inverted dispatch model: the *server* schedules, the *client* executes.

3. **A dedicated FIFO-queued bash channel** for UI-initiated commands (git probes): `useBashCommandRunner` holds three queues — waiting (pre-auth), pending (sent, awaiting `command_id` echo), and active (awaiting terminal output) — correlating results by server-assigned id (`src/hooks/use-bash-command-runner.ts:61-205`).

4. **Human-in-the-loop gating of dispatched work.** Tool calls can be held at `AWAITING_USER_CONFIRMATION`; confirmation policy is chosen at conversation start (`AlwaysConfirm` / `ConfirmRisky` with threshold, `src/api/agent-server-adapter.ts:600-604`) and released via REST `respond_to_confirmation` (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:38-58`, `src/hooks/mutation/use-respond-to-confirmation.ts:12-32`).

Ordering and observability of dispatch are handled on the receive path: streaming deltas are batched per animation frame but force-flushed before any non-delta event so durable events cannot overtake their own streamed text (`src/utils/streaming-delta-batcher.ts:30-35`, flush call at `src/contexts/conversation-websocket-context.tsx:549-554`); events are deduplicated by id and re-sorted by timestamp (`src/stores/use-event-store.ts:92-151`); every event carries `id`/`tool_name`/`tool_call_id`, and errors are forwarded to telemetry with those identifiers (`src/contexts/conversation-websocket-context.tsx:596-608`).

## Rating

**6 / 10** — Present and well-engineered on the client side, but the decisive scheduling machinery is out of this source's view.

Rationale against the rubric:
- The model that *is* visible is clear and explicit: transport selection with queueing fallback (`sendMessage` result type literally reports `{ queued }`, `src/contexts/conversation-websocket-context.tsx:88-90`), typed client-tool specs with MCP-style annotations (`src/api/canvas-ui-client-tool.ts:9-20`), FIFO bash queues, confirmation policy config.
- There are real tests guarding dispatch edge cases: reconnect-replay must not re-run non-idempotent side-effects (#1656) (`__tests__/contexts/conversation-websocket-context.test.tsx:396-548`), delta batcher ordering under thousands of sub-frame deltas (`__tests__/utils/streaming-delta-batcher.test.ts:145`), chronological sort/dedup on bulk add (`__tests__/stores/use-event-store.test.ts:127,148`).
- It stops short of 7-8 because: (a) the actual scheduler that turns validated LLM tool calls into running work lives in the sibling SDK and cannot be verified under this study's source-isolation rule; (b) several safeguards are best-effort (localStorage claim ledger tolerates replay risk when storage fails, `src/services/child-conversation-launch.ts:220-226`); (c) there is no tool batching and no dedicated dispatch tracing beyond the event stream.

## Evidence Collected

Every entry cites workspace-relative paths into `studies/agent-harness-study/sources/openhands/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Dispatcher (run trigger, WS-first) | `sendMessage` sends `{...message, run: true}` over the open socket | src/contexts/conversation-websocket-context.tsx:1131-1141 |
| Dispatcher (REST queue fallback) | Socket not OPEN → `ConversationClient.sendEvent(conversationId, {role:"user", content}, { run: true })`; returns `{ queued: true }` | src/contexts/conversation-websocket-context.tsx:1100-1120 |
| Remote-execution contract | Runtime-services block states tools execute inside the agent server sandbox | AGENTS.md (Runtime Services section); scripts/runtime-services-info.mjs |
| Client-tool registration | `client_tools: [CANVAS_UI_CLIENT_TOOL, LAUNCH_CHILD_CONVERSATION_CLIENT_TOOL]` only for `openhands` agent kind; schema-conflict warning comment | src/api/agent-server-adapter.ts:1111-1119 |
| Client-tool spec interface | `ClientToolSpec` with name/description/JSON-schema parameters + `readOnlyHint/destructiveHint/idempotentHint/openWorldHint` annotations | src/api/canvas-ui-client-tool.ts:9-20 |
| Client-tool executor (UI) | WS handler routes `isCanvasUIActionEvent` → `handleCanvasUIAction(action, conversationId)` | src/contexts/conversation-websocket-context.tsx:724-729 |
| Client-tool executor (launch) | WS handler routes `isLaunchChildConversationActionEvent` → async launch + result report-back | src/contexts/conversation-websocket-context.tsx:731-740 |
| Launch executor body | Validates params server-can't (enum gap), claims tool call, launches local/cloud child, never rejects | src/services/child-conversation-launch.ts:99-194, 505-536 |
| Idempotency ledger | `claimToolCall` localStorage ledger keyed by parent conversation; replay mid-flight dropped | src/services/child-conversation-launch.ts:196-227 |
| Worker queue (bash) | Three-stage FIFO: `waitingQueueRef` → `pendingQueueRef` → `activeCommandsRef` map keyed by server `command_id` | src/hooks/use-bash-command-runner.ts:66-72, 104-139 |
| Bash queue drain on connect | Queued commands flushed after auth handshake completes | src/hooks/use-bash-command-runner.ts:87-102 |
| Handshake safeguard | `startHandshakeWatchdog` aborts CONNECTING hang so it can't block the chat events socket | src/hooks/use-bash-command-runner.ts:82-85 |
| Confirmation gate (policy) | `AlwaysConfirm` default; `ConfirmRisky {threshold:"HIGH", confirm_unknown:true}` for LLM analyzer | src/api/agent-server-adapter.ts:600-604 |
| Confirmation gate (release) | Buttons find latest awaiting action, mark submitted id, POST `respond_to_confirmation` | src/components/shared/buttons/conversation-confirmation-buttons.tsx:30-58 |
| Confirmation REST route | Cloud vs local split for `/events/respond_to_confirmation` | src/api/event-service/event-service.api.ts:40-69 |
| Ordering — delta barrier | Deltas buffered; `flush()` before any non-delta event "so it can't overtake them" | src/contexts/conversation-websocket-context.tsx:548-554 |
| Batching (render-level) | rAF-coalesced batcher commits ≤1 store update/frame; merges adjacent deltas | src/utils/streaming-delta-batcher.ts:30-75 |
| Determinism — dedup + sort | Dedup by event id; `addEvents` re-sorts both `events` and `uiEvents` by timestamp | src/stores/use-event-store.ts:100-107, 159-208 |
| Replay safety | Duplicate detection before non-idempotent side-effects (#1656) | src/contexts/conversation-websocket-context.tsx:556-568 |
| Action→observation replacement | Observation replaces its action by `action_id`; ACP tool-call pairs merged by `tool_call_id` | src/utils/handle-event-for-ui.ts:404-441 |
| Execution-status state machine | `ExecutionStatus` (incl. `WAITING_FOR_CONFIRMATION`, `STUCK`) mapped to UI `AgentState` | src/hooks/use-agent-state.ts:10-35 |
| Scheduling traces (errors) | Errors tracked to telemetry with `toolName`/`toolCallId` metadata | src/contexts/conversation-websocket-context.tsx:596-608 |
| Tool-set selection config | Default toolset `terminal/file_editor/task_tracker`; browser gated by env + `/server_info usable_tools`; task set gated by `enable_sub_agents` | src/api/agent-server-adapter.ts:113-115, 631-659 |
| Optimistic send watchdog | Pending message flips to error after 150 s if no server echo | src/stores/optimistic-user-message-store.ts:14, 131-139 |
| Tests — replay/idempotency | "reconnect replay does not re-run non-idempotent side-effects" incl. terminal input/output | __tests__/contexts/conversation-websocket-context.test.tsx:396-548 |
| Tests — batch ordering | Byte-for-byte order across thousands of 1-char deltas faster than 60 Hz | __tests__/utils/streaming-delta-batcher.test.ts:145 |
| Tests — store determinism | Bulk add sorts chronologically; de-duplicates; delta compaction per sender | __tests__/stores/use-event-store.test.ts:127-242 |

## Answers to Dimension Questions

**1. How does a tool call start?**
Two paths. Agent tools: the LLM decides server-side; the client only triggers a run by sending a user message with `run: true` (WS path `src/contexts/conversation-websocket-context.tsx:1131-1135`; REST path `src/api/conversation-service/agent-server-conversation-service.api.ts:381-400`), or releases a held call via `respond_to_confirmation` (`src/hooks/mutation/use-respond-to-confirmation.ts:12-32`). Client tools: the server emits an `ActionEvent`; the browser executes it on receipt (`src/contexts/conversation-websocket-context.tsx:727-740`).

**2. Is tool execution inline or queued?**
Remote-server execution for agent tools — never inline in the browser. On the client there are three queueing mechanisms: the REST send-event queue used when the WebSocket is down (`{ queued: true }`, `src/contexts/conversation-websocket-context.tsx:1100-1120`); the bash runner's FIFO queues buffering commands until auth handshake completes (`src/hooks/use-bash-command-runner.ts:189-200`); and the optimistic pending-message store holding user bubbles until the echoed event confirms them (`src/stores/optimistic-user-message-store.ts:115-142`). Client tools execute inline in the WS message handler (the launch tool does long-running async work detached via `void`, `src/contexts/conversation-websocket-context.tsx:734-740`).

**3. Are tool calls ordered?**
Yes, enforced defensively rather than trusted from the wire. Server order is preserved for durable events (flush-before-enqueue barrier, `src/contexts/conversation-websocket-context.tsx:553-554`); out-of-order arrivals self-heal via timestamp re-sort (`src/stores/use-event-store.ts:137-151`) and observation-for-action replacement keyed by `action_id` regardless of arrival position (`src/utils/handle-event-for-ui.ts:432-441`). Echo matching for optimistic bubbles explicitly assumes out-of-order delivery is possible and matches by content, not order (`src/stores/optimistic-user-message-store.ts:169-198`). Bash command echoes are paired strictly FIFO with the oldest outstanding request (`src/hooks/use-bash-command-runner.ts:112-121`). Caveat: timestamp sort uses lexicographic ISO comparison (`src/stores/use-event-store.ts:33-34`), which breaks across mixed timezone offsets.

**4. Can tools be batched?**
No evidence of tool-call batching anywhere in this source. `getAgentTools` builds a name→spec map and sends a flat list at conversation start (`src/api/agent-server-adapter.ts:646-677`); messages carry one content payload each. The only batching found is render-level coalescing of streaming deltas (`src/utils/streaming-delta-batcher.ts:30-38`) — a UI throughput optimization, not tool scheduling. Parallelism exists only at the delegation level: `launch_child_conversation` spawns independent conversations ("parallel children do not fight over the same files", tool description at `src/api/launch-child-conversation-client-tool.ts:36-38`).

**5. Is dispatch observable?**
Partially. Every dispatched unit of work is an event with stable `id`, `tool_name`, `tool_call_id`, `timestamp` (type fields at `src/types/agent-server/core/events/action-event.ts:35-40`), persisted server-side and replayable via REST search with pagination/sort/filters (`src/api/event-service/event-service.api.ts:102-181`). Errors flow to telemetry tagged with `eventId`, `toolName`, `toolCallId` and classification (`src/contexts/conversation-websocket-context.tsx:596-608`). But there is no dedicated dispatch trace/span concept (no timing of enqueue→execute, no queue-depth metrics); the answer to "why did this tool run now" must be reconstructed from the event log.

## Architectural Decisions

- **Remote-server scheduling with a thin trigger surface.** The frontend deliberately owns no agent-loop state; its dispatch API is "send message with `run: true`" plus status observation (`ExecutionStatus` mapping at `src/hooks/use-agent-state.ts:10-35`). This trades local control for a single authoritative scheduler.
- **Inverted client tools.** Registering JSON-schema tool specs at conversation start (`src/api/agent-server-adapter.ts:1116-1119`) lets server-side agents invoke browser capabilities without new server endpoints. The cost is acknowledged in-code: the server acks before work happens, so results travel out-of-band as a follow-up user message prefixed `CHILD_CONVERSATION_RESULT_PREFIX` (`src/services/child-conversation-launch.ts:451-497`).
- **Transport degradation instead of failure.** WS-down does not block dispatch; it switches to a queued REST path whose semantics differ (no optimistic bubble, `queued: true` tells the caller to skip it, `src/contexts/conversation-websocket-context.tsx:1118-1120`).
- **Idempotency handled at the consumer, not the transport.** Reconnects replay backlogs (`resend_mode='since'/'all'`, `src/contexts/conversation-websocket-context.tsx:964-1002`), so each non-idempotent side-effect guards itself: dedup check before side-effects (#1656, `src/contexts/conversation-websocket-context.tsx:556-568`), submitted-event-id set for confirmations (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:44-47`), localStorage claim ledger for launches (`src/services/child-conversation-launch.ts:199-227`).
- **Safety gating moved to conversation configuration.** Instead of a runtime permission engine in the client, dispatch risk is configured up front via `confirmation_policy` and `security_analyzer` in the start payload (`src/api/agent-server-adapter.ts:593-618, 1120-1121`).

## Notable Patterns

- **Barrier-then-commit streaming**: buffered deltas are always flushed before a durable event commits, preserving causal display order without giving up frame batching (`src/contexts/conversation-websocket-context.tsx:548-554`).
- **Three-phase command correlation** (waiting → pending → active) with distinct failure semantics per phase, and wholesale rejection on socket death (`rejectAll`, `src/hooks/use-bash-command-runner.ts:141-149`).
- **Claim-before-work ledger** for exactly-once-ish launches, claimed *before* network I/O so even an in-flight replay is dropped (`src/services/child-conversation-launch.ts:202-204`).
- **Corrective-feedback errors**: client-side validation failures are returned to the agent as structured guidance text rather than thrown, because the server already reported success (`src/services/child-conversation-launch.ts:99-109, 499-504`).
- **Capability-gated toolsets**: tools are included/excluded from the start payload based on env flags, `/server_info.usable_tools`, and settings (`browser_toolsEnabled() && isAgentServerToolAvailable(name)`, `src/api/agent-server-adapter.ts:631-643`).

## Tradeoffs

- **Remote dispatch = zero client visibility into scheduling internals.** Queue depth, worker assignment, and retry policy on the server are invisible; the UI can only show coarse `ExecutionStatus`. Debugging "why did my command wait" requires server logs outside this source.
- **Client-tool ack-before-execute** makes the agent's view of success diverge from reality until the follow-up message arrives; the design compensates with the result-prefix protocol but accepts a window where the agent may act on stale assumptions (`src/api/launch-child-conversation-client-tool.ts:54-59`).
- **localStorage-based idempotency** is scoped per-browser: clearing site data re-arms replay risk for billable cloud launches, and storage-full silently proceeds (`src/services/child-conversation-launch.ts:220-226`).
- **Frame-batched deltas** add a codepath where transient and durable events take different commit routes; correctness depends on every non-delta handler remembering to flush first (currently done in both handlers, `src/contexts/conversation-websocket-context.tsx:550-554, 781-786`).
- **Lexicographic timestamp sort** is cheap and usually right, but cross-timezone or clock-skewed server timestamps would misorder history pages.

## Failure Modes / Edge Cases

- **Replayed events re-running side-effects** — mitigated by id dedup + per-feature guards; regression-tested (`__tests__/contexts/conversation-websocket-context.test.tsx:512-548`).
- **Hung bash handshake blocking the chat socket** — bounded by `startHandshakeWatchdog` (`src/hooks/use-bash-command-runner.ts:82-85`).
- **Echo never arrives (WS drop post-send)** — optimistic bubble flips to retriable error after 150 s watchdog (`src/stores/optimistic-user-message-store.ts:131-139`).
- **Cloud child provisioning hangs** — bounded poll (3 s interval, 180 s ceiling) returns the still-provisioning task instead of hanging (`src/services/child-conversation-launch.ts:36-38, 365-384`).
- **Worktree creation failure on scratch workspaces** — automatic downgrade to `shared` isolation with an explanatory note carried back to the agent (`src/services/child-conversation-launch.ts:308-323`).
- **Malformed frames** — bash runner ignores unparseable messages; WS handler warns and continues (`src/hooks/use-bash-command-runner.ts:108-110`, `src/contexts/conversation-websocket-context.tsx:742-744`).
- **Unknown canvas tab names** — logged to console rather than silently ignored, keeping dispatch failures diagnosable (`src/services/canvas-ui.ts:54-61`).

## Future Considerations

- Promote the launch claim-ledger from localStorage to a server-side idempotency key so replay safety survives browser state loss.
- Add dispatch-level telemetry spans (enqueue time, ack time, execution start) alongside the existing error tracking to make "why did this tool run now" answerable from telemetry rather than event archaeology.
- Unify the two WS handlers (main/planning), which currently duplicate the entire dispatch switch (`src/contexts/conversation-websocket-context.tsx:537-961`) — divergence between them is a standing drift risk (already noted in-code at lines 649-651).
- Expose queue depth/health of the bash-events FIFO so UI-initiated probes degrade visibly under load.

## Questions / Gaps

- **Where is the actual scheduler?** How the agent-server serializes LLM tool calls into executors (worker pool? asyncio tasks? subprocess pool?) is defined in the sibling `software-agent-sdk` repo, which source isolation forbids inspecting. No evidence found within this source beyond the remote-execution contract in `AGENTS.md`.
- **No tool batching evidence.** Searched `src/api`, `src/hooks`, `src/contexts`, `src/stores` for batch/group submission of multiple tool calls; only render-level delta batching exists (`src/utils/streaming-delta-batcher.ts`). Whether the server batches parallel tool calls cannot be determined here.
- **Server-side ordering guarantees** (does the events socket preserve emission order?) are assumed, not proven; the client's defensive sorting suggests they are not fully trusted, but no spec/test in this source pins the server behavior.
- **`SendMessageRequest` has no client-supplied correlation id** (`src/api/conversation-service/agent-server-conversation-service.types.ts:60-63`); echo matching relies on content equality, which would misattribute two identical concurrent messages despite the FIFO fallback.

---

Generated by `dimensions/07.01-tool-scheduling-and-dispatch.md` against `openhands`.

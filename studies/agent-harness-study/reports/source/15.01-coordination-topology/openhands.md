# Source Analysis: openhands

## 15.01 Coordination Topology

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19 (React Router 7, Zustand, TanStack Query, WebSocket) — the "Agent Canvas" frontend of the OpenHands multi-repo system (see `AGENTS.md`, repo map table) |
| Analyzed | 2026-08-25 |

## Summary

This source is the **frontend** of the OpenHands system, so its coordination topology is a *UI-mediated, server-brokered hub-and-spoke* model rather than an in-process agent mesh. Three distinct coordination mechanisms coexist:

1. **Main conversation + singleton planning sub-conversation (static supervisor–worker).** `ConversationWebSocketProvider` holds two parallel WebSocket connections per conversation view — one for the main conversation and one for the planning sub-conversation (`src/contexts/conversation-websocket-context.tsx:136-141`, `src/contexts/conversation-websocket-context.tsx:402-421`). The planning sub-conversation is created on demand as a linked child with `agentType: "plan"` (`src/hooks/use-handle-plan-click.ts:62-83`) and is explicitly a singleton: *"Currently, there is only one sub-conversation and it uses the planning agent"* (`src/contexts/conversation-websocket-context.tsx:407`).

2. **Dynamic parent→child delegation via the `launch_child_conversation` client tool.** The agent can spawn independent child conversations on `local` (git-worktree isolated) or `cloud` (separate sandbox) targets (`src/api/launch-child-conversation-client-tool.ts:61-112`). Children are deliberately *disconnected* peers: "It does not block you, you do not see its output, and it cannot see this conversation's history" (`src/api/launch-child-conversation-client-tool.ts:16-18`). The only return channel is a machine-prefixed `[child-conversation] ` message injected back into the parent thread (`src/constants/child-conversation.ts:20`, `src/services/child-conversation-launch.ts:488-496`).

3. **In-loop subagent delegation via `task_tool_set` (server-side TaskAction/TaskObservation).** Gated client-side by the `enable_sub_agents` setting (`src/api/agent-server-adapter.ts:636-641`) and rendered as query/result cards showing `subagent_type`, `task_id` and the subagent's returned answer (`src/components/features/chat/tool-visualizers/task/task.tsx:49-85`).

All traffic converges on the agent-server as broker; agents never talk to each other directly. Discovery is a static server-persisted field: `AppConversation.sub_conversation_ids` (`src/api/conversation-service/agent-server-conversation-service.types.ts:214`) fetched through `useSubConversations` → `batchGetAppConversations` (`src/hooks/query/use-sub-conversations.ts:33-47`).

**Topology sketch of one multi-agent run:**

```
                         ┌────────────────────────────┐
   user input ──────────▶│  Agent Server (broker/SPOF) │
                         └───────────┬────────────────┘
              resend_mode='since' WS │      ▲ sendEvent/run (WS or REST queue)
                                     ▼      │
        ┌──────────────── main conversation event stream ─────────────────┐
        │                       (browser coordinator)                     │
        │  ConversationWebSocketProvider + shared Zustand event store     │
        └──────┬──────────────────────────────────┬───────────────────────┘
               │ planning WS (resend_all)         │ client-tool interception
               ▼                                  ▼
   [planning sub-conversation]          launch_child_conversation
   singleton, agentType="plan"          ├── local child (worktree/shared)
   flagged isFromPlanningAgent          └── cloud child (own sandbox,
   merged into same UI stream               no server-side parent link)
                                        result → "[child-conversation]" msg
                                        posted back into parent thread
```

## Rating

**7 / 10.** Clear, well-documented coordination model with dedicated tests and unusually strong operational safeguards for a browser-side coordinator: a localStorage idempotency ledger against tool-call replays (`src/services/child-conversation-launch.ts:205-227`), worktree-isolation fallback logic with explicit conflict warnings (`src/services/child-conversation-launch.ts:300-323`), version-gated parent links (`src/constants/child-conversation.ts:30-37`), per-stream delta batching to prevent cross-agent stream merging (`src/contexts/conversation-websocket-context.tsx:162-180`), and replay dedup keyed by event id (`src/contexts/conversation-websocket-context.tsx:557-568`). It falls short of 8+ because the topology is hardcoded in places (planning sub-conversation pinned to index `[0]` at `src/contexts/conversation-websocket-context.tsx:408`, `858-861`, `908-909`), there is no live observability of child-conversation state beyond a toast URL (`src/services/child-conversation-launch.ts:469-479`), and the planning socket still runs the legacy `resend_all` history mode that the main socket migrated away from (`src/contexts/conversation-websocket-context.tsx:1004-1008`; AGENTS.md confirms it is pending migration).

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Dual-WebSocket provider (main + planning) | Separate connection states `mainConnectionState` / `planningConnectionState` merged into one UI status | `src/contexts/conversation-websocket-context.tsx:136-141`, `424-461` |
| Static singleton planning sub-conversation | Comment "Currently, there is only one sub-conversation and it uses the planning agent"; `subConversations[0]` selected | `src/contexts/conversation-websocket-context.tsx:402-421` |
| Planning sub-conversation creation | `createConversation({ parentConversationId, agentType: "plan", entryPoint: "plan_sub_conversation" })`, guarded to run once per parent | `src/hooks/use-handle-plan-click.ts:42-93` |
| Parent link persisted server-side | `parent_conversation_id` sent on start payload (local path); SDK feature gate `MIN_AGENT_SERVER_VERSION_FOR_PARENT_LINK = "1.37.1"` | `src/api/agent-server-adapter.ts:1165-1167`; `src/constants/child-conversation.ts:30-37` |
| Server-side discovery field | `sub_conversation_ids: string[]` on `AppConversation` | `src/api/conversation-service/agent-server-conversation-service.types.ts:214` |
| Sub-conversation discovery hook | `useSubConversations(ids)` → `batchGetAppConversations`, backend-keyed cache | `src/hooks/query/use-sub-conversations.ts:33-52` |
| Delegation tool spec | `LAUNCH_CHILD_CONVERSATION_CLIENT_TOOL` with `target`/`task`/`isolation` schema and non-idempotent annotations | `src/api/launch-child-conversation-client-tool.ts:61-112` |
| Child independence contract | "The child runs on its own… you do not see its output… cannot see this conversation's history" | `src/api/launch-child-conversation-client-tool.ts:14-38` |
| Result return channel | `[CHILD_CONVERSATION_RESULT_PREFIX = "[child-conversation] "]` message posted into parent via `sendMessage` | `src/constants/child-conversation.ts:11-20`; `src/services/child-conversation-launch.ts:459-497` |
| Client-tool dispatch over WS | `isLaunchChildConversationActionEvent(event)` → `handleLaunchChildConversationAction(action, conversationId, toolCallId)` inside the main WS handler | `src/contexts/conversation-websocket-context.tsx:734-740` |
| Launch execution + isolation | Local children request the parent's working dir; `worktree` default with `shared` fallback and explicit conflict note | `src/services/child-conversation-launch.ts:272-350`, `265-270` |
| Replay/idempotency safeguard | `claimToolCall` localStorage ledger drops replayed tool calls before any network work | `src/services/child-conversation-launch.ts:196-227` |
| In-loop subagents gating | `task_tool_set` attached only when `agentSettings.enable_sub_agents === true` | `src/api/agent-server-adapter.ts:115`, `636-641` |
| Subagent task visualization | Task visualizer renders `subagent_type`, `task_id`, parent query, subagent result | `src/components/features/chat/tool-visualizers/task/task.tsx:41-85` |
| Sender discrimination across streams | `isFromPlanningAgent` flag applied to every planning-socket event; separate delta batchers per socket (#1656) | `src/contexts/conversation-websocket-context.tsx:172-180`, `793-798`; `src/utils/handle-event-for-ui.ts:31-37` |
| Communication channel fallback | `sendMessage` sends over WS when open, otherwise queues via REST `sendEvent(..., { run: true })` | `src/contexts/conversation-websocket-context.tsx:1094-1144` |
| Reconnect safeguards | Exponential backoff reconnect with max-attempt cap per socket instance | `src/hooks/use-websocket.ts:110-134` |
| Cloud child has no server link | `parent_conversation_id: null` for cloud launches; `parent_link_note` explains why | `src/services/child-conversation-launch.ts:416-419`, `444-448` |
| Async provisioning polling | `waitForCloudConversationId` polls start task (3 s interval, 180 s bound); UI-side `useSubConversationTaskPolling` polls every 3 s until READY/ERROR | `src/services/child-conversation-launch.ts:365-384`; `src/hooks/query/use-sub-conversation-task-polling.ts:24-55` |
| Hidden machine messages | Chat suppresses goal-loop re-prompts and child-launch result payloads from rendering | `src/components/conversation-events/chat/event-content-helpers/should-render-event.ts:105-113` |
| Behavioral test coverage | ~20 cases covering validation, local/cloud targets, isolation fallbacks, replay, goal-loop interaction | `__tests__/services/child-conversation-launch.test.ts:122-560` |

## Answers to Dimension Questions

**1. How do agents coordinate?**
They don't coordinate peer-to-peer at all. Coordination is mediated: (a) the *agent-server* brokers all events over per-conversation WebSockets (`src/utils/websocket-url.ts` builds the URL; handler wiring at `src/contexts/conversation-websocket-context.tsx:1069-1075`); (b) the *browser* acts as executor for client-defined tools — it intercepts `launch_child_conversation` ActionEvents mid-stream and performs the launch on the agent's behalf (`src/contexts/conversation-websocket-context.tsx:731-740`), then feeds the outcome back into the parent loop as a synthetic user message so the LLM relay learns the child's id/URL (`src/services/child-conversation-launch.ts:451-497`). Planning-agent content is merged into the same UI event stream but tagged `isFromPlanningAgent` to keep senders distinct (`src/utils/handle-event-for-ui.ts:31-37`). Plan-mode chat input routes messages to the planning socket instead of the main one (`src/contexts/conversation-websocket-context.tsx:1094-1099`, mode from `src/stores/conversation-store.ts:17`).

**2. Is the topology fixed or dynamic?**
Mixed. The *shape* is fixed: exactly one optional planning sub-conversation per parent (singleton guard at `src/hooks/use-handle-plan-click.ts:50-59`, hardcoded `subConversations[0]` at `src/contexts/conversation-websocket-context.tsx:407-408`), plus an unbounded but practically shallow fan-out of independent child conversations created at runtime (`launch_child_conversation`, dynamic). Children cannot themselves be observed or managed by the parent after launch — the relationship is fire-and-forget except for the single result message. Depth is implicitly bounded: cloud children get no parent link at all (`src/services/child-conversation-launch.ts:416-419`), so deep chains lose lineage server-side.

**3. Is there a single point of failure?**
Yes — the agent-server. Every conversation's events flow through it; if it is unreachable, both sockets error and the UI surfaces a connection banner only after a previously successful connection (`src/contexts/conversation-websocket-context.tsx:987-993`, `1049-1055`). The frontend mitigates transient loss (exponential-backoff reconnect, `src/hooks/use-websocket.ts:110-134`) but has no alternate transport beyond the REST message-queue fallback for *sending* (`src/contexts/conversation-websocket-context.tsx:1100-1129`). A secondary fragility: the launch ledger lives in `window.localStorage`, and a full/corrupt store silently accepts replay risk (`src/services/child-conversation-launch.ts:214-226`).

**4. Can agents discover each other?**
Not autonomously. Discovery is structural and human/LLM-mediated: the server persists `sub_conversation_ids` on the parent (`src/api/conversation-service/agent-server-conversation-service.types.ts:214`) which the UI resolves via batch fetch (`src/hooks/query/use-sub-conversations.ts:33-52`); the launching agent learns a child's id only from the `[child-conversation]` result message it relays (`src/services/child-conversation-launch.ts:213-234` test asserts id/URL reporting). A delegated child is told nothing about its siblings or parent state — the tool description mandates fully self-contained briefs precisely because there is no inter-agent visibility (`src/api/launch-child-conversation-client-tool.ts:32-38`). No registry, capability advertisement, or peer lookup exists anywhere in this source.

## Architectural Decisions

- **Server-brokered star, not agent mesh.** All events traverse the agent-server; the frontend never routes agent↔agent traffic. Evidenced by the dual-socket provider being the only event ingress (`src/contexts/conversation-websocket-context.tsx:1069-1075`) and the API access rules confining all calls to typed clients (`AGENTS.md`, "API Access Rules").
- **Delegation = new conversation, not shared context.** Children are independent conversations with their own sandbox/worktree, chosen so parallel children "do not fight over the same files" (`src/api/launch-child-conversation-client-tool.ts:36-37`); isolation defaults to `worktree` (`src/services/child-conversation-launch.ts:191`).
- **Client-side tool execution for UI-owned capabilities.** The launch tool is acknowledged by the server before the browser does any work; the outcome travels back as a message (`src/services/child-conversation-launch.ts:451-457`). Same pattern as `canvas_ui` actions (`src/types/agent-server/core/base/action.ts:315-325`).
- **One shared event store, sender-tagged.** Rather than separate stores per agent, planning events are flagged and every merge/dedup helper is sender-scoped to prevent cross-agent contamination (regression #1656 documented inline at `src/utils/handle-event-for-ui.ts:31-37`, `78-98`, `120-134`).
- **Version-gated topology features.** Parent-link persistence requires agent-server ≥ 1.37.1; older servers produce an honest `parent_link: false` report instead of silent divergence (`src/constants/child-conversation.ts:30-37`; `src/services/child-conversation-launch.ts:236-250`).

## Notable Patterns

- **ClientToolSpec + generated action kind**: SDK naming convention `ClientAction_<tool>` contained in one constant module (`src/constants/child-conversation.ts:3-9`).
- **Claim-before-act idempotency ledger**: tool call ids are recorded in localStorage before any network side effect so socket-reconnect replays (`resend_mode: "all"`) or REST/WS races cannot double-launch billable cloud conversations (`src/services/child-conversation-launch.ts:196-227`).
- **Corrective-guidance error envelope**: every malformed launch becomes actionable guidance text for the LLM instead of a thrown error, because the server already ACK'd the call (`src/services/child-conversation-launch.ts:85-89`, `499-504`).
- **Bounded async provisioning**: cloud child ids are resolved by polling the start task with hard timeouts (3 s × 180 s) rather than hanging (`src/services/child-conversation-launch.ts:36-38`, `365-384`).
- **Graceful degradation ladder**: worktree → shared-with-warning → reported failure (`src/services/child-conversation-launch.ts:303-323`).
- **ACP runtime swap (adjacent)**: alternative agent runtimes (Claude Code, Codex, Gemini CLI via Agent Client Protocol) replace the agent behind a conversation rather than joining a team (`src/constants/acp-providers.ts:1-15`; running-card lifecycle at `src/components/conversation-events/chat/event-content-helpers/should-render-event.ts:126-140`).

## Tradeoffs

- **Independence vs. observability**: children can't collide on files, but the parent gets only a one-shot JSON result; there is no way to watch, cancel, or join a child from the UI (no evidence found — searched `src/components/features/conversation-panel/` and hooks for child-status polling; only start-task polling exists, `src/hooks/query/use-sub-conversation-task-polling.ts:24-55`).
- **Shared event store vs. stream safety**: merging planning events into the main stream simplifies rendering but forces sender-scoped logic in every reconciliation path; three separate code comments cite bug #1656 as the cost of getting this wrong (`src/utils/handle-event-for-ui.ts:31-37`, `82-83`, `133-134`).
- **Client-executed tools gain UI leverage but lose atomicity**: the server ACKs before work happens, so failures must round-trip through natural-language guidance rather than structured tool errors (`src/services/child-conversation-launch.ts:99-109`, `499-504`).
- **localStorage ledger**: survives page refreshes without a server round-trip, but is origin-scoped, capacity-limited, and best-effort (`src/services/child-conversation-launch.ts:220-225`).

## Failure Modes / Edge Cases

- **Replayed tool calls**: guarded by the claim ledger; covered by tests asserting a replay is ignored (`src/services/child-conversation-launch.ts:205-227`; verified at `__tests__/services/child-conversation-launch.test.ts:490-508`).
- **Scratch-directory parents**: a workspace with unborn HEAD cannot host a worktree; the launch pre-detects this via stored metadata and downgrades to `shared`, warning that "the two agents may conflict over the same files" (`src/services/child-conversation-launch.ts:252-270`, `299-306`).
- **Worktree creation race**: even when metadata looks fine, worktree creation failure falls back to `shared` once, then reports the original error (`src/services/child-conversation-launch.ts:308-323`).
- **Goal-loop interference**: posting the result message would cancel an active `/goal` loop, so reporting is skipped while one is active (`src/services/child-conversation-launch.ts:480-486`; tested at `__tests__/services/child-conversation-launch.test.ts:510-533`).
- **Cross-backend cache bleed**: sub-conversation queries are backend-keyed so a `null` fetched while a cloud backend was active cannot leak into local visits (`src/hooks/query/use-sub-conversations.ts:37-44`).
- **Stale cloud URLs while PAUSED**: the provider suppresses the conversation URL during sandbox pause to avoid connecting to a dead host (`src/contexts/websocket-provider-wrapper.tsx:24-33`).
- **Enum gap in server validation**: the SDK drops `enum` from the pydantic model, so bad `target`/`isolation` values reach the browser and must be rejected client-side (`src/services/child-conversation-launch.ts:99-124`, `173-181`).

## Future Considerations

- Generalize the planning socket: replace the `subConversations[0]` assumption with per-sub-conversation sockets/handlers so N workers become possible (`src/contexts/conversation-websocket-context.tsx:402-421`).
- Migrate planning history to the REST-first `resend_mode='since'` pattern already used by the main socket (`src/contexts/conversation-websocket-context.tsx:1004-1008`; AGENTS.md notes the migration is pending).
- Add child-conversation lifecycle observability (status polling, cancel, relink) — currently only the launch moment is observable.
- Move the launch ledger server-side or into IndexedDB to shed localStorage limits and multi-tab hazards.
- Close the lineage gap where cloud children carry no parent link, breaking recursive topology reconstruction.

## Questions / Gaps

- **Where does the actual subagent loop live?** The in-loop `TaskAction`/`TaskObservation` pair and `task_tool_set` semantics belong to the `software-agent-sdk` repo (per `AGENTS.md` repo map); this source shows only the client-side gate (`src/api/agent-server-adapter.ts:636-641`) and rendering (`src/components/features/chat/tool-visualizers/task/task.tsx:49-85`). Whether subagents run concurrently or sequentially could not be determined here — no evidence found in this directory.
- **Is there any cancellation path for a launched child?** Searched for `cancel`/`pause`/`stop` in `src/services/child-conversation-launch.ts`, the conversation service, and the conversation panel: none targets child conversations specifically. No evidence found of child management post-launch.
- **Multi-tab double-launch risk**: the ledger is per-origin localStorage shared across tabs, so two tabs processing the same replayed event appear safe (claim check), but concurrent claims racing between tabs were not tested — `__tests__/services/child-conversation-launch.test.ts` exercises sequential replays only.
- **Planning-agent ↔ main-agent data flow**: the plan content reaches the UI via `readConversationFile` on `Plan.md` observations (`src/contexts/conversation-websocket-context.tsx:904-938`), but how the main agent consumes the plan is server-side and out of this source's boundary.

---

Generated by `Dimension 15.01: Coordination Topology` against `openhands`.

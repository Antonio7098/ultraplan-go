# Source Analysis: openhands

## Dimension 04.08: Agent-as-Tool and Workflow-as-Tool Composition

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19, React Router 7, Vite, TanStack Query (`package.json:1-30`); the "agent-canvas" frontend of the OpenHands multi-repo system (backend agent/tool logic lives in the sibling `software-agent-sdk` repo, per `AGENTS.md` "Repository Map") |
| Analyzed | 2026-08-23 |

## Summary

This repository is the OpenHands **frontend** ("agent-canvas"), so composition is studied at the layer where conversations are configured, client tools are registered, and nested-run events are rendered. Three distinct agent-composition mechanisms are implemented or surfaced here:

1. **Server-side subagent delegation via the `task_tool_set`** — the frontend gates whether new conversations receive the SDK's `task` tool based on the `enable_sub_agents` setting (`src/api/agent-server-adapter.ts:631-644`, default off in `src/services/settings.ts:50`), and renders the resulting `TaskAction`/`TaskObservation` event pair with a dedicated visualizer (`src/components/features/chat/tool-visualizers/task/task.tsx:49-85`). The actual subagent execution lives server-side; the wire contract (`prompt`, `subagent_type`, `resume` → `task_id`, `subagent`, `status`) is fully typed in `src/types/agent-server/core/base/action.ts:282-299` and `src/types/agent-server/core/base/observation.ts:305-326`.

2. **A fully frontend-implemented "agent as tool": the `launch_child_conversation` client tool** — a JSON-schema tool spec sent to the agent-server per conversation (`src/api/launch-child-conversation-client-tool.ts:61-112`, attached in `src/api/agent-server-adapter.ts:1116-1119`). When the LLM calls it, the browser intercepts the action over WebSocket (`src/contexts/conversation-websocket-context.tsx:734-740`), validates parameters the server cannot check (`src/services/child-conversation-launch.ts:110-194`), creates a real child conversation locally (git-worktree isolated) or on OpenHands Cloud (`child-conversation-launch.ts:272-449`), and posts a structured JSON result back into the parent conversation so the agent learns the child's id and URL (`child-conversation-launch.ts:459-497`).

3. **UI-initiated nested runs** — the planning-agent sub-conversation is created by the UI with `parentConversationId` + `agentType: "plan"` (`src/hooks/use-handle-plan-click.ts:62-67`) and its events stream into the same chat tagged `isFromPlanningAgent` (`src/contexts/conversation-websocket-context.tsx:793-798`). ACP agents (Claude Code, Codex, …) run as subprocesses whose inner tool calls surface as `ACPToolCallEvent`s keyed by `tool_call_id` (`src/types/agent-server/core/events/acp-tool-call-event.ts:38-96`).

The model is unusually explicit for a frontend: typed input/output contracts, corrective-guidance error envelopes instead of thrown errors, replay deduplication of non-idempotent launches, and version-gated parent links. The main gaps are structural: **no recursion depth guard** (children receive the same launch tool), **no cross-conversation cost attribution**, and no child trace IDs beyond the parent-link conversation id.

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale: the `launch_child_conversation` mechanism is a well-engineered agent-as-tool with a documented JSON-schema contract (`src/api/launch-child-conversation-client-tool.ts:14-59`), exhaustive behavioral tests including failure and replay paths (`__tests__/services/child-conversation-launch.test.ts:122-533`), idempotency ledgering against socket replays (`src/services/child-conversation-launch.ts:196-227`), bounded cloud polling (180 s ceiling, `child-conversation-launch.ts:37-38`), and graceful isolation fallbacks (`child-conversation-launch.ts:300-323`). It stops short of 8–10 because recursion is prevented only by prompt advice, not code; child costs are invisible to the parent; and nested execution has no trace identifiers beyond the parent-link field, which is silently absent below agent-server 1.37.1 (`src/constants/child-conversation.ts:30-37`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Subagent delegation tool gating | `TASK_TOOL_SET_NAME = "task_tool_set"` constant; `shouldIncludeTool()` attaches it only when `agentSettings.enable_sub_agents === true` and the server advertises it | `src/api/agent-server-adapter.ts:115`, `src/api/agent-server-adapter.ts:636-641` |
| Sub-agents disabled by default | `enable_sub_agents: false` in `DEFAULT_SETTINGS.agent_settings` | `src/services/settings.ts:50` |
| Settings toggle UI | `ENABLE_SUB_AGENTS_FIELD_KEY = "enable_sub_agents"` found dynamically inside `agent_settings_schema` sections | `src/routes/agent-settings.tsx:47`, `src/routes/agent-settings.tsx:71-101` |
| TaskAction input contract | `prompt`, `subagent_type`, optional `description`, optional `resume` ("Task id to resume from, when continuing an existing subagent task") | `src/types/agent-server/core/base/action.ts:282-299` |
| TaskObservation result contract | `content`, `is_error`, `task_id`, `subagent`, lifecycle `status` | `src/types/agent-server/core/base/observation.ts:305-326` |
| Event union membership | `"TaskAction"` documented as "The `task` tool delegating work to a spawned subagent"; `"TaskObservation"` as its result | `src/types/agent-server/core/base/base.ts:39-40`, `src/types/agent-server/core/base/base.ts:49-50` |
| Task result rendering | `taskVisualizer` renders subagent name, task id, parent query, and markdown result with error styling | `src/components/features/chat/tool-visualizers/task/task.tsx:49-85` |
| Task transcript export | `getTaskActionDetails` / `getTaskObservationDetails` serialize subagent, query, task_id, result | `src/utils/transcript-export/index.ts:201-223` |
| Client-tool spec interface | `ClientToolSpec { name, description, parameters, annotations }` (MCP-style hint annotations) | `src/api/canvas-ui-client-tool.ts:9-20` |
| Launch tool JSON schema | Enumerated `target` ("local"/"cloud") and `isolation` ("worktree"/"shared"); required `["target", "task"]`; `idempotentHint: false`, `openWorldHint: true` | `src/api/launch-child-conversation-client-tool.ts:61-112` |
| Launch tool prompt guidance | "One call per delegated task. Do NOT call this tool twice for the same task."; child "does not block you... cannot see this conversation's history" | `src/api/launch-child-conversation-client-tool.ts:38`, `:16-18` |
| Client tools attached to conversations | `client_tools: [CANVAS_UI_CLIENT_TOOL, LAUNCH_CHILD_CONVERSATION_CLIENT_TOOL]` only for OpenHands-kind agents; ACP gets `[]` | `src/api/agent-server-adapter.ts:1116-1119` |
| Server schema-conflict caveat | Agent-server caches client tool schemas per name and rejects re-registration with a different schema (`ClientToolSchemaConflictError`) | `src/api/agent-server-adapter.ts:1111-1115` |
| WebSocket interception point | `handleLaunchChildConversationAction(event.action, conversationId, event.tool_call_id)` dispatched from main WS handler | `src/contexts/conversation-websocket-context.tsx:734-740` |
| Action discriminator + type guard | `LAUNCH_CHILD_CONVERSATION_ACTION_KIND = "ClientAction_launch_child_conversation"`; guard discriminates on `tool_name` | `src/constants/child-conversation.ts:8-9`, `src/types/agent-server/type-guards.ts:196-200` |
| Typed action shape | `LaunchChildConversationAction { target, task, title?, repository?, branch?, isolation? }` | `src/types/agent-server/core/base/action.ts:332-341` |
| Client-side validation rationale | SDK drops `enum` when building the pydantic model, so bad `target`/`isolation` values reach the browser and must be rejected there | `src/services/child-conversation-launch.ts:99-109` |
| Cross-parameter rules | local+repository/branch rejected; cloud+isolation rejected; branch without repository rejected; empty task rejected — each returns corrective guidance | `src/services/child-conversation-launch.ts:143-181` |
| Structured result schemas | `LaunchSuccess { status:"launched", conversation_id, url, initial_status, isolation, parent_link, ... }` / `LaunchFailure { status:"error", error, guidance }` | `src/services/child-conversation-launch.ts:42-74` |
| Result delivery channel | Result posted back as a `user` message prefixed `[child-conversation] ` because "client tools have no result channel"; hidden from chat, visible to the agent | `src/services/child-conversation-launch.ts:488-496`, `src/constants/child-conversation.ts:11-20`, `src/components/conversation-events/chat/event-content-helpers/should-render-event.ts:45-51`, `:110-112` |
| Parent/child link | `parent_conversation_id` on start request requires agent-server ≥ 1.37.1; older servers silently drop it and the result reports `parent_link: false` with an explanatory note | `src/constants/child-conversation.ts:30-37`, `src/services/child-conversation-launch.ts:241-250`, `src/api/conversation-service/agent-server-conversation-service.api.ts:477-481` |
| Local child isolation | Child requests the parent's own working dir; `workspaceMode: "new_worktree"` vs `"local_repo"` maps to `worktree`/`shared`; worktree failure retries once as shared with an `isolation_note` | `src/services/child-conversation-launch.ts:277-298`, `:300-323` |
| Worktree feasibility pre-check | Scratch workspaces (no repo/workspace metadata → unborn HEAD) skip the worktree outright to avoid a server-side 500 | `src/services/child-conversation-launch.ts:252-267` |
| Cloud child launch | Bounded poll loop `CLOUD_START_POLL_INTERVAL_MS = 3_000`, `CLOUD_START_POLL_TIMEOUT_MS = 180_000`; ERROR status or timeout reported instead of hanging | `src/services/child-conversation-launch.ts:36-38`, `:365-384` |
| Cloud parent-link suppression | `parent_conversation_id: null` for cloud children — a dangling local id would hide the child from Cloud's list, which filters out anything with a parent | `src/services/child-conversation-launch.ts:416-419` |
| Replay/idempotency ledger | `claimToolCall()` records handled `tool_call_id`s per conversation in localStorage before any network work, so socket replays cannot start a second billable conversation | `src/services/child-conversation-launch.ts:196-227`, dispatch at `:510` |
| Never-reject error policy | Handler "Never rejects: every failure is turned into corrective guidance for the agent" since the server already acknowledged success | `src/services/child-conversation-launch.ts:499-528` |
| Goal-loop interaction | Result message suppressed while an active `/goal` loop runs, because any inbound message cancels the goal loop | `src/services/child-conversation-launch.ts:481-486` |
| Planning sub-conversation creation | UI creates nested run with `parentConversationId: conversation.id, agentType: "plan", entryPoint: "plan_sub_conversation"`; guarded against duplicates via `sub_conversation_ids` and a persisted pending-task id | `src/hooks/use-handle-plan-click.ts:50-83` |
| Nested-run tracing in UI | Planning events flagged `isFromPlanningAgent: true` and deduped separately from main-agent events | `src/contexts/conversation-websocket-context.tsx:788-801`, `src/stores/use-event-store.ts:11` |
| Sub-conversation fetch | `useSubConversations(subConversationIds)` fetches children listed under `conversation.sub_conversation_ids` (5-min staleTime) | `src/hooks/query/use-sub-conversations.ts:22-46`, `src/api/conversation-service/agent-server-conversation-service.types.ts:214` |
| ACP subprocess tool tracing | `ACPToolCallEvent` carries `tool_call_id`, lifecycle `status` (pending/in_progress/completed/failed), `tool_kind`, raw input/output, `is_error`; exactly two persisted events per tool call | `src/types/agent-server/core/events/acp-tool-call-event.ts:38-96`, `src/components/conversation-events/chat/event-content-helpers/should-render-event.ts:124-137` |
| Cost accounting scope | Metrics summed across `usage_to_metrics` entries within one conversation only; nothing aggregates child-conversation costs into the parent | `src/contexts/conversation-websocket-context.tsx:216-276`, `src/hooks/use-live-conversation-metrics.ts:7-55` |
| Per-conversation iteration bound | `max_iterations` (default 500) is set per start request — each child conversation gets its own independent bound | `src/api/agent-server-adapter.ts:1122-1125` |
| Adapter tests (gating) | "includes task_tool_set when sub-agents are enabled…", "omits task_tool_set when sub-agents are disabled even if the server advertises it", "omits browser_tool_set and task_tool_set when the server does not advertise them" | `__tests__/api/agent-server-adapter.test.ts:301-327`, `:329-347`, `:277` |
| Adapter tests (client tools) | "omits the client tool for an inline ACP agent", "sends the client tool for an OpenHands profile", "sends the client tool when resuming a conversation" | `__tests__/api/agent-server-adapter.test.ts:731-781` |
| Launcher test suite | Validation matrix (`it.each` over 6 malformed calls), local/cloud launch, parent-link reporting, worktree fallbacks, replayed tool call, goal-loop suppression | `__tests__/services/child-conversation-launch.test.ts:138-190`, `:192-394`, `:396-486`, `:490-532` |

## Answers to Dimension Questions

1. **Can one agent call another?** Yes, through three channels. (a) The SDK's `task` tool (`task_tool_set`) delegates to named subagents when enabled — the frontend controls availability via `enable_sub_agents` (`src/api/agent-server-adapter.ts:636-641`) and renders the delegation pair (`src/components/features/chat/tool-visualizers/task/task.tsx:49-85`); execution semantics live in the sibling SDK repo, outside this source boundary. (b) Any OpenHands-kind conversation can spawn an *independent* child conversation via the browser-executed `launch_child_conversation` tool (`src/services/child-conversation-launch.ts:505-535`) — this is agent-to-agent composition where the "tool executor" is the frontend itself. (c) ACP agents run whole other coding agents as subprocesses with their inner tool calls traced (`src/types/agent-server/core/events/acp-tool-call-event.ts:34-53`). Additionally, the user can trigger a planning sub-agent from the UI (`src/hooks/use-handle-plan-click.ts:62-67`).

2. **Are child runs bounded?** Partially. Each conversation — parent or child — carries its own `max_iterations` (default 500) and `stuck_detection: true` on its start request (`src/api/agent-server-adapter.ts:1122-1126`), so a child is individually bounded but there is **no aggregate budget across the tree**. The launch path itself is bounded: cloud provisioning polls stop after 180 s (`src/services/child-conversation-launch.ts:37-38,365-384`). No depth limit exists on nesting.

3. **Are child run costs attributed?** No. Cost metrics are computed strictly per conversation: the WS handler sums `usage_to_metrics` entries *within* the active conversation (`src/contexts/conversation-websocket-context.tsx:224-233`) and the combined hook merges store plus current conversation only (`src/hooks/use-live-conversation-metrics.ts:28-55`). The `LaunchSuccess` payload carries the child's id/url/status but no cost field (`src/services/child-conversation-launch.ts:42-66`). The design acknowledges cost exists — the replay-guard comment warns a duplicate cloud launch would be "billable" (`:199-203`) — but attribution stops there. For the planning sub-conversation, however, cost *visibility* is shared at the UI level since both streams render in one chat.

4. **Can nested tools recurse forever?** Nothing structurally prevents it. Children created through `createConversation` inherit the caller's current settings (`src/api/conversation-service/agent-server-conversation-service.api.ts:449-491`), and that path builds the same `client_tools` array containing `launch_child_conversation` for OpenHands-kind agents (`src/api/agent-server-adapter.ts:1116-1119`) — so a child can launch grandchildren without limit. The only guards are non-depth guards: the localStorage replay ledger prevents *duplicate* launches of the same tool call (`src/services/child-conversation-launch.ts:205-227`), validation rejects malformed calls (`:110-194`), and the tool description advises "Do NOT call this tool twice for the same task" (`src/api/launch-child-conversation-client-tool.ts:38`) — advisory, not enforced. No evidence of a max-depth parameter was found anywhere in `src/` (searched for `max_depth`, `recursion`, `depth limit`).

5. **Does the parent receive structured results?** Yes, in both mechanisms, though by different channels. The `task` tool returns a typed observation with `task_id`, `subagent`, `status`, content, and `is_error` delivered in-band by the server (`src/types/agent-server/core/base/observation.ts:305-326`). Because client tools are acknowledged by the agent-server *before* the browser does any work, `launch_child_conversation` results travel out-of-band: a JSON `LaunchSuccess`/`LaunchFailure` envelope posted as a follow-up user message under the `[child-conversation] ` prefix, which the chat filters from display while the LLM still sees it (`src/services/child-conversation-launch.ts:488-496`, `src/constants/child-conversation.ts:11-20`, `should-render-event.ts:110-112`). Failures always carry actionable `guidance` strings (`:68-72`).

## Architectural Decisions

- **Composition ownership split across repos**: this frontend owns *whether* the `task` tool exists (settings gate + tool attach), while the SDK owns *how* subagents execute. The AGENTS.md "Repository Map" codifies that agent/tool behavior belongs in `software-agent-sdk` (repo root `AGENTS.md`, "Repository Map" section). Consequence: this study can verify contracts and gating, but delegation depth/budget logic is unobservable here.
- **Client-defined tools as a composition primitive**: rather than waiting for server support, the launch capability ships as a `ClientToolSpec` JSON schema registered per conversation (`src/api/canvas-ui-client-tool.ts:9-20`); the SDK generates a namespaced action kind (`ClientAction_<tool-name>`, `src/constants/child-conversation.ts:3-9`). This makes the frontend a genuine tool executor — unusual and powerful, but it means the tool's guarantees (validation completeness, idempotency) are client responsibilities.
- **Corrective-guidance error envelope instead of failures**: the handler never throws; every failure becomes `{status: "error", error, guidance}` fed back to the LLM so it can self-correct (`src/services/child-conversation-launch.ts:85-89,499-528`). This treats the calling agent as a recovery loop, mirroring the dimension-04.06 pattern of error envelopes.
- **Version-gated linkage honesty**: rather than assuming the parent link persisted, the launcher compares the cached agent-server version against `MIN_AGENT_SERVER_VERSION_FOR_PARENT_LINK = "1.37.1"` and reports `parent_link: false` with a note when unsupported (`src/constants/child-conversation.ts:30-37`, `src/services/child-conversation-launch.ts:241-250`) — preventing the agent from believing a relationship that does not exist.
- **Isolation as a first-class parameter**: `worktree` vs `shared` maps directly onto git worktree mechanics with automatic downgrade plus an explanatory `isolation_note` when the workspace cannot host a worktree (`src/services/child-conversation-launch.ts:252-267,300-323`).

## Notable Patterns

- **Replay-safe side effects in a reconnecting stream**: because WebSocket resends can replay ActionEvents, the launcher claims each `tool_call_id` in a localStorage ledger *before* any network work, dropping mid-flight replays too (`src/services/child-conversation-launch.ts:196-227`); tested at `__tests__/services/child-conversation-launch.test.ts:490-506`. Corrupt-ledger and full-storage cases degrade gracefully (proceed, accepting replay risk) rather than blocking.
- **Out-of-band result channel for pre-acknowledged tools**: the `[child-conversation] ` prefix doubles as machine protocol and UI filter — one constant drives both the writer and the hider (`src/constants/child-conversation.ts:20`, `should-render-event.ts:50-51`).
- **Nested-stream flagging instead of separate stores**: planning-sub-conversation events flow through the same event store with an `isFromPlanningAgent` marker, letting one chat render two nested runs while keeping dedup per source (`conversation-websocket-context.tsx:793-801`, `src/utils/handle-event-for-ui.ts:35-37`).
- **Two-events-per-call lifecycle contract**: ACP subprocess tool calls persist exactly one `started` and one terminal event per `tool_call_id`, and the UI merges them in place like an action→observation pair (`acp-tool-call-event.ts:10-21`, `should-render-event.ts:124-137`).
- **Capability negotiation with the server**: tool attachment checks `/server_info` advertisement (`isAgentServerToolAvailable`) in addition to local settings (`src/api/agent-server-adapter.ts:631-644`), so gating degrades cleanly against older backends.

## Tradeoffs

- **Frontend as tool executor gains latency-free UX integration at the cost of guarantee strength**: validation must re-implement what a server would enforce (and explicitly patch the SDK's dropped `enum` gap, `child-conversation-launch.ts:99-109`), and durability of the dedup ledger depends on `window.localStorage` being available (`:220-225`).
- **Independent child conversations maximize parallelism but minimize observability**: the child "does not block you, you do not see its output" (`launch-child-conversation-client-tool.ts:16-17`); the parent gets only an id/url/status snapshot, never streamed progress, cost, or completion notifications.
- **Per-conversation bounds simplify reasoning but allow unbounded trees**: each run is individually capped (500 iterations, stuck detection) yet nothing caps fan-out or nesting depth — the safety story leans on LLM compliance with prose instructions.
- **Honest degradation adds report complexity**: `parent_link`, `isolation_note`, and `parent_link_note` fields make results truthful across backend versions but bloat the contract the LLM must parse (`child-conversation-launch.ts:42-66`).
- **Hidden protocol messages keep the chat clean but complicate debugging**: the result message is deliberately filtered from the UI (`should-render-event.ts:105-112`), so a user inspecting the transcript will not see the exact JSON their agent acted on unless they export it.

## Failure Modes / Edge Cases

- **Socket replay double-launch**: mitigated by the claim ledger; tested with an explicit "ignores a replayed tool call" case (`__tests__/services/child-conversation-launch.test.ts:490-506`). Residual risk accepted when localStorage is unavailable (`child-conversation-launch.ts:222-225`).
- **Unborn-HEAD worktree failure**: scratch workspaces make `git worktree add` fail server-side with a 500; pre-checked via conversation metadata and downgraded to `shared` (`child-conversation-launch.ts:252-267,303-306`); even unexpected worktree errors get one fallback attempt with the original error preserved if both fail (`:311-323`, tests at `test:324-393`).
- **Cloud provisioning limbo**: if the sandbox never exposes `app_conversation_id` within 180 s, the caller reports the still-provisioning task instead of hanging (`:365-384`).
- **Dangling parent links**: older local servers silently drop `parent_conversation_id`; cloud parents cannot link at all — both cases surface `parent_link: false` + note rather than silent unlinkage (`:241-250,444-448`).
- **Goal-loop clobbering**: posting the result message during an active `/goal` loop would cancel the loop, so delivery is deferred and only the toast informs the user (`:481-486`, test at `test:510-532`).
- **Title rename best-effort**: local starts carry no title field; a failed rename never fails the launch (`:327-334`).
- **Schema cache poisoning across dev sessions**: editing either client-tool schema requires restarting a long-running dev agent-server due to `ClientToolSchemaConflictError` (`agent-server-adapter.ts:1111-1115`) — an operational edge case documented in-code.

## Future Considerations

- Add a **nesting-depth or fan-out cap** enforced in `validateLaunchParams` (e.g., refuse launches from conversations that already have a `parent_link: true` provenance) — today the only recursion defense is prose (`src/api/launch-child-conversation-client-tool.ts:38`).
- Surface **child-run status and cost to the parent**: a lightweight poll of `sub_conversation_ids` children (the fetch hook already exists, `src/hooks/query/use-sub-conversations.ts:22-46`) could feed an aggregate budget view alongside `max_budget_per_task`.
- Replace the **prefix-matched result channel** with a first-class client-tool result event; the code itself flags the current approach as the workaround for a missing channel (`child-conversation-launch.ts:451-458`, `constants/child-conversation.ts:11-19`).
- Persist the **dedup ledger server-side** (or key it off the event stream's own idempotency) instead of browser localStorage, making replay protection survive cross-device or cleared-storage scenarios.
- Migrate the **planning sub-conversation** to the REST-then-WebSocket history pattern used by the main conversation (noted as still on legacy `resend_all` in repo root `AGENTS.md`, conversation-history section).

## Questions / Gaps

- **Subagent execution semantics are out of boundary.** Whether `task_tool_set` subagents have their own iteration caps, budgets, or depth guards is decided in `software-agent-sdk`, which rule 1 forbids inspecting here. All claims about delegation internals are limited to the wire contract (`action.ts:282-299`, `observation.ts:305-326`).
- **No child trace/span identifiers.** Beyond `parent_conversation_id` and the returned `conversation_id`, no trace-id propagation was found (searched `trace`, `span`, `correlation` patterns in `src/`); "No clear evidence found" for hierarchical tracing.
- **Subagent catalog definition not in this repo.** `subagent_type` values (e.g., fixtures use `"code-explorer"`, `__tests__/components/features/chat/tool-visualizers/test-utils.tsx:197`) are produced server-side; no registry or enum constrains them here.
- **Whether resumed tasks share context.** `TaskAction.resume` implies continuation of a prior subagent task (`action.ts:296-298`), but no frontend logic reads or writes it beyond pass-through rendering; behavior unverifiable in this source.

---

Generated by `dimensions/04.08-agent-as-tool-and-workflow-as-tool-composition` against `openhands`.

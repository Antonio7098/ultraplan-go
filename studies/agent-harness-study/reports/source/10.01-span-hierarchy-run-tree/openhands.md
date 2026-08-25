# Source Analysis: openhands

## Dimension 10.01: Span Hierarchy and Run Tree

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React (Vite), Zustand, TanStack Query, WebSocket; the "agent-canvas" frontend that consumes the OpenHands Agent Server |
| Analyzed | 2026-08-24 |

## Summary

This source is the OpenHands **frontend** ("Agent Canvas"), not the agent runtime. There is no conventional distributed-tracing system here: no OpenTelemetry SDK, no span/trace IDs, no W3C `traceparent` propagation anywhere in app code (`@opentelemetry/api` appears only as vitest's optional peer dependency in `package-lock.json:20891` and `package-lock.json:20907`; grep for `sentry|datadog|performance.mark|span` finds only HTML `<span>` elements).

Instead, the harness's observability backbone is an **event-sourced conversation log** produced by the agent-server and rendered/exported by this UI. Every execution step — user/agent messages, LLM tool calls, tool results, guardrail hooks, rejections, errors, condensation, goal-loop state — is a typed event in one persisted, timestamp-ordered stream (`OpenHandsEvent` union at `src/types/agent-server/core/openhands-event.ts:25-46`). The "span tree" is emulated with **id-based references inside a flat log**: observations point back to actions via `action_id`/`tool_call_id` (`src/types/agent-server/core/events/observation-event.ts:23`, `src/types/agent-server/core/events/observation-event.ts:38`), parallel tool calls from one model response share an `llm_response_id` (`src/types/agent-server/core/events/action-event.ts:56`), and guardrail hooks carry `action_id`/`message_id` links (`src/types/agent-server/core/events/hook-execution-event.ts:69-74`).

Within a single conversation this yields a genuinely coherent, replayable run tree with a built-in viewer (the chat) and multiple exports. Across boundaries it is weaker: nested executions do not truly nest — planning-agent events from a second WebSocket are merged into the same store behind a client-only boolean tag, and `launch_child_conversation` spawns a separate conversation whose events never join the parent's log. Model (LLM) calls have no first-class span; they are inferred from action payloads plus aggregated cost/latency metrics keyed by usage id rather than linked to specific actions.

## Rating

**5 / 10** — Present but partial and fragile at the boundaries.

Rationale against the rubric:

- A single coherent trace exists **per conversation**, with explicit id linkage for every tool call→result pair, hook/guardrail evaluation, rejection, error classification, and condensation, backed by tests (pairing logic in `src/utils/handle-event-for-ui.ts:433-441`, tested via `src/utils/transcript-export/index.test.ts:85-104`; error forwarding tests in `__tests__/contexts/conversation-websocket-context.test.tsx:550-620`). That clears the bottom band.
- But there is no tracing standard or trace-id propagation across process boundaries, model calls are not represented as spans, sub-agent streams are stitched client-side with a non-persisted flag, and child conversations are structurally outside the parent trace (cloud children get no link at all). Those gaps keep it out of the 7–8 band.

## Evidence Collected

Every entry includes a file path with line numbers (relative to the selected source directory).

| Area | Evidence | File:Line |
|------|----------|-----------|
| Trace provider | No OTel/tracing vendor in app code; `@opentelemetry/api` only as vitest optional peer dep | `package-lock.json:20891` |
| Event-sourced trace root | `BaseEvent { id, timestamp, source }` on all events; ids are ULID/UUID | `src/types/agent-server/core/base/event.ts:10-25` |
| Full event taxonomy (one union) | ActionEvent, MessageEvent, ObservationEvent, UserRejectObservation, AgentErrorEvent, SystemPromptEvent, ACPToolCallEvent, HookExecutionEvent, Condensation*, ConversationStateUpdate*, Pause, ServerError, StreamingDelta | `src/types/agent-server/core/openhands-event.ts:25-46` |
| Parent-child span relationship (tool result → tool call) | Observation carries `tool_call_id` and `action_id` referencing the originating action | `src/types/agent-server/core/events/observation-event.ts:18-23`, `:35-39` |
| Sibling grouping under one model call | `llm_response_id` groups parallel tool calls from the same LLM response | `src/types/agent-server/core/events/action-event.ts:52-56` |
| Model-call evidence on spans | `thought`, `reasoning_content`, Anthropic `thinking_blocks`, raw `tool_call`, `summary` on each ActionEvent | `src/types/agent-server/core/events/action-event.ts:12-71` |
| Guardrail verdicts as span attributes | `security_risk` assessment and optional `critic_result` per action | `src/types/agent-server/core/events/action-event.ts:58-66` |
| Guardrail hooks in-tree | `HookExecutionEvent` with `blocked`, `exit_code`, `reason`, and `action_id`/`message_id` links to the triggering event | `src/types/agent-server/core/events/hook-execution-event.ts:20-99` |
| Human rejection spans | `UserRejectObservation` with `rejection_reason` + `action_id` | `src/types/agent-server/core/events/observation-event.ts:42-52` |
| Sub-agent (ACP) lifecycle spans | Two persisted events per `tool_call_id`: started (pending/in_progress) then terminal (completed/failed) | `src/types/agent-server/core/events/acp-tool-call-event.ts:11-21`, `:49-53` |
| Context-management spans | `CondensationEvent.forgotten_event_ids` names exactly which events left the LLM view | `src/types/agent-server/core/events/condensation-event.ts:5-27` |
| Client-side span pairing | Observation replaces its action in the UI array by matching `event.id === event.action_id`; ACP terminal replaces started by `tool_call_id` | `src/utils/handle-event-for-ui.ts:433-441`, `:404-416` |
| Streaming deltas merged/superseded into final events | Delta batching + finalize-in-place so streamed text renders once | `src/utils/handle-event-for-ui.ts:20-29`, `:231-250` |
| Run tree flattening/grouping (viewer) | Consecutive action/observation cards folded into collapsible `EventGroup`s (`EVENT_GROUP_MIN_SIZE = 2`) | `src/components/conversation-events/chat/group-events.ts:13-60` |
| Dual-socket merge for planning agent | Separate WS connection; its events enter the same store tagged `isFromPlanningAgent: true` (client-only field) | `src/contexts/conversation-websocket-context.tsx:172-180`, `:793-798`; type at `src/stores/use-event-store.ts:10-12` |
| Delivery continuity (no trace-id propagation) | Reconnect replays deduped by event id (#1656); resume anchored by latest REST timestamp via `resend_mode='since'` | `src/contexts/conversation-websocket-context.tsx:559-568`, `:368-373`; `src/hooks/query/use-conversation-history.ts:22-28` |
| Cross-process correlation headers | `X-OpenHands-Client`, `X-OpenHands-Client-Version` for cloud ingress facets | `src/api/client-source.ts:12-20` |
| Telemetry distinct-id propagation | Automation consent posts browser PostHog distinct ID under `X-OpenHands-Telemetry-Distinct-Id` | `src/api/automation-service/automation-service.api.ts:73-78` |
| Error telemetry with server-log correlation | `trackError` promotes `eventId` to `error_id`, attaches classification kind | `src/utils/error-handler.ts:17-46` |
| Error-forwarding tests | Assertions that classifications + `eventId`/`toolCallId` reach telemetry for main and planning agents | `__tests__/contexts/conversation-websocket-context.test.tsx:550-620` |
| Metrics tied to model usage ids | `usage_to_metrics` keyed by arbitrary ids ("default", "condenser", "profile:<name>:<uuid>"); `TokenUsage.response_id`, `response_latencies[].response_id` | `src/types/agent-server/core/events/conversation-state-event.ts:10-54` |
| Combined metrics rollup | Stats events combined across usage keys into the metrics meter | `src/contexts/conversation-websocket-context.tsx:216-276` |
| Handoff/sub-conversation launch | Child runs as a separate conversation; local children persist `parent_conversation_id` (server ≥ 1.37.1); replay guarded by tool-call ledger | `src/services/child-conversation-launch.ts:285-298`, `:236-250`, `:196-227` |
| Cloud handoffs unlinked | Cloud child of local parent sends `parent_conversation_id: null` — no cross-system link survives | `src/services/child-conversation-launch.ts:416-447` |
| Handoff results re-enter parent stream | Launch outcome posted back as user message with `CHILD_CONVERSATION_RESULT_PREFIX`, hidden from chat rendering | `src/services/child-conversation-launch.ts:488-496`; `src/components/conversation-events/chat/event-content-helpers/should-render-event.ts:45-51`, `:110-111` |
| Sub-agent task tool in-tree | `TaskAction`/`TaskObservation` kinds for `task`-tool delegation to spawned subagents | `src/types/agent-server/core/base/base.ts:39-50`, `:40`, `:50` |
| Trace export (transcript) | Markdown/HTML export pairs observations to actions via `actionsById`, includes hook details | `src/utils/transcript-export/index.ts:201-247`, `:322`, `:432` |
| Full-history loader | Paginates newest→oldest with cursor/timestamp anchors and id-dedup, merges live store | `src/utils/transcript-export/load-complete-events.ts:31-60` |
| Raw trajectory export | `FileClient.downloadTrajectory` zip; `getTrajectory()` fetches up to 10,000 raw events | `src/api/conversation-service/agent-server-conversation-service.api.ts:674-680`; `src/api/conversation-service/conversation-service.api.ts:54-63` |
| Fork-point lineage | Server-side `parent_id` readable only through single-event endpoint (search API omits it) | `src/api/conversation-service/agent-server-conversation-service.api.ts:830-841` |
| Automation run records | `run_id → conversation_id → bash_command_id` chain for cross-service log retrieval; status/cost/duration | `src/types/automation.ts:75-98` |
| Analytics ≠ tracing | PostHog business-milestone capture; session recording explicitly disabled | `src/services/telemetry.ts:345-362` |

## Answers to Dimension Questions

**1. Is there a single coherent trace tree?**
Within one conversation, yes — as a coherent flat event log rather than a hierarchical tree. All execution artifacts land in one typed union (`src/types/agent-server/core/openhands-event.ts:25-46`) persisted server-side, delivered over both REST pagination and WebSocket (`src/hooks/query/use-conversation-history.ts:30-97`, `src/api/event-service/event-service.api.ts:102-181`), deduplicated by id in the client store (`src/stores/use-event-store.ts:92-129`), and ordered by timestamp with re-sorting on out-of-order arrival (`src/stores/use-event-store.ts:24-53`, `:131-135`). It is not a *tree* in the span sense: there is no `parent_span_id`; hierarchy must be reconstructed from reference fields (`action_id`, `tool_call_id`, `llm_response_id`) by consumers like the UI pairing logic and transcript exporter.

**2. Are all execution steps represented?**
Nearly all, inside the log: model tool calls + results, reasoning/thinking content, guardrail risk assessments, critic evaluations, hook executions (with block decisions and exit codes), user rejections, condensation of forgotten context, pause/error/server-error, and per-model cost/latency/token stats (citations above). Two caveats: (a) LLM inference itself has no dedicated event/span — it is visible only through the resulting ActionEvent payload fields and aggregate `LLMMetrics` (`src/types/agent-server/core/events/conversation-state-event.ts:25-41`); (b) the *rendered* view intentionally hides several logged steps (goal-loop re-prompts matched by brittle string prefixes, machine payloads, successful model switches — `should-render-event.ts:15-43`, `:76-111`), so UI-visible coverage < logged coverage.

**3. Do handoffs and subagent calls nest correctly?**
Partially, and inconsistently by mechanism:
- *Planning sub-agent*: a second WebSocket's events are merged into the same view tagged `isFromPlanningAgent` (`src/contexts/conversation-websocket-context.tsx:793-798`) — co-presented, not nested; the tag is a client-only boolean that exists only in memory (`src/stores/use-event-store.ts:10-12`) and its streaming deltas need sender-scoped guards to avoid cross-agent misattribution (#1656 comments at `src/utils/handle-event-for-ui.ts:31-37`).
- *ACP subprocess agents* (Claude Code, Codex, Gemini CLI): their tool calls appear as first-class `ACPToolCallEvent`s with lifecycle status in the same stream (`src/types/agent-server/core/events/acp-tool-call-event.ts:34-96`) — the best-behaved nesting.
- *`task`-tool subagents*: delegated work appears inline as TaskAction/TaskObservation pairs (`src/types/agent-server/core/base/base.ts:39-50`).
- *Child conversations* (`launch_child_conversation`): do **not** nest. Local children carry a server-side `parent_conversation_id` (`src/services/child-conversation-launch.ts:297`), but cloud children deliberately send `parent_conversation_id: null` (`src/services/child-conversation-launch.ts:416-419`), and the only return path into the parent's trace is a synthetic user message wrapping JSON (`src/services/child-conversation-launch.ts:488-496`). You cannot follow the child's internal run from the parent's trace.

**4. Can you follow a request from start to finish?**
Yes within a conversation, via three complementary surfaces: the live chat/terminal/browser views fed by one store; the transcript export which reconstructs complete history with action↔observation pairing (`src/utils/transcript-export/index.ts:322`, `:432`; `src/utils/transcript-export/load-complete-events.ts:31-60`); and the raw trajectory download (`src/api/conversation-service/agent-server-conversation-service.api.ts:674-680`). Across processes it degrades to bookkeeping: coarse identity headers for cloud ingress (`src/api/client-source.ts:12-20`), a telemetry distinct-id header for automation consent (`src/api/automation-service/automation-service.api.ts:73-78`), `error_id` promotion for correlating client errors with server logs (`src/utils/error-handler.ts:28-44`), and automation `run_id → conversation_id → bash_command_id` chains (`src/types/automation.ts:75-98`). There is no end-to-end trace id shared between browser, ingress, agent-server, sandbox tools, and LLM gateway.

## Architectural Decisions

1. **Event sourcing instead of spans.** The trace primitive is a durable, typed event log with ULID ids and ISO timestamps (`src/types/agent-server/core/base/event.ts:10-19`). Hierarchy is derived, not stored. This makes the log simultaneously the UI feed, replay anchor, export format, and billing data source.
2. **Reference-field lineage over parent pointers.** Each observation/hook/rejection points at its cause by id (`observation-event.ts:38`; `hook-execution-event.ts:69-74`), and sibling tool calls share `llm_response_id` (`action-event.ts:56`) — a minimal, composable alternative to nested spans that keeps the wire format flat and append-only.
3. **Server-authoritative log, client-rendered tree.** Pairing (action superseded by observation; ACP started replaced by terminal) happens in client state at render time (`src/utils/handle-event-for-ui.ts:404-441`), so different viewers can choose different projections (chat vs. transcript vs. trajectory dump).
4. **Separation of execution observability from product analytics.** The event log observes the agent; PostHog observes the human/product funnel, consent-gated with session recording disabled (`src/services/telemetry.ts:345-362`). Errors bridge the two via `trackError` carrying event ids/classifications (`src/utils/error-handler.ts:17-46`).
5. **Conversation-per-subexecution.** Delegation (planning agent, child conversations) is modeled as separate conversations with optional parent links, not nested traces — a deliberate isolation choice that trades traceability for workspace/credential isolation (isolation modes in `src/constants/child-conversation.ts`; worktree fallback logic in `src/services/child-conversation-launch.ts:265-323`).

## Notable Patterns

- **Span pairing with idempotent replacement**: observations overwrite their actions by id in the UI array; unknown-action observations append defensively (`src/utils/handle-event-for-ui.ts:433-441`). The same pattern generalizes to ACP two-phase tool events (`:404-416`).
- **Replay safety**: reconnect replays are deduped by event id before side effects run, because handlers like cache invalidation and model-switch recording are not idempotent (#1656 guard at `src/contexts/conversation-websocket-context.tsx:559-568`); the launch-child tool additionally persists a localStorage ledger of handled `toolCallId`s to prevent billable double launches (`src/services/child-conversation-launch.ts:196-227`).
- **Timestamp-anchored resume**: instead of continuation tokens, the socket resumes with `resend_mode='since'` + the last REST event's timestamp (`src/contexts/conversation-websocket-context.tsx:368-400`), a temporal stand-in for trace context.
- **Trace-as-export**: three gradated exports — human transcript (markdown/html), full raw trajectory (zip via `downloadTrajectory`), and CSV/JSON activity-log rows for automation runs with computed duration and cost (`src/utils/automation-activity-log-export.ts:12-27`, `:76-108`).
- **Collapsible run grouping** in the viewer folds consecutive tool steps into one row while hoisting agent thoughts out as standalone items (`src/components/conversation-events/chat/group-events.ts:13-60`; AGENTS.md "Action grouping" section).

## Tradeoffs

- **Flat log + derived hierarchy** keeps persistence simple and lossless, but every consumer must reimplement tree reconstruction; the search API even omits fork `parent_id`, forcing per-event fetches (`src/api/conversation-service/agent-server-conversation-service.api.ts:830-841`).
- **Client-only merge tags** (`isFromPlanningAgent`) avoid schema churn but make attribution volatile: it lives only in the memory-resident store and depends on which socket delivered the event, not on anything persisted.
- **Aggregate model metrics** (`usage_to_metrics`) give cost/latency visibility without leaking prompts, but `response_latencies[].response_id` cannot be joined back to a specific ActionEvent by any code in this repo — per-turn cost attribution stops at the usage-key level (`src/types/agent-server/core/events/conversation-state-event.ts:35-41`, `:44-54`).
- **Conversation-isolated delegation** buys strong isolation and simple mental models but fragments the trace exactly at the most interesting moments (handoffs), acknowledged in code comments about links that "mean nothing" across systems (`src/services/child-conversation-launch.ts:416-419`).

## Failure Modes / Edge Cases

- **Out-of-order and duplicated delivery**: handled by timestamp re-sorting and id dedup (`src/stores/use-event-store.ts:24-53`, `:99-107`), but events without ids (streaming deltas) bypass dedup by design (`:94-97`).
- **Cloud history pagination regression**: if the backend lacks timestamp filters, event search falls back to a limit-only request and returns an *empty page* rather than looping (`src/api/event-service/event-service.api.ts:143-163`) — silent truncation of older trace history in that mode.
- **Malformed oldest event** during scroll-up pagination throws and disables further backfill, surfacing an error banner rather than failing silently (AGENTS.md "useLoadOlderEvents" notes).
- **Cross-agent delta bleed**: without sender-scoped guards, a planning-agent stream could concatenate onto the main agent's bubble or strip its reasoning — fixed twice (#1656) in `src/utils/handle-event-for-ui.ts:31-37`, `:78-98`.
- **Goal-loop machinery leaks**: injected re-prompt messages are indistinguishable from real user input in the persisted event, so filtering matches prompt text prefixes — explicitly documented as "brittle by design" (`should-render-event.ts:15-26`).
- **Unlinked cloud handoffs**: a cloud child launched from a local parent is unreachable from any persisted link; only the toast URL returned in the tool result preserves the relationship (`src/services/child-conversation-launch.ts:433-448`).

## Future Considerations

- Adopt W3C Trace Context (or OTel) headers between canvas ↔ ingress ↔ agent-server ↔ automation so requests can be followed across process boundaries today tracked only via coarse headers and id bookkeeping.
- Persist an agent-source discriminator (e.g., `conversation_id` or a stable `origin` field) on merged sub-agent events instead of the in-memory `isFromPlanningAgent` flag.
- Emit a first-class model-call event (or enrich `LLMMetrics` entries with the triggering action/event ids) so latency and cost can be attributed to individual steps.
- Expose `parent_id` in the events search API so fork lineage can be rendered without N+1 single-event fetches.
- Replace prefix-matching filters for goal-loop re-prompts with a persisted marker field on the event, as the code comment already recommends (`should-render-event.ts:20-22`).

## Questions / Gaps

- **No evidence found** of any distributed-trace identifiers (trace/span ids, `traceparent`) crossing the browser↔server boundary; searches covered `otel|opentelemetry|traceparent|traceId|trace_id|sentry|datadog` across the whole source.
- **No evidence found** for eval-run tracing: the dimension mentions evals, but nothing in this frontend captures evaluation runs as traces (the closest artifact is the exported mock-LLM completion-request history used by tests, described in AGENTS.md's mock-LLM framework section).
- Whether the agent-server internally emits OpenTelemetry spans could not be verified from this repository — the runtime lives in the separate `software-agent-sdk` repo (see `docs/architecture.md:23` and the repo map in AGENTS.md); this analysis covers only what is observable from the frontend contract.
- Guardrail *policy enforcement* (confirmation flows) is visible as events (`respondToConfirmation` at `src/api/event-service/event-service.api.ts:40-69`; `UserRejectObservation`), but whether a policy engine evaluates before or after the LLM call is not determinable from this source alone.

---

Generated by `10.01-span-hierarchy-and-run-tree` against `openhands`.

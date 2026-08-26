# Source Analysis: openhands

## 05.01 Short-Term Conversation Memory

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (`@openhands/agent-canvas` v1.15.0) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19, Zustand, TanStack Query, WebSocket; Vite build |
| Analyzed | 2026-08-26 |

> Citation convention: all `file:line` citations below are workspace-relative and rooted at `studies/agent-harness-study/sources/openhands/`.

**Scope boundary (established up front).** This repo is *only the frontend* of a multi-repo system: "This repo (`OpenHands/OpenHands`) is **only the agent-canvas frontend**" (`AGENTS.md:24`), while "agents, tools, conversations, events, and the REST/WebSocket API surface" — i.e. the code that actually assembles LLM message history per call — live in the sibling `OpenHands/software-agent-sdk` repo (`AGENTS.md:31`, `AGENTS.md:40`). The model-facing history builder (system-prompt injection, condenser implementation, View construction) is therefore **not inside this source** and could not be inspected without violating source-isolation rules. What this source owns — and what this study analyzes — is the client-side short-term-memory pipeline: conversation-history storage, windowed retrieval, incremental replay, compaction control/verification, role/tool-message contracts, and fork/edit semantics.

## Summary

OpenHands Agent Canvas keeps recent conversation state as an append-only event log synchronized with the agent-server. Storage is a global Zustand event store holding two parallel arrays — raw chronological `events` (with an id `Set` for O(1) dedup) and render-ready `uiEvents` (`studies/agent-harness-study/sources/openhands/src/stores/use-event-store.ts:55-129`). History is loaded **REST-first, then WebSocket**: the most recent 50 events are fetched with `sort_order='TIMESTAMP_DESC'` and reversed to chronological order (`src/hooks/query/use-conversation-history.ts:10,43-71`), after which the WebSocket subscribes with `resend_mode='since'` + `after_timestamp=<latest preloaded event>` so only strictly newer events stream in (`src/contexts/conversation-websocket-context.tsx:964-1002`). Older history is backfilled on demand via keyset/timestamp pagination when the user scrolls up (`src/hooks/use-load-older-events.ts:89-165`).

The actual selection of what the model sees is server-side condensation: this repo consumes its contract — a `CondensationEvent` carrying `forgotten_event_ids` ("removed from the View given to the LLM"), an optional `summary`, and a `summary_offset` (`src/types/agent-server/core/events/condensation-event.ts:8-27`) — exposes a manual **Compact context** action that POSTs `/api/conversations/{id}/condense` (`src/api/conversation-service/agent-server-conversation-service.api.ts:717-745`), verifies the result by watching for the `Condensation` event plus a drop in live `per_turn_token` metrics with a 90 s timeout (`src/hooks/use-await-context-compaction.ts:57-163`), and surfaces condenser configuration (`enable_default_condenser: true`, `condenser_max_size: 240`) through schema-driven settings (`src/services/settings.ts:18-19,42-45`; `src/routes/condenser-settings.tsx:3-14`). Roles follow the LLM wire format directly: `Message.role: "user" | "system" | "assistant" | "tool"` with `tool_calls`, `tool_call_id`, and Anthropic-style thinking blocks (`src/types/agent-server/core/base/event.ts:28-48`); outbound user turns are sent with `role: "user"` over WS or REST fallback (`src/hooks/use-send-message.ts:54-58`; `src/contexts/conversation-websocket-context.tsx:1094-1144`). History can be **forked** (branch at any message via `from_event_id`, or edit-and-rebranch by resolving the message's `parent_id`) but not edited in place.

## Rating

**6 / 10** — Present, well-engineered client side, but the dimension's core mechanism (per-call model-visible history selection) is implemented outside this source.

Rationale against the rubric:

- What this source owns is genuinely strong: a clear storage model with tests (`__tests__/stores/use-event-store.test.ts:96-261` covers dedup, bulk-add re-sorting, delta compaction, action→observation replacement), explicit interfaces to server memory (`CondensationEvent.forgotten_event_ids` semantics documented at `src/types/agent-server/core/events/condensation-event.ts:13-16`), pagination/cache behavior locked by tests (`__tests__/hooks/query/use-conversation-history.test.tsx:76-433`, including cache-key stability and gcTime assertions), and operational safeguards everywhere (dedup on replay, WS gating on first load, compaction outcome verification with timeout, optimistic-send watchdog).
- It does not reach 7–8 because (a) the decisive question of this dimension — what history is selected and passed back to the model each call — cannot be verified from this repo at all (the condenser implementation and View construction live in `software-agent-sdk`, per `AGENTS.md:31`); (b) several paths degrade silently by design (cloud pagination falls back to an empty page, `src/api/event-service/event-service.api.ts:149-163`); (c) the planning sub-conversation still uses legacy full-resend replay (`src/contexts/conversation-websocket-context.tsx:1004-1008`); and (d) goal-loop machine prompts are filtered by brittle text-prefix matching, acknowledged "Brittle by design" in-code (`src/components/conversation-events/chat/event-content-helpers/should-render-event.ts:15-26`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Message store (client) | Zustand `useEventStore`: raw `events[]` + renderable `uiEvents[]` + `eventIds` Set for O(1) dedup; single global store keyed by `loadedConversationId` | `studies/agent-harness-study/sources/openhands/src/stores/use-event-store.ts:55-90` |
| Store append/dedup/merge | `appendEvent` skips known ids, merges streaming deltas in place, rebuilds `uiEvents` via `handleEventForUI` | `studies/agent-harness-study/sources/openhands/src/stores/use-event-store.ts:92-129` |
| Bulk seed + re-sort | `addEvents` bulk-inserts REST pages and re-sorts once by timestamp so older pages slot into position | `studies/agent-harness-study/sources/openhands/src/stores/use-event-store.ts:159-208` |
| Session-history builder (windowed tail) | `useConversationHistory`: fetches last `INITIAL_HISTORY_PAGE_SIZE = 50` events, `TIMESTAMP_DESC`, reversed to chronological | `studies/agent-harness-study/sources/openhands/src/hooks/query/use-conversation-history.ts:10,43-71` |
| Cache policy | `staleTime: 0`, `gcTime` 30 min, `refetchOnMount: "always"` batches events missed while away; retry capped at 1 to avoid holding the WS gate | `studies/agent-harness-study/sources/openhands/src/hooks/query/use-conversation-history.ts:91-96` |
| Message selector (older pages) | `useLoadOlderEvents`: paginates `timestampLt < oldest known`, ref/guarded against duplicate requests, stops on short page or missing `next_page_id` | `studies/agent-harness-study/sources/openhands/src/hooks/use-load-older-events.ts:89-165` |
| Events retrieval API | `EventService.searchEvents` with `limit/page_id/sort_order/timestamp__gte/timestamp__lt`; cloud App-API vs local typed-client split; cloud filter fallback returns empty page | `studies/agent-harness-study/sources/openhands/src/api/event-service/event-service.api.ts:102-181` |
| Incremental replay anchor | Main WS options: `resend_mode:'since'` + `after_timestamp=<latest preloaded>`; falls back to `'all'` when REST returned nothing or errored | `studies/agent-harness-study/sources/openhands/src/contexts/conversation-websocket-context.tsx:964-1002` |
| WS gate on first history load | Socket URL stays `null` while `isPreloadingHistory` so `'since'` has a meaningful anchor; background refetches never tear down a live socket | `studies/agent-harness-study/sources/openhands/src/contexts/conversation-websocket-context.tsx:376-400` |
| Replay safety | Reconnect replays deduped by id; non-idempotent side-effects skipped for already-known events (#1656) | `studies/agent-harness-study/sources/openhands/src/contexts/conversation-websocket-context.tsx:553-568` |
| Per-conversation isolation | Atomic `clearEventsForConversation` + metrics reset on conversation switch, ordered before re-seed via `useLayoutEffect` | `studies/agent-harness-study/sources/openhands/src/contexts/conversation-websocket-context.tsx:293-318` |
| UI view construction | `handleEventForUI`: observations replace their actions by `action_id`; `ThinkObservation`/`FinishObservation` dropped (action kept); ACP tool-call started→terminal merged in place by `tool_call_id` | `studies/agent-harness-study/sources/openhands/src/utils/handle-event-for-ui.ts:348-448` |
| Streaming delta handling | Deltas batched per frame, merged by sender flag; flushed before subsequent events to preserve order | `studies/agent-harness-study/sources/openhands/src/contexts/conversation-websocket-context.tsx:162-180`; `src/utils/handle-event-for-ui.ts:20-37` |
| Role contract | `Message.role: "user"\|"system"\|"assistant"\|"tool"`, content parts text/image, `tool_calls`, `tool_call_id`, `reasoning_content`/thinking blocks | `studies/agent-harness-study/sources/openhands/src/types/agent-server/core/base/event.ts:28-48` |
| Exact LLM message exposure | `MessageEvent.llm_message` is "the exact LLM message for this message event"; also carries `activated_skills`, `extended_content` | `studies/agent-harness-study/sources/openhands/src/types/agent-server/core/events/message-event.ts:5-25` |
| Condensation contract | `CondensationEvent.kind="Condensation"` with `forgotten_event_ids` ("removed from the View given to the LLM"), optional `summary`, `summary_offset`; sibling `CondensationRequestEvent`, `CondensationSummaryEvent` | `studies/agent-harness-study/sources/openhands/src/types/agent-server/core/events/condensation-event.ts:5-52` |
| Condensation detection guard | `isCondensationEvent` matches `kind === "Condensation"` | `studies/agent-harness-study/sources/openhands/src/types/agent-server/type-guards.ts:286-289` |
| Manual compaction trigger | `POST /api/conversations/{id}/condense` (cloud proxied to runtime host; local via `ConversationClient.condenseConversation`) | `studies/agent-harness-study/sources/openhands/src/api/conversation-service/agent-server-conversation-service.api.ts:717-745` |
| Compaction verification | `useAwaitContextCompaction`: waits for post-request `Condensation` event + lower `per_turn_token`; outcomes `compacted/no_change/timeout`; 2.5 s metrics settle, 90 s timeout; baseline ids captured pre-request to win races | `studies/agent-harness-study/sources/openhands/src/hooks/use-await-context-compaction.ts:39-163`; baseline capture at `src/hooks/use-compact-context-action.ts:83-99` |
| Condenser configuration | Defaults `enable_default_condenser: true`, `condenser_max_size: 240`; flat↔nested mapping to agent settings `condenser.enabled`/`condenser.max_size`; schema-driven settings page section `condenser` | `studies/agent-harness-study/sources/openhands/src/services/settings.ts:18-19,42-45`; `src/api/settings-service/settings-service.api.ts:380-399`; `src/routes/condenser-settings.tsx:3-14` |
| Context-window observability | Live meter from `context_window`/`per_turn_token` (WS stats aggregated across usage ids incl. `"condenser"`; REST fallback) | `studies/agent-harness-study/sources/openhands/src/hooks/use-context-window-usage.ts:31-52`; `src/contexts/conversation-websocket-context.tsx:216-276` |
| Tool messages retained (wire) | Tool-call structures (`ChatCompletionMessageToolCall{id,type,function}`) and `tool_call_id` on `Message` | `studies/agent-harness-study/sources/openhands/src/types/agent-server/core/base/event.ts:33-48` |
| Tool messages retained (client view) | Action/Observation pairs kept in raw store; UI replaces action with observation; errors tracked with `tool_name`+`tool_call_id` | `studies/agent-harness-study/sources/openhands/src/utils/handle-event-for-ui.ts:418-441`; `src/contexts/conversation-websocket-context.tsx:598-608` |
| Outbound role handling | User sends `{role:"user", content:[text,image…]}` over WS with `run:true`; REST queue fallback `sendEvent(role:"user")` when socket closed | `studies/agent-harness-study/sources/openhands/src/hooks/use-send-message.ts:36-58`; `src/contexts/conversation-websocket-context.tsx:1094-1144` |
| Optimistic send w/ echo reconcile | Pending queue scoped per conversation, matched by echoed text, FIFO fallback, 150 s watchdog flips stuck sends to error | `studies/agent-harness-study/sources/openhands/src/stores/optimistic-user-message-store.ts:14,115-198`; consume sites at `src/contexts/conversation-websocket-context.tsx:610-624,343-351` |
| Fork / edit-and-rebranch | `forkConversation(from_event_id)` requires agent-server ≥ 1.31.0, cloud unsupported; edit mode resolves `parent_id` (single-event endpoint since search omits it), excludes message, prefills edited text | `studies/agent-harness-study/sources/openhands/src/hooks/mutation/use-fork-conversation.ts:22-70`; `src/api/conversation-service/agent-server-conversation-service.api.ts:792-842` |
| Client-side drops (render layer) | Goal-loop re-prompts and child-conversation JSON results filtered from chat; `SwitchLLMAction`/`PlanningFileEditorAction`/user bash commands hidden; state updates hidden except finished goal | `studies/agent-harness-study/sources/openhands/src/components/conversation-events/chat/event-content-helpers/should-render-event.ts:23-145` |
| Tests: history selection | Tail request shape, desc→chrono reversal, full-page-without-`next_page_id` heuristic, malformed-page rejection, retry-once, no focus-refetch, 30-min cache | `studies/agent-harness-study/sources/openhands/__tests__/hooks/query/use-conversation-history.test.tsx:76-433` |
| Tests: store behavior | Dedup, bulk-add sort, delta compaction (incl. cross-sender #1656 case), action→observation replacement, clear | `studies/agent-harness-study/sources/openhands/__tests__/stores/use-event-store.test.ts:96-261` |
| Tests: compaction | Token-freed reporting, early-condensation race (event landed before POST ack), timeout classification | `studies/agent-harness-study/sources/openhands/src/hooks/use-await-context-compaction.test.ts:58-147`; UI toast outcomes at `src/hooks/use-compact-context-action.test.tsx:78-103` |

## Answers to Dimension Questions

**1. What conversation history does the model see?**
Not decidable from this source. The per-call assembly lives in `software-agent-sdk` (`AGENTS.md:31,40`). This repo's evidence bounds the answer: every `MessageEvent` persists "the exact LLM message" (`src/types/agent-server/core/events/message-event.ts:6-9`), and condensation removes exactly the `forgotten_event_ids` from "the View given to the LLM", optionally splicing in a summary at `summary_offset` (`src/types/agent-server/core/events/condensation-event.ts:13-26`). So the architecture implies: full persisted event log + a condenser-defined window/summary view — but the window policy itself is unverifiable here. **No evidence found in this repo for the concrete selection algorithm.**

**2. What gets dropped?**
Two layers. Model side: whatever the condenser marks forgotten (`forgotten_event_ids`, summarized into `summary`) — observable but not controlled here; condenser defaults ship enabled with `max_size: 240` (`src/services/settings.ts:18-19`). Client/render side (does not affect the model): `ThinkObservation` and `FinishObservation` are never added to `uiEvents` (the actions stand in, `src/utils/handle-event-for-ui.ts:419-430`); finalized messages supersede streamed deltas which are stripped (`src/utils/handle-event-for-ui.ts:225-250`); goal-loop re-prompts ("The goal is NOT yet complete…", "Resuming a goal…") and child-conversation result payloads are hidden from chat by prefix match (`src/components/conversation-events/chat/event-content-helpers/should-render-event.ts:15-51`).

**3. Are tool messages retained?**
Yes at the protocol level: `Message` supports `role: "tool"`, `tool_calls`, and `tool_call_id` (`src/types/agent-server/core/base/event.ts:28-48`), and the event union includes Action/Observation pairs plus two-phase `ACPToolCallEvent`s keyed by `tool_call_id` (`src/types/agent-server/core/openhands-event.ts:25-46`; merge logic `src/utils/handle-event-for-ui.ts:404-416`). Whether tool-role entries survive condensation toward the model is decided server-side — no client evidence either way.

**4. Is memory per user/thread/session?**
Per-conversation (thread). The client event store is deliberately global-but-single-conversation: switching conversations atomically clears events under a `loadedConversationId` invariant (`src/stores/use-event-store.ts:59-89`; `src/contexts/conversation-websocket-context.tsx:303-318`), the history query key includes `conversationId` + host + session key so backend swaps refetch (`src/hooks/query/use-conversation-history.ts:34-41`), pending optimistic bubbles are scoped by `conversationId` (`src/stores/optimistic-user-message-store.ts:16-37`), and only lightweight per-conversation metadata/drafts persist in localStorage (`conversation-state-{id}`, `src/utils/conversation-local-storage.ts:37-76`). Full durable memory is server-side (cloud history even survives sandbox recycling — cloud event search hits the App API "Persisted by the cloud backend — survives the runtime sandbox", `src/api/event-service/event-service.api.ts:18-25`).

**5. Can history be edited or forked?**
Fork yes, edit no (no in-place mutation API exists client-side). Fork branches at any message via `POST /fork` with `from_event_id` (local agent-server ≥ 1.31.0; older servers silently copy everything — detected via `leaf_event_id` check; cloud unsupported) (`src/hooks/mutation/use-fork-conversation.ts:53-70`; `src/api/conversation-service/agent-server-conversation-service.api.ts:792-817`). "Edit message" resolves the target's `parent_id` and branches one step earlier, excluding the original turn, then prefills the composer with replacement text (`use-fork-conversation.ts:42-67`; `parent_id` helper at `agent-server-conversation-service.api.ts:829-842`). Drafts are editable pre-send only (`draftMessage` in localStorage, cleared on confirmed echo, `src/contexts/conversation-websocket-context.tsx:621-622`).

## Architectural Decisions

1. **REST-first tail, WebSocket-since replay.** History loads as a bounded 50-event tail over REST; the socket opens only after that settles and anchors `resend_mode='since'` at the newest timestamp (`src/contexts/conversation-websocket-context.tsx:376-400,964-1002`). This eliminates double-streaming history and makes reconnects cheap; overlap between refetch page and replay is tolerated and deduped by id.
2. **Dual-array event store (raw vs UI view).** Raw `events` preserve the authoritative log; `uiEvents` applies presentation transforms (observation supersedes action, delta merging) so rendering logic stays declarative (`src/stores/use-event-store.ts:55-58`, `src/utils/handle-event-for-ui.ts:337-347`).
3. **Compaction is server-executed, client-verified.** The client triggers but never performs condensation, and refuses to trust the HTTP ack: completion requires a fresh `Condensation` event plus a measured `per_turn_token` drop, else reports `timeout` as a failure (`src/hooks/use-await-context-compaction.ts:57-61,150-154`).
4. **Conversation-scoped state lifecycle.** One global store with an atomic clear+rebind action prevents cross-conversation leakage and half-applied states during switches (`src/stores/use-event-store.ts:82-89`; `src/contexts/conversation-websocket-context.tsx:303-311`).
5. **Fork-based editing instead of mutable history.** Corrections produce new branches from `parent_id`/`from_event_id`, preserving append-only semantics end-to-end (`src/hooks/mutation/use-fork-conversation.ts:22-27`).

## Notable Patterns

- **Anchor-and-dedupe consistency**: every incremental channel (WS `since`, older-pages pagination, background refetch) converges on the same id-set dedup plus one-shot timestamp re-sort (`src/stores/use-event-store.ts:137-151,202-207`).
- **Optimistic send with echo reconciliation**: pending bubbles carry both display text and the exact wire string, matched FIFO-exact against the server echo, with a 150 s watchdog converting silent failures into retryable errors (`src/stores/optimistic-user-message-store.ts:24-37,131-139,169-198`).
- **Sender-scoped streaming**: deltas merge only within a sender (main vs planning agent) so concurrent agents sharing one store cannot corrupt each other's transcript (#1656 regression, `src/utils/handle-event-for-ui.ts:31-37`; test `__tests__/stores/use-event-store.test.ts:227`).
- **Graceful degradation flags**: cloud backends lacking pagination filters get one attempt, a console warning naming the required upstream fix, and a clean stop instead of an infinite loop (`src/api/event-service/event-service.api.ts:116-163`).
- **Type-guard-driven event discrimination**: ~30 narrow guards (`isCondensationEvent`, `isUserMessageEvent`, …) gate every consumer branch (`src/types/agent-server/type-guards.ts:95-107,286-289`).

## Tradeoffs

- **Client mirror ≠ model truth.** The rich client pipeline guarantees UI fidelity, not prompt fidelity; nothing here can prove what the next LLM call contains beyond the persisted event contract.
- **Unbounded in-session growth.** The store appends for the lifetime of the session (only streaming deltas are compacted); long-running `/goal` loops grow `events`/`uiEvents` without a cap (`src/stores/use-event-store.ts:153-208`).
- **Silent empty-page degradation** on unpatched cloud servers means users simply can't scroll past ~50 events, with only a console warning (`src/api/event-service/event-service.api.ts:153-162`).
- **Prefix-matched filtering of machine prompts** couples frontend copy to SDK constants — flagged "Brittle by design" with a proposed durable fix (persisted flag) left undone (`should-render-event.ts:15-22`).
- **Legacy dual path for planning agent**: still `resend_all` + count-based loading, doubling maintenance surface until migrated (`src/contexts/conversation-websocket-context.tsx:182-191,1004-1044`).
- **Cache-vs-liveness balancing**: `staleTime: 0` + `refetchOnMount: 'always'` trades extra REST traffic on every return visit for batch correctness of missed events (`src/hooks/query/use-conversation-history.ts:73-96`).

## Failure Modes / Edge Cases

- **Reconnect replay duplication**: stale `since` anchors resend old events; ids dedupe them and side-effect handlers bail on duplicates to keep effects idempotent (`src/contexts/conversation-websocket-context.tsx:553-568`).
- **Compaction race**: the `/condense` POST can return after the server already emitted its `Condensation` event; baseline event ids are snapshotted before the request fires so fast condensations aren't missed (`src/hooks/use-compact-context-action.ts:85-88`; tested at `src/hooks/use-await-context-compaction.test.ts:141-147`).
- **First-load failure**: if the initial REST page errors (retry capped at 1), the WS falls back to `resend_mode='all'` so live events still arrive (`src/hooks/query/use-conversation-history.ts:86-90`; `src/contexts/conversation-websocket-context.tsx:389-391`).
- **Malformed pagination data**: non-array `page.items` throws a descriptive error rather than poisoning the store (`src/hooks/use-load-older-events.ts:136-140`; `__tests__/hooks/query/use-conversation-history.test.tsx:180`).
- **Stuck optimistic bubble**: watchdog flips unanswered sends to error state with retry affordance after 150 s (`src/stores/optimistic-user-message-store.ts:131-139`).
- **Cross-conversation staleness**: metrics from a prior conversation would stick in the new one's meter; reset on switch (`src/contexts/conversation-websocket-context.tsx:313-317`).
- **Older-server fork ambiguity**: servers ignoring `from_event_id` are detected via `leaf_event_id` mismatch to avoid double-prefilling the editor (`use-fork-conversation.ts:59-68`).

## Future Considerations

- Migrate the planning sub-conversation onto the REST-tail + `since` pattern to retire the count-based `resend_all` loader (`src/contexts/conversation-websocket-context.tsx:182-187`).
- Replace text-prefix goal-loop filtering with a persisted marker on the event (the in-code suggested fix) so chat filtering can't drift from SDK prompt copy (`should-render-event.ts:20-22`).
- Bound the client event store (e.g., virtualize or cap raw arrays) for very long autonomous sessions.
- Type `from_event_id` in `@openhands/typescript-client`'s `ForkConversationRequest` instead of casting (`agent-server-conversation-service.api.ts:808-817`).
- Expose condenser provenance in the usage panel (which usage id condensed what) — today `"condenser"` appears only as an anonymous key in metric aggregation (`src/contexts/conversation-websocket-context.tsx:224-226`).

## Questions / Gaps

- **What exact windowing/summarization policy feeds the LLM each turn?** No evidence found in this repository — searched all of `src/` for condenser/view/history-builder implementations (`grep -i "condens|view|history"`); only settings keys, UX, and the event contract exist. The implementation resides in the sibling `software-agent-sdk` repo, outside this study's isolation boundary.
- **Are `tool`-role messages preserved through condensation?** Unanswerable here; only the wire types and client retention are visible.
- **Does the server honor `condenser_max_size` as events, tokens, or messages?** Frontend copy says tokens ("Maximum number of tokens the condenser keeps after summarization", `src/mocks/settings-handlers.ts:333`) but the unit semantics are defined server-side and unverifiable from this source.
- **Is there any cross-conversation/user-level short-term memory?** None found — by design each conversation is isolated; anything user-scoped would be long-term memory (dimension 05.03 territory).

---

Generated by `dimensions/05.01-short-term-conversation-memory.md` against `openhands`.

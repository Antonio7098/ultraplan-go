# Source Analysis: openhands

## 07.07 Tool Output Streaming

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (OpenHands Agent Canvas frontend, `@openhands/agent-canvas` v1.15.0) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19, Zustand stores, TanStack Query, xterm.js, WebSocket transport against the Python `software-agent-sdk` agent-server |
| Analyzed | 2026-08-25 |

> Citation convention: all paths below are relative to the source root `studies/agent-harness-study/sources/openhands/`. This repo is **only the frontend** of a multi-repo system (`AGENTS.md`, "Repository Map" section); tool execution itself lives in `software-agent-sdk`, so server-side streaming behavior is inferred from the event contract the frontend consumes.

## Summary

Tool output in this codebase is not streamed as a continuous pipe; it arrives as a discrete, typed event stream over one WebSocket per conversation. The central dispatcher is `ConversationWebSocketProvider` (`src/contexts/conversation-websocket-context.tsx:121`), which fans each parsed `OpenHandsEvent` into dedicated stores. Three distinct streaming mechanisms exist:

1. **LLM token/reasoning streaming** — `StreamingDeltaEvent`s (`src/types/agent-server/core/events/streaming-delta-event.ts:3-8`) are buffered by an animation-frame batcher (`src/utils/streaming-delta-batcher.ts:36-75`) and merged in place into a single provisional bubble, then reconciled against the durable final `MessageEvent`/`ActionEvent` (`src/utils/handle-event-for-ui.ts:231-301`). Streaming is explicitly forced on at conversation start (`src/api/agent-server-adapter.ts:895-897`) and after model switches (`src/api/conversation-service/agent-server-conversation-service.api.ts:904-906`).
2. **ACP sub-agent tool-call lifecycle streaming** — `ACPToolCallEvent` (`src/types/agent-server/core/events/acp-tool-call-event.ts:38-96`) carries a `pending`/`in_progress`/`completed`/`failed` status; the SDK persists two events per `tool_call_id` and the UI replaces the running card in place with the terminal one (`src/utils/handle-event-for-ui.ts:404-416`). Notably, an earlier design that fanned out one cumulative-output frame per `ToolCallProgress` was deliberately removed because it "flashed half-formed cards mid-stream" (`src/components/conversation-events/chat/event-content-helpers/should-render-event.ts:124-134`).
3. **Long-running-loop progress** — `/goal` loops stream live `GoalStatus` (round count, judge verdict, running/interrupted state) as `ConversationStateUpdateEvent`s (`src/types/agent-server/core/events/conversation-state-event.ts:77-97`), rendered live and inline (`src/components/features/chat/goal-status-content.tsx:46-128`).

The major gap: **native OpenHands tools (bash, file editor, browser) have no incremental output channel**. A bash command's output reaches the terminal store as one complete `ExecuteBashObservation` after execution finishes (`src/contexts/conversation-websocket-context.tsx:663-670`), appended wholesale to xterm via `useCommandStore.appendOutput` (`src/stores/command-store.ts:21-24`). During a long-running command the user sees only a typing-indicator chip naming the running action (`src/components/features/chat/typing-indicator.tsx:70-124`). Partial visibility for such commands is achieved by convention instead: the tool schema lets the model send an empty command to "view additional logs when previous exit code is `-1`", or `C-c` to interrupt (`src/types/agent-server/core/base/action.ts:27-44`).

## Rating

**6 / 10** — Present but inconsistent across tool kinds.

What exists is genuinely well-engineered: token streaming has an explicit event interface, animation-frame coalescing proven byte-exact under 5,000-delta bursts (`__tests__/utils/streaming-delta-batcher.test.ts:145-172`), in-place reconciliation so text never renders twice, sender-scoping between main/planning agents, and regression tests for reconnect replay and conversation switches (`__tests__/contexts/conversation-websocket-context.test.tsx:792-879`). Interruption is first-class (immediate `/interrupt` locally, sandbox pause on cloud, goal stop+interrupt chaining). However, the dimension's core subject — *partial output from a long-running tool* — is absent for native tools (single all-or-nothing observation events), was consciously removed for ACP tools (`should-render-event.ts:132-134`), and partial outputs are by design non-durable (deltas are transient and never persisted). That inconsistency, plus the fact that mid-execution feedback for long shell commands degrades to a spinner chip, keeps this below the "clear model across the board" band.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| WS event transport | Per-conversation WebSocket provider dispatches every event through `handleMainMessage`; separate planning-agent socket | `src/contexts/conversation-websocket-context.tsx:121-180`, `538-758` |
| Delta resume protocol | REST-first history then `resend_mode='since'` + `after_timestamp` so only post-anchor events re-stream; falls back to `resend_mode='all'` | `src/contexts/conversation-websocket-context.tsx:359-400`, `964-973` |
| Token stream enablement | `llm.stream = true` forced at conversation start ("parity with ACP agents"); re-forced after `/switch_profile` | `src/api/agent-server-adapter.ts:895-897`; `src/api/conversation-service/agent-server-conversation-service.api.ts:904-906` |
| Delta event shape | `StreamingDeltaEvent { content, reasoning_content }`, source `"agent"` | `src/types/agent-server/core/events/streaming-delta-event.ts:3-8` |
| Progress batching | Animation-frame batcher coalesces deltas; `flush()` before any non-delta event so durable events can't overtake streamed text; `reset()` drops buffer on switch | `src/utils/streaming-delta-batcher.ts:30-75`; wired at `src/contexts/conversation-websocket-context.tsx:162-180`, `548-554`, `520-529` |
| In-place merge | Adjacent same-sender deltas fold into one accumulating event; deltas excluded from `eventIds` dedup set | `src/stores/use-event-store.ts:92-129` |
| Final-event reconciliation | `finalizeStreamingDeltasInPlace` supersedes streamed deltas with canonical MessageEvent/FinishAction; thought duplication fix (#1534) strips streamed text when ActionEvent repeats it | `src/utils/handle-event-for-ui.ts:225-250`, `267-301`, `387-402` |
| Sender scoping | Main vs planning agent deltas never merge (#1656) despite shared store | `src/utils/handle-event-for-ui.ts:31-37`, `84-98` |
| Tool-call lifecycle events | `ACPToolCallStatus = pending \| in_progress \| completed \| failed`; two persisted events per `tool_call_id`; UI replaces started card by `tool_call_id` | `src/types/agent-server/core/events/acp-tool-call-event.ts:10-21`, `52-66`; `src/utils/handle-event-for-ui.ts:341-346`, `404-416` |
| Removed per-frame output fan-out | Old terminal-only gate existed because source fanned out cumulative output per `ToolCallProgress`, flashing half-formed cards; fan-out removed | `src/components/conversation-events/chat/event-content-helpers/should-render-event.ts:124-137` |
| Running-card semantics | Non-terminal ACP statuses map to `undefined` result → card shows as "running" (no check mark) | `src/components/conversation-events/chat/event-content-helpers/get-observation-result.ts:6-20` |
| Bash output delivery | Whole observation appended once on arrival: content blocks joined → `appendOutput`; command echoed via `appendInput` on the action | `src/contexts/conversation-websocket-context.tsx:657-670`; store at `src/stores/command-store.ts:15-25` |
| Terminal rendering | Read-only xterm, scrollback 10000, incremental index-based redraw from command store (whole-block writes, no char-level streaming) | `src/hooks/use-terminal.ts:100-115`, `174-190` |
| Model-driven partial-output polling | `ExecuteBashAction.command`: empty string views additional logs when exit code `-1`; `C-c` interrupts running process; `timeout` triggers continue-or-stop | `src/types/agent-server/core/base/action.ts:27-44` |
| Goal progress stream | `GoalStatus {active, status, iteration, max_iterations, verdict}` streamed per lifecycle point as `ConversationStateUpdateEvent key:"goal"` | `src/types/agent-server/core/events/conversation-state-event.ts:77-97`, `140-144` |
| Goal UI + cancellation | Live banner/inline row with iteration + verdict; Stop chains `stopGoal` + `pauseConversation` because backend leaves in-flight turn running | `src/components/features/chat/goal-status-content.tsx:29-45`, `87-95` |
| Cancellation | Local mode uses `/interrupt` so in-flight LLM requests cancel immediately; cloud pauses sandbox (waits for current call); max-iterations error auto-pauses | `src/hooks/mutation/conversation-mutation-utils.ts:36-61`; `src/hooks/use-handle-ws-events.ts:55-61` |
| Socket resilience | Reconnect backoff 1s→30s with ≤30% jitter; handshake watchdog aborts stuck CONNECTING sockets; replaced-socket close suppression | `src/hooks/use-websocket.ts:18-31`, `61-64`, `101-139` |
| Replay idempotency | Reconnect backlog deduped by id; non-idempotent side-effects skipped for replayed events (#1656) | `src/contexts/conversation-websocket-context.tsx:557-568` |
| Transient deltas | Deltas are "never persisted/resent"; excluded from `eventIds` to avoid O(n²) Set copies per token | `src/stores/use-event-store.ts:94-97`, `170-173` |
| Live activity chip | `deriveLiveActivity` names the currently-running action or `in_progress` ACP call; `aria-live="polite"` status role | `src/components/features/chat/typing-indicator.tsx:28-50`, `70-161` |
| Browser partial updates | Each `BrowserObservation` replaces screenshot in browser store (last-frame-wins visual update) | `src/contexts/conversation-websocket-context.tsx:672-681` |
| Export marking | Transcript export splits streamed reasoning with `{ streaming: true }` flag | `src/utils/transcript-export/index.ts:370-380` |
| Tests — batching | Byte-exact ordering across 5000 deltas faster than 60Hz; flush/reset semantics; store integration test proving single reconciled bubble and `eventIds.size === 2` | `__tests__/utils/streaming-delta-batcher.test.ts:53-172`, `175-238` |
| Tests — provider | Buffers deltas then flushes before final message; stale buffered delta from previous conversation discarded on switch | `__tests__/contexts/conversation-websocket-context.test.tsx:792-831`, `833-879` |
| Tests — reconciliation | Extensive suite: finalize-in-place, thought supersede, reasoning-only deltas, cross-sender isolation | `__tests__/utils/handle-event-for-ui.test.ts:295-1000+` |

## Answers to Dimension Questions

**1. Can tools stream progress?**
Partially. Three tiers: (a) assistant tokens/reasoning stream continuously as `StreamingDeltaEvent`s over WebSocket, enabled by forcing `llm.stream = true` (`src/api/agent-server-adapter.ts:895-897`); (b) ACP sub-agent tool calls stream a two-phase lifecycle (started→terminal) rendered as a live-updating card (`src/utils/handle-event-for-ui.ts:404-416`); (c) `/goal` loops stream structured per-round progress (`src/types/agent-server/core/events/conversation-state-event.ts:78-81`). But native tools (bash/file editor/browser) emit **no incremental output**: their results arrive as single complete ObservationEvents (`src/contexts/conversation-websocket-context.tsx:663-681`). Mid-flight ACP output frames were deliberately removed as a UX hazard (`should-render-event.ts:132-134`).

**2. Are partial outputs durable?**
No — by explicit design. Streaming deltas are transient: the store comments they are "never persisted/resent" and skips id-tracking for them (`src/stores/use-event-store.ts:94-97`); a page reload during streaming loses the partial bubble until the durable final event lands. Durable outputs (observations, messages) are persisted server-side and re-obtainable via REST-first history plus `resend_mode='since'` WS replay (`src/contexts/conversation-websocket-context.tsx:278-282`, `964-973`). Buffered-but-uncommitted deltas are intentionally dropped on conversation switch (`src/contexts/conversation-websocket-context.tsx:520-529`, tested at `__tests__/contexts/conversation-websocket-context.test.tsx:833-879`).

**3. Does the model act on partial output?**
Not determinable inside this repository — whether partial tool output is fed back into the LLM context is decided by `software-agent-sdk` (out of boundary; searched `src/` for any client-side injection of partial observations into follow-up requests and found none, which is consistent with the client being purely a display consumer). What the repo does show: the tool schema gives the *model* an explicit polling mechanism for long-running commands — an empty `command` fetches additional logs when exit code is `-1`, and `C-c` interrupts (`src/types/agent-server/core/base/action.ts:29-39`) — implying the model regains agency over partial output by re-invoking the tool rather than receiving pushed chunks.

**4. Can users interrupt?**
Yes, with mode-dependent semantics. Local conversations use `/interrupt` specifically so "in-flight LLM requests are cancelled immediately rather than waiting" (`src/hooks/mutation/conversation-mutation-utils.ts:55-60`); cloud pauses the sandbox after the current LLM call (`:38-51`). Goal loops have a dedicated Stop that must chain loop-cancel + conversation-interrupt because the backend's stop "deliberately leaves the in-flight agent turn running" (`goal-status-content.tsx:87-93`; `conversation-mutation-utils.ts:94-99`). At the transport layer, users are protected from hangs by exponential-backoff reconnects and a handshake watchdog that aborts stuck CONNECTING sockets (`src/hooks/use-websocket.ts:61-64`, `110-139`).

**5. Are partial outputs clearly marked?**
Yes for what streams. Provisional streamed text renders as a normal agent bubble plus collapsible thinking, then is atomically superseded by the canonical final event so nothing renders twice (`event-message.tsx:188-209`; `handle-event-for-ui.ts:225-250`). Running tool calls are marked by absence-of-result styling (no success check) plus a live activity chip naming the exact running action/tool (`get-observation-result.ts:9-12`; `typing-indicator.tsx:130-160`). Goal progress rows show iteration counts and judge verdicts inline. Exported transcripts split streamed reasoning with a `{ streaming: true }` marker (`transcript-export/index.ts:370-374`). Unstreamed partial command output is inherently unmarked — there is simply none displayed until completion.

## Architectural Decisions

1. **Single ordered event stream as the only substrate.** All tool/token/state traffic shares one JSON WebSocket per conversation with typed discriminators (`kind`) guarded by type guards (`src/types/agent-server/type-guards.ts:278-281`); there is no side-channel for tool stdout. This makes ordering, dedup, and replay uniform but caps throughput granularity at whole events.
2. **REST-first history, delta-replay socket.** The socket waits for the initial REST page and subscribes with `resend_mode='since'` anchored to the last preloaded timestamp, avoiding full re-streaming while keeping overlap deduped (`src/contexts/conversation-websocket-context.tsx:278-282`, `375-400`).
3. **Render-side coalescing instead of wire throttling.** The server emits per-token events; the client batches commits to ≤1 render per animation frame and flushes synchronously before any non-delta event to preserve causal order (`src/utils/streaming-delta-batcher.ts:30-35`, `50-60`). Transport simplicity is traded for client-side buffering complexity.
4. **Provisional-then-canonical reconciliation.** Streamed deltas are disposable previews; the final MessageEvent/ActionEvent is authoritative and replaces them in place, keeping persistence clean (deltas never stored) and rendering duplicate-free (`src/utils/handle-event-for-ui.ts:225-250`).
5. **Two-event tool-call lifecycle over continuous progress frames.** For ACP tools, the SDK persists exactly one `started` and one terminal event per call; earlier per-progress-frame cumulative output was removed because half-formed cards flashed mid-stream (`should-render-event.ts:124-134`). Stability was chosen over richer live output.
6. **Model-driven polling for long commands.** Rather than pushing partial logs, the `ExecuteBashAction` contract invites the model to re-poll (empty command) or interrupt (`C-c`) (`src/types/agent-server/core/base/action.ts:29-39`).

## Notable Patterns

- **Sender-scoped merging**: because main and planning sockets feed one global store, delta merges and reconciliation are scoped by an `isFromPlanningAgent` flag to prevent cross-agent text bleed (regression #1656) (`src/utils/handle-event-for-ui.ts:31-37`, `120-144`).
- **Idempotent replay guards**: WS reconnects replay from a possibly stale anchor; the handler checks `eventIds` before applying non-idempotent side-effects like optimistic-message consumption (`src/contexts/conversation-websocket-context.tsx:556-568`).
- **Ordering invariant enforcement at the batcher boundary**: "Callers MUST `flush()` before any non-delta event so a durable message/action can't render ahead of its own streamed text" (`src/utils/streaming-delta-batcher.ts:33-34`), enforced at both handlers (`conversation-websocket-context.tsx:553-554`, `785-786`).
- **Tolerant stream matching**: reconciliation trims whitespace variants and tolerates unstripped `<function=` XML markers in streamed tool-call prompts (`src/utils/handle-event-for-ui.ts:171-204`).
- **Test-seamable timing**: the batcher accepts an injectable `DeltaFlushScheduler`, enabling deterministic frame control in tests (`src/utils/streaming-delta-batcher.ts:5-19`; used in `__tests__/utils/streaming-delta-batcher.test.ts:28-51`).

## Tradeoffs

- **Whole-event granularity vs liveness**: users get correct, dedupable, resumable output, but a multi-minute build shows nothing in the terminal tab until it exits — only the typing chip indicates progress.
- **Transient deltas vs durability**: skipping persistence for deltas avoids O(n²) id-set copying per token (`use-event-store.ts:94-97`) and keeps the server log clean, but means a reload mid-stream drops partial text until the durable event arrives, and buffered deltas are discarded on navigation.
- **Client-side batching vs server-side throttling**: the wire carries one event per token (simple server), paid for by client buffering logic plus the strict flush-ordering discipline every handler must maintain.
- **Stability over richness for ACP**: removing per-progress cumulative frames eliminated flashing/half-formed cards but also eliminated live tool output for external agents entirely (`should-render-event.ts:132-134`).

## Failure Modes / Edge Cases

- **Reconnect replay storms**: a stale `after_timestamp` anchor replays the backlog; mitigated by id-based store dedup plus side-effect skip guards (#1656) (`conversation-websocket-context.tsx:556-568`).
- **Cross-conversation delta leakage**: deltas buffered for conversation A would merge into B's stream after a switch; fixed by `reset()` on switch and covered by test (`conversation-websocket-context.tsx:520-529`; test at `conversation-websocket-context.test.tsx:856-878`).
- **Deltas faster than frames**: thousands of tokens between frames could starve renders; the batcher coalesces per frame and a stress test proves byte-exact reconstruction at 5000 deltas/60 frames (`streaming-delta-batcher.test.ts:145-172`).
- **Durable event overtaking its own stream**: without the pre-event flush, a final message could render before the text it supersedes; enforced and tested (`conversation-websocket-context.test.tsx:808-817`).
- **Duplicated streamed thought**: models repeating streamed text as `thought` caused double rendering (#1534); reconciliation matches segments in order and clears the delta (`handle-event-for-ui.ts:267-301`).
- **Socket handshake lock**: a CONNECTING socket stuck against Chrome's per-host lock holds the pipeline; the watchdog aborts it into the normal close/reconnect path (`use-websocket.ts:61-64`).
- **Timeout ambiguity**: bash timeout maps to a distinct `timeout` result status (`exit_code === -1`) rather than generic error (`get-observation-result.ts:29-35`), letting the UI ask continue-or-stop per the action contract (`base/action.ts:37-39`).

## Future Considerations

- **Incremental output channel for native tools**: a chunked `TerminalObservation`-style append event (mirroring `appendOutput`'s existing whole-block interface at `command-store.ts:21-24`) would let long commands render progressively using the same batching/reconciliation machinery already proven for tokens.
- **Durable partial snapshots**: persisting periodic cumulative output (server-side) would make reload-mid-command safe, complementing the model-driven empty-command polling in `base/action.ts:29`.
- **Unify planning-agent history loading** on the REST-then-`since` pattern; it still uses `resend_all=true` with count-based completion detection (`conversation-websocket-context.tsx:185-199`, `1004-1044`), which is the fragile legacy path.
- **Progress metadata on native actions**: an optional `progress` field on long-running actions would let the activity chip show percentages instead of titles only (`typing-indicator.tsx:70-124`).

## Questions / Gaps

- **Does the model see partial tool output mid-execution?** No evidence found in this source. Search boundary: all of `src/` (types, api adapters, services, hooks) for partial/incremental observation handling; the agent loop and context assembly live in the sibling `software-agent-sdk` repo, which is outside the isolation rule for this study. The empty-command polling contract (`src/types/agent-server/core/base/action.ts:29`) suggests polling-by-design, but confirmation requires the SDK repo.
- **Server emission cadence for `StreamingDeltaEvent`**: the frontend forces `llm.stream = true` (`agent-server-adapter.ts:895-897`), but whether the server coalesces chunks before emission is not observable here.
- **ACP `content` blocks during progress**: the event type allows mixed content blocks (`acp-tool-call-event.ts:87-89`), but with only two persisted lifecycle events, whether intermediate content is ever populated cannot be verified from this repo.
- **No evidence found** of any numeric/fractional progress reporting for native tools (searched for `progress`, `percent`, `progress_callback` patterns in `src/`); only goal loops expose iteration counts (`conversation-state-event.ts:86-88`).

---

Generated by `07.07-tool-output-streaming` against `openhands`.

# Source Analysis: openhands

## Dimension 07.02: Sequential vs Parallel Tool Execution

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (agent-canvas) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React (Vite), Zustand stores, WebSocket + REST against a separate Python agent-server (`software-agent-sdk`) |
| Analyzed | 2026-08-26 |

## Summary

This repository is **only the browser frontend** of the OpenHands multi-repo system; `AGENTS.md` states explicitly that agent/tool behaviour lives in the sibling Python `software-agent-sdk` repo, and that tool calls (terminal, file_editor, browser) "execute here" (i.e., in the agent-server sandbox), not in this codebase. Consequently, there is **no tool executor in this source at all**: no scheduler loop over actions, no semaphores, no worker pool. All `Promise.all`/`Promise.allSettled` call sites in `src/` are UI-level work (file uploads, config fetches), never agent-action orchestration.

What this source *does* own is the protocol and UX layer that makes server-side sequential-vs-parallel execution tractable and observable:

1. **Protocol modeling of parallel function calling**: every `ActionEvent` carries a per-call `tool_call_id`, plus an `llm_response_id` documented as grouping "results of parallel function calling from the same LLM response" (`src/types/agent-server/core/events/action-event.ts:40-56`). Observations carry `action_id`/`tool_call_id` back-links (`src/types/agent-server/core/events/observation-event.ts:23,38`).
2. **The user-facing concurrency knob**: `tool_concurrency_limit`, described as "Maximum number of tool calls to execute concurrently per agent step. 1 = sequential (default)" — surfaced in the Agent settings UI only when the backend schema exposes it (`src/routes/agent-settings.tsx:48,264-277,525-541`), with min=1/step=1 constraints mirroring the SDK's `ge=1` int (`src/utils/sdk-settings-field-metadata.ts:40-49`).
3. **Side-effect gating via risk metadata rather than read-only flags**: a `SecurityRisk` enum (UNKNOWN/LOW/MEDIUM/HIGH) attached to each action (`src/types/agent-server/core/base/common.ts:59-64`; `action-event.ts:59-61`), translated into confirmation policies (`NeverConfirm` / `ConfirmRisky` threshold HIGH / `AlwaysConfirm`) sent at conversation start (`src/api/agent-server-adapter.ts:593-618,1120-1121`), with `ExecutionStatus.WAITING_FOR_CONFIRMATION` (`common.ts:67-75`) and a `respond_to_confirmation` REST endpoint.
4. **Deterministic ordering machinery**: a timestamped, ULID-keyed event log preserved by the frontend store with O(1) dedup, out-of-order detection, and full re-sort (`src/stores/use-event-store.ts:41-53,99-107,131-151`); observations replace their action card in place by matching `uiEvent.id === event.action_id` (`src/utils/handle-event-for-ui.ts:432-441`).
5. **Failure isolation for the two browser-executed client tools** (`canvas_ui`, `launch_child_conversation`, dispatched at `src/contexts/conversation-websocket-context.tsx:724-740`): the launch handler is documented "Never rejects: every failure is turned into corrective guidance for the agent" and is guarded by a localStorage idempotency ledger keyed on `tool_call_id` (`src/services/child-conversation-launch.ts:196-227,499-536`).

The verdict on the dimension question ("Does concurrency improve latency without risking corruption?") is **deferred by design to the backend**: the frontend enforces nothing about concurrency itself but provides explicit identifiers, a validated user control defaulting to sequential, deterministic ordering, and replay-safe idempotency for the tools it does execute locally.

## Rating

**6 / 10 — Present but partially out of scope; clear model with tests where this source owns the problem.**

Rationale:
- The parallelism *model* is explicit and well-documented at the protocol level (`llm_response_id`, `tool_call_id`, `action_id`), and the concurrency limit is a first-class, schema-driven setting with save-time validation and dedicated unit tests (`__tests__/routes/build-agent-profile-fields.test.ts:96-140`, `__tests__/routes/agent-settings.test.tsx:156-187`) — that alone satisfies much of the 7–8 band.
- However, the actual executor, serialization of write tools, resource-conflict detection, and failure aggregation across concurrent actions all live in `software-agent-sdk`, outside this source's boundary. Within this source they are unverifiable ("No evidence found" in-repo; the search boundary is stated below). The frontend compensates operationally where it executes tools itself (idempotency ledger, never-reject handlers, per-message error isolation), which is genuinely strong engineering, but it cannot prove the core latency-vs-corruption tradeoff from here.
- Not rated higher because read/write side-effect classification is coarse (a 4-level risk enum, no per-tool `read_only` flag in native action types), and because enforcement being remote means the safeguard chain cannot be observed end-to-end in this repository.

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Parallel-call grouping ID | `llm_response_id`: "Groups related actions from same LLM response … tracking and managing results of parallel function calling" | `src/types/agent-server/core/events/action-event.ts:51-56` |
| Per-tool-call identity | `tool_call_id` on ActionEvent; verbatim LLM `tool_call` kept for message reconstruction | `src/types/agent-server/core/events/action-event.ts:38-49` |
| Raw LLM tool-call shape | `ChatCompletionMessageToolCall {id, function.name, function.arguments}` | `src/types/agent-server/core/base/event.ts:41-48` |
| Result-to-action linkage | ObservationEvent `tool_call_id` (:23), `action_id` (:38), `UserRejectObservation.rejection_reason`+`action_id` (:42-52) | `src/types/agent-server/core/events/observation-event.ts:23,38,42-52` |
| Concurrency setting key | `TOOL_CONCURRENCY_FIELD_KEY = "tool_concurrency_limit"`; surfaced only when backend schema exposes it | `src/routes/agent-settings.tsx:48,264-269` |
| Concurrency semantics doc | Mock schema description: "Maximum number of tool calls to execute concurrently per agent step. 1 = sequential (default)", integer, default 1 | `src/mocks/settings-handlers.ts:92-105` |
| Input constraints mirror SDK | `min: 1, step: 1` — "Mirrors the SDK's ``tool_concurrency_limit`` constraint (``int`` with ``ge=1``)" | `src/utils/sdk-settings-field-metadata.ts:40-49` |
| Save-time coercion/validation | Schema-driven `coerceFieldValue`, throws on bad input; non-nullable int, empty input skipped not nulled | `src/routes/agent-settings.tsx:525-541` |
| Profile-save reset semantics | Blank field emits schema default (not omitted) so whole-profile merge can't silently retain stale value (#1571 review) | `src/routes/agent-settings.tsx:133-188` |
| Side-effect/risk metadata | `SecurityRisk` enum UNKNOWN/LOW/MEDIUM/HIGH; `security_risk` attached per action | `src/types/agent-server/core/base/common.ts:59-64`; `src/types/agent-server/core/events/action-event.ts:59-61` |
| Confirmation policy translation | `NeverConfirm` / `ConfirmRisky{threshold:"HIGH", confirm_unknown:true}` / `AlwaysConfirm` sent as `confirmation_policy` in start payload | `src/api/agent-server-adapter.ts:593-605,1120-1121` |
| Security analyzer selection | LLM / Pattern / PolicyRail analyzers mapped from settings | `src/api/agent-server-adapter.ts:607-617` |
| Confirmation state machine | `ExecutionStatus.WAITING_FOR_CONFIRMATION` among idle/running/paused/finished/error/stuck | `src/types/agent-server/core/base/common.ts:67-75` |
| Confirmation response endpoint | POST `/api/conversations/{id}/events/respond_to_confirmation` with `{accept, reason?}` | `src/api/event-service/event-service.api.ts:29-69` |
| No local executor | All `Promise.all(Settled)` sites are uploads/config/UI, never action orchestration (verified by grep across `src/`) | `src/utils/file-processing.ts:63,90`; `src/api/conversation-file-upload.api.ts:161`; `src/components/features/chat/chat-interface.tsx:350` |
| Event ingestion loop | Single WS message → type-guard dispatch chain; one event per frame, already executed server-side | `src/contexts/conversation-websocket-context.tsx:538-758` |
| Per-message failure isolation | Each WS frame parsed in its own try/catch; malformed frames warn and never halt the stream | `src/contexts/conversation-websocket-context.tsx:742-744` |
| Deterministic ordering store | Out-of-order detection (`needsSorting`), O(1) dedup by event id, timestamp re-sort on add/bulk-add | `src/stores/use-event-store.ts:41-53,99-107,131-151` |
| In-place result replacement | Observation replaces its action via `uiEvent.id === event.action_id`; ACP events merge by `tool_call_id` | `src/utils/handle-event-for-ui.ts:404-416,432-441` |
| Ordering tests | `handleEventForUI` replaces correct action when multiple exist; ACP dedup appends/replaces by `tool_call_id` | `__tests__/utils/handle-event-for-ui.test.ts:174-192,240-270` |
| Store tests | `use-event-store.test.ts` covers add/dedup/sort behaviour | `__tests__/stores/use-event-store.test.ts` |
| Client tool #1 (UI-only) | `canvas_ui` handled locally after immediate server ack — pure tab/file navigation | `src/services/canvas-ui.ts:32-64`; dispatch `src/contexts/conversation-websocket-context.tsx:724-729` |
| Client tool #2 (async work) | `launch_child_conversation` — real network work in browser, result posted back as user message | `src/services/child-conversation-launch.ts:459-497,505-536`; dispatch :731-740 |
| Idempotency ledger (replay lock) | `claimToolCall()` claims `toolCallId` in localStorage before any network work so replays can't double-launch (Cloud = billable) | `src/services/child-conversation-launch.ts:196-227` |
| Failure containment contract | "Never rejects: every failure is turned into corrective guidance for the agent, because the agent-server has already told it the call succeeded" | `src/services/child-conversation-launch.ts:499-536` |
| Worktree conflict avoidance | Child conversations default to git-worktree isolation so siblings don't collide over files; falls back to shared dir with explicit consequence note | `src/services/child-conversation-launch.ts:252-270,300-323` |
| FIFO queue (user commands) | Three-stage queue refs waiting→pending→active keyed by server-assigned `command_id`; `rejectAll()` valve on socket failure | `src/hooks/use-bash-command-runner.ts:68-72,114-128,141-149` |
| Parallel-call UI acknowledgment | Consecutive actions folded into collapsible groups, `EVENT_GROUP_MIN_SIZE = 2` | `src/components/conversation-events/chat/group-events.ts:13,77-79` |
| Concurrency setting tests | Coercion to number, blank→default, custom default, throws on non-numeric | `__tests__/routes/build-agent-profile-fields.test.ts:89-140` |
| Settings save test | Changing the input persists `agent_settings_diff.tool_concurrency_limit = 4` | `__tests__/routes/agent-settings.test.tsx:156-187` |

## Answers to Dimension Questions

**1. Can multiple tools run in one turn?**
Yes, at the protocol level. The event model explicitly anticipates several actions sharing one LLM response: each carries its own `tool_call_id` while sharing an `llm_response_id` whose doc comment names "parallel function calling" (`src/types/agent-server/core/events/action-event.ts:40-56`). Whether they actually run concurrently is decided server-side by `tool_concurrency_limit`, whose schema text says "Maximum number of tool calls to execute concurrently per agent step. 1 = sequential (default)" (`src/mocks/settings-handlers.ts:92-105`). Default is therefore sequential. The frontend consumes whatever order of ActionEvents arrives, one per WS frame (`src/contexts/conversation-websocket-context.tsx:538-758`).

**2. Which tools are safe to parallelize?**
No evidence found for a per-tool read-only/safe-to-parallelize declaration in this source. Native action types (`src/types/agent-server/core/base/action.ts`) expose no `read_only` flag; side-effect information exists only as a coarse `SecurityRisk` level per action (`common.ts:59-64`; `action-event.ts:59-61`) used for confirmation gating, and ACP sub-agent display kinds `execute|edit|read|fetch|other` (`acp-tool-call-event.ts:8`), which classify for rendering, not scheduling. The closest safety mechanism is structural: child-conversation launches isolate siblings in git worktrees so parallel child agents can't corrupt each other's files, with a documented fallback consequence when shared (`src/services/child-conversation-launch.ts:252-270,300-323`).

**3. Are write tools serialized?**
Not enforced anywhere in this source — enforcement belongs to the backend executor behind `tool_concurrency_limit` (default 1 = sequential, i.e., fully serialized unless the user raises it: `src/mocks/settings-handlers.ts:95-99`, constraint `min:1` at `src/utils/sdk-settings-field-metadata.ts:44-49`). The frontend's only serialization-adjacent safeguards apply to its two locally executed tools: the canvas_ui tool mutates UI stores synchronously (single-threaded, trivially serialized), and the launch tool claims its `tool_call_id` in an idempotency ledger *before* any network work so replays during reconnect cannot double-execute a non-idempotent, potentially billable write (`claimToolCall`, `src/services/child-conversation-launch.ts:196-227`).

**4. How are failures aggregated?**
Per-action isolation rather than aggregation. Each WS frame is independently try/caught so one malformed or failing event never halts the stream (`src/contexts/conversation-websocket-context.tsx:742-744`); a failing action surfaces as its own inline error event while other pending actions keep flowing. For the browser-executed launch tool, failures are deliberately *converted into data*: the handler "never rejects" and instead posts structured `{status:"error", error, guidance}` JSON back to the agent as a follow-up user message, because the server already ACKed the tool call and has no return channel otherwise (`reportLaunchResult`/`handleLaunchChildConversationAction`, `src/services/child-conversation-launch.ts:459-497,499-536`). Rejected risky actions produce `UserRejectObservation` with `rejection_reason` (`observation-event.ts:42-52`).

**5. Is result order deterministic?**
Yes, within this source's responsibilities. Events arrive on a server-authored, globally ordered log where every event has a ULID `id` and ISO `timestamp` (`src/types/agent-server/core/base/event.ts:10-25`); observations link to exactly one action via `action_id`/`tool_call_id` (`observation-event.ts:23,38`), which is precisely the mechanism a backend needs to reassemble ordered tool results for the LLM. The frontend hardens this: the store detects out-of-order arrival and re-sorts by timestamp (`src/stores/use-event-store.ts:41-53,131-151`), dedups replays by id (:99-107), reconnects use `resend_mode:'since'` anchored to the last preloaded REST event, buffered streaming deltas are force-flushed before later non-delta events so tokens can't be overtaken (`conversation-websocket-context.tsx:966-976,548-554`), and observations replace their action card *in place* keyed by `action_id` so UI position reflects execution identity, not arrival luck (`src/utils/handle-event-for-ui.ts:432-441`, tested at `__tests__/utils/handle-event-for-ui.test.ts:174-192`).

## Architectural Decisions

1. **Executor out of the browser.** All agent tool execution was pushed into the Python agent-server; the frontend is an event-log renderer plus settings surface (`AGENTS.md` repo map; start payload sends tools + `confirmation_policy` + `conversation_settings` at `src/api/agent-server-adapter.ts:1008-1130`). This makes sequential-vs-parallel a backend policy with a frontend control, at the cost of this repo being unable to guarantee anything about actual concurrency.
2. **Group-by-response identifiers instead of batch envelopes.** Rather than a single "multi-action" event, each tool call becomes its own `ActionEvent` tied together by `llm_response_id` and distinguished by `tool_call_id` (`action-event.ts:40-56`). This keeps streaming incremental (each action can complete independently) while preserving the logical batch.
3. **Risk-based confirmation instead of read/write classification.** Safety gating uses a per-action `SecurityRisk` assessment (optionally LLM-predicted, kept verbatim on `tool_call` per the comment at `action-event.ts:42-49`) driving `ConfirmRisky`/`AlwaysConfirm` policies (`agent-server-adapter.ts:593-618`), rather than declaring which tools are side-effect-free.
4. **Client tools compensate for fire-and-forget ACKs.** Because the server acknowledges client-defined tools immediately, the launch handler must both report results through an out-of-band user message and be total (never throw) — an explicit architectural rule documented at `src/services/child-conversation-launch.ts:455-503`.
5. **Idempotency at the edge, not in a queue.** Replay protection is a per-conversation localStorage ledger claimed before network work (`child-conversation-launch.ts:205-227`) rather than a centralized command queue — appropriate for a single-tab UI, weaker if multiple tabs share state.

## Notable Patterns

- **Schema-driven settings with mirrored constraints**: the UI derives the concurrency field from the backend's `agent_settings_schema`, layers local min/step metadata that "mirrors the SDK's ``ge=1``", and reuses schema coercion for save-time validation (`src/routes/agent-settings.tsx:525-541`; `sdk-settings-field-metadata.ts:40-49`) — single source of truth stays backend-side, with graceful hiding on older servers (`agent-settings.tsx:264-266`).
- **In-place event reconciliation**: observations overwrite their action slot by id, and ACP status updates replace earlier frames by `tool_call_id` (`handle-event-for-ui.ts:404-441`) — the UI models "one slot per tool call", mirroring how the model expects one result per call.
- **Failure-as-guidance**: errors from client-executed tools are packaged with corrective instructions for the agent ("retry only if the cause looks transient") instead of being swallowed or thrown (`child-conversation-launch.ts:522-527`).
- **Graceful degradation with stated consequences**: worktree-isolation failures fall back to a shared directory and explicitly warn that the agents "may conflict over the same files" (`child-conversation-launch.ts:269-270`) — an honest statement of the corruption risk the isolation normally prevents.
- **UI-level acknowledgment of parallelism**: chat groups consecutive action cards into collapsible runs once ≥2 stack up (`group-events.ts:13,77-79`), a direct UX response to multi-tool turns.

## Tradeoffs

- **Remote execution buys simplicity, loses enforceability here.** The frontend can't verify that `tool_concurrency_limit` is honored, that writes stay serialized above the default, or that parallel results return at all; correctness rests entirely on the SDK repo. Conversely, the browser never holds locks that could deadlock.
- **Coarse risk enum vs. fine-grained effect metadata.** Four risk levels are cheap for users to configure (`confirmation_mode` + `security_analyzer`, `src/types/settings.ts:128-129`) but cannot express "safe to run concurrently with X"; the harness gives users a blunt sequential/concurrent dial instead.
- **Idempotency ledger vs. multi-client reality.** localStorage claiming is simple and synchronous, but two browser tabs (or a cleared storage — the code proceeds accepting replay risk when storage fails, `child-conversation-launch.ts:222-225`) can defeat it; a server-side claim would be robust but adds round-trips.
- **Timestamp re-sorting vs. strict append-only.** Re-sorting on out-of-order arrival fixes jitter but relies on clock quality of the server timestamps; ULID ids provide tie-breaking identity (`event.ts:10-19`, `use-event-store.ts:41-53`).
- **Never-reject client tools vs. silent divergence.** Converting all failures into agent-visible messages keeps the protocol consistent (the model always learns the outcome) but means UI toast and agent guidance can diverge from any server-side belief that the tool succeeded.

## Failure Modes / Edge Cases

- **Replayed ActionEvents after reconnect/reload**: mitigated by the `claimToolCall` ledger claimed mid-flight-safe (before network I/O) and dedup-by-event-id in the store (`child-conversation-launch.ts:199-227`; `use-event-store.ts:99-107`).
- **Corrupt or full localStorage**: ledger read failures restart a fresh ledger; write failures proceed accepting replay risk rather than blocking launches (`child-conversation-launch.ts:214-216,222-225`).
- **Out-of-order observation before action**: if the action isn't found in UI events, the observation is appended rather than dropped (`handle-event-for-ui.ts:438-441`, tested at `__tests__/utils/handle-event-for-ui.test.ts:146`).
- **Malformed WS frames**: caught per-message with a console warning; stream continues (`conversation-websocket-context.tsx:742-744`).
- **Stuck optimistic messages**: user-message echo matching has exact-content-first matching plus a 150 s watchdog that flips stuck bubbles to retryable errors without blocking subsequent sends (`optimistic-user-message-store.ts:131-139,169-198`).
- **Older servers lacking the concurrency field**: the field is hidden and skipped cleanly rather than sending a rejected null (`agent-settings.tsx:264-266,537-541`).
- **Unborn-HEAD workspaces**: `git worktree add` would 500, so scratch workspaces are detected up-front and downgraded to shared isolation with a warning (`child-conversation-launch.ts:252-267,300-304`).

## Future Considerations

- Surface execution-mode feedback in the UI (e.g., which actions of one `llm_response_id` ran concurrently vs sequentially) — the grouping IDs already exist (`action-event.ts:51-56`), so observability is one rendering pass away.
- Add per-tool effect classification (read-only/write) to complement the risk enum, enabling smarter defaults than a global concurrency integer (`common.ts:59-64`; `mocks/settings-handlers.ts:92-105`).
- Move client-tool idempotency claims server-side (or into IndexedDB with cross-tab locking) so multiple tabs can't double-launch billable Cloud children (`child-conversation-launch.ts:205-227`).
- Document the intended interaction between `tool_concurrency_limit > 1` and worktree/shared isolation for parallel child-conversation launches, since shared-directory fallback is the one path where concurrency could corrupt state visible to users (`child-conversation-launch.ts:269-270`).

## Questions / Gaps

- **No evidence found in this source for**: the executor that honors `tool_concurrency_limit`, semaphore/lock implementations, resource-conflict detection between concurrent tools, or cross-action failure aggregation strategy. Searched `src/` for `semaphore|mutex|lock|Promise.all|Promise.allSettled|queue|concurrency`; all hits were UI-level queues/uploads (`src/hooks/use-bash-command-runner.ts:68-72`, `src/utils/file-processing.ts:63,90`) — these answers live in the `OpenHands/software-agent-sdk` repo, which is out of this study's isolation boundary.
- Whether raising `tool_concurrency_limit` above 1 changes observation ordering guarantees (interleaving semantics) cannot be verified here; the frontend tolerates arbitrary arrival order, so it is compatible with either behavior.
- The mock schema (`settings-handlers.ts:92-105`) mirrors but does not prove the production `/api/settings/agent-schema`; live-backend wording could differ (the UI handles this defensively by trusting schema text, `agent-settings.tsx:562-575`).
- No test exercises two client-tool actions arriving interleaved in one WS burst; isolation today rests on independent try/catch plus idempotent claiming rather than tested interleavings.

---

Generated by `07.02-sequential-vs-parallel-tool-execution` against `openhands`.

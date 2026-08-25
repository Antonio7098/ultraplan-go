# Source Analysis: openhands

## Dimension 10.03: Causal Links and Lineage

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (OpenHands Agent Canvas) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19 (Vite), Zustand stores, TanStack Query; consumes the Python `software-agent-sdk` agent-server via `@openhands/typescript-client` |
| Analyzed | 2026-08-25 |

## Summary

OpenHands Canvas is a frontend harness whose lineage story is built on a **persisted, typed event stream** rather than scattered logs. Every event carries a server-assigned `id` and ISO `timestamp` (`src/types/agent-server/core/base/event.ts:10-25`), and the schema encodes causal edges explicitly: an `ObservationEvent` points back to its `action_id` and `tool_call_id` (`src/types/agent-server/core/events/observation-event.ts:9-39`); an `ActionEvent` retains the raw LLM tool call, a `tool_call_id`, a `llm_response_id` grouping parallel calls from one LLM response, the pre-action thought, and an LLM-assessed `security_risk` (`src/types/agent-server/core/events/action-event.ts:15-66`). Tool definitions are themselves lineage records — the `SystemPromptEvent` persists the system prompt plus the full OpenAI-format tool list offered to the model (`src/types/agent-server/core/events/system-event.ts:5-25`). Model-version lineage is tracked at three levels: per-usage metrics keyed by usage id (`"default"`, `"condenser"`, `"profile:<name>:<uuid>"`) with `model_name`, per-call `response_id`s and cost/latency arrays (`src/types/agent-server/core/events/conversation-state-event.ts:10-47`); mid-run profile switches recorded as `SwitchLLMAction`/`SwitchLLMObservation` events with `active_model` (`src/types/agent-server/core/base/action.ts:301-310`); and conversation-level fields `agent.llm.model`, `current_model_id/name`, and `launched_agent_profile {agent_profile_id, revision}` (`src/api/agent-server-adapter.ts:89-110`).

The strongest operational evidence that these links are real is that the frontend **reconstructs causal chains programmatically**: the transcript exporter builds an `actionsById` map from event ids and resolves every observation to its producing action to render summaries/details (`src/utils/transcript-export/index.ts:275-277`, `319-323`, `431-441`), and it refuses to export a history it cannot prove complete (cursor-cycle detection, event-count cross-check, strict-pagination rethrow; `src/utils/transcript-export/load-complete-events.ts:85-124`, `153-160`, `src/api/event-service/event-service.api.test.ts:29-40`).

The weak seams are at the approval boundary and in client-side provenance stores: the confirmation request carries only `{accept}` — no action id, and its optional `reason` is never populated (`src/api/event-service/event-service.types.ts:1-4`, `src/hooks/mutation/use-respond-to-confirmation.ts:21-23`) — while the UI identifies the "awaiting action" heuristically as the last agent-sourced event in the store (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:30-36`). Optimistic user messages are reconciled to echoed `UserMessageEvent`s by exact content string match, not by correlation id (`src/stores/optimistic-user-message-store.ts:169-198`).

**Can you trace a final answer back to the specific tool call that provided each fact?** At event granularity, yes: a `FinishAction.message` or agent `MessageEvent` can be walked backward through `ObservationEvent.action_id → ActionEvent.tool_call/tool_call_id → llm_response_id → SystemPromptEvent.tools`, and the transcript export demonstrates this reconstruction end-to-end. At fact granularity (per-sentence citations inside prose answers), no — no mechanism in this repo links individual claims to their source observations.

## Rating

**7 / 10** — Clear lineage model with explicit interfaces, tests, and operational safeguards.

- **Earns 7–8 band traits**: explicit id-based causal edges in the wire schema (`action_id`, `tool_call_id`, `llm_response_id`, `forgotten_event_ids`, hook `action_id`/`message_id`); dedup-by-id before side effects (`src/contexts/conversation-websocket-context.tsx:556-568`); completeness-proving transcript export with dedicated unit tests (`src/utils/transcript-export/load-complete-events.test.ts`, `src/utils/transcript-export/index.test.ts:91` uses `llm_response_id` in fixtures); strict-pagination contract tested in `src/api/event-service/event-service.api.test.ts:24-55`.
- **Kept from 9–10**: approval linkage is heuristic and id-less; user-message↔echo correlation is content-based; several lineage artifacts (conversation metadata incl. `active_profile`, child-launch ledger) live in browser `localStorage` (`src/api/conversation-metadata-store.ts:4`, `src/services/child-conversation-launch.ts:40`) and are neither durable nor auditable across clients; interactive history pagination can silently degrade on unpatched cloud backends (`src/api/event-service/event-service.api.ts:149-163`) even though exports do not.

## Evidence Collected

Every entry cites `sources/openhands/<path>:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Event identity & ordering | All events carry unique `id` + ISO `timestamp` + `source` ("agent"/"user"/"environment"/"hook") | `src/types/agent-server/core/base/event.ts:10-25`, `src/types/agent-server/core/base/common.ts:51-56` |
| Output→input: observation→action | `ObservationEvent.action_id` ("The action id that this observation is responding to") + `tool_call_id`/`tool_name` | `src/types/agent-server/core/events/observation-event.ts:9-39` |
| Output→input: raw LLM call retained | `ActionEvent.tool_call` keeps the verbatim LLM tool call (name + JSON arguments string) alongside the parsed `action`; doc explains divergence (e.g. LLM-predicted `security_risk`) | `src/types/agent-server/core/events/action-event.ts:33-49`, `src/types/agent-server/core/base/event.ts:41-48` |
| Parallel-call grouping | `llm_response_id` groups actions from the same LLM response | `src/types/agent-server/core/events/action-event.ts:51-56` |
| Prompt/tool-definition persistence | `SystemPromptEvent` stores system prompt text, full `ChatCompletionToolParam[]` list, and optional runtime-injected `dynamic_context` | `src/types/agent-server/core/events/system-event.ts:5-25` |
| Context provenance (skills) | User `MessageEvent` records `activated_skills[]` and `extended_content[]` added by agent context; UI derives a synthetic "Skill Ready" event with id `${userEvent.id}-skill-ready` | `src/types/agent-server/core/events/message-event.ts:5-19`, `src/components/conversation-events/chat/event-content-helpers/create-skill-ready-event.ts:35-59` |
| Memory lineage (condensation) | `CondensationEvent.forgotten_event_ids` lists exactly which events left the LLM view, with summary + insertion offset | `src/types/agent-server/core/events/condensation-event.ts:5-27` |
| Hook observability | `HookExecutionEvent` links to `action_id` (tool hooks) or `message_id` (prompt hooks) and records command, exit code, stdout/stderr, `hook_input` | `src/types/agent-server/core/events/hook-execution-event.ts:20-99` |
| Approval→action link (server side) | `UserRejectObservation.action_id` + `rejection_reason`; execution status `WAITING_FOR_CONFIRMATION` gates the flow | `src/types/agent-server/core/events/observation-event.ts:42-52`, `src/types/agent-server/core/base/common.ts:71` |
| Approval→action link (client gap) | Confirmation POST body is `{accept, reason?}` with no event/action reference; `reason` never set by caller | `src/api/event-service/event-service.types.ts:1-4`, `src/hooks/mutation/use-respond-to-confirmation.ts:21-23`, endpoint path `src/api/event-service/event-service.api.ts:40-69` |
| Risk-aware confirmation UI | High-risk warning driven by `ActionEvent.security_risk`; duplicate submissions prevented by in-memory `submittedEventIds` keyed by event id | `src/components/shared/buttons/conversation-confirmation-buttons.tsx:44-47`, `102-118`, store `src/stores/event-message-store.ts:14-26` |
| Model version: per-component metrics | `UsageToMetrics` keyed by usage id ("default", "condenser", "profile:<name>:<uuid>"); `TokenUsage.model`+`response_id`; `costs[]` and `response_latencies[]` each carry `model` | `src/types/agent-server/core/events/conversation-state-event.ts:10-54` |
| Model version: live switching | `SwitchLLMAction{profile_name, reason}` / `SwitchLLMObservation{profile_name, active_model, reason}`; WS handler mirrors switch into conversation metadata (`active_profile`) and query cache (`active_model`) | `src/types/agent-server/core/base/action.ts:301-310`, `src/contexts/conversation-websocket-context.tsx:688-722` |
| Model version: history replay | Past switches rebuilt from persisted observations, entries keyed `history-switch:<event.id>` for idempotent reseeding | `src/hooks/chat/record-model-switch-message.ts:17-61`, `src/stores/model-store.ts:18-27` |
| Model version: conversation level | `ConversationInfo.agent.llm.model`, `current_model_id/current_model_name` (ACP runtime), `launched_agent_profile{agent_profile_id, revision}`; resolved into `AppConversation.llm_model` | `src/api/agent-server-adapter.ts:74-110`, `357-364` |
| Artifact↔run: transcript export | Export fetches full history, cross-checks against `/events/count`, names file `conversation-{conversationId}.md/html`, embeds model in header | `src/components/features/conversation/transcript-export-modal.tsx:77-120`, header rendering `src/utils/transcript-export/index.ts:520-525` |
| Artifact↔run: completeness proof | Cursor-cycle detection, non-advancing pagination errors, and final count verification throw instead of exporting partial history | `src/utils/transcript-export/load-complete-events.ts:85-124`, `153-160` |
| Artifact↔run: screenshots/files | `BrowserObservation.screenshot_data` embedded inline in the linked observation event; uploads land under the conversation workspace path and are referenced by name in the message content | `src/types/agent-server/core/base/observation.ts:42-55`, `src/api/conversation-file-upload.api.ts:72-145`, attachment note `src/stores/optimistic-user-message-store.ts:27-31` |
| Sub-run↔parent link | Start request sends `parent_conversation_id` + `agent_profile_id`; result reports `parent_link` flag with version-gated note (link needs agent-server ≥ 1.37.1) | `src/api/conversation-service/agent-server-conversation-service.api.ts:428-445`, `src/services/child-conversation-launch.ts:62-65`, `236-250` |
| Replay-safe client tools | Child-launch executions claimed per `(conversationId, toolCallId)` in a localStorage ledger so WS replays cannot double-launch billable conversations; result fed back as prefixed JSON user message | `src/services/child-conversation-launch.ts:196-227`, `488-497` |
| Dedup/idempotence safeguards | Store dedups by event id; WS side-effects skipped for replayed events (#1656); ACP started/terminal events merged per `tool_call_id`; observations replace actions by `action_id` | `src/stores/use-event-store.ts:99-107`, `src/contexts/conversation-websocket-context.tsx:556-568`, `src/utils/handle-event-for-ui.ts:404-416`, `432-441` |
| History pagination anchors | REST-first tail load (`TIMESTAMP_DESC`), scroll-up backfill anchored at oldest timestamp, WS reconnects with `resend_mode='since' <latest timestamp>` | `src/hooks/query/use-conversation-history.ts:21-29`, `src/hooks/use-load-older-events.ts:29-39` |

## Answers to Dimension Questions

**1. Can every output be traced to its inputs?**
Largely yes at event granularity. Observations carry `action_id` + `tool_call_id` back to the exact action and raw LLM tool call (`src/types/agent-server/core/events/observation-event.ts:9-39`; `action-event.ts:33-49`); assistant messages/thoughts ride inside events whose `thought`, `reasoning_content`, and `thinking_blocks` preserve the generating context (`src/types/agent-server/core/events/action-event.ts:15-25`). Two exceptions: (a) streamed token deltas are explicitly transient — they "are never persisted/resent" and are superseded by the authoritative final event (`src/stores/use-event-store.ts:94-97`, `src/utils/handle-event-for-ui.ts:225-250`), so the durable record starts at finalized events; (b) optimistic user bubbles are matched to echoed `MessageEvent`s by exact content string with a FIFO fallback, not by id (`src/stores/optimistic-user-message-store.ts:169-198`) — a heuristic correlation that can mis-pop under munged payloads.

**2. Is provenance preserved through transformations?**
Mostly. Condensation records which event ids were forgotten and inserts a summary with a documented offset (`src/types/agent-server/core/events/condensation-event.ts:14-27`); skill/agent-context augmentation is stamped onto the user `MessageEvent` as `activated_skills`/`extended_content` (`message-event.ts:12-19`); the UI's action→observation replacement preserves the causal pair by swapping in place via `action_id` (`src/utils/handle-event-for-ui.ts:432-441`). The one transformation that flattens provenance is transcript export: it renders human-readable Markdown/HTML where tool cards are grouped and details are optional (`src/utils/transcript-export/index.ts:285-289`, `421-427`) — ids survive only implicitly through ordering, not as printed references.

**3. Are model versions tracked in lineage?**
Yes, unusually well. Per-usage metrics carry `model_name`, per-call `response_id`, cost rows with `model`+`timestamp`, and latency rows with `model`+`response_id`, keyed by component usage id including per-profile keys `"profile:<name>:<uuid>"` (`src/types/agent-server/core/events/conversation-state-event.ts:10-54`). Mid-conversation switches persist as `SwitchLLMObservation{profile_name, active_model}` events that are replayable from history after reload (`src/hooks/chat/record-model-switch-message.ts:17-61`). Conversation records expose `agent.llm.model`, runtime `current_model_id/name`, and `launched_agent_profile.revision` (`src/api/agent-server-adapter.ts:89-110`). Gap: no evidence the frontend joins `response_latencies/token_usages.response_id` back to specific `ActionEvent`s — searched `response_id` usages in `src/` (only metric plumbing in `conversation-state-event.ts` and adapter metrics merging at `src/api/agent-server-adapter.ts:365-386`); the join exists in data but is not exercised.

**4. Can causal chains be audited?**
Yes, and this is the standout capability. The transcript exporter rebuilds chains (`actionsById` map + `event.action_id` resolution), verifies completeness against an independent event count, detects cursor loops, and fails closed rather than exporting a truncated chain (`src/utils/transcript-export/index.ts:275-277`, `431-441`; `load-complete-events.ts:85-124`, `153-160`; strict-pagination contract tested at `src/api/event-service/event-service.api.test.ts:29-40`). Interactive auditing has a caveat: chat-history pagination on cloud falls back to an empty page when filter params 500, i.e., silently stops loading older events (`src/api/event-service/event-service.api.ts:149-163`) — acceptable for scroll UX but a trap for informal audits that don't use export. Hook events extend auditability to guardrail execution with blocking reasons (`hook-execution-event.ts:42-59`).

## Architectural Decisions

1. **Lineage lives in the event stream, not a side table.** Every causal edge (action→observation, action→approval outcome, prompt→tools, context injection, condensation) is just another typed event in one append-only history fetched via one paginated API (`src/types/agent-server/core/openhands-event.ts:25-46`; search API `src/api/event-service/event-service.api.ts:102-181`). This makes chains reconstructible by any consumer, including the export pipeline.
2. **Keep the raw LLM payload, not just the parsed action.** `tool_call` is retained beside `action` specifically because the two can diverge (e.g., LLM-predicted `security_risk` present only in the raw call) (`src/types/agent-server/core/events/action-event.ts:42-49`) — an auditable decision to prefer fidelity over tidiness.
3. **Server-authoritative identity, client-side dedup.** Ids are assigned server-side; the client treats them as the dedup key both in the store (`use-event-store.ts:99-107`) and before non-idempotent side effects like analytics and model-switch handling (`conversation-websocket-context.tsx:556-568`).
4. **Fail-closed exports, fail-open chat.** Transcript export throws on unverifiable completeness; ordinary chat pagination degrades gracefully to avoid infinite retries (`event-service.api.ts:143-163` vs `load-complete-events.ts:106-119`). Lineage strictness is proportional to the stakes of the consumer.
5. **Client-side state for display-only lineage.** Profile name vs model string disambiguation (`active_profile`), workspace/repo attachment, and the child-launch ledger are deliberately localStorage-resident because the server round-trips only the model string (`src/api/conversation-metadata-store.ts:30-37`; #1082 rationale) — pragmatic, but it forks lineage truth between browser and server.
6. **Convention-based client-tool feedback.** Client-executed tools (child conversation launch) report outcomes to the agent as prefixed-JSON user messages since the server already ACKed the tool call before the browser acted (`src/services/child-conversation-launch.ts:451-497`) — causality preserved by message convention, not schema.

## Notable Patterns

- **Pair-supersession rendering**: an observation replaces its action card in place by matching `uiEvent.id === event.action_id`, giving users a single artifact per cause-effect pair (`src/utils/handle-event-for-ui.ts:432-441`); ACP tool calls merge two lifecycle events per `tool_call_id` the same way (`acp-tool-call-event.ts:11-16`; `handle-event-for-ui.ts:404-416`).
- **Derived-but-deterministic synthetic ids**: the Skill Ready banner fabricates `${userEvent.id}-skill-ready`, deriving new artifacts from source event ids so downstream anchoring stays stable (`create-skill-ready-event.ts:52-59`).
- **Anchor-based temporal lineage in UI state**: model-switch chips anchor to `anchorEventId` (last renderable event), seeded from history with ids like `history-switch:<event.id>` so reload reseeding is idempotent (`src/stores/model-store.ts:8-27`, `record-model-switch-message.ts:43-57`).
- **Thought hoisting with de-dup by action id** when mixed action/observation arrays could double-render narration (documented in AGENTS.md grouping rules; implemented in `group-events` consumed at `src/utils/transcript-export/index.ts:285-308`).
- **Replay-safe claiming**: non-idempotent client tools claim `(conversation, toolCallId)` before any network work so reconnect replays drop mid-flight duplicates (`child-conversation-launch.ts:196-227`).
- **Stable-sort preservation of causal order**: equal-timestamp events keep server/store order during export sorting, with the invariant documented at the sort site (`load-complete-events.ts:148-152`).

## Tradeoffs

- **Schema-first lineage vs. request-level approval binding.** The event stream gives rich post-hoc chains, but the approval *request* is stateless (`respond_to_confirmation` with `{accept}`), pushing action identification to a client heuristic over store contents (`conversation-confirmation-buttons.tsx:30-36` — note the predicate returns true for *any* agent-sourced event while awaiting confirmation). Simpler protocol, weaker binding.
- **Content-keyed correlation vs. correlation ids for user messages.** Exact-string echo matching avoids server changes but breaks down on server-side content munging; the FIFO fallback mitigates but documents the fragility (`optimistic-user-message-store.ts:71-84`).
- **Ephemeral streaming vs. durable record.** Token deltas give responsive UX but are intentionally outside the durable lineage; reasoning that exists only in deltas is preserved into the UI view but the persisted history begins at finalized events (`use-event-store.ts:94-97`, `handle-event-for-ui.ts:146-169`).
- **localStorage convenience vs. audit durability.** Child-launch ledger, `active_profile`, and repo/workspace metadata would not survive a different browser/profile, weakening cross-session auditability despite strong server-side event lineage (`conversation-metadata-store.ts:50-70`).
- **Graceful degradation vs. silent truncation.** Cloud chat pagination returns empty pages on unsupported filters to stop retry loops (`event-service.api.ts:153-163`); the tradeoff is that an in-chat "full history" may be shorter than the true chain unless the user exports.

## Failure Modes / Edge Cases

- **Approval race/misattribution**: if the last agent-sourced event in the store is not the action actually pending confirmation (e.g., out-of-order delivery, planning-agent traffic sharing the store), the confirm/reject buttons bind to the wrong candidate; duplicate-submission protection exists only in-memory per session (`conversation-confirmation-buttons.tsx:30-36`, `93-100`; store `event-message-store.ts:14-26`).
- **Echo mismatch strands bubbles**: server-side whitespace stripping triggers the FIFO fallback path; genuinely divergent bodies pop the wrong pending entry when several sends are in flight (`optimistic-user-message-store.ts:186-189`).
- **Unpatched cloud backends truncate interactive history**: timestamp-filter 500s collapse older-events loading to a no-op with only a console warning (`event-service.api.ts:157-163`).
- **Parent-link absence on older servers**: children launched against agent-server < 1.37.1 exist without any persisted parent relationship; the UI compensates with an explanatory `parent_link_note` rather than a broken assumption (`child-conversation-launch.ts:236-250`).
- **Ledger corruption tolerated over safety**: a corrupt child-launch localStorage ledger resets to empty, accepting double-launch risk rather than blocking launches (`child-conversation-launch.ts:214-216`, `222-225`).
- **Malformed historical events**: transcript building skips entries that fail against the current TS schema instead of aborting (`transcript-export/index.ts:488-491`), so very old chains may export with holes.
- **Metric-to-action join unused**: because `response_id` is never joined to `ActionEvent` in this codebase, cost attribution precision depends entirely on the server's usage-id keying; a mislabeled usage id would be invisible client-side (search boundary: greps for `response_id` across `src/` surfaced only metric definitions/plumbing).

## Future Considerations

- Bind approvals to explicit action ids end-to-end: include the pending `ActionEvent.id` (and populate the existing `reason` field) in `ConfirmationResponseRequest`, making the client heuristic unnecessary (`event-service.types.ts:1-4`, `use-respond-to-confirmation.ts:21-23`).
- Add a correlation id to outbound user messages (e.g., client-generated ULID echoed in the resulting `MessageEvent.id` namespace) to replace content matching (`optimistic-user-message-store.ts:81-84`).
- Surface `llm_response_id`/`response_id` joins in the UI — e.g., per-step token/cost tooltips linking an ActionEvent to its exact LLM call — the data already exists in `TokenUsage`/`response_latencies` (`conversation-state-event.ts:10-41`).
- Promote client-side lineage (metadata store, launch ledger) to server-persisted settings or conversation tags (`tags` field already generic: `agent-server-adapter.ts:98-106`) for cross-device auditability.
- Print event-id references (or deep links) in exported transcripts so exported chains remain navigable back to the primary event stream.
- Fact-level citation rendering for agent answers (mapping answer segments to source observations) would close the remaining gap between event traceability and claim traceability.

## Questions / Gaps

- **No evidence found** for any UI/API surface that renders per-fact citations linking answer text to originating observations; searched citation/provenance-style terms and event-content helpers (`get-event-content.tsx`, `parse-message-from-event`) — rendering operates on whole-event content only.
- **No evidence found** that the frontend correlates `TokenUsage.response_id` / `response_latencies[].response_id` to individual `ActionEvent`s; the join is possible with the shipped schema but unexercised here (may exist in the sibling `software-agent-sdk` repo, which is out of scope for this isolated study).
- Server-side persistence details (how event ids are generated — ULID vs UUID — and whether `respond_to_confirmation` validates the pending action server-side) are owned by the agent-server SDK and cannot be verified from this repository; the frontend types only constrain the client half (`base/event.ts:11-14` comment says "ULID/UUID").
- Whether replays over the planning-agent socket still use `resend_all` (AGENTS.md notes it pending migration) could not be confirmed in the read window of `conversation-websocket-context.tsx`; if so, its side-effect handlers rely solely on the same dedup guard audited above.

---

Generated by `Dimension 10.03: Causal Links and Lineage` against `openhands`.

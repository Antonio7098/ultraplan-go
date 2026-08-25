# Source Analysis: openhands

## Dimension 11.04: Context Provenance and Integrity

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (OpenHands Agent Canvas frontend, `@openhands/agent-canvas`) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19, Zustand, React Query, `@openhands/typescript-client` |
| Analyzed | 2026-08-25 |

## Summary

OpenHands (this repo is the agent-canvas **frontend**) treats provenance as a first-class property of its context model rather than an afterthought. The unit of agent/user/tool context is the typed event (`BaseEvent`), and every event on the wire carries three provenance fields: a unique `id`, an ISO `timestamp`, and a `source` drawn from a closed four-value union (`"agent" | "user" | "environment" | "hook"`). Concrete event families narrow the union further (observations are always `"environment"`, hook executions always `"hook"`), and causal lineage is preserved through explicit ID links: observations point back to their originating action via `action_id`, actions carry `tool_call_id` plus an `llm_response_id` that groups parallel tool calls from one LLM response, and hook events reference both the action and message they observed.

Trust is modeled two ways: a `SecurityRisk` enum (`UNKNOWN/LOW/MEDIUM/HIGH`) attached to every action — surfaced as warnings in the chat UI and used to frame the human-confirmation flow — and explicit trust directives embedded in injected system-prompt context (the `<RUNTIME_SERVICES>` block tells the agent to trust it "over guessing"). Transformation history is recorded by the backend's condensation protocol (`CondensationEvent` lists `forgotten_event_ids` plus an optional summary), while the frontend keeps a strict separation between the canonical event log and derived view transformations (streaming deltas merged, actions superseded by their observations), so the raw provenance-bearing record is never destroyed. Provenance survives serialization trivially because the provenance fields *are* the wire format: every inbound WebSocket message is JSON-parsed and runtime-validated against the `id`/`timestamp`/`source` contract before it may enter the store, REST history returns the same typed events, and transcript exports preserve author attribution, timestamps, and action linkage into Markdown/HTML.

The main gaps are on the freshness-semantics and visualization side: there is no TTL/staleness annotation beyond raw timestamps, `security_risk` is an LLM self-assessment limited to actions, the client adds an ephemeral `isFromPlanningAgent` origin flag that exists only in memory, and condensation markers (`forgotten_event_ids`) are consumed as signals but never rendered or applied to the UI's own history view.

## Rating

**7 / 10** — Clear provenance model with typed interfaces, runtime validation at the serialization boundary, trust-driven UI safeguards, and pinned test coverage. Falls short of 8–9 because freshness is implicit (timestamps only, no staleness semantics), trust annotation covers only actions (and is LLM self-reported with no client-side verification chain), transformation records from condensation are not visualized or enforced client-side, and one origin discriminator (`isFromPlanningAgent`) is ad-hoc client-side state outside the wire schema.

## Evidence Collected

Every entry includes a file path with line numbers relative to the workspace root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Source annotation (base) | `BaseEvent` requires `id` (ULID/UUID), ISO `timestamp`, and `source: SourceType` on every event | sources/openhands/src/types/agent-server/core/base/event.ts:10-25 |
| Source taxonomy | `SourceType = "agent" \| "user" \| "environment" \| "hook"` closed union | sources/openhands/src/types/agent-server/core/base/common.ts:55-56 |
| Per-kind source narrowing | Observations pin `source: "environment"`; hooks pin `"hook"`; pause events pin `"user"`; streaming deltas pin `"agent"` | sources/openhands/src/types/agent-server/core/events/observation-event.ts:9-13; sources/openhands/src/types/agent-server/core/events/hook-execution-event.ts:26-29; sources/openhands/src/types/agent-server/core/events/pause-event.ts:8; sources/openhands/src/types/agent-server/core/events/streaming-delta-event.ts:5 |
| Causal lineage (observation → action) | `ObservationEvent.action_id` links each observation to the action it responds to; also carries `tool_name` + `tool_call_id` | sources/openhands/src/types/agent-server/core/events/observation-event.ts:15-38 |
| Causal lineage (parallel calls) | `ActionEvent.llm_response_id` "groups related actions from same LLM response"; `tool_call_id` ties to LLM API call | sources/openhands/src/types/agent-server/core/events/action-event.ts:37-56 |
| Hook observability lineage | `HookExecutionEvent.action_id` / `message_id` link hook runs to observed events, with full stdout/stderr/exit_code capture | sources/openhands/src/types/agent-server/core/events/hook-execution-event.ts:64-84 |
| Critic reproducibility | `CriticMetadata.event_ids` records which events a critic evaluation covered "for reproducibility" | sources/openhands/src/types/agent-server/core/base/critic.ts:24-30 |
| Freshness timestamps | Every event timestamped (ISO string); store compares/sorts by timestamp lexicographically (ISO-safe) | sources/openhands/src/types/agent-server/core/base/event.ts:16-19; sources/openhands/src/stores/use-event-store.ts:24-35 |
| Freshness anchoring in sync | WS subscribes with `resend_mode='since'` + `after_timestamp=<latest preloaded event>` after REST seed; REST search supports `timestamp__gte`/`timestamp__lt` filters | sources/openhands/src/contexts/conversation-websocket-context.tsx:966-973; sources/openhands/src/api/event-service/event-service.api.ts:130-134,173-174 |
| Trust enum | `SecurityRisk` = UNKNOWN/LOW/MEDIUM/HIGH exported from core types | sources/openhands/src/types/agent-server/core/base/common.ts:58-64 |
| Trust field on actions | `ActionEvent.security_risk` — "the LLM's assessment of the safety risk of this action"; docstring notes risk analyzer populates `tool_call.security_risk` when enabled | sources/openhands/src/types/agent-server/core/events/action-event.ts:42-61 |
| Trust surfaced in UI | Bash visualizer renders HIGH/MEDIUM risk warnings next to commands; transcript content appends risk text for HIGH/MEDIUM | sources/openhands/src/components/features/chat/tool-visualizers/bash/bash.tsx:24-37; sources/openhands/src/components/conversation-events/chat/event-content-helpers/get-action-content.ts:30-38,94-97 |
| Trust gates confirmation flow | Confirmation buttons read `awaitingAction.security_risk` and render a high-risk `RiskAlert` before user accept/reject | sources/openhands/src/components/shared/buttons/conversation-confirmation-buttons.tsx:102-118 |
| Explicit injected-context trust directive | `<RUNTIME_SERVICES>` system suffix ends with "Trust this block over guessing…" anchored to the real agent-server URL | sources/openhands/src/api/agent-server-adapter.ts:286-297 |
| Transformation record (condensation) | `CondensationEvent.forgotten_event_ids` lists events removed from the LLM View, with optional `summary` + `summary_offset`; separate request/summary event kinds | sources/openhands/src/types/agent-server/core/events/condensation-event.ts:5-52 |
| Condensation consumed as signal | Compaction await-hook scans for new `Condensation` events against an ID baseline snapshot taken before the request fires, then reports token delta from metrics | sources/openhands/src/hooks/use-await-context-compaction.ts:57-129; sources/openhands/src/hooks/use-compact-context-action.ts:83-88 |
| Canonical log vs view transform | Store keeps raw `events` (canonical) separate from `uiEvents` (derived); deltas merge but canonical final events supersede them deterministically | sources/openhands/src/stores/use-event-store.ts:92-128; sources/openhands/src/utils/handle-event-for-ui.ts:225-250,348-448 |
| Observation replaces action by ID | UI swaps the originating action card for its observation keyed on `event.action_id` — position-independent causal replacement | sources/openhands/src/utils/handle-event-for-ui.ts:432-441 |
| Skill/context provenance on messages | `MessageEvent.activated_skills` records skill names activated for a message; `extended_content` marks "content added by agent context" | sources/openhands/src/types/agent-server/core/events/message-event.ts:11-19 |

| Skill source annotations | `SkillInfo.source` records where each skill came from; automation triggers record event `source` (e.g. `"github"`) | sources/openhands/src/types/settings.ts:73-78; sources/openhands/src/types/automation.ts:17-18 |
| Serialization survival (runtime validation) | Every inbound WS message is JSON-parsed then validated by `isAgentServerEvent`/`isBaseEvent`, requiring non-empty `id`, non-empty `timestamp`, and a valid `source` union value before entering the store | sources/openhands/src/contexts/conversation-websocket-context.tsx:541-547,763-779; sources/openhands/src/types/agent-server/type-guards.ts:45-62,299-301 |
| Dedup integrity across transports | Store dedupes by event ID across REST seed and WS resend; bulk-add re-sorts by timestamp; pinned by tests | sources/openhands/src/stores/use-event-store.ts:99-107,159-208; sources/openhands/__tests__/stores/use-event-store.test.ts:127-157 |
| Export preserves provenance | Transcript export derives author attribution from `event.source`, emits optional ISO timestamps, and resolves observation summaries back through `actionsById.get(event.action_id)` | sources/openhands/src/utils/transcript-export/index.ts:121-124,264-277,347-368,431-441 |
| Export completeness proof | `loadCompleteTranscriptEvents` de-duplicates pages by event ID and fails hard if the fetched count cannot prove completeness (no silent partial export) | sources/openhands/src/utils/transcript-export/load-complete-events.ts:33-161 |
| Immutable telemetry attribution | `before_send` hook spreads frozen `client_source`/`client_version` properties last so event producers cannot strip or override attribution | sources/openhands/src/services/telemetry.ts:110-115,132-146,359 |
| Client-side origin augmentation | Planning-agent socket tags every event `{ ...event, isFromPlanningAgent: true }` at ingestion because both sockets share one store; delta merging is scoped per sender (#1656) | sources/openhands/src/contexts/conversation-websocket-context.tsx:172-180,793-798; sources/openhands/src/stores/use-event-store.ts:10-12; sources/openhands/src/utils/handle-event-for-ui.ts:31-37 |

## Answers to Dimension Questions

**1. Does each context item know where it came from?**
Yes — strongly. Every context unit is a `BaseEvent` carrying `source: SourceType` from a closed four-value union (`sources/openhands/src/types/agent-server/core/base/common.ts:55-56`), and specific event families narrow it further (observations are always `"environment"` at `sources/openhands/src/types/agent-server/core/events/observation-event.ts:13`; hooks always `"hook"` at `sources/openhands/src/types/agent-server/core/events/hook-execution-event.ts:29`). Beyond actor identity, causal origin is explicit: `ObservationEvent.action_id` (`observation-event.ts:38`), `ActionEvent.tool_call_id`/`llm_response_id` (`action-event.ts:40,56`), and `HookExecutionEvent.action_id`/`message_id` (`hook-execution-event.ts:69,74`). One exception: the planning-vs-main-agent distinction is not in the wire schema — the client synthesizes it at the socket boundary (`isFromPlanningAgent`, `sources/openhands/src/contexts/conversation-websocket-context.tsx:177,793-798`).

**2. Is freshness tracked?**
Partially. All events carry ISO `timestamp` (`sources/openhands/src/types/agent-server/core/base/event.ts:19`), the store re-sorts out-of-order arrivals by timestamp (`sources/openhands/src/stores/use-event-store.ts:41-53,131-135`), history pagination and WS resume are timestamp-anchored (`resend_mode='since'` + `after_timestamp`, `sources/openhands/src/contexts/conversation-websocket-context.tsx:966-973`), and REST search supports `timestamp__gte/__lt` filters (`sources/openhands/src/api/event-service/event-service.api.ts:130-134`). However, there is no age/TTL/staleness metadata anywhere — no evidence of any "how old is this context" query surfaced to users or code. Searched `src/` for `freshness|stale|ttl|age` patterns around events; only React Query cache-staleness defaults exist, unrelated to event semantics.

**3. Is trust level indicated?**
Yes, but narrowly. `SecurityRisk` (UNKNOWN/LOW/MEDIUM/HIGH) is attached to every `ActionEvent` as "the LLM's assessment of the safety risk" (`sources/openhands/src/types/agent-server/core/events/action-event.ts:58-61`), rendered as warnings in the bash visualizer (`sources/openhands/src/components/features/chat/tool-visualizers/bash/bash.tsx:29-37`) and used to decorate the human-confirmation gate with a high-risk alert (`sources/openhands/src/components/shared/buttons/conversation-confirmation-buttons.tsx:102-118`). Injected prompt context carries an explicit trust directive ("Trust this block over guessing", `sources/openhands/src/api/agent-server-adapter.ts:286-296`). Limitations: messages and observations have no trust fields; the risk value is LLM self-reported (populated only "when LLM risk analyzer is enabled", `action-event.ts:46-47`) with no independent verification visible client-side.

**4. Are transformations traceable?**
Mostly yes at the schema level, partially in practice. The condensation protocol explicitly records what was transformed: `forgotten_event_ids`, `summary`, and `summary_offset` (`sources/openhands/src/types/agent-server/core/events/condensation-event.ts:13-26`). The frontend consumes these events as completion signals and measures token impact (`sources/openhands/src/hooks/use-await-context-compaction.ts:112-129`) but does **not** render condensation markers or remove forgotten events from its own view — searched all of `src/components/` and `src/routes/` for `Condensation`: zero rendering references. Client-side view transformations (delta merging, action→observation replacement) are deterministic functions over the canonical log kept separately in the store (`sources/openhands/src/stores/use-event-store.ts:92-128` vs `uiEvents`), and transcript exports replay them identically with completeness proofs (`sources/openhands/src/utils/transcript-export/load-complete-events.ts:150-161`). Hook execution events provide full stdout/stderr observability of PreToolUse/PostToolUse transformations (`hook-execution-event.ts:76-89`).

## Architectural Decisions

1. **Events as the universal context currency.** All agent context (messages, actions, observations, state changes, condensations, hooks, streaming deltas) is a discriminated union of typed events extending one provenance-bearing base (`sources/openhands/src/types/agent-server/core/base/event.ts:10-25`). This makes provenance uniform instead of per-feature.
2. **Provenance fields ARE the wire format.** Rather than maintaining a parallel metadata layer, `id`/`timestamp`/`source` are required members of every serialized event, and the client runtime-validates them at the deserialization boundary (`sources/openhands/src/types/agent-server/type-guards.ts:45-62`) before anything enters app state. Invalid provenance means the event is dropped, not stored degraded.
3. **Canonical log vs. derived view.** The Zustand store keeps the untouched event array alongside a `uiEvents` projection rebuilt by pure transforms (`handleEventForUI`), so presentation-level transformations never corrupt the provenance record (`sources/openhands/src/stores/use-event-store.ts:92-135`).
4. **ID-based causal linkage over positional ordering.** Observations find their actions by `action_id`, ACP tool-call pairs merge by `tool_call_id` (`sources/openhands/src/utils/handle-event-for-ui.ts:404-416`), and compaction detection baselines by event-ID sets (`sources/openhands/src/hooks/use-await-context-compaction.ts:79-81`) — robust to reordering, duplication, and dual-transport (REST seed + WS resend) delivery.
5. **Client-augmented origin for multi-agent disambiguation.** Because main and planning sockets share one store and wire events don't distinguish agents, the client tags ingestion origin (`isFromPlanningAgent`) and scopes all delta-merge logic per sender, with regression tests citing bug #1656 (`sources/openhands/src/utils/handle-event-for-ui.ts:31-37`).
6. **Completeness-proven exports.** Transcript export refuses to produce a partial transcript: pagination must terminate via cursor or be provable via an independent event count, else it throws (`sources/openhands/src/utils/transcript-export/load-complete-events.ts:100-119,153-160`).

## Notable Patterns

- **Discriminated unions with narrowed `source` literals**: subtypes tighten the base union (e.g., `source: "environment"` on observations), giving compile-time guarantees about who produces what (`sources/openhands/src/types/agent-server/core/events/observation-event.ts:11-13`).
- **Runtime type-guards mirroring static types**: `isBaseEvent` re-validates the exact union membership at runtime, acting as a serialization firewall (`sources/openhands/src/types/agent-server/type-guards.ts:45-62`).
- **Baseline-snapshot pattern for change detection**: compaction snapshots `eventIds` *before* issuing the HTTP request because the ack can race the server's Condensation emission (`sources/openhands/src/hooks/use-compact-context-action.ts:83-88`) — a subtle ordering-aware integrity measure documented inline and tested (`use-await-context-compaction.test.ts:17`).
- **Immutable attribution via last-write-wins spreading**: telemetry `before_send` spreads frozen `CANVAS_EVENT_PROPERTIES` after producer properties, making `client_source` impossible to override (`sources/openhands/src/services/telemetry.ts:137-145`).
- **Trust text co-located with risk enums**: risk-to-copy mapping lives in one helper reused by both live cards and exports (`sources/openhands/src/components/conversation-events/chat/event-content-helpers/get-action-content.ts:30-38`).

## Tradeoffs

- **Strict validation vs. forward compatibility.** Dropping events whose `source` falls outside today's four-value union protects integrity but would silently discard future source kinds; the transcript exporter takes the opposite tack, catching per-entry errors and skipping malformed payloads rather than aborting (`sources/openhands/src/utils/transcript-export/index.ts:488-491`), trading guaranteed fidelity for resilience.
- **Lexicographic timestamp comparison** is O(1) and correct for ISO strings, but silently depends on the backend emitting consistent UTC ISO format (`sources/openhands/src/stores/use-event-store.ts:33-34`); mixed-precision or offset-bearing timestamps would sort incorrectly.
- **Self-reported risk.** Surfacing `security_risk` in the UI gives humans real signal at the confirmation gate, but since it originates from the same LLM being policed (when the analyzer is enabled at all), the trust chain has a single link.
- **Ephemeral client augmentation.** `isFromPlanningAgent` fixes real misattribution bugs (#1656) but lives outside the persisted schema: it survives only in the in-memory store and must be re-derived on replay (`conversation-websocket-context.tsx:793-798`).
- **UI ignores condensation bookkeeping it receives.** Keeping all events visible after the LLM's view was condensed avoids destroying local history, but means the UI's chat can diverge from what the agent actually remembers — the `forgotten_event_ids` data is present yet unused client-side.

## Failure Modes / Edge Cases

- **Out-of-order delivery** is handled: bulk inserts re-sort by timestamp and tests pin newest-first seeding (`sources/openhands/__tests__/stores/use-event-store.test.ts:127-146`).
- **Duplicate delivery across REST+WS** collapses by ID dedup, except id-less streaming deltas which are deliberately excluded from tracking for performance (`sources/openhands/src/stores/use-event-store.ts:94-97`).
- **Malformed pagination inputs** fail loudly: a missing oldest-timestamp anchor is treated as exhausted rather than paginating blindly (`sources/openhands/src/hooks/use-load-older-events.ts:113-120`), and a non-array `page.items` response throws instead of corrupting the store (`sources/openhands/src/hooks/use-load-older-events.ts:136-139`).
- **Cloud backend lacking filter support**: event search attempts filtered requests and falls back to an empty page (never a duplicate-prone limit-only refetch) when the server 500s on timestamp filters (`sources/openhands/src/api/event-service/event-service.api.ts:143-163`).
- **Unverifiable export completeness** throws instead of exporting a truncated tail (`sources/openhands/src/utils/transcript-export/load-complete-events.ts:106-119`) — integrity preferred over availability.
- **Uncovered edge**: an invalid-but-parseable event (e.g., unknown `source` string) is dropped entirely at the WS boundary with no user-visible or logged indication in the examined handler paths (`sources/openhands/src/contexts/conversation-websocket-context.tsx:779` guard simply skips the else branch) — silent provenance failure.

## Future Considerations

- Add staleness/TTL semantics: surface event age or "as-of" indicators, especially for resumed conversations where REST-seeded history may be hours old (the `after_timestamp` plumbing already exists to compute deltas).
- Render condensation state in the chat: mark or collapse `forgotten_event_ids` so users see the same view boundary the LLM sees; the data arrives today but is discarded visually.
- Promote `isFromPlanningAgent` (or an equivalent agent identifier) into the wire schema or derive it from conversation IDs to remove the ephemeral client-side patch and its merge-scoping special cases.
- Extend trust annotation beyond actions (e.g., observation trust for MCP/tool outputs) and document the server-side risk-analyzer contract so clients know when `UNKNOWN` means "not analyzed" versus "analyzed and uncertain".
- Log or toast dropped events failing `isAgentServerEvent` to convert silent provenance failures into observable ones.

## Questions / Gaps

- Where does the security-risk analyzer run and under what configuration is `security_risk` populated? The docstring says "when LLM risk analyzer is enabled" (`sources/openhands/src/types/agent-server/core/events/action-event.ts:46-47`), but the analyzer itself lives in the backend SDK (`OpenHands/software-agent-sdk`), outside this source's boundary — not verifiable here.
- No evidence found of any UI or service consuming `forgotten_event_ids` beyond the compaction-completion signal; whether intentional (history preservation policy) or an oversight could not be determined from this repository alone.
- Whether `CondensationSummaryEvent` is ever emitted/rendered by current servers: the type exists (`sources/openhands/src/types/agent-server/core/events/condensation-event.ts:40-52`) but no consumer was found in `src/`.
- Freshness of file/workspace content shown in the Files tab (cache headers, ETags) was not studied in depth; the dimension focus here was conversation-context provenance, where evidence is strong.

---

Generated by `Dimension 11.04: Context Provenance and Integrity` against `openhands`.

# Source Analysis: openhands

## Reason/Act/Observe Cadence

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | Python (FastAPI app server) + TypeScript (React frontend); the agent loop itself is delegated to external packages `openhands-sdk`, `openhands-agent-server`, `openhands-tools` (pinned in `pyproject.toml:47-49`) |
| Analyzed | 2026-07-13 |

## Summary

The Reason/Act/Observe cadence is **not implemented in this source tree**. The OpenHands
repository at this commit is a frontend + app-server shell that delegates the actual
reasoning → action → observation loop to three external PyPI packages
(`openhands-sdk==1.29.0`, `openhands-agent-server==1.29.0`,
`openhands-tools==1.29.0`; `pyproject.toml:47-49`). The app server's job is to
configure the agent (LLM, condenser, security analyzer, hooks), persist events,
and proxy `send-message` to the agent server
(`openhands/app_server/app_conversation/app_conversation_router.py:552-583`).
Direct evidence of the cadence in this repo is therefore limited to:

1. The event type system imported from `openhands.sdk.event`, which exposes
   `MessageEvent`, `ActionEvent`, `ObservationEvent`, `AgentErrorEvent`,
   `PauseEvent`, `CondensationEvent`, `HookExecutionEvent`,
   `ConversationStateUpdateEvent`, and `SystemPromptEvent` (see references at
   `openhands/app_server/event_callback/webhook_router.py:57`,
   `openhands/app_server/event_callback/set_title_callback_processor.py:26`,
   `frontend/src/types/v1/core/events/index.ts:1-10`).
2. Configuration of the `LLMSummarizingCondenser` that condenses observation
   history (`openhands/app_server/app_conversation/app_conversation_service_base.py:578-612`,
   `openhands/app_server/utils/llm_metadata.py:40`).
3. Storage and routing of `ActionEvent` / `ObservationEvent` pairs as they are
   streamed from the agent server via webhook
   (`openhands/app_server/event_callback/webhook_router.py:453-522`).
4. Switch-tool mid-loop LLM switching observable through `ObservationEvent`
   carrying `SwitchLLMObservation.active_model`
   (`openhands/app_server/event_callback/webhook_router.py:483-496`).

All actual loop mechanics — model output parsing, tool-call extraction,
observation injection, hook execution (Pre/PostToolUse/UserPromptSubmit/Session
Start/End/Stop), thinking-block capture, and the inner step driver — are
implemented inside `openhands-agent-server` and `openhands-sdk` and are not
present as source in this repository.

## Rating

**3 / 10**

Rationale: from inside this source tree the cadence is only **observable**, not
**implemented**. The repository imports the SDK types, persists the events,
and ships a typed UI for rendering each phase, but it does not contain a step
loop, parser, scratchpad, or observation injector. Without the SDK source in
hand, a reader of this repo can only confirm *that* there are observation
events; they cannot inspect *how* the model is asked to act, *how* observations
are formatted, *what* happens on failure, or *whether* failed observations
are retried. The split into three external packages and the lack of any
in-repo loop file makes this a poorly observable seam from a study
standpoint. There are unit tests for condenser creation
(`tests/unit/app_server/test_app_conversation_service_base.py:300-440`) and
event-callback filtering (`tests/unit/app_server/test_sql_event_callback_service.py:174-558`),
but nothing that exercises the inner cadence itself. The system only rises
above "absent" because the event schema and condenser configuration are
clearly designed around an explicit Reason → Act → Observe → Condense loop
(see Observation tool_call_id ↔ Action.tool_call_id pairing at
`frontend/src/types/v1/core/events/observation-event.ts:20` /
`frontend/src/types/v1/core/events/action-event.ts:40`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Loop lives in external SDK, not in this repo | `openhands-agent-server==1.29.0`, `openhands-sdk==1.29.0`, `openhands-tools==1.29.0` dependencies | `pyproject.toml:47-49` |
| App server is a thin proxy for `send-message` | `await httpx_client.post(f'{agent_server_url}/api/conversations/{conversation_id}/events', json={'role': ..., 'content': ..., 'run': ...})` | `openhands/app_server/app_conversation/app_conversation_router.py:555-567` |
| Explicit Reason → Act → Observe event types imported | `from openhands.sdk.event import ConversationStateUpdateEvent, ObservationEvent` | `openhands/app_server/event_callback/webhook_router.py:57` |
| `ActionEvent.tool_call_id` matches `ObservationEvent.tool_call_id` | Comment in observation schema: "The tool call id that this observation is responding to" | `frontend/src/types/v1/core/events/observation-event.ts:20` |
| `ActionEvent.action`, `tool_call`, `thought`, `reasoning_content`, `thinking_blocks`, `security_risk`, `llm_response_id` are first-class fields | `ActionEvent` interface | `frontend/src/types/v1/core/events/action-event.ts:11-72` |
| Condensation (`CondensationEvent`, `CondensationRequestEvent`, `CondensationSummaryEvent`) is a first-class event kind | Condensation event types | `frontend/src/types/v1/core/events/condensation-event.ts:5-46` |
| Stats split by `agent` and `condenser` usage_id, indicating per-component cost/token accounting | `usage_to_metrics: { 'agent': ..., 'condenser': ... }` | `tests/unit/app_server/test_webhook_router_stats.py:101-122`, `openhands/app_server/utils/llm_metadata.py:40` |
| Condenser configured per-agent-type with usage_id `condenser` or `planning_condenser` | `_create_condenser` constructs `LLMSummarizingCondenser(llm=llm.model_copy(update={'usage_id': 'condenser'/'planning_condenser'}))` | `openhands/app_server/app_conversation/app_conversation_service_base.py:578-612` |
| Condenser knobs (`enabled`, `max_size`) exposed in user settings | `condenser=CondenserSettings(enabled=True, max_size=200)` | `tests/unit/storage/data_models/test_settings.py:78` |
| `ActionEvent` carries the LLM-supplied `security_risk` and an optional `critic_result` | `security_risk: SecurityRisk; critic_result?: CriticResult \| null` | `frontend/src/types/v1/core/events/action-event.ts:61-66` |
| Hook execution event exposes `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `SessionStart`, `SessionEnd`, `Stop` types with `blocked`/`exit_code`/`reason` | `HookExecutionEvent` interface | `frontend/src/types/v1/core/events/hook-execution-event.ts:1-100` |
| App server surfaces the same hook event types via `/api/v1/app-conversations/{id}/hooks` | `event_types = ['pre_tool_use', 'post_tool_use', 'user_prompt_submit', 'session_start', 'session_end', 'stop']` | `openhands/app_server/app_conversation/app_conversation_router.py:1495-1502` |
| `ObservationEvent` carries `is_error`, `error`, `exit_code`, `timeout`, `truncated` — failed observations are surfaced in-band | `ExecuteBashObservation.error / .timeout`, `GlobObservation.truncated`, `MCPToolObservation.is_error` | `frontend/src/types/v1/core/base/observation.ts:57-83, 220-244, 9-22` |
| Agent-initiated mid-loop LLM switch observed via `ObservationEvent` of `SwitchLLMObservation` carrying `active_model` | `if isinstance(event, ObservationEvent) and isinstance(event.observation, SwitchLLMObservation) and event.observation.active_model:` | `openhands/app_server/event_callback/webhook_router.py:483-489` |
| Event-callback service routes `ObservationEvent` (not just `MessageEvent`) through app-server callbacks | `callback1_request = CreateEventCallbackRequest(event_kind='ActionEvent')`, `callback2_request = CreateEventCallbackRequest(event_kind='ObservationEvent')` | `tests/unit/app_server/test_sql_event_callback_service.py:166-178` |
| Stats events stream from agent server → app server via webhook, used to update token/cost records | `if event.key == 'stats': await app_conversation_info_service.process_stats_event(event, conversation_id)` | `openhands/app_server/event_callback/webhook_router.py:470-473` |
| Conversation storage persists prompt/completion/cache/reasoning tokens and `accumulated_cost` from `ConversationStats` | `accumulated_cost, prompt_tokens, completion_tokens, cache_read_tokens, cache_write_tokens, reasoning_tokens, context_window, per_turn_token` | `tests/unit/app_server/test_webhook_router_stats.py:81-87` |
| `ConversationStateUpdateEvent.key` enum includes `full_state`, `execution_status`, `stats` | `key: 'full_state' \| 'execution_status' \| 'stats'` | `frontend/src/types/v1/core/events/conversation-state-event.ts:79` |
| `AgentErrorEvent` is a separate event kind with `tool_name`, `tool_call_id`, `error`, surfaced from scaffold | `AgentErrorEvent extends BaseEvent` | `frontend/src/types/v1/core/events/observation-event.ts:51-72` |
| `PauseEvent` is a first-class event kind (no model output parser in repo) | `export * from "./pause-event"` | `frontend/src/types/v1/core/events/index.ts:9` |
| `SystemPromptEvent` is a first-class event kind (no scratchpad prompt code in repo) | `export * from "./system-event"` | `frontend/src/types/v1/core/events/index.ts:10` |
| App server has NO loop, parser, or observation formatter | `find ... -name "*.py"` (218 files) yields zero matches for `agent_loop`, `tool_call`, `tool_calls`, `ActionEvent`, `ObservationEvent` outside the webhook/tests glue above | `openhands/` tree-wide grep |
| Model output parsing is not present in this source | `grep -rn 'react\|ReAct'` returns only GitHub PR reaction noise | tree-wide grep |

## Answers to Dimension Questions

1. **How does the model decide actions?**
   Not visible in this source. `ActionEvent` (`frontend/src/types/v1/core/events/action-event.ts:11`)
   shows the *result* (a single `action`, `tool_name`, `tool_call_id`,
   `tool_call`, `security_risk`, optional `summary`, `llm_response_id`), plus
   `reasoning_content` and `thinking_blocks`. There is no code here that
   parses a model response into an `Action`. The parsing lives in
   `openhands-sdk`/`openhands-agent-server`, which are imported but whose
   source is not in this repository. The `ActionBase` discriminated-union
   payload that the parser produces is referenced only as a re-export
   (`openhands/app_server/event_callback/webhook_router.py:576-583` proxies
   the request). Plan vs default agent kinds both flow through the same
   `StartConversationRequest` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1502`).

2. **How are observations returned?**
   Observations are first-class events with `source: 'environment'`,
   `tool_name`, `tool_call_id` that ties them back to the originating
   `ActionEvent` (`frontend/src/types/v1/core/events/observation-event.ts:6-21`).
   Each observation payload carries tool-specific structured fields (e.g.
   `ExecuteBashObservation.exit_code / .timeout / .error`,
   `frontend/src/types/v1/core/base/observation.ts:57-83`; `GrepObservation.truncated`,
   line 275). Observations stream from the agent server into the app server's
   `webhook_router.on_event` endpoint, are persisted by `EventServiceBase.save_event`,
   and are dispatched to any registered `EventCallback` matching
   `event_kind == 'ObservationEvent'`
   (`openhands/app_server/event_callback/sql_event_callback_service.py:208-214`).
   The frontend renders observation content via dedicated helpers that branch
   on the observation kind and surface `**Output:**`/`**Error:**` markers
   (`frontend/__tests__/components/v1/chat/event-content-helpers/get-observation-content.test.ts:27-95`).

3. **Is reasoning visible or hidden?**
   Both, depending on layer. Reasoning content is *captured* in two places on
   `ActionEvent`: the opaque `reasoning_content: string | null` field for
   reasoning-model intermediate text, and `thinking_blocks:
   (ThinkingBlock | RedactedThinkingBlock)[]` for Anthropic-style structured
   thinking (`frontend/src/types/v1/core/events/action-event.ts:20-25`).
   Reasoning is also expressed as plain-text `thought: TextContent[]` that
   the model emits before the action (`action-event.ts:15`). Whether that
   reasoning is *shown* to the user is a UI decision — the React components
   render it as part of the message stream. The app server itself never
   hides or strips reasoning; it stores the full action event
   (`openhands/app_server/event/event_service_base.py:190-197`). The model
   never sees the reasoning text again on the next step unless the SDK loop
   puts it back into the context; that loop code is not in this source tree.

4. **Are failed observations fed back?**
   Indication only, not implementation. Observation payloads distinguish
   success vs failure structurally (`MCPToolObservation.is_error`,
   `ExecuteBashObservation.error / .timeout`, `BrowserObservation.error`,
   `FinishObservation.is_error`,
   `frontend/src/types/v1/core/base/observation.ts:9-22, 42-55, 57-83, 24-33`).
   There is also a separate `AgentErrorEvent` for scaffold-level errors that
   carry `tool_name`, `tool_call_id`, and `error` as first-class fields
   (`frontend/src/types/v1/core/events/observation-event.ts:51-72`).
   Hook execution failures are surfaced through `HookExecutionEvent` with
   `success: boolean`, `blocked: boolean`, `exit_code`, `reason`
   (`frontend/src/types/v1/core/events/hook-execution-event.ts:40-65`).
   Whether the SDK loop re-prompts the model on these failures is decided
   inside the SDK and is not visible here. The settings layer exposes
   `max_iterations` via `ConversationSettings` from `openhands.sdk.settings`
   (`openhands/app_server/settings/settings_models.py:32-35, 109-110`), but
   the value is forwarded to the agent server, not interpreted by the app
   server.

5. **Can observations be compacted?**
   Yes, via the `LLMSummarizingCondenser` configured in
   `_create_condenser` (`openhands/app_server/app_conversation/app_conversation_service_base.py:578-612`).
   Condensation produces a `CondensationEvent` with `forgotten_event_ids` and
   an optional `summary` (`frontend/src/types/v1/core/events/condensation-event.ts:5-25`),
   and a follow-up `CondensationSummaryEvent` carries the LLM-written summary
   (`condensation-event.ts:36-46`). The condenser runs as a separate LLM
   caller with `usage_id='condenser'` (or `'planning_condenser'` for the
   planning agent) so its cost is tracked independently
   (`app_conversation_service_base.py:598-603`,
   `tests/unit/app_server/test_webhook_router_stats.py:115-122, 237-241`).
   The user can disable it or set `max_size` via `CondenserSettings`
   (`tests/unit/storage/data_models/test_settings.py:78-87`); when
   `max_size` is left None, the SDK default of 240 events is used
   (`app_conversation_service_base.py:594-608`). The actual replacement of
   forgotten events with the summary is performed by the SDK loop, not in
   this repo.

## Architectural Decisions

- **Decoupled agent core**: the Reason/Act/Observe loop is intentionally
  externalized to `openhands-agent-server`, `openhands-sdk`, and
  `openhands-tools` (`pyproject.toml:47-49`). This repo's app server is a
  thin orchestration, persistence, and configuration layer. The trade-off is
  that the cadence is opaque to anyone reading only this repo.
- **Event-sourced persistence**: every step produces a typed event
  (`MessageEvent`, `ActionEvent`, `ObservationEvent`, `AgentErrorEvent`,
  `PauseEvent`, `CondensationEvent`, `HookExecutionEvent`,
  `ConversationStateUpdateEvent`, `SystemPromptEvent` —
  `frontend/src/types/v1/core/events/index.ts:1-10`). Events are stored as
  one JSON file per event id via `EventServiceBase.save_event`
  (`openhands/app_server/event/event_service_base.py:190-197`) and replayed
  for export (`iter_events_for_export`,
  `openhands/app_server/event/event_service_base.py:145-156`).
- **Stateless app server, stateful sandbox**: `send_message` is a pure HTTP
  POST to the agent server inside the sandbox
  (`openhands/app_server/app_conversation/app_conversation_router.py:552-583`).
  The agent server runs the loop locally; the app server only sees the
  events that the agent server chooses to push back via webhook.
- **Hooks modeled separately from actions**: hooks (`PreToolUse`,
  `PostToolUse`, `UserPromptSubmit`, `SessionStart`, `SessionEnd`, `Stop`)
  are surfaced as their own `HookExecutionEvent` with `blocked` and
  `reason` fields (`frontend/src/types/v1/core/events/hook-execution-event.ts:1-100`).
  This means a hook that blocks a tool call becomes a first-class
  observability event rather than being silently embedded in an
  `ObservationEvent` payload.
- **Per-component LLM accounting**: each LLM caller (`agent`,
  `condenser`, `planning_condenser`) gets its own `usage_id` so
  `ConversationStats` can attribute tokens and cost independently
  (`openhands/app_server/app_conversation/app_conversation_service_base.py:598-603`,
  `openhands/app_server/utils/llm_metadata.py:40`,
  `tests/unit/app_server/test_webhook_router_stats.py:101-122`).
- **Action–Observation correlation via shared `tool_call_id`**: every
  `ActionEvent.tool_call_id` and `ObservationEvent.tool_call_id` are
  strings that must match across the pair
  (`frontend/src/types/v1/core/events/action-event.ts:40`,
  `frontend/src/types/v1/core/events/observation-event.ts:20`). This makes
  pairing robust even if events arrive out of order, and is the only
  explicit linkage the schema documents for matching actions to their
  results.

## Notable Patterns

- **Discriminated-union event model**: every event kind has a `kind`
  discriminator (`BaseEvent.kind`), used by both the Python `Event` base
  and the TypeScript `BaseEvent` (`frontend/src/types/v1/core/events/index.ts:1-10`).
- **Webhooks as the only observability seam**: the agent server is treated
  as an opaque worker. The app server relies on `webhook_router.on_event`
  to receive every event after the fact
  (`openhands/app_server/event_callback/webhook_router.py:453-522`). There
  is no in-repo streaming/SSE push from the agent server directly to the
  frontend.
- **Polling for transient agent state**: where webhooks can't supply
  information fast enough (e.g. auto-titling), the app server falls back to
  polling with retry (`SetTitleCallbackProcessor._poll_for_title`,
  `openhands/app_server/event_callback/set_title_callback_processor.py:37-79`).
- **Condenser enabled by default**: `_create_condenser` is always called
  with a configured LLM (even when `condenser_max_size is None`,
  `openhands/app_server/app_conversation/app_conversation_service_base.py:594-608`).
  Users can only disable it via the explicit `enabled=False` flag in
  `CondenserSettings` (`tests/unit/storage/data_models/test_settings.py:126-139`).

## Tradeoffs

- **Strong separation, weak locality**: putting the loop in a separate
  package lets the same loop be reused by the CLI and other front-ends,
  but means the cadence is not auditable from this repo alone. A reviewer
  must clone the SDK to see the parser, the scratchpad, the retry policy,
  or the actual prompt that mixes the previous observation back into the
  next step.
- **Event-sourced persistence is robust but storage-heavy**: each event
  is a separate JSON file
  (`openhands/app_server/event/event_service_base.py:190-197`); there is no
  compaction or batching. Long conversations accumulate tens of thousands
  of files, and `iter_events_for_export` loads them all into memory before
  streaming out
  (`openhands/app_server/event/event_service_base.py:145-156`,
  `export_max_events: int = 10000` at
  `openhands/app_server/app_conversation/live_status_app_conversation_service.py:223`).
- **Webhook-only observability**: missed webhooks mean lost events. There
  is no explicit replay or backfill endpoint in the app server for events
  the agent server emitted but the webhook failed to deliver (compare
  with `_track_conversation_terminal`'s "find last error message" loop,
  `openhands/app_server/event_callback/webhook_router.py:148-153`,
  which silently tolerates missing events).
- **Per-component cost tracking is good, but per-step visibility is
  weak**: stats are aggregated into `usage_to_metrics` keyed by
  `usage_id`, but there is no in-repo surface for "tokens used by step N"
  or "observation cost". Step-level inspection would require opening the
  raw event log.
- **`max_iterations` is opaque to the app server**: the setting is
  passed through to the agent server via `ConversationSettings.create_request()`
  (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1509`),
  but the app server has no way to enforce or even read the current
  iteration count.

## Failure Modes / Edge Cases

- **Missing event webhook**: if the agent server drops an event webhook
  (transient network failure between sandbox and app server), the event is
  lost from the app server's view. Terminal-state analytics loops through
  `events` looking for `last_error` and other state updates
  (`openhands/app_server/event_callback/webhook_router.py:148-153`); a
  missing `execution_status` event means the analytics signal is missed.
- **`SwitchLLMObservation.active_model` propagation race**: the comment
  at `openhands/app_server/event_callback/webhook_router.py:475-481` notes
  that the mid-run model switch only becomes visible to the chat header
  when the `ObservationEvent` carries `active_model`; without that field
  the header would stay stale until the next conversation-info webhook.
- **Hook `blocked=true` semantics**: a hook that blocks an action emits
  a `HookExecutionEvent` with `blocked: true` and `reason`
  (`frontend/src/types/v1/core/events/hook-execution-event.ts:46-59`), but
  how the SDK loop subsequently feeds the rejection back to the model as
  an observation is not visible from this repo.
- **`truncated: true` on Glob/Grep**: indicates the tool returned only the
  first 100 results (`frontend/src/types/v1/core/base/observation.ts:220-244,
  247-276`). The model receives the truncation flag in-band; whether it
  can re-query to recover more is an SDK-loop concern not visible here.
- **Empty condenser LLM**: `apply_server_overrides` sets
  `condenser.llm.usage_id = 'condenser'` even when the main LLM has no
  metadata (`tests/unit/app_server/test_live_status_app_conversation_service.py:996-1010`),
  so a misconfigured agent that lacks a condenser would silently fall back
  to no condensation (the assignment is only made `if
  agent.condenser is not None`,
  `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1352-1356`).
- **Conversation-execution-status terminal inference**: webhook analytics
  treats `ConversationExecutionStatus.ERROR` and `STUCK` as terminal
  (`openhands/app_server/event_callback/webhook_router.py:141-144`); a
  `STUCK` event may indicate the loop is wedged, but the app server has
  no remediation path beyond emitting the analytics event.

## Future Considerations

- **Bring loop into this repo for reviewability**: the cadence is
  arguably the most important part of an agent harness. Sourcing it from
  three external packages with no in-repo reference implementation makes
  audits expensive. A documented pointer to the SDK repository (or
  vendoring) would help.
- **Step-level observability**: events carry enough structure to derive
  per-step timing and cost, but the app server only aggregates stats at
  the conversation level. Surfacing per-step metrics in the UI would help
  debugging without changing the SDK.
- **Event ordering guarantees**: webhook delivery order is not
  documented to be preserved. If reordering becomes a problem,
  `EventServiceBase.save_event` uses `event.id.hex` for the file name
  (`openhands/app_server/event/event_service_base.py:191-197`), so the
  on-disk layout is correct, but in-memory processing inside
  `on_event` (`openhands/app_server/event_callback/webhook_router.py:453-522`)
  depends on `events` arriving in order to correctly chain
  `stats → SwitchLLMObservation → execution_status` updates.
- **Hook schema evolution**: hook event types are hard-coded
  (`openhands/app_server/app_conversation/app_conversation_router.py:1495-1502`).
  Adding a new hook event type requires coordinated changes in the SDK,
  the app server, and the frontend.

## Questions / Gaps

- **Where exactly does the model output get parsed into an
  `ActionEvent`?** No code in this repo. Imported from `openhands-sdk`
  (`openhands/app_server/event_callback/webhook_router.py:57`,
  `openhands/app_server/app_conversation/live_status_app_conversation_service.py:117-131`).
- **Does the SDK loop re-prompt on `is_error`/`error`/timeout
  observations?** Cannot be determined from this source. The hooks
  system implies yes, but the policy is in the SDK.
- **How is `reasoning_content` used on the next step?** Captured in
  `ActionEvent.reasoning_content`
  (`frontend/src/types/v1/core/events/action-event.ts:20`), but its
  consumption on the next step is SDK-internal.
- **What is the actual system prompt that frames each step?** The app
  server injects `<HOST>...</HOST>`, planning-agent boundaries, and
  per-secret prefixes
  (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1592-1609`),
  but the system prompt that contains the ReAct template lives in the
  SDK.
- **Is there a retry counter on failed observations?** No evidence in
  this source. `ObservationEvent` carries the failure but no retry
  metadata; whether the loop retries is SDK behavior.
- **Is there a "scratchpad" prompt or working-memory slot?** No
  `scratchpad` symbol anywhere in the openhands source tree
  (`grep -rn 'scratchpad'` returns nothing relevant). The closest analog
  is `ActionEvent.thought` (model-emitted pre-action text) and
  `extended_content` on `MessageEvent`
  (`frontend/src/types/v1/core/events/message-event.ts:19`).
- **How does the loop terminate besides `FinishObservation`?** The
  schema includes `FinishObservation` (`is_error`, `content`)
  (`frontend/src/types/v1/core/base/observation.ts:24-33`) and
  `ConversationStateUpdateEvent.execution_status`
  (`frontend/src/types/v1/core/events/conversation-state-event.ts:94-97`),
  but the state machine that maps observation → status is in the SDK.
- **What is the `tool_call_id` minting strategy?** Both events carry it
  but no minting code is visible. Implied to come from the LLM provider's
  tool-call id (per `ActionEvent.tool_call_id` doc comment
  `frontend/src/types/v1/core/events/action-event.ts:38-40`).
- **Is `llm_response_id` used to group parallel tool calls?** The
  schema comment says so (`frontend/src/types/v1/core/events/action-event.ts:51-56`),
  but the app server does not consume it; whether the SDK uses it for
  ordering is not visible here.

---

Generated by `dimensions/03.02-reason-act-observe-cadence.md` against `openhands`.
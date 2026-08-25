# Source Analysis: openhands

## 03.09 Completion and Finalization Semantics

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | Python 3.12+ (FastAPI, SQLAlchemy 2 async, Pydantic v2, Litellm 1.84.1, OpenAI SDK 2.33); React/TypeScript frontend. Monorepo: OSS `openhands-ai` package (`openhands/app_server/`, `openhands/analytics/`, `openhands/server/`) plus `enterprise/` SaaS overlay with extra integrations, billing, and analytics persistence. |
| Analyzed | 2026-07-19 |

## Summary

This source ships the OSS `openhands-ai` package and the `enterprise/` SaaS extension. The package pins its core execution layer to external Python packages declared at `pyproject.toml:60-62`:

```
openhands-sdk==1.29.0
openhands-agent-server==1.29.0
openhands-tools==1.29.0
```

Those packages carry the agent loop, the runtime, the `ConversationExecutionStatus` enum, the `is_terminal()` predicate, the LLM `finish_reason`-aware tool-call machinery, the `agent_final_response` endpoint, and the `/ask_agent` side-channel. None of that code lives in this checkout; when the OSS app-server boots, the in-process agent code is imported from those wheels.

As a result, the **completion and finalization semantics visible in this repo are the app-server boundary**: the receiving side. They are concrete but narrow:

1. **Final output schemas** — the only canonical "output" persisted on the app-server is `AppConversationInfo` (`openhands/app_server/app_conversation/app_conversation_models.py:112-184`) plus its `MetricsSnapshot` (`app_conversation_models.py:25,129,191`) and the per-event trajectory exported as zip (`live_status_app_conversation_service.py:2328-2440`). The "agent's final text answer" lives on the agent-server's `/api/conversations/{id}/agent_final_response` endpoint; the app-server treats it as a string blob and does **not** validate it.
2. **Model finish reasons** — not handled here. The `ConversationExecutionStatus` enum is imported, not defined (`app_conversation_models.py:24`). Whether `stop` / `tool_calls` / `length` become `FINISHED`, `STUCK`, or `IDLE` is decided in the SDK package.
3. **Pending tool calls blocking completion** — handled by the SDK loop (`ConversationExecutionStatus.WAITING_FOR_CONFIRMATION` is mapped in the frontend only, `frontend/src/hooks/use-agent-state.ts:32-33`); the app-server has no `pending_tool_calls` concept and no gating on outbox actions.
4. **Usage / cost finalization** — real and well-defined. `AppConversationInfo.metrics: MetricsSnapshot` (`app_conversation_models.py:129`) holds `accumulated_cost`, `max_budget_per_task`, and `accumulated_token_usage.{prompt,completion,cache_read,cache_write,reasoning,context_window,per_turn}_tokens` (rebuilt in `sql_app_conversation_info_service.py:530-544`). Updates arrive either:
   - **Batched, via webhook** on `ConversationStateUpdateEvent(key='stats')` → `SQLAppConversationInfoService.process_stats_event` (`sql_app_conversation_info_service.py:472-512`) → `update_conversation_statistics` (`sql_app_conversation_info_service.py:390-470`); or
   - **Inline** when the agent-server posts a `ConversationInfo` snapshot to `/api/v1/webhooks/conversations` (`webhook_router.py:333-451`) using `conversation_info.stats.get_combined_metrics()` (`webhook_router.py:410`).
   Cost is also surfaced as terminal-state analytics events (`webhook_router.py:111-188`).
5. **Finalization events** — yes, but centralized: the same `ConversationStateUpdateEvent(key='execution_status')` event is consumed in **five** places (`webhook_router.py:499-511`, `slack_v1_callback_processor.py:49`, `jira_v1_callback_processor.py:55`, `jira_dc_v1_callback_processor.py:63`, `gitlab_v1_callback_processor.py:51`, `github_v1_callback_processor.py:51`, `bitbucket_v1_callback_processor.py:51`, `bitbucket_dc_v1_callback_processor.py:51`). Only the webhook handler fans-out for analytics; the integration callbacks each filter `event.value == 'finished'` *string*-equal (not enum) and then call either `/agent_final_response` (Slack, GitHub) or `/ask_agent` (Jira, Jira DC, GitLab, Bitbucket, Bitbucket DC) to fabricate a user-facing message.
6. **Output validation** — `SlackV1CallbackProcessor._get_final_response` raises `RuntimeError('Agent final response was empty')` if the response is whitespace (`slack_v1_callback_processor.py:175-178`); otherwise string-only with no schema, no max-length, no policy. `JiraV1CallbackProcessor` adds zero validation on top of the SDK's `/ask_agent` (`jira_v1_callback_processor.py:153-181`). Final output is **not schema-validated** anywhere.
7. **Incomplete run status** — `AppConversationStartTaskStatus.ERROR` (`app_conversation_models.py:262-272`) records failed start with `detail: str | None`; webhook errors are caught at every callback boundary (`webhook_router.py:519-521`, callback processors: see Slack/GitHub `handle_callback_error`); quota-exceeded is detected by heuristic string match (`webhook_router.py:70-90`).

Done is a multi-typed concept: `ConversationExecutionStatus.is_terminal()` (SDK), `event.value == 'finished'` (string match in callbacks), `AppConversationInfo.acp_server`/title fields populated (`live_status_app_conversation_service.py:479-497`), PostHog `conversation finished` / `errored` (`analytics_service.py:221-283`, `EVENTS.md:24-28`). The model can falsely declare done without the app-server noticing.

## Rating

**Score: 5 / 10**

Rationale:

- Strong, persistent cost/usage storage with idempotent updates and zero-default fallback (`sql_app_conversation_info_service.py:90-99,364-372,447-470,529-537`) — survives sandbox loss.
- Clear terminal-state envelope (`ConversationExecutionStatus.is_terminal()` semantics at `webhook_router.py:506`) with a centralised webhook handler that fans out to analytics + per-conversation callbacks.
- Finalization analytics are typed and well-documented (`analytics_service.py:221-283`, `EVENTS.md:24-28`) with seven PostHog properties including `terminal_state`, `accumulated_cost_usd`, `prompt_tokens`, `completion_tokens`.
- Conversation export captures `meta.json` + per-event JSON (`live_status_app_conversation_service.py:2328-2352`) for replay/trajectory, with SaaS-only Redis lock and configurable `export_max_events` / `export_lock_ttl_seconds` (`live_status_app_conversation_service.py:2465-2483`) — operational safety on a concrete failure surface.
- Frontend maps the V1 status enum to the legacy `AgentState` (`use-agent-state.ts:11-43`) and special-cases archived sandboxes (`use-agent-state.ts:17-19`), so UI states "done", "error", and "archived" are decoupled.
- Tests exercise the stats-event wiring (`tests/unit/app_server/test_webhook_router_stats.py:1-569`), the integration "finished only" filter (`tests/unit/integrations/slack/test_slack_v1_callback_processor.py:147-193`), the `agent_final_response` empty-error path (`tests/unit/integrations/slack/test_slack_v1_callback_processor.py:275`), and the `_classify_error_type` taxonomy in `webhook_router`.

Where it falls short of higher scores:

- The **core loop is in an external pinned wheel**, not in this repo. Findings here describe the *boundary*, not the loop. A reader cannot audit done-ness determinism from this checkout alone — only verify what the app-server observes.
- Integration callbacks all **string-compare** `event.value == 'finished'` against the SDK status string (`slack_v1_callback_processor.py:56`, `jira_v1_callback_processor.py:61`, etc.). Any future SDK rename (`'completed'`, `'idle'`-but-final) silently breaks integration summaries — no enum import, no migration, no test for the value space beyond literal `'finished'`.
- "What counts as done" for outbound integrations is heterogeneous: Slack/GitHub read `/agent_final_response`; Jira family calls `/ask_agent` with a hard-coded `get_summary_instruction()` prompt, meaning the "final answer" posted to Jira is a **second LLM call** synthesised from conversation events, not the agent's last reply. This is by design (`openhands-ai` does not surface the original agent message inline) but it doubles cost per integration and trusts the SDK to keep both endpoints stable.
- The `/agent_final_response` consumer treats the payload as `str(payload.get('response') or '').strip()` — no length cap, no schema, no tool-call fence (`slack_v1_callback_processor.py:175-178`, `github_v1_callback_processor.py:196-199`). A model that emits an entire diff of a 200KB file as its "final response" will be pushed to Slack/GitHub verbatim. No configurable max-bytes.
- `AppConversationStartTaskStatus.ERROR` is reached without a separate `WITH_WARNINGS` state (`app_conversation_models.py:262-272`); warnings are folded into a redacted `detail: str` (`live_status_app_conversation_service.py:537-538`) and a Postgres-style status enum with eight values but no warning axis.
- `is_terminal()` and `ConversationExecutionStatus` come from `openhands.sdk` (`webhook_router.py:56`); this repo does not pin or test that those enum values match what the agent-server actually emits. Frontend/test code hard-codes the lowercase strings `{'finished', 'completed', 'error', 'errored', 'failed', 'stopped'}` in two scripts (`scripts/issue_good_first_issue_check_openhands.py:19-31`, `scripts/issue_duplicate_check_openhands.py:323-329`) — divergent taxonomy.
- `_classify_error_type` uses **keyword string match** on the SDK's `last_error` value (`webhook_router.py:70-90`); misspellings, locale-translated errors, or wrapper text defeat the category. There is no enum-grade error-type contract.
- `live_status_app_conversation_service._build_app_conversations` reads `conversation_info.execution_status if conversation_info else None` (`live_status_app_conversation_service.py:639-641`); a sandbox that loses its `ConversationInfo` mid-cleanup silently downgrades the status to `null` for the UI without flipping the stored record to a terminal state.
- Title setting tolerates agent-server hiccups by retrying on every MessageEvent (`set_title_callback_processor.py:31-79`); the trailing "title still unavailable" path simply leaves the placeholder title `Conversation {id.hex[:5]}` (`live_status_app_conversation_service.py:481`), so a missing or broken title surface is "done-ish".
- `event.value` lookup for `'finished'` is case sensitive; nothing is normalised in either layer.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| External runtime pin (loop lives in wheel, not in repo) | `pyproject.toml` pins `openhands-agent-server`, `openhands-sdk`, `openhands-tools` to `1.29.0` | `pyproject.toml:60-62` |
| `ConversationExecutionStatus` is imported, not defined | enum used to type fields and gate finalization | `openhands/app_server/app_conversation/app_conversation_models.py:24` |
| `is_terminal()` invoked from SDK in webhook finalization | terminal-state predicate | `openhands/app_server/event_callback/webhook_router.py:506` |
| App-server's final-output schema (chat-level) | `AppConversation` (extends `AppConversationInfo`) | `openhands/app_server/app_conversation/app_conversation_models.py:173-184` |
| Metrics snapshot stored on each conversation | `metrics: MetricsSnapshot \| None` on `AppConversationInfo` and `AppConversation` | `openhands/app_server/app_conversation/app_conversation_models.py:129,191` |
| Persistent column schema for cost / token counts | `accumulated_cost`, `prompt_tokens`, `completion_tokens`, `cache_read_tokens`, `cache_write_tokens`, `reasoning_tokens`, `context_window`, `per_turn_token`, `max_budget_per_task` | `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:89-99` |
| Webhook cost collector (info webhook path) | builds metrics from `conversation_info.stats.get_combined_metrics()` | `openhands/app_server/event_callback/webhook_router.py:410` |
| Webhook saves event into `StoredConversationMetadata` (initial) | `merge(stored)` + `commit()` | `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:386-388` |
| Stats-event handler (delta path) | `process_stats_event` -> `update_conversation_statistics` | `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:472-512` |
| Stats handler zeroes nullable tokens instead of trusting the incoming type | `or 0` defaulting | `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:530-537` |
| `last_updated_at` bumped on every stats update | timestamp proxy for "finalization freshness" | `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:467-470` |
| `ConversationStateUpdateEvent(key='execution_status')` consumer | entry point for terminal-status events | `openhands/app_server/event_callback/webhook_router.py:499-511` |
| `ConversationStateUpdateEvent(key='stats')` consumer | entry point for stats events | `openhands/app_server/event_callback/webhook_router.py:469-473` |
| Error classification by string match | taxonomy (`budget_exceeded`, `model_error`, `runtime_error`, `timeout`, `user_cancelled`, `unknown`) | `openhands/app_server/event_callback/webhook_router.py:70-90` |
| Analytics finalization events | `track_conversation_finished`, `track_conversation_errored`, `track_credit_limit_reached` | `openhands/app_server/event_callback/webhook_router.py:146-188` |
| Catalogue of typed finalization events | PostHog event schema docs | `openhands/analytics/EVENTS.md:24-28,89-101` |
| Typed analytics service shape for finished/errored | keyword signatures | `openhands/analytics/analytics_service.py:221-283` |
| Slack "finished-only" filter (string equality) | callback early-returns unless `event.value == 'finished'` | `enterprise/integrations/slack/slack_v1_callback_processor.py:46-57` |
| Slack final-response fetch endpoint | GET `${agent}/api/conversations/{id}/agent_final_response` | `enterprise/integrations/slack/slack_v1_callback_processor.py:149-209` |
| Slack validator rejects empty response | `RuntimeError('Agent final response was empty')` | `enterprise/integrations/slack/slack_v1_callback_processor.py:175-178` |
| Slack validator detail string only — no schema | `final_response = str(payload.get('response') or '').strip()` | `enterprise/integrations/slack/slack_v1_callback_processor.py:175` |
| GitHub "finished-only" filter (string equality) | callback early-returns unless `event.value == 'finished'` | `enterprise/integrations/github/github_v1_callback_processor.py:46-59` |
| GitHub final-response fetch endpoint | GET `${agent}/api/conversations/{id}/agent_final_response` | `enterprise/integrations/github/github_v1_callback_processor.py:170-209` |
| GitHub validator rejects empty response | `RuntimeError('Agent final response was empty')` | `enterprise/integrations/github/github_v1_callback_processor.py:196-199` |
| Jira / Jira DC / GitLab / Bitbucket / Bitbucket DC call `/ask_agent` instead | secondary LLM call to fabricate summary | `enterprise/integrations/jira/jira_v1_callback_processor.py:160-201`, `enterprise/integrations/jira_dc/jira_dc_v1_callback_processor.py:170-221`, `enterprise/integrations/gitlab/gitlab_v1_callback_processor.py:147-185`, `enterprise/integrations/bitbucket/bitbucket_v1_callback_processor.py`, `enterprise/integrations/bitbucket_data_center/bitbucket_dc_v1_callback_processor.py` |
| Jira `_request_summary` builds an extra LLM round-trip | `get_summary_instruction()` is passed to `AskAgentRequest` | `enterprise/integrations/jira/jira_v1_callback_processor.py:142-151` |
| `get_summary_instruction()` defined | prompt template | `enterprise/integrations/utils.py` (`get_summary_instruction` referenced at `enterprise/integrations/jira/jira_v1_callback_processor.py:6`) |
| Frontend typedef for `V1ExecutionStatus` enum (terminal set) | `FINISHED`, `ERROR`, `STUCK` plus intermediate states | `frontend/src/types/v1/core/base/common.ts:67-75` |
| Frontend status-archive fallback | archived (`sandbox_status === 'MISSING'`) reports `STOPPED` | `frontend/src/hooks/use-agent-state.ts:11-43` |
| Frontend uses `STUCK` → `ERROR` mapping | single-state collapse | `frontend/src/hooks/use-agent-state.ts:38-39` |
| Frontend persistence of execution status in Zustand | state store written only on `key='execution_status'` / `key='full_state'` events | `frontend/src/stores/v1-conversation-state-store.ts:18-27`, `frontend/src/contexts/conversation-websocket-context.tsx:451-461,646-657` |
| `ConversationErrorEvent` and `ServerErrorEvent` are displayable errors | per-event types in payload | `frontend/src/types/v1/core/events/conversation-state-event.ts:112-155` |
| `isAgentErrorEvent` distinguishes inline agent errors from environment errors | per-event banner logic | `frontend/src/contexts/conversation-websocket-context.tsx:411-426` |
| `AppConversationStartTaskStatus` enum (start-time finalization) | `WORKING`, `WAITING_FOR_SANDBOX`, `PREPARING_REPOSITORY`, `RUNNING_SETUP_SCRIPT`, `SETTING_UP_GIT_HOOKS`, `SETTING_UP_SKILLS`, `STARTING_CONVERSATION`, `READY`, `ERROR` | `openhands/app_server/app_conversation/app_conversation_models.py:262-272` |
| Start-task failure path keeps a redacted `detail` | `redact_text_secrets(redact_api_key_literals(str(exc)))` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:534-538` |
| Conversation trajectory export as zip (event history product) | `meta.json` + per-event JSON files | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2328-2352` |
| Streaming Zip writer (incremental export) | `_StreamingZipBuffer` adapter | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:147-173` |
| Redis export lock + retry, SaaS-only when `lock_required` | `try_acquire_redis_lock`, `refresh_lock_periodically` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2354-2421` |
| Export size guard | raises `ConversationExportTooLarge` over `export_max_events` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2303-2313` |
| SetTitle callback retries on every MessageEvent | no deadline → "best-effort" forever | `openhands/app_server/event_callback/set_title_callback_processor.py:31-79,130-152` |
| SetTitle callback disables itself once a title lands | self-disabling | `openhands/app_server/event_callback/set_title_callback_processor.py:151-152` |
| Callback execution hits `execute_callbacks` sequentially, not in parallel | per-event iteration | `openhands/app_server/event_callback/webhook_router.py:561-573` |
| Callback execution exceptions are stored as `EventCallbackResult` | persistence of failure | `openhands/app_server/event_callback/sql_event_callback_service.py:233-250` |
| Callback results have `SUCCESS / ERROR` only (no warning axis) | `EventCallbackResultStatus` enum | `openhands/app_server/event_callback/event_callback_result_models.py:11-14` |
| Auto title creation alongside new conversations | saved in `on_conversation_update` | `openhands/app_server/event_callback/webhook_router.py:421-435` |
| Live status aggregation: sandbox-state + server-state | `_build_conversation` joins two sources | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:630-663` |
| Live status degrades to `execution_status: None` if agent-server call fails | swallowed by `httpx.HTTPStatusError` block | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:607-628` |
| Sandbox-status-missing fallback | `SandboxStatus.MISSING` and `execution_status=None` accepted | `openhands/app_server/app_conversation/app_conversation_models.py:174-181` |
| Pricing/quota signal flows via webhook → analytics | `track_credit_limit_reached` only | `openhands/app_server/event_callback/webhook_router.py:166-172` |
| Triage scripts hard-code the terminal status taxonomy | `FAILED = {'error','errored','failed','stopped'}`, `SUCCESSFUL = {'completed','finished'}` | `scripts/issue_good_first_issue_check_openhands.py:19-31`, `scripts/issue_duplicate_check_openhands.py:323-329` |
| Triage scripts call `agent_final_response` | string-only | `scripts/issue_good_first_issue_check_openhands.py:345-355` |
| `agent_final_response` endpoint mention in docs/scripts | pointed at by external callers | `scripts/issue_good_first_issue_check_openhands.py:345-355`, `scripts/issue_duplicate_check_openhands.py:368-374` |
| `SwitchLLMObservation` is the agent-side way to *change* the model during a run | observation type used by webhook to update `llm_model` mid-run | `openhands/app_server/event_callback/webhook_router.py:476-496` |
| Conversation export telemetry | `track_trajectory_downloaded` | `openhands/app_server/app_conversation/app_conversation_router.py:1571-1605` |
| Error message store UX: budget errors persist until LLM works again | "Clear budget error only after a fresh agent event" | `frontend/src/contexts/conversation-websocket-context.tsx:144-159` |
| Test coverage for stats storage | parametrised over `process_stats_event` | `tests/unit/app_server/test_webhook_router_stats.py:97-185,211-376` |
| Test coverage for `_classify_error_type` (the heuristic taxonomy) | unit tests with parameterized strings | (referenced in `tests/unit/app_server/test_webhook_router_stats.py`, see `tests/unit/test_analytics_service.py`) |
| Test coverage for Slack "finished only" filter and final-response retrieval | mocks `_request_final_response` & URL assertion | `enterprise/tests/unit/integrations/slack/test_slack_v1_callback_processor.py:147-193,274-275,291-309` |
| Test coverage for ignored non-`execution_status` keys | explicit assertion | `enterprise/tests/unit/integrations/jira/test_jira_v1_callback_processor.py:85-89` |
| Test coverage for `finished vs running` distinction in GitLab | unit test asserts `value='finished'` triggers summary request | `enterprise/tests/unit/integrations/gitlab/test_gitlab_v1_callback_processor.py:70-86` |
| `app-server` startup wiring of both webhook + REST routers | V1 app server entry | `openhands/app_server/app.py` (imported by `openhands/server/app.py`) |
| Legacy V0 state machine | `AgentState` enum persisted for compatibility | `frontend/src/types/agent-state.tsx:1-23` |
| Empty-events final state | `ExecutionStatus` updated on `key='execution_status'` events only, not back-filled at conversation open | `frontend/src/contexts/conversation-websocket-context.tsx:451-461` |

## Answers to Dimension Questions

1. **What counts as done?**
   - **At the agent-server (in the pinned SDK wheel):** `ConversationExecutionStatus.FINISHED`, `ERROR`, `STUCK` — all return `True` from `is_terminal()` (`openhands/app_server/event_callback/webhook_router.py:506`). The repo does not declare the enum or the predicate.
   - **At the app-server webhook:** same enum, surfaced through `ConversationStateUpdateEvent(key='execution_status', value=<status-string>)` (`webhook_router.py:499-511`). All terminal states are logged; only `FINISHED` triggers outbound integration summaries.
   - **At integrations (Slack/GitHub):** explicitly `event.value == 'finished'` (`slack_v1_callback_processor.py:56`, `github_v1_callback_processor.py:58`). The string identity is the gate; an enum change upstream silently bypasses the summary post.
   - **At integrations (Jira/Jira DC/GitLab/Bitbucket/Bitbucket DC):** same `finished`-only filter, then a secondary `/ask_agent` call (which itself can fail without invalidating the run). `should_request_summary` is a per-callback fire-once guard.
   - **At the start-task lifecycle:** `AppConversationStartTaskStatus.READY` (success) or `ERROR` (terminal failure with `detail: str`) (`app_conversation_models.py:262-272`, `live_status_app_conversation_service.py:534-538`).
   - **At the user-archived view:** any live `execution_status` becomes `AgentState.STOPPED` when `sandbox_status === 'MISSING'` (`frontend/src/hooks/use-agent-state.ts:17-19`). This is a UI override, not a runtime terminal state.

2. **Can the model falsely declare done?**
   Yes, in two senses.
   - At the app-server boundary: an integration callback accepting `event.value == 'finished'` cannot detect that the model emitted a malformed `FINISHED` event without comparing against `is_terminal()` semantics. `ValueError` is caught at every callback boundary and silently turned into an `EventCallbackResultStatus.ERROR` row (`sql_event_callback_service.py:233-250`); the conversation itself is not downgraded.
   - At integration posts: the `/agent_final_response` payload is treated as a final answer with zero validation beyond "is it non-empty after `.strip()`". A model that emits `"Done!"` triggers the success branch on Slack/GitHub (`slack_v1_callback_processor.py:175-178`); a model that emits a 1MB blob also triggers success.

3. **Are final outputs validated?**
   No schema validation anywhere on the app-server side.
   - `SlackV1CallbackProcessor` validates only `len(response.strip()) > 0` and (separately) `httpx.HTTPStatusError` (`slack_v1_callback_processor.py:172-209`).
   - `GithubV1CallbackProcessor` is identical (`github_v1_callback_processor.py:192-209`).
   - `JiraV1CallbackProcessor._ask_question` parses with `AskAgentResponse.model_validate` (one Pydantic-class boundary, no schema); the response text body is posted verbatim (`jira_v1_callback_processor.py:176-201,213-242`).
   - The OpenHands SDK on the agent-server side has the `AgentResponse` schema; this repo does not revalidate it.

4. **Is usage/cost finalized?**
   Yes — and persistently. `StoredConversationMetadata` keeps `accumulated_cost`, prompt/completion/cache tokens, `max_budget_per_task`, `reasoning_tokens`, `context_window`, `per_turn_token` (`sql_app_conversation_info_service.py:90-99`). Updates occur:
   - **On every `ConversationStateUpdateEvent(key='stats')`** — `process_stats_event` → `update_conversation_statistics` writes the latest values conditionally (only when not None) and bumps `last_updated_at` (`sql_app_conversation_info_service.py:447-470`).
   - **On any `ConversationInfo` webhook** — `on_conversation_update` stores `conversation_info.stats.get_combined_metrics()` (`webhook_router.py:410`).
   - **Analytics finalization cost fired only** when the `execution_status` event is terminal (`webhook_router.py:111-188`), giving one final-cost capture per conversation.

5. **Can a run complete with warnings?**
   Partially. The framework has a typed terminal status (`FINISHED`) plus `ERROR`/`STUCK` as failure states, and the event-callback plumbing has `EventCallbackResultStatus.SUCCESS / ERROR` (`event_callback_result_models.py:11-14`) — none of these expose a "CompletedWithWarnings" surface. `AppConversationStartTaskStatus.ERROR` similarly has no `WARNING` axis. Per-callback warnings are logged (`_logger.warning(...)` in `live_status_app_conversation_service.py`, `app_conversation_service_base.py`, etc.) but not exposed at the API or UI layer.

## Architectural Decisions

- **Externalise the loop.** Whole agent runtime (controller, LLM loop, finish-reason handling) was extracted into separately versioned wheels (`openhands-sdk`, `openhands-agent-server`, `openhands-tools`) and pinned per release (`pyproject.toml:60-62`). The app-server is intentionally a thin receiving layer (webhooks, callbacks, storage). The trade-off is reproducibility (pinned wheels) versus auditability (the actual "what counts as done" can't be read in this checkout).
- **Two-source state model.** Live runtime status from the agent-server, persisted status from SQL (`live_status_app_conversation_service._build_conversation`, `_build_app_conversations` at `live_status_app_conversation_service.py:540-587`). The `AppConversation.execution_status` field returns `None` if the agent-server call fails (`live_status_app_conversation_service.py:639-641`).
- **Webhook-driven finalization.** `agent-server` POSTs batches of events to `/api/v1/webhooks/conversations` and `/api/v1/webhooks/events/{conversation_id}` (`webhook_router.py:333-451`); the app-server stores events, recomputes metrics from stats events, and dispatches callbacks in a background task (`webhook_router.py:513-517`). The webhook hand-off decouples terminal-state semantics from local in-process code.
- **EventCallbackProcessor as a discriminated-union plugin.** All status-aware hooks (`SetTitleCallbackProcessor`, `SlackV1CallbackProcessor`, `JiraV1CallbackProcessor`, `GitlabV1CallbackProcessor`, etc.) extend a shared abstract class with `event_kind: ClassVar[EventKind]` (`event_callback_models.py:40-54`). This lets the framework multiplex per-event handling, but each consumer re-implements the same `key == 'execution_status' and value == 'finished'` guard — five copies (`slack_v1_callback_processor.py:46-57`, `jira_v1_callback_processor.py:52-62`, `jira_dc_v1_callback_processor.py:60-70`, `gitlab_v1_callback_processor.py:47-58`, `github_v1_callback_processor.py:46-59`, `bitbucket_v1_callback_processor.py:46-58`, `bitbucket_dc_v1_callback_processor.py:46-58`).
- **Two terminal-response channels.** `/agent_final_response` returns the agent's last assistant text (Slack, GitHub consume it directly: `slack_v1_callback_processor.py:149-209`, `github_v1_callback_processor.py:170-209`); `/ask_agent` takes a question and returns a fresh LLM response (Jira / Jira DC / GitLab / Bitbucket / Bitbucket DC). This produces different "summaries" by integration with no shared validation.
- **Trajectory export is a first-class product.** Locked (Redis in SaaS) streaming export to a ZIP containing `meta.json` plus each event as its own JSON file (`live_status_app_conversation_service.py:2328-2421`). Bounded by `export_max_events: 10_000` by default. This is the closest equivalent to a durable "final answer".
- **Metrics reconciliation skipped.** When `update_conversation_statistics` runs on a stats event, fields are updated **only when not None** (`sql_app_conversation_info_service.py:447-465`) — there is no monotonic guarantee that `accumulated_cost` strictly increases. A late-stats event with a smaller `accumulated_cost` would silently overwrite the persisted peak.

## Notable Patterns

- **Single enum, two identity checks.** The set of valid terminal status strings is *string-compared* in the integration callbacks (`event.value == 'finished'`) rather than enum-compared; the same enum value would have to be re-imported into the app-server for type safety. `scripts/issue_*_check_openhands.py` use `{completed, finished}` and `{error, errored, failed, stopped}` as their own taxonomy. See `scripts/issue_good_first_issue_check_openhands.py:19-31` and `scripts/issue_duplicate_check_openhands.py:323-329`.
- **Background-task callback dispatch.** `_run_callbacks_in_bg_and_close` (`webhook_router.py:561-573`) is fire-and-forget: a `webhook_router.on_event` returns immediately after `asyncio.create_task(...)`, so the response time of the agent-server's batch POST is unaffected by callback latency. The lack of join on the task means there is no "wait for all finalization side-effects" guarantee.
- **Webhook auth via `X-Session-API-Key`.** `valid_sandbox` (`webhook_router.py:235-278`) binds every webhook to the owning sandbox and injects `SpecifyUserContext`; webhook callbacks therefore have a clear authenticated context, and callback execution is granted ADMIN-only user context (`slack_v1_callback_processor.py:230-237`, `jira_v1_callback_processor.py:111-114`).
- **Quoted prompt unification.** The planning agent's hard-coded system-message suffix (`live_status_app_conversation_service.py:185-196`) forbids asking "Ready to proceed?" or running implementation commands — a prompt-level completion contract that makes the planning-agent "done" state explicit at the LLM level.
- **Set-title callback is a polling client.** It sleeps 3s × 4 attempts before giving up for the current MessageEvent (`set_title_callback_processor.py:31-79`), retries on every subsequent MessageEvent, and self-disables only after a successful save. The "done" of titling is `EventCallbackStatus.DISABLED` (`set_title_callback_processor.py:151-152`).
- **Streaming-friendly trajectory export.** `_StreamingZipBuffer` (`live_status_app_conversation_service.py:147-173`) is a small `io.RawIOBase` that only implements `write()` and `tell()`, satisfying `zipfile.ZipFile` for incremental emission. This avoids buffering a full trajectory before the first byte.
- **Webhook-level terminal-event fanout.** `is_terminal()` callers deduplicate per-call to `_track_conversation_terminal` (`webhook_router.py:506-511`). The function itself iterates events searching for `ConversationStateUpdateEvent(key='last_error')` rather than caching last error elsewhere (`webhook_router.py:148-152`), implying last-error is replayed through every webhook batch.

## Tradeoffs

- **Single-line status match is brittle.** All five integration callbacks hard-code `event.value == 'finished'` against a lowercase status literal. A future SDK rename to `'completed'` or `'idle'` (already legal in the V0 `AgentState` taxonomy at `frontend/src/types/agent-state.tsx:1-15` and the V1 `ConversationStatus` union at `frontend/src/types/conversation-status.ts:1-9`) would silently stop the integration summaries with no error path.
- **Final-answer end-point divergence.** `/agent_final_response` (Slack, GitHub) trusts that there *is* a stored final answer; `/ask_agent` (Jira family) calls the LLM again. Both endpoints are imported but only defined in the SDK wheel — the app-server has no recompile-time check that either endpoint's contract matches its assumed payload (`payload.get('response')`).
- **Synthesised summaries double cost.** Every "post to Jira" run triggers a second LLM call with `get_summary_instruction()` (`jira_v1_callback_processor.py:142-151`). The PR-triggered runs typically complete on a tiny model and cheaply, but enterprise-tier runs across many integrations each pay full price twice.
- **Stats-event updates are conditional.** `update_conversation_statistics` only writes fields whose values are not None (`sql_app_conversation_info_service.py:447-465`). Late-arriving stats with smaller numbers will silently lower the persisted peak (e.g., a partial LLM-call retraction). Acceptable for tokens; risky for `accumulated_cost`.
- **No event backlog reservoir.** `WebhookRouter.on_event` saves events to the app-server's `EventService` before processing callbacks (`webhook_router.py:464-466`); storage depends on `EventService` persistence model (`event_service.py:1-74` — abstract). If the app-server crashes between save and callback dispatch, events are stored but finalization side-effects (analytics, summaries) may not fire.
- **Title retries forever.** `SetTitleCallbackProcessor` keeps `status == ACTIVE` and re-runs from every MessageEvent until the agent-server returns a non-empty title (`set_title_callback_processor.py:130-138`). A perpetually "untitled" conversation is a possible final state if the agent-server's `Conversation.title` never populates.
- **Archive = STOPPED, not ERROR.** The frontend returns `AgentState.STOPPED` (not `ERROR`) for archived conversations (`use-agent-state.ts:17-19`). Mis-classification between "user archived" and "agent failed" is now a UI concern, not a backend distinction.
- **Delete is a cancellation, not a finalization.** `ConversationExecutionStatus.DELETING` is treated as an early-return / no-op by `on_conversation_update` (`webhook_router.py:351-352`); the actual SQL row deletion happens later in `_delete_from_database` (`live_status_app_conversation_service.py:2271-2290`). The deletion path does not update analytics via `track_conversation_deleted` — that's a separate SaaS-side hook (`analytics_service.py:285-303`).
- **Redis lock only when SaaS.** The conversation export lock is SaaS-only by default (`live_status_app_conversation_service.py:2323-2326,2465-2483`). OSS deployments get best-effort exports without a server-side mutex — concurrent exports from two operators will both succeed.
- **Slack/GitHub ignore V1 `agent_kind`.** A webhook arriving for an ACP (`acp`) conversation triggers the same Slack callback logic without an `acp_server` awareness check (`slack_v1_callback_processor.py:46-57`).

## Failure Modes / Edge Cases

- **Late stats event with smaller cost reduces persisted peak.** Conditional update in `update_conversation_statistics` (`sql_app_conversation_info.py:447-465`) means non-monotonic drops are silently accepted; downstream dashboards reading `accumulated_cost` will see the latest *write*, not the maximum.
- **String-based terminal filter rejects SDK rename.** `event.value == 'finished'` (`slack_v1_callback_processor.py:56` etc.) breaks silently.
- **`_classify_error_type` is keyword matching.** String variants or translated errors defeat the taxonomy (`webhook_router.py:70-90`). No test for non-ASCII / case-mixed inputs in the visible suite.
- **Empty final-response → integration failure mode, not "stuck".** `RuntimeError('Agent final response was empty')` is raised, the callback records `ERROR`, and PostHog/Slack learn nothing; conversation remains in `FINISHED` state. Consumers can read the error from the `EventCallbackResult` table, but the conversation's `execution_status` is unaffected.
- **Webhook-payload containing stale `last_error`.** `_track_conversation_terminal` walks the *entire* event list for the latest `ConversationStateUpdateEvent(key='last_error')` (`webhook_router.py:148-152`). If the agent replays events with a stale error after a fixed run, the analytics still classify it as errored.
- **Sandbox deleted between start and read.** `live_status_app_conversation_service._build_conversation` (`live_status_app_conversation_service.py:630-663`) substitutes `sandbox_status=SandboxStatus.MISSING` and `execution_status=None`, which the UI then renders as `STOPPED` (`use-agent-state.ts:17-19`). The PostgreSQL record survives, but the live UI loses its terminal status surface.
- **Webhook authentication regresses silently.** `ensure_conversation_found` raises `AuthError` only if the conversation exists and the user mismatch is detected (`webhook_router.py:298-301`); a webhook with a missing conversation *creates a stub* (`webhook_router.py:289-296`), so off-policy webhooks can persist side-effect records.
- **Background callback never joins.** `asyncio.create_task(_run_callbacks_in_bg_and_close(...))` (`webhook_router.py:513-517`) is fire-and-forget; a slow callback holds up later analytics events for that conversation.
- **Webhook delivery during start-up race.** The first inbound `on_event` can arrive before `ConversationInfo` has been registered via `on_conversation_update` (`webhook_router.py:333-451`); `valid_conversation` falls back to stub creation (`webhook_router.py:289-296`), which means partial analytics attribution by `created_by_user_id`.
- **Set-title callback keeps polling.** `SetTitleCallbackProcessor` retries on every MessageEvent with no deadline. A multi-day conversation with no MessageEvent that lands a title will be permanently titled `Conversation {id.hex[:5]}`.
- **Switch-LLM mid-run not finalization-aware.** `webhook_router.py:476-496` updates the conversation's recorded `llm_model` from `SwitchLLMObservation` without flushing cost or analytics; the post-switch run cost is accumulated onto the new `llm_model` column.
- **No idempotency on `track_conversation_finished`.** The webhook handler doesn't de-duplicate by `(conversation_id, terminal_state)`; multiple webhook deliveries of the same `execution_status` event would emit multiple PostHog events. Tests do not assert uniqueness.
- **ConversationExportTooLarge only checks event count.** A small conversation with large event payloads can blow export memory (`live_status_app_conversation_service.py:2304-2313`). There is no byte-budget check.

## Future Considerations

- **Move integration callbacks behind a typed reducer.** The five near-identical "finished-only" filters could collapse into one `TerminalConversationCallback` base class that imports `ConversationExecutionStatus` and asserts `is_terminal() and value is FINISHED`. See `enterprise/integrations/{slack,jira,jira_dc,gitlab,github,bitbucket,bitbucket_data_center}/.../v1_callback_processor.py`.
- **Adopt a real "final answer" contract on the agent-server.** Replace `payload.get('response')` with a typed model and a length cap (e.g., 16 KB for Slack, 64 KB for GitHub). Add a `whatsapp`-equivalent discrimination between the agent's *actual last message* and a synthesised summary.
- **Convert cost-finalization to a "max-watermark" semantic.** Drop the conditional update and instead write `stored.accumulated_cost = max(stored.accumulated_cost or 0, new_value)` to defend against late-stats retraction.
- **Deprecate `/ask_agent` for summaries.** Use a deterministic extractor over the last `MessageEvent(source='agent')` instead of re-prompting the LLM.
- **Persist last-known `execution_status` separately from live agent-server state.** Currently `AppConversation.execution_status` is read directly from the live sandbox; a sandbox outage erases the terminal state from the UI even though `StoredConversationMetadata.last_updated_at` was bumped on the last stats event.
- **Promote `SetTitleCallbackProcessor` to a deadline.** Cap retries to N MessageEvents or M seconds, then either disable or fall back to a hash of `id`.
- **Distinct `track_conversation_finished_idempotency_key`.** Use a hash of `(conversation_id, terminal_state, last_updated_at)` to deduplicate PostHog events.
- **Make export lock mandatory when `app_mode == 'saas'` (defaulted today).** Currently toggled by env; raise this to a hard error in SaaS so the lock can't be disabled accidentally.

## Questions / Gaps

- **What *exactly* does the agent-server send as `ConversationInfo.execution_status` on completion?** Only the SDK enum members (`FINISHED`, `ERROR`, `STUCK`, `DELETING`) are referenced — what does the runtime do for sandbox-disconnect? No evidence found in this repo. The pinned wheel `openhands-sdk==1.29.0` would need to be inspected separately.
- **Does the SDK keep `finish_reason` on `MessageEvent`, and is it surfaced to the app-server?** No `finish_reason` reference in this repo; the agent-server's `/api/conversations/{id}/events` shape is not visible here. The frontend only sees `ConversationStateUpdateEvent` values, never raw `MessageEvent.llm_response.choices[*].finish_reason`.
- **Is the `MetricsSnapshot` an SDK-emitted object that includes both agent and condenser metrics?** `sql_app_conversation_info_service.py:399-401` reads `usage_to_metrics.get('agent')` only. The `condenser` and any third-party (e.g. router) metrics are silently dropped.
- **What happens when the agent-server returns `FINISHED` but the conversation has unfired tool calls?** The app-server has no concept of `pending_tool_calls`. Either the SDK loop guarantees no outbox remains at the moment of `FINISHED`, or there is an unobserved gap. No evidence in this repo.
- **Does the SDK ever emit a 'cancelled' / 'killed' status, or only `FINISHED` / `ERROR` / `STUCK`?** Hard to verify from this checkout; the integration callbacks only react to `'finished'`, leaving the rest as silent ignore. No test exercises `'error'`, `'errored'`, `'failed'`, `'stopped'` for any callback.
- **Are long-running conversations at risk of webhook-event-history growing unboundedly?** `EventService.save_event` is abstract (`event_service.py:60-62`); concrete implementations are in `filesystem_event_service.py`, `aws_event_service.py`, `google_cloud_event_service.py`, etc., which are themselves thin. No retention/pruning visible in this repo.
- **How does `_track_conversation_terminal` react to multi-batch arrivals?** Each webhook deliver can re-fire the terminal-state analytics if the agent-server emits the `execution_status` event again. No idempotency layer is visible.
- **Does the agent-server's `/agent_final_response` reflect the latest `MessageEvent` always, or a stored snapshot?** Slack/GitHub depend on this, but the contract is undocumented in this repo. Tests mock it.
- **Does the planner (`AgentType.PLAN`) ever produce its own final answer?** Planning agent is configured to refuse execution (`live_status_app_conversation_service.py:185-196`), but how the user-of-a-planning-conversation signals "make this an implementation conversation" is implicit ("click Build button").
- **What triggers `ConversationExecutionStatus.STUCK`?** Heuristic match for `'timeout' / 'timed out' / 'budget' / 'cancel' / 'model'` covers four-fifths of the cases (`webhook_router.py:70-90`), but the SDK almost certainly has its own detection (e.g., repeated identical actions or empty LLM responses). Not visible here.
- **How is `last_updated_at` used downstream?** A bump at every stats event exists (`sql_app_conversation_info_service.py:467-470`), but no read-side use of this field is visible in this checkout.
- **What happens with non-string `event.value` for `execution_status`?** The wrapper expects a str-convertible value (`webhook_router.py:505`: `ConversationExecutionStatus(event.value)`); a richer object would raise `ValueError`, which is swallowed (`webhook_router.py:510-511`).
- **Are ACP-mode completions observable in the same way?** `agent_kind == 'acp'` adjusts trigger and webhook-side fields (`webhook_router.py:369-389`, `live_status_app_conversation_service.py:464-477`), but the terminal-state fan-out is the same. Whether ACP's terminal answer is accessible via `/agent_final_response` is unverified.

---

Generated by `03.09-completion-and-finalization-semantics` against `openhands`.

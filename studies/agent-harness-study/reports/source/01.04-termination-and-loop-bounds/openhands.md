# Source Analysis: openhands

## Termination and Loop Bounds

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | Python 3.12 (FastAPI app server) + React/TypeScript frontend; agent loop in separately-installed `openhands.sdk` and `openhands.agent_server` packages |
| Analyzed | 2026-07-02 |

## Summary

The OpenHands source tree under inspection is the **app server** (`openhands/app_server/`) and the **V0 server shim** (`openhands/server/`). Neither contains the agent's own iteration loop. The loop, `max_iterations` enforcement, stuck detection, and budget enforcement all live in the `openhands.sdk` package (`software-agent-sdk` repo, referenced by the code but not present in this source tree — only imported). The app server's contribution to termination/loop-bounds is:

1. **Wiring of loop-bound settings.** `Settings.conversation_settings.max_iterations` and `Settings.max_budget_per_task` are persisted (`openhands/app_server/settings/settings_models.py:127, 134`), flow through `_build_start_conversation_request_for_user` / `_build_acp_start_conversation_request`, and are pushed into the SDK's `StartConversationRequest` via `ConversationSettings.create_request()` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1706-1731, 1903-1923`).
2. **Terminal-state surface.** The app server imports `ConversationExecutionStatus` from the SDK (`openhands/app_server/app_conversation/app_conversation_models.py:24`) and surfaces it as `AppConversation.execution_status` (`:178`). The `is_terminal()` method is used at the webhook boundary (`openhands/app_server/event_callback/webhook_router.py:506`).
3. **Webhook-driven analytics on terminal events.** A `ConversationStateUpdateEvent` with `key='execution_status'` and a terminal value triggers `track_conversation_finished` or `track_conversation_errored` (`openhands/app_server/event_callback/webhook_router.py:111-188, 498-511`). Error category is a string match on the last `last_error` event (`webhook_router.py:70-90`).
4. **Operational bounds unrelated to the agent loop.** Sandbox capacity, sandbox startup, and per-sandbox conversation count are bounded in the app server (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:212-214, 2444-2453`); conversation `SendMessage` is gated on `SandboxStatus.RUNNING` (`openhands/app_server/app_conversation/app_conversation_router.py:508-529`).

Because the actual termination logic, stuck detector, iteration counter, and budget enforcer are in the imported SDK, the source tree itself **does not implement loop bounds** — it only **forwards** them. The quality bar is therefore about whether the configuration is correctly wired, persisted, and surfaced to the user, not about the internal termination algorithm. On those axes the wiring is coherent and tested; on the dimension of "how does the loop stop" the source offers no direct evidence beyond the SDK contract.

## Rating

**5 / 10** — Coherent, well-tested configuration pipeline (`max_iterations`, `max_budget_per_task`, terminal-state webhook), but the source contains **no implementation** of the loop itself, no stuck-loop detector, and no termination policy. The actual enforcement lives in the separately-installed `openhands.sdk` package, which is imported but not present in this source tree. Score reflects: present-but-weak (executed by code you don't own), partial observability (terminal state is a string code, no structured exhaustion reason), and no visible tests for the loop-bound or stuck-detector behavior in this repo.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| `max_iterations` documented in legacy V0 config | `#max_iterations = 500` | `config.template.toml:56` |
| `max_budget_per_task` documented in legacy V0 config | `#max_budget_per_task = 0.0` | `config.template.toml:53` |
| `max_concurrent_conversations` / `conversation_max_age_seconds` documented | legacy V0 config (not surfaced in app server) | `config.template.toml:91, 94` |
| `conversation_settings` docstring mentions `max_iterations` | persistence shape | `openhands/app_server/settings/settings_models.py:108-110` |
| `Settings.max_budget_per_task` field | top-level, not under `conversation_settings` | `openhands/app_server/settings/settings_models.py:127` |
| `Settings.conversation_settings: ConversationSettings` | Pydantic field with `default_factory=ConversationSettings` | `openhands/app_server/settings/settings_models.py:134-136` |
| Conversation settings round-trip through `update()` (deep merge) | applies `conversation_settings_diff` | `openhands/app_server/settings/settings_models.py:230-240` |
| `max_iterations` passes through to agent server via `create_request` | `# to create_request() so that max_iterations, confirmation_mode, and ...` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1900-1918` |
| `max_iterations` passed in LLM-path `create_request` | `conv_settings = user.conversation_settings.model_copy(...)` then `conv_settings.create_request(...)` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1706-1731` |
| `max_iterations` test plumbing | `_TestUserInfo.conversation_settings` constructs `ConversationSettings(... max_iterations=...)` | `tests/unit/app_server/test_live_status_app_conversation_service.py:127-135` |
| `max_iterations` round-trip through settings API | test asserts `cs['max_iterations'] == 100` | `tests/unit/app_server/test_settings_api.py:171-201` |
| Settings API rejects `max_iterations` of 5 via POST | test uses `{'conversation_settings': {'max_iterations': 5}}` | `tests/unit/app_server/test_settings_api.py:227` |
| `max_budget_per_task` persisted as DB column | `max_budget_per_task: Mapped[float \| None]` | `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:94` |
| `max_budget_per_task` DB migration | Alembic column add | `openhands/app_server/app_lifespan/alembic/versions/001.py:175` |
| `max_budget_per_task` updated from agent-server `stats` event | `stored.max_budget_per_task = agent_metrics.max_budget_per_task` | `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:426-451` |
| `max_budget_per_task` round-trip test | `stored.max_budget_per_task = 5.0` then unchanged on `None` | `tests/unit/app_server/test_webhook_router_stats.py:168, 187, 302-323` |
| Conversation metrics snapshotted on save | `accumulated_cost`, `prompt_tokens`, etc. persisted on every save | `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:350-388` |
| `execution_status` field on `AppConversation` | `execution_status: ConversationExecutionStatus \| None` | `openhands/app_server/app_conversation/app_conversation_models.py:178-181` |
| `execution_status` populated by merging sandbox `ConversationInfo` | `_build_conversation` copies from `conversation_info.execution_status` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:630-663` |
| Terminal-state detection at webhook | `if exec_status.is_terminal():` | `openhands/app_server/event_callback/webhook_router.py:506` |
| `STUCK` is treated as a terminal error class | `is_error = exec_status in (ERROR, STUCK)` | `openhands/app_server/event_callback/webhook_router.py:141-144` |
| Error category classifier (string match) | `budget_exceeded / model_error / runtime_error / timeout / user_cancelled / unknown` | `openhands/app_server/event_callback/webhook_router.py:70-90` |
| `conversation finished` analytics event | captures `terminal_state`, `accumulated_cost_usd`, `prompt_tokens`, `completion_tokens`, `llm_model` | `openhands/analytics/analytics_service.py:221-253` |
| `conversation errored` analytics event | captures `error_type`, `error_message`, `terminal_state` | `openhands/analytics/analytics_service.py:255-283` |
| Analytics terminal-state event catalog | `terminal_state` is the SDK enum string; STUCK is a valid value | `openhands/analytics/EVENTS.md:24-25, 35` |
| Slack callback uses SDK `execution_status` to decide if work is "finished" | `if event.value != 'finished': return None` | `enterprise/integrations/slack/slack_v1_callback_processor.py:49-57` |
| App server `SendMessage` rejects non-RUNNING sandbox | returns 409 with `Sandbox is {status}` | `openhands/app_server/app_conversation/app_conversation_router.py:508-529` |
| App server `SendMessage` rejects ERROR sandbox with 503 | explicit | `openhands/app_server/app_conversation/app_conversation_router.py:515-519` |
| App server `SendMessage` rejects MISSING/archived sandbox with 410 | explicit | `openhands/app_server/app_conversation/app_conversation_router.py:508-513` |
| Sandbox startup timeout (operational) | `sandbox_startup_timeout: int = Field(default=120)` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2444-2446` |
| `max_num_conversations_per_sandbox` (capacity, not loop bound) | defaults to 20 | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2450-2453` |
| Sandbox status enum used as a hard pre-condition on writes | `SandboxStatus.RUNNING` gate | `openhands/app_server/app_conversation/app_conversation_router.py:521-528` |
| No `recursion_limit`, no `loop_detector`, no `stuck_detector` in source | search returned no matches | `grep -r "recursion_limit\|loop_detect\|stuck_detector" openhands/` → 0 matches |
| `MAX_ITERATION` / `max_steps` / `max_turns` not in source | search returned 0 | `grep` of source for these names returned only `max_iterations` (settings) |
| `max_concurrent_conversations` / `conversation_max_age_seconds` only in config template, no Python enforcement | only template comments | `config.template.toml:91, 94`; `grep` of `openhands/` returned 0 matches |
| `enable_default_condenser` (LLM context overflow protection) only in config template | no Python enforcement found | `config.template.toml:88`; `grep` of `openhands/` returned 0 matches |

## Answers to Dimension Questions

1. **What stops the loop?**
   Inside this source tree: nothing. The actual stop is in the imported `openhands.sdk` package, signaled to the app server as a `ConversationExecutionStatus` value (`FINISHED`, `ERROR`, or `STUCK`) on the `execution_status` field of `ConversationInfo` (`openhands/app_server/event_callback/webhook_router.py:141-144, 506`). The app server reacts to that webhook but does not produce the event.

2. **Are limits configurable?**
   Yes for what the app server owns: `max_iterations` and `max_budget_per_task` are user-settable through the settings API (`openhands/app_server/settings/settings_models.py:127, 134-136`, `tests/unit/app_server/test_settings_api.py:171-201`). Defaults are provided by the SDK's `ConversationSettings` default factory. The legacy V0 knobs `max_concurrent_conversations`, `conversation_max_age_seconds`, and `enable_default_condenser` are documented in `config.template.toml:88-94` but have no Python enforcement code paths in this source.

3. **Is exhaustion treated differently from success?**
   In the app-server analytics path: yes. `_track_conversation_terminal` branches on `exec_status in (ERROR, STUCK)` vs everything else (`openhands/app_server/event_callback/webhook_router.py:141-188`). Successful finish goes to `track_conversation_finished`; error/stuck goes to `track_conversation_errored` plus, if the string-matcher classifies the error as `budget_exceeded`, `track_credit_limit_reached` (`:146-172`). The Slack integration only posts a summary for `event.value == 'finished'` (`enterprise/integrations/slack/slack_v1_callback_processor.py:56-57`). The raw enum is persisted as `terminal_state` in analytics properties.

4. **Are stuck loops detected before the hard limit?**
   The app server treats `STUCK` as a terminal class (`:141-144`) but **does not implement** the detection. Stuck detection is a SDK responsibility; this source contains no `stuck_detector`, no `loop_detector`, no event-pattern scan, and no repetition hash. A search for `recursion_limit`, `loop_detect`, `stuck_detector` across `openhands/` returns zero matches.

5. **Does the user get a useful final state?**
   Partially. The user-facing `AppConversation.execution_status` is `None` when the sandbox is not `RUNNING` (archived case) and otherwise reflects the SDK-reported enum (`openhands/app_server/app_conversation/app_conversation_models.py:178-181`, `live_status_app_conversation_service.py:630-663`). The frontend differentiates `STOPPED` + `isArchived` for archived conversations (per repo `AGENTS.md` "Archived Conversations" section). The exact `last_error` event value is captured in the analytics `error_message` (truncated to 500 chars, `webhook_router.py:151`), but is not exposed in the `AppConversation` API response.

## Architectural Decisions

- **Loop is owned by the SDK, not the app server.** The app server treats the agent loop as a black box inside the sandbox and consumes its terminal status via a webhook. This keeps the app server stateless w.r.t. iteration and concentrates termination logic in one place.
- **`max_iterations` is a *Conversation* setting, not a *Session* setting.** It lives under `Settings.conversation_settings.max_iterations` (`:134`) and is forwarded into the SDK's `ConversationSettings` rather than the agent itself; the SDK is responsible for the actual counter (`openhands/app_server/settings/settings_models.py:108-110`).
- **`max_budget_per_task` is a top-level Settings field** (`:127`), not under `conversation_settings`. It is mirrored to a dedicated DB column (`sql_app_conversation_info_service.py:94`) and updated from each `stats` event the agent server emits (`:390-456`).
- **Terminal states are an enum from the SDK.** `ConversationExecutionStatus` is imported, not re-defined (`app_conversation_models.py:24`). The local model accepts it as-is and exposes it as `AppConversation.execution_status`.
- **Error classification is best-effort string match.** `_classify_error_type` (`:70-90`) pattern-matches the last `last_error` event value into broad buckets; this is documented as a deliberate per-CONTEXT.md decision (`:74`).
- **Sandbox capacity bounds are independent of the agent loop.** `max_num_conversations_per_sandbox` and `sandbox_startup_timeout` exist for multi-tenant capacity control, not loop termination (`live_status_app_conversation_service.py:212-214, 2444-2453`).

## Notable Patterns

- **Pluggable agent kind** — `_build_start_conversation_request_for_user` (LLM agent) and `_build_acp_start_conversation_request` (ACP agent) both reach `conv_settings.create_request(...)` so the same `max_iterations` channel flows to both. The comment at `live_status_app_conversation_service.py:1900-1902` calls this out explicitly to prevent drift.
- **Reactive terminal handling via webhook** — `_track_conversation_terminal` is fire-and-forget (`asyncio.create_task` is used at `webhook_router.py:513-517` for callback dispatch); the webhook returns `Success()` even on internal failure, guarded by `try/except Exception` (`:519-520`).
- **Settings reconciliation in `update()`** — `update()` deep-merges `conversation_settings_diff` and explicitly rejects the legacy flat `conversation_settings` key to prevent silent drops (`settings_models.py:193-200, 230-240`).
- **Operational gates on writes** — every "do something to a conversation" call (`SendMessage`, `SwitchLLM`, profile switch, git state read) checks `SandboxStatus.RUNNING` first (`app_conversation_router.py:508-529, 716, 818, 1204`), surfacing 409/410/503 for non-running, archived, and errored sandboxes respectively. This is *not* loop-bounds but is the closest the app server gets to "stop accepting work."

## Tradeoffs

- **Centralized loop logic, decentralized wiring.** The app server cleanly forwards loop-bound settings but is a thin shell over the SDK. Anyone analyzing termination must leave this source tree; the only artifact here is the enum and the analytics taxonomy.
- **Terminal state as opaque string.** `is_terminal()` returns `True/False`; the enum value (`finished`, `error`, `stuck`) is forwarded unchanged to analytics as `terminal_state` (`webhook_router.py:163, 179`). There is no structured `reason`/`exhausted_iterations`/`budget_exhausted` field in the local model.
- **Error classification by string match** is fragile (`webhook_router.py:70-90`) — a model that emits "budget" inside a longer message gets mis-classified as `budget_exceeded`.
- **No `max_concurrent_conversations` / `conversation_max_age_seconds` enforcement in this source** despite being documented in `config.template.toml:91, 94`. These V0 knobs appear to be honored only by the V0 server or by the SDK, not the V1 app server.
- **The `last_error` message is captured only for analytics** (truncated to 500 chars, `webhook_router.py:151`), not exposed on the `AppConversation` API response. The user-visible state is the enum, not the message.

## Failure Modes / Edge Cases

- **SDK missing the `execution_status` field**: `AppConversation.execution_status` is `None` whenever the sandbox is not `RUNNING` (`app_conversation_models.py:178-181`). The frontend treats `sandbox_status === "MISSING"` as "archived" and disables interactive UI (per `AGENTS.md` "Archived Conversations" section). If the SDK ever emits a non-enum string, the webhook's `try/except` swallows the conversion error (`:510-511`).
- **`SendMessage` during stuck**: The HTTP layer does not inspect `execution_status`; it only checks `SandboxStatus` (`app_conversation_router.py:508-529`). A conversation whose agent is `STUCK` but whose sandbox is still `RUNNING` will accept new messages, which the agent-server will then likely reject or buffer.
- **Webhook ordering**: A `STUCK` event is treated as `error` and routed to `track_conversation_errored`, but the same `execution_status` value could in theory arrive in a `ConversationStateUpdateEvent` after the sandbox has already been reaped, in which case the analytics call hits a missing `app_conversation_info`. There is no visible guard for this race.
- **Stuck detection missing in this source**: if the SDK version pinned in production does not implement stuck detection, the only safety net is `max_iterations` or `max_budget_per_task`. There is no app-server-level fallback.
- **`max_budget_per_task` not refreshed to DB on every event**: `update_conversation_statistics` only updates fields that are not `None` (`sql_app_conversation_info_service.py:447-456`). A `None` from the agent server will leave stale values in the DB.
- **Settings apply only at conversation start**: there is no evidence in this source of mid-conversation updates to `max_iterations` or `max_budget_per_task` taking effect; the SDK would have to honor a live update path, which is not visible here.

## Future Considerations

- **Surface `last_error` (or a structured `termination_reason`) on `AppConversation` API** so the frontend can show "stopped: budget exceeded" rather than just `execution_status = STUCK`.
- **Promote the error classifier from string match to a structured payload** — the agent server should emit `error_category: budget_exceeded | stuck | timeout | ...` and the app server should pass it through. The string match in `webhook_router.py:70-90` is the most fragile part of the termination pipeline.
- **Wire `max_concurrent_conversations` / `conversation_max_age_seconds` / `enable_default_condenser`** to runtime enforcement in the V1 app server or delete them from the config template to avoid documentation drift (`config.template.toml:88-94`).
- **Block writes when `execution_status` is terminal** (or at least surface a clear error). Currently `SendMessage` only checks `SandboxStatus` (`app_conversation_router.py:508-529`).
- **Add tests in this repo for the wiring** — there are no tests for `_track_conversation_terminal`'s `STUCK` branch or for the `budget_exceeded` -> `credit_limit_reached` chain.

## Questions / Gaps

- **What is the default value of `max_iterations`?** Not visible in this source. The settings docstring mentions it (`settings_models.py:108-110`) but the default is owned by `openhands.sdk.ConversationSettings`, which is imported (`settings_models.py:32-40`). `tests/unit/app_server/test_live_status_app_conversation_service.py:127-135` only tests explicit values; default-value behavior is not asserted.
- **Where is `enable_default_condenser` / `conversation_max_age_seconds` / `max_concurrent_conversations` enforced?** Documented in `config.template.toml:88-94`, no Python reference in this source. Either the V0 server or the SDK owns them; this source does not.
- **Does the SDK expose a structured "I stopped because `max_iterations`" reason?** Only the `terminal_state` enum string is forwarded; no `termination_reason` or `exhausted_resource` field is visible.
- **Are `FINISHED` and `IDLE` distinguishable from the app server's perspective?** `is_terminal()` is used (`webhook_router.py:506`) but the comment in the SDK (visible only via grep of imports) suggests `IDLE` is non-terminal. This source does not assert or test the distinction.
- **How is "user cancelled" represented?** The string-matcher returns `user_cancelled` for messages containing `cancel` (`webhook_router.py:83-84`), but no code path in this source explicitly emits a cancellation event. Cancellation must originate in the SDK or the agent server.
- **Is there any path for the app server to *force* termination** (e.g., admin "stop conversation" button)? No `stop` / `interrupt` / `cancel` endpoint is registered in `app_conversation_router.py` (the only delete-style route is `DELETE /{conversation_id}` at `:892`, which deletes the conversation metadata but does not signal the running agent loop). Searched: no `POST /conversations/{id}/stop` or similar.

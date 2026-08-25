# Source Analysis: openhands

## 01.01 Execution Model Taxonomy

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `sources/agent-harness-study/sources/openhands` |
| Language / Stack | Python (FastAPI app_server), Docker containers running external `openhands-sdk` agent_server |
| Analyzed | 2026-07-02 |

## Summary

This OpenHands checkout has been refactored to a thin orchestration app_server over a separate agent runtime. The repository contains **no agent loop, controller, or runtime** of its own: there is no `AgentController`, no `step()` method, no in-process ReAct loop, no `Runtime`, and no event-sourced turn machine inside `openhands/`. All advancement of agent execution happens out of process inside the `openhands-sdk` package (`openhands.agent_server`, pinned `openhands-sdk==1.29.0` in `pyproject.toml:61`), which is launched inside a sandbox (Docker container, remote runtime, or local Python process).

From the perspective of *this* repository's app_server, the execution model is a **multi-model hybrid**:

1. **Outer model: streaming async-generator state machine** for conversation startup. The `_start_app_conversation` generator (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:309`) drives a `AppConversationStartTask` through an explicit `AppConversationStartTaskStatus` FSM (`openhands/app_server/app_conversation/app_conversation_models.py:262`: WORKING → WAITING_FOR_SANDBOX → PREPARING_REPOSITORY → RUNNING_SETUP_SCRIPT → SETTING_UP_GIT_HOOKS → SETTING_UP_SKILLS → STARTING_CONVERSATION → READY/ERROR). Each `yield task` is a persisted state transition; the router streams those to the client (`live_status_app_conversation_service.py:300`, `app_conversation_router.py:1633` `_stream_app_conversation_start`).

2. **Agent-runtime model: out-of-process persistent conversation server.** The actual LLM/tool loop lives in the spawned `agent_server` process (Docker container via `DockerSandboxService.start_sandbox` at `docker_sandbox_service.py:484`, or process via `ProcessSandboxService._start_agent_process` at `process_sandbox_service.py:110`). That process owns the per-conversation event stream, agent state, and tool execution. The app_server only sees it via REST + webhooks.

3. **Event integration: webhook-driven fanout with polling fallback.** The agent_server pushes `ConversationInfo` and per-event payloads to `POST /api/v1/webhooks/conversations` (`webhook_router.py:333`) and `POST /api/v1/webhooks/events/{conversation_id}` (`webhook_router.py:453`). Webhook handlers save events to disk (`event/event_service_base.py:190` `save_event`) and invoke registered `EventCallbackProcessor`s (`event_callback/sql_event_callback_service.py:208` `execute_callbacks`). When webhooks are unavailable (no public `web_url`), `RemoteSandboxServiceInjector.inject` lazily starts one global polling task (`remote_sandbox_service.py:926`, function `poll_agent_servers` at `remote_sandbox_service.py:682` with `while True` and `asyncio.sleep(sleep_interval)` at line 765).

4. **No monolithic main loop.** There is no `if __name__ == "__main__"` in this repo for the agent. The runtime entrypoint is `uvicorn openhands.server.listen:app` (`Makefile:262`), which is the FastAPI app (`openhands/server/listen.py:6` → `openhands/app_server/app.py:54`). All agent execution is delegated to sidecar processes.

The "one sentence" answer to what advances execution: **a webhook (or, in dev/no-public-URL mode, a polling loop) from the external `openhands-sdk` agent_server process fires an HTTP callback into the app_server, which persists a batch of events and runs any registered `EventCallbackProcessor`s, while the agent loop itself ticks entirely inside the sandboxed agent-server process.**

## Rating

**6 / 10** — Present but split across models.

Rationale:
- The orchestration model is **explicit, documented in a published FSM** (`AppConversationStartTaskStatus` at `app_conversation_models.py:262`), and the streaming endpoint (`app_conversation_router.py:1633`, `live_status_app_conversation_service.py:300`) is easy to follow.
- However, the *real* agent loop is in a separate package (`openhands-sdk==1.29.0` in `pyproject.toml:61`). Anyone reading this repo alone cannot find it; you must follow the `DockerSandboxService` image name (`docker_sandbox_spec_service.py:38`), the process-spawn command (`process_sandbox_service.py:441`, `agent_server_module='openhands.agent_server'`), or the `agent_server_url` field on `SandboxInfo`. The boundary is real but the seam is leaky (e.g., `live_status_app_conversation_service.py:429-430` injects `user_id` into the JSON body because the pinned SDK doesn't yet expose it on `StartConversationRequest`).
- Operational safeguards exist (sandbox startup timeout + poll at `live_status_app_conversation_service.py:866-872`; `_verify_agent_server_version` at line 874 to fail-fast on SDK drift; background polling as webhook fallback at `remote_sandbox_service.py:925-932`; Redis locks for export at `live_status_app_conversation_service.py:2315-2326`).
- The model is **not yet "mature/durable"** under failure: polling is started lazily with no centralized supervisor (`remote_sandbox_service.py:924-926`), there's no retry queue for failed event webhooks, the "while True" in `poll_agent_servers` at `remote_sandbox_service.py:699` is long-lived with no cancellation contract outside `CancelledError`, and there's documented duplication risk between the SDK's own notion of start-task state and the app_server's persisted `AppConversationStartTask` (see the comment block at `live_status_app_conversation_service.py:418-430`).

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Runtime entrypoint (outer process) | `uvicorn openhands.server.listen:app` started by Makefile; `listen.py` is a deprecated shim re-exporting `app_server.app.app` | `Makefile:262`; `openhands/server/listen.py:6`; `openhands/app_server/app.py:54` |
| FastAPI app + lifespan composition | `FastAPI(title='OpenHands', ...)` mounted with `combine_lifespans`; `OssAppLifespanService.run_alembic_on_startup` | `openhands/app_server/app.py:54-60`; `app.py:48-51`; `openhands/app_server/app_lifespan/oss_app_lifespan_service.py:13-21` |
| V1 router assembly | Single `APIRouter(prefix='/api/v1')` mounting 13 feature routers | `openhands/app_server/v1_router.py:24-37` |
| Conversation-start FSM (explicit) | `AppConversationStartTaskStatus` enum with 9 states | `openhands/app_server/app_conversation/app_conversation_models.py:262-271` |
| Conversation-start async-generator state machine | `_start_app_conversation` yields the task through WAITING_FOR_SANDBOX, STARTING_CONVERSATION, READY | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:309-538` (esp. `342`, `410`, `521`) |
| Sandbox state polling loop | `_wait_for_sandbox_start` uses shared `wait_for_sandbox_running` that polls until RUNNING or timeout | `live_status_app_conversation_service.py:804-872`; `openhands/app_server/sandbox/sandbox_service.py:93-140` (loop at `119-138`) |
| Agent-runtime launcher (Docker) | `DockerSandboxService.start_sandbox` runs the sandbox image with `command=['--port', '8000']` (the agent_server entrypoint) and `WEBHOOK_CALLBACK_VARIABLE` pointing back to the app | `openhands/app_server/sandbox/docker_sandbox_service.py:381-515` (container run at `484-508`); `docker_sandbox_spec_service.py:35-52`; `sandbox_service.py:26` |
| Agent-runtime launcher (process) | `ProcessSandboxService._start_agent_process` runs `python -m openhands.agent_server --port <port>` in a per-sandbox working dir | `openhands/app_server/sandbox/process_sandbox_service.py:110-159`; module default `agent_server_module='openhands.agent_server'` at `441-444` |
| Agent-runtime launcher (remote runtime API) | `RemoteSandboxService` delegates start_sandbox to a runtime API | `openhands/app_server/sandbox/remote_sandbox_service.py:419-471`; ingest poll at `771-839` |
| Streaming SSE-style progress | `POST /api/v1/app-conversations/stream-start` returns a `StreamingResponse` yielding JSON task dumps; non-stream variant hands off via `asyncio.create_task(_consume_remaining(...))` so the request can return early | `openhands/app_server/app_conversation/app_conversation_router.py:981-993`; `_stream_app_conversation_start` at `1633-1651`; background drain at `405` and `_consume_remaining` at `1619-1630` |
| Sandbox lifecycle (Docker) | start → poll-until-RUNNING / pause via `docker.pause` / resume via `unpause` / delete via `container.stop + remove` | `openhands/app_server/sandbox/docker_sandbox_service.py:381-575` |
| Event integration via webhook | `POST /webhooks/conversations` and `POST /webhooks/events/{conversation_id}` | `openhands/app_server/event_callback/webhook_router.py:333-450` (conversations) and `453-522` (events) |
| Event-persist (fanout from webhook to event_store) | `event_service.save_event` called inside the webhook handler | `webhook_router.py:464-466`; save implementation `openhands/app_server/event/event_service_base.py:190-197` |
| Callback fanout (per-event processors) | `execute_callbacks` queries `ACTIVE` callbacks for `(conversation_id, event.kind)` and runs them with `asyncio.gather` | `openhands/app_server/event_callback/sql_event_callback_service.py:208-232` |
| Background polling fallback | `poll_agent_servers(api_url, api_key, sleep_interval)` is an infinite `while True`/`asyncio.sleep` task started once when no public `web_url` is configured | `openhands/app_server/sandbox/remote_sandbox_service.py:682-768` (while True at `699`, sleep at `765`); bootstrap at `921-932` |
| SetTitle callback processor (polling-style retry) | `_poll_for_title` retries `GET /conversations/{id}` up to 4×3 s waiting for the agent_server's title | `openhands/app_server/event_callback/set_title_callback_processor.py:37-79` |
| Pending-message queue (deferred delivery model) | `PendingMessageService` stores messages in `pending_messages` table by `task-{uuid}` then by `conversation_id`; delivered in order to `/api/conversations/{id}/events` with `run=True` | `openhands/app_server/pending_messages/pending_message_service.py:80-185`; delivery loop at `live_status_app_conversation_service.py:1925-2005` |
| AsyncGenerator streaming contract for start | `_stream_app_conversation_start` opens a new `InjectorState` so the dependency context outlives the original request and yields one JSON object per task-update | `app_conversation_router.py:1633-1651` |
| Send-message endpoint is a pure proxy | `POST /api/v1/app-conversations/{id}/send-message` forwards to agent_server `POST /api/conversations/{id}/events` with no extra logic | `app_conversation_router.py:429-590` (forward at `552-590`); explicit "thin proxy" docstring at `454-470` |
| No in-repo loop/runner | Grepping this repo for `AgentController`, `step_loop`, `Runtime`, `if __name__ == "__main__":` of an agent loop returns no matches in `openhands/`; the agent loop is the external `openhands.agent_server` module referenced everywhere | `pyproject.toml:61` pin `openhands-sdk==1.29.0`; usage in `process_sandbox_service.py:441-444`, `docker_sandbox_service.py:486` |

## Answers to Dimension Questions

1. **What is the primary execution model?**
   From inside this repo, the primary model is an **async-generator streaming state machine over HTTP**, composed of three concerns:
   - A start-task FSM that yields incremental `AppConversationStartTask` updates (`live_status_app_conversation_service.py:309-538`).
   - A sandbox lifecycle (`STARTING/RUNNING/PAUSED/ERROR/MISSING` at `sandbox_models.py:9-15`) that is polled-until-ready (`sandbox_service.py:119-138`).
   - An event-store fanout triggered by webhooks from the out-of-process agent_server (`webhook_router.py:453-522`, `sql_event_callback_service.py:208-232`).
   The actual agent think-act loop lives in the external `openhands-sdk` `agent_server` process; we explicitly *do not* see it in this repo.

2. **Is it explicit or emergent?**
   The startup FSM is explicit (`AppConversationStartTaskStatus` enum at `app_conversation_models.py:262-271`), and the stream-SSE contract is explicit (`_stream_app_conversation_start` and `StreamingResponse` at `app_conversation_router.py:981-993`). The integration between "what the agent_server is doing" and "what the app_server sees" is *also* explicit through webhooks *and* an emergent polling loop in dev/OSS mode (`poll_agent_servers` at `remote_sandbox_service.py:682`). The seam between the two (which fields the SDK reads vs. ignores from `StartConversationRequest`) is sometimes emergent rather than type-checked — see the comment at `live_status_app_conversation_service.py:418-430` injecting `user_id` directly into the JSON body because the pinned SDK doesn't expose it yet.

3. **Does the model match the product shape?**
   Yes for the orchestration surface: an HTTP-only app that boots/sandboxes/proxies events is naturally implemented as `async-generator state machine + sandbox polling + webhook fanout`. The product model (multi-user, multi-conversation, multiple sandboxes per user with grouping) drives the explicit `SandboxGroupingStrategy` (`live_status_app_conversation_service.py:674-774`) and the `_find_running_sandbox_for_user` / `_select_sandbox_by_strategy` selection logic, which is the right shape for a hosted SaaS. However the model does not match the *agent* part of the product — the agent loop is hidden in another package, which makes this repo's "model of execution" easy to misread.

4. **Is the model easy to explain to a new contributor?**
   Partially yes, partially no.
   - Easy: the startup FSM (`AppConversationStartTaskStatus`) and the route table (`v1_router.py:24-37`) can be walked in one sitting.
   - Confusing: a contributor reading `live_status_app_conversation_service.py` has to discover that the loop they are looking for lives in another package. There is no top-of-file diagram or docstring that points to the SDK repo. The README at `openhands/app_server/README.md:1-27` does not even mention that the agent loop lives elsewhere.

5. **Does the system mix models cleanly or accidentally?**
   Mostly cleanly at the orchestration boundary (app_server ↔ agent_server over HTTP). Inside app_server:
   - Startup uses an `AsyncGenerator` state machine (explicit).
   - Sandbox readiness uses `asyncio` polling (`sandbox_service.py:119-138`, `live_status_app_conversation_service.py:691-705`).
   - Event ingestion uses webhooks primarily, polling secondarily (`remote_sandbox_service.py:682`).
   - Pending-message replay is a per-conversation sequential loop (`live_status_app_conversation_service.py:1977-1996`).
   These are not accidental — each matches a different concern. The smell is that polling fallback (remote) is module-global state (`global polling_task` at `remote_sandbox_service.py:924`) started lazily on first request, with no supervisor; cancellation only on `CancelledError`.

## Architectural Decisions

- **Split runtime from control plane.** The agent execution loop is intentionally delegated to the `openhands-sdk` `agent_server` process; this repo owns authentication, sandbox lifecycle, callback fanout, and persistence. Pinned as a regular dependency: `pyproject.toml:61` `openhands-sdk==1.29.0`.
- **Per-conversation `ConversationInfo`/`execution_status` is owned by the agent_server, mirrored by the app_server.** The app_server's stored `AppConversationInfo` (`live_status_app_conversation_service.py:479-494`) is rebuilt from webhook payloads (`webhook_router.py:395-413`) so the two views stay loosely consistent.
- **Webhooks first, polling second.** When `web_url` is reachable, agent_server is configured with `WEBHOOK_CALLBACK_VARIABLE` (`docker_sandbox_service.py:418-420`) so callbacks land at `POST /api/v1/webhooks`; otherwise a single global poller is created on first `RemoteSandboxService` inject (`remote_sandbox_service.py:921-932`).
- **Streaming over request/response for startup.** Both `POST /api/v1/app-conversations` (`app_conversation_router.py:363-410`) and `POST /api/v1/app-conversations/stream-start` (`981-993`) hand off the start generator to a background `asyncio.create_task` so the HTTP request can return with a `READY`/`ERROR` task while the sandbox finishes initializing.
- **Sandbox reuse strategy is pluggable.** `SandboxGroupingStrategy` (`live_status_app_conversation_service.py:228-231`) supports `NO_GROUPING`, `ADD_TO_ANY`, `GROUP_BY_NEWEST`, `LEAST_RECENTLY_USED`, `FEWEST_CONVERSATIONS`. Capping at `max_num_conversations_per_sandbox: 20` is configurable per deployment.
- **Filesystem-backed event store by default.** `EventServiceBase._load_event`/`_store_event` are abstract; concrete backends are filesystem (`filesystem_event_service.py`) or AWS/Google Cloud (`aws_event_service.py`, `google_cloud_event_service.py`). Saves are dispatched to `loop.run_in_executor` (`event_service_base.py:191-197`) for I/O concurrency.
- **Heavy use of dependency-injection via `InjectorState`.** Services are selected at runtime based on env-config (`AppServerConfig` at `config.py`, factories at `config.py:436-505`), supporting SaaS vs OSS swap of implementations without code changes.
- **Two-tier auth model.** User requests use JWT/cookie auth; sandbox callbacks use `X-Session-API-Key` headers validated against `SandboxService.get_sandbox_record_by_session_api_key` (`webhook_router.py:235-278`).
- **Per-conversation subscription for "live" status.** `LiveStatusAppConversationService` (`live_status_app_conversation_service.py:200`) merges DB-stored info with a live `GET /api/conversations` to surface `execution_status`, `conversation_url`, and `session_api_key` on the projection (`630-663`).

## Notable Patterns

- **Async-generator-backed streaming**: `_start_app_conversation` is an `AsyncGenerator[AppConversationStartTask, None]` (`live_status_app_conversation_service.py:311`) consumed by both polling patterns (router `anext` at `app_conversation_router.py:381`) and streaming responses (`_stream_app_conversation_start` at `1633-1651`).
- **Webhook-driven event sourcing**: Webhook payloads are validated through Pydantic models imported from the SDK (`ConversationInfo`, `Event`, `ConversationStateUpdateEvent` at `webhook_router.py:16,56-57`), then persisted to a directory tree rooted at `{prefix}/{user_id}/v1_conversations/{conversation_id_hex}/` (`conversation_paths.py:11-36`, `event_service_base.py:66-84`).
- **Pluggable processors via `EventCallbackProcessor` subclass**: `get_event_kind()` class-var routes the event-type → processor-class filtering in SQL (`event_callback_models.py:40-46` + `sql_event_callback_service.py:208-216`). Includes `SetTitleCallbackProcessor` polling the agent_server, and enterprise `GithubV1CallbackProcessor` (`enterprise/integrations/github/github_v1_callback_processor.py:31-60`) posting PR comments on `ConversationStateUpdateEvent` with `execution_status == 'finished'`.
- **Long-running tasks are fire-and-forget `asyncio.create_task` with their own dependency context**: e.g. `_consume_remaining` (`app_conversation_router.py:1619-1630`), `_run_callbacks_in_bg_and_close` (`webhook_router.py:561-573`), `refresh_lock_periodically` (`live_status_app_conversation_service.py:2398`).
- **HTTP-friendly polling**: `while True` is used for *only* two responsibilities in this repo: waiting for a sandbox to reach `RUNNING` (`sandbox_service.py:119-138`; `live_status_app_conversation_service.py:691-705`) and the global poll loop (`remote_sandbox_service.py:699-768`). All other loops are `AsyncGenerator`-driven.
- **SDK adapter patches documented in code**: explicit comments explain cases where the app_server has to work around pinned-SDK limitations (`live_status_app_conversation_service.py:418-430` injecting `user_id` into the JSON body; `874-919` `_verify_agent_server_version` working around SDK drift on custom images).

## Tradeoffs

- **Pro: very clean seam.** App_server has *no* agent code; the SDK can iterate independently. The app_server can be deployed without an agent-server image.
- **Pro: clean streaming UX.** `AppConversationStartTask` is a typed FSM that the client can poll or subscribe to. Polling tasks are persisted (`save_app_conversation_start_task` at `live_status_app_conversation_service.py:304`), so a disconnected client can rejoin via `GET /api/v1/app-conversations/start-tasks/search` (`app_conversation_router.py:996-1035`).
- **Con: dual sources of truth for start-task state.** The SDK's own notion of "conversation started" (its `ConversationInfo`) is partially reconstructed into `AppConversationInfo` via webhook (`webhook_router.py:395-413`). When webhooks miss, the app_server's projection can drift. Polling fallback (`poll_agent_servers`) mitigates this but adds a separate reconciliation path.
- **Con: global long-running tasks without a supervisor.** `RemoteSandboxServiceInjector.inject` stores `polling_task` as a module-level global (`remote_sandbox_service.py:924-932`). There is no graceful shutdown on app lifespan exit — it only stops on `CancelledError`.
- **Con: blocker via HTTP round trips.** Every state transition (`WAITING_FOR_SANDBOX → ... → READY`) crosses a process boundary into the sandbox; `_wait_for_sandbox_start` polls every `poll_frequency` seconds (`live_status_app_conversation_service.py:867-872`), which can be slow in the worst case (`sandbox_startup_timeout: 120` default).
- **Con: streaming endpoint requires keeping DB+httpx alive past the request scope**, hence `_consume_remaining` and the comment about `keep_open` (`app_conversation_router.py:374-376`, `405`). If the consumer disappears, the start task can be left running indefinitely.
- **Con: model is implicit across the repo boundary.** A reader of `live_status_app_conversation_service.py` has to know that "the loop" is not in this file. There is no `arch.md` or top-of-folder diagram pointing to the `openhands-sdk` package that owns the agent loop.

## Failure Modes / Edge Cases

- **Sandbox not reachable / SDK version drift**: `_verify_agent_server_version` (`live_status_app_conversation_service.py:874-919`) and the `custom_agent_server_image()` branch on `response.raise_for_status` failure (`live_status_app_conversation_service.py:448-457`) raise `SandboxError` with an actionable hint to rebuild the image.
- **Sandbox start timeout**: `wait_for_sandbox_running` raises `SandboxError(f'Sandbox failed to start within {timeout}s')` after `sandbox_startup_timeout` (`sandbox_service.py:140`, default 120s in `live_status_app_conversation_service.py:2444-2449`).
- **Sandbox paused / non-running on message**: `POST /send-message` returns 409 with a "use /resume" message (`app_conversation_router.py:521-529`), 503 for ERROR (`514-519`), 410 for `MISSING` (archived, `509-513`).
- **Agent-server crash mid-run**: The app_server has no detection of an agent_server going down mid-conversation other than future webhook/poll failures; the live-status projection will simply stop updating. `pause_old_sandboxes` will eventually pause the container (`sandbox_service.py:203-244`).
- **Pending-message replay**: `_process_pending_messages` deletes every queued message regardless of delivery success (`live_status_app_conversation_service.py:1996-2003`); if delivery fails on the first attempt, the message is silently lost after warning.
- **Polling fallback duplicates webhook writes** if the agent_server is also pushing webhooks (it isn't, by config, but the dual-path design is fragile).
- **Module-global polling task**: Started on first `RemoteSandboxService` use (`remote_sandbox_service.py:924-926`). Process restart required for clean cancel on misconfiguration.
- **Conversation export lock expiry**: `refresh_lock_periodically` runs as a side task during streaming export (`live_status_app_conversation_service.py:2398-2402`), but a slow client could let the Redis lock expire (`export_lock_ttl_seconds: 3600`, refresh at `30s`).

## Future Considerations

- **Document the runtime split prominently.** A top-of-repo or `openhands/app_server/README.md` paragraph pointing at `openhands-sdk.agent_server` would help newcomers understand the loop is external.
- **Replace "global polling task" with a managed lifespan task.** Started in `AppLifespanService.__aenter__` and cancelled in `__aexit__`, so failures during shutdown are observable.
- **Make webhook ingestion durable.** Currently webhooks return 200 after `event_service.save_event` (`webhook_router.py:464-466`); if disk write fails after HTTP ack is sent, the loss is silent. A dead-letter queue or outbox table would help.
- **Type-check the seam explicitly.** The comment at `live_status_app_conversation_service.py:418-430` showing `user_id` being injected into JSON because the pinned SDK doesn't expose the field is a smell that will keep recurring — bump SDK pin or vendor the model.
- **Add startup FSM tests.** `tests/unit/app_server/` has extensive router/service tests but I did not find tests of the full `_start_app_conversation` flow across all states. The huge `live_status_app_conversation_service.py` (2574 lines) would benefit from a state-transition matrix test.
- **Reduce coupling between app_server `AppConversationInfo` and SDK `ConversationInfo`.** Today the webhook handler manually merges tags and recomputes `trigger`/`agent_kind` from a discriminated-union (`webhook_router.py:357-475`). Pushing that to a small mapper class would make this easier to evolve as the SDK adds fields.

## Questions / Gaps

- **Where is the actual agent loop?** Not in this repo. It lives in the `openhands-sdk` package pinned at `pyproject.toml:61` (`openhands-sdk==1.29.0`). The agent_server is launched as `python -m openhands.agent_server --port <port>` (`process_sandbox_service.py:127-131`) or as the Dockerfile `CMD` of `sandbox_spec.id` (`docker_sandbox_service.py:486`).
- **No concrete anchor for the per-turn loop inside this repo.** I searched for `step_loop`, `AgentController`, `class Controller`, `class Runtime`, `step(self)`, `while not finished`, `run_until_finished` patterns; none exist within `openhands/`. The closest analog is the `while True` in `poll_agent_servers` (`remote_sandbox_service.py:699`), and that polls every *conversation*, not every agent turn.
- **Polling vs webhook consistency**: No clear evidence found in this repo for how the app_server detects webhook loss (e.g., a watchdog that flips to polling if no webhooks arrive within N seconds). The polling-only mode is selected statically based on `web_url` presence (`remote_sandbox_service.py:921-924`).
- **Tests of FSM transitions across sandbox-failure paths**: I found `tests/unit/app_server/test_app_conversation_router.py` and webhook-router tests, but no evidence of a "sandbox start times out → READY transitions to ERROR and task is persisted" end-to-end test.

---

Generated by `dimensions/01.01-execution-model-taxonomy.md` against `openhands`.

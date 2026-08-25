# Source Analysis: openhands

## LLM Turn Loop Structure

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | Python (FastAPI backend) + React/TypeScript frontend |
| Analyzed | 2026-07-13 |

## Summary

The selected source `openhands` is the OpenHands V1 application server — a
FastAPI-based orchestrator that manages sandboxes, conversations, events,
secrets, settings, and user context. **The LLM turn loop is not implemented
inside this repository.** The actual agent loop (prompt assembly, model call,
output interpretation, action execution, persistence of turns) lives in the
externally-installed `openhands-sdk` package (which provides
`openhands.sdk.conversation.Conversation` and `openhands.sdk.agent.Agent`),
and is hosted inside a separate runtime process/container launched as an
"agent server" by the `SandboxService`.

The OpenHands V1 server only owns the loop boundary: it routes user messages
to the running sandbox (`POST /api/v1/app-conversations/{id}/send-message` →
`{agent_server_url}/api/conversations/{id}/events`), stores events it
receives back from the agent server, persists conversation metadata in SQL,
and exposes WebSocket/SSE streams of the events. There is no `while not
state.finished`, no `agent.step()`, no `controller.run()` style loop in the
source. The orchestrator therefore should not be rated as if it were the
agent harness; instead, this report documents where the boundary lives, what
the in-source artifacts are, and what the loop itself is — by inspection of
the SDK that is imported.

## Rating

**2 / 10** — as an LLM turn loop, this source is essentially absent. The loop
itself lives in `openhands-sdk` (external). What exists here is the loop
boundary plumbing (send-message proxy, event persistence, callback
processors), but no in-source runner loop, no turn records that the source
itself writes during a turn, no message assembly code, no final-output
condition. A reader looking at this repository alone cannot draw the loop in
five files; they have to follow imports into the SDK and into the
`sandbox.exposed_urls["AGENT_SERVER"]` HTTP endpoint. The rating is low
because the dimension explicitly asks about the loop in this source, and
there is none.

## Evidence Collected

Every entry below is a file in `studies/agent-harness-study/sources/openhands`,
with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Top-level layout — no agent/controller dirs in `openhands/` | `ls openhands/` shows only `analytics/`, `app_server/`, `db/`, `server/`, `version.py` — no `controller/`, `agent/`, `core/`, `runtime/`, `loop/` | `openhands/app_server/` (tree) |
| Legacy V0 server is a deprecated shim re-exporting V1 | `from openhands.app_server.app import app` | `openhands/server/listen.py:6` |
| Legacy V0 server `app.py` is also a shim | Re-exports `app` from `openhands.app_server.app` | `openhands/server/app.py:8-15` |
| Legacy V0 `__main__` only boots uvicorn against the V1 app | `uvicorn.run('openhands.server.listen:app', ...)` | `openhands/server/__main__.py:17` |
| App entrypoint mounts V1 routers only | `app.include_router(v1_router.router)` | `openhands/app_server/app.py:71` |
| V1 router only mounts conversations/events/sandboxes/settings/secret routes — no agent-loop router | APIRouter includes event/conversation/sandbox/etc., no `/runs`, `/turns`, `/steps` | `openhands/app_server/v1_router.py:23-37` |
| No `while ... finished` / `agent.step()` / `conversation.run()` / `controller.step()` in `openhands/openhands` | grep returns no matches | `openhands/**` (search boundary) |
| Loop body lives in external SDK — `Conversation` imported but not defined here | `from openhands.sdk import Agent, AgentContext, LocalWorkspace` and `from openhands.sdk.llm import LLM` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:117-125` |
| `Event` itself comes from SDK | `from openhands.sdk import Event` | `openhands/app_server/event/event_service.py:11` |
| Loop actually runs in `openhands.agent_server` (separate installed package) | `agent_server_module: str = 'openhands.agent_server'` | `openhands/app_server/sandbox/process_sandbox_service.py:442-444` |
| Process-based sandbox spawns `python -m openhands.agent_server` per sandbox | `cmd = [python_executable, '-m', self.agent_server_module, '--port', str(port)]` | `openhands/app_server/sandbox/process_sandbox_service.py:126-132` |
| Docker sandbox also runs the agent_server image | `get_agent_server_image()`, env `AGENT_SERVER` | `openhands/app_server/sandbox/docker_sandbox_service.py:598-666`; `openhands/app_server/sandbox/docker_sandbox_spec_service.py:38-48` |
| The "AGENT_SERVER" exposed URL is how the orchestrator reaches the in-sandbox loop | `for exposed_url in sandbox.exposed_urls: if exposed_url.name == AGENT_SERVER: agent_server_url = exposed_url.url` | `openhands/app_server/sandbox/sandbox_models.py:27`; `openhands/app_server/app_conversation/app_conversation_router.py:196-200` |
| `send-message` endpoint is a thin HTTP proxy to the agent server | `response = await httpx_client.post(f'{agent_server_url}/api/conversations/{conversation_id}/events', json={...})` | `openhands/app_server/app_conversation/app_conversation_router.py:552-568` |
| Explicit "intentionally a thin proxy ... no additional processing logic" comment | docstring at top of `send_message_to_conversation` | `openhands/app_server/app_conversation/app_conversation_router.py:440-470` |
| New conversation is created by POSTing `StartConversationRequest` to the agent server | `await self.httpx_client.post(f'{agent_server_url}/api/conversations', json=body_json, ...)` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:436-441` |
| In-sandbox loop is fenced by SDK version (custom image must match) | `_verify_agent_server_version(agent_server_url, ...)`; `expected_sdk_version()` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:354-356`; `_expected_sdk_version` at `:176-181` |
| Event persistence is one-event-per-file in a per-conversation dir | `path = (await self.get_conversation_path(conversation_id)) / f'{id_hex}.json'` | `openhands/app_server/event/event_service_base.py:190-198` |
| Events are loaded concurrently from disk in an executor pool | `_load_events_from_paths` uses `loop.run_in_executor` + `Semaphore(_event_load_concurrency())` | `openhands/app_server/event/event_service_base.py:56-64` |
| Events are sorted in-memory by ISO `timestamp` string, then offset-paginated | `items.sort(key=lambda e: e.timestamp, ...)` and `start_offset = int(page_id)` | `openhands/app_server/event/event_service_base.py:127-141` |
| Event export iterates sorted events | `items.sort(key=lambda event: event.timestamp)` then `for event in items: yield event` | `openhands/app_server/event/event_service_base.py:154-156` |
| EventRouter has read-only endpoints (`search`, `count`, batch `get`) — no turn-writing endpoint exposed here | `@router.get('/search')`, `@router.get('/count')`, `@router.get('')` | `openhands/app_server/event/event_router.py:29-109` |
| `save_event` is internal-only ("Internal method intended not be part of the REST api") | comment on `save_event` abstract method | `openhands/app_server/event/event_service.py:62` |
| Conversation lifecycle states exposed to UI (no `STEP`, `TURN`) | `AppConversationStartTaskStatus`: `WORKING → WAITING_FOR_SANDBOX → PREPARING_REPOSITORY → RUNNING_SETUP_SCRIPT → SETTING_UP_GIT_HOOKS → SETTING_UP_SKILLS → STARTING_CONVERSATION → READY (or ERROR)` | `openhands/app_server/app_conversation/app_conversation_service.py:103-105` |
| `SandboxStatus` enum used at the loop boundary to gate send-message | `STARTING / RUNNING / PAUSED / ERROR / MISSING` | `openhands/app_server/sandbox/sandbox_models.py:9-15`; gate logic `app_conversation_router.py:508-529` |
| `live_status_app_conversation_service.py` proxies a "live" status from the sandbox and merges it with stored data | `await self._get_live_conversation_info(sandbox, ...)` returns a list of `ConversationInfo` (SDK type) | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:589-599` |
| `Laminar.set_trace_user_id()` is set on the *app-server side* via a manually injected `user_id` field because the pinned SDK lacks it on the request model | comment block at `live_status_app_conversation_service.py:418-430`; `body_json['user_id'] = laminar_user_id` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:418-430` |
| `SwitchProfileRequest` / `SwitchAcpModelRequest` exist to mid-conversation mutate the running LLM | model classes defined; routes `/{conversation_id}/switch_profile` and ACP variant exist | `openhands/app_server/app_conversation/app_conversation_models.py:373-383`; `app_conversation_router.py:619-641, 797` |
| Event callbacks (e.g. auto-title) are wired post-start — not part of the loop itself | `processors.append(SetTitleCallbackProcessor())` after `POST /api/conversations` returns | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:499-518` |
| Pending messages are queued in SQL while a conversation is not yet `READY` and replayed once it is | `PendingMessageService` abstract + `SQLPendingMessageService`; `_process_pending_messages(task_id, conversation_id, agent_server_url, session_api_key)` is called after `READY` | `openhands/app_server/pending_messages/pending_message_service.py:41-185`; caller at `live_status_app_conversation_service.py:525-532` |
| LLM metadata assembled on the app-server side (LiteLLM trace tags) | `get_llm_metadata(model_name, llm_type, conversation_id, user_id)` returns trace_version/tags/session_id/trace_user_id; `should_set_litellm_extra_body` gates `openhands/` prefix | `openhands/app_server/utils/llm_metadata.py:28-73`; gate at `:10-26`; consumed at `live_status_app_conversation_service.py:1340-1365` |
| No in-tree LLM model call (no `litellm.completion`, no `client.chat.completions.create`, no `model.invoke`) | grep against `litellm\|model_completion\|generate_response\|call_llm` returns only metadata helpers | `openhands/**` (search boundary) |
| No turn records persisted by this source during a turn | `save_event` writes events *the agent server emits*, not turn records the orchestrator constructs | `openhands/app_server/event/event_service_base.py:190-198` |
| No message assembly in this source — the SDK builds the messages | `live_status_app_conversation_service.py` only constructs `StartConversationRequest` and a `SendMessageRequest` body of `{role, content, run}` | `live_status_app_conversation_service.py:391-441`; `app_conversation_router.py:553-568` |

## Answers to Dimension Questions

1. **What happens during one turn?**
   In this source, almost nothing happens. There is no "one turn" concept
   here. The closest artifact is the `POST /api/v1/app-conversations/{id}/send-message`
   handler
   (`openhands/app_server/app_conversation/app_conversation_router.py:440-590`):
   it (a) validates the conversation exists and the sandbox is `RUNNING`,
   (b) reads the agent-server URL out of `sandbox.exposed_urls`
   (`name == "AGENT_SERVER"`), (c) POSTs `{role, content, run}` to the
   agent-server endpoint `{agent_server_url}/api/conversations/{id}/events`.
   The orchestrator then either streams or polls back events via
   `EventService.search_events`
   (`openhands/app_server/event/event_service_base.py:94-143`) for the
   frontend. The actual prompt assembly, model call, tool execution, and
   final-output condition all occur inside the in-sandbox agent server,
   driven by `openhands.sdk.conversation.Conversation.run()` (not in this
   source).

2. **Does every turn call the model?**
   Unknown from this source. This source does not call the model at all —
   there is no `litellm.completion`, no `LLM.generate`, no `client.chat.*`.
   The only LLM-adjacent code here is the metadata builder
   (`openhands/app_server/utils/llm_metadata.py:28-73`) and the model
   discovery service
   (`openhands/app_server/config_api/llm_model_service.py:15-60`). Whether
   a turn inside the SDK calls the model is a question about
   `openhands.sdk`, not this repo.

3. **Can a turn include multiple tool calls?**
   Unknown from this source. This source never sees a "turn" or a "tool
   call" — those are SDK abstractions. The closest in-source artifact is
   `EventCallback`/`EventCallbackProcessor`
   (`openhands/app_server/event_callback/`), which is a *post-hoc*
   transformer of events the agent server already emitted.

4. **Is turn state persisted?**
   Per-turn state is not persisted by this source. What is persisted:
   (a) `AppConversationInfo` rows in SQL
   (`openhands/app_server/app_conversation/sql_app_conversation_info_service.py`),
   (b) per-event JSON files written under
   `{prefix}/{user_id}/v1/conversations/{conversation_id}/{event_id}.json`
   (`openhands/app_server/event/event_service_base.py:83-84, 190-198`),
   (c) `PendingMessage` rows queued while the conversation is not yet
   `READY`
   (`openhands/app_server/pending_messages/pending_message_service.py:27-40`).
   The SDK writes the per-turn state inside the sandbox; the orchestrator
   receives that as a stream of `Event` objects.

5. **Is the loop generic or agent-specific?**
   There is no loop in this source to evaluate. The orchestrator is
   deliberately generic over *agent kind*: it accepts both the default
   `openhands` agent and `acp` agents via a discriminated union on
   `StartConversationRequest.agent`
   (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:463-477`)
   and routes to the same `/api/conversations` endpoint on the agent server
   either way. The "loop" itself is generic *inside the SDK*, not in this
   repo.

## Architectural Decisions

- **Loop extraction out of the OSS repo.** The actual turn loop, prompt
  assembly, model call wrapper, and tool dispatch were moved out of
  `openhands/` into the `openhands-sdk` package
  (`from openhands.sdk import Agent, AgentContext, LocalWorkspace` and
  `from openhands.sdk import Event` at
  `openhands/app_server/app_conversation/live_status_app_conversation_service.py:117-125`
  and `openhands/app_server/event/event_service.py:11`). The legacy V0
  server is now a 15-line shim
  (`openhands/server/app.py:1-15`, `openhands/server/listen.py:1-8`).
- **Loop runs in an isolated sandbox, not in this process.** The
  orchestrator launches the loop as either (a) a subprocess running
  `python -m openhands.agent_server`
  (`openhands/app_server/sandbox/process_sandbox_service.py:126-132, 442-444`)
  or (b) a Docker container running the agent-server image
  (`openhands/app_server/sandbox/docker_sandbox_service.py:598-666`).
  Communication is HTTP, gated by `X-Session-API-Key`
  (`app_conversation_router.py:562-566`).
- **The orchestrator owns only the boundary.** `send_message_to_conversation`
  is documented as "intentionally a thin proxy that forwards messages to
  the agent server without additional processing logic"
  (`app_conversation_router.py:440-470`).
- **Event persistence is a write-once, read-many append-only log.** One
  JSON file per event, sorted by ISO timestamp string in memory
  (`event_service_base.py:127-156`). There is no turn grouping or step
  index — events are flat by event-id and timestamp.
- **Per-conversation lifecycle is hidden behind `SandboxStatus` and
  `AppConversationStartTaskStatus`.** The orchestrator gates
  send-message on `SandboxStatus == RUNNING`
  (`app_conversation_router.py:508-529`) and exposes a coarse-grained
  `READY / ERROR` task status
  (`app_conversation_service.py:103-105`).

## Notable Patterns

- **Proxy-and-persist at the loop boundary.** The orchestrator's job at
  the loop boundary is to (1) own the sandbox, (2) POST messages to the
  agent server, (3) receive events, (4) write them as JSON files,
  (5) expose them via FastAPI. Pattern visible end-to-end in
  `live_status_app_conversation_service.py:436-518` and
  `event_service_base.py:56-198`.
- **One-event-per-file storage.** Avoids write contention and gives the
  exporter
  (`iter_events_for_export`,
  `event_service_base.py:145-156`) a clean stream.
- **Pending-message queue for unready conversations.** If a user sends a
  message before the conversation is `READY`, the message goes into the
  `pending_messages` SQL table
  (`pending_message_service.py:27-40`) and is replayed to the agent server
  after start
  (`live_status_app_conversation_service.py:525-532`).
- **Event callbacks are pluggable processors.** Event types
  (`EventCallback`) and processors (`SetTitleCallbackProcessor`) live
  outside the loop and run against already-persisted events
  (`live_status_app_conversation_service.py:499-518`,
  `openhands/app_server/event_callback/`).
- **Discriminated union over agent kind.** The orchestrator accepts both
  the default agent and ACP-vended agents over the same
  `POST /api/conversations` contract
  (`live_status_app_conversation_service.py:463-477`).
- **Sandbox grouping strategy.** `ADD_TO_ANY`, `GROUP_BY_NEWEST`,
  `LEAST_RECENTLY_USED`, `FEWEST_CONVERSATIONS` allow one sandbox to host
  multiple conversations; this is a sandbox-allocation strategy, not a
  loop strategy
  (`live_status_app_conversation_service.py:721-774`).

## Tradeoffs

- **Loop is opaque to this codebase.** All loop observability must come
  from the agent-server's HTTP API and from the event log. There is no
  in-source hook to add custom turn policies, retries, or per-turn
  instrumentation without forking the SDK.
- **Loose coupling via HTTP.** The orchestrator can evolve independently
  of the loop, but it cannot intervene mid-turn — at most it can switch
  the LLM (`switch_profile`, `switch_acp_model`) between turns.
- **Event log is timestamp-ordered, not turn-ordered.** Without an
  in-source turn grouping, the UI cannot reconstruct "one turn = one
  model call + N tool calls" without consulting the SDK's `Event`
  subtypes.
- **Two storage layers.** Events are persisted by the orchestrator
  (`event_service_base.py`) and also by the agent server in its own
  filesystem (`V1_CONVERSATIONS_DIR` is shared — see
  `event_service_base.py:17`). This is fine but is not deduplicated.
- **Pending messages are only delivered once at start.** They are not
  re-delivered if the agent server is restarted mid-conversation; only
  the SDK-side message queue survives that
  (`pending_message_service.py:81-120`).
- **ProcessSandboxService is process-global.** `_processes` is a module
  dict (`process_sandbox_service.py:64`), so this implementation does not
  scale across multiple workers — the `RemoteSandboxService` is the
  production-grade backend
  (`openhands/app_server/sandbox/remote_sandbox_service.py`).

## Failure Modes / Edge Cases

- **Custom agent-server image mismatch.** `_verify_agent_server_version`
  raises a `SandboxError` if the in-sandbox SDK version differs from the
  pinned one
  (`live_status_app_conversation_service.py:354-358`); the create-500 hint
  at `:443-457` exists precisely because mismatches fail opaquely.
- **Send-message to a non-running sandbox returns 409.** The boundary
  strictly enforces `SandboxStatus == RUNNING`
  (`app_conversation_router.py:508-529`); `PAUSED` requires a resume,
  `STARTING/STOPPING/ERROR/MISSING` are 503/409/410.
- **Pending-message delivery is best-effort.** `_process_pending_messages`
  is called inside the start-task flow
  (`live_status_app_conversation_service.py:525-532`); a crash between
  `READY` and replay drops messages.
- **Sandbox port exhaustion.** `_find_unused_port` walks up to 10,000
  ports before raising `SandboxError('No available ports found')`
  (`process_sandbox_service.py:92-102`). In the process backend this is
  a hard limit; the remote backend does not have it.
- **Agent server unreachable mid-conversation.** `httpx.RequestError` is
  surfaced as 502 Bad Gateway
  (`app_conversation_router.py:579-584`); there is no in-source retry.
- **Event pagination is in-memory offset.** The pagination uses an int
  `page_id` and sorts events on every page request
  (`event_service_base.py:127-141`); a very large conversation is
  expensive to page through.
- **Event timestamp is compared as an ISO string.** See
  `event_service_base.py:121-123`: `event.timestamp < timestamp_gte_str`.
  This depends on the SDK emitting lexically-sortable ISO timestamps and
  will silently misbehave if a future SDK emits a different format.

## Future Considerations

- **Bring loop observability into the orchestrator.** Add a typed "turn
  id" or "run id" projection to events so the UI can group turns without
  SDK-internal knowledge.
- **Persist turn-level summary in `AppConversationInfo`.** Today the
  orchestrator persists conversation-level metadata
  (`app_conversation_models.py`) but no per-turn summary; analytics
  (`analytics/analytics_service.py:225-249`) only sees LLM token counts.
- **Surface the SDK's loop exceptions.** Today a sandbox-side loop crash
  surfaces as `sandbox.status == ERROR` only after the sandbox process
  actually dies; richer signals (e.g. a callback webhook on loop failure)
  would help.
- **Move pending-message replay off the start-task path.** Decouple
  delivery from start so message replay survives agent-server restarts
  without coupling to `READY` transition.

## Questions / Gaps

- **Where exactly does the loop run?** The actual
  `Conversation.run()` / `Agent.step()` lives in `openhands-sdk`
  (external package, not in this source). To answer the dimension
  properly one would need to study `openhands-sdk` directly — see
  `openhands.app_server.app_conversation.live_status_app_conversation_service.py:117-125`
  for the imports that pull the SDK in.
- **Is the loop generic across agent kinds?** Generic inside the SDK
  (`Agent` discriminated union over `default` and `acp`); the
  orchestrator treats both kinds uniformly
  (`live_status_app_conversation_service.py:463-477`).
- **Is turn state persisted across crashes?** Inside the sandbox yes
  (SDK-side event store); in the orchestrator only as already-emitted
  events — there is no in-source snapshot of "where in the loop am I".
- **Can a turn include multiple tool calls?** Determined by the SDK
  (likely yes, based on the agent-server `event_service.py:390` we
  observed in the installed package — but that is outside the selected
  source).
- **What is the final-output condition?** Not visible in this source;
  the SDK decides when to stop a conversation.
- **No tests of the loop itself live in this source.** All
  `tests/unit/app_server/*` tests target routers, services, and proxies;
  none exercise an end-to-end turn. Tests that look "agent-like"
  (e.g. `tests/unit/app_server/test_live_status_app_conversation_service.py`)
  mock the agent-server responses.

---

Generated by `03.01-llm-turn-loop-structure` against `openhands`.

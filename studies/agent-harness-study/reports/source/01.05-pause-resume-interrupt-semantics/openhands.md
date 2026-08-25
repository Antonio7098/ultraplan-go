# Source Analysis: openhands

## 01.05: Pause, Resume, and Interrupt Semantics

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | Python (FastAPI app server + SQLAlchemy), React frontend, in-container agent-server (external `openhands-sdk` package) |
| Analyzed | 2026-07-02 |

## Summary

OpenHands exposes a clear two-tier model for pause/resume. The outer tier is a **sandbox-level** pause/resume that toggles the runtime container (or remote runtime, or local process) between active and frozen states, exposed as `POST /api/v1/sandboxes/{id}/pause` and `POST /api/v1/sandboxes/{id}/resume` (`openhands/app_server/sandbox/sandbox_router.py:84-105`). The inner tier is the **conversation** state inside the agent server, which is observable via webhook callbacks at `/api/v1/webhooks/conversations` that fire on "start, pauses, resumes, or delete" (`openhands/app_server/event_callback/webhook_router.py:339`).

Sandbox pause is a synchronous API call (Docker `container.pause()`, process `psutil.suspend()`, or remote runtime `POST /pause`), but the **persisted state** is the row in `v1_remote_sandbox` (id, owner, spec, session_api_key_hash, created_at) plus the conversation metadata in `conversation_metadata`. The session API key is **invalidated on pause** and a **new one is returned on resume** as a deliberate security control (`remote_sandbox_service.py:501-540`, `session_auth.py:13-87`). The app server does **not** auto-resume on subsequent requests; explicit callers must POST to `/resume` first, otherwise the agent-context helper returns `None` for paused sandboxes (`app_conversation_router.py:170-172`) and conversation endpoints return `409 Conflict` with a hint to resume (`app_conversation_router.py:521-529`, `send_message_to_conversation`).

There is **no explicit human-in-the-loop approval gate endpoint** at the app-server layer. The only relevant knob is the `confirmation_mode` user setting, which is mapped to a `ConfirmationPolicyBase` (`AlwaysConfirm` / `ConfirmRisky` / `NeverConfirm`) and forwarded to the agent server at conversation start (`app_conversation_service_base.py:641-652`, `settings_models.py:109`). Pause/resume works in concert with that policy but the app server never queues pending action approvals or persists them across pauses.

Resume after process restart is **partially supported**: the `StoredRemoteSandbox` and `StoredConversationMetadata` rows survive the app server restart, but the live runtime may not — the runtime status is queried fresh from the remote runtime API on every `get_sandbox` call (`remote_sandbox_service.py:188-206`), and the dropped `is_paused` column (migrations 011/012) shows that earlier attempts to cache pause state in the DB were abandoned because external terminations left the cache stale. The resume point is **deterministic only at the sandbox boundary**; inside the agent server it relies on the SDK's own session persistence (e.g. ACP's `acp_isolate_data_dir` and `base_state.json`, `live_status_app_conversation_service.py:1876-1877`).

Resume does not create a new execution branch — it continues the same sandbox and the same conversation. The same `StoredConversationMetadata.conversation_id` is reused, and the new session API key is just rotated. Multiple callers (e.g. UI + automation) targeting the same sandbox do not coordinate; there is no resume lock or owner check at the app-server layer, only a rate-limit exemption (`middleware.py:18`) for the resume endpoint itself.

## Rating

**6 / 10 — Present at the sandbox boundary with explicit interfaces, but the resume determinism and approval-gate story are weak, and the runtime/SDK is treated as a black box whose internal pause semantics are not visible to the app server.**

Rationale:

- Explicit pause/resume API surface at the sandbox level with three concrete implementations (remote, Docker, process) and dedicated test coverage (`sandbox_router.py:84-105`, `sandbox_service.py:86-194`, `test_docker_sandbox_service.py:794-889`, `test_remote_sandbox_service.py:518-609`).
- Synchronous pause: `container.pause()`, `process.suspend()`, or remote `POST /pause`. Persisted state: the `v1_remote_sandbox` row and `conversation_metadata` row. The app server never assumes a sandbox is paused from local state — it always asks the runtime (see dropped `is_paused` column, migration `012.py:19-29`).
- Session API key rotation on resume is a strong, deliberate operational safeguard (`remote_sandbox_service.py:501-540`); session key auth is rejected for non-running sandboxes (`session_auth.py:73-87`), and `validate_session_key` is a centralized check used by both the sandbox router and the user router.
- Webhook callback for conversation lifecycle events is unified and used as the "source of truth" push from the runtime to the app server (`webhook_router.py:333-450`). This is the closest thing to a "resume fires" event.
- The rate-limiter explicitly carves out `POST /sandboxes/{id}/resume` from rate limiting so bursty reopen polling cannot starve the resume path (`middleware.py:18`, `test_middleware.py:159-181`).
- Deductions: (a) no checkpoint serialization at the app-server layer — the agent-server (in-container) is the only thing that knows how to restore event history, and the app server has no visibility into whether a paused agent loop can actually be resumed vs. just relit; (b) no human-in-the-loop approval API; pending messages are queue-based only and require the agent to be in a state that accepts them; (c) `pause_old_sandboxes` can fire concurrently and the LRU is the runtime's `/list` endpoint, not a DB transaction, so a sandbox the app server thinks is paused can be GC'd by the runtime; (d) `app_conversation_router.py:174` collapses STARTING, ERROR, MISSING into 404, masking the resume-able from the non-resume-able cases; (e) the `update_openapi.py` script (line 19-20) only knows about V0 `/api/conversations` paths, so V1 resume endpoints are not surfaced in the public OpenAPI document.

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.py:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Pause/resume HTTP endpoints | `POST /sandboxes/{id}/pause` and `POST /sandboxes/{id}/resume` returning `Success` | `openhands/app_server/sandbox/sandbox_router.py:84-105` |
| Sandbox pause/resume abstract methods | `resume_sandbox(sandbox_id) -> bool` and `pause_sandbox(sandbox_id) -> bool` on the `SandboxService` ABC | `openhands/app_server/sandbox/sandbox_service.py:85-194` |
| Remote pause — invalidates session key | `stored_sandbox.session_api_key_hash = None` before `POST /pause` | `openhands/app_server/sandbox/remote_sandbox_service.py:542-570` |
| Remote resume — issues new session key | Stores the `session_api_key` returned by the runtime and rotates the hash | `openhands/app_server/sandbox/remote_sandbox_service.py:501-540` |
| Status mapping (runtime → app server) | `paused → SandboxStatus.PAUSED`, `stopped → MISSING`, `running → RUNNING` | `openhands/app_server/sandbox/remote_sandbox_service.py:55-61` |
| Docker pause | `container.pause()` if status == `'running'` | `openhands/app_server/sandbox/docker_sandbox_service.py:536-548` |
| Docker resume | `container.unpause()` for `'paused'` or `container.start()` for `'exited'` | `openhands/app_server/sandbox/docker_sandbox_service.py:517-534` |
| Process pause/resume | `psutil.Process.suspend()` / `psutil.Process.resume()` driven by in-memory `_processes` map | `openhands/app_server/sandbox/process_sandbox_service.py:359-385` |
| Per-implementation start_sandbox timeout / wait | `wait_for_sandbox_running` polls up to `timeout` (default 120s) until status `RUNNING` | `openhands/app_server/sandbox/sandbox_service.py:93-140` |
| Persisted sandbox identity | `StoredRemoteSandbox` columns: `id, created_by_user_id, sandbox_spec_id, session_api_key_hash, created_at` | `openhands/app_server/sandbox/remote_sandbox_service.py:73-95` |
| Migration 011: add `is_paused` column | Added for concurrency-limit check, defaulted to `False` for existing rows | `openhands/app_server/app_lifespan/alembic/versions/011.py:19-38` |
| Migration 012: drop `is_paused` column | Dropped because external terminations left the cache stale; runtime `/list` is now source of truth | `openhands/app_server/app_lifespan/alembic/versions/012.py:19-29` |
| Session API key check rejects paused | `validate_session_key` raises 401 if `sandbox_info.status != SandboxStatus.RUNNING` | `openhands/app_server/sandbox/session_auth.py:73-87` |
| Per-sandbox session key rotation on resume | Security note: "When a sandbox is resumed, the runtime-api generates a new session_api_key" | `openhands/app_server/sandbox/remote_sandbox_service.py:501-507` |
| Rate limit exempts resume | `_RESUME_RE` matches `^/api/v1/sandboxes/[^/]+/resume/?$` and bypasses limiter | `openhands/app_server/middleware.py:18, 134-141` |
| Resume rate-limit test | `test_rate_limit_middleware_exempts_sandbox_resume` confirms POST is not 429 | `tests/unit/server/test_middleware.py:159-181` |
| Conversation start task state machine | `WORKING → WAITING_FOR_SANDBOX → … → STARTING_CONVERSATION → READY/ERROR` | `openhands/app_server/app_conversation/app_conversation_models.py:262-271` |
| `pause_old_sandboxes` is a sweep that races pause/resume | Uses runtime `/list` to find running sandboxes; pauses the oldest until under `max_num_sandboxes` | `openhands/app_server/sandbox/remote_sandbox_service.py:598-621` |
| Resume in conversation start path | `_wait_for_sandbox_start` calls `sandbox_service.resume_sandbox(sandbox.id)` if `SandboxStatus.PAUSED` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:850-852` |
| Paused-sandbox guard in agent-context helper | `_get_agent_server_context` returns `None` for paused sandboxes (caller decides 409 vs 200) | `openhands/app_server/app_conversation/app_conversation_router.py:170-172` |
| `send_message` returns 409 on paused | Detail: `Sandbox is {status}. Use POST /api/v1/sandboxes/{id}/resume to resume it first.` | `openhands/app_server/app_conversation/app_conversation_router.py:521-529` |
| `send_message` test for paused case | `test_returns_409_for_paused_sandbox` asserts 409 with `/resume` hint | `tests/unit/app_server/test_send_message_endpoint.py:260-295` |
| ConversationInfo for paused sandbox returns `None` for execution_status | `execution_status` is only set when `sandbox.status == RUNNING` (transitively) | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:639-663` |
| Paused guard for skills, hooks, profile, acp_model, git endpoints | 4 endpoints translate paused into 409 or 200-empty | `openhands/app_server/app_conversation/app_conversation_router.py:716, 818, 1204` |
| `switch_profile` / `switch_acp_model` 409 on paused | OpenAPI response codes say "Sandbox is paused; resume it before switching" | `openhands/app_server/app_conversation/app_conversation_router.py:620-783` |
| Webhook — start/pause/resume/delete callback | `POST /webhooks/conversations` upserts conversation info; ignores DELETING | `openhands/app_server/event_callback/webhook_router.py:333-450` |
| `ConversationStateUpdateEvent` flow | `process_stats_event` updates metrics on every event; analytics hooks fire on terminal status | `openhands/app_server/event_callback/webhook_router.py:453-512` |
| `pause` invalidates in-flight `agent_info` | `_get_agent_server_context` early-returns for paused, so all proxied endpoints short-circuit | `openhands/app_server/app_conversation/app_conversation_router.py:170-178` |
| Conversation start state stored separately | `StoredConversationMetadata` keeps `conversation_id, sandbox_id, parent_conversation_id, last_updated_at, metrics, llm_model, agent_kind, tags, pr_number, title` | `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:65-117` |
| Start task state stored separately | `StoredAppConversationStartTask` keeps the original `request: AppConversationStartRequest` plus `status` | `openhands/app_server/app_conversation/sql_app_conversation_start_task_service.py:54-74` |
| Event persistence | `EventServiceBase` writes events under `{prefix}/{user_id}/v1_conversations/{conversation_id_hex}` | `openhands/app_server/event/event_service_base.py:66-80` |
| Conversation directory naming | `V1_CONVERSATIONS_DIR = 'v1_conversations'` and `get_conversation_dir(uuid) -> 'v1_conversations/{hex}'` | `openhands/app_server/conversation_paths.py:12, 15-35` |
| Sandbox auto-pause on startup | `start_sandbox` and `resume_sandbox` call `pause_old_sandboxes(max_num_sandboxes - 1)` first to keep total ≤ N | `openhands/app_server/sandbox/remote_sandbox_service.py:425, 509` |
| LLM profile re-seed on every start/resume | `_seed_sandbox_profiles` re-runs because profiles live on app-server, not sandbox FS | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:361, 925-999` |
| Confirmation policy selection | `confirmation_mode=True` → `AlwaysConfirm`/`ConfirmRisky`; else `NeverConfirm` | `openhands/app_server/app_conversation/app_conversation_service_base.py:641-652` |
| No app-server-level approval API | No router or service exposes "approve pending action" or "list pending approvals" | No clear evidence found (searched `app_server/**` for `approve|approval|require_user|human`) |
| Pending messages survive start (not pause) | `_process_pending_messages` flushes queued messages once conversation reaches READY, then deletes them | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1925-2006` |
| Resume required for "no-grouping" resume flow | `_wait_for_sandbox_start` only auto-resumes the `sandbox.id` already known to the request; it does not search for paused sandboxes to resume | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:820-865` |
| `delete_sandbox` is what ends the conversation for good | Unlike `pause_sandbox`, `delete_sandbox` removes the `StoredRemoteSandbox` row and posts `POST /stop` to the runtime | `openhands/app_server/sandbox/remote_sandbox_service.py:572-596` |
| Conversation auto-delete on sandbox delete | `_finalize_sandbox_delete` runs only when `count_conversations_by_sandbox_id == 0` | `openhands/app_server/app_conversation/app_conversation_router.py:868-889` |
| Same `conversation_id` is reused on resume | The `conversation_id` is part of the persisted metadata; resume does not mint a new one | `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:65-117` |
| `event_callback` for execution status (start/pause/interrupt/delete) | Comment confirms the agent-server fires the webhook on those transitions | `openhands/app_server/event_callback/webhook_router.py:480` |
| ACP session self-resume note | "Isolate the CLI data dir onto the durable /workspace tree so the SDK self-resumes the provider session (session/load from base_state.json) across pause/resume" | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1875-1877` |

## Answers to Dimension Questions

1. **Can execution pause safely?**
   Yes, at the sandbox boundary. `POST /api/v1/sandboxes/{id}/pause` calls a synchronous pause primitive (Docker `container.pause()`, remote `POST /pause`, or `psutil.suspend()`) and immediately nulls the `session_api_key_hash` so any in-flight requests fail authentication (`remote_sandbox_service.py:553-555`, `session_auth.py:73-87`). What it does **not** do is coordinate with an in-progress agent step — there is no handshake to drain the agent server before pausing, so an agent that is mid-tool-call will be suspended at an OS-level boundary. The runtime may then either (a) accept the pause and resume cleanly, or (b) report `MISSING` on the next `get_sandbox` if it was GC'd in the meantime.

2. **Can it resume after a crash?**
   Partially. The app server's `v1_remote_sandbox` and `conversation_metadata` rows survive a crash, and the runtime's status is re-queried on every read (`remote_sandbox_service.py:188-206`). The runtime can be re-resumed via `POST /sandboxes/{id}/resume` which calls `POST /resume` on the runtime API (`remote_sandbox_service.py:501-540`). Whether the agent-server inside the sandbox can recover its event log and replay state is the SDK's responsibility — the app server does not own that, but the comment at `live_status_app_conversation_service.py:1875-1877` confirms that ACP sessions are expected to self-resume from `base_state.json` on the durable workspace tree. The dropped `is_paused` column (`012.py:19-29`) is explicit evidence that earlier attempts to cache pause state in the DB were abandoned.

3. **Is the resume point deterministic?**
   At the sandbox boundary, yes — the `sandbox_id` is the only input, the runtime status is queried fresh, and the new `session_api_key` is returned synchronously (`remote_sandbox_service.py:501-540`). At the conversation level, the same `conversation_id` is reused, but the resume point inside the agent loop is **not** under the app server's control — it depends on whatever checkpoint / event log the agent server kept alive while paused. If the runtime is GC'd and re-created, the agent server may start fresh.

4. **What happens if the world changed while paused?**
   - **Secrets**: the session API key is invalidated on pause, so a paused sandbox cannot read user secrets until resume (`session_auth.py:13-87`). On resume, a new key is returned and old keys are useless.
   - **Runtime status**: the app server never trusts local state; it always asks the runtime via `_get_sandbox_status_from_runtime` (`remote_sandbox_service.py:188-206`). If the runtime was deleted externally, the next read returns `SandboxStatus.MISSING` and the conversation becomes "archived" (`sandbox_status: MISSING`, `app_conversation_models.py:174-177`).
   - **Pending messages**: they are deleted once the conversation reaches `READY` (not `PAUSED`), so a pause after start does not cause messages to leak (`live_status_app_conversation_service.py:1925-2006`). However, if a message is sent to a paused sandbox via `send_message`, it returns 409 — there is no auto-queue path for paused-sandbox messages.
   - **Multiple users**: there is no ownership check beyond `created_by_user_id` (`sql_app_conversation_info_service.py:144-150`); any user who knows a `sandbox_id` can issue `POST /resume` against their own session.

5. **Can multiple people or systems resume the same run?**
   Yes, but without coordination. There is no resume lock, no version stamping on the sandbox row, and no per-call audit beyond the standard access logs. The first `POST /resume` wins; concurrent resumes are race-free at the DB level because the `session_api_key_hash` update is a single write (`remote_sandbox_service.py:529-535`), but the last-writer for that hash is whoever the runtime hands a new key to. The only coordination is the `pause_old_sandboxes` sweep, which is itself a side effect of `start_sandbox` and `resume_sandbox` (`remote_sandbox_service.py:425, 509`) — it is a heuristic, not a lock.

## Architectural Decisions

- **Sandbox-first abstraction**: pause/resume is a property of the *runtime container*, not the conversation. The app server never asks "is the agent paused?"; it asks "is the sandbox running, paused, or missing?" and rejects requests that need a live agent server if the sandbox is not `RUNNING` (`app_conversation_router.py:170-178`, `170-172`).
- **Session API key rotation as a security boundary**: rather than trusting a `paused` flag, the app server invalidates the key on pause and mints a new one on resume. This is a stronger guarantee than a "you can't use it while paused" check, and it makes leaked keys useless after a single pause/resume cycle (`remote_sandbox_service.py:501-540`, `session_auth.py:73-87`).
- **No DB-cached pause state**: migration `012.py:19-29` is the receipt for this decision. The remote runtime is the source of truth; the DB holds only identity, ownership, and historical conversation metadata.
- **Webhook as the agent-server → app-server event bus**: pause/resume of an *agent run* is signaled via the same `POST /webhooks/conversations` callback as start/delete (`webhook_router.py:339`). The app server does not poll the agent server for status; it only polls the runtime for sandbox status.
- **Resumability is the SDK's problem**: the live status service's note on `acp_isolate_data_dir` (`live_status_app_conversation_service.py:1875-1877`) is the clearest statement that the app server treats the in-sandbox agent server as opaque. It will *trigger* a resume but not *verify* the agent can pick up where it left off.
- **Resume is rate-limit-exempt but otherwise unauthenticated beyond the session key**: the middleware carve-out (`middleware.py:18`) tells us the team views resume as a critical recovery path. There is no idempotency token, however, so duplicate `POST /resume` calls reissue a new session key each time.

## Notable Patterns

- **Polymorphic sandbox service**: `RemoteSandboxService`, `DockerSandboxService`, and `ProcessSandboxService` all conform to the same ABC and implement pause/resume differently, but they share `pause_old_sandboxes` for the concurrency sweep (`sandbox_service.py:203-244`).
- **Centralized session-key auth**: `validate_session_key` is the single point of truth for "is this sandbox's session key still valid?", and it is called from both the sandbox router and the user router (`session_auth.py:37-100`).
- **Proxy pattern for agent-server endpoints**: instead of duplicating the agent-server's API, the app server exposes thin proxies (`send-message`, `switch_profile`, `switch_acp_model`, `git/changes`) that share `_get_agent_server_context` and short-circuit on paused (`app_conversation_router.py:133-216, 440-590, 619-769, 772-866`).
- **Per-sandbox LLM profile re-seeding**: every start/resume re-seeds the user's LLM profiles into the sandbox because SaaS keeps them in the app server, not the sandbox FS (`live_status_app_conversation_service.py:925-999`).
- **Cooperative concurrency control**: `start_sandbox` and `resume_sandbox` both call `pause_old_sandboxes(max_num_sandboxes - 1)` before doing their real work, so the system self-regulates sandbox count without an external scheduler (`remote_sandbox_service.py:425, 509`).

## Tradeoffs

- **Pause is cheap, but the state of the *agent* inside the sandbox is opaque.** The app server treats the agent server as a black box that may or may not survive pause/resume intact. The comment at `live_status_app_conversation_service.py:1875-1877` shows this is a known, accepted tradeoff.
- **Session API key rotation is strong but means every resume re-issues secrets lookup.** Anything that cached the old key (e.g. a long-lived worker) loses its access until it learns the new key. This is mitigated by the webhook callback which fires on resume and re-delivers the new key to the app server, but it is a sharp edge for third-party integrations.
- **The dropped `is_paused` column is a win for correctness but a cost for query speed.** Every sandbox lookup now requires a runtime API call. The team accepted this to avoid the stale-cache problem.
- **No human-in-the-loop approval API at the app-server layer.** The `confirmation_mode` setting is forward to the agent server and enforced there, but the app server does not see or queue approvals. This is fine for the SaaS UI (which talks to the agent server directly) but limits what the SaaS backend can do on the user's behalf.
- **Webhook-as-event-bus is simple but has no ordering or dedup guarantees.** The comment at `webhook_router.py:480` lists "start/pause/interrupt/delete" as the firing set, but the app server has no way to detect missed callbacks. Polling (`poll_agent_servers`, `remote_sandbox_service.py:682-768`) is the recovery path when webhooks are unavailable (e.g. localhost without a public `web_url`).

## Failure Modes / Edge Cases

- **Stale `StoredRemoteSandbox` rows** can outlive the runtime. The pause/resume path uses `pause_old_sandboxes` to reap running sandboxes, but a `StoredRemoteSandbox` whose runtime has been externally deleted stays in the DB and surfaces as `MISSING` until the next conversation start tries to re-attach (`remote_sandbox_service.py:598-621`, `get_sandbox` returns `SandboxStatus.MISSING`).
- **Race between `start_sandbox` and `resume_sandbox`**: both call `pause_old_sandboxes(max_num_sandboxes - 1)` first, so two concurrent operations can each sweep independently. The last-writer for the row's `session_api_key_hash` wins.
- **Paused sandbox + new conversation start**: the conversation start path will auto-resume a paused sandbox if it is the one the user is targeting (`live_status_app_conversation_service.py:850-852`), but it does **not** find a paused sandbox on the user's behalf — the caller has to specify the `sandbox_id` (or be in `ADD_TO_ANY` / similar grouping mode that filters by `RUNNING` only, `live_status_app_conversation_service.py:698`).
- **`send-message` to paused sandbox returns 409 with no auto-queue** (`app_conversation_router.py:521-529`). Messages are only auto-queued for *starting* conversations via `_process_pending_messages` (`live_status_app_conversation_service.py:1925-2006`).
- **Confirmation mode changes mid-flight**: the policy is set at conversation start; a user toggling `confirmation_mode` in settings does not retroactively change the policy of a running conversation. There is no `/conversations/{id}/confirmation_policy` endpoint exposed by the app server.
- **Rate-limit exemption on resume** (`middleware.py:18`) means a malicious caller who knows a `sandbox_id` can force the app server to repeatedly call the remote runtime `/resume` API, potentially DoSing the runtime side. No retry budget is enforced.

## Future Considerations

- **Surfacing resume in OpenAPI**: the `update_openapi.py` script (`scripts/update_openapi.py:19-20`) only knows V0 paths. A user reading the public OpenAPI cannot discover `/api/v1/sandboxes/{id}/pause` or `/resume`.
- **Idempotent resume**: rotate the `session_api_key` even when the runtime reports "already running" so a repeat resume never produces a stale key, but add an `Idempotency-Key` header or equivalent so retries don't churn the runtime.
- **Resume audit / ownership**: log the `user_id` and timestamp of every `/resume` call, and add an OpenAPI-documented ownership check (e.g. only the user that paused may resume without a token).
- **Agent-level pause state**: surface the `execution_status == AWAITING_USER_INPUT` (if/when the SDK supports it) into a top-level `pending_approval` field on `AppConversation` so the UI can show "Approve tomorrow" affordances.
- **Pending approval persistence**: if the agent server is extended with explicit human-in-the-loop approval, add an app-server endpoint that queues the approval and replays it after resume.
- **Notification when resume creates a new session key**: the webhook callback already exists, but the app server has no first-class "session-key-rotated" event the UI can use to refresh state.

## Questions / Gaps

- **What does the agent server do internally on resume?** The app server treats the agent server as opaque. Whether a paused agent run can truly resume at the same step depends on the in-container agent server's checkpointing, which is not visible in this repo. Search boundary: `openhands/app_server/**`. Result: only one comment (`live_status_app_conversation_service.py:1875-1877`) acknowledges the SDK's role.
- **Is there an interrupt API for the agent server?** No app-server endpoint was found that proxies `/interrupt` or similar to the agent server. Search boundary: `openhands/app_server/**`. The only interrupt-like reference is the comment in `webhook_router.py:480` listing the set of events that fire the conversation webhook.
- **Are paused sandboxes auto-cleaned?** `pause_old_sandboxes` only pauses running ones (`remote_sandbox_service.py:598-621`). A paused sandbox can sit forever; only an explicit `delete_sandbox` removes the `StoredRemoteSandbox` row.
- **Do multiple users sharing a sandbox coordinate resume?** The grouping strategy in `_find_running_sandbox_for_user` (`live_status_app_conversation_service.py:674-720`) is per-user, so a user typically has their own sandbox. The shared-sandbox case exists but is undocumented.
- **What does the "ack sandbox resume" event look like to the UI?** The webhook at `webhook_router.py:333-450` fires on conversation-level changes, not on raw sandbox resume. The frontend must poll `GET /sandboxes/{id}` to learn that a `POST /resume` succeeded.
- **Is `confirmation_mode` a real approval gate or just a hint?** It is a hint forwarded to the agent server's confirmation policy. The app server has no API to list, approve, or reject pending actions.

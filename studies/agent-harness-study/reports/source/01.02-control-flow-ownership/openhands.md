# Source Analysis: openhands

## Control-Flow Ownership

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | Python 3.12+ / FastAPI app server (`openhands/app_server/`) that fronts an external `openhands-agent-server` + `openhands-sdk` runtime |
| Analyzed | 2026-07-02 |

## Summary

OpenHands splits control flow across two layers. The in-repo `app_server` is a **control-plane orchestrator**, not an agent loop: it manages sandboxes that host the agent-server, defines the conversation start/finish lifecycle as an explicit enum (`AppConversationStartTaskStatus`, `openhands/app_server/app_conversation/app_conversation_models.py:262-271`), reads the live execution status from the agent-server via `GET /api/conversations` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:608-617`), and pushes user-facing actions (send message, switch LLM, switch ACP model, set security analyzer, fetch hooks) as HTTP requests to the agent-server. The actual "what runs next" decision — the agent's loop, stuck detection, iteration cap, tool dispatch, and security enforcement — lives in the external `openhands-sdk` and `openhands-agent-server` packages (pinned at `openhands-agent-server==1.29.0` and `openhands-sdk==1.29.0`, declared in `pyproject.toml:60-61`). Runtime override authority over the LLM's next step is plumbed through three first-class SDK objects that this repo configures and pushes into the agent-server: `ConfirmationPolicyBase` (`AlwaysConfirm` / `ConfirmRisky` / `NeverConfirm`, selected at `openhands/app_server/app_conversation/app_conversation_service_base.py:641-652`), `SecurityAnalyzerBase` (`LLMSecurityAnalyzer`, instantiated at `app_conversation_service_base.py:614-639` and pushed at `app_conversation_service_base.py:654-702`), and the agent's `max_iterations` / `conversation_settings` (exposed at `openhands/app_server/settings/settings_models.py:109` and forwarded via `StartConversationRequest` at `live_status_app_conversation_service.py:1898-1923`). Termination is observed (not authored) here: a webhook callback receives `execution_status` updates, classifies `ERROR`/`STUCK`/`DELETING` (`openhands/app_server/event_callback/webhook_router.py:141-167`), and short-circuits the `DELETING` transition (`event_callback/webhook_router.py:351-352`). The control model is therefore a thin, explicit, well-tested proxy in front of an opaque runtime; transitions and override knobs are explicit at the boundary, but the loop itself cannot be exercised inside this repo without the SDK.

## Rating

**7 / 10** — Clear, explicit, and uniform control model at the app-server boundary: explicit status enums, typed REST endpoints, explicit security-analyzer push, webhook-driven termination classification, and a strong unit-test surface that mocks the agent-server end-to-end (`tests/unit/app_server/test_live_status_app_conversation_service.py:159-867+`, `tests/unit/app_server/test_app_conversation_router.py:72-877+`). Runtime override authority is real and not symbolic — `LLMSecurityAnalyzer` + `ConfirmRisky` + `max_iterations` actually gate the agent's next step inside the SDK. It does not reach 8–10 because (a) the most interesting parts of control flow (the loop itself, stuck detection, tool dispatch, message routing inside the agent) live outside this repo in `openhands-sdk`/`openhands-agent-server` and are therefore not inspectable here, (b) "transition decisions" inside the agent-server are only observed as opaque `execution_status` enums projected from the SDK (`app_conversation_models.py:24,178-181`), not authored in this repo, and (c) "control flow without an LLM" is testable only for the app-server's HTTP plumbing — the runtime override surface (`ConfirmationPolicyBase`, `SecurityAnalyzerBase`, `ConversationSettings`) is configured by string-to-instance mapping (`app_conversation_service_base.py:614-652`) rather than a richer policy graph.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Two-layer architecture boundary | `pyproject.toml` pins the agent loop into external packages; `app_server/` is described as a FastAPI server that interacts with the SDK | `pyproject.toml:60-61`, `openhands/app_server/README.md:5-7` |
| App-side lifecycle state machine (this repo's authoritative transitions) | `AppConversationStartTaskStatus` enum: `WORKING → WAITING_FOR_SANDBOX → PREPARING_REPOSITORY → RUNNING_SETUP_SCRIPT → SETTING_UP_GIT_HOOKS → SETTING_UP_SKILLS → STARTING_CONVERSATION → READY` (or `ERROR`) | `openhands/app_server/app_conversation/app_conversation_models.py:262-271` |
| App-side lifecycle status documented in abstract service | Same status progression documented in `AppConversationService.start_app_conversation` docstring | `openhands/app_server/app_conversation/app_conversation_service.py:103-105` |
| Runtime status read (next-step observation) | `_get_live_conversation_info` issues `GET /api/conversations?ids=...` to the agent-server, parses `ConversationInfo.execution_status` (an SDK enum re-exported at line 24) | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:589-628` |
| Runtime status projected to REST consumers | `AppConversation.execution_status: ConversationExecutionStatus \| None` is read from `conversation_info.execution_status` and exposed to the frontend | `openhands/app_server/app_conversation/app_conversation_models.py:173-187`, `openhands/app_server/app_conversation/live_status_app_conversation_service.py:639-660` |
| Conversation start delegated to agent-server (start of control) | `_start_app_conversation` issues `POST /api/conversations` with the assembled `StartConversationRequest`; `ConversationInfo` is the response | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:436-459` |
| User message dispatch (LLM-driven next turn) | `send_message_to_conversation` validates sandbox is `RUNNING`, then proxies `POST /api/conversations/{id}/events` with `{"role","content","run"}` | `openhands/app_server/app_conversation/app_conversation_router.py:429-590` |
| Runtime LLM switch (runtime override of agent's next call) | `switch_conversation_profile` fingerprints the resolved LLM into a fresh `usage_id` (caching-collision mitigation documented at 671-699) and POSTs to `/api/conversations/{id}/switch_llm` | `openhands/app_server/app_conversation/app_conversation_router.py:619-769` |
| ACP model switch (separate runtime override path) | `switch_conversation_acp_model` POSTs to `/api/conversations/{id}/switch_acp_model`; surfaces 400/504 directly and 502 for HTTP errors | `openhands/app_server/app_conversation/app_conversation_router.py:772-865` |
| Security analyzer (runtime override of dangerous actions) | `_create_security_analyzer_from_string` maps settings strings → `LLMSecurityAnalyzer` (or None); `_select_confirmation_policy` picks `NeverConfirm` / `ConfirmRisky` / `AlwaysConfirm` based on `confirmation_mode` and analyzer | `openhands/app_server/app_conversation/app_conversation_service_base.py:614-652` |
| Security analyzer pushed to the agent-server | `_set_security_analyzer_from_settings` POSTs to `/api/conversations/{id}/security_analyzer`; failures are logged, not raised | `openhands/app_server/app_conversation/app_conversation_service_base.py:654-702` |
| ConversationSettings plumbing (max_iterations, confirmation_mode, security_analyzer) | Docstring: "Conversation settings (max_iterations, confirmation_mode, security_analyzer) live in `conversation_settings`."; flow into `StartConversationRequest.create_request()` | `openhands/app_server/settings/settings_models.py:109`, `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1898-1923` |
| Sandbox preconditions gate the runtime (rejecting requests) | `send_message_to_conversation` returns 404 / 410 / 409 / 503 based on `SandboxStatus` (RUNNING, MISSING, ERROR, PAUSED/STARTING/STOPPING) | `openhands/app_server/app_conversation/app_conversation_router.py:508-548` |
| Runtime termination evaluator (ERROR/STUCK classification) | `_track_conversation_terminal` classifies `ConversationExecutionStatus.ERROR` and `STUCK` as terminal errors; `_classify_error_type` maps to budget/timeout/cancel/model/runtime | `openhands/app_server/event_callback/webhook_router.py:70-90,111-167` |
| DELETING transition short-circuits the webhook | `on_conversation_update` returns `Success()` immediately when `execution_status == DELETING` | `openhands/app_server/event_callback/webhook_router.py:349-352` |
| Hooks load delegated to agent-server (pre/post tool override surface) | `load_hooks_from_agent_server` POSTs `{"project_dir": ...}` to `/api/hooks` and returns `HookConfig` (raises on connection error but swallows on startup) | `openhands/app_server/app_conversation/hook_loader.py:43-148` |
| Hook surface modelled in this repo | `HookDefinitionResponse`, `HookMatcherResponse`, `HookEventResponse`; `event_type` strings like `'pre_tool_use'`, `'post_tool_use'`, `'stop'` are the runtime override points the SDK supports | `openhands/app_server/app_conversation/app_conversation_models.py:321-347`, `openhands/app_server/app_conversation/hook_loader.py:14` |
| Pending messages replayed through the agent-server | `_process_pending_messages` POSTs each queued message to `/api/conversations/{id}/events` with `run: True` so the runtime resumes after sandbox startup | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1925-2006` |
| Conversation delete = runtime termination | `delete_app_conversation` calls `app_conversation_service.delete_app_conversation`, which calls `_delete_from_agent_server`; webhook treats `DELETING` as terminal | `openhands/app_server/app_conversation/app_conversation_router.py:892-910`, `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2238-2290`, `openhands/app_server/event_callback/webhook_router.py:351-352` |
| AppConversationStartTask persistence (status transitions observed) | Status yields are persisted via `app_conversation_start_task_service.save_app_conversation_start_task` between every async yield so failure recovery can rehydrate | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:300-339` |
| Error path transitions to ERROR | All exceptions in `_start_app_conversation` set `task.status = ERROR`, attach `task.detail` (with secret redaction), and yield the terminal task | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:534-538` |
| Status kinds the app-server reads | Sandbox status from `sandbox.status`; agent status from `conversation_info.execution_status`; both projected into `AppConversation.sandbox_status` / `execution_status` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:638-660`, `openhands/app_server/sandbox/sandbox_models.py` (SandboxStatus enum) |
| Testability of app-server control flow without an LLM | `tests/unit/app_server/test_live_status_app_conversation_service.py` and `test_app_conversation_router.py` mock the agent-server end-to-end; 130+ test methods cover status, security analyzer, confirmation policy, send-message, switch-profile, switch-acp-model | `tests/unit/app_server/test_live_status_app_conversation_service.py:66-867+`, `tests/unit/app_server/test_app_conversation_router.py:72-877+` |
| Specific runtime-override tests | `test_security_analyzer_from_string_*`, `test_select_confirmation_policy_*` exercise the analyzer/confirmation mapping without an LLM | `tests/unit/app_server/test_app_conversation_service_base.py` (referenced from `app_conversation_service_base.py:614-652`) |

## Answers to Dimension Questions

1. **Who decides what happens next?**
   Inside this repo: no one — the app-server is a proxy. The SDK's `Conversation`/`Agent` running inside the external `openhands-agent-server` (pinned at 1.29.0, `pyproject.toml:60-61`) owns the next-step decision. This repo's contribution is (a) the explicit start-finish state machine `AppConversationStartTaskStatus` (`app_conversation_models.py:262-271`) and (b) explicit configuration of the three override surfaces — `ConversationSettings` (max_iterations / confirmation_mode / security_analyzer, `settings_models.py:109`), `ConfirmationPolicyBase` (`AlwaysConfirm` / `ConfirmRisky` / `NeverConfirm`, `app_conversation_service_base.py:641-652`), and `LLMSecurityAnalyzer` (`app_conversation_service_base.py:614-639`). Those are pushed into the agent-server via `POST /api/conversations/{id}/security_analyzer` (`app_conversation_service_base.py:688-693`) and `StartConversationRequest` (`live_status_app_conversation_service.py:436-441`).

2. **Can the LLM bypass runtime control?**
   No, within the scope of this repo's authority: the runtime owns three veto points — confirmation policy (`NeverConfirm` vs. `ConfirmRisky` vs. `AlwaysConfirm`), security analyzer (`LLMSecurityAnalyzer`), and `max_iterations`. The agent-server interprets these on every step. The app-server cannot bypass them either; the only way it can stop a running agent is by deleting the conversation (which causes `execution_status=DELETING` and short-circuits the webhook at `event_callback/webhook_router.py:351-352`). What this repo does not control is any case where the SDK's internal loop ignores its own policies — that contract lives in `openhands-sdk` and is not inspectable here.

3. **Can the runtime reject or rewrite the next action?**
   Yes, via three runtime-mediated channels exposed here: (a) `LLMSecurityAnalyzer` can rate every tool call as risky and trigger confirmation (`app_conversation_service_base.py:631-632`, policy mapped at `app_conversation_service_base.py:641-652`); (b) the sandbox lifecycle itself gates whether a send-message even reaches the agent-server (`MISSING` → 410, `ERROR` → 503, `PAUSED/STARTING/STOPPING` → 409, only `RUNNING` accepts writes, `app_conversation_router.py:508-548`); (c) `delete_app_conversation` raises the runtime's authority over the conversation lifecycle (the webhook treats `DELETING` as terminal, `webhook_router.py:351-352`). The app-server cannot inject tool calls into a running conversation — only messages and configuration changes.

4. **Are transitions explicit or scattered?**
   Explicit at the boundary, opaque beyond it. The app-server's transitions are explicit: `AppConversationStartTaskStatus` enumerates the start lifecycle (`app_conversation_models.py:262-271`); the `AppConversationStartTask` model documents the yield pattern with a docstring example (`app_conversation_service.py:85-116`); the SDK's `ConversationExecutionStatus` is the single enum projected into REST responses (`app_conversation_models.py:178-181`); error and stuck classifications are explicit strings in `_classify_error_type` (`webhook_router.py:70-90`). Inside the agent-server the transition graph (loop → step → tool → observation → next action) is not modeled here — it is the SDK's responsibility.

5. **Is control flow testable without calling an LLM?**
   Yes for the app-server's HTTP plumbing. `tests/unit/app_server/test_live_status_app_conversation_service.py` (130+ test methods) and `tests/unit/app_server/test_app_conversation_router.py` mock the agent-server and exercise status projection, security-analyzer string→instance mapping, confirmation-policy selection, sandbox preconditions, profile-switch fingerprinting, and ACP model switching with no LLM in the loop. The agent-loop itself (next-step inside the SDK) is not directly testable in this repo because it lives in `openhands-sdk` / `openhands-agent-server`.

## Architectural Decisions

- **Two-layer architecture.** This repo is the **control plane** (sandboxes, persistence, REST, webhooks). The agent-server container running inside each sandbox is the **runtime**. The split is pinned by `pyproject.toml:60-61` (`openhands-agent-server==1.29.0`, `openhands-sdk==1.29.0`) and described in `openhands/app_server/README.md:5-7`. The boundary is enforced by HTTP — every action crosses it as an authenticated request to `agent_server_url` with `X-Session-API-Key` (`live_status_app_conversation_service.py:431-441`).

- **Explicit lifecycle enums.** Two enums model the system's control state: `AppConversationStartTaskStatus` for the start phase (`app_conversation_models.py:262-271`) and `SandboxStatus` for the runtime's host state. Agent execution status is projected (not authored) as `ConversationExecutionStatus` re-exported from the SDK (`app_conversation_models.py:24,178-181`).

- **String→instance policy mapping.** Three user-facing settings strings (`security_analyzer`, `confirmation_mode`, `max_iterations`) are converted to typed SDK objects via three small factories in `app_conversation_service_base.py:614-652` and `settings_models.py:109`. This keeps the REST surface simple while preserving SDK-typed enforcement inside the agent-server.

- **Webhook-driven termination.** The agent-server pushes `execution_status` transitions to `POST /webhooks/conversations` (`event_callback/webhook_router.py:333-450`), and the app-server reacts to `DELETING` (short-circuit, `:351-352`), `ERROR`/`STUCK` (analytics classification, `:141-167`), and event streams via `POST /webhooks/events/{conversation_id}` (`:453-505`). The app-server does not poll for termination; it is told.

- **Confirmation is opt-in.** `NeverConfirm` is the default unless `confirmation_mode=True` (`app_conversation_service_base.py:645-647`). When on, the choice between `ConfirmRisky` and `AlwaysConfirm` is driven by whether `security_analyzer == "llm"` (`app_conversation_service_base.py:648-652`). The intent (visible in code comments) is to only prompt the user when the analyzer can classify risk.

- **Sandbox lifecycle gates every user action.** The `send_message` endpoint refuses writes unless the sandbox is `RUNNING` (`app_conversation_router.py:508-548`). Pause/resume is therefore a hard runtime control surface that the app-server can engage indirectly through the sandbox service.

- **Hooks delegated to agent-server.** Hook loading (`hook_loader.py:43-148`) and execution are entirely on the agent-server side. This repo only models the response shape (`HookEventResponse`, `HookMatcherResponse`, `HookDefinitionResponse` at `app_conversation_models.py:321-347`) and passes the configured `HookConfig` to the agent-server. The user-visible hook event types (`'pre_tool_use'`, `'post_tool_use'`, `'stop'`, etc.) are strings — not a typed enum in this repo.

- **ACP and OpenHands agents share the same control-flow path.** The `StartConversationRequest` factory handles both `Agent` and `ACPAgent` arms of the `AgentBase` discriminated union (`live_status_app_conversation_service.py:1880-1923`), and the webhook projects both (`event_callback/webhook_router.py:369-393`). For ACP conversations the runtime override authority is delegated to the ACP subprocess via `POST /api/conversations/{id}/switch_acp_model` (`app_conversation_router.py:824-830`).

## Notable Patterns

- **HTTP boundary as the only API surface.** Every interaction with the runtime is an `httpx_client.post(...)` to a sandbox URL. There are no in-process function calls to the SDK's loop from this repo — only `Agent` / `LLM` *construction* (`live_status_app_conversation_service.py:117-119`) and `StartConversationRequest.create_request(...)` (`:1918`). The runtime is opaque from here.

- **Persisted task yields.** `_start_app_conversation` yields an `AppConversationStartTask` between every async step and persists it (`live_status_app_conversation_service.py:300-339,342-343,410-412,521-523`). This makes the start lifecycle recoverable: a worker that crashes mid-start can be resumed from the last persisted state.

- **Status projection, not authoring.** `AppConversation.execution_status` is read from `conversation_info.execution_status` returned by the agent-server (`live_status_app_conversation_service.py:638-660`). The app-server never mutates execution status — it only mirrors it.

- **Secret-aware modeling.** `ConversationSecretEnricher` (`app_conversation/conversation_secret_enricher.py`) and `redact_url_credentials` (`app_conversation_models.py:51-58`) prevent the app-server from leaking credentials while still allowing them to flow into the agent-server's request body. Sandbox settings API keys are obtained via `LookupSecret` so raw secrets never traverse the SDK client (cross-referenced in `AGENTS.md`).

- **Fingerprinted LLM profile usage_ids.** `switch_conversation_profile` computes a SHA-1 fingerprint of the resolved LLM payload and bakes it into the `usage_id` (`app_conversation_router.py:688-699`) so the agent-server's first-write-wins LLM registry cannot silently dedupe two semantically distinct profiles.

- **Discriminated union for agents.** The repo consistently uses `AgentBase` (OpenHands `Agent` vs. ACP `ACPAgent`) as a `DiscriminatedUnionMixin` rather than parallel code paths: webhook handling (`webhook_router.py:369-393`), request construction (`live_status_app_conversation_service.py:463-477,1891-1923`), and model serialization (`app_conversation_models.py:154-156`).

## Tradeoffs

- **No first-party view of the agent loop.** The most interesting control-flow questions (next-action dispatch, stuck detection, max-iteration enforcement, retry/backoff, parallel tool calls) live in `openhands-sdk`. A reader of this repo sees only the projection (`execution_status` enum, the three override surfaces). This is a deliberate separation of concerns but it limits the depth of analysis possible from inside the source.

- **String-typed hooks.** Hook event types are free-text strings (`HookEventResponse.event_type`, `app_conversation_models.py:340`). The runtime's actual hook vocabulary (`pre_tool_use`, `post_tool_use`, `stop`, ...) lives in the SDK and is not mirrored here. A typo in a hook config will not be caught until the agent-server rejects it.

- **Two-policy confirmation model.** `AlwaysConfirm` vs. `ConfirmRisky` is binary (`app_conversation_service_base.py:641-652`). There is no support for per-tool confirmation overrides — the granularity is "is the analyzer LLM-backed" or not.

- **Sandbox lifecycle is a coarse control knob.** Stopping an agent requires pausing the sandbox (a separate, heavier operation than just refusing the next message). The app-server cannot engage `RunControl`-style cooperative stop; it can only refuse messages at the boundary (`app_conversation_router.py:521-529`).

- **Error classification is best-effort.** `_classify_error_type` is a string-match heuristic (`webhook_router.py:70-90`) — `budget`, `timeout`, `cancel`, `model`, `runtime`, `unknown`. It cannot reason about errors it has never seen and relies on the agent-server populating a `last_error` field on the conversation state (`webhook_router.py:149-151`).

- **SDK version drift risk.** The SDK is pinned (`pyproject.toml:60-61`) but the app-server accommodates drift with feature-flag fallbacks (e.g. the `user_id` workaround at `live_status_app_conversation_service.py:421-430` for SDKs that don't expose `StartConversationRequest.user_id`). Drift is surfaced explicitly to operators (`:448-457`).

- **Test surface is wide but shallow.** `tests/unit/app_server/test_live_status_app_conversation_service.py` and `test_app_conversation_router.py` are large (130+ and 30+ test methods) but every test mocks the agent-server — none exercise an actual loop. Confidence in the boundary is high; confidence in the runtime's behavior under the configured policies is not produced by this repo.

## Failure Modes / Edge Cases

- **SDK version mismatch on custom sandbox images.** A custom agent-server image with a stale `openhands-sdk` may 500 on conversation create. The startup path detects this, looks up the expected pinned version, and raises a `SandboxError` with an actionable rebuild hint (`live_status_app_conversation_service.py:443-458`).

- **Sandbox not RUNNING.** Any `send-message` against a paused/starting/stopping/missing/error sandbox returns a typed HTTP error (`app_conversation_router.py:508-548`). The user must `POST /api/v1/sandboxes/{id}/resume` before retrying (`:434-436,524-528`).

- **Security-analyzer push failures are non-fatal.** `_set_security_analyzer_from_settings` logs and swallows exceptions (`:698-702`). A failed analyzer push means the agent starts with no analyzer — a silent safety downgrade. The conversation itself is not blocked.

- **Pending-message replay is per-conversation, sequential.** Pending messages queued before conversation-ready are POSTed one at a time after startup (`live_status_app_conversation_service.py:1977-1995`). A mid-replay failure is logged per-message and processing continues — the queue is deleted regardless of success/failure (`:1997-2002`), so a replay that 500s on the agent-server will lose messages.

- **DELETING during webhook delivery.** `on_conversation_update` short-circuits on `DELETING` (`webhook_router.py:351-352`) but does not clean up the conversation row — it returns `Success()` and leaves the existing record intact. Subsequent deletes rely on the operator's `delete_app_conversation` call.

- **ERROR / STUCK events are observed, not authored.** If the agent-server stops pushing webhooks, the app-server will not detect termination. There is no liveness sweep on running conversations; status is only refreshed when something calls `get_app_conversation` / `search_app_conversations` (`live_status_app_conversation_service.py:282-339`).

- **Switch-profile cache collision mitigation is fingerprint-based.** `switch_conversation_profile` derives `usage_id` from `sha1(fingerprint)[:12]` (`:694-699`). Two profiles that differ only in fields pydantic considers equivalent (e.g. whitespace) will dedupe incorrectly. The docstring documents this trade-off explicitly (`:671-687`).

- **ACP model switch timeouts bubble up.** If the ACP subprocess doesn't respond to `session/set_model` within the agent-server's timeout, the agent-server returns 504 and the app-server surfaces it directly (`app_conversation_router.py:836-848`). A timeout leaves the model-switch pending in the subprocess.

- **`max_iterations` enforcement is not directly observable here.** `ConversationSettings.max_iterations` is forwarded to `StartConversationRequest.create_request()` (`live_status_app_conversation_service.py:1900-1923`), but the cap is enforced inside the SDK. There is no path here to observe "iteration N of M" mid-run.

## Future Considerations

- **First-class stuck/iteration visibility.** Add an app-server endpoint that surfaces iteration count, last-action time, and stuck-reason by reading additional fields from `ConversationStateUpdateEvent` (the same channel `webhook_router.py:149-151` already pulls `last_error` from). Currently the only termination signal is the enum value.

- **Typed hook event vocabulary.** Replace `HookEventResponse.event_type: str` with a Pydantic enum mirrored from the SDK (e.g. `'pre_tool_use' | 'post_tool_use' | 'stop' | ...`). Would catch misconfiguration at startup instead of at the agent-server.

- **Bring confirmation-policy wiring out of strings.** `_create_security_analyzer_from_string` accepts only `"llm"` or `"none"`/`None` (`app_conversation_service_base.py:614-639`); everything else logs a warning and falls back to None. A typed config schema (with validation + migration) would be safer.

- **Health-checked webhooks.** If the agent-server is up but webhooks stop firing, the app-server will silently fall behind. A heartbeat endpoint (`/api/conversations/{id}/status` polled on a schedule) would close that gap without requiring SDK changes.

- **Refactor `_start_app_conversation` into smaller generators.** The current 230-line async generator mixes sandbox waiting, profile setup, plugin loading, secrets, hooks, and conversation creation (`live_status_app_conversation_service.py:309-538`). A future refactor could split it into composable `AsyncGenerator[AppConversationStartTask, None]` steps that are independently testable and recoverable.

- **Move security-analyzer push to a fatal-or-skip policy.** Currently `_set_security_analyzer_from_settings` swallows push failures (`app_conversation_service_base.py:698-702`). If an enterprise operator configures `security_analyzer=llm`, a failed push should arguably fail the conversation start, not silently downgrade safety.

- **Direct tests of webhook → termination mapping.** The error/stuck classification lives at `webhook_router.py:70-90,141-167` but is not enumerated in `tests/unit/app_server/test_webhook_router_*.py`. Coverage exists for auto-title, ACP, parent conversations, stats, auth — but not for `_track_conversation_terminal`'s error type mapping.

## Questions / Gaps

- **Does the SDK expose iteration count or last-action timestamp back to the app-server?** `ConversationStateUpdateEvent` is imported (`webhook_router.py:57`) and the webhook handler reads `key='execution_status'` (`:502-505`) and `key='last_error'` (`:150`). Iteration count is forwarded into the start request (`live_status_app_conversation_service.py:1900-1923`) but no evidence in this repo that the runtime streams back iteration telemetry. No evidence found for iteration observability in the source.

- **What happens when the runtime hits `max_iterations`?** The agent-server stops the loop, but there is no `ConversationExecutionStatus.MAX_ITERATIONS_REACHED` enum value visible in this repo. Termination is reported through the same `ERROR`/`FINISHED` enum path, with `last_error` populated by the SDK. The app-server cannot distinguish "agent gave up" from "model returned nothing" from "max iterations exceeded".

- **Can `ConfirmationPolicyBase` be swapped mid-run?** The agent-server exposes `POST /api/conversations/{id}/security_analyzer` (`app_conversation_service_base.py:688-693`), which strongly suggests yes — but no router endpoint in this repo changes confirmation policy at runtime. Changing it requires re-creating the conversation.

- **Where is the `SendMessageRequest` validated?** The router accepts `AppSendMessageRequest` (`app_conversation_router.py:442`) which wraps an `AgentServer SendMessageRequest` (`app_conversation_models.py:9-14`). Validation of content blocks (`TextContent | ImageContent`, `:361-365`) lives in the SDK; this repo does not add any pre-validation beyond `min_length=1`.

- **Is the hook runtime actually invoked in the agent-server's loop, or only loaded?** `hook_loader.py:43-148` fetches `HookConfig` and the response model `GetHooksResponse` exposes it to the UI (`app_conversation_router.py:1406`). There is no evidence in this repo that hooks are attached to the running agent at runtime — only that they are loaded and viewable. No clear evidence found for hook *invocation* during the agent loop.

- **How does the runtime handle `pre_tool_use` hook veto?** The hook event type string `'pre_tool_use'` is in the response model (`app_conversation_models.py:340`) but the rejection semantics live in the SDK. The app-server does not see veto events — only the agent-server's resulting observations.

- **Does `AlwaysConfirm` block the loop or just pause it?** `_select_confirmation_policy` (`app_conversation_service_base.py:641-652`) maps `confirmation_mode=True` + non-LLM analyzer to `AlwaysConfirm`. The semantics of "always confirm" (i.e. whether the agent loop polls the user synchronously vs. yields a confirmation event) are not defined here.

---

Generated by `dimension-01.02-control-flow-ownership` against `openhands`.
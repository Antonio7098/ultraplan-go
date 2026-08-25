# Source Analysis: openhands

## 03.03 Tool-Calling Roundtrip Control

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (All-Hands-AI/OpenHands) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | Python 3.12, FastAPI application server (`openhands/app_server/`) with React/TypeScript frontend. The runtime agent core lives in three external packages consumed via PyPI: `openhands-sdk==1.29.0`, `openhands-tools==1.29.0`, `openhands-agent-server==1.29.0`. Sandboxes run the agent-server container image `ghcr.io/openhands/agent-server:1.29.0-python`. |
| Analyzed | 2026-07-14 |

## Summary

The OpenHands repository in this study is the **V1 application server (control plane)**, not the agent runtime. The README is explicit about this split: *"The source code for OpenHands Agent and Agent Server lives in OpenHands/software-agent-sdk"* (`README.md:52`). The cross-repo testing skill reinforces it: `software-agent-sdk` is the *"agent core"* (`openhands-sdk`/`openhands-workspace`/`openhands-tools`), while this repo is the *"Cloud backend — FastAPI server (`openhands/app_server/`), sandbox management, auth, enterprise integrations"* (`.agents/skills/cross-repo-testing/SKILL.md:20-21`).

Consequence for Dimension 03.03: **all of the in-loop tool-calling mechanics — extraction, JSON parsing, Pydantic argument validation, executor dispatch, result formatting, repair/retry, error-into-message conversion — live in `software-agent-sdk` and are not present in this source tree.** This repo only participates in the roundtrip at the *edges*:

- **Tool registration and toolset wiring.** `LiveStatusAppConversationService._build_start_conversation_request_for_user` selects the toolset via `get_default_tools(...)` / `get_planning_tools(...)` from the `openhands.tools.preset` namespace (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1617-1625`) and serializes it into `StartConversationRequest.tools` (line 1633).
- **MCP server hosting.** `app_server/mcp/mcp_router.py` mounts a `fastmcp.FastMCP` instance under `/mcp` exposing five integration tools (`create_pr`, `create_mr`, `create_bitbucket_pr`, `create_bitbucket_data_center_pr`, `create_azure_devops_pr`) plus a Tavily proxy under the `tavily` namespace (`mcp_router.py:43`, `:72`, `:147-487`).
- **MCP wiring into the agent.** `_configure_llm_and_mcp` builds an `mcp_config` dict in the SDK-required `{'mcpServers': {name: ...}}` shape, registers the local MCP server with `X-OpenHands-ServerConversation-ID` and `X-Session-API-Key` headers, then merges user-defined MCP servers (`live_status_app_conversation_service.py:1286-1315`).
- **SwitchLLM tool gating.** The app-server attaches the SDK's built-in `SwitchLLMTool` to the agent when ≥2 valid LLM profiles exist (`live_status_app_conversation_service.py:1653-1664`) and seeds the sandbox profile store via `_seed_sandbox_profiles` so the tool can resolve names (`:359-362`, `:925-963`).
- **Confirmation policy selection.** `_select_confirmation_policy` maps the user `confirmation_mode` boolean and `security_analyzer` string to SDK classes `NeverConfirm`, `ConfirmRisky`, `AlwaysConfirm` (`openhands/app_server/app_conversation/app_conversation_service_base.py:641-652`).
- **Secret name validation before tool surface.** `validate_secret_name` and `validate_secrets_dict` enforce blocked prefixes (`LLM_*`), reserved names, and value-size limits on API-provided secrets that may end up as tool arguments (`openhands/app_server/constants.py:97-169`).
- **Inbound observation handling.** `webhook_router.on_event` persists events and reacts to `ObservationEvent` carrying `SwitchLLMObservation` to keep `AppConversationInfo.llm_model` in sync (`openhands/app_server/event_callback/webhook_router.py:453-522`). It does **not** inspect other `ObservationEvent` payload shapes.

Tool-call extraction, argument parsing, retry/repair, error injection back into the loop, and confirmation interrupts all sit downstream of the agent-server. The app-server cannot crash or stall a tool call directly; the worst it can do is fail to start the agent-server, in which case `_wait_for_sandbox_start` raises before any tool roundtrip occurs.

## Rating

**3 / 10** — The repository **does not implement the tool-calling roundtrip**; it only configures and observes it. Where it does implement roundtrip-adjacent code (MCP server hosting, confirmation policy selection, secret name validation, post-hoc observation reconciliation), the work is clearly written and well tested, but it does not exercise the roundtrip itself. There is no tool-call parser, no argument validator (beyond secret-name shape), no executor wrapper, no result formatter, no invalid-tool repair, no in-band tool-error visibility, and no per-call approval gate inside this source tree. A reader looking for those mechanics has to follow the comment trail at `live_status_app_conversation_service.py:422` ("`StartConversationRequest` (added in software-agent-sdk#3242)") into the agent-sdk repo.

The implementation earns points for what it does cover:
- The MCP server uses `FastMCP(..., mask_error_details=True)` (`mcp_router.py:43`) so internal exceptions are not leaked back to the model.
- Errors from the integration services are funneled through `fastmcp.exceptions.ToolError` so the agent sees structured failures (`mcp_router.py:113, 210, 284, 351, 418, 485`).
- `webhook_router._run_callbacks_in_bg_and_close` runs `EventCallbackProcessor`s sequentially in a background task, so callbacks cannot block the webhook response (`webhook_router.py:513-517`, `:561-573`).
- `webhook_router._import_all_tools` eagerly walks `openhands.tools.*` packages at startup so the webhook can deserialize tool-typed events without `ImportError` on first hit (`webhook_router.py:576-586`).

What drags the rating down:
- No code in this repo parses a tool call from model output.
- No code validates that the JSON the model produced for a tool actually matches the tool's argument schema (Pydantic schema lives in `openhands-tools`/`openhands-sdk`).
- No code wraps a tool call in error-handling, timeout, or retry logic — `_select_confirmation_policy` is the only safety interlock, and it operates on the agent as a whole, not on individual calls.
- No code formats a tool result for re-injection into the LLM context.
- No code repairs or auto-retries a malformed tool call; the system relies entirely on the SDK to do so.
- Tool approval is per-agent (`AlwaysConfirm`/`ConfirmRisky`) but not per-call, and there is no in-app-server hook for "user approves this one specific call."
- All evidence about *how* a tool call flows is in comments and config; there is no roundtrip-shaped code path to point at.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Top-level split between app-server and SDK | README redirects to `software-agent-sdk`; `.agents/skills/cross-repo-testing/SKILL.md` documents the two-repo data flow | `README.md:52`; `.agents/skills/cross-repo-testing/SKILL.md:14-21, 24` |
| SDK packages consumed by this repo | `pyproject.toml` pins `openhands-sdk==1.29.0`, `openhands-tools==1.29.0`, `openhands-agent-server==1.29.0` | `pyproject.toml` (dependencies + `[tool.poetry.dependencies]`) |
| Container image carrying the agent runtime | `AGENT_SERVER_IMAGE = 'ghcr.io/openhands/agent-server:1.29.0-python'` | `openhands/app_server/sandbox/sandbox_spec_service.py:16` |
| Toolset selection at conversation start | `get_planning_tools(plan_path=plan_path)` for `AgentType.PLAN`; `get_default_tools(enable_browser=True, enable_sub_agents=user.agent_settings.enable_sub_agents)` for default | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1617-1625` |
| Toolset serialised into the start request | `update={'tools': tools}` applied to `user.agent_settings` before `create_agent()` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1630-1641` |
| Built-in sub-agent registration | `register_builtins_agents(enable_browser=True)` called before `get_default_tools(...)`; `agent_definitions` populated from `get_registered_agent_definitions()` when `enable_sub_agents=True` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1619, 1624-1625` |
| SwitchLLMTool injection | `SwitchLLMTool.__name__` appended to `agent.include_default_tools` when ≥2 valid profiles | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:125, 1648-1664` |
| Sandbox profile seeding for SwitchLLMTool | Upserts user LLM profiles into the agent-server profile store before conversation start so the SDK-side tool can resolve names | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:925-963` (esp. `:930-935`) |
| MCP server wiring into the agent | `_configure_llm_and_mcp` builds `{mcpServers: {default: {...}}}` with `X-OpenHands-ServerConversation-ID` and `X-Session-API-Key` headers, then merges user MCP servers | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1286-1315` |
| MCP server hosting on the app-server | `mcp_server = FastMCP('mcp', mask_error_details=True)`, mounted at `/mcp` with stateless HTTP | `openhands/app_server/mcp/mcp_router.py:43`; `openhands/app_server/app.py:33, 59` |
| MCP tool surface (5 hosted tools + Tavily proxy) | `create_pr`, `create_mr`, `create_bitbucket_pr`, `create_bitbucket_data_center_pr`, `create_azure_devops_pr` registered with `@mcp_server.tool()`; Tavily mounted under `tavily` namespace | `openhands/app_server/mcp/mcp_router.py:147, 215, 289, 356, 423`; `:49-75` |
| Tool-call argument schema (MCP) | Each `@mcp_server.tool` uses `pydantic.Field(description=...)` for argument hints; FastMCP derives the JSON schema | `openhands/app_server/mcp/mcp_router.py:148-163, 216-235, 290-302, 357-369, 424-436` |
| MCP tool errors surfaced to the model | `from fastmcp.exceptions import ToolError`; each tool wraps integration failures and re-raises as `ToolError(str(error))` | `openhands/app_server/mcp/mcp_router.py:8, 113, 210, 284, 351, 418, 485` |
| Confirmation policy selector (per-call approval does not exist; per-agent only) | `_select_confirmation_policy` maps `(confirmation_mode, security_analyzer)` to `NeverConfirm`, `ConfirmRisky`, or `AlwaysConfirm` | `openhands/app_server/app_conversation/app_conversation_service_base.py:641-652` |
| Security analyzer string parsing | `_create_security_analyzer_from_string` accepts `"llm"` or `"none"`, returns `LLMSecurityAnalyzer()` or `None`; unknown values log a warning and default to `None` | `openhands/app_server/app_conversation/app_conversation_service_base.py:614-639` |
| Secret-name validation (the only argument-shape guard in this repo) | `validate_secret_name` blocks reserved names (`OPENVSCODE_SERVER_ROOT`, `OH_*`, `WORKER_*`) and reserved prefix (`LLM_`); allows `OVERRIDABLE_SYSTEM_SECRETS` | `openhands/app_server/constants.py:44-129` |
| Secret-value size limits | `validate_secrets_dict` enforces `MAX_API_SECRETS_COUNT=50`, `MAX_API_SECRET_NAME_LENGTH=256`, `MAX_API_SECRET_VALUE_LENGTH=65536` | `openhands/app_server/constants.py:14-29, 134-169` |
| Webhook receiving tool events from the agent-server | `on_event` accepts `list[Event]`, persists them, and runs registered `EventCallbackProcessor`s in a background task | `openhands/app_server/event_callback/webhook_router.py:453-522, 561-573` |
| ObservationEvent handling (only for LLM switch) | `isinstance(event, ObservationEvent) and isinstance(event.observation, SwitchLLMObservation)` updates `AppConversationInfo.llm_model` | `openhands/app_server/event_callback/webhook_router.py:483-496` |
| Event callback machinery for tool events | `EventCallbackProcessor` is a discriminated-union class with `event_kind: ClassVar[EventKind] = 'MessageEvent'`; SQL store matches callbacks by `(conversation_id, status, event_kind)` | `openhands/app_server/event_callback/event_callback_models.py:40-94`; `openhands/app_server/event_callback/sql_event_callback_service.py:48-67, 208-250` |
| Eager tool import for webhook deserialization | `_import_all_tools()` walks `openhands.tools.*` at startup so pydantic can deserialize tool-typed events without `ImportError` | `openhands/app_server/event_callback/webhook_router.py:15, 576-586` |
| Tests confirming `ActionEvent`/`ObservationEvent` are opaque round-tripped | Tests assert `event_kind='ActionEvent'` and `'ObservationEvent'` rows survive save/search; they do not inspect payloads | `tests/unit/app_server/test_sql_event_callback_service.py:83, 169, 174, 199, 204, 230, 235, 339, 344, 524, 671` |
| Explicit boundary comment on `tools` and `confirmation_mode` | `create_request()` is responsible for `max_iterations`, `confirmation_mode`, and `security_analyzer` propagation; ACP path mirrors regular path | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1900-1902` |
| No parser, validator, executor, repair code exists in this repo | Targeted grep for `tool_call`, `function_call`, `ActionEvent`, `tool_use_id`, `ToolResult`, `execute_command` (workspace only), `ToolExecutor`, `messages.create` returns zero matches for in-loop roundtrip code | `grep -n` across `openhands/` (see "Questions / Gaps" below) |

## Answers to Dimension Questions

1. **How are tool calls represented?**
   Not represented in this repo. The agent-server emits `ActionEvent` instances (`openhands.sdk.event.ActionEvent`); the app-server receives them as opaque `Event` payloads via the `/webhooks/events/{conversation_id}` endpoint and persists them. The closest the app-server gets to "representation" is the MCP tool argument schemas declared via `pydantic.Field(description=...)` on the `@mcp_server.tool()` decorators (`openhands/app_server/mcp/mcp_router.py:148-163` and analogues for the other four tools). Those schemas describe what *the hosted MCP tools* accept, not what the model emits.

2. **What happens if arguments are invalid?**
   - **Hosted MCP tools (`mcp_router.py`):** invalid arguments fail Pydantic validation inside FastMCP *before* the body of the tool runs. Tool bodies additionally wrap downstream exceptions in `fastmcp.exceptions.ToolError` (`mcp_router.py:113, 210, 284, 351, 418, 485`), which FastMCP surfaces to the calling agent-server as a structured failure.
   - **User-provided secrets (the only in-app-server argument-shape validation):** `validate_secret_name` rejects blocked names/prefixes with `ValueError` (`constants.py:107-129`); `validate_secrets_dict` rejects oversized dicts/values (`constants.py:134-169`).
   - **Tool calls emitted by the model:** no parsing, no validation, no repair lives in this repo. Whatever repair exists is inside `openhands-sdk`.

3. **Are tool results structured?**
   Tool results that come back from the agent-server arrive as `ObservationEvent` instances with a typed `.observation` payload (referenced indirectly via `SwitchLLMObservation` at `webhook_router.py:60, 487`). The app-server does not format them; it stores the event as-is. The only formatting logic it performs is *secret redaction* (`redact_text_secrets`) before logging (`webhook_router.py:69, 105`; `tests/unit/utils/test_redact.py:18-126`).

4. **Are tool errors visible to the model?**
   - For the five hosted MCP tools: yes — `ToolError` is raised inside the tool body and `mask_error_details=True` (`mcp_router.py:43`) keeps the model from seeing stack traces while still receiving the error string.
   - For everything else: the model sees whatever the agent-server chooses to expose. The app-server's `_track_conversation_terminal` classifies conversation-level errors into `budget_exceeded`, `model_error`, `runtime_error`, `timeout`, `user_cancelled`, `unknown` for analytics (`webhook_router.py:70-90`), but that classification is **post hoc** — it does not feed back into the in-loop model context.

5. **Can tool calls be approved before execution?**
   Per-agent only, not per-call. The user `confirmation_mode` setting combined with `security_analyzer` chooses `NeverConfirm`, `ConfirmRisky`, or `AlwaysConfirm` (`app_conversation_service_base.py:641-652`). The selector is invoked once at conversation start (referenced indirectly via `create_request()` at `live_status_app_conversation_service.py:1900-1902`) and is consumed by the SDK. The app-server itself exposes no endpoint that approves a specific pending `ActionEvent`. There is no `/api/v1/.../confirm-tool-call` or analogous route.

## Architectural Decisions

- **Hard split between control plane (this repo) and agent runtime (software-agent-sdk).** README and the cross-repo skill codify the boundary (`README.md:52`; `.agents/skills/cross-repo-testing/SKILL.md:14-35`). The app-server's job is to *configure* tools and *observe* events, not to *execute* them. This is the single largest architectural decision affecting 03.03: it removes most of the roundtrip from the surface area of this study.
- **Tools configured by name, not by class.** The `StartConversationRequest` carries a `tools` list populated by helper functions (`get_default_tools(...)` returns SDK-side tool objects that are serialised via Pydantic); the agent-server reconstructs them from the wire payload. This means tool schema changes inside the SDK can affect app-server behaviour without app-server code changes (the converse is also true — tool gating logic in `live_status_app_conversation_service.py:1648-1664` is app-server code that the SDK tool has to honour).
- **MCP over per-call HTTP.** Instead of implementing an in-app-server tool call HTTP endpoint, the app-server mounts FastMCP under `/mcp` (`app.py:33, 59`). The agent-server speaks MCP to it, which is the same protocol the agent-server uses for *user-defined* MCP servers in `mcp_config`. This keeps the tool-call wire format uniform across hosted integration tools, user MCP servers, and Tavily.
- **Secret safety by prefix and name.** `LLM_*` env vars are blocked outright because they are auto-forwarded to the agent-server container and used for enforcement (timeouts, retries, model restrictions) (`constants.py:65-71`). User-supplied secrets cannot escape the app-server's LLM controls by overwriting them.
- **Webhooks are the only observation path.** All `ActionEvent`/`ObservationEvent` traffic flows through `webhook_router.on_event` (`webhook_router.py:453-522`). The app-server therefore has full *capture* of tool events but no programmatic way to influence them once emitted.

## Notable Patterns

- **Stateless FastMCP with masked errors.** `mcp_router.py:43` shows the standard FastMCP pattern for sandboxing hosted tools: `mask_error_details=True` plus `ToolError` for control-flow failures.
- **Wholesale JSON merge for `mcp_config`.** The `mcp_config` field is replaced wholesale rather than deep-merged, preventing stale MCP servers from leaking across settings updates (`tests/unit/app_server/utils/test_jsonpatch_compat.py:42-156`).
- **Hook surface, not tool surface.** Pre/post-tool-use hooks are exposed by the agent-server as `HookConfig` event types (`app_conversation_router.py:1494-1502`); the app-server only lists and surfaces them. The hook execution is entirely within the agent-server.
- **Background callback execution with bounded retries.** `webhook_router._run_callbacks_in_bg_and_close` runs callbacks sequentially in a background task so the webhook returns immediately, and `SetTitleCallbackProcessor` polls the agent-server up to 4 times before giving up (`set_title_callback_processor.py:32-34, 133-138`).
- **SDK version handshake.** `LiveStatusAppConversationService._verify_agent_server_version` reads the pinned `openhands-sdk` version via `importlib.metadata.version` and compares it against the agent-server's reported version, raising a `SandboxError` on mismatch (`live_status_app_conversation_service.py:176-181, 354-356, 449-457, 908-923`). This catches the failure mode where a custom sandbox image ships an SDK newer or older than what the app-server expects — the most likely way a tool roundtrip would silently break.

## Tradeoffs

- **Clean architectural separation vs. lost observability.** Putting the roundtrip in a separate repo keeps this app-server focused, but means the app-server cannot inspect, log, retry, or repair tool calls without first adding API surface to the agent-server. Today, only `SwitchLLMObservation` gets special treatment (`webhook_router.py:483-496`); everything else is opaque.
- **FastMCP hosting vs. in-process tools.** Hosting integration tools in FastMCP gives uniform transport and `mask_error_details=True`, but each tool call is an HTTP round-trip from agent-server back to app-server, with auth headers (`X-OpenHands-ServerConversation-ID`, `X-Session-API-Key`) that the app-server has to thread through every request (`mcp_router.py:167-186, 240-258, 313-329, 386-401, 451-465`).
- **Per-agent confirmation policy vs. per-call approval.** Mapping `confirmation_mode` to a single `ConfirmationPolicyBase` (`app_conversation_service_base.py:641-652`) is simple but coarse — a user cannot say "approve `terminal` automatically but require confirmation for `browser`." The SDK is responsible for any finer-grained rule.
- **Secret redaction at log time vs. at storage time.** The app-server relies on `redact_text_secrets` and `sanitize_config` (imported at `live_status_app_conversation_service.py:126-130`) to scrub values before logging, but stores `SecretSource` objects (e.g. `StaticSecret`, `LookupSecret`) in their original form so the agent-server can fetch them (`live_status_app_conversation_service.py:122, 1585`).
- **`mask_error_details=True` vs. diagnosability.** FastMCP is configured to mask errors, which protects the model from internals but also hides the underlying cause from operators. Logger lines (`mcp_router.py:165, 211, 282, 350, 417, 484`) are the only diagnostic handle.

## Failure Modes / Edge Cases

- **Custom sandbox image with mismatched SDK.** `_verify_agent_server_version` rejects the conversation start with a clear `SandboxError` pointing at the image and expected SDK version (`live_status_app_conversation_service.py:908-923, 449-457`). Without this guard, a tool roundtrip would fail with opaque schema errors.
- **MCP server unreachable at conversation start.** `_configure_llm_and_mcp` silently falls back to no `default` MCP server when `web_url` is absent (`live_status_app_conversation_service.py:830-841`); tests assert that `mcp_config` is empty in this case.
- **User MCP config invalid JSON.** `_merge_custom_mcp_config` logs and skips malformed servers rather than crashing the conversation start; tests confirm this graceful degradation (`tests/unit/app_server/test_live_status_app_conversation_service.py:2106-2128`).
- **Integration tool returns no PR number.** `save_pr_metadata` extracts PR/MR numbers from the tool response with three regexes (`mcp_router.py:115-130`); on miss it logs a warning and does not update `app_conversation_info.pr_number`. The agent still gets the raw response back.
- **Webhook event loss.** `webhook_router.on_event` swallows all exceptions and logs them (`webhook_router.py:519-521`); the agent-server receives `Success()` regardless. There is no retry/ack protocol — events that fail to persist are lost silently.
- **Conversation lookup race.** `valid_conversation` creates a stub `AppConversationInfo` when the conversation is unknown (`webhook_router.py:281-301`), then `on_conversation_update` later fills it in (`webhook_router.py:333-450`). A tool event arriving before the conversation-info webhook can therefore be associated with a stub.
- **SwitchLLMTool race.** `webhook_router` updates `AppConversationInfo.llm_model` only when the current persisted value differs from the agent-reported model (`webhook_router.py:490-496`); simultaneous switches can race but the `info != switched_model` check makes the write idempotent.
- **Secrets dict over API size limits.** `validate_secrets_dict` enforces `MAX_API_SECRETS_COUNT=50` and `MAX_API_SECRET_VALUE_LENGTH=65536` (`constants.py:14-29, 134-169`); abuse via large `secrets` payloads is bounded.
- **Pydantic-shadowed secrets.** `validate_secret_name` blocks `LLM_*` overrides outright (`constants.py:65-71`), preventing users from escaping app-server LLM controls.
- **`mask_error_details=True` swallowing stack traces.** MCP tool failures reach the model as `ToolError` strings only (`mcp_router.py:210, 284, 351, 418, 485`), so a repeated failure may not give the model enough context to self-correct.

## Future Considerations

- **Bring per-tool confirmation into the app-server.** Today `_select_confirmation_policy` returns a single `ConfirmationPolicyBase`. A future revision could expose per-tool overrides via `agent_settings` and route the choice into `create_request()`.
- **Surface richer observation data.** Only `SwitchLLMObservation` is special-cased in `webhook_router.on_event` (`webhook_router.py:483-496`). Other observation types (`FileEditObservation`, `CommandObservation`, etc.) are silently persisted; an `EventCallbackProcessor` per observation kind would let operators add custom dashboards or alerts.
- **Tool-call payload inspection for analytics.** Today the app-server only classifies *conversation-level* errors (`webhook_router.py:70-90`); per-tool-call error classification would require looking inside `ActionEvent` payloads, which the SDK has not yet exposed.
- **Tool-call approval endpoint.** A `POST /api/v1/conversations/{id}/approve-tool-call` route could plug into the SDK's confirmation flow; the current API surface (`v1_router.py`) has no such route.
- **Schema-locked MCP tool arguments.** The hosted MCP tools use only `Field(description=...)` and let FastMCP derive the JSON schema. Adding `Annotated[..., StringConstraints(...)]` / `conint(ge=1, le=...)` etc. would catch more invalid arguments at the schema boundary rather than inside the integration service calls.
- **MCP server load shedding.** The hosted MCP server has no rate limiter; a long-running `create_pr` call from one conversation could starve others. Adding per-conversation concurrency limits would harden the surface.
- **Webhook delivery semantics.** `on_event` returns `Success()` even when persistence fails (`webhook_router.py:519-521`). A retry/ack protocol (e.g. 5xx on persistence failure so the agent-server replays) would close the silent-loss failure mode.

## Questions / Gaps

- **Where in `openhands-sdk` is `ActionEvent` parsed and dispatched to a tool executor?** The repo's only references to `ActionEvent` are in tests (`tests/unit/app_server/test_sql_event_callback_service.py`) and the test data (`tests/unit/app_server/test_event_router.py`). Searching the rest of `openhands/` returns no implementation. The extraction/parser/repair logic must live in `openhands-sdk` (https://github.com/OpenHands/software-agent-sdk) which is not part of this study.
- **How does the SDK represent malformed-tool-call repair?** No code path in this repo repairs a malformed call. The closest hint is the SDK's confirmation flow, which is invoked once per agent run, not per call.
- **What is the schema for `OpenHandsAgentSettings.tools`?** The field is passed through Pydantic via `create_agent()` (`live_status_app_conversation_service.py:1641`), so the schema lives in the SDK. App-server tests mock `get_default_tools` (`tests/unit/app_server/test_live_status_app_conversation_service.py:880, 916, 945, 1012, 1076, 1127, 1164, 1222, 1271`) rather than inspecting it.
- **What happens when the MCP server returns a non-string error?** `ToolError` accepts any stringifiable input (`mcp_router.py:8`); complex exception objects are coerced with `str(error)`. Whether this preserves enough context for the model to self-correct was not measured.
- **Is there a per-tool-call retry budget inside the agent-server?** Nothing in this repo suggests one. The only loop-bounding primitive here is `max_iterations` on `ConversationSettings` (`settings_models.py:109`).
- **Are MCP tool calls authenticated end-to-end?** Headers are threaded (`X-Session-API-Key`, `X-OpenHands-ServerConversation-ID`), and `get_access_token`/`get_user_id` extract per-request identity (`mcp_router.py:171-173, 244-246, 316-318, 389-391, 454-456`), but the integration service calls (`github_service.create_pr(...)`, etc.) are signed with the user's OAuth token, not the session key. A leaked session key would not yield PR write access — only the existing OAuth scopes.

---

Generated by `03.03-tool-calling-roundtrip-control` against `openhands`.
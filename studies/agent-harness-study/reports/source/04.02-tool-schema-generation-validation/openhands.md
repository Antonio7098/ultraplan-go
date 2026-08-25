# Source Analysis: openhands

## Tool Schema Generation and Validation

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | Python 3.12 + FastAPI + FastMCP + Pydantic (v1.x); tool execution delegated to external `openhands-sdk` (1.29.0) and `openhands-tools` (1.29.0) packages declared in `pyproject.toml:14-22` |
| Analyzed | 2026-07-22 |

## Summary

OpenHands separates the **tool-definition layer** from the **server-side request layer**. The repository examined (`openhands/`) does **not** define the actual agent tools (BashTool, FileEditorTool, BrowserTool, GlobTool, etc.); those live in the externally-pinned `openhands-tools` package and are consumed via `get_default_tools(...)` and `get_planning_tools(...)` in `openhands/app_server/app_conversation/live_status_app_conversation_service.py:132-138, 1617-1625`. Locally-defined tools are limited to five MCP-server-exposed wrappers around provider integrations (GitHub PR, GitLab MR, Bitbucket PR, Bitbucket DC PR, Azure DevOps PR) at `openhands/app_server/mcp/mcp_router.py:147-487`.

**Schema source:** Pydantic `Field(description=...)` inside `typing.Annotated` parameter hints, consumed by the `FastMCP` decorator (`openhands/app_server/mcp/mcp_router.py:43, 147-487`). Schema introspection (e.g. `MCPConfig(...)` in `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1634` and `openhands/app_server/settings/settings_models.py:16-17`) is also delegated to FastMCP. A separate API endpoint surfaces a Pydantic-derived settings schema: `GET /api/v1/settings/agent-schema` returns `export_agent_settings_schema().model_dump(mode='json')` from the SDK (`openhands/app_server/settings/settings_router.py:267-270`).

**Validation before execution:** Pydantic automatic type coercion performed by FastMCP at invocation time. Manually-written validators add per-domain safety nets: `validate_secret_name` rejects blocked names/prefixes/over-length names (`openhands/app_server/constants.py:97-129`), `validate_secrets_dict` enforces count and byte-length caps (`openhands/app_server/constants.py:134-160`), and `StrictLLM` overrides `LLM.model_config` to `extra='forbid'` so unknown LLM fields fail loudly (`openhands/app_server/settings/llm_profiles.py:96-105`). For LLM-profile schema drift the validator `@field_validator('profiles', mode='before')` on `LLMProfiles._skip_invalid_profiles` skips bad profiles instead of failing the whole payload (`openhands/app_server/settings/llm_profiles.py:128-148`).

**Descriptions:** Every MCP tool parameter carries a Pydantic `Field(description=...)` annotation (`openhands/app_server/mcp/mcp_router.py:149-162, 217-234, 290-302, 358-369, 425-436`). These are propagated by FastMCP into the JSON Schema exposed to clients. The description docstrings are intentionally prescriptive, e.g. the `labels` description instructs the model to fetch existing labels first and not invent new ones (`openhands/app_server/mcp/mcp_router.py:159-162`).

**Invalid argument behavior:** Two layers. Pydantic validation errors in FastMCP cause a `ToolError` to be returned to the model (`openhands/app_server/mcp/mcp_router.py:113, 210, 284, 351, 418, 485`). Internally-failed provider calls also raise `ToolError` after `logger.error(...)`. FastMCP is constructed with `mask_error_details=True` (`openhands/app_server/mcp/mcp_router.py:43`), so error detail is intentionally hidden from clients. There is no explicit automatic re-prompting schema here — the model is expected to react to the tool error on its own, since the `ToolError` becomes part of the conversation.

**Schema portability across providers:** Schemas are produced in JSON Schema form (the de-facto standard across MCP, OpenAI function-calling and Anthropic tool-use). `MCPConfig` is imported from `fastmcp.mcp_config` (`openhands/app_server/settings/settings_models.py:16-17`) and reused in provider setups (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1628-1634`). `app_mode`-conditional Tavily proxy initialization in the same file (`openhands/app_server/mcp/mcp_router.py:49-76`) shows the system was designed to work across SaaS, OSS, and remote-MCP-backed providers via the same MCP envelope.

## Rating

**5/10**

Rationale:

- **Strengths**
  - **Schema generation is fully type-driven** via `typing.Annotated[T, Field(description=...)]` + FastMCP's decorator-driven introspection (`openhands/app_server/mcp/mcp_router.py:147-163`), meaning descriptions and defaults live next to the implementation. No handwritten JSON Schema in the codebase — eliminates a class of drift bugs.
  - **Validation is defensive at multiple boundaries.** FastMCP/Pydantic validates per-call; `StrictLLM` enforces `extra='forbid'` for LLM config input (`openhands/app_server/settings/llm_profiles.py:96-105`); secret-name validation rejects blocked env vars before they reach the sandbox (`openhands/app_server/constants.py:97-129`); `_skip_invalid_profiles` provides tolerance when stored profiles become invalid after schema drift (`openhands/app_server/settings/llm_profiles.py:128-148`).
  - **Descriptions are unusually prescriptive.** Multiple tool labels (`labels: ... If labels are provided, they must be selected from the repository's existing labels. Do not invent new ones. If the repository's labels are not known, fetch them first.` at `openhands/app_server/mcp/mcp_router.py:159-162, 233-235`) actively steer model behavior, not just declare parameter shape.
  - **Defaults are explicit at the parameter level** (`draft: ... = True`, `labels: ... = None`) and the Pydantic `Field(default=...)` pattern is used consistently throughout `settings_models.py` and `config_models.py:13-55`.
  - **ToolError envelope** is consistent (`openhands/app_server/mcp/mcp_router.py:113, 210, 284, 351, 418, 485`) and `mask_error_details=True` (`openhands/app_server/mcp/mcp_router.py:43`) prevents internal stack-trace leakage to the LLM client.

- **Limitations**
  - **The repository does not contain the agent-facing tool definitions.** All BashTool/FileEditorTool/BrowserTool/etc. schemas live in `openhands-tools` (pinned to 1.29.0 in `pyproject.toml:18`). This study can only inspect the *consumer* side; the *producer* side is out of scope. Therefore, the rating can't be defended on the basis of agent-tool schemas.
  - **No visible schema-level unit tests.** `tests/unit/server/test_openapi_schema_generation.py:1-110` only verifies that `/openapi.json` returns a valid OpenAPI schema; no test asserts the JSON-Schema shape of an MCP tool, its `required` array, the presence of `description` fields, or that an invalid argument call round-trips as a `ToolError`. `tests/unit/mcp/test_mcp_integration.py` only tests user-auth + MCP-config plumbing, not schema correctness (`tests/unit/mcp/test_mcp_integration.py:1-95`).
  - **FastMCP's internal schema-introspection is a black box** in this repo — the framework code is external (`fastmcp` package). There is no local override, no schema-version migration, no explicit handler for "model sent extra/unknown keys".
  - **`ToolError` does not produce a structured correction prompt.** When invalid args arrive, `ToolError` is raised verbatim (`openhands/app_server/mcp/mcp_router.py:210, 284, 351, 418, 485`); the harness does not inject guidance like "the field `repo_name` must be `owner/repo`" beyond what FastMCP/Pydantic already formats. Repair is delegated to the model's next turn.
  - **`mask_error_details=True` is a global setting** (`openhands/app_server/mcp/mcp_router.py:43`). There is no per-tool knob to expose richer diagnostics when needed (e.g. for debugging without redeploying).
  - **No documented fallback for "the model sends a malformed JSON blob entirely".** Both MCP servers and Pydantic tolerate partial objects, but there is no local demonstrative handler.
  - **Secrets validation tests** are excellent (`tests/unit/app_server/test_constants.py:1-163`), but they validate *secrets*, not *tool arguments*. They don't substitute for argument-schema tests.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Schema source | FastMCP decorator on typed functions uses `typing.Annotated[T, Field(description=...)]`. Five PR/MR-creation tools defined locally. | `openhands/app_server/mcp/mcp_router.py:147-163, 215-236, 289-303, 356-370, 423-437` |
| Schema source | FastMCP server constructed; `mask_error_details=True` masks exception bodies from clients. | `openhands/app_server/mcp/mcp_router.py:43` |
| Schema source | `MCPConfig` imported from `fastmcp.mcp_config` for both SDK and app-server use. | `openhands/app_server/settings/settings_models.py:16-17` |
| Schema source | MCP server-side MCPConfig is rebuilt and passed into the agent settings dict. | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1628-1634` |
| Function signature | Local `create_pr` GitHub tool: every parameter annotated with `Field(description=...)`; `draft` defaulted to `True`; `labels` defaulted to `None`. | `openhands/app_server/mcp/mcp_router.py:149-162` |
| Function signature | Local `create_mr` GitLab tool: `id` typed as `int \| str` to accept either ID or URL-encoded path; `description` defaults to `None`; `labels` doc strongly instructs against invented labels. | `openhands/app_server/mcp/mcp_router.py:217-234` |
| Function signature | Local `create_bitbucket_pr`, `create_bitbucket_data_center_pr`, `create_azure_devops_pr` — uniform parameter shape (`repo_name`, `source_branch`, `target_branch`, `title`, `description`). | `openhands/app_server/mcp/mcp_router.py:289-302, 356-369, 423-436` |
| Validator | `validate_secret_name` rejects blocked names, blocked prefixes (`LLM_*`), and over-length names; configurable via env vars. | `openhands/app_server/constants.py:97-129` |
| Validator | `validate_secrets_dict` enforces max count, max byte length; handles both `str` and `SecretStr`. | `openhands/app_server/constants.py:134-160` |
| Validator | `StrictLLM` overrides `extra='ignore'` (base `LLM`) to `extra='forbid'` so typos in API input fail loudly instead of being dropped. | `openhands/app_server/settings/llm_profiles.py:96-105` |
| Validator | `LLMProfiles._skip_invalid_profiles` tolerant-loads stored profiles (schema drift tolerance), warning rather than failing the entire `Settings` load. | `openhands/app_server/settings/llm_profiles.py:128-148` |
| Validator | `_reconcile_active` model validator keeps the active-profile pointer valid. | `openhands/app_server/settings/llm_profiles.py:150-155` |
| Validator | `MCPAgentSettings` discriminated-union merge handled by SDK-side `apply_agent_settings_diff`; explicitly comments that the SDK owns the merge. | `openhands/app_server/settings/settings_models.py:215-217` |
| Description utility | MCP route reuses FastMCP `Annotated[..., Field(description=...)]` for every parameter; descriptions are passed verbatim into the JSON Schema exposed to clients. | `openhands/app_server/mcp/mcp_router.py:149-162, 217-234` |
| Validation strictness | FastMCP `mask_error_details=True` causes validation exceptions to be returned to clients only via a generic `ToolError` envelope. | `openhands/app_server/mcp/mcp_router.py:43` |
| Default handling | `draft: Annotated[bool, Field(description='Whether PR opened is a draft')] = True` — default on the function signature, not via Pydantic `Field(default=...)`. | `openhands/app_server/mcp/mcp_router.py:156` |
| Default handling | `labels: ... = None` for both `create_pr` and `create_mr`. | `openhands/app_server/mcp/mcp_router.py:157-162, 230-235` |
| Error → correction | `ToolError(str(error))` is raised after `logger.error(...)`; no harness-managed re-prompt. | `openhands/app_server/mcp/mcp_router.py:208-210, 282-284, 349-351, 416-418, 484-485` |
| Schema portability | Tavily MCP proxy mounted under `tavily` namespace, server-side regardless of provider. | `openhands/app_server/mcp/mcp_router.py:49-76` |
| Schema portability | `MCPConfig` reused as both server-side config (`Settings`) and SDK-side config (`live_status_app_conversation_service`). | `openhands/app_server/settings/settings_models.py:16-17, 47-49`; `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1628-1634` |
| Settings schema endpoint | `GET /api/v1/settings/agent-schema` returns `export_agent_settings_schema().model_dump(mode='json')` from `openhands.sdk.settings`. | `openhands/app_server/settings/settings_router.py:267-270` |
| Settings schema endpoint | `GET /api/v1/settings/conversation-schema` returns `ConversationSettings.export_schema().model_dump(mode='json')`. | `openhands/app_server/settings/settings_router.py:273-276` |
| Test | Test asserts that the FastAPI app generates a valid `/openapi.json` schema (covers HTTP layer only, not MCP/tool schemas). | `tests/unit/server/test_openapi_schema_generation.py:83-110` |
| Test | Tests for the MCP router cover `get_conversation_link`, `init_tavily_proxy`, and the `stateless_http` deprecation — no schema-shape tests. | `tests/unit/server/routes/test_mcp_routes.py:1-288` |
| Test | Extensive tests for `validate_secret_name` and `validate_secrets_dict`; they exercise length, prefix, blocked-set, `SecretStr`, unicode byte-length, and env-var override. | `tests/unit/app_server/test_constants.py:1-163` |
| Test | MCP integration test imports `RemoteMCPServer` from `fastmcp.mcp_config`, but only checks plumbing with `MCPConfig(...).mcpServers` — not schema validity. | `tests/unit/mcp/test_mcp_integration.py:6-7, 21-46` |

## Answers to Dimension Questions

1. **Are schemas generated or handwritten?**
   Generated. Tool schemas in `mcp_router.py` are produced by FastMCP's `@mcp_server.tool()` decorator reflecting over `typing.Annotated`-typed parameters (`openhands/app_server/mcp/mcp_router.py:147-163`). LLM-profile settings schemas come from Pydantic's `model_dump(mode='json')` (`openhands/app_server/settings/settings_router.py:267-276`). There is no handwritten JSON Schema dict for tools in the local source.

2. **Are descriptions useful?**
   Yes — and they go beyond shape to actively shape model behavior. Example: `If labels are provided, they must be selected from the repository's existing labels. Do not invent new ones. If the repository's labels are not known, fetch them first.` (`openhands/app_server/mcp/mcp_router.py:159-162`); the `id` parameter on `create_mr` is typed `int \| str` so it "accepts either ID or URL-encoded path of the project" (`openhands/app_server/mcp/mcp_router.py:217-219`); the `title` parameter on every provider encourages the model to start the title with `DRAFT:` or `WIP:` when applicable (`openhands/app_server/mcp/mcp_router.py:223-227, 296-300, 363-367, 430-434`). These are explicit guidance, not boilerplate.

3. **Is validation strict?**
   Mixed. **Strict on secrets** — `StrictLLM` rejects unknown fields (`openhands/app_server/settings/llm_profiles.py:96-105`), blocked secret names/prefixes are rejected (`openhands/app_server/constants.py:97-129`), and value byte-length is enforced (`openhands/app_server/constants.py:134-160`). **Loose on stored profile drift** — `_skip_invalid_profiles` silently skips rather than failing (`openhands/app_server/settings/llm_profiles.py:128-148`). **Implicit for tool args** — relies on FastMCP's automatic Pydantic validation; there is no local hook to check e.g. "did the model send `None` for a required arg, and what should we tell it?"

4. **Are defaults handled clearly?**
   Yes for the MCP tools — defaults are baked into the Python signature (`draft=True` at `mcp_router.py:156`, `labels=None` at `mcp_router.py:157-162, 230-235`), which FastMCP propagates into the JSON Schema `default` and removes the parameter from `required`. For the LLM/LLM-profiles settings, defaults come from `Field(default_factory=...)` and `Field(default=None)` on the Pydantic models (`openhands/app_server/settings/settings_models.py:118-146`, `openhands/app_server/settings/llm_profiles.py:123-124`).

5. **Are schemas portable across providers?**
   Yes, via the MCP wire protocol. `MCPConfig` is reused between server-side settings (`openhands/app_server/settings/settings_models.py:16-17`) and the in-conversation SDK instance (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1628-1634`). Tavily's external MCP server is mounted under a `tavily` namespace so its tools appear as `tavily_*` (`openhands/app_server/mcp/mcp_router.py:71-72`). Tools are surfaced through a uniform MCP transport (FastMCP's `StreamableHttpTransport`) so any MCP-speaking client can consume them. **Caveat:** the agent-side mapping from JSON Schema to provider-specific function-calling format (OpenAI Chat Completions, Anthropic, OpenRouter, etc.) lives entirely inside the external `openhands-sdk` package — portability between providers is implemented there, not in this repo.

## Architectural Decisions

- **Server-as-thin-wrapper for tools.** The five local MCP tools are CRUD-style wrappers around existing `XxxServiceImpl` REST clients (`GithubServiceImpl.create_pr`, `GitLabServiceImpl.create_mr`, `BitBucketServiceImpl.create_pr`, `BitbucketDCServiceImpl.create_pr`, `AzureDevOpsServiceImpl.create_pr`) — see `openhands/app_server/mcp/mcp_router.py:195-203, 270-277, 337-343, 404-410, 471-477`. The MCP layer adds the JSON Schema envelope, agent-facing descriptions, and conversation-link enrichment, but no new business logic.
- **Typed parameters as the single source of truth.** Both schema and runtime type checks flow from the same `typing.Annotated[T, Field(description=...)]` declaration (e.g. `mcp_router.py:147-162`). This eliminates the schema-vs-implementation drift class of bug.
- **Strict-on-input, tolerant-on-storage.** API input uses `StrictLLM(extra='forbid')` (`openhands/app_server/settings/llm_profiles.py:96-105`); stored settings tolerate single-profile corruption via `_skip_invalid_profiles` (`openhands/app_server/settings/llm_profiles.py:128-148`). The philosophy: fail loud early, recover soft later.
- **`mask_error_details=True` as a server-wide default.** The MCP server is constructed once with `mask_error_details=True` (`openhands/app_server/mcp/mcp_router.py:43`). Internal exceptions are still logged via `openhands.app_server.utils.logger.openhands_logger` (`openhands/app_server/mcp/mcp_router.py:41`), so observability is preserved server-side.
- **External SDK owns the agent-tool layer.** `get_default_tools(enable_browser=True, enable_sub_agents=...)` and `get_planning_tools(plan_path=...)` are imported from `openhands.tools.preset` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:132-138`). The repo deliberately does not vendor agent tools, trading local control for fast upstream iteration.

## Notable Patterns

- **`Annotated[..., Field(description=...)]` everywhere.** Search the codebase: every tool parameter that should be discoverable to the model uses this idiom (`openhands/app_server/mcp/mcp_router.py:149-162, 217-234, 290-302, 358-369, 425-436`). This is FastMCP's preferred signal for schema generation.
- **ToolError as the universal error envelope.** Every failure path in `mcp_router.py` collapses to `raise ToolError(str(error))` (`mcp_router.py:208-210, 282-284, 349-351, 416-418, 484-485`), after `logger.error(...)`. The pattern keeps the shape of failures predictable for the model.
- **Validation that's specific to the value being validated, not generic.** `validate_secret_name` rejects reserved names that would override container-level config (`openhands/app_server/constants.py:44-61`), and `BLOCKED_SECRET_PREFIXES = ('LLM_',)` blocks any `LLM_*` secret user-set to circumvent app-server LLM controls (`openhands/app_server/constants.py:66-70`). This is "validation as policy enforcement" rather than mere shape-checking.
- **`StrictLLM` vs base `LLM` split.** The base `LLM` model has `extra='ignore'`, leading to silently-dropped fields. `StrictLLM` overrides to `extra='forbid'` for API input (`openhands/app_server/settings/llm_profiles.py:96-105`). The split is a deliberate choice — ignore-on-storage protects against storage evolution, forbid-on-input protects against user-facing typos.
- **`object.__setattr__` bypass when re-entering validators.** When `validate_assignment=True` is on (`openhands/app_server/settings/llm_profiles.py:121`), `_reconcile_active` and `rename()` deliberately use `object.__setattr__` to avoid infinite recursion (`openhands/app_server/settings/llm_profiles.py:153-154, 248-249, 260-261`). This is the canonical Pydantic idiom for self-updating validators.
- **FastMCP `init_tavily_proxy` pattern.** Mount an external MCP server under a namespace, so tools are aggregated but their API keys stay out of the agent's view (`openhands/app_server/mcp/mcp_router.py:49-76`). The pattern could generalize to other upstream MCP servers.

## Tradeoffs

- **Local thinness vs. control.** By not vendoring agent tools, OpenHands gains fast iteration but loses the ability to locally tune schemas to provider quirks. A model like `gpt-4-turbo-2024-04-09` and `claude-3-5-sonnet` need different schema idioms; this lives outside the repo.
- **FastMCP speed-to-prototype vs. introspection depth.** FastMCP's automatic schema generation is fast and almost-correct; the trade-off is that there's no local hook to inspect or rewrite the produced schema. If a tool needs an unusual JSON-Schema construct (e.g. `oneOf`, polymorphic discriminator), the workaround is to switch to hand-rolled MCP servers, which doesn't happen here.
- **`mask_error_details=True` everywhere.** Masking is a safe default for production, but debugging an integration becomes harder. There's no per-tool or per-environment override path in the local code.
- **`_skip_invalid_profiles` is silent.** A user who finds their settings unusable won't know that one specific profile was dropped. The library logs a warning (`openhands/app_server/settings/llm_profiles.py:147`), but this is a deliberate tolerance choice that might not be obvious.
- **Pydantic-centric schema generation.** If a future schema requirement diverges from what Pydantic's `model_dump(mode='json')` produces, the codebase will need either manual JSON-Schema rewrites or a Pydantic swap. Examples: `additionalProperties: false`, `oneOf`, custom `format` keywords.
- **`int | str` overloading.** `id: Annotated[int | str, Field(...)]` for GitLab (`openhands/app_server/mcp/mcp_router.py:217-219`) accepts both numeric IDs and URL-encoded paths. This is forgiving but makes the schema's `type` field `"integer", "string"` — which some providers serialize differently. There's no provider-specific adapter to flatten this.

## Failure Modes / Edge Cases

- **Invalid MCP argument payloads from the model.** FastMCP/Pydantic raise validation errors that are caught and rethrown as `ToolError` (`openhands/app_server/mcp/mcp_router.py:210, 284, 351, 418, 485`), but the inner message is masked by `mask_error_details=True` (`mcp_router.py:43`). Consequence: the model sees a generic error string, not a precise per-field message. Repair is up to the model's next turn.
- **Unknown LLM fields in API input.** `StrictLLM(extra='forbid')` (`openhands/app_server/settings/llm_profiles.py:96-105`) catches these on the `settings` API. The settings endpoint at `settings_router.py` uses `_load_persisted_agent_settings` → `validate_agent_settings` from the SDK (`openhands/app_server/settings/settings_models.py:63-73`); unknown fields are rejected at the SDK layer.
- **Stored LLM profile schema drift.** If an upgraded `LLM` model rejects previously-valid stored fields, `_skip_invalid_profiles` skips them with a warning (`openhands/app_server/settings/llm_profiles.py:128-148`). Users may silently lose saved profiles; the warning is server-side only.
- **Active-profile pointer staleness.** If the user edits `agent_settings.llm` directly, `reconcile_active_profile` clears `llm_profiles.active` (`openhands/app_server/settings/settings_models.py:170-184`); `_reconcile_active` model validator does the same after rename/delete (`openhands/app_server/settings/llm_profiles.py:150-155, 248-249, 260-261`). Mostly robust.
- **Secret-name injection.** `validate_secret_name` rejects reserved env-var names (`openhands/app_server/constants.py:97-129`), so a model cannot override `OPENVSCODE_SERVER_ROOT`, `LLM_*`, etc. via the secrets API. Without it, a model could break the sandbox (`constants.py:36-43` comment block).
- **Tavily proxy init failure.** Caught and logged at `mcp_router.py:74-75`, returns gracefully; users without `TAVILY_API_KEY` simply don't get those tools (`mcp_router.py:58-60`). No silent crash.
- **`create_pr` etc. provider failures** are caught and rethrown as `ToolError` with `logger.error(...)` (`mcp_router.py:208-210`). The model sees an error string but no traceback (masked).
- **MCP server-side extraction of PR number** from the response text uses regex (`mcp_router.py:115-130`). A provider that returns PR numbers in an unexpected format (e.g. emoji-prefixed, custom templates) would be misparsed; the `else` branch logs a warning (`mcp_router.py:138-140`).

## Future Considerations

- **Per-tool error-message control.** Today's global `mask_error_details=True` (`mcp_router.py:43`) is binary. A per-tool opt-out (e.g. via decorator flag) would give debuggability without re-deploying with `mask_error_details=False`.
- **Local schema-version marker.** Schemas are produced on the fly by FastMCP; there's no local record of which schema version a given tool produced, which complicates "did the model see schema X or X+1?" forensics. A simple schema-hash registry could help.
- **Self-describing error messages.** Errors are returned via `ToolError(str(error))` (`mcp_router.py:210`). Tailoring them to be model-instructional (e.g. "the field `repo_name` must be `owner/repo`") rather than opaque provider-error echoes would improve model self-correction.
- **Tests for invalid-argument behavior.** Adding tests to `tests/unit/server/routes/test_mcp_routes.py` that call `create_pr` with intentionally malformed inputs and assert the `ToolError` shape would close a clear gap (currently only the Tavily-proxy and conversation-link paths are tested there — `tests/unit/server/routes/test_mcp_routes.py:1-288`).
- **Schema drift policy.** `_skip_invalid_profiles` is silent on the wire (`openhands/app_server/settings/llm_profiles.py:128-148`). Surfacing dropped profiles through the API response (or an event) would let the UI inform the user.
- **Tool schema for "remote MCP" tools.** Tavily is mounted via `mcp_server.mount(namespace='tavily', ...)` (`mcp_router.py:72`). The aggregated tool list is the union of locally-defined tools and mounted-server tools — there is no local list of "all currently exposed tools" snapshot. A discovery endpoint (e.g. `GET /api/v1/mcp/tools`) would help monitoring.
- **Provider-specific schema flatteners.** `id: int | str` (`mcp_router.py:217-219`) and similar unions will serialize differently across OpenAI/Anthropic/OpenRouter. Adding a normalization step (analogous to the .NET-side `NormalizePortableValues` referenced in `agent-framework.md`) would reduce per-provider support work.
- **Hooks for the tool layer.** The codebase has `HookConfig` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:118, 1670-1685`) but it's only wired into agent-server context, not into MCP tool invocation. A middleware hook on the MCP tool path could enable per-tenant policy (e.g. only allow GitHub under certain users).

## Questions / Gaps

- **Where are the agent tool schemas actually defined?** Tools used at runtime (BashTool, FileEditorTool, BrowserTool, TaskTracker, etc.) live in `openhands-tools==1.29.0`, an external PyPI package pinned in `pyproject.toml:18`. Without that source, this dimension can be assessed only on the *consumption* side. **No evidence found** for agent-tool schemas inside `studies/agent-harness-study/sources/openhands/openhands/`.
- **Is there a per-tool customization layer?** `OPENHANDS_SOURCE` doesn't show a wrapper around `get_default_tools` that injects custom `Field(description=...)` overrides. Searches for `tool_schema`, `SchemaTransformer`, and `FunctionToolSchema` returned **no evidence**. (Patterns: `tests/unit/app_server/test_live_status_app_conversation_service.py:1992` imports `MCPConfig, RemoteMCPServer` but never inspects tool schemas.)
- **How does the model receive the JSON Schema for non-MCP tools?** Likely from the SDK's own OpenAI/Anthropic-format conversion, but **no local code** shows the conversion path. Worth investigating upstream.
- **What happens when a model sends an extra, unknown parameter to a non-MCP tool?** FastMCP's behavior for the local MCP tools is to raise `ToolError`; the SDK-side behavior for the agent tools is invisible from this repo.
- **Are there schema-level unit tests anywhere?** `tests/unit/server/test_openapi_schema_generation.py` tests the HTTP OpenAPI schema, not the MCP JSON Schema. **No evidence found** of a test that asserts the JSON-Schema shape of a single MCP tool (e.g. "create_pr has parameter `repo_name: { type: 'string', description: ... }`").
- **Does the MCP server expose a list-tools endpoint?** FastMCP exposes `tools/list` over MCP protocol by default, but local code does not appear to expose it over HTTP separately. **Gap.** A `/api/v1/mcp/tools` snapshot endpoint would simplify monitoring and auditability.
- **Schema migration policy on agent-settings?** `_load_persisted_agent_settings` calls `validate_agent_settings(data)` which "applies registered schema migrations, canonicalizes the legacy `agent_kind: 'llm'` tag" (`openhands/app_server/settings/settings_models.py:64-73`) — but the actual migrations live in the SDK and are not visible here. **Gap.**

Generated by `04.02-tool-schema-generation-validation.md` against `openhands`.

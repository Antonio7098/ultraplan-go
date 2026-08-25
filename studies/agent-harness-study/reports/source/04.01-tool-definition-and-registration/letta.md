# Source Analysis: letta

## Tool Definition and Registration

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI server, Pydantic schemas, SQLAlchemy ORM, PostgreSQL/SQLite), with optional TypeScript tool sources and MCP servers |
| Analyzed | 2026-07-19 |

## Summary

Letta treats tools as first-class, persisted database rows (`ToolModel` at `letta/orm/tool.py:17`) whose lifecycle is managed by a single `ToolManager` service (`letta/services/tool_manager.py:206`). Built-in tools are declared as plain Python functions in module namespaces (`letta/functions/function_sets/base.py`, `multi_agent.py`, `voice.py`, `builtin.py`, `files.py`) with type-annotated signatures and Google-style docstrings. There are no decorators; schemas are derived statically via `generate_schema` (`letta/functions/schema_generator.py:409`) or via the AST parser `derive_openai_json_schema` (`letta/functions/functions.py:287`). New tools enter the system through one of four paths: (1) HTTP `POST /v1/tools/` (custom tools), (2) `PUT /v1/tools/` upsert, (3) `POST /v1/tools/add-base-tools` / server-startup upsert of the hard-coded `LETTA_TOOL_MODULE_NAMES` list (`letta/constants.py:47-53`), or (4) MCP-server resync (`letta/services/mcp_server_manager.py:284`). Execution is dispatched by `ToolExecutorFactory` which maps `ToolType` enum values to executor classes (`letta/services/tool_executor/tool_execution_manager.py:35-43`). Tools are isolated per organization (unique constraint `(name, organization_id)` at `letta/orm/tool.py:31`) and further scoped by project_id; the `LETTA_TOOL_SET` set of static names guards which functions are surfaced as built-ins (`letta/constants.py:178-187`).

## Rating

**8 / 10** — Clear model with explicit interfaces, transactional concurrency controls, multiple sandbox backends, MCP adapter with health validation, per-org/per-project scoping, and OTel tracing. Falls short of 9 because: bootstrap still requires editing `constants.py` plus `LETTA_TOOL_MODULE_NAMES`, the `function_map` dispatch inside `LettaCoreToolExecutor` (`letta/services/tool_executor/core_tool_executor.py:41-56`) duplicates name knowledge with the schema layer, MCP tools re-connect per call, and tag-update semantics have a documented bug for empty tags (`letta/services/tool_manager.py:334-339`).

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Base schema | `BaseTool` (ID prefix `tool`), `Tool` (id, tool_type, name, tags, source_code, json_schema, args_json_schema, return_char_limit, pip/npm_requirements, default_requires_approval, enable_parallel_execution, metadata_, project_id) | `letta/schemas/tool.py:31-69` |
| Tool categories | `ToolType` enum: `CUSTOM`, `LETTA_CORE`, `LETTA_MEMORY_CORE`, `LETTA_MULTI_AGENT_CORE`, `LETTA_SLEEPTIME_CORE`, `LETTA_VOICE_SLEEPTIME_CORE`, `LETTA_BUILTIN`, `LETTA_FILES_CORE`, `EXTERNAL_MCP` (langchain/composio marked DEPRECATED) | `letta/schemas/enums.py:212-224` |
| Schema refresh on load | `Tool.refresh_source_code_and_json_schema` regenerates `json_schema` from module/function for built-in tools via `get_json_schema_from_module` | `letta/schemas/tool.py:74-107` |
| ToolCreate (custom) | `source_code` required; TypeScript tools REQUIRE explicit `json_schema` (cannot derive) | `letta/schemas/tool.py:110-143` |
| ToolCreate.from_mcp factory | Converts `MCPTool` → `ToolCreate` (wrapper stub + normalized schema + `mcp:<server>` tag) | `letta/schemas/tool.py:145-169` |
| Built-in tool modules | Module path constants for `core`, `multi_agent`, `voice`, `builtin`, `files`; aggregated list `LETTA_TOOL_MODULE_NAMES` | `letta/constants.py:41-53` |
| Built-in tool name lists | `BASE_TOOLS`, `BASE_MEMORY_TOOLS`, `MULTI_AGENT_TOOLS`, `BASE_SLEEPTIME_TOOLS`, `BASE_VOICE_SLEEPTIME_TOOLS`, `BUILTIN_TOOLS`, `FILES_TOOLS` aggregated into `LETTA_TOOL_SET` | `letta/constants.py:112-187` |
| Parallel-safe set | `LETTA_PARALLEL_SAFE_TOOLS` referenced by `Tool.enable_parallel_execution` at upsert | `letta/constants.py:189-197`, `letta/services/tool_manager.py:1185-1193` |
| Reserved kwargs | `TOOL_RESERVED_KWARGS = ["self", "agent_state"]` excluded from generated schema | `letta/constants.py:538`, `letta/functions/schema_generator.py:453` |
| Max name length | `MAX_TOOL_NAME_LENGTH = 48` (Modal function-name budget) | `letta/constants.py:69-72`, enforced at `letta/services/tool_manager.py:232-235` |
| Base tool loader | `load_function_set(module)` walks `dir(module)` skipping `_`-prefixed and any duplicate function names | `letta/functions/functions.py:390-417` |
| Schema introspection | `get_json_schema_from_module` (used to refresh built-in schemas) and `derive_openai_json_schema` (AST parsing of `ToolCreate`) | `letta/functions/functions.py:287-388` |
| Core tool stubs | Plain Python functions with Google-style docstrings + type annotations; `Tool.notImplemented` is the pattern (e.g., `memory`, `send_message`, `archival_memory_search`) | `letta/functions/function_sets/base.py:10-100,246-280`; `letta/functions/function_sets/builtin.py:4-78`; `letta/functions/function_sets/voice.py:10-83`; `letta/functions/function_sets/files.py:10-97` |
| Multi-agent tools | Module `letta.functions.function_sets.multi_agent` exporting `send_message_to_agent_and_wait_for_reply`, `send_message_to_agents_matching_tags`, `send_message_to_agent_async` | `letta/functions/function_sets/multi_agent.py:60-190` |
| Schema generator | `generate_schema(function, ...)` parses docstring + `inspect.signature`; rejects missing type annotations and missing param descriptions | `letta/functions/schema_generator.py:409-499` |
| MCP schema generator | `generate_tool_schema_for_mcp` normalizes MCP `inputSchema` (anyOf deduplication, $ref resolution, strict-mode compatibility) | `letta/functions/schema_generator.py:694-789` |
| MCP server types | `MCPServerType = {SSE, STDIO, STREAMABLE_HTTP}`; config classes `SSEServerConfig`, `StdioServerConfig`, `StreamableHTTPServerConfig` (with templated auth `{{ VAR \| default }}`) | `letta/functions/mcp_client/types.py:36-296` |
| MCP wrapper stub | `generate_mcp_tool_wrapper(mcp_tool_name)` returns a Python stub that raises `RuntimeError` if executed directly | `letta/functions/helpers.py:19-28` |
| LangChain wrapper | `generate_langchain_tool_wrapper` produces a wrapper that imports + instantiates a langchain class and calls `_run` (legacy path) | `letta/functions/helpers.py:31-59` |
| MCP tool model | `MCPTool` extends `mcp.Tool` and adds optional `health: MCPToolHealth` | `letta/functions/mcp_client/types.py:21-34` |
| ORM model | `Tool(SqlalchemyBase, OrganizationMixin, ProjectMixin)` with unique `(name, organization_id)`, `(organization_id, project_id, name)` and indexes on `created_at,name`, `organization_id` | `letta/orm/tool.py:17-61` |
| Manager service | `ToolManager` exposes `create_or_update_tool_async`, `create_tool_async`, `bulk_upsert_tools_async`, `get_tool_by_id_async`, `list_tools_async`, `upsert_base_tools_async`, etc. | `letta/services/tool_manager.py:206-1452` |
| Atomic upsert | `_atomic_upsert_tool_postgresql` uses `INSERT … ON CONFLICT (name, organization_id) DO UPDATE` | `letta/services/tool_manager.py:285-370` |
| Concurrency safety | `_check_tool_name_conflict_with_lock_async` uses `SELECT … FOR UPDATE (nowait=False)` | `letta/services/tool_manager.py:583-611` |
| Tool hash for Modal | `metadata_["tool_hash"] = compute_tool_hash(tool)`; diff triggers Modal redeploy | `letta/services/tool_manager.py:306-309, 1011-1013` |
| Bootstrap | `Server.init_async` calls `tool_manager.upsert_base_tools_async(actor=self.default_user)` | `letta/server/server.py:386` |
| Auto-sync on list | `list_tools_async` computes `LETTA_TOOL_SET - existing_tool_names` and re-runs `upsert_base_tools_async` if any missing | `letta/services/tool_manager.py:650-674` |
| Upsert base flow | Imports every `LETTA_TOOL_MODULE_NAMES` module, calls `load_function_set`, maps each name → `ToolType` via the membership tests against `BASE_TOOLS`, `BASE_MEMORY_TOOLS`, etc., then bulk-upserts | `letta/services/tool_manager.py:1147-1208` |
| Executor factory | `ToolExecutorFactory._executor_map: ClassVar[Dict[ToolType, Type[ToolExecutor]]]` with default fallback to `SandboxToolExecutor` | `letta/services/tool_executor/tool_execution_manager.py:32-66` |
| Core executor dispatch | `function_map` dict in `LettaCoreToolExecutor.execute` mapping string names to bound methods | `letta/services/tool_executor/core_tool_executor.py:41-76` |
| Builtin executor dispatch | `function_map` for `run_code`, `run_code_with_tools`, `web_search`, `fetch_webpage` (E2B-backed) | `letta/services/tool_executor/builtin_tool_executor.py:32-50` |
| MCP executor | `ExternalMCPToolExecutor` parses MCP server name from `tag.startswith("mcp:")`, delegates to `MCPManager.execute_mcp_server_tool` | `letta/services/tool_executor/mcp_tool_executor.py:22-87` |
| Sandboxed execution | `SandboxToolExecutor` selects Modal → E2B → Local; serializes agent_state for copy; verifies memory hash unchanged | `letta/services/tool_executor/sandbox_tool_executor.py:24-148` |
| Arg coercion | AST parser reads annotations from source and coerces JSON-string args to declared types (with `allow_unsafe_eval` for Modal path) | `letta/functions/ast_parsers.py:90-168` |
| MCP listing + health | `MCPServerManager.list_mcp_server_tools` connects, normalizes schema, computes `MCPToolHealth` (STRICT_COMPLIANT / NON_STRICT_ONLY / INVALID) | `letta/services/mcp_server_manager.py:163-190` |
| MCP resync | `resync_mcp_server_tools` reconciles persisted vs. live tools: deletes missing, updates changed, adds new (skipping INVALID health) | `letta/services/mcp_server_manager.py:284-396` |
| MCP add single | `add_tool_from_mcp_server` invokes `ToolCreate.from_mcp` → `tool_manager.create_mcp_tool_async` → `MCPToolsModel` mapping | `letta/services/mcp_server_manager.py:226-281` |
| Server-side OAuth | `ServerSideOAuth` + `MCPOAuth` ORM session model for MCP OAuth flows | `letta/services/mcp/server_side_oauth.py`, `letta/orm/mcp_oauth.py` |
| REST surface | `POST /tools`, `PUT /tools`, `PATCH /tools/{id}`, `DELETE /tools/{id}`, `GET /tools`, `POST /tools/search`, `POST /tools/add-base-tools`, `POST /tools/run`, MCP group (`/tools/mcp/servers/...`) | `letta/server/rest_api/routers/v1/tools.py:44-859` |
| Filter scopes | `tool_types`, `exclude_tool_types`, `names`, `tool_ids`, `project_id`, `search`, `return_only_letta_tools`, `exclude_letta_tools`; project filter uses `project_id == X OR project_id IS NULL` | `letta/services/tool_manager.py:679-799` |
| Org isolation | All queries scoped by `actor.organization_id`; `_check_tool_name_conflict_with_lock_async` enforces it | `letta/services/tool_manager.py:567-611` |
| Test helpers | `tests.helpers.utils.create_tool_from_func(func)` constructs a `Tool` from any Python callable using `parse_source_code` + `generate_schema` | `letta/tests/helpers/utils.py:93-101` |
| Tests of builtin tools | `tests/integration_test_builtin_tools.py` exercises `run_code`, `web_search`, `programmatic_tool_calling_compose_tools`, `run_code_injects_tool_source_code` | `letta/tests/integration_test_builtin_tools.py:124-436` |
| MCP schema tests | `tests/test_tool_schema_parsing.py`, `tests/test_tool_schema_parsing_files/` exercise schema generation pipeline | `letta/tests/test_tool_schema_parsing.py`, `letta/tests/test_tool_schema_parsing_files/` |
| OTel tracing | `@trace_method` decorators on every ToolManager method and on every executor's `execute` | `letta/services/tool_manager.py:33,209,211, ...`; `letta/services/tool_executor/*.py` |
| Tool-type enforcement | `Tool.update_tool_by_id_async` checks `f"def {new_name}" in source_code` (Python) or `f"function {new_name}" in source_code` (TS) before accepting rename | `letta/services/tool_manager.py:946-966` |

## Answers to Dimension Questions

1. **How does a new tool enter the system?**
   Four paths, each mediated by `ToolManager`:
   - **Custom (Python or TypeScript):** `POST /v1/tools/` or `PUT /v1/tools/` with a `ToolCreate` body; `create_or_update_tool_async` derives the JSON schema if absent (`letta/services/tool_manager.py:209-281`).
   - **Built-in sync:** Server startup (`letta/server/server.py:386`) or `POST /v1/tools/add-base-tools` walks `LETTA_TOOL_MODULE_NAMES`, imports each module, calls `load_function_set`, and bulk-upserts the names in `LETTA_TOOL_SET` with the appropriate `ToolType` (`letta/services/tool_manager.py:1137-1208`).
   - **MCP:** `POST /v1/tools/mcp/servers/{name}/resync` reconciles DB rows with the live server, and `POST /v1/tools/mcp/servers/{name}/{tool_name}` adds a single tool by calling `ToolCreate.from_mcp` → `ToolManager.create_mcp_tool_async` (`letta/services/mcp_server_manager.py:226-396`).
   - **Lazy auto-repair:** Any `list_tools_async` invocation with no cursor detects missing base-tool rows and re-runs `upsert_base_tools_async` (`letta/services/tool_manager.py:650-674`).

2. **Is tool registration explicit?**
   Yes. Built-in registration is explicit via the `LETTA_TOOL_MODULE_NAMES` list and per-bucket name constants (`BASE_TOOLS`, `BASE_MEMORY_TOOLS`, `MULTI_AGENT_TOOLS`, `BUILTIN_TOOLS`, `FILES_TOOLS`). Custom tools are explicit API calls. There is no Python-side decorator or auto-discovery from `dir(package)`.

3. **Can tools be discovered dynamically?**
   Discovery is split:
   - **MCP:** `list_mcp_server_tools` connects to a registered server, normalizes each tool's JSON schema, computes a health status, and returns them (`letta/services/mcp_server_manager.py:163-190`). The schema is then reified into a `Tool` row via `ToolCreate.from_mcp`.
   - **Tool search:** `ToolSearchRequest` supports `vector`, `fts`, and `hybrid` modes via Turbopuffer (`letta/schemas/tool.py:227-234`), and tools can be filtered by `tool_types`, `tags`, `project_id` (`letta/services/tool_manager.py:679-799`).
   - **No runtime introspection of unknown Python packages.** Tools from arbitrary code must be uploaded via the API as source.

4. **Are names stable?**
   Names are the canonical identifier for agents and for internal routing, but they live behind the per-org DB unique constraint (`(name, organization_id)`, `letta/orm/tool.py:30-32`). Renames require both the new `name` field and a matching `def <new_name>` (Python) or `function <new_name>` (TypeScript) in `source_code` (`letta/services/tool_manager.py:946-966`), plus a matching JSON schema name (`letta/services/tool_manager.py:237-242`). MCP tool names must equal the server's `MCPTool.name`. Tool names are capped at 48 characters to fit Modal function names (`letta/constants.py:69-72`).

5. **Can multiple agents see different tool sets?**
   Yes, via three layers:
   - **Organization isolation:** Every query is scoped by `actor.organization_id` (`letta/services/tool_manager.py:567-611, 699`).
   - **Project scoping:** `project_id` filter matches tools whose `project_id` equals the agent's project OR is `NULL` (global); a `UNIQUE (organization_id, project_id, name)` constraint prevents collisions (`letta/orm/tool.py:32`, `letta/services/tool_manager.py:702-704`).
   - **Per-agent attachment:** The `tools_agents` join table attaches specific tool IDs to each agent. Agents also have `tool_rules` and `tool_type` filters (`ToolRuleType` enum at `letta/schemas/enums.py:182-197`). Built-in agent types (`AgentType`) such as `memgpt_agent`, `letta_v1_agent`, `voice_convo_agent` map to different base-tool buckets (`letta/constants.py:115-151`).

## Architectural Decisions

- **Plain Python functions + docstring-derived JSON schema.** No decorators or metaclasses; schemas come from `inspect.signature` + Google-style docstring parsing (`letta/functions/schema_generator.py:409-499`). Trade-off: simpler tool authorship but every parameter must be type-annotated and documented or schema generation raises.
- **Pydantic-first schema layer with SQLAlchemy ORM.** `Tool` (Pydantic, `letta/schemas/tool.py:35`) and `ToolModel` (SQLAlchemy, `letta/orm/tool.py:17`) are bridged by `to_pydantic()`; `_id_prefix_` comes from the `PrimitiveType.TOOL` enum (`letta/schemas/enums.py:21, 31-43`).
- **Hard-coded built-in registry.** `LETTA_TOOL_MODULE_NAMES` + `LETTA_TOOL_SET` make the list of "first-class" built-in tools explicit and greppable; runtime dispatch lives in `function_map` dicts inside executors (`core_tool_executor.py:41-56`, `builtin_tool_executor.py:32-37`).
- **One manager, many executors.** `ToolManager` owns persistence; `ToolExecutorFactory` owns dispatch (`letta/services/tool_executor/tool_execution_manager.py:35`). New tool kinds require (a) adding to `ToolType`, (b) creating an executor, (c) registering in `_executor_map`.
- **Per-tool sandbox flag.** `metadata_["sandbox"] == "modal"` opts a custom tool into Modal execution; absence falls back to E2B or local based on credentials (`letta/services/tool_executor/sandbox_tool_executor.py:69-128`).
- **MCP servers are first-class resources.** `MCPServerModel` + `MCPTools` mapping table preserve the relationship between a server and its tools; per-server auth, custom headers, OAuth, and tool health are tracked separately (`letta/schemas/mcp.py:29-100`, `letta/orm/mcp_server.py`, `letta/services/mcp_server_manager.py:56-396`).
- **Atomic PostgreSQL upsert.** `_atomic_upsert_tool_postgresql` uses `ON CONFLICT (name, organization_id) DO UPDATE` for race-free multi-writer registration (`letta/services/tool_manager.py:285-370`).
- **Tool hash for cache invalidation.** `compute_tool_hash` lets Modal-sandboxed tools detect source changes and redeploy only when needed (`letta/services/tool_manager.py:306-309, 1011-1062`).

## Notable Patterns

- **Function-set module convention.** `letta/functions/function_sets/*.py` modules are loaded by `load_function_set`, which iterates `dir(module)`, skips underscore-prefixed callables, rejects duplicates, and generates schemas via `generate_schema` (`letta/functions/functions.py:390-417`).
- **Stub-and-dispatch.** Built-in functions raise `NotImplementedError` so accidental direct execution is loud; real logic is in the executors' `function_map` (`letta/functions/function_sets/builtin.py:14-78`, `letta/services/tool_executor/core_tool_executor.py:41-56`).
- **Health-aware MCP integration.** `MCPToolHealth` classifies schemas as `STRICT_COMPLIANT` / `NON_STRICT_ONLY` / `INVALID`; `INVALID` tools are skipped during resync (`letta/functions/mcp_client/types.py:21-34`, `letta/services/mcp_server_manager.py:266-279, 375-379`).
- **Templated MCP auth.** Server configs allow `{{ VAR | default }}` substitution in tokens/headers via `BaseServerConfig.get_tool_variable` (`letta/functions/mcp_client/types.py:58-99`).
- **Two-phase schema generation.** Python custom tools without an explicit `json_schema` first try AST parsing, then fall back to docstring parsing (`letta/services/tool_schema_generator.py:18-100`); TypeScript requires an explicit schema (`letta/schemas/tool.py:138-143`).
- **Project-aware global fallback.** Tools without a `project_id` (NULL) are visible to every project; tools with `project_id` are restricted to that project (`letta/services/tool_manager.py:702-704`).
- **Bulk-upsert with deadlock-safe ordering.** `_bulk_upsert_postgresql` sorts by `tool.name` to acquire row locks in a consistent order (`letta/services/tool_manager.py:1218-1223`).

## Tradeoffs

- **Adding a built-in tool requires multi-file edits.** New function → add to a module file, add the name to a `BASE_*` constant, possibly add to `LETTA_TOOL_SET`, and possibly add a `ToolType`. There is no single registry decorator.
- **Schema/runtime name duplication.** The executor `function_map` must be kept in sync with the function-set modules; drift yields `ValueError("Unknown function: ...")` at runtime (`letta/services/tool_executor/core_tool_executor.py:58-60`).
- **MCP execution re-connects per call.** No persistent MCP client pool; `execute_tool_async` opens a fresh client in `MCPServerManager.execute_mcp_server_tool` (`letta/services/mcp_server_manager.py:192-223`), adding latency.
- **Strict upsert semantics for tags.** Empty `tags=[]` from an upsert is intentionally ignored (documented as a "TODO intentional bug") so existing tags cannot be cleared via the normal API path (`letta/services/tool_manager.py:334-339`).
- **Schema fragility.** Custom tools without explicit `json_schema` rely on AST parsing of docstrings; complex Pydantic models with forward references or non-Google-style docs can fail (`letta/functions/functions.py:287-388`).
- **Mixed Python execution paths.** Core/builtin tools run in-process inside the server; custom tools run in a sandbox (E2B/Modal/local). Latency and error semantics differ between the two, which complicates debugging.

## Failure Modes / Edge Cases

- **Missing type annotation** → `generate_schema` raises `TypeError` (`letta/functions/schema_generator.py:457-458`).
- **Missing docstring parameter description** → `ValueError` during schema generation (`letta/functions/schema_generator.py:464-465`).
- **TypeScript tool without explicit schema** → `ToolCreate.validate_typescript_requires_schema` rejects with `ValueError` (`letta/schemas/tool.py:138-143`).
- **Tool name > 48 chars** → `LettaInvalidArgumentError` at upsert (`letta/services/tool_manager.py:232-235`).
- **Schema name ≠ source code name** → `LettaToolNameSchemaMismatchError` (`letta/services/tool_manager.py:237-242`).
- **Concurrent rename to same name** → `_check_tool_name_conflict_with_lock_async` blocks on `SELECT FOR UPDATE`; loser raises `LettaToolNameConflictError` (`letta/services/tool_manager.py:1019-1027`).
- **MCP server unreachable** → `list_mcp_server_tools` logs and re-raises (`letta/services/mcp_server_manager.py:183-187`); resync endpoint translates to `HTTPException(404, code=MCPServerUnavailable)` (`letta/services/mcp_server_manager.py:308-315`).
- **INVALID MCP schema** → single-add logs warning but proceeds; resync skips it entirely (`letta/services/mcp_server_manager.py:266-279, 375-379`).
- **Malformed tool row** → `_list_tools_async` catches `ValueError` / `ModuleNotFoundError` / `AttributeError` and deletes the offending row (`letta/services/tool_manager.py:780-797`).
- **`agent_state.tools` is deep-copied before sandbox execution** to prevent nested tool calls from leaking mutations (`letta/services/tool_executor/sandbox_tool_executor.py:171-178`).
- **Memory integrity assertion** after sandbox execution (`letta/services/tool_executor/sandbox_tool_executor.py:135-138`).
- **MCP stdio disabled by default** for multi-tenant safety (`letta/settings.py:45-54`).

## Future Considerations

- **Centralize the function-map dispatch.** Today each executor duplicates the function-name → method mapping; a shared registry indexed by `Tool` metadata could remove the drift risk between `function_sets/*.py` and the executor dispatch tables.
- **Cache MCP clients.** Per-server connection pooling would cut latency on hot agents (`letta/services/mcp_server_manager.py:200-223`).
- **Schema-source-of-truth unification.** Tools store both `json_schema` and `source_code`; a diff/regenerate path that always derives schema from source for `CUSTOM` tools would simplify the create-vs-update branching in `ToolManager`.
- **Tag-clearing flow.** Resolve the documented "intentional bug" that prevents clearing tags on upsert (`letta/services/tool_manager.py:334-339`).
- **Decorator or module-attribute alternative.** A `@letta_tool` decorator or a per-module `__letta_tools__` list would let new built-in tools be added without editing `constants.py`; trade-off is increased surface area for accidental registration.
- **Polymorphic ORM model.** The `Tool` model already has a TODO comment suggesting polymorphic inheritance for "superset always available + subset scoped to org" (`letta/orm/tool.py:19-23`); this could simplify executor dispatch.
- **Project-scoped MCP servers.** Currently MCP servers are org-scoped; multi-tenant deployments may want per-project MCP server registries.

## Questions / Gaps

- No evidence found that tools can be hot-reloaded (changing `source_code` for a built-in tool without restart): `Tool.refresh_source_code_and_json_schema` only refreshes built-in JSON schemas on read, not their source_code (`letta/schemas/tool.py:74-107`).
- No evidence found that a generic plugin/entry-point mechanism exists for third parties to add new tool categories without editing the `LETTA_TOOL_MODULE_NAMES` and `ToolType` enum.
- No evidence found that name-conflict detection spans cross-tool-type namespaces (e.g., a custom tool and an MCP tool with the same name within one organization): the unique constraint is `(name, organization_id)` only (`letta/orm/tool.py:31`), not `(name, tool_type, organization_id)`.
- No evidence found of a schema versioning story: `json_schema` is overwritten in place during updates; old versions are not retained (`letta/services/tool_manager.py:881-1086`).

---

Generated by `04.01-tool-definition-and-registration.md` against `letta`.
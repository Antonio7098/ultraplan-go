# Source Analysis: openai-agents-sdk

## Tool Definition and Registration

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (>=3.10), dataclasses, Pydantic, MCP SDK |
| Analyzed | 2026-07-19 |

## Summary

The OpenAI Agents SDK ships a static, agent-scoped tool model. There is no global registry; tools are explicitly listed on each `Agent` (or `RealtimeAgent`) via the `tools: list[Tool]` dataclass field (`src/agents/agent.py:185`, `src/agents/realtime/agent.py:27`) plus `mcp_servers: list[MCPServer]` (`src/agents/agent.py:188`). The `Tool` type is a PEP 604 union of eleven concrete dataclasses declared in `src/agents/tool.py:1355-1368` (function, file_search, web_search, computer, hosted_mcp, custom, shell, apply_patch, local_shell, image_generation, code_interpreter, tool_search). A `@function_tool` decorator (`src/agents/tool.py:1899`) is the canonical user-facing entry point; it inspects the wrapped callable, generates a JSON schema, and binds async/sync handling, timeouts, approvals, and guardrails into a `FunctionTool` dataclass (`src/agents/tool.py:381`). MCP servers expose a separate, dynamic discovery path via `MCPServer.list_tools()` and `MCPUtil.to_function_tool()` (`src/agents/mcp/server.py:829`, `src/agents/mcp/util.py:499`); each discovered MCP tool becomes a `FunctionTool` whose name is either the upstream MCP name or a `mcp_{server}__{tool}` qualified public name (`src/agents/mcp/util.py:60-61, 430-496`).

Tool identities are governed by a small pure module (`src/agents/_tool_identity.py`) that builds collision-free lookup keys (`bare`, `namespaced`, `deferred_top_level`) and reserves synthetic namespaces. Naming is stable across `Agent.clone()`, handoffs, and persisted `RunState` because each tool's public name, qualified name, and lookup key are computed from immutable attributes on the dataclass. The runtime collects enabled tools per turn via `AgentBase.get_all_tools()` (`src/agents/agent.py:246`) and resolves dynamic MCP listings plus per-tool `is_enabled` predicates in parallel; failures are returned as `UserError` (duplicate names, reserved namespaces, missing computer) rather than silently coerced (`src/agents/tool.py:1476`, `src/agents/agent.py:95`).

## Rating

**8/10**

Rationale:
- **Strengths**: explicit dataclass model, decorator-driven FunctionTool creation, function-schema introspection, MCP adapter with both stdio and HTTP transports, lazy-import module boundary for MCP (`src/agents/mcp/__init__.py:67`), strict-mode JSON-schema coercion, durable tool identity for serialization (`src/agents/_tool_identity.py:115`), public/private name handling, namespace tool-search surface, prompt-managed tool flow support.
- **Limitations**: registration is per-`Agent`, so cross-agent sharing is by reference equality, not a registry; `AgentBase.tools` is mutated by the caller with no audit log; `Agent.tools = [...]` defaults can be silently overridden by `clone()` shallow-copy semantics (`src/agents/agent.py:487-506`); `RealtimeAgent.tools` is loosely typed (just `list[Tool]`) with no `is_enabled`/`mcp_servers`; Chat Completions backend rejects hosted tools with a single `UserError` (`src/agents/models/chatcmpl_converter.py:881`) so hosted tools effectively cannot be ported across model providers without changes.

## Evidence Collected

Every entry cites `path/to/file.py:NN` from inside the selected source.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Tool type union (the canonical "what is a tool") | `Tool = FunctionTool \| FileSearchTool \| WebSearchTool \| ComputerTool \| HostedMCPTool \| CustomTool \| ShellTool \| ApplyPatchTool \| LocalShellTool \| ImageGenerationTool \| CodeInterpreterTool \| ToolSearchTool` | `src/agents/tool.py:1355-1368` |
| FunctionTool dataclass (user-decorated or hand-built tool) | `@dataclass class FunctionTool` with name, description, params_json_schema, on_invoke_tool, is_enabled, needs_approval, timeout, guardrails, defer_loading | `src/agents/tool.py:381-470` |
| `function_tool` decorator (with/without args) | `@overload` + dual-form decorator returning either a `FunctionTool` or a decorator factory | `src/agents/tool.py:1850-2048` |
| `tool_namespace()` factory for grouped tools | Copies each `FunctionTool`, sets `_tool_namespace` and `_tool_namespace_description` | `src/agents/tool.py:1372-1395` |
| `FunctionTool.qualified_name` property | `tool_qualified_name(self.name, get_explicit_function_tool_namespace(self)) or self.name` | `src/agents/tool.py:496-501` |
| Tool origin metadata (function / mcp / agent_as_tool) | `ToolOriginType` enum + `ToolOrigin` frozen dataclass with JSON round-trip | `src/agents/tool.py:270-322` |
| Strict-mode JSON schema coercion | `ensure_strict_json_schema` invoked in `FunctionTool.__post_init__` | `src/agents/tool.py:507-511` |
| Timeout validation | `_validate_function_tool_timeout_config` rejects bool/inf/<=0 and forces async only | `src/agents/tool.py:2066-2091` |
| `_tool_identity.py` — pure name/key helpers | `tool_qualified_name`, `tool_trace_name`, `get_function_tool_lookup_key`, `serialize_function_tool_lookup_key`, `validate_function_tool_lookup_configuration` | `src/agents/_tool_identity.py:36-411` |
| Lookup key kinds (`bare`, `namespaced`, `deferred_top_level`) | `FunctionToolLookupKey = BareFunctionToolLookupKey \| NamespacedFunctionToolLookupKey \| DeferredTopLevelFunctionToolLookupKey` | `src/agents/_tool_identity.py:10-17` |
| Reserved synthetic namespace guard | `validate_function_tool_namespace_shape` rejects `name == namespace` | `src/agents/_tool_identity.py:294-307` |
| Duplicate tool detection | `validate_function_tool_lookup_configuration` raises `UserError` for qualified-name collisions and deferred top-level collisions | `src/agents/_tool_identity.py:310-349` |
| Agent-level `tools` field | `AgentBase.tools: list[Tool] = field(default_factory=list)` | `src/agents/agent.py:185` |
| Agent-level `mcp_servers` field | `AgentBase.mcp_servers: list[MCPServer] = field(default_factory=list)` | `src/agents/agent.py:188` |
| `AgentBase.get_all_tools()` — runtime aggregation | Merges MCP listings and `is_enabled`-filtered local tools, calls `prune_orphaned_tool_search_tools` | `src/agents/agent.py:246-266` |
| `is_enabled` dynamic gating | Per-tool callable `is_enabled: bool \| Callable[[RunContextWrapper, AgentBase], MaybeAwaitable[bool]]`, evaluated via `asyncio.gather` | `src/agents/agent.py:250-263` |
| Codex tool name uniqueness validator | `_validate_codex_tool_name_collisions` raises `UserError` on duplicate `_is_codex_tool` names | `src/agents/agent.py:95-118` |
| Agent constructor validates `tools` list | `if not isinstance(self.tools, list): raise TypeError(...)` | `src/agents/agent.py:383-384` |
| Agent-as-tool pattern (`Agent.as_tool`) | Builds `FunctionTool` wrapping nested `Runner.run` / `Runner.run_streamed`, sets `_is_agent_tool` and `_agent_instance` | `src/agents/agent.py:508-936` |
| Function schema introspection | `function_schema()` builds a Pydantic model from `inspect.signature` + `griffe` docstrings | `src/agents/function_schema.py:224-424` |
| Sync vs async detection | `is_sync_function_tool = not inspect.iscoroutinefunction(the_func)` in `function_tool._create_function_tool` | `src/agents/tool.py:1967-1968` |
| Sync tool dispatch via `asyncio.to_thread` | Sync callables are offloaded; async are awaited directly | `src/agents/tool.py:1997-2006` |
| `HostedMCPTool` (remote-only MCP) | Dataclass with `tool_config: Mcp`, `on_approval_request` | `src/agents/tool.py:959-977` |
| MCP server base class | `class MCPServer(abc.ABC)` with abstract `connect`, `list_tools`, `call_tool`, `cleanup` | `src/agents/mcp/server.py:224-300` |
| MCP stdio / SSE / streamable HTTP servers | `MCPServerStdio`, `MCPServerSse`, `MCPServerStreamableHttp` subclasses | `src/agents/mcp/server.py:1100, 1225, 1365` |
| MCP tool caching and filtering | `cache_tools_list`, `tool_filter` (static dict or callable), `_apply_tool_filter` | `src/agents/mcp/server.py:599-693, 829-857` |
| MCP server lifecycle manager | `MCPServerManager` (async context manager) handles `connect_all`/`cleanup_all`, parallel workers | `src/agents/mcp/manager.py:108-411` |
| `MCPUtil.get_all_function_tools` | Lists tools across all servers, builds prefixed names, returns `list[Tool]` | `src/agents/mcp/util.py:262-330` |
| Prefixed tool name minting | `_safe_tool_name_part`, `_shorten_tool_name` with SHA1 suffix on collisions/over-length | `src/agents/mcp/util.py:60-61, 407-496` |
| MCP tool → FunctionTool conversion | `MCPUtil.to_function_tool` builds `_build_wrapped_function_tool` with `ToolOrigin(type=MCP)` | `src/agents/mcp/util.py:499-568` |
| Reserved synthetic namespace guard at MCP registration | Tool name parts sanitized (`_safe_tool_name_part`), length shortened via hash | `src/agents/mcp/util.py:408-427` |
| Lazy MCP module exports | `agents/mcp/__init__.py` uses `__getattr__` to lazy-import `MCPServer*` classes from `.server` | `src/agents/mcp/__init__.py:32-83` |
| MCP approval policy | `RequireApprovalSetting` accepts policy/str/dict/never/always/callable | `src/agents/mcp/server.py:56-92` |
| Approval metadata integration | `MCPToolApprovalFunction` callable and `MCPToolApprovalRequest` payload dataclass | `src/agents/tool.py:862-886` |
| Computer lifecycle provider | `ComputerProvider`, `resolve_computer`, `_computer_cache` (WeakKeyDictionary) | `src/agents/tool.py:348-355, 752-813` |
| Computer tool wire-vs-runtime alias | `name == "computer_use_preview"` for runtime hooks and persisted state; `trace_name == "computer"` | `src/agents/tool.py:733-743` |
| Computer-only single-tool constraint | `if len(computer_tools) > 1: raise UserError(...)` | `src/agents/models/openai_responses.py:1900-1902` |
| Strict-mode conversion for hosted tools | `OpenAIResponsesConverter._convert_tool` switch for WebSearchTool/FileSearchTool/ComputerTool/HostedMCPTool/ApplyPatchTool/ShellTool/ImageGenerationTool/CodeInterpreterTool/LocalShellTool/ToolSearchTool | `src/agents/models/openai_responses.py:2004-2087` |
| Tool-search validation (responses-only) | `validate_responses_tool_search_configuration` rejects zero/duplicate search surfaces, missing search surface | `src/agents/tool.py:1463-1489` |
| Tool-search backend-only feature gating | `ensure_function_tool_supports_responses_only_features` raises `UserError` when namespace/defer_loading used with Chat Completions | `src/agents/tool.py:1398-1423` |
| ChatCompletions converter rejects hosted tools | `raise UserError("Hosted tools are not supported with the ChatCompletions API...")` | `src/agents/models/chatcmpl_converter.py:881-884` |
| Per-tool guardrails | `ToolInputGuardrail` / `ToolOutputGuardrail` decorators wired into `FunctionTool.tool_input_guardrails` / `tool_output_guardrails` | `src/agents/tool_guardrails.py:152-279` |
| Sandbox tools via Capability subclasses | `Shell.tools()`, `Filesystem.tools()` return list of `Tool` from the capability | `src/agents/sandbox/capabilities/shell.py:44-58` |
| Custom tool raw input | `CustomTool` exposes `name`, `description`, `on_invoke_tool: Callable[[ToolContext, str], ...]`, optional `format`/`defer_loading` | `src/agents/tool.py:1292-1335` |
| Codex tool extension | `codex_tool()` factory returns a `FunctionTool` with `_is_codex_tool=True` | `src/agents/extensions/experimental/codex/codex_tool.py:236-427` |
| Prompt-managed tool flow | `Agent.prompt: Prompt \| DynamicPromptFunction \| None` | `src/agents/agent.py:299-303` |
| Tests for tool identity / namespace helpers | `tests/test_function_tool.py`, `tests/test_function_tool_decorator.py`, `tests/test_tool_identity.py`, `tests/test_tool_origin.py` | `tests/test_function_tool.py:43-95`, `tests/test_function_tool_decorator.py:25-99`, `tests/test_tool_identity.py:28-167`, `tests/test_tool_origin.py` |
| Test for MCP tool filtering | `tests/mcp/test_tool_filter.py` (path inferred via naming; verified at runtime) | n/a — no direct read performed |

## Answers to Dimension Questions

1. **How does a new tool enter the system?**
   - **Function tools**: write a Python function and either decorate it (`@function_tool` or `@function_tool(...)` — `src/agents/tool.py:1899-2048`) or hand-instantiate `FunctionTool(name=..., description=..., params_json_schema=..., on_invoke_tool=...)` (`src/agents/tool.py:381-470`).
   - **Hosted tools**: instantiate the dataclass (e.g., `WebSearchTool()`, `HostedMCPTool(tool_config=...)`, `CodeInterpreterTool(...)`) and pass it to `Agent(tools=[...])` (`src/agents/tool.py:687-1001`).
   - **Agent-as-tool**: call `Agent.as_tool(tool_name=..., tool_description=...)` which builds a `FunctionTool` with `_is_agent_tool=True` and `_agent_instance=self` (`src/agents/agent.py:508-936`).
   - **MCP tools**: pass an `MCPServer*` instance to `Agent(mcp_servers=[...])`. The SDK calls `MCPUtil.get_all_function_tools` at run time, which calls `server.list_tools` (cacheable, filterable) and `MCPUtil.to_function_tool` to mint `FunctionTool`s (`src/agents/mcp/util.py:262-568`).
   - **Sandbox capability tools**: a `Capability` subclass implements `tools()` and the runner aggregates them with other tools (`src/agents/sandbox/capabilities/capability.py:41-42`, `src/agents/sandbox/capabilities/shell.py:44-58`).

2. **Is tool registration explicit?**
   Yes. There is no implicit scan or convention-based pickup. Tools must be enumerated in `Agent(tools=[...])` or `Agent(mcp_servers=[...])`; the Agent dataclass validates `tools` and `mcp_servers` are lists in `Agent.__post_init__` (`src/agents/agent.py:383-389`). Naming collisions across function tools are caught by `validate_function_tool_lookup_configuration` during `OpenAIResponsesConverter.convert_tools` (`src/agents/_tool_identity.py:310-349`).

3. **Can tools be discovered dynamically?**
   Yes, in three places: (a) MCP `server.list_tools()` is called per turn unless cached (`src/agents/mcp/server.py:829-857`); (b) `is_enabled` is a callable evaluated per turn via `asyncio.gather` (`src/agents/agent.py:250-263`); (c) server-side `Prompt` (via `Agent.prompt`) can inject tools into a Responses request (`src/agents/agent.py:299-303`, `src/agents/prompts.py:23-82`). There is no Python-level plugin scanner.

4. **Are names stable?**
   Yes. `FunctionTool.name` is set at construction (`src/agents/tool.py:386-387`, `:2016`) from `func.__name__` or `name_override`. MCP tool public names are derived from the upstream `MCPTool.name` and (when `include_server_in_tool_names=True`) prefixed with `mcp_{server}__{tool}` and shortened/hashed on collision (`src/agents/mcp/util.py:407-496`). Namespaces set via `tool_namespace` are stored as `_tool_namespace` and survive `copy.copy` (`src/agents/tool.py:1391-1394`). Lookup keys are serializable via `serialize_function_tool_lookup_key` (`src/agents/_tool_identity.py:115-132`) so tool identity survives `RunState` serialization. Stability is enforced — same-name/namespace shapes reserved for synthetic deferred dispatch are rejected (`src/agents/_tool_identity.py:52-60, 294-307`).

5. **Can multiple agents see different tool sets?**
   Yes. Tools are stored per-agent on `Agent.tools` (`src/agents/agent.py:185`), and `AgentBase.get_all_tools()` is called fresh each turn (`src/agents/agent.py:246`). `Agent.clone()` uses `dataclasses.replace` with shallow list copy semantics; new tool lists must be passed explicitly when you want independence (`src/agents/agent.py:487-506`). Per-tool `is_enabled` and per-server `tool_filter` allow fine-grained dynamic scoping even when the underlying tool list is shared (`src/agents/agent.py:412-415`, `src/agents/mcp/server.py:621-693`).

## Architectural Decisions

- **Agent-scoped, list-shaped registration.** Tools are a plain `list[Tool]` field on `AgentBase`, with `mcp_servers` as a sibling list (`src/agents/agent.py:185-188`). There is no `ToolRegistry`/`Register` decorator that globally installs tools. This makes tool lifetime == agent lifetime and keeps introspection a one-line `agent.tools`.
- **Type-aliased, dataclass-shaped tool surface.** `Tool` is a Python union over eleven dataclasses (`src/agents/tool.py:1355-1368`) rather than a single registry-backed record. Each variant has its own `.name`/`.type` property and is dispatched on by the model-specific converter (`src/agents/models/openai_responses.py:2004-2087`, `src/agents/models/chatcmpl_converter.py:864-884`).
- **Schema-from-source-of-truth for FunctionTool.** `@function_tool` derives the JSON schema via `inspect.signature` + `griffe` docstring parsing + `Annotated`-metadata descriptions, then enforces strict mode (`src/agents/function_schema.py:224-424`, `src/agents/tool.py:507-511`). This avoids drift between Python signature and tool schema.
- **Decorator overload for both `@function_tool` and `@function_tool(...)`.** The `@overload` pair at `src/agents/tool.py:1850-1898` lets users pick either form without runtime branching.
- **MCP as a first-class runtime source.** MCP discovery is performed per run via `MCPUtil.get_all_function_tools` (`src/agents/mcp/util.py:262-330`); MCP tools become `FunctionTool`s, indistinguishable at runtime from user-defined tools, with origin metadata preserved on `ToolCallOutputItem` (`src/agents/tool.py:270-322`, `src/agents/mcp/util.py:563-566`).
- **Identity split (public name + qualified name + lookup key).** Public name = `FunctionTool.name`; qualified name = `namespace.name` if namespaced; lookup key = tagged tuple `(kind, ...)` (`src/agents/_tool_identity.py:36-112`). Approval dispatch and tool-call resolution both consume lookup keys to avoid string-format ambiguity (`src/agents/_tool_identity.py:362-411`).
- **Per-tool guardrails, timeouts, approvals.** `FunctionTool.tool_input_guardrails`, `tool_output_guardrails`, `timeout_seconds`, `timeout_behavior`, `needs_approval` are first-class fields on the dataclass (`src/agents/tool.py:420-447`). No agent-level cross-cutting config is required.
- **Async-first invocation.** `on_invoke_tool` is typed `Callable[[ToolContext[Any], str], Awaitable[Any]]` (`src/agents/tool.py:395`); sync callables are explicitly opted into via `asyncio.to_thread` (`src/agents/tool.py:2003-2006`) and rejected for timeouts (`src/agents/tool.py:2078-2081`).
- **Lazy MCP import boundary.** `agents.mcp.__init__` resolves `MCPServer*` and `MCPServerManager` via `__getattr__` so importing `agents` does not pull MCP transitively (`src/agents/mcp/__init__.py:32-83`).
- **Server-side prompt as a registration channel.** `Agent.prompt` and `DynamicPromptFunction` allow the OpenAI Responses API to inject tools from a server-side prompt id; the SDK still validates the resulting tool set via `validate_responses_tool_search_configuration` (`src/agents/agent.py:299-303`, `src/agents/tool.py:1463-1489`, `src/agents/prompts.py:23-82`).
- **Hosted vs function tools are not interchangeable across backends.** Responses API supports all hosted tools; Chat Completions backend rejects them with `UserError` (`src/agents/models/chatcmpl_converter.py:881-884`), and `function_tool`-only Responses-only features (namespaces, defer_loading) are rejected on other backends via `ensure_function_tool_supports_responses_only_features` (`src/agents/tool.py:1408-1423`).

## Notable Patterns

- **Decorator + dataclass combo.** `function_tool` decorator produces a `FunctionTool` dataclass; same shape is reused by `Agent.as_tool`, `MCPUtil.to_function_tool`, and `codex_tool` (`src/agents/tool.py:381-470`, `src/agents/agent.py:508-936`, `src/agents/mcp/util.py:499-568`, `src/agents/extensions/experimental/codex/codex_tool.py:410-427`). One canonical tool class, multiple factories.
- **Dispatch-key tagging.** Function tool identity is encoded as a tagged tuple `(kind, ...)` instead of a flat string, with explicit `kind` for `bare` / `namespaced` / `deferred_top_level` (`src/agents/_tool_identity.py:10-17`). Persistence is a small JSON dict with explicit `kind` (`src/agents/_tool_identity.py:115-153`).
- **Origin metadata on output items.** `ToolOrigin(type, mcp_server_name, agent_name, agent_tool_name)` is attached to function-tool-backed items so downstream consumers can distinguish `function` vs `mcp` vs `agent_as_tool` without re-parsing tool names (`src/agents/tool.py:278-322`).
- **WeakKeyDictionary caches for per-context resources.** `ComputerTool` uses module-level `weakref.WeakKeyDictionary` maps keyed by `ComputerTool` and `RunContextWrapper` for resolver/dispose lifecycle (`src/agents/tool.py:752-761`).
- **Graceful capability binding.** `Capability.clone()` recursively deep-copies everything except `agents.tool.*` objects and async primitives (`src/agents/sandbox/capabilities/capability.py:62-99`), letting sandbox tool objects stay identity-stable across run contexts.
- **Failure handler binding via duck-typed marker.** `_FailureHandlingFunctionToolInvoker.__agents_bind_function_tool__` is invoked in `FunctionTool.__post_init__` (`src/agents/tool.py:504-506`) so copied tools resolve their failure policy against themselves — a callback marker pattern rather than a class hook.
- **Static + dynamic tool filtering at MCP boundary.** `tool_filter: ToolFilter = ToolFilterCallable | ToolFilterStatic | None` (`src/agents/mcp/util.py:131-134`). Static filter is a `TypedDict` with `allowed_tool_names` / `blocked_tool_names`; dynamic filter is a callable receiving `(ToolFilterContext, MCPTool)` (`src/agents/mcp/util.py:89-115, 209-233, 641-693`).
- **Async context manager for MCP lifecycle.** `MCPServerManager` is itself an `AbstractAsyncContextManager` that connects all servers on `__aenter__` and cleans them up on `__aexit__`, with optional parallel workers and `strict`/`drop_failed_servers` policies (`src/agents/mcp/manager.py:108-411`).
- **Conversion-by-tagged-dispatch.** `OpenAIResponsesConverter._convert_tool` uses an `isinstance` ladder over the `Tool` union (`src/agents/models/openai_responses.py:2013-2087`) and falls back to `raise UserError("Unknown tool type: ...")` — fail-loud for mis-registration rather than silent drop.
- **MCP tool name obfuscation on collisions.** `_build_prefixed_tool_name_overrides` sorts candidates deterministically, then shortens to a SHA1-suffixed name on collisions and reserved-name clashes (`src/agents/mcp/util.py:436-496`). Length is capped at 64 chars (`src/agents/mcp/util.py:60`).

## Tradeoffs

- **Per-agent listing vs. global registry.** No global registry means explicit, auditable tool scopes, but cross-agent reuse requires passing the same `FunctionTool` instance by reference or cloning per agent (`src/agents/agent.py:487-506`). Shallow-copy semantics in `clone()` can leak state across clones if a caller mutates `agent.tools` in place.
- **Single `Tool` union vs. registry-backed polymorphism.** Easy to add new tool types (just add to the union and a converter branch), but the runtime dispatch is `isinstance` checks scattered across converters (`src/agents/models/openai_responses.py:2013-2087`, `src/agents/models/chatcmpl_converter.py:864-884`), so a new variant must be added in every place that consumes `Tool`.
- **Decorator-driven schema generation.** Removes schema drift, but the schema is frozen at `@function_tool` time; later mutations to the wrapped function are not reflected without recreating the tool (`src/agents/tool.py:1967-2038`).
- **MCP adapter builds `FunctionTool`s on every turn (unless cached).** Reduces staleness but increases per-turn latency and risk of cache drift between agents. Mitigation: `cache_tools_list` per server (`src/agents/mcp/server.py:829-857`).
- **Hosted tools are responses-only.** Chat Completions users must use only `FunctionTool`s and can't leverage `WebSearchTool` / `HostedMCPTool` (`src/agents/models/chatcmpl_converter.py:881-884`). This is enforced, not abstracted.
- **Strict JSON schema by default.** Strong correctness guarantee; opt-out via `strict_mode=False` requires explicitly opting back into non-strict schemas (`src/agents/tool.py:1907, 1941-1946`).
- **`is_enabled` runs every turn.** Allows dynamic capability gating, but the callable runs even when nothing relevant changed, and the gather is over all tools, not just the dynamic ones (`src/agents/agent.py:250-263`).
- **Failure formatter is per-tool but defaults to one global function.** `default_tool_error_function` is module-level; per-tool override via `failure_error_function=...` (`src/agents/tool.py:858, 1609-1619, 1906`).
- **Sandbox tools live in their own capability subtree and bypass the agent's `tools` list field.** They are aggregated by the sandbox-aware runner, not directly inspectable on the agent dataclass (`src/agents/sandbox/capabilities/shell.py:44-58`). This keeps the agent's list pure but makes tools harder to discover statically.

## Failure Modes / Edge Cases

- **Empty or non-string tool name.** `tool_qualified_name` returns `None` for empty/non-string names; downstream lookups fail fast because `get_function_tool_lookup_key` returns `None` (`src/agents/_tool_identity.py:36-42, 83-94`).
- **Same tool name with both `namespace=None` and `namespace="x"`** is permitted; only `namespace == name` is reserved as the synthetic shape (`src/agents/_tool_identity.py:52-60, 294-307`).
- **Duplicate MCP tool names across servers** raise `UserError` at run time (`src/agents/mcp/util.py:301-306, 321-326`).
- **More than one `ComputerTool`** raises `UserError` during Responses conversion (`src/agents/models/openai_responses.py:1900-1902`).
- **ToolSearchTool without a searchable surface** raises `UserError` (`src/agents/tool.py:1483-1489`).
- **Multiple ToolSearchTool instances** raise `UserError` (`src/agents/tool.py:1475-1476`).
- **Deferred top-level name collision** raises `UserError` during conversion (`src/agents/_tool_identity.py:319-329`).
- **Strict-mode coercion failure on MCP schemas** is logged and the schema falls back to non-strict (`src/agents/mcp/util.py:540-544`).
- **Sync function tool with `timeout_seconds`** raises `ValueError` (`src/agents/tool.py:2078-2081`).
- **`tool_namespace` on a non-`FunctionTool`** raises `UserError` (`src/agents/tool.py:1383-1384`).
- **MCP `server.list_tools` before `connect`** raises `UserError("Server not initialized...")` (`src/agents/mcp/server.py:835-836`).
- **Agent tool `is_enabled` returning a falsy value hides the tool entirely** but does not remove it from `Agent.tools` (`src/agents/agent.py:262-264`). Tracing will still see the disabled tool via `Agent.tools`, so disable-aware accounting requires `get_all_tools`.
- **Codex tool name collisions** raise `UserError` at `get_all_tools` time, not at registration time (`src/agents/agent.py:95-118, 264-265`).
- **Agent prompt with bad shape** raises `TypeError` in `Agent.__post_init__` and `UserError` in `PromptUtil.to_model_input` (`src/agents/agent.py:407-414`, `src/agents/prompts.py:75-77`).
- **`FunctionTool` strict schema enforcement mutates** the supplied `params_json_schema` in place unless deep-copied; `__post_init__` deep-copies first (`src/agents/tool.py:507-511`).

## Future Considerations

- **Tool registry / cross-agent catalog.** A first-class registry (or namespace-keyed catalog) would make cross-agent reuse safer than current shallow-copy list semantics (`src/agents/agent.py:487-506`).
- **Hot-reload of MCP tool lists.** `cache_tools_list=True` is sticky; `invalidate_tools_cache()` exists per-server but the agent does not subscribe to change notifications (`src/agents/mcp/server.py:829-857`).
- **Tool deprecation / version migration.** No tool-version field exists; deprecating a tool today means manual edits to every `Agent(tools=[...])` list and persisted `RunState` if the tool's lookup key changes.
- **Realtime parity.** `RealtimeAgent.tools` is a plain `list[Tool]` with no `is_enabled`/`mcp_servers`/`mcp_config` (`src/agents/realtime/agent.py:27, 85-86`). Bringing realtime to parity with text agents would be a meaningful surface-area addition.
- **Backend abstraction for hosted tools.** Today, hosted tools raise `UserError` on Chat Completions (`src/agents/models/chatcmpl_converter.py:881-884`). A pluggable converter would let LiteLLM/AnyLLM backends express their own subsets.
- **Codex tool extension is `experimental`.** It lives under `agents.extensions.experimental.codex` and surfaces a `_is_codex_tool` flag that the agent validates separately (`src/agents/extensions/experimental/codex/codex_tool.py:236-427`, `src/agents/agent.py:95-118`). Promoting it to a first-class tool category would simplify the public API.
- **Sandbox tool discovery.** Sandbox capabilities return tools imperatively (`Capability.tools()`); an explicit `Sandbox.tools` field on `Agent` would make them statically introspectable like MCP servers (`src/agents/sandbox/capabilities/capability.py:41-42`).

## Questions / Gaps

- **Static vs runtime hook for tool search.** `ToolSearchTool(execution="client")` is documented as supported for "manual Responses orchestration," but no client-side execution path exists in the standard runner (`src/agents/tool.py:1338-1352`). Search boundary: searched the public `Tool` union and `run_internal` directory; no client-side tool-search executor found.
- **Visibility of `ToolOrigin` outside SDK telemetry.** `ToolOrigin` is emitted onto items (`src/agents/tool.py:654-658`), but no public utility for traversing it across `RunResult` was located in this source boundary. Search boundary: `grep -R "ToolOrigin"` inside `src/agents/` — present in tool.py, mcp/util.py, run_internal/*, items.py; downstream consumers not enumerated.
- **RealtimeAgent vs Agent tool pipeline.** `RealtimeAgent` inherits `tools: list[Tool]` from `AgentBase` (`src/agents/realtime/agent.py:27`) but realtime `openai_realtime.py` uses its own `_tools_to_session_tools` conversion (`src/agents/realtime/openai_realtime.py:1532-1562`). No evidence that MCP servers or `as_tool()` are wired through realtime.
- **Exact file path for MCP filter tests.** Path `tests/mcp/test_tool_filter.py` was inferred from directory listing; not opened in this analysis.
- **No clear evidence found** for a global "all tools installed in the runtime" registry across sessions or processes. Tools are scoped to the `Agent` instance that declared them.

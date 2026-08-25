# Source Analysis: crewai

## Tool Context and Dependency Injection

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (>=3.10), Pydantic v2, LangChain interop, MCP SDK |
| Analyzed | 2026-07-27 |

## Summary

CrewAI ships an **implicit / dependency-injection-by-class-attribute** model rather than a centralized typed context object. Tools that need collaborators declare them as Pydantic fields on a `BaseTool` subclass (`tools/agent_tools/base_agent_tools.py:18`, `tools/memory_tools.py:31`, `tools/cache_tools/cache_tools.py:12`); collaborators are wired at construction time by the `Crew` (e.g. `crew.py:1787-1789` calls `read_file_tool.set_files(files)`) or by the agent (`agent/core.py:1188-1197` builds MCP tools via `MCPToolResolver`). The `ToolUsage` orchestrator (`tools/tool_usage.py:89-112`) hoards the runtime context (agent, task, tools_handler, function_calling_llm, action, fingerprint_context) on `self`, and the actual `BaseTool._run` signature stays hermetic — it receives only the LLM-supplied arguments plus its own pre-bound fields.

There is **no equivalent of `FunctionInvocationContext` or `ToolCallHookContext` that the tool itself can read**. Instead, the **only** context-bearing object the tool can observe is `ToolCallHookContext` (`hooks/tool_hooks.py:24-77`), and that is delivered to global hook callables — not to the tool's `_run`. Tools are essentially black boxes; the executor passes them parsed args and a `config={"security_context": ...}` dict for fingerprint metadata (`tool_usage.py:1023-1054`, but only used for telemetry, not propagated to `_run`). Security fingerprints and tokens live on `SecurityConfig` (`security/security_config.py:20-58`) and are read off the *agent* by the executor (`tool_usage.py:1034-1052`), never injected into the tool.

Testability is **mixed**. `BaseTool.run()` works in true isolation (`base_tool.py:314-331`), and unit tests like `tests/tools/test_base_tool.py:14-46` instantiate `Tool()` directly. But tools that *do* depend on collaborators (memory, files, delegation) hold them as Pydantic fields, which means constructing them in tests requires either real instances or `MagicMock()` injection — there is no shared "context" object the test can supply. The `ToolUsage` orchestrator does not expose a clean seam for unit-testing the wiring without `MagicMock` everywhere (`tests/tools/test_tool_usage.py:116-153` uses `MagicMock()` for `tools_handler`, `task`, `function_calling_llm`, `agent`, `action`).

Notable strengths: **typed Pydantic field injection** (every collaborator is a `Field(...)` with a description), **declarative `EnvVar` declaration** on tools (`base_tool.py:96-100, 145-148`) that propagates to `pyproject.toml` generation (`utilities/project_utils.py:599-630`), **typed hooks** with `BeforeToolCallHook`/`AfterToolCallHook` protocols (`hooks/types.py:87-123`) for cross-cutting concerns, **thread-safe usage limiting** with a `threading.Lock` (`base_tool.py:192, 295-312`), **per-invocation MCP client factories** so concurrent calls do not share state (`tools/mcp_native_tool.py:88-119`), and **tool filter contexts** (`mcp/filters.py:17-29`) carrying `(agent, server_name, run_context)` to dynamic filters.

Notable weaknesses: **secrets flow through `os.environ` or via platform integration tokens that the tool never sees**, **no cancellation token is injected** into `_run` (`tools/*.py` search for `cancel`/`CancellationToken` returns nothing), **tools read `PRINTER` and `Telemetry` via module globals** (`tool_usage.py:12, 24`), the **`config={"security_context": ...}`** dict built by `ToolUsage._build_fingerprint_config` (`tool_usage.py:1023-1054`) is *not* forwarded to the tool's `_run` — it is built and dropped (it is only used for telemetry events), and the `BaseAgentTool` pattern injects `agents: list[BaseAgent]` (`tools/agent_tools/base_agent_tools.py:18`) but with no permission/visibility control — every delegate tool can call every listed agent.

## Rating

**5/10**

Rationale:
- **Strengths**
  - Tools can declare typed collaborators as Pydantic fields (`BaseAgentTool.agents`, `RecallMemoryTool.memory`, `ReadFileTool._files`, `CacheTools.cache_handler`) — explicit, IDE-friendly, and serializable.
  - Typed `ToolCallHookContext` (`hooks/tool_hooks.py:24-77`) gives `before_tool_call`/`after_tool_call` hooks a clean, typed DI surface (`tool_name`, `tool_input`, `tool`, `agent`, `task`, `crew`, `tool_result`, `raw_tool_result`).
  - `MCPToolResolver` constructs a fresh `MCPClient` per invocation (`tools/mcp_native_tool.py:88-119`) and ships a typed `ToolFilterContext` (`mcp/filters.py:17-29`) — explicit DI for MCP filtering.
  - `BaseTool.run()` is callable in true isolation (`base_tool.py:314-331`); validation, usage limit, and concurrency (`threading.Lock` at `base_tool.py:192`) are all internal to the tool.
  - `EnvVar` declaration on tools (`base_tool.py:96-100, 145-148`) is project-aware — `pyproject.toml` generation reads it (`utilities/project_utils.py:599-630`).
  - Hooks are typed protocols (`hooks/types.py:87-123`) with `runtime_checkable` registration and a registration API that supports global registration/unregistration (`tool_hooks.py:128-302`).
- **Limitations**
  - **No per-call context object the tool can read**. `BaseTool._run` only sees Pydantic-field-bound collaborators and LLM-supplied args. There is no `ctx: ToolContext` parameter, no `kwargs` of runtime metadata — analogous to agent-framework's `FunctionInvocationContext` is absent.
  - **Runtime context is hoarded on `ToolUsage`**, not shared with the tool: `tools_handler`, `function_calling_llm`, `fingerprint_context`, `agent`, `task`, `action` (`tool_usage.py:89-112`). The tool has no way to read these.
  - **`config={"security_context": ...}`** built at `tool_usage.py:1023-1054` is **not forwarded** to `tool.ainvoke(input, config=...)` for the tool's use — it is only constructed for telemetry events. Wait, that is incorrect — the call site *is* `tool.ainvoke(input=..., config=fingerprint_config)` at `tool_usage.py:340-347, 580-589`, but `CrewStructuredTool.invoke` (`tools/structured_tool.py:340-364`) and `ainvoke` (`tools/structured_tool.py:296-328`) accept `config` and **ignore it** (`config` is a positional argument that is never read). So fingerprint data is dropped on the floor at the tool boundary.
  - **No cancellation token**. A search across `lib/crewai/src/crewai/tools/*.py` returns nothing for `cancel`/`CancellationToken`/`asyncio.CancelledError` (except `mcp_tool_wrapper.py:193` where it is re-raised after a timeout, not propagated to the tool).
  - **Secrets rely on `os.environ` and module-level globals**. `PRINTER` from `crewai_core.printer` (`tool_usage.py:12`) is a global; `Telemetry()` is instantiated fresh on every `ToolUsage.__init__` (`tool_usage.py:99`) but is not passed to the tool. `SecurityConfig.fingerprint` lives on the *agent*, not on the tool (`security/security_config.py:20-58`).
  - **Permissions are not enforced by context**. `BaseAgentTool` accepts `agents: list[BaseAgent]` and lets the delegate tool call any of them (`tools/agent_tools/base_agent_tools.py:18, 80-86`). There is no allowlist inside the tool.
  - **Tools depend on global module state** in subtle ways: `BaseTool._generate_description` reads the JSON schema, `_run_attempts` and `_max_parsing_attempts` are mutating instance state on `ToolUsage` not the tool. `Telemetry` is instantiated at `ToolUsage.__init__` (`tool_usage.py:99`), not at tool construction.
  - **`_logger: Logger` on `CrewStructuredTool`** (`structured_tool.py:141`) is created by `default_factory=Logger` but is **never used inside the class** (`structured_tool.py` search for `self._logger` returns nothing). It is dead DI.
  - **`ToolUsage` requires `MagicMock` everywhere** for unit tests (`tests/tools/test_tool_usage.py:120-126, 147-152, 180-187`) because the constructor takes six positional/keyword collaborators, none of which are typed behind a `ToolContext` seam.
  - **`StructuredTool.invoke` uses `asyncio.run` for sync funcs and runs sync in the executor for async funcs** (`structured_tool.py:322-328, 356-364`), which makes tools that share state across invocations under concurrent `await tool.ainvoke(...)` calls subject to thread-pool scheduling. There is no per-tool concurrency limit.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| `BaseTool` is a Pydantic model that takes collaborators as fields | `BaseTool(BaseModel, ABC)` with `name`, `description`, `env_vars`, `args_schema`, `result_schema`, `cache_function`, `result_as_answer`, `max_usage_count`, `current_usage_count` | `lib/crewai/src/crewai/tools/base_tool.py:103-191` |
| `EnvVar` declarative dependency for tools | `class EnvVar(BaseModel): name: str; description: str; required: bool = True; default: str \| None = None` | `lib/crewai/src/crewai/tools/base_tool.py:96-100` |
| `BaseTool._run` is hermetic — no context object | `def _run(self, *args, **kwargs)` is the abstract sync body; only Pydantic fields + LLM args are reachable | `lib/crewai/src/crewai/tools/base_tool.py:375-391` |
| Tool run seam in isolation | `BaseTool.run` only validates kwargs, claims usage, calls `_run`, optionally `asyncio.run`. No agent/task/llm dependency required | `lib/crewai/src/crewai/tools/base_tool.py:314-331` |
| `BaseAgentTool` injects `agents: list[BaseAgent]` as a Pydantic field | `class BaseAgentTool(BaseTool): agents: list[BaseAgent] = Field(...)` — collab injected via field at construction | `lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:15-18` |
| `ReadFileTool` injects `_files` via a setter | `_files: dict[str, FileInput] \| None = PrivateAttr(default=None)`; `set_files` mutates it post-construction | `lib/crewai/src/crewai/tools/agent_tools/read_file_tool.py:37-45` |
| `Crew._add_file_tools` wires `_files` on `ReadFileTool` | `read_file_tool = ReadFileTool(); read_file_tool.set_files(files); return self._merge_tools(...)` | `lib/crewai/src/crewai/crew.py:1787-1789` |
| `CacheTools` injects `cache_handler` via field with `default_factory` | `cache_handler: CacheHandler = Field(description="Cache Handler for the crew", default_factory=CacheHandler)` | `lib/crewai/src/crewai/tools/cache_tools/cache_tools.py:11-15` |
| `RecallMemoryTool` / `RememberTool` inject `memory: Any` | `memory: Any = Field(exclude=True)` — collaborator bound via field | `lib/crewai/src/crewai/tools/memory_tools.py:31, 81` |
| `create_memory_tools` factory wires `memory` to both tools | Tools created with `memory=memory` constructor arg; read-only memory gets only recall tool | `lib/crewai/src/crewai/tools/memory_tools.py:104-130` |
| `ToolUsage` hoards runtime context on `self` | `tools_handler`, `tools`, `task`, `function_calling_llm`, `agent`, `action`, `fingerprint_context` are ctor fields; `_telemetry` and `_run_attempts` are also stored | `lib/crewai/src/crewai/tools/tool_usage.py:89-112` |
| `_build_fingerprint_config` builds security context but tool ignores `config` | Builds `{"security_context": {"agent_fingerprint": ..., "task_fingerprint": ...}}`; passed as `config=...` to `tool.invoke/ainvoke` | `lib/crewai/src/crewai/tools/tool_usage.py:1023-1054` |
| `tool.invoke(input, config=...)` accepts config but ignores it | `config` parameter is declared but never used inside `invoke` or `ainvoke` | `lib/crewai/src/crewai/tools/structured_tool.py:296-328, 340-364` |
| `ToolCallHookContext` is the *only* typed runtime context object — but only for hooks, not for tools | Carries `tool_name`, `tool_input`, `tool`, `agent`, `task`, `crew`, `tool_result`, `raw_tool_result` | `lib/crewai/src/crewai/hooks/tool_hooks.py:24-77` |
| Tool hooks are global registered callables | `_before_tool_call_hooks`, `_after_tool_call_hooks` are module-level lists; `register_before_tool_call_hook` / `register_after_tool_call_hook` mutate them | `lib/crewai/src/crewai/hooks/tool_hooks.py:124-194` |
| Typed hook protocols (`Hook`, `BeforeToolCallHook`, `AfterToolCallHook`) | `class BeforeToolCallHook(Hook["ToolCallHookContext", bool \| None], Protocol)`; `runtime_checkable` on `Hook` | `lib/crewai/src/crewai/hooks/types.py:17-131` |
| Hooks are run inside the orchestrator, not by the tool | `execute_tool_and_check_finality` walks `get_before_tool_call_hooks()` and `get_after_tool_call_hooks()` and passes `ToolCallHookContext` to each | `lib/crewai/src/crewai/utilities/tool_utils.py:96-141, 216-263` |
| `ToolFilterContext` for MCP dynamic filtering | `class ToolFilterContext(BaseModel): agent: Any; server_name: str; run_context: dict[str, Any] \| None` | `lib/crewai/src/crewai/mcp/filters.py:17-29` |
| `MCPToolResolver._resolve_native` passes `ToolFilterContext(agent=...)` to filters | Builds `ToolFilterContext(agent=self._agent, server_name=server_name, run_context=None)` and passes to `mcp_config.tool_filter(context, tool)` | `lib/crewai/src/crewai/mcp/tool_resolver.py:386-399` |
| `MCPToolWrapper` constructs `MCPServerHTTP` per call (no shared state) | `MCPToolWrapper.__init__` stores `mcp_server_params`; each `_run_async` opens a fresh `streamablehttp_client` | `lib/crewai/src/crewai/tools/mcp_tool_wrapper.py:19-52, 162-203` |
| `MCPNativeTool` builds a fresh `MCPClient` per call | `client_factory` callable invoked inside `_run_async`; try/finally disconnects | `lib/crewai/src/crewai/tools/mcp_native_tool.py:88-119` |
| `MCPToolResolver` builds per-tool `client_factory` closures so parallel calls do not share state | `def _client_factory() -> MCPClient: ...` created inside `_resolve_native` and passed to each `MCPNativeTool` | `lib/crewai/src/crewai/mcp/tool_resolver.py:411-453` |
| `MCPNativeTool._run` preserves `contextvars` when crossing event loops | `ctx = contextvars.copy_context(); with concurrent.futures.ThreadPoolExecutor() as executor: future = executor.submit(ctx.run, asyncio.run, coro)` | `lib/crewai/src/crewai/tools/mcp_native_tool.py:88-92` |
| `BaseTool._claim_usage` uses `threading.Lock` for concurrency safety | `_usage_lock: threading.Lock = PrivateAttr(default_factory=threading.Lock)`; locked check-and-increment | `lib/crewai/src/crewai/tools/base_tool.py:192, 295-312` |
| `_logger` declared on `CrewStructuredTool` is dead DI | `_logger: Logger = PrivateAttr(default_factory=Logger)` declared; never used inside the class | `lib/crewai/src/crewai/tools/structured_tool.py:141` |
| `Telemetry()` instantiated at `ToolUsage.__init__`, not at tool construction | `self._telemetry: Telemetry = Telemetry()` | `lib/crewai/src/crewai/tools/tool_usage.py:99` |
| `PRINTER` imported from `crewai_core.printer` is a module global | `from crewai_core.printer import PRINTER`; `PRINTER.print(content=..., color=...)` | `lib/crewai/src/crewai/tools/tool_usage.py:12` (used at `:138, :150, :166, :186, :198, :215, :403, :417, :442`) |
| `BaseTool.from_langchain` constructs from langchain-style objects | `Tool.from_langchain(tool)` reads `tool.func`, `tool.args_schema`, `tool.result_schema` and rebuilds as CrewAI `Tool` | `lib/crewai/src/crewai/tools/base_tool.py:411-456, 583-638` |
| `BaseAgent` validator coerces langchain tools to `Tool.from_langchain` | `if isinstance(tool, BaseTool): processed_tools.append(tool); elif all(hasattr(tool, attr) for attr in required_attrs): processed_tools.append(Tool.from_langchain(tool))` | `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:511-537` |
| `Agent.get_mcp_tools` wires `MCPToolResolver` with `agent` reference (logger comes from agent) | `self._mcp_resolver = MCPToolResolver(agent=self, logger=self._logger); return self._mcp_resolver.resolve(mcps)` | `lib/crewai/src/crewai/agent/core.py:1188-1197` |
| `AgentTools` factory injects `agents` into `DelegateWorkTool` and `AskQuestionTool` | `delegate_tool = DelegateWorkTool(agents=self.agents, ...); ask_tool = AskQuestionTool(agents=self.agents, ...)` | `lib/crewai/src/crewai/tools/agent_tools/agent_tools.py:22-35` |
| `Agent.get_delegation_tools` returns tools with `agents` populated from crew roster | `agent_tools = AgentTools(agents=agents); return agent_tools.tools()` | `lib/crewai/src/crewai/agent/core.py:1173-1175` |
| `_extract_env_vars` reads `EnvVar` field from `pyproject.toml` tool config | `for env_var in default: if isinstance(env_var, EnvVar): return [{"name": ..., "description": ..., ...}]` | `lib/crewai/src/crewai/utilities/project_utils.py:599-630` |
| `SecurityConfig` is a Pydantic model that lives on agent/task, not on tool | `class SecurityConfig(BaseModel): fingerprint: Fingerprint = Field(default_factory=Fingerprint, ...)` | `lib/crewai/src/crewai/security/security_config.py:20-58` |
| `Fingerprint` UUIDs are auto-generated per agent | `_uuid_str: str = PrivateAttr(default_factory=lambda: str(uuid4()))` | `lib/crewai/src/crewai/security/fingerprint.py:54` |
| `crewai_event_bus.emit` used as global event sink in `ToolUsage` | `crewai_event_bus.emit(self, ToolUsageStartedEvent(**event_data))` | `lib/crewai/src/crewai/tools/tool_usage.py:273, 513, 786, 795, 945, 965, 996` |
| `platform_integration_token` lives in a `ContextVar` with env-var fallback | `_platform_integration_token: contextvars.ContextVar[str \| None] = contextvars.ContextVar(...)`; `os.getenv("CREWAI_PLATFORM_INTEGRATION_TOKEN")` fallback | `lib/crewai/src/crewai/context.py:25-48` |
| `platform_context()` is a `@contextmanager` for safely setting the token | Token-based set/reset via `_platform_integration_token.set(integration_token)` / `.reset(token)` | `lib/crewai/src/crewai/context.py:51-62` |
| `ExecutionContext` Pydantic snapshot of ContextVars | `class ExecutionContext(BaseModel): current_task_id, flow_request_id, flow_id, ...` plus `capture_execution_context` / `apply_execution_context` helpers | `lib/crewai/src/crewai/context.py:85-134` |
| `CrewContext` is a Pydantic model carried in OpenTelemetry baggage | `class CrewContext(BaseModel): id: str \| None; key: str \| None`; `get_crew_context()` reads via `opentelemetry.baggage.get_baggage("crew_context")` | `lib/crewai/src/crewai/utilities/crew/models.py:6-17`, `lib/crewai/src/crewai/utilities/crew/crew_context.py:1-16` |
| `tool.from_langchain(...)` only resolves dotted paths when `CREWAI_DESERIALIZE_CALLBACKS=1` | `_trusted_deserialize()` reads env-var; refuses by default with `set CREWAI_DESERIALIZE_CALLBACKS=1 to allow` | `lib/crewai/src/crewai/types/callback.py:22-94` |
| `CacheHandler` injected via `CacheTools.cache_handler` field with `default_factory` | `cache_handler: CacheHandler = Field(default_factory=CacheHandler)` | `lib/crewai/src/crewai/tools/cache_tools/cache_tools.py:11-15` |
| `ToolsHandler.on_tool_use` writes to cache via injected `CacheHandler` | `if self.cache and should_cache: ... self.cache.add(tool=..., input=..., output=...)` | `lib/crewai/src/crewai/agents/tools_handler.py:26-52` |
| Test: `BaseTool` instantiated standalone, `run()` invoked, no agent | `class MyCustomTool(BaseTool): ...; tool = MyCustomTool(); assert tool.run("What is the meaning of life?") == ...` | `lib/crewai/tests/tools/test_base_tool.py:48-82` |
| Test: schema validation + usage count + sanitization exercised in isolation | `TestBaseToolRunValidation.test_run_with_no_args_raises_validation_error`, `test_run_increments_usage_after_validation`, `test_run_does_not_increment_usage_on_validation_error` | `lib/crewai/tests/tools/test_base_tool.py:265-318` |
| Test: `ReadFileTool` requires `set_files()` to be useful | `setup_method` creates `ReadFileTool()`; `_run` returns `"No input files available."` until `set_files` is called | `lib/crewai/tests/tools/agent_tools/test_read_file_tool.py:18-42` |
| Test: `ToolUsage` requires `MagicMock` for `tools_handler`, `task`, `function_calling_llm`, `agent`, `action` | `tool_usage = ToolUsage(tools_handler=MagicMock(), tools=[tool], task=MagicMock(), function_calling_llm=MagicMock(), agent=MagicMock(), action=MagicMock())` | `lib/crewai/tests/tools/test_tool_usage.py:119-126, 146-152, 180-187` |
| Test: `platform_context` preserves/restores `ContextVar` and tolerates exceptions | `test_platform_context_manager_preserves_existing_token`, `test_platform_context_manager_exception_handling` | `lib/crewai/tests/test_context.py:88-127` |
| `ToolUsage.__init__` parameter count and required collaborators | `tools_handler`, `tools`, `task`, `function_calling_llm`, `agent`, `action`, `fingerprint_context` — six required collaborators before any logic runs | `lib/crewai/src/crewai/tools/tool_usage.py:89-112` |
| `MCPServerConfig` carries `env`, `headers`, `tool_filter` per-server | `env: dict[str, str] \| None = Field(default=None, description="Environment variables to pass to the process.")`; `headers: dict[str, str] \| None = Field(default=None, description="Optional HTTP headers for authentication...")` | `lib/crewai/src/crewai/mcp/config.py:39-46, 71-78, 110-116` |
| `MCPToolResolver._resolve_external` injects `mcp_server_params` per-tool | `wrapper = MCPToolWrapper(mcp_server_params=server_params, tool_name=tool_name, tool_schema=schema, server_name=server_name)` | `lib/crewai/src/crewai/mcp/tool_resolver.py:249-256` |

## Answers to Dimension Questions

### 1. What context does a tool receive?

A tool receives **only** what is bound to it as a Pydantic field on its `BaseTool` subclass (or wrapped `CrewStructuredTool`), plus the LLM-supplied arguments passed to `_run`. There is no per-call runtime context object. Collaborators that are *not* Pydantic fields can be smuggled in via a post-construction setter (e.g. `ReadFileTool.set_files`) or by holding the value on a private attribute (`BaseAgentTool.agents`).

The runtime metadata that the executor knows about — `agent`, `task`, `tools_handler`, `function_calling_llm`, `fingerprint_context`, `action`, `last_used_tool`, `cache` — is stored on `ToolUsage` (`lib/crewai/src/crewai/tools/tool_usage.py:99-112`), not exposed to the tool.

### 2. Is context explicit or global?

**Mostly global** for the tool's perspective. Tools read `PRINTER` from `crewai_core.printer` (`lib/crewai/src/crewai/tools/tool_usage.py:12`) and `Telemetry` is instantiated fresh per `ToolUsage.__init__` (`lib/crewai/src/crewai/tools/tool_usage.py:99`), not injected. `crewai_event_bus` is a module-level singleton (`lib/crewai/src/crewai/events/event_bus.py`). `Logger` is created with `default_factory=Logger` on `CrewStructuredTool` (`lib/crewai/src/crewai/tools/structured_tool.py:141`) but never used.

**Explicit where it matters**: collaborators that vary per-deployment (agents, memory, cache_handler, files) are wired via Pydantic fields on the tool subclass (`lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:18`, `lib/crewai/src/crewai/tools/memory_tools.py:31`, `lib/crewai/src/crewai/tools/cache_tools/cache_tools.py:12`).

### 3. Are secrets passed safely?

Secrets are **not** passed to the tool at all. The fingerprint data the executor builds (`security_context` with agent/task fingerprints, `lib/crewai/src/crewai/tools/tool_usage.py:1023-1054`) is constructed and passed as `config=...` to `tool.invoke` / `tool.ainvoke`, but `CrewStructuredTool.invoke`/`ainvoke` (`lib/crewai/src/crewai/tools/structured_tool.py:296-364`) accept `config` and **never read it** — the parameter is silently dropped. So even the framework's own security metadata never reaches the tool body.

The platform integration token lives in a `ContextVar` with an `os.environ` fallback (`lib/crewai/src/crewai/context.py:25-48`) and is read on demand by the executor / resolver. MCP server credentials travel in `MCPServerHTTP.headers` and `MCPServerStdio.env` (`lib/crewai/src/crewai/mcp/config.py:39-46, 71-78`) — they are wired into the transport at discovery time, not into the tool.

`Tool.from_langchain` refuses to deserialize dotted-path callbacks unless `CREWAI_DESERIALIZE_CALLBACKS=1` (`lib/crewai/src/crewai/types/callback.py:88-93`) — opt-in for unsafe round-trips. This is the closest thing to a secret-safety guard in the tool DI layer.

### 4. Can tools be unit tested?

**Yes for hermetic tools** (no collaborators). `BaseTool.run()` works without an agent or crew (`lib/crewai/src/crewai/tools/base_tool.py:314-331`), as exercised in `lib/crewai/tests/tools/test_base_tool.py:14-82, 154-184, 250-318, 626-697`.

**Partially for tools with collaborators** — `MagicMock` injection is the standard pattern (`lib/crewai/tests/tools/test_tool_usage.py:119-126, 146-152, 180-187`). Tools like `BaseAgentTool` accept `agents: list[BaseAgent]` as a field, so the test must instantiate real agents or mocks (`lib/crewai/tests/tools/agent_tools/test_agent_tools.py:8-16` uses a real `Agent` plus VCR cassettes).

**No for tool-resolver integration**: `MCPToolResolver` is wired against a real `Agent` (`lib/crewai/src/crewai/agent/core.py:1188-1197`); MCP unit tests are limited to discovery (`lib/crewai/tests/mcp/test_tool_resolver_native.py`) and transport-level (`test_stdio_transport.py`, `test_sse_transport.py`).

### 5. Can context enforce permissions?

**Weakly**. Hooks can block a tool call before execution (`hooks/types.py:87-104`, `hooks/tool_hooks.py:128-162`) and `ToolCallHookContext.tool_input` is mutable so they can sanitize input. But once a tool is in flight, no permission system checks which `agents` it can delegate to (`lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:18, 80-86` lets any delegate tool call any agent in `agents`). The MCP `ToolFilterContext` carries an `agent` reference but no `permissions` field (`lib/crewai/src/crewai/mcp/filters.py:17-29`); dynamic filters must encode their own policy. Static filters are name-allowlist/blocklist only (`mcp/filters.py:38-88`). The fingerprint data is built but never enforced.

## Architectural Decisions

- **Pydantic-field injection over a context object**. Collaborators are declared as `Field(...)` on the tool subclass. This makes serialization (`base_tool.py:51-78`) and checkpointing (`base_tool.py:51-78`, `types/callback.py:153-158`) trivial at the cost of a fragile DI story: tests must build real collaborators or `MagicMock`.
- **Tool execution is brokered by `ToolUsage`**, not the tool itself. `ToolUsage.use()` (`tool_usage.py:132-220`) and `_use()` (`tool_usage.py:469-707`) own parsing, validation, cache lookup, retry, telemetry, hooks, and result formatting. The tool is a callable at the end of a pipeline.
- **Per-call MCP client factory** (`mcp/tool_resolver.py:411-453`, `tools/mcp_native_tool.py:88-119`) gives concurrent calls independent transports. This is the one place CrewAI explicitly solves "fresh state per call" for tools.
- **Hooks as global registered callables** rather than per-tool. `register_before_tool_call_hook` / `register_after_tool_call_hook` (`hooks/tool_hooks.py:128-194`) push to module-level lists. No DI surface on individual tools.
- **Tool usage limit enforced with `threading.Lock`** on the tool instance (`base_tool.py:192, 295-312`) — the one place a per-tool mutex lives.
- **`CrewStructuredTool.invoke`/`ainvoke` accept a `config` dict and drop it on the floor** (`structured_tool.py:296-328, 340-364`) — the seam exists but is unused.

## Notable Patterns

- **Pydantic-as-DI** — every collaborator that varies per-deployment is a `Field(...)` on the tool subclass.
- **Module-singleton event bus** — `crewai_event_bus.emit(self, Event(...))` (`tool_usage.py:273, 513, 786, 795, 945, 965, 996`) is the cross-cutting observability channel; tools themselves do not emit events.
- **Setter-based late binding** — `ReadFileTool.set_files(...)` (`tools/agent_tools/read_file_tool.py:39-45`) lets the crew wire runtime data after construction.
- **Tool-typed-callable serialization** — `SerializableCallable` (`types/callback.py:153-158`) is a Pydantic `Annotated` type that round-trips dotted paths, with an explicit `CREWAI_DESERIALIZE_CALLBACKS=1` opt-in for the unsafe direction.
- **Per-server MCP credentials** travel as `MCPServerHTTP.headers` / `MCPServerStdio.env` (`mcp/config.py:39-46, 71-78`); they are server-level, not per-tool.
- **ThreadPoolExecutor + contextvars copy** to bridge sync MCP tools from inside a running event loop (`tools/mcp_native_tool.py:88-92`).
- **Hook context** (`ToolCallHookContext`) is a clean, well-typed value object — but only available to hook callables, never to the tool body.

## Tradeoffs

- **Pydantic-field injection** gives serialization and IDE help but forces every collaborator into the tool's constructor. Tests that want a "context lite" must use `MagicMock`, which the test file does (`tests/tools/test_tool_usage.py:120-126`).
- **No per-call context object** keeps the tool signature clean (`_run(self, *args, **kwargs)`) but means runtime metadata (`agent`, `task`, `tools_handler`, `fingerprint_context`) is unreachable from `_run`. Compare with `FunctionInvocationContext` in agent-framework (which carries `session`, `metadata`, `kwargs`, `tools`, `result`).
- **Module-singleton `PRINTER` and `crewai_event_bus`** simplify code paths but make per-tool log routing impossible without monkey-patching.
- **`config={"security_context": ...}` passed through to `tool.invoke(input, config=...)`** is a hint of a context seam, but `CrewStructuredTool.invoke`/`ainvoke` accept and ignore `config` — the seam is dead code.
- **MCP `client_factory` per-invocation** ensures thread-safety for concurrent calls (`mcp_native_tool.py:88-119`) at the cost of opening a fresh MCP session per call (no connection reuse).
- **Tool usage limit** is enforced with `threading.Lock` (`base_tool.py:192`), but the lock is per-tool-instance; an agent that owns many tools cannot coordinate a single budget across them.

## Failure Modes / Edge Cases

- **Dead `config` parameter**: `CrewStructuredTool.invoke(input=..., config=...)` (`structured_tool.py:340-364`) and `ainvoke(input=..., config=...)` (`structured_tool.py:296-328`) accept `config` and never read it. Any fingerprint/permission data the executor tries to forward is silently dropped.
- **`_logger` declared, never used**: `_logger: Logger = PrivateAttr(default_factory=Logger)` on `CrewStructuredTool` (`structured_tool.py:141`) is dead DI. If a tool wanted per-instance logging, the field exists but the orchestration does not consult it.
- **`Telemetry()` instantiated at `ToolUsage.__init__`** (`tool_usage.py:99`) — every tool execution creates a new `Telemetry`; no shared state. Telemetry events are emitted via `crewai_event_bus`, which is global.
- **Concurrent `BaseTool.run` with `asyncio.iscoroutine(result)` branch** (`base_tool.py:328-329`) calls `asyncio.run(result)` from within an already-running event loop and raises `RuntimeError`. `Tool.run` (decorator path, `base_tool.py:526-528`) has the same problem.
- **MCP cancel-scope errors** if `_run` is called inside a `contextvars.copy_context()` + `ThreadPoolExecutor` path (`mcp_native_tool.py:88-92`); this is a workaround, not a clean cancellation primitive.
- **Tool memory fields are typed `Any`** (`memory_tools.py:31, 81`) — no static guarantee that the memory object actually has `recall` / `remember`. A wrong-shaped collaborator fails at call time, not at construction.
- **`platform_integration_token` fallback to `os.environ`** (`context.py:39-48`) silently picks up a token if the env var is set; tests must `os.environ.clear()` to isolate, and `lib/crewai/tests/test_context.py:22-43, 195-200` proves the team has had to mock `os.getenv` for that reason.
- **`ToolUsage._run_attempts` and `_max_parsing_attempts` mutate on `self`** (`tool_usage.py:100-102, 425, 665, 872`); a `ToolUsage` instance is not reusable across concurrent calls — call sites create a fresh `ToolUsage` per tool execution (`utilities/tool_utils.py:78-86, 198-206`).

## Future Considerations

- **Promote `_logger` to a real DI seam** on `BaseTool`/`CrewStructuredTool`. The field exists; the wiring does not. A per-tool logger would enable request-scoped logs.
- **Replace `MagicMock` injection with a `ToolContext` value object** that carries `agent`, `task`, `tools_handler`, `fingerprint_context`. The hook system already models the shape (`ToolCallHookContext`). A read-only `ToolContext` accessible via a `ContextVar` (mirroring `platform_integration_token` at `context.py:25-48`) would let tools query collaborators without smuggling them through Pydantic fields.
- **Actually use the `config` parameter** on `CrewStructuredTool.invoke`/`ainvoke` (`structured_tool.py:296-364`) and pass it to `_run` as a keyword. The infrastructure is there; the wiring is missing.
- **Inject a cancellation token** into `_run` (or a `ContextVar` for cooperative cancel). MCP has timeouts (`tools/mcp_tool_wrapper.py:10-13, 156-160`) but the timeout lives on the wrapper, not the user-defined tool.
- **Permissions layer on `BaseAgentTool`** — the `agents: list[BaseAgent]` field (`tools/agent_tools/base_agent_tools.py:18`) has no allowlist. A `permissions: set[str]` field plus a static check would make delegation enforceable.
- **Move `Telemetry()` off the executor** into a DI field on `ToolUsage` so tests can pass a recording double without `unittest.mock.patch`.
- **Stop using `asyncio.run` inside sync paths** (`base_tool.py:328-329, 526-528`, `structured_tool.py:357, 362`) — this fails silently inside running loops. `Tool.run` is meant for sync use; the async path should be `arun`.

## Questions / Gaps

- Where, if anywhere, is `config={"security_context": ...}` supposed to land inside the tool? `CrewStructuredTool.invoke`/`ainvoke` accept it and ignore it. Is there a documented contract for tools to read it?
- What is the canonical way to inject an `Agent` (vs. `agents` list) into a single-target tool? `BaseAgentTool` accepts a list; single-target callers instantiate one-element lists — no ergonomic alternative.
- Are hooks (`ToolCallHookContext`) the only path for cross-tool context, or is there a hidden DI seam for tool authors? The search for `ToolContext` / `ToolRuntime` / `tool_context` returns nothing in `lib/crewai/src/crewai/tools/`.
- Why is `SecurityConfig.fingerprint` (`security/security_config.py:39-41`) reachable on the agent but not on the tool? A `Tool.security_config` would let user tools gate on fingerprint without monkey-patching the executor.
- Where is the cancellation primitive for user tools? `asyncio.CancelledError` is only seen in `mcp_tool_wrapper.py:193` as a fallback after timeout. There is no first-class injection.
- Does the executor document that `CrewStructuredTool._logger` is intentional placeholder DI? `structured_tool.py:141` declares it but no code uses it.

---

Generated by `studies/agent-harness-study/reports/source/04.04-tool-context-dependency-injection/dimension-04.04-tool-context-and-dependency-injection` against `crewai`.

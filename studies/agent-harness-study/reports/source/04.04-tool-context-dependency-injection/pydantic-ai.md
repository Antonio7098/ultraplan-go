# Source Analysis: pydantic-ai

## 04.04 Tool Context and Dependency Injection

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python 3.10+ (dataclasses, generics, ContextVar, async/await), Pydantic-core validation |
| Analyzed | 2026-07-27 |

## Summary

Pydantic AI uses an explicit, type-parameterized `RunContext[AgentDepsT]` dataclass as the single per-run dependency container passed to every tool that opts in by typing its first parameter as `RunContext[...]`. Dependencies (`deps`) are user-defined dataclasses supplied per call (`agent.run(..., deps=...)`), constructed into `RunContext` inside the agent graph via `build_run_context` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1396`), and made available to toolsets, hooks, validators, and capabilities through the same context object. The system is consistent across toolsets, model layers, output validators, deferred execution (Temporal/Prefect), MCP, and override contexts. There is **no global state** for tools; the only contextual escape hatch is the per-agent `ContextVar` overrides (`_override_deps`, `_override_model`, etc. in `pydantic_ai_slim/pydantic_ai/agent/__init__.py:545-554`) that are explicitly set by the `agent.override()` context manager (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:1780`). Tools are fully unit-testable: the typed `RunContext` is the only contract a tool sees, and `TestModel` / `FunctionModel` in `pydantic_ai_slim/pydantic_ai/models/test.py` and `pydantic_ai_slim/pydantic_ai/models/function.py` exercise tools without booting real providers.

## Rating

**8 / 10**

Rationale:
- **+** Single, fully typed `RunContext[AgentDepsT]` dataclass carrying user deps, model, usage, agent, messages, validation context, tracer, retries, run_id, conversation_id, capabilities, tool manager, enqueueable pending messages (`pydantic_ai_slim/pydantic_ai/_run_context.py:36-127`). Tested and documented in `docs/dependencies.md:1-310`.
- **+** Deps are user-defined dataclasses (not framework-owned), tested via `test_deps.py:1-134`, including a nested `agent.override(deps=...)` test.
- **+** Tools can be unit-tested without booting the app: `RunContext` is constructible directly (`tests/test_capabilities.py:4093-4094`: `RunContext(deps=deps, model=TestModel(), usage=RunUsage(), run_step=0)`), and toolsets/tools are invoked via the public `TestModel`/`FunctionModel` paths (e.g. `tests/test_tools.py:144`, `tests/test_capabilities.py:4137`).
- **+** Context differs appropriately across surfaces: tool body, `prepare`/`args_validator`/`ToolPrepareFunc`, `for_run` capability hook, `before_run`/`after_run` hooks, `RunContext.tools` (`_run_context.py:220-225`), `available_tool_names` (`_run_context.py:181-218`), `pending_messages` queue (`_run_context.py:106-113`), output validators, deferred execution (`TemporalRunContext` at `pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_run_context.py:17`).
- **+** Override is contextual (`ContextVar`) and stackable (nested `with agent.override(...)`); proven by `tests/test_deps.py:80-93`.
- **−** Permission enforcement is per-tool opt-in (`requires_approval=True` on `Tool`, `pydantic_ai_slim/pydantic_ai/tools.py:455`, `576`) plus the `ApprovalRequiredToolset` wrapper (`pydantic_ai_slim/pydantic_ai/toolsets/approval_required.py:16-32`); there is **no first-class ACL/role model** — a malicious or sloppy tool gets full access to whatever the caller stuffed into `deps`.
- **−** Secrets rely on the caller to pass them via `deps` (e.g. `MyDeps.api_key` in `docs/dependencies.md:64-82`); there is no framework-managed secret store, scoping, or scrubbing in logs/traces. `trace_include_content` (`_run_context.py:56`) is the only toggle.
- **−** `RunContext` is a `kw_only=True` dataclass with ~20 fields (lines 36-151); the surface is broad but not always populated (e.g. `model_settings` is `None` in tool hooks per the docstring at line 98-105). Future-readers need to consult the field docstrings to learn what is/isn't available at each hook point.
- **−** Testability is excellent in-process, but durable-execution toolsets must reimplement serialization (`TemporalRunContext.serialize_run_context` at `durable_exec/temporal/_run_context.py:47-66`), and a tool needing `tool_manager` (e.g. a sandbox dispatcher) cannot run in Temporal because the field is intentionally dropped — see the warning at `_run_context.py:122-124`.
- **−** Minor: a few fields have fragile invariants (e.g. `loaded_capability_ids` and `discovered_tool_names` are shared by reference across `replace(...)` copies, see `_agent_graph.py:1423-1427`); this is documented but easy to misuse.

Score placement: solid 8 — explicit, typed, tested, with a documented override mechanism and observable model, but lacks a built-in permission/secret boundary, and the context object is dense enough to invite confusion about which fields are populated in which hook.

## Evidence Collected

Every entry includes file path with line numbers. Format: `path/to/file.py:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Tool context class | `RunContext` generic dataclass: `RunContext[RunContextAgentDepsT]` with deps, model, usage, agent, prompt, messages, validation_context, tracer, retries, tool_call_id, tool_name, retry, max_retries, run_step, run_id, conversation_id, metadata, tool_manager, capabilities, loaded_capability_ids, discovered_tool_names, enqueue, last_attempt, available_capability_ids, available_tool_names, tools | `pydantic_ai_slim/pydantic_ai/_run_context.py:36-225` |
| Tool context type alias | `AgentDepsT` / `RunContextAgentDepsT` typevars used throughout | `pydantic_ai_slim/pydantic_ai/_run_context.py:29-33` |
| Tool function signatures | `ToolFuncContext: Callable[Concatenate[RunContext[AgentDepsT], ToolParams], Any]`, `ToolFuncPlain: Callable[ToolParams, Any]`, `ToolFuncEither = ToolFuncContext \| ToolFuncPlain`, `ArgsValidatorFunc` (also takes `RunContext`), `ToolPrepareFunc`, `ToolSelectorFunc`, `NativeToolFunc` — all take `RunContext` | `pydantic_ai_slim/pydantic_ai/tools.py:66-167`, `pydantic_ai_slim/pydantic_ai/tools.py:231-244` |
| Tool registration carries deps type | `Tool.__init__` builds `FunctionSchema` (and infers `takes_ctx`); `from_schema` accepts `takes_ctx` to wire context-passing | `pydantic_ai_slim/pydantic_ai/tools.py:467-567`, `pydantic_ai_slim/pydantic_ai/tools.py:582-636` |
| Tool function-schema build | `function_schema(...)` inspects the first parameter to detect `RunContext[...]`; emits explicit errors when context annotation is misused (right position, first arg only) | `pydantic_ai_slim/pydantic_ai/_function_schema.py:103-174` |
| Tool invocation calls into user code with RunContext | `FunctionSchema.call(self, args_dict, ctx)` injects `ctx` as first positional arg only when `takes_ctx`; sync functions run in `run_in_executor`, async awaited directly | `pydantic_ai_slim/pydantic_ai/_function_schema.py:80-100` |
| Function toolset stores callable with context | `FunctionToolsetTool.call_func: Callable[[dict[str, Any], RunContext[AgentDepsT]], Awaitable[Any]]`; `call_tool(name, tool_args, ctx, tool)` is the per-toolset dispatch entry point | `pydantic_ai_slim/pydantic_ai/toolsets/function.py:31-42`, `pydantic_ai_slim/pydantic_ai/toolsets/function.py:644-658` |
| Abstract toolset contract | `AbstractToolset.get_tools(ctx: RunContext[AgentDepsT])` and `call_tool(name, tool_args, ctx, tool)`; per-run/per-step hooks `for_run`/`for_run_step`; lifecycle `__aenter__/__aexit__`; `get_instructions(ctx)` | `pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:74-180` |
| Wrapped toolsets forward context | `WrapperToolset.call_tool` simply forwards `ctx` to the wrapped toolset (used by approval, prefix, rename, deferred loading, capability loader, search) | `pydantic_ai_slim/pydantic_ai/toolsets/wrapper.py:63-66` |
| Agent graph builds the RunContext | `build_run_context(ctx)` constructs a `RunContext[DepsT]` from `GraphAgentDeps`/state: `user_deps`, `agent`, `model`, `usage`, `prompt`, `messages`, `validation_context`, `tracer`, `instrumentation_version`, `run_step`, `run_id`, `conversation_id`, `metadata`, `tool_manager`, `capabilities`, `loaded_capability_ids`, `discovered_tool_names`, `pending_messages`. Refreshes capability/tool state across steps. | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1396-1428`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1431-1467` |
| Per-step context variant for tool calls | `_handle_tool_calls` replaces `retry`/`max_retries` onto the context for the tool invocation | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1260-1288` |
| Validation context resolution | `build_validation_context` resolves a callable `validation_context` against the `RunContext` (the framework-provided pydantic ValidationContext used by tool argument and output validators) | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1459-1468` |
| Tool manager exposes ctx to nested calls | `ToolManager._raw_execute` passes `validated.ctx` (already a `RunContext[AgentDepsT]`) into `toolset.call_tool(...)` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:740-769` |
| Pending message injection (tool → agent) | `RunContext.enqueue(...)` adds to `pending_messages`; raises `UserError` if there's no live queue (synthetic contexts) | `pydantic_ai_slim/pydantic_ai/_run_context.py:227-271` |
| Per-agent ContextVar overrides | `_override_deps`, `_override_model`, `_override_toolsets`, `_override_tools`, `_override_builtin_tools`, `_override_name`, `_override_instructions`, `_override_metadata`, `_override_model_settings`, `_override_output_retries` | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:544-554` |
| Override() context manager | `Agent.override(...)` sets `ContextVar` tokens for any subset of (name, deps, model, toolsets, tools, native_tools, instructions, metadata, model_settings, retries, spec) and resets on exit. Nested overrides are supported via the ContextVar token stack. | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1780-1944` |
| Deps resolution at run time | `_get_deps` returns the override if present, otherwise the per-call `deps` argument | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:2536-2546` |
| Wrapper agent override forwards deps | `WrapperAgent.override(...)` forwards `deps=` to the wrapped agent | `pydantic_ai_slim/pydantic_ai/agent/wrapper.py:299-355` |
| Capabilities also get the context | `AbstractCapability.for_run(ctx: RunContext[AgentDepsT])`, `before_run(ctx)`, `after_run(ctx)`, `before_model_request(ctx)`, `after_model_request(ctx)`, `wrap_model_request(ctx)`, all 20+ capability hooks receive `RunContext` | `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:279-284`, `pydantic_ai_slim/pydantic_ai/capabilities/wrapper.py:96-425` |
| Capability creates per-run instance from ctx | `for_run(ctx)` default returns `self`; can return a fresh instance with `ctx`-derived state | `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:279-284` |
| Permission / approval gate (tool call) | `ApprovalRequiredToolset.call_tool` raises `ApprovalRequired` when `ctx.tool_call_approved` is `False` and the per-tool predicate returns `True`. Tool-level `requires_approval=True` sets `kind='unapproved'` on the tool definition | `pydantic_ai_slim/pydantic_ai/toolsets/approval_required.py:16-32`, `pydantic_ai_slim/pydantic_ai/tools.py:649` |
| Deferred tools run cross-boundary | `TemporalRunContext` serializes a subset of `RunContext` (`run_id`, `metadata`, `retries`, `tool_call_id`, `tool_name`, `tool_call_approved`, `tool_call_metadata`, `retry`, `max_retries`, `run_step`, `usage`, `loaded_capability_ids`, `discovered_tool_names`, `capability_loaded`) — `tool_manager`, `capabilities`, `messages`, `tracer`, `agent` intentionally excluded | `pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_run_context.py:17-91` |
| History processor takes context | `_HistoryProcessorSyncWithCtx` / `_HistoryProcessorAsyncWithCtx` accept `RunContext[DepsT]` | `pydantic_ai_slim/pydantic_ai/_history_processor.py:13-15` |
| System prompt runs with context | `SystemPromptRunner.run(run_context)` calls user function with `RunContext[AgentDepsT]` | `pydantic_ai_slim/pydantic_ai/_system_prompt.py:25-42` |
| Common / capability-driven tools | `ImageGenerationTool.__call__(ctx, prompt)`, `XSearchTool.__call__(ctx, query)` — take `RunContext[Any]` | `pydantic_ai_slim/pydantic_ai/common_tools/image_generation.py:78`, `pydantic_ai_slim/pydantic_ai/common_tools/x_search.py:53` |
| Native tool factory takes context | `WebFetchCapability`, `WebSearchCapability`, `XSearchCapability` accept `Callable[[RunContext[AgentDepsT]], ...]` factories | `pydantic_ai_slim/pydantic_ai/capabilities/web_search.py:48`, `pydantic_ai_slim/pydantic_ai/capabilities/web_fetch.py:51`, `pydantic_ai_slim/pydantic_ai/capabilities/x_search.py:80` |
| AgentInfo (model-only context for test fakes) | `FunctionModel` second arg exposes `function_tools`, `output_tools`, `model_request_parameters`, `model_settings`, `instructions`, `allow_text_output` — read-only model-side context (does not carry `deps`) | `pydantic_ai_slim/pydantic_ai/models/function.py:225-247` |
| Testing: dependency override behavior | `tests/test_deps.py` — module-level `Agent(TestModel(), deps_type=MyDeps)`, `@agent.tool` decorator produces a tool that receives `RunContext[MyDeps]`; nested `agent.override(deps=...)` is verified to nest and unwind correctly | `tests/test_deps.py:1-93` |
| Testing: tool-with-context via TestModel | `tests/test_tools.py` registers tools and exercises them via `FunctionModel` / `TestModel` without any model network IO | `tests/test_tools.py:130-176`, `tests/test_tools.py:963-1014` |
| Testing: constructing a RunContext by hand | `tests/test_capabilities.py` builds `RunContext(deps=..., model=TestModel(), usage=RunUsage(), run_step=0)` and exercises capability hooks against it | `tests/test_capabilities.py:4093-4138`, `tests/test_capabilities.py:14269` |
| Typed supertype test for deps | `tests/typed_deps.py` verifies that tools typed with `RunContext[DepsA]`/`RunContext[DepsB]` compile against `Agent(deps_type=AgentDeps)` (multiple inheritance of deps type) | `tests/typed_deps.py:1-86` |
| Public docs: deps and override pattern | `docs/dependencies.md` walks dataclass deps, `RunContext[MyDeps]` typing, async/sync, override for tests with `TestMyDeps` subclass | `docs/dependencies.md:1-310` |

## Answers to Dimension Questions

1. **What context does a tool receive?**
   A single `RunContext[AgentDepsT]` dataclass carrying: the user-supplied deps, model, usage, agent, prompt, messages, validation_context, OpenTelemetry tracer, instrumentation version, retries per tool, retry/max_retries, run_step, run_id, conversation_id, metadata, tool_manager (current run step's tool registry), capabilities registry, loaded_capability_ids, discovered_tool_names, pending_messages queue, tool_call_id, tool_name, tool_call_approved, tool_call_metadata, partial_output. Computed properties `tools`, `available_tool_names`, `available_capability_ids`, `last_attempt`, and the `enqueue(...)` method are also exposed. (`pydantic_ai_slim/pydantic_ai/_run_context.py:36-271`)

2. **Is context explicit or global?**
   Explicit. Tools opt in by typing the first parameter as `RunContext[...]`; non-context tools (`tool_plain`) deliberately exclude it. The framework detects context by inspecting the first parameter's annotation when building the `FunctionSchema` (`pydantic_ai_slim/pydantic_ai/_function_schema.py:149-172`). The only global channel is the per-agent `ContextVar` override (`_override_deps`, `agent/__init__.py:545`) and the per-agent `ContextVar` for capturing run messages (`_messages_ctx_var`, `_agent_graph.py:2061`). There is no module-level global; tools receive the same `RunContext` they would in production.

3. **Are secrets passed safely?**
   Secrets are passed through the user's `deps` object (e.g. `MyDeps.api_key`, `docs/dependencies.md:64-82`). They are not framework-managed; the framework does not add scope, encryption, scrubbing, or rotation. The closest lever is `RunContext.trace_include_content: bool = False` (`_run_context.py:56`) which controls whether message bodies reach the tracer; users can opt out. Documented practice is to put secrets in `deps` and pass them at `agent.run(..., deps=...)` per call.

4. **Can tools be unit tested?**
   Yes. `RunContext` is constructible directly (`tests/test_capabilities.py:4093-4094`). `TestModel` and `FunctionModel` exercise tools end-to-end without providers (`pydantic_ai_slim/pydantic_ai/models/test.py:77-186`, `pydantic_ai_slim/pydantic_ai/models/function.py:225-247`). `agent.override(deps=...)` makes swapping deps for a test subclass trivial (`docs/dependencies.md:283-302`). Existing tests prove this: `tests/test_deps.py:34-93`, `tests/test_tools.py:142-176`, `tests/test_capabilities.py:4093-4138`. Tools run on `anyio` and so are testable under both `pytest-asyncio` and `pytest-anyio`.

5. **Can context enforce permissions?**
   Partially. Permission is opt-in per tool via `Tool(..., requires_approval=True)` (`pydantic_ai_slim/pydantic_ai/tools.py:482`, `576`) and per-toolset via `ApprovalRequiredToolset` (`pydantic_ai_slim/pydantic_ai/toolsets/approval_required.py:16-32`), which gates `call_tool` on `ctx.tool_call_approved`. There is **no built-in ACL, role check, or scope**: the enforcement is purely "does this tool call require approval?". Callers (capability hooks, native tool selectors) can hide tools via `available_tool_names` (`_run_context.py:181-218`) and the `ToolSelector` mechanism (`pydantic_ai_slim/pydantic_ai/tools.py:156-228`), which is the closest thing to a permission surface.

## Architectural Decisions

- **Single generic context object.** A tool never sees more than `RunContext[AgentDepsT]`; everything (model, deps, usage, retries, capabilities, traces, queues) is funnelled through one dataclass. This makes the surface easy to mock and to reason about, at the cost of a wide dataclass.
- **Deps as user dataclass, not framework class.** The framework declares the *type parameter* (`deps_type=MyDeps`) for static typing but never instantiates user deps (`agent/__init__.py` only references `_deps_type` for typing). User control over the shape keeps secrets/services/IO clients in user space.
- **`takes_ctx` is inferred, not declared.** `FunctionSchema.function_schema(...)` detects a `RunContext[...]` first-parameter annotation (`_function_schema.py:149-172`) and emits precise errors if it is misused (wrong position, missing annotation). The `Tool` constructor and `FunctionToolset.tool` decorator accept an explicit override only via the now-deprecated `tool_plain()` path.
- **ContextVar-based overrides instead of threadlocals / monkey-patching.** `agent.override(...)` uses standard library `ContextVar.set/reset` (`agent/__init__.py:1872-1944`). Override scope is lexical (with-block) and stackable, and is exposed via `WrapperAgent.override(...)` (`agent/wrapper.py:299-355`).
- **Tool composition via toolsets, not inheritance.** `AbstractToolset` (`toolsets/abstract.py:74-180`) is the unit of composition with explicit `for_run` / `for_run_step` lifecycle hooks. Cross-cutting behavior (approval, prefixing, renaming, filtering, deferred loading, metadata injection) is implemented as `WrapperToolset` subclasses (per the AGENTS.md rule cited in `pydantic_ai_slim/pydantic_ai/AGENTS.md`, "Extend `WrapperToolset` for cross-cutting toolset behavior"), keeping core classes stable.
- **Durable execution is a typed serialization boundary.** `TemporalRunContext` (`durable_exec/temporal/_run_context.py:17-91`) makes the serialization contract explicit, dropping non-serializable fields (`capabilities`, `tool_manager`, `tracer`, `messages`, `agent`). The same pattern is used for Prefect and DBOS.
- **Capability-first extension model.** Cross-cutting behavior (instructions, tools, hooks, model settings, event processing) goes into a `Capability`/`AbstractCapability` rather than new `Agent` kwargs (per `pydantic_ai_slim/pydantic_ai/capabilities/AGENTS.md`). Capabilities receive `RunContext` at every hook.
- **Tool-call identity flows in the context.** `tool_call_id`, `tool_name`, `tool_call_approved`, `tool_call_metadata`, `partial_output` are all fields of `RunContext` (`_run_context.py:62-85`), so a tool can read its own call identity without separate parameters — this is how `ApprovalRequiredToolset` and deferred-execution flows read approval state.

## Notable Patterns

- **Concatenate[RunContext, …] for context-taking callables.** Used in `ToolFuncContext` and `ArgsValidatorFunc` (`pydantic_ai_slim/pydantic_ai/tools.py:66-94`) to keep the rest of the user signature inspectable.
- **Function-schema-based injection.** `FunctionSchema.call(args_dict, ctx)` is the only place that injects `ctx` (`_function_schema.py:80-100`); all other layers pass `ctx` around but the user-facing decision is encoded at schema build time.
- **Capability `for_run(ctx)` for per-run state isolation.** Default returns `self`, but subclasses can return a fresh instance configured from `ctx.deps` (`capabilities/abstract.py:279-284`).
- **Mutating shared sets across `replace(...)` clones.** `_loaded_capability_ids` and `_discovered_tool_names` are intentionally shared by reference between `RunContext` copies; mutating them in `_refresh_*` (`_agent_graph.py:1446-1447`) keeps all copies in sync without forking. Documented invariant in `_agent_graph.py:1423-1427`.
- **`pending_messages` queue for tools to inject mid-run.** `RunContext.enqueue(*content, priority=...)` (`_run_context.py:227-271`) is safe from any thread because the drain happens between graph nodes.
- **Wrapper-toolsets for cross-cutting behavior.** `ApprovalRequiredToolset`, `PrefixedToolset`, `RenamedToolset`, `FilteredToolset`, `PreparedToolset`, `DeferredLoadingToolset`, `SetMetadataToolset`, `IncludeReturnSchemasToolset` — each is a `WrapperToolset` forwarding `ctx` unchanged.
- **`ToolPrepareFunc`/`ToolsPrepareFunc` for context-aware schema mutation.** Tools/agents can rewrite their JSON schema before the model sees it, gated on `RunContext.deps` (`tools.py:95-154`).
- **`ToolSelector`:** literal `'all'`, sequence of names, metadata dict, or a `Callable[[RunContext, ToolDefinition], bool]` — composable filter used by capabilities and wrapper toolsets (`tools.py:156-228`).

## Tradeoffs

- **Wide dataclass vs. focused DI.** Putting `usage`, `tracer`, `messages`, `tool_manager`, `capabilities`, etc. into one context is convenient for tool authors, but produces a context object whose availability differs by hook (`model_settings` is `None` in tool hooks per `_run_context.py:98-105`); users must read the docstring to learn what's safe to touch at each hook.
- **User-deps vs. framework-managed secrets.** Passing user dataclasses keeps the library slim and unopinionated, but pushes the burden of secret rotation, scoping, and redaction onto the caller.
- **Capability pattern vs. subclassing the Agent.** Adding a new cross-cutting concern via `Capability` keeps `Agent` stable but increases the count of pluggable surfaces users must learn (capabilities, toolsets, native tools, model settings, hooks). The override context manager has to forward each of them.
- **Durable-execution serialization whitelist.** Dropping `tool_manager`/`capabilities`/`tracer` from `TemporalRunContext` (`durable_exec/temporal/_run_context.py:22-23`) means tools that rely on those fields cannot run in Temporal without reimplementing serialization. The cost is documented; the benefit is a clean serializable contract.
- **`ContextVar` overrides vs. explicit args.** Using `ContextVar` lets deeply nested application code override without threading the new deps through every signature (see `docs/dependencies.md:230-302`), but it also makes dependency injection implicit for anyone who reads the call chain without knowing about `_override_deps`.

## Failure Modes / Edge Cases

- **Synthetic contexts without a queue.** `Agent.system_prompt_parts` builds a `RunContext` with `pending_messages=None`; calling `enqueue` from that synthetic context raises `UserError` (`_run_context.py:262-266`). The error message explicitly names the cause.
- **Tool function with wrong context annotation.** `_function_schema.py:162-171` collects errors for "first parameter annotated `RunContext` but `takes_ctx` is False", "annotation only allowed at first position", and "context annotation in a non-first position". These surface as `UserError` before the model is ever called.
- **Override leak across async boundaries.** `ContextVar` is correct across `asyncio` tasks, but if a tool spawns a thread (the framework already wraps sync tools in `run_in_executor`, `_function_schema.py:87`) and the thread reads `get_current_run_context()` (`_run_context.py:282-288`), it will see the current ContextVar copy — safe, but worth knowing.
- **Durable-execution field omission.** A tool reading `ctx.tool_manager` inside a Temporal activity raises `UserError` thanks to `TemporalRunContext.__getattribute__` (`durable_exec/temporal/_run_context.py:35-45`), which translates the `AttributeError` into a guided error pointing at subclassing `TemporalRunContext.serialize_run_context`.
- **`replace(...)` forking shared sets.** Mis-forking `loaded_capability_ids`/`discovered_tool_names` would silently break capability loads and tool search — explicitly guarded by the comment at `_agent_graph.py:1423-1427`.
- **`ToolDeferred` / `ApprovalRequired` propagation.** When a tool raises these, the tool manager / handler resolves them inline via capability hooks or returns `DeferredToolRequests`; if no handler is installed, the exception propagates to the agent graph (`tool_manager.py:780-797`, `_agent_graph.py:1720-1730`).
- **Tool-call idempotence under retries.** `ctx.retry`/`ctx.max_retries` is mutated per call (`_agent_graph.py:1266-1270`) and `failed_tools` is tracked in `ToolManager` so the same tool failure does not exceed the agent's retry budget (`tool_manager.py:760-766`).
- **Model retries vs. tool retries.** `agent.override(retries=...)` cannot set tool retries (explicit `UserError` at `agent/__init__.py:1842-1849`); tool retries are configured at construction only.

## Future Considerations

- **Add a structured capability/secrets envelope.** Today `RunContext` carries no first-class place for scoped secrets; a `ctx.secrets: SecretsResolver` (or similar) would let `trace_include_content` go further and reduce the boilerplate in `MyDeps` examples.
- **Permission primitive on the context.** A `ctx.permissions: PermissionSet` (e.g. read-only by default, capabilities can grant per-tool scopes) would complement `requires_approval` and make `available_tool_names` driven by an explicit policy rather than a hook function.
- **Slim down `RunContext` into role-specific views.** Hooks that don't need `messages`/`usage`/`tool_manager` could receive a narrower `ToolHookContext`; the cost would be more types but fewer "what's safe to read here?" docstrings.
- **Make `tool_manager` durable.** Several docstrings point at this (`_run_context.py:122-124`); a serializable proxy would let sandbox dispatchers run in Temporal.
- **Document a canonical test pattern.** The `tests/typed_deps.py` and `tests/test_deps.py` files together show the contract; promoting them into `docs/testing.md` (if it exists) would lower the bar for new contributors. Search shows `docs/testing.md` does not exist as a top-level entry — only `tests/AGENTS.md` documents the testing philosophy.
- **Surface "available fields per hook" as a machine-checked table.** A typed registry mapping `(hook_name) -> RunContextSubset` could be enforced by the schema generator.

## Questions / Gaps

- **No `docs/testing.md` index page found.** Only `tests/AGENTS.md` describes the testing philosophy; the public override pattern lives in `docs/dependencies.md:230-302`. Searched `docs/ | grep testing` returned nothing.
- **No framework-managed secret store.** No evidence of `SecretsResolver`, `KeyVault`, or similar in the inspected tree. Searched for `secret`, `vault`, `redact` in `pydantic_ai_slim/pydantic_ai/` — no framework-level concept found.
- **No first-class permission/role model.** Permission is per-tool `requires_approval` plus `ToolSelector` predicates; there is no ACL type in `pydantic_ai_slim/pydantic_ai/`. `docs/dependencies.md` does not document permissions at all — `docs/deferred-tools.md` is the canonical permission doc (out of scope for this dimension).
- **`model_settings` is `None` in tool hooks.** Explicitly noted in `_run_context.py:98-105`; tools that want model settings must read them at construction time. No evidence of a planned change.
- **Secrets in `trace_include_content=False`** is the only built-in safeguard. There is no redaction hook on tool results before they reach the model.

---

Generated by `dimensions/04.04-tool-context-and-dependency-injection.md` against `pydantic-ai`.
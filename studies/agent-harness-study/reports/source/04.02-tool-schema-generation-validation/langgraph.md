# Source Analysis: langgraph

## Tool Schema Generation and Validation

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (Pydantic v1 + v2) on top of langchain-core ≥ 1.3.1 |
| Analyzed | 2026-07-22 |

## Summary

LangGraph does **not** generate JSON Schemas for tool arguments itself. Schema
production is delegated to `langchain_core.tools` (`BaseTool`, the `@tool`
decorator, `args_schema`, `get_input_schema()`). LangGraph’s contribution to
the schema lifecycle is at the **execution and dispatch** boundary, in
`ToolNode` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:622-1579`):

1. **Registration.** Plain callables are coerced into `BaseTool` via
   `create_tool(...)` (`tool_node.py:781`); already-built `BaseTool`s are
   stored verbatim (`tool_node.py:783`). Per-tool injected-argument metadata
   (`_InjectedArgs`) is computed once via `_get_all_injected_args(tool)`
   (`tool_node.py:786`, `tool_node.py:1967-2030`) by reading both the
   `BaseTool.get_input_schema()` model fields *and* the runtime
   `get_type_hints(func, include_extras=True)` annotations. This is what
   later determines which parameters are hidden from the schema presented to
   the LLM and which are stripped before injection (`tool_node.py:1422-1428`).
2. **Dispatch and injection.** The dispatch path (`_run_one`, `_arun_one`)
   (`tool_node.py:1014-1222`) defers tool-name validation so that
   `wrap_tool_call` interceptors can short-circuit calls. Real execution
   uses `tool.invoke(call_args, config)` (sync) /
   `tool.ainvoke(call_args, config)` (async) (`tool_node.py:958`,
   `tool_node.py:1105`), which means **Pydantic argument validation runs
   inside `langchain_core.tools.StructuredTool.invoke`** using the schema
   that LangGraph never authored.
3. **Validation result handling.** `pydantic.ValidationError` raised during
   `invoke` is intercepted and translated into a `ToolInvocationError`
   (`tool_node.py:339-380`, `tool_node.py:959-966`, `tool_node.py:1106-1113`)
   whose `filtered_errors` strip out `InjectedState`, `InjectedStore`, and
   `ToolRuntime` arguments via `_filter_validation_errors(...)`
   (`tool_node.py:510-563`). The `ToolNode` then routes the error through
   `handle_tool_errors` (`tool_node.py:984-1012`, `tool_node.py:1131-1159`)
   and emits a `ToolMessage(status="error", ...)`. The default handler
   `_default_handle_tool_errors` (`tool_node.py:383-391`) catches
   `ToolInvocationError` only, so invocation-shape errors become recoverable
   feedback while arbitrary runtime exceptions bubble up unless the caller
   widens the handler.
4. **Tool-name validation.** A separate, schema-independent guard catches
   calls to unknown tool names and returns the `INVALID_TOOL_NAME_ERROR_TEMPLATE`
   (`tool_node.py:108-110`, `tool_node.py:1268-1279`) listing the registered
   tool names.
5. **Deprecated path.** `ValidationNode` (`libs/prebuilt/langgraph/prebuilt/tool_validator.py:47-221`)
   remains as a deprecated (`LangGraphDeprecatedSinceV10`,
   `tool_validator.py:43-46`) validation-only node that runs `BaseModel.model_validate`
   / `BaseModelV1.validate` against the call’s `args` (`tool_validator.py:193-198`)
   and returns `ToolMessage` with `additional_kwargs={"is_error": True}`
   on failure (`tool_validator.py:208-214`). It is explicitly NOT how
   `ToolNode` validates (`tool_node.py:341-343`, `tool_node.py:959`).

There is **no code in the LangGraph source that calls
`model_json_schema()`, constructs `parameters: {type: "object", ...}`
dicts, or sets `strict: True` on a tool definition**. Portability across
model providers is therefore entirely a function of what
`langchain_core.tools.BaseTool` emits via `.bind_tools(...)` and how each
provider-specific chat model adapter translates that schema.

## Rating

**6 / 10**

Clear separation between validation, error formatting, and dispatch; well
covered by tests for invalid-args, wrong-type, missing-required, injected
args, and unknown-tool-name. However, schemas themselves are not authored
in this codebase, descriptions are inherited passively from docstrings, no
JSON-Schema-level assertion is made that `strict` mode will be honored by
the provider, and the deprecated `ValidationNode` retains v1-Pydantic code
paths that diverge from the `ToolNode` path.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Plain-callable coercion to `BaseTool` | `if not isinstance(tool, BaseTool): tool_ = create_tool(...)` inside `ToolNode.__init__` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:779-786` |
| Injected-arg precomputation | `_get_all_injected_args(tool)` walks `tool.get_input_schema()` + `get_type_hints(func, include_extras=True)` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:786`, `libs/prebuilt/langgraph/prebuilt/tool_node.py:1967-2030` |
| Schema source read at runtime | `full_schema = tool.get_input_schema()` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1980` |
| LLM-controlled vs. injected split | `all_injected_keys` set collected by `_is_injected_arg_type(...)` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1994-2000` |
| Strip caller-supplied injected args before invocation | `stripped_args = {k: v for k, v in tool_call_copy["args"].items() if k not in injected.all_injected_keys}` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1424-1429` |
| Tool invocation entrypoint | `tool.invoke(call_args, config)` (sync) / `tool.ainvoke(call_args, config)` (async) — Pydantic validation happens inside langchain_core here | `libs/prebuilt/langgraph/prebuilt/tool_node.py:958`, `libs/prebuilt/langgraph/prebuilt/tool_node.py:1105` |
| `ValidationError` → `ToolInvocationError` translation | `except ValidationError as exc: ... raise ToolInvocationError(...)` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:959-966`, `libs/prebuilt/langgraph/prebuilt/tool_node.py:1106-1113` |
| Filter injected args from error details | `_filter_validation_errors(exc, injected)` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:510-563`, used at `libs/prebuilt/langgraph/prebuilt/tool_node.py:962`, `libs/prebuilt/langgraph/prebuilt/tool_node.py:1109` |
| `ToolInvocationError` exception type | `class ToolInvocationError(ToolException)` with formatted message | `libs/prebuilt/langgraph/prebuilt/tool_node.py:339-380` |
| Error templates | `INVALID_TOOL_NAME_ERROR_TEMPLATE`, `TOOL_CALL_ERROR_TEMPLATE`, `TOOL_EXECUTION_ERROR_TEMPLATE`, `TOOL_INVOCATION_ERROR_TEMPLATE` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:108-121` |
| Default error handler | `_default_handle_tool_errors` catches `ToolInvocationError`, re-raises everything else | `libs/prebuilt/langgraph/prebuilt/tool_node.py:383-391` |
| Type-driven handler introspection | `_infer_handled_types(handler)` reads first-parameter annotation from `get_type_hints(handler)` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:444-507` |
| Tool-name validation (unknown tools) | `_validate_tool_call` returns `INVALID_TOOL_NAME_ERROR_TEMPLATE`-formatted ToolMessage | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1268-1279` |
| Wrap-tool-call intercept deferral | Comment "Validation is deferred to _execute_tool_sync to allow interceptors to short-circuit requests for unregistered tools" | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1030-1031`, `libs/prebuilt/langgraph/prebuilt/tool_node.py:1177-1178` |
| Agent-side `bind_tools` gate | `model = cast(BaseChatModel, model).bind_tools(tool_classes + llm_builtin_tools)` inside `create_react_agent` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:586-588` |
| Pre-bind consistency check | `_should_bind_tools` raises `ValueError` if bound tool names don’t match passed tools | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:173-217` |
| `InjectedState` / `InjectedStore` / `ToolRuntime` annotations | Marker classes that hide params from LLM-visible schema | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1753-1901` |
| `ToolRuntime` injection | Dataclass carrying state, store, context, config, stream_writer, tool_call_id, tools | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1662-1750` |
| Deprecated `ValidationNode` schema sources | Accepts `BaseTool`, `BaseModel`, `BaseModelV1`, or `Callable` (latter via `create_schema_from_function`) | `libs/prebuilt/langgraph/prebuilt/tool_validator.py:118-166` |
| `ValidationNode` validation call | `schema.model_validate(call["args"])` / `schema.validate(call["args"])` | `libs/prebuilt/langgraph/prebuilt/tool_validator.py:194-197` |
| `ValidationNode` error feedback path | `ToolMessage(content=format_error(...), ..., additional_kwargs={"is_error": True})` | `libs/prebuilt/langgraph/prebuilt/tool_validator.py:208-214` |
| Tests: invalid arg type via `ToolNode` | `test_tool_node_error_handling_default_invocation` | `libs/prebuilt/langgraph/tests/test_tool_node.py:269-294` |
| Tests: unknown tool name | `test_tool_node_incorrect_tool_name` asserts `"Error: tool3 is not a valid tool, try one of [tool1, tool2]."` | `libs/prebuilt/langgraph/tests/test_tool_node.py:565-591` |
| Tests: `ValidationError` raised when `handle_tool_errors=False` | `test_tool_node_handle_tool_errors_false` (lines 472-533) | `libs/prebuilt/langgraph/tests/test_tool_node.py:472-533` |
| Tests: injected-arg error filtering | `test_filter_injected_state_validation_errors`, `test_filter_injected_store_validation_errors`, `test_filter_tool_runtime_validation_errors`, `test_filter_multiple_injected_args` | `libs/prebuilt/langgraph/tests/test_tool_node_validation_error_filtering.py:41-263` |
| Tests: model-controlled errors preserved | `test_no_filtering_when_all_errors_are_model_args` | `libs/prebuilt/langgraph/tests/test_tool_node_validation_error_filtering.py:265-321` |
| Tests: `ValidationNode` invalid-arg path | `test_validation_node` parametrizes function / `BaseModel` / `BaseModelV1` / `@tool`, asserts `is_error` flag on second ToolMessage | `libs/prebuilt/langgraph/tests/test_validation_node.py:35-87` |
| Tests: not-required state field | `test_injected_state_not_required_field_missing_injects_none` | `libs/prebuilt/tests/test_injected_state_not_required.py:87-120` |
| Tests: `_should_bind_tools` mismatched-name error | `test_should_bind_tools` raises `ValueError` on missing/extra tools | `libs/prebuilt/tests/test_react_agent.py:1492-1527` |
| Tests: custom BaseTool with `args_schema` | `MyCustomTool(... args_schema=CustomToolSchema)` invoked through `ToolNode` | `libs/prebuilt/tests/test_tool_node.py:688-697` |
| Tests: error-handler type inference | `test__infer_handled_types` exercises `Union` annotations, `Exception` subclasses | `libs/prebuilt/tests/test_react_agent.py:436-496` |
| Dependency on langchain-core for schemas | `pyproject.toml` pins `langchain-core>=1.3.1` | `libs/prebuilt/pyproject.toml:28` |

## Answers to Dimension Questions

1. **Are schemas generated or handwritten?**
   Schemas are *generated* by `langchain_core.tools`. LangGraph imports
   `BaseTool`, `create_tool`, `InjectedToolArg`, `args_schema`, and
   `get_input_schema` but never instantiates a Pydantic model or builds a
   `parameters` JSON-Schema object itself
   (`libs/prebuilt/langgraph/prebuilt/tool_node.py:76-84`,
   `libs/prebuilt/langgraph/prebuilt/tool_node.py:1980`,
   `libs/prebuilt/langgraph/prebuilt/tool_validator.py:24`). Plain callables
   are wrapped with `create_tool(...)` at registration
   (`tool_node.py:781`), and existing `BaseTool`s keep their authored
   `args_schema`. The deprecated `ValidationNode` is the only place that
   accepts a raw Pydantic `BaseModel` as a "tool schema"
   (`tool_validator.py:116-166`).

2. **Are descriptions useful?**
   Descriptions are **inherited passively** from the function docstring and
   from each field’s `Field(description=...)`. There is no LangGraph-side
   validator that asserts a non-empty description, that warns on missing
   descriptions, or that surfaces descriptions to the LLM in a normalized
   form. The base `BaseTool` API and the chat-model provider adapter are
   responsible for forwarding `description` into the tool-calling payload;
   LangGraph only enforces that *annotations* marked as injected
   (`InjectedState`, `InjectedStore`, `ToolRuntime`,
   `_is_injected_arg_type(...)`) are stripped before the schema is read for
   injection (`tool_node.py:1994-2000`, `tool_node.py:1422-1429`). The
   default error templates do include the tool name (`tool_node.py:108-121`),
   which gives the model a coarse pointer, but per-argument descriptions
   are not used in error messages.

3. **Is validation strict?**
   Validation strictness is delegated to Pydantic and to the chat model
   provider. LangGraph catches `pydantic.ValidationError`
   (`tool_node.py:959`, `tool_node.py:1106`) and converts it into a
   recoverable `ToolMessage(status="error")` via the configurable
   `handle_tool_errors` (`tool_node.py:984-1012`). By default, the
   handler only swallows `ToolInvocationError`, so unexpected runtime
   exceptions still propagate. Unknown tool names are caught earlier
   (`tool_node.py:1268-1279`) and produce a structured error ToolMessage.
   The deprecated `ValidationNode` is strictly schema-bound: it rejects
   tools whose `args_schema` is not a `BaseModel`
   (`tool_validator.py:148-154`) and refuses anything that is not a
   `BaseModel`, `BaseModelV1`, or `Callable`
   (`tool_validator.py:156-166`).

4. **Are defaults handled clearly?**
   Defaults flow through Pydantic → `langchain_core` → JSON Schema; LangGraph
   only re-uses them indirectly through `field_info.is_required()` when
   deciding whether a missing `InjectedState` argument should raise or
   default to `None` (`tool_node.py:2010-2012`, `tool_node.py:1396`,
   `tool_node.py:1404`). There is no LangGraph code that surfaces defaults
   to the model or to error messages, but the test
   `test_pydantic_state_with_default_field_missing_works`
   (`libs/prebuilt/tests/test_injected_state_not_required.py:227-256`)
   confirms that Pydantic defaults on injected state fields do propagate.
   For LLM-controlled fields, default behavior is whatever the chat-model
   adapter negotiated with Pydantic; LangGraph does not intervene.

5. **Are schemas portable across providers?**
   **Not directly.** Schemas are whatever `BaseTool.args_schema` produces
   via Pydantic’s `model_json_schema`. LangGraph calls
   `model.bind_tools(tool_classes + llm_builtin_tools)` with `BaseTool`
   instances (`chat_agent_executor.py:586-588`); provider-specific
   translation happens inside each chat model’s `bind_tools`
   implementation (OpenAI/Anthropic/etc.). The only portability check
   inside LangGraph is the name-equality check in `_should_bind_tools`
   (`chat_agent_executor.py:173-217`), which supports both OpenAI-style
   `{type: "function", function: {name: ...}}` and Anthropic-style
   `{name: ...}` payloads (`chat_agent_executor.py:202-211`). There is
   no assertion that all providers will accept the same Pydantic types
   (e.g., `Union`, `Literal`, recursive models), and there is no
   conversion or fallback path.

## Architectural Decisions

- **Validation belongs in the tool, not in the node.** Argument schema is
  generated and stored on the `BaseTool` instance, then enforced inside
  `tool.invoke(...)`/`tool.ainvoke(...)`
  (`tool_node.py:958`, `tool_node.py:1105`). `ToolNode` only inspects the
  `ValidationError` and rewraps it.
- **Errors become messages, not exceptions, by default.** Validation
  failures are turned into `ToolMessage(status="error", ...)` so that the
  LLM can self-correct on the next turn. The default
  `_default_handle_tool_errors` (`tool_node.py:383-391`) scopes this to
  invocation-shape errors; arbitrary tool runtime errors re-raise so the
  developer sees them.
- **Injected arguments are silently filtered from error reports.** A
  consistent invariant across the codebase: `_InjectedArgs` is built once
  at registration (`tool_node.py:786`), then used both to *strip* injected
  keys before invocation (`tool_node.py:1424-1429`) and to *filter* them
  from `ValidationError.errors()` before they reach the LLM
  (`tool_node.py:510-563`, `tool_node.py:962`, `tool_node.py:1109`).
  The rationale is stated in the module docstring
  (`tool_node.py:1421-1423`): callers should not be able to forge
  `InjectedToolArg` values via `ToolCall.args`.
- **Tool-name validation is separable from argument validation.** The
  name check runs in `_validate_tool_call` (`tool_node.py:1268-1279`)
  before any model-level validation, so a call to a non-existent tool
  short-circuits with a list of registered names rather than a Pydantic
  traceback.
- **`ToolNode` and `ValidationNode` are independent code paths.** Despite
  the similar name, `ValidationNode` is a schema-only check that returns
  a JSON dump of validated args (`tool_validator.py:194-198`); it does
  *not* call the tool. It is deprecated in favor of
  `create_agent` + `ToolNode` (`tool_validator.py:43-46`).
- **Interceptors can defer validation.** `_run_one` builds a
  `ToolCallRequest` with `tool=None` for unknown tools
  (`tool_node.py:1032-1040`) and lets `wrap_tool_call` short-circuit. The
  `_execute_tool_sync`/`_execute_tool_async` paths then call
  `_validate_tool_call` only if the interceptor forwards the request
  (`tool_node.py:944-950`, `tool_node.py:1091-1097`). This decouples
  dispatch from argument validation.

## Notable Patterns

- **Per-tool `_InjectedArgs` dataclass built once.** A single pass over the
  tool’s schema and type hints is performed at `ToolNode.__init__` time
  (`tool_node.py:1967-2030`) so per-call dispatch can avoid repeated
  reflection. `_optional_state_args` is captured from
  `field_info.is_required()` (`tool_node.py:2010-2012`), giving
  `_inject_tool_args` enough information to silently default missing state
  fields to `None` (`tool_node.py:1396`, `tool_node.py:1404`).
- **Error filtering by `loc` tuple.** `_filter_validation_errors` keeps
  only errors whose first `loc` element is not an injected arg name
  (`tool_node.py:548`) and strips injected keys from `error["input"]`
  (`tool_node.py:553-558`), preventing PII-like system values from
  appearing in LLM-facing error messages.
- **Type-driven handler policy.** `_infer_handled_types`
  (`tool_node.py:444-507`) reads the first parameter’s annotation via
  `get_type_hints` and converts it into a runtime filter; it rejects
  non-`Exception` types (`tool_node.py:482-490`,
  `tool_node.py:493-503`). This lets `handle_tool_errors` be expressed as
  a typed callable.
- **Defensive strip of caller-supplied injected keys.** Even though the
  LLM-visible schema omits injected args, `ToolNode` strips them again
  right before invocation (`tool_node.py:1422-1429`). The comment
  explicitly cites the threat model: "This prevents an LLM from forging
  hidden InjectedToolArg fields via ToolCall.args"
  (`tool_node.py:1421-1423`).
- **Two input-shape modes for `ToolNode`.** "list of messages" vs. "dict
  with messages key" vs. "raw `tool_call_with_context` Send payload" all
  share the same dispatch path (`tool_node.py:1224-1266`), so a single
  schema contract works across deployment shapes.

## Tradeoffs

- **Latent dependency on `langchain_core` schema behavior.** Any change in
  Pydantic ↔ JSON-Schema output, or in `BaseTool`’s handling of injected
  args, propagates silently into LangGraph because there is no local
  schema-version check. The only contract test is the executable suite,
  which depends on the pinned `langchain-core>=1.3.1`
  (`libs/prebuilt/pyproject.toml:28`).
- **Errors lose precision.** Pydantic’s `ValidationError` is collapsed
  into `loc: msg` lines by `ToolInvocationError`
  (`tool_node.py:362-369`), so the model sees a simplified message
  rather than the rich type-info Pydantic provides. This is a deliberate
  trade for LLM readability but does mean the developer also sees the
  simplified form unless `handle_tool_errors=False`.
- **Default handler is permissive of schema errors but strict of runtime
  errors.** `_default_handle_tool_errors` only catches
  `ToolInvocationError` (`tool_node.py:383-391`). A Pydantic error raised
  *outside* of the schema-validation path (e.g., from a custom validator
  that fails after model construction) would propagate, which can be
  surprising.
- **`ValidationNode` is a dead end for new code.** It supports Pydantic
  v1 explicitly (`tool_validator.py:29-30`, `tool_validator.py:196-198`)
  and is decorated with `LangGraphDeprecatedSinceV10`
  (`tool_validator.py:43-46`). The recommended path is
  `create_react_agent(...)`, which routes through `ToolNode`.
- **Injected-state filtering can mask real bugs.** If a developer
  misspells an `InjectedState("messages")` argument, the validation
  error for that field is suppressed by
  `_filter_validation_errors` (`tool_node.py:548`), so the LLM never
  learns about the misconfiguration. The developer only sees it if they
  set `handle_tool_errors=False`.

## Failure Modes / Edge Cases

- **Unknown tool names** return a structured `ToolMessage` listing
  available tools (`tool_node.py:1268-1279`). Coverage:
  `test_tool_node_incorrect_tool_name` (`test_tool_node.py:565-591`).
- **Missing required LLM-controlled arguments** are caught by Pydantic
  inside `tool.invoke` and reported as `ToolInvocationError`. Coverage:
  `test_filter_injected_store_validation_errors`
  (`test_tool_node_validation_error_filtering.py:95-152`).
- **Wrong-type arguments** are caught and the error path is verified for
  sync and async dispatch:
  `test_tool_node_error_handling_default_invocation`
  (`test_tool_node.py:269-294`),
  `test_filter_injected_state_validation_errors`
  (`test_tool_node_validation_error_filtering.py:41-93`).
- **Forged injected args in `tool_call["args"]`** are stripped before
  invocation (`tool_node.py:1424-1429`). No explicit test, but the
  defensive strip and the comment at `tool_node.py:1421-1423` document
  the intent.
- **Tools raising `GraphInterrupt`/`GraphBubbleUp`** always propagate
  (`tool_node.py:982-983`, `tool_node.py:1129-1130`), even when
  `handle_tool_errors=True`. Documented in the comment block
  (`tool_node.py:973-981`).
- **Wrapper throwing an exception** is caught at the top of
  `_run_one`/`_arun_one` (`tool_node.py:1054-1067`,
  `tool_node.py:1205-1222`) and converted to an error `ToolMessage`
  unless `handle_tool_errors=False`. Tests:
  `test_interceptor_exception_with_unregistered_tool`,
  `test_async_interceptor_exception_with_unregistered_tool`
  (`tests/test_tool_node_interceptor_unregistered.py:337-511`).
- **Mismatched bound tools** between `model.bind_tools(...)` and the
  tools passed to `create_react_agent` raise `ValueError`
  (`chat_agent_executor.py:194-198`, `chat_agent_executor.py:214-215`).
  Test: `test_should_bind_tools` (`tests/test_react_agent.py:1492-1527`).
- **Schema-less tools** are rejected by `ValidationNode`
  (`tool_validator.py:144-154`) but accepted by `ToolNode`, which only
  requires a callable or `BaseTool`. If a plain function lacks a type
  hint, Pydantic will produce an `Any`-typed schema and validation
  becomes a no-op for that field.

## Future Considerations

- **Move schema generation into LangGraph.** Today, schema portability,
  field descriptions, and Pydantic quirks all leak in from
  `langchain_core`. A thin schema-emit helper that emits a single,
  stable JSON-Schema dialect (with explicit `additionalProperties: false`
  and `required` arrays) would let LangGraph guarantee portability and
  add `strict: True` for OpenAI-compatible providers.
- **Surface per-field descriptions in error messages.** Right now the
  model only learns that `value: Input should be a valid integer`. With
  field descriptions available, a templated error could read `value
  (the user's age in years): Input should be a valid integer`, improving
  self-correction.
- **Test cross-provider schema portability.** The provider-agnostic
  shape in `_should_bind_tools` (`chat_agent_executor.py:202-211`)
  handles name mismatch but not feature mismatch (e.g., `Union`,
  `Literal`, recursive models). A snapshot test that asserts the
  emitted schema for a representative tool against multiple provider
  adapters would catch drift.
- **Replace `ValidationNode` outright.** It is already deprecated
  (`tool_validator.py:43-46`), but the dual code paths still exist in
  the package. Removing it after one or two minor versions would
  eliminate the v1-Pydantic branch
  (`tool_validator.py:29-30`, `tool_validator.py:196-198`).
- **Emit a warning when a tool has no description or empty docstring.**
  Useful descriptions are the single biggest lever for tool-call
  accuracy, and LangGraph is well-positioned to nudge developers
  because it already inspects every registered tool at
  `ToolNode.__init__` time (`tool_node.py:779-786`).

## Questions / Gaps

- **No evidence found** of JSON-Schema `strict` mode negotiation in the
  LangGraph source. `bind_tools` is delegated to the chat model
  (`chat_agent_executor.py:586-588`); whether the resulting payload sets
  `strict: true` is provider-controlled and not asserted here.
- **No evidence found** that LangGraph emits or transforms field-level
  descriptions beyond what `BaseTool` already exposes.
- **No evidence found** of a default-value contract for LLM-controlled
  fields; only injected-state defaults are observed
  (`tool_node.py:2010-2012`).
- **No evidence found** for `additionalProperties: false` or
  `required: [...]` being explicitly set on the schema; behavior is
  inherited from Pydantic and from each provider adapter.
- Search boundary: the analysis inspected
  `libs/prebuilt/langgraph/prebuilt/{tool_node.py,tool_validator.py,chat_agent_executor.py,_tool_call_*.py,interrupt.py}`,
  the corresponding test suite under `libs/prebuilt/tests/`, and
  `pyproject.toml`. The `langgraph` core library contributes no tool-schema
  generation code; its `pregel/_tools.py` only handles streaming and
  callback wiring (`libs/langgraph/langgraph/pregel/_tools.py:25-265`).

---

Generated by `04.02-tool-schema-generation-and-validation` against `langgraph`.

# Source Analysis: openai-agents-sdk

## 04.02 - Tool Schema Generation and Validation

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+ / Pydantic v2 / OpenAI Responses + Chat Completions + Realtime APIs |
| Analyzed | 2026-07-22 |

## Summary

The OpenAI Agents SDK generates tool argument schemas automatically from Python function signatures by introspecting `inspect.signature` and `typing.get_type_hints`, building a Pydantic `BaseModel` dynamically via `pydantic.create_model`, then exporting the model's `model_json_schema()` and shaping it into OpenAI's "strict" JSON Schema dialect (`src/agents/function_schema.py:224-424`). `FunctionTool.__post_init__` re-runs the strict-schema transform on the stored schema before any provider sees it (`src/agents/tool.py:503-512`). Description text comes from four layered sources - the function docstring (parsed by `griffe`, with auto-detected `google | numpy | sphinx` style), `Annotated[..., "<desc>"]` metadata, `pydantic.Field(..., description=...)`, and explicit `description_override` (`src/agents/function_schema.py:148-281`); the docstring description wins over `Annotated` text and `Field` `description` falls back if docstring text is missing for that parameter (`tests/test_function_schema.py:540-587`). Argument JSON parsing happens at tool invocation time: `_parse_function_tool_json_input` rejects non-object JSON with `ModelBehaviorError`, then Pydantic `ValidationError` is wrapped and routed through a `_FailureHandlingFunctionToolInvoker` (`src/agents/tool.py:526-568`, `src/agents/tool.py:1575-1598`), where the default `default_tool_error_function` produces a model-visible retry message ("An error occurred while parsing tool arguments. Please try again with valid JSON. Error: ...") instead of failing the run (`src/agents/tool.py:1609-1618`). Generation is fed by Pydantic's `BaseModel`, which means constraints (`gt`, `ge`, `le`, `min_length`, `pattern`, etc.) flow into the schema verbatim (`src/agents/function_schema.py:317-404`; `tests/test_function_schema.py:456-887`). The JSON-Schema dialect is OpenAI-specific: `ensure_strict_json_schema` rewrites Pydantic output (forces `additionalProperties: false`, adds `required` from `properties`, merges single-entry `allOf`, collapses `oneOf` → `anyOf`, expands `$ref`s) and raises `UserError` if you set `additionalProperties: true` (`src/agents/strict_schema.py:18-149`). Schemas are emitted verbatim to the Responses and Chat Completions wire formats (`"parameters": tool.params_json_schema`, `strict: tool.strict_json_schema`) and the parser rejects things Responses cannot express, e.g. bare `Mapping` parameters (`src/agents/function_schema.py:435-441`, `tests/test_function_schema.py:434-442`). The same `params_json_schema` is reused unchanged by `tool_to_openai` (`src/agents/models/chatcmpl_converter.py:865-879`) and the Realtime bridge (`src/agents/realtime/openai_realtime.py:1543-1550`), so schemas are portable across the three backends even though the strict dialect is OpenAI-tuned.

## Rating

**8 / 10** - Clear, well-tested, observable schema pipeline with explicit strict-mode handling and recovery hooks. Reasons it does not reach 9-10: the strict schema dialect is OpenAI-specific (subset of JSON Schema 2020-12 plus targeted mutations), so non-OpenAI providers may need to relax or drop strict mode; `ensure_strict_json_schema` is destructive and copies are required to avoid mutating shared schemas (`src/agents/mcp/util.py:535-544`); defaults can be lost (e.g. `default: None` is stripped, `src/agents/strict_schema.py:121-122`); and validation/correction is implicit through `ModelBehaviorError` rather than an explicit "retry" path separate from the rest of the failure pipeline.

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Signature + type-hint introspection | `inspect.signature` walk + `get_type_hints(..., include_extras=True)` pulled into a Pydantic `BaseModel` via `create_model` | `src/agents/function_schema.py:263-407` |
| Pydantic `BaseModel` is the schema source | `from pydantic import BaseModel, Field, create_model`; `dynamic_model = create_model(f"{func_name}_args", __base__=BaseModel, **fields)` | `src/agents/function_schema.py:13`, `src/agents/function_schema.py:407` |
| JSON schema produced from Pydantic model | `dynamic_model.model_json_schema()` then optionally strict-transformed | `src/agents/function_schema.py:410-412` |
| `FIELD`-style + `Annotated` descriptions merged into schema | description precedence docstring > Annotated > Field; `merge_field_infos` for `Annotated[T, Field(...)]` | `src/agents/function_schema.py:215-222`, `src/agents/function_schema.py:370-404` |
| Docstring parsing via `griffe` | `generate_func_documentation` uses griffe + auto style detection (`google`, `numpy`, `sphinx`) | `src/agents/function_schema.py:91-187` |
| Description override / docstring info disabling | `description_override`, `use_docstring_info` arguments | `src/agents/function_schema.py:228-261`, `src/agents/function_schema.py:284` |
| `*args` / `**kwargs` mapped to `list[T]` / `dict[K, V]` | Annotation rewrites for `VAR_POSITIONAL` / `VAR_KEYWORD` | `src/agents/function_schema.py:333-368` |
| `RunContextWrapper` / `ToolContext` stripped from schema, must be first | `takes_context` detection; `UserError` on non-first position | `src/agents/function_schema.py:289-315` |
| Bare `Mapping` raises in strict mode | Pydantic `Mapping` JSON Schema includes unmappable `additionalProperties` pattern; rejected upstream by `_ensure_strict_json_schema` | `src/agents/strict_schema.py:51-64`, `tests/test_function_schema.py:434-442` |
| `_suppress_griffe_logging` keeps schema generation quiet | raises `WARNING` to `ERROR` for missing annotations | `src/agents/function_schema.py:136-145` |
| Static strict-schema normalizer | `ensure_strict_json_schema` mutates dict, expanding `$defs`/`definitions`, removing `default: None`, merging single-entry `allOf`, converting `oneOf` to `anyOf` | `src/agents/strict_schema.py:18-149` |
| Strict-mode guarantees: `additionalProperties: false`, `required` filled | `properties` -> `required` set from keys | `src/agents/strict_schema.py:51-74` |
| `$ref` resolution to inline schema | unwrap via `resolve_ref`, preserves sibling keys, recursively re-tightens | `src/agents/strict_schema.py:129-149` |
| Strict schema copies must be made before mutation | MCP code deepcopies before passing to strictify, then keeps original as non-strict fallback | `src/agents/mcp/util.py:528-545` |
| `FunctionTool` dataclass stores schema verbatim | `params_json_schema: dict[str, Any]` field | `src/agents/tool.py:392-393` |
| `FunctionTool.__post_init__` re-applies strict transform on stored schema | `if self.strict_json_schema: ensure_strict_json_schema(copy.deepcopy(...))` | `src/agents/tool.py:507-510` |
| Strict mode is on by default in `@function_tool` | `strict_mode: bool = True` (alias `strict_json_schema`) | `src/agents/tool.py:1859`, `src/agents/tool.py:1907` |
| Validation before execution via dynamic Pydantic model | `schema.params_pydantic_model(**json_data)` then catch `ValidationError` -> `ModelBehaviorError` | `src/agents/tool.py:1978-1990` |
| JSON parsing before Pydantic validation | `_parse_function_tool_json_input` rejects non-object JSON (`[]`, `"value"`, `123`, `null`, `true`) | `src/agents/tool.py:1575-1598`, `tests/test_function_tool.py:758-782` |
| Per-tool `failure_error_function` (default / custom / None) | `default_tool_error_function`, `_build_handled_function_tool_error_handler` | `src/agents/tool.py:1519-1618` |
| Default formatter emits a model-recoverable retry message | "An error occurred while parsing tool arguments. Please try again with valid JSON. Error: ..." | `src/agents/tool.py:1609-1618` |
| Invalid-JSON redacts payload when `DONT_LOG_TOOL_DATA` is set | raises without `__cause__` / `__context__` to avoid `JSONDecodeError.doc` leaking | `src/agents/tool.py:1583-1593`, `tests/test_function_tool.py:786-813` |
| Pydantic `ValidationError` rewrapped as `ModelBehaviorError` | catches and re-raises with full Pydantic message | `src/agents/tool.py:1989-1990` |
| `default_tool_error_function` is resolved at invoke time (overridable) | `monkeypatch.setattr(tool_module, "default_tool_error_function", patched_default)` test confirms runtime patching | `tests/test_function_tool.py:601-617` |
| `failure_error_function=None` re-raises `ModelBehaviorError` to caller | surface JSON-decode / validation errors | `tests/test_function_tool.py:2857-2876` |
| Tool-call resolution: missing tool name -> `ModelBehaviorError` or model-visible error | `tool_not_found_behavior = "raise_error" \| "return_error_to_model"` in `RunConfig` | `src/agents/run_config.py:330-338`, `src/agents/run_internal/turn_resolution.py:1958-1970` |
| Tool error formatter callback with kind discriminator (`tool_not_found` etc.) | `RunConfig.tool_error_formatter` callback | `tests/test_agent_runner.py:4493-4534` |
| `Agent.as_tool` schema generation | `params_adapter.json_schema()` then `ensure_strict_json_schema` | `src/agents/agent.py:577-583` |
| Agent-tool `failure_error_function` propagation | passed through `_build_wrapped_function_tool` | `src/agents/agent.py:913-932` |
| `CustomTool` (Responses raw text input) skips schema pipeline | `format: object \| None = None` flows straight to Responses | `src/agents/tool.py:1292-1335` |
| MCP tool schema adoption + best-effort strict conversion | `convert_schemas_to_strict`, deep-copy, fallback to non-strict on conversion failure | `src/agents/mcp/util.py:497-568` |
| Wire schemas go to Responses and Chat Completions verbatim | `"parameters": tool.params_json_schema`, `"strict": tool.strict_json_schema` | `src/agents/models/openai_responses.py:1962-1977`, `src/agents/models/chatcmpl_converter.py:864-879` |
| Wire schemas go to Realtime session tools verbatim | `parameters=tool.params_json_schema` | `src/agents/realtime/openai_realtime.py:1543-1550` |
| `tool_to_openai` rejects Responses-only features on Chat Completions | `ensure_function_tool_supports_responses_only_features` | `src/agents/tool.py:1408-1424` |
| Output schema is also `strict`-compat (`AgentOutputSchema`) | `ensure_strict_json_schema(_output_schema)`; raises `UserError` if not strict-compatible | `src/agents/agent_output.py:79-120` |
| Manual `FunctionTool` accepts a pre-built schema + `strict_json_schema` flag | `FunctionTool(..., strict_json_schema=False)` keeps Pydantic schema verbatim | `tests/test_function_tool.py:335-388` |
| Schema immutability: `FunctionTool.__post_init__` deepcopies before strictify | deep-copies `params_json_schema` so caller-owned dict is not mutated | `src/agents/tool.py:507-510`, `tests/test_function_tool.py:737-755` |
| `params_json_schema` returned by `to_call_args` rebuilds Python call shape | handles positional only / positional-or-keyword / var / keyword-only | `src/agents/function_schema.py:44-76` |
| Documented schema generation contract | "auto-parses signature", `inspect`, `griffe`, dynamic Pydantic model | `docs/tools.md:249-257`, `docs/tools.md:432-441` |
| Pydantic `Field` constraints explicitly supported in docs | default-based + `Annotated` forms | `docs/tools.md:439-457` |
| `mcp_config["convert_schemas_to_strict"]` documented as best-effort | "If a schema cannot be converted, the original schema is used." | `docs/mcp.md:30-54` |
| Snapshot test pins generated schema shape for docstring extraction | `params_json_schema` snapshot for `get_weather` | `tests/test_function_tool_decorator.py:217-248` |
| OneOf→AnyOf + discriminated unions preserved | `test_strict_schema_oneof.py` covers `Annotated[..., Field(discriminator=...)]` | `tests/test_strict_schema_oneof.py:1-150` |

## Answers to Dimension Questions

1. **Are schemas generated or handwritten?**
   Generated. `function_schema()` (`src/agents/function_schema.py:224-424`) introspects a Python function's signature/type hints and dynamically builds a Pydantic `BaseModel` (`src/agents/function_schema.py:407`), then exports its `model_json_schema()` (`src/agents/function_schema.py:410`). Handwritten schemas are supported only via `FunctionTool(..., params_json_schema=...)` (manual construction) or via `Agent.as_tool`/`CodexTool` which derive them from a Pydantic model the user supplies (`src/agents/agent.py:577-583`, `src/agents/extensions/experimental/codex/codex_tool.py:322`); even those handwritten schemas are then run through `ensure_strict_json_schema` before being sent to the model.
2. **Are descriptions useful?**
   Yes, with strong fidelity to Python conventions. Sources are layered: `griffe`-parsed docstrings (Google/NumpY/Sphinx with auto-detection, `src/agents/function_schema.py:91-187`), `Annotated[T, "<desc>"]` strings (`src/agents/function_schema.py:206-281`), and `pydantic.Field(..., description=...)` merged via `FieldInfo.merge_field_infos` (`src/agents/function_schema.py:215-222`, `src/agents/function_schema.py:370-404`). Precedence is documented and tested: docstring description > `Annotated` > `Field` (`tests/test_function_schema.py:540-587`). `description_override` lets users supply a complete replacement (`src/agents/function_schema.py:228`). MCP tools get a fallback description-to-title resolution (`src/agents/_mcp_tool_metadata.py:38-58`). Snapshot tests pin the exact schema including descriptions (`tests/test_function_tool_decorator.py:217-248`).
3. **Is validation strict?**
   Yes by default (`strict_json_schema=True` on `FunctionTool`, `src/agents/tool.py:408`). Strict mode is enforced by re-running `ensure_strict_json_schema` on the stored schema in `FunctionTool.__post_init__` (`src/agents/tool.py:507-510`); set `strict_mode=False` (alias `strict_json_schema=False`) at the decorator to opt out (`src/agents/tool.py:1859`, `src/agents/tool.py:1907`). Pydantic constraints (`gt`, `ge`, `le`, `min_length`, `max_length`, `pattern`, ...) flow directly into the schema and are tested at `tests/test_function_schema.py:456-887`. Argument validation uses Pydantic at invocation: `ModelBehaviorError` is raised on JSON decoding failure or schema mismatch, then routed through `default_tool_error_function` so the model sees the error (`src/agents/tool.py:1609-1618`, `src/agents/tool.py:1989-1990`).
4. **Are defaults handled clearly?**
   Carefully but with edge cases. `Field(default=...)` and `Annotated[T, Field(default=...)]` are preserved in the generated schema (`src/agents/function_schema.py:391-404`, `tests/test_function_schema.py:487-521`); Pydantic populates them when arguments are omitted. `*args` defaults to `default_factory=list` and `**kwargs` to `default_factory=dict` (`src/agents/function_schema.py:347-368`). `ensure_strict_json_schema` strips `default: None` because OpenAI strict mode treats `nullable` + missing-default equivalently (`src/agents/strict_schema.py:121-122`) - the test pins this behaviour (`tests/test_strict_schema.py:59-70`); callers relying on `None` defaults will not see them serialized, which can surprise non-OpenAI providers.
5. **Are schemas portable across providers?**
   The JSON Schema dialect is OpenAI-specific (strict). The same `params_json_schema` flows unchanged to Responses (`src/agents/models/openai_responses.py:1962-1977`), Chat Completions (`src/agents/models/chatcmpl_converter.py:864-879`), Realtime (`src/agents/realtime/openai_realtime.py:1543-1550`), and Codex (`src/agents/extensions/experimental/codex/codex_tool.py:413`). Providers that don't speak OpenAI strict mode can pass `strict_mode=False` (`src/agents/tool.py:1907`) or use `AgentOutputSchema(SomeType, strict_json_schema=False)` (`src/agents/agent_output.py:79-120`) to opt out. Chat Completions explicitly forbids Responses-only features via `ensure_function_tool_supports_responses_only_features` (e.g. `defer_loading=True`, `tool_namespace`) (`src/agents/tool.py:1408-1424`). MCP schemas are best-effort strictified with a documented fallback (`docs/mcp.md:49-54`, `src/agents/mcp/util.py:534-545`).

## Architectural Decisions

- **Pydantic as the single source of truth**: schemas are derived from a runtime-built `BaseModel` so typing, constraints, defaults, validators, and docstring descriptions collapse into one normalized artifact (`src/agents/function_schema.py:407-411`).
- **Separation of "schema generation" and "wire-format normalization"**: `function_schema()` produces a vanilla JSON Schema; `ensure_strict_json_schema` then shapes it to the OpenAI strict dialect. The transform is idempotent on the empty schema (`src/agents/strict_schema.py:7-29`).
- **`strict_json_schema` is the default**: explicit recommendation in code comments and docs (`src/agents/tool.py:409-410`, `src/agents/function_schema.py:41-42`, `src/agents/tool.py:1941-1945`).
- **Invariants over options**: rather than letting Pydantic's JSON Schema float freely, `ensure_strict_json_schema` enforces `additionalProperties: false`, fills `required`, normalizes `default: None`, and merges `allOf` (single entry), because the OpenAI API rejects relaxations (`src/agents/strict_schema.py:51-122`).
- **`ModelBehaviorError` as a single exception bucket** for validation failures of all kinds (bad JSON, schema mismatch, missing tool, missing call_id), distinguished only by message prefix and tracing - reused across function, MCP, custom, computer, shell, and apply_patch tools (`src/agents/tool.py:1575-1598`, `src/agents/mcp/util.py:660-680`, `src/agents/run_internal/tool_execution.py:613-776`).
- **Failure formatting is a per-tool, configurable pipeline**: `failure_error_function` (`default_tool_error_function` / custom / `None`) is resolved at invoke time so it survives tool copy/replace (`tests/test_function_tool.py:601-687`) and the default is patched safely via `monkeypatch.setattr` (`tests/test_function_tool.py:602-617`).
- **Tool-call recovery is configurable per run**: `RunConfig.tool_not_found_behavior="return_error_to_model"` causes tool-name mismatches to be returned as a `function_call_output` rather than raising, allowing the model to retry (`tests/test_agent_runner.py:4459-4534`); a `tool_error_formatter` callback can rewrite the message.

## Notable Patterns

- Function signature → Pydantic `BaseModel` → JSON Schema → OpenAI strict JSON Schema pipeline is end-to-end introspected, with fallback paths for `Annotated` metadata, default factories, and `*args`/`**kwargs` (`src/agents/function_schema.py:224-424`).
- Docstring parser via `griffe` with auto-style detection (Google/NumpY/Sphinx) and prioritized over `Annotated` (`src/agents/function_schema.py:91-187`, `tests/test_function_schema.py:540-559`).
- `ensure_strict_json_schema` is recursive and idempotent on the empty schema, returns fresh deep copies, and runs both eagerly (per call) and lazily (`FunctionTool.__post_init__`) (`src/agents/strict_schema.py:18-29`, `src/agents/tool.py:507-510`).
- `_FailureHandlingFunctionToolInvoker` wraps the tool invoker so deep-copied/replaced tools resolve their own failure policy (`src/agents/tool.py:526-568`).
- JSON-decode error redaction: when `_debug.DONT_LOG_TOOL_DATA` is set, the wrapper raises without attaching the raw payload via `__cause__`/`__context__` to prevent `JSONDecodeError.doc` from leaking into tracebacks (`src/agents/tool.py:1583-1593`, `tests/test_function_tool.py:786-813`).
- `OneOf` → `AnyOf` rewrite plus discriminator preservation for OpenAI strict mode (Pydantic `Annotated[..., Field(discriminator="...")]`) (`src/agents/strict_schema.py:90-103`, `tests/test_strict_schema_oneof.py:69-92`).
- MCP conversion is opt-in (`convert_schemas_to_strict=True`) with a deep-copy fallback to the un-strictified schema on conversion failure (`src/agents/mcp/util.py:528-545`, `docs/mcp.md:30-54`).
- `FunctionTool.__post_init__` always deepcopys before strictifying so caller-owned schema dicts are not mutated (`src/agents/tool.py:507-510`, `tests/test_function_tool.py:737-755`).
- `tool_to_openai` (Chat Completions path) requires the strict schema but explicitly refuses Responses-only extensions, keeping the Chat/Responses split enforceable (`src/agents/tool.py:1408-1424`, `src/agents/models/chatcmpl_converter.py:864-879`).

## Tradeoffs

- **OpenAI strict dialect coupling**: `ensure_strict_json_schema` enforces OpenAI's strict-mode constraints (`additionalProperties: false`, mandatory `required`, `default: None` stripped, `oneOf` collapsed). Non-OpenAI providers may need to opt out per tool or output schema (`src/agents/strict_schema.py:51-122`, `src/agents/agent_output.py:112-120`).
- **Destructive schema normalization**: every code path that wants strict mode must pass a deep copy into `ensure_strict_json_schema` - the function mutates in place (`src/agents/strict_schema.py:18-29`). The MCP code path makes this explicit (`src/agents/mcp/util.py:534-545`); `FunctionTool.__post_init__` does it on the user's behalf.
- **`default: None` loss**: stripping `default: None` is the documented behavior for OpenAI strict mode but is a one-way ratchet - reverting to non-strict retains the lost default (`src/agents/strict_schema.py:121-122`, `tests/test_strict_schema.py:59-70`).
- **Implicit "correction" via exception-to-string**: invalid arguments are routed through `ModelBehaviorError` → formatter → `ToolCallOutputItem`. There is no explicit retry signal beyond the message text ("Please try again with valid JSON") (`src/agents/tool.py:1609-1618`).
- **No unified schema versioning**: schemas are JSON Schema fragments built on demand; there is no shared `schema_version` field, so consumers cannot distinguish SDK revisions from Pydantic revisions at the schema level.
- **`Mapping` types are not strict-safe**: declaring a `Mapping[str, int]` parameter raises `UserError` from `ensure_strict_json_schema`, forcing callers to use concrete `dict` annotations in strict mode (`src/agents/strict_schema.py:51-64`, `tests/test_function_schema.py:434-442`).

## Failure Modes / Edge Cases

- **Non-object JSON input** (`[]`, `"value"`, `123`, `null`, `true`) is rejected by `_parse_function_tool_json_input` with `ModelBehaviorError("Invalid JSON input for tool ... expected a JSON object")` (`src/agents/tool.py:1595-1598`, `tests/test_function_tool.py:758-782`).
- **Bad JSON** keeps raw payload in `ModelBehaviorError.__cause__.doc` when `DONT_LOG_TOOL_DATA=False`, but suppresses it when the flag is on (`src/agents/tool.py:1583-1593`, `tests/test_function_tool.py:786-844`).
- **Missing required argument** raises `ModelBehaviorError` from Pydantic and is funnelled into the formatter or re-raised when `failure_error_function=None` (`src/agents/tool.py:1989-1990`, `tests/test_function_tool.py:150-155`, `tests/test_run_step_execution.py:2857-2876`).
- **Empty input string**: `parse_function_tool_json_input` treats `""` as `{}` (`src/agents/tool.py:1579`); the Pydantic validator then surfaces a clear `ValidationError` for required fields (`tests/test_function_tool.py:402-409`).
- **Tool name mismatch** (`tool_not_found_behavior="raise_error"` by default, `"return_error_to_model"` opt-in): model sees a `function_call_output` describing the missing tool when configured (`src/agents/run_internal/turn_resolution.py:1958-1970`, `tests/test_agent_runner.py:4459-4534`).
- **`strict_json_schema=True` on un-strictifiable types**: `FunctionTool(..., strict_json_schema=True, params_json_schema=...)` calls `ensure_strict_json_schema` on whatever dict you pass; passing `{"type": "object", "additionalProperties": True}` raises `UserError` (`src/agents/strict_schema.py:51-64`, `tests/test_strict_schema.py:48-57`).
- **Non-first-position context param**: a `RunContextWrapper` after the first parameter raises `UserError` in `function_schema()` (`src/agents/function_schema.py:306-315`, `tests/test_function_schema.py:392-400`).
- **MCP strictify failure**: schemas that cannot be made strict are kept in their original (non-strict) form, with a `logger.info` note (`src/agents/mcp/util.py:534-545`).
- **`Optional[T]` / nullable fields**: handled implicitly via `default: None` strip; `Function[None]` types require `strict_json_schema=False` in some scenarios (no direct test found for `Union[T, None]` strict normalization, see Evidence / Failure modes for related strict-mode constraints).
- **Async tool timeouts**: `timeout_behavior="error_as_result"` (default) returns a model-visible string; `="raise_exception"` raises `ToolTimeoutError` (`src/agents/tool.py:439-447`, `tests/test_function_tool.py:896-987`).
- **Output schema strict failure**: `AgentOutputSchema(SomeType, strict_json_schema=True)` with a non-strictifiable type raises `UserError` (`src/agents/agent_output.py:112-120`).
- **Cancellation hooks**: `asyncio.CancelledError` is intercepted, the failure formatter is invoked when present, and a `_FunctionToolCancelledError` adapter keeps the public `Exception` contract (`src/agents/tool.py:1657-1692`, `tests/test_function_tool.py:847-869`).

## Future Considerations

- A per-tool `additionalProperties: False` semantics are strict-mode only; relaxations currently require turning off strict mode wholesale. A feature like a `runtime_strict=True` argument that locks the wire schema but uses a permissive server-side schema for non-OpenAI providers could widen portability (`src/agents/tool.py:1907`, `src/agents/strict_schema.py:51-64`).
- A shared `schema_version` or `sdk_version` annotation in emitted schemas would help downstream consumers reason about breaking changes, especially because Pydantic versions and `griffe` updates can shift field ordering or default handling.
- The current path makes `default: None` lossy. Future OpenAI strict-mode changes around nullable defaults could be tracked behind an explicit `keep_null_default: bool` flag rather than a silent strip (`src/agents/strict_schema.py:121-122`).
- Explicit "retry" semantics (e.g., tagging which kinds of errors should bounce back to the model versus fail the run) are currently implicit in the message text. A structured `ToolErrorKind` enum on `ModelBehaviorError` or alongside `tool_error_formatter` would make call sites easier to reason about (`src/agents/run_config.py:308-312`).
- The `failure_error_function` chain is resolved at invoke time; for high-throughput systems, memoizing or batching might reduce per-call cost.
- For non-OpenAI providers with stricter `additionalProperties: true` semantics, schema-level guidance (or per-tool opt-out of strict mode without losing validation) would avoid forcing a global `strict_json_schema=False` (`src/agents/strict_schema.py:51-64`, `src/agents/tool.py:1907`).
- The decoupling between `function_schema()` generation and the strict transform is clean, but consumers authoring their own schemas still need to know to call both. Lifting `ensure_strict_json_schema` into a single helper used by the public SDK entry points (`function_tool`, `as_tool`, MCP `to_function_tool`) reduces drift (`src/agents/agent.py:577-583`, `src/agents/extensions/experimental/codex/codex_tool.py:322`, `src/agents/mcp/util.py:528-545`, `src/agents/function_schema.py:410-412`).

## Questions / Gaps

- **Are non-OpenAI providers exercised in tests?** No evidence found in this source for third-party providers receiving `params_json_schema` verbatim - the wire calls exist only for Responses (`src/agents/models/openai_responses.py:1962-1977`), Chat Completions (`src/agents/models/chatcmpl_converter.py:864-879`), Realtime (`src/agents/realtime/openai_realtime.py:1543-1550`), and Codex (`src/agents/extensions/experimental/codex/codex_tool.py:322`). Searched under `src/agents/extensions/models/` and `src/agents/models/`; only OpenAI-backbone files consume `params_json_schema`. `litellm_model.py` and `any_llm_model.py` exist but consume via inherited model classes without direct `params_json_schema` use; how those models re-shape strict-mode schemas for downstream providers is not visible from this source.
- **Multi-tenant strict-mode downgrade**: the docs note "If a schema cannot be converted, the original schema is used" for MCP (`docs/mcp.md:49-54`), but no centralized helper exists for `@function_tool` callers who hit `UserError` from `ensure_strict_json_schema`; they must discover and reapply with `strict_mode=False`.
- **No schema-level versioning metadata**: searches for `schema_version`, `sdk_version`, or `version` annotations next to emitted JSON Schema returned no hits in `src/agents/function_schema.py`, `src/agents/strict_schema.py`, or `src/agents/tool.py`.
- **Schema immutability across `__copy__` and `dataclasses.replace`**: `FunctionTool.__copy__` re-copies fields via `dataclasses.replace` (`src/agents/tool.py:513-523`); whether `params_json_schema` is shared or deepcopied across replacements is not asserted by a dedicated test in the snippets inspected.
- **Validation error message leakage**: when `DONT_LOG_TOOL_DATA=False`, the `ModelBehaviorError.__cause__` retains the JSON payload (`tests/test_function_tool.py:840-844`); whether all exception chains (e.g. across guards or handoffs) similarly redact is not directly tested for tool input paths.
- **Portability of `strict_json_schema=False` schemas across providers**: no integration test in the repo demonstrates emitting a non-strict schema to, for example, an open-source model provider with stricter or looser JSON Schema acceptance. The wiring (`strict=false`) is present (`src/agents/models/openai_responses.py:1971`, `src/agents/models/chatcmpl_converter.py:877`) but no tests appear to verify downstream consumer behavior.

---

Generated by `04.02-tool-schema-generation-and-validation` against `openai-agents-sdk`.

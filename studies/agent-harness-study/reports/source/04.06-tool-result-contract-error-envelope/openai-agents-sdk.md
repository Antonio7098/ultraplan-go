# Source Analysis: openai-agents-sdk

## 04.06 Tool Result Contract and Error Envelope

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (asyncio, Pydantic, OpenAI Responses/Chat Completions APIs) |
| Analyzed | 2026-08-23 |

## Summary

The OpenAI Agents SDK implements a layered tool-result contract. At the center is a dual-shape output convention: tools may return plain strings (stringified via `str()`) or structured output objects (`ToolOutputText`, `ToolOutputImage`, `ToolOutputFileContent` and their TypedDict variants), which are converted into provider-native content parts (`input_text`/`input_image`/`input_file`) at serialization time. A `FunctionToolResult` dataclass pairs the logical output with the provider raw item, nested-agent interruptions, and custom data. Errors are deliberately converted into model-visible feedback rather than crashes: a per-tool `failure_error_function` (defaulting to a generic retry-oriented message that includes `str(error)`) turns exceptions into tool-output strings; timeouts default to `timeout_behavior="error_as_result"`; approval rejections become typed rejection items (shell rejections even carry `exit_code: 1`); and programmatic ("program caller") tool errors are encoded as schema-compatible JSON `{"error": ...}` objects. Redaction is applied to traces and logs (`trace_include_sensitive_data=False` replaces error details with "Tool execution failed. Error details are redacted.") while the model-facing envelope intentionally keeps actionable text. Large results are handled by an opt-in `ToolOutputTrimmer` input filter (sliding-window previews of old turns) plus mandatory truncation in the shell/sandbox stack (token/byte budgets with explicit `…N tokens truncated…` markers). Results are replayable: raw items are keyed by `call_id`, persisted (including SDK-only `custom_data`) in `RunState`, and orphaned calls are pruned to keep replays protocol-valid.

## Rating

**8 / 10** — The contract is explicit, documented (`docs/tools.md:444-459`, `docs/tools.md:521-598`), and heavily tested (dedicated suites for failure formatting, redaction, trimming, MCP error mapping). Operational safeguards are real: schema-validated outputs (`src/agents/items.py:861-900`), trace/log redaction (`src/agents/util/_tool_errors.py:5-14`), timeout envelopes (`src/agents/tool.py:2118-2171`), and truncation (`src/agents/extensions/tool_output_trimmer.py:87-135`). It falls short of 9–10 because: (1) there is no single universal success/failure envelope — success/failure signaling differs per tool family (plain string vs `{"error": ...}` JSON vs shell `outcome.exit_code`), so generic consumers cannot uniformly distinguish them; (2) the default model-visible error embeds raw `str(error)` (`src/agents/tool.py:1872`), which aids self-correction but can leak internals to the model; and (3) large-result summarization is opt-in rather than a default pipeline stage.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Structured output models | `ToolOutputText`, `ToolOutputImage` (requires image_url or file_id), `ToolOutputFileContent` (requires one of file_data/file_url/file_id) + TypedDict variants | `src/agents/tool.py:205-276` |
| Union type adapter for dict outputs | `ValidToolOutputPydanticModelsTypeAdapter` validates dict-shaped outputs | `src/agents/tool.py:279-282` |
| Tool invocation contract | `FunctionTool.on_invoke_tool` doc: return structured types, string, list, or `str()`-able; raise or return error string | `src/agents/tool.py:455-466` |
| Result wrapper | `FunctionToolResult` dataclass: tool, output, run_item (None when interrupted), interruptions, agent_run_result | `src/agents/tool.py:374-392` |
| Output item wrapper | `ToolCallOutputItem`: raw_item + typed `output` + SDK-only `custom_data` (not replayed to model) | `src/agents/items.py:430-452` |
| Output schema enforcement | `output_type`/`output_json_schema` must be strict JSON object schema; UserError otherwise | `src/agents/tool.py:2193-2237` |
| Output serializer | `ItemHelpers.tool_call_output_item`: TypeAdapter validation, JSON-object enforcement, structured conversion | `src/agents/items.py:845-923` |
| Structured conversion | `_convert_tool_output_as_structured` / per-type mapping to `input_text`/`input_image`/`input_file`; empty-list guard | `src/agents/items.py:946-1033` |
| Chat Completions serializer | Tool outputs become tool messages; non-text content dropped unless `preserve_tool_output_all_content=True` (placeholder or strict UserError) | `src/agents/models/chatcmpl_converter.py:819-855` |
| Error formatter contract | `ToolErrorFunction = Callable[[RunContextWrapper, Exception], str]` | `src/agents/tool.py:196` |
| Default error envelope | `default_tool_error_function`: "An error occurred while running the tool. Please try again. Error: {str(error)}"; JSON-parse errors get targeted guidance | `src/agents/tool.py:1863-1872` |
| Failure interception | `_FailureHandlingFunctionToolInvoker.__call__` catches Exception, invokes failure formatter, returns string or re-raises | `src/agents/tool.py:653-667` |
| Formatter resolution | `resolve_function_tool_failure_error_function`: default formatter disabled for schema-backed programmatic calls (free text would violate schema) | `src/agents/tool.py:1902-1911` |
| Cancellation envelope | CancelledError adapted to Exception for formatter; cancellation converted to model-visible string with redacted span error | `src/agents/tool.py:1944-1981`, `src/agents/run_internal/tool_execution.py:2069-2095` |
| Timeout envelope | `timeout_behavior="error_as_result"` (default) returns model-visible message; `raise_exception` raises `ToolTimeoutError` | `src/agents/tool.py:499-507`, `src/agents/tool.py:2144-2171` |
| Programmatic error encoding | `function_tool_error_output` wraps SDK-generated errors as `{"error": ...}` JSON for program-caller calls | `src/agents/run_internal/items.py:803-821` |
| Rejection envelopes | `function_rejection_item` ("Tool execution was not approved."), shell rejection (stderr + exit_code 1), apply_patch rejection (status failed) | `src/agents/run_internal/items.py:824-899`, `src/agents/tool.py:194` |
| Run-level error formatter | `RunConfig.tool_error_formatter` with `ToolErrorFormatterArgs` (kinds: approval_rejected, tool_not_found) | `src/agents/run_config.py:83-105` |
| Unhandled failures | Non-AgentsException tool failures re-raised as `UserError(f"Error running tool {name}: {e}")` after span annotation | `src/agents/run_internal/tool_execution.py:1839-1852` |
| Trace redaction | `get_trace_error` / `REDACTED_TOOL_ERROR_MESSAGE` = "Tool execution failed. Error details are redacted." | `src/agents/util/_tool_errors.py:5-14`, `src/agents/util/_error_tracing.py:11,46-53` |
| Redacted exception scrubbing | `_prepare_data_redacted_error` / `_discard_exception_graph`: clears args, tracebacks, `__cause__`, `__context__` at public boundaries | `src/agents/exceptions.py:52-53,181-215,315-405` |
| Safe stringification | `_model_error_text` avoids invoking side-effecting `__str__` when output would be discarded by redaction | `src/agents/util/_error_tracing.py:70-86` |
| MCP error mapping | `invoke_mcp_tool` re-raises MCP/user/cancellation errors into the failure pipeline; wraps others in `AgentsException` with server name | `src/agents/mcp/util.py:703-762` |
| MCP formatter override | Server-level `failure_error_function` overrides agent default; `_UNSET` sentinel semantics | `src/agents/mcp/server.py:549-578,848-854` |
| Large-result trimming (opt-in) | `ToolOutputTrimmer` input filter: recent-turn window, `[Trimmed: tool — N chars → M char preview]`, structured-part-aware trimming, non-mutating | `src/agents/extensions/tool_output_trimmer.py:87-135,136-202,243-336` |
| Shell output truncation | `truncate_shell_outputs` honors `max_output_length`; `normalize_max_output_length` clamps negatives | `src/agents/run_internal/tool_execution.py:1002-1039,1077-1080` |
| Token/byte truncation | `TruncationPolicy` (bytes/tokens), head+tail split, explicit markers `…N tokens truncated…` / `…N chars truncated…`, "Total output lines: N" prefix | `src/agents/sandbox/util/token_truncation.py:11-47,89-113,186-206` |
| Artifact references | `ToolOutputImage.file_id` / `ToolOutputFileContent.file_id/file_url/file_data` link to stored artifacts instead of inlining | `src/agents/tool.py:219-266`, `src/agents/items.py:1008-1031` |
| Sandbox view_image | Returns `ToolOutputImage` with base64 data URL for sandbox-hosted images | `src/agents/sandbox/capabilities/tools/view_image.py:112-158` |
| Replayability | `tool_output_identity` maps outputs to (invocation type, call_id); orphan function calls pruned for valid replay; `custom_data` persisted/restored in RunState | `src/agents/_tool_invocation.py:298-316`, `src/agents/run_state.py:1984-1989,5250-5262`, `tests/test_run_internal_items.py:64-121` |
| Redaction tests | `test_function_tool_error_trace_respects_sensitive_data_setting` asserts redacted span text and model-visible fallback | `tests/test_run_step_execution.py:654-724` |
| Programmatic error tests | `test_programmatic_structured_tool_errors_are_encoded_as_json_objects` / `test_direct_function_tool_errors_preserve_plain_text` | `tests/test_run_internal_items.py:32-74` |
| Failure formatter tests | Default resolution, cancellation normalization, `dataclasses.replace` survival, timeout-not-rewritten cases | `tests/test_function_tool.py:698-800,1220,1324-1341` |
| Timeout-as-result test | `test_execute_approved_tools_timeout_returns_error_as_result` | `tests/test_agent_runner.py:7537` |
| MCP error tests | Default error hides URL credentials; server override/None semantics | `tests/mcp/test_mcp_util.py:1264,1489-1554` |
| Trimmer tests | Allowlists, multiple old outputs trimmed, structured outputs | `tests/extensions/test_tool_output_trimmer.py:68-111,549-571,988` |
| Docs | Tool output types, timeouts (`error_as_result` default), `failure_error_function` customization | `docs/tools.md:444-459,521-598` |

## Answers to Dimension Questions

1. **Is every tool result structured?**
   Hybrid by design. Function tools accept free-form strings as a first-class path (`str()` fallback, `src/agents/items.py:916,951`), but structured outputs are fully supported: Pydantic `ToolOutputText/Image/FileContent` models, their TypedDict variants, and lists of them (`src/agents/tool.py:205-282`); dict-shaped outputs are validated against the union adapter (`src/agents/items.py:990-998`). Tools may declare `output_type`/`output_json_schema`, after which every result is validated against a strict object schema before serialization (`src/agents/items.py:861-900`, `src/agents/tool.py:2193-2237`). Hosted/shell/custom/computer tools each have provider-typed envelopes (`ToolCallOutputTypes`, `src/agents/items.py:420-427`). So: not uniformly structured, but structured where declared, and always wrapped in a typed run item (`ToolCallOutputItem`, `src/agents/items.py:430-452`).

2. **Can the model distinguish success from failure?**
   Yes, through content rather than a universal status field. Failures become model-visible strings via the default formatter ("An error occurred while running the tool. Please try again. Error: ...", `src/agents/tool.py:1863-1872`); programmatic calls receive JSON `{"error": ...}` (`src/agents/run_internal/items.py:803-821`); timeouts produce a dedicated message ("Tool 'x' timed out after N seconds.", `src/agents/tool.py:1881-1883`); rejections produce "Tool execution was not approved." (`src/agents/tool.py:194`); shell results carry structured `outcome.exit_code`/timeout status (`src/agents/run_internal/tool_execution.py:887-999`). There is no cross-family boolean/sentinel envelope, so a model must parse per-tool conventions.

3. **Are stack traces hidden from users/models?**
   Models never receive tracebacks — only `str(error)` inside the default formatter message. Traces and logs are redacted when `trace_include_sensitive_data=False`: span errors read "Tool execution failed. Error details are redacted." (`src/agents/util/_tool_errors.py:5-14`), asserted in `tests/test_run_step_execution.py:655-724`. Redaction goes deep: public-boundary errors are replaced with payload-free exceptions whose args, tracebacks, `__cause__`, and `__context__` are scrubbed (`src/agents/exceptions.py:181-215,315-405`), and redaction avoids even invoking provider `__str__` when the text would be discarded (`src/agents/util/_error_tracing.py:70-86`). Caveat: the *developer-facing* exception (`UserError(f"Error running tool {name}: {e}")`, `src/agents/run_internal/tool_execution.py:1852`) intentionally retains the raw message, and the *model-facing* default formatter includes raw `str(error)` (`src/agents/tool.py:1872`) — a deliberate self-correction tradeoff, not an accident (tests assert both, `tests/test_run_step_execution.py:672,713`).

4. **Are large results handled safely?**
   Yes, at multiple layers, though the general-purpose layer is opt-in. `ToolOutputTrimmer` (`src/agents/extensions/tool_output_trimmer.py:87-135`) replaces old-turn outputs with bounded previews without mutating originals (`:136-142`), understands structured `input_text/image/file` parts and drops opaque parts with counts (`:272-390`), and trims tool-search payloads including schema prose (`:392-495`). Mandatory safeguards exist in the shell/sandbox stack: `max_output_length` clamping (`src/agents/run_internal/tool_execution.py:1002-1039`) and token/byte truncation with explicit markers and head+tail preservation (`src/agents/sandbox/util/token_truncation.py:89-113,186-206`). The Chat Completions adapter drops non-text content unless explicitly preserved (`src/agents/models/chatcmpl_converter.py:825-849`). There is no default global output-size cap for arbitrary function-tool returns.

5. **Can tool results be replayed?**
   Yes. Raw output items are `call_id`-keyed provider dicts (`src/agents/items.py:918-923`), matched back to invocations by `tool_output_identity` (`src/agents/_tool_invocation.py:298-316`), persisted in `RunState` including SDK-only `custom_data` and `tool_origin` (`src/agents/run_state.py:1984-1989,5250-5262`), and replay pruning removes orphan function calls so resumed histories stay protocol-valid (`tests/test_run_internal_items.py:64-179`). The trimmer only rewrites the model input at call time, never the persisted items (`src/agents/extensions/tool_output_trimmer.py:136-142`).

## Architectural Decisions

1. **Strings-first output contract with opt-in structure.** The `on_invoke_tool` contract accepts anything stringifiable, while structured outputs (`ToolOutput*`, declared schemas) are converted/validated at the serialization boundary (`src/agents/tool.py:455-466`, `src/agents/items.py:845-923`). This keeps tool authoring trivial while giving strict guarantees when declared.

2. **Errors-as-results over crashes.** The default pipeline converts tool exceptions into model-visible strings via `failure_error_function` (`src/agents/tool.py:653-667,1863-1872`), so a failed tool call becomes recovery feedback; `failure_error_function=None` restores fail-the-run semantics (`tests/test_run_step_execution.py:626-651`). Timeouts default to the same philosophy (`src/agents/tool.py:499-507`).

3. **Dual-audience redaction policy.** Redaction protects traces/logs/telemetry, not the model: `trace_include_sensitive_data=False` scrubs span data (`src/agents/util/_tool_errors.py:8-14`) while the model envelope keeps actionable text (`src/agents/tool.py:1872`). The AGENTS.md guidance explicitly considers `__context__`, traceback frames, and chained exceptions in redaction design (repo `AGENTS.md`, "When redacting OpenAI tool, MCP, model, or provider payloads...").

4. **Provider-shape fidelity.** Outputs are serialized into provider-native shapes (`input_text`/`input_image`/`input_file`, `src/agents/items.py:1003-1033`) and program-caller errors are re-encoded as schema-conforming JSON so strict programmatic contracts are never violated by free text (`src/agents/run_internal/items.py:803-821`; default formatter disabled for such calls, `src/agents/tool.py:1902-1911`).

5. **Separation of model-visible and SDK-only data.** `custom_data` on `ToolCallOutputItem` is explicitly not replayed to the model but is persisted for applications (`src/agents/items.py:447-452`, `src/agents/run_state.py:1987-1989`).

## Notable Patterns

- **Sentinel-based formatter resolution:** `_UNSET_FAILURE_ERROR_FUNCTION` distinguishes "user passed None" (raise) from "not configured" (default formatter) (`src/agents/tool.py:202,1886-1911`).
- **Cancellation-as-Exception adapter:** `_FunctionToolCancelledError` preserves the public `ToolErrorFunction` Exception contract while carrying the original `CancelledError` (`src/agents/tool.py:1944-1961`), with dedicated tests for cancelled siblings (`tests/test_run_step_execution.py:813-988`).
- **Failure arbitration in parallel batches:** `_FunctionToolFailure` orders concurrent failures by priority (Cancelled < Exception < BaseException) then call order, so the root cause surfaces rather than a cancellation shadow (`src/agents/run_internal/tool_execution.py:180-304`).
- **Envelope-per-tool-family rejections:** shell rejections synthesize a realistic failure envelope (`stderr` + `exit_code: 1`, `src/agents/run_internal/items.py:852-872`) instead of a bare string, keeping the provider payload type-valid.
- **Non-destructive context-window management:** trimming happens in a `call_model_input_filter` just before each model call, leaving persisted history intact (`src/agents/extensions/tool_output_trimmer.py:1-26,136-202`).
- **Defense-in-depth redaction:** exception-graph scrubbing clears nested state reachable through args, groups, and instance dicts to prevent payload leakage via `__context__` (`src/agents/exceptions.py:315-405`).

## Tradeoffs

- **Self-correction vs leakage:** the default model-visible error includes raw `str(error)` (`src/agents/tool.py:1872`). This maximizes the model's ability to retry correctly but can expose internal details (paths, tokens in messages) to the LLM — a prompt-injection/exfiltration surface applications must handle with a custom `failure_error_function`.
- **Flexibility vs uniformity:** accepting "anything `str()`-able" (`src/agents/tool.py:461-466`) makes tool authoring easy but means downstream code cannot assume a structured success/failure envelope across tools.
- **Opt-in trimming vs default safety:** `ToolOutputTrimmer` is powerful but must be explicitly added to `RunConfig.call_model_input_filter` (`src/agents/extensions/tool_output_trimmer.py:8-20`); runs without it can still accumulate unbounded old outputs.
- **Provider-shape fidelity vs complexity:** maintaining distinct envelopes for function, custom, shell, local-shell, apply_patch, and computer tools (e.g., `src/agents/run_internal/items.py:803-899`) yields valid provider payloads at the cost of many parallel code paths.
- **Strict schema enforcement vs runtime failures:** schema-violating tool outputs raise `UserError` at serialization time (`src/agents/items.py:868-871,894-895`) — surfacing bugs early but converting a soft failure into a run failure.

## Failure Modes / Edge Cases

- **Model-visible leakage:** raw exception text reaches the model by default (asserted in `tests/test_run_step_execution.py:713`).
- **Non-text-only outputs on Chat Completions:** image/file-only results are dropped and replaced by a placeholder unless `preserve_tool_output_all_content=True` (`src/agents/models/chatcmpl_converter.py:825-849`) — a silent-fidelity-loss mode (warned, or raised under strict validation).
- **Empty structured output lists:** an empty list would otherwise emit an empty structured-output list and drop the tool result; guarded to stringify instead (`src/agents/items.py:966-971`).
- **Timeout races:** if the underlying task completes exactly at the deadline, its real result/exception wins over the synthetic timeout (`src/agents/tool.py:2144-2149`); tool-raised `ToolTimeoutError`s are never rewritten (`tests/test_function_tool.py:1324-1341`).
- **Late sibling failures:** background tasks failing after failure propagation are reported via loop exception handlers with policy-specific messages instead of being swallowed (`src/agents/run_internal/tool_execution.py:207-255`).
- **MCP cancellation:** inner cancellations are converted into `MCPToolCancellationError` and then formatted by the failure pipeline (`src/agents/mcp/util.py:711-732`; `tests/mcp/test_mcp_util.py:970-1036`).
- **Malformed provider payloads:** missing `call_id` on shell/apply-patch calls raises `ModelBehaviorError` before execution (`src/agents/run_internal/tool_execution.py:638-643,767-772`); tool-call IDs are guarded against malformed non-OpenAI inputs (`src/agents/run_internal/tool_execution.py:626-635`).
- **Redaction vs catchability:** redacted cancellations remain catchable via a payload-free `CancelledError`/`Exception` hybrid (`src/agents/exceptions.py:40-41`; `tests/test_error_logging_redaction.py:1353`).

## Future Considerations

- Add an optional universal status envelope (e.g., `{"ok": bool, "error": ...}`) or a documented convention so generic orchestrators can branch on success/failure without per-tool parsing.
- Consider making result-size guarding a default pipeline stage (a conservative built-in truncation) with `ToolOutputTrimmer` as the configurable superset, rather than purely opt-in.
- Provide a supported redaction mode for model-visible errors (e.g., `failure_error_function` preset that strips exception internals) so security-sensitive apps need not hand-write formatters; today the safe path is custom code (`src/agents/tool.py:196,1863-1872`).
- Generalize artifact linking beyond OpenAI `file_id`/data URLs (`src/agents/tool.py:219-266`) to an artifact-store abstraction so large binary results can be referenced portably across model providers.

## Questions / Gaps

- **No evidence found** for a global, always-on output-size limit for arbitrary function-tool returns; the searched boundary was `src/agents/` (tool.py, run_internal/tool_execution.py, extensions/tool_output_trimmer.py, sandbox/util/token_truncation.py) and `docs/tools.md`. Truncation exists only for shell/sandbox stacks and the opt-in trimmer.
- **No evidence found** for stack traces being included in any model-facing envelope; the model channel carries only formatter strings (`src/agents/tool.py:1863-1872`) — verified by reading the formatter and serializer paths, not by runtime observation.
- The exact upstream release provenance of the analyzed checkout was taken from the cloned `main` HEAD (`2334679`, "fix: enforce public type alias contract coverage (#4595)"); behavior may differ in older published versions.
- Sandbox `Dir`/`File` artifact entries (`src/agents/sandbox/entries/artifacts.py:52-89`) are workspace-materialization constructs, not tool-result artifact links; if the dimension intends "artifact store for results," that concept is only partially present (file_id/data-URL references) — stated explicitly to avoid over-claiming.

---

Generated by `dimensions/04.06-tool-result-contract-and-error-envelope` against `openai-agents-sdk`.

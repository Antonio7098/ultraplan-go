# Source Analysis: openai-agents-sdk

## 03.03 Tool-Calling Roundtrip Control

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+ / asyncio / Pydantic / OpenAI Responses API |
| Analyzed | 2026-07-14 |

## Summary

The OpenAI Agents SDK implements a structured, recoverable tool-calling roundtrip
centered on the Responses API. A `ModelResponse` arrives with heterogeneous output
items (`src/agents/run_internal/turn_resolution.py:1551-1949`); each one is
classified and routed into a dedicated run container (`ToolRunFunction`,
`ToolRunComputerAction`, `ToolRunShellCall`, `ToolRunCustom`,
`ToolRunApplyPatchCall`, `ToolRunLocalShellCall`, `ToolRunMCPApprovalRequest`) in
`src/agents/run_internal/run_steps.py:62-130`. Function tools are dispatched in
parallel inside `_FunctionToolBatchExecutor`
(`src/agents/run_internal/tool_execution.py:1358-1970`), which encapsulates a
module-level asyncio task graph, an arbitration policy for concurrent failures
(`_FunctionToolFailure`, `tool_execution.py:173-289`), and bounded post-invoke
and cancellation drains. Arguments are JSON-string parsed with a guarded helper
(`_parse_function_tool_json_input`, `src/agents/tool.py:1575-1598`) and then
Pydantic-validated against a strict JSON schema that is computed from the
function signature (`src/agents/function_schema.py:224-424`,
`src/agents/strict_schema.py:18-149`). Outputs are normalized through structured
mappers (`ToolOutputText`, `ToolOutputImage`, `ToolOutputFileContent`,
`src/agents/tool.py:190-267`) and converted into a `ToolCallOutputItem`
(`src/agents/items.py:389-449`) whose `raw_item` matches the Responses API
shape (`function_call_output`, `shell_call_output`, `apply_patch_call_output`,
`computer_call_output`, `custom_tool_call_output`).
Approvals are first-class: tool definitions carry `needs_approval` policies
(`src/agents/tool.py:426-433`, `src/agents/tool.py:1221-1231`,
`src/agents/tool.py:1269-1279`, `src/agents/tool.py:1300-1303`), and the runner
emits `ToolApprovalItem` interruptions (`src/agents/items.py:508-580`) that flow
through HITL state. Errors are surfaced via `ModelBehaviorError` for malformed
input and `default_tool_error_function` for runtime failure, both retained as
model-visible strings (`src/agents/tool.py:1575-1618`,
`src/agents/run_internal/tool_execution.py:1127-1180`). Concurrency is bounded
by `RunConfig.tool_execution.max_function_tool_concurrency`
(`src/agents/run_config.py:96-118`). The system is mature: validation is strict,
recovery is built into the executor, and parallel failures are arbitrated
deterministically.

## Rating

**8 / 10** — Clear, durable, exercised by tests. Strict JSON schemas, Pydantic
argument validation, parallel execution with deterministic arbitration, bounded
timeout, structured output variants, and a full HITL approval path. The schema
is not extensible without consumer signoff (no pluggable repair pipeline for
malformed tool calls — failures terminate the turn via `ModelBehaviorError` or
inject a recovery string and continue). Approval state is mostly in-memory on
`RunContextWrapper`, with explicit serialization only via `RunState`.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Tool call extraction (response items -> run containers) | `process_model_response` walks `response.output` and dispatches each item type into a dedicated `ToolRunXxx` container | `src/agents/run_internal/turn_resolution.py:1551-1949` |
| Tool-call lookup / identity | `get_function_tool_lookup_key_for_call` resolves call_id + optional namespace into a `function_tool_map` entry | `src/agents/_tool_identity.py` (imported at `turn_resolution.py:30-41`) |
| Argument JSON parsing (guarded) | `_parse_function_tool_json_input` validates JSON and raises `ModelBehaviorError` with redacted payload | `src/agents/tool.py:1575-1598` |
| Pydantic argument validation against strict schema | `schema.params_pydantic_model(**json_data)` runs after ensure-strict-schema | `src/agents/tool.py:1984-1990`, `src/agents/function_schema.py:407-410`, `src/agents/strict_schema.py:18-149` |
| Function-tool timeout / async wrapping | `invoke_function_tool` schedules with `asyncio.wait_for`, falls back to formatter or raises `ToolTimeoutError` | `src/agents/tool.py:1806-1847` |
| Timeout config validation at construction | `_validate_function_tool_timeout_config` ensures positive finite number, async-only | `src/agents/tool.py:2066-2091` |
| Output normalization (text/image/file/structured) | `ToolOutputText`, `ToolOutputImage`, `ToolOutputFileContent`; `ItemHelpers._convert_tool_output` | `src/agents/tool.py:190-267`, `src/agents/items.py:802-874` |
| Output item serialization (`function_call_output`) | `ItemHelpers.tool_call_output_item` builds `{call_id, output, type}` per Responses API | `src/agents/items.py:783-799` |
| Tool-call output item that gets replayed to the model | `ToolCallOutputItem.to_input_item` strips SDK-only fields (`status`, `shell_output`, `provider_data`) | `src/agents/items.py:421-449` |
| Parallel function-tool batch executor | `_FunctionToolBatchExecutor` with concurrency cap, sibling cancellation, post-invoke drain, deterministic failure arbitration | `src/agents/run_internal/tool_execution.py:1358-1970` |
| Deterministic failure arbitration | `_FunctionToolFailure`, `_select_function_tool_failure`, `_merge_late_function_tool_failure` policies | `src/agents/run_internal/tool_execution.py:173-289` |
| Tool-call ID extraction (defensive against malformed payloads) | `extract_tool_call_id` accepts dicts or objects and returns `None` when missing | `src/agents/run_internal/tool_execution.py:597-606` |
| Disabled-tool refusal | Disabled FunctionTool raises `ModelBehaviorError` rather than silent skip | `src/agents/run_internal/tool_execution.py:1407-1417` |
| Approval flow: pre-approval guardrails + `ToolApprovalItem` interruption | `_maybe_execute_tool_approval` issues pending approval, intercepts rejection, runs input guardrails first when configured | `src/agents/run_internal/tool_execution.py:1629-1736` |
| Approval signature for function tools | `needs_approval` accepts bool or `(run_context, tool_params, call_id) -> bool` | `src/agents/tool.py:426-433` |
| Tool-approval state object | `ToolApprovalItem` carries agent, raw_item, tool_name, namespace, tool_origin, lookup key, bare-name alias flag | `src/agents/items.py:508-580` |
| Approval decision storage on RunContext | `approve_tool`, `reject_tool`, `get_approval_status` resolve namespaces and lookup keys | `src/agents/run_context.py:355-445` |
| Resume-from-approval (HITL) | `execute_approved_tools` reuses `_FunctionToolBatchExecutor` after `RunState.approve()` | `src/agents/run_internal/tool_execution.py:2145-2330` |
| Tool-error formatter hook for model-visible errors | `RunConfig.tool_error_formatter` (`ToolErrorFormatterArgs` with `kind="approval_rejected" \| "tool_not_found"`) | `src/agents/run_config.py:69-92`, used at `tool_execution.py:1149-1180` and `turn_resolution.py:217-256` |
| Default failure-formatter friendlier for JSON-decode errors | `default_tool_error_function` produces a parsing-args message for JSONDecodeError chains | `src/agents/tool.py:1609-1618` |
| Tool-not-found fallback that *injects* output rather than crash when configured | `_build_tool_not_found_output_items` + `tool_not_found_behavior` mode "return_error_to_model" | `src/agents/run_internal/turn_resolution.py:213-274` |
| Synthetic model-visible output on approval rejection | `function_rejection_item`, `shell_rejection_item`, `apply_patch_rejection_item` | `src/agents/run_internal/items.py:349-409` |
| Errors continue or end the loop? | Function-tool failure cancels siblings, drains, raises (caller can fail run); `default_tool_error_function` allows the loop to continue with an injected string | `src/agents/run_internal/tool_execution.py:1480-1559`; recovery path at `tool.py:526-568` |
| Tool-call input guardrails (per-tool, pre-invoke) | `_execute_tool_input_guardrails` returns rejection message or raises `ToolInputGuardrailTripwireTriggered` | `src/agents/run_internal/tool_execution.py:2337-2368` |
| Tool-call output guardrails (per-tool, post-invoke) | `_execute_tool_output_guardrails` may replace or raise based on guardrail behavior | `src/agents/run_internal/tool_execution.py:2371-2406` |
| Concurrency cap | `ToolExecutionConfig.max_function_tool_concurrency` and `pre_approval_tool_input_guardrails` flag | `src/agents/run_config.py:96-118` |
| Hook integration around tool calls | `RunHooks.on_tool_start` / `on_tool_end` and per-agent `Agent.hooks` variants | `src/agents/run_internal/tool_execution.py:1757-1764`, `tool_execution.py:1848-1855` |
| Test: invalid JSON continues the loop with a formatter message | `test_input_guardrail_runs_on_invalid_json` | `tests/test_run_step_execution.py:2818-2855` |
| Test: invalid JSON with `failure_error_function=None` raises `ModelBehaviorError` | `test_invalid_json_raises_with_failure_error_function_none` | `tests/test_run_step_execution.py:2859-2876` |
| Test: missing shell `call_id` raises `ModelBehaviorError` | `test_shell_missing_call_id_raises_model_behavior_error` | `tests/test_hitl_error_scenarios.py:745-766` |
| Test: streaming tool-call argument reconstruction | `test_streamed_tool_call_arguments.py` exists as a dedicated test module | `tests/test_streaming_tool_call_arguments.py` |
| Approvals tests (state + resume) | `tests/test_run_context_approvals.py`, `tests/test_run_internal_approvals.py` | `tests/test_run_context_approvals.py:1-233` |

## Answers to Dimension Questions

1. **How are tool calls represented?**
   Tool calls come out of the model as typed response items
   (`ResponseFunctionToolCall`, `ResponseComputerToolCall`, `ResponseCustomToolCall`,
   `LocalShellCall`, `ShellCall`, `apply_patch_call`, `McpApprovalRequest`,
   `McpCall`, `ResponseFileSearchToolCall`, `ResponseFunctionWebSearch`,
   `ResponseCodeInterpreterToolCall`, `ImageGenerationCall`,
   `ResponseToolSearchCall`). Each is normalized into a `RunItem` subclass —
   primarily `ToolCallItem` (`src/agents/items.py:347-377`) — and, for items the
   runner can execute locally, into a `ToolRun*` dataclass
   (`src/agents/run_internal/run_steps.py:62-130`). Function calls are routed by
   `get_function_tool_lookup_key_for_call` and resolved through a function-tool
   lookup map (`turn_resolution.py:1574-1949`). Outputs are returned as
   `ToolCallOutputItem` (`items.py:389-419`) whose `raw_item` payload matches
   the Responses API tool-output types.

2. **What happens if arguments are invalid?**
   Two-layer guard. Layer 1: `_parse_function_tool_json_input`
   (`src/agents/tool.py:1575-1598`) decodes the JSON; on `JSONDecodeError` or
   non-object input it raises `ModelBehaviorError` with redaction policy
   (`_debug.DONT_LOG_TOOL_DATA`). Layer 2: the schema's Pydantic model is
   instantiated from the decoded dict, raising `ValidationError` if required
   fields or types mismatch (`tool.py:1984-1990`). Either error is caught by
   `_FailureHandlingFunctionToolInvoker.__call__` (`tool.py:526-568`), which
   routes through `failure_error_function` if configured. If the user supplies
   one, the result is returned to the model as a string so the loop continues;
   if `failure_error_function` is `None`, the original `ModelBehaviorError`
   propagates up and the runner surfaces it (`tests/test_run_step_execution.py:2859-2876`).

3. **Are tool results structured?**
   Yes. Outputs can be plain strings, lists, or one of three Pydantic-validated
   structured shapes: `ToolOutputText`, `ToolOutputImage`, `ToolOutputFileContent`
   (`src/agents/tool.py:190-267`). `ItemHelpers._convert_tool_output`
   (`src/agents/items.py:802-824`) checks for structured shapes first
   (all-or-nothing for lists) before falling back to `str()`. The resulting
   `raw_item` carries typed entries (`input_text`, `input_image`, `input_file`)
   when structured and a plain string otherwise.

4. **Are tool errors visible to the model?**
   Yes, by default. The default formatter `default_tool_error_function`
   (`src/agents/tool.py:1609-1618`) returns a friendly string that the model
   consumes on the next turn; for JSON-decode failures the message is
   specifically tuned (`default_tool_error_function` returns
   "An error occurred while parsing tool arguments…"). Users can override the
   formatter per tool (`tool.py:1858-1867`) or globally via
   `RunConfig.tool_error_formatter` (used for `approval_rejected` /
   `tool_not_found` kinds, `run_config.py:69-92`,
   `tool_execution.py:1149-1180`, `turn_resolution.py:217-256`). Errors raised
   from `invoke_function_tool` are also attached to the active tracing span via
   `_error_tracing.attach_error_to_current_span` (`tool.py:1615-1620`) so traces
   capture them.

5. **Can tool calls be approved before execution?**
   Yes, with multiple control surfaces:
   - `FunctionTool.needs_approval` (`tool.py:426-433`) — bool or async callable
     evaluated per call with parsed args.
   - Per tool/call `ToolApprovalItem` placeholders are emitted as interruptions
     (`items.py:508-580`, `tool_execution.py:1686-1700`).
   - Pre-approval tool-input guardrails can run when configured
     (`run_config.ToolExecutionConfig.pre_approval_tool_input_guardrails`,
     `tool_execution.py:1658-1675`) so users see "rejected" instead of "needs
     approval" when guardrails trip on the proposed args.
   - Approvals / rejections land on `RunContextWrapper.approve_tool` /
     `reject_tool` (`run_context.py:355-375`) and are replayed by
     `RunState.approve()` / `reject()` (`run_state.py:336-353`).
   - On resume, `execute_approved_tools`
     (`run_internal/tool_execution.py:2145-2330`) re-runs only the approved calls
     through the same executor.
   - Hosted MCP calls additionally support a callback path
     (`on_approval_request`, `tool.py:970`, `tool_planning.py:99-137`).

## Architectural Decisions

- **Response-shape fidelity at the boundary.** `ToolCallOutputItem.raw_item`
  matches the Responses API tool-output shape exactly
  (`function_call_output`, `shell_call_output`, `computer_call_output`,
  `apply_patch_call_output`, `custom_tool_call_output`), so any compliance gap
  surfaces immediately. SDK-only metadata is kept out of `raw_item`
  (`items.py:421-449`) and off the wire.

- **Strict JSON-schema enforcement by default.** `function_schema` builds a
  Pydantic model from the function signature then forces a strict schema in
  `ensure_strict_json_schema` (`strict_schema.py:18-149`). `oneOf` is rewritten
  to `anyOf`, `$ref`s are inlined, `additionalProperties` is set to `false`,
  and `required` is the full property set — exactly what the Responses API
  strict-tools require.

- **Per-tool execution context that adapts.** `_get_function_tool_invoke_context`
  (`tool.py:1772-1803`) inspects the wrapper's signature annotation to decide
  whether to pass `ToolContext` (rich) or fork into `RunContextWrapper` (narrower)
  so third-party wrappers don't leak runtime-only metadata through downstream
  serializers.

- **Failure policy with a recovery string and a failure-error function.** A
  default formatter (`default_tool_error_function`) and a per-tool opt-in
  formatter let users choose between "send a string back to the model" vs
  "raise and end the turn." This is enforced at construction via
  `set_function_tool_failure_error_function` and routed through
  `maybe_invoke_function_tool_failure_error_function`
  (`tool.py:1632-1692`).

- **HITL state as a first-class interrupt.** Approvals produce
  `ToolApprovalItem` interruptions rather than triggering errors. The run
  loop checks `get_approval_status` before each tool execution
  (`tool_execution.py:1651-1736`) and routes approved/non-approved into a
  synthetic output (`function_rejection_item`, items.py:349-365) so the next
  model turn sees both kinds of outcome as ordinary function outputs.

- **Deterministic concurrent-failure arbitration.** Parallel function-tool
  execution uses an arbitration policy that prefers the "most informative"
  failure (`Exception > asyncio.CancelledError` ordering,
  `tool_execution.py:243-289`) and merges late post-invoke failures without
  masking the root cause. Cancellation drains wait up to 0.25 s for siblings
  to self-progress, then re-classify and raise
  (`_FUNCTION_TOOL_CANCELLED_DRAIN_SECONDS`).

- **Input/output guardrails as first-class per-tool hooks.**
  `tool_input_guardrails` run before invocation and can either reject the
  content or raise. `tool_output_guardrails` run after invocation and can
  replace the result (`tool_execution.py:2337-2406`).

- **Concurrency capped, not unbounded.** `RunConfig.tool_execution.max_function_tool_concurrency`
  (`run_config.py:96-118`) lets the caller dial down parallel fan-out
  without altering provider-side `parallel_tool_calls`.

## Notable Patterns

- **Two recovery paths per tool failure.** The wrapping invoker
  (`_FailureHandlingFunctionToolInvoker`, `tool.py:526-568`) plus the public
  `default_tool_error_function` together mean a tool author's choice between
  "raise the model-transparent error string" and "let it crash" maps cleanly to
  one config flag.

- **`ToolExecutionPlan` as a turn-scoped dispatch table.** The plan separates
  "needs approval" work from "execute now" work and keeps MCP callbacks,
  approval-rejection items, and run-waiting output items as parallel lists
  (`run_internal/tool_planning.py:177-300`). This makes resume logic
  straightforward — a resume builds a smaller plan that reuses the same
  executor (`run_internal/turn_resolution.py:848-1500`).

- **Strict output stripping before replay.** `ToolCallOutputItem.to_input_item`
  (`items.py:421-449`) removes `status`, `shell_output`, `provider_data` from
  hosted tool outputs because the Responses API doesn't (yet) accept them. This
  isolates wire-shape negotiation from the runner.

- **Identity-based dedupe.** `_tool_call_identity` hashes `(call_id, name,
  normalized-args)` to dedupe tool-call items across resumed turns
  (`tool_planning.py:69-174`). This keeps toolcall rematerialization safe when
  the runner re-derives items from persisted state.

- **`get_mapping_or_attr` against heterogeneous payloads.** Most helpers use
  `get_mapping_or_attr(tool_call, "X")` (`tool_execution.py:590-594`) so a tool
  call can be a Pydantic model or a dict; this is what allows serialization
  boundaries to round-trip through dict snapshots and avoids brittle
  `isinstance` ladders.

## Tradeoffs

- **Strict schema by default** improves the chance of well-formed JSON from the
  model but raises the friction for tools that want optional fields with type
  variants (`oneOf` not supported in nested strict contexts; rewritten to
  `anyOf`, `strict_schema.py:90-101`).

- **Failure surfaces as a string by default**, not an exception. That's user-
  friendly for the model but means a tool author who *wants* an exception must
  remember to pass `failure_error_function=None` (and the SDK's default
  tests rely on that, `tests/test_run_step_execution.py:2859-2876`).

- **Approvals live on the in-memory `RunContextWrapper`** by default.
  Persistence requires explicit `RunState.approve()` /
  `RunState.reject()`/`save_resumed_turn_items` (referenced at
  `run_state.py:336-353`, `session_persistence.py`). The SDK does not
  auto-serialize approval state into `Session`.

- **Parallel executor aggressively cancels siblings on first failure.**
  `_raise_failure_after_draining_siblings` cancels in-flight tasks and waits up
  to 0.25 s for self-progress (`_FUNCTION_TOOL_CANCELLED_DRAIN_SECONDS`). This
  removes wasted work but trades off a small fixed-tail cancellation cost
  (`tool_execution.py:1480-1559`).

- **Tool-not-found behavior depends on configuration.** The default is
  `raise_error`; setting `ToolNotFoundBehavior.return_error_to_model`
  (`run_config.py:66`) converts missing tools into model-visible output. Two
  paths through the same logic increase the surface for callers to choose the
  wrong policy unless they read carefully.

- **Tail-binding context vs. `ToolContext`.** Picking the wrong context type
  can leak runtime metadata through downstream serializers; the SDK inspects
  annotations rather than letting the author pass it
  (`tool.py:1772-1803`). This is robust but means callers should avoid weird
  decorators that drop annotations.

## Failure Modes / Edge Cases

- **Non-JSON tool arguments:** raised as `ModelBehaviorError("Invalid JSON
  input for tool …")`. With the default formatter, the loop continues with a
  friendly parsing message; without it, the run terminates
  (`tests/test_run_step_execution.py:2818-2876`).

- **Pydantic validation failure for valid-JSON-but-wrong-shape arguments:**
  same escalation path — `ModelBehaviorError` chained from `ValidationError`,
  formatter turns it into a recoverable string or surfaces it
  (`tool.py:1984-1990`, `tool.py:526-568`).

- **Tool call missing `call_id`:** the executor returns a rejection in the HITL
  resume path (`execute_approved_tools`, `tool_execution.py:2212-2228`);
  shell-call payloads without `call_id` raise `ModelBehaviorError`
  (`tool_execution.py:609-614`,
  `tests/test_hitl_error_scenarios.py:745-766`).

- **Tool invoked while disabled:** `ModelBehaviorError("Tool X is currently
  disabled for agent Y")` rather than silent skip
  (`tool_execution.py:1407-1417`).

- **Tool not in lookup map:** `_build_tool_not_found_output_items` builds
  rejection output messages and appends them to `new_step_items`
  (`turn_resolution.py:259-274`). With `tool_not_found_behavior="raise_error"`,
  the same logic raises; with `"return_error_to_model"`, a synthetic
  `ToolCallOutputItem` is fed to the model.

- **Approval rejected mid-turn:** `_maybe_execute_tool_approval` short-circuits
  with `function_rejection_item` carrying the rejection message
  (`tool_execution.py:1702-1736`); the loop continues with that as the
  model-visible output.

- **Sibling failures during parallel execution:** failures are arbitrated
  deterministically and re-raised; post-invoke tasks get a bounded
  `_FUNCTION_TOOL_POST_INVOKE_WAIT_SECONDS` (0.1 s) to surface late errors
  before they are dropped into `call_exception_handler`
  (`tool_execution.py:1533-1546`).

- **Timeout on a function tool:** `asyncio.wait_for` in `invoke_function_tool`
  raises `asyncio.TimeoutError`; the SDK converts it to either a
  `ToolTimeoutError` (when `timeout_behavior="raise_exception"`) or a
  formatter-supplied string (when `timeout_behavior="error_as_result"`)
  (`tool.py:1814-1847`).

- **Streaming partial arguments:** reconstructed on stream completion
  (`test_streaming_tool_call_arguments.py` exists; the streaming path mirrors
  the non-streaming one per `AGENTS.md`).

- **Redaction of sensitive payloads on logs/traces:** when
  `_debug.DONT_LOG_TOOL_DATA` is set, the parser raises `ModelBehaviorError`
  outside the `except` block so the raw payload isn't carried in
  `__context__` (`tool.py:1583-1593`).

## Future Considerations

- **Pluggable repair pipelines.** Today a malformed tool call either crashes
  the turn or is reported to the model as a fixed string. There is no
  `tool_repair_function`/`arg_repair_callback` hook comparable to, say,
  `failure_error_function`. Adding one would close the gap between "model
  tried" and "model recovered."

- **Explicit approval persistence.** For `Session`-backed long-running runs,
  approval state must be hoisted into the session explicitly via
  `RunState`. A persistent approval list on `Session` (or a
  `persist_approvals=True` default) would reduce HITL footguns.

- **Observability for parallel tool failures.** Arbitration is sound but the
  merged failure is logged at the chosen priority only once. Surfacing the
  set of cancelled-sibling failures in tracing data would help diagnose
  cascading failures.

- **`additionalProperties=False` ergonomic escape hatch.** `strict_schema.py`
  raises `UserError` when a user-generated schema has
  `additionalProperties: true` but no `strict=False` opt-out at the schema
  level. A per-tool `strict_mode=False` (already supported via
  `function_tool(strict_mode=False)`) plus clearer error messages would reduce
  accidental blocked tool definitions.

- **Default tool guardrails (security posture).** Today, every tool only
  enforces guardrails if the author attaches them. A future-default for
  shell/computer/apply_patch tools could plausibly block obviously dangerous
  operations by default.

## Questions / Gaps

- **Streaming argument repair.** How partial-deleted/duplicated streaming
  arguments are reconstructed across reconnects is not visible in the static
  path surveyed here; the streaming code path is the source of truth
  (`src/agents/run_internal/streaming.py` not fully traversed).

- **`ToolNotFoundBehavior` defaults differ across SDK versions.** The
  `ToolNotFoundBehavior` setting is referenced from `run_config.py:66` but its
  default is not located in the searched paths; reviewers should confirm
  against `RunConfig.__init__`.

- **Custom-tool error visibility.** `CustomToolAction` logs `output_text` to
  `logger.error` on exception (`tool_actions.py:705`) but does not consult
  `failure_error_function`; the model-visible output is the same string from
  `format_shell_error`. This asymmetry with `FunctionTool` is intentional but
  worth documenting for custom-tool authors.

Generated by `03.03-tool-calling-roundtrip-control` against `openai-agents-sdk`.

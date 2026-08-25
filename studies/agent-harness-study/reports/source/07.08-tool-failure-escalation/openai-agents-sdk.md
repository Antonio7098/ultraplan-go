# Source Analysis: openai-agents-sdk

## Dimension 07.08: Tool Failure Escalation

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk (OpenAI Agents SDK, Python) |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+, asyncio, pydantic, OpenAI Responses API |
| Analyzed | 2026-08-25 |

All citations below are relative to the source root above (e.g. `src/agents/tool.py:1863` = `studies/agent-harness-study/sources/openai-agents-sdk/src/agents/tool.py:1863`).

## Summary

The SDK implements tool failure escalation as a layered pipeline with an explicit decision at every layer about who sees the failure: the model, the application developer, the human operator, or the tracing/log backend.

1. **Model-facing (default recovery path).** A crashing function tool does not fail the run by default. The invoker wrapper `_FailureHandlingFunctionToolInvoker.__call__` (`src/agents/tool.py:653-667`) catches any exception and routes it through `maybe_invoke_function_tool_failure_error_function` (`src/agents/tool.py:1964-1981`) to a configurable formatter. The default, `default_tool_error_function` (`src/agents/tool.py:1863-1872`), returns `"An error occurred while running the tool. Please try again. Error: <str(error)>"` as the tool output item, deliberately making the failure a recovery path: the model sees actionable text and can correct arguments or choose another tool.
2. **Application-facing escape hatch.** Passing `failure_error_function=None` to `@function_tool` disables formatting so exceptions propagate to the caller (`src/agents/tool.py:2551-2553`, documented in `docs/tools.md:576-606`). At the run-loop boundary `_run_single_tool` re-raises `AgentsException` subclasses untouched and wraps foreign exceptions in `UserError(f"Error running tool {name}: {e}")` (`src/agents/run_internal/tool_execution.py:1839-1852`).
3. **Timeouts are first-class failures** with two explicit escalation modes: `timeout_behavior="error_as_result"` returns a model-visible message (`default_tool_timeout_error_message`, `src/agents/tool.py:1881-1883`), while `"raise_exception"` raises `ToolTimeoutError` (`src/agents/exceptions.py:507-516`) and fails the run (`src/agents/tool.py:2144-2171`).
4. **Human escalation** is via approval interruptions: tools declaring `needs_approval` pause the run; pending approvals surface on `RunResult.interruptions` (`src/agents/result.py:515-516`) and a human resolves them through `RunState.approve()` / `RunState.reject(..., rejection_message=...)` (`src/agents/run_state.py:1255-1294`); the rejection text becomes the model-visible tool output on resume.
5. **Operator visibility** is Python logging on the `openai.agents` logger plus tracing span errors — both redacted by default (`OPENAI_AGENTS_DONT_LOG_TOOL_DATA` defaults to True, `src/agents/_debug.py:16-17`; trace strings become "Tool execution failed. Error details are redacted.", `src/agents/util/_tool_errors.py:5-14`).
6. **Retries exist only for model calls**, not tools. `ModelRetrySettings`, composable `retry_policies` (`src/agents/retry.py:209-231, 304-464`), and the runner loop `get_response_with_retry` (`src/agents/run_internal/model_retry.py:574-679`) exhaust a retry budget (`attempt > max_retries → RetryDecision(retry=False)`, `src/agents/run_internal/model_retry.py:403-404`) and then re-raise the original error. Tool-level recovery is delegated entirely to the model via the error envelope, or to humans via approvals.

The overall design answers the dimension's framing question directly: a failed tool is by default a recovery path for the model, a dead end only when the developer opts out (`failure_error_function=None`), raises (`timeout_behavior="raise_exception"`), or the failure is classified as non-recoverable (approval rejection is still returned as output, but malformed model behavior like tool-not-found fails the run unless `tool_not_found_behavior="return_error_to_model"`).

## Rating

**Score: 9 / 10**

Rationale against the rubric:

- **Clear model with explicit interfaces**: failure policy is a first-class constructor parameter per mechanism — `failure_error_function` (`src/agents/tool.py:2516`), `timeout_behavior`/`timeout_error_function` (`src/agents/tool.py:2470-2471`), `tool_error_formatter` (`src/agents/run_config.py:448`), `tool_not_found_behavior` (`src/agents/run_config.py:472`). Each is typed, documented in docstrings, and covered in `docs/tools.md:521-606` and `docs/running_agents.md:157-260`.
- **Tests prove behavior**: default vs custom formatters (`tests/test_function_tool.py:488-554`), cancellation normalization into the formatter contract (`tests/test_function_tool.py:713-731`), formatter survival across copies (`tests/test_function_tool.py:784-832`), HITL error scenarios (`tests/test_hitl_error_scenarios.py:240-583`), span-error attachment and redaction (`tests/test_tracing_errors.py:38-764`).
- **Operational safeguards**: payload-redaction-by-default across logs, traces, and exception chains (`src/agents/_debug.py:12-28`; `src/agents/exceptions.py:159-215` detaches traceback state from redacted errors), replay-safety vetoes before any retry (`src/agents/run_internal/model_retry.py:389-493`), and deterministic arbitration of concurrent tool failures (`src/agents/run_internal/tool_execution.py:258-304, 1685-1714`).
- **Not a 10 because**: there is no built-in tool-level retry/backoff abstraction (recovery is wholly delegated to the model or humans), no `on_tool_error` lifecycle hook (hooks cover start/end only, `src/agents/lifecycle.py:70-103`), no aggregated failure metrics/alerting surface, and the default error string embeds raw `str(error)` model-side regardless of redaction settings (`src/agents/tool.py:1872`).

## Evidence Collected

Every entry cites file paths with line numbers relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Failure taxonomy (SDK exception hierarchy) | `AgentsException` base with `run_data`; subclasses `ModelBehaviorError`, `UserError`, `ToolTimeoutError`, `MCPToolCancellationError`, guardrail tripwires | `src/agents/exceptions.py:434-504`, `src/agents/exceptions.py:507-572` |
| Model-facing default error envelope | `default_tool_error_function` returns "An error occurred while running the tool. Please try again. Error: …"; special JSON-parse variant | `src/agents/tool.py:1863-1872` |
| Formatter selection & opt-out | `failure_error_function` param; `None` means raise instead of return; resolution honors programmatic output schemas | `src/agents/tool.py:2551-2553`, `src/agents/tool.py:1902-1911` |
| Catch-and-format wrapper | `_FailureHandlingFunctionToolInvoker.__call__` catches `Exception`, formats to string, logs handled error; re-raises if no formatter | `src/agents/tool.py:653-667` |
| Malformed model input classification | Invalid JSON / schema validation → `ModelBehaviorError` (model-caused), message includes input unless redacted | `src/agents/tool.py:1829-1852`, `src/agents/tool.py:2625-2631` |
| Timeout escalation modes | `error_as_result` default returns model-visible timeout text; `raise_exception` raises `ToolTimeoutError` | `src/agents/tool.py:2144-2171`, `src/agents/tool.py:499-507` |
| Cancellation as formatted failure | CancelledError coerced to Exception contract for formatters (`_FunctionToolCancelledError`); cancellation path attaches "Tool execution cancelled" span error | `src/agents/tool.py:1944-1961`, `src/agents/run_internal/tool_execution.py:2069-2095` |
| Fatal-path wrapping at run boundary | Non-`AgentsException` from tool body wrapped in `UserError("Error running tool {name}: {e}")`; span gets "Error running tool" | `src/agents/run_internal/tool_execution.py:1839-1852` |
| Handled-error observability | SpanError "Error running tool (non-fatal)" + `logger.error("%s failed: %s", ...)` with exc_info, honoring redaction | `src/agents/tool.py:1773-1826`, wiring at `src/agents/tool.py:2678-2682` |
| Trace redaction | `get_trace_tool_error` → "Tool execution failed. Error details are redacted." when sensitive data excluded | `src/agents/util/_tool_errors.py:5-14`, `src/agents/util/_error_tracing.py:46-67` |
| Log redaction defaults | `OPENAI_AGENTS_DONT_LOG_MODEL_DATA` / `OPENAI_AGENTS_DONT_LOG_TOOL_DATA` both default True | `src/agents/_debug.py:12-28` |
| Operator logger & levels | `openai.agents` logger; `log_tool_action_error/warning/debug` gate payloads per policy | `src/agents/logger.py:7`, `src/agents/logger.py:134-188` |
| MCP failure passthrough | `MCPError` re-raised into FunctionTool failure pipeline with warning log; JSON decode → `ModelBehaviorError`; same handler/span labels ("MCP tool") | `src/agents/mcp/util.py:703-759`, `src/agents/mcp/util.py:567-585` |
| MCP server connect failures | `MCPServerManager` records per-server errors in `self._errors`, strict mode raises first failure | `src/agents/mcp/manager.py:217-237`, `src/agents/mcp/manager.py:399-441`, `src/agents/mcp/manager.py:504-526` |
| Tool-not-found escalation switch | Default raises `ModelBehaviorError("Tool X not found in agent Y")`; `tool_not_found_behavior="return_error_to_model"` emits recoverable `function_call_output` | `src/agents/run_config.py:78`, `src/agents/run_config.py:472-478`, `src/agents/run_internal/turn_resolution.py:2183-2189`, message builder `src/agents/run_internal/turn_resolution.py:258-306` |
| Run-wide error formatter | `RunConfig.tool_error_formatter` receives `kind` ∈ {"approval_rejected","tool_not_found"}, tool type/name/call_id, default message; failures in the formatter fall back to defaults | `src/agents/run_config.py:83-105`, usage `src/agents/run_internal/tool_execution.py:1253-1297` |
| Approval-rejection envelope | `REJECTION_MESSAGE = "Tool execution was not approved."`; rejection items built for function/shell/apply_patch/custom; shell rejection encodes exit_code 1 + stderr | `src/agents/tool.py:194`, `src/agents/run_internal/items.py:23-25`, `src/agents/run_internal/items.py:803-899` |
| Human-in-the-loop API | `RunState.approve/reject(..., rejection_message)`; pending items on `RunResult.interruptions`; serializable paused runs | `src/agents/run_state.py:985`, `src/agents/run_state.py:1255-1294`, `src/agents/result.py:515-516,649-651`, `docs/human_in_the_loop.md:3-101` |
| Programmatic approval callbacks | `on_approval`/`on_approval_request` per tool family auto-decide without pausing | `src/agents/tool.py:1097-1100`, `src/agents/tool.py:1375-1378`, `src/agents/run_internal/tool_execution.py:1162-1213` |
| Nested (agent-as-tool) interruption propagation | `FunctionToolResult.interruptions`; nested interruptions surfaced on outer run and kept distinct across calls | `src/agents/tool.py:388-392`, `src/agents/run_internal/tool_execution.py:2205-2237`, test `tests/test_hitl_error_scenarios.py:434-535` |
| Parallel failure arbitration | Batch executor cancels siblings on first failure, drains within bounded windows (0.25 s cancelled / 0.1 s post-invoke), merges late failures by priority (Cancelled < Exception < BaseException) without masking root cause | `src/agents/run_internal/tool_execution.py:167-169`, `src/agents/run_internal/tool_execution.py:258-304`, `src/agents/run_internal/tool_execution.py:1685-1714` |
| Cross-category sibling failure | `sibling_category_failure` asyncio.Event coordinates function/shell/computer categories in one turn | `src/agents/run_internal/tool_planning.py:976-1048`, drain logic `src/agents/run_internal/tool_execution.py:1753-1775` |
| Background-task failure reporting | Late sibling/post-invoke task exceptions routed to loop exception handler with distinct messages; CancelledError silenced | `src/agents/run_internal/tool_execution.py:207-255` |
| Model-call retries (not tools) | `ModelRetrySettings.max_retries/backoff/policy`; composable policies (`never/provider_suggested/network_error/retry_after/http_status/all/any`) | `src/agents/retry.py:209-231`, `src/agents/retry.py:304-464` |
| Retry loop & exhaustion | `get_response_with_retry`: policy evaluation, rewind+backoff, `attempt > max_retries → no retry → raise`; conversation_locked compatibility retries capped | `src/agents/run_internal/model_retry.py:574-679`, exhaustion check `src/agents/run_internal/model_retry.py:403-404` |
| Replay-safety vetoes | Abort/streamed-output/stateful-request/provider-unsafe replays blocked even when policy says retry; explicit `approve_unsafe_replay` required | `src/agents/run_internal/model_retry.py:389-493`, `src/agents/retry.py:116-133` |
| Retry-exhaustion accounting | Failed attempts recorded as zero-token request usage entries on final response usage | `src/agents/run_internal/model_retry.py:338-350` |
| Run-failure snapshot for callers | `RunErrorDetails` (input, new_items, raw_responses, last_agent, accumulated tool guardrail results) attached via `AgentsException.run_data` | `src/agents/exceptions.py:413-431` |
| Error handlers keyed by kind | `RunErrorHandlers`: `max_turns`, `model_refusal`, `invalid_final_output` — note: no tool-failure key | `src/agents/run_error_handlers.py:50-55` |
| Lifecycle hooks | `on_tool_start`/`on_tool_end` only; no `on_tool_error` hook exists | `src/agents/lifecycle.py:70-103`, `src/agents/lifecycle.py:148-181` |
| Programmatic-tool schema bypass | SDK-generated error strings encoded as `{"error": ...}` JSON to satisfy declared output schemas | `src/agents/run_internal/items.py:803-821`, applied `src/agents/run_internal/tool_execution.py:2115-2131` |
| Tests: formatter defaults & customization | Default envelope asserted equal to `default_tool_error_function` output; sync/async custom formatters receive typed errors | `tests/test_function_tool.py:488-554` |
| Tests: cancellation & copy semantics | CancelledError normalized for formatter; formatter survives `dataclasses.replace`/copy | `tests/test_function_tool.py:713-731`, `tests/test_function_tool.py:784-832` |
| Tests: HITL scenarios | Resume-after-approve/reject across function/shell/apply_patch/MCP; nested rejections; duplicate-approval guards | `tests/test_hitl_error_scenarios.py:240-583` |
| Tests: span errors & redaction | Tool call errors mark spans; redacted runs never stringify exceptions; span failure cannot replace run exception | `tests/test_tracing_errors.py:143-213`, `tests/test_tracing_errors.py:606-764` |
| Docs: documented escalation contract | Timeouts, `failure_error_function` semantics incl. `None` → re-raise, manual `FunctionTool` must self-handle | `docs/tools.md:521-606` |

## Answers to Dimension Questions

1. **Who sees tool failure?**
   - *Model*: always by default — the formatter output becomes the `function_call_output` (`src/agents/tool.py:653-667`, `default_tool_error_function` at `src/agents/tool.py:1863-1872`); timeouts likewise by default (`src/agents/tool.py:2144-2171`).
   - *Application developer*: on opt-out (`failure_error_function=None`) or when the SDK classifies the failure as fatal (`AgentsException` re-raised; foreign exceptions wrapped in `UserError`, `src/agents/run_internal/tool_execution.py:1839-1852`). Callers also get `RunErrorDetails` snapshots on failed runs (`src/agents/exceptions.py:413-431`).
   - *Human*: when a tool declares `needs_approval` and no stored/auto decision exists — the run pauses with `interruptions` (`src/agents/result.py:515-516`; flow in `docs/human_in_the_loop.md:49-57`).
   - *Operator*: via the `openai.agents` logger (handled failures logged at ERROR with exc_info unless redacted, `src/agents/tool.py:1814-1824`) and trace spans carrying `SpanError` entries (`src/agents/run_internal/tool_execution.py:1844-1849`).

2. **Is the error actionable?**
   Largely yes. Messages name the tool (`Invalid JSON input for tool {name}`, `src/agents/tool.py:1838`; `Tool '{name}' timed out after N seconds`, `src/agents/tool.py:1881-1883`; `Tool {qualified_name} not found in agent {name}`, `src/agents/run_internal/turn_resolution.py:2187-2188`). The default model-facing text explicitly instructs "Please try again" (`src/agents/tool.py:1872`), and the JSON-parse variant tells the model to resend valid JSON including the parse error (`src/agents/tool.py:1865-1871`). Operator logs include tool name, input JSON, and full traceback unless `DONT_LOG_TOOL_DATA` redacts them (`src/agents/tool.py:1817-1824`). The run-level `tool_error_formatter` gives applications structured context (kind, tool_type, tool_name, call_id) to produce their own messages (`src/agents/run_config.py:86-102`). One caveat: the default envelope embeds raw `str(error)` model-side, which is actionable for the model but can leak internals unless the tool author customizes the formatter.

3. **Can the model recover?**
   Yes, by design. Failures formatted as tool outputs stay in-context so the model can retry with corrected arguments, pick another tool, or answer without the tool. Recovery is widened further by opt-in switches: `tool_not_found_behavior="return_error_to_model"` appends a `function_call_output` and reruns the model instead of raising (`src/agents/run_config.py:472-478`, `docs/running_agents.md:197-213`), and approval rejections return the rejection reason to the model so it can adapt (`src/agents/run_state.py:1277-1281`). The model cannot recover from `timeout_behavior="raise_exception"`, guardrail tripwires, or disabled-tool misuse — those intentionally end the run.

4. **When is failure escalated to a human?**
   Only through the approval/interruption mechanism, not through failures per se: `needs_approval` on function/shell/apply_patch/custom/hosted-MCP tools and `Agent.as_tool(..., needs_approval=...)` pause execution and surface `ToolApprovalItem`s to the caller, who resumes after deciding (`src/agents/run_state.py:1255-1294`; nested propagation at `src/agents/agent.py:979-981`). Applications that can decide in code use `on_approval` callbacks so no human is needed (`src/agents/run_internal/tool_execution.py:1191-1212`). There is no automatic human escalation on repeated failures or retry exhaustion — exhaustion of model retries simply re-raises to the caller (`src/agents/run_internal/model_retry.py:666-667`).

5. **Are failures grouped by cause?**
   Implicitly, via a deliberate exception taxonomy: model-caused (`ModelBehaviorError`), developer-caused (`UserError`), operational (`ToolTimeoutError`, `MCPToolCancellationError`), safety (`*GuardrailTripwireTriggered`) — each with distinct messages and handling paths (`src/agents/exceptions.py:434-572`). Within a parallel batch, concurrent failures are arbitrated by priority (Cancelled < Exception < BaseException, earliest call wins ties) and merged with source labels `direct`/`cancelled_teardown`/`post_invoke` so late failures never mask the root cause (`src/agents/run_internal/tool_execution.py:172-186, 258-304`). However, there is no aggregate grouping/counting surface (no metrics, buckets, or alert hooks); run-level `error_handlers` cover only `max_turns`/`model_refusal`/`invalid_final_output`, not tool failures (`src/agents/run_error_handlers.py:50-55`).

## Architectural Decisions

1. **Failure-as-output is the default; failure-as-exception is opt-in.** `failure_error_function` defaults to the generic formatter rather than to raising; only an explicit `None` opts into propagation (`src/agents/tool.py:2516`, docstring `src/agents/tool.py:2551-2553`). This inverts the naive design where tool exceptions crash the run, making model self-correction the zero-configuration behavior.
2. **One shared failure pipeline for wrapped tools.** All function tools — user-defined (`@function_tool`, `src/agents/tool.py:2673-2698`), MCP-derived (`src/agents/mcp/util.py:567-585`), and agent-as-tool — are constructed through `_build_wrapped_function_tool` which binds the same catch/format/log/span machinery (`src/agents/tool.py:698-756`), guaranteeing consistent escalation semantics across tool origins.
3. **Strict separation of model-visible text from operator diagnostics.** Model outputs, operator logs, and trace spans each have independent redaction policies: `DONT_LOG_TOOL_DATA` gates log payloads (`src/agents/_debug.py:16-17`), `trace_include_sensitive_data` gates span error strings (`src/agents/run_internal/tool_execution.py:1840-1843` via `src/agents/util/_tool_errors.py:8-14`), and redacted exceptions actively scrub traceback frames and `__cause__`/`__context__` chains to prevent payload leakage (`src/agents/exceptions.py:140-178, 315-405`; rationale in `src/agents/tool.py:1840-1847` comment).
4. **Retry authority belongs to providers and policies, not blind loops.** Runner-managed retries require either provider-suggested advice or an explicit policy, with hard vetoes for aborts, already-streamed responses, stateful requests, and provider-marked replay-unsafe failures (`src/agents/run_internal/model_retry.py:389-493`). `RetryDecision.approve_unsafe_replay` is deliberately separate from `retry=True` (`src/agents/retry.py:123-129`).
5. **Deterministic arbitration over nondeterministic crashes.** When one tool in a parallel batch fails, siblings are cancelled and drained within bounded windows, late failures are merged with priority ordering and source attribution, and background leftovers are reported through the loop exception handler rather than swallowed (`src/agents/run_internal/tool_execution.py:1685-1714`, `207-255`).
6. **Human escalation is a resumable state machine, not a callback requirement.** Approvals serialize into `RunState`, survive persistence, support sticky always-approve/reject decisions, and carry optional rejection messages that become model-visible output on resume (`src/agents/run_state.py:1255-1294`; deserialization tests at `tests/test_hitl_error_scenarios.py:726-780`).

## Notable Patterns

- **Handled-vs-fatal span labeling**: successful-formatting failures attach `"Error running tool (non-fatal)"` while propagating failures attach `"Error running tool"` (`src/agents/tool.py:2679-2680`, `src/agents/run_internal/tool_execution.py:1846`), letting trace consumers distinguish recovered from run-killing failures.
- **Formatter-contract normalization for cancellation**: `asyncio.CancelledError` (a `BaseException`) is adapted into the public `(ctx, Exception)` formatter contract via `_FunctionToolCancelledError` so one signature covers all failure modes (`src/agents/tool.py:1944-1961`).
- **Schema-aware error encoding**: for programmatic tool callers with declared output schemas, SDK-generated error strings bypass validation and are encoded as `{"error": "<text>"}` JSON so the error envelope remains schema-conformant (`src/agents/run_internal/items.py:803-821`, `src/agents/run_internal/tool_execution.py:2115-2131`); correspondingly, the default formatter is disabled for such contexts since free-form text would violate the schema (`src/agents/tool.py:1907-1909`).
- **Defense-in-depth formatter fallback**: if an application's `tool_error_formatter` throws or returns a non-string, the SDK falls back to the default message and logs the formatter failure (`src/agents/run_internal/tool_execution.py:1275-1295`, `src/agents/run_internal/turn_resolution.py:288-304`) — a broken customizer degrades to defaults instead of breaking the run.
- **Failure-source provenance in batches**: `_FunctionToolFailure.source` (`direct`/`cancelled_teardown`/`post_invoke`) plus order index gives post-mortems exact attribution of which invocation triggered teardown versus which failed late (`src/agents/run_internal/tool_execution.py:180-186`).
- **Redaction-preserving raise patterns**: repeated care to raise redacted errors *outside* `except` blocks and to drop payload-bearing locals so tracebacks cannot recover sensitive data (`src/agents/tool.py:1840-1847`, `src/agents/tool.py:2276-2285`).

## Tradeoffs

- **Recovery-by-default vs silent degradation**: returning `"An error occurred… Error: {str(error)}"` keeps runs alive but can hide chronic tool breakage behind model-visible strings; operators must watch ERROR logs/traces to notice repeated failures, since nothing else aggregates them.
- **Payload leakage surface in default messages**: the default envelope embeds raw exception text model-side regardless of `DONT_LOG_TOOL_DATA` (which governs logs, not model outputs). Tools handling sensitive data must supply custom formatters; the SDK documents this but does not enforce it (`docs/tools.md:589-594`).
- **No tool-level retry**: transient tool failures (network blips in an HTTP tool) immediately consume a model turn to "tell" the model rather than retrying cheaply in-process. This is simple and avoids side-effect duplication, but costs tokens/latency compared to a bounded tool retry with backoff.
- **Complexity of the parallel arbitration machinery**: ~800 lines of draining/priority/merge logic (`src/agents/run_internal/tool_execution.py:167-550`) buys precise failure attribution under concurrency at a significant maintenance cost; simpler designs would leak CancelledErrors or mask root causes.
- **Approval UX shifts burden to the application**: pausing on `needs_approval` requires the app to persist/resume `RunState`; the alternative `on_approval` callbacks trade human oversight for automation per tool family (`docs/human_in_the_loop.md:91-101`).
- **Compatibility-driven field placement**: internal formatter fields (`_failure_error_function`, `_use_default_failure_error_function`) exist partly to keep positional constructor compatibility for public dataclasses (comments at `src/agents/tool.py:477-478, 495` and repo policy in `AGENTS.md`), adding indirection to formatter resolution (`src/agents/tool.py:1886-1911`).

## Failure Modes / Edge Cases

- **Unhandled formatter crash inside the wrapper**: if a custom `failure_error_function` itself raises during handled-error formatting, the original failure is replaced by the formatter's exception (the wrapper has no inner try around `maybe_invoke_function_tool_failure_error_function`, `src/agents/tool.py:653-667`) — the run then escalates through the fatal path instead.
- **Formatter misbehavior is contained elsewhere**: `tool_error_formatter` (run-level, used for rejections/tool-not-found) is guarded — exceptions and non-string returns fall back to defaults with logged warnings (`src/agents/run_internal/tool_execution.py:1275-1295`). The two formatter layers have different robustness guarantees.
- **Disabled tool invoked anyway**: a model calling a dynamically disabled tool raises `ModelBehaviorError("Tool X is currently disabled for agent Y")` — fatal, not model-recoverable (`src/agents/run_internal/tool_execution.py:1612-1619`).
- **Cancellation during failure propagation**: sibling tasks cancelled during teardown get a bounded 0.25 s drain (with a 64-step immediate-progress cap) so their failures can still surface; anything later is reported via the loop exception handler and cannot override the primary failure (`src/agents/run_internal/tool_execution.py:167-168, 488-529, 1700-1714`).
- **Timeout race**: if the tool completes exactly at the deadline with an exception, the underlying exception is re-raised instead of the timeout (`src/agents/tool.py:2144-2149`); a completed-but-cancelled task yields the timeout error.
- **MCP server-level failure accumulation**: connect failures per server are recorded and only raised in strict mode or when all servers fail, so partial MCP degradation continues with remaining servers (`src/agents/mcp/manager.py:399-441, 504-526`).
- **Hosted MCP cancellation**: a cancelled hosted MCP call is converted into `MCPToolCancellationError` with server/tool naming rather than surfacing a bare `CancelledError` (`src/agents/mcp/util.py:711-728`).
- **Resume-time tool removal**: if an approved tool disappears between pause and resume, the run raises `ModelBehaviorError` unless `tool_not_found_behavior="return_error_to_model"` is set, in which case a cached result or error output is substituted (`src/agents/run_internal/turn_resolution.py:2181-2199`).

## Future Considerations

- **Add an `on_tool_error` lifecycle hook** alongside `on_tool_start`/`on_tool_end` (`src/agents/lifecycle.py:70-103`) so applications can route failures to alerting/metrics without wrapping every tool.
- **Bounded in-process tool retry primitive**: a `ToolRetrySettings(max_attempts, backoff, retry_on=...)` analogous to `ModelRetrySettings` would let transient tool failures retry without spending a model turn; it could reuse the replay-veto ideas from `src/agents/retry.py:116-133` for side-effectful tools.
- **Aggregate failure telemetry**: counters/buckets keyed by the existing taxonomy (`ModelBehaviorError` vs `UserError` vs timeout vs MCP) would make "grouped by cause" observable; today only traces/logs carry this implicitly.
- **Redaction-aware default envelope**: consider gating the `Error: {str(error)}` suffix in `default_tool_error_function` (`src/agents/tool.py:1872`) on a data-minimization setting, keeping the "please try again" instruction while dropping payload text.
- **Harden the custom `failure_error_function` path**: wrapping the formatter invocation in a fallback try/except (mirroring `tool_error_formatter`'s guard, `src/agents/run_internal/tool_execution.py:1275-1281`) would prevent a buggy formatter from masking the original error.

## Questions / Gaps

- No evidence found for external alerting integrations (webhooks, metrics exporters, PagerDuty-style escalation): searches across `src/agents/` for alerting hooks beyond logging/tracing returned none; operator notification is exclusively `logging` + tracing processors (`src/agents/logger.py:134-150`, `src/agents/tracing/`).
- No evidence found for tool-invocation retry counting or circuit-breaker state per tool; retry bookkeeping exists only at the model-call layer (`src/agents/run_internal/model_retry.py:338-350, 574-679`).
- The exact intended consumer of `is_sdk_generated_error` beyond schema bypass (e.g., whether downstream UIs should special-case SDK-generated failures) is not documented in code or docs; only the encoding behavior is specified (`src/agents/run_internal/items.py:809`).
- Whether the default formatter's inclusion of `str(error)` is considered acceptable for production privacy postures is unstated; docs show customizing it as the remedy (`docs/tools.md:589-594`) but no policy knob enforces redaction on model-facing envelopes.

---

Generated by `dimensions/07.08-tool-failure-escalation.md` against `openai-agents-sdk`.

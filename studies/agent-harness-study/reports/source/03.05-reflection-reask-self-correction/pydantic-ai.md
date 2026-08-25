# Source Analysis: pydantic-ai

## 03.05: Reflection, ReAsk, and Self-Correction Loops

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (async-first agent framework on top of Pydantic + pydantic-graph + httpx) |
| Analyzed | 2026-07-14 |

## Summary

Pydantic AI implements reflection and self-correction as a **bounded, exception-driven retry loop** woven through the agent graph in `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1`. There is no separate "reflection agent" or critic model. Instead, every recoverable failure is signalled by raising `pydantic_ai.exceptions.ModelRetry` (`pydantic_ai_slim/pydantic_ai/exceptions.py:40`), which the graph converts into a `pydantic_ai.messages.RetryPromptPart` (`pydantic_ai_slim/pydantic_ai/messages.py:1407`) attached to a fresh `ModelRequest`. The model then sees its own previous response in history plus a structured error and is given another chance.

Retry budgets are tracked in two dimensions: a **per-tool** counter inside `RunContext.retries: dict[str, int]` (`pydantic_ai_slim/pydantic_ai/_run_context.py:60`) and a **global output retry** counter `GraphAgentState.output_retries_used` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:127`). Both are bounded, with over-budget cases raising `pydantic_ai.exceptions.UnexpectedModelBehavior` (`pydantic_ai_slim/pydantic_ai/exceptions.py:203`) rather than looping forever.

The retry contract surfaces at every layer where the model can fail: tool argument schema validation (`pydantic_ai_slim/pydantic_ai/tool_manager.py:215`), tool body execution (`pydantic_ai_slim/pydantic_ai/toolsets/function.py:656`), structured output validation (`pydantic_ai_slim/pydantic_ai/_output.py:121`), output validators (`pydantic_ai_slim/pydantic_ai/_output.py:417`), capability hooks (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:566`), empty / thinking-only responses (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1166`), unknown tool names (`pydantic_ai_slim/pydantic_ai/tool_manager.py:369`), and post-call checks like `IncompleteToolCall` (`pydantic_ai_slim/pydantic_ai/exceptions.py:308`). This makes the "when does the agent self-correct" question easy to answer: at every site where state is rejected for being invalid.

The evidence supplied to the model during correction is concrete. Validation errors are serialised as a `list[pydantic_core.ErrorDetails]` payload with `loc`, `msg`, `type`, and `input` (`pydantic_ai_slim/pydantic_ai/messages.py:1422`), the `RetryPromptPart.model_response()` method renders them as JSON and adds "Fix the errors and try again." (`pydantic_ai_slim/pydantic_ai/messages.py:1466`). The original response — including the bad tool call or text — is preserved in message history before the retry prompt, so the model can see exactly what it produced.

Correction is bounded by `AgentRetries` (`pydantic_ai_slim/pydantic_ai/agent/abstract.py:106-129`), which is a TypedDict with `tools` and `output` budgets. Defaults are 1 for both. Each retry consumes one unit of the relevant budget; once exhausted the run raises `UnexpectedModelBehavior` with a specific message ("Exceeded maximum output retries (N)" or "Tool {name!r} exceeded max retries count of {N)"). Concurrent runs are isolated because `OutputToolset.for_run_step` shallow-copies retry state (test `tests/test_agent.py:10766-10808` verifies two concurrent runs on the same agent each see `[0, 1, 2]`).

## Rating

**9 / 10**

The framework has a clear, exception-based reflection contract that is consistent across tool calls, output validation, capability hooks, and edge cases like empty responses. Retry budgets are bounded at two layers (tool-local and run-global), enforced with distinct error messages, overridable per-tool via `ToolOutput(max_retries=...)` (`pydantic_ai_slim/pydantic_ai/output.py:117-119`), and overridable per-run via `agent.run(retries={'output': N})`. The model sees rich, structured evidence (full Pydantic error details, original response preserved, retry-counter context exposed via `ctx.retry`/`ctx.max_retries`/`ctx.last_attempt`). Extensive unit tests in `tests/test_agent.py`, `tests/test_capabilities.py`, `tests/test_tools.py`, `tests/test_streaming.py`, and `tests/test_tenacity.py` cover budget enforcement, isolation, and the various retry triggers. There is no formal evaluator/scoring model — improvement is implicit (does the next response validate?), which is the only reason this is not a 10. The system *can* recover from bad model output without hallucinating success because every retry attempt either passes validation or raises a hard `UnexpectedModelBehavior` carrying the original cause.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Core reflection exception | `ModelRetry` is the user-facing signal that the model should be re-prompted. Pydantic serialisable so it survives storage round-trips. | `pydantic_ai_slim/pydantic_ai/exceptions.py:40-77` |
| Tool-body / output-fn reask exception | `ToolRetryError` carries a `RetryPromptPart` payload — this is the contract between `ToolManager` and `_agent_graph`. | `pydantic_ai_slim/pydantic_ai/exceptions.py:273-306` |
| Unexpected-behavior terminator | `UnexpectedModelBehavior` is the stop condition when budgets are exhausted. Has a `body` field for diagnostic capture. | `pydantic_ai_slim/pydantic_ai/exceptions.py:203-229` |
| RetryPromptPart message type | The actual message sent back to the model. Holds `list[ErrorDetails] | str`, `tool_name`, `tool_call_id`. | `pydantic_ai_slim/pydantic_ai/messages.py:1407-1444` |
| Human-readable retry rendering | Renders the structured error list as JSON with "Fix the errors and try again." tail. Strips `input` for `NativeOutput` top-level errors to avoid duplicating large JSON the model already produced. | `pydantic_ai_slim/pydantic_ai/messages.py:1446-1468` |
| Global output retry counter (state) | `output_retries_used: int` is the per-run global budget counter, separate from per-tool counters. | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:127` |
| Budget consumption helper | `consume_output_retry()` increments and raises `UnexpectedModelBehavior` on exhaustion. `check_incomplete_tool_call()` is called before raising to detect truncated tool calls. | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:162-179` |
| RunContext per-tool counters | `retries: dict[str, int]` is the per-tool counter map; `retry` and `max_retries` are the active slot values. | `pydantic_ai_slim/pydantic_ai/_run_context.py:60-77` |
| Last-attempt signal | `last_attempt` is exposed to tools/validators so they can switch to a fallback path on the final try. | `pydantic_ai_slim/pydantic_ai/_run_context.py:153-156` |
| ToolManager per-tool cap | `_check_max_retries()` raises `UnexpectedModelBehavior` when `ctx.retries[name] == max_retries`. | `pydantic_ai_slim/pydantic_ai/tool_manager.py:177-181` |
| ToolManager validation→retry | `_wrap_error_as_retry()` converts `ValidationError` or `ModelRetry` into a `ToolRetryError(RetryPromptPart)`. | `pydantic_ai_slim/pydantic_ai/tool_manager.py:183-191` |
| Per-tool counter propagation | `for_run_step()` builds a `retries` map for the next run step from the previous step's `failed_tools`. | `pydantic_ai_slim/pydantic_ai/tool_manager.py:120-145` |
| Per-tool retry-budget context | `FunctionToolset.get_tools()` resolves `max_retries` precedence (tool > toolset > agent). | `pydantic_ai_slim/pydantic_ai/toolsets/function.py:609-642` |
| Tool timeout → retry | `anyio.fail_after` wraps the call; on timeout, raises `ModelRetry(f'Timed out after {timeout} seconds.')` which counts against the retry budget. | `pydantic_ai_slim/pydantic_ai/toolsets/function.py:644-658` |
| Unknown tool retry | `_resolve_tool()` raises `ModelRetry(f'Unknown tool name: {name!r}. ...')` listing available tools. | `pydantic_ai_slim/pydantic_ai/tool_manager.py:356-370` |
| Per-tool retry consumed | `failed_tools.add(name)` ensures the counter advances even on validation failure (not just execution failure). | `pydantic_ai_slim/pydantic_ai/tool_manager.py:407-417` |
| Output retry-prompt builder | `_make_retry_prompt()` builds the `RetryPromptPart` with `tool_name`/`tool_call_id` from `RunContext`. | `pydantic_ai_slim/pydantic_ai/_output.py:121-129` |
| Output validation hooks | `run_output_validate_hooks()` runs `before/wrap/after_output_validate` and `on_output_validate_error`; `ModelRetry`/`ValidationError` from any hook is wrapped into a retry. | `pydantic_ai_slim/pydantic_ai/_output.py:132-179` |
| Output process hooks | Same pattern for `process` hooks — `ModelRetry` from any hook triggers a retry. | `pydantic_ai_slim/pydantic_ai/_output.py:182-226` |
| Output validator retry path | `OutputValidator.validate()` propagates `ModelRetry` raw; caller decides whether to wrap. Validators see the global output retry context (`ctx.retry`, `ctx.max_retries`). | `pydantic_ai_slim/pydantic_ai/_output.py:417-437`, `pydantic_ai_slim/pydantic_ai/tool_manager.py:630-651` |
| Agent-level retry API | `AgentRetries` TypedDict with `tools` and `output` keys. Per-run override only changes `output`; per-tool tool retries are immutable. | `pydantic_ai_slim/pydantic_ai/agent/abstract.py:106-129` |
| Agent init wiring | `Agent.__init__` normalises `retries` (with deprecation warnings for the split `tool_retries`/`output_retries` kwargs) and propagates to `_max_output_retries` / `_max_tool_retries`. | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:368-518` |
| Per-tool override | `ToolOutput.max_retries` overrides the per-tool default; tracked in `OutputToolset._max_retries_overrides`. | `pydantic_ai_slim/pydantic_ai/output.py:117-119`, `pydantic_ai_slim/pydantic_ai/_output.py:1409-1488` |
| Model-request failure → retry | `ModelRequestNode._make_request()` catches `ModelRetry` from `wrap_model_request` / `on_model_request_error` and calls `_build_retry_node()`; original response preserved in history if the handler ran. | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:814-835` |
| After-hook rejection → retry | `after_model_request` raising `ModelRetry` appends the response then routes to retry. | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:964-982` |
| Retry node construction | `_build_retry_node()` increments the budget, builds `RetryPromptPart(content=error.message)`, returns a fresh `ModelRequestNode`. | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1027-1041` |
| Empty / thinking-only retry | When the model returns nothing actionable and the output type can't be `None`, an empty `ModelRequest` is sent back (resubmits the same prompt). | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1101-1173` |
| "Please call a tool" prompt | When the only response parts are non-actionable, the model is told `"Please call a tool"` or `"Please return text"` etc. as the retry message. | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1243-1249` |
| Token-limit retry suppression | If the model hit `finish_reason='length'` before any output, no retry is attempted — raises `UnexpectedModelBehavior` directly. | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1110-1114` |
| Content-filter terminator | `ContentFilterError` is raised (not retried) when the response is empty and `finish_reason='content_filter'`. | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1116-1130` |
| Incomplete tool call detection | `check_incomplete_tool_call()` detects `finish_reason='length'` while a `ToolCallPart` is still being streamed. | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:146-160` |
| Retry observability | `FunctionModel` (the test model) inspects the last message for `RetryPromptPart` and synthesises a retry response — useful for harness-style reflection testing. | `pydantic_ai_slim/pydantic_ai/models/test.py:240-263` |
| HTTP transport retries | `TenacityTransport`/`AsyncTenacityTransport` wrap httpx with `tenacity.retry` for network-layer reflection (separate from agent-layer retries). | `pydantic_ai_slim/pydantic_ai/retries.py:117-294` |
| Retry-After header support | `wait_retry_after` parses the HTTP header and waits the requested duration (or falls back to exponential). | `pydantic_ai_slim/pydantic_ai/retries.py:312-381` |
| Capability `after_model_request` retry contract | Docstring explicitly documents that raising `ModelRetry` here rejects the response and asks the model to try again; counts against the output budget. | `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:566-578` |
| Capability `wrap_model_request` retry contract | Same — raising `ModelRetry` skips `on_model_request_error` and retries directly. | `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:581-594` |
| Capability `on_model_request_error` retry contract | Same — raising `ModelRetry` retries instead of propagating. | `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:596-617` |
| Capability tool-validate hooks | `before/after/wrap_tool_validate` and `on_tool_validate_error` all document that `ModelRetry` triggers a retry. | `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:621-707` |
| Capability tool-execute hooks | `before/after/wrap_tool_execute` and `on_tool_execute_error` all trigger retry on `ModelRetry`. | `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:712-820` |
| Test: budget enforcement | `test_output_validator_exceeds_output_retries` proves that always-failing validator exhausts budget and raises `Exceeded maximum output retries (2)`. | `tests/test_agent.py:10735-10763` |
| Test: per-tool counter isolation | `test_concurrent_runs_output_retry_isolation` proves two concurrent runs each see `[0,1,2]` — no shared state. | `tests/test_agent.py:10766-10808` |
| Test: per-tool counter, tool switch | `test_output_validator_retry_counter_with_tool_switch` proves global counter keeps advancing across output-tool switches. | `tests/test_agent.py:10811-10840` |
| Test: per-tool retry override | `test_tool_output_max_retries_overrides_agent_retries` proves `ToolOutput(max_retries=5)` beats the agent's `retries={'output': 1}`. | `tests/test_agent.py:578-595` |
| Test: args-validator retry budget | `test_args_validator_max_retries_exceeded` proves `args_validator` retries respect the per-tool budget and raise `exceeded max retries`. | `tests/test_tools.py:3620-3637` |
| Test: streaming ModelRetry from output fn | `test_output_function_model_retry_in_stream` proves `ModelRetry` from a `ToolOutput` function in `run_stream()` surfaces as `UnexpectedModelBehavior` (caused by `ModelRetry`) — not `ToolRetryError`. | `tests/test_streaming.py:3353-3381` |
| Test: after_model_request retry | `test_after_model_request_model_retry` proves the capability hook path produces a clean re-prompt cycle. | `tests/test_capabilities.py:11621-11690` |
| Test: after_model_request budget | `test_after_model_request_model_retry_max_retries` proves the capability-hook retry path also counts against the output budget. | `tests/test_capabilities.py:11692-11712` |
| Test: timeout → retry | `test_tool_timeout_exceeds_retry_limit` proves timeouts escalate to `UnexpectedModelBehavior` after the budget. | `tests/test_tools.py:2934-...` |
| Test: RetryPromptPart format | `tests/test_messages.py:1523-...` proves the error rendering matches the snapshot and strips `input` from top-level `NativeOutput` errors. | `tests/test_messages.py:1523-1597` |
| Test: tenacity transport | `tests/test_tenacity.py` (700+ lines) covers `TenacityTransport`/`AsyncTenacityTransport` including `retry-after` header parsing and fallback strategies. | `tests/test_tenacity.py:1-700` |

## Answers to Dimension Questions

1. **When does the agent self-correct?**
   - On any `ModelRetry` raised by a tool body, output function, output validator, or capability hook — `pydantic_ai_slim/pydantic_ai/exceptions.py:40-77` defines the exception.
   - On any `pydantic.ValidationError` from tool argument schema validation or structured output validation — converted to `RetryPromptPart` at `pydantic_ai_slim/pydantic_ai/tool_manager.py:183-191` and `pydantic_ai_slim/pydantic_ai/_output.py:121-129`.
   - On unknown tool names — `pydantic_ai_slim/pydantic_ai/tool_manager.py:356-370` raises `ModelRetry` listing available tools.
   - On tool execution timeout — `pydantic_ai_slim/pydantic_ai/toolsets/function.py:644-658` raises `ModelRetry`.
   - On empty / thinking-only responses that can't be interpreted — `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1101-1173` either resubmits (empty request) or asks the model to take an action ("Please call a tool", "Please return text").
   - On capability-hook rejections via `after_model_request`, `wrap_model_request`, `on_model_request_error`, `before/after_tool_validate`, `before/after_tool_execute`, `wrap_tool_validate`, `wrap_tool_execute`, `on_tool_*_error`, `before/after/wrap_output_validate`, `on_output_validate_error`, `before/after/wrap_output_process`, `on_output_process_error` — every one of them documents `ModelRetry` as a retry trigger in `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:566-820`.

2. **Is correction bounded?**
   - Yes, in two layers. **Per-tool**: `RunContext.retries[name]` is compared against `tool.max_retries` (or `_max_output_retries` for output tools) at `pydantic_ai_slim/pydantic_ai/tool_manager.py:177-181`. **Global output**: `GraphAgentState.output_retries_used` is compared against `_max_output_retries` at `pydantic_ai_slim/pydantic_ai/_agent_graph.py:162-179`. Both raise `UnexpectedModelBehavior` with distinct messages on exhaustion.
   - Defaults are 1 for each budget — see `pydantic_ai_slim/pydantic_ai/agent/__init__.py:368-374`. Both are configurable via `Agent(retries={'tools': N, 'output': N})`, `agent.run(retries={'output': N})` (per-run override is output-only — `pydantic_ai_slim/pydantic_ai/agent/abstract.py:106-129` documents this asymmetry), and `ToolOutput(max_retries=N)` (`pydantic_ai_slim/pydantic_ai/output.py:117-119`).
   - The HTTP layer has independent tenacity-based bounds via `TenacityTransport`/`AsyncTenacityTransport` (`pydantic_ai_slim/pydantic_ai/retries.py:117-294`).

3. **What evidence is shown during correction?**
   - For Pydantic validation failures: a `list[ErrorDetails]` containing `loc` (path), `msg` (human-readable), `type` (error code), and `input` (offending value), rendered as JSON in `RetryPromptPart.model_response()` (`pydantic_ai_slim/pydantic_ai/messages.py:1446-1468`).
   - For tool validation retries, the `input` is preserved at the top level so the model can see what arguments it sent alongside the error (`pydantic_ai_slim/pydantic_ai/messages.py:1458-1462`).
   - For `NativeOutput` retries (no `tool_name`), the input is *stripped* from top-level errors to avoid duplicating the large JSON payload the model already produced (`pydantic_ai_slim/pydantic_ai/messages.py:1454-1462`).
   - For unknown tool names, the message lists the available tools (`pydantic_ai_slim/pydantic_ai/tool_manager.py:364-369`).
   - The original model response (text or tool call) is preserved in message history before the `RetryPromptPart` so the model can see exactly what it produced — `pydantic_ai_slim/pydantic_ai/_agent_graph.py:752-757`, `:828-835`, `:980-982`.
   - Tool `ctx.retry`, `ctx.max_retries`, `ctx.last_attempt` (`pydantic_ai_slim/pydantic_ai/_run_context.py:66-77,153-156`) let tool bodies adapt on the final attempt (e.g. fall back to a default value).

4. **Does correction improve outputs or hide errors?**
   - The framework never silently swaps in a "good" output — every retry attempt either validates or surfaces a hard error. The model sees the exact reason for the previous failure and is given another chance to respond.
   - There is no critic model that scores the new output. Improvement is implicit (does the next response validate?).
   - On budget exhaustion, the framework raises `UnexpectedModelBehavior` carrying the most recent validation error via `__cause__` (`pydantic_ai_slim/pydantic_ai/exceptions.py:211-228`), so callers can inspect what went wrong rather than getting a generic failure.
   - `capture_run_messages()` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2064-2099`) gives the caller the entire conversation including every `RetryPromptPart` and the model's responses, so post-mortem analysis is straightforward.

5. **Are repeated failures escalated?**
   - Yes — `UnexpectedModelBehavior` is the escalation endpoint (`pydantic_ai_slim/pydantic_ai/exceptions.py:203-229`). The user must catch it.
   - Test coverage in `tests/test_agent.py:10760` (`Exceeded maximum output retries (2)`) and `tests/test_tools.py:3636` (`Tool {name!r} exceeded max retries count of N`) confirms the exact message format.
   - There is no built-in "escalate to a different model" hook at the retry layer (that lives at the `FallbackModel` layer, which is a *transport* concern, not a reflection one — `pydantic_ai_slim/pydantic_ai/models/__init__.py`).

## Architectural Decisions

- **Single exception as the reask signal.** `ModelRetry` (`pydantic_ai_slim/pydantic_ai/exceptions.py:40`) is the only way to ask the model to try again. This unifies reask across tools, validators, hooks, and edge cases like unknown tools. The cost is that downstream code must catch `ModelRetry` and wrap it in `ToolRetryError` (carrying `RetryPromptPart`) before it crosses into `_agent_graph` — this wrapping is done by `ToolManager._wrap_error_as_retry()` (`pydantic_ai_slim/pydantic_ai/tool_manager.py:183-191`) and `_make_retry_prompt()` (`pydantic_ai_slim/pydantic_ai/_output.py:121-129`).
- **Two-tier retry budget.** Per-tool counter (`RunContext.retries[name]`) for tool-level recovery and global counter (`GraphAgentState.output_retries_used`) for output-level recovery. This lets a single bad tool not block a different tool from succeeding later in the run, while still preventing global infinite loops on persistent output errors.
- **Counter propagation across run steps.** `ToolManager.for_run_step()` (`pydantic_ai_slim/pydantic_ai/tool_manager.py:120-145`) increments per-tool retries only for tools that failed in the previous step (`failed_tools`), so retries are not double-counted across successive invocations.
- **Original response preserved before retry prompt.** Every retry site appends the response (or partial result) to history *before* the `RetryPromptPart` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:752-757, 828-835, 980-982`). This gives the model the exact context of what it produced, not just an abstract error message.
- **Asymmetric run override.** `agent.run(retries={'output': N})` overrides only the output budget, not the tool budget (`pydantic_ai_slim/pydantic_ai/agent/abstract.py:106-129`). The tool manager is built once at agent construction, so changing per-tool limits at run time would be unsafe.
- **Capability hooks as composition points.** Rather than baking reflection into the core graph, every retry trigger is a capability hook that the user can override (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:566-820`). This lets advanced users intercept and replace the retry semantics (e.g. log retries, swap models, switch to a structured re-prompt).
- **HTTP retries are a separate concern.** `TenacityTransport`/`AsyncTenacityTransport` (`pydantic_ai_slim/pydantic_ai/retries.py:117-294`) handle transport-level failures (network errors, 429s) with their own budgets via tenacity. This is intentionally not unified with the agent-level retry budget — they target different failure modes.

## Notable Patterns

- **`RetryPromptPart` carries structured `ErrorDetails` JSON, not just a string.** This means the model sees `loc` + `msg` + `type` + `input` per error rather than a generic "validation failed" message — `pydantic_ai_slim/pydantic_ai/messages.py:1446-1468`.
- **`RetryPromptPart.model_response()` strips `input` for `NativeOutput` top-level errors** to avoid duplicating the large JSON payload the model just produced. Nested errors keep `input` because they're harder for the model to reconstruct — `pydantic_ai_slim/pydantic_ai/messages.py:1454-1462`.
- **`last_attempt` boolean property on `RunContext`** lets tools and validators switch to a fallback on the final attempt rather than retrying blindly — `pydantic_ai_slim/pydantic_ai/_run_context.py:153-156`.
- **`check_incomplete_tool_call()` distinguishes truncation from ordinary validation failure.** When `finish_reason == 'length'` and the last part is a truncated `ToolCallPart`, the retry is suppressed with a tailored error message — `pydantic_ai_slim/pydantic_ai/_agent_graph.py:146-160`.
- **Empty response handling has three branches.** (a) Allow `None` and pass through — `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1133-1150`. (b) Try to recover text from history — `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1152-1165`. (c) Resubmit an empty `ModelRequest`, betting the model will produce content this time — `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1166-1173`. The last is the closest the framework comes to "blind retry" — it costs one budget unit.
- **Capability-hook retries count against the output budget, not a separate hook budget.** This is documented explicitly at `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:566-578` and tested in `tests/test_capabilities.py:11692-11712`.
- **`ToolRetryError` as a control-flow exception.** `pydantic_ai_slim/pydantic_ai/exceptions.py:273-306` deliberately wraps `RetryPromptPart` in an exception rather than returning a value, so retry propagation is independent of the tool's normal return path. The `_call_tools` dispatcher catches it as a control-flow signal — `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2023-2024`.
- **`FunctionModel` honours `RetryPromptPart` in the test model.** `pydantic_ai_slim/pydantic_ai/models/test.py:240-263` detects retry prompts and synthesises a retry response — useful for testing harness-style reflection without a real model.

## Tradeoffs

- **No formal critic / evaluator agent.** Improvement is implicit — the next response either validates or it doesn't. There's no separate scoring pass. This keeps the design simple but means the framework can't say "your output is 60% likely to be wrong, try harder" — it can only say "this specific field failed validation".
- **Default budget is 1.** A single retry is often insufficient for non-deterministic failures. Users have to opt into higher budgets per-run or per-agent — `pydantic_ai_slim/pydantic_ai/agent/abstract.py:368-374`.
- **`agent.run(retries={'output': N})` is asymmetric.** Tool retries cannot be overridden per run because the tool manager is built at agent construction — `pydantic_ai_slim/pydantic_ai/agent/abstract.py:106-129`. This is documented but may surprise users expecting symmetric override.
- **Retry prompt format is opaque to the model.** The `ErrorDetails` JSON dump is verbose; some models (notably smaller ones) may not extract the actionable part cleanly. There is no machine-readable retry contract — the model has to parse JSON in natural-language context.
- **Empty-response blind retry can burn budget on bad luck.** When the model returns nothing and `None` is not allowed, the framework resubmits the previous prompt as an empty `ModelRequest` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1166-1173`). This costs one output retry even though it gives no new information to the model.
- **HTTP and agent retries are siloed.** A 429 from the provider triggers tenacity retry (`pydantic_ai_slim/pydantic_ai/retries.py:117-294`); a model-rejection after success triggers agent retry (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1027-1041`). Both can fire in a single run, and the budgets are independent — so worst case is `tenacity_attempts * output_retries` model invocations on a single user request.
- **`ToolManager.failed_tools` carries across run steps within a run.** A tool that fails validation increments the counter, and that counter is used to construct the next step's `retries` map (`pydantic_ai_slim/pydantic_ai/tool_manager.py:120-145`). This is correct, but it means a tool that fails validation on the *last* allowed attempt will keep its high counter in `RunContext.retries[name]` for subsequent run steps even though the budget is exhausted.
- **`output_retries_used` is on the global state, not the per-tool state.** So a switch between output tools still consumes the same global budget, not a per-tool budget (`pydantic_ai_slim/pydantic_ai/tool_manager.py:620-630` documents a known caveat about `ctx.last_attempt` semantics).

## Failure Modes / Edge Cases

- **`IncompleteToolCall` mid-stream.** If `finish_reason == 'length'` while a `ToolCallPart` is still being emitted, `check_incomplete_tool_call()` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:146-160`) detects this *before* retry consumption, so the error message points to "increase `max_tokens` or simplify the prompt" rather than masking it as a retry exhaustion.
- **Token-limit on empty / thinking-only response.** When `finish_reason == 'length'` and the model produced nothing, retry is suppressed outright — `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1110-1114`. Retrying with the same token limit would just hit the limit again.
- **Content-filter empty response.** `ContentFilterError` is raised immediately for empty responses with `finish_reason == 'content_filter'` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1116-1130`). Retrying would risk the same filter.
- **Model returns text when a tool call is required.** The fallback prompt lists available actions: `"Please call a tool"`, `"Please return text"`, `"Please return an image"`, `"include your response in a tool call"` — `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1215-1246`.
- **Output validator always fails.** Test `tests/test_agent.py:10735-10763` confirms this raises `UnexpectedModelBehavior(Exceeded maximum output retries (N))` after the configured budget.
- **Concurrent runs on the same agent.** `tests/test_agent.py:10766-10808` proves that two concurrent runs each get independent retry counters — `OutputToolset.for_run_step` shallow-copies the retry state.
- **Streaming `ModelRetry` from `ToolOutput` function.** `tests/test_streaming.py:3353-3381` proves `run_stream()` surfaces it as `UnexpectedModelBehavior(__cause__=ModelRetry(...))` rather than `ToolRetryError` — important because the streaming path doesn't go through `validate_response_output` the same way.
- **`RetryPromptPart` in message history breaks replayability assumptions in providers.** `tests/models/test_xai.py:466` (`test_xai_reorders_retry_prompt_tool_results_by_tool_call_id`) shows that some providers need retry prompts reordered to match tool call IDs — this is a provider-side mitigation, not a pydantic-ai limitation.
- **`ModelRetry` from `on_model_request_error` skips `on_model_request_error`.** Documented in `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:581-594` and `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:596-617`. If a user wants logging on every failure, they must do it in `wrap_model_request` (which always runs).
- **Re-asking after handler ran preserves the response in history** (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:828-835`). This is the right behaviour for transparency but doubles the message count on every reask — useful for `capture_run_messages` debugging, costly for token usage.

## Future Considerations

- **No mention of an explicit "reflection" or "self-evaluation" capability.** Users who want critic-style reflection must build it themselves by combining `after_model_request` (to score) and `ModelRetry` (to feed back). A first-class `Critic` capability would be a natural addition.
- **Per-tool retry counter persistence across runs.** Today `RunContext.retries` is built fresh per run from `ToolManager.failed_tools` (`pydantic_ai_slim/pydantic_ai/tool_manager.py:120-145`). A tool that always fails in run N will fail in run N+1 regardless. Persistent retry history (across runs, with backoff) is not present.
- **Retry prompt verbosity for large JSON errors.** `RetryPromptPart.model_response()` (`pydantic_ai_slim/pydantic_ai/messages.py:1446-1468`) dumps full error JSON. For models with small context windows, this could be a budget sink on reask. A "compressed" mode (just `loc` + `msg`) might be useful.
- **Streaming partial-output retries.** Output validators fire on partial outputs during streaming (`pydantic_ai_slim/pydantic_ai/_output.py:609-614`). A validator that raises `ModelRetry` on a partial output will discard the partial and restart from scratch — the model loses its work-in-progress. This is intentional but undocumented; users must guard with `ctx.partial_output` (`pydantic_ai_slim/pydantic_ai/_run_context.py:84-85`).
- **`ctx.last_attempt` semantics for output validators** are subtly inconsistent. The comment at `pydantic_ai_slim/pydantic_ai/tool_manager.py:626-629` flags this as a known issue (#5238) — when `ToolOutput(max_retries=N)` exceeds `max_output_retries`, the validator's `last_attempt` can fire before the run actually terminates.
- **No multi-model reflection.** A retry that always fails on GPT-5 will keep failing. There's no built-in way to escalate to a different model for reask — that's the job of `FallbackModel`, but `FallbackModel` only kicks in on transport errors (`docs/models/overview.md:334`). Validation retries always go to the same model.
- **Default empty-response retry may burn budget.** The framework resubmits an empty `ModelRequest` when the model returns nothing and `None` is not allowed (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1166-1173`). With a default budget of 1, that's the user's entire budget on a single bad-luck call. Worth a doc note.
- **No retry budget for capability hooks.** The same global output budget is shared between structured output retries, output-validator retries, and capability-hook retries (`tests/test_capabilities.py:11692-11712` verifies this). A user with both an output validator and an `after_model_request` hook will hit the budget faster than they might expect.
- **HTTP Retry-After parsing has no test for malformed values beyond fall-through.** `tests/test_tenacity.py:519` covers invalid format, but untrusted input handling could be tightened.

## Questions / Gaps

- **Is there an idiomatic way to retry with a different model on `ModelRetry`?** Not in pydantic-ai itself — `FallbackModel` is transport-only. Users who want reflection-with-fallback must wrap their own retry loop.
- **What happens to streaming `TextPart` events when the response is later rejected by `after_model_request`?** The response is preserved in history (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:980-982`), so the model sees the streamed text as if it had committed — but the user has already seen it via the event stream. This is a subtle correctness gap that the framework does not document.
- **Is the `output_retries_used` counter exposed in `RunContext` for tool calls?** Yes via `ctx.retry` and `ctx.max_retries` (`pydantic_ai_slim/pydantic_ai/_run_context.py:66-77`), but only on the output-tool path. Function tools see only per-tool counters, not the global output counter. A tool body that wants to know "am I on the last output-level attempt" can't.
- **What is the default `max_retries` per tool when no `retries` config is provided?** `1` (`pydantic_ai_slim/pydantic_ai/toolsets/function.py:613` — `max_retries = tool.max_retries if tool.max_retries is not None else self.max_retries`, falling back to `ctx.max_retries`). For function tools with no tool or toolset override, this is the agent default (`1`).
- **How does `Agent.iter()` differ from `Agent.run()` w.r.t. retry budget consumption?** They use the same `GraphAgentState.output_retries_used` counter — retries are shared across iterations of the same run. Resuming with `agent.run(..., deferred_tool_results=...)` starts a fresh budget (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:289-291` initialises a new `GraphAgentState`).
- **Are retries deterministic in the test model?** `pydantic_ai_slim/pydantic_ai/models/test.py:240-263` only inspects retry prompts to decide which tool to call next — there's no deterministic "good" output generator for arbitrary retry prompts. Tests that exercise retry behaviour with structured outputs use `FunctionModel` (`tests/test_agent.py`) rather than `TestModel`.

---

Generated by `dimensions/03.05-reflection-reask-and-self-correction-loops.md` against `pydantic-ai`.
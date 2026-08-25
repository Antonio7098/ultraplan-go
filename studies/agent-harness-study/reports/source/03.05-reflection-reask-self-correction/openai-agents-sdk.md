# Source Analysis: openai-agents-sdk

## Reflection, ReAsk, and Self-Correction Loops

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+ / asyncio / OpenAI Agents SDK (Responses + Chat Completions) |
| Analyzed | 2026-07-14 |

## Summary

The OpenAI Agents SDK does **not** ship an explicit `reask`/`reflection`/`critic` abstraction
(none of those terms appear in `src/agents/`). Instead it implements four distinct, layered
self-correction surfaces that map directly to the dimension questions:

1. **Model-call retries** for transient transport/server failures — opt-in via `ModelSettings.retry`
   and `retry_policies` in `src/agents/retry.py:1` and `src/agents/run_internal/model_retry.py:1`.
2. **Tool-error feedback to the model** so the LLM can correct itself next turn — implemented via
   `RunConfig.tool_not_found_behavior` and `RunConfig.tool_error_formatter`
   (`src/agents/run_config.py:308`, `:332`), `FunctionTool.failure_error_function`
   (`src/agents/tool.py:458`), `FunctionTool.timeout_behavior`
   (`src/agents/tool.py:439`), and per-tool input/output guardrails
   (`src/agents/tool_guardrails.py:60`).
3. **Run-level bounded loop** that prevents unbounded self-correction — `DEFAULT_MAX_TURNS = 10`
   (`src/agents/run_config.py:33`) plus `MaxTurnsExceeded` escalation in
   `src/agents/exceptions.py:56` and the run-error handler hook in
   `src/agents/run_error_handlers.py:1`.
4. **Per-step guardrail tripwires** that can `reject_content` (feedback to model) or
   `raise_exception` (halt) — `src/agents/run_internal/guardrails.py:1`,
   `src/agents/tool_guardrails.py:40-117`.

The reflection budget is explicit and bounded: `max_turns` caps the LLM re-prompt cycle, and the
model-retry layer has an independent `max_retries` cap. The evidence channel back into the LLM is
the standard Responses/Chat-Completions `function_call_output` item, so the model sees the failure
in its own schema — there is no shadow "reflection prompt" or critic agent in the loop.

## Rating

**6 / 10 — Present but inconsistent, weakly documented, and fragile at scale.**

The mechanisms exist and are testable, but the SDK has no unified "reflection" surface, the
policy composition is biased toward transport-retry over validation-retry, and the self-correction
channels are scattered across four configuration points (`tool_not_found_behavior`,
`tool_error_formatter`, `failure_error_function`, `timeout_behavior`, tool guardrails) that each
opt in independently. There is no general-purpose "ReAsk the model because the answer was weak"
mechanism — only error-derived re-prompts.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Retry policy abstraction | `RetryPolicy`, `RetryPolicyContext`, `RetryDecision`, `_RetryPolicies` namespace | `src/agents/retry.py:114-137`, `:231-361` |
| Built-in retry predicates | `never`, `provider_suggested`, `network_error`, `retry_after`, `http_status`, `all`, `any` | `src/agents/retry.py:232-358` |
| Retry settings dataclass | `ModelRetrySettings` (opt-in, `max_retries` + backoff) | `src/agents/retry.py:161-182` |
| Normalized error model | `ModelRetryNormalizedError` (status, code, retry_after, network/abort/timeout flags) | `src/agents/retry.py:49-89` |
| Hard veto / replay-safe approval | `_with_hard_veto`, `_with_replay_safe_approval` decision modifiers | `src/agents/retry.py:201-228` |
| Model-retry orchestration loop | `get_response_with_retry` non-streamed wrapper | `src/agents/run_internal/model_retry.py:511-607` |
| Streamed retry orchestration | `stream_response_with_retry` with event-based abort detection | `src/agents/run_internal/model_retry.py:610-724` |
| Retry decision gate (attempt-vs-budget) | `_evaluate_retry` honors `attempt > max_retries` → `RetryDecision(retry=False)` | `src/agents/run_internal/model_retry.py:370-433` (`:382`) |
| Stream-event-based retry abort | `_stream_event_blocks_retry` (anything other than `response.created` / `response.in_progress` disables retry) | `src/agents/run_internal/model_retry.py:44`, `:357-367` |
| Default backoff curve | `initial_delay=0.25`, `max_delay=2.0`, `multiplier=2.0`, `jitter=true` | `src/agents/run_internal/model_retry.py:39-42`, `:261-290` |
| Compatibility retry budget | Hard-coded `COMPATIBILITY_CONVERSATION_LOCKED_RETRIES = 3` for `conversation_locked` errors | `src/agents/run_internal/model_retry.py:43`, `:551-570`, `:670-688` |
| Retry wired into runner | `get_response_with_retry` called from `get_new_response` | `src/agents/run_internal/run_loop.py:1882-1902` |
| Streamed retry wired in | `stream_response_with_retry` called from streamed `get_new_response` | `src/agents/run_internal/run_loop.py:1460-1481` |
| Provider retry-advice hooks | `Model.get_retry_advice` overridable per model adapter | `src/agents/models/interface.py:48`, `src/agents/models/openai_responses.py:974-1029`, `src/agents/models/openai_chatcompletions.py:93` |
| Max-turns cap (run-level) | `DEFAULT_MAX_TURNS = 10` | `src/agents/run_config.py:33` |
| Max-turns cap enforcement | `current_turn += 1`, `current_turn > max_turns` → `MaxTurnsExceeded` | `src/agents/run.py:1057-1066`, `src/agents/run_internal/run_loop.py:875-956` |
| `MaxTurnsExceeded` exception | Defined with `run_data` attachment | `src/agents/exceptions.py:56-63` |
| Run-error handler hook | `RunErrorHandlers` keyed by `max_turns` / `model_refusal` | `src/agents/run_error_handlers.py:29-54` |
| Error handler resolution + validation | `resolve_run_error_handler_result` + `validate_handler_final_output` (catches bad JSON / schema) | `src/agents/run_internal/error_handlers.py:80-107`, `:128-166` |
| Persisted error data | `RunErrorDetails` snapshot with input/items/responses/last_agent/guardrail results | `src/agents/exceptions.py:30-43` |
| Tool-not-found behavior enum | `ToolNotFoundBehavior = Literal["raise_error", "return_error_to_model"]` | `src/agents/run_config.py:66` |
| Tool-not-found default message | `_default_tool_not_found_message` produces `"Tool '{name}' not found."` | `src/agents/run_internal/turn_resolution.py:213-214` |
| Tool-not-found resolution w/ formatter | `_resolve_tool_not_found_message` + `_build_tool_not_found_output_items` emit `function_call_output` to model | `src/agents/run_internal/turn_resolution.py:217-281` |
| Tool-not-found path branching | Returns error to model if `run_config.tool_not_found_behavior == "return_error_to_model"`; else `raise ModelBehaviorError` | `src/agents/run_internal/turn_resolution.py:1958-1970` |
| Tool-error formatter hook | `ToolErrorFormatterArgs` (`kind: "approval_rejected" \| "tool_not_found"`) | `src/agents/run_config.py:69-92`, `:308-312` |
| Failure-as-result default | `default_tool_error_function` returns `"An error occurred while running the tool. Please try again. Error: ..."` | `src/agents/tool.py:1609-1618` |
| Tool timeout behavior enum | `ToolTimeoutBehavior = Literal["error_as_result", "raise_exception"]` | `src/agents/tool.py:182`, `:439-447` |
| Timeout-as-result message | `default_tool_timeout_error_message` | `src/agents/tool.py:1839-1842` |
| Per-tool failure error function | `_failure_error_function` field; `maybe_invoke_function_tool_failure_error_function` | `src/agents/tool.py:458-470`, `src/agents/run_internal/tool_execution.py:1805-1823` |
| Rejection message constant | `DEFAULT_APPROVAL_REJECTION_MESSAGE = "Tool execution was not approved."` | `src/agents/tool.py:181` |
| Tool-input guardrail allow/reject/raise | `AllowBehavior`, `RejectContentBehavior`, `RaiseExceptionBehavior` discriminator | `src/agents/tool_guardrails.py:40-117` |
| Tool-input guardrail handling | `raise_exception` halts; `reject_content` returns message to model | `src/agents/run_internal/tool_execution.py:2363-2366` |
| Tool-output guardrail handling | `raise_exception` halts; `reject_content` rewrites model-visible result | `src/agents/run_internal/tool_execution.py:2400-2404` |
| Input guardrails tripwire | `run_input_guardrails` raises `InputGuardrailTripwireTriggered` on first hit | `src/agents/run_internal/guardrails.py:110-142` |
| Output guardrails tripwire | `run_output_guardrails` raises `OutputGuardrailTripwireTriggered` on first hit | `src/agents/run_internal/guardrails.py:145-177` |
| Tripwire exceptions | `InputGuardrailTripwireTriggered`, `OutputGuardrailTripwireTriggered`, `ToolInputGuardrailTripwireTriggered`, `ToolOutputGuardrailTripwireTriggered` | `src/agents/exceptions.py:121-174` |
| Input-guardrails run-once constraint | "Input guardrails run only on the first turn and only for the starting agent" | `AGENTS.md` (root contributor guide) and `src/agents/run.py:227-232` |
| `tool_use_behavior` "run_llm_again" default | `Agent.tool_use_behavior` default value | `src/agents/agent.py:345-365` |
| `reset_tool_choice` anti-loop guard | "ensure that the agent doesn't enter an infinite loop of tool usage" | `src/agents/agent.py:367-369` |
| Re-ask smoke test (tool-not-found) | Test asserts the missing-tool message is fed back and the LLM recovers | `tests/test_agent_runner.py:4458-4489` |
| Re-ask smoke test (formatter override) | Test asserts `tool_error_formatter` shapes the model-visible text | `tests/test_agent_runner.py:4493-4534` |
| Re-ask smoke test (streamed) | Streamed variant: `test_streamed_tool_not_found_behavior_returns_error_to_model` | `tests/test_agent_runner_streamed.py:142-155` |
| Max-turns escalation test | Test asserts `error_handlers["max_turns"]` is called and validates output schema | `tests/test_max_turns.py:298-410` |
| Refusal escalation test | Test asserts `error_handlers["model_refusal"]` short-circuits to safe fallback | `tests/test_max_turns.py:170-234` |
| Default config check | `test_tool_not_found_behavior_defaults_to_raise_error` | `tests/test_run_config.py:205-215` |

## Answers to Dimension Questions

### 1. When does the agent self-correct?

Three feedback paths exist; all use the model's own message channel rather than a separate
reflection prompt.

- **Tool errors as messages** (`tool_use_behavior="run_llm_again"` default
  `src/agents/agent.py:345-365`): when `run_config.tool_not_found_behavior == "return_error_to_model"`
  (`src/agents/run_config.py:332-338`), an unresolved function tool call is converted into a
  `function_call_output` item (`src/agents/run_internal/turn_resolution.py:259-281`) and the loop
  re-prompts the model on the next turn. The default of `"raise_error"` short-circuits the run.
- **Tool exception messages** (`src/agents/tool.py:1609-1618`,
  `src/agents/run_internal/tool_execution.py:1805-1823`): when a `FunctionTool` raises, the
  default `failure_error_function` returns a "please try again" string that becomes the
  `function_call_output`, again letting the next turn self-correct.
- **Tool input/output guardrails** with `reject_content`
  (`src/agents/tool_guardrails.py:40-117`, applied in
  `src/agents/run_internal/tool_execution.py:2363-2366` and `:2400-2404`): the rejection message
  is fed back to the model as the tool result, allowing the model to retry with a different input
  or abandon that tool.

There is no dedicated ReAsk loop for "weak answer" / "schema mismatch" cases beyond these error
channels. Structured output validation is server-side via the Responses API and surfaces as a
`ModelBehaviorError` if parsing fails (`src/agents/run_internal/turn_resolution.py:45`).

### 2. Is correction bounded?

Yes, with three explicit caps:

- **`DEFAULT_MAX_TURNS = 10`** (`src/agents/run_config.py:33`) is the per-run cap on the
  model re-prompt cycle. It is incremented before each `run_single_turn`
  (`src/agents/run_internal/run_loop.py:875-876`, `src/agents/run.py:1057`) and throws
  `MaxTurnsExceeded` once exceeded.
- **`ModelRetrySettings.max_retries`** (`src/agents/retry.py:165`) caps the transport-error
  retries within a single model request; `_evaluate_retry` enforces
  `attempt > max_retries → RetryDecision(retry=False)` (`src/agents/run_internal/model_retry.py:382`).
- **Conversation-locked retry budget of 3** (`COMPATIBILITY_CONVERSATION_LOCKED_RETRIES`,
  `src/agents/run_internal/model_retry.py:43`, applied at `:551-570` and `:670-688`) is a separate
  hard-coded budget for the legacy `conversation_locked` server error.

The "stop_on_first_tool" / `StopAtTools` `tool_use_behavior` values
(`src/agents/agent.py:345-365`) also bound correction by short-circuiting the loop before the
LLM sees another tool result. `reset_tool_choice = True` (default,
`src/agents/agent.py:367-369`) is documented as the anti-infinite-tool-loop guard.

However, the **counted reflection bound is only `max_turns`**; an agent that repeatedly emits
tool-not-found calls will cycle through `max_turns` re-prompts with only `MAX_TURNS` evidence
to the operator — there is no per-tool-call attempt counter that resets on a successful tool call.

### 3. What evidence is shown during correction?

The model sees:

- **The original tool call** (preserved as the `ResponseFunctionToolCall` `ToolCallItem`,
  `src/agents/run_internal/turn_resolution.py:1949-1951`).
- **A `function_call_output` item with the failure text** — built via
  `ItemHelpers.tool_call_output_item(call, message)` (`src/agents/run_internal/turn_resolution.py:277`).
- **Default messages** are short and human-readable:
  - `Tool '{name}' not found.` (`src/agents/run_internal/turn_resolution.py:213-214`)
  - `Tool execution was not approved.` (`src/agents/tool.py:181`)
  - `An error occurred while running the tool. Please try again. Error: ...` (`src/agents/tool.py:1609-1618`)
- **Custom messages** via `tool_error_formatter`
  (`src/agents/run_config.py:308-312`) which can change the wording per `kind`.

Outside the LLM channel, traces and spans record every failure via `_error_tracing.attach_error_to_*`
(`src/agents/run_internal/turn_resolution.py:1952-1957`, `src/agents/tool.py:1550-1571`). The
`RunErrorDetails` snapshot (`src/agents/exceptions.py:30-43`) bundles the run's items and raw
responses so error-handler callbacks see the full context.

### 4. Does correction improve outputs or hide errors?

Improvement is **permissive but not evaluated**. The SDK:

- Lets the model see the error text on the next turn (improvement opportunity).
- Records the failure on the current span (`src/agents/tool.py:1550-1558`,
  `src/agents/run_internal/turn_resolution.py:1952-1957`).
- Lets the user override what the model sees (`tool_error_formatter`,
  `failure_error_function`, `timeout_error_function`) — useful but also a way to suppress
  detail.
- Does **not** retry the same tool call automatically. The correction path is "give the model
  the error and let it decide what to do next," not "re-invoke the tool with the same args."

So correction can improve outputs when the model has the skill to recover, but the SDK does not
detect loops ("same tool call emitted three times in a row") and does not grade whether the
re-prompted output is better than the prior turn.

### 5. Are repeated failures escalated?

Yes, with two paths:

- **Run-level: `MaxTurnsExceeded`** with an opt-in `error_handlers["max_turns"]` callback
  (`src/agents/run_error_handlers.py:50-54`,
  `src/agents/run_internal/error_handlers.py:128-166`). The callback receives the full
  `RunErrorData` snapshot and can return a final output that is then validated against the
  agent's output schema before being committed.
- **Refusal-level: `ModelRefusalError`** with `error_handlers["model_refusal"]` (same file) for
  model refusals.
- **Guardrail-level: tripwire exceptions** that propagate immediately and do not re-prompt:
  `InputGuardrailTripwireTriggered` (`src/agents/exceptions.py:121-131`),
  `OutputGuardrailTripwireTriggered` (`:134-144`), and the tool guardrail variants
  `ToolInputGuardrailTripwireTriggered` / `ToolOutputGuardrailTripwireTriggered` (`:147-174`).
- **Tool-level: timeout raising** when `timeout_behavior == "raise_exception"`
  (`src/agents/tool.py:1834-1835`) and tool-call exception re-raising at
  `src/agents/run_internal/tool_execution.py:1621-1623` (only `UserError` is re-raised after
  wrapping other exceptions, so the run halts with a hard error rather than another retry).

There is no per-tool-call attempt counter or "this tool has failed N times, give up" logic.
Escalation is only via the global turn counter or a developer-defined guardrail/error handler.

## Architectural Decisions

- **Reuse Responses-API `function_call_output` as the feedback channel.** Rather than inventing a
  reflection prompt template, the SDK pushes failures back through the same item type the model
  emits, so self-correction looks like normal multi-turn tool execution. This minimizes schema
  work but means the model only sees a free-form string, not a structured "why it failed" record.
- **Opt-in retry policy with hard veto.** `RetryPolicy` returns `RetryDecision` objects whose
  `_hard_veto` flag can block retries even when `attempt <= max_retries`
  (`src/agents/retry.py:201-228`, `:298-358`). This lets providers say "no, do not retry this"
  without disabling the policy.
- **Transport retry and validation retry are separate.** `ModelRetrySettings` only covers
  transient model-side failures (`src/agents/retry.py:16-42`). Tool validation failures go
  through the tool-execution and turn-resolution paths, not the retry helper.
- **Streamed vs non-streamed retry are mirrored.** `get_response_with_retry`
  (`src/agents/run_internal/model_retry.py:511-607`) and `stream_response_with_retry`
  (`:610-724`) share `_evaluate_retry` (`:370-433`) and the same defaults, satisfying the
  contributor guide rule that streaming and non-streaming paths stay behaviorally aligned.
- **Replay-safety is a first-class concern.** State-modifying requests (with
  `previous_response_id` or `conversation_id`) require a policy decision that
  `_approves_replay` (`:201-208`, `:412-419`) or provider `replay_safety == "safe"` to allow a
  retry. This prevents the runner from re-sending a side-effectful request after a network
  blip without explicit blessing.
- **Max-turns is configurable per call, not just per agent.** All Runner entrypoints accept
  `max_turns` (`src/agents/run.py:205`, `:289`, `:370`), defaulting to 10
  (`src/agents/run_config.py:33`).
- **Error handlers are decoupled from exceptions.** `RunErrorHandlers`
  (`src/agents/run_error_handlers.py:50-54`) are a `TypedDict` of named callbacks; missing keys
  mean the error propagates as-is. There is no implicit escalation handler — by default
  `MaxTurnsExceeded` and `ModelRefusalError` both raise.

## Notable Patterns

- **Decorator ergonomics for guardrails**: `@input_guardrail` / `@output_guardrail`
  (`src/agents/guardrail.py:202-271`, `:305-342`) and `@tool_input_guardrail` /
  `@tool_output_guardrail` (`src/agents/tool_guardrails.py:215-279`) provide a uniform
  registration pattern that wraps user functions into `InputGuardrail` / `OutputGuardrail` /
  `ToolInputGuardrail` / `ToolOutputGuardrail` dataclasses.
- **`AgentToolUseTracker`** (`src/agents/run_internal/tool_use_tracker.py:50-116`) tracks which
  tools each agent has called so `reset_tool_choice` can fire — a structural anti-loop measure
  but not a correction mechanism.
- **`async as_completed` for tripwires**: `run_input_guardrails` /
  `run_output_guardrails` (`src/agents/run_internal/guardrails.py:110-177`) cancel pending
  siblings on the first tripwire, which is the standard pattern for first-failure-wins.
- **Retry safety annotations**: `retry_policy_retries_safe_transport_errors` /
  `retry_policy_retries_all_transient_errors` (`src/agents/retry.py:153-158`,
  `_mark_retry_capabilities` at `:142-150`) tag policies so the runner knows whether it is
  safe to skip a request even when transport-level retries are disabled.
- **Telemetry integration**: every failure path emits a `SpanError` with `tool_name` and a
  redacted error string (`src/agents/tool.py:1550-1571`,
  `src/agents/run_internal/tool_execution.py:1611-1623`), giving OpenTelemetry / Agents Tracing
  a consistent failure signal.

## Tradeoffs

- **No reflection prompt template.** The SDK relies on the model to understand "Tool not found"
  messages and adjust. This keeps the SDK simple but means model quality determines whether
  correction actually happens — and there is no test that asserts the model recovered correctly
  (only that the failure text was delivered, e.g.
  `tests/test_agent_runner.py:4458-4489`).
- **`tool_not_found_behavior` defaults to `"raise_error"`.** Most users will hit
  `ModelBehaviorError` on the first hallucinated tool name unless they explicitly opt in
  (`src/agents/run_config.py:332-338`). The trade-off favors safety (no silent re-prompts) over
  resilience.
- **Streamed retry is conservative.** Once any non-`response.created` / `response.in_progress`
  event has been emitted, retry is disabled
  (`src/agents/run_internal/model_retry.py:44`, `:357-367`). This is correct for stateful
  streams but means a transient network blip mid-stream cannot be transparently retried.
- **Conversation-locked retry is hard-coded at 3** and uses 1.0 · 2^(n-1) backoff
  (`src/agents/run_internal/model_retry.py:560`). Callers can opt out only by setting
  `max_retries=0`, after which `model_settings.retry.policy` is also bypassed — there is no
  way to keep the 3 retries for `conversation_locked` while disabling others, except by using
  a retry policy.
- **`MaxTurnsExceeded` does not distinguish "stuck in a tool loop" from "legitimately ran out
  of turns."** A user-supplied `error_handlers["max_turns"]` callback receives the snapshot and
  can decide, but there is no SDK-side detector for repeated identical tool calls.
- **Per-tool failure is opt-in.** A `FunctionTool` only honors `failure_error_function` when
  the wrapper's `_FailureHandlingFunctionToolInvoker` is bound
  (`src/agents/tool.py:540-568`, `:599-650`), which the `@function_tool` decorator sets up.
  Users who invoke `FunctionTool` directly without the decorator get the default wrapping
  but bypass the `failure_error_function` hook.
- **`tool_error_formatter` kinds are a closed enum.** Today `kind` is
  `Literal["approval_rejected", "tool_not_found"]` (`src/agents/run_config.py:73`). Extending
  to "tool exception" would require changing the union.

## Failure Modes / Edge Cases

- **Unbounded message-history growth.** Each retry that fails-as-message adds a
  `function_call_output` to the conversation, so 10 turns × N tool errors per turn can blow
  past the context window. There is no automatic compaction triggered by failed tools.
- **Concurrent sibling tool calls with one failure.** `_raise_failure_after_draining_siblings`
  (`src/agents/run_internal/tool_execution.py:1480-1509`) lets in-flight siblings complete
  before re-raising — this can leak timing information but it does keep trace ordering stable.
- **`tool_input_guardrail` rejection mid-batch.** Rejection stops the loop after the first
  rejecting guardrail (`src/agents/run_internal/tool_execution.py:2363-2366`); downstream
  guardrails in the same list are skipped, which can surprise users who assume a list implies
  "all of these checks."
- **Schema drift in error-handler output.** `validate_handler_final_output`
  (`src/agents/run_internal/error_handlers.py:80-107`) raises `UserError` if the handler
  returns a value that does not satisfy the agent's `output_type`. This is strict but means
  handlers must return the agent's structured schema, not arbitrary dicts.
- **`MaxTurnsExceeded` after a resume.** The contributor guide notes "Resuming an
  interruption from `RunState` must not increment the turn counter" (`AGENTS.md`), but the
  stream path stores `current_turn = max_turns` when handlers consume the error
  (`src/agents/run_internal/run_loop.py:951-953`), so a resumed run that immediately exceeds
  again will not re-enter the handler.
- **Provider-managed retries still fire on the first attempt when no policy is set
  (`src/agents/run_internal/model_retry.py:482-491`).** Setting `model_settings.retry` without
  a `policy` does **not** add runner-managed retries but **does** disable provider retries after
  the first attempt — surprising if the intent was "let the SDK retry transient failures."

## Future Considerations

- A unified `reflection` or `reask` policy would let callers say "after a tool returns weak
  output, ask the model to justify before passing to the next tool." Today the closest
  primitives are the `tool_use_behavior="stop_on_first_tool"` flag, the
  `ToolsToFinalOutputFunction` callable (`src/agents/agent.py:345-365`), or a custom guardrail.
- A `kind="tool_exception"` extension to `ToolErrorFormatterArgs.kind` would let
  `tool_error_formatter` own all model-visible failure text in one place, instead of splitting
  the surface across `failure_error_function`, `timeout_error_function`, and `tool_error_formatter`.
- A per-tool attempt counter that escalates after N identical calls would close the loop on
  the most common infinite-loop pattern ("model hallucinates the same tool every turn").
- Auto-detection of "model is producing the same tool call as before" would let
  `MaxTurnsExceeded` carry an actionable `reason` instead of just `f"Max turns ({n}) exceeded"`
  (`src/agents/run.py:1066`).

## Questions / Gaps

- **Is there a documented test that the model recovers from a `tool_not_found` retry with the
  default formatter?** `tests/test_agent_runner.py:4458-4489` only asserts the message reaches
  the model; the actual recovery is via a `FakeModel` script, not a real LLM.
- **How does the `Streamed` path expose the retry counter?** The
  `failed_retry_attempts_out` parameter on `stream_response_with_retry`
  (`src/agents/run_internal/model_retry.py:618`) is plumbed into
  `stream_failed_retry_attempts` (`src/agents/run_internal/run_loop.py:1458`), but I could not
  trace how this counter is surfaced to `RunResultStreaming` consumers — no public attribute
  read was found in this scan.
- **No evidence of a "reflection prompt" feature.** Greps for `reflection`, `reask`, and
  `critic` return no hits in `src/agents/`. The SDK is self-correction-by-error-channel, not
  self-correction-by-critic.
- **`reset_tool_choice = True`** (`src/agents/agent.py:367-369`) is the only documented
  anti-loop measure for tool choice; its actual impact is delegated to
  `maybe_reset_tool_choice` (`src/agents/run_internal/tool_execution.py:167`) and deserves
  closer inspection for bound correctness.
- **Tool guardrails have no retries.** A `reject_content` guardrail
  (`src/agents/tool_guardrails.py:40-117`) feeds the model a message once and trusts the model
  to adjust; the SDK never re-invokes the guardrail on a follow-up turn. Whether this is
  intentional or an oversight is not documented.

---

Generated by `03.05-reflection-reask-self-correction-loops` against `openai-agents-sdk`.

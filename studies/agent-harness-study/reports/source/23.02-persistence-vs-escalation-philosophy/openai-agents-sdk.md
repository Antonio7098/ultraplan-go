# Source Analysis: openai-agents-sdk

## 23.02 Persistence vs Escalation Philosophy

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (asyncio, pydantic; OpenAI Responses/Chat Completions model layer) |
| Analyzed | 2026-08-26 |

Citation convention: all paths below are workspace-relative to the source root
`studies/agent-harness-study/sources/openai-agents-sdk/` (e.g., `src/agents/run.py:1466`).

## Summary

The OpenAI Agents SDK implements persistence as a layered set of explicit mechanisms rather than one monolithic retry loop. The run loop (`Runner.run`, `src/agents/run.py:254-358`) persists by iterating turns until a final output appears, delegating on handoffs (`NextStepHandoff`, consumed at `src/agents/run.py:1392-1404`), feeding tool failures back to the model as tool-call outputs (`src/agents/tool.py:1863-1872`, `src/agents/tool.py:1964-1981`), and pausing for human approval via serialized `RunState` snapshots (`src/agents/run_state.py:749-851`). Model-call retries are strictly opt-in and policy-driven: `ModelSettings.retry: ModelRetrySettings` (`src/agents/model_settings.py:188`) wires `max_retries`, backoff, and a composable policy callback into the runner-managed retry runtime (`src/agents/retry.py:209-231`, `src/agents/run_internal/model_retry.py:574-682`). Escalation is equally explicit: typed exceptions (`MaxTurnsExceeded`, guardrail tripwires — `src/agents/exceptions.py:444-572`) plus per-kind error handlers (`max_turns`, `model_refusal`, `invalid_final_output`) that can synthesize a validated final output instead of raising (`src/agents/run_error_handlers.py:50-55`, invoked at `src/agents/run.py:1482-1490`). A distinctive trait is replay-safety governance: stateful requests fail closed before any retry is allowed unless explicitly approved (`src/agents/run_internal/model_retry.py:440-479`). Persistence decisions are observable through debug logs, structured tracing spans, usage accounting of failed attempts, and streaming events. There is no built-in "replan" strategy beyond re-prompting with error feedback; replanning intelligence is delegated to the model or to application-supplied handlers.

## Rating

**9 / 10.**

Rationale against the rubric's top band ("Mature, durable, observable, extensible, and proven under failure or scale"):

- Clear model with explicit interfaces: opt-in retry settings with a composable policy DSL (`never`, `provider_suggested`, `network_error`, `retry_after`, `http_status`, `any`/`all` combinators) in `src/agents/retry.py:304-461`; typed escalation handlers in `src/agents/run_error_handlers.py:50-55`.
- Operational safeguards are exceptional: replay-unsafe veto logic (`src/agents/run_internal/model_retry.py:403-479`), session-input rewind before replay (`src/agents/run.py:608`, `src/agents/run_internal/run_loop.py:2591-2600`), timeout cancellation with traceback-local scrubbing (`src/agents/run_internal/model_retry.py:288-315`).
- Proven under failure: 96 tests in `tests/models/test_model_retry.py` cover timeouts, parent cancellation, provider-advice overrides, conversation-lock compatibility, and replay approvals; 34 tests in `tests/test_max_turns.py`; resume-path tests in `tests/test_run_impl_resume_paths.py`.
- Not a 10 because: retry/persistence decisions are logged only at DEBUG level (`src/agents/run_internal/model_retry.py:669-676`), there is no first-class replanning mechanism (it is implicit in "return errors to the model"), and autonomy is composed from many independent knobs (`max_turns=None` can fully disable the stop mechanism, `src/agents/run_config.py:581-582`) rather than a coherent, documented autonomy-level dial.

## Evidence Collected

Every entry cites files inside the selected source directory.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Run-loop persistence contract | Loop until final output; handoff → rerun with new agent; tool calls → rerun; documented raise cases | `src/agents/run.py:274-286` |
| Turn-limit enforcement (non-streaming) | `current_turn > max_turns` → attach span error → build `MaxTurnsExceeded` → handler or raise | `src/agents/run.py:1465-1490` |
| Turn-limit enforcement (streaming) | Same check in streamed loop; handler result synthesized into final output, else break with sentinel | `src/agents/run_internal/run_loop.py:1542-1574` |
| Default turn budget / disable knob | `DEFAULT_MAX_TURNS = 10`; `max_turns: int \| None` where ``None`` disables the limit | `src/agents/run_config.py:45`, `src/agents/run_config.py:578-582` |
| Opt-in runner-managed model retries | `ModelSettings.retry: ModelRetrySettings` ("Opt-in runner-managed retry settings") | `src/agents/model_settings.py:188-189` |
| Retry settings surface | `ModelRetrySettings{max_retries, backoff, policy}`; policy excluded from serialization | `src/agents/retry.py:209-231` |
| Backoff configuration | `initial_delay`, `max_delay`, `multiplier`, `jitter` with defaults 0.25s/2.0s/2.0x/jitter-on | `src/agents/retry.py:15-32`, `src/agents/run_internal/model_retry.py:46-49` |
| Policy DSL | `_RetryPolicies.never/provider_suggested/network_error/retry_after/http_status/all/any` | `src/agents/retry.py:304-461` |
| Retry evaluation core | `_evaluate_retry`: budget check, abort/emitted-event hard vetoes, three replay vetoes, delay resolution | `src/agents/run_internal/model_retry.py:389-493` |
| Non-streaming retry driver | `get_response_with_retry` loop with compatibility conversation-lock retries and debug logging | `src/agents/run_internal/model_retry.py:597-681` |
| Streaming retry driver | `stream_response_with_retry` tracks `emitted_retry_unsafe_event` once non-safe events were yielded | `src/agents/run_internal/model_retry.py:684-891` |
| Replay-safety fail-closed rule | Stateful (`previous_response_id`/`conversation_id`) requests blocked from replay without explicit approval or provider-safe classification | `src/agents/run_internal/model_retry.py:454-479` |
| Provider-retry coordination | `_should_disable_provider_managed_retries` keeps hidden SDK retries off runner-managed replays; `max_retries=0` disables all retries | `src/agents/run_internal/model_retry.py:516-554` |
| Conversation-locked legacy retries | Up to 3 compatibility retries with exponential 1s/2s/4s delays; explicit opt-out via `max_retries=0` | `src/agents/run_internal/model_retry.py:50`, `src/agents/run_internal/model_retry.py:504-513`, `src/agents/run_internal/model_retry.py:618-640` |
| Pre-replay rewind | `rewind_model_request` rewinds tracker input + session items before each replay attempt | `src/agents/run_internal/run_loop.py:2591-2600`, `src/agents/run_internal/run_loop.py:2214-2237` |
| Rewind snapshot bookkeeping | `last_saved_input_snapshot_for_rewind` captured so conversation-lock retries rewind exactly those items | `src/agents/run.py:606-608`, `src/agents/run.py:938-954`, `src/agents/run.py:1050-1062` |
| Tool failure → model feedback (persist-and-continue) | `default_tool_error_function` returns "An error occurred while running the tool…" as tool output | `src/agents/tool.py:1863-1872` |
| Configurable tool-failure formatter | `maybe_invoke_function_tool_failure_error_function` resolves per-tool formatter; `None` means no conversion | `src/agents/tool.py:1964-1981`, `src/agents/tool.py:1902-1911` |
| Tool-not-found behavior switch | `tool_not_found_behavior: "raise_error" \| "return_error_to_model"` enforced during turn resolution | `src/agents/run_config.py:472-479`, `src/agents/run_internal/turn_resolution.py:2183-2205` |
| Tool timeout behaviors | `_FUNCTION_TOOL_TIMEOUT_BEHAVIORS = ("error_as_result", "raise_exception")`; `ToolTimeoutError` carries tool name + seconds | `src/agents/tool.py:1875-1883`, `src/agents/exceptions.py:507-516` |
| Delegation via handoffs | `class Handoff`; `NextStepHandoff` swaps `current_agent` and continues loop; handoff history nesting config | `src/agents/handoffs/__init__.py:126`, `src/agents/run.py:1392-1404`, `src/agents/run_config.py:374-389` |
| Delegation via sub-agents | `Agent.as_tool(..., needs_approval=...)` wraps an agent as a tool with its own `max_turns` | `src/agents/agent.py:583-630`, `src/agents/agent.py:713` |
| Pause/resume (HITL) | `NextStepInterruption` → `RunResult.interruptions`; resumable step stored on `RunState._current_step` | `src/agents/run.py:1170-1215`, `src/agents/run_state.py:826-830` |
| Serializable pause boundary | `RunState` docstring: durable pause/resume snapshot incl. approvals, usage, schema-versioned | `src/agents/run_state.py:749-851` |
| State serialization APIs | `to_json()` and async `from_string()`; `approve()`/`reject()` record human decisions | `src/agents/run_state.py:1704`, `src/agents/run_state.py:2100`, `src/agents/run_state.py:1255-1270` |
| Approval rejection escalation-to-model | `append_approval_error_output` emits synthetic tool output so the model sees why approval failed | `src/agents/run_internal/approvals.py:24-43` |
| Typed escalation exceptions | `MaxTurnsExceeded`, `ModelBehaviorError`, `ModelRefusalError`, input/output/tool guardrail tripwires | `src/agents/exceptions.py:444-572` |
| Per-kind error handlers | `RunErrorHandlers{"max_turns","model_refusal","invalid_final_output"}`; result becomes validated final output | `src/agents/run_error_handlers.py:50-55`, `src/agents/run_internal/error_handlers.py:226-262`, `src/agents/run_internal/error_handlers.py:170-205` |
| Handler-output persistence | Handler output runs final-output hooks + output guardrails and is saved to session/history | `src/agents/run_loop.py` via `finalize_max_turns_handler_output` `src/agents/run_internal/run_loop.py:742-786`, wired `src/agents/run.py:1525-1536` |
| Retry decision logging | `logger.debug("Retrying failed model request in %ss (attempt %s/%s)")` and conversation-lock variant | `src/agents/run_internal/model_retry.py:669-676`, `src/agents/run_internal/model_retry.py:879-886`, `src/agents/run_internal/model_retry.py:631-636` |
| Log redaction policy | `OPENAI_AGENTS_DONT_LOG_MODEL_DATA` / `..._TOOL_DATA` default True; redacted messages when enabled | `src/agents/_debug.py:13-25`, `src/agents/logger.py:31-51` |
| Tracing observability | Agent span per agent; task/turn spans; max-turns span error `{"max_turns": N}`; generic agent-span error attach | `src/agents/run.py:1457-1463`, `src/agents/tracing/create.py:124-139`, `src/agents/run.py:1467-1473`, `src/agents/run_internal/error_handlers.py:75-110` |
| Usage observability of retries | Failed attempts recorded as zero-token request entries via `apply_retry_attempt_usage` | `src/agents/run_internal/model_retry.py:338-350` |
| Timeout as bounded attempt | `ModelSettings.timeout` bounds each model attempt cooperatively via asyncio cancellation | `src/agents/model_settings.py:215-221`, `src/agents/run_internal/model_retry.py:288-315` |
| Test proof of retry behavior | e.g. `test_get_response_with_retry_blocks_unsafe_stateful_timeout_replay`, `..._allows_stateful_retry_when_provider_marks_safe` | `tests/models/test_model_retry.py:179`, `tests/models/test_model_retry.py:1267` |
| Test proof of max-turn handling | 34 tests incl. handler synthesis behavior | `tests/test_max_turns.py:1-34` |
| Documented philosophy | `max_turns` raise semantics + `error_handlers` keys explained in docs | `docs/running_agents.md:41`, `docs/running_agents.md:471-495` |

## Answers to Dimension Questions

**1. Does the agent persist or escalate on failure?**
Both, along distinct axes. Within a turn, transient *model-call* failures persist only if the application opts in via `ModelSettings.retry` (`src/agents/model_settings.py:188`); otherwise the failure escalates after provider-managed transport retries are exhausted (`src/agents/run_internal/model_retry.py:539-550`). Within a turn, *tool* failures persist by default: they are converted to model-visible error outputs so the model can adapt (`src/agents/tool.py:1863-1872`), with an opt-out (`failure_error_function=None`, `src/agents/tool.py:1902-1911`) and an analogous switch for unknown tools (`tool_not_found_behavior`, `src/agents/run_config.py:472-479`). At the loop level, exhaustion of `max_turns` escalates to `MaxTurnsExceeded` unless a handler converts it into a final output (`src/agents/run.py:1465-1490`). Guardrail tripwires always escalate immediately as exceptions (`src/agents/exceptions.py:519-572`). Human-in-the-loop flows pause instead of failing: pending approvals produce `interruptions` and a serializable `RunState` (`src/agents/run.py:1170-1215`, `src/agents/run_state.py:749-762`).

**2. Is persistence configurable?**
Extremely. Granularity includes: (a) retry budget `max_retries` including an explicit zero-disable (`src/agents/run_internal/model_retry.py:524-531`); (b) backoff shape (`initial_delay/max_delay/multiplier/jitter`, `src/agents/retry.py:15-32`); (c) full custom policy callbacks receiving normalized error facts and provider advice (`RetryPolicyContext`, `src/agents/retry.py:142-185`); (d) combinator policies `any`/`all` with hard-veto semantics preserved across composition (`src/agents/retry.py:376-461`); (e) per-tool failure formatters and timeout behaviors (`src/agents/tool.py:705`, `src/agents/tool.py:1875-1878`); (f) run-level knobs `max_turns` (including unlimited), `tool_not_found_behavior` (`src/agents/run_config.py:472-487`). Settings merge hierarchically: agent-level `ModelSettings.resolve(override)` deep-merges retry settings (`src/agents/model_settings.py:284-287`, `src/agents/model_settings.py:377-411`).

**3. Are escalation paths clear?**
Yes — they are typed and enumerable. Three handler kinds (`max_turns`, `model_refusal`, `invalid_final_output`) map 1:1 to exception classes (`src/agents/run_error_handlers.py:29-55`); everything else raises a subclass of `AgentsException` carrying `RunErrorDetails` (`src/agents/exceptions.py:413-441`). Handler results are schema-validated and persisted like normal finals (`src/agents/run_internal/error_handlers.py:170-205`, `src/agents/run_internal/run_loop.py:742-786`), so escalation does not bypass safety checks. One subtlety: a configured handler returning `None` falls through to raising (`src/agents/run.py:1489-1490`; streaming marks `_max_turns_handled = False`, `src/agents/run_internal/run_loop.py:1570-1574`), which is documented but easy to miss.

**4. Are persistence decisions observable?**
Partially, with strong tracing but weak default logging. Every retry decision is logged at DEBUG level with attempt counts and delays (`src/agents/run_internal/model_retry.py:669-676`, `879-886`), invisible at INFO+; traces capture max-turn breaches as span errors (`src/agents/run.py:1467-1473`) and generic failures on agent spans (`src/agents/run_internal/error_handlers.py:75-110`); usage entries record failed attempts as zero-token requests (`src/agents/run_internal/model_retry.py:338-350`); streaming consumers see raw response events. Log content is redacted by default (`OPENAI_AGENTS_DONT_LOG_MODEL_DATA` default True, `src/agents/_debug.py:13-20`), trading diagnosability for safety. There is no structured, machine-readable "persistence decision log" (e.g., a retry event stream) beyond trace spans.

**Autonomy levels (rubric question):** Yes, the agent can be tuned more or less persistent per run/per agent: `max_turns=1..N` tightens the loop, `max_turns=None` removes it entirely (`src/agents/run_config.py:578-582`); retries range from fully disabled to policy-customized (`src/agents/run_internal/model_retry.py:516-554`); tools can be fire-and-forget or approval-gated (`needs_approval`, documented at `docs/human_in_the_loop.md:43`); guardrails add hard stops. However, these compose implicitly — there is no single named "autonomy level" abstraction.

## Architectural Decisions

1. **Opt-in runner retries over always-on retries** (`src/agents/model_settings.py:188-189`, `src/agents/run_internal/model_retry.py:539-550`). Default persistence comes from provider-managed transport retries; runner-managed retries require explicit configuration. This makes retry cost/latency a deliberate choice and lets the SDK coordinate exactly one retry owner per attempt.
2. **Fail-closed replay governance** (`src/agents/run_internal/model_retry.py:403-479`). Aborts, already-emitted stream events, provider-marked unsafe replays, and stateful requests are hard-blocked from blind retry; only explicit `approve_unsafe_replay` or provider-safe classification lifts them, with separate veto types deliberately not folded together (comment block at lines 443-470). This encodes "persistence must never corrupt server-side state."
3. **Rewind-before-replay invariant** (`src/agents/run_internal/run_loop.py:2591-2600`, `src/agents/run.py:606-608`). Every replay rewinds conversation-tracker input and exact persisted session items first, keeping sessions consistent across retries.
4. **Errors-as-feedback over crash-by-default for tools** (`src/agents/tool.py:1863-1872`, `src/agents/run_config.py:472-479`). The model sees failures and can self-correct; crashing remains available via explicit configuration.
5. **Escalation as typed, pluggable handlers** (`src/agents/run_error_handlers.py:44-55`). Instead of ad-hoc fallbacks, terminal conditions dispatch to keyed handlers whose outputs pass the same validation/guardrails as organic finals (`src/agents/run_internal/run_loop.py:742-786`).
6. **Durable pause as a first-class state** (`src/agents/run_state.py:749-851`). Interruptions freeze into a versioned, serializable snapshot with approval ledger, enabling process restarts between decision and continuation — persistence measured in hours/days, not milliseconds.
7. **Compatibility quarantine** (`src/agents/run_internal/model_retry.py:618-651`). Legacy `conversation_locked` retries survive behind a compatibility flag that only an explicit `max_retries=0` disables, isolating historical behavior from the new policy engine.

## Notable Patterns

- **Policy-object DSL with capability metadata**: policies are marked with `retries_safe_transport_errors` / `retries_all_transient_errors` attributes that propagate through `any`/`all` composition (`src/agents/retry.py:186-206`, `396-461`), letting downstream code reason about what a composed policy covers without executing it.
- **Normalized error facts**: provider-specific exceptions are reduced to `ModelRetryNormalizedError` (status/code/retry-after/is_abort/is_network/is_timeout) with provider adapters able to override fields explicitly (`src/agents/retry.py:50-89`, override merge `src/agents/run_internal/model_retry.py:134-155`).
- **Cancellation hygiene**: timeout cancellation drains the model task, shields cleanup, scrubs traceback locals, and discards reference graphs so cancelled provider payloads cannot leak through frames (`src/agents/run_internal/model_retry.py:213-255`, `288-315`; graph discard in `src/agents/exceptions.py:315-405`).
- **Usage truthfulness**: failed attempts are surfaced in usage as zero-token request entries rather than hidden (`src/agents/run_internal/model_retry.py:338-350`).
- **Dual-loop symmetry**: streaming and non-streaming paths implement the same max-turn/handler/escalation semantics separately but behaviorally aligned (`src/agents/run_internal/run_loop.py:1542-1574` vs `src/agents/run.py:1465-1562`), per repo guidance in `AGENTS.md`.

## Tradeoffs

- **Safety vs simplicity in retries**: the three-veto replay system (`src/agents/run_internal/model_retry.py:443-479`) prevents dangerous replays but makes legitimate stateful retries require ceremony (explicit approvals or provider advice); applications wanting aggressive retries face friction by design.
- **Observability vs noise**: retry logging at DEBUG (`src/agents/run_internal/model_retry.py:669`) plus default-on redaction (`src/agents/_debug.py:13-20`) means production operators may see neither attempts nor reasons without configuring both log level and redaction flags.
- **Flexibility vs discoverability**: autonomy is spread across `RunConfig` (~20 fields), `ModelSettings.retry`, per-tool formatters, and approvals; nothing aggregates them into a viewable effective-autonomy profile.
- **Unbounded persistence risk**: `max_turns=None` legitimately exists (`src/agents/run_config.py:580-582`) but removes the only loop-level stop; combined with a stubborn model this yields unbounded cost with no SDK-side circuit breaker other than external cancellation.
- **Implicit legacy path**: conversation-locked retries run even when users configure unrelated retry policies (`src/agents/run_internal/model_retry.py:504-513`), preserving history at the cost of surprise retries.

## Failure Modes / Edge Cases

- **Handler returns None**: configured `max_turns` handler that returns `None` still raises `MaxTurnsExceeded` (`src/agents/run.py:1489-1490`); in streaming, `_max_turns_handled` flips False and the event queue closes (`src/agents/run_internal/run_loop.py:1570-1574`).
- **Mid-stream failure after visible output**: once any event beyond `response.created/in_progress` was yielded (`_RETRY_SAFE_STREAM_EVENT_TYPES`, `src/agents/run_internal/model_retry.py:51`), retry is permanently vetoed for that attempt (`emitted_retry_unsafe_event`, `src/agents/run_internal/model_retry.py:806-807`, `421-425`) — correct for consistency, but consumers must handle the raised error themselves.
- **Session write failure during escalation**: if persisting guardrail-trip items fails, the original tripwire is replaced by a redacted persistence error to avoid leaking payloads (`src/agents/run.py:1312-1316`, redaction helper `src/agents/run_internal/run_loop.py:674-739`).
- **Timeout vs provider retry interaction**: an explicit no-retry budget also disables hidden provider retries (`src/agents/run_internal/model_retry.py:524-531`), so "disable retries" truly means zero attempts-after-failure.
- **Nested-agent approval surfacing**: approvals raised inside `Agent.as_tool` bubble to the outer run's interruptions and must be resolved on the outer `RunState` (`docs/human_in_the_loop.md:5-7`), which can surprise callers expecting inner-run resolution.

## Future Considerations

- Promote persistence-decision telemetry above DEBUG or emit structured retry/span events keyed by attempt, delay, veto reason, and policy identity, so autonomy tuning can be verified operationally.
- Consider an aggregate "effective autonomy" descriptor (turns budget, retry budget, approval mode, guardrails) computed from merged configs to aid auditing.
- A bounded-cost circuit breaker (token/usage ceiling alongside `max_turns`) would complement the existing `max_turns=None` escape hatch.
- First-class replanning hooks (e.g., planner-in-the-loop on repeated tool failure) currently have to be built atop error handlers + `call_model_input_filter` (`src/agents/run_config.py:438-446`).

## Questions / Gaps

- No evidence found of a dedicated replanning module or strategy object; search covered `run_internal/`, `run_config.py`, `retry.py`, and `docs/` (terms: replan, plan, strategy). Replanning is only implicit via error feedback to the model.
- No evidence found of persistence-decision metrics exported beyond tracing spans and Python logging (no counters/event bus); searched `tracing/`, `usage.py`, `logger.py`.
- Whether provider adapters uniformly populate `ModelRetryAdvice.replay_safety` could not be confirmed for all providers; evidence shows the OpenAI adapter path (`src/agents/models/_openai_retry.py` referenced from `src/agents/run_internal/model_retry.py:16-24`), while third-party adapters in `extensions/models/` (e.g., `litellm_model.py`) were not audited for retry-advice parity within this dimension's scope.

---

Generated by `dimension 23.02-persistence-vs-escalation-philosophy` against `openai-agents-sdk`.

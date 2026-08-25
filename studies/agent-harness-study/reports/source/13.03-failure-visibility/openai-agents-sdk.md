# Source Analysis: openai-agents-sdk

## Dimension 13.03: Failure Visibility

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk (OpenAI Agents SDK, Python) |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (asyncio, pydantic, OpenAI Python SDK / httpx2) |
| Analyzed | 2026-08-25 |

> All citations below are workspace-relative to the studied source root `studies/agent-harness-study/sources/openai-agents-sdk/`.

## Summary

The SDK implements failure visibility as an explicit four-surface contract with per-stakeholder redaction policy:

1. **Model**: tool failures are converted into model-visible error *strings* returned as tool outputs (`src/agents/tool.py:1863-1872`, default formatter; `src/agents/run_internal/tool_execution.py:2117-2131` for schema-aware encoding), designed so the model can retry. A `failure_error_function` of `None` opts a tool out and re-raises instead (`src/agents/tool.py:1902-1911`).
2. **User/developer (caller)**: a typed exception hierarchy (`AgentsException` subclasses in `src/agents/exceptions.py:434-572`) with structured payloads, plus a `RunErrorDetails` snapshot attached to every failed run (`src/agents/run.py:2132-2144`, `src/agents/run_internal/run_loop.py:1955-1965`) with a payload-safe pretty printer (`src/agents/util/_pretty_print.py:42-53`).
3. **Developer logs**: two named Python loggers (`openai.agents`, `openai.agents.tracing`) with policy-gated helpers that drop exception bodies/tracebacks unless data logging is explicitly enabled (`src/agents/logger.py:7-11`, `31-51`; flags in `src/agents/_debug.py:12-28`, secure-by-default ON).
4. **Operator**: span-level error propagation (`SpanError` on every failed model/tool/guardrail step) exported through a batched trace pipeline to the OpenAI Traces dashboard, extensible via custom processors (`src/agents/tracing/spans.py:373-406`, `src/agents/tracing/processors.py:541-763`, `docs/tracing.md:3,148-163`).

A distinctive theme is **redaction-aware visibility**: the same failure event is rendered differently per surface — full detail to the raising caller, message-only records in logs when `DONT_LOG_*` is set (`src/agents/logger.py:41-42`), fixed placeholder text ("Error details are redacted.") in traces when sensitive-data tracing is off (`src/agents/util/_error_tracing.py:46-53`), and even traceback detachment and exception-graph scrubbing so redacted errors cannot leak payload data through `__context__` or frame locals (`src/agents/exceptions.py:140-215`, `315-405`). This behavior is heavily tested (`tests/test_error_logging_redaction.py`, `tests/test_tracing_errors.py`, `tests/test_provider_span_errors.py`).

## Rating

**Score: 8 / 10**

Rationale against the rubric:
- **Clear model with tests (7–8 band)**: each stakeholder has a distinct, documented surface — model-visible error strings (`src/agents/tool.py:1863`), typed exceptions + `RunErrorDetails` (`src/agents/exceptions.py:413-431`), gated loggers (`src/agents/logger.py`), and span errors/traces (`src/agents/tracing/spans.py:396-406`). Redaction semantics are enforced by dedicated tests, e.g. "redacted tracing never stringifies the exception" (`tests/test_tracing_errors.py:764-774`) and provider span error redaction (`tests/test_provider_span_errors.py:99,259-279`), plus end-to-end streamed-redaction tests (`tests/test_error_logging_redaction.py:1297-1341,1442-1497`).
- **Explicit interfaces**: `RunConfig.tool_error_formatter`, `tool_not_found_behavior`, `output_guardrail_blocked_message`, `trace_include_sensitive_data`, `error_handlers` (`src/agents/run_config.py:404-496`, `src/agents/run_error_handlers.py:50-56`); per-tool `failure_error_function`/timeout policies (`src/agents/tool.py:705,714-715,1886-1911`).
- **Operational safeguards**: batched export with backoff/jitter and non-fatal failure logging (`src/agents/tracing/processors.py:158-221`), queue-drop warnings (`604,621`), shutdown flush deadlines (`623-668`), ingest sanitization/truncation (`261-319`).
- Why not 9–10: no metrics/alerting layer beyond traces; debug flags are read once at import time and cannot be changed at runtime (`src/agents/_debug.py:20-27`); `ConsoleSpanExporter` bypasses the logging framework using raw `print()` (`src/agents/tracing/processors.py:30-41`); trace telemetry can be silently dropped under queue pressure (logged warning only).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Model sees tool failures as retryable strings | `default_tool_error_function` returns "An error occurred while running the tool. Please try again. Error: …", with special JSON-parse guidance | `src/agents/tool.py:1863-1872` |
| Per-tool failure policy (return vs raise) | `_use_default_failure_error_function` resolution; `None` disables return-to-model | `src/agents/tool.py:1902-1911`, `1886-1899` |
| Model sees timeout errors | `default_tool_timeout_error_message`: "Tool 'X' timed out after N seconds."; `ToolTimeoutError` carries name+seconds | `src/agents/tool.py:1881-1883`, `src/agents/exceptions.py:507-516` |
| Model sees rejection reason | `DEFAULT_APPROVAL_REJECTION_MESSAGE = "Tool execution was not approved."`; custom reason supported; formatter override | `src/agents/tool.py:194`, `src/agents/run_internal/tool_execution.py:1230-1297`, `src/agents/run_config.py:448-452` |
| Missing-tool errors configurable | `tool_not_found_behavior="return_error_to_model"` produces model-visible output "Tool 'X' not found." | `src/agents/run_config.py:472-478`, `src/agents/run_internal/turn_resolution.py:258-306` |
| Schema-safe programmatic error encoding | `function_tool_error_output` wraps string errors as `{"error": ...}` JSON for program callers | `src/agents/run_internal/items.py:803-821` |
| Span input/output only when sensitive tracing on | `span_fn.span_data.input = tool_call.arguments` guarded by `config.trace_include_sensitive_data` | `src/agents/run_internal/tool_execution.py:1818-1819,1854-1855` |
| Tool exceptions → span errors, wrapped for user | catch-all attaches `SpanError("Error running tool", {tool_name, error})`, re-raises `AgentsException` as-is else wraps as `UserError(f"Error running tool {name}: {e}")` | `src/agents/run_internal/tool_execution.py:1839-1852` |
| Trace-safe error strings | `get_trace_error` swaps detail for "Tool execution failed. Error details are redacted." | `src/agents/util/_tool_errors.py:5-14`, `src/agents/util/_error_tracing.py:46-53` |
| Handled (formatted) tool errors still recorded | handled-error reporter attaches span error and policy-gated logs | `src/agents/tool.py:1783-1826` |
| Cancellation surfaced to model or span | cancelled invocation routed through failure formatter; span gets "Tool execution cancelled" | `src/agents/run_internal/tool_execution.py:2069-2095`, `src/agents/tool.py:1944-1961` |
| Typed user-facing exceptions | `AgentsException` base; MaxTurnsExceeded, ModelBehaviorError, ModelRefusalError (carries refusal text), ModelTimeoutError, UserError, tripwires | `src/agents/exceptions.py:434-572` |
| Failure context snapshot for callers | `RunErrorDetails` attached to raised exceptions on both run paths | `src/agents/run.py:2122-2145`, `src/agents/run_internal/run_loop.py:1948-1966`, `src/agents/exceptions.py:413-441` |
| Payload-safe pretty print | counts only (items/responses/guardrail results), no payload dump | `src/agents/util/_pretty_print.py:42-53` |
| Streaming errors delivered as exceptions | `stream_events()` drains queued events then raises stored exception; docstring documents raise contract | `src/agents/result.py:882-891,1015-1024`, `src/agents/result.py:1047-1075` |
| No stream error event type | `StreamEvent` union lacks an error variant (voice pipeline does have one) | `src/agents/stream_events.py:51-61`, `src/agents/voice/events.py:32-38` |
| Generic agent-span error on any failed run | `attach_generic_agent_error` marks agent span "Error in agent run"; skips more-specific existing errors; never alters what is raised | `src/agents/run_internal/error_handlers.py:43-110`, applied at `run_loop.py:1941-1947,1967-1975` and `run.py:2122-2131` |
| Failed model calls always mark their span | `model_span_errors` records terminal/streaming failures that may never be raised | `src/agents/util/_error_tracing.py:89-157` |
| Redaction without invoking hostile `__str__` | `_model_error_text` stringifies only when exporting; placeholder otherwise; unrenderable fallback | `src/agents/util/_error_tracing.py:70-86`, test at `tests/test_tracing_errors.py:764-774` |
| Log redaction policy gates | `log_model_action_*`/`log_tool_action_*` drop exc body+traceback when flags set but keep message; diagnostic extra under `openai_agents_diagnostic_context` | `src/agents/logger.py:31-51,77-246`, field at `:11` |
| Secure-by-default logging flags | `OPENAI_AGENTS_DONT_LOG_MODEL_DATA` / `OPENAI_AGENTS_DONT_LOG_TOOL_DATA` default True | `src/agents/_debug.py:12-28` |
| Named loggers, no handlers attached | `logging.getLogger("openai.agents")`, `"openai.agents.tracing"`; documented config path | `src/agents/logger.py:7`, `src/agents/tracing/logger.py:3`, `docs/config.md:199-228` |
| Operator trace export | `BackendSpanExporter` posts to `https://api.openai.com/v1/traces/ingest` with retries/backoff/jitter; all failures "[non-fatal]" logged | `src/agents/tracing/processors.py:44-45,118-221` |
| Batch processor safeguards | background thread, queue-full drops with warnings, exporter exceptions contained so worker survives, shutdown/flush deadlines | `src/agents/tracing/processors.py:541-718` |
| Ingest-side sanitization | 100 KB field truncation and usage-key allowlist before posting to OpenAI endpoint | `src/agents/tracing/processors.py:46-54,258-319` |
| Span error export shape | `Span.export()` includes `"error": self._error` | `src/agents/tracing/spans.py:373-406` |
| Guardrail-blocked output placeholder | `OUTPUT_GUARDRAIL_BLOCKED_TOOL_OUTPUT = "Output withheld by an output guardrail."`; results sanitized data-free; custom placeholder hook | `src/agents/run_internal/blocked_output.py:40,137-158,164-230`, `src/agents/run_config.py:489-496` |
| Run-level error handlers | `RunErrorHandlers{max_turns, model_refusal, invalid_final_output}` convert failures into final outputs with run data supplied | `src/agents/run_error_handlers.py:16-64`, `src/agents/run_internal/error_handlers.py:113-136,226-262` |
| MCP log names scrub credentials | URL query/fragment stripping for MCP server log names | `src/agents/mcp/_logging.py:11-40` |
| Tracing disable knobs | `OPENAI_AGENTS_DISABLE_TRACING` env (cached once), `set_tracing_disabled`, `RunConfig.tracing_disabled` | `src/agents/tracing/provider.py:341-352`, `src/agents/run_config.py:397-399` |
| Sensitive-data trace default | `OPENAI_AGENTS_TRACE_INCLUDE_SENSITIVE_DATA` defaults to true | `src/agents/run_config.py:53-56,404-410` |
| Docs tie behavior to implementation | logging/redaction guide matches `_debug.py` flags and logger names | `docs/config.md:199-248` |

## Answers to Dimension Questions

**1. Is the model informed of failures?**
Yes, by design. Function-tool exceptions become model-visible result strings through the default `failure_error_function` (`src/agents/tool.py:1863-1872`), including targeted guidance for malformed JSON arguments ("Please try again with valid JSON"). Approval rejections return "Tool execution was not approved." with optional caller-provided reason (`src/agents/run_internal/tool_execution.py:1230-1297`). Tool timeouts produce explicit timeout messages (`src/agents/tool.py:1881-1883`), and cancellation can be converted into a model-visible result (`src/agents/run_internal/tool_execution.py:2069-2095`). Opt-outs exist: setting `failure_error_function=None` makes errors raise instead (`src/agents/tool.py:1902-1911`), and `tool_not_found_behavior` chooses between raising `ModelBehaviorError` and returning "Tool 'X' not found." to the model (`src/agents/run_config.py:472-478`). Program-caller tools get errors encoded as `{"error": ...}` JSON to satisfy output schemas (`src/agents/run_internal/items.py:803-821`).

**2. Is the user informed appropriately?**
Yes, with a layered approach. Callers get a typed exception hierarchy with structured fields (e.g., `ModelRefusalError.refusal`, `ToolTimeoutError.timeout_seconds`) rather than opaque strings (`src/agents/exceptions.py:444-572`). Every failed run attaches `RunErrorDetails` (input, new items, raw responses, guardrail results) to the exception, pretty-printed as counts-only summaries (`src/agents/util/_pretty_print.py:42-53`), so users see scope without accidental payload exposure. Streaming consumers receive failures as exceptions raised from `stream_events()` after queued events drain (`src/agents/result.py:940-951,1015-1024`); there is no dedicated error stream-event type (`src/agents/stream_events.py:61`), which is a minor gap versus the voice pipeline's `VoiceStreamEventError` (`src/agents/voice/events.py:32-38`).

**3. Can developers debug failures?**
Yes. Developers can (a) read `exc.run_data` snapshots, (b) enable full-detail logs by setting `OPENAI_AGENTS_DONT_LOG_MODEL_DATA=0` / `..._TOOL_DATA=0` (`docs/config.md:230-248`), and (c) inspect spans, where every failing tool/model/handoff step records a `SpanError` (`src/agents/run_internal/tool_execution.py:1839-1849`; `src/agents/util/_error_tracing.py:125-157`). The generic agent-span marker ensures a failed run is visible even when a child already carries the specific error (`src/agents/run_internal/error_handlers.py:47-110`). Notably, debugging hooks respect redaction: enabling sensitive tracing is required to see real exception text in spans, and the code deliberately avoids invoking exception `__str__` when the output would be discarded (`src/agents/util/_error_tracing.py:70-86`).

**4. Can operators detect failure patterns?**
Partially, via traces rather than metrics. The default pipeline exports all spans/errors to the OpenAI Traces dashboard (`docs/tracing.md:3`), with batch retries, backoff, and deadline-bounded flushes (`src/agents/tracing/processors.py:118-221,623-668`), and operators can add replacement processors to forward to other backends (`docs/tracing.md:148-163`). There is no built-in metrics aggregation, alerting, or health endpoint — pattern detection depends on the dashboard or custom processors. Telemetry self-failures are non-fatal and warn-logged, which protects the app but means operators must watch logs to notice dropped batches (`src/agents/tracing/processors.py:604,621,709-717`).

**Configurability of detail levels:** Strong. Per-run (`trace_include_sensitive_data`, `tracing_disabled`, `tool_error_formatter`, `tool_not_found_behavior`, `output_guardrail_blocked_message`, `error_handlers` — `src/agents/run_config.py:397-496`), per-tool (`failure_error_function`, `timeout_behavior` of `error_as_result` vs `raise_exception`, `timeout_error_function` — `src/agents/tool.py:705,713-715`), global env flags for log/trace data (`src/agents/_debug.py:12-28`, `src/agents/tracing/provider.py:341-352`), and standard Python logging levels (`docs/config.md:199-228`).

## Architectural Decisions

1. **Errors-as-data to the model, exceptions to the caller.** Tool failures are formatted into strings fed back into the conversation (`src/agents/tool.py:1863-1872`) while simultaneously being recorded on spans and logs (`src/agents/tool.py:1783-1826`); only an explicit opt-out (`failure_error_function=None`) lets exceptions escape to the caller mid-run.
2. **One redaction policy, three renderings.** A single failure event renders as full text (caller/spans when sensitive tracing on), a fixed placeholder in traces/logs otherwise (`src/agents/util/_error_tracing.py:46-53`, `src/agents/run_internal/error_handlers.py:94-101`), and message-only log lines when data logging is disabled (`src/agents/logger.py:41-42`).
3. **Redacted exceptions as first-class objects.** Marking (`_mark_error_data_redacted`), traceback clearing/detachment, exception-graph scrubbing, and replacement with payload-free errors at public boundaries (`src/agents/exceptions.py:140-215,315-410`) ensure suppressed display cannot still leak via `__context__`, frames, or nested groups — mirroring the repo's own security guidance in `AGENTS.md` ("Suppressing display with `raise ... from None` is not enough").
4. **Failure knowledge recorded at point-of-knowledge.** Streaming providers learn about terminal failures before raising; `model_span_errors`/`record_model_error_on_span` keep spans accurate even when the consumer closes the generator early (`src/agents/util/_error_tracing.py:96-157`).
5. **Telemetry isolation.** All tracing-export failures are non-fatal, contained, and deadline-bounded so observability problems can never take down agent runs (`src/agents/tracing/processors.py:696-717`).

## Notable Patterns

- **Policy-gated logging helpers**: `log_model_action_*` / `log_tool_action_*` / `log_model_and_tool_action_*` centralize the decision to include `exc` + `exc_info` vs message-only (`src/agents/logger.py:77-246`), with lazy diagnostic extras attached under the `openai_agents_diagnostic_context` record key (`src/agents/logger.py:9-28`).
- **Defense-in-depth against hostile payloads**: exact built-in descriptor access instead of attribute lookups when handling redacted exceptions, so provider-controlled `__getattr__`/metaclass callbacks cannot observe or mutate redaction (`src/agents/exceptions.py:56-124,292-312`); tests assert redacted paths never call `__str__`/`__repr__` (`tests/test_error_logging_redaction.py:93-150`).
- **Structured span errors everywhere**: `SpanError(message, data)` with `tool_name` and redaction-aware `error` text attached uniformly across function tools, shell/custom/apply-patch/computer actions, handoff filters, and guardrail blocks (`src/agents/run_internal/tool_execution.py:1840-1849`, `src/agents/run_internal/tool_actions.py:146-163,585-605,773-789,1004-1021`).
- **Data-free replacements over deletion**: guardrail-blocked terminal outputs are rebuilt from allowlisted fields with a fixed placeholder so replay/persistence stay provider-valid while removing content (`src/agents/run_internal/blocked_output.py:112-158,464-539`).
- **Credential hygiene in diagnostics**: MCP server names used in logs have URLs stripped of credentials/query/fragments (`src/agents/mcp/_logging.py:11-40`).
- **Test-backed visibility guarantees**: dedicated suites assert redaction of span errors (`tests/test_tracing_errors.py:620,764-774`), provider span errors (`tests/test_provider_span_errors.py:99-279`), and end-to-end streamed/user-visible redacted messages like `"Error details are redacted."` (`tests/test_error_logging_redaction.py:1677,1739`).

## Tradeoffs

- **Secure defaults reduce out-of-the-box diagnosability**: with `DONT_LOG_MODEL_DATA`/`DONT_LOG_TOOL_DATA` default True (`src/agents/_debug.py:13-17`) and traces defaulting to *include* sensitive data (`src/agents/run_config.py:55`), the defaults are asymmetric — logs hide payloads while traces carry them. Teams must consciously configure both to match their posture.
- **Import-time flag caching**: `DONT_LOG_*` constants are computed once at module import (`src/agents/_debug.py:20-27`), so runtime toggling requires monkeypatching (as tests do); the tracing env-disable flag is similarly cached on first use (`src/agents/tracing/provider.py:341-352`). Long-lived processes cannot change verbosity without restarts.
- **Exceptions, not events, for streaming failures**: simple for try/except users, but UIs consuming `StreamEvent` must handle termination via exception rather than a typed error event (`src/agents/stream_events.py:61`, `src/agents/result.py:1015-1024`).
- **Non-fatal telemetry drops**: protecting the run loop means silent-ish data loss under queue saturation or exporter outages; only warnings remain (`src/agents/tracing/processors.py:604,621,676-679`).
- **Complexity cost of redaction guarantees**: ~900-line blocked-output machinery and exception-graph scrubbing are hard to audit, though heavily tested; this is deliberate spend to make "invisible failure data" provably invisible.

## Failure Modes / Edge Cases

- **Unformattable exceptions**: an exception whose `__str__` raises yields `"Unrenderable <Type>"` on model spans (`src/agents/util/_error_tracing.py:79-86`) or `"Error details are unavailable."` on agent spans (`src/agents/run_internal/error_handlers.py:63-72`), tested at `tests/test_tracing_errors.py:756-774`.
- **Formatter misbehavior contained**: a user-supplied `tool_error_formatter` that throws or returns non-strings falls back to the SDK default message and logs the failure (`src/agents/run_internal/tool_execution.py:1275-1295`, `src/agents/run_internal/turn_resolution.py:288-304`); the blocked-message formatter's exceptions/coroutines are neutralized (`src/agents/run_internal/blocked_output.py:210-227`).
- **No active span**: attaching a span error when tracing is inactive downgrades to a (redaction-aware) warning instead of failing the run (`src/agents/util/_error_tracing.py:60-67`).
- **Late sibling/post-invoke tool failures** after one parallel tool fails are merged by priority without masking the root cause, and orphaned task exceptions are reported through the asyncio loop handler (`src/agents/run_internal/tool_execution.py:258-304,207-255`).
- **Redacted-error catchability**: redacted cancellations remain catchable as Exceptions via a payload-free subclass, preserving control-flow semantics while dropping data (`src/agents/exceptions.py:40-41,242-278`; test `tests/test_error_logging_redaction.py:1353+`).
- **Traceback loss tradeoff**: clearing tracebacks on redacted errors improves safety but removes stack context for the caller; the type/message skeleton (e.g., `ModelBehaviorError("Error details are redacted.")`) is retained (`src/agents/exceptions.py:204-215`).

## Future Considerations

- Add a metrics/counters interface (or OTel bridge) so operators can alert on failure rates without scraping traces; today only trace processors exist (`docs/tracing.md:148-163`).
- Make `DONT_LOG_*` and tracing-disable flags hot-reloadable (read per-use or expose setters) for long-running services (`src/agents/_debug.py:20-27`).
- Consider an error stream-event variant (mirroring `VoiceStreamEventError`, `src/agents/voice/events.py:32-38`) for structured streaming failure reporting.
- Route `ConsoleSpanExporter` through the logging framework instead of `print()` (`src/agents/tracing/processors.py:30-41`).
- Document the asymmetric defaults (logs hide data, traces include it by default) side by side to prevent misconfiguration (`docs/config.md:179-248`).

## Questions / Gaps

- **Operator dashboards beyond OpenAI's**: no first-party OpenTelemetry exporter ships in-tree; support is via the generic `TracingProcessor` interface (`src/agents/tracing/processor_interface.py`). Searched `src/agents/tracing/` for "otel"/"opentelemetry" integrations — none found; external processors are referenced only in docs (`docs/tracing.md:204` "External tracing processors list").
- **Log-based failure metrics**: no evidence of structured error codes or machine-readable log envelopes beyond the single `openai_agents_diagnostic_context` extra (`src/agents/logger.py:11`); pattern detection would rely on parsing prose messages.
- **Realtime failure surfaces**: Realtime sessions were not audited in depth here (out of the core run-loop scope examined); `src/agents/realtime/model_events.py` likely contains its own error events — flagged as unverified.

---

Generated by `Dimension 13.03: Failure Visibility` against `openai-agents-sdk`.

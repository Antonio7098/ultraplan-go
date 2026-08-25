# Source Analysis: agent-framework

## Dimension 13.03: Failure Visibility

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (`agent-framework-core`) and .NET (C#, `Microsoft.Agents.AI*`) monorepo; Go directory is a README placeholder (`go/README.md`) |
| Analyzed | 2026-08-24 |

## Summary

Agent Framework implements failure visibility as a four-layer, stakeholder-specific system rather than a single error channel. **Model-facing**: tool exceptions are converted into sanitized function results ("Error: Function failed.") by default, with full exception detail gated behind an explicit `include_detailed_errors` opt-in flag and a consecutive-error circuit breaker that stops the tool loop with a logged warning (`python/packages/core/agent_framework/_tools.py:1412-1434`, `_tools.py:2718-2731`). **Developer-facing**: a typed exception hierarchy auto-logs at a per-exception configurable level (default DEBUG) with inner-exception chains (`python/packages/core/agent_framework/exceptions.py:15-38`), and Python workflows emit structured failure events carrying `error_type`, `message`, `traceback`, and `executor_id` before re-raising (`python/packages/core/agent_framework/_workflows/_events.py:70-99`, `_workflow.py:601-623`). **Operator-facing**: OpenTelemetry spans capture `error.type` attributes, recorded exceptions, and `StatusCode.ERROR` status, with raw message/tool payloads additionally gated behind an explicit sensitive-data opt-in env var (`python/packages/core/agent_framework/observability.py:2788-2791`, `observability.py:920-934`). **User-facing**: the DevUI server converts execution failures into typed `error` events on the event stream plus `AgentFailedEvent` payloads (`python/packages/devui/agent_framework_devui/_executor.py:415-422`). The .NET side mirrors the model-facing policy for skills (`dotnet/src/Microsoft.Agents.AI/Skills/AgentSkillsProviderOptions.cs:33`) and models workflow failures as typed events (`ExecutorFailedEvent.cs:12-19`, `WorkflowErrorEvent.cs:13`, `SubworkflowErrorEvent.cs:12-17`). The design is deliberate about *who may see what*: detail is opt-in for the model, payload content is opt-in for telemetry, but structural failure facts (type, message, location) are always visible.

## Rating

**8 / 10.**

Rationale against the rubric:

- The model/user/developer/operator split is explicit in code, not incidental: sanitization default with documented opt-in (`_tools.py:1399`, `_tools.py:1427-1428`), per-exception log-level control (`exceptions.py:25`), semconv-based OTel attributes (`observability.py:232`, `304`).
- Behavior is pinned by tests covering both sides of the `include_detailed_errors` switch, including argument-validation errors (`python/packages/core/tests/core/test_function_invocation_logic.py:2346-2402`, `test_function_invocation_logic.py:2482-2537`) and workflow state/error events (`python/packages/core/tests/workflow/test_workflow_states.py`).
- Operational safeguards exist: consecutive-error circuit breaker (`_tools.py:1352-1353`, `2718-2731`), convergence limits raising typed exceptions (`python/packages/core/agent_framework/_workflows/_runner.py:177`), and sticky instrumentation disable that `enable_sensitive_telemetry()` refuses to silently override without `force=True` (`observability.py:1228-1232`).

Not scored 9-10 because: one DevUI streaming path logs the exception but does not yield an error event (a literal `# Could yield error event here` comment, `python/packages/devui/agent_framework_devui/_executor.py:255-256`); workflow failure/diagnostic events are intentionally not forwarded across the `workflow.as_agent()` boundary, so agent-level callers lose structured failure visibility unless they consume the workflow stream directly (`_events.py:133-143`); and this analysis did not find equivalent Python-side documentation of the .NET HTTP/hosting error mapping (see Gaps).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Model sees sanitized tool errors by default | `_function_execution_error_result` returns "Error: Function failed."; full detail only when `include_detailed_errors=True`; raw exception string still attached via `exception=` field on the result content | `python/packages/core/agent_framework/_tools.py:1412-1434` |
| Detail-level configurability (model) | `FunctionInvocationConfiguration` fields `max_consecutive_errors_per_request`, `terminate_on_unknown_calls`, `include_detailed_errors`; default `include_detailed_errors=False`; config validation | `python/packages/core/agent_framework/_tools.py:1352-1408` |
| Developer sees tool failures in logs | WARNING log names the function, states an error result was returned to model, includes `%r` of the exception | `python/packages/core/agent_framework/_tools.py:1420-1425` |
| Circuit breaker for repeated failures | `_update_consecutive_error_count` stops further function calls after max consecutive errors and logs a warning | `python/packages/core/agent_framework/_tools.py:2718-2731` |
| Error outcome queryable | `had_errors` property exposes whether any execution produced an error result | `python/packages/core/agent_framework/_tools.py:1895-1896` |
| Approval requests not masked as errors | `UserInputRequiredException` carries `contents` so sub-agent approval requests propagate instead of being swallowed as a generic tool error; constructed with `log_level=None` | `python/packages/core/agent_framework/exceptions.py:184-211` |
| Typed exception hierarchy with self-logging | `AgentFrameworkException.__init__` logs message (default DEBUG=10, per-instance override incl. `None`) with `exc_info=inner_exception` | `python/packages/core/agent_framework/exceptions.py:15-38` |
| Exception taxonomy per subsystem | Agent / ChatClient / Integration / Content / Tool / Middleware / Settings / Workflow families | `python/packages/core/agent_framework/exceptions.py:44-262` |
| Structured workflow failure details | `WorkflowErrorDetails` dataclass: `error_type`, `message`, optional `traceback` (formatted from live exception), `executor_id`, `extra` | `python/packages/core/agent_framework/_workflows/_events.py:70-99` |
| Failure event taxonomy | Event types include `failed` (terminal), `error` (non-fatal user code), `warning`, `executor_failed` | `python/packages/core/agent_framework/_workflows/_events.py:104-130` |
| Failure surfacing order guaranteed | On exception: drain pending events → yield structured `failed` event → yield `FAILED` status → add OTel `workflow.error` span event with `error.message`/`error.type` → record exception on span → re-raise | `python/packages/core/agent_framework/_workflows/_workflow.py:601-623` |
| Boundary filtering reduces caller visibility | `AGENT_FORWARDED_EVENT_TYPES` forwards only output/intermediate/request_info across `as_agent()`; lifecycle + diagnostic + executor bookkeeping (incl. failures) stay inside | `python/packages/core/agent_framework/_workflows/_events.py:133-143` |
| Typed checkpoint/convergence failures | `WorkflowConvergenceException` after non-convergence; multiple `WorkflowCheckpointException` raise sites name the failing executor/checkpoint | `python/packages/core/agent_framework/_workflows/_runner.py:177`, `304-429` |
| Operator: span error convention | `capture_exception` sets `error.type` attribute, records the exception event, sets `StatusCode.ERROR` with repr | `python/packages/core/agent_framework/observability.py:2788-2791` |
| Operator: streaming error captured on span | Stream error skips final-response hooks and is captured on the chat span; outer exception path captures then re-raises | `python/packages/core/agent_framework/observability.py:1710-1728` |
| Operator: MCP span errors | `set_mcp_span_error` sets `error.type` attribute + `StatusCode.ERROR` with JSON-RPC description | `python/packages/core/agent_framework/observability.py:2361-2374` |
| Operator: semconv attribute names | `ERROR_TYPE="error.type"`, `WORKFLOW_ERROR="workflow.error"`, build-error attributes | `python/packages/core/agent_framework/observability.py:232`, `304`, `309-311` |
| Sensitive-detail gating (operator) | Message/tool-payload attributes only recorded when `SENSITIVE_DATA_ENABLED` (= instrumentation AND sensitive data); env var `ENABLE_SENSITIVE_DATA` opt-in | `python/packages/core/agent_framework/observability.py:920-934`, `1297`, `1741-1762` |
| Sticky disable safeguard | `enable_sensitive_telemetry()` no-ops when instrumentation was explicitly disabled unless `force=True` | `python/packages/core/agent_framework/observability.py:1228-1240` |
| User (DevUI): error events on stream | Entity/workflow/HIL/checkpoint failures each `logger.exception/error` then yield `{"type": "error", ...}`; agent path yields both `AgentFailedEvent` and backward-compat dict | `python/packages/devui/agent_framework_devui/_executor.py:324-326`, `415-422`, `545-547`, `572-574`, `596-597` |
| User (DevUI): inconsistent streaming path | Streaming execution logs exception but only comments "Could yield error event here" — client receives no error event on this path | `python/packages/devui/agent_framework_devui/_executor.py:255-256` |
| .NET model-facing parity | Skills provider mirrors `FunctionInvokingChatClient.IncludeDetailedErrors` policy via its own option | `dotnet/src/Microsoft.Agents.AI/Skills/AgentSkillsProviderOptions.cs:23-33`, `dotnet/src/Microsoft.Agents.AI/Skills/AgentSkillsProvider.cs:450` |
| .NET workflow failure events | `ExecutorFailedEvent(executorId, err)`; base `WorkflowErrorEvent(Exception?)`; `SubworkflowErrorEvent(subworkflowId, e)` adds scope id | `dotnet/src/Microsoft.Agents.AI.Workflows/ExecutorFailedEvent.cs:12-19`, `WorkflowErrorEvent.cs:13`, `SubworkflowErrorEvent.cs:12-18` |
| .NET runner converts throw to event | In-proc runner catches generic `Exception` and raises `WorkflowErrorEvent` instead of crashing the stream | `dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcessRunner.cs:230-234` |
| Tests pin the sanitization contract | Tests assert generic messages with `False` (default) vs detailed messages with `True`, for both execution and argument-validation failures | `python/packages/core/tests/core/test_function_invocation_logic.py:2346-2402`, `2482-2537` |

## Answers to Dimension Questions

**1. Is the model informed of failures?**
Yes, deterministically. A tool exception never escapes mid-loop by default; it is converted into a `function_result` content whose text is the constant `"Error: Function failed."` plus (opt-in) the exception string, and the raw exception string is always attached out-of-band in the `exception=` field of the content (`python/packages/core/agent_framework/_tools.py:1412-1434`). The model also gets loop-level feedback: after N consecutive errors the loop abandons further calls and forces a text response, logging the threshold (`_tools.py:2718-2731`). Unknown-function behavior is itself a config knob (`terminate_on_unknown_calls`, `_tools.py:1354-1355`). Special control-flow exceptions are deliberately excluded from this conversion so approval requests surface as first-class content rather than error strings (`python/packages/core/agent_framework/exceptions.py:184-211`).

**2. Is the user informed appropriately?**
In DevUI, yes for most paths: entity execution, agent runs, human-in-the-loop submission, checkpoint resume, and workflow execution all catch failures, log them, and yield typed `error` stream events with a message and entity id (`python/packages/devui/agent_framework_devui/_executor.py:324-326`, `415-422`, `545-547`, `572-574`, `596-597`); the agent path additionally yields a structured `AgentFailedEvent`. One gap: the streaming execution path only logs (`logger.exception`) and does not yield an error event — the code comment `# Could yield error event here` marks it as unfinished (`_executor.py:255-256`). For workflow-as-agent consumers, `failed`/`executor_failed` events are intentionally filtered out of the forwarded event set (`python/packages/core/agent_framework/_workflows/_events.py:133-143`), so end users of the agent facade see a raised exception but not the intermediate structured failure stream.

**3. Can developers debug failures?**
Strongly yes. Three complementary channels: (a) the typed hierarchy self-logs with inner-exception chains at a per-class overridable level (`python/packages/core/agent_framework/exceptions.py:21-38`); (b) workflow failures produce `WorkflowErrorDetails` with formatted traceback, failing executor id, and free-form extra context, delivered as events *before* the exception re-raises (`_workflows/_events.py:80-99`, `_workflows/_workflow.py:601-610`); (c) checkpoint and restore failures name the specific executor or checkpoint id (`_workflows/_runner.py:304-429`). Tool-loop warnings include the exception repr even when the model-facing text stays sanitized (`_tools.py:1420-1425`).

**4. Can operators detect failure patterns?**
Yes, via OpenTelemetry: every chat/workflow/MCP span carries `error.type` (semconv key defined at `observability.py:232`), recorded exception events, and `StatusCode.ERROR` status (`observability.py:2788-2791`, `2361-2374`); workflow runs add a `workflow.error` span event with `error.message`/`error.type` attributes (`_workflows/_workflow.py:615-622`). Pattern detection on *payloads* (e.g., which prompts correlate with failures) requires opting into sensitive capture via `ENABLE_SENSITIVE_DATA` / `enable_sensitive_telemetry()`, which is sticky-disabled-safe (`observability.py:1207-1240`). Metric histograms exist for token usage and duration alongside error capture (`observability.py:1733-1739`). What was not evidenced: prebuilt dashboards or alert rules shipped in-repo (none found in the searched directories).

## Architectural Decisions

1. **Sanitize-by-default, expose-by-opt-in toward the model.** The default model-facing error string is a fixed constant, with detail unlocked per client instance through configuration rather than per-call improvisation (`_tools.py:1399`, `1426-1428`). This prevents prompt-injection-via-stack-trace while keeping the escape hatch one flag away.
2. **Failures as typed events, not just throws.** Both runtimes convert in-flight executor failures into event objects (`WorkflowErrorDetails`-carrying events in Python; `ExecutorFailedEvent`/`WorkflowErrorEvent`/`SubworkflowErrorEvent` in C#) so observers can watch a run degrade without catching exceptions (`_workflows/_events.py:104-130`; `dotnet/src/Microsoft.Agents.AI.Workflows/ExecutorFailedEvent.cs:12`).
3. **Ordering guarantee on failure:** drain pending diagnostics → structured failed event → FAILED status → telemetry → re-raise (`_workflows/_workflow.py:601-623`). Observers never miss context because the throw happened first.
4. **Telemetry detail decoupled from failure detection.** Structural failure signals (`error.type`, status codes, span events) are always emitted; content-bearing attributes require the sensitive-data opt-in (`observability.py:920-934`, `1741-1762`). This lets operators monitor without ingesting PII.
5. **Boundary-scoped visibility.** The `as_agent()` facade deliberately drops lifecycle/failure diagnostics from forwarding (`_events.py:133-143`) — a decision that protects the agent abstraction but trades away failure richness for facade consumers.
6. **Control-plane exceptions separated from error-plane.** `UserInputRequiredException` exists specifically so HITL requests are not misreported as tool errors (`exceptions.py:184-211`).

## Notable Patterns

- **Dual-channel error results:** the model-visible `result` string is sanitized while the machine-readable `exception` field retains the full string on the same content object (`_tools.py:1429-1434`) — downstream UI/log code can choose its fidelity level without re-running anything.
- **Self-logging exceptions:** constructing an exception emits the developer log automatically, so merely creating (not throwing) an error leaves a trace (`exceptions.py:33-34`).
- **Circuit breaker with observability:** the consecutive-error limiter logs at WARNING with the configured maximum, making runaway-model loops visible in standard logs (`_tools.py:2727-2731`).
- **Semconv alignment:** attribute keys follow OpenTelemetry GenAI conventions including version-gated experimental attributes (`observability.py:232`, `903-908` region defining `use_latest_experimental_gen_ai_semconv` semantics).
- **Mirrored policy across languages:** .NET reproduces the same IncludeDetailedErrors tradeoff for skills, citing the shared `FunctionInvokingChatClient` rationale (`dotnet/src/Microsoft.Agents.AI/Skills/AgentSkillsProviderOptions.cs:23-33`).

## Tradeoffs

- **Sanitization vs. model self-correction:** with `include_detailed_errors=False`, a model cannot read the actual exception and may retry blindly; the framework mitigates with the `exception=` metadata field, but that field's usefulness depends on the provider protocol honoring it (`_tools.py:1429-1434`).
- **Facade simplicity vs. failure richness:** filtering `failed`/`executor_failed` events out of `as_agent()` keeps the agent API clean but means orchestrators built on the facade cannot observe partial failure structure without dropping to the workflow API (`_events.py:133-143`).
- **Sensitive-telemetry gating vs. debuggability in production:** operators who need prompt-level failure forensics must flip a global process-wide flag with an explicit warning that it should only be used in dev/test (`observability.py:1207-1219`); there is no per-span middle ground visible in the searched code.
- **Event-conversion vs. fail-fast in .NET:** wrapping any exception in `WorkflowErrorEvent` keeps streams alive (`InProcessRunner.cs:230-234`), but callers who ignore events could mistake a dead run for a healthy idle one unless they also check terminal status.

## Failure Modes / Edge Cases

- **DevUI streaming silent failure:** clients of the streaming execution path receive no error event today; only server logs show the exception (`python/packages/devui/agent_framework_devui/_executor.py:255-256`).
- **Traceback formatting can silently degrade:** `WorkflowErrorDetails.from_exception` swallows formatter failures and stores `traceback=None`, so the most severe crashes (broken `__repr__`/traceback machinery) yield the least detail (`_events.py:88-92`) — though `error_type`/`message` survive.
- **Consecutive-error limit counts per request:** a model alternating success/failure indefinitely evades the breaker since only consecutive errors count (`_tools.py:2718-2731`); total volume is bounded separately by `max_function_calls` if configured (`_tools.py:1342-1346`).
- **Best-effort parallel batch overshoot:** documented behavior allows exceeding call budgets within one parallel batch, meaning error bursts can be larger than the limit implies (`_tools.py:1348-1351`).
- **Sticky disable confusion:** calling `enable_sensitive_telemetry()` after `disable_instrumentation()` silently no-ops (logged info only) unless `force=True` (`observability.py:1228-1236`) — an operator debugging "why am I not seeing payloads" must find that log line.

## Future Considerations

- Complete the DevUI streaming error-event path flagged by the inline comment (`_executor.py:256`) so all user-facing surfaces share the typed-error contract.
- Expose an opt-in forwarding mode for `failed`/`executor_failed` events across the `as_agent()` boundary for hosts that want facade-level failure telemetry (`_events.py:133-143`).
- Document or ship operator dashboards/alert definitions keyed on the emitted semconv attributes (`error.type`, `workflow.error`) so the strong trace-level story translates into monitoring defaults.
- Consider a per-run (not per-client-instance) granularity for `include_detailed_errors` so multi-tenant hosts can vary exposure by request.

## Questions / Gaps

- **Hosting/HTTP layer mapping (both languages):** how agent exceptions translate into HTTP status codes/response bodies for remote users was not traced in this pass. Searched `python/packages/hosting-a2a/` and `dotnet/src/Microsoft.Agents.AI.Hosting*/` directory listings but did not locate a central exception handler; no evidence found either way within the search boundary.
- **Monitoring dashboards:** no in-repo dashboard JSON/Grafana definitions referencing `error.type` or `workflow.error` were found under the inspected source trees; evidence limited to the emission side (`observability.py:232`, `_workflow.py:615-622`).
- **.NET test parity for error visibility:** Python tests pinning `include_detailed_errors` were verified (`test_function_invocation_logic.py:2346-2537`); an equivalent .NET test sweep was not performed in this pass, so .NET-side behavioral guarantees rest on source reading only.
- **Log-format standardization for operators:** Python uses the `agent_framework` logger namespace (`exceptions.py:12`, `observability.py:117`), but whether .NET uses matching category names for cross-runtime correlation was not verified.

---

Generated by `13.03-failure-visibility` against `agent-framework`.

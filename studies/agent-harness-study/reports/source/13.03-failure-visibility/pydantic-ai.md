# Source Analysis: pydantic-ai

## Dimension 13.03: Failure Visibility

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (pydantic, pydantic-graph, OpenTelemetry, httpx/httpx2, tenacity) |
| Analyzed | 2026-08-24 |

> Citation convention: all `file:line` paths below are relative to the source root
> `studies/agent-harness-study/sources/pydantic-ai/` (e.g. `pydantic_ai_slim/pydantic_ai/exceptions.py:57`
> resolves to `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/exceptions.py:57`).

## Summary

Pydantic AI treats failure visibility as a first-class, four-audience design problem with distinct
channels per stakeholder:

1. **Model**: failures are converted into typed message-history parts — `RetryPromptPart` for
   retryable errors (`pydantic_ai_slim/pydantic_ai/messages.py:1637`) and
   `ToolReturnPart(outcome='failed')` for terminal ones (`pydantic_ai_slim/pydantic_ai/messages.py:1335`),
   rendered provider-appropriately (native error channels where available, `{"error": ...}` JSON framing otherwise,
   `pydantic_ai_slim/pydantic_ai/messages.py:1450-1469`). The framework explicitly distinguishes
   "the model should retry" (`ModelRetry`, `pydantic_ai_slim/pydantic_ai/exceptions.py:57`) from
   "the model should see and adapt" (`ToolFailed`, `pydantic_ai_slim/pydantic_ai/exceptions.py:100`).
2. **User (application developer calling the library)**: a typed exception hierarchy
   (`AgentRunError` family, `pydantic_ai_slim/pydantic_ai/exceptions.py:251`) with rich payloads — HTTP status/body/headers on
   `ModelHTTPError` (`exceptions.py:525-609`), full message history on `RunCancelled`
   (`exceptions.py:268-446`), actionable hints appended to `UsageLimitExceeded` (`exceptions.py:462-471`),
   and `capture_run_messages()` for post-mortem inspection of any failed run
   (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2522-2563`).
3. **End user (chat UI)**: UI adapters translate failures into protocol-level error chunks
   (`tool-input-error`, `tool-output-error`, `error` in the Vercel AI adapter,
   `pydantic_ai_slim/pydantic_ai/ui/vercel_ai/_event_stream.py:177-190`,
   `response_types.py:82-156`).
4. **Operator**: an OpenTelemetry instrumentation capability records spans with
   `StatusCode.ERROR`, escaped exception events, a `pydantic_ai.tool.failure_stage` attribute to
   distinguish validation vs execution failures (`pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:325-381`),
   and configurable content redaction (`include_content=False`,
   `pydantic_ai_slim/pydantic_ai/models/instrumented.py:77`).

Detail levels are configurable at every layer: instrumentation format versions 2–6
(`models/instrumented.py:79`), content/binary redaction toggles, per-tool/per-toolset/run retry
budgets that determine *when* a failure escalates from model-visible retry to run-aborting exception,
and MCP `tool_error_behavior` mapping server errors to `'retry'`/`'failed'`/`'error'`.

## Rating

**9 / 10**

Rationale: This is the rare harness where each stakeholder has a purpose-built channel with tests
proving the exact wire bytes (e.g., `tests/test_tool_failed_wire.py:291-316` asserts Anthropic's native
`is_error` flag per outcome; `tests/test_messages.py:2105-2180` pins retry-prompt formatting). Failure
semantics are explicit interfaces (`ModelRetry` vs `ToolFailed` vs raw exception propagation), not
conventions; escalation is budgeted and documented; telemetry distinguishes deferrals from errors
(instrumentation v5) and validation from execution failures. It loses the last point because: there is
no built-in structured-log channel or shipped monitoring dashboard (OTel-only, appropriate for a
library but leaves operators to assemble dashboards), the `error.type` span attribute is an open TODO
(`pydantic_ai_slim/pydantic_ai/_instrumentation.py:464-465`), the history-mutation warning is best-effort
and skipped on errored runs (`pydantic_ai_slim/pydantic_ai/exceptions.py:681-684`), and one UI protocol
(AG-UI) cannot carry failed outcomes across reloads (`docs/ui/ag-ui.md:468-470`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Model-visible retry signal | `ModelRetry(message)` docstring: "send a retry prompt back to the model asking it to try again"; serializable via pydantic core schema | `pydantic_ai_slim/pydantic_ai/exceptions.py:57-97` |
| Model-visible terminal failure | `ToolFailed(message)`: "produces a failed tool result the model sees... does not consume the tool's retry budget"; bound via `UsageLimits` | `pydantic_ai_slim/pydantic_ai/exceptions.py:100-147` |
| Retry prompt part schema | `RetryPromptPart.content` holds either `str` (ModelRetry) or `list[ErrorDetails]` (ValidationError); documented causes list | `pydantic_ai_slim/pydantic_ai/messages.py:1637-1674` |
| Canonical model-facing rendering | `RetryPromptPart.from_error()` — "This is the exact message the model receives when the error is handled by the agent loop, so anything else presenting the failure (e.g. instrumentation spans) must build it the same way" | `pydantic_ai_slim/pydantic_ai/messages.py:1676-1697` |
| Retry text formatting | `model_response()`: appends `'Fix the errors and try again.'`; strips top-level `input` duplication for output retries, keeps it for tool-call retries | `pydantic_ai_slim/pydantic_ai/messages.py:1699-1721` |
| Error-wrapping for providers without native error channel | `model_response_str(wrap_if_error=True)` frames failed returns as `{"error": ...}` | `pydantic_ai_slim/pydantic_ai/messages.py:1450-1469` |
| Native provider error channel | Anthropic tool result sets `is_error=request_part.outcome == 'failed'`; Google uses its prescribed `{'error': ...}` response key | `pydantic_ai_slim/pydantic_ai/models/anthropic.py:1861`; `pydantic_ai_slim/pydantic_ai/models/google.py:1198-1206` |
| Outcome taxonomy on returns | `outcome: Literal['success', 'failed', 'denied', 'interrupted']` on every `ToolReturnPart` | `pydantic_ai_slim/pydantic_ai/messages.py:1335` |
| Tool-retry budget enforcement | `_check_max_retries` raises `UnexpectedModelBehavior('Tool {name!r} exceeded max retries count of {N}. Consider raising the retry limit, or see the docs ...')` chained `from error` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:256-265` |
| Validation-failure → retry conversion | `_wrap_error_as_retry` builds `RetryPromptPart.from_error(...)` → `ToolRetryError`; `_wrap_error_as_failed` builds `outcome='failed'` return | `pydantic_ai_slim/pydantic_ai/tool_manager.py:267-282` |
| Deferred-result failure dispatch | Handler-supplied `ToolFailed`/`ModelRetry` results become failed return / retry prompt respectively | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:676-694`; `pydantic_ai_slim/pydantic_ai/tool_manager.py:1216-1243` |
| Sub-agent cancellation isolation | `cancelled_sub_agent_return`: caller's model sees `ToolReturnPart(outcome='failed')` reading `'The sub-agent run was cancelled: ...'` instead of the whole run tearing down | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:41-62,701-702` |
| Output-retry budget exhaustion message | `UnexpectedModelBehavior(f'Exceeded maximum output retries ({max_retries})')` with original error as `__cause__` | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:496-503,517-521` |
| Empty-response retry prompt | Model told `'Please {" or ".join(alternatives)}.'` when response has no usable output | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2055-2058` |
| Validator `ModelRetry` → retry node | `_build_retry_node` creates `ModelRequest(parts=[RetryPromptPart(content=error.message)])` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1797-1810` |
| Interrupted-tool synthesis | Dangling tool calls closed out with `outcome='interrupted'` returns + metadata marker so resumed histories are valid | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2866-2898` |
| Developer post-mortem API | `capture_run_messages()` context manager; partial `ModelResponse`/`ModelRequest` captured with `state='interrupted'` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2522-2563` |
| Cancellation carries history | `RunCancelled.all_messages()/new_messages()` return resumable snapshots; `from_cancellation()` traverses `__cause__`/`__context__` chains | `pydantic_ai_slim/pydantic_ai/exceptions.py:321-362,364-403` |
| Provider error detail preservation | `ModelHTTPError` keeps `status_code`, `body`, lowercased `headers`, parsed `retry_after`, and even `suggested_model_id` ("Did you mean ...?") | `pydantic_ai_slim/pydantic_ai/exceptions.py:525-609` |
| Unexpected-behavior body retention | `UnexpectedModelBehavior.__str__` appends pretty-printed response body for diagnosis | `pydantic_ai_slim/pydantic_ai/exceptions.py:486-504` |
| Usage-limit messages | `UsageLimitExceeded` auto-appends a hint URL; checks name exact limit and current usage | `pydantic_ai_slim/pydantic_ai/exceptions.py:459-471`; `pydantic_ai_slim/pydantic_ai/usage.py:496-570` |
| All-models-failed visibility | `FallbackExceptionGroup('All models from FallbackModel failed', all_errors)` aggregates every inner failure plus `ResponseRejected` count | `pydantic_ai_slim/pydantic_ai/models/fallback.py:545-554` |
| Run-error capability hook | `on_run_error(ctx, *, error)` lets developers observe/clean up on failure; `wrap_run` can recover | `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:537`; `docs/capabilities/custom.md:766-790` |
| Stale-telemetry warning | `MessageHistoryMutatedWarning` warns when in-place history mutation makes recorded span attributes diverge from what was sent; explicitly best-effort, skipped on errored runs | `pydantic_ai_slim/pydantic_ai/exceptions.py:668-685`; emitted at `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:224-234` |
| OTel error recording | Tool spans `record_exception(e, escaped=True)` + `StatusCode.ERROR` for all failures; retry prompts also recorded as `tool_result_attr` so traces show what the model saw | `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:472-489` |
| Validation-failure spans | Dedicated error span with `logfire.msg='invalid tool call: ...'` and `pydantic_ai.tool.failure_stage='validation'`; without content capture only `exception.type` event (no args leak) | `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:325-381` |
| Deferral ≠ error | Instrumentation v5: `CallDeferred`/`ApprovalRequired` record deferral-name/metadata attributes but leave span UNSET ("deferrals are control flow, not errors") | `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:452-471`; `docs/logfire.md:340-344` |
| Run-span failure state | On error, run span's `finally` still records last-seen messages + `pydantic_ai.all_messages`, so the trace shows how far the run got | `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:206-234` |
| Content redaction knobs | `InstrumentationSettings(include_content, include_binary_content, version)`; versions 2–6 validated, 2–4 deprecated | `pydantic_ai_slim/pydantic_ai/models/instrumented.py:63-164` |
| User-facing chat errors | Vercel adapter emits `ErrorChunk(error_text=str(error))`, `ToolInputErrorChunk`, `ToolOutputErrorChunk` carrying the same text the model receives | `pydantic_ai_slim/pydantic_ai/ui/vercel_ai/response_types.py:82-156`; `pydantic_ai_slim/pydantic_ai/ui/vercel_ai/_event_stream.py:177-190,377-458` |
| MCP server-error surfacing choice | `MCPToolset(tool_error_behavior=...)`: `'retry'`→`ModelRetry`, `'failed'`→`ToolFailed`, `'error'`→raw `fastmcp.ToolError` | `pydantic_ai_slim/pydantic_ai/.agents/skills/building-pydantic-ai-agents/references/TOOLS-CORE.md:94` |
| HTTP transport retries below the agent | tenacity transports + `wait_retry_after` re-send failed HTTP requests invisibly to the agent (documented layering) | `pydantic_ai_slim/pydantic_ai/retries.py:140-239,514-588`; `docs/retries.md:19` |
| Docs: debugging model errors | Worked example printing error, `e.__cause__`, and captured messages | `docs/agent.md:1531-1600` |
| Tests: wire-format proof | Parametrized test asserting only `outcome='failed'` sets Anthropic `is_error`; OpenAI/Responses framed-error snapshots | `tests/test_tool_failed_wire.py:209-333` |
| Tests: retry-prompt rendering | Tests pinning top-level input stripping, nested-input retention, and rendered text | `tests/test_messages.py:2105-2245` |
| Tests: max-retry escalation | Dozens of assertions on exact escalation messages across budgets (per-tool/toolset/output) | `tests/test_agent.py:1042,2623,13393-13743`; `tests/test_tools.py:1542,2643` |

## Answers to Dimension Questions

### 1. Is the model informed of failures?

Yes, systematically. Every failure class has a defined model-visible representation:

- Tool argument validation failures become a `RetryPromptPart` whose content is the Pydantic
  `ErrorDetails` list, rendered by `RetryPromptPart.model_response()` into numbered validation errors +
  `'Fix the errors and try again.'` (`pydantic_ai_slim/pydantic_ai/messages.py:1699-1721`). Rendering
  deliberately tunes information density: for output retries the model's own generated JSON is already
  in context, so top-level errors drop the redundant `input` field; tool-call retries keep nested inputs
  so the model can locate the bad argument (`messages.py:1707-1715`; pinned by
  `tests/test_messages.py:2105-2180`).
- Tools request retries with `ModelRetry(message)` — the string goes verbatim to the model
  (`pydantic_ai_slim/pydantic_ai/exceptions.py:71-73`; wired at `_tool_execution.py:684-690`).
- Terminal failures use `ToolFailed`, producing a `ToolReturnPart(outcome='failed')`. Providers with a
  native error channel receive it natively (Anthropic `is_error=True`, `anthropic.py:1861`; Gemini
  `{'error': ...}` key, `google.py:1198-1206`); others get JSON-framed `{"error": ...}`
  (`messages.py:1467-1468`). A dedicated wire-format test suite proves this per provider
  (`tests/test_tool_failed_wire.py`), including the anti-footgun case that legitimate `{"error": ...}`
  tool output is never double-framed (`test_tool_failed_wire.py:269-283`).
- Even non-failure oddities are surfaced as guidance: a response with no actionable parts yields
  `'Please return text or include your response in a tool call.'` style retry prompts built from
  computed alternatives (`_agent_graph.py:2026-2058`).

### 2. Is the user informed appropriately?

Yes, with layered fidelity. Application developers get typed exceptions carrying exactly the data
needed to act: `ModelHTTPError` preserves status, body, normalized headers, parsed `retry-after`, and
even suggests corrected model ids (`exceptions.py:525-609`); `UsageLimitExceeded` embeds a remediation
hint and doc link (`exceptions.py:462-471`); max-retry escalation names the tool, the count, and links
the docs (`tool_manager.py:262-265`), always chained `from` the underlying error for root-cause access.
For end users of chat UIs, adapters map internal parts to protocol error states — Vercel
`tool-input-error`/`tool-output-error` chunks reuse `RetryPromptPart.model_response()` so the human sees
the same explanation the model got (`ui/vercel_ai/_event_stream.py:437-458`); mid-stream aborts flush an
`ErrorChunk` with finish reason `'error'` (`_event_stream.py:177-190`). Cancellation is treated as a
first-class outcome rather than an opaque error: `RunCancelled` exposes the partial history for resume,
and external asyncio cancellation keeps its standard semantics while attaching recoverable state
(`exceptions.py:268-362`).

### 3. Can developers debug failures?

Yes, unusually well for a Python library:

- `capture_run_messages()` yields the full request/response exchange of a failed run, including partial
  messages tagged `state='interrupted'` after mid-stream exceptions (`_agent_graph.py:2551-2554`); the
  docs demonstrate the canonical print-error/cause/messages debug loop (`docs/agent.md:1539-1564`).
- Exception chaining is preserved everywhere (`raise ... from error` at `tool_manager.py:265`;
  `wrapped.__cause__ = e.__cause__ or e` at `_tool_execution.py:502`), and `FallbackExceptionGroup`
  aggregates all fallback attempts so no failure is swallowed (`fallback.py:551-554`).
- Capability hooks give programmatic observability at every lifecycle point:
  `on_run_error`, `on_node_run_error`, `on_model_request_error`, `on_tool_validate_error`,
  `on_tool_execute_error`, `on_output_validate_error`, `on_output_process_error`
  (`docs/capabilities/custom.md:784-790`; base declarations at
  `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:537,905`).
- A subtle correctness hazard is made loud: if history was mutated in place such that recorded span
  attributes may not match what the model actually received, the run ends with
  `MessageHistoryMutatedWarning` (`instrumentation.py:224-234`), and the warning's limits are honestly
  documented (`exceptions.py:681-684`).

### 4. Can operators detect failure patterns?

Yes, via standards-based telemetry rather than bespoke logs. The `Instrumentation` capability opens
run/model/tool spans; all failure paths set `StatusCode.ERROR` and record escaped exception events
(`capabilities/instrumentation.py:444-489`). Queryability is designed in: a
`pydantic_ai.tool.failure_stage` attribute separates validation from execution failures
(`instrumentation.py:354`), and instrumentation v6 moves tool results (including retries answering a
tool call) onto the GenAI-conventional `role='tool'` message so backends can filter them
(`docs/logfire.md:346-353`). Metrics (tokens, cost, time-to-first-chunk) are recorded outside the span
to avoid double-counting (`_instrumentation.py:493-500,520-522`). Operators can detect patterns like
"this tool fails at validation" or "runs stall before first chunk" with plain OTel queries. Gaps: there
are no shipped dashboards/alert definitions (reasonable for a library), and `error.type` on request
spans is an acknowledged TODO (`_instrumentation.py:464-465`).

### 5. Are failure detail levels configurable?

Extensively:

- Telemetry detail: `include_content` (drop prompts/completions/args/results while keeping structure),
  `include_binary_content`, tracer/meter provider injection, and six versioned attribute schemas
  (`models/instrumented.py:76-164`). With content capture off, validation-failure spans record only the
  exception type — deliberately omitting rejected arguments (`instrumentation.py:365-379`).
- Escalation thresholds: retry budgets are configurable per tool (`tools.py:378,410-423`), per toolset
  (`toolsets/function.py:589-614`), per run (`retries={'tools': N}`), and separately for outputs
  (`output.py:117-148`) — these decide whether a failure stays model-visible or aborts the run.
- Error-mapping policy: MCP servers choose `retry`/`failed`/`raw-error` surfacing
  (`TOOLS-CORE.md:94`); capabilities can intercept any failure via `on_*_error` hooks and convert,
  replace, or suppress it (`docs/capabilities/custom.md:766-790`).
- Transport-layer retries (tenacity) are opt-in and sit below the agent loop, so operators choose
  which failures the model never sees (`docs/retries.md:19`; `retries.py:140-239`).

## Architectural Decisions

1. **Failures are messages, not just exceptions.** The core decision: anything the model should react
   to is encoded as a message-history part (`RetryPromptPart`, failed `ToolReturnPart`) so the failure
   survives serialization, replay, UI transmission, and multi-provider round-trips
   (`messages.py:1637-1674`, `1335`). Exceptions like `ToolRetryError`/`ToolFailedError` exist purely as
   in-process signals carrying those parts (`exceptions.py:616-661`).

2. **One canonical rendering, shared by all surfaces.** `RetryPromptPart.from_error` is documented as
   "the exact message the model receives", and instrumentation is required to build identical text
   (`messages.py:1684-1687`; honored at `capabilities/instrumentation.py:362-363`). This prevents the
   classic drift where the operator's view of a failure differs from the model's.

3. **Retryable vs terminal is an explicit author choice.** `ModelRetry` consumes budget;
   `ToolFailed` doesn't and tells the model to adapt instead (`exceptions.py:57-147`); everything else
   propagates and aborts unless an error hook recovers (`docs/retries.md:131`). The taxonomy avoids
   overloading one mechanism.

4. **Provider-native error channels where they exist, JSON framing elsewhere.** Centralized in
   `wrap_if_error` parameters so each adapter declares what its wire supports
   (`messages.py:1419-1423`), proven by parametrized wire tests (`tests/test_tool_failed_wire.py:291-316`).

5. **Telemetry follows GenAI semantic conventions with versioned escape hatches.** Format versions
   2–6 let the project evolve attribute shapes (deferral semantics in v5, tool-role messages in v6)
   without breaking existing dashboards (`docs/logfire.md:320-353`).

6. **Cancellation is stateful, not just an error.** `RunCancelled` bundles the resumable history,
   usage, and ids so callers can recover work (`exceptions.py:281-312`), and dangling tool calls are
   repaired with synthesized `outcome='interrupted'` returns marked in metadata
   (`_agent_graph.py:2866-2898`).

## Notable Patterns

- **Failure-stage tagging for queryability**: `pydantic_ai.tool.failure_stage='validation'` on spans,
  keeping the `execute_tool` operation name so backends group it with sibling tool spans
  (`capabilities/instrumentation.py:349-354`).
- **Control-flow-vs-error distinction in telemetry**: `CallDeferred`/`ApprovalRequired` annotate the
  span (`tool_deferral_name_attr`, metadata) but leave status UNSET under v5+, so dashboards don't
  count approvals as incidents (`capabilities/instrumentation.py:452-471`).
- **Information-density tuning in model-facing errors**: dropping duplicated `input` for output
  retries while retaining it for tool calls (`messages.py:1707-1715`) shows deliberate token-economy
  thinking about what the model needs to see.
- **Isolation of sub-agent failures**: a cancelled sub-agent becomes a failed tool return visible to
  the parent's model, converting an infrastructure event into domain feedback
  (`_tool_execution.py:41-62`).
- **Status stubs keep parallel strategies coherent**: under `end_strategy` variants, losing/skipped
  output tools get deterministic status strings ('Final result processed.',
  'Output not used - addressing tool retries from this round first.', etc.) centralized as constants
  (`_tool_execution.py:32-38`), so the model always sees why its output wasn't used.
- **Escaped-exception events preserve stack traces** while `record_exception=False,
  set_status_on_exception=False` gives the framework full control of span semantics
  (`capabilities/instrumentation.py:444-449`).

## Tradeoffs

- **Richness vs leakage risk.** Model-facing validation errors include input values and error details
  (`messages.py:1716`); this maximizes repairability but echoes user data back through provider
  traffic. Mitigation exists only on the telemetry side (`include_content=False`), not for the
  model-bound payload itself.
- **OTel-only observability.** There is no stdlib-logging or structured-log integration; environments
  without an OTel backend have essentially no operator channel beyond exceptions. The TODO on
  `error.type` attributes confirms the monitoring story is still maturing
  (`_instrumentation.py:464-465`).
- **Best-effort consistency warnings.** The stale-span warning is skipped on errored runs (to avoid
  displacing the real exception) and its absence guarantees nothing (`exceptions.py:681-684`) — a
  reasonable ordering choice, but it means the most fragile runs are exactly the unaudited ones.
- **Protocol ceilings inherited by UIs.** AG-UI has no outcome/error field, so failed outcomes survive
  reload only via a namespaced encrypted-value workaround, and older streams reconstruct as success
  (`docs/ui/ag-ui.md:468-470`) — the framework compensates for, but cannot fix, downstream protocol
  limits.
- **Invisible transport retries.** Tenacity-level HTTP retries intentionally bypass agent awareness
  (`docs/retries.md:19`); good for latency, but operators must remember failures counted at the agent
  layer differ from network-level attempt counts.

## Failure Modes / Edge Cases

- **Retry-budget exhaustion**: per-tool counters raise `UnexpectedModelBehavior` naming tool and limit
  with a docs link (`tool_manager.py:256-265`); negative budgets raise immediately due to `>=`
  comparison (`tool_manager.py:259-260`), tested at `tests/test_agent.py:13660`.
- **Duplicate `tool_call_id`s on resume**: fail-closed with `UserError` listing the duplicates rather
  than silently mis-binding supplied results (`_tool_execution.py:405-420`).
- **Absorbed output failures**: when another output wins, a sibling's max-retries failure is demoted to
  a status stub ('Output tool not used - output failed validation.') and only raised if nothing won
  (`_tool_execution.py:573-578,1224+`).
- **Mid-stream errors during parallel tool batches**: sibling tasks are cancelled and drained so no
  orphaned tasks leak, and partially completed tool returns still surface into history
  (`_tool_execution.py:828-847`).
- **Content-filter nuance**: partial/refusal text alongside `finish_reason='content_filter'` is ordinary
  output by default; the opt-in capability extends erroring to every filtered response and serializes
  the full response into `body` for inspectability (`docs/capabilities/raise-content-filter-error.md:32`).
- **Serialization safety of failures themselves**: `ModelRetry`, `ToolFailed`, `RunCancelled`,
  `UnexpectedModelBehavior` all implement `__reduce__`/pydantic schemas so failures survive durable-exec
  boundaries (`exceptions.py:81-97,127-147,315-316`).
- **Unparseable Retry-After headers** fall back to exponential backoff rather than failing the wait
  strategy (`retries.py:562-588`).

## Future Considerations

- Add `error.type` attributes on model-request spans (self-identified TODO,
  `_instrumentation.py:464-465`) to enable failure-type aggregation without content capture.
- Consider a redaction story for model-bound validation details (today only telemetry is redactable),
  e.g., truncating `input_value` above a size threshold.
- Ship or document reference OTel dashboard/alert definitions for the failure-stage and deferral
  attributes the spans already emit.
- Extend the AG-UI outcome-preservation mechanism documentation as the upstream protocol gains native
  error/outcome fields.

## Questions / Gaps

- **No log-format evidence found.** Searches for logging integration (`grep` for `logging.`/
  logger usage in `pydantic_ai_slim/pydantic_ai/`) surface only warnings and OTel; the "Log formats"
  evidence item in the dimension is answered by "spans only". Boundary searched: the slim package and
  `docs/logfire.md`.
- **Monitoring dashboards**: none shipped in-repo (expected for a library); the closest artifacts are
  the `logfire.json_schema` hints embedded in span attributes to render tables in Logfire
  (`capabilities/instrumentation.py:268-277`, `399-416`).
- Whether `on_run_error` recovery interacts cleanly with streamed runs that have already committed
  partial output is documented only at the hook level (`docs/capabilities/custom.md:460`); no test was
  located covering recovery-after-stream-commit specifically. Searched `tests/test_capabilities.py`
  for stream-commit + `on_run_error` combinations without a direct match.

---

Generated by `13.03-failure-visibility` against `pydantic-ai`.

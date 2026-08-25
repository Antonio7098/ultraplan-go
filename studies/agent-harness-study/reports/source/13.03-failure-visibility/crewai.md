# Source Analysis: crewai

## 13.03 Failure Visibility

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (monorepo: `lib/crewai`, `lib/crewai-core`, `lib/crewai-tools`), Pydantic, OpenTelemetry, Rich-based console output |
| Analyzed | 2026-08-23 |

## Summary

CrewAI implements failure visibility as a first-class, layered subsystem rather than ad-hoc string handling. The core design is a declarative failure model (`ToolFailure` / `ToolFailureRecord`) in `lib/crewai/src/crewai/tools/tool_failure.py` that separates "what the model sees" (a plain-text message) from "what the framework knows" (a structured record with reason, machine code, retryability, and call context). Every stakeholder tier is addressed:

1. **Model**: failures are fed back as text messages so the LLM can recover — tool exceptions become `"Error executing tool: {e}"` tool-role messages on the native path (`lib/crewai/src/crewai/agents/crew_agent_executor.py:996`) or i18n error strings on the ReAct path (`lib/crewai/src/crewai/tools/tool_usage.py:151-167`); parse failures are appended to the conversation (`lib/crewai/src/crewai/utilities/agent_utils.py:763-770`); max-iteration and guardrail failures inject explicit corrective instructions from `lib/crewai/src/crewai/translations/en.json`.
2. **User**: red console panels via a central printer/formatter, gated by `verbose` flags (`lib/crewai/src/crewai/utilities/agent_utils.py:733-740`, `lib/crewai/src/crewai/events/utils/console_formatter.py:497-539`), plus exceptions re-raised to the caller at task/crew boundaries.
3. **Developer**: a typed event bus emitting dedicated error events for every layer (`tool_usage_error`, `llm_call_failed`, `task_failed`, `crew_kickoff_failed`, etc.), correlation IDs on every event, structured `tool_failures` lists on `TaskOutput`/`LiteAgentOutput`/`CrewOutput`, and per-handler fault isolation in the bus itself.
4. **Operator**: anonymous OpenTelemetry spans ("Tool Usage Error", "Tool Repeated Usage"), an opt-out trace-collection listener that batches all lifecycle/failure events for remote inspection, documented third-party observability integrations, and env-var telemetry kill switches.

The most distinctive mechanism is the configurable `ToolFailurePolicy` (`ignore` / `warn` / `raise`) resolvable per tool → task → agent → crew with precedence rules, backed by an extensive test suite covering end-to-end aborts, event ordering, and retry interactions.

## Rating

**8/10** — Failure visibility is explicit, typed, tested, and configurable at every stakeholder tier, with strong evidence of maturity under failure (event-before-abort ordering tests, retry-limit interaction tests, concurrent-execution-safe collectors). It misses 9–10 because: (a) the default `Logger` silently drops all log lines when `verbose=False`, including errors (`lib/crewai/src/crewai/utilities/logger.py:25-33`), giving developers no non-stdout sink; (b) events carry `error=str(e)` strings, discarding stack traces; (c) `handle_unknown_error` deliberately suppresses litellm errors from the user entirely (`lib/crewai/src/crewai/utilities/agent_utils.py:730-731`); and (d) operator dashboards depend on external services (CrewAI AMP trace batches, third-party OTel vendors) with no built-in local aggregation.

## Evidence Collected

Every entry includes a file path with line numbers, relative to the source directory.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Model-facing failure taxonomy | `ToolFailureReason` enum: TOOL_REPORTED, EXCEPTION, MCP_ERROR, USAGE_LIMIT, UNKNOWN_TOOL, INVALID_INPUT | lib/crewai/src/crewai/tools/tool_failure.py:35-54 |
| Model sees failure as plain text | `ToolFailure.as_agent_message()` renders message + optional code; docstring says "model behavior is unchanged" | lib/crewai/src/crewai/tools/tool_failure.py:71-108 |
| Declarative detection only | `detect_tool_failure()` accepts only explicit `ToolFailure`; no string-sniffing | lib/crewai/src/crewai/tools/tool_failure.py:154-162 |
| Exception → model text | Native path wraps tool exceptions as `f"Error executing tool: {e}"` returned into the `role:"tool"` message | lib/crewai/src/crewai/agents/crew_agent_executor.py:995-1010; lib/crewai/src/crewai/agents/crew_agent_executor.py:1059-1065 |
| ReAct-path error strings to model | `use()` returns `ToolUsageError.message` / exception text as the observation string | lib/crewai/src/crewai/tools/tool_usage.py:151-167 |
| Unknown-tool feedback to model | "Action '{tool_name}' don't exist, these are the only available Actions: …" raised then returned as text | lib/crewai/src/crewai/tools/tool_usage.py:826-853 |
| Parse-failure loop informs model | `handle_output_parser_exception` appends `e.error` as a user message; agent retries | lib/crewai/src/crewai/utilities/agent_utils.py:743-778 |
| Max-iterations informs model | Appends `force_final_answer` instruction ("You MUST give your absolute best final answer…") before one final LLM call | lib/crewai/src/crewai/utilities/agent_utils.py:376-433; lib/crewai/src/crewai/translations/en.json (`force_final_answer`) |
| Usage-limit feedback to model | "Tool 'X' has reached its usage limit…" returned as result and recorded as `USAGE_LIMIT` failure | lib/crewai/src/crewai/tools/tool_usage.py:581-595; lib/crewai/src/crewai/tools/tool_usage.py:791-808 |
| Guardrail failure fed back to model | `validation_error` template: "Previous attempt failed validation: {error} … Try again" | lib/crewai/src/crewai/translations/en.json (`validation_error`); lib/crewai/src/crewai/utilities/guardrail.py:77-101 |
| Context-window failure UX | Summarize-and-retry vs. `SystemExit` under `respect_context_window=False` | lib/crewai/src/crewai/utilities/agent_utils.py:795-832 |
| User console errors (verbose-gated) | Red PRINTER output on every tool-error branch, only when `self.agent.verbose` | lib/crewai/src/crewai/tools/tool_usage.py:153-157, 474-475, 731-732 |
| Unknown-error user panel | `handle_unknown_error` prints "An unknown error occurred…" but returns early for litellm errors | lib/crewai/src/crewai/utilities/agent_utils.py:715-740 |
| Failure panels replace success panels | `should_render_success_panel` returns False when a failure exists; red failure panel renders reason/code/message | lib/crewai/src/crewai/events/utils/console_formatter.py:497-539 |
| Caller sees hard failures | `kickoff` re-raises after emitting `CrewKickoffFailedEvent(error=str(e))` | lib/crewai/src/crewai/crew.py:1068-1078 |
| Task failure event + re-raise | `TaskFailedEvent(error=str(e))` emitted in sync and async paths before re-raising | lib/crewai/src/crewai/task.py:798-801; lib/crewai/src/crewai/task.py:954-957 |
| Streaming failure signaling | Streaming thread calls `signal_error(ctx.state, exc)` so stream consumers see the failure | lib/crewai/src/crewai/crew.py:1018-1029 |
| Typed failure events per layer | Tool: `ToolUsageErrorEvent`, `ToolFailureDetectedEvent`, `ToolValidateInputErrorEvent`, `ToolSelectionErrorEvent`, `ToolExecutionErrorEvent` | lib/crewai/src/crewai/events/types/tool_usage_events.py:78-132 |
| Finished-event carries failure flag | `ToolUsageFinishedEvent.failure` lets a trace UI mark failed without correlating a second event | lib/crewai/src/crewai/events/types/tool_usage_events.py:63-75 |
| LLM failure events | `LLMCallFailedEvent(error=…)` emitted on streaming/nonstreaming LLM failures | lib/crewai/src/crewai/events/types/llm_events.py:117-121; lib/crewai/src/crewai/llm.py:1117-1126 |
| Agent/task/crew/flow error events | `AgentExecutionErrorEvent`, `LiteAgentExecutionErrorEvent`, `MethodExecutionFailedEvent`, `FlowFailedEvent` | lib/crewai/src/crewai/events/types/agent_events.py:52-92; lib/crewai/src/crewai/events/types/flow_events.py:55-111 |
| Event correlation IDs | Every `BaseEvent` has `event_id`, `parent_event_id`, `previous_event_id`, `triggered_by_event_id`, `emission_sequence` | lib/crewai/src/crewai/events/base_events.py:69-87 |
| Bus isolates listener crashes | Sync handler exceptions collected & printed per handler; async uses `gather(..., return_exceptions=True)` | lib/crewai/src/crewai/events/event_bus.py:401-427; lib/crewai/src/crewai/events/event_bus.py:429-456 |
| Structured failures on outputs | `TaskOutput.tool_failures: list[ToolFailureRecord]` + `has_tool_failures`; same on LiteAgentOutput/CrewOutput | lib/crewai/src/crewai/tasks/task_output.py:50-61; lib/crewai/src/crewai/lite_agent_output.py:54-62; lib/crewai/src/crewai/crews/crew_output.py:45 |
| Record context for debugging | `ToolFailureRecord` carries tool_name, args, agent_role, task_name/id; `summary()` for logs | lib/crewai/src/crewai/tools/tool_failure.py:111-143 |
| Concurrent-safe collection | `tool_failure_collector()` ContextVar isolates records across concurrent tasks sharing an agent | lib/crewai/src/crewai/tools/tool_failure.py:260-285 |
| Policy configurability | `ToolFailurePolicy` IGNORE/WARN/RAISE; resolution precedence tool→task→agent→crew, malformed policy logs warning instead of raising | lib/crewai/src/crewai/tools/tool_failure.py:57-68, 177-208 |
| Policy enforcement point | `handle_tool_failure` records, emits `ToolFailureDetectedEvent`, raises `ToolExecutionFailedError` only under RAISE (event precedes abort) | lib/crewai/src/crewai/tools/tool_failure.py:324-382 |
| MCP retries classify errors | Timeout/auth/not-found/connection/parsing classification drives retry; final error surfaced to model after N attempts | lib/crewai/src/crewai/tools/mcp_tool_wrapper.py:91-154 |
| Operator telemetry spans | "Tool Usage Error" and "Tool Repeated Usage" OTel spans with llm/tool attributes | lib/crewai/src/crewai/telemetry/telemetry.py:611-694 |
| Telemetry never breaks runs | `SafeOTLPSpanExporter.export` catches exceptions, logs, returns FAILURE; init wrapped in try/except setting ready=False | lib/crewai/src/crewai/telemetry/telemetry.py:71-91, 128-158 |
| Telemetry opt-out switches | `OTEL_SDK_DISABLED`, `CREWAI_DISABLE_TELEMETRY`, `CREWAI_DISABLE_TRACKING` env checks | lib/crewai/src/crewai/telemetry/telemetry.py:160-167 |
| Trace listener subscribes to all failures | Handlers for `crew_kickoff_failed`, `task_failed`, `agent_execution_error`, `llm_call_failed`, `tool_usage_error`, `tool_failure_detected`, A2A failures | lib/crewai/src/crewai/events/listeners/tracing/trace_listener.py:321-322, 348-349, 380-381, 405-406, 417-424, 741-745 |
| Remote batch failure marking | Failed/incomplete trace batches marked via Plus API so operators see collection gaps | lib/crewai/src/crewai/events/listeners/tracing/trace_batch_manager.py:240-247, 339-342 |
| Verbose logger drops errors by default | `Logger.log` returns early unless `verbose`; output goes to stdout PRINTER only, no file/stderr sink | lib/crewai/src/crewai/utilities/logger.py:17-33 |
| Retry visibility | `AgentExecutionErrorEvent` emitted each failed attempt inside retry loop; litellm errors pass through un-retried | lib/crewai/src/crewai/agent/core.py:721-747 |
| Tests prove intended behavior | warn records+emits without stopping (:328), raise aborts (:366) with event-before-raise (:376), agent sees failure as plain text (:423), finished-event carries failure (:406), native unknown-tool recording (:550), retry limit does not swallow abort (:689), broad handlers let aborts through (:741) | lib/crewai/tests/tools/test_tool_failure.py:175-741 |
| Docs tie-in | Telemetry opt-out documented; observability vendor integrations (Langfuse, Langtrace, Datadog, Arize Phoenix, …) | docs/edge/en/telemetry.mdx:26; docs/edge/en/observability/overview.mdx |

## Answers to Dimension Questions

**1. Is the model informed of failures?**
Yes, consistently. All three tool paths convert failures into conversation-visible text: the ReAct path returns i18n error strings as observations (`lib/crewai/src/crewai/tools/tool_usage.py:151-167, 717-732`), the native function-calling path writes `"Error executing tool: {e}"` into the `role:"tool"` message (`lib/crewai/src/crewai/agents/crew_agent_executor.py:996, 1059-1065`), and declared `ToolFailure`s render via `as_agent_message()` (`lib/crewai/src/crewai/tools/tool_failure.py:104-108`, verified by test at `lib/crewai/tests/tools/test_tool_failure.py:423`). Parse failures, max iterations, repeated usage, usage limits, and guardrail violations all append explicit recovery instructions to the message list (`lib/crewai/src/crewai/utilities/agent_utils.py:763, 403-410`; `lib/crewai/src/crewai/tools/tool_usage.py:512-514, 581-595`; `lib/crewai/src/crewai/translations/en.json`). One caveat: because the model's input is plain text, a framework error string is indistinguishable from successful output unless tools use `ToolFailure` — which is exactly why the declarative mechanism exists (`lib/crewai/src/crewai/tools/tool_failure.py:1-11`).

**2. Is the user informed appropriately?**
Partially, and gated on `verbose`. Errors print as red panels/messages when `agent.verbose` is set (`lib/crewai/src/crewai/tools/tool_usage.py:153-157`; `lib/crewai/src/crewai/utilities/agent_utils.py:733-740`), and reported tool failures swap success panels for red failure panels showing reason/code/message (`lib/crewai/src/crewai/events/utils/console_formatter.py:497-539`, tested at `lib/crewai/tests/tools/test_tool_failure.py:508-528`). Hard failures always propagate as exceptions to the caller (`lib/crewai/src/crewai/crew.py:1078`; `lib/crewai/src/crewai/task.py:957`). Two gaps: with `verbose=False` the `Logger` drops even error-level lines entirely (`lib/crewai/src/crewai/utilities/logger.py:25-33`), and `handle_unknown_error` intentionally hides litellm provider errors from the user (`lib/crewai/src/crewai/utilities/agent_utils.py:730-731`).

**3. Can developers debug failures?**
Yes — this is the strongest tier. Developers get: typed error events per subsystem with full correlation metadata (event/parent/previous IDs and emission sequence, `lib/crewai/src/crewai/events/base_events.py:82-87`); structured `ToolFailureRecord`s including args, agent role, and task identity attached to `TaskOutput.tool_failures` (`lib/crewai/src/crewai/tasks/task_output.py:50-61`; populated at `lib/crewai/src/crewai/task.py:849-887`); retry-attempt counters in tool events (`run_attempts`, `lib/crewai/src/crewai/tools/tool_usage.py:1060-1087`); and Python logging on selected paths (`logging.error` in `lib/crewai/src/crewai/llm.py:1098`). Weaknesses: events carry `str(e)` not tracebacks (`lib/crewai/src/crewai/task.py:800`; `lib/crewai/src/crewai/events/types/task_events.py:51`), and there is no structured file/JSON log sink — developer debugging beyond the current process relies on the event bus or tracing export.

**4. Can operators detect failure patterns?**
Yes, through two channels. First, anonymous OTel spans explicitly designed for pattern detection — "Tool Usage Error" and "Tool Repeated Usage" ("might indicate an issue") exported to CrewAI's collector, disableable by env vars (`lib/crewai/src/crewai/telemetry/telemetry.py:160-167, 611-694`). Second, the opt-in `TraceCollectionListener` forwards every failure event type (crew/task/agent/LLM/tool/memory/knowledge/A2A) to remotely stored trace batches, and marks batches failed when collection itself fails (`lib/crewai/src/crewai/events/listeners/tracing/trace_listener.py:321-424`; `trace_batch_manager.py:240-247, 339-342`). There are no built-in dashboards or alerting; operators must consume these exports via CrewAI AMP or the documented third-party integrations (`docs/edge/en/observability/overview.mdx`). Note the anonymous telemetry deliberately excludes prompts/task descriptions unless `share_crew` is enabled (`lib/crewai/src/crewai/telemetry/telemetry.py:1-7, 574-578`), limiting operator detail by design.

## Architectural Decisions

1. **Declarative over inferred failure detection.** Only an explicit `ToolFailure` return marks a soft failure; the module header explains Slack `{"ok": false}` used to be recorded as success, and string-sniffing was rejected (`lib/crewai/src/crewai/tools/tool_failure.py:1-11, 154-162`).
2. **Policy object separate from visibility.** `ToolFailurePolicy` decouples "the tool failed" from "what happens next" (record/event/abort), resolved with strict precedence tool→task→agent→crew and a safe fallback on invalid values (`lib/crewai/src/crewai/tools/tool_failure.py:57-68, 177-208`).
3. **Central typed event bus as the debug backbone.** All failure reporting flows through `crewai_event_bus.emit` with Pydantic-typed events; handler exceptions are isolated so observability can never break execution (`lib/crewai/src/crewai/events/event_bus.py:401-456`).
4. **Dual-channel stakeholder separation.** The same failure produces a model-readable string AND a structured record/event — e.g., a failed tool cannot become `result_as_answer` (`lib/crewai/src/crewai/tools/tool_usage.py:666-674`) while its text still reaches the agent.
5. **Fail-open observability.** Telemetry init/export failures degrade to no-op (`lib/crewai/src/crewai/telemetry/telemetry.py:71-91, 152-158`); trace-batch failures are marked, never raised (`lib/crewai/src/crewai/events/listeners/tracing/first_time_trace_handler.py:234-239`).
6. **Concurrent-correctness in accounting.** Per-execution failure collectors via ContextVar prevent shared agents from mixing records across concurrent tasks (`lib/crewai/src/crewai/tools/tool_failure.py:265-285`).

## Notable Patterns

- **Event-before-abort ordering**: `handle_tool_failure` emits `ToolFailureDetectedEvent` *before* raising under RAISE, so subscribers observe the cause even on aborting runs (`lib/crewai/src/crewai/tools/tool_failure.py:359-382`; tested at `lib/crewai/tests/tools/test_tool_failure.py:376`).
- **Paired start/finish events with scope matching**: the bus validates `*_started`/`*_ended` pairing and warns on mismatch/empty pops (`lib/crewai/src/crewai/events/event_bus.py:548-563`; `lib/crewai/src/crewai/events/event_context.py:194-223`).
- **Honest metrics dimensions**: parse failures report an empty tool name to avoid putting raw model output into telemetry dimensions (`lib/crewai/src/crewai/tools/tool_usage.py:925-931`).
- **i18n error templates**: user/model-facing error prose is centralized in `lib/crewai/src/crewai/translations/en.json` and consumed via `I18N_DEFAULT.errors(...)`.
- **Graceful degradation wrappers**: `SafeOTLPSpanExporter` and trace handlers catch-and-log so monitoring outages never surface as run failures (`lib/crewai/src/crewai/telemetry/telemetry.py:71-91`).
- **Retry-aware visibility**: every failed attempt emits its own `AgentExecutionErrorEvent` and closes its own execution-started scope (`lib/crewai/src/crewai/agent/core.py:733-747`), and RAISE-policy aborts are exempt from the retry limit (`lib/crewai/tests/tools/test_tool_failure.py:689`).

## Tradeoffs

- **Text-first model feedback vs. fidelity**: the model gets recoverable prose, but stack traces and structured details stay out of the conversation; richer context lives only in events/records.
- **Verbose-gated console vs. silent-by-default runs**: keeping stdout quiet when `verbose=False` also silences error logs (`lib/crewai/src/crewai/utilities/logger.py:25-33`) — a deliberate simplicity tradeoff that hurts headless deployments.
- **Anonymous telemetry vs. operator depth**: privacy-preserving defaults mean operators get counts/models, not content, unless they opt into `share_crew` (`lib/crewai/src/crewai/telemetry/telemetry.py:574-578`).
- **SaaS-centric tracing**: the built-in trace pipeline targets CrewAI AMP batches; self-hosted operators depend on third-party integrations rather than a neutral local sink.
- **Policy flexibility vs. behavior predictability**: four resolution scopes make behavior powerful but require reading precedence rules (`resolve_tool_failure_policy`, `lib/crewai/src/crewai/tools/tool_failure.py:177-208`); mitigated by an extensive precedence test matrix (`lib/crewai/tests/tools/test_tool_failure.py:175-322`).

## Failure Modes / Edge Cases

- **Silent errors in production mode**: any code path relying solely on `Logger.log("error", …)` emits nothing when `verbose=False` (`lib/crewai/src/crewai/utilities/logger.py:25-33`).
- **Suppressed provider errors**: litellm-originated exceptions produce no user-facing message at all (`lib/crewai/src/crewai/utilities/agent_utils.py:730-731`), only the eventual raise/retry events.
- **Lost tracebacks**: `error=str(e)` on `TaskFailedEvent`/`CrewKickoffFailedEvent`/`LLMCallFailedEvent` discards stack traces (`lib/crewai/src/crewai/task.py:800`; `lib/crewai/src/crewai/crew.py:1072-1076`).
- **Partial streaming responses mask errors**: a mid-stream exception returns whatever accumulated instead of failing (`lib/crewai/src/crewai/llm.py:1098-1115`), visible only via a logged warning.
- **Context-window dead-end**: with `respect_context_window=False`, failure surfaces as `SystemExit` — abrupt for library consumers (`lib/crewai/src/crewai/utilities/agent_utils.py:830-832`).
- **Fuzzy tool selection**: `_select_tool` matches names above a 0.85 similarity ratio (`lib/crewai/src/crewai/tools/tool_usage.py:810-825`), so a near-miss name can execute an unintended tool instead of erroring.
- **IGNORE policy erases signals**: under `ignore`, nothing is recorded, emitted, or flagged — intentional pre-1.16 compatibility that makes soft failures invisible again (`lib/crewai/src/crewai/tools/tool_failure.py:60-61, 304-321`; `lib/crewai/src/crewai/tasks/task_output.py:52-55`).

## Future Considerations

- Add a non-stdout logging sink (file/JSON/stderr) that respects severity independently of `verbose`, so error-level events survive headless runs.
- Preserve exception objects or tracebacks on failure events (e.g., serialized `traceback` field alongside `error: str`).
- Surface litellm/provider errors through a sanitized user panel instead of suppressing them entirely.
- Ship a vendor-neutral local trace/log exporter so operator monitoring does not require CrewAI AMP or a third-party SaaS.
- Extend the declarative `ToolFailure` contract deeper into bundled tools (currently adoption is concentrated in framework paths; MCP integration maps `isError` separately at `lib/crewai/src/crewai/tools/tool_failure.py:44-45`).

## Questions / Gaps

- No evidence found of a built-in metrics dashboard or alerting component inside this repository; operator-facing aggregation appears delegated to external services. Searched `lib/crewai/src/crewai/telemetry/`, `docs/edge/en/observability/`, and grep for "dashboard"/"alert" patterns.
- No evidence found of structured (JSON) application-log formatting anywhere under `lib/crewai/src/crewai/utilities/` — logging is either the verbose-gated `Logger` or scattered stdlib `logging.getLogger(__name__)` usage (e.g., `lib/crewai/src/crewai/events/event_bus.py:62`).
- The exact retention/query capabilities of remotely stored trace batches could not be verified from source alone (client-side code lives in `lib/crewai/src/crewai/events/listeners/tracing/trace_batch_manager.py`; server side is out of repo scope).
- Whether `ToolExecutionErrorEvent` (`lib/crewai/src/crewai/events/types/tool_usage_events.py:113-132`) is still emitted anywhere in current code paths was inconclusive; emitters found were limited to `ToolUsageErrorEvent` variants — possible legacy duplication worth confirming.

---

Generated by `13.03-failure-visibility` against `crewai`.

# Source Analysis: crewai

## 07.08 Tool Failure Escalation

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (pydantic-based monorepo: `lib/crewai` core framework, `lib/crewai-tools` tool library, `lib/cli`) |
| Analyzed | 2026-08-24 |

## Summary

CrewAI treats a tool failure as a first-class, structured object rather than an error string. The centerpiece is `ToolFailure` (`lib/crewai/src/crewai/tools/tool_failure.py:71-108`), a frozen pydantic envelope carrying `message`, a categorical `reason`, an optional machine-readable `code`, a `retryable` hint, and free-form `details`. A tool declares failure by *returning* a `ToolFailure` from `_run`; detection is strictly declarative — the framework never guesses that a string "looks like" an error (`detect_tool_failure`, `lib/crewai/src/crewai/tools/tool_failure.py:154-162`). Six failure causes are enumerated in `ToolFailureReason` (`tool_failure.py:35-54`): tool-reported, exception, MCP `isError`, spent usage limit, unknown tool, and invalid input.

What happens after detection is governed by a four-scope policy chain, `tool_failure_policy` ∈ {ignore, warn, raise}, resolved most-specific-first (tool → task → agent → crew → default `warn`; `resolve_tool_failure_policy`, `tool_failure.py:177-208`). Under the default `warn`, the failure is recorded on a context-isolated collector, emitted as `ToolFailureDetectedEvent` on the event bus, attached to `TaskOutput.tool_failures`, printed as a red console panel under verbosity — and the agent still receives plain prose via `as_agent_message()` so it can recover by trying something else. Under `raise`, the run aborts with `ToolExecutionFailedError`, and a dedicated passthrough tuple prevents any of the framework's broad retry handlers from swallowing that abort (`_passthrough_exceptions = (ToolExecutionFailedError,)`, `lib/crewai/src/crewai/agent/core.py:141`).

Model-facing errors are recovery-oriented: every error is rendered back to the LLM as text (ReAct "Observation" or native `role:"tool"` message) with actionable content — available tool names on a wrong name, accepted inputs on an exception, and a format reminder plus "Moving on then" once in-call retries are exhausted. Separate retry systems exist per layer: parse/execution retries inside `ToolUsage` (3 attempts), guardrail retries at the task layer with the validation error fed back as context, task-level retries bounded by `max_retry_limit` (default 2) at the agent layer, and exponential-backoff transport retries with error classification inside the MCP client. There is no automatic escalation of a tool failure to a human; humans see failures through verbose panels and event subscriptions, and `TaskOutput.has_tool_failures` lets calling code decide whether a nominally successful run actually succeeded. The design's honest weak spots: the `retryable` flag has no automated consumer, telemetry spans carry no failure reason, and several legacy string-error paths (delegation tools, native-path generic messages) bypass the structured envelope.

## Rating

**8 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale:
- **Explicit taxonomy + envelopes**: `ToolFailureReason`/`ToolFailure`/`ToolFailureRecord` (`tool_failure.py:35-143`) give machine-readable grouping by cause, with call context (tool, args, agent role, task id) attached.
- **Configurable reaction**: three policies with documented precedence and end-to-end tests at every scope (`lib/crewai/tests/tools/test_tool_failure.py:175-322`, `366-405`, `460-506`, `706-739`).
- **Proven under failure**: ~90 dedicated tests including adversarial ones — asserting no broad `except Exception` can downgrade a `raise` abort (`test_tool_failure.py:741-764`), that retries don't swallow aborts (`689-704`), thread/concurrency isolation (`1350-1462`), and cache poisoning prevention (`918-930`).
- **Observable**: paired events with matching ids (`test_tool_failure.py:1218-1240`), console panels distinguishing reported failure vs raised error (`console_formatter.py:496-511`), structured output fields (`tasks/task_output.py:50-61`).
- Not 9–10 because: the `retryable` field is produced but never consumed automatically (searched all of `lib/` for `.retryable` consumers — only production sites exist); operator telemetry (`telemetry.py:666-694`) records error occurrences without reason/cause attributes; and two parallel execution stacks (`tool_usage.py` ReAct path vs `agent_utils.py` native path) duplicate the failure-handling logic with divergent message formats, plus a third copy in `experimental/agent_executor.py`.

## Evidence Collected

Every entry cites `path:line` relative to `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai` unless noted.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Failure taxonomy | `ToolFailureReason`: TOOL_REPORTED, EXCEPTION, MCP_ERROR, USAGE_LIMIT, UNKNOWN_TOOL, INVALID_INPUT | `tools/tool_failure.py:35-54` |
| Error envelope | `ToolFailure(message, reason, code, retryable, details)` frozen pydantic model | `tools/tool_failure.py:71-108` |
| Model-facing rendering | `as_agent_message()` appends `(code: X)`; used verbatim by `_format_tool_output_for_agent` | `tools/tool_failure.py:104-108`; `tools/structured_tool.py:59-63` |
| Record envelope | `ToolFailureRecord` adds tool_name, args, agent_role, task_name/id; `summary()` one-liner for logs | `tools/tool_failure.py:111-143` |
| Abort exception | `ToolExecutionFailedError` wraps the record; raised only under RAISE policy | `tools/tool_failure.py:146-151,381-382` |
| Policy enum | IGNORE ("pre-1.16 behavior"), WARN (default), RAISE | `tools/tool_failure.py:57-68` |
| Policy resolution precedence | most specific wins: tool → original_tool → task → agent → crew → WARN; invalid value logged and skipped | `tools/tool_failure.py:177-208` |
| Policy fields per scope | BaseTool field | `tools/base_tool.py:188-194` |
| Policy fields per scope | CrewStructuredTool field | `tools/structured_tool.py:214` |
| Policy fields per scope | Task override ("for this task only") | `task.py:283-287` |
| Policy fields per scope | Agent field | `agents/agent_builder/base_agent.py:307-313`; `lite_agent.py:231-237` |
| Policy fields per scope | Crew baseline | `crew.py:239-243` |
| Detection is declarative | `detect_tool_failure` matches only `ToolFailure` instances | `tools/tool_failure.py:154-162` |
| Exception→failure bridge | `failure_from_exception` sets code=exception class name | `tools/tool_failure.py:165-174` |
| Central handler | `handle_tool_failure`: record → emit `ToolFailureDetectedEvent` → optional RAISE | `tools/tool_failure.py:324-384` |
| Event emission before abort | comment: emitted before RAISE so subscribers see it even on aborting run | `tools/tool_failure.py:371-379`; test `tests/tools/test_tool_failure.py:376` |
| Execution-scoped collection | ContextVar `tool_failure_collector()` isolates concurrent executions on shared agents | `tools/tool_failure.py:260-285` |
| Dedup across guardrail retries | `merge_tool_failures` drops equivalent records | `tools/tool_failure.py:211-234` |
| ReAct path usage-limit failure | inline `ToolFailure(reason=USAGE_LIMIT)` when `max_usage_count` spent | `tools/tool_usage.py:581-595` (sync), `324-338` (async) |
| Usage limit check | message template built in `_check_usage_limit` | `tools/tool_usage.py:791-808` |
| Atomic usage claim (native path) | `_claim_usage` returns ToolFailure instead of raising, "so every execution path records a spent limit" | `tools/base_tool.py:302-324` |
| Unknown tool (ReAct) | fuzzy `SequenceMatcher > 0.85` match; emits `ToolSelectionErrorEvent`; raises with available-tools list | `tools/tool_usage.py:810-853` |
| Unknown tool (execute wrapper) | i18n `wrong_tool_name` + `handle_tool_failure(UNKNOWN_TOOL)` | `utilities/tool_utils.py:344-360` |
| Unknown tool (native path) | `"Tool not found"` result + UNKNOWN_TOOL failure | `utilities/agent_utils.py:1701-1702,1787-1794` |
| Invalid input (native arg decode) | parse error no longer swallowed into empty args; `handle_tool_failure(INVALID_INPUT)` + tool message back to LLM | `utilities/agent_utils.py:1659-1681` |
| Invalid input (ReAct parse) | `ToolUsageError(tool_arguments_error)` returned, not raised | `tools/tool_usage.py:889-910` |
| JSON repair ladder | json.loads → ast.literal_eval → json5 → repair_json, then `ToolValidateInputErrorEvent` + raise | `tools/tool_usage.py:939-985` |
| Raised-tool handling (ReAct) | `on_tool_error` emits `ToolUsageErrorEvent`; retry until `_max_parsing_attempts`; then i18n `tool_usage_exception` + "Moving on then." + format slice | `tools/tool_usage.py:708-736` (sync), `451-479` (async) |
| Retry counters | `_run_attempts=1`, `_max_parsing_attempts=3` (2 for big OpenAI models) | `tools/tool_usage.py:109-110,130-135` |
| Parse-failure retry loop | recursion until attempts exhausted; returns `tool_usage_error` text with format instructions | `tools/tool_usage.py:912-937` |
| Repeated-usage circuit breaker | identical consecutive call returns `task_repeated_usage` text instead of re-executing | `tools/tool_usage.py:779-789,509-525` |
| Native path raised-tool handling | `"Error executing tool: {e}"` becomes both the tool message and `failure_from_exception(e)`; `ToolUsageErrorEvent` emitted | `utilities/agent_utils.py:1767-1786` |
| Failed tool ≠ final answer | `result_as_answer` suppressed when `last_failure is not None` (all three paths) | `utilities/tool_utils.py:337-342,174-179`; `utilities/agent_utils.py:1861-1870`; `tools/tool_usage.py:666-674,409-417` |
| Cache hygiene | `CacheHandler.add` refuses to store a `ToolFailure` ("replaying one would make a transient error permanent") | `agents/cache/cache_handler.py:23-41` |
| MCP isError conversion | `_MCPToolResult.is_error` → `ToolFailure(reason=MCP_ERROR, details={server, tool})` | `tools/mcp_native_tool.py:133-147`; `mcp/client.py:597-613` |
| MCP operator events | `MCPToolExecutionFailedEvent(error, error_type="tool_error", started_at, failed_at)` | `mcp/client.py:480-503` |
| MCP transport retries | `_retry_operation`: exponential backoff `2**attempt`; auth/unauthorized non-retryable→ConnectionError; "not found"→ValueError; exhaustion raises ConnectionError | `mcp/client.py:688-739` |
| TaskOutput surfacing | `tool_failures: list[ToolFailureRecord]` + `has_tool_failures` property | `tasks/task_output.py:50-61` |
| CrewOutput aggregation | `CrewOutput.tool_failures` flattens across tasks | `crews/crew_output.py:36-47` |
| Collector wiring into outputs | `with tool_failure_collector() as execution_failures:` → `TaskOutput(..., tool_failures=...)` | `task.py:693-731` (async), `849-886` (sync) |
| Guardrail retry accumulation | failures from blocked attempts survive retries via `merge_tool_failures(accumulated_failures, retry_failures)` | `task.py:1339-1437` (sync), `1462-1558` (async) |
| Guardrail exhaustion | raises `Exception("Task failed {guardrail} validation after N retries. Last error: ...")` | `task.py:1376-1385` |
| Guardrail retry feedback to model | i18n `validation_error`: "Previous attempt failed validation: {error} ... Try again" | `translations/en.json:55`; consumed `task.py:1394-1397` |
| Agent-level retry bound | `max_retry_limit: int = 2` | `agent/core.py:255-258` |
| Retry exhaustion (agent layer) | `_check_execution_error`: litellm errors re-raised immediately; else increment `_times_executed`, raise past `max_retry_limit` | `agent/core.py:721-747` |
| Abort passthrough | `_passthrough_exceptions = (ToolExecutionFailedError,)` excluded from retry handlers; `_times_executed == 0` asserted in test | `agent/core.py:141,919`; `tests/tools/test_tool_failure.py:689-704` |
| Step executor treats abort as deliberate | re-raises `ToolExecutionFailedError` instead of returning `StepResult(success=False)` which would let plan continue | `agents/step_executor.py:184-187,227-230` |
| LiteAgent loop passthrough | `except ToolExecutionFailedError: raise` before generic handlers | `lite_agent.py:980-982` |
| User-facing console (raised error) | red "Tool Failed" panel with Tool/Iteration/Attempt/Error | `events/utils/console_formatter.py:545-570` |
| User-facing console (reported failure) | red "⚠️ Tool Failure" panel with Reason/Code/Message/Policy; success panel suppressed | `events/utils/console_formatter.py:496-543` |
| Listener wiring | `@crewai_event_bus.on(ToolUsageErrorEvent/ToolFailureDetectedEvent/ToolUsageFinishedEvent)` route to formatter | `events/event_listener.py:545-584` |
| Verbose direct prints | PRINTER red prints on selection/validation/execution errors | `tools/tool_usage.py:153-157,474-475,731-732,932-933` |
| Operator telemetry | PostHog/OpenTelemetry span "Tool Usage Error" with llm+tool_name attributes | `telemetry/telemetry.py:666-694` |
| Parse failures anonymized in metrics | deliberate empty tool_name so raw model output never enters a metrics dimension | `tools/tool_usage.py:925-929` |
| Error counter on task | `Task.tools_errors` incremented at every failure site | `task.py:148,1138-1140`; `tools/tool_usage.py:156,164,...` |
| Model-facing message catalog | `errors` block: force_final_answer(_error), unexisting coworker, repeated usage, wrong name, arguments error, usage exception, validation_error | `translations/en.json:45-56` |
| Delegation tools (legacy strings) | coworker-not-found and execution errors return formatted strings, no `ToolFailure` | `tools/agent_tools/base_agent_tools.py:88-124` |
| Iteration cap recovery path | `handle_max_iterations_exceeded` injects `force_final_answer` prompt instead of dying | `utilities/agent_utils.py:376-433` |
| Human review scope | `human_input` = "human review the final answer", not tool failures | `task.py:233-236` |
| Flow HITL exists but separate | `HumanFeedbackPending` pause/resume providers are flow-level control flow | `flow/async_feedback/types.py:148-294` |
| Docs match implementation | Reporting/policy/inspection guide incl. precedence sentence | `docs/edge/en/concepts/tools.mdx:337-455` |

## Answers to Dimension Questions

**1. Who sees tool failure?**
Everyone, in different shapes. The *model* sees prose: either `as_agent_message()` for declared failures (`tools/structured_tool.py:62-63`) or i18n error templates (`translations/en.json:45-56`) delivered as the Observation / tool message. The *user* sees rich-console panels (red "Tool Failed" / "⚠️ Tool Failure"; `events/utils/console_formatter.py:513-570`) gated on `verbose`. *Programmatic consumers* see structured data: `ToolFailureDetectedEvent` on the bus (`tools/tool_failure.py:362-379`), a `failure` field on `ToolUsageFinishedEvent` (`events/types/tool_usage_events.py:63-75`), and `tool_failures` on `TaskOutput`/`CrewOutput`/`LiteAgentOutput` (`tasks/task_output.py:50-61`). *Operators* get OpenTelemetry/PostHog spans ("Tool Usage Error", `telemetry/telemetry.py:666-694`) and MCP lifecycle events (`mcp/client.py:489-503`).

**2. Is the error actionable?**
Mostly yes. Unknown-tool messages enumerate valid tools (`translations/en.json:52`, `tools/tool_usage.py:836`); exception messages include what inputs the tool accepts (`en.json:53`, `tools/tool_usage.py:717-726`); exhausted-parse messages append the format reminder and "Moving on then." (`tools/tool_usage.py:467-469`); schema-validation failures append a generated schema hint (`tools/base_tool.py:296-299`). Weakest spot: the native path's bare `"Error executing tool: {e}"` (`utilities/agent_utils.py:1768`) offers no input guidance, and delegation errors embed role names but no remediation (`tools/agent_tools/base_agent_tools.py:89-107`).

**3. Can the model recover?**
Yes — this is the design's core bet. Failures are returned to the loop as ordinary text rather than thrown, so the model can adjust; the same-input retry is intercepted by a repeated-usage breaker (`tools/tool_usage.py:779-789`); malformed calls fall back to an instructor-LLM reparse before failing (`tools/tool_usage.py:912-921`); JSON repair runs a four-strategy ladder before giving up (`tools/tool_usage.py:948-985`); and after in-call retries exhaust, execution continues rather than aborting. At the task layer, guardrail rejections feed the validation error back as context for another attempt (`task.py:1394-1408`). Recovery ends when iteration caps trigger `force_final_answer` (`utilities/agent_utils.py:376-433`) or `raise` policy aborts deliberately.

**4. When is failure escalated to a human?**
Never automatically. Options are: `tool_failure_policy="raise"` aborts the run and hands `ToolExecutionFailedError` to the caller (`tools/tool_failure.py:381-382`); humans watch verbose panels; subscribers handle `ToolFailureDetectedEvent` themselves (documented pattern, `docs/edge/en/concepts/tools.mdx:441-455`); callers inspect `has_tool_failures` post-kickoff. `task.human_input` covers final-answer review only (`task.py:233-236`), and flow-level `HumanFeedbackPending` HITL is unrelated to tool failures (`flow/async_feedback/types.py:148`). No evidence found of any hook that pages/alerts a human specifically on tool failure.

**5. Are failures grouped by cause?**
Yes, structurally. `ToolFailureReason` gives six categories (`tools/tool_failure.py:35-54`), a test asserts every member is actually produced (`tests/tools/test_tool_failure.py:1577`), records dedupe by (tool, message, code, task_id, args) (`tools/tool_failure.py:211-234`), and each record carries correlation ids (agent_role/task_id; event ids match the paired finished event, tests at `test_tool_failure.py:1218-1240`). Telemetry counts errors per tool_name but does not tag spans with reason — grouping by cause stops at the application layer.

## Architectural Decisions

1. **Declarative over heuristic failure detection.** Only an explicit `ToolFailure` return marks failure; strings about errors are never misread (`tools/tool_failure.py:154-162`, doc rationale `docs/edge/en/concepts/tools.mdx:374-378`). Tradeoff: tools that don't opt in remain invisible to the machinery — the pre-existing string-error culture (delegation tools, `base_agent_tools.py:88-124`) keeps working but untracked.
2. **Policy resolution as a scope chain.** One attribute name (`tool_failure_policy`) on five classes with most-specific-wins semantics and safe degradation on bad values (`tools/tool_failure.py:190-208`) — a small, uniform config surface instead of per-layer flags.
3. **Errors-as-data flowing forward, exceptions reserved for policy.** The model always gets text (recovery path); the *framework* gets objects (observability path); only RAISE converts data back into a control-flow exception, protected by `_passthrough_exceptions` (`agent/core.py:141`) so retries can't swallow it (tested: `tests/tools/test_tool_failure.py:741-764`).
4. **ContextVar-scoped collection for concurrency correctness.** Records accumulate in a `tool_failure_collector()` context rather than mutable agent state alone, because agents are shared across concurrent tasks (`tools/tool_failure.py:260-285`; tests `1350-1462`).
5. **Three parallel execution paths kept behaviorally aligned by tests, not shared code.** ReAct (`tools/tool_usage.py`), native function-calling (`utilities/agent_utils.py:1602-1879`), and experimental executor duplicate the detect→record→emit sequence; comments and cross-referencing tests ("Mirrors the native paths...", `utilities/tool_utils.py:95-96`) enforce parity.

## Notable Patterns

- **Envelope + record split**: the tool-authored `ToolFailure` vs framework-contextualized `ToolFailureRecord` (`tools/tool_failure.py:71-143`) keeps authoring simple while downstream gets correlation data.
- **Suppression predicates for console truthfulness**: `should_render_success_panel(failure)` swaps green for red; EXCEPTION-reason failures skip the second panel since `ToolUsageErrorEvent` already printed one (`events/utils/console_formatter.py:496-511`).
- **Cache poisoning defense**: failures are never cached (`agents/cache/cache_handler.py:26-41`), and a blocked-by-hook call clears any attributed cached failure (`utilities/agent_utils.py:1741-1746`).
- **Metrics hygiene comment**: parse failures intentionally report an empty tool name so unbounded model output never enters a metrics dimension (`tools/tool_usage.py:925-929`).
- **Fuzzy tool selection**: `SequenceMatcher` ratio > 0.85 rescues near-miss tool names before declaring unknown (`tools/tool_usage.py:812-825`).
- **Model-tier adaptive retries**: `_max_parsing_attempts` drops from 3 to 2 for large OpenAI models (`tools/tool_usage.py:130-135`).

## Tradeoffs

- **Dual representation risk**: a failure is simultaneously prose (to the model) and object (to the framework). The seam is handled carefully today (`structured_tool.py:59-63`), but any new formatting path must remember both halves; the `result_as_answer` exclusion had to be replicated at three call sites (`utilities/tool_utils.py:341`, `utilities/agent_utils.py:1869`, `tools/tool_usage.py:671`).
- **WARN default favors availability over strictness**: a crew finishes "successfully" with failures attached; docs explicitly warn users to check `has_tool_failures` before trusting `raw` (`docs/edge/en/concepts/tools.mdx:437-439`). Silent-tolerant defaults shift the burden onto callers who forget.
- **Retry budget opacity**: the ReAct path mixes parse retries and execution retries into one `_run_attempts` counter against `_max_parsing_attempts` (`tools/tool_usage.py:109-110,711-736`), so a flaky tool consumes the same budget as a malformed call, and neither is configurable per tool.
- **Duplication cost**: sync/async twins inside `tool_usage.py` (`_use`/`_ause`, nearly 250 mirrored lines) triple the maintenance surface for failure-handling changes.

## Failure Modes / Edge Cases

- **RAISE swallowed by a future handler**: mitigated, not eliminated — five handlers historically downgraded the abort; the suite now asserts passthrough at each site (`tests/tools/test_tool_failure.py:741-764`), but a brand-new `try/except` elsewhere would need its own test.
- **Legacy untracked failures**: delegation tools returning error strings (`tools/agent_tools/base_agent_tools.py:121-123`) and native-path generic messages produce no record under WARN; a run can look clean while the model repeatedly failed to delegate.
- **`retryable` is dead metadata**: set by producers (`tools/tool_failure.py:95-98`, `lib/crewai-tools/src/crewai_tools/tools/crewai_platform_tools/crewai_platform_action_tool.py:97`) with zero automated readers (repo-wide search found only production sites and a test assertion). Transient-vs-permanent classification must be reimplemented by every consumer.
- **Guardrail-exhaustion raises bare `Exception`** with a formatted string (`task.py:1382-1385`) — no typed error carries the accumulated `tool_failures`, unlike the RAISE-policy path.
- **Telemetry blind to cause**: `tool_usage_error` spans record llm/tool_name only (`telemetry/telemetry.py:666-694`); operators cannot distinguish rate-limits from schema mismatches without replaying events.
- **MCP timeout misclassification window**: `_retry_operation` classifies by substring (`"authentication"`, `"not found"` in lowercased message, `mcp/client.py:719-727`), so a tool whose payload contains those phrases would be misclassified as non-retryable.
- **Repeated-usage breaker is memory-of-one**: only the immediately previous call is compared (`tools/tool_usage.py:784-789`), so alternating A,B,A,B loops evade it until iteration caps bite.

## Future Considerations

- Wire `ToolFailure.retryable` into an actual retry decision point (e.g., let USAGE_LIMIT/UNKNOWN_TOOL be non-retrying while transient codes auto-retry with backoff inside `ToolUsage`), turning advisory metadata into behavior.
- Add `reason`/`code` attributes to the "Tool Usage Error" telemetry span and type the guardrail-exhaustion error so programmatic handlers can branch on it.
- Extract the shared detect→record→emit sequence out of `tool_usage.py`, `agent_utils.py`, and `experimental/agent_executor.py` to retire the comment-enforced parity ("Mirrors the native paths...").
- Migrate delegation tools (`tools/agent_tools/base_agent_tools.py:88-124`) to return `ToolFailure(UNKNOWN_TOOL/EXCEPTION)` so their failures join the record stream.
- Document an operator recipe (event subscription → alerting) for teams wanting human escalation on tool failure; today that integration is left entirely to users despite the event being designed for exactly that (`docs/edge/en/concepts/tools.mdx:441-455`).

## Questions / Gaps

- **No automatic human-in-the-loop escalation for tool failures was found.** Searched `human_input`, `HumanFeedback*`, `hitl_*`, and event listener registrations across `lib/crewai/src`. The nearest mechanisms (task final-answer review `task.py:233-236`; flow HITL `flow/async_feedback/types.py:222-294`) are orthogonal to tool failures. If such escalation exists, it lives outside this repository (platform/enterprise features referenced by `plus_api.py` were not part of the studied surface).
- **No persistence/audit log of failure records** beyond `TaskOutput`/event streams was found (no database/file sink in the OSS code searched); long-term failure analytics depend on consumers exporting events themselves.
- Whether the `experimental/agent_executor.py` copy of the failure pipeline (lines `1935`, `2129`) is scheduled for deletion or promotion could not be determined from the source; it tracks `tool_failure.py` API but duplicates logic.

---

Generated by `07.08-tool-failure-escalation` against `crewai`.

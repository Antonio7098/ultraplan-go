# Source Analysis: crewai

## Dimension 13.04: Recovery vs Escalation

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (monorepo: `lib/crewai` core framework, `lib/crewai-core`, `lib/crewai-tools`, `lib/crewai-files`, `lib/cli`, `lib/devtools`) |
| Analyzed | 2026-08-25 |

> Citation convention: all paths below are relative to the source root
> `studies/agent-harness-study/sources/crewai/`.

## Summary

CrewAI implements recovery as a stack of independent, per-layer retry loops rather than a single recovery engine. At the bottom, native LLM clients retry transport failures (`max_retries`, default 2, in each provider). Above that, the agent layer retries a whole task execution up to `max_retry_limit` (default 2) when an exception escapes the executor, while deliberately excluding timeouts and tool-policy aborts from the retry path. Guardrails add their own bounded retry loop (default 3) that re-prompts the agent with the validation error. Context-window overflow is recovered by LLM summarization of the conversation instead of failing.

Escalation to humans is implemented exclusively as human-in-the-loop feedback rather than alerting/paging: a pluggable `HumanInputProvider` protocol for task-level feedback, a richer flow-level HITL system where a provider can raise `HumanFeedbackPending` to pause execution with automatic state persistence and later resume, and hook-based approval gates around individual tool calls. There is no first-class "escalation" concept — the word does not appear in the codebase; escalation is implicit either as "raise to caller" or "ask a human via a provider".

Every failure and recovery decision is observable through an extensive typed event bus taxonomy (`AgentExecutionErrorEvent`, `ToolFailureDetectedEvent`, `LLMCallFailedEvent`, guardrail events carrying `retry_count`, flow paused/failed events) plus opt-in file logging and telemetry spans, which makes recovery decisions auditable even though there is no dedicated audit log schema for them.

**Rating: 7/10**

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Agent task retry config | `max_retry_limit: int = Field(default=2, ...)` on the agent; private attempt counter `_times_executed` | `lib/crewai/src/crewai/agent/core.py:255-258`, `lib/crewai/src/crewai/agent/core.py:208` |
| Retry decision point | `_check_execution_error`: passthrough exceptions re-raised, litellm errors never retried, otherwise counter incremented and raised once `> max_retry_limit`; every failed attempt emits `AgentExecutionErrorEvent` | `lib/crewai/src/crewai/agent/core.py:721-747` |
| Sync/async retry handlers | `_handle_execution_error` / `_handle_execution_error_async` re-invoke `execute_task` / `aexecute_task` | `lib/crewai/src/crewai/agent/core.py:749-789` |
| Timeouts not retried | `TimeoutError` caught separately, event emitted, re-raised immediately; enforced via thread-pool future timeout | `lib/crewai/src/crewai/agent/core.py:872-881`, `lib/crewai/src/crewai/agent/core.py:888-926`; config at `lib/crewai/src/crewai/agent/core.py:211-214`, validation at `lib/crewai/src/crewai/agent/utils.py:304-314` |
| Deliberate aborts excluded from retry | `_passthrough_exceptions = (ToolExecutionFailedError,)` honored in both error check and timeout wrapper | `lib/crewai/src/crewai/agent/core.py:141`, `lib/crewai/src/crewai/agent/core.py:731-732`, `lib/crewai/src/crewai/agent/core.py:919-923` |
| Tool failure policy model | Declarative `ToolFailure` (with `retryable` flag), `ToolFailurePolicy` enum IGNORE/WARN/RAISE, `ToolExecutionFailedError` abort | `lib/crewai/src/crewai/tools/tool_failure.py:57-68`, `lib/crewai/src/crewai/tools/tool_failure.py:95-98`, `lib/crewai/src/crewai/tools/tool_failure.py:146-152` |
| Policy precedence resolution | tool → task → agent → crew → default WARN; malformed policy values are warned about and skipped, not fatal | `lib/crewai/src/crewai/tools/tool_failure.py:177-208` |
| Policy application + event | `handle_tool_failure` records failure, emits `ToolFailureDetectedEvent`, raises under RAISE policy | `lib/crewai/src/crewai/tools/tool_failure.py:324-384` |
| Task guardrail retry loop | `guardrail_max_retries` default 3 (deprecated alias `max_retries`); per-guardrail independent counters; final failure raises generic Exception with last error | `lib/crewai/src/crewai/task.py:275-282`, `lib/crewai/src/crewai/task.py:304`, `lib/crewai/src/crewai/task.py:1332-1392` |
| Guardrail retry re-prompt | Failed validation is fed back as task context and the whole task re-executes; tool-failure records accumulated across attempts | `lib/crewai/src/crewai/task.py:1394-1408`, `lib/crewai/src/crewai/task.py:1339-1341` |
| LiteAgent guardrail retry | Standalone agent path retries guardrail with its own counter and raises after limit | `lib/crewai/src/crewai/lite_agent.py:276-277`, `lib/crewai/src/crewai/lite_agent.py:742-769` |
| Guardrail events carry retry_count | `LLMGuardrailStartedEvent`/`CompletedEvent` include `retry_count` | `lib/crewai/src/crewai/utilities/guardrail.py:162-185` |
| Context-window recovery | `respect_context_window` default True → summarize messages via LLM; opted-out → `SystemExit`; `LLMContextLengthExceededError` intentionally propagated to executor for this decision | `lib/crewai/src/crewai/agent/core.py:251-254`, `lib/crewai/src/crewai/utilities/agent_utils.py:795-832`, `lib/crewai/src/crewai/llm.py:1909-1913` |
| Max-iteration graceful stop | On reaching `max_iter`, one forced final-answer LLM call instead of crash ("Requesting final answer") | `lib/crewai/src/crewai/utilities/agent_utils.py:363-433`, used at `lib/crewai/src/crewai/agents/crew_agent_executor.py:343-344` |
| Parse-error self-repair | `OutputParserError` appended to message history so the agent retries parsing ("agent will retry" logged after 3 silent iterations) | `lib/crewai/src/crewai/utilities/agent_utils.py:743-778` |
| Provider-level retries | OpenAI/Anthropic/Azure clients expose `max_retries: int = 2` (configurable); Bedrock uses adaptive retry mode with 3 attempts | `lib/crewai/src/crewai/llms/providers/openai/completion.py:223`, `lib/crewai/src/crewai/llms/providers/anthropic/completion.py:221`, `lib/crewai/src/crewai/llms/providers/azure/completion.py:82`, `lib/crewai/src/crewai/llms/providers/bedrock/completion.py:316` |
| Unsupported-param self-heal retry | On "'stop' unsupported" errors, drops the param and retries once; emits `LLMCallFailedEvent` on other failures | `lib/crewai/src/crewai/llm.py:1914-1957` |
| Bounded reasoning-effort retry | OpenAI responses API retries once with `reasoning_effort=None` for incompatible-tool 400s; tests assert it neither retries genuine unknown models nor unrelated 400s nor forever | `lib/crewai/src/crewai/llms/providers/openai/completion.py:539-547`, `lib/crewai/tests/llms/openai/test_tools_reasoning_effort_retry.py:157-173` |
| Task-level HITL flag | `human_input` field on Task; plumbed into executor inputs as `ask_for_human_input` | `lib/crewai/src/crewai/task.py:233`, `lib/crewai/src/crewai/agent/core.py:946`, `lib/crewai/src/crewai/agents/crew_agent_executor.py:227,243-244` |
| Human input provider protocol | `HumanInputProvider` Protocol with sync/async feedback flow; default terminal-based `SyncHumanInputProvider`; empty input ends the loop gracefully; swap via `set_provider` ContextVar | `lib/crewai/src/crewai/core/providers/human_input.py:60-66`, `lib/crewai/src/crewai/core/providers/human_input.py:147-264`, `lib/crewai/src/crewai/core/providers/human_input.py:445-483` |
| Async non-blocking prompt | `_async_readline` uses event-loop stdin I/O, falls back to `asyncio.to_thread(input)` on unsupported platforms | `lib/crewai/src/crewai/core/providers/human_input.py:425-442` |
| Flow HITL pause/resume | Providers raise `HumanFeedbackPending` (explicitly "not an error, a control flow signal"); framework persists state automatically and returns the pending object as kickoff value | `lib/crewai/src/crewai/flow/async_feedback/types.py:148-218`, `lib/crewai/src/crewai/flow/runtime/__init__.py:1524-1554` |
| HumanFeedbackProvider protocol | Pluggable console/Slack/email providers; async providers notify external system then raise pending | `lib/crewai/src/crewai/flow/async_feedback/types.py:222-298`, `lib/crewai/src/crewai/flow/async_feedback/providers.py:19` |
| Paused ≠ failed distinction | Method execution catches `HumanFeedbackPending` and emits `MethodExecutionPausedEvent` instead of `MethodExecutionFailedEvent` | `lib/crewai/src/crewai/flow/runtime/__init__.py:2948-2987` |
| Pending-feedback persistence | SQLite backend stores pending feedback context for later resume (`from_pending(...).resume(...)`) | `lib/crewai/src/crewai/flow/persistence/sqlite.py:31-51`, `lib/crewai/src/crewai/flow/persistence/base.py:70-108` |
| Hook approval gates | `ToolCallHookContext.request_human_input` enables per-tool approval prompts (e.g., block destructive tools unless "approve") | `lib/crewai/src/crewai/hooks/tool_hooks.py:86-128`, example at `lib/crewai/src/crewai/hooks/decorators.py:234` |
| Training-mode escalation to human | `crew.train()` sets `task.human_input = True` on every task | `lib/crewai/src/crewai/crew.py:927-938` |
| Crew failure propagation | `kickoff` catches all exceptions, dispatches EXECUTION_END(status="failed"), emits `CrewKickoffFailedEvent`, re-raises — no crew-level retry | `lib/crewai/src/crewai/crew.py:1068-1086` |
| Failure event taxonomy | Dedicated failed-event types across subsystems (crew, train, flow, method, LLM call, MCP, knowledge, skills, tasks, agents) | `lib/crewai/src/crewai/events/event_listener.py:28-118` |
| Audit surfaces | Event listener wires failures into formatter status + telemetry; telemetry records tool usage errors and run outcome with exception class name only; optional `output_log_file` logs task started/completed | `lib/crewai/src/crewai/events/event_listener.py:192-235`, `lib/crewai/src/crewai/telemetry/telemetry.py:666-674`, `lib/crewai/src/crewai/telemetry/telemetry.py:1071-1124`, `lib/crewai/src/crewai/crew.py:340`, `lib/crewai/src/crewai/crew.py:1843-1850` |
| Retry behavior tests | `test_agent_max_retry_limit` proves exactly one retry at `max_retry_limit=1`; scope-per-attempt event test; guardrail retry tests incl. per-guardrail tracking | `lib/crewai/tests/agents/test_agent.py:1292-1325`, `lib/crewai/tests/utilities/test_events.py:424`, `lib/crewai/tests/test_task_guardrails.py:482`, `lib/crewai/tests/test_task_guardrails.py:734` |
| Abort-not-retried test | `test_retry_limit_does_not_swallow_the_abort` asserts `_times_executed == 0` under RAISE policy | `lib/crewai/tests/tools/test_tool_failure.py:689-704` |
| HITL resumption regression tests | HITL resumption skips completed listeners; conditional/cyclic resumption cases | `lib/crewai/tests/test_flow_resumability_regression.py:14,56,93` |

## Answers to Dimension Questions

### 1. When does the system retry vs escalate?

Retry happens inside three nested loops, each with its own budget:

- **LLM transport layer**: provider-native clients retry connection/rate-limit failures (`max_retries=2` defaults, e.g. `lib/crewai/src/crewai/llms/providers/openai/completion.py:223`). One-shot protocol self-heals also exist here (drop unsupported `stop` param, `lib/crewai/src/crewai/llm.py:1921-1946`; drop `reasoning_effort`, `lib/crewai/src/crewai/llms/providers/openai/completion.py:539-547`).
- **Agent layer**: any exception escaping the executor that is not a litellm error and not a `_passthrough_exceptions` member triggers a full task re-execution until `_times_executed > max_retry_limit` (`lib/crewai/src/crewai/agent/core.py:721-768`).
- **Guardrail layer**: failed output validation re-executes the task with the error as context up to `guardrail_max_retries` times before raising (`lib/crewai/src/crewai/task.py:1343-1408`).

In-loop soft recovery without consuming any retry budget: parse errors are fed back into the message history (`lib/crewai/src/crewai/utilities/agent_utils.py:763-778`) and hitting `max_iter` forces one final-answer call instead of aborting (`lib/crewai/src/crewai/utilities/agent_utils.py:376-433`).

Escalation happens when: (a) retries are exhausted and the exception propagates to the caller — the crew level adds no retry, only failure events plus re-raise (`lib/crewai/src/crewai/crew.py:1068-1078`); (b) `tool_failure_policy=RAISE` turns a declared tool failure into an immediate `ToolExecutionFailedError` abort (`lib/crewai/src/crewai/tools/tool_failure.py:381-382`); (c) a human is explicitly asked via `human_input=True` or a flow-level feedback provider. Certain failures bypass recovery entirely by design: `TimeoutError` (`lib/crewai/src/crewai/agent/core.py:872-881`) and litellm-originated errors (`lib/crewai/src/crewai/agent/core.py:743-744`) are never retried.

### 2. Are escalation thresholds configurable?

Yes, but per-layer rather than through one policy object: `max_retry_limit` per agent (`lib/crewai/src/crewai/agent/core.py:255-258`), `guardrail_max_retries` per task with a deprecated legacy alias (`lib/crewai/src/crewai/task.py:275-282`) and again per LiteAgent (`lib/crewai/src/crewai/lite_agent.py:276-277`), `max_execution_time` per agent (`lib/crewai/src/crewai/agent/core.py:211-214`), `max_iter` per agent loop, provider `max_retries` per LLM client, and `tool_failure_policy` resolvable at four scopes with most-specific-wins semantics (`lib/crewai/src/crewai/tools/tool_failure.py:177-208`). Who gets asked (the human channel) is fully pluggable via `set_provider` (`lib/crewai/src/crewai/core/providers/human_input.py:465-474`) and the `HumanFeedbackProvider` protocol (`lib/crewai/src/crewai/flow/async_feedback/types.py:222-247`). What is *not* configurable: backoff timing between task retries (none exists), and there is no severity/classification scheme that routes different failure classes to different escalation targets.

### 3. Can the system stop gracefully?

Yes, in several graded ways. The cleanest example is the flow HITL design: raising `HumanFeedbackPending` pauses execution, auto-persists state and pending-feedback context, emits `MethodExecutionPausedEvent`/`FlowPausedEvent` (explicitly distinguished from failed), and returns the pending object as a *value* so callers need no try/except (`lib/crewai/src/crewai/flow/async_feedback/types.py:148-172`, `lib/crewai/src/crewai/flow/runtime/__init__.py:2948-2973`). At the agent loop, `max_iter` exhaustion produces a best-effort final answer rather than an error (`lib/crewai/src/crewai/utilities/agent_utils.py:397-401`), and an empty human feedback string cleanly ends the feedback loop (`lib/crewai/src/crewai/core/providers/human_input.py:256-258`). Two graceless edges remain: context overflow with `respect_context_window=False` kills the process with `SystemExit` rather than a domain exception (`lib/crewai/src/crewai/utilities/agent_utils.py:824-832`), and exhausted guardrail retries raise bare `Exception` instead of a typed error (`lib/crewai/src/crewai/task.py:1382-1385`).

### 4. Are recovery decisions auditable?

Largely yes, via the event bus rather than a dedicated audit store. Every failed task attempt emits `AgentExecutionErrorEvent` including on attempts that will be retried (`lib/crewai/src/crewai/agent/core.py:733-742`); guardrail evaluations emit started/completed events carrying `retry_count` (`lib/crewai/src/crewai/utilities/guardrail.py:162-185`); tool failures are recorded structurally on `TaskOutput.tool_failures` and emitted as `ToolFailureDetectedEvent` with the resolved policy attached (`lib/crewai/src/crewai/tools/tool_failure.py:346-379`), and merged/deduplicated across retry attempts so blocked attempts survive to the final output (`lib/crewai/src/crewai/tools/tool_failure.py:211-234`, tested at `lib/crewai/tests/tools/test_tool_failure.py:811,1600`). Kickoff/train failures emit their own events consumed by the listener for status formatting and telemetry spans (`lib/crewai/src/crewai/events/event_listener.py:192-235`), telemetry records tool usage errors and end-of-run outcome with the exception class name (`lib/crewai/src/crewai/telemetry/telemetry.py:666-674,1071-1124`), and `output_log_file` gives per-task started/completed lines (`lib/crewai/src/crewai/crew.py:1843-1886`). Gaps: no persisted audit record of *why* a retry was chosen vs escalated beyond the event stream, and the file log records no "failed" status line in `_process_task_result`'s completed-only logging.

## Architectural Decisions

1. **Layered, independent retry budgets instead of a central supervisor.** Provider clients, agent task execution, and guardrails each own their own counters and policies. This keeps concerns local but means budgets can compound (a guardrail retry re-executes the task, which itself may consume agent retries).
2. **Explicit non-retryable classes.** `_passthrough_exceptions = (ToolExecutionFailedError,)` plus litellm-error short-circuit encode "deliberate stops must not be retried as transient errors", with a dedicated test proving the abort survives `max_retry_limit=3` (`lib/crewai/src/crewai/agent/core.py:141,731-744`, `lib/crewai/tests/tools/test_tool_failure.py:689-704`).
3. **Declarative tool failure signaling.** Tools return a structured `ToolFailure` (message/reason/code/retryable) instead of error strings; detection "is strictly declarative — nothing guesses whether a string looks like an error" (`lib/crewai/src/crewai/tools/tool_failure.py:1-11,154-162`).
4. **Escalation-as-control-flow for flows.** `HumanFeedbackPending` subclasses `Exception` purely so it can unwind the stack, but is documented as "not an error, a control flow signal" and returned as a value to callers, with automatic persistence (`lib/crewai/src/crewai/flow/async_feedback/types.py:148-172`).
5. **Context pressure solved by summarization, not failure.** The executor decides between LLM summarization and abort based on `respect_context_window`, with `LLMContextLengthExceededError` deliberately re-raised from the LLM layer for exactly that decision point (`lib/crewai/src/crewai/llm.py:1909-1913`).
6. **Observability through a typed event bus.** A large taxonomy of failed/paused events across all subsystems (`lib/crewai/src/crewai/events/event_listener.py:28-118`) makes recovery decisions consumable by external systems without parsing logs.

## Notable Patterns

- **Policy precedence chain**: `resolve_tool_failure_policy` walks tool → wrapped original tool → task → agent → crew, warning on invalid values instead of crashing mid-call (`lib/crewai/src/crewai/tools/tool_failure.py:177-208`) — thoroughly unit-tested (`lib/crewai/tests/tools/test_tool_failure.py:174-323`).
- **Per-attempt event scoping**: each retry re-enters `execute_task`, opening a fresh `agent_execution_started` scope so traces stay balanced per attempt (`lib/crewai/src/crewai/agent/core.py:733-734`, test at `lib/crewai/tests/utilities/test_events.py:424`).
- **Accumulated failure memory across retries**: guardrail retries merge tool-failure lists so earlier blocked attempts are not erased by the retry's rebuilt output (`lib/crewai/src/crewai/task.py:1339-1341,1366-1372`).
- **ContextVar-based isolation**: human-input providers and tool-failure collectors use `ContextVar`s so concurrent executions don't cross-contaminate (`lib/crewai/src/crewai/core/providers/human_input.py:445-448`, `lib/crewai/src/crewai/tools/tool_failure.py:260-280`).
- **Bounded self-healing retries at protocol boundaries**: single-shot param-drop retries with explicit negative tests ensuring no infinite loops (`lib/crewai/tests/llms/openai/test_tools_reasoning_effort_retry.py:157-173`).

## Tradeoffs

- **Simplicity of blind retry over error classification**: the agent-layer retry treats any non-litellm exception identically — no backoff, jitter, or transient/permanent discrimination — trading token cost and latency predictability for implementation simplicity.
- **Local autonomy vs global coherence**: each layer's independent knobs are easy to reason about individually but compose unpredictably; worst case a single user-visible failure can consume up to `guardrail_max_retries × (max_retry_limit + 1)` full task executions.
- **Console-default HITL**: the built-in providers block on stdin (`input()`), ideal for CLIs, useless for servers — mitigated by the provider protocols being genuinely pluggable.
- **Event-stream auditability vs queryable audit log**: rich events make recovery observable live, but reconstructing a retry/escalation history requires an event-bus consumer; nothing persists decisions by default except opt-in flow-state checkpoints and `output_log_file`.
- **Graceful degradation breadth vs typed errors**: several stop conditions (SystemExit, bare `Exception` from guardrails) sacrifice programmatic handleability for terse implementation.

## Failure Modes / Edge Cases

- **Non-retryable failures silently retried**: any bug (e.g., a coding error in a callback) consumes the full retry budget because classification only excludes litellm modules and `ToolExecutionFailedError` (`lib/crewai/src/crewai/agent/core.py:731-747`).
- **Timeout leaks work**: the timed-out future is cancelled but the underlying thread's work cannot be interrupted (`future.cancel()` on a running future is advisory, `lib/crewai/src/crewai/agent/core.py:912-926`).
- **SystemExit on context overflow** bypasses normal exception handling/telemetry paths and kills interpreters embedding CrewAI (`lib/crewai/src/crewai/utilities/agent_utils.py:830-832`).
- **Deprecated duplicate knobs**: `max_retries` vs `guardrail_max_retries` on Task (`lib/crewai/src/crewai/task.py:275-282`, migration shim at `lib/crewai/src/crewai/task.py:578-582`) can desync expectations.
- **Callable guardrails dropped on checkpointing**: JSON checkpoint serialization warns and drops callable guardrails, so restored crews resume *without* validation (`lib/crewai/src/crewai/utilities/guardrail.py:12-31`).
- **Empty-string ambiguity in HITL**: empty input both means "approve/satisfied" and terminates the loop (`lib/crewai/src/crewai/core/providers/human_input.py:256-258`) — an accidental Enter silently accepts the result.
- **Invalid policy values degrade quietly**: a typo'd `tool_failure_policy` falls through to the next scope with only a warning (`lib/crewai/src/crewai/tools/tool_failure.py:198-207`), arguably safer than crashing but potentially masking intent.

## Future Considerations

- Introduce a shared, typed retry/backoff utility with error classification (transient vs permanent) at the agent layer, replacing the unconditional re-invoke in `_handle_execution_error` (`lib/crewai/src/crewai/agent/core.py:749-768`).
- Add a first-class escalation policy object (thresholds, channels, severity mapping) unifying the per-layer knobs; the `HumanFeedbackProvider` and event bus already provide the transport seams.
- Replace `SystemExit` and bare `Exception` paths with domain exceptions (e.g., a typed `GuardrailValidationError`) while preserving behavior via re-wrapping.
- Persist a structured decision log entry (attempt number, policy, outcome) alongside existing events for post-hoc audit queries.
- Extend `output_log_file` to log failed task terminations, matching the started/completed entries.

## Questions / Gaps

- No evidence found of any external human notification mechanism (email/Slack/webhook on failure) shipped in-repo — searched for "escalat", "notify", "webhook" patterns across `lib/`; the only supported path is building one atop the event bus or implementing a `HumanFeedbackProvider`. Slack appears solely as docstring examples (`lib/crewai/src/crewai/flow/human_feedback.py:40-43`, `lib/crewai/src/crewai/flow/async_feedback/types.py:179-194`).
- No evidence found of configurable inter-retry delay/backoff anywhere in the task/guardrail retry loops; only provider-native clients own backoff strategies (e.g., Bedrock adaptive mode, `lib/crewai/src/crewai/llms/providers/bedrock/completion.py:316`).
- Whether the hierarchical process manager applies different retry/escalation rules than sequential was not verified in depth; manager setup exists (`lib/crewai/src/crewai/crew.py:1507-1548`) but no distinct recovery policy surfaced during inspection.
- The exact interaction when `human_input=True` coincides with retry exhaustion (feedback prompt vs error precedence) was not exercised in the reviewed tests beyond `test_agent_max_retry_limit` using `human_input=True` merely as an invocation-shape fixture (`lib/crewai/tests/agents/test_agent.py:1300-1305`).

---

Generated by `Dimension 13.04: Recovery vs Escalation` against `crewai`.

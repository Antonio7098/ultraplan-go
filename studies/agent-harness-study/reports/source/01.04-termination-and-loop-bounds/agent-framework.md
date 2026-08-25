# Source Analysis: agent-framework

## Termination and Loop Bounds

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (core: agent-framework-core) + .NET sibling |
| Analyzed | 2026-07-02 |

## Summary

The Microsoft Agent Framework has three independent execution loops that can each iterate, and each has its own termination regime:

1. **Function-calling loop** inside the chat client (`use_function_invocation` / `FunctionInvocationLayer` in `agent_framework/_tools.py`). Bounded by `max_iterations` (default 40, `DEFAULT_MAX_ITERATIONS` at `_tools.py:90`) and an optional `max_function_calls` budget; on exhaustion it issues a final model call with `tool_choice="none"` so the model is forced to produce text. A `max_consecutive_errors_per_request` counter (default 3) aborts the loop on repeated tool errors.
2. **Workflow runner loop** in `agent_framework/_workflows/_runner.py` (Pregel-style supersteps). Bounded by `max_iterations` (default 100, `DEFAULT_MAX_ITERATIONS` in `_workflows/_const.py:4`). On exhaustion it raises `WorkflowConvergenceException` (`_runner.py:152-153`). Validation warns on self-loops at build time (`_workflows/_validation.py:401-413`).
3. **Agent re-invocation loop** in `agent_framework/_harness/_loop.py` (`AgentLoopMiddleware`). Bounded by `max_iterations` (default 10 for predicates, 5 for judge loops). On exhaustion the loop short-circuits before `should_continue` runs, and the result aggregator carries the final response back.

Across all three, termination is **runtime-driven** (counter + user predicate or budget), not model-driven. Finish reasons from the model (`stop`, `length`, `tool_calls`, `content_filter` in `_types.py:1648`) flow through but are not interpreted as a stop condition by the loops themselves — a `length` finish still costs an iteration. There is **no stuck-loop detector** (no hash-of-last-response, no repetition check, no cosine similarity) — the framework relies entirely on the hard cap and user-supplied predicates.

## Rating

**7 / 10** — Clear model with explicit interfaces, configurable per-loop caps, validated inputs, distinct exhaustion error types, and observability hooks. Missing: no stuck-loop detection short of the hard cap; finish reasons from the LLM are not loop-control signals; workflow convergence is binary (converged or exception) with no partial-failure diagnostics beyond iteration count.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Default iteration caps | `DEFAULT_MAX_ITERATIONS = 40` (function loop), `DEFAULT_MAX_ITERATIONS = 10` (agent re-invocation), `DEFAULT_JUDGE_MAX_ITERATIONS = 5`, `DEFAULT_MAX_ITERATIONS = 100` (workflows) | `python/packages/core/agent_framework/_tools.py:90` ; `python/packages/core/agent_framework/_harness/_loop.py:117` ; `python/packages/core/agent_framework/_harness/_loop.py:122` ; `python/packages/core/agent_framework/_workflows/_const.py:4` |
| Per-tool execution budget | `max_invocations` and `max_invocation_exceptions` on `FunctionTool` raise `ToolException` once exceeded | `python/packages/core/agent_framework/_tools.py:308-309` ; `python/packages/core/agent_framework/_tools.py:520-531` |
| Function-loop config schema | `FunctionInvocationConfiguration` TypedDict with `max_iterations`, `max_function_calls`, `max_consecutive_errors_per_request`, `terminate_on_unknown_calls`; validator rejects `< 1` | `python/packages/core/agent_framework/_tools.py:1344-1421` |
| Non-streaming loop driver | Bounded `for attempt_idx in range(attempt_start, max_iterations if loop_enabled else 0)`. After loop exhausts, forces a final model call with `tool_choice="none"` so the model produces a plain text answer instead of leaving orphaned `function_call` items. | `python/packages/core/agent_framework/_tools.py:2567-2696` |
| Streaming loop driver | Same bound; on loop exhaust logs `"Maximum iterations reached"` and runs a final streaming inner call with `tool_choice="none"`. | `python/packages/core/agent_framework/_tools.py:2717-2858` |
| `max_function_calls` best-effort behavior | When the cumulative tool budget is hit, `tool_choice="none"` is set; the docstring notes the limit is **post-batch** so a single batch of parallel calls can overshoot. | `python/packages/core/agent_framework/_tools.py:2586-2592` ; `python/packages/core/agent_framework/_tools.py:2636-2644` ; `python/packages/core/agent_framework/_tools.py:1354-1363` |
| Consecutive-error stop | `errors_in_a_row >= max_errors` (default 3) flips `_handle_function_call_results` action to `"stop"`; on the next iteration `tool_choice="none"` is forced. | `python/packages/core/agent_framework/_tools.py:91` ; `python/packages/core/agent_framework/_tools.py:2248-2270` ; `python/packages/core/agent_framework/_tools.py:2316-2322` |
| Middleware-driven termination | `MiddlewareTermination` exception (control-flow) used to short-circuit; loops in the pipeline catch it and abort the function-calling loop. | `python/packages/core/agent_framework/_middleware.py:72-77` ; `python/packages/core/agent_framework/_middleware.py:997` ; `python/packages/core/agent_framework/_middleware.py:1221` ; `python/packages/core/agent_framework/_middleware.py:1407` |
| `tool_choice='required'` runaway guard | After one iteration under `tool_choice="required"`, reset to default to break the "always call a tool" loop. | `python/packages/core/agent_framework/_tools.py:2647-2652` ; `python/packages/core/agent_framework/_tools.py:2819-2823` |
| Unknown-call handling | `terminate_on_unknown_calls` raises when the model names a tool that is not in the tool map. | `python/packages/core/agent_framework/_tools.py:1719` ; `python/packages/core/agent_framework/_tools.py:1366` |
| Workflow superstep cap | `Runner.__init__(max_iterations=100)`; `while self._iteration < self._max_iterations`; raises `WorkflowConvergenceException` if messages still remain after the cap. | `python/packages/core/agent_framework/_workflows/_runner.py:41` ; `python/packages/core/agent_framework/_workflows/_runner.py:64` ; `python/packages/core/agent_framework/_workflows/_runner.py:99-150` ; `python/packages/core/agent_framework/_workflows/_runner.py:152-153` |
| Convergence exception type | `WorkflowConvergenceException(WorkflowRunnerException)` raised with the configured `max_iterations` in the message. | `python/packages/core/agent_framework/exceptions.py:251-254` ; `python/packages/core/agent_framework/_workflows/_runner.py:153` |
| Workflow build-time cap | `WorkflowBuilder(max_iterations=DEFAULT_MAX_ITERATIONS)` is wired through to `Workflow.__init__` and to `Runner(max_iterations=...)`. | `python/packages/core/agent_framework/_workflows/_workflow_builder.py:91-104` ; `python/packages/core/agent_framework/_workflows/_workflow_builder.py:151` ; `python/packages/core/agent_framework/_workflows/_workflow_builder.py:871` ; `python/packages/core/agent_framework/_workflows/_workflow.py:285-357` |
| Workflow serialization of cap | `to_dict()` records `max_iterations` so checkpoints can be cross-checked. | `python/packages/core/agent_framework/_workflows/_workflow.py:395` |
| Workflow self-loop / dead-end warnings | `_validate_self_loops` and `_validate_dead_ends` log at build time, not at run time. Self-loop is a warning (`logger.warning` with the message "may cause infinite recursion if not properly handled with conditions"). | `python/packages/core/agent_framework/_workflows/_validation.py:161-162` ; `python/packages/core/agent_framework/_workflows/_validation.py:401-413` ; `python/packages/core/agent_framework/_workflows/_validation.py:415-428` |
| Agent loop middleware cap | `AgentLoopMiddleware(max_iterations: int \| None = DEFAULT_MAX_ITERATIONS)`; constructor validates `>= 1` or `None`. | `python/packages/core/agent_framework/_harness/_loop.py:117` ; `python/packages/core/agent_framework/_harness/_loop.py:264` ; `python/packages/core/agent_framework/_harness/_loop.py:332-335` |
| Agent loop judge factory | `with_judge(...)` defaults to `DEFAULT_JUDGE_MAX_ITERATIONS=5` (judge loops are more expensive and probabilistic). | `python/packages/core/agent_framework/_harness/_loop.py:122` ; `python/packages/core/agent_framework/_harness/_loop.py:344-396` |
| Agent loop stop precedence | `_evaluate_stop`: cap is checked **before** `should_continue`, so the predicate is not called once the cap fires; the cap's stop returns `(True, None)` so no spurious feedback is injected. | `python/packages/core/agent_framework/_harness/_loop.py:653-664` |
| Agent loop continuation predicates | `todos_remaining(provider)` and `background_tasks_running(provider)` return predicates that the agent loop can drive; the latter docstring explicitly tells callers to "pair it with `max_iterations` so the loop is guaranteed to stop." | `python/packages/core/agent_framework/_harness/_loop.py:751-796` |
| Finish-reason vocabulary | `FinishReasonLiteral = Literal["stop", "length", "tool_calls", "content_filter"]` carried on `ChatResponse`, `AgentResponse`, `AgentResponseUpdate`. | `python/packages/core/agent_framework/_types.py:1648-1672` ; `python/packages/core/agent_framework/_types.py:2234` ; `python/packages/core/agent_framework/_types.py:2516` ; `python/packages/core/agent_framework/_types.py:2634` ; `python/packages/core/agent_framework/_types.py:2892` |
| Workflow run state surface | `WorkflowRunState` enum (`STARTED`, `IN_PROGRESS`, `IN_PROGRESS_PENDING_REQUESTS`, `IDLE`, `IDLE_WITH_PENDING_REQUESTS`, `FAILED`, `CANCELLED`) emitted as `WorkflowEvent.status(state)` so consumers can tell IDLE-with-pending-requests from true completion. | `python/packages/core/agent_framework/_workflows/_events.py:58-67` ; `python/packages/core/agent_framework/_workflows/_workflow.py:589-603` |
| Workflow failure surface | `WorkflowEvent.failed(details)` with `WorkflowErrorDetails(error_type, message, traceback, executor_id, extra)` is emitted before the exception propagates. | `python/packages/core/agent_framework/_workflows/_events.py:70-99` ; `python/packages/core/agent_framework/_workflows/_events.py:261-267` ; `python/packages/core/agent_framework/_workflows/_workflow.py:606-619` |
| Telemetry of finish reason | `OtelAttr.FINISH_REASONS` is recorded per response so exhaustion vs. stop is observable in traces. | `python/packages/core/agent_framework/observability.py:199` ; `python/packages/core/agent_framework/observability.py:2368-2389` ; `python/packages/core/agent_framework/observability.py:2484-2490` |
| Test: agent loop stops at cap | `test_loop_stops_at_max_iterations` sets `max_iterations=3`, asserts `client.call_count == 3`. | `python/packages/core/tests/core/test_harness_loop.py:212-219` |
| Test: workflow convergence failure | `test_runner_run_until_convergence_not_completed` builds a cycle and asserts `WorkflowConvergenceException` with `"Runner did not converge after 5 iterations."`. | `python/packages/core/tests/workflow/test_runner.py:120-151` |
| Test: self-loop warning | `test_self_loop_detection_warning` asserts the warning text "may cause infinite recursion" is logged. | `python/packages/core/tests/workflow/test_validation.py:328-338` |
| Test: max-iterations exhausted telemetry | `test_agent_invoke_span_aggregates_usage_on_max_iterations_exhaustion` confirms the aggregated usage on exhaustion is exported. | `python/packages/core/tests/core/test_observability.py:4262-4271` |

## Answers to Dimension Questions

1. **What stops the loop?**
   - **Function-calling loop** (`_tools.py:2567`): stops on (a) the model producing no `function_call` content, (b) a `function_approval_request` content, (c) `max_iterations` cap, (d) `max_function_calls` budget, (e) `max_consecutive_errors_per_request`, (f) a middleware raising `MiddlewareTermination`, (g) `terminate_on_unknown_calls` flag.
   - **Workflow superstep loop** (`_runner.py:99-153`): stops on (a) `not has_messages()` (true convergence), or (b) `max_iterations` cap (raises `WorkflowConvergenceException`).
   - **Agent re-invocation loop** (`_harness/_loop.py:653-664`): stops on (a) `should_continue` predicate returning false, (b) `max_iterations` cap (cap is checked before the predicate to avoid wasted work), (c) `MiddlewareTermination` raised downstream.

2. **Are limits configurable?**
   - Yes. `FunctionInvocationConfiguration["max_iterations"]`, `["max_function_calls"]`, `["max_consecutive_errors_per_request"]`, `["terminate_on_unknown_calls"]` (`_tools.py:1392-1398`).
   - `WorkflowBuilder(max_iterations=…)` (`_workflow_builder.py:91`) wired to `Workflow` and `Runner`.
   - `AgentLoopMiddleware(max_iterations=…)` (`_harness/_loop.py:264`); `None` is allowed for an explicit unbounded loop.
   - `FunctionTool(max_invocations=…, max_invocation_exceptions=…)` (`_tools.py:308-309`).
   - MCP `sampling_max_tokens` and `sampling_max_requests` (`_mcp.py:110-111`, `_mcp.py:366-367`).
   - Validation in `normalize_function_invocation_configuration` (`_tools.py:1415-1420`) rejects `max_iterations < 1` and `max_function_calls < 1`.
   - `AgentLoopMiddleware` rejects `max_iterations < 1` (`_harness/_loop.py:332-333`).

3. **Is exhaustion treated differently from success?**
   - **Function-calling loop**: exhaustion of `max_iterations` triggers a final model call with `tool_choice="none"` (`_tools.py:2664-2696`) — the loop returns a normal `ChatResponse`, **not** an exception. So the caller cannot distinguish "the model gave up" from "the model answered" unless they read the `finish_reason` and count `function_call_count`.
   - **Workflow loop**: exhaustion raises `WorkflowConvergenceException` (`_runner.py:153`) and the workflow emits `WorkflowEvent.failed(details)` (`_workflow.py:606-615`) plus sets `WorkflowRunState.FAILED` (`_workflow.py:616-619`). Clearly distinguished.
   - **Agent loop**: cap short-circuits the predicate and returns whatever `last_result` the final iteration produced (`_harness/_loop.py:661-664`). The aggregated response carries the final iteration's `finish_reason` and the `iteration` number is not surfaced. **No exhaustion marker on the response** — caller must inspect telemetry or log.

4. **Are stuck loops detected before the hard limit?**
   - Only the `tool_choice="required"` reset (`_tools.py:2647-2652`, `_tools.py:2819-2823`) is a targeted anti-stuck mechanism, and it only fires when the caller explicitly sets `tool_choice="required"`. The `max_consecutive_errors_per_request` is a soft stuck-loop detector but only for errors, not for repeated-but-successful identical calls.
   - **No generic repetition detector** (no last-N-response hash, no cosine similarity, no progress signal) in any of the three loops. The hard cap is the only safety net.
   - Validation warns on workflow self-loops at build time (`_validation.py:401-413`) but cannot detect runtime cycles that depend on data.

5. **Does the user get a useful final state?**
   - **Function-calling loop**: yes — a normal `ChatResponse` is returned, usage is aggregated (`_tools.py:2628-2630`), and on exhaustion the model is forced to produce a text answer so the user sees something readable. The OTel exporter records `FINISH_REASONS` (`observability.py:2484-2490`).
   - **Workflow loop**: yes — `WorkflowRunState` and `WorkflowErrorDetails` distinguish IDLE, IDLE_WITH_PENDING_REQUESTS, FAILED. The exception message includes the configured cap (`_runner.py:153`).
   - **Agent loop**: partial — `AgentResponse` carries the final iteration's messages and aggregated usage (`_harness/_loop.py:692-702`); but the loop does not stamp an exhaustion reason on the response, so the user has to look at logs (`logger.info("Maximum function calls reached ...")` at `_tools.py:2587-2591`, `_tools.py:2639-2643`, `_tools.py:2734-2738`, `_tools.py:2811-2815`).

## Architectural Decisions

- **Three independent loops, three separate caps.** The framework does not unify "max_iterations" across function-calling, workflow supersteps, and agent re-invocation. A complex agent could be bounded by all three at once (e.g., `5 × 40 × 100` worst-case if every layer is hit). The user must reason about each independently.
- **Configurable over hard-coded.** All three loops default to a sensible cap (40 / 100 / 10) but every cap is overridable, including `None` for the agent loop. Validation in `normalize_function_invocation_configuration` (`_tools.py:1401-1421`) and `AgentLoopMiddleware.__init__` (`_harness/_loop.py:332-333`) prevents invalid configs.
- **Loop exhaustion is a normal return, not an exception, in the function-calling loop.** This is a deliberate design choice: after `max_iterations`, the loop makes a final non-tool call so the model is forced to summarize. This trades off observability (no exception) for usability (the caller always gets a `ChatResponse`).
- **Workflow convergence is an exception.** Distinct from the function-calling loop, the workflow runner raises `WorkflowConvergenceException` so the framework can emit `WorkflowEvent.failed` and set `WorkflowRunState.FAILED`.
- **`max_iterations` is checked before the predicate in the agent loop.** `_harness/_loop.py:661-663` explicitly states the cap "short-circuits before `should_continue` is evaluated (so an expensive predicate/judge is not called once the cap has fired)." This is a clean precedence rule.
- **`tool_choice="required"` is a known loop accelerator** and the framework actively defends against it by resetting the field after one iteration (`_tools.py:2647-2652`).
- **Cap-on-budget is best-effort post-batch.** `_tools.py:1354-1363` documents that a single parallel batch can overshoot the cap. The implementation always lets the in-flight batch finish before stopping.

## Notable Patterns

- **Hard-cap + predicate / budget layering** — every loop combines a hard counter with a user-supplied decision (`should_continue`, the workflow's `has_messages`, the function loop's `function_call` content). The cap is the safety net; the predicate is the policy.
- **Stop precedence, declared in the source** — `AgentLoopMiddleware._evaluate_stop` (`_harness/_loop.py:653-664`) reads the cap first; the function loop checks `terminate_on_unknown_calls` first (`_tools.py:1719`) before the function runs; the workflow's `_run_iteration` always commits state at the superstep boundary (`_runner.py:141`).
- **Final non-tool call after exhaustion** — `_tools.py:2664-2696` and `_tools.py:2835-2858` issue one more model call with `tool_choice="none"`, ensuring the user always gets a textual answer rather than orphaned function-call items.
- **Cap is configurable on the tool, not just the loop** — `FunctionTool.max_invocations` (`_tools.py:308-309`) and `max_invocation_exceptions` (`_tools.py:309`) are per-tool caps that raise `ToolException` mid-loop, so a single misbehaving tool cannot exhaust the whole call budget.
- **Telemetry for diagnostics** — `OtelAttr.FINISH_REASONS` is exported per response (`observability.py:2484-2490`) so the difference between "model said stop" and "loop said stop" is observable in traces.
- **Status event timeline** — `_workflow.py:582-603` emits `WorkflowEvent.status` for each transition so consumers can reconstruct the run lifecycle.

## Tradeoffs

- **No stuck-loop detector** — three independent hard caps but no last-N-response comparison, no progress metric, no hash-based repetition check. A model that returns an identical, technically-successful response on every iteration burns the whole cap. The `AgentLoopMiddleware` is the only layer that gives the user a hook to detect this (`should_continue`); the function-calling loop does not.
- **Cap and finish-reason are orthogonal** — a `finish_reason="length"` (token limit) still costs an iteration. The framework records it but does not use it as a control signal.
- **Best-effort function-call budget** — `max_function_calls` is checked after the in-flight parallel batch (`_tools.py:1354-1363`, `_tools.py:2636-2644`). A model that issues 20 parallel calls when the cap is 10 still executes all 20. This is documented but surprising; the alternative (pre-flight check) would require dropping tool calls mid-batch.
- **Function-loop exhaustion is silent on the response** — the final `ChatResponse` does not carry an exhaustion marker, only a `logger.info` line. The user must read the log or count usage to know the loop hit the cap.
- **Workflow self-loop detection is a warning, not an error** — `_validation.py:401-413` calls `logger.warning` for self-loops, so a graph that cannot converge still builds. The runner then fails at runtime via `WorkflowConvergenceException`. This is correct behavior for a library but means a user can deploy an unbounded workflow.
- **Three loops, three defaults, three semantics** — the same word ("max_iterations") means "LLM roundtrips" in `_tools.py`, "agent re-invocations" in `_harness/_loop.py`, and "Pregel supersteps" in `_workflows/_runner.py`. A user picking `max_iterations=10` in three different places gets three different safety envelopes.

## Failure Modes / Edge Cases

- **All parallel tool calls finish before the cap is checked** — `_tools.py:1354-1363`, `_tools.py:2636-2644`. A model that fires 50 parallel calls in one batch will execute all 50 even if the budget is 10.
- **Identical-successful response loop** — a model that emits the same `function_call` payload every iteration (and the function returns the same value) will run until `max_iterations` (`_tools.py:2567`) or `max_invocations` (`_tools.py:520-523`) trips. There is no progress detector.
- **`tool_choice="required"` not handled when set mid-loop** — the reset only fires after one iteration (`_tools.py:2647-2652`); a model that never stops calling tools will still drain iterations until the cap.
- **Workflow with a data-dependent cycle** — `_validation.py:401-413` warns at build time but a cycle whose only exit is data-driven (`executor_a -> executor_b -> executor_a` that converges on the value, not the structure) will still build. If the exit never fires, the runner raises `WorkflowConvergenceException` at the cap (`_runner.py:153`). The exception surfaces `WorkflowErrorDetails` (`_events.py:70-99`) so the failure is observable.
- **Conversation-id-aware clients** — when `response.conversation_id` is set, `prepped_messages` is cleared and the service holds the transcript (`_tools.py:2613-2614`, `_tools.py:2654-2661`). The cap still applies, but a model that never converges against a server-side transcript will exhaust `max_iterations` server-side; the framework cannot detect this from `finish_reason` alone.
- **Loop-disabled branch** — when `enabled=False`, the for-loop iterates zero times (`_tools.py:2567`, `_tools.py:2717`) and the request falls through to `super_get_response`. This is the safe path, but it means a user who disables function invocation cannot rely on cap-based protection.
- **Agent loop with no `should_continue`** — `_harness/_loop.py:336` requires the predicate; there is no default, so the only safety is the cap. A user who sets `max_iterations=None` and never returns False gets an unbounded loop. The docstring (`_harness/_loop.py:255-258`) explicitly warns about this.
- **Streaming `MiddlewareTermination` after `process()` returns** — `_harness/_loop.py:547-552` handles the case where a downstream middleware raises after the loop is already streaming, but the comment notes this requires explicit handling. A user middleware that raises a non-`MiddlewareTermination` exception will surface as a generic stream error, not a clean loop stop.
- **Asyncio event-loop assumption** — `Workflow.status` docstring (`_workflow.py:368-377`) calls out that the status is read "on a single asyncio event loop" and that the assignment is "atomic under the CPython GIL, so no locking is required." Multi-loop or threaded workflows are not safe.

## Future Considerations

- **Stuck-loop detector.** Add a lightweight last-N-response hash or progress metric so identical, technically-successful iterations don't burn the full cap. The agent loop already exposes the right hook (`_harness/_loop.py:653-664`); the function loop has no such hook.
- **Exhaustion marker on `ChatResponse` / `AgentResponse`.** When `max_iterations` is hit, stamp an `additional_properties` flag (e.g., `"loop_exhausted": "max_iterations"`) so the user can distinguish "model said stop" from "loop said stop" without reading logs.
- **Unify "max_iterations" semantics.** The same name means three different things across `_tools.py`, `_harness/_loop.py`, and `_workflows/_runner.py`. Either rename or document the layering explicitly (e.g., "Total bound = `function_loop_iterations × harness_iterations × workflow_iterations`").
- **Pre-flight budget check.** `_tools.py:2636-2644` accepts that one parallel batch can overshoot. A pre-flight decision (e.g., truncate the parallel set to the remaining budget) would make `max_function_calls` exact, at the cost of model confusion.
- **`finish_reason="length"` as a soft signal.** The framework records it (`observability.py:2368-2389`) but does not act on it. A configurable "truncate on length and force a summary call" would close the loop on a common failure mode.
- **Cycle detection at build time.** `_validation.py:401-413` only flags self-loops. A full graph analysis (Tarjan's SCC or simple DFS) could warn on cyclic subgraphs whose only exit is data-dependent, before the workflow runs.
- **Cancellation surface.** `WorkflowRunState.CANCELLED` is defined (`_events.py:67`) but the runner is not cancellation-aware at the iteration level — `asyncio.CancelledError` is propagated (`_runner.py:115-120`) but the workflow does not emit a CANCELLED status. The C# sibling likely has a richer cancellation model; a Python equivalent would be useful.
- **Cross-loop budget.** A single user-facing budget that decomposes into the three internal caps (function / harness / workflow) would prevent a user from being surprised that they configured three seemingly identical knobs that mean different things.

## Questions / Gaps

- **What is the wall-clock budget?** None of the three loops exposes a timeout / `asyncio.wait_for` for the iteration itself. The `MCP` long-running task path has a `max_task_wait` (`_mcp.py:84-86`) but the agent / workflow / function loops have no wall-clock cap. A user that wants "stop after 30 seconds" has to layer `asyncio.wait_for` themselves.
- **Are finish reasons exposed to the agent loop's `should_continue`?** `_harness/_loop.py:226-239` documents that `last_result` is passed in, and `AgentResponse.finish_reason` is populated (`_harness/_loop.py:697`), so a predicate can read it. But there is no built-in helper (analogous to `todos_remaining` or `background_tasks_running`) for "stop when `finish_reason` is `stop`". The user has to write it.
- **Does the `tool_choice="required"` reset hold across re-invocations of the agent loop?** `_tools.py:2647-2652` resets per function-loop iteration, but inside an `AgentLoopMiddleware` the function loop runs once per harness iteration. The reset is correct within one function loop but the harness does not need to know about it. No evidence found of an interaction.
- **Does `max_iterations` count the final non-tool call?** `_tools.py:2567` shows the loop is `for attempt_idx in range(attempt_start, max_iterations if loop_enabled else 0)`, and `_tools.py:2664-2696` is the post-loop final call. The final call is **outside** the loop, so a `max_iterations=N` config allows `N` roundtrips plus 1 final summary call. This is documented in spirit but not explicitly called out in the docstring.
- **Is `terminate_on_unknown_calls` honored in the streaming path?** `_tools.py:1719` is in `_try_execute_function_calls`; the streaming branch (`_tools.py:2700+`) calls the same shared functions, so yes — but a test asserting this end-to-end was not located in the search.
- **Does the workflow runner check `max_iterations` against the iteration count at superstep entry or exit?** `_runner.py:99` checks at entry (`while self._iteration < self._max_iterations`); the post-loop check at `_runner.py:152-153` is only for the "did not converge" message. So a workflow that exactly hits the cap completes without exception; a workflow that exceeds it by even one message raises. The semantic is "less than cap = OK, equal to cap + still has messages = raise". This may surprise users who expect "equal to cap = raise".
- **No evidence found** of a built-in "max agent run wall-clock" timeout on the agent loop, on the workflow loop, or on the function-calling loop. The MCP long-running task is the only place a wall-clock cap is enforced.
- **No evidence found** of repetition detection in any of the three loops. The only progress signal is the iteration counter and the `max_consecutive_errors_per_request` budget.

---

Generated by `studies/agent-harness-study/sources/agent-framework` against `agent-framework` (Python core package).

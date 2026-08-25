# Source Analysis: agent-framework

## 23.02 Persistence vs Escalation Philosophy

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework (Microsoft Agent Framework) |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (`python/packages/core`, `python/packages/orchestrations`) and .NET (`dotnet/src`); `go/` contains only a README, no code |
| Analyzed | 2026-08-24 |

> **Path convention:** all citations below are relative to the source root `studies/agent-harness-study/sources/agent-framework/`. Example: `python/packages/core/agent_framework/_tools.py:95` resolves to `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_tools.py:95`.

## Summary

Agent Framework implements persistence as a **layered, bounded-by-default** system rather than a single retry engine. At the innermost layer, the function-invocation loop keeps calling model+tools up to a hard iteration cap (`DEFAULT_MAX_ITERATIONS = 40`, `python/packages/core/agent_framework/_tools.py:95`) and a consecutive-tool-error cap (`DEFAULT_MAX_CONSECUTIVE_ERRORS_PER_REQUEST = 3`, `_tools.py:96`); when a cap fires it **degrades gracefully** — tools are disabled and the model is forced to produce a final text answer instead of raising. Above that, the harness-level `AgentLoopMiddleware` re-runs the whole agent while a caller-supplied `should_continue` predicate holds, bounded by its own safety cap (default 10; judge mode default 5) with an explicit opt-in to unbounded looping (`python/packages/core/agent_framework/_harness/_loop.py:122-127,260-262`). Multi-agent orchestration adds genuine **replanning**: Magentic detects stalls via an LLM-produced progress ledger, then resets context, broadcasts a reset signal, replans, and emits a typed `REPLANNED` event (`python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:1117-1125,1156-1192`). **Escalation to humans** is a first-class path in three forms: session-backed tool-approval requests that pause the run and return control to the caller (`_harness/_tool_approval.py:343-418`), workflow `request_info` events with checkpoint-based pause/resume across process restarts (`_workflows/_workflow_context.py:403-434`; `_workflows/_workflow.py:271`), and Magentic plan sign-off review requests (`_magentic.py:956-958,1040-1055`). By contrast, the workflow graph engine itself is **fail-fast**: an executor exception emits a structured `executor_failed`/`failed` event and terminates the run with no built-in retry policy (`_workflows/_executor.py:311`; `_workflows/_workflow.py:606-622`). Persistence decisions are logged at every stop point via Python logging and are partially observable through OTel spans that aggregate usage on exhaustion.

## Rating

**8 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale: every loop has a configurable bound with validation (`normalize_function_invocation_configuration` rejects bad caps at `_tools.py:1403-1408`); unbounded operation requires an explicit `None` opt-in (`_loop.py:260-262`); escalation paths (tool approval, plan review, request_info) are typed control-plane mechanisms, not conventions; exhaustion paths have dedicated tests including orphaned-call prevention (`tests/core/test_function_invocation_logic.py:1574-1637`) and telemetry aggregation (`tests/core/test_observability.py:5735-5780`); and the model is mirrored cross-language (.NET `LoopAgent.DefaultMaxIterations = 10` with the same approval escape hatch, `dotnet/src/Microsoft.Agents.AI/Harness/Loop/LoopAgent.cs:68,196,464`). It stops short of 9-10 because there is no generic transient-failure retry policy for tool execution or workflow executors (retries exist only ad hoc inside Magentic ledger parsing), stop reasons are recorded as log strings rather than structured telemetry attributes, and the judge fallback is deliberately biased to keep looping on ambiguous verdicts, which can burn budget.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Function-loop persistence cap | `DEFAULT_MAX_ITERATIONS: Final[int] = 40`; read into loop at `_get_response_with_function_invocation` | `python/packages/core/agent_framework/_tools.py:95`, `_tools.py:3199` |
| Graceful degradation on cap exhaustion | Phase 3 sets `tool_choice="none"` and requests one final response without tools; logs `"Maximum iterations reached (%d). Requesting final response without tools."` | `python/packages/core/agent_framework/_tools.py:3307-3325` |
| Consecutive tool-error stop | Counter resets on success, warning log when limit reached; configured per request | `python/packages/core/agent_framework/_tools.py:96,2718-2733,3614-3615` |
| Function-call budget (configurable) | `max_function_calls` documented as "primary knob for controlling cost"; best-effort semantics documented; enforced via `_disable_tools_at_function_call_limit` | `python/packages/core/agent_framework/_tools.py:1342-1351,2736-2748` |
| Budget survives approval pauses | Budget state persisted under `_FUNCTION_INVOCATION_BUDGET_STATE_KEY`; "executions that pause for user input still consume max_function_calls" | `python/packages/core/agent_framework/_tools.py:101,3193,3225-3229`; core AGENTS.md |
| Cap config normalization/validation | Defaults block + `ValueError` guards for non-positive caps | `python/packages/core/agent_framework/_tools.py:1389-1409` |
| Harness loop persistence | `AgentLoopMiddleware` loops on `should_continue`; `max_iterations` short-circuits before predicate evaluation | `python/packages/core/agent_framework/_harness/_loop.py:217-263,747-758` |
| Unbounded opt-in guard | Docstring warns unbounded loops "relies entirely on `should_continue`"; constructor raises if cap < 1 | `python/packages/core/agent_framework/_harness/_loop.py:260-262,337-338` |
| Judge-driven continue/stop | `.with_judge`: second LLM emits `JudgeVerdict`; loop continues while unanswered; reasoning fed back as next input (replan-by-feedback) | `python/packages/core/agent_framework/_harness/_loop.py:153-213,349-415` |
| HITL escape hatch in loops | `_has_pending_approval_request` stops loop and returns approval request to caller *before* predicates/caps | `python/packages/core/agent_framework/_harness/_loop.py:442-459,541-544,645-648` |
| Todo/background-task loop helpers | `todos_remaining(looping_modes=...)` gates looping by operating mode; `background_tasks_running()` pairs with cap | `python/packages/core/agent_framework/_harness/_loop.py:866-884,925-958` |
| Harness wiring of loop + approval | `create_harness_agent(loop_should_continue=..., loop_max_iterations=...)`; loop prepended outermost ahead of `ToolApprovalMiddleware` | `python/packages/core/agent_framework/_harness/_agent.py:302,339,636-654` |
| Tool-approval escalation | Session-backed queue; pending requests returned to caller as `function_approval_request`; resume injects collected responses | `python/packages/core/agent_framework/_harness/_tool_approval.py:343-418` |
| Auto-approval configurability | `auto_approval_rules` callbacks + security warning about name-collision bypass | `python/packages/core/agent_framework/_harness/_tool_approval.py:351-379` |
| Workflow convergence guard | Runner loop bounded by `max_iterations` (default 100); raises `WorkflowConvergenceException("Runner did not converge after N iterations.")` | `python/packages/core/agent_framework/_workflows/_runner.py:56,121,176-177`; `_const.py:4` |
| Builder-configurable convergence | `WorkflowBuilder(max_iterations=...)` forwarded into the workflow | `python/packages/core/agent_framework/_workflows/_workflow_builder.py:93,154` |
| Workflow pause/resume (HITL) | Executors call `ctx.request_info(...)` → `request_info` events; host responds via `send_responses`; checkpoint storage enables pause/resume across restarts | `python/packages/core/agent_framework/_workflows/_workflow_context.py:403-434`; `_workflow.py:250-253,271,143-149` |
| Workflow fail-fast on executor error | `executor_failed` event with `WorkflowErrorDetails` (type/message/traceback/executor_id) | `python/packages/core/agent_framework/_workflows/_executor.py:311`; `_events.py:70-99,124` |
| Workflow failure surfacing + OTel | Status → `FAILED`, `failed` event, span exception capture with `error.type`/`error.message` attributes | `python/packages/core/agent_framework/_workflows/_workflow.py:606-622` |
| Sub-workflow error propagation | Parent converts sub-workflow `FAILED` into an `error` event naming the sub-workflow | `python/packages/core/agent_framework/_workflows/_workflow_executor.py:559-573` |
| Magentic stall detection | Progress-ledger answers increment/decrement `stall_count`; exceeding `max_stall_count` triggers `_reset_and_replan` | `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:1116-1125` |
| Replan machinery | Reset clears chat history, increments `reset_count`; broadcast `MagenticResetSignal`; manager `replan()` updates facts+plan; typed `REPLANNED` event emitted | `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:386-394,775-783,790,1156-1192` |
| Persistence knobs (orchestration) | Manager ctor: `max_stall_count=3`, `max_reset_count`, `max_round_count`, `progress_ledger_retry_count=3` | `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:469-481,541-590` |
| Bounded retry with backoff (only place) | Progress-ledger JSON parse retried up to `progress_ledger_retry_count` with linear backoff, warning per attempt, terminal `RuntimeError` | `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:719-739` |
| Ledger failure escalates to replan | Exception in `create_progress_ledger` caught → warning logged → `_reset_and_replan` | `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:1087-1092` |
| Termination message on limits | Round/reset limit exceeded → `logger.error`, assistant termination message yielded, workflow marked terminated | `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:1245-1263` |
| Human plan sign-off | `require_plan_signoff` → `MagenticPlanReviewRequest` via `ctx.request_info`; human approves or requests revisions (triggers `REPLANNED`) | `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:956-958,994-1055` |
| Group-chat round limit | `logger.warning("%s reached max_rounds=%s; forcing completion.")` then forced completion message | `python/packages/orchestrations/agent_framework_orchestrations/_base_group_chat_orchestrator.py:507-538` |
| Autonomy levels (handoff) | `with_autonomous_mode(agents, prompts, turn_limits)` — per-agent autonomous turn limits before returning control to user | `python/packages/orchestrations/agent_framework_orchestrations/_handoff.py:819-848` |
| Autonomy levels (modes) | `AgentModeProvider` default map: `plan` (interactive, ask clarifications, get approval) vs `execute` ("Work autonomously... keep going") | `python/packages/core/agent_framework/_harness/_mode.py:15-71` |
| Observability of exhaustion | OTel test: invoke span aggregates usage across in-loop calls + final exhaustion call | `python/packages/core/tests/core/test_observability.py:5735-5780` |
| Tests for stop behavior | Loop stops at `max_iterations` (non-streaming & streaming); limit leaves no orphaned function calls; final `tool_choice=none` call asserted | `python/packages/core/tests/core/test_harness_loop.py:220-230,1250-1276`; `tests/core/test_function_invocation_logic.py:1527-1637` |
| .NET parity | `LoopAgent.DefaultMaxIterations = 10`, `HasPendingApprovalRequests` escape hatch, structured log `"LoopAgent reached the maximum of {MaxIterations} iterations and stopped."` | `dotnet/src/Microsoft.Agents.AI/Harness/Loop/LoopAgent.cs:68,196-204,464-484` |
| .NET harness knob | `HarnessAgentOptions.MaximumIterationsPerRequest` forwarded to `FunctionInvokingChatClient.MaximumIterationsPerRequest` | `dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgentOptions.cs:182-185`; `HarnessAgent.cs:267-268` |

## Answers to Dimension Questions

**1. Does the agent persist or escalate on failure?**
Both, chosen per layer. The innermost function-invocation loop persists until a cap, then degrades gracefully (final answer with tools disabled, `_tools.py:3307-3325`); consecutive tool errors similarly stop tool use rather than aborting the request (`_tools.py:2718-2733`). Consecutive errors do not trigger retries — there is no built-in transient-error backoff in this loop; the only true retry-with-backoff found is Magentic's progress-ledger parse retry (`_orchestrations/_magentic.py:719-739`). Multi-agent orchestration escalates *inward* first (reset + replan, `_magentic.py:1156-1192`) and *to humans* when configured (tool approval, plan sign-off, `request_info`). Workflow graphs escalate by failing fast with structured error details (`_workflow.py:606-622`).

**2. Is persistence configurable?**
Yes, extensively: `function_invocation_configuration["max_iterations"|"max_function_calls"|"max_consecutive_errors_per_request"]` with validation (`_tools.py:1389-1409`); `AgentLoopMiddleware(max_iterations=None|int)` incl. explicit unbounded mode (`_loop.py:269,291-294`); `WorkflowBuilder(max_iterations=...)` (`_workflow_builder.py:93`); Magentic `max_stall_count`/`max_reset_count`/`max_round_count`/`progress_ledger_retry_count` (`_magentic.py:469-481,541-590`); group chat `with_max_rounds` (`_group_chat.py:834-849`); handoff autonomous turn limits (`_handoff.py:819-848`); approval auto-rules (`_tool_approval.py:351-379`).

**3. Are escalation paths clear?**
Yes — three typed mechanisms: (a) tool approval: `function_approval_request` content queued in session state and returned to the caller, resumed by injecting responses (`_tool_approval.py:381-418`); (b) workflow `request_info` events with request-id correlation and host-side resume (`_workflow_context.py:403-434`; `_workflow.py:143-149,883-888`); (c) orchestration-specific reviews (`MagenticPlanReviewRequest/Response`, `_magentic.py:806-855`). Loops additionally treat a pending approval as a mandatory stop that preempts both caps and predicates (`_loop.py:442-459`).

**4. Are persistence decisions observable?**
Partially. Every stop decision logs: iteration-cap exhaustion (`_tools.py:3309-3312`, .NET equivalent `LoopAgent.cs:480-484`), function-call budget (`_tools.py:2743-2748`), consecutive-error limit (`_tools.py:2729-2732`), group-chat round limit (`_base_group_chat_orchestrator.py:511-515`), and Magentic limit termination (`_magentic.py:1253`). OTel spans aggregate usage across exhausted loops (`test_observability.py:5735-5780`), and workflows emit lifecycle events (`superstep_started/completed`, `executor_failed`, `failed`, `_events.py:118-125`). **Gap:** no dedicated span attribute or event type records *why* a loop stopped (cap vs predicate vs approval); the reason lives in log strings and injected messages only.

## Architectural Decisions

1. **Bounded-by-default persistence everywhere.** Each loop layer ships a conservative default cap (40 tool iterations, 10 agent-loop iterations, 5 judge iterations, 100 workflow supersteps) and requires an explicit `None` to go unbounded (`_tools.py:95`; `_loop.py:122-127`; `_const.py:4`). This makes runaway agents a deliberate misconfiguration rather than an accident.
2. **Graceful degradation over exception in chat loops.** Exhausting the tool loop forces a `tool_choice="none"` final answer and, if even that fails, substitutes fallback text `"Function invocation limit reached before a final answer could be produced."` (`_tools.py:102-104,3325`) — the caller gets a usable response plus evidence, not an exception.
3. **Fail-fast workflow engine.** Executor exceptions produce structured `WorkflowErrorDetails` and a terminal `FAILED` state; no framework-level retry or error-routing policy exists for executors (`_executor.py:311`; `_workflow.py:606-622`). Recovery strategies (retry/replan) are pushed up to orchestrators or hosts.
4. **Replanning confined to multi-agent orchestration.** Stall detection → reset → replan is implemented in Magentic only; single-agent loops offer replan-by-feedback instead (judge reasoning relayed as next input, `_loop.py:203-211`).
5. **Humans as typed control-plane participants.** Approval requests/responses and plan reviews are dedicated content types/events with id-correlation and session/checkpoint durability, not prompt hacks.
6. **Cross-language behavioral parity.** The Python `AgentLoopMiddleware` approval escape hatch explicitly mirrors the C# `LoopAgent.HasPendingApprovalRequests` (`_loop.py:447-451`; `LoopAgent.cs:196,464`), keeping persistence philosophy consistent across stacks.

## Notable Patterns

- **Stop-precedence chain:** approval-pending check → `max_iterations` short-circuit → `should_continue` predicate (`_loop.py:541-559,747-758`). Expensive predicates/judges are never called after the cap fires.
- **Budget accounting across pauses:** the function-call budget persists in session state so approval round-trips cannot be used to evade the cap (`_tools.py:101,3193`).
- **Stall counter with decay:** `stall_count += 1` on no-progress rounds and `- 1` on progress rounds, so isolated stalls don't trigger resets (`_magentic.py:1116-1120`).
- **Persistence-biased judge fallback:** for clients without structured output, an ambiguous judge reply keeps the loop running (`MORE` wins, `_loop.py:192-199`) — persistence is preferred over premature stop, compensated by a lower default cap (5).
- **Mode-gated autonomy:** `todos_remaining(looping_modes=["execute"])` lets the same agent loop autonomously in execute mode but stay interactive in plan mode (`_loop.py:925-958`; `_mode.py:32-71`).
- **Forced-completion pattern:** group chat and Magentic emit an explicit assistant termination message when limits hit (`_base_group_chat_orchestrator.py:520-538`; `_magentic.py:1251-1261`), making stop causes visible in-band to consumers.

## Tradeoffs

- **Best-effort budgets overshoot:** `max_function_calls` is checked after each parallel batch, so a batch larger than the remaining budget still executes fully (`_tools.py:1348-1351`) — cost predictability is traded for simplicity.
- **Judge persistence bias can waste budget:** ambiguity keeps looping by design; mitigated only by the smaller default cap.
- **No transient-failure retry in the main tool loop:** three consecutive tool errors permanently disable tools for that request (`_tools.py:2718-2733`); flaky-but-recoverable tools degrade the answer quality instead of being retried.
- **Fail-fast workflows shift burden to hosts:** applications wanting retries must build them atop executors/checkpoints themselves.
- **String-based observability for stop reasons:** easy to read in consoles, but not machine-queryable like the rest of the OTel surface.

## Failure Modes / Edge Cases

- **Unbounded loop misuse:** `max_iterations=None` relies entirely on the predicate terminating (`_loop.py:260-262`); an always-true predicate hangs the run — accepted, documented risk.
- **Ledger-parse storms:** repeated progress-ledger JSON failures each trigger full context reset + participant reset + replan; bounded by `max_reset_count` but expensive when hit (`_magentic.py:1087-1092,1246-1263`).
- **Approval middleware requires a session:** `ToolApprovalMiddleware.process` raises `RuntimeError` without `AgentSession` (`_tool_approval.py:384-385`).
- **Orphaned tool calls at cap:** the loop must not return dangling function calls once limits disable tools — covered by tests asserting cleanup and a metadata-only follow-up call (`test_function_invocation_logic.py:1574-1637`).
- **Auto-approval rule collisions:** a name-matching auto-approval rule can silently approve unrelated tools — called out as an explicit security warning (`_tool_approval.py:365-376`).
- **Terminated Magentic instances are not reusable:** further messages raise `RuntimeError` directing users to rebuild via the builder (`_magentic.py:921-925`).

## Future Considerations

- Expose a structured stop-reason (cap fired / predicate false / approval pending / error limit) on `AgentResponse` or as a span attribute, replacing log-string-only observability.
- Add an optional pluggable retry policy for tool execution (backoff/classifier), since today only the consecutive-error *stop* exists.
- Consider a failure-handling strategy hook for workflow executors (retry/reroute/dead-letter) analogous to the edge/group-chat knobs, to avoid host-side reimplementation.
- Extend the Magentic-style replan machinery as an optional middleware so single agents can opt into ledger-based stall detection.

## Questions / Gaps

- No generic "ask the human a free-form question mid-run" primitive was found outside tool-approval and workflow `request_info` plumbing; searching for `escalat*`, `ask_user`, and `human_input` across `python/packages/core` surfaced no dedicated API. If such a primitive exists outside core/orchestrations, it was outside this study's boundary.
- The `go/` directory contains only a README (no Go sources), so persistence behavior could not be assessed for a third stack.
- Provider packages beyond `core`/`orchestrations` (e.g., `foundry`, `anthropic`) were not audited individually; transport-level retries (e.g., SDK-internal HTTP retries) were treated as out of scope because they are owned by vendor SDKs, not this codebase.
- The .NET side was sampled only at parity points (`LoopAgent`, `ToolApprovalAgent`, `HarnessAgentOptions`), not exhaustively.

---

Generated by dimension `23.02 Persistence vs Escalation Philosophy` against `agent-framework`.

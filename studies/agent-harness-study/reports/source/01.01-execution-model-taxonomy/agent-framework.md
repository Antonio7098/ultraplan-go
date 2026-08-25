# Source Analysis: agent-framework

## 01.01 — Execution Model Taxonomy

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python 3.10+ (`agent_framework`, `agent_framework_durabletask`, `agent_framework_orchestrations`, …) and C#/.NET (`Microsoft.Agents.AI*`, `Microsoft.Agents.AI.Workflows`, `Microsoft.Agents.AI.Harness`, `Microsoft.Agents.AI.DurableTask`) — a single mono-repo with two first-class parallel implementations |
| Analyzed | 2026-07-02 |

## Summary

Microsoft Agent Framework (MAF) is a multi-language SDK that implements **three layered execution models**, and they coexist by deliberate design:

1. A **Pregel-style superstep graph executor** for `Workflow` (Python `agent_framework._workflows.Workflow` ⇒ `Runner.run_until_convergence`; .NET `Microsoft.Agents.AI.Workflows.Workflow` ⇒ `InProcessRunner.RunSuperStepAsync` via `ISuperStepRunner`). The model is named explicitly in module docstrings: `python/.../_workflows/_runner.py:31` ("a workflow in Pregel supersteps") and `python/.../_workflows/_workflow.py:206-223` ("a Pregel-like model, running in supersteps until the graph becomes idle").
2. A **middleware-driven step loop** for plain agent calls (`Agent.run` in Python; `AIAgent.RunAsync` in .NET). Python's `AgentMiddleware` pipeline wraps `call_next()` repeatedly; the function-calling iteration itself lives in `FunctionInvocationLayer.get_response` (`python/.../_tools.py:2565-2696`) as a `for attempt_idx in range(...)` over `max_iterations` (default 40 at `_tools.py:90`).
3. An **explicit experimental outer loop** — `AgentLoopMiddleware` in Python and `LoopAgent` in .NET — that *re-invokes the wrapped agent* (call_next) until a `should_continue` predicate or judge gives up, with `max_iterations=10` by default (`python/.../_harness/_loop.py:117`, `dotnet/.../Harness/Loop/LoopAgent.cs:67`).

On top of these, the .NET engine layers three *delivery shapes* (`Execution/ExecutionMode.cs:5-23`: `OffThread`, `Lockstep`, `Subworkflow`) that control how superstep events are streamed, and a **durable-task model** (`python/packages/durabletask/agent_framework_durabletask/_workflows/orchestrator.py:689-911` — `run_workflow_orchestrator`) re-encodes the same Pregel superstep graph as a generator-based, replayable durable orchestration.

The **one-sentence answer** to "what advances execution": in either stack, the **workflow runner advances by iterating a synchronous superstep that drains pending messages, delivers them concurrently to target executors per edge group, and terminates when `has_messages()` is false**; the **non-workflow agent** advances by **one chat-client round-trip per loop iteration** in the `FunctionInvocationLayer.get_response` step loop, with optional middlewares (`AgentLoopMiddleware` / `LoopAgent`) wrapping whole agent runs in an outer iteration.

## Rating

**Score: 8 / 10 — Clear, well-documented, observable model layered cleanly across both stacks; small deductions for distribution of looping responsibility and parallel implementations.**

Rationale:

- Both implementations name the model (`"Pregel supersteps"`) in user-facing docstrings and expose `superstep_started` / `SuperStepStartedEvent` as first-class types (`python/.../_workflows/_runner.py:31`, `python/.../_workflows/_events.py:162-163`).
- Hard guard against non-converging graphs: `DEFAULT_MAX_ITERATIONS = 100` with `WorkflowConvergenceException` (`python/.../_workflows/_const.py:4`, `python/.../_workflows/_runner.py:152-153`). Function-calling loop bounded at 40 (`python/.../_tools.py:90`), outer agent loop bounded at 10 (`python/.../_harness/_loop.py:117`). The .NET `LoopAgent` carries `DefaultMaxIterations = 10` (`dotnet/.../Harness/Loop/LoopAgent.cs:67`).
- Multiple orchestrators (`SequentialOrchestrator`, `ConcurrentOrchestrator`, `GroupChatOrchestrator`, `MagenticOrchestrator`, `HandoffOrchestrator`) compile to the same `Runner` engine rather than defining their own loops (`python/packages/orchestrations/agent_framework_orchestrations/__init__.py`).
- Convergence rule is explicit (`if not await self._ctx.has_messages(): break`, `python/.../_workflows/_runner.py:149-150`).
- Deducted for (a) the agent-level loop vocabulary being fragmented across at least three implementations — `FunctionInvocationLayer.get_response`, `AgentLoopMiddleware`, and the .NET `FunctionInvokingChatClient` + `LoopAgent` decorator — rather than one obvious primary; (b) the function-calling loop lives in the per-provider chat-client substrate (`FunctionInvocationLayer` / `FunctionInvokingChatClient`) rather than the agent core, which forces mental jumps when reasoning about a single "agent loop"; (c) two parallel language implementations without a shared engine.

## Evidence Collected

Every entry includes a workspace-relative file path with line numbers, format `path/to/file.ext:NN`.

### Python — Workflow engine (Pregel superstep)

| Area | Evidence | File:Line |
|------|----------|-----------|
| Model named | Module docstring: `"""A class to run a workflow in Pregel supersteps."""` | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_runner.py:31` |
| Main iteration | `Runner.run_until_convergence` is an `async def` returning `AsyncGenerator[WorkflowEvent, None]`; main loop increments `self._iteration` then yields `WorkflowEvent.superstep_started(...)` | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_runner.py:78-101` |
| Per-superstep delivery | `_run_iteration` drains messages grouped by source executor and concurrently delivers via `_deliver_messages` using `asyncio.gather(*tasks)` | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_runner.py:160-211` |
| Convergence rule | Termination condition: `if not await self._ctx.has_messages(): break` | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_runner.py:149-150` |
| Iteration cap | `DEFAULT_MAX_ITERATIONS = 100` | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_const.py:4` |
| Convergence exception | Raises `WorkflowConvergenceException` when `self._iteration >= self._max_iterations and await self._ctx.has_messages()` | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_runner.py:152-153` |
| Superstep events | `WorkflowEvent.superstep_started(iteration=...)` / `superstep_completed(...)`; exported in `__all__` | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_events.py:119-122`, `:162-163`, `:315-322` |
| Workflow description | `"""A graph-based execution engine … a Pregel-like model, running in supersteps until the graph becomes idle."""` | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_workflow.py:206-223` |
| Entry point — non-streaming | `Workflow.run(message=None, *, stream=False, …) -> Awaitable[WorkflowRunResult]` overload at `_workflow.py:687-699`; implementation calls `_run_workflow_with_tracing` and consumes `_runner.run_until_convergence()` | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_workflow.py:687-770` |
| Entry point — streaming | `Workflow.run(..., stream=True)` returning `ResponseStream[WorkflowEvent, WorkflowRunResult]` | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_workflow.py:674-685` |
| Per-superstep checkpoint | `_create_checkpoint_if_enabled(previous_checkpoint_id)` invoked after each superstep (and at superstep 0 for "checkpoint after start executor ran") | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_runner.py:96-97`, `:144` |
| Executor message handling | `Executor.execute(message, source_executor_ids, state, runner_context, …)` resolves the matching `@handler`, emits `executor_invoked`, then `executor_completed` events | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_executor.py:219-294` |
| Handler decorator | `@handler` / `@handler(input=…, output=…, workflow_output=…)` registers typed handlers on the executor | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_request_info_mixin.py:126-180`, `python/.../_executor.py:529-595` |
| Edge runner indirection | `EdgeRunner.send_message` abstract base; `create_edge_runner` factory builds per-edge-group runners (`DirectEdgeRunner`, `FanOutEdgeRunner`, `FanInEdgeRunner`, `SwitchCaseEdgeRunner`) | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_edge_runner.py:27-58` |
| Run state taxonomy | `WorkflowRunState = IDLE / IN_PROGRESS / IN_PROGRESS_PENDING_REQUESTS / IDLE_WITH_PENDING_REQUESTS / FAILED / CANCELLED` | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_workflow.py:286-316` (status emissions at `:531-603`) |
| Orchestrators compile to graph | `SequentialOrchestrator`, `ConcurrentOrchestrator`, `GroupChatOrchestrator`, `MagenticOrchestrator`, `HandoffOrchestrator` exposed via `agent_framework_orchestrations/__init__.py` | `sources/agent-framework/python/packages/orchestrations/agent_framework_orchestrations/__init__.py` (per core `AGENTS.md` summary) |
| Magentic — nested turn loop | `MagenticOrchestrator._run_outer_loop` calls `_run_inner_loop` and resets when progress stalls (the language describes "outer loop handles replanning", "inner loop waits for responses") | `sources/agent-framework/python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:1187-1204` (`_run_outer_loop` docstring and body), `:1050-1100` (`_run_inner_loop`) |
| Group chat round cap | "max_rounds" parameter "to prevent infinite loops" | `sources/agent-framework/python/packages/orchestrations/agent_framework_orchestrations/_group_chat.py:137`, `:316` |

### Python — Agent (non-workflow) step loop

| Area | Evidence | File:Line |
|------|----------|-----------|
| Default iteration cap | `DEFAULT_MAX_ITERATIONS: Final[int] = 40` | `sources/agent-framework/python/packages/core/agent_framework/_tools.py:90` |
| Default errors-per-request cap | `DEFAULT_MAX_CONSECUTIVE_ERRORS_PER_REQUEST: Final[int] = 3` | `sources/agent-framework/python/packages/core/agent_framework/_tools.py:91` |
| Config TypedDict | `FunctionInvocationConfiguration` with `enabled`, `max_iterations`, `max_function_calls`, `max_consecutive_errors_per_request`, `terminate_on_unknown_calls` | `sources/agent-framework/python/packages/core/agent_framework/_tools.py:1344-1421` |
| Non-streaming step loop | `for attempt_idx in range(attempt_start, max_iterations if loop_enabled else 0)` — bounded `for` over `max_iterations`; on exhaustion forces a final call with `tool_choice="none"` so the model produces text | `sources/agent-framework/python/packages/core/agent_framework/_tools.py:2549-2696` |
| Streaming step loop | Same `range`-bounded for loop paired with `async for update in …` emission | `sources/agent-framework/python/packages/core/agent_framework/_tools.py:2717-2842` |
| Inner tool dispatch | `_auto_invoke_function(function_call_content, …)` with middleware pipeline and tool map | `sources/agent-framework/python/packages/core/agent_framework/_tools.py:1424-1593` |
| Agent runtime | `RawAgent.run` / `Agent.run` builds `_RunContext`, calls `client.get_response(..., stream=...)` (or non-streaming), then `_parse_non_streaming_response` / `_parse_streaming_response` | `sources/agent-framework/python/packages/core/agent_framework/_agents.py:899-977` |

### Python — Outer agent loop (experimental harness)

| Area | Evidence | File:Line |
|------|----------|-----------|
| Outer loop predicate default | `DEFAULT_MAX_ITERATIONS = 10` | `sources/agent-framework/python/packages/core/agent_framework/_harness/_loop.py:117` |
| Judge-driven outer loop default | `DEFAULT_JUDGE_MAX_ITERATIONS = 5` (tighter because LLM-judged loops are costly and probabilistic) | `sources/agent-framework/python/packages/core/agent_framework/_harness/_loop.py:122` |
| Loop class | `class AgentLoopMiddleware(AgentMiddleware):` annotated `@experimental(feature_id=ExperimentalFeature.HARNESS)` | `sources/agent-framework/python/packages/core/agent_framework/_harness/_loop.py:211-212` |
| Non-streaming run loop body | `while True: await call_next(); … stop,feedback = await self._evaluate_stop(loop_kwargs, work_iterations)`; stop precedence per iteration: `max_iterations` → `should_continue` | `sources/agent-framework/python/packages/core/agent_framework/_harness/_loop.py:439-513` |
| Streaming run loop body | Nested `while True` inside the streaming generator, with `await call_next(); async for update in inner: yield update`; injected "nudge" messages surface between iterations | `sources/agent-framework/python/packages/core/agent_framework/_harness/_loop.py:518-609` |
| Judge condition builder | `_build_judge_condition(judge_client, instructions)` builds an async predicate returning `(continue_bool, reasoning)`; explicit verdict markers (`VERDICT: DONE` / `VERDICT: MORE`) for clients that ignore structured output | `sources/agent-framework/python/packages/core/agent_framework/_harness/_loop.py:148-208` |
| Helper predicates | `todos_remaining(provider)` and `background_tasks_running(provider)` are `should_continue` factories; both reach into session state to decide | `sources/agent-framework/python/packages/core/agent_framework/_harness/_loop.py:751-796` |
| Middleware composition | Harness agent places `ToolApprovalMiddleware` outermost (it has its own `while True` re-invocation loop, `_harness/_tool_approval.py:388-405`) and folds user middleware inside | `sources/agent-framework/python/packages/core/agent_framework/_harness/_agent.py:499-520` |
| Tool approval inner loop | `while True: context.messages = self._inject_collected_responses(…); … await call_next(); … if not all_auto_approved: return; context.messages = []; context.result = None` | `sources/agent-framework/python/packages/core/agent_framework/_harness/_tool_approval.py:371-407` |

### .NET — Workflow engine (Pregel superstep)

| Area | Evidence | File:Line |
|------|----------|-----------|
| Execution mode enum | `internal enum ExecutionMode { OffThread, Lockstep, Subworkflow }` (delivery-shape choice, not a different engine) | `sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Workflows/Execution/ExecutionMode.cs:5-23` |
| Runner abstraction | `internal interface ISuperStepRunner` exposing `RunSuperStepAsync(CancellationToken)` returning `ValueTask<bool>` plus `HasUnprocessedMessages`, `HasUnservicedRequests` | `sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Workflows/Execution/ISuperStepRunner.cs:9-42` |
| In-process runner | `internal sealed class InProcessRunner : ISuperStepRunner, ICheckpointingHandle` | `sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcessRunner.cs:23` |
| Superstep delivery + concurrency | `RunSuperStepAsync` runs per-receiver delivery in parallel: `List<Task> receiverTasks = …; await Task.WhenAll(receiverTasks);` | `sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcessRunner.cs:305-336` |
| Streaming event loop | `StreamingRunEventStream.RunLoopAsync` waits for input, drives `while (this._stepRunner.HasUnprocessedMessages && !linkedSource.Token.IsCancellationRequested) { await this._stepRunner.RunSuperStepAsync(…) }`, writes an `InternalHaltSignal` epoch after every halt | `sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StreamingRunEventStream.cs:78-141` |
| Lockstep event stream | `internal sealed class LockstepRunEventStream : IRunEventStream` with a `do { … } while (!ShouldBreak())` superstep drain | `sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Workflows/Execution/LockstepRunEventStream.cs:103-163` |
| Per-step tracer | `InProcStepTracer.Advance` / `Complete` raise `SuperStepStartedEvent` / `SuperStepCompletedEvent` | `sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcStepTracer.cs:31-70` |
| Workflow definition | `public class Workflow` holds `ExecutorBindings`, `Edges`, `OutputExecutors`, `Ports`, `StartExecutorId`; ownership via `TakeOwnership`/`ReleaseOwnershipAsync` | `sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Workflows/Workflow.cs:19-198` |
| Subworkflow piggyback | `WorkflowHostExecutor` per `docs/.../specialized/WorkflowHostExecutor.cs` registers a child `ISuperStepRunner` via `ExecutionMode.Subworkflow` (parent's run loop drives the child) | `sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Workflows/Execution/InProcessRunnerContext.cs:528-540` (join pattern), `:299` (subworkflow runner field) |
| Orchestration builders → graph | `SequentialWorkflowBuilder`, `ConcurrentWorkflowBuilder`, `HandoffWorkflowBuilder`, `MagenticWorkflowBuilder`, `GroupChatWorkflowBuilder` derive from `OrchestrationBuilderBase<T>` so they compile to the same `Workflow` | `sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Workflows/SequentialWorkflowBuilder.cs:23`, `dotnet/src/Microsoft.Agents.AI.Workflows/ConcurrentWorkflowBuilder.cs:27`, `dotnet/src/Microsoft.Agents.AI.Workflows/HandoffWorkflowBuilder.cs:22-35`, `dotnet/src/Microsoft.Agents.AI.Workflows/MagenticWorkflowBuilder.cs:29` |
| Predefined execution environments | `InProcessExecutionEnvironment` exposes preset configurations (OffThread, Concurrent, Lockstep, Subworkflow) | `sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Workflows/InProcessExecution.cs:24-42` |

### .NET — Agent (non-workflow) runtime and outer loop

| Area | Evidence | File:Line |
|------|----------|-----------|
| Base agent class | `abstract partial class AIAgent` — `RunCoreAsync` / `RunCoreStreamingAsync` are abstract | `sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Abstractions/AIAgent.cs:38`, `:332-342`, `:502-506` |
| Function-invocation decorator | `FunctionInvocationDelegatingAgent` (and external `FunctionInvokingChatClient`) implement the inner tool-call loop; MAF wraps it but does not own the loop | `sources/agent-framework/dotnet/src/Microsoft.Agents.AI/FunctionInvocationDelegatingAgent.cs:15-87` |
| Outer loop default cap | `LoopAgent.DefaultMaxIterations = 10` | `sources/agent-framework/dotnet/src/Microsoft.Agents.AI/Harness/Loop/LoopAgent.cs:67-68` |
| Outer loop body | `RunCoreAsync` uses `while (true) { AgentResponse response = await InnerAgent.RunAsync(...); iteration++; … if (HasPendingApprovalRequests(response)) return …; if (iteration >= _maxIterations) return …; var step = await EvaluateAndBuildNextAsync(...); if (!step.ShouldContinue) return … }` | `sources/agent-framework/dotnet/src/Microsoft.Agents.AI/Harness/Loop/LoopAgent.cs:132-212` |
| Outer loop body — streaming | Parallel `RunCoreStreamingAsync` uses `while (true) { await foreach (var update in this.InnerAgent.RunStreamingAsync(...)) { … } … }` — yielding updates as they arrive, then evaluating after each iteration | `sources/agent-framework/dotnet/src/Microsoft.Agents.AI/Harness/Loop/LoopAgent.cs:215-321` |
| Evaluators | `AIJudgeLoopEvaluator`, `CompletionMarkerLoopEvaluator`, `DelegateLoopEvaluator`, `TodoCompletionLoopEvaluator` plug into the same loop; first evaluator requesting re-invoke wins | `sources/agent-framework/dotnet/src/Microsoft.Agents.AI/Harness/Loop/AIJudgeLoopEvaluator.cs`, `CompletionMarkerLoopEvaluator.cs`, `DelegateLoopEvaluator.cs`, `TodoCompletionLoopEvaluator.cs`; priority rule documented in `LoopAgent.cs:33-37` |
| Harness composition | `HarnessAgent.BuildAgent` registers `LoopAgent` first (outermost), then `UseToolApproval`, then `UseOpenTelemetry` | `sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgent.cs:140-169` |
| Chat-client pipeline (inner→outer) | `FunctionInvokingChatClient` ⇒ `MessageInjectingChatClient` ⇒ `PerServiceCallChatHistoryPersistingChatClient` ⇒ optional `AIContextProviderChatClient with CompactionProvider` | `sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgent.cs:30-38` (doc-comment), code at `:200-330` |

### Cross-cutting — Durable execution model

| Area | Evidence | File:Line |
|------|----------|-----------|
| Durable orchestrator (generator) | `def run_workflow_orchestrator(ctx, workflow, …)` is a **generator-based, deterministic, replayable** orchestrator — the same MAF `Workflow` retargeted to a durable host | `sources/agent-framework/python/packages/durabletask/agent_framework_durabletask/_workflows/orchestrator.py:689-911` |
| Same superstep loop shape | `while pending_messages and iteration < workflow.max_iterations:` (line 775), with phase structure (prepare all tasks → execute in parallel → route results → check fan-in → HITL pause → publish status) | `sources/agent-framework/python/packages/durabletask/agent_framework_durabletask/_workflows/orchestrator.py:775-904` |
| Parallel task execution | `raw_results = yield ctx.task_all(all_tasks)` (line 796) — single durable `when_all` per superstep across both activities and agents | `sources/agent-framework/python/packages/durabletask/agent_framework_durabletask/_workflows/orchestrator.py:793-797` |
| HITL pause | `while True: raw_response = yield ctx.wait_for_external_event(request_id)` — wait/replay-friendly external-event pattern | `sources/agent-framework/python/packages/durabletask/agent_framework_durabletask/_workflows/orchestrator.py:863-900` |
| Replay safety | Events are emitted only when `not ctx.is_replaying` (line 757); events accumulate per superstep and are published into `set_custom_status` | `sources/agent-framework/python/packages/durabletask/agent_framework_durabletask/_workflows/orchestrator.py:754-767` |
| Convergence exception parity | Mirrors core: "Match the core WorkflowRunner: if the loop stopped because max_iterations was reached … raise `WorkflowConvergenceException`" | `sources/agent-framework/python/packages/durabletask/agent_framework_durabletask/_workflows/orchestrator.py:906-909` |
| Registration model | Each non-agent executor becomes a *durable activity*; each agent becomes a *durable entity*; the orchestrator drives both | `sources/agent-framework/python/packages/durabletask/agent_framework_durabletask/_workflows/registration.py:1-78`, `dotnet/src/Microsoft.Agents.AI.DurableTask/` |

## Answers to Dimension Questions

### 1. What is the primary execution model?

A **Pregel-style superstep graph executor** is the headline model and is explicit in both stacks (`python/.../_workflows/_runner.py:31`, `python/.../_workflows/_workflow.py:206-223`, mirrored by `dotnet/.../Microsoft.Agents.AI.Workflows.InProc/InProcessRunner.cs` driving `ISuperStepRunner.RunSuperStepAsync`). On top of it sit three delivery modes (`OffThread` / `Lockstep` / `Subworkflow`, `dotnet/.../Microsoft.Agents.AI.Workflows/Execution/ExecutionMode.cs:5-23`) and a *separate* **agent run loop** that is implemented in at least three places: (a) the function-call step loop inside `FunctionInvocationLayer.get_response` (`python/.../_tools.py:2565-2696`, `.NET FunctionInvokingChatClient`), (b) the optional middleware-level outer loop `AgentLoopMiddleware` (`python/.../_harness/_loop.py:211-796`) / `LoopAgent` (`dotnet/.../Harness/Loop/LoopAgent.cs:65-547`), and (c) per-call-around re-invocation in `ToolApprovalMiddleware` (`python/.../_harness/_tool_approval.py:388-407`) and the `.NET ToolApprovalAgent` decorator.

### 2. Is it explicit or emergent?

Explicit. The model is named ("Pregel supersteps"), the iteration counter is a first-class property (`self._iteration` in Python, `SuperStepStartInfo.StepNumber` in .NET), the convergence rule is one literal `if not await self._ctx.has_messages(): break` (`python/.../_workflows/_runner.py:149-150`) and `HasUnprocessedMessages` going false (`.NET StreamingRunEventStream.cs:110-114`), and the cap is `DEFAULT_MAX_ITERATIONS = 100` in Python. The .NET side lacks a numeric cap and instead relies on "no work remaining" termination (`dotnet/.../Microsoft.Agents.AI.Workflows/InProc/InProcessRunner.cs:212-241`).

### 3. Does the model match the product shape?

Yes. The product README explicitly advertises "graph-based … sequential, concurrent, handoff, and group collaboration patterns" (`README.md:34-46`) and durable multi-agent workflows (`README.md:46`), which is exactly what superstep graphs describe well: parallel activation of message receivers with deterministic fan-in, request ports, and a global checkpoint boundary. The single-turn chat-agent model matches the consumer shape (`Agent.run(...)` in Python; `AIAgent.RunAsync` in .NET).

### 4. Is the model easy to explain to a new contributor?

Workflow engine: yes — two sentences suffice across both languages:

> "Execution advances one superstep at a time. Each superstep drains every enqueued message, delivers each message to its target executor concurrently per edge group, and emits `superstep_started` / `superstep_completed` events with an incrementing iteration counter; the runner exits when no messages remain (Python) or when `HasUnprocessedMessages` is false (.NET), and the per-step boundary is the checkpoint boundary."

Agent-level loops: harder — three layers must each be summarized (function-call step loop, optional outer loop middleware, optional tool-approval re-invoke), and the same concept lives in different files in Python and .NET (`_tools.py`'s `for attempt_idx in range(...)` vs. `FunctionInvokingChatClient`; `_harness/_loop.py`'s `AgentLoopMiddleware` vs. `dotnet/.../Harness/Loop/LoopAgent.cs`). A new contributor must understand which surface they're on.

### 5. Does the system mix models cleanly or accidentally?

Cleanly overall, but **with notable layering**:

- The five orchestration patterns (sequential/concurrent/group-chat/Magentic/handoff) **all compile to the same `Workflow`/`Runner`** via builders that derive from `OrchestrationBuilderBase<T>`; they are not separate engines.
- The `.NET ExecutionMode` enum cleanly separates delivery shape (`OffThread` vs. `Lockstep` vs. `Subworkflow`) from execution semantics — the same `ISuperStepRunner` powers all three (`Execution/ExecutionMode.cs:5-23`).
- The `AgentLoopMiddleware` / `LoopAgent` outer loop deliberately sits **outside** the workflow engine, applied as `AIAgentBuilder.Use(...)` decoration (`.NET HarnessAgent.cs:149-156`) or as an `AgentMiddleware` middleware (Python). Users opt into it explicitly.
- Function-calling iteration is implemented in the chat-client layer (`FunctionInvocationLayer` / `FunctionInvokingChatClient`), not in `AIAgent`/`Agent` itself — a deliberate choice visible in `python/.../_agents.py:722-724` ("The provided chat client does not support function invoking, this might limit agent capabilities.").
- Durability is **not** an accidental extension: `durabletask/_workflows/orchestrator.py:689-911` re-encodes the same graph engine as a generator-based, replayable orchestration, and the convergence exception is explicitly "matched" to the core runner.

## Architectural Decisions

- **Pregel supersteps, not stateful agent loops.** Both languages bind on the same model. The per-superstep boundary is exactly the checkpoint boundary (`_runner.py:144`, `.NET InProcessRunner.cs:332`), so *convergence = checkpoint*.
- **Two parallel implementations.** Python and .NET are independent trees with mirrored type names (`Workflow`, `Run`, `SuperStepStartedEvent`, `RequestInfoEvent`, `OutputExecutors`). No shared engine exists in the repo.
- **Function-calling iteration lives in the chat-client substrate.** `FunctionInvocationLayer.get_response` in Python (`_tools.py:2388-2842`) and `FunctionInvokingChatClient` in .NET (external, wrapped by MAF). The agent loop story is therefore "the chat client owns inner iteration; the agent owns outer iteration; the workflow engine owns graph iteration".
- **All orchestration patterns → same `Workflow`.** Sequential/concurrent/group-chat/Magentic/handoff builders compile to the same engine rather than implementing their own loop. Magentic's outer-loop/inner-loop vocabulary (`_magentic.py:1187-1204`) is a *graph* structure (compiles to executors with edges), not a separate engine.
- **Multiple event-stream shapes; one model.** `ExecutionMode` (`ExecutionMode.cs:5-23`) selects `StreamingRunEventStream` (default — events emitted as they happen), `LockstepRunEventStream` (events batched per superstep), or `Subworkflow` (child `ISuperStepRunner` driven by parent). They're delivery shapes, not separate engines.
- **Concurrency inside a step is per-edge, not per-step.** `asyncio.gather(*tasks)` over edge-runners (`_runner.py:202-210`) and `Task.WhenAll(receiverTasks)` over executors (`.NET InProcessRunner.cs:310-318`), with explicit per-edge ordering guarantees (`_runner.py:191-193`).
- **Distinct iteration caps at each loop layer.** Workflow: 100 (`_const.py:4`); function-calling: 40 (`_tools.py:90`); outer agent loop: 10 (`_harness/_loop.py:117`, `LoopAgent.cs:67`); judge outer loop: 5 (`_harness/_loop.py:122`).

## Notable Patterns

- **Three nested iteration layers**:
  1. Workflow superstep loop (`Runner.run_until_convergence` / `ISuperStepRunner.RunSuperStepAsync`) — 100 iterations max.
  2. Agent function-call step loop (`FunctionInvocationLayer.get_response` / `FunctionInvokingChatClient`) — 40 iterations max.
  3. Agent outer loop (`AgentLoopMiddleware` / `LoopAgent`) — 10 iterations max by default.
- **Per-superstep checkpoint boundary**: explicit at `_runner.py:144` and `InProcessRunner.cs:332`. Checkpoint failures fall back gracefully (`.NET _create_checkpoint_if_enabled` swallows errors with logging only).
- **Edge runner indirection**: per-edge-group classes chosen by `create_edge_runner(edge_group, executors)` in Python (`.py:18-19`) and a parallel strategy in `.NET Execution/EdgeRunner.cs`-derived types (`DirectEdgeRunner`, `FanOutEdgeRunner`, `FanInEdgeRunner`, `ResponseEdgeRunner`).
- **Output designation as an allow-list**: `output_from`/`intermediate_output_from` on `WorkflowBuilder` (`_workflow.py:286-316`); allows the public `Workflow.run()` interface to stay singular while composition varies (referenced in core `AGENTS.md`).
- **Generator-based durable re-encoding**: `durabletask/_workflows/orchestrator.py:689-911` is a *generator* that yields orchestration primitives (`ctx.task_all(...)`, `ctx.wait_for_external_event(...)`); the host records each yield and replays it deterministically.
- **Middleware re-invocation via `call_next`**: in Python, an agent middleware's `process(context, call_next)` can be designed to call `call_next()` multiple times (see `AgentLoopMiddleware.process`, `_harness/_loop.py:398-421`, and `ToolApprovalMiddleware.process`'s `while True`, `_harness/_tool_approval.py:388-405`).
- **Streaming turn loop with epoch gating**: `.NET StreamingRunEventStream` emits an `InternalHaltSignal(currentEpoch, capturedStatus)` after each input→processing→halt cycle so the consumer's `await foreach` knows when to break (`StreamingRunEventStream.cs:122-126`).
- **Status channel separation**: control-plane status (`RunStatus` / `WorkflowRunState`) is decoupled from data-plane executor events.

## Tradeoffs

- **Three layered loops is more vocabulary but more flexibility**: a contributor can choose where to bound (workflow step, agent step, outer agent loop) but must know which layer they are reasoning about.
- **Synchronous supersteps vs. Python cooperative concurrency**: the runner explicitly notes "true parallelism is not realized in Python" (`_runner.py:171`); edge ordering holds but cross-edge fan-in is best-effort.
- **Convergence is the only termination detector**: no fingerprint-based cycle detection. Runaway graphs converge only by hitting `max_iterations` and throwing `WorkflowConvergenceException` (`_runner.py:152-153`).
- **Function-calling loop is not under `Agent`'s direct control**: callers compose `FunctionInvokingChatClient` (or wrap a client that lacks it) themselves, so an `Agent` that wraps an un-layered chat client silently loses tool-call iteration (warning emitted at `_agents.py:723-724`).
- **Two-language parallelism doubles the surface**: a new edge group type must be implemented, validated, and tested in both `python/.../_workflows/_edge.py` and `dotnet/.../Microsoft.Agents.AI.Workflows/Edge.cs`.
- **`AgentLoopMiddleware` is experimental `@experimental(feature_id=ExperimentalFeature.HARNESS)`** (`_harness/_loop.py:211`); until promoted, adopters may need to lock to a specific version.
- **Streaming + unbounded channel**: `.NET StreamingRunEventStream._eventChannel` is unbounded (`StreamingRunEventStream.cs:39-44`); pathological fan-out can buffer many events before consumer drain. The `Lockstep` mode is the explicit countermeasure.
- **`SingleReader = true` channel + `Interlocked.CompareExchange` gate** (`.NET StreamingRunEventStream.cs:42`, `AsyncRunHandle.cs:64-67`) means a second concurrent enumerator throws `InvalidOperationException`.

## Failure Modes / Edge Cases

- **`WorkflowConvergenceException`** when `max_iterations` is reached and messages are still pending (Python `_runner.py:152-153`; durable host mirrors this at `durabletask/_workflows/orchestrator.py:906-909`). Users see this as a hard failure.
- **Concurrent fresh `message=` runs blocked** in Python: "Cannot start a new run with 'message' while in-flight executor …" at `_workflow.py:803-810`. Resumption requires `checkpoint_id`.
- **Subworkflow ownership constraints** (.NET): a `Workflow` cannot simultaneously be a subworkflow and a top-level runnable, and cannot be reused as a subworkflow of multiple parents (`Workflow.cs:166-179`).
- **Checkpoint ↔ graph fingerprint mismatch**: restoring an old checkpoint against a changed graph raises `WorkflowCheckpointException` (Python `_runner.py:275-279`; analogous in .NET). Mismatch is detected via `graph_signature_hash`.
- **In-loop runner reset on shared executors**: .NET `Workflow.TakeOwnership` rejects reuse when shared executors do not implement `IResettableExecutor` (`Workflow.cs:158-165`).
- **`ToolApprovalMiddleware` requires `AgentSession`**: raises `RuntimeError` without one (`_harness/_tool_approval.py:373-374`); a `while True` middleware must not be paired with a stateless run.
- **Tool-iteration loop forces a final `tool_choice="none"` call** after `max_iterations` so the model emits text instead of orphan tool calls (`_tools.py:2665-2683`).
- **`max_function_calls` is best-effort**: "checked after each batch of parallel tool calls completes" (`_tools.py:2636-2645`); a batch of 20 parallel calls when the limit is 10 still runs.
- **Streaming single-reader enforcement**: `.NET AsyncRunHandle.cs:64-67` throws on the second concurrent enumerator.
- **Functional workflow behavior divergence**: `_functional.py` is experimental `FUNCTIONAL_WORKFLOWS`; `RequestInfo` interrupts surface as `WorkflowInterrupted` (a `BaseException`-derived signal) which code catching plain `Exception` will not see (see previous study's note 148).
- **Durable orchestrator replay safety**: `durabletask/_workflows/orchestrator.py:754-757` must use `ctx.is_replaying` to gate `set_custom_status` and event emission, or live events double-fire.

## Future Considerations

- A **shared numeric iteration cap on the .NET workflow side** would close a parity gap with Python's `DEFAULT_MAX_ITERATIONS = 100`.
- **Cycle detection earlier in the convergence loop** (e.g., a fingerprint of `{iteration, pending-message digests}`) could turn `WorkflowConvergenceException` from a last-resort guard into a useful diagnostic.
- **Single-engine cross-language parity**: Python and .NET remain independent trees; a normative shared schema (the kind partly present in `_typing_utils.py` and `AgentRequestMessageSourceAttribution.cs`) could evolve into a normative shared type.
- **Promote `AgentLoopMiddleware` to released**: the harness loop is `@experimental(HARNESS)` and consumers currently must assemble the harness factory `create_harness_agent` (`_harness/_agent.py:239-525`) or wire `AgentLoopMiddleware` manually; graduating it would make the outer-loop vocabulary standard.
- **Promote `FunctionalWorkflow`** (`@experimental(FUNCTIONAL_WORKFLOWS)`): same `Workflow.run()` signature, but a separate runtime; observable behavior (events, checkpoints) should converge.
- **Convergence fingerprinting for durable orchestration**: `durabletask/_workflows/orchestrator.py:775` re-uses `max_iterations` rather than a graph-trace-based stop condition; replaying long workflows would benefit from explicit early termination.
- **Workflow start/progress latency under `Lockstep`**: deliberately slower (per-step batching); flag this in `InProcessExecution.cs` so users picking a preset know.

## Questions / Gaps

- **No clear evidence found** for any cycle-prevention / hash-based loop detector in `python/.../_workflows/` or `dotnet/.../Microsoft.Agents.AI.Workflows/`. Termination is `not has_messages()` (Python) / `HasUnprocessedMessages` going false (.NET) only.
- **No numeric `max_iterations` on the .NET `ISuperStepRunner`** was found in the searched tree (search boundary: `grep -n "max_iterations\|MaxIterations" sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Workflows/Execution/**/*.cs`). Termination is solely by absence of work. Search boundary explicitly excludes user-facing `LoopAgentOptions.MaxIterations` (`dotnet/.../Harness/Loop/LoopAgent.cs:67, 123`), which is a separate (outer-loop) bound.
- **No shared cross-language execution engine**: no extracted core (C, WASM, or otherwise) was found in the repo. The 1:1 mirroring of model is by convention. `No evidence found` in the searched tree.
- **`AgentLoopMiddleware` and `LoopAgent` carry different default caps**: 10 each (and 5 for the judge pattern in Python), but the rationale strings differ (`LoopAgent.cs:67` constant only; Python `_harness/_loop.py:114-122` has the rationale "guard against runaway re-invocation" / "LLM-judged loops are costly and probabilistic"). A normative shared cap would be a small win.
- **`max_iterations=10` for `LoopAgent` does not document its precedence with `LoopEvaluator.MaxIterations`** in the source; readers must deduce "first evaluator requesting re-invoke wins" from `LoopAgent.cs:32-37`.
- **`ToolApprovalMiddleware.process`'s outer `while True`** (`_harness/_tool_approval.py:388-405`) is not labelled as an iteration loop in code comments; only the docstring of `ToolApprovalRuleToolResponse` and the behavior of `auto_approval_rules` indicate intent.

---

Generated by `01.01-execution-model-taxonomy` against `agent-framework`.

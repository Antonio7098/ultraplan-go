# Source Analysis: agent-framework

## 01.02 — Control-Flow Ownership

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `sources/agent-framework` |
| Language / Stack | Python (3.11+) and .NET (C#, Microsoft.Extensions.AI). Python is the primary, larger implementation; .NET provides a parallel Agent Framework + Workflows + Harness. |
| Analyzed | 2026-07-02 |

## Summary

The framework owns control flow through layered, explicit state machines rather than leaving it to the model. Three control-flow loops are stacked:

1. **Function-invocation loop inside the chat client** — `FunctionInvocationLayer.get_response()` (`python/packages/core/agent_framework/_tools.py:2567`) iterates a model round-trip / tool-execute cycle with a fixed budget (`max_iterations` default 40, `max_function_calls` optional, `max_consecutive_errors_per_request` default 3). The loop is a `for attempt_idx in range(max_iterations)` (runtime-driven, never model-driven). Each iteration runs `_process_function_requests` which returns one of three actions — `"return"` / `"continue"` / `"stop"` (`_tools.py:2209`, `_tools.py:2265`). On exhaustion the framework forces a final non-tool model call (`_tools.py:2668-2696`). The model cannot bypass this loop; it is the framework that calls `super_get_response` and decides what happens next.

2. **Workflow superstep loop (Pregel-style)** — `Runner.run_until_convergence` (`python/packages/core/agent_framework/_workflows/_runner.py:78`) iterates `while self._iteration < self._max_iterations:` (default `DEFAULT_MAX_ITERATIONS = 100`, `_workflows/_const.py:4`), draining messages per superstep, executing all matching `Executor.execute()` handlers (`_workflows/_executor.py:219`), then halting on "no more messages" or raising `WorkflowConvergenceException` if the cap is hit (`_runner.py:152`). The .NET equivalent is `StreamingRunEventStream.RunLoopAsync` (`dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StreamingRunEventStream.cs:56`) which loops `while HasUnprocessedMessages` calling `RunSuperStepAsync` (`InProc/InProcessRunner.cs:212`). Transitions are typed events (`SuperStepStartedEvent`, `SuperStepCompletedEvent`, `ExecutorInvokedEvent`, `ExecutorFailedEvent`).

3. **Re-invocation / outer loops driven by evaluators** — `AgentLoopMiddleware` (`python/packages/core/agent_framework/_harness/_loop.py:206-470`) and the .NET `LoopAgent` (`dotnet/src/Microsoft.Agents.AI/Harness/Loop/LoopAgent.cs:133-212`) re-invoke the inner agent based on a `should_continue` predicate (or chat-client judge). Both expose explicit `max_iterations` caps (Python `DEFAULT_MAX_ITERATIONS = 10`, .NET `LoopAgent.DefaultMaxIterations = 10`), `next_message` / on-behalf-of feedback, and runtime override via `MiddlewareTermination` (`_middleware.py:72-79`) or `LoopEvaluation.Stop` (`LoopEvaluation.cs:59`). The model never decides how many times it runs — the framework does, and stops on cap, error, pending approval, or eval-stop.

The model can choose **what** to do (which tool, which function call, what text, which next speaker in Magentic), but it cannot choose **how the loop terminates, how many iterations run, or which executor dispatches next**. The runtime has clear, explicit authority to reject or rewrite actions: `tool_choice = "none"` (`_tools.py:2592,2644,2673,2816`), `MaxIterationsPerRequest` on `FunctionInvokingChatClient` (via `Microsoft.Extensions.AI`), `MaxOutputTokens`, cap on `max_function_calls` even mid-batch (`_tools.py:2636-2644`), `WorkflowConvergenceException` on unbounded graphs, `_check_round_limit` (`_base_group_chat_orchestrator.py:499`) and `_max_rounds` for group chat / Magentic. Transitions are explicit typed actions and events, not scattered branches. The whole stack is testable without an LLM through `MockChatClient` / `MockBaseChatClient` (`tests/core/conftest.py:219-260`) and dedicated suites like `tests/core/test_function_invocation_logic.py` (`test_max_iterations_limit` line 1000, `test_max_function_calls_limits_parallel_invocations` line 1296).

## Rating

**9 / 10** — Mature, durable, observable, extensible, and proven under failure.

The control-flow story is one of the strongest parts of this framework. There is an explicit, layered set of state machines: `FunctionRequestResult.action ∈ {return, continue, stop}`, `_max_iterations` + `_max_rounds` + `MaxIterationsPerRequest` enforced by the runtime (not the model), typed workflow events, `MiddlewareTermination` as a first-class control-flow primitive, `RunStatus` enum (`RunStatus.cs`), `LoopEvaluation` enum-like, and `MagenticProgressLedger` slots for "next action" decisions. The model is never in charge of termination or routing; it returns typed content (`function_call`, `function_approval_request`, text, structured `next_speaker`) and the runtime interprets it under explicit guards. Test coverage of these mechanics is heavy (see `tests/core/test_function_invocation_logic.py`, `tests/core/test_harness_loop.py`, orchestrations `tests/test_magentic.py`). A non-trivial reason this is not a perfect score: there are scattered "stop" / "return" / "continue" string literals inside `_tools.py:2209,2265,2273,2376` that could be an enum (`Literal` type alias only); some orchestration helpers like `_check_round_limit_and_yield` (`_base_group_chat_orchestrator.py:520`) and the `_run_inner_loop_helper` orchestration of Magentic (`_magentic.py:1060`) mix control flow into the orchestrator body instead of a single transition function; and the "router" concept is implicit in `Executor._find_handler` and `EdgeGroup` matching rather than a named `Router` primitive.

## Evidence Collected

Every entry includes a file path with line numbers. Format `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Function-invocation loop (Python) | The chat-client `FunctionInvocationLayer.get_response` runs a `for attempt_idx in range(max_iterations)` loop driven by the runtime, not the model. | `python/packages/core/agent_framework/_tools.py:2567` |
| Loop action enum | The action set returned by the inner step: `Literal["return", "continue", "stop"]` — three explicit termination/intransit states. | `python/packages/core/agent_framework/_tools.py:2209` |
| Action emitted from result handler | `"action": "stop" if reached_error_limit else "continue"` — runtime-driven stop on consecutive-error budget. | `python/packages/core/agent_framework/_tools.py:2265` |
| Approval-driven action rewrite | `_process_function_requests` rewrites `"continue"`/`"stop"` → `"return"` when approval flow is needed or middleware raised `MiddlewareTermination`. | `python/packages/core/agent_framework/_tools.py:2374-2377` |
| Loop budget constants | `DEFAULT_MAX_ITERATIONS = 40`, `DEFAULT_MAX_CONSECUTIVE_ERRORS_PER_REQUEST = 3` — explicit runtime caps. | `python/packages/core/agent_framework/_tools.py:90-91` |
| Loop configuration | `FunctionInvocationConfiguration` TypedDict — typed surface for the loop's levers. | `python/packages/core/agent_framework/_tools.py:1344-1398` |
| Runtime override of model choice | `mutable_options["tool_choice"] = "none"` — runtime forces no-tool call after budget exhaustion / errors / cap. | `python/packages/core/agent_framework/_tools.py:2592,2635,2644,2673,2739,2741,2816,2844` |
| Forced final non-tool turn | After the iteration budget is exhausted, the framework runs `super_get_response(..., options=mutable_options)` with `tool_choice="none"` to avoid orphaned function_call items. | `python/packages/core/agent_framework/_tools.py:2664-2696` |
| Streaming loop variant | Parallel streaming version of the same `for attempt_idx in range(max_iterations)` loop. | `python/packages/core/agent_framework/_tools.py:2717-2833` |
| `tool_choice="required"` reset | Reset `tool_choice` after one iteration when caller sets `"required"` so the loop doesn't repeat forever. | `python/packages/core/agent_framework/_tools.py:2647-2652` |
| Hosted-tool approval pass-through | `_is_hosted_tool_approval` lets MCP/hosted tool approvals flow through unchanged. | `python/packages/core/agent_framework/_tools.py:1927-1937` |
| Middleware termination primitive | `MiddlewareTermination` exception — first-class control-flow primitive that propagates past the middleware pipeline. | `python/packages/core/agent_framework/_middleware.py:72-79` |
| Function-middleware pipeline (Python) | `FunctionMiddlewarePipeline.execute` — composes middleware around the tool call but lets `MiddlewareTermination` short-circuit. | `python/packages/core/agent_framework/_middleware.py:963-1000` |
| Agent-middleware pipeline (Python) | `AgentMiddlewarePipeline.execute` — same pattern at the agent-run level. | `python/packages/core/agent_framework/_middleware.py:876-927` |
| Chat-middleware pipeline (Python) | `ChatMiddlewarePipeline.execute` — request/response interception at the chat-client level. | `python/packages/core/agent_framework/_middleware.py:1036-1100` |
| Progressive tool exposure | Middleware can mutate the live tools list via `FunctionInvocationContext.add_tools` / `remove_tools`, but the docstring warns changes apply on the next function-call iteration, not mid-batch. | `python/packages/core/agent_framework/_middleware.py:266-352` |
| Agent-layer run dispatch | `AgentMiddlewareLayer.run` (Python) composes middleware and forwards to `BaseAgent.run`. | `python/packages/core/agent_framework/_middleware.py:1332-1420` |
| Chat-client-base run dispatch | `BaseAgent._call_chat_client` is the single point where the agent hands off to the chat client (the function-invocation loop owner). | `python/packages/core/agent_framework/_agents.py:995-1021` |
| Agent-run entry | `BaseAgent.run` prepares context, then either `_run_non_streaming` or `_run_streaming` — both ultimately delegate to the chat client. | `python/packages/core/agent_framework/_agents.py:948-977` |
| Provider hooks | `AgentContextProvider.before_run` / `after_run` hooks provide observation around the loop. | `python/packages/core/agent_framework/_sessions.py:621-641` |
| Workflow superstep loop (Python) | `Runner.run_until_convergence` — `while self._iteration < self._max_iterations` Pregel-style superstep driver. | `python/packages/core/agent_framework/_workflows/_runner.py:99-156` |
| Superstep events | `WorkflowEvent.superstep_started` / `superstep_completed` — explicit transition events. | `python/packages/core/agent_framework/_workflows/_runner.py:101,146` |
| Convergence cap | Raises `WorkflowConvergenceException` if the workflow does not converge after `max_iterations`. | `python/packages/core/agent_framework/_workflows/_runner.py:152-153` |
| Workflow iteration cap default | `DEFAULT_MAX_ITERATIONS = 100` — runtime-enforced safety bound for any workflow. | `python/packages/core/agent_framework/_workflows/_const.py:4` |
| Executor dispatch | `Executor.execute` finds a handler by message type via `_find_handler` and invokes it with a runtime-built `WorkflowContext`. | `python/packages/core/agent_framework/_workflows/_executor.py:219-294` |
| Handler discovery / registry | Handlers are registered via the `@handler` decorator and stored in `self._handlers` (typed by message type). | `python/packages/core/agent_framework/_workflows/_executor.py:329-353` |
| Workflow context send/yield | `WorkflowContext.send_message` / `yield_output` — the only ways an executor tells the runtime what should happen next. | `python/packages/core/agent_framework/_workflows/_workflow_context.py:308-391` |
| Request-info / human-in-the-loop | `ctx.request_info(...)` halts the superstep until `ctx.send_response(...)` returns. | `python/packages/core/agent_framework/_workflows/_workflow_context.py:424` |
| Edge runner dispatch | `EdgeRunner.send_message` carries a `WorkflowMessage` across edges; FanIn/FanOut/SwitchCase live there. | `python/packages/core/agent_framework/_workflows/_edge_runner.py:41-307` |
| Workflow switch / router | `SwitchCaseEdgeGroupCase` — declarative conditional routing. | `python/packages/core/agent_framework/_workflows/_edge.py:689-731` |
| Approval gating inside function call | `_handle_function_call_results` returns `"return"` if any result is `function_approval_request` or `function_call` (human-in-the-loop halt). | `python/packages/core/agent_framework/_tools.py:2228-2246` |
| Magentic orchestrator | Magentic owns the inner loop: model-driven `next_speaker` from a `MagenticProgressLedger`, but the orchestrator checks limits, stalls, and replan. | `python/packages/core/agent_framework/orchestrations/agent_framework_orchestrations/_magentic.py:1050-1185` |
| Magentic ledger slots | `is_request_satisfied`, `is_in_loop`, `is_progress_being_made`, `next_speaker`, `instruction_or_question` — explicit transition decisions. | `python/packages/core/agent_framework/orchestrations/agent_framework_orchestrations/_magentic.py:310-321` |
| Group-chat termination | `_check_round_limit` / `_check_terminate_and_yield` — runtime stops the loop even if the model wants to continue. | `python/packages/core/agent_framework/orchestrations/agent_framework_orchestrations/_base_group_chat_orchestrator.py:354-538` |
| Handoff (model-driven routing) | Agents invoke `handoff_to_<target>` synthetic tools; `_AutoHandoffMiddleware` short-circuits the tool and yields `{handoff_to: <id>}`. | `python/packages/core/agent_framework/orchestrations/agent_framework_orchestrations/_handoff.py:130-152` |
| Handoff dispatch | `HandoffAgentExecutor._is_handoff_requested` + `_run_agent` inspect the response, then the executor routes to the named target via `ctx.send_message`. | `python/packages/core/agent_framework/orchestrations/agent_framework_orchestrations/_handoff.py:351-406` |
| Agent-loop middleware (Python) | `AgentLoopMiddleware.should_continue` predicate + `next_message` + `max_iterations` (default 10, judge default 5) re-invoke the agent until done. | `python/packages/core/agent_framework/_harness/_loop.py:206-470` |
| Loop stop precedence | "Stop precedence per iteration is `max_iterations` → `should_continue`, evaluated before `record_feedback`" — runtime order is documented. | `python/packages/core/agent_framework/_harness/_loop.py:11-25` |
| Judge-driven loop | `AgentLoopMiddleware.with_judge` — second chat client decides via a structured `JudgeVerdict`. Stop precedence: `"MORE"` wins ambiguous replies, "FAIL before DONE" pattern. | `python/packages/core/agent_framework/_harness/_loop.py:148-196` |
| Non-streaming run loop in agent (Python) | `BaseAgent.run` prepares context, calls `_call_chat_client`, then finalizes — agent itself doesn't loop, the chat client does. | `python/packages/core/agent_framework/_agents.py:962-1049` |
| .NET ChatClientAgent RunCore | `ChatClientAgent.RunCoreAsync` calls `chatClient.GetResponseAsync` exactly once; the function-invocation loop lives in `FunctionInvokingChatClient` from `Microsoft.Extensions.AI`. | `dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientAgent.cs:205-264` |
| .NET pipeline construction (harness) | `HarnessAgent` builds `FunctionInvokingChatClient → MessageInjectingChatClient → PerServiceCallChatHistoryPersistingChatClient → AIContextProviderChatClient`, then wraps the agent with `ToolApprovalAgent → OpenTelemetryAgent → LoopAgent`. | `dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgent.cs:131-255` |
| .NET function-invocation config | `MaximumIterationsPerRequest` configures the `FunctionInvokingChatClient` cap; configurable from the harness. | `dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgent.cs:228-230` |
| .NET in-process superstep runner | `InProcessRunner.RunSuperStepAsync` — the basic unit of workflow progression; `currentStep.HasMessages` decides whether to run. | `dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcessRunner.cs:212-243` |
| .NET superstep delivery | `RunSuperstepAsync` fans out per-receiver delivery with `asyncio`-style concurrency and raises `SuperStepCompletedEvent`. | `dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcessRunner.cs:305-334` |
| .NET streaming run loop | `StreamingRunEventStream.RunLoopAsync` — `while HasUnprocessedMessages: RunSuperStepAsync`, halts on `RunStatus.Idle` / `RunStatus.PendingRequests`. | `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StreamingRunEventStream.cs:87-140` |
| .NET lockstep variant | `LockstepRunEventStream` — same `while HasUnprocessedMessages` loop, batched per superstep. | `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/LockstepRunEventStream.cs:103` |
| .NET halt signal | `RunStatus` enum (Running / Idle / PendingRequests / Ended) — explicit runtime states; the runtime halts the loop on Idle/PendingRequests. | `dotnet/src/Microsoft.Agents.AI.Workflows/RunStatus.cs:11` |
| .NET Magentic orchestrator | `MagenticOrchestrator.RunCoordinationRoundAsync` checks `taskContext.CheckLimits()` (round + reset), updates progress ledger, halts on `IsRequestSatisfied`, resets on stalls, dispatches to `NextSpeaker`. | `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticOrchestrator.cs:246-335` |
| .NET Magentic ledger | `MagenticProgressLedger` slots: `IsRequestSatisfied`, `IsInLoop`, `NextSpeaker`, `InstructionOrQuestion` — typed transition decisions. | `dotnet/src/Microsoft.Agents.AI.Workflows/MagenticProgressLedger.cs:17-39` |
| .NET Handoff filtering | `HandoffToolCallFilteringBehavior` enum controls whether handoff tool calls surface or are intercepted. | `dotnet/src/Microsoft.Agents.AI.Workflows/HandoffToolCallFilteringBehavior.cs:8` |
| .NET HandoffBuilder | `HandoffWorkflowBuilder` — declarative handoff topology; control flow follows edges. | `dotnet/src/Microsoft.Agents.AI.Workflows/HandoffWorkflowBuilder.cs:1-50` |
| .NET LoopAgent | `LoopAgent.RunCoreAsync` runs `while(true): agent.run → HasPendingApproval → max_iterations → evaluators → next iteration`, breaking on `step.ShouldContinue == false`. | `dotnet/src/Microsoft.Agents.AI/Harness/Loop/LoopAgent.cs:169-211` |
| .NET LoopAgent safety cap | `_maxIterations = Throw.IfLessThan(options?.MaxIterations ?? DefaultMaxIterations, 1)` — explicit, runtime-enforced cap (default 10). | `dotnet/src/Microsoft.Agents.AI/Harness/Loop/LoopAgent.cs:123` |
| .NET AIJudgeLoopEvaluator | Judge-driven loop evaluator: returns `LoopEvaluation.Stop()` on answered, else continue with feedback. | `dotnet/src/Microsoft.Agents.AI/Harness/Loop/AIJudgeLoopEvaluator.cs:169-172` |
| .NET approval-pause | `LoopAgent` halts on pending approval requests: `if (HasPendingApprovalRequests(response)) return BuildResult(...)` — runtime cannot let the model burn past an approval. | `dotnet/src/Microsoft.Agents.AI/Harness/Loop/LoopAgent.cs:189-193` |
| .NET LoopEvaluation type | `LoopEvaluation.Stop()` static factory + `Continue(feedback, messages)` — typed transition decisions, not booleans. | `dotnet/src/Microsoft.Agents.AI/Harness/Loop/LoopEvaluation.cs:46-79` |
| .NET LoopContext | `LoopContext` exposes `Iteration`, `LastResponse`, `Feedback`, mutable per-run state bag — observable surface to evaluators. | `dotnet/src/Microsoft.Agents.AI/Harness/Loop/LoopContext.cs:17-90` |
| .NET ExternalRequest / ExternalResponse | Out-of-band human-in-the-loop primitives that suspend and resume the workflow — explicit halt/resume. | `dotnet/src/Microsoft.Agents.AI.Workflows/ExternalRequest.cs:1-50`, `dotnet/src/Microsoft.Agents.AI.Workflows/ExternalResponse.cs:1-50` |
| .NET subworkflow join | `WorkflowExecutor` and `ISuperStepJoinContext` — subworkflows can request input and synchronize. | `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/WorkflowHostExecutor.cs:1-50` |
| .NET GroupChat base | `GroupChatManager` is an abstract class with `SelectNextAgentAsync` / `ShouldTerminateAsync` overridable — extension points are typed. | `dotnet/src/Microsoft.Agents.AI.Workflows/GroupChatManager.cs:106` |
| Tests for max_iterations | `test_max_iterations_limit`, `test_max_iterations_no_orphaned_function_calls`, `test_max_iterations_makes_final_toolchoice_none_call`, `test_max_iterations_preserves_all_fcc_messages` — explicit coverage of the cap path. | `python/packages/core/tests/core/test_function_invocation_logic.py:1000,1047,1110,1169` |
| Tests for max_function_calls | `test_max_function_calls_limits_parallel_invocations`, `test_max_function_calls_single_calls_per_iteration`, `test_max_function_calls_none_means_unlimited` — explicit coverage of the cumulative cap. | `python/packages/core/tests/core/test_function_invocation_logic.py:1296,1348,1399` |
| Tests for cap validation | `test_function_invocation_config_validation_max_iterations` rejects `0` / `-1` — runtime refuses a runaway config. | `python/packages/core/tests/core/test_function_invocation_logic.py:1783-1799` |
| Tests for loop middleware | `test_loop_stops_at_max_iterations`, `test_streaming_stops_at_max_iterations`, `test_judge_respects_default_max_iterations` — runtime cap honored under streaming and judge modes. | `python/packages/core/tests/core/test_harness_loop.py:212,1079,808` |
| Tests for Magentic orchestration | `test_magentic_workflow_plan_review_approval_to_completion`, plus progress-ledger fake implementations — control flow testable without an LLM. | `python/packages/orchestrations/tests/test_magentic.py:300,417,558,789` |
| Mock chat client for control-flow tests | `MockChatClient` / `MockBaseChatClient` fixtures and `max_iterations` parametrization — entire loop testable without an LLM. | `python/packages/core/tests/core/conftest.py:219-260` |

## Answers to Dimension Questions

1. **Who decides what happens next?**
   The **runtime owns** the next-step decision; the **model only contributes content**. The chain is:
   - Chat-client loop: `FunctionInvocationLayer._get_response` (`python/packages/core/agent_framework/_tools.py:2567`) — `for attempt_idx in range(max_iterations)` plus `_process_function_requests` returning `FunctionRequestResult.action ∈ {return, continue, stop}` (`_tools.py:2209,2265`).
   - Workflow loop: `Runner.run_until_convergence` (`python/packages/core/agent_framework/_workflows/_runner.py:99-156`) iterates `Executor.execute` handlers per superstep; .NET equivalent is `StreamingRunEventStream.RunLoopAsync` (`dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StreamingRunEventStream.cs:87-140`).
   - Routing inside an executor: `Executor._find_handler` (`_workflows/_executor.py:254`) chooses by message type; `EdgeGroup` (FanOut/FanIn/SwitchCase) routes on declared conditions.
   - Re-invocation loops: `AgentLoopMiddleware` (`_harness/_loop.py:206`) and `LoopAgent` (`dotnet/src/Microsoft.Agents.AI/Harness/Loop/LoopAgent.cs:133-212`) call `should_continue` / `LoopEvaluator`s after each full agent run.

2. **Can the LLM bypass runtime control?**
   No. The LLM can only return typed content (text, `function_call`, `function_approval_request`, structured `next_speaker`). The runtime interprets that content under explicit guards:
   - `tool_choice = "none"` after budget or error (`_tools.py:2592,2635,2673,2844`).
   - `_max_iterations` and `_max_function_calls` enforced even when the model wants more (`_tools.py:2567,2636,2717`).
   - `_max_rounds` / `_check_round_limit` enforced even if the model's ledger says continue (`_base_group_chat_orchestrator.py:499-538`).
   - `_check_within_limits_or_complete` halts Magentic at round/reset limit (`_magentic.py:1222-1253`).
   - `WorkflowConvergenceException` on workflow non-convergence (`_runner.py:152`).
   - .NET `LoopAgent` halts on `HasPendingApprovalRequests` and on `iteration >= _maxIterations` (`LoopAgent.cs:189-200`).
   The LLM cannot "decide" to call tools beyond `max_function_calls`, run beyond `max_iterations`, route past unknown speakers, or burn past pending approval — the runtime strips the path each time.

3. **Can the runtime reject or rewrite the next action?**
   Yes, in several ways:
   - **Rewrite tool choice to "none"**: `_tools.py:2592,2635,2644,2673`.
   - **Drop tool results or rewrite role**: `_handle_function_call_results` (`_tools.py:2217-2270`).
   - **Raise `MiddlewareTermination`** to short-circuit pipelines (`_middleware.py:72-79`); the FunctionMiddlewarePipeline explicitly does NOT suppress it (`_middleware.py:996-998`).
   - **Force "return" on approval flow**: when any result is `function_approval_request`, the loop returns even mid-iteration (`_tools.py:2240-2246`).
   - **Force non-tool final call** when budget exhausted (`_tools.py:2668-2696`).
   - **Stop and yield** group-chat completion when `_check_round_limit` or `_check_terminate` triggers (`_base_group_chat_orchestrator.py:354-372,499-538`).
   - **`LoopEvaluation.Stop`** stops re-invocation even if agent wants more (`dotnet/src/Microsoft.Agents.AI/Harness/Loop/LoopEvaluation.cs:59`).

4. **Are transitions explicit or scattered?**
   Mostly explicit, with some scatter:
   - **Explicit typed actions**: `FunctionRequestResult.action ∈ {return, continue, stop}` (`_tools.py:2209`), `LoopEvaluation.Stop/Continue` (`LoopEvaluation.cs:46-79`), `RunStatus` (`RunStatus.cs:11`), `WorkflowEvent.type` (`_workflows/_events.py:122-158`), `MagenticProgressLedger` slots.
   - **Explicit transition functions**: `_process_function_requests`, `_handle_function_call_results`, `_run_iteration`, `_check_within_limits_or_complete`, `RunCoordinationRoundAsync`, `EvaluateAndBuildNextAsync`.
   - **Scatter**: the string literals `"return" / "continue" / "stop"` (`_tools.py:2209,2265,2273,2337,2627,2632,2645,2802,2806`) could be a single enum; the Handoff routing decision lives inside `HandoffAgentExecutor._run_agent` rather than a named `Router`.

5. **Is control flow testable without calling an LLM?**
   Yes, comprehensively. The `tests/core/conftest.py:219-260` fixtures define `MockChatClient` and `MockBaseChatClient` that fully exercise `FunctionInvocationLayer` without any external API. `tests/core/test_function_invocation_logic.py:1000-1399` covers every cap path (`max_iterations`, `max_function_calls`, errors, parallel). `tests/core/test_harness_loop.py:212,1079` covers the loop middleware end-to-end with no LLM. Orchestration tests use a `FakeMagenticManager` (`tests/test_magentic.py:131-140,558,789`) so Magentic control flow is fully tested without a real manager LLM. The .NET side similarly fakes `IChatClient` to validate `LoopAgent` / `MagenticOrchestrator` transitions.

## Architectural Decisions

- **Layered loops, not a single mega-loop**: chat-client function invocation → workflow superstep → outer re-invocation. Each layer has its own bounded state machine and its own next-step authority.
- **Three-action loop vocabulary (`return` / `continue` / `stop`)**: deliberately small so every consumer can pattern-match. The Python side encodes it as `Literal["return", "continue", "stop"]` (`_tools.py:2209`) rather than a class — pragmatic but unenforced.
- **One superstep = one batch of independent edges**: Pregel-style synchronous batching (`_runner.py:99-156`) keeps replay deterministic and gives checkpointing a clean unit. The runner drains messages, fans out across `EdgeRunner`s, and only commits state after every deliverer reports done.
- **Stream-aware finalization**: streaming uses the same `for attempt_idx` loop as non-streaming (`_tools.py:2717`) but breaks early if any message lacks `function_call` / `function_approval_request` (`_tools.py:2773-2778`). Approval pauses surface as `yield ChatResponseUpdate(contents=...)` then exit (`_tools.py:2797-2801`).
- **Hosted-tool approval routing**: approval flow does not pass through local execution — hosted/MCP tools flow through unchanged (`_tools.py:1927-1937`).
- **Magentic ledger is structured, not free-form**: slots like `next_speaker`, `instruction_or_question` are typed strings, not JSON blobs — the orchestrator still has authority to fall back if the model's answer is invalid (`_magentic.py:1121-1131`).
- **Handoff uses synthetic tools**: `_AutoHandoffMiddleware` short-circuits `handoff_to_<id>` calls into `{handoff_to: <id>}` payloads (`_handoff.py:130-152`). This keeps the routing decision visible to the framework rather than hidden inside opaque tool execution.
- **`tool_choice="required"` reset**: protects against the model getting stuck in a "required-tool" infinite loop — reset after one iteration (`_tools.py:2647-2652`).
- **Loop-aware fresh context**: `LoopAgent` with `FreshContextPerIteration` snapshots and restores the session each iteration so no leaked state pollutes the next run (`LoopAgent.cs:235-237`).
- **`.NET` mirrors Python's model + adds the same vocabulary**: `RunStatus`, `LoopEvaluation`, `MagenticProgressLedger` are the .NET analogs of the Python `Literal` action and Magentic ledger.

## Notable Patterns

- **Pipeline-as-decorator**: `FunctionInvocationLayer`, `ChatMiddlewareLayer`, `AgentMiddlewareLayer` are composable Python `class` mixins that wrap a parent (`super_get_response`) with middleware + post-processing (`_tools.py:2388`, `_middleware.py:1104,1254`).
- **Caller-side control**: `BaseAgent._call_chat_client` is a single point that the framework can swap (e.g. apply `AIContextProvider`s, replace chat client per-run) without exposing the loop to the agent (`_agents.py:995-1021`).
- **Typed events everywhere**: `WorkflowEvent` (Python) and `WorkflowEvent`/`ExecutorInvokedEvent`/`ExecutorCompletedEvent`/`ExecutorFailedEvent` (C#) make transitions observable (`_workflows/_events.py:122-158`).
- **Per-service-call persistence**: `PerServiceCallChatHistoryPersistingChatClient` persists after every chat-client call inside the loop, so a crashed tool execution still leaves a recoverable transcript (`dotnet/src/Microsoft.Agents.AI/ChatClient/PerServiceCallChatHistoryPersistingChatClient.cs:200`).
- **Approval-gated loops**: `LoopAgent` halts when a tool-approval request is pending — the loop cannot bypass the user (`LoopAgent.cs:189-193`).
- **Middleware-driven progressive disclosure**: `FunctionInvocationContext.add_tools/remove_tools` lets middleware mutate the live tools list between iterations (`_middleware.py:266-352`).
- **Inner-loop serialization in Magentic**: `_run_inner_loop_helper` is described as "Serialized with a lock" (`_magentic.py:1054`) — prevents two supersteps from racing the ledger update.

## Tradeoffs

- **`Literal` instead of `Enum`**: `FunctionRequestResult.action` uses a `Literal` (`_tools.py:2209`) rather than a class/Enum, which is ergonomic but offers no first-class exhaustiveness on the consumer side. Consumers do pattern-match, but adding a fourth action requires updating every return site.
- **Multiple loop budgets (`max_iterations`, `max_function_calls`, `max_rounds`, `max_reset_count`, `max_stall_count`)** give callers fine control but multiply the surface area to document. The Python file `python/packages/core/agent_framework/_harness/_loop.py` alone has three iterations-cap constants (`DEFAULT_MAX_ITERATIONS`, `DEFAULT_JUDGE_MAX_ITERATIONS`, `DEFAULT_MAX_ITERATIONS=100` for workflows at `_workflows/_const.py:4`).
- **`max_function_calls` is best-effort mid-batch**: the runtime checks after each batch completes, so a 20-call batch can overshoot a cap of 10 (`_tools.py:2636-2644` plus the docstring at `_tools.py:1360-1363`). This is documented but easy to miss.
- **Magentic ledger is JSON structured but unstructured for the model**: the framework tells the model what to emit, but parsing relies on the model's compliance; failures trigger replanning (`_magentic.py:1080-1085`) and the orchestrator's "best-effort" next-speaker fallback (`_magentic.py:1121-1131`).
- **Outer loops are decorators (Python) or wrapping agents (.NET)**, which is clean but means the loop is not visible in the agent's own `run()` signature — discoverability suffers, so configuration is heavily documented in `_loop.py:11-25` and `LoopAgentOptions.cs`.
- **Handoff agent tools are mixed-tool risky**: by default the framework auto-bypasses approval for non-approval tools in mixed batches (`python/packages/core/agent_framework/_harness/_tool_approval.py`), which keeps the loop running but requires explicit configuration to disable (`dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgentOptions.cs:206-215`).
- **`tool_choice="none"` reset is implicit**: the framework rewrites a model-issued `tool_choice` after budget exhaustion. The behavior is correct but is documented as a comment, not a typed guarantee.

## Failure Modes / Edge Cases

- **Concurrent superstep errors**: `InProcessRunner.RunSuperStepAsync` propagates exceptions but first surfaces pending events (`InProcessRunner.cs:122-130`); one executor's failure does not silently drop other executors' events.
- **Concurrent runs (`ExecutionMode.OffThread` + `enableConcurrentRuns`)**: handled via the per-runner `_nextStep` swap and atomic `Interlocked.Exchange` (`InProcessRunnerContext.cs:33,204`). Race surface is bounded.
- **Long-running MCP task cancellation**: explicit `_send_with_one_reconnect` and `max_task_wait` cap (`_mcp.py:1535-1541` documented in AGENTS.md "MCPTaskOptions"); failures fire best-effort `tasks/cancel` so the runtime cannot be left with dangling work.
- **Cancellation propagation**: `Runner.run_until_convergence` cancels the iteration task on `CancelledError` (`_runner.py:115-120`); the .NET `StreamingRunEventStream` has linked `CancellationToken`s (`StreamingRunEventStream.cs:58-59`).
- **Tool-call orphaning**: if the model returns `function_call` items but no result handler matches, the framework forces a final `tool_choice="none"` call so the response is clean text (`_tools.py:2664-2696`).
- **`tool_choice="required"` infinite-loop trap**: explicitly reset (`_tools.py:2647-2652`).
- **Streaming approval pause**: a model that emits `function_approval_request` mid-stream causes the loop to return — caller must supply responses on next call. The `_is_hosted_tool_approval` short-circuit preserves hosted-tool behavior (`_tools.py:1927-1937`).
- **Session-snapshot edge in LoopAgent**: `_freshContextPerIteration` requires a session snapshot; if the caller passes a session that can't be serialized, the snapshot step fails. The agent catches and reports this in `_initialSessionSnapshot` (`LoopAgent.cs:235-237`).
- **Judge ambiguity**: `AgentLoopMiddleware` judge marks `MORE` wins over `DONE` to keep the loop running on ambiguous replies (`_loop.py:189-196`).
- **Stream-vs-non-stream divergence**: streaming stops early when no `function_call` items appear (`_tools.py:2773-2778`), non-streaming continues until `action != "continue"` (`_tools.py:2627-2662`). Tests exist for both paths.
- **Magentic replan infinite-loop**: stall count is decremented when `IsProgressBeingMade` is true, but `max_reset_count` is the hard ceiling (`_magentic.py:1238-1253`). The .NET equivalent is `taskContext.CheckLimits()` (`MagenticOrchestrator.cs:248`).

## Future Considerations

- **Promote `Literal["return", "continue", "stop"]` to an `Enum`**: would let consumers pattern-match exhaustively and remove the string literals scattered through `_tools.py`. Backwards-compatible if `__call__` returns the enum value.
- **Unify the budget surface**: `max_iterations`, `max_function_calls`, `max_consecutive_errors_per_request`, `max_rounds`, `max_reset_count`, `max_stall_count`, `MaxIterationsPerRequest`, `LoopAgentOptions.MaxIterations` could live behind a single `LoopBudget` TypedDict / record. This would simplify the harness.
- **Explicit `Router` primitive**: Handoff's tool-based routing and Magentic's ledger-based routing both produce a "next-executor" decision but neither is named `Router`. Introducing one would clarify extension points.
- **Make `tool_choice="none"` rewrite observable**: log or emit a typed event when the runtime overrides the model's choice. This would help audit a runaway tool call.
- **Promote hosted-tool approval to a first-class TypedDict**: currently approval lives partly in `_tools._is_hosted_tool_approval` and partly in `MCPTool` call paths. A unified `ApprovalPolicy` would reduce surprise.
- **Streaming superstep budget**: the .NET `StreamingRunEventStream` halts on Idle/PendingRequests but does not currently surface an explicit `RunStatus.MaxIterationsReached` for superstep counts. Adding it would parallel the Python `WorkflowConvergenceException`.
- **Test concurrency edge cases** in the `enableConcurrentRuns=True` path explicitly — current tests cover OffThread sequentially.

## Questions / Gaps

- **Scattered string literals**: the `"return" / "continue" / "stop"` actions live in `_tools.py:2209,2265,2273,2337,2627,2632,2645,2802,2806` and would benefit from a single enum source. No clear evidence of a single transition dispatcher class.
- **`FunctionTool.parse_result` and tool approval routing** are spread across `_harness/_tool_approval.py`, `_tools._is_hosted_tool_approval`, and `security.py`. The full control-flow picture for hosted/MCP tools requires reading four files.
- **Magentic ledger parsing**: when the model returns non-conforming JSON for the progress ledger, the orchestrator falls back (`_magentic.py:1080-1085,1121-1131`); the failure-mode coverage of malformed responses is partly documented in tests but the production edge cases (partial JSON, schema drift, multi-language ledger fields) are not fully enumerated in the source comments.
- **Loop observable surface**: the Python harness exposes `iteration`, `last_result`, `messages`, `session`, `progress`, `feedback` to `should_continue` / `next_message` (`_loop.py:131-138`). The .NET `LoopContext` exposes similar fields. No shared observable contract across the two stacks — porting a custom evaluator across languages requires re-implementation.
- **Cancellation**: Python `asyncio.CancelledError` handling inside the function-invocation loop is at `_runner.py:115-120` (workflows), but inside `FunctionInvocationLayer._get_response` there is no explicit `try/except CancelledError`; behavior under cancellation depends on the inner `super_get_response` and `execute_function_calls` (`_tools.py:2594-2604,2547-2585`). No clear evidence found of a documented cancel contract.
- **`max_function_calls` is checked after a parallel batch**, which can overshoot the cap by up to one full batch's worth of calls. This is documented (`_tools.py:1360-1363`) but the magnitude is implicit.
- **`MiddlewareTermination` and `LoopEvaluation.Stop`** are similar in spirit but live in separate layers; there is no single "stop-the-world" runtime hook.

---

Generated by `dimensions/01.02-control-flow-ownership.md` against `agent-framework`.
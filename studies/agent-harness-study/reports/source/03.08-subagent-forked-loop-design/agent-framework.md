# Source Analysis: agent-framework

## 03.08 — Subagent and Forked-Loop Design

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (primary) + .NET (parity) — multi-package monorepo, Pydantic-based core, OpenTelemetry-traced |
| Analyzed | 2026-07-17 |

## Summary

Microsoft Agent Framework exposes delegation through **three first-class mechanisms**, each with different isolation, lifetime, and observability semantics:

1. **Agent-as-Tool** (`agent.as_tool()` in `python/packages/core/agent_framework/_agents.py:485-579`) — the parent agent invokes a child `Agent` synchronously inside its own function-call loop, wrapped as a `FunctionTool`. The child gets a fresh `AgentSession` by default (`session=ctx.session if propagate_session else None`, `_agents.py:562`); `propagate_session=True` shares the parent's session. The child is invoked through `self.run(...)` directly, so its `invoke_agent` span naturally nests inside the parent's span via OTel context propagation. Returns `text`, raises `UserInputRequiredException` for user-input requests (`_agents.py:568-569`), and propagates `function_invocation_kwargs` to the child through `FunctionInvocationContext.kwargs` (verified by `python/packages/core/tests/core/test_as_tool_kwargs_propagation.py:30-170`).

2. **BackgroundAgentsProvider** (`python/packages/core/agent_framework/_harness/_background_agents.py:240-521`) — a `ContextProvider` that injects six framework tools (`background_agents_start_task`, `_wait_for_first_completion`, `_get_task_results`, `_get_all_tasks`, `_continue_task`, `_clear_completed_task`, `_background_agents.py:316-495`) into a parent agent. Each child gets its own `AgentSession` (`bg_agent.create_session()`, `_background_agents.py:338`) and runs as an `asyncio.Task` via `asyncio.create_task(_run_agent(bg_agent.run(input, session=sub_session)))` (`_background_agents.py:341`). **Children run concurrently and in parallel** with the parent's loop; the parent's LLM decides when to wait. Errors are captured in `BackgroundTaskInfo.error_text` via `completed_task.exception()` (`_background_agents.py:197-200`), surfaced as `"Task failed: ..."` strings, and never propagate as exceptions.

3. **WorkflowExecutor / WorkflowAgent / orchestration patterns** (`python/packages/core/agent_framework/_workflows/_workflow_executor.py:103-697`, `_workflows/_agent.py:52-922`, `python/packages/orchestrations/agent_framework_orchestrations/_handoff.py:200-561`, `_magentic.py:1329-1366`, `_group_chat.py:96-540`, `_sequential.py:55-269`, `_concurrent.py`) — these wrap agents as graph executors that run as siblings within a `Workflow` (no parent/child relationship, all peers inside one workflow run span). A `WorkflowExecutor` itself wraps a *whole* child workflow as one executor (`_workflow_executor.py:103-264`) with per-execution-id state isolation (`ExecutionContext`, `_workflow_executor.py:37-54`), support for concurrent sub-workflow executions (`_workflow_executor.py:362`), checkpointing (`_workflow_executor.py:473-535`), and request/response bubbling via `SubWorkflowRequestMessage`/`SubWorkflowResponseMessage` (`_workflow_executor.py:57-100`).

Handoff uses an agent-as-tool *pattern at the tool-call level*: each agent is wrapped as a `HandoffAgentExecutor` (`_handoff.py:200-235`), gets synthetic `handoff_to_<target>` tools injected (`_handoff.py:332-347`), and an `_AutoHandoffMiddleware` (`_handoff.py:130-152`) short-circuits handoff tool calls with `MiddlewareTermination(result=...)` and triggers routing via `ctx.send_message(AgentExecutorRequest(...), target_id=handoff_target)` (`_handoff.py:408-414`). Magentic wraps participants in `MagenticAgentExecutor` (`_magentic.py:1329-1366`) and adds a `MagenticResetSignal` handler that **resets each child's session, cache, and conversation on replan** (`_magentic.py:1348-1367`) — an explicit acknowledgement that children have private state.

Delegation can happen without losing accountability: every child invocation is a real `agent.run(...)` (not a synthetic tool stub), the resulting `AgentResponse` returns to the parent as a tool result, OTel context naturally produces nested `invoke_agent` spans (`observability.py:1743-2014`), per-child `session_id` and `service_session_id` are tracked (`_agents.py:411-449`), subagent exceptions surface through `UserInputRequiredException` (`exceptions.py:184-209`) or are captured as `BackgroundTaskStatus.FAILED` payloads, and orchestration children are checkpointable and resettable as named units.

## Rating

**8 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

The framework has a deliberate, well-tested model of delegation spanning in-tool, in-background, and in-workflow patterns. The three mechanisms each have explicit isolation, lifecycle, and propagation contracts. Tests cover nested delegation kwargs propagation (`test_as_tool_kwargs_propagation.py:111-172`), background agent concurrency and error capture (`test_harness_background_agents.py`), nested sub-workflows (`test_sub_workflow.py`), handoff middleware interception (`test_handoff.py:1115-1148`), and Magentic replan resets (`test_magentic.py`). Deductions: (1) child traces rely on OTel context propagation rather than explicit parent-span attributes on the child (no `parent_run_id` / `delegated_by` field on `AgentResponse`); (2) the BackgroundAgentsProvider cannot cancel in-flight children from outside (no cancellation API; cancellation only fires when a task's event loop shuts down); (3) `UserInputRequiredException` is the only formally-typed error bubbling path — other child exceptions during `as_tool` wrapping become plain tool errors; (4) `MagenticAgentExecutor.handle_magentic_reset` reassigns `self._agent_thread = self._agent.create_session()` even though the property name suggests it is from a `BaseAgent`-style naming and is not consistent with how `BaseAgent` exposes sessions; (5) `propagate_session=True` couples child memory to parent memory in ways that are documented (`_agents.py:506-508`) but the lifecycle (when does the shared session diverge again?) is implicit.

## Evidence Collected

Every entry includes a file path with line numbers from the selected source.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Agent-as-Tool wrapper | `as_tool(...)` returns a `FunctionTool` whose body calls `self.run(str(kwargs.get(arg_name, "")), stream=True, session=ctx.session if propagate_session else None, function_invocation_kwargs=dict(ctx.kwargs))` | `python/packages/core/agent_framework/_agents.py:485-579` |
| Fresh vs shared session for as_tool | `propagate_session=False` default; `True` shares parent session | `python/packages/core/agent_framework/_agents.py:494,506-508,562` |
| UserInputRequiredException for user-input bubbling | `raise UserInputRequiredException(contents=final_response.user_input_requests)` if sub-agent emits `user_input_requests` | `python/packages/core/agent_framework/_agents.py:568-569` |
| UserInputRequiredException type definition | Carries `contents: list[Any]` from sub-agent | `python/packages/core/agent_framework/exceptions.py:184-209` |
| BackgroundAgentsProvider class | `ContextProvider` injecting 6 background-task tools; per-session `_RuntimeState` (in-flight `asyncio.Task`s and child `AgentSession`s) | `python/packages/core/agent_framework/_harness/_background_agents.py:240-298` |
| Background task start tool — fresh child session + asyncio.create_task | `sub_session = bg_agent.create_session()` then `asyncio.create_task(_run_agent(bg_agent.run(input, session=sub_session)))` | `python/packages/core/agent_framework/_harness/_background_agents.py:316-346` |
| Concurrent first-completion wait | `await asyncio.wait([t for _, t in waitable], return_when=asyncio.FIRST_COMPLETED)` | `python/packages/core/agent_framework/_harness/_background_agents.py:376-379` |
| Background error capture | `exception = completed_task.exception(); if exception is not None: task_info.status = BackgroundTaskStatus.FAILED; task_info.error_text = str(exception)` | `python/packages/core/agent_framework/_harness/_background_agents.py:187-204` |
| LOST status for orphaned tasks | If `in_flight` task is gone from runtime state, mark `LOST` | `python/packages/core/agent_framework/_harness/_background_agents.py:218-221` |
| Background error surfacing | `get_task_results` returns `"Task failed: {error_text or 'Unknown error'}"` (string, not exception) | `python/packages/core/agent_framework/_harness/_background_agents.py:411-417` |
| BackgroundAgentsProvider is experimental | `@experimental(feature_id=ExperimentalFeature.HARNESS)` | `python/packages/core/agent_framework/_harness/_background_agents.py:51,240` |
| AgentExecutor — wrapping agent as a workflow node | `class AgentExecutor(Executor)` invokes `self._agent.run(self._cache, stream=..., session=self._session, ...)` | `python/packages/core/agent_framework/_workflows/_agent_executor.py:119-431` |
| Per-executor session ownership | `self._session = session or self._agent.create_session()` (executor-owned AgentSession, distinct from caller's session) | `python/packages/core/agent_framework/_workflows/_agent_executor.py:169` |
| Per-agent invoke kwargs routing | `_resolve_executor_kwargs` uses `GLOBAL_KWARGS_KEY` sentinel for shared, executor id for per-agent | `python/packages/core/agent_framework/_workflows/_agent_executor.py:571-601` |
| Workflow-level kwargs plumbing for subagents | `_run_core` stores `WORKFLOW_RUN_KWARGS_KEY`; per-executor resolution at `AgentExecutor._prepare_agent_run_args` | `python/packages/core/agent_framework/_workflows/_workflow.py:557-567`; `python/packages/core/agent_framework/_workflows/_agent_executor.py:548-569` |
| WorkflowAgent wrapping a workflow as an agent | `class WorkflowAgent(BaseAgent)` exposes `run(...)` that drives `self.workflow.run(...)` | `python/packages/core/agent_framework/_workflows/_agent.py:52-479` |
| WorkflowAgent.run kwargs pass-through to subagents | Docstring: `function_invocation_kwargs: ... for agent name or agent executor id to kwargs, or a flat mapping of kwargs for all tool invocations` | `python/packages/core/agent_framework/_workflows/_agent.py:186-192,734-738` |
| WorkflowExecutor (sub-workflow wrapping) | `class WorkflowExecutor(Executor)` with `_execution_contexts` dict keyed by `execution_id` for per-call isolation | `python/packages/core/agent_framework/_workflows/_workflow_executor.py:103-302` |
| Concurrent sub-workflow invocations | `Each invocation gets its own execution context`; concurrent via `asyncio.gather` in `_process_workflow_result` | `python/packages/core/agent_framework/_workflows/_workflow_executor.py:197-224,568,583,691-693` |
| Sub-workflow request/response wire | `SubWorkflowRequestMessage(source_event, executor_id)` → parent → `SubWorkflowResponseMessage(data, source_event)` → child | `python/packages/core/agent_framework/_workflows/_workflow_executor.py:57-100,587-601,647-697` |
| Sub-workflow error bubbling | On `WorkflowRunState.FAILED` the executor finds the `failed` event and emits `WorkflowEvent.error(exception)` to the parent | `python/packages/core/agent_framework/_workflows/_workflow_executor.py:607-621` |
| Sub-workflow kwargs propagation | `parent_kwargs = ctx.get_state(WORKFLOW_RUN_KWARGS_KEY, {})`; unwraps `__global__` and forwards `fi_kwargs`/`ci_kwargs` to `self.workflow.run(...)` | `python/packages/core/agent_framework/_workflows/_workflow_executor.py:388-412` |
| Sub-workflow checkpoint save/restore | `on_checkpoint_save`/`on_checkpoint_restore` serialize `_execution_contexts` and `_request_to_execution` | `python/packages/core/agent_framework/_workflows/_workflow_executor.py:473-535` |
| Handoff synthetic tool injection | `_apply_auto_tools` adds `handoff_to_<target>` `FunctionTool`s with `approval_mode="never_require"` and empty body that is intercepted by middleware | `python/packages/orchestrations/agent_framework_orchestrations/_handoff.py:303-347` |
| Handoff middleware short-circuit | `_AutoHandoffMiddleware` injects `{HANDOFF_FUNCTION_RESULT_KEY: target_id}` and `raise MiddlewareTermination(result=...)` | `python/packages/orchestrations/agent_framework_orchestrations/_handoff.py:130-152` |
| Handoff routing — real executor handoff, not tool-call | `_run_agent_and_emit` sends `AgentExecutorRequest(messages=[], should_respond=True)` to `target_id=handoff_target` and emits `WorkflowEvent("handoff_sent", data=HandoffSentEvent(...))` | `python/packages/orchestrations/agent_framework_orchestrations/_handoff.py:395-416` |
| Handoff broadcasting | `_broadcast_messages` fans out to all participants via `ctx.send_message(AgentExecutorRequest(messages, should_respond=False))` | `python/packages/orchestrations/agent_framework_orchestrations/_handoff.py:471-482` |
| Handoff user-input request when no handoff and not autonomous | `await ctx.request_info(HandoffAgentUserRequest(response), list[Message])` | `python/packages/orchestrations/agent_framework_orchestrations/_handoff.py:436` |
| MagenticAgentExecutor reset handler | `handle_magentic_reset` clears cache, full_conversation, pending requests, and reassigns `self._agent_thread = self._agent.create_session()` | `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:1348-1367` |
| Magentic builder wraps participants in MagenticAgentExecutor | `if isinstance(participant, SupportsAgentRun): executors.append(MagenticAgentExecutor(participant))` | `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:1762-1768` |
| Magentic agent invocation by orchestrator | `_send_request_to_participant` sends `AgentExecutorRequest(messages, should_respond=True)` and emits `WorkflowEvent("group_chat", data=GroupChatRequestSentEvent(...))` | `python/packages/orchestrations/agent_framework_orchestrations/_base_group_chat_orchestrator.py:443-495` |
| Group chat selection function -> next speaker | `_invoke_agent` runs manager agent with `response_format=AgentOrchestrationOutput` and parses `next_speaker` | `python/packages/orchestrations/agent_framework_orchestrations/_group_chat.py:487-499` |
| SequentialBuilder participants | Each `SupportsAgentRun` wrapped in `AgentExecutor` (or `AgentApprovalExecutor` if request-info enabled); last is default workflow output | `python/packages/orchestrations/agent_framework_orchestrations/_sequential.py:187-265` |
| Agent invoke span — chat-span parent | `_trace_agent_invocation` emits `OtelAttr.AGENT_INVOKE_OPERATION = "invoke_agent"` span and `_capture_current_agent_system_instructions` checks `chat_parent.span_id == agent_context.span_id` to confirm chat span is parented under agent invoke | `python/packages/core/agent_framework/observability.py:297,1743-1842,2297-2321` |
| Workflow run span | `OtelAttr.WORKFLOW_RUN_SPAN = "workflow.run"`, used by `Workflow._run_iteration_impl` and `_functional.FunctionWorkflow.run` | `python/packages/core/agent_framework/observability.py:254`; `python/packages/core/agent_framework/_workflows/_workflow.py:519-522`; `python/packages/core/agent_framework/_workflows/_functional.py:971` |
| Message trace-context propagation | `ctx.send_message` injects W3C `traceparent` and records `source_span_ids` on `WorkflowMessage` | `python/packages/core/agent_framework/_workflows/_workflow_context.py:316-336` |
| `SubWorkflowRequestMessage` data class | `source_event: WorkflowEvent; executor_id: str; create_response(data)` returns `SubWorkflowResponseMessage` after `is_instance_of` validation | `python/packages/core/agent_framework/_workflows/_workflow_executor.py:72-100` |
| Session isolation: create_session / get_session | `BaseAgent.create_session` and `get_session` produce `AgentSession(session_id, service_session_id)`; each call without args returns a fresh session | `python/packages/core/agent_framework/_agents.py:411-449` |
| Per-call chat client middleware / chat_client integration | `_trace_agent_invocation` wraps `execute()` callable and the chat-client span is created as a child of the agent-invoke span via `trace.get_current_span()` | `python/packages/core/agent_framework/observability.py:1491-1492,1753,1779` |
| Test: nested delegation kwargs propagate A→B→C | `test_as_tool_nested_delegation_propagates_kwargs` simulates B calling C via `as_tool` and asserts kwargs reach C | `python/packages/core/tests/core/test_as_tool_kwargs_propagation.py:111-172` |
| Test: UserInputRequiredException from as_tool | `test_as_tool_raises_on_user_input_request` verifies exception propagates with `consent_link` | `python/packages/core/tests/core/test_agents.py:2667-2686` |
| Test: background agents failure capture | `test_harness_background_agents.py` covers `should_fail=True` agent path and `BackgroundTaskStatus.FAILED` recording | `python/packages/core/tests/core/test_harness_background_agents.py:1-538` |
| Test: workflow-as-agent kwargs propagation | `test_workflow_as_agent_run_propagates_kwargs_to_underlying_agent` and `test_subworkflow_kwargs_propagation` and `test_nested_subworkflow_kwargs_propagation` (3 levels deep) | `python/packages/core/tests/workflow/test_workflow_kwargs.py:566-786` |
| Test: sub-workflow concurrent invocations | `test_concurrent_sub_workflow_invocations` exercises the per-execution-id isolation path | `python/packages/core/tests/workflow/test_sub_workflow.py:362-477` |
| Test: sub-workflow intermediate bubbling | `test_subworkflow_intermediate_event_carries_executor_id` regression guard for `WorkflowExecutor` source attribution | `python/packages/core/tests/workflow/test_sub_workflow.py:619-691` |
| Test: handoff middleware short-circuits | `test_auto_handoff_middleware_intercepts_handoff_tool_call` verifies `MiddlewareTermination` is raised | `python/packages/orchestrations/tests/test_handoff.py:1115-1133` |
| Test: handoff resume with approval | `test_tool_approval_responses_are_not_replayed_from_history` and `test_simple_handoff_workflow_with_approval_request` | `python/packages/orchestrations/tests/test_handoff.py:318-396,1350-1435` |
| Test: Magentic as_agent does not accept conversation | `test_magentic_as_agent_does_not_accept_conversation` | `python/packages/orchestrations/tests/test_magentic.py:270-282` |
| Test: edge fan-out/fan-in trace context propagation | `test_single_edge_group_tracing_success`, `test_fan_out_edge_group_tracing_success` verify trace_id/span_id linking via span links | `python/packages/core/tests/workflow/test_edge.py:306-355,780-832` |
| Test: workflow spans build/run/lifecycle | `test_span_creation_and_attributes`, `test_end_to_end_workflow_tracing` cover span kinds (workflow, processing, sending) | `python/packages/core/tests/workflow/test_workflow_observability.py:101-300` |

## Answers to Dimension Questions

1. **Can agents delegate?** Yes, via three mechanisms:
   - In-tool: `agent.as_tool()` (`_agents.py:485-579`)
   - In-background: `BackgroundAgentsProvider` injects 6 background-task tools (`_harness/_background_agents.py:316-495`)
   - In-workflow: `AgentExecutor` wraps an `Agent` as a workflow node (`_workflows/_agent_executor.py:119-602`); a `WorkflowExecutor` wraps an entire `Workflow` as a node (`_workflows/_workflow_executor.py:103-697`); handoff/group-chat/magentic/sequential/concurrent orchestrations wrap multiple agents in a `Workflow` graph (`packages/orchestrations/agent_framework_orchestrations/`).

2. **Is delegation just a tool call or a real child run?**
   - `as_tool()` is a real `self.run(...)` invocation (`_agents.py:559-567`) — a real child run inside the parent's span via OTel context propagation, not a synthetic stub.
   - `BackgroundAgentsProvider.start_task` is a real `asyncio.create_task(_run_agent(bg_agent.run(input, session=sub_session)))` (`_harness/_background_agents.py:341`) — a true child run in a separate asyncio task.
   - `AgentExecutor._run_agent(...)` is a real `self._agent.run(...)` (`_workflows/_agent_executor.py:425-431`) — a true child run inside the executor's session, fired from within the workflow runner's loop.
   - `WorkflowExecutor.process_workflow` is a real `await self.workflow.run(...)` (`_workflows/_workflow_executor.py:408-412`) — a true nested workflow run.
   - Handoff routing is *not* a tool call: `_AutoHandoffMiddleware` short-circuits the tool call with `MiddlewareTermination` (`_handoff.py:152`), but the actual delegation is a workflow graph edge send (`ctx.send_message(AgentExecutorRequest(...), target_id=handoff_target)`, `_handoff.py:408-411`) — a sibling executor receives control.

3. **Is child state isolated?**
   - `as_tool()`: default `propagate_session=False` creates a fresh `AgentSession` for the child; with `propagate_session=True` the parent's session is reused (`_agents.py:494,562`).
   - `BackgroundAgentsProvider`: each background task gets its own `sub_session = bg_agent.create_session()` (`_harness/_background_agents.py:338`), tracked in `_runtime.background_sessions[task_id]` so it can be re-used on `continue_task` (`_harness/_background_agents.py:454-456`).
   - `AgentExecutor`: `self._session = session or self._agent.create_session()` (`_workflows/_agent_executor.py:169`); the executor owns its own session, separate from the workflow's overall session.
   - `WorkflowExecutor`: per-execution isolation via `_execution_contexts[execution_id]` (`_workflows/_workflow_executor.py:299`) — concurrent invocations of the same `WorkflowExecutor` get independent `ExecutionContext` instances.
   - `MagenticAgentExecutor`: child session is *replaced* on replan via `handle_magentic_reset` (`_magentic.py:1366`), confirming that the framework treats Magentic child state as private and disposable.

4. **Are child traces linked to parent traces?** Yes, in two complementary ways:
   - **In-process (sync) child**: when a parent agent invokes `agent.run(...)` directly (as in `as_tool`), the child `_trace_agent_invocation` starts an `invoke_agent` span. OTel context propagation makes the child's `invoke_agent` span a child of the parent's `invoke_agent` span (`observability.py:1743-1842`). The chat-client spans inside the child are confirmed parented to the child agent span via `_capture_current_agent_system_instructions` (`observability.py:2297-2321`).
   - **Workflow-graph child**: `WorkflowMessage.trace_contexts` and `source_span_ids` are propagated through `ctx.send_message` (`_workflows/_workflow_context.py:316-336`) and linked into edge-processing and processing spans (`_workflows/_edge_runner.py:86-365`). Tests assert `span.links[0].context.trace_id == int(...)` (`tests/workflow/test_edge.py:352-355`).

5. **Can child loops run in parallel?**
   - `BackgroundAgentsProvider`: yes — children run as `asyncio.Task`s; multiple can be in flight simultaneously, and `wait_for_first_completion` uses `asyncio.wait(..., return_when=asyncio.FIRST_COMPLETED)` to await any of them (`_harness/_background_agents.py:376-379`). The provider's instructions explicitly say "Creating a background task does not block, and background tasks run concurrently" (`_harness/_background_agents.py:32-33`).
   - `WorkflowExecutor`: yes — concurrent invocations of the same `WorkflowExecutor` are fully isolated per execution_id (`_workflow_executor.py:197-224`); `_process_workflow_result` uses `asyncio.gather` for fan-out of sub-workflow outputs (`_workflow_executor.py:568,583`).
   - Handoff / Magentic / Sequential / Concurrent orchestrations: run inside a single `Workflow.run(...)` which uses its own async scheduler (`_runner.py`, `_workflow.py`); fan-out edges can deliver to multiple participants concurrently (verified by `test_fan_out_edge_group_tracing_success`, `tests/workflow/test_edge.py:780-832`).
   - `as_tool()`: no — synchronous, blocks the parent's tool-call loop until the child finishes.

## Architectural Decisions

- **Three distinct delegation surfaces** are exposed: a tool-call surface (`as_tool()`), a background-task surface (`BackgroundAgentsProvider`), and a workflow-graph surface (`AgentExecutor`/`WorkflowExecutor` + orchestrations). This separation reflects an explicit design choice that **inline delegation** (within one tool-call loop), **fire-and-forget delegation** (out-of-loop with eventual wait), and **structured delegation** (graph-based, peer executors) have different correctness and observability profiles.

- **Session ownership defaults to fresh-isolate**: `as_tool()` uses a fresh `AgentSession` unless `propagate_session=True` (`_agents.py:494,562`); `BackgroundAgentsProvider` always creates a fresh `sub_session` (`_harness/_background_agents.py:338`); `AgentExecutor` creates its own session per executor (`_workflows/_agent_executor.py:169`). Shared sessions are an opt-in.

- **Error bubbling is typed at the surface boundary**: `UserInputRequiredException` is the only formally-typed exception for `as_tool` user-input propagation (`exceptions.py:184-209`, `_agents.py:568-569`). Other child exceptions during `as_tool` propagate as standard `ToolException`/`AgentException` paths via the chat-client/function-invocation layer. `BackgroundAgentsProvider` never raises; failures become `BackgroundTaskStatus.FAILED` strings (`_harness/_background_agents.py:411-413`). `WorkflowExecutor` converts sub-workflow `failed` events into `WorkflowEvent.error(exception)` (`_workflow_executor.py:607-621`).

- **Per-run kwargs with `GLOBAL_KWARGS_KEY` sentinel**: `Workflow.run(...)` accepts `function_invocation_kwargs`/`client_kwargs` (flat mapping or per-executor mapping), stored in workflow `State` under `WORKFLOW_RUN_KWARGS_KEY`, and resolved by `AgentExecutor._resolve_executor_kwargs` against the executor id (`_workflows/_workflow.py:557-567`; `_workflows/_agent_executor.py:548-601`). Per-executor dicts use the executor id as the key; `__global__` applies to all. This is the canonical mechanism for a parent to inject runtime context (auth tokens, trace ids) into every nested agent.

- **OTel context-based trace linking instead of explicit parent IDs**: child `invoke_agent` spans become children of the parent's `invoke_agent` span automatically via OTel's context propagation (`observability.py:1743-2014`). Workflow-graph messages carry `trace_contexts` and `source_span_ids` as W3C `traceparent` headers (`_workflows/_workflow_context.py:316-336`). This means **trace correlation is structural** (who is on the call stack when) rather than explicit (no `parent_run_id` field on `AgentResponse`).

- **Handoff as workflow routing, not tool-call delegation**: `_AutoHandoffMiddleware` short-circuits the synthetic `handoff_to_<target>` tool (`_handoff.py:130-152`) but the actual delegation is `ctx.send_message(AgentExecutorRequest(...), target_id=...)` (`_handoff.py:408-411`) and `handoff_sent` events are emitted for observability (`_handoff.py:412-414`). This is deliberately modeled as **agent-to-agent routing** rather than as a nested tool call.

- **Magentic explicitly resets child state on replan**: `MagenticAgentExecutor.handle_magentic_reset` clears cache, full conversation, pending requests, and creates a new `AgentSession` (`_magentic.py:1348-1367`). This is the framework's acknowledgement that a Magentic agent-as-tool would be wrong here; children should start fresh after replanning.

## Notable Patterns

- **`BackgroundAgentsProvider` as the explicit concurrency primitive**: parents call `background_agents_start_task(...)` (returns a task id immediately), then `background_agents_wait_for_first_completion([ids])` (blocks parent LLM until any completes), then `background_agents_get_task_results(id)` (returns text). This pattern of *model-orchestrated async work* is the closest analog to Anthropic-style "agent-as-tool-with-fork" patterns. The state is persisted in `session.state["background_agents"]` so the parent can leave and resume (`_harness/_background_agents.py:162-174`).

- **Workflow-as-Agent (`WorkflowAgent`)** (`_workflows/_agent.py:52-922`): a `Workflow` is wrapped as a `BaseAgent` so it can be `as_agent()`-ed or `as_tool()`-ed like any other agent. Workflow events with `type='output'` and `type='request_info'` are converted to agent-level messages and approvals (`_workflows/_agent.py:483-714`). This means delegation chains can compose workflows inside workflows inside tools without any special-case glue.

- **`SubWorkflowRequestMessage` / `SubWorkflowResponseMessage` envelopes** (`_workflows/_workflow_executor.py:57-100`): the boundary between child and parent workflows is a typed envelope, not an opaque channel. `create_response` validates that the response data matches the expected type via `is_instance_of` (`_workflow_executor.py:88-99`). This makes parent intercept/forward predictable.

- **`is_handoff_requested` content sniffing** (`_handoff.py:484-529`): handoff detection reads the last `function_result` content from the response, parses the JSON payload, looks for the `HANDOFF_FUNCTION_RESULT_KEY = "handoff_to"` key, and extracts the target id. This makes handoff signaling a structured contract between the model output and the framework's routing.

- **Context provider "before_run" injection of delegation tools**: `BackgroundAgentsProvider.before_run` defines the 6 tools inside the method closure using `self` capture and registers them via `context.extend_tools(self.source_id, [...])` (`_harness/_background_agents.py:498-509`). This means the same provider can be attached to any agent without wiring.

- **ToolApprovalMiddleware-aware mixed batches**: when a session is present, approval requests for non-approval-required tools in the same batch are treated as approved, hidden, and stored in session state keyed to the visible request ids, then reinjected when the visible flow resumes (per the core `AGENTS.md` description).

## Tradeoffs

- **Tool-call delegation is synchronous** (`as_tool()` blocks the parent's LLM iteration). This is good for correctness (parent sees the result before its next turn) but means `as_tool` cannot be used for genuinely parallel work — `BackgroundAgentsProvider` is the answer for that, at the cost of more boilerplate and no automatic cancellation.

- **`as_tool()` returns a `str` text** (`_agents.py:571`: `return final_response.text`). Structured content (function calls, tool results) emitted by the child is collapsed into text for the parent. Multi-modal content does not propagate via this path; only via the streaming variant if a `stream_callback` is provided (and even then the callback receives the `AgentResponseUpdate`, but the parent's model only sees the final `text`).

- **`propagate_session=True` couples parent and child memory** (`_agents.py:506-508`). Two agents writing to the same session can race on writes or see interleaved tool calls; the framework does not detect or warn about this.

- **`BackgroundAgentsProvider` children cannot be cancelled externally** — only through event-loop shutdown, asyncio task cancellation, or task completion/timeout via `_finalize_task` (`_harness/_background_agents.py:187-204`). There is no `background_agents_cancel_task` tool.

- **`MagenticAgentExecutor` reset drops session continuity** (`_magentic.py:1366`). Any service-side conversation id held by the old session is lost; the next message will start a new one. This is the explicit design (the docstring says "Magentic pattern requires a reset operation upon replanning", `_magentic.py:1341-1344`) but it has real cost for hosted chat clients.

- **Handoff requires `Agent` (not `SupportsAgentRun`) participants** (`_handoff.py:695-700`): `Participants must be Agent instances. Got {type(participant).__name__}. Handoff workflows require Agent because they rely on cloning, tool injection, and middleware capabilities.` This restricts handoff to the higher-level `Agent` API.

- **Sub-workflow request forwarding is opt-in via `propagate_request=True`** (`_workflow_executor.py:285-289`). Default behavior is to wrap requests in `SubWorkflowRequestMessage` so a parent executor can intercept them, which adds a layer of indirection compared to a flat `request_info` propagation.

- **Magentic accepts only single-task input** (`_magentic.py:923-924`): `ValueError("Magentic only support a single task message to start the workflow.")`. This makes Magentic ill-suited as a downstream workflow node in a chain unless the upstream reduces to one message.

- **Child traces have no explicit `parent_run_id` attribute on `AgentResponse`**. OTel context propagation handles correlation, but if a downstream consumer wants to attribute a child response to a parent by id, they must read the OTel trace or rely on `service_session_id` (which only links to the chat-client conversation, not the parent agent).

## Failure Modes / Edge Cases

- **Sub-agent mid-tool-call user-input requests**: `as_tool()` raises `UserInputRequiredException` carrying the `user_input_request` Content items (`_agents.py:568-569`); the test `test_user_input_request_propagates_through_as_tool` (`test_function_invocation_logic.py:3891-3930`) verifies propagation through the function-invocation layer.

- **Sub-agent exceptions during streaming `as_tool`**: any other exception is propagated through `FunctionTool.parse_result` → middleware → the parent chat client's error handling. There is no special-case for `as_tool`-wrapped sub-agent failures; they appear as tool errors in the parent's chat response.

- **`BackgroundAgentsProvider` orphaned tasks on provider loss**: `_refresh_task_state` marks tasks `LOST` if their `in_flight` reference is gone (`_harness/_background_agents.py:218-222`). This is the failure mode when the provider instance is recreated mid-run; the docstring explicitly notes "Runtime state (in-flight asyncio.Task objects, child AgentSession handles) is inherently non-serializable and cannot survive process restarts" (`_harness/_background_agents.py:288-292`).

- **`BackgroundAgentsProvider` cancellation**: if the parent agent's run is cancelled mid-task, the underlying `asyncio.Task` is cancelled; `_finalize_task` records `task_info.status = FAILED; task_info.error_text = "Task was canceled."` (`_harness/_background_agents.py:193-195`).

- **`WorkflowExecutor` failure of sub-workflow**: `WorkflowRunState.FAILED` triggers `WorkflowEvent.error(exception)` to the parent with `Sub-workflow {self.workflow.id} failed with error: {error_type} - {error_message}` (`_workflow_executor.py:607-621`). The parent's reaction depends on its own edge wiring.

- **Handoff to unknown target**: `ValueError("Agent '{id}' attempted to handoff to unknown target '{handoff_target}'. Valid targets are: ...")` (`_handoff.py:397-401`).

- **Duplicate handoff tool name**: `ValueError("Agent '{id}' already has a tool named '{name}'. Handoff tool name conflicts with existing tool.")` (`_handoff.py:319-324`).

- **`as_tool` agent without name**: `ValueError("Agent tool name cannot be None. Either provide a name parameter or set the agent's name.")` (`_agents.py:535-536`).

- **Magentic replan cap**: `max_reset_count` limits how many times the orchestrator can issue a `MagenticResetSignal` (`_magentic.py:1415, 1416`); once exceeded, the workflow likely surfaces a failure rather than continuing.

- **Workflow restart in invalid state**: `_run_core` raises `AgentException("The underlying workflow is in an invalid state to restart: {final_state}")` if `WorkflowRunState` is neither `IDLE_WITH_PENDING_REQUESTS` nor `IDLE` (`_workflows/_agent.py:478-479`).

## Future Considerations

- **Cancellation API for background tasks**: a `background_agents_cancel_task` tool would close the loop on `BackgroundTaskStatus.RUNNING` tasks and make shutdown semantics explicit.

- **Explicit `parent_run_id` / `delegated_by` field on `AgentResponse`**: would make child attribution cheap without depending on OTel spans.

- **Single-sub-agent test for nested `invoke_agent` spans**: the existing tests assert kwargs propagation but not span-tree shape. A test like `test_as_tool_creates_child_agent_invoke_span` would lock in the parent-child trace contract.

- **Asynchronous `as_tool()` variant**: today, delegating to a long-running sub-agent via `as_tool()` blocks the parent. An `as_background_tool()` (or `propagate_async=True`) that returns a `Task` would unify the in-tool and in-background surfaces.

- **Sub-workflow request routing without envelopes**: a flat `propagate_request=True` only propagates *one* layer (`_workflow_executor.py:285-289`). Multi-level sub-workflows need explicit envelope unwrapping at each level.

- **Magentic session-resume cross-replan**: the current `handle_magentic_reset` resets the session entirely (`_magentic.py:1366`); preserving a checkpoint handle so the next plan could resume the prior session would let Magentic benefit from conversation history across replans.

- **Cancellation propagation for `WorkflowExecutor`**: today, cancelling the parent does not necessarily cancel a sub-workflow in flight (`_workflow_executor.py:691-693` calls `self.workflow.run(responses=...)` synchronously without wiring cancellation tokens).

- **A `BackgroundAgentsProvider` "list in flight + continue" UX**: the existing tools cover this, but a higher-level "wait for all" or "wait for any" tool with timeout would reduce parent LLM round-trips.

## Questions / Gaps

- **Where does the `service_session_id` get linked for cross-agent chat-client conversations?** `BaseAgent.create_session` (`_agents.py:411-432`) creates a session; `get_session` (`_agents.py:434-449`) creates one with a known service id. But I did not find evidence that `as_tool()`'s child agent *shares* its `service_session_id` with the parent when `propagate_session=True`, only that the `AgentSession` object is reused. Confirming this would clarify whether service-side thread continuity actually follows.

- **How does `_trace_agent_invocation` behave when `OBSERVABILITY_SETTINGS.ENABLED` is `False`?** Returns `execute()` directly (`observability.py:1757-1758`), so there is no agent-invoke span at all. Child attribution is then lost entirely — no spans to walk up.

- **What happens to `service_session_id` when a `MagenticAgentExecutor` resets?** The reset calls `self._agent.create_session()` (`_magentic.py:1366`), which by default returns a fresh `AgentSession` with no `service_session_id`. So the next chat-client call must allocate a new service-side conversation. This is correct but worth documenting.

- **Does the `BackgroundAgentsProvider` ever surface partial streaming output to the parent?** Today, only `result_text: str | None` is stored (`_harness/_background_agents.py:62`), so streaming progress is invisible to the parent until the task completes. A `background_agents_get_task_stream` tool would help.

- **Does `as_tool()` mid-stream errors propagate as `UserInputRequiredException` or as a different exception?** The code path checks `final_response.user_input_requests` after `get_final_response()` (`_agents.py:567-569`), so streaming errors that occur before that point would surface through the `tool.invoke` pipeline as standard exceptions. The intent is clear from the test (`test_as_tool_raises_on_user_input_request`, `test_agents.py:2667-2686`) but other mid-stream error classes are undocumented.

---

Generated by `studies/agent-harness-study/03.08-subagent-forked-loop-design` against `agent-framework`.
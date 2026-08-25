# Source Analysis: langgraph

## 03.02 Reason/Act/Observe Cadence

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (3.10+), core `langgraph` package plus `langgraph-prebuilt` and `langgraph-checkpoint*` siblings in a monorepo under `libs/` |
| Analyzed | 2026-07-13 |

## Summary

LangGraph does **not** run its own Reason/Act/Observe loop engine. The cadence is the graph itself: a `StateGraph` whose two nodes (`agent` → `tools`) form the loop, wired together by a conditional edge that inspects `AIMessage.tool_calls` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:831-860`). Reasoning happens inside the chat-model step (i.e. inside `model.invoke` / `model.ainvoke`), and LangGraph never parses free-text "Thought:" / "Action:" / "Observation:" markers — it relies on the structured `tool_calls` field on the model's `AIMessage`.

Action extraction lives in `ToolNode._parse_input` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1224-1266`), which pulls `tool_calls` off the latest `AIMessage` (`tool_node.py:1258-1265`) and parallelizes them via `executor.map` (`tool_node.py:821-823`). Observations are written back as `ToolMessage` objects bound by `tool_call_id` (`tool_node.py:1007-1012`, `tool_node.py:1154-1159`), so the next model step always sees a fully-formed chat-history with paired request/result messages.

There is **no built-in compaction** of observations. Long histories are surfaced to the user via the optional `pre_model_hook` slot on `create_react_agent` (`chat_agent_executor.py:396-413`), which can overwrite `state["messages"]` with `RemoveMessage` or replace `state["llm_input_messages"]` for the next step. There is no summarizer middleware inside LangGraph itself. Reasoning is **hidden by default** — there is no `Thought` channel, no scratchpad buffer, and no separation of chain-of-thought from tool-call decisions. The framework stores nothing beyond what the model returned (`AIMessage` content, `additional_kwargs`, `response_metadata`, `usage_metadata`); when a model emits reasoning text it travels as ordinary `AIMessage.content`.

Failure feedback is one of the better-developed parts of the system. `ToolNode` defaults to a callable error handler (`tool_node.py:753`) that captures `ToolInvocationError` (Pydantic validation failures, `tool_node.py:957-966`) and converts it into an `error`-status `ToolMessage` (`tool_node.py:1007-1012`). The error's content is filtered so that InjectedState / InjectedStore / ToolRuntime argument values never leak into the message the model sees (`tool_node.py:510-563`, `tool_node.py:1421-1429`). Validation that "every `AIMessage.tool_call` has a matching `ToolMessage`" runs before the model is invoked again (`chat_agent_executor.py:243-271`, `chat_agent_executor.py:651`); mismatches raise `INVALID_CHAT_HISTORY`. The framework also supports streaming partial tool output via a separate `tools` channel and a `ToolCallStream` projection (`libs/prebuilt/langgraph/prebuilt/_tool_call_stream.py:17-117`, `libs/langgraph/langgraph/pregel/_tools.py:35-268`).

A loop-safety brake lives in the `AgentState.remaining_steps` managed value: when `remaining_steps < 2` and the agent emitted `tool_calls`, `_are_more_steps_needed` (`chat_agent_executor.py:620-634`) returns `True` and `call_model` substitutes a final "Sorry, need more steps to process this request." `AIMessage` (`chat_agent_executor.py:684-692`), instead of looping forever or letting `recursion_limit` abort the run. The pregel engine still enforces a hard `recursion_limit` cap (`libs/langgraph/langgraph/pregel/main.py:3020-3022`).

The framework is decoupled from any single ReAct textual convention — it ships **two different tool-dispatch strategies** (`v1` for a single bulk call vs `v2` for fan-out via `Send` per `tool_call`, `chat_agent_executor.py:843-859`) and lets the user interpose hooks (`pre_model_hook`, `post_model_hook`) at every model invocation (`chat_agent_executor.py:874-963`). This makes the cadence deliberately observable and rewritable but pushes parsing, compaction, and prompt shaping to the caller.

## Rating

**7 / 10** — The Reason/Act/Observe cadence is implemented as an explicit `StateGraph` cycle with strong, tested guarantees: (a) structured `tool_calls` parsing (no fragile text parsing), (b) tool execution parallelization with `executor.map`, (c) typed `ToolMessage` observations with `tool_call_id` correlation, (d) a configurable `handle_tool_errors` policy that converts errors into observations the next model call will see, (e) Pydantic-validation-error filtering that hides InjectedState/Store/Runtime values, (f) chat-history validation before each model call, (g) partial-output streaming on a dedicated `tools` channel with a first-class `ToolCallStream` projection, and (h) a managed-value `remaining_steps` guardrail. Reasoning is intentionally **not** first-class — there is no scratchpad, no `Thought` message type, and no automatic compaction. The framework surfaces compaction through `pre_model_hook` / `llm_input_messages`, leaving the user to implement it. The loop also has no concept of "failed observation → re-plan with feedback": an error becomes a regular `ToolMessage(status="error")` and the model is free to repeat the same call, which is the documented intended behavior. Documentation is partly in code (long docstrings), partly in the deprecated `create_react_agent` (now superseded by `langchain.agents.create_agent` per `chat_agent_executor.py:53-56, 274-277`), with tests in `libs/prebuilt/tests/test_tool_node.py` (2430 lines), `test_react_agent.py` (2156 lines), `test_tool_call_transformer.py` (427 lines), and `test_on_tool_call.py` covering error handling, parallel fan-out, validation filtering, and streaming.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| ReAct factory (graph-as-loop) | `create_react_agent` builds `agent → tools` cycle | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:278-516` |
| Conditional edge: tools vs end | `should_continue` reads `last_message.tool_calls` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:831-859` |
| ReAct dispatch variants (v1 bulk vs v2 fan-out) | `version` branch returns `"tools"` or `[Send(...)]` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:843-859` |
| Tool-call extraction (no text parsing) | `_parse_input` walks `reversed(messages)` for `AIMessage.tool_calls` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1224-1266` |
| Parallel tool execution | `executor.map(self._run_one, ...)` over tool calls | `libs/prebuilt/langgraph/prebuilt/tool_node.py:821-823` |
| Async parallel tool execution | `asyncio.gather(*coros)` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:855-860` |
| Observation as `ToolMessage` bound by `tool_call_id` | `_execute_tool_sync` builds success ToolMessage | `libs/prebuilt/langgraph/prebuilt/tool_node.py:922-971` |
| Error → `ToolMessage(status="error")` | caught exception routed to handler, returned as error ToolMessage | `libs/prebuilt/langgraph/prebuilt/tool_node.py:982-1012` |
| Same for async | `_execute_tool_async` mirrors error path | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1069-1159` |
| Default error handler | `_default_handle_tool_errors` catches `ToolInvocationError` only | `libs/prebuilt/langgraph/prebuilt/tool_node.py:383-391` |
| Configurable error policy | `handle_tool_errors` accepts bool/str/callable/exception-type/tuple | `libs/prebuilt/langgraph/prebuilt/tool_node.py:749-754`, `tool_node.py:394-441` |
| Pydantic validation error filter | `_filter_validation_errors` strips InjectedState/Store/Runtime keys | `libs/prebuilt/langgraph/prebuilt/tool_node.py:510-563` |
| Defensive: LLM cannot forge InjectedState | `stripped_args` overwrites caller-supplied injected values | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1421-1429` |
| Validation error → typed exception | `ToolInvocationError` raised with filtered errors | `libs/prebuilt/langgraph/prebuilt/tool_node.py:339-380`, `tool_node.py:957-966` |
| Unknown tool name → observation | `_validate_tool_call` returns error ToolMessage | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1268-1279` |
| Error-template content (what the model sees) | `TOOL_CALL_ERROR_TEMPLATE` / `TOOL_INVOCATION_ERROR_TEMPLATE` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:108-121` |
| Chat-history invariant check before each model call | `_validate_chat_history` raises `INVALID_CHAT_HISTORY` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:243-271` |
| `_validate_chat_history` is invoked in `_get_model_input_state` | guards the ReAct loop against orphaned `tool_calls` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:651` |
| Loop guardrail — soft | `_are_more_steps_needed` substitutes a final AI message | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:620-634` |
| Loop guardrail — final AI message | `"Sorry, need more steps to process this request."` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:684-692`, `chat_agent_executor.py:711-719` |
| Loop guardrail — hard | Pregel enforces `recursion_limit` | `libs/langgraph/langgraph/pregel/main.py:3020-3022`, `pregel/main.py:3501-3503` |
| `remaining_steps` is a managed value | `RemainingStepsManager.get(scratchpad)` | `libs/langgraph/langgraph/managed/is_last_step.py:18-23` |
| Tool runtime context injection | `ToolRuntime(state, context, config, store, stream_writer, tools, ...)` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1662-1730` |
| Tool-body partial-output stream emission | `ToolRuntime.emit_output_delta` reads ContextVar writer | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1732-1750` |
| Writer is installed by Pregel callback handler | `StreamToolCallHandler._start` sets `_tool_call_writer` | `libs/langgraph/langgraph/pregel/_tools.py:121-165` |
| Tool lifecycle events on `tools` channel | `tool-started` / `tool-output-delta` / `tool-finished` / `tool-error` | `libs/langgraph/langgraph/pregel/_tools.py:121-201` |
| Streaming handle (`ToolCallStream`) | `output_deltas`, `output`, `error`, `completed` | `libs/prebuilt/langgraph/prebuilt/_tool_call_stream.py:17-117` |
| Transformer projecting channel events into `run.tool_calls` | `ToolCallTransformer.process` | `libs/prebuilt/langgraph/prebuilt/_tool_call_transformer.py:44-150` |
| Compact projection for clients | `ToolCallTransformer._normalize_tool_output` flattens serialized ToolMessage | `libs/prebuilt/langgraph/prebuilt/_tool_call_transformer.py:34-41` |
| Conditional routing utility (`tools_condition`) | last-AIMessage tool_calls ⇒ `"tools"`, else `END` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1582-1659` |
| Per-`tool_call` fan-out via Send | `[Send("tools", ToolCallWithContext(...))]` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:849-859`, `tool_node.py:286-307` |
| Interceptors (wrap_tool_call / awrap_tool_call) | Hooked around each tool call, can short-circuit or retry | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1014-1067`, `tool_node.py:1161-1222` |
| Wrap interface (request + execute closure) | `ToolCallWrapper = Callable[[ToolCallRequest, Callable], ToolMessage | Command]` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:202-205` |
| Subgraph-as-tool pause propagation | `GraphBubbleUp` always re-raised through tool execution | `libs/prebuilt/langgraph/prebuilt/tool_node.py:87`, `tool_node.py:982-983`, `tool_node.py:1129-1130` |
| State injection to tools (state hidden from LLM) | `InjectedState` / `InjectedStore` / `ToolRuntime` markers | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1753-1901` |
| Tool-message content serialization fallback | `msg_content_output` JSON-encodes non-string returns | `libs/prebuilt/langgraph/prebuilt/tool_node.py:309-336` |
| ToolMessage status enum (`success` / `error`) | default is `success`, error path sets `status="error"` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1007-1012`, `tool_node.py:1154-1159` |
| Termination path: `return_direct` tools bypass next LLM call | `route_tool_responses` and `should_return_direct` set | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:597`, `chat_agent_executor.py:970-988` |
| Optional `pre_model_hook` (compaction/trimming slot) | user-supplied runnable may overwrite `messages` or set `llm_input_messages` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:396-413`, `chat_agent_executor.py:636-658` |
| Optional `post_model_hook` (HITL, guardrails slot) | runs after the LLM but before routing | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:425-432`, `chat_agent_executor.py:917-963` |
| Structured-response turn (post-loop) | `generate_structured_response` runs only after loop ends | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:744-786`, `chat_agent_executor.py:808-819` |
| Errors while streaming tools → stream closure | `_tool_call_writer.reset` swallows ValueError on context-mismatched end | `libs/langgraph/langgraph/pregel/_tools.py:213-222` |
| Parallel test verifies v1 vs v2 tool fan-out | `test_react_agent_parallel_tool_calls` | `libs/prebuilt/tests/test_react_agent.py:599-676` |
| Chat-history invariant test | `test__validate_messages` | `libs/prebuilt/tests/test_react_agent.py:371-433` |
| Error handling test matrix | `test_tool_node_error_handling` | `libs/prebuilt/tests/test_tool_node.py:319-369` |
| InjectedState cannot be forged by LLM | `test_tool_node_injected_state_overwrites_llm_value` | `libs/prebuilt/tests/test_tool_node.py:2208-2238` |
| Tool error event populates `ToolCallStream.error` | `test_tool_error_populates_error_field` | `libs/prebuilt/tests/test_tool_call_transformer.py:395` |
| `ValidationNode` (deprecated, schema-only) | validates `tool_calls`, returns ToolMessage with `additional_kwargs.is_error=True` | `libs/prebuilt/langgraph/prebuilt/tool_validator.py:184-221` |

## Answers to Dimension Questions

1. **How does the model decide actions?**
   The chat-model call is delegated to LangChain (`model.invoke` / `model.ainvoke` at `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:677,679,705,707`). When the model returns an `AIMessage` with non-empty `tool_calls`, `should_continue` (`chat_agent_executor.py:831-859`) routes the next step to the `tools` node. The decision is fully driven by the provider-parsed `tool_calls` field of the `AIMessage`; LangGraph never inspects free-text "Action:" markers. Optional `bind_tools` happens in `_should_bind_tools` (`chat_agent_executor.py:173-217`) and is enforced once at graph-construction time, not at every step.

2. **How are observations returned?**
   `ToolNode._execute_tool_sync` / `_execute_tool_async` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:922-1012` and `tool_node.py:1069-1159`) run each tool and return either a `ToolMessage(content=..., tool_call_id=call["id"], name=call["name"])` for success or `ToolMessage(content=..., tool_call_id=call["id"], name=call["name"], status="error")` for handled errors. Multiple outputs are combined (`_combine_tool_outputs` at `tool_node.py:862-920`) and pushed into the state via `add_messages` (`libs/langgraph/langgraph/graph/message.py:60-244`). The model in the next loop step sees the **raw tool output** as the `content` of the `ToolMessage` (or a JSON-encoded string when the tool returns arbitrary Python objects, per `msg_content_output` at `tool_node.py:309-336`). There is no automatic summarization step.

3. **Is reasoning visible or hidden?**
   Hidden by default. The framework stores only what the model returned — `AIMessage.content`, `additional_kwargs`, `response_metadata`, `usage_metadata`. There is no `Thought` message type, no separate scratchpad buffer, and no way to address reasoning independently of the final tool-call decisions. If a model emits reasoning text, it lives in `AIMessage.content` like any other assistant content. The framework does not surface `reasoning_content` / `reasoning_tokens` separately; that data must be parsed by the caller from `usage_metadata` if the provider populates it (the field is documented in `libs/langgraph/tests/__snapshots__/test_large_cases.ambr:91` schema). Pregel's "scratchpad" (`libs/langgraph/langgraph/_internal/_scratchpad.py:8-19`) is unrelated to model reasoning — it tracks `step`, `stop`, call/interrupt/subgraph counters, and resume values for the engine.

4. **Are failed observations fed back?**
   Yes, by default. The default `handle_tool_errors` callable (`_default_handle_tool_errors` at `libs/prebuilt/langgraph/prebuilt/tool_node.py:383-391`) catches `ToolInvocationError` (raised on Pydantic validation failure, `tool_node.py:957-966`) and produces a `ToolMessage(status="error")` with a templated message (`TOOL_CALL_ERROR_TEMPLATE` at `tool_node.py:111`). Other exceptions propagate unless the user opts in via `handle_tool_errors=True`, `type[Exception]`, tuple of types, or a callable (`tool_node.py:749-754`). The result is added to state by the next `add_messages` pass and the next model invocation sees the error as a regular observation. `_validate_chat_history` (`chat_agent_executor.py:243-271`) additionally enforces the invariant "every `tool_call` has a matching `ToolMessage`" before each model call. The framework deliberately does **not** "re-plan with feedback" — an error is an observation like any other, and the model may repeat the same call. Interceptors (`wrap_tool_call` / `awrap_tool_call`, `tool_node.py:202-205`, `tool_node.py:1014-1067`, `tool_node.py:1161-1222`) are the documented escape hatch for retry logic.

5. **Can observations be compacted?**
   Not by the framework. There is no built-in compaction of tool observations or message history. The supported entry points are user-supplied `pre_model_hook` (`chat_agent_executor.py:396-413`, `chat_agent_executor.py:724-742`) which may return `{"messages": [RemoveMessage(id=REMOVE_ALL_MESSAGES), ...]}` or `{"llm_input_messages": [...]}` to override the history presented to the next model step, and `RemoveMessage` (`libs/langgraph/langgraph/graph/message.py:60-244` reducer) which the `add_messages` reducer honors. Streaming partial tool output is supported via a separate `tools` channel and a `ToolCallStream` projection (`libs/prebuilt/langgraph/prebuilt/_tool_call_stream.py:17-117`, `libs/langgraph/langgraph/pregel/_tools.py:35-268`) but that is **streaming**, not compaction — the full output eventually lands in the `ToolMessage`.

## Architectural Decisions

- **Graph-as-loop.** The Reason/Act/Observe cycle is encoded as a `StateGraph` with two nodes (`agent`, `tools`) and a conditional edge (`should_continue` at `chat_agent_executor.py:831-859`). This makes the cadence observable and rewritable — anyone can insert intermediate nodes — at the cost of pushing prompt shaping and compaction to the caller. `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:485-497` documents the cycle as a Mermaid sequence diagram.
- **No textual ReAct parser.** `ToolNode._parse_input` (`tool_node.py:1224-1266`) trusts the structured `tool_calls` field on `AIMessage`. The framework intentionally depends on LangChain/chat-model providers to surface tool calls as structured data; free-text "Action: foo(...)" parsing is not supported.
- **Dispatch duality (v1 vs v2).** `version="v1"` returns a single `"tools"` edge, running all `tool_calls` inside one `ToolNode` step; `version="v2"` (the default, `chat_agent_executor.py:305,845-859`) fans each `tool_call` out via `Send("tools", ToolCallWithContext(...))` so parallel execution can be checkpointed and resumed per-call (`tool_node.py:286-307`).
- **Observations are first-class state.** Every `ToolMessage` is pushed into `state["messages"]` and is automatically re-injected into the next model call. There is no separate "scratchpad" — the chat history IS the scratchpad, plus `add_messages` reducer dedup (`graph/message.py:60-244`).
- **Errors become observations by default.** `ToolNode` is opinionated: `handle_tool_errors` defaults to a callable that catches `ToolInvocationError` (`tool_node.py:383-391`), so the ReAct loop self-corrects on schema-validation failures without any user wiring.
- **Defense in depth on injection.** Tool argument injection (`InjectedState`, `InjectedStore`, `ToolRuntime`) happens after the model emits `tool_calls`, and `stripped_args` (`tool_node.py:1421-1429`) overwrites any LLM-supplied value for an injected argument with the trusted server-side value, preventing the model from forging hidden state.
- **Loop-safety layered.** A managed `RemainingSteps` value (`libs/langgraph/langgraph/managed/is_last_step.py:18-23`) is read by `_are_more_steps_needed` (`chat_agent_executor.py:620-634`) to convert a would-be over-budget loop into a final text response. The Pregel engine enforces a hard `recursion_limit` (`pregel/main.py:3020-3022`). Graph-level `interrupt_before` / `interrupt_after` provide an additional HITL pause knob (`chat_agent_executor.py:302-305`).

## Notable Patterns

- **Interceptor-based extension.** `wrap_tool_call` / `awrap_tool_call` (`tool_node.py:1014-1067`, `tool_node.py:1161-1222`) expose the tool-execution moment as `ToolCallRequest` + `execute(...)` closure, allowing retries, caching, short-circuiting, and dynamic registration of new tools (`tool_node.py:1354-1361`). The closure is documented to be callable multiple times (`tool_node.py:215-217`) for retry logic.
- **ContextVar-scoped writer.** `ToolRuntime.emit_output_delta` (`tool_node.py:1732-1750`) reads `_tool_call_writer` (a `ContextVar` set in `StreamToolCallHandler._start` at `pregel/_tools.py:142-156`) so tools can stream partial output without threading a writer through their signature. The handler resets the ContextVar even across context boundaries (`pregel/_tools.py:213-222`).
- **Streaming as a separate channel.** Tool-execution events live on `tools`, not `messages` (`pregel/_messages.py:308-334` for the v2 message handler explicitly drops finalized `ToolMessage` so the messages stream does not double-count). A native transformer (`_tool_call_transformer.py:44-150`) projects these into per-tool `ToolCallStream` handles with `output_deltas`, `output`, `error`, and `completed`.
- **ToolMessage from tool return value.** Tools can return a `ToolMessage` directly, a `Command(update=[...])`, a list, or a tuple (`tool_node.py:1432-1453`). `_validate_tool_command_list` (`tool_node.py:1455-1501`) enforces that exactly one terminating `ToolMessage` (matching `tool_call_id`) exists across the list — a precise structural invariant.
- **Chat-history validation as a runtime invariant.** `_validate_chat_history` (`chat_agent_executor.py:243-271`) catches the most common ReAct-loop bug class ("orphaned `tool_calls`") with a clear error code (`ErrorCode.INVALID_CHAT_HISTORY`) before each model step.

## Tradeoffs

- **Decoupled cadence vs. no built-in compaction.** Because the loop is a plain graph, the framework owns no opinion about prompt shaping, summarization, or scratchpad design. A user wanting an "auto-compact observations" behavior must implement it via `pre_model_hook` and `RemoveMessage`. This is a deliberate extensibility point, but it also means there is no reference implementation shipped.
- **Hidden reasoning.** By treating the `AIMessage` as the only source of model state, the framework is robust against any provider-specific reasoning format but cannot distinguish a "thought" from a "decision". Tools that need to influence the model at the reasoning level must operate on the full `content` string, not on a parsed thought stream.
- **Errors are observations, not signals.** A failed tool produces a regular `ToolMessage(status="error")`; the framework does not maintain an `errors_in_a_row` counter, does not flip `tool_choice="none"` after N failures, and does not expose a structured "re-plan" path. Retries and error-escalation logic is the caller's job, typically via `wrap_tool_call` (see `libs/prebuilt/tests/test_on_tool_call.py:725-880` for retry patterns).
- **`handle_tool_errors` defaults.** The default callable (`_default_handle_tool_errors` at `tool_node.py:383-391`) only catches `ToolInvocationError`. Other exceptions (e.g. `ValueError` from inside the tool body) re-raise by default; this can surprise users expecting "tools never crash the agent". The other defaults (`True`, `(Exception,)`, callable-typed) are all available and tested (`libs/prebuilt/tests/test_tool_node.py:319-471`).
- **Send-fan-out version split.** `v1` vs `v2` (`chat_agent_executor.py:843-859`) creates two subtly different observable behaviors: v2 fans each call into its own checkpointable task (good for HITL pauses and parallelism), v1 bundles them. The version flag is part of the public surface; switching versions changes the stream shape (`libs/prebuilt/tests/test_react_agent.py:599-676`).
- **Pydantic-1/2 split in validation path.** `ValidationNode` (deprecated) handles both `BaseModel` and `BaseModelV1` (`tool_validator.py:184-221`) and is retained for backwards compatibility, not for new work.

## Failure Modes / Edge Cases

- **LLM cannot forge InjectedState.** Covered by `_filter_validation_errors` (`tool_node.py:510-563`) and the `stripped_args` overwrite (`tool_node.py:1421-1429`). Tested at `libs/prebuilt/tests/test_tool_node.py:2208-2238`.
- **Unknown tool name.** `_validate_tool_call` (`tool_node.py:1268-1279`) produces a `ToolMessage(status="error")` with `INVALID_TOOL_NAME_ERROR_TEMPLATE` so the loop self-corrects (`tool_node.py:108-110`). Tested at `libs/prebuilt/tests/test_tool_node.py:252-266`.
- **Orphaned tool calls in chat history.** `_validate_chat_history` (`chat_agent_executor.py:243-271`) raises `INVALID_CHAT_HISTORY` before the model is invoked. Tested at `libs/prebuilt/tests/test_react_agent.py:371-433`.
- **Validation failure.** `ValidationError` is wrapped in `ToolInvocationError` (`tool_node.py:957-966`) and routed through the configured `handle_tool_errors` handler. By default it becomes an error `ToolMessage`. Tested at `libs/prebuilt/tests/test_tool_node.py:269-471` and `test_tool_node_validation_error_filtering.py`.
- **Subgraph pauses inside a tool.** `GraphBubbleUp` is always re-raised through `ToolNode` (`tool_node.py:982-983`, `tool_node.py:1129-1130`) so `interrupt()` calls inside a tool (or inside a graph-as-tool) propagate correctly.
- **Loop guardrail.** `RemainingSteps < 2` short-circuits the loop with a final `AIMessage("Sorry, need more steps to process this request.")` (`chat_agent_executor.py:684-692`). Hard cap `recursion_limit` is enforced by Pregel (`pregel/main.py:3020-3022`).
- **Parallel tool calls where one returns `Command` and the rest return `ToolMessage`.** `_combine_tool_outputs` (`tool_node.py:862-920`) mixes them, combining all parent `Command(graph=PARENT, goto=[Send(...)])` outputs into a single parent command.
- **Streaming tool whose callbacks fire in a different context.** `_tool_call_writer.reset` swallows the `ValueError` (`pregel/_tools.py:213-222`) so the run is not torn down by a benign ContextVar lifecycle mismatch; the writer's lifetime is bounded by the enclosing task.
- **Return-direct tools.** A tool whose `return_direct=True` is honored by `route_tool_responses` (`chat_agent_executor.py:970-988`) — the graph routes to `END` rather than back to the agent, even with parallel `Send` fan-out.

## Future Considerations

- **No first-class reasoning channel.** If the framework ever needs to address model reasoning independently (for compaction, observability, or replay), a `ReasoningContent` / scratchpad abstraction would have to be added. Today the only carrier is `AIMessage.content`, which forces callers to parse provider-specific formats.
- **No automatic compaction / summarization.** A built-in observation summarizer (analogous to `langchain-core`'s `trim_messages` or a sliding-window strategy) would close the loop on context growth. Today this is delegated to `pre_model_hook` and `RemoveMessage`.
- **No consecutive-error counter.** Unlike frameworks that flip `tool_choice="none"` after N failures to force the model out of a retry loop, LangGraph relies on the model itself to break a retry cycle. A `errors_in_a_row` managed value would be a natural addition.
- **`create_react_agent` is deprecated.** The docstring (`chat_agent_executor.py:53-56, 274-277, 308-318`) explicitly tells users to migrate to `langchain.agents.create_agent`. The ReAct cadence primitives (`ToolNode`, `tools_condition`, error handling, interceptors) remain supported because the new factory uses them under the hood.
- **Two `DEFAULT_MAX_ITERATIONS`-style caps.** `remaining_steps` is the soft cap inside `create_react_agent`; `recursion_limit` is the hard cap inside Pregel (`pregel/main.py:3020-3022`, `libs/langgraph/langgraph/_internal/_config.py:184-186`). They measure different things and both must be set correctly to avoid surprise aborts.

## Questions / Gaps

- **No evidence of any "Thought" / "Action:" / "Observation:" text-protocol parser.** Searched `libs/prebuilt/langgraph/prebuilt/*.py` and `libs/langgraph/langgraph/pregel/*.py`. The framework only consumes structured `tool_calls`. No evidence found for an alternative ReAct-style textual parser.
- **No evidence of reasoning compaction or summarization in core.** Searched for `trim`, `compaction`, `summarize`, `scratchpad` (the framework's `scratchpad` is engine-internal, not reasoning). Only the `pre_model_hook` user-extension point exists (`chat_agent_executor.py:396-413`). No built-in implementation found.
- **No evidence of automatic error-driven re-planning.** The framework surfaces errors as observations but does not synthesize "feedback" messages or maintain a retry counter. Interceptors are the documented path for that logic (`tool_node.py:215-275`).
- **`AIMessage.reasoning` / hidden chain-of-thought fields.** No evidence of explicit handling in LangGraph. `usage_metadata.output_token_details.reasoning` is provider-populated but the framework does not branch on its presence; reasoning text travels as ordinary `content`. (Snapshot at `libs/langgraph/tests/__snapshots__/test_large_cases.ambr:91` shows the field is documented in the schema.)
- **`MessageGraph` (legacy) vs `MessagesState` + `StateGraph`.** `MessageGraph` (`graph/message.py:312-369`) is deprecated; the canonical path is `Annotated[list, add_messages]` via `MessagesState` (`graph/message.py:372-373`). The same `add_messages` reducer is used either way.

---

Generated by `03.02-reason-act-observe-cadence` against `langgraph`.
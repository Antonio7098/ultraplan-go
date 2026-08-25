# Source Analysis: agent-framework

## 03.01 LLM Turn Loop Structure

### Source Info

| Field | Value |
|-------|-------|
| Name | microsoft/agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (3.11+; partial `dotnet/` sibling), `agent_framework` package (`python/packages/core/agent_framework`) |
| Analyzed | 2026-07-12 |

## Summary

The model-call loop lives **at the chat-client layer**, not on the `Agent` class.

`Agent.run()` (`python/packages/core/agent_framework/_agents.py:899-977`) is a thin façade that calls `_prepare_run_context()` (line 948) and then hands the assembled context to `self.client.get_response()` via `_call_chat_client()` (`python/packages/core/agent_framework/_agents.py:995-1021`).

The actual iterative turn loop lives in `FunctionInvocationLayer.get_response()` (`python/packages/core/agent_framework/_tools.py:2467-2873`), where each iteration performs: (1) a tool-approval pre-pass, (2) a single `_inner_get_response` chat call to the leaf client, (3) tool execution via `_try_execute_function_calls` / `_execute_function_calls`, (4) usage aggregation and `service_session_id` propagation via `_update_continuation_state` (`python/packages/core/agent_framework/_tools.py:1883-1901`), (5) message list bookkeeping (`prepped_messages.extend(...)` for stateless APIs, `prepped_messages = []` + `prepped_messages.append(response.messages[-1])` for conversation-based APIs at lines 2654-2661 / 2825-2832). The whole thing is bounded by `max_iterations` (`DEFAULT_MAX_ITERATIONS: int = 40`, `_tools.py:90`) and `max_function_calls`. Termination branches are: assistant emits no `function_call` / `function_approval_request` (`_tools.py:2348-2357`), `max_function_calls` reached, `max_iterations` reached, error threshold reached, or middleware-driven `MiddlewareTermination`.

Persistence is delegated to history providers: `HistoryProvider.before_run` / `after_run` (`python/packages/core/agent_framework/_sessions.py:506-534`) load and save messages around each `agent.run()` call, with `PerServiceCallHistoryPersistingMiddleware` (`_sessions.py:570-744`) installing per-iteration persistence when configured. Turn state is not persisted as discrete records; instead, every turn's new messages are appended to the conversation buffer (either in `AgentSession.state["messages"]` via `InMemoryHistoryProvider.save_messages` at `_sessions.py:878-890` or forwarded to the service through `service_session_id`).

The same layered loop is reused across every `Agent` (class signature `class Agent(AgentMiddlewareLayer, AgentTelemetryLayer, RawAgent[OptionsCoT])` at `_agents.py:1635-1640`) and across every `BaseChatClient` subclass via the mixin chain `BaseChatClient` → `ChatTelemetryLayer` → `ChatMiddlewareLayer` → `FunctionInvocationLayer` (`_clients.py:975-999`). The same `FunctionInvocationLayer` is even re-entered from the harness's experimental `AgentLoopMiddleware` (`_harness/_loop.py:212`), but at the **agent** layer — that middleware drives the wrapped agent *itself* in a loop, not the function-invocation loop inside one call.

## Rating

**8 / 10** — Clear model with named phases, separation of agent- and client-layer loops, explicit bounds, measurable tests (`test_function_invocation_logic.py:1000-1290`, `test_harness_loop.py:212-360`), and OTel spans that align 1:1 with each model call (lines 1596, 1614 in `observability.py` give `_get_span` boundaries). Deductions: the loop is split across `_tools.py` and `_sessions.py` so the path from "turn begins" to "turn persisted" is non-trivial to trace without reading four files; the once-per-run `after_run` persistence model can lose work between turns on crash (only `PerServiceCallHistoryPersistingMiddleware` closes that gap, and only when explicitly enabled); the harness layer's `AgentLoopMiddleware` is a separate, parallel loop with its own `DEFAULT_MAX_ITERATIONS = 10` (`_harness/_loop.py:117`) — naming collisions across the two loops (tool-loop "iterations" = model roundtrips, harness "iterations" = entire agent invocations) are easy to misread.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Runner loop | `FunctionInvocationLayer.get_response` iterative loop | `python/packages/core/agent_framework/_tools.py:2551-2696` |
| Runner loop streaming branch | `FunctionInvocationLayer.get_response` streaming loop | `python/packages/core/agent_framework/_tools.py:2700-2866` |
| Turn boundary (non-streaming) | `for attempt_idx in range(attempt_start, max_iterations if loop_enabled else 0)` | `python/packages/core/agent_framework/_tools.py:2567` |
| Iteration cap constant | `DEFAULT_MAX_ITERATIONS: Final[int] = 40` | `python/packages/core/agent_framework/_tools.py:90` |
| Per-call + per-function-call caps | `FunctionInvocationConfiguration` TypedDict | `python/packages/core/agent_framework/_tools.py:1344-1395` |
| Termination "no function calls" path | `function_calls = _extract_function_calls(response); if not (function_calls and tools): return action="return"` | `python/packages/core/agent_framework/_tools.py:2346-2357` |
| Termination "stop on error threshold" | `_handle_function_call_results` returns `action="stop"` when `errors_in_a_row >= max_errors`; forces `tool_choice="none"` next pass | `python/packages/core/agent_framework/_tools.py:2248-2265`, `2645-2652` |
| Termination "max_iterations final text" | Sets `tool_choice="none"` and makes a final plain model call | `python/packages/core/agent_framework/_tools.py:2664-2696` (and streaming mirror `2835-2866`) |
| Agent runner façade | `Agent.run` → `_prepare_run_context` → `_call_chat_client` → `client.get_response` | `python/packages/core/agent_framework/_agents.py:899-1021` |
| Agent layer composition | `class Agent(AgentMiddlewareLayer, AgentTelemetryLayer, RawAgent[OptionsCoT])` | `python/packages/core/agent_framework/_agents.py:1635-1640` |
| Runner context TypedDict (loop inputs) | `_RunContext` carries session, options, messages, function invocation kwargs | `python/packages/core/agent_framework/_agents.py:170-182` |
| Model call wrapper (chat client) | `BaseChatClient.get_response` → `_inner_get_response` (provider-implemented) | `python/packages/core/agent_framework/_clients.py:413-555` |
| Layered client composition | `ChatTelemetryLayer` → `FunctionInvocationLayer` → `ChatMiddlewareLayer` docstring stitching | `python/packages/core/agent_framework/_clients.py:975-999` |
| Message assembly (tool result) | `_handle_function_call_results` builds `Message(role="tool", contents=function_call_results)` and appends to `response.messages` | `python/packages/core/agent_framework/_tools.py:2261-2269` |
| Message assembly (approval wrapper) | Approval requests are appended to the assistant message and `action="return"` | `python/packages/core/agent_framework/_tools.py:2228-2246` |
| Conversation-based vs stateless prepped_messages reset | `prepped_messages = []` + only the new tool results retained; otherwise append | `python/packages/core/agent_framework/_tools.py:2613-2661` (and streaming `2780-2832`) |
| Continuation token propagation | `_update_continuation_state` writes `service_session_id` back to `AgentSession` and options | `python/packages/core/agent_framework/_tools.py:1883-1901` |
| Streaming continuation propagation in agent | `_propagate_conversation_id` eagerly captures `service_session_id` from raw updates | `python/packages/core/agent_framework/_agents.py:1085-1099` |
| Internal/local sentinel conversation id | `LOCAL_HISTORY_CONVERSATION_ID = "agent_framework_local_history_persistence"` | `python/packages/core/agent_framework/_sessions.py:537-543` |
| History persistence (load) | `HistoryProvider.before_run` calls `get_messages` and `extend_messages` on the SessionContext | `python/packages/core/agent_framework/_sessions.py:506-516` |
| History persistence (save) | `HistoryProvider.after_run` aggregates inputs/outputs/context and calls `save_messages` | `python/packages/core/agent_framework/_sessions.py:518-534` |
| In-memory default provider | `InMemoryHistoryProvider.get_messages` / `save_messages` read/write `state["messages"]` | `python/packages/core/agent_framework/_sessions.py:814-890` |
| Auto-injection of in-memory history | `Agent._prepare_run_context` injects `InMemoryHistoryProvider()` when no providers and no service storage | `python/packages/core/agent_framework/_agents.py:1202-1209` |
| Per-call persistence middleware (turn-level) | `PerServiceCallHistoryPersistingMiddleware` | `python/packages/core/agent_framework/_sessions.py:570-744` |
| Session model | `AgentSession` with `session_id`, `service_session_id`, `state`; serializable via `to_dict`/`from_dict` | `python/packages/core/agent_framework/_sessions.py:746-811` |
| SessionContext (per-invocation mutable) | `SessionContext` carrying `input_messages`, `context_messages`, `response`, etc. | `python/packages/core/agent_framework/_sessions.py:154-348` |
| OpenTelemetry chat-completion spans (per turn) | `ChatTelemetryLayer.get_response` opens a span per `_get_response` and per stream finalize | `python/packages/core/agent_framework/observability.py:1436-1652` |
| OpenTelemetry agent-invoke spans (per `agent.run`) | `AgentTelemetryLayer._trace_agent_invocation` opens per-agent-invocation span | `python/packages/core/agent_framework/observability.py:1725-1900` |
| Token-usage aggregation across turns | `add_usage_details` per loop pass | `python/packages/core/agent_framework/_tools.py:2605`, `2685` |
| Test: per-loop max_iterations cap | `test_max_iterations_limit` | `python/packages/core/tests/core/test_function_invocation_logic.py:1000-1044` |
| Test: no orphaned function calls after cap | `test_max_iterations_no_orphaned_function_calls` | `python/packages/core/tests/core/test_function_invocation_logic.py:1047-1100` |
| Test: final `tool_choice=none` turn after cap | `test_max_iterations_makes_final_toolchoice_none_call` | `python/packages/core/tests/core/test_function_invocation_logic.py:1110-1160` |
| Test: thread integrity across cap | `test_max_iterations_thread_integrity_with_agent` | `python/packages/core/tests/core/test_function_invocation_logic.py:1229-1290` |
| Test: streaming max_iterations cap | `test_streaming_max_iterations_limit` | `python/packages/core/tests/core/test_function_invocation_logic.py:2849-2899` |
| Test: harness loop cap (separate) | `test_loop_stops_at_max_iterations` | `python/packages/core/tests/core/test_harness_loop.py:212-260` |
| Test: agent invoke span aggregates usage on exhaustion | `test_agent_invoke_span_aggregates_usage_on_max_iterations_exhaustion` | `python/packages/core/tests/core/test_observability.py:4262-4275` |
| Harness-level loop (separate from tool loop) | `AgentLoopMiddleware` wraps `call_next()` with `while True` at agent boundary | `python/packages/core/agent_framework/_harness/_loop.py:454-660` |
| Harness loop iteration cap | `DEFAULT_MAX_ITERATIONS = 10`, `DEFAULT_JUDGE_MAX_ITERATIONS = 5` | `python/packages/core/agent_framework/_harness/_loop.py:117-122` |

## Answers to Dimension Questions

1. **What happens during one turn?** A turn is one pass through `FunctionInvocationLayer.get_response`'s `for attempt_idx in range(attempt_start, max_iterations)` loop (`_tools.py:2567`). The phases, in order:
   1. **Pre-tool approval pass** — `_process_function_requests(..., response=None, ...)` checks `prepped_messages` for any `function_approval_response` items, executes those approved tools immediately, and either returns "continue" or "return/stop" (`_tools.py:2287-2334`).
   2. **One model call** — `super_get_response(messages=prepped_messages, stream=stream, options=mutable_options, ...)` invokes the leaf `_inner_get_response` of `BaseChatClient` (`_tools.py:2594-2604`, streaming mirror `2744-2755`).
   3. **Tool execution pass** — `_process_function_requests(response=response, ...)` extracts `function_call` / `function_approval_request` items, runs `_execute_function_calls` (`_tools.py:2359-2376`).
   4. **Result handling** — `_handle_function_call_results` either (a) for approval-gated or declaration-only responses returns `action="return"` with the result folded into the assistant message (`_tools.py:2228-2246`), or (b) for tool results builds `Message(role="tool", contents=function_call_results)`, appends to `response.messages`, returns `action="continue"` or `action="stop"` based on `errors_in_a_row` (`_tools.py:2248-2270`).
   5. **Budget / cap side-effects** — update `total_function_calls` and `errors_in_a_row`; if either cap is hit, set `mutable_options["tool_choice"] = "none"` to suppress further tool calls (`_tools.py:2586-2592`, `2636-2644`).
   6. **Continuation state** — `_update_continuation_state` copies `response.conversation_id` back into `kwargs`/`options` and persists it on `AgentSession.service_session_id` (`_tools.py:1883-1901`, called from `_tools.py:2606-2611`, `2686-2691`).
   7. **Message list bookkeeping** — for conversation-managed APIs, drop everything except the latest assistant message (`_tools.py:2654-2660`, streaming `2825-2831`); otherwise extend `prepped_messages` with `response.messages` (`_tools.py:2661`, `2832`).
   8. **Usage aggregation** — `aggregated_usage = add_usage_details(aggregated_usage, response.usage_details)` (`_tools.py:2605`, `2685`).
2. **Does every turn call the model?** Yes. Each `attempt_idx` produces exactly one `super_get_response` call to the chat client (`_tools.py:2594`, `2765-2766`, `2860`). The only turns that do not produce a model call are the harness-level "iteration" turns of `AgentLoopMiddleware`, which skip the model by their own logic — but that is a different, outer loop.
3. **Can a turn include multiple tool calls?** Yes. The model emits all of its `function_call` items in one assistant message; `_extract_function_calls` (`_tools.py:2347`) returns them as a `Sequence[Content]`; `_execute_function_calls` (`_tools.py:2359`) runs them in parallel against `tool_map` and middleware. The framework deliberately tracks `max_iterations` (model roundtrips) and `max_function_calls` (cumulative tool calls) as **two separate** caps because of this (`_tools.py:1344-1395`).
4. **Is turn state persisted?** It depends on which knob you use.
   - **Default agent run**: persistence is run-scoped. `HistoryProvider.before_run` loads before the very first model call, and `after_run` saves once after the loop exits (`_sessions.py:506-534`). Between turns inside one `agent.run`, no separate persistence happens.
   - **`require_per_service_call_history_persistence=True`**: `PerServiceCallHistoryPersistingMiddleware` (`_sessions.py:570-744`) persists after every model call, with different load/skip behaviour depending on whether the service stores history server-side.
   - **Service-managed APIs** (e.g., OpenAI Responses with `store=True`): the service holds the transcript; the agent only updates `AgentSession.service_session_id` (`_tools.py:1883-1901`).
   - The framework does **not** mark a turn boundary in the persisted transcript (no turn id / turn index / completion_token in `Message`); the loop relies on message ordering, the stored `conversation_id`, and aggregated `UsageDetails` for state. Concrete evidence: `Message` and `ChatResponse` expose only `role`, `contents`, `usage_details`, `response_id` (see `_types.py` uses at `_agents.py:1039-1049`).
5. **Is the loop generic or agent-specific?** The function-invocation loop is **fully generic** — it lives on `FunctionInvocationLayer`, a mixin applied to every `BaseChatClient` subclass (`_clients.py:975-999`). It is reused across the OpenAI, Azure, Foundry, Anthropic, Bedrock, Ollama, Gemini, Mistral, GitHub Copilot clients via this single class. The **agent-level** loop is the experimental `AgentLoopMiddleware` (`_harness/_loop.py:212`) which is wrapped onto a specific agent via `Agent(..., middleware=[AgentLoopMiddleware(...)])`. `AgentLoopMiddleware` is itself not tied to any particular agent class — it works against any `SupportsAgentRun` (`_harness/_loop.py:50-58`).

## Architectural Decisions

- **Layered design via cooperative multiple inheritance.** `Agent(AgentMiddlewareLayer, AgentTelemetryLayer, RawAgent)` (`_agents.py:1635-1640`) composes middleware and telemetry around `RawAgent`'s `run`. The matching client side is `BaseChatClient` → `ChatTelemetryLayer` → `FunctionInvocationLayer` → `ChatMiddlewareLayer` (`_clients.py:975-999`). Docstrings are explicitly stitched between layers (`_clients.py:978-999`).
- **Loop placed at the chat client, not the agent.** This pushes the function-call loop down to where the model call actually happens, so any agent that uses a `FunctionInvocationLayer`-decorated client gets tool calling for free (`_tools.py:2388-2478`, agent without tool support logged at `_agents.py:722-725`).
- **Iterate over `prepped_messages`, not over a raw call counter.** The loop maintains an explicit `prepped_messages` buffer and trims/restores it each turn (`_tools.py:2559`, `2654-2661`). The same pattern is reproduced for streaming (`_tools.py:2710`, `2780-2832`).
- **Two-tier termination policy.** Distinct `max_iterations` (model round-trips, default 40 at `_tools.py:90`) and `max_function_calls` (cumulative tool calls, optional at `_tools.py:1394`) with a separate `max_consecutive_errors_per_request` (default 3, `_tools.py:91`). They cover distinct runaway modes (model keeps calling tools vs. one model call keeps emitting many tools vs. broken tools throwing repeatedly).
- **Forced clean-text tail after every cap.** When `max_iterations` is reached, the loop sets `tool_choice="none"` and makes a final plain model call (`_tools.py:2664-2696`, streaming `2835-2866`). Tests at `test_function_invocation_logic.py:1047-1160` specifically guard against orphaned `function_call` items left without matching `function_result` content after the cap.
- **Tri-state action enum.** `_process_function_requests` and `_handle_function_call_results` use a 3-string action set `"return" | "stop" | "continue"` (`_tools.py:2227-2246`, `2264-2270`) where `"stop"` flips the next pass to text-only.
- **Mutable options, immutable messages.** `mutable_options` is rebuilt on every iteration as `dict(options)` (`_tools.py:2530-2533`) so cross-iteration tool overrides don't bleed back to the caller; messages are mutated in place on the response object.
- **Middleware-overrideable function pipeline.** Function middleware can call `MiddlewareTermination` to short-circuit the loop (`_tools.py:2374-2377`); the harness tool approval middleware (experimental) leans on this for batch-approval bypass (`_harness/_tool_approval.py:365-450`).
- **Per-service-call vs once-per-run persistence are explicit.** `_resolve_per_service_call_history_providers` (`_agents.py:802-824`) and the related warning (`_agents.py:1230-1239`) make the user's choice audible and validated; service-managed ids and the framework's local sentinel (`_sessions.py:537-543`) are kept distinct so they don't accidentally leak.
- **Two unrelated loops both named "max_iterations".** The tool-loop cap (`_tools.py:90`, value 40) and the harness cap (`_harness/_loop.py:117`, value 10; judge variant 5 at line 122) share a name; they have separate docstrings and tests but in code search both `DEFAULT_MAX_ITERATIONS` show up unprefixed. There is also a workflow-level cap of 100 in `_workflows/_const.py:4`.

## Notable Patterns

- **Layer docstring stitching** to keep the public `Agent.run` and `BaseChatClient.get_response` docstrings consistent across mixin chains (`_clients.py:978-999`).
- **Cooperative async-with pattern** where layers wrap `super_get_response` rather than overriding it (e.g., `FunctionInvocationLayer.get_response` calls `super().get_response` at `_tools.py:2487`, then again at `2596` to recurse into the next layer down).
- **Trailing-component insertion** for tool results so the loop reuses the existing `response.messages` list (`_tools.py:2236-2262`) instead of building a fresh response, simplifying the streaming aggregation.
- **Progressive tool exposure.** Tools can be added/removed during a run via `context.add_tools(...)` / `context.remove_tools(...)` (per `core/AGENTS.md`); the layer maintains a single `mutable_options["tools"]` list so all iterations see the same list (`_tools.py:2549-2550`).
- **Service-vs-stateless branching** inside the loop body (`_tools.py:2613-2614`, `2654-2661`) so the same loop runs against providers that maintain server-side conversations and providers that don't.
- **Per-iteration `attempt_idx`** used for traceability (`_tools.py:2557-2568`), forwarded to tool-execution error reporting and to the budget state (`_tools.py:2507`).
- **OTel span scope align 1:1 with the model call** rather than the agent run: a chat-completion span opens at `_observability.py:1596` and closes at `1576`; an agent-invoke span opens at `_observability.py:1874` (`_observability.py:1596`, `1874`). The framework specifically accumulates inner chat usage into the agent span on `max_iterations` exhaustion (`test_observability.py:4262`).

## Tradeoffs

- **Loop split between two files.** `_process_function_requests` is in `_tools.py`, and the persistence side is in `_sessions.py`. Tracing any specific turn from model call → tool result → history save touches at least three files (`_tools.py`, `_sessions.py`, `_agents.py`); the `PerServiceCallHistoryPersistingMiddleware` layer adds a fourth (`_sessions.py:570-744`). The dimension prompt's bar of "can you draw the loop without reading five files" is just met, not exceeded.
- **No explicit `Turn` record type.** The framework models a turn as an `attempt_idx` plus the messages appended to `prepped_messages` plus `usage_details` on the response. There is no `Turn` dataclass with id, started_at, finished_at, finish_reason. An external system reading this after the fact has to reconstruct turns from message boundaries.
- **Two `DEFAULT_MAX_ITERATIONS` constants.** One cap is for tool-loop model roundtrips (`_tools.py:90`), one is for harness outer-loop iterations (`_harness/_loop.py:117`). Both surfaces are named `DEFAULT_MAX_ITERATIONS` and are imported under similar names; grep returns both hits. A reader who searches for "loop iterations" finds ten occurrences across `_tools.py`, `_middleware.py`, `_harness/_loop.py`, `_workflows/_workflow.py`, `_workflows/_const.py`. The framework does not unify them.
- **Per-call opt-in durability.** Per-call persistence is gated behind `require_per_service_call_history_persistence=True` (`_agents.py:737`). The default flow (`_sessions.py:506-534`) only persists on `after_run`, so a crash mid-loop loses all but the user prompt. The harness enables per-call persistence by default (`_harness/_agent.py:519`), but the plain `Agent` does not.
- **Service-managed conversation id must be cleared if framework-managed.** `_clear_internal_conversation_id` (`_tools.py:1904-1908`) exists precisely because of the dual mode; tests verify `response.conversation_id is None` after a framework-only run. Leakage of the sentinel conversation id (`_sessions.py:537`) into a downstream client that doesn't know it would be a silent bug.
- **Streaming and non-streaming loops are siblings, not one loop with a flag.** Most logic is duplicated across `_tools.py:2553-2698` and `_tools.py:2703-2866`. New turn handling (e.g., tool approval for hosted tool calls) has to be implemented twice.
- **`function_invocation_configuration` is mutated at call time.** The `mutable_options` dict is rebuilt and mutated for `tool_choice` flips (`_tools.py:2592`, `2644`, `2673`) — caller's options dict is not affected (it's `dict(options)` at `_tools.py:2530`), but the function-cancellation surface for cancelling mid-loop has to be reached through `Budget`/`attempt_idx` rather than a structured cancellation handle.
- **`Aggregated usage` lives on `response.usage_details` only after the loop returns.** It is built per pass but applied at `2685` or `2928` for streaming. Users see only the final, aggregated usage and lose the per-turn breakdown unless they wrap `ChatMiddleware` to capture each pass.

## Failure Modes / Edge Cases

- **Orphaned `function_call` items if a tool throws and `max_iterations` is reached.** Mitigated explicitly by `_handle_function_call_results` injecting results even on error, and by the final `tool_choice="none"` pass (`_tools.py:2664-2696`). Covered by `test_max_iterations_no_orphaned_function_calls` (`test_function_invocation_logic.py:1047-1100`).
- **Tool errors in a row reaching the threshold.** `_handle_function_call_results` flips `errors_in_a_row` and returns `action="stop"` (`_tools.py:2248-2256`); the outer loop flips `tool_choice="none"` so the model speaks its result instead of looping forever. Without this, a permanently broken tool would infinite-loop the agent.
- **`tool_choice == "required"` would loop forever.** Mitigated at `_tools.py:2648-2652` (and streaming `2818-2823`): reset to `None` after one iteration.
- **`max_function_calls` boundary overshoot.** Acknowledged in code comments: "checked after each batch of parallel calls completes, so the current batch always runs to completion even if it overshoots" (`_tools.py:2637-2638`). This is intentional but worth noting for callers who need hard caps.
- **Service thread id mid-stream.** Handled by `_propagate_conversation_id` eager propagation (`_agents.py:1085-1099`) so callers can already see the new id before the stream finishes.
- **Conversation id propagation across conversation-managed APIs.** Verified by `test_max_iterations_thread_integrity_with_agent` (`test_function_invocation_logic.py:1229-1290`).
- **Streaming stream error swallowed if result hooks fire.** The OTel layer explicitly skips `get_final_response()` on stream errors (`observability.py:1543-1548`) to avoid double-firing `after_run` on error paths. This is a correctness fix tied to turn-finalization rather than the loop itself, but it shapes observable behavior.
- **Local sentinel conversation id leaking.** `_clear_internal_conversation_id` (`_tools.py:1904-1908`) plus `is_local_history_conversation_id` checks at `_agents.py:1072`, `1095` keep the framework-only sentinel from leaking into client options.
- **`fresh_context=True` + session interaction.** The harness loop snapshots and restores `AgentSession` (`_harness/_loop.py:417-437`) precisely because a session retains history that the loop otherwise expects to discard; this is a non-obvious coupling.

## Future Considerations

- **Promote per-service-call persistence to default for `Agent`.** Currently only the harness agent enables it (`_harness/_agent.py:519`); promoting it would close the "crash mid-loop loses work" gap that the default `HistoryProvider` has.
- **Unify the two `DEFAULT_MAX_ITERATIONS` constants**, or rename them (`TOOL_LOOP_DEFAULT_MAX_ITERATIONS` vs `HARNESS_LOOP_DEFAULT_MAX_ITERATIONS`). Both are imported widely; today they collide in search.
- **Add an explicit `TurnRecord`/`AgentTurn` dataclass** with id, timestamps, attempt_idx, finish_reason, and tool_call summary, and persist it alongside messages. This would give callers structured turn boundaries without re-parsing message history.
- **Deduplicate streaming and non-streaming loops.** Both branches share `_process_function_requests`, `_handle_function_call_results`, and `_update_continuation_state` verbatim; pulling the loop body into a single helper would halve the maintenance surface.
- **Surface per-turn usage and per-turn finish_reason** alongside `AggregatedUsage`. Right now, callers see only the aggregate; capturing `_update_continuation_state` per pass into a list would let callers inspect each turn.
- **Expose `attempt_idx` to context providers.** Right now it's an internal counter; making it visible to `HistoryProvider.before_run`/`after_run` would let providers record per-turn subhistories.
- **Standardize on response-id vs conversation-id.** The framework tracks both, suppresses internal ones (`_tools.py:1904`), and propagates them differently for streaming (`_agents.py:1085-1099`) vs non-streaming (`_tools.py:1883-1901`). A single canonical "conversation identifier" object could simplify the contracts.

## Questions / Gaps

- **How is mid-stream cancellation communicated?** The grep surfaces no explicit `asyncio.CancelledError` handling in the loop body. `MCPTaskOptions` honors cancellation in `_mcp.py`, but the chat client's loop does not appear to expose a structured cancellation surface. **No clear evidence found** for how `agent.run()` cancellation during turn 2 of 5 is propagated through `FunctionInvocationLayer`.
- **What exactly constitutes a "turn" to providers?** `AgentLoopMiddleware` exposes `iteration` to its callback (`_harness/_loop.py:229-240`), but `FunctionInvocationLayer`'s `attempt_idx` is not surfaced to `HistoryProvider`. **No clear evidence found** that history providers can distinguish "turn 1" vs "turn 2" of an in-progress `agent.run()`.
- **What is the canonical span-tree shape?** `_observability.py:1596` shows one chat-completion span per model call, and `_observability.py:1874` shows one agent-invoke span per `agent.run`, but no diagram or docs path documents the relationship (nested under agent? sequenced?). **No clear evidence found** in code comments; would need to read `test_observability.py:4262` and related tests to verify.
- **Is there a checkpoint/restart seam in the function-invocation loop?** `_sessions.py:71-127` defines session state serialization via `to_dict` / `from_dict`, but the loop state (`prepped_messages`, `errors_in_a_row`, `total_function_calls`, `budget_state`) is **not** persisted. The framework depends entirely on the server-side conversation or on the in-memory `prepped_messages` for replay; restartability would require re-running from the beginning or replaying the server transcript. **No clear evidence found** for a recoverable in-flight turn.
- **What is the boundary on `errors_in_a_row`?** Only `default=3` is documented (`_tools.py:91`); no maximum-inversion discussion. **No clear evidence found** for the failure mode if a tool becomes permanently broken across many `agent.run` calls.

---

Generated by `dimensions/03.01-llm-turn-loop-structure/DIMENSION.md` against `agent-framework`.

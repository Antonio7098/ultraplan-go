# Source Analysis: pydantic-ai

## Dimension 15.02: Message Routing and Termination

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (asyncio), pydantic_graph state-machine runtime, provider-agnostic LLM models |
| Analyzed | 2026-08-26 |

## Summary

Pydantic AI does not implement multi-agent message routing as a first-class "router/speaker-selection" subsystem (there is no group chat, no round-robin speaker registry, no handoff message type). Instead, routing is expressed through four composable mechanisms, all documented in `docs/multi-agent-applications.md:3-9`:

1. **Agent delegation** — a parent agent's function tool awaits a child agent run and returns its output to the parent model (`docs/multi-agent-applications.md:13-105`). Routing is implicit: the parent model decides to invoke the delegate by emitting a tool call.
2. **Programmatic hand-off** — application code calls agents in succession and routes via `message_history` sharing (`docs/multi-agent-applications.md:205-215`, `docs/message-history.md:503`).
3. **Graph-based control flow** — explicit multi-agent orchestration as a `pydantic_graph` state machine (`docs/multi-agent-applications.md:360-362`).
4. **Output functions / union output types as forced handoffs** — the model is forced to call an output tool that ends the run; the output function body may call another agent (`docs/output.md:118-129`).

Within a single run, message routing is a deterministic three-node graph: `UserPromptNode → ModelRequestNode → CallToolsNode → (ModelRequestNode \| End)` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:501`, `_agent_graph.py:1106`, `_agent_graph.py:1816`). The single routing decision per turn lives in `CallToolsNode`: tool calls are classified by `ToolKind` (`'function' | 'output' | 'external' | 'unapproved'`, `pydantic_ai_slim/pydantic_ai/tools.py:539`) and dispatched accordingly (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:374-430`).

Termination is guaranteed without human intervention under bounded budgets: the graph ends when a node returns `End(result)` (`_agent_graph.py:2276-2296`); runaway loops are capped by `UsageLimits` with a **default `request_limit=50`** (`pydantic_ai_slim/pydantic_ai/usage.py:429`, enforced always — see `_run_context.py:70-78`), retry budgets (`usage.py` checks plus `output_retries_used`, `_agent_graph.py:361-378`; per-tool retries in `pydantic_ai_slim/pydantic_ai/tool_manager.py:256-265`), continuation caps for suspended provider turns (`pydantic_ai_slim/pydantic_ai/models/_continuation.py:70-97`), and cooperative cancellation (`pydantic_ai_slim/pydantic_ai/_cancel.py:42-257`). Deferred tools deliberately *pause* rather than loop: the run ends with a `DeferredToolRequests` output that must be resolved externally (`pydantic_ai_slim/pydantic_ai/_deferred.py:26-42`, `_tool_execution.py:1043-1050`).

Because there is no symmetric multi-party conversation inside one run, classic group-chat deadlock (two speakers waiting on each other forever) cannot occur structurally; the residual risks are unbounded delegate recursion across nested runs and provider-side stuck continuations — both of which have explicit guards.

## Rating

**8/10** — Clear, well-tested routing model (three end strategies with dedicated test suites at `tests/test_agent.py:4710-5737+`), layered termination safeguards (request/token/cost/tool-call limits, retry budgets, continuation ceilings) each with enforcement points and tests (`tests/test_usage_limits.py:49-88`), sophisticated cancellation semantics including sub-agent isolation (`_tool_execution.py:41-62`, `docs/agent.md:841-847`). Not 9-10 because: multi-agent routing itself has no framework-enforced contract (delegation correctness relies on user code passing `ctx.usage`/`deps`, `docs/multi-agent-applications.md:20-25`); there is no cycle detection for recursive delegation (a self-delegating agent is only stopped by usage limits); and cost-limit enforcement can silently degrade when pricing data is missing (warning only, `usage.py:528-535`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Run-level routing topology | Three-node agent graph; `CallToolsNode` docstring: "decides whether to end the run or make a new request" | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1816-1817` |
| Tool-call classification (the router) | Each `ToolCallPart` classified once by `get_tool_def(...).kind` into `'function'|'output'|'external'|'unapproved'|'unknown'` buckets | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:379-388` |
| Tool kinds | `ToolKind = Literal['function', 'output', 'external', 'unapproved']`; kind docs explain execution ownership | `pydantic_ai_slim/pydantic_ai/tools.py:539`, `tools.py:593-601` |
| End strategies (termination policy per response) | `EndStrategy = Literal['early','graceful','exhaustive']` with detailed ordering/retry semantics; default changed to `'graceful'` in v2 | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:99-130` |
| Strategy implementations | `_EarlyProcessor`, `_GracefulProcessor`, `_ExhaustiveProcessor` (parallel execution segmented by `sequential=True` barriers) | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:1054-1078`, `1082-1107`, `1111-1291` |
| Winner selection among outputs | First valid output by emission order wins; losers get status parts ("Output tool not used...") | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:1216-1222`, status strings at `32-38` |
| Retry-wins override | A `ModelRetry` from a function tool revokes a co-emitted winning output ("retry-wins") | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:33`, `_apply_retry_wins` at `895-908` region, trigger detection at `644-652` |
| Termination point | `_handle_final_result` appends trailing tool returns then returns `End(final_result)` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2276-2296` |
| Graph driver terminates on `End` | `Graph.run` loops `graph_run.next(event)` until `StopAsyncIteration` + `EndMarker` | `pydantic_graph/pydantic_graph/graph_builder.py:240-279` |
| Usage limits (runaway guard) | `UsageLimits`: `cost_limit`, `request_limit=50` (default), `tool_calls_limit`, token limits, `per_request_input_tokens_limit` | `pydantic_ai_slim/pydantic_ai/usage.py:418-472` |
| Limit enforcement points | Pre-request check `_agent_graph.py:1621`; post-response checks in `_append_response` at `1786-1794`; projected tool-call check at `_tool_execution.py:444-448`; always-on default noted at `_run_context.py:70-78` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1621`, `1786-1794`; `_tool_execution.py:444-448`; `_run_context.py:70-78` |
| Output-retry budget | `consume_output_retry` raises `UnexpectedModelBehavior` past `max_output_retries` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:361-378` |
| Per-tool retry budget | `ToolManager._check_max_retries` uses `>=` so negative budgets raise immediately "instead of looping forever" | `pydantic_ai_slim/pydantic_ai/tool_manager.py:256-265` |
| Model-response retry hook | Capability `after_model_request` raising `ModelRetry` builds a retry node via `_build_retry_node` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1738-1745`, `1797-1810` |
| Empty/thinking-only responses | No-actionable-output responses trigger `RetryPromptPart` or terminate if output allows `None` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1909-1975` |
| Truncated tool calls | `check_incomplete_tool_call` raises `IncompleteToolCall` instead of looping on `finish_reason='length'` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:345-359` |
| Continuation caps (deadlock backstop) | `MAX_GENERATION_CONTINUATIONS = 10` guards "a model that never leaves 'suspended'"; `MAX_BACKGROUND_POLLS = 1000` last-resort net for same-id polls "since a pending poll adds no tokens" | `pydantic_ai_slim/pydantic_ai/models/_continuation.py:70-97` |
| Suspended-turn resume routing | History ending in `suspended` response routes to resume path; new prompt on top of suspended turn raises `UserError` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:589-626` |
| Pending-tool-call precedence | Resume with history ending in a response with `tool_calls` routes straight to `CallToolsNode` before instructions | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:601-618` |
| Deferred tools pause the run | `DeferredToolRequests(calls=[...], approvals=[...])` becomes the run's final output; requires being in `output_schema` else `UserError` | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:964-1050`; types at `pydantic_ai_slim/pydantic_ai/_deferred.py:26-96` |
| Deferred result binding safety | Resume validates every eligible call has a result; duplicate `tool_call_id`s fail closed | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:392-423` |
| Inline resolution loop protection | A re-deferring approved call extends the pending batch again but surfaces it as output rather than looping | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:1017-1041` |
| Sub-agent cancellation isolation | `cancelled_sub_agent_return`: nested run's `RunCancelled` becomes a failed tool return, not parent teardown | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:41-62` |
| Cancellation machinery | `CancellationToken` (thread-safe, many-to-many), `RunCancellation.bind/resolve/release_issued`, external-vs-first-party arbitration | `pydantic_ai_slim/pydantic_ai/_cancel.py:42-257` |
| Outer-edge translation | `CancelledError` → `RunCancelled` only when first-party; nested-run cancellation re-stamped with outer history | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1765-1800` |
| Delegation pattern (routing by tool call) | Parent tool awaits `joke_generation_agent.run(..., usage=ctx.usage)`; usage shared so limits span the tree | `docs/multi-agent-applications.md:27-77` |
| Programmatic hand-off | App code sequences two agents; shared `UsageLimits(request_limit=15)` spans both | `docs/multi-agent-applications.md:205-336` |
| Output functions as handoff | "the model is forced to call one of them, the call ends the agent run... to hand off to another agent" | `docs/output.md:118-129` |
| Handoff context contract | `message_history` sharing between agents documented as the context-transfer mechanism | `docs/multi-agent-applications.md:211-215`, `docs/message-history.md:503` |
| Tests: termination strategies | ~30 tests e.g. `test_early_strategy_stops_after_first_final_result`, graceful/exhaustive variants | `tests/test_agent.py:4710`, `5655`, `5737` |
| Tests: usage-limit termination | `test_request_token_limit`, `test_total_token_limit`, `test_retry_limit`, streamed mid-stream enforcement | `tests/test_usage_limits.py:49-88`, `160` |
| Tests: deferred termination/resume | `test_tool_raises_call_deferred`, `test_resume_deferred_tool_with_invalid_output_call`, `test_deferred_tool_without_output_type` | `tests/test_tools.py:1639`, `1733`, `2147` |

## Answers to Dimension Questions

### 1. How are messages routed?

Two levels:

- **Inside a run**: messages flow through a fixed node sequence. `UserPromptNode.run` inspects history tail state and routes: deferred results → `CallToolsNode` resume (`_agent_graph.py:543-544`); interrupted request → dangling tool calls repaired with synthesized returns (`_agent_graph.py:546-556`); suspended response → resume path (`_agent_graph.py:589-600`); pending tool calls → `CallToolsNode` before any new instructions (`_agent_graph.py:610-614`); otherwise → `ModelRequestNode`. After the model responds, `ModelRequestNode._finish_handling` hands off to `CallToolsNode` (`_agent_graph.py:1747-1753`), which either loops back to `ModelRequestNode` with tool-return parts (`_agent_graph.py:2174-2182`) or ends with `End(final_result)`.
- **To tools**: `process_tool_calls` classifies each call by `ToolDefinition.kind` (`_tool_execution.py:379-388`): `'function'` executes locally and feeds the return to the model; `'output'` ends the run; `'external'`/`'unapproved'` are stubbed and surfaced as `DeferredToolRequests`; unknown names execute as error-generating placeholders so the model can correct.
- **Across agents**: no central router. Routing happens by (a) the parent model choosing a delegate tool, (b) application code sequencing runs with `message_history` handoff, or (c) explicit `pydantic_graph` edges.

### 2. How is the next speaker selected?

There is no speaker-selection subsystem — only one "speaker" (one agent/model) acts per node step. The closest analogues: within a turn, the model implicitly selects work by emitting tool calls, and the emission order is authoritative (first valid output tool by emission index wins, `_tool_execution.py:1216-1222`); across agents, selection is made by whichever mechanism the developer chose (delegate tool invocation, output-function branch, or graph edges). For parallel tool batches, ordering is controlled by `parallel_execution_mode` and `sequential=True` barriers (`tools.py:583-591`, `_tool_execution.py:1141-1149`).

### 3. How are handoffs managed?

Three contracts, all explicit:

- **Delegation**: plain `await other_agent.run(...)` inside a tool; the documented contract is to pass `usage=ctx.usage` so child consumption counts against the parent budget (`docs/multi-agent-applications.md:20`) and compatible `deps` (`docs/multi-agent-applications.md:99-104`). These are conventions, not enforced by the framework (see Gaps). Sync delegation from inside a run raises `UserError` (`docs/multi-agent-applications.md:79-82`).
- **Programmatic hand-off**: caller passes `result.all_messages()` as the next agent's `message_history`; union output types (e.g. `FlightDetails | Failed`) let the caller branch (`docs/multi-agent-applications.md:219-313`). `output_tool_return_content='Please try again.'` injects corrective feedback into shared history (`docs/multi-agent-applications.md:274-276`).
- **Forced handoff**: output functions — "the model is forced to call one of them, the call ends the agent run, and the result is not passed back to the model" (`docs/output.md:122`), making the output-function body the natural place to start the next agent.
- **External handoff**: `DeferredToolRequests` output + `DeferredToolResults` resume binds by `tool_call_id`, with fail-closed validation on mismatched/duplicate IDs (`_deferred.py:44-96`, `_tool_execution.py:408-420`).

Cancellation during handoff is precisely specified: a sub-agent cancelling itself becomes a failed tool return to the parent; whole-tree stop requires a shared `CancellationToken` (`_tool_execution.py:41-62`, `docs/agent.md:841-847`).

### 4. When does a group conversation terminate?

A run terminates when `CallToolsNode` reaches one of:

- A valid final output (text/schema/image validated, or winning output tool) → `End(FinalResult)` (`_agent_graph.py:1962-1964`, `2040-2049`, `2276-2296`).
- Deferred tool calls present → run ends with `DeferredToolRequests` output (human/service picks it up later) (`_tool_execution.py:1043-1050`).
- An exception: `UsageLimitExceeded` (pre-request `usage.py:492-514`, post-response `_agent_graph.py:1786-1794`, pre-tool-call `_tool_execution.py:444-448`), `UnexpectedModelBehavior` from exhausted output/per-tool retries (`_agent_graph.py:361-378`, `tool_manager.py:256-265`), `IncompleteToolCall` (`_agent_graph.py:345-359`), `ContentFilterError` (`_agent_graph.py:1934-1947`), or `RunCancelled` (`agent/__init__.py:1778-1786`).

There is no built-in "group conversation" that outlives a run; cross-agent conversations continue only by explicit relaunching with shared history, where the caller owns termination (e.g. a bounded `for _ in range(3)` retry loop, `docs/multi-agent-applications.md:259-277`). So yes — a conversation terminates without human intervention whenever budgets are configured; even with zero configuration the always-on default `request_limit=50` bounds any single run (`usage.py:429`, `_run_context.py:74-78`).

### 5. Is deadlock possible?

Classic multi-party deadlock cannot arise structurally: a run is a single-driver acyclic node loop, and the graph driver simply iterates until `End` (`graph_builder.py:274-279`). The realistic livelock/runaway vectors and their guards:

- **Infinite model↔tool ping-pong**: bounded by `request_limit` (default 50) and token/cost limits (`usage.py:429-459`).
- **Retry storms**: separate budgets for output retries (`consume_output_retry`, `_agent_graph.py:361-378`), per-tool retries (`tool_manager.py:256-265`), and model-request retries (`_build_retry_node`, `_agent_graph.py:1797-1810`); all eventually raise rather than loop.
- **Recursive delegation** (an agent delegating to itself transitively): usage accrual via shared `ctx.usage` makes the parent's limits catch runaway children, but there is **no explicit delegation-depth detector** — an agent that delegates without sharing usage could recurse until asyncio stack/resource exhaustion (see Gaps).
- **Provider-stuck suspended turns**: `MAX_GENERATION_CONTINUATIONS=10` vs `MAX_BACKGROUND_POLLS=1000`, chosen deliberately because a same-id poll adds no tokens and would evade usage limits (`models/_continuation.py:70-97`) — a thoughtful, documented deadlock analysis.
- **Waiting forever on humans/services**: impossible by construction — deferred calls end the run; nothing blocks awaiting external results (`_tool_execution.py:1043-1050`). Re-deferral after inline resolution also terminates by surfacing the batch as output (`_tool_execution.py:1030-1041`).
- **Orphaned async work**: sibling tool tasks are cancelled and drained on any exception or consumer close (`_tool_execution.py:1209-1214`, `828-837`); leaked issued cancellations are released to avoid contaminating later work (`_cancel.py:244-257`).

## Architectural Decisions

1. **Routing as a tiny explicit state machine, not a registry.** All intra-run control flow funnels through three node types whose transitions are returned values (`_agent_graph.py:1839-1851`), making the routing table auditable in one file and steerable by capabilities wrapping streams (`_agent_graph.py:1874-1889`).
2. **Termination policy parameterized per run** via `end_strategy` ('early'/'graceful'/'exhaustive') with the v2 default changed to 'graceful' and the difference documented exhaustively (`_agent_graph.py:99-130`).
3. **Classification-before-execution router.** Every tool call is classified into exactly five kinds before any side effect (`_tool_execution.py:374-430`), so termination decisions (skip vs execute vs defer) never depend on execution outcomes of siblings.
4. **Budgets as the universal loop-breaker.** Rather than step counters sprinkled through logic, one `UsageLimits` object is checked at three lifecycle points (before request, after response, before tool batch) (`usage.py:492-560`, `_agent_graph.py:1621`, `1786-1794`).
5. **Pause-don't-block for externals.** Deferred tools convert potential blocking waits into a terminating output type, moving wait responsibility to the application (`_deferred.py:26-42`).
6. **Precise cancellation attribution.** First-party vs external cancellation is disambiguated by counting issued cancels and consuming them via `Task.uncancel()` (`_cancel.py:203-242`), and nested-run cancellations are isolated as tool failures (`_tool_execution.py:41-62`) — unusual care for multi-agent trees.

## Notable Patterns

- **Retry-wins**: a successful output tool's result is revoked if a sibling function tool raised `ModelRetry` in the same round — the retry prompt replaces the winner's status part (`_tool_execution.py:33`, `895-908`) — preventing premature termination while corrections are pending.
- **Barrier segmentation** of parallel tool execution (`_segment_by_barriers`, `_tool_execution.py:232`, applied at `794-797`, `1147-1149`) gives deterministic ordering hooks inside otherwise concurrent dispatch.
- **Fail-closed ID matching**: duplicate or unmatched `tool_call_id`s on deferred resume raise `UserError` instead of best-effort binding (`_tool_execution.py:405-420`, `966-973`).
- **Status-part protocol in history**: skipped/lost outputs leave machine-readable `ToolReturnPart` strings (`_tool_execution.py:30-38`) so the model observes why its output wasn't used — routing decisions become visible in conversation state.
- **Continuation merge taxonomy**: `MergeMode` distinguishes same-id poll, fresh-generation replace, and accumulate, each with its own ceiling rationale (`models/_continuation.py:99-112`).

## Tradeoffs

- **Convention over contract for delegation**: passing `usage=ctx.usage` and compatible `deps` is documented (`docs/multi-agent-applications.md:20-25`, `99-104`) but unchecked; omitting them silently decouples child budgets from parents. Temporal activities copy the run context, breaking usage propagation entirely (`docs/multi-agent-applications.md:84-85`).
- **Emission-order authority**: picking the first valid output by emission index (`_tool_execution.py:1216-1222`) is simple and model-aligned but means a "better" later output is discarded; `'exhaustive'` mitigates by running everything yet still selects by order.
- **Default-on request limit (50)** trades surprise terminations for guaranteed termination; long legitimate runs must remember to raise it (`usage.py:429`).
- **Cost limits are advisory when pricing data is absent** — warning-only degradation (`usage.py:528-535`) favors availability over strict enforcement.
- **Text-vs-tools precedence subtlety**: plain text never preempts co-emitted tool calls except under `'early'` with schema-validated text, a rule requiring a long comment block to justify (`_agent_graph.py:2016-2031`, `2118-2131`) — correct but cognitively heavy.

## Failure Modes / Edge Cases

- **Truncated tool call** (`finish_reason='length'` mid-args): raises `IncompleteToolCall` with actionable guidance instead of retrying (`_agent_graph.py:345-359`); empty responses under length limit raise immediately (`_agent_graph.py:1927-1931`).
- **Thinking-only responses** don't force retries when output allows `None` — avoids punishing models that finish via tools (`_agent_graph.py:1915-1970`).
- **New prompt over suspended turn** is rejected with `UserError` so a provider-side job isn't leaked (`_agent_graph.py:619-626`).
- **Interrupted tool-execution requests** get synthesized tool returns so resumed histories never carry dangling calls (`_agent_graph.py:546-556`, repair at `2816+`).
- **Consumer bails mid-stream**: `CallToolsNode.stream` closes the wrapped generator, records partial tool returns as an `interrupted` request (`_agent_graph.py:2158-2172`), and re-raises the original stream error rather than an assertion (`_agent_graph.py:1846-1851`).
- **Known race window in cancellation attribution**: user-initiated `Task.uncancel()` followed by a matching external cancel can misattribute first-party status (#7240), documented at `_cancel.py:218-223`.
- **Nested `RunCancelled` escaping to parent** is re-stamped with parent history to avoid resuming the wrong conversation, with the deeper semantic question tracked upstream (`agent/__init__.py:1771-1777`, issue #7199).

## Future Considerations

- Add an opt-in delegation-depth or delegation-graph detector for recursive delegation cycles (currently only budget-based containment).
- Enforce (or at least warn about) delegation hygiene: detect `await other_agent.run(...)` inside tools lacking `usage=ctx.usage` so cross-agent budget coupling doesn't depend on docs compliance.
- Consider making `request_limit`'s exhaustion recoverable via a capability hook (today `UsageLimitExceeded` is terminal), enabling summarization-and-continue patterns natively.
- Resolve #7240 issuance-identity tracking to close the cancellation-attribution race (`_cancel.py:218-223`).

## Questions / Gaps

- **No evidence found** of any framework-level speaker-selection, round-robin, or group-chat routing module; searches for `speaker|round.robin|group chat` across `pydantic_ai_slim/pydantic_ai/**` matched only realtime-audio `SpeechPart.speaker` validation (`messages.py:2082-2092`). Multi-agent coordination is intentionally delegated to the four documented patterns.
- **No evidence found** of a delegation-cycle/deadlock detector spanning nested agent runs; containment relies on usage accounting, which is conventionally coupled.
- The exact behavior when a `'graceful'`/'exhaustive' run receives structured-text output alongside *only* deferred calls is handled (`_agent_graph.py:2024-2025` comment says deferred calls are left to normal processing), but I did not trace a dedicated test for that specific combination; nearest coverage is `test_early_strategy_does_not_preempt_deferred_tool_calls` (`tests/test_agent.py:5024`).
- Cost-limit enforcement quality depends on genai-prices coverage per model; beyond the warning path (`usage.py:528-535`) there is no fallback estimator visible in the studied files.

---

Generated by `dimensions/15.02-message-routing-and-termination` against `pydantic-ai`.

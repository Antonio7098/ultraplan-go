# Source Analysis: pydantic-ai

## Dimension 06.05: Objective and Progress Tracking

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (pydantic, pydantic-graph, OpenTelemetry, anyio; packages: `pydantic_ai_slim`, `pydantic_graph`, `pydantic_evals`, `clai`) |
| Analyzed | 2026-08-26 |

## Summary

Pydantic AI has **no first-class "goal" object** — there is no plan, milestone list, or task tree. Instead, the goal of a run is represented implicitly but *structurally*: the run's target is a typed output value (`output_type` schema), and completion is defined as reaching an `End[FinalResult]` graph node carrying a fully validated output (`pydantic_ai_slim/pydantic_ai/result.py:1031-1044`, `pydantic_ai_slim/pydantic_ai/run.py:143-161`). Progress is tracked through several concrete, machine-readable mechanisms rather than model self-reports:

1. **Budget counters** — `RunUsage` (requests, successful tool calls, tokens, cost) accumulated in `GraphAgentState` and enforced against `UsageLimits` before each request, after each response, per streamed chunk, and before tool-call batches (`pydantic_ai_slim/pydantic_ai/usage.py:337-390`, `pydantic_ai_slim/pydantic_ai/usage.py:417-573`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:298-334`).
2. **Step markers** — `run_step`, `retry`/`max_retries`, and per-tool retry counts exposed on `RunContext` so tools/hooks can see where in the run they are (`pydantic_ai_slim/pydantic_ai/_run_context.py:97-116`).
3. **Typed event streams** — `PartStartEvent`/`PartDeltaEvent`/`PartEndEvent`/`FinalResultEvent` for streaming, plus `FunctionToolCallEvent`/`FunctionToolResultEvent` and `DeferredToolRequestsEvent` for tool lifecycle, all discriminated by `event_kind` (`pydantic_ai_slim/pydantic_ai/messages.py:3854-3920`, `pydantic_ai_slim/pydantic_ai/messages.py:3946-4047`).
4. **Blocker recording in history** — failed attempts become `RetryPromptPart`s appended to the message history, and interrupted/suspended turns are persisted with explicit `state` markers (`pydantic_ai_slim/pydantic_ai/messages.py:1637-1697`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2158-2172`).
5. **Observability surfaces** — OTel agent-run/model-request/tool spans, UI protocol adapters (AG-UI, Vercel AI), and a CLI `/usage` command render progress externally (`pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:152-234`, `pydantic_ai_slim/pydantic_ai/ui/_event_stream.py:96-140`, `pydantic_ai_slim/pydantic_ai/_cli/__init__.py:484-511`).

Completion is checked mechanically — schema validation, output validators, and usage limits — not semantically. Independent semantic judgement of success exists only outside the run loop, in the separate `pydantic_evals` package (evaluators incl. LLM-as-a-judge) and its online-evaluation capability. The system is deliberately honest about activity vs. progress: activity (tokens, requests) is measured precisely; goal progress is inferred solely from whether the structured output finally validates.

## Rating

**7 / 10**

Rationale: The framework earns the 7–8 band ("clear model with tests, explicit interfaces, and operational safeguards") for its *resource* progress model: `RunUsage`/`UsageLimits` are explicit public interfaces with dense test coverage (`tests/test_usage_limits.py:44-927`, 56 tests), enforcement points exist at every boundary (pre-request `_agent_graph.py:1621`, post-response `:1787-1794`, mid-stream `result.py:1052-1060`, pre-tool-batch `_tool_execution.py:444-448`, continuation `_agent_graph.py:875-893`), and blockers/retries are durably recorded in replayable message history. Completion criteria are unambiguous and cannot be self-declared by the model.

It does not reach 8–10 because: (a) there is no first-class goal/milestone representation — no object answers "how close to done is this task?", only "how much budget was consumed"; (b) final success is never independently verified *within* a run — a structurally valid output ends the run regardless of whether it satisfies user intent (semantic checking is delegated to the separate offline/batch `pydantic_evals` package); (c) progress semantics are provider-reported token counts, which are trusted inputs, not independently audited.

## Evidence Collected

Every entry includes a file path with line numbers relative to the source root `studies/agent-harness-study/sources/pydantic-ai`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Goal representation | Run output type drives completion: `FinalResult` marker dataclass stores validated output + originating tool name/id | `pydantic_ai_slim/pydantic_ai/result.py:1031-1044` |
| Goal representation | `AgentRun.result` becomes available only when graph reaches `End[FinalResult]` | `pydantic_ai_slim/pydantic_ai/run.py:143-161` |
| Goal representation | Graph terminal state: `_handle_final_result` returns `End(final_result)` after appending tool returns to history | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2276-2296` |
| Goal representation | Streaming-only shortcut node `SetFinalResult` immediately ends the run once streaming produced a final result | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2301-2310` |
| Run state (progress ledger) | `GraphAgentState`: `message_history`, `usage: RunUsage`, `output_retries_used`, `run_step`, `run_id`, `conversation_id` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:298-334` |
| Usage counters | `RunUsage` fields: `requests`, `tool_calls` ("successful tool calls executed"), input/output/cache/audio tokens, `cost` | `pydantic_ai_slim/pydantic_ai/usage.py:337-369` |
| Usage extraction | `RequestUsage.extract()` pulls provider-reported usage via genai-prices; returns zeroed usage if unpriceable | `pydantic_ai_slim/pydantic_ai/usage.py:303-334` |
| Budget limits | `UsageLimits`: cost/request/tool-call/token limits + per-request input limit; default `request_limit=50` | `pydantic_ai_slim/pydantic_ai/usage.py:417-472` |
| Limit checks | `check_before_request`, `check_tokens`, `check_before_tool_call`, `check_per_request_input_tokens` raise `UsageLimitExceeded` | `pydantic_ai_slim/pydantic_ai/usage.py:492-573` |
| Limit enforcement points | Pre-request check in `_prepare_request`; post-response checks in `_append_response` (tokens, cost, per-request input) | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1603-1621`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1778-1795` |
| Mid-stream enforcement | `_get_usage_checking_stream_response` re-checks token limits on every streamed chunk | `pydantic_ai_slim/pydantic_ai/result.py:1047-1062` |
| Continuation enforcement | `_check_continuation_usage` checks provisional totals mid-turn for Anthropic `pause_turn`/OpenAI background continuations | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:875-893` |
| Tool-call budget projection | Pre-execution projection: `projected_usage.tool_calls += len(function_indices)` then `check_before_tool_call` | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:444-448` |
| Tool success counting | `usage.tool_calls += 1` only after successful execution (and on `SkipToolExecution`) | `pydantic_ai_slim/pydantic_ai/tool_manager.py:984`, `pydantic_ai_slim/pydantic_ai/tool_manager.py:1025` |
| Step counter | `ctx.state.run_step += 1` at each model request preparation; exposed as `RunContext.run_step` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1471`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1649`, `pydantic_ai_slim/pydantic_ai/_run_context.py:115-116` |
| Retry markers | `consume_output_retry` increments `output_retries_used`, raises `UnexpectedModelBehavior` past budget | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:361-378` |
| Blocker recording | `RetryPromptPart` documents all six retry causes (arg validation failure, tool `ModelRetry`, unknown tool, wrong text output, output validation failure, validator `ModelRetry`) and lands in history | `pydantic_ai_slim/pydantic_ai/messages.py:1637-1697` |
| Blocker recording (truncation) | `check_incomplete_tool_call` raises `IncompleteToolCall` when finish_reason='length' truncated a tool call; distinguishes budget exhaustion from truncation | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:345-359`, `pydantic_ai_slim/pydantic_ai/exceptions.py:664-665` |
| Partial-progress persistence | On exception during tool execution, collected tool returns are appended with `state='interrupted'` for later inspection/resume | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2158-2172` |
| Cancellation snapshot | `RunCancelled` carries detached snapshot: messages, new_message_index, usage, metadata, run/conversation ids | `pydantic_ai_slim/pydantic_ai/exceptions.py:268-330`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2381-2393` |
| Progress events | Stream event union `PartStartEvent/PartDeltaEvent/PartEndEvent/FinalResultEvent` discriminated by `event_kind` | `pydantic_ai_slim/pydantic_ai/messages.py:3848-3920` |
| Milestone event | `FinalResultEvent`: emitted mid-stream when response part will produce the final result (`models/__init__.py` detects it from output-schema match) | `pydantic_ai_slim/pydantic_ai/messages.py:3904-3915`, `pydantic_ai_slim/pydantic_ai/models/__init__.py:1982-1994` |
| Tool lifecycle events | `FunctionToolCallEvent`/`OutputToolCallEvent` (+ `args_valid`) and `FunctionToolResultEvent`/`OutputToolResultEvent` | `pydantic_ai_slim/pydantic_ai/messages.py:3946-4047` |
| Blocked-on-human event | `DeferredToolRequestsEvent` signals calls paused awaiting approval or external execution | `pydantic_ai_slim/pydantic_ai/messages.py:4059-4065` |
| Streaming partial outputs | `AgentStream.stream_output` yields partial validated outputs (`allow_partial=True`), swallowing transient ValidationError/ModelRetry until final strict validation | `pydantic_ai_slim/pydantic_ai/result.py:74-101` |
| Stream completeness flag | `is_complete` set when stream consumed; guards double-append | `pydantic_ai_slim/pydantic_ai/result.py:484`, `pydantic_ai_slim/pydantic_ai/result.py:1019-1028` |
| Completion = validated output | Output validate hooks convert `ValidationError`/`ModelRetry` into `ToolRetryError` → retry prompt back to model | `pydantic_ai_slim/pydantic_ai/_output.py:121-173` |
| Text can't fake completion | Under `end_strategy='early'`, plain text only wins if it validates against a schema processor; unstructured text explicitly must not silently end the run | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2118-2232` |
| Run-level final cost check | After run completes: `usage_limits.check_cost(result.usage)` (with warn if cost unavailable) | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1839` |
| Traces/UI | Instrumentation capability creates `invoke_agent` span with `gen_ai.agent.name/call.id/conversation.id`; run-end attributes include full message history | `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:152-259` |
| Traces/UI metrics | OTel histograms `gen_ai.client.token.usage`, `operation.cost`, `gen_ai.client.operation.time_to_first_chunk` | `pydantic_ai_slim/pydantic_ai/models/instrumented.py:167-203` |
| Traces/UI adapters | `UIEventStream` transforms native events into AG-UI/Vercel-AI protocol events; tracks pending tool calls and open parts to close them on error | `pydantic_ai_slim/pydantic_ai/ui/_event_stream.py:96-140` |
| Traces/UI CLI | `/usage` slash command renders session totals (turns, tokens, requests, tool calls) or JSON | `pydantic_ai_slim/pydantic_ai/_cli/__init__.py:484-511` |
| Independent evaluation | `Evaluator` base class + `EvaluationResult{name,value,reason}`; bool=assertion, int/float=score, str=label | `pydantic_evals/pydantic_evals/evaluators/evaluator.py:30-100` |
| Model-judgement evals | `LLMJudge` evaluator (LLM-as-a-judge) among common evaluators | `pydantic_evals/pydantic_evals/evaluators/common.py:225` |
| Eval progress metrics | `TaskRun` accumulator extracts `requests`, `cost`, token usage from OTel span tree per evaluated task run | `pydantic_evals/pydantic_evals/_task_run.py:14-77` |
| Online evals | `OnlineEvaluation` capability runs evaluators (e.g. `OutputNotEmpty`) inside live runs | `pydantic_evals/pydantic_evals/online_capability.py:52-78` |
| Tests (limits) | `test_retry_limit` asserts request_limit=1 exceeded error; `test_tool_call_limit`, `test_output_tool_not_counted`, streamed token-limit tests | `tests/test_usage_limits.py:74-87`, `tests/test_usage_limits.py:735`, `tests/test_usage_limits.py:793`, `tests/test_usage_limits.py:89-179` |

## Answers to Dimension Questions

### 1. What is the goal?

There is no declarative goal object. The goal of a run is the production of a value matching the agent's declared `output_type`. Internally that is a `FinalResult(output, tool_name, tool_call_id)` carried out of the graph inside an `End` node (`pydantic_ai_slim/pydantic_ai/result.py:1031-1044`, `pydantic_ai_slim/pydantic_ai/run.py:143-161`). For deferred-tool flows, the run may instead legitimately terminate with `DeferredToolRequests` (approval/external execution pending), which is a designed alternative terminal state, not a failure (`pydantic_ai_slim/pydantic_ai/messages.py:4059-4065`, `pydantic_ai_slim/pydantic_ai/_deferred.py:30-32`). Anything about *user intent* beyond "a valid output was produced" lives entirely in the prompt/schema the developer supplies.

### 2. How is progress measured?

Four concrete ledgers, all framework-computed:

- **Resource consumption**: `RunUsage.requests`, `.tool_calls` (only successful executions count, `pydantic_ai_slim/pydantic_ai/tool_manager.py:984,1025`), token buckets and best-effort USD cost summed from provider responses (`pydantic_ai_slim/pydantic_ai/usage.py:337-414`).
- **Loop position**: `run_step` incremented per model request (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1471,1649`) and `retry`/`max_retries` plus per-tool `retries` dict surfaced on `RunContext` (`pydantic_ai_slim/pydantic_ai/_run_context.py:97-116`).
- **Message history growth**: `new_messages()` slices by `new_message_index` keyed off stamped `run_id`s (`pydantic_ai_slim/pydantic_ai/run.py:178-183`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:264-296`).
- **Streaming granularity**: part start/delta/end events and debounced partial `ModelResponse` snapshots give sub-request progress (`pydantic_ai_slim/pydantic_ai/messages.py:3848-3920`, `pydantic_ai_slim/pydantic_ai/result.py:103-120`).

Progress is thus measured by tests of *budget and structure*, not by distance-to-goal. There is no percent-complete, no milestone record, no plan-state diff.

### 3. Can the model fake progress?

Largely no, with two caveats. The model cannot declare itself done: ending requires either (a) an output-tool call whose arguments pass schema validation, or (b) text that validates under a structured-output processor; plain prose is explicitly prevented from silently preempting pending tool calls under `end_strategy='early'` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2118-2138`, docstring at `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2197-2203`: "Plain, unstructured text output ... accepts *any* text, so the model's preamble ... must not silently win"). Counters are computed from actual executions and provider-reported usage, not model claims. Caveats: (i) the model *can* burn budget arbitrarily (each extra request/tool call consumes the shared `UsageLimits` envelope), so it can exhaust progress without producing any; (ii) token/cost figures are trusted from the provider (`RequestUsage.extract`, `pydantic_ai_slim/pydantic_ai/usage.py:303-334`) — the library normalizes and sums them but does not audit them; when pricing data is missing while a `cost_limit` is set, enforcement degrades to a warning (`CostNotFoundWarning`, `pydantic_ai_slim/pydantic_ai/usage.py:528-535`).

### 4. Are blockers recorded?

Yes, durably and in-band. Failures become `RetryPromptPart`s in the replayable message history with cause taxonomy documented in the class docstring (`pydantic_ai_slim/pydantic_ai/messages.py:1637-1650`); truncation blockers are distinguished from retry-budget exhaustion via `IncompleteToolCall` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:345-359`); human-in-the-loop blocks surface as `DeferredToolRequestsEvent` and a resumable `DeferredToolRequests` output (`pydantic_ai_slim/pydantic_ai/messages.py:4059-4065`); partially completed steps are persisted with `state='interrupted'` requests (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2158-2172`) and suspended provider turns with `state='suspended'` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:589-600`); cancellation produces a `RunCancelled` snapshot preserving everything completed so far (`pydantic_ai_slim/pydantic_ai/exceptions.py:268-330`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2381-2393`). What is *not* recorded is a dedicated "blocked because X, needs Y" milestone object — blockers are conversation content plus exceptions, not structured task records.

### 5. Is final success independently checked?

Within a run: only mechanically. Success means the output survived Pydantic schema validation, user output validators, and usage-limit checks (`pydantic_ai_slim/pydantic_ai/_output.py:126-173`, `pydantic_ai_slim/pydantic_ai/usage.py:492-573`). No component judges whether the validated output actually fulfills the user's request; a confidently wrong answer that type-checks is a successful run. Independent semantic verification exists in sibling package `pydantic_evals`: batch `Evaluator`s (including `LLMJudge`, `pydantic_evals/pydantic_evals/evaluators/common.py:225`) score outputs against datasets with `EvaluationResult{name, value, reason}` (`pydantic_evals/pydantic_evals/evaluators/evaluator.py:60-81`), report-level statistical evaluators analyze whole experiments (`pydantic_evals/pydantic_evals/evaluators/report_common.py:107-316`), and online evaluators can run inside production runs via `OnlineEvaluation` capability (`pydantic_evals/pydantic_evals/online_capability.py:52-78`). But this is opt-in, decoupled from the run loop, and does not gate run completion.

## Architectural Decisions

- **Completion as graph terminal state**: The agent loop is a pydantic-graph run whose output type is `FinalResult[OutputDataT]`; `AgentRun.result` is `None` until `End` is reached (`pydantic_ai_slim/pydantic_ai/run.py:98-161`). This makes "done" a typed, inspectable property of the state machine rather than a convention.
- **Single mutable ledger shared by reference**: `GraphAgentState.usage` and `message_history` are mutated in place and shared into every `RunContext` copy, with explicit invariant comments forbidding reassignment that would fork identity (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:413-421`, `2369-2378`). This keeps counters coherent across capability hooks.
- **Defense-in-depth limit enforcement**: Checks fire pre-request, post-response, per-stream-chunk, per-tool-batch (with projected usage including in-flight calls), and on continuation merges (`pydantic_ai_slim/pydantic_ai/usage.py:492-573`, `pydantic_ai_slim/pydantic_ai/result.py:1052-1060`, `pydantic_ai_slim/pydantic_ai/_tool_execution.py:444-448`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:875-893`). The docstring at `pydantic_ai_slim/pydantic_ai/usage.py:421-424` states the split explicitly: request counts checked before each request, token counts after each response.
- **Blockers as message parts, not side channels**: Retries/interruptions are encoded in the same serialized history used for replay and durable execution, so progress and blockers survive checkpointing uniformly.
- **Separation of run-time tracking from evaluation**: `pydantic_evals` is a distinct package; the run loop never consults evaluators. Semantic success judgement is deliberately out-of-band.

## Notable Patterns

- **Honest accounting of "successful" work**: `RunUsage.tool_calls` is documented as "Number of *successful* tool calls" (`pydantic_ai_slim/pydantic_ai/usage.py:347-348`) and is incremented only post-success (`pydantic_ai_slim/pydantic_ai/tool_manager.py:1025`); failures instead feed `failed_tools`/retry budgets (`pydantic_ai_slim/pydantic_ai/tool_manager.py:1021-1023`).
- **Retry-wins invariant**: under graceful/exhaustive end strategies, if any function tool produced a `RetryPromptPart`, a co-produced final result is revoked so the model addresses the blocker next round (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:285,892-900`) — completion is postponed when unresolved blockers exist in the same step.
- **Partial output streaming with strict-final gating**: `stream_output` yields `allow_partial=True` validations opportunistically, suppressing transient errors, then performs one strict `allow_partial=False` validation guaranteed to be the last yield (`pydantic_ai_slim/pydantic_ai/result.py:93-101`) — consumers see progress without being able to mistake it for completion.
- **Mid-stream milestone detection**: `FinalResultEvent` is emitted the moment a streamed part matches the output schema, before validation completes (`pydantic_ai_slim/pydantic_ai/models/__init__.py:1982-1994`) — an early "this will be the answer" signal for UIs, kept distinct from the actual validated result.
- **Distinguishing unknown-cost from zero-cost**: `cost: Decimal | None` keeps "unpriced" distinguishable from free (`pydantic_ai_slim/pydantic_ai/usage.py:130-135`).
- **Evaluation metrics derived from spans**: `extract_span_tree_metrics` recomputes requests/cost/tokens from OTel attributes rather than a parallel tracker, so eval reports share one source of truth with tracing (`pydantic_evals/pydantic_evals/_task_run.py:59-74`).

## Tradeoffs

- **Structural vs. semantic success**: cheap, deterministic, provider-agnostic completion checks; but "ran to completion" ≠ "achieved the objective". Users needing outcome verification must build or adopt `pydantic_evals` pipelines themselves.
- **No goal object**: maximal flexibility (any Python type can be the goal) at the cost of zero built-in vocabulary for milestones, plans, or percent-complete; multi-step objectives must be decomposed manually into multiple runs or graph nodes.
- **Provider-trusted usage numbers**: summing provider-reported tokens avoids double-counting and works across providers, but means budget enforcement is only as truthful as the provider's accounting (mitigated slightly by opt-in pre-request counting, `count_tokens_before_request`, `pydantic_ai_slim/pydantic_ai/usage.py:459-472`).
- **History-as-blocker-log**: elegant for replay/durability, but answering "what went wrong and why" requires parsing message parts and exceptions rather than reading a structured blocker table.
- **Warning-degradation on unpriced models**: setting a `cost_limit` for an unpriced model silently downgrades enforcement to a warning (`pydantic_ai_slim/pydantic_ai/usage.py:528-535`) — visible, but easy to miss.

## Failure Modes / Edge Cases

- **Truncated tool call masquerading as retry exhaustion**: when a response hits `max_tokens` mid-tool-call, `consume_output_retry` first calls `check_incomplete_tool_call` so the raised error names truncation and suggests raising `max_tokens` instead of a misleading retry-budget message (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:374-378`).
- **Continuation double-counting**: continuation segments accumulate provisional usage checked against a throwaway copy, committed exactly once on merge, so Anthropic `pause_turn`/background chains neither bypass nor double-apply token caps (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:875-885`).
- **Sub-agent usage drift**: nested `agent.run()` calls do not automatically increment parent usage; tests pin this behavior and users must merge `RunUsage` explicitly (`tests/test_usage_limits.py:209,402`).
- **Output tool not counted against tool-call limits**: output (final-answer) tools are excluded from `tool_calls_limit` projections so finishing is never blocked at the limit boundary (`tests/test_usage_limits.py:793,927`, `pydantic_ai_slim/pydantic_ai/_tool_execution.py:444-448` projects only `function_indices`).
- **Streamed limit violations mid-flight**: token limits are enforced chunk-by-chunk during streaming (`pydantic_ai_slim/pydantic_ai/result.py:1052-1060`), tested including the case where the output-token limit trips before stream end (`tests/test_usage_limits.py:160-179`).
- **Cancellation preserves partial progress**: `RunCancelled.all_messages()/usage` let callers resume rather than lose completed steps (`pydantic_ai_slim/pydantic_ai/exceptions.py:364-434`).

## Future Considerations

- A lightweight structured goal/milestone primitive (e.g., optional checklist on `GraphAgentState` surfaced through events) would let harnesses distinguish goal progress from resource consumption without sacrificing the current minimalism.
- Surfacing `UsageLimits` remaining budget as a standard attribute on run spans/UI events would make budget-based progress externally observable without custom computation (today consumers combine `ctx.usage` + `ctx.usage_limits` themselves, per `pydantic_ai_slim/pydantic_ai/_run_context.py:70-82`).
- An opt-in hook binding an in-run judge (from `pydantic_evals`) to output validators would close the gap between structural completion and outcome verification while keeping the default loop unchanged.

## Questions / Gaps

- No evidence found of any milestone/task-list/plan-tracking API anywhere in the source; searches across `pydantic_ai_slim/pydantic_ai/` for goal/milestone/plan/percent concepts returned nothing relevant (only `run_step`, retries, usage).
- Token/cost figures are taken from provider responses; no independent audit or reconciliation mechanism was found (`pydantic_ai_slim/pydantic_ai/usage.py:303-334`).
- Whether `pydantic_evals` online evaluations can *gate* a run (fail it) rather than merely observe was not established from the code read here; `OnlineEvaluation` appears observational (`pydantic_evals/pydantic_evals/online_capability.py:52-78`).
- Realtime sessions enforce the same limits (`pydantic_ai_slim/pydantic_ai/realtime/_session.py:2236-2295`) but their UI progress reporting was not analyzed in depth (out of scope for this dimension's core loop focus).

---

Generated by dimension `06.05-objective-and-progress-tracking` against `pydantic-ai`.

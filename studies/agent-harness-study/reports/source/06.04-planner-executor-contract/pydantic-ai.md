# Source Analysis: pydantic-ai

## Dimension 06.04: Planner/Executor Contract

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python 3.10+ / Pydantic, pydantic-graph, uv workspace (`pydantic_ai_slim`, `pydantic_graph`) |
| Analyzed | 2026-08-27 |

## Summary

Pydantic AI has no standalone `Planner` class. Planning is performed by the LLM via the agent graph's `ModelRequestNode` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1376`) which emits a `ModelResponse` containing `ToolCallPart`/`TextPart` records; execution is performed by `CallToolsNode` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1815`) delegating to `_tool_execution.process_tool_calls` (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:254`) and `ToolManager` (`pydantic_ai_slim/pydantic_ai/tool_manager.py:143`). The contract is the LLM's `ModelResponse` ↔ `ToolCallPart`/`ToolDefinition` ↔ `ValidatedToolCall`/`ModelRequest` loop, strongly typed (Pydantic `ToolDefinition`, `OutputSchema`, `ModelRequestParameters`, `messages.ToolCallPart`) and validated on every turn. The executor never trusts raw planner output; it validates schemas, availability, and retry budgets, can reject, rewrite, skip, or defer calls, and feeds structured failure back via `RetryPromptPart`/`ToolReturnPart`. Execution proceeds safely on a bad plan because validation, capability hooks, deferred-tool gating, and `UsageLimits`/retry ceilings isolate failures and force model-side correction.

## Rating

**Rating: 8 / 10**

Pydantic AI presents a clear, explicitly typed planner/executor boundary with exhaustive validation, per-tool retry budgets, availability-refusal logic, hook interception at every lifecycle point, and durable-execution replay fidelity. The graph (`build_agent_graph`) makes the Planner→Executor→Planner loop first-class and observable. Deduction from 9/10 because the contract is implicit in `ModelResponse`/`ToolCallPart` rather than a dedicated `Plan` schema document, cross-agent planning is application-level (no first-party planner delegate primitive), and bad-plan behavior depends on model respect for `RetryPromptPart` with no static plan linter.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Planner output schema | `ModelResponse` dataclass with `parts: Sequence[ModelResponsePart]`, `finish_reason`, `provider_details`, state (`complete`/`suspended` etc.) | `pydantic_ai_slim/pydantic_ai/messages.py:2540-2620` |
| Planner output: tool calls | `ToolCallPart(BaseToolCallPart)` with `tool_name`, `args: str|dict`, `tool_call_id`, `tool_kind`, `@discriminator(part_kind='tool-call')`, `narrow_type()` promotion | `pydantic_ai_slim/pydantic_ai/messages.py:2277-2296` |
| Planner alternative outputs | `TextPart`, `ThinkingPart`, `FilePart`, `NativeToolCallPart`, `ToolSearchCallPart`, `LoadCapabilityCallPart` in `ModelResponsePart` discriminated union | `pydantic_ai_slim/pydantic_ai/messages.py:2521-2536` |
| Planner request parameters (what planner sees) | `ModelRequestParameters` dataclass: `function_tools: list[ToolDefinition]`, `output_tools`, `output_mode`, `output_object`, `tool_visibility: dict[str,ToolVisibility]`, `revealed_tool_names`, `instruction_parts` | `pydantic_ai_slim/pydantic_ai/models/__init__.py:174-292` |
| Executor interface: manager | `ToolManager` generic `ToolManager[DepsT]` with `validate_tool_call`, `execute_tool_call`, `validate_output_tool_call`, `handle_call`, `for_run_step`, `parallel_execution_mode`, `is_sequential` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:143-739` |
| Executor: validation boundary | `ValidatedToolCall[DepsT]` dataclass separating `call`, `tool`, `ctx`, `args_valid`, `validated_args`, `validation_error`, `deferral` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:56-95` |
| Executor: tool definition | `ToolDefinition` dataclass: `name`, `parameters_json_schema: ObjectJsonSchema`, `description`, `strict`, `sequential`, `kind: ToolKind`, `tool_kind: ToolPartKind`, `metadata`, `timeout`, `defer_loading` | `pydantic_ai_slim/pydantic_ai/tools.py:544-748` |
| Planner→Executor handoff | `ModelRequestNode._prepare_request` → `_prepare_request_parameters` → `ModelRequestContext` → `wrap_model_request` → `model.request` → `CallToolsNode` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1451-1624`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:768-843` |
| Planner→Executor dispatch | `CallToolsNode._handle_tool_calls` → `process_tool_calls(tool_manager, tool_calls, final_result, ctx, output_parts …)` with strategies `_Early/_Graceful/_ExhaustiveProcessor` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2075-2156`, `pydantic_ai_slim/pydantic_ai/_tool_execution.py:254-322` |
| Contract validation: schema | `ToolManager._validate_tool_args` uses `tool.args_validator.validate_json/validate_python(…, context=ctx.validation_context)` Pydantic schema validator + `args_validator_func` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:307-349` |
| Contract validation: unknown tool | `_resolve_tool` raises `ModelRetry(f"Unknown tool name: {name!r}. Available tools: …")` — unknown planner tool becomes retry prompt, not crash | `pydantic_ai_slim/pydantic_ai/tool_manager.py:497-518` |
| Contract validation: deferred tool | `_unavailable_reason` + `_ToolUnavailable(ModelRetry)` + `is_tool_available` gate; first refusal free via `availability_refused` set | `pydantic_ai_slim/pydantic_ai/tool_manager.py:98-112`, `pydantic_ai_slim/pydantic_ai/tool_manager.py:520-551`, `pydantic_ai_slim/pydantic_ai/tool_manager.py:588-609` |
| Contract validation: output schema | `OutputSchema.build` dispatch (`TextOutputSchema`/`ToolOutputSchema`/`NativeOutputSchema`/`PromptedOutputSchema`/`AutoOutputSchema`), `BaseOutputProcessor.validate/hook_validate`, `UnionOutputProcessor` | `pydantic_ai_slim/pydantic_ai/_output.py:432-682` |
| Executor can reject | `validate_tool_call` returns `args_valid=False` + `validation_error: ToolRetryError|ToolFailedError`, `_check_max_retries` → `UnexpectedModelBehavior` at `retries[name] >= max_retries` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:257-266`, `pydantic_ai_slim/pydantic_ai/tool_manager.py:570-608` |
| Executor can modify | `ToolPrepareFunc[DepsT]` per-tool `prepare: (ctx,ToolDefinition)→ToolDefinition|None` filtering/rewriting; capability `prepare_tools/prepare_output_tools` on every `ToolManager.for_run_step` | `pydantic_ai_slim/pydantic_ai/tools.py:104-132`, `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:438-476` |
| Executor can modify via hooks | `wrap_tool_validate`/`before_tool_validate`/`after_tool_validate`/`wrap_tool_execute`/`before_tool_execute`/`after_tool_execute`; `SkipToolValidation(args)`, `SkipToolExecution(result)`, `before_/after_` can raise `ModelRetry` to redirect | `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:748-936`, `pydantic_ai_slim/pydantic_ai/exceptions.py:203-228` |
| Feedback: retry prompt | `ToolRetryError(RetryPromptPart)` raised for validation/execution failures, appended as `RetryPromptPart` to next `ModelRequest.parts`, `consume_output_retry` budgets, `retry_wins` invariant suppresses winning output | `pydantic_ai_slim/pydantic_ai/exceptions.py:616-649`, `pydantic_ai_slim/pydantic_ai/_tool_execution.py:884-915`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:362-379` |
| Feedback: terminal failure | `ToolFailed` → `ToolFailedError(ToolReturnPart(outcome='failed'))` surfaced to model without consuming retry budget; distinct from `ModelRetry` | `pydantic_ai_slim/pydantic_ai/exceptions.py:100-147`, `pydantic_ai_slim/pydantic_ai/tool_manager.py:274-283` |
| Feedback: deferral | `CallDeferred|ApprovalRequired` → `DeferredToolRequests` end-of-run result, resumable via `DeferredToolResults`, `handle_deferred_tool_calls` capability hook | `pydantic_ai_slim/pydantic_ai/tools.py:15-22`, `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:1098-1124`, `pydantic_ai_slim/pydantic_ai/_tool_execution.py:964-1010` |
| Feedback: model-request retry | `ModelRetry` from `after_model_request`/`wrap_model_request`/`on_model_request_error` appends response to history and builds `_build_retry_node` (`ModelRequest(parts=[RetryPromptPart])`), counts toward `output` budget | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1439-1445`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1797-1810` |
| Contract typed | Strict Pyright mode (`tool.pyright:typeCheckingMode= strict`), `ToolManager[DepsT]` generic, `OutputSchema[OutputDataT]` generic, `Agent[DepsT,OutputDataT]` generics, `ModelResponse`/`ToolCallPart` dataclasses with `ModelMessagesTypeAdapter: TypeAdapter[list[ModelMessage]]` | `pyproject.toml:332-336`, `pydantic_ai_slim/pydantic_ai/tool_manager.py:56`, `pydantic_ai_slim/pydantic_ai/_output.py:432`, `pydantic_ai_slim/pydantic_ai/messages.py:2769` |
| Cross-agent planning | No `PlannerAgent`/`ExecutorAgent` types; cross-agent is library composition: `Agent.tool` wrapping another `Agent.run`, `pydantic_graph` `Graph` with `UserPromptNode→ModelRequestNode→CallToolsNode` nodes, `select_model` per-step model selector, `FallbackModel` retry across models | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:394-412`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:443-489`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:3313-3336` |
| History repair / plan hygiene | `_clean_message_history` = `_drop_orphaned_tool_results` → `_repair_dangling_tool_calls` (synthesized `ToolReturnPart(state='interrupted')`) → `_merge_consecutive_messages`; `sanitize_messages` strips client-injected system prompts / dangling `ToolCallPart`s | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2964-2990`, `pydantic_ai_slim/pydantic_ai/messages.py:2954-3010` |
| Tests: validation & retry | Tests covering unknown tool retry, validation failure → `RetryPromptPart`, `ToolFailed`, `availability_refused`, output validation hooks, `end_strategy`, deferred tools, tool search gating | `tests/test_tools.py:1+`, `tests/test_tool_availability.py:1+`, `tests/test_capabilities.py`, `tests/models/test_model_request_parameters.py` |

## Answers to Dimension Questions

**1. What does the planner output?**

The planner is the LLM. Its output is a `ModelResponse` (`pydantic_ai_slim/pydantic_ai/messages.py:2540`) whose `parts: Sequence[ModelResponsePart]` is a discriminated union of `TextPart`, `ThinkingPart`, `ToolCallPart` (`pydantic_ai_slim/pydantic_ai/messages.py:2277` — `tool_name`, `args: str|dict[str,Any]`, `tool_call_id`, `tool_kind`), `NativeToolCallPart`/`NativeToolReturnPart`, `ToolSearchCallPart`, `LoadCapabilityCallPart`, `FilePart`, `SpeechPart`, `CompactionPart`. The request side that constrains planning is `ModelRequestParameters` (`pydantic_ai_slim/pydantic_ai/models/__init__.py:174`) carrying `function_tools: list[ToolDefinition]`, `output_tools`, `output_mode` (`tool`/`native`/`prompted`/`auto`/`text`/`image`), `tool_visibility`, `revealed_tool_names`, and `instruction_parts`. Non-tool outputs flow through `OutputSchema`/`BaseOutputProcessor` (`pydantic_ai_slim/pydantic_ai/_output.py:432`) which may produce tool-call style output (`ToolOutputSchema`) or plain text/image. There is no separate `Plan` DTO — the plan is a `ModelResponse`.

**2. Can the executor trust it?**

No; the executor treats every `ModelResponse` as untrusted and validates before executing. `ToolManager.validate_tool_call` (`pydantic_ai_slim/pydantic_ai/tool_manager.py:610`) resolves the name (`_resolve_tool` → `ModelRetry` for unknown, `_ToolUnavailable` for gated tools), validates JSON/Python args against the Pydantic `SchemaValidator` (`pydantic_ai_slim/pydantic_ai/tool_manager.py:307`), runs `args_validator_func`, and fires `before_tool_validate`/`wrap_tool_validate`/`after_tool_validate` + `on_tool_validate_error`. Output tools go through the disjoint path `validate_output_tool_call` with `run_output_validate_hooks` (`pydantic_ai_slim/pydantic_ai/tool_manager.py:739`, `pydantic_ai_slim/pydantic_ai/_output.py:126`). Unknown or args-invalid calls are not executed; they become `RetryPromptPart`s. Budget exhaustion (`_check_max_retries` in `pydantic_ai_slim/pydantic_ai/tool_manager.py:257`) raises `UnexpectedModelBehavior`. So execution cannot proceed on a bad plan without the executor first vetoing or converting it to a model-visible error.

**3. Can the executor modify it?**

Yes, at multiple interception layers:

- **Before validation:** per-tool `Tool.prepare(ctx,tool_def) → ToolDefinition|None` (`pydantic_ai_slim/pydantic_ai/tools.py:104`, `pydantic_ai_slim/pydantic_ai/tools.py:486` `prepare_tool_def`) can drop, rename, or rewrite the advertised schema; capability `prepare_tools(ctx,tool_defs: list[ToolDefinition])` (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:438`) can filter the whole visible toolset per step via `PreparedToolset`/`WrapperToolset`.
- **Validation:** `before_tool_validate(args: RawToolArgs) → RawToolArgs`, `wrap_tool_validate(args,handler) → ValidatedArgs`, `after_tool_validate(args: ValidatedArgs) → ValidatedArgs` (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:748-813`) can mutate `args`; `SkipToolValidation(validated_args)` and `on_tool_validate_error` can synthesize a valid result.
- **Execution:** `before_tool_execute(args)→args`, `wrap_tool_execute(args,handler)→result`, `after_tool_execute(result)→result`, `on_tool_execute_error` (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:846-936`) can replace args, short-circuit with `SkipToolExecution(result)`, or map errors to retries.
- **Dispatch time:** `ToolManager.handle_call`/`validate+execute` split lets `CallToolsNode` validate all calls first (emitting accurate `FunctionToolCallEvent(args_valid=…)`) then execute via `process_tool_calls` (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:254`) with `end_strategy` (`early`/`graceful`/`exhaustive` in `pydantic_ai_slim/pydantic_ai/_agent_graph.py:100`) governing ordering/barriers and skipping. Native fallback `unless_native/with_native` in `_resolve_request_tools` (`pydantic_ai_slim/pydantic_ai/models/__init__.py:831`) can also drop local tools when native is supported.

**4. How is failure fed back?**

Four distinct channels, all queued for the next planner turn:

- **Retry:** `ModelRetry(message)` from tools/validators/hooks → `ToolRetryError(RetryPromptPart(content=message, tool_name, tool_call_id))` appended to next `ModelRequest.parts` (`pydantic_ai_slim/pydantic_ai/exceptions.py:57`, `pydantic_ai_slim/pydantic_ai/tool_manager.py:269`). Validation failures → `_make_validation_failure` marks `failed_tools` and `retries[tool]++` (`pydantic_ai_slim/pydantic_ai/tool_manager.py:570`); availability refusal `_ToolUnavailable` is free once (`availability_refused` set, `pydantic_ai_slim/pydantic_ai/tool_manager.py:595`) then charged normally. When any real function tool retries in the same round as a valid output, `retry_wins` suppresses the output (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:881-909`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:362`) so the model fixes the retry first.
- **Terminal failure:** `ToolFailed(message)` → `ToolFailedError(ToolReturnPart(outcome='failed'))` visible to model but budget-neutral (`pydantic_ai_slim/pydantic_ai/exceptions.py:100`).
- **Model-request feedback:** `ModelRetry` thrown from `after_model_request`/`wrap_model_request` is caught in `ModelRequestNode._finish_handling`/`_make_request` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1739`), the offending `ModelResponse` is appended for context, and a fresh `ModelRequestNode(ModelRequest(parts=[RetryPromptPart]))` is enqueued; it consumes `GraphAgentState.consume_output_retry` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:362`).
- **Deferral / human-in-loop:** `CallDeferred`/`ApprovalRequired`/`Tool(kind='external'|'unapproved')` → `DeferredToolRequests` graph-terminating result, later resumed via `DeferredToolResults` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:660-706`, `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:1098`). History hygiene (`_repair_dangling_tool_calls` with synthesized `outcome='interrupted'` returns, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2820`) ensures even interrupted runs can be retried without leaking dangling calls.

**5. Is the contract typed?**

Yes — strong static and runtime typing end-to-end. `ModelResponse`/`ToolCallPart`/`ToolDefinition`/`ModelRequestParameters`/`ValidatedToolCall`/`OutputSchema[OutputDataT]`/`Agent[DepsT,OutputDataT]` are all Pydantic dataclass / Python `dataclass` generics with explicit type parameters (`pydantic_ai_slim/pydantic_ai/tool_manager.py:56`, `pydantic_ai_slim/pydantic_ai/_output.py:432`). Serialization is governed by `ModelMessagesTypeAdapter: TypeAdapter[list[ModelMessage]]` (`pydantic_ai_slim/pydantic_ai/messages.py:2769`) with discriminators on `part_kind`+`tool_kind` (`pydantic_ai_slim/pydantic_ai/messages.py:2444`, `pydantic_ai_slim/pydantic_ai/messages.py:2494`). Tool parameters are a validated `ObjectJsonSchema` derived via `FunctionSchema`/`GenerateToolJsonSchema` with `SchemaValidator` (`pydantic_ai_slim/pydantic_ai/tools.py:266`, `pydantic_ai_slim/pydantic_ai/_function_schema.py`). Output schemas are `TypeAdapter`-backed validators with strict-mode, `validate_python`/`validate_json` and union discriminators (`pydantic_ai_slim/pydantic_ai/_output.py:1089`). The repo enforces `pyright typeCheckingMode=strict` (`pyproject.toml:334`) with coverage requiring 100% branch coverage on these paths.

## Architectural Decisions

- **LLM-as-planner, graph-as-executor:** Planning is not a Python function that returns a `Plan` DTO; it is the LLM generating a `ModelResponse`. Execution is a distinct `CallToolsNode` separate from `ModelRequestNode`. The `Graph` (`UserPromptNode → ModelRequestNode → CallToolsNode → (ModelRequestNode|End)`) built by `build_agent_graph` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2574`) makes the loop explicit while keeping the planner black-box. This favors flexibility over inspectability — good for free-form agents, weaker for auditable multi-step plans.

- **Two-phase validate→execute with events:** `validate_tool_call` then `execute_tool_call` (or the convenience `handle_call`) lets `CallToolsNode` emit accurate `FunctionToolCallEvent(args_valid=…)` / `OutputToolCallEvent` before execution and choose the right failure channel. Decisions: output tools use a disjoint hook family (`run_output_validate/process_hooks`) so user tool hooks cannot accidentally intercept structured output.

- **Tool vs. capability gating:** Deferred tools (`defer_loading=True` on `ToolDefinition`, `pydantic_ai_slim/pydantic_ai/tools.py:618`) plus `ModelRequestParameters.tool_visibility`/`revealed_tool_names` (`pydantic_ai_slim/pydantic_ai/models/__init__.py:174`) let the planner discover tools incrementally (tool search, `load_capability`). `is_tool_available` (`pydantic_ai_slim/pydantic_ai/tool_manager.py:520`) + `parse_discovered_tools`/`parse_loaded_capabilities` history scan (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2415`) enforces that the model has seen a tool's schema before it can be called. This is an availability refusal (`_ToolUnavailable`) — free once — not an "unknown tool" error; separates "haven't discovered yet" from "never existed."

- **Retrieval-coupled hooks + prepared toolsets:** `prepare_tools` is attached to `PreparedToolset`/`WrapperToolset` and recomputed on `ToolManager.for_run_step` per `run_step`. This makes per-step tool filtering atomic with the request and OTel-visible. Alternative (ad-hoc filtering inside `CallToolsNode`) would have split advertised vs. executable sets.

- **Durability-aware design:** `ModelRequestNode.model_request`/`model_request_stream` continuation helpers (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:911,1044`) and `_with_outgoing_reveal_state` ensure the same validated `ModelRequestParameters` cross activity/step/task boundaries for Temporal/DBOS/Prefect, so replay re-validates deterministically.

## Notable Patterns

- **Discriminated-union message parts with typed promotion:** `ModelResponsePart`/`ModelRequestPart` use callable discriminators on `(part_kind, tool_kind)` (`pydantic_ai_slim/pydantic_ai/messages.py:2444`) plus `_TOOL_CALL_NARROWERS` registries to promote `ToolCallPart` → `ToolSearchCallPart` etc. when `tool_kind` is set, avoiding `tool_name` collision while keeping `ModelMessagesTypeAdapter` round-trip safe.

- **Availability-refusal budget isolation:** `availability_refused: set[str]` (`pydantic_ai_slim/pydantic_ai/tool_manager.py:160`) is disjoint from `retries` so the first "call before reveal" guidance never steals the budget the tool will need on its legitimate retry. Decision is run-scoped (not per-step) so it survives the mandatory next-turn correction.

- **Retry-wins invariant:** Under `end_strategy='graceful'|'exhaustive'`, a function-tool `RetryPromptPart` vetoes any co-emitted winning output (`_is_retry_wins_trigger`, `pydantic_ai_slim/pydantic_ai/_tool_execution.py:881`), forcing the model to address the error. Exempt for `early` (no function tools run with a winning output) and externally-committed streamed outputs (`final_result_was_set_externally`).

- **Hooks taxonomy mirroring Pydantic lifecycle:** Pairs `before_/after_/wrap_/on_error_` at run, node, model-request, tool-validate, tool-execute, output-validate, and output-process boundaries (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:478-1137`), with `Hooks[AgenDepsT]` decorator sugar (`pydantic_ai_slim/pydantic_ai/capabilities/hooks.py:718`). Tool hooks take `ToolDefinition`+`call` so a wrapper can scope by `tool_name` or metadata.

- **Pipeline history hygiene as pure transforms:** `_clean_message_history` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2964`) chains `drop_orphaned → repair_dangling → merge_consecutive` as `list[ModelMessage]→list[ModelMessage]` pure functions, with `sanitize_messages` as the public client-trust boundary.

## Tradeoffs

- **Opaque planner vs. auditable plan:** Because planning is delegated to the LLM and represented only as `ModelResponse.parts`, there is no machine-checkable `Plan` (steps, dependencies, preconditions). Audit trails rely on message history and OTel (`capture_run_messages`, `_otel_messages`). Teams needing declarative plans must impose their own `OutputSchema` (e.g. Pydantic model describing steps) and treat plan execution as downstream — workable but not framework-prescribed.

- **Maximal executor mutability vs. predictability:** Per-tool `prepare` + capability `prepare_tools` + six tool-hook points + `SkipTool*` control-flow exceptions make the executor freely rewritable, but the interaction order (before → wrap(handler) → after → on_error, with output vs. tool hook split) is subtle; misordering can silently bypass a validator that was assumed outermost. The `CombinedCapability` topological sort mitigates but cannot eliminate composition complexity.

- **Eager validation strictness vs. LLM friendliness:** Pydantic validators reject unknown args/default missing faithfully, incrementing retry counters quickly. The `availability_refused` one-free-refusal and `_ToolUnavailable` guidance plus `ValidationError → RetryPromptPart` with formatted `ErrorDetails` help, but a chatty validator can still burn the default `max_retries=1` / `max_output_retries=1` (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:658`) on the first turn, ending runs in `UnexpectedModelBehavior`. Operators must tune `Agent(retries={tools:…, output:…})`.

- **Parallel execution modes vs. simplicity:** `ParallelExecutionMode` (`parallel` / `sequential` / `parallel_ordered_events`) plus per-tool `sequential` barrier (`ToolDefinition.sequential`) and `end_strategy` variants give fine-grained concurrency but the precedence (`end_strategy` dictates tool ordering, `is_sequential` carves segments in `_tool_execution.py:232`) is not surfacing at the `ToolDefinition` site; mis-setting `sequential=True` on one tool constrains the whole batch in ways not visible without reading `process_tool_calls`.

- **Deferred-tool gating vs. latency:** Search-gated/capability-gated tools shrink prompt size and prevent hallucination, but each reveal requires a model turn (`tool_search`/`load_capability`) and a new `ModelRequestParameters` resolution (`_with_outgoing_reveal_state` → `prepare_request`). Naive use adds round-trips; the framework offers no automatic reveal planner.

## Failure Modes / Edge Cases

- **Hallucinated / unknown tool:** `_resolve_tool` in `pydantic_ai_slim/pydantic_ai/tool_manager.py:502` raises `ModelRetry("Unknown tool name: … Available tools: …")`; caught in `validate_tool_call` → `ValidatedToolCall(args_valid=False, validation_error=ToolRetryError)` → `RetryPromptPart` on next request. If hallucination repeats, `_check_max_retries` (`pydantic_ai_slim/pydantic_ai/tool_manager.py:257`) raises `UnexpectedModelBehavior("exceeded max retries for '…'" )` ending the run. No automatic self-healing beyond retry budget.

- **Tool called before reveal:** `_unavailable_reason` (`pydantic_ai_slim/pydantic_ai/tool_manager.py:520`) emits `_ToolUnavailable` retry prompt naming the fix (`search for it`, or `call load_capability for '…'`). First occurrence per tool is budget-free (`availability_refused`), second counts. A plan that ignores the fix repeatedly will still exhaust retries.

- **Schema-valid JSON string vs. object:** `ToolCallPart.args` may be `str` (raw `validate_json`) or `dict` (`validate_python`) (`pydantic_ai_slim/pydantic_ai/tool_manager.py:329`). Malformed JSON (e.g. cut off by `finish_reason='length'` / token limit) triggers `ValidationError → RetryPromptPart` or `IncompleteToolCall` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:346`). No plan repair synthesizes corrected JSON; the model is asked to retry with the schema hint.

- **Output validator rejects structured output:** `run_output_validate_hooks` / `run_output_process_hooks` (`pydantic_ai_slim/pydantic_ai/_output.py:126`) wrap `ValidationError`/`ModelRetry` as `ToolRetryError` (tool path) or `RetryPromptPart` (text path) and count against `GraphAgentState.output_retries_used` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:362`). `UnexpectedModelBehavior("Exceeded maximum output retries (N)")` terminates; validator returning a default value is not supported without `on_output_validate_error`.

- **Co-emitted output + retry:** Under `graceful`/`exhaustive`, a single `ModelResponse` containing both a valid output-tool result and a function-tool retry triggers `retry_wins`: winning output's `ToolReturnPart("Final result processed.")` is replaced with `"Output not used …"` (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:892`) and `final_result=None` so the retry surfaces. A plan that repeatedly pairs correct output with a failing tool can loop until `max_output_retries` or `UsageLimits` fire.

- **Deferred tools never resolved:** `Tool.kind='external'|'unapproved'` → `DeferredToolRequests` terminating `End` (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:966`). If the caller never supplies `DeferredToolResults` and no `handle_deferred_tool_calls` handler resolves it, the run ends successfully but without a user-visible output; callers must not mistake this for finality. `ApprovalRequired` without a handler surfaces the same way.

- **Client-injected plan forgery:** `sanitize_messages` (`pydantic_ai_slim/pydantic_ai/messages.py:2954`) strips client-supplied `ToolCallPart`s whose `tool_call_id` is not in `resolved_tool_call_ids`, plus system prompts, non-allowlisted `FileUrl` schemes, and dangling tail tool calls; undrained interleavings of user-provided `message_history` with a prior run's dangling calls are repaired before the model sees them. Bypass requires trusted `message_history` on the server path.

- **Token-limit truncation while emitting a tool call:** `GraphAgentState.check_incomplete_tool_call` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:346`) detects `finish_reason='length'` with trailing `ToolCallPart.args_as_dict(raise_if_invalid=True)` failure and re-raises `IncompleteToolCall` with advice to raise `max_tokens`; `CallToolsNode._run_stream` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1929`) handles `length` with no actionable output the same way.

## Future Considerations

- Add a first-party **declarative `Plan` schema** option (`OutputSchema` capturing `list[Step{tool_name,args,depends_on}]`) plus a built-in `PlanExecutor` capability that validates dependencies, orders steps, and simulates dry-run before side effects. This would make cross-agent planning (planner agent → executor agent) framework-native rather than application-pattern, and provide static plan linting not dependent on model cooperation.

- Reify `ModelResponse`→`ToolManager` contract as an exported protocol (`PlannerOutput = ModelResponse`, `ExecutorInput = list[ToolCallPart]`, `ExecutorResult = ModelRequest`) with versioned `PlanValidationResult` so downstream tools (UI adapters, durable replay, evaluators) can type-check without importing internals.

- Provide **budget-aware progressive guidance** for availability refusals: today the prompt is static (`search for it`, `load capability`). A capability that tracks repeated refusals per tool and escalates (e.g. inline schema after second failure, or auto-revealing) would reduce hallucination loops without user code.

- Unify `max_retries` / `max_output_retries` per-tool vs. per-run semantics — the executor's output budget (`_output.run_output_validate_hooks`) vs. `ToolOutput(max_retries=N)` gap could surface as a distinct `OutputRetryBudgetExceeded` rather than the shared `UnexpectedModelBehavior`, improving observability.

## Questions / Gaps

- No explicit `Plan` / `Step` / `ExecutionResult` interface: planner output is LLM-prose-governed; whether a particular agent instance respects tool schemas or produces parseable JSON is evaluated at runtime via `validate_tool_call` and not via a pre-execution plan check. Static analysis of plan safety (e.g. "does this plan require a capability not yet loaded?") is possible but not exposed as a `plan.validate(history)→Report` API.

- Cross-agent planning (delegating planner to a second agent) is demonstrated only via examples/docs (`agent.tool` wrapping another `Agent`, or building a `pydantic_graph` with explicit nodes) — no in-tree test exercising planner→executor handoff across two `Agent` instances was found; 100% coverage on `Agent` and `ToolManager` is intra-run.

- No load-bearing gap blocks a production use of the planner/executor contract: the typed contract, validation, retry budgets, hooks, and history hygiene are implemented and tested. The gap is granularity of guarantees for *application-level* multi-step plans beyond a single `ModelResponse`.

---

Generated by `Dimension 06.04: Planner/Executor Contract` against `pydantic-ai`.

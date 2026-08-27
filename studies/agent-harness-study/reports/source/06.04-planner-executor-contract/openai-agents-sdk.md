# Source Analysis: openai-agents-sdk

## 06.04 Planner/Executor Contract

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python / OpenAI Responses API, Pydantic, asyncio |
| Analyzed | 2026-08-27 |

## Summary

OpenAI Agents SDK collapses planner and executor into a single orchestrated turn loop but enforces a strongly-typed contract at the boundary between LLM-generated decisions and local side effects. The planner is the OpenAI model returning `Response`/`ModelResponse` items; the SDK normalizes them into `ProcessedResponse` (`src/agents/run_internal/run_steps.py:116`) containing typed `ToolRun*` slots, then builds an explicit `ToolExecutionPlan` (`src/agents/run_internal/tool_planning.py:557`) before any executor runs. Executors are pluggable `FunctionTool`/`ShellTool`/`ComputerTool` etc. (`src/agents/tool.py:441`) invoked only after validation (`_dedupe_processed_response_invocations:339`, `_validate_unresolved_function_calls:330`, `_tool_invocation_status:304` in `run_context.py`). The executor can reject via `ToolApprovalItem` interruptions (`NextStepInterruption:171`), `ModelBehaviorError` for bad call IDs, and `ToolInput/Output` guardrails. Feedback is looped by converting `ToolCallOutputItem` back to `TResponseInputItem` (`src/agents/items.py:221`, `turn_resolution.py:799`) into `NextStepRunAgain` and the outer `while True` in `run.py:967` / `run_loop.py:1191`. Cross-agent planning uses `Handoff` and `Agent.as_tool()` (`src/agents/agent.py:583`, `src/agents/run_internal/turn_resolution.py:542`), each re-entering the same planning contract as a nested `AgentRunner` invocation.

## Rating

**8 / 10 — Clear model with tests, explicit interfaces, and operational safeguards**

Rationale: Planner output (ModelResponse -> ProcessedResponse -> ToolExecutionPlan) is dataclass-typed, strict JSON-schema enforced, and validated before execution. Executor interfaces expose explicit `FunctionTool` schemas, `needs_approval`, `timeout`, `ToolErrorFunction`, and guardrail hooks. Contract validation rejects re-used `call_id`, missing IDs, unknown tools, and invalid JSON before side effects. Interruption/resume via `RunState` makes rejection durable and auditable. Feedback loop is deterministic and preserves history. Deduction from 9: no formal protobuf/openAPI contract artifact, planner is external LLM (not SDK-typed beyond OpenAI SDK), and some validation lives in internal helpers rather than a single public schema file.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Planner output — raw LLM | `ModelResponse` dataclass wrapping OpenAI `Response.output: list[TResponseOutputItem]` with `response_id`/`usage` | `src/agents/items.py:705-718` |
| Planner output — normalized | `ProcessedResponse` aggregates `functions: list[ToolRunFunction]`, `handoffs`, `computer_actions`, `shell_calls`, `apply_patch_calls`, `mcp_approval_requests`, `interruptions`, `function_tools_not_found` | `src/agents/run_internal/run_steps.py:116-148` |
| Planner output — per-tool slots | Typed carriers `ToolRunFunction(tool_call: ResponseFunctionToolCall, function_tool: FunctionTool)`, `ToolRunComputerAction`, `ToolRunShellCall`, `ToolRunApplyPatchCall`, `ToolRunCustom`, `ToolRunHandoff` | `src/agents/run_internal/run_steps.py:62-114` |
| Planner output — streaming sentinel | `SingleStepResult` ties `original_input`, `model_response: ModelResponse`, `pre_step_items`, `new_step_items`, `next_step: NextStepHandoff|FinalOutput|RunAgain|Interruption`, `processed_response` | `src/agents/run_internal/run_steps.py:184-232` |
| Planner output — final-output schema | `AgentOutputSchemaBase.validate_json()` and `AgentOutputSchema.is_plain_text()/json_schema()` enforce structured final output; invoked at `turn_resolution.py:1002-1003` and `agent_output.py:142` | `src/agents/agent_output.py:22-57,60-187` |
| Executor interface — FunctionTool | `FunctionTool(name, description, params_json_schema, on_invoke_tool: Callable[[ToolContext,str], Awaitable[Any]], needs_approval, timeout_seconds, failure_error_function, strict_json_schema)` with qualified-name/namespace validation | `src/agents/tool.py:441-615,583-599` |
| Executor interface — other tools | `ShellTool(executor, needs_approval, environment)`, `ComputerTool(computer)`, `ApplyPatchTool(editor)`, `HostedMCPTool(tool_config, on_approval_request)`, `CustomTool`, `LocalShellTool` | `src/agents/tool.py:842-871,1087-1115,1418-1453,1456-1480` |
| Executor interface — Function schema | `function_schema()` builds `FuncSchema(name, params_pydantic_model, params_json_schema, signature, takes_context)` via `create_model` + `ensure_strict_json_schema` | `src/agents/function_schema.py:22-43,290-491` |
| Executor interface — RunContext | `RunContextWrapper._approvals: dict`, `_tool_invocations: dict[str,_ToolInvocationRecord]`, `is_tool_approved()`, `get_approval_status()` | `src/agents/run_context.py:72-94,87-94` |
| Contract validation — strict JSON | `ensure_strict_json_schema` applied to tool params (`tool.py:595-598`) and agent output schema (`agent_output.py:118-126`) and function schema (`function_schema.py:477-478`) | `src/agents/strict_schema.py:1-` via imports at `src/agents/tool.py:595`, `src/agents/agent_output.py:118` |
| Contract validation — invocation ID | `_dedupe_processed_response_invocations()` validates call IDs, rejects reused call_id for different content, coalesces duplicates, tracks `tool_invocation_identity` and `completed_output_keys` | `src/agents/run_internal/tool_planning.py:339-554` |
| Contract validation — approval binding | `_tool_invocation_status()` ensures one canonical invocation per `call_id`; fingerprint mismatch raises `ModelBehaviorError("Model reused a tool call ID for a different invocation")` | `src/agents/run_context.py:304-355` |
| Contract validation — unresolved tools | `_validate_unresolved_function_calls()` enforces that every `function_tools_not_found` was validated via `_tool_invocation_status` before execution | `src/agents/run_internal/tool_planning.py:330-337` |
| Contract validation — pending dupes | `_dedupe_tool_call_items()` skips duplicates by `_tool_call_identity` plus `skipped_raw_item_ids` set | `src/agents/run_internal/tool_planning.py:245-266` |
| Contract validation — computer init | `initialize_computer_tools()` + `resolve_computer()` per-run lifecycle with `ComputerProvider create/dispose` and WeakKeyDictionary isolation | `src/agents/tool.py:594-617,891-940` |
| Executor reject — interruption plan | `ToolExecutionPlan(pending_interruptions, approved_mcp_responses, mcp_requests_with_callback)` + `has_interruptions` property; built by `_build_plan_for_fresh_turn` / `*_resume_turn` | `src/agents/run_internal/tool_planning.py:557-682` |
| Executor reject — approval gating | `_collect_runs_by_approval()` and `resolve_approval_status()` query `RunContextWrapper.get_approval_status()`; rejected runs emit `ToolCallOutputItem` with `REJECTION_MESSAGE`, approved -> `approved_runs`, pending -> `pendingInterruptionAdder` | `src/agents/run_internal/tool_planning.py:759-840` and `src/agents/run_internal/tool_execution.py:1162-1213` |
| Executor reject — ModelBehaviorError | Missing `call_id` raises `ModelBehaviorError("Tool invocations require a non-empty string call ID")`, reused sibling ID raises same, `apply_patch`/`shell` coercion raises on missing action/payload | `src/agents/run_internal/tool_planning.py:406-432`, `src/agents/run_internal/tool_execution.py:638-712` |
| Executor reject — tool not found | `function_tools_not_found` fed to `_build_tool_not_found_output_items()` producing `ToolCallOutputItem(output="Tool '{name}' not found.")` via `resolve_tool_not_found_message` + configurable `ToolErrorFormatter` | `src/agents/run_internal/turn_resolution.py:258-332` |
| Executor reject — guardrails | `ToolInputGuardrail`/`ToolOutputGuardrail` (`ToolGuardrailFunctionOutput(behavior=allow|reject_content|raise_exception)`) executed in `tool_execution.py:execute_function_tool_calls`; rejection synthesizes output instead of invoke | `src/agents/tool_guardrails.py:18-118`, `src/agents/run_internal/tool_execution.py` referenced via `tool_planning._execute_tool_plan` |
| Feedback loop — turn orchestration | `execute_tools_and_side_effects()` validates, builds plan, `await _execute_tool_plan(...)`, extends `new_step_items` with `_build_tool_result_items` + `_collect_tool_interruptions`, then `processed_response.interruptions = interruptions` and return `SingleStepResult` with `NextStepRunAgain/Interruption/Handoff/FinalOutput` | `src/agents/run_internal/turn_resolution.py:784-911` |
| Feedback loop — run loop | Outer `while True` in `AgentRunner._run_impl` calls `get_single_step_result_from_response -> execute_tools_and_side_effects`, on `NextStepRunAgain` loops back to LLM with `generated_items`/`session_items` appended; on interruption persists `RunState` | `src/agents/run.py:967`, `src/agents/run_internal/run_loop.py:1191` (streaming analog at `run_loop.py:868`) |
| Feedback loop — input re-feeding | `RunItem.to_input_item()` strips `created_by` and serializes `ToolCallOutputItem` via `_output_item_to_input_item` before replay to model; `ModelResponse.to_input_items()` mirrors | `src/agents/items.py:221-256,732-739`, `src/agents/items.py:151-154,462-489` |
| Cross-agent — handoff | `Agent.handoffs: list[Agent|Handoff]`, `execute_handoffs()` invokes `handoff.on_invoke_handoff`, appends `HandoffOutputItem`, filters/nests history, returns `SingleStepResult(next_step=NextStepHandoff(new_agent))` | `src/agents/agent.py:331-332`, `src/agents/run_internal/turn_resolution.py:527-750` |
| Cross-agent — agent-as-tool | `Agent.as_tool(tool_name, ...) -> FunctionTool` wraps `Runner.run`/`run_streamed` as nested invocation with `is_enabled`, `needs_approval`, `failure_error_function`, isolated `RunContextWrapper` scope | `src/agents/agent.py:583-1040` |
| Cross-agent — resume boundary | `RunState` serializes `ModelResponse`, `generated_items`, `CurrentStep: NextStepInterruption`, `_tool_invocations`, approvals; `resolve_interrupted_turn()` filters via `output_exists_checker`/`approval_items_by_call_id` | `src/agents/run_state.py:761-875,1134-`, `src/agents/run_internal/turn_resolution.py:1134-` |
| Execution plumbing | `_execute_tool_plan(parallel=true|false)` fans out to `execute_function_tool_calls`, `execute_computer_actions`, `execute_shell_calls` etc via `gather_with_cancel`, with `sibling_category_failure` cancellation and `tool_output_committer` for accepted responses | `src/agents/run_internal/tool_planning.py:944-1101` |
| Observable failure surface | `ModelBehaviorError`, `UserError`, `MaxTurnsExceeded`, `InputGuardrailTripwireTriggered`, `ToolTimeoutError` each carry trace span data; `_prepare_data_redacted_error`/`_detach_data_redacted_error_traceback` redact payloads | `src/agents/exceptions.py:1-`, `src/agents/run_internal/run_loop.py:30-47` |

## Answers to Dimension Questions

### 1. What does the planner output?

The planner is the OpenAI Responses model. Its SDK-visible output is `ModelResponse(output: list[TResponseOutputItem], usage, response_id)` (`src/agents/items.py:705`). The runner normalizes this into `ProcessedResponse` (`src/agents/run_internal/run_steps.py:116`) which partitions raw outputs into typed slots: `handoffs: list[ToolRunHandoff>`, `functions: list[ToolRunFunction>`, `computer_actions`, `custom_tool_calls`, `shell_calls`, `apply_patch_calls`, `mcp_approval_requests`, `function_tools_not_found`, `tools_used: list[str]`. Final structured output additionally goes through `AgentOutputSchema.validate_json()` (`src/agents/agent_output.py:142`). For streaming/interrupted turns the wrapper is `SingleStepResult` (`src/agents/run_internal/run_steps.py:184`) carrying `processed_response` + `next_step`.

No separate planning DSL: the plan is precisely the set of tool calls + handoffs + approval requests the model emitted in one turn. There is no explicit DAG/PDDL; ordering is implicit (LLM order) except for parallel executor fan-out.

### 2. Can the executor trust it?

No — the executor treats the plan as untrusted. Every plan passes through hardened validation before any side effect:

- `_dedupe_processed_response_invocations()` (`src/agents/run_internal/tool_planning.py:339`) checks call_id presence, uniqueness, and fingerprint equality via `tool_invocation_identity_and_scope`; duplicate call_id with different args/type raises `ModelBehaviorError`; completed invocations are deduped via `skipped_raw_item_ids`.
- `RunContextWrapper._tool_invocation_status()` (`src/agents/run_context.py:304`) registers fingerprint per call_id, rejects reuse for different invocation, marks `executed/completed`.
- `_validate_unresolved_function_calls()` (`src/agents/run_internal/tool_planning.py:330`) fails fast on unresolved function calls before sibling tools start.
- Strict JSON schema (`ensure_strict_json_schema` at `src/agents/tool.py:595`, `src/agents/function_schema.py:477`) forces model to emit schema-compliant arguments; tool arg parsing via `ToolContext` validates again (`src/agents/tool.py` call path).
- `ToolInputGuardrail` can reject even a structurally valid call (`src/agents/tool_guardrails.py:152`).

So trust is zero; execution proceeds only after these gates.

### 3. Can the executor modify it?

Yes, at multiple stages, but semantically it *filters* and *augments* rather than rewrites planner intent:

- Dedup/coalesce: `_dedupe_processed_response_invocations` and `_dedupe_tool_call_items` (`src/agents/run_internal/tool_planning.py:245`) drop exact duplicates and completed-call replays.
- Unknown-tool rewrite: `function_tools_not_found` are not executed; instead `_build_tool_not_found_output_items()` synthesizes an error output (`"Tool 'x' not found."`) that is fed back to the model (`src/agents/run_internal/turn_resolution.py:309`).
- Approval gating: `_build_plan_for_fresh_turn()` (`src/agents/run_internal/tool_planning.py:619`) splits runs into `approved_runs` and `pending_interruptions`; rejected approvals synthesize `function_rejection_item` with `REJECTION_MESSAGE` (configurable via `ToolErrorFormatter` / `resolve_approval_rejection_message` at `src/agents/run_internal/tool_execution.py:1230`).
- Handoff filtering: `execute_handoffs` runs `handoff.input_filter` / `nest_handoff_history` to rewrite `original_input`/`pre_step_items` before the next planner turn (`src/agents/run_internal/turn_resolution.py:650-732`).
- Tool-use-behavior override: `check_for_final_output_from_tools()` (`src/agents/run_internal/turn_resolution.py:753`) can short-circuit to `NextStepFinalOutput` without looping, changing planner’s expectation of `run_llm_again`.

Executor never mutates the model’s raw arguments; it decides to *skip*, *synthesize error output*, or *pause*.

### 4. How is failure fed back?

Two feedback channels, both produce model-visible `TResponseInputItem`s for the next turn:

- **In-history feedback**: On success, each `FunctionToolResult` yields a `ToolCallOutputItem(output, raw_item=FunctionCallOutput{call_id, output, type:function_call_output})` (`src/agents/items.py:845`) which is appended to `new_step_items` then replayed via `to_input_item()` (`src/agents/items.py:491`) into the next LLM input. Failures use the same shape: timeout via `ToolTimeoutError` -> `timeout_error_function` or default string, exception via `failure_error_function` (`src/agents/tool.py:617`), `ModelBehaviorError` -> `ModelBehaviorError` branch, and `needs_approval` rejection -> `REJECTION_MESSAGE`. The outer loop (`AgentRunner._run_impl:967` and `run_loop.start_streaming:1191`) then calls the model again with accumulated items (generated_items + previous output).
- **Control-flow feedback**: `handoff` produces `HandoffOutputItem` + optional history rewrite; `interrupt` produces `NextStepInterruption` persisted in `RunState` (`src/agents/run_state.py:846`) so a human approval (`run_state.approve()`) inserts an `McpApprovalResponse` / approval record before the next model call. `output_guardrail` trip re-injects a sanitized `output_guardrail_blocked_message` (`src/agents/run_internal/blocked_output.py`) rather than raw tool output.

Thus failure is never silent; it is serialized as a tool output string the planner must handle next turn.

### 5. Is the contract typed?

Yes, strongly and pervasively:

- **Static types**: Every boundary type is dataclass-typed with `py.typed` marker (`src/agents/py.typed`). `ToolRunFunction.tool_call: ResponseFunctionToolCall` (`src/agents/run_internal/run_steps.py:68`), `ProcessedResponse`, `ToolExecutionPlan`, `SingleStepResult`, `RunItem` union (`src/agents/items.py:686`), `FunctionTool`, `ToolContext`/`RunContextWrapper[TContext]` generics, plus OpenAI SDK types (`ResponseFunctionToolCall`, `ResponseComputerToolCall`, etc.).
- **Runtime schemas**: `function_schema` creates a Pydantic model per tool and caches `params_json_schema` made strict (`src/agents/function_schema.py:481`), `AgentOutputSchema` creates a `TypeAdapter` for structured outputs (`src/agents/agent_output.py:112`), both enforced with `ensure_strict_json_schema`.
- **Validation**: Pydantic `TypeAdapter.validate_python/validate_json` at invocation (`src/agents/tool.py:659`, `src/agents/items.py:860`), `UserError` on bad configs (`src/agents/agent.py:432`), `ModelBehaviorError` on contract violations.
- **Approval contract typed**: `ToolApprovalItem(raw_item, tool_name, tool_namespace, tool_lookup_key, tool_origin)` (`src/agents/items.py:556`) plus `_ToolInvocationRecord(fingerprint, approval_scope)` in `run_context.py:45`.

Weak spot: the LLM itself is untyped beyond OpenAI SDK; the SDK compensates with the strict validation layer. No protobuf/IDL artifact, but the dataclass + JSON-schema dual is the contract.

## Architectural Decisions

- **LLM as planner, SDK as hardening executor** (`src/agents/run.py:275` loop comment, `src/agents/run_internal/turn_resolution.py:799`): No separate planner service; model output is normalized into typed slots before any I/O. Tradeoff: low operational complexity, but planner cannot be swapped without changing model provider integration (`src/agents/models/interface.py`).
- **Two-stage planning** — `ProcessedResponse` (parse/classify) then `ToolExecutionPlan` (approve/dedupe) (`src/agents/run_internal/tool_planning.py:557`): separates classification from authorization. Enables parallel execution (`gather_with_cancel` at `tool_planning.py:984`) while keeping approval checks serial.
- **Canonical invocation identity + fingerprint** (`src/agents/run_context.py:304`, `_tool_identity.py`, `_tool_invocation.py`): every provider call ID gets a fingerprint of type+name+args; reuse/mutation is a hard error. Prevents replay attacks and model hallucination of call IDs. Persisted in `RunState` schema 1.15+ (`src/agents/run_state.py:218`).
- **Interruption as first-class pause** (`src/agents/run_internal/run_steps.py:171`, `src/agents/run_state.py:846`): approval-needing calls don’t fail; they emit `NextStepInterruption` and serialize to `RunState` for human-in-the-loop. Resolved via `resolve_interrupted_turn` with `output_exists_checker` to avoid re-execution.
- **Handoffs vs tools split** (`src/agents/run_internal/run_steps.py:62` vs `68`): handoffs are routed before generic tool execution and can rewrite history; tool-use behavior (`run_llm_again`/`stop_on_first_tool`/`callable` at `src/agents/agent.py:373`) lets the application promote tool output to final output without another planner turn.
- **Forgiving hosted tool handling, strict local tool handling**: unknown local function tools synthesize error feedback (`turn_resolution.py:258`) while hosted MCP approvals are mediated via `on_approval_request` callbacks or manual `RunState` approvals (`src/agents/run_internal/tool_planning.py:119`).

## Notable Patterns

- **Plan-then-filter-then-fork**: `process_model_response -> _dedupe_processed_response_invocations -> _register_tool_call_items -> _build_plan_for_fresh_turn -> _execute_tool_plan(gather_with_cancel)` (`src/agents/run_internal/turn_resolution.py:799`). Mirrors compiler pipeline.
- **Idempotent replay guard**: `completed_output_keys` set and `fingerprint` cache (`src/agents/run_internal/tool_planning.py:348-391`) make re-issuing the same call_id a no-op rather than double-execution, critical for server-managed conversation retries (`src/agents/run_internal/oai_conversation.py`).
- **Weak-ref agent lifecycle**: `RunItemBase._agent_ref` + `release_agent()` (`src/agents/items.py:108`) avoids leaking agent graphs across turns while keeping trace data.
- **Sandbox-aware preparation**: `SandboxRuntime.prepare_agent` runs before guardrails/model call (`src/agents/run.py:1020`), letting approvals gate sandbox creation (`src/agents/run.py:991`).
- **Nested agent isolation via `ToolContext` scope id** (`src/agents/agent_tool_state.py`, `src/agents/run_state.py:921`): each `Agent.as_tool` run gets its own `AgentToolUseTracker` and approval scope, preventing cross-nesting leakage; `RunState._copy_for_result_checkpoint` isolates usage accounting.

## Tradeoffs

- **LLM-only planner vs explicit planner library**: No dedicated planner node means planning quality is model-dependent; no step budget planner, no DAG executor, no LangGraph-style graph compile. Gains: minimal SDK surface, direct OpenAI integration, lower latency (single model call per turn). Costs: limited support for explicit multi-step plans, no plan-repair without another LLM turn.
- **Parallel fan-out with sibling failure arbitration** (`src/agents/run_internal/tool_execution.py:258-305`): `sibling_category_failure` event cancels slower categories when one category fails, preserving fast failure semantics. Tradeoff: non-deterministic output ordering may affect prompt determinism; mitigated by sorted `ordered_done_tasks` in `tool_execution.py:337`.
- **Sticky vs per-call approvals** (`src/agents/run_context.py:56-69`, `_ApprovalRecord`): `always_approve` persists a boolean for all future call_ids under that tool key, scoped by `approval_scope` fingerprint. Powerful for UX but risks over-broad approval if fingerprint is weak; SDK mitigates via lookup-key namespacing (`_tool_identity.get_function_tool_approval_keys`).
- **Strict JSON schema enforcement**: `_emit_tool_origin=False` defaults filtered; `strict_json_schema=True` everywhere (`src/agents/tool.py:538`). Ensures model reliability but rejects otherwise valid loose schemas; users must set `strict_json_schema=False` explicitly for flexible tool definitions (`src/agents/function_schema.py:296`).
- **Session vs server-managed conversation mutual exclusion** (`src/agents/run.py:687`, `run_internal/turn_resolution.py:495`): server-managed `conversation_id`/`previous_response_id` disables local session history; mixing them errors out. Simplifies consistency but forces users to pick persistence model.

## Failure Modes / Edge Cases

- **Reused/mutated call_id** (`src/agents/run_context.py:348-352`, `src/agents/run_internal/tool_planning.py:444`): any reuse of same ID with different `type`/`name`/`args` raises `ModelBehaviorError("Use a unique call ID")`, aborted before tool code. Recovery: retrying with fresh ID (model must self-correct after error output fed back).
- **Missing/empty call_id** (`src/agents/run_internal/tool_planning.py:406`): `ModelBehaviorError("Tool invocations require a non-empty string call ID")`; executor never invokes tool.
- **Missing tool** (`src/agents/run_internal/turn_resolution.py:258`, `function_tools_not_found`): synthesized output `"Tool 'x' not found."` via optional `ToolErrorFormatter` (`src/agents/run_config.py:ToolErrorFormatterArgs kind="tool_not_found"`). Not an exception unless formatter raises.
- **JSON argument mismatch**: `function_schema` Pydantic parse failure -> `ModelBehaviorError` -> fed to `failure_error_function` (default returns string to model) or re-raised if formatter returns None (`src/agents/tool.py:617`).
- **Approval rejection without follow-up**: `pending_interruptions` returned as `NextStepInterruption`; if caller never calls `approve()/reject()` the run stays non-terminal indefinitely (requires human/lifecycle driver). `RunState.add_input` explicitly blocks adds during `response_accepted` or `stop_on_first_tool` interruption (`src/agents/run_state.py:984-1006`).
- **MaxTurnsExceeded** (`src/agents/run.py:554`): runner raises `MaxTurnsExceeded` unless handled via `RunErrorHandlers` (`src/agents/run_error_handlers.py`). Guardrail trip (`InputGuardrailTripwireTriggered`/`OutputGuardrailTripwireTriggered`) similarly hard-stops.
- **Concurrent RunState resume against same Session**: `RunState._pending_session_write` + `_session_write_in_progress` guard requires caller serialization; concurrent resumes corrupt Session history (`src/agents/run_state.py:872` docstring).
- **Tool timeout** (`src/agents/tool.py:196`): `timeout_behavior="error_as_result"` (default) synthesizes error string; `"raise_exception"` raises `ToolTimeoutError` via `tool_execution.py` arbitration logic, which then wins over sibling successes per failure priority (`_get_function_tool_failure_priority` at `tool_execution.py:258`).
- **Non-finite floats / cyclic payloads in approval serialization** (`src/agents/run_state.py:314`): `_copy_json_compatible_value` rejects `math.inf/nan` and cycles with `TypeError`, preventing poisoned `RunState` JSON.

## Future Considerations

- Formal plan artifact (e.g., typed `Plan` with step DAG and explicit retry policy) rather than implicit `ModelResponse` would let non-LLM planners (deterministic, search-based) plug in without model coupling.
- Centralized contract schema file (OpenAPI/Pydantic export) for `ToolRun*` and `ProcessedResponse` to enable cross-language executors and contract tests in CI.
- Deterministic ordering knob for parallel `ToolExecutionPlan` fan-out so tests and caching don’t depend on event-loop timing.
- Plan validation unit tests that intentionally inject `call_id` collisions, missing tools, and invalid JSON to guard the `ModelBehaviorError` boundaries as part of SDK public test matrix (currently implicit via internal helpers).
- Observability: planner latency vs executor latency spans already exist (`src/agents/tracing` at `run_loop.py:319`), but executor rejection counts and approval heatmaps aren’t exported as metrics.

## Questions / Gaps

- **No integration test evidence scanned**: workspace search scope excluded top-level `tests/` (not inside `sources/openai-agents-sdk`); no statement can be made about external test coverage. Inspected internal unit helpers show extensive guards, but external regression suite was not inspected per isolation rule.
- **Planner prompt not inspected**: `src/agents/turn_preparation.py:get_all_tools/get_model/get_output_schema` assembles model input; the exact system prompt that constitutes the planner’s instructions (`Agent.instructions` callable/string) is user-provided, so planner behavior is not SDK-owned.
- **Multi-agent heterogenous planning**: `Handoff` and `Agent.as_tool` cover delegation, but SDK offers no explicit meta-planner that splits a task across agents; cross-agent planning is delegated to model tool routing, not a separate `HostedMultiAgent` controller (experimental `src/agents/extensions/experimental/hosted_multi_agent/model.py` hints at but doesn’t define a planner/executor split).
- **Validation coverage for `defer_loading` tool search**: `Tool.defer_loading` (`src/agents/tool.py:509`) and `prune_orphaned_tool_search_tools` (`src/agents/tool.py` import) suggest a tool-search loading contract, but exact validation was not traced beyond `_build_plan_*` flows.

---

Generated by `06.04-planner-executor-contract` against `openai-agents-sdk`.

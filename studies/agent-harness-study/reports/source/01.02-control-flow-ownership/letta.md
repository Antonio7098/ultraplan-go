# Source Analysis: letta

## 01.02 Control-Flow Ownership

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python 3.12, FastAPI, Pydantic, SQLAlchemy, OpenTelemetry, custom LLM adapters (Anthropic / OpenAI / Bedrock / Google / MiniMax / vLLM / SGLang) |
| Analyzed | 2026-07-02 |

## Summary

Letta owns the agent loop in a runtime-level `step()` / `stream()` driver (`letta/agents/letta_agent_v3.py:222`, `letta/agents/letta_agent_v3.py:443`), wrapped by an `AgentLoop.load()` factory (`letta/agents/agent_loop.py:19`). Control is split between three explicit owners:

1. **The Python runtime** picks the next "step" by incrementing `i` inside `for i in range(max_steps)` (`letta/agents/letta_agent_v3.py:328`, `letta/agents/letta_agent_v3.py:569`).
2. **The LLM** proposes the next action via tool-call arguments inside `invoke_llm()` (`letta/agents/letta_agent_v3.py:1155`).
3. **A `ToolRulesSolver` policy object** (`letta/helpers/tool_rule_solver.py:24`) and a transition function `_decide_continuation` (`letta/agents/letta_agent_v3.py:1967`) decide whether the proposed action is allowed, must be rewritten, must wait for approval, or ends the loop.

The runtime wins: even if the LLM proposes a disallowed tool, `parallel_tool_calls` truncation (`letta/agents/letta_agent_v3.py:1337`), tool-rule filtering (`letta/agents/letta_agent_v3.py:2041`), and prefilled-arg rewriting via `merge_and_validate_prefilled_args` (`letta/agents/helpers.py:465`) override LLM intent. A separate class of run-level authorities (`_check_run_cancellation` at `letta/agents/letta_agent_v2.py:750`, `StepProgression` enum at `letta/schemas/step.py:68`, `StopReasonType` enum at `letta/schemas/letta_stop_reason.py:9`) drives exit and persistence decisions without consulting the model.

Multi-agent flows add a fourth owner: a `SleeptimeMultiAgent` super-class wraps the step result and dispatches background tasks to other agents (`letta/groups/sleeptime_multi_agent_v3.py:127`, `letta/groups/sleeptime_multi_agent_v4.py:132`).

## Rating

**Score: 9/10 — Mature, durable, observable, extensible, and proven under failure or scale.**

Justification: transitions are explicit (`_decide_continuation`, `StepProgression`, `StopReasonType`), runtime guards override every model action (approval gate at `letta/agents/letta_agent_v3.py:1709`, rule violation rewrite at `letta/agents/letta_agent_v3.py:1826`), parallel execution is opt-in and validated (`letta/agents/letta_agent_v3.py:1849`), and the control surface is testable without an LLM (`tests/test_tool_rule_solver.py:34`–`1133`). The only deductions are: control logic is split across three near-duplicate drivers (`LettaAgent`, `LettaAgentV2`, `LettaAgentV3` — see `letta/agents/letta_agent.py`, `letta/agents/letta_agent_v2.py`, `letta/agents/letta_agent_v3.py`), and the v1 driver still overrides heart-beat semantics differently (`request_heartbeat=True` at `letta/agents/letta_agent_v2.py:910` vs. `False` at `letta/agents/letta_agent_v3.py:2071`).

## Evidence Collected

Every entry includes a `path:line` citation from the selected source.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Loop driver / next-step owner | `for i in range(max_steps):` loop in blocking `step()` | `letta/agents/letta_agent_v3.py:328` |
| Loop driver / streaming | `for i in range(max_steps):` loop in `stream()` | `letta/agents/letta_agent_v3.py:569` |
| Loop driver / v2 | Step loop with `continue`/`break` based on `self.should_continue` | `letta/agents/letta_agent_v2.py:402-413` |
| Factory selecting driver | `AgentLoop.load()` chooses by `agent_type` / `enable_sleeptime` | `letta/agents/agent_loop.py:19-62` |
| Abstract contract | ABC with `step`, `stream`, `build_request` | `letta/agents/base_agent_v2.py:18-105` |
| Transition function | `_decide_continuation` — returns `(continue, reason, stop_reason)` | `letta/agents/letta_agent_v3.py:1967-2036` |
| Transition function (v2) | Earlier `_decide_continuation` with `request_heartbeat` arg | `letta/agents/letta_agent_v2.py:1241-1285` |
| Termination enums | `StopReasonType` (12 outcomes) | `letta/schemas/letta_stop_reason.py:9-22` |
| Run-level status | `RunStatus` (`created`, `running`, `completed`, `failed`, `cancelled`) | `letta/schemas/enums.py:152-161` |
| Run-status mapping | `StopReasonType.run_status` property | `letta/schemas/letta_stop_reason.py:24-49` |
| Step-level FSM | `StepProgression = {START, STREAM_RECEIVED, RESPONSE_RECEIVED, STEP_LOGGED, LOGGED_TRACE, FINISHED}` | `letta/schemas/step.py:68-74` |
| Step persistence status | `StepStatus = {PENDING, SUCCESS, FAILED, CANCELLED}` | `letta/schemas/enums.py:268-274` |
| Step lifecycle writers | `log_step_async`, `update_step_stop_reason`, `update_step_error_async`, `update_step_cancelled_async` | `letta/services/step_manager.py:339, 368, 519, 773, 798, 812` |
| Cancellation runtime gate | `_check_run_cancellation` queried at top of each step | `letta/agents/letta_agent_v2.py:750-757` |
| Cancellation call site | `if run_id and await self._check_run_cancellation(run_id):` | `letta/agents/letta_agent_v3.py:1031-1035` |
| Credit gate | `await credit_task` stops loop on failure | `letta/agents/letta_agent_v3.py:334-339` |
| Approval-request gate | `_maybe_get_approval_messages` + pause with `requires_approval` | `letta/agents/letta_agent_v3.py:973, 1682-1709` |
| Tool-rule source | `BaseToolRule` schema + 9 discriminated subtypes | `letta/schemas/tool_rule.py:13-373` |
| Rule solver (state machine) | `ToolRulesSolver` tracks `tool_call_history`, caches prefilled args, exposes `should_force_tool_call` | `letta/helpers/tool_rule_solver.py:24-298` |
| Rule decision driver | `solver.get_allowed_tool_names` intersects child/parent/conditional/max-count rules | `letta/helpers/tool_rule_solver.py:96-172` |
| Force-tool enforcement | LLM call passes `force_tool_call` and `requires_subsequent_tool_call` to disable free-form text | `letta/agents/letta_agent_v3.py:1092-1108` |
| Tool-rule continuation hints | `solver.is_terminal_tool`, `is_continue_tool`, `has_children_tools`, `get_uncalled_required_tools` | `letta/helpers/tool_rule_solver.py:174-207` |
| Required-before-exit enforcement | Loop continues with explicit heartbeat when required tool missing | `letta/agents/letta_agent_v3.py:1625-1641, 1994-1997` |
| Tool violation rewrite (runtime) | `_build_rule_violation_result` converts violation to tool-return error | `letta/agents/helpers.py:501-505` |
| Tool violation call site | Violated specs run `_run_one` returning an error instead of executing | `letta/agents/letta_agent_v3.py:1822-1836` |
| Prefilled-arg rewrite | `merge_and_validate_prefilled_args` overrides LLM-provided values | `letta/agents/helpers.py:465-493` |
| Prefilled provenance | `last_prefilled_args_provenance` populated from rule source | `letta/helpers/tool_rule_solver.py:138-170` |
| Parallel truncation | If `parallel_tool_calls=False` and LLM returned ≥2, keep only the first | `letta/agents/letta_agent_v3.py:1337-1342` |
| Parallel grouping | `enable_parallel_execution` flag decides `asyncio.gather` vs serial | `letta/agents/letta_agent_v3.py:1840-1862` |
| Anthropic parallel override | Sets `disable_parallel_tool_use` when any constraining tool rule present | `letta/agents/letta_agent_v3.py:1119-1149` |
| OpenAI parallel override | Same intent via `parallel_tool_calls=False` | `letta/agents/letta_agent_v3.py:1132-1140` |
| Auto-mode routing | `routing_client.resolve_auto_mode_config`, `apply_reroute_rules` | `letta/agents/letta_agent_v3.py:1051-1065` |
| Fallback routing | Circuit-breaker fallback per `RoutingClient.get_fallback_handle` | `letta/agents/letta_agent_v3.py:1184-1212` |
| Compaction gate | Runtime pauses to compact after step if `context_token_estimate > threshold` | `letta/agents/letta_agent_v3.py:1438-1505` |
| System-prompt overflow abort | `_check_for_system_prompt_overflow` raises before issuing LLM call | `letta/agents/letta_agent_v3.py:741-756` |
| Context retry / fallback | `ContextWindowExceededError` triggers compaction + retry loop | `letta/agents/letta_agent_v3.py:1218-1299` |
| Multi-agent handoff | `SleeptimeMultiAgentV3` runs foreground step then background tasks | `letta/groups/sleeptime_multi_agent_v3.py:62-79` |
| Sleeptime dispatch | `for sleeptime_agent_id in self.group.agent_ids:` with `_issue_background_task` | `letta/groups/sleeptime_multi_agent_v3.py:145-153` |
| Multi-agent V4 multi-loop | Background frequency counter + `safe_create_task` | `letta/groups/sleeptime_multi_agent_v4.py:132-198` |
| Approval response routing | `validate_approval_tool_call_ids` then `_decide_continuation` | `letta/agents/helpers.py:121-145, 294-297` |
| Idempotency on approval retry | Surfaces past `tool_returns` to skip duplicated tool dispatch | `letta/agents/helpers.py:230-293` |
| Approval-pause HTTP code | `PendingApprovalError → HTTP 409` | `letta/server/rest_api/routers/v1/agents.py:1801-1806` |
| Default approval rules at agent create | `RequiresApprovalToolRule` auto-appended for tools with `default_requires_approval=True` | `letta/services/agent_manager.py:487-488` |
| Modify approval at runtime | `PATCH /{agent_id}/tools/approval/{tool_name}` adds/removes rule | `letta/services/agent_manager.py:3064-3080`, `letta/server/rest_api/routers/v1/agents.py:714-740` |
| Tool executor factory | `ToolExecutorFactory._executor_map` keyed by `ToolType` enum | `letta/services/tool_executor/tool_execution_manager.py:32-65` |
| Tool executor dispatch | `execute_tool_async` routes to right `ToolExecutor` subclass | `letta/services/tool_executor/tool_execution_manager.py:94-160` |
| Tool executor call site | LettaAgentV3 `_execute_tool` defers to `ToolExecutionManager` | `letta/agents/letta_agent_v2.py:1288-1335` |
| Step checkpoint started | `_step_checkpoint_start` writes PENDING step row with metrics | `letta/agents/letta_agent_v2.py:941-966` |
| Step LLM timing | `_step_checkpoint_llm_request_start/_finish` measure provider latency | `letta/agents/letta_agent_v2.py:969-985` |
| Step checkpoint finished | Updates Step with `stop_reason`, metrics, span end | `letta/agents/letta_agent_v2.py:988-1010` |
| Tests without LLM | `ToolRulesSolver` exhaustive unit suite covering every rule type | `tests/test_tool_rule_solver.py:34-1133` |
| Approval flow integration | `integration_test_client_side_tools.py` validates `requires_approval` pause & resume | `tests/integration_test_client_side_tools.py:84-396` |
| Cancellation integration | `integration_test_cancellation.py` asserts `stop_reason=cancelled` | `tests/integration_test_cancellation.py:164-208` |

## Answers to Dimension Questions

### 1. Who decides what happens next?
The runtime does. Each call to `_step` (`letta/agents/letta_agent_v3.py:894`) executes exactly one LLM→tools round-trip and then yields `letta_message` chunks; the surrounding `for i in range(max_steps)` loop (`letta/agents/letta_agent_v3.py:328` blocking, `:569` streaming) drives iteration. Inside `_step`, after `_handle_ai_response` (`letta/agents/letta_agent_v3.py:1345`) the runtime computes a continuation tuple via `_decide_continuation` (`letta/agents/letta_agent_v3.py:1967-2036`) that decides whether to `break`, persist, or fire another step. The LLM only picks **what** to call (the function name + args); the runtime is the **scheduler**.

### 2. Can the LLM bypass runtime control?
No. Three guardrails run regardless of model output:
- Tool allow-list filtering `_get_valid_tools()` (`letta/agents/letta_agent_v3.py:2039-2074`) intersects `ToolRulesSolver` output with `agent_state.tools`, so any name not in the resolved set becomes a written error (`_build_rule_violation_result`, `letta/agents/helpers.py:501-505`, called from `letta/agents/letta_agent_v3.py:1826`).
- Parallel truncation keeps only `tool_calls[0]` when `parallel_tool_calls=False` is configured (`letta/agents/letta_agent_v3.py:1337-1342`).
- Prefilled-arg merging overrides LLM-provided values with `ToolRulesSolver.last_prefilled_args_by_tool` and validates them against the JSON schema, raising `ValueError` on bad pre-fills that produce a synthesized error (`letta/agents/letta_agent_v3.py:1789-1809`, merge helper at `letta/agents/helpers.py:465-493`).
- The system can also reject a *valid* next action entirely by setting `_require_tool_call = true` via the `tool_choice` request parameter (`letta/agents/letta_agent_v3.py:1092-1108`), forcing a tool call even if the model wanted to emit free-form text.

### 3. Can the runtime reject or rewrite the next action?
Yes — see §2 above. In addition: the runtime pauses the entire agent loop on `requires_approval` (`letta/agents/letta_agent_v3.py:1709`, surfaced as `StopReasonType.requires_approval` and HTTP 409 in `letta/server/rest_api/routers/v1/agents.py:1801-1806`); it aborts on `insufficient_credits` (`letta/agents/letta_agent_v3.py:337`); it aborts on `cancelled` (`letta/agents/letta_agent_v3.py:1031-1035`); it rewrites a runaway `step()` with `StopReasonType.max_steps` after the last iteration (`letta/agents/letta_agent_v3.py:394-395`); and it reroutes to a fallback model when a handle is rate-limited (`letta/agents/letta_agent_v3.py:1184-1212`).

### 4. Are transitions explicit or scattered?
Explicit. The transition vocabulary is encoded in three enums:
- `StopReasonType` (`letta/schemas/letta_stop_reason.py:9-22`) with `.run_status` mapping (`letta/schemas/letta_stop_reason.py:24-49`).
- `RunStatus` (`letta/schemas/enums.py:152-161`).
- `StepProgression` (`letta/schemas/step.py:68-74`).

The _decision_ lives in one place: `_decide_continuation` (`letta/agents/letta_agent_v3.py:1967`), with v2's near-duplicate at `letta/agents/letta_agent_v2.py:1241`. Transitions are propagated to the row store via `step_manager.update_step_stop_reason` (`letta/services/step_manager.py:339-365`) and `run_manager.update_run_by_id_async` (`letta/server/rest_api/routers/v1/agents.py:1818-1826`). The only "scatter" is in legacy v1 (`letta/agents/letta_agent.py`) which still ships alongside v2 and v3 — see "Future Considerations" below.

### 5. Is control flow testable without calling an LLM?
Yes — the most decisive evidence is `tests/test_tool_rule_solver.py` (1133 lines) which constructs a `ToolRulesSolver` with each rule subtype and asserts transitions / allow-lists / prefill caches without any LLM or DB (`tests/test_tool_rule_solver.py:34-1133`). Tool dispatch routing is unit-tested via the `ToolExecutor` abstract base (`letta/services/tool_executor/tool_executor_base.py:16-46`). State-machine semantics (`RunStatus` from `StopReasonType`) are tested at `tests/test_run_status_conversion.py`. End-to-end _runtime_ transitions (cancel, approval) require an LLM and live in integration tests (`tests/integration_test_cancellation.py`, `tests/integration_test_client_side_tools.py`), but the policy layer is fully unit-testable.

### 6. If the model asks for a dangerous or invalid next step, does the runtime have authority to stop it?
Yes. Multiple paths:
- A dangerous tool routed through `RequiresApprovalToolRule` is intercepted at `letta/agents/letta_agent_v3.py:1687-1696` and `letta/agents/letta_agent_v3.py:1709`: control jumps to `StopReasonType.requires_approval`, no tool executes.
- A tool violating a `ChildToolRule` / `ParentToolRule` / `MaxCountPerStep` rule is intercepted at `letta/agents/letta_agent_v3.py:1780, 1826`: an error result is synthesized and the loop continues via heartbeat.
- An unknown tool name (LLM hallucination) is caught at `letta/agents/letta_agent_v3.py:1780` (`tool_rule_violated = name not in valid_tool_names`) and again routed through `_build_rule_violation_result` (`letta/agents/helpers.py:501`).
- A dangerous tool chosen when `no_tool_call` is required triggers `StopReasonType.no_tool_call` (`letta/schemas/letta_stop_reason.py:17`).
- At the run level, an external operator can flip `RunStatus` to `cancelled` and the runtime exits at `letta/agents/letta_agent_v3.py:1031-1035`.

## Architectural Decisions

| Decision | Where | Why it matters |
|---|---|---|
| Split control between an out-of-loop driver (`step`/`stream`) and an in-loop method (`_step`). | `letta/agents/letta_agent_v3.py:222`, `:894` | One LLM round-trip per step is a clean unit of work, persistence, and observability. |
| Iterate via classic `for i in range(max_steps)` rather than an async loop or graph. | `letta/agents/letta_agent_v3.py:328, 569` | Familiar control flow, easy to reason about ordering; observability hooks (`_step_checkpoint_*` at `letta/agents/letta_agent_v2.py:941-1003`) run between iterations without suspending. |
| Co-locate policy logic (`ToolRulesSolver`) with the schema (`ToolRule` discriminated union) so transitions are pure functions of (history, rule-set, last-response). | `letta/helpers/tool_rule_solver.py:24`, `letta/schemas/tool_rule.py:1-373` | Allows exhaustive unit testing — see `tests/test_tool_rule_solver.py`. |
| Encode state in `StopReasonType` and `StepProgression` enums instead of free-form strings. | `letta/schemas/letta_stop_reason.py:9`, `letta/schemas/step.py:68` | Type-safe, exhaustively switchable, and mappable to `RunStatus` via a single property at `letta/schemas/letta_stop_reason.py:24-49`. |
| Allow the runtime to **rewrite** model output (prefilled args, violation errors) rather than only reject it. | `letta/agents/helpers.py:465-493, 501-505`, `letta/agents/letta_agent_v3.py:1789-1809, 1826-1836` | Lets the harness steer without burning LLM retries. |
| Pause via `requires_approval` rather than letting the model proceed for HITL. | `letta/agents/letta_agent_v3.py:1709`, `letta/agents/helpers.py:280-310` | Makes HITL a first-class loop state rather than an exception or an external hook. |
| Persist steps as PENDING rows at the start, then update with stop reason. | `letta/agents/letta_agent_v2.py:941-966`, `letta/services/step_manager.py:100-167` | Crash-resilient: a step's outcome is recoverable from disk even if the runtime dies mid-loop. |
| Discriminate between server-side, client-side, and MCP tools at the executor level. | `letta/services/tool_executor/tool_execution_manager.py:32-65` | Different sandbox / runtime characteristics without leaking into the agent driver. |
| Multi-agent: foreground agent's step is followed by background sleeptime-agent tasks. | `letta/groups/sleeptime_multi_agent_v3.py:62-79, 127-153` | The foreground step is the "owner" of the user turn; background steps are owned by a separate scheduler but never block the foreground response. |

## Notable Patterns

- **Single source of truth for stop reasons.** `StopReasonType.run_status` (`letta/schemas/letta_stop_reason.py:24-49`) maps 12 distinct outcomes to 4 terminal `RunStatus` values, eliminating string-typed coupling between Step / Run and the HTTP layer.
- **Policy objects over inline decisions.** `ToolRulesSolver` (`letta/helpers/tool_rule_solver.py:24-298`) is a Pydantic model that holds cached state (`tool_call_history`, `last_prefilled_args_*`) and exposes a typed method per decision (`is_terminal_tool`, `has_children_tools`, `should_force_tool_call`, `get_uncalled_required_tools`).
- **Per-step checkpoint pattern.** Init → START → STREAM_RECEIVED → RESPONSE_RECEIVED → STEP_LOGGED → LOGGED_TRACE → FINISHED (`letta/schemas/step.py:68-74`) plus `StepStatus` (`letta/schemas/enums.py:268-274`) means every iteration produces a recoverable artifact and an OpenTelemetry span (`letta/agents/letta_agent_v2.py:941-1003`).
- **Adapter for transport, factory for executor.** `LettaLLMAdapter` (`letta/adapters/letta_llm_adapter.py`) toggles blocking vs streaming vs SGLang-native in `stream()` (`letta/agents/letta_agent_v3.py:511-550`). `ToolExecutorFactory` (`letta/services/tool_executor/tool_execution_manager.py:32-65`) routes by `ToolType`. This keeps the control-flow decision out of provider and executor details.
- **Concurrency is opt-in per tool.** `enable_parallel_execution` (`letta/schemas/tool.py:62`, `:128`, `:204`) is a per-tool column in the ORM (`letta/orm/tool.py:56`); the runtime uses it to split `exec_specs` into `parallel_items` vs `serial_items` before `asyncio.gather` (`letta/agents/letta_agent_v3.py:1840-1862`).
- **Runtime-level idempotency for approval retries.** When a stale approval arrives, `_prepare_in_context_messages_no_persist_async` scans the last 10 messages (then the full DB) for a matching `tool_returns` and synthesizes a keep-alive message (`letta/agents/helpers.py:230-293`).
- **Routing client owns fail-over, agent owns fallback config.** Auto-mode and rate-limit fallback are handled inside the loop (`letta/agents/letta_agent_v3.py:1051-1212`), separate from the scheduler's iteration control.

## Tradeoffs

| Tradeoff | Cost | Where |
|---|---|---|
| **Three parallel drivers (`LettaAgent`, `LettaAgentV2`, `LettaAgentV3`)** | Duplication of `_handle_ai_response` (1714 lines vs 1595 lines vs ~340 lines difference) and subtle semantic drift (heartbeat on/off, parallel tool override). | `letta/agents/letta_agent.py`, `letta/agents/letta_agent_v2.py`, `letta/agents/letta_agent_v3.py` |
| **`for i in range(max_steps)` is bounded, not policy-based** | When `max_steps == DEFAULT_MAX_STEPS`, you can't dynamically tune the budget from inside the loop without comparing `i` to a copy of `DEFAULT_MAX_STEPS` (`letta/agents/letta_agent_v3.py:394-395`). | `letta/agents/letta_agent_v3.py:328, 569, 394` |
| **Approval gating lives behind an HTTP 409** | Server enforces "next user message must be approval or it's a `PendingApprovalError`" outside the runtime. The runtime itself happily re-enters, but the gateway rejects — acceptable but worth knowing. | `letta/agents/helpers.py:280-310`, `letta/server/rest_api/routers/v1/agents.py:1801-1810` |
| **`StepProgression` is an int enum, not a true FSM** | The `finally:` block in `_step` (`letta/agents/letta_agent_v3.py:1541-1592`) re-checks `step_progression` ranges manually; there's no transition function or invalidation guard. Drift between the FS and the writer is possible. | `letta/agents/letta_agent_v3.py:940, 1546-1574` |
| **Rule-violation rewrites are formatted errors, not actual no-ops** | The LLM sees a `ToolConstraintError` string as a tool return (`letta/agents/helpers.py:504`), which it must parse. The runtime doesn't "skip" the call — it allows a synthetic error tool return that the model reads and reacts to. | `letta/agents/helpers.py:501-505` |
| **Compaction gating is best-effort, not guaranteed** | The post-step compaction block (`letta/agents/letta_agent_v3.py:1438-1505`) wraps in `try/except` and returns without aborting; pre-step `_refresh_messages` is similarly swallow-exception. A persistent failure could allow context overflow on later steps. | `letta/agents/letta_agent_v3.py:968-971, 1506-1509` |
| **Auto-mode routing can swap `llm_client` mid-step** | `letta/agents/letta_agent_v3.py:1072-1074` rewires `llm_adapter.llm_client` and `llm_config` mid-step. Works, but interrupts any state cached on the adapter. | `letta/agents/letta_agent_v3.py:1048-1087` |

## Failure Modes / Edge Cases

| Mode | Behavior | Cited |
|---|---|---|
| LLM emits unknown tool name | Detected at `letta/agents/letta_agent_v3.py:1780` (`tool_rule_violated = True`); executed as `ToolExecutionResult(status="error", func_return="[ToolConstraintError]…")` via `_build_rule_violation_result` (`letta/agents/helpers.py:501-505`); loop continues with heartbeat (`letta/agents/letta_agent_v3.py:2006`). | lines above |
| LLM refuses to call a required-before-exit tool | `_decide_continuation` returns `(True, "ToolRuleViolated: You must call … at least once…", None)` (`letta/agents/letta_agent_v3.py:1994-1997`); heartbeat message is injected. | line above |
| LLM call returns empty | `LLMEmptyResponseError` → `StopReasonType.invalid_llm_response` → `raise` (`letta/agents/letta_agent_v3.py:1180-1182, 1529`). | lines above |
| Rate-limit / 5xx from LLM provider | Caught at `letta/agents/letta_agent_v3.py:1183-1212`; if a fallback handle exists, swaps `active_llm_config`/`active_llm_client` and `continue`s the inner `for llm_request_attempt` loop. No fallback → `StopReasonType.llm_api_error` and raise. | lines above |
| Context window exhausted mid-call | `ContextWindowExceededError` triggers compaction + retry (`letta/agents/letta_agent_v3.py:1218-1284`) bounded by `summarizer_settings.max_summarizer_retries`. | lines above |
| System prompt itself exceeds context | `_check_for_system_prompt_overflow` (`letta/agents/letta_agent_v3.py:741-756`) raises `SystemPromptTokenExceededError` → `StopReasonType.context_window_overflow_in_system_prompt` (treated as `failed` in `letta/schemas/letta_stop_reason.py:39-43`). | lines above |
| Run cancelled by external actor | `_check_run_cancellation` (`letta/agents/letta_agent_v2.py:750-757`) returns True on `RunStatus.cancelled`; runtime sets `StopReasonType.cancelled` (`letta/agents/letta_agent_v3.py:1031-1035`). | lines above |
| Credits exhausted | `credit_task = safe_create_task_with_return(self._check_credits())` is awaited at top of next iteration (`letta/agents/letta_agent_v3.py:334-339`); on failure → `StopReasonType.insufficient_credits`. | lines above |
| Tool crashes (any exception in `execute_tool_async`) | `ToolExecutionManager.execute_tool_async` returns `ToolExecutionResult(status="error", …)` and never propagates (`letta/services/tool_executor/tool_execution_manager.py:131-155`). The agent loop continues. | lines above |
| `asyncio.CancelledError` during tool exec | Caught and converted to error result (same as above). | `letta/services/tool_executor/tool_execution_manager.py:131-142` |
| Mid-stream SSE failure (after first chunk) | `stream()` `except Exception` block sets `StopReasonType.llm_api_error` (or `context_window_overflow_in_system_prompt` for the right exception) and yields `event: error` instead of finish chunks (`letta/agents/letta_agent_v3.py:649-692`). Adapter always closed in `finally` (`letta/agents/letta_agent_v3.py:737-739`). | lines above |
| Idempotent approval re-submit | Detected by `validate_persisted_tool_call_ids`; treated as already handled (`letta/agents/helpers.py:230-267`). | lines above |
| Approval submitted when no request pending | `PendingApprovalError` → HTTP 409 from the REST layer (`letta/server/rest_api/routers/v1/agents.py:1801-1806`). | lines above |
| Tool-parallel count mismatch (provider ignored `parallel_tool_calls=False`) | Client-side truncation at `letta/agents/letta_agent_v3.py:1337-1342`. | lines above |

## Future Considerations

- **Consolidate drivers**. `letta/agents/letta_agent.py` (1714 lines, includes old heart-beat semantics) and `letta/agents/letta_agent_v2.py` (1487 lines) are still active paths. The `AgentLoop.load()` factory deliberately keeps them in rotation (`letta/agents/agent_loop.py:19-62`). A single driver would shrink the policy surface considerably.
- **`StepProgression` as a real FSM**. The current `int` enum is walked linearly via if/elif at `letta/agents/letta_agent_v3.py:1546-1574`. Codifying allowed transitions (e.g., FINISHED cannot follow START) would catch persistence drift.
- **Dynamic `max_steps`**. The bound lives entirely in the driver; rules like "stop when N consecutive errors" or "stop when total spend > X" require extra plumbing.
- **Cancel granularity**. `_check_run_cancellation` is polled at the **start** of each step (`letta/agents/letta_agent_v3.py:1031`), so a tool that runs for 60s cannot be cancelled mid-execution. A cancellation token handed to `ToolExecutionManager.execute_tool_async` would close the gap.
- **Tool policy auto-binding**. `RequiresApprovalToolRule` is auto-appended at agent creation (`letta/services/agent_manager.py:487-488`), but the corresponding `is_requires_approval_tool` check happens at request time (`letta/agents/letta_agent_v3.py:1690`). A design surface in the agent schema to toggle approval at runtime (`modify_approvals_async` at `letta/services/agent_manager.py:3064`) is good but could be exposed via more endpoints.
- **Parallel-by-default** is currently opt-in (`enable_parallel_execution`), and Anthropic gets parallel-tool-use disabled globally whenever any rule other than `requires_approval` is present (`letta/agents/letta_agent_v3.py:1119-1149`). This is conservative but means real gains from parallel tool use require paying down rule surface first.
- **Multi-agent delegation scope**. `SleeptimeMultiAgentV3` only dispatches background tasks on `every N` turns (`letta/groups/sleeptime_multi_agent_v3.py:139-141`). Making the trigger more pluggable (per-agent subscriptions, content-driven fan-out) is feasible but not present.

## Questions / Gaps

- **Is `requires_approval` enforced only when both `tool_rules_solver.is_requires_approval_tool` and `client_tool_names` agree?** Lines `letta/agents/letta_agent_v3.py:1690-1695` partition requested/allowed tool calls. Why does the runtime not refuse to execute a server tool that the LLM mis-routes as client-side (or vice versa)?
- **What's the contract for `_require_tool_call` when the model emits no tool at all?** `letta/agents/letta_agent_v3.py:124-125, 1644-1647` shows a no-op content-less path that ends with `end_turn` when no uncalled required tools remain; but the LLM is told to use `tool_choice=required` (via `requires_subsequent_tool_call` at `:1105`). Is the docstring at `:107` ("Support tool rules") a list, or a TODO?
- **Where is `client_skills` consumed in the loop?** Referenced at `letta/agents/letta_agent_v3.py:182, 258, 1096` but only injected into the request body — does the runtime control skill activation as it does for tools? No evidence found in this dimension.
- **Are v1 paths still exercised?** `LettaAgent` and `_decide_continuation(at letta/agents/letta_agent.py:1488-?)` exist in the tree but are not referenced by `AgentLoop.load` (`letta/agents/agent_loop.py:19-62`) which only returns `LettaAgentV3` or `LettaAgentV2`. Confirm by reading the dashboard / run history.
- **State during transcript mutations**: `_refresh_messages` (`letta/agents/letta_agent_v2.py:759-777`) is swallow-on-exception (`letta/agents/letta_agent_v3.py:968-971`). What happens when the DB is unreachable mid-step — is `step_progression` still advanced to `FINISHED` even though persistence failed? No evidence found in this dimension.

---

Generated by `01.02-control-flow-ownership` against `letta`.

# Source Analysis: openai-agents-sdk

## 07.02 Sequential vs Parallel Tool Execution

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+ (asyncio; `openai-agents` SDK) |
| Analyzed | 2026-08-26 |

All paths below are workspace-relative to the studied source root `studies/agent-harness-study/sources/openai-agents-sdk/`.

## Summary

The SDK runs a **hybrid, category-based concurrency model**. When a model response contains tool calls, they are split into a `ToolExecutionPlan` with six categories (function tools, computer actions, custom tools, shell calls, apply-patch calls, local-shell calls). The six category executors are launched concurrently via a cancellation-propagating gather (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/tool_planning.py:975-1036`). Within that fan-out:

- **Function tools run in parallel by default**: one `asyncio.Task` per call (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/tool_execution.py:1655-1665`), optionally capped by `RunConfig.tool_execution.max_function_tool_concurrency` (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_config.py:136-158`).
- **Side-effect-bearing built-in tools run serially within their category**: shell, local-shell, apply-patch, custom-tool, and computer-action calls are executed in a plain sequential loop (docstrings say "serially": `studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/tool_execution.py:2334-2459`).

Failure handling is deliberately engineered: per-batch failure arbitration with priority + call-order tie-breaking, sibling cancellation with bounded draining windows, cross-category failure signaling through an `asyncio.Event`, and background-task exception reporting. Result ordering is deterministic — results are keyed to their original plan position and rebuilt in that order regardless of completion order.

Tools do **not** declare side effects; there is no read-only/parallelizable flag on `FunctionTool`. Parallelism policy is decided entirely by tool *category* plus run-level config. The one resource-conflict mechanism is MCP transport-level request serialization for Streamable HTTP servers.

## Rating

**Score: 9 / 10**

Rationale (per rubric): this is a mature, durable design with an explicit concurrency model, operational safeguards, and proven failure behavior:

- Clear model: parallel function tools vs serial write-tools is explicit in code and docstrings (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/tool_execution.py:2343`, `:2370`, `:2397`, `:2424`).
- Explicit interface: `ToolExecutionConfig.max_function_tool_concurrency` validated at construction (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_config.py:152-158`) and documented against provider-side `parallel_tool_calls` (`studies/agent-harness-study/sources/openai-agents-sdk/docs/running_agents.md:191`).
- Safeguards under failure: failure arbitration (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/tool_execution.py:258-304`), bounded sibling drain (0.25 s / 64-step limits, `:167-168`), cross-category event (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/tool_planning.py:1035`), and loop-level reporting of late background failures (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/tool_execution.py:207-255`).
- Proven under failure/scale: dozens of dedicated tests cover default fan-out, caps, order preservation, queued-call suppression after failure, sibling cancellation semantics, parent cancellation, and mixed-category isolation (`studies/agent-harness-study/sources/openai-agents-sdk/tests/test_run_step_execution.py:251-363`, `:727-1160`, `:1375`, `:1452`).

Not 10 because: function tools have no side-effect declaration, so users cannot mark an individual function tool as unsafe to parallelize (short of approval gating or wrapping it as another tool kind), and no resource-conflict detection exists among concurrent function tools themselves — safety is the tool author's responsibility by convention.

## Evidence Collected

Every entry cites `studies/agent-harness-study/sources/openai-agents-sdk/<path>:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Category fan-out | `_execute_tool_plan` gathers all six executors concurrently when `parallel=True` (default); both call sites use the default | `src/agents/run_internal/tool_planning.py:944-1036`; callers `src/agents/run_internal/turn_resolution.py:858`, `:2471` |
| Cross-category failure signal | `sibling_category_failure = asyncio.Event()` set via `on_child_failure=sibling_category_failure.set` | `src/agents/run_internal/tool_planning.py:976`, `:1035` |
| Parallel batch executor | `_FunctionToolBatchExecutor` owns task states, pending set, arbitration state | `src/agents/run_internal/tool_execution.py:1552-1594` |
| One task per call | `_create_tool_task` wraps `_run_single_tool` in `asyncio.create_task`; drain uses `asyncio.wait(..., FIRST_COMPLETED)` | `src/agents/run_internal/tool_execution.py:1655-1665`, `:1667-1683` |
| Concurrency limit | `max_function_tool_concurrency` field + validation (≥ 1); slot-filling refills as tasks settle | `src/agents/run_config.py:136-158`; `src/agents/run_internal/tool_execution.py:1590-1594`, `:1643-1653` |
| Default = start all | cap `None` → `available_slots = len(pending_tool_runs)`; `isolate_parallel_failures` defaults to `len(tool_runs) > 1` | `src/agents/run_internal/tool_execution.py:1643-1649`, `:1573-1575` |
| Serial write-tools | `execute_custom_tool_calls`, `execute_local_shell_calls`, `execute_shell_calls`, `execute_apply_patch_calls`, `execute_computer_actions` each loop sequentially | `src/agents/run_internal/tool_execution.py:2334-2358`, `:2361-2385`, `:2388-2412`, `:2415-2439`, `:2442+` |
| No side-effect metadata | `FunctionTool` fields include `needs_approval`, timeouts, guardrails — no parallelism/read-only flag | `src/agents/tool.py:440-559` |
| Failure arbitration | `_FunctionToolFailure(order, source)`; priority CancelledError(0) < Exception(1) < BaseException(2); ties broken by lower call order | `src/agents/run_internal/tool_execution.py:180-186`, `:258-283` |
| Sibling cancellation + bounded drain | cancel siblings, drain cancelled tasks ≤ 0.25 s / 64 self-progress steps; wait post-invoke siblings ≤ 0.1 s | `src/agents/run_internal/tool_execution.py:167-169`, `:1685-1714`, `:488-548` |
| Nested-category failure drain | executor detects `sibling_category_failure.is_set()` and settles its own tasks without propagating | `src/agents/run_internal/tool_execution.py:1626-1635`, `:1753-1775` |
| Deterministic ordering | results stored keyed by tool-run identity from tasks sorted by `order`; final list iterates original `tool_runs` order | `src/agents/run_internal/tool_execution.py:337-356`, `:2239-2241` |
| Per-tool timeout | `timeout_seconds` enforced with `asyncio.wait_for`; behaviors `error_as_result` / `raise_exception` | `src/agents/tool.py:496-507`, `:2127-2163` |
| Error-to-model conversion | agent default `failure_error_function=default_tool_error_function` turns tool exceptions into model-visible strings | `src/agents/agent.py:599`; `src/agents/tool.py:1863-1870` |
| Gather primitive | `gather_with_cancel` cancels and drains siblings on first raise; used across the pipeline | `src/agents/util/_asyncio_tasks.py:93-114` |
| MCP resource conflict control | Streamable HTTP server sets `_serialize_session_requests=True`; every session request passes through `asyncio.Lock` (`_request_lock`) | `src/agents/mcp/server.py:2250`, `:932-961` |
| Incremental output commit | finished tasks commit `ToolCallOutputItem`s mid-batch via `tool_output_committer` | `src/agents/run_internal/tool_execution.py:2140-2142`; wiring `src/agents/run_internal/turn_resolution.py:840-864`, `:2448` |
| Provider-side knob | `ModelSettings.parallel_tool_calls` controls model emission only, separate from SDK execution | `src/agents/model_settings.py:114-118`; docs `docs/running_agents.md:191`; reference `.agents/references/tool-execution-lifecycle.md:23` |
| Tests: default fan-out | `test_function_tool_concurrency_default_starts_all_calls` asserts max observed concurrency == 3 | `tests/test_run_step_execution.py:251-283` |
| Tests: cap + order | `test_function_tool_concurrency_cap_limits_calls_and_preserves_output_order` asserts max == 2 and outputs ordered ok-1/ok-2/ok-3 despite reversed completion times | `tests/test_run_step_execution.py:287-323` |
| Tests: queued calls not started after failure | cap=1 with failing first call → queued tool never starts; raises `UserError("Error running tool failing_tool: boom")` | `tests/test_run_step_execution.py:327-363` |
| Tests: cross-category isolation | `test_mixed_tool_calls_preserve_shell_output_when_function_tool_cancelled`: cancelled function tool yields error string while shell output survives | `tests/test_run_step_execution.py:1163-1189` |
| Tests: parent cancellation | `test_execute_function_tool_calls_parent_cancellation_skips_post_invoke_work`; eager-task-factory safety at `:1452` | `tests/test_run_step_execution.py:1375`, `:1452` |

## Answers to Dimension Questions

**1. Can multiple tools run in one turn?**
Yes, two levels. Across categories, up to six executors run concurrently under `gather_with_cancel` (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/tool_planning.py:984-1036`). Within the function-tool category, every call becomes its own asyncio task and all start immediately unless capped (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/tool_execution.py:1643-1665`). The number of local handlers running at once can be limited with `RunConfig.tool_execution.max_function_tool_concurrency` (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_config.py:139-144`).

**2. Which tools are safe to parallelize?**
By design, arbitrary user-defined function tools (including MCP tools surfaced as function tools) — the SDK makes no distinction because there is no metadata to declare otherwise (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/tool.py:440-559`). Built-in side-effecting surfaces (shell, apply-patch, custom/client tools, computer actions) are treated as unsafe and serialized per category (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/tool_execution.py:2334-2459`). Agent-as-tool nested runs execute inside the parallel batch like any other function tool.

**3. Are write tools serialized?**
Yes, within each write category: shell/local-shell/apply-patch/custom/computer executors iterate calls sequentially (docstrings "Run ... serially", `studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/tool_execution.py:2370`, `:2397`, `:2424`, `:2343`, `:2451`). However, a write-category call *can* overlap with concurrent function tools in the same turn since categories run together (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/tool_planning.py:984-1036`). There is no global write lock spanning categories; isolation between them is handled by cancellation semantics instead.

**4. How are failures aggregated?**
Three mechanisms. (a) Inside the function batch, completed tasks are recorded and the "preferred" failure selected by priority then lowest call order (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/tool_execution.py:258-283`, `:327-356`); the winner is raised after siblings are cancelled and drained, merging late teardown/post-invoke failures without masking the root cause (`:286-304`, `:1685-1714`). (b) If a non-function category fails, `on_child_failure` sets `sibling_category_failure`, and the function executor drains its own in-flight work rather than racing the exception (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/tool_planning.py:1035`; `tool_execution.py:1626-1635`). (c) Individually, tool exceptions become model-visible strings through `failure_error_function` defaults unless configured to raise (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/agent.py:599`, `src/agents/tool.py:1863-1870`); sibling-cancelled tools likewise get converted error outputs (`tests/test_run_step_execution.py:1183-1186`).

**5. Is result order deterministic?**
Yes. Each task carries its plan `order`; completion recording sorts done tasks by order before storing into `results_by_tool_run` keyed by tool-run identity (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/tool_execution.py:337-356`), and the final results list is assembled by iterating the original `tool_runs` sequence (`:2239-2241`). The order test proves outputs stay ordered even when completion times are inverted (`tests/test_run_step_execution.py:296-323`). Streaming emission order may differ (outputs commit as they finish via `tool_output_committer`, `tool_execution.py:2140-2142`), but the canonical item list is deterministic.

## Architectural Decisions

1. **Category-based policy instead of per-tool metadata.** Parallelism is decided by which executor owns the call, not by declared tool attributes (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/tool_planning.py:944-1036`). This keeps `FunctionTool` simple but pushes safety responsibility onto tool authors.
2. **Parallel-by-default for function tools, opt-in cap.** `max_function_tool_concurrency=None` starts everything; the cap exists for resource-bound hosts (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_config.py:139-156`).
3. **Fail-fast with graceful sibling teardown, not fail-and-collect.** A raising function tool cancels its siblings; cancelled tasks get a bounded window (0.25 s, 64 progress steps) to unwind, and post-invoke phases get 0.1 s to surface in-flight failures before propagation (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/tool_execution.py:167-169`, `:1685-1714`).
4. **Explicit separation of provider-side vs SDK-side concurrency.** `ModelSettings.parallel_tool_calls` governs how many calls the model emits; `tool_execution.max_function_tool_concurrency` governs local handler concurrency (`studies/agent-harness-study/sources/openai-agents-sdk/docs/running_agents.md:191`; `.agents/references/tool-execution-lifecycle.md:23`).
5. **Transport-aware MCP serialization.** Only Streamable HTTP sessions force serialized requests (single-session protocol constraint), leaving stdio/SSE free to interleave (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/mcp/server.py:952-961`, `:2250`).

## Notable Patterns

- **Slot-refill scheduling**: a lightweight semaphore equivalent — `_fill_tool_task_slots` pops queued runs whenever a task settles, avoiding a separate semaphore primitive (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/tool_execution.py:1643-1653`, `:1683`).
- **Failure arbitration dataclass**: `_FunctionToolFailure(error, order, source)` with deterministic precedence makes multi-failure outcomes reproducible (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/tool_execution.py:180-186`, `:267-283`).
- **Cancellation-aware invoke wrapper**: `_await_invoke_task` shields the invoke task, distinguishes sibling-teardown vs parent cancellation, and re-raises real (non-cancel) causes discovered during drain so root causes aren't swallowed as cancellations (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/tool_execution.py:2168-2203`).
- **Background-task hygiene**: detached cleanup/post-invoke tasks get done-callbacks that route exceptions to the loop's exception handler with contextual messages, preventing silent loss ("Task exception was never retrieved") (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/tool_execution.py:207-255`, `:1700-1707`).
- **Eager-task-factory compatibility**: explicit support for `asyncio.eager_task_factory` environments with a dedicated test (`tests/test_run_step_execution.py:1452`; progress-deadline helper `src/agents/run_internal/_asyncio_progress.py`).

## Tradeoffs

- **Latency vs corruption risk for function tools is left to the developer.** The harness assumes user function tools are concurrency-safe; nothing prevents two parallel calls mutating shared context state except the `RunContextWrapper` being shared. The rubric question "does concurrency improve latency without risking corruption?" is answered "yes, by convention" — the only structural protections are category serialization and MCP request locks.
- **Serial write-tools trade throughput for predictability**, which is the right default for patch/shell surfaces, but means a turn mixing many hosted shell calls cannot overlap them even if independent.
- **Fail-fast cancels possibly-completed work**: a sibling that already produced side effects gets cancelled during unwinding; the SDK converts this into a model-visible error output rather than a result, relying on the model to recover next turn (`studies/agent-harness-study/sources/openai-agents-sdk/tests/test_run_step_execution.py:1183-1189`).
- **Bounded-drain constants are hard-coded** (0.25 s / 64 steps / 0.1 s, `studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/tool_execution.py:167-169`), favoring simplicity over tunability; slow cleanup handlers longer than these windows become fire-and-forget background tasks.

## Failure Modes / Edge Cases

- **Queued calls never start after an earlier failure under a cap** — intentional, tested (`studies/agent-harness-study/sources/openai-agents-sdk/tests/test_run_step_execution.py:327-363`).
- **Late failures during teardown/post-invoke are merged** with the triggering failure by priority so fatal errors (e.g., `BaseException`) are not masked (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/tool_execution.py:286-304`).
- **Parent cancellation skips post-invoke hooks/output-guardrail work** while still attaching reporters to orphaned tasks (`:1375` test; `:1777-1783` implementation).
- **Cross-category race**: if a shell call fails while function tasks are mid-flight, the executor drains them via the `sibling_category_failure` event path instead of letting the gather's cancel hit unguarded code (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/tool_execution.py:1631-1632`).
- **Per-tool timeouts** convert to either model-visible messages or raised `ToolTimeoutError` depending on `timeout_behavior` (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/tool.py:499-507`; test `tests/test_run_step_execution.py:1193-1220`).
- Not found in this source: any deadlock or lost-wakeup handling around `asyncio.wait` on empty sets — the drain loop exits when `pending_tasks` empties (`tool_execution.py:1671`), and no evidence of starvation issues appears in tests reviewed.

## Future Considerations

- A `side_effects`/`read_only` declaration on `FunctionTool` would let authors opt specific tools out of parallel batches without re-categorizing them; today the only lever is `needs_approval` gating (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/tool.py:486-493`).
- Making the drain windows configurable would help hosts with slow cleanup paths.
- Cross-category write/write conflicts (e.g., function tool writing files while apply-patch mutates them) currently rely on user discipline; a shared advisory lock registry would be a natural extension point inside `_execute_tool_plan`.

## Questions / Gaps

- **No evidence found for per-function-tool conflict detection** (resource locks, semaphores beyond slot counting, dependency graphs between calls). Searched `src/agents/` for `Semaphore`, `Lock(`, and related terms; the only lock usage tied to tool invocation is the MCP `_request_lock` (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/mcp/server.py:933`).
- **No evidence found that `parallel=False` is ever exercised in-tree**: both production call sites of `_execute_tool_plan` omit the flag (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/turn_resolution.py:858`, `:2471`); the sequential branch exists (`tool_planning.py:1037-1101`) but no internal caller selects it (likely reserved for tests/embedders).
- Sandbox materialization has its own separate concurrency knobs (`SandboxConcurrencyLimits`, `studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_config.py:161-174`); it was treated as out of scope for tool-call execution and not analyzed in depth here.

---

Generated by dimension 07.02 (Sequential vs Parallel Tool Execution) against `openai-agents-sdk`.

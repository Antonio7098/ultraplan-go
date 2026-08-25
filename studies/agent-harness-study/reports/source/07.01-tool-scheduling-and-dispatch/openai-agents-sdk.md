# Source Analysis: openai-agents-sdk

## Dimension 07.01: Tool Scheduling and Dispatch

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+ (asyncio, httpx, pydantic; OpenAI Responses/Chat Completions APIs) |
| Analyzed | 2026-08-24 |

Citation convention: all paths below are relative to the selected source directory `studies/agent-harness-study/sources/openai-agents-sdk/`.

## Summary

Tool dispatch in the OpenAI Agents SDK is a single-process asyncio pipeline with no external queue, broker, or worker pool. A model response is first converted into typed "tool run" records (`ProcessedResponse`, `src/agents/run_internal/run_steps.py:117-131`), then a turn-scoped `ToolExecutionPlan` is built (`src/agents/run_internal/tool_planning.py:557-573,619-646`) that partitions work by tool category (function, computer, custom, shell, apply_patch, local shell, handoff, MCP approval). Dispatch happens in `_execute_tool_plan` (`src/agents/run_internal/tool_planning.py:944-1101`): the six local-tool category executors run concurrently under `gather_with_cancel` (default), while function tools additionally get intra-category concurrency as individually created `asyncio.Task`s managed by `_FunctionToolBatchExecutor` (`src/agents/run_internal/tool_execution.py:1552-2305`), optionally capped by `RunConfig.tool_execution.max_function_tool_concurrency` (`src/agents/run_config.py:136-158`). All non-function categories execute serially in model order (`src/agents/run_internal/tool_execution.py:2334-2498`). Execution order of *completion* is nondeterministic, but emitted results are deterministically reordered to match model call order (`tool_execution.py:2239-2305`; resume path sort at `turn_resolution.py:2503-2508`), and concurrent failure arbitration is deterministic by priority then call order (`tool_execution.py:258-304`). Dispatch is observable at three layers: per-invocation tracing spans (`function_span`), lifecycle hooks (`on_tool_start`/`on_tool_end`), and streaming run-item events (`tool_called`/`tool_output`). The design is documented as a maintainer contract in `.agents/references/tool-execution-lifecycle.md:21-29` and user-facing config in `docs/running_agents.md:170-191`.

## Rating

**9 / 10** — Clear, explicit scheduling model with a dedicated execution-planning module, an explicit concurrency knob, deterministic ordering and failure-arbitration rules, and unusually deep failure-path test coverage (dozens of tests for sibling cancellation, parent cancellation, drain windows, and late failures). Observable via traces, hooks, and stream events. Not a 10 because: bounded cleanup windows are fixed magic numbers (`0.25s` drain, `64` steps, `0.1s` post-invoke wait, `src/agents/run_internal/tool_execution.py:167-169`), the progress-inspection helper relies on private asyncio attributes on a best-effort basis (`src/agents/run_internal/_asyncio_progress.py:1-8`), and there is no pluggable scheduler abstraction beyond the single concurrency cap (acceptable for an in-process SDK).

## Evidence Collected

Every entry cites a file path with line numbers, relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Discovery phase | Model output parsed into typed runs (`ToolRunFunction`, `ToolRunComputerAction`, `ToolRunShellCall`, ...) grouped in `ProcessedResponse` | `src/agents/run_internal/run_steps.py:62-148` |
| Planning phase | `ToolExecutionPlan` dataclass holds per-category work lists plus pending interruptions | `src/agents/run_internal/tool_planning.py:557-573` |
| Fresh-turn plan builder | `_build_plan_for_fresh_turn` maps processed response buckets into the plan | `src/agents/run_internal/tool_planning.py:619-646` |
| Dispatcher | `_execute_tool_plan` fans out to category executors under `gather_with_cancel(...)` with `on_child_failure=sibling_category_failure.set` | `src/agents/run_internal/tool_planning.py:944-1036` |
| Sequential dispatch mode | Non-parallel branch awaits each category executor in fixed order | `src/agents/run_internal/tool_planning.py:1037-1090` |
| Function-tool executor | `_FunctionToolBatchExecutor.execute()` resolves enabled tools, validates disabled tools before side effects | `src/agents/run_internal/tool_execution.py:1596-1641` |
| Task creation | `_create_tool_task` wraps each function call in `asyncio.create_task(self._run_single_tool(...))` | `src/agents/run_internal/tool_execution.py:1655-1665` |
| Concurrency cap | `_fill_tool_task_slots` refills slots up to `config.tool_execution.max_function_tool_concurrency` | `src/agents/run_internal/tool_execution.py:1643-1653`, `1590-1594` |
| Config surface | `ToolExecutionConfig.max_function_tool_concurrency` documented as SDK-side cap distinct from provider `parallel_tool_calls`; validated ≥ 1 | `src/agents/run_config.py:136-158`, `469-470`; validation test `tests/test_run_config.py:368-379` |
| Provider-side parallelism control | `ModelSettings.parallel_tool_calls` forwarded to Responses/Chat Completions requests | `src/agents/model_settings.py:114`; `src/agents/models/openai_responses.py:908-909`; `docs/models/index.md:414` |
| Sync handler offloading | Decorated sync function tools run via `asyncio.to_thread` so they don't block the loop | `src/agents/tool.py:2638-2647` |
| Timeout enforcement | `_invoke_function_tool_with_metadata` wraps invocation in `asyncio.wait_for` when `timeout_seconds` set; timeouts only supported for async handlers | `src/agents/tool.py:2118-2171`; `tests/test_function_tool.py:1404` |
| Serial categories | `execute_custom_tool_calls` / `execute_local_shell_calls` / `execute_shell_calls` / `execute_apply_patch_calls` / `execute_computer_actions` loop serially ("Run ... serially") | `src/agents/run_internal/tool_execution.py:2334-2498` |
| MCP approval callbacks batched concurrently | `execute_mcp_approval_requests` builds one coroutine per request and `gather_with_cancel`s them | `src/agents/run_internal/tool_planning.py:119-196` |
| Deterministic result ordering | Results stored per tool-run id, rebuilt in `tool_runs` order in `_build_function_tool_results` | `src/agents/run_internal/tool_execution.py:2239-2305` |
| Resume-order determinism | Committed outputs sorted by original response call positions | `src/agents/run_internal/turn_resolution.py:2446-2457`, `2503-2508` |
| Dedupe before execution | Duplicate call IDs within a response raise `ModelBehaviorError`; completed invocations filtered by identity fingerprint | `src/agents/run_internal/tool_planning.py:339-554` (esp. 405-486) |
| Failure arbitration | `_select_function_tool_failure`: priority CancelledError < Exception < BaseException, ties broken by call order | `src/agents/run_internal/tool_execution.py:258-283` |
| Sibling teardown policy | Bounded drain constants `_FUNCTION_TOOL_CANCELLED_DRAIN_SECONDS=0.25`, step limit 64, post-invoke wait 0.1s | `src/agents/run_internal/tool_execution.py:167-169` |
| Category-level sibling failure | `sibling_category_failure` asyncio.Event cancels nested function tasks when another category fails | `src/agents/run_internal/tool_planning.py:964-976,1035`; `src/agents/util/_asyncio_tasks.py:93-114`; test `tests/test_run_step_execution.py:4271` |
| Parent vs tool-local cancellation | Shielded invoke task; parent cancellation cancels remaining tasks and attaches late-exception reporters | `src/agents/run_internal/tool_execution.py:2168-2203`, `1777-1783` |
| Dispatch tracing | Each invocation wrapped in `function_span(trace_tool_name)`; input/output only recorded when `trace_include_sensitive_data` | `src/agents/run_internal/tool_execution.py:1800-1819,1854-1856`; span factory `src/agents/tracing/create.py:155-185` |
| Hooks around dispatch | Run-level + agent-level `on_tool_start`/`on_tool_end` gathered around invocation | `src/agents/run_internal/tool_execution.py:2022-2029`, `2158-2165` |
| Streaming observability | `stream_step_items_to_queue` maps items to `tool_called` / `tool_output` events; approval placeholders skipped | `src/agents/run_internal/streaming.py:28-65` |
| Redaction-aware logging | `log_tool_action_error` suppresses tool payload-bearing exceptions unless tool-data logging enabled | `src/agents/run_internal/tool_execution.py:1099-1117`; `src/agents/_debug.py` (`DONT_LOG_TOOL_DATA`) |
| Documented contract | Maintainer reference mandates model-order outputs, sibling isolation, deterministic failure selection, review checklist | `.agents/references/tool-execution-lifecycle.md:21-47` |
| User docs | `tool_execution.max_function_tool_concurrency` documented and explicitly distinguished from provider-side `parallel_tool_calls` | `docs/running_agents.md:170-191` |
| Concurrency behavior tests | Default starts all calls; cap limits concurrency but preserves output order; queued calls unstarted after failure | `tests/test_run_step_execution.py:250-363` |
| Failure-path test depth | ~30 tests covering sibling cancellation, parent cancellation, late fatal exceptions, hook/guardrail failure priority | `tests/test_run_step_execution.py:626-2719`, `4271-4542`; helper tests `tests/test_asyncio_tasks.py:12-152` |

## Answers to Dimension Questions

**1. How does a tool call start?**
The model response is processed into executable work during discovery (`process_model_response`, `src/agents/run_internal/turn_resolution.py:2684+`, bucketed in `ProcessedResponse`, `src/agents/run_internal/run_steps.py:117-131`). Then `execute_tools_and_side_effects` (`src/agents/run_internal/turn_resolution.py:784-919`) dedupes invocations by identity (`_dedupe_processed_response_invocations`, `src/agents/run_internal/tool_planning.py:339`), partitions approvals (`_collect_mcp_approval_plan`, `tool_planning.py:593-616`), builds the plan (`tool_planning.py:619-646`), and hands it to `_execute_tool_plan`. For function tools, start means `asyncio.create_task(_run_single_tool(...))` (`src/agents/run_internal/tool_execution.py:1655-1665`); approval checks and pre-execution guardrails run inside the task before invocation (`tool_execution.py:1822-1838,1858-1990`).

**2. Is tool execution inline or queued?**
In-process, never externally queued. Two levels of scheduling exist: (a) cross-category — all six local tool categories run concurrently under `gather_with_cancel` by default (`src/agents/run_internal/tool_planning.py:975-1036`), or strictly sequentially in a fixed order if `parallel=False`; (b) intra-category — function tools run as individual asyncio tasks started eagerly, with an optional FIFO slot-filling cap (`_fill_tool_task_slots`, `src/agents/run_internal/tool_execution.py:1643-1653`). Computer/custom/shell/apply_patch/local-shell calls execute serially within their category (`tool_execution.py:2334-2498`). Sync decorated handlers are offloaded to threads via `asyncio.to_thread` (`src/agents/tool.py:2643-2647`). There is no worker pool, remote dispatcher, or durable job queue.

**3. Are tool calls ordered?**
Dispatch order follows model emission order (FIFO task creation, `tool_execution.py:1623-1652`), completion order does not, but public output order is restored deterministically: results are keyed by tool-run identity and rebuilt in `tool_runs` order (`_build_function_tool_results`, `tool_execution.py:2239-2305`), and the resume path explicitly sorts outcomes by original call position (`src/agents/run_internal/turn_resolution.py:2503-2508`). This ordering guarantee is codified as a rule: "Preserve model order in emitted outputs even when handlers complete out of order" (`.agents/references/tool-execution-lifecycle.md:25`). When multiple failures race, which failure propagates is deterministic: highest priority wins (fatal > exception > cancelled) with ties broken by lowest call order (`_select_function_tool_failure`, `tool_execution.py:267-283`; tests `tests/test_run_step_execution.py:1884,1933`).

**4. Can tools be batched?**
Yes, at turn granularity. Every tool call emitted in one model response becomes part of one `ToolExecutionPlan` (`src/agents/run_internal/tool_planning.py:557-573`) executed in one dispatch round. Within the batch: function tools run concurrently (cappable), other categories serialize internally, and MCP approval callbacks batch concurrently (`tool_planning.py:195-196`). There is no sub-turn batching API (e.g., grouping arbitrary subsets) and no cross-request batching; batching scope equals the provider response.

**5. Is dispatch observable?**
Yes, through three independent channels: (a) tracing — every function-tool invocation opens a `function_span` named from tool identity (`src/agents/run_internal/tool_execution.py:1800-1805`), with errors attached via `SpanError` (`tool_execution.py:1840-1849`) and sensitive payloads gated behind `trace_include_sensitive_data` (`src/agents/run_config.py:404-410`); hosted/built-in tools trace through `with_tool_function_span` (`tool_execution.py:1120-1139`) and action classes (`tool_actions.py:119-203`); (b) lifecycle hooks — `RunHooks.on_tool_start/on_tool_end` and agent hooks fire once per call including failure/cancellation boundaries (`tool_execution.py:2022-2029,2158-2165`; contract at `.agents/references/tool-execution-lifecycle.md:35`); (c) streaming — `tool_called` and `tool_output` `RunItemStreamEvent`s are emitted per item (`src/agents/run_internal/streaming.py:40-47`). Additionally, structured debug logging exists but is redacted by default (`tool_execution.py:1099-1117`).

## Architectural Decisions

1. **Plan-then-execute separation.** Discovery (`process_model_response`) is deliberately separated from permission-to-run decisions (`tool_planning.py`) and invocation (`tool_execution.py`); this is stated policy: "Keep discovery, approval partitioning, and invocation as separate phases" (`.agents/references/tool-execution-lifecycle.md:7`). Concretely, `execute_tools_and_side_effects` composes the three phases (`src/agents/run_internal/turn_resolution.py:806-865`).
2. **Category-partitioned dispatch rather than a uniform executor queue.** `ToolExecutionPlan` has one list per tool kind (`src/agents/run_internal/tool_planning.py:561-569`), letting function tools get concurrency semantics while side-effect-heavy categories (shell, apply_patch, computer) stay serialized.
3. **Eager task start with optional cap instead of lazy scheduling.** All function calls start immediately unless `max_function_tool_concurrency` is set; the cap is implemented as slot refill from a FIFO list (`src/agents/run_internal/tool_execution.py:1643-1683`), and a failed task prevents queued calls from ever starting (`tests/test_run_step_execution.py:327-363`).
4. **Provider-side vs SDK-side parallelism kept orthogonal.** `ModelSettings.parallel_tool_calls` controls what the model may emit; `RunConfig.tool_execution.max_function_tool_concurrency` controls local execution (`src/agents/run_config.py:139-144`; `docs/running_agents.md:189-191`).
5. **Cancellation is a protocol, not a flag.** Sibling failure triggers cancel → bounded drain → post-invoke wait → deterministic failure merge (`src/agents/run_internal/tool_execution.py:1685-1715`), while parent cancellation must propagate promptly with late exceptions observed via done-callbacks (`tool_execution.py:1777-1783`; `.agents/references/tool-execution-lifecycle.md:27-28`).
6. **Invocation identity as the dedupe key.** Calls are fingerprinted by `(type, call_id, content hash)` and duplicate or already-completed identities are dropped or rejected before any callback runs (`src/agents/run_internal/tool_planning.py:89-116,339-486`), making re-dispatch after resume idempotent-by-construction.

## Notable Patterns

- **`gather_with_cancel` primitive**: gather that reports the first child failure, then cancels and drains siblings, with an `on_child_failure` hook used to signal category-level failure across executors (`src/agents/util/_asyncio_tasks.py:93-114`; usage `tool_planning.py:984-1036`).
- **Batch executor object**: mutable per-batch state (task states, results keyed by tool-run id, guardrail accumulators) encapsulated in `_FunctionToolBatchExecutor` (`src/agents/run_internal/tool_execution.py:1552-1601`) instead of module globals.
- **Deterministic failure arbitration**: explicit priority ordering plus tie-break by call order (`tool_execution.py:258-304`), so raising behavior doesn't depend on task-set iteration order.
- **Shielded inner invocation**: `asyncio.shield(invoke_task)` distinguishes "outer bookkeeping task cancelled" from "tool itself cancelled", enabling tool-local failure policies like `failure_error_function` for cancelled siblings (`tool_execution.py:2168-2203`).
- **Best-effort progress introspection**: `_asyncio_progress.get_function_tool_task_progress_deadline` inspects coroutine frames and private loop attributes to decide whether a cancelled task can still make progress, failing safe with `None` (`src/agents/run_internal/_asyncio_progress.py:1-8,18-80`).
- **Streaming as projection**: dispatch observability for streaming consumers is a pure mapping from internal `RunItem` types to event names (`src/agents/run_internal/streaming.py:28-65`), keeping the scheduler unaware of subscribers.

## Tradeoffs

- **Eager start maximizes latency win but amplifies side-effect races**: all function calls begin immediately by default; users must opt into capping via config (`docs/running_agents.md:189`). Serial-by-default for shell/computer/apply_patch trades throughput for predictable device/file mutation order.
- **Fixed bounded cleanup windows favor prompt failure propagation over completeness**: a sibling whose cleanup exceeds 0.25s/64 steps/0.1s is abandoned as a background task with only a loop-level exception report (`src/agents/run_internal/tool_execution.py:167-169,207-255`).
- **Private asyncio internals for progress detection**: enables precise drain behavior across CPython versions but is inherently fragile; mitigated by fail-safe `None` returns (`src/agents/run_internal/_asyncio_progress.py:1-8`).
- **No pluggable scheduler**: the only tuning knobs are the concurrency cap and the sequential/parallel switch; there is no priority, affinity, rate limiting, or external queue. For an in-process SDK this keeps the mental model small, at the cost of large-scale deployment patterns being out of scope.
- **Order restoration happens at collection, not execution**: callers observing intermediate state (e.g., incremental commit callbacks, `tool_execution.py:2140-2142`; `turn_resolution.py:2448-2460`) can see out-of-order completions; only final item lists are guaranteed ordered.

## Failure Modes / Edge Cases

- **Sibling failure isolation**: a failing function tool cancels siblings, drains them within bounds, preserves already-successful outputs, and raises one deterministically chosen error (`tool_execution.py:1685-1715`; tested at `tests/test_run_step_execution.py:626-756,2235-2582`). Queued-but-unstarted calls are never started after a failure (`tests/test_run_step_execution.py:327-363`).
- **Cross-category failure**: a failure in computer/shell/custom/apply_patch sets `sibling_category_failure`, causing nested function batches to settle and drain rather than hang (`tool_planning.py:976,1035`; `tests/test_run_step_execution.py:4271,4338`).
- **Parent cancellation**: propagates promptly; tool cleanup is not awaited indefinitely and late background exceptions are reported without masking the cancellation (`tool_execution.py:1777-1783,225-255`; `tests/test_run_step_execution.py:2582-2719`, `4486,4542`).
- **Late/post-invoke failures**: post-invoke work (output guardrails, custom-data extraction, hooks) gets a separate 0.1s settlement window and its own failure source tag so it cannot silently mask or be masked by the triggering error (`tool_execution.py:172-186,1738-1751`; tests `2035-2235`).
- **Duplicate/reused call IDs**: rejected with `ModelBehaviorError` before execution, preventing double dispatch (`tool_planning.py:429-458`; dedicated suite `tests/test_tool_approval_call_id_reuse.py`).
- **Disabled tools mid-flight**: a configured-but-disabled tool fails before any sibling starts, and enablement-check errors cancel sibling checks (`tests/test_run_step_execution.py:1503,1544`).
- **Timeouts**: only async handlers support `timeout_seconds`; sync handlers raise at decoration time (`tests/test_function_tool.py:1268-1418`), and timeout conversion is a distinct policy from ordinary failure (`tool.py:2144-2171`).
- **Known softness**: drain-window constants and private-attribute introspection mean pathological handlers (e.g., self-rescheduling sleep loops) can exceed cleanup budgets; behavior is bounded and reported but not fully drained (module docstring admits best-effort semantics, `_asyncio_progress.py:1-8`; bound test at `tests/test_run_step_execution.py:2287`).

## Future Considerations

- Expose the drain/settlement constants (`tool_execution.py:167-169`) as configuration for latency-sensitive hosts that need longer cleanup grace periods.
- Consider a stable public hook or event for "task queued vs started vs settled" to make cap-deferred dispatch externally visible (today only inferable via `on_tool_start` timing).
- Reduce reliance on private asyncio attributes in `_asyncio_progress.py` as public coroutine-introspection APIs evolve.
- If cross-process scale-out ever becomes a goal, the `ToolExecutionPlan` boundary is the natural seam for an out-of-process executor, since plans are already plain data (`tool_planning.py:557-573`) — but nothing today persists or transports them.

## Questions / Gaps

- No evidence found of any durable/persistent job queue or remote tool dispatch mechanism: searches across `src/agents/` for queue/scheduler abstractions surfaced only in-process asyncio primitives (`src/agents/util/_asyncio_tasks.py`, `asyncio.Queue` in `streaming.py:28`). The realtime and voice subsystems have their own loops (`src/agents/realtime/session.py`, `src/agents/voice/pipeline.py`) but were treated as out of scope for the core runner dispatch path.
- The `parallel=False` branch of `_execute_tool_plan` (`tool_planning.py:1037-1090`) is reachable only from internal call sites; no public `RunConfig` field was found that toggles it, and I did not locate a test exercising the non-parallel branch end-to-end. It appears reserved for internal reuse (e.g., nested/resume flows); exact intended entry points are undocumented.
- Whether the fixed drain constants were tuned empirically could not be determined from the repository (no benchmarks or rationale comments found beyond the constant names).
- Hosted (provider-executed) tools have no SDK-side scheduling by definition; only their approval responses are dispatched locally (`run_steps.py:125-126` comment "Hosted tools have already run").

---

Generated by dimension 07.01 (tool-scheduling-and-dispatch) against `openai-agents-sdk`.

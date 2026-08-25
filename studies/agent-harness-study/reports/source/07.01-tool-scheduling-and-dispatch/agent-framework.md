# Source Analysis: agent-framework

## 07.01 Tool Scheduling and Dispatch

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework (Microsoft Agent Framework) |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (asyncio) and C#/.NET; Go SDK lives in a separate repository (`README.md:17`) |
| Analyzed | 2026-08-24 |

## Summary

Microsoft Agent Framework dispatches tool calls through an in-process, asyncio-based function-invocation loop in Python, and by delegating to the external `Microsoft.Extensions.AI` `FunctionInvokingChatClient` pipeline in .NET. In Python, a validated model response containing `function_call` contents is extracted (`python/packages/core/agent_framework/_tools.py:2675`), classified as a whole batch (approval-required / declaration-only / unknown / executable — `python/packages/core/agent_framework/_tools.py:1779-1795`), and then executed **concurrently**: one `asyncio.create_task` per call, each created inside `contextvars.copy_context().run(...)` so OTel span context propagates (`python/packages/core/agent_framework/_tools.py:1848-1862`), joined with `asyncio.gather` which preserves batch order in the returned transcript (`python/packages/core/agent_framework/_tools.py:1864-1879`). Individual execution runs through an optional function-middleware pipeline into `FunctionTool.invoke`, which validates arguments against the JSON schema (`python/packages/core/agent_framework/_tools.py:687-691`) and runs sync function bodies on a worker thread via `asyncio.to_thread` (`python/packages/core/agent_framework/_tools.py:562`). Scheduling strategy is therefore **inline within the request loop, parallel per batch** — there is no external queue or worker pool. Safety is enforced with iteration/call budgets (`max_iterations` default 40, `max_function_calls` default unlimited, `max_consecutive_errors_per_request` default 3 — `python/packages/core/agent_framework/_tools.py:95-96`, `1332-1386`), whole-batch cancellation on fatal middleware failure (`python/packages/core/agent_framework/_tools.py:1863-1876`), and best-effort settlement of dangling calls on service-managed conversations (`python/packages/core/agent_framework/_tools.py:3082-3157`). Dispatch is traced with OpenTelemetry `execute_tool` spans keyed by tool call id (`python/packages/core/agent_framework/observability.py:2284-2322`) and governed by a formal specification with scenario-to-test mapping (`docs/specs/004-python-function-calling-loop.md:325-340,433-532`). Long-running remote work is supported transparently for MCP tools advertising `taskSupport == "required"` via the SEP-2663 task lifecycle (`python/packages/core/agent_framework/_mcp.py:2078-2081`, `2209-2311`). The runtime can explain why a tool ran now at several levels: per-call spans, session-persisted approval state, and budget accounting.

## Rating

**Score: 8/10**

Rationale against the rubric:

- **Clear model with explicit interfaces (7–8 band):** The entire Python dispatch path is a readable, layered pipeline — `FunctionInvocationLayer.get_response` → `_get_response_with_function_invocation` / `_stream_response_with_function_invocation` → `_process_model_function_calls` → `_execute_function_calls` → `_try_execute_function_call_groups` → `_execute_single_function_call` → `_auto_invoke_function` → `FunctionTool.invoke` (`python/packages/core/agent_framework/_tools.py:3036,3174,3337,2971,1905,1725,1664,1437,587`). Configuration is a typed, validated dict (`FunctionInvocationConfiguration`, `python/packages/core/agent_framework/_tools.py:1332-1409`).
- **Tests:** Extensive, including budget overshoot under parallel batches (`python/packages/core/tests/core/test_function_invocation_logic.py:1895-1943`), sibling cancellation on middleware failure (`python/packages/core/tests/core/test_middleware_with_agent.py:2548-2659`), and trace parenting for parallel spans (`python/packages/core/tests/core/test_observability.py:5991-6034`).
- **Operational safeguards:** Batch-wide cancel-and-await on fatal signals, cooperative-cancellation limitations documented and tested (`python/packages/core/tests/core/test_middleware_with_agent.py:2596-2659`), dangling-call settlement for service-side threads, MCP task deadlines and abandonment semantics.
- **Why not 9–10:** There is no durable queue, worker pool, or retry/backoff policy for local tools (a crashed process loses in-flight work); `max_function_calls` is explicitly best-effort and can overshoot by the size of one batch (`python/packages/core/agent_framework/_tools.py:1348-1351`, verified by `test_max_function_calls_limits_parallel_invocations` asserting `exec_counter == 6` with a limit of 5); sync tool bodies cannot be interrupted once handed to `asyncio.to_thread`; and on .NET the actual scheduling loop lives outside this repository in `Microsoft.Extensions.AI`, so ordering/concurrency guarantees are only indirectly evidenced here.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Dispatcher entry (Python) | `FunctionInvocationLayer.get_response` binds one executor per run with config/middleware/session and routes to shape-specific loops | python/packages/core/agent_framework/_tools.py:3628-3690 |
| Function-call extraction | Only actionable (`function_call`, not `informational_only`) contents are dispatched | python/packages/core/agent_framework/_tools.py:1654-1655, 2675 |
| Batch classification | Approval-required, declaration-only, unknown-call, executable classification happens before any execution | python/packages/core/agent_framework/_tools.py:1775-1795 |
| Parallel execution | One `asyncio.create_task` per call inside `contextvars.copy_context().run(...)`, joined via `asyncio.gather` | python/packages/core/agent_framework/_tools.py:1848-1864 |
| Deterministic result order | Result groups return in input order regardless of completion order; spec states "Parallel calls retain model order" | python/packages/core/agent_framework/_tools.py:1878-1879; docs/specs/004-python-function-calling-loop.md:340 |
| Executor (single call) | `_execute_single_function_call` wraps `_auto_invoke_function`, suspending run-persistence gating during nested execution | python/packages/core/agent_framework/_tools.py:1664-1695 |
| Middleware-wrapped invocation | `FunctionInvocationContext` + pipeline execute around `tool.invoke`; `MiddlewareTermination`/`MiddlewareFailure` are explicit control-flow signals | python/packages/core/agent_framework/_tools.py:1596-1641 |
| Argument validation before run | Schema validation of parsed arguments prior to dispatch; failures become error results, not exceptions | python/packages/core/agent_framework/_tools.py:1522-1543, 656-691 |
| Sync-offloading executor | Sync tool bodies run off the event loop via `asyncio.to_thread`; async bodies run inline | python/packages/core/agent_framework/_tools.py:555-563 |
| Per-tool rate limiting | `max_invocations` / max-exception limits raise `ToolException` in `__call__` | python/packages/core/agent_framework/_tools.py:530-553 |
| Execution budgets/config | `max_iterations` (default 40), `max_function_calls` (default None), `max_consecutive_errors_per_request` (default 3), `terminate_on_unknown_calls` | python/packages/core/agent_framework/_tools.py:95-96, 1332-1386, 1389-1409 |
| Loop phases | Phase 1 approval resolution → Phase 2 alternate model turns and batch execution → Phase 3 final no-tools response when budget exhausted | python/packages/core/agent_framework/_tools.py:3213-3335 |
| Budget enforcement point | Limit checked after each completed batch; forces `tool_choice="none"` and disables advertised tools next iteration | python/packages/core/agent_framework/_tools.py:2714-2750, 3302 |
| Streaming dispatch parity | Streaming loop streams each turn, finalizes, executes its calls, then advances transcript; same phases as non-streaming | python/packages/core/agent_framework/_tools.py:3398-3460 |
| Failure containment | On `BaseException` mid-gather: cancel all in-flight sibling tasks, await them, re-raise | python/packages/core/agent_framework/_tools.py:1863-1876 |
| Dangling-call settlement | Service-managed conversation gets synthetic error results + continuation advance after a fail-closed abort | python/packages/core/agent_framework/_tools.py:3082-3157, 3275-3290 |
| Approval-gated dispatch | Mixed batches surface only host-visible approvals; session-backed safe siblings stored as already-approved | python/packages/core/agent_framework/_tools.py:1796-1832; python/packages/core/AGENTS.md (approval harness section) |
| MCP long-running tasks | Tools advertising `taskSupport=="required"` route to SEP-2663 lifecycle: augmented `tools/call` → poll `tasks/get` → `tasks/result`, with deadlines and abandon-vs-terminal rules | python/packages/core/agent_framework/_mcp.py:2078-2081, 2209-2311 |
| MCP retry/reconnect | Plain `call_tool` retries once after reconnect on connection loss | python/packages/core/agent_framework/_mcp.py:2096-2155 |
| Dispatch tracing | `execute_tool` span operation with tool name/call id, duration histogram, exception capture | python/packages/core/agent_framework/observability.py:341, 2284-2322, _tools.py:733-801 |
| Trace context propagation | Tests assert parallel `execute_tool` spans nest under the active agent span | python/packages/core/tests/core/test_observability.py:5991-6034 |
| Background scheduling (harness) | Background agents run via `asyncio.create_task` tracked in an `in_flight_tasks` registry | python/packages/core/agent_framework/_harness/_background_agents.py:118, 479, 619 |
| Workflow-level execution | Workflow executors run as concurrent tasks gathered per superstep; sync handlers offloaded with `asyncio.to_thread` | python/packages/core/agent_framework/_workflows/_runner.py:222-229; _workflows/_function_executor.py:160,172 |
| Formal contract | Spec defines ordering, cancellation, budget scenarios with test mapping | docs/specs/004-python-function-calling-loop.md:325-340, 433-532 |
| .NET delegation | Function invocation loop is the external `FunctionInvokingChatClient`; MAF adds middleware by wrapping each `AIFunction` | dotnet/src/Microsoft.Agents.AI/FunctionInvocationDelegatingAgent.cs:70-87; dotnet/src/Microsoft.Agents.AI/FunctionInvocationDelegatingAgentBuilderExtensions.cs:44-50 |
| .NET pipeline wiring | Harness composes approval binding → bypassing → `UseFunctionInvocation(MaximumIterationsPerRequest)` → message injection → history persistence → OTel | dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgent.cs:250-281 |
| .NET mixed-approval workaround | `ApprovalNotRequiredFunctionBypassingChatClient` counters M.E.AI's all-or-nothing approval batching | dotnet/src/Microsoft.Agents.AI/ChatClient/ApprovalNotRequiredFunctionBypassingChatClient.cs:16-24 |

## Answers to Dimension Questions

1. **How does a tool call start?**
   A model response's `function_call` contents are extracted from the finalized response (`python/packages/core/agent_framework/_tools.py:2675`), normalized to actionable calls (`python/packages/core/agent_framework/_tools.py:1750-1757`), and classified batch-first: if any call needs approval, is declaration-only, or (optionally) unknown, the whole batch pauses before any execution (`python/packages/core/agent_framework/_tools.py:1775-1843`). Executable batches go to `_execute_single_function_call` → `_auto_invoke_function`, which resolves the tool from a name→tool map, parses and schema-validates arguments, and invokes through the middleware pipeline (`python/packages/core/agent_framework/_tools.py:1510-1616`).

2. **Is tool execution inline or queued?**
   Inline and in-process. Calls run concurrently within the current request via `asyncio.gather` over per-call tasks (`python/packages/core/agent_framework/_tools.py:1848-1864`) — not queued jobs. Two adjacent substrates exist: MCP long-running tools become server-side polled tasks surfaced synchronously to the caller (`python/packages/core/agent_framework/_mcp.py:2277-2302`), and the background-agents harness schedules whole agent runs as `asyncio.create_task`s held in an in-flight registry (`python/packages/core/agent_framework/_harness/_background_agents.py:479`). No persistent queue or worker pool exists anywhere in-repo.

3. **Are tool calls ordered?**
   Yes at two levels. Task creation follows model emission order, and `asyncio.gather` returns results in task order, so each call keeps its ordered result group even though completion times differ (`python/packages/core/agent_framework/_tools.py:1864-1879`). The formal spec pins this: "Parallel calls retain model order in the returned transcript" (`docs/specs/004-python-function-calling-loop.md:340`). Start-order determinism is implied by sequential task creation but wall-clock interleaving is inherently nondeterministic; no evidence of strict serialization options was found.

4. **Can tools be batched?**
   Yes — batching is the native unit. All actionable calls in one model turn form one batch executed concurrently while preserving per-call result groups (`python/packages/core/agent_framework/_tools.py:1733-1746`, `1882-1902`), tested with three parallel calls per iteration (`python/packages/core/tests/core/test_function_invocation_logic.py:1905-1942`). Progressive exposure also mutates the shared live tools list between iterations (`python/packages/core/agent_framework/_tools.py:3652-3658`).

5. **Is dispatch observable?**
   Substantially. Every execution emits an OTel `execute_tool` span carrying operation, tool name, and tool call id, plus a duration histogram and optional argument/result attributes gated behind sensitive-data settings (`python/packages/core/agent_framework/observability.py:341, 929, 2284-2322`; `python/packages/core/agent_framework/_tools.py:733-801`). Parallel spans preserve the agent-span parent via copied contexts, asserted by tests (`python/packages/core/tests/core/test_observability.py:5991-6034`). MCP calls add client spans per semconv (`python/packages/core/agent_framework/observability.py:2328-2358`), and debug logging traces batch classification (`python/packages/core/agent_framework/_tools.py:1764-1768`). What is *not* exposed is a dedicated "why did this run now" audit record tying a dispatch decision to its trigger (approval event, model call id chain) beyond what spans/session state imply.

## Architectural Decisions

1. **Own the loop in Python, borrow it in .NET.** Python implements the full function-calling loop in-repo as a chat-client layer (`FunctionInvocationLayer`, `python/packages/core/agent_framework/_tools.py:3036`) composed onto clients via class mixins (`python/packages/core/agent_framework/_clients.py:994-1001`). .NET instead requires the external M.E.AI `FunctionInvokingChatClient` and injects behavior by wrapping each `AIFunction` in a middleware-aware decorator (`dotnet/src/Microsoft.Agents.AI/FunctionInvocationDelegatingAgent.cs:70-87`; builder guard at `dotnet/src/Microsoft.Agents.AI/FunctionInvocationDelegatingAgentBuilderExtensions.cs:44-47`). Consequence: cross-language behavioral parity must be maintained deliberately (the repo documents .NET-style behaviors being ported, e.g., mixed-batch approval bypass in `python/packages/core/AGENTS.md`).

2. **Batch-classify-then-execute.** Classification of the entire batch precedes any execution, so a single approval-needing call pauses the whole batch rather than partially executing siblings (`python/packages/core/agent_framework/_tools.py:1775-1832`). This trades latency for side-effect safety.

3. **Concurrency without queues.** Dispatch is `asyncio` tasks per batch with `to_thread` offloading for sync bodies (`python/packages/core/agent_framework/_tools.py:555-563, 1848-1864`). Simple and zero-infrastructure, but durability is bounded by process lifetime.

4. **Fail-closed escape hatch.** Ordinary tool exceptions convert into `function_result` error contents fed back to the model (`python/packages/core/agent_framework/_tools.py:1412-1434`), but `MiddlewareFailure` aborts the run, cancels the batch, and settles service-side state (`python/packages/core/agent_framework/_tools.py:1863-1876`, `3082-3157`). This separates "model-recoverable" from "host-must-stop" failures.

5. **Spec-driven loop evolution.** The function-calling loop has a written specification with a scenario-to-test mapping that changes to the area must update (`docs/specs/004-python-function-calling-loop.md:433-532`; enforced by `python/AGENTS.md` "Function-Calling Loop Changes" section). Rare discipline for framework internals.

## Notable Patterns

- **Context-preserving fan-out:** `contextvars.copy_context().run(asyncio.create_task, ...)` ensures OTel/ambient context reaches parallel tasks (`python/packages/core/agent_framework/_tools.py:1848-1862`).
- **Ordered result groups:** A `_FunctionExecutionBatch` keeps one content group per call, flattened later — enabling multi-content results (e.g., user-input requests) without breaking call/result pairing (`python/packages/core/agent_framework/_tools.py:1882-1902`).
- **Budget-as-state:** Iteration/call budgets persist in a `budget_state` dict across streaming/non-streaming re-entry so approval resumes don't reset accounting (`python/packages/core/agent_framework/_tools.py:3193-3200, 3610-3613`).
- **Transparent LRO:** Server-advertised long-running MCP tools are indistinguishable to the caller from synchronous ones; fallback paths protect legacy servers and avoid double-execution of side effects (`python/packages/core/agent_framework/_mcp.py:2078-2081`, docstring 2210-2231).
- **Decorator compensation:** .NET decorators patch specific upstream loop behaviors (all-or-nothing approvals, declaration-only early termination) rather than forking the loop (`dotnet/src/Microsoft.Agents.AI/ChatClient/ApprovalNotRequiredFunctionBypassingChatClient.cs:16-24`; `InvocableFunctionBypassingChatClient.cs:16-24`).
- **Cooperative-cancellation honesty:** Tests document exactly what happens to a sync tool body that survives cancellation (side effect completes, result discarded, no further model call) (`python/packages/core/tests/core/test_middleware_with_agent.py:2596-2659`).

## Tradeoffs

- **Parallelism vs. cost control:** `max_function_calls` cannot preempt an in-flight batch, so a single oversized model turn can exceed the cap (`python/packages/core/agent_framework/_tools.py:1348-1351`). Chosen deliberately ("best-effort") to keep batch semantics simple.
- **Inline execution vs. durability:** No persistence of in-flight tool work; a crash loses it. Durability exists only above this layer (workflow checkpointing) or remotely (MCP tasks).
- **Sync offload vs. cancellability:** `asyncio.to_thread` keeps the loop responsive but makes sync bodies uninterruptible; results are discarded but side effects remain (`python/packages/core/agent_framework/_tools.py:1866-1872` comment; test at `python/packages/core/tests/core/test_middleware_with_agent.py:2596-2659`).
- **Externalized .NET loop vs. observability:** Reusing M.E.AI avoids duplicate maintenance but moves concurrency/ordering guarantees out of this repository's auditable surface; in-repo evidence for .NET scheduling is limited to wiring and compensating decorators.
- **Whole-batch approval pause vs. throughput:** Pausing all calls when one needs approval is safe but serializes mixed batches through human latency.

## Failure Modes / Edge Cases

- **Mid-batch fatal failure:** cancels and awaits every sibling task, then re-raises; late-completing sync results never reach the transcript/model/history (`python/packages/core/agent_framework/_tools.py:1863-1876`).
- **Service-conversation orphaned calls:** a fail-closed abort would leave unresolved `function_call` items that providers reject on the next request; mitigated by best-effort settlement with synthetic error results and `tool_choice="none"` (`python/packages/core/agent_framework/_tools.py:3094-3112`).
- **Unknown tool names:** default converts to an error result; `terminate_on_unknown_calls=True` raises instead (`python/packages/core/agent_framework/_tools.py:1484-1491`, `1794-1795`; tests `python/packages/core/tests/core/test_function_invocation_logic.py:2225-2294`).
- **Consecutive errors:** after `max_consecutive_errors_per_request` (default 3) the loop stops invoking tools (`python/packages/core/agent_framework/_tools.py:96, 2126+ test`).
- **MCP task ambiguity:** connection loss before a task id raises "task state unknown" without re-issue, preventing double-start of side-effecting operations; post-id reconnect retries target the same task id (`python/packages/core/agent_framework/_mcp.py:2243-2256`).
- **Budget exhaustion UX:** exhausted loops force one final tool-free response and synthesize fallback text if the reply would otherwise be blank (`python/packages/core/agent_framework/_tools.py:2013-2036`, `3307-3335`).
- **Declaration-only calls:** never executed locally; converted to user-input requests that pause workflows (`python/packages/core/agent_framework/_tools.py:1833-1843`).

## Future Considerations

- A pre-execution (or per-task semaphore) variant of `max_function_calls` would close the batch-overshoot gap for cost-sensitive deployments (`python/packages/core/agent_framework/_tools.py:1348-1351`).
- An interruption story for sync tools (e.g., documented cancellation-token convention or thread-abort boundaries) would make batch cancellation guarantees uniform across sync/async bodies.
- Exposing dispatch decisions (trigger = model turn N, approval event id, budget state) as structured telemetry attributes on `execute_tool` spans would directly answer "why did this tool run now" from traces alone.
- For .NET, documenting or asserting the M.E.AI version's concurrency/ordering contract alongside the spec would give cross-language parity the same evidentiary footing the Python loop has (`docs/specs/004-python-function-calling-loop.md` covers Python only).

## Questions / Gaps

- **No evidence found** for configurable concurrency limits (worker pool size, per-run parallelism caps) in either language; searched `python/packages/core` for `Semaphore|gather|TaskGroup` patterns and `dotnet/src` for `Task.WhenAll|Parallel.` — dispatch concurrency is unbounded except by batch size.
- **No evidence found** for priority ordering or dependency graphs among calls within a batch; execution order is strictly model-emission order.
- **No evidence found** in-repo for .NET-side parallel-dispatch semantics (ordering, cancellation) since the loop resides in the external `Microsoft.Extensions.AI` package; only integration points and compensating decorators are visible here (`dotnet/src/Microsoft.Agents.AI/ChatClient/*.cs`).
- Retry/backoff for local tools appears limited to MCP transport reconnect (`python/packages/core/agent_framework/_mcp.py:2096-2155`); whether other transports need equivalent policies is undocumented.

---

Generated by `07.01-tool-scheduling-and-dispatch` against `agent-framework`.

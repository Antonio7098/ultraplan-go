# Source Analysis: agent-framework

## 07.02 Sequential vs Parallel Tool Execution

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Polyglot monorepo: Python (`python/packages/*`, asyncio-based) and .NET (`dotnet/src/*`, TPL-based). The `go/` directory is a placeholder pointing to a separate repository (`go/README.md:1-3`). |
| Analyzed | 2026-08-26 |

## Summary

The two primary stacks make opposite default choices, and both are explicit about it.

**Python: parallel by default within a model turn.** The function-invocation layer executes every actionable function call in one model response concurrently. `_try_execute_function_call_groups` creates one `asyncio.Task` per call — each created inside `contextvars.copy_context().run(...)` so the active OTel agent span is preserved per parallel invocation — and awaits them with `asyncio.gather`, which returns results in input order as "one ordered content group per function call" (`python/packages/core/agent_framework/_tools.py:1846-1879`). Synchronous tool bodies are offloaded with `asyncio.to_thread` so they do not block the event loop while siblings run (`python/packages/core/agent_framework/_tools.py:555-563`). There is no concurrency cap: nothing in the invocation path limits how many calls in a single batch run at once.

**.NET: sequential by default, opt-in concurrency.** `ChatClientAgentOptions.AllowConcurrentInvocation` (default `false`) is the switch; when set, it configures `FunctionInvokingChatClient.AllowConcurrentInvocation = true` on the pipeline (`dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientAgentOptions.cs:64-75`, wiring in `dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientExtensions.cs:93-109`). The concurrent executor itself lives in the external Microsoft.Extensions.AI package, not this repo; the repo owns only the flag, its default, and tests that pin both. A separate knob, `AllowMultipleToolCalls`, controls whether the model may emit multiple calls per turn and is documented as independent of execution concurrency (`ChatClientAgentOptions.cs:69-70`; mirrored in `dotnet/src/Microsoft.Agents.AI.Workflows.Declarative/ResponseAgentProvider.cs:50-59`).

Both stacks share three properties: failures are isolated per call (ordinary exceptions become error result contents rather than aborting the batch), batch ordering is deterministic (results keep model order), and there is **no general side-effect metadata or write-tool serialization** — tools declare approval requirements, but read/write semantics are not modeled by the scheduler. Conflict management exists only where specific providers know they need it (e.g., an MCP header snapshot lock).

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards on the Python side; weaker generality prevents a higher score.

Rationale:

- Python's contract is specified in a maintained spec with a scenario-to-test mapping (`docs/specs/004-python-function-calling-loop.md:321-343,439,528,532`), including subtle failure semantics (batch cancellation on fatal middleware failure, cooperative cancellation of sync siblings) that are actually tested (`python/packages/core/tests/core/test_middleware_with_agent.py:2548-2594,2596-2659`).
- Ordering determinism is implemented (`asyncio.gather` preserves task order) and asserted ("Parallel calls retain model order in the returned transcript", `docs/specs/004-python-function-calling-loop.md:340`).
- Operational safeguards exist for runaway loops (`max_iterations` default 40, `max_function_calls`, consecutive-error circuit breaker default 3 — `python/packages/core/agent_framework/_tools.py:95-96,1380-1409`) and observability survives parallelism (`_tools.py:1846-1862`; test at `python/packages/core/tests/core/test_observability.py:5992-6068`).
- What keeps it out of 8–10: no side-effect declaration model, so write tools run concurrently with everything else and corruption avoidance is delegated to individual providers (ad hoc locks); no configurability over batch width (no semaphore); the .NET stack's actual concurrent executor is external to the repo and its serial-by-default behavior means the two stacks disagree on the default answer to this dimension.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Parallel executor (Python) | Batch tasks built per call and awaited via `asyncio.gather`; docstring states "Execute multiple function calls concurrently while preserving per-call result groups" | `python/packages/core/agent_framework/_tools.py:1725-1747,1848-1864` |
| Span context propagation | Each task created under `contextvars.copy_context().run(asyncio.create_task, ...)` so "the active agent span is preserved for every parallel tool invocation" | `python/packages/core/agent_framework/_tools.py:1846-1862` |
| Sync tool offloading | Sync functions routed through `asyncio.to_thread` during async invocation | `python/packages/core/agent_framework/_tools.py:555-563` |
| Result ordering | Gather returns ordered groups; loop appends them as one tool-role message preserving order | `python/packages/core/agent_framework/_tools.py:1878-1879,2862` |
| Spec'd ordering guarantee | "Parallel calls retain model order in the returned transcript" | `docs/specs/004-python-function-calling-loop.md:340` |
| Per-call failure isolation | Ordinary exceptions converted to error `function_result` contents (`_function_execution_error_result`), loop continues | `python/packages/core/agent_framework/_tools.py:1412-1417,1573-1574,1640-1641` |
| Control-flow escapes | `MiddlewareTermination` / `MiddlewareFailure` / `UserInputRequiredException` deliberately re-raised, never converted to results | `python/packages/core/agent_framework/_tools.py:1569-1572,1635-1639,1696-1722` |
| Batch failure isolation | On loud escape, all sibling tasks cancelled then awaited (`return_exceptions=True`) before propagating | `python/packages/core/agent_framework/_tools.py:1863-1876` |
| Cooperative-cancellation limit | Sync sibling in a worker thread cannot be interrupted; may complete side effects but result discarded | `docs/specs/004-python-function-calling-loop.md:324-338`; test `python/packages/core/tests/core/test_middleware_with_agent.py:2596-2659` |
| Consecutive-error breaker | `max_consecutive_errors_per_request` (default 3) counted per executed batch via `had_errors`; limit forces `tool_choice="none"` stop | `python/packages/core/agent_framework/_tools.py:96,1383,2718-2733,2856-2860` |
| Budget limits | `max_iterations` (default 40) caps LLM roundtrips; `max_function_calls` caps total invocations; both enforced *after* each parallel batch completes (documented overshoot) | `python/packages/core/agent_framework/_tools.py:95,1332-1409,1348-1351,2714-2758` |
| Budget accounting unit | Count = number of executed result groups per batch (`function_call_count=len(execution.result_groups)`), not per content item | `python/packages/core/agent_framework/_tools.py:3005-3013` |
| Approval pauses whole batch | Any `always_require` tool in the batch halts execution before any sibling runs; safe siblings stored as already-approved for later resume | `python/packages/core/agent_framework/_tools.py:1775-1832` |
| Resource-conflict handling (MCP) | Header-provider snapshot serialized with instance-level `asyncio.Lock`: "parallel invocations on the same instance would otherwise overwrite each other's snapshot"; dedicated concurrency test | `python/packages/core/agent_framework/_mcp.py:3123-3130`; `python/packages/core/tests/core/test_mcp.py:6532-6590` |
| No semaphore / batch cap | No `Semaphore`/concurrency-limit anywhere in core invocation path (searched `Semaphore`, `max_concurrency`, `Lock(` across `python/packages/core/agent_framework/`) | — (negative finding) |
| Side-effect metadata | Only indirect: `approval_mode` gate + free-form `additional_properties` consumed by security middleware (`source_integrity`, `security_label` taint tracking); no read-only/destructive flag feeding scheduling | `python/packages/core/agent_framework/_tools.py:311-353,530-553`; `python/packages/core/agent_framework/security.py:699-733,1091` |
| Model-side parallel knob | `allow_multiple_tool_calls` option translates to OpenAI `parallel_tool_calls`; stripped when no tools present | `python/packages/openai/agent_framework_openai/_chat_completion_client.py:209-210,248,789-791` |
| Workflow fan-out | Edge runners run concurrently via `gather`; per-edge message ordering preserved; same-target delivery serialized ("true parallelism is not realized in Python") | `python/packages/core/agent_framework/_workflows/_runner.py:179-229`; `python/packages/core/agent_framework/_workflows/_edge_runner.py:286` |
| CodeAct fan-out | Monty bridge resolves pending host calls with `asyncio.gather`; instructions tell generated code to use `asyncio.gather` for fan-out; integration test verifies concurrent dispatch | `python/packages/monty/agent_framework_monty/_monty_bridge.py:20,300-314`; `python/packages/monty/agent_framework_monty/_instructions.py:78-79`; `python/packages/monty/tests/monty/test_monty_codeact_integration.py:146-158` |
| .NET opt-in flag | `AllowConcurrentInvocation` (default `false`), independent from `AllowMultipleToolCalls`; applied to external `FunctionInvokingChatClient` | `dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientAgentOptions.cs:64-75`; `dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientExtensions.cs:93-109` |
| .NET declarative surface | Same flags exposed on declarative providers; docs state calls are "processed serially" by default | `dotnet/src/Microsoft.Agents.AI.Workflows.Declarative/ResponseAgentProvider.cs:35-59`; `dotnet/src/Microsoft.Agents.AI.Workflows.Declarative.Foundry/AzureAgentProvider.cs:258` |
| .NET default pinned by tests | Asserts default false; asserts flag propagates to FICC whether pre-existing or factory-created | `dotnet/tests/Microsoft.Agents.AI.UnitTests/ChatClient/ChatClientAgentOptionsTests.cs:27`; `dotnet/tests/Microsoft.Agents.AI.UnitTests/ChatClient/ChatClientExtensionsTests.cs:75-110` |
| .NET workflow concurrency | Fan-out/delivery uses `Task.WhenAll` (agent-level, not tool-level) | `dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcessRunner.cs:316-330`; `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/FanOutEdgeRunner.cs:32` |

Parallel-execution tests observed (non-exhaustive): `test_multiple_function_calls_parallel_execution` (`python/packages/core/tests/core/test_function_invocation_logic.py:3650-3695`), streaming twin (`test_function_invocation_logic.py:4810-4859`), budget overshoot with parallel batches (`test_function_invocation_logic.py:1895-1943`), sibling cancellation twins (`test_middleware_with_agent.py:2548-2594,2596-2659`), span nesting (`python/packages/core/tests/core/test_observability.py:5992-6068`), MCP header serialization (`python/packages/core/tests/core/test_mcp.py:6532-6590`).

## Answers to Dimension Questions

1. **Can multiple tools run in one turn?**
   Yes, on both stacks, gated differently. Python: always — every actionable call in one model response executes concurrently (`_tools.py:1848-1864`). Whether the *model* emits multiple calls is separately controlled by `allow_multiple_tool_calls` → OpenAI `parallel_tool_calls` (`python/packages/openai/agent_framework_openai/_chat_completion_client.py:209-210,248`). .NET: multiple calls per turn require `AllowMultipleToolCalls`, and their parallel *execution* additionally requires `AllowConcurrentInvocation = true`; otherwise the batch runs serially (`dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientAgentOptions.cs:64-75`).

2. **Which tools are safe to parallelize?**
   Not modeled. There is no read-only/write/idempotent metadata on `FunctionTool` that the scheduler consults (`python/packages/core/agent_framework/_tools.py:311-353`). The implicit answer is "everything in an executable batch," subject to two gates that effectively serialize: approval-required tools pause the entire batch before anything executes (`_tools.py:1775-1832`), and declaration-only/hosted calls are returned rather than executed locally (`_tools.py:1833-1843`). Provider-specific safety is handled where known — e.g., MCPStreamableHTTPTool with a header_provider self-serializes via `asyncio.Lock` because concurrent calls would corrupt per-call credentials (`_mcp.py:3126-3130`).

3. **Are write tools serialized?**
   No, not generally. Nothing in the invocation layer distinguishes writes; a write tool in an executable batch runs concurrently with all siblings. Serialization exists only inside specific providers/stores that own mutable state: striped file locks for session stores (`_sessions.py:1904,2274,2438-2443`), write locks in file memory/file access harnesses (`_harness/_file_memory.py:268`, `_harness/_file_access.py:1343`), todo mutation locks per session (`_harness/_todo.py:483-489`), and the MCP header lock above. Cross-tool, cross-resource conflicts are left to tool authors.

4. **How are failures aggregated?**
   Three tiers in Python. (a) Ordinary tool/middleware exceptions become terminal error `function_result` contents carrying exception name/details per `include_detailed_errors`, and the loop continues (`_tools.py:1573-1574,1640-1641`, envelope built at `1412+`). (b) Batch error signal: `had_errors` on `_FunctionExecutionBatch` (`_tools.py:1894-1902`) feeds the consecutive-error counter; reaching `max_consecutive_errors_per_request` (default 3) stops tool use for the request (`_tools.py:2718-2733`, action `"stop"` at `2865-2870`). (c) Fatal escape: `MiddlewareFailure` cancels all in-flight siblings, waits for them, and propagates without being converted into a result; on service-managed conversations the dangling calls are settled first so hosted history stays valid (`_tools.py:1863-1876,3275-3290`; spec `docs/specs/004-python-function-calling-loop.md:324-338`). In .NET, failure aggregation is owned by the external `FunctionInvokingChatClient`.

5. **Is result order deterministic?**
   Yes. `asyncio.gather` returns results in task-creation order, groups are kept per call, and the loop appends them as a single tool-role message in that order (`_tools.py:1864,1878-1879,2862`). The spec makes it contractual: "Parallel calls retain model order in the returned transcript" (`docs/specs/004-python-function-calling-loop.md:340`). Streaming emits the same ordered contents as updates (`_handle_function_call_results` → streaming updates at `_tools.py:2869`; tested at `test_function_invocation_logic.py:4810-4859`). Note this is transcript order — completion times are non-deterministic by design, which the workflow runner documents explicitly for multi-source delivery (`_workflows/_runner.py:188-190`).

## Architectural Decisions

1. **Batch-granularity concurrency, turn-granularity iteration.** Parallelism exists *within* one model turn's calls; iterations remain strictly sequential (model call → execute batch → fold results → next model call) in both streaming and non-streaming loops (`_tools.py:3240-3305` non-streaming, `3337+` streaming). This keeps replay/approval logic tractable while capturing the latency win where it matters.
2. **Order-preserving gather instead of completion-order collection.** Results are correlated by position/call occurrence rather than arrival time, making transcripts deterministic and provider-safe (`_tools.py:1878-1879`; spec line 340).
3. **Errors-as-data, control-flow-as-exceptions.** Ordinary failures are data flowing back to the model; only deliberate signals (`MiddlewareFailure` fail-closed abort, `MiddlewareTermination`, user-input requests) break the batch — a clean separation that makes the cancellation path rare and testable (`_tools.py:1569-1574,1696-1722`).
4. **Fail-closed batch cancellation with honest limits.** On fatal abort the framework cancels siblings but openly documents (and tests) that an in-flight sync thread cannot be interrupted — its side effects may land even though its result is discarded (`docs/specs/004-python-function-calling-loop.md:336-338`; `test_middleware_with_agent.py:2596-2659`).
5. **Post-batch enforcement of budgets.** `max_function_calls` deliberately checks after each batch, accepting overshoot up to the batch size rather than mid-flight cancellation (`_tools.py:1348-1351`); the spec maps this to tests (`docs/specs/004-python-function-calling-loop.md:532`).
6. **Stacks intentionally diverge on default.** Python optimizes latency (parallel default); .NET optimizes safety (serial default, opt-in concurrency), pushing the real executor into Microsoft.Extensions.AI (`ChatClientAgentOptions.cs:64-75`).
7. **Conflict management pushed to owners.** Rather than a central locking service, components that hold shared mutable state own their locks (MCP headers, session stores, memory/todo stores) — consistent with the framework's composition style.

## Notable Patterns

- **Context-preserving task spawn**: `contextvars.copy_context().run(asyncio.create_task, ...)` keeps OTel span hierarchy correct under parallelism (`_tools.py:1846-1862`), verified by a telemetry test asserting exactly two nested tool spans (`test_observability.py:5992-6068`).
- **Per-call result groups** (`list[list[Content]]`) instead of flat lists, so one call producing multiple contents (e.g., user-input requests) still counts and correlates as one unit (`_FunctionExecutionBatch`, `_tools.py:1882-1902`; budget counting at `3008`).
- **Mixed-batch approval split**: when only some calls need approval, safe siblings are hidden, stored session-side keyed to visible request ids, and auto-resumed later — concurrency-friendly HITL without re-prompting (`_tools.py:1796-1832`).
- **Spec-with-test-mapping**: the function-calling loop has a written contract whose scenarios map to named tests, including all parallel/failure cases (`docs/specs/004-python-function-calling-loop.md:439,528,532`) — unusual rigor for concurrency behavior.
- **Model-facing concurrency hint**: Monty CodeAct instructions teach generated code to fan out with `asyncio.gather`, extending parallelism into the model-authored-code tier (`_instructions.py:78-79`; bridge gather at `_monty_bridge.py:312`).

## Tradeoffs

- **Latency vs corruption safety**: unbounded intra-turn parallelism maximizes throughput but offers no protection when two calls touch the same resource; the framework bets that tools are either independent or self-locking. The MCP header lock shows the cost: each conflict-prone component must rediscover and fix the problem itself (`_mcp.py:3126-3130`).
- **Post-hoc budgets vs precision**: checking `max_function_calls` after the batch avoids partial batches and orphaned call/result pairs but can overshoot the budget by up to the batch size — accepted and documented (`_tools.py:1348-1351`).
- **Cooperative cancellation vs honesty**: cancelling siblings on fatal failure stops further work promptly, but sync tools in threads may still complete side effects; the framework chooses transparency (documented + tested) over a false guarantee (`test_middleware_with_agent.py:2596-2659`).
- **Approval-first serialization vs availability**: one approval-needing call freezes the whole batch, trading parallelism for a coherent resume boundary (`_tools.py:1775-1832`).
- **Two stacks, two defaults**: predictable behavior per language, but cross-stack behavioral parity for the same logical agent does not hold for this dimension.

## Failure Modes / Edge Cases

- **Unbounded batch width**: a model emitting N calls spawns N concurrent executions (async tasks; sync ones also consume N threads via `to_thread`). No configuration exists to cap this; resource exhaustion depends entirely on the model's output and tool bodies (inferred from absence — searches for semaphores/concurrency caps in the invocation path found none).
- **Sync-sibling side effects survive abort**: documented cooperative-cancellation limitation — the thread finishes, side effects persist, result discarded (`test_middleware_with_agent.py:2609-2659`).
- **Shared-instance races under parallelism**: `MCPTool` lifecycle/load/header state needs internal locks (`_mcp.py:554-556,3126-3130`); analogous unprotected mutable state on tools would be racy. One concrete inferred risk: `FunctionTool.invocation_count`/`invocation_exception_count` are plain `+=` increments executed inside worker threads for sync tools (`_tools.py:530-553,562`), so parallel invocations of the same sync tool instance can race on its lifetime counters; no test covers this.
- **Error-counting granularity**: the circuit breaker counts *batches* with errors (`errors_in_a_row += 1` per `had_errors` batch), not individual failed calls — one failing call among five successes advances the streak by one, and five simultaneous failures advance it by one (`_tools.py:2718-2733,2856-2860`).
- **Budget overshoot**: up to (limit − prior total + batch size) extra executions can occur before the loop disables tools (`test_function_invocation_logic.py:1939-1943` asserts exactly this: limit 5, six executions).
- **Same-target workflow delivery is serialized**: cross-source messages to one target arrive "one at a time in any order" — determinism per edge but interleaving across sources (`_workflows/_runner.py:186-193`).

## Future Considerations

- Add optional side-effect/read-only metadata on tools (or honor existing `additional_properties` security labels) so schedulers could auto-serialize conflicting tools instead of relying on per-provider locks (`security.py:1091` already defines the vocabulary).
- Expose a batch-concurrency limit (semaphore or `max_parallel_invocations` in `FunctionInvocationConfiguration`, `_tools.py:1332-1386`) for cost/thread-pool control under large model-emitted batches.
- Protect per-instance invocation counters against threaded parallel invocation, or document single-threaded assumptions alongside `max_invocations` (`_tools.py:335-350`).
- Close the .NET gap between repo-owned behavior and the external FICC executor with integration tests that observe actual concurrent execution, not just flag plumbing (`ChatClientExtensionsTests.cs:75-110` currently asserts wiring only).

## Questions / Gaps

- **What exactly does external `FunctionInvokingChatClient` do on failure/ordering?** Its source is not in this repo (external NuGet dependency per `dotnet/AGENTS.md` "External Dependencies"), so .NET-side failure aggregation, ordering, and cancellation semantics could not be verified here — only the flag surface and defaults were inspected. Searched `dotnet/src` for `Task.WhenAll`/`AllowConcurrentInvocation`; no local concurrent-invocation implementation exists.
- **No evidence found** for any timeout/backpressure applied to a parallel batch as a unit (timeouts appear to be per-request/per-tool elsewhere; dimension 07.04 territory).
- **No evidence found** of retry-once/idempotency handling tied to parallel batches in this repo's invocation layer; retries are delegated to callers/providers.
- The Go implementation was out of scope by construction: `go/README.md:1-3` defers to a separate repository, so this dimension could not be assessed for Go.

---

Generated by `07.02-sequential-vs-parallel-tool-execution` against `agent-framework`.

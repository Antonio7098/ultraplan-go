# Source Analysis: letta

## 07.01 Tool Scheduling and Dispatch

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python 3.11+ / FastAPI, asyncio, SQLAlchemy (async), aiomultiprocess, APScheduler, OpenTelemetry |
| Analyzed | 2026-08-24 |

> Note on citations: all file paths below are relative to the selected source root `studies/agent-harness-study/sources/letta/`.

## Summary

Letta dispatches validated tool calls **inline inside the agent's asyncio step loop** rather than through a durable worker queue. A single factory (`ToolExecutorFactory`) routes each call to one of seven typed executors based on the tool's `ToolType`; the chosen executor is awaited directly on the request's event loop. Scheduling strategy varies by tool type: core/builtin/file/MCP tools run in-process, custom sandboxed tools run as OS subprocesses or in remote sandboxes (E2B/Modal), and the experimental cross-agent batch path executes tools in an out-of-process pool via `aiomultiprocess`.

Ordering is deterministic where it matters: per-step results are written back into their original call-index positions, serial tools always run in list order, and only tools explicitly flagged `enable_parallel_execution=True` (default `False`) are dispatched concurrently via `asyncio.gather`. When provider-level `parallel_tool_calls=false`, surplus tool calls are truncated client-side to the first call. Dispatch is heavily observable: OTEL spans with `tool_execution_started`/`completed` events, two dedicated metrics (count + latency histogram), persisted per-step `StepMetrics.tool_execution_ns`, per-step stop reasons, and an event-loop watchdog.

There is no persistent per-tool-call queue; durability exists only at coarser granularities — Runs track status in the DB, sleeptime sub-agents are fire-and-forget asyncio tasks with Run records, and the LLM-batch pipeline (APScheduler + Postgres advisory-lock leader election + Anthropic batch polling) resumes agent loops from persisted batch items. A Temporal-based ("Lettuce") offload exists only as a stubbed client.

## Rating

**Score: 7/10**

Rationale: Letta earns the 7–8 band ("clear model with tests, explicit interfaces, and operational safeguards"): an explicit abstract executor interface (`letta/services/tool_executor/tool_executor_base.py:16-46`) plus a declarative type→executor map (`letta/services/tool_executor/tool_execution_manager.py:35-43`); a per-tool concurrency flag (`letta/schemas/tool.py:62-64`); deterministic result placement (`letta/agents/letta_agent_v3.py:1855-1862`); timeouts and process isolation for sandbox tools (`letta/settings.py:36`, `letta/services/tool_sandbox/local_sandbox.py:196-207`); cancellation checks between steps (`letta/agents/letta_agent_v3.py:1030-1035`); and rich dispatch observability (`letta/agents/letta_agent_v2.py:1336-1348`, `letta/otel/metric_registry.py:105-152`). It does not reach 9–10 because: (a) inline dispatch means a crashed/restarted server loses in-flight steps — only batch mode is resumable; (b) the configured MCP timeout (`letta/settings.py:43`) is never applied to actual MCP tool invocation (`letta/services/mcp/base_client.py:104-113`), so an MCP call can hang a step indefinitely; (c) three agent-loop generations (V1 legacy `letta_agent.py`, V2 single-call, V3 multi-call) coexist, so the dispatch model is not uniform; and (d) the V3 parallel-dispatch branch itself appears untested (searches found no direct tests of `_handle_ai_response`; nearest coverage is cancellation and sandbox integration tests, e.g. `tests/managers/test_cancellation.py:63-143`, `tests/integration_test_tool_execution_sandbox.py:289-406`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Agent loop selection | Factory picks V2/V3/Sleeptime loop from agent type & flags | letta/agents/agent_loop.py:18-63 |
| Step loop | `step()` iterates up to `max_steps`, breaking on `should_continue`/stop reason | letta/agents/letta_agent_v3.py:222-396 |
| Core step method | `_step()` = one LLM call + tool execution + persistence | letta/agents/letta_agent_v3.py:895-1040 |
| Tool-call extraction | Multi-call APIs gathered first, then single-call fallback | letta/agents/letta_agent_v3.py:1326-1333 |
| Parallel enforcement | Surplus tool calls truncated when `parallel_tool_calls=false` | letta/agents/letta_agent_v3.py:1335-1342 |
| Approval gate | Requires-approval & client-side tools diverted to approval message; loop stops | letta/agents/letta_agent_v3.py:1681-1709 |
| Exec specs | Per-call spec built: id/name/args/violation/prefill-error | letta/agents/letta_agent_v3.py:1770-1819 |
| Dispatch split | Single call → await; multiple → `enable_parallel_execution` partition + `asyncio.gather` vs sequential | letta/agents/letta_agent_v3.py:1821-1862 |
| Result ordering | Results written back into original index order | letta/agents/letta_agent_v3.py:1855-1862 |
| Continuation aggregation | Per-tool continue flags aggregated with `any()`; parallel steps force continuation | letta/agents/letta_agent_v3.py:1944-1963 |
| Rule-driven continuation | Terminal/child/continue/required-before-exit rules decide looping | letta/agents/letta_agent_v3.py:1966-2036 |
| Executor interface | Abstract `ToolExecutor.execute(...)` contract | letta/services/tool_executor/tool_executor_base.py:16-46 |
| Dispatcher/factory | `ToolExecutorFactory._executor_map` maps 7 `ToolType`s → executor classes; default = sandbox | letta/services/tool_executor/tool_execution_manager.py:32-65 |
| Execution entry | `ToolExecutionManager.execute_tool_async` awaits executor; catches CancelledError/Exception; trims returns | letta/services/tool_executor/tool_execution_manager.py:94-160 |
| In-process core executor | Function-name → coroutine map for memory/search/send_message tools | letta/services/tool_executor/core_tool_executor.py:29-76 |
| Sandbox routing chain | Modal → E2B/local fallback based on metadata + config | letta/services/tool_executor/sandbox_tool_executor.py:69-130 |
| Subprocess execution | Local sandbox spawns `asyncio.create_subprocess_exec` with timeout, terminate→kill escalation | letta/services/tool_sandbox/local_sandbox.py:192-207 |
| Timeout config | `tool_sandbox_timeout: float = 180`; `mcp_execute_tool_timeout: float = 60.0` | letta/settings.py:36, letta/settings.py:43 |
| MCP call site | MCP tool executed via manager/client; no timeout wrapper applied at call | letta/services/tool_executor/mcp_tool_executor.py:50-63; letta/services/mcp/base_client.py:104-129 |
| Concurrency flag | `enable_parallel_execution` field (default False) documented as concurrent-execution opt-in | letta/schemas/tool.py:62-64 |
| Entry point (blocking) | `send_message` REST handler creates Run, loads loop, awaits `step()` | letta/server/rest_api/routers/v1/agents.py:1746-1798 |
| Background processing | `_process_message_background` runs same loop as detached task, updates Run status | letta/server/rest_api/routers/v1/agents.py:2063-2161 |
| Streaming admission | Redis conversation lock + OTID-derived dedup token before stream starts | letta/services/streaming_service.py:269-347 |
| Cancellation | Run status polled at step boundaries; `cancel_run` marks cancelled | letta/agents/letta_agent_v3.py:1030-1035; letta/agents/letta_agent_v2.py:750-757; letta/services/run_manager.py:619-639 |
| Fire-and-forget tasks | Sleeptime agents dispatched via `safe_create_task` with strong-ref registry | letta/groups/sleeptime_multi_agent_v4.py:171-199; letta/utils.py:1166-1211 |
| Batch scheduler | APScheduler job + Postgres advisory-lock leader election | letta/jobs/scheduler.py:15-79 |
| Batch polling loop | Cron polls Anthropic batches, gathers item results, resumes agent loops concurrently | letta/jobs/llm_batch_job_polling.py:170-239 |
| Batch tool execution | Tools run in `aiomultiprocess.Pool`; `rethink_memory` bulk-special-cased | letta/agents/letta_agent_batch.py:397-451 |
| Direct tool trigger | `POST /agents/{id}/tools/{tool_name}` executes tool outside the loop | letta/server/rest_api/routers/v1/agents.py:748-816 |
| Tracing decorator | `trace_method` wraps methods into OTEL spans w/ param scrubbing | letta/otel/tracing.py:228-268 |
| Span events | `tool_execution_started`/`completed` events with duration_ms/success/tool_type | letta/agents/letta_agent_v2.py:1312-1348 |
| Metrics | `count_tool_execution` counter + `hist_tool_execution_time_ms` histogram | letta/otel/metric_registry.py:105-152; letta/services/tool_executor/tool_execution_manager.py:113-118,156-160 |
| Persisted step metrics | `StepMetrics.tool_execution_ns` etc. stored per step | letta/schemas/step_metrics.py:13-26; letta/agents/letta_agent_v2.py:941-997 |
| Event-loop watchdog | Thread watchdog detects event-loop hangs, records lag metric | letta/monitoring/event_loop_watchdog.py:26-112; letta/test_watchdog_hang.py:29-81 |
| Remote-offload stub | `LettuceClient.step/cancel/status` are no-op placeholders (Temporal integration not present) | letta/services/lettuce/lettuce_client_base.py:36-101 |

## Answers to Dimension Questions

**1. How does a tool call start?**
A tool call originates from the LLM response inside `_step()`: tool calls are collected from the adapter (`letta/agents/letta_agent_v3.py:1326-1333`), optionally truncated if `parallel_tool_calls=false` (`letta/agents/letta_agent_v3.py:1335-1342`), then handed to `_handle_ai_response()` (`letta/agents/letta_agent_v3.py:1345-1366`). There they are validated against allowed tool names (`letta/agents/letta_agent_v3.py:1780`), checked against the requires-approval/client-tool gates (`letta/agents/letta_agent_v3.py:1682-1709`), assembled into exec specs with prefill merging (`letta/agents/letta_agent_v3.py:1770-1819`), and finally routed by `ToolExecutorFactory.get_executor()` keyed on `ToolType` (`letta/services/tool_executor/tool_execution_manager.py:103-111`). Tools can also start without an agent loop via the direct REST trigger `run_tool_for_agent` (`letta/server/rest_api/routers/v1/agents.py:748-816`) or the tools router (`letta/server/rest_api/routers/v1/tools.py:798-828`).

**2. Is tool execution inline or queued?**
Inline. `_run_one` awaits `self._execute_tool(...)` directly in the step coroutine (`letta/agents/letta_agent_v3.py:1822-1841`), which awaits `ToolExecutionManager.execute_tool_async` (`letta/agents/letta_agent_v2.py:1330-1335`). There is no broker/queue for individual tool calls. Queue-like behavior exists only above the tool layer: whole messages can be processed as background tasks (`letta/server/rest_api/routers/v1/agents.py:2063-2112`), sleeptime sub-agents run as fire-and-forget tasks (`letta/groups/sleeptime_multi_agent_v4.py:188-198`), and the Anthropic-batch pipeline uses APScheduler + DB-backed polling with out-of-process execution (`letta/jobs/scheduler.py:60-75`, `letta/jobs/llm_batch_job_polling.py:214-237`, `letta/agents/letta_agent_batch.py:424-425`). Execution location depends on tool type: in-process coroutines for core/builtin/MCP tools, OS subprocess (180 s timeout) or E2B/Modal sandboxes for custom tools (`letta/services/tool_sandbox/local_sandbox.py:192-207`, `letta/services/tool_executor/sandbox_tool_executor.py:69-130`).

**3. Are tool calls ordered?**
Yes, deterministically. Results are placed back at their original spec index (`letta/agents/letta_agent_v3.py:1855-1862`), so message creation order matches LLM call order regardless of completion order (`letta/agents/letta_agent_v3.py:1916-1933`). Within a step: parallel-flagged tools run concurrently via `asyncio.gather`; all others run strictly sequentially in list order (`letta/agents/letta_agent_v3.py:1843-1862`). Cross-provider ordering is enforced client-side by truncating to the first call when `parallel_tool_calls` is disabled (`letta/agents/letta_agent_v3.py:1335-1342`; analogous Gemini handling at `letta/llm_api/google_vertex_client.py:639-648`). Coarser ordering is governed by tool rules — init-first, child-of-parent, terminal/exit-loop, required-before-exit, max-count-per-step (`letta/schemas/tool_rule.py:254-360`) evaluated by `_decide_continuation` (`letta/agents/letta_agent_v3.py:2003-2036`). The legacy V2 loop handles exactly one tool call per step with heartbeat-controlled continuation (`letta/agents/letta_agent_v2.py:1122-1250`).

**4. Can tools be batched?**
Two distinct mechanisms. (a) Intra-step batching: one LLM response may contain several tool calls, dispatched with the parallel/serial split described above. (b) Cross-agent batch mode: `LettaAgentBatch` fans a step out to many agents through Anthropic's Message Batches API (`letta/agents/letta_agent_batch.py:126-231`); after polling completes, tool execution runs across all agents in an `aiomultiprocess.Pool` (`letta/agents/letta_agent_batch.py:423-425`), with `rethink_memory` hoisted into a single bulk block update for efficiency (`letta/agents/letta_agent_batch.py:420-451`). This path is explicitly marked Anthropic-only with TODOs (`letta/agents/letta_agent_batch.py:99-100`, `letta/jobs/llm_batch_job_polling.py:195-197`). Note the docstring claims "out-of-process worker" for every tool, but the Pool is only used in the batch path — normal steps never leave the event loop for non-sandbox tools.

**5. Is dispatch observable?**
Extensively. Every executor entry point is wrapped by `@trace_method` OTEL spans (`letta/otel/tracing.py:228-268`; applied at `letta/services/tool_executor/tool_execution_manager.py:94`, `sandbox_tool_executor.py:27`, `mcp_tool_executor.py:25`). Each execution emits span events `tool_execution_started`/`tool_execution_completed` carrying duration, success, tool type, and tool id (`letta/agents/letta_agent_v2.py:1312-1348`). Two metrics capture outcomes: a counter with success/failure attributes incl. `step.id` on error, and a latency histogram tagged `tool.name` (`letta/services/tool_executor/tool_execution_manager.py:113-118,156-160`; definitions `letta/otel/metric_registry.py:105-152`). Wall-clock timing persists to `StepMetrics.tool_execution_ns` (`letta/schemas/step_metrics.py:22`; write at `letta/agents/letta_agent_v3.py:1864-1866`, `letta/agents/letta_agent_v2.py:1173-1174`). Steps themselves are logged PENDING→finalized with stop reasons and errors (`letta/agents/letta_agent_v2.py:941-966`, `letta/services/step_manager.py:339,368,419,519`). Operational observability includes the event-loop hang watchdog (`letta/monitoring/event_loop_watchdog.py:26-112`) and background-task counting (`letta/utils.py:1160-1211`).

## Architectural Decisions

1. **Type-keyed executor factory instead of conditional dispatch.** A class-level map from `ToolType` to executor class makes the dispatch surface explicit and extensible; unknown types fall back to the sandbox executor (`letta/services/tool_executor/tool_execution_manager.py:35-57`). Adding a tool runtime = adding one enum value + one class.
2. **Inline, awaited dispatch over a work queue.** Simplicity and transactional locality win: tool side effects (memory updates via managers) happen in the same process/loop that persists messages immediately afterwards (`letta/agents/letta_agent_v2.py:1230-1238`). Durability is delegated upward to Run records and the separate batch pipeline.
3. **Opt-in parallelism with conservative default.** `enable_parallel_execution` defaults to `False` (`letta/schemas/tool.py:62-64`), and providers are told to disable parallel tool calling unless configured otherwise (`letta/llm_api/openai_client.py:430,474-476`; `letta/agents/letta_agent_v3.py:1111-1148`). The runtime additionally truncates misbehaving providers' extra calls (`letta/agents/letta_agent_v3.py:1335-1342`).
4. **Process-isolate user-authored code.** Custom tools never run in the server process: local subprocess with terminate→kill escalation, or E2B/Modal remote sandboxes, with Modal tried first only when tool metadata requests it (`letta/services/tool_sandbox/local_sandbox.py:192-207`, `letta/services/tool_executor/sandbox_tool_executor.py:69-130`). Results cross back via marker-delimited stdout parsing, recently switched from pickle to JSON for security (`letta/services/tool_sandbox/local_sandbox.py:213-214`; commit `113153571` 2026-05-14).
5. **Leader-elected cron for batch scheduling.** APScheduler runs only on the instance holding a Postgres advisory lock, avoiding duplicate batch polling across replicas; non-Postgres deployments degrade to no-election (`letta/jobs/scheduler.py:41-79`).
6. **HITL as a dispatch interrupt, not a filter.** Approval-requiring and client-side tools halt the loop with a `requires_approval` stop reason and an approval message; approved calls re-enter the pipeline later through the approval-response path (`letta/agents/letta_agent_v3.py:1682-1709`, `973-1029`).

## Notable Patterns

- **Exec-spec staging:** building a normalized spec list (id/name/args/violation/error) before execution decouples validation from dispatch and lets denials and prefill-errors short-circuit uniformly (`letta/agents/letta_agent_v3.py:1770-1827`).
- **Index-preserving gather:** `asyncio.gather` results zipped back onto `(idx, spec)` pairs keep output ordering stable under concurrency (`letta/agents/letta_agent_v3.py:1854-1862`).
- **Safe background tasks:** `safe_create_task` wraps coroutines, keeps strong references to prevent GC, names tasks by source line, and logs failures — including suppressing expected cancellation exceptions (`letta/utils.py:1190-1211`).
- **Fallback chains:** Modal → E2B/local for sandboxes (`letta/services/tool_executor/sandbox_tool_executor.py:76-130`), and graceful Redis degradation for conversation locks (`letta/services/streaming_service.py:280-295`).
- **Credit check pipelining:** the next step's credit verification is fired as a task during current-step teardown and awaited at next-step entry (`letta/agents/letta_agent_v3.py:333-339,389-390`).
- **Deduplication tokens:** OTID-derived request tokens allow retry recovery of identical background requests before lock acquisition (`letta/services/streaming_service.py:272-295`).

## Tradeoffs

- **Inline execution ↔ crash fragility:** a server restart mid-step abandons the tool call and its step (Run stays failed/stale); nothing replays it. Only the batch path reconstructs state from persisted batch items (`letta/agents/letta_agent_batch.py:233-270`). This trades durability for simplicity.
- **Concurrency ↔ shared-state hazards:** parallel `asyncio.gather` of tools that mutate memory relies on per-tool discipline; the sandbox executor even asserts memory immutability for sandbox tools (`letta/services/tool_executor/sandbox_tool_executor.py:135-138`), pushing conflict-avoidance into convention rather than mechanism.
- **Determinism ↔ wall-clock latency:** strict index-order result placement and serial-by-default execution make traces reproducible but serialize independent slow tools unless authors remember to set the parallel flag.
- **Three loop generations ↔ consistency:** V1 (`letta/agents/letta_agent.py:174-907`), V2 (one tool/step, heartbeat-based), and V3 (multi-call) implement overlapping dispatch semantics, tripling the surface where behavior can drift (e.g., V1's "temp hack" taking only the first parallel call at `letta/agents/helpers.py:380-381` vs V3's full support).
- **Observability depth ↔ overhead:** `trace_method` reflects over every parameter and scrubs large ones (`letta/otel/tracing.py:238-268`), and metrics/spans/events fire on every execution — rich, but paid on the hot path.

## Failure Modes / Edge Cases

- **MCP hang risk:** `mcp_execute_tool_timeout` is defined (`letta/settings.py:43`) but `MCPClient.execute_tool` awaits `session.call_tool` with no timeout (`letta/services/mcp/base_client.py:104-107`); a stuck MCP server stalls the entire agent step (mitigated only by external cancellation).
- **Timeout escalation:** local-sandbox timeout terminates then force-kills after 5 s if needed, raising a descriptive `TimeoutError` (`letta/services/tool_sandbox/local_sandbox.py:198-207`).
- **Cancellation windows:** cancellation is polled at step boundaries (`letta/agents/letta_agent_v3.py:1030-1035`), so a running tool completes before the loop notices; in-flight tool futures aren't individually cancelled (though `execute_tool_async` maps `CancelledError` to an error result, `letta/services/tool_executor/tool_execution_manager.py:131-142`).
- **Parallel continuation quirk:** multi-call steps force `aggregate_continue=True` unless terminal/max-steps, so the agent always takes another turn to summarize parallel results (`letta/agents/letta_agent_v3.py:1954-1963`).
- **Rule/runtime trust gap:** with tool rules present, the code trusts earlier validation that `parallel_tool_calls=false` was enforced, commenting "At runtime, we trust..." (`letta/agents/letta_agent_v3.py:1766-1768`); misconfiguration could admit unordered parallel calls.
- **Batch-mode last-write-wins:** bulk `rethink_memory` collapses multiple agents' writes into a dict keyed by block id, explicitly noted as sensitive to overwrite races (`letta/agents/letta_agent_batch.py:443-444`).
- **Event-loop blocking detection:** a thread-based watchdog logs hangs > threshold and records lag, but cannot recover the loop (`letta/monitoring/event_loop_watchdog.py:110-154`).
- **Streaming skip for aggregated parallel returns:** parallel tool returns lacking per-call ids are excluded from token streaming (`letta/agents/letta_agent_v3.py:1414-1420`), a deliberate gap clients must tolerate.

## Future Considerations

- Apply `mcp_execute_tool_timeout` (or per-call `asyncio.wait_for`) around MCP invocations; the setting is currently dead configuration.
- Introduce a durable per-step/per-tool intent record (or adopt the stubbed Lettuce/Temporal client, `letta/services/lettuce/lettuce_client_base.py:60-87`) so inline dispatch survives restarts.
- Consolidate V1/V2/V3 dispatch into the V3 unified exec-spec model and delete legacy heartbeat plumbing (`REQUEST_HEARTBEAT_PARAM`, `letta/constants.py:217`; V2 usage `letta/agents/letta_agent_v2.py:1126`).
- Add targeted tests for the V3 parallel/serial partition and index restoration (`letta/agents/letta_agent_v3.py:1840-1862`), which currently lack direct unit coverage.
- Generalize the batch pipeline beyond Anthropic (TODOs at `letta/agents/letta_agent_batch.py:99-100`, `letta/jobs/llm_batch_job_polling.py:195-197`) and replace the brittle `rethink_memory` string-matching special case (`letta/agents/letta_agent_batch.py:399-437`).

## Questions / Gaps

- **Test coverage of dispatch core:** searches across `tests/` for `_handle_ai_response`, `enable_parallel_execution`, and gather-based dispatch found only indirect coverage (cancellation, schema parsing, sandbox integration: `tests/managers/test_cancellation.py`, `tests/test_tool_schema_parsing.py`, `tests/integration_test_tool_execution_sandbox.py`). No evidence found of a unit test exercising the parallel/serial split itself.
- **V1 loop status:** `letta/agents/letta_agent.py` remains in-tree with its own `_handle_ai_response`, but `AgentLoop.load` never selects it (`letta/agents/agent_loop.py:18-63`); whether it serves any live route was not confirmed within this source boundary.
- **Sandbox timeout configurability:** `tool_sandbox_timeout=180` is a global constant; per-tool overrides were not found in the schema (`letta/schemas/tool.py` exposes no timeout field).
- **Queue semantics for `run_tool_for_agent`:** the direct REST trigger shares `execute_tool_async` but bypasses Run/Step tracking; audit trail for out-of-loop executions beyond logs/metrics was not evidenced.

---

Generated by `dimensions/07.01-tool-scheduling-and-dispatch` against `letta`.

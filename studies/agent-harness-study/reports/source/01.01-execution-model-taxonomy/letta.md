# Source Analysis: letta

## Execution Model Taxonomy

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI server), Pydantic, SQLAlchemy, OpenTelemetry, APScheduler |
| Analyzed | 2026-07-02 |

## Summary

Letta's execution model is a **classic bounded step loop** (the classic "ReAct-style" agent cycle) wrapped in a thin factory class. The primary entry point is `AgentLoop.load()` at `letta/agents/agent_loop.py:19`, which returns one of four loop implementations chosen by agent type and group membership. All production paths funnel through `step()`/`stream()` methods on `LettaAgentV2`/`LettaAgentV3`, both of which execute `for i in range(max_steps)` (`DEFAULT_MAX_STEPS = 50` from `letta/constants.py:75`) and call `_step()` per iteration. Each `_step()` makes one LLM call, executes any returned tool calls, persists messages, and lets `ToolRulesSolver` decide whether to continue (`continue_stepping`/`should_continue` flag).

The model is **explicit** in `LettaAgentV2`/`V3` (a documented, namespaced `StepProgression` enum at `letta/schemas/step.py:68-74` tracks START → STREAM_RECEIVED → RESPONSE_RECEIVED → STEP_LOGGED → LOGGED_TRACE → FINISHED inside each `_step()`). The repo layers **multiple coexisting execution models**: legacy `LettaAgent` (`letta/agents/letta_agent.py`), unified `LettaAgentV2` (`letta/agents/letta_agent_v2.py`), simplified `LettaAgentV3` (`letta/agents/letta_agent_v3.py`), `VoiceAgent`/`VoiceSleeptimeAgent` (custom OpenAI-stream loop), `LettaAgentBatch` (anthropic-batched async model with `step_until_request`/`resume_step_after_request`), and `SleeptimeMultiAgentV3/V4` (extends `LettaAgentV2`/`V3` to fan out background "sleeptime" agent runs after the foreground step completes).

**One sentence**: Execution advances by iterating `for i in range(max_steps)` and calling `_step()` once per iteration, where each `_step()` runs a single LLM call, executes any returned tool calls, and lets a `ToolRulesSolver`-driven `should_continue`/`stop_reason` flag decide whether to continue the loop.

## Rating

**5/10 — Present but multi-model and inconsistent**

The Letta loop is well-architected at the per-step level (clear `StepProgression`, well-named checkpoints, telemetry, retry/fallback logic). But the project carries **three concurrent in-tree step-loop implementations** (`letta_agent.py`, `letta_agent_v2.py`, `letta_agent_v3.py`), a **factory-class shell** (`AgentLoop`) that exists mostly to pick between them, and **two more loops** (`VoiceAgent`, `LettaAgentBatch`). The mix of `BaseAgent` (legacy) vs `BaseAgentV2` (current) interfaces, the `max_summarizer_retries + 1` retry pattern inlined into `_step()` rather than lifted into a separate retry abstraction, and the duplicated "continue stepping" decision logic in both `_step()` and `_decide_continuation()` show the model is **explicit but inconsistently applied** rather than cleanly designed. Documentation of the loop is sparse — the only canonical source is the code itself.

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Factory entry point | `AgentLoop.load(agent_state, actor)` returns one of 4 implementations by type | `letta/agents/agent_loop.py:15-63` |
| Agent type taxonomy | `AgentType` enum: memgpt_agent, memgpt_v2_agent, letta_v1_agent, react_agent, workflow_agent, sleeptime_agent, voice_convo_agent, voice_sleeptime_agent | `letta/schemas/agent.py:38-51` |
| Step-budget constant | `DEFAULT_MAX_STEPS = 50` | `letta/constants.py:75` |
| V3 outer loop (blocking) | `for i in range(max_steps)` invokes `self._step(...)` and inspects `self.should_continue` / `self.stop_reason` | `letta/agents/letta_agent_v3.py:222-441` |
| V3 outer loop (streaming) | `for i in range(max_steps)` with parallel `safe_create_task_with_return(self._check_credits())` | `letta/agents/letta_agent_v3.py:443-717` |
| V3 inner step | `_step()` body: load step metrics → maybe call LLM → handle approval → persist → emit messages → return | `letta/agents/letta_agent_v3.py:895-1600` |
| V2 outer loop | `for i in range(max_steps)` with `credit_task` parallelism (same shape as V3) | `letta/agents/letta_agent_v2.py:191-297` (blocking), `:300-442` (streaming) |
| V2 inner step | `_step()` builds request, calls `llm_adapter.invoke_llm(...)`, runs `_handle_ai_response(...)` | `letta/agents/letta_agent_v2.py:445-727` |
| Continue/stop decision | `_decide_continuation()` evaluates `request_heartbeat`, tool rules (`is_terminal_tool`, `has_children_tools`, `is_continue_tool`) | `letta/agents/letta_agent_v2.py:1245-1285`; `letta/agents/letta_agent_v3.py:1985-2036` |
| StepProgression enum | `START, STREAM_RECEIVED, RESPONSE_RECEIVED, STEP_LOGGED, LOGGED_TRACE, FINISHED` | `letta/schemas/step.py:68-74` |
| Step checkpoints | `_step_checkpoint_start/llm_request_start/llm_request_finish/finish` | `letta/agents/letta_agent_v2.py:940-1034` |
| Legacy step loop | `LettaAgent.step()` and `_step()` | `letta/agents/letta_agent.py:174-211`, `:562-880` |
| Voice step loop | `for _ in range(max_steps)` rebuilds memory each iter, calls OpenAI streaming + `_handle_ai_response` | `letta/agents/voice_agent.py:115-199` |
| VoiceSleeptimeAgent | Wraps legacy `LettaAgent._step()` for fixed-state voice flows | `letta/agents/voice_sleeptime_agent.py:70-110` |
| Batch (asynchronous) loop | `step_until_request()` (build batch, send), `resume_step_after_request()` (poll, exec tools, repeat) | `letta/agents/letta_agent_batch.py:126-270` |
| Batch scheduler | `AsyncIOScheduler` with PG `pg_try_advisory_lock` leader election polls Anthropic batches | `letta/jobs/scheduler.py:1-228`, `letta/jobs/llm_batch_job_polling.py:170-247` |
| Sleeptime multi-agent (V3) | Extends `LettaAgentV2.step()` to fan out background "sleeptime" agent runs after foreground response | `letta/groups/sleeptime_multi_agent_v3.py:42-79` |
| Sleeptime multi-agent (V4) | Same shape but extends `LettaAgentV3`; runs participants via `safe_create_task` and tracks run_ids | `letta/groups/sleeptime_multi_agent_v4.py:44-82`, `:131-167` |
| Streaming service | Wraps `agent_loop.stream()` with SSE keepalive, error-aware stream, Redis-backed resumability | `letta/services/streaming_service.py:180-928` |
| Concurrency control | `acquire_conversation_lock(redis)` keyed by `conversation_id` or `agent_id`; OTID-based request deduplication | `letta/data_sources/redis_client.py:194-247`, `letta/services/streaming_service.py:269-329` |
| Tool execution factory | `ToolExecutorFactory._executor_map` dispatches by `ToolType` (CORE, BUILTIN, FILES, MCP, SANDBOX, MULTI_AGENT) | `letta/services/tool_executor/tool_execution_manager.py:32-66` |
| LLM call indirection | `LettaLLMAdapter` / `LettaLLMRequestAdapter` / `LettaLLMStreamAdapter` / `SimpleLLMRequestAdapter` / `SimpleLLMStreamAdapter` / `SGLangNativeAdapter` | `letta/adapters/*.py` |
| Provider fallback | `LLMRouterClient.resolve_auto_mode_config` + per-handle `record_failure`/`record_success` circuit breaker | `letta/agents/letta_agent_v3.py:1042-1193` |
| HTTP entry | `POST /v1/agents/{agent_id}/messages` calls `agent_loop.step()` or routes to streaming | `letta/server/rest_api/routers/v1/agents.py:1662-1826` |
| HTTP entry (conversation) | `POST /v1/conversations/{id}/messages` | `letta/server/rest_api/routers/v1/conversations.py:300-376`, `:483-547` |
| Async background task | `_process_message_background()` via `safe_create_shielded_task(...)` returns Run object | `letta/server/rest_api/routers/v1/agents.py:2081-2284` |
| Lettuce offload | `LettuceClient.step(...)` returns run id (no-op base) — async offload target | `letta/services/lettuce/lettuce_client_base.py:9-87` |
| Tool rule solver | `ToolRulesSolver` evaluates terminal/continue/child/init rules to set `continue_stepping` | `letta/helpers/tool_rule_solver.py` (referenced from `_decide_continuation`) |

## Answers to Dimension Questions

1. **What is the primary execution model?**
   A bounded **step loop** (classic agentic ReAct-style cycle). Outer driver is `for i in range(max_steps)` in `step()`/`stream()` (`letta/agents/letta_agent_v3.py:328`, `letta/agents/letta_agent_v2.py:233`); inner body is `_step()` which performs one LLM round-trip + tool execution + persistence per iteration. A central `should_continue` boolean and `stop_reason` enum value, set by `_decide_continuation()` and `_handle_ai_response()`, determine whether to break out of the loop.

2. **Is it explicit or emergent?**
   **Explicit**. The shape is documented in:
   - `BaseAgentV2`/`LettaAgentV2` docstrings (`letta/agents/base_agent_v2.py:18-22`, `letta/agents/letta_agent_v2.py:77-84`),
   - The `StepProgression` enum (`letta/schemas/step.py:68-74`),
   - The `AgentLoop` factory with named branches (`letta/agents/agent_loop.py:19-62`),
   - Named checkpoint helpers `_step_checkpoint_start/llm_request_start/llm_request_finish/finish` (`letta/agents/letta_agent_v2.py:940-1034`).
   However, the **continuation semantics** are emergent — `should_continue` is set in at least 6 places inside `_step()` (LLM errors, no tool call, invalid tool call, cancellation, etc.) and again in `_decide_continuation()`.

3. **Does the model match the product shape?**
   **Yes for single-turn agents, mixed for the multi-agent/sleeptime cases.** A single Letta user message → step loop fits naturally. But the product also exposes background "sleeptime" agents (`SleeptimeMultiAgentV3/V4` extending `LettaAgentV2/V3`), voice realtime streaming (`VoiceAgent.step_stream`), Anthropic batch mode (`LettaAgentBatch.step_until_request`/`resume_step_after_request`), and Lettuce offload — each defines its own model layered on top of the step loop, which makes the overall execution model a hybrid rather than a single shape.

4. **Is the model easy to explain to a new contributor?**
   **Partially**. The entrypoint `AgentLoop.load()` is straightforward to find. Reading `letta/agents/letta_agent_v3.py:222-441` shows the canonical outer loop quickly. However, contributors must navigate:
   - Three parallel agent classes (`letta_agent.py`, `letta_agent_v2.py`, `letta_agent_v3.py`) that share ~80% of the shape,
   - Two base-class interfaces (`BaseAgent`, `BaseAgentV2`),
   - The `BaseAgentV2.step()` signature has 13 parameters (`letta/agents/base_agent_v2.py:51-64`),
   - The `StreamingService` (`letta/services/streaming_service.py`) wraps the loop with ~700 lines of keepalive, cancellation, error, and Redis logic before the loop is reached.
   The shape is recoverable but the surface area is wide.

5. **Does the system mix models cleanly or accidentally?**
   **Accidentally**. The same `step()` returns a `LettaResponse` for both blocking and the inline streaming call (`letta/agents/letta_agent_v2.py:288`, `letta/agents/letta_agent_v3.py:426`). `stream()` and `step()` are mirror implementations, not unified. The `agent_loop.py` factory has four branches that all do basically the same thing (`return LettaAgentV3(...)`, `return SleeptimeMultiAgentV4(...)`, `return LettaAgentV2(...)`, `return SleeptimeMultiAgentV3(...)`). Sleeptime fanning-out is implemented by overriding `step()`/`stream()` and calling `super().step(...)` then `await self.run_sleeptime_agents(...)` (`letta/groups/sleeptime_multi_agent_v4.py:65-82`), which works but couples the foreground loop to background task spawning without a clean orchestration boundary. The Batch mode uses a completely different shape (`step_until_request`/`resume_step_after_request`) and is reachable only through a special HTTP path (`POST /v1/messages/batch`, `letta/server/rest_api/routers/v1/messages.py:147`).

## Architectural Decisions

- **Adapter pattern for LLM transport.** `LettaLLMAdapter` (`letta/adapters/letta_llm_adapter.py`) wraps blocking vs streaming vs SGLang-native into one interface (`letta/agents/letta_agent_v3.py:510-550`). The same `_step()` body works for blocking, token-streaming, and RL-training backends.
- **Step-level checkpoint machine.** `StepProgression` is used as a numeric marker inside `finally:` to decide which kind of error-recovery to run (`letta/agents/letta_agent_v3.py:1545-1586`). Clever but unusual — relies on integer ordering of an enum.
- **Credit-check parallelism.** `safe_create_task_with_return(self._check_credits())` is fired at end of each step and awaited at start of the next (`letta/agents/letta_agent_v3.py:390`, `:574`). This hides billing latency inside LLM round-trip overhead — a good latency-hiding choice.
- **Cancellation as stop_reason.** A `cancelled` `StopReasonType` is propagated through `letta/agents/letta_agent_v3.py:1032-1034`, `:359` and surfaces at the streaming boundary (`letta/services/streaming_service.py:737-747`) without aborting the underlying request.
- **Sleeptime as override, not orchestration layer.** Sleeptime is `super().step(...)` then `await self.run_sleeptime_agents(...)` (`letta/groups/sleeptime_multi_agent_v4.py:65-82`). The fan-out is fire-and-forget (`safe_create_task`) with the foreground run returning immediately.
- **Background execution via Lettuce.** `LettuceClient.step(...)` is a stub interface (`letta/services/lettuce/lettuce_client_base.py:60-87`) for offloading the agent loop to a remote service; called from `send_message_async` (`letta/server/rest_api/routers/v1/agents.py:2252-2263`).
- **Concurrency gates via Redis.** A lock keyed on `conversation_id` (or `agent_id` if no conversation) prevents concurrent execution per agent (`letta/services/streaming_service.py:269-329`); duplicate OTIDs short-circuit to the existing stream.

## Notable Patterns

- **Two-tier for-loop with credit pre-fetch.** Outer loop ticks at `max_steps`; per-step body does tool execution + persistence; inter-step credit check is concurrent with LLM call start. (`letta/agents/letta_agent_v3.py:328-395`)
- **LLM-call retry inside `_step()`.** `for llm_request_attempt in range(summarizer_settings.max_summarizer_retries + 1)` retries on `ContextWindowExceededError` after summarization (`letta/agents/letta_agent_v2.py:518-573`, `letta/agents/letta_agent_v3.py:1093-1216`).
- **Auto-mode router with circuit breaker.** `LLMRouterClient.resolve_auto_mode_config` selects provider; failures record into `record_failure(handle)` and switch to fallback (`letta/agents/letta_agent_v3.py:1042-1193`).
- **Tools dispatched by ToolType factory.** `ToolExecutorFactory._executor_map` selects Core / Builtin / Files / MCP / Sandbox / Multi-agent executor per call (`letta/services/tool_executor/tool_execution_manager.py:35-43`).
- **Run/job persistence decoupled from loop.** `RunManager` and `StepManager` are managers called from inside the loop, not part of the loop structure (`letta/services/run_manager.py`, `letta/services/step_manager.py`).
- **Streaming is _additive_ to step.** `stream()` is essentially `step()` with chunks yielded as they're produced (`letta/agents/letta_agent_v3.py:443-717`); blocking-vs-streaming is a per-call argument rather than a different execution model.

## Tradeoffs

- **Three parallel step-loop implementations.** Legacy `LettaAgent` (`letta_agent.py`, 1983 lines), current `LettaAgentV2` (`letta_agent_v2.py`, 1487 lines), simplified `LettaAgentV3` (`letta_agent_v3.py`, 2134 lines). All three have nearly identical shape (`step()`/`stream()`/`_step()` with `for i in range(max_steps)`), but they diverge in tool rule handling, retry strategies, and approval flow. Maintenance cost is high and the choice between them is opaque to callers (decided by `agent_type` in `agent_loop.py`).
- **Base-class duplication.** `BaseAgent` (`letta/agents/base_agent.py`) and `BaseAgentV2` (`letta/agents/base_agent_v2.py`) define different `step()`/`stream()` signatures; `LettaAgentV2` extends V2 base, but `VoiceAgent`/`LettaAgentBatch`/`EphemeralAgent`/`SleeptimeMultiAgentV2` all extend the V1 base.
- **StepProgression as ordered int.** Using an enum's integer ordering as a control-flow threshold (`if step_progression < StepProgression.STEP_LOGGED`) couples error-handling logic to enum declaration order (`letta/agents/letta_agent_v3.py:1555`).
- **Inlined retry loop.** `for llm_request_attempt in range(summarizer_settings.max_summarizer_retries + 1)` is inlined in `_step()` rather than lifted into a generic retry helper. Same pattern duplicated in `LettaAgentV2._step` and `LettaAgentV3._step`.
- **Sleeptime fanning-out hides in subclass.** Background task spawning lives in the subclass's `step()` override rather than a separate orchestrator. A contributor tracing one agent's runtime behavior must read `SleeptimeMultiAgentV4.step()` and `_participant_agent_step()` to know what runs.
- **Voice loop diverges.** `VoiceAgent.step_stream` (`letta/agents/voice_agent.py:118-199`) does not go through `_step()`; it uses OpenAI streaming primitives directly with its own memory rebuild per iteration.
- **Max-steps enforcement is implicit on `max_steps - 1`.** `_step()` receives `remaining_turns` and treats the last step as a "final step" (`letta/agents/letta_agent_v2.py:608`), but the `for i in range(max_steps)` driver still sets `stop_reason=max_steps` only at end of loop, not mid-step (`letta/agents/letta_agent_v3.py:394-395`).

## Failure Modes / Edge Cases

- **`max_steps` hit without `stop_reason` set.** Inside the V3 loop, `stop_reason=max_steps` is only set on the last iteration (`letta/agents/letta_agent_v3.py:394-395`). If `_step()` raises before reaching that line, the loop ends with `stop_reason=None`; the post-loop fallback `if self.stop_reason is None: self.stop_reason = LettaStopReason(stop_reason=StopReasonType.end_turn.value)` (`:412-413`) **misclassifies failures as `end_turn`**.
- **Concurrent requests to same agent undefined.** Docstring admits it (`letta/server/rest_api/routers/v1/agents.py:1672-1675`). The Redis lock mitigates for streaming with `conversation_id`, but a POST without `conversation_id` falls back to `agent_id` only when `should_lock=True`.
- **`_step()` requires `run_id` when `enforce_run_id_set=True`.** `assert run_id is not None` (`:930-931`); a non-run caller fails the assertion rather than degrading gracefully.
- **StepProgression branches.** Recovery paths depend on which checkpoint last succeeded (`letta/agents/letta_agent_v3.py:1546-1586`); a bug where `step_progression` is incorrectly assigned can skip persistence, leaving orphaned `Step` rows.
- **Cancellation race.** `sleeptime_multi_agent_v4.stream()` issues background tasks in a `finally:` block (`letta/groups/sleeptime_multi_agent_v4.py:126-129`); the comment notes "stream is throwing a GeneratorExit even though... client is getting the whole stream" — known reliability bug with `GeneratorExit` handling.
- **`Should_continue` set inside retry.** If `ContextWindowExceededError` retries succeed, the loop still returns `should_continue=False` from the original `_handle_ai_response` path (`letta/agents/letta_agent_v2.py:593-614`); the outer loop will then break on the next iteration regardless of tool-rule output.
- **Approval response handling.** If an approval arrives with empty `tool_calls`/`tool_call_denials`/`tool_returns`, `_step()` returns early with `StopReasonType.invalid_tool_call` and `should_continue=False` (`letta/agents/letta_agent_v3.py:1009-1017`); no retry path exists for malformed approval payloads.
- **`AgentLoop.load()` fallback to base class.** When `enable_sleeptime=True` but `multi_agent_group is None`, agent falls back to plain `LettaAgentV3`/`V2` and logs a warning (`letta/agents/agent_loop.py:22-34`, `:45-57`) — operational telemetry missed.

## Future Considerations

- **Convergence on V3.** V2 is still the default for non-letia-v1 agents (`letta/agents/agent_loop.py:60`), and V1 (`LettaAgent`) is still reachable for voice_sleeptime agents. A future consolidation would route all paths through V3 and delete the V1/V2 step-loop duplication.
- **Unified streaming entrypoint.** `step()` and `stream()` are nearly identical bodies with chunk-yielding semantics added. A single driver that always yields chunks (and `step()` simply collects them) would remove the duplication that produced bugs like V3's `if i == max_steps - 1 and self.stop_reason is None` (`letta/agents/letta_agent_v3.py:394-395`).
- **Lift retry/circuit-breaker out of `_step()`.** Both `max_summarizer_retries` retry and `LLMRouterClient.record_failure`/fallback are inlined in `_step()`. A `with retry_or_fallback(...):` wrapper would shorten `_step()` and make it readable.
- **Background task orchestration.** Sleeptime fan-out via subclass `super().step()` then `run_sleeptime_agents()` is fragile. An explicit `Orchestrator` that subscribes the foreground run completion and spawns follow-up runs would decouple the loop from the multi-agent shape.
- **StopReason classification on exception.** The post-loop `if self.stop_reason is None: ...` defaults to `end_turn` which conflates "succeeded with no tool call" with "exception before stop_reason was set". An explicit `StopReasonType.unknown` default would be safer.
- **Agent-loop API stability.** `BaseAgentV2.step()` already takes 13 parameters (`letta/agents/base_agent_v2.py:51-64`); a `Request` Pydantic model would be more maintainable.

## Questions / Gaps

- Is there documentation outside the source (README, design docs) explaining the choice to maintain two parallel base classes (`BaseAgent` vs `BaseAgentV2`)? No README-level evidence found in `README.md` or `*.md` files in the source root.
- Why does `LettaAgentV2` still ship as the default for non-V1 agents (`letta/agents/agent_loop.py:60`) when `LettaAgentV3` exists with a "stripped down / simplified" goal (`letta/agents/letta_agent_v3.py:101-110`)? No design note found.
- How does `LettuceClient.step` interact with the foreground agent loop when both run concurrently for the same agent? No evidence in source beyond the stub interface (`letta/services/lettuce/lettuce_client_base.py:60-87`).
- What guarantees does `AgentLoop.load()` give about the returned type's conformance to `BaseAgentV2`? The factory's return type annotation is `BaseAgentV2` but `SleeptimeMultiAgentV4` extends `LettaAgentV3`, not `BaseAgentV2` directly (`letta/agents/agent_loop.py:19`).
- The `should_continue_map` (`letta/agents/letta_agent_batch.py:66`) is a per-agent continuation flag set by `LettaAgentBatch` — but this never feeds into the singleton loop. The two models (singleton step loop vs batched map-of-continuations) have no shared abstraction.

---

Generated by `01.01-execution-model-taxonomy` against `letta`.
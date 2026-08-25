# Source Analysis: crewai

## Dimension 13.02 — Retry, Fallback, and Degraded Mode

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (monorepo: `lib/crewai`, `lib/crewai-tools`, `lib/crewai-files`, `lib/cli`; LiteLLM + native provider SDKs) |
| Analyzed | 2026-08-25 |

All citations below are relative to the source root `studies/agent-harness-study/sources/crewai/`.

## Summary

CrewAI implements retries as several independent, per-subsystem loops rather than one shared policy: agent-level task retries (`max_retry_limit`, default 2), guardrail retries (`guardrail_max_retries`, default 3), tool-parse retries (3 attempts), MCP operation retries with exponential backoff and error classification, A2A auth/streaming retries with backoff, and HTTP-level retries delegated to provider SDKs (`max_retries=2` on native OpenAI/Azure/Anthropic clients; adaptive boto3 retries for Bedrock). Backoff is inconsistent: exponential (`2**attempt`) exists in the MCP, A2A, LanceDB, and Brave Search paths, but the core agent/guardrail/tool retry loops re-enter immediately with no delay.

Fallback models/providers do not exist as a first-class concept. There is no `fallback_llm` or backup-provider chain anywhere in the codebase; a single-provider outage is only absorbed by SDK-level retries plus the agent's `max_retry_limit`. What does exist are targeted fallbacks: chat-completions→Responses API failover, `reasoning_effort="none"` retry-on-reject, text-based tool-calling fallback prompts, streaming→non-streaming fallback, an environment/default model fallback when no LLM is configured, and curated offline fallbacks in the CLI model catalog.

Degraded mode is handled at several points: context-window overflow degrades to message summarization by default (`respect_context_window=True`), tracing/telemetry failures never block execution, tool failures default to a warn-and-continue policy with structured `ToolFailure(retryable=...)` metadata, memory saves degrade to synchronous writes when the async pool is down, and Bedrock falls back on empty streams. No circuit breaker implementation exists. Retry counters (`_times_executed`, `_guardrail_retry_count`) are in-memory private attributes; retry state is not persisted, though flows/crews have separate checkpointing and flow-resume persistence that enable recovery after a failure.

## Rating

**6 / 10** — Present but inconsistent. Retries are configurable at multiple layers with sensible defaults and good edge-case tests, and degraded paths exist for context overflow, tracing, tools, and memory. But there is no fallback provider chain (so a sustained provider outage fails all requests after local retries), no circuit breakers, backoff is missing from the central agent/task/tool retry loops, retry implementations are fragmented across ~10 ad-hoc sites, and retry state is not durable.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Agent task retry config | `max_retry_limit: int = Field(default=2, ...)` — "Maximum number of retries for an agent to execute a task when an error occurs" | `lib/crewai/src/crewai/agent/core.py:255-258` |
| Agent retry loop | `_check_execution_error` increments `_times_executed`, raises immediately if `e.__class__.__module__.startswith("litellm")` or passthrough exception; `_handle_execution_error` re-invokes `execute_task` (no backoff) | `lib/crewai/src/crewai/agent/core.py:721-747`, `749-789` |
| Retry counter not persisted | `_times_executed: int = PrivateAttr(default=0)` — in-memory only, reset per process | `lib/crewai/src/crewai/agent/core.py:208` |
| Passthrough exceptions skip retries | `_passthrough_exceptions = (ToolExecutionFailedError,)` — tool-policy aborts are never retried | `lib/crewai/src/crewai/agent/core.py:140-141` |
| Guardrail retry config (task) | `guardrail_max_retries: int = Field(default=3, ...)`; deprecated `max_retries` migrated via `handle_max_retries_deprecation` | `lib/crewai/src/crewai/task.py:275-282`, `574-582` |
| Guardrail retry loop (task) | `max_attempts = self.guardrail_max_retries + 1`; aborts after limit with "Task failed {guardrail} validation after N retries" | `lib/crewai/src/crewai/task.py:1337-1383`, sync+async `1458-1504` |
| Guardrail retry loop (agent) | `_guardrail_retry_count >= guardrail_max_retries` → raise; else increment and log "Guardrail failed. Retrying (n/N)" | `lib/crewai/src/crewai/lite_agent.py:752-760`, counter at `309` |
| Tool usage retries | `_max_parsing_attempts: int = 3` (2 for some agents); on exception under the cap `should_retry = True` → recursive `ause()` re-entry; over the cap returns `ToolUsageError` message and moves on | `lib/crewai/src/crewai/tools/tool_usage.py:110`, `134`, `451-479`, `497-501` |
| Tool failure policy | `IGNORE`/`WARN` (default)/`RAISE` modes; `ToolFailure.retryable` field ("Whether retrying the same call could plausibly succeed"); `failure_from_exception(..., retryable=False)` | `lib/crewai/src/crewai/tools/tool_failure.py:59-68`, `95-98`, `165-174` |
| MCP client retries | `MCP_MAX_RETRIES = 3`, `MCP_TOOL_EXECUTION_TIMEOUT = 30`; `_retry_operation` uses `wait_time = 2**attempt` exponential backoff and classifies errors: authentication/unauthorized and "not found" fail fast; everything else retryable | `lib/crewai/src/crewai/mcp/client.py:45-47`, `688-739` |
| MCP discovery retries | `_retry_mcp_discovery` — exponential backoff over `MCP_MAX_RETRIES` attempts | `lib/crewai/src/crewai/mcp/tool_resolver.py:37-39`, `537-560` |
| MCP tool wrapper | `_retry_with_exponential_backoff` wrapping timeout-bounded execution (`wait_time = 2**attempt`) | `lib/crewai/src/crewai/tools/mcp_tool_wrapper.py:86-110`, `156-159` |
| A2A auth retry | `retry_on_401(request_fn, max_retries=3)` refreshes token then retries with `backoff_time = 2**attempt` | `lib/crewai/src/crewai/a2a/auth/utils.py:194-244` |
| A2A streaming resubscribe | Resubscribe loop `MAX_RESUBSCRIBE_ATTEMPTS` with `backoff = RESUBSCRIBE_BACKOFF_BASE * (2**attempt)` | `lib/crewai/src/crewai/a2a/updates/streaming/handler.py:135-141` |
| A2A rate-limit semantics | `RateLimitExceededError` carries `retry_after`; `is_retryable_error(code)` whitelist (408/409/429/5xx) | `lib/crewai/src/crewai/a2a/errors.py:352-364`, `464-478` |
| Native provider HTTP retries | OpenAI/Azure/Anthropic completions expose `max_retries: int = 2` forwarded to the provider SDK config (only when non-default) | `lib/crewai/src/crewai/llms/providers/openai/completion.py:223`, `369-372`, `415-416`; `azure/completion.py:82`, `215-218`; `anthropic/completion.py:221`, `300-301` |
| Bedrock adaptive retries | boto3 Config `retries={"max_attempts": 3, "mode": "adaptive"}` (client-side adaptive rate limiting/retry mode) | `lib/crewai/src/crewai/llms/providers/bedrock/completion.py:316` |
| Unsupported-param degradation | On "'stop' unsupported" error: append to `additional_drop_params`, log "Retrying LLM call without the unsupported 'stop'", recurse once; global `litellm.drop_params = True` set at init | `lib/crewai/src/crewai/llm.py:1909-1946`, `726-733` |
| Chat-completions → Responses fallback | `_call_completions` catches Responses-only 404, remembers model, retries via `_call_responses` | `lib/crewai/src/crewai/llms/providers/openai/completion.py:497-564` |
| Bounded protocol retries | `_rejects_reasoning_effort_with_tools(cause)` → single retry with `reasoning_effort="none"`; tests assert it neither retries forever nor retries unrelated 400s | `lib/crewai/src/crewai/llms/providers/openai/completion.py:538-547`; `lib/crewai/tests/llms/openai/test_tools_reasoning_effort_retry.py:157-173` |
| Text tool-calling fallback | `build_text_tool_calling_fallback_message(...)` injected into conversation when native tool calling unsupported/failing | `lib/crewai/src/crewai/utilities/agent_utils.py:1491`; used in `lib/crewai/src/crewai/agents/crew_agent_executor.py:470-479` |
| Streaming fallback | `test_streaming_fallback_to_non_streaming` documents stream-error → non-streaming call fallback contract | `lib/crewai/tests/utilities/test_events.py:1434-1482` |
| Default-model fallback | When `llm_value is None`: env `MODEL`/`MODEL_NAME`/`OPENAI_MODEL_NAME` else `DEFAULT_LLM_MODEL = "gpt-4.1-mini"` | `lib/crewai/src/crewai/utilities/llm_utils.py:97-108`, `194-200`; `lib/crewai/src/crewai/constants.py:348` |
| Context-window degradation | `respect_context_window=True` (default): `handle_context_length` summarizes messages instead of failing; opted-out path raises `SystemExit`; executor decides between summarize/abort | `lib/crewai/src/crewai/agent/core.py:251-254`; `lib/crewai/src/crewai/utilities/agent_utils.py:795-832`; `lib/crewai/src/crewai/llm.py:1909-1913` |
| Telemetry degradation | Trace batch init: 1 retry on 5xx only, then "Continuing without tracing"; ephemeral fallback on 401/403; tests confirm no retry on exceptions/4xx | `lib/crewai/src/crewai/events/listeners/tracing/trace_batch_manager.py:175-209`; `lib/crewai/tests/tracing/test_tracing.py:1242-1268` |
| Memory write degradation | If async save pool is shut down, "runs synchronously as a fallback so late saves still succeed" | `lib/crewai/src/crewai/memory/unified_memory.py:302-311` |
| Storage commit retries | LanceDB optimistic-concurrency retries: 5 attempts, `_RETRY_BASE_DELAY = 0.2s` doubling per attempt (≈6.2s max) | `lib/crewai/src/crewai/memory/storage/lancedb_storage.py:34-39`, `129-150` |
| Tool-level HTTP retry w/ Retry-After | Brave Search: `_is_retryable` (429 rate-limited yes, quota exhaustion no; 5xx yes); `_retry_delay` prefers server `Retry-After` header else `2**attempt`; 3 attempts | `lib/crewai-tools/src/crewai_tools/tools/brave_search_tool/base.py:51-79`, `180-231` |
| Converter retries | Structured-output conversion loops up to `max_attempts` before raising `ConverterError` | `lib/crewai/src/crewai/utilities/converter.py:106-176`; test `lib/crewai/tests/utilities/test_converter.py:490-514` |
| Circuit breaker absence | Searches for `breaker|half.open|fail.closed` across `lib/**` match only CLI skill-sync fail-closed semantics (unrelated to providers); no shared open/half-open/closed state machine exists | search across `lib/` (pattern `breaker|half.open|degraded mode`) — no provider-path hits |
| Fallback provider absence | Searches for `fallback_llm|fallback_model|backup_provider` return no matches in `lib/` | search across `lib/` — no hits |
| Checkpointing (durable recovery) | `CheckpointConfig`: auto-checkpoints on events (default `["task_completed"]`), JSON/SQLite providers, `restore_from` resumes crew/flow | `lib/crewai/src/crewai/state/checkpoint_config.py:160-234`; failure-event vocabulary at `14-143` |
| Flow resume persistence | `SQLiteFlowPersistence.save_pending_feedback/load_state` supports pause/resume ("Later, resume with feedback … flow.resume(...)") | `lib/crewai/src/crewai/flow/persistence/sqlite.py:24-50`, `205-281` |
| Kickoff output replay store | `latest_kickoff_task_outputs.db` persists per-task outputs/inputs for replay (not retry counters) | `lib/crewai/src/crewai/memory/storage/kickoff_task_outputs_storage.py:19-58` |
| Retry event-scope tests | `test_agent_retries_close_the_scope_of_every_attempt` (2 starts, paired error events, 1 task_failed) and success-path twin | `lib/crewai/tests/utilities/test_events.py:376-469` |
| Guardrail retry accounting tests | `test_guardrail_retry_preserves_earlier_failures`, `test_retry_limit_does_not_swallow_the_abort`, `test_guardrail_retry_usage_includes_all_attempts` | `lib/crewai/tests/tools/test_tool_failure.py:689-811`; `lib/crewai/tests/agents/test_lite_agent.py:1228` |
| Documented defaults | Agents doc table: `max_retry_limit` default 2; Tasks doc: guardrail failures retried up to `guardrail_max_retries` | `docs/edge/en/concepts/agents.mdx:60`; `docs/edge/en/concepts/tasks.mdx:67`, `491-501` |

## Answers to Dimension Questions

**1. Are retries configurable?**
Yes, at four distinct layers, each configured separately: agent task execution (`Agent.max_retry_limit`, default 2, `lib/crewai/src/crewai/agent/core.py:255`), guardrails (`Task.guardrail_max_retries` / `Agent.guardrail_max_retries`, default 3, `lib/crewai/src/crewai/task.py:279`, `agent/core.py:326`), provider HTTP transport (`LLM max_retries` forwarded to OpenAI/Azure/Anthropic SDKs, default 2, e.g. `llms/providers/openai/completion.py:223`; documented in `docs/edge/en/concepts/llms.mdx:178`), and MCP operations (`MCPClient.max_retries`, default 3, `mcp/client.py:81`). However, configurability is fragmented: backoff timing (where it exists at all, always `2**attempt`) is hard-coded, not configurable, and there is no single retry-policy object spanning layers.

**2. Are fallback providers available?**
No evidence found for fallback models or backup providers. Searches for `fallback_llm`, `fallback_model`, and `backup_provider` across the monorepo return zero provider-failover results. The nearest mechanisms are: (a) LiteLLM used as a *fallback implementation* for models without a native CrewAI provider (`lib/crewai/src/crewai/llm.py:2391-2516` comments: "litellm fallback path"), (b) OpenAI chat-completions→Responses API endpoint failover (`llms/providers/openai/completion.py:549-564`), and (c) a default model chosen from env vars when none is supplied (`utilities/llm_utils.py:103-108`). None of these reroute traffic to a second provider during an outage.

**3. Does the system degrade gracefully?**
Largely yes within a run. Context-window overflow summarizes instead of aborting by default (`utilities/agent_utils.py:815-823`); unsupported parameters are dropped and the call retried (`llm.py:1914-1946`); tracing/telemetry failures log and continue without blocking (`trace_batch_manager.py:193-205`); tool failures default to WARN-and-continue with structured records (`tools/tool_failure.py:63-68`); memory saves fall back to synchronous execution (`memory/unified_memory.py:302-311`); Bedrock substitutes content on empty streams (`bedrock/completion.py:1153`). The notable non-graceful case: with `respect_context_window=False`, context overflow raises `SystemExit` (`agent_utils.py:830-832`).

**4. Are circuit breakers used to prevent cascading failure?**
No clear evidence found. No open/half-open/closed state machine, failure-rate tracker, or cooldown gate exists in the codebase (searched `breaker`, `half-open`, `fail.closed` patterns). Cascading failure is limited indirectly: bounded attempt counters stop runaway loops (agent `max_retry_limit`; tool `_run_attempts > _max_parsing_attempts` at `tool_usage.py:455`; tool `max_usage_count`; per-instance `requests_per_second` throttling in `brave_search_tool/base.py:133-138`), and Bedrock's adaptive retry mode provides client-side throttling awareness (`bedrock/completion.py:316`). These are per-call guards, not cross-call breakers — repeated calls against a dead endpoint will each pay the full retry cost.

## Architectural Decisions

- **Layered ownership of retries.** Each layer owns its own retry concern: SDK owns transport retries (`max_retries=2` passed through, `openai/completion.py:223`), the framework owns semantic retries (agent/guardrail/tool loops), and infrastructure adapters own their own (MCP `mcp/client.py:688`, LanceDB `lancedb_storage.py:129`). There is no central orchestrator.
- **Fail fast on non-transient errors.** The MCP client classifies auth and not-found errors as terminal and raises immediately while treating everything else as retryable (`mcp/client.py:719-727`); A2A defines an explicit retryable status-code whitelist (`a2a/errors.py:464-478`); Brave Search excludes quota exhaustion from retries because "retrying will never succeed until the billing period resets" (`brave_search_tool/base.py:51-64`). This is the most mature error-classification thinking in the repo.
- **litellm errors bypass framework retries.** Any exception whose module starts with `"litellm"` is re-raised without consuming an agent retry (`agent/core.py:743-744`) — presumably to avoid double-retrying what litellm already retried internally, at the cost of inconsistency with the native-provider paths.
- **Policy-driven abort vs. continue for tools.** `ToolFailurePolicy` (IGNORE/WARN/RAISE, `tools/tool_failure.py:59-68`) makes the degrade-or-abort decision explicit and user-selectable, with `ToolExecutionFailedError` registered as a passthrough that even skips agent retries (`agent/core.py:140-141`).
- **Durability delegated to checkpointing, not retries.** Long-running recovery is handled by event-triggered checkpoints with restore (`state/checkpoint_config.py:160-204`) and flow persistence with resume (`flow/persistence/sqlite.py:24-50`) rather than persisted retry journals.

## Notable Patterns

- **Exponential backoff as a copy-paste idiom:** `wait_time = 2**attempt` appears independently in `mcp/client.py:714`, `tools/mcp_tool_wrapper.py:110`, `a2a/auth/utils.py:243`, and `a2a/updates/streaming/handler.py:137` — same formula, five implementations, no jitter.
- **Retry-After respect:** the Brave Search tool prefers the server-supplied `Retry-After` header over computed backoff (`brave_search_tool/base.py:67-79`); A2A mirrors this with `RateLimitExceededError.retry_after` (`a2a/errors.py:356-364`). The LLM layer does not honor `Retry-After`.
- **Self-correcting parameter degradation:** instead of failing on unsupported features, CrewAI mutates its own request config (drop `stop`, `reasoning_effort="none", drop params via `litellm.drop_params`) and retries once (`llm.py:1914-1946`; `openai/completion.py:538-556`).
- **Event-scope hygiene around retries:** dedicated tests prove every retried attempt opens and closes its own `agent_execution_started` scope so telemetry/event consumers see consistent lifecycles (`tests/utilities/test_events.py:376-421`).
- **Negative caching for offline fallbacks (CLI):** failed catalog fetches serve a curated fallback but are negatively cached briefly so recovery isn't masked (`lib/cli/src/crewai_cli/model_catalog.py:55`, `623-644`).

## Tradeoffs

- **No fallback provider chain ⇒ outage sensitivity.** The dimension's guiding question — can the system survive a provider outage without failing all requests? — is answered *partially*: transient blips are absorbed (SDK retries × agent retries ≈ up to ~6 attempts), but a sustained outage of the configured provider fails every request. Users must build failover themselves.
- **Immediate re-entry retries risk hammering.** Agent, guardrail, and tool retry loops recurse instantly with zero delay (`agent/core.py:749-768`, `tool_usage.py:497-499`), which is ineffective against rate limits and adds load during incidents; only MCP/A2A paths sleep.
- **Fragmentation vs. flexibility.** Because each subsystem hand-rolls its loop, behaviors diverge (backoff presence, error classification, jitter, observability), making global reasoning about worst-case latency/attempts difficult; a shared retry abstraction would trade some per-layer nuance for consistency.
- **In-memory retry counters simplify but weaken recovery.** `_times_executed` resets on process restart (`agent/core.py:208`), so a crashed process restarts with a fresh retry budget; checkpoints compensate for crews/flows but not for the retry budget itself.
- **Silent degradation can mask bugs.** WARN-by-default tool failures and drop-param self-healing keep runs alive but can produce lower-quality outputs without hard signals unless consumers inspect `TaskOutput.tool_failures` / drop logs.

## Failure Modes / Edge Cases

- **Retry budget exhaustion:** guardrail loops abort with a descriptive error after `guardrail_max_retries` (`task.py:1376-1383`); tool parse failures beyond `_max_parsing_attempts` inject a formatted `ToolUsageError` string and move on (`tool_usage.py:455-469`) — the run continues with a degraded observation rather than aborting.
- **Double-retry avoidance:** litellm-originated errors skip agent retries entirely (`agent/core.py:743-744`); conversely, native OpenAI suppresses premature `call.failed` events for errors that `_call_completions` will retry itself (`openai/completion.py:2035-2038`, mirrored at `2463`).
- **Bounded protocol retries verified by tests:** reasoning-effort rejection retries exactly once, never on unrelated 400s, never forever (`tests/llms/openai/test_tools_reasoning_effort_retry.py:157-173`); genuine unknown-model errors are not mistaken for Responses-only 404s (`tests/llms/openai/test_responses_only_models.py:102`).
- **Abort-vs-degrade conflict:** `SystemExit` on opted-out context overflow (`agent_utils.py:830-832`) kills the whole process mid-run — harsh compared to the RAISE policy available for tools.
- **Quota vs. throttle confusion avoided:** Brave treats 429 RATE_LIMITED as retryable but QUOTA_LIMITED as terminal (`brave_search_tool/base.py:51-64`) — a distinction the LLM provider layer lacks (it relies on the SDK).
- **Tracing must never break execution:** init failures, 4xx, and exceptions all short-circuit to "Continuing without tracing" with `trace_batch_id = None` (`trace_batch_manager.py:178-205`).

## Future Considerations

- Introduce a configurable fallback chain (e.g., `fallback_llm=[...]` on `Agent`/`Crew`) so a provider outage can reroute; the existing native-provider registry (`llm.py:663-715`) plus the LiteLLM fallback path provide the plumbing.
- Add exponential backoff (ideally with jitter and `Retry-After` support) to the agent/guardrail/tool retry loops, or unify them behind a small shared retry helper modeled on `mcp/client.py:_retry_operation`.
- Add a lightweight circuit breaker (per provider/model key) fed by consecutive-failure counts to avoid paying full retry cost against dead endpoints; the event bus (`crewai_event_bus`) already emits `LLMCallFailedEvent` (`llm.py:1948-1956`) which could drive it.
- Persist retry budgets (or derive them from checkpoints) so resumed runs don't silently reset attempt counts.
- Honor `Retry-After` headers in the LLM provider layer, matching the behavior already present in the Brave/A2A paths.

## Questions / Gaps

- **Why do litellm-module exceptions bypass agent retries?** `agent/core.py:743-744` gives no rationale comment; whether this avoids double retries or simply punishes litellm users is undocumented (searched docs and code history boundaries; no evidence found).
- **Is streaming→non-streaming fallback actually implemented in the LLM call path, or only tested?** The test mocks `llm.call` itself (`tests/utilities/test_events.py:1449-1463`), so it documents intent more than mechanism; I did not find an explicit catch-and-retry-non-streaming branch in `llm.py`'s call path (searched `stream.*fallback`, `disable_streaming`). Treated as inferred intent, not implemented behavior.
- **No evidence found for retry metrics/observability:** beyond `AgentExecutionErrorEvent` and trace metadata `max_retry_limit` reporting (`telemetry/telemetry.py:371`, `475`), there is no per-attempt latency/backoff telemetry for the internal retry loops.
- The `HTTPTransportKwargs.retries` field (`llms/hooks/transport.py:44`) exposes httpx connection retries only for the interceptor transport; whether any default interceptor sets it could not be confirmed from configuration alone.

---

Generated by `Dimension 13.02: Retry, Fallback, and Degraded Mode` against `crewai`.

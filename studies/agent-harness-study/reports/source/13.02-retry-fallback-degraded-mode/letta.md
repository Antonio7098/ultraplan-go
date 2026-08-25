# Source Analysis: letta

## Dimension 13.02 — Retry, Fallback, and Degraded Mode

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI server, asyncio, SQLAlchemy/Postgres, Redis optional, multiple LLM provider SDKs) |
| Analyzed | 2026-08-25 |

> All citations below are workspace-relative to `studies/agent-harness-study/sources/letta/`.

## Summary

Letta implements resilience as several loosely-coupled layers rather than one unified retry framework:

1. **Request-level retries** are scattered across provider clients and infrastructure helpers, each with its own hand-rolled exponential-backoff loop. The legacy sync LLM path uses a `retry_with_exponential_backoff` decorator (`letta/llm_api/llm_api_tools.py:38-118`, applied at `letta/llm_api/llm_api_tools.py:122`) that retries only HTTP 429 with up to 20 hardcoded retries. The ChatGPT-OAuth client has its own 3-attempt loop with a curated `_RETRYABLE_ERRORS` tuple (`letta/llm_api/chatgpt_oauth_client.py:100-102,397-448`), and the Gemini client retries 5xx/MALFORMED_FUNCTION_CALL responses (`letta/llm_api/google_vertex_client.py:55,133-198`). Provider SDK-native retries are configurable for Anthropic/Gemini (`letta/settings.py:181,218`).
2. **Agent-loop retries** are semantic: the v3 step loop wraps each LLM request in a bounded retry driven by `summarizer_settings.max_summarizer_retries` (`letta/agents/letta_agent_v3.py:1093`), where context-window failures trigger message compaction and re-request (`letta/agents/letta_agent_v3.py:1218-1263`). The older loop retries empty/malformed responses with backoff (`letta/agent.py:354-425`).
3. **Fallback routing** is architected through an `LLMRoutingClient` service with auto-mode handles ("letta/auto"), per-step resolution, failure/success recording, and content-based rerouting (`letta/services/llm_router/__init__.py:1-18`, integrated in `letta/agents/letta_agent_v3.py:1050-1211`). However, the Redis-backed implementation that actually performs circuit-breaking and fallback selection is **not present in this repository**; the OSS base class is a stub of noops and `RuntimeError`s (`letta/services/llm_router/llm_router_client_base.py:14-97`), and the feature is disabled by default (`letta/settings.py:119-122`).
4. **Degraded mode** is implemented as a pod-level readiness subsystem: a shared readiness state machine (`letta/monitoring/readiness_state.py:8-84`), three load gates plus an event-loop watchdog that transition the pod to "degraded" after sustained overload (`letta/monitoring/load_gate.py:29-47`, `letta/monitoring/event_loop_watchdog.py:212-261`), and a `/ready` endpoint returning 503 when degraded (`letta/server/rest_api/routers/v1/health.py:15-45`). Every gate defaults OFF (`letta/settings.py:600-637`).

**Bottom line on the dimension's key question** (*can the system survive a provider outage without failing all requests?*): In the OSS code as shipped, no — a hard provider outage exhausts request-level retries and fails the step, because model fallback and circuit breaking require the missing Redis-backed router module and an opt-in flag. The scaffolding to survive it (fallback switch inside the step loop, breaker signals, billing-aware reroute bookkeeping) is fully wired in `letta_agent_v3.py`, so deployments with the enterprise router module get genuine outage survival.

## Rating

**6 / 10** — Present but inconsistent and partially unavailable.

Rationale:
- Retry logic is pervasive and mostly sound individually (exponential backoff + jitter, transient-error classification, telemetry events), but there are at least eight independent hand-rolled implementations with divergent knobs, several hardcoded constants (`max_retries=20` at `letta/llm_api/llm_api_tools.py:43`; `TPUF_MAX_RETRIES = 3` at `letta/helpers/tpuf_client.py:31`), and duplicated transient-error classifiers.
- Fallback/circuit-breaker has a clean explicit interface and full consumer-side integration, but the implementing module is absent from the source tree, making the flagship feature inert out-of-the-box (`letta/services/llm_router/__init__.py:6-12` silently falls back to a stub).
- Degraded mode is well-designed (stabilization windows, multi-source recovery, k8s drain semantics) but entirely opt-in via non-default flags (`letta/settings.py:607,613,619,623,627`).
- Dedicated tests exist only for the embeddings chunk-retry path; none cover the LLM retry decorator, fallback routing, circuit breaker, or readiness gating.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Generic LLM retry decorator (429-only, jittered exp. backoff, max_retries=20) | `retry_with_exponential_backoff`; emits `llm_retry_attempt` / `llm_max_retries_exceeded` OTEL events; raises `RateLimitExceededError` | `letta/llm_api/llm_api_tools.py:38-118`, `letta/errors.py:374-383` |
| Legacy sync entry point decorated with retry | `@retry_with_exponential_backoff` on `create()` | `letta/llm_api/llm_api_tools.py:121-122` |
| ChatGPT OAuth client retry policy | `MAX_RETRIES = 3`, `_RETRYABLE_ERRORS` (httpx read/write/connect/remote-protocol + LLMConnectionError); `wait = 2**attempt`; streaming refuses retry after partial yield (`has_yielded`) | `letta/llm_api/chatgpt_oauth_client.py:100-102`, `:397-448`, `:626-705` |
| Gemini retry policy | `MAX_RETRIES = model_settings.gemini_max_retries`; retries 503/500 and `FinishReason.MALFORMED_FUNCTION_CALL`, injecting corrective prompt text on retry | `letta/llm_api/google_vertex_client.py:55`, `:133-198`, `letta/settings.py:218` |
| SDK-configurable provider retries | `anthropic_max_retries: int = 3` passed into `anthropic.AsyncAnthropic`/`anthropic.Anthropic` constructors; same for Bedrock | `letta/settings.py:181`, `letta/llm_api/anthropic_client.py:440-461`, `letta/llm_api/bedrock_client.py:66-74` |
| Error taxonomy feeding retry decisions | `LLMRateLimitError`, `LLMServerError`, `LLMProviderOverloaded`, `LLMTimeoutError`, `LLMConnectionError`, `LLMEmptyResponseError`, `ContextWindowExceededError` | `letta/errors.py:259-314`, `:352-361` |
| Per-provider error mapping into taxonomy | `handle_llm_error` maps openai/httpx exceptions → `LLMTimeoutError`/`LLMConnectionError`/`LLMRateLimitError` etc.; Anthropic maps overloaded → `LLMProviderOverloaded` | `letta/llm_api/openai_client.py:1216-1281`, `letta/llm_api/anthropic_client.py:1096-1143` |
| Agent-loop semantic retry (v3) | `for llm_request_attempt in range(summarizer_settings.max_summarizer_retries + 1)`; `ContextWindowExceededError` triggers `compact()` then retry; `LLMEmptyResponseError`/`ValueError` are fatal; rate-limit/server/overloaded trigger fallback-or-fail | `letta/agents/letta_agent_v3.py:1093`, `:1177-1182`, `:1183-1213`, `:1217-1263` |
| Older agent loop retry (empty response) | `empty_response_retry_limit=3`, `backoff_factor=0.5`, `max_delay=10.0`; ValueError retried, all else raised | `letta/agent.py:306-308`, `:354-430` |
| Summarizer-driven retry loops (legacy architecture) | `max_summarization_retries` bounds `_build_and_request_from_llm_*`; `_handle_llm_error` repairs context between attempts | `letta/settings.py:82,96`, `letta/agents/letta_agent.py:1426-1476`, `:1492-1547`, `letta/agents/letta_agent_v2.py:518,563` |
| Fallback router interface (OSS stub) | Conditional import of Redis-backed `llm_router_client`; falls back to noop base; `record_failure`/`record_success` noop; `get_fallback_handle` returns None; `resolve_auto_mode_config` raises RuntimeError | `letta/services/llm_router/__init__.py:1-18`, `letta/services/llm_router/llm_router_client_base.py:14-97` |
| Auto-mode handles (fallback-capable model listing) | `auto_mode_enabled` default False; synthetic `letta/auto*` models injected into listing; placeholder LLMConfig for storage | `letta/settings.py:119-122`, `letta/services/provider_manager.py:941-959`, `:985-1004` |
| Step-time fallback integration | Resolve auto config → apply reroute rules → swap LLM client; persist resolved model for billing even on partial failure; on `LLMRateLimitError`/`LLMServerError`/`LLMProviderOverloaded`: record_failure, fetch fallback config, continue loop with fallback; record_success when fallback routes exist | `letta/agents/letta_agent_v3.py:1050-1087`, `:1171-1176`, `:1183-1211` |
| Circuit breaker (named) | Module docstring "circuit breaker and fallback support"; base-class docstrings: "Auto mode requires Redis for circuit breaker support" | `letta/services/llm_router/__init__.py:1`, `letta/services/llm_router/llm_router_client_base.py:36-40,60-64,80-82` |
| Degraded/readiness state machine | States {ready, warming, draining, degraded, manual_disable}; multi-source `mark_degraded`/`clear_degraded` (recover only when ALL sources clear); OTel gauge per state | `letta/monitoring/readiness_state.py:8-21`, `:63-84`, `:52-53` |
| Degradation gates & thresholds | `ReadinessSettings`: master kill switch, event-loop-lag gate (5000ms), fg/bg in-flight gates (10/15), admission-wait gate (300ms), stabilization windows (30s degrade / 15s recover) — all default OFF | `letta/settings.py:600-637` |
| Event-loop watchdog degradation | Heartbeat-lag measurement from separate thread; `_maybe_degrade_readiness` / `_maybe_recover_readiness`; started in app lifespan | `letta/monitoring/event_loop_watchdog.py:44-108`, `:212-261`, `letta/server/rest_api/app.py:181-195,256-261` |
| Load gates wiring | fg/bg counters around run execution; admission-wait sampled around conversation-lock acquisition | `letta/monitoring/load_gate.py:29-47`, `letta/services/streaming_service.py:297-328`, `:574-580`, `:814-816` |
| Readiness endpoint behavior | `/ready` returns 503 for {degraded, manual_disable, warming} and draining (configurable) when enforcement enabled; `/health` always 200 | `letta/server/rest_api/routers/v1/health.py:15-45` |
| DB connection retry | 3 attempts, delay doubling from 0.1s, then `LettaServiceUnavailableError` | `letta/server/db.py:84-116` |
| DB deadlock retry | `_DEADLOCK_MAX_RETRIES = 3`, PG 40P01 detection across create/update/delete paths, then `DatabaseDeadlockError` | `letta/orm/sqlalchemy_base.py:40-66`, `:591-784`, `:920,964-967` |
| Turbopuffer retry decorator | `async_retry_with_backoff` (3 retries, 1s initial, base 2, ≤10% jitter) applied to ~13 operations; `is_transient_error` classifies by exception type + message patterns | `letta/helpers/tpuf_client.py:31-34`, `:79-154`, `:37-76`, applications at `:399,490,641,790,1562-2003` |
| Pinecone retry | Exponential backoff with jitter, capped max delay, `PINECONE_RETRY_BACKOFF_FACTOR` | `letta/helpers/pinecone_utils.py:47-110` |
| Redis operation retry | `with_retry` decorator (3 attempts, 0.1s × 2^attempt) on get/set/streams; timeout metric recorded | `letta/data_sources/redis_client.py:127-152` |
| ClickHouse trace-write retry | Fire-and-forget background task, 3 retries, 1s×2^n backoff | `letta/services/llm_trace_writer.py:24-25`, `:117-163` |
| Duplicate-request recovery polling | Same-OTID lock contention: 3 polls at 250ms/500ms/1s to recover run mapping | `letta/services/streaming_service.py:304-318` |
| Idempotent-retry support state (persisted, TTL) | otid→run_id mapping stored in Redis to resolve 409 duplicates on client retry | `letta/data_sources/redis_client.py:250-289` |
| Embeddings adaptive retry | Failed batches split (reduce batch size → split text) and re-enqueued until minimum size | `letta/llm_api/openai_client.py:1085-1203` |
| Tests covering retry behavior | `test_openai_embedding_retry_logic`, `test_openai_embedding_minimum_chunk_failure`, `test_token_limit_retry_splits_batch`, `test_single_item_batch_no_retry` | `tests/test_embeddings.py:103-141,181-205`, `tests/test_file_processor.py:68-95,153+` |
| Watchdog validation script (manual, not pytest) | Root-level script asserting hang detection logging | `test_watchdog_hang.py:29-87` |

## Answers to Dimension Questions

**1. Are retries configurable?**
Partially. Configurable through env-driven settings: `anthropic_max_retries` (`letta/settings.py:181`), `gemini_max_retries` (`letta/settings.py:218`), `multi_agent_send_message_max_retries` (`letta/settings.py:350`), summarizer retry bounds (`letta/settings.py:82,96`), and SSE keepalive/timeout windows (`letta/settings.py:290-291,429-432`). But many core policies are hardcoded constants: the generic LLM retry decorator fixes `max_retries=20`, `exponential_base=2`, and 429-only scope (`letta/llm_api/llm_api_tools.py:40-46`); turbopuffer/pinecone/deadlock/db/trace-writer limits are module constants (`letta/helpers/tpuf_client.py:31-34`, `letta/orm/sqlalchemy_base.py:40-41`, `letta/server/db.py:84-85`). There is no single user-facing retry configuration surface.

**2. Are fallback providers available?**
Architecturally yes, operationally not in OSS. The routing-client seam exists (`letta/services/llm_router/__init__.py:6-12`) and the step loop performs real fallback switching — swapping `active_llm_config`/client and continuing the retry loop (`letta/agents/letta_agent_v3.py:1192-1211`). But `get_llm_routing_client()` silently degrades to the noop stub whenever the Redis-backed module import fails, and every meaningful method on the stub either noops or raises `RuntimeError` (`letta/services/llm_router/llm_router_client_base.py:38-40,42-46,62-64,82,84-93`). Auto-mode handles additionally require `model_settings.auto_mode_enabled=True` (default False, `letta/settings.py:119-122`; enforcement at `letta/services/provider_manager.py:987-988`). No static per-model "fallback model" field was found in `LLMConfig` or provider schemas — fallback topology is exclusively server/enterprise-side.

**3. Does the system degrade gracefully?**
Yes at the pod level, selectively at the request level. The readiness subsystem degrades liveness vs readiness correctly: `/health` stays 200 while `/ready` flips to 503 under sustained event-loop lag, in-flight saturation, or admission waits, with anti-flapping stabilization windows and multi-source recovery (`letta/monitoring/readiness_state.py:63-84`, `letta/settings.py:630-637`, `letta/server/rest_api/routers/v1/health.py:24-45`). Graceful drain during shutdown is supported (`letta/server/rest_api/app.py:256-261`). However all gating is off by default (`letta/settings.py:607-628`), so stock deployments do not degrade — they just saturate. Within a step, degradation is coarse: invalid/empty LLM responses abort immediately rather than degrading (`letta/agents/letta_agent_v3.py:1177-1182`). Smaller graceful degradations exist, e.g. non-fatal serialization (`letta/utils.py:882`) and excluding degraded embedding providers from selection (`letta/llm_api/openai_client.py:731`).

**4. Are circuit breakers used to prevent cascading failure?**
Named and interfaced, but not implemented in this source. The breaker concept appears explicitly in the router docs and its signal API (`record_failure`/`record_success`, `letta/services/llm_router/__init__.py:1`, `letta/services/llm_router/llm_router_client_base.py:42-46`), and the step loop feeds those signals only for models with fallback routes (`letta/agents/letta_agent_v3.py:1171-1174,1189-1190`). The actual threshold/open-half-open logic lives behind the missing Redis-backed module; docstrings state breaker support requires Redis (`letta/services/llm_router/llm_router_client_base.py:36-40`). No other generic circuit breaker (e.g., around MCP calls, tool sandbox, or DB) was found — searched patterns `circuit.?breaker|CircuitBreaker` across the repo; only the llm_router hits matched.

**5. Is retry state persisted?**
Mostly no. All observed retry loops keep counters in local variables; nothing writes attempt counts to Postgres. Two partial exceptions: (a) circuit-breaker/fallback health state is designed to be persisted externally in Redis (per docstrings), but that implementation is absent here; (b) the otid→run_id mapping persisted in Redis with TTL supports idempotent client retries across duplicate requests (`letta/data_sources/redis_client.py:250-271`). Errored steps/runs do record terminal error status (`track_errored_messages`, `track_stop_reason`, `letta/settings.py:384-385`), but that is outcome tracking, not retry-state persistence.

## Architectural Decisions

1. **Retry ownership split between SDKs and app code.** Anthropic/Gemini retries delegate to vendor SDK knobs (`letta/llm_api/anthropic_client.py:440-461`), while OpenAI-compatible paths historically relied on the app-level decorator and now map errors into a common taxonomy for upper layers to decide (`letta/llm_api/openai_client.py:1216-1281`). This avoids double-retry but creates per-provider behavioral differences (e.g., OpenAI SDK clients are constructed with `max_retries=0` in helper paths, `letta/llm_api/openai.py:481,514`).

2. **Semantic retry over blind retry.** The dominant retry axis in the agent loop is *context repair*: on `ContextWindowExceededError`, compact messages, rebuild system prompt, and re-request within the same bounded loop (`letta/agents/letta_agent_v3.py:1218-1263`). Retrying without modification is reserved for transport/transient errors.

3. **Router as an injectable seam with silent stub fallback.** Rather than shipping a broken half-feature, the package imports the real router if present and otherwise substitutes a noop class (`letta/services/llm_router/__init__.py:6-12`) — keeping OSS builds green while enterprise gets breakers. The cost is silent capability loss discoverable only at runtime (`RuntimeError`s from `llm_router_client_base.py:38-40,62-64,82`).

4. **Fallback is billing-aware and stream-safe.** Resolved model info is persisted onto the step even if resolution partially fails, so charging matches the actual model (`letta/agents/letta_agent_v3.py:1075-1087`); the ChatGPT streaming client refuses to retry once chunks have been yielded to avoid duplicate emission (`letta/llm_api/chatgpt_oauth_client.py:699-705`).

5. **Degradation targets scheduling, not functionality.** Degraded mode removes the pod from load-balancer rotation (503 on `/ready`) instead of reducing per-request functionality; recovery requires *all* degradation sources to clear, preventing premature flapping (`letta/monitoring/readiness_state.py:63-84`).

6. **Fail-fast for deterministic errors, retry for transient only.** A consistent classification layer (`is_transient_error` in `letta/helpers/tpuf_client.py:37-76`; `_RETRYABLE_ERRORS` in `letta/llm_api/chatgpt_oauth_client.py:102`; error-code checks in decorators) prevents retrying 400-class failures.

## Notable Patterns

- **Decorator-based retries with telemetry events**: each infra client wraps operations with a bespoke decorator emitting OTEL events (`llm_retry_attempt`, `turbopuffer_retry_attempt`) and metrics (`redis_timeout_counter`) — observable but non-uniform (`letta/llm_api/llm_api_tools.py:72-93`, `letta/helpers/tpuf_client.py:114-141`, `letta/data_sources/redis_client.py:137-142`).
- **Adaptive input shrinking as retry strategy**: embedding failures halve batch size / split text and re-enqueue failed chunks rather than sleeping-and-retrying (`letta/llm_api/openai_client.py:1085-1203`), unit-tested in `tests/test_embeddings.py:103-141`.
- **Prompt mutation between retries**: Gemini retries append corrective instructions about special characters after `MALFORMED_FUNCTION_CALL` (`letta/llm_api/google_vertex_client.py:186-191`).
- **Deadlock-specific retry with PG-code sniffing**: `40P01` detection across three encodings (`letta/orm/sqlalchemy_base.py:56-66`).
- **Watchdog out-of-band monitoring**: heartbeat scheduling measured against monotonic time from a separate thread so a frozen loop cannot hide itself (`letta/monitoring/event_loop_watchdog.py:85-108,42-45`), validated manually by `test_watchdog_hang.py:29-87`.
- **Test-infrastructure retries kept separate**: flaky-test mitigation lives in test helpers, not product code (`tests/helpers/utils.py:20-81`).

## Tradeoffs

- **Fragmentation vs fitness**: each subsystem's retry fits its failure profile (DB deadlocks, DNS blips, provider 429s), but there is no shared abstraction — adding a provider means re-implementing backoff, jitter, and transient classification, with drift risk (e.g., sync `time.sleep` in `letta/llm_api/llm_api_tools.py:104` would block an event loop if called async-context, versus `asyncio.sleep` in newer code).
- **Silent stub fallback vs loud failure**: conditional import keeps OSS simple but hides the absence of fallback/breaker features until a `RuntimeError` surfaces mid-step.
- **Opt-in safety vs default resilience**: readiness gating defaults preserve backward compatibility (`letta/settings.py:600-607`) at the cost of stock deployments having no automated degraded behavior.
- **Aggressive legacy retry (20×, unbounded wall time)** maximizes completion odds for CLI-era usage (`letta/llm_api/llm_api_tools.py:43`) but can hold resources far longer than interactive callers expect; newer paths cap at 3–5.
- **Redis as resilience prerequisite**: locks, idempotency mappings, and (intended) breaker state all assume Redis (`letta/data_sources/redis_client.py:663-681`); without it a noop client quietly removes those guarantees.

## Failure Modes / Edge Cases

- **Provider outage in OSS deployment**: 429-only decorator doesn't cover 5xx/connection loss on the legacy path; the v3 loop re-raises `LLMRateLimitError`/`LLMServerError`/`LLMProviderOverloaded` when no fallback handle exists (`letta/agents/letta_agent_v3.py:1212-1213`) → step fails; recovery depends on client retry + otid idempotency mapping (Redis-only).
- **Fallback storm**: when fallback fires, subsequent iterations keep the fallback config active "and any subsequent retries (e.g. compaction)" (`letta/agents/letta_agent_v3.py:1201-1211`) — good for stickiness, but there is no visible attempt cap distinct from `max_summarizer_retries`, so repeated primary flapping relies solely on that small bound.
- **Retry amplification**: layered retries multiply (SDK 3× × loop `(3+1)` × per-attempt inner policies), worst case delaying failure notification by minutes given 20×429-retry with exponential growth (`letta/llm_api/llm_api_tools.py:43,97-104`).
- **Jitter asymmetry**: generic decorator applies multiplicative jitter before sleep but grows delay even on the final failed check; turbopuffer adds ≤10% jitter after sleep decision (`letta/llm_api/llm_api_tools.py:97`, `letta/helpers/tpuf_client.py:148-150`) — minor divergence in thundering-herd protection.
- **Degradation blind spots when disabled**: with `enforcement_enabled=False` (default), watchdog still logs/dumps stacks on hangs but never gates traffic (`letta/monitoring/event_loop_watchdog.py:216-217`).
- **Noop Redis masks lock/idempotency loss**: initialization failure downgrades to `NoopAsyncRedisClient` with only a warning (`letta/data_sources/redis_client.py:678-680`), silently disabling duplicate-request recovery that retries depend on.

## Future Considerations

- Consolidate the ~8 bespoke backoff loops into one shared, injectable retry policy (configurable max_attempts/base/jitter/classifier), preserving existing telemetry event names.
- Ship an OSS reference implementation of `LLMRoutingClient` (in-memory or Postgres-backed breaker) so fallback works without Redis, and make stub degradation log loudly at startup.
- Persist per-handle breaker state and expose it (e.g., admin endpoint) for observability of fallback health.
- Enable at least the admission-wait or event-loop-lag gate by default in production compose files (`compose.yaml`, `development.compose.yml`) since all machinery already exists.
- Add unit tests for `retry_with_exponential_backoff`, the v3 fallback-switch branch, and readiness transitions (current coverage: embeddings retry only, `tests/test_embeddings.py:103-205`).

## Questions / Gaps

- **Where is the Redis-backed `llm_router_client`?** Searched entire source tree for `llm_router_client.py` and `circuit` symbols; only the base stub and conditional import exist. Conclusion: enterprise-only module, not auditable here. No evidence found in-repo of breaker thresholds (failure count, cooldown, half-open probing).
- **What do `AUTO_MODE_HANDLES` contain and how are fallback routes chosen?** `provider_manager.py:941-959` lists synthetic handles but the constant's definition and route mapping were not found in the audited tree (likely alongside the enterprise module). No evidence found.
- **Is `retry_with_exponential_backoff` still on any hot path?** It decorates only the legacy sync `create()` (`letta/llm_api/llm_api_tools.py:122`); v3 agents use `LLMClient` classes whose internal retry behavior varies by provider. Extent of legacy-path usage in current defaults could not be fully traced.
- **Multi-agent send retries**: `multi_agent_send_message_max_retries` is defined (`letta/settings.py:350`) but no consumer was found in the audited tree — possibly consumed by group-chat code paths renamed or moved; searched `services/` and `agents/`. No evidence found of its usage.
- **Run-level automatic re-execution**: no evidence found of a job/run supervisor that automatically retries failed runs; runs transition to error states and it is left to callers (searched `services/` for retry/attempt patterns; only per-operation loops matched).

---

Generated by `dimensions/13.02-retry-fallback-degraded-mode.md` against `letta`.

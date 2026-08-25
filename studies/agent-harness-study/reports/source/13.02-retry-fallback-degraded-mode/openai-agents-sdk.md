# Source Analysis: openai-agents-sdk

## Dimension 13.02 — Retry, Fallback, and Degraded Mode

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (asyncio, pydantic, httpx, OpenAI SDK) |
| Analyzed | 2026-08-25 |

## Summary

The SDK implements an opt-in, policy-driven **runner-managed retry** system for model calls that is one of the most carefully engineered retry stacks observed in agent harnesses. Retry behavior is configured through `ModelRetrySettings` on `ModelSettings` (`src/agents/retry.py:209-230`, `src/agents/model_settings.py:188-189`), decided by composable `RetryPolicy` callables (`src/agents/retry.py:185`, `src/agents/retry.py:304-462`), executed by dedicated streaming/non-streaming retry loops (`src/agents/run_internal/model_retry.py:574-681`, `src/agents/run_internal/model_retry.py:684-891`), and enriched by a provider-advice protocol (`Model.get_retry_advice`, `src/agents/models/interface.py:59-65`). The distinguishing feature is a **replay-safety boundary**: aborts, already-emitted stream output, stateful (`previous_response_id`/`conversation_id`) requests, and provider-marked-unsafe failures are hard vetoes unless explicitly approved via `RetryDecision.approve_unsafe_replay` (`src/agents/run_internal/model_retry.py:419-479`, `src/agents/retry.py:124-129`).

By contrast, **fallback providers/models do not exist as a failure-handling mechanism**. `MultiProvider` routes by model-name prefix at resolution time only (`src/agents/models/multi_provider.py:62-252`); there is no code path that switches models or providers after a failure. There are **no circuit breakers** (searches for "circuit"/"degraded" across `src/` return nothing) and no generic degraded mode. Partial degradation exists in adjacent layers: run error handlers can synthesize a fallback final output for terminal model-behavior errors (`src/agents/run_error_handlers.py:50-55`), tracing export failures are non-fatal with bounded retries (`src/agents/tracing/processors.py:159-221`), and MCP servers have their own configurable retry/backoff plus isolated-session recovery (`src/agents/mcp/server.py:874-944`, `src/agents/mcp/server.py:2325-2460`).

Retry state is deliberately not persisted: the policy callback is excluded from serialization (`src/agents/retry.py:219-220`) and attempt counters live only inside one model-call invocation (`src/agents/run_internal/model_retry.py:585-597`; no "retry" matches in `src/agents/run_state.py`). What *is* coordinated across retries is durable conversation state: session rewind, server-conversation tracker rewind, and post-success re-commit (`src/agents/run_internal/session_persistence.py:727-782`, `src/agents/run_internal/run_loop.py:2214-2216`, `src/agents/run_internal/run_loop.py:2628-2634`).

## Rating

**8 / 10.**

Rationale against the rubric: this is a clear, explicit, well-tested retry model with operational safeguards (replay-safety vetoes, hidden-retry coordination, per-attempt usage accounting, timeout/cancellation hygiene), extensive tests (`tests/models/test_model_retry.py`, 3714 lines; `tests/models/test_openai_retry_helpers.py`, 193 lines), first-class documentation (`docs/models/index.md:512-604`), runnable examples (`examples/basic/retry.py`, `examples/basic/retry_litellm.py`), and test-double support (`src/agents/testing/model.py:188-198`, `306-308`). It falls short of 9-10 because two of the dimension's sub-areas are absent rather than mature: no fallback-model/provider failover under sustained outage (a single-provider outage still fails requests once the budget is exhausted), no circuit breaker or degraded-mode machinery, and retry observability is limited to debug logs plus zero-token usage entries rather than dedicated spans or metrics.

## Evidence Collected

Every entry cites the selected source with file path and line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Public retry API surface | `ModelRetrySettings`, `RetryDecision`, `RetryPolicyContext`, `retry_policies` exported from package root | src/agents/__init__.py:102-111 |
| Backoff config schema | `ModelRetryBackoffSettings` fields `initial_delay`, `max_delay`, `multiplier`, `jitter` (validated ≥ 0) | src/agents/retry.py:15-33 |
| Retry settings container | `ModelRetrySettings(max_retries, backoff, policy)`; policy is runtime-only, excluded from serialization | src/agents/retry.py:209-230 |
| Policy decision object | `RetryDecision(retry, delay, reason, approve_unsafe_replay)` with internal `_hard_veto`/`_approves_replay` flags | src/agents/retry.py:116-133 |
| Policy context | `RetryPolicyContext(error, attempt, max_retries, stream, normalized, provider_advice, ...)` + `response_started`/`replay_safety`/`stateful_request` properties | src/agents/retry.py:142-182 |
| Built-in policies | `never()`, `provider_suggested()`, `network_error()`, `retry_after()`, `http_status([...])` combinators `all()`/`any()` with veto-aware merging | src/agents/retry.py:304-462 |
| Policy capability marking | `_mark_retry_capabilities` tags whether a policy retries safe transport errors (drives websocket pre-event retry disabling) | src/agents/retry.py:186-206 |
| Default backoff constants | 0.25 s initial, 2.0 s max, ×2 multiplier, jitter enabled | src/agents/run_internal/model_retry.py:46-49 |
| Backoff computation | Exponential growth capped at max, ±12.5% uniform jitter when enabled | src/agents/run_internal/model_retry.py:175-204 |
| Non-streaming retry loop | `get_response_with_retry`: attempt counters, rewind before replay, delay preference policy-delay > retry-after > backoff | src/agents/run_internal/model_retry.py:574-681 |
| Streaming retry loop | `stream_response_with_retry`: per-attempt deadline, single-task stream ownership, pre/post-event retry windows | src/agents/run_internal/model_retry.py:684-891 |
| Retry safety evaluation | `_evaluate_retry`: budget check, abort/emitted-output absolute vetoes, three separate replay vetoes (request-level, stateful fail-closed, provider-unsafe) | src/agents/run_internal/model_retry.py:389-493 |
| Stream-event retry window | Only `response.created`/`response.in_progress` are retry-safe; any other event sets `emitted_retry_unsafe_event` | src/agents/run_internal/model_retry.py:51, 384-386, 806-807 |
| Legacy compatibility retry | `conversation_locked` BadRequestError retried up to 3× with 1s·2^n delay, even without configured retries; opt-out via `max_retries=0` | src/agents/run_internal/model_retry.py:50, 504-513, 619-640 |
| Failed-attempt usage accounting | `apply_retry_attempt_usage` prepends zero-token request entries so request counts reflect retries | src/agents/run_internal/model_retry.py:318-350 |
| Per-attempt timeouts | `ModelSettings.timeout` bounds each attempt; `ModelTimeoutError` raised after cooperative cancel + cleanup drain; feeds `is_timeout` into policy context | src/agents/model_settings.py:215-221; src/agents/run_internal/model_retry.py:288-315 |
| Error normalization | Status code, error code, request id, retry-after, network/timeout/abort classification extracted over the full exception chain | src/agents/run_internal/model_retry.py:116-156; src/agents/models/_retry_runtime.py:16-135 |
| Retry-after parsing | `retry-after-ms` (ms float) and `retry-after` (seconds or HTTP-date) parsed from response headers anywhere in error chain | src/agents/models/_retry_runtime.py:60-97 |
| Provider advice protocol | `Model.get_retry_advice()` default returns None; adapters override to supply replay safety/delay guidance | src/agents/models/interface.py:59-65 |
| OpenAI advice rules | Honors `x-should-retry` header, network/timeout classes, status {408,409,429,5xx+}, `unsafe_to_replay` attribute; marks stateful HTTP failures replay-safe | src/agents/models/_openai_retry.py:41-112 |
| Websocket-specific advice | Phase-aware timeout advice ("request lock wait"/"connect" safe vs send/receive phases), never-sent disconnects safe, overload frames retryable | src/agents/models/openai_responses.py:1119-1188 |
| Transport replay tagging | WS errors tagged `_openai_agents_ws_replay_safety`/`_ws_response_started`; frame-send marks request potentially transmitted | src/agents/models/openai_responses.py:1321-1323, 1383-1389 |
| Pre-event disconnect transport retry | One silent reconnect allowed only if no event received AND request frame not sent; otherwise wrapped with safe/unsafe classification | src/agents/models/openai_responses.py:1391-1420 |
| Hidden provider-retry coordination | ContextVar disables provider-managed retries; consumed via `with_options(max_retries=0)` (Responses & Chat Completions) and litellm `num_retries=0` | src/agents/models/_retry_runtime.py:138-158; src/agents/models/openai_responses.py:1064-1071; src/agents/models/openai_chatcompletions.py:801; src/agents/extensions/models/litellm_model.py:639-643 |
| When hidden retries are disabled | Decision matrix by attempt number, statefulness, replay-unsafe requests, explicit `max_retries<=0` opt-out | src/agents/run_internal/model_retry.py:516-554 |
| Runner wiring (streaming) | `stream_response_with_retry(...)` invoked from run loop with `model_settings.retry`, advice callback, timeout, PTC-tool replay veto | src/agents/run_internal/run_loop.py:2220-2245 |
| Runner wiring (non-streaming) | `get_response_with_retry(...)` invoked likewise; success re-marks input as sent/accepted to server conversation tracker | src/agents/run_internal/run_loop.py:2603-2634 |
| Session rewind on retry | `rewind_session_items` pops retry-owned suffix after fingerprint verification; restores popped items on mismatch | src/agents/run_internal/session_persistence.py:727-782, 978-1040, 1043-1065 |
| Post-rewind consistency check | `wait_for_session_cleanup` verifies rewound items left the tail (bounded attempts, linear backoff); stray retry-owned tail items stripped up to a known server item | src/agents/run_internal/session_persistence.py:834-876, 784-821, 1068-1098 |
| Retry-state non-persistence | Attempt counters are function locals; policy excluded from serialization; RunState contains no retry fields | src/agents/run_internal/model_retry.py:585-597; src/agents/retry.py:219-220 |
| MCP tool-call retries | `max_retry_attempts`, `retry_backoff_seconds_base/max` constructor knobs; exponential backoff with optional cap; `-1` = unlimited | src/agents/mcp/server.py:874-944, 1219-1239 |
| MCP isolated-session recovery | Stdio failures can retry inside an isolated session; budget charged twice per isolated retry; `_IsolatedSessionRetryFailed` guard | src/agents/mcp/server.py:525-526, 2325-2460 |
| MCP server reconnect | Manager `retry_failed_servers(failed_only=True)` cleanup+reconnect path for failed servers | src/agents/mcp/manager.py:319-345 |
| Sandbox infra retries | `retry_async` decorator: fixed/linear/exponential backoff, transient statuses {500,502,503,504}; used by Docker/Modal/Daytona/Cloudflare/Vercel/Blaxel/E2B sandboxes | src/agents/sandbox/util/retry.py:14-127; src/agents/sandbox/sandboxes/docker.py:1359; src/agents/extensions/sandbox/e2b/sandbox.py:1337 |
| Tracing export retries | BatchTraceProcessor: `max_retries=3` default, exponential backoff + 10% jitter, deadline/shutdown-aware sleeps, 4xx treated as permanent | src/agents/tracing/processors.py:63-84, 158-256 |
| Degraded final output | `RunErrorHandlers` keyed by `max_turns`/`model_refusal`/`invalid_final_output` return replacement final output instead of failing the run | src/agents/run_error_handlers.py:35-55 |
| Test coverage (model retry) | 3714-line suite: budgets, vetoes, provider coordination, cancellation, timeouts, usage entries, streaming windows, websocket pre-event retries | tests/models/test_model_retry.py:150-2997 (representative) |
| Provider-advice unit tests | Header/status/network classification tests | tests/models/test_openai_retry_helpers.py:1-193 |
| Scripted retry testing | `ModelStep.raise_error(..., retry_advice=...)` lets tests script provider advice consumed by runner policies | src/agents/testing/model.py:188-198, 269, 306-308 |
| Docs | "Runner-managed retries" section incl. safety boundaries and deep-merge semantics | docs/models/index.md:512-604 |
| Examples | Full example composing `provider_suggested()+retry_after()+network_error()+http_status([...])` with logging wrapper policy | examples/basic/retry.py:22-81 |

## Answers to Dimension Questions

**1. Are retries configurable?**
Yes, extensively — but opt-in. Configuration lives in `ModelSettings.retry: ModelRetrySettings` (`src/agents/model_settings.py:188-189`) accepting `max_retries`, structured backoff (`initial_delay/max_delay/multiplier/jitter`, `src/agents/retry.py:15-33`), and an arbitrary sync-or-async policy callable (`src/agents/retry.py:185`, evaluated at `src/agents/run_internal/model_retry.py:165-172`). Policies compose via `retry_policies.any/all` (`src/agents/retry.py:376-461`) and settings deep-merge between `RunConfig.model_settings` and agent-level overrides (`src/agents/model_settings.py:284-287, 377-392`, documented at `docs/models/index.md:598-604`). Defaults are conservative: without `ModelSettings(retry=...)` the runner schedules zero retries for general requests (`src/agents/run_internal/model_retry.py:435-436, 654-657`), relying instead on provider-managed retries on first attempts (`src/agents/run_internal/model_retry.py:545-549`). One legacy knob remains implicit: the `conversation_locked` compatibility retry runs even unconfigured and is disabled only via explicit `max_retries=0` (`src/agents/run_internal/model_retry.py:504-513`).

**2. Are fallback providers available?**
No — not as failure handling. `MultiProvider` maps name prefixes (`openai/`, `litellm/`, `any-llm/`) to providers at model-resolution time (`src/agents/models/multi_provider.py:199-225`); its `_fallback_providers` cache (`src/agents/models/multi_provider.py:153, 190-197`) means "built-in providers for extra prefixes," not failover targets. Unknown prefixes either raise `UserError` or pass through to the OpenAI-compatible endpoint (`unknown_prefix_mode`, `src/agents/models/multi_provider.py:222-225`). No evidence found of any code path that swaps model/provider after a failed attempt; searches for "fallback" in model-selection contexts returned only routing and display fallbacks. A sustained single-provider outage therefore exhausts the retry budget and fails the run.

**3. Does the system degrade gracefully?**
Partially, through targeted mechanisms rather than a general degraded mode:
- Model-call failures degrade into typed errors with normalized facts (`ModelTimeoutError`, `src/agents/run_internal/model_retry.py:308-315`) and, for terminal behavioral errors (`MaxTurnsExceeded`, `ModelRefusalError`, `ModelBehaviorError`), applications can install `RunErrorHandlers` that return a substitute `final_output` validated against the same schema instead of failing the run (`src/agents/run_error_handlers.py:28-55`; documented constraint: it does not retry the model call, `docs/running_agents.md:500`).
- Telemetry degrades gracefully: tracing export errors are logged `[non-fatal]` and retried with bounded backoff so observability outages never break runs (`src/agents/tracing/processors.py:159-256`).
- Streaming degrades safely: once any user-visible event has been emitted, retries are vetoed rather than risking duplicate output (`src/agents/run_internal/model_retry.py:419-433`).
- No CPU/queue shedding, feature toggles, or reduced-capability modes exist (no "degraded" matches in `src/`).

**4. Are circuit breakers used to prevent cascading failure?**
No evidence found. Searches for "circuit", "CircuitBreaker", and "degraded" across the selected source return no matches. The closest protective mechanisms are the finite retry budgets (`max_retries`, enforced at `src/agents/run_internal/model_retry.py:403-404`; capped compatibility retries at `src/agents/run_internal/model_retry.py:628-640`), per-attempt timeouts (`src/agents/model_settings.py:215-221`), bounded tracing export retries (`src/agents/tracing/processors.py:211-215`), and bounded session-cleanup verification loops (`src/agents/run_internal/session_persistence.py:851-876`). There is no shared failure counter, open/half-open state, or request-fast-fail path.

## Architectural Decisions

1. **Policy-as-code over declarative config.** Retries are decided by user-supplied callables receiving rich context (`src/agents/retry.py:142-185`), with built-in combinators as conveniences (`src/agents/retry.py:304-462`). This makes exotic requirements (per-error-code logic, telemetry side effects — see `examples/basic/retry.py:32-70`) expressible without new config surface.
2. **Two-layer retry with explicit ownership handoff.** Provider-managed retries stay enabled on the first attempt for backward compatibility, then are switched off (`with_options(max_retries=0)` / `num_retries=0`) whenever the runner takes over scheduling, preventing compounding retry storms (`src/agents/run_internal/model_retry.py:516-554`, applied at `src/agents/models/openai_responses.py:1064-1071`, `src/agents/extensions/models/litellm_model.py:639-643`). The handoff decision is centralized behind one predicate plus ContextVars (`src/agents/models/_retry_runtime.py:138-158`).
3. **Replay-safety as a first-class, fail-closed contract.** Three independent vetoes (local side effects e.g. Programmatic Tool Calling; stateful `previous_response_id`/`conversation_id` requests; provider-marked unsafe failures) block retries regardless of policy verdict; lifting them requires provider "safe" advice or an explicit `approve_unsafe_replay=True` that ordinary `retry=True` cannot imply (`src/agents/run_internal/model_retry.py:443-479`, `src/agents/retry.py:124-129`).
4. **Provider-advice protocol on the `Model` interface.** Adapters translate transport specifics (headers like `x-should-retry`, websocket frames, timeout phases) into a normalized `ModelRetryAdvice` (`src/agents/models/interface.py:59-65`, `src/agents/models/_openai_retry.py:41-112`, `src/agents/models/openai_responses.py:1119-1188`), keeping runner logic provider-agnostic.
5. **Delay precedence chain.** Explicit policy delay → provider `retry_after` → computed backoff (`src/agents/run_internal/model_retry.py:481-491`), so server-specified pacing always wins over local heuristics.
6. **Durable-state repair instead of durable retry counters.** Rather than persisting attempt state, each replay first rewinds sessions and server-conversation tracking with fingerprint-verified suffix matching and restore-on-mismatch (`src/agents/run_internal/session_persistence.py:727-782, 978-1065`), then re-commits accepted input after success (`src/agents/run_internal/run_loop.py:2628-2634`).
7. **Opt-in runtime-only semantics.** The whole system is off by default and the policy is never serialized (`src/agents/retry.py:219-220`), matching the SDK's stance that "the SDK does not retry general model requests unless you set `ModelSettings(retry=...)`" (`docs/models/index.md:514`).

## Notable Patterns

- **Capability flags on closures:** policies advertise what they cover (`retries_safe_transport_errors`, `retries_all_transient_errors`) via attributes set by `_mark_retry_capabilities` (`src/agents/retry.py:186-206`); the runner reads these to decide whether websocket pre-event transport retries must also be disabled (`src/agents/run_internal/model_retry.py:557-571`).
- **Veto algebra in combinators:** `any()`/`all()` preserve hard vetoes, allow "delegable" replay vetoes to be overridden by later approvals, and merge delays/reasons deterministically (`src/agents/retry.py:249-301, 376-461`).
- **Exception-chain forensics:** every classifier walks `__cause__`/`__context__` chains with cycle protection (`src/agents/models/_retry_runtime.py:16-23`), including duck-typed legacy-httpx and websockets library checks (`src/agents/run_internal/model_retry.py:52-113`).
- **Zero-token usage bookkeeping:** failed attempts are recorded as synthetic zero-token request entries so downstream cost/request metrics include retries (`src/agents/run_internal/model_retry.py:318-350`, streamed variant at 806-811).
- **Single-owner task pattern for timed streams:** stream construction, pulls, and close run inside one asyncio task so timeouts cancel cleanly without leaking provider iterators or traceback references (`src/agents/run_internal/model_retry.py:213-315`).
- **Test-first ergonomics:** `ScriptedModel` accepts scripted `retry_advice` per failure (`src/agents/testing/model.py:188-198, 306-308`), letting policy tests run without real providers (`docs/testing.md:186-199`).
- **Layered reuse of one decorator:** the sandbox layer shares a single `retry_async` implementation across seven sandbox vendors with pluggable transient-predicates (`src/agents/sandbox/util/retry.py:65-127`).

## Tradeoffs

- **Safety versus availability:** the fail-closed stateful-request rule means a transient blip on a `previous_response_id` follow-up fails unless the app grants `approve_unsafe_replay` or the provider returns safe advice (`src/agents/run_internal/model_retry.py:454-469`) — correct for duplicate-side-effect avoidance, but it shifts resilience burden to application code and provider cooperation.
- **Opt-in default reduces out-of-the-box survivability:** fresh deployments get no runner retries at all (`src/agents/run_internal/model_retry.py:435-436`); surviving a rate-limit burst requires knowing about `retry_policies` and wiring them.
- **Hidden-retry coordination adds coupling:** correctness depends on subtle matrices (attempt number × statefulness × policy presence) spread across `_should_disable_provider_managed_retries`, `_should_disable_websocket_pre_event_retry`, and per-adapter checks (`src/agents/run_internal/model_retry.py:516-571`) — heavily tested but cognitively expensive to modify.
- **No cross-provider escape hatch:** prefix routing is static (`src/agents/models/multi_provider.py:199-225`); during a full OpenAI outage there is no automatic rerouting to litellm/any-llm even though both are registered.
- **Compatibility retry is implicit:** `conversation_locked` retries consume up to 3 attempts invisibly to the policy layer (`src/agents/run_internal/model_retry.py:619-640`), which can surprise users who assume all retries flow through their policy.
- **Observability gap:** retries surface only as debug logs (`src/agents/run_internal/model_retry.py:669-676`) and zero-token usage entries; there are no retry spans/counters, complicating production diagnosis of flaky-provider periods.

## Failure Modes / Edge Cases

Handled explicitly:
- Abort/cancellation propagation with cooperative cleanup draining, ensuring parent cancellation is never swallowed by retry loops (`src/agents/run_internal/model_retry.py:213-255, 301-303, 827-830`; tests at `tests/models/test_model_retry.py:318-407`).
- Timeout after partial stream output: retry vetoed because output was already emitted (`src/agents/run_internal/model_retry.py:419-433`; test `test_stream_response_with_retry_does_not_retry_timeout_after_output`, `tests/models/test_model_retry.py:2053`).
- Provider advice conflicts: provider normalization may add but never clear abort evidence (`src/agents/run_internal/model_retry.py:151-154`); explicit `False`/`None` overrides respected via `_explicit_fields` (`tests/models/test_model_retry.py:1013-1088`).
- Session rewind mismatch or pop failure: restore previously popped items and skip rewind with a warning rather than corrupting history (`src/agents/run_internal/session_persistence.py:1017-1065`).
- Stray retry-owned items left in server-managed conversations get stripped up to a known server item (`src/agents/run_internal/session_persistence.py:800-821`).
- Uncapped exponential overflow guarded in MCP backoff via `math.ldexp`/`copysign(inf)` (`src/agents/mcp/server.py:1224-1228`).

Residual risks:
- Jitter uses module-global `random` without seeding hooks (`src/agents/run_internal/model_retry.py:204`), making exact-delay tests dependent on monkeypatching (the test suite does exactly this).
- `wait_for_session_cleanup` gives up after ~5 attempts (~1.5 s total) leaving potential temporary duplicates (`src/agents/run_internal/session_persistence.py:874-876`).
- Message-substring network classification ("connection error", "socket hang up", etc., `src/agents/run_internal/model_retry.py:107-113`) can misclassify non-network errors phrased similarly.

## Future Considerations

- Add an explicit **fallback-model chain** (ordered list tried on policy-exhausted failure) — the `MultiProviderMap` already centralizes provider lookup (`src/agents/models/multi_provider.py:18-59`) and would be the natural seam.
- Introduce **circuit-breaker state** per provider/prefix to fast-fail during sustained outages; the capability-flag mechanism (`src/agents/retry.py:186-206`) shows the team already reasons about cross-cutting retry metadata where such state could live.
- Emit **retry spans/events** (attempt number, delay, reason, provider advice) into the existing tracing system (`src/agents/tracing/span_data.py`) for production observability parity with the tracing-export retry logging.
- Promote the legacy `conversation_locked` compatibility path into a normal, visible policy once the release boundary allows removing the implicit behavior (`src/agents/run_internal/model_retry.py:504-513`).
- Document a recommended baseline policy (the `examples/basic/retry.py:22-30` composition is a strong candidate) so default-off does not translate to default-fragile.

## Questions / Gaps

- **Is there any supported multi-provider failover?** No evidence found. Searched `fallback`, `MultiProvider`, provider selection paths in `src/agents/models/`; all hits were routing/display fallbacks, none triggered by failure.
- **Are retries observable beyond debug logs?** Only partially: zero-token usage entries (`src/agents/run_internal/model_retry.py:338-350`) and `logger.debug` lines. No retry-specific span data found in `src/agents/tracing/`.
- **Does resume-from-serialized-run-state preserve mid-retry progress?** Not applicable by design: retries are scoped inside one model call and `RunState` carries no retry fields (no matches for "retry" in `src/agents/run_state.py`). Resume replays from persisted turn items instead.
- **Realtime retry behavior:** realtime sessions expose no runner-style retry policy; only an invocation-restart warning exists (`src/agents/realtime/session.py:1145`). Outage survival for realtime relies on the underlying connection, outside this dimension's model-retry scope.

---

Generated by `13.02-retry-fallback-and-degraded-mode` against `openai-agents-sdk`.

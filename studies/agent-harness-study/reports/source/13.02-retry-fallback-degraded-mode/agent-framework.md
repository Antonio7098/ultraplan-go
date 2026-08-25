# Source Analysis: agent-framework

## Dimension 13.02 — Retry, Fallback, and Degraded Mode

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Polyglot monorepo: Python (`python/packages/*`), .NET/C# (`dotnet/src/*`); `go/` is a stub README pointing at a separate repository |
| Analyzed | 2026-08-25 |

## Summary

The framework has **no unified retry/fallback/circuit-breaker abstraction**. Resilience is instead distributed across four distinct mechanisms: (1) user-authored middleware on the model-call path, with the official sample delegating to the third-party `tenacity` library for exponential backoff on rate limits; (2) hand-rolled bounded retry loops (default 3 attempts, linear backoff) inside multi-agent orchestrators for structured-output parse failures; (3) a carefully engineered reconnect-and-retry protocol for MCP tool transports that explicitly guards against double-execution of side-effecting tools; and (4) a genuine, health-check-integrated **degraded mode** in the .NET Foundry hosting layer, where failed toolbox enumeration defers to per-request retries while keeping the container routable. Fallback models/providers are entirely absent — provider selection is construction-time only. Retry attempt counters are in-memory only, though workflow checkpointing provides crash-resume semantics that partially compensate.

## Rating

**5 / 10** — Present but inconsistent and fragile as a whole. The MCP transport resilience (`_mcp.py`) is genuinely mature: idempotency-aware, deadline-bounded, and well tested. The Foundry hosting degraded mode is a real operational safeguard. But the primary model-call path ships no built-in retry policy, no fallback providers, and no circuit breakers; retry behavior must be assembled by each application from middleware plus a third-party library. Retry configuration is fragmented (different parameter names, defaults, and backoff styles per component), and streaming responses are explicitly excluded from the documented retry approach.

## Evidence Collected

All paths are workspace-relative under the source root `studies/agent-harness-study/sources/agent-framework/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| No framework-level retry dependency | No Polly (.NET) or tenacity (Python) in package manifests; grep for `Polly`, `ResiliencePipeline`, `tenacity` over `dotnet/src`, `dotnet/Directory.Packages.props`, `python/packages/**`, `python/pyproject.toml` returned no source hits (tenacity appears only in one sample) | `studies/agent-harness-study/sources/agent-framework/dotnet/Directory.Packages.props:1` |
| Middleware extension point for retries | `AgentMiddleware` docstring includes a `RetryMiddleware(max_retries=3)` example looping over `call_next()` | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_middleware.py:546-566` |
| Sanctioned chat-path retry pattern (sample) | Tenacity `AsyncRetrying(stop=stop_after_attempt(3), wait=wait_exponential(multiplier=1, min=4, max=10), retry=retry_if_exception_type(RateLimitError))` applied via class decorator and via class/function chat middleware | `studies/agent-harness-study/sources/agent-framework/python/samples/02-agents/auto_retry.py:87-93, 134-142, 163-169` |
| Streaming excluded from sample retry | Decorator returns original `get_response` untouched when `stream=True`: "Streaming retry is more complex; fall back to the original behaviour" | `studies/agent-harness-study/sources/agent-framework/python/samples/02-agents/auto_retry.py:82-84` |
| OpenAI client wrapper does not surface retries | `AsyncOpenAI(**client_args)` built from api_key/org/base_url/timeout only; no `max_retries` passed, so SDK defaults apply implicitly unless caller injects a pre-built client | `studies/agent-harness-study/sources/agent-framework/python/packages/openai/agent_framework_openai/_shared.py:235-246` |
| .NET SDK-level retry plumbing | `FoundryChatClient` propagates caller-supplied `ClientPipelineOptions.RetryPolicy` into the project-level `AIProjectClientOptions`; policies added via `AddPolicy` deliberately do NOT propagate | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Foundry/FoundryChatClient.cs:624-646` |
| Sample configures Azure SDK retry | `new AIProjectClientOptions { RetryPolicy = new ClientRetryPolicy(3) }` in production-readiness harness sample | `studies/agent-harness-study/sources/agent-framework/dotnet/samples/02-agents/Harness/BuildYourOwnClaw/Claw_Step04_ProductionReady/ClawAgent/ClawAgentFactory.cs:81-84` |
| Magentic ledger parse retry (Python) | Bounded loop `while attempts < self.progress_ledger_retry_count` with backoff `asyncio.sleep(0.25 * attempts)`, default count 3, raises after exhaustion | `studies/agent-harness-study/sources/agent-framework/python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:719-739` |
| Magentic ledger retry configurable | `progress_ledger_retry_count: int \| None = None` constructor kwarg, default 3 | `studies/agent-harness-study/sources/agent-framework/python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:544, 588-590` |
| Magentic ledger parse retry (.NET) | Same pattern: loop to `TaskLimits.MaxProgressLedgerRetryCount`, `Task.Delay(250 * attempts)`, emits `WorkflowWarningEvent` per failure | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticManager.cs:66-106` |
| Ledger retry default (.NET) | `public const int DefaultMaxProgressLedgerRetryCount = 3;` | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticTaskContext.cs:14-17` |
| GroupChat speaker-selection retry (Python) | Optional `retry_attempts` kwarg; on exception decrements counter and re-prompts model with "Your input could not be parsed due to an error: {ex}. Please try again."; raises when exhausted | `studies/agent-harness-study/sources/agent-framework/python/packages/orchestrations/agent_framework_orchestrations/_group_chat.py:309-331, 526-542` |
| MCP tool-call reconnect-and-retry | Single attempt loop: on `ClosedResourceError` or "session terminated" `McpError`, calls `self.connect(reset=True)` and retries once; non-connection errors fail immediately | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_mcp.py:2108-2160` |
| Proactive connection health check | `_ensure_connection` pings the server before use and reconnects if invalid; disables ping permanently when server answers `-32601` (method-not-found) | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_mcp.py:1984-2021` |
| Idempotency-aware task retry policy | Before a `task_id` exists, connection loss raises "cannot safely retry" rather than re-sending (avoiding double execution of side-effecting tools); after `task_id` is known, `_send_with_one_reconnect` allows exactly one reconnect-and-retry against the same id | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_mcp.py:2243-2246, 2372-2381, 2509-2546` |
| Transient-error classification during LRO polling | Only HTTP 408 REQUEST_TIMEOUT treated as transient during `tasks/get` polling; retried bounded by client-side `max_task_wait` deadline; all other errors raise `_MCPTaskAbandoned` | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_mcp.py:2385-2413` |
| MCP reconnect tests | `test_mcp_tool_reconnects_after_session_terminated_error`, `test_mcp_tool_call_tool_raises_after_reconnection_still_fails`, ping-failure reconnect tests | `studies/agent-harness-study/sources/agent-framework/python/packages/core/tests/core/test_mcp.py:1347, 4801, 5367-5417` |
| Skills cache retry-on-failure semantics | Failed fetch leaves cache empty so next call retries; failed refresh keeps previously cached list and "keeps retrying on subsequent calls until one succeeds" | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_skills.py:3824-3840` |
| Degraded startup status enum (.NET) | `FoundryToolboxStartupStatus.Degraded = 5` (plus `ConsentRequired`, `NoEndpoint`): container stays routable, enumeration deferred to per-request resolution | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/FoundryToolboxStartupStatus.cs:39-64` |
| Degraded mode wired into health checks | Health check reports Healthy for Degraded/ConsentRequired/NoEndpoint, Unhealthy for Pending/failed toolboxes, exposing `failedToolboxes` data for operators | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/FoundryToolboxHealthCheck.cs:34-79` |
| Per-request degraded retry loop | `RetryDeferredToolboxesAsync` re-enumerates deferred toolboxes per request; persistent failures stay deferred ("retried on a later request") | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/FoundryToolboxService.cs:334-404` |
| Degraded retry invoked from request pipeline | Handler calls `.RetryDeferredToolboxesAsync(cancellationToken)` at start of request handling | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/AgentFrameworkResponseHandler.cs:289` |
| Orchestration self-healing (reset/replan) | Python Magentic reset clears history and resets stall count (`reset_count += 1`); .NET orchestrator catches ledger failures (non-cancellation) and triggers `ResetAndReplanAsync` instead of failing the workflow | `studies/agent-harness-study/sources/agent-framework/python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:389-394`; `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticOrchestrator.cs:266-281` |
| Stall/reset bounds prevent runaway loops | `max_stall_count: int = 3`, `max_reset_count`, `max_round_count` caps on the orchestration loop | `studies/agent-harness-study/sources/agent-framework/python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:475-477, 541-543` |
| Checkpoint storage abstraction (resume after failure) | `CheckpointStorage` Protocol (`save`/`load`/`list_checkpoints`/`get_latest`), `InMemoryCheckpointStorage`, `FileCheckpointStorage` implementations | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_workflows/_checkpoint.py:129-199, 202, 249` |
| Orchestrator state survives checkpoints | Magentic context serializes `stall_count`/`reset_count` and manager saves task ledger via `on_checkpoint_save` | `studies/agent-harness-study/sources/agent-framework/python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:345-355, 753-758` |
| Corrupt-state quarantine enables retry | `FileSessionStore._quarantine_corrupt_snapshot` moves an unchanged corrupt snapshot aside "so a retry can recover" | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_sessions.py:2054-2065` |
| Fallback models/providers — absent | Greps for `fallback.*model`, `model.*fallback`, backup/failover provider patterns over `python/packages/core`, `python/packages/*/agent_framework*`, `dotnet/src/Microsoft.Agents.AI*` found no runtime model-failover code; only unrelated fallbacks (serialization compat, lenient parsing) | search boundary noted in Answers below |
| Circuit breakers — absent | Grep for `circuit.break`, `half.open`, `breaker` over `python/packages/` and `dotnet/src/` matched only a comment about a "half-open client" HTTP state, not a breaker pattern | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/FoundryToolboxService.cs:622` |

## Answers to Dimension Questions

1. **Are retries configurable?**
   Partially, but inconsistently. Each subsystem exposes its own knob with different shapes: `progress_ledger_retry_count` (`_magentic.py:544`), `retry_attempts` on the group-chat orchestrator (`_group_chat.py:309`), `MaxProgressLedgerRetryCount` in .NET task limits (`MagenticTaskContext.cs:14`), and transport-level `ClientRetryPolicy(3)` configured on the Azure SDK client in samples (`ClawAgentFactory.cs:84`). There is **no top-level retry policy type** shared across the framework; the model-call path itself is only configurable by authoring custom middleware (`_middleware.py:552-561`) and wiring tenacity yourself (`auto_retry.py:116-142`). The Python OpenAI wrappers never pass `max_retries` to the underlying SDK (`_shared.py:235-246`), so default SDK behavior applies silently unless callers inject a pre-built client.

2. **Are fallback providers available?**
   No evidence found. I searched `python/packages/core/agent_framework/`, all provider packages (`openai/`, `foundry/`, `anthropic/`, `bedrock/`, `ollama/`, …), and `dotnet/src/` for fallback-model, model-failover, and backup-provider patterns. Provider/model selection happens exclusively at client-construction time (e.g., `OpenAISettings` model resolution, `_shared.py:217-233`). If a provider endpoint fails, the error propagates to the caller; surviving an outage requires the application to construct its own alternate client and swap it. The only "fallback" code paths are protocol-compatibility fallbacks (e.g., legacy-server fallback for augmented MCP task calls, `_mcp.py:2355-2359`), which serve wire-format tolerance, not availability.

3. **Does the system degrade gracefully?**
   Yes in specific surfaces, most concretely in .NET Foundry hosting: `FoundryToolboxStartupStatus.Degraded` keeps the container routable when toolbox enumeration fails at startup, surfacing deferred state through the readiness health check and retrying per request where per-user identity becomes available (`FoundryToolboxStartupStatus.cs:56-64`, `FoundryToolboxHealthCheck.cs:53-58`, `FoundryToolboxService.cs:334-404`). Multi-agent orchestrations degrade by design: Magentic resets and replans on ledger failure within stall/reset/round budgets (`MagenticOrchestrator.cs:266-281`, `_magentic.py:389-394, 541-543`) rather than aborting the run. At the single-agent/chat-client level there is no degradation mode — a provider outage fails the run outright unless the application adds retry middleware.

4. **Are circuit breakers used to prevent cascading failure?**
   No clear evidence found. Searches for circuit-breaker/half-open patterns across both language trees produced no breaker implementation. The nearest analogues are: the MCP proactive ping health check (`_mcp.py:1984-2021`) which prevents sending calls into dead connections, and the deferred-toolbox mechanism, which avoids repeated synchronous failure amplification by moving broken toolboxes out of the hot path until a later request. Neither implements open/half-open/closed state tracking, trip thresholds, or automatic recovery probing.

## Architectural Decisions

1. **Resilience as composition, not framework feature.** The middleware pipeline (`ChatMiddleware`/`AgentMiddleware`, `_middleware.py:535-566`) is the designated seam for cross-cutting retry logic, demonstrated by the auto-retry sample rather than baked into `BaseChatClient`. This keeps the core dependency-free (no tenacity/Polly) but shifts correctness burden onto every application.
2. **Idempotency-first retry safety for side-effecting tools.** The MCP long-running-task layer refuses blind re-sends when a request's outcome is unknown ("cannot safely retry (server may have started the operation)", `_mcp.py:2377-2379`) and restricts reconnect-retry to operations keyed by an existing `task_id` (`_mcp.py:2517-2546`). This is a deliberate trade of availability for exactly-once semantics.
3. **Fail-toward-routable in hosting.** The Foundry hosting health-check maps degraded states (`Degraded`, `ConsentRequired`, `NoEndpoint`) to Healthy so platform traffic continues, converting startup failures into per-request lazy resolution (`FoundryToolboxHealthCheck.cs:42-58`).
4. **Bounded self-healing loops in orchestration.** Rather than retrying indefinitely, Magentic tracks stall/reset counters serialized into checkpoints (`_magentic.py:345-355`) and terminates with a diagnostic message when limits are hit (`MagenticOrchestrator.cs:256-261`).
5. **Crash-resume delegated to checkpointing, not retry persistence.** Workflow checkpoints (`_checkpoint.py:129-199`) plus session-snapshot quarantine (`_sessions.py:2054-2065`) provide durable recovery points, while per-attempt retry counters remain ephemeral.

## Notable Patterns

- **Linear micro-backoff for parse retries**: both languages use `delay = 0.25s × attempt` around JSON-parse failures of model output (`_magentic.py:735`, `MagenticManager.cs:100`) — cheap jitter-free backoff tuned for nondeterministic-generation errors rather than network congestion.
- **Error-informed re-prompting**: the group-chat retry does not merely repeat the call; it feeds the failure text back to the selector model as a corrective instruction (`_group_chat.py:537-542`).
- **One-shot reconnect helper**: `_send_with_one_reconnect` centralizes "at most one reconnect" semantics for task operations (`_mcp.py:2525-2546`) with an explicit `AssertionError` unreachable guard.
- **Best-effort compensation**: abandoned remote tasks trigger fire-and-forget `tasks/cancel` (`_mcp.py:2573-2574`), pairing retries with cleanup of orphaned server-side work.
- **Cache-as-circuit-breaker-lite**: `CachingSkillsSource` serves stale data after failed refreshes (`_skills.py:3838-3840`), absorbing upstream flakiness without a formal breaker.

## Tradeoffs

- **Composition-over-convention**: zero core dependencies and maximal flexibility, at the cost that two applications of the same framework can have wildly different outage behavior; nothing enforces that any retry policy exists at all.
- **Exactly-once over availability (MCP)**: refusing ambiguous re-sends protects side-effecting tools but means a dropped pre-task response fails the call even though a retry would usually be safe for read-only tools — no per-tool idempotency opt-in exists.
- **Degraded-mode asymmetry between stacks**: the sophisticated routable-degraded machinery exists only in .NET Foundry.Hosting; the Python side has orchestration-level healing but no equivalent hosting-layer degradation story.
- **Sample-only canonical retry**: the tenacity-based pattern lives in `samples/02-agents/auto_retry.py`, so it can drift from framework internals; its streaming exclusion (`auto_retry.py:82-84`) leaves the most common production modality (streaming agents) without documented retry coverage.
- **Fragmented configuration vocabulary**: `progress_ledger_retry_count` vs `retry_attempts` vs `ClientRetryPolicy(n)` vs implicit OpenAI SDK defaults makes fleet-wide tuning error-prone.

## Failure Modes / Edge Cases

- **Total provider outage**: single-agent runs fail immediately with the provider error; without user-wired middleware there is no backoff, no failover, and no breaker — the dimension's guiding question ("survive a provider outage without failing all requests") is answered *no* at the chat-client tier.
- **Streaming + retry gap**: the documented retry approaches skip streaming; a mid-stream `RateLimitError` after partial tokens has no defined recovery path (`auto_retry.py:80-84`).
- **Silent SDK defaults**: Python OpenAI/Azure clients inherit the SDK's internal retry defaults without exposure or documentation in the wrapper (`_shared.py:235-246`), producing unobservable double-retry when users also add middleware (middleware wraps SDK retries → multiplicative attempt counts).
- **Backoff without jitter**: `0.25 * attempts` linear sleeps in Magentic managers are deterministic; many concurrent orchestrators would synchronize retry storms against the same model endpoint.
- **Unbounded poll loop risk contained by deadline**: `tasks/get` polling loops forever except for the `max_task_wait` deadline (`_mcp.py:2394-2405`); a misconfiguration of `None` (the default) removes the bound.
- **Quarantine-only corruption handling**: `FileSessionStore` quarantines syntactically malformed snapshots but schema/state-decoder failures preserve the original file, so persistently corrupt snapshots keep failing until manually removed (`_sessions.py:1968-1972, 2054-2065`).

## Future Considerations

- Ship an optional first-party retry policy (configurable predicate/backoff/jitter) installable as chat middleware, so the tenacity sample pattern becomes a supported default rather than a copy-paste recipe.
- Add fallback-model/provider chaining (e.g., ordered endpoint list with health tracking) — currently the single largest gap versus the dimension's survival question.
- Introduce a lightweight circuit-breaker decorator for chat clients and MCP tools, with trip metrics exposed via the existing OpenTelemetry integration.
- Extend the .NET Foundry degraded-mode pattern (deferred work + per-request retry + health reporting) to the Python hosting packages for parity.
- Expose and document the underlying SDK retry knobs (`max_retries` on Python OpenAI clients; `RetryPolicy` propagation already handled on .NET) and make retry/middleware interaction observable to avoid hidden multiplicative retries.
- Consider per-tool idempotency hints so safe (read-only) MCP tools can opt into aggressive reconnect-and-retry even before a `task_id` exists.

## Questions / Gaps

- No test coverage was found for the Python Magentic progress-ledger retry loop itself (`grep` over `python/packages/orchestrations/tests/test_magentic.py` for `retry` returned nothing); only MCP reconnect paths have direct tests (`test_mcp.py:1347, 4801-4818, 5367-5417`). The .NET `MagenticManager` retry similarly lacks a dedicated unit test in the inspected tree.
- Whether the Go implementation has retry support could not be assessed: `go/README.md:1-3` defers to the external `microsoft/agent-framework-go` repository, outside this source's isolation boundary.
- No evidence of retry-count telemetry/metrics (only warning logs and workflow warning events were observed); whether OTel spans capture attempt numbers was not verified.
- The `CachingSkillsSource` retry-on-next-call behavior is documented in docstrings (`_skills.py:3824-3840`) and asserted indirectly in skill tests, but no test name specifically pins the "failed fetch leaves cache empty" contract was identified during this pass.

---

Generated by `Dimension 13.02: Retry, Fallback, and Degraded Mode` against `agent-framework`.

# Source Analysis: pydantic-ai

## Dimension 13.02: Retry, Fallback, and Degraded Mode

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (pydantic-core, httpx2, tenacity, anyio; optional Temporal/DBOS/Prefect durable engines) |
| Analyzed | 2026-08-25 |

## Summary

Pydantic AI treats "retry" as five explicitly separated layers that do not share budgets: HTTP transport retries, model fallback, tool retries, output retries, and model-request hook retries (`docs/retries.md:5-15`). Transport-level retry/backoff is opt-in and built on tenacity via replayable HTTP transports plus a `Retry-After`-aware wait strategy (`pydantic_ai_slim/pydantic_ai/retries.py:140`, `retries.py:239`, `retries.py:514`). Provider outage survival is achieved through composition rather than one mechanism: a first-class `FallbackModel` wrapper tries a sequence of providers and never re-attempts the same one (`pydantic_ai_slim/pydantic_ai/models/fallback.py:86`), while semantic/model-visible corrections flow through `ModelRetry` exceptions converted into `RetryPromptPart` messages consumed against per-tool and output budgets (`pydantic_ai_slim/pydantic_ai/exceptions.py:57`, `pydantic_ai_slim/pydantic_ai/messages.py:1637`). There is no circuit breaker; the closest safeguards are `UsageLimits` runaway protection (`pydantic_ai_slim/pydantic_ai/usage.py:418`) and concurrency queue caps (`pydantic_ai_slim/pydantic_ai/exceptions.py:474`). Degradation is granular rather than a single mode: capability hooks may synthesize responses on model errors, pricing failures degrade to `None`, MCP tool errors are configurable as retry/error/failed, and native tools fall back to local implementations. Retry state is in-memory per run, persisted into message history (`RetryPromptPart`), and serialized across durable-execution boundaries.

## Rating

**8 / 10** — A clear, layered retry/failure model with explicit interfaces (`RetryConfig`, `FallbackModel.fallback_on`, `ModelRetry`/`ToolFailed`, `UsageLimits`), exceptional documentation depth (a dedicated taxonomy page mapping all five layers, `docs/retries.md`), extensive test coverage (~88 fallback-specific tests in `tests/models/test_fallback.py`, 35 transport-retry tests in `tests/test_tenacity.py`, ~250 retry-related tests repo-wide), and operational safeguards (budget enforcement, cost accounting across rejected fallback responses, span-attribute correction for observability). Not higher because: there is no circuit-breaker mechanism, transport retries default to off (deliberate, but means out-of-the-box provider flakiness fails runs), response-based fallback does not work for streaming (`docs/models/overview.md:411-412`), and retry budgets are not durable outside the opt-in durable-execution engines.

## Evidence Collected

Every entry cites file paths with line numbers relative to `studies/agent-harness-study/sources/pydantic-ai`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Five-layer retry taxonomy | Doc table separating transport, fallback, tool, output, hook retries with distinct budgets | `docs/retries.md:5-15` |
| Retry config type | `RetryConfig` TypedDict exposing tenacity `stop`/`wait`/`retry`/`before_sleep`/`reraise` knobs | `pydantic_ai_slim/pydantic_ai/retries.py:72-134` |
| Retrying sync transport | `HTTPX2TenacityTransport` wraps any transport with `@retry(**config)`; closes failed responses before retrying; documents non-replayable-stream limitation | `pydantic_ai_slim/pydantic_ai/retries.py:140-236` |
| Retrying async transport | `AsyncHTTPX2TenacityTransport` async counterpart | `pydantic_ai_slim/pydantic_ai/retries.py:239-339` |
| Backoff strategy | `wait_retry_after` honors integer-second and HTTP-date `Retry-After` values capped by `max_wait` (default 300s), falls back to exponential backoff `max=60` | `pydantic_ai_slim/pydantic_ai/retries.py:514-588` |
| Parsed retry-after on API errors | `ModelHTTPError.retry_after` property parses seconds/HTTP-date; headers normalized lowercase | `pydantic_ai_slim/pydantic_ai/exceptions.py:534-540`, `exceptions.py:580-609` |
| Providers raise structured HTTP errors | `_map_api_errors` wraps SDK `APIStatusError` ≥400 into `ModelHTTPError` with headers/body/suggested model id | `pydantic_ai_slim/pydantic_ai/models/openai.py:217-236`; also `models/anthropic.py:333`, `models/google.py:409`, `models/groq.py:94` |
| Transport retries off by default | "Nothing retries at this layer unless you install a retrying transport" | `docs/models/http-request-retries.md:19` |
| Fallback model | `FallbackModel(default_model, *fallback_models, fallback_on=(ModelAPIError,))` sequential chain | `pydantic_ai_slim/pydantic_ai/models/fallback.py:104-133` |
| Configurable trigger conditions | `fallback_on` accepts exception types, sync/async exception handlers, response handlers, or mixed sequences; handler kind auto-detected from first-param type hints | `pydantic_ai_slim/pydantic_ai/models/fallback.py:50-56`, `fallback.py:67-82`, `fallback.py:135-182` |
| Guard against empty policy | Empty `fallback_on` raises `UserError` instead of silently accepting everything | `pydantic_ai_slim/pydantic_ai/models/fallback.py:160-165` |
| All-models-failed surface | `FallbackExceptionGroup('All models from FallbackModel failed', ...)` aggregates exceptions plus `ResponseRejected` | `pydantic_ai_slim/pydantic_ai/models/fallback.py:544-554`; `pydantic_ai_slim/pydantic_ai/exceptions.py:612-613` |
| Streaming fallback | `request_stream` tries each model; mid-stream failures propagate (documented limitation for response-based triggers) | `pydantic_ai_slim/pydantic_ai/models/fallback.py:320-406`; `docs/models/overview.md:410-412` |
| Cost accounting under fallback | Rejected-but-billed responses accumulate cost onto the winning response's usage | `pydantic_ai_slim/pydantic_ai/models/fallback.py:296-307` |
| Continuation pinning & rewind | Suspended responses pin their model via `metadata['__pydantic_ai__']['fallback_model_id']`; pinned-continuation failure cancels the abandoned server-side job, rewinds history, and retries the chain with a `replace_previous_response` stamp | `pydantic_ai_slim/pydantic_ai/models/fallback.py:37-42`, `fallback.py:253-280`, `fallback.py:492-532` |
| Observability of actual model | `_set_span_attributes` refreshes OTel span `gen_ai.request.model` etc. to the inner model that actually answered | `pydantic_ai_slim/pydantic_ai/models/fallback.py:477-489` |
| SDK-retry interplay guidance | Docs recommend disabling provider SDK retries (`max_retries=0`) so fallback is immediate | `docs/models/overview.md:260` |
| Tool retry signal | `ModelRetry` exception carries a message returned to the model; serializable via pydantic core schema | `pydantic_ai_slim/pydantic_ai/exceptions.py:57-97` |
| Terminal-failure alternative | `ToolFailed` records `ToolReturnPart(outcome='failed')` without consuming the retry budget; bounded by `UsageLimits` instead | `pydantic_ai_slim/pydantic_ai/exceptions.py:100-119` |
| Retry prompt message part | `RetryPromptPart` renders validation errors or messages with `'Fix the errors and try again.'`; `from_error` is the canonical builder | `pydantic_ai_slim/pydantic_ai/messages.py:1637-1721` |
| Budget enforcement (output) | `GraphAgentState.consume_output_retry` increments `output_retries_used` and raises `UnexpectedModelBehavior('Exceeded maximum output retries (N)')` past budget | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:361-378` |
| Retry node construction | `_build_retry_node` appends a `RetryPromptPart`-only `ModelRequest`; hook-raised retries preserve the original response in history | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1439-1445`, `_agent_graph.py:1797-1810` |
| Budget enforcement (tools) | `ToolManager._check_max_retries` uses `>=` so negative budgets fail fast; raises actionable `UnexpectedModelBehavior` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:256-265` |
| Per-tool counters reset on success | `for_run_step` carries retries forward, dropping succeeded tools' counters and incrementing failed ones | `pydantic_ai_slim/pydantic_ai/tool_manager.py:187-220` |
| Configurable budgets | `Agent(retries=N)` or `retries={'tools': N, 'output': N}`; default 1; validated non-negative | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:170`, `agent/__init__.py:352-358`; `pydantic_ai_slim/pydantic_ai/tools.py:281-283` |
| Precedence ladder | Per-tool → per-toolset → override → per-run → spec → agent-wide → built-in default of 1 | `docs/tools-advanced.md:562-576` |
| Timeout-as-retry | Tool timeouts wrap execution in `anyio.fail_after` and raise `ModelRetry('Timed out after N seconds.')` | `pydantic_ai_slim/pydantic_ai/toolsets/function.py:684-691`; documented caveat `docs/timeouts.md:27-30` |
| Runaway-loop safeguard | `UsageLimits.request_limit` defaults to 50 and is checked before each request; token/cost limits checked after responses | `pydantic_ai_slim/pydantic_ai/usage.py:418-547` |
| Load shedding | `ConcurrencyLimitExceeded` raised when concurrency queue depth exceeds `max_queued` | `pydantic_ai_slim/pydantic_ai/exceptions.py:474-475` |
| Error-suppression extension point | `AbstractCapability.on_model_request_error` may return a synthetic `ModelResponse` to suppress provider errors, or raise `ModelRetry` to convert them into prompts | `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:724-745` |
| Hook-driven rejection | `after_model_request` raising `ModelRetry` rejects a produced response while keeping it in history | `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:694-707` |
| Pricing never fails a run | `best_effort_price` degrades lookup failures to `None` (expected) or `CostCalculationFailedWarning` (unexpected) | `pydantic_ai_slim/pydantic_ai/_cost.py:62-95` |
| MCP tool error policy | `MCPToolset.tool_error_behavior='retry'\|'error'\|'failed'` maps server errors to retry prompt, raise, or failed result (default `'retry'`) | `pydantic_ai_slim/pydantic_ai/mcp.py:764`, `mcp.py:875`, `mcp.py:1387-1391` |
| Native→local tool fallback | `NativeOrLocalTool` pairs provider-native tools with local function-tool fallbacks; unsupported natives without fallback raise an actionable error | `pydantic_ai_slim/pydantic_ai/capabilities/native_or_local.py:19-54`; `pydantic_ai_slim/pydantic_ai/models/__init__.py:1855-1868` |
| Retry state across durable boundaries | Temporal run context serializes `ctx.retries`, `retry`, `max_retries` into activities; `ModelRetry` has a wire variant `_ModelRetry(kind='model_retry')` round-tripped by `wrap_tool_call_result` | `pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_run_context.py:147-153`; `pydantic_ai_slim/pydantic_ai/durable_exec/_toolset.py:158-190` |
| History-persisted retries | Retry prompts remain in message history and replay to the model on later runs unless filtered via `ProcessHistory` | `docs/retries.md:43-97` |
| Test coverage: fallback | 88 tests spanning handlers, streaming, instrumentation, continuation pinning, rewind recovery, lifecycle/concurrency | `tests/models/test_fallback.py:105-3347` (e.g. `test_response_handler_triggered`:1413, `test_fallback_streaming_pinned_continuation_fails_falls_back`:2909) |
| Test coverage: transports/wait | 35 tests: validator-triggered retries, exhaustion, `Retry-After` seconds/date/max-wait/case-insensitivity, deprecation warnings | `tests/test_tenacity.py:59-789` |

## Answers to Dimension Questions

**1. Are retries configurable?**
Yes, at every layer. Transport retries accept full tenacity semantics through `RetryConfig` (`pydantic_ai_slim/pydantic_ai/retries.py:72-134`) including custom stop/wait/retry predicates and sleep functions. Tool and output budgets are configurable agent-wide, per-run, per-block (`override`), per-toolset, and per-tool, with a documented precedence ladder and built-in default of 1 (`docs/tools-advanced.md:562-576`; `pydantic_ai_slim/pydantic_ai/agent/__init__.py:352-358`). Negative values are rejected at construction (`pydantic_ai_slim/pydantic_ai/tools.py:281-283`). One nuance: transport retries require installing the `[retries]` extra and wiring a transport yourself — nothing retries by default below the agent loop (`docs/models/http-request-retries.md:14-19`).

**2. Are fallback providers available?**
Yes — `FallbackModel` composes any number of named or instantiated models into a sequential chain (`pydantic_ai_slim/pydantic_ai/models/fallback.py:104-127`), triggered by default on `ModelAPIError` (covering `ModelHTTPError` 4xx/5xx) and customizable to arbitrary exception predicates or even response-content inspection via `fallback_on` handlers (`pydantic_ai_slim/pydantic_ai/models/fallback.py:108`, `fallback.py:135-182`). Per-model `ModelSettings` are supported for heterogeneous chains (`docs/models/overview.md:313-332`). Validation failures deliberately do *not* trigger fallback — they use same-model retries instead (`docs/models/overview.md:404`). Exhausting every model raises `FallbackExceptionGroup` aggregating all causes (`pydantic_ai_slim/pydantic_ai/models/fallback.py:544-554`). Limitations: response-based (semantic) fallback works only for non-streaming requests (`docs/models/overview.md:410-412`).

**3. Does the system degrade gracefully?**
Largely yes, though there is no single "degraded mode" flag. Mechanisms observed: capability hooks can suppress model-request errors by returning synthetic responses (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:724-745`); tool timeouts degrade into model-visible retry prompts (`pydantic_ai_slim/pydantic_ai/toolsets/function.py:690-691`); MCP tool errors are policy-configurable between retry/error/failed (`pydantic_ai_slim/pydantic_ai/mcp.py:764`); pricing failures never fail a run (`pydantic_ai_slim/pydantic_ai/_cost.py:62-95`); native tools without provider support either fall back to local implementations or raise actionable configuration errors listing the missing install group (`pydantic_ai_slim/pydantic_ai/models/__init__.py:1855-1868`); and `ToolFailed` distinguishes terminal failures from retryable ones so the model adapts instead of looping (`pydantic_ai_slim/pydantic_ai/exceptions.py:100-112`). Even rejected fallback responses keep their billed cost in the final accounting (`pydantic_ai_slim/pydantic_ai/models/fallback.py:296-307`), which prevents silent cost loss under degradation.

**4. Are circuit breakers used to prevent cascading failure?**
No evidence found. A repo-wide search for `circuit` across `pydantic_ai_slim/pydantic_ai` returned no breaker-like implementation (searches covered class names, comments, and docstrings). The protective mechanisms that exist are different in kind: `UsageLimits` bounds total requests/tokens/cost per run with a default `request_limit=50` (`pydantic_ai_slim/pydantic_ai/usage.py:418-496`), `ConcurrencyLimitExceeded` sheds load when queue depth exceeds `max_queued` (`pydantic_ai_slim/pydantic_ai/exceptions.py:474-475`), and `FallbackModel` stops at the first healthy provider rather than tracking health over time. There is no shared state that trips a provider "open" after repeated failures across runs; each run starts probing the first model again.

## Architectural Decisions

1. **Explicit layer separation with independent budgets.** Rather than one retry knob, the framework names five layers and states they "don't share budgets" (`docs/retries.md:3-15`). This avoids the classic failure where transport retries multiply with agent retries into unbounded loops, and it makes cost/latency reasoning per layer possible.
2. **Fallback is modeled as a `Model` wrapper, not an agent feature.** `FallbackModel` implements the same `Model` interface (`request`, `request_stream`) as any provider adapter (`pydantic_ai_slim/pydantic_ai/models/fallback.py:229`, `fallback.py:320`), making fallback orthogonal to agents, usable with `direct` model calls, and stackable/composable.
3. **Retries as conversation, not as loops.** Tool/output/hook retries append `RetryPromptPart`s to history so the model self-corrects with context (`pydantic_ai_slim/pydantic_ai/messages.py:1637-1650`), bounded by budgets that raise `UnexpectedModelBehavior` when exhausted (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:374-378`). The model, not a blind scheduler, decides what to change between attempts.
4. **Opt-in transport resilience.** Tenacity lives behind an optional dependency group and user-supplied clients (`pydantic_ai_slim/pydantic_ai/retries.py:34-45`), keeping the slim package dependency-light and letting users pick retryable status codes (`docs/models/http-request-retries.md:43-64`).
5. **Stateless continuation routing for suspended responses.** Fallback chains stamp routing metadata into response `metadata['__pydantic_ai__']` and rewind cleanly on pinned-model failure, cancelling abandoned server-side jobs first to avoid duplicate billing (`pydantic_ai_slim/pydantic_ai/models/fallback.py:262-274`, `fallback.py:492-501`).
6. **Durability as a compatibility target.** The durable-execution guidelines require preserving "message history, retries" across engine boundaries (`pydantic_ai_slim/pydantic_ai/durable_exec/AGENTS.md`), realized by serializing retry counters into activity contexts (`pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_run_context.py:147-153`) and giving `ModelRetry` both a pydantic schema (`pydantic_ai_slim/pydantic_ai/exceptions.py:81-97`) and a durable wire shape (`pydantic_ai_slim/pydantic_ai/durable_exec/_toolset.py:174-177`).

## Notable Patterns

- **Header-honoring backoff**: both the transport wait strategy (`wait_retry_after`, `pydantic_ai_slim/pydantic_ai/retries.py:562-588`) and the exception surface (`ModelHTTPError.retry_after`, `pydantic_ai_slim/pydantic_ai/exceptions.py:580-609`) parse `Retry-After` in seconds *and* HTTP-date form with caps, respecting provider rate-limit contracts.
- **Handler auto-detection by type hints**: `fallback_on` inspects the first parameter's annotation to distinguish exception handlers from response handlers, with runtime-import errors surfaced as `UserError` rather than silent misclassification (`pydantic_ai_slim/pydantic_ai/models/fallback.py:67-82`; `docs/models/overview.md:424-426`).
- **Counter hygiene**: per-tool retry counters reset on success and live in a dict keyed by tool name, so alternating fail/succeed patterns don't starve a budget of 1 (`pydantic_ai_slim/pydantic_ai/tool_manager.py:193-203`; `docs/retries.md:35`); availability refusals are deliberately kept out of the retry budget (`pydantic_ai_slim/pydantic_ai/tool_manager.py:159-166`).
- **Observability-correct fallback**: spans are patched to attribute the response to whichever inner model actually served it (`pydantic_ai_slim/pydantic_ai/models/fallback.py:477-489`), and dedicated instrumented tests verify the failing model is recorded (`tests/models/test_fallback.py:563`, `tests/models/test_fallback.py:699`).
- **Negative-budget hardening**: retry checks use `>=` so a misconfigured negative limit terminates immediately instead of looping forever (`pydantic_ai_slim/pydantic_ai/tool_manager.py:259-261`).
- **Tested failure-path documentation**: docs examples like the retry-prompt history walkthrough are executable and snapshot-tested (`docs/retries.md:45-91`).

## Tradeoffs

- **No default transport resilience.** Out of the box, a transient 503 fails the run; users must construct a retrying client per provider (`docs/models/http-request-retries.md:19`, `docs/models/http-request-retries.md:27-70`). This keeps dependencies slim but shifts correctness burden to users, who must also remember to disable SDK-internal retries to avoid double-retrying ahead of fallback (`docs/models/overview.md:260`).
- **Streaming gaps in semantic fallback.** Response-content-based `fallback_on` cannot save a stream that already emitted events — only connection/request-time exceptions fall back (`pydantic_ai_slim/pydantic_ai/models/fallback.py:330-333`).
- **Per-tool counters can be gamed by hallucination.** An unknown tool name gets its own budget keyed under the invented name, so a model inventing a fresh wrong name each step keeps earning new budgets, bounded only by the agent-wide `tools` budget and `UsageLimits` (`docs/retries.md:37`).
- **Timeouts are cooperative, not preemptive.** Sync tools run to completion in worker threads despite the deadline firing; a user-thrown `TimeoutError` is indistinguishable from the enforced deadline and becomes a possibly-false "Timed out" retry prompt (`docs/timeouts.md:12`, `docs/timeouts.md:29-30`).
- **No cross-run failure memory.** Without circuit breaking, every new run re-probes the primary provider, adding latency during sustained outages (mitigated only by user-side transport retries or manual model choice).

## Failure Modes / Edge Cases

- **All fallbacks exhausted**: surfaces as `FallbackExceptionGroup` containing every underlying exception plus a `ResponseRejected` sentinel when handlers rejected content (`pydantic_ai_slim/pydantic_ai/models/fallback.py:544-554`), preserving root causes for diagnosis.
- **Budget exhaustion mid-parallel-output**: when multiple output tools race, max-retry errors are captured per call and only raised if no sibling produced a valid winner, otherwise absorbed as skip-status parts (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:489-503`, `_tool_execution.py:570-578`).
- **Truncated tool calls on token limits**: `check_incomplete_tool_call` converts a length-truncated tool-call response into `IncompleteToolCall` with guidance instead of an opaque validation error (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:345-359`).
- **Suspended-job leaks**: pinned continuations that fail cancel the orphaned background job best-effort before rewinding; cancellation without a resolvable pin fans out to all inner models, relying on strict per-model guards to avoid false cancels (`pydantic_ai_slim/pydantic_ai/models/fallback.py:408-431`).
- **Expired suspended jobs**: resuming beyond provider retention raises `SuspendedResponseExpired` advising a clean restart rather than an opaque provider error (`pydantic_ai_slim/pydantic_ai/exceptions.py:449-456`).
- **Unretryable request bodies**: streamed (non-replayable) bodies raise `httpx2.StreamConsumed` on retry attempts; the limitation is documented at both transports (`pydantic_ai_slim/pydantic_ai/retries.py:151-153`, `retries.py:250-252`).

## Future Considerations

- **Circuit breaker / health tracking**: a stateful wrapper analogous to `FallbackModel` could trip providers open after N consecutive `ModelAPIError`s within a window, eliminating cold-start latency during sustained outages; the existing wrapper-model pattern (`pydantic_ai_slim/pydantic_ai/models/wrapper.py`) is a natural host.
- **Streaming-safe semantic fallback**: buffering the first chunk(s) until a response-handler verdict would extend response-based fallback to `run_stream`.
- **Global tool-call budget option**: an optional run-wide tool-retry ceiling alongside per-tool counters would close the hallucinated-name loophole (`docs/retries.md:37`).
- **Default-on conservative transport retry** (e.g., idempotent GET/count-tokens paths) could reduce first-run flakiness without violating the slim-package philosophy.

## Questions / Gaps

- **Is retry state ever persisted to disk by the framework itself?** No evidence found outside durable engines. Search boundary: `grep -rn "persist|checkpoint|snapshot"` concepts were traced through `GraphAgentState` (in-memory dataclass, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:299-343`), `RunContext.retries` (`pydantic_ai_slim/pydantic_ai/_run_context.py:97-114`), and durable-exec serialization (`pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_run_context.py:147`). Within a plain process, budgets die with the run; with Temporal/DBOS/Prefect, counters and history survive replay/recovery. Users wanting cross-process retry memory without those engines get no framework support.
- **Does `FallbackModel` coordinate with `UsageLimits` to avoid double-charging probes?** Partially: rejected responses' costs are folded into the winning usage (`pydantic_ai_slim/pydantic_ai/models/fallback.py:303-307`), but request-count accounting for failed probes was not examined in depth here.
- **Rate-limit-specific fallback**: no built-in shortcut maps 429s to a different provider with `Retry-After` honored before switching; users compose this from `fallback_on=(lambda e: isinstance(e, ModelHTTPError) and e.status_code == 429)` plus transport retries. Absence confirmed by search; not documented as a provided recipe.

---

Generated by `dimensions/13.02-retry-fallback-and-degraded-mode` against `pydantic-ai`.

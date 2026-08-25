# Source Analysis: opa

## Dimension 13.02: Retry, Fallback, and Degraded Mode

### Source Info

| Field | Value |
|-------|-------|
| Name | opa (Open Policy Agent) |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | Go (policy engine; HTTP server, plugins, Rego evaluator, WASM SDK) |
| Analyzed | 2026-08-25 |

## Summary

OPA is not an LLM agent harness, so "fallback models/providers" does not apply in the model-routing sense. Instead, resilience is implemented around four long-running loops — bundle download (`v1/download/download.go`), OCI bundle download (`v1/download/oci_download.go`), decision-log upload (`v1/plugins/logs/plugin.go`), and status upload (`v1/plugins/status/plugin.go`) — plus the policy-author-controlled `http.send` builtin (`v1/topdown/http.go`).

Retry behavior follows one repeated pattern: an infinite loop that on failure waits using a shared exponential-backoff helper (`v1/util/backoff.go:14-16`, gRPC-style factor 1.6 with ±20% jitter) capped by the configured `max_delay_seconds`, and resets the retry counter after any success. Configuration is split: poll/upload delays are user-configurable (`min_delay_seconds`/`max_delay_seconds`, `v1/download/config.go:27-33`), while base backoff delay and caps are hard-coded constants (e.g., `minRetryDelay = 100ms`, `v1/download/download.go:32-34`; `maxRetryDelay = 60s` for `http.send`, `v1/topdown/http.go:54-58`).

Fallbacks exist at the protocol level, not the provider level: long-polling degrades to periodic polling when the server does not advertise support (`v1/download/download.go:235-245`, `isLongPollSupported` at `v1/download/download.go:437-439`); oversized decision-log events are re-encoded without the non-deterministic builtins cache before being dropped (`v1/plugins/logs/encoder.go:173-198`). There are no circuit breakers anywhere in the codebase. Degradation is instead handled structurally: OPA keeps serving the last-known-good policy when a bundle fails to download or activate (`v1/plugins/bundle/plugin.go:513-544`, verified by test at `v1/plugins/bundle/plugin_test.go:3994-4000`), bounded buffers drop telemetry oldest-first with metric counters rather than blocking decisions (`v1/plugins/logs/buffer.go:32-57`), and plugin health is surfaced through ERROR/WARN states (`v1/plugins/plugins.go:129-146`). Retry counters themselves are in-memory loop locals; only bundle ETags (in the store) and persisted bundles (on disk) survive restarts.

**Can the system survive a provider outage without failing all requests? Yes.** A bundle-service outage never affects query evaluation: downloads retry forever with jittered exponential backoff while the previously activated bundle keeps serving, and decision logs buffer in memory (dropping under pressure) rather than failing decisions. The main durability gap is that buffered-but-unuploaded decision logs are lost on process restart.

## Rating

**7 / 10** — Clear, consistent retry model with tests, explicit interfaces, and real operational safeguards (last-known-good activation, bounded buffers with drop observability, graceful shutdown flush). It misses 8+ because there are no circuit breakers, no multi-endpoint/service failover, the backoff timing loops themselves lack direct unit tests, base delays/caps are hard-coded per subsystem, and the decision-log buffer is memory-only.

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Shared backoff algorithm | `DefaultBackoff(base, maxNS, retries)` = gRPC-style exponential backoff (factor 1.6, jitter ±0.2), capped at max, 0 for `retries == 0`; documented as "Same algorithm used in gRPC" | `v1/util/backoff.go:12-43` |
| Backoff shim (v0 API) | Public v0 wrapper delegating to `v1` implementation | `util/backoff.go:13-22` |
| Bundle downloader retry loop | On error: `delay = util.DefaultBackoff(minRetryDelay, parsedMaxDelaySeconds, retry)`; on success: uniform random delay in `[min,max]`; `retry++` only after a failed wait, reset to 0 on success | `v1/download/download.go:215-262` |
| Downloader hard-coded base delay | `minRetryDelay = time.Millisecond * 100` | `v1/download/download.go:32-34` |
| Polling config | `PollingConfig{MinDelaySeconds, MaxDelaySeconds, LongPollingTimeoutSeconds}`; defaults 60s/120s; validation requires `max >= min` | `v1/download/config.go:15-33, 57-71` |
| OCI downloader retry | Identical loop shape to HTTP downloader: backoff on error, jittered poll interval on success | `v1/download/oci_download.go:194-233` |
| Decision-log uploader retry | Same pattern in `Plugin.loop()`: `util.DefaultBackoff(minRetryDelay, MaxDelaySeconds, retry)` on upload error; success uses random delay in `[MinDelaySeconds, MaxDelaySeconds]` | `v1/plugins/logs/plugin.go:902-961` |
| Decision-log reporting config | `ReportingConfig{BufferType, BufferSizeLimitBytes, BufferSizeLimitEvents, UploadSizeLimitBytes, MinDelaySeconds, MaxDelaySeconds, MaxDecisionsPerSecond, Trigger}`; defaults 300s/600s, 10000 events | `v1/plugins/logs/plugin.go:262-293, 349-380` |
| Shutdown flush retry | `flushDecisions` retries `p.b.Upload(ctx)` every 1s until context deadline; logs "Plugin stopped with decisions possibly still in buffer." if it gives up | `v1/plugins/logs/plugin.go:686-713` |
| Requeue-on-failure semantics | Failed chunks stay queued; docs state OPA requeues the last chunk and applies exponential backoff on non-2xx | `docs/docs/management-decision-logs.md:86-89` |
| `http.send` retry option | `max_retry_attempts` in allowed request keys; retried with `util.DefaultBackoff(100ms, 60s, i)`; context cancellation aborts the wait | `v1/topdown/http.go:90, 54-58, 699-735` |
| `http.send` retry validation & tests | Invalid/negative/fractional retry params rejected (`TestInvalidRetryParam`); network-error retry exercised end-to-end (`TestHTTPSendRetryRequest`) | `v1/topdown/http_test.go:613-624`; `v1/topdown/http_slow_test.go:126-174` |
| Long-poll → periodic fallback | If server response lacks bundle content type, `longPollingEnabled` is cleared and the loop falls back to min/max-delay polling | `v1/download/download.go:235-245, 276-277, 437-439` |
| Long-poll fallback test | `TestStartStopWithLongPollNotSupported` verifies polling continues when long-poll unsupported | `v1/download/download_test.go:251-281` |
| Long-poll fallback documented | "If the server does not support `long polling`, OPA will fallback to the regular periodic polling." | `docs/docs/management-bundles/index.md:202-204` |
| Encoder degradation (ND-cache drop) | Oversized event: first try as-is; then drop `nd_builtin_cache` and re-encode; only then drop event, incrementing `decision_logs_encoding_failure` | `v1/plugins/logs/encoder.go:152-213` |
| ND-cache drop documented | Docs describe dropping the key and retrying chunk encoding as best-effort | `docs/docs/management-decision-logs.md:109-113` |
| Bounded FIFO buffer (drop-oldest) | `logBuffer.Push` drops front elements when limit exceeded; returns dropped count; oversized single item dropped immediately | `v1/plugins/logs/buffer.go:11-57` |
| Drop observability | Counters: `decision_logs_dropped_rate_limit_exceeded`, `decision_logs_dropped_buffer_size_limit_exceeded`, `decision_logs_dropped_buffer_size_limit_bytes_exceeded`, `decision_logs_encoding_failure` | `v1/plugins/logs/plugin.go:274-277` |
| Rate limiting (backpressure) | Optional `rate.Limiter` from `MaxDecisionsPerSecond`; over-limit events dropped with error log, never blocking evaluation | `v1/plugins/logs/eventBuffer.go:155-170`; `v1/plugins/logs/sizeBuffer.go:114-117` |
| Buffer drop tests | `TestEventBuffer_Push`, `TestLogBufferDropsMultipleItems`, `TestLogBufferOversizedItem` verify drop counts via metrics | `v1/plugins/logs/eventBuffer_test.go:27-131`; `v1/plugins/logs/buffer_test.go:12-92` |
| Graceful shutdown flush test | `TestPluginGracefulShutdownFlushesDecisions` | `v1/plugins/logs/plugin_test.go:2020` |
| Plugin degraded-state vocabulary | `StateNotReady`/`StateOK`/`StateErr`/`StateWarn` — WARN explicitly means "operating, but in a potentially dangerous or degraded state" | `v1/plugins/plugins.go:125-146` |
| Last-known-good policy retention | On download or activation failure, `SetError` records failure but previously activated policies remain in store; ETag cache restored so next poll re-requests correctly | `v1/plugins/bundle/plugin.go:513-544`; `v1/plugins/bundle/status.go:63-98` |
| Retention verified by test | Test asserts previous revision `quickbrownfaux-2` still active after failed activation of `quickbrownfaux-3` | `v1/plugins/bundle/plugin_test.go:3985-4000` |
| Persisted-bundle startup recovery | `loadAndActivateBundlesFromDisk` loads last persisted bundle at startup and retries activation up to `maxActivationRetry` | `v1/plugins/bundle/plugin.go:378-435` |
| Activation retry cap | `maxActivationRetry = 10` with rationale comment (inter-bundle dependencies; bound wasted time) | `v1/plugins/bundle/plugin.go:34-44`; same constant in discovery: `v1/plugins/discovery/discovery.go:44-48, 269-301` |
| Bundle persistence to disk | Successful snapshot bundles saved under persist dir; used on restart | `v1/plugins/bundle/plugin.go:546-560`; discovery equivalent: `v1/plugins/discovery/discovery.go:308-319` |
| ETag persistence (retry-adjacent state) | `WriteEtagToStore` called on activation; bundle plugin re-reads ETag from store at init to resume conditional polling after restart | `v1/bundle/store.go:134-135, 850`; `v1/plugins/bundle/plugin.go:350-376` |
| Status plugin: no retry | Event-driven `oneShot` uploads; failures logged but not retried (no backoff loop exists here) | `v1/plugins/status/plugin.go:351-409` |
| REST client: no transport-level retry | No retry/backoff logic in shared plugin REST client | `v1/plugins/rest/rest.go` (searched `retry|backoff`: no matches) |
| WASM SDK loader retry | Infinite retry loop with `defaultBackoff(MinRetryDelay=100ms, l.maxDelay, retry)`; only `context.Canceled` escapes | `internal/wasm/sdk/opa/loader/http/loader.go:23-25, 154-178`; backoff impl: `internal/wasm/sdk/opa/loader/http/util.go:12-42` |
| Buffered logger discard fallback | `ResolveBufferedLogger(fallback)`: flush buffered logs to fallback logger or discard; SDK passes nil ("otherwise discard (no fallback)") | `v1/plugins/plugins.go:1265-1293`; `v1/sdk/opa.go:275-277` |
| Credential chain fallback (authn) | AWS/custom auth credential providers fall back through configured sources (test: "Fallback to Environment Credential") — config resolution, not request retry | `v1/plugins/rest/rest_test.go:1883` |

## Answers to Dimension Questions

### 1. Are retries configurable?

**Partially.** What is configurable:

- Poll intervals for bundle/OCI/status downloads: `min_delay_seconds`, `max_delay_seconds`, `long_polling_timeout_seconds` (`v1/download/config.go:27-33`), validated at `v1/download/config.go:57-71`.
- Decision-log reporting cadence: `min_delay_seconds`, `max_delay_seconds` plus buffer sizing (`v1/plugins/logs/plugin.go:283-293`).
- Per-call retry count for policy-authored HTTP: `max_retry_attempts` on `http.send` (`v1/topdown/http.go:90`).

What is NOT configurable: the backoff curve itself. Base delay (100 ms) and caps (60 s for `http.send`, `parsedMaxDelaySeconds` otherwise) are hard-coded per call site (`v1/download/download.go:32-34`, `v1/topdown/http.go:54-58`, `v1/plugins/logs/plugin.go:264`), and jitter/factor (0.2/1.6) live inside `v1/util/backoff.go:15`. There is no single retry-policy type or interface; each subsystem re-implements the same ~20-line loop (download.go:218-261, oci_download.go:197-232, logs/plugin.go:905-945).

### 2. Are fallback providers available?

**No provider/model fallbacks — protocol-level fallbacks only.** OPA has exactly one upstream service per plugin role: decision-log config resolves to a single `service` name (`v1/plugins/logs/plugin.go:329-341`), and there is no automatic failover to a second endpoint. Searches for `fallback` across Go sources found no alternate-provider machinery (hits were AST/planner internals like `v1/ast/env.go:158` and UI components). The real fallback behaviors are:

- Long-polling degrades to periodic polling based on the server's response content type (`v1/download/download.go:276-277, 437-439`).
- Decision-log encoding degrades by shedding the `nd_builtin_cache` field before dropping the event (`v1/plugins/logs/encoder.go:178-198`).
- Custom log plugins (`Logger`/`LoggerPlugin` interfaces, `v1/plugins/logs/plugin.go:806-818`) let deployments substitute their own durable sinks, which is the supported answer for provider failover.
- Auth credentials resolve through ordered provider chains with environment fallback (`v1/plugins/rest/rest_test.go:1883`), but this is configuration resolution, not request-time failover.

### 3. Does the system degrade gracefully?

**Yes — this is the strongest resilience property in the codebase.**

- **Policy availability is decoupled from bundle delivery.** Download or activation failures set plugin status (`v1/plugins/bundle/plugin.go:513-521, 536-544`) but leave the currently active bundle serving traffic; a test pins this exact behavior (`v1/plugins/bundle/plugin_test.go:3994-4000`).
- **Restart survival**: bundles with `persist` enabled are written to disk after successful activation (`v1/plugins/bundle/plugin.go:546-560`) and reloaded at startup with up-to-10 activation attempts to handle inter-bundle dependency ordering (`v1/plugins/bundle/plugin.go:378-435, 34-44`).
- **Telemetry sheds load before availability**: decision-log buffers are bounded FIFOs that drop oldest-first (`v1/plugins/logs/buffer.go:40-49`), optional rate limiting drops excess events without blocking queries (`v1/plugins/logs/eventBuffer.go:162-166`), and every drop path increments a named counter (`v1/plugins/logs/plugin.go:274-277`).
- **Explicit degraded-state signaling**: plugins expose `NOT_READY`/`OK`/`ERROR`/`WARN` states where WARN denotes "operating, but in a potentially dangerous or degraded state" (`v1/plugins/plugins.go:142-145`).
- **Shutdown flush**: on Stop with a deadline, buffered decisions are drained with 1 s retry intervals, and the "decisions possibly still in buffer" warning marks partial loss honestly (`v1/plugins/logs/plugin.go:686-713`).

### 4. Are circuit breakers used to prevent cascading failure?

**No explicit circuit breakers.** A search for `circuit` returned only short-circuit boolean-evaluation semantics (e.g., `v1/topdown/eval.go:4438, 4497`) — nothing implementing failure-rate-based open/half-open/closed states. Mitigation against cascading failure is achieved indirectly:

- Unbounded retry is impossible where caps exist: bundle/discovery activation retries stop at `maxActivationRetry = 10` (`v1/plugins/bundle/plugin.go:44`; `v1/plugins/discovery/discovery.go:48`), and `http.send` stops at the author-supplied attempt count (`v1/topdown/http.go:717-719`).
- Jittered backoff prevents synchronized hammering of a recovering service (`v1/util/backoff.go:36-38`).
- Memory pressure is bounded by buffer limits and rate limiting rather than queueing unboundedly (`v1/plugins/logs/buffer.go:35-50`).
- However, the bundle/log downloader loops retry indefinitely (bounded only by the 60 s–style max delay cap), and the status plugin performs fire-and-forget uploads with no protection against repeated failure storms beyond logging (`v1/plugins/status/plugin.go:359-405`).

## Architectural Decisions

1. **One shared backoff primitive, many bespoke loops.** The math lives in `v1/util/backoff.go:14-43` (and a near-identical copy for the WASM SDK at `internal/wasm/sdk/opa/loader/http/util.go:12-42` — duplication, likely for module isolation), but scheduling, counter reset semantics, and context handling are re-implemented per consumer (`v1/download/download.go:215-262`, `v1/plugins/logs/plugin.go:902-961`, `v1/topdown/http.go:708-733`). This trades some consistency risk for per-loop flexibility (e.g., reconfigure-aware timers in the log uploader).

2. **Retry on error, poll on success.** All downloader-style loops distinguish failure (exponential backoff toward `max_delay_seconds`) from success (uniform random jitter in `[min,max]` seconds) — e.g., `v1/download/download.go:232-245`. This doubles as thundering-herd protection for fleet-wide polls.

3. **Last-known-good semantics over fail-stop.** Bundles activate transactionally into the store; failures never roll back the active revision (`v1/plugins/bundle/plugin.go:536-544`). Availability of policy evaluation is treated as strictly more important than freshness.

4. **Telemetry is best-effort by design.** Decision-log delivery never back-pressures policy evaluation; overflow is absorbed by dropping with metrics (`v1/plugins/logs/buffer.go:11-13`), and encoding degrades by shedding optional payload fields first (`v1/plugins/logs/encoder.go:153-176`).

5. **Durable state limited to what restart needs.** Only ETags (`v1/bundle/store.go:134-135`) and persisted bundles reach disk; retry counters and log buffers stay in memory. Restart recovery therefore optimizes for correctness (resume conditional GETs, reload last-known-good policy), not for zero-telemetry-loss.

## Notable Patterns

- **Reset-on-success retry counters**: `retry = 0` after any successful cycle in all loops (`v1/download/download.go:252-256`, `v1/plugins/logs/plugin.go:934-938`, `v1/download/oci_download.go:222-227`) — backoff measures consecutive failures, not total age.
- **Requeue-not-drop on upload failure**: failed decision-log chunks remain in the buffer for the next cycle (`docs/docs/management-decision-logs.md:86-89`), with `TestPluginRequeueBufferPreserved` pinning the behavior (`v1/plugins/logs/plugin_test.go:741`).
- **Graceful degradation ladder in encoding**: fit as-is → drop ND cache → drop event, each step observable (`v1/plugins/logs/encoder.go:152-213`).
- **Protocol capability probing**: `Prefer: modes=snapshot,delta` and long-poll negotiation with automatic downgrade (`v1/download/download.go:292-309, 437-439`).
- **Status surfacing as the observability contract**: every failure path calls `status.SetError`, classifying compile vs HTTP vs other errors (`v1/plugins/bundle/status.go:63-98`), consumed by the status plugin and `/v1/plugins` API.
- **Test-guarded resilience claims**: drop accounting (`v1/plugins/logs/buffer_test.go:58-92`), retry parameter validation (`v1/topdown/http_test.go:613-624`), retry-under-network-failure (`v1/topdown/http_slow_test.go:126-174`), shutdown flush (`v1/plugins/logs/plugin_test.go:2020`), and failed-activation retention (`v1/plugins/bundle/plugin_test.go:3994-4000`).

## Tradeoffs

- **Indefinite retry vs. circuit breaking**: the eternal retry loops guarantee eventual convergence after outages but can mask a permanently broken endpoint (only per-plugin status reveals it). A circuit breaker would fail faster and reduce useless load, at the cost of more state machinery.
- **In-memory log buffer vs. durability**: bounded memory protects the eval hot path (`v1/plugins/logs/buffer.go:11-13`) but silently loses buffered decisions on crash/restart; the shutdown-flush warning (`v1/plugins/logs/plugin.go:710`) acknowledges this. Durable disk spooling is left to custom `plugin` sinks (`v1/plugins/logs/plugin.go:800-818`).
- **Hard-coded backoff constants vs. configurability**: operators can tune poll cadence but not escalation speed; a slow-recovering service always waits ≥100 ms × 1.6^n regardless of workload.
- **Single-service binding vs. failover simplicity**: one service per plugin keeps config and auth simple (`v1/plugins/logs/plugin.go:329-341`) but makes HA dependent on the endpoint's own load balancer, not OPA.
- **Code duplication for module isolation**: `v1/util/backoff.go` vs `internal/wasm/sdk/opa/loader/http/util.go:12-42` duplicate the algorithm, risking drift (the copies are currently identical in behavior).

## Failure Modes / Edge Cases

- **Timer/reconfigure race handled deliberately**: the log uploader cancels its timer loop on reconfigure so new delay settings take effect cleanly (`v1/plugins/logs/plugin.go:948-954`); mis-handling here would leak goroutines or use stale config.
- **ETag rollback on failure**: after a failed download/activation/persist, the downloader's ETag cache is reset to the last known value so the next conditional GET re-fetches correctly (`v1/plugins/bundle/plugin.go:516-519, 539-542, 553-556`).
- **Oversized single event**: an event larger than the entire buffer limit is dropped immediately (`v1/plugins/logs/buffer.go:36-38`); larger than `upload_size_limit_bytes` triggers the ND-cache-shedding ladder before final drop with `decision_logs_encoding_failure` (`v1/plugins/logs/encoder.go:179-212`).
- **Context cancellation during backoff sleep**: every sleep is selectable against ctx.Done, so stop/shutdown is prompt (`v1/download/download.go:250-260`, `v1/topdown/http.go:727-732`).
- **`http.send` retries only network errors**: HTTP 5xx responses do NOT trigger retries — only `client.Do` transport errors do (`v1/topdown/http.go:708-714`), and `context.Canceled` aborts immediately (line 721-723). Policy authors expecting server-error retry must implement it themselves.
- **Startup panic on corrupt local ETag state**: unreadable ETag from the store panics rather than proceeding inconsistently (`v1/plugins/bundle/plugin.go:369-373`, acknowledged TODO).
- **Status plugin silent loss**: status update failures are logged once and the update is lost — acceptable for telemetry, but means monitoring pipelines relying on status pushes can miss events during their own outages (`v1/plugins/status/plugin.go:359-405`).

## Future Considerations

- Extract a common `retryLoop` (config + callback + context) to eliminate four divergent implementations and make backoff parameters injectable/testable; the duplicated WASM copy (`internal/wasm/sdk/opa/loader/http/util.go:12-42`) would fold in naturally.
- Add direct unit tests for backoff timing/reset semantics of the downloader and log-uploader loops (today only `http.send` retry and buffer behavior have dedicated tests).
- Consider an opt-in durable spool for decision logs (disk-backed buffer behind the existing `buffer_type` knob, `v1/plugins/logs/plugin.go:285`) for deployments with strict no-loss requirements.
- An optional circuit-breaker mode for bundle services (open after N consecutive failures for a cool-down window) would cut useless traffic during prolonged outages while preserving current semantics as the default.
- Multi-service failover lists (try service B after service A fails N times) would address HA without external load balancers.

## Questions / Gaps

- **No evidence found for retry-state persistence**: retry counters are plain locals inside loop functions (`v1/download/download.go:218`, `v1/plugins/logs/plugin.go:905`, `v1/download/oci_download.go:197`); nothing writes them to store/disk. Search boundary: `retry|backoff` across all `.go` files plus targeted reads of all loop implementations listed above. Only ETags (`v1/bundle/store.go:134-135`) and bundles themselves persist.
- **No evidence found for circuit-breaker logic** anywhere in the repository (searched `circuit`, `breaker`, `degrade|Degrad` across Go sources; the only "degraded state" reference is the WARN plugin-state comment at `v1/plugins/plugins.go:142-145`).
- Whether the status plugin intentionally omits backoff (simplicity?) or it is an oversight could not be determined from code alone; CHANGELOG line 7538 region and git history were not inspected further within source-isolation rules.
- The `go.mod` dependency `github.com/cenkalti/backoff/v5` (`go.mod:65`) appears unused by first-party code searched (only indirect deps matched in `e2e/go.mod:28-29`); its consumer is presumably vendored tooling — not confirmed within the source tree.

---

Generated by `Dimension 13.02: Retry, Fallback, and Degraded Mode` against `opa`.

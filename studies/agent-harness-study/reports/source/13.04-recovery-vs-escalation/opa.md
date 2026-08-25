# Source Analysis: opa

## Dimension 13.04: Recovery vs Escalation

### Source Info

| Field | Value |
|-------|-------|
| Name | opa (Open Policy Agent) |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | Go; HTTP server + plugin framework (bundle/discovery/logs/status plugins), Rego evaluator (`topdown`), pluggable storage |
| Analyzed | 2026-08-25 |

## Summary

OPA is designed to run unattended, so it has **no in-process human escalation at all**: a repo-wide search for "escalat" across `v1/` returns no hits. Instead it implements a three-layer recovery model:

1. **Retry-forever with jittered exponential backoff** for all remote interactions (bundle download `v1/download/download.go:215-262`, discovery download shares the same downloader, decision-log upload `v1/plugins/logs/plugin.go:902-961`). The retry loop never gives up; the backoff caps out and the process keeps serving on its last-known-good state.
2. **Last-known-good preservation**: failed bundle downloads or activations leave the previously activated policy/data in the store intact because activation runs inside a storage transaction that is aborted on error (`v1/plugins/bundle/plugin.go:607-694`, `v1/storage/storage.go:95-108`), and the previous ETag is restored so the next poll re-syncs correctly (`v1/plugins/bundle/plugin.go:516-519`). Persisted bundles on disk provide crash-recovery at startup (`v1/plugins/bundle/plugin.go:378-435`, `v1/plugins/discovery/discovery.go:160-192`).
3. **Fail-fast escalation by process exit for unrecoverable states**: plugin startup failure aborts server start (`v1/runtime/runtime.go:686-689`) which exits with status 1 (`v1/runtime/runtime.go:651-656`); a listener failure exits immediately (`v1/runtime/runtime.go:833-835`); a bundle deactivation that would leave inconsistent state panics deliberately (`v1/plugins/bundle/plugin.go:220-224`). Escalation "to a human" is externalized: structured status objects are pushed to a remote `/status/` endpoint and/or console logs (`v1/plugins/status/plugin.go:475-511`), and health endpoints expose plugin state to orchestrators (Kubernetes-style probes) which own restart/page decisions (`v1/server/server.go:1299-1345`).

The design principle is: transient failures are absorbed silently-with-telemetry; only invariant violations (inconsistent store, failed boot) escalate, and escalation means "stop the process and let the orchestrator decide".

## Rating

**8 / 10** — A clear, explicit recovery model with strong tests and operational safeguards: unbounded backoff retry for remote I/O, transactional last-known-good activation, configurable thresholds, and multiple observable escalation surfaces (status service, Prometheus counters, health endpoints). It falls short of 9–10 because a few paths are ad-hoc: panic-on-deactivation-failure carries an acknowledged TODO (`v1/plugins/bundle/plugin.go:221-222`), discovery reconfiguration can be partially applied if a plugin `Start` fails mid-way through the new plugin set (`v1/plugins/discovery/discovery.go:420-437`), and status updates themselves are not retried between events.

## Evidence Collected

Every entry includes a file path with line numbers. All paths relative to `sources/opa`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Retry/backoff core | `DefaultBackoff(base, maxNS, retries)` exponential w/ ±20% jitter | v1/util/backoff.go:14-43 |
| Bundle download retry loop | On error uses backoff from 100ms floor; retry counter reset on success; never terminates except ctx cancel | v1/download/download.go:215-262 |
| Backoff cap config | Delay capped by `parsedMaxDelaySeconds` from polling config | v1/download/download.go:232-234 |
| ETag invalidation on failure | `d.etag = ""` before invoking callback so next attempt re-downloads | v1/download/download.go:264-273 |
| HTTP status → typed error | Non-200/304 becomes `HTTPError{StatusCode}` consumed by status reporting | v1/download/download.go:411-419, 441-447 |
| Last-known-good on download error | Status error set; previous etag re-set on downloader; returns error | v1/plugins/bundle/plugin.go:513-520 |
| Last-known-good on activation error | Same pattern after `activate()` fails | v1/plugins/bundle/plugin.go:536-543 |
| Transactional activation | Activation inside write txn; error ⇒ abort ⇒ store unchanged | v1/plugins/bundle/plugin.go:607-694; v1/storage/storage.go:95-108 |
| Crash recovery from disk | `loadAndActivateBundlesFromDisk` on plugin Start when persistence enabled | v1/plugins/bundle/plugin.go:112-133, 378-435 |
| Disk-persist failure tolerated if older version | "Older version of activated bundle persisted, ignoring error" | v1/plugins/bundle/plugin.go:739-754 |
| Discovery crash recovery | `loadAndActivateBundleFromDisk` when `config.Persist` set | v1/plugins/discovery/discovery.go:160-192 |
| Decision-log upload retry | Infinite loop, backoff on error, jittered poll interval on success | v1/plugins/logs/plugin.go:902-961 |
| Log drop under pressure | FIFO buffer drops oldest + increments Prometheus counter | v1/util/channel.go:9-31; v1/plugins/logs/eventBuffer.go:163-170; v1/plugins/logs/sizeBuffer.go:115,229-230 |
| Drop counter names | `decision_logs_dropped_rate_limit_exceeded`, `..._buffer_size_limit_exceeded`, `..._buffer_size_limit_bytes_exceeded` | v1/plugins/logs/plugin.go:274-276 |
| Rego-configurable drop decision | `drop_decision` config path; eval failure logs error and keeps event | v1/plugins/logs/plugin.go:273, 441-448, 763-773 |
| http.send bounded retry | `max_retry_attempts` param, 100ms–60s backoff, context cancel honored | v1/topdown/http.go:54-58, 699-735 |
| http.send `raise_error` | Caller chooses error vs undefined result — per-call escalation configurability | v1/topdown/http.go:1488-1497 |
| Plugin lifecycle states | `NOT_READY`/`OK`/`ERROR`/`WARN` state machine on Manager | v1/plugins/plugins.go:125-145 |
| Plugin start fail-fast | `Manager.Start` returns first plugin Start error | v1/plugins/plugins.go:870-918 |
| Boot escalation to exit | Serve propagates Start error → `StartServer` calls `os.Exit(1)` | v1/runtime/runtime.go:651-656, 686-689 |
| Listener failure exit | `errc` branch logs "Listener failed." then `os.Exit(1)` | v1/runtime/runtime.go:833-835 |
| Readiness gate at boot | `waitPluginsReady` polls plugin states until `ReadyTimeout`; failure aborts serve | v1/runtime/runtime.go:790-795, 1073-1091 |
| Panic on inconsistent deactivate | Deliberate panic w/ TODO acknowledging it "shouldn't panic" but OPA "shouldn't continue in a potentially inconsistent state" | v1/plugins/bundle/plugin.go:220-224 |
| Panic on etag load failure from store | Same fail-stop philosophy at init | v1/plugins/bundle/plugin.go:358-376 |
| Discovery refuses unrecoverable changes | "We don't currently support changes to the discovery configuration... errors would be unrecoverable"; rejects discovery-service/key updates | v1/plugins/discovery/discovery.go:491-526 |
| Manual vs periodic trigger semantics | Trigger returns errors only in manual mode; "periodic bundles will be retried" | v1/plugins/bundle/plugin.go:265-289 |
| Status push (human-notification surface) | POSTs full snapshot to `/status/<partition>`; non-2xx → error logged, retried on next event | v1/plugins/status/plugin.go:497-509, 351-431 |
| Console fallback channel | `ConsoleLogs` option writes status update to local log | v1/plugins/status/plugin.go:478-483 |
| Structured bundle status audit record | `Code`/`Message`/`HTTPCode`/`Errors` + `LastRequest` vs `LastSuccessful*` timestamps | v1/plugins/bundle/status.go:24-98 |
| Decision-log upload status | `decision_log_error` code with message/http_code mirrored into status | v1/plugins/logs/status/status.go:19-44; v1/plugins/logs/plugin.go:1365-1374 |
| Health endpoint escalation surface | `/health?plugins` fails when any plugin state != OK; `bundlesReady` gates on discovery+bundles | v1/server/server.go:1299-1345, 1279-1297 |
| Custom Rego health policy | `/health/<path>` evaluates user policy over `{plugin_state, plugins_ready}` input | v1/server/server.go:1347-1416 |
| Graceful stop | SIGINT/SIGTERM → graceful shutdown with configurable periods; manager Stop honors `gracefulShutdownPeriod` | v1/runtime/runtime.go:814-815, 829-832, 1032-1071; v1/plugins/plugins.go:920-947 |
| Eval failure isolation | Per-request eval error → HTTP 500 (`CodeInternal`, `MsgEvaluationError`); server keeps running | v1/server/writer/writer.go:34 |
| Tests: downloader reports failure & continues | `TestFailureAuthn`, `TestFailureNotFound`, `TestFailureUnexpected` assert typed errors surface via callback | v1/download/download_test.go:750-850 |
| Tests: manual-trigger error propagation | `TestPluginManualTriggerActivationErrorFile/Server` assert errors returned to caller in manual mode | v1/plugins/bundle/plugin_test.go:6782-6899 |
| Tests: activation failure surfaces in status | `TestPluginOneShotCompileError`, `TestPluginOneShotHTTPError` | v1/plugins/bundle/plugin_test.go:3020, 3114 |

## Answers to Dimension Questions

**1. When does the system retry vs escalate?**
Retry dominates. Any error from a periodic bundle download, discovery download, or decision-log upload triggers an infinite backoff-retry loop (`v1/download/download.go:232-256`, `v1/plugins/logs/plugin.go:917-945`); success resets the retry counter (`v1/download/download.go:252-256`). Even non-retryable-looking errors like compile failures are simply recorded in status and retried at the next poll (`v1/plugins/bundle/plugin.go:536-543`). Escalation (process death) happens only for: (a) failed plugin `Start` during boot (`v1/runtime/runtime.go:686-689` → `os.Exit(1)` at :651-656), (b) readiness timeout (`--ready-timeout`) before serving (`v1/runtime/runtime.go:790-795`), (c) listener runtime failure (`v1/runtime/runtime.go:833-835`), and (d) storage-inconsistency risk during bundle deactivation or etag load, implemented as panic (`v1/plugins/bundle/plugin.go:220-224, 372`). Per-request evaluation failures never escalate; they become HTTP 500 responses (`v1/server/writer/writer.go:34`).

**2. Are escalation thresholds configurable?**
Largely yes. Backoff ceilings come from polling config (`min_delay_seconds`/`max_delay_seconds`, injected as parsed ns values — `v1/download/config.go`, used at `v1/download/download.go:230-244`); decision-log reporting has its own `MinDelaySeconds`/`MaxDelaySeconds` (`v1/plugins/logs/plugin.go:917-922`). The `http.send` builtin exposes per-call `max_retry_attempts` and `raise_error` so policy authors choose retry count and whether failure manifests as error or undefined (`v1/topdown/http.go:54-58, 88-90, 703, 1488-1497`). Buffer limits and rate limits that decide *dropping* thresholds are config-driven (`v1/plugins/logs/plugin.go:286-287, 413-429`). What is *not* configurable: there is no retry-count ceiling or dead-letter threshold that flips any background loop into an ERROR→exit escalation; readiness timeout and shutdown periods are the main knobs (`v1/runtime/runtime.go:790-792, 1033-1040`). Trigger mode (manual vs periodic) also changes the contract: manual mode returns errors to the API caller instead of absorbing them (`v1/plugins/bundle/plugin.go:277-283`).

**3. Can the system stop gracefully?**
Yes. SIGINT/SIGTERM are trapped (`v1/runtime/runtime.go:814-815`) and route to `gracefulServerShutdown`, which supports a pre-shutdown wait period, a bounded graceful-shutdown timeout, trace/meter provider flushing, and storage close (`v1/runtime/runtime.go:1032-1071`). The plugin manager stops every plugin under the same bounded context so plugins can flush (e.g., status plugin flushes pending statuses and warns "statuses possibly not sent" on deadline — `v1/plugins/status/plugin.go:255-293`; manager-level `gracefulShutdownPeriod` at `v1/plugins/plugins.go:938-947`). One caveat documented in code: calling `Manager.Stop` twice hangs (`v1/plugins/plugins.go:925`).

**4. Are recovery decisions auditable?**
Partially. Every attempt/failure/recovery is logged (`Bundle load failed` `v1/plugins/bundle/plugin.go:514`, `Bundle activation failed` :537, `Persisting bundle to disk failed` :551, `Waiting %v before next download/retry.` `v1/download/download.go:247`) and structured `Status` records keep an auditable timeline: `LastRequest` vs `LastSuccessfulRequest`/`LastSuccessfulDownload`/`LastSuccessfulActivation` plus typed `Code`/`Message`/`HTTPCode` (`v1/plugins/bundle/status.go:24-98`). Drop decisions emit named Prometheus counters (`v1/plugins/logs/plugin.go:274-276`) and the generic FIFO helper counts every evicted event (`v1/util/channel.go:24-31`). All of this streams to the remote status service/console (`v1/plugins/status/plugin.go:497-509`). However there is no dedicated append-only recovery audit log, and dropped *status/log* payloads are counted but not preserved, so a burst of drops can lose the very evidence needed to audit them.

## Architectural Decisions

1. **Escalation is delegated to the orchestrator, not implemented in-process.** OPA's contract with operators is: expose truth (status snapshots, health endpoints, metrics), never self-terminate except on boot/invariant failures. Health handlers translate internal plugin state machine (`StateNotReady/OK/ERROR/WARN`, `v1/plugins/plugins.go:125-145`) into liveness/readiness signals orchestrators act on (`v1/server/server.go:1299-1345`).
2. **Last-known-good over rollback.** Failed activations abort their transaction leaving the previous bundle active (`v1/storage/storage.go:95-108`), and the previous ETag is deliberately re-installed so a recovered service re-syncs rather than skipping (`v1/plugins/bundle/plugin.go:516-519`). There is no explicit rollback machinery because none is needed under transactional activation.
3. **Fail-stop where consistency cannot be proven.** Deactivation failure panics even though the author notes it "probably shouldn't panic", because continuing "in a potentially inconsistent state" is worse (`v1/plugins/bundle/plugin.go:220-224`). Discovery similarly refuses config changes it cannot safely undo (`v1/plugins/discovery/discovery.go:491-494, 505-510, 518-526`).
4. **Single shared backoff primitive.** All loops (download, OCI download, logs, http.send) reuse `util.DefaultBackoff` (`v1/util/backoff.go:14-43`), giving uniform jittered-exponential behavior and making retry behavior predictable across subsystems.
5. **Bounded-memory degradation.** When downstream is slow/full, the system degrades by dropping oldest data with counters (logs buffers `v1/util/channel.go:9-31`; status channel `v1/plugins/status/plugin.go:299, 304`) rather than blocking policy evaluation — availability outranks completeness.

## Notable Patterns

- **Callback-carried error propagation**: the downloader wraps errors in `Update.Error` and joins callback errors (`errors.Join`, `v1/download/download.go:270-273`) so consumers decide absorb-vs-report.
- **Readiness latching**: bundle plugin flips manager state to OK once, on first successful activation of every configured bundle, then never regresses it (`checkPluginReadiness`, `v1/plugins/bundle/plugin.go:590-605`); the server's `allPluginsOkOnce` latch mirrors this so a later ERROR doesn't flap readiness after startup (`v1/server/server.go:1363-1378`). This encodes "ready = has been healthy at least once", a deliberate probe-stability choice.
- **Typed error taxonomy for status**: `SetError` classifies AST compile errors vs `download.HTTPError` vs other into structured fields (`v1/plugins/bundle/status.go:63-98`), enabling machine-readable escalation upstream.
- **Trigger-mode polymorphism**: the same plugin behaves differently under `manual` vs `periodic` triggering — errors returned vs absorbed-and-retried (`v1/plugins/bundle/plugin.go:277-283`, `v1/plugins/logs/plugin.go:869-892`).

## Tradeoffs

- **Infinite retry without give-up threshold**: a permanently broken bundle URL produces endless retries at capped delay forever; the only signal is status/metrics — acceptable for an agent, but it means some failures effectively *never* escalate beyond telemetry unless an operator watches it (`v1/download/download.go:220-261`).
- **Status delivery is best-effort**: failed status uploads are logged and retried only opportunistically on the next status-changing event (`v1/plugins/status/plugin.go:351-431`); a long-quiet system may not re-deliver an earlier failure promptly.
- **Partial discovery reconfiguration**: `Discovery.reconfigure` starts new plugins in order; an error partway leaves some plugins started with the new config while the function returns error and marks disco status errored — no atomic apply/rollback across the plugin set (`v1/plugins/discovery/discovery.go:420-437`).
- **Panic-based fail-stop** trades debuggability for safety; stack-trace crashes in production are the cost of guaranteeing consistency (`v1/plugins/bundle/plugin.go:220-224`).
- **Drops preserve liveness at cost of fidelity**: decision-log and status drops under pressure are invisible except as counters (`v1/util/channel.go:9-31`).

## Failure Modes / Edge Cases

- **Boot with unreachable bundle service + `--ready-timeout` set**: `waitPluginsReady` times out and the process exits nonzero (`v1/runtime/runtime.go:790-795`), letting the orchestrator retry the pod — clean delegation, but with no timeout configured OPA serves indefinitely without policies.
- **Repeated bad bundle (compile error)**: each poll downloads, fails compile, sets `bundle_error` status with AST errors (`v1/plugins/bundle/plugin.go:536-543`; classification at `v1/plugins/bundle/status.go:77-84`), restores old etag; old policy keeps serving — correct behavior, but bandwidth wasted until server fixes the bundle.
- **Persistence-directory corruption**: disk-load failures mark the bundle status error and continue with remote loading (`v1/plugins/bundle/plugin.go:389-421`); a partially-written persisted tarball does not wedge startup.
- **Storage write error during reconfiguration**: escalates all the way to panic (`v1/plugins/bundle/plugin.go:213-224`) — process dies rather than serve divergent routing tables.
- **Decision-log masking/drop-policy eval failure**: logged, event still enqueued unmasked-ish (masking failure skips enqueue of masked copy) — i.e., privacy-critical masking failures degrade to "log not sent" rather than "log sent unmasked" (`v1/plugins/logs/plugin.go:763-780, 1048+`).
- **http.send with exhausted retries**: returns last error to policy as undefined/error depending on `raise_error`; bounded so a hung downstream can't stall evaluation forever (`v1/topdown/http.go:708-734`).

## Future Considerations

- Replace the deactivation panic with an explicit error + safe-state transition, resolving the inline TODO (`v1/plugins/bundle/plugin.go:220-224`).
- Make discovery reconfiguration transactional (stage plugin set, start all-or-nothing) to eliminate partial-apply windows (`v1/plugins/discovery/discovery.go:420-437`).
- Add optional give-up/dead-letter thresholds for background loops (e.g., N consecutive activation failures → `StateErr` hard signal) for deployments that want louder escalation than telemetry.
- Persist or spool dropped decision-log/status events to disk when buffers overflow so audits remain possible after bursts.

## Questions / Gaps

- No evidence found of any human-notification mechanism beyond logs/status-service/health endpoints (searched for `escalat`, `human`, `operator intervention` across `v1/`, `plugins`, `runtime` — zero hits). Notification is fully delegated to external consumers of those signals.
- No dedicated recovery-decision audit log exists; auditability relies on standard logging plus structured status payloads (see Answers Q4).
- Retry-count tuning for background loops is limited to delay bounds; no evidence found of configurable max-attempt budgets for the download/log loops (only `http.send`'s `max_retry_attempts`, `v1/topdown/http.go:703`).
- Behavior of `sdk`-embedded usage (library mode) regarding recovery was not deeply analyzed; this study focused on the server/runtime path inside the selected source.

---

Generated by dimension `13.04-recovery-vs-escalation` against `opa`.

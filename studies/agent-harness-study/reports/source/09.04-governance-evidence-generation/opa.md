# Source Analysis: opa

## Governance Evidence Generation

### Source Info

| Field | Value |
|-------|-------|
| Name | opa |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | Go (OPA - Open Policy Agent) / Rego |
| Analyzed | 2026-08-26 |

## Summary

OPA is a policy decision point, not an agentic harness with tool approvals or workflow orchestration. Its governance evidence is centered on **decision logs** — per-decision JSON events recording input/result/bundles/metrics/timestamps — buffered, masked/dropped via Rego policies, and uploaded as gzip-compressed JSON arrays to a remote HTTP service, console, or custom plugin (`v1/plugins/logs/plugin.go:38-76`, `v1/plugins/logs/encoder.go:32-73`). Complementary evidence comes from **status reporting** (`v1/plugins/status/plugin.go:41-49`), **bundle status timestamps** (`v1/plugins/bundle/status.go:24-39`), **provenance/version info** (`v1/server/types/types.go:225-238`), and **request/trace identifiers** propagated via context (`v1/logging/logging.go:274-283`, `v1/server/buffer.go:18-43`). There is no unified compliance report for a "run", no tool-execution log, no approval-timestamp workflow, and no durable retention policy — buffers are volatile in-memory with bounded FIFO semantics that drop oldest entries (`v1/plugins/logs/buffer.go:32-57`, `v1/plugins/logs/plugin.go:265-281`). Evidence is highly machine-readable (structured JSON/slog, documented schema in `docs/docs/management-decision-logs.md:61-85`) and test-covered (`v1/plugins/logs/plugin_test.go:1-80` contains 4298 lines), but reproducibility requires external bundle revision capture and is degraded for non-deterministic builtins despite `nd_builtin_cache` capture (`v1/plugins/logs/plugin.go:63`).

## Rating

**5 / 10** — Present but inconsistent for the dimension.

Rationale: Decision-level evidence is mature (explicit `EventV1`/`Info` types, pluggable sinks, adaptive chunking, masking/drop, Prometheus counters, 3 trigger modes, comprehensive tests) aligning with 7-8 on the decision-log sub-problem. However, against the full Governance Evidence Generation rubric — policy decision records with approval timestamps, tool execution logs, consolidated compliance reports, SIEM integration, and retention enforcement — OPA covers only one axis. No approval workflow or tool log exists by design; no retention policy is enforced beyond volatile bounded buffers; SIEM export is bring-your-own-endpoint via generic REST plugin with no prebuilt Syslog/CEF/OTLP-SIEM adapters; and no per-run report generator exists. This incompleteness and retention fragility pull the score to the 4-6 band. 5 reflects strong engineering of what exists, but narrow scope versus the dimension's expectations.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Evidence report generation — decision log event schema | `EventV1` struct with labels, decision_id, bundles, path, input, result, metrics, timestamp, req_id, rule_labels, custom, nd_builtin_cache, masked/erased | `v1/plugins/logs/plugin.go:49-76` |
| Evidence report generation — bundle revision provenance | `BundleInfoV1{Revision}` and `ProvenanceV1`/`ProvenanceBundleV1` with version/build_timestamp/hostname/revision/bundles map | `v1/plugins/logs/plugin.go:79-81`, `v1/server/types/types.go:225-238` |
| Evidence report generation — server-side decision capture | `type Info struct` holding Txn, Bundles, DecisionID, TraceID, Timestamp, Input, Results, Metrics, RequestID, Custom, etc.; `decisionLogger.Log` builds Info from context and calls pluggable logger | `v1/server/buffer.go:18-43`, `v1/server/server.go:3102-3193` |
| Evidence report generation — data response carries decision_id + provenance | `DataResponseV1{DecisionID, Provenance, Result, Metrics}` returned inline per request | `v1/server/types/types.go:265-272` |
| Evidence report generation — status envelope aggregating bundle + decision-log errors | `UpdateRequestV1{Labels,Bundles,Bundles,Discovery,DecisionLogs,Plugins,Metrics}` wraps bundle discovery and decision-log status | `v1/plugins/status/plugin.go:41-49` |
| Evidence report generation — bundle activation/download timestamps | `Status{Name,ActiveRevision,LastSuccessfulActivation,LastSuccessfulDownload,LastRequest,Code,HTTPCode}` with UTC timestamps | `v1/plugins/bundle/status.go:24-39` |
| Policy decision records | `EventV1.Path`, `Query`, `Input`, `Result`, `DecisionID`, `Bundles[revision]` captured; rule_labels/ids included when annotated | `v1/plugins/logs/plugin.go:57-64,71`, `docs/docs/management-decision-logs.md:61-85` |
| Approval timestamps | No evidence found — `GOVERNANCE.md:33` discusses GitHub PR approvals for code, not runtime request approvals; grep for `approval\|approved` shows only docs/community notes, no runtime approval timestamp fields | `GOVERNANCE.md:33`, `docs/docs/contrib-code.md:144` (no runtime struct) |
| Tool execution logs | No evidence found — OPA has no agent tool layer; closest is `nd_builtin_cache` capturing non-deterministic builtin I/O (e.g., `http.send`) via `topdown` NDBCache serialization | `v1/plugins/logs/plugin.go:63`, `v1/server/server.go:3164-3170` |
| SIEM integration code — generic HTTP upload | `uploadChunk` POSTs gzip `application/json` to `client` at `*Resource` (default `/logs`) with retry/backoff in `loop`/`doOneShot` | `v1/plugins/logs/plugin.go:1143-1162`, `v1/plugins/logs/plugin.go:902-992` |
| SIEM integration code — pluggable sink interfaces | `Logger` interface (`Log(ctx,EventV1)`) and `plugins.LoggerPlugin` (slog.Handler) dispatch via `Plugin.Log` switch; custom backend tested in `plugin_test.go` | `v1/plugins/logs/plugin.go:38-43`, `v1/plugins/logs/plugin.go:800-818` |
| SIEM integration code — REST service config and status SIEM-like surface | Decision logs configurable via `service`+`resource`+`partition_name`; status plugin similarly POSTs to `/status/<partition>`; metrics exposed for SIEM alerting | `v1/plugins/logs/plugin.go:304-317`, `v1/plugins/status/plugin.go:497-510` |
| Machine-readable encoding — slog attrs | `eventToAttrs` maps EventV1 to `[]slog.Attr` with typed fields; `eventToFields` produces `map[string]any` with RFC3339Nano timestamp | `v1/plugins/logs/plugin.go:1201-1239`, `v1/plugins/logs/plugin.go:1287-1363` |
| Machine-readable encoding — chunk wire format | `chunkEncoder` writes gzip JSON arrays bounded by `upload_size_limit_bytes`; `chunkDecoder.decode` reverses | `v1/plugins/logs/encoder.go:32-73`, `v1/plugins/logs/encoder.go:464-487` |
| Decision ID / trace correlation | `generateDecisionID()` called per request, stored via `logging.WithDecisionID` and `attribute.String("opa.decision_id")` for OTel span | `v1/server/server.go:2759`, `v1/server/server.go:1115-1116`, `v1/server/server.go:3212-3215`, `v1/logging/logging.go:274-283` |
| Masking / redaction evidence | `maskEvent`/`dropEvent` evaluate Rego at `defaultMaskDecisionPath="/system/log/mask"` and `defaultDropDecisionPath="/system/log/drop"`; masked/erased pointers recorded | `v1/plugins/logs/plugin.go:1048-1141`, `v1/plugins/logs/plugin.go:272-274`, `v1/plugins/logs/mask.go:1-60` |
| Retention — buffer limits and drop counters | `ReportingConfig{BufferType,BufferSizeLimitBytes,BufferSizeLimitEvents,UploadSizeLimitBytes}` with limits 0-unlimited vs 10000 events default; `logBuffer.Push` FIFO drops oldest; metrics `decision_logs_dropped_*` | `v1/plugins/logs/plugin.go:283-293`, `v1/plugins/logs/plugin.go:265-280`, `v1/plugins/logs/buffer.go:32-57`, `v1/plugins/logs/plugin.go:274-277` |
| Retention — no durable storage | Search for `retention|retained|storage.*policy|expir` in `v1/plugins/logs` returns no durable retention policy (only unrelated test expiration) | `v1/plugins/logs/plugin_test.go:2136` (only context expiration) |
| Logging abstraction | `logging.Logger` interface (Debug/Info/Warn/Error/WithFields/GetLevel/SetLevel) plus `SlogHandler` adapter and `BufferedLogger` flush/forward | `v1/logging/logging.go:30-40`, `v1/logging/logging.go:297-350`, `v1/logging/buffered_logger.go:19` |
| Tests showing intended behavior | Slow-tagged plugin tests covering custom backend, buffering, masking, reconfigure, manual trigger | `v1/plugins/logs/plugin_test.go:70-80`, `v1/plugins/logs/buffer_test.go:1`, `v1/plugins/logs/encoder_test.go:1`, `v1/plugins/logs/mask_test.go:1` |

## Answers to Dimension Questions

### 1. What evidence does the system produce?

**Structured decision logs are the primary artifact.** Per evaluation, OPA emits an `EventV1` (`v1/plugins/logs/plugin.go:49-76`) containing `timestamp` (RFC3339Nano, `v1/plugins/logs/plugin.go:68,1294`), `decision_id`/`batch_decision_id`/`trace_id`/`span_id` (`v1/plugins/logs/plugin.go:51-54`), `labels` identifying the instance (`v1/plugins/logs/plugin.go:50`), `bundles` with per-bundle `revision` (`v1/plugins/logs/plugin.go:56`), `path`/`query`, `input`/`result`/`mapped_result`/`intermediate_results`, `nd_builtin_cache` for replay (`v1/plugins/logs/plugin.go:63`), `metrics`, `req_id`, `requested_by`, `request_context.http.headers` (opt-in), and `erased`/`masked` pointer lists. Server-side capture is via `server.Info` (`v1/server/buffer.go:18-43`) populated by `decisionLogger.Log` (`v1/server/server.go:3108-3193`) on every Data/Query/Compile invocation, including OTel span IDs when tracing is enabled (`v1/server/server.go:3172-3176`).

**Inline response evidence:** Every `DataResponseV1` can carry `decision_id` and `provenance` (version, build_timestamp, bundles) (`v1/server/types/types.go:265-272`).

**Management evidence:** `status.UpdateRequestV1` aggregates `bundles`, `discovery`, `decision_logs` (error code/message/http_code, `v1/plugins/logs/status/status.go:23-28`), and `plugins` state plus runtime `metrics` (`v1/plugins/status/plugin.go:41-49`). Bundle status adds last-success timestamps for download/request/activation (`v1/plugins/bundle/status.go:26-33`).

**What is NOT produced:** No consolidated compliance report for a run/session, no approval timestamps, no tool execution trace (only NDBCache for builtins), and no policy-check checklist artifact. These are correctly absent for a policy engine but count as gaps against the dimension.

### 2. Is evidence machine-readable?

**Yes for decision logs and status.** Wire format is a gzip-compressed JSON array of events with a documented schema (`docs/docs/management-decision-logs.md:30-85`, `v1/plugins/logs/plugin.go:1143-1150`). Code paths `eventToFields` (`v1/plugins/logs/plugin.go:1287-1363`) and `eventToAttrs` (`v1/plugins/logs/plugin.go:1201-1239`) produce `map[string]any` / `[]slog.Attr` typed structured logs, consumable by any JSON or slog handler. `chunkEncoder`/`chunkDecoder` (`v1/plugins/logs/encoder.go:32-73,464-487`) enforce byte-accurate round-tripping. Status payloads are `json.Marshal`'d (`v1/plugins/status/plugin.go:554-567`). The plugin extension point accepts arbitrary `slog.Handler` or `Logger` implementations (`v1/plugins/logs/plugin.go:800-818`), enabling direct fan-out to SIEM-friendly log sinks without custom parsers. Pretty/compact toggling and configurable timestamp formats are supported (`v1/logging/logging.go:33-40`). Limitation: no native CEF/Syslog/OTLP-SIEM transformer; SIEM integration is endpoint-agnostic REST — consumers must adapt schema themselves.

### 3. Can evidence be reproduced?

**Partially; best-effort.** For a given `decision_id`, the decision log records `input`, `result`, and the bundle `revision` (`v1/plugins/logs/plugin.go:56-60`) plus `labels` and `timestamp`, allowing re-evaluation against that bundle snapshot if the bundle store is retained. `nd_builtin_cache` (`v1/plugins/logs/plugin.go:63`, populated at `v1/server/server.go:3164-3170`) preserves non-deterministic builtin input/output maps to aid replay, but when an event exceeds `upload_size_limit_bytes` the cache is intentionally dropped (`v1/plugins/logs/encoder.go:108-131`, `v1/plugins/logs/plugin.go:1221-1224`) with metrics `decision_logs_nd_builtin_cache_dropped`. There is no built-in bundle store snapshot shipping with each event, no versioned input artifact beyond what the caller sent, and no idempotent "re-run report" API. Status/bundle timestamps are wall-clock derived (`time.Now().UTC()` at `v1/plugins/bundle/status.go:44`) and not anchored to a correlation run ID. Thus, deterministic decisions are reproducible given external bundle archival; non-deterministic decisions may lose cache and cannot be perfectly replayed.

### 4. Are evidence retention policies enforced?

**No durable retention; only volatile bounded buffering with drop-oldest semantics.** Configuration offers `buffer_size_limit_bytes` (default 0 = unlimited) and `buffer_size_limit_events` (default 10000) and `upload_size_limit_bytes` (32KB default) with min/max clamping (`v1/plugins/logs/plugin.go:267-280,283-293,373-429`). The size buffer uses `logBuffer` FIFO that drops front entries when `usage+size>limit` (`v1/plugins/logs/buffer.go:32-57`); the event buffer uses a bounded channel (documentation in `v1/plugins/logs/README.md:22-29`). On non-2xx upload, the chunk is requeued and retried with exponential backoff (`docs/docs/management-decision-logs.md:86-89`, `v1/plugins/logs/plugin.go:902-992`); on buffer overflow, metrics `decision_logs_dropped_buffer_size_limit_bytes_exceeded` / `_events` increment but data is lost. No TTL, no disk persistence, no WORM archiving, no retention-duration config, and no enforcement that evidence survives restart (`plugin.Stop` flushes best-effort only if `Service != ""` and deadline permits, `v1/plugins/logs/plugin.go:665-713`). Search for retention keywords yields no policy (`No evidence found` beyond GitHub workflow `retention-days` for artifacts). This is the primary fragility for compliance use-cases.

## Architectural Decisions

- **Decision-log as post-evaluation side channel, not inline result:** `server.Info` decoupled then fanned out to `Logger` plugins (`v1/server/server.go:3108-3193`, `v1/plugins/logs/plugin.go:716-822`). Keeps evaluation hot-path unblocked but means logging failures are non-fatal (wrapped as `decision_logs: %w` at `v1/server/server.go:3188`). Tradeoff: degraded observability if logger silently drops via policy.

- **Dual buffer strategies with adaptive compression:** `sizeBuffer` (compressed chunks, unlimited by default, lock-heavy) vs `eventBuffer` (raw events, channel-backed, compress-on-upload) selectable via `buffer_type` (`v1/plugins/logs/plugin.go:612-629`, `v1/plugins/logs/README.md:12-90`). Adaptive `uncompressedLimit` (`v1/plugins/logs/encoder.go:49-73,94-101,139-156`) guesses compression ratio to avoid double-gzip. This optimizes throughput but adds complexity (scaleUp/scaleDown recursion, `scalingDown` guard at `v1/plugins/logs/encoder.go:320-324`).

- **Policy-driven masking and drop via Rego refs:** `maskDecisionRef`/`dropDecisionRef` parsed from paths `/system/log/mask` and `/system/log/drop` (`v1/plugins/logs/plugin.go:272-273,432-449`), prepared once (`prepareOnce`, `v1/plugins/logs/plugin.go:493-514`) and re-prepared on compiler updates (`v1/plugins/logs/plugin.go:897-900`). Enables data governance without code changes, at cost of an extra Rego eval per decision — failures logged but do not block evaluation (`v1/plugins/logs/plugin.go:766-789`).

- **Pluggable sinks over native SIEM connectors:** Interface `Logger` (`v1/plugins/logs/plugin.go:38-43`) and `plugins.LoggerPlugin` (slog.Handler) plus console/REST (`v1/plugins/logs/plugin.go:790-819`, `v1/plugins/status/plugin.go:489-510`). Keeps OPA neutral to SIEM vendor; delegates transformation to operators.

- **Status aggregation as second-order evidence:** `status.Plugin` buffers last values per domain and snapshots them on trigger (`v1/plugins/status/plugin.go:56-71,537-552`), reported periodically (`v1/plugins/status/plugin.go:351-431`). Provides health audit trail (bundle revisions, last success timestamps) distinct from per-decision logs.

## Notable Patterns

- **In-memory decision evidence pipeline with backpressure telemetry:** `chunkEncoder` scales uncompressed limit exponentially (`v1/plugins/logs/encoder.go:94-101`) and emits metrics histograms/counters for scale events and dropped logs (`v1/plugins/logs/encoder.go:17-29`, `v1/plugins/logs/plugin.go:274-277`).

- **Context-propagation for correlation:** `RequestContext`, `HTTPRequestContext`, `DecisionID`, `BatchDecisionID` threaded via `context.Context` keys (`v1/logging/logging.go:224-294`) and attached to both structured logger fields and decision events (`v1/server/server.go:3131-3153`). Enables tracing from ingress through eval to stored event.

- **Graceful degradation on size pressure:** When an event exceeds `upload_size_limit_bytes`, encoder drops `nd_builtin_cache` and retries (`v1/plugins/logs/encoder.go:108-131`); on repeated failure it increments `enc_log_exceeded_upload_size_limit_bytes` and drops the event. Prevents one large NDBCache from stalling the pipeline.

- **Consistent error surface for audit:** Bundle and decision-log errors normalized to `{code, message, http_code}` (`v1/plugins/bundle/status.go:65-98`, `v1/plugins/logs/status/status.go:32-51`, `v1/server/types/types.go:15-24`). Enables uniform SIEM alerting.

## Tradeoffs

- **Completeness vs scope fidelity:** OPA excels at decision attestation but consciously omits agentic concerns (agent steps, tool calls, approvals). Integrating OPA into an harness requires the harness to emit its own evidence and correlate via `decision_id`; OPA alone cannot satisfy a full governance audit.

- **Latency vs durability:** Periodic/min-delay upload (default 300-600s, `v1/plugins/logs/plugin.go:265-266,915-930`) batches for efficiency and tolerates transient endpoint failures via requeue/backoff, but introduces window for volatile loss on crash/restart. No WAL guarantees delivery.

- **Flexibility vs out-of-box compliance:** Pluggable logger and configurable resource path (`v1/plugins/logs/plugin.go:451-461`) allow routing to any SIEM, but operators must build and operate the collector, schema mapping, and retention themselves.

- **Privacy vs fidelity:** Masking (`v1/plugins/logs/mask.go:1-60`) can upsert/remove paths or fail-undefined handling, with `erased`/`masked` pointers retained for audit (`v1/plugins/logs/plugin.go:64-65`). Aggressive masking improves compliance but reduces debuggability; misconfigured mask rules are logged as skipped (`v1/plugins/logs/plugin.go:1088-1091`) rather than failing closed.

- **Unbounded default buffer vs loss:** Default `buffer_size_limit_bytes=0` (unlimited, `v1/plugins/logs/plugin.go:271`) prevents drops under burst but risks OOM; switching to bounded event buffer (`v1/plugins/logs/README.md:15-29`) trades determinism for memory safety and introduces event drops.

## Failure Modes / Edge Cases

- **Buffer overflow silent loss:** Exceeding `buffer_size_limit_bytes/events` drops oldest chunks/events and only increments Prometheus counters (`decision_logs_dropped_*` at `v1/plugins/logs/plugin.go:274-277`); no persistent dead-letter queue. Clients polling `status.UpdateRequestV1.DecisionLogs` see last error code but not count of lost decisions directly.

- **Non-2xx remote silos evidence:** Upload failure causes requeue + exponential backoff (`v1/plugins/logs/plugin.go:922-923`); prolonged outage can fill bounded buffers and cascade to drops, or bloat unbounded buffers. `Stop` with deadline may leave buffer non-empty (`v1/plugins/logs/plugin.go:665-713` logs "Plugin stopped with decisions possibly still in buffer").

- **Oversized event after NDBCache strip:** Single decision larger than `upload_size_limit_bytes` even after cache removal is dropped with `logEncodingFailureCounterName` (`v1/plugins/logs/encoder.go:181-186`, `v1/plugins/logs/encoder.go:205-212`). Mitigation requires raising upload limit (capped at 4GB, `v1/plugins/logs/plugin.go:270,379-393`) or reducing input/result size.

- **Mask/drop policy errors are non-blocking:** If compilation or `Eval` of mask/drop fails, `maskEvent`/`dropEvent` returns error and `Log` logs and proceeds without masking/dropping or skips event masking (`v1/plugins/logs/plugin.go:766-789`). A broken governance policy degrades evidence hygiene rather than failing the request.

- **Race between request cancel and decision log:** Fixed at `v1/server/server.go:3186-3188` by using `context.WithoutCancel` for logger invocation; historically a cancel could race mask/drop eval (referenced in `v1/server/server_test.go:4423-4458`).

- **Concurrent reconfigure:** `reconfigMtx` RWLock (`v1/plugins/logs/plugin.go:479`) blocks pushes during buffer type reconfiguration; `push` acquires read lock (`v1/plugins/logs/plugin.go:1040-1046`). Misuse could stall hot path if reconfigure uploads stall.

- **Schema extensibility vs strict consumers:** `intermediate_results` and `custom` maps are lazily JSON round-tripped (`v1/plugins/logs/plugin.go:1264-1277`); unexpected types that fail `util.RoundTrip` are silently omitted from `eventToFields` fields (`v1/plugins/logs/plugin.go:1307-1333`).

## Future Considerations

- **Unified run/compliance report API:** Add `GET /v1/report?decision_id=` or session-scoped aggregation producing a signed envelope (decision + provenance + bundle revisions + status) so `Can the system produce a compliance report for a given run?` becomes yes. Currently requires external orchestration.

- **Durable retention/WAL:** Introduce optional disk-backed buffer or configurable TTL/sink acknowledgment (e.g., spool to volume, Storm/OTLP exporter with retry journal) to enforce retention. Currently retention is not enforced.

- **First-class approval chain:** For harnesses embedding OPA, expose approval constructs (who approved, when, on what policy version) as part of `EventV1` or a linked `DecisionContext` extension; model after `RequestedBy`/`RequestContext` but with verifiable attestation.

- **SIEM-native exporters:** Ship optional formatters for OCSF/CEF/JSON-SIEM or OpenTelemetry log exporter so `eventToAttrs` can push directly to Splunk/Datadog/Elastic without a custom translator.

- **Reproducibility bundle snapshot handle:** Include bundle content hash or inline bundle manifest hash in `Bundles` (currently only `revision` string, `v1/plugins/logs/plugin.go:79-81`) and expose a replay endpoint `opa replay --decision-id` that replays with archived store via `nd_builtin_cache`.

- **Loss accounting in status:** Expose dropped-event counters directly in `Status.DecisionLogs` rather than only Prometheus, so remote audit can quantify gaps.

## Questions / Gaps

- **Per-run report generation:** No evidence found for a built-in report combining multiple decisions into a compliance document (e.g., `reports/repo/09.04` style). Could a `server.Info` aggregator be feasible without stateful session tracking? Needs product decision.

- **Retention enforcement:** Search confirms no `retention` config or time-based expiry in decision-log or bundle plugins. If regulatory retention (e.g., 7 years) is required, is expectation that SIEM guarantees it, or should OPA offer retention-assured sink with ack protocol?

- **Approval workflow integration:** No runtime concept of approval — would OPA be expected to consume external approval service verdicts as `input` and simply log them, or should it own the workflow? Clarify harness boundary.

- **Tool execution provenance:** `nd_builtin_cache` covers `http.send` and similar but not generic agent tools. If OPA were reused for tool gating, where would tool stdout/stderr, exit code, and duration be persisted?

- **Dropped-event audit:** Prometheus-only drop reporting may be invisible to SIEM if metrics pipeline diverges from log pipeline. Should drops also emit a synthetic decision log event or status detail with counts?

- **Timestamp trust:** `LastSuccessfulActivation` etc. use local `time.Now().UTC()` (`v1/plugins/bundle/status.go:44`), vulnerable to clock skew. Should evidence include monotonic or notary-signed time?

---

Generated by `Dimension 09.04: Governance Evidence Generation` against `opa`.

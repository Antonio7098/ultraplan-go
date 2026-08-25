# Source Analysis: opa

## Security Auditability

### Source Info

| Field | Value |
|-------|-------|
| Name | opa (Open Policy Agent) |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | Go; policy language Rego; plugin architecture (bundles, decision logs, status, discovery); REST + gRPC server; Go SDK |
| Analyzed | 2026-08-24 |

## Summary

OPA's auditability story is centered on **decision logs**: a first-class, schema-versioned record of every policy query (`EventV1`, `v1/plugins/logs/plugin.go:49-76`) that is generated per request with a crypto-random UUID (`v1/runtime/runtime.go:1152-1158`), returned to the caller in the API response (`v1/server/server.go:1644-1646`), and delivered to a remote HTTP service, local console, or pluggable sink through a buffered uploader with retries (`v1/plugins/logs/plugin.go:716-822`, `plugin.go:902-961`). Redaction is itself auditable: mask/drop policies run over each event and record which paths were erased or masked on the event (`v1/plugins/logs/mask.go:178`, `mask.go:197`; fields at `v1/plugins/logs/plugin.go:64-65`). Delivery health is observable via a status stream and Prometheus counters (`v1/plugins/logs/plugin.go:1365-1374`, `v1/plugins/status/plugin.go:537-552`).

The model is weaker outside the decision-log path. The server's own authorization layer (`system.authz`) evaluates allow/deny per request but emits no decision ID and no decision-log event (`v1/server/authorizer/authorizer.go:107-165`) — unauthorized attempts are only visible as generic access-log lines. The authenticated caller identity is injected into the *authorization* input (`authorizer.go:216-219`) but is **not** a field of decision-log events; events carry only `requested_by` (client address) plus optionally configured HTTP headers (`v1/plugins/logs/plugin.go:737`, `plugin.go:745-759`). There is no human-approval workflow inside OPA; the nearest analog is bundle signature verification (JWT against configured public keys, `v1/bundle/verify.go:70-94`), an out-of-band approval mechanism for policy content. Durability under failure is bounded by in-memory buffers: overflow, rate limiting, and encoding failures silently drop events and increment counters rather than writing tombstones (`v1/plugins/logs/eventBuffer.go:161-171`, `eventBuffer.go:180-196`; counter names at `v1/plugins/logs/plugin.go:274-277`).

Overall: a clear, well-tested, explicitly interfaced decision-audit pipeline ("can the system defend a risky action after the fact?" — yes, for policy decisions, when operators enable it), with identifiable gaps in who-did-what attribution, authorization-decision auditing, and crash durability.

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale: Decision logging is a designed-in feature with a documented event schema (`docs/docs/management-decision-logs.md:5-11` states the auditing goal), a stable public extension interface (`logs.Logger`, `v1/plugins/logs/plugin.go:39-43`), extensive tests (`TestPluginMasking` at `v1/plugins/logs/plugin_test.go:2355`, `TestPluginDrop` at `plugin_test.go:2774`, drop-counter tests at `plugin_test.go:4138-4195`, sink integration at `logger_plugin_integration_test.go:73`), and operational safeguards (status surfacing of upload errors, Prometheus metrics, retry/backoff). It does not reach 8–9 because authorization decisions are unaudited, caller identity is absent from events, buffering is memory-only with silent loss paths, and records have no integrity protection (no signing/hashing of the log stream).

## Evidence Collected

Every entry cites `path:line` relative to `sources/opa`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Policy decision record (schema) | `EventV1` struct: `decision_id`, `batch_decision_id`, `trace_id`, `span_id`, `bundles`, `path`, `query`, `input`, `result`, `erased`, `masked`, `requested_by`, `timestamp`, `req_id`, `rule_labels`, `request_context`, `custom` | `v1/plugins/logs/plugin.go:49-76` |
| Decision ID generation | UUID v4 from `crypto/rand` via `uuid.New(rand.Reader)`; empty string if generation fails | `v1/runtime/runtime.go:1152-1158` |
| Decision ID factory wiring | ID is generated only when the decision logger is enabled (`logs.Lookup(rt.Manager) != nil`) | `v1/runtime/runtime.go:955-963` |
| Server wiring of IDs + logger | `WithDecisionIDFactory(rt.decisionIDFactory).WithDecisionLoggerWithErr(rt.decisionLogger)` | `v1/runtime/runtime.go:720-721` |
| ID in request context & OTel span | `logging.WithDecisionID(r.Context(), decisionID)` + `annotateSpan(ctx, decisionID)` on every query path (5 handlers) | `v1/server/server.go:1115-1117`, `1526-1527`, `1754-1755`, `2334-2335`, `2394-2395`; span attr at `3212-3216` |
| Decision ID returned to caller | `types.DataResponseV1{DecisionID: decisionID}` — callers can correlate response ↔ log event | `v1/server/server.go:1644-1646`; also `1895`; documented at `docs/docs/rest-api.md:812-814` |
| Event assembly from request | `Info` struct (`Txn`, `Bundles`, `RemoteAddr`, `HTTPRequestContext`, `InputAST`, `Error`, `Metrics`, `TraceID/SpanID`, `EvaluatedRuleLabels`, `Custom`) | `v1/server/buffer.go:18-43`; populated at `v1/server/server.go:3144-3182` |
| Logging decoupled from client cancellation | `context.WithoutCancel(ctx)` so a disconnect cannot race mask/drop evaluation | `v1/server/server.go:3184-3189` |
| Drop policy (silent suppression) | Default path `/system/log/drop`; evaluated before buffering; `rs.Allowed()` drops event | `v1/plugins/logs/plugin.go:273`, `772-774`, `1102-1141` |
| Mask policy (redaction) | Default path `/system/log/mask`; prepared-query caching invalidated on reconfigure/compiler update | `v1/plugins/logs/plugin.go:272`, `1048-1100`, `897-900` |
| Mask ops and audit-of-redaction | `remove`/`upsert` ops; applied paths appended to `event.Erased` / `event.Masked` | `v1/plugins/logs/mask.go:20-27`, `178`, `197`; surfaced in console fields at `plugin.go:1328-1329` |
| Delivery fan-out | console (`logEvent` → ConsoleLogger), remote service (buffer push), custom plugin (`Logger` interface or slog `plugins.LoggerPlugin`) | `v1/plugins/logs/plugin.go:790-819`, `1164-1168`, `39-43` |
| Buffered upload w/ retry & backoff | upload loop, randomized min/max delay, exponential backoff on failure | `v1/plugins/logs/plugin.go:902-961`; config `ReportingConfig` at `283-293` |
| Silent loss paths (counters only) | Rate-limit drop, buffer-size drops, encoding-failure drops increment metric counters and log errors — no tombstone events for lost decisions | `v1/plugins/logs/eventBuffer.go:161-171`, `180-196`; `sizeBuffer.go:115-125`, `227-232`; counter names `plugin.go:274-277` |
| Upload error observability | `setStatus` propagates into status plugin snapshot (`decision_logs.code/http_code/metrics`) | `v1/plugins/logs/plugin.go:1365-1374`; `v1/plugins/logs/status/status.go:23-51`; `v1/plugins/status/plugin.go:312-313`, `537-552`; docs `docs/docs/management-status.md:256-259` |
| HTTP security event (access) log | `LoggingHandler`: "Received request."/"Sent response." with `client_addr`, `req_id`, `req_method`, `req_path`, `resp_status`, `resp_bytes`, `resp_duration`; bodies at debug level | `v1/runtime/logging.go:60-174` (recv `117`, send `172`); wired at `v1/runtime/runtime.go:785-786` |
| Request ID semantics | Per-process atomic counter (`atomic.AddUint64`), not globally unique | `v1/runtime/logging.go:68` |
| Caller identity plumbing | Bearer token handler sets identity = raw token string; TLS handler sets cert subject; `Identity(r)` read into authz input | `v1/server/identifier/token.go:22-30`; `tls.go:26`; `identifier.go:18-25`; consumed at `v1/server/authorizer/authorizer.go:216-219` |
| Identity absent from decision events | `EventV1` has no identity field; only `requested_by` (RemoteAddr) and opt-in `request_context.http.headers` | `v1/plugins/logs/plugin.go:67`, `72`, `737`, `745-759`; header config `295-301` |
| Authorization decisions not logged | `Basic.ServeHTTP` evaluates `system.authz` inline, returns 401 (+optional policy-provided `reason`); no decision ID, no decision-log emission | `v1/server/authorizer/authorizer.go:107-165` (eval `128`, deny `164`) |
| Approval mechanism (out-of-band) | Bundle signature verification: JWT signature checked against configured public keys; failure blocks activation and sets bundle status error | `v1/bundle/verify.go:62-94`; key configs `v1/plugins/bundle/config.go:83`; activation failure handling `v1/plugins/bundle/plugin.go:391-392`, `417-418`, `515` |
| Capability usage traces in events | Non-deterministic builtin results cached into events (`nd_builtin_cache`, opt-in); evaluated-rule metadata labels (`rule_labels`) | `v1/plugins/logs/plugin.go:63`, `71`, `177-183`, `1348-1353`; docs `docs/docs/configuration.md:1040`, `docs/docs/policy-language.md:2650` |
| NDB cache may be dropped to fit limits | Encoder drops `nd_builtin_cache` (counter `decision_logs_nd_builtin_cache_dropped`) when event exceeds upload size | `v1/plugins/logs/encoder.go:21`, `105-124` |
| Distributed tracing correlation | W3C trace/span IDs copied onto events when valid span present | `v1/server/server.go:3172-3176` |
| Policy print() routed to logs | `loggingPrintHook` emits policy `print()` output tagged with request-context fields | `v1/runtime/logging.go:20-37` |
| Config validation surface | Service/console/plugin selection, partition name, buffer types validated with explicit errors | `v1/plugins/logs/plugin.go:303-317`, `319-464` |
| Tests demonstrating behavior | Masking incl. upsert/erase tracking; drop rules; rate-limit drop counters; no-loss guarantees; custom sink integration | `v1/plugins/logs/plugin_test.go:2355`, `2774`, `1594`, `4138`, `4195`; `logger_plugin_integration_test.go:73`, `171` |
| Documented design goal | "…information that enables auditing and offline debugging of policy decisions"; ecosystem section on auditing | `docs/docs/management-decision-logs.md:5-11`, `305-315` |

## Answers to Dimension Questions

**1. Who did what?**
Partially answerable. *What* is well captured: each query produces an immutable-ish event with the queried `path`/`query`, full `input`, `result`, bundle revisions, timestamps, and metrics (`v1/plugins/logs/plugin.go:49-76`). *Who* is weak: events record `requested_by` = client socket address (`v1/server/server.go:3150`, mapped at `v1/plugins/logs/plugin.go:737`) plus operator-configured HTTP headers (`plugin.go:745-759`). The authenticated identity exists only in the authorization layer's input (`v1/server/authorizer/authorizer.go:216-219`) and never lands in the event schema. Notably, the bearer-token identifier sets the identity to the raw token string (`v1/server/identifier/token.go:28`), so even if operators capture the `Authorization` header via `request_context.http.headers`, they would be recording bearer credentials into audit logs. Attribution therefore depends on network-level identity or application-supplied headers — an auditor cannot, from OPA alone, reliably answer "which principal made this decision".

**2. What policy allowed it?**
Answerable to a point. Events identify the decision entrypoint (`path`/`query`) and the active bundles and revisions (`Bundles` map, `v1/plugins/logs/plugin.go:56`, `717-720`), and rule-level metadata labels show which rules were evaluated (`RuleLabels`, `plugin.go:71`; `docs/docs/policy-language.md:2650`). However, the event does not record *which rule expression produced the result*, nor a policy-content hash beyond bundle revision — reconstructing "why" requires fetching the exact bundle revision that was live at the event timestamp. Provenance data (versions, revisions) is available on demand in responses (`v1/server/server.go:1652-1654`). For the server's own API authorization, nothing links a 401 to a policy evaluation: the authorizer returns a bare error code with optional free-text `reason` (`v1/server/authorizer/authorizer.go:146-164`), and no decision ID is minted for denied requests.

**3. Was a human involved?**
No human-in-the-loop mechanism exists inside OPA, and none is implied by the design — it is an inline policy engine. The human element is out-of-band: policy authors sign bundles (JWT signatures verified at load against locally configured public keys or a registered verifier plugin, `v1/bundle/verify.go:70-94`), which acts as a pre-deployment approval gate; verification failure prevents activation and surfaces in bundle status (`v1/plugins/bundle/plugin.go:391-392`, `417-418`). There is no approval record, no signer identity persisted with the activated bundle in the decision stream, and no interactive escalation/approval flow for risky decisions.

**4. Can auditors reconstruct the decision?**
Largely yes for allowed queries, with caveats. Correlation keys are strong: `decision_id` appears both in the synchronous API response and in the async log event (`v1/server/server.go:1644-1646` vs `3135`, `3149`), `batch_decision_id` groups batch queries (`v1/logging/logging.go:285-294`), and `trace_id`/`span_id` tie events to distributed traces (`server.go:3172-3176`). Reconstruction degrades in four ways: (a) silent loss — rate-limited, buffer-overflowed, or unencodable events vanish leaving only aggregate counters (`v1/plugins/logs/eventBuffer.go:161-171`, `180-196`; counter names `plugin.go:274-277`), with no tombstone identifying *which* decisions were lost; (b) buffers are in-memory only — a crash loses undelivered decisions, and shutdown flush is best-effort with an explicit "stopped with decisions possibly still in buffer" warning (`v1/plugins/logs/plugin.go:686-713`); (c) intentional suppression — the `/system/log/drop` policy removes events pre-buffering, leaving only a debug line (`plugin.go:772-774`); (d) no integrity protection — events are plain gzip JSON over HTTP to the service (`plugin.go:1143-1162`), unsigned and unchained, so tamper-evidence depends entirely on the receiving infrastructure.

## Architectural Decisions

1. **Audit trail as a plugin, not a core path dependency.** Decision logging lives behind the plugin manager (`logs.New`, looked up at `v1/runtime/runtime.go:959`, `966-971`); the eval path tolerates its absence (`decisionLogger.Log` no-ops without a sink, `v1/server/server.go:3122-3124`) and even decision-ID generation is disabled when no logger is configured (`v1/runtime/runtime.go:959-962`). This keeps hot-path overhead zero-by-default but means auditing is opt-in configuration, not an invariant.
2. **Policy-governed redaction instead of hardcoded filtering.** Mask/drop are themselves Rego policies evaluated against the serialized event (`v1/plugins/logs/plugin.go:1048-1100`, `1102-1141`), giving deployment-specific control while recording applied operations on the event (`erased`/`masked` arrays).
3. **Correlation-first identifiers.** Crypto-random UUID decision IDs minted per request and echoed in responses (`v1/server/server.go:1644-1646`), plus OpenTelemetry span attributes (`server.go:3212-3216`), make response↔log↔trace joins a first-class contract.
4. **At-least-effort delivery, not guaranteed delivery.** FIFO in-memory buffers with drop-oldest semantics and retry/backoff (`v1/plugins/logs/buffer.go:32-56`, `plugin.go:902-961`) trade completeness for bounded resource use.
5. **Extensible sinks via two interfaces.** Legacy `Logger` (`Log(ctx, EventV1) error`, `plugin.go:39-43`) and modern slog-based `plugins.LoggerPlugin` fan-out (`plugin.go:806-816`), with a shipped file-based JSON-lines sink (`v1/plugins/logger/file/plugin.go:105`).

## Notable Patterns

- **Prepared-query caching for audit policies**: mask/ddrop queries are prepared once and invalidated on compiler updates and reconfiguration (`v1/plugins/logs/plugin.go:493-514`, `861-862`, `897-900`) — audit machinery avoids re-compiling Rego per event.
- **Self-describing loss accounting**: every drop mode has a dedicated Prometheus counter name constant (`plugin.go:274-277`, `encoder.go:21`), and these counters are embedded into subsequent console events' metrics maps, making loss visible even in the log stream itself (`plugin_test.go:1363-1366`).
- **Cancellation-safe logging**: `context.WithoutCancel(ctx)` ensures a client disconnect mid-request cannot race or abort the mask/drop evaluation (`v1/server/server.go:3184-3189`).
- **Status-as-audit-signal**: upload failures flow into the same status pipeline used for bundles/discovery (`UpdateDecisionLogsStatus`, `v1/plugins/status/plugin.go:312-313`), giving one operational channel for all control-plane health.
- **Schema discipline**: `EventV1.AST()` hand-maintains a Rego-value mirror of the struct so masking can run without a JSON round trip, with a warning comment requiring sync (`v1/plugins/logs/plugin.go:46-48`, `100-249`).

## Tradeoffs

- **Opt-in auditing vs zero-overhead default**: no logger configured ⇒ no decision IDs at all (`v1/runtime/runtime.go:959-962`); deployments that skip configuration have no post-hoc reconstruction material whatsoever.
- **Bounded memory vs completeness**: `buffer_size_limit_bytes`/events caps and `max_decisions_per_second` protect the process (`ReportingConfig`, `plugin.go:283-293`) at the cost of silently losing the oldest/largest events — reasonable for liveness, hostile to forensic completeness.
- **Redaction vs verifiability**: mask `upsert` can rewrite values (`v1/plugins/logs/mask.go:22`, `114-119`); since events are unsigned, an operator-controlled mask policy could alter history without detection. The `erased`/`masked` lists disclose removals but not original values (by design) and don't protect against malicious modification upstream of the collector.
- **Header passthrough vs sensitive-data leakage**: configurable `request_context.http.headers` aids attribution but puts the burden on deployers to avoid capturing credentials like raw Authorization tokens.
- **Debug verbosity vs secret exposure**: debug-level request/response body capture (`v1/runtime/logging.go:83-113`, `142-170`) is valuable forensically but can write sensitive payloads to general-purpose logs.

## Failure Modes / Edge Cases

- **Encoding failure loses individual events** with only an error log naming the decision ID (`v1/plugins/logs/eventBuffer.go:180-196`); multi-event chunk decode failures drop whole chunks (`eventBuffer.go:100-107`, `sizeBuffer.go:70`).
- **Rate limiting drops newest decisions outright** rather than shedding load elsewhere (`eventBuffer.go:162-168`), so bursty traffic creates unlogged windows; a status test confirms counters reach operators but not identities of lost events (`plugin_test.go:1594`).
- **Shutdown with undelivered logs**: if the stop deadline expires, flush aborts with "Plugin stopped with decisions possibly still in buffer." (`v1/plugins/logs/plugin.go:704-712`).
- **Drop-policy eval errors swallow the event**: if the drop query errors, `Log` returns nil after logging an error — the decision is *not* buffered (`plugin.go:766-770`); same for mask failures (`785-788`), i.e., audit-policy misconfiguration suppresses rather than fails open/closed explicitly.
- **Authorization denials leave thin traces**: 401s appear only as access-log lines (`resp_status`), since the authorizer bypasses the decision pipeline entirely (`v1/server/authorizer/authorizer.go:128-164`).
- **Non-unique auxiliary IDs**: `req_id` is a per-process atomic counter (`v1/runtime/logging.go:68`), colliding across replicas; only the UUID `decision_id` is globally unique.
- **NDB cache truncation changes event content under size pressure** (`v1/plugins/logs/encoder.go:105-124`), meaning identical decisions can serialize differently depending on transport conditions — a subtlety when diffing historical events.

## Future Considerations

- Add the authenticated principal to `EventV1` (sanitized, not the raw bearer token) so decision events self-describe attribution (`v1/plugins/logs/plugin.go:49-76`; identity source at `v1/server/identifier/identifier.go:18-25`).
- Emit decision-log events (or a distinct audit event type) for `system.authz` denials, including the authz decision path and reason, to close the biggest who-was-denied gap (`v1/server/authorizer/authorizer.go:140-165`).
- Provide tombstone/loss-ledger events on rate-limit or buffer-overflow drops (e.g., count + first/last dropped `decision_id`s), upgrading loss from aggregate counters to reconstructable gaps (`v1/plugins/logs/eventBuffer.go:161-171`).
- Offer optional durable spooling (disk-backed buffer) and/or signed event chains for deployments requiring tamper evidence and crash survival (`v1/plugins/logs/plugin.go:466-473` buffer interface is the natural extension seam).
- Record the producing policy fingerprint (compiler hash or bundle content digest, not just revision string) on each event to bind outcomes to exact policy text (`v1/plugins/logs/plugin.go:717-720`).

## Questions / Gaps

- No evidence found of any approval/sign-off record tied to decision-log configuration changes: discovery-driven reconfiguration swaps logger config automatically (`v1/plugins/logs/plugin.go:855-867`, `994-1038`) with no operator-action audit trail beyond generic logs. Searched `cmd/`, `runtime/`, `plugins/discovery/` for approval/approval-flow symbols — none exist (search terms: `approv`, `sign-off`, `admission`).
- No evidence found of log-integrity features (hash chaining, signing of exported events, WORM export). Search covered `v1/plugins/logs/`, `internal/prometheus/`, and `docs/docs/security.md` (no audit mentions found there).
- Whether batch-decision IDs are populated anywhere in-tree: `BatchDecisionID` exists on `server.Info` (`v1/server/buffer.go:23`) and flows into events (`plugin.go:725`, context helpers at `v1/logging/logging.go:285-294`), but the core server never generates one — population appears delegated to external callers (e.g., Envoy/gRPC integrations not fully audited within this boundary).
- Human-review workflows for policy content (e.g., CI gates) are outside this repository's boundary; only the verification half (signature check at load) is evidenced here.

---

Generated by `dimension 08.04-security-auditability` against `opa`.

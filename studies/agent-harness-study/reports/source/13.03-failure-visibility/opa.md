# Source Analysis: opa

## Dimension 13.03: Failure Visibility

### Source Info

| Field | Value |
|-------|-------|
| Name | opa (Open Policy Agent) |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | Go (logrus logging, Prometheus client, OpenTelemetry, net/http server, cobra CLI) |
| Analyzed | 2026-08-24 |

## Summary

OPA is a policy decision engine rather than an LLM agent harness, so this study maps the four stakeholders onto OPA's actual roles: the "model" role is played by **programmatic callers** (the Go SDK / `rego` API consumers and, transitively, any automated decision-maker that consumes OPA's verdicts), users are **HTTP API clients and CLI operators of `opa eval`**, developers consume **structured logs, decision-log events and OTel traces**, and operators consume **Prometheus metrics, `/health`, `/v1/status`, and status-plugin pushes**.

Failure visibility in OPA is deliberately layered and machine-readable end to end:

1. The engine produces typed errors with source locations (`v1/topdown/errors.go:29-34`, `v1/ast/errors.go:82-87`).
2. The HTTP layer maps error types to stable JSON envelopes with fixed codes (`v1/server/writer/writer.go:27-42`, `v1/server/types/types.go:117-134`).
3. Every failed evaluation is also recorded in the decision log with an `error` field (`v1/plugins/logs/plugin.go:66`, `plugin.go:781-783`) and correlated via `decision_id`.
4. Operators get failure-pattern metrics (`bundle_failed_load_counter{name,code,message}` at `v1/plugins/status/metrics.go:72-78`), plugin state gauges, health probes that explain *why* they fail (`v1/server/server.go:1418-1428`), and OTel spans carrying `opa.decision_id`.

Detail level is configurable on nearly every surface (log level/format flags, `explain=`, `?metrics`, masking rules, `--strict-builtin-errors`). The main weaknesses are that dropped decision-log events are only visible as log lines (no metric), and the defined-but-unused `evaluation_error` code means runtime eval failures are reported as generic `internal_error`.

## Rating

**8 / 10** — Clear model with tests, explicit interfaces, and operational safeguards. All four stakeholder surfaces have structured, test-asserted failure contracts (e.g., exact 500-body with file/row/col asserted in `v1/server/server_test.go:1575-1596`; log fields asserted in `v1/runtime/logging_test.go:313-345`). Not a 9–10 because: (a) decision-log drops have no aggregated counter — only `logger.Error` lines (`v1/plugins/logs/eventBuffer.go:165`, `sizeBuffer.go:117,125,232`); (b) `CodeEvaluation = "evaluation_error"` is declared (`v1/server/types/types.go:120`) but `writer.ErrorAuto` routes topdown errors to `internal_error` (`v1/server/writer/writer.go:33-34`); (c) undefined-decision semantics differ across surfaces (HTTP v0 → 404 envelope vs SDK → typed error vs CLI exit-code-only).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Engine error type (what callers see) | `topdown.Error{Code, Message, Location}`; renders as `file:row: col: code: message` | `v1/topdown/errors.go:29-34`, `84-98` |
| Eval error taxonomy | Codes `eval_internal_error`, `eval_cancel_error`, `eval_conflict_error`, `eval_type_error`, `eval_builtin_error`, `eval_with_merge_error` | `v1/topdown/errors.go:36-61` |
| Parse/compile errors carry location + details | `ast.Error{Code, Message, Location, Details}`; codes `rego_parse_error`, `rego_type_error`, `rego_unsafe_var_error`, etc.; `Details.Lines()` appends expression context | `v1/ast/errors.go:48-66`, `76-119` |
| Cancellation is distinguishable | `IsCancel(err)` matches `eval_cancel_error` via `errors.Is` | `v1/topdown/errors.go:69-82` |
| SDK error surface for embedders | `sdk.Error{Code, Message}`; `opa_undefined_error` + `IsUndefinedErr` helper | `v1/sdk/opa.go:537-563` |
| Strict builtin errors option | `StrictBuiltinErrors` field on SDK eval options | `v1/sdk/opa.go:367`, `522` |
| REST API error codes | Stable constants: `internal_error`, `evaluation_error`, `unauthorized`, `invalid_parameter`, `invalid_operation`, `resource_not_found`, `resource_conflict`, `undefined_document` | `v1/server/types/types.go:117-127` |
| Error envelope shape | `ErrorV1{code, message, errors[]}`; `WithError`/`WithASTErrors` nest detailed sub-errors | `v1/server/types/types.go:129-168` |
| Fixed human-readable messages | `MsgCompileModuleError`, `MsgEvaluationError`, `MsgUnauthorizedError`, `MsgUndefinedError`, … | `v1/server/types/types.go:170-184` |
| Error→HTTP-status mapping | `writer.ErrorAuto`: BadRequest→400, write-conflict→404-notfound, `topdown.IsError`→500+internal_error w/ nested detail, invalid patch→400, not-found→404, default→500 | `v1/server/writer/writer.go:27-42` |
| Undefined decision response | v0 returns 404 `undefined_document`, distinguishing "document missing" vs "document undefined" by checking compiler rules | `v1/server/server.go:1209-1227` |
| Test asserting full eval-error body | Expected body includes `errors[].location.{file,row,col}` for `eval_conflict_error` | `v1/server/server_test.go:1575-1596` |
| Authz rejection detail | Policy-provided `reason` string surfaced in 401 message; fallback generic message; missing authz policy → 500 | `v1/server/authorizer/authorizer.go:136`, `152-164` |
| Decision ID correlation | `DataResponseV1.DecisionID` returned to clients; decision id injected into ctx before eval | `v1/server/types/types.go:267`, `v1/server/server.go:1115-1117` |
| Non-fatal warnings in success responses | `warning{code:"api_usage_warning", message:"'input' key missing…"}` attached to DataResponseV1 | `v1/server/server.go:1903`, `v1/server/types/types.go:295-309` |
| Failed decisions logged too | `server.Info.Error` captured; `execQuery`/`v0QueryPath` call `logger.Log(..., err, ...)` on every failure path | `v1/server/buffer.go:37`, `v1/server/server.go:1014-1018`, `1169`, `1203-1206` |
| Decision-log event carries error | `EventV1.Error` serialized into event AST for mask/drop policy inspection | `v1/plugins/logs/plugin.go:66`, `201-206`, `781-783` |
| Access-log middleware | "Received request."/"Sent response." entries with `req_id`, `client_addr`, `resp_status`, `resp_bytes`, `resp_duration`; debug adds `req_body`/`resp_body` | `v1/runtime/logging.go:60-173` |
| Log-field test evidence | Assertions on `resp_body`, "Sent response." messages per request | `v1/runtime/logging_test.go:313-345` |
| Log format/level configurability | `--log-format` json (default)/text/json-pretty; `--log-level` debug/info/warn/error | `cmd/run.go:67`, `243-244`; `internal/logging/logging.go:18-42` |
| Structured request context fields | `RequestContext.Fields()` → `client_addr`, `req_id`, `req_method`, `req_path` | `v1/logging/logging.go:230-250` |
| Print statements visible with source line | print hook attaches `line` field from policy location | `v1/runtime/logging.go:20-37` |
| Early-startup logs preserved | BufferedLogger(1000) captures logs before plugins start | `v1/runtime/runtime.go:413-417` |
| Distributed tracing | OTLP grpc/http exporter, per-request spans, sampler config; otel internal errors rerouted to OPA logger | `internal/distributedtracing/distributedtracing.go:98-197`, `311-317`; wired at `v1/runtime/runtime.go:521-534` |
| Prometheus failure observability | `http_request_duration_seconds{code,handler,method}`, `http_request_cancellations{code,handler,method}`; status captured from response writer; `/metrics` registered | `internal/prometheus/prometheus.go:42-79`, `82-97` |
| Registry gather failures logged | `All()` logs "Failed to gather metrics…" via injected `errorLogger` | `internal/prometheus/prometheus.go:109-118`; `v1/runtime/runtime.go:1132-1136` |
| Status snapshot endpoint | `GET /v1/status` returns plugin snapshot; 500 if status plugin disabled | `v1/server/server.go:2476-2485` |
| Remote status push payload | `UpdateRequestV1{labels, bundles, discovery, decision_logs, metrics, plugins}` pushed to control plane or console | `v1/plugins/status/plugin.go:39-49`, `554-569` |
| Bundle failure status fidelity | `bundle.Status{Code,Message,Errors[],HTTPCode}`; `SetError` preserves individual `ast.Errors` for compile failures, HTTP status code for download failures | `v1/plugins/bundle/status.go:25-39`, `63-98` |
| Operator failure metrics | `bundle_failed_load_counter{name,code,message}`, `bundle_loaded_counter`, `plugin_status_gauge{name,status}`, `opa_info` | `v1/plugins/status/metrics.go:49-106`; reset/increment at `v1/plugins/status/plugin.go:570-592` |
| Health checks explain failure reason | `/health` 500 body contains `{"error": "unable to perform evaluation" \| "one or more bundles are not activated" \| …}`; policy-driven `/health/{path}` reports undefined/unexpected value | `v1/server/server.go:1300-1345`, `1347-1428` |
| Decision-log upload failure status | Upload errors set `decision_logs` status `{code,message,http_code}` shown in status updates | `v1/plugins/logs/plugin.go:975-991`, `1365-1372`; `v1/plugins/logs/status/status.go:23-32` |
| Drop events loudly logged | Rate-limit/serialization/buffer-full drops emit `logger.Error("Decision log dropped…")` | `v1/plugins/logs/eventBuffer.go:165`; `sizeBuffer.go:70`, `117`, `125`, `227-232` |
| Explain modes for debugging decisions | `explain=full\|notes\|fails\|debug` returns trace events to clients | `v1/server/server.go:2576-2604`; param at `v1/server/types/types.go:570` |
| Detail-level knobs per surface | `--strict-builtin-errors`, `--show-builtin-errors` (CLI); `?metrics`, `?instrument`, `?pretty`, `strict-builtin-errors` (API params) | `cmd/eval.go:354-356`; `v1/server/types/types.go:564-580`, `608-610` |
| Decision-log redaction with audit trail | Mask policy (default `/system/log/mask`) erases/masks fields; events record which paths were touched | `v1/plugins/logs/plugin.go:64-65`, `185-199`, `272`, `310-315`, `1048-1090` |
| Programmatic Rego debugger | DAP-inspired Debug API (breakpoints, variables) for policy authors | `v1/debug/README.md:1-14`, `v1/debug/debugger.go` |
| Docs tied to implementation | Monitoring doc documents OTel spans incl. `opa.decision_id` attribute and metric table | `docs/docs/monitoring.md:5-16`, `38-44`, `62-67` |

## Answers to Dimension Questions

**1. Is the model informed of failures?**
There is no LLM in OPA's loop; the closest analogs are both well served. Embedded callers receive fully typed, unwrappable errors: `topdown.Error` exposes `Code`/`Message`/`Location` and implements `errors.Is` matching so cancellation vs conflict vs type errors are programmatically distinguishable (`v1/topdown/errors.go:63-98`); the SDK wraps undefined decisions in its own coded error `opa_undefined_error` detectable via `IsUndefinedErr` (`v1/sdk/opa.go:547-563`). Automated decision consumers over HTTP get `decision_id` in every response (`v1/server/types/types.go:267`) plus machine-checkable `code` fields, and even *successful* responses can carry non-fatal `api_usage_warning` notices (`v1/server/server.go:1903`). One caveat: undefinedness is signalled differently per surface (HTTP v0 404 envelope, v1 GET 200 `{}`, SDK typed error, CLI exit code 1 with `--fail`) — consistent within each interface, but not uniform across them.

**2. Is the user informed appropriately?**
Yes. The REST API always answers failures with the same envelope `{code, message, errors[]}` where `errors[]` preserves engine-level detail including policy file/row/col locations — asserted verbatim in tests (`v1/server/server_test.go:1575-1596`). Status mapping is deterministic via `ErrorAuto` (`v1/server/writer/writer.go:27-42`). Authorization rejections surface the policy-authored `reason` when provided instead of a bare denial (`v1/server/authorizer/authorizer.go:152-157`). Health probes return a JSON body explaining *which* subsystem failed (`v1/server/server.go:1311-1343`, `1418-1428`). On the CLI, errors render through a presentation layer that converts known types to `{message, code, location, details}` output and unwraps wrapper errors to avoid opaque strings (`internal/presentation/presentation.go:143-217`); exit codes distinguish error (2) from `--fail`-triggered undefined (1) (`cmd/eval.go:331-341`).

**3. Can developers debug failures?**
Yes, unusually well. Every failed evaluation is written to the decision log stream with the `error` embedded in the event (`v1/plugins/logs/plugin.go:66`, `781-783`), correlated to the access log via shared `req_id` and to responses/traces via `decision_id`. Access logs include status, duration, byte counts, and (at debug level) full request/response bodies with gzip-aware decoding (`v1/runtime/logging.go:83-120`, `142-169`). `explain=` query parameters return eval traces to the caller (`v1/server/server.go:2576-2604`), print statements carry the originating policy location (`v1/runtime/logging.go:20-37`), OTel spans link requests to decisions (`docs/docs/monitoring.md:5-16`), and a DAP-style debugger supports breakpoint-level debugging of Rego (`v1/debug/README.md:1-14`). Startup logs are buffered so pre-plugin failures aren't lost (`v1/runtime/runtime.go:413-417`).

**4. Can operators detect failure patterns?**
Mostly yes. Prometheus exposes per-handler latency histograms labeled with response `code` (so 5xx rates are queryable) plus a cancellations counter (`internal/prometheus/prometheus.go:42-58`), bundle load failures as a labeled counter `{name, code, message}` (`v1/plugins/status/metrics.go:72-78`), and plugin state gauges (`plugin_status_gauge`). The status plugin pushes rich failure snapshots (per-bundle code/message/errors/http_code, decision-log uploader status) to a remote service or console (`v1/plugins/status/plugin.go:39-49`; `v1/plugins/bundle/status.go:63-98`). Gap: dropped decision-log events — silent data loss in an audit channel — are only reported through `logger.Error` lines (`v1/plugins/logs/eventBuffer.go:165`; `sizeBuffer.go:117-125`, `227-232`) with no Prometheus counter or status flag, making aggregate drop-rate detection hard.

**5. Are failure detail levels configurable?**
Yes, extensively: global `--log-level`/`--log-format` (`cmd/run.go:243-244`); per-request `explain`, `metrics`, `instrument`, `pretty` params (`v1/server/types/types.go:564-580`); `--strict-builtin-errors`/`--show-builtin-errors` to promote or collect built-in failures (`cmd/eval.go:354-355`); decision-log mask/drop policies with configurable decision path (`v1/plugins/logs/plugin.go:272`, `310-315`); status console/prometheus toggles (`v1/plugins/status/plugin.go:75-83`); OTel sampling and span-processor tuning (`internal/distributedtracing/distributedtracing.go:168-183`).

## Architectural Decisions

1. **Single canonical error envelope at every boundary.** Engine errors (`ast.Error`, `topdown.Error`) are converted once — to `ErrorV1` for HTTP (`v1/server/writer/writer.go:46-55`), to `OutputError` for the CLI (`internal/presentation/presentation.go:250-256`), to `sdk.Error` for embedders (`v1/sdk/opa.go:537-545`) — each retaining code + location. This keeps failure semantics stable across surfaces.
2. **Failures are data, not just signals.** Failed evaluations produce the same decision-log event structure as successful ones, with `error` as a first-class field (`v1/plugins/logs/plugin.go:66`, `781-783`), so downstream mask/drop policies can treat failures uniformly.
3. **Type-switch dispatch over sentinel checks for status mapping.** `ErrorAuto` inspects error types (`types.IsBadRequest`, `storage.IsWriteConflictError`, `topdown.IsError`, …) to pick HTTP status/code (`v1/server/writer/writer.go:27-42`).
4. **Correlation IDs everywhere.** `decision_id` generated per request (`v1/server/server.go:1115-1117`), threaded into responses, decision logs, and OTel span attributes (`docs/docs/monitoring.md:10-13`); `req_id` threads access-log pairs (`v1/runtime/logging.go:68`).
5. **Redaction without losing auditability.** Masking records *what* was erased/masked inside the event itself (`v1/plugins/logs/plugin.go:185-199`), preserving failure-investigation capability while hiding sensitive values.

## Notable Patterns

- **Distinguishing "missing" from "undefined".** For undefined results, the server consults the compiler to decide between `document missing` and `document undefined` messages (`v1/server/server.go:1216-1219`) — a subtle but valuable diagnostic distinction.
- **Location-carrying errors as a first-class convention.** Both parse-time (`ast.Error.Location`, `v1/ast/errors.go:85`) and eval-time (`topdown.Error.Location`, `v1/topdown/errors.go:32`) errors attach source positions, and `AppendText` renders them uniformly (`v1/topdown/errors.go:89-98`).
- **Structured errors survive nesting.** CLI output unwraps wrapped errors to recover structured ones rather than printing one opaque string (#3663 comment, `internal/presentation/presentation.go:192-202`); similarly `bundle.Status.SetError` keeps each compile error individually (`v1/plugins/bundle/status.go:77-84`).
- **Policy-governed observability pipelines.** Drop (`dropEvent`) and mask (`maskEvent`) decisions for logs are themselves evaluated as Rego queries against the event (`v1/plugins/logs/plugin.go:766-775`, `785-788`), with failures of those queries logged and the event still emitted conservatively.
- **Graceful degradation of telemetry.** Prometheus registry gather failures are logged through an injected error sink rather than panicking (`internal/prometheus/prometheus.go:109-118`); collector registration failures log but don't abort (`v1/plugins/status/metrics.go:131-140`).

## Tradeoffs

- **Debug-level body capture vs memory/privacy.** Logging full `req_body`/`resp_body` at debug (`v1/runtime/logging.go:99-103`, `168`) gives excellent forensic detail but buffers entire payloads in memory and risks logging sensitive data unless carefully gated.
- **Stable message strings vs flexibility.** Fixed messages (`v1/server/types/types.go:170-184`) give clients reliable matching targets but constrain how precisely specific failures can be described; nuance moves into the free-form `message` of nested errors.
- **Generic 500 for eval failures.** Routing all `topdown.Error`s to HTTP 500 `internal_error` (`v1/server/writer/writer.go:33-34`) avoids leaking status-mapping complexity to clients, at the cost of making policy-caused conflicts indistinguishable from server bugs at the status-code level (detail survives only in the nested `errors[]`).
- **Console-first drop reporting.** Emitting drop notifications only to the logger keeps the pipeline simple but sacrifices aggregate visibility compared to a counter.
- **v0/v1 semantic divergence.** Keeping v0's 404-on-undefined while v1 GET returns 200 `{}` preserves backward compatibility but splits the user-facing failure contract.

## Failure Modes / Edge Cases

- **Decision-log drops under backpressure.** Buffer overflow, rate limiting, and serialization failures drop events with only an error line (`v1/plugins/logs/sizeBuffer.go:117-125`, `227-232`); if the OPA logger itself is misconfigured, drops become effectively invisible.
- **Mask/drop policy failure.** If evaluating the mask rule fails, the event is still logged but the failure is only console-visible (`v1/plugins/logs/plugin.go:785-788`) — potential PII exposure path mitigated only by operator alerting on those lines.
- **Authz policy undefined.** A configured-but-undefined authorization policy yields 500 with `authorization policy missing or undefined` (`v1/server/authorizer/authorizer.go:134-137`) — correctly treated as misconfiguration, not denial.
- **Status plugin not enabled.** `GET /v1/status` fails with 500 `status plugin not enabled` (`v1/server/server.go:2477-2480`) — discoverable, but requires knowing the plugin must be configured.
- **Cancellation.** Client disconnects are separately observable as `http_request_cancellations` increments keyed by status/method (`internal/prometheus/prometheus.go:94-96`) and `eval_cancel_error` codes (`v1/topdown/errors.go:41-42`, `69-72`).
- **Health check semantics.** `/health?plugins` failing names no offending plugin in the body ("one or more plugins are not up", `v1/server/server.go:1339-1342`); the caller must follow up with `/v1/status` or use `/health/ready` policy input which does expose per-plugin states (`v1/server/server.go:1355-1379`).

## Future Considerations

- Add a Prometheus counter (and/or status-plugin field) for dropped decision-log events, labeled by cause (rate-limit, buffer-full, encode-error) — turning current log-line-only visibility (`v1/plugins/logs/sizeBuffer.go:117-125`) into alertable signal.
- Either retire `CodeEvaluation` (`v1/server/types/types.go:120`) or use it in `writer.ErrorAuto` so runtime eval failures are semantically distinct from genuine internal errors.
- Consider surfacing the failing plugin name(s) in the `/health?plugins` error body alongside the boolean condition (`v1/server/server.go:1327-1342`).
- Unify undefined-decision signalling documentation across SDK/CLI/HTTP surfaces to reduce cross-surface surprise.

## Questions / Gaps

- No dedicated UI exists in-repo (no dashboards); "user-facing error pages" map entirely to the REST/CLI surfaces studied above. Search covered `v1/server/`, `internal/presentation/`, and docs; no HTML error rendering was found — consistent with OPA being headless infrastructure.
- `ast.Error.Details` marshaling through `ErrorV1.Errors []error` (`v1/server/types/types.go:133`) relies on `json` tags of `ast.Error` (`v1/ast/errors.go:82-87`); whether `Details` round-trips into API responses was not verified by execution (static reading suggests it serializes only when the concrete type marshals cleanly).
- Long-term durability under sustained failure storms (e.g., Prometheus scrape during registry contention) could not be assessed statically; no chaos/failure-injection tests were found under `e2e/` for the observability paths specifically.

---

Generated by `13.03-failure-visibility` against `opa`.

# Source Analysis: opa

## Dimension 09.03 — Governance UX and Operator Workflow

### Source Info

| Field | Value |
|-------|-------|
| Name | opa (Open Policy Agent) |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | Go, embedded Rego engine, HTTP/JSON REST APIs, Prometheus metrics |
| Analyzed | 2026-08-26 |

## Summary

OPA is a policy decision point (PDP), not a governance workflow engine, and its code contains **no approval dashboards, review queues, or approve/reject handlers**. A repo-wide search for `dashboard`, `approval`, and `approve` in Go sources returns only an incidental test-fixture string (`v1/ast/index_test.go:1314`), and the repository ships no HTML assets outside docs (`find -name "*.html"` is empty). OPA's design instead assumes the human governance workflow lives in the surrounding control plane: policy changes are gated by bundle signatures at activation time (`v1/bundle/bundle.go:916`, `v1/bundle/verify.go:70`) and by atomic compile-or-reject semantics in the bundle plugin (`v1/plugins/bundle/plugin.go:607-681`), not reviewed by humans inside the product.

What OPA does provide is a mature operator *visibility* layer that answers "what did the system decide and why is it unhealthy" without reading code:

1. **Decision logs** — every decision is recorded as a structured `EventV1` (`v1/plugins/logs/plugin.go:49-76`) with decision ID, full input, result, active bundle revisions, evaluated rule labels, metrics, and caller info; events can be emitted to console (`v1/plugins/logs/plugin.go:790-794`), uploaded to a service, or routed to custom plugins via the `Logger` interface (`v1/plugins/logs/plugin.go:38-43`).
2. **Status reporting** — an aggregate `UpdateRequestV1` snapshot of bundle, discovery, decision-log, and plugin states (`v1/plugins/status/plugin.go:41-49`), deliverable to console (`status/plugin.go:554-568`), HTTP services, or Prometheus collectors (`v1/plugins/status/metrics.go:36-43`).
3. **Health endpoints** — `/health` checks evaluation capability, bundle activation, and per-plugin state (`v1/server/server.go:1299-1345`, route at `server.go:880`), plus a Rego-evaluated `/health?policy=` endpoint that turns any policy into a health probe over live plugin state input (`server.go:1347-1416`).
4. **Policy inspection APIs** — `/v1/policies` lists raw source and compiled AST for every loaded module (`v1/server/server.go:2179-2214`), and PUT returns precise compile errors (`server.go:2266-2275`) so operators see exactly what is loaded and what was rejected.
5. **Decision provenance** — `explain=full|notes|fails` query params produce topdown traces for data and compile APIs (`server.go:1813-1814`, `1932-1933`, response builder at `server.go:2576`), and each API response carries a `decision_id` (`v1/server/types/types.go:267`) correlating the caller-visible outcome with the decision-log evidence record (documented contract: `docs/docs/management-decision-logs.md:11,66`).

Bulk operations exist as bulk *evaluation* (`opa exec`), not bulk approval: `opa exec` recursively evaluates one decision per input file (`cmd/exec.go:36-56`), writes a JSON report with per-item decision IDs and errors (`cmd/internal/exec/json_reporter.go:11-35`), and converts outcomes to exit codes for CI gating (`cmd/exec.go:84-86`).

The core dimension question — "Can a human operator approve or block actions without reading code?" — splits cleanly for OPA: **blocking** yes, through health gates, signature verification, and reject-on-compile-error activation; **approving** no, because there is no pending-action concept anywhere in the codebase.

## Rating

**4 / 10** — Present but partial.

Rationale against the rubric: the observability half of this dimension (evidence trails, status surfacing, exception visibility) is implemented to a 7–8 standard — explicit interfaces (`Logger`, `v1/plugins/logs/plugin.go:38-43`), extensive tests (`TestMaskRuleMask` `v1/plugins/logs/mask_test.go:182`, `TestStatusUpdateBuffer` `v1/plugins/status/plugin_test.go:42`, `TestPluginPrometheus` `status/plugin_test.go:154`, `TestUnversionedGetHealth*` `v1/server/server_test.go:97-134`), and operational safeguards (fail-closed masking, buffered uploads). But the workflow half — approval dashboards, pending/approved/rejected queues, exception triage UIs, packaged evidence artifacts, bulk operator actions — is absent by design, which caps the score under this rubric. The score reflects "governance usability by humans" as scoped here, judged on the shipped artifact alone; in deployment practice much of it is delegated to ecosystem tools (documented intent: `docs/docs/management-status.md:309` mentions ecosystem projects consuming status).

## Evidence Collected

Every entry cites `path:NN`. All paths relative to `studies/agent-harness-study/sources/opa`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| No dashboard/approval concepts | Search for `dashboard\|approval\|approve` across Go sources matches only a test fixture string `access-approvers`; no HTML templates shipped | `v1/ast/index_test.go:1314` |
| Decision log event schema (evidence record) | `EventV1`: labels, decision_id, batch_decision_id, trace/span IDs, bundles+revisions, path, query, input, result, erased/masked, error, requested_by, rule_labels, request_context, custom | `v1/plugins/logs/plugin.go:49-76` |
| Pluggable log sink interface | `Logger interface { plugins.Plugin; Log(context.Context, EventV1) error }` | `v1/plugins/logs/plugin.go:38-43` |
| Console sink for zero-tooling review | `ConsoleLogs bool json:"console"` config; event written via console logger when enabled | `v1/plugins/logs/plugin.go:312`, `v1/plugins/logs/plugin.go:790-794` |
| Drop policy (operator-controlled suppression) | Default drop decision path `/system/log/drop`; `dropEvent` evals it per event | `v1/plugins/logs/plugin.go:273`, `v1/plugins/logs/plugin.go:1102-1132` |
| Mask policy (redaction before upload) | Default mask path `/system/log/mask`; `maskEvent` builds rule set from policy output and applies it | `v1/plugins/logs/plugin.go:272`, `v1/plugins/logs/plugin.go:1048-1100` |
| Mask rule mechanics | `newMaskRuleSet`, `maskRuleSet.Mask` iterate ops (remove/insert) over event paths | `v1/plugins/logs/mask.go:336-394`, `v1/plugins/logs/mask.go:128` |
| Audit trail of redaction | Events record `erased` and `masked` field paths so reviewers see what was redacted | `v1/plugins/logs/plugin.go:64-65` |
| Fail-closed masking safeguard | On mask/drop eval error, event is logged as failed and dropped rather than uploaded unmasked | `v1/plugins/logs/plugin.go:766-770`, `v1/plugins/logs/plugin.go:785-788` |
| Status aggregation model | `UpdateRequestV1{Labels, Bundle(s), Discovery, DecisionLogs, Metrics, Plugins}` snapshot | `v1/plugins/status/plugin.go:41-49` |
| Status sinks | `console` + `prometheus` config flags; service/partition upload; plugin backend | `v1/plugins/status/plugin.go:75-83` |
| Console status log ("what's wrong now") | `logUpdate` emits `type: openpolicyagent.org/status` message with full update JSON | `v1/plugins/status/plugin.go:554-568` |
| Per-bundle error surface | `bundle.Status{Name, ActiveRevision, LastSuccessfulActivation/Download/Request, Code, Message, Errors, Metrics, HTTPCode}`; `SetError` maps `ast.Errors` → `bundle_error` with per-line errors | `v1/plugins/bundle/status.go:25-39`, `v1/plugins/bundle/status.go:63-98` |
| Bulk status ingestion | `BulkUpdateBundleStatus(map[string]*bundle.Status)` feeds multi-bundle state into status channel | `v1/plugins/status/plugin.go:303-306` |
| Decision-log health surfaced | `UpdateDecisionLogsStatus(lstat.Status)`; upload failure code/message/http_code struct | `v1/plugins/status/plugin.go:313-317`, `v1/plugins/logs/status/status.go:23-28` |
| Plugin lifecycle broadcast | Manager registers status listener after loop start; initial state `StateNotReady` | `v1/plugins/status/plugin.go:234-244`, `v1/plugins/status/plugin.go:211` |
| Prometheus metrics | Collectors: `opa_info`, `plugin_status`, load counters/gauges, last-request/activation/download gauges | `v1/plugins/status/metrics.go:36-43` |
| Health gate (liveness incl. bundles/plugins) | `/health` verifies eval capability, optional bundle activation, optional all-plugins-OK with `exclude_plugin` escape hatch | `v1/server/server.go:1299-1345`, route at `v1/server/server.go:880` |
| Policy-driven health check | `/health?policy=` builds `{plugin_state, plugins_ready}` input document and evals a Rego query as pass/fail probe | `v1/server/server.go:1347-1416` |
| Loaded-policy inventory API | `GET /v1/policies[/{id}]` returns raw source + compiled AST per module | `v1/server/server.go:2151-2177`, `v1/server/server.go:2179-2214` |
| Rejected-change feedback | `PUT /v1/policies/{id}` returns 400 with structured AST errors on parse failure | `v1/server/server.go:2266-2275` |
| Decision provenance traces | `explain=full\|notes` params attach topdown trace to data responses; also on compile API | `v1/server/server.go:1813-1814`, `v1/server/server.go:1932-1933`, `v1/server/server.go:2576` |
| Decision ID correlation | Response type carries `decision_id` linking caller outcome ↔ decision-log event (docs confirm traceability contract) | `v1/server/types/types.go:267`, `docs/docs/management-decision-logs.md:66` |
| REPL operator console | Interactive commands: `trace/notes/fails` explain modes, `unknown`, `metrics`, `profile`, `partial` workflows | `v1/repl/repl.go:83-108`, `v1/repl/repl.go:364-411` |
| Machine gate replacing human approval | Bundle JWT signature verified during activation (`VerifyBundleSignature`, `verifyJWTSignature`) | `v1/bundle/bundle.go:916`, `v1/bundle/verify.go:70-118` |
| Atomic activation (block bad change) | `activate` compiles candidate bundle in transaction; failure keeps prior policy and records status error | `v1/plugins/bundle/plugin.go:607-681` |
| Bulk operation: batch evaluation CLI | `opa exec <path>...` recursively processes input files/dirs; documented manual plugin trigger order Discovery→Bundle→Status→Decision Logs | `cmd/exec.go:35-57`, `cmd/exec.go:141-166` |
| Bulk exit-code gating | `--fail`, `--fail-defined`, `--fail-non-empty` flags convert decision outcomes to non-zero exit codes | `cmd/exec.go:84-86` |
| Bulk report format | JSON reporter accumulates `{decision_id, path, error?, result?[]}` per item; error counts drive fail flags and exit codes | `cmd/internal/exec/json_reporter.go:11-16`, `cmd/internal/exec/json_reporter.go:45-54`, `cmd/internal/exec/exec.go:32` |
| Tests proving operator surfaces | Mask rule behavior; status buffering; Prometheus export; health endpoint variants | `v1/plugins/logs/mask_test.go:182`, `v1/plugins/status/plugin_test.go:42`, `v1/plugins/status/plugin_test.go:154`, `v1/server/server_test.go:97-134` |

## Answers to Dimension Questions

**1. Can operators see what needs review?**
There is no "needs review" queue because nothing pends. The nearest equivalent is *seeing what is broken or stale*: bundle statuses expose last-successful download/activation timestamps, active revision, and error code/message/stack (`v1/plugins/bundle/status.go:25-39`); decision-log upload failures surface code/message/HTTP status (`v1/plugins/logs/status/status.go:23-28`); plugin states stream to listeners (`v1/plugins/status/plugin.go:234-244`). An operator watching console logs (`openpolicyagent.org/status` events, `v1/plugins/status/plugin.go:554-568`) or Prometheus gauges (`v1/plugins/status/metrics.go:38-43`) sees degraded components immediately, but must bring their own alerting/review tooling on top.

**2. Can they act on approvals efficiently?**
No approve/reject actions exist to act on. The efficient-action story is different: policy *changes* are approved out-of-band and enforced mechanically — signed bundles verify at activation (`v1/bundle/bundle.go:916`), and a bad bundle fails compilation atomically while the previous policy stays serving (`v1/plugins/bundle/plugin.go:675-681`). For bulk work, `opa exec` evaluates thousands of inputs in one invocation with CI-friendly exit codes (`cmd/exec.go:84-86`), letting an operator validate a policy change against a corpus before publishing. There is no bulk approve/reject of decisions or exceptions; no evidence found for anything matching those verbs.

**3. Are exceptions surfaced?**
Yes, as structured telemetry rather than a triage UI: activation failures carry parsed AST error lists (`v1/plugins/bundle/status.go:77-84`), HTTP download failures carry status codes (`status.go:86-90`), decision-log delivery failures carry code/message/http_code (`v1/plugins/logs/status/status.go:23-28`), and the health endpoints condense all of it into orchestrator-consumable pass/fail with an `exclude_plugin` parameter to tolerate known-degraded components (`v1/server/server.go:1304-1308`). Notably, masking/drop policy evaluation failures are fail-closed — the event is discarded rather than leaked unmasked (`v1/plugins/logs/plugin.go:785-788`) — and the failure itself is logged server-side, though nothing re-queues the lost event.

**4. Is the governance UI usable under pressure?**
There is no UI; under incident pressure an operator relies on: (a) `curl /health` for instant go/no-go (`v1/server/server.go:1299`); (b) console decision/status logs for a single-process, zero-dependency view (`v1/plugins/logs/plugin.go:790`, `v1/plugins/status/plugin.go:564-566`); (c) `GET /v1/policies` to diff what actually loaded vs. what was expected (`v1/server/server.go:2179`); (d) `opa eval --explain`/REPL `trace` to replay a contested decision (`v1/repl/repl.go:378-394`). This is usable for diagnosis but requires fluency in OPA's APIs and Rego — an operator cannot answer "why was this request denied?" without reading either policy code or a trace. That is acceptable for a PDP but would score low if OPA claimed to be an operator-facing governance product.

## Architectural Decisions

- **Separation of PDP and control plane**: OPA ships decision machinery plus telemetry exports; approval workflow, dashboards, and queues are delegated to external systems that consume decision logs, status updates, and Prometheus metrics (`v1/plugins/logs/plugin.go:5-6` package comment "decision log buffering and uploading"; ecosystem note `docs/docs/management-status.md:309`). Every operator-facing surface has both a local (console/Prometheus) and remote (HTTP service) sink, so governance visibility works with or without an external platform.
- **Machine-enforced change gates instead of human review steps**: trust is established cryptographically at deploy time — bundle signatures verified during activation (`v1/bundle/bundle.go:916`) — and bad changes are rejected atomically at compile (`v1/plugins/bundle/plugin.go:607-681`). Human judgment moves upstream (authoring, testing, signing), keeping the runtime free of interactive pauses.
- **Policy-governed audit trail**: even the evidence trail is governed by policy — mask and drop rules are themselves Rego decisions evaluated per event (`v1/plugins/logs/plugin.go:1048-1132`), giving operators a programmable redaction layer with an auditable residue (`erased`/`masked` fields, `plugin.go:64-65`).

## Notable Patterns

- **Snapshot-and-push status aggregation**: components push typed status deltas into channels; the status plugin folds them into a versioned `UpdateRequestV1` snapshot available on demand (`Snapshot()` at `v1/plugins/status/plugin.go:330`) or pushed periodically — a clean pattern for multi-source operator visibility.
- **Correlation-ID threading end-to-end**: `decision_id` generated per query appears in the API response (`v1/server/types/types.go:267`), the decision-log event (`v1/plugins/logs/plugin.go:52,724`), and batch reports (`cmd/internal/exec/json_reporter.go:11`), enabling join-free cross-surface investigation.
- **Exit-code-as-policy-verdict**: `opa exec`'s `--fail*` flags translate decision outcomes into process exit codes (`cmd/exec.go:84-86`), making governance checks composable into shell pipelines and CI where no dashboard exists.
- **Dual-mode everything**: nearly every governance surface supports console mode for small/local deployments and service mode for centralized ops (`v1/plugins/logs/plugin.go:312`, `v1/plugins/status/plugin.go:79`), including a documented backwards-compat default to the first configured service (`v1/plugins/status/plugin.go:100-111`).

## Tradeoffs

- **No inbox means no accountability chain**: because there are no pending/approved states, OPA cannot answer "who approved this policy?" — that lives entirely in external VCS/signing infrastructure. Teams without that surrounding tooling get none of this dimension's workflow value.
- **Fail-closed masking trades completeness for safety**: a mask-policy evaluation error silently drops the decision event (`v1/plugins/logs/plugin.go:786-787` returns nil after logging); auditors lose the record entirely rather than receiving an unmasked copy — correct for privacy, lossy for forensics, and there is no dead-letter queue.
- **Telemetry richness vs. operator learning curve**: the console/Prometheus/API surfaces are powerful but textual and API-first; under pressure, an operator unfamiliar with Rego traces cannot easily interpret `explain=full` output, unlike a purpose-built review UI.
- **Health endpoint conservatism**: `/health` with `plugins=true` fails if *any* non-excluded plugin is not OK (`v1/server/server.go:1330-1341`); the `exclude_plugin` param mitigates flapping dependencies but requires operators to know and maintain exclusion lists.

## Failure Modes / Edge Cases

- **Lost decision events on shutdown**: `Stop` drains the buffer with a deadline and explicitly warns "Plugin stopped with decisions possibly still in buffer." (`v1/plugins/logs/plugin.go:704-712`) — evidence gaps possible during restarts.
- **Drop/mask misconfiguration is silent-ish**: if the configured mask/drop policy path doesn't exist or errs, events flow unmasked (mask) or are all logged as failed (drop error path returns nil, `plugin.go:766-770`); detection depends on reading error logs, since no dedicated status alarm distinguishes "no rules matched" from "policy broken".
- **Bundle error status granularity**: `SetError` collapses non-AST/non-HTTP failures into a bare `message` string with nil `Errors` (`v1/plugins/bundle/status.go:92-97`), limiting root-cause detail for some failure classes.
- **Batch evaluation partial failure**: `opa exec` reports per-item errors in the JSON report and counts them (`cmd/internal/exec/json_reporter.go:53-54`), but a malformed item does not abort the run — appropriate for bulk analysis, surprising if an operator expects all-or-nothing semantics.
- **Stale-policy window**: activation failure leaves the old policy serving with an error status (`v1/plugins/bundle/plugin.go:675-681`); availability is preserved but operators may not notice they are running outdated policy unless monitoring bundle `last_successful_activation` (`v1/plugins/bundle/status.go:28`).

## Future Considerations

- A dead-letter path for decision-log events lost to mask/drop evaluation errors or shutdown races, so audit gaps become visible countable metrics instead of log lines.
- Exposing mask/drop policy *evaluation* health (e.g., "mask policy errored N times") as a first-class status field alongside `lstat.Status` (`v1/plugins/logs/status/status.go:23-28`).
- Packaging a self-contained "evidence pack" export (decision event + explain trace + active bundle revision + policy hash keyed by `decision_id`) to serve external review workflows that today must stitch together `/v1/data?explain=full` responses and decision logs manually.
- Documented reference integration showing how a control plane consumes `UpdateRequestV1` and decision-log streams to build the review queue/dashboard that this dimension expects — the extension points already exist (`Logger` interface at `v1/plugins/logs/plugin.go:38-43`, plugin-backend config at `v1/plugins/logs/plugin.go:800-818`).

## Questions / Gaps

- **Approval workflow**: No clear evidence found — searched `dashboard|Dashboard`, `approval|approve`, `review queue`, and HTML/template assets across the entire source tree; only incidental test-string matches (`v1/ast/index_test.go:1314`). Conclusion: intentionally absent, consistent with OPA's PDP role and its GOVERNANCE.md being about project maintainership, not product feature.
- **Pending/approved/rejected action states**: No evidence found beyond bundle *revision* tracking (`ActiveRevision`, `v1/plugins/bundle/status.go:27`) and reject-at-compile behavior (`v1/server/server.go:2266-2275`); there is no persisted state machine for human-reviewable actions.
- **Evidence-pack generation**: Partial evidence only — per-decision `EventV1` records (`v1/plugins/logs/plugin.go:49-76`) and explain traces (`v1/server/server.go:2576`) exist, but no mechanism packages these into a reviewer-ready artifact.
- **Usability-under-pressure validation**: No UX testing or operator studies found in-repo (searched `e2e/`, which contains only API/CLI protocol tests per `e2e/README.md`); usability claims here rest on API ergonomics observed in code, not user evidence.

---

Generated by `09.03-governance-ux-and-operator-workflow` against `opa`.

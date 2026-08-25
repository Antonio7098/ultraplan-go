# Source Analysis: opa

## Dimension 10.03: Causal Links and Lineage

### Source Info

| Field | Value |
|-------|-------|
| Name | opa (Open Policy Agent) |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | Go (policy engine: Rego evaluator, HTTP server, Go SDK, bundle distribution) |
| Analyzed | 2026-08-25 |

## Summary

OPA is not an LLM agent harness, so the dimension's vocabulary maps onto policy-evaluation equivalents: "outputs" are policy decisions (`result` values returned by the Data/Query APIs and SDK), "inputs" are request input documents plus queries, "tools" are built-in function calls, "retrieved context" is bundle data/policies, and "model versions" translate to OPA build version + bundle revisions. Under that mapping, causal linkage is a first-class design goal implemented through four cooperating mechanisms:

1. **Decision log events** (`v1/plugins/logs/plugin.go:49-76`) that atomically bind a `decision_id`, the echoed `input`, the `result`, the evaluated path/query, per-bundle revisions, trace/span IDs, and metrics into one record. The same `decision_id` is returned in the API response (`v1/server/types/types.go:267`), annotated on the OpenTelemetry span (`v1/server/server.go:99`, `v1/server/server.go:3212-3215`), and stamped on every decision-log event (`v1/server/server.go:3135`, `v1/server/server.go:3149`).
2. **Query explain traces** with explicit parent-child query IDs (`v1/topdown/trace.go:82-96`) filtered by the dedicated `lineage` package, which reconstructs the Enter/Redo event path leading to any Note/Fail event by walking the `ParentID` chain (`v1/topdown/lineage/lineage.go:48-86`).
3. **Non-deterministic builtin cache** (`nd_builtin_cache`) recording every invocation of non-deterministic builtins as input→output mappings (`v1/topdown/builtins/builtins.go:37-66`, populated at `v1/topdown/eval.go:2166-2171`), explicitly intended for debugging and decision replay (`docs/docs/management-decision-logs.md:81`).
4. **Provenance payloads** combining OPA build identity (version/commit/timestamp) with active bundle revisions (`v1/server/types/types.go:225-238`, assembled at `v1/server/server.go:2766-2787`) — the closest analog to "model version" lineage.

Redaction transformations are self-documenting: masked/erased JSON pointers are recorded back onto the event itself (`v1/plugins/logs/mask.go:197`, fields at `v1/plugins/logs/plugin.go:64-65`). The main gaps are that data documents in the store carry no per-value provenance through transformations, explain traces are captured only synchronously per request and never persisted, decision-log events omit engine build provenance (only bundle revisions), and authorization decisions on the same request are not linked into the decision log.

## Rating

**7 / 10** — Clear lineage model with explicit interfaces (`EventV1`, `TraceEventV1`, `ProvenanceV1`), test coverage (e.g., `TestDecisionIDs` at `v1/server/server_test.go:4324`, provenance tests at `v1/server/server_test.go:3221-3340`, ND-cache tests in `v1/plugins/logs/plugin_test.go`), and operational safeguards (mask/drop policies, upload-size degradation strategy with metric counters). It falls short of 8–9 because lineage is per-decision only: traces are not persisted alongside events, store data has no per-node provenance, decision-log events lack engine-version fields, and intermediate evaluation results sit behind an opt-in env var.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Decision ID generation (server) | Per-request `generateDecisionID()`, injected via factory; wired from runtime which only generates IDs when the decision logger plugin is enabled | `v1/server/server.go:1526-1528`, `v1/server/server.go:2759-2764`, `v1/runtime/runtime.go:955-963` |
| Decision ID in context + span | `logging.WithDecisionID` carries the ID through the request; OTel span gets `opa.decision_id` attribute | `v1/logging/logging.go:276-283`, `v1/server/server.go:99`, `v1/server/server.go:3212-3215` |
| Output→input binding | `EventV1` binds `decision_id`, `input`, `result`, `path`, `query`, `labels`, `bundles`, `timestamp`, `requested_by`, `req_id` in one record | `v1/plugins/logs/plugin.go:49-76` |
| Response echo of decision ID | `DataResponseV1.DecisionID` returned to caller on Data API GET/POST | `v1/server/types/types.go:267`, `v1/server/server.go:1644-1646` |
| Server-side event assembly | `decisionLogger.Log` copies input (AST + raw), results, error, metrics, rule labels, bundle revisions into `server.Info` | `v1/server/server.go:3108-3193`, `v1/server/buffer.go:18-48` |
| SDK decision IDs | SDK generates UUID if unset, returns it in `DecisionResult.ID`, logs via same plugin | `v1/sdk/opa.go:394-402`, `v1/sdk/opa.go:425-440` |
| Trace event parent chain | `topdown.Event` has `QueryID`/`ParentID`, AST node, source location, local bindings | `v1/topdown/trace.go:82-96` |
| Lineage reconstruction | `lineage.Filter` walks ParentID chain backwards to build the causal path to Note/Fail events; `Full`/`Notes`/`Fails`/`Debug` modes | `v1/topdown/lineage/lineage.go:14-86` |
| Explain API surface | `explain=notes/fails/full/debug` parameter mapped to lineage filters; served in response `explanation` field | `v1/server/server.go:2576-2600`, `v1/server/server.go:2905-2921`, `v1/server/types/types.go:330-334` |
| Trace event wire schema | `TraceEventV1` exposes `op`, `query_id`, `parent_id`, `type`, `node`, `locals`, `message` | `v1/server/types/types.go:425-434` |
| Builtin call provenance | `NDBCache` stores per-builtin input→output objects; written after each non-deterministic builtin call; consulted before re-invocation | `v1/topdown/builtins/builtins.go:37-66`, `v1/topdown/eval.go:2166-2171`, `v1/topdown/eval.go:2116-2132` |
| ND cache in logs | Event field `nd_builtin_cache` populated when enabled; gated on server flag + plugin presence | `v1/plugins/logs/plugin.go:63`, `v1/server/server.go:1571-1574`, `v1/runtime/runtime.go:728-733` |
| Evaluated-rule lineage | `EvaluatedRuleTracker` records merged annotation labels of successfully evaluated rules → surfaced as `rule_labels` in events | `v1/topdown/evaluated.go:11-50`, `v1/topdown/eval.go:2400`, `v1/plugins/logs/plugin.go:71`, `v1/server/server.go:3160` |
| Bundle revision lineage | Manifest `revision`, `roots`, `rego_version`, per-file rego versions, metadata | `v1/bundle/bundle.go:147-165` |
| Provenance payload | `ProvenanceV1` = OPA version/build commit/timestamp/hostname + per-bundle revisions; returned when `?provenance=true`; also returned by SDK decisions | `v1/server/types/types.go:225-238`, `v1/server/server.go:2766-2787`, `v1/server/server.go:1533`, `v1/sdk/opa.go:387` |
| Distributed tracing correlation | Event records W3C-compliant `trace_id`/`span_id` from the active OTel span | `v1/server/server.go:3172-3176`, `docs/docs/management-decision-logs.md:67` |
| Self-documenting redaction | Mask upserts append the applied pointer to `event.Masked`; removals recorded as `erased` pointers on the event | `v1/plugins/logs/mask.go:197`, `v1/plugins/logs/plugin.go:64-65` |
| Drop/mask policies evaluated over event AST | Mask/drop rules are Rego queries against the serialized event (`AST()` conversion keeps lineage fields addressable) | `v1/plugins/logs/plugin.go:1048-1100`, `v1/plugins/logs/plugin.go:100-249` |
| Artifact-run association | Bundle status tracks `active_revision`, `last_successful_activation/download/request` per bundle, reported by status plugin | `v1/plugins/bundle/status.go:25-46`, `v1/plugins/status/plugin.go:42-48` |
| Degradation safeguard under size limits | Encoder drops `nd_builtin_cache` first when chunks exceed `upload_size_limit_bytes`, incrementing a drop-counter metric | `v1/plugins/logs/encoder.go:105-120`, `v1/plugins/logs/encoder.go:179-208`, `docs/docs/management-decision-logs.md:109-110` |
| Tests: ID propagation & provenance | `TestDecisionIDs` (IDs 1..4 across GET/POST), `TestDataProvenanceSingleBundle/MultiBundle`, explain tests | `v1/server/server_test.go:4324-4374`, `v1/server/server_test.go:3221-3341`, `v1/server/server_test.go:3000-3200` |
| Tests: lineage filter | `TestFilter` validates parent-chain reconstruction | `v1/topdown/lineage/lineage_test.go:16` |

## Answers to Dimension Questions

**1. Can every output be traced to its inputs?**
Yes, within a single decision, when decision logging is enabled. The `EventV1` record binds result, echoed input (both raw Go value and AST), query/path, labels, and bundle revisions under one `decision_id` (`v1/plugins/logs/plugin.go:49-76`); the same ID is returned to the caller (`v1/server/server.go:1644-1646`) and attached to the OTel span (`v1/server/server.go:3212-3215`). Caveats: (a) decision IDs are generated *only* when the decision logger plugin or a custom factory is configured (`v1/runtime/runtime.go:955-963`), so deployments without logging get no linkage; (b) Query API events log the query text but not per-result-row attribution beyond `rule_labels`.

**2. Is provenance preserved through transformations?**
Partially. Three transformation points are covered: (i) masking/redaction records exactly which JSON pointers were erased or upserted on the event itself (`v1/plugins/logs/mask.go:197`, `v1/plugins/logs/plugin.go:64-65`); (ii) non-deterministic builtin calls are journaled as input→output pairs, preserving the origin of values like `http.send` responses and `rand`/`uuid` outputs even under caching/replay (`v1/topdown/eval.go:2166-2171`); (iii) partial-evaluation/save operations emit `SaveOp` trace events distinguishing saved-not-evaluated expressions (`v1/topdown/trace.go:45-47`). Not covered: base/data documents carry no per-node provenance once patched into the store, so a value read from `data.*` cannot be attributed to the bundle, file, or write request that produced it.

**3. Are model versions tracked in lineage?**
No models exist; the analogs are tracked unevenly. Engine build identity (version, build commit, timestamp, hostname) and active bundle revisions are available via `ProvenanceV1` on API responses (`?provenance=true`, `v1/server/server.go:1533`, `v1/server/server.go:2766-2787`) and in SDK `DecisionResult.Provenance` (`v1/sdk/opa.go:387`). Bundles additionally pin their Rego language dialect (`rego_version`, `file_rego_versions` at `v1/bundle/bundle.go:151-161`). However, the decision log event itself does **not** embed engine build provenance — it carries bundle revisions (`v1/plugins/logs/plugin.go:56`, `v1/plugins/logs/plugin.go:728-729`) but no OPA version field, so correlating an old event with the binary that produced it requires out-of-band knowledge.

**4. Can causal chains be audited?**
Yes, with three complementary views: (i) synchronous per-request explain traces exposing op/query_id/parent_id/local-bindings chains (`v1/server/server.go:2576-2600`, `v1/server/types/types.go:425-434`) reconstructed via the lineage package's ParentID walk (`v1/topdown/lineage/lineage.go:48-86`); (ii) durable decision logs correlated across services via W3C `trace_id`/`span_id` (`v1/server/server.go:3172-3176`); (iii) replay support through `nd_builtin_cache` (`docs/docs/management-decision-logs.md:81`). Limitation: explain traces live only in a per-request `BufferTracer` (`v1/server/server.go:1576-1580`) and are never persisted, so auditing historical decisions relies solely on what the (possibly masked/dropped) log event retained.

## Architectural Decisions

- **One flat, self-contained event schema instead of a linked event graph.** All causally relevant context (input, result, query, bundles, trace IDs) is denormalized into `EventV1` (`v1/plugins/logs/plugin.go:49-76`) rather than referenced by foreign keys. This makes each event independently auditable but duplicates inputs/results and grows payloads (hence the size-limit machinery).
- **Lineage as post-hoc filtering of a complete eval trace.** The tracer records everything; `lineage.Full/Notes/Fails/Debug` derive views (`v1/topdown/lineage/lineage.go:14-43`). Filtering reconstructs ancestor paths lazily from `ParentID`s instead of maintaining tree structures during evaluation.
- **Provenance-by-recording for non-determinism.** Rather than banning non-deterministic builtins, OPA journals their operand/output pairs in `NDBCache` and reuses cached entries on re-invocation within a decision (`v1/topdown/eval.go:2116-2132`, `v1/topdown/eval.go:2166-2171`), making replays deterministic and auditable.
- **Policy-governed logging.** Whether an event is logged at all (`dropEvent`) and how it is redacted (`maskEvent`) are themselves Rego queries evaluated against the event AST (`v1/plugins/logs/plugin.go:1048-1100`) — the same mechanism being audited governs its own audit trail, with redactions recorded in-band.
- **Opt-in lineage depth.** Decision IDs only when logging is enabled (`v1/runtime/runtime.go:955-963`), `nd_builtin_cache` behind a config flag (`v1/runtime/runtime.go:728-733`), intermediate results behind `OPA_DECISIONS_INTERMEDIATE_RESULTS` (`v1/server/server.go:105`).

## Notable Patterns

- **Context-key correlation:** decision IDs flow through `context.Context` accessors (`v1/logging/logging.go:274-294`) so deeply stacked layers (HTTP handler → rego → topdown) can stamp the same ID without parameter threading.
- **Dual representation for policy-addressability:** the event is converted to an AST object (`EventV1.AST()`, kept manually in sync per the warning comment at `v1/plugins/logs/plugin.go:46-48`) so mask/drop policies can introspect every lineage field natively.
- **Deprecated-field migration discipline:** `Revision`/legacy single-bundle fields coexist with `Bundles` maps in both events (`v1/plugins/logs/plugin.go:55-56`) and provenance responses (`v1/server/server.go:2774-2784`), showing lineage-schema evolution handled without breaking consumers.
- **Graceful degradation with observability:** when uploads exceed size limits, the encoder drops `nd_builtin_cache` first and counts it via a Prometheus counter (`v1/plugins/logs/encoder.go:21-22`, `v1/plugins/logs/encoder.go:179-208`) — provenance loss is explicit, not silent.

## Tradeoffs

- **Auditability vs payload cost:** echoing full inputs/results plus optional ND caches makes events large; mitigation is chunked uploads and deterministic cache dropping, at the price of incomplete replay fidelity for oversized decisions (`v1/plugins/logs/encoder.go:105-120`).
- **Privacy vs completeness:** masking protects sensitive inputs but the audit trail then contains redacted values; recording `erased`/`masked` pointers preserves the *shape* of what was removed while destroying content (`v1/plugins/logs/mask.go:197`).
- **Synchronous explain vs durability:** capturing traces per-request avoids storage overhead but means the richest causal view exists only while the request is alive; production auditing must rely on flatter log events.
- **Denormalization vs consistency:** because each event snapshots bundle revisions at decision time, long-lived audits can reconstruct which policy version produced any result — but there is no join back to the exact module contents unless the operator retains bundles externally.

## Failure Modes / Edge Cases

- **Silent absence of IDs:** if neither the decision logger nor a factory is configured, `generateDecisionID` returns "" (`v1/runtime/runtime.go:959-962`); responses then lack `decision_id` entirely, breaking downstream correlation.
- **UUID generation failure:** `generateDecisionID` swallows the uuid error and returns "" (`v1/runtime/runtime.go:1152-1158`); similarly the SDK aborts the decision on UUID failure (`v1/sdk/opa.go:394-400`).
- **Mask/drop policy errors drop the event:** if masking fails, the event is discarded with only a log message (`v1/plugins/logs/plugin.go:785-788`) — a lineage gap triggered by policy bugs, though evaluation is decoupled from client cancellation (`v1/server/server.go:3184-3189`).
- **ND-cache loss under size pressure:** large `nd_builtin_cache` payloads are dropped to fit upload limits (`v1/plugins/logs/encoder.go:189-208`), making replay impossible for exactly the largest/most complex decisions.
- **Undefined/failed evaluations still logged with `error`** but without results (`v1/server/server.go:1638-1642`), so causal chains terminate in errors rather than outputs.
- **Authz blind spot:** per-request authorization decisions (authorizer middleware) are not emitted as decision-log events (no logging references found in `v1/server/authorizer/authorizer.go`), so the chain "who was allowed to ask → what was decided" cannot be fully audited from logs alone.

## Future Considerations

- Embed engine build provenance (`version`/`build_commit`) directly into `EventV1` so historical events self-describe the producing binary, matching what `ProvenanceV1` already offers on live responses (`v1/server/types/types.go:225-233`).
- Persist or reference explain traces (e.g., trace-ID-addressable trace archives) so post-hoc audits of logged decisions can retrieve full causal paths, not just summaries.
- Add per-node data provenance to the store (bundle/file/write-origin tags on data paths) to close the largest provenance gap identified above.
- Emit authorization decisions as regular decision-log events (distinct path) to unify approval/gating chains with the decisions they gate.
- Promote intermediate-results capture from env-var experiment to supported configuration now that `intermediate_results` is a stable event field (`v1/plugins/logs/plugin.go:61`, `v1/server/server.go:105`).

## Questions / Gaps

- No evidence found for human-in-the-loop approvals anywhere in the codebase; searches across `v1/` for approval workflows returned nothing (closest artifact-integrity control is bundle signature verification tying artifacts to signing key IDs, `v1/bundle/verify.go:70-169`).
- No evidence found that batch decisions are generated in this codebase: `BatchDecisionID` fields exist (`v1/plugins/logs/plugin.go:52`, `v1/logging/logging.go:285-294`) and flow into events (`v1/plugins/logs/plugin.go:725`), but no in-repo producer sets `WithBatchDecisionID` outside tests — presumably consumed by downstream distributions (e.g., Enterprise OPA).
- The `mapped_result` field (`v1/plugins/logs/plugin.go:62`) appears in the event schema, but the searched server/SDK paths populate `MappedResults` only structurally (`v1/server/buffer.go:35`); the mapping hook's activation path was not traced further.
- Whether WASM-target evaluations produce equivalent lineage (traces, rule labels) was not verified; explain machinery is implemented in the Go evaluator (`v1/topdown/`), and no equivalent tracer was found for the WASM path (`v1/wasm/`).

---

Generated by `10.03-causal-links-and-lineage` against `opa`.

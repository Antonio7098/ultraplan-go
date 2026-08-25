# Source Analysis: temporal

## Security Auditability

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go; gRPC frontend API; Cassandra/MySQL/PostgreSQL + Elasticsearch/OpenSearch persistence; OTel metrics/logs |
| Analyzed | 2026-08-24 |

## Summary

Temporal approaches auditability primarily through its **workflow execution history**: every state change to a workflow is appended as an immutable, sequentially numbered `HistoryEvent` to an append-only, transaction-chained storage table (`schema/cassandra/temporal/schema.cql:58-70`), retrievable through `GetWorkflowExecutionHistory` (`service/frontend/workflow_handler.go:960-1023`) and exportable to external object storage via the archival subsystem (`service/history/archival/archiver.go:30-86`). Actor attribution rides inside those events: caller-supplied `Identity` strings are validated and threaded from the frontend into history-event attributes for workflow-task starts/completions and activity completions (`service/frontend/workflow_handler.go:1664-1666`, `service/history/workflow/workflow_task_state_machine.go:796-840`, `service/history/workflow/mutable_state_impl.go:4604-4610`). Bulk operator actions (batch terminate/cancel/signal/reset) additionally stamp the operator into an indexed `BatcherUser` search attribute plus a reason memo, making them queryable in the visibility store (`service/frontend/workflow_handler.go:5917-5953`, `common/searchattribute/sadefs/constants.go:29`).

At the API boundary there is a real authorization framework — pluggable claim mapper + authorizer wired as gRPC interceptors (`common/authorization/interceptor.go:129-238`, `service/frontend/fx.go:286-326`) with a deny-by-default role model (`common/authorization/default_authorizer.go:35-65`) and strong anti-spoofing of principal headers (`common/headers/headers.go:125-143`). However, the *security-specific* audit artifacts are thin: policy decisions are counted as metrics but never persisted with a decision ID or full context (`common/authorization/interceptor.go:304-331`); namespace mutation "wide events" are emitted without any caller identity and are doubly disabled by default (noop logger provider + dynamic-config gate: `temporal/fx.go:222-224`, `common/dynamicconfig/constants.go:239-242`); there is no human-approval mechanism anywhere in the server; and out of the box both authorizer and claim mapper default to allow-all/admin-for-everyone (`common/authorization/authorizer.go:64-73`, `common/authorization/claim_mapper.go:52-54`), meaning recorded identities are self-asserted unless operators configure JWT/mTLS.

**Bottom line:** auditors can reconstruct *what happened to each workflow* exceptionally well, and *which policy allowed an API call* only if they control the authorizer plugin themselves — the server ships no policy-decision record, no approval trail, and no default-on security event stream.

## Rating

**7 / 10** — Clear, tested model for action traceability (immutable event-sourced history with identity fields, archival, integrity checksums, batch-operator attribution, well-tested authz interceptors), but the security-specific layers are inconsistent: authorization denials leave no persistent record beyond a metric counter, lifecycle wide-events lack actor identity and ship disabled, and there is no approval workflow. The system can answer "who did what to this workflow" after the fact, but cannot natively answer "what policy allowed it and was a human involved."

## Evidence Collected

Every entry includes a file path with line numbers relative to `sources/temporal/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Append-only history storage | `history_node` table stores event batches keyed by `(tree_id, branch_id, node_id, txn_id)` with `prev_txn_id` back-pointer forming a chain; comments note branches fork on reset rather than rewrite | `schema/cassandra/temporal/schema.cql:58-70` |
| Execution record storage | `executions` table row types include Shard/Execution/TransferTask/TimerTask/ReplicationTask/VisibilityTask | `schema/cassandra/temporal/schema.cql:7-19` |
| History read API | `GetWorkflowExecutionHistory` paginated read with archived-history fallback | `service/frontend/workflow_handler.go:960-1023` |
| Archival read path | If namespace archival read enabled and run already archived, serve history from archive | `service/frontend/workflow_handler.go:991-997` |
| Archival request payload | Archival `Request` carries BranchToken, NextEventID, CloseFailoverVersion, workflow metadata, targets `history` and/or `visibility` | `service/history/archival/archiver.go:30-86` |
| Identity captured on task completion | `request.Identity` written into `ActivityTaskCompletedEventAttributes` via history builder | `service/history/workflow/mutable_state_impl.go:4604-4610` |
| Identity captured on workflow tasks | Worker `Identity` recorded in `WorkflowTaskStartedEvent` (line 796-807) and `WorkflowTaskCompletedEvent` (829-840) | `service/history/workflow/workflow_task_state_machine.go:796-840` |
| Heartbeat identity retention | Last worker identity stored in activity info (`RetryLastWorkerIdentity`) | `service/history/workflow/mutable_state_impl.go:2105-2120` |
| Server-side identity defaults | `defaultUserTerminateIdentity = "frontend-service"` substituted when clients omit identity on terminate | `service/frontend/workflow_handler.go:113`, `service/frontend/workflow_handler.go:2493` |
| Identity validation | Identity length limits enforced on signal/update/terminate/pause APIs | `service/frontend/workflow_handler.go:1087-1088`, `1226-1227`, `1289-1290`, `1361-1362` |
| System actor identities | Reserved identities `history-service` and `history-resetter` distinguish system-initiated mutations | `service/history/consts/const.go:13-14`; checked at `service/history/ndc/workflow_resetter.go:1081`, `1380` |
| Batch operator attribution | Operator identity placed into `BatcherUser` search attribute + reason/type/executions into memos of the batch-operation workflow | `service/frontend/workflow_handler.go:5917-5953`; admin variant `service/frontend/admin_handler.go:1360-1405` |
| `BatcherUser` is a first-class index | Predefined keyword search attribute `BatcherUser` (and ES mapping field) enabling "find all bulk ops by user X" queries | `common/searchattribute/sadefs/constants.go:29,159`; `schema/elasticsearch/visibility/versioned/v14/index_template_v7.json:57` |
| Per-target propagation | Batch activities propagate `Identity` + `Reason` into each target workflow's terminate/signal/reset calls | `service/worker/batcher/activities.go:651-652`, `698-750`, `760-826` |
| Authorization interception point | `authInterceptor.Intercept` sits mid-chain in the frontend unary interceptor order; stream auth via `InterceptStream` | `service/frontend/fx.go:286-315`, `323-326` |
| Auth info extraction | AuthInfo assembled from bearer header, TLS subject cert, extra data, audience | `common/authorization/interceptor.go:251-285` |
| Decision result shape | `Result{Decision, Reason, Principal}` — has reason and server-computed principal but **no decision ID and no sink** | `common/authorization/authorizer.go:38-50` |
| Deny handling = metrics only | On deny: increments `ServiceErrUnauthorizedCounter` and returns generic error; no log line, no persisted record. Only internal authorizer errors get logged ("Authorization error") | `common/authorization/interceptor.go:304-331`; counter defined at `common/metrics/metric_defs.go:694` |
| Default role model | Health APIs open; nil claims denied; system/namespace Admin>Writer>Reader roles mapped against per-API scope/access metadata; deny-by-default fallthrough | `common/authorization/default_authorizer.go:35-65`; API permission map helpers `common/authorization/frontend_api.go:13-27` |
| Anti-spoofing | Inbound principal headers always stripped before authorization; verified principal optionally re-set downstream (`EnablePrincipalPropagation`) | `common/authorization/interceptor.go:156-158,175-177,219-221`; `common/headers/headers.go:125-143` |
| Cross-namespace authorization | Commands in `RespondWorkflowTaskCompleted` targeting other namespaces re-authorized per target namespace+API | `common/authorization/interceptor.go:347-417` |
| Defaults are open | Empty `authorizer` config ⇒ `noopAuthorizer` (allow-all); empty `claimMapper` ⇒ admin claims for everyone; server warns without `--allow-no-auth` | `common/authorization/authorizer.go:64-73`; `common/authorization/noop_authorizer.go:12-14`; `common/authorization/claim_mapper.go:42-59`; `cmd/server/main.go:197-209` |
| JWT subject extraction | Default JWT claim mapper maps `sub` to `Claims.Subject` and permissions claim to roles — the only built-in cryptographic identity source | `common/authorization/default_jwt_claim_mapper.go:78-110` |
| Config example | `global.authorization` with `default` authorizer/claimMapper and JWKS key source | `config/development-jwt.yaml:66-71` |
| Namespace lifecycle wide events | `namespace_registered` / `namespace_updated` / `namespace_renamed` OTEL log events with before/after snapshots; explicitly exclude `security_token` from payloads | `common/wideevents/namespace_lifecycle_events.go:240-379`; builders `service/frontend/namespace_lifecycle_events.go:186-223` |
| Wide events disabled by default twice | Emitter gated by `EmitNamespaceLifecycleEvents` (default off); OTEL LoggerProvider defaults to noop unless `WithCustomEventLoggerProvider` set | `common/dynamicconfig/constants.go:239-242`; `temporal/fx.go:219-224`; `temporal/server_option.go:246-251`; noop emitter `common/wideevents/events.go:36-38` |
| **No actor identity in lifecycle events** | `NamespaceUpdatedInput` carries before/after/requested fields but no caller/principal field; `UpdateNamespace` handler passes no identity into the event | `common/wideevents/namespace_lifecycle_events.go:329-346`; `service/frontend/namespace_handler.go:634-641` |
| Workflow-rule creator record | Rare non-history attribution: `CreatedByIdentity` persisted on workflow rules | `service/frontend/namespace_handler.go:733-786` |
| Integrity checksums | CRC32 over mutable-state snapshot generated on update and verified on load (mismatch logged + metric; errors currently ignored pending tooling) | `service/history/workflow/checksum.go:15-36`; `service/history/workflow/mutable_state_impl.go:557-571`, `9126`; payload schema `api/checksum/v1/message.pb.go:28-56` |
| Checksum sampling knobs | Dynamic config probabilities for checksum generation/verification | `common/dynamicconfig/constants.go:2779-2785` |
| Per-request access logging (debug-only) | Logs method, namespace, TLS server name and client-cert thumbprint — at Debug level only | `common/rpc/interceptor/namespace_logger.go:40-59` |
| HTTP API parity | REST gateway forwards TLS peer info and `Authorization-Extras` header into the same gRPC auth path | `service/frontend/http_api_server.go:53`, `256-271` |
| DLQ review surface | Replication/DLQ message inspection and merge APIs exposed on history handler for operator reconciliation | `service/history/handler.go:1536-1602` |
| Tests: interceptor behavior | Suite covers unauthorized/unknown callers, error masking vs exposure, alternate headers, cross-namespace dedup, principal spoof stripping, stream auth | `common/authorization/interceptor_test.go:105-165`, `340-540`, `591-612`, `663-854` |
| Tests: wide events | Before/after snapshot and failover flag assertions | `common/wideevents/namespace_lifecycle_events_test.go:199+` |

## Answers to Dimension Questions

### 1. Who did what?

**Mostly yes, with an important caveat.** Every workflow mutation lands in the immutable history log with sequential event IDs, timestamps, and versions (`schema/cassandra/temporal/schema.cql:62-67`), and mutating APIs carry a caller `Identity` string that is threaded into event attributes (`service/history/workflow/mutable_state_impl.go:4604-4610`, `service/history/workflow/workflow_task_state_machine.go:796-840`). System-initiated changes are tagged with reserved identities (`service/history/consts/const.go:13-14`). **Caveat:** by default `Identity` is a client-*asserted*, unauthenticated string — merely length-checked (`service/frontend/workflow_handler.go:1664-1666`). Cryptographic binding of caller to action requires operators to deploy the JWT claim mapper over mTLS/JWT (`common/authorization/default_jwt_claim_mapper.go:97-101`), because the default claim mapper grants admin to everyone (`common/authorization/claim_mapper.go:52-54`). For bulk operations specifically, attribution is stronger: operator identity becomes a searchable index entry (`service/frontend/workflow_handler.go:5934-5940`).

### 2. What policy allowed it?

**Not answerable from server records.** The authorizer contract returns `{Decision, Reason, Principal}` (`common/authorization/authorizer.go:40-46`), but allows are never logged and denies only bump a metric counter before returning a generic error (`common/authorization/interceptor.go:322-328`). There is no policy-decision log, no decision ID, and no record linking a granted action to the claims that permitted it. An auditor who wants this must supply their own `Authorizer` implementation (`Authorizer` interface at `common/authorization/authorizer.go:54-56`) and do their own recording — nothing in-tree persists decisions. The *rules themselves* are inspectable: the default authorizer's role matrix is documented in code (`common/authorization/default_authorizer.go:25-34`), and per-API scope/access metadata is centralized (`common/api` method metadata consumed via `common/authorization/default_authorizer.go:45`).

### 3. Was a human involved?

**No approval machinery exists in the OSS server.** A repo-wide search for approval concepts found no implementation (only incidental matches such as a comment in `service/frontend/admin_handler.go:892,1002`). Destructive operations proceed immediately once authorized: e.g., `TerminateWorkflowExecution` executes without confirmation or second-party sign-off, and even batch operations over thousands of workflows start on a single authorized request (`service/frontend/workflow_handler.go:5942-5969`). The closest proxies to human accountability are the required `Reason` memo and `BatcherUser` attribution on batch operations (`service/frontend/workflow_handler.go:5917-5925`) — a record of *intent*, not *consent*. (Temporal Cloud's externally-managed audit-log feature is outside this source.)

### 4. Can auditors reconstruct the decision?

**Partially.** For *actions*: yes, comprehensively — the history log plus visibility records allow replaying exactly what happened to any workflow, including which identity string triggered each transition, and archived history survives retention deletion (`service/frontend/workflow_handler.go:991-997`, `service/history/archival/archiver.go:30-60`). For *policy decisions*: no — see answer 2; the decision moment leaves only a metric blip (`common/metrics/metric_defs.go:694`). For *namespace administration*: nearly — opt-in wide events capture full before/after state diffs including owner, active cluster, archival URIs, and bad-binary removals (`common/wideevents/namespace_lifecycle_events.go:316-379`), but they omit the acting principal and are disabled by default (`temporal/fx.go:222-224`, `common/dynamicconfig/constants.go:239-242`).

## Architectural Decisions

1. **Event sourcing as the primary audit substrate.** Rather than a parallel audit log, Temporal makes the workflow history itself the durable record — append-only writes through `AppendHistoryNodes`, chained transactions (`prev_txn_id`), and fork-on-reset branching (`schema/cassandra/temporal/schema.cql:58-70`). This guarantees action logs and system behavior can never diverge.
2. **Pluggable authorization at a single choke point.** All external traffic funnels through one unary/stream interceptor pair with `ClaimMapper`/`Authorizer`/`JWTAudienceMapper` extension points (`common/authorization/interceptor.go:83-127`, wired at `service/frontend/fx.go:191-216,286-326`), keeping enforcement uniform including streaming RPCs.
3. **Server-computed principal vs client assertion.** The interceptor computes a `Principal` from verified TLS certs or signed JWTs and strips inbound principal headers to prevent spoofing (`common/authorization/interceptor.go:156-158`; `common/headers/headers.go:125-135`) — but this principal feeds authorization, not the persisted `Identity` fields, which remain caller-supplied.
4. **Attribution-by-search-attribute for bulk operations.** Instead of a separate ops-audit table, batch operations encode operator/reason into indexed search attributes and memos on a dedicated tracking workflow (`service/frontend/workflow_handler.go:5917-5953`), reusing the visibility store as the query engine.
5. **Opt-in structured telemetry, not mandated sinks.** New-style "wide events" emit through OpenTelemetry log records with stable event names (`common/wideevents/events.go:14-52`), leaving export/batching to the operator's OTEL pipeline — flexible, but zero coverage until configured.
6. **Integrity via sampled checksums, not enforced verification.** Mutable-state CRC32 checks are probabilistic (dynamic config) and mismatches are logged rather than fatal (`service/history/workflow/mutable_state_impl.go:557-571`) — a durability guard, explicitly not a tamper-control.

## Notable Patterns

- **Reserved system identities** (`history-service`, `history-resetter`) let auditors distinguish machine-initiated from human-initiated transitions (`service/history/consts/const.go:13-14`, used at `service/history/ndc/workflow_resetter.go:1081`).
- **Defensive identity substitution**: when a terminate request lacks identity, the frontend stamps `"frontend-service"` rather than leaving it empty (`service/frontend/workflow_handler.go:113`, `2489-2493`) — every event has *some* attribution.
- **Cross-namespace re-authorization** with per-(namespace,API) de-duplication for commands embedded in workflow-task responses (`common/authorization/interceptor.go:347-417`) — closing a classic confused-deputy hole, with tests (`common/authorization/interceptor_test.go:340-540`).
- **Error-detail hygiene**: authorizer errors are masked behind a generic message unless `ExposeAuthorizerErrors` is enabled (`common/authorization/interceptor.go:314-321`; `common/dynamicconfig/constants.go:934-938`).
- **Snapshot-diff event design**: lifecycle events carry `Requested`, `Before`, and `After` states so tampering between request and persist is visible (`common/wideevents/namespace_lifecycle_events.go:305-346`).

## Tradeoffs

- **History-as-audit-log vs security-event semantics.** Workflow history captures workflow-domain truth exhaustively, but non-workflow actions (namespace CRUD, cluster ops, failed authorization attempts) live outside it, in weaker, optional channels.
- **Self-asserted identity vs friction.** Requiring authenticated principals for every SDK would break broad compatibility; Temporal instead keeps `Identity` free-form and lets operators upgrade trust via JWT/mTLS — trading default audit strength for adoption ease.
- **Metrics-only denial records vs hot-path performance.** Persisting every decision would add write amplification to a latency-critical path; Temporal chose counters (`common/authorization/interceptor.go:309-323`), sacrificing post-hoc decision reconstruction.
- **OTEL wide events vs guaranteed capture.** Decoupling emission from storage (`common/wideevents/events.go:40-52`) avoids coupling to any backend, but means audit coverage depends entirely on deployment configuration, and the noop defaults make silent absence likely.
- **Debug-level access logs vs log volume/cost.** Logging method+cert thumbprint on every call at info level would be expensive at Temporal scale, so it is Debug-gated (`common/rpc/interceptor/namespace_logger.go:53`).

## Failure Modes / Edge Cases

- **Unconfigured deployments have no trustworthy "who".** With the default noop claim mapper, all callers present as admin (`common/authorization/claim_mapper.go:52-54`); recorded identities are arbitrary client strings. An attacker with network reachability can act under any name.
- **Denial storms are invisible forensically.** Repeated denied probes leave only aggregate counters — no source IP, claims, or API breakdown survives (`common/authorization/interceptor.go:322-328`).
- **Streaming auth can be turned off wholesale.** `DisableStreamingAuthorizer` bypasses authz on stream endpoints (`common/authorization/interceptor.go:195`; `common/dynamicconfig/constants.go:149-153`) — an availability escape hatch that also removes the audit checkpoint.
- **Checksum mismatch is swallowed.** Verification failures increment a metric and log, but the load proceeds ("we ignore checksum verification errors for now", `service/history/workflow/mutable_state_impl.go:562-569`) — corrupted state would not block operation, and there is no quarantine path.
- **Wide-event loss is undetectable.** Because the default provider discards events (`temporal/fx.go:222-224`) and OTEL export is asynchronous, a misconfigured pipeline yields silently empty namespace-mutation history.
- **Archival gaps**: archival is per-namespace opt-in with URI-based providers; runs closed before archival enablement or with misconfigured URIs simply expire with retention, leaving auditors with nothing (`config/development-jwt.yaml` namespaceDefaults section; `service/history/archival/archiver.go`).
- **Read access is unaudited.** `GetWorkflowExecutionHistory` and visibility queries perform no caller-attributed record at default log levels (`service/frontend/workflow_handler.go:960-1023`) — data exfiltration via reads is untraceable.

## Future Considerations

1. **Persist policy-decision records.** Extend `Result` (`common/authorization/authorizer.go:40-46`) with a decision ID and add an opt-in decision sink (log-based at minimum) capturing claims hash, API, namespace, and outcome — directly answering dimension questions 2 and 4.
2. **Add actor identity to lifecycle wide events.** Thread the computed `Principal` from the interceptor into `NamespaceUpdatedInput`/`NamespaceRegisteredInput` (`service/frontend/namespace_lifecycle_events.go:196-223`) so namespace mutations become attributable.
3. **Promote verified principal into recorded identity.** When `EnablePrincipalPropagation` is on and a verified principal exists, prefer it (or record it alongside) client-supplied `Identity` in history events, closing the self-assertion gap.
4. **Ship a default-on minimal audit event set.** Emitting authz denials and namespace mutations at info severity by default (even before full wide-event plumbing) would materially improve out-of-box defensibility.
5. **Escalate checksum mismatches.** Add a configurable failure/quarantine mode instead of unconditional continuation (`service/history/workflow/mutable_state_impl.go:562-569`).
6. **Optional read auditing.** A dynamic-config-gated access log for history/visibility reads (already half-built at Debug level, `common/rpc/interceptor/namespace_logger.go:40-59`) would cover the exfiltration blind spot.

## Questions / Gaps

- **No approval/second-person controls:** searched `approval|approved` across all Go sources; no evidence found beyond unrelated matches (`service/frontend/admin_handler.go:892,1002`). Human-in-the-loop gating is absent from the OSS server.
- **No decision IDs anywhere:** searched `audit` across Go/proto/yaml — the only hits are two incidental comments in `service/frontend/admin_handler.go:892,1002`. There is no audit subsystem in-tree; Temporal Cloud's audit log is external to this repository (no evidence found here).
- **External API module boundary:** `HistoryEvent` attribute definitions (e.g., `identity` fields) live in the external `go.temporal.io/api` dependency, not vendored in this source tree. Attribution claims above are grounded in server-side usage sites cited (identity threading, length validation, builder calls); exact proto field docs were not inspectable under the single-source isolation rule.
- **Retention enforcement details:** how visibility rows and executions age out per-namespace retention was not traced end-to-end; only retention-as-namespace-config surfaced via lifecycle event snapshots (`common/wideevents/namespace_lifecycle_events.go:291`).
- **Dynamic config change auditing:** whether modifications to dynamic config keys (e.g., disabling the streaming authorizer) are themselves logged could not be determined; no evidence found in the config client paths examined (`common/dynamicconfig/static_client.go` et al.).

---

Generated by `dimensions/08.04-security-auditability` against `temporal`.

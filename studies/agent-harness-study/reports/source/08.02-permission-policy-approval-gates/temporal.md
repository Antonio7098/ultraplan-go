# Source Analysis: temporal

## Dimension 08.02: Permission Policy and Approval Gates

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (Temporal server: gRPC frontend/history/matching/worker services, persistence, JWT/mTLS auth) |
| Analyzed | 2026-08-26 |

## Summary

Temporal gates sensitive operations with a **stateless, per-request authorization layer** plugged into the frontend gRPC interceptor chain. There is no first-class "approval" object anywhere in the server — a `grep` for `approval` across all `.go`, `.md`, and `.yaml` files in the source returns zero hits (search boundary: entire `studies/agent-harness-study/sources/temporal` tree). Instead, the model is:

1. **Permission schema**: a bitmask role system (`RoleWorker|RoleReader|RoleWriter|RoleAdmin`) held per subject at *system* scope and per-*namespace* in `Claims` (`common/authorization/roles.go:8-36`). Every public API method carries static metadata declaring its required scope (cluster vs namespace) and access level (read/write/admin) (`common/api/metadata.go:18-46`, map entries at `common/api/metadata.go:70-208`).
2. **Policy evaluation**: a pluggable `Authorizer` interface receives `Claims` + `CallTarget` (API name, namespace, request) and returns `DecisionAllow`/`DecisionDeny` with an optional reason and principal (`common/authorization/authorizer.go:24-56`). The shipped `defaultAuthorizer` implements the documented reader/writer/admin × namespace/system matrix (`common/authorization/default_authorizer.go:25-65`).
3. **Enforcement points**: a unary gRPC interceptor on the frontend (`common/authorization/interceptor.go:129-185`, wired at `service/frontend/fx.go:298`), a stream interceptor (`common/authorization/interceptor.go:188-238`), and manual re-entry for the Nexus HTTP surface (`service/frontend/nexus_handler.go:177-195`, `service/frontend/nexus_completion_http_handler.go:585-617`, `service/frontend/nexus_operation_http_handler.go:313-337`).
4. **Identity**: claims are derived per request from a bearer JWT (signature via JWKS keys refreshed on an interval, expiry and audience validated during parse) or from the mTLS peer certificate subject (`common/authorization/default_jwt_claim_mapper.go:76-110`, `common/authorization/interceptor.go:251-285`).

Because decisions are recomputed from credentials on every call, there is nothing to persist or revoke at the application layer: "revocation" is delegated to token expiry (`claims.Valid()` at `common/authorization/default_jwt_claim_mapper.go:194`), signing-key rotation (`common/authorization/default_token_key_provider.go:113-136`), and certificate rotation. Human-in-the-loop approvals exist only as user-level workflow patterns (signals/updates), not as server-enforced gates.

**Important default posture**: when unconfigured, both authorizer and claim mapper fall back to no-op implementations that grant `System: RoleAdmin` to everyone (`common/authorization/authorizer.go:64-73`, `common/authorization/claim_mapper.go:52-54`, `common/authorization/noop_authorizer.go:12-14`). The gate is fail-open unless an operator opts in.

## Rating

**7 / 10** — Clear model with explicit interfaces, per-API permission metadata, scoped roles, extensive unit tests of the decision matrix and interceptor behavior, and operational safeguards (metrics, generic error masking, principal-spoofing defense). It stops short of 9–10 because the safe behavior is opt-in rather than default (noop authorizer grants admin), the `Access` metadata is explicitly advisory (`common/api/metadata.go:10-13`), internal services (history/matching) carry no authorization at all (network-trust model), there is no persisted/expiring approval artifact to study, and granularity is capped at API×namespace×role — no per-workflow-type or per-endpoint policy in the default implementation.

## Evidence Collected

Every entry cites workspace-relative paths under `studies/agent-harness-study/sources/temporal/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Decision enum | `DecisionDeny` / `DecisionAllow`; deny-by-default ordering (`iota + 1` so zero-value is neither) | `common/authorization/authorizer.go:14-19` |
| Policy input schema | `CallTarget{APIName, Namespace, NexusEndpointName, Request}` — comment notes it "can be extended" with WorkflowType/TaskQueue resources | `common/authorization/authorizer.go:21-34` |
| Policy result | `Result{Decision, Reason, Principal}` — server-computed identity returned on allow | `common/authorization/authorizer.go:39-50` |
| Authorizer SPI | Single-method interface `Authorize(ctx, claims, target)`; selected by config string (`""`→noop, `"default"`→default, else boot error) | `common/authorization/authorizer.go:52-73` |
| Role bitmask | `RoleWorker=1<<0, RoleReader, RoleWriter, RoleAdmin`; validity check rejects unknown bits | `common/authorization/roles.go:8-21` |
| Claims schema | `Subject`, system-wide `System Role`, `Namespaces map[string]Role`, free-form `Extensions`, `AuthType` | `common/authorization/roles.go:23-36` |
| Per-API permission metadata | Every WorkflowService method tagged `{Scope, Access, Polling}`; e.g. `RegisterNamespace`/`UpdateNamespace` = namespace-admin, `TerminateWorkflowExecution` = namespace-write | `common/api/metadata.go:70-194` (71, 74, 114) |
| Operator APIs locked to admin | `DeleteNamespace`, search-attribute mutations, cluster ops, Nexus endpoint CRUD = admin; cluster-scoped where appropriate | `common/api/metadata.go:195-208` (199, 200-207) |
| Internal AdminService default | Any `/temporal.server.api.adminservice.v1.AdminService/` call defaults to `{ScopeCluster, AccessAdmin}` | `common/api/metadata.go:228-229` |
| Default policy rules | Health checks always allowed; nil claims denied; cluster-scoped methods check `claims.System`; namespace-scoped check `System \| Namespaces[ns]`; role must be >= required | `common/authorization/default_authorizer.go:35-65` (38-40, 41-43, 48-57, 59-64) |
| Access → role mapping | read-only→Reader, write→Writer, else Admin | `common/authorization/default_authorizer.go:67-77` |
| Interceptor enforcement (unary) | Claims mapped from TLS+header, context enhanced, then `Authorize`; deny aborts before handler | `common/authorization/interceptor.go:129-185` (144-183) |
| Interceptor chain position | `authInterceptor.Intercept` placed after namespace validator, before handover/redirection | `service/frontend/fx.go:286-299` (298) |
| Interceptor construction | Built from config header names + four dynamicconfig knobs incl. `ExposeAuthorizerErrors`, `EnableCrossNamespaceCommands`, `EnablePrincipalPropagation`, `DisableStreamingAuthorizer` | `service/frontend/fx.go:191-216` |
| Stream enforcement | `InterceptStream` authorizes handshake without request body/namespace; can be disabled via dynamicconfig | `common/authorization/interceptor.go:187-238` (195-235) |
| Cross-namespace command gating | Worker-supplied commands (`SignalExternalWorkflow`, `StartChildWorkflowExecution`, `RequestCancelExternalWorkflow`) re-authorized against each target namespace+API; deduped per ns:api pair | `common/authorization/interceptor.go:347-417` (357-360, 365-368, 406-414) |
| Principal spoofing defense | Inbound `temporal-principal-*` headers always stripped, even when authorizer disabled | `common/authorization/interceptor.go:156-158`; `common/headers/headers.go:125-135` |
| Principal propagation | Server-computed principal re-injected as headers only when `frontend.enablePrincipalPropagation` enabled for the namespace | `common/authorization/interceptor.go:175-177`; `common/dynamicconfig/constants.go:940-945` |
| Error masking + metrics | Denials emit `ServiceErrUnauthorizedCounter`; authorizer errors masked behind generic `PermissionDenied` unless `frontend.exposeAuthorizerErrors` | `common/authorization/interceptor.go:304-331` (317-328); `common/dynamicconfig/constants.go:934-939` |
| Identity source: JWT | Bearer token parsed; `sub` + configurable permissions claim (default `permissions`) mapped to roles; custom regex with named groups supported | `common/authorization/default_jwt_claim_mapper.go:76-147` |
| Expiry/audience validation | `claims.Valid()` enforces `exp/nbf`; audience verified against configured value | `common/authorization/default_jwt_claim_mapper.go:185-201` (194, 197-199) |
| Key rotation (revocation path) | JWKS fetched from http/file URIs, swapped atomically under lock on `RefreshInterval` ticker; HMAC rejected | `common/authorization/default_token_key_provider.go:43-57, 113-136, 189-191` |
| mTLS identity | Peer client cert subject extracted from verified chain [0] as `TLSSubject` for claim mapping | `common/authorization/interceptor.go:69-81, 251-285` |
| Auth config schema | `jwtKeyProvider`, `permissionsClaimName`, `permissionsRegex`, `authorizer`, `claimMapper`, `authHeaderName`, `audience`, `remoteClusterAuth.require` | `common/config/config.go:641-674` |
| Noop claim mapper danger | Unconfigured claim mapper returns `&Claims{System: RoleAdmin}` for everyone | `common/authorization/claim_mapper.go:42-59` |
| Internal-frontend trust | Internal frontend decorates claim mapper to always return system admin with principal type `temporal`/name `internal` | `temporal/fx.go:599-608`; `common/authorization/claim_mapper.go:61-78`; `common/authorization/principal.go:7-8` |
| System-worker detection downstream | Matching service trusts principal type `temporal`, falling back to internal task-queue prefix when principal absent | `service/matching/workers/registry_impl.go:523-532` |
| Nexus gRPC-path gating | Nexus operations authorize with `CallTarget{APIName, Namespace, NexusEndpointName}` (endpoint name available for narrow policies), then namespace state validation | `service/frontend/nexus_handler.go:172-201` |
| Nexus HTTP completion gating | HTTP handler rebuilds `AuthInfo` from request TLS+headers, maps claims, authorizes, adapts errors | `service/frontend/nexus_completion_http_handler.go:576-617` |
| Nexus operation HTTP gating | Same GetAuthInfo/GetClaims pattern for operation route handlers (audience getter TODO noted) | `service/frontend/nexus_operation_http_handler.go:313-337` (322-324) |
| Audit trail of sensitive ops | Remote-cluster lifecycle events record `auth_subject`/`auth_type` from mapped claims plus caller info | `service/frontend/remote_cluster_lifecycle_events.go:177-190` |
| Outbound cross-cluster auth | Optional per-RPC bearer token on remote-frontend dials; fails closed (`Unauthenticated`) if require=true and token empty; boot validation rejects require-without-provider | `common/rpc/rpc.go:279-299`; `temporal/fx.go:299-300` |
| Dynamic config kill-switches | `system.enableCrossNamespaceCommands` (default false), `system.disableStreamingAuthorizer` (default false) | `common/dynamicconfig/constants.go:144-153` |
| Policy tests: decision matrix | ~25 table cases covering system/namespace × reader/writer/admin × allow/deny, case-sensitive namespace mismatch, health-check bypass | `common/authorization/default_authorizer_test.go:101-156` |
| Policy tests: config selection | noop/default/unknown authorizer selection from YAML string | `common/authorization/default_authorizer_test.go:158-181` |
| Policy tests: interceptor | Allow/deny/error paths, unknown-namespace metric tagging, error exposure toggle, alternate headers, cross-ns authorized/unauthorized/disabled/dedup, principal propagation enabled/disabled/spoofed-strip, stream auth (authorized, unauthorized, disabled, invalid token, audience, mTLS-only) | `common/authorization/interceptor_test.go:105-148, 160-184, 214-268, 340-495, 497-611, 663-881` |
| Policy tests: role validity | Bitmask combination validation incl. invalid values 32/33/64/125 | `common/authorization/roles_test.go:7-29` |
| Policy tests: claim mapping | RSA/ECDSA tokens, missing kid/alg/sub rejection, permission parsing, namespace case sensitivity, audience match | `common/authorization/default_jwt_claim_mapper_test.go:77-161, 178-197` |
| Integration wiring | Test clusters inject custom `WithAuthorizer`/`WithClaimMapper`; XDC suite captures outbound tokens per cluster/API | `tests/testcore/onebox.go:146-148`; `tests/xdc/remote_cluster_auth_test.go:112-179` |
| Example deployment config | `authorizer: "default"` + `claimMapper: "default"` + file-based JWKS | `config/development-jwt.yaml:66-71` |

## Answers to Dimension Questions

1. **Which actions require approval?**
   When an authorizer is configured, every non-health-check frontend API call requires an explicit `DecisionAllow` — health/gRPC health and `GetSystemInfo` bypass (`common/authorization/frontend_api.go:8-11`, `default_authorizer.go:38-40`). Sensitivity is tiered by static metadata: admin-only operations include `RegisterNamespace`, `UpdateNamespace`, `DeprecateNamespace` (`common/api/metadata.go:71,74,75`), `DeleteNamespace` and search-attribute mutation (`common/api/metadata.go:196-199`), and all cluster-scoped operator/Nexus-endpoint CRUD (`common/api/metadata.go:200-207`); destructive workflow operations like terminate/delete/reset are Writer-tier (`common/api/metadata.go:110-115`); reads are Reader-tier. There is no human approval step; "approval" is the per-request Allow decision.
2. **Who can approve?**
   The configured `Authorizer` plugin decides, using `Claims` produced by the configured `ClaimMapper` from JWT or mTLS identity (`common/authorization/authorizer.go:52-56`, `claim_mapper.go:27-31`). Under the default policy: system Admin approves everything; namespace Admin approves their namespaces' admin APIs; Writers cover non-admin writes; Readers cover non-admin reads (`common/authorization/default_authorizer.go:25-34`). Operators may ship arbitrary plugins via `WithAuthorizer` (`temporal/server_option.go:98-99`) or YAML selection (`common/authorization/authorizer.go:64-73`).
3. **Are approvals scoped and expiring?**
   Scoped yes: per-API method (metadata map), per-namespace (`Claims.Namespaces`), per-scope (cluster vs namespace), plus target-namespace re-checks for cross-namespace worker commands (`common/authorization/interceptor.go:351-417`) and an unused-but-plumbed `NexusEndpointName` resource field (`common/authorization/authorizer.go:30-31`, passed at `service/frontend/nexus_handler.go:180`). Narrower-than-namespace grants are possible only in custom authorizers that inspect `CallTarget.Request`. Expiring: only in the credential sense — JWT `exp` enforced (`default_jwt_claim_mapper.go:194`), audience bound (`:197-199`), signing keys rotated on interval (`default_token_key_provider.go:52-56`); no approval is cached or persisted server-side, so revocation takes effect at next request once tokens expire or keys rotate. There is no TTL'd, stored "grant" artifact to expire early.
4. **Can policy override model intent?**
   Temporal has no LLM/model intent; the nearest analog is *worker/workflow-initiated intent*. Yes, policy overrides it: a worker holding write access in namespace A cannot exfiltrate actions into namespace B via workflow commands — each cross-namespace command is independently authorized against the target namespace and corresponding API name (`common/authorization/interceptor.go:373-414`), tested at `common/authorization/interceptor_test.go:399-431`. Spoofed caller-principal headers are stripped regardless of configuration (`common/authorization/interceptor.go:156-158`, test at `interceptor_test.go:591-611`), preventing forged identity downstream (e.g., matching's `isSystemWorker` trust check, `service/matching/workers/registry_impl.go:527-532`).

> **Focal question — Can approval be granted narrowly rather than globally?**
> Yes, to a point. Grants are composable bitmasks per (subject × namespace) plus a system scope (`roles.go:8-14, 29-31`), evaluated per-API against advisory access tags, so one caller can be Reader on ns-a and Admin on ns-b while another is Worker-only. Cross-namespace commands add per-(target-namespace × API) narrowing. However the default authorizer cannot express narrower resources (specific workflow types, task queues, schedules, or individual Nexus endpoints): `CallTarget` documents this as future extension space (`authorizer.go:22-23`), and `NexusEndpointName` exists but the stock `defaultAuthorizer` ignores it. Anything narrower requires a custom `Authorizer`.

## Architectural Decisions

- **Authorization as a plugin seam, not a built-in RBAC engine**: the server defines `Authorizer`/`ClaimMapper` interfaces and ships minimal implementations; richer policy (external IdP-driven) is expected out-of-band. Selection is fail-closed on unknown strings at startup (`authorizer.go:64-73`, `claim_mapper.go:80-89`) but fail-open when simply unset (noop).
- **Static per-method permission metadata colocated in code**: a single hand-maintained map classifies every API (`common/api/metadata.go:70-216`) rather than annotations scattered per handler; the doc explicitly marks `Access` as advisory so plugins may ignore it (`metadata.go:10-13`).
- **Stateless per-request decisions**: nothing about a decision is written to persistence; approvals cannot be pre-granted or queued, trading auditability/persistence for simplicity and instant consistency with rotated credentials.
- **Identity pushed to standards (JWT/JWKS/mTLS)**: Temporal validates but never issues end-user credentials; permission strings live in a token claim parsed by configurable regex (`config/config.go:644-647`, `default_jwt_claim_mapper.go:44-63`).
- **Defense-in-depth at boundaries**: principal headers stripped inbound (`headers.go:125-135`), outbound cross-cluster calls optionally forced to carry tokens with fail-closed semantics (`rpc/rpc.go:292-294`), and error details masked by default (`dynamicconfig/constants.go:934-939`).
- **Internal services exempt from authorization**: history/matching/worker fx wiring contains no authorization interceptor (verified: `grep authorization` over `service/{history,matching,worker}/fx.go` returns nothing); they rely on internode TLS/network isolation and the dedicated internal-frontend with auto-admin claim mapper (`temporal/fx.go:599-608`, rationale in `config/config.go:563-579`).

## Notable Patterns

- **Re-authorize embedded intents**: requests carrying sub-intents (worker commands targeting other namespaces) are decomposed into synthetic `CallTarget`s and authorized individually, with dedup keyed `namespace:api` (`interceptor.go:370-414`).
- **Server-computed principal, opt-in propagation**: the Allow result carries a principal constructed server-side (`default_authorizer.go:60-62`); it only flows downstream as headers when explicitly enabled, closing spoofing paths (`interceptor.go:171-177`).
- **Observable denials**: latency histogram + unauthorized/failed counters tagged by real-namespace-or-unknown tag to bound cardinality (`interceptor.go:309-345`), plus lifecycle audit events embedding `auth_subject` for sensitive cluster mutations (`remote_cluster_lifecycle_events.go:184-190`).
- **Table-driven policy tests**: the full role×scope×access matrix lives in one test table, making intended policy legible (`default_authorizer_test.go:101-156`).

## Tradeoffs

- **Fail-open default vs ease of adoption**: empty config yields admin-for-all (`claim_mapper.go:52-54`, `noop_authorizer.go:12-14`). Safe posture requires deliberate opt-in (`config/development-jwt.yaml:66-71`).
- **Advisory access tags vs guaranteed least privilege**: a custom authorizer ignoring `Access` can silently downgrade guarantees; the type system does not enforce consumption (`metadata.go:10-13`).
- **Namespace×role granularity vs operational demand**: fine-grained ACLs (per endpoint/workflow type) need custom plugins; the hook fields exist but stock logic ignores them (`authorizer.go:30-33`).
- **Network-trust internals vs zero-trust ambitions**: skipping authz inside the cluster simplifies design but means a compromised pod bypasses the entire permission layer; mitigations are TLS + optional internal-frontend separation (`config/config.go:563-579`).
- **Dynamic-config kill-switches vs accidental exposure**: `system.disableStreamingAuthorizer` and `frontend.exposeAuthorizerErrors` are convenient but are global booleans that weaken gates if flipped casually (`constants.go:149-153, 934-939`).

## Failure Modes / Edge Cases

- **Unknown API names default to deny** in the default authorizer (`ScopeUnknown` → deny, `default_authorizer.go:55-57`; unmapped AdminService prefix still gets cluster-admin treatment, `metadata.go:228-229`), so newly added methods fail closed until classified.
- **Namespace case sensitivity**: `BAR` ≠ `bar` denies by design (`default_authorizer_test.go:128-129`); combined with JWT permission strings being case-sensitive (`default_jwt_claim_mapper_test.go:142-161`), mis-cased IdP configs silently yield `RoleUndefined`.
- **Nil claims on authenticated-looking traffic**: missing/invalid tokens produce nil claims → deny (`default_authorizer.go:41-43`), while claim-mapper failures log internally and return a generic error to avoid disclosure (`interceptor.go:146-152`).
- **Key-refresh gap**: JWKS refresh is interval-based; a key revoked upstream remains trusted until next tick, and initial fetch failure only logs (`default_token_key_provider.go:46-51`) — server starts even if keys never loaded.
- **Stream blind spot**: stream authorization runs without request body or namespace (`interceptor.go:224-230`), so namespace-scoped policies can't differentiate streams except via the global disable switch.
- **Spoofed principal hygiene depends on strip-always ordering**: stripping happens in both interceptors even when auth is off (`interceptor.go:156-158, 219-221`), protecting downstream consumers like `isSystemWorker` (`registry_impl.go:527-532`); any new entrypoint (e.g., raw HTTP) must replicate this manually — the Nexus completion path notably does not show a strip call (`nexus_completion_http_handler.go:585-598`).
- **No approval replay protection**: since decisions aren't recorded, an allowed long-lived token keeps working until expiry; there's no server-side session kill list.

## Future Considerations

- Extend `CallTarget` with workflow-type/task-queue resources (the code itself flags this: `authorizer.go:22-23`) and wire `NexusEndpointName` into the default authorizer for per-endpoint policy.
- Add a first-class persisted grant/approval abstraction (e.g., audited, expiring elevation for batch/destructive ops) instead of relying solely on credential lifetime.
- Ship integration-test coverage of `defaultAuthorizer` through the real frontend stack (current `tests/` usage exercises custom authorizers, `tests/testcore/onebox.go:146-148`).
- Consider fail-closed startup when `authorizer != noop` but JWKS sources failed to load, and metrics/alerts on key-refresh failure (`default_token_key_provider.go:46-51`).
- Document and lint the "always strip principal headers" invariant for non-gRPC entrypoints.

## Questions / Gaps

- **No evidence found** for any persisted or human-approved authorization artifact: searches for `approval`, `approve(d)`, `confirm` across Go/YAML/docs in the source tree returned no relevant hits; approval persistence and revocation-beyond-credential-expiry are therefore not implemented in this source (search boundary: `grep -ri approval` and related terms over `studies/agent-harness-study/sources/temporal`, all file types).
- **No evidence found** that `IsReadOnlyNamespaceAPI`/`IsReadOnlyGlobalAPI` (`common/authorization/frontend_api.go:13-23`) are called anywhere outside their definition — possibly dead code reserved for external use.
- Whether Temporal Cloud adds approval workflows cannot be assessed from this OSS source; nothing here implies or refutes it.
- The audience getter for Nexus HTTP surfaces is an acknowledged TODO (`nexus_completion_http_handler.go:586`, `nexus_operation_http_handler.go:323`), so audience-bound tokens are not fully enforced on those routes yet.

---

Generated by `Dimension 08.02: Permission Policy and Approval Gates` against `temporal`.

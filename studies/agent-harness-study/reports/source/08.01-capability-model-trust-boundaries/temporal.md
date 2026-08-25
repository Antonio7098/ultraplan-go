# Source Analysis: temporal

## Capability Model and Trust Boundaries

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (gRPC server; frontend/history/matching/worker services; protobuf APIs) |
| Analyzed | 2026-08-24 |

## Summary

Temporal is not an LLM agent harness; it is a durable workflow orchestration server. Its "capability model" is therefore an **API capability model**: the system exposes a fixed, enumerated surface of gRPC/HTTP operations (`StartWorkflowExecution`, `RespondWorkflowTaskCompleted`, admin/operator APIs, Nexus operations), and every capability is a server-side API call that external callers (SDK clients, workers) can only *request* — the server's history service is the sole authority that actually mutates workflow state. Workers never hold state authority; they receive opaque task tokens and must call back through authorized write APIs.

The model has four clearly separated layers:

1. **Transport trust** — optional TLS/mTLS per listener group (internode, frontend, remote clusters, system worker), with `requireClientAuth` upgrading to `RequireAndVerifyClientCert` (`common/rpc/encryption/local_store_tls_provider.go:341-355`).
2. **Authentication** — pluggable `ClaimMapper` converting JWT bearer tokens or mTLS subjects into `Claims` (`common/authorization/claim_mapper.go:17-31`), validated against RSA/ECDSA JWKS keys fetched from configured URIs (`common/authorization/default_token_key_provider.go:113-187`).
3. **Authorization** — pluggable `Authorizer` receiving `(Claims, CallTarget)` per request via a gRPC interceptor chain (`common/authorization/interceptor.go:129-185`); the default authorizer maps role bitmasks (Worker/Reader/Writer/Admin × System/Namespace scopes) against per-API metadata (`common/authorization/default_authorizer.go:35-65`, `common/api/metadata.go:70-216`).
4. **State authority** — namespace-scoped validation and task-token binding (`common/rpc/interceptor/namespace_validator.go:226-231`, `common/tasktoken/token.go:9-61`); cross-namespace workflow commands are re-authorized against the target namespace (`common/authorization/interceptor.go:351-417`).

The critical caveat is that **the entire authorization layer is opt-in and defaults to allow-all**: with no config, both claim mapper and authorizer are no-ops that grant `System: RoleAdmin` / unconditional allow (`common/authorization/claim_mapper.go:52-54`, `common/authorization/noop_authorizer.go:12-14`, `common/authorization/authorizer.go:64-73`). Security posture depends on operators wiring real implementations — the interfaces exist for this but ship unconfigured.

## Rating

**8/10.**

Rationale against the rubric:

- **Clear model with explicit interfaces (7–8 band)**: The capability surface is exhaustively enumerated in code as per-API metadata (`common/api/metadata.go:70-194` covers ~120 WorkflowService methods with Scope/Access annotations). The `Authorizer`/`ClaimMapper` split (`common/authorization/authorizer.go:54-56`, `common/authorization/claim_mapper.go:29-31`) cleanly separates authentication from authorization and lets deployments substitute custom policy. Role algebra is a small, well-defined bitmask (`common/authorization/roles.go:8-21`). Authority is consistently enforced server-side in interceptors positioned explicitly in the chain (`service/frontend/fx.go:286-326`) — callers can request power (start workflows, signal, cancel) without possessing it (all mutations flow through history-service ownership of state).
- **Tests exist and are meaningful**: role matrix tests (`common/authorization/default_authorizer_test.go:15-74`), cross-namespace command authz tests including unauthorized cases (`common/authorization/interceptor_test.go:340-432`), principal-header spoofing tests (`common/authorization/interceptor_test.go:591`), stream authz tests (`common/authorization/interceptor_test.go:663-777`).
- **Not 9–10 because**: default-deny is absent (no-op allow-all by design for dev-friendliness); task tokens are unsigned protobuf blobs (`common/tasktoken/serializer.go:15-26`) whose integrity relies on transport security rather than cryptographic binding; Nexus callback tokens are explicitly unencrypted with "encryption support will come later" (`common/nexus/callback_token.go:23-33`); and there is no built-in egress/network policy for Nexus outbound calls — endpoint targets are operator-registered URLs trusted wholesale.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Capability enumeration | Every frontend API annotated with Scope (cluster vs namespace) and Access (read/write/admin); ~120 methods listed | `common/api/metadata.go:70-194` |
| Access metadata is advisory | Doc comment states field "is completely advisory. Any authorizer implementation may implement whatever logic it chooses" | `common/api/metadata.go:9-13` |
| Authorizer interface | `Authorize(ctx, caller *Claims, target *CallTarget)` plugin interface; `CallTarget` carries API name, namespace, deserialized request | `common/authorization/authorizer.go:24-34,54-56` |
| Claims structure | `Subject`, system-level `Role`, per-namespace `map[string]Role`, `AuthType` | `common/authorization/roles.go:25-36` |
| Role bitmask | `RoleWorker=1<<0, RoleReader, RoleWriter, RoleAdmin`; validity check rejects unknown bits | `common/authorization/roles.go:8-21` |
| Default authorizer policy | Health checks always allowed; nil claims denied; role ≥ required-role comparison per scope; unknown scope → deny | `common/authorization/default_authorizer.go:35-65` |
| Required-role mapping | Read→Reader, Write→Writer, else Admin | `common/authorization/default_authorizer.go:68-77` |
| JWT claim mapper | Bearer token parse, JWKS key lookup by `kid`, audience verification, permissions claim → namespace/system roles | `common/authorization/default_jwt_claim_mapper.go:76-110,149-201` |
| Key provider | RSA/ES256 only (HMAC rejected, `default_token_key_provider.go:189-191`); keys refreshed from http(s)/file URIs | `common/authorization/default_token_key_provider.go:93-95,171-187` |
| mTLS subject extraction | Client cert subject pulled from verified chain[0] as identity input to ClaimMapper | `common/authorization/interceptor.go:69-81,251-285` |
| Interceptor wiring order | Auth interceptor placed after error masking/business-ID, before handover/rate limiters; stream interceptor separate | `service/frontend/fx.go:286-326` |
| Error hygiene | Generic `PermissionDenied` returned unless `ExposeAuthorizerErrors` dynamicconfig enabled (default false) | `common/authorization/interceptor.go:314-328`, `common/dynamicconfig/constants.go:934-939` |
| Principal spoofing defense | Inbound principal headers always stripped before handler, even when authorizer disabled | `common/authorization/interceptor.go:156-158,219-221`; `common/headers/headers.go:125-135` |
| Cross-namespace commands | `SignalExternalWorkflow`/`StartChildWorkflow`/`CancelExternalWorkflow` inside `RespondWorkflowTaskCompleted` re-authorized against target namespace; gated off by default (`system.enableCrossNamespaceCommands=false`) | `common/authorization/interceptor.go:351-417`; `common/dynamicconfig/constants.go:144-148` |
| Namespace/task-token binding | Namespace extracted from task token takes priority over request; mismatch rejected when `frontend.enableTokenNamespaceEnforcement=true` (default true) | `common/rpc/interceptor/namespace_validator.go:233-257,355-364`; `common/dynamicconfig/constants.go:924-928` |
| Task token content | Opaque-to-worker protobuf: namespaceID/workflowID/runID/event IDs/vector clock/version; plain Marshal, no signature | `common/tasktoken/token.go:9-61`; `common/tasktoken/serializer.go:15-26` |
| Namespace lifecycle gating | Per-API allowed namespace states (e.g. start only in REGISTERED); handover read-blocking allowlist | `common/rpc/interceptor/namespace_validator.go:42-87,366-406` |
| mTLS config | `requireClientAuth` → `tls.RequireAndVerifyClientCert` with client CA pool; default `NoClientCert` | `common/rpc/encryption/local_store_tls_provider.go:341-364`; `common/config/config.go:192-193` |
| TLS segmentation | Separate Internode / Frontend / SystemWorker / RemoteClusters groups + per-host overrides | `common/config/config.go:144-174` |
| Internal trust boundary | `internal-frontend` listener swaps in `internalClaimMapper` granting `System: RoleAdmin` with type "temporal"/name "internal" — internode traffic authenticated by network/TLS, not per-request authz | `temporal/fx.go:599-608`; `common/authorization/claim_mapper.go:61-78`; `common/authorization/principal.go:4-9` |
| Remote cluster outbound auth | `RemoteClusterAuth.Require` fails outbound cross-cluster RPCs lacking a token | `common/config/config.go:662-666`; `common/rpc/rpc.go:100-104` |
| HTTP (REST) API parity | gRPC-gateway HTTP server reuses the same unary interceptor chain incl. auth | `service/frontend/fx.go:1066-1078`; `service/frontend/http_api_server.go:76,397-400` |
| Nexus callback token | Base64 completion proto handed to external handler; version-checked but unsigned ("encryption support will come later") | `common/nexus/callback_token.go:16-53,117-127` |
| Nexus callback validation | Completion target resolved from token; namespace existence checked; CHASM vs HSM ref validation | `common/nexus/callback_token.go:72-115`; `service/frontend/nexus_completion_http_handler.go:125-157` |
| Nexus endpoints cluster-admin | Create/Update/Delete/Get/List Nexus endpoints all `ScopeCluster, AccessAdmin` | `common/api/metadata.go:203-207` |
| Default allow-all | Empty `authorizer`/`claimMapper` config selects noop variants; noop mapper grants `System: RoleAdmin` to everybody | `common/authorization/authorizer.go:64-73`; `common/authorization/claim_mapper.go:42-59,80-89`; `common/authorization/noop_authorizer.go:12-14` |
| Ops config example | JWT auth enabled via `authorizer: "default"`, `claimMapper: "default"`, file-based JWKS URI | `config/development-jwt.yaml:66-71`; docker template wires env-based key sources at `config/docker.yaml:252-260` |
| Tests: role matrix | Claims fixtures for none/namespace reader-writer-admin/system levels vs targets | `common/authorization/default_authorizer_test.go:15-74` |
| Tests: interceptor | Authorized/unauthorized/cross-namespace/spoofed-principal/stream-authz cases | `common/authorization/interceptor_test.go:105,123,340,399,591,663-777` |

## Answers to Dimension Questions

**1. What can the agent do?**

In Temporal's vocabulary the "agents" are SDK clients and workers. Clients holding Writer on a namespace can start/signal/cancel/terminate/reset workflows, create schedules, start batch operations, respond to updates — each individually enumerated with Scope+Access in `common/api/metadata.go:70-194`. Readers get query/list/describe APIs (e.g., `QueryWorkflow` at `common/api/metadata.go:127`). Namespace admins additionally manage namespaces and search attributes (`RegisterNamespace`/`UpdateNamespace` at `common/api/metadata.go:71,74`; OperatorService admin set at `common/api/metadata.go:195-208`). Workers can poll task queues and complete/fail/heartbeat tasks — all `ScopeNamespace, AccessWrite` (`common/api/metadata.go:79-90`). Crucially, workers executing user code (activities/workflows) have **zero filesystem/host capabilities granted by Temporal itself** — Temporal grants no compute sandbox; worker hosts run arbitrary user code with whatever OS privileges the deployer gave them. The server-side capability model deliberately stops at the API boundary.

**2. What can the model only request but not directly do?**

Everything stateful. A worker receiving a workflow task cannot mutate workflow state directly: it returns *commands* (schedule activity, start child workflow, signal external workflow) embedded in `RespondWorkflowTaskCompletedRequest`, and history service validates and applies them transactionally or rejects them via task failures/replay. Cross-namespace commands within those responses are separately re-authorized against the target namespace before acceptance (`common/authorization/interceptor.go:362-415`). Similarly, external Nexus handlers cannot complete operations arbitrarily: they present the callback token issued at dispatch time, and the server resolves the target namespace/workflow/run from the token rather than trusting the caller's assertion (`service/frontend/nexus_completion_http_handler.go:127-157`). This is the strongest realization of "request power without possessing power" in the codebase.

**3. Where is authority enforced?**

Three chokepoints, in order: (a) the gRPC interceptor chain on the frontend — auth interceptor at `service/frontend/fx.go:298`, followed by namespace state validation at `service/frontend/fx.go:306`; (b) the default authorizer's role comparison `hasRole >= getRequiredRole(metadata.Access)` at `common/authorization/default_authorizer.go:59`, driven by the advisory per-API metadata table; (c) history service as the single writer of execution state (task-token-bound completions validated for namespace match at `common/rpc/interceptor/namespace_validator.go:233-257`). For internal service-to-service traffic, authority shifts to the network boundary: the internal-frontend grants automatic admin claims (`temporal/fx.go:599-608`), so internode mTLS (`requireClientAuth`) plus network isolation *is* the enforcement mechanism — per-request authz is not applied internally.

**4. Are dangerous capabilities isolated?**

Partially, by construction rather than by sandboxing:
- Admin-only cluster-wide operations (delete namespace, nexus endpoint management, search attributes) require `RoleAdmin` at cluster scope (`common/api/metadata.go:196-207`).
- Cross-namespace lateral movement is feature-flagged off by default (`system.enableCrossNamespaceCommands=false`, `common/dynamicconfig/constants.go:144-148`) and re-authorized per target namespace when enabled.
- Principal identity cannot be forged via headers: inbound `principal-type`/`principal-name` headers are unconditionally stripped (`common/authorization/interceptor.go:156-158`; test at `common/authorization/interceptor_test.go:591`).
- However, "dangerous" user-code execution (activities touching disk/network/secrets) is entirely outside the server's control — isolation is delegated to worker deployment topology. There is no resource-level sandbox, secrets broker, or egress proxy in this repository. Payload encryption is expected client-side via DataConverter codecs; the server stores payloads opaquely.

## Architectural Decisions

1. **Enumerated capability table instead of ad-hoc checks.** A single map annotates every API with scope/access/polling metadata (`common/api/metadata.go:70-216`), making the capability surface auditable in one place and consumable by any authorizer implementation. The comment that Access is "advisory" (`common/api/metadata.go:9-13`) keeps the server neutral about policy engines.
2. **AuthN/AuthZ as replaceable plugins.** `ClaimMapper` and `Authorizer` are one-method interfaces selected by string config (`common/authorization/claim_mapper.go:80-89`, `common/authorization/authorizer.go:64-73`), letting enterprises plug OIDC/OPA-style policy without forking. Temporal Cloud's own RBAC is out-of-tree; OSS ships only the default JWT mapper + role-comparison authorizer.
3. **Capability tokens for workers (task tokens).** Tasks carry self-describing tokens (`common/tasktoken/token.go:9-61`) so completions bind to a specific execution/event identity; combined with vector clocks for fencing. Trust here is transport-based, not cryptographic — an attacker with network access to the frontend and a valid Worker role could replay a captured token until fenced by clock/version mismatch.
4. **Two frontends, two trust domains.** Public `frontend` enforces per-request authz; `internal-frontend` auto-grants admin claims (`temporal/fx.go:599-608`) and relies on internode mTLS/network segregation. This trades uniform authz coverage for performance on the hot internal path.
5. **Fail-closed errors, fail-open defaults.** Authorization failures return generic `PermissionDenied` unless explicitly exposed (`common/authorization/interceptor.go:314-328`; default false at `common/dynamicconfig/constants.go:934-939`) — yet the whole subsystem defaults to allow-all when unset (`common/authorization/authorizer.go:67-68`).

## Notable Patterns

- **Interceptor-chain layering**: auth runs early, after error-masking and routing-key extraction but before namespace-state, rate-limit, and telemetry interceptors (`service/frontend/fx.go:286-315`), so unauthorized requests are cheaply rejected while still being counted per-namespace (`common/authorization/interceptor.go:309-345`, with negative-cache-safe namespace existence check at `common/authorization/interceptor.go:337-343`).
- **Defense-in-depth against header spoofing**: `StripPrincipal` runs even when the authorizer is nil (`common/authorization/interceptor.go:156-158,219-221`).
- **Deduplicated cross-namespace authz**: target namespace/API pairs are memoized per request to avoid duplicate policy evaluations (`common/authorization/interceptor.go:370-371,397-414`; test `TestMultipleCommands_AuthDeduplication` at `common/authorization/interceptor_test.go:497`).
- **Streaming parity**: long-poll APIs (task polls, updates) get a dedicated stream interceptor with its own bypass flag `system.disableStreamingAuthorizer` (`common/authorization/interceptor.go:187-238`; flag at `common/dynamicconfig/constants.go:149-153`), since namespace isn't available pre-handshake (`common/authorization/interceptor.go:224-229`).
- **Snippets as public contract**: `@@@SNIPSTART` markers around `Claims`, `Role`, `Authorizer` (`common/authorization/roles.go:5-16`, `common/authorization/authorizer.go:21-58`) indicate these types are documentation-published extension points.

## Tradeoffs

- **Dev-friendliness vs secure-by-default**: no-op auth gives instant local usability but means an internet-exposed unconfigured cluster accepts everything as system admin. The mitigation is purely documentary (deployment guides), not enforced in code.
- **Advisory access metadata vs guaranteed least privilege**: any custom authorizer may ignore `Access`; conversely the shipped default trusts the static table, so adding a new API requires remembering to annotate it — an unannotated method yields `ScopeUnknown` and the default authorizer denies it (`common/authorization/default_authorizer.go:55-56`), which is accidentally fail-closed, a happy asymmetry.
- **Transport-trust internals vs zero-trust internals**: internal admin claims remove per-request overhead but enlarge the blast radius of any network-layer compromise between services.
- **Unsigned task/callback tokens vs latency and simplicity**: no HMAC/signature step keeps the hot path fast, but means token confidentiality/integrity rides entirely on mTLS/TLS and that leaked tokens remain usable until superseded (vector-clock fencing helps for stale tokens only).
- **Feature-flagged dangerous features**: cross-namespace commands default-off reduces risk but also hides the authorization path behind a global dynamicconfig flag, so enabling it cluster-wide is a coarse lever (`NewGlobalBoolSetting`, `common/dynamicconfig/constants.go:144`).

## Failure Modes / Edge Cases

- **Unauthenticated misconfiguration**: with authorizer configured but claimMapper returning empty claims, every non-health request is denied (`claims == nil → resultDeny`, `common/authorization/default_authorizer.go:41-43`) — fail-closed, but can lock out entire fleets on JWT key rotation failure since key refresh errors are only logged (`common/authorization/default_token_key_provider.go:46-51,104-109`).
- **Namespace metric-tag cardinality guard**: auth metrics only tag namespaces that already exist, avoiding unbounded cardinality from garbage inputs (`common/authorization/interceptor.go:337-343`).
- **Stream handshake blind spot**: stream authorization sees no request body, so namespace-scoped decisions are impossible at handshake time; the namespace arrives later in messages (`common/authorization/interceptor.go:224-229`).
- **Task-token/request namespace divergence**: historically the request namespace was trusted over the token; enforcement is now on by default but can be disabled via dynamicconfig (`common/dynamicconfig/constants.go:924-928`, mismatch error at `common/rpc/interceptor/namespace_validator.go:39`), leaving a window for operators who turn it off.
- **Nexus callbacks to arbitrary URLs**: external-endpoint callbacks use a per-deployment URL template (`components/nexusoperations/executors.go:125-165`); the token travels as a plaintext header (`common/nexus/callback_token.go:19`) and completion validity rests on the unguessability of the URL plus TLS — no signature verification exists yet (explicit TODO at `common/nexus/callback_token.go:28-33`).
- **JWT algorithm confusion**: mitigated — HMAC keys unsupported and `SupportedMethods` pins RS256/ES256 (`common/authorization/default_token_key_provider.go:93-95,189-191`), and parser validates against the allowlist (`common/authorization/default_jwt_claim_mapper.go:155`).

## Future Considerations

- Sign or encrypt Nexus callback tokens (the code reserves the evolution path: "More fields and encryption support will come later", `common/nexus/callback_token.go:28-33`) so completion does not depend on callback-URL secrecy.
- Add cryptographic binding (HMAC) to task tokens so worker-side compromise or TLS termination proxies cannot mint/modify them.
- Provide a first-party OPA/Cedar-style authorizer example in-tree, since the plugin point exists (`common/authorization/authorizer.go:54-56`) but integrators currently reimplement policy glue.
- Make `frontend.enableTokenNamespaceEnforcement` immutable/removable once fleet compatibility allows, closing the operator-opt-out gap (`common/dynamicconfig/constants.go:924-928`).
- Extend per-API metadata into richer resources (workflow type, task queue) — the `CallTarget` docstring already anticipates this (`common/authorization/authorizer.go:23-24`).

## Questions / Gaps

- **Secrets access**: No evidence found of server-side secret management (vault integration, payload encryption at rest) inside this repository. Searched `common/persistence` and config for encryption-at-rest hooks; payloads are treated as opaque blobs and encryption is delegated to client-side data converters, with datastore TLS handled via `common/config/config.go` persistence settings. Archiver providers take credential *paths* from config (e.g., GCS keyfile at `config/development-jwt.yaml:130`), meaning secret custody follows standard file/env mechanisms.
- **Network/egress policy**: No evidence found of outbound-call filtering for Nexus handlers or archivers beyond operator-controlled endpoint registration (`common/api/metadata.go:203-207`) and callback URL templates (`components/nexusoperations/executors.go:125-146`). What I searched: `common/nexus`, `components/nexusoperations`, `common/rpc`. SSRF surface is thus governed entirely by which endpoints operators register.
- **Per-workflow or per-user delegation**: The claims model supports roles per namespace only (`common/authorization/roles.go:29-31`); there is no evidence of per-workflow-type ACLs. `Extensions any` (`common/authorization/roles.go:33`) is the designated escape hatch, unused by the shipped mappers.
- **Rate limiting as capability throttle**: namespace and global rate/concurrency limiters sit in the interceptor chain (`service/frontend/fx.go:307-309`), but their configuration semantics were not analyzed in depth here (out of dimension scope); treat as present-but-unexamined safeguard.

---

Generated by `08.01-capability-model-and-trust-boundaries` against `temporal`.

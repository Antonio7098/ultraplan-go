# Source Analysis: temporal

## 02.07 Session, Thread, and User Boundaries

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (`go.mod` at `sources/temporal/go.mod`); gRPC frontend; pluggable SQL/NoSQL persistence (Postgres, MySQL, SQLite, Cassandra, Elasticsearch); UUID-based domain identifiers; JWT/mTLS authentication via `common/authorization`. |
| Analyzed | 2026-07-11 |

## Summary

Temporal is a workflow-orchestration engine, not an agent harness, so the dimension vocabulary maps as follows: there is no native concept of "session", "thread", or "conversation". The unit of durable user-supplied identity is the **Namespace** (human-friendly `Name` + opaque UUID `ID`), which is enforced as the universal tenant boundary. Inside a namespace, durable per-workflow identity is `(NamespaceID, WorkflowID, RunID)`. Per-request authentication is layered: gRPC method metadata (`Scope: Cluster` vs `Scope: Namespace`) drives a per-API role check; `Claims` carry a `System` bitmask plus a `Namespaces map[string]Role`, and the interceptor also authorizes the *target* namespace of cross-namespace commands (`common/authorization/interceptor.go:347-417`). Storage confirms the model: every primary key in `schema/postgresql/v12/temporal/schema.sql` and the visibility schema starts with `namespace_id`, so two workflows with the same `WorkflowID` in different namespaces cannot collide. There is no "Thread", "Session", "Conversation", "Workspace", "Project", or "Tenant" identifier anywhere in the codebase; the analogous "session" name in `tests/versioning_3_test.go:211-280` is a *worker-side* worker-versioning resource concept, not a user/thread boundary. **Two users cannot accidentally share state across namespaces**, but they can operate inside the same namespace by design, so multi-tenancy is a deployment-level decision rather than a server-level enforcement. Auth is *opt-in* (noop authorizer is the default in `common/authorization/authorizer.go:64-73`, and `DefaultAuthEnabled` is off by default), and JWT subjects are claimed, not database-backed.

## Rating

**7 / 10** — Clear namespace-as-tenant model with explicit types, validated at every gRPC entrypoint (`common/rpc/interceptor/namespace_validator.go`), enforced through SQL `PRIMARY KEY (..., namespace_id, ...)` (`schema/postgresql/v12/temporal/schema.sql:43,61,80,92`), and projected onto per-API role bits (`common/api/metadata.go`, `common/authorization/default_authorizer.go:35-65`). Strong evidence of (a) tests for cross-namespace leaking (`common/authorization/default_authorizer_test.go:126-148`, `common/authorization/interceptor_test.go:341-476`), and (b) storage keys that prevent accidental collisions. Weaker evidence of (a) operational fade — `EnableCrossNamespaceCommands` defaults to `false` but exists, dynamically toggled per cluster (`common/dynamicconfig/constants.go:148-152`); (b) `enableTokenNamespaceEnforcement` is also a dynamic-config opt-in (`common/rpc/interceptor/namespace_validator.go:355-364`, `:578-617`); (c) the noop authorizer and noop claim mapper will silently bypass every check when not configured (`common/authorization/authorizer.go:64-73`, `common/authorization/claim_mapper.go:48-54`); (d) no built-in cross-workflow or cross-namespace "memory" service of the kind the dimension calls "session". Score breakdown below.

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.go:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Primary ID: namespace `Name` + `ID` (UUID) | `type Name string`, `type ID string`, `NewID()` returns UUIDv4 | `common/namespace/namespace.go:32-33`, `:74-76` |
| Primary ID: namespace is the durable tenant row | `namespace.ID` flows into all `GetNamespace` / `ListNamespaces` persistence requests | `common/persistence/data_interfaces.go` (entire `GetNamespaceRequest`, `CreateWorkflowExecutionRequest`, `CreateNamespaceRequest` carry `NamespaceID` or `Name` per row, e.g. `:274-407`) |
| Primary ID: workflow key tri-tuple | `WorkflowKey{NamespaceID, WorkflowID, RunID}` | `common/definition/workflow_key.go:9-13`, `:17-27` |
| Primary ID: chasm execution tri-tuple | `ExecutionKey{NamespaceID, BusinessID, RunID}` | `chasm/ref.go:17-21` |
| ID schemas: namespace metadata persistence row | `persistencespb.NamespaceInfo{Id, Name, State, Owner, Data}` constructed by `RegisterNamespace` | `service/frontend/namespace_handler.go:188-209` |
| ID schemas: namespace metadata table | `CREATE TABLE namespaces(partition_id, id BYTEA, name VARCHAR(255) UNIQUE, …)` with PK keyed on `partition_id + id` | `schema/postgresql/v12/temporal/schema.sql:1-11` |
| ID schemas: executions table partitioned by `namespace_id` | `PRIMARY KEY (shard_id, namespace_id, workflow_id, run_id)` | `schema/postgresql/v12/temporal/schema.sql:30-44` |
| ID schemas: current_executions / current_chasm_executions / buffered_events | All PKs start with `(shard_id, namespace_id, …)`; `current_chasm_executions` PK `(shard_id, namespace_id, business_id, archetype_id)` | `schema/postgresql/v12/temporal/schema.sql:46-93` |
| ID schemas: task_queues / task_queue_user_data | PKs include `namespace_id BYTEA NOT NULL` (worker-versioning, builds) | `schema/postgresql/v12/temporal/schema.sql:142-158` |
| ID schemas: visibility table indexed by `namespace_id` first | `executions_visibility` with `namespace_id CHAR(64) NOT NULL`; `PRIMARY KEY (namespace_id, run_id)`; 7 secondary indexes all keyed `(namespace_id, …)` | `schema/postgresql/v12/visibility/schema.sql:17`, `:109-119` |
| Store query: namespace lookup by name → ID translation at every frontend entrypoint | `namespaceID, err := wh.namespaceRegistry.GetNamespaceID(namespace.Name(request.GetNamespace()))` then propagated into RPCs to matching/history | `service/frontend/workflow_handler.go:554-557`, `:3227-3230`, `:3283-3287` |
| Store query: persistence requests carry `NamespaceID`, not name | `CreateWorkflowExecutionRequest` ultimately writes to the (shard_id, namespace_id, workflow_id, run_id) PK | `common/persistence/data_interfaces.go:204-218`, `:291-297`, `:401-416` |
| Authorization filter: `ScopeCluster` vs `ScopeNamespace` per API | `workflowServiceMetadata` map declares each API's Scope/Access | `common/api/metadata.go:69-249` |
| Authorization filter: default authorizer joins System + per-Namespace roles | `hasRole = claims.System | claims.Namespaces[target.Namespace]` | `common/authorization/default_authorizer.go:35-65` |
| Authorization filter: cross-namespace commands separately authorized | `authorizeTargetNamespaces` walks `RespondWorkflowTaskCompletedRequest.Commands` and re-authorizes `SignalExternalWorkflow`, `StartChildWorkflow`, `RequestCancelExternalWorkflow` against the target namespace's roles | `common/authorization/interceptor.go:347-417` |
| Authorization filter: history-side namespace validator | `validateCrossNamespaceCall` rejects cross-namespace commands unless the dynamic-config `EnableCrossNamespaceCommands` is true AND both namespaces share a cluster list | `service/history/api/command_attr_validator.go:581-624` |
| Authorization filter: dynamic-config knobs | `EnableCrossNamespaceCommands = NewGlobalBoolSetting(…, false, …)`, `EnablePrincipalPropagation`, `DisableStreamingAuthorizer`, `EnableTokenNamespaceEnforcement`, `ExposeAuthorizerErrors` | `common/dynamicconfig/constants.go:148-152`; `common/rpc/interceptor/namespace_validator.go:28`, `:355-364` |
| Authorization filter: noop authorizer bypass | `noopAuthorizer.Authorize` always returns `DecisionAllow` | `common/authorization/noop_authorizer.go:5-13` |
| Authorization filter: noop claim mapper grants system-wide admin | `noopClaimMapper.GetClaims` returns `&Claims{System: RoleAdmin}` so a misconfigured server is wide-open | `common/authorization/claim_mapper.go:48-54` |
| Thread config: `NamespaceValidateIntercept` rejects requests with no namespace set on the request (or derived from a task token) | `extractNamespaceFromRequest` switch handles 4 special cases (`DescribeNamespace`, `RegisterNamespace`, `Operator.DeleteNamespace`, search-attributes admin) | `common/rpc/interceptor/namespace_validator.go:233-322`, `:111-129` |
| Thread config: task-token / request-namespace parity check (when feature on) | `checkNamespaceMatch` returns `errTaskTokenNamespaceMismatch` if `requestNamespace.ID() != tokenNamespace.ID()`; gated by `EnableTokenNamespaceEnforcement` | `common/rpc/interceptor/namespace_validator.go:355-364` |
| Thread config: namespace state must be correct for the API | `checkNamespaceState`, `allowedNamespaceStatesPerAPI` (e.g. `StartWorkflowExecution` only on `REGISTERED`, `DeleteNamespace` allows deleted) | `common/rpc/interceptor/namespace_validator.go:42-55`, `:366-386` |
| Thread config: namespace handover state closes write paths to prevent stale reads | `checkReplicationState` returns `ErrNamespaceHandover` unless method is in `allowedMethodsDuringHandover` | `common/rpc/interceptor/namespace_validator.go:388-406`, `:57-87` |
| Thread config: per-tenant rate limits at both cluster and namespace level | `NamespaceRateLimitInterceptor` configured with `GlobalNamespaceRPS`, `GlobalNamespaceVisibilityRPS`, `MaxNamespaceRPSPerInstance`, `MaxNamespaceBurstRatioPerInstance` | `service/frontend/fx.go:524-602` |
| Thread config: per-namespace "concurrent long-running request" cap | `NamespaceCountLimitInterceptorProvider` (Cluster-wide `MaxConcurrentLongRunningRequestsPerInstance`) | `service/frontend/fx.go:604-618` |
| Tenant isolation: principal headers are stripped at the door so a caller cannot impersonate another tenant's identity | `headers.StripPrincipal(ctx)` is called unconditionally before the authorizer is invoked | `common/authorization/interceptor.go:158`, `common/headers/headers.go:125-135` |
| Tenant isolation: principal is recomputed server-side and stored as `commonpb.Principal{Type, Name}` in `Result.Principal` after `Authorize` returns allow | `Result{Decision, Reason, Principal *commonpb.Principal}`; set into context only when `enablePrincipalPropagation(namespace)` is on | `common/authorization/authorizer.go:38-50`, `common/authorization/interceptor.go:175-177`, `:304-331` |
| Tenant isolation: principal propagation header list (server-set, never trusted inbound) | `propagateHeaders` lists the two `temporal-principal-*` headers | `common/headers/headers.go:24-25`, `:30-43` |
| Tenant isolation: `internalClaimMapper` identifies server-internal calls as `Principal{Type:"temporal", Name:"internal"}` with `System: RoleAdmin` | Used by the InternalFrontend to bypass external API auth checks | `common/authorization/claim_mapper.go:61-78`, `common/authorization/principal.go:4-8` |
| Memory lookup rules: per-workflow memo is part of `WorkflowExecutionInfo`, not cross-workflow | `map<string, Payload> memo = 53;` inside `WorkflowExecutionInfo` | `proto/internal/temporal/server/api/persistence/v1/executions.proto:56`, `:146` |
| Memory lookup rules: search attributes are filtered by a per-namespace mapper; cross-namespace lookup is not exposed | `GetCustomSearchAttributesMapper` reads from the registry; `AliasFields` is called with the request's namespace | `common/namespace/nsregistry/registry.go:387-403`, `service/frontend/workflow_handler.go:3251-3257` |
| Memory lookup rules: no "shared memory store" between workflows — grep for `SessionID`, `ThreadID`, `ConversationID` returns no hits in `sources/temporal/` | `No code paths found` | `grep -n "SessionID\\|ThreadID\\|ConversationID" sources/temporal -r` (negative result; matches limited to Cassandra session, worker session stats in `tests/versioning_3_test.go:211-280`) |
| Fork / clone: `ContinueAsNew`, retry, cron, and reset all generate a fresh RunID within the same `(NamespaceID, WorkflowID)` | `baseRunID, newRunID` arguments on `AddWorkflowTaskFailedEvent`, `ReplicateHistoryEvents`, `NewRunID  string` fields | `service/history/interfaces/mutable_state.go:77`, `service/history/interfaces/engine.go:64`, `:132` |
| Fork / clone: child workflows carry their own `(NamespaceID, WorkflowID, RunID)` | `ChildExecutionInfos` map persisted in mutable state, no cross-process pointer | `common/persistence/data_interfaces.go:355`, `:386` |
| Test evidence: namespace-scoped authorization | `claimsNamespaceAdmin` on `targetNamespaceWriteBar` denied; `claimsBarAdmin` on `BAR` denied (case-sensitive) — guards typo/case bypass | `common/authorization/default_authorizer_test.go:124-149` |
| Test evidence: cross-namespace commands suite | `TestCrossNamespaceCommands_Authorized`, `TestCrossNamespaceCommand_Unauthorized`, `TestCrossNamespaceCommand_DisabledFeature`, `TestMultipleTargetNamespaces` | `common/authorization/interceptor_test.go:341-476`, `:614` |
| Test evidence: token/request namespace enforcement | `Test_StateValidationIntercept_TokenNamespaceEnforcement` table covers matched, mismatched, and disabled cases | `common/rpc/interceptor/namespace_validator_test.go:578-619` |

## Answers to Dimension Questions

### 1. What scopes state?

State is partitioned at three nested scopes. The outermost is **cluster** (a Temporal deployment; multi-cluster is a separate dimension). Inside the cluster the durable tenant is **namespace**, identified by both a user-supplied `Name` (`common/namespace/namespace.go:36`) and an opaque `ID` (`common/namespace/namespace.go:33`). Inside a namespace, durable per-workflow identity is `(NamespaceID, WorkflowID, RunID)` (`common/definition/workflow_key.go:9-13`); CHASM (the component framework) uses the analog `(NamespaceID, BusinessID, RunID)` (`chasm/ref.go:17-21`). Inside a Workflow, the `RunID` covers one *execution attempt*; the `WorkflowID` covers retries, ContinueAsNew chains, and cron-resets — a single WorkflowID may have many RunIDs.

Per-user identity is layered on top of the namespace scope via JWT `Subject` + `Namespaces map[string]Role` claims (`common/authorization/roles.go:25-36`). Per-process identity (worker versioning, deploys) is encoded in `BuildId` / `Deployment` series and task-queue scaffolding (`common/worker_versioning/`, schema rows in `tasks / task_queues / task_queue_user_data`).

There is **no** `tenant_id`, `workspace_id`, `project_id`, `session_id`, `thread_id`, or `conversation_id` type anywhere in the repo. A grep for `SessionID|ThreadID|ConversationID` against `sources/temporal/` returns only false positives (Cassandra DB sessions, cluster-member session times, and worker-versioning "session" task queues in `tests/versioning_3_test.go:211-280` — those are worker-side resource conventions, not user/session boundaries).

### 2. Can state leak across users or sessions?

**Across namespaces: no.** Every persistence request is keyed (directly or transitively) by `namespace_id`, every table has a `namespace_id`-prefixed primary key, and the namespace is resolved and validated by `common/rpc/interceptor/namespace_validator.go:233-322` *before* the request reaches a handler. The `Authorization` interceptor separately re-checks per-call (`common/authorization/interceptor.go:160-184`). For polls / task completions the additional `EnableTokenNamespaceEnforcement` flag closes the "I have a task token for namespace A but I'm calling with namespace B" hole (`common/rpc/interceptor/namespace_validator.go:355-364`).

**Across users inside the same namespace: yes, by design.** Two JWT subjects with `Namespace: RoleWriter` on the same namespace can read and write each other's workflows. Authorization keys off `Namespace` and role, not off `Subject`. There is no subject-scoped table or row that gets filtered with `created_by_subject = ?`. The granularity below namespace is the WorkflowID, which *is* the user-supplied identity — the server assumes WorkflowIDs are unique per logical workflow the user wants to isolate.

**Temporal's threat model is namespace trust, not user trust:** a single namespace is considered an authenticated micro-tenant run by one human or one team. The deprecation of the JWT `sub` claim for per-workflow authz is explicit in `common/authorization/default_authorizer.go:54` (`claims.System | claims.Namespaces[target.Namespace]`) — only `target.Namespace` is checked, not the caller's `Subject`.

**Cross-namespace commands can leak intent across tenants** when `EnableCrossNamespaceCommands` is on (`common/dynamicconfig/constants.go:148-152`): `SignalExternalWorkflow`, `StartChildWorkflow`, `RequestCancelExternalWorkflow` will be authorized against the *target* namespace by the interceptor (`common/authorization/interceptor.go:347-417`) and the `validateCrossNamespaceCall` history-side check (`service/history/api/command_attr_validator.go:581-624`). Default is `false`, so this is behind a feature flag.

**Auth can be silently disabled.** Out of the box the authorizer is `noopAuthorizer` (`common/authorization/authorizer.go:65-72`) and the claim mapper is `noopClaimMapper` which grants `System: RoleAdmin` (`common/authorization/claim_mapper.go:48-54`). A misconfigured cluster will admit any caller; there is no startup assertion that the authorizer is "real".

**Operator/Admin paths** are cluster-scoped, not namespace-scoped (see `ScopeCluster` in `common/api/metadata.go:73-86`). A `RegisterNamespace`, `UpdateNamespace`, `ListNamespaces`, or any admin operation implicitly trusts `System` role.

### 3. Is cross-thread memory explicit?

There is no cross-workflow memory service. Memory is per-workflow: the `memo` map and `search_attributes` are fields of `WorkflowExecutionInfo` (`proto/internal/temporal/server/api/persistence/v1/executions.proto:146`). Search attributes cross namespace boundaries only via the public Visibility API (which itself is namespace-scoped — `DescribeWorkflowExecution` is `ScopeNamespace`, see `common/api/metadata.go:74-77`). There is no `lookupMemory(otherWorkflowId)` API; cross-workflow must be implemented by the application through external channels (e.g., a Nexus endpoint or a child workflow). The Nexus operation model — which is the only "intra-server RPC to another workflow's runtime" mechanism — is itself namespace-scoped (`common/nexus/`, `components/nexus/`) and requires explicit endpoint registration before invocation.

So the answer to "is cross-thread memory explicit?" is **"there is no implicit cross-workflow memory; cross-workflow coordination is via explicit user code using external stores or Nexus/callback APIs."** No leakage is possible because no shared store exists.

### 4. Are tenant boundaries enforced at storage level?

**Yes, structurally.** Every persistable table the workflow runtime writes to is keyed by `namespace_id`:

- `executions PK (shard_id, namespace_id, workflow_id, run_id)` (`schema/postgresql/v12/temporal/schema.sql:30-44`)
- `current_executions PK (shard_id, namespace_id, workflow_id)` (`:46-62`)
- `current_chasm_executions PK (shard_id, namespace_id, business_id, archetype_id)` (`:64-81`)
- `buffered_events PK (shard_id, namespace_id, workflow_id, run_id, id)` (`:83-93`)
- `task_queues` / `task_queue_user_data` PK includes `namespace_id BYTEA NOT NULL` (`:142-158`)
- `executions_visibility PK (namespace_id, run_id)` plus 7 indexes all `(namespace_id, …)` (`schema/postgresql/v12/visibility/schema.sql:109-119`)

The Cassandra and Elasticsearch schemas follow the same `namespace_id` partition-key discipline (`schema/cassandra/`, `schema/elasticsearch/`). Because `namespace.Name` is mutable (a namespace can be renamed), the `Name` is never used as a partition key — only the opaque `namespace.ID` is, which is what guarantees rename safety (`service/frontend/namespace_handler.go:379-651`).

What is *not* enforced at storage level is `created_by`. Row-level audit columns like `created_by_user_id` do not exist on the persistence tables I sampled; identities do not propagate into the row. Authorization above the storage layer is the only thing linking a user/principal to a row.

### 5. Can users fork sessions safely?

A "fork" in Temporal maps to **ContinueAsNew / retry / reset / child workflow**, all of which create a fresh `RunID` under the same `(NamespaceID, WorkflowID)` or a new `(NamespaceID, ChildWorkflowID, ChildRunID)`. The history is append-only per Run; the parent Run is sealed, the child gets its own RunID and its own isolation in the `executions` table. There is no rename of an existing Run, no in-place mutation of `WorkflowExecutionInfo` outside the transaction layer (`common/persistence/`), and no fork that silently aliases a prior Run.

Whether the user "forks safely" depends on the namespace contract: a different user with `RoleWriter` on the same namespace can `TerminateWorkflow`, `ResetWorkflow`, or `StartWorkflowExecution` with a different `WorkflowIdConflictPolicy` and overwrite the original run (`service/history/api/command_attr_validator.go` `validateWorkflowIDReusePolicy`). That is the standard user-trust-the-namespace story.

## Architectural Decisions

- **Two-id namespace model.** `Name` is human-facing, `ID` (UUID) is the durable storage key. `Name` can change via `UpdateNamespace` (`service/frontend/namespace_handler.go:451-475`), but `ID` is permanent. All persistence keys and all RPCs use `ID`; only the public API surface accepts `Name`. Tradeoff: callers can rename without breaking data, but the registry must atomically update both `nameToID` and `idToNamespace` (`common/namespace/nsregistry/registry.go:712-720`, `:820-844`) and the rename window is observed by API callers, who can hit the registry before/after a refresh.
- **Cache-first namespace registry.** `namespace/nsregistry/registry.go` keeps two `map`s (`nameToID` and `idToNamespace`) with readthrough, NotFound TTL cache, watch-or-poll, and a callback system so that downstream caches (e.g. config caches, history shard routing) refresh on namespace events (`:273-321`, `:556-652`, `:407-430`). This puts the tenant-boundary decision into a single hot-path and lets every request validate the namespace cheaply.
- **gRPC interceptor chain puts auth and namespace validation *outside* the handler.** The chain order at `service/frontend/fx.go:274-303` is: mask-errors → service-error → frontend-error → businessID-extract → `NamespaceValidateIntercept` → log → metrics → `AuthInterceptor` → handover → redirection → telemetry → health → `StateValidationIntercept` → rate-limit → sdk-version → caller-info. Anything that touches a workflow has been validated for namespace existence, state, and authorization before the handler runs.
- **Per-API role metadata, not per-handler checks.** Every method has `Scope` (Cluster/Namespace), `Access` (Read/Write/Admin), and `Polling` declared in `common/api/metadata.go:69-249`. The default authorizer looks up the API's required role and compares against the caller's role bits. Adding an API requires declaring metadata; forgetting makes the default deny (`common/authorization/default_authorizer.go:55-58`).
- **Cross-namespace commands as a feature flag.** The system explicitly distinguishes intra-namespace and cross-namespace calls in commands (`service/history/api/command_attr_validator.go:581-624`) and re-authorizes target namespaces in the interceptor (`common/authorization/interceptor.go:347-417`). Default is off.
- **Internal principal is a known identity.** Server-to-server RPC is uniformly tagged `InternalPrincipalType = "temporal" / "internal"` (`common/authorization/principal.go:4-8`) with `System: RoleAdmin`. There is no escaping this identity: inbound `temporal-principal-*` headers are stripped unconditionally (`common/authorization/interceptor.go:158`, `common/headers/headers.go:125-135`).

## Notable Patterns

- **Single nested tri-tuple `(NamespaceID, WorkflowID, RunID)`** is the cross-cutting key in the entire data model: persistence (`common/persistence/data_interfaces.go:200+`), chasm (`chasm/ref.go:17`), execution manager (`service/history/interfaces/engine.go:64`), history events (`common/persistence/data_interfaces.go:333`), archiver (`common/archiver/interface.go:18-32`).
- **Fail-closed defaults with dynamic-config overrides.** `EnableCrossNamespaceCommands=false`, `EnableTokenNamespaceEnforcement` (off by default), auth off by default. Failure mode is silent insecurity if you flip these on without reviewing the corresponding code path.
- **Table-level scoping, not row-level scoping.** Namespaces act as partitions (Redis Cluster-style `partition_id`, SQL PK-style `namespace_id`). There is no user/principal column.
- **Two principal models coexist.** Per-namespace via JWT claims, plus a server-internal fixed identity for `internalClaimMapper` (`common/authorization/claim_mapper.go:61-78`). Internal RPCs bypass authentication by being typed.
- **Cache invalidation on namespace events.** Registry broadcasts via `StateChangeCallbackFn` (`common/namespace/namespace.go` + `common/namespace/nsregistry/registry.go:279-321`); history service and worker subscribe to react to renames, state transitions, handovers.

## Tradeoffs

- **"Tenant = namespace" leaks the assumption that one namespace is one trust boundary.** Operators that try to wedge many users into one namespace will need to layer their own user-scoped filtering; the server will not enforce it. Conversely, operators that give each user a namespace pay for one row per user plus a refresh hit per call.
- **`Name`-mutability buys ergonomics at the cost of two-map consistency.** Renames must be atomic across `nameToID` and `idToNamespace` (`common/namespace/nsregistry/registry.go:626-632`); in between there is a window where `GetNamespace(oldName)` returns NotFound.
- **Dynamic config as an attack surface.** `EnableCrossNamespaceCommands`, `EnableTokenNamespaceEnforcement`, `EnablePrincipalPropagation`, `DisableStreamingAuthorizer` are all globally togglable via dynamic config (`common/dynamicconfig/constants.go:148-152` plus `common/dynamicconfig/constants.go` for the namespace filter pattern). A compromised `system.updateDynamicConfig` flow can quietly open cross-namespace paths.
- **Auth + claim mapper each have a noop default.** Lower friction for self-hosted single-user setups, but means accidental production deploys with `claimMapper = ""` are wide-open (`common/authorization/claim_mapper.go:80-89`).
- **`NamespaceValidateIntercept` excludes token-from-token verification** unless `EnableTokenNamespaceEnforcement` is on (`common/rpc/interceptor/namespace_validator.go:355-364`); a stolen task token + a different `request.Namespace` would otherwise silently authorize. The default deny is conservative.
- **Namespace handover state** blocks read paths to prevent stale reads (`common/rpc/interceptor/namespace_validator.go:388-406`). Tradeoff: data unavailability during the handover window.
- **No "user" table.** Identities live in the user's IdP, not in Temporal. The schema has zero `user_id` columns. Tradeoff: zero schema migrations when you swap JWT providers; cost: Temporal cannot surface "who created workflow X" without a sidecar.

## Failure Modes / Edge Cases

- **`EnableCrossNamespaceCommands=true` cluster** with a JWT whose `Namespaces` map omits the target namespace is denied at the frontend interceptor (`common/authorization/interceptor.go:347-417`) but the `validateCrossNamespaceCall` history-side check (`service/history/api/command_attr_validator.go:581-624`) is namespace-routing-only — it does not re-check claims. The defense-in-depth works only as long as the command passes through the frontend; replication of pre-existing workflows bypasses the frontend entirely.
- **Task tokens carry a `NamespaceId`** (`common/persistence/data_interfaces.go` + `tasktoken.Serializer`); without `EnableTokenNamespaceEnforcement`, a worker can reuse a token from one namespace on another. The validator still requires the request to mention *some* namespace and the namespaces must exist (`common/rpc/interceptor/namespace_validator.go:324-353`), but a mismatched request is currently allowed when the feature flag is off (`:355-364`, test coverage at `namespace_validator_test.go:603-618`).
- **Cache poisoning on rename.** A readthrough that resolves the old name to the old ID is impossible (the registry updates the `nameToID` map atomically), but a client that caches the `ID -> Namespace` map locally will see stale config. There is no client-side cache invalidation hook.
- **Concurrency on `RegisterNamespace`.** Race between two clients registering the same `Name`: frontends both pass the existence check, then one wins the `CreateNamespace` and the other gets a duplicate-key error after persistence (`service/frontend/namespace_handler.go:122-132`). No namespaced idempotency token is exposed to the client.
- **`NoopAuthorizer` in production.** Configuration without an explicit `Authorization.Authorizer` value returns `NewNoopAuthorizer()` (`common/authorization/authorizer.go:65-72`); every namespace then sees the full set of internal admin operations. Audit logs (`metrics.ServiceErrUnauthorizedCounter`) never increment.
- **`internalClaimMapper` is hard-coded; it cannot be reassigned to a non-internal principal.** A caller that can route an RPC through the `InternalFrontendService` (``primitives.InternalFrontendService`)` sees `RoleAdmin`. Split-brain exposure if the InternalFrontend listener is reachable from outside the cluster.
- **Streaming RPC auth (`disableStreamingAuthorizer`).** When set to true, the stream interceptor is bypassed (`common/authorization/interceptor.go:188-238`). The default is false (auth on), but the knob exists.
- **Namespace state transition (e.g. `REGISTERED → DEPRECATED → DELETED`) is non-atomic with workflow refusal.** A workflow can be in flight from a client while a namespace is being deprecated; the namespace validator rejects *new* `REGISTERED`-required APIs (`common/rpc/interceptor/namespace_validator.go:42-55`) but does not cancel in-flight workflows.

## Future Considerations

- **Per-namespace audit chain.** Today the server logs `WorkflowID` and `RunID` but not the JWT `Subject`. For compliance use-cases (`Created by who?`) the principal would have to be plumbed into persistence metadata.
- **`namespace_id` as a column on every table** is already done; the natural next step is exposing it as a queryable attribute via the Visibility API without re-keying.
- **Implicit "user" identity per workflow** would require schema and a `created_by_principal` field. No schema migration exists yet.
- **Cross-namespace callback / Nexus.** Nexus endpoints can be registered per-namespace (`common/nexus/endpoint_registry.go`); once registered they can be called cross-namespace by clients with the right role. Authorization is API-level, not endpoint-level (any client authorized on the namespace can hit any endpoint in it).
- **`EnableTokenNamespaceEnforcement` default off.** The natural progression would be to default on.
- **CHASM (`chasm/`) is a newer component framework** that reuses the same `NamespaceID` scope (`chasm/ref.go:17-21`). No "session" concept exists here either; CHASM introduces an `archetype_id` and a `componentPath` for sub-component addressing but neither carries a session/thread/user notion.

## Questions / Gaps

- Is the cluster-level `System` role scoped per-cluster (Temporal cluster) or per-deployment? In multi-cluster (`global` namespace) scenarios, the `System` claim propagates globally because the JWT is presented to whichever cluster is active.
- The "mTLS principal" path (`common/authorization/interceptor.go:251-285` reads `pkix.Name` from client cert) bypasses `Namespaces` resolution entirely and falls back to the `noopClaimMapper` if no `AuthInfo.AuthToken` is set. The default claim mapper for mTLS-only deployments is unclear.
- `OperatorHandlerImpl` and `AdminHandler` accept `GetNamespace` lookups by either ID or name (`common/rpc/interceptor/namespace_validator.go:266-322`); there is no separate `Operator` namespace scope distinct from `Cluster`.
- Worker-versioning "sessions" (`tests/versioning_3_test.go:211-280`) are worker-side resource concepts, not user-facing session boundaries; this report treats them as out-of-scope but notes their naming collision.
- No evidence found for: per-user rate limits (only per-namespace), per-user persistence filters, cross-namespace memory, "thread" or "conversation" identifiers.

---

Generated by `dimensions/02.07-session-thread-user-boundaries.md` against `temporal`.

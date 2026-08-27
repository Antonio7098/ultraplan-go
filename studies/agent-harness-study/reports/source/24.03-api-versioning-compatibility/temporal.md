# Source Analysis: temporal

## API Versioning and Compatibility

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go 1.26.4 / gRPC+Protobuf / Cassandra/MySQL/PostgreSQL/SQLite + Elasticsearch |
| Analyzed | 2026-08-27 |

## Summary

Temporal Server (go.temporal.io/server) treats API compatibility as a layered problem: **SDK↔Server wire compatibility** via semver header negotiation, **server↔server and SDK↔server feature negotiation** via `supported-features`/`Capabilities`, and **persisted schema compatibility** via `curr_version`/`min_compatible_version` manifests with startup verification. The control plane (`common/headers/version_checker.go:12-61`, `common/rpc/interceptor/sdk_version.go:31-45`, `service/frontend/workflow_handler.go:3498-3525`) is executable and tested. Schema migration (`tools/common/schema/updatetask.go:156-307`, `common/persistence/schema/version.go:10-30`) is sequential and idempotency-tolerant but only enforces forward monotonicity. Proto evolution relies heavily on `deprecated = true` annotations and `[cleanup-old-wv]` markers scattered across `proto/internal/temporal/server/api/**` without a formal deprecation policy document or changelog in-repo. Persisted workflow state handles versioning transitions gracefully (worker-deployment versioning v1→v2→v3 coexistence) but the compatibility model is policy-plus-code, not proof — integrators cannot upgrade without at least auditing `SupportedClients`/`SupportedServerVersions` ranges and schema manifests.

## Rating

**6 / 10** — Present but inconsistent, weakly documented, fragile at the edges.

**Rationale:** Executable semver negotiation + feature flags + capability advertisement + schema-version manifests exist and are tested (`common/headers/version_checker_test.go:28-112`, `common/rpc/interceptor/sdk_version_test.go:13-66`). Proto deprecations are pervasive and startup schema verification gates rollout (`temporal/fx.go:950-964`). However: no `CHANGELOG.md`/`VERSION`/`docs/versioning` in source; deprecation timelines are `TODO`/`cleanup-old-wv` comments, not policy; `ServerVersion` is a single hardcoded string (`common/headers/version_checker.go:26`) mutated by a GitHub workflow, not derived; backwards-compatibility is tested only for the integer feature flag, not for proto field removal or persisted-state round-trips; and `go.mod:5-9` retracts are the only formal breaking-change signal.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Client/Server Semver Negotiation | `ServerVersion = "1.32.0"` + `SupportedServerVersions = ">=1.0.0 <2.0.0"` + `SupportedClients` map with `<2.0.0` / `<3.0.0` ranges; `VersionChecker.ClientSupported` parses both headers bidirectionally and returns `ClientVersionNotSupported`/`ServerVersionNotSupported` | `common/headers/version_checker.go:26-31`, `common/headers/version_checker.go:44-53`, `common/headers/version_checker.go:116-148` |
| Feature/Capability Negotiation | `FeatureFollowsNextRunID = "follows-next-run-id"`; `AllFeatures` joined by `,`; `ClientSupportsFeature` splits `supported-features` header; `GetSystemInfo` returns hardcoded `Capabilities` with comment "older servers will respond without the field which is implied false" | `common/headers/version_checker.go:31-42`, `common/headers/version_checker.go:150-163`, `service/frontend/workflow_handler.go:3506-3525` |
| Feature Negotiation Usage | `FixFollowEvents` checks `ClientSupportsFeature(ctx, FeatureFollowsNextRunID)` to decide whether to fake a `ContinuedAsNew` event for old SDKs (pre-Sept 2021); `TODO: remove once we no longer support SDK versions prior to ... Revisit once we have an SDK deprecation policy` | `service/history/api/get_history_util.go:555-586`, `service/history/api/get_history_util.go:572-574` |
| gRPC Interception | `SDKVersionInterceptor.Intercept` records SDK name/version and calls `versionChecker.ClientSupported`; `RecordSDKInfo` bounded to `defaultMaxSetSize=100`; `GetAndResetSDKInfo` feeds `VersionChecker` telemetry | `common/rpc/interceptor/sdk_version.go:12-75` |
| Header Propagation | `ClientNameHeaderName`, `ClientVersionHeaderName`, `SupportedServerVersionsHeaderName`, `SupportedFeaturesHeaderName` constants; `Propagate` copies headers frontend→history/matching; `SetVersions` injects internal pairs | `common/headers/headers.go:14-18`, `common/headers/headers.go:60-78`, `common/headers/version_checker.go:55-61` |
| VersionChecker Tests | `TestClientSupported` covers empty ctx, unknown client, malformed version, out-of-range `<3.0.0`, supported-server range mismatch, and multi-feature header parsing; expects typed errors | `common/headers/version_checker_test.go:28-113` |
| SDKVersionInterceptor Tests | Covers two distinct SDK tuples, over-capacity rejection, empty version/name not recorded, `GetAndResetSDKInfo` returns sorted slice | `common/rpc/interceptor/sdk_version_test.go:13-66` |
| Frontend Gate | `WorkflowHandler.StartBatchOperation`, `UpdateWorkflowExecution`, `PollWorkflowExecutionUpdate`, `DescribeTaskQueue/Sticky` call `versionChecker.ClientSupported(ctx)` before business logic | `service/frontend/workflow_handler.go:5777-5779`, `service/frontend/workflow_handler.go:6049-6051`, `service/frontend/workflow_handler.go:6092-6094`, `service/frontend/workflow_handler.go:6222-6224` |
| Nexus Gate | `nexus_handler.go:265` and `nexus_completion_http_handler.go:674` enforce `clientVersionChecker.ClientSupported` on Nexus ingress | `service/frontend/nexus_handler.go:265`, `service/frontend/nexus_completion_http_handler.go:674` |
| Schema Version Constants | Per-store release versions: Cassandra `Version="1.13"`; MySQL `Version="1.19" VisibilityVersion="1.14"`; PostgreSQL `Version="1.19" VisibilityVersion="1.14"` | `schema/cassandra/version.go:5-6`, `schema/mysql/v8/version.go:5-6`, `schema/postgresql/v12/version.go:5-6` |
| Schema Manifest Contract | `manifest` struct requires `CurrVersion` + `MinCompatibleVersion` + `Description` + `SchemaUpdateCqlFiles`; `readManifest` normalizes semver and errors on missing `MinCompatibleVersion`; `schema/cassandra/README.md` documents manifest semantics | `tools/common/schema/updatetask.go:38-47`, `tools/common/schema/updatetask.go:267-307`, `schema/cassandra/README.md:30-44` |
| Schema Upgrade Execution | `executeUpdates` → `execStmts` (whitelist CREATE/ALTER/INSERT/DROP/DO, idempotent via "already exist"/"not found" substring) → `updateSchemaVersion(cs.version, MinCompatibleVersion)` + `WriteSchemaUpdateLog` | `tools/common/schema/updatetask.go:108-167`, `tools/common/schema/updatetask.go:132-154`, `tools/common/schema/updatetask.go:251-265` |
| Schema Verification at Startup | `verifyPersistenceCompatibleVersion` calls `cassandra.VerifyCompatibleVersion` and `sql.VerifyCompatibleVersion`; `common/persistence/schema/version.go:9-30` ensures `installed >= expected` (allows rollback after upgrade because changes are backwards-compatible) | `temporal/fx.go:950-964`, `common/persistence/cassandra/version_checker.go:19-62`, `common/persistence/schema/version.go:9-30` |
| SQL Schema Version Table | `schema_version(version_partition, db_name, curr_version, min_compatible_version)` DDL + UPSERT query for MySQL/PostgreSQL/SQLite; cassandra `schema_version(keyspace_name, curr_version, min_compatible_version)` | `common/persistence/sql/sqlplugin/mysql/admin.go:11-23`, `common/persistence/sql/sqlplugin/postgresql/admin.go:13-27`, `common/persistence/sql/sqlplugin/sqlite/admin.go:11-19`, `tools/cassandra/cqlclient.go:56-63` |
| Proto Deprecation Prevalence | 77 `deprecated` hits across `proto/internal/temporal/server/api/**`: old versioning fields marked `Deprecated. [cleanup-old-wv]`, `deprecated = true` on `raw_history`, `adminservice FrontendHttpAddress`, `taskqueue build_id` fields, etc. | `proto/internal/temporal/server/api/persistence/v1/executions.proto:632-688`, `proto/internal/temporal/server/api/taskqueue/v1/message.proto:21-143`, `proto/internal/temporal/server/api/deployment/v1/message.proto:26-298` |
| Versioning Data Evolution | `VersioningData` still carries `versions` + `unversioned_ramp_data` as `[deprecated = true]` alongside new `map<string, WorkerDeploymentVersionData> versions = 2`; comment "Fallback to deployment version when build ID not present" | `proto/internal/temporal/server/api/persistence/v1/task_queues.proto:102-119`, `proto/internal/temporal/server/api/taskqueue/v1/message.proto:116` |
| Worker-Version Compatibility Logic | `version_sets.go` manages `CompatibleVersionSets` (pre-deployment versioning) with max-sets limit; `FindBuildId`, `use_compatible_version` flag persisted in `ActivityInfo`; tests like `versioning_test.go:399,1803` exercise compat sets | `service/matching/version_sets.go:48-435`, `proto/internal/temporal/server/api/persistence/v1/task_queues.proto:49-82`, `proto/internal/temporal/server/api/persistence/v1/executions.proto:634` |
| Persisted Workflow Compatibility Shims | `MutableStateImpl.GetActivityType` falls back to `GetActivityScheduledEvent` "for backwards compatibility"; scheduler `SchedulerWorkflowVersion` comment "Used to keep track of schedules version ... for backward compatibility" | `service/history/workflow/mutable_state_impl.go:1656-1671`, `service/worker/scheduler/workflow.go:163` |
| Module Retraction as Breaking-Change Signal | `go.mod:5-9` retracts `v1.30.0`, `v1.26.1`, `v1.26.0` (accidental publish) | `go.mod:5-9` |
| Version Telemetry | `VersionChecker.performVersionCheck` phones home via `versioninfo.Caller.Call(VersionCheckRequest{Product, Version, Arch, OS, DB, ClusterID, SDKInfo})` every 24h; persists `VersionInfo` to `ClusterMetadata` | `service/frontend/version_checker.go:48-151`, `common/versioninfo/request.go:10-20` |
| No Formal Policy Docs | No `CHANGELOG*`, `VERSION`, `MIGRATION*`, or `docs/versioning` found at source root; `docs/` exists but contains no versioning policy | `schema/cassandra/README.md:1-45` (only schema manifest docs), `go.mod:69` pinned `go.temporal.io/api v1.63.5` (external versioning) |

## Answers to Dimension Questions

### 1. Which APIs are stable, experimental, deprecated, or internal?

- **Stable / Public**: `go.temporal.io/api v1.63.5` (pinned in `go.mod:69`) is the external contract. `service/frontend/workflow_handler.go:3498-3525` `GetSystemInfo` capabilities are the explicit stability surface — each boolean (e.g., `SupportsSchedules`, `BuildIdBasedVersioning`, `Nexus`) is stable and advertised as `true`; absence implies `false` for older servers.
- **Internal**: `proto/internal/temporal/server/api/**` (e.g., `historyservice`, `matchingservice`, `persistence`) is explicitly internal (`proto/internal/...`) — not versioned for external consumers. Tooling lives under `tools/` and `common/`.
- **Deprecated**: Extensive `deprecated = true` / `// Deprecated.` annotations across persistence and deployment protos (`proto/internal/temporal/server/api/persistence/v1/executions.proto:632-688`, `proto/internal/temporal/server/api/taskqueue/v1/message.proto:21-143`, `proto/internal/temporal/server/api/deployment/v1/message.proto:26-298`). Marker `[cleanup-old-wv]` signals staged removal of Build-ID-based versioning after Worker-Deployment versioning landed. AdminService has `// Deprecated. Use operatorservice instead.` (`proto/internal/temporal/server/api/adminservice/v1/service.proto:85-97`).
- **Experimental / Opt-in**: `common/headers/headers.go:27` `temporal-experiment` header + `IsExperimentRequested` (`common/headers/headers.go:106-123`) gates experimental features via `*` or named experiments, length-limited to 100 chars.
- **No explicit stability matrix**: No `api/OPERATOR.md` or `docs/api-stability` found; stability is inferred from proto package (`api` vs `internal`) and deprecation comments, not a published tier list.

### 2. How are users warned before breaking changes?

- **Executable warning – SDK rejection**: `SDKVersionInterceptor.Intercept` (`common/rpc/interceptor/sdk_version.go:40-42`) returns `ClientVersionNotSupported` (`common/headers/version_checker.go:131`) before handler execution; frontend explicitly gates `StartBatchOperation` etc. Error typing is preserved through `common/rpc/interceptor/request_error_handler.go:171-172` (not retryable). Users see a typed `serviceerror`.
- **Executable warning – Server↔Server mismatch**: `VersionChecker.ClientSupported` also validates `SupportedServerVersions` header against `ServerVersion` (`common/headers/version_checker.go:137-145`) returning `ServerVersionNotSupported`; internal `SetVersions` propagates this on every inter-service call (`common/headers/version_checker.go:100-102`).
- **Executable warning – Schema mismatch**: Startup fails fast with `cassandra schema version compatibility check failed` / `sql schema version compatibility check failed` (`temporal/fx.go:957-961`) if DB `curr_version` < expected code version.
- **Non-executable warnings**: Proto deprecation comments and `go.mod` retractions are the only pre-break signals. No `@deprecated` migration guide, no `CHANGELOG.md` in this source snapshot, and `TODO(bergundy): Extract and save version info per SDK` (`service/frontend/version_checker.go:158`) shows upgrade advisories are incomplete. `service/history/api/get_history_util.go:572-574` literally `TODO: remove once we no longer support SDK versions prior to ~Sept 2021. Revisit once we have an SDK deprecation policy.` — acknowledges missing policy.

### 3. Are old clients, plugins, traces, or persisted artifacts still usable?

- **Old SDK clients**: Yes, within `<2.0.0` (and `<3.0.0` for UI). The server supports semver ranges (`common/headers/version_checker.go:44-53`); unknown clients bypass validation (`common/headers/version_checker.go:124-133`: `if clientName != "" && clientVersion != "" && known`). `headers_test.go:31,51,75` and `version_checker_test.go:45-52` confirm empty/unknown clients are accepted. However, `TestSDKVersionRecorder` (`common/rpc/interceptor/sdk_version_test.go:58-65`) and `TestGetSystemInfo` (`service/frontend/workflow_handler_test.go:2657-2670`) show newer SDK versions declare broader `AllFeatures`.
- **Persisted artifacts (workflow histories, executions)**: Yes, with shims. `MutableStateImpl.GetActivityType` reads `ActivityInfo.ActivityType` if present else re-parses `ActivityTaskScheduled` event (`service/history/workflow/mutable_state_impl.go:1656-1671`). `ActivityInfo.use_compatible_version`, `last_independently_assigned_build_id`, `last_worker_version_stamp` are all marked deprecated but still read (`proto/internal/temporal/server/api/persistence/v1/executions.proto:631-653`). `VersioningData` retains both old `DeploymentVersionData versions [deprecated]` and new `WorkerDeploymentVersionData` map (`proto/internal/temporal/server/api/persistence/v1/task_queues.proto:102-119`).
- **Traces / Replication history**: Event `Version` + `VersionHistory` is branch-aware (`service/history/api/get_workflow_util.go:100`, `service/history/replication/sync_state_retriever.go:456`); older branches remain readable. `HistoryBuilder.CreateActivityTaskStartedEvent` still carries `versioningStamp` for replay.
- **Plugins / Worker deployments**: Transitional. Old Build-ID versioning (`CompatibleVersionSets` in `service/matching/version_sets.go`) coexists with Worker Deployment versioning. `workflow_handler.go:6668-6673` validates mutually exclusive `add_new_compatible_version` vs deployment APIs. Ramping data retains untyped fallback (`proto/internal/temporal/server/api/persistence/v1/executions.proto:632` comment).
- **Not proven durable**: No `replay_test.go` round-trip for proto-field removal was found beyond scheduler `replay_test.go:19` and `workerdeployment/replaytester/replay_test.go:23` ("tests workflow logic backwards compatibility from previous versions") — these test workflow *logic*, not proto/schema downgrades.

### 4. Does compatibility rely on policy alone or executable tests?

- **Executable**: 
  - Semver negotiation is unit-tested (`common/headers/version_checker_test.go:28-113` — 13 cases including malformed, out-of-range, multi-feature).
  - SDK interception is tested (`common/rpc/interceptor/sdk_version_test.go:13-66`).
  - Schema `minCompatibleVersion` is validated in `tools/common/schema/updatetask_test.go:140-218` (empty min version rejected, manifest/version mismatch error).
  - `sortAndFilterVersions` and `readSchemaDir` are tested (`tools/common/schema/version_test.go`).
  - Startup `VerifyCompatibleVersion` is integration-gated in `temporal/fx.go:950-964` (fails process start).
  - Versioning compat-sets have dedicated tests (`service/matching/version_sets_test.go:262-322`, `service/matching/matching_engine_test.go:2915`).
- **Policy / Convention**:
  - Proto deprecation lifecycle is comment-driven (`[cleanup-old-wv]`) with no automated `buf breaking` or golden-file check in `Makefile` (`make proto`, `make update-go-api` exist but no `make check-compat` found).
  - `go.mod` retractions are manual.
  - `GetSystemInfo` capabilities rely on the convention that missing = false (`service/frontend/workflow_handler.go:3509-3511`), not a versioned contract test.
  - `SupportedClients = "<2.0.0"` is a broad range — the promise is "any 1.x SDK works", but there is no per-method compatibility matrix or contract test pinning request/response shapes.
- **Verdict**: Core semver + schema path is **executable + tested**; proto/persisted-state evolution is **policy + deprecation comments**, not property-tested.

## Architectural Decisions

- **Bidirectional semver negotiation at the edge** — `common/headers/version_checker.go:116-148` validates both `client-version ∈ SupportedClients[clientName]` **and** `ServerVersion ∈ supportedServerVersions`. Decision: fail fast with typed errors rather than silently degrading. Tradeoff: unknown clients are allow-listed (passthrough), so new SDKs without server update still work, but server cannot enforce policy on them.
- **Header-based capability negotiation, not URL versioning** — Client capabilities (`supported-features: follows-next-run-id`) and serverCapabilities (`GetSystemInfoResponse.Capabilities`) are additive booleans. Decision: avoids `/v1` vs `/v2` endpoints; allows incremental rollout. Tradeoff: every capability needs client opt-in and server fallback; `FixFollowEvents` hack exists precisely because capability was added after the fact.
- **Persisted schema carries both `curr_version` and `min_compatible_version`** — `tools/common/schema/updatetask.go:156-167` writes both on each migration; `common/persistence/schema/version.go:22-28` only checks `curr_version >= expected`. `min_compatible_version` is stored but not enforced at startup (only used by tooling to refuse downgrades). Decision: enables rollback to any version ≥ `min_compatible_version`. Tradeoff: operator can accidentally run old binary against newer schema that claims compatibility but hasn't been tested.
- **Monolithic `ServerVersion` constant mutated by CI** — `common/headers/version_checker.go:26` `ServerVersion = "1.32.0"` with comment "can be changed by the create-tag Github workflow. If you change the var name or move it, be sure to update the workflow." Decision: single source of truth, no `VERSION` file. Tradeoff: version is decoupled from `go.mod` module version; `git describe` drift possible if workflow not run.
- **Coexistence of three versioning generations** — `proto/internal/temporal/server/api/persistence/v1/executions.proto:632-688` + `task_queues.proto:102-119` simultaneously carry `use_compatible_version` (v1), `BuildId` sets (v2), and `WorkerDeploymentVersion` (v3). Marked deprecated but not removed. Decision: zero-downtime rolling upgrades across versioning models. Tradeoff: persistence structs bloat; every reader must handle `nil` vs deprecated field.
- **24h phone-home version check with override via ClusterMetadata** — `service/frontend/version_checker.go:21-134` polls `versioninfo.Caller` (`common/versioninfo/caller.go`) and persists `VersionInfo` (current/recommended/alerts) to `persistence.ClusterMetadata`. Decision: centralize upgrade advisories. Tradeoff: requires outbound network; `TODO: Extract and save version info per SDK` shows per-SDK advisories not yet implemented.

## Notable Patterns

- **Whitelist-enforced schema statements** — `tools/common/schema/updatetask.go:63-64,251-265` only allows `CREATE|ALTER|INSERT|DROP|DO` prefixes; any other CQL is rejected. Idempotency via `already exist` / `not found` substring match tolerates partial retries.
- **Deprecation as staged cleanup tags** — Consistent `// Deprecated. [cleanup-old-wv]` and `// Deprecated. Clean up with versioning-2/3.1` markers allow `grep cleanup-old-wv` to find all removal candidates. No automated removal; `make fmt-imports` / `make lint-code` don't enforce it.
- **Fallback reads for removed fields** — `service/history/workflow/mutable_state_impl.go:1656-1671` checks new field first, then reconstructs from history event. Same pattern in `service/frontend/workflow_handler.go:6426-6478` (`nolint:staticcheck` for deprecated `Error`/`Failure` fields).
- **Capabilities as server-advertised booleans** — `service/frontend/workflow_handler.go:3511-3524` advertises ~10 booleans; clients branch on presence/absence, not version number. Mirrors gRPC `supported-features` negotiation.
- **Experiment header as feature flag** — `common/headers/headers.go:27-123` `temporal-experiment` supports `*` wildcard and comma-separated values, length-capped to prevent abuse; consulted via `IsExperimentRequested` rather than version bump.

## Tradeoffs

- **Broad allow-range vs strict pinning** — `SupportedClients: "<2.0.0"` and `SupportedServerVersions: ">=1.0.0 <2.0.0"` (`common/headers/version_checker.go:29,44-53`) maximize compatibility window but defer breaking changes to a future 2.0. Benefit: no frequent major bumps. Cost: 2.0 negotiation is untested; range semantics could silently accept a future incompatible 1.99.
- **Hardcoded `ServerVersion` vs build-time injection** — Simple but risks skew between binary and `go.mod` tag. Benefit: trivial `NewDefaultVersionChecker` (`common/headers/version_checker.go:78-79`). Cost: local builds report `1.32.0` regardless of branch.
- **Storing `min_compatible_version` without enforcing at startup** — Allows operational flexibility (rollback) but shifts safety to operator discipline. Benefit: upgrades don't block rollback. Cost: `VerifyCompatibleVersion` (`common/persistence/schema/version.go:26-28`) only checks `installed < expected`, not `expected < minCompatible`, so running ancient binary against new schema that narrowed `minCompatible` won't be caught.
- **Keeping deprecated proto fields forever-ish** — Guarantees old persisted data deserializes (`proto/internal/temporal/server/api/persistence/v1/executions.proto:632-688`). Benefit: no migration of historical workflow histories. Cost: wire size, cognitive load, and `oneof build_id_info` branching forever.
- **Header propagation via `metadata.AppendToOutgoingContext`** — `common/headers/version_checker.go:100-102` ensures every internal hop carries version info. Benefit: service-to-service version checks possible. Cost: header length grows; `propagateHeaders` whitelist must be manually extended (`common/headers/headers.go:32-42`).

## Failure Modes / Edge Cases

- **Unknown SDK bypasses validation** — `ClientSupported` (`common/headers/version_checker.go:124-133`) skips check if `clientName` not in `SupportedClients`. A malicious or future SDK with breaking wire format will be admitted; failure surfaces later as deserialization error, not version error.
- **Missing headers treated as compatible** — `version_checker_test.go:38-52` explicitly tests `""` client-name/version and `background` context as `expectErr: false`. Misconfigured client that omits `client-version` silently bypasses semver gate.
- **Schema downgrade not guarded** — `VerifyCompatibleVersion` permits `installed > expected` (comment: "allow rollbacks since we only make backwards compatible schema changes" `common/persistence/schema/version.go:21-22`). If a future migration is *not* backwards compatible (e.g., column drop), rollback will corrupt data; manifest `MinCompatibleVersion` is not consulted at boot.
- **Partial schema upgrade leaves `schema_update_history` incomplete** — `execStmts` tolerates `already exist`/`not found` (`tools/common/schema/updatetask.go:142-146`) but any other error aborts mid-migration. No transactional DDL; retry resumes from `curr_version` but may re-execute already-applied statements.
- **Versioned task-queue compat-set overflow** — `service/matching/version_sets.go:48` enforces `VersionCompatibleSetLimitPerQueue` (dynamic config `common/dynamicconfig/constants.go:516`). Exceeding limit returns `FailedPrecondition`; no automatic compaction, operator must delete sets.
- **Capability drift during rolling upgrade** — `GetSystemInfo` capabilities are hardcoded per binary (`service/frontend/workflow_handler.go:3512-3523`). During mixed-version rolling deploy, clients may see different `Capabilities` responses depending on which frontend they hit; no versioned cache invalidation.
- **Phone-home failure is silent** — `performVersionCheck` (`service/frontend/version_checker.go:93-97`) records `VersionCheckFailedCount` metrics but does not surface to operator beyond metrics; `VersionInfo` staleness only checked via `isUpdateNeeded` (hour threshold `service/frontend/version_checker.go:132-133`).
- **Experiment header DoS** — `IsExperimentRequested` (`common/headers/headers.go:106-112`) skips headers >100 chars but processes `SplitSeq` per header value; a client flooding many small `temporal-experiment` values still iterates O(n) without rate limit.

## Future Considerations

- **Introduce `buf breaking` / `protock` CI** to enforce proto backward-compatibility (field number reuse, type changes) before merge — currently only `make proto` generates, no breaking check.
- **Enforce `min_compatible_version` at startup**, not just `curr_version`, and add `tools/common/schema/version_test.go` case that fails when `expected < minCompatible` to prevent unsafe downgrades.
- **Publish a deprecation policy document** (e.g., `docs/deprecation.md`) mapping `[cleanup-old-wv]` tags to removal SLI (e.g., 2 minor releases) and automate `TODO` aging via linter.
- **Version `Capabilities` explicitly** — add `Capabilities.MinServerVersion` or `ApiVersion` field to `GetSystemInfoResponse` so SDKs can assert required capabilities without probing booleans.
- **Replace hardcoded `ServerVersion` with `debug.ReadBuildInfo` or ldflags injection**; add self-test that binary version matches `go.mod` retracted versions and `schema/{cassandra,mysql,postgresql}/version.go` expectations.
- **Add persisted-state round-trip tests** — serialize/deserialize `WorkflowMutableState` with old vs new `VersioningData` shapes (including unknown fields) to guarantee forward/backward wire reads, similar to existing `version_sets_test.go` but at proto level.
- **Bound `supported-features` / `temporal-experiment` cardinality** — enforce max features per request and max distinct experiments to harden `ClientSupportsFeature`/`IsExperimentRequested` against header bloat.

## Questions / Gaps

- **No evidence of external API changelog or migration guide**: Searched `CHANGELOG*`, `MIGRATION*`, `docs/versioning`, `go.mod` retractions only. `No clear evidence found` — changelog presumably lives in GitHub Releases, not in-repo. Search boundary: `sources/temporal/**` excluding `.git`.
- **Compatibility across SDK/CLI/Server surfaces**: SDKs are covered (`SupportedClients`), CLI is included (`ClientNameCLI: "<2.0.0"`), but OpenAPI/HTTP (`ClientNameServerHTTP`, `nexusrpc`) versioning not found beyond `temporal/fx.go:957` cassandra/sql checks.
- **Trace/persisted artifact long-term retention**: How long are deprecated proto fields retained on disk before compaction? `cleanup-old-wv` comments imply eventual deletion but no retention SLA. Search: `grep -r cleanup-old-wv proto/` shows 30+ hits with no issue tracker link.
- **Operator-facing `min_compatible_version` visibility**: `schema_version` table stores it, but no CLI command was found to surface it (`temporal-cassandra-tool`, `temporal-sql-tool` not enumerated here due to source isolation to `sources/temporal` only). Needs tooling verification.
- **Version negotiation for batch/nexus**: `StartBatchOperation` gates on `ClientSupported`, but does it also gate on specific `Capabilities` (e.g., `Nexus`) ? No evidence of per-RPC capability check beyond version range.

---

Generated by `Dimension 24.03: API Versioning and Compatibility` against `temporal`.

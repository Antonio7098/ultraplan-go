# Source Analysis: temporal

## Embedding and Host Integration Ergonomics (Dimension 24.04)

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (Uber Fx DI, gRPC, SQLite/Cassandra/MySQL/PostgreSQL persistence, OpenTelemetry) |
| Analyzed | 2026-08-25 |

## Summary

Temporal's server is a deliberately embeddable Go library. The `temporal` package exposes a minimal `Server` interface (`Start()`/`Stop()`, `temporal/server.go:16-19`) constructed via `NewServer(opts ...ServerOption)` (`temporal/server.go:44-46`), where ~25 functional options let a host inject configuration, storage, policy/identity, telemetry, interceptors, and lifecycle behavior (`temporal/server_option.go:36-255`). Internally the server is assembled as an Uber Fx dependency graph; each service (frontend/history/matching/worker) is its own fx.App whose dependencies are materialized from host options and re-supplied into per-service graphs (`temporal/fx.go:139-161`, `418-510`). The CLI binary itself is just a thin consumer of the same embedding API (`cmd/server/main.go:222-243`), which is strong evidence that in-process embedding is a first-class mode rather than an afterthought.

Four embedding modes are observable in-tree: (1) **in-process library** via `NewServer`; (2) **test/dev server** via `temporaltest.NewServer` wrapping a SQLite-backed `LiteServer` (`temporaltest/server.go:132-172`, `temporaltest/internal/lite_server.go:203-216`); (3) **CLI subprocess** (`cmd/server/main.go`); and (4) **network service API**, which is the primary integration contract — hosts interact with running workflows through gRPC SDK clients even when the server runs in-process (`temporal/server_test.go:104-123`, `temporaltest/internal/lite_server.go:329-332`). There is no plugin/worker-hosting mechanism inside this repo for third-party code; extension happens through injected interfaces and gRPC interceptors instead.

The model is explicit and well-tested (an in-process end-to-end test asserts zero unexpected error/warn logs across startup and shutdown, `temporal/server_test.go:42-98`, `218-309`), but several load-bearing options are marked "experimental," lifecycle has sharp edges (no restarts, double-stop panics), and two process-global OTEL side effects can collide with a host application's telemetry setup.

## Rating

**8 / 10** — A clear, option-driven embedding model with explicit interfaces for every major dependency category (storage, policy, identity, telemetry, config), exercised by real tests including custom authorizer injection (`temporaltest/server_test.go:193-221`). It falls short of 9-10 because: key extensibility points carry "experimental, may be changed or removed" caveats (`temporal/server_option.go:148,178,186,216,250`), the lifecycle forbids restarts and panics on repeated `Stop` (`temporal/fx.go:349-351`, `temporal/server_impl.go:110`), `otel.SetErrorHandler` is overwritten process-globally (`temporal/fx.go:981,1089`), and embedded startup still relies on timing workarounds (`temporaltest/server.go:168-169`).

## Evidence Collected

Every entry includes a file path with line numbers relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Library entry point | `Server interface { Start() error; Stop() error }`; `NewServer(opts ...ServerOption)` delegates to fx-based construction | `temporal/server.go:15-20`, `temporal/server.go:43-46` |
| Service selection | `ForServices` restricts which of frontend/history/matching/worker/internal-frontend start; validated against allow-list | `temporal/server_option.go:57-65`, `temporal/server_options.go:84-89`, `temporal/server.go:22-41` |
| Host-injectable dependency surface | `serverOptions` struct fields: logger, authorizer, claimMapper, audienceGetter, TLS provider, dynamicConfigClient, dataStoreFactory, visibilityStoreFactory, archiver factories, clientFactoryProvider, persistenceFactoryProvider, searchAttributesMapper, unary/stream interceptors, metricHandler, eventLoggerProvider, tokenProvider, testHooks | `temporal/server_options.go:35-69` |
| Config injection modes | Programmatic struct (`WithConfig`), file path, env/config-dir hierarchy, or fully embedded env-only template | `temporal/server_option.go:37-55`, `common/config/loader.go:119-158`, `common/config/loader.go:111-117` |
| Zero-config startup | Embedded YAML template rendered from environment variables (e.g., `DB`, `CASSANDRA_SEEDS`, `SQLITE_MODE`) used when no config file given | `common/config/config_template_embedded.yaml:4-166`, `cmd/server/main.go:176-178` |
| Storage injection | `WithCustomDataStoreFactory` (experimental), `WithCustomVisibilityStoreFactory`, `AbstractDataStoreFactory` interface "to implement custom datastore support outside of the Temporal core" | `temporal/server_option.go:147-159`, `common/persistence/client/abstract_data_store_factory.go:12-26` |
| Persistence plumbing | Custom factories threaded through namespace init and cluster-metadata bootstrap | `temporal/server_impl.go:92-107`, `temporal/fx.go:646-680` |
| Policy injection | `WithAuthorizer`; `Authorizer.Authorize(ctx, caller, target) (Result, error)` with deny/allow decision, reason, computed principal | `temporal/server_option.go:98-103`, `common/authorization/authorizer.go:52-58`, `38-50` |
| Identity mapping | `WithClaimMapper`, `WithAudienceGetter`, `WithTokenProvider` (outbound remote-cluster auth), plus validation errors naming the exact missing option | `temporal/server_option.go:112-124`, `223-228`, `temporal/fx.go:299-308` |
| Auth wired into pipeline | Authorization interceptor placed inside the frontend interceptor chain; internal-frontend gets a fixed claim mapper | `service/frontend/fx.go:298`, `599-608` |
| Telemetry injection | `WithCustomMetricsHandler` implementing `metrics.Handler` (counter/gauge/timer/histogram/batch); default built from config if absent | `temporal/server_option.go:230-236`, `common/metrics/metrics.go:15-47`, `temporal/fx.go:210-217` |
| Structured events + tracing | `WithCustomEventLoggerProvider` (OTEL LoggerProvider, no-op default); span exporters merged from config, env vars, and custom exporters | `temporal/server_option.go:246-254`, `temporal/fx.go:219-225`, `966-1009` |
| Interceptor extension | `WithChainedFrontendGrpcInterceptors` / `WithAdditionalStreamInterceptors` appended after the internal chain (deprecation TODO noted) | `temporal/server_option.go:200-221`, `service/frontend/fx.go:286-330` |
| Test hooks | `WithTestHooks` injects synchronization hooks for tests into the DI graph | `temporal/server_option.go:238-244`, `temporal/fx.go:243-246,500-502` |
| Lifecycle: blocking start | `InterruptOn(ch)` makes `Start()` block until signal then auto-Stop; nil channel means block forever; documented single-use only | `temporal/server_option.go:76-82`, `temporal/fx.go:349-365` |
| Lifecycle: ordered shutdown | Services stopped in reverse init order (matching→history→frontend→worker) with 5-minute per-service stop timeout; metrics handler stopped last | `temporal/server_impl.go:44-53,109-124`, `temporal/fx.go:372-379` |
| Startup failure semantics | Per-service start errors aggregated with multierr; fx rolls back started hooks on failure | `temporal/server_impl.go:126-146`, `temporal/fx.go:1250-1263` |
| Signal handling opt-in | OS signal registration only occurs when the host calls `InterruptCh()` — not implicitly by the library | `temporal/interrupt.go:9-21` |
| Error surfacing to host | Errors returned from `Start`/`Stop` as wrapped Go errors; all runtime logs flow through the injected logger (test fails on any unexpected warn/error log) | `cmd/server/main.go:235-242`, `temporal/server_test.go:218-309` |
| Progress/streaming surface | Workflow progress reaches hosts via gRPC long-poll APIs through the frontend service; in-process servers still communicate over loopback gRPC | `temporal/server_test.go:100-156`, `temporaltest/internal/lite_server.go:324-340` |
| Approval/authz surfacing | Denials surface to callers as `PermissionDenied` gRPC status with reason — demonstrated by injecting a deny-all claim mapper in a test | `temporaltest/server_test.go:181-221` |
| Test-server embedding | `temporaltest.NewServer`: ephemeral SQLite, random namespace, free ports, auto-cleanup via `t.Cleanup`; `WithBaseServerOptions` escape hatch (exempted from compat guarantees) | `temporaltest/server.go:128-172`, `temporaltest/options.go:47-51`, `temporaltest/README.md:3-7` |
| CLI embedding consumer | `cmd/server` builds authorizer/claimMapper from config then calls the identical `temporal.NewServer(...)` API used by embedders | `cmd/server/main.go:196-243` |
| Static membership mode | `WithStaticHosts` replaces ringpop-based dynamic membership with a static host map (validated for completeness at construction) | `temporal/server_option.go:67-74`, `temporal/fx.go:289-297` |
| Global side effect | `otel.SetErrorHandler(...)` overwrites the process-global OTEL error handler during start and again on tracer shutdown | `temporal/fx.go:978-985,1088-1091` |

## Answers to Dimension Questions

### 1. Can the harness run inside another application without owning the whole process?

Yes, with caveats. `NewServer` returns immediately after wiring; `Start()` blocks only if the host passes `InterruptOn` (`temporal/server_option.go:76-82`, `temporal/fx.go:351-365`), and signal handlers are registered solely via the opt-in `InterruptCh()` helper (`temporal/interrupt.go:9-21`). The `temporaltest` suite proves non-blocking in-process operation under `go test` (`temporaltest/server.go:163-171`), and multiple servers can coexist in one test process when ports and database names are de-conflicted (`temporal/server_test.go:189-205`, `temporaltest/server_test.go:193-194` uses `t.Parallel()`). Caveats: a server instance cannot be restarted after Stop ("This function should be called only once", `temporal/fx.go:349-351`), a second `Stop()` would panic on double channel close (`temporal/server_impl.go:110` closes `stoppedCh` unconditionally), and embedded startup mutates process-global OTEL state (`temporal/fx.go:981`), so a host that also configures OTEL globally will have its handler replaced.

### 2. Can the host supply policy, tools, identity, storage, telemetry, and secrets?

Policy: yes — pluggable `Authorizer` receives caller claims plus a `CallTarget` with the full API name, namespace, and deserialized request (`common/authorization/authorizer.go:24-56`), wired into the request chain before rate limiting (`service/frontend/fx.go:286-320`). Identity: yes — claim mapper, JWT audience mapper, outbound token provider, and TLS provider are all injectable options (`temporal/server_options.go:49-52,67`, `temporal/server_option.go:106-124,223-228`). Storage: yes — custom data-store and visibility-store factories plus a persistence-service resolver and Elasticsearch HTTP client (`temporal/server_option.go:126-159`), though the data store factory is flagged experimental. Telemetry: yes — metrics handler, OTEL event logger provider, and span exporters (config/env/custom merge precedence documented at `temporal/fx.go:1006-1008`). Tools: not applicable in-process — Temporal's activity/tool model lives in the separate SDK worker processes; the server offers no tool registry hook, which is correct for its role but means "tools" integration happens outside this harness boundary. Secrets: partially — there is no secrets-provider interface; credentials arrive via config YAML templated from environment variables (`common/config/config_template_embedded.yaml:22-23,56-57`) or via `WithConfig` structs, so a host wanting vault-backed secret resolution must pre-render config or inject a custom TLS/datastore factory.

### 3. Are lifecycle, cancellation, shutdown, and error propagation explicit?

Mostly yes. Start/Stop are the entire public lifecycle (`temporal/server.go:16-19`); startup timeouts derive from membership join config (`temporal/server_impl.go:126-131`); services start in dependency order and stop in reverse order with a 5-minute budget (`temporal/server_impl.go:44-53,109-124`); failed starts aggregate per-service errors and fx rolls back already-started hooks (`temporal/server_impl.go:138-145`, `temporal/fx.go:1250-1263`). Gaps: cancellation is coarse — there is no context-parameterized Start/Stop, so a host cannot bound startup/shutdown time itself; restart is unsupported (`temporal/fx.go:349-351`); and double-Stop is a latent panic (`temporal/server_impl.go:110`). Runtime error visibility is log-based through the injected logger rather than event/callback based — acceptable but means hosts must parse logs (or use admin APIs) to observe degradation.

### 4. Does the integration model work for both local-first and service deployments?

Yes — unusually well. The same options API spans ephemeral local embedding (SQLite memory mode, `temporaltest/internal/lite_server.go:80-88`), persistent local files (`lite_server.go:39-46` schema auto-migration), static single-process membership without ringpop (`temporal/server_option.go:67-74`), and full multi-host clusters with dynamic membership, mTLS, and remote-cluster auth (`temporal/fx.go:299-308`). The CLI's `start --config-file` / env-template paths mirror the library paths exactly (`cmd/server/main.go:167-178`). The main asymmetry: production-grade deployment features (schema version verification against Cassandra/SQL at `temporal/fx.go:950-964`, cluster metadata reconciliation at `temporal/fx.go:646-772`) execute automatically inside embedded starts too, so embedding into a desktop-style app pulls in significant operational machinery unless carefully configured down.

## Architectural Decisions

1. **Options-pattern façade over a DI graph.** Hosts see plain functional options (`temporal/server_option.go`), while internally everything is resolved through Uber Fx graphs; the bridge materializes options into `ServiceProviderParamsCommon` and re-provides them per service graph, explicitly acknowledged as a workaround ("This is not ideal...", `temporal/fx.go:418-425`).
2. **CLI as proof of embedding.** The official binary consumes the same `NewServer` API as external embedders (`cmd/server/main.go:222-234`), guaranteeing the library path cannot rot.
3. **Interface-first extension points.** Authorizer, ClaimMapper, metrics.Handler, AbstractDataStoreFactory, VisibilityStoreFactory, FactoryProvider, and archiver factories are small Go interfaces with mockgen mocks checked in (`common/authorization/authorizer_mock.go`, `common/metrics/metrics_mock.go:20`, `client/client_factory_mock.go:174-199`).
4. **Network API as the stable contract.** Even same-process clients dial gRPC loopback (`temporaltest/internal/lite_server.go:329-332`); this keeps the embedding API small and pushes behavioral compatibility onto versioned protos instead of Go internals.
5. **Graceful-degradation defaults.** Every optional dependency has a built-from-config or no-op fallback: noop dynamic-config client (`temporal/fx.go:236-240`), no-op event logger (`temporal/fx.go:222-225`), noop authorizer gated behind an `--allow-no-auth` acknowledgment flag (`cmd/server/main.go:197-209`).

## Notable Patterns

- **Validation-at-construction with actionable messages**: misconfiguration such as `TokenProvider` without remote-cluster TLS fails fast at `NewServerFx` time with instructions naming the fix (`temporal/fx.go:299-308`).
- **Log-contract testing**: `errorLogDetector` wraps the injected logger and fails the test on any unexpected warn/error during a full start→workload→stop cycle (`temporal/server_test.go:75-98,301-309`) — an executable specification of the logging surface hosts inherit.
- **Test-only seams promoted to options**: `testhooks.TestHooks` is plumbed through the production DI graph behind a "tests only" option (`temporal/server_option.go:238-244`), avoiding a parallel test build path.
- **Tiered compatibility promises**: `temporaltest` documents that `WithBaseServerOptions` may break in any release while the rest of the package follows semver (`temporaltest/README.md:3-7`).
- **Exporter precedence merging**: span exporters resolve config < env < custom-code, giving embedders deterministic override power (`temporal/fx.go:987-1009`).

## Tradeoffs

- **Small stable surface vs. deep configurability**: anything not exposed as a `ServerOption` requires reaching into fx modules (documented override types for tracing at `temporal/fx.go:1024-1040`), which couples embedders to internals.
- **gRPC-loopback integration vs. in-proc calls**: uniform and compat-safe, but adds serialization/port-management overhead and motivates hacks like the 100ms ringpop sleep (`temporaltest/server.go:168-169`).
- **No-op defaults vs. silent misconfiguration**: discarding structured events by default keeps embedding cheap (`temporal/fx.go:222-225`) but observability must be consciously added.
- **Experimental labels on core storage hooks** (`temporal/server_option.go:148,186`) keep flexibility honest but deter production embedders who need stable custom-storage support.
- **Auto-migrations in LiteServer** (`temporaltest/internal/lite_server.go:226-239`) trade convenience for hidden DDL execution inside a host process.

## Failure Modes / Edge Cases

- **Double Stop panic**: `close(s.stoppedCh)` runs unconditionally (`temporal/server_impl.go:110`); a second `Stop()` (e.g., host cleanup racing defer) panics the host process.
- **No restart support**: fx apps are single-shot; hosts needing cycling must construct fresh instances, re-picking ports/namespaces (`temporal/fx.go:349-351`).
- **Process-global telemetry clobbering**: both success-path start and tracer-shutdown replace `otel.SetErrorHandler`, silently overriding a host's own handler (`temporal/fx.go:981,1089`).
- **Prometheus reporter leak**: the default prometheus handler "does not shut down in-between test runs," forcing port randomization in tests (`temporal/server_test.go:190-194`) — a real constraint when embedding multiple servers.
- **Startup race workaround**: fixed 100ms sleep to avoid a ringpop label panic reveals timing sensitivity in embedded single-process membership (`temporaltest/server.go:168-169`).
- **Persisted-state overrides config silently**: mismatched cluster metadata (shard count, failover increment) logs a warning and adopts persisted values (`temporal/fx.go:873-904`) — surprising for embedders treating config as authoritative.
- **Transient noise is expected**: the log-contract test whitelists known transient warn/errors during start/stop (`temporal/server_test.go:264-298`), meaning naive "any error log = broken" monitoring will false-positive.

## Future Considerations

- Stabilize or wrap the experimental storage/client-factory options behind a supported SPI, since they are the linchpin for hosting Temporal atop another platform's storage (`temporal/server_option.go:147-153,177-191`).
- Add context-aware `Start(ctx)`/idempotent `Stop` semantics to remove the double-close hazard and give hosts shutdown budget control (`temporal/server.go:16-19`, `temporal/server_impl.go:109-124`).
- Scope OTEL global mutation behind an opt-in so embedded servers coexist with host telemetry (`temporal/fx.go:978-991`).
- Provide a first-class secrets-provider hook analogous to `metrics.Handler` rather than env-var templating (`common/config/config_template_embedded.yaml:22-23`).
- Replace the interceptor-append model with a named insertion-point API; the in-chain TODO already concedes the current design limits ordering control (`service/frontend/fx.go:316-318`).
- Promote `LiteServer`'s refactor TODO into typed `ServerOption`s so the test-server conveniences (SQLite, namespaces, migrations) become generally embeddable (`temporaltest/internal/lite_server.go:4`).

## Questions / Gaps

- No evidence found of an officially documented embedding guide inside this repo (README targets Docker/compose operators); the embedding story is discoverable only from package docs, option comments, and tests. Searched `README.md`, `docs/`, and package doc comments.
- No evidence found for in-process approval/workflow-signal callbacks to the embedding host beyond gRPC APIs and interceptors; approvals (authz denials, update validations) are surfaced as RPC errors, and I found no push/event callback interface for host UX. Searched `temporal/server_option.go`, `service/frontend/fx.go`, and `common/authorization/`.
- Whether Temporal Cloud (hosted-service mode) shares this embedding code could not be verified from this repository — no evidence found in-tree; searched `docs/` and top-level README.
- Resource ownership boundaries between host and server (e.g., who closes the ES HTTP client passed via `WithElasticsearchHttpClient`) are not specified anywhere I could find; `metricHandler.Stop` is called by the server (`temporal/server_impl.go:120-122`) but no equivalent contract is stated for injected clients. Searched `temporal/` and `common/rpc/encryption`.

---

Generated by `dimensions/24.04-embedding-and-host-integration-ergonomics.md` against `temporal`.

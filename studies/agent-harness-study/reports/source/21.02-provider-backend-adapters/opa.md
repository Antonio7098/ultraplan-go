# Source Analysis: opa

## Dimension 21.02: Provider and Backend Adapters

### Source Info

| Field | Value |
|-------|-------|
| Name | opa |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | Go (Open Policy Agent); BadgerDB embedded KV for disk store; wazero WASM runtime; OpenTelemetry SDK |
| Analyzed | 2026-08-25 |

## Summary

OPA is organized around a small set of explicit Go interfaces that abstract its external backends, each with at least two concrete implementations and a config-driven selection path. The most important abstraction is `storage.Store` (`v1/storage/interface.go:20-44`), implemented by an in-memory store (`v1/storage/inmem/inmem.go`) and a Badger-backed disk store (`v1/storage/disk/disk.go`); the runtime picks between them at startup from the `storage.disk` config key or a registered custom builder (`v1/runtime/runtime.go:490-519`). Outbound HTTP backends are uniformly routed through `rest.Client` with a pluggable `HTTPAuthPlugin` chain selected by reflection over the config's `credentials` block plus named custom plugins (`v1/plugins/rest/rest.go:42-51`, `rest.go:88-120`). Decision-log sinks are swappable three ways (console / service / external plugin) behind the `logs.Logger` interface (`v1/plugins/logs/plugin.go:39-43`, `plugin.go:790-815`). Bundle sources swap between HTTP services and OCI registries behind the bundle plugin's `Loader` interface (`v1/plugins/bundle/plugin.go:50`, selection at `plugin.go:456`). Distributed tracing is injected through a package-level registry (`v1/tracing/tracing.go:22-35`) whose default implementation is registered by an optional feature import (`v1/features/tracing/tracing.go:14-16`) and whose OTLP transport (grpc vs http) is config-selected (`internal/distributedtracing/distributedtracing.go:129-140`). Third-party extensibility is first-class via the `plugins.Factory` contract and `runtime.RegisterPlugin` (`v1/plugins/plugins.go:89-92`, `v1/runtime/runtime.go:95`). The main gaps: the store cannot be swapped without a restart, several registries are single-slot globals rather than maps, and the inter-query cache has only one in-memory implementation.

## Rating

**8 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale: nearly every external dependency (store, auth mechanism, decision-log destination, bundle source, tracing transport, signing/verification) sits behind a named Go interface with multiple shipped implementations, is selected from externalized JSON configuration, and is covered by dedicated unit/integration tests (e.g., `v1/plugins/rest/auth_test.go:13-371`, `v1/storage/disk/disk_test.go`). It falls short of 9-10 because store backends cannot be exchanged at runtime (restart required, `v1/runtime/runtime.go:505-519`), some registries silently overwrite on double registration (`v1/tracing/tracing.go:33-35`; last-writer-wins global slot at `v1/runtime/runtime.go:71-72`), and there is no external-cache backend for the inter-query builtin cache.

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Store abstraction | `Store` interface (NewTransaction/Read/Write/Commit/Truncate/Abort) plus optional `MakeDirer`, `NonEmptyer`, `Closer` capability interfaces | `v1/storage/interface.go:20-63` |
| Store capability defaults | `WritesNotSupported`, `PolicyNotSupported`, `TriggersNotSupported` embeddable stubs let partial backends satisfy the big interface | `v1/storage/interface.go:144-148`, `160-180`, `240-245` |
| Store impl #1 | In-memory store with behavioral options `OptRoundTripOnWrite`, `OptReturnASTValuesOnRead` | `v1/storage/inmem/inmem.go`; `v1/storage/inmem/opts.go:21-33` |
| Store impl #2 | Disk (Badger) store with partitions | `v1/storage/disk/disk.go`; partitions in `v1/storage/disk/config.go:21` |
| Store test double | Mock store built on inmem options for consumer tests | `internal/storage/mock/mock.go:73-79` |
| Runtime store selection | Switch: disk when configured → registered custom backend → default inmem | `v1/runtime/runtime.go:490-519` |
| Custom storage registration | Global `registeredStorageBackend` slot + `RegisterStorageBackend(builder)`; `Params.StoreBuilder` injection point | `v1/runtime/runtime.go:71-72`, `104-108`, `275-276`, `498-503` |
| Disk config externalization | `OptionsFromConfig` reads `storage.disk` (`directory`, `auto_create`, `partitions`, `badger` keys) from OPA config | `v1/storage/disk/config.go:29-67` |
| Plugin factory SPI | `Factory{Validate, New}` invoked per config key; documented `plugins: <name>` config mapping | `v1/plugins/plugins.go:89-92`, `42-66` |
| Plugin lifecycle | `Plugin{Start, Stop, Reconfigure}`; note "Currently OPA will not call Stop on plugins" caveat | `v1/plugins/plugins.go:106-110`, comment at `101` |
| Third-party plugin registration | `runtime.RegisterPlugin(name, factory)` | `v1/runtime/runtime.go:95` |
| Decision-log sink SPI | `Logger` interface embedding `plugins.Plugin` + `Log(ctx, EventV1)` | `v1/plugins/logs/plugin.go:39-43` |
| Sink selection | Console (`config.ConsoleLogs`), service push (`config.Service`), or named plugin (`config.Plugin` dispatching to `Logger` or `plugins.LoggerPlugin`) | `v1/plugins/logs/plugin.go:311-313`, `790-815` |
| slog-based logger plugin | `LoggerPlugin{Plugin; Logger() slog.Handler}` alternative sink contract | `v1/plugins/plugins.go:117-123` |
| Auth plugin abstraction | `HTTPAuthPlugin{NewClient(Config), Prepare(*http.Request)}` with strict one-time/per-request split | `v1/plugins/rest/rest.go:42-51` |
| Auth implementations | bearer (`bearerAuthPlugin`), OAuth2 client credentials, client TLS, AWS S3 signing, GCP metadata, Azure managed identity, default anonymous | `v1/plugins/rest/auth.go:47`, `62`, `236`, `712`, `964`; `v1/plugins/rest/gcp.go`; `v1/plugins/rest/auth_tls.go` |
| Auth selection mechanism | Reflection over `Config.Credentials` fields ("a maximum one credential method must be specified") or named lookup via `Credentials.Plugin` | `v1/plugins/rest/rest.go:61-69`, `88-120` |
| Custom auth plugin resolution | `Manager.AuthPlugin(name)` resolves registered plugins into rest clients via `AuthPluginLookupFunc` | `v1/plugins/plugins.go:740`; `v1/plugins/rest/rest.go:85`, `152-158` |
| AWS provider internals | ECR/KMS signing providers under internal package consumed by aws auth plugin & OCI downloads | `internal/providers/aws/ecr.go`, `kms.go` |
| Bundle source swap | `Loader` interface abstracts downloaders; OCI branch chosen when service/client type == `oci` | `v1/plugins/bundle/plugin.go:50`, `66`, `456-461` |
| Downloaders | HTTP `Downloader` (`download.New`) vs `OCIDownloader` (`download.NewOCI`, with build-tag fallback stub `oci_download_unavailable.go:13`) | `v1/download/download.go:83`; `v1/download/oci_download.go:36` |
| Tracing injection point | `HTTPTracingService` interface + package-global registry; nil-safe no-op when unregistered | `v1/tracing/tracing.go:22-55` |
| Tracing feature wiring | Optional import `features/tracing` registers otelhttp-backed factory in `init()` | `v1/features/tracing/tracing.go:14-16`, `20-26` |
| Trace export transport | Config `distributed_tracing.type` selects otlptracegrpc vs otlptracehttp exporters | `internal/distributedtracing/distributedtracing.go:98`, `109`, `129-140`, validation at `227-235` |
| Metrics export | Separate `internal_metrics.Init` meter provider alongside tracer provider | `v1/runtime/runtime.go:526-529` |
| Inter-query cache | `InterQueryCache` interface (Get/Insert/Delete/UpdateConfig) with single in-memory factory `newCache` | `v1/topdown/cache/cache.go:250-257`, `264-266` |
| Cache reconfiguration propagation | `Manager.RegisterCacheTrigger` pushes new cache config to live caches on discovery updates | `v1/plugins/plugins.go:1331-1333` |
| Bundle signer/verifier registries | `Signer`/`Verifier` interfaces; id-keyed maps with reserved `_default` id and `RegisterSigner`/`RegisterVerifier` | `v1/bundle/sign.go:18-21`, `109-127`, init map at `131-135`; `v1/bundle/verify.go:265-274` |
| Logger abstraction | `logging.Logger` interface (logrus-backed `StandardLogger`) injectable into runtime/SDK | `v1/logging/logging.go:30-40`, `60-70` |
| Embedded SDK injection points | `sdk.Options{Logger, ConsoleLogger, Store}`; defaults: buffered logger and `inmem.New()` | `v1/sdk/options.go:44-52`, `68-70`, `137-138` |
| Runtime hot-reconfig | `Manager.Reconfigure(newCfg)` + per-plugin `Reconfigure`; discovery plugin drives it (its own Reconfigure is a no-op) | `v1/plugins/plugins.go:980`; `v1/plugins/discovery/discovery.go:204` |
| Adapter tests (auth) | e.g., `TestOCIWithAWSAuthSetsUpECRAuthPlugin`, `TestOauth2WithAWSKMS`, `TestOauthWithAzureKV`, bearer header attachment | `v1/plugins/rest/auth_test.go:13`, `98`, `142`, `371` |
| Adapter tests (logs) | ~48 Test funcs incl. buffer-type behavior and plugin-sink integration | `v1/plugins/logs/plugin_test.go`; `v1/plugins/logs/logger_plugin_integration_test.go` |
| Adapter tests (store) | disk store suite (12 tests), inmem suite + examples, shared storage conformance tests | `v1/storage/disk/disk_test.go`; `v1/storage/inmem/inmem_test.go`; `v1/storage/storage_test.go` |
| Backend-swap benchmarks | Build-tag pair `bench_disk`/`!bench_disk` swaps disk vs inmem store in same e2e benchmark | `v1/test/e2e/authz/disk.go:11-15`; `v1/test/e2e/authz/nodisk.go:8-10` |

## Answers to Dimension Questions

**1. Are backends swappable?**
Yes, at startup, for every major backend class. Storage swaps between in-memory and Badger-disk purely via the `storage.disk` config block (`v1/runtime/runtime.go:490-519`, key parsed at `v1/storage/disk/config.go:29-40`), or via a custom builder supplied programmatically (`Params.StoreBuilder`, `v1/runtime/runtime.go:275-276`) or registered globally (`RegisterStorageBackend`, `v1/runtime/runtime.go:104-108`). The embedded SDK exposes `Store` as an option (`v1/sdk/options.go:68-70`). Outbound service connections (bundle/discovery/status/decision-log uploads) all go through `rest.New` where the credential backend is chosen from config (`v1/plugins/rest/rest.go:88-120`). Bundle sources swap between plain HTTPS services and OCI registries by setting the service `type: oci` (`v1/plugins/bundle/plugin.go:456-461`). Answering the dimension's headline question directly: switching the equivalent of Postgres→SQLite (disk→inmem store) **is** a pure config change (`storage.disk` present vs absent).

**2. Which backends have multiple implementations?**
- Storage: inmem, disk/Badger, mock-for-tests, plus user-registered builders (`v1/storage/inmem/`, `v1/storage/disk/disk.go`, `internal/storage/mock/mock.go:73`).
- REST auth: seven built-in plugin types + custom named plugins (`v1/plugins/rest/rest.go:61-69`).
- Decision-log sinks: console logger, remote service, third-party plugin (two contracts: `logs.Logger` and slog `plugins.LoggerPlugin`) (`v1/plugins/logs/plugin.go:790-815`).
- Bundle downloaders: HTTP vs OCI (`v1/download/download.go:83`, `v1/download/oci_download.go:36`).
- Tracing export transports: OTLP gRPC vs HTTP (`internal/distributedtracing/distributedtracing.go:129-140`).
- Bundle signers/verifiers: default implementations plus registry-registered alternatives (`v1/bundle/sign.go:118-127`).
Single-implementation abstractions (interface exists but only one real backend): `InterQueryCache` (`v1/topdown/cache/cache.go:264-266` — in-memory only; no Redis/external driver found anywhere in the tree), and the WASM resolver runtime which is fixed to wazero (`internal/wasm/sdk/internal/wasm/vm.go:16-17`).

**3. Can backends be swapped at runtime?**
Partially. The store is bound once during `runtime.New` and never replaced; there is no live store migration (selection code runs only at startup, `v1/runtime/runtime.go:505-519`). What *is* hot-swappable is configuration-driven behavior of plugins: discovery delivers new config and calls `Manager.Reconfigure` then each `Plugin.Reconfigure` (`v1/plugins/plugins.go:980`, `109`), so e.g. the decision-log plugin can change buffer/sink parameters mid-flight (`Reconfigure` at `v1/plugins/logs/plugin.go:856`) and inter-query caches receive updated limits via cache triggers (`v1/plugins/plugins.go:1331-1333`). Tracing transport (grpc↔http) requires restart since exporters are initialized once (`internal/distributedtracing/distributedtracing.go:98`). The `HTTPTracingService` itself is a process-lifetime singleton set by import side effects (`v1/tracing/tracing.go:30-35`) — composition-time, not runtime, selection.

**4. Are adapter implementations tested?**
Yes, substantially. Auth adapters have a dedicated suite covering AWS ECR/KMS, Azure Key Vault, OAuth2 client assertions, and bearer behavior (`v1/plugins/rest/auth_test.go:13-371`); the logs plugin has ~48 tests plus integration tests for both sink contracts (`v1/plugins/logs/plugin_test.go`, `logger_plugin_integration_test.go`); both stores ship full suites (`v1/storage/disk/disk_test.go`, `v1/storage/inmem/inmem_test.go`, shared conformance in `v1/storage/storage_test.go`), and consumers are tested against a mock store (`internal/storage/mock/mock.go:73`). A build-tagged e2e pair runs identical benchmarks against disk vs inmem stores, demonstrating interchangeability in practice (`v1/test/e2e/authz/disk.go:11`, `nodisk.go:8`). Gap: no evidence found of automated conformance tests forcing *custom* third-party `storage.Store` implementations through the full server stack beyond the mock usage; the contract relies on the interface doc comments (`v1/storage/interface.go:19-44`).

## Architectural Decisions

1. **One wide `Store` interface + opt-in capability interfaces.** Rather than many small interfaces, OPA defines a broad `Store` and lets backends embed `WritesNotSupported`/`PolicyNotSupported`/`TriggersNotSupported` stubs for unsupported facets, with optional `Closer` for shutdown hooks (`v1/storage/interface.go:20-63`, `144-180`, `240-245`). This keeps the engine's single storage dependency stable while making partial backends expressible.
2. **Precedence-chain factory for the store.** Explicit `StoreBuilder` param overrides registered global backend overrides built-in disk/inmem defaults (`v1/runtime/runtime.go:498-519`) — programmatic injection wins over global registration wins over config-derived defaults.
3. **Reflection-based auth selection.** `rest.Config.Credentials` is a struct of nullable plugin instances; `AuthPlugin()` scans fields via reflect, enforcing exactly-one credential method, avoiding per-plugin if/else churn as new auth methods are added (`v1/plugins/rest/rest.go:103-115`).
4. **Feature-package composition for tracing.** Core packages depend only on the tiny `tracing.Options`/`HTTPTracingService` seam; the OpenTelemetry dependency is pulled in by importing `features/tracing`, keeping otel out of builds that don't want it (`v1/tracing/tracing.go:5-9`, `v1/features/tracing/tracing.go:14-16`).
5. **Registries keyed by id for crypto operations.** Signer/verifier lookup uses string-id maps with a reserved `_default`, letting bundles name their signing plugin in config (`v1/bundle/sign.go:24`, `109-127`).
6. **Plugins as the universal extension vehicle.** Custom sinks, custom auth, and even custom storage all funnel through the manager/factory machinery (`plugins.Factory` at `v1/plugins/plugins.go:89-92`; `Manager.AuthPlugin` at `740`; `RegisterStorageBackend` at `v1/runtime/runtime.go:104`).

## Notable Patterns

- **Registry pattern (global var + Register function):** `RegisterHTTPTracing` (`v1/tracing/tracing.go:33-35`), `RegisterSigner`/`GetSigner` (`v1/bundle/sign.go:109-127`), `RegisterVerifier` (`v1/bundle/verify.go:274`), `RegisterPlugin` (`v1/runtime/runtime.go:95`), `RegisterStorageBackend` (`v1/runtime/runtime.go:104-108`).
- **Nil-safe decorator/no-op fallback:** `tracing.NewTransport/NewHandler` pass through untouched when no tracing service is registered (`v1/tracing/tracing.go:40-55`); `rest.Config.AuthPlugin` returns `&defaultAuthPlugin{}` when no credentials configured (`v1/plugins/rest/rest.go:117-119`).
- **Build-tag variant pairs:** OCI downloader has a compile-time stub for builds without OCI support (`v1/download/oci_download.go:36` vs `oci_download_unavailable.go:13`); e2e store benchmarks toggle backends via tags (`v1/test/e2e/authz/disk.go:11`).
- **Option structs for behavioral tuning without new types:** inmem `OptRoundTripOnWrite`/`OptReturnASTValuesOnRead` trade CPU vs memory within one backend (`v1/storage/inmem/opts.go:21-33`).
- **Config-first construction:** every adapter receives a parsed config struct with `validateAndInjectDefaults` semantics (e.g., `v1/plugins/logs/plugin.go:584-602`, `v1/keys/keys.go:42-56`), so defaults live beside validation.
- **Test doubles shipped in-tree:** `internal/storage/mock` records expected Read/Write calls (`internal/storage/mock/mock.go:57-79`) instead of leaving mocks to consumers.

## Tradeoffs

- **Wide `Store` interface lowers the floor for new backends but raises accidental complexity:** implementors must reason about transactions, triggers, policies, and `Truncate` copy-and-swap semantics even when delegating most of it to stubs (`v1/storage/interface.go:33-44`).
- **Global single-slot registries are simple but lossy:** only one storage backend and one tracing service can ever be active; a second registration silently replaces the first (`v1/runtime/runtime.go:71-72`; `v1/tracing/tracing.go:30-35`), unlike the id-keyed signer/verifier maps which allow many.
- **Reflection-based auth selection trades explicitness for low maintenance cost:** adding a credential type needs zero changes to selection logic, but errors surface only at client construction time and field order determines ambiguity diagnostics (`v1/plugins/rest/rest.go:103-115`).
- **Startup-only store binding avoids live-migration bugs at the cost of restart-required backend changes** (`v1/runtime/runtime.go:505-519`).
- **Import-side-effect feature wiring keeps dependencies lean but makes presence invisible in config:** whether tracing is compiled in depends on the binary's import graph, not on any config file (`v1/features/tracing/tracing.go:14-16`).

## Failure Modes / Edge Cases

- **Duplicate/misconfigured auth credentials fail fast with a clear error** ("a maximum one credential method must be specified", unknown plugin name) at `Client` creation (`v1/plugins/rest/rest.go:110-111`, `96-99`) — good failure locality.
- **Unknown tracing transport rejected at config validation**, not at first span: `distributed_tracing.type` must be grpc/http/unset (`internal/distributedtracing/distributedtracing.go:227-235`).
- **Missing signer id produces a descriptive error**: `no signer exists under id %s` (`v1/bundle/sign.go:112-114`), and the reserved default id cannot be shadowed (`sign.go:121-123`).
- **Decision-log plugin-name resolution failure surfaces per-event**, returning `plugin %q not found` inside `Log` (`v1/plugins/logs/plugin.go:803-806`) — a mis-pointed `decision_logs.plugin` config degrades logging at runtime rather than at load time.
- **OCI unavailable builds fail at download time** through the stubbed downloader (`v1/download/oci_download_unavailable.go:13`), so a config referencing `type: oci` against a non-OCI binary fails operationally rather than statically.
- **Disk store directory problems abort startup** unless `auto_create` is set (`v1/storage/disk/config.go:48-54`).
- **Plugin lifecycle gap:** the framework documents that `Stop` may never be called ("Currently OPA will not call Stop on plugins", `v1/plugins/plugins.go:101`), so backend adapters relying on Stop for flush/close need their own safeguards (the store's `Closer` is honored separately on shutdown, `v1/storage/interface.go:57-63`).

## Future Considerations

- Introduce a conformance harness that third-party `storage.Store` authors can run against the exact behaviors the server/topdown rely on (today the implicit contract is spread across `v1/storage/storage_test.go` and doc comments).
- Convert the single-slot `registeredStorageBackend` and `tracing` globals to id-keyed registries (matching the signer/verifier design) to allow coexisting candidate backends and detect conflicts explicitly.
- An `InterQueryCache` external-backend implementation would complete the caching story; the existing `UpdateConfig` method on the interface (`v1/topdown/cache/cache.go:255`) shows the reconfiguration plumbing already anticipates alternate drivers.
- Surface compiled-in feature packages (e.g., tracing, OCI) in runtime info so operators can distinguish "feature disabled" from "feature absent" failures.
- Honor `Plugin.Stop` or document per-plugin teardown expectations more strongly, since sink adapters buffering events (event buffers in `v1/plugins/logs/eventBuffer.go`) depend on shutdown flushing.

## Questions / Gaps

- No evidence found of a vector-database or message-queue abstraction; OPA's domain simply lacks these backend classes, and searches for redis/external cache drivers returned nothing outside unrelated matches. Search boundary: whole-tree grep across the selected source for `redis`, `queue`, `vector`.
- Whether any officially maintained out-of-tree storage backends exist could not be verified from this source alone; in-tree evidence supports the mechanism (`RegisterStorageBackend`, `v1/runtime/runtime.go:101-108`) but not ecosystem adoption.
- The exact guarantee ordering between `Commit` and trigger callbacks is documented only in a doc comment (`v1/storage/interface.go:228-230`); no test specifically asserting cross-backend ordering was located.

---

Generated by dimension `21.02-provider-and-backend-adapters` against `opa`.

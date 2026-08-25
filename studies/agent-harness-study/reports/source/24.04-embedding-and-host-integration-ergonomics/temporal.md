# Source Analysis: temporal

## Dimension 24.04: Embedding and Host Integration Ergonomics

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (gRPC services, uber-go/fx dependency injection, OpenTelemetry, Zap logging) |
| Analyzed | 2026-08-24 |

## Summary

Temporal is a durable-execution/workflow platform rather than an LLM agent harness, so its "harness" is a set of long-running gRPC services. Its embedding story is nonetheless unusually deliberate and has three tiers:

1. **In-process library mode** — `temporal.NewServer(opts...)` builds a server object with just `Start()`/`Stop()` (`temporal/server.go:16-19`, `temporal/server.go:44-46`) that a host process can own. The shipped CLI is itself just another embedder: `cmd/server/main.go:222-234` constructs the identical `NewServer(...)` object and calls `Start()`.
2. **Test-first embedding** — the `temporaltest` package wraps the same library behind `TestServer` with auto-created namespaces, clients, workers, and `t.Cleanup` teardown (`temporaltest/server.go:132-171`, `temporaltest/server.go:144-146`), backed by an ephemeral SQLite `LiteServer` (`temporaltest/internal/lite_server.go:203-302`).
3. **Deep extension via interfaces and fx overrides** — hosts supply storage, policy, identity, telemetry, and secrets through ~25 documented functional options (`temporal/server_option.go:37-255`) or by replacing providers inside the exported `TopLevelModule` fx graph (`temporal/fx.go:140-160`), which accepts a caller-supplied top-level module (`temporal/fx.go:163-175`).

Dependency injection is real, not cosmetic: `ServerOptionsProvider` resolves every injectable dependency with explicit defaults (`temporal/fx.go:177-347`), and construction fails fast on invalid combinations (e.g., a token provider without remote-cluster TLS: `temporal/fx.go:306-308`). The main ergonomic weaknesses are a one-shot, non-context-aware lifecycle (`Start()` may be called only once — `temporal/fx.go:349-351`; blocking start waits on a signal channel, not a context — `temporal/fx.go:357-362`), a few process-global side effects (OTEL error handler: `temporal/fx.go:981`; pprof listener never stopped: `common/pprof/fx.go:21-28`), "experimental" disclaimers on several important extension options, and essentially zero in-repo embedding documentation (docs cover internal architecture only; no embedding guide found under `docs/`).

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards. The option surface is broad and consistently implemented, defaults are resolved in one auditable place, lifecycle order is deterministic, and embedding behavior is exercised by dedicated tests (`temporal/server_test.go:42-157`, `temporaltest/server_test.go:31-260`). It falls short of 8–9 because: (a) `Start()` is single-use and shutdown cannot be driven by a host `context.Context`; (b) key extension points (`WithCustomDataStoreFactory` at `temporal/server_option.go:148-149`, `WithClientFactoryProvider` at `temporal/server_option.go:177-179`, stream interceptors at `temporal/server_option.go:212-217`, event logger at `temporal/server_option.go:246-251`) carry "experimental, may change or be removed" caveats; (c) a failed multi-service start leaves earlier-started services running unless the host calls `Stop()` (`temporal/server_impl.go:138-145`); and (d) embedding guidance lives only in code comments, not documentation.

## Evidence Collected

Every entry cites file path with line numbers (relative to the source root).

| Area | Evidence | File:Line |
|------|----------|-----------|
| Library entry point | `Server` interface is exactly `Start() error` / `Stop() error`; `NewServer(opts...)` constructor | `temporal/server.go:15-20`, `temporal/server.go:43-46` |
| Service selection | `Services` / `DefaultServices` lists; `ForServices(names)` option to run any subset in one process | `temporal/server.go:25-40`, `temporal/server_option.go:57-65` |
| Configuration injection | `WithConfig(*config.Config)` programmatic config; `WithConfigLoader(dir, env, zone)`; `WithServerConfigFilePath`; mutually exclusive validation | `temporal/server_option.go:36-55`, `temporal/server_options.go:106-119` |
| Config loading modes | Embedded-template-with-env-vars (`WithEmbedded`), single-file, and hierarchical dir loading | `common/config/loader.go:111-117`, `common/config/loader.go:119-158`, `common/config/loader.go:178-215` |
| DI graph | `TopLevelModule` fx options exported; `NewServerFx` accepts a caller-supplied top-level module | `temporal/fx.go:139-161`, `temporal/fx.go:163-175` |
| Dependency resolution | `serverOptionsProvider` struct enumerates all host-suppliable types; `ServerOptionsProvider` applies defaults when host omits them | `temporal/fx.go:98-136`, `temporal/fx.go:177-347` |
| Host-provided policy | `WithAuthorizer` / `WithClaimMapper` / `WithAudienceGetter`; `Authorizer.Authorize(ctx, claims, target)` and `ClaimMapper.GetClaims` interfaces | `temporal/server_option.go:98-124`, `common/authorization/authorizer.go:52-58`, `common/authorization/claim_mapper.go:27-33` |
| Host-provided storage | `WithCustomDataStoreFactory` implementing `AbstractDataStoreFactory`; plus visibility store and history/visibility archiver factories | `temporal/server_option.go:147-175`, `common/persistence/client/abstract_data_store_factory.go:12-26` |
| Host-provided telemetry | `WithCustomMetricsHandler` (`metrics.Handler` incl. `Stop(logger)`), OTEL span exporters mergeable from config/env/code, `WithCustomEventLoggerProvider` | `temporal/server_option.go:230-236`, `common/metrics/metrics.go:15-47`, `temporal/fx.go:978-1022`, `temporal/server_option.go:246-255` |
| Host-provided identity/secrets | `WithTLSConfigFactory`, `WithTokenProvider` (outbound remote-cluster auth), fail-fast check that token provider requires TLS config | `temporal/server_option.go:105-110`, `temporal/server_option.go:223-228`, `common/rpc/auth/token_provider.go:13`, `temporal/fx.go:299-308` |
| Host-provided dynamic config | `WithDynamicConfigClient` accepting `dynamicconfig.Client` interface (in-memory lookup contract documented) | `temporal/server_option.go:140-145`, `common/dynamicconfig/client.go:10-32` |
| Inter-service client seam | `client.FactoryProvider` interface injectable to customize RPC client creation | `client/clientfactory.go:42-54`, `temporal/server_option.go:177-183` |
| Frontend API interception | `WithChainedFrontendGrpcInterceptors` and `WithAdditionalStreamInterceptors`; custom unary interceptors appended innermost, before the retry interceptor | `temporal/server_option.go:200-221`, `service/frontend/fx.go:315-321`, `service/frontend/fx.go:323-328` |
| Lifecycle management | Ordered multi-service start (`initOrder`: matching→history→frontend→worker) and reverse-order stop | `temporal/server_impl.go:44-53`, `temporal/server_impl.go:126-146`, `temporal/server_impl.go:109-124` |
| Startup/shutdown timeouts | Fixed `serviceStartTimeout=15s` (doubled against membership max join) and per-service `serviceStopTimeout=5m` | `temporal/server.go:11-12`, `temporal/server_impl.go:127-131`, `temporal/fx.go:372-379` |
| Cancellation semantics | Blocking-start via `InterruptOn(<-chan any)`; `InterruptCh()` wraps OS signals; nil channel blocks forever (documented) | `temporal/server_option.go:76-82`, `temporal/interrupt.go:9-21` |
| Graceful shutdown | Frontend `Stop()`: fail health check → mark draining → wait failure-detection → stop handlers → drain traffic → close listeners | `service/frontend/service.go:544-575` |
| Error surfacing to host | Construction-time validation returns errors (`loadAndValidate`); start failures aggregate per service via `multierr`; per-service stop errors are logged, not returned | `temporal/server_options.go:84-104`, `temporal/server_impl.go:138-145`, `temporal/fx.go:372-379` |
| Client-facing error mapping | Mask-error interceptor outermost, then service-error interceptor, in the frontend unary chain; shared services chain appends host stream interceptors after internal ones | `service/frontend/fx.go:286-292`, `service/fx.go:144-156`, `service/fx.go:159-180` |
| Test-server SDK | `temporaltest.NewServer(WithT(t))` auto-registers cleanup, pre-registers namespace, manages clients/workers lifetime | `temporaltest/server.go:132-171`, `temporaltest/options.go:19-52`, `temporaltest/server.go:113-126` |
| Local-first SQLite server | `LiteServer` builds full `config.Config` programmatically (ports, cluster metadata, archival disabled) and wraps `temporal.Server` | `temporaltest/internal/lite_server.go:74-171`, `temporaltest/internal/lite_server.go:269-302` |
| In-process clients | `LiteServer.NewClientWithOptions` dials loopback frontend; `TestServer` tracks and closes created clients on `Stop()` | `temporaltest/internal/lite_server.go:329-332`, `temporaltest/server.go:92-111` |
| Embedding tests | Full end-to-end embed test with custom logger/interceptor/dynamic-config; authz denial test using custom claim mapper; custom SDK client interceptor test | `temporal/server_test.go:71-157`, `temporal/server_test.go:85-91`, `temporaltest/server_test.go:193-221`, `temporaltest/server_test.go:223-260` |
| Global state caveats | `otel.SetErrorHandler` mutated at start and again on trace-provider stop; test acknowledges Prometheus reporter does not shut down between runs | `temporal/fx.go:981`, `temporal/fx.go:1087-1092`, `temporal/server_test.go:190-195` |
| Background work disclosure | Dynamic-config file client refresh goroutine tied to server-owned `stopChan`; pprof listener started in lifecycle with commented-out OnStop | `temporal/fx.go:230-232`, `common/pprof/fx.go:19-29` |

## Answers to Dimension Questions

### 1. Can the harness run inside another application without owning the whole process?

**Yes.** `temporal.NewServer(opts...)` returns an ordinary object whose non-blocking `Start()` returns control to the host (`temporal/fx.go:351-365`); blocking-until-interrupt is strictly opt-in via `InterruptOn` (`temporal/server_option.go:76-82`). Multiple servers can coexist in one process — `temporaltest` starts parallel per-test servers with `t.Parallel()` (`temporaltest/server_test.go:63-92`) — proving there is no required singleton. Caveats: a server instance cannot be restarted ("This function should be called only once" — `temporal/fx.go:349-351`), and a handful of process-wide effects remain (OTEL global error handler at `temporal/fx.go:981`, OS signal registration only if the host opts into `InterruptCh()` at `temporal/interrupt.go:9-21`, default Prometheus registry reuse noted at `temporal/server_test.go:190-194`).

### 2. Can the host supply policy, tools, identity, storage, telemetry, and secrets?

**Yes across the board**, each through a named interface:

- **Policy**: `Authorizer` gets structured `CallTarget{APIName, Namespace, Request}` per call (`common/authorization/authorizer.go:24-56`); wired into the frontend chain (`service/frontend/fx.go:120`, applied via `authInterceptor.Intercept` at `service/frontend/fx.go:298`).
- **Identity**: `ClaimMapper` maps `AuthInfo` (JWT/mTLS) to claims (`common/authorization/claim_mapper.go:17-33`); `WithAudienceGetter` for JWT audiences (`temporal/server_option.go:119-124`).
- **Storage**: `AbstractDataStoreFactory` explicitly exists "to implement custom datastore support outside of the Temporal core" (`common/persistence/client/abstract_data_store_factory.go:14-16`); SQL plugins additionally register by blank import (`cmd/server/main.go:21-23`).
- **Telemetry**: `metrics.Handler` including batch/"wide events" and `Stop` (`common/metrics/metrics.go:20-47`); OTEL exporters merged with documented precedence custom > env > config (`temporal/fx.go:997-1009`); structured-event `otellog.LoggerProvider` (`temporal/fx.go:222-225`).
- **Secrets**: `encryption.TLSConfigProvider` (`temporal/server_option.go:105-110`) and outbound `auth.TokenProvider` (`temporal/server_option.go:223-228`) — with construction-time cross-validation (`temporal/fx.go:299-308`).
- **Tools** (analog): persistence/archiver plugins plus the SDK-worker model — workflow/activity logic always lives in host-owned external workers registered via `worker.Registry` (`temporaltest/server.go:42-63`), i.e., the host fully owns executable extensions by design.

### 3. Are lifecycle, cancellation, shutdown, and error propagation explicit?

**Mostly yes, with three gaps.** Explicit: ordered start/stop keyed by `initOrder` with reverse-order teardown (`temporal/server_impl.go:47-53`, `temporal/server_impl.go:112-118`); fx lifecycle hooks bind `ServerImpl.Start/Stop` (`temporal/fx.go:943-948`) and each service's hooks (`service/frontend/fx.go:1098-1100`); graceful drain protocol (`service/frontend/service.go:544-575`); config/schema version validated before anything starts (`temporal/fx.go:191-195`, `temporal/fx.go:950-964`). Gaps: (a) cancellation is channel/signal-based, not `context.Context`-based — a host cannot pass its own cancellation context into `Start()`/`Stop()` (`temporal/server.go:15-20`); (b) if a later service fails to start, already-started services keep running and the host must call `Stop()` itself — `startServices` accumulates errors but does not roll back peers (`temporal/server_impl.go:138-145`); (c) `Stop()` errors from individual service graphs are logged and swallowed (`temporal/fx.go:372-379`), and the fixed 5-minute per-service stop timeout is not configurable by the embedder (`temporal/server.go:12`).

### 4. Does the integration model work for both local-first and service deployments?

**Yes — this is a strength.** One code path serves: single-binary dev mode with all four services in-process (`temporal.DefaultServices`, `temporal/server.go:33-40`); split production deployments via `ForServices` + ringpop membership; static-host deployments that disable dynamic membership entirely (`WithStaticHosts`, validated for completeness at `temporal/fx.go:289-297`); and ephemeral local/test use via SQLite-backed `LiteServer` with free-port allocation and in-memory mode (`temporaltest/internal/lite_server.go:80-100`). Programmatic `WithConfig` means no filesystem dependency is forced on embedders, while env-var-only bootstrapping exists for containers (`config.WithEmbedded` consumed at `cmd/server/main.go:176-177`; env keys at `common/config/loader.go:29-45`).

## Architectural Decisions

1. **Functional-options facade over an fx graph.** Embedders see plain `ServerOption` functions (`temporal/server_option.go:26-34`); internally everything is resolved into typed fx providers (`temporal/fx.go:310-346`). This hides DI complexity from casual embedders while keeping the graph overridable for advanced ones.
2. **One default-resolution point.** All "if the host didn't supply X, build default X" logic is centralized in `ServerOptionsProvider` (`temporal/fx.go:199-255`), making the effective configuration auditable in a single function.
3. **Nested fx graphs with an admitted workaround.** Each service is its own `fx.App`; common deps are realized into `ServiceProviderParamsCommon` in the server graph and re-provided into service graphs. The code openly documents this as "not ideal… a workaround" (`temporal/fx.go:418-425`) — honest, but it means host `fx.Replace` of internals does not automatically reach service sub-graphs except through the propagated params.
4. **Fail-fast construction.** Invalid service names, missing config, schema-version mismatch, static-host coverage gaps, and token/TLS inconsistencies all fail in `NewServer`/`loadAndValidate` before any goroutine exists (`temporal/server_options.go:84-104`, `temporal/fx.go:192-195`, `temporal/fx.go:290-297`, `temporal/fx.go:299-308`).
5. **Test server as a productized embedder.** `temporaltest` treats in-process embedding as a first-class supported mode with its own backward-compat policy (`temporaltest/README.md:5`), and `LiteServer` deliberately *wraps* (not embeds) `temporal.Server` to reserve room for future lifecycle hooks (`temporaltest/internal/lite_server.go:304-316`).
6. **Custom interceptors run innermost.** Host interceptors are appended after the entire internal chain (rate limiting, authz, validation) and before the retry interceptor (`service/frontend/fx.go:286-321`), prioritizing safety over host controllability; a TODO marks `WithChainedFrontendGrpcInterceptors` for deprecation (`service/frontend/fx.go:317`).

## Notable Patterns

- **Wrapper-over-embedding structs** to preserve API evolution freedom (`temporaltest/internal/lite_server.go:306-308` comment).
- **Static initializer tables**: `initOrder` map encodes inter-service dependencies as data (`temporal/server_impl.go:47-53`).
- **Env-var templated config**: YAML rendered as a sprig template when `# enable-template` is present, enabling secret injection without code (`common/config/loader.go:226-252`).
- **Adapter pattern for third-party logging**: `FxLogAdapter` funnels all fx events into Temporal's tagged logger, only surfacing failures (`temporal/fx.go:1147-1286`).
- **Error-log detector test harness**: the embedding smoke test installs a logger that fails the test on unexpected warn/error logs, enumerating known-transient messages (`temporal/server_test.go:218-309`) — embedding quality is asserted, not assumed.
- **Explicit "no direct env access" discipline**: server code reads environment variables only through the config loader; `temporal/environment/env.go:10-16` documents that helpers exist solely for tests/tools.

## Tradeoffs

- **Channel-vs-context lifecycle**: `InterruptOn(<-chan any)` integrates naturally with OS signals for CLI use (`temporal/interrupt.go:9-21`) but forces hosts that manage lifetimes via contexts to bridge manually.
- **Replaceability vs. stability**: the most powerful seams (custom data stores, client factories, stream interceptors, event logger) are stamped experimental (`temporal/server_option.go:148`, `:162-170`, `:178`, `:186`, `:212-216`, `:246-250`), trading contract durability for freedom to redesign.
- **Innermost interceptor placement** keeps host code away from rate-limit/auth decisions (safe for correctness, hostile for host-owned observability/policy enforcement at the edge) — custom interceptors are appended at `service/frontend/fx.go:316-318`, after the full internal chain starting at `service/frontend/fx.go:286-315`.
- **Per-test parallel servers** demonstrate isolation, but paid for with workarounds: a magic 100 ms sleep to dodge a ringpop panic (`temporaltest/server.go:168-169`) and random ports/Prometheus-address juggling in tests (`temporal/server_test.go:190-195`).
- **Swallowed shutdown errors** simplify the `Server` API (`Stop() error` can't represent five services' worth of failures) at the cost of diagnosability, mitigated only by logs (`temporal/fx.go:372-379`).

## Failure Modes / Edge Cases

- **Partial start without rollback**: `startServices` continues past a failing service and reports combined errors, leaving earlier-started services live; recovery requires the host to call `Stop()` (`temporal/server_impl.go:138-145`). Note the fx layer does log "start failed, rolling back" for other lifecycle hooks (`temporal/fx.go:1250-1256`), but the manual service-app loop sits outside that protection.
- **Global OTEL handler contention**: every embedded server sets `otel.SetErrorHandler` at exporter start (`temporal/fx.go:981`) and replaces it again during tracer shutdown (`temporal/fx.go:1087-1092`); two servers in one process silently clobber each other's handlers.
- **pprof listener leak**: the pprof HTTP server has OnStart only; the OnStop hook is literally commented out with a TODO (`common/pprof/fx.go:21-28`), so embedded servers leak a listener on Stop.
- **Non-restartability**: reusing a `Server` after `Stop()` is unsupported by contract (`temporal/fx.go:349-351`); embedders must construct a fresh instance (and re-run schema checks).
- **Startup races in test embedding**: ringpop can panic within 100 ms of start unless the test server sleeps (`temporaltest/server.go:166-169`) — evidence that concurrent in-process membership bring-up remains fragile.
- **Interceptor misplacement panics**: a frontend interceptor supplied via `WithChainedFrontendGrpcInterceptors` will `panic` if it sees a non-frontend handler in the test helper's guard pattern (`temporal/server_test.go:207-216` shows the expected defensive style hosts must adopt).
- **Config-source exclusivity**: mixing `--config-file` with env/dir/zone flags is rejected (`temporal/server_options.go:107-110`, `cmd/server/main.go:149-152`) — predictable, but embedders composing config sources programmatically must pick exactly one.

## Future Considerations

- Add `context.Context` parameters to `Start`/`Stop` (or a `Stop(ctx)`) so host-owned deadlines replace fixed 15 s/5 m constants (`temporal/server.go:11-12`).
- Roll back already-started services when `startServices` fails partway (`temporal/server_impl.go:138-145`), or expose partial-state status on `Server`.
- Graduate the experimental extension options (`WithCustomDataStoreFactory`, `WithClientFactoryProvider`, stream interceptors, event logger) to stable contracts or document concrete instability risks.
- Restore pprof OnStop and make the OTEL error handler per-instance rather than global (`common/pprof/fx.go:24-28`, `temporal/fx.go:981`).
- Publish an embedding guide in-repo; today the closest thing to docs is `temporaltest/README.md` (7 lines) plus scattered comments.
- Replace the nested-fx propagation workaround with a single shared graph or per-module option propagation, as the code itself suggests (`temporal/fx.go:418-425`).

## Questions / Gaps

- **No in-repo embedding documentation found.** Searched `docs/` for embedding/host/SDK-entry material — only internal architecture notes (`docs/architecture/*.md`); the embedding contract must be reverse-engineered from `temporal/server_option.go`. Stated design goals for embedders are therefore inferred from code, not documented intent.
- **Progress/streaming/approval surfacing** (dimension step 4) has no host callback layer: progress reaches external consumers exclusively through public gRPC APIs (workflow updates, long-poll) and error normalization happens in the interceptor chain (`service/fx.go:159-180`). No evidence of a richer host-facing event/callback API beyond logs/metrics was found in the embedding packages searched (`temporal/`, `temporaltest/`, `service/frontend/fx.go`).
- **Multi-server OTEL interaction** is untested: no test found covering two `NewServer` instances' interaction with global OTEL state; the claim of interference above rests on reading `temporal/fx.go:981`/`1087-1092`, not on observed failing behavior.
- **`GetCommonServiceOptions` override reachability**: whether host `fx.Replace` in a custom `topLevelModule` propagates into per-service sub-graphs is undocumented and untested in-repo; only the propagated `ServiceProviderParamsCommon` set demonstrably reaches services (`temporal/fx.go:425-509`).
- **Windows/embedding constraints** (signal handling, port binding) were out of scope of the searched code paths; no evidence either way.

---

Generated by `dimension 24.04-embedding-and-host-integration-ergonomics` against `temporal`.

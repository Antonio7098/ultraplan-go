# Source Analysis: opa

## Embedding and Host Integration Ergonomics (Dimension 24.04)

### Source Info

| Field | Value |
|-------|-------|
| Name | opa |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | Go (module `github.com/open-policy-agent/opa`, go.mod:1); v1 API under `v1/`, v0 compatibility shims at repo root (`sdk/opa.go:14-37`) |
| Analyzed | 2026-08-25 |

## Summary

OPA is designed from the ground up as an embeddable policy engine and offers four distinct embedding modes, each with an explicit dependency-injection surface:

1. **Low-level Go library** — the `rego` package builds an evaluator via functional options (`rego.New(options ...func(r *Rego))`, `v1/rego/rego.go:1428`), letting hosts inject store, compiler, transaction, custom built-ins, tracers, print hooks, time/seed sources, and capabilities.
2. **High-level SDK** — `sdk.New(ctx, sdk.Options)` (`v1/sdk/opa.go:66`) wraps the plugin manager, bundle/discovery machinery, decision logging, and readiness signaling into a lifecycle-managed `OPA` object (`v1/sdk/opa.go:41-55`), with atomic in-place reconfiguration (`v1/sdk/opa.go:149-166`).
3. **Full server runtime** — `runtime.NewRuntime` + `rt.Serve(ctx)` (`v1/runtime/runtime.go:384`, `v1/runtime/runtime.go:661`) expose the REST API; embedders building custom binaries configure everything through `runtime.Params` (`v1/runtime/runtime.go:125-314`). The CLI itself is just such an embedder (`cmd/run.go:381`, `cmd/run.go:396-401`).
4. **WASM/external-engine target** — pluggable evaluation targets behind `rego.TargetPlugin` / `TargetPluginEval` (`v1/rego/plugins.go:18-25`) registered globally.

Hosts retain ownership of policy supply (bundles/modules/store), tools (custom built-ins), identity (auth plugins for outbound HTTP), storage (in-memory, disk, or fully custom stores), telemetry (loggers, metrics, Prometheus registerer, OTel tracer provider, query tracers/profilers). Lifecycle is context-driven with explicit ready channels and plugin status states. The main ergonomic debts are a handful of process-global registries (built-ins, runtime plugins/storage/hook registries, `sdk.SetDefaultOptions`, `bundle.BundleExtStore`), two `os.Exit` paths in server mode, a non-idempotent `Manager.Stop`, and no first-class secrets provider or result streaming.

## Rating

**8 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale: embedding is a first-class product surface, not an afterthought. There is a dedicated SDK package with documented examples (`docs/docs/integration.md:207-262`), an extensions guide covering built-ins, plugins, and storage backends (`docs/docs/extensions.md:10`, `docs/docs/extensions.md:190`, `docs/docs/extensions.md:369`), typed hook interfaces with validation (`v1/hooks/hooks.go:100-113`), plugin status state machine (`v1/plugins/plugins.go:129-146`), graceful shutdown periods (`v1/plugins/plugins.go:397`, `920-947`), and extensive tests exercising out-of-tree plugins, hooks, reconfiguration, and panics (`v1/sdk/opa_test.go:160`, `180`, `273`). It falls short of 9-10 because: (a) several registration APIs mutate process-global state (`v1/rego/rego.go:761`, `v1/rego/plugins.go:36-43`, `v1/runtime/runtime.go:95-122`, `v1/sdk/options.go:29`, `v1/bundle/bundle.go:463`); (b) server mode can kill the host process (`os.Exit(1)` at `v1/runtime/runtime.go:835` and `651-655`); (c) `Manager.Stop` is documented as non-reentrant ("You cannot call this twice, or it will hang", `v1/plugins/plugins.go:925`); and (d) buffered startup logs are silently discarded when no logger plugin is configured (`v1/sdk/options.go:44-48`, `v1/plugins/plugins.go:1289-1290`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| SDK entry point | `sdk.New` returns lifecycle-managed `*OPA`; struct holds id/state/logger/plugins/store/hooks/config | `v1/sdk/opa.go:64-131`, `v1/sdk/opa.go:41-55` |
| SDK DI options | `Options{Config, Logger, ConsoleLogger, Ready, Plugins, ID, Store, Hooks, ManagerOpts}` | `v1/sdk/options.go:36-97` |
| Readiness contract | Host-supplied `Ready chan struct{}` closed when all plugin states are OK; `New` blocks if channel omitted | `v1/sdk/options.go:54-57`, `v1/sdk/opa.go:209-229`, `v1/sdk/opa.go:279-285` |
| Atomic reconfigure | `Configure` swaps manager in-place; equal config short-circuits; old manager stopped on background goroutine | `v1/sdk/opa.go:145-166`, `v1/sdk/opa.go:250-261` |
| Shutdown | `OPA.Stop` delegates to `Manager.Stop`; instance cannot be restarted | `v1/sdk/opa.go:290-299` |
| Decision API | `Decision(ctx, DecisionOptions)` with per-call input, path, ND-cache, tracer, profiler, metrics, decision ID, HTTP round-tripper override | `v1/sdk/opa.go:302-359`, `v1/sdk/opa.go:362-381` |
| Partial eval to host | `Partial` + host-mappable results via `PartialQueryMapper` interface (`MapResults`/`ResultToJSON`) | `v1/sdk/opa.go:447-506`, `v1/sdk/opa.go:508-513` |
| Typed error surfacing | `Error{Code, Message}` with `UndefinedErr` sentinel and `IsUndefinedErr` helper | `v1/sdk/opa.go:537-563` |
| Query cache invalidation | Compiler trigger clears prepared-query cache on policy change | `v1/sdk/opa.go:203-207` |
| Plugin access from host | `OPA.Plugin(name)` for manual triggers/status (documented bundle-trigger workflow) | `v1/sdk/opa.go:133-143`, `docs/docs/integration.md:300-339` |
| Out-of-tree plugins | `Plugins map[string]plugins.Factory` wired into discovery; test proves end-to-end registration | `v1/sdk/opa.go:237-246`, `v1/sdk/opa_test.go:160-181` |
| Plugin contract | `Factory.Validate/New` + `Plugin.Start/Stop/Reconfigure`; `Triggerable`, `LoggerPlugin` optional interfaces | `v1/plugins/plugins.go:89-123` |
| Plugin status machine | `StateNotReady/OK/Err/Warn`, `StatusListener`, `RegisterPluginStatusListener` | `v1/plugins/plugins.go:127-146`, `v1/plugins/plugins.go:188`, `v1/plugins/plugins.go:1047` |
| Manager DI options | Logger, console logger, print hook, router, Prometheus registerer, tracer provider, hooks, version checker, TLS floors | `v1/plugins/plugins.go:405-535` |
| Graceful shutdown | `GracefulShutdownPeriod` option bounds each plugin's Stop via context timeout; store `Close` honored when implemented | `v1/plugins/plugins.go:397-403`, `v1/plugins/plugins.go:926-952` |
| Buffered logging | Default SDK logger buffers 1000 entries; flushed to slog-based logger plugin or discarded/fallback after start | `v1/sdk/options.go:117-119`, `v1/logging/buffered_logger.go:17-31`, `v1/plugins/plugins.go:1270-1290` |
| Logger abstraction | Hosts implement 7-method `logging.Logger`; optional `LoggerWithContext` for trace propagation; global logger deprecated | `v1/logging/logging.go:29-57`, `v1/logging/logging.go:72-79` |
| Hooks system | `Hook any` with capability interfaces: config rewrite, discovery-config rewrite, inter-query cache sharing, bundle pre-activate; unknown types rejected by `Validate` | `v1/hooks/hooks.go:16-32`, `v1/hooks/hooks.go:70-98`, `v1/hooks/hooks.go:100-113` |
| Low-level library options | `rego.Store`, `rego.Compiler`, `rego.Transaction`, `rego.Runtime`, `rego.Time`, `rego.Seed`, `rego.Capabilities`, `rego.PrintHook`, `rego.CompilerHook`, ~60 more | `v1/rego/rego.go:1125`, `1114`, `1156`, `1206`, `1215`, `1223`, `1344`, `1368`, `1406` |
| Prepared queries | `PreparedEvalQuery.Eval(ctx, EvalOption...)`; `EvalContext` exposes cancel, caches, seed, txn to advanced hosts | `v1/rego/rego.go:561-581`, `v1/rego/rego.go:135-197`, `v1/rego/rego.go:200-457` |
| Scoped custom built-ins | `Function1..FunctionDyn`, `FunctionDecl` attach functions to a single Rego object only | `v1/rego/rego.go:830-875` |
| Global custom built-ins | `RegisterBuiltin1..4/Dyn` "adds a built-in function globally inside the OPA runtime" | `v1/rego/rego.go:760-828` |
| Cancellation | Context cancellation spawns one-off goroutine calling `topdown.Cancel`; `BuiltinContext.Cancel` checked in eval loop; host may supply `EvalExternalCancel` | `v1/rego/rego.go:2382-2395`, `v1/topdown/cancel.go:13-16`, `v1/topdown/builtins.go:41`, `v1/topdown/eval.go:379`, `v1/rego/rego.go:428-431` |
| Caller metadata pass-through | `EvalRequestMetadata`/`EvalResponseMetadata` for wrapping projects to attach per-query metadata surfaced in `BuiltinContext` | `v1/rego/rego.go:437-451`, `v1/topdown/builtins.go:56-57` |
| Storage injection | SDK `Options.Store` (default inmem); `rego.Store` option; runtime `DiskStorage`, `Params.StoreBuilder`, global `RegisterStorageBackend` | `v1/sdk/options.go:68-70`, `137-139`, `v1/rego/rego.go:1125`, `v1/runtime/runtime.go:270-276`, `v1/runtime/runtime.go:101-108` |
| Identity (outbound) | `HTTPAuthPlugin{NewClient, Prepare}` resolvable by name from service credentials config via `AuthPluginLookupFunc` / `Manager.AuthPlugin` | `v1/plugins/rest/rest.go:42-51`, `84-99`, `v1/plugins/plugins.go:740` |
| Telemetry injection | Metrics + instrumentation options; query tracers & profiler per decision; Prometheus registerer and OTel tracer provider on manager | `v1/rego/rego.go:219-268`, `v1/sdk/opa.go:368-371`, `v1/plugins/plugins.go:438-457` |
| Runtime params | Server-mode DI: addrs, auth/authz schemes, TLS material, router, decision-ID factory, loggers, hooks, store builder, shutdown periods | `v1/runtime/runtime.go:125-314` |
| Global runtime registries | `RegisterPlugin`, `RegisterStorageBackend`, `RegisterHook` — explicitly justified for embedders that build their own binary off the CLI | `v1/runtime/runtime.go:91-122` |
| Server lifecycle ownership | `Serve` installs SIGINT/SIGTERM handler; listener failure calls `os.Exit(1)`; `StartServer` exits process on error | `v1/runtime/runtime.go:810-838`, `v1/runtime/runtime.go:651-656` |
| CLI as embedder | `cmd/run.go` constructs `runtime.Params` then `runtime.NewRuntime`; same package used by custom binaries | `cmd/run.go:381`, `cmd/run.go:396-401` |
| Error format to clients | Server `ErrorV1{Code, Message, Errors []error}` structured error envelope | `v1/server/types/types.go:131-137` |
| Decision logs | `logs.Lookup(manager)` + `logger.Log(ctx, record)` invoked per SDK decision; masking policy default `/system/log/mask` | `v1/sdk/opa.go:425-440`, `v1/plugins/logs/plugin.go:649`, `716`, `272` |
| Global bundle-store hook | `bundle.BundleExtStore` package var consulted by SDK when host did not pass a store | `v1/bundle/bundle.go:456-484`, `v1/sdk/opa.go:184-192` |
| Docs: embedding example | Full SDK example incl. mock bundle server, config bytes, `defer opa.Stop(ctx)` | `docs/docs/integration.md:200-262` |
| Tests: embedding behavior | SDK tests cover plugins, config hooks, plugin panic recovery, discovery, decision logging/masking, query caching | `v1/sdk/opa_test.go:160`, `180`, `213`, `273`, `1525`, `1665`, `1847`, `1913` |

## Answers to Dimension Questions

**1. Can the harness run inside another application without owning the whole process?**
Yes — this is OPA's primary design goal. The `rego` package is a pure library with zero background work: evaluation runs synchronously on the caller's goroutine, transactions are opened/aborted around each call, and prepared queries are cached per-instance (`v1/rego/rego.go:561-581`, `v1/sdk/opa.go:725-757`). The SDK adds only bounded background work scoped to the returned object (plugin status loop `v1/plugins/plugins.go:612`, bundle/download plugins) and never touches signals or exit codes. Caveat: *server mode* does assume process ownership — it registers signal handlers (`v1/runtime/runtime.go:814-815`) and exits the process on listener failure (`os.Exit(1)` at `v1/runtime/runtime.go:835`; `StartServer` at `v1/runtime/runtime.go:651-655`). Hosts embedding the server should use `Serve(ctx)` and treat listener errors carefully, since the exit bypasses the deferred `Manager.Stop` (`v1/runtime/runtime.go:691`).

**2. Can the host supply policy, tools, identity, storage, telemetry, and secrets?**
- **Policy**: yes — bundles over HTTP/file (`docs/docs/integration.md:229-243`, `279-298`), inline modules (`rego.Module`, `v1/rego/rego.go:1053`), pre-parsed modules/bundles (`1065`, `1107`), injected compilers (`1114`), plus discovery-driven policy (`v1/sdk/opa.go:237-246`).
- **Tools**: yes — scoped custom built-ins (`rego.Function1..FunctionDecl`, `v1/rego/rego.go:830-875`) and process-wide ones (`RegisterBuiltin1..Dyn`, `761-828`); built-ins receive `BuiltinContext` with request context, caches, seed, cancel flag (`v1/topdown/builtins.go:36-60`).
- **Identity**: partially — outbound service calls support named auth plugins (`HTTPAuthPlugin`, `v1/plugins/rest/rest.go:42-51`) and built-in bearer/OAuth2/TLS/S3/GCP/Azure credential modes (`61-69`); server-side authn/authz schemes come via `Params.Authentication/Authorization` (`v1/runtime/runtime.go:141-145`). There is no generic identity abstraction passed into evaluation beyond input documents.
- **Storage**: yes — any `storage.Store` implementation can be injected at every layer (`v1/sdk/options.go:68-70`, `v1/rego/rego.go:1125`, `v1/runtime/runtime.go:275-276`, `101-108`).
- **Telemetry**: yes — replaceable loggers (`v1/sdk/options.go:44-52`), metrics instances (`v1/rego/rego.go:219`), tracers/profilers per decision (`v1/sdk/opa.go:368-370`), Prometheus registry and OpenTelemetry tracer provider (`v1/plugins/plugins.go:438`, `445`), distributed tracing opts (`452`).
- **Secrets**: no first-class secret store. Secrets enter either as literal values inside config bytes (e.g., bundle verification keys, `v1/plugins/plugins.go:587-590`) or must be fetched inside a custom auth plugin or custom built-in. This forces hosts with secret managers to wrap OPA rather than configure it.

**3. Are lifecycle, cancellation, shutdown, and error propagation explicit?**
Largely yes. Initialization is explicit and fallible (`sdk.New` returns error before any goroutines leak past manager start, `v1/sdk/opa.go:109-130`); readiness is a first-class concept with host-owned channels and plugin status listeners (`v1/sdk/opa.go:209-229`); reconfiguration is atomic with rollback semantics ("If the configuration update cannot be successfully applied, the old configuration will remain intact", `v1/sdk/opa.go:145-148`); cancellation threads through `context.Context` into the evaluator (`v1/rego/rego.go:2382-2395`) and even into built-ins via `BuiltinContext.Cancel` (`v1/topdown/builtins.go:41`); shutdown honors configurable grace periods and closes stores implementing `Close` (`v1/plugins/plugins.go:938-952`). Weak spots: `Manager.Stop` cannot be called twice without hanging (`v1/plugins/plugins.go:925`), the replaced manager during `Configure` is stopped on a detached goroutine whose completion the host cannot observe (`v1/sdk/opa.go:256-261`), and the doc comment on `Plugin` claims "Currently OPA will not call Stop on plugins" (`v1/plugins/plugins.go:101`) which contradicts `Manager.Stop` actually stopping all registered plugins (`945-947`) — stale documentation that misleads embedders about cleanup guarantees.

**4. Does the integration model work for both local-first and service deployments?**
Yes. Local-first: file-scheme bundles or shipped policy files with the inmem store require zero network config (`docs/docs/integration.md:272-298`); the REPL/CLI path works without a server (`cmd/run.go:396-401`). Service deployment: services/bundles/discovery/decision-log/status plugins provide the full management plane (`v1/sdk/opa.go:237-246`), with manual bundle triggering for event-driven refresh (`docs/docs/integration.md:300-339`). Cross-language embedding is served by WASM compilation targets (`rego.Target("wasm")`, `v1/rego/rego.go:1351`, backed by the `wasm/` tree) and the pluggable `TargetPlugin` registry (`v1/rego/plugins.go:18-43`). One asymmetry: the hosted-service variant of OPA itself is outside this repository, so multi-tenant control-plane ergonomics could not be assessed here.

## Architectural Decisions

- **Two-tier embedding API**: a maximalist low-level library (`rego`: 60+ functional options, `v1/rego/rego.go:929-1425`) beneath a deliberately small high-level SDK (`sdk.Options` has 11 public fields, `v1/sdk/options.go:36-97`). Hosts choose their exposure level; the SDK is implemented *on top of* the same public packages it exposes, dogfooding the extension points.
- **Configuration as opaque bytes**: both SDK and runtime accept raw YAML/JSON config readers (`v1/sdk/options.go:38-42`) parsed once by `plugins.Manager` (`v1/plugins/plugins.go:537-541`), keeping the config schema evolvable independently of the Go API — but meaning hosts get stringly-typed config rather than typed structs at the boundary.
- **Plugin manager as the composition root**: storage, compiler, wasm resolvers, services, keys, hooks, and status all hang off `plugins.Manager` (`v1/plugins/plugins.go:190-245`), which both the SDK and the server share. This gives one consistent place to inject dependencies regardless of embedding mode.
- **Capability-interface hooks**: instead of one fat callback struct, hooks declare narrow interfaces (`ConfigHook`, `InterQueryCacheHook`, `BundlePreActivateHook`, …) discovered via type assertion, with upfront validation rejecting unrecognized hook types (`v1/hooks/hooks.go:16-32`, `100-113`). Extensible without breaking existing hooks.
- **Per-decision observability opt-in**: tracer, profiler, metrics, ND-cache, and round-tripper overrides travel with each `DecisionOptions` call (`v1/sdk/opa.go:362-381`) rather than being instance-wide, so a host can trace one request in production without paying globally.
- **Prepared-query caching keyed by policy version**: SDK caches `PreparedEvalQuery` per path and invalidates on compiler triggers (`v1/sdk/opa.go:203-207`, `610-634`), hiding the compile/eval split from casual embedders while leaving it available to performance-sensitive ones.

## Notable Patterns

- **Functional options everywhere**: identical option idioms across `rego.Rego` (`v1/rego/rego.go:1428`), `plugins.Manager` (`v1/plugins/plugins.go:369-535`), and `discovery` — a host engineer learns one pattern.
- **Ready-channel handshake**: `New` blocks until ready unless the host supplies its own `Ready` channel (`v1/sdk/options.go:110-115`), supporting both simple blocking init and event-loop-friendly async init; docs explicitly warn about this ("needed or else sdk.New will block", `docs/docs/integration.md:317`).
- **Buffered-then-resolved logging**: early startup logs are captured in a ring buffer and replayed into whichever logger plugin resolves later, avoiding lost diagnostics during plugin bring-up (`v1/sdk/options.go:117-119`, `v1/plugins/plugins.go:1270-1288`).
- **Print statement routing**: policy `print()` output is routed through an injectable `print.Hook`, defaulted to the host logger with location fields (`v1/sdk/opa.go:174-182`, `775-782`).
- **Metadata smuggling for wrappers**: `EvalRequestMetadata`/`EvalResponseMetadata` exist specifically "for use by wrapping projects" to move caller context through evaluation into custom built-ins (`v1/topdown/builtins.go:56-57`, `v1/rego/rego.go:437-451`).
- **Test doubles shipped as API**: `sdktest.NewServer`/`MockBundle` used directly in the official embedding example (`docs/docs/integration.md:210-224`) gives hosts an immediate way to test their integration.

## Tradeoffs

- **Global registries vs. convenience**: process-wide built-in registration (`v1/rego/rego.go:761`), target-plugin registration that panics on duplicates (`v1/rego/plugins.go:36-43`), runtime-level plugin/storage/hook registries (`v1/runtime/runtime.go:95-122`), and `SetDefaultOptions` (`v1/sdk/options.go:29-33`) make init()-style composition easy but mean two embedded OPA instances in one process share those namespaces — multi-tenant or multi-version embedding requires discipline. Mitigations exist (instance-scoped `Function1`, `sdk.Options.Plugins` map, `ResetTargetPlugins` for tests, `v1/rego/plugins.go:45-49`) but the split between global and scoped APIs is not uniformly enforced.
- **Opaque config vs. typed safety**: byte-oriented config enables YAML/JSON and remote discovery, but hosts lose compile-time checking; a typo surfaces only at `New`/`Init`.
- **Server-mode convenience vs. host control**: `os.Exit(1)` on listener failure (`v1/runtime/runtime.go:835`) simplifies standalone operation but is hostile inside a larger process that has its own supervisor/restart logic.
- **Background stop vs. deterministic shutdown**: deferring old-manager `Stop()` to a goroutine avoids deadlocks during reconfig but means `Configure` returning does not guarantee prior resources are released (`v1/sdk/opa.go:250-261`).
- **Synchronous results vs. streaming**: decisions return complete `ResultSet`s; there is no partial/streaming output channel for long-running queries. Hosts wanting progress must rely on tracing/profiling hooks (`v1/sdk/opa.go:368-370`) or wrap evaluation themselves.

## Failure Modes / Edge Cases

- **Silent log loss**: if no logger plugin is configured and no fallback logger supplied, buffered startup logs are discarded (`v1/plugins/plugins.go:1270-1290`; SDK passes nil fallback at `v1/sdk/opa.go:277`) — debugging early failures becomes harder exactly when things fail early.
- **Double-stop hang**: calling `Manager.Stop` twice hangs (`v1/plugins/plugins.go:925`); the SDK shields users somewhat since `OPA.Stop` is single-shot ("The OPA cannot be restarted", `v1/sdk/opa.go:290`), but direct-manager embedders must track this themselves.
- **Plugin panic handling**: SDK explicitly tests that a panicking plugin does not take down the host process (`v1/sdk/opa_test.go:273 TestPluginPanic`), indicating deliberate containment.
- **Process exit on listener error**: `Serve`'s `errc` branch exits the process, skipping deferred `Manager.Stop` cleanup (`v1/runtime/runtime.go:833-835` vs `691`) — port conflicts become fatal crashes rather than recoverable errors.
- **Reconfiguration cost**: `Configure` rebuilds the entire manager and caches rather than diffing ("we could be more intelligent about re-configuration", `v1/sdk/opa.go:154-155`), so frequent config updates are expensive.
- **Stale lifecycle documentation**: `v1/plugins/plugins.go:101` ("Currently OPA will not call Stop on plugins") contradicts actual behavior (`945-947`) and could cause embedders to skip implementing proper cleanup.
- **Blocking init surprise**: forgetting to pass `Ready` makes `sdk.New` block indefinitely if bundles never activate (readiness waits for all plugins OK, `v1/sdk/opa.go:209-229`); there is no built-in timeout at the SDK layer.

## Future Considerations

- Introduce a typed configuration builder alongside raw bytes so hosts get compile-time validation of the config surface they inject.
- Make `Manager.Stop` idempotent (e.g., sync-once semantics) and return the completion of old-manager teardown from `SDK.Configure` so resource ownership is observable.
- Provide an instance-scoped replacement for `RegisterBuiltin*` and the target-plugin registry so multiple isolated engines can coexist in one process without namespace collisions.
- Replace `os.Exit` paths in `v1/runtime/runtime.go:651-655`/`835` with error returns (or an injectable failure handler) so server-mode embedders keep control of process lifetime.
- Add a secrets-provider interface analogous to `HTTPAuthPlugin` so credential material can be resolved dynamically instead of living in static config.
- Consider an optional streaming/chunked evaluation API for hosts that need incremental results or long-running query progress.

## Questions / Gaps

- **Approvals / human-in-the-loop**: No evidence found — nothing in the codebase models approval workflows, which is expected for a deterministic policy engine; nearest analogues are bundle activation gating and decision-log audit (`v1/plugins/logs/plugin.go:716`). Search boundary: `v1/sdk`, `v1/rego`, `v1/plugins`, `v1/runtime`, docs.
- **Multi-instance isolation guarantees**: I found no documentation or tests describing behavior when multiple `sdk.OPA` instances coexist with globally registered built-ins; the interaction between `SetDefaultOptions` (`v1/sdk/options.go:23-33`) and concurrent `New` calls appears mutex-protected but semantically last-write-wins. Impact on real embedders unverified.
- **Non-Go embedding depth**: the WASM target exists (`v1/rego/rego.go:1351`, `wasm/` tree), but whether the maintained bindings (outside this repo) cover lifecycle/cancellation parity with the Go SDK could not be verified within this source's boundaries.
- **Progress reporting granularity**: beyond plugin states and per-decision metrics/tracing, there is no progress callback for long compilations; whether this matters in practice was not measurable from the source alone.

---

Generated by dimension `24.04-embedding-and-host-integration-ergonomics` against `opa`.

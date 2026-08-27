# Source Analysis: opa

## 21.03 Extension Compatibility Testing

### Source Info

| Field | Value |
|-------|-------|
| Name | opa |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | Go 1.26 (module `github.com/open-policy-agent/opa` `go.mod:1`), Rego |
| Analyzed | 2026-08-27 |

## Summary

OPA exposes three mature extension surfaces — runtime plugins (`Factory`/`Plugin`), custom built-ins (`ast.RegisterBuiltin` + `topdown.RegisterBuiltinFunc` / `rego.RegisterBuiltin*` / `rego.Function*`), and custom storage/TargetPlugin — with explicit interfaces and rich documentation/examples. Operational safeguards exist ( `Manager.UpdatePluginStatus` `StateOK/StateErr` lifecycle, status listeners, compiler `deprecated` enforcement). However there is **no exported conformance test suite or fixture library** that lets a third-party author `go test` their impl against the contract; verification depends on author-written tests and running OPA integration tests. Stability guarantees are not codified (no `STABILITY.md`, no `//go:embed` versioned contract); breaking changes are communicated reactively via `CHANGELOG.md` and release notes under Semantic Versioning, with a `deprecated` field in `capabilities.json`/`builtin_metadata.json` and a `v0-compatibility` shim for the Rego language but not for the Go plugin/builtin API.

## Rating

**5 / 10 — Present but inconsistent, weakly documented, fragile for contract verification**

Interfaces are explicit and well-exercised internally, examples are thorough, and status/deprecation machinery exists. But the core question — *can an extension author verify their implementation against the contract without copying internal tests?* — is **no**: no `PluginConformanceSuite`, no `testutil.NewFakeManager`, no `builtinTestHarness` is exported. Stability tier and breaking-change policy are implicit (SemVer + changelog) rather than documented, making upgrades fragile for external plugin/builtin authors.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Plugin Factory contract | `type Factory interface { Validate(*Manager, []byte) (any,error); New(*Manager, any) Plugin }` with 2-step Validate→New doc | `studies/agent-harness-study/sources/opa/v1/plugins/plugins.go:89-92` |
| Plugin lifecycle contract | `type Plugin interface { Start(context.Context) error; Stop(context.Context); Reconfigure(context.Context, any) }` + comment `Currently OPA will not call Stop` | `studies/agent-harness-study/sources/opa/v1/plugins/plugins.go:106-110` |
| Plugin state model | `type State string` + `StateNotReady/StateOK/StateErr/StateWarn` constants | `studies/agent-harness-study/sources/opa/v1/plugins/plugins.go:127-145` |
| Status reporting | `type Status struct { State State; Message string }` and `Manager.UpdatePluginStatus` / `PluginStatus` / listeners | `studies/agent-harness-study/sources/opa/v1/plugins/plugins.go:170-176` and `studies/agent-harness-study/sources/opa/v1/plugins/plugins.go:1066-1073` |
| Manager registration | `Manager.Register(name string, plugin Plugin)` appends to slice and sends `statusInitPlugin` via channel loop | `studies/agent-harness-study/sources/opa/v1/plugins/plugins.go:702-713` |
| Runtime registration | Global registry `registeredPlugins map[string]plugins.Factory` + `func RegisterPlugin(name string, factory plugins.Factory)` (idempotent) | `studies/agent-harness-study/sources/opa/v1/runtime/runtime.go:68-69` / `studies/agent-harness-study/sources/opa/v1/runtime/runtime.go:95-99` |
| Storage extension | `type StorageBackendBuilder func(ctx context.Context, logging.Logger, prometheus.Registerer, []byte, string) (storage.Store,error)` + `RegisterStorageBackend` | `studies/agent-harness-study/sources/opa/v1/runtime/runtime.go:89-90` / `studies/agent-harness-study/sources/opa/v1/runtime/runtime.go:104-108` |
| Target plugin (Wasm/compile) | `type TargetPlugin interface { IsTarget(string) bool; PrepareForEval(...) (TargetPluginEval,error)}` + global `RegisterPlugin` panics on duplicate | `studies/agent-harness-study/sources/opa/v1/rego/plugins.go:18-21` / `studies/agent-harness-study/sources/opa/v1/rego/plugins.go:36-43` |
| Built-in declaration contract | `var Builtins []*Builtin` + `func RegisterBuiltin(b *Builtin)` appends to slice+map, not thread-safe, init-only | `studies/agent-harness-study/sources/opa/v1/ast/builtins.go:15-40` |
| Built-in implementation contract | `type BuiltinFunc func(bctx BuiltinContext, operands []*ast.Term, iter func(*ast.Term)error) error` + `func RegisterBuiltinFunc(name string, f BuiltinFunc)` + error wrapper | `studies/agent-harness-study/sources/opa/v1/topdown/builtins.go:62-67` / `studies/agent-harness-study/sources/opa/v1/topdown/builtins.go:89-91` |
| High-level builtin helpers | `func RegisterBuiltin1/2/3/4/Dyn` in rego package: registers decl + wraps impl via `memoize`/`finishFunction` | `studies/agent-harness-study/sources/opa/v1/rego/rego.go:761-772` / `studies/agent-harness-study/sources/opa/v1/rego/rego.go:816-828` |
| Per-query builtin injection | `func Function1/2/3/4/Dyn(decl *Function, impl ...) func(*Rego)` stored in `r.builtinDecls/r.builtinFuncs` | `studies/agent-harness-study/sources/opa/v1/rego/rego.go:831-868` |
| Builtin metadata | `Builtin` struct includes `Description, Decl *types.Function, Nondeterministic bool, Categories, CanSkipBctx` and `deprecated` flag exported | `studies/agent-harness-study/sources/opa/builtin_metadata.json:578` / `studies/agent-harness-study/sources/opa/capabilities.json:44` |
| Deprecation enforcement | `Deprecated BuiltinMap` comment + `All/Any/SetDiff` deprecated built-ins still present; compiler `strict` rejects `deprecated built-in function calls` | `studies/agent-harness-study/sources/opa/v1/ast/builtins.go:621` / `studies/agent-harness-study/sources/opa/v1/ast/compile_test.go:3832-3852` |
| Internal plugin tests (not conformance suite) | `TestManagerPluginStatusListener`, `TestPluginStatusUpdateOnStartAndStop`, `TestExternalSourceIntegration` exercise Manager internals but are not exported | `studies/agent-harness-study/sources/opa/v1/plugins/plugins_test.go:103-168` / `studies/agent-harness-study/sources/opa/v1/plugins/plugins_test.go:170-201` / `studies/agent-harness-study/sources/opa/v1/plugins/plugins_test.go:662-703` |
| Runtime plugin tests | `TestRegisterPlugin`, `TestRegisterPluginNotStartedWithoutConfig`, `TestRegisterPluginBadBootConfig` validate registration via temp FS, not reusable harness | `studies/agent-harness-study/sources/opa/v1/runtime/plugins_test.go:57-86` / `studies/agent-harness-study/sources/opa/v1/runtime/plugins_test.go:89-117` |
| Builtin test example | `TestCustomBuiltinIterator` shows author pattern `NewQuery(...).WithBuiltins(map[string]*Builtin{...})` but is internal to `topdown` package | `studies/agent-harness-study/sources/opa/v1/topdown/builtins_test.go:12-33` |
| Mock fixtures (internal, not exported) | `type testPlugin struct { m *Manager }`, `type Tester struct{}`, `type Factory struct{}` used only inside test files | `studies/agent-harness-study/sources/opa/v1/plugins/plugins_test.go:186-201` / `studies/agent-harness-study/sources/opa/v1/runtime/plugins_test.go:20-54` |
| Example — custom builtin | Hello builtin and `github.repo` builtin with `Memoize/Nondeterministic` fields, complete `rego.Function{Decl: types.NewFunction(...)}` samples | `studies/agent-harness-study/sources/opa/docs/docs/extensions.md:46-60` / `studies/agent-harness-study/sources/opa/docs/docs/extensions.md:97-116` |
| Example — custom plugin | Full `PrintlnLogger` plugin: `Config`, `Start/Stop/Reconfigure/Log`, `Factory.Validate/New`, `runtime.RegisterPlugin` + yaml `decision_logs.plugin` | `studies/agent-harness-study/sources/opa/docs/docs/extensions.md:226-280` / `studies/agent-harness-study/sources/opa/docs/docs/extensions.md:290-305` / `studies/agent-harness-study/sources/opa/docs/docs/extensions.md:318-340` |
| Example — storage backend | `RegisterStorageBackend` snippet with `func(ctx, logger, registerer, config, id) (storage.Store,error)` | `studies/agent-harness-study/sources/opa/docs/docs/extensions.md:385-401` |
| Rego example test | `rego.RegisterBuiltin2` used in `v1/rego/example_test.go` | `studies/agent-harness-study/sources/opa/v1/rego/example_test.go:1057` |
| Stability / compat docs | `v0 Backwards Compatibility` doc describes `--v0-compatible` flag and `SetRegoVersion` for Rego language migration, not for Go plugin/builtin API stability | `studies/agent-harness-study/sources/opa/docs/docs/v0-compatibility.md:8-52` |
| Breaking-change communication | `CHANGELOG.md` preamble `adheres to [Semantic Versioning]` and per-release breaking notes (e.g., IR Evaluators must interpret new statement, OPA 1.0 deprecated builtins removal) | `studies/agent-harness-study/sources/opa/CHANGELOG.md:4-5` / `studies/agent-harness-study/sources/opa/CHANGELOG.md:2725` / `studies/agent-harness-study/sources/opa/CHANGELOG.md:2993-2998` |
| Shim / compatibility layer | `ast/builtins.go` and `plugins/plugins.go` are type-alias shims `type Factory = v1.Factory` preserving v0 import path | `studies/agent-harness-study/sources/opa/plugins/plugins.go:73` / `studies/agent-harness-study/sources/opa/ast/builtins.go:16-18` |

## Answers to Dimension Questions

### 1. Are extension contracts tested?
**Partially — internally but no author-facing conformance suite.**  
The `Factory` (`v1/plugins/plugins.go:89`) and `Plugin` (`v1/plugins/plugins.go:106`) interfaces are exercised by internal manager tests (`v1/plugins/plugins_test.go:103`, `v1/plugins/plugins_test.go:170`, `v1/runtime/plugins_test.go:57`). The built-in contract (`RegisterBuiltin` `v1/ast/builtins.go:22`, `RegisterBuiltinFunc` `v1/topdown/builtins.go:89`, `rego.RegisterBuiltin*` `v1/rego/rego.go:761`) is tested via `v1/topdown/builtins_test.go:12` and indirectly via `v1/rego/rego_test.go:3257-3361` (`TestDescriptionRegisterBuiltin1..Dyn`), plus `v1/ast/compile_test.go:3832` strict-deprecated checks. There is **no exported conformance harness** (e.g., `plugin.ConformanceSuite(t, myFactory)` or `topdown.BuiltinConformance`) that an external author can import to verify `Validate`/`New`/`Start`/`Stop`/`Reconfigure` ordering, status transitions, or built-in arity/type errors. Authors must copy internal mocks (`testPlugin` `v1/plugins/plugins_test.go:186`, `Tester` `v1/runtime/plugins_test.go:20`) into their own tests. Search for `conformance` yields only blog references to an external `opa-compliance-test` IR repo, not code in this source tree.

### 2. Are fixtures provided for extension authors?
**No.** No package exports test fixtures, fakes, or helpers for plugin/builtin authors. `Manager` construction (`v1/plugins/plugins.go:537 New`) requires real `storage.Store` (`inmem.New()`), config bytes, and optional `Manager` options; tests create their own `testPlugin`/`mockExternalSource` (`v1/plugins/plugins_test.go:621-643`) locally and do not export them. `util/test.WithTempFS` (`v1/runtime/plugins_test.go:65`) is generic FS helper, not plugin-specific. The built-in example in tests uses `NewQuery(...).WithBuiltins(map[string]*Builtin{...})` (`v1/topdown/builtins_test.go:15`) which is internal API (`query` struct not `rego.Rego`). The public `rego.Function1..Dyn` helpers (`v1/rego/rego.go:831`) serve as declarative fixtures but provide no transaction/cancel/tracing scaffolding. Extension doc `docs/docs/extensions.md:45` lists manual steps without referencing a fixture library.

### 3. Are examples provided?
**Yes, strong.** `docs/docs/extensions.md` is the canonical extension guide:
- Custom built-ins: complete runnable `hello` function (`docs/docs/extensions.md:46-60`), `github.repo` with memoization/nondeterminism, HTTP + `bctx.Context`, `ast.As`, `ast.ValueFromReader` (`docs/docs/extensions.md:97-177`), and appendix full `main` both as per-query (`docs/docs/extensions.md:450-506`) and global `rego.RegisterBuiltin2` (`docs/docs/extensions.md:526-574`).
- Custom plugin: decision-logger `PrintlnLogger` implements `Start`/`Stop`/`Reconfigure`/`Log`, `Factory` implements `Validate` via `util.Unmarshal` and `New` via `m.UpdatePluginStatus(..., StateNotReady)` (`docs/docs/extensions.md:226-305`), registration `runtime.RegisterPlugin(PluginName, Factory{})` and `cmd.RootCommand.Execute()` (`docs/docs/extensions.md:318-324`), plus config YAML (`docs/docs/extensions.md:338-343`) and contrib repo link.
- Storage: `RegisterStorageBackend` snippet (`docs/docs/extensions.md:385-401`).
- Code-level examples: `v1/rego/example_test.go:1057` (`RegisterBuiltin2`), `build/generate-extended-cases/extended_cases.go:160-177` registers `test.sleep` + `rego.RegisterPlugin`.

### 4. Are stability guarantees documented?
**No explicit guarantee; implicit via SemVer and deprecation machinery.**  
`CHANGELOG.md:4` states `adheres to [Semantic Versioning]`, which implies breaking changes gated to major versions, and each release enumerates breaking changes (e.g., `CHANGELOG.md:2725` IR evaluator breaking change, `CHANGELOG.md:2993` OPA 1.0 deprecated-builtin removal), but no `STABILITY.md` / `API_COMPATIBILITY.md` defines tiers (stable/alpha/deprecated) for `plugins.Factory`/`Plugin` or `RegisterBuiltin` signatures. The only documented compatibility mode is for the Rego language (`docs/docs/v0-compatibility.md:8`: `--v0-compatible` flag, `ast.RegoV0` vs `RegoV1`), not for the Go extension API. Stability is implemented mechanically: `builtin_metadata.json:578` `deprecated:true`, `capabilities.json:44` `deprecated:true`, `v1/ast/builtins.go:621` `RegexMatchDeprecated`, `v1/ast/compile_test.go:3832` strict-mode errors, and `plugins/plugins.go:73` shim aliasing `v0`→`v1` — but none is accompanied by a documented deprecation timeline or support window. `CONTRIBUTING.md:30` wire-format stability for IR proto (`v1/ir/plan.proto`) is the only strong guarantee textually documented.

## Architectural Decisions

| Decision | Location | Tradeoff |
|----------|----------|----------|
| Two-phase plugin instantiation `Factory.Validate(config []byte) (any,error)` → `Factory.New(Manager, any) Plugin` | `studies/agent-harness-study/sources/opa/v1/plugins/plugins.go:89-92` | Validates config before allocating resources; but forces authors to juggle raw bytes + parsed config and deal with `any` typing with no generated config schema helper. |
| Global plugin registry `registeredPlugins map[string]plugins.Factory` + `runtime.RegisterPlugin` in `init()` | `studies/agent-harness-study/sources/opa/v1/runtime/runtime.go:68-99` | Simple static linking; prevents dynamic plugin discovery and makes testing require global mutation (needs `ResetTargetPlugins` `v1/rego/plugins.go:45`). |
| Status channel loop `pluginStatusLoop` with `statusUpdate/statusInitPlugin/statusRegisterListener` messages | `studies/agent-harness-study/sources/opa/v1/plugins/plugins.go:1389-1427` | Centralizes status fan-out, avoids races, supports `Manager.Stop` graceful drain (`v1/plugins/plugins.go:954-958`); but hides status history — only `copyPluginStatus` snapshot is queryable. |
| Built-in registry as mutable global slice+map `Builtins, BuiltinMap` mutated by `RegisterBuiltin` at init, documented as not thread-safe | `studies/agent-harness-study/sources/opa/v1/ast/builtins.go:15-40` | Low ceremony for global builtins; but concurrent registration after start risks `concurrent map read/write` panic with no guard. |
| Per-`Rego` builtin injection via `r.builtinDecls/r.builtinFuncs` maps vs global `RegisterBuiltin*` | `studies/agent-harness-study/sources/opa/v1/rego/rego.go:693-695` + `v1/rego/rego.go:831-868` | Per-query isolation enables test fixtures without global pollution; but duality (global vs per-query) confuses authors choosing between `rego.RegisterBuiltin2` and `rego.Function2`. |
| Deprecation via `capabilities.json` `deprecated:true` + `ast.Compiler` strict error | `studies/agent-harness-study/sources/opa/capabilities.json:44` / `studies/agent-harness-study/sources/opa/v1/ast/compile_test.go:3832` | Allows tooling (Regal, `opa check --strict`) to surface deprecation without code break; but runtime still registers deprecated symbols, so removal requires major version. |

## Notable Patterns

- **Factory+Manager pattern with status callback:** Plugin never owns its lifecycle thread; `Manager` calls `Start` after `Init`, and plugin reports health via `m.UpdatePluginStatus(name, &Status{State: StateOK})` (`docs/docs/extensions.md:244-245`, `v1/plugins/plugins_test.go:191`). Mirrors Kubernetes controller pattern.
- **Error wrapping with location:** Builtin wrapper `handleBuiltinErr` (`v1/topdown/builtins.go:180-193`) maps `builtins.ErrOperand→TypeErr`, other `→BuiltinErr` and decorates with `name + ": " + err.Error()` plus `ast.Location`, enabling precise policy-site errors.
- **Memoization opt-in:** `rego.Function{Memoize:true}` (`docs/docs/extensions.md:130`, `v1/rego/rego.go:893-926` `memoize`) keys on `decl.Name + arg.String()` via `bctx.Cache`, useful for I/O builtins.
- **Shim aliasing for v0↔v1 migration:** `plugins/plugins.go:73 type Factory = v1.Factory` and `ast/builtins.go:16 func RegisterBuiltin(...)` expose same impl under two import paths, easing semver migration but doubling doc surface.
- **Extra extension vectors:** `Manager.ExtraRoute/ExtraMiddleware/ExtraAuthorizerRoute` (`v1/plugins/plugins.go:788-814`), `RegisterExternalSource` (`v1/plugins/plugins.go:848-858`), `RegisterStorageBackend` (`v1/runtime/runtime.go:104`), `WithHooks` (`v1/plugins/plugins.go:234`). Each is documented in `docs/docs/extensions.md:372` but lacks dedicated samples.

## Tradeoffs

- **Explicit contract vs no harness:** Interfaces are crystal-clear and narrow (3 methods on `Plugin`), yet author confidence must be built from reading internal tests rather than running a vendor-provided suite. This raises the cost of correct error-path handling (e.g., forgetting `Reconfigure` may silently drop discovery updates).
- **Global registration simplicity vs test isolation:** `RegisterPlugin` global map enables `go build -o opa++` drop-in pattern (`docs/docs/extensions.md:330`), but pollutes `global` state across tests; `ResetTargetPlugins` exists only for `rego` target plugins (`v1/rego/plugins.go:45`), not for runtime plugins, forcing `test.WithTempFS` + subprocess or build tags for isolation.
- **Rich examples vs missing fixtures:** Documentation ships full copy-pasteable mains; however the lack of `sdk/plugintest` or `topdown/testutil` means authors re-derive boilerplate for `Manager` construction, `storage.NewTransaction`, `logging.Logger`, `tracing.Options`, and `print.Hook` scaffolding.
- **Deprecation flag vs documented timeline:** Machine-readable `deprecated:true` lets `opa fmt/check --strict` fail fast (`v1/ast/compile_test.go:4393`), but without a published removal window authors cannot plan migrations; `CHANGELOG.md:2998` notes removal in OPA 1.0 only retrospectively.
- **Per-query vs global builtins:** Per-query injection is safe for embedding and tests, but global `rego.RegisterBuiltin*` remains the documented path for extending `opa run` (`docs/docs/extensions.md:526-574`), steering most users toward the global, harder-to-test path.

## Failure Modes / Edge Cases

- **Concurrent `RegisterBuiltin` after start** panics (`v1/ast/builtins.go:18-24` comment: map not thread-safe) — no runtime guard.
- **Duplicate target plugin** panics (`v1/rego/plugins.go:39-41` `panic("plugin already registered "+name)`) — no idempotent check.
- **`Plugin.Stop` never called** per spec (`v1/plugins/plugins.go:82` `Currently OPA will not call Stop on plugins`) except via `Manager.Stop` (`v1/plugins/plugins.go:926-947` iterates `toStop[i].Stop(ctx)`); long-running plugin leaks if manager not stopped (e.g., `rego.Rego` embedding without `Manager.Stop`).
- **Missing `Reconfigure`** silently ignores discovery-driven config changes; `Manager.Reconfigure` (`v1/plugins/plugins.go:980-1032`) only updates shared `keys/services/cache` and does not auto-call `plugin.Reconfigure` except via discovery plugin path (`v1/plugins/discovery/discovery_test.go:3800` flow) — custom plugin may stay on stale config.
- **Status `StateNotReady` vs `StateErr` ambiguity:** Doc says `If no status is provided the plugin is assumed to be working OK` (`docs/docs/extensions.md:214`), but `Manager.PluginStatus` snapshot (`v1/plugins/plugins.go:1035`) only shows last `UpdatePluginStatus`; health checker (`v1/runtime/runtime.go:1079` `pluginsReady`) treats any non-`StateOK` as not ready, so transient errors block readiness indefinitely.
- **Built-in arity mismatch** surfaces only at compile time via `BuiltinMap` lookup and runtime via `terms[i].String()` memoization loop (`v1/rego/rego.go:909` `for i < Decl.Arity()`) — out-of-bounds if decl arity mismatches registered func arity.
- **`Validate` returning `any` without type assertion guard** in `Factory.New` example (`docs/docs/extensions.md:296` `config.(Config)`) panics on type mismatch if discovery sends unexpected shape.
- **Capabilities drift:** Adding a builtin without updating `builtin_metadata.json`/`capabilities.json` passes unit tests but breaks `opa build` with `--capabilities` file; no CI guard mentioned beyond `docs/docs/contrib-code.md:30` IR/proto check — builtin metadata lacks equivalent `buf breaking`.

## Future Considerations

- Export a `plugintest` package with `FakeManager`, `ConformanceSuite(t, Factory, validConfig, invalidConfig)` asserting Validate→New→Start→Reconfigure→Stop sequence and status transitions, mirroring `v1/plugins/plugins_test.go:103` logic.
- Export a `builtintest` harness: `harness := builtintest.New(t, decl, impl); harness.Eval("hello(\"bob\") == \"hello, bob\"")` wrapping `topdown.NewQuery(...).WithBuiltins` (`v1/topdown/builtins_test.go:15`) for external authors.
- Document stability tiers for Go extension APIs (e.g., `plugins.Factory` = stable, `topdown.BuiltinContext` fields = experimental) and add `//go:fix` annotated deprecation with removal version, as currently only Rego language has `v0-compatibility.md`.
- Publish breaking-change policy (e.g., `docs/docs/contributing/breaking-changes.md`) committing to changelog + GitHub release notes + `deprecated` flag lead time, and enforce via `CHANGELOG.md` template CI.
- Add `RegisterPlugin` idempotency or `MustRegister` variant plus test helper `ResetRegisteredPlugins` (analogous to `ResetTargetPlugins` `v1/rego/plugins.go:45`) to improve test isolation without global pollution.

## Questions / Gaps

- No evidence of automated extension compatibility gates in CI (e.g., `go test ./.../extended_cases` `build/generate-extended-cases/extended_cases.go:90` generates cases but not as reusable conformance vectors for external repos).
- No fixture for `ExternalRuleSource` (`v1/plugins/plugins_test.go:621` `mockExternalSource` internal) or `storage.Store` mock (`internal/storage/mock`) exported for plugin authors needing data-layer integration.
- Stability of `BuiltinContext` fields (`Metrics, Tracers, RoundTripper` `v1/topdown/builtins.go:37-60`) — are they covered by SemVer? No doc.
- Does `capabilities.json` versioning provide machine-verifiable contract for builtin authors? No test asserts `BuiltinMap` ⊆ `capabilities.json` in this source.
- How are breaking changes communicated beyond changelog — is there a `proposals/` process (`studies/agent-harness-study/sources/opa/proposals` dir exists but no extension-specific RFC template inspected due to isolation to single source)? No proposal reviewed.

---

Generated by `Dimension 21.03: Extension Compatibility Testing` against `opa`.

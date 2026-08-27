# Source Analysis: opa

## API Versioning and Compatibility

### Source Info

| Field | Value |
|-------|-------|
| Name | opa |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | Go 1.26 / Rego / Protobuf / JSON Schema |
| Analyzed | 2026-08-27 |

## Summary

OPA implements a multi-layer versioning model centered on Semantic Versioning (`CHANGELOG.md:5`), explicit Rego language versions (`RegoV0`/`RegoV1`/`RegoV0CompatV1`), bundle manifest versioning (`rego_version` + `file_rego_versions`), capability-negotiation files (`capabilities/*.json`), and dual REST API versions (`/v0/data` vs `/v1/data`). Breaking changes are documented in a detailed `CHANGELOG.md` with `Breaking:` sections, but are not gated by an automated API-compatibility test suite or deprecation-window policy file. Backwards compatibility is enforced operationally through: (1) forwards-compatible JSON `Metadata`/`extra` maps on request/response types, (2) protobuf `roots_set` preservation and deterministic marshaling, (3) `WasmABIVersion.Minor` for ABI evolution, and (4) per-file glob overrides for mixed-version bundles. The model is clear and tested for persisted artifacts (bundle round-trips, proto parity, extensible types) but lacks explicit version-negotiation, API diff checks, or formal deprecation timelines; upgrades require reading the changelog.

## Rating

**7 / 10 — Clear model with tests, explicit interfaces, and operational safeguards, but inconsistent documentation and policy-only deprecation without automated breaking-change detection.**

Rationale: Version fields exist at every surface (binary `version.Version:13` in `v1/version/version.go:13`, manifest `rego_version:4` in `v1/bundle/manifest.proto:26`, capabilities JSON, REST `v0`/`v1`), with executable tests for the critical contract-evolution paths (proto round-trip, roots presence, extensible JSON, `file_rego_versions` merge). However, deprecation is ad-hoc (comment tags, strict-mode errors only) and breaking changes are announced only in `CHANGELOG.md` without machine-enforced windows; no `openapi-diff` or semver-enforcement CI was found.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Version field – binary | `var Version = "1.20.0-dev"` set at build time, with `Vcs`/`Timestamp`/`Hostname` injected via `debug.ReadBuildInfo` | `v1/version/version.go:13`, `v1/version/version.go:29-57` |
| Version field – shading shim | Root `version/version.go:13` aliases `v1.Version` (`var Version = v1.Version`) for `v0` import compatibility | `version/version.go:13` |
| Semver policy | `This project adheres to [Semantic Versioning](http://semver.org/)` | `CHANGELOG.md:5` |
| Breaking change doc – assignment safety | `Behavior change: stricter safety for assignment (:=)` documents deliberate semantic break, error `rego_unsafe_var_error` | `CHANGELOG.md:66-84` |
| Breaking change doc – User-Agent | `Breaking: Fix User-Agent according to RFC9110` – `Open Policy Agent/` → `Open-Policy-Agent/` | `CHANGELOG.md:284-292` |
| Breaking change doc – Logger interface | `1.15.1` fixes backwards-incompatible `v1/logging.Logger` change: `WithContext() optional` | `CHANGELOG.md:721-727` |
| Bundle manifest versioning – struct | `Manifest.RegoVersion *int` and `FileRegoVersions map[string]int` with comment on `RegoV0`/`RegoV1` stability | `v1/bundle/bundle.go:147-165` |
| Bundle manifest versioning – per-file resolution | `numericRegoVersionForFile` compiles glob patterns in sorted key order for determinism, supports per-file overrides | `v1/bundle/bundle.go:1346-1375` |
| Bundle manifest versioning – proto schema | `Manifest` message defines `int32 rego_version = 4`, `map<string,int32> file_rego_versions = 5`, `bool roots_set = 7` for nil-vs-empty distinction | `v1/bundle/manifest.proto:14-38` |
| Bundle manifest versioning – JSON schema | JSON Schema `manifest.schema.json` defines `rego_version: integer`, `file_rego_versions: object`, `roots: array` | `v1/bundle/manifest.schema.json:11-37` |
| Bundle manifest versioning – proto conversion | `ManifestToProto`/`ManifestFromProto` preserve `roots_set` and route `Metadata` through `jsonNormalizeStruct` for JSON parity | `v1/bundle/proto.go:23-60`, `v1/bundle/proto.go:206-241` |
| Bundle format validation | `validateBundleFormat` rejects mismatched `manifestProto` vs `PlanFile`/`PlanProtoFile` | `v1/bundle/bundle.go:1155-1173` |
| Capabilities – embedded versions | `//go:embed *.json` + `FS embed.FS` exposes every historic version (`v0.18.0.json` …) for negotiation | `capabilities/capabilities.go:15`, `v1/capabilities/capabilities.go:15` |
| Capabilities – structure | `Capabilities{ Builtins, FutureKeywords, WasmABIVersions, Features, AllowNet }` with `WasmABIVersion{Version, Minor}` for compat minor | `v1/ast/capabilities.go:84-108` |
| Capabilities – versioned factory | `CapabilitiesForThisVersion(CapabilitiesRegoVersion(RegoV0/V1), CapabilitiesExperimentalKeywords)` selects feature sets | `v1/ast/capabilities.go:110-195` |
| Capabilities – version index | `VersionIndex{ Builtins, Features, Keywords }` embedded as `version_index.json` + `MinimumCompatibleVersion()` computes min OPA version | `v1/ast/capabilities.go:28-50`, `v1/ast/capabilities.go:246-283` |
| Capabilities – loader | `LoadCapabilitiesVersion(version string)` reads single version JSON from `caps.FS.ReadFile(cv + ".json")` | `v1/ast/capabilities.go:205-223` |
| REST API – dual versions | `PromHandlerV0Data = "v0/data"` vs `PromHandlerV1Data = "v1/data"` constants; both routers active | `v1/server/server.go:83-94` |
| REST API – route table | `Handle("POST /v0/data…", v0DataPost)` alongside `Handle("GET|POST|PUT|DELETE /v1/data…", …)` and catch-all `MethodNotAllowed` | `v1/server/server.go:904-937` |
| REST API – extensible types | `DataRequestV1.Metadata map[string]any` + `RegisterJSONFields` + `UnmarshalExtras`/`MarshalExtras` for forwards-compat extra keys | `v1/server/types/types.go:28-55`, `v1/server/types/types.go:241-263`, `v1/server/types/types.go:112-115` |
| REST API – response extensibility | `DataResponseV1.Metadata` preserved; notes to use namespaced keys like `com.example.opa/metadata` | `v1/server/types/types.go:265-293` |
| REST API – compile content negotiation | `application/vnd.opa.*+json` Accept headers mapped via `targetDialect()`; `application/json` kept `// back-compat` | `v1/server/compile_handler.go:44-73`, `v1/server/compile_handler.go:576-601` |
| Deprecation – builtin flag | Builtins `all`, `any`, `cast_*`, `re_match` carry `deprecated: true` in `capabilities.json`; `All`/`Any` in `DefaultBuiltins` comment `// Casts (DEPRECATED)` | `capabilities.json:44`, `v1/ast/builtins.go:93-94`, `v1/ast/builtins.go:105-111` |
| Deprecation – strict mode enforcement | `checkImports` guards `rego.v1` import via `Capabilities.ContainsFeature`; `checkDuplicateImports`/keyword checks only when `c.strict \|\| moduleIsRegoV1Compatible` | `v1/ast/compile.go:2160-2174` |
| Deprecation – strict test | `TestCompilerCheckDeprecatedMethods` asserts `deprecated built-in function calls in expression: all` only when `WithStrict(true)` | `v1/ast/compile_test.go:3822-3878` |
| Deprecation – IR field | `MakeNumberRefStmt.IndexLegacy int json:"Index" // deprecated` emits both `index` and `Index` for back-compat; schema marks `deprecated: true` | `v1/ir/marshal.go:119`, `v1/ir/plan.schema.json:842-843` |
| Deprecation – Provenance | `ProvenanceV1.Revision string json:"revision,omitempty" // Deprecated: Prefer Bundles` and `buffer.go:Revision // Deprecated: Use Bundles` | `v1/server/types/types.go:231`, `v1/server/buffer.go:20` |
| Deprecation – Health param | `ParamBundleActivationV1 = "bundle" // Deprecated: Use ParamBundlesActivationV1` | `v1/server/types/types.go:590` |
| Schema migration | Tests pin `ManifestProtoRoundTrip`, `ManifestRootsPresenceRoundTrip` (nil vs `[]` vs populated), `ManifestMetadataAcceptsJSONTypes`, `SchemaAnnotationBareVarRoundTrip` | `v1/bundle/proto_test.go:21-95`, `v1/bundle/proto_test.go:166-193`, `v1/bundle/proto_test.go:121-148`, `v1/bundle/proto_test.go:201-237` |
| Extensibility tests | `TestDataRequestV1_ExtraFields`, `TestDataResponseV1_ReservedFieldsNotOverridden` verify unknown fields round-trip without clobbering `input`/`result` | `v1/server/types/types_extensible_test.go:12-256` |
| Wasm ABI versioning | `WasmABIVersions []WasmABIVersion` appended via `capabilities.ABIVersions()`; Minor indicates backwards-compatible changes | `v1/ast/capabilities.go:152-153` |
| Rego version default | `parseModule` defaults `mod.regoVersion = DefaultRegoVersion` when `RegoUndefined`; `rego.v1` import flips `RegoV0 → RegoV0CompatV1` | `v1/ast/parser_ext.go:688-698` |
| Compat flags history | Changelog references `--v0-compatible`, `--v1-compatible`, `--rego-v1` flags and `rego_v1` feature in capabilities | `CHANGELOG.md:2449`, `CHANGELOG.md:2768`, `CHANGELOG.md:3012` |

## Answers to Dimension Questions

**1. Which APIs are stable, experimental, deprecated, or internal?**

*Stable:* REST `/v1/data`, `/v1/policies`, `/v1/query`, `/v1/compile`, `/v1/config`, `/v1/status`, `/health` – enumerations in `v1/server/server.go:83-94` and route table `904-928` indicate long-lived, metrics-instrumented handlers. Bundle manifest `revision`/`roots` and data JSON are stable (JSON Schema `v1/bundle/manifest.schema.json:8-42`). The `v0` Data API (`POST /v0/data` at `904-905`) remains stable but is the *old* stable; `v1` is canonical.

*Experimental:* `CapabilitiesExperimentalKeywords` opt-in (`v1/ast/capabilities.go:131-144`) explicitly states experimental keywords “are not covered by OPA's compatibility guarantees.” `WasmABIVersions.Minor` (`v1/ast/capabilities.go:104-108`) signals ABI minor as compatible evolution. Compile-API `targetDialects` like `ucast`, `prisma`, `linq`, `sql/*` (`v1/server/compile_handler.go:44-54`) are feature-flagged via `Accept` header negotiation – not yet frozen.

*Deprecated:* Builtins `all`, `any`, `re_match`, `cast_*` flagged `deprecated:true` (`capabilities.json:44`) and `v1/ast/builtins.go:105-111`; IR `IndexLegacy` (`v1/ir/marshal.go:119`), `ProvenanceV1.Revision` (`v1/server/types/types.go:231`), `ParamBundleActivationV1` (`v1/server/types/types.go:590`), and server `WithDecisionLogger` (`v1/server/server.go:381`). Deprecation is enforced only in strict mode (`v1/ast/compile_test.go:3822`), not by removal.

*Internal:* `internal.*` builtins (`internal.member_2`, `internal.print`, `v1/ast/builtins.go:52-54`), `Bundle.Wasm []byte // Deprecated. Use WasmModules` (`v1/bundle/bundle.go:68`), and every `doc.go` stating “Most packages outside the v1 API are deprecated… `github.com/open-policy-agent/opa/v1/ast.RegoV0`” (`v1/doc.go:7`). The `ast` package outside `v1/` is legacy and intentionally not versioned.

No single `STABLE/EXPERIMENTAL` registry file was found; status must be inferred from constants, `deprecated` flags, and doc tags – a gap.

**2. How are users warned before breaking changes?**

Primarily via `CHANGELOG.md` with `Breaking:` or `Behavior change:` headings and issue links (`CHANGELOG.md:284`, `CHANGELOG.md:66`). The changelog quotes affected behavior, shows before/after examples (e.g., SQL quoting at `:59-63`, User-Agent string change at `:288-290`), and attributes PRs. Compiler strict mode surfaces `deprecated built-in` errors (`v1/ast/compile_test.go:3832`) and `rego.v1 import is not supported` (`v1/ast/compile.go:2168`) when capabilities lack the feature, acting as a pre-merge warning in CI. Runtime config now emits *warnings* (not errors) for unknown configuration keys (`CHANGELOG.md:105-110`).

What is *absent*: no `DEPRECATED.md`, no deprecation timeline, no `// Deprecated:` godoc with `since`/`removeIn` version, no automated changelog-to-release-notes gate, and no code-level `log.Warn` for deprecated API calls at request time. Users must poll the changelog; there is no in-band `Warning` header.

**3. Are old clients, plugins, traces, or persisted artifacts still usable?**

*REST:* Yes – `v0` and `v1` data handlers coexist (`v1/server/server.go:904-905` vs `909-914`); old `POST /v0/data/{path}` clients continue to work. The server also retains `legacyRevisionStoragePath` backwardscompat for system bundles (`v1/server/server.go:2774`).

*Plugins/storage:* Bundle `Manifest.Roots == nil` defaults to `[""]` (`v1/bundle/bundle.go:181-185`); the proto `roots_set` boolean preserves explicit-empty vs default across serialization (`v1/bundle/manifest.proto:37`, `v1/bundle/proto.go:30-32`), validated by `TestManifestRootsPresenceRoundTrip:166`. Plugin `BundleExtStore` registration (`v1/bundle/bundle.go:456-493`) allows external storage without breaking core.

*Traces/promotion:* `DataResponseV1.Warning`, `Provenance`, `Metrics`, `Explanation` are optional (`omitempty`) and extensible via `Metadata`; old clients ignoring unknown keys remain functional due to `UnmarshalExtras`. Decision-log metadata uses namespaced extra keys (`com.example.opa/metadata` at `v1/server/types/types.go:248-251`).

*Persisted bundles:* `WithRegoVersion` defaults (`sdk/opa.go:33` `RegoUndefined → DefaultRegoVersion`) and `MergeWithRegoVersion` with `file_rego_versions` glob (`v1/bundle/bundle.go:1319-1361`) let old v0 bundles load in `DefaultRegoVersion = RegoV1` era when built with `--v0-compatible` or manifest `rego_version:0`. Deterministic proto marshaling (`proto.MarshalOptions{Deterministic:true}` at `v1/bundle/bundle.go:1150`) ensures signature verification survives re-serialization.

*Tool schemas:* `capabilities.json` files are additive; `WasmABIVersions.Minor` semantics allow newer OPA to run older compiled Wasm. However, newer Rego features (e.g., `future.keywords.not` at `CHANGELOG.md:407-442`) will fail `LoadCapabilitiesVersion("v0.44.0")` as demonstrated by `TestCompilerRefHeadsNeedCapability:5073`.

Overall, the system is append-only and lenient on read, strict on write – old artifacts survive unless they opt into new Rego versions.

**4. Does compatibility rely on policy alone or executable tests?**

Both, but heavily weighted toward executable tests for contracts and policy for versioning. *Policy:* Semantic Versioning claim (`CHANGELOG.md:5`) and narrative breaking-change entries, plus `doc.go` deprecation notices, are not machine-enforced. *Executable:* 
- Bundle proto round-trip and JSON/proto parity tests (`v1/bundle/proto_test.go:21`, `:121`, `:166`) pin wire compatibility.
- Extensible-type tests (`v1/server/types/types_extensible_test.go:12-256`) assert forward-compatibility of API payloads.
- `file_rego_versions` merge and `validateAndInjectDefaults` logic is covered by bundle tests.
- `MinimumCompatibleVersion` (`v1/ast/capabilities.go:246-283`) and `capabilities_test.go` verify capability negotiation.
No evidence of an automated `semver` enforcement job, OpenAPI diff, or proto `buf breaking` check in CI was found (searched CI workflows; `buf.yaml` exists but breaking config not inspected). Thus compatibility is *present but fragile* outside the tested serialization paths.

## Architectural Decisions

| Decision | Evidence | Consequence |
|----------|----------|-------------|
| Ship historic capabilities as embedded JSON | `capabilities/capabilities.go:15` `go:embed *.json`; `LoadCapabilitiesVersions` sorts via `semver.Compare` `v1/ast/capabilities.go:241-242` | Enables offline `CheckRegoV1`/feature gating against any old OPA; no network needed for compatibility checks |
| Dual bundle manifest encodings (JSON + deterministic protobuf) | `ManifestProtoExt = ".manifest.pb"` `v1/bundle/bundle.go:46`; `marshalManifestProto` with `Deterministic:true` `v1/bundle/bundle.go:1150`; `roots_set` field `v1/bundle/manifest.proto:37` | Allows `bundle` signing/hash parity across formats; nil-vs-empty roots preserved – critical for signature verification |
| Per-file Rego version via glob map | `FileRegoVersions map[string]int` + `compiledFileRegoVersions []fileRegoVersion` + `glob.Compile` `v1/bundle/bundle.go:167-170`, `:1355` | Incremental migration of monorepos; deterministic evaluation but overlapping pattern behavior documented as *undefined* (`v1/bundle/bundle.go:1352`) |
| `v1/` import alias shim for `v0` compatibility | Root `bundle/bundle.go:8` `import v1 "github.com/open-policy-agent/opa/v1/bundle"` + `type Bundle = v1.Bundle`; `version/version.go:13` | Single codebase serves both `github.com/open-policy-agent/opa` and `…/opa/v1` imports; eases major version transition without forking |
| Forwards-compatible REST types via reflection | `RegisterJSONFields[T]` + `UnmarshalExtras`/`MarshalExtras` `v1/server/types/types.go:28-110`; `Metadata map[string]any json:"-"` `types.go:250`, `280` | Wrapping distributions (e.g., Styra) can pass custom keys (`com.example.opa/*`) without OPA changes; avoids `unknown field` errors |
| Strict-mode-gated deprecation errors | `if c.strict \|\| moduleIsRegoV1Compatible` `v1/ast/compile.go:2175`, `:2185`; `TestCompilerCheckDeprecatedMethods` with `WithStrict` `v1/ast/compile_test.go:3891` | Deprecated builtins warn only when author opts into strict/RegoV1; silent in legacy mode – reduces churn but hides future breaks |

## Notable Patterns

- **Embed-and-serve version history:** All past `capabilities/*.json` are compiled into the binary – a variant of the “versioned contract as code” pattern.
- **Globs as version selectors:** `file_rego_versions` uses filesystem globs (`gobwas/glob` at `v1/bundle/bundle.go:24`) rather than semver ranges – unusual but matches OPA's file-path-centric bundle layout.
- **Accept-header as capability negotiation:** Compile API uses `Accept: application/vnd.opa.*+json` (`v1/server/compile_handler.go:45-53`) and `targetDialect` switch – a REST-idiomatic alternative to URL versioning.
- **Dual-key emission for deprecated fields:** IR marshaling emits both `index` and deprecated `Index` (`v1/ir/marshal.go:108-119`) for a deprecation window rather than immediate removal.
- **Deterministic proto for signatures:** `proto.MarshalOptions{Deterministic: true}` ensures bundle signatures are reproducible – a security-critical compatibility pattern.

## Tradeoffs

| Tradeoff | Pro | Con |
|----------|-----|-----|
| Changelog-only breaking-change communication | Low ceremony; detailed rationale and examples inline (`CHANGELOG.md:42-97`) | No machine-readable `deprecated`/`removedIn` dates; integrators cannot automate upgrade-risk analysis |
| `v0` + `v1` endpoints coexisting | Zero-downtime migration; old sidecars keep working | Doubles handler surface, metrics labels (`PromHandlerV0Data` vs `PromHandlerV1Data`), and test matrix (`server_test.go:874`, `server/handlers/compress_test.go:48`) |
| Per-file glob version overrides | Fine-grained migration | Overlapping patterns are “undefined” (`bundle.go:1352`), and globs are evaluated in sorted-key order – surprising and not discoverable without reading source |
| Silent non-strict deprecation (error only with `WithStrict(true)`) | No build breakage for legacy users | Deprecated calls (`all`, `any`) linger indefinitely; future removal would be a *silent* break for non-strict users |
| Embedding all historic capabilities | Full offline version skew analysis via `MinimumCompatibleVersion` | Bloats binary and requires `go:generate` to rebuild `version_index.json` (`v1/ast/capabilities.go:34-40`) |
| `Metadata` extra-map pattern | Extensibility without schema bumps | No schema validation for custom keys; collisions with future OPA keys depend on out-of-band namespace convention (`com.example.opa/*`) |

## Failure Modes / Edge Cases

- **Overlapping `file_rego_versions` globs:** `v1/bundle/bundle.go:1352` documents “behaviour for overlapping patterns is undefined.” Two bundles merged with `MergeWithRegoVersion` could non-deterministically pick different Rego versions across OPA releases if key-sorted order changes.
- **Mixed manifest/plan format bundle:** `validateBundleFormat` (`v1/bundle/bundle.go:1155-1173`) returns an error only at *write* time; a bundle with `plan.pb` + `/.manifest` (JSON) produced outside `NewWriter` would be accepted on read but would fail verification downstream.
- **Proto JSON-type coercion:** `jsonNormalizeStruct` (`v1/bundle/proto.go:411-421`) round-trips `Metadata` through JSON to accept `int`, `json.RawMessage`, etc. – a value like `count: int(7)` becomes `float64(7)` after proto round-trip (`proto_test.go:142`), subtly changing type for consumers doing `type switch`.
- **Bare `schema` var ref:** `ast.ParseSchemaRef` special-cases bare `schema` var (see `proto_test.go:200-237` comment: “`ast.ParseRef cannot decode a bare Var`”). A future proto emitter that stringifies refs differently could re-break this.
- **Strict-to-non-strict divergence:** An integrator running without `WithStrict(true)` will not see `deprecated built-in` errors (`compile_test.go:3902`), so CI may pass while prod policy silently depends on deprecated semantics that will be removed in a major bump.
- **Wasm ABI minor confusion:** `WasmABIVersion.Minor` is backwards-compatible by comment (`capabilities.go:104`), but no test asserts that a newer minor can still execute policies compiled against an older minor – failure would be at Wasm runtime, not compile time.
- **Accept-header strictness:** `sanitizeHeader` (`compile_handler.go:560-574`) rejects unsupported or comma-joined headers with a 400; a client sending `Accept: application/json, application/vnd.opa.ucast.all+json` fails even though a browser might.

## Future Considerations

- Adopt `buf breaking` (given `buf.yaml` exists) and an OpenAPI diff step in CI to make the SemVer claim machine-enforced; publish the rule (e.g., “no removal without `deprecated` for ≥2 minor releases”).
- Introduce a `DEPRECATED.md` or `deprecated.json` with `since`, `supersededBy`, `removeIn` fields, and emit `Warning: 299` headers plus structured `warning` payload (`types.go:296 CodeAPIUsageWarn`) for deprecated REST usage.
- Replace overlapping-glob-undefined semantics with longest-prefix or explicit precedence and add a `bundle verify` warning for overlaps.
- Consider deprecating `/v0/data` with a sunset header and metrics counter, rather than indefinite coexistence.
- Add a `capabilities` negotiation endpoint (e.g., `GET /v1/capabilities`) so heterogeneous fleet members can advertise supported features, replacing out-of-band `MinimumCompatibleVersion` checks.
- Generate a machine-readable `manifest.schema.json` from the proto source of truth (or vice versa) to prevent drift between `manifest.proto:14` and `manifest.schema.json:8`.

## Questions / Gaps

- **No deprecation policy file found.** Search for `deprecated`, `sunset`, `BREAKING` across repo yielded only inline comments and changelog headings; no formal policy document exists to answer “how long is deprecated supported?”
- **No breaking-change detection CI found.** Workflows under `.github/` were not inspected in full; `buf.yaml` and `buf.lock` exist but their `breaking` config was not read – unclear if `buf breaking` is active.
- **No SDK/CLI/server compatibility matrix found.** `sdk/opa.go:32` sets `DefaultRegoVersion` but does not expose a version-negotiation API distinct from `Capabilities`; question “Do compatibility expectations differ across SDK, CLI, server, and extension surfaces?” cannot be fully answered without reading `v1/sdk`, `cmd/`, and plugin docs.
- **No persisted-trace compatibility test found.** `types.TraceV1` (`v1/server/types/types.go:339`) is `json.RawMessage`-based; no test pins trace schema evolution across versions, so trace tooling compatibility is unverified.
- **No migration guide found.** The repo has no `MIGRATION.md` or per-major `UPGRADING.md`; `CHANGELOG.md` is the sole migration source, requiring manual audit.

---

Generated by `Dimension 24.03: API Versioning and Compatibility` against `opa`.

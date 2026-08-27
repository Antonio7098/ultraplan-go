# Source Analysis: temporal

## 21.03 Extension Compatibility Testing

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go 1.26 / gRPC / protobuf / Uber Fx |
| Analyzed | 2026-08-27 |

## Summary

Temporal exposes three primary extension surfaces: (1) CHASM libraries (`Library`/`RegistrableComponent`/`RegistrableTask`), (2) archival plugins (`HistoryArchiver`/`VisibilityArchiver` with `CustomHistoryArchiverFactory`/`CustomVisibilityArchiverFactory`), and (3) auth plugins (`Authorizer`/`ClaimMapper`/`AudienceMapper`). The contracts are defined as Go interfaces with `go:generate mockgen` mocks, and forward compatibility is encouraged via `UnimplementedComponent`/`UnimplementedLibrary` embeddings. No dedicated conformance test suite exists that external authors can import to verify an implementation exhaustively. The closest mechanisms are `Registry.Register` runtime validation, the in-memory `chasmtest.Engine` fixture, per-implementation archival unit tests, and the `tests/archival_test.go:78-114` custom-archiver integration demonstration. Examples are present in `common/archiver/README.md` and `chasm/lib/tests/library.go`. Stability is governed implicitly by proto `buf-breaking` CI ( `Makefile:17` / `develop/buf-breaking.sh:63` ) and the unimplemented-embedding pattern, not by an explicit extension-stability policy or versioned contract document. Breaking-change communication relies on release tags and proto linting, with no `CHANGELOG.md` or `BREAKING_CHANGES.md` in-repo.

## Rating

**Score: 5 / 10 — Present but inconsistent, weakly documented, or fragile**

Rationale: Interfaces are explicit and mocked, runtime registry validation is strong, and two partial fixtures (`chasmtest.Engine`, archival custom-factory) exist with documented examples. However, there is no importable conformance harness (no `archiver/conformance_suite.go` or `chasm/conformance` package), no test fixture published for `Authorizer`/`ClaimMapper`, no stability/SLA document for extensions, and breaking-change detection covers only protos ( `proto/internal`, `chasm/lib` ), not Go interface evolution. An external author cannot `go test` a compliance matrix without copying internal suite code.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Archival contract | `HistoryArchiver` interface with `Archive`/`Get`/`ValidateURI` | `studies/agent-harness-study/sources/temporal/common/archiver/interface.go:44-59` |
| Archival contract | `VisibilityArchiver` interface with `Archive`/`Query`/`ValidateURI` | `studies/agent-harness-study/sources/temporal/common/archiver/interface.go:76-94` |
| Archival contract | `mockgen` directives for archivers (evidence interfaces are intended to be mocked/extended) | `studies/agent-harness-study/sources/temporal/common/archiver/interface.go:1` |
| Archival extension | `CustomHistoryArchiverFactory` / `CustomVisibilityArchiverFactory` interfaces and functional adapters | `studies/agent-harness-study/sources/temporal/common/archiver/provider/provider.go:54-67` |
| Archival extension | `NewCustom*Params` structs exposing `Scheme`, `ExecutionManager`, `Logger`, `MetricsHandler`, `Configs map[string]any` | `studies/agent-harness-study/sources/temporal/common/archiver/provider/provider.go:35-49` |
| Archival extension | Provider first consults custom factory, falls back to built-ins via `ErrUnknownScheme` (override capability) | `studies/agent-harness-study/sources/temporal/common/archiver/provider/provider.go:131-171` |
| Archival extension | Server options `WithCustomHistoryArchiverFactory` / `WithCustomVisibilityArchiverFactory` injection | `studies/agent-harness-study/sources/temporal/temporal/server_option.go:161-171` |
| Archival example | README Option 2 external implementation walkthrough with factory code | `studies/agent-harness-study/sources/temporal/common/archiver/README.md:86-193` |
| Archival example | Functional integration test using `customHistoryArchiver` / `customVisibilityArchiver` counting `Archive` calls | `studies/agent-harness-study/sources/temporal/tests/archival_test.go:78-114` |
| Archival example | Custom scheme `customtest` exercised via `TestCustomArchiver` asserting factory was called | `studies/agent-harness-study/sources/temporal/tests/archival_test.go:297-322` |
| Archival tests | Per-implementation conformance-like suites (exit per store) — `historyArchiverSuite` filestore | `studies/agent-harness-study/sources/temporal/common/archiver/filestore/history_archiver_test.go:58-107` |
| Archival tests | Equivalent S3 / GCloud archiver tests (pattern repeated, not abstracted into shared harness) | `studies/agent-harness-study/sources/temporal/common/archiver/s3store/history_archiver_test.go:1` , `common/archiver/gcloud/history_archiver_test.go:1` |
| Archival provider test | Provider suite validates custom factory params forwarding and caching, not full archiver behavior | `studies/agent-harness-study/sources/temporal/common/archiver/provider/provider_test.go:89-124` |
| CHASM contract | `Library` interface (`Name`, `Components`, `Tasks`, `RegisterServices`, `NexusServices`, `NexusServiceProcessors`, `mustEmbedUnimplementedLibrary`) | `studies/agent-harness-study/sources/temporal/chasm/library.go:11-23` |
| CHASM contract | `Component` / `TerminableComponent` / `RootComponent` interfaces | `studies/agent-harness-study/sources/temporal/chasm/component.go:13-52` |
| CHASM contract | Forward-compat via `UnimplementedLibrary` / `UnimplementedComponent` with `mustEmbed*` private method | `studies/agent-harness-study/sources/temporal/chasm/library.go:25-52` , `chasm/component.go:54-57` |
| CHASM contract | `mockgen` for `Library` and `Component` (authoring support but not conformance) | `studies/agent-harness-study/sources/temporal/chasm/library.go:1` , `chasm/component.go:1` |
| CHASM registry validation | `Registry.Register` validates name regex, duplicate lib, duplicate FQN, ID collision, struct kind, GoType collision, context key conflict, visibility alias | `studies/agent-harness-study/sources/temporal/chasm/registry.go:69-102` , `chasm/registry.go:234-284` , `chasm/registry.go:354-366` |
| CHASM fixture | Lightweight in-memory `chasmtest.Engine` implementing full `chasm.Engine` for unit tests, with `NotFound` semantics and idempotency checks | `studies/agent-harness-study/sources/temporal/chasm/chasmtest/test_engine.go:30-45` , `chasm/chasmtest/test_engine.go:130-166` |
| CHASM fixture | `WithTimeSource` option and poll-notify synchronisation mimicking production engine | `studies/agent-harness-study/sources/temporal/chasm/chasmtest/test_engine.go:70-79` , `chasm/chasmtest/test_engine.go:270-329` |
| CHASM fixture | Export helpers exposing registry internals for tests: `Component`, `Task`, `ComponentFor` | `studies/agent-harness-study/sources/temporal/chasm/export_test.go:8-36` |
| CHASM example library | Canonical example `tests.Library` registering `PayloadStore` component with `BusinessIDAlias`, `SearchAttributes`, `ContextValues` and two tasks | `studies/agent-harness-study/sources/temporal/chasm/lib/tests/library.go:24-67` |
| CHASM examples | Production libraries: `CoreLibrary` (`vis`), `Scheduler`, `Callback`, `Activity`, `Workflow`, `NexusOperation` | `studies/agent-harness-study/sources/temporal/chasm/library_core.go:4-19` , `chasm/scheduler.go:1-12` , `chasm/lib/callback/library.go:10` |
| Auth contract | `Authorizer` interface `Authorize(ctx, caller, target) (Result, error)` | `studies/agent-harness-study/sources/temporal/common/authorization/authorizer.go:54-55` |
| Auth contract | `ClaimMapper` interface `GetClaims(authInfo) (*Claims, error)` | `studies/agent-harness-study/sources/temporal/common/authorization/claim_mapper.go:29-31` |
| Auth contract | `AudienceMapper` / `TokenKeyProvider` interfaces (also pluggable) | `studies/agent-harness-study/sources/temporal/common/authorization/audience_mapper.go:1` , `common/authorization/token_key_provider.go:1` |
| Auth selection | `GetAuthorizerFromConfig` / `GetClaimMapperFromConfig` only allow `""` → noop or `"default"` → built-in; unknown string → error (no generic factory) | `studies/agent-harness-study/sources/temporal/common/authorization/authorizer.go:64-72` , `claim_mapper.go:80-88` |
| Auth mock fixture | `mockgen` for `Authorizer`, `ClaimMapper`, `AudienceMapper` provide test doubles but no behavior suite | `studies/agent-harness-study/sources/temporal/common/authorization/authorizer_mock.go:1` , `claim_mapper_mock.go:1` |
| Persistence / generic store | `common/persistence/tests` contains test suites (`HistoryEventsSuite`, `VisibilityPersistenceSuite`, `QueueV2TestSuite`) used for persistence implementation conformance, not exposed to external archiver authors | `studies/agent-harness-study/sources/temporal/common/persistence/tests/history_store.go:29-51` , `visibility_persistence_suite.go:1` |
| Stability / proto breaking | Makefile `buf-breaking` dependency in `ci-build-misc` | `studies/agent-harness-study/sources/temporal/Makefile:17` |
| Stability / proto breaking | Script that compares `image.bin`/`chasm.bin` against PR merge-base and main via `buf breaking` | `studies/agent-harness-study/sources/temporal/develop/buf-breaking.sh:63-73` |
| Stability / proto breaking | Lint config for `proto/internal` and `chasm/lib` via `buf lint` | `studies/agent-harness-study/sources/temporal/Makefile:434-437` |
| Stability / authoring docs | Testing guide describes unit vs integration, `test_dep` tags, but not extension stability | `studies/agent-harness-study/sources/temporal/docs/development/testing.md:1` |
| Stability / CHASM doc | CHASM architecture doc describes `ExecutionKey`, `ComponentPath`, `ComponentRef` VT guards but no version guarantee | `studies/agent-harness-study/sources/temporal/docs/architecture/chasm.md:129-322` |
| Versioning | `go.mod` retracts `v1.30.0`/`v1.26.1`/`v1.26.0`—implies version discipline but no extension-compatibility policy file | `studies/agent-harness-study/sources/temporal/go.mod:5-9` |
| Missing | No `common/archiver/conformance_test.go` or `archiver/provider/conformance` harness found | searched `common/archiver/**/*` — no such file |
| Missing | No `CHANGELOG.md`, `BREAKING_CHANGES.md`, or `docs/architecture/stability.md` in repo | `docs/**/*.md` glob — none matches stability; root glob — no CHANGELOG |
| Missing | No auth-specific conformance suite or fixture package under `common/authorization` | directory listing shows only `*_test.go` / `*_mock.go`, no `suite` or `fixture` |

## Answers to Dimension Questions

**1. Are extension contracts tested?**
Partially. Each contract has adjacent test coverage but not a dedicated importable conformance suite.

* Archival: every built-in store duplicates a suite (`filestore/history_archiver_test.go:58` validates `ValidateURI`, `Archive` invalid URI/request, iterator errors, mutated history, `Get` paging, etc.; same for `s3store`, `gcloud`). This validates behavior expectations informally, but is not factored into a shared `ArchiverConformanceSuite` an external author can run (`No evidence found` for a shared harness in `common/archiver/provider`). Provider tests (`provider/provider_test.go:89-124`) test only factory wiring/caching/config forwarding, not archiver semantics.
* CHASM: `Registry.Register` is exercised thoroughly (`chasm/registry_test.go:51-93` checks name validation, duplicate FQN/ID, collision). Runtime validation on registration does enforce structural conformance (`chasm/registry.go:237-284`). The `chasmtest.Engine` satisfies engine conformance implicitly but no `LibraryConformanceSuite` loops over a library's components. Integration tests in `tests/archival_test.go:297` + `chasm/*_test.go` act as de facto conformance but are not published as a library.
* Auth: only normative interface + defaults tested (`default_authorizer_test.go`, `default_jwt_claim_mapper_test.go`, `interceptor_test.go`). No auth extension conformance harness.

**2. Are fixtures provided for extension authors?**
Fragmented.

* For CHASM: **yes — the best fixture in the repo**. `chasm/chasmtest/test_engine.go:88` provides `NewEngine(t, registry)` that faithfully implements conflict/reuse policies, request-ID dedup (`chasm/chasmtest/test_engine.go:240-247`), `PollComponent` blocking (`270-329`), and is used by `chasm/lib/tests/fx.go:13` to register example library. `export_test.go` exposes introspection helpers.
* For archival: **partial**. `interface_mock.go` / `provider_mock.go` / `filestore/gcloud/s3store` provide mocks but no one-line harness. Authors must vendor `archiver.URI`, `HistoryArchiver`, and replicate `historyArchiverSuite::newTestHistoryArchiver` pattern (`filestore/history_archiver_test.go:557-564`). No `archiverstest.NewHarness(t)` helper.
* For auth: **minimal**. Only mocks (`authorizer_mock.go`, `claim_mapper_mock.go`). No `FakeClaims` builder or `AuthorizerTestSuite` with matrix of `CallTarget` combos.

**3. Are examples provided?**
Yes — the strongest pillar.

* Archival external path fully documented in `common/archiver/README.md:86-193` with copy-paste `CustomHistoryArchiverFactoryFunc`/`CustomVisibilityArchiverFactoryFunc` examples and YAML `customStores` config. Mini-example implementations `customHistoryArchiver`/`customVisibilityArchiver` live in `tests/archival_test.go:78-114`.
* CHASM examples are live: `chasm/lib/tests/library.go:24-67` is the canonical minimal library; `chasm/lib/tests/fx.go:13` shows Fx wiring; production references `chasm/lib/activity`, `chasm/lib/workflow`, `chasm/lib/callback`, `chasm/lib/scheduler` illustrate idioms. `chasm/test_library_test.go:8-58` is an additional test-library example.
* Auth examples are limited to `default_authorizer.go` / `default_jwt_claim_mapper.go` implementations themselves—there is no `examples/custom-authorizer` directory.

**4. Are stability guarantees documented?**
No, for extensions; yes for protos only.

* No file states “`HistoryArchiver` is stable since vX; method `<X>` added in vY”. `UnimplementedComponent` comment in `chasm/component.go:54` says “Embed UnimplementedComponent to get forward compatibility” and `library.go:25` mirrors it — the only explicit stability hint, relying on Go embedding to absorb new interface methods. `Registry.validateName` / ID hashing protects wire compatibility indirectly.
* Proto stability is formal: `Makefile:479-482` runs `buf-breaking` against PR merge-base; `develop/buf-breaking.sh:63-73` runs `buf breaking` for `proto/internal` and `chasm/lib`. `proto/internal/buf.yaml` / `chasm/lib/buf.yaml` configure `BREAKING` rules. This guarantees API proto compatibility, not Go archiver-interface compatibility.
* No `CHANGELOG.md` at root (confirmed absent), no `docs/architecture/stability.md`, no semver policy doc. `go.mod:5-9` retracts bad versions but does not promise extension compatibility across minors. Auth config dispatch (`authorizer.go:64-72` accepting only `""`/`"default"`) signals intentional closed extension via config string, but is not documented as a stability pledge.

## Architectural Decisions

| Decision | Location | Consequence for Compatibility |
|----------|----------|------------------------------|
| Factory-fallback provider with `ErrUnknownScheme` | `common/archiver/provider/provider.go:131-171` | Allows clean override of any scheme, including built-ins, without code changes to provider; mismatch between `Configs` source (`customStores` vs built-in sections) is a documented gap (`README.md:263`). |
| `UnimplementedComponent`/`UnimplementedLibrary` embedding with private `mustEmbed*` | `chasm/component.go:54-57`, `chasm/library.go:25-52` | Adds interface methods without breaking existing implementations; callers must not implement `mustEmbed*` manually—enforces forward compat via compile-time trap. |
| `Registry` strong validation (name regex, FQN/ID collision, GoType uniqueness, struct-kind check) | `chasm/registry.go:234-284` | Catches archetype collision early; moves conformance from tests into runtime, so misbehaving extension fails fast on startup. |
| In-memory `chasmtest.Engine` mirroring production semantics | `chasm/chasmtest/test_engine.go:30-45` | Lets library authors run without a full cluster, but couples fixture to internal `persistencespb.WorkflowExecutionState` mocking (`520-596`). |
| String-dispatch for auth (`config.Authorization.Authorizer == "default"`) | `common/authorization/authorizer.go:64` , `claim_mapper.go:80` | Intentionally narrow extension surface; external authors must replace via Fx override, not config—undocumented injection point. |
| Proto breaking via `buf` covering `proto/internal` + `chasm/lib` only | `Makefile:479-482`, `develop/buf-breaking.sh:54-73` | Shields public API protos; does not shield Go interfaces like `HistoryArchiver`. |

## Notable Patterns

* **Functional adapter for factories**: `type CustomHistoryArchiverFactoryFunc func(NewCustomHistoryArchiverParams) (HistoryArchiver, error)` with method `NewCustomHistoryArchiver` (`provider.go:65-92`) lets consumers write lambdas instead of structs — lowers boilerplate (`archival_test.go:130-141`).
* **Caching after first Get**: provider memoizes per-scheme archivers after creation (`provider.go:177-183`, `239-245`) — extension instantiated once; concurrent access tested (`provider_test.go:266-309`).
* **Upfront name validator**: `nameValidator = regexp.MustCompile(^[A-Za-z_][A-Za-z0-9_]*$)` (`registry.go:18`) shared by library/component/task names; single source of truth.
* **Payload + VT guards for callbacks**: `docs/architecture/chasm.md:303-322` and `chasm/nexus_completion.go:20-67` encode versioned-transition checks to reject stale `ComponentRef` callbacks — a stability mechanism for distributed extension state.
* **Mock generation as contract documentation**: every extensible interface carries `//go:generate mockgen` (`interface.go:1`, `authorizer.go:1`, `claim_mapper.go:1`) — implies expectation to mock in consumer tests.

## Tradeoffs

* **Runtime vs test-time conformance**: `Registry.Register` validates structural conformance at startup (`registry.go:69`), reducing need for a separate conformance binary but delaying feedback to `go run` rather than `go test`.
* **Three independent archival suites vs one shared harness**: duplicating `TestArchive_Fail_InvalidURI` per store proves each satisfies spec but multiplies maintenance; factoring a harness would let external stores import it.
* **Fixture fidelity vs simplicity**: `chasmtest.Engine` faithfully reproduces business-ID reuse/conflict policies and `PollComponent` long-poll semantics (`test_engine.go:155-434`), at cost of manually maintained mock backend state (`synctest`-style `MockNodeBackend:520-579`) that can drift from production `service/history/chasm_engine.go`.
* **Unimplemented embedding**: gains additive evolution cheaply, but if a core method semantics change (e.g., adding required return value) the no-op default in `UnimplementedLibrary` silently satisfies compilation and can hide semantic mismatches.
* **Auth closed dispatch**: hard-coded `"default"` vs `""` switch (`authorizer.go:66`) keeps surface minimal and avoids scheme explosion, but forces non-default extensions to bypass documented config path and use lower-level Fx wiring.

## Failure Modes / Edge Cases

* **Archiver interop gaps**: `Archive` may be retried by caller; progress recording via `ArchiveOption` (`README.md:200-229`) is opt-in. An extension that ignores `GetFeatureCatalog(opts...).ProgressManager` will redo work or double-write. No conformance test asserts retry/idempotency handling.
* **Duplicate `ArchetypeID`**: two libraries registering `FullyQualifiedName("lib","Comp")` that hash to same `uint32` via `GenerateTypeID` triggers `component ID %d collision` at `registry.go:255`. External author may hit this without deterministic collision test.
* **Context key conflict**: `Registry.registerComponent:259-262` rejects if two components register same `any` key in `WithContextValues`. No ahead-of-time linter; manifest failure at register.
* **Payload-store visibility requirement**: `validateVisibilityBusinessIDAlias:348-352` — forgetting `WithBusinessIDAlias` on a component containing `Field[*Visibility]` returns cryptic error. No compile-time guard.
* **Custom scheme shadowing built-in**: injecting `filestore` scheme via custom factory (`provider.go:131`) silently overrides built-in; `Configs` then comes from `customStores.filestore` (`README.md:263`) while built-in config section is ignored — easy misconfiguration with no warning.
* **Proto non-breaking Go break**: `buf-breaking` passes, but adding a method to `HistoryArchiver` is Go-breaking for external implementors even though it is not proto-breaking. No CI step checks Go interface diff.
* **Auth extension silent fallback**: typo in `config.Authorization.Authorizer` yields `unknown authorizer` error (`authorizer.go:71`), but mis-wiring `ClaimMapper` could fall back to `noopClaimMapper` (system admin) if not validated — test gap.
* **Poll timeout confusion**: `chasmtest.Engine.PollComponent:325` returns `(nil,nil)` on `ctx.Done()` to emulate long-poll; external test that asserts non-nil error on timeout will be flaky.

## Future Considerations

* **Publish an importable conformance harness**: Extract archival checks from `filestore/history_archiver_test.go:83-506` into `common/archiver/archivertest.Suite` or `provider/archivertest` that external modules import with `go test -run TestArchiverConformance`. Structure after `common/persistence/tests/*.go` but decoupled from cluster bring-up.
* **CHASM library conformance helper**: Add `chasm/chasmtest.LibraryConformanceSuite` that registers a library, asserts FQN set, exercises `Tasks()` handlers via `test_engine.go`, and validates search-attribute wiring.
* **Auth fixtures package**: Expose `authorization/authorizertest` with `FakeAuthInfo`, canned `CallTarget` matrix (e.g., `StartWorkflowExecution`, `Nexus callback`, system-internal), and golden `Decision` expectations.
* **Versioned interface documentation**: Add `docs/development/extensions.md` or `docs/architecture/extension-contracts.md` declaring per-interface stability tier (`HistoryArchiver: beta`, `Library: alpha`), semver rule (minor may add methods; majors break), and embedding requirement.
* **Go interface breaking check**: complement `buf-breaking` with `go-apidiff` or `jech/breakcheck` on `common/archiver`, `common/authorization`, `chasm` packages in `ci-build-misc`.
* **Breaking-change log**: reintroduce `CHANGELOG.md` (currently absent) with `Breaking Changes` section per release, or consume GitHub releases feed into docs; link from `temporal/server_option.go:161` commentary.
* **Fail-loud on `customStores` vs built-in mismatch**: `provider.go:135-143` could log warning when custom factory handles a scheme that also has built-in config, or when built-in config is set but ignored due to override.

## Questions / Gaps

* `common/persistence/DataStore` / `client.AbstractDataStoreFactory` appear extensible but no search evidence shows them being treated as a supported third-party extension — are they considered internal only? (no provider comparable to `ArchiverProvider`).
* `chasm/component_mock.go` / `library_mock.go` suggest third-party component mocking, but no guidance exists on whether mocking `Component` is supported vs implementing real type — inferred intent, not documented.
* No evidence of extension version negotiation (capability advertisement) comparable to `common/headers.version_checker.go` which exists for SDK version, not for archiver/library version.
* Stability window for CHASM lib protos: `chasm/lib/.../proto/v1/*.proto` are covered by `buf lint` (`Makefile:437`) but `buf-breaking.sh:68-73` handles `CHASM_BINPB` differently from `INTERNAL_BINPB` — does breaking on CHASM protos block PRs? Not visible in `.github/workflows` without cross-source access.
* How are breaking Go-interface changes announced today? Absence of `CHANGELOG.md` and of `BREAKING_CHANGES.md` suggests GitHub Releases are canonical, but this is unverified (source isolation prohibits inspecting .github/workflows release notes generation).

---

Generated by `21.03-extension-compatibility-testing` against `temporal`.

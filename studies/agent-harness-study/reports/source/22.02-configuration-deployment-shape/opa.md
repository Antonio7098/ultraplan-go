# Source Analysis: opa

## Dimension 22.02: Configuration and Deployment Shape

### Source Info

| Field | Value |
|-------|-------|
| Name | opa |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | Go (single static binary), Rego policy language, spf13/cobra CLI, Docker/Kubernetes deployment, embedded SDK for Go embedders |
| Analyzed | 2026-08-25 |

## Summary

OPA's configuration model is a layered, single-binary design. One binary (`main.go:1`, command tree in `cmd/commands.go:25-52`) serves all modes — interactive REPL, HTTP server, one-shot evaluator, and an embeddable Go SDK — with mode selection driven purely by flags and config (`cmd/run.go:396-401`). Configuration is assembled in layers: a YAML/JSON config file with `${VAR}` environment interpolation (`internal/config/config.go:80-97,128-157`), deep-merged `--set` / `--set-file` CLI overrides that win over the file (`internal/config/config.go:99-126,160-184`; flags at `cmd/flags.go:30-36`), plus a generic env-var-to-flag mapping so any flag can be set as `OPA_<COMMAND>_<FLAG>` (`cmd/internal/env/env.go:25-50`). The merged config is then validated by an embedded **Rego** policy that injects defaults and warns on unknown keys (`v1/config/validate.go:19-27`, `v1/config/validate.rego:25-60`), with per-plugin validation injecting defaults downstream (e.g. `v1/download/config.go:43-87`). There is no first-class "environment" concept (dev/staging/prod); parity is achieved by the same binary + different config files, and runtime reconfiguration is delegated to the discovery plugin, which can rewrite most of the running config from a downloaded bundle while protecting boot-critical sections from change (`v1/plugins/discovery/discovery.go:466-545`). Feature flags are not a formal system: features are toggled by build tags (`opa_wasm`, `opa_no_oci`), by import-for-side-effect registration packages (`features/wasm/wasm.go:16-18`), by presence of config keys (`v1/config/config.go:243-258`), and a handful of ad-hoc environment variables.

## Rating

**8/10.**

Rationale against the rubric:

- Clear, tested layering model with explicit precedence (`internal/config/config_test.go:310-495` covers file+override combinations; `TestMergeValues*` at `internal/config/config_test.go:134-308`).
- Unusual and strong validation story: OPA validates its own configuration with an embedded Rego policy (`v1/config/validate.rego:1-165`), injects defaults there (`v1/config/validate.rego:25-33`), warns on unknown options at any depth (`v1/config/validate.rego:46-60`), and keeps the schema extensible via `RegisterConfigSpec`/`SpecsFromStruct` (`v1/config/spec.go:32-47`) so plugin packages register their own key schemas (`internal/metricsexport/metricsexport.go:51`, `v1/plugins/server/decoding/config.go:37`).
- Operational safeguards: active-config redaction of credentials and crypto keys before exposure via `GET /v1/config` (`v1/config/config.go:299-326`, `v1/server/server.go:2466-2474`), discovery reconfiguration guardrails (`v1/plugins/discovery/discovery.go:491-526`), readiness gating for orchestrated deployments (`v1/runtime/runtime.go:260-263,790-795`), graceful shutdown knobs (`v1/runtime/runtime.go:1032-1071`).
- Not 9-10 because: environments are not modeled or named anywhere in code (parity is implied by "same binary, different config" rather than designed); feature-flag support is fragmented across build tags, import side effects, config-key presence and scattered `os.Getenv` calls; unknown-option handling is warning-only, so a typo silently falls into `Extra` (`v1/config/config.go:182-187`) unless someone reads logs; and the two overlapping env mechanisms (`${VAR}` interpolation vs `OPA_*` flag mapping) have no unified precedence documentation in code.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Config struct | Top-level `Config` holds raw JSON sections (`services`, `bundles`, `decision_logs`, ...) plus `Extra` for unrecognized keys | `v1/config/config.go:86-110` |
| Config parse pipeline | `ParseConfig` runs raw config through embedded validation policy, overlays changed keys, parses default decision paths | `v1/config/config.go:118-154` |
| Layering: file load | `Load(configFile, overrides, overrideFiles)` reads base file then applies overrides | `internal/config/config.go:80-97` |
| Env interpolation | `${VAR}` regex substitution inside config file text and `--set` values; undefined vars kept literal | `internal/config/config.go:129-157` |
| Deep merge | `mergeValues` recursively merges maps, overrides scalar/list values | `internal/config/config.go:160-184` |
| `--set` / `--set-file` flags | Defined once, reused across run/exec commands | `cmd/flags.go:30-36`, `cmd/run.go:250-251` |
| Env→flag mapping | Viper `AutomaticEnv` maps every flag to `OPA_<CMD>_<FLAG>`; explicitly-set flags win (`f.Changed` check) | `cmd/internal/env/env.go:23-44` |
| Runtime wiring of config | `NewRuntime` calls `config.Load(...)` then builds store/tracing/metrics/plugins from it | `v1/runtime/runtime.go:449,490-559` |
| Embedded Rego validator | `//go:embed validate.rego`, query `data.opa.config = x` | `v1/config/validate.go:19-27` |
| Defaults injection in Rego | `default_decision := "/system/main"`, patches unioned over user config | `v1/config/validate.rego:16-18,25-33` |
| Unknown-key warnings | `walk`-based check against `_specs` patterns incl. `*` wildcard segments | `v1/config/validate.rego:46-60,88-163` |
| Extensible schema registration | `RegisterConfigSpec`, reflection-derived `SpecsFromStruct[T]` | `v1/config/spec.go:32-47` |
| Spec registration callers | metrics_export, server.decoding, server.encoding, server.metrics register their subtrees | `internal/metricsexport/metricsexport.go:51`, `v1/plugins/server/decoding/config.go:37`, `v1/plugins/server/encoding/config.go:21`, `v1/plugins/server/metrics/config.go:34` |
| Warnings surfaced at startup | Manager config warnings logged after plugin manager init | `v1/runtime/runtime.go:564-567`; also on discovery updates `v1/plugins/discovery/discovery.go:485-489` |
| Per-plugin validation+defaults | Downloader polling defaults (min 60s/max 120s) and error checks; bundle source validation; logs/status trigger-mode validation | `v1/download/config.go:15-17,43-87`, `v1/plugins/bundle/config.go:158-208`, `v1/plugins/logs/plugin.go:343`, `v1/plugins/status/plugin.go:113` |
| Active config API (redacted) | `ActiveConfig()` strips `credentials`, `key`, `private_key`; served by `/v1/config` | `v1/config/config.go:299-326,423-478`, `v1/server/server.go:2466-2474` |
| Dynamic reconfiguration | Discovery plugin downloads bundle → evaluates Rego to produce config → `Manager.Reconfigure` restarts/reconfigures plugins | `v1/plugins/discovery/discovery.go:420-437,466-545`, `v1/plugins/plugins.go:980-1032` |
| Boot-config protection | Discovered changes to discovery service or boot-specified keys rejected; local overrides reapplied over discovered config | `v1/plugins/discovery/discovery.go:439-464,505-526` |
| Reconfigure preserves boot labels/persistence | Labels merge-only from bootstrap config; persistence directory never erased | `v1/plugins/plugins.go:1004-1015` |
| Deployment: server vs REPL | `--server/-s` flag selects `Serve` vs `StartREPL` on the same runtime object | `cmd/run.go:223,396-401`, `v1/runtime/runtime.go:651,865` |
| Deployment: SDK embedding | `sdk.New(ctx, Options{Config: io.Reader, ...})` runs plugins without server; runtime `Configure()` supported | `v1/sdk/options.go:36-97,149-180`, `v1/sdk/opa.go:66,174-200` |
| Deployment: container | Distroless-style image, non-root UID 1000:1000, `ENTRYPOINT ["/opa"]`, `CMD ["run"]` | `Dockerfile:12-28` |
| Deployment docs | Sidecar/node-agent vs centralized service tradeoff table | `docs/docs/deploy/index.mdx:20-30` |
| Storage backend switch | Disk storage via config (`storage.disk`) or params; custom backends via `RegisterStorageBackend`; default inmem | `v1/runtime/runtime.go:101-108,270-276,505-519`, config key `v1/config/validate.rego:133-134` |
| Build-tag features | `opa_wasm` gates WASM server/test paths; `opa_no_oci` removes OCI download support | `v1/server/features.go:5`, `v1/download/oci_download_unavailable.go:1` |
| Import-side-effect features | Importing `features/wasm` registers the `wasm` eval engine | `features/wasm/wasm.go:9-18`, `v1/features/wasm/wasm.go:16-18` |
| Plugin enablement = config presence | `PluginNames()` returns bundles/status/decision_logs/custom names based on non-nil config sections | `v1/config/config.go:243-258` |
| Ad-hoc env toggles | `OPA_DECISIONS_INTERMEDIATE_RESULTS` package-level read; `OPA_VERSION_CHECK_SERVICE_URL` overrides telemetry endpoint; AWS/Azure credential chains read cloud env vars | `v1/server/server.go:105`, `internal/versioncheck/versioncheck.go:38-41,82`, `v1/plugins/rest/aws.go:79-97`, `v1/plugins/rest/azure.go:61-124` |
| Telemetry opt-out | `EnableVersionCheck` param; `--skip-version-check`/deprecated `--disable-telemetry` flags | `v1/runtime/runtime.go:241-243,774-777`, `cmd/run.go:255-257,351-356` |
| Readiness gate | `ReadyTimeout` waits for plugins before binding traffic (k8s readiness) | `v1/runtime/runtime.go:260-263,788-795,1073-1091` |
| Graceful shutdown | `ShutdownWaitPeriod` before shutdown, `GracefulShutdownPeriod` during, SIGINT/SIGTERM handled | `v1/runtime/runtime.go:234-239,814-815,1032-1071` |
| Tests: layering | `TestLoadConfigWithParamOverride`, `...FileOverride`, `...NoConfigFile`, `TestSubEnvVars*`, `TestMergeValues*` | `internal/config/config_test.go:25-133,310-495` |
| Tests: validation policy | Unknown-option warnings, defaults injection, non-string decision errors, root spec matches Go struct | `v1/config/validate_test.go:18-89` |
| Tests: roundtrip/safety | `TestActiveConfig`, `TestExtraConfigFieldsRoundtrip`, `TestConfigClone` | `v1/config/config_test.go:179,408,441` |
| E2E suites per concern | authz, tls, certrefresh, h2c, shutdown, oci, distributedtracing, metricsexport harnesses exercise full runtime | `v1/test/e2e/` (e.g. `v1/test/e2e/testing.go`) |

## Answers to Dimension Questions

### 1. Is configuration layered?

**Yes — four distinct layers plus dynamic reconfiguration.**

1. Base config file (JSON/YAML) loaded from `-c/--config-file` (`cmd/flags.go:26-28`, `internal/config/config.go:84-97`).
2. Environment interpolation applied to the file *text* and to `--set` strings via `${VAR}` placeholders (`internal/config/config.go:92,103,129-157`).
3. CLI overrides `--set key=val` and `--set-file key=path`, deep-merged with precedence over the file (`internal/config/config.go:99-125,160-184`; tests proving nested override behavior at `internal/config/config_test.go:174-235`).
4. Every flag itself can come from the environment as `OPA_<COMMAND>_<FLAG>` through viper automatic-env binding, where an explicitly-passed flag always wins (`cmd/internal/env/env.go:29-44`).
5. Post-parse, the embedded Rego policy injects defaults only for absent options and forces `labels.id`/`labels.version` (`v1/config/validate.rego:25-33`).

On top of static layering, the discovery plugin reconfigures a running process from a downloaded bundle, but the *boot* config is treated as privileged: discovered config cannot change the discovery service (`v1/plugins/discovery/discovery.go:505-510`) nor keys present in boot config (`discovery.go:512-526`), and local overrides are re-applied over the discovered config with overridden keys reported in status (`discovery.go:439-464,392-397`). `Manager.Reconfigure` additionally refuses to drop bootstrap labels or the persistence directory (`v1/plugins/plugins.go:1004-1015`).

### 2. Are environments managed cleanly?

**There is no named environment abstraction — no dev/staging/prod concept exists in code.** What exists instead are the primitives that make per-environment configs possible: labels intended for environment tagging (docs example uses `labels.environment: production`, `docs/docs/configuration.md:40-44`), `${VAR}` interpolation so secrets/endpoints can be injected from the orchestrator (`internal/config/config.go:131-157`), and `OPA_*` env vars for every flag (`cmd/internal/env/env.go:23-44`). Distributed tracing has a `deployment_environment` resource attribute key (`v1/config/validate.rego:150-153`), which is the closest thing to an environment marker in the schema. This is a deliberate "same binary everywhere" stance rather than a managed-environments feature; it works but leaves environment conventions entirely to operators. Cloud credentials are resolved through standard provider env-var chains rather than OPA-specific ones (`v1/plugins/rest/aws.go:79-97`, `v1/plugins/rest/azure.go:61-124`), which fits 12-factor practice.

### 3. Are deployment modes documented?

**Yes, in both docs and code structure.** The docs define the two canonical topologies — sidecar/node-agent vs centralized service — with a latency/fault-tolerance tradeoff table (`docs/docs/deploy/index.mdx:20-30`) plus per-platform guides (`docs/docs/deploy/k8s/`, `docs/docs/deploy/docker/`, aws/, azure/, google-cloud/). In code, the modes are: REPL vs server selected by `--server` (`cmd/run.go:75-89,396-401`); embedded SDK mode where OPA runs as a library with plugin support but no HTTP server (`v1/sdk/opa.go:66-130`, `v1/sdk/options.go:36-97`); and the offline toolchain commands (`eval`, `exec`, `build`, `test`, `bench`) registered in `cmd/commands.go:35-50`. The container shape is minimal and production-oriented: fixed non-root numeric user for k8s `runAsNonRoot` compatibility (`Dockerfile:12-16`) and `run` as default CMD (`Dockerfile:27-28`).

### 4. Are feature flags supported?

**Partially — there is no first-class feature-flag system; toggling happens through four separate mechanisms:**

- **Build tags**: `opa_wasm` compiles in/out WASM evaluation paths (`v1/server/features.go:5`, Makefile adds `-tags=opa_wasm` at `Makefile:21`); `opa_no_oci` strips OCI bundle downloads (`v1/download/oci_download_unavailable.go:1`).
- **Import-for-side-effect**: importing `features/wasm` registers the wasm eval engine in an `init()` (`v1/features/wasm/wasm.go:16-18`); similarly `runtime.RegisterPlugin`/`RegisterStorageBackend`/`RegisterHook` let embedders add capabilities at link time (`v1/runtime/runtime.go:95-122`).
- **Config-key presence as enablement**: a plugin runs iff its config section exists — `PluginNames()` derives enabled plugins from non-nil `bundles`/`status`/`decision_logs`/`plugins` entries (`v1/config/config.go:243-258`). This is the dominant "flag" mechanism.
- **Ad-hoc environment variables**: e.g. `OPA_DECISIONS_INTERMEDIATE_RESULTS` read into a package var at init (`v1/server/server.go:105`), `OPA_VERSION_CHECK_SERVICE_URL` for telemetry redirection (`internal/versioncheck/versioncheck.go:82`).

The fragmentation means there's no uniform place to audit what is enabled; behavior emerges from build inputs, imports, config, and environment simultaneously.

### 5. Is configuration validated?

**Yes, unusually thoroughly — with a novel twist: OPA validates its config using Rego, its own policy language.** `ParseConfig` feeds the raw config into an embedded module compiled once and cached (`v1/config/validate.go:19-27`, compiler reuse in `internal/configpolicy/configpolicy.go:171-195`). The policy (a) injects defaults via patch fragments (`v1/config/validate.rego:25-33`), (b) emits fatal errors such as non-string decision paths (`v1/config/validate.rego:35-41`), and (c) warns on unrecognized option keys at any depth by walking the document against pattern specs including `*` wildcards for map entries (`v1/config/validate.rego:43-74,88-163`). The schema is kept honest against the Go structs by a test asserting the root spec matches the `Config` struct fields (`v1/config/validate_test.go:89`) and stays extensible: subsystems call `RegisterConfigSpec(config.SpecsFromStruct[T](...))` to derive key sets reflectively from their decode types (`v1/config/spec.go:32-100`; callers listed above). Residual semantic checks that Rego can't express happen in Go — decision paths must parse (`v1/config/config.go:145-151`). Beyond the core, each plugin validates its own section and injects defaults (bundle polling bounds and pairing rules at `v1/download/config.go:57-84`; service references must resolve, `v1/plugins/bundle/config.go:186-199,225-232`). Validation severity is asymmetric by design: structural typos warn (surfaced via `logger.Warn` at `v1/runtime/runtime.go:564-567`), semantic violations error.

## Architectural Decisions

1. **Raw JSON sections in the central config, typed parsing deferred to owners.** Core `Config` stores most subsystems as `json.RawMessage` (`v1/config/config.go:87-104`), letting each plugin own its schema evolution while unknown-but-valid keys round-trip through `Extra` (`v1/config/config.go:204-227`, test `v1/config/config_test.go:408`). The cost — an opaque top level — was later mitigated with the Rego-based unknown-key warnings.
2. **Self-hosted validation policy.** Using Rego (`v1/config/validate.rego`) instead of hand-written checks makes defaults declarative and auditable, exercises OPA's own engine on every startup, and lets specs be data supplied as input (`input.specs`, `v1/config/validate.go:33-41`).
3. **Boot config is sovereign.** Anything needed to reach the control plane (discovery service URL, verification keys, labels id/version, persistence dir) survives reconfiguration untouched (`v1/plugins/discovery/discovery.go:491-526`, `v1/plugins/plugins.go:1004-1015`), preventing a bad remote config from bricking the agent — the classic failure mode of remote-config systems.
4. **One binary, many modes.** REPL/server/SDK/offline-tools all share `Runtime`/plugin-manager construction (`v1/runtime/runtime.go:384-614`, `v1/sdk/opa.go:174-200`), so config semantics do not fork per mode.
5. **Helm-compatible override syntax.** Services accept either array or map form explicitly because Helm cannot address array elements (`internal/config/config.go:37-40,53-72`), and `strvals`-based `--set` follows chart-tooling conventions.

## Notable Patterns

- **Buffered early logging**: startup logs go to a 1000-entry buffer flushed once a logger plugin resolves, so misconfigurations during boot aren't lost (`v1/runtime/runtime.go:413-417`, resolution at `runtime.go:693-698`).
- **Readiness as a first-class lifecycle state**: `ServerWaitingForPlugins → ServerInitialized` transitions gated on plugin status with configurable timeout (`v1/runtime/runtime.go:353-360,788-795,1073-1091`) — directly supports k8s probes.
- **URL shorthand desugaring to config overrides**: `opa run -s https://host/bundle.tar.gz` becomes synthesized `services.cliN.url`/`bundles.cliN.*` override entries (`v1/runtime/runtime.go:433-447,1113-1130`) — new UX built atop the existing override layer rather than a parallel mechanism.
- **Reflection-derived config schemas**: `SpecsFromStruct[T]` walks JSON tags to generate validation specs, keeping the validator in sync with decode types mechanically (`v1/config/spec.go:43-100`).
- **Secret redaction at the observation boundary**: `ActiveConfig()` deletes `credentials`, `key`, `private_key` before serving `/v1/config` (`v1/config/config.go:299-326,423-478`).
- **Brand parameterization**: the whole command tree is instantiated per brand (`Command(rootCommand, brand)`, `cmd/commands.go:25-52`), enabling OPA derivatives without forking config plumbing.

## Tradeoffs

- **Warning-only unknown keys**: a misspelled option lands in `Extra` and is ignored functionally (`v1/config/config.go:182-187`); safety depends on operators reading warnings. A strict mode does not exist.
- **Two env mechanisms, no documented precedence between them**: `${VAR}` substitution operates on config text (`internal/config/config.go:129`), `OPA_<CMD>_<FLAG>` operates on flags (`cmd/internal/env/env.go:25`); they compose implicitly, and an undefined `${VAR}` is left literal rather than rejected (`config.go:147-153`), which can silently embed placeholder text like `${MISSING}` into URLs/tokens.
- **Config-enablement coupling**: enabling a plugin requires a config stanza, so "turn off decision logs" is a deletion rather than a boolean flip; convenient for declarative deploys but makes accidental enablement via stray empty objects possible (`decision_logs: {}` enables the plugin per `v1/config/config.go:250-252`).
- **Deep-merge semantics for lists are overwrite-only**: arrays from overrides replace arrays wholesale (test `internal/config/config_test.go:237`), pushing users toward map-form services/bundles.
- **Build-tag features fragment the artifact matrix**: wasm/no-OCI variants mean binaries are not fully interchangeable; capability introspection exists (`capabilities.json`, `opa capabilities`) but deployment pipelines must track which variant they ship.

## Failure Modes / Edge Cases

- **Literal `${VAR}` leakage**: unset env vars stay as-is by design ("we do not play by bash rules", `internal/config/config.go:147-153`); covered by tests (`internal/config/config_test.go:94-133`).
- **Discovery update with unrecoverable changes**: changes to the discovery service are rejected outright because rollback would require tracking history (`v1/plugins/discovery/discovery.go:491-494` comment); failed reconfigs set plugin error status and clear cache (`discovery.go:368-373`).
- **Persisted discovery bundle fallback**: if `persist: true`, the last good discovery bundle is activated from disk on cold start, bounded by 10 activation retries (`v1/plugins/discovery/discovery.go:44-48,160-180`) — resilience against control-plane outages.
- **Token-auth-without-authorization misconfiguration** detected and loudly logged as ineffective (`v1/runtime/runtime.go:680-682`).
- **Listener failure mid-flight** escalates to `os.Exit(1)` rather than half-serving (`v1/runtime/runtime.go:833-835`).
- **TLS partial config** (cert without key, or vice versa) fails fast at flag processing (`cmd/run.go:443-455`).
- **Insecure v0 default bind**: default `:8181` binds all interfaces only under `--v0-compatible`; a public-interface warning is emitted (`v1/runtime/runtime.go:666-669`, `cmd/run.go:389-391`).

## Future Considerations

- Introduce an explicit, validated environment/deployment-profile layer (e.g., named profiles in the Rego validation policy) instead of relying on operator convention around labels and interpolation.
- Consolidate feature toggling behind one registry (config-declared, build-time-declared, import-declared) so enabled-feature audits become mechanical; today an auditor must inspect tags, imports, config, and env vars separately.
- Offer a strict-validation mode where unknown keys are fatal, given the machinery already computes them (`Warnings`, `Extra`).
- Extend `RegisterConfigSpec` coverage beyond the current five registrants (`internal/metricsexport/metricsexport.go:51`, `v1/plugins/server/{encoding,decoding,metrics}/config.go`) so more plugin-owned subtrees move out of the hard-coded `_core_specs` list (`v1/config/validate.rego:88-163`).
- Document the interplay of `${VAR}` substitution and `OPA_*` flag variables in a single precedence reference; currently each mechanism is discoverable only from code.

## Questions / Gaps

- No evidence found of a staging/prod differentiation mechanism beyond config files themselves; searched for `environment`, `env`, `profile`, `stage` concepts in `v1/config/` and `v1/runtime/` — only the tracing `deployment_environment` resource key matched (`v1/config/validate.rego:150-153`).
- No evidence found of centralized secret management (Vault/KMS integration) in the config loader; secrets arrive via `${VAR}` interpolation or cloud auth plugins (`v1/plugins/rest/auth.go`), and are redacted only at the `/v1/config` output boundary.
- Whether the `OPA_DECISIONS_INTERMEDIATE_RESULTS` env toggle (`v1/server/server.go:105`) is intended as long-term surface or transitional hack could not be determined from code alone; it bypasses the config validation pipeline entirely.

---

Generated by `Dimension 22.02: Configuration and Deployment Shape` against `opa`.

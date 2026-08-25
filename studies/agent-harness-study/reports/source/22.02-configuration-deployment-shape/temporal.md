# Source Analysis: temporal

## Dimension 22.02: Configuration and Deployment Shape

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (Temporal server: frontend/history/matching/worker services, gRPC, Cassandra/MySQL/PostgreSQL/SQLite/Elasticsearch backends) |
| Analyzed | 2026-08-25 |

## Summary

Temporal server treats configuration as a first-class subsystem with three explicit loading strategies and a separate, runtime-mutable "dynamic config" layer that doubles as the feature-flag mechanism. Static configuration is a strongly-typed `Config` struct (`common/config/config.go:30-56`) loaded from YAML via a functional-options loader (`common/config/loader.go:129-141`) that supports (a) an embedded, environment-variable-driven template compiled into the binary (`common/config/loader.go:20-21`, `common/config/config_template_embedded.yaml:1`), (b) a single explicit `--config-file`, or (c) a legacy directory hierarchy of `base.yaml` → `<env>.yaml` → `<env>_<zone>.yaml` overlays (`common/config/loader.go:172-176`, `common/config/loader.go:283-310`). Config files may optionally be Go templates rendered with Sprig functions, gated by an `# enable-template` comment (`common/config/loader.go:227-252`, `common/config/loader.go:268-279`).

Deployment shape is "one binary, many topologies": `temporal-server start` runs all default services in one process by default but accepts per-service selection (`cmd/server/main.go:136-142`, `temporal/server.go:25-46`), with a documented init order for co-hosted services (`temporal/server_impl.go:40-48`). The official container entrypoint resolves bind/broadcast IPs from env vars and execs the same binary (`docker/scripts/sh/entrypoint.sh:1-16`, `docker/targets/server.Dockerfile:1-16`), while release engineering covers multi-OS/arch binaries plus DB tooling (`.goreleaser.yml:15-60`) and CI builds/publishes multi-arch images (`.github/workflows/build-and-publish.yml:41-77`).

Feature flags are implemented as a typed dynamic-config registry with per-key defaults and descriptions, constraint-based overrides (namespace / task queue / task type / shard), file polling with change subscriptions, monotonic percentage rollouts, and scheduled gradual changes (`common/dynamicconfig/collection.go:30-53`, `common/dynamicconfig/registry.go:9-34`, `common/dynamicconfig/rollout.go:17-27`, `common/dynamicconfig/gradual_change.go:15-24`). Validation happens at multiple layers: struct-tag validation at load time (`common/config/validator.go:10-14`, `common/config/config.go:695-712`), semantic persistence checks including password-via-command resolution (`common/config/persistence.go:35`), and a dedicated `validate-dynamic-config` CLI command (`cmd/server/main.go:82-107`). The answer to the dimension's guiding question — can the same binary run dev/staging/prod with config-only changes? — is yes, demonstrated end-to-end from local Make targets through docker env-var deployment.

## Rating

**Score: 9 / 10**

Rationale against the rubric:

- **Clear model**: Three explicitly documented loading strategies selected by mutually exclusive flags with a startup guard rejecting conflicting flag combinations (`cmd/server/main.go:144-153`); precedence hierarchy documented in code comments (`common/config/loader.go:161-176`).
- **Tests**: Loader behavior covered by `TestLoad` / `TestPathResolution` (`common/config/loader_test.go:15`, `common/config/loader_test.go:187`); persistence validators by `common/config/persistence_test.go:12-260`; rollout monotonicity boundaries by `TestRolloutAccepts_Boundaries` / `_Monotonic` (`common/dynamicconfig/rollout_test.go:14`, `common/dynamicconfig/rollout_test.go:24`); YAML error handling cases in `common/dynamicconfig/file_based_client_test.go:898-975`.
- **Operational safeguards**: `render-config` masks secret fields before printing (`common/config/config.go:720-725`); missing authorizer triggers a warning unless `--allow-no-auth` is passed (`cmd/server/main.go:203-209`); dynamic-config update failures are throttled rather than fatal (`common/dynamicconfig/collection.go:202-207`); the collection registers itself as pingable for deadlock detection (`common/dynamicconfig/fx.go:15-19`, `common/dynamicconfig/collection.go:146`).
- **Extensibility/proven under scale**: The `Client` interface is designed for alternate sources with explicit guidance to keep lookups in-memory (`common/dynamicconfig/client.go:13-33`); conversion caching uses weak pointers to bound memory (`common/dynamicconfig/collection.go:49-53`).

It stops short of 10 because two overlapping static-loading models remain in production code (the legacy hierarchy is deprecated but still the path taken when `--env` is supplied, `cmd/server/main.go:170-175` vs. `common/config/loader.go:164-166`), template activation is implicit magic detected by scanning the first 1KB of a file for a comment (`common/config/loader.go:268-279`), and there is no built-in remote/centralized static-config source in this repo (only the file-based dynamic client; extension requires implementing `Client` yourself).

## Evidence Collected

Every entry includes a workspace-relative path with line numbers (paths relative to the source root `studies/agent-harness-study/sources/temporal/`).

| Area | Evidence | File:Line |
|------|----------|-----------|
| Typed config model | Root `Config` struct aggregating Global/Persistence/Log/ClusterMetadata/Services/Archival/DynamicConfigClient/etc. | `common/config/config.go:30-56` |
| Functional-options loader | `Load(opts...)` with `WithEnv`, `WithConfigDir`, `WithZone`, `WithConfigFile`, `WithEmbedded` | `common/config/loader.go:71-141` |
| Embedded env-var config | `//go:embed config_template_embedded.yaml`; used when no flags given | `common/config/loader.go:20-21`, `common/config/loader.go:145-148` |
| Layering precedence | Legacy hierarchy `base.yaml` → `<env>.yaml` → `<env>_<zone>.yaml`, later files override earlier | `common/config/loader.go:172-176`, `common/config/loader.go:283-310` |
| Template rendering | Sprig-fueled Go templates activated by `# enable-template` comment within first 1KB | `common/config/loader.go:227-252`, `common/config/loader.go:268-279` |
| Env-var keys | `TEMPORAL_ROOT`, `TEMPORAL_CONFIG_DIR`, `TEMPORAL_ENVIRONMENT`, `TEMPORAL_AVAILABILITY_ZONE`, `TEMPORAL_SERVER_CONFIG_FILE_PATH`, `TEMPORAL_ALLOW_NO_AUTH` | `common/config/loader.go:29-45` |
| CLI surface | `start` / `render-config` / `validate-dynamic-config` subcommands; flags bound to env vars | `cmd/server/main.go:80-124` |
| Service selection topology | `--service` repeatable flag + `TEMPORAL_SERVICES` env; defaults to Frontend/History/Matching/Worker | `cmd/server/main.go:130-142`, `temporal/server.go:25-46` |
| Co-hosted service ordering | Documented init order matching→history→frontend→worker for single-process dev servers | `temporal/server_impl.go:40-48` |
| Optional internal-frontend | `USE_INTERNAL_FRONTEND` conditional service block in docker template; publicClient must be empty when IFE enabled | `docker/config_template.yaml:283-292`, `common/config/config.go:703-706` |
| Dynamic config client selection | File-based client if `dynamicConfigClient` configured, else noop; injectable override | `temporal/fx.go:227-240` |
| Feature-flag registry | Global registry panics on duplicate key registration; settings registered only in static initializers | `common/dynamicconfig/registry.go:13-27` |
| Typed settings w/ docs+defaults | `setting[T,P]` carries key, default, converter, description; generated constructors e.g. `NewNamespaceBoolSetting` | `common/dynamicconfig/setting.go:16-21`, `common/dynamicconfig/setting_gen.go:45-49` |
| Constraint model | Values constrained by namespace/taskQueueName/taskType; documented format | `config/dynamicconfig/README.md:1-38`, `common/dynamicconfig/shared_structure.go` |
| Runtime updates | `FileBasedClient` polls file every `PollInterval` (min 5s) and reloads on mtime change | `common/dynamicconfig/file_based_client.go:23`, `common/dynamicconfig/file_based_client.go:133-141` |
| Change notifications | `NotifyingClient.Subscribe` fan-out; Collection subscribes and dispatches to typed subscribers | `common/dynamicconfig/client_subscriptions.go:23-30`, `common/dynamicconfig/collection.go:131`, `common/dynamicconfig/collection.go:186` |
| Percentage rollout | `RolloutAccepts` stable-hash monotonic gate [0,100] | `common/dynamicconfig/rollout.go:17-27` |
| Scheduled flag flips | `GradualChange[T]` Old→New over Start..End window per key | `common/dynamicconfig/gradual_change.go:15-24` |
| Dynamic config validation | `LoadYamlFile` returns structured Warnings/Errors; CLI `validate-dynamic-config` exits non-zero on errors | `common/dynamicconfig/yaml_loader.go:21-37`, `cmd/server/main.go:82-107` |
| Static config validation | `validator.v2` tags (`nonzero`, custom `persistence_custom_search_attributes`) applied after load | `common/config/validator.go:10-49`, tag sites `common/config/config.go:259-638` |
| Semantic validation | `Config.Validate` → Persistence/Archival/publicClient checks; `serverOptions.loadAndValidate` at fx bootstrap | `common/config/config.go:694-712`, `common/config/persistence.go:35`, `temporal/fx.go:180`, `temporal/server_options.go:84` |
| Secret handling | DB password resolvable via external command; masked output for `render-config` | `common/config/persistence.go:8`, `common/config/persistence.go:285-300` (password-command validation/resolution), `common/config/config.go:720-725` |
| Dev environment parity | Per-datastore dev configs (`development-cass-es.yaml`, `development-mysql8.yaml`, …) launched via `make start-*` with `--config-file` | `config/development-*.yaml`, `Makefile:705-730` |
| Docker deployment | Alpine non-root image, `entrypoint.sh` derives `BIND_ON_IP`/`TEMPORAL_BROADCAST_ADDRESS` then `exec temporal-server start` | `docker/targets/server.Dockerfile:1-16`, `docker/scripts/sh/entrypoint.sh:5-16` |
| Multi-arch build matrix | docker-bake variables (SERVER_VERSION, platforms, ALPINE_TAG single source of truth) | `docker/docker-bake.hcl:1-40` |
| Release engineering | GoReleaser v2: server + cassandra/sql/elasticsearch tools + tdbg, CGO_ENABLED=0, linux/darwin/windows × amd64/arm64 | `.goreleaser.yml:15-60` |
| CI publish pipeline | `build-and-push-docker` job using composite `build-docker-images` action with tagging outputs | `.github/workflows/build-and-publish.yml:41-77`, `.github/actions/build-docker-images/action.yml:1-55` |
| Local dependency stack | docker-compose with MySQL/Cassandra/Postgres/Elasticsearch (+ OS-specific overlay files) | `develop/docker-compose/docker-compose.yml:1-50`, `Makefile:683-701` |
| Env var policy | Comment: server code does not read env vars directly except via templated config/test helpers | `temporal/environment/env.go:10-16` |

## Answers to Dimension Questions

### 1. Is configuration layered?

Yes, twice over. Static config layers by file: `base.yaml` (lowest), then `<env>.yaml`, then `<env>_<zone>.yaml` (highest), merged sequentially so later keys override earlier ones (`common/config/loader.go:196-211`, candidate construction at `common/config/loader.go:283-310`). On top of that sits a second, runtime layer: dynamic config values override static defaults per key with constraint scoping, resolved by `Collection` over any `Client` implementation (`common/dynamicconfig/client.go:13-33`, `common/dynamicconfig/collection.go:113`). Additionally, individual files can themselves be parameterized Go/Sprig templates pulling from process environment variables when marked `# enable-template` (`common/config/loader.go:227-252`).

### 2. Are environments managed cleanly?

Largely yes. Environment selection is explicit (`--env` / `TEMPORAL_ENVIRONMENT`, defaulting to `development`, `cmd/server/main.go:55-61`, `common/config/loader.go:180-182`), availability-zone overlays are supported including a documented backwards-compat typo key (`common/config/loader.go:36-40`), and the container path needs no env-named files at all because the embedded template renders purely from environment variables (`common/config/loader.go:145-148`, `common/config/config_template_embedded.yaml:4-120`). The repo's own guidance states server code does not consume env vars directly outside the templating/test-helper paths (`temporal/environment/env.go:10-16`). Caveat: the legacy directory scheme is still the live path whenever `--env/--config/--zone` are passed and is self-described as "Deprecated… should not be used in new code" (`common/config/loader.go:164-166`), so two models coexist. Dev parity is handled concretely via `development-*.yaml` variants and a compose stack for dependencies (`config/development-cass-es.yaml`, `config/development-mysql8.yaml`, `develop/docker-compose/docker-compose.yml:1-50`).

### 3. Are deployment modes documented?

Yes, at the code/artifact level. Single-process all-in-one is the default (`DefaultServices`, `temporal/server.go:33-41`); per-service processes are first-class via repeated `--service` flags or `TEMPORAL_SERVICES` (`cmd/server/main.go:130-142`); co-hosting order is specified with rationale (`temporal/server_impl.go:40-48`); an optional internal-frontend tier is conditionally wired in config (`docker/config_template.yaml:275-292`, validated at `common/config/config.go:703-706`). Packaging is documented by artifacts: multi-arch Dockerfile with non-root user (`docker/targets/server.Dockerfile:1-16`), bake-based multi-platform builds (`docker/docker-bake.hcl:1-40`), cross-compiled tool binaries (`.goreleaser.yml:15-60`), and CI publication workflows (`.github/workflows/build-and-publish.yml:41-77`). There is no long-form ops document inside this repo describing deployment topologies; documentation exists mostly as code comments and compose headers (`develop/docker-compose/docker-compose.yml:1-4`) — presumably richer docs live outside this source tree.

### 4. Are feature flags supported?

Yes, comprehensively — dynamic config *is* the feature-flag system. Keys are statically registered typed settings with defaults, converters, and human-readable descriptions, generated from a template (`common/dynamicconfig/setting.go:16-21`, generator at `cmd/tools/gendynamicconfig/main.go:1-30`); ~3,700 lines of catalogued keys live in `common/dynamicconfig/constants.go`. Values support constraint-scoped overrides (namespace / task queue / task type / shard filters listed at `common/dynamicconfig/collection.go:83-88`) and hot-reload: the file client polls (`common/dynamicconfig/file_based_client.go:133-141`) and pushes diffs to subscribers (`common/dynamicconfig/collection.go:186`). Two rollout primitives stand out: `RolloutAccepts` gives monotonic stable-hash percentage gating explicitly designed so "a key accepted at percent P is accepted at every percent >= P" (`common/dynamicconfig/rollout.go:8-27`), and `GradualChange[T]` schedules per-key value flips across a time window (`common/dynamicconfig/gradual_change.go:15-24`). The `Client` interface is intentionally narrow so operators can substitute remote flag sources without touching call sites (`common/dynamicconfig/client.go:13-33`).

### 5. Is configuration validated?

Yes, in depth, though spread across layers. At load time every strategy ends in `validate.Validate(config)` using `gopkg.in/validator.v2` tags plus one custom rule for persistence custom search attributes (`common/config/loader.go:213-214`, `common/config/loader.go:264-265`, `common/config/validator.go:10-49`). Structural semantics are checked by `Config.Validate` → `Persistence.Validate` (store/type consistency) → `Archival.Validate`, plus the internal-frontend/publicClient mutual-exclusion rule and enum check on `forceTLSConfig` (`common/config/config.go:694-712`, `common/config/persistence.go:35`, `common/config/archival.go:17`). Bootstrap re-validates via `so.loadAndValidate()` before fx graph construction (`temporal/fx.go:180`, `temporal/server_options.go:84`). Dynamic config files get their own loader producing typed warnings vs. errors and a CLI validator usable in pipelines (`common/dynamicconfig/yaml_loader.go:21-37`, `cmd/server/main.go:82-107`). Tests cover both happy and failure paths (`common/config/loader_test.go:15` includes invalid-YAML expectations; `common/dynamicconfig/file_based_client_test.go:951` feeds garbage input). Notably, the docker template even validates its own `$db` input and fails rendering on unsupported databases (`config/docker.yaml:11-14`).

## Architectural Decisions

1. **Compile the default config into the binary.** The embedded template (`common/config/loader.go:20-21`) means the container distribution needs zero mounted files; all variation flows through env vars rendered at startup (`common/config/config_template_embedded.yaml:4-120`). This trades YAML-file flexibility for 12-factor-style operability and makes "same binary everywhere" literal.
2. **Separate static from dynamic config instead of making everything hot-reloadable.** Structural/topology settings (ports, datastores, shards) are static and validated eagerly; behavioral knobs (~thousands of keys in `common/dynamicconfig/constants.go`) are runtime-mutable. The boundary is enforced by type: `Collection` accessors dominate service code, while `config.Config` is injected once.
3. **Narrow extension point for config sources.** A single-method `Client` interface with explicit performance contract ("GetValue is called very often! You should not synchronously call out to an external system", `common/dynamicconfig/client.go:13-33`) lets deployments plug remote flag stores while keeping hot-path lookups in-memory behind atomic snapshots (`common/dynamicconfig/file_based_client.go:38`).
4. **Templates opt-in per file, not global.** Requiring `# enable-template` in the first 1KB (`common/config/loader.go:268-279`) avoids accidental interpolation of plain YAML containing `{{ }}`, at the cost of a discoverability quirk.
5. **Fail-safe dynamic config operation.** Update failures log (throttled past 1000 occurrences) but never crash the server; the last-good snapshot stays served via `atomic.Value` (`common/dynamicconfig/file_based_client.go:38`, `common/dynamicconfig/collection.go:202-207`).
6. **One artifact, many tools.** GoReleaser ships the server plus cassandra/sql/es schema tools and `tdbg` from the same module (`.goreleaser.yml:15-60`), keeping deployment tooling version-locked to the server.

## Notable Patterns

- **Functional options for loader composition** (`loadOption` closures, `common/config/loader.go:61-117`) mirrored by server options (`temporal.WithConfig`, `temporal.ForServices` at `cmd/server/main.go:222-234`).
- **Generic typed settings**: `setting[T any, P any]` where the phantom type `P` encodes precedence (global/namespace/task-queue/…) so each accessor family is compile-time distinct (`common/dynamicconfig/setting.go:11-21`); subscription APIs are code-generated per precedence (`common/dynamicconfig/setting_gen.go:937`, `common/dynamicconfig/setting_gen.go:1073`, `common/dynamicconfig/setting_gen.go:1209`).
- **Weak-pointer caches** for converted dynamic values and constraint indexes to avoid unbounded memory on hot keys (`common/dynamicconfig/collection.go:49-53`).
- **Secret masking at the presentation boundary**: `Config.String()` pipes the YAML encoder through `masker.MaskYaml` with a default field-name list (`common/config/config.go:720-725`), so diagnostics never need to remember to redact.
- **Credentials via indirection**: DB passwords may be fetched by executing a command rather than stored inline, with timeout/trailing-newline edge cases tested (`common/config/persistence_test.go:63-108`).
- **Environment abstraction discipline**: a dedicated `environment` package funnels test/tool env-var reads, keeping production server code free of direct `os.Getenv` (`temporal/environment/env.go:10-16`).

## Tradeoffs

- **Two static loaders coexist.** The legacy directory hierarchy remains reachable and is the only path supporting zone overlays; the newer `--config-file` and embedded paths don't do layering at all (`cmd/server/main.go:167-178`). Operators get flexibility, but mental model duplication and drift risk (e.g., duplicated cassandra/mysql/postgres blocks between `config_template_embedded.yaml` and `docker/config_template.yaml`).
- **Template-by-comment is powerful but implicit.** Whether a file interpolates depends on scanning bytes, not extension or declaration (`common/config/loader.go:230-236`) — surprising when moving config between environments changes whether `{{ }}` is literal.
- **File-polling feature flags vs. latency/freshness.** Default poll floor is 5s (`common/dynamicconfig/file_based_client.go:23`); notification-based subscription mitigates this only for clients implementing `NotifyingClient` (`common/dynamicconfig/client_subscriptions.go:23-30`). No built-in push source in OSS.
- **Validation split across stages.** Struct-tag validation catches little beyond nonzero fields; most semantic correctness surfaces later at fx bootstrap (`temporal/fx.go:180`) or persistence-client construction, so some misconfigurations fail at startup rather than parse time — good enough for servers (fail-fast boot), weaker for tooling that wants early linting.
- **Co-hosted services simplify dev, complicate prod reasoning.** Running four services in one process shares fate and resources; the codebase acknowledges this is "typically a development server" (`temporal/server_impl.go:41-42`), pushing prod toward per-service processes without enforcing it.

## Failure Modes / Edge Cases

- **Bad YAML aborts boot** with wrapped errors naming the file (`common/config/loader.go:217-224`, `common/config/loader.go:260-262`); invalid dynamic config content produces structured errors/warnings distinguishing blocking vs. advisory issues (`common/dynamicconfig/yaml_loader.go:21-37`).
- **Dynamic config file deleted/unreadable mid-run**: poll errors are logged and throttled, not fatal; stale values continue serving (`common/dynamicconfig/file_based_client.go:138-141`, `common/dynamicconfig/collection.go:202-207`). Risk: silent staleness — partially offset by pingable health registration (`common/dynamicconfig/fx.go:15-19`).
- **Duplicate dynamic config keys panic at registration**, but only during static init, surfacing programmer error immediately (`common/dynamicconfig/registry.go:22-25`).
- **Conflicting CLI flags rejected upfront**: `--config-file` cannot be combined with `--config/--env/--zone/--root` (`cmd/server/main.go:149-151`).
- **Missing authorizer**: warns loudly and telegraphs future hard requirement unless `--allow-no-auth` is set (`cmd/server/main.go:203-209`) — an operational guard against accidentally shipping authless.
- **Cluster metadata mismatches**: persisted cluster metadata wins over changed config values with an explanatory log rather than refusing to start (`temporal/server.go:18-20`).
- **Container networking**: entrypoint resolves hostname→IP and special-cases wildcard binds to set broadcast addresses, avoiding ringpop advertising of unreachable addresses (`docker/scripts/sh/entrypoint.sh:5-16`).

## Future Considerations

- Retire or freeze the legacy hierarchical loader once zone-overlay demand is absorbed by templated single files, collapsing to one static model (`common/config/loader.go:164-166`).
- Promote template activation from comment-sniffing to explicit syntax/extension to remove the implicit trigger (`common/config/loader.go:268-279`).
- Ship a reference remote `Client` implementation (or subscribe-capable admin API) so OSS users aren't pushed to write bespoke integrations for centralized flag management (`common/dynamicconfig/client.go:13-33`).
- Deepen static validation (cross-field port/range checks, datastore-type/plugin-name consistency) so more errors surface at parse time alongside `validate-dynamic-config`-style linting for static files (`cmd/server/main.go:82-107` shows the pattern already exists for dynamic config).
- Deduplicate the embedded vs. docker templates or generate both from one source to prevent divergence in supported backend matrices.

## Questions / Gaps

- **No evidence found in-tree for a staging-specific story** beyond the generic env-file mechanism: nothing named staging exists under `config/`, and no workflow doc describes promoting configs; searched `config/*.yaml`, `docs/`, `Makefile`, and CI workflows. Staging parity rests entirely on the generic env/layering machinery.
- **Feature-flag administration UI/API**: this repo contains the consumption side (settings, clients, validators) and the file-format docs (`config/dynamicconfig/README.md:1-10`), but no operator tooling to set flags remotely (e.g., no HTTP control-plane endpoint writes dynamic config); searched `service/frontend` and `common/dynamicconfig` for writers. Administration appears delegated to editing the watched file or external systems.
- **Zone-layer usage frequency**: `WithZone` is fully implemented (`common/config/loader.go:93-99`) but no shipped example config exercises `*_az.yaml`; actual operational use could not be verified from the source alone.
- The empty placeholder `common/config/template_coverage_test.go:1` suggests intended-but-unrealized coverage linking embedded template keys to `Config` struct fields; current coverage of the embedded template's full key set was not found elsewhere.

---

Generated by `Dimension 22.02: Configuration and Deployment Shape` against `temporal`.

# Source Analysis: letta

## Dimension 22.02: Configuration and Deployment Shape

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python 3.11+ (FastAPI, SQLAlchemy/asyncpg, pydantic-settings, Alembic, Docker/Compose) |
| Analyzed | 2026-08-25 |

## Summary

Letta implements a three-layer configuration model: (1) an optional hierarchical YAML file (`conf.yaml`) is deep-merged across user/project/explicit locations, (2) flattened into `LETTA_*` / provider-prefixed environment variables at import time, and (3) consumed by eight pydantic-settings singletons (`Settings`, `ModelSettings`, `ToolSettings`, `SummarizerSettings`, `LogSettings`, `TelemetrySettings`, `ReadinessSettings`, `TestSettings`) in `letta/settings.py`. Environment variables take precedence over the YAML file (`letta/config_file.py:214-232`). Deployment shape is "one artifact, many modes": a single Docker image embeds Postgres, Redis, and an OpenTelemetry Collector, with a startup script that auto-launches internal dependencies when external ones are not configured (`letta/server/startup.sh:24-51`) — so the same binary runs as a self-contained all-in-one container, a compose service beside pgvector/nginx, or against fully managed Postgres/Redis purely via env vars. Environments (dev/prod/canary) are handled through one free-form `settings.environment` string used for telemetry tagging and prod-only behavior gating; this works but suffers from inconsistent, case-sensitive comparisons (`"PRODUCTION"` vs `"prod"`). Feature flags are static boolean settings requiring restart (no dynamic flag service). Config validation is partial: pydantic enforces types and some range constraints, but YAML load errors are silently swallowed and unknown keys ignored.

## Rating

**7 / 10 — Clear model with explicit typed interfaces and real operational safeguards, but the config layer itself has no direct tests, silent failure modes, and environment-name inconsistency keep it below the 8+ bar.**

Rationale:
- The layering strategy is coherent and documented in-repo (`conf.yaml:1-9` maps every section to its env prefix; `letta/config_file.py:1-46` documents precedence).
- The deployment question — *"Can the same binary run in dev, staging, and prod with config changes only?"* — is answered **yes**: CI publishes exactly one multi-arch image per release (`.github/workflows/docker-image.yml:33-41`), and mode differences (internal vs external Postgres/Redis, OTEL export target, secure mode) are all env-var driven.
- Deductions: silent exception swallowing during YAML load (`letta/config_file.py:97-98`), import-time mutation of the process environment plus module-level singletons (`letta/settings.py:15`, `letta/settings.py:640-648`) making flags restart-only, case-sensitive `"prod"`/`"PRODUCTION"` gating comparisons, and a still-live legacy configparser config (`letta/config.py:41`) used by Alembic.

## Evidence Collected

Every entry includes a workspace-relative file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Config loader | `load_config_file()` collects candidate paths, deep-merges YAML | letta/config_file.py:59-100 |
| Layering precedence doc | Header documents `~/.letta/conf.yaml`, `./conf.yaml`, `LETTA_CONFIG_PATH`; env vars win | conf.yaml:1-9 |
| Config locations | `DEFAULT_USER_CONFIG = ~/.letta/conf.yaml`, `DEFAULT_PROJECT_CONFIG = ./conf.yaml` | letta/config_file.py:55-56 |
| Env override path | `LETTA_CONFIG_PATH` appended to candidate list | letta/config_file.py:79-81 |
| Deep merge | `_deep_merge()` — later files override earlier, recursive for dicts | letta/config_file.py:103-111 |
| Silent failure | YAML read wrapped in `try/except Exception: pass` | letta/config_file.py:92-98 |
| YAML→env flattening | `config_to_env_vars()` maps `letta:`→`LETTA_*`, `model:`→provider prefixes, `tool:`→prefix-based, `datadog:`→`DD_*` | letta/config_file.py:177-211 |
| Env precedence | `apply_config_to_env()` only sets vars not already in `os.environ` | letta/config_file.py:214-232 |
| Import-time application | `apply_config_to_env()` runs at module import, before settings objects exist | letta/settings.py:11-15 |
| Settings singletons | Eight BaseSettings instances created once at module level | letta/settings.py:640-648 |
| Main settings class | `Settings(BaseSettings)` with `env_prefix="letta_"`, `extra="ignore"` | letta/settings.py:278-279 |
| `.env` file support | `ModelSettings` reads `env_file=".env"` | letta/settings.py:115 |
| Environment field | `settings.environment` described as "(prod, dev, canary, etc. - lowercase values used for OTEL tags)" | letta/settings.py:284-287 |
| Test env isolation | `TestSettings` uses `letta_test_` prefix and `~/.letta/test` dir | letta/settings.py:514-517 |
| DB selection | `letta_pg_uri` property falls back to localhost default; `database_engine` picks POSTGRES vs SQLITE | letta/settings.py:471-493 |
| Engine config from settings | NullPool when `disable_sqlalchemy_pooling`, else pool sizing fields | letta/server/db.py:30-43 |
| Forced SSL default | asyncpg `connect_args["ssl"]="require"` unless URI already specifies sslmode | letta/server/db.py:44-53 |
| Alembic URI from settings | `alembic/env.py` derives sync URL via `settings.database_engine` / `get_database_uri_for_context` | alembic/env.py:22-26 |
| Legacy config system | `LettaConfig` dataclass + configparser, path from `MEMGPT_CONFIG_PATH` | letta/config.py:40-42 |
| Legacy config used by migrations | `LettaConfig.load()` imported in Alembic env for SQLite fallback | alembic/env.py:7,26 |
| CLI entry point | `letta = "letta.main:app"`; bare invocation defaults to running server | pyproject.toml:84-85; letta/main.py:12-16 |
| Server CLI options | `--port/--host/--debug/--secure/--localhttps` typer options | letta/cli/cli.py:17-26 |
| All-in-one image | Runtime stage based on `pgvector/pgvector:0.8.1-pg15`, installs redis-server + otelcol-contrib + Node | Dockerfile:42-72 |
| Build-time env default | `ARG LETTA_ENVIRONMENT=DEV` set in both stages | Dockerfile:14-15,79-80 |
| Auto-internal Redis | startup.sh starts local redis-server if `LETTA_REDIS_HOST` unset | letta/server/startup.sh:24-35 |
| Auto-internal Postgres | startup.sh launches embedded postgres if `LETTA_PG_URI` unset | letta/server/startup.sh:39-51 |
| Migration gate | `alembic upgrade head` failure aborts container start | letta/server/startup.sh:54-60 |
| OTEL collector mode select | ClickHouse vs Signoz vs file-export collector config chosen by env vars | letta/server/startup.sh:80-91 |
| Compose service mode | `compose.yaml`: letta_server + pgvector sidecar + nginx reverse proxy | compose.yaml:1-65 |
| Dev compose | `development.compose.yml`: source mounts, `WATCHFILES_FORCE_POLLING=true`, reload | development.compose.yml:1-28 |
| vLLM sidecar mode | `docker-compose-vllm.yaml`: letta + vLLM GPU service wired by env | docker-compose-vllm.yaml:1-30 |
| Test/dev compose variant | `dev-compose.yaml` builds local Dockerfile, separate pgdata volume | dev-compose.yaml:1-45 |
| Server engine toggles | uvloop policy, Granian engine, uvicorn fallback all keyed off settings | letta/server/rest_api/app.py:901-911,916-948,959-987 |
| Uvicorn knobs | `uvicorn_workers/reload/timeout_keep_alive` settings | letta/settings.py:397-399 |
| Secure mode | Password middleware enabled by `LETTA_SERVER_PASSWORD` env or `--secure` argv | letta/server/rest_api/app.py:172,797-799 |
| Sentry env tag | Sentry initialized with `environment=os.getenv("LETTA_ENVIRONMENT", "undefined")` | letta/server/rest_api/app.py:300-308 |
| Datadog env wiring | `DD_ENV` etc. derived from `settings.environment or "development"` before ddtrace init | letta/server/rest_api/app.py:310-323 |
| CORS origins | Hardcoded localhost list + `ACCEPTABLE_ORIGINS` env append + hardcoded `app.letta.com` push at app creation | letta/settings.py:244-256; letta/server/rest_api/app.py:795,811-817 |
| Readiness gates config | `ReadinessSettings` master switch, drain 503, event-loop-lag/in-flight/admission thresholds with ge/le constraints | letta/settings.py:600-637 |
| Readiness endpoint | `/v1/ready/` returns 503 on warming/degraded/draining only when enforcement enabled | letta/server/rest_api/routers/v1/health.py:24-45 |
| Readiness state machine | `warming→ready→degraded/draining` transitions tracked with metrics | letta/monitoring/readiness_state.py:8-55 |
| Load gate | Per-pod fg/bg in-flight and admission-wait gates using ReadinessSettings | letta/monitoring/load_gate.py:1-40 |
| Feature flags (static) | Boolean toggles: `use_lettuce_for_file_uploads`, `use_letta_v1_agent`, `enable_batch_job_polling`, `use_vertex_structured_outputs_experimental`, `use_asyncio_shield` | letta/settings.py:409-412,419,460 |
| Provider feature flags | `anthropic_sonnet_1m` / `anthropic_opus_1m` beta context-window switches | letta/settings.py:182-201 |
| Deployment-safety flag | `mcp_disable_stdio=True` default documented as unsuitable-for-multi-tenant guard | letta/settings.py:45-54 |
| Security flag | `no_default_actor` disables default-actor fallback | letta/settings.py:466-469 |
| Experimental YAML section | Explicit "Experimental" block in shipped conf template | conf.yaml:106-110 |
| Range validation examples | `otel_preferred_temporality` (ge=0,le=2), LLM timeouts (ge=10,le=1800), Datadog port (ge=1,le=65535) | letta/settings.py:379,429-432,543 |
| Env-name normalization | `_normalize_environment_tag` maps DEV/DEVELOPMENT/STAGING→dev, else lowercases | letta/otel/resource.py:13-31 |
| Prod-gated token counting | Anthropic count-tokens API only when `settings.environment == "PRODUCTION"` | letta/agent.py:1302-1305 |
| Prod-gated tools | `send_message_to_agent` raises on Letta Cloud when `environment == "prod"` | letta/functions/function_sets/multi_agent.py:171-173 |
| Prod-gated tool upserts | Base-tool set excludes local-only tools when `environment == "prod"` | letta/services/tool_manager.py:649-652 |
| Case-sensitive device.id gate | `device.id` resource attribute emitted unless `_env != "prod"` exact match | letta/otel/resource.py:56-57 |
| Test parametrization of env | Fixtures parametrize `settings.environment` with both `"prod"` and `"PRODUCTION"` | tests/managers/conftest.py:779-786; tests/test_managers.py:1022-1027 |
| Test-time flag mutation | Tests mutate singleton attributes directly (`settings.use_tpuf = True`) and restore | tests/conftest.py:161-165 |
| Optional-dependency packaging | `postgres`, `redis`, `sqlite`, `server`, `desktop`, `experimental`, `bedrock`, `modal` extras shape install footprint | pyproject.toml:87-161 |
| Image publishing | Single versioned+latest image pushed to Docker Hub on release, amd64+arm64 | .github/workflows/docker-image.yml:33-41 |
| Runtime sandbox config API | Per-agent sandbox configs/env-vars stored in DB, CRUD via REST | letta/server/rest_api/routers/v1/sandbox_configs.py:26-59 |
| memfs split-mode setting | `memfs_service_url` proxies git memory ops to a dedicated sidecar service | letta/settings.py:341-347 |

## Answers to Dimension Questions

### 1. Is configuration layered?
**Yes — explicitly, in four layers with defined precedence.**
1. **Defaults** in pydantic field definitions (`letta/settings.py:22-502`).
2. **YAML file(s)**: up to three files merged deepest-last — `~/.letta/conf.yaml`, `./conf.yaml`, then `LETTA_CONFIG_PATH`, with explicit-path argument highest (`letta/config_file.py:69-100`). Merge semantics: recursive dict overlay (`letta/config_file.py:103-111`).
3. **Environment variables**: applied last-wins because `apply_config_to_env()` skips keys already present in `os.environ` (`letta/config_file.py:229-232`); pydantic-settings then reads them with prefixes (`letta/settings.py:279`, `75`, `521`, `535`, `603`).
4. **Process arguments**: a few legacy paths sniff `sys.argv` directly — `--use-file-pg-uri` at import time (`letta/settings.py:264-270`), `--debug`/`--secure`/`--no-generation`/`--localhttps` inside `app.py` (`letta/server/rest_api/app.py:410,797,869,913`).

The mapping between YAML sections and env prefixes is bidirectional and documented both in the shipped template (`conf.yaml:5-9`) and the loader docstring (`letta/config_file.py:6-10`). One wrinkle: layering is implemented by *writing into the process environment* at import time (`letta/settings.py:15`), which is global mutable state — any code reading `os.environ` before `letta.settings` import sees different values, and the YAML layer is invisible afterwards (you cannot tell whether a value came from file or shell).

### 2. Are environments managed cleanly?
**Partially. There is a single mechanism (`settings.environment`) but no enforced vocabulary and inconsistent consumers.**
- The field is free-form text (`letta/settings.py:284-287`); the Docker build defaults it to `DEV` (`Dockerfile:14-15`).
- Telemetry normalizes known values for tags (`letta/otel/resource.py:13-31`), and Datadog/Sentry consume it (`letta/server/rest_api/app.py:303,312`).
- But behavioral gating compares raw strings case-sensitively with two different spellings: `"PRODUCTION"` in `letta/agent.py:1303` versus `"prod"` in `letta/functions/function_sets/multi_agent.py:172`, `letta/services/tool_manager.py:652`, and `letta/services/helpers/agent_manager_helper.py:1307`. Setting `LETTA_ENVIRONMENT=production` or `PROD` would silently skip *both* gates. Tests encode this ambiguity by parametrizing fixtures with each spelling separately (`tests/managers/conftest.py:779-786` uses `None|"prod"`; `tests/test_managers.py:1022-1027` uses `None|"PRODUCTION"`).
- Staging appears only as a tag-normalization input mapped onto dev (`letta/otel/resource.py:27-28`); there is no staging-specific configuration surface in-repo. No evidence of per-environment config files; environments differ solely through injected variables, which keeps parity high but validation absent.

### 3. Are deployment modes documented?
**Implemented well in code and Compose examples; formal documentation lives outside the repo.**
- **All-in-one (embedded deps)**: the runtime image bundles Postgres (pgvector base), Redis, Node, and otelcol (`Dockerfile:42-98`); `startup.sh` conditionally starts internal Postgres/Redis only when `LETTA_PG_URI`/`LETTA_REDIS_HOST` are absent, waits on health, runs Alembic, and fails fast on migration error (`letta/server/startup.sh:24-60`).
- **Service/sidecar**: `compose.yaml` wires server + pgvector + nginx (`compose.yaml:23-64`); `docker-compose-vllm.yaml` adds a GPU vLLM sidecar configured purely via `LETTA_LLM_ENDPOINT*` vars; `scripts/docker-compose.yml` provides bare Postgres+Redis for development.
- **Dev mode**: `development.compose.yml` mounts source with hot-reload polling; `uvicorn_reload`/Granian reload paths configurable in settings (`letta/server/rest_api/app.py:959-977`).
- **Split-service mode**: `memfs_service_url` redirects git-memory operations to a dedicated service (`letta/settings.py:341-347`), and provider traces can be dual-written to postgres/clickhouse/socket backends (`letta/settings.py:570-597`).
- **Client access** is exclusively HTTP via the external `letta-client` SDK (`pyproject.toml:46`; e.g., `tests/conftest.py:65`); the old in-process embedded client is gone from `letta/client/__init__.py` (empty file), leaving `desktop` as a vestigial extra (`pyproject.toml:142-158`). README defers to docs.letta.com (`README.md:13,33`), so in-repo "documentation" of modes is effectively the compose files themselves.

### 4. Are feature flags supported?
**Yes, as static typed booleans; no dynamic/runtime flag service.**
- Roughly 30+ toggle-style settings ship as `BaseSettings` booleans, including an explicit `# Experimental` section in both code (`letta/settings.py:409-412`) and the YAML template (`conf.yaml:106-110`). Examples: `use_lettuce_for_file_uploads` (Temporal-based upload pipeline gate, `letta/settings.py:412`), `use_letta_v1_agent` architecture switch (`letta/settings.py:460`), `enable_batch_job_polling` cron gate (`letta/settings.py:419`), beta provider features like `anthropic_sonnet_1m` with removal notes in their descriptions (`letta/settings.py:182-191`).
- Flags are read from import-time singletons (`letta/settings.py:640-648`), so changes require process restart; tests confirm the pattern of mutating attributes in-process (`tests/conftest.py:161-165`) rather than any refresh mechanism.
- A search for LaunchDarkly/Statsig/Unleash/generic `feature_flag` strings returned **no matches** across the source tree.
- One adjacent runtime-config surface exists: per-agent/per-org sandbox execution settings (env vars, pip requirements) are stored in the database and edited over REST (`letta/server/rest_api/routers/v1/sandbox_configs.py:26-59`), but these are domain configuration, not release flags.

### 5. Is configuration validated?
**Types yes, ranges selectively, cross-field consistency no, malformed files silently accepted.**
- Pydantic enforces types/coercion for all settings classes and applies range guards on selected numeric fields: temporality enum-range (`letta/settings.py:379`), LLM request/stream timeouts bounded 10–1800 s (`letta/settings.py:429-432`), readiness thresholds (`letta/settings.py:614-637`), Datadog port (`letta/settings.py:543`).
- Gaps:
  - YAML parse/read failures are swallowed with `except Exception: pass` (`letta/config_file.py:92-98`) — a typo'd `conf.yaml` yields defaults with zero signal.
  - Every settings class sets `extra="ignore"`, so misspelled keys vanish silently.
  - Partial DB specs degrade quietly: if only some of `pg_db/pg_user/pg_password/pg_host/pg_port` are set, `letta_pg_uri` falls back to the localhost default rather than erroring (`letta/settings.py:471-478`).
  - `plugin_register_dict` does ad-hoc string parsing that can raise unhandled `ValueError` on malformed input (`letta/settings.py:496-502`).
  - There is no startup dump/log of effective resolved configuration (only incidental logging such as the PostgreSQL `statement_timeout` probe, `letta/server/rest_api/app.py:209-225`).
  - **No unit tests target the config layer itself**: searches for `apply_config_to_env`/`config_to_env_vars` usages found only production call sites (`letta/settings.py:15`), never test files.

## Architectural Decisions

1. **Config-file-to-env bridging instead of native YAML settings.** Rather than teaching pydantic-settings about YAML, Letta translates the YAML hierarchy into environment variables before settings construction (`letta/config_file.py:177-232`, invoked at `letta/settings.py:15`). This gives every subsystem (including third-party libs like ddtrace and the OTel collector launcher) a uniform `ENV_VAR` contract, at the cost of environment pollution and losing provenance of values.
2. **One image, dependency-inclusive by default.** The decision to base the runtime image on `pgvector/pgvector` and auto-start internal Postgres/Redis when external URIs are missing (`Dockerfile:42-72`, `letta/server/startup.sh:24-51`) makes "same binary dev→prod via env only" literally true, and makes first-run UX trivial (`docker run` works with zero env).
3. **Free-form environment string with localized gating.** Instead of an environment enum, a single optional string drives telemetry tags and scattered prod checks (`letta/settings.py:284-287`). This is flexible but delegates correctness (spelling/case) to operators.
4. **Typed faceted settings classes.** Splitting concerns into eight focused `BaseSettings` subclasses with distinct env prefixes (`letta/settings.py:640-648`) avoids one god-object and namespaces the variable space (`LETTA_`, `LETTA_LOGGING_`, `LETTA_TELEMETRY_`, `LETTA_READINESS_`, ...).
5. **Static flags over dynamic flags.** All behavioral toggles are restart-time booleans; operational safety is instead handled by runtime *readiness* machinery (`ReadinessSettings`, `letta/monitoring/load_gate.py:14-40`, `/v1/ready/` in `letta/server/rest_api/routers/v1/health.py:24-45`) — degradation is a health-probe concern, not a config-reload concern.
6. **Migration-gated boot.** Container startup hard-fails if `alembic upgrade head` fails (`letta/server/startup.sh:54-60`), tying schema state to deploy success rather than lazily failing requests.

## Notable Patterns

- **Precedence documentation co-located with data**: `conf.yaml` doubles as living documentation, annotating each key with its env-var name (`conf.yaml:5-9,134,144`).
- **Deep-merge composition** of user/project/explicit config files enabling org-wide baselines overridden by project-local tweaks (`letta/config_file.py:69-111`).
- **Env-driven backend selection** recurring across subsystems: DB engine POSTGRES/SQLITE (`letta/settings.py:492-493`), trace backends comma-list `postgres,clickhouse,socket` with property parsing (`letta/settings.py:571-597`), OTEL collector export target (`letta/server/startup.sh:80-91`).
- **Credential-presence-derived capability**: sandbox type inferred from which API keys exist — E2B if `e2b_api_key`, else LOCAL (`letta/settings.py:62-71`); Modal detected via paired tokens (`letta/settings.py:57-59`).
- **Graceful-degradation wrappers**: Datadog, watchdog, NLTK prefetch, and instrumentation setup each log-and-continue on failure so observability misconfig cannot block serving (`letta/server/rest_api/app.py:186-207,310-408`).
- **Security posture via defaults**: MCP stdio transport disabled by default for multi-tenant safety with rationale in the field description (`letta/settings.py:45-54`); opt-out intended only for local deployments.

## Tradeoffs

- **Uniformity vs transparency**: bridging YAML→env means every consumer shares one interface, but misconfigured files fail invisibly (`letta/config_file.py:97-98`) and effective-config introspection is lost.
- **Zero-config first run vs implicit coupling**: embedding Postgres/Redis in the image (`Dockerfile:42-72`) maximizes accessibility but produces a heavyweight image and process supervision handled in shell rather than a supervisor/init system.
- **Flexibility vs correctness of `environment`**: free-form strings avoid release-coupled enums but have produced divergent literals (`letta/agent.py:1303` vs `letta/services/tool_manager.py:652`).
- **Restart-safe simplicity vs iteration speed**: static flags are predictable in production but make gradual rollouts impossible in-process; contrast with the DB-backed sandbox config which *is* runtime-mutable (`sandbox_configs.py:26-59`) — two different philosophies coexist.
- **Extras-based packaging** (`pyproject.toml:87-161`) slims installs but means the tested matrix (CI installs many extras together, `.github/workflows/core-unit-sqlite-test.yaml:29`) can drift from minimal user installs.

## Failure Modes / Edge Cases

- **Malformed `conf.yaml` silently ignored** → server starts entirely on defaults/env; operator believes custom pool sizes or timeouts are active when they are not (`letta/config_file.py:92-98`).
- **Case-sensitive prod gates mis-fire**: `LETTA_ENVIRONMENT=Production` disables the Anthropic count-token endpoint path *and* fails to block cloud-disallowed agent-to-agent messaging tools (`letta/agent.py:1303`; `letta/functions/function_sets/multi_agent.py:172`).
- **Partial Postgres env specs** (e.g., password omitted) fall through to the `letta:letta@localhost` default instead of failing fast (`letta/settings.py:471-478`).
- **Import-order sensitivity**: because `apply_config_to_env()` mutates `os.environ` at `import letta.settings`, anything importing settings before setting env vars locks in stale values; conversely `--use-file-pg-uri` detection depends on `sys.argv` contents at import time (`letta/settings.py:262-270`), fragile under programmatic/embedded use.
- **Singleton flag mutation leaks across tests/workers**: tests must manually save/restore attributes like `settings.use_tpuf` (`tests/conftest.py:161-165`); multi-worker uvicorn/Granian processes each rebuild singletons independently, so per-worker divergence is possible for non-deterministic inputs.
- **CORS list mutated at app creation** — `settings.cors_origins.append("https://app.letta.com")` runs on every `create_application()` call, repeatedly appending under repeated invocations within one process (`letta/server/rest_api/app.py:795`).
- **Legacy config duality**: Alembic still consults `LettaConfig.load()` for the SQLite fallback path (`alembic/env.py:7,26`), so migration behavior depends on the deprecated configparser file even in fully env-driven deployments.
- **Silent argv sniffing**: `--secure`/`--debug`/`--localhttps` are honored only if they appear verbatim in `sys.argv` (`letta/server/rest_api/app.py:410,797,913`), bypassing the documented CLI options and env equivalents (`LETTA_SERVER_SECURE` exists but only alongside argv check, `app.py:797`).

## Future Considerations

- Introduce an environment enum or normalized comparison helper (e.g., `is_prod()`) and migrate the five comparison sites (`letta/agent.py:1303`, `letta/functions/function_sets/multi_agent.py:172`, `letta/services/tool_manager.py:652`, `letta/services/helpers/agent_manager_helper.py:1307`, `letta/otel/resource.py:57`) to eliminate casing hazards.
- Fail loudly (or at least warn + metric) on YAML parse errors and unknown keys; add an effective-config debug endpoint or startup log line redacting secrets.
- Add direct unit tests for `load_config_file` precedence, `_deep_merge`, and `config_to_env_vars` mapping — currently the most load-bearing untested code in the repo's config path.
- Retire or quarantine `LettaConfig` (`letta/config.py:41`) so migrations depend solely on `Settings`.
- Replace `sys.argv` sniffing with explicit CLI/env plumbing; consolidate `LETTA_SERVER_SECURE` handling.
- Consider runtime-refreshable flags (DB- or file-backed) for the experimental toggles if rollout velocity matters; alternatively document restart requirement prominently in `conf.yaml`.

## Questions / Gaps

- **Staging parity**: no in-repo artifact defines a staging topology (only a tag alias in OTEL normalization, `letta/otel/resource.py:27-28`). How Letta Cloud actually stages deploys is not visible in this source. — Searched `README.md`, `.github/workflows/*`, compose files; no staging manifests found.
- **Dynamic feature flags**: none exist in-tree (searched for common flag-service names; no matches). If cloud deployments use remote flags, that logic lives outside this repository.
- **Kubernetes manifests/Helm charts**: readiness/drain plumbing strongly implies k8s operation (`ReadinessSettings` comments reference k8s draining, `letta/settings.py:607-610`), but no deployment charts are included in-repo. No clear evidence found within the studied boundary.
- **Config hot-reload**: no watcher or reload path for `conf.yaml` was found beyond uvicorn/Granian code reload (`app.py:929-932`); concluded restart-required, based on singleton construction at `letta/settings.py:640-648`.

---

Generated by `22.02-configuration-and-deployment-shape` against `letta`.

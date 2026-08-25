# Source Analysis: langgraph

## Dimension 22.02 — Configuration and Deployment Shape

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (core, CLI, checkpoints) + TypeScript (sdk-js); monorepo under `libs/` |
| Analyzed | 2026-08-25 |

## Summary

LangGraph's configuration story is split across two layers. The **core library** (`libs/langgraph`) has almost no deployment configuration: it exposes runtime context via `RunnableConfig` accessors (`libs/langgraph/langgraph/config.py:17-29`) and reads exactly two env-tunable constants at import time (`libs/langgraph/langgraph/_internal/_config.py:32-35`). Nearly all configuration and deployment machinery lives in the **CLI package** (`libs/cli`), which is built around a single JSON file, `langgraph.json`, validated by `validate_config_file` (`libs/cli/langgraph_cli/config.py:608-648`) against a fully documented `TypedDict` schema (`libs/cli/langgraph_cli/schemas.py:615-773`). From that one file the CLI derives five distinct deployment shapes: an in-process dev server (`langgraph dev`, `libs/cli/langgraph_cli/cli.py:758-862`), a local docker-compose stack with Redis/Postgres/debugger (`langgraph up`, `libs/cli/langgraph_cli/docker.py:190-301`), a production OCI image (`langgraph build`, `libs/cli/langgraph_cli/cli.py:380-464`), a managed-platform deployment with local or remote builds (`langgraph deploy`, `libs/cli/langgraph_cli/deploy.py`), and a client-side "embedded" consumption mode via `RemoteGraph` (`libs/langgraph/langgraph/pregel/remote.py:118-127`). A second axis, engine runtime mode (`combined_queue_worker` vs `distributed`, `libs/cli/langgraph_cli/cli.py:172-177`), swaps base images and adds orchestrator/executor compose services.

The model is clear and heavily validated (50+ unit tests on config alone, e.g. `libs/cli/tests/unit_tests/test_config.py:136`), but there is **no true config layering**: one flat JSON file, no environment profiles, no merge of base/override files. "Environments" are handled implicitly through env vars and license keys rather than a first-class dev/staging/prod concept, and there is no feature-flag system — only static startup toggles such as the `disable_*` HTTP route switches in `HttpConfig`.

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale: configuration has a single authoritative schema (`libs/cli/langgraph_cli/schemas.py:615-773`), a strict validator with actionable remediation messages (`libs/cli/langgraph_cli/config.py:323-560`), typo detection with did-you-mean suggestions (`libs/cli/langgraph_cli/config.py:593-605`), build-command injection sanitization (`libs/cli/langgraph_cli/config.py:20-38,72-78`), reserved-env-var protection for platform deploys (`libs/cli/langgraph_cli/deploy.py:34-79`), and dense test coverage (`libs/cli/tests/unit_tests/test_config.py`, `test_docker.py`, `test_deploy_helpers.py`). The same built image can be promoted across environments with env-only changes. It falls short of 8+ because layering/environment profiles are absent (single-file config; no staging/prod distinction beyond host URLs), the closest thing to feature flags are static route toggles, validation error types are inconsistent (`click.UsageError` vs bare `ValueError`, compare `libs/cli/langgraph_cli/config.py:345` with `config.py:525`), and at least one flag consumed by code (`disable_persistence`, `libs/cli/langgraph_cli/cli.py:859`) is not in the known-keys set (`libs/cli/langgraph_cli/config.py:564-590`), so `langgraph validate` warns about it as unknown.

## Evidence Collected

Every entry cites workspace-relative paths from the selected source directory.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Config loader | `validate_config_file` loads `langgraph.json` as JSON then delegates to `validate_config`; cross-checks Node `engines` in sibling `package.json` | `libs/cli/langgraph_cli/config.py:608-648` |
| Config schema | `Config` TypedDict documents every top-level key (python_version, graphs, env, store, auth, http, webhooks, …) | `libs/cli/langgraph_cli/schemas.py:615-773` |
| Defaulting / canonicalization | `validate_config` fills defaults (python 3.11, debian distro) and rebuilds the dict with a fixed key set | `libs/cli/langgraph_cli/config.py:367-389` |
| Known-key allowlist | `_KNOWN_CONFIG_KEYS` used to detect typos; `get_unknown_keys` emits difflib "did you mean" warnings | `libs/cli/langgraph_cli/config.py:564-605` |
| Validate command | `langgraph validate` prints valid/invalid plus unknown-key warnings | `libs/cli/langgraph_cli/cli.py:870-904` |
| Version constraints | Min Python 3.11 / Node 20 enforced with remediation hints; deprecated bullseye distro rejected | `libs/cli/langgraph_cli/config.py:81-82,395-453,470-482` |
| api_version pinning | Semver parsing incl. prerelease ranges `~=`/`>~=` resolved against live PyPI index | `libs/cli/langgraph_cli/config.py:39-51,250-294` |
| Env management (config field) | `env` accepts inline dict or `.env` path | `libs/cli/langgraph_cli/schemas.py:717-727` |
| Env resolution fallback | Deploy resolves env from config field or falls back to cwd `.env`; missing file warned, None values filtered | `libs/cli/langgraph_cli/deploy.py:504-533`; tests at `libs/cli/tests/unit_tests/test_deploy_helpers.py:162-240` |
| Env → container injection | `_build_runtime_env_vars` bakes `LANGGRAPH_STORE/AUTH/ENCRYPTION/HTTP/WEBHOOKS/CHECKPOINTER/UI/UI_CONFIG` and `LANGSERVE_GRAPHS` into Dockerfile `ENV` lines | `libs/cli/langgraph_cli/config.py:1132-1162` |
| Compose env wiring | `config_to_compose` emits env vars/env_file and docker-compose `develop.watch` rebuild triggers | `libs/cli/langgraph_cli/config.py:1651-1723` |
| Reserved env var blocklist | `RESERVED_ENV_VARS` (POSTGRES_URI, REDIS_URI, license keys, tracing controls…) silently skipped when uploading secrets | `libs/cli/langgraph_cli/deploy.py:34-79,542-554` |
| Dev vs prod credential split | `langgraph up` banner: local dev needs `LANGSMITH_API_KEY`; production needs `LANGGRAPH_CLOUD_LICENSE_KEY` | `libs/cli/langgraph_cli/cli.py:295-299` |
| Host environment awareness | `_smith_dashboard_base_url` maps prod/dev/eu/staging `api.host.langchain.com` hosts to matching dashboards | `libs/cli/langgraph_cli/deploy.py:661-679`; tests at `libs/cli/tests/unit_tests/test_deploy_helpers.py:550-573` |
| Deployment mode: in-mem dev server | `langgraph dev` with hot reload, debugpy port, tunnel, SSL options; requires `langgraph-cli[inmem]` extra | `libs/cli/langgraph_cli/cli.py:663-862` |
| Deployment mode: local compose stack | `compose_as_dict` builds Redis 6 + pgvector Postgres 16 + optional debugger + API service with healthchecks | `libs/cli/langgraph_cli/docker.py:190-301` |
| Deployment mode: image build | `langgraph build` produces tagged OCI image; JS vs Python build context selection | `libs/cli/langgraph_cli/cli.py:380-464`; `libs/cli/langgraph_cli/docker.py:333-400` |
| Deployment mode: platform deploy | `langgraph deploy` group [Beta]: create/update deployments, revisions, push tokens, GCS source upload | `libs/cli/langgraph_cli/deploy.py:1540-1560`; `libs/cli/langgraph_cli/host_backend.py:73-205` |
| Local vs remote build resolution | `_resolve_build_mode` auto-detects Docker availability; `--remote/--no-remote` override; `--image` forces local | `libs/cli/langgraph_cli/deploy.py:562-590,1500-1510,1629-1636` |
| Revision sources | `internal_docker` (pushed image digest) vs `internal_source` (uploaded tarball remote build) | `libs/cli/langgraph_cli/deploy.py:1651-1662`; `libs/cli/langgraph_cli/host_backend.py:120-172` |
| Engine runtime modes | `--engine-runtime-mode combined_queue_worker\|distributed`; distributed sets `N_JOBS_PER_WORKER="0"`, picks `langchain/langgraph-executor` base image, adds orchestrator+executor services | `libs/cli/langgraph_cli/cli.py:172-177`; `libs/cli/langgraph_cli/docker.py:267-268`; `libs/cli/langgraph_cli/config.py:1554-1563,1725-1778` |
| Source/deps modes | Dependency-list pip installs vs uv-workspace `source.kind=uv` with lockfile-driven builds | `libs/cli/langgraph_cli/schemas.py:593-612`; `libs/cli/langgraph_cli/config.py:484-513`; `libs/cli/langgraph_cli/uv_lock.py:1-80` |
| Client/embedded mode | `RemoteGraph` wraps LangGraph Server API so deployed graphs embed as nodes in other graphs | `libs/langgraph/langgraph/pregel/remote.py:118-127,132-179` |
| Feature-flag-like toggles | `HttpConfig.disable_*`: assistants, threads, runs, store, mcp, a2a, meta, ui, webhooks routes | `libs/cli/langgraph_cli/schemas.py:442-536` |
| Feature-flag-like toggles (behavior) | `middleware_order: auth_first\|middleware_first`; `enable_custom_route_auth`; `pip_installer auto/pip/uv`; `keep_pkg_tools` | `libs/cli/langgraph_cli/schemas.py:511-529`; `libs/cli/langgraph_cli/config.py:410-416,1283-1293,1242-1260` |
| CLI telemetry opt-out | `LANGGRAPH_CLI_NO_ANALYTICS=1` short-circuits analytics | `libs/cli/langgraph_cli/analytics.py:89-90` |
| Core-library env knobs | Only `LANGGRAPH_DEFAULT_RECURSION_LIMIT` and `LANGGRAPH_DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT`, read once at import | `libs/langgraph/langgraph/_internal/_config.py:32-35` |
| Build-command sanitization | Disallowed shell metacharacters and lone `&` rejected to prevent injection via install/build commands | `libs/cli/langgraph_cli/config.py:20-38,72-78`; enforced at `libs/cli/langgraph_cli/cli.py:430-441` |
| Distro security advisory | `warn_non_wolfi_distro` recommends wolfi over debian/bookworm on every build/up/dockerfile | `libs/cli/langgraph_cli/util.py:14-43` |
| Validation tests | `test_validate_config` asserts full default canonicalization and failure cases | `libs/cli/tests/unit_tests/test_config.py:136-256` |
| Deployment-mode tests | Distributed compose topology and combined-mode N_JOBS assertions | `libs/cli/tests/unit_tests/test_docker.py:371-440` |
| Dockerfile generation tests | End-to-end Dockerfile generation for pip, uv-lock, node, multiplatform, webhooks, UI | `libs/cli/tests/unit_tests/test_config.py:657-1350` |

## Answers to Dimension Questions

**1. Is configuration layered?**
No. Configuration comes from exactly one JSON file (`langgraph.json`, default per `libs/cli/langgraph_cli/constants.py`, referenced at `libs/cli/langgraph_cli/cli.py:93`). There is no base-file/override-file merge, no per-environment config inheritance, and no CLI-level profile selection. Layering exists only *within* resolution order for individual values: click option → environment variable → `.env` file (e.g., API key resolution checks `--api-key` flag, then `.env`, then process env at `libs/cli/langgraph_cli/deploy.py:1237-1264`; deployment name via `envvar=_DEPLOYMENT_NAME_ENV` at `deploy.py:1330-1338`). Defaults are filled by canonicalization inside `validate_config` (`libs/cli/langgraph_cli/config.py:367-389`) rather than by layered files. Environment-variable values themselves are injected into images at build time (`_build_runtime_env_vars`, `config.py:1132-1162`), which means changing them requires rebuild or platform-level secret update — not re-layered at boot for self-hosted images.

**2. Are environments managed cleanly?**
Partially, but implicitly. There is no `dev/staging/prod` concept anywhere in the config schema. Instead: (a) credential shape distinguishes local dev (`LANGSMITH_API_KEY`) from licensed production (`LANGGRAPH_CLOUD_LICENSE_KEY`) — `libs/cli/langgraph_cli/cli.py:295-299`; (b) the deploy tooling recognizes prod/dev/eu/staging backend hostnames when deriving dashboard URLs — `libs/cli/langgraph_cli/deploy.py:661-679`, tested at `libs/cli/tests/unit_tests/test_deploy_helpers.py:550-573`; (c) everything else rides on the free-form `env` field or `.env` file (`libs/cli/langgraph_cli/schemas.py:717-727`). This keeps parity high — the same image runs anywhere with different env — but environment promotion policy is left entirely to the user; nothing in the repo models "staging".

**3. Are deployment modes documented?**
Yes, at the interface level. Each mode is a first-class CLI command with detailed help text embedding config examples (`up` at `libs/cli/langgraph_cli/cli.py:41-101`, `build` at `cli.py:380-417`, `dockerfile` at `cli.py:527-557`, `dev` at `cli.py:663-761`, `validate` at `cli.py:870-904`, `new` at `cli.py:912-922`). The `Config` TypedDict carries extensive docstrings that function as reference documentation (`libs/cli/langgraph_cli/schemas.py:615-773`). The distributed engine mode is documented in the option help ("'distributed' uses separate executor and orchestrator containers", `cli.py:172-177`) and its compose topology is pinned down by tests (`libs/cli/tests/unit_tests/test_docker.py:371-413`). The `deploy` command is explicitly labeled "[Beta]" in its group help (`libs/cli/langgraph_cli/deploy.py:1540-1560`). Repo-root docs (`docs/`) contain only generated redirect metadata, so narrative documentation lives outside this source tree — within the code itself, documentation quality is high.

**4. Are feature flags supported?**
No dedicated feature-flag system exists (grep for `FEATURE|feature_flag|feature-flag` across `libs/` returns nothing). What exists are static, config-time toggles: nine `disable_*` HTTP route switches (`libs/cli/langgraph_cli/schemas.py:451-499`), middleware ordering choice (`schemas.py:511-521`), installer/toolchain selection (`pip_installer`, `keep_pkg_tools` at `libs/cli/langgraph_cli/config.py:410-416,1242-1260`), and an analytics kill-switch env var (`libs/cli/langgraph_cli/analytics.py:89-90`). These are read once at startup/build and cannot be flipped at runtime without redeployment. The core library's only remotely flag-like knobs are two import-time env vars (`libs/langgraph/langgraph/_internal/_config.py:32-35`).

**5. Is configuration validated?**
Yes — this is the strongest part of the dimension. `validate_config` (`libs/cli/langgraph_cli/config.py:323-560`) enforces: minimum language versions with fix suggestions (`config.py:395-448`), distro allowlist (`config.py:471-482`), mutually exclusive internal fields (`config.py:343-347`), import-string format for graphs/auth/encryption/http/checkpointer paths (`config.py:521-543,882-887`), uv-source structural rules including mutual exclusion with `dependencies` (`config.py:484-513`), rejection of removed legacy keys with migration hints (`config.py:515-519`), reserved local-package names (`config.py:712-735`), and existence checks on every referenced local module (`config.py:893-897,940-943,1026-1031,1081-1084`). Unknown keys produce did-you-mean warnings via `get_unknown_keys` (`config.py:593-605`) surfaced by both `langgraph validate` (`cli.py:883-904`) and normal flows. Shell-metacharacter sanitization guards custom build commands (`config.py:20-78`). Coverage is backed by ~50 unit tests in `libs/cli/tests/unit_tests/test_config.py`. Weak spots: mixed exception types (bare `ValueError` at `config.py:525-528,548-551,555-559` vs `click.UsageError` elsewhere) and the `disable_persistence` key read at `libs/cli/langgraph_cli/cli.py:859` that is absent from `_KNOWN_CONFIG_KEYS` (`config.py:564-590`).

## Architectural Decisions

1. **One declarative file compiles to many targets.** `langgraph.json` is a single source of truth that lowers to three distinct artifacts: an inline Dockerfile (`python_config_to_docker` / `node_config_to_docker`, `libs/cli/langgraph_cli/config.py:1263-1551`), a docker-compose stdin document (`config_to_compose`, `config.py:1651-1780`), and a platform-API payload (`HostBackendClient.create_deployment`, `libs/cli/langgraph_cli/host_backend.py:73-92`). Compilation, not interpretation, is the core mechanism.
2. **Validate-and-canonicalize pipeline.** Every entry point funnels through `validate_config_file` before any artifact generation (`libs/cli/langgraph_cli/cli.py:445,562,822,1001`), guaranteeing downstream code sees a normalized dict.
3. **Path rewriting as a build step.** Host-relative graph/auth/encryption/checkpointer/app paths are rewritten to their in-container `/deps/...` locations before Dockerfile emission (`_update_graph_paths` et al., `libs/cli/langgraph_cli/config.py:827-1106`), keeping user config portable across host/container.
4. **Deployment shape selected by flags, not artifacts.** `engine_runtime_mode` (`combined_queue_worker` default) changes the base image, worker concurrency (`N_JOBS_PER_WORKER=0` disables in-process workers, `libs/cli/langgraph_cli/docker.py:267-268`), and adds orchestrator/executor services (`libs/cli/langgraph_cli/config.py:1725-1778`) — one config, two topologies.
5. **Secrets kept out of images where possible.** Platform deploys upload env-derived values as `secrets` to the control plane while filtering a reserved blocklist (`libs/cli/langgraph_cli/deploy.py:542-554`), and `.env` files are excluded from build contexts by generated `.dockerignore` (`libs/cli/langgraph_cli/cli.py:472-524`).
6. **Core library stays config-free.** `libs/langgraph` intentionally contains no deployment config surface; graph behavior tuning travels through `RunnableConfig`/contextvars (`libs/langgraph/langgraph/config.py:17-29`), separating framework concerns from ops concerns.

## Notable Patterns

- **Schema-as-documentation**: exhaustive TypedDict docstrings double as the config reference (`libs/cli/langgraph_cli/schemas.py` throughout), keeping help text adjacent to types.
- **Remediation-oriented errors**: nearly every validation failure prints a copy-pastable fix snippet, e.g. `'  "source": {"kind": "uv", "root": ".."}'` (`libs/cli/langgraph_cli/config.py:422-428`).
- **Capability probing before orchestration**: `check_capabilities` detects Docker/compose plugin-vs-standalone and healthcheck support, adapting generated compose output (`libs/cli/langgraph_cli/docker.py:100-142,250-254,287-293`).
- **Auto-degradation**: pip vs uv installer chosen from base-image version support (`_image_supports_uv`, `libs/cli/langgraph_cli/config.py:1228-1239`); node package manager inferred from lockfiles/`packageManager` (`config.py:1165-1222`); local vs remote build chosen from machine capability (`libs/cli/langgraph_cli/deploy.py:562-590`).
- **Dual human/JSON output**: `_Emitter` provides structured JSON-lines events for CI alongside colored text for humans (`libs/cli/langgraph_cli/deploy.py:127-282`).
- **Watch-mode parity**: `develop.watch` rebuild actions derived from config dependencies keep hot-reload semantics in containers (`libs/cli/langgraph_cli/config.py:1667-1683`).

## Tradeoffs

- **Simplicity vs layering**: the single-file model is easy to reason about but forces users to duplicate whole `langgraph.json` files per environment or manage divergence entirely through env vars/secrets.
- **Build-time baking vs runtime flexibility**: injecting `store`/`auth`/`http` configs as Dockerfile `ENV` JSON (`libs/cli/langgraph_cli/config.py:1132-1162`) simplifies the server but means config changes require image rebuilds in self-hosted flows; only the platform path supports secret rotation via API (`host_backend.py:120-139`).
- **Network-dependent pinning**: compatible `api_version` ranges resolve against live PyPI (`libs/cli/langgraph_cli/config.py:230-247,276`), improving freshness at the cost of build-time network dependence and nondeterminism.
- **Silent filtering for safety**: reserved env vars are dropped with only a note during deploy (`libs/cli/langgraph_cli/deploy.py:547-550`); safer than failing, but a mistyped `DATABASE_URI` in `.env` quietly never reaches the deployment.
- **Strictness vs extensibility**: the fixed canonical key set rejects unknown keys loudly, which catches typos but makes third-party extensions to the config file impossible without upstream changes.

## Failure Modes / Edge Cases

- **Import-time env capture**: `DEFAULT_RECURSION_LIMIT` is bound at module import (`libs/langgraph/langgraph/_internal/_config.py:32-35`); setting the env var after import (e.g., dynamically in tests) has no effect.
- **Undocumented key gap**: `disable_persistence` is passed to the in-mem server (`libs/cli/langgraph_cli/cli.py:859`) but is not in `_KNOWN_CONFIG_KEYS` (`libs/cli/langgraph_cli/config.py:564-590`), so `langgraph validate` flags it as unrecognized — schema and consumer have drifted.
- **Inconsistent validation severity**: some path-format violations raise plain `ValueError` (`libs/cli/langgraph_cli/config.py:525-528,532-536,539-543`) instead of `click.UsageError`, producing inconsistent UX depending on which rule trips.
- **Env quoting in Dockerfile**: config sub-dicts are serialized with `json.dumps` wrapped in single quotes into `ENV` lines (`config.py:1136-1153`); values are JSON-safe, but any future non-dict string interpolation into these lines would need shell quoting care.
- **Cross-compilation dependency**: deploys from ARM hosts require buildx; capability check fails closed with actionable message (`libs/cli/langgraph_cli/docker.py:80-94`, `can_build_locally`).
- **JS/Python asymmetry**: `langgraph dev` cannot serve JS graphs in the Python CLI (`libs/cli/langgraph_cli/cli.py:823-826`), so dev/prod parity is weaker for Node projects (they must use the npm CLI or docker flows).
- **Prebuilt-image platform gate**: `--image` deploys reject anything not `linux/amd64` up front (`libs/cli/langgraph_cli/deploy.py:369-402`), preventing late failures in the cloud build.

## Future Considerations

- Introduce named environment profiles (e.g., `langgraph.json` variants or `"environments": {...}` section) so dev/staging/prod deltas are declarative rather than convention.
- Add `disable_persistence` to the schema/known keys or remove the consumer, closing the drift between `libs/cli/langgraph_cli/cli.py:859` and `libs/cli/langgraph_cli/config.py:564-590`.
- Normalize all validation failures to `click.UsageError` for uniform exit codes and formatting.
- Promote the reserved-env-var blocklist to a hard warning summary listing skipped variables, reducing silent-drop surprises (`libs/cli/langgraph_cli/deploy.py:542-554`).
- Cache PyPI version lookups for `api_version` range resolution to make offline/reproducible builds possible (`libs/cli/langgraph_cli/config.py:230-247`).
- Graduate `langgraph deploy` out of Beta and stabilize its revision/source semantics (`internal_docker` vs `internal_source`) as a public contract (`libs/cli/langgraph_cli/deploy.py:1540-1560`).

## Questions / Gaps

- **Where does `N_JOBS_PER_WORKER` get consumed?** The distributed compose sets it (`libs/cli/langgraph_cli/docker.py:267-268`) and `langgraph dev` accepts `--n-jobs-per-worker` (`cli.py:685-690`), but the consuming server lives in the external `langgraph-api` package, which is not part of this source tree; its behavior could not be verified here (search boundary: `libs/` only).
- **No evidence found** of any runtime feature-flag service integration (LaunchDarkly-style) or staged rollout mechanism anywhere in `libs/`; searched for `FEATURE|feature_flag|feature-flag` and reviewed all env-var toggles listed above.
- **Narrative deployment documentation** (guides explaining when to choose `dev` vs `up` vs `build` vs `deploy`) is absent from this repository's `docs/` (only redirect metadata at `docs/`); if such docs exist, they live in an external docs repo, so "documented deployment modes" is assessed from in-code help text and type docstrings only.
- **Environment variable precedence for the API server at runtime** (process env vs baked `ENV` lines vs platform secrets) is determined inside the external `langgraph-api` server image, outside this source; only the generation side is verifiable here.

---

Generated by `22.02-configuration-and-deployment-shape` against `langgraph`.

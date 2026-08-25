# Source Analysis: crewai

## Dimension 22.02: Configuration and Deployment Shape

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python 3.10–3.13, uv workspace (6 packages: `crewai`, `crewai-core`, `cli`, `crewai-tools`, `crewai-files`, `devtools`), Pydantic v2, Click CLI |
| Analyzed | 2026-08-25 |

> Citation convention: all paths below are relative to the source root `studies/agent-harness-study/sources/crewai/`.

## Summary

CrewAI's configuration is **environment-variable-first with informal layering**, not a unified config system. There is no single typed runtime-settings registry; instead ~50 distinct `CREWAI_*` variables are read ad hoc via `os.getenv` across packages, supplemented by (a) a small JSON file-based `Settings` model persisted to `~/.config/crewai/settings.json` for platform/CLI state (`lib/crewai-core/src/crewai_core/settings.py:147`), (b) per-project `.env` files loaded by dotenv at multiple import points (`lib/crewai/src/crewai/project/crew_base.py:130`, `lib/crewai/src/crewai/llm.py:71`) and with `override=True` in the CLI runners (`lib/cli/src/crewai_cli/run_crew.py:313-317`), and (c) per-user consent data in `.crewai_user.json` (`lib/crewai-core/src/crewai_core/user_data.py:23-26`). Where layering does exist it follows a consistent implicit pattern — explicit argument > env var > file > hardcoded default — implemented independently in at least four subsystems (platform URL, tracing enablement, LLM provider keys/endpoints, settings file merge).

There are no named dev/staging/prod environments anywhere in the codebase; "environment" means the process context (CI, serverless, container, notebook — detected from marker variables in `lib/crewai-core/src/crewai_core/runtime_env.py:104-159`). Test parity is handled well: `.env.test` seeds fake provider credentials to mimic CI (`/.env.test:1-16`), and pytest fixtures isolate storage per test (`conftest.py:220-252`).

Deployment supports three shapes: embedded library use (`crew.kickoff()` in-process), local CLI execution through uv-managed project subprocesses (`lib/cli/src/crewai_cli/run_crew.py:750-771`), and hosted deployment to the CrewAI AMP platform via `crewai deploy create/push` using either a git remote or a ZIP upload carrying `.env` values (`lib/cli/src/crewai_cli/deploy/main.py:289-378`). The strongest part of this dimension is **validation**: pydantic validators gate LLM/provider config at construction, and a dedicated pre-deploy validator checks nine categories of project health before upload (`lib/cli/src/crewai_cli/deploy/validate.py:1-25`). Feature flags as a concept do not exist; boolean env toggles serve as de-facto flags.

## Rating

**Score: 6 / 10** — Present but inconsistent, weakly documented, occasionally fragile.

Rationale against the rubric:
- Layering exists and is applied consistently *where implemented* (param → env → file → default), and the file-based `Settings` model is tested including corruption and permission cases (`lib/cli/tests/test_config.py:49-64`, `lib/cli/tests/test_config.py:135-146`), which approaches the 7–8 band for that subsystem.
- But there is no unified config model: runtime knobs are scattered across dozens of ad-hoc `os.getenv` reads, three separate hand-rolled `.env` parsers exist (`lib/cli/src/crewai_cli/utils.py:120-139`, `lib/cli/src/crewai_cli/utils.py:179-189`, inline parsing at `lib/cli/src/crewai_cli/deploy/validate.py:885-892`), no dev/staging/prod environment concept exists, and feature flags are informal boolean toggles.
- Operational safeguards are real where they appear (0o600 atomic config writes `lib/crewai-core/src/crewai_core/settings.py:50-73`; telemetry that can never crash the app `lib/crewai-core/src/crewai_core/telemetry.py:66-74`; pre-deploy validation gates `lib/cli/src/crewai_cli/deploy/main.py:25-61`).

Answer to the dimension question — *"Can the same binary run in dev, staging, and prod with config changes only?"*: **Partially.** The same code runs locally or on the AMP platform with only `.env`/dashboard-env changes (`lib/cli/src/crewai_cli/deploy/main.py:537-543` sends `env` alongside the deploy payload), and non-interactive/headless behavior is switchable via `CREWAI_DMN` (`lib/cli/src/crewai_cli/utils.py:70-75`). But since CrewAI is a library rather than a server binary, "prod" is whatever process embeds it; there is no first-class environment abstraction, so parity relies entirely on callers managing env vars correctly.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| File-based user settings | `Settings` BaseModel persisted to JSON; `DEFAULT_CONFIG_PATH = ~/.config/crewai/settings.json` | `lib/crewai-core/src/crewai_core/settings.py:26`, `lib/crewai-core/src/crewai_core/settings.py:147-191` |
| Settings layering (file < kwargs) | `merged_data = {**file_data, **data}` — constructor kwargs override file contents | `lib/crewai-core/src/crewai_core/settings.py:211-220` |
| Corrupt-file tolerance | Invalid/unreadable JSON silently degrades to `{}` | `lib/crewai-core/src/crewai_core/settings.py:212-217`; test `lib/cli/tests/test_config.py:135-146` |
| Config path fallback chain | Default home path → temp dir → CWD → in-memory `/dev/null` sentinel | `lib/crewai-core/src/crewai_core/settings.py:76-107`, `lib/crewai-core/src/crewai_core/settings.py:196-201` |
| Secure writes | Atomic 0o600 temp-file replace; dedicated config dirs chmod'd 0o700, shared dirs skipped | `lib/crewai-core/src/crewai_core/settings.py:50-73`, `lib/crewai-core/src/crewai_core/settings.py:29-47`; tests `lib/cli/tests/test_config.py:164-201` |
| Key classification | `USER_SETTINGS_KEYS` / `CLI_SETTINGS_KEYS` / `READONLY_SETTINGS_KEYS` / `HIDDEN_SETTINGS_KEYS` drive reset & UI | `lib/crewai-core/src/crewai_core/settings.py:110-144`; enforced in `crewai settings set` at `lib/cli/src/crewai_cli/settings/main.py:77-96` |
| Shared settings module | Both `crewai.settings` and `crewai_cli.config` re-export `crewai_core.settings` | `lib/crewai/src/crewai/settings.py:9-18`, `lib/cli/src/crewai_cli/config.py:9-18` |
| dotenv loading (library) | Module-level `load_dotenv()` on import of crew base metaclass and `LLM` | `lib/crewai/src/crewai/project/crew_base.py:130`, `lib/crewai/src/crewai/llm.py:71` |
| dotenv loading (CLI) | `.env` in CWD loaded with `override=True` before running crews/flows | `lib/cli/src/crewai_cli/run_crew.py:313-317`, `lib/cli/src/crewai_cli/run_declarative_flow.py:70-74` |
| Import-order hazard awareness | litellm lazy-loaded specifically because its module-level `load_dotenv()` pollutes env (e.g., `MODEL=`) | `lib/crewai/src/crewai/llm.py:74-78`, `lib/crewai/src/crewai/llm.py:113-123` |
| Layered resolution: platform URL | arg > `CREWAI_PLUS_URL` env > settings file `enterprise_base_url` > `DEFAULT_CREWAI_ENTERPRISE_URL` constant | `lib/crewai-core/src/crewai_core/plus_api.py:174-178`; defaults `lib/crewai-core/src/crewai_core/constants.py:14-22` |
| Layered resolution: tracing | explicit override param > `CREWAI_TRACING_ENABLED` env > recorded user consent | `lib/crewai/src/crewai/events/listeners/tracing/utils.py:108-137`; mirrored for CLI display in `lib/crewai-core/src/crewai_core/user_data.py:77-91` |
| Layered resolution: OpenAI creds | `api_key` arg > `OPENAI_API_KEY`; endpoint arg > `OPENAI_BASE_URL` > `OPENAI_API_BASE`; missing endpoint hard-error when `custom_openai=True` | `lib/crewai/src/crewai/llms/providers/openai/completion.py:258-285` |
| YAML crew/task config merge | `process_config` folds nested `config:` dicts into Agent/Task pydantic models, never overriding explicit values | `lib/crewai/src/crewai/utilities/config.py:6-35`; used at `lib/crewai/src/crewai/task.py:376`, `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:522` |
| Declarative agent schema | `AgentConfig`/`TaskConfig` TypedDicts type the YAML surface (`config/agents.yaml`, `config/tasks.yaml`) | `lib/crewai/src/crewai/project/crew_base.py:42-127`; default paths resolved at `lib/crewai/src/crewai/project/crew_base.py:147-156` |
| Storage location config | `CREWAI_STORAGE_DIR` overrides appdirs-derived data dir | `lib/crewai-core/src/crewai_core/paths.py:11-26` |
| Runtime credential injection | Tool-repo credentials injected as `UV_INDEX_<NAME>_*` env vars into uv/poetry subprocesses | `lib/crewai-core/src/crewai_core/tool_credentials.py:12-56`; consumed at `lib/cli/src/crewai_cli/run_crew.py:466-477` |
| Per-request token override | `CREWAI_PLATFORM_INTEGRATION_TOKEN` env fallback behind a contextvar setter | `lib/crewai/src/crewai/context.py:25-48` |
| Environment detection | Marker tables for CI, serverless, PaaS, hosted IDEs, notebooks, containers; precedence-ordered | `lib/crewai-core/src/crewai_core/runtime_env.py:104-159`, `lib/crewai-core/src/crewai_core/runtime_env.py:231-268` |
| Coding-assistant detection | Session-scoped markers for claude_code/codex/cursor/etc.; closed literal return set prevents PII leakage | `lib/crewai-core/src/crewai_core/runtime_env.py:84-94`, `lib/crewai-core/src/crewai_core/runtime_env.py:176-189` |
| Headless/non-interactive mode | `CREWAI_DMN` truthiness check disables TUI/prompts across CLI | `lib/cli/src/crewai_cli/utils.py:70-75`; consumed at `lib/cli/src/crewai_cli/run_crew.py:334-335`, `lib/cli/src/crewai_cli/input_prompt.py:50` |
| Telemetry opt-out | `OTEL_SDK_DISABLED` ∨ `CREWAI_DISABLE_TELEMETRY` ∨ `CREWAI_DISABLE_TRACKING` | `lib/crewai-core/src/crewai_core/telemetry.py:232-237`; documented `docs/edge/en/telemetry.mdx:26` |
| Telemetry safety | Exporter swallows failures; singleton init failure leaves `ready=False`; never installs global tracer provider | `lib/crewai-core/src/crewai_core/telemetry.py:66-74`, `lib/crewai-core/src/crewai_core/telemetry.py:199-230`, `lib/crewai-core/src/crewai_core/telemetry.py:256-271` |
| Deployment: hosted mode | `DeployCommand.deploy/create_crew` push via git remote URL or ZIP upload incl. env vars | `lib/cli/src/crewai_cli/deploy/main.py:289-332`, `lib/cli/src/crewai_cli/deploy/main.py:347-378`, payload `lib/cli/src/crewai_cli/deploy/main.py:520-543` |
| Env upload for deploys | `.env` parsed and sent as deployment `env` map | `lib/cli/src/crewai_cli/utils.py:120-139`; used at `lib/cli/src/crewai_cli/deploy/main.py:365-366` |
| Pre-deploy validation gate | Blocking ERROR findings abort deploy unless `--skip-validate`; lockfile bootstrap interplay | `lib/cli/src/crewai_cli/deploy/main.py:25-61`, `lib/cli/src/crewai_cli/deploy/main.py:148-167` |
| Validator check catalog | Nine documented categories incl. stale lockfile, hatch target, import smoke, version pin | `lib/cli/src/crewai_cli/deploy/validate.py:1-25`, checks at `lib/cli/src/crewai_cli/deploy/validate.py:347-958` |
| Env cross-check validation | Regex-scans user code for referenced API-key vars and warns if absent from `.env`/os.environ | `lib/cli/src/crewai_cli/deploy/validate.py:854-912` (hint table `:87-108`) |
| Local runner mode | `crewai run` executes project scripts via `uv run` subprocess with augmented env | `lib/cli/src/crewai_cli/run_crew.py:750-771`; JSON crews run in project venv `lib/cli/src/crewai_cli/run_crew.py:441-493` |
| TUI→deploy chaining | Run TUI offers deploy; auth failure falls back to interactive login retry | `lib/cli/src/crewai_cli/run_crew.py:370-371`, `lib/cli/src/crewai_cli/run_crew.py:496-535` |
| Security escape hatches | `CREWAI_TOOLS_ALLOW_UNSAFE_PATHS` bypass vs `CREWAI_TOOLS_FORCE_SAFE_PATHS` tenant override (force wins + warns) | `lib/crewai-tools/src/crewai_tools/security/safe_path.py:7-11`, `lib/crewai-tools/src/crewai_tools/security/safe_path.py:79-89` |
| Test environment parity harness | `.env.test` fakes all provider keys "to mimic the GitHub Actions CI environment" | `/.env.test:1-16`, `/.env.test:118-131` |
| Test isolation fixtures | Loads `.env.test` then local `.env` (no override); per-test tmp `CREWAI_STORAGE_DIR`; `CREWAI_TESTING=true`; network blocked | `conftest.py:115-118`, `conftest.py:220-252`, `pyproject.toml:151` (`--block-network`) |
| VCR recording policy | Record mode from `PYTEST_VCR_RECORD_MODE`; forced `none` under `GITHUB_ACTIONS` | `conftest.py:411-427`, `/.env.test:152` |
| Docs: deployment story | Production guide points to CrewAI Enterprise + `crewai deploy create`; no self-hosted mode documented | `docs/edge/en/concepts/production-architecture.mdx:124-135` |

## Answers to Dimension Questions

**1. Is configuration layered?**
Yes, but informally and re-implemented per subsystem. The recurring chain is explicit argument > environment variable > persisted file > hardcoded constant: platform base URL (`lib/crewai-core/src/crewai_core/plus_api.py:174-178`), tracing enablement (`lib/crewai/src/crewai/events/listeners/tracing/utils.py:112-116` documents the priority order), OpenAI credentials/endpoints (`lib/crewai/src/crewai/llms/providers/openai/completion.py:264-279`), and `Settings.__init__` merging constructor kwargs over file JSON (`lib/crewai-core/src/crewai_core/settings.py:219`). Nested YAML `config:` dicts merge into Agent/Task models without clobbering explicit fields (`lib/crewai/src/crewai/utilities/config.py:22-32`). However, there is no central registry: each of the ~50 `CREWAI_*` variables found via repo-wide search is read independently with its own truthiness convention (`lower() == "true"` vs `in ("true","1")` vs `not in {"", "0", "false", "no", "off"}`), e.g. comparing `lib/crewai-core/src/crewai_core/telemetry.py:232-237` with `lib/cli/src/crewai_cli/utils.py:70-75` and `lib/crewai-tools/src/crewai_tools/security/safe_path.py:75-76`.

**2. Are environments managed cleanly?**
There is **no named-environment concept** (no dev/staging/prod switches exist anywhere; searches for `staging`/`production` in CLI sources only hit comments and a temp staging directory for ZIP archiving at `lib/cli/src/crewai_cli/deploy/archive.py:128-143`). What is managed cleanly is *process-context detection* and the *test environment*: an ordered marker table classifies runs as ci/serverless/paas/hosted_ide/notebook/container (`lib/crewai-core/src/crewai_core/runtime_env.py:152-159`); a headless enterprise mode suppresses all prompts (`CREWAI_DMN`, `lib/cli/src/crewai_cli/utils.py:70-75`); and the test suite has a deliberate parity harness — `.env.test` replicates CI secrets with fake values (`/.env.test:4-5`), fixtures force isolated storage and disable telemetry/network (`conftest.py:220-252`, `pyproject.toml:151`). Prod configuration of deployed crews happens server-side through the dashboard env vars uploaded at deploy time (`lib/cli/src/crewai_cli/deploy/validate.py:855-857` notes "the platform sets vars server-side").

**3. Are deployment modes documented?**
Minimally. Three modes are implemented — embedded library (`crew.kickoff()`), local CLI run via uv project scripts (`lib/cli/src/crewai_cli/run_crew.py:750-771`), and hosted AMP deployment (`lib/cli/src/crewai_cli/deploy/main.py:170-671`) — but the shipped docs cover only the hosted path, deferring to an external platform guide (`docs/edge/en/concepts/production-architecture.mdx:124-135`). The production doc recommends Flow-first architecture, `@persist` checkpointing, and async kickoff (`docs/edge/en/concepts/production-architecture.mdx:137-155`), i.e., deployment guidance is architectural rather than operational. No evidence of documentation for self-hosting the execution runtime was found inside this repository.

**4. Are feature flags supported?**
No formal feature-flag mechanism exists (repo-wide case-insensitive search for `feature.?flag` in code returns nothing; the only hit is the literal section header "Feature Flags/Testing Modes" in `/.env.test:141`). Instead, boolean env toggles act as de-facto flags with varying ergonomics: telemetry kill-switches (`lib/crewai-core/src/crewai_core/telemetry.py:232-237`), tracing opt-in/opt-out plus first-run consent prompt (`lib/crewai/src/crewai/events/listeners/tracing/utils.py:108-183`), headless mode (`CREWAI_DMN`), safe-path escape hatch with a tenant-proof override (`lib/crewai-tools/src/crewai_tools/security/safe_path.py:79-89`), DML allowance for NL2SQL (`CREWAI_NL2SQL_ALLOW_DML`), and flow script execution (`CREWAI_ALLOW_FLOW_SCRIPT_EXECUTION`). One flag lifecycle is visible in tests: skills commands were gated behind `CREWAI_EXPERIMENTAL` and have now graduated, with regression tests asserting the gate is gone (`lib/cli/tests/skills/test_cli_commands.py:3-5`, `lib/cli/tests/skills/test_cli_commands.py:21-40`).

**5. Is configuration validated?**
Substantially yes, in two tiers. Tier 1 — construction-time pydantic validation: `LLM` model validators normalize/derive fields (`lib/crewai/src/crewai/llm.py:717-733`); provider configs raise `ValueError` on impossible combinations such as `custom_openai=True` without any resolvable endpoint (`lib/crewai/src/crewai/llms/providers/openai/completion.py:263-274`) and defer client construction until deployment env vars exist (`lib/crewai/src/crewai/llms/providers/openai/completion.py:287-298`); security fingerprints reject empty/type-mismatched seeds (`lib/crewai/src/crewai/security/security_config.py:43-58`). Tier 2 — pre-deploy validation: `DeployValidator` runs nine check categories mirroring observed production failures (module docstring lists them, `lib/cli/src/crewai_cli/deploy/validate.py:1-25`), emits coded ERROR/WARNING findings (`lib/cli/src/crewai_cli/deploy/validate.py:60-84`), blocks deploy on errors unless `--skip-validate` (`lib/cli/src/crewai_cli/deploy/main.py:52-61`), and even cross-checks API keys referenced in code against `.env`/environment (`lib/cli/src/crewai_cli/deploy/validate.py:854-912`) and flags stale `crewai` pins older than 1.13.0 (`lib/cli/src/crewai_cli/deploy/validate.py:914-958`). Gaps: `Settings.dump()` swallows all write exceptions silently (`lib/crewai-core/src/crewai_core/settings.py:249-250`), so persistence failures are unobservable; and env-flag parsing is not validated for typos (unknown `CREWAI_*` vars are simply ignored).

## Architectural Decisions

1. **Shared core package for cross-cutting config.** Settings, constants, telemetry, user-data, and runtime detection live in `crewai-core` and are re-exported by both the framework (`lib/crewai/src/crewai/settings.py:1-18`) and the CLI (`lib/cli/src/crewai_cli/config.py:1-18`), explicitly so a CLI-only process never imports the heavy `crewai` package (`lib/crewai-core/src/crewai_core/runtime_env.py:3-5`).
2. **Env vars as the universal runtime knob.** Every behavioral toggle (telemetry, tracing, headless mode, security bypasses, storage paths) resolves through `os.environ` rather than a settings object, enabling container/platform injection without code changes — the property that makes the same code deployable to AMP with dashboard-supplied env (`lib/cli/src/crewai_cli/deploy/main.py:520-543`).
3. **Secrets kept out of the settings file where possible, hardened where not.** Auth tokens go through a separate `TokenManager` (`lib/crewai-core/src/crewai_core/settings.py:262-264`); tool-repo passwords that must live in the file are protected by atomic 0o600 writes and hidden from `crewai settings list` (`lib/crewai-core/src/crewai_core/settings.py:50-73`, `lib/crewai-core/src/crewai_core/settings.py:140-144`).
4. **Fail-open library, fail-closed deployer.** Runtime config problems degrade quietly (corrupt settings file → defaults, `lib/crewai-core/src/crewai_core/settings.py:212-217`; telemetry export failure → debug log, `lib/crewai-core/src/crewai_core/telemetry.py:66-74`) while deployment inputs fail loudly via blocking validation (`lib/cli/src/crewai_cli/deploy/main.py:52-61`).
5. **Detection tables over scattered checks.** Assistant/runtime identification uses single shared ordered tables walked by both telemetry and event emission, with an explicit comment that hard-coding order in two places caused drift (`lib/crewai/src/crewai/utilities/env.py:33-40`, `lib/crewai-core/src/crewai_core/runtime_env.py:49-94`).
6. **Closed-vocabulary telemetry attributes.** `detect_coding_agent`/`detect_runtime_context` can only return literals from the marker tables, structurally preventing PII from entering telemetry (`lib/crewai-core/src/crewai_core/runtime_env.py:176-189`).

## Notable Patterns

- **Priority-documented resolver functions.** `should_enable_tracing` states its own precedence contract in the docstring and implements it in eight lines (`lib/crewai/src/crewai/events/listeners/tracing/utils.py:108-137`); the CLI status command mirrors the same gate so displayed state matches runtime behavior (`lib/crewai-core/src/crewai_core/user_data.py:77-91`).
- **Escape-hatch with override hierarchy.** `ALLOW_UNSAFE_PATHS` is honored only if `FORCE_SAFE_PATHS` is unset, and setting both logs a warning naming the ignored variable — designed so managed tenants cannot self-bypass (`lib/crewai-tools/src/crewai_tools/security/safe_path.py:79-89`).
- **Import-time pollution defense.** litellm is lazy-loaded because its module-level `dotenv.load_dotenv()` would overwrite unrelated env vars like `MODEL`; the comment records the incident (`lib/crewai/src/crewai/llm.py:74-78`).
- **Non-interactive guards layered on consent prompts.** Trace-viewing prompts bail out early in test environments and non-TTY contexts ("CI, API servers, Docker") to avoid 20-second blocks (`lib/crewai/src/crewai/events/listeners/tracing/utils.py:467-501`).
- **Sentinel-path persistence guard.** When no writable config location exists, `Settings` uses `config_path=/dev/null` and `dump()` becomes a no-op, keeping the object usable read-only (`lib/crewai-core/src/crewai_core/settings.py:196-201`, `lib/crewai-core/src/crewai_core/settings.py:236-237`).
- **Env-var contract for subprocess handoff.** Trained-agent files, JSON crew definitions, and inputs travel from CLI parent to project-venv child processes exclusively through named env vars (`lib/cli/src/crewai_cli/run_crew.py:37-40`, `lib/cli/src/crewai_cli/run_crew.py:468-477`).
- **Workspace-level supply-chain pinning as config.** `[tool.uv] exclude-newer` plus per-package cutoffs and `override-dependencies` encode security-driven dependency policy in the root manifest (`pyproject.toml:172-263`).

## Tradeoffs

- **Flexibility vs discoverability.** Any behavior can be changed via env var without touching code, but there is no enumerated catalog; users discover knobs from docs pages, error messages that advertise escape hatches (`lib/crewai-tools/src/crewai_tools/security/safe_path.py:26`), or source reading.
- **Decentralized resolution vs duplication.** Each subsystem implementing param > env > file independently keeps coupling low but produces divergent boolean-parsing semantics and three separate `.env` parsers with differing robustness (none handle quotes or multi-line values): `lib/cli/src/crewai_cli/utils.py:120-139` vs `lib/cli/src/crewai_cli/utils.py:179-189` vs `lib/cli/src/crewai_cli/deploy/validate.py:885-892`.
- **Import-time dotenv convenience vs determinism.** Library-side `load_dotenv()` at import (`lib/crewai/src/crewai/project/crew_base.py:130`) makes quickstart scripts work but couples config to import order — precisely the bug class the litellm comment describes (`lib/crewai/src/crewai/llm.py:74-78`).
- **Best-effort config fallbacks vs surprising side effects.** Falling back to writing `settings.json` into `/tmp` or the CWD guarantees function on locked-down machines (`lib/crewai-core/src/crewai_core/settings.py:76-107`) but can leak credentials into shared locations; mitigated by 0o600 modes and refusing to chmod shared dirs (`lib/crewai-core/src/crewai_core/settings.py:29-47`) yet still visible in `Path.cwd()`.
- **Silent degradation vs observability.** Swallowing exceptions in `Settings.dump()` and telemetry keeps agents running (a reasonable choice for a library embedded in other apps) but hides misconfiguration from operators.
- **Hosted-platform simplicity vs portability.** Steering all production deployments to CrewAI AMP simplifies ops but ties the deployment shape to a proprietary control plane; no in-repo alternative (Dockerfile, Helm chart, server entrypoint) exists.

## Failure Modes / Edge Cases

- **Silent settings write loss:** every `dump()` exception path is `pass` (`lib/crewai-core/src/crewai_core/settings.py:249-250`); a read-only filesystem yields no error and no persistence. Tests assert the happy path and file modes (`lib/cli/tests/test_config.py:99-126`) but not write-failure surfacing.
- **Corrupt settings file resets silently:** malformed JSON discards the entire file contents on next load (`lib/crewai-core/src/crewai_core/settings.py:212-217`); user org/token state vanishes without warning (covered by test `lib/cli/tests/test_config.py:135-146`, so it is known-and-accepted).
- **`.env` upload parser fragility at deploy time:** `fetch_and_json_env_file` splits on the first `=` and strips nothing else — quoted values keep their quotes, multi-line values corrupt subsequent lines (`lib/cli/src/crewai_cli/utils.py:126-130`), and these exact strings become the deployed environment (`lib/cli/src/crewai_cli/deploy/main.py:365-372`).
- **Env-flag typo invisibility:** `CREWAI_DISABLE_TELEMTRY=true` (sic) matches nothing and telemetry stays on; no unknown-var warnings exist anywhere.
- **Inconsistent truthiness conventions:** `CREWAI_DMN` treats `"no"/"off"` as false (`lib/cli/src/crewai_cli/utils.py:75`) while telemetry flags accept anything ≠ `"true"` (`lib/crewai-core/src/crewai_core/telemetry.py:232-237`) — the same mental model yields opposite results per flag.
- **Fallback config location collision:** two projects running in the same CWD share `./crewai_settings.json` fallback state when `$HOME` is unwritable (`lib/crewai-core/src/crewai_core/settings.py:88-89`).
- **Module-import side effects:** importing `crewai.project.crew_base` mutates `os.environ` from whichever `.env` is nearest at import time (`lib/crewai/src/crewai/project/crew_base.py:130`), which can mask or override shell-exported values depending on load order (CLI avoids this by loading with `override=True` itself, `lib/cli/src/crewai_cli/run_crew.py:315-317`).
- **Consent prompt in semi-attended contexts:** first-run tracing confirmation fires on any interactive TTY unless `CREWAI_TESTING` is set (`lib/crewai/src/crewai/events/listeners/tracing/utils.py:172-188`); scheduled jobs attached to a pseudo-TTY could block on the confirm (bounded by the click prompt, no timeout, unlike the 20s-bounded view prompt at `:487`).

## Future Considerations

- **Centralize runtime env config** into a typed, validated settings facade in `crewai-core` (the package already hosts the pattern for file settings), enumerating the ~50 `CREWAI_*` variables with one truthiness parser and optional unknown-var warnings; this converts today's convention-scattered surface into an auditable contract.
- **Replace the three hand-rolled `.env` parsers** with a single shared parser (python-dotenv is already a dependency) so quoted/multi-line values survive the deploy upload path (`lib/cli/src/crewai_cli/utils.py:120-139`).
- **Surface persistence failures** from `Settings.dump()` via logged warnings at minimum (`lib/crewai-core/src/crewai_core/settings.py:249-250`).
- **Introduce an explicit environment profile concept** (e.g., `CREWAI_ENV=dev|staging|prod` selecting defaults for telemetry verbosity and prompt suppression) to formalize what is currently achieved by combining `CREWAI_DMN` + `CREWAI_TESTING` + telemetry flags manually.
- **Document the deployment matrix**: one page describing embedded/local-run/AMP modes, their config surfaces, and the security escape-hatch overrides would close the gap between implemented capability and documented capability noted under Question 3.
- **Promote validated env-var presence checks** (the `DeployValidator._check_env_vars` regex scan, `lib/cli/src/crewai_cli/deploy/validate.py:862-883`) into a reusable lint so local `crewai run` benefits from the same preflight the deploy path gets.

## Questions / Gaps

- **No self-hosted/server deployment artifact found.** Searches for Dockerfiles, entrypoints, or server modules within the source returned only the ZIP-archive staging helper and the hosted-platform client; if a self-hosting story exists it lives outside this repository. Search boundary: full tree of `studies/agent-harness-study/sources/crewai`.
- **No formal feature-flag service integration.** If product flags are evaluated remotely (e.g., via the Plus API), no evidence appears in the client code; only local env toggles were found.
- **Staging-equivalent for the platform client untested in-repo.** `CREWAI_PLUS_URL` enables pointing at alternate backends (`lib/crewai-core/src/crewai_core/plus_api.py:176`), but no tests exercise a staged-platform workflow; `.env.test` sets `CREWAI_PLUS_URL=https://fake.crewai.com` purely as a placeholder (`/.env.test:130`).
- **`CREWAI_EXPERIMENTAL` lifecycle end-state unclear.** Tests document that skills graduated from the gate (`lib/cli/tests/skills/test_cli_commands.py:3-5`), but whether other experimental surfaces still consult the variable elsewhere could not be confirmed — no non-test reader was found in current sources.
- **OAuth2/enterprise settings validation depth unknown.** `oauth2_extra` accepts arbitrary dicts with no schema (`lib/crewai-core/src/crewai_core/settings.py:188-191`); whether the platform tolerates malformed extras is undetermined from this source alone.

---

Generated by `dimensions/22.02-configuration-deployment-shape.md` against `crewai`.

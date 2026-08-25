# Source Analysis: pydantic-ai

## Dimension 22.02: Configuration and Deployment Shape

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python 3.10+ (pydantic, anyio, httpx2; uv workspace with `pydantic-ai-slim`, `pydantic-graph`, `pydantic-evals`, `clai`) |
| Analyzed | 2026-08-25 |

All citations below are workspace-relative paths rooted at `studies/agent-harness-study/sources/pydantic-ai/`.

## Summary

Pydantic AI is an embedded library, not a deployed service, so its "configuration and deployment shape" is a library-configuration story rather than a server-infrastructure story. Configuration is layered at three distinct, independently documented layers: (1) provider credentials/endpoints resolve explicit constructor argument → environment variable → built-in default across ~30 per-provider classes (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/providers/groq.py:138-139`); (2) `ModelSettings` merge model defaults with per-run overrides via a shallow dict spread (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/settings.py:502-511`); (3) model profiles resolve through a documented four-step chain — `DEFAULT_PROFILE` → provider profile → user override → native-tool intersection (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/models/__init__.py:863-903`). Declarative configuration exists as validated YAML/JSON `AgentSpec` files loaded by `Agent.from_file` with generated JSON schemas for editor validation.

Deployment modes are multiple and documented: embedded dependency (slim package + optional extras), CLI chat (`pai`/`clai`), local web chat service (Starlette app with CDN/offline UI support), UI protocol adapters mounted into the host application, and durable-execution engines (Temporal, DBOS, Prefect in-tree; Restate external). There is no first-class dev/staging/prod environment abstraction and no formal feature-flag system; environment differentiation is delegated entirely to OS environment variables, with one notable operational safeguard: the global `ALLOW_MODEL_REQUESTS` kill switch that the project's own test suite enables by default to prevent accidental paid API calls. The same code runs identically in dev, staging, and prod with configuration changes only — trivially true because it is a library whose every environment-specific input (API keys, base URLs, regions) arrives via env vars or constructor arguments.

## Rating

**7 / 10**

The config model is clear and consistent: the argument→env→default precedence rule is applied uniformly across every provider class, profile/settings layering has documented resolution orders enforced by tests that derive truth from the actual wire payloads (`studies/agent-harness-study/sources/pydantic-ai/tests/models/test_model_settings_support.py:1-24`), spec files get real pydantic validation plus editor schemas, and misconfiguration fails early with actionable `UserError`s including "did you mean" suggestions (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/models/__init__.py:1548-1561`). It falls short of 9-10 on three counts: no environment abstraction beyond raw env vars, no feature-flag mechanism, and generic `ModelSettings` is deliberately not runtime-validated (a typo'd key is silently dropped by design), which trades correctness feedback for cross-provider portability.

## Evidence Collected

Every entry cites a workspace-relative path rooted at `studies/agent-harness-study/sources/pydantic-ai/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Config precedence (arg → env → default) | Groq provider reads explicit `api_key`, falls back to `GROQ_API_KEY` env, then default base URL; same pattern in ~30 providers | `pydantic_ai_slim/pydantic_ai/providers/groq.py:138-139` |
| Missing-key fail-fast + placeholder workaround | OpenAI provider raises `UserError` when no key/env/client; substitutes `'api-key-not-set'` for keyless local servers behind `OPENAI_BASE_URL` | `pydantic_ai_slim/pydantic_ai/providers/openai.py:96-108` |
| Keyless newcomer hint | `missing_api_key_error()` appends a pointer to the keyless test model to every credential error | `pydantic_ai_slim/pydantic_ai/providers/__init__.py:26-39` |
| Dual-name env fallback chain | Gateway resolves `PYDANTIC_AI_GATEWAY_API_KEY` then legacy `PAIG_API_KEY`; base URL env → region inference from key | `pydantic_ai_slim/pydantic_ai/providers/gateway.py:151-160` |
| Provider registry | `infer_provider_class` maps provider-name strings to classes via lazy imports; unknown names raise `ValueError` | `pydantic_ai_slim/pydantic_ai/providers/__init__.py:142-281` |
| Model-id parsing & dispatch | `parse_model_id` splits `provider:model`; `infer_model` routes to concrete model classes | `pydantic_ai_slim/pydantic_ai/models/__init__.py:1442-1457, 1529-1650` |
| Typo suggestions | `_suggest_known_model_name` uses `difflib.get_close_matches` to suggest near-miss model ids in errors | `pydantic_ai_slim/pydantic_ai/models/__init__.py:1460-1475` |
| Settings type & contract | `ModelSettings(TypedDict, total=False)`; docstring defines the machine-checked "Supported by:" lists | `pydantic_ai_slim/pydantic_ai/settings.py:90-114` |
| Settings layering | `merge_model_settings(base, overrides)` = shallow `base \| overrides` spread preferring overrides; called from `Model.prepare_request` | `pydantic_ai_slim/pydantic_ai/settings.py:502-511`; `pydantic_ai_slim/pydantic_ai/models/__init__.py:608` |
| Profile layering order | Documented 4-step resolution: `DEFAULT_PROFILE` → `provider.model_profile()` → user partial-dict/callable override → supported-native-tools intersection | `pydantic_ai_slim/pydantic_ai/models/__init__.py:863-903` |
| Profile merge primitive | `merge_profile` spreads layers left-to-right, translating deprecated keys per layer | `pydantic_ai_slim/pydantic_ai/profiles/__init__.py:249-262` |
| Base defaults layer | Fully populated `DEFAULT_PROFILE` dict used as base layer | `pydantic_ai_slim/pydantic_ai/profiles/__init__.py:217-235` |
| Declarative agent config | `AgentSpec(BaseModel)` fields incl. `model`, `model_settings`, `retries`, `capabilities`; `from_file`/`from_text`/`from_dict` validate via pydantic | `pydantic_ai_slim/pydantic_ai/agent/spec.py:33-111` |
| Spec schema generation | Emits JSON Schema sidecars (`extra='forbid'`) and YAML `$schema:` header line for editor validation | `pydantic_ai_slim/pydantic_ai/agent/spec.py:27-30, 113-163, 197` |
| Agent-from-file entry point | `Agent.from_file(path)` loads spec then forwards all overrides to `from_spec` | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:989-1047` |
| Cost kill switch | Global `ALLOW_MODEL_REQUESTS`; `check_allow_model_requests()` raises `RuntimeError` when disabled; context-manager override | `pydantic_ai_slim/pydantic_ai/models/__init__.py:1388-1439` |
| Test-env safety default | Test suite sets `ALLOW_MODEL_REQUESTS = False` globally; fixtures re-enable per test | `tests/conftest.py:91` |
| Kill-switch enforcement points | Called at top of every paid request path (request/stream/count_tokens) in each adapter | e.g. `pydantic_ai_slim/pydantic_ai/models/openai.py:1033, 2071` |
| CLI state location | `PYDANTIC_AI_HOME = ~/.pydantic-ai` stores prompt history; overridable `config_dir` parameter | `pydantic_ai_slim/pydantic_ai/_cli/__init__.py:50-56, 353-356` |
| XDG-aware cache dir | Web UI HTML cache honors `XDG_CACHE_HOME` / `LOCALAPPDATA`, atomic writes with lock | `pydantic_ai_slim/pydantic_ai/ui/_web/app.py:47-62, 79-101` |
| Offline/air-gapped deployment support | Default CDN URL vs `OFFLINE_HTML_URL` single-file build "for air-gapped deployments"; `html_source` accepts Path/URL/string | `pydantic_ai_slim/pydantic_ai/ui/_web/app.py:34-39, 104-131` |
| Web service hardening | `allowed_hosts` normalized up-front; DNS-rebinding protection returns 421 for unlisted Host headers | `pydantic_ai_slim/pydantic_ai/ui/_web/app.py:203-208, 216-236` |
| Local web server args | `clai web` binds `--host 127.0.0.1 --port 7932` by default; `--allowed-host` for reverse proxies/tunnels | `pydantic_ai_slim/pydantic_ai/_cli/__init__.py:185-204, 195-196` |
| Deployment shape via extras | Optional dependency groups gate every integration (providers, cli, web, ui, ag-ui, temporal, dbos, prefect, mcp, spec…) | `pydantic_ai_slim/pyproject.toml:72-161` |
| Entry points | `pai` script → `pydantic_ai._cli:cli_exit`; standalone `clai` package delegates to the same CLI | `pydantic_ai_slim/pyproject.toml:166-168`; `clai/clai/__init__.py:9-11` |
| Install/deploy documentation | Slim install + full extras list documented; TLS trust-store guidance for minimal containers/proxies | `docs/install.md:14-19, 39-80` |
| Durable-execution deployment mode | `BaseDurabilityCapability` shared base for Temporal/DBOS/Prefect; engines listed incl. external Restate/Kitaru/Airflow | `pydantic_ai_slim/pydantic_ai/durable_exec/_base.py:40-66`; `docs/durable_execution/overview.md:5-17` |
| Cross-boundary config integrity | Unregistered `Model` instances are rejected workflow-side rather than rebuilt from `model_id` ("would quietly go to another endpoint with other credentials") | `pydantic_ai_slim/pydantic_ai/durable_exec/_base.py:421-456` |
| Durable model registry | `models=` mapping registered at bind time; `'default'` reserved; string models deferred to worker-side resolution | `pydantic_ai_slim/pydantic_ai/durable_exec/_base.py:348-380, 382-397` |
| MCP config env expansion | `${VAR}` / `${VAR:-default}` expansion recurses through JSON MCP server configs; undefined var raises `ValueError` | `pydantic_ai_slim/pydantic_ai/mcp.py:1877-1918` |
| Settings-support wire test | Module docstring explains deriving each settings field's "Supported by" list from outgoing payload diffs, both directions | `tests/models/test_model_settings_support.py:1-24` |
| Provider env-var tests | Parametrized provider→env-var table asserts `infer_provider` behavior and missing-key errors | `tests/providers/test_provider_names.py:43-60` |
| HTTP defaults | Shared default timeout (600s, matching OpenAI client) and user-agent-bearing client factory | `pydantic_ai_slim/pydantic_ai/_http.py:24-29, 54-65` |

## Answers to Dimension Questions

1. **Is configuration layered?**
Yes, explicitly, at three independent layers. Credentials/base URLs: constructor argument → environment variable → hardcoded default, applied uniformly (e.g. `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/providers/groq.py:138-139`, `.../providers/gateway.py:151-160`). Model settings: model-level defaults merged with per-run overrides (`merge_model_settings`, `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/settings.py:502-511`, invoked at `.../models/__init__.py:608`). Profiles: a documented four-step chain — global defaults, provider defaults, user partial-dict or callable override, capability intersection (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/models/__init__.py:863-903`; `merge_profile` at `.../profiles/__init__.py:249-262`). A fourth declarative layer exists for agents: YAML/JSON `AgentSpec` files validated by pydantic (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/agent/spec.py:33-111`). Merging is deliberately shallow (non-recursive), noted in-code at `settings.py:507`.

2. **Are environments managed cleanly?**
There is no dev/staging/prod concept anywhere in the codebase. Environment differentiation is delegated wholesale to standard OS environment variables per provider (~30 distinct names, e.g. `GOOGLE_API_KEY`/`GEMINI_API_KEY` at `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/providers/google.py:145`; AWS region/token chain at `.../providers/google_cloud.py:83-94`), which is clean in the Unix sense but means the framework offers no abstraction, validation, or namespacing of environments. Two mitigations exist: the `ALLOW_MODEL_REQUESTS` kill switch prevents accidental paid calls outside prod-like contexts (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/models/__init__.py:1388-1423`; enabled-by-default in tests at `tests/conftest.py:91`), and `.env` loading appears only in examples/dev tooling, never in the core library (e.g. `examples/pydantic_ai_examples/realtime_webrtc/app.py:46,59`). No evidence found of any dotenv loader inside `pydantic_ai_slim`.

3. **Are deployment modes documented?**
Yes. Five modes are implemented and documented: embedded library (slim install with opt-in extras, `studies/agent-harness-study/sources/pydantic-ai/docs/install.md:39-80` and `pydantic_ai_slim/pyproject.toml:72-161`); CLI chat (`pai` entry point at `pydantic_ai_slim/pyproject.toml:166-168`); locally-served web chat with air-gapped offline UI variant and DNS-rebinding host allowlisting (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/ui/_web/app.py:34-39, 203-236`); UI protocol adapters (AG-UI/Vercel AI) designed to mount into the *user's* Starlette app, with an explicit version-policy comment distinguishing who owns the framework (`pydantic_ai_slim/pyproject.toml:145-150`); and durable execution on Temporal/DBOS/Prefect with external Restate/Kitaru/Airflow (`studies/agent-harness-study/sources/pydantic-ai/docs/durable_execution/overview.md:5-17`).

4. **Are feature flags supported?**
No formal feature-flag mechanism exists (searches for `feature flag`, `FeatureFlag`, `toggle`, `is_flag` across the source return nothing). The nearest analogues are: install-time feature gating via optional extras (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pyproject.toml:72-161`); the process-global `ALLOW_MODEL_REQUESTS` toggle with scoped context-manager override (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/models/__init__.py:1388-1439`); class-level instrumentation toggle `Agent.instrument_all()` (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/agent/__init__.py:1049-1052`); and per-model capability flags carried in profiles rather than scattered isinstance checks (mandated by `pydantic_ai_slim/pydantic_ai/AGENTS.md`, rule 587). Forward-compatibility gating exists only inside the AG-UI adapter per `pydantic_ai_slim/pydantic_ai/ui/AGENTS.md`. These are static or process-wide switches, not runtime-targetable flags.

5. **Is configuration validated?**
Strongly at the boundaries, deliberately loosely inside the hot path. Validated: `AgentSpec` is a pydantic `BaseModel` (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/agent/spec.py:33-49, 101-111`) with a strict `extra='forbid'` schema published for editors (`spec.py:197`); missing credentials raise actionable `UserError`s with a keyless-model hint (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/providers/__init__.py:26-39`, `.../providers/openai.py:96-108`); malformed model ids produce close-match suggestions (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/models/__init__.py:1548-1561`); undefined MCP env references raise at load time (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/mcp.py:1908`). Not validated: generic `ModelSettings` is a `total=False` TypedDict, so unsupported or misspelled generic keys are silently ignored at runtime — a stated design choice ("silently ignore unsupported generic tuning settings… keeps client code portable", `pydantic_ai_slim/pydantic_ai/models/AGENTS.md`, rules 912/26). The drift risk this creates is backstopped by `studies/agent-harness-study/sources/pydantic-ai/tests/models/test_model_settings_support.py:1-24`, which derives the docstring "Supported by:" lists from real outgoing payloads in both directions.

**Dimension question — can the same binary run in dev, staging, and prod with config changes only?** Yes, structurally: pydantic-ai ships no server and holds no environment-specific state; every differing input (keys, base URLs, regions, model selection, timeouts, instrumentation) enters through constructor arguments, env vars, or spec files. The durable-execution mode adds one caveat: model instances cannot be serialized across the workflow boundary and must be re-resolvable worker-side from strings/registries (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/durable_exec/_base.py:44-56, 450-456`), so worker environments must carry equivalent provider configuration — still config-only, but it must be duplicated to workers.

## Architectural Decisions

- **Providers own configuration.** Each provider class encapsulates authentication, base URL, HTTP client lifecycle, and profile inference (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/providers/__init__.py:42-100`), keeping provider-specific config out of graph/tool code (per `pydantic_ai_slim/pydantic_ai/providers/AGENTS.md`).
- **Convention-over-configuration model selection.** A bare string `'openai:gpt-5'` flows through `parse_model_id` → registry lookup → lazy provider construction → model dispatch (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/models/__init__.py:1529-1650`), so most users configure nothing beyond an env var.
- **Typed, provider-prefixed settings instead of dicts.** `ModelSettings` subclasses use `{provider}_*` field prefixes so autocomplete/type-checking reveals which provider owns each knob (rule 71 in `pydantic_ai_slim/pydantic_ai/AGENTS.md`; unified-vs-provider-specific precedence documented at `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/settings.py:85-87`).
- **Declarative specs as a separate, optional surface.** YAML/JSON agent definitions round-trip through pydantic validation and emit JSON Schemas for editor-time checking, but remain strictly additive to programmatic construction (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/agent/spec.py:174-228`).
- **Fail-safe defaults for spend.** The cost kill switch defaults open (`True`) in the library but closed in the test suite, and every adapter calls the check before any billable request (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/models/__init__.py:1402-1423`).
- **Configuration integrity across durability boundaries.** Rather than serializing configured objects, only identity strings cross; unregistered instances are rejected loudly to prevent silently hitting different endpoints with different credentials (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/durable_exec/_base.py:436-456`).

## Notable Patterns

- **Uniform three-tier fallback idiom**: `arg or os.getenv('X') or default` repeated verbatim across all provider constructors makes precedence predictable and greppable (e.g. `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/providers/sambanova.py:102,110`).
- **Layered profile merging with deprecation translation per layer**: `_translate_legacy_profile_keys` runs on each input to `merge_profile`, so a legacy key in any layer still overrides correctly (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/profiles/__init__.py:200-214, 249-262`).
- **Documentation-as-test-contract**: the `Supported by:` bullets in `ModelSettings` docstrings are parsed and verified against wire-captured payloads by `studies/agent-harness-study/sources/pydantic-ai/tests/models/test_model_settings_support.py:1-24` — config documentation cannot silently drift.
- **Editor-integrated config validation**: emitted `$schema:` headers and JSON-Schema sidecars give IDEs the same validation the runtime applies (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/agent/spec.py:27-30, 230-246`; same pattern for eval datasets at `pydantic_evals/pydantic_evals/dataset.py:77-81, 784-789`).
- **Environment-variable templating inside declarative configs**: MCP server JSON supports `${VAR}`/`${VAR:-default}` expansion compatible with Claude Code conventions, failing fast on unset variables (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/mcp.py:1877-1918`).
- **Security-conscious local serving**: host-header allowlist middleware applied app-wide, patterns normalized at construction so bad input errors immediately rather than on first request (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/ui/_web/app.py:216-236`).

## Tradeoffs

- **Portability over precision in settings.** Silently ignoring unsupported generic settings keeps client code model-portable but hides typos and unsupported combos; the project consciously rejects client-side guards for provider-namespaced settings (rules 26/912/562 in `pydantic_ai_slim/pydantic_ai/models/AGENTS.md`), relying on docs contracts and tests instead.
- **Env-var sprawl vs zero-dependency simplicity.** ~30 provider-specific variable names (`OPENAI_API_KEY`, `CO_API_KEY`, `HF_TOKEN`, dual `PYDANTIC_AI_GATEWAY_API_KEY`/`PAIG_API_KEY` fallbacks…) deliver 12-factor configurability with no config file, parser, or schema in the core — but there is no single place to audit what the process was configured with.
- **Shallow merges keep semantics obvious.** `base | overrides` and dict-spread profile merges are easy to predict, at the price of whole-object replacement when nested values collide (noted in-code at `studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/settings.py:497-499, 507`).
- **Global toggles are simple but coarse.** `ALLOW_MODEL_REQUESTS` is process-wide, not per-agent/per-run; scoped control requires the context manager (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/models/__init__.py:1426-1439`).
- **Extras-as-deployment-shape shifts weight to install time.** Features like web serving, durable execution, and realtime are gated by pip extras (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pyproject.toml:72-161`), keeping the slim core light but making deployment manifests responsible for selecting groups.

## Failure Modes / Edge Cases

- **Deferred auth failures on keyless-compatible endpoints.** Providers serving local/OpenAI-compatible servers substitute placeholder keys (`'api-key-not-set'`) instead of erroring (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/providers/openai.py:106-108`, `.../providers/ollama.py:104-113`), pushing misconfiguration discovery to first request time.
- **Silent wrong-endpoint hazard in durable mode.** Rebuilding a `Model` from its id on a worker would target "another endpoint with other credentials"; the design converts this into a loud `UserError` requiring registration (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/durable_exec/_base.py:450-456`).
- **Implicit gateway routing.** When no route is given, `gateway_provider` derives it from the upstream provider name and infers the base URL's region from the API key itself (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/providers/gateway.py:158-167`) — convenient but invisible unless inspected.
- **Azure's long fallback chains.** Voice-live vs chat paths consult overlapping env vars (`AZURE_OPENAI_*`, `AZURE_VOICELIVE_*`, `OPENAI_API_VERSION`) across many branches (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/providers/azure.py:193-295`); partial configuration can resolve to surprising combinations.
- **MCP expansion failure at load.** A referenced-but-unset variable without `:-` default aborts config loading with `ValueError` (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/mcp.py:1907-1908`) — fail-fast, but only for MCP configs.
- **Host allowlist misconfiguration behind proxies.** Serving through a tunnel without `--allowed-host` yields 421s from the DNS-rebinding guard (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/ui/_web/app.py:203-208`); the fix is documented but runtime errors don't self-explain.
- **Legacy-client transition window.** Passing a legacy `httpx.AsyncClient` where `httpx2` is expected warns now and breaks at v3 (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/_http.py:80-102`), a config-compat edge tracked by explicit deprecation warnings.

## Future Considerations

- An optional unified configuration surface (e.g. a validated project-level config combining providers, models, and agents) would reduce env-var sprawl; today only per-agent specs and eval dataset files have declarative shapes.
- The planned v3 removal of legacy `httpx` support will collapse the dual-client branching in `_http.py` and several provider constructors, simplifying HTTP configuration (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/_http.py:80-102`).
- Durable-mode configuration could benefit from tooling that verifies a worker environment can rebuild every registered model id before workflows ship.
- If feature-flag needs grow, the existing capability/profile machinery (`AbstractCapability`, `ModelProfile` flags) is the natural extension point rather than introducing a parallel flag system.

## Questions / Gaps

- No evidence found of any dev/staging/prod environment abstraction, config-file loader (dotenv/TOML/etc.), or config hot-reload inside the core packages; searched `dotenv`, `BaseSettings`, `getenv` usage, and `docs/`. `.env` handling exists only in examples and dev scripts (e.g. `examples/pydantic_ai_examples/realtime_webrtc/app.py:46,59`).
- No evidence found of runtime redaction or audit logging of resolved configuration values (e.g. masking keys in logs/traces); secret hygiene is visible only in the VCR cassette serializer within tests (`tests/json_body_serializer.py`), not at the config layer.
- No evidence found of per-agent scoping for `ALLOW_MODEL_REQUESTS`; enforcement is module-global only.
- The `clai` CLI persists only prompt history under `~/.pydantic-ai` (`studies/agent-harness-study/sources/pydantic-ai/pydantic_ai_slim/pydantic_ai/_cli/__init__.py:50-56`); no persistent CLI settings file (theme, default model persistence) was found — defaults are hardcoded (`default_model='openai:gpt-5'`, `:140`).

---

Generated by dimension 22.02 (Configuration and Deployment Shape) against `pydantic-ai`.

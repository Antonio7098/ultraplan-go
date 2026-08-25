# Source Analysis: openai-agents-sdk

## Dimension 22.02: Configuration and Deployment Shape

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+ library (`openai-agents` on PyPI), asyncio, pydantic v2, httpx2/openai SDK v3 |
| Analyzed | 2026-08-25 |

## Summary

The OpenAI Agents Python SDK is an **in-process library**, not a deployable service, so its "deployment shape" is expressed as pluggable execution backends rather than build modes. Configuration follows one consistent pattern repeated at every boundary: typed settings objects or plain dicts normalized by a shared coercion layer, layered as **explicit argument → process-wide default setter → environment variable**, with fail-fast validation (unknown fields raise `TypeError`, invalid enum-ish values raise `ValueError`/`UserError`). There is no config file loader and no formal feature-flag system; behavior toggles are environment variables (`OPENAI_AGENTS_*`, `OPENAI_*`) plus opt-in constructor fields for beta features. Deployment flexibility is delivered through interchangeable `ModelProvider`s (OpenAI, LiteLLM, any-llm via model-name prefixes) and interchangeable sandbox/session backends (unix-local/Docker/seven hosted sandbox providers; SQLite/Redis/Mongo/SQLAlchemy/Dapr sessions), enabling the same agent code to move from laptop to container to hosted production by changing run-config objects only — which the project documents explicitly as a decision guide.

## Rating

**8 / 10**

Rationale: The configuration model is clear, consistently enforced, and heavily tested. Every public configuration boundary funnels through `coerce_dataclass_config`/`coerce_pydantic_config` (`src/agents/_config_coercion.py:45-97`) which rejects misspelled fields with actionable errors; layering precedence (explicit > global default > env) is both implemented (`src/agents/models/openai_agent_registration.py:114-121`) and regression-tested (`tests/models/test_agent_registration.py:23-62`). Deployment-mode parity (same agent, swap sandbox client) is an explicit design goal documented with a decision table (`docs/sandbox/clients.md:9-19`). It falls short of 9-10 because: (1) debug/log flags are captured once at module import (`src/agents/_debug.py:20-25`), so runtime env changes are ignored; (2) process-wide configuration lives in mutable module globals with no reset API (`src/agents/models/_openai_shared.py:9-15`), relying on test monkeypatching; (3) the env-var surface is scattered across ~15+ variables with no single registry or consolidated reference beyond prose docs; and (4) legacy compatibility shims create dual sources of truth for transport defaults (`src/agents/models/_openai_shared.py:14-15,60-68`).

## Evidence Collected

Every entry includes a file path with line numbers, relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Global config facade | `set_default_openai_key/client/api/responses_transport/harness` functions compose shared-state setters and tracing key propagation | src/agents/_config.py:13-55 |
| Public exports of config setters | Re-exported from package root with docstrings noting `OPENAI_API_KEY` fallback | src/agents/__init__.py:278-338 |
| Process-wide default state | Module-level globals `_default_openai_key`, `_default_openai_client`, `_use_responses_by_default`, transport flag + getter/setter pairs | src/agents/models/_openai_shared.py:9-68 |
| Lazy client construction | Client created only on first model use; explicit options beat shared default, else `AsyncOpenAI(...)` reads `OPENAI_BASE_URL`/`OPENAI_WEBSOCKET_BASE_URL` env | src/agents/models/openai_provider.py:136-178 |
| Conflicting-config guard | Passing `openai_client` together with api_key/base_url/org/project raises `UserError` instead of silently ignoring duplicates | src/agents/models/openai_provider.py:90-98 |
| Shared HTTP connection pool | Process-level singleton `httpx2.AsyncClient` reused across providers to share connection pools | src/agents/models/openai_provider.py:31-42 |
| Prefix-based provider routing | `MultiProvider` maps `openai/`, `litellm/`, `any-llm/` prefixes; resolution order: explicit map > builtin fallbacks > prefix modes > `UserError` | src/agents/models/multi_provider.py:62-74,190-225 |
| Routing mode validation | `openai_prefix_mode`/`unknown_prefix_mode` validated with `UserError` listing allowed values | src/agents/models/multi_provider.py:176-188 |
| Default model via env | `get_default_model()` returns `OPENAI_DEFAULT_MODEL` env value, defaulting to `gpt-5.6-luna`; per-model reasoning-effort presets keyed by regex | src/agents/models/default_models.py:10,42-69,99-103 |
| Harness-ID three-layer resolution | `_resolve_str(explicit, default, os.getenv("OPENAI_AGENT_HARNESS_ID"))` — explicit beats stored default beats env | src/agents/models/openai_agent_registration.py:39-52,114-121 |
| Per-run config object | `RunConfig` dataclass: model/provider overrides, guardrails, tracing, sessions, sandbox, tool-execution policies; dict forms accepted at boundaries | src/agents/run_config.py:350-572 |
| Run-config validation | `__post_init__` rejects bad `tool_name_collision_policy`, empty/async blocked-message formatters, coerces nested dicts | src/agents/run_config.py:531-572 |
| Dict normalization entry point | `_coerce_run_config(value)` normalizes dicts at public runner boundaries | src/agents/run_config.py:606-608 |
| Coercion core | `coerce_dataclass_config` raises `TypeError` on unknown fields; pydantic variant respects `extra="allow"` policy and aliases | src/agents/_config_coercion.py:45-97 |
| Env-driven sensitive-data default | `RunConfig.trace_include_sensitive_data` defaults from `OPENAI_AGENTS_TRACE_INCLUDE_SENSITIVE_DATA` (default true) | src/agents/run_config.py:53-56,404-410 |
| Debug/log redaction flags | `DONT_LOG_MODEL_DATA`/`DONT_LOG_TOOL_DATA` parsed from env at import time, secure-by-default (true) | src/agents/_debug.py:12-28 |
| Tracing kill switch | `OPENAI_AGENTS_DISABLE_TRACING` read once on first use, cached; manual `set_disabled` always wins over env | src/agents/tracing/provider.py:339-356 |
| Trace export env fallbacks | Exporter resolves api_key/org/project from `OPENAI_API_KEY`/`OPENAI_ORG_ID`/`OPENAI_PROJECT_ID`; warns and skips export when no key | src/agents/tracing/processors.py:106-116,130-134 |
| Per-run tracing config | `TracingConfig` TypedDict (`api_key`, `include_task_and_turn_spans`) resolved per trace creation | src/agents/tracing/config.py:6-18 |
| Tracing shutdown safeguard | Global trace provider flushed via `atexit` handler with timeout; initialized lazily so import has no network side effects | src/agents/tracing/setup.py:16-66 |
| Deployment extras (packaging) | Optional dependency groups: docker, e2b, modal, daytona, cloudflare, runloop, vercel, blaxel, redis, mongodb, sqlalchemy, dapr, voice, realtime, litellm, any-llm, s3, temporal | pyproject.toml:36-59 |
| Sandbox backend availability gating | Unix-local excluded on win32; Docker import wrapped in try/except so base imports work without the optional extra | src/agents/sandbox/sandboxes/__init__.py:13-41 |
| Sandbox deployment parity doc | "the `SandboxAgent` definition stays the same while the sandbox client ... change[s] in `SandboxRunConfig`"; decision guide local→docker→hosted | docs/sandbox/clients.md:3,9-19,38-53,96-116 |
| Session backend selection doc | Table mapping `SQLiteSession` (local dev) → Redis/SQLAlchemy/Mongo/Dapr (distributed/production) | docs/sessions/index.md:203-209 |
| Codex CLI sidecar mode | Experimental extension spawns `codex exec --experimental-json` subprocess with configurable stream limits and env allowlist override | src/agents/extensions/experimental/codex/exec.py:47-59,120-132,214-231 |
| Opt-in workaround flag | `OPENAI_AGENTS_ENABLE_LITELLM_SERIALIZER_PATCH` gates a LiteLLM private-API patch, off by default | src/agents/extensions/models/litellm_model.py:125-126 |
| Beta feature flags as fields | `nest_handoff_history` documented as "Opt-in beta", disabled by default; server-managed conversations auto-disable it with a warning | src/agents/run_config.py:374-381 |
| Config docs page | Startup-time defaults, dict-vs-object normalization contract, env var tables for keys/endpoints/logging | docs/config.md:15-32,36-93,191-248 |
| Tests: default key/client/API shape | Error when no key; set_default_openai_key/client propagate into models; Responses-vs-ChatCompletions switch | tests/test_config.py:25-92 |
| Tests: transport validation & conflict guard | Invalid transport rejected; client+conflicting-args regression test (issue #3808, bare-assert bug) | tests/test_config.py:113-136 |
| Tests: env endpoint fallbacks | `OPENAI_BASE_URL`/`OPENAI_WEBSOCKET_BASE_URL` captured into client kwargs; explicit arg beats env | tests/test_config.py:161-199 |
| Tests: debug flag parsing matrix | Both log flags tested across unset/0/1/true/false | tests/test_debug.py:7-53 |
| Tests: tracing env kill-switch semantics | Env read-on-first-use, cached after warmup, manual override wins both directions | tests/tracing/test_tracing_env_disable.py:13-96 |
| Tests: harness-ID precedence | explicit > default > env, plus env-only and disabled cases | tests/models/test_agent_registration.py:23-81 |
| Tests: run-config dict normalization & rejection | Unknown first-party dict fields raise; invalid cwd/tool policies rejected; env-sensitive-data matrix | tests/test_run_config.py:43-70,88-91,175-182,315-365 |

## Answers to Dimension Questions

### 1. Is configuration layered?

Yes, with a consistent three-tier precedence: **explicit call-site argument → process-wide default setter → environment variable**. The clearest proof is harness-ID resolution, where `_resolve_str(explicit=..., default=..., env_name="OPENAI_AGENT_HARNESS_ID")` iterates candidates in that order (`src/agents/models/openai_agent_registration.py:114-121`). The same pattern holds for model clients: provider constructor args take priority, otherwise the shared default client/key set via `set_default_openai_key`/`set_default_openai_client` (`src/agents/models/openai_provider.py:138-178`, `src/agents/_config.py:13-31`), otherwise env (`OPENAI_API_KEY`, `OPENAI_BASE_URL`, `OPENAI_WEBSOCKET_BASE_URL`). Within a run, `RunConfig` overlays agent-level settings: `model_settings.resolve()` merges non-None override values onto base settings with special deep-merge handling for `extra_args` and retry policy (`src/agents/model_settings.py:254-289`). A second axis of layering is *scope*: SDK-wide startup defaults (docs/config.md:3), per-run `RunConfig` (`src/agents/run_config.py:350`), and per-call `SessionSettings`/`ModelSettings` overrides (`src/agents/memory/session_settings.py:41-60`). What is *not* layered: there is no config-file tier (no TOML/YAML/dotenv loading exists anywhere under `src/agents/` — verified by grep for `yaml|.toml|dotenv|configparser`; the only YAML hit is a sandbox mount config path constant at `src/agents/sandbox/entries/mounts/patterns.py:354`), and no named environment profiles ("dev"/"prod") — environment differentiation is delegated entirely to env-var values and injected objects.

### 2. Are environments managed cleanly?

Cleanly for a library, though by composition rather than by an environment abstraction. There is no built-in notion of dev/staging/prod; instead the SDK provides the seams: (a) endpoints and credentials come from env or injected clients (`src/agents/models/openai_provider.py:162-171`); (b) execution location is chosen per-run via `SandboxRunConfig.client` — unix-local for laptops, Docker for container parity, seven hosted providers for production (`docs/sandbox/clients.md:21-34,96-116`), with the doc explicitly framing this as "production-style parity"; (c) conversation storage scales from embedded SQLite to Redis/SQLAlchemy/Mongo/Dapr (`docs/sessions/index.md:203-209`, backend modules under `src/agents/extensions/memory/`). Operational hygiene is good: secrets default to redaction in logs and traces (`src/agents/_debug.py:20-28`, `src/agents/run_config.py:53-56`), the trace exporter skips export with a warning when unconfigured rather than crashing (`src/agents/tracing/processors.py:133`), and the global trace provider flushes on interpreter exit (`src/agents/tracing/setup.py:16-24,34-36`). Two rough edges: the `DONT_LOG_*` flags are evaluated at import time (`src/agents/_debug.py:20-25`), so setting them after `import agents` has no effect — the docs acknowledge this by instructing users to export them "before your app starts" (docs/config.md:241-246); and the tracing disable flag is cached after first use by design, mid-run env flips being deliberately ignored (`src/agents/tracing/provider.py:340-352`, tested in `tests/tracing/test_tracing_env_disable.py:55-67`).

### 3. Are deployment modes documented?

Yes, unusually well for the sandbox/session axes. `docs/sandbox/clients.md` opens with the thesis — keep the `SandboxAgent` definition identical and change only the sandbox client in `SandboxRunConfig` (`docs/sandbox/clients.md:3`) — then gives a goal-based decision table (fastest local iteration → `UnixLocalSandboxClient`; container isolation → `DockerSandboxClient`; hosted/production isolation → hosted clients; `docs/sandbox/clients.md:13-19`), a concrete unix-local→Docker migration snippet changing only run-config lines (`docs/sandbox/clients.md:38-53`), and a supported-hosted-platforms table keyed to packaging extras (`docs/sandbox/clients.md:104-116`). Session-backend tradeoffs are tabulated similarly (`docs/sessions/index.md:203-209`). Model-provider routing including OpenAI-compatible proxies is documented with prefix-mode guidance (`docs/models/index.md:187-226`). The primary deployment mode — an in-process async library embedded in the host application — is implicit rather than stated as such; there is no page describing service embedding (e.g., FastAPI wiring appears only in tests/fastapi and examples). The experimental Codex integration's sidecar-subprocess shape is documented under tools (stream limit env var, valid range, default; `docs/tools.md:894`).

### 4. Are feature flags supported?

No formal feature-flag system (no flag registry, no remote flags, no percentage rollout). Instead there are three mechanisms: (1) **environment-variable toggles**, mostly operational: `OPENAI_AGENTS_DISABLE_TRACING` (`src/agents/tracing/provider.py:347-352`), `OPENAI_AGENTS_DONT_LOG_MODEL_DATA`/`_TOOL_DATA` (`src/agents/_debug.py:12-17`), `OPENAI_AGENTS_TRACE_INCLUDE_SENSITIVE_DATA` (`src/agents/run_config.py:53-56`), `OPENAI_AGENTS_ENABLE_LITELLM_SERIALIZER_PATCH` as an explicit opt-in workaround for a third-party bug (`src/agents/extensions/models/litellm_model.py:125-126`, docs/models/index.md:705-708), `OPENAI_AGENTS_CODEX_SUBPROCESS_STREAM_LIMIT_BYTES` with enforced min/max bounds (`src/agents/extensions/experimental/codex/exec.py:19-22`), and `OPENAI_DEFAULT_MODEL` (`src/agents/models/default_models.py:10,103`). (2) **Constructor-level beta opt-ins**: `nest_handoff_history` labeled "Opt-in beta ... disabled by default while we stabilize" (`src/agents/run_config.py:374-377`), and `MultiProvider` prefix/unknown modes gated behind explicit opt-in to preserve historical semantics (`src/agents/models/multi_provider.py:69-73`). (3) **Packaging extras as coarse capability flags** (`pyproject.toml:36-59`) with graceful degradation at import (`src/agents/sandbox/sandboxes/__init__.py:30-41`). All flags are static per-process; nothing supports runtime toggling short of calling `set_tracing_disabled()` (`src/agents/tracing/provider.py:332-337`) or mutating globals.

### 5. Is configuration validated?

Yes — this is a standout area. A single coercion module backs all public boundaries: `coerce_dataclass_config` accepts the typed instance or a dict, raising `TypeError` listing unknown fields (`src/agents/_config_coercion.py:45-66`), and `coerce_pydantic_config` does the same while honoring `extra="allow"` escape hatches and validation aliases (`src/agents/_config_coercion.py:69-97`). `RunConfig.__post_init__` validates enum-like strings, rejects empty or async blocked-message formatters with specific messages, and recursively coerces nested `sandbox`/`tool_execution`/`model_settings`/`session_settings` dicts (`src/agents/run_config.py:531-572`). Domain-specific validators enforce resource safety: sandbox concurrency/archive limits must be ≥1 or explicitly None (`src/agents/run_config.py:152-158,177-181,206-215`), memory consolidation caps at 4096 raw memories (`src/agents/sandbox/config.py:113-121`), and subprocess stream limits are clamped to [64KiB, 64MiB] (`src/agents/extensions/experimental/codex/exec.py:19-22`). Mutually exclusive options are caught eagerly: passing `openai_client` with any connection-setting argument raises `UserError` at construction (`src/agents/models/openai_provider.py:90-98`), with a regression test documenting that a prior bare-`assert` implementation silently passed under `python -O` (`tests/test_config.py:131-136`). Validation happens at configuration time, before side effects — matching the repo's own review rule preferring "an actionable error during construction or validation, before invocation" (AGENTS.md:98, Scope Discipline section). Test coverage for these paths is broad: unknown-field rejection (`tests/test_run_config.py:175-182`), invalid tool-name-collision policy (`tests/test_run_config.py:409-418`), dictionary-normalization equivalence (`tests/test_run_config.py:43-70`), and env parsing matrices (`tests/test_debug.py:7-53`, `tests/test_run_config.py:315-365`).

## Architectural Decisions

1. **Library-first, zero-service architecture.** The wheel ships only `src/agents` (`pyproject.toml:115-116`); there is no daemon, server, or CLI in the core. "Deployment" is therefore a composition problem solved via injected `ModelProvider`, `BaseSandboxClient`, and `Session` implementations (`src/agents/run_config.py:222-240,353-359`), keeping the same process image runnable anywhere.
2. **Typed-object-or-dict at every public boundary.** The uniform contract — instances pass through untouched, dicts are coerced, unknown fields fail loudly (`src/agents/_config_coercion.py:45-97`, docs/config.md:15-32) — trades a little duplication for typo-proof configuration across dozens of settings types.
3. **Process-wide defaults as module globals with explicit setters, not a config object.** `set_default_openai_key/client/api` mutate globals in `_openai_shared` (`src/agents/models/_openai_shared.py:18-50`). This keeps the zero-config path (`Agent` + `Runner.run`) frictionless, at the cost of hidden global state that tests must monkeypatch and multi-tenant processes cannot vary per-tenant except by passing explicit clients/providers everywhere.
4. **Lazy initialization for anything with side effects.** The OpenAI client is created on first model lookup so missing API keys don't break import (`src/agents/models/openai_provider.py:136-137`); the trace provider initializes lazily "so importing the SDK does not create network clients" (`src/agents/tracing/setup.py:42-44`). This makes import cheap and side-effect-free — important for library consumers.
5. **Prefix-routed multi-provider model resolution.** Model strings like `litellm/openai/gpt-4.1` route to alternate providers through `MultiProvider`, with fallback providers imported lazily only when referenced and cached thereafter (`src/agents/models/multi_provider.py:164-174,190-197`). Ambiguity in the `openai/` prefix is resolved by explicit opt-in modes rather than silent heuristics (`src/agents/models/multi_provider.py:199-225`).
6. **Secure defaults with opt-out redaction.** Model/tool payloads are excluded from logs and traces by default (`src/agents/_debug.py:20-28`, `src/agents/run_config.py:53-56`), flipping exposure requires an affirmative env action documented with a warning about exception chains retaining payload data (docs/config.md:230-248).
7. **Optional integrations behind extras with soft imports.** Sandbox Docker support wraps its import in try/except so the core never hard-depends on it (`src/agents/sandbox/sandboxes/__init__.py:30-41`); mypy overrides mark all sandbox vendor SDKs as missing-import-tolerant (`pyproject.toml:157-183`).

## Notable Patterns

- **Precedence chain helper.** A tiny generic function iterates `(explicit, default, env)` candidates skipping empties — reused as the canonical layering idiom (`src/agents/models/openai_agent_registration.py:114-121`).
- **`__post_init__` as validation-and-coercion hook.** Dataclass configs normalize themselves on construction (`src/agents/run_config.py:531-572`, `src/agents/sandbox/config.py:95-111`), so invalid states cannot exist past the constructor.
- **Declared-type reflection for subclass-friendly coercion.** `_declared_dataclass_type` reads actual field annotations so subclasses of `RunConfig` get their own nested types coerced correctly (`src/agents/_config_coercion.py:14-34`).
- **Overlay-with-special-cases merging.** `ModelSettings.resolve()` performs shallow non-None overlay but deep-merges `extra_args` dicts and retry backoff policy — a pragmatic hybrid (`src/agents/model_settings.py:266-289`).
- **Env-flag caching with documented staleness semantics.** The tracing disable flag is read once and cached, with manual override always winning; the rationale ("avoid surprises mid-run") is written into the docstring (`src/agents/tracing/provider.py:340-344`).
- **Backend option polymorphism keyed by backend ID.** `SandboxRunConfig.__post_init__` looks up the options class from `BaseSandboxClientOptions._options_class_for_type(backend_id)` and cross-checks a user-supplied `"type"` discriminator against the selected client, coercing dicts accordingly (`src/agents/run_config.py:294-327`).
- **Platform-conditional exports.** `UnixLocalSandbox*` symbols simply don't exist on Windows, with a TYPE_CHECKING branch preserving typing (`src/agents/sandbox/sandboxes/__init__.py:13-28`).

## Tradeoffs

- **Global-default convenience vs. testability/multi-tenancy.** Setters like `set_default_openai_api` mutate process state (`src/agents/_config.py:27-31`); concurrent tenants wanting different defaults must thread explicit clients/providers through every call site. Tests compensate by resetting env/globals via monkeypatch (`tests/test_config.py:153-158` directly pokes the legacy private flag).
- **Fail-loud dict coercion vs. forward compatibility.** Rejecting unknown fields catches typos early but means adding a field in a newer SDK version breaks older callers that pass newer dicts — mitigated only by pinning versions; the pydantic `extra="allow"` path exists precisely for provider passthrough exceptions (`src/agents/_config_coercion.py:84-96`).
- **Import-time flags vs. runtime flexibility.** Capturing `DONT_LOG_*` at import makes behavior deterministic within a process but forces restart-or-reimport workflows for debugging (docs/config.md:241-246 acknowledges "before your app starts").
- **Env-var sprawl vs. discoverability.** ~15+ variables spread across modules (`OPENAI_AGENT_HARNESS_ID`, `CODEX_PATH`, `CODEX_API_KEY`, `BL_API_KEY`, `BL_REGION`, `CLOUDFLARE_SANDBOX_API_KEY`, etc. — see grep hits in `src/agents/extensions/experimental/codex/exec.py:266`, `src/agents/extensions/experimental/codex/codex_tool.py:616-624`, `src/agents/extensions/sandbox/blaxel/sandbox.py:1062,1217`, `src/agents/extensions/sandbox/cloudflare/sandbox.py:456,1600`) is flexible but has no machine-readable catalog; discovery relies on prose docs.
- **Same-agent-anywhere sandbox story vs. capability skew.** Swapping unix-local for Docker keeps definitions identical, but capabilities differ (e.g., `SandboxPathGrant.host_path` remapping is Docker-only; `docs/sandbox/clients.md:36`), so "parity" requires knowing per-backend caveats.
- **Beta-by-constructor-field vs. flag infrastructure.** Shipping `nest_handoff_history` as a documented opt-in field (`src/agents/run_config.py:374-381`) avoids flag plumbing but offers no kill switch without a redeploy.

## Failure Modes / Edge Cases

- **Post-import env changes ignored for logging flags.** Because `DONT_LOG_MODEL_DATA`/`DONT_LOG_TOOL_DATA` bind at import (`src/agents/_debug.py:20-25`), a ops playbook that exports `OPENAI_AGENTS_DONT_LOG_TOOL_DATA=0` into a running process sees nothing; restart required.
- **Missing API key surfaces late and from the openai lib.** With no explicit key and no env var, the failure is an `openai.OpenAIError` raised on first `get_model` (`tests/test_config.py:25-28,55-59`), not an SDK-level configuration error message pointing at the fix.
- **Trace export silently skipped without a key.** The exporter logs a warning and continues per grouped item (`src/agents/tracing/processors.py:130-134`) — safe, but a misconfigured prod deployment loses all traces with only a warning.
- **Legacy dual transport flags can disagree transiently.** `_use_responses_websocket_by_default` is kept as a backward-compat shim synced on read, with a comment admitting internal code/tests still mutate it (`src/agents/models/_openai_shared.py:14-15,60-68`) — two sources of truth bridged by reconciliation logic.
- **Windows capability cliff.** On win32 the entire `UnixLocalSandbox*` family is absent (`src/agents/sandbox/sandboxes/__init__.py:13`), so config referencing it fails at import rather than with a friendly message.
- **Subprocess sidecar inherits the full environment by default.** `CodexExec._build_env` copies all of `os.environ` unless an explicit override dict is provided (`src/agents/extensions/experimental/codex/exec.py:214-220`) — convenient but risks leaking unrelated secrets to the child process; containment depends on the caller supplying `env=`.
- **Coverage exclusions hide backend risk.** Vendor-heavy sandbox modules are omitted from coverage (`pyproject.toml:185-201`), meaning configuration bugs in e2b/modal/daytona/etc. adapters rely on integration tests rather than unit suites.

## Future Considerations

- Centralize the environment-variable surface into one module (names, parsers, defaults) to enable generated docs and drift detection; today each toggle is hand-rolled near its consumer.
- Provide a `reset_defaults()` testing/utility API instead of encouraging mutation of private globals (`src/agents/models/_openai_shared.py:18-50`), and deprecate the `_use_responses_websocket_by_default` shim outright.
- Convert import-time debug flags to lazy reads (mirroring the tracing provider's documented cache-once pattern, `src/agents/tracing/provider.py:339-352`) for consistency between the two mechanisms.
- Document the primary "embedded library" deployment mode (e.g., ASGI/FastAPI embedding) as explicitly as the sandbox-client matrix.
- Add a first-class error for "no API key configured anywhere" at `Runner` level to replace the raw `openai.OpenAIError` surfacing observed in `tests/test_config.py:25-28`.

## Questions / Gaps

- No evidence found of staged rollout or percentage-based feature flagging anywhere in the source (searched `src/agents/` for flag registries, rollout, bucketing — only per-process env toggles exist).
- No evidence found of config-file support (TOML/YAML/env-file loading) in the SDK itself; if projects want file-based configuration they must build it. Search boundary: `src/agents/**` greps for `yaml`, `.toml`, `dotenv`, `configparser`, `json.load`.
- Staging-environment management specifically (as distinct from dev/prod seams) is not addressed by the repository; no CI/deploy manifests for consuming applications exist here beyond the package publish workflow (`.github/workflows/publish.yml`), so claims about staging parity rest on the sandbox-client and session-backend documentation rather than observed deployment pipelines.
- The `temporal` extra is declared in `pyproject.toml:56-59` but no corresponding runtime module was located under `src/agents/` during this pass; its role (likely workflow-durable execution samples) remains unclear from the selected source alone.

---

Generated by `dimensions/22.02-configuration-and-deployment-shape.md` against `openai-agents-sdk`.

# Source Analysis: agent-framework

## API Versioning and Compatibility

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python 3.10-3.14, .NET 8/9/10, Go (stub), monorepo (uv workspaces + MSBuild Central Package Management) |
| Analyzed | 2026-08-27 |

## Summary

Agent-framework uses a two-level lifecycle (package stage + feature stage) rather than classical API version fields. Every Python package carries `PACKAGE_STATUS.md:6-52` (`alpha`/`beta`/`rc`/`released`/`deprecated`) with semver enforced for `released` packages (`CHANGELOG.md:5-6`, `python/pyproject.toml:7` `version="1.15.0"`) and date-stamped `1.0.0a<YYMMDD>`/`1.0.0b<YYMMDD>` for pre-1.0 (`python/.github/skills/python-package-management/SKILL.md:224-228`). .NET mirrors this with `VersionSuffix=preview|alpha` per `.csproj` and central `PackageValidationBaselineVersion` (`dotnet/src/Microsoft.Agents.AI.AgentHooks/Microsoft.Agents.AI.AgentHooks.csproj:21`, `dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/Microsoft.Agents.AI.Foundry.Hosting.csproj:6-8`). API maturity inside a `released` package is signaled by `@experimental`/`@release_candidate` decorators on `ExperimentalFeature`/`ReleaseCandidateFeature` (`python/packages/core/agent_framework/_feature_stage.py:43-69`) that add docstring banners and one-time `ExperimentalWarning`/`FutureWarning` at the caller site. Deprecation uses `typing_extensions.deprecated`/`warnings.DeprecationWarning` with explicit `stacklevel` and migration guidance. Persisted contracts have explicit version fields (`WorkflowCheckpoint.version="1.0"` at `python/packages/core/agent_framework/_workflows/_checkpoint.py:98`, `FileSessionStore _SESSION_SNAPSHOT_VERSION="1.0"` at `python/packages/core/agent_framework/_sessions.py:115`) plus a global typed checkpoint/wrapped state registry (`register_state_type` at `python/packages/core/agent_framework/_sessions.py:325`, `register_checkpoint_type` at `python/packages/core/agent_framework/_workflows/_checkpoint_encoding.py:64`) and a restricted unpickler allowlist. Brevity of `@experimental(feature_id=...)` and lack of a central deprecation catalog plus no executable backwards-compat matrix means safety still leans on policy/docs rather than hard CI enforcement — hence rating 6.

## Rating

**6 / 10 — Present but inconsistent, weakly documented, fragile in edges**

Rationale: Package- and feature-level staging is explicit, consistently documented (`PACKAGE_STATUS.md`, `SKILL.md` lifecycles, `_feature_stage.py`), semver is declared and CHANGELOG marks every `BREAKING` with scope (`[BREAKING]`, `[BREAKING — experimental]`, `[BREAKING — beta]`), and persisted snapshot versions + type registries exist. Against that, (a) there is no single compatibility/migration policy doc, (b) deprecation notices are scattered `warnings.warn` without unified `DEPRECATED.md`, (c) no executable backwards-compat or version-negotiation test suite was found — only unit checks for serialization round-trips and semconv switches, (d) protocol versioning (MCP `protocolVersion`, AG-UI events, hosting channel codecs) is handled ad-hoc per package, and (e) failed checkpoint/session deserialization leaves behavior to caller (exception vs quarantine) without automated rollback.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Package lifecycle model | Package stage taxonomy and feature-stage decorator rules; `alpha`→`beta`→`rc`→`released` with classifier mapping | `python/.github/skills/python-package-management/SKILL.md:202-236` |
| Package lifecycle model | Feature enums `ExperimentalFeature`/`ReleaseCandidateFeature` described as current inventories, not stable introspection | `python/packages/core/agent_framework/_feature_stage.py:43-69` |
| Package status inventory | 33 packages mapped to `alpha/beta/rc/released/deprecated` with `agent-framework-core` `released` | `python/PACKAGE_STATUS.md:14-52` |
| Feature inventory | 14 experimental feature IDs + 0 RC, inventory tied to decorator usage | `python/PACKAGE_STATUS.md:64-153` |
| Feature-stage mechanics | `@experimental` injects `.. warning:: Experimental` banner, sets `__feature_stage__`/`__feature_id__`, warns once via `ExperimentalWarning` with frame-aware caller resolution | `python/packages/core/agent_framework/_feature_stage.py:29-34,161-168,304-383,435-442` |
| Release-candidate mechanics | `@release_candidate` adds `.. note:: Release candidate`, no runtime warning | `python/packages/core/agent_framework/_feature_stage.py:35-40,445-455` |
| Deprecated pattern | Canonical version-conditional import `warnings.deprecated` / `typing_extensions.deprecated` and mapping table Released→no decorator, Deprecated→`@deprecated("...")` | `python/.github/skills/python-feature-lifecycle/SKILL.md:108-138` |
| Root version field | Core version read from distribution metadata with fallback, exported as `__version__` | `python/packages/core/agent_framework/__init__.py:21-24` |
| Root meta version | Meta package pins `version="1.15.0"` and depends on `agent-framework-core[all]==1.15.0` | `python/pyproject.toml:7,26` |
| Core package version | `agent-framework-core` semver declaration `version="1.15.0"` + classifier `Production/Stable` | `python/packages/core/pyproject.toml:7,15` |
| Semver claim | `CHANGELOG.md` header declares Keep a Changelog + SemVer compliance | `python/CHANGELOG.md:5-6` |
| Breaking-change signaling | CHANGELOG entries distinguish `[BREAKING]`, `[BREAKING — experimental]`, `[BREAKING — beta]` across 1.10-1.15 (e.g., workflow checkpoints replayable, `create_harness_agent` graduation, `hosting-responses` conversation ID helpers) | `python/CHANGELOG.md:19-20,58-59,104,181,191-194` |
| .NET suffix staging | Per-project `VersionSuffix` `preview`/`alpha` + `PackageValidationBaselineVersion` for API compat validation | `dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/Microsoft.Agents.AI.Foundry.Hosting.csproj:6,25` ; `dotnet/src/Microsoft.Agents.AI.Mcp/Microsoft.Agents.AI.Mcp.csproj:6,26` ; `dotnet/src/Microsoft.Agents.AI.AgentHooks/Microsoft.Agents.AI.AgentHooks.csproj:21` |
| .NET central versioning | Central Package Management with CPM pinning, shared `Directory.Build.props` per-TFM settings | `dotnet/Directory.Packages.props:2-6` ; `dotnet/Directory.Build.props:12-14` |
| Deprecation warnings (runtime) | `DeprecationWarning` for implicit Pydantic session state registration and `FileHistoryProvider` `dumps`/`loads` | `python/packages/core/agent_framework/_sessions.py:396-401,453,2249-2253` |
| Deprecation warnings (workflows) | `Runner` deprecated with warning, `WorkflowEvent.emit() type='data'` deprecated alias, `reset_for_new_run` deprecated | `python/packages/core/agent_framework/_workflows/_runner.py:30-40` ; `python/packages/core/agent_framework/_workflows/_events.py:282-291` ; `python/packages/core/agent_framework/_workflows/_runner_context.py:183,485` |
| Backward-compat shims | `WorkflowEvent` retains `type="data"` as deprecated alias for `intermediate`; `Runner` lazily warns via `__getattr__` | `python/packages/core/agent_framework/_workflows/_events.py:141,282` |
| Persisted-format version: checkpoint | `WorkflowCheckpoint.version: str = "1.0"` + docstring that version is format version | `python/packages/core/agent_framework/_workflows/_checkpoint.py:74,98` |
| Persisted-format version: session snapshot | `Literal` typed snapshot `version: str = _SESSION_SNAPSHOT_VERSION` where `_SESSION_SNAPSHOT_VERSION = "1.0"`; explicit mismatch check that raises with file path | `python/packages/core/agent_framework/_sessions.py:115,537,1974-1976` |
| Session state registry | `register_state_type(cls, type_id=?, encoder=?, decoder=?)` with global uniqueness, collision failure, package-qualified ID guidance | `python/packages/core/agent_framework/_sessions.py:325-392` |
| Checkpoint type registry | `register_checkpoint_type(cls)` process-wide + `_BUILTIN_ALLOWED_TYPE_KEYS` + `_RestrictedUnpickler.find_class` allowlist (builtins + `agent_framework.*` + `openai.types.*` + caller extras) | `python/packages/core/agent_framework/_workflows/_checkpoint_encoding.py:64-238` |
| Serialization mixin | `SerializationMixin.to_dict()` injects `type` discriminator, `from_dict()` validates `type` mismatch; shallow `type` identifier fallback `TYPE`/`type`/snake_case | `python/packages/core/agent_framework/_serialization.py:306-346,542-546,618-646` |
| Session snapshot codec | `msgspec.Struct _SessionSnapshot` with typed `type/version` + `serialization_format="msgpack"|"json"` hooks; file extension selection + non-finite float fallback | `python/packages/core/agent_framework/_sessions.py:530-551,108-122` |
| Checkpoint encoding | JSON-native passthrough vs pickle+base64 envelope with `__pickled__`/`__type__` markers, blocked framework globals | `python/packages/core/agent_framework/_workflows/_checkpoint_encoding.py:83-88,241-322` |
| Capability / version negotiation | AGENT-HOOKS-0.1 enforcement behind opt-in `agent-hooks` extra; `create_agent_hooks_middleware` gated by `agent-framework-core[agent-hooks]` | `python/packages/core/pyproject.toml:36-37` ; `python/PACKAGE_STATUS.md:72-77` |
| Capability / version negotiation | MCP `protocolVersion` echo in tests (`LATEST_PROTOCOL_VERSION`), `MCPTool` allowed `allowed_tools`/`additional_tool_argument_names` scopes; MCP `header_provider` not consumed from `function_invocation_kwargs` | `python/packages/core/tests/core/test_mcp.py:5286-5288,6437` ; `python/packages/core/agent_framework/_mcp.py:534-554` (context from grep) |
| Capability / version negotiation | OpenTelemetry GenAI semconv switch: `GEN_AI_LATEST_EXPERIMENTAL_OPT_IN="gen_ai_latest_experimental"`, `use_latest_experimental_gen_ai_semconv` defaults to `True`, `ENABLE_MESSAGE_EVENTS` defaults to `True` for backward compat | `python/packages/core/agent_framework/observability.py:739-742,894-909` ; `python/packages/core/tests/core/test_observability.py:2013,3411-3479` |
| Feature usage bitmask versioning | `docs/decisions/0033-feature-usage-bitmask-user-agent.md` registry v1→v2 migration: reserved bits stay decodable by older decoders | `docs/decisions/0033-feature-usage-bitmask-user-agent.md:540-546` |
| Session store migration design | ADR 0034 describes file store quarantine policy: only syntactically malformed snapshots quarantined; schema/version/decoder failures preserve file for recovery; `version` describes payload shape, not codec | `docs/decisions/0034-python-session-store-serialization.md:275-299` |
| Release validation | Dependency-bounds lower/upper matrix + `validate-python-release` probes `lowest-direct`/`highest` on min Python | `python/.github/skills/python-package-management/SKILL.md:73-83` ; `python/pyproject.toml:402-416` |
| Foundry Hosting contract versioning | `FoundryHosting` description marks v2.0-only Responses protocol, not compatible with v1; `FoundrySessionStore` path derivation | `dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/Microsoft.Agents.AI.Foundry.Hosting.csproj:8` ; `docs/decisions/0034-python-session-store-serialization.md:212-232` |
| SDK vs CLI vs server surfaces | Python `PACKAGE_STATUS.md` per-package stages differ across `hosting` `alpha` vs `core` `released`; .NET adds `IsReleased`/`IsReleaseCandidate` flags per `Directory.Build.props:19-20` | `python/PACKAGE_STATUS.md:6-52` ; `dotnet/Directory.Build.props:19-20` |
| Changelog as compatibility log | Unreleased section template + per-package `Added/Changed/Fixed/Removed` with breaking tags | `python/CHANGELOG.md:8-10,12-41` |

## Answers to Dimension Questions

**1. Which APIs are stable, experimental, deprecated, or internal?**
- **Stable (`released`)**: `agent-framework-core`, `agent-framework-ag-ui`, `agent-framework-declarative`, `agent-framework-foundry`, `agent-framework-github-copilot`, `agent-framework-openai`, `agent-framework-orchestrations`, plus meta `agent-framework` (`python/PACKAGE_STATUS.md:17,19,29-32,36,48`). Inside those, APIs *without* `@experimental`/`@release_candidate` are the stable surface by default (`python/.github/skills/python-feature-lifecycle/SKILL.md:92-101`).
- **Release-candidate**: No feature-level `rc` entries today, but per-package `rc` stage exists in taxonomy (`python/PACKAGE_STATUS.md:10`, `python/.github/skills/python-package-management/SKILL.md:179-188`). .NET uses `IsReleaseCandidate` flag (`dotnet/Directory.Build.props:19`).
- **Experimental**: Either whole package (`alpha`/`beta` such as `hosting`, `hosting-a2a`, `foundry-hosting`, `mem0`, etc. at `python/PACKAGE_STATUS.md:18-52`) or per-API `@experimental(feature_id=ExperimentalFeature.X)` — current inventory: `AGENT_HOOKS`, `DECLARATIVE_AGENTS`, `EVALS`, `FILE_HISTORY`, `FIDES`, `FOUNDRY_TOOLS`, `FOUNDRY_PREVIEW_TOOLS`, `FUNCTIONAL_WORKFLOWS`, `HARNESS`, `MCP_LONG_RUNNING_TASKS`, `MCP_SKILLS`, `PROGRESSIVE_TOOLS`, `SESSION_STORE`, `TO_PROMPT_AGENT` (`python/PACKAGE_STATUS.md:64-150`, `python/packages/core/agent_framework/_feature_stage.py:52-67`). Example: `python/packages/core/agent_framework/_mcp.py:346` `MCPTaskOptions`, `python/packages/core/agent_framework/_sessions.py:1794` `SessionStore`, `python/packages/core/agent_framework/_workflows/_functional.py:74` `FunctionalWorkflow`.
- **Deprecated**: Single package `agent-framework-azure-ai` (`python/PACKAGE_STATUS.md:58`), plus API-level deprecated aliases: `Runner` (`python/packages/core/agent_framework/_workflows/_runner.py:30-40`), `WorkflowEvent.emit/type='data'` (`python/packages/core/agent_framework/_workflows/_events.py:282-289`), `Workflow.reset_for_new_run` (`python/packages/core/agent_framework/_workflows/_runner_context.py:183`), `FileHistoryProvider(dumps/loads)` and implicit Pydantic `register_state_type` (`python/packages/core/agent_framework/_sessions.py:396,2249`), `agent_framework.azure`/`openai` legacy wrappers per ADR 0021 (`docs/decisions/0021-provider-leading-clients.md:35-38`).
- **Internal**: `_`-prefixed modules (`_sessions.py`, `_checkpoint_encoding.py`, `_feature_stage.py`, `_serialization.py`), `IsPackable=false` projects like `Microsoft.Agents.AI.Declarative` (`dotnet/src/Microsoft.Agents.AI.Declarative/Microsoft.Agents.AI.Declarative.csproj:6`) and `Microsoft.Agents.AI.Hyperlight`, and `__getattr__`-lazy provider namespaces (not enumerated as public `__all__`).

**2. How are users warned before breaking changes?**
Via three complementary channels, but without a single migration guide catalog: (i) CHANGELOG `[BREAKING]` prefix scoped to `experimental`/`beta` vs stable (`python/CHANGELOG.md:19,58,104`); (ii) runtime warnings — `ExperimentalWarning(FutureWarning)` for experimental use (`python/packages/core/agent_framework/_feature_stage.py:84-85,236-262`) with single dedup per `feature_id`, and `DeprecationWarning` for deprecated paths (`python/packages/core/agent_framework/_sessions.py:396`); the `@deprecated` pattern recommends `warnings.deprecated`/`typing_extensions.deprecated` with a replacement string (`python/.github/skills/python-feature-lifecycle/SKILL.md:108-118`); (iii) ADRs that explicitly state breaking and mitigation (e.g., ADR 0034 quarantine vs preserve policy at `docs/decisions/0034-python-session-store-serialization.md:275-279`, ADR 0021 wrapper consolidation at `docs/decisions/0021-provider-leading-clients.md:30-38`). Gap: no automated CHANGELOG lint enforcing the `[BREAKING]` prefix, and per-package deprecations lack central `DEPRECATED.md`.

**3. Are old clients, plugins, traces, or persisted artifacts still usable?**
Selectively:
- **Clients/plugins**: MCP `LATEST_PROTOCOL_VERSION` negotiation is present but not version-gated in code; tests pin `protocolVersion` echo (`python/packages/core/tests/core/test_mcp.py:5286`), while Foundry Hosting deliberately drops v1 compatibility (`dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/Microsoft.Agents.AI.Foundry.Hosting.csproj:8` notes `v2.0 only and is not compatible with v1`). AG-UI and hosting channel protocols are package-versioned rather than independently versioned.
- **Persisted artifacts**: Workflow checkpoints carry `version="1.0"` but `FileCheckpointStorage._checkpoint_encoding` is codec-driven, not format-version-driven; `decode_checkpoint_value` verifies `_TYPE_MARKER` equality and blocks disallowed types rather than migrating old versions (`python/packages/core/agent_framework/_workflows/_checkpoint_encoding.py:362-383,204-238`). Session snapshots enforce `snapshot.version == _SESSION_SNAPSHOT_VERSION` and raise on mismatch (`python/packages/core/agent_framework/_sessions.py:1974`); ADR 0034 explicitly proposes reader-first migration (detect codec from first byte, widen glob) but notes MessagePack must not become default while mixed fleets coexist (`docs/decisions/0034-python-session-store-serialization.md:294-299`). Functional workflow resume checks `checkpoint.workflow_name` + compatible version and raises `"created by a different version ... not compatible"` (`python/packages/core/agent_framework/_workflows/_functional.py:989-990`).
- **Traces**: OTel GenAI semconvs are versioned with `gen_ai_latest_experimental` opt-in and `ENABLE_MESSAGE_EVENTS` defaulting to baseline for backward compat (`python/packages/core/agent_framework/observability.py:739-742,894-909`, `python/packages/core/tests/core/test_observability.py:2013`); `LATEST_EXPERIMENTAL_GEN_AI_ATTRIBUTES` set gates newer attributes (`python/packages/core/agent_framework/observability.py:398-404`).

**4. Does compatibility rely on policy alone or executable tests?**
Mostly policy + spot tests, not a matrix. Policy artifacts: semver claim (`python/CHANGELOG.md:6`), lifecycle checklists (`python/.github/skills/python-package-management/SKILL.md:166-236`), ADR-published driver tables (`docs/decisions/0034-python-session-store-serialization.md:38-89`). Executable signals that exist: (a) `pyproject` dependency-bounds lower/upper probes (`validate-dependency-bounds-project`, `validate-python-release` at `python/.github/skills/python-package-management/SKILL.md:73-83`), (b) `.NET` API compat validation via `PackageValidationBaselineVersion` on some projects (`dotnet/src/Microsoft.Agents.AI.Workflows.Declarative.Mcp/Microsoft.Agents.AI.Workflows.Declarative.Mcp.csproj:20`), (c) unit tests for snapshot round-trips, checkpoint encode/decode, `test_observability` semconv switches, and `test_mcp` protocolVersion echo. Missing: no repo-wide `backwards_compat_test.go/py` matrix that loads N-1 serialized `Message`/`AgentSession`/`WorkflowCheckpoint` golden files and asserts `from_dict`/decode succeeds; no CI job that installs previous `agent-framework-core` and replays traces/prompts.

## Architectural Decisions

- **Decision: Two-level lifecycle (package stage + feature stage).** Package stage sets default maturity; `@experimental`/`@release_candidate` are exceptions inside `released` packages. Keeps `beta` packages decorator-free while letting `released` packages incubate `HARNESS`/`EVALS`/etc. (`python/.github/skills/python-feature-lifecycle/SKILL.md:12-36`, `python/packages/core/agent_framework/_feature_stage.py:386-455`).

- **Decision: Semver per released package, date-stamped pre-1.0 for alpha/beta.** `released: X.Y.Z` vs `1.0.0aYYMMDD`/`1.0.0bYYMMDD`/`1.0.0rcN` with matching PyPI `Development Status` classifiers (`python/.github/skills/python-package-management/SKILL.md:223-235`). `package-upgrades` skill covers internal `agent-framework-core` floor bumps.

- **Decision: Explicit type registry for durable session state + allowlist for checkpoints.** `register_state_type` enforces globally unique `type_id`, stable codec, collision fail-fast (`python/packages/core/agent_framework/_sessions.py:325-391`); `register_checkpoint_type` + `_RestrictedUnpickler` reduces pickle blast radius (`python/packages/core/agent_framework/_workflows/_checkpoint_encoding.py:64-238`). App code registers at module import so cold-start deserialization works without consumer plumbing.

- **Decision: msgspec typed snapshot as internal DTO, not base class.** `AgentSession` stays plain; internal `msgspec.Struct _SessionSnapshot` validates `type/version` and carries state payload via single hook, giving one typed encode/decode per file write/read plus optional `msgpack` (`docs/decisions/0034-python-session-store-serialization.md:234-285`, `python/packages/core/agent_framework/_sessions.py:530-551`). Benchmark at `docs/decisions/0034-python-session-store-serialization.md:169-173` justifies not regressing.

- **Decision: Capability-based feature-usage bitmask with reserved bits.** Registry v1→v2 adds bits without breaking older decoders (they see reserved as zero), tying adoption telemetry to User-Agent version string (`docs/decisions/0033-feature-usage-bitmask-user-agent.md:540-546`, `dotnet/src/Microsoft.Agents.AI.Abstractions/FeatureUsage.cs:24,122`).

- **Decision: Opt-in experimental hosting seams via extras / protocol v2 hard break.** `agent-hooks` behind `agent-framework-core[agent-hooks]` (`python/packages/core/pyproject.toml:36`), Durable Task extraction behind core shim (`docs/decisions/0032-durable-azure-functions-extraction.md:41`), Foundry Hosting v2-only container protocol (`dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/Microsoft.Agents.AI.Foundry.Hosting.csproj:8`) — breaking is staged via ADR.

## Notable Patterns

- **Experimental warning once-per-feature with attribution.** Deduplicates on `(_WARNED_FEATURES: set[tuple[type[Warning], str]])` keyed by `(category, feature_id)` (`python/packages/core/agent_framework/_feature_stage.py:28`), routes through caller-frame resolution to emit at user site, not framework site (`_resolve_user_frame`, `_warn_on_feature_use` at `python/packages/core/agent_framework/_feature_stage.py:202-262`).

- **Serialization `type` discriminator + INJECTABLE exclusion.** Every `SerializationMixin.to_dict()` injects `type`, excludes `INJECTABLE`/`DEFAULT_EXCLUDE`/`_`-attrs, and `from_dict()` validates mismatch (`python/packages/core/agent_framework/_serialization.py:325-346,542-546`); enables `Message`/`Workflow` round-trips without version branch.

- **Opaque session IDs mapped to portable filenames.** `_is_literal_session_file_stem_safe` + `_session_file_stem` encodes non-portable IDs via `urlsafe_b64encode` or `sha256` digest (`python/packages/core/agent_framework/_sessions.py:148-177`) — versioning of session *identity* is filesystem-safe, not semantic.

- **Checkpoint hybrid JSON+pickle envelope.** JSON-native scalars pass through; non-JSON types become `{ "__pickled__": base64, "__type__": "module:qualname" }` with post-decode `_verify_type` (`python/packages/core/agent_framework/_workflows/_checkpoint_encoding.py:83-89,241-384`).

- **GenAI semconv version gating.** `LATEST_EXPERIMENTAL_GEN_AI_ATTRIBUTES` frozenset + `use_latest_experimental_gen_ai_semconv` predicate control `TOOL_DEFINITIONS`/cache/reasoning attributes (`python/packages/core/agent_framework/observability.py:398-404,895-909`).

## Tradeoffs

- **Per-package semver without enforced single source of truth for breaking detection.** `PackageValidationBaselineVersion` is set on only a subset of .NET projects (e.g., `Workflows.Declarative.Mcp` at `dotnet/src/Microsoft.Agents.AI.Workflows.Declarative.Mcp/Microsoft.Agents.AI.Workflows.Declarative.Mcp.csproj:20`, absent on `Foundry`, `Mcp`, `Harness`) — inconsistent enforcement risks silent compat drift. Python relies on contributor discipline to add `[BREAKING]` in CHANGELOG, not lint.

- **Date-stamped pre-1.0 makes ordering predictable but not semantic.** `1.0.0b260721` → `1.0.0b260822` is monotonic but hides whether the bump is patch/minor/major (`python/.github/skills/python-package-management/SKILL.md:225`); consumers on `--pre` float may ingest breaking alpha without semver signal.

- **Restricted unpickler improves safety but allows framework-wide `agent_framework.*` wildcard.** `find_class` auto-allows any `agent_framework.` top-level class (`python/packages/core/agent_framework/_workflows/_checkpoint_encoding.py:222-228`) — convenient for compat but broad attack surface; blocked list is only 4 entries (`_BLOCKED_FRAMEWORK_GLOBAL_KEYS`).

- **msgspec JSON default keeps dumps readable, but dual-format plan is unfunded.** ADR 0034's reader-first migration (detect codec from first byte, widen `glob("*.json")`) is still TODO; today `FileCheckpointStorage` glob ignores future `*.msgpack` files, so a premature default flip would silently resume from no checkpoint on mixed fleets (`docs/decisions/0034-python-session-store-serialization.md:294-299`).

- **Feature-usage bitmask adoption vs presence signal is richer but couples version to telemetry.** Bit registry v1→v2 reuses reserved bits safely, yet any new bit requires coordinated `AGENT_FRAMEWORK_USER_AGENT` version bump (`docs/decisions/0033-feature-usage-bitmask-user-agent.md:540`, `python/packages/core/agent_framework/_telemetry.py:13,26-33`).

## Failure Modes / Edge Cases

- **Checkpoint deserialization hard-fails on type mismatch or disallowed type.** `_verify_type` raises `WorkflowCheckpointException` on `expected != actual` (`python/packages/core/agent_framework/_workflows/_checkpoint_encoding.py:377-383`); `_RestrictedUnpickler.find_class` raises `UnpicklingError` wrapped as `WorkflowCheckpointException` (`_base64_to_unpickle` at line 410). Without allowlist entry for a new app state type, roll-forward of a checkpoint that contains that type crashes resume — no fallback migration function.

- **Session snapshot version mismatch aborts load, file preserved.** `FileSessionStore` decode checks `snapshot.version != _SESSION_SNAPSHOT_VERSION` and leaves file in place for manual recovery (`python/packages/core/agent_framework/_sessions.py:1974-1976`, ADR 0034 `275-279`). Saves are atomic last-writer-wins `json` dumps (`python/packages/core/agent_framework/_sessions.py:XXX` encoding hook), so concurrent writers can silently overwrite — no CAS/ETag/sequence guard.

- **Functional workflow checkpoint rejected on topology change.** `checkpoint.workflow_name` + `graph_signature_hash` validated; mismatch raises `not compatible with the current version` (`python/packages/core/agent_framework/_workflows/_functional.py:989-990` and `\_checkpoint.py:81-84`). No forward-migration shim exists — user must drain or recreate workflow.

- **Implicit Pydantic registration emits warning but still round-trips same-process.** Same-process tests pass, but cold-start restoration fails without explicit `register_state_type` — lake of `DeprecationWarning` that is easily missed in CI (`python/packages/core/agent_framework/_sessions.py:394-452`).

- **Otel semconv toggle changes span attribute set.** Flipping `OTEL_SEMCONV_STABILITY_OPT_IN` from default `gen_ai_latest_experimental` to baseline drops `gen_ai.tool.definitions`/cache/reasoning attributes, breaking dashboards that assume latest set (`python/packages/core/agent_framework/observability.py:894-934`, `LATEST_EXPERIMENTAL_GEN_AI_ATTRIBUTES`).

- **Mixed-version fleet + codec default flip = silent resume from empty.** Per ADR 0034, older readers only glob `*.json`; if a newer writer defaults to `*.msgpack` the older fleet finds no checkpoint and resumes stateless without error (`docs/decisions/0034-python-session-store-serialization.md:294-299`).

- **.NET ADR 0021 wrapper renames require deprecated aliases; missing shim breaks compile.** `OpenAIResponsesClient→OpenAIChatClient`, `Azure*` consolidation to `_deprecated_azure_openai.py` single file — obsolete helpers are "only as migration shims" and slated for deletion (`docs/decisions/0021-provider-leading-clients.md:30-39`).

## Future Considerations

- Adopt a central `Compatibility.md` and `DEPRECATED.md` plus `python/scripts/check_changelog_breaking.py` lint that enforces `[BREAKING]` wording and requires migration note for stable `released` bumps — current guidance lives scattered across SKILLs and ADRs.
- Make `PackageValidationBaselineVersion` non-empty on every packable .NET project; wire `Microsoft.DotNet.ApiCompat` to CI so breaking is executable, not just per-project opt-in.
- Add goldens-backed backwards-compat test matrix: serialize sample `Message`/`AgentSession`/`WorkflowCheckpoint` with current version, commit as `tests/compat-goldens/v1.15/`, then assert `from_dict`/`decode_checkpoint_value` round-trips on every PR.
- Complete ADR 0034 reader-first migration: widen `FileCheckpointStorage.list_checkpoints` glob to `*.*` with codec detection from first byte; keep write default `json` one release, then expose `serialization_format="msgpack"` only after mixed-fleet window closes.
- Narrow checkpoint unpickler wildcard: replace `module.startswith("agent_framework.")` auto-allow with explicit `FRAMEWORK_ALLOWED_TYPE_KEYS` allowlist generated from `src/` exports, and expand blocked globals as new helper callables appear.
- Stabilize `FunctionalWorkflow` and `SESSION_STORE` experimental surfaces (`python/PACKAGE_STATUS.md:119,141`) via RC step with versioned wire `type` discriminator so checkpoints can be upgraded by a `migrate(snapshot_version)` function rather than hard rejection.
- Introduce per-protocol semver (`mcp.protocol.version`, `ag-ui` event schema version, hosting Responses container v2→v3) with `MinSupportedVersion`/`CurrentVersion` negotiation instead of single package version.

## Questions / Gaps

- No evidence found for a formal LTS or deprecation window (e.g., "deprecated for 2 minor releases then removed") — SKILL states lifecycle stages but not timelines; search boundary: `python/.github/skills/python-feature-lifecycle/SKILL.md`, `python/PACKAGE_STATUS.md`, `docs/decisions/`.
- No evidence found for automated semver breaking-change detection on Python (e.g., `griffe`/`semver-diff` CI check) — only dotnet `PackageValidationBaselineVersion` which is sparsely set.
- No evidence found for client-capability negotiation beyond extra-opt-in (`agent-hooks`) and MCP `allowed_tools` filtering — no `ClientHello { min_version, max_version, capabilities }` handshake.
- No evidence found for schema migration functions `migrate_v1_to_v2(checkpoint_dict)` — only raise-on-mismatch pattern; search boundary: `python/packages/core/agent_framework/_workflows/_checkpoint*`, `_sessions.py`.
- Changelog covers `Unreleased` template per release skill but not a per-package `MIGRATION.md` with before/after code samples — `docs/decisions/0030-hosted-platform-context-agentserver-2.0.md:23` mentions keeping hosted samples source-compatible but without versioned guide.

---

Generated by `Dimension 24.03: API Versioning and Compatibility` against `agent-framework`.

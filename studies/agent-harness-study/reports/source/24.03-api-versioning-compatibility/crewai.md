# Source Analysis: crewai

## API Versioning and Compatibility

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python / Pydantic, uv workspace (crewai, crewai-core, crewai-cli, crewai-tools, crewai-files, devtools) |
| Analyzed | 2026-08-27 |

## Summary

CrewAI uses a single synchronized semver string (`1.15.17`) across all workspace packages with dynamic `hatchling` versioning, rather than independently versioned APIs. The only formal persisted-contract versioning is the `crewai_version` stamp in `RuntimeState` checkpoints with a thin `_migrate` hook for pre-1.14.6 checkpoints. Public surface is informally tiered: `crewai.experimental` is explicitly unstable (may break without major bump), core `Agent/Crew/Task/Flow/Knowledge/Tool` is treated as stable, and deprecated fields surface `FutureWarning`/`warn_deprecated` shims that map old args to the new shape. Capability negotiation exists only for the A2A layer (transport + content-type + protocol version literal) and not for the core SDK. Changelogs and per-field migration guides live in frozen `docs/v*/en/changelog.mdx` snapshots, but there is no machine-enforced semver gate, no API-extractor, and no compatibility test matrix covering old clients/traces/plugins. Backwards-compatibility is tested for checkpoint restore and version-cache logic, but not broadly for breaking API changes.

## Rating

**5 / 10 — Present but inconsistent, weakly documented, and fragile.**

Rationale: version stamping and deprecation warnings exist and are exercised in code, and A2A negotiation is well-implemented; however migration coverage is narrow (single `<1.14.6` checkpoint block), experimental vs stable boundaries are informal, breakage communication is docs-only, and there are no executable compatibility contracts (e.g., schema snapshot tests, semver enforcement, plugin compatibility harness). Upgrading without auditing internal changes is risky for checkpoint persistence and experimental surfaces.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Version field – single source of truth | `__version__ = "1.15.17"` hard-pinned in SDK, core, and CLI | `lib/crewai/src/crewai/__init__.py:51`, `lib/crewai-core/src/crewai_core/__init__.py:1`, `lib/cli/src/crewai_cli/__init__.py:1` |
| Dynamic versioning | `build-backend = hatchling`, `[tool.hatch.version] path = "src/crewai/__init__.py"` and equivalents for core/CLI | `lib/crewai/pyproject.toml:150-155`, `lib/crewai-core/pyproject.toml:30-35`, `lib/cli/pyproject.toml:39-42` |
| Workspace pin | `crewai-core==1.15.17` and `crewai-cli==1.15.17` exact pins; `crewai-tools==1.15.17` as optional dep | `lib/crewai/pyproject.toml:11-12,58` |
| Commitizen / bump policy | `commitizen bump_map {feat=MINOR, fix=PATCH, perf=PATCH}` and conventional schema | `pyproject.toml:156-169` |
| Version detection + PyPI freshness | `get_crewai_version()` via `importlib.metadata`, fallback `unknown`; `get_latest_version_from_pypi()` with 24h file cache, yank filtering, and `is_newer_version_available()` / `is_current_version_yanked()` | `lib/crewai-core/src/crewai_core/version.py:25-194` |
| Version re-export shim | Deprecated `crewai.utilities.version` warns and re-exports from `crewai_core.version` | `lib/crewai/src/crewai/utilities/version.py:1-14` |
| User warning – newer/yanked banner | `ConsoleFormatter._show_version_update_message_if_needed()` checks `is_newer_version_available()` and `is_current_version_yanked()`, hides in CI (`CI` env) and when `verbose=False` | `lib/crewai/src/crewai/events/utils/console_formatter.py:60-112` |
| Persisted contract version | `RuntimeState._serialize()` embeds `"crewai_version": get_crewai_version()`; `_deserialize` calls `_migrate(data)` | `lib/crewai/src/crewai/state/runtime.py:190-214` |
| Schema migration hook | `_migrate()` compares stored `Version(raw)` vs current, warns on missing version, backfills discriminators for `<1.14.6`: `memory_kind` and `source_type` | `lib/crewai/src/crewai/state/runtime.py:89-175` |
| Checkpoint tests – version | Serialize includes version; deserialize warns/migrates on mismatch/missing | `lib/crewai/tests/test_checkpoint.py:205-236` |
| Version utility tests | Tests for cache freshness, PyPI fetch with yank filtering, `is_newer_version_available`, `_find_latest_non_yanked_version`, `_is_version_yanked`, yank banner | `lib/crewai/tests/cli/test_version.py:23-507`, `lib/crewai-core/tests/test_smoke.py:22` |
| Changelog / upgrade notes | Commit `Embed crewai_version in checkpoints with migration framework` + per-release changelogs frozen under `docs/v*/en/changelog.mdx`; snapshots managed by devtools | `docs/v1.14.3/en/changelog.mdx:277`, `docs/v1.15.17/en/changelog.mdx:1-3600` (snapshot set), `scripts/docs/freeze_current_edge.py:63` |
| Deprecation guide – flow | `flows/inputs-id-deprecation.mdx`: `inputs={"id": <uuid>}` deprecated → `restore_from_state_id` | `docs/v1.15.17/en/guides/flows/inputs-id-deprecation.mdx:3-54` |
| Experimental surface | Module docstring: `Experimental CrewAI surface — APIs here may change without major-version bumps.` Lazy re-export of `AgentExecutor`, evaluation, conversational types | `lib/crewai/src/crewai/experimental/__init__.py:1,23-72` |
| Conversational experimental warning | Pin-your-version disclaimer on `RouterConfig` / `ConversationConfig` | `lib/crewai/src/crewai/experimental/conversational.py:51,73,150` |
| Deprecated A2A config | `@deprecated("A2AConfig is deprecated ... remove in v2.0.0")` with `FutureWarning`; deprecated fields `transport_protocol`, `supported_transports`, `use_client_preference`, `preferred_transport`, `signatures` migrated via `_migrate_*` + warnings | `lib/crewai/src/crewai/a2a/config.py:303-322,363-462,543-549,690-706` |
| Agent deprecations | `CrewAgentExecutor` → `AgentExecutor` (`DeprecationWarning`), `function_calling_llm` (`deprecated=`), `allow_code_execution` / `reasoning` (`warnings.warn`), `Task.max_retries` (`warnings.warn`) | `lib/crewai/src/crewai/agent/core.py:165-172,235,375-387,1258`, `lib/crewai/src/crewai/task.py:576-578` |
| CLI deprecation helper | `warn_deprecated(kind="command|flag", old, new)` prints yellow `click.secho`; used for `crewai tool create → crewai create tool`, `crewai flow kickoff → crewai run`, `--n_iterations → --n-iterations`, `--skip_provider → --skip-provider` | `lib/cli/src/crewai_cli/utils.py:52-63`, `lib/cli/src/crewai_cli/cli.py:192-942` |
| Pydantic field deprecations | `signatures` and `preferred_transport` marked `deprecated=True, exclude=True` on `A2AServerConfig` | `lib/crewai/src/crewai/a2a/config.py:660-683` |
| Protocol version literal | `ProtocolVersion = Literal["0.2.0", ..., "0.3.0", "0.4.0"]`; `A2AServerConfig.protocol_version` defaults `"0.3.0"` | `lib/crewai/src/crewai/a2a/types.py:38-48`, `lib/crewai/src/crewai/a2a/config.py:616-619` |
| A2A version error | `UNSUPPORTED_VERSION = -32009` + `UnsupportedVersionError(requested_version, supported_versions)` with JSON-RPC mapping | `lib/crewai/src/crewai/a2a/errors.py:70-71,114,300-313` |
| Transport negotiation | `negotiate_transport()` 3-step: client `preferred` → server `preferred` → `fallback` first-match; emits `A2ATransportNegotiatedEvent`, raises `TransportNegotiationError` | `lib/crewai/src/crewai/a2a/utils/transport.py:109-215` |
| Content-type negotiation | `negotiate_content_types()` with wildcard-aware `_mime_types_compatible`, skill-specific `input_modes`/`output_modes`, `strict` flag, emits `A2AContentTypeNegotiatedEvent` | `lib/crewai/src/crewai/a2a/utils/content_type.py:189-280` |
| Tracing version stamping | `crewai_version` added to trace metadata/batch + telemetry spans | `lib/crewai/src/crewai/events/listeners/tracing/trace_listener.py:814-926`, `lib/crewai/src/crewai/events/listeners/tracing/trace_batch_manager.py:33-51`, `lib/crewai/src/crewai/telemetry/telemetry.py:528-1315` |
| Skill version pinning | `_pin_skill_refs()` folds `skill_versions[*].version` onto bare `@org/name` refs to avoid silent upgrade on new skill publish | `lib/crewai/src/crewai/utilities/agent_utils.py:1270-1336` |
| Tool checkpoint compat | `format_description_for_llm()` strips composite prefix idempotently for tools deserialized from older checkpoints | `lib/crewai/src/crewai/tools/structured_tool.py:145-182` |
| Memory dimension break | Docs warn 1536-dim vs `text-embedding-3-large` mismatch requires `crewai reset-memories -m` or explicit embedder pin | `docs/v1.15.14/en/concepts/memory.mdx:521` |

## Answers to Dimension Questions

**1. Which APIs are stable, experimental, deprecated, or internal?**

- **Stable (treated, not formally versioned):** `Agent`, `Crew`, `Task`, `Flow` (non-conversational DSL), `Knowledge`, `LLM/BaseLLM`, `CheckpointConfig/RuntimeState`, `BaseTool/CrewStructuredTool` — all re-exported from `lib/crewai/src/crewai/__init__.py:8-21,187-205`. Stability is by convention; no `@stable` marker or API stability annotation exists. Version is the package version (`1.15.17`).
- **Experimental:** Everything under `crewai.experimental` — `AgentExecutor`/`CrewAgentExecutorFlow`, evaluation harness (`AgentEvaluator`, `ExperimentRunner`), conversational `Flow` mixin (`RouterConfig`, `ConversationState`) — explicitly documented as breakable without major bump (`lib/crewai/src/crewai/experimental/__init__.py:1`, `lib/crewai/src/crewai/experimental/conversational.py:1-3,45-51`). Also `experimental.agent_executor` is the recommended replacement for `CrewAgentExecutor` (`lib/crewai/src/crewai/agent/core.py:168`).
- **Deprecated (warnings, removal deferred):** `crewai.a2a.config.A2AConfig` → `A2AClientConfig`/`A2AServerConfig` (`lib/crewai/src/crewai/a2a/config.py:363-368` removal `v2.0.0`), `transport_protocol`/`supported_transports`/`use_client_preference` (`lib/crewai/src/crewai/a2a/config.py:303-322,447-461`), `A2AServerConfig.preferred_transport`/`signatures` (`lib/crewai/src/crewai/a2a/config.py:678-705` `v2.0.0`), `CrewAgentExecutor` (`lib/crewai/src/crewai/agent/core.py:165-172`), `function_calling_llm` (`lib/crewai/src/crewai/agent/core.py:235`), `allow_code_execution`/`code_execution_mode`/`reasoning` (`lib/crewai/src/crewai/agent/core.py:375-387`), `Task.max_retries` (`lib/crewai/src/crewai/task.py:576`), `agent.i18n` (`docs/v1.15.17/en/guides/advanced/customizing-prompts.mdx:165`), `inputs.id` for `@persist` flows (`docs/v1.15.17/en/guides/flows/inputs-id-deprecation.mdx:8`), CLI `crewai tool create`/`crewai flow kickoff` and flags `--n_iterations`, `--skip_provider` (`lib/cli/src/crewai_cli/cli.py:316-942`, `lib/cli/src/crewai_cli/utils.py:52-63`), `crewai.utilities.*` shims (`lib/crewai/src/crewai/utilities/version.py:13`, `lib/crewai/src/crewai/utilities/paths.py:16`).
- **Internal:** `crewai_core.version`/`lock_store`/`paths`/`printer`/`token_manager` utilities exposed only for workspace reuse, checkpoint listener internals (`_find_checkpoint`, `_SENTINEL`), event bus internals (`is_crewai_internal` marker `lib/crewai/src/crewai/events/event_listener.py:329`), flow `internal_routes` (`lib/crewai/src/crewai/experimental/conversational_mixin.py:194`). No explicit `internal` visibility annotation beyond docstrings and `PrivateAttr`.

No central stability matrix; callers must infer from `experimental/` prefix, `@deprecated` decorators, and deprecation-guide MDX.

**2. How are users warned before breaking changes?**

- **Code warnings:** SDK surfaces use `typing_extensions.deprecated` (`FutureWarning`) and `warnings.warn` (`DeprecationWarning`/`FutureWarning`) at import/validation time, e.g., `A2AConfig` deprecation (`lib/crewai/src/crewai/a2a/config.py:363`), transport field migrations (`lib/crewai/src/crewai/a2a/config.py:310-320`), and agent param warnings (`lib/crewai/src/crewai/agent/core.py:375`). CLI surfaces use `warn_deprecated()` yellow `click.secho` at command dispatch (`lib/cli/src/crewai_cli/utils.py:60`, `lib/cli/src/crewai_cli/cli.py:807`).
- **Version/banner hints:** `ConsoleFormatter._show_version_update_message_if_needed()` (`lib/crewai/src/crewai/events/utils/console_formatter.py:60`) prints an update panel when `is_newer_version_available()` is true (suppressed when `verbose=False` or `CI` is set), plus a `Yanked Version` panel with reason when `is_current_version_yanked()` is true. This is freshness/yank signaling, not pre-breakage notice.
- **Docs changelogs/migration guides:** Per-version `docs/v*/en/changelog.mdx` and topical guides (e.g., `inputs-id-deprecation.mdx`, `customizing-prompts.mdx`, `a2a-agent-delegation.mdx:35`). `AGENTS.md:10-12` freezes `docs/v*/` snapshots (`lib/crewai-core` docs snapshot script `scripts/docs/freeze_current_edge.py`). No formal deprecation schedule, `BREAKING CHANGE` footer, or enforced sunset period is codified; `A2AConfig` removal is the only stamped `v2.0.0` hint.
- **Gap:** No compile-time or CI lint that fails on usage of deprecated symbols, no semver-breaking-change label automation visible in `.github/workflows` (lint, tests, publish), and removal dates are sparse.

**3. Are old clients, plugins, traces, or persisted artifacts still usable?**

- **Old SDK clients:** Pinned via `crewai-core==1.15.17` / `crewai==1.15.17` (`lib/crewai/pyproject.toml:11`), but there is no compatibility matrix or LTS branch. Upgrading is all-or-nothing; minor/patch may contain behavioral changes (commitizen `feat=MINOR`). No shims for renamed top-level APIs beyond deprecated aliases.
- **Plugins / extensions:** A2A layer is the only plugin-like extension with handshake: server advertises `preferred_transport` + `additional_interfaces` and client negotiates (`lib/crewai/src/crewai/a2a/utils/transport.py:109-192`, `lib/crewai/src/crewai/a2a/utils/content_type.py:189-228`); unsupported version raises `UNSUPPORTED_VERSION` (`lib/crewai/src/crewai/a2a/errors.py:300`). For skills/tools shipped via registry, `_pin_skill_refs` (`lib/crewai/src/crewai/utilities/agent_utils.py:1270`) pins `@org/name@version` to avoid silent drift, but there is no tool-schema version negotiation or feature-flag discovery at runtime. Expired/added tool args have no formal schema evolution.
- **Traces / telemetry:** Each trace/span is stamped with `crewai_version` (`lib/crewai/src/crewai/events/listeners/tracing/trace_listener.py:814`, `lib/crewai/src/crewai/events/listeners/tracing/trace_batch_manager.py:43`, `lib/crewai-core/src/crewai_core/telemetry.py:306-431`) but there is no server-side trace schema migration; `first_time_trace_handler` simply echoes `original_metadata.get("crewai_version")` (`lib/crewai/src/crewai/events/listeners/tracing/first_time_trace_handler.py:97`). Old trace batches remain readable as JSON but newer fields are not backfilled.
- **Persisted checkpoints (saved state):** Usable with limits. `RuntimeState` persists `crewai_version`, `parent_id`, `branch`, `entities`, `event_record` (`lib/crewai/src/crewai/state/runtime.py:190-214`) and `_migrate` backfills `memory_kind`/`source_type` for checkpoints `<1.14.6` (`lib/crewai/src/crewai/state/runtime.py:115-175`). Missing version is treated as `0.0.0` with warning (`lib/crewai/src/crewai/state/runtime.py:106-107`) and `ValueError` is raised only for uninferrable knowledge source `source_type` (`lib/crewai/src/crewai/state/runtime.py:147-150`) requiring re-checkpoint after `1.14.6+`. Deeper schema changes (new discriminators, new `Flow.initial_state` marker) are handled ad-hoc. `lib/crewai/tests/test_checkpoint.py:212-236` verifies the single `0.1.0`→current path; broader history is untested.
- **Prompts / templates:** No versioned prompt contract; `prompt_file` overrides (`docs/v1.15.17/en/guides/advanced/customizing-prompts.mdx:165`) are file-based with no migration.

**4. Does compatibility rely on policy alone or executable tests?**

- **Executable tests exist but are narrow:** Version-cache / yank / banner logic is well tested (`lib/crewai/tests/cli/test_version.py:23-507`, `lib/crewai-core/tests/test_smoke.py:22`, `lib/crewai-core/tests/test_telemetry_deploy.py:173-180` patches `get_crewai_version`). Checkpoint lineage/version tests exercise `_migrate` and `from_checkpoint` chaining/forking/pruning (`lib/crewai/tests/test_checkpoint.py:191-803`).
- **Policy-dominant elsewhere:** Breaking-change staging, semver, and deprecation timelines are commit-message convention (`pyproject.toml:156-169`) and changelog prose, not enforced by CI (no API surface snapshot, no `cog`-breaking gate, no cross-version integration harness). A2A negotiation (`lib/crewai/src/crewai/a2a/utils/transport.py`, `content_type.py`) is implemented but has no dedicated unit tests discovered under `lib/crewai/tests` for those negotiation utilities (no evidence found in search). Skill pinning (`lib/crewai/src/crewai/utilities/agent_utils.py:1304`) has no companion backwards-compat test. In practice, compatibility relies on docs + warning shims + the single checkpoint migration path, not a comprehensive executable contract suite.

## Architectural Decisions

- **Monorepo, mono-version:** All workspace packages (`crewai`, `crewai-core`, `crewai-cli`, `crewai-tools`, `crewai-files`) share one `__version__` (`1.15.17`) and exact pins (`lib/crewai/pyproject.toml:11-12`). Simplifies upgrade but means any package bump is a breaking surface for all consumers; no independent API versioning per surface.
- **Core extraction to `crewai-core`:** Version, telemetry, paths, lock, and token logic moved to `crewai_core` and re-exported via shim (`lib/crewai/src/crewai/utilities/version.py:7`, `lib/crewai/src/crewai/plus_api.py:3`, `lib/crewai-core/src/crewai_core/version.py:1-6`). Enables CLI and SDK to share version freshness/yank logic without duplication.
- **Checkpoint as versioned envelope:** `RuntimeState` is a `RootModel` whose JSON envelope carries `crewai_version` (`lib/crewai/src/crewai/state/runtime.py:192`) and lineage (`parent_id`, `branch`, `checkpoint_id` via `PrivateAttr`). Deserialization is `model_validator(mode="wrap")` that calls `_migrate` before `handler(data["entities"])` (`lib/crewai/src/crewai/state/runtime.py:200-214`). Branch-aware providers (`JsonProvider`/`SqliteProvider` with `branch` subdirs and `parent_id` chaining) preserve lineage across forks.
- **Deprecation via Pydantic-native + warning shims:** Deprecated fields use `exclude=True, deprecated=True` plus `model_validator(mode="after")` that calls `warnings.warn` and `object.__setattr__` to migrate to the new shape (`lib/crewai/src/crewai/a2a/config.py:303-322,447-462,690-706`), preserving wire compatibility while nudging callers. CLI uses a parallel `warn_deprecated()` presentation layer (`lib/cli/src/crewai_cli/utils.py:52-63`).
- **A2A as capability-negotiated subsystem:** Transport and content-type negotiation are explicit, priority-ordered handshakes with event emission (`lib/crewai/src/crewai/a2a/utils/transport.py:35-45,109-213`, `lib/crewai/src/crewai/a2a/utils/content_type.py:42-56,189-280`, `lib/crewai/src/crewai/a2a/types.py:37-48`). Protocol version is a literal union (`ProtocolVersion`) with a dedicated error code (`lib/crewai/src/crewai/a2a/errors.py:70,300`). This contrasts with the rest of the SDK, which has no negotiation.

## Notable Patterns

- **Warning-layer duplication:** SDK warnings (`FutureWarning`/`DeprecationWarning` + `@deprecated`) vs CLI warnings (`click.secho` yellow) for the same logical deprecation (e.g., `crewai tool create` → `crewai create tool`). Two presentation channels, one compatibility intent.
- **Migration shim that preserves old field on the wire:** Deprecated `transport_protocol`/`supported_transports` are `exclude=True` but still read via validator and mapped onto `transport.preferred`/`supported` (`lib/crewai/src/crewai/a2a/config.py:429-438,543-549`), so old payloads deserialize but new serializations omit the legacy keys.
- **Idempotent tool description fixup:** `format_description_for_llm()` strips a previously composed `"Tool Name/Arguments/Description"` block before recomposing (`lib/crewai/src/crewai/tools/structured_tool.py:145-182`), making checkpoint-deserialized tools self-healing across prompt-format changes.
- **Cache-then-fetch version check:** `get_latest_version_from_pypi()` reads `~/.cache/crewai/version_cache.json` (via `appdirs.user_cache_dir`) and reuses it for 24h (`lib/crewai-core/src/crewai_core/version.py:42-58,112-146`), with graceful `URLError`/`JSONDecodeError` swallowing. Banner visibility is gated by `verbose` and `CI` (`lib/crewai/src/crewai/events/utils/console_formatter.py:68-78`).
- **Skill pinning at hydration time:** `_pin_skill_refs()` joins bare `@org/name` with `skill_versions[].version` at `Agent` creation (`lib/crewai/src/crewai/utilities/agent_utils.py:1270-1336`), an anti-drift pattern but implemented only for registry skills, not for local tools or LLM provider versions.

## Tradeoffs

- **Simplicity vs granularity:** One global version string avoids matrix testing but forces all surfaces to rev together; a fix in `crewai-cli` produces a new `crewai` version even if the SDK surface is unchanged.
- **Lenient migration vs strict failure:** `_migrate` treats missing `crewai_version` as `0.0.0` with a warning and backfills only what it can infer (`lib/crewai/src/crewai/state/runtime.py:106-150`); uninferrable knowledge sources raise `ValueError` requiring a fresh checkpoint. This favors forward progress over strict determinism, but leaves failed restores with no auto-upgrade path.
- **Warning verbosity vs discoverability:** `FutureWarning` is silent by default in Python, so SDK deprecations (`FutureWarning` in `lib/crewai/src/crewai/a2a/config.py:310`) may be invisible unless `-W` is enabled, while CLI `click.secho` warnings are always visible. Inconsistent signal strength.
- **Centralized version freshness vs offline hermeticity:** 24h PyPI poll + file cache works for long-lived developer machines but depends on network and `appdirs` cache state; failures are silent (`return None` in `lib/crewai-core/src/crewai_core/version.py:145-146`), so yank detection may lag 24h or be skipped offline.
- **Negotiation depth:** A2A negotiation is well specified, but adding a new `ProtocolVersion` or `TransportType` literal (`"HTTP+JSON"` with `+` is unusual in `Literal`) requires coordinated SDK+server upgrades; there is no version-range or capability advertisement beyond the enumerated literals.

## Failure Modes / Edge Cases

- **Checkpoint from future version:** `Version(raw)` compared to current (`lib/crewai/src/crewai/state/runtime.py:102-103`) only logs `DEBUG` migration for older versions; a checkpoint stamped `1.16.x` loaded on `1.15.17` skips migration entirely and may fail Pydantic validation on new discriminator/field, with no forward-compat handling.
- **Missing or corrupt `crewai_version`:** Treated as `0.0.0` with `logger.warning` (`lib/crewai/src/crewai/state/runtime.py:106-107`), then bulk backfill runs; non-string `content` knowledge sources cannot be backfilled and raise `ValueError: Legacy knowledge source ...` requiring manual re-checkpoint (`lib/crewai/src/crewai/state/runtime.py:147-150`).
- **Partial yank on PyPI:** `all(f.get("yanked",False) for f in files)` means a version with one yanked and one non-yanked wheel is considered not-yanked (`lib/crewai-core/src/crewai_core/version.py:80-81,100-101`); `test_partially_yanked_files_not_considered_yanked` confirms this (`lib/crewai/tests/cli/test_version.py:198-204`), which matches PyPI semantics but could hide a platform-specific yank.
- **Yank/banner cache staleness and suppression:** Cache lives at `~/.cache/crewai/version_cache.json` and is only refreshed every 24h (`lib/crewai-core/src/crewai_core/version.py:56`); `is_current_version_yanked()` will not surface a freshly yanked release for up to 24h, and the banner is fully suppressed when `CI=true|1` or `verbose=False` or `ContextVar(_disable_version_check)` is set (`lib/crewai/src/crewai/events/utils/console_formatter.py:26-78`), so CI deployments may run yanked code silently.
- **A2A negotiation failures:** `TransportNegotiationError` and `ContentTypeNegotiationError` (`lib/crewai/src/crewai/a2a/utils/transport.py:48-73`, `lib/crewai/src/crewai/a2a/utils/content_type.py:58-96`) surface only when no overlap is found; `negotiate_content_types(strict=False)` (default) silently returns empty `input_modes`/`output_modes` lists with `negotiation_success=False` instead of raising, which callers may ignore.
- **Skill pinning misses:** `_pin_skill_refs` only pins when `skill_versions` is present and the bare ref is not already `name@version` (`lib/crewai/src/crewai/utilities/agent_utils.py:1320-1336`); agents loaded without registry metadata or with mixed `name@version` and bare refs will partially pin, leading to inconsistent skill versions across a crew.
- **Memory/embedder dimension drift:** Upgrading from `text-embedding-ada-002`/1536-dim to `text-embedding-3-large` silently breaks local Chroma/lancedb stores; recovery is manual `crewai reset-memories -m` or explicit `embedder` pin (`docs/v1.15.14/en/concepts/memory.mdx:521`), with no automated migration.
- **Protocol version reach:** `ProtocolVersion` is closed literal (`"0.2.0"`..`"0.4.0"`, `lib/crewai/src/crewai/a2a/types.py:38-48`); a server advertising `"0.5.0"` fails validation before negotiation, yielding `UnsupportedVersionError` (`lib/crewai/src/crewai/a2a/errors.py:300`) with no graceful downgrade.

## Future Considerations

- Adopt a central stability matrix (e.g., `STABILITY.md` + module-level `__stability__` markers) distinguishing `stable` vs `experimental` vs `internal`, and enforce via `__all__` and linter so new `experimental` additions require opt-in import.
- Add semver enforcement in CI: API surface snapshot (e.g., `griffe`/`pydantic` schema exports) gated by `conventional-commits` breaking indicator; publish workflow (`\.github/workflows/publish.yml:40-42`) should reject `BREAKING` on patch bumps.
- Expand `RuntimeState._migrate()` beyond `<1.14.6` with a registered-migration pattern (version → callable) and property-based tests that round-trip checkpoints from each minor, including forward-check (future-version) and `branch`/`parent_id` invariants.
- Negotiate broader contracts: expose `protocol_version` and `TransportType` ranges (e.g., `supported_versions: list[ProtocolVersion]`) and capability advertisement for extensions, rather than a single `protocol_version` literal; align `ClientTransportConfig.preferred` + `supported` with server `preferred` + `additional_interfaces` symmetrically.
- Make deprecation warnings observable: unify SDK and CLI channels, emit `DeprecationWarning` with `stacklevel` and optionally a telemetry event, and add CI job that fails on deprecated-symbol usage outside `tests/`.
- Version the persisted tool/knowledge schemas explicitly (e.g., `schema_version` per `BaseTool`/`KnowledgeSource` union) so new discriminators can be added without inferring from structural fields; pair with schema snapshot tests.
- For embedder/memory, add dimension-aware storage header and auto-migration or clear error path instead of docs-only `reset-memories` guidance.

## Questions / Gaps

- No evidence found for a formal deprecation lifecycle (e.g., `deprecated → pending removal → removed` with sunset dates) beyond the single `v2.0.0` note for `A2AConfig` (`lib/crewai/src/crewai/a2a/config.py:365`). Searched `*.py` for `deprecated`, `FutureWarning`, `warn`, and `docs/*.mdx` for migration guides — only ad-hoc per-field notices.
- No evidence found for feature-flag or capability-flag enumeration outside A2A (e.g., no `capabilities`/`flags` field on `Crew`/`Agent`/`Task`). Grepped `crewai/src/crewai` for `feature_flag`, `capability`, `negotiat` — only A2A hits.
- No evidence found for backwards-compatibility tests covering old client → new server or old persisted `knowledge_sources`/`tools` schemas across more than one historical version. `lib/crewai/tests/test_checkpoint.py` covers only the `<1.14.6` backfill path and `0.1.0` stamp.
- No evidence found for extension/plugin versioning contract (e.g., `mcp`, `skills` registry, or `crewai-tools` tool-version manifest) with version negotiation beyond `skill_versions` pinning (`lib/crewai/src/crewai/utilities/agent_utils.py:1270`).
- No evidence found for an API changelog enforcement hook (e.g., PR label `breaking` or `changelog:required`) in `.github/workflows/*.yml`; `commitizen` `bump_map` exists but no workflow validates it.

---

Generated by `Dimension 24.03: API Versioning and Compatibility` against `crewai`.

# Source Analysis: agent-framework

## Dimension 21.03: Extension Compatibility Testing

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python + .NET (C#) / `agent-framework-core`, provider packages, `Microsoft.Agents.AI.*` |
| Analyzed | 2026-08-27 |

## Summary

Agent Framework defines multiple explicit extension contracts (`BaseChatClient`/`SupportsChatGetResponse`, `AgentMiddleware`/`ChatMiddleware`/`FunctionMiddleware`, `SkillsSource`/`Skill`/`SkillResource`/`SkillScript`, `ContextProvider`/`HistoryProvider`, `MCPTool`, workflow `Executor`) with coverage via large internal test suites and 8+ executable skill samples plus in-code docstring examples. However it provides **no public conformance test suite or exported test fixtures** that a third-party extension author can import to self-verify; fixtures (`MockChatClient`, `MockBaseChatClient`) are test-internal in `tests/core/conftest.py`. Stability is documented via semver, lifecycle stages (`released` vs `beta/alpha/rc`), `@experimental` warnings, and (for .NET) automated Package Validation with `CompatibilitySuppressions.xml`; breaking changes are communicated via `CHANGELOG.md` `[BREAKING]` tags and the `breaking change` PR label, but there is no formal deprecation policy or compatibility test harness.

## Rating

**Score: 5 / 10** — Present but inconsistent: strong interface definitions, extensive internal tests, and rich examples exist, but no externalized conformance harness/fixtures and stability guarantees are fragmented across semver docs, experimental warnings, and .NET-only automated validation. An extension author can infer the contract and copy examples but cannot run a single `verify_my_extension` suite.

Rationale: `BaseChatClient` (`python/packages/core/agent_framework/_clients.py:217`) and `SupportsChatGetResponse` (`_clients.py:85`) are well-typed Protocols/ABCs with overloads; `SkillsSource` hierarchy (`_skills.py:1393+`) is fully abstract and tested (~6100 lines in `test_skills.py`/`test_mcp_skills.py`); samples cover every skill flavor. Yet no `agent_framework.testing` fixtures or `conformance` marker/kit is published, and stability relies on per-package `PACKAGE_STATUS.md` lifecycle plus ad-hoc `CHANGELOG` notes rather than a unified compatibility matrix or hosted contract tests.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Extension contract: Chat client (Protocol) | `SupportsChatGetResponse` Protocol with 3 overloads of `get_response(messages, stream, options, compaction_strategy, ...)` + `additional_properties`; structural typing via `@runtime_checkable` | `python/packages/core/agent_framework/_clients.py:85-200` |
| Extension contract: Chat client (ABC) | `BaseChatClient` ABC requiring `_inner_get_response(messages, stream, options, **kwargs)`; generic `OptionsCoT`, `STORES_BY_DEFAULT`, `to_dict` override; docstring shows `class CustomChatClient(BaseChatClient): async def _inner_get_response(...)` | `python/packages/core/agent_framework/_clients.py:217-269`, `python/packages/core/agent_framework/_clients.py:413-439` |
| Extension contract: Custom ChatClient example | Docstring example for implementing `BaseChatClient` and `SupportsChatGetResponse` (structural) with `isinstance(client, SupportsChatGetResponse)` check | `python/packages/core/agent_framework/_clients.py:239-268`, `python/packages/core/agent_framework/_clients.py:100-128` |
| Extension contract: Skills | `Skill` ABC, `SkillResource`/`SkillScript` ABCs, `InlineSkill`/`ClassSkill`/`FileSkill`, `SkillsSource` ABC with `get_skills(context: SkillsSourceContext)`, decorators `Aggregating/Caching/Filtering/Deduplicating` + `SkillFrontmatter` validation | `python/packages/core/agent_framework/_skills.py:106-605`, `python/packages/core/agent_framework/_skills.py:606-665`, `python/packages/core/agent_framework/_skills.py:820-1133` |
| Extension contract: Skills custom extension guide | Full 5-step custom extension tutorial: `CloudSkillResource`, `CloudSkillScript`, `CloudSkill`, `CloudSkillsSource` + builder registration `UseSource(new CloudSkillsSource(...))` | `docs/decisions/0021-agent-skills-design.md:524-732` |
| Extension contract: Middleware | `AgentMiddleware`, `ChatMiddleware`, `FunctionMiddleware` + `AgentContext`/`ChatContext`/`FunctionInvocationContext` pipelines; `call_next()` no-args signature | `python/packages/core/agent_framework/_middleware.py` (via `AGENTS.md` reference) and `python/packages/core/tests/core/test_middleware.py:36-178` |
| Extension contract: Custom History/Context provider | `HistoryProvider`, `ContextProvider`, `SessionStore`/`FileSessionStore` abstract surfaces; sample `custom_history_provider.py` demonstrates external impl (referenced in `python/samples/02-agents/conversations/`) | `python/packages/core/agent_framework/_sessions.py:1514-1680` (via grep), `python/samples/02-agents/conversations/custom_history_provider.py:17` |
| Conformance tests (internal) – Chat client | `MockChatClient` (duck-type impl) and `MockBaseChatClient(FunctionInvocationLayer, ChatMiddlewareLayer, ChatTelemetryLayer, BaseChatClient)` with `_inner_get_response` override used across ~30 test files; validates both Protocol and ABC paths | `python/packages/core/tests/core/conftest.py:108-290` |
| Conformance tests (internal) – Skills | 6500+ lines: `TestDiscoverResourceFiles`, `TestSymlinkDetection`, `TestSkillsProvider`, `TestSkillsProviderCodeSkill`, `TestInlineSkill`, `TestClassSkill`; covers file discovery, symlink/junction guards, deduplication, sorting, XML escaping, progressive disclosure | `python/packages/core/tests/core/test_skills.py:1-35`, `python/packages/core/tests/core/test_skills.py:279-898`, `python/packages/core/tests/core/test_mcp_skills.py:1-145` |
| Conformance tests (internal) – Middleware/Workflows | `TestAgentMiddlewarePipeline`, `TestChatMiddlewarePipeline`, `TestFunctionMiddlewarePipeline` (execution order, termination, streaming); `tests/workflow/*` validates `WorkflowBuilder`, `TypeCompatibilityError`, `WorkflowValidationError` | `python/packages/core/tests/core/test_middleware.py:135-1103`, `python/packages/core/tests/workflow/test_workflow_builder.py:21-369`, `python/packages/core/tests/workflow/test_validation.py:12-105` |
| Test fixtures (internal, not exported) | `chat_history`, `ai_tool`, `tool_tool`, `client`, `chat_client_base`, `agent`, `agent_session`, `create_junction_or_skip` fixtures; internal `MockAgent`/`MockAgentSession` helpers | `python/packages/core/tests/core/conftest.py:75-356`, `python/packages/core/tests/conftest.py:15-97` |
| Extension fixtures gap | No `agent_framework.testing` or `agent_framework.testkit` package; `python/packages/core/agent_framework/__init__.py:47` exports `BaseChatClient` but not test helpers; `pyproject.toml:26-31` core dependencies do not expose test fixtures | `python/packages/core/agent_framework/__init__.py:47`, `python/packages/core/pyproject.toml:25-31` |
| Example implementations – Skills | 8 runnable samples: `file_based_skill.py` (`SkillsProvider.from_paths(..., script_runner=subprocess_script_runner)`), `code_defined_skill.py` (`InlineSkill` + `@skill.resource`/`@skill.script`), `class_based_skill.py` (`ClassSkill` + `@ClassSkill.resource/script`), `mcp_based_skill`, `mixed_skills`, `skill_filtering`, `skills_auto_approval` | `python/samples/02-agents/skills/file_based_skill/file_based_skill.py:41-70`, `python/samples/02-agents/skills/code_defined_skill/code_defined_skill.py:51-156`, `python/samples/02-agents/skills/class_based_skill/class_based_skill.py:37-124`, `python/samples/02-agents/skills/README.md:9-18` |
| Example implementations – README index | `samples/README.md` catalog + per-package sample env inventory; `samples/02-agents/skills/README.md:1-66` progressive disclosure table (Advertise → Load → Read → Run) with File vs Code vs Class comparison | `python/samples/README.md:3-36`, `python/samples/02-agents/skills/README.md:15-52` |
| Stability guarantees – Semver | `CHANGELOG.md` header declares Keep a Changelog + Semantic Versioning; entries tag `[BREAKING]` and `[BREAKING — experimental/beta]` with PR links | `python/CHANGELOG.md:1-26`, `python/CHANGELOG.md:20-60` |
| Stability guarantees – Package lifecycle | `PACKAGE_STATUS.md` buckets `alpha`/`beta`/`rc`/`released`/`deprecated`; `released` = "should not have breaking changes between versions"; 35 packages inventoried | `python/PACKAGE_STATUS.md:4-52` |
| Stability guarantees – Feature stages | `ExperimentalFeature` enum (14 IDs incl `MCP_SKILLS`, `HARNESS`, `AGENT_HOOKS`), `experimental` decorator emits one-time `ExperimentalWarning` with custom formatter; `release_candidate` notes; `SKILLS` warning filter in tests `warnings.filterwarnings("ignore", message=r"\[SKILLS\].*")` shows experimental scoping | `python/packages/core/agent_framework/_feature_stage.py:43-86`, `python/packages/core/tests/core/conftest.py:19-23`, `python/packages/core/agent_framework/_skills.py:68` |
| Stability guarantees – .NET automated validation | `CONTRIBUTING.md:77-104` Automated API Compatibility Validation via Package Validation (`dotnet build -c Release` + baseline `PackageValidationBaselineVersion` in `dotnet/nuget/nuget-package.props`); suppression via `CompatibilitySuppressions.xml` (`dotnet build ... /p:ApiCompatGenerateSuppressionFile=true`) | `CONTRIBUTING.md:71-104` |
| Breaking change policy – PR process | PR template requires `[x] This is not a breaking change.` else `breaking change` label or `[BREAKING]` title prefix; `label-pr.yml` auto-syncs label from title; `title_prefix.js` defines `BREAKING_CHANGE_LABEL` | `.github/pull_request_template.md:43`, `.github/workflows/label-pr.yml:53`, `.github/scripts/title_prefix.js:3` |
| Breaking change communication – Examples | Repeated `[BREAKING — experimental] Require functional workflow definitions to be built...` and `[BREAKING] Make workflow checkpoints fully replayable` entries demonstrate policy in practice (experimental breaks allowed, stable breaks flagged) | `python/CHANGELOG.md:58-59`, `python/CHANGELOG.md:104` |

## Answers to Dimension Questions

**1. Are extension contracts tested?**
Partially — internally yes, as conformance-like suites but not as public author-facing harness.
* Positive: `BaseChatClient`/`SupportsChatGetResponse` exercised via `MockBaseChatClient` across `test_middleware_with_agent.py`, `test_harness_tool_approval.py`, `test_harness_loop.py`; `SkillsSource` hierarchy via `Aggregating/Caching/Filtering/Deduplicating` tested with context-aware `SkillsSourceContext` (`python/packages/core/tests/core/test_skills.py:81-91`, `test_mcp_skills.py:145`); middleware pipelines, workflow validation, and MCP task options all have dedicated suites.
* Gap: No exported `pytest` marker or helper module (e.g., `agent_framework.testing.assert_chat_client_conforms`) that an external author could `pip install` and run against their implementation. Conformance is verified implicitly by the framework's own integration tests, not by a standalone spec test suite.

**2. Are fixtures provided for extension authors?**
No public fixtures; internal fixtures only and must be copied.
* `python/packages/core/tests/core/conftest.py:75-356` defines `MockChatClient` (protocol) and `MockBaseChatClient` (full layered stack) and `MockAgent`/`MockAgentSession` — but they live under `tests/` and are not re-exported via `agent_framework` or a `testing` extra. `python/packages/core/tests/conftest.py:15-27` adds `chat_history`-scoped fixtures similarly internal.
* The `.NET` repo does not publish test helpers either; `dotnet/tests/` is isolated.
* Authors must replicate the patterns shown in `_clients.py:105-128` docstrings or sample custom clients rather than importing a test kit.

**3. Are examples provided?**
Yes — rich and multi-idiom.
* File-based: `python/samples/02-agents/skills/file_based_skill/file_based_skill.py:41-70` with `SKILL.md` + `subprocess_script_runner`.
* Code-defined: `python/samples/02-agents/skills/code_defined_skill/code_defined_skill.py:51-135` with `InlineSkill` + `@skill.resource`/`@skill.script`.
* Class-based: `python/samples/02-agents/skills/class_based_skill/class_based_skill.py:37-124` with `ClassSkill`.
* Mixed/MCP/Filtering/Auto-approval plus `docs/decisions/0021-agent-skills-design.md:536-732` cloud-source walkthrough provide copy-pasteable custom extension templates. Each sample declares precise `pip install agent-framework-*` deps via PEP 723 metadata.

**4. Are stability guarantees documented?**
Fragmented but present.
* Versioning: `python/CHANGELOG.md:6` semver claim + `python/PACKAGE_STATUS.md:9-10` definition of `released` (no breaking changes) vs `beta/rc` (may still break). `_feature_stage.py:43-66` documents experimental IDs and one-time warnings (`_clients.py`/`_skills.py` experimental marks).
* .NET stability: `CONTRIBUTING.md:77-104` package-validation baseline + `dotnet/nuget/nuget-package.props` baseline version; `Directory.Packages.props` pins major dependencies.
* Policy enforcement: `CONTRIBUTING.md:71-75` "Contributions must maintain API signature and behavioral compatibility. Contributions that include breaking changes will be rejected." plus PR template/label automation.
* Gaps: No unified stability matrix linking Python feature stages to .NET `IsReleaseCandidate`/`IsGenerallyAvailable`; no deprecation timeline policy (e.g., N-1 support, `@deprecated` migration guide); breaking changes for experimental features occur frequently and are only noted ad-hoc in `CHANGELOG`.

## Architectural Decisions

* **Dual contract model (ABC + Protocol)** — `BaseChatClient` ABC for subclassing (`_clients.py:217`) vs `SupportsChatGetResponse` `@runtime_checkable` Protocol for duck-typing (`_clients.py:85`) enables both typed and dynamic clients but splits conformance surface; tests must cover both paths via `MockBaseChatClient` vs `MockChatClient`.
* **Decorator-based skill composition** — `SkillsSource` + `Aggregating/Filtering/Caching/Deduplicating` decorators (`_skills.py:36-38` and `docs/decisions/0021-agent-skills-design.md:91-100`) trades per-source caching control for verbosity; `SkillsProvider` auto-wraps only built-in leaf sources in `Caching+Deduplicating` but leaves caller-supplied sources unwrapped to avoid cross-tenant leakage (documented in `python/packages/core/AGENTS.md` skills section).
* **File-based skill trust boundary** — `FileSkillsSource` defines trust at configured root paths (`python/packages/core/AGENTS.md:FileSkillsSource` notes + `test_skills.py:1063-1075` "configured root may itself be a link") while every segment below is link-checked via `is_link_or_reparse_point` with fail-closed `OSError → skip`. Hardened archive extraction via `_normalize_archive_member_name` (zip-slip guard) mirrors the same principle for `MCPSkillsSource`.
* **Feature-stage warning dedup** — `_feature_stage.py:236-263` `_WARNED_FEATURES` set + `_resolve_user_frame()` ensures one-time per-process `ExperimentalWarning` pointing at caller frame, not internals; `HARNESS` dedup key shared across three providers prevents duplicate warnings when `create_harness_agent` wires multiple experimental providers.
* **.NET Package Validation vs Python semver** — .NET opts into release-time breaking-change detection against NuGet baseline; Python relies on changelog discipline + runtime `ExperimentalWarning`. The asymmetry means cross-language parity of stability guarantees is not centrally attested.

## Notable Patterns

* **Protocol + ABC layering** — Every provider contract exposes both a `Protocol` (for `isinstance` checks without inheritance) and an `ABC` with `Generic[OptionsCoT]` typing and `additional_properties` dict for forward-compat extensions (`_clients.py:62,85,217`). Overloads capture `ChatOptions[ResponseModelBoundT]` to preserve typed `response_format`.
* **Lazy public surface** — `python/packages/core/agent_framework/__init__.py:47` lazy `__getattr__` + `__init__.pyi` keeps extension contracts import-light; `_LAZY_MODULE_EXPORTS` must stay synced with `__all__` per `python/packages/core/AGENTS.md:Root Public API`.
* **Skills progressive disclosure** — `load_skill` → `read_skill_resource` → `run_skill_script` tool trio with `approval_mode="always_require"` default and static auto-approval rules `SkillsProvider.read_only_tools_auto_approval_rule` / `all_tools_auto_approval_rule` (server_label scoped). Pattern prevents unconstrained file access while allowing unattended samples.
* **Arg-shape disambiguation** — File scripts advertise `{"type":"array","items":{"type":"string"}}` (`_skills.py:506-512`) while inline scripts generate JSON Schema from signature via `FunctionTool` (`_skills.py:399-404`) plus optional `SkillScriptArgumentParser` for vLLM string-arg backends. Instruction prompt explicitly distinguishes the two shapes (`test_skills.py:685-688`).
* **In-memory vs disk isolation divergence** — `MCPSkillsSource` archive handling unpacks entirely in memory (`_skills.py:50-58` description) vs .NET extracting to disk; Python deliberately avoids temp-dir leak/prune footgun at cost of memory pressure (bounded via `archive_max_file_count`/`archive_max_uncompressed_size_bytes`).

## Tradeoffs

* **Fail-closed link checks vs usability** — Every filesystem segment is `OSError`-aware and rejected as unsafe (`test_skills.py:1121-1147`), which hardens against symlink/junction escape but forces authors to avoid symlinked skill layouts even when they control the root; the "configured root may be a link" exception is subtle and easily missed.
* **In-memory MCP archives vs temp-dir** — Eliminates disk leakage and Windows `prune-of-unowned-subdirs` risk, but caps archive size/count and requires callers to size `archive_max_*` correctly; large skills can OOM where .NET would spill to disk.
* **Decorator composition vs provider magic** — Explicit `Caching/Filtering/Deduplicating` decorators give fine-grained control and testability (`test_skills.py:81-91` `_CountingSkillsSource`) but increase boilerplate versus `AgentSkillsProviderBuilder` that hides wiring; consumers mixing both can double-cache or miss deduplication.
* **Experimental velocity vs stability** — Marking `MCP_SKILLS`, `HARNESS`, `SESSION_STORE` etc as experimental (`_feature_stage.py:43-66`) lets the team iterate, but `CHANGELOG.md:58-60` shows frequent `[BREAKING — experimental]` churn; extension authors on experimental surfaces must pin via git SHA or expect breakage — no LTS branch is offered.
* **No public test kit reduces maintenance but shifts cost** — Keeping `MockChatClient` internal avoids publishing/supporting a test API, yet forces every third-party provider package (`agent-framework-anthropic`, `-openai`, `-foundry`, etc.) to reimplement similar mocks, increasing drift risk and making cross-provider parity harder to audit.

## Failure Modes / Edge Cases

* **Missing conformance harness → silent breaking** — Without an exported conformance suite, a custom `SkillsSource` that omits `context: SkillsSourceContext` or returns wrong `Skill` types passes import but fails at `await SkillsProvider.before_run(...)` with opaque `TypeError` rather than a clear "contract violation" diagnostic. No `mypy --strict` plugin enforces the `SkillsSource` ABC across packages aside from `pyrightconfig.dependency.json` isolated checks.
* **Symlink-planted resource bypass attempt** — Test `TestSymlinkDetection.test_discover_skips_symlinked_resource` (`test_skills.py:957-980`) and junction guard `test_junction_is_detected_and_excluded` (`test_skills.py:1080-1098`) show the framework correctly skips leaked resources, but an author who disables `search_depth` or uses `resource_filter` to re-allow symlinked paths could reintroduce escape if they misunderstand the trust boundary.
* **Arg-parser misconfiguration** — `InlineSkillScript.run` raises `TypeError` if `args` arrives as `str` without `argument_parser` (`_skills.py:434-441`) or as `list` for inline scripts (`_skills.py:442-447`); vLLM users who forget `argument_parser=` get runtime errors only during LLM tool calls, not at provider construction.
* **Cache staleness across tenants** — Caller-supplied `SkillsSource` is never auto-wrapped in `CachingSkillsSource` (`AGENTS.md` skills section) precisely to avoid leaking skills across `agent`/`session` contexts; an author who manually wraps with no `cache_isolation_key_selector` and shares one provider across tenants will silently serve stale cross-tenant skills. `refresh_interval` staleness is measured with `time.monotonic()`, so system clock jumps do not affect expiry but container pause/suspend does (monotonic pauses).
* **Experimental promotion without codemod** — When `HARNESS` or `MCP_SKILLS` graduates from experimental to stable, the `__feature_stage__` metadata disappears and `ExperimentalWarning` stops; code that suppressed warnings via `warnings.filterwarnings("ignore", message=r"\[SKILLS\].*")` (`tests/core/conftest.py:19`) becomes dead configuration with no migration lint.
* **Breaking change suppression drift** — .NET `CompatibilitySuppressions.xml` is deleted after each release per `CONTRIBUTING.md:101` and baseline bumped in `nuget-package.props`. If a Python `released` package ships a break without a suppression file to track, the only signal is a manual `[BREAKING]` note in `CHANGELOG.md`; `uv`/`pip` dependency bounds (`pyproject.toml:26-31`) may still resolve the breaking version unless the author pins `agent-framework-core>=X`.

## Future Considerations

* **Publish a public conformance kit** — Export `MockChatClient`/`MockBaseChatClient` equivalents plus `assert_skills_source_conforms(source)` and `assert_chat_client_conforms(client)` helpers from a new `agent_framework.testing` module, with a `pytest` plugin that runs a curated matrix (non-streaming, streaming, tool_choice=none, compaction, file resources) against any `SupportsChatGetResponse` or `SkillsSource`. Port the existing internal suites (`test_skills.py`, `test_middleware.py`) as the seed.
* **Add Python package validation parity** — Introduce `shared_tasks.toml` or `python/scripts/validate_api_compat.py` that diffs public `__all__` / `__init__.pyi` against a baseline (stored in `python/.api-baseline/`) during CI, mirroring .NET Package Validation, and require `ApiCompatSuppression.json` for intentional breaks.
* **Stabilize and document lifecycle → semver mapping** — Extend `PACKAGE_STATUS.md` with per-stage promises (e.g., `beta`: MINOR may break, `rc`: PATCH only, `released`: no breaking without MAJOR) and a deprecation policy (annotation `@deprecated` → 1 MINOR warning → removal in next MAJOR, with `PendingDeprecationWarning`).
* **Provide fixture extra** — Ship `agent-framework-core[testing]` or standalone `agent-framework-testkit` that re-exports `_CountingSkillsSource`, `MockAgent`, and the `_SOURCE_CTX` helper (`test_skills.py:67-74`) so extension authors can write `from agent_framework.testing import make_source_context, CountingSkillsSource`.
* **Cross-language conformance matrix** — Publish a table in `docs/specs/` mapping each extension point (ChatClient, SkillsSource, HistoryProvider, MCPTool, Workflow Executor) to its Python and .NET parity status and link to the corresponding ADR (e.g., `0021-agent-skills-design`) so divergent behaviors (like archive in-memory vs disk) are discoverable before porting.
* **Versioned archive/extension negotiation** — Add `FeatureIndex`/`mark_feature_used` (`_skills.py:71`) telemetry to skills + `User-Agent` bitmask (see `docs/specs/004-feature-usage-telemetry.md`) to detect extension contract version mismatches in production and trigger graceful fallback.

## Questions / Gaps

* **No public `agent_framework.testing` kit found** — Grep across `python/packages/core/agent_framework/__init__.py` and `pyproject.toml` confirms test helpers are internal; no `py.typed` testing stub exists. Confirm whether the Durable Agent Framework extension (`agent-framework-durable-extension`) publishes its own harness fixtures that could serve as reference.
* **No semver enforcement tooling for Python** — Unlike .NET's `EnablePackageValidation`, Python CI scripts (`python/scripts/`) focus on sample validation and dependency bounds (`python/scripts/dependencies/validate_dependency_bounds.py`) but not API surface diffing; intention unclear whether Python stability is enforced by release skill (`python/.github/skills/agent-framework-py-release/SKILL.md:33,246`) alone.
* **Coverage of `allow_preview` vs stable tool paths** — Foundry provider's `ALLOW_PREVIEW` feature flag interactions with chat client options are marked experimental but not exercised in the public sample matrix; unclear how extension authors should handle preview-only tool schemas without breaking stable clients.
* **Decommissioned sub-packages** — `PACKAGE_STATUS.md:56-58` notes `agent-framework-azure-ai` deprecation but does not specify migration path stability for its exported symbols; verify whether a `DeprecationWarning` with removal version is emitted or if symbols were hard-removed.
* **MCP skills stability communication** — `MCP_SKILLS` remains experimental per `PACKAGE_STATUS.md:131` and `samples/02-agents/skills/README.md:61-66`, yet `test_mcp_skills.py:145` asserts experimental status; no ADR states the graduation criteria or timeline for MCP skills to become `released`.

---

Generated by `Dimension 21.03: Extension Compatibility Testing` against `agent-framework`.

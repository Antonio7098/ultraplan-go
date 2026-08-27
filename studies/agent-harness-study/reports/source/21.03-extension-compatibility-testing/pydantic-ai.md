# Source Analysis: pydantic-ai

## Dimension 21.03: Extension Compatibility Testing

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python 3.10+ / uv workspace (pydantic-ai-slim, pydantic-graph, pydantic-evals, clai) |
| Analyzed | 2026-08-27 |

## Summary

Pydantic AI treats extensibility as a first-class product surface: `AbstractCapability` (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:162`), `AbstractToolset` (`pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:76`), `Model` (`pydantic_ai_slim/pydantic_ai/models/__init__.py:366`) and `Provider` (`pydantic_ai_slim/pydantic_ai/providers/__init__.py:42`) are documented extension contracts with guides under `docs/capabilities/custom.md` and `docs/extensibility.md`. Operational stability is mature: `docs/version-policy.md:1` defines semver guarantees, `docs/changelog.md:93` enumerates V2 breaking changes, and CI gates public-API compatibility via `griffe` in `.github/scripts/check_api_compatibility.py:1` invoked from `.github/workflows/ci.yml:142`. What is absent is a conformance harness for extension authors themselves — no `conformance_test`, `extension_test_fixture`, or waivered contract test exists for custom capabilities, toolsets, models or providers. Testing instead relies on the built-in doubles `TestModel`/`FunctionModel` (`pydantic_ai_slim/pydantic_ai/models/test.py:62`, `pydantic_ai_slim/pydantic_ai/models/function.py:52`) and narrative docs; there is no importable fixture package or CLI (`pydantic ai test my-extension`) that validates an out-of-tree implementation against the contract, nor machine-readable stability guarantees per extension point.

## Rating

**Score: 6 / 10 — Present but inconsistent, weakly documented, or fragile**

Pydantic AI has explicit, typed extension interfaces and mature release-process safeguards (version policy, changelog, automated griffe diff with `.github/api-compatibility-allowlist.json:1` waivers, pre-commit/typecheck), plus extensive runnable doc examples. However extension *compatibility testing* as asked by this dimension — conformance suites, fixtures, and verifiable stability per contract that an external author can run — is incomplete: contracts are defined via ABCs but not pinned by dedicated conformance tests; fixtures are internal test helpers, not a published extension-test kit; and stability beyond the generic semver pledge is not per-interface documented. An extension author can copy examples but cannot run `pytest --extension-conformance` to prove correctness.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Conformance test suites — public API diff gate | `run_griffe(package, search_path, against)` shelling `griffecli check --against <tag>` and `parse_findings` fingerprinting | `.github/scripts/check_api_compatibility.py:106` |
| Conformance test suites — CI invocation on every PR | `Check public API compatibility with the latest release` job that fetches prior stable tag and runs `check_api_compatibility.py --against "$release_tag"` | `.github/workflows/ci.yml:142` |
| Conformance test suites — regression for the gate itself | `test_check_api_compatibility.py` covers waiver load, finding parse, emit | `.github/scripts/test_check_api_compatibility.py:6` |
| Extension test fixtures — absence of dedicated suite | `grep -r conformance|extension.*fixture` across `pydantic_ai_slim/` and `tests/` returns no extension conformance harness; only generic fixtures | `tests/conftest.py:476` (create_module helper is generic, not extension-scoped) |
| Capability contract definition | `class AbstractCapability(ABC, Generic[AgentDepsT])` with ~30 hooks (`get_toolset`, `get_wrapper_toolset`, `wrap_run`, `before_model_request`, etc.) and dataclass metadata `id/description/defer_loading` | `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:162` |
| Toolset contract definition | `class AbstractToolset(ABC, Generic[AgentDepsT])` with abstract `id`, `get_tools`, `call_tool` and helpers `filtered/prefixed/prepared` | `pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:76` |
| Model contract definition | `class Model(AbstractModel, Generic[InterfaceClient])` with abstract `request`/`request_stream`, profile resolution, `prepare_request`, `supported_tool_deferral_modes` | `pydantic_ai_slim/pydantic_ai/models/__init__.py:366` |
| Provider contract definition | `class Provider(ABC, Generic[InterfaceClient])` with abstract `name/base_url/client` and `infer_provider_class` registry | `pydantic_ai_slim/pydantic_ai/providers/__init__.py:42` |
| Wrapper/delegation pattern for extensions | `class WrapperModel(Model)` delegates `request`, `request_stream`, `profile`, etc. via `__getattr__` — idiomatic base for custom models | `pydantic_ai_slim/pydantic_ai/models/wrapper.py:32` |
| Wrapper toolset pattern | `WrapperToolset` subclasses override `call_tool` to intercept execution; docs recommend extending it for cross-cutting behavior | `pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:76` via `docs/toolsets.md:590` and `docs/extensibility.md:62` |
| Extension test fixtures — TestModel double | `class TestModel(Model)` with `supported_tool_deferral_modes`/`supported_tool_addition_modes`, seedable `_JsonSchemaTestData.generate()` for tool args, `last_model_request_parameters` capture | `pydantic_ai_slim/pydantic_ai/models/test.py:62` |
| Extension test fixtures — FunctionModel double | `class FunctionModel(Model)` wrapping user `function: AgentInfo => ModelResponse` with parallel `FunctionStreamedResponse` | `pydantic_ai_slim/pydantic_ai/models/function.py:52` |
| Extension test fixtures — FunctionToolset builder | `FunctionToolset` with `tool`/`tool_plain` decorators and `toolsets=[FunctionToolset([...])]` pattern pervasive in tests | `tests/conftest.py:476` and `tests/test_agent.py:9331` (`foo_toolset = FunctionToolset()`) |
| Example implementations — custom capability | 4 runnable snippets: plain class, dataclass, custom `__init__`, typed deps + `before_model_request`; `get_toolset` returning `FunctionToolset`, `get_wrapper_toolset` with `WrapperToolset` subclass | `docs/capabilities/custom.md:7` |
| Example implementations — custom toolset | `Building a Custom Toolset` requires implementing `get_tools()` + `call_tool()` and optionally `get_instructions()`; lifecycle hooks `for_run`/`for_run_step`/`__aenter__` documented | `docs/toolsets.md:861` |
| Example implementations — custom model | `To implement support ... subclass Model` plus pointer to `openai.py` reference impl; `WrapperModel` for wrapping fallback | `docs/models/overview.md:114` |
| Example implementations — third-party ecosystem | Lists `SkillsToolset`, `TodoToolset`, `FileSystemToolset`, `LangChainToolset` as community toolsets; capability publishing via `get_serialization_name`/`from_spec` | `docs/extensibility.md:22` and `docs/toolsets.md:883` |
| Stability documentation — version policy | Semver since V1 Sep 2025 / V2 2026-06-23; no intentional breaking changes in minor releases; deprecations survive until next major (>=3 months); explicit list of non-breaking changes (message parts, span attributes) | `docs/version-policy.md:1` |
| Stability documentation — beta caveat | `minor releases may introduce beta features (beta module) ... API/behaviors may not be stable` | `docs/version-policy.md:22` |
| Stability documentation — upgrade guide | Enumerates V2.0 breaking groups (providers Grok→xAI, Google provider rename, `ModelProfile` dataclass→TypedDict, `Agent` kwargs→capabilities etc.) with before→after tables | `docs/changelog.md:93` |
| Breaking change policy — allowlist mechanism | `Waiver(against, fingerprint=sha256, reason, pull_request)` and `Allowlist(allowed_breakages)` validated via Pydantic; `PACKAGES = {pydantic_ai: pydantic_ai_slim, ...}` | `.github/scripts/check_api_compatibility.py:14` |
| Breaking change policy — empty allowlist current | `{"allowed_breakages": []}` — no waived breakages active at HEAD; `::error` emitted per `emit_annotation` on unwaived finding; `::warning` on waived | `.github/api-compatibility-allowlist.json:1` and `.github/scripts/check_api_compatibility.py:71` |
| Interface stability enforcement — kw-only dataclasses | `test_new_public_dataclasses_are_keyword_only()` + `_KW_ONLY_ALLOWLIST` grandfathering; walker runs out-of-process via `kw_only_walker.py` | `tests/test_public_interface_contracts.py:40` |
| Interface stability enforcement — agent wrapper parity | `test_agent_implementation_signature_parity` and `test_agent_implementation_forwarding_parity` guard `AbstractAgent/WrapperAgent/TemporalAgent/DBOSAgent/PrefectAgent` `run/run_sync/run_stream/iter/override` signatures | `tests/test_public_interface_contracts.py:196` |
| Spec-based extension registration | `custom_capability_types` on `Agent.from_spec`/`Agent.from_file` plus package naming `pydantic-ai-*` | `docs/extensibility.md:31` |
| Capability ordering stability | `CapabilityOrdering(position, wraps, wrapped_by, requires)` + topological sort in `CombinedCapability`; `get_ordering()` hooks | `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:116` |

## Answers to Dimension Questions

### 1. Are extension contracts tested?

**Partially, but not as a conformance harness for external authors.**

- **What is tested:** The repository enforces *its own* public API stability via `griffecli` diff in CI (`pydantic_ai_slim/.github/scripts/check_api_compatibility.py:106`, `.github/workflows/ci.yml:142`) with a waivered allowlist (`pydantic_ai_slim/.github/api-compatibility-allowlist.json:1`). Interface-shape invariants for the agent surface are pinned by meta-tests (`tests/test_public_interface_contracts.py:102`, `:197`, `:319`). Every extension point is an `ABC` with `abstractmethod` so Python will `TypeError` on missing `request`/`get_tools`/`call_tool` implementations.
- **What is not tested:** No dedicated `tests/test_*_conformance.py` suite iterates over a `CustomModel`/`CustomToolset`/`CustomCapability` fixture and asserts lifecycle, serialization, or defer-loading behavior. The contracts are exercised indirectly through VCR + `TestModel` agent tests (`tests/test_agent.py:9331` etc.) rather than via a contract-driven property suite. Verdict: **No conformance test suite extension authors can execute**.

### 2. Are fixtures provided for extension authors?

**Ad-hoc, internal-only — not a published fixture library.**

- `pydantic_ai_slim/pydantic_ai/models/test.py:62` (`TestModel`) and `pydantic_ai_slim/pydantic_ai/models/function.py:52` (`FunctionModel`) are the de-facto fixtures: documented in `docs/testing.md:195` and reused across every docs example to capture `last_model_request_parameters`. `FunctionToolset` (`tests/test_agent.py:9331`) and `WrapperModel`/`WrapperToolset` (`pydantic_ai_slim/pydantic_ai/models/wrapper.py:32`) provide compostable bases. `tests/conftest.py:476` exposes `create_module` for dynamic module synthesis.
- Gaps: These live in `pydantic_ai_slim`/`tests`, are not exported as a `pydantic_ai.testing` helpers package, have no `ExtensionTestKit` marker, and carry no contract-level assertions (e.g., “your `call_tool` must honor `RunContext.tool_call_metadata`”). An external author must copy-paste from tests/docs rather than `pip install pydantic-ai[testing]` and import fixtures.

### 3. Are examples provided?

**Yes — comprehensive, runnable doc examples plus a community registry.**

- `docs/capabilities/custom.md:7` (plain/dataclass/init/typed variants), `:104` (`get_toolset`), `:146` (`get_wrapper_toolset`), `:263` (`get_model`/`resolve_model_id`), `:639` (lifecycle hooks), and `:1031` (“Test custom capabilities the same way you test agents — using `TestModel`/`FunctionModel`”).
- `docs/toolsets.md:861` (`AbstractToolset` skeleton), `docs/models/overview.md:114` (`Model`/`StreamedResponse` subclass + `WrapperModel`), `docs/extensibility.md:8` (matrix of `AbstractToolset`/`WrapperToolset`/`Model`/`AbstractAgent`/`WrapperAgent`).
- `docs/extensibility.md:48` and `docs/toolsets.md:883` catalog third-party toolsets/capabilities (`pydantic-ai-skills`, `pydantic-ai-todo`, `pydantic-ai-filesystem-sandbox`, `pydantic-ai-ejentum`), confirming the ecosystem actually consumes the contracts.
- `examples/` ships ~30 standalone agents (`examples/pydantic_ai_examples/weather_agent.py:1`, `bank_support.py`, `rag.py`, etc.) that are executed in CI via `tests/test_examples.py`/`test-examples` job (`.github/workflows/ci.yml:477`), so examples are compilation-tested.

### 4. Are stability guarantees documented?

**Yes at the release level; partial at per-interface granularity.**

- `docs/version-policy.md:1` pledges no intentional breaking changes in minor releases post-V1, deprecations live until next major (>=3 months after V2.0 2026-06-23), and enumerates permitted non-breaking changes (new message parts/stream events/optional fields, OTel span attributes). `docs/changelog.md:93` provides a mechanical V1→V2 migration map with PR links and before/after tables.
- Beta disclaimer (`docs/version-policy.md:22`) explicitly scopes instability to `beta` modules.
- Process: `.github/scripts/check_api_compatibility.py:51`’s `--against` tag + `allowlist.json` waiver (fingerprint = `sha256(package\0path\0message)`) enforces “preserve compatibility or follow the allowed compatibility-impact process” with `::error`/`::warning` annotations. `tests/test_public_interface_contracts.py:40` locks public dataclass constructor shape and agent-wrapper forwarding.
- Gap: No `STABILITY.md` or per-contract `@stable`/`@experimental` marker distinguishing which extension points (`AbstractCapability.before_tool_validate` vs. experimental `deferred_tool_handler`) are covered by the semver pledge vs. `beta`. Breaking-change advisories are release-level (`changelog.md`/`release notes` URL in `pyproject.toml:92`), not pushed via in-code deprecation inventory per extension method.

## Architectural Decisions

| Decision | Evidence | Rationale / Tradeoff |
|----------|----------|----------------------|
| ABC-based contracts over duck-typed protocols | `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:162` (`class AbstractCapability(ABC)`), `toolsets/abstract.py:76`, `models/__init__.py:366`, `providers/__init__.py:42` | Explicit `abstractmethod` gives import-time failure and Pyright exhaustiveness (`assert_never`) at cost of rigidity vs. `Protocol` structural typing that would allow lighter third-party shims. |
| `TestModel`/`FunctionModel` as universal doubles | `pydantic_ai_slim/pydantic_ai/models/test.py:62`, `models/function.py:52` | Keeps doc examples and unit tests runnable without API keys and without VCR cassettes; trade is low fidelity (no provider metadata, no truncation/compaction wire behavior). |
| Wrapper/decorator base classes (`WrapperModel`, `WrapperToolset`) | `pydantic_ai_slim/pydantic_ai/models/wrapper.py:32`, `pydantic_ai_slim/pydantic_ai/toolsets/wrapper.py` doc at `docs/toolsets.md:590` | Enables middleware-style extensions (logging, caching) without combinatorial subclasses; requires authors to understand delegation (`__getattr__`, `__aenter__`) and `for_run` isolation. |
| Griffe-based public-API diff vs. snapshot tests | `.github/scripts/check_api_compatibility.py:106` | Precise cross-tag diff with fingerprint waivers; cheaper than maintaining hand-written API snapshots but detects only surface signature breaks, not behavioral contract regressions. |
| Capability naming + `from_spec` registry for packaging | `docs/extensibility.md:24`, `capabilities/abstract.py:262` (`get_serialization_name`/`from_spec`) | Allows spec-driven (`Agent.from_spec`) extension discovery with `custom_capability_types`; couples stability to name strings and forces package-prefix discipline (`pydantic-ai-*`). |
| Central `ModelProfile` TypedDict as capability oracle | `pydantic_ai_slim/pydantic_ai/profiles/__init__.py:110`, `docs/models/overview.md:72` | Single dict drives provider/model capability branching; TypedDict merger (`merge_profile`) permits cross-class key preservation but removes `isinstance` narrowing, pushing authors toward `profile.get(...)` guards. |

## Notable Patterns

- **Middleware chaining with ordering tier:** `CapabilityOrdering(position='outermost'|'innermost', wraps, wrapped_by, requires)` (`capabilities/abstract.py:116`) + `CombinedCapability` topological sort — reusable hooks compose deterministically, analogous to ASGI/Express middleware.
- **WrapperToolset as cross-cutting decorator:** `FilteredToolset`/`PrefixedToolset`/`PreparedToolset`/`ApprovalRequiredToolset` all delegate to an inner `AbstractToolset`; recommended over forking each concrete toolset (`toolsets/AGENTS.md:7`).
- **Profile-guarded tool visibility:** `ModelRequestParameters.tool_visibility: dict[str, ToolVisibility]` (`models/__init__.py:180`) + `prepare_request` populating it lets `TestModel._get_tool_calls` (`models/test.py:192`) respect `'withheld'|'via_history'` without provider I/O.
- **Lifecycle isolation via `for_run`/`for_run_step`:** Toolsets and capabilities return fresh instances per run/step (`toolsets/abstract.py:112`, `capabilities/abstract.py:309`) to avoid cross-run state leakage — tested heavily in durability tests (`tests/test_capability_process_event_stream.py:120` creating `InnerWrapper`).
- **Spec opt-out for non-serializable extensions:** `get_serialization_name() -> None` signals “not loadable from YAML” — intentional escape hatch preventing over-constraint of extension constructors.

## Tradeoffs

| Tradeoff | Pro | Con |
|----------|-----|-----|
| Internal doubles instead of conformance suite | Fast, deterministic docs/tests; no live API costs | Extension defects that only appear against real providers (e.g., `CompactionPart` wire) pass local tests but fail in production; no red/green signal for external authors |
| Griffe signature diff only | Catches accidental public breakage on every PR (`ci.yml:142`) | Ignores behavioral contract drift (e.g., change to when `wrap_tool_validate` may defer) and field-level JSON-schema tightening |
| ABC rigidity | Tooling-assisted implementation, clear “must override” list | Harder to add optional hook methods without minor-version break; Pyright `reportPrivateImportUsage` suppressions proliferate in tests |
| Beta-module scoping for instability | Fast innovation without semver debt | `beta`-label heuristic is coarse — capabilities marked beta in code but not in docs lead to surprise breakage; no `docs/version-policy.md` automation links beta flags to changelog category |
| Examples as “fixture” | Runnable, copy-paste friendly; CI-guarded via `test-examples` matrix (`ci.yml:477`) | Examples drift from contract: doc update may fix prose while leaving `get_tools` error handling stale; no property test pins example’s error paths |

## Failure Modes / Edge Cases

- **Silent non-conformance:** A custom `Model` that implements `request` but omits `request_stream` or mishandles `ToolVisibility='withheld'` will typecheck (Pyright strict) yet be rejected only at runtime by a specific agent flow (e.g., `agent.run_stream()` raises `NotImplementedError` from base `Model` at `models/__init__.py:552`), with no earlier conformance lint.
- **Stale waiver fingerprint:** `check_api_compatibility.py:46` fingerprints `package\0path\0message`; a docstring reflow that rewords the griffe message changes the fingerprint, invalidating an existing allowlist entry and failing CI even though the breaking surface is unchanged. No fuzzy matching.
- **Beta-leakage to stable:** Because `DEFAULT_PROFILE` merges provider/user profiles (`models/__init__.py:864`), a beta provider flag (e.g., `tool_deferral_mode`) can leak into a stable profile dict without warning — custom models reading `profile.get('anthropic_*')` on non-Anthropic routes see real values only after V2’s TypedDict merge fix (`changelog.md:158`).
- **Fixture version skew:** `TestModel.supported_tool_deferral_modes = frozenset({'standalone','with_tool_search'})` (`models/test.py:77`) now tracks production modes; a test written against an older `TestModel` that asserted `defer_loading=False` may silently pass while production adapters enforce new visibility rules — no migration codemod.
- **Churn from generated examples:** `examples/` and `docs` snippets are tested via `find_examples` (`pyproject.toml:238 exclude /tests` but includes `docs/**/*.py`), so a bulk `ruff format` or `pyright: ignore` change can churn snapshots (`inline-snapshot` format-command `pyproject.toml:497`) and mask a real contract drift.

## Future Considerations

- Publish a dedicated `pydantic_ai.testing` extension kit: `CapabilityConformanceSuite`, `ToolsetConformanceSuite`, `ModelConformanceSuite` with parametrized pytest classes that assert `prepare_request` visibility resolution, deferred-loading roundtrips, `for_run` isolation, and `wrap_*` error propagation — analogous to `pluggy`/`pytest` plugin conformance fixtures.
- Add a `pydantic ai check-extension <module>` CLI wrapping `griffe` against the extension’s declared ABCs and running the above suite plus a `TestModel`-backed “golden trace” snapshot, so `python -m pydantic_ai check-extension my_plugin` is the documented verification step (referenced from `docs/extensibility.md`).
- Export `TestModel`/`FunctionModel`/`FunctionToolset` as stable `pydantic_ai.testing` symbols with semver guarantees and dedicated docs, rather than relying on their current `pydantic_ai.models.test` import path which is still treated as “test helper” in coverage config (`pyproject.toml:439`).
- Per-contract stability annotations: add `@experimental`/`@stable` decorators or `__stability__` markers on `AbstractCapability` hooks and document them in `docs/version-policy.md:22` with a compatibility matrix, so the allowlist can optionally scope to `stable` surfaces only.
- Emit machine-readable breaking-change reports on release (JSON tying `Fingerprint` to `changelog.md` anchor) and feed them into `check_api_compatibility.py:71`’s `emit_annotation` message so PR authors get a link to the migration recipe, not just “Preserve compatibility or follow the allowed compatibility-impact process.”
- Contract-level `assert_never` exhaustiveness tests for message-part handling in custom models (`ModelResponsePart` union) — currently only in-repo providers pin exhaustive branches (`models/AGENTS.md:65`), but a shared helper `assert_exhaustive_part_handling(CustomModel)` would proactively break third-party adapters when a new `CompactionPart` ships.

## Questions / Gaps

- No evidence found of conformance tests that run a third-party capability against a recorded provider cassette and assert the exact `ModelRequestParameters` wire shape — searched `tests/**/*.py` for `conformance`, `extension.*fixture`, `AbstractCapability.*test`, all returning only the generic coverage and signature-parity tests.
- No evidence found of fixtures shipped for external consumption (no `pydantic_ai_slim/pydantic_ai/testing/` package, no `pyproject.toml` extra `[testing]` exporting helpers) — fixtures are internal to `pydantic_ai_slim` and `tests/conftest.py`.
- No evidence found of per-extension breaking-change notification channel (no `BREAKING_CHANGES.md` per contract, no GitHub release label `extension-break` filtering) — only global `docs/changelog.md` and `git tag` release notes (`pyproject.toml:92` `Changelog = .../releases`).
- Open question: Intent behind `AbstractCapability._safe_at_runtime: ClassVar[bool] = False` (`capabilities/abstract.py:187`) and the referenced `#5477` — whether third-party durability interactions will eventually require a public `is_safe_at_runtime()` hook that also serves as a stability-tier marker.

---

Generated by `Dimension 21.03: Extension Compatibility Testing` against `pydantic-ai`.

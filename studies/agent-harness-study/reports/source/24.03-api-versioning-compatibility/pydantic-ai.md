# Source Analysis: pydantic-ai

## API Versioning and Compatibility

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python 3.10+ / uv workspace (pydantic-ai-slim, pydantic-graph, pydantic-evals, clai) / Hatch + uv-dynamic-versioning |
| Analyzed | 2026-08-27 |

## Summary

Pydantic AI implements a deliberate semver-style compatibility model: Pep440 git-tag versioning via `uv-dynamic-versioning` (`pyproject.toml:8-11`), an explicit `Version Policy` (`docs/version-policy.md:1-31`) that forbids intentional breaking changes in minors and gates removals to majors (≥3 months after V2.0 on 2026-06-23), and an automated public-API break detector (`check_api_compatibility.py:106-130` + `ci.yml:142-158`) using `griffe` against the latest stable tag with a fingerprinted allowlist (`api-compatibility-allowlist.json:2`). Deprecations are executable (emitting `PydanticAIDeprecationWarning` (`_warnings.py:4-10`), a `UserWarning` subclass visible by default) and were staged across V1.100+ before V2 removal, documented in a full Upgrade Guide (`changelog.md:1-340`) and Migration Map (`migration.md:1-176`). Persisted contracts (`ModelMessagesTypeAdapter` at `messages.py:2768`) preserve backward compatibility via `AliasChoices` validation aliases for renamed fields (`vendor_details→provider_details` `messages.py:2578`, `request_tokens→input_tokens` `usage.py:91`, etc.) and explicit regression tests (`tests/test_messages.py:575-740`). Version negotiation exists narrowly: OTel instrumentation versions 2-6 (`_instrumentation.py:32`, `models/instrumented.py:79-165`, deprecation warning on 2-4), MCP SDK v1/v2 dual-field reading (`_mcp_compat.py:14-33`), and AG-UI forward-compat skipping of unknown discriminated-union tags (`ui/ag_ui/_forward_compat.py:64-118`) plus lowest-versions testing. The approach is strong for Python public API and stored message histories, but intentionally permits additive message-part/field additions and span-attribute changes as non-breaking (`version-policy.md:14-15`), so an upgrade is not fully zero-audit.

## Rating

**7 / 10** — Clear model with tests, explicit version policy, staged deprecations, and CI-enforced break detection; proven for message-history round-tripping and SDK-version coexistence. Downgraded from 9 because (a) instrumentation attribute names/defaults may churn in minors by policy, (b) beta (`beta` module) surfaces carry no stability guarantee, and (c) compatibility for traces/prompts/tool-schemas relies on defensive coding rather than property-based contract tests across all versions.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Version fields & policy | `uv-dynamic-versioning` Pep440 git versioning; `[tool.hatch.version] source = "uv-dynamic-versioning"` | `pyproject.toml:5-11` |
| Version fields & policy | Version Policy: V1 Sep 2025, V2 2026-06-23, no intentional breaking in minors, deprecations removed next major ≥3mo, security fixes 6mo | `docs/version-policy.md:3-6` |
| Version fields & policy | Explicit non-breaking carve-outs: new message parts/stream events/optional fields, OTel span attribute churn, `__repr__` changes | `docs/version-policy.md:11-16` |
| Version fields & policy | Beta feature clause: `beta` module, API/behavior may break in minors, expected to graduate in months | `docs/version-policy.md:22-25` |
| Version fields & policy | Package version exposed via `__version__ = _metadata_version('pydantic_ai_slim')` | `pydantic_ai_slim/pydantic_ai/__init__.py:376` |
| Changelog & migration | Upgrade Guide with split "not covered by deprecation warnings" vs "covered by deprecation warnings" and recommended path via latest V1 | `docs/changelog.md:93-108` |
| Changelog & migration | `ModelMessagesTypeAdapter` continuity promise: "Message history serialized with V1 continues to deserialize in V2" | `docs/changelog.md:110` |
| Changelog & migration | Before→after migration map for every V1→V2 rename (agent config→capabilities, `GeminiModel→GoogleModel`, `MCPServer*→MCPToolset`, `Usage→RunUsage`, etc.) | `docs/migration.md:10-176` |
| Deprecation notices | Dedicated `PydanticAIDeprecationWarning(UserWarning)` so deprecations visible by default (libraries' `DeprecationWarning` silent) | `pydantic_ai_slim/pydantic_ai/_warnings.py:4-10` |
| Deprecation notices | `@deprecated(..., category=PydanticAIDeprecationWarning)` on `exa_*` tools/`ExaToolset` with "will be removed in v3" | `pydantic_ai_slim/pydantic_ai/common_tools/exa.py:280-285,336-340,447-451` |
| Deprecation notices | Deprecated transports `TenacityTransport` / `AsyncTenacityTransport` warn toward `HTTPX2TenacityTransport` | `pydantic_ai_slim/pydantic_ai/retries.py:372-374,458-460` |
| Deprecation notices | `httpx.AsyncClient` support deprecated toward `httpx2.AsyncClient` | `pydantic_ai_slim/pydantic_ai/_http.py:98-101` |
| Deprecation notices | `GitHubProvider` deprecated (GitHub Models retired 2026-07-30, removed in v3) | `pydantic_ai_slim/pydantic_ai/providers/github.py:34-35` |
| Deprecation notices | `CompletedStreamedResponse` positional `(model_request_parameters, response)` and `events` alias deprecations | `pydantic_ai_slim/pydantic_ai/models/__init__.py:1252-1311` |
| Deprecation notices | Profile legacy keys `tool_additions` / `deferred_tools_require_tool_search` translate with `PydanticAIDeprecationWarning` | `pydantic_ai_slim/pydantic_ai/profiles/__init__.py:192-214` |
| Backwards-compat tests | Griffe-based API break detector: `run_griffe` invokes `griffecli check <pkg> --search <path> --against <tag>` and fingerprints `f"{package}\\0{path}\\0{message}"` SHA256 | `.github/scripts/check_api_compatibility.py:106-148` |
| Backwards-compat tests | CI gate fetches latest stable release tag and `uv run python check_api_compatibility.py --against "$release_tag"`; blocks merge without waiver | `.github/workflows/ci.yml:142-158` |
| Backwards-compat tests | Allowlist schema (`Waiver` requires `against`, `fingerprint` /^[0-9a-f]{64}$/, `reason`, `pull_request` URL) — currently empty | `.github/api-compatibility-allowlist.json:1-3`, `.github/scripts/check_api_compatibility.py:23-29` |
| Backwards-compat tests | Waiver usage: `waiver_key = (against, finding.fingerprint)` → error if missing, warning if allowed; unused waivers also fail | `.github/scripts/check_api_compatibility.py:68-92` |
| Schema migration | `ModelMessagesTypeAdapter = TypeAdapter(list[ModelMessage], config=...)` documented for (de)serializing histories | `pydantic_ai_slim/pydantic_ai/messages.py:2768-2771` |
| Schema migration | Legacy `vendor_details`/`vendor_id` accepted via `AliasChoices('provider_details','vendor_details')` etc. on `ModelResponse` | `pydantic_ai_slim/pydantic_ai/messages.py:2577-2585` |
| Schema migration | Legacy `request_tokens`/`response_tokens` accepted via `AliasChoices('input_tokens','request_tokens')` on `UsageBase` with custom wrap validator preserving arbitrary fields | `pydantic_ai_slim/pydantic_ai/usage.py:88-92,109-113,143-188` |
| Schema migration | Regression test: pre-refactor stored messages with `request_tokens`/`vendor_details` still validate and map to new names | `tests/test_messages.py:575-645` |
| Schema migration | Regression test: `vendor_details`/`vendor_id` history replays through `agent.run(message_history=...)` | `tests/test_messages.py:707-740` |
| Schema migration | `UploadedFileProviderName` keeps `'google-gla'/'google-vertex'` for backward compat while current code emits `'google'/'google-cloud'` | `pydantic_ai_slim/pydantic_ai/messages.py:791-806` |
| Schema migration | `ToolAvailabilityDeltaPart` tools alias `AliasChoices('tools_added','added')` | `pydantic_ai_slim/pydantic_ai/messages.py:1803` |
| Capability negotiation | Instrumentation `DEFAULT_INSTRUMENTATION_VERSION = 5`; `InstrumentationSettings.version: Literal[2,3,4,5,6]` with `2,3,4` emitting `PydanticAIDeprecationWarning` | `pydantic_ai_slim/pydantic_ai/_instrumentation.py:32`, `pydantic_ai_slim/pydantic_ai/models/instrumented.py:79,154-164` |
| Capability negotiation | Version-aware OTel mapping: `version>=4` uses `BlobPart`/`UriPart`+`modality`, `version>=6` moves tool returns to `role='tool'` | `pydantic_ai_slim/pydantic_ai/messages.py:1063-1085,1113-1132`, `pydantic_ai_slim/pydantic_ai/models/instrumented.py:410-436` |
| Capability negotiation | Model capability flags in `ModelProfile` TypedDict (`supports_tools`, `supports_thinking`, `tool_deferral_mode`, `tool_addition_mode`, etc.) and `merge_profile` layering | `pydantic_ai_slim/pydantic_ai/profiles/__init__.py:47-262` |
| Capability negotiation | MCP SDK v1/v2 compat: `is_mcp_sdk_v2()` version parse `>=2.0.0` + field reader trying `snake_case` then `wire_name(camelCase)` fallback | `pydantic_ai_slim/pydantic_ai/_mcp_compat.py:14-33` |
| Capability negotiation | AG-UI forward compat: unknown `role`/`type` discriminators skipped only if `id: str` contract satisfied; keeps malformed failures | `pydantic_ai_slim/pydantic_ai/ui/ag_ui/_forward_compat.py:32-118` |
| Capability negotiation | AG-UI version floor `>=0.1.10`; bump disallowed, new functionality gated behind `requires_ag_ui('<version>')` in tests, lowest-versions job exercises floor | `pydantic_ai_slim/pydantic_ai/ui/AGENTS.md:3-10`, `tests/test_ag_ui.py:100,179,1658` |
| Capability negotiation | Runtime version checks for AG-UI feature gating (imports guarded, not module-level skip) | `tests/test_ag_ui.py:172-185` |
| Feature flags / beta | No `beta` submodule directory found in `pydantic_ai_slim/pydantic_ai`; beta mechanism is policy-level (docs) rather than code-enforced namespace in this snapshot — no `pydantic_ai/beta` package present | `pydantic_ai_slim/pydantic_ai/` directory listing (no `beta/` entry) |
| Compatibility tests | Message round-trip tests for `ModelMessagesTypeAdapter` preserving `run_id`/`conversation_id` and back-compat missing `conversation_id` | `tests/test_messages.py:862-908` |
| Compatibility tests | Arbitrary usage field preservation across dump/validate cycle | `tests/test_messages.py:669-704` |
| Compatibility tests | `test_legacy_vendor_message_history_replays_through_agent` ensures stored history feeds `agent.run` | `tests/test_messages.py:707-740` |

## Answers to Dimension Questions

### 1. Which APIs are stable, experimental, deprecated, or internal?

* **Stable:** Everything under `pydantic_ai`, `pydantic_graph`, `pydantic_evals`, `clai` public packages since V2 (post-2026-06-23) per `docs/version-policy.md:5`. The griffe gate (`ci.yml:142-158` + `check_api_compatibility.py:106-130`) treats any public surface change vs latest tag as a break requiring an explicit fingerprint waiver (`api-compatibility-allowlist.json:2`) — currently empty, so default is "no break allowed".
* **Deprecated (warnings + removal in next major):** Marked with `typing_extensions.deprecated` + `PydanticAIDeprecationWarning`. Examples: Exa common tools → `ExaSearch` harness capability (`common_tools/exa.py:280`); `TenacityTransport` → `HTTPX2TenacityTransport` (`retries.py:372`); `httpx.AsyncClient` → `httpx2.AsyncClient` (`_http.py:98`); `GitHubProvider` (retired) → removed v3 (`providers/github.py:34`); instrumentation versions 2-4 (`models/instrumented.py:160`); profile keys `tool_additions`/`deferred_tools_require_tool_search` (`profiles/__init__.py:192`). All carry "will be removed in v3" messages and some are covered in `migration.md`.
* **Experimental / Beta:** Defined by policy only (`version-policy.md:22-25`): "indicated by a `beta` module" and may break in minors. No `beta/` package was observed in the checked-out tree; the mechanism is currently documentary rather than a code-enforced namespace.
* **Internal:** Leading-underscore modules (`_instrumentation.py`, `_mcp.py`, `_warnings.py`, etc.), `pydantic_graph.persistence`/`mermaid` removed in V2 with no replacement (`changelog.md:119`), and provider internals. The policy explicitly says `__repr__` changes are never breaking (`version-policy.md:16`).

### 2. How are users warned before breaking changes?

Three-layer staging:

1. **Deprecation warnings in V1:** `PydanticAIDeprecationWarning` (`_warnings.py:4`) inherits from `UserWarning` (visible by default, unlike `DeprecationWarning`). Call sites use `warnings.warn(..., PydanticAIDeprecationWarning, stacklevel=2)` and `@deprecated(..., category=PydanticAIDeprecationWarning)`. V1.100 forked to V2 beta b7 so users on `>=1.100` see warnings for bulk of V2 removals (`changelog.md:103-105`).
2. **Documentation with migration snippets:** `changelog.md:93-226` lists every breaking group with PR links and before→after tables; `migration.md:1-176` is the fast grep table. Each PR/release note is required to include a **compatibility impact** warning when exercising the non-breaking carve-outs (`version-policy.md:20`).
3. **CI enforcement requiring explicit waivers:** `check_api_compatibility.py:68-92` errors on any Griffe-detected break unless a fingerprinted waiver (`against`, `sha256(message)`, `reason`, `pull_request` URL) is present; unused waivers also fail. This forces PR authors to either preserve compatibility or document the impact reviewably.

Behavior changes not warn-able (e.g., `end_strategy: 'early'→'graceful'`, `ModelProfile` TypedDict switch, slimmer default extras) are explicitly listed under "Changes not covered by deprecation warnings" (`changelog.md:112-187`) and must be reviewed even with zero warnings.

### 3. Are old clients, plugins, traces, or persisted artifacts still usable?

* **Persisted message histories / traces (`ModelMessagesTypeAdapter`):** Yes for stored DB histories. Legacy field names are kept as validation aliases, not removed: `vendor_details`→`provider_details` and `vendor_id`→`provider_response_id` (`messages.py:2577-2585`), `request_tokens`/`response_tokens`→`input_tokens`/`output_tokens` (`usage.py:88-113`), plus custom `__get_pydantic_core_schema__` that preserves arbitrary extra usage fields (`usage.py:143-188`). Tests prove round-tripping of pre-refactor histories (`test_messages.py:575-645`) and replay through `agent.run(message_history=...)` (`test_messages.py:707-740`). `UploadedFile` retains `google-gla`/`google-vertex` provider names (`messages.py:803-806`). The Upgrade Guide explicitly promises V1 histories deserialize in V2 (`changelog.md:110`).
* **Old clients / SDK plugins (MCP, AG-UI):** Partially. MCP reads both camelCase and snake_case MCP SDK field names (`_mcp_compat.py:26-33`) and branches on `is_mcp_sdk_v2()` (`_mcp_compat.py:14-17`) — coexistence, not just forward migration. AG-UI clients on a newer `ag-ui-protocol` than the server are forward-compatible by design: `_forward_compat.py:64-118` re-parses the body and drops only unknown `role`/`type` items that satisfy the shared-contract check (`id: str` for messages), leaving malformed payloads to still fail. Version flooring is pinned at `>=0.1.10` (`ui/AGENTS.md:8`) and exercised via `requires_ag_ui`-gated tests and a `test-lowest-versions` CI job.
* **Traces / OTel spans:** **Not fully stable across minors by design.** Policy explicitly reserves the right to change span attributes and instrumentation defaults in minors (`version-policy.md:15`). In V2, default instrumentation flipped to version 5 and run spans moved from `gen_ai.usage.*` to `gen_ai.aggregated_usage.*` (`changelog.md:125-126,180-185`); dashboards must migrate or pin `use_aggregated_usage_attribute_names=False`. Instrumentation versions 2-4 still function but warn.
* **Plugins / tool schemas / prompts stored with histories:** Tool schemas may be re-derived from `ModelProfile` (now `TypedDict` dict-spread, `profiles/__init__.py:249-262`). Old pickled profiles using dataclass attribute access require migration per `changelog.md:130-165`.

### 4. Does compatibility rely on policy alone or executable tests?

Both, but heavily weighted toward executable enforcement for Python API and message histories:

* **Executable:** Griffe public-API diff in CI (`ci.yml:142-158` + `check_api_compatibility.py:106-130`) with SHA256-fingerprinted waivers anchored to a specific `against` tag; fingerprint includes `package\0path\0message` so silent message rewrites create a new fingerprint and fail closed. BlockBuster, mypy, and the full matrix (including `lowest-versions` and `fastmcp-4` jobs) run per PR. Message-history compat has dedicated unit tests (`test_messages.py:575-740`, `862-908`) and AG-UI compat tests (`test_ag_ui.py:1658+` with `requires_ag_ui`). `kw_only` and agent-wrapper forwarding parity tests (`test_public_interface_contracts.py:102-356`) enforce additive-safety of public dataclass signatures.
* **Policy:** `version-policy.md:5-6,20` forbids breaking in minors and requires deprecation-until-next-major + compatibility-impact notes. Beta module instability and the additive carve-outs (new parts/fields, OTel attributes) rely on policy + "code defensively" guidance (`version-policy.md:14-15`) rather than a contract test that a new optional field won't break a consumer.
* **Gap:** No property-based or fuzz test that a future `PartStartEvent`/`PartDeltaEvent` variant round-trips through `ModelMessagesTypeAdapter` and UI adapters without loss; and OTel attribute stability is explicitly policy-permitted to churn.

## Architectural Decisions

| Decision | Why | Evidence |
|----------|-----|----------|
| Pep440 git-tag versioning via `uv-dynamic-versioning` | Reproducible, tag-driven releases; avoids manual version file drift | `pyproject.toml:8-11` |
| Semver-like policy (V2 2026-06-23, ≥3mo before next major, 6mo security support) | Predictable upgrade cadence | `docs/version-policy.md:3-8` |
| Additive-only message evolution (new parts/fields with defaults) + `ModelMessagesTypeAdapter` | Permits extension without major; `AliasChoices` preserves DB histories | `docs/version-policy.md:14-15`, `messages.py:2768`, `messages.py:2577`, `usage.py:88-92` |
| `PydanticAIDeprecationWarning(UserWarning)` visible-by-default | Libraries' `DeprecationWarning` is silent; this ensures users see migration nudges without `-W` | `pydantic_ai_slim/pydantic_ai/_warnings.py:4-10` |
| Griffe + fingerprint waiver gate in CI | Machine-enforced break detection; waiver is reviewable and tag-scoped | `.github/scripts/check_api_compatibility.py:23-48,68-92`, `.github/workflows/ci.yml:142-158` |
| Instrumentation versioned separately (2-6, default 5) | Decouples telemetry wire from package version; allows iterative OTel spec alignment | `pydantic_ai_slim/pydantic_ai/_instrumentation.py:32`, `models/instrumented.py:79,129-136` |
| MCP field dual-read (snake/camel) + `is_mcp_sdk_v2()` | Smooth SDK v1/v2 transition without forking code | `pydantic_ai_slim/pydantic_ai/_mcp_compat.py:14-33` |
| AG-UI forward-compat filter (`skip_unknown_tagged_items`) with `id: str` gate | Older server skips only provably-new functionality; malformed payloads still fail | `pydantic_ai_slim/pydantic_ai/ui/ag_ui/_forward_compat.py:92-118`, `ui/AGENTS.md:8` |
| `ModelProfile` as `TypedDict(total=False)` + `merge_profile` dict-spread | Enables partial overrides and cross-class field preservation (deliberate in V2) | `pydantic_ai_slim/pydantic_ai/profiles/__init__.py:47,217-262`, `docs/changelog.md:120,130-165` |
| Allow OTel attribute churn as non-breaking | Acknowledge OTel GenAI spec instability; policy lets defaults follow spec | `docs/version-policy.md:15` |

## Notable Patterns

* **Staged deprecation → major removal:** V1.100+ emits warnings for bulk of V2 removals; V2 then turns `return None` in `prepare_tools` into `TypeError`, flips `openai:` prefix to Responses API, etc. (`changelog.md:53-58,192-226`).
* **Fingerprint-scoped waivers:** Waiver key is exactly `(tag, sha256(package\0path\0message))`, not free-text; prevents waiver reuse across releases and silently changed break messages (`check_api_compatibility.py:46-48,68-92`).
* **Validation aliases for persisted-state migration:** `AliasChoices` on renamed fields rather than data-migration scripts (`messages.py:2577-2585`, `usage.py:91,111`).
* **Additive schema evolution with arbitrary-field preservation:** `UsageBase.__get_pydantic_core_schema__` captures unknown keys into `__dict__` and re-emits them (`usage.py:143-188`), so future token-detail keys survive round-trip.
* **Version-aware telemetry:** `InstrumentationSettings.version` drives distinct codepaths for message serialization and tool-call OTel parts (`messages.py:1067-1132`, `models/instrumented.py:410-436`).
* **Lowest-versions + extra-matrix testing:** `ci.yml:346-451` runs `lowest-direct` resolution; `fastmcp-4` isolated job (`ci.yml:500-543`) validates without polluting primary lock.

## Tradeoffs

| Tradeoff | Pro | Con |
|----------|-----|-----|
| Additive message evolution permitted without major | Extensions ship faster; no major for every new `ThinkingPart`/`FilePart` | Consumers must code defensively; exhaustive `match` on `part_kind` can silently ignore new variants until they handle `assert_never` (`messages.py` discriminator) |
| OTel attributes allowed to churn in minors | Can track evolving OTel GenAI spec without waiting for major | Production dashboards/alerts on run-span usage (`gen_ai.usage.*` → `gen_ai.aggregated_usage.*`) break on minor upgrade unless pinning `use_aggregated_usage_attribute_names=False`; documented but still an audit item |
| Visible `UserWarning`-based deprecations | Users see warnings without `-W`; migration more likely | Noise for users who deliberately ignore warnings; requires explicit `filterwarnings` to silence |
| Griffe static API diff | Fast, no test harness needed; catches signature/field changes | Cannot catch semantic/behavioral breaks (e.g., `end_strategy` default flip, `sequential` barrier semantics) which still require release-note review |
| Slimmer default `pydantic-ai` extras in V2 (`changelog.md:124`) | Smaller install, faster CI, clearer dependency | Bare `pip install pydantic-ai` on V1→V2 silently drops `bedrock`, `groq`, etc.; second-order break not caught by API diff |

## Failure Modes / Edge Cases

* **Missed instrumentation migration:** Upgrading V1→V2 without reading `changelog.md:125-126` leaves token dashboards double-counting `gen_ai.usage.*` on parent+child spans. Mitigation: pin `InstrumentationSettings(version=5, use_aggregated_usage_attribute_names=False)` or update dashboards.
* **History stored with `vendor_*` keys but re-validated with strict model:** Works today via `AliasChoices`, but writing those keys back out emits new canonical names; a system that string-compares raw JSON histories will see churn (`messages.py:2577-2585`).
* **MCP SDK v3:** `is_mcp_sdk_v2()` checks `>=2.0.0` with `tuple(map(int, groups))` (`_mcp_compat.py:16`). A hypothetical v3 that re-renames fields again would silently mis-read until `mcp_field_value` fallback added.
* **AG-UI malformed-vs-new confusion:** A client bug that sends `role="assistant"` without `id: str` is treated as malformed and correctly fails, but a legitimate new role that the union also requires `id` for will be correctly skipped — reliance on single-field `id` contract is fragile if AG-UI ever introduces a role without `id` (`_forward_compat.py:95-102`).
* **Empty allowlist drift:** `api-compatibility-allowlist.json:2` being `[]` is correct now, but a PR that introduces an intentional break without adding a waiver will fail CI late (after `griffe` fetch), not at typecheck time.
* **Beta adoption risk:** Code importing `pydantic_ai.beta.*` (if introduced) has no stability promise; minor update can break it without waiver (`version-policy.md:24`).
* **Provider profile cross-pollination:** V2 resolved profiles now carry cross-class fields where V1 filtered them (`changelog.md:158-164`); custom `Model` subclasses that `profile.get('anthropic_supports_adaptive_thinking')` on non-Anthropic routes will now branch differently.

## Future Considerations

* Add a `compat_version` or `schema_version` field to `ModelMessagesTypeAdapter` envelope if additive evolution ever needs non-additive migration (currently versionless list).
* Promote AG-UI `_forward_compat` skip to return structured `skipped` labels to observability (already returns `frozenset[str]` at `ui/ag_ui/_forward_compat.py:64`) so skipped-message metrics surface in spans.
* Encode instrumentation version policy into `pyproject.toml` classifiers or runtime negotiation (currently `DEFAULT_INSTRUMENTATION_VERSION = 5` constant vs `version` kwarg) — automate deprecation window for 2-4 removal (`models/instrumented.py:156-157 TODO(v3)`).
* Publish a machine-readable deprecation catalog (JSON) alongside `migration.md` so coding agents and `check_api_compatibility.py` can cross-link waiver fingerprints to migration recipes.
* Consider pinning `ag-ui-protocol` floor bump requires opt-in via `check_api_compatibility.py` allowlist; today a floor bump is just a doc change (`ui/AGENTS.md:3-5`).

## Questions / Gaps

* No evidence found for explicit version negotiation on `prompt` templates or `tool` JSON Schemas beyond OTel — search of `prompts`, `template`, `toolsets` for `version`/`negotiat*` returned only instruction-joining helpers. Caller must treat prompt text as opaque.
* No evidence found for SDK-vs-server capability handshake (e.g., `capabilities` exchange) at install time; provider fact discovery is static via `ModelProfile` flags, not negotiated at `Agent` construction. Searched `capabilities/`, `providers/`, `agent/` for `negotiat*`, `handshake`, `capab*` + `version`.
* Tests assert history *deserialization* but no long-lived fuzz corpus testing migration from every historical V0.x tag to V2; only targeted snapshots (`test_messages.py:575`).
* Beta surface not present to audit in this checkout; version-policy claim of "beta module" is accepted as design intent per `version-policy.md:24`.
* CLI (`clai`) vs SDK version divergence not covered by same Griffe gate? `PACKAGES` mapping includes `clai` (`check_api_compatibility.py:18`) so it is gated, but CLI semver guarantees vs SDK not separately documented.

---

Generated by `Dimension 24.03: API Versioning and Compatibility` against `pydantic-ai`.

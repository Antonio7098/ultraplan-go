# Source Analysis: openhands

## API Versioning and Compatibility

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19 + React Router 7 (Vite 8), @openhands/typescript-client 1.38.1, @openhands/extensions 0.18.0 |
| Analyzed | 2026-08-27 |

## Summary

`openhands` in this workspace is the **agent-canvas frontend only** (`package.json:2` `@openhands/agent-canvas@1.15.0`), not the agent-server SDK. Version and compatibility logic is concentrated in a single executable gate — `src/api/agent-server-compatibility.ts:18-19` reads a floor (`config/defaults.json:9` `compatibility.minimumAgentServer: "1.28.0"`) and `loadAgentServerInfo()` (`src/api/agent-server-compatibility.ts:320`) fetches `GET /server_info` via `@openhands/typescript-client` `ServerClient` with a 5s timeout (`src/api/agent-server-compatibility.ts:15`), parses semver (`parseAgentServerVersion():273`, `compareAgentServerVersions():243`), and throws typed errors (`AgentServerUnsupportedVersionError:72`, `AgentServerUnknownVersionError:86`) that `root.tsx:22` and health checks surface as UI. Capability negotiation is lightweight: `usable_tools` advertized on `/server_info` (`src/api/agent-server-compatibility.ts:27`, `src/mocks/settings-handlers.ts:661`), `X-OpenHands-Client` / `X-OpenHands-Client-Version` headers (`src/api/client-source.ts:12-13`) and automation `DeploymentCapabilities` discovery (`src/api/automation-service/automation-service.api.ts:539`). Persisted settings are versioned (`src/services/settings.ts:36` `schema_version: 6` / `:55` `schema_version: 1`) and migrated through the generic `misc_settings.app_preferences` container introduced in agent-server 1.27 (`src/api/settings-service/settings-service.api.ts:47`). Releases are automated via release-please and conventional commits (`release-please-config.json:8`, `.agents/skills/release.md:25`, `.github/release.yml:1`) with no checked-in changelog or deprecation policy — breaking changes are signaled only by semver bump and the runtime version gate.

## Rating

**6 / 10 — Present but inconsistent / fragile**

Rationale: a clear, tested version floor and capability probe exist (executable `assertAgentServerVersionIsSupported` + 6 unit tests), but it covers only the local `agent-server` surface. Automation and cloud surfaces use best-effort discovery that degrades to `"unknown"`; persisted-conversation and event-schema compatibility is delegated to the `software-agent-sdk` sibling repo (explicit per `AGENTS.md` repository map) with no contract tests in this repo; there is no deprecation timeline, migration guide, or breaking-change linter — compatibility relies on human-enforced conventional commits and release-please automation plus a single runtime error, so a production integration can detect an incompatible server at startup but cannot safely assess upgrade impact without auditing `software-agent-sdk`.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Version field & compatibility floor | `config/defaults.json:3-10` pins `versions.{agentServer: "1.42.1", agentCanvas: "1.15.0", automation: "1.8.0"}` and `compatibility.minimumAgentServer: "1.28.0"` — single source of truth consumed by JS launchers, Docker entrypoint, and CI | `config/defaults.json:3` |
| Version enforcement implementation | `MINIMUM_COMPATIBLE_AGENT_SERVER_VERSION` imported from defaults, `AgentServerInfo` extends `BaseServerInfo` with `sdk_version`, `usable_tools`, `runtime_services`; `parseAgentServerVersion`, `compareAgentServerVersions`, `assertAgentServerVersionIsSupported`, `loadAgentServerInfo` (5000 ms timeout, 401 handling, cache) | `src/api/agent-server-compatibility.ts:18,25,273,243,293,320` |
| Display helpers | `getDisplayAgentServerVersion` / `getDisplayAgentServerSdkVersion` / `getCachedAgentServerVersion` null on `unknown` or malformed (`dev-build`) versions | `src/api/agent-server-compatibility.ts:205,215,235` |
| Executable compatibility tests | 6 vitest cases: missing/unknown vs old, malformed, too-old, badge null, `sdk_version` fallback, dedicated sdk display | `src/api/agent-server-compatibility.test.ts:14` |
| Bootstrap gating | `OptionService.getConfig()` calls `loadAgentServerInfo()` to enforce floor and cache `usable_tools` before app renders | `src/api/option-service/option-service.api.ts:2` |
| Health polling | `useBackendsHealth` validates local backends via `SettingsClient` then `ServerClient.getServerInfo()` to hit compatibility floor every 10s | `src/hooks/query/use-backends-health.ts:78` |
| Settings schema versioning | `LATEST_SETTINGS_VERSION=5`, `DEFAULT_SETTINGS.agent_settings.schema_version=6`, `conversation_settings.schema_version=1` | `src/services/settings.ts:3,36,55` |
| Persisted settings migration | `MiscSettings{app_preferences}` container (added 1.27), `SettingsApiResponse.misc_settings`, `SettingsUpdateRequest.misc_settings_diff` deep-merge semantics, `transformApiResponse` hoists `misc_settings.app_preferences` onto flat `Settings`, `syncDerivedSettings` back-compat; flat `app_preferences/app_preferences_diff` replaced before stable, legacy localStorage migration removed in #1337 | `src/api/settings-service/settings-service.api.ts:47,55,77,321,358` |
| Mock fidelity for migration | `GET /server_info` returns `version: "1.29.3"` + `usable_tools: [terminal,file_editor,task_tracker,browser_tool_set]`; `GET/PATCH /api/settings` validates at least one `*_diff` present, deep-merges `misc_settings.app_preferences`, handles `X-Expose-Secrets: encrypted/plaintext` | `src/mocks/settings-handlers.ts:648,656,969,1047` |
| Capability negotiation — tools | `usable_tools` (not `available_tools`) from `software-agent-sdk#3028`; `isAgentServerToolAvailable()` defaults to `true` when server omits metadata; browser `browser_tool_set` and `task_tool_set` gated on this | `src/api/agent-server-compatibility.ts:34,149` |
| Capability negotiation — headers | `AGENT_CANVAS_CLIENT_SOURCE="agent_canvas"`, `AGENT_CANVAS_CLIENT_VERSION=packageJson.version`, `OPENHANDS_CLIENT_HEADER="X-OpenHands-Client"`, `OPENHANDS_CLIENT_VERSION_HEADER`, `AGENT_CANVAS_CLIENT_HEADERS` sent on automation and cloud proxy calls | `src/api/client-source.ts:3,12` |
| Capability negotiation — automation discovery | `AutomationService.getCapabilities()` `GET /api/automation/capabilities`, `useDeploymentCapabilities` with `meta.disableToast` — older deployments without discovery are expected; `assessCapabilityRequirements` set-membership check, `"unknown"` verdict when probe fails | `src/api/automation-service/automation-service.api.ts:539`, `src/hooks/query/use-manifest-capabilities.ts:21,36`, `src/manifests/manifest-capabilities.ts:36` |
| Skill extension compatibility | `SkillInfo.compatibility?: string\|null`, `BundledSkill.compatibility`, passed from `@openhands/extensions/skills` `SKILLS_CATALOG` through `buildBundledSkills` | `src/types/settings.ts:87`, `src/api/agent-server-adapter.ts:711,744` |
| Workspaces version gate | `getWorkspacesUnsupportedMessage` maps `isAgentServerVersionError` (from `typescript-client`) to `HOME$WORKSPACES_UNSUPPORTED_AGENT_SERVER` i18n with `actualVersion/requiredVersion` interpolation | `src/utils/workspaces-compatibility.ts:6` |
| Feature flags | `WebClientFeatureFlags{hide_llm_settings, hide_users_page}` via `GET /api/v1/web-client/config` | `src/api/option-service/option.types.ts:20`, `src/mocks/settings-handlers.ts:932` |
| Public API surface versioning | `package.json:206` `exports` map defines stable subpaths (`.`, `./browser`, `./conversation`, `./files`, `./settings`, `./sidebar`, `./terminal`, `./i18n`) with `dist/**` + `types`; library build `BUILD_LIB=true` with `preserveModules` + `external: [react, react-dom, react-router]` | `package.json:206`, `vite.config.ts:215` |
| Library contract tests | `package-library.test.ts:29` asserts exports shape, `vite-config.test.ts` guards library config | `__tests__/package-library.test.ts:29` |
| Release automation | `release-please-config.json:7` `release-type: node`, `skip-changelog:true`, `extra-files: [config/defaults.json$.versions.agentCanvas, helm/agent-canvas/Chart.yaml, README.md]`; `.github/release.yml` groups by `type: feat/fix` labels; version derived from conventional commits (`fix→patch, feat→minor, !/BREAKING CHANGE→major`) | `release-please-config.json:7`, `.github/release.yml:1`, `.agents/skills/release.md:25` |
| Changelog gap | `CHANGELOG.md:8` only `Unreleased` + `1.0.0-alpha.2 (2025-05-11)`; `release-please-config.json:10` `skip-changelog:true` — GitHub Releases is the changelog, no committed migration guide | `CHANGELOG.md:8` |
| Breaking-change documentation | No `MIGRATION.md`, `DEPRECATION.md`, or `docs/migration*`; `.agents/skills/release.md:25` notes `BREAKING CHANGE` footer drives major but no warning period documented; PR template `<!-- Optional: migrations, config changes -->` is advisory | `release-please-config.json:1` (absence), `.github/pull_request_template.md:62` |
| Back-compat branching | `agent-server-adapter.ts:830` `TODO(#1019)` notes `acp_isolate_data_dir` gated by unreleased `typescript-client` version; branch per `getDeploymentMode()==docker` for GitHub MCP patch, linear SSE→streamable patch | `src/api/agent-server-adapter.ts:830`, `AGENTS.md:635` (MCP patches) |
| Deprecation re-exports | `src/routes/mcp-settings.tsx:2` re-exports new `src/routes/mcp.tsx` to preserve `MCPSettings` library symbol after `/settings/mcp` removal in #1337; `AGENTS.md:546` documents flat `app_preferences_diff`→`misc_settings` container migration before stable | `src/routes/mcp-settings.tsx:2` |

## Answers to Dimension Questions

**1. Which APIs are stable, experimental, deprecated, or internal?**

No explicit stability taxonomy exists in-repo. Implied tiers from evidence:

* **Stable — npm public surface:** `package.json:206-248` `exports` (`"."`, `"./browser"`, `"./conversation"`, `"./files"`, `"./settings"`, `"./sidebar"`, `"./terminal"`, `"./i18n"`). Tested in `__tests__/package-library.test.ts:29`. Breaking a subpath would be a major via conventional commit `!` (`release-please-config.json:7`).
* **Stable — required agent-server:** `GET /server_info` and `GET/PATCH /api/settings` via `SettingsClient` are assumed stable; the floor `config/defaults.json:9` `1.28.0` is enforced at startup (`src/api/agent-server-compatibility.ts:320`).
* **Internal — agent-server & automation:** `AGENTS.md:Repository Map` declares this repo owns only frontend consumption; new endpoints and tool logic belong in `software-agent-sdk`; typed access is via `@openhands/typescript-client` pinned to `1.38.1` (`package.json:27`). Direct `axios`/`fetch` to `/api/*` is banned by `src/api/no-direct-agent-server-calls.test.ts` (cited in `AGENTS.md`).
* **Deprecated / experimental — skills & features:** `SkillInfo.compatibility` (`src/types/settings.ts:87`) is informational. MCP marketplace deprecated entries (GitLab, Google Maps, etc.) are dropped by catalog filter (`__tests__/constants/extensions-catalogs.test.ts:57`). `WebClientFeatureFlags.hide_llm_settings` (`src/api/option-service/option.types.ts:20`) gates experimental pages. The flat `app_preferences_diff` shape was deprecated before stable release and migrated to `misc_settings` (`src/api/settings-service/settings-service.api.ts:47` + `AGENTS.md:546`). No `Deprecated:` headers or sunset dates are used.

**2. How are users warned before breaking changes?**

* **Executable pre-flight, not advance notice.** `loadAgentServerInfo()` (`src/api/agent-server-compatibility.ts:320`) blocks app load when `actual < 1.28.0`, throwing `AgentServerUnsupportedVersionError` with message `Agent Canvas requires agent-server X or newer; this backend is running Y` (`src/api/agent-server-compatibility.ts:78`). Health polling (`src/hooks/query/use-backends-health.ts:78`) re-probes every 10 s and shows a version-mismatch toast. This is detection at startup, not a deprecation warning.
* **Release-please semver** derives the next version from conventional-commit types (`feat→minor, fix→patch, !/BREAKING CHANGE→major` per `.agents/skills/release.md:25`). GitHub Releases are the changelog (`.github/release.yml:1` groups by `type:` label, `CHANGELOG.md:5` is stale — `Unreleased` is empty). There is no `CHANGELOG.md` update, no `DEPRECATION.md`, no `migration.md`, and no `warn` logs for deprecated fields.
* **Telemetry/client headers** carry the frontend version non-disruptively (`X-OpenHands-Client-Version: packageJson.version` via `src/api/client-source.ts:12`) for server-side observability, but the server does not use them to negotiate a compatibility window.
* **Result:** an operator learns a breaking floor only after upgrading either side and hitting the runtime error; there is no N-1 support window or staged rollout documented.

**3. Are old clients, plugins, traces, or persisted artifacts still usable?**

* **Old frontends against new servers:** yes — `isAgentServerToolAvailable:149` returns `true` when `usable_tools` is absent, and `transformApiResponse:321` tolerates missing `misc_settings`. New fields are additive.
* **New frontends against old servers:** gated. If server < `1.28.0`, frontend refuses to load data (`assertAgentServerVersionIsSupported:293`). If server lacks `misc_settings` (pre-1.27), `src/api/settings-service/settings-service.api.ts:338` falls back to defaults; if `GET /api/automation/capabilities` 404s, `use-manifest-capabilities.ts:29` treats it as `unknown` rather than error — older deployments still render with manifest defaults. `WorkspacesClient` delegates its own preflight to `typescript-client` (comment `src/api/workspaces-service/workspaces-service.api.ts:7`).
* **Persisted settings:** versioned (`src/services/settings.ts:36` `schema_version`); migration is deep-merge on `misc_settings_diff` so a `PATCH {"misc_settings_diff":{"app_preferences":{"language":"fr"}}}` leaves siblings untouched (`src/api/settings-service/settings-service.api.ts:70`). Lists (`disabled_skills`) are replaced wholesale. The former flat `app_preferences` storage (localStorage) was drained in one release then removed — no backward read of that key remains.
* **Persisted conversations / traces / tool schemas:** **Not tested here.** The repository map explicitly assigns conversation, event, and tool persistence to `software-agent-sdk`; this repo only reads them via `RemoteWorkspace` / `RemoteEventsList` from `typescript-client`. No trace-replay or event-migration tests exist in this checkout. `config/defaults.json:8-13` notes a temporary `agentClientProtocol<0.11` pin because `acp 0.11.0` reordered `PromptRequest` args and broke the SDK — evidence that cross-repo compatibility pinning is manual and fragile.
* **Plugins/skills:** `@openhands/extensions` bumped to `0.18.0` (`package.json:26`) is bundled at build time; `buildBundledSkills():722` injects public skills directly and sets `load_public_skills:false` to avoid server cloning. Old servers that still expect the clone silently ignore it. Skill `compatibility` string is not enforced by the frontend beyond pill display (`src/components/features/skills/build-skill-pills.tsx:101`).

**4. Does compatibility rely on policy alone or executable tests?**

Mixed — **policy dominates, with a narrow executable core:**

* **Executable:** `src/api/agent-server-compatibility.test.ts:14` covers version parsing/fallback (6 cases, including `sdk_version` fallback); `__tests__/package-library.test.ts:29,88` locks `exports` shape and exact-pinned versions; `src/mocks/settings-handlers.ts:1047-1124` mirrors the real PATCH validation so `npm run dev:mock` exercises the `misc_settings` merge path; `src/api/no-direct-agent-server-calls.test.ts` (guard cited in `AGENTS.md`) fails CI on raw `axios`/`fetch` to `/api/*`; `.github/workflows/ci.yml:77` runs `npm run build:lib` as a build-compatibility gate.
* **Policy:** release-please conventional commits (`release-please-config.json:7`), PR title lint (`.github/workflows/pr.yml` per `AGENTS.md`), exact-pinned deps (`__tests__/package-library.test.ts:88` pattern but only tests the pin, not API compatibility), and `AGENTS.md:546+` prose requiring `handle_deprecated_model_fields` for Python events — which lives in the other repo and is not enforced here. No contract tests for settings schema shape, automation capability set, or event version; no `npm dist-tag` channel test for pre-release upgrades; no backwards-compat matrix in CI.
* **Assessment:** the only enforced compatibility invariant is the single numeric floor. Everything else (header contracts, capability sets, persisted conversation shapes) is covered by convention and mocked happy-path tests.

## Architectural Decisions

* **Single numeric floor as compatibility model** — `config/defaults.json:9` `minimumAgentServer: "1.28.0"` plus semver comparison in `src/api/agent-server-compatibility.ts:243` is the sole version gate. Tradeoff: simple to reason about and enforce at startup, but coarse — a non-breaking server minor that changes tool names would still pass the floor.

* **`misc_settings` generic container for frontend-owned prefs** — `src/api/settings-service/settings-service.api.ts:47` introduces `MiscSettings{app_preferences?}` as a deep-merged container so future categories (`ui_preferences`, etc.) add siblings without new top-level keys. `transformApiResponse:321` hoists back to flat `Settings`. Tradeoff: extensible wire shape, but requires both frontend and agent-server to implement identical deep-merge (mismatch risk if server changes merge semantics).

* **Bundled skills with `load_public_skills:false`** — `src/api/agent-server-adapter.ts:722,780` ships `@openhands/extensions` `SKILLS_CATALOG` in the JS bundle and tells the server to skip its own clone. Tradeoff: eliminates clone latency and `EXTENSIONS_REF` drift, but couples frontend and extensions versions — a stale frontend ships stale skills until re-published.

* **Capability-as-set-membership not versioned protocol** — `usable_tools: string[]` (`src/api/agent-server-compatibility.ts:27`) and `DeploymentCapabilities{features, triggerKinds}` (`src/manifests/manifest-capabilities.ts:7`) are string sets compared via `includes`. `getCachedAgentServerInfo` gracefully degrades to `true`/`"unknown"` when the server omits data. Tradeoff: tolerant to older servers, but typo-sensitive and invisible until a feature silently disappears.

* **Release-please + conventional commits as versioning oracle** — `release-please-config.json:7` with `include-component-in-tag:false` and `.agents/skills/release.md:25` derives semver from PR titles; `skip-changelog:true` delegates notes to GitHub Releases. Tradeoff: frictionless automation, but no file to audit breaking changes offline.

## Notable Patterns

* **Runtime version probe with cached result** — `loadAgentServerInfo()` caches `cachedAgentServerInfo` + `host` (`src/api/agent-server-compatibility.ts:31,397`) and short-circuits local probe for `cloud` backends to avoid CORS (`:324`). Pattern is replicated for health polling (`use-backends-health.ts:78`).

* **Split encrypted/plaintext settings planes** — `GET /api/settings` with `X-Expose-Secrets: encrypted` for conversation start (`src/api/settings-service/settings-service.api.ts:430`) vs redacted `"**********"` for display, with 5-minute in-memory cache split (`redacted` vs `encrypted`: `src/api/settings-service/settings-service.api.ts:162`). Prevents redacted values leaking into `POST /api/conversations`.

* **Header-based version telemetry** — `AGENT_CANVAS_CLIENT_HEADERS` (`src/api/client-source.ts:17`) attached to every `localAutomationAxios` request (`src/api/automation-service/automation-service.api.ts:83`) and cloud proxy calls, retained as Datadog facets server-side.

* **Graceful degradation for discovery** — `useSetupCapabilities:44` returns `{supported:"unknown", capabilities:null}` when `GET /api/automation/capabilities` fails, allowing manifests to render with their own defaults. Contrast with the hard fail for `minimumAgentServer`.

* **Exact-pinned dependencies as reproducibility gate** — `__tests__/package-library.test.ts:88` enforces `/^\d+\.\d+\.\d+/` for all direct deps, exceptions only for `@openhands/extensions` and `@openhands/typescript-client`. Overrides pinned in `package.json:186`.

## Tradeoffs

* **Coarse floor vs precise contract:** one semver floor avoids maintaining a matrix, but a breaking change that keeps the same major still slips through the floor check. Fix would be per-feature capability flags — currently only tools and a few flags use that model.

* **Bundling extensions vs server cloning:** bundling makes skills version-pinned to the frontend release, reducing server variability but increasing release coupling; server cloning would allow skill updates without a frontend release at cost of latency and `EXTENSIONS_REF` consistency.

* **`misc_settings` extensibility vs migration burden:** generic container future-proofs the wire shape, but required a coordinated frontend+server change with a one-release localStorage drain that is now deleted — operators who skipped that release would have silently dropped prefs (mitigated by “before either side reached stable” per `AGENTS.md:546`).

* **GitHub Releases vs committed CHANGELOG:** saves churn (`skip-changelog:true`) and lets release-please own the file, but `CHANGELOG.md:8` is frozen at alpha and `npm view` is the only offline source for breaking notes.

* **5 s probe timeout vs full health check:** short timeout (`src/api/agent-server-compatibility.ts:15`) avoids blocking UI, but a cold-started Python agent-server (50 MB uvx download, 30–90 s per `AGENTS.md:electron` notes) would fail the probe and show `AgentServerUnavailableError` until the proxy is up, even though the underlying server will soon be ready.

## Failure Modes / Edge Cases

* **Stale cached `/server_info` masks host mismatch** — `getCachedAgentServerInfo({host})` (`src/api/agent-server-compatibility.ts:141`) returns null only when called with a non-matching host; callers that omit `host` (`src/api/client-source.ts:4`) read whatever host populated the cache. Tab with two local backends could render the wrong `usable_tools` set and silently drop `browser_tool_set`/`task_tool_set` from conversation payloads (`src/api/agent-server-adapter.ts:631`).

* **`parseAgentServerVersion:273` strict `major.minor.patch` check** — `dev-build`, `1.42`, or `v1.42.1+build.1` without patch component returns `null` and is treated as `UnknownVersion` not `Unsupported`. A dev server identifying as `1.42.1-dev` would still parse (splits on `-`, takes `core`), but `unknown` string is explicitly trapped (`src/api/agent-server-compatibility.ts:199`), so an operator can’t distinguish a malformed version from a missing one.

* **`sdk_version` fallback hides version mismatch** — `getRawAgentServerVersion:188` falls back from `version` to `sdk_version`. If a proxy injects a synthetic `version` without `sdk_version`, the displayed badge (`src/components/features/backends/backend-version.tsx:7`) and floor check diverge — display could show `1.28.0` while the actual SDK is older.

* **`compareAgentServerVersions:261` prerelease ordering** — `1.28.0` vs `1.28.0-rc.1` compares via `localeCompare` on the prerelease string, which is lexicographic not semver-aware (`rc.10 < rc.2`). Pre-release servers would be mis-classified as unsupported or supported depending on lexical order.

* **Deep-merge for `misc_settings_diff` assumes two-level shape** — `src/api/settings-service/settings-service.api.ts:70` and mock `src/mocks/settings-handlers.ts:1114` merge only one nested level (`app_preferences`). A future third-level category would be shallow-merged and sibling fields at depth 2 would be clobbered.

* **Cloud settings flat keys vs local nested diff** — `saveSettings:644` builds `cloudPayload.app_preferences` as flat keys while local expects `misc_settings_diff.app_preferences`. A change to use the same code path for both kinds without the branch would send flat keys to a local server that validates `*_diff` presence and returns 400.

* **`agentClientProtocol` transitive pin** — `config/defaults.json:13` `agent-client-protocol<0.11` is consumed only by `dev-safe.mjs` uvx install and mock-ACP venv, not by `package.json` overrides. A new contributor using a different install path could get `acp 0.11.0` and hit `PromptRequest` validation errors (`ACP error: 2 validation errors`) with no frontend warning.

* **No persisted-conversation version gate** — starting a conversation against a server that enabled `client_tools` caching (`src/api/agent-server-adapter.ts:1111` comment about `ClientToolSchemaConflictError`) will fail if the frontend changed a tool schema without restarting the dev server; the error is opaque and not mapped to a compatibility message.

## Future Considerations

* Add an explicit deprecation/migration doc (`docs/compatibility.md`) and encode the N-1 support window for `minimumAgentServer` so operators know how long a server version stays valid after a frontend major.
* Promote the `exports` contract test (`__tests__/package-library.test.ts:29`) to a semver-breaking CI gate that fails on `exports` removal/change without a `BREAKING CHANGE` footer.
* Replace lexicographic prerelease compare (`src/api/agent-server-compatibility.ts:267`) with semver-aware prerelease numeric comparison or delegate to a `semver` library.
* Version automation capabilities (e.g., `capabilities.api_version`) so `assessCapabilityRequirements` can branch on capability schema, not just string presence; log client version header mismatches server-side.
* Add an integration test that replays a persisted `DirectConversationInfo` with `schema_version=6` against the current adapter to catch drift when `software-agent-sdk` changes conversation tags or agent kinds.

## Questions / Gaps

* No evidence found for a formal deprecation policy, migration guide, or breaking-change detection beyond conventional-commit major — searched `CHANGELOG.md`, `docs/`, `release-please-config.json`, `.github/pull_request_template.md:62`, `AGENTS.md` (only localStorage drain note). If a policy lives outside this repo (e.g., `software-agent-sdk` ADR), it is not linked here.
* No evidence for backwards-compatibility tests for persisted conversations, event streams, or prompt/tool schemas — searched `__tests__/`, `src/mocks/`, `tests/e2e/` for `schema_version` migration tests; only the settings `misc_settings` merge is mocked.
* No evidence for feature-flag or capability version negotiation beyond set membership — searched `X-*Version`, `feature_flags`, `usable_tools`, `DeploymentCapabilities`; version fields are strings but not negotiated.
* Compatibility expectations differ by surface (frontend floor is executable, automation discovery is best-effort `"unknown"`, cloud branching is policy in `settings-service.api.ts:451`/`cloud/settings-service.api.ts`) but are not documented as a matrix — confirmed by absence of any `docs/compatibility*` or `specs/*` compatibility spec.
* Trace/prompt versioning not inspectable in this repo — `AGENTS.md:Repository Map` assigns trace/prompt persistence to `software-agent-sdk`; no trace JSON schemas were found under `src/`.

---

Generated by `Dimension 24.03: API Versioning and Compatibility` against `openhands`.

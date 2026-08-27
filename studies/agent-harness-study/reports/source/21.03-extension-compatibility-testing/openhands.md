# Source Analysis: openhands

## 21.03 Extension Compatibility Testing

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript + React (Vite, React Router 7), Vitest, `@openhands/extensions` 0.18.0, `@openhands/typescript-client` 1.38.1 |
| Analyzed | 2026-08-27 |

## Summary

`openhands` is the Agent Canvas frontend; its primary extension surface is not a generic plugin SDK but two declarative contracts published by the sibling `OpenHands/extensions` repo and consumed as a version-pinned npm package (`@openhands/extensions@0.18.0`): setup entries (`SetupEntry`/`SetupBlock`) that drive automation creation, and the `AUTOMATION_INTERFACE` manifest that drives navigation, page copy, attributes, endpoints, and dashboard composition. Both contracts are treated as untrusted cross-repo data with host-owned admission validators that fail closed on version mismatch. Conformance is proven against live contract fixtures shipped by the extensions package, supplemented by synthetic factory data and bundle-packing checks. Plugins/MCP/skills use a thinner dynamic catalog path with no comparable conformance harness. Stability is enforced mechanically (pinned dependency + build-time snapshot + semver checks + compatibility floor) rather than via a published stability/breaking-change policy document.

## Rating

**7/10 — Clear model with tests, explicit interfaces, and operational safeguards; not yet mature/durable with public stability guarantees or uniform coverage across all extension kinds.**

Rationale: `src/manifests/` implements a full trust-boundary conformance suite (two validators, registry gate, derivation parity tests against live fixtures) with fixtures and factory examples. This earns the "clear model with tests" tier. Points deducted for (a) no published stability/SLA or breaking-change changelog for extension authors, (b) plugin/MCP/local-skill extension points have only service smoke tests, no contract fixtures, and (c) examples for extension authors live as test factories/fixtures rather than documented, runnable scaffolds.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Conformance — Setup entry validator (trust boundary) | `validateSetupEntry` enforces markup-free copy, semver template versions, single trigger kind, repo-picker presence for event triggers, bundle path/command containment, and rejects any unknown keys; version fails closed. | `src/manifests/manifest-validation.ts:576-622` |
| Conformance — Interface validator | `validateInterfaceManifest` enforces `INTERFACE_VERSION` pin, exact route match against `mountedRoutes`, closed-key objects, docs URL prefix, attribute type/required pin to host dialog, `createBundle`/`uploads` optionality for backward compat. | `src/manifests/interface-validation.ts:709-757` |
| Conformance — Setup contracts tested vs live fixtures | `automation-setup.test.ts` imports 3 published JSON fixtures (`github-pr-reviewer`, `github-repo-monitor`, `incident-retrospective-drafter`) and asserts `buildCreatePayload`/`buildPreflightBody`/`buildAssistedMessage` and error-map derivation equal the fixture-pinned request bodies. | `__tests__/manifests/automation-setup.test.ts:92-240` |
| Conformance — Interface contracts tested | `interface-validation.test.ts` covers ~15 base invariants + 8 sub-page invariants (markup injection, route mismatch, endpoint `{id}` placement, icon/metric/filter/sort validity, whole-surface atomicity) and multi-error reporting. | `__tests__/manifests/interface-validation.test.ts:39-361` |
| Conformance — Setup validation edge cases | `manifest-validation.test.ts` covers 20+ rejection cases including shell metacharacter entrypoints, path traversal (`../`), `config.json` collision, placeholder namespace, trigger collision — and admits semver prerelease/build suffixes. | `__tests__/manifests/manifest-validation.test.ts:70-421` |
| Conformance — Registry gate | `createSetupRegistry` skips card-only entries, warns and drops failed-admission entries, deduplicates by `id` first-wins, never renders partial UI. | `src/manifests/manifest-registry.ts:19-52` , `__tests__/manifests/manifest-registry.test.ts:9-77` |
| Conformance — Bundle packing | `packBundle` asserts packed files come from `getAutomationBundleFiles` (package content, not repo), executable bits, and `config.json` parity with `buildCreatePayload` template provenance. | `src/manifests/manifest-bundle.ts:68-104` , `__tests__/manifests/manifest-bundle.test.ts:77-177` |
| Conformance — Capabilities matrix | `assessCapabilityRequirements` checked against `capabilities.json` fixture matrix (deployment shapes × blockedBy lists). | `__tests__/manifests/manifest-capabilities.test.ts:86-125` |
| Extension types/interfaces | `SetupEntry`/`SetupBlock`/`SetupBundle`/`InterfaceManifest` + `INTERFACE_VERSION`/`SETUP_VERSION` + closed value sets for field types, trigger kinds, metrics, icons — host owns shape, extensions assign without adapter. | `src/manifests/types.ts:13-489` |
| Test fixtures — live contract fixtures | `BUNDLES` loaded from `@openhands/extensions/testing/automations/*.json` (3 fixtures) with preflight/create/conversation exchanges and `capabilities.json` deployment shapes. | `__tests__/manifests/automation-setup.test.ts:2-4` , `__tests__/manifests/manifest-capabilities.test.ts:2-5` |
| Test fixtures — factory examples | `createSetupEntry`/`createSetup`/`createInterfaceManifest`/`createInterfaceManifestWithSubPages` factories provide minimal admissible manifests with distinct copy to detect fallback leakage. | `__tests__/manifests/manifest-test-data.ts:14-255` |
| Example implementations | `examples/acp-docker/` runnable Docker-compose for ACP agents (pinned vs `latest` paths, env generation from `config/defaults.json`). | `examples/acp-docker/README.md:1-99` , `examples/acp-docker/docker-compose.yml:9-25` |
| Example — interface surface docs | `automation-interface.ts` documents seam: host serves no automation-specific datum without an admitted manifest; `hasAutomationInterface()` gate. | `src/manifests/automation-interface.ts:1-17` , `src/manifests/automation-interface.ts:84-106` |
| Stability — version pin & build snapshot | `@openhands/extensions` exact-pinned `0.18.0`; `PUBLIC_SKILLS` is immutable build-time snapshot; `SKILLS_CATALOG`/`AUTOMATION_CATALOG` baked at `vite build`. | `package.json:26` , `src/api/skills-service.ts:29-34` , `src/manifests/manifest-sources.ts:1-32` |
| Stability — fail-closed version checks | `setup.version !== SETUP_VERSION` and `candidate.version !== INTERFACE_VERSION` short-circuit with explicit errors; template/bundle versions validated as full semver (including prerelease/build). | `src/manifests/manifest-validation.ts:587-592` , `src/manifests/interface-validation.ts:716-721` , `src/manifests/manifest-validation.ts:37-38` |
| Stability — agent-server compatibility floor | `MINIMUM_COMPATIBLE_AGENT_SERVER_VERSION` from `config/defaults.json:compatibility.minimumAgentServer` (1.28.0) enforced by `assertAgentServerVersionIsSupported` with semver parse + prerelease precedence, cached `/server_info`. | `src/api/agent-server-compatibility.ts:18-19` , `src/api/agent-server-compatibility.ts:243-318` , `config/defaults.json:8-10` |
| Stability — no explicit breaking-change policy | `CHANGELOG.md` delegates to Keep-a-Changelog + SemVer statement; `.github/release.yml` groups by `type:` labels; `.agents/skills/release.md` derives next version from conventional commits but does not promise extension-specific stability window. | `CHANGELOG.md:1-6` , `.github/release.yml:1-14` , `.agents/skills/release.md:25` |
| Plugin extension point (weak coverage) | `PluginsService` delegates to typed `PluginsClient`/`FileClient`; no fixtures, no versioned contract — empty catalog fallback on cloud/missing endpoint. | `src/api/plugins-service.ts:67-148` , `src/api/plugins-service.test.ts:35-159` |
| Skill extension point | `SkillsService` merges `SKILLS_CATALOG` (public) + `SkillsClient.getSkills({load_public:false})` (user/project); skills are `SKILL_CATEGORY_IDS` from extensions but host has no skill-conformance harness. | `src/api/skills-service.ts:36-67` |

## Answers to Dimension Questions

### 1. Are extension contracts tested?

**Yes — thoroughly for the two declarative automation contracts, weakly for other extension points.** The manifest layer treats extension data as untrusted and tests it as a conformance boundary:

* `validateSetupEntry` and `validateInterfaceManifest` together enforce ~40 invariants (markup-free copy, semver provenance, placeholder namespaces, trigger uniqueness, file/path containment, route/endpoint/metric/icon allowlists, attribute pinning, sub-page atomicity). Any violation rejects the whole manifest (`valid:false`, errors array) — no partial render (`src/manifests/manifest-validation.ts:576-622`, `src/manifests/interface-validation.ts:709-757`).
* Derivation correctness is pinned against live contract fixtures published by `OpenHands/extensions` (`@openhands/extensions/testing/automations/*.json`). `automation-setup.test.ts` asserts that `buildCreatePayload`, `buildPreflightBody`, and `buildAssistedMessage` produce byte-equal bodies to those fixtures, and that the derived error map recovers payload-path→field mappings (`__tests__/manifests/automation-setup.test.ts:132-240`). The fixtures were verified against the live automation service with `extra="forbid"` create models, so divergence would be a 422 in production.
* Capabilities compatibility is matrix-tested: every catalog entry × every deployment shape from `capabilities.json` is checked that `assessCapabilityRequirements` agrees with the fixture's `blockedBy` list (`__tests__/manifests/manifest-capabilities.test.ts:86-125`).
* Bundle packing has its own conformance suite: tar contents, `config.json` provenance parity, and executable-bit assignment (`__tests__/manifests/manifest-bundle.test.ts:77-177`).

For the other extension points — marketplace plugins (`src/api/plugins-service.ts:76-117`), MCP integrations, and skills (`src/api/skills-service.ts:36-67`) — only behavioral delegation tests exist (e.g., `plugins-service.test.ts:35-159` asserts client call counts and fallbacks). There are no fixture-pinned contract tests for plugin manifests, MCP server specs, or skill frontmatter. Extension authors of those kinds cannot run a local conformance harness in this repo.

### 2. Are fixtures provided for extension authors?

**Partially — fixtures exist for automation setup/interface authors, but as package-published contracts rather than a standalone test harness.**

* Live fixtures are published alongside the extensions package under `@openhands/extensions/testing/automations/` and imported directly here (`incident-retrospective-drafter.json`, `github-pr-reviewer.json`, `github-repo-monitor.json`, `capabilities.json`) (`__tests__/manifests/automation-setup.test.ts:2-4`). An extension author can read the same JSON in the installed npm package; in `agent-canvas` the `vite.config.ts:36-43` resolves the extensions skills directory from `node_modules/@openhands/extensions` at build time, so fixtures are always on disk for local validation, but the repo does not ship a CLI like `npx check-extension ./my-skill`.
* Reusable factory fixtures (`__tests__/manifests/manifest-test-data.ts:14-255`) provide minimal admissible manifests and helper `createSetupEntryWith`/`createInterfaceManifestWith` for fault-injection testing. These are test-only exports (not published from `src/`), yet they document the minimal valid shape an author can copy.
* No dedicated `fixtures/` directory for external authors; `src/fixtures/` contains only `home-automations-demo` unrelated to extension authoring. Skill/plugin authors rely on `getAutomationBundleFiles` bridging the repository-file indirection (`src/manifests/manifest-bundle.ts:29-39`) without a scaffold generator.

Verdict: fixtures exist and are authoritative, but ergonomics for an outside author (“copy this folder, run this command”) are indirect.

### 3. Are examples provided?

**Yes — but examples are test-resident rather than a documented example repo, except for ACP container usage.**

* The factory manifests in `__tests__/manifests/manifest-test-data.ts:14-255` serve as canonical minimal examples: `createSetup` shows a `cron` trigger + `repo-picker` + `text` arg wiring; `createInterfaceManifest` shows routes/navigation/pages/attributes/importExport/endpoints with distinct copy values so authors can distinguish manifest data from host defaults. `createInterfaceManifestWithSubPages` shows the complete sub-page surface (overview tiles, filters, sort, insights, templates).
* The published catalog entries themselves are executable examples once `@openhands/extensions` is installed; `src/manifests/manifest-sources.ts:18-32` and `src/manifests/automation-interface.ts:1-17` explain that the catalog is the single source and no wiring is needed to add a new entry.
* End-to-end runnable examples exist for ACP extension (custom agents): `examples/acp-docker/README.md:1-99` walks through a pinned vs `latest` container setup, credential materialisation, and `VITE_BACKEND_BASE_URL` wiring. This is the only `examples/` directory in the repo.
* No `examples/skills/` or `examples/plugins/` scaffolds; skill authoring guidance lives in the external `OpenHands/extensions` repo (`AGENTS.md:33,41` notes that skills/automations/integrations belong there).

### 4. Are stability guarantees documented?

**Mechanically yes, documentarily no.** Stability is enforced by code and config rather than by a published SLA:

* **Exact pin + build snapshot:** `package.json:26` pins `@openhands/extensions@0.18.0`; `src/api/skills-service.ts:29-34` and `src/manifests/manifest-sources.ts:1-21` snapshot `SKILLS_CATALOG`/`AUTOMATION_CATALOG` at build time — updating the catalog requires bumping the dep and rebuilding, so a deployed Canvas cannot break from a live extensions drift.
* **Fail-closed versioning:** `SETUP_VERSION = "1.0"` (`src/manifests/types.ts:13`) and `INTERFACE_VERSION = "1.0"` (`src/manifests/types.ts:230`) are checked first in both validators; a format the host does not recognise is refused rather than interpreted with current rules (`src/manifests/manifest-validation.ts:587-592`, `src/manifests/interface-validation.ts:716-721`). Bundle/template versions are validated as full semver (`src/manifests/manifest-validation.ts:37-38`), with prerelease/build suffixes admitted as forward-compatible provenance.
* **Agent-server floor:** `config/defaults.json:8-10` declares `compatibility.minimumAgentServer: "1.28.0"`, enforced by `assertAgentServerVersionIsSupported` with proper semver precedence and `AGENT_SERVER_UNSUPPORTED_VERSION`/`UNKNOWN_VERSION` error states surfaced in the UI (`src/api/agent-server-compatibility.ts:18-19`, `src/api/agent-server-compatibility.ts:293-399`).
* **Backward-compatible gaps:** `interface-validation.ts:68-73` explicitly marks `createBundle`/`uploads` endpoints as optional so a package predating bundles is still admitted; `manifest-sources.ts:28-32` reads `AUTOMATION_INTERFACE` reflectively so a package that predates the export yields `undefined` without a build error.
* **What is missing:** No `STABILITY.md`, `API.md`, or deprecation schedule. `CHANGELOG.md:1-6` claims SemVer, `.github/release.yml:1-14` and `.agents/skills/release.md:25` describe conventional-commit-driven releases via `release-please`, but neither promises a notice window or retention window for extension-deprecated fields. Breaking changes to the manifest shape would surface as console warnings (`Rejected a setup manifest:` in `src/manifests/manifest-registry.ts:32`, `Rejected the automation interface manifest:` in `src/manifests/automation-interface.ts:75`) and a silent fallback (missing nav/routes), not as a versioned migration guide.

## Architectural Decisions

* **Host as trust boundary, not consumer:** Extension data is `unknown[]` at ingestion (`src/manifests/manifest-sources.ts:19`, `src/manifests/manifest-registry.ts:20`), validated without importing the publisher's schema — the host does not trust types that shipped alongside the data it validates. This decision isolates validation from publisher drift but forces the host to duplicate the shape declaratively (contrast with a shared zod schema).
* **All-or-nothing admission:** One bad field rejects the whole manifest and reverts to host defaults/silent drop rather than rendering a partially-trusted mix (`src/manifests/manifest-validation.ts:1-16`, `src/manifests/interface-validation.ts:1-9`, `src/manifests/automation-interface.ts:65-82`). This prevents injection/namespace-traversal bugs from being partially applied, at the cost of visibility (author sees only `console.warn` aggregation).
* **Build-time catalog snapshot via npm version pin:** Rather than fetching `OpenHands/extensions` at runtime, the Canvas bakes `AUTOMATION_CATALOG`/`SKILLS_CATALOG`/`AUTOMATION_INTERFACE` at `vite build` (`src/api/skills-service.ts:29-34`, `src/manifests/manifest-sources.ts:18-21`, `vite.config.ts:36-43`). This makes Canvas deployments hermetic and cacheable, at the cost that new skills/automations require a dep bump + rebuild rather than appearing instantly.
* **Derivation ownership in host:** Interfaces like `SetupEntry` mirror `extensions` declarations so entries assign without an adapter (`src/manifests/types.ts:1-11`), but everything derivable (request body, routes, analytics, review screen) is generated in `src/manifests/automation-setup.ts:1-13` as the single automation-aware module. This keeps other `manifests/` files generic but centralises a 434-line derivation that mirrors Python `_render_payload` witnessed by contract fixtures.
* **Typed-client gate for plugin/MCP:** Local plugin/MCP calls go through `@openhands/typescript-client` with helper-assembled options, and cloud plugins short-circuit to empty (`src/api/plugins-service.ts:76-117`). This enforces API version coupling via the client dep rather than raw `fetch`, trading off that a new plugin endpoint requires a client release before Canvas can consume it (`AGENTS.md` API Access Rules).

## Notable Patterns

* **Error-code translation over literal messages:** Local validation returns `{code:"required"}` etc. so the host can translate, while manifest-authored copy is literal and markup-checked (`src/manifests/manifest-local-validation.ts:26-32`, `src/manifests/manifest-validation.ts:28`, `src/manifests/interface-validation.ts:29`). This separates author-owned vs host-owned strings.
* **Placeholder namespace allowlist:** Only `{{form.*}}` and `{{automation.*}}` placeholders are permitted; any other `{{` fails admission (`src/manifests/manifest-validation.ts:46-49`), and `BUNDLE_SOURCE_PATTERN` restricts bundle file sources to `skills/` or `automations/` (`src/manifests/manifest-validation.ts:44-45`). This prevents exfiltration/SSRF via config interpolation.
* **Whole-surface atomicity for sub-pages:** The sub-page surface (routes/templates/nav/pages overview/filters/sort/insights) must be declared whole or not at all (`src/manifests/interface-validation.ts:475-498`) — a partial declaration would render navigation to a missing page.
* **Reverse-mapped error attribution:** `deriveErrorMap` re-derives the request with `{{form.<field>}}` stand-ins and walks the payload to map service rejection paths back to form fields (`src/manifests/automation-setup.ts:387-433`), so authors don't declare the mapping and can't mis-map it.
* **First-wins deduplication with loud rejection:** Duplicate `id` is rejected with `console.warn` rather than last-wins (`src/manifests/manifest-registry.ts:37-42`), preventing author-impersonation by later catalog entries.

## Tradeoffs

* **Pin + rebuild vs live fetch:** Pinning `@openhands/extensions@0.18.0` guarantees hermetic builds and a reviewable diff per catalog change, but delays new extensions and requires cross-repo release coordination; a live fetch would give instant availability but break hermeticity and require runtime signature/trust handling not currently implemented.
* **Strict fail-closed vs graceful degradation:** Rejecting the whole manifest on one bad field maximises safety (no injection, no partially-rendered dashboard) but hides progress — authors see an aggregated warnings list (`interface-validation.test.ts:348-361` asserts multi-error reporting) with no per-field UI in production beyond `console.warn`. A field-level fallthrough would be more forgiving but risk rendering a misleading partial surface.
* **Browser-side validation depth vs duplication:** The host duplicates much of the service's `extra="forbid"` validation client-side so preflight can run on blur without a round trip (`src/manifests/manifest-local-validation.ts:1-11`). This is fast and defensive but duplicates logic that must be kept in sync with the Python mirror in `OpenHands/extensions` — contract fixtures are the only sync mechanism.
* **Harness coverage bias:** Investment is concentrated on declarative manifests; imperative plugins/MCP/skills have thin service tests. This concentrates verification where the security boundary is highest, but leaves plugin contract regressions to be found in `mock-llm` E2E or production rather than at admission.

## Failure Modes / Edge Cases

* **Stale pinned package silently withholds new extensions:** Because the catalog is baked, a published extension can be live in `OpenHands/extensions` for days before Canvas users see it — there is no staleness warning/bubble in the UI beyond the next dep bump.
* **Whole-interface fallback is invisible:** A single `docsUrl` prefix mismatch (`src/manifests/interface-validation.ts:730-734` requires `https://docs.openhands.dev/`) or unknown top-level key (`src/manifests/interface-validation.ts:724`) nulls `ADMITTED` (`src/manifests/automation-interface.ts:84`) and makes `hasAutomationInterface()` false — nav entries disappear and routes 404 without a user-facing “your interface is rejected” banner, only `console.warn`.
* **Bundle endpoints absent on old interface:** A manifest predating bundles is admitted, but `missingCreateEndpoints` (`src/manifests/automation-setup.ts:86-89`) will block bundle creation at form render with an error the host holds no path to handle — the author must upgrade the pinned extensions version, not the manifest.
* **Fixture drift:** Fixtures are generated by the extensions repo's `_render_payload` path; a host derivation that uses a different placeholder interpolation (e.g., `interpolateValue` vs `interpolateText` semantics in `src/manifests/automation-setup.ts:328-347`) would pass local validation but fail with a 422 from the live service — fixtures are the sole guard.
* **Path traversal via dot segments:** `isRelativePath` explicitly rejects `.`/`..` segments despite them matching `BUNDLE_PATH_PATTERN` (`src/manifests/manifest-validation.ts:148-154`), but an author could still declare a deeply-nested allowed path that extracts to a confusing layout — no depth limit is enforced.
* **Placeholder injection via `{{automation.setup}}`:** Caught by rendering as empty text rather than recursing (`__tests__/manifests/manifest-bundle.test.ts:125-135` asserts `{{automation.setup}}` resolves to `""`), but a future author adding a new namespace would need both type and validator updates or admission will reject all their manifests.
* **Plugin fallback hides regressions:** `PluginsService` swallows endpoint absence/unreachability and returns `[]` (`src/api/plugins-service.ts:86-90`, `102-116`), so a broken `PluginsClient.getPluginsMarketplace` on a local backend looks identical to “no marketplace plugins” — no toast/error boundary surfaces the failure, and there is no conformance fixture to pin the catalog shape.

## Future Considerations

* Publish a stability surface doc: freeze `InterfaceManifest`/`SetupEntry` shapes per `INTERFACE_VERSION`/`SETUP_VERSION`, promise a deprecation window (e.g., one minor with warnings before removal), and expose `REJECTED_MANIFEST_COUNT` telemetry. The current mechanical guarantees (pin + fail-closed) are sound but not communicable to external authors.
* Extract a runnable extension conformance CLI (`npx @openhands/agent-canvas check-extension ./my-automation --fixtures`) that reuses `validateSetupEntry`/`validateInterfaceManifest` + `assessCapabilityRequirements` + `packBundle` so authors can verify outside the Canvas Vitest run — today the harness is coupled to `vitest` + `node_modules/@openhands/extensions` resolution.
* Promote test-only factories in `__tests__/manifests/manifest-test-data.ts` to a `src/manifests/testing/` export (or publish as `@openhands/extensions/testing` already does for live fixtures) so examples are importable without copying test files.
* Add visible admission diagnostics: surface `InterfaceValidationResult.errors`/`SetupValidationResult.errors` in a dev-only panel or `data-testid` rather than only `console.warn`, so authors and operators can distinguish “manifest rejected” from “no manifest published”.
* Extend fixture pinning to plugins/MCP: publish a plugin catalog fixture (analogous to `capabilities.json`) and assert `PluginsService` parsing against it; otherwise the plugin extension contract will remain un-pinned and regressions will escape to E2E.

## Questions / Gaps

* **No evidence of a breaking-change communication channel for extension authors:** `.github/release.yml:1-14`, `.agents/skills/release.md:25`, and `CHANGELOG.md:1-6` describe Canvas releases, not extension contract evolution. Search for `BREAKING`/`stability` in docs and `src/manifests` found only validator comments (`manifest-validation.ts:32-33`) and the standard SemVer mention. *Search boundary:* `grep stability|breaking|BREAKING` across `docs/`, `.github/`, `src/manifests/`, `config/`.
* **No runtime conformance for skill frontmatter:** `SKILL_CATEGORY_IDS` imported from `@openhands/extensions/skills` (`__tests__/utils/skill-category.test.ts:2`) and `SKILL_CATALOG` mapping (`src/api/skills-service.ts:12-24`) show no validation harness comparable to manifests. *Gap:* an extension author cannot verify a new skill's `triggers`/`category`/`content` shape locally beyond the build succeeding.
* **No MCP interface-version equivalent:** MCP marketplace entries are sourced via `MCP_MARKETPLACE` from `@openhands/extensions/integrations` (`__tests__/utils/mcp-marketplace-utils.test.ts:11`) but the host patches entries procedurally (`AGENTS.md` `getMcpMarketplaceCatalog` patches for Linear/GitHub) rather than through a versioned, validated interface. Whether a patch is still needed under a given extensions version is not contract-tested.
* **Example discoverability:** `examples/acp-docker/README.md:1-99` is discoverable, but `__tests__/manifests/manifest-test-data.ts:78-139` examples are not linked from `docs/` or `src/manifests/README`. *Gap:* an extension author grepping `docs/` will not find the minimal manifest shape without reading test code.

---

Generated by `Dimension 21.03: Extension Compatibility Testing` against `openhands`.

# Source Analysis: openhands

## Prompt Templating and Variable Contracts

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (`@openhands/agent-canvas`) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19 + Vite + react-router (frontend SPA; agent loop lives in the separate `software-agent-sdk` repo) |
| Analyzed | 2026-08-25 |

*All file paths below are relative to the source root `studies/agent-harness-study/sources/openhands/`.*

## Summary

This source is the OpenHands **frontend** ("Agent Canvas"); it composes but does not execute prompts. Three distinct parameterization layers exist, each with its own contract model:

1. **System-message suffix rendering** — `buildRuntimeServicesSystemSuffix()` (`src/api/agent-server-adapter.ts:215-300`) hand-renders a `<RUNTIME_SERVICES>` markdown block from a parsed-and-validated `RuntimeServicesInfo` object and attaches it to conversation-start payloads as `AgentContext.system_message_suffix` (`src/api/agent-server-adapter.ts:784-786`).
2. **A purpose-built manifest template engine** — `src/manifests/manifest-template.ts` implements plain `{{namespace.path}}` substitution (no expression language) for extension-authored automation *prompts*, event filters, bundle configs, and setup messages. It is backed by an admission-time trust boundary (`src/manifests/manifest-validation.ts`), form-value validation with an escaping guard (`src/manifests/manifest-local-validation.ts`), fixture-pinned contract tests (`__tests__/manifests/automation-setup.test.ts`), and error-to-field mapping (`deriveErrorMap`, `src/manifests/automation-setup.ts:403-434`).
3. **i18next interpolation for UI copy** — ~2,265 `{{...}}` placeholders across 15 locales in `src/i18n/translation.json`, with React-side escaping (`src/i18n/index.ts:52-60`) and test-enforced placeholder parity across locales (`__tests__/i18n/translation-completeness.test.ts:118-135`). This layer never reaches the LLM.

The strongest variable-contract engineering is in layer 2: namespaces are declared constants (`SETUP_PLACEHOLDER_NAMESPACES`, `src/manifests/types.ts:30`), unknown placeholder namespaces are rejected at admission (`src/manifests/manifest-validation.ts:46-49,112-117`), rendered outputs are pinned against live-service-verified fixtures, and per-metric placeholder allowlists exist even for dashboard copy (`src/manifests/interface-validation.ts:311-330`). Weaknesses: missing variables substitute silently as empty strings with no observability, i18next call-site option objects are untyped (`as never` cast at `src/i18n/index.ts:115`), and values interpolated into agent-bound prompt text are not neutralized against natural-language injection (a deliberate but only comment-documented trust decision).

## Rating

**7 / 10**

Rationale against the rubric:

- **Clear model**: two purpose-built engines plus one library engine, each scoped to a distinct surface — no general-purpose template library is smuggled in (no handlebars/mustache/ejs in `package.json` dependencies; verified by dependency scan). The manifest engine explicitly documents its design stance: "Interpolation is plain substitution — there is no expression language, so a setup block cannot express behavior here" (`src/manifests/manifest-template.ts:4-8`).
- **Tests**: fixture-pinned rendered-request tests (`__tests__/manifests/automation-setup.test.ts:225-241,333-347`), suffix-rendering and payload-integration tests (`__tests__/api/agent-server-adapter.test.ts:1214-1410`), escaping-guard tests (`__tests__/manifests/manifest-local-validation.test.ts:65-74`), cross-locale placeholder-parity tests (`__tests__/i18n/translation-completeness.test.ts:118-135`), and admission-rejection tests for unknown namespaces (`__tests__/manifests/manifest-validation.test.ts:80`).
- **Explicit interfaces**: declared namespace constants, typed scopes (`SetupScope`, `src/manifests/manifest-template.ts:14-18`), typed field constraints (`SetupFieldConstraints`, `src/manifests/types.ts:37-45`), per-metric allowlists (`OVERVIEW_TILE_PLACEHOLDERS`, `src/manifests/types.ts:350-361`).
- **Operational safeguards**: admission-time rejection of whole manifests on any failed check (`src/manifests/manifest-validation.ts:15-17`), markup ban (`MARKUP_PATTERN`, `:27-28`), expression-literal breakout guard, message length cap (`MAX_MESSAGE_LENGTH = 2000`, `:64-65`), server-side preflight of every draft.
- **Why not 8-10**: missing-variable behavior in all three layers degrades silently to blank output without warnings or telemetry; i18n variable contracts are not checked at call sites; three parallel templating idioms impose consistency cost; `manifest-template.ts` has no dedicated unit-test file (coverage is indirect via consumers); values substituted into LLM-bound prompts are unescaped by design with no documented threat model statement beyond code comments. The manifest subsystem alone would score 8; the surrounding layers keep the composite at 7.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Template engine (system suffix) | Hand-rolled line-array markdown renderer producing `<RUNTIME_SERVICES>` block; no template library used | `src/api/agent-server-adapter.ts:215-300` |
| Input schema / parsing | `RuntimeServicesInfo` interface; `parseRuntimeServicesInfo` recursively JSON-parses strings, rejects non-objects, requires `services` key | `src/api/agent-server-adapter.ts:128-151,153-173` |
| Variable injection point | Suffix attached to `agent_context.system_message_suffix` only when defined; omitted otherwise | `src/api/agent-server-adapter.ts:784-786` |
| Missing-field fallbacks | Fallback descriptions when backend omits `description`; explicit "Automation backend: not running" line when automation service absent | `src/api/agent-server-adapter.ts:242,248,254,260,275-279` |
| Suffix tests | Renders block, anchors don't-guess warning to actual URL, absent-automation case, malformed-JSON case, payload integration | `__tests__/api/agent-server-adapter.test.ts:1214-1366,1368-1410` |
| Template engine (manifests) | Custom `{{dotted.path}}` engine: `PLACEHOLDER_PATTERN`, `getByPath`, type-preserving `interpolateValue`, string-only `interpolateText`/`interpolateValues` | `src/manifests/manifest-template.ts:12,21-26,65-73,76-90` |
| Explicit variable namespaces | `SETUP_PLACEHOLDER_NAMESPACES = ["form", "automation"]`; scope interface `SetupScope { form?, automation? }` | `src/manifests/types.ts:29-30`, `src/manifests/manifest-template.ts:14-18` |
| Namespace validation (admission) | `UNKNOWN_PLACEHOLDER_PATTERN` fails any `{{` not opening a known namespace; enforced by `SetupChecker.placeholders/templateCopy/templateValue` | `src/manifests/manifest-validation.ts:46-49,112-133` |
| Prompt template validation | Direct-mode entries must declare exactly one of `prompt` or `bundle`; `setup.prompt` and `setup.filter` validated as templated request-body strings | `src/manifests/manifest-validation.ts:514-528,549-555` |
| Bundle config validation | Every string leaf of `config.json` tree validated for placeholders; non-strings type-checked | `src/manifests/manifest-validation.ts:478-506` |
| Escaping / injection prevention (markup) | `MARKUP_PATTERN = /<[A-Za-z/!]/` — "Copy must never be able to inject markup into the host"; enforced on all user-visible copy | `src/manifests/manifest-validation.ts:27-28,101-110` |
| Escaping / injection prevention (expressions) | Opt-in `format: "safeExpressionLiteral"` constraint rejects `" ' \` characters that break out of expression string literals (e.g. JMESPath filters); manifests cannot supply their own regexes | `src/manifests/manifest-local-validation.ts:24,149-154`, `src/manifests/types.ts:40-45` |
| Type-preservation guard | Whole-placeholder templates preserve resolved list shape (`"repos": "{{form.repositories}}"` → array); object graphs cannot be injected into payloads | `src/manifests/manifest-template.ts:52-73` |
| Value validation before interpolation | required/minLength/maxLength/invalidOption checks over declared fields; multi-value fields validated per entry | `src/manifests/manifest-local-validation.ts:116-164` |
| Missing-variable behavior (manifests) | `toText()` returns `""` for missing/non-scalar values — silent blank substitution, documented as intentional | `src/manifests/manifest-template.ts:35-37` |
| Prompt rendering call site | `buildCreatePayload` renders automation `prompt` via `interpolateText(setup.prompt, scope)`; event filter interpolated into trigger | `src/manifests/automation-setup.ts:185-225,272-280` |
| Config tree rendering | `interpolateConfig` recurses arrays/objects, templates only string leaves; numbers/booleans/null pass through typed | `src/manifests/automation-setup.ts:323-347` |
| Assisted-setup seed message | Skill command + interpolated `setup.message` joined into conversation seed; message capped at 2000 chars ("can never become a channel for runtime instructions") | `src/manifests/automation-setup.ts:375-385`, `src/manifests/manifest-validation.ts:64-65,389` |
| Error-to-field mapping | `deriveErrorMap` re-builds payload with `{{form.<name>}}` stand-ins to recover which field produced each payload path; service 422s mapped back to inputs | `src/manifests/automation-setup.ts:387-434`, `__tests__/manifests/automation-setup.test.ts:365-386,409-425` |
| Contract fixtures | Rendered request bodies pinned to live-service-verified fixtures from `@openhands/extensions/testing`; divergence = production 422 | `__tests__/manifests/automation-setup.test.ts:62-96,225-241` |
| Per-metric placeholder allowlist | Dashboard tile copy may embed only placeholders its metric exposes (`{{active}}` for `automations` metric, none for others); violations rejected at admission | `src/manifests/types.ts:350-361`, `src/manifests/interface-validation.ts:311-330` |
| Tile copy rendering | `interpolateValues(template, value.placeholderValues)` with host-computed metric values | `src/routes/automations-list.tsx:58,302`, `src/manifests/automation-insights.ts:231-301` |
| i18n template engine | i18next + react-i18next; loadPath itself is a template `/locales/{{lng}}/{{ns}}.json` | `src/i18n/index.ts:1-6,50` |
| i18n escaping decision | `interpolation.escapeValue: false` because React escapes at render time (avoids double-escaping paths like `~/.codex/auth.json`); justified claim: translated strings never render via `dangerouslySetInnerHTML` (verified: sole occurrence renders internal CSS keyframes, not content) | `src/i18n/index.ts:52-60`, `src/components/shared/text-shimmer.tsx:72-76` |
| i18n variable scale | ~2,265 `{{...}}` occurrences across 15 locales (e.g. `{{count}}` ×540, `{{name}}` ×510, `{{path}}` ×105) | `src/i18n/translation.json` (aggregate count) |
| i18n key contract | Generated `I18nKey` enum from translation.json via build script; ESLint `i18next/no-literal-string` (`jsx-only` mode + attribute includes) forces all UI copy through keyed `t()` calls | `scripts/make-i18n-translations.cjs`, `eslint.config.js:212-232` |
| i18n placeholder-parity contract | Test requires every locale to carry exactly English's set of `{{...}}` placeholders per key | `__tests__/i18n/translation-completeness.test.ts:20-21,118-135` |
| i18n call-site gap | `t(key, options)` proxy casts options `as never` — TypeScript does not verify callers supply each placeholder value; some call sites locally re-declare `escapeValue: false` | `src/i18n/index.ts:113-116`, `src/components/features/launch/plugin-launch-modal.tsx:206-210`, `src/components/features/backends/backend-form-modal.tsx:167` |
| Initial-message assembly | Query + conversation instructions joined `\n\n` (concatenation, not templating); automation launch prompt passes skill trigger command through verbatim | `src/api/agent-server-adapter.ts:679-695`, `src/utils/automation-catalog.ts:76-93` |
| Skill content flow | Bundled skills mapped 1:1 into `agent_context.skills` (name/content/trigger/source); no client-side template processing of skill bodies; disabled skills filtered by name deny-list | `src/api/agent-server-adapter.ts:703-747,749-788`, `src/api/skills-service.ts:36-64` |
| Suffix persistence semantics | Profile editor merge deliberately preserves `system_message_suffix` across saves (whole-profile overwrite would otherwise wipe it) | `src/components/features/settings/agent-profiles/merge-agent-profile-save-input.ts:6-16,22-30` |
| Known deferred templating | TODO acknowledges default conversation title should become `{{shortId}}` interpolation but is kept literal | `src/api/agent-server-adapter.ts:310-315` |

## Answers to Dimension Questions

### 1. How are prompts parameterized?

Three mechanisms, matched to surface risk:

- **Agent-bound system suffix**: imperative string building over a validated data object. `buildRuntimeServicesSystemSuffix()` pushes fixed prose lines interleaved with values read off `RuntimeServicesInfo` (`src/api/agent-server-adapter.ts:221-299`). The input is normalized first — JSON-string forms are recursively parsed and shape-checked by `parseRuntimeServicesInfo()` (`src/api/agent-server-adapter.ts:153-173`) — so the renderer never sees raw untrusted text.
- **Agent-bound automation prompts/filters/configs**: extension-authored templates stored as declarative data, rendered at launch time by the manifest engine. `buildCreatePayload()` renders `setup.prompt` with `{ form: values, automation: entry }` scope (`src/manifests/automation-setup.ts:196-203`); event-trigger `filter` strings are interpolated the same way (`:272-280`); bundle `config.json` trees are walked leaf-by-leaf (`interpolateConfig`, `:328-347`). Rendered prompts go to the automation service, whose runs feed them to the agent.
- **UI copy (never LLM-bound)**: i18next interpolation through `t(I18nKey.X, { var })` calls, with keys constrained to a generated enum and literals banned in JSX by lint (`eslint.config.js:212-232`).

There is deliberate non-parameterization too: skill contents and the initial user message are passed through verbatim (`src/api/agent-server-adapter.ts:736-745`, `:679-695`); the frontend performs no variable expansion inside skill bodies — trigger matching and system-prompt injection of skills happen server-side in the SDK.

### 2. Are variable contracts explicit?

**For the manifest subsystem, yes — unusually so.** The allowed placeholder namespaces are a published constant (`SETUP_PLACEHOLDER_NAMESPACES = ["form", "automation"]`, `src/manifests/types.ts:30`); the scope is a typed interface (`SetupScope`, `src/manifests/manifest-template.ts:14-18`); admission rejects any `{{...}}` outside those namespaces outright rather than rendering it (`src/manifests/manifest-validation.ts:46-49,112-117` — "A manifest that fails any check is rejected outright", `:15-17`). Even dashboard tile copy has a per-metric allowlist: a tile for a metric exposing no placeholders may not contain `{{` at all (`src/manifests/interface-validation.ts:322-328`). Which fields exist is declared per-entry in the form schema (`SetupFormField`, `src/manifests/types.ts:47-63`), and `deriveErrorMap()` mechanically proves the bidirectional link between fields and rendered payload paths by rebuilding the payload with `{{form.<name>}}` stand-ins (`src/manifests/automation-setup.ts:403-434`).

**Partially elsewhere.** The runtime-services suffix's inputs have a documented interface (`RuntimeServicesInfo`, `src/api/agent-server-adapter.ts:121-151`) including a legacy-key migration note for `vite` → `frontend` (`:139-141`), but it is hand-checked rather than schema-validated. For i18n, the *key* contract is strong (generated enum + lint + completeness tests), and cross-*locale* placeholder parity is test-enforced (`__tests__/i18n/translation-completeness.test.ts:118-135`), but the *call-site* contract is implicit: options are cast `as never` (`src/i18n/index.ts:115`), so forgetting a variable compiles cleanly.

### 3. Is missing-variable behavior predictable?

Predictable, but in the weakest possible sense: everything silently degrades to blank.

- Manifest engine: `toText()` maps missing values, objects, and unknown shapes to `""` ("Missing values render as blank; callers that care show their own fallback", `src/manifests/manifest-template.ts:35-37`). No warning is logged and no test pins a specific missing-field render; predictability comes from the simplicity of the rule, not from feedback.
- Mitigations make this mostly safe in practice: local validation rejects blanks in `required` fields before rendering (`src/manifests/manifest-local-validation.ts:122-126`), defaults seed undeclared-but-empty state (`:104-114`), and the authoritative preflight endpoint validates the fully rendered draft server-side before creation (`buildPreflightBody`, `src/manifests/automation-setup.ts:353-365`).
- Runtime-services suffix: absence is handled structurally, not silently — optional lines are omitted, descriptions get hardcoded fallbacks, and a missing automation entry produces an explicit "not running" sentence instead of nothing (`src/api/agent-server-adapter.ts:239-279`). Malformed input yields `undefined`, which suppresses the suffix entirely (`:218-219`), pinned by tests (`__tests__/api/agent-server-adapter.test.ts:1215-1238`).
- i18next: default library behavior (missing value → empty substitution); the repo adds no handler or logging for it. A regression note shows awareness of the failure class — a test mock "returns the key and drops interpolation values" (`src/components/features/automations/git-sync/git-sync-activity-row.test.tsx:5`).

### 4. Are variables properly escaped?

The repo draws a layered, mostly well-reasoned boundary:

- **Markup injection (host-rendered copy)**: banned at the source. All manifest-authored visible copy must match no `/<[A-Za-z/!]/` pattern or the entire manifest is rejected (`src/manifests/manifest-validation.ts:27-28,101-110`). This protects the host DOM rather than relying on downstream sanitization.
- **Expression-context injection (server-side filters)**: guarded opt-in. Fields consumed inside JMESPath-style filter expressions can declare `format: "safeExpressionLiteral"`, which rejects `"`, `'`, and `\` — the characters that would break out of an expression string literal (`src/manifests/manifest-local-validation.ts:24,149-154`). Critically, manifests supply only the *name* of the check from a closed set, "so they cannot hand the host a pathological pattern" (`src/manifests/types.ts:40-45`). Request-body strings are exempted from the markup rule because they are "never rendered" (`src/manifests/manifest-validation.ts:124-133`) — a correct distinction between display surfaces and transport surfaces.
- **Type confusion**: prevented structurally. A whole-placeholder template resolves to the value's own type only if it is a string list; anything else flattens to text, so "a manifest cannot state one value and put its own object graph into the request body" (`src/manifests/manifest-template.ts:52-64`).
- **HTML escaping (UI)**: delegated to React by disabling i18next's escaper, with an accurate inline justification and a verified invariant (the only `dangerouslySetInnerHTML` in `src/` emits internal CSS keyframes, `src/components/shared/text-shimmer.tsx:72-76`).
- **Prompt injection into the LLM: not addressed.** Form values are interpolated verbatim into agent-bound prompt and filter text (`src/manifests/automation-setup.ts:202`); there is no neutralization of instruction-like content, and the size cap applies only to manifest-authored messages, not user-entered values (`MAX_MESSAGE_LENGTH`, `src/manifests/manifest-validation.ts:64-65`). Given these are user-configured automations running under the user's own authority, this is defensible, but the trust model ("what a manifest is permitted to do", `src/manifests/manifest-validation.ts:4-8`) is expressed only in code comments — no doc ties it together.

## Architectural Decisions

- **No third-party template engine for prompt surfaces.** Both agent-bound templaters are in-house and minimal (`src/api/agent-server-adapter.ts:215-300`, `src/manifests/manifest-template.ts`). This eliminates a whole class of template-engine injection (no expression evaluation exists anywhere in the pipeline) at the cost of reimplementing interpolation (~90 lines).
- **Admission-time rejection over render-time tolerance.** Manifest problems fail the whole catalog entry before any UI or request exists: "It never renders a partial UI, because everything downstream treats its content as instructions" (`src/manifests/manifest-validation.ts:15-17`). Validation is a trust boundary between repos, deliberately not deferring to any schema shipped with the manifests being validated (`:7-8`).
- **Host-owned derivation, manifest-owned variation.** Templates declare only what varies (`prompt`, `filter`, `message`, bundle config); everything derivable — request shape, endpoints, error mapping, analytics — is generated by the host (`src/manifests/automation-setup.ts:1-14`). The Python reference implementation in the extensions repo is mirrored, and divergence is caught by fixtures rather than hope (`__tests__/manifests/automation-setup.test.ts:8-13,62-70`).
- **Escape-at-render for UI, constrain-at-source for data.** i18next escaping is off because React owns HTML safety (`src/i18n/index.ts:52-60`), while data-bound surfaces get source-level bans and character-class checks. Each layer's escaping strategy matches its consumer.
- **Suffix as opt-in enrichment, not always-on prompt surgery.** The `<RUNTIME_SERVICES>` block is attached only when the backend advertises runtime services (`src/api/agent-server-adapter.ts:784-786`), and profile-save merges explicitly preserve it so whole-profile overwrites don't strip it (`src/components/features/settings/agent-profiles/merge-agent-profile-save-input.ts:6-16`).

## Notable Patterns

- **Self-describing error maps**: building the payload twice — once with real values, once with `{{form.x}}` stand-ins — and diffing paths recovers the field→payload mapping with zero per-entry declaration (`src/manifests/automation-setup.ts:387-434`). Server 422 errors land on the exact input that caused them (`__tests__/manifests/automation-setup.test.ts:365-386`).
- **Fixture-as-contract testing**: rendered request bodies are pinned to JSON fixtures that were "verified against the live service," so a template-engine change that alters output fails CI as a would-be production 422 (`__tests__/manifests/automation-setup.test.ts:62-96,225-241,333-347`).
- **Negative-space assertions**: tests assert what must *not* appear — no hardcoded `:8000` in the don't-guess line, no "Vite frontend" label for static builds, no alternate auth header (`__tests__/api/agent-server-adapter.test.ts:1268,1291-1293,1310-1311`).
- **Allowlist-not-denylist placeholder validation**: unknown-placeholder detection is a negative lookahead over known namespaces (`src/manifests/manifest-validation.ts:47-49`) and per-metric exposed names (`src/manifests/interface-validation.ts:323-326`), so new namespaces fail closed until admitted.
- **Documented escape hatches**: every deviation (escapeValue off, legacy `vite` key, literal title instead of `{{shortId}}`) carries a comment explaining why and what would re-enable the cleaner path (`src/api/agent-server-adapter.ts:139-141,310-315`).

## Tradeoffs

- **Silent-blank substitution vs. fail-fast**: `""` for missing variables keeps the engine total (renders never throw mid-launch) but hides authoring mistakes; the safety net is validation + preflight + error mapping, all of which run only on interactive setup flows — a programmatically assembled payload skips local validation entirely.
- **Three templating idioms vs. one shared engine**: hand-built suffix lines, `{{namespace.path}}` manifests, and i18next coexist. Each fits its surface (trusted internal data / untrusted cross-repo data / localized copy), but a contributor must learn which contract applies where, and improvements (e.g., missing-value warnings) would need triple implementation.
- **Plain substitution vs. expressiveness**: no filters, conditionals, or formatting exist in the manifest engine. The extensions repo compensates by moving logic into skills themselves ("An automation whose skill declares no command has the request spelled out instead", `src/utils/automation-catalog.ts:79-85`), keeping the template language trivially auditable.
- **Verbatim interpolation into LLM-bound text vs. injection hardening**: maximal fidelity of user intent into automation prompts, zero protection against instruction-shaped values. Consistent with a single-user, self-hosted threat model, but a liability if automation catalogs ever run with elevated or shared authority.
- **Test-enforced locale parity vs. call-site typing**: parity tests guarantee translations stay substitutable, but nothing guarantees call sites pass the variables — the residual bug class (missing option → blank in UI) remains open.

## Failure Modes / Edge Cases

- **Malformed runtime-services input** (non-JSON string, missing `services`): parse returns `null` → suffix omitted entirely; conversation starts without service topology and the agent may probe wrong ports — the exact failure the block exists to prevent (`src/api/agent-server-adapter.ts:153-173,218-219`; tested `__tests__/api/agent-server-adapter.test.ts:1232-1238`).
- **Legacy `vite` frontend key**: accepted during a one-release migration window (`src/api/agent-server-adapter.ts:134-141`); a stale launcher emitting the old key still renders correctly.
- **Manifest referencing an undeclared form field**: passes namespace validation (namespace is checked, not field existence), interpolates to `""` silently; caught only if the field was also `required` (blank-form rejection) or if preflight rejects the resulting draft.
- **Quote-bearing values near expressions**: rejected up front with `unsafeExpressionLiteral` when the field declares the constraint (`__tests__/manifests/manifest-local-validation.test.ts:65-74`); the historical catalog case was removed when the affected entry moved to cron, and the constraint survives only via synthetic-test coverage — noted honestly in a comment (`__tests__/manifests/automation-setup.test.ts:388-394`).
- **Whole-profile overwrite wiping prompt config**: mitigated by the kind-aware merge that preserves `system_message_suffix` et al.; a kind switch intentionally does NOT carry the old variant's fields because the server union is `extra="forbid"` (`src/components/features/settings/agent-profiles/merge-agent-profile-save-input.ts:13-16`).
- **Locale drift**: a translator adding/dropping a `{{var}}` breaks the parity test per key+locale (`__tests__/i18n/translation-completeness.test.ts:118-135`), preventing runtime blanks in localized UI.
- **Bundle config type erosion**: non-string leaves are written through untouched so scripts read native types; only string leaves are templated, so `"timeout": "{{form.timeout}}"` would arrive as a *string* — an authoring foot-gun the docs acknowledge implicitly by typing config leaves (`src/manifests/automation-setup.ts:323-347`, `src/manifests/types.ts:76-83`).

## Future Considerations

- Add an admission check that every `{{form.<name>}}` referenced by `prompt`/`filter`/`message`/bundle config names a declared field — closing the undeclared-field→silent-blank gap described above; `collectPlaceholderFields()` already extracts the names (`src/manifests/automation-setup.ts:387-393`).
- Emit a dev-mode warning (or record telemetry) when `toText()` drops a non-blank-capable value, making missing-variable behavior observable instead of merely deterministic.
- Type i18next options: derive a per-key variable map from `translation.json` during `make-i18n-translations.cjs` so `t()` calls are checked at compile time, replacing the `as never` cast (`src/i18n/index.ts:113-116`).
- Give `manifest-template.ts` direct unit tests (whole-placeholder typing, dotted-path traversal, array joining) instead of relying on consumer-level coverage.
- Document the prompt-injection trust model (verbatim user values into agent prompts) in `specs/` alongside the existing spec-ID convention, since it is currently inferable only from comments.

## Questions / Gaps

- **Where is the authoritative agent system prompt assembled?** Out of this source's scope: the SDK (`software-agent-sdk`) owns final system-prompt composition and skill injection (`src/api/agent-server-adapter.ts:697-701` describes the hand-off; AGENTS.md confirms the repo split). This analysis covers the frontend's contributions only; "No evidence found" within this repository for core prompt-template storage.
- **Does the automation service re-validate rendered prompts?** Preflight validates drafts (`POST /v1/validate`, `src/manifests/types.ts:213-218`) and create models are `extra="forbid"` (`src/manifests/automation-setup.ts:12-13`), but whether the service inspects prompt *content* could not be verified from this source — the service lives in another repo.
- **Is i18next's missing-value behavior pinned by any test?** No test in `__tests__/i18n/` exercises a `t()` call with a missing variable; behavior rests on library defaults. Searched `__tests__/i18n/*` and `src/i18n/*` for such coverage.
- **Any runtime guard against oversized rendered prompts?** `MAX_MESSAGE_LENGTH` caps only manifest-authored messages (`src/manifests/manifest-validation.ts:64-65`); rendered `prompt` size depends solely on field `maxLength` constraints, which manifests may omit. No aggregate limit found in this source.

---

Generated by dimension `12.02: Prompt Templating and Variable Contracts` against `openhands`.

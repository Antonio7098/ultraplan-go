# Source Analysis: openhands

## Docs, Examples, and Contributor Workflow (Dimension 22.03)

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (`@openhands/agent-canvas`, "Agent Canvas") |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19, Vite 8, React Router 7, Vitest + Playwright; Node >=22.12 (`package.json:19-21`); small Python surface for CI scripts and E2E mock servers |
| Analyzed | 2026-08-24 |

All file paths below are relative to the source root `studies/agent-harness-study/sources/openhands/`.

## Summary

OpenHands (Agent Canvas) is a frontend-centric repo with an unusually strong, CI-enforced contributor operating model — but a deliberately narrow documentation scope. The canonical contributor guide is not a `CONTRIBUTING.md` (none exists) but a massive `AGENTS.md` (693 lines) that pairs every rule with a concrete enforcement mechanism: API-access rules backed by a guard test (`src/api/no-direct-agent-server-calls.test.ts`, referenced from `AGENTS.md:349-354`), i18n rules enforced by ESLint (`AGENTS.md:445-449`), and PR-template requirements validated by a Python checker in CI (`.github/scripts/check_pr_description.py:39`). A dedicated `docs/DEVELOPMENT.md` covers dev workflows end to end, including mutation testing (`docs/DEVELOPMENT.md:124-149`) and embedding/customization of the published library (`docs/DEVELOPMENT.md:151-177`). Tutorials are strong for what lives in this repo — onboarding external ACP agents (`docs/ACP_AGENTS.md`), self-hosting, release smoke testing (`docs/TESTING_MATRIX.md`).

The gaps are structural. Examples cover exactly one extension type (a containerized ACP agent server, `examples/acp-docker/README.md:1-99`); there are no in-repo examples or tutorials for authoring skills, tools, automations, MCP integrations, evals, memory, tracing, or policies — those are explicitly routed to sibling repos via a repository-placement table (`AGENTS.md:22-41`) and out-of-repo doc links (`src/constants/skills-docs.ts:1-2`). No generated API reference exists: the public library surface is documented only through TypeScript declaration emit (`npm run build:lib`, `package.json:105`) and an exports map (`package.json:206-248`). No template/starter repos are present.

**Bottom line on the dimension's guiding question** ("Can a new contributor add a tool in under an hour without asking for help?"): for a *frontend/UI/library* change, yes — bootstrap script, one-command full-stack dev launcher, mock mode, and layered docs make this realistic. For adding an agent *tool* (or skill, eval, automation), no — this repo will redirect them to `OpenHands/software-agent-sdk` or `OpenHands/extensions` (`AGENTS.md:33,41`) without providing starter material here.

## Rating

**6 / 10** — Present but inconsistent coverage.

Rationale against the rubric:

- What exists is high quality and unusually well enforced (CI-guarded contribution rules, spec-ID traceability with 51 grep-able `@spec` tags across `src/` and `__tests__/`, PR/issue readiness gates, three E2E tiers documented in `AGENTS.md:203-308`). That clears the "4–6" floor comfortably and touches the "clear model with tests and operational safeguards" band for the repo's own scope.
- It does not reach a durable 7–8 for this dimension because: examples cover 1 of ~8 extension types the dimension cares about; API documentation is neither generated nor browsable (declarations only); there is no conventional `CONTRIBUTING.md` entry point (discoverability risk for human contributors who don't expect guidance in `AGENTS.md`); and no template/starter projects exist.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Contributor guide (de facto) | `AGENTS.md` acts as the contributor handbook: repo-placement map, API access rules, testing rules, i18n rules | `AGENTS.md:1-693` (map at `:22-41`; API rules `:349-443`; testing rules `:309-328`; magic-string rules `:445-505`) |
| No CONTRIBUTING.md / CODE_OF_CONDUCT.md | Filesystem search at repo root and depth 2 found no `CONTRIBUTING*` or `CODE_OF_CONDUCT*` | (search boundary: root + `.github/`) |
| Development guide | Local workflow (`npm run dev` full stack via `uvx`), env-var tables, version pinning overrides | `docs/DEVELOPMENT.md:5-79`, `:179-194` |
| Mutation testing documented | Stryker usage incl. incremental/diff modes and report path `reports/mutation.html` | `docs/DEVELOPMENT.md:124-149` |
| Library embedding tutorial | `AgentServerUIProviders` usage sample with `styleOverrides`; CSS isolation strategy | `docs/DEVELOPMENT.md:151-177`; architecture context `docs/architecture.md:58-60` |
| Docs index | All seven docs cross-linked (architecture, ACP agents, development, self-hosting, DefenseClaw, testing matrix) | `docs/README.md:1-10` |
| Quickstart tutorials | Three install paths (npm global, Docker sandbox, from source) with prerequisites | `README.md:50-122`; Windows variant `README.windows.md:1` |
| ACP agents tutorial | Full walkthrough: mermaid protocol diagram, provider table, auth matrix, switching/model flows | `docs/ACP_AGENTS.md:17-27`, `:33-47`, `:49-80` (239 lines total) |
| Only example directory | `examples/acp-docker` = containerized ACP agent-server quick start (README + docker-compose.yml) | `examples/acp-docker/README.md:1-99`; compose wiring `:13-31` |
| Extension authoring delegated elsewhere | Placement table routes skills/automations/MCP to `OpenHands/extensions` and endpoints/tools to `software-agent-sdk` | `AGENTS.md:29-41` |
| Skill-authoring docs are external links | UI constants point to hosted docs and an example command cloning from the extensions repo | `src/constants/skills-docs.ts:1-7` |
| API docs NOT generated | No typedoc/doc-gen anywhere in scripts; library surface = `.d.ts` emit + package exports map | `package.json:74-115` (no doc script), `:105` (`build:lib`), `:206-248` (exports map) |
| Runtime OpenAPI pointer only | Automation backend advertises `docs_url`/`openapi_url` via `/server_info.runtime_services` | `AGENTS.md:142-176` (shape + example block) |
| Inline JSDoc with examples | Utility modules carry `@example` blocks (informal, not rendered to a doc site) | e.g. `src/utils/utils.ts:42,56,166`; `src/hooks/use-telemetry.ts:44` |
| Spec-driven traceability | Stable-ID spec files; code/tests tagged `// @spec LLD-001 …` (51 occurrences found via grep) | `specs/llm-defaults.md:3-8`; `specs/backend-management.md:3-11`; rule stated in `AGENTS.md` Additional Notes |
| PR template with HUMAN/AGENT contract | Draft-first template; HUMAN section reserved for humans; AGENT section must show end-to-end evidence | `.github/pull_request_template.md:1-16`, fields `:18-58` |
| PR description validator | Requires Why/Summary/How-to-Test, ≥20-char human note, screenshots for frontend/bug PRs, issue linkage with `ready-for-dev` label, type/label consistency | `.github/scripts/check_pr_description.py:1-39`, checks `:387` ff.; wired via `.github/workflows/pr-description-check.yml` |
| Issue readiness gate | Auto-manages `ready-for-dev` label: bugs need repro method + screenshot + acceptance criteria; enhancements need desired behavior + acceptance criteria | `.github/workflows/issue-readiness-check.yml:2-13` |
| CI quality gates | Matrix CI (ubuntu+windows) on all PRs/pushes; separate live-E2E, mock-LLM E2E, Docker E2E, desktop workflows | `.github/workflows/ci.yml:3-37`; workflow inventory `.github/workflows/` (20 files) |
| Pre-commit hooks + lint-staged | Husky pre-commit runs lint-staged: eslint/prettier, staged typecheck, translation-completeness check | `.husky/pre-commit:1`; `package.json:116-127` |
| Agent bootstrap script | `.openhands/setup.sh`: installs uv, `npm ci`, seeds `.env`, generates i18n declarations | `.openhands/setup.sh:9-28`, `:39` |
| Annotated env template | Every var commented with behavior and cross-links to examples/docs | `.env.sample:1-30` |
| Release smoke-test matrix | P0/P1/P2 priorities across Install×OS×Agent and Automations matrices | `docs/TESTING_MATRIX.md:3-34` |
| Release process guide | Trunk-based release-please flow as an agent-invocable skill with triggers | `.agents/skills/release.md:1-31` |
| Review guidelines skill | Mandatory APPROVE-or-COMMENT review policy; eval-risk PRs must not be auto-approved | `.agents/skills/custom-codereview-guide.md:9-24` |

## Answers to Dimension Questions

**1. Are contribution guides clear?**
Yes — exceptionally, once found. `AGENTS.md` is the de facto contributor handbook and every normative claim is paired with its enforcement mechanism: the "no raw fetch to agent-server" rule names the exact failing test (`AGENTS.md:349-354`), the i18n rules name the ESLint rules that fail (`AGENTS.md:445-449`), and the PR contract is machine-checked (`.github/scripts/check_pr_description.py:39`, required fields `"Why", "Summary", "How to Test"`). The placement table (`AGENTS.md:22-41`) prevents wasted work by telling contributors up front which changes belong in sibling repos. Weakness: no `CONTRIBUTING.md`, so a newcomer following GitHub conventions finds nothing; discoverability depends on knowing to read `AGENTS.md`. `docs/DEVELOPMENT.md` is task-clear (run, test, mutate, embed) but thin on "how to add X" recipes.

**2. Are examples comprehensive?**
No. Exactly one runnable example exists — `examples/acp-docker` (containerized ACP agent server with credential onboarding, `examples/acp-docker/README.md:11-93`) — and it is good: pinned-image reproducibility via `config/defaults.json`, teardown notes, and gotcha callouts. But it covers a single extension type (external ACP agents). There are no in-repo examples for skills, automations, MCP integrations, library embedding apps, custom backends, or eval harnesses; embedding is covered only as prose-plus-snippet in `docs/DEVELOPMENT.md:157-175`. By design, most extension authoring lives in sibling repos (`AGENTS.md:33,41`), so comprehensiveness inside this repo is structurally capped.

**3. Is API documentation available?**
Not generated. `package.json` contains no doc-generation tooling (`package.json:74-115`); there is no TypeDoc, no docs site source, no rendered reference. The public npm surface is defined mechanically: subpath exports with `types` entries (`package.json:206-248`) plus declaration files emitted by `build:lib` (`package.json:105`), which gives IDE-level docs but nothing browsable or narrated. Informal JSDoc `@example` blocks exist in utilities (`src/utils/utils.ts:42,56,166`) but are not compiled into any artifact. The only OpenAPI exposure referenced is runtime-provided by the automation backend (`docs_url`/`openapi_url` advertised through `/server_info.runtime_services`, `AGENTS.md:142-176`) — that documents the backend's automation API, not this repo's own interfaces. Architecture-level boundaries are documented instead (`docs/architecture.md:5-43`).

**4. Are there tutorials for key tasks?**
For this repo's actual tasks, largely yes: running the stack (`README.md:63-118`, `docs/DEVELOPMENT.md:5-54`), pointing at alternative agent-server versions (`docs/DEVELOPMENT.md:56-72`), mocking (`docs/DEVELOPMENT.md:104-108`), embedding/theming the library (`docs/DEVELOPMENT.md:151-177`), onboarding external ACP agents including auth edge cases (`docs/ACP_AGENTS.md:49-80`), self-hosting (`docs/SELF_HOSTING.md`), and pre-release QA (`docs/TESTING_MATRIX.md:3-34`). For the dimension's extension types (tools, evals, memory, tracing, policies), no tutorials exist here — searches for "how to add skill/tool" patterns in `docs/` returned nothing, and the UI itself links outward for skill authoring (`src/constants/skills-docs.ts:1-2`). Template/starter repos: **No evidence found** within the source; searches for "starter"/"boilerplate"/template projects produced no relevant hits (search boundary: all tracked `.md`/`.json`/`.ts` excluding lockfile).

## Architectural Decisions

1. **Docs-as-enforcement over docs-as-prose.** Contribution rules live beside their CI tripwires rather than in narrative guides — the API rule cites its guard test (`AGENTS.md:354`), and the PR contract is a Python validator run in CI (`.github/scripts/check_pr_description.py:39`). This trades convention-compliance (`CONTRIBUTING.md`) for non-circumventability.
2. **Multi-repo separation of concerns.** The repo explicitly scopes itself to the frontend and pushes tools/endpoints to `software-agent-sdk` and skills/automations/integrations to `extensions` (`AGENTS.md:22-41`). Documentation breadth is sacrificed for placement correctness.
3. **Spec-ID traceability.** Behavior contracts are captured as stable-ID specs under `specs/` (`specs/backend-management.md:3-11`) and tagged in code with `// @spec` comments (51 occurrences), making requirement→implementation→test chains greppable.
4. **Library-first packaging.** The same codebase ships as standalone app and embeddable npm library, with the public interface defined purely structurally via exports maps and declaration emit (`package.json:206-248`) rather than a generated reference.

## Notable Patterns

- **HUMAN/AGENT split PR template**: AI contributors must provide execution evidence in a segregated section while a reserved `HUMAN:` section stays untouchable (`.github/pull_request_template.md:3-14`; rule restated at `AGENTS.md:43-49`) — a governance pattern purpose-built for agentic development.
- **Tiered verification ladder documented for contributors**: unit/Vitest → mutation (Stryker diff mode, `docs/DEVELOPMENT.md:130-141`) → mocked Playwright → mock-LLM full-stack E2E with selective execution driven by `test-mapping.json` (`AGENTS.md:223-255`) → live LLM E2E gated behind a label (`AGENTS.md:203-222`) — each tier's cost/trigger is written down.
- **Self-bootstrapping onboarding**: `.openhands/setup.sh` makes "clone and go" deterministic, including generating i18n declarations required before typecheck (`.openhands/setup.sh:9-39`).
- **Operational knowledge encoded as agent skills**: release cutting and code-review policy are packaged as invocable skills with trigger frontmatter (`.agents/skills/release.md:1-8`, `.agents/skills/custom-codereview-guide.md:1-7`).
- **Failure-driven debugging playbook**: `AGENTS.md:270-308` teaches contributors how to read CI artifacts (accessibility-tree snapshots, common locator/view-mode failure modes) rather than just how tests pass.

## Tradeoffs

- **Enforcement vs. convention**: replacing `CONTRIBUTING.md` with `AGENTS.md` maximizes agent compliance but breaks the standard discovery path for human contributors arriving via GitHub's community standards.
- **Repo scoping vs. self-containedness**: routing extension authoring to sibling repos keeps this repo honest (`AGENTS.md:41`) but means the dimension's core question — "can a contributor add a tool safely?" — cannot be answered from inside this source at all; the safety guidance for tool/eval/memory work simply isn't here.
- **Structural API surface vs. learnability**: exports maps + declarations (`package.json:206-248`) give compiler-checked APIs with zero doc-drift, but embedding consumers get no narrative reference beyond one snippet (`docs/DEVELOPMENT.md:166-175`).
- **Doc volume vs. freshness**: the 693-line `AGENTS.md` encodes deep operational state (e.g., telemetry consent flow, Electron packaging traps); the repo mitigates drift with "update this section in the same PR" rules (`AGENTS.md` Live-E2E section), but the single-file monolith concentrates staleness risk.

## Failure Modes / Edge Cases

- **Newcomer dead-end**: a contributor following GitHub's "add a CONTRIBUTING file" suggestion or the README footer (`README.md:139-144`) reaches dev-setup docs but never learns about the mandatory PR contract until CI rejects their PR (the validator posts feedback comments, per `.github/workflows/pr-description-check.yml`).
- **Extension-type expectations mismatch**: someone wanting to add an eval, tool, or tracing hook finds zero in-repo material; if they miss the placement table they may open misplaced PRs (which the review skill then redirects — friction, not prevention).
- **Undocumented API consumption**: because the library surface lacks generated docs, host-app integrators depend on reading `dist/*.d.ts`; breaking-change communication relies on conventional-commit release notes (no `CHANGELOG.md`; noted in `.agents/skills/release.md:27-29`).
- **Example rot**: the lone example pins images through `config/defaults.json` (`examples/acp-docker/README.md:24-38`), which is good, but with n=1 examples there is no structural pressure keeping an examples suite coherent as the product grows.

## Future Considerations

- Add a thin `CONTRIBUTING.md` that points to `AGENTS.md` sections and the PR/issue gates, satisfying convention while preserving enforcement.
- Generate a browsable API reference from the existing `build:lib` declarations (e.g., TypeDoc on `src/lib/index.ts` barrels) — low effort since declaration emit already exists (`package.json:105`).
- Add one minimal embedding example app (consuming `@openhands/agent-canvas/browser` or `/conversation`) to complement `docs/DEVELOPMENT.md:157-175`.
- Mirror the placement table (`AGENTS.md:22-41`) into the README's "More documentation" list so scope redirection is visible before a contributor clones anything.

## Questions / Gaps

- **Hosted docs content unverifiable from this source**: many claims link to `docs.openhands.dev` (e.g., `src/constants/skills-docs.ts:2`, checklist links in `src/components/features/sidebar/sidebar-onboarding-checklist.constants.ts:8,138-145`); whether those pages teach safe tool/skill authoring could not be inspected under source-isolation rules.
- **No evidence found** of template/starter repositories, scaffolding generators, or `npm create`-style init flows anywhere in the tracked tree (searched `starter`, `boilerplate`, `template project` patterns across docs/config/source).
- Whether the sibling repos (`software-agent-sdk`, `extensions`, `typescript-client`) compensate for the missing extension-type tutorials is out of scope for this isolated study and remains unknown.

---

Generated by dimension 22.03 (`studies/agent-harness-study/reports/source/22.03-docs-examples-contributor-workflow` study) against `openhands` (`studies/agent-harness-study/sources/openhands`).

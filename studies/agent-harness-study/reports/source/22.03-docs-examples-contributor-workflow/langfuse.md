# Source Analysis: langfuse

## Dimension 22.03: Docs, Examples, and Contributor Workflow

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript monorepo — Next.js 14 (`web/`), BullMQ worker (`worker/`), shared package (`packages/shared/`), pnpm + Turborepo, Prisma + ClickHouse |
| Analyzed | 2026-08-25 |

## Summary

Langfuse treats contributor enablement as a first-class, CI-enforced system rather than a static document. A conventional human-facing `CONTRIBUTING.md` (578 lines) covers the full loop — environment setup via one-command dev stack (`CONTRIBUTING.md:268-283`), architecture and network diagrams (`CONTRIBUTING.md:64-101`), test strategy against a real database (`CONTRIBUTING.md:342-397`), commit conventions (`CONTRIBUTING.md:338-340`), and API-spec maintenance (`CONTRIBUTING.md:542-560`). Layered on top is an unusually mature tree of AI-agent contributor guides: a canonical `.agents/AGENTS.md` (193 lines) with per-package children (`web/AGENTS.md` at 318 lines, `packages/shared/AGENTS.md`, `worker/AGENTS.md`, plus feature-local guides) whose discovery symlinks are validated in CI by `pnpm run agents:check` (`package.json:11`, `.agents/README.md:151-160`). Step-by-step "Playbooks" exist for the repo's main extension types — public API endpoints (`web/AGENTS.md:258-264`), Postgres/ClickHouse schema changes (`packages/shared/AGENTS.md:106-133`), queue contract changes (`packages/shared/AGENTS.md:134-145`), evaluator data model (`web/src/features/evals/v2/AGENTS.md:1-16`), model-price additions (`CONTRIBUTING.md:530-540` plus the `add-model-price` skill). API documentation is generated, not hand-written: Fern definitions under `fern/apis/**` export OpenAPI specs into the shipped app (`package.json:41`) and a CI workflow auto-generates Python/TS server SDKs on every `fern/**` change (`.github/workflows/sdk-api-spec.yml:5-13`). The main gaps are that user-facing tutorials, examples, and starter content deliberately live outside this repository (separate `langfuse-docs` and SDK repos, `CONTRIBUTING.md:21`); in-repo runnable examples are limited to a seed CLI with ~20 scenarios and framework-trace fixtures. No template/starter repos exist inside this source.

## Rating

**8 / 10** — Clear, tested, and operationally safeguarded contributor model. Contribution guides are explicit and layered (human `CONTRIBUTING.md`; machine `AGENTS.md` hierarchy; 35+ skills), key extension types have step-by-step playbooks, and API docs are generated from source-of-truth Fern definitions with CI automation. Safeguards are enforced mechanically: PR-title linting (`.github/workflows/validate-pr-title.yml:24-38`), agent-shim path validation that fails CI on broken doc links (`.agents/README.md:141-143`), Husky pre-commit hooks (`CONTRIBUTING.md:246-260`), and per-PR full-stack preview deployments (`CONTRIBUTING.md:417-420`). It falls short of 9–10 because: (1) there is no in-repo examples directory or starter/template project — tutorials live in external repos outside study scope; (2) SDK generation requires a signed-in Fern account (`CONTRIBUTING.md:556-560`), adding a paid-tool dependency to the docs workflow; (3) some feature-level guides (e.g., evaluators v2) are terse data-model notes rather than end-to-end tutorials (`web/src/features/evals/v2/AGENTS.md:1-16`). For the rubric question — *can a new contributor add a tool-like extension in under an hour?* — for covered types (public endpoint, model price, seeder scenario, schema migration) the playbooks make that realistic without asking for help.

## Evidence Collected

Every entry cites `path:line` relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Contribution guide | Full contributor handbook: making-a-change flow, issue-first policy, good-first-issue label | `CONTRIBUTING.md:34-40` |
| Contribution guide | Tech-stack inventory (Next.js 14, tRPC, Prisma, Zod, Fern) | `CONTRIBUTING.md:48-62` |
| Architecture overview | Mermaid network diagram of web/worker/Postgres/ClickHouse/Redis/S3 topology | `CONTRIBUTING.md:70-95` |
| Repository structure | Monorepo layout description (web, worker, packages/shared, ee) | `CONTRIBUTING.md:103-113` |
| Dev setup | One-command stack bootstrap `pnpm run dx` + demo credentials + seeder | `CONTRIBUTING.md:268-289` |
| Dev setup | GitHub Codespace/devcontainer option stated | `CONTRIBUTING.md:124` |
| Agent guides (root) | Canonical `.agents/AGENTS.md` with symlinked `AGENTS.md`/`CLAUDE.md` discovery | `.agents/README.md:12-14`, `.agents/README.md:127-142` |
| Agent guides (packages) | Package-local guides: `web/AGENTS.md` (318 lines), `worker/AGENTS.md` (88), `packages/shared/AGENTS.md` (175), `ee/AGENTS.md` (36), feature-local evals guides | `web/AGENTS.md:1-318`, `packages/shared/AGENTS.md:1-175`, `web/src/features/evals/v2/AGENTS.md:1-16` |
| Agent shim enforcement | `agents:sync` / `agents:check` scripts; CI fails if an `AGENTS.md` lacks its generated shim or cites broken paths | `package.json:11-12`, `.agents/README.md:141-143`, `.agents/README.md:147-160` |
| Skills library | 35+ tool-neutral skills incl. `backend-dev-guidelines` (7 reference docs), `clickhouse-best-practices` (25 rule files), `code-review`, `security-review`, `skill-creator` meta-skill | `.agents/skills/backend-dev-guidelines/SKILL.md:1`, `.agents/skills/clickhouse-best-practices/SKILL.md:1`, `.agents/skills/skill-creator/SKILL.md:1`, `.agents/README.md:303-317` |
| Architecture principles | Written principles doc for high-scale observability with "practical defaults for agents" | `.agents/ARCHITECTURE_PRINCIPLES.md:13-49` |
| Playbook: public API | 4-step playbook: route → typed contract → server test → Fern regeneration | `web/AGENTS.md:258-264` |
| Playbook: tRPC endpoint | Router/procedure → register in root → reuse auth patterns → tests | `web/AGENTS.md:250-256` |
| Playbook: DB schema | Postgres migration steps and ClickHouse MV-change rules cross-referenced to skill | `packages/shared/AGENTS.md:106-133` |
| Playbook: queue contracts | Queue payload zod-schema change procedure with backward-compat rule | `packages/shared/AGENTS.md:134-145` |
| Tutorial: model prices | Editing default-model prices incl. ordering/`updated_at` rules + transition-period migration requirement | `CONTRIBUTING.md:530-540` |
| Tutorial: seeder scenarios | Seed CLI usage catalog (~20 scenarios) + rules for adding new scenarios deterministically | `packages/shared/scripts/seeder/AGENTS.md:8-29`, `packages/shared/scripts/seeder/AGENTS.md:48-67` |
| Tutorial: trace fixtures | How-to for adding real framework-trace fixture files | `packages/shared/scripts/seeder/utils/framework-traces/README.md:6-13` |
| API docs generation | Fern definitions for server/client/organizations APIs (27+ endpoint spec files) | `fern/apis/server/definition/api.yml:1`, `fern/apis/server/definition/prompts.yml:1` |
| API docs generation | `openapi:export` exports OpenAPI yml into app + syncs deprecation flags from Fern metadata | `package.json:41`, `CONTRIBUTING.md:546-552` |
| Generated API reference | Exported OpenAPI spec shipped with web app (`web/public/generated/api/openapi.yml`, ~497 KB) | `web/public/generated/api/openapi.yml:1` |
| SDK codegen automation | CI workflow regenerates Python/TS SDKs on `fern/**` push and opens PRs to SDK repos | `.github/workflows/sdk-api-spec.yml:5-13` |
| SDK generators config | Fern generator config for Pydantic v2 Python and TypeScript node SDKs | `fern/apis/server/generators.yml:14-36` |
| Commit conventions | Conventional commits required; squash merges; enforced by PR-title workflow with allowed-type list | `CONTRIBUTING.md:338-340`, `.github/workflows/validate-pr-title.yml:24-38` |
| PR template | Structured template with type-of-change checkboxes and self-review mandate | `.github/PULL_REQUEST_TEMPLATE.md:13-26` |
| Issue/discourse templates | Bug-report issue form; Ideas/Support discussion templates; CODEOWNERS present | `.github/ISSUE_TEMPLATE/bug_report.yml:1`, `.github/DISCUSSION_TEMPLATE/ideas.yml:1`, `.github/CODEOWNERS:1` |
| Test documentation | Test isolation via `.env.test` (separate PG database, Redis db 1); per-package run commands documented | `CONTRIBUTING.md:345-363`, `CONTRIBUTING.md:365-397` |
| Preview environments | Per-PR full-stack preview at `pr-<N>.preview.langfuse.com` built by CI | `CONTRIBUTING.md:415-420`, `.agents/AGENTS.md` (preview guidance) |
| Dead-code detection | knip configured across workspaces to catch unused exports/files | `knip.jsonc:1-45` |
| Externalized docs | Documentation lives in separate `langfuse-docs` repo; SDKs in `langfuse-python`/`langfuse-js` repos | `CONTRIBUTING.md:21` |
| User-facing tutorials (external) | README links feature tutorials and integration guides hosted on langfuse.com | `README.md:78-88`, `README.md:136-143` |

## Answers to Dimension Questions

### 1. Are contribution guides clear?

Yes, unusually so. The human guide is comprehensive and current: it states the issue-first policy (`CONTRIBUTING.md:36`), lists exact prerequisites pinned via `.nvmrc` (`CONTRIBUTING.md:119`), gives copy-paste commands through first successful login with demo credentials (`CONTRIBUTING.md:268-283`), documents known rough edges ("this will fail on the very first run. Please run it again", `CONTRIBUTING.md:268`), and explains test-database isolation semantics (`CONTRIBUTING.md:357-363`). The AI-contributor layer adds scoped context loading: root guide plus per-package `AGENTS.md` files with an explicit maintenance contract ("Update this file in the same PR when entry points, commands, or contracts change", `web/AGENTS.md:10-13`), and CI validates both shim existence and cited file paths (`package.json:12`, `.agents/README.md:151-160`) — meaning the guides themselves are tested artifacts.

### 2. Are examples comprehensive?

Partially, and mostly relocated. Within this repo, runnable example material consists of: the seed CLI with ~20 deterministic scenarios covering traces, sessions, media, scores, and cost outliers (`packages/shared/scripts/seeder/AGENTS.md:8-29`); real captured framework traces used as UI fixtures (`packages/shared/scripts/seeder/utils/framework-traces/README.md:1-19`); six `.env.*.example` files documenting configuration surfaces; and Storybook stories referenced as the styling-change workflow (`web/AGENTS.md`, storybook skill). There is **no top-level `examples/` directory**, no cookbook, and no sample application in-repo — searching directories matching `example|starter|template|cookbook|tutorial` returns only prisma migration folders, GitHub templates, and the product's evaluator-template gallery UI. End-user examples/tutorials (SDK quickstarts, integration walkthroughs) live in the external `langfuse-docs` and SDK repositories (`CONTRIBUTING.md:21`, `README.md:78-88`); per the source-isolation rules those were not inspected.

### 3. Is API documentation available?

Yes, and it is generated from maintained definitions. The three API surfaces (server, client, organizations) are specified as Fern YAML under `fern/apis/**` (e.g., `fern/apis/server/definition/api.yml:1`, 20+ resource files). `pnpm run openapi:export` produces OpenAPI documents consumed by the online reference and ships them inside the app at `web/public/generated/api/openapi.yml` (`package.json:41`); a companion script syncs deprecation notices from availability metadata (`CONTRIBUTING.md:550-552`). Server SDKs (Pydantic-v2 Python, TypeScript) are code-generated via `fern/apis/server/generators.yml:14-36`, and CI regenerates them automatically whenever `fern/**` changes (`.github/workflows/sdk-api-spec.yml:5-13`). Contributors changing public endpoints must update Fern sources and regenerated outputs in the same PR (`web/AGENTS.md:258-264`), keeping docs coupled to code. Caveat: local SDK generation requires an authenticated Fern account (`CONTRIBUTING.md:558-560`).

### 4. Are there tutorials for key tasks?

For internal extension tasks, yes — as embedded step-by-step playbooks rather than standalone tutorial documents: add/change a tRPC endpoint (`web/AGENTS.md:250-256`), add/change a public REST endpoint including Fern updates (`web/AGENTS.md:258-264`), error-handling conventions (`web/AGENTS.md:266-270`), frontend features (`web/AGENTS.md:272-282`), Postgres and ClickHouse schema migrations (`packages/shared/AGENTS.md:106-133`), queue payload contract evolution (`packages/shared/AGENTS.md:134-145`), export-surface changes (`packages/shared/AGENTS.md:146-157`), default model/pricing edits (`CONTRIBUTING.md:530-540`), new seeder scenarios (`packages/shared/scripts/seeder/AGENTS.md:63-66`), and OpenAPI/Fern updates (`CONTRIBUTING.md:542-560`). The 35-skill `.agents/skills/` library provides deeper workflows (backend guidelines with testing/middleware/routing references, ClickHouse best practices split into ~25 rule files). For product-user tasks (instrumenting tracing, building evaluations) tutorials exist but externally at langfuse.com (`README.md:78-88`). No evidence found of in-repo tutorials for the harness-specific extension axis "add an agent/tool/memory" — expected, since Langfuse is an observability platform whose analog extensions (evaluators, models, API resources) are the ones covered above.

### Template/starter repos

No evidence found within this source. Searched directory names matching `template|starter|boilerplate|cookiecutter` and README/CONTRIBUTING mentions; the only hits are product-feature code (evaluator template gallery under `web/src/features/evals/v2/fns/templateGallery/`) and GitHub issue/discussion templates. The nearest equivalents are environment starters: the devcontainer/Codespaces path (`CONTRIBUTING.md:124`), Codex Cloud bootstrap scripts (`scripts/codex/setup.sh`, referenced at `CONTRIBUTING.md:132-149`), and the Cursor Cloud environment contract (`.cursor/environment.json`, `.cursor/Dockerfile`, described in `.agents/README.md:226-247`).

## Architectural Decisions

1. **Docs-as-tested-artifacts for agent contributors.** Guidance lives in versioned `.agents/**` files with generated discovery shims; a checker script verifies shims and resolves every path cited by an `AGENTS.md`, failing CI on drift (`.agents/README.md:147-160`, `package.json:11-12`). This converts documentation rot from a review-time concern into a build failure.
2. **Single source of truth for API surface.** Hand-maintained Fern definitions generate OpenAPI output, deprecation flags, and two language SDKs (`package.json:41`, `.github/workflows/sdk-api-spec.yml:5-13`); hand-editing generated outputs is banned (`.agents/AGENTS.md`, "Generated Files").
3. **Context-scoped guidance hierarchy.** Root `AGENTS.md` holds universal rules; package guides own package contracts; skills own recurring deep workflows — explicitly instructed to place guidance "in the narrowest `AGENTS.md` that owns it" (`.agents/AGENTS.md`, Shared Agent Setup).
4. **Real-infrastructure testing over mocks.** Tests run against live Postgres/ClickHouse/Redis with a documented isolation env-file (`CONTRIBUTING.md:345-363`), and demo data flows through a verified seeding CLI rather than ad-hoc fixtures (`packages/shared/scripts/seeder/AGENTS.md:61-62`).
5. **Externalization of user docs.** Product documentation, SDKs, and tutorials are separate repositories (`CONTRIBUTING.md:21`), keeping this repo's contributor docs focused on code change mechanics.

## Notable Patterns

- **Inverted-checklist PR template**: checklist items are phrased as admissions of failure ("I haven't read the contributing guide…"), nudging honest self-assessment (`.github/PULL_REQUEST_TEMPLATE.md:32-38`).
- **Skill-with-scripts pattern**: the `add-model-price` skill ships executable validators (`validate-pricing-file.mjs` with its own test file) alongside prose (`.agents/skills/add-model-price/scripts/`), making guidance enforceable.
- **Meta-skill for authoring skills**: `skill-creator/SKILL.md` plus generator scripts standardize how new guidance is added (`.agents/skills/skill-creator/SKILL.md:1`).
- **Preview-per-PR**: every pull request gets a disposable full-stack preview URL, and agents are told to attach proof-of-fix against it (`.agents/AGENTS.md`, preview guidance; `CONTRIBUTING.md:415-420`).
- **Determinism contract for example data**: seeder scenarios must be reproducible (seeded RNG, no wall-clock in ORDER BY keys) and verify writes via ClickHouse readback (`packages/shared/scripts/seeder/AGENTS.md:52-62`).
- **Machine-readable scenario catalog**: `pnpm run seed -- list --json` exposes scenarios "for machines", treating example-data provisioning as a programmatic interface (`packages/shared/scripts/seeder/AGENTS.md:10`, 31-35).

## Tradeoffs

- **Repo separation vs. discoverability**: keeping docs/tutorials/examples in `langfuse-docs` simplifies this repo's scope but means a fresh clone contains no user-facing learning material; contributors must know to look elsewhere (`CONTRIBUTING.md:21`). Some cross-references acknowledge this by linking the docs-repo markdown mirror directly (`web/AGENTS.md`, architecture handbook links).
- **Generated-docs fidelity vs. tooling lock-in**: Fern guarantees spec/SDK consistency but requires an account for local generation (`CONTRIBUTING.md:558-560`) and pins generator versions (`fern/apis/server/generators.yml:15-31`).
- **Guide density vs. onboarding speed**: the `AGENTS.md` hierarchy is thorough (1,388+ lines across five core guides) but assumes pnpm/turborepo fluency; a newcomer following only the root file may miss package-level contracts until they touch those packages — mitigated by automatic context loading of nested guides (`.agents/README.md:136-139`).
- **CI-enforced docs vs. contribution friction**: requiring Fern updates, seeder registration, and shim sync per change raises per-PR ceremony, traded for consistently accurate interfaces.

## Failure Modes / Edge Cases

- **Path validation escape hatch**: references escaping the repo (`../langfuse-docs/**`) are only checked when they resolve, so stale cross-repo doc links can persist silently in standalone clones (`.agents/README.md:156-160`).
- **First-run failure is documented, not fixed**: `pnpm run dx` is known to fail on the initial invocation (`CONTRIBUTING.md:268`) — an accepted papercut that could still stall unfamiliar contributors.
- **Knip partially disabled**: dead-code detection ignores `files|exports|types` issues for all main packages pending workspace tracking fixes, weakening the safety net the config implies (`knip.jsonc:4-9`).
- **Test-environment coupling**: unit tests write/delete real data and default to the dev database unless `.env.test` is created (`CONTRIBUTING.md:345-350`); skipping that step risks destructive surprises, though it is prominently warned about.
- **Thin spots in the guide tree**: some feature guides are data-model notes rather than procedures (e.g., evaluators v2, 16 lines, `web/src/features/evals/v2/AGENTS.md:1-16`), so coverage depth varies by area.

## Future Considerations

- Vendor a minimal in-repo `examples/` (or a git submodule pointer into `langfuse-docs`) so clones demonstrate one end-to-end extension of each type (endpoint, evaluator, model price) without network access to the docs site.
- Replace the Fern-account requirement for local SDK/OpenAPI generation with a credential-free export path, keeping authenticated generation for publish-time only.
- Upgrade terse feature guides (evals v2) to the same playbook format used in `web/AGENTS.md` and `packages/shared/AGENTS.md`.
- Re-enable knip `files/exports/types` checks once workspace tracking false-positives are resolved (`knip.jsonc:4-9`).
- Consider CI validation that exported OpenAPI artifacts (`web/public/generated/**/*.yml`) match current Fern sources, closing the gap where a contributor forgets `openapi:export`.

## Questions / Gaps

- User-facing tutorial quality and example coverage in `langfuse-docs`, `langfuse-python`, and `langfuse-js` could not be assessed: those repositories are outside this task's single-source boundary (rule: no cross-source filesystem access). Only their existence and linkage are evidenced here (`CONTRIBUTING.md:21`).
- Whether the DeepWiki recommendation (`CONTRIBUTING.md:44-46`) stays current is unverifiable in-repo; it is an external generated-resource link.
- Effectiveness of the CLA flow beyond the documented troubleshooting steps (`CONTRIBUTING.md:562-578`) cannot be observed from static files alone.

---

Generated by `dimensions/22.03-docs-examples-contributor-workflow.md` against `langfuse`.

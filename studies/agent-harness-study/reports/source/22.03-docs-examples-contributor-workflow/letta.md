# Source Analysis: letta

## Dimension 22.03: Docs, Examples, and Contributor Workflow

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python 3.11–3.13 (FastAPI server, SQLAlchemy/Alembic, uv); Fern for API docs + generated Python/TypeScript SDKs; GitHub Actions CI |
| Analyzed | 2026-08-24 |

All citations below are relative to the source root `studies/agent-harness-study/sources/letta`.

## Summary

Letta's contributor workflow is strong on **environment setup, CI automation, and API documentation generation**, but weak on **in-repo examples and extension-authoring tutorials**. A single `CONTRIBUTING.md` covers forking, PostgreSQL/pgvector setup, uv dependency management, Alembic migrations, testing, formatting, and PR submission (`CONTRIBUTING.md:11-183`). API documentation is genuinely machine-generated: a checked-in OpenAPI spec (`fern/openapi.json`, 239 paths) is validated by `fern check` on every PR (`.github/workflows/fern-check.yml:18-20`) and drives automated publishing of docs plus generated Python and TypeScript SDKs (`.github/workflows/fern-docs-publish.yml:21`, `.github/workflows/fern-sdk-python-publish.yml:37-43`). However, the in-repo `examples/` directory was emptied ("chore: remove old examples (#6255)", git history of `examples/`) and now contains only three leftover data files (`examples/notebooks/data/task_queue_system_prompt.txt:1`, `examples/notebooks/data/shared_memory_system_prompt.txt:1`, `examples/notebooks/data/handbook.pdf`), with teaching material delegated to the external docs site referenced from `README.md:5-6,39`. There is exactly one task-specific tutorial in-repo — webhook setup (`WEBHOOK_SETUP.md`) — and it cites stale monorepo paths (`apps/core/letta/...` at `WEBHOOK_SETUP.md:180-194`) that no longer exist in this layout, as does the leftover Nx config `project.json:6-9`. Nothing in the repository teaches contributors how to add builtin tools, agents, memory features, tracing, or policies; those flows must be reverse-engineered from existing code. Notably, Letta invests heavily in a distinctive AI-disclosure contributor-safety workflow (`AI_POLICY.md:5-37`, enforced by issue templates and an auto-close bot).

## Rating

**5 / 10 — Present but inconsistent, weakly documented, and fragile in places.**

- **Present**: clear environment-setup guide (`CONTRIBUTING.md:28-94`), PR template with test instructions (`.github/pull_request_template.md:4-8`), pre-commit with secret scanning (`.pre-commit-config.yaml:14-18`), semantic PR titles enforced in CI (`.github/workflows/core-lint.yml:49-51`), and a mature Fern-based docs/SDK pipeline.
- **Inconsistent/fragile**: zero runnable in-repo examples after removal commit #6255; stale internal docs pointing at a defunct `apps/core` monorepo layout (`WEBHOOK_SETUP.md:180-194`, `project.json:6-9`, `project.json:13`); lint tooling drift between CONTRIBUTING (black, `CONTRIBUTING.md:149-153`) and CI/pre-commit (ruff + ty, `.pre-commit-config.yaml:20-31`, `.github/workflows/core-lint.yml:61-67`).
- On the rubric's guiding question — *can a new contributor add a tool in under an hour without asking for help?* — a **user-facing custom tool via the SDK**: plausibly yes, but only using external docs (the repo itself offers no guidance). A **builtin tool contributed to this repo**: no; it requires inferring the module-registration convention from `letta/constants.py` (`LETTA_BUILTIN_TOOL_MODULE_NAME` etc., imported at `letta/schemas/tool.py:4-15`) and mimicking docstring-driven schema generation in `letta/functions/function_sets/base.py:10-68`, with no written guide.

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Contribution guide | Full walkthrough: fork/clone, Postgres+pgvector SQL setup, env vars, uv sync, alembic, pre-commit | `CONTRIBUTING.md:11-94` |
| Contribution guide — changes | Branching, DB migration creation (`alembic revision --autogenerate`) | `CONTRIBUTING.md:96-123` |
| Contribution guide — testing | `uv run pytest -s tests`; "add new tests in tests/" for major features | `CONTRIBUTING.md:125-142` |
| Contribution guide — dependencies | `uv add <PACKAGE>`, optional extras guidance under `[project.optional-dependencies]` | `CONTRIBUTING.md:144-145` |
| Contribution guide — submission | black formatting check, PR creation steps, review flow, Docker dev compose | `CONTRIBUTING.md:149-183` |
| Examples directory (gap) | Only leftover data files remain; no notebooks or code | `examples/notebooks/data/task_queue_system_prompt.txt:1` |
| Examples removed | Git history: "chore: remove old examples (#6255)" is the latest commit touching `examples/` | git log, `sources/letta/.git` |
| Ruff still excludes examples | Lint config excludes `examples/*` although dir is empty (vestigial) | `pyproject.toml:174-177` |
| External docs delegation | README points to docs.letta.com quickstart/API reference instead of local examples | `README.md:5-6`, `README.md:38-39` |
| README hello-world examples | TS + Python agent-create/send-message snippets in README itself | `README.md:42-110` |
| API docs generated (Fern) | Checked-in OpenAPI spec, title "Letta API", 239 paths | `fern/openapi.json:2-4` |
| API docs customization | Fern overrides: server URLs (Cloud/Self-hosted) and MCP endpoints hidden from SDKs | `fern/openapi-overrides.yml:1-40` |
| Docs validity gate | `fern check` runs on every PR to main | `.github/workflows/fern-check.yml:18-20` |
| Docs publishing | `fern generate --docs` on push to main | `.github/workflows/fern-docs-publish.yml:21` |
| SDK generation | Python SDK release workflow (PyPI publish via fern generate) | `.github/workflows/fern-sdk-python-publish.yml:37-43` |
| SDK preview on spec change | TS SDK preview job triggers only when `fern/openapi.json` or overrides change | `.github/workflows/fern-sdk-typescript-preview.yml:8-11` |
| Tutorials (single in-repo example) | Step-completion webhook tutorial: architecture, config, payload, FastAPI receiver example, testing steps | `WEBHOOK_SETUP.md:37-160` |
| Stale tutorial paths | Webhook doc references `apps/core/letta/services/webhook_service.py`; actual file is `letta/services/webhook_service.py` (no `apps/` dir exists) | `WEBHOOK_SETUP.md:180-194` |
| Stale build tooling | Nx `project.json` sets `sourceRoot: apps/core` and runs commands with `cwd: apps/core`, which does not exist | `project.json:6-9`, `project.json:13` |
| Config self-documentation | `conf.yaml` header maps config keys to env-var prefixes; commented reference config | `conf.yaml:1-9` |
| Env var examples | `.env.example` documents OpenAI/Ollama/vLLM variables for Docker | `.env.example:1-22` |
| Pre-commit safety | TruffleHog secret scanning at pre-commit and pre-push | `.pre-commit-config.yaml:14-18` |
| Pre-commit formatting | ruff check --fix, ruff format, ty type check as local hooks | `.pre-commit-config.yaml:20-31` |
| CI quality gates | Semantic PR title enforcement; Pyright; ruff check/format on changed files | `.github/workflows/core-lint.yml:49-67` |
| AI contribution policy | Mandatory AI-use disclosure, human-in-the-loop verification, no AI media | `AI_POLICY.md:5-21` |
| AI policy enforcement | Auto-close/lock of non-compliant issues; org members and TRUSTED_CONTRIBUTORS exempt | `AI_POLICY.md:27-37`, `.github/TRUSTED_CONTRIBUTORS:1-13` |
| Issue template gate | AI Disclosure checkboxes + required human-verification phrase in bug report template | `.github/ISSUE_TEMPLATE/bug_report.yml:12-34` |
| Automated issue guard | GitHub Action validates disclosure, checks allowlist, closes and labels violations | `.github/workflows/issue-guard.yml:64-76`, `.github/workflows/issue-guard.yml:158-168` |
| PR template | Requires "How to test" commands/outputs; asks >500-line PRs to be split or justified | `.github/pull_request_template.md:4-14` |
| Test harness ergonomics | Session fixture auto-starts a Letta server thread and polls `/v1/health` so `pytest` works without manual server startup | `tests/conftest.py:23-47` |
| Tests as API documentation | SDK-level tests grouped by resource (agents, blocks, tools, MCP servers) double as usage examples | `tests/sdk/agents_test.py:1`, `tests/sdk/tools_test.py:1` |
| Builtin-tool authoring model | `memory()` tool docstring with Args + worked Examples section — the implicit pattern for new builtin tools | `letta/functions/function_sets/base.py:10-68` |
| Tool extension surface | `Tool` schema exposes source_code, json_schema, pip/npm requirements, requires_approval | `letta/schemas/tool.py:35-56` |
| In-code doc links | Server error-help text links to memory-blocks guide and `docs.letta.com/llms.txt` | `letta/server/rest_api/proxy_helpers.py:221-222` |
| Stale external link | Local-LLM README points to old `letta.readme.io` docs domain | `letta/local_llm/README.md:3` |
| Terse plugin doc | Plugin system documented in ~16 lines with config-string format only | `letta/plugins/README.md:1-16` |
| Template/starter repos | None found; closest artifact is Modal tool-execution skeleton server README | `sandbox/resources/server/README.md:1-10` |

## Answers to Dimension Questions

### 1. Are contribution guides clear?

**Mostly yes for setup, no for extensions.** `CONTRIBUTING.md:11-94` gives copy-pasteable Postgres/pgvector bootstrap SQL (`CONTRIBUTING.md:46-62`), uv install and sync commands (`CONTRIBUTING.md:64-79`), and migration commands (`CONTRIBUTING.md:111-123`). Gaps: (a) it recommends `black -l 140` before PRs (`CONTRIBUTING.md:149-153`) while CI and pre-commit actually run ruff + ty (`pyproject.toml:171-198`, `.pre-commit-config.yaml:20-31`) — a follower of the letter of the guide would fail CI style gates; (b) there is no section on adding builtin tools, agents, memory primitives, tracing, or policies; (c) testing guidance is one line ("add new tests in the tests/ directory", `CONTRIBUTING.md:141-142`) despite a complex test layout (`tests/integration_test_*.py`, `tests/sdk/`, `tests/performance_tests/`, per-dir `pytest.ini` files). The AI-disclosure policy is unusually explicit and mechanically enforced (`AI_POLICY.md:5-37`, `.github/workflows/issue-guard.yml:113-168`), which makes expectations unambiguous even if strict.

### 2. Are examples comprehensive?

**No — in-repo examples are effectively absent.** The `examples/` tree holds only three data files used by long-gone notebooks (`examples/notebooks/data/handbook.pdf`, `examples/notebooks/data/task_queue_system_prompt.txt:1`); commit #6255 ("chore: remove old examples") emptied it, yet ruff/ty configs still exclude the path (`pyproject.toml:174-177`, `pyproject.toml:205-206`). Coverage of extension types now lives entirely outside the repo: README carries one hello-world pair of snippets (`README.md:42-110`), and everything else is delegated to docs.letta.com (`README.md:5-6,39`). Within this repository there are no examples for tools, MCP, sandboxing, multi-agent groups, evals, tracing, or policies. The nearest substitutes are tests-as-documentation (`tests/sdk/agents_test.py:1` et al.) and the builtin tool docstrings (`letta/functions/function_sets/base.py:10-68`).

### 3. Is API documentation available?

**Yes, and it is pipeline-generated rather than hand-maintained — the strongest part of this dimension.** The OpenAPI contract is checked into the repo (`fern/openapi.json`, 239 paths, info block at lines 2-4), validated by `fern check` on every PR (`.github/workflows/fern-check.yml:18-20`), published as docs on merge to main (`.github/workflows/fern-docs-publish.yml:21`), and consumed to generate both Python and TypeScript SDKs with preview jobs gated on spec-file diffs (`.github/workflows/fern-sdk-typescript-preview.yml:8-11`) and a versioned PyPI release path (`.github/workflows/fern-sdk-python-publish.yml:37-43`). Fern overrides deliberately hide internal MCP-server-admin endpoints from public SDKs (`fern/openapi-overrides.yml:10-40`), showing curation. Caveat: nothing in-repo documents how `fern/openapi.json` is regenerated from the FastAPI app (schema caching exists at `letta/server/rest_api/app.py:139`), so spec updates rely on tribal knowledge.

### 4. Are there tutorials for key tasks?

**One, and it is partially stale.** `WEBHOOK_SETUP.md` is a genuine end-to-end tutorial — architecture, env vars, payload schema, auth, a complete FastAPI receiver example, failure-mode table, and testing steps (`WEBHOOK_SETUP.md:37-160`). But its "File Locations" appendix cites `apps/core/letta/services/webhook_service.py`, `apps/core/letta/agents/temporal/...`, and `apps/core/letta/services/webhook_service_test.py` (`WEBHOOK_SETUP.md:180-194`), none of which exist here; the real service lives at `letta/services/webhook_service.py` (with tests at `letta/services/webhook_service_test.py`). The same rot affects `project.json` (Nx targets with `cwd: apps/core`, `project.json:6-9,13`) — evidence that the repo migrated out of a monorepo without updating contributor-facing artifacts. No tutorials exist for the tasks the dimension names: adding agents, tools, evals, memory, tracing, or policies. Configuration is the exception: `conf.yaml:1-9` self-documents key→env-var mappings and `.env.example:1-22` covers provider setup.

## Architectural Decisions

1. **Docs live outside the code repo.** Teaching material was consolidated into the external docs site; the repo keeps only setup/contributor logistics (`CONTRIBUTING.md`) plus one feature tutorial (`WEBHOOK_SETUP.md`). Consequence: fast iteration on tutorials without code releases, but the repo alone cannot onboard an extension author.
2. **Contract-first API documentation.** The OpenAPI spec is a first-class, reviewed artifact in version control (`fern/openapi.json`), not a build byproduct — PRs break on invalid specs (`.github/workflows/fern-check.yml:18-20`) and SDKs regenerate automatically downstream.
3. **Policy-as-code for contributions.** Contributor trust rules are executable: required template fields plus phrase verification (`.github/ISSUE_TEMPLATE/bug_report.yml:12-34`), an allowlist file (`.github/TRUSTED_CONTRIBUTORS:1-13`), and a bot that closes/locks violations (`.github/workflows/issue-guard.yml:113-168`).
4. **Tests double as usage documentation.** The `tests/sdk/` suite exercises the public client surface resource-by-resource (e.g., `tests/sdk/tools_test.py`), implicitly serving as the only in-repo SDK examples.

## Notable Patterns

- **Self-starting test fixture**: `tests/conftest.py:23-47` boots the FastAPI server in a background thread and health-polls `/v1/health`, so a newcomer can run `uv run pytest -s tests` right after setup without learning server orchestration.
- **Docstring-driven tool schemas**: builtin tools declare behavior purely through Google-style docstrings with `Examples:` blocks (`letta/functions/function_sets/base.py:10-68`); these feed JSON-schema generation (`get_json_schema_from_module`, imported at `letta/schemas/tool.py:19`), making existing tools the de facto authoring reference.
- **Commented canonical config file**: `conf.yaml:1-9` opens with a key→env-prefix mapping table and inline comments, functioning as a living configuration reference.
- **In-product documentation links**: runtime help text routes users to external guides, including an LLM-consumable `llms.txt` index (`letta/server/rest_api/proxy_helpers.py:221-222`).
- **Vestigial-path debt markers**: empty-but-excluded `examples/` (`pyproject.toml:174-177`) and dead `apps/core` targets (`project.json:9,13`) quietly document the repo's monorepo past.

## Tradeoffs

- **External docs vs. repo self-containment**: centralizing tutorials at docs.letta.com keeps them current, but violates the study premise of teaching contributors where the code lives; nothing pins docs versions to code versions.
- **Generated SDKs vs. discoverability**: the Fern pipeline guarantees API-reference accuracy but hides the regeneration procedure — no script or CONTRIBUTING section explains how `fern/openapi.json` is produced from the FastAPI app.
- **Strict AI policy vs. friction**: mandatory disclosure phrases and auto-close bots (`.github/workflows/issue-guard.yml:158-168`) deter spam and unreviewed AI dumps, at the cost of a higher first-contact barrier for drive-by contributors.
- **CI rigor vs. guide accuracy**: moving linting from black/isort (still described in `CONTRIBUTING.md:149-153` and `project.json:60-62`) to ruff+ty improved speed but left the human-facing guide wrong.

## Failure Modes / Edge Cases

- **Stale-path onboarding trap**: a contributor following `WEBHOOK_SETUP.md:159,180-194` will search for `apps/core/**` files that do not exist and may open PRs against imagined paths.
- **Silent example rot already happened once**: examples were deleted wholesale (#6255) while configs still reference them; future readers of `pyproject.toml:174-177` may assume examples exist elsewhere in-repo.
- **Style-gate mismatch**: running only the documented `black . -l 140` (`CONTRIBUTING.md:152`) does not satisfy the actual gates (ruff check/format, ty; `.github/workflows/core-lint.yml:61-67`, `.pre-commit-config.yaml:27-31`), producing avoidable CI failures.
- **Undocumented builtin-tool registration**: without a guide, a new builtin tool author may miss the module-name constants registration step (`LETTA_BUILTIN_TOOL_MODULE_NAME` etc., `letta/schemas/tool.py:10-14`), yielding tools that never load.
- **Outdated branding links**: `letta/local_llm/README.md:3` still points to `letta.readme.io`, the pre-Fern docs host; such links decay without a checker.

## Future Considerations

- Restore minimal runnable examples (or a stub README in `examples/` linking each external tutorial category: tools, MCP, sandbox, multi-agent, tracing) so the directory stops being a dead end.
- Add an "Authoring builtin tools" section to `CONTRIBUTING.md` covering function_sets placement, docstring-to-schema conventions (`letta/functions/function_sets/base.py:10-68`), module constants registration (`letta/schemas/tool.py:4-15`), and the matching test locations.
- Replace stale `apps/core` references in `WEBHOOK_SETUP.md:180-194` and delete or regenerate `project.json` for the standalone layout.
- Align `CONTRIBUTING.md:149-153` with the real gates: `uv run ruff check`, `ruff format`, and `ty check`.
- Document the OpenAPI regeneration step (FastAPI → `fern/openapi.json`) next to the existing `fern check` gate.
- Add a link-rot checker for in-repo markdown (would have caught `letta/local_llm/README.md:3`).

## Questions / Gaps

- **How is `fern/openapi.json` regenerated?** Searched `scripts/`, workflows, `pyproject.toml`, and the server package for a dump command; only schema caching at `letta/server/rest_api/app.py:139` was found. No evidence of a documented export step — likely manual or tribal knowledge.
- **Where do eval-authoring tutorials live?** No eval-related examples, guides, or templates exist in-repo; the model-sweep machinery under `.github/scripts/model-sweep/model_sweep.py:1` is internal CI tooling, not contributor documentation. No evidence found of in-repo eval onboarding.
- **Is there an official starter/template repo?** None referenced anywhere in this repository; `sandbox/resources/server/README.md:1-10` describes a Modal-internal TypeScript skeleton for tool execution, not a user starter. External links in `README.md:12` point to `letta-code` (a CLI product), not a template.
- **Why does `tests/` mix naming schemes** (`test_*.py`, `integration_test_*.py`, `*_test.py` under `tests/sdk/`)? Intent unclear from the repo; per-directory `pytest.ini` files (`tests/pytest.ini:1`) suggest ad-hoc evolution rather than documented convention.

---

Generated by `22.03-docs-examples-and-contributor-workflow` against `letta`.

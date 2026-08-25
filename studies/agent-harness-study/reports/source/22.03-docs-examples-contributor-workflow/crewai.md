# Source Analysis: crewai

## Dimension 22.03 — Docs, Examples, and Contributor Workflow

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (uv workspace: `crewai`, `crewai-tools`, `crewai-files`, `cli`, `devtools`); Mintlify MDX docs site; GitHub Actions CI |
| Analyzed | 2026-08-25 |

## Summary

CrewAI treats contributor enablement as a first-class product surface. The repo ships a thorough `CONTRIBUTING.md` (`.github/CONTRIBUTING.md:1-173`) covering setup, workspace layout, branching, code style, commits, PRs, testing, strict mypy, dependency management, and a dedicated docs workflow. The documentation site (`docs/`, Mintlify) is organized into an always-current **Edge** version plus per-release frozen snapshots (`docs/v1.10.0/` … `docs/v1.15.17/`), in four locales (`en`, `ar`, `ko`, `pt-BR`), registered in a single nav manifest (`docs/docs.json:57-60`). Extension is taught through task-oriented tutorials that map onto concrete extension points in code: custom tools (`docs/edge/en/learn/create-custom-tools.mdx:18-54` → `BaseTool` at `lib/crewai/src/crewai/tools/base_tool.py:103`), custom LLMs (`docs/edge/en/learn/custom-llm.mdx:8-35` → `BaseLLM` at `lib/crewai/src/crewai/llms/base_llm.py:150`), event listeners for tracing (`docs/edge/en/concepts/event-listener.mdx:8-16` → `CrewAIEventsBus` at `lib/crewai/src/crewai/events/event_bus.py:95`, `BaseEventListener` at `lib/crewai/src/crewai/events/base_event_listener.py:8`), and pluggable memory storage (`docs/edge/en/concepts/memory.mdx:765` → `StorageBackend` protocol at `lib/crewai/src/crewai/memory/storage/backend.py:45`). Scaffolding is built into the CLI (`crewai create crew|flow|tool|skill|template`, `lib/cli/src/crewai_cli/cli.py:187-277`) from embedded project templates that even include a generated `AGENTS.md` for AI coding assistants (`lib/cli/src/crewai_cli/templates/AGENTS.md:1-10`). The two biggest gaps are the absence of a generated Python API reference — the "API Reference" tab documents only the hosted AMP REST API (`docs/docs.json:423-436`, `docs/edge/en/api-reference/introduction.mdx:5-9`) — and five tutorial pages that exist on disk but are unreachable from navigation.

## Rating

**8 / 10**

Rationale against the rubric: this is a clear, tested model with explicit interfaces and operational safeguards (broken-link CI at `.github/workflows/docs-broken-links.yml:64`, frozen-snapshot immutability policy enforced by release tooling at `lib/devtools/src/crewai_devtools/cli.py:1135`,1190, translation-parity workflow in `DOCS_TRANSLATIONS.md:1-15`). It falls short of 9–10 because: (a) no generated class/function API reference exists and the pydocstyle lint rule that would enforce docstrings is disabled (`pyproject.toml:67`), (b) examples live entirely out-of-tree (`docs/edge/en/examples/example.mdx:8-60`) so they can drift from core releases, (c) five Learn tutorials are orphaned from navigation, and (d) documentation code snippets are not executed or validated in CI. On the rubric's litmus question — *can a new contributor add a tool in under an hour without asking for help?* — the answer is yes: `create-custom-tools.mdx` is complete and self-contained (subclass + decorator patterns, typed outputs, caching, async at `docs/edge/en/learn/create-custom-tools.mdx:161-226`), backed by a scaffold command (`lib/cli/src/crewai_cli/cli.py:247`) and a PyPI publishing path (`docs/edge/en/guides/tools/publish-custom-tools.mdx:5-15`).

## Evidence Collected

Every entry cites a file path with line numbers, relative to `sources/crewai`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Contribution guide | Full setup, structure, branching, style, commits, PR rules | `.github/CONTRIBUTING.md:13-107` |
| Testing & type-check commands documented | `pytest`/`mypy` invocations per package; mypy run on 3.10–3.13 in CI | `.github/CONTRIBUTING.md:108-136` |
| AI-contributor policy | LLM-authored PRs/issues must carry `llm-generated` label | `.github/CONTRIBUTING.md:3-7` |
| Docs-editing rules for agents | Edit only `docs/edge/en/*`; never touch frozen `docs/v*/`; preview/broken-link commands | `AGENTS.md:17-28` |
| Translation parity process | Sync `ar`/`ko`/`pt-BR` after every English change; git-diff-driven workflow | `DOCS_TRANSLATIONS.md:1-15` |
| README contribution section points to guide | Quick-start commands + "Contributing to the docs" section | `README.md:601-633` |
| Mintlify site config with Edge version | `"version": "Edge"` plus per-release snapshot tabs | `docs/docs.json:57-60` |
| Nav coverage of extension topics | Tools catalog groups (7 categories), Observability (18 pages), Learn group incl. hooks sub-group | `docs/docs.json:212-344`, `346-368`, `370-407` |
| Frozen snapshots exist per release | Directories `v1.10.0` … `v1.15.17` alongside `edge` | `docs/` (directory listing), freeze automation at `lib/devtools/README.md:38` |
| Broken-link CI gate | Prunes frozen versions, runs `mint broken-links` on docs-touching PRs | `.github/workflows/docs-broken-links.yml:34,64` |
| Custom tools tutorial | `BaseTool` subclass and `@tool` decorator patterns with input schemas | `docs/edge/en/learn/create-custom-tools.mdx:18-54` |
| Typed tool outputs + async tools | Pydantic return annotations, `result_schema`, `_run`/`_arun` | `docs/edge/en/learn/create-custom-tools.mdx:56-159,178-226` |
| Tool publishing guide | Package/publish community tools to PyPI, "tools contract" | `docs/edge/en/guides/tools/publish-custom-tools.mdx:5-40` |
| Tool contract implemented in code | `BaseTool(BaseModel, ABC)` exported from `crewai.tools` | `lib/crewai/src/crewai/tools/base_tool.py:103`, re-exported via `lib/crewai/src/crewai/tools/__init__.py:1-19` |
| Custom LLM tutorial ↔ code | `BaseLLM` quick start maps to abstract base class | `docs/edge/en/learn/custom-llm.mdx:8-35`; `lib/crewai/src/crewai/llms/base_llm.py:150` |
| Event-listener (tracing) tutorial ↔ code | Bus/listener architecture described then implemented | `docs/edge/en/concepts/event-listener.mdx:8-16`; `lib/crewai/src/crewai/events/event_bus.py:95`; `base_event_listener.py:8` |
| Memory backend extension point | "Custom backend: implement the StorageBackend protocol" | `docs/edge/en/concepts/memory.mdx:765,871`; `lib/crewai/src/crewai/memory/storage/backend.py:45`; factory hook `lib/crewai/src/crewai/memory/storage/factory.py:28` |
| Evals/testing tutorial | `crewai test` CLI with iterations/model flags and scoring table | `docs/edge/en/concepts/testing.mdx:9-50` |
| Observability integrations | Per-vendor tracing guides (Langfuse, Phoenix, MLflow, …) | `docs/edge/en/observability/tracing.mdx` + 17 vendor pages (`docs/docs.json:346-368`) |
| Hooks tutorials group | Six pages covering kickoff/step/tool/LLM/execution-boundary hooks | `docs/docs.json:396-406` |
| Examples hosted out-of-tree | Cards linking to `crewAI-examples` and `crewAI-quickstarts` repos | `docs/edge/en/examples/example.mdx:8-60`; `docs/edge/en/examples/cookbooks.mdx:8-40` |
| CLI scaffolding of new projects | `crewai create <type>` supporting crew, flow, tool, skill, template | `lib/cli/src/crewai_cli/cli.py:187-277` (docstring at :198) |
| Embedded project templates | Full starter trees for crew/flow/tool/declarative_flow/json_crew | `lib/cli/src/crewai_cli/templates/{crew,flow,tool,declarative_flow,json_crew}/` (directory listing) |
| Generated AGENTS.md in scaffolds | Auto-written reference for AI assistants with version-freshness protocol | `lib/cli/src/crewai_cli/templates/AGENTS.md:1-30` |
| AI-native docs surface | Skills pack installs, `llms.txt` machine-readable docs, docs MCP server | `docs/edge/en/guides/coding-tools/build-with-ai.mdx:20-105` |
| API Reference = REST platform only | Tab contains kickoff/status/inputs/resume endpoints, not Python API | `docs/docs.json:423-436`; `docs/edge/en/api-reference/introduction.mdx:5-9` |
| No generated Python API docs | No mkdocstrings/sphinx/pdoc config anywhere; pydocstyle rule commented out | `pyproject.toml:67` (grep over `pyproject.toml` files found no doc-gen tooling) |
| Orphaned tutorials | 5 files under `learn/` absent from `docs.json` edge nav | `docs/edge/en/learn/{bring-your-own-agent,a2a-agent-delegation,a2ui,streaming-crew-execution,streaming-flow-execution}.mdx` vs `docs.json` (scripted check returned exactly these five) |
| Release tooling freezes docs | Step 6 of `devtools release` freezes Edge into versioned snapshot | `lib/devtools/README.md:38`; `[docs-freeze]` prefix logic `lib/devtools/src/crewai_devtools/cli.py:1135,1190-1193` |

## Answers to Dimension Questions

**1. Are contribution guides clear?**
Yes — unusually so. `.github/CONTRIBUTING.md` covers prerequisites (`.github/CONTRIBUTING.md:13-17`), exact setup commands (`:19-28`), a workspace/package table mapping all four `lib/` packages (`:32-41`), conventional-commit branching (`:43-55`), code style rules including modern generics and type-narrowing preferences (`:71-78`), commit format with examples (`:80-98`), PR discipline including automatic `size/XL` labeling (`:100-106`), per-package test commands (`:108-122`), and strict multi-version mypy (`:124-136`). The docs sub-workflow is split across `AGENTS.md:17-28` (frozen-snapshot rules, preview commands) and `DOCS_TRANSLATIONS.md:1-15`. One friction point: every English docs edit requires syncing three translations before finishing the task (`AGENTS.md:27-28`), which raises the cost of small doc fixes but keeps locale parity mechanical rather than aspirational.

**2. Are examples comprehensive?**
Broadly yes, but out-of-tree. The Examples tab indexes full crews (marketing strategy, recruitment, game builder), flows (self-evaluation loop, human-in-the-loop lead scoring), and integrations via cards into the separate `crewAI-examples` repo, plus notebook-style cookbooks in `crewAI-quickstarts` (`docs/edge/en/examples/example.mdx:8-60`, `cookbooks.mdx:8-40`). Coverage spans most extension types (crews, flows, guardrails, planning, reasoning, custom LLMs). The tradeoff is decoupling: examples are not pinned to framework releases inside this repo, so a given cookbook may lag current APIs with no in-repo mechanism detecting that. Within the repo itself there are no runnable example projects — only scaffold templates under `lib/cli/src/crewai_cli/templates/` — so "does the latest core still work with example X" cannot be verified by CI here (No clear evidence found of any cross-repo example smoke test).

**3. Is API documentation available?**
Partially. There is **no generated Python API reference**: grep found no mkdocstrings/Sphinx/pdoc configuration in any `pyproject.toml`, and the pydocstyle lint rule is explicitly disabled pending cleanup (`pyproject.toml:67`, convention declared at `:107-108`). Docstrings are requested but not enforced (`CONTRIBUTING.md:75` asks for Google-style docstrings). The site's "API Reference" tab (`docs/docs.json:423-436`) documents only the hosted CrewAI AMP REST endpoints (`kickoff`, `status`, `inputs`, `resume`; `docs/edge/en/api-reference/introduction.mdx:5-33`). Contributors extending deep internals must read source directly; the mitigation is that key extension ABCs are small and discoverable (`base_tool.py:103`, `base_llm.py:150`, `backend.py:45`).

**4. Are there tutorials for key tasks?**
Yes, with one notable hygiene failure. The Learn section (`docs/docs.json:370-407`) covers: creating custom tools (`create-custom-tools.mdx`), publishing tools (`guides/tools/publish-custom-tools.mdx`), custom LLMs (`custom-llm.mdx:8-35`), agent customization and custom manager agents, six execution-hook variants (`docs.json:396-406`), event listeners for tracing/monitoring (`concepts/event-listener.mdx:8-16`), pluggable memory backends (`concepts/memory.mdx:765`), streaming consumption, and evals via `crewai test` (`concepts/testing.mdx:9-25`). Policies/guardrails are covered as task-level guardrails and human-in-the-loop patterns rather than a distinct "policy" extension tutorial. However, five tutorial files are orphaned — `bring-your-own-agent.mdx`, `a2a-agent-delegation.mdx`, `a2ui.mdx`, `streaming-crew-execution.mdx`, `streaming-flow-execution.mdx` exist under `docs/edge/en/learn/` but appear nowhere in `docs.json`, making them reachable only by direct URL.

## Architectural Decisions

- **Edge + frozen snapshot docs model.** All edits land in `docs/edge/<lang>/` and are frozen verbatim into `docs/v<X.Y.Z>/` at each release by `devtools release` step 6 (`lib/devtools/README.md:38`), with canonical redirects repointed (`README.md:623-633`). This makes published docs immutable per version at the cost of a 50k-line nav manifest (`docs/docs.json`) and CI pruning hacks to keep link checks fast (`.github/workflows/docs-broken-links.yml:29-35`).
- **Examples out-of-tree.** Maintaining examples in `crewAI-examples`/`crewAI-quickstarts` keeps the core repo lean and lets examples iterate independently, trading away version-coupling and in-repo verification.
- **Scaffolding as onboarding.** `crewai create` generates working project skeletons from embedded templates (`lib/cli/src/crewai_cli/cli.py:187-277`), including a pre-written `AGENTS.md` that instructs AI assistants to verify installed version, read changelog, and consult live docs before writing CrewAI code (`lib/cli/src/crewai_cli/templates/AGENTS.md:8-20`) — a deliberate defense against stale training data.
- **AI-native contributor surface.** Beyond human docs: `llms.txt`, a docs MCP server, and an installable skills pack are documented as first-class entry points (`docs/edge/en/guides/coding-tools/build-with-ai.mdx:20-105`), and the contribution policy itself addresses AI agents (`CONTRIBUTING.md:3-7`).

## Notable Patterns

- **Tutorials anchored to concrete interfaces.** Every "extend X" tutorial names the exact base class/protocol and mirrors it in code: `BaseTool` (`create-custom-tools.mdx:31-39` ↔ `base_tool.py:103`), `BaseLLM` (`custom-llm.mdx:22-35` ↔ `base_llm.py:150`), `BaseEventListener` (`event-listener.mdx` ↔ `base_event_listener.py:8`), `StorageBackend` (`memory.mdx:765` ↔ `backend.py:45`). This makes the "can I build this?" question answerable from docs alone.
- **Operational safeguards around docs.** Broken-link CI gated on docs paths (`.github/workflows/docs-broken-links.yml:3-12,64`), snapshot immutability enforced via PR-title prefix in release tooling (`cli.py:1135,1190-1193`), auto-generated tool spec sync committed by CI (`generate-tool-specs.yml:49-61`).
- **Layered guidance by audience.** Human contributors get `CONTRIBUTING.md`; AI agents get root `AGENTS.md` plus scaffolded per-project `AGENTS.md`; agents wanting live knowledge get MCP/`llms.txt`.
- **Progressive disclosure in docs IA.** Get Started → Guides → Core Concepts → per-tool reference catalog → Observability → Learn (`docs.json:74-414`), separating conceptual learning from API-like lookup.

## Tradeoffs

- **Translation mandate vs. velocity.** Requiring `ar`/`ko`/`pt-BR` sync for every English edit (`AGENTS.md:27-28`, `DOCS_TRANSLATIONS.md`) guarantees parity but taxes every doc change and likely discourages incremental improvements.
- **Immutability vs. maintenance cost.** Frozen snapshots preserve historical accuracy but bloat the nav manifest and require runtime pruning inside CI to keep checks feasible (`.github/workflows/docs-broken-links.yml:29-35`).
- **Out-of-tree examples vs. drift.** Decoupled example repos reduce core-repo noise but remove the compile/test feedback loop; nothing in this repo validates linked examples still work.
- **Curated hand-written reference vs. generated API docs.** Hand-maintained tool catalog pages (~90 pages under `docs/edge/en/tools/`, `docs.json:212-344`) read well but duplicate information derivable from `tool.specs.json` / source signatures, and can silently diverge.
- **Docstring quality unenforced.** With ruff's `D` rules disabled (`pyproject.toml:67`), the eventual API-reference story has weaker raw material than the prose docs suggest.

## Failure Modes / Edge Cases

- **Navigation drift.** Five Learn tutorials are unreachable from nav (verified by scripted diff of `docs/edge/en/learn/*.mdx` against `docs.json`): `bring-your-own-agent.mdx`, `a2a-agent-delegation.mdx`, `a2ui.mdx`, `streaming-crew-execution.mdx`, `streaming-flow-execution.mdx`. Content exists but discovery fails — the broken-link checker does not catch nav omissions.
- **Enforcement claim not visible in-repo.** `README.md:628-631` states CI rejects modifications to frozen snapshots without a `[docs-freeze]` title, and devtools code references a "docs-snapshots CI guard" (`lib/devtools/src/crewai_devtools/cli.py:1190`, `docs_check.py:394`), but no such workflow exists under `.github/workflows/` (enumerated: 14 workflows, none matching). Enforcement presumably lives in branch protection or another repo — unverifiable within this source boundary.
- **Untested snippets can rot.** Nothing executes MDX code blocks in CI; e.g., `custom-llm.mdx:17` uses `typing.Dict/List` style that the project's own style rules forbid elsewhere (`CONTRIBUTING.md:73-78`), a small but telling divergence.
- **Versioned-doc staleness trap for readers.** Users landing on old snapshots see outdated extension APIs; mitigated by default-version redirects but not eliminated.
- **Out-of-tree example breakage surfaces only as user reports**, since neither broken-link CI nor tests exercise linked notebooks/repos.

## Future Considerations

- Generate a Python API reference (mkdocstrings/pdoc over `lib/crewai/src/crewai`) into a dedicated Reference tab; re-enable ruff `D` rules once docstring debt is paid down (`pyproject.toml:67`).
- Add the five orphaned Learn pages to `docs.json` or delete them; consider a CI check asserting every `docs/edge/*/learn/*.mdx` file appears in nav.
- Add a lightweight snippet-extraction test job that imports/executes fenced Python blocks from high-traffic tutorials (`create-custom-tools.mdx`, `custom-llm.mdx`).
- Pin or periodically validate out-of-tree example repos against the latest released `crewai` version (e.g., a scheduled cross-repo smoke workflow), closing the drift gap noted above.
- Clarify where the `[docs-freeze]` enforcement actually lives (branch protection config or external repo) in `CONTRIBUTING.md` so the safeguard is auditable.

## Questions / Gaps

- No in-repo runnable example projects were found (searched top-level directories and `lib/`); all end-user examples are external links. Whether the external repos track core releases could not be verified within the source-isolation boundary.
- The contents/quality of the `crewAIInc/skills` pack, `llms.txt`, and the docs MCP server are external artifacts referenced by docs (`build-with-ai.mdx:26-105`); they were not inspected per isolation rules.
- The claimed CI snapshot guard ("docs-snapshots") is referenced in code comments (`cli.py:1190`) but absent from `.github/workflows/`; its actual enforcement location remains an open question.
- Translation completeness was spot-checked structurally (`ar`, `ko`, `pt-BR` directories mirror `en` layout) but not content-compared; parity quality is asserted by process (`DOCS_TRANSLATIONS.md`) rather than verified here.

---

Generated by `dimensions/22.03-docs-examples-contributor-workflow.md` against `crewai`.

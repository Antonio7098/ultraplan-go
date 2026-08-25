# Source Analysis: openai-agents-sdk

## 22.03: Docs, Examples, and Contributor Workflow

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (3.10–3.14), uv, pytest, MkDocs Material + mkdocstrings, GitHub Actions, Make |
| Analyzed | 2026-08-24 |

> Citation convention: all `file:line` paths below are relative to the source root `studies/agent-harness-study/sources/openai-agents-sdk`.

## Summary

The repository teaches extension and contribution through four reinforcing layers rather than a single guide. First, a dense contributor guide at the root (`AGENTS.md:1`, mirrored as `CLAUDE.md:1`) covers repo structure, a mandatory local verification stack (`make format/lint/typecheck/tests`, `AGENTS.md:247-256`), documentation verification tiers (`AGENTS.md:80-88`), public API compatibility rules (`AGENTS.md:108-116`), and PR/commit conventions (`AGENTS.md:286-295`). Second, user-facing MkDocs documentation is extensive (903-line tools guide, 571-line testing recipe book, quickstarts for text/sandbox/voice/realtime) and is rendered into a generated API reference via mkdocstrings (`mkdocs.yml:25-43`) with auto-created stub pages (`docs/scripts/generate_ref_files.py:46-73`). Third, 216 Python example files across 15+ directories cover agents, tools, handoffs, memory sessions, MCP, model providers, voice, realtime, and sandbox workflows, with a scripted runner (`examples/run_examples.py:1-10`, `Makefile:96-123`). Fourth, a maintainer knowledge layer — `.agents/skills/` (14 skills including `code-change-verification`) and `.agents/references/` (17 architecture invariants with a reference map, `.agents/references/README.md:31-48`) — encodes where to read before touching each runtime boundary. CI enforces all of it on every PR: lint, dual typecheck (mypy + pyright), tests across five Python versions plus Windows, coverage gate ≥85%, docs build, and a frozen public-API release contract (`.github/workflows/tests.yml:16-398`). A new contributor can plausibly add a function tool in well under an hour by combining `docs/tools.md:303-402` (annotated walkthrough with expected output), `examples/basic/tools.py:14-27` (minimal runnable sample), and `docs/testing.md:70-114` (deterministic test recipe).

## Rating

**8 / 10** — Clear, mature model: every extension type has runnable examples plus prose tutorials; testing utilities are shipped as part of the SDK (`src/agents/testing/model.py:249`); docs builds, translation safety, and API-contract freezing are CI-enforced. It falls short of 9–10 for three concrete gaps: no conventional root-level `CONTRIBUTING.md` entry point (the guide is named `AGENTS.md` and is agent-workflow oriented), no first-class evals examples or framework guidance (evals appear only as host-side scripts inside sandbox tutorials), and no template/starter repo referenced anywhere in-repo.

## Evidence Collected

Every entry cites a file path relative to `sources/openai-agents-sdk/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Contributor guide | Root guide titled "Contributor Guide" covering structure, workflow, testing, PR rules | `AGENTS.md:1-13` |
| Mandatory verification stack | Ordered `make format → lint → typecheck → tests` required before completion | `AGENTS.md:247-256` |
| Docs verification tiers | Editorial / Content / Structural tiers gate how much review+build a docs change needs | `AGENTS.md:80-88` |
| Public API compatibility policy | Constructor field order, `__all__` membership, import paths declared compatibility contracts | `AGENTS.md:108-116` |
| PR template | Summary/Test plan/Issue/Checks checklist incl. running `.agents/skills/code-change-verification/scripts/run.sh` | `.github/PULL_REQUEST_TEMPLATE/pull_request_template.md:1-19` |
| Maintainer skill library | 14 skills: code-change-verification, implementation-strategy, final-review, release-candidate-prep, docs-sync, sensitive-logging-audit, etc. | `.agents/skills/` (directory listing) |
| Architecture reference map | "Read before changing" table mapping 17 boundary references to code areas | `.agents/references/README.md:31-48` |
| Test-suite contributor doc | xdist shard-safety, `serial` marker semantics, snapshot workflow, determinism guidelines | `tests/README.md:5-37` |
| Examples catalog (user docs) | Categorized index linking every example dir with per-file annotations | `docs/examples.md:5-133` |
| Example runner ops README | `make examples-run` lifecycle, filters, per-example logs under `.tmp/examples-start-logs/` | `examples/README.md:1-18` |
| Example runner implementation | Discovers `__main__`-guarded files, skips interactive/server/audio/external unless included, writes logs | `examples/run_examples.py:1-10` |
| Make targets for examples/integration | `examples-run/-status/-stop/-logs/-tail`; 15+ `integration-tests-*` profiles | `Makefile:101-195` |
| Function-tool tutorial | Annotated `@tool` walkthrough with schema output shown, docstring-driven descriptions | `docs/tools.md:303-402` |
| Minimal tool example | 27-line runnable sample: Pydantic output model + `@tool` + Agent wiring | `examples/basic/tools.py:14-27` |
| Testing recipes shipped in SDK | `ScriptedModel` deterministic model double is part of the library, not just dev deps | `src/agents/testing/model.py:249`; `docs/testing.md:43-66` |
| Testing recipe book | Decision table + copy-paste pytest recipes for tools, streaming, failures, sandbox, realtime, voice | `docs/testing.md:7-23`, `70-114` |
| Memory examples | 15 session examples: SQLite, Redis, SQLAlchemy, MongoDB, Dapr, encrypted, compaction, HITL variants | `examples/memory/` (listing); `docs/examples.md:73-88` |
| Guardrails/policies tutorial | Input/output/tool guardrails + "Implementing a guardrail" walkthrough | `docs/guardrails.md:20-77`; `examples/basic/tool_guardrails.py` |
| Tracing customization tutorial | Custom processors via `add_trace_processor()` / `set_trace_processors()`, sensitive-data controls | `docs/tracing.md:138-160` |
| End-to-end tutorials with evals | Sandbox tutorials with fixture data, Dockerfile, READMEs, and host-side `evals.py` validation scripts | `examples/sandbox/tutorials/dataroom_metric_extract/README.md:1-45`; `examples/sandbox/tutorials/dataroom_metric_extract/evals.py:214-302` |
| Generated API reference | mkdocstrings plugin (google style), 48+ stub pages under `docs/ref/`, watch mode on `src/agents` | `mkdocs.yml:25-43`, `94-201`, `379-380`; `docs/ref/` listing |
| Ref-stub generator | Script scans `src/agents/**/*.py` and creates missing mkdocstrings pages | `docs/scripts/generate_ref_files.py:46-73` |
| i18n docs pipeline | en/ja/ko/zh navs; generated translations under `docs/ja`,`ko`,`zh` (do-not-edit rule) | `mkdocs.yml:44-333`; `AGENTS.md:145` |
| LLM-oriented docs artifacts | `llms.txt` / `llms-full.txt` curated indexes of all doc pages | `docs/llms.txt:7-59`; `docs/llms-full.txt:7-113` |
| Docs deploy automation | Deploys on push to main when `docs/**` or `mkdocs.yml` change, skipping non-docs pushes | `.github/workflows/docs.yml:3-12`, `20-36` |
| CI enforcement matrix | Jobs: lint, typecheck, mypy-win32, tests (py3.10–3.14), macOS sandbox, MCP v1 compat, packaged contract, docs build | `.github/workflows/tests.yml:16-398` |
| Coverage gate | `coverage report --fail-under=85` fails the build on coverage drop | `Makefile:197-202` |
| Release process documented | Release/changelog page wired into nav; contract freeze commands in Make | `mkdocs.yml:92`; `Makefile:9-32`; `docs/release.md` |

## Answers to Dimension Questions

### 1. Are contribution guides clear?

Yes, unusually so. `AGENTS.md:1-13` opens with scope and a table of contents, then gives exact commands for setup (`make sync`, `AGENTS.md:175-177`), focused tests (`uv run pytest -s -k <pattern>`, `AGENTS.md:204-217`), snapshot fixing (`AGENTS.md:219-230`), and a mandatory run order (`AGENTS.md:247-256`). It also defines *when* documentation needs which rigor (three verification tiers, `AGENTS.md:80-88`) and treats public API signatures as a compatibility contract (`AGENTS.md:108-116`). The PR template operationalizes this with a checkbox for the verification script (`.github/PULL_REQUEST_TEMPLATE/pull_request_template.md:17`). Two caveats: there is no file literally named `CONTRIBUTING.md` (GitHub's standard discovery path), and the guide assumes an agent-assisted workflow (skill invocations like `$code-change-verification`) that may read as noise to a purely human contributor.

### 2. Are examples comprehensive?

Nearly, with one gap. Coverage by extension type: agents/patterns (`examples/agent_patterns/`, cataloged at `docs/examples.md:7-22`), basic tools (`examples/basic/tools.py:14-27`), hosted/local tools incl. shell approval flows (`examples/tools/`, 17 files), memory/session backends (15 files, `examples/memory/`), MCP stdio/SSE/streamable-HTTP/hosted (`docs/examples.md:52-71`), third-party model providers (`examples/model_providers/`, 8 files), voice and realtime (`examples/voice/`, `examples/realtime/`), guardrails/HITL (`examples/basic/tool_guardrails.py`, `docs/examples.md:20-22`), and full multi-agent apps (`examples/customer_service/`, `examples/research_bot/`). Examples are kept executable at scale via the discovery-based runner (`examples/run_examples.py:4-10`) and are individually annotated in the docs catalog (`docs/examples.md:11-131`). The gap: **evals** have no dedicated example category — evaluation appears only as host-side validation scripts inside sandbox tutorials (`examples/sandbox/tutorials/dataroom_metric_extract/evals.py:240-302`).

### 3. Is API documentation available?

Yes, generated and CI-checked. mkdocstrings renders Google-style docstrings from `src/agents` directly (`mkdocs.yml:25-43`), a generator creates one stub page per public module (`docs/scripts/generate_ref_files.py:51-68`), the nav wires ~110 ref pages covering core, tracing, realtime, voice, and extensions (`mkdocs.yml:94-201`), and `watch: src/agents` enables live preview while coding (`mkdocs.yml:379-380`). A dedicated CI job runs `make build-docs` on docs-affecting PRs (`.github/workflows/tests.yml:371-398`), and deployment to gh-pages is automated (`.github/workflows/docs.yml:48-50`). Additionally, `llms.txt`/`llms-full.txt` provide machine-consumable doc indexes (`docs/llms-full.txt:44-112`).

### 4. Are there tutorials for key tasks?

Yes. Quickstart-to-production tracks exist for each modality: text (`docs/quickstart.md:1-30`), sandbox (`docs/sandbox_agents.md` + concepts guide, nav `mkdocs.yml:57-61`), voice (`docs/voice/quickstart.md:3-125`), realtime (`docs/realtime/quickstart.md`, nav `mkdocs.yml:62-65`). Task tutorials include adding tools (`docs/tools.md:303-402`, with expected schema output shown at `374-402`), implementing guardrails (`docs/guardrails.md:77+`), custom tracing processors (`docs/tracing.md:148-160`), choosing memory strategies (`docs/running_agents.md:274-292`, `docs/sessions/index.md:646-731` covers custom session implementations), and writing deterministic tests (`docs/testing.md` decision table at `7-23`). Deepest end-to-end tutorials live in `examples/sandbox/tutorials/` with fixture generators, Dockerized execution, expected artifacts, and eval scripts (`examples/sandbox/tutorials/dataroom_metric_extract/README.md:11-45`).

### Template/starter repos

No clear evidence found within this source. Searches for "template"/"starter" across `README.md`, `docs/index.md`, and `docs/examples.md` surfaced only the prompt-template feature example (`docs/examples.md:35`). The repo links to its JS/TS sibling (`README.md:8`) but references no official starter/template repository. External starter repos may exist outside this checkout; that is out of study scope.

## Architectural Decisions

1. **Docs-as-enforced-pipeline, not docs-as-artifacts.** The reference section is generated from source modules (`docs/scripts/generate_ref_files.py:51-68`) and rebuilt in CI on every docs-touching PR (`.github/workflows/tests.yml:393-395`), keeping API docs from drifting from code.
2. **Testing utilities shipped inside the SDK.** `ScriptedModel` lives at `src/agents/testing/model.py:249` and is publicly documented (`docs/testing.md:27-35`) — contributors and users share one deterministic testing model, and the repo's own `tests/README.md:7` mandates preferring it over bespoke mocks.
3. **Layered knowledge bases split by audience.** User behavior contracts in `docs/`, maintainer invariants in `.agents/references/` (with explicit inclusion criteria, `.agents/references/README.md:9-26`), and workflow automation in `.agents/skills/`. The reference map tells a contributor exactly which document gates which runtime change (`.agents/references/README.md:31-48`).
4. **Examples as a tested corpus.** Rather than static snippets, examples form a discoverable suite executed by a purpose-built runner with filtering, auto-approvals, and per-example logs (`examples/run_examples.py:6-10`; `Makefile:101-123`).
5. **Compatibility enforced mechanically.** The released-API contract is frozen to JSON and validated against prospective releases in CI (`.github/workflows/tests.yml:296-332`; `tests/README.md:48`), turning a documentation-adjacent promise ("public API stability") into a checkable artifact.

## Notable Patterns

- **Decision-table navigation**: nearly every long guide opens with an "I want to…" routing table — `docs/tools.md:15-23`, `docs/testing.md:9-23`, `docs/models/index.md:8-21` — reducing time-to-first-code.
- **Runnable-with-expected-output tutorials**: the function-tool tutorial shows the actual JSON schema output behind a collapsible note (`docs/tools.md:374-402`), teaching by verification rather than assertion.
- **Docs tiers matched to risk**: editorial edits skip full site builds; structural changes trigger generators + `make build-docs` (`AGENTS.md:80-88`) — proportionate process instead of uniform ceremony.
- **Translation-aware authoring rules**: English prose must state actor/scope/ordering explicitly because it is machine-translated to ja/ko/zh (`AGENTS.md:118-121`; generated trees `docs/ja/`, `docs/ko/`, `docs/zh/`).
- **Skill-encoded workflow**: repetitive governance (final review, PR summary, release prep) is captured as reusable skill definitions with scripts (e.g., the verification runner referenced from the PR template, `.github/PULL_REQUEST_TEMPLATE/pull_request_template.md:17`).

## Tradeoffs

- **Discoverability vs. density**: consolidating everything into `AGENTS.md` (~300 lines of policy before the structure guide) maximizes agent readability but gives human newcomers no `CONTRIBUTING.md` beacon and no short "first PR" path; the learning path must be assembled from `docs/quickstart.md` + `docs/examples.md`.
- **Breadth vs. maintenance cost**: 216 example files and four locale trees multiply the surface a contributor must keep consistent; mitigated by generation (`translate_docs.py`, ref stubs) but still a real tax — the docs-release-timing policy (`AGENTS.md:76-78`) exists precisely because this coupling bites.
- **SDK-bundled test doubles**: shipping `ScriptedModel` in-library couples test ergonomics to the public API surface (it is contract-frozen like other exports, `tests/README.md:48`), buying consistency at the price of slower evolution.
- **Ops-heavy examples README**: `examples/README.md:1-18` documents the runner lifecycle but not a curated beginner sequence; the pedagogical index lives separately in `docs/examples.md`, splitting the entry point.

## Failure Modes / Edge Cases

- **Stale example risk**: examples depend on live services and model names; the runner mitigates with inclusion flags and logging (`examples/README.md:16`), but nothing in-repo schedules periodic example health checks beyond manual `make examples-run`.
- **Guide drift across three copies of truth**: `AGENTS.md`, `CLAUDE.md`, and `.agents/references/` overlap; if they diverge, a contributor following `CLAUDE.md` may contradict a newer reference-map invariant (the map's own maintenance rules, `.agents/references/README.md:50-54`, only govern the references directory).
- **Unreleased-behavior docs trap**: the separate docs-only PR rule (`AGENTS.md:76-78`) prevents documenting unreleased features, but a contributor who misses it ships docs that mislead users of the latest release until coordination happens.
- **Eval gap**: without first-class eval examples, a contributor building agentic quality loops has no sanctioned pattern to copy and may improvise unsafe ones (the tutorials' host-side `evals.py` pattern exists but is buried in one tutorial subtree).

## Future Considerations

- Add a root `CONTRIBUTING.md` that links to `AGENTS.md`, a 15-minute "your first tool + test" path (`docs/tools.md:303` + `docs/testing.md:70`), and the example catalog — cheap fix, closes the standard-entry-point gap.
- Promote the sandbox-tutorial eval pattern into a general "testing agent quality" guide and an `examples/evals/` category.
- Reference or vendor an official starter/template project for new applications.
- Add a scheduled CI job that runs a smoke subset of `make examples-run` to catch bitrot in the 216-file corpus.

## Questions / Gaps

- **Template/starter repos**: no in-repo evidence; search boundary was `README.md`, `docs/*.md`, and `examples/**` for "template"/"starter"/"contribut". External ecosystems were out of scope per source isolation.
- **Evals**: no `evals` module in `src/agents` and no dedicated example category was found; conclusion rests on directory listings and grep over `examples/` and `docs/index.md`.
- **Human-first onboarding time**: the "under an hour" judgment is inferred from doc completeness (tool tutorial + minimal sample + test recipe), not measured with a real newcomer session; no evidence of usability testing exists in-repo.

---

Generated by Dimension 22.03 (docs-examples-contributor-workflow) against openai-agents-sdk.

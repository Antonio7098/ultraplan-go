# Source Analysis: pydantic-ai

## 22.03 Docs, Examples, and Contributor Workflow

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python 3.10–3.13 (uv workspace monorepo: `pydantic_ai_slim/`, `pydantic_graph/`, `pydantic_evals/`, `clai/`), MkDocs-style markdown docs with mkdocstrings API reference, pytest + pytest-examples CI |
| Analyzed | 2026-08-24 |

> Citation convention: file paths below are relative to the studied source root (`studies/agent-harness-study/sources/pydantic-ai`).

## Summary

Pydantic AI treats documentation as a tested build artifact rather than prose. Every Python code fence in `README.md`, `docs/`, and all three library packages is discovered by `tests/test_examples.py:109` (`find_examples('README.md', 'docs', 'pydantic_ai_slim', 'pydantic_graph', 'pydantic_evals')`) and executed and linted in CI against mocked models (`tests/test_examples.py:231-392`). The contributor surface has four layers: a process guide (`CONTRIBUTING.md`), an agent-oriented engineering contract (`AGENTS.md:58-101`), extracted coding rules (`agent_docs/index.md` plus topic guides), and per-directory instructions (14 directory-scoped `AGENTS.md` files listed in root `AGENTS.md`). Extension types are each covered by a dedicated tutorial: capabilities/hooks (`docs/capabilities/custom.md`, `docs/hooks.md`), toolsets (`docs/toolsets.md:859`), models (`docs/models/overview.md:114`), agents (`docs/extensibility.md:80-85`), evals (`docs/evals/` + runnable `examples/pydantic_ai_examples/evals/example_01_generate_dataset.py`), tracing (`docs/logfire.md`), and packaging/publishing conventions (`docs/extensibility.md:21-41`). The API reference is generated from source via mkdocstrings directives (e.g. `docs/api/agent.md:3`) across 31 pages under `docs/api/`, wired into a 1,261-line navigation manifest (`docs/navigation.yml`) whose links and anchors are CI-enforced (`.github/workflows/ci.yml:200-207`).

The main caveats: (1) the site build/render config lives in an external repo (`pydantic/unified-docs` — no `mkdocs.yml` exists in this repository; verified by glob), so contributors cannot render docs locally without that second checkout; (2) there is no template/starter project inside this source — starter material is delegated to the external `pydantic-ai-harness` repo and third-party listings; and (3) memory and policy/guardrail tutorials intentionally point at the external harness rather than being taught end-to-end here.

## Rating

**9 / 10.**

Rationale against the rubric:

- **7–8 bar ("clear model with tests, explicit interfaces, operational safeguards") is exceeded**: docs code examples are not just present but *executed* in CI (`tests/test_examples.py:389-392` runs `eval_example.run_print_check` on every fence); front-page drift between README and docs index is blocked by a parity test (`tests/test_docs_parity.py:49-59`) including an em-dash ban (`tests/test_docs_parity.py:62-64`); link/anchor resolution is CI-gated offline (`.github/workflows/ci.yml:203-207`) plus a weekly external-link sweep (`.github/workflows/link-check.yml:25`); an agentic docs-drift detector files issues when docs diverge from code (`.github/workflows/pydantic-ai-docs-drift.md:26`).
- **Toward 9–10 ("mature, durable, observable")**: contributor knowledge is versioned as machine-readable rule files with regression-tested workflow guards (`.github/scripts/agentic_workflow_guard.py` described in `.github/workflows/AGENTS.md`), and a shipped agent skill teaches agent-building patterns (`pydantic_ai_slim/pydantic_ai/.agents/skills/building-pydantic-ai-agents/SKILL.md:1-9`, 11 reference documents).
- **Why not 10**: docs rendering is not reproducible inside this repo (external `unified-docs` build; only assets/link checks run here per `AGENTS.md` "Development workflow" and `.github/workflows/manually-deploy-docs.yml:21-29`); no in-repo template/starter project; memory and policies are documented mostly as pointers to the external harness (e.g. `docs/index.md:44`, `docs/extensibility.md:43-45`).
- **The rubric's litmus test — "Can a new contributor add a tool in under an hour without asking for help?"** — Yes. `docs/tools.md` walks tool registration with schema/docstring extraction (`docs/tools.md:260-262`), `TestModel` allows verification with no API keys (`docs/testing.md:12-26`), and every snippet shown is already proven to run by CI. Adding a *core* feature is deliberately harder: it requires issue alignment and maintainer assignment first (`CONTRIBUTING.md:32-40`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Contribution guide (process) | Issue-first flow, "champion" model, PR review expectations, priority ordering | `CONTRIBUTING.md:3-12`, `CONTRIBUTING.md:42-50`, `CONTRIBUTING.md:52-89` |
| Contribution guide (mechanics) | `make install` setup, pre-commit, `make help`; bare `make` = format+lint+typecheck+testcov | `CONTRIBUTING.md:97-121`, `Makefile:89-90`, `Makefile:92-103` |
| Contributor coding standards | Rules extracted from PR review patterns; topic guides for docs/API design/architecture | `agent_docs/index.md:3-139` |
| Requirements of all contributions | Backward-compat, type safety, "comprehensive tests covering 100% of code paths", doc updates mandatory | `AGENTS.md:58-76` |
| Directory-scoped guidance | 14 `AGENTS.md` files enumerated for contributors/agents working in specific dirs | `AGENTS.md` (Repository structure/Coding Guidelines sections, root) |
| Docs-examples execution | All fences under README/docs/packages found, linted, and executed with mocked models | `tests/test_examples.py:109`, `tests/test_examples.py:231-392` |
| Example regeneration workflow | `make update-examples` rewrites doc example output via pytest `--update-examples` | `Makefile:81-83` |
| Standalone examples app | 20+ runnable example programs incl. realtime, RAG, SQL gen, Slack, evals | `examples/pydantic_ai_examples/` (dir listing), `examples/README.md:11` |
| Examples imported in CI | `test-examples` job imports every example module on Python 3.11–3.14 | `.github/workflows/ci.yml:477-498` |
| Evals extension coverage | Dedicated docs tree + 4 numbered runnable examples with datasets and custom evaluators | `docs/navigation.yml:599` section; `examples/pydantic_ai_examples/evals/example_01_generate_dataset.py` … `example_04_compare_models.py` |
| Memory extension coverage | Native `MemoryTool` tutorial w/ Anthropic backend subclassing; long-term memory delegated to external Harness | `docs/native-tools.md:698-765`, `docs/index.md:44` |
| Tracing/observability docs | Logfire/OTel instrumentation guide incl. span attribute tuning | `docs/logfire.md:16-18`, `docs/logfire.md:417` |
| Policy/guardrail tutorials | Guardrail capability walkthrough (PII redaction); deferred-tool approval resolved "from policy" | `docs/capabilities/custom.md:1037+`, `docs/capabilities/on-demand.md:383-409`, `docs/realtime/tools.md:104-135` |
| Custom toolset tutorial | "Building a Custom Toolset" section behind `AbstractToolset` | `docs/toolsets.md:859`, `docs/toolsets.md:6` |
| Custom model tutorial | "Custom Models" section; OpenAI-compatible shortcut noted | `docs/models/overview.md:114-117` |
| Capability authoring tutorial | Full lifecycle: typing deps, tools, instructions, settings, selection/resolution, hook tables, cancellation warnings | `docs/capabilities/custom.md:3-51`, `docs/capabilities/custom.md:367-377`, `docs/capabilities/custom.md:462-470` |
| Publishing extension packages | `pydantic-ai-<name>` naming convention, spec registration via `custom_capability_types` | `docs/extensibility.md:21-41` |
| Generated API reference | mkdocstrings object directives with curated member lists; 31 API pages incl. sub-packages | `docs/api/agent.md:3-27`, `docs/api/` (dir listing) |
| Navigation ownership | `navigation.yml` owns sidebar/routes/redirects; update required when moving pages; anchor pinning `{#custom-id}` | `CONTRIBUTING.md:139-156`, `docs/navigation.yml:1-1261` |
| Link/anchor enforcement | Offline lychee check incl. fragments fails the PR; weekly full-web sweep opens issues | `.github/workflows/ci.yml:200-207`, `.github/workflows/link-check.yml:25-34` |
| Front-page sync contract | README/docs-index mirrored examples must be code-identical; enforced in tests | `tests/test_docs_parity.py:15-26`, `tests/test_docs_parity.py:49-64` |
| Docs drift detection | Scheduled agentic workflow detects docs-vs-code mismatch and files `[docs-drift]` issues | `.github/workflows/pydantic-ai-docs-drift.lock.yml:26`, `.github/workflows/pydantic-ai-docs-drift.lock.yml:601-608` |
| PR guard automation | Unlinked non-docs PRs auto-closed with guidance; docs-only PRs exempt | `.github/workflows/pr-guard.yml:178-205` |
| Testing tutorial for users/contributors | `TestModel`/`FunctionModel` strategy, `ALLOW_MODEL_REQUESTS=False` safety switch | `docs/testing.md:9-14`, `docs/testing.md:16-32` |
| Agent-facing teaching artifact | Shipped skill `building-pydantic-ai-agents` v1.1.1 with 11 references (tools, capabilities, testing…) | `pydantic_ai_slim/pydantic_ai/.agents/skills/building-pydantic-ai-agents/SKILL.md:1-9`, `.../references/` (11 files) |
| Template/starter repos | None found in this source; nearest analogues are the external Harness repo and third-party ecosystem lists | grep `starter\|template repo\|cookiecutter` → no relevant hits; `docs/extensibility.md:43-45`, `docs/toolsets.md:906,935`, `docs/capabilities/third-party.md:25-35` |
| Docs build location | Rendering handled by external `unified-docs` repo; preview gated behind `trigger:docs` label | `CONTRIBUTING.md:150-151`, `.github/workflows/manually-deploy-docs.yml:21-29`, `ci.yml:183-184` comment |

## Answers to Dimension Questions

**1. Are contribution guides clear?**
Yes, unusually so. `CONTRIBUTING.md:3-12` states the decision rules up front (bug → issue with repro; feature → issue before code; fix → maintainer-assigned issue first) and warns explicitly that unaligned large PRs stall (`CONTRIBUTING.md:39-40`). It is honest about non-standard review behavior: priority-order review, maintainers may rewrite or supersede contributed code (`CONTRIBUTING.md:54-70`), automated review is advisory (`CONTRIBUTING.md:72-78`). Mechanical setup is complete and one-command (`CONTRIBUTING.md:97-137`, backed by `Makefile:11-19`). Depth goes further than most projects: domain acceptance criteria for new model integrations with quantitative thresholds (`CONTRIBUTING.md:158-165`) and extracted style law in `agent_docs/index.md` with topic guides (`agent_docs/index.md:131-138`). Clarity caveat: the volume is high — CONTRIBUTING + AGENTS.md + 14 directory AGENTS.md files + 4 topic guides — so a newcomer must know which layer to read.

**2. Are examples comprehensive?**
Coverage across the dimension's extension types is strong and *executable*: agents/tools/RAG/streaming/multi-agent in `examples/pydantic_ai_examples/` (20+ entries incl. `realtime_voice.py`, `rag.py`, `sql_gen.py`, `medical_agent_delegation.py`); evals with four numbered progression examples (`examples/pydantic_ai_examples/evals/example_01_generate_dataset.py` through `example_04_compare_models.py`); tracing via `docs/logfire.md`; memory via native `MemoryTool` with a concrete backend subclass (`docs/native-tools.md:698-765`); policies via guardrail/approval capability recipes (`docs/capabilities/custom.md:1037+`, `docs/realtime/tools.md:104-121`). Crucially, comprehensiveness is enforced: fences in README/docs/packages are executed and linted in CI (`tests/test_examples.py:104-130`, `231-392`), standalone example modules must import cleanly on four Python versions (`.github/workflows/ci.yml:477-498`), and stale printed outputs are auto-updatable (`Makefile:81-83`). Gaps: memory and guardrails-as-products are taught as pointers to the external Harness (`docs/index.md:44`, `docs/capabilities/overview.md:91-109` capability matrix marks Memory/Guardrails rows as "Harness"), so end-to-end in-repo coverage of those two is partial.

**3. Is API documentation available?**
Yes, generated. `docs/api/*.md` uses mkdocstrings directives with curated member allowlists (e.g. `docs/api/agent.md:3-27`), spanning core, models, toolsets, capabilities, evals, graph, UI, realtime (31 pages). Docstrings feed both the reference and tool schemas — griffe-based parameter extraction is a documented product feature (`docs/tools.md:260-262`) and griffe quirks are patched in-source (`pydantic_ai_slim/pydantic_ai/_griffe.py:41-95`). The published-site convention requires reference-style links `[Element][module.path]` for hover navigation (docs/AGENTS.md, "Documentation" braindump). Limitation: the generator/site config itself is out-of-tree (published by `pydantic/unified-docs`; this repo checks assets and links only — `ci.yml:183-198`), and docstring anchors aren't link-checked in CI (explicitly noted in docs/AGENTS.md braindump, rule about `.agents`/fragment checking scope).

**4. Are there tutorials for key tasks?**
Yes. Key tasks map to step-by-step guides: building a custom capability (`docs/capabilities/custom.md` — ~1,100 lines incl. typed-dependency example at :55-100, wrapper toolset at :151-191, lifecycle tables at :422-435, cancellation-teardown warnings at :462-465), quick hooks without subclassing (`docs/hooks.md:8-44`), custom toolsets (`docs/toolsets.md:859`), custom models (`docs/models/overview.md:114`), publishing packages (`docs/extensibility.md:21-41`), unit-testing agents (`docs/testing.md`), and a shipped agent-consumable skill mirroring these patterns (`building-pydantic-ai-agents/SKILL.md` + 11 references). Tutorials are written to be pasted-and-run because CI proves them.

## Architectural Decisions

- **Docs-as-tests.** Documentation snippets are first-class test subjects: `find_examples` harvests fences from README/docs/packages (`tests/test_examples.py:109`) and each becomes a parametrized pytest case with model inference mocked (`tests/test_examples.py:241-243`) and provider env keys stubbed (`tests/test_examples.py:278-318`). This makes doc rot a CI failure instead of a slow decay.
- **Layered contributor contracts by audience.** Human process (`CONTRIBUTING.md`), agent/human engineering law (`AGENTS.md:58-101`), distilled review-derived rules (`agent_docs/index.md`), and directory-scoped context (per-dir `AGENTS.md`s). The split keeps global invariants (backward compat, 100% path coverage, doc updates) separate from local mechanics.
- **Extension routing over accumulation.** The contribution guide actively diverts capabilities to the external Pydantic AI Harness or third-party `pydantic-ai-*` packages rather than growing core (`CONTRIBUTING.md:28-30`), with explicit upstreaming criteria once APIs stabilize.
- **Out-of-tree docs rendering, in-tree guarantees.** Site generation is delegated to `unified-docs` while this repo enforces everything checkable statically: anchors/links (`.github/workflows/ci.yml:200-207`), image weight (`ci.yml:198`), navigation manifest validity (`.github/scripts/test_docs_navigation_workflow.py` invoked at `ci.yml:72`), and front-page parity (`tests/test_docs_parity.py`).
- **Machine-operable contributor knowledge.** Skills (`SKILL.md` frontmatter at `pydantic_ai_slim/pydantic_ai/.agents/skills/building-pydantic-ai-agents/SKILL.md:1-9`) and gh-aw agentic workflows with compile-time guards (`.github/workflows/AGENTS.md` policy-guard table) treat agents as first-class contributors.

## Notable Patterns

- **Executed-docs pipeline details worth copying:** fence-level opt-outs are declarative (`{test="skip" lint="skip"}` per docs/AGENTS.md rules), cross-example dependencies resolve through a `requires=` prefix mechanism that stitches titled examples together (`tests/test_examples.py:340-346`), and deterministic output is produced by mocking `infer_model`, time, randomness, and HTTP (`tests/test_examples.py:241-258`).
- **Parity-by-test:** eight marker snippets must exist and be code-identical across `docs/index.md` and `README.md` after comment-stripping (`tests/test_docs_parity.py:15-24`, `_normalize` at :40-46) — a cheap solution to the two-surface README problem.
- **Self-describing failure paths in tutorials:** capability docs teach teardown discipline for spawned tasks and cancellation semantics (`docs/capabilities/custom.md:462-470`), and durable-execution limits are documented with the exact failure (workflow-task vs activity failure) (`docs/durable_execution/temporal.md:281`).
- **Progressive example numbering for evals:** `example_01_generate_dataset.py` → `example_04_compare_models.py` forms a curriculum inside `examples/pydantic_ai_examples/evals/`.
- **Ecosystem curation as documentation:** third-party toolsets/capabilities are catalogued in-repo (`docs/toolsets.md:906,935`, `docs/capabilities/third-party.md:25-35`), partially substituting for a starter-repo program.

## Tradeoffs

- **High gate friction vs maintainer throughput.** Requiring issue alignment and assignment before code, with possible auto-close of unassigned PRs (`CONTRIBUTING.md:37`, `pr-guard.yml:178-205`), filters noise but raises the floor for drive-by contributions; the project compensates by exempting trivial fixes and docs-only changes (`CONTRIBUTING.md:18-20`, `pr-guard.yml:182,196-198`).
- **Docs truthfulness vs docs portability.** Executing every fence yields near-zero doc rot but couples the test suite to a large mock surface (~260 lines of prompt→response scripting, `tests/test_examples.py:475-738`) that must evolve with every doc change.
- **Externalized rendering vs single-repo onboarding.** Keeping mkdocs config in `unified-docs` centralizes brand/site infra across Pydantic projects but means "build the docs locally" is impossible from this checkout alone — a real gap for contributors verifying visual changes (only a label-triggered PR preview exists, `CONTRIBUTING.md:150-151`).
- **Honest-but-unusual contribution messaging.** Stating that contributed code may be rewritten wholesale (`CONTRIBUTING.md:62-68`) sets expectations accurately but reads as discouraging to traditional OSS contributors; the champion model (`CONTRIBUTING.md:42-50`) mitigates by offering a non-code contribution path.

## Failure Modes / Edge Cases

- **Docstring-anchor blind spot:** CI checks fragment links in markdown files but "not on one inside a docstring" (docs/AGENTS.md braindump, rule tied to `.github/workflows/ci.yml`), so API-reference anchors can break undetected until rendered.
- **Example-skip escape hatch:** fences marked `{test="skip"}` bypass execution; the docs convention restricts skips to unavoidable cases (external services, credentials, non-determinism) precisely because untested examples were observed to drift (docs/AGENTS.md rule:941). Coverage of skipped fences therefore relies on review discipline.
- **Mock coupling:** `print_callback` regexes normalize nondeterministic output (`tests/test_examples.py:395-405`); a doc example whose output format changes outside these patterns will fail confusingly for doc authors unfamiliar with the harness.
- **Workflow-file self-testing gap:** PRs editing `pull_request_target` workflows (the ones enforcing PR policy) cannot exercise their own changes — they run from `main` (`.github/workflows/AGENTS.md`, "A PR that edits bots.yml…"), so guard regressions surface only post-merge.
- **Cross-repo dependency risk:** memory/guardrail/harness tutorials and the capability matrix are external URLs (`docs/index.md:44`, `docs/capabilities/overview.md:91-109`); upstream restructuring breaks learning paths here, caught only by the weekly link checker (`.github/workflows/link-check.yml:25-34`), not at PR time (offline mode excludes external hosts, `ci.yml:200-202`).

## Future Considerations

- Vendor a minimal `mkdocs`/preview configuration or document a one-command local render using `unified-docs`, closing the last big "can't verify locally" gap (`CONTRIBUTING.md:150-151` implies previews need maintainer labels).
- Add a `templates/` or cookiecutter-style starter for `pydantic-ai-<name>` capability packages to make the naming convention at `docs/extensibility.md:29` actionable in minutes.
- Mirror (or deeply link with summaries) the Harness memory/guardrails tutorials so the two extension types currently routed out-of-repo have in-repo on-ramps consistent with the others.
- Extend anchor validation to docstrings, or add a scheduled job that renders the API reference and checks fragments, eliminating the known docstring-anchor blind spot.

## Questions / Gaps

- **No evidence found** for any in-repo template/starter project: searches for `starter`, `template repo`, and `cookiecutter` across the source returned only unrelated matches (`tests/realtime/ws_cassettes.py:96`); the conclusion rests on absence of evidence plus positive evidence of external delegation (`docs/extensibility.md:43-45`).
- The exact mkdocstrings/MkDocs plugin configuration is not inspectable here (out-of-tree build), so claims about generated-nav behavior beyond the directive syntax in `docs/api/agent.md:3-27` could not be verified from this source alone.
- Whether the docs-drift detector's issue-filing loop actually catches regressions at useful precision could not be assessed from static files (its prompt is fetched at runtime from a Logfire-managed variable — `.github/workflows/pydantic-ai-docs-drift.lock.yml:1680`).

---

Generated by `22.03-docs-examples-and-contributor-workflow` against `pydantic-ai`.

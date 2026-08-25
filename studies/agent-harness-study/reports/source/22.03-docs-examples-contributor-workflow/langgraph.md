# Source Analysis: langgraph

## Dimension 22.03: Docs, Examples, and Contributor Workflow

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (monorepo of 8 libs under `libs/`, one JS/TS SDK stub), uv + Makefile + GitHub Actions |
| Analyzed | 2026-08-24 |

Citation convention: paths are relative to the source directory `sources/langgraph/`.

## Summary

LangGraph teaches contributors through **operational scaffolding rather than in-repo prose**. The repository carries a machine-oriented contributor contract — `AGENTS.md`/`CLAUDE.md` with a monorepo map and dependency graph (`AGENTS.md:3-55`), per-library Makefiles (`libs/langgraph/Makefile:114-132`), and a CI pipeline that enforces lint/test, SDK sync/async parity, and config-schema stability (`.github/workflows/ci.yml:58-157`). Governance is automated: external PRs must reference a maintainer-approved issue and the author must be assigned to it, enforced by a sophisticated label/comment/close workflow (`.github/workflows/require_issue_link.yml:226-441`) plus PR-title linting with fixed types/scopes (`.github/workflows/pr_lint.yml:19-45`). Starter projects are first-class: `langgraph new` scaffolds from three maintained template repos (`libs/cli/langgraph_cli/templates.py:10-25`, documented at `libs/cli/README.md:31-38`), and checkpointer extension authors get a runnable conformance suite (`libs/checkpoint-conformance/README.md:25-45`).

The tradeoff is that **user-facing teaching material has been externalized**: the `examples/` directory is explicitly "retained purely for archival purposes" (`examples/README.md:3`), most root notebooks are one-cell redirect stubs (`examples/tool-calling.ipynb`), and all docs URLs funnel to docs.langchain.com via a generated redirect site (`docs/generate_redirects.py:20,49-53`, deployed by `.github/workflows/deploy-redirects.yml:1-12`). No API-doc generation toolchain (mkdocs/sphinx/pdoc) exists in any `pyproject.toml`; the API reference lives at reference.langchain.com (`README.md:64`, `docs/llms.txt:31`). In-code docstrings partially compensate: they are Google-style, complete, and include runnable examples (`libs/langgraph/langgraph/graph/state.py:606-664`).

A new contributor can run the dev loop in minutes (`make format/lint/test` is documented and CI-enforced), but cannot complete a *contribution* without leaving the repo (external contributing guide) and passing an approval gate that requires maintainer interaction before a PR will survive.

## Rating

**6 / 10 — Present but inconsistent.**

Rationale against the rubric:

- The contributor *workflow* alone would score 7–8: explicit interfaces (Make targets, TEST variable at `AGENTS.md:11-17`), operational safeguards (CI matrix, schema-drift guard at `ci.yml:123-157`, SDK parity check at `.github/scripts/check_sdk_methods.py:36-65`), and governance automation (`require_issue_link.yml`).
- But the dimension's core question — does the repo teach how to add agents, tools, evals, memory, tracing, and policies? — is answered mostly *outside* the repo. In-repo examples are archived stubs (`examples/README.md:3-11`); there is no in-repo API-doc pipeline; no in-repo tutorial covers policies/tracing extensions; the historical example set never covered policy authoring at all.
- "Can a new contributor add a tool in under an hour without asking for help?" — For *using* tools, yes via external quickstart/templates; for *contributing* code changes, the mechanical loop is fast but the approval gate (issue must be pre-approved and assigned, `.github/PULL_REQUEST_TEMPLATE.md:33`) makes help-seeking structurally mandatory before any PR lands.

## Evidence Collected

Every entry includes a file path with line numbers relative to `sources/langgraph/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Contributor guide (agent-oriented) | Monorepo layout, required `make format/lint/test` before PR, `TEST=path/to/test.py make test` override | `AGENTS.md:3-17` |
| Library map | One-paragraph description of each of 8 libraries | `AGENTS.md:19-31` |
| Dependency map | ASCII diagram of downstream dependents; warning that changes propagate | `AGENTS.md:33-55` |
| Duplicated agent instructions | CLAUDE.md mirrors AGENTS.md verbatim for Claude Code sessions | `CLAUDE.md:1-57` |
| Root build orchestration | Root Makefile fans out install/lint/format/lock/test into every `libs/*` | `Makefile:9-68` |
| Per-lib dev workflow | `coverage`, `test` (auto-starts docker postgres/redis + dev server), `test_watch`, `lint`, `type`, `spell_check` targets | `libs/langgraph/Makefile:34-132` |
| PR checklist | Title format `TYPE(SCOPE)`, mandatory `Fixes #xx`, must run make format/lint/test, one-package rule, no lockfile edits without permission | `.github/PULL_REQUEST_TEMPLATE.md:13-35` |
| External-PR approval gate | External PRs closed unless linked to approved issue AND author assigned; auto-reopen on fix | `.github/PULL_REQUEST_TEMPLATE.md:33`, `.github/workflows/require_issue_link.yml:226-235,365-441` |
| Maintainer bypass path | Org-member reopen or label removal applies `bypass-issue-check`; non-members defensively re-labeled | `.github/workflows/require_issue_link.yml:174-212` |
| PR title linting | Enforced types (feat/fix/docs/...) and scopes matching library names | `.github/workflows/pr_lint.yml:19-45` |
| Issue intake | Blank issues disabled; forum/docs/API-reference contact links; separate docs-issue repo | `.github/ISSUE_TEMPLATE/config.yml:1-14` |
| CI structure | Path-filtered matrix over 8 Python libs; separate langgraph test matrix; concurrency cancellation | `.github/workflows/ci.yml:21-107` |
| SDK parity safeguard | AST-based check that Sync*/async client method sets match exactly | `.github/scripts/check_sdk_methods.py:36-65`, wired at `ci.yml:109-121` |
| Config-schema drift guard | Regenerates `schemas/schema.json` and fails if not committed | `.github/workflows/ci.yml:144-157` |
| Examples status | "retained purely for archival purposes and is no longer updated"; moved to consolidated docs | `examples/README.md:1-11` |
| Example stubs | Root notebooks contain only a redirect markdown cell | `examples/tool-calling.ipynb:1` (single-cell source array) |
| Historical example breadth | RAG variants (7 notebooks), multi-agent (2), HITL wait-user-input, chatbot simulation evals incl. LangSmith trajectory eval, LangSmith run-id tracing | `examples/rag/`, `examples/multi_agent/multi-agent-collaboration.ipynb`, `examples/human_in_the_loop/wait-user-input.ipynb`, `examples/chatbot-simulation-evaluation/langsmith-agent-simulation-evaluation.ipynb`, `examples/run-id-langsmith.ipynb` |
| In-repo tutorials (archived) | SQL agent notebook and TNT-LLM tutorial directory | `examples/tutorials/sql-agent.ipynb`, `examples/tutorials/tnt-llm/tnt-llm.ipynb` |
| Docs externalization | All legacy paths redirect to docs.langchain.com; catch-all 404 → overview page | `docs/generate_redirects.py:20,56-75,118-127` |
| Redirect deployment | GitHub Pages deploy triggered by `docs/**` changes | `.github/workflows/deploy-redirects.yml:1-12` |
| LLM-readable docs index | `llms.txt` lists Overview/Core Concepts/How-To/Tutorials/Reference links, all external | `docs/llms.txt:1-31` |
| API reference location | Hosted externally at reference.langchain.com; no sphinx/mkdocs/pdoc config anywhere in `libs/*/pyproject.toml` (searched) | `README.md:64`, `libs/prebuilt/README.md:23` |
| Docstring quality | `add_node` overload documents Args, `!!! warning` blocks, and a full runnable TypedDict example | `libs/langgraph/langgraph/graph/state.py:590-664` |
| Starter templates | Three templates (Deep Agent, Agent, New LangGraph Project) with Python/JS variants downloaded as ZIPs from dedicated template repos | `libs/cli/langgraph_cli/templates.py:10-25` |
| Template UX | Interactive numbered picker with language choice; `langgraph new [PATH] --template TEMPLATE_NAME` CLI | `libs/cli/langgraph_cli/templates.py:43-91`, `libs/cli/README.md:31-38` |
| Extension conformance suite | Checkpointer authors register `@checkpointer_test` and call `validate()`; checks blob round-trips, metadata, namespace isolation | `libs/checkpoint-conformance/README.md:19-45` |
| Security documentation | Generated threat model covering trust boundaries for all shipped libs; vulnerability reporting via GitHub advisories | `.github/THREAT_MODEL.md:1-40` |
| Prebuilt component docs | README shows `create_react_agent` end-to-end (model+tools→app) and `ToolNode` usage with links to hosted API reference | `libs/prebuilt/README.md:30-79` |

## Answers to Dimension Questions

1. **Are contribution guides clear?**
   Yes, with two caveats. The in-repo contract is precise and unambiguous for the mechanical loop: which commands to run (`AGENTS.md:5-17`), what the monorepo contains and what depends on what (`AGENTS.md:19-55`), and exactly what a PR must satisfy (`.github/PULL_REQUEST_TEMPLATE.md:13-35`). The canonical full contributing guide, however, is an external URL (`README.md:75`, `PULL_REQUEST_TEMPLATE.md:5`), and the issue-approval requirement means clarity of process ≠ autonomy of process: an outside contributor cannot proceed past step 1 without maintainer interaction (`require_issue_link.yml:382-401`).

2. **Are examples comprehensive?**
   Not anymore, in-repo. The archive spans agents/tools (`react-agent-from-scratch.ipynb`, `tool-calling.ipynb` — both stubs), memory/HITL (`human_in_the_loop/wait-user-input.ipynb`), evals (`chatbot-simulation-evaluation/*.ipynb`), tracing (`run-id-langsmith.ipynb`), and multi-agent patterns — but every root-level notebook is now a redirect shell, and nothing in-repo covers policy extension or the functional API. Coverage of the newer extension surfaces exists only as docstrings and tests, not examples.

3. **Is API documentation available?**
   Available but not generated from this repo. There is no Sphinx/MkDocs/pdoc dependency in any library manifest (grep over `libs/*/pyproject.toml` found none); the hosted reference at reference.langchain.com is linked everywhere (`README.md:64`, `libs/cli/README.md:22`, `libs/checkpoint-conformance/README.md:22`). The repo's own `docs/` directory contains only redirect plumbing and `llms.txt`. Quality of the eventual output depends entirely on docstring discipline, which is high where inspected (`state.py:590-664`).

4. **Are there tutorials for key tasks?**
   Two remain in-repo under `examples/tutorials/` (SQL agent, TNT-LLM) but sit inside the explicitly archived tree (`examples/README.md:3`); current tutorials, the quickstart, and even a free structured course (LangChain Academy, `README.md:73`) are external. For the specific key task of writing a safe checkpointer, the conformance suite README doubles as a mini-tutorial (`checkpoint-conformance/README.md:25-45`).

## Architectural Decisions

1. **Docs-as-redirects architecture.** Rather than maintaining a parallel docs site, the repo converts its entire `docs/` surface into SEO-preserving meta-refresh redirects with anchor forwarding and a catch-all 404 (`docs/generate_redirects.py:22-75,124-127`). This consolidates content in one place (docs.langchain.com) at the cost of in-repo self-containment.
2. **Governance encoded as automation, not prose.** Rules that other projects state in CONTRIBUTING.md are executable here: semantic PR titles validated by a pinned action (`pr_lint.yml:15-45`), issue-link + assignment enforcement with labeled comments and auto-close/reopen (`require_issue_link.yml:282-441`), and blank issues disabled in favor of structured forms (`ISSUE_TEMPLATE/config.yml:1`).
3. **Machine-first contributor contract.** AGENTS.md/CLAUDE.md target AI coding agents as first-class contributors, including docstring formatting law (no Sphinx double-backticks, `AGENTS.md:57`) and a dependency blast-radius map (`AGENTS.md:38-55`).
4. **Safeguard-by-CI for cross-cutting contracts.** Contracts that span packages are checked mechanically instead of documented as conventions: sync/async SDK parity via AST diffing (`check_sdk_methods.py:8-33`), and `langgraph.json` schema stability via regeneration-and-diff (`ci.yml:144-157`).
5. **Templates live in dedicated repos.** Starter projects are versioned independently and fetched at scaffold time (`templates.py:10-25`), decoupling template iteration from core releases.
6. **Extension correctness taught through test harnesses.** The checkpointer contract is taught by a shippable conformance package (`checkpoint-conformance/README.md:19-45`) rather than a how-to page — "here is the validator" instead of "here are the rules".

## Notable Patterns

- **Dual agent instruction files**: AGENTS.md and CLAUDE.md are byte-similar duplicates so different agent harnesses pick up identical rules (`AGENTS.md:1` vs `CLAUDE.md:1`).
- **Make-target fan-out**: root Makefile iterates `libs/*` and delegates to nested Makefiles, keeping per-lib workflows uniform (`Makefile:21-68`).
- **Test ergonomics as onboarding**: `TEST=path/to/test.py make test` (`AGENTS.md:11-15`) and docker-service orchestration inside `make test` (`libs/langgraph/Makefile:61-77`) lower the cost of running realistic integration tests.
- **Defensive workflow scripting**: the issue-gate bot handles races (live-label refetch, `require_issue_link.yml:214-224`), non-maintainer label tampering (`:187-198`), comment deduplication via HTML markers (`:150-155`), and cancels orphaned runs after closing a PR (`:444-462`).
- **Archival-with-signposting**: stale content is kept but wrapped in prominent "moved" notices at directory level (`examples/README.md:7-11`) and file level (`tool-calling.ipynb` single redirect cell).
- **Runnable documentation in docstrings**: public APIs embed copy-pasteable examples with expected outputs (`state.py:634-660`).

## Tradeoffs

1. **Consolidation vs. self-containment.** Single-source-of-truth docs eliminate drift between repo and site, but a contributor working offline or through GitHub code search encounters dead-end stubs (`examples/tool-calling.ipynb`) and must context-switch to an external site mid-task.
2. **Strict gating vs. contribution velocity.** Mandatory pre-approved issues and assignment (`.github/PULL_REQUEST_TEMPLATE.md:33`) filter noise effectively but raise the floor cost of drive-by fixes; typo/doc PRs follow the same heavy gate.
3. **Executable rules vs. discoverability.** Encoding policy in workflows makes it unambiguous, but the *reasoning* behind rules lives in YAML comments (e.g., why `pull_request_target` never checks out head code, `require_issue_link.yml:16-17`) that contributors won't browse.
4. **Docstring-driven API docs vs. verified docs.** Reference quality tracks code automatically, yet nothing in-repo compiles/tests docstring examples; correctness of examples depends on review discipline (the AGENTS.md backtick rule only normalizes formatting).
5. **Externalized templates vs. supply-chain surface.** Scaffolding downloads ZIPs over plain HTTPS from `main` branches of template repos with no pinning (`templates.py:104-114`), trading reproducibility for always-fresh templates.

## Failure Modes / Edge Cases

- **Stale-archive confusion**: a newcomer landing on `examples/rag/langgraph_agentic_rag.ipynb` may not realize until reading carefully that the entire tree is unmaintained; only `examples/README.md:3` says so, and subdirectories have no individual notices.
- **Gate false positives**: the issue-link checker caps verification at 5 linked issues (`require_issue_link.yml:243-250`); a valid PR referencing more could be mis-evaluated, though non-404 API errors deliberately fail loudly rather than close wrongly (`:266-277`).
- **Redirect drift**: legacy paths mapped in `docs/redirects.json` point at today's docs URLs; future docs reorganizations would silently strand old deep links despite the catch-all (`generate_redirects.py:56-75`).
- **Template availability coupling**: `langgraph new` hard-fails offline or if a template repo renames its default branch (URLs hardcode `archive/refs/heads/main.zip`, `templates.py:13-23`).
- **Docs bug reporting detour**: documentation issues go to a *different* repository (`ISSUE_TEMPLATE/config.yml:10-14`), so contributors fixing a docstring may file against the wrong tracker.
- **Two contributing surfaces**: guidance split across AGENTS.md (mechanics), PR template (checklist), and the external guide (policy) can disagree silently over time since only the first two are versioned here.

## Future Considerations

- Add a short top-level CONTRIBUTING.md that summarizes the external guide and links the approval-gate flow, so the repo is navigable without network access.
- Mark each archived notebook with a banner cell (currently only some have redirects) or add per-directory README stubs mirroring `examples/README.md`.
- Pin starter-template revisions (tag or commit SHA) in `TEMPLATES` with an opt-in `--latest` flag for reproducible scaffolds (`templates.py:10-25`).
- Doctest or compile docstring examples in CI (e.g., a `--doctest-modules` slice) to convert the existing docstring discipline into verified documentation.
- Extend the conformance-harness pattern beyond checkpointers (e.g., a serializer or store conformance package) to give other extension authors the same teach-by-validator experience.

## Questions / Gaps

- **In-repo API-doc generation**: none found. Searched all `libs/*/pyproject.toml` for mkdocs/sphinx/pdoc and `.github/workflows/` for doc-publish jobs; only `deploy-redirects.yml` touches `docs/`. Where reference.langchain.com builds from remains unverifiable inside this source.
- **Policy-extension teaching material**: no example, guide, or tutorial for authoring custom policies (retry/cache/timeout/trace policy types appear as parameters at `state.py:598-603` but have no in-repo worked example). Possibly covered in the external docs; not assessable here.
- **Effectiveness of the external contributing guide** (clarity, accuracy) cannot be evaluated — it is outside this source boundary.
- **Whether archived notebooks still execute** against current APIs was not tested (would require installing deps); their stub status suggests they are not expected to.

---

Generated by `dimensions/22.03-docs-examples-and-contributor-workflow.md` against `langgraph`.

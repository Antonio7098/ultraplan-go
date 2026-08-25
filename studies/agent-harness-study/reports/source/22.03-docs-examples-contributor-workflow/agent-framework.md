# Source Analysis: agent-framework

## Dimension 22.03: Docs, Examples, and Contributor Workflow

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework (Microsoft Agent Framework) |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (uv + poethepoet, multi-package monorepo), C#/.NET (dotnet CLI), Go (external repo stub) |
| Analyzed | 2026-08-24 |

> Citation convention: all `file:line` paths below are relative to the source root
> `studies/agent-harness-study/sources/agent-framework/`.

## Summary

Microsoft Agent Framework treats contributor enablement as a first-class product surface. The repo layers contributor documentation at four levels: a top-level `CONTRIBUTING.md` with an issue-first PR workflow and an automated .NET API-compatibility gate (`CONTRIBUTING.md:42-106`); per-language dev guides with exact build/test/lint commands (`python/DEV_SETUP.md:152-189`, `dotnet/AGENTS.md:5-15`); a large structured sample corpus organized as a progressive curriculum (`python/samples/01-get-started/README.md:18-27`, `dotnet/samples/AGENTS.md:8-61`); and an ADR + spec process for design decisions (`docs/decisions/README.md:7-24`, `docs/specs/spec-template.md`). Unusually, it also maintains dedicated instruction files for AI coding agents — `AGENTS.md` per language plus task-scoped "skills" directories (`python/.github/skills/`, `dotnet/.github/skills/build-and-test/SKILL.md`) — and CI workflows that daily re-execute samples end-to-end with AI-assisted output verification (`.github/workflows/python-sample-validation.yml:4-6,47-53`, `.github/workflows/dotnet-verify-samples.yml:1-24`). The main gaps are API reference docs: user-facing API documentation is hosted externally on Microsoft Learn (linked from `README.md:100-105`) rather than generated in-repo, the in-repo doc-generation workflow references a `docs-full` poe task that does not exist in this tree (`.github/workflows/python-docs.yml:36-38`), and there is no in-repo template/starter project.

## Rating

**8 / 10** — Clear contributor model with tests, explicit interfaces, and operational safeguards. Contribution guides are unusually complete (issue-first workflow, breaking-change policy enforced by Package Validation in `CONTRIBUTING.md:77-106`, 85% coverage gate in `python/DEV_SETUP.md:187`), and examples cover every extension type with ~492 Python sample files and 248 .NET sample projects that are linted, type-checked, and continuously re-validated by CI. It falls short of 9–10 because (a) generated API reference documentation is not produced inside this repo and its generation pipeline has a dangling reference (`poe docs-full` is undefined; upload step commented out at `.github/workflows/python-docs.yml:39`), (b) no template/starter repo exists here, and (c) minor guideline drift exists between `python/samples/SAMPLE_GUIDELINES.md` and `python/samples/AGENTS.md`.

## Evidence Collected

Every entry includes a file path with line numbers relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Contributing guide | Full guide: reporting issues, DOs/DON'Ts, breaking-change policy, step-by-step PR workflow, per-language setup, CI expectations | `CONTRIBUTING.md:1-177` |
| Breaking-change enforcement | .NET Package Validation runs on build/pack against published NuGet baseline; suppression-file instructions for approved breaks | `CONTRIBUTING.md:77-106`; config referenced at `dotnet/nuget/nuget-package.props` |
| Issue-first API policy | "DON'T make new APIs without filing an issue" | `CONTRIBUTING.md:69` |
| Python dev setup | uv install steps, venv creation, `uv run poe setup/install/prek-install` | `python/DEV_SETUP.md:53-77` |
| Python test commands | `poe test -P <pkg>` fan-out vs `-A` aggregate sweep; integration tests auto-skip without keys | `python/DEV_SETUP.md:122-158` |
| Coverage gate | "CI automatically enforces at least 85% line coverage for every package classified Beta or Production/Stable" | `python/DEV_SETUP.md:176-189` |
| Python coding standard | Google-style docstrings required for all public functions/classes/modules | `python/CODING_STANDARD.md:5-24`; enforced via ruff `"D"` pydocstyle checks at `python/pyproject.toml:131` |
| .NET conventions | XML docs required on all public methods/classes; UTF-8 BOM; copyright headers; AAA test comments | `dotnet/AGENTS.md:34-43` |
| XML docs observed | 12 `<summary>` blocks in `AIAgentBuilder.cs`; 22 in `ChatClientAgent.cs` | `dotnet/src/Microsoft.Agents.AI/AIAgentBuilder.cs:1-12`; `dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientAgent.cs` |
| ADR process | Numbered ADR procedure with full/short templates, proposed→accepted lifecycle, decider sign-off via PR approval | `docs/decisions/README.md:7-24`; templates at `docs/decisions/adr-template.md`, `docs/decisions/adr-short-template.md` |
| ADR corpus | 38 numbered decision records incl. tools, middleware, telemetry, hosting | `docs/decisions/0002-agent-tools.md`, `docs/decisions/0007-agent-filtering-middleware.md`, `docs/decisions/0021-agent-skills-design.md` |
| Specs with test mapping | Function-calling loop spec defines required behavior + scenario-to-test mapping; changes gated on spec updates | `docs/specs/004-python-function-calling-loop.md`; template at `docs/specs/spec-template.md` |
| AI-agent contributor docs | Root Copilot instructions route to per-language AGENTS.md; python AGENTS.md indexes 7 skills (development, testing, code-quality, feature-lifecycle, package-management, pull-requests, release) | `.github/copilot-instructions.md:1-17`; `python/AGENTS.md:9-18`; `python/.github/skills/python-development/SKILL.md` et al. |
| .NET agent skills | Build-and-test and project-structure skill files exist under dotnet | `dotnet/.github/skills/build-and-test/SKILL.md:1-7`; `dotnet/.github/skills/project-structure/SKILL.md:1-2` |
| PR/issue templates | Structured PR template with motivation/description/review-focus sections; language-specific issue templates | `.github/pull_request_template.md:1-30`; `.github/ISSUE_TEMPLATE/dotnet-issue.yml`, `python-issue.yml`, `feature-request.yml` |
| Sample corpus size | 492 Python sample `.py` files; 248 .NET sample `.csproj` projects (counted via find) | `python/samples/**`, `dotnet/samples/**` |
| Sample curriculum (Python) | 7-step get-started README table from hello-agent to graph workflows | `python/samples/01-get-started/README.md:18-27` |
| Sample curriculum (.NET) | Documented directory layout with one-concept-per-project principle | `dotnet/samples/AGENTS.md:8-61,63-76`; `dotnet/samples/README.md:13-19` |
| Tool-extension example | `@tool` decorator demo: ~60-line self-contained sample with snippet tags and approval-mode safety note | `python/samples/01-get-started/02_add_tools.py:20-31,44-58` |
| Tools concept coverage | 19 tool samples indexed: explicit schemas, approvals, invocation caps, DI, progressive exposure | `python/samples/02-agents/tools/README.md:5-40` |
| Provider coverage (.NET) | AgentProviders folders for anthropic, azure, custom, dapr, foundry, github-copilot, google-gemini, ollama, onnx, openai | `dotnet/samples/02-agents/AgentProviders/` (layout documented at `dotnet/samples/AGENTS.md:19-29`) |
| Provider coverage (Python) | Per-provider folder with README index | `python/samples/02-agents/providers/README.md` (referenced from `python/samples/01-get-started/README.md:31`) |
| Extension-type coverage | Dedicated sample trees for middleware, memory/RAG, MCP, A2A, AG-UI, observability, evaluation, declarative, skills, hosting, harness | `python/samples/02-agents/{middleware,mcp,observability,evaluation,skills}/`; `dotnet/samples/02-agents/{AgentWithMemory,AgentWithRAG,ModelContextProtocol,Evaluation,Harness}/` |
| Workflow samples | Checkpoint/resume, HITL, orchestration (handoff/magentic), declarative YAML workflows | `dotnet/samples/03-workflows/{Checkpoint,HumanInTheLoop,Orchestration,Declarative}/`; `python/samples/03-workflows/{checkpoint,human-in-the-loop,orchestrations,declarative}/` |
| Migration guides | AutoGen and Semantic Kernel migration sample trees + SK→AF upgrade prompt for automated migration agents | `python/samples/autogen-migration/`; `python/samples/semantic-kernel-migration/`; `.github/upgrades/prompts/SemanticKernelToAgentFramework.md:1,1440-1460` |
| Declarative templates | Ready-to-run YAML agent/workflow definitions acting as lightweight starter templates | `declarative-agents/agent-samples/{azure,chatclient,foundry,openai}/`; `declarative-agents/workflow-samples/CustomerSupport.yaml` |
| Hosted docs (API reference) | MS Learn overview/quickstart/tutorials/user-guide links are the canonical API docs | `README.md:98-105`; also `dotnet/README.md:23-27` |
| Doc-gen pipeline gap | Release-triggered "Python - Create Docs" workflow calls `uv sync --all-packages --dev --docs` and `uv run poe docs-full`; upload to Learn is commented out | `.github/workflows/python-docs.yml:3-6,35-39` |
| Missing poe task verified | No `docs-full` target or `--docs` dependency group found in any pyproject/shared_tasks file | searched `python/pyproject.toml`, `python/shared_tasks.toml`, `python/packages/*/pyproject.toml` (no matches) |
| Sample CI validation (Python) | Daily scheduled validation executes get-started samples against live Foundry, saves playbooks and reports | `.github/workflows/python-sample-validation.yml:4-6,47-60` |
| Sample CI verification (.NET) | Weekday workflow builds and executes sample projects with deterministic + AI-powered output verification | `.github/workflows/dotnet-verify-samples.yml:1-24` |
| Samples type-checked | Samples checked by pyright in relaxed profile; exclusion list for samples needing external packages | `python/samples/SAMPLE_GUIDELINES.md:111-115`; configs `python/pyrightconfig.samples.json`, `python/pyrightconfig.samples.py310.json` |
| Snippet tags for docs | Named `# <snippet_name>` regions embedded in samples for Learn `:::code` integration | `python/samples/AGENTS.md` (Snippet tags section); `dotnet/samples/AGENTS.md:108-116` |
| External extension repos | Go SDK and Durable Task/Azure Functions extensions live in separate repos, linked from README | `README.md:16-17,171,180`; stub `go/README.md:1-3` |
| Community support channels | Discord, weekly office hours, support/security policies, transparency FAQ | `COMMUNITY.md:1-40` (office hours section); `SUPPORT.md:1-30`; `SECURITY.md:1-30`; `TRANSPARENCY_FAQ.md:1-20` |

## Answers to Dimension Questions

**1. Are contribution guides clear?**
Yes — exceptionally so. `CONTRIBUTING.md:108-133` gives a nine-step fork→branch→test→PR workflow including reviewer-conversation etiquette (`CONTRIBUTING.md:135-150`). Each language has its own setup guide with literal commands (`CONTRIBUTING.md:152-168`: `uv run poe build`, `uv run poe test -A -m "not integration"`, `dotnet test --filter-query ...`). Safety rails are explicit: breaking changes are rejected by policy and detected mechanically by .NET Package Validation with documented CP000x remediation steps (`CONTRIBUTING.md:71-106`), and new public APIs require an issue first (`CONTRIBUTING.md:69`). Design-level contributions follow a written ADR process with templates and sign-off rules (`docs/decisions/README.md:9-24`). One caveat: high-risk areas (the Python function-calling loop) require external contributors to check with the core team first (`python/AGENTS.md`, Function-Calling Loop Changes section).

**2. Are examples comprehensive?**
Yes. All major extension types have dedicated, indexed sample sets in both languages: tools (`python/samples/02-agents/tools/README.md:5-40`), middleware (`python/samples/02-agents/middleware/`), memory/context providers (`dotnet/samples/02-agents/AgentWithMemory/` — 8 steps covering Mem0, Valkey, CosmosNoSql, Foundry), RAG/vector stores (`dotnet/samples/02-agents/AgentWithRAG/` — 5 steps), tracing/observability (`python/samples/02-agents/observability/`, `dotnet/samples/02-agents/AgentOpenTelemetry/`), evaluation (`dotnet/samples/02-agents/Evaluation/`), MCP client+server+auth (`dotnet/samples/02-agents/ModelContextProtocol/`), multi-agent protocols (A2A, AG-UI), declarative YAML agents (`declarative-agents/agent-samples/`), hosting (`python/samples/04-hosting/`), and even agent-harness construction (`dotnet/samples/02-agents/Harness/Harness_Step01_Research` … `Step05_Loop`). Ten model providers have .NET provider samples (`dotnet/samples/02-agents/AgentProviders/`). Migration from AutoGen and Semantic Kernel is covered by dedicated sample trees (`python/samples/autogen-migration/`, `python/samples/semantic-kernel-migration/`).

**3. Is API documentation available?**
Partially. Canonical API reference lives on Microsoft Learn (`README.md:100-105`), not in this repository. The raw material for generated docs exists and is enforced — XML doc comments are mandatory on all public .NET APIs (`dotnet/AGENTS.md:39`, verified present in `dotnet/src/Microsoft.Agents.AI/AIAgentBuilder.cs` and `ChatClientAgent.cs`) and Google-style docstrings are enforced for Python via ruff's `"D"` ruleset (`python/pyproject.toml:131`, standard at `python/CODING_STANDARD.md:5-24`) — but the in-repo generation pipeline is incomplete: `.github/workflows/python-docs.yml:36-38` invokes `uv sync --all-packages --dev --docs` and `uv run poe docs-full`, neither of which is defined anywhere in the Python workspace files I searched, and the upload step is commented out at line 39. No docfx/mkdocs/sphinx/pdoc configuration exists in the tree (searched `*.md,yml,yaml,json,props,csproj,toml`). So: source annotations yes, generated API docs in-repo no.

**4. Are there tutorials for key tasks?**
Yes. Both languages ship a numbered progressive tutorial (`python/samples/01-get-started/README.md:18-27`: hello agent → tools → multi-turn → memory → functional workflow → graph workflow; `dotnet/samples/01-get-started/01_hello_agent` → `06_host_your_agent`), plus workflow-specific `_start-here` tracks (`dotnet/samples/03-workflows/_StartHere/01_Streaming` … `07_WriterCriticWorkflow`). External MS Learn tutorials and a user guide are linked from `README.md:98-105`. The "add a tool" tutorial question is answered directly by `python/samples/01-get-started/02_add_tools.py:20-58` — a single `@tool`-decorated function wired into `Agent(tools=[...])`, roughly 60 readable lines, runnable after two env vars — comfortably under an hour.

## Architectural Decisions

- **Docs-as-curriculum**: samples are organized 01→05 with strictly increasing complexity and a "one concept per file/project" rule, documented as enforceable convention rather than folklore (`python/samples/AGENTS.md` design principles 1–2; `dotnet/samples/AGENTS.md:63-76`).
- **Dual audience documentation**: every convention is written twice — once for humans (DEV_SETUP.md, SAMPLE_GUIDELINES.md) and once for AI coding agents (AGENTS.md files, `.github/skills/*/SKILL.md`) — acknowledging that agents are now first-class contributors.
- **Mechanically enforced contribution quality**: instead of relying on review alone, the repo automates gates — Package Validation for API compatibility (`CONTRIBUTING.md:79-88`), an 85% per-package coverage floor (`python/DEV_SETUP.md:187`), five parallel type-checkers over tests/samples (`python/DEV_SETUP.md:314-325`), and markdown code-block linting (`python/DEV_SETUP.md:365-369`).
- **Samples treated as tested artifacts, not decoration**: samples carry snippet tags for Learn integration (`dotnet/samples/AGENTS.md:108-116`), are type-checked (`python/pyrightconfig.samples.json`), and are re-executed daily against live services with saved "playbooks" (`.github/workflows/python-sample-validation.yml:47-60`) or deterministic+AI output verification on weekdays (`.github/workflows/dotnet-verify-samples.yml:1-10`).
- **Design decentralization via ADRs/specs**: architectural intent is captured in 38 numbered ADRs and a specs directory with scenario-to-test mappings (`docs/decisions/`, `docs/specs/004-python-function-calling-loop.md`), keeping rationale durable across contributor turnover.

## Notable Patterns

- **Progressive numbering convention**: `01_get_started…05_end_to_end` tiers repeated identically in Python and .NET (`python/samples/` vs `dotnet/samples/AGENTS.md:9-61`), making cross-language navigation transferable.
- **Per-package READMEs as mini API docs**: 35 Python package READMEs and 9 .NET package READMEs document install/lifecycle/usage at package granularity (`python/packages/*/README.md`).
- **Safety guidance embedded in samples themselves**: e.g., the approval-mode note warning that `approval_mode="never_require"` is for brevity only and `"always_require"` belongs in production (`python/samples/01-get-started/02_add_tools.py:22-24`); credential-choice warnings in `dotnet/samples/AGENTS.md:91-93`.
- **Sample authoring contract**: PEP 723 inline metadata keeps samples self-contained without polluting dev dependencies (`python/samples/SAMPLE_GUIDELINES.md:20-37`); mandated file structure order (`SAMPLE_GUIDELINES.md:5-18`).
- **AI-verifiable sample execution**: the Python sample-validation system stores replayable playbooks and reports (`python/scripts/sample_validation/playbook.py`, `replay_executor.py`), turning "docs still work" into a monitored property.

## Tradeoffs

- **Centralized Learn docs vs in-repo docs**: hosting API reference on learn.microsoft.com keeps polished docs but makes the repo self-inconsistent for offline/external contributors; the dangling `docs-full` reference shows the split has already caused drift (`.github/workflows/python-docs.yml:36-39`).
- **Strictness vs contribution friction**: issue-first API policy, ADR requirements, and "don't surprise us with big PRs" (`CONTRIBUTING.md:63-69`) protect architecture but raise the cost of drive-by contributions; the repo compensates with very precise local commands.
- **AI-powered sample validation vs quota/cost**: the AI-based validators depend on Copilot quota — evidence: the `validate-02-agents` job is disabled with `if: false # Temporarily disabled - to free up Copilot quota for other jobs` (`.github/workflows/python-sample-validation.yml:62-64`), meaning most concept samples currently lack continuous live validation.
- **Monorepo breadth vs doc depth per language**: Python receives the richest contributor scaffolding (CODING_STANDARD, DEV_SETUP, PACKAGE_STATUS, CHANGELOG); .NET relies more on AGENTS.md skills; Go is a pointer stub (`go/README.md:1-3`), so guidance quality is uneven across languages.

## Failure Modes / Edge Cases

- **Broken doc-generation path**: a release would run `poe docs-full` and fail (task undefined; `--docs` group undefined) — `.github/workflows/python-docs.yml:35-39`. Verified by searching all `pyproject.toml`/`shared_tasks.toml` files.
- **Guideline drift between overlapping docs**: `python/samples/SAMPLE_GUIDELINES.md:11-12` requires `load_dotenv()` in every sample while the same repo's `python/samples/AGENTS.md` forbids it in basic samples; `SAMPLE_GUIDELINES.md:130-131` contains open TODOs ("Update folder structure to our new needs") and a naming rule (`step<number>_<name>.py`) that current files violate (`01_hello_agent.py`). A contributor following either file alone produces inconsistent PRs.
- **Silently disabled safeguards**: sample validation jobs can be switched off by quota without changing docs (`.github/workflows/python-sample-validation.yml:64`), degrading the guarantee that examples actually run.
- **Referenced content living outside the source**: durable/Azure Functions and Go guidance point to sibling repos (`README.md:17,171`); a reader cannot complete those paths from this repository.
- **Skill-path ambiguity**: root `.github/skills/` contains only `pull-requests` while language skills live under `python/.github/skills/` and `dotnet/.github/skills/`; cross-language navigation of these instructions depends on correctly resolving relative scopes (e.g., `dotnet/AGENTS.md:7` resolves within `dotnet/`, not repo root).

## Future Considerations

- Restore or remove the Python API-doc pipeline: define the `docs-full` poe target and `--docs` group (or switch to mkdocs/pdoc config committed in-tree) so `.github/workflows/python-docs.yml` becomes executable end-to-end.
- Reconcile `python/samples/SAMPLE_GUIDELINES.md` with `python/samples/AGENTS.md` (env-var policy, naming format, stale TODOs at lines 129-131) into a single source of truth.
- Add a minimal in-repo starter/template project (or `dotnet new` template / cookiecutter) to complement `declarative-agents/` YAML samples; none was found (searched `*template*`, `*starter*` excluding ADR/spec templates).
- Re-enable or replace quota-dependent AI sample validation (`.github/workflows/python-sample-validation.yml:64`) with cheaper deterministic smoke checks so coverage of `02-agents` samples doesn't silently lapse.
- Mirror the Python-grade contributor scaffolding (coding standard doc, changelog discipline) for .NET, which currently leans on `dotnet/AGENTS.md` conventions alone.

## Questions / Gaps

- Where exactly do the Learn API references get built? The upload step is commented out (`.github/workflows/python-docs.yml:39`) and no OpenPublishing/docfx config exists in this tree; the actual publishing mechanism is not observable from this source (searched all workflows and config files). Stated as "No evidence found in this repository."
- Whether the missing `poe docs-full` task was recently removed or never landed could not be determined from the snapshot (no git archaeology performed beyond working tree inspection).
- Effectiveness of the daily sample-validation reports (artifact-only, uploaded at `.github/workflows/python-sample-validation.yml:55-60`) — whether failures block merges or merely notify could not be confirmed from workflow definitions alone.

---

Generated by dimension 22.03 (Docs, Examples, and Contributor Workflow) against `agent-framework`.

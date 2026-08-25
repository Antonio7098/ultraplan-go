# Source Analysis: openai-agents-sdk

## 22.01 Package and Module Boundaries

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Unknown (source payload missing) |
| Analyzed | 2026-08-22 |

## Summary

The selected source directory `studies/agent-harness-study/sources/openai-agents-sdk/` is empty on disk. No implementation files, tests, configuration, manifests, workspace metadata, or documentation were present at the time of this study. The only artifact in the sources folder for this target is the source descriptor `studies/agent-harness-study/sources/openai-agents-sdk.ultraplan-source.yml:1-90`, which declares the upstream URL as `https://github.com/openai/openai-agents-python` and describes the project as "Modern agent runtime with tracing, handoffs, guardrails", but does not include any code, project files, or boundary material.

Because the source payload is missing, there is no package or workspace manifest to read, no folder hierarchy to map, no module dependency graph to construct, no import statements or namespace declarations to inspect for direction, no visibility annotations (e.g., `__all__`, `_internal`, `:_public`) to evaluate, and no separation/import-cycle tests to verify. All dimension steps, evidence requirements, and questions are therefore answered against a null evidence base. The analysis below follows the template, but every section explicitly records "No clear evidence found" together with the search boundary that was actually executed.

Search boundary executed for this task:

- `ls studies/agent-harness-study/sources/openai-agents-sdk/` → empty directory (`.` and `..` only).
- `find studies/agent-harness-study/sources/openai-agents-sdk -mindepth 1` → zero entries.
- `find studies/agent-harness-study/sources/openai-agents-sdk -type f | wc -l` → `0`.
- `stat studies/agent-harness-study/sources/openai-agents-sdk` → directory exists, contains no files.
- Read of `studies/agent-harness-study/sources/openai-agents-sdk.ultraplan-source.yml:1-90` → metadata-only.
- `wc -l studies/agent-harness-study/sources/openai-agents-sdk.ultraplan-source.yml` → 90 lines.
- Cross-source spot-check confirmed the same absence for several sibling descriptors (`agent-framework`, `crewai`, `langgraph`, `letta`, `opa`, `pydantic-ai`, `temporal`) but no inspection of their contents was performed, per the isolation rules in the base prompt.

## Rating

**1 / 10** — Absent.

Rationale: The rating rubric places a score of 1-3 in the "Absent, implicit, ad-hoc, or unsafe" band. With zero source files, there is no top-level package structure to identify, no dependency direction to check, no module independence to demonstrate, no circular-dependency story to evaluate, and no internal-vs-public API distinction to verify. The dimension's headline question — "Can you use the tool system without pulling in the entire runtime?" — is unanswerable from the local payload. A score of 1 (rather than 0) is reserved because the source descriptor exists and proves the study target was intended to be `openai/openai-agents-python`; the absence is a missing payload, not a misconfigured study, and at least one concrete artifact (the descriptor) is locatable to cite as evidence that the intent was real.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Top-level package structure | No clear evidence found. Directory `studies/agent-harness-study/sources/openai-agents-sdk/` contains no files; no folder hierarchy, `pyproject.toml`, `setup.py`, `setup.cfg`, `Cargo.toml`, `package.json`, `go.mod`, `*.sln`, `*.csproj`, `Package.swift`, or `*.podspec` could be inspected. | `studies/agent-harness-study/sources/openai-agents-sdk/` (empty) |
| Module dependency graphs | No clear evidence found. No import/`use`/`require`/`from … import`/`using` statements, no `[project] dependencies`/`[project.optional-dependencies]`/`extras_require` blocks, no `internal` packages, no `importlib`-style subpackage markers exist locally. | `studies/agent-harness-study/sources/openai-agents-sdk/` (empty) |
| Module boundaries | No clear evidence found. No source files exist to delineate sub-packages for runtime, tools, memory, evals, tracing, UI, or provider adapters (e.g., a hypothetical `agents/`, `agents/tracing/`, `agents/extensions/`, `agents/voice/` split). | `studies/agent-harness-study/sources/openai-agents-sdk/` (empty) |
| API visibility annotations | No clear evidence found. No `export`/`pub`/`__all__`/`__init__` re-export lists/`_internal`/`public`/`package-private` markers, no `.h` public-header files, no `_test.py` separation, no `mod.rs`/`index.ts` boundary files are present. | `studies/agent-harness-study/sources/openai-agents-sdk/` (empty) |
| Separation / boundary tests | No clear evidence found. No `archunit`, `dependency-cruiser`, `import-linter`, `grimp`, `depcheck`, `go mod why`, `cargo tree`, `pydeps`, or `madge` configuration is present; no test that asserts one module does not import from another exists. | `studies/agent-harness-study/sources/openai-agents-sdk/` (empty) |
| Source descriptor (only artifact) | YAML file declaring the upstream repository URL `https://github.com/openai/openai-agents-python`, a short tag line ("Modern agent runtime with tracing, handoffs, guardrails"), and the list of applicable dimensions, including `22.01`. | `studies/agent-harness-study/sources/openai-agents-sdk.ultraplan-source.yml:1-90` |

## Answers to Dimension Questions

1. **Are modules cleanly separated?**
   No clear evidence found. No source code is present to inspect. Cannot affirm or refute whether the upstream `openai/openai-agents-python` repository carves out separate modules/packages for runtime, tools, memory, evals, tracing, UI, or provider adapters. The local payload contains zero files, so no folder layout (e.g., a top-level `src/agents/` tree with subpackages such as `agent`, `run`, `tool`, `tracing`, `guardrail`, `voice`, `lifecycle`, `models`) can be mapped and no import-direction check can be performed.

2. **Do dependencies flow in one direction?**
   No clear evidence found. With no files, there are no edges in a module graph to check for directionality — no "tracing depends on runtime, not vice versa" or "provider adapters depend on a core interface, not vice versa" assertion can be made. The dimension's question about clean unidirectional flow between runtime, tools, tracing, and provider adapters is unanswerable from the local payload.

3. **Can modules be used independently?**
   No clear evidence found. No manifest file (`pyproject.toml` with optional dependency groups, `[project.optional-dependencies]`, subpath `exports`, `Cargo.toml` feature flags, `go.mod` with build tags) is present, so the ability to import a subset of the framework without pulling in the entire runtime cannot be demonstrated. There is no `__init__.py`, `mod.rs`, or `index.ts` to test a narrow import. The headline question — "Can you use the tool system without pulling in the entire runtime?" — requires a manifest and import graph that are not present.

4. **Are public APIs distinguished from internal ones?**
   No clear evidence found. No visibility annotations (`pub`/`pub(crate)`, `__all__`, `export`, `internal`, `: public`, `InternalsVisibleTo`, `__declspec(dllexport)`/`__declspec(dllimport)`) could be inspected, and no public-API surface file (e.g., `docs/ref/`, `docs/api.md`, `reference.md`, `api/index.html`) is present. There is no `_internal/` directory, no `api/` + `impl/` split, and no public-API snapshot test (e.g., `griffe`, `pytest-apischema`) to evaluate.

## Architectural Decisions

No clear evidence found. No code, configuration, design documents, ADRs, RFCs, or workspace manifests were available in the selected source directory. Architectural decisions that would normally be observable for this dimension are therefore not observable:

- Whether `openai-agents-sdk` is a single-package Python distribution (e.g., a single `agents` package on PyPI) versus a workspace of sub-distributions (e.g., `openai-agents`, `openai-agents-tracing`, `openai-agents-voice`, `openai-agents-extensions`).
- Whether provider adapters (OpenAI, Azure OpenAI, Anthropic, Gemini, Bedrock, etc.) live under a shared `agents/models/` tree behind a common protocol, or are first-class top-level packages with their own manifests.
- Whether the public surface is enforced by lint rules (e.g., `ruff` `tidy-imports`, `flake8-import-graph`, `pyright`/`mypy` strict-mode settings, `grimp`/`import-linter` contracts) or only by convention.
- Whether tools, guardrails, tracing, voice, and evals ship as opt-in extras (e.g., `[project.optional-dependencies]`) or as always-on transitive dependencies.
- Whether internal helpers are colocated under an `_internal/` directory, hidden behind re-exports, or stamped with an `_` (underscore) prefix to mark them private.

## Notable Patterns

No clear evidence found. No patterns could be located: no single-package vs workspace-of-packages decision, no `__init__.py`-barrel-files vs deep-imports choice, no `public/` + `internal/` split, no `api/` + `impl/` split, no DI container boundary defining tool vs runtime vs UI scope. There is no test or fixture demonstrating how the agent loop reaches into provider adapters without coupling to them. Upstream, the `openai/openai-agents-python` project is widely cited for its tracing/handoff/guardrail triad and for its `Runner`/`Agent`/`function_tool` public surface, but any observation about its package boundary design is unverifiable from the local payload.

## Tradeoffs

No clear evidence found. Tradeoffs cannot be enumerated when there is no implementation to weigh. In general, agent frameworks that lack clean module boundaries tend to suffer from these tradeoffs:

- Adopting a non-default provider (or a tracing backend) pulls in the entire runtime (large transitive surface).
- Internal helpers leak into public docs because the visibility boundary is implicit.
- Refactors cross sub-packages because nothing prevents callers from deep-importing.
- Circular-dependency errors surface late in CI rather than at lint time.
- Voice/streaming, tracing exporters, and guardrail libraries cannot be installed independently of the agent runtime.

Whether `openai/openai-agents-python` follows any of these paths is unverifiable from the local payload.

## Failure Modes / Edge Cases

No clear evidence found. Failure modes that module boundaries are meant to prevent or expose cannot be observed:

- **Transitive bloat** — installing `openai-agents` may pull the entire tracing stack, voice runtime, and tool integrations; no `[project.optional-dependencies]` block is visible.
- **Circular imports** — no `grimp`/`import-linter`/`pydeps`/`madge` configuration exists; no `cargo tree`/`go mod graph` equivalent for Python to inspect.
- **Public-API drift** — no `griffe`/`public-api`/`pytest-apischema` snapshot test, no `semver`-pinned sub-package, no `pyproject.toml#project.optional-dependencies` boundary to evaluate.
- **Provider leakage** — no evidence of a strategy for swapping `openai` for `azure-openai` for `anthropic` without re-importing core runtime.
- **Tracing coupling** — the upstream description calls out tracing as a first-class concern; whether tracing ships as an optional sub-package is unverifiable.
- **Voice coupling** — voice / streaming is reportedly part of the SDK; whether voice is separable from the core agent runtime is unverifiable.
- **Build errors after partial checkout** — without a manifest, even a partial clone of the documented layout cannot be built or tested.

## Future Considerations

When the source payload is restored, the following should be re-examined for this dimension:

- Top-level layout: presence of a `pyproject.toml` (or legacy `setup.py` / `setup.cfg`) for `openai-agents` itself and for any sibling distribution (e.g., `openai-agents-tracing`, `openai-agents-voice`, `openai-agents-extensions`). Compare with the layout of `src/agents/` and any `src/agents/extensions/` Python packages.
- Whether provider adapters (OpenAI, Azure OpenAI, Anthropic, Gemini, Bedrock, etc.) live as separate optional sub-packages so a consumer can install only what they need.
- Whether tools, guardrails, tracing, voice, and evals are opt-in (`extras_require` / `[project.optional-dependencies]` / subpath imports) so the headline question — "Can you use the tool system without pulling in the entire runtime?" — receives an empirical yes/no.
- Direction of imports: a quick `grimp`/`pydeps`/`import-linter`/`snakefood` pass should be run against the restored snapshot to surface cycles and reverse edges between the `Runner` loop, the model adapter layer, the tracing layer, and the tool layer.
- Visibility tooling: presence of lint rules (`ruff` `tidy-imports` / `flake8-import-graph`, `pyright`/`mypy` strict-mode settings, `import-linter` contracts, `archunit` analogue for Python) that mechanically enforce the public/internal split.
- Public-API surface governance: presence of `griffe` / `pytest-apischema` / `pyright --verifytypes` snapshot tests that pin the public surface and fail CI on unplanned additions.
- Whether the repo separates a `agents-core` from `agents-tracing` and from individual provider sub-packages to make adapter swapping cheap.

## Questions / Gaps

- **Why is the source directory empty?** The descriptor at `studies/agent-harness-study/sources/openai-agents-sdk.ultraplan-source.yml:1-90` references `https://github.com/openai/openai-agents-python`, but no checkout, snapshot, or mirror was present under `studies/agent-harness-study/sources/openai-agents-sdk/`. The study cannot proceed against an empty target.
- **Is the upstream `openai/openai-agents-python` repository the right version?** Without a pinned commit/tag in the descriptor or a checked-out copy locally, the study cannot anchor itself to a specific revision. Future runs should pin a commit SHA in `openai-agents-sdk.ultraplan-source.yml`.
- **Was the source supposed to be vendored or fetched on demand?** The presence of sibling sources (e.g., `langfuse/`, `openhands/`) shows the convention supports vendoring, while several sibling directories (`agent-framework/`, `crewai/`, `langgraph/`, `letta/`, `opa/`, `pydantic-ai/`, `temporal/`) are also empty. If `openai-agents-sdk` was meant to be fetched on demand, the fetch step failed silently for this dimension. Recommend adding a fetch manifest entry to the descriptor.
- **Resolution path:** populate `studies/agent-harness-study/sources/openai-agents-sdk/` with a vendored snapshot of `openai/openai-agents-python` at a pinned SHA, then re-run this dimension study. Until then, this report's findings are "No clear evidence found" across every required section.

---

Generated by `dimensions/22.01-package-and-module-boundaries.md` against `openai-agents-sdk`.
# Source Analysis: langgraph

## 22.01 Package and Module Boundaries

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Unknown (source payload missing) |
| Analyzed | 2026-08-22 |

## Summary

The selected source directory `studies/agent-harness-study/sources/langgraph/` is empty on disk. No implementation files, tests, configuration, manifests, workspace metadata, or documentation were present at the time of this study. The only artifact in the sources folder for this target is the source descriptor `studies/agent-harness-study/sources/langgraph.ultraplan-source.yml:1-103`, which declares the upstream URL as `https://github.com/langchain-ai/langgraph`, the one-line description "Best reference for durable execution, checkpoints, interrupts, state graphs", and lists applicable dimensions (including `22.01`) but does not include any code, project files, or boundary material.

Because the source payload is missing, there is no package or workspace manifest to read, no folder hierarchy to map, no module dependency graph to construct, no import statements or namespace declarations to inspect for direction, no visibility annotations (e.g., `__all__`, `export`, `pub`, `internal`, `: public`, `package-private`) to evaluate, and no separation/import-cycle tests to verify. All dimension steps, evidence requirements, and questions are therefore answered against a null evidence base. The analysis below follows the template, but every section explicitly records "No clear evidence found" together with the search boundary that was actually executed.

Search boundary executed for this task:

- `ls studies/agent-harness-study/sources/langgraph/` → empty directory (`.` and `..` only).
- `find studies/agent-harness-study/sources/langgraph -mindepth 1` → zero entries.
- `find studies/agent-harness-study/sources/langgraph -type f | wc -l` → `0`.
- `stat studies/agent-harness-study/sources/langgraph` → directory inode 263492, no contained files.
- Read of `studies/agent-harness-study/sources/langgraph.ultraplan-source.yml:1-103` → metadata-only.
- No cross-source inspection was performed; sibling sources under `studies/agent-harness-study/sources/` were intentionally not read, per the isolation rules in the base prompt.

## Rating

**1 / 10** — Absent.

Rationale: The rating rubric places a score of 1-3 in the "Absent, implicit, ad-hoc, or unsafe" band. With zero source files, there is no top-level package structure to identify, no dependency direction to check, no module independence to demonstrate, no circular-dependency story to evaluate, and no internal-vs-public API distinction to verify. The dimension's headline question — "Can you use the tool system without pulling in the entire runtime?" — is unanswerable from the local payload. A score of 1 (rather than 0) is reserved because the source descriptor exists and proves the study target was intended to be `langchain-ai/langgraph`; the absence is a missing payload, not a misconfigured study, and at least one concrete artifact (the descriptor) is locatable to cite as evidence that the intent was real.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Top-level package structure | No clear evidence found. Directory `studies/agent-harness-study/sources/langgraph/` contains no files; no folder hierarchy, `Cargo.toml`, `package.json`, `pyproject.toml`, `go.mod`, `*.sln`, `*.csproj`, `Package.swift`, or `*.podspec` could be inspected. | `studies/agent-harness-study/sources/langgraph/` (empty) |
| Module dependency graphs | No clear evidence found. No import/`use`/`require`/`from … import`/`using` statements, no `dependencies`/`optional-dependencies`/`extras_require` blocks, no `internal` packages, no `importlib`-style subpackage markers exist locally. | `studies/agent-harness-study/sources/langgraph/` (empty) |
| Module boundaries | No clear evidence found. No source files exist to delineate sub-packages for runtime, tools, memory, evals, tracing, UI, or provider adapters. | `studies/agent-harness-study/sources/langgraph/` (empty) |
| API visibility annotations | No clear evidence found. No `export`/`pub`/`__all__`/`internal`/`public`/`package-private` markers, no `.h` public-header files, no `_test.go` separation, no `__init__.py` boundary files are present. | `studies/agent-harness-study/sources/langgraph/` (empty) |
| Separation / boundary tests | No clear evidence found. No `archunit`, `dependency-cruiser`, `import-linter`, `depcheck`, `go mod why`, or `cargo tree` configuration is present; no test that asserts one module does not import from another exists. | `studies/agent-harness-study/sources/langgraph/` (empty) |
| Source descriptor (only artifact) | YAML file declaring the upstream repository URL `https://github.com/langchain-ai/langgraph`, the description "Best reference for durable execution, checkpoints, interrupts, state graphs", and the list of applicable dimensions, including `22.01`. | `studies/agent-harness-study/sources/langgraph.ultraplan-source.yml:1-103` |

## Answers to Dimension Questions

1. **Are modules cleanly separated?**
   No clear evidence found. No source code is present to inspect. Cannot affirm or refute whether the upstream `langchain-ai/langgraph` repository carves out separate modules/packages for runtime, tools, memory, evals, tracing, UI, or provider adapters. The local payload contains zero files, so no folder layout (e.g., a `graph/`, `channels/`, `checkpoints/`, `pregel/`, `agents/`, or `store/` split — patterns commonly associated with langgraph's public layout) can be mapped and no import-direction check can be performed.

2. **Do dependencies flow in one direction?**
   No clear evidence found. With no files, there are no edges in a module graph to check for directionality — no "checkpoints depend on runtime, not vice versa" assertion can be made. The dimension's question about clean unidirectional flow between runtime, tools, and provider adapters is unanswerable from the local payload.

3. **Can modules be used independently?**
   No clear evidence found. No manifest file (`pyproject.toml` with optional dependency groups, `Cargo.toml` feature flags, `go.mod` with build tags, etc.) is present, so the ability to import a subset of the framework without pulling in the entire runtime cannot be demonstrated. There is no `__init__.py`, `mod.rs`, or `index.ts` to test a narrow import.

4. **Are public APIs distinguished from internal ones?**
   No clear evidence found. No visibility annotations (`pub`/`pub(crate)`, `__all__`, `export`, `internal`, `: public`, `InternalsVisibleTo`, `ObservableObject`, `__declspec(dllexport)`/`__declspec(dllimport)`) could be inspected, and no public-API surface file (e.g., `docs/api.md`, `reference.md`, `api/index.html`) is present. The `langfuse/` sibling has a `CONTRIBUTING.md` and `README.md` that hint at a documentation-driven public surface, but that is out of scope here.

## Architectural Decisions

No clear evidence found. No code, configuration, design documents, ADRs, RFCs, or workspace manifests were available in the selected source directory. Architectural decisions that would normally be observable for this dimension are therefore not observable:

- Whether the framework is shipped as a single PyPI package versus a workspace of sub-packages (e.g., a hypothetical `langgraph-core`, `langgraph-checkpoint`, `langgraph-pregel`, `langgraph-runtime` split — the kind of split the dimension would want to confirm is real and not just marketing).
- Whether provider adapters (OpenAI, Anthropic, etc.) live under a shared `providers/` tree or are first-class top-level packages.
- Whether the public surface is enforced by lint rules (e.g., `ruff`/`pyright` strict-mode settings, `import-linter`, `grimp` cycle check, custom conftest that forbids deep imports of `langgraph._internal`) or only by convention.
- Whether tools, memory, evals, and tracing ship as opt-in sub-packages behind extras (e.g., `langgraph[checkpoint]`, `langgraph[postgres]`, `langgraph[sqlite]`) or as always-on dependencies.
- Whether internal APIs are colocated under an `internal/` directory, hidden behind re-exports prefixed with `_`, or stamped with an `Internal` marker.

## Notable Patterns

No clear evidence found. No patterns could be located: no single-package vs workspace decision, no `__init__.py`-barrel-files vs deep-imports choice, no `public/` + `internal/` split, no `api/` + `impl/` split, no DI container boundary defining tool vs runtime vs UI scope. There is no test or fixture demonstrating how the agent loop reaches into provider adapters without coupling to them.

## Tradeoffs

No clear evidence found. Tradeoffs cannot be enumerated when there is no implementation to weigh. In general, agent frameworks that lack clean module boundaries tend to suffer from these tradeoffs:

- Adopting a non-default checkpoint backend (e.g., Postgres vs SQLite) pulls in the entire runtime plus every other supported backend's driver.
- Internal helpers leak into public docs because the visibility boundary is implicit — common when the `langgraph/_internal/` namespace is reachable through normal imports.
- Refactors cross sub-packages because nothing prevents callers from deep-importing private modules.
- Circular-dependency errors surface late in CI rather than at lint time — particularly relevant for a graph runtime where the Pregel engine, checkpoint layer, and channel abstractions can easily form cycles.

Whether `langchain-ai/langgraph` follows any of these paths is unverifiable from the local payload.

## Failure Modes / Edge Cases

No clear evidence found. Failure modes that module boundaries are meant to prevent or expose cannot be observed:

- **Transitive bloat** — installing `langgraph` could pull every checkpoint backend's database driver; no opt-in extras visible.
- **Circular imports** — no `grimp`/`pydeps`/`import-cycle` checker config exists.
- **Public-API drift** — no `public-api` snapshot test, no `semver`-pinned sub-package, no `pyproject.toml#optional-dependencies` boundary to evaluate.
- **Provider leakage** — no evidence of a strategy for swapping LLM providers (OpenAI vs Anthropic vs Bedrock) without re-importing core runtime.
- **Build errors after partial checkout** — without a manifest, even a partial clone of the documented layout cannot be built or tested.

## Future Considerations

When the source payload is restored, the following should be re-examined for this dimension:

- Top-level layout: presence of a workspace root (e.g., `pyproject.toml` with multiple `[tool.poetry] includes`, `Cargo.toml` workspace, `pnpm-workspace.yaml`, `Microsoft.sln` with multiple `.csproj`) vs a single-project repo. For langgraph specifically, check whether `libs/langgraph/`, `libs/checkpoint/`, `libs/sdk-py/`, etc. follow the libs-first monorepo convention typical of LangChain projects.
- Whether provider adapters (OpenAI, Anthropic, Bedrock, etc.) live as separate optional packages so a consumer can install only what they need.
- Whether tools, memory, evals, and tracing are opt-in (extras, feature flags, optional deps, subpath imports) so the headline question — "Can you use the tool system without pulling in the entire runtime?" — receives an empirical yes/no.
- Direction of imports: a quick `pydeps`, `grimp --cycles`, `pyright --verifytypes`, or `ruff --select TID` pass should be run against the restored snapshot to surface cycles and reverse edges.
- Visibility tooling: presence of lint rules (`import-linter`, `dependency-cruiser`, `grimp`, `eslint-plugin-boundaries`, `pyright`/`ruff` strict-mode settings, custom pytest markers) that mechanically enforce the public/internal split.
- Public-API surface governance: presence of a snapshot test that pins the public surface and fails CI on unplanned additions.
- Whether the repo separates a `langgraph-core` from `langgraph-checkpoint-postgres` / `langgraph-checkpoint-sqlite` to make adapter swapping cheap — a particularly important question given the dimension's intent.

## Questions / Gaps

- **Why is the source directory empty?** The descriptor at `studies/agent-harness-study/sources/langgraph.ultraplan-source.yml:1-103` references `https://github.com/langchain-ai/langgraph`, but no checkout, snapshot, or mirror was present under `studies/agent-harness-study/sources/langgraph/`. The study cannot proceed against an empty target.
- **Is the upstream `langchain-ai/langgraph` repository the right version?** Without a pinned commit/tag in the descriptor or a checked-out copy locally, the study cannot anchor itself to a specific revision. Future runs should pin a commit SHA in `langgraph.ultraplan-source.yml`.
- **Was the source supposed to be vendored or fetched on demand?** The presence of sibling sources (e.g., `agent-framework/`, `pydantic-ai/`) suggests the convention is to vendor a snapshot. If `langgraph` was meant to be fetched on demand, the fetch step failed silently for this dimension. Recommend adding a fetch manifest entry to the descriptor.
- **Resolution path:** populate `studies/agent-harness-study/sources/langgraph/` with a vendored snapshot of `langchain-ai/langgraph` at a pinned SHA, then re-run this dimension study. Until then, this report's findings are "No clear evidence found" across every required section.

---

Generated by `dimensions/22.01-package-and-module-boundaries.md` against `langgraph`.

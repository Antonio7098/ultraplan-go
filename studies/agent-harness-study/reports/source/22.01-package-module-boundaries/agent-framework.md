# Source Analysis: agent-framework

## 22.01 Package and Module Boundaries

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Unknown (source payload missing) |
| Analyzed | 2026-08-22 |

## Summary

The selected source directory `studies/agent-harness-study/sources/agent-framework/` is empty on disk. No implementation files, tests, configuration, manifests, workspace metadata, or documentation were present at the time of this study. The only artifact in the sources folder for this target is the source descriptor `studies/agent-harness-study/sources/agent-framework.ultraplan-source.yml:1-119`, which declares the upstream URL as `https://github.com/microsoft/agent-framework` and lists applicable dimensions (including `22.01`) but does not include any code, project files, or boundary material.

Because the source payload is missing, there is no package or workspace manifest to read, no folder hierarchy to map, no module dependency graph to construct, no import statements or namespace declarations to inspect for direction, no visibility annotations (e.g., `__all__`, `export`, `pub`, `internal`, `: public`, `package-private`) to evaluate, and no separation/import-cycle tests to verify. All dimension steps, evidence requirements, and questions are therefore answered against a null evidence base. The analysis below follows the template, but every section explicitly records "No clear evidence found" together with the search boundary that was actually executed.

Search boundary executed for this task:

- `ls studies/agent-harness-study/sources/agent-framework/` → empty directory (`.` and `..` only).
- `find studies/agent-harness-study/sources/agent-framework -mindepth 1` → zero entries.
- `find studies/agent-harness-study/sources/agent-framework -type f | wc -l` → `0`.
- `stat studies/agent-harness-study/sources/agent-framework` → directory inode 263486, no contained files.
- Read of `studies/agent-harness-study/sources/agent-framework.ultraplan-source.yml:1-119` → metadata-only.
- No cross-source inspection was performed; sibling sources under `studies/agent-harness-study/sources/` were intentionally not read, per the isolation rules in the base prompt.

## Rating

**1 / 10** — Absent.

Rationale: The rating rubric places a score of 1-3 in the "Absent, implicit, ad-hoc, or unsafe" band. With zero source files, there is no top-level package structure to identify, no dependency direction to check, no module independence to demonstrate, no circular-dependency story to evaluate, and no internal-vs-public API distinction to verify. The dimension's headline question — "Can you use the tool system without pulling in the entire runtime?" — is unanswerable from the local payload. A score of 1 (rather than 0) is reserved because the source descriptor exists and proves the study target was intended to be Microsoft's `agent-framework`; the absence is a missing payload, not a misconfigured study, and at least one concrete artifact (the descriptor) is locatable to cite as evidence that the intent was real.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Top-level package structure | No clear evidence found. Directory `studies/agent-harness-study/sources/agent-framework/` contains no files; no folder hierarchy, `Cargo.toml`, `package.json`, `pyproject.toml`, `go.mod`, `*.sln`, `*.csproj`, `Package.swift`, or `*.podspec` could be inspected. | `studies/agent-harness-study/sources/agent-framework/` (empty) |
| Module dependency graphs | No clear evidence found. No import/`use`/`require`/`from … import`/`using` statements, no `dependencies`/`extras_require`/`optionalDependencies` blocks, no `internal` packages, no `importlib`-style subpackage markers exist locally. | `studies/agent-harness-study/sources/agent-framework/` (empty) |
| Module boundaries | No clear evidence found. No source files exist to delineate sub-packages for runtime, tools, memory, evals, tracing, UI, or provider adapters. | `studies/agent-harness-study/sources/agent-framework/` (empty) |
| API visibility annotations | No clear evidence found. No `export`/`pub`/`__all__`/`internal`/`public`/`package-private` markers, no `.h` public-header files, no `_test.go` separation, no `__init__.py` boundary files are present. | `studies/agent-harness-study/sources/agent-framework/` (empty) |
| Separation / boundary tests | No clear evidence found. No `archunit`, `dependency-cruiser`, `import-linter`, `depcheck`, `go mod why`, or `cargo tree` configuration is present; no test that asserts one module does not import from another exists. | `studies/agent-harness-study/sources/agent-framework/` (empty) |
| Source descriptor (only artifact) | YAML file declaring the upstream repository URL `https://github.com/microsoft/agent-framework` and the list of applicable dimensions, including `22.01`. | `studies/agent-harness-study/sources/agent-framework.ultraplan-source.yml:1-119` |

## Answers to Dimension Questions

1. **Are modules cleanly separated?**
   No clear evidence found. No source code is present to inspect. Cannot affirm or refute whether the upstream `microsoft/agent-framework` repository carves out separate modules/packages for runtime, tools, memory, evals, tracing, UI, or provider adapters. The local payload contains zero files, so no folder layout (e.g., a `runtime/`, `tools/`, `providers/openai/`, `providers/azure/` split) can be mapped and no import-direction check can be performed.

2. **Do dependencies flow in one direction?**
   No clear evidence found. With no files, there are no edges in a module graph to check for directionality — no "providers depend on runtime, not vice versa" assertion can be made. The dimension's question about clean unidirectional flow between runtime, tools, and provider adapters is unanswerable from the local payload.

3. **Can modules be used independently?**
   No clear evidence found. No manifest file (`package.json` with subpath `exports`, `pyproject.toml` with optional dependency groups, `Cargo.toml` feature flags, `go.mod` with build tags, `.csproj` per-feature, etc.) is present, so the ability to import a subset of the framework without pulling in the entire runtime cannot be demonstrated. There is no `__init__.py`, `mod.rs`, or `index.ts` to test a narrow import.

4. **Are public APIs distinguished from internal ones?**
   No clear evidence found. No visibility annotations (`pub`/`pub(crate)`, `__all__`, `export`, `internal`, `: public`, `InternalsVisibleTo`, `ObservableObject`, `__declspec(dllexport)`/`__declspec(dllimport)`) could be inspected, and no public-API surface file (e.g., `docs/api.md`, `reference.md`, `api/index.html`) is present. The `langfuse/` sibling does have a `CONTRIBUTING.md` and `README.md` that hint at a documentation-driven public surface, but that is out of scope here.

## Architectural Decisions

No clear evidence found. No code, configuration, design documents, ADRs, RFCs, or workspace manifests were available in the selected source directory. Architectural decisions that would normally be observable for this dimension are therefore not observable:

- Whether the framework is a single crate/package versus a Cargo/PyPI/npm workspace with sub-packages per concern.
- Whether provider adapters live under a shared `providers/` tree or are first-class top-level packages.
- Whether the public surface is enforced by lint rules (e.g., `rustc-private` boundary, `eslint-plugin-import` `no-restricted-paths`, `import-linter`) or only by convention.
- Whether tools, memory, evals, and tracing ship as opt-in sub-packages behind feature flags or as always-on dependencies.
- Whether internal APIs are colocated under an `internal/` directory, hidden behind re-exports, or stamped with an `Internal` prefix.

## Notable Patterns

No clear evidence found. No patterns could be located: no single-package vs workspace-of-crates decision, no `index.ts`-barrel-files vs deep-imports choice, no `public/` + `internal/` split, no `api/` + `impl/` split (common in C# and JVM ecosystems), no DI container boundary defining tool vs runtime vs UI scope. There is no test or fixture demonstrating how the agent loop reaches into provider adapters without coupling to them.

## Tradeoffs

No clear evidence found. Tradeoffs cannot be enumerated when there is no implementation to weigh. In general, agent frameworks that lack clean module boundaries tend to suffer from these tradeoffs:

- Adopting a non-default provider pulls in the entire runtime (large transitive surface).
- Internal helpers leak into public docs because the visibility boundary is implicit.
- Refactors cross sub-packages because nothing prevents callers from deep-importing.
- Circular-dependency errors surface late in CI rather than at lint time.

Whether `microsoft/agent-framework` follows any of these paths is unverifiable from the local payload.

## Failure Modes / Edge Cases

No clear evidence found. Failure modes that module boundaries are meant to prevent or expose cannot be observed:

- **Transitive bloat** — installing one sub-package pulls the whole runtime; no opt-in feature flag visible.
- **Circular imports** — no `import-cycle` checker config (e.g., `madge`, `dependency-cruiser`, `go mod graph`, `cargo metadata`) exists.
- **Public-API drift** — no `public-api` snapshot test, no `semver`-pinned sub-package, no `.pubignore`/`MANIFEST.in`/`package.json#exports` boundary to evaluate.
- **Provider leakage** — no evidence of a strategy for swapping `openai` for `azure-openai` for `anthropic` without re-importing core runtime.
- **Build errors after partial checkout** — without a manifest, even a partial clone of the documented layout cannot be built or tested.

## Future Considerations

When the source payload is restored, the following should be re-examined for this dimension:

- Top-level layout: presence of a workspace root (e.g., `Cargo.toml` workspace, `pnpm-workspace.yaml`, `pyproject.toml` with sub-packages, `Microsoft.sln` with multiple `.csproj`) vs a single-project repo.
- Whether provider adapters (OpenAI, Azure OpenAI, Anthropic, etc.) live as separate optional packages so a consumer can install only what they need.
- Whether tools, memory, evals, and tracing are opt-in (feature flags, optional deps, subpath imports) so the headline question — "Can you use the tool system without pulling in the entire runtime?" — receives an empirical yes/no.
- Direction of imports: a quick `madge --circular`/`pnpm why`/`cargo tree --invert`/`go list -deps` pass should be run against the restored snapshot to surface cycles and reverse edges.
- Visibility tooling: presence of lint rules (`import-linter`, `dependency-cruiser`, `archunit`, `eslint-plugin-boundaries`, `pyright`/`ruff` strict-mode settings) that mechanically enforce the public/internal split.
- Public-API surface governance: presence of `microsoft-agent-framework.api.json` (or analogous `cargo-public-api`, `apidoc`, `pyright --verifytypes`) snapshot tests that pin the public surface and fail CI on unplanned additions.
- Whether the repo separates an `agent-framework-core` from `agent-framework-azure-ai` / `agent-framework-openai` to make adapter swapping cheap.

## Questions / Gaps

- **Why is the source directory empty?** The descriptor at `studies/agent-harness-study/sources/agent-framework.ultraplan-source.yml:1-119` references `https://github.com/microsoft/agent-framework`, but no checkout, snapshot, or mirror was present under `studies/agent-harness-study/sources/agent-framework/`. The study cannot proceed against an empty target.
- **Is the upstream `microsoft/agent-framework` repository the right version?** Without a pinned commit/tag in the descriptor or a checked-out copy locally, the study cannot anchor itself to a specific revision. Future runs should pin a commit SHA in `agent-framework.ultraplan-source.yml`.
- **Was the source supposed to be vendored or fetched on demand?** The presence of sibling sources (e.g., `langgraph/`, `pydantic-ai/`) suggests the convention is to vendor a snapshot. If `agent-framework` was meant to be fetched on demand, the fetch step failed silently for this dimension. Recommend adding a fetch manifest entry to the descriptor.
- **Resolution path:** populate `studies/agent-harness-study/sources/agent-framework/` with a vendored snapshot of `microsoft/agent-framework` at a pinned SHA, then re-run this dimension study. Until then, this report's findings are "No clear evidence found" across every required section.

---

Generated by `dimensions/22.01-package-and-module-boundaries.md` against `agent-framework`.

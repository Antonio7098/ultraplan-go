# Source Analysis: opa

## Package and Module Boundaries

### Source Info

| Field | Value |
|-------|-------|
| Name | opa |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | Not determinable from local artifacts. The source metadata (`sources/opa.ultraplan-source.yml:2`) names the upstream URL `https://github.com/open-policy-agent/opa`; the on-disk checkout is empty. |
| Analyzed | 2026-08-22 |

## Summary

No analysis was possible. The selected source directory `studies/agent-harness-study/sources/opa/` is empty: it contains no files, no subdirectories, no `.git`, no `go.mod`, no `Makefile`, no manifests, no source code, no tests, no docs, no CI configuration. As a result, every dimension question (module separation, dependency direction, independent re-use, public/internal API distinction, circular dependencies) is unanswered because there is nothing to inspect. The study cannot borrow evidence from sibling sources (e.g., `agent-framework/`, `crewai/`, `langgraph/`, `letta/`, `pydantic-ai/`) because the source isolation rules forbid cross-source filesystem access. This report therefore records the search boundary, the rated score under the "absent" band of the rubric, and the concrete remediation needed to make a future study possible.

## Rating

**Score: 1 / 10 — Absent**

Rationale (per the rubric):
- The "Package structure" evidence row is empty.
- The "Module dependency graph" evidence row is empty.
- The "Module boundaries" evidence row is empty.
- The "API visibility annotations" evidence row is empty.
- The "Separation tests" evidence row is empty.

Per the 1–3 band ("Absent, implicit, ad-hoc, or unsafe"), 1 is the appropriate score: there is literally no package or module boundary to evaluate because the source content is missing. Any score above 3 would imply evidence that does not exist in the selected source directory.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Package structure | No clear evidence found. The source directory `studies/agent-harness-study/sources/opa/` contains zero files. `ls -la` against the directory returns only `.` and `..`; `find -type f` returns no rows. | `studies/agent-harness-study/sources/opa/` (empty directory) |
| Module dependency graph | No clear evidence found. No `go.mod`, `package.json`, `pyproject.toml`, `Cargo.toml`, `BUILD`, `WORKSPACE`, `MODULE.bazel`, or any equivalent manifest is present. | `studies/agent-harness-study/sources/opa/` (no manifests) |
| Module boundaries | No clear evidence found. No top-level Go packages, Python packages, or other module roots exist on disk. | `studies/agent-harness-study/sources/opa/` (no packages) |
| API visibility annotations | No clear evidence found. No Go files (so no exported vs. unexported identifiers), no Python `__all__`, no TypeScript `export` declarations, no Java `public` modifiers, no Rust `pub` items. | `studies/agent-harness-study/sources/opa/` (no source files) |
| Separation tests | No clear evidence found. No test files of any language; no `*_test.go`, `test_*.py`, `*.spec.ts`, `Cargo` integration tests. | `studies/agent-harness-study/sources/opa/` (no tests) |
| Source manifest (only artifact present) | The UltraPlan source descriptor names the upstream URL and lists applicable dimensions. | `studies/agent-harness-study/sources/opa.ultraplan-source.yml:2` |

### Search boundary (what was checked)

- `ls -la studies/agent-harness-study/sources/opa/` — confirmed empty (only `.` and `..`).
- `find studies/agent-harness-study/sources/opa/ -type f` — returned no files.
- No hidden files (`.git`, `.gitignore`, `.opahiddenconfig`, etc.) exist in the directory.
- No symlinks, no nested subdirectories.

## Answers to Dimension Questions

1. **Are modules cleanly separated?** — Unknown. No modules exist on disk to evaluate. Answer cannot be derived from the selected source.
2. **Do dependencies flow in one direction?** — Unknown. No import graph exists on disk to evaluate.
3. **Can modules be used independently?** — Unknown. Nothing is present to compose. Note: the dimension's closing question — "Can you use the tool system without pulling in the entire runtime?" — is the canonical signal here, and the only honest answer with empty source is "cannot be evaluated."
4. **Are public APIs distinguished from internal ones?** — Unknown. No source files means no visibility modifiers, no `internal/`, no `pkg/` vs. `cmd/` separation, no GoDoc, no OpenAPI/JSON-Schema.

## Architectural Decisions

No clear evidence found. The selected source contains no implementation, configuration, documentation, or design notes from which to infer any architectural decision.

## Notable Patterns

No clear evidence found.

## Tradeoffs

No clear evidence found. Typical OPA tradeoffs (e.g., single-binary distribution vs. library re-use, Rego compiler coupling to the SDK, the `internal/` package convention in Go) cannot be cited because no Go files are present. Public reputation for those tradeoffs exists in the broader ecosystem but is out of scope for this isolated study.

## Failure Modes / Edge Cases

- **Study invalidated by missing source.** The dominant failure mode is operational: the UltraPlan source preparation step did not materialize the OPA checkout into `studies/agent-harness-study/sources/opa/`. The downstream effect is that this dimension (and every other applicable dimension listed in `studies/agent-harness-study/sources/opa.ultraplan-source.yml:4-37`) will produce identical "no evidence" reports until the source is populated.
- **Cannot detect circular dependencies.** No Go module graph means no `go mod graph` output to analyze; no Python project means no `pip show` or `pydeps` data.
- **Cannot distinguish runtime vs. SDK surface.** The dimension's intent (separate runtime, tools, memory, evals, tracing, UI, provider adapters) requires source artifacts that do not exist here.

## Future Considerations

1. **Populate the source directory before re-running.** Either (a) clone the upstream repository at the pinned commit referenced by the study config, or (b) extract the subset of OPA relevant to the dimension (e.g., the `sdk/` Go package and its `go.mod` for module-boundary analysis) into `studies/agent-harness-study/sources/opa/`. After population, this dimension can be re-run and re-scored.
2. **If a partial checkout is intentional,** then the dimension should be re-scoped to match the partial artifact. For example, if only `sdk/` is checked out, the analysis can evaluate SDK-level module boundaries, but cannot speak to runtime vs. UI vs. provider-adapter separation.
3. **Add a pre-flight check.** UltraPlan source descriptors (`studies/agent-harness-study/sources/opa.ultraplan-source.yml:1-37`) should be validated at study-start to ensure the directory is non-empty; otherwise the study is wasted compute.
4. **Lock the upstream version.** Because OPA evolves quickly, pin the commit (e.g., via the descriptor's `ref:` field if added) so re-runs are reproducible.

## Questions / Gaps

- Is the empty `sources/opa/` directory intentional, or did the source materialization step fail silently?
- Which version/commit of upstream OPA is the study meant to evaluate? The descriptor at `studies/agent-harness-study/sources/opa.ultraplan-source.yml:2` lists only the URL `https://github.com/open-policy-agent/opa`, with no commit SHA, tag, or branch.
- Should the study evaluate OPA-the-policy-engine, or OPA-as-a-library (the Go `sdk/` package and embedded use cases)? These have materially different module-boundary characteristics.
- Are there excluded subpaths (e.g., `cmd/`, `internal/`, `download/`, `server/`, `rego/`) that the dimension should focus on, or exclude?

---

Generated by `22.01-package-and-module-boundaries` against `opa`.

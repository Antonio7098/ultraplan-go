# Source Analysis: temporal

## 22.01 Package and Module Boundaries

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Unknown (Go expected for `temporalio/temporal`, source not present on disk) |
| Analyzed | 2026-08-22 |

## Summary

The selected source directory `studies/agent-harness-study/sources/temporal` is empty on the local filesystem. A recursive listing with `ls -la` and `find -type f` returns no files, no hidden files, no subdirectories, and no manifest (no `go.mod`, `go.sum`, `service/`, `common/`, `components/`, or `README.md`). The source has not been materialised into the study workspace, so per the source-isolation rule ("You are studying exactly one selected source. You may ONLY access files inside that source's directory") no inspection of code, configuration, tests, or docs inside the project was possible. The accompanying `temporal.ultraplan-source.yml` (`sources/temporal.ultraplan-source.yml:1`) only declares metadata (URL, applicable dimensions) — it is not part of the source tree and cannot substitute for code evidence.

The analysis below therefore records the absence of evidence rather than fabricating findings. The search boundary was the directory itself: every file and path cited as missing is at the root of `studies/agent-harness-study/sources/temporal/`.

## Rating

**1 / 10** — Absent.

Rationale: Package and module boundaries cannot be evaluated when no package, no modules, no `go.mod`, and no source files exist in the inspected directory. The rubric anchor for 1–3 ("Absent, implicit, ad-hoc, or unsafe") applies because there is literally no code to evaluate. The rating is not a judgment on the `temporalio/temporal` project itself; it is a judgment on the evidence available under the source-isolation constraint.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Top-level package structure | No `go.mod`, `go.sum`, `Gopkg.toml`, `vendor/`, `service/`, `common/`, or `components/` directory present. Searched: `studies/agent-harness-study/sources/temporal/*` — zero matches. | `studies/agent-harness-study/sources/temporal/:1` (directory empty) |
| Module dependency graph | No `*.go` files, no import surface to inspect. | `studies/agent-harness-study/sources/temporal/:1` (directory empty) |
| Module boundaries | No Go internal-package convention markers (`internal/` directory, package-level doc, `pkg/` vs `internal/` split) could be located. | `studies/agent-harness-study/sources/temporal/:1` (directory empty) |
| API visibility annotations | No Go visibility tokens (`pkg/`, `internal/`, no exported-vs-unexported symbol analysis possible). | `studies/agent-harness-study/sources/temporal/:1` (directory empty) |
| Separation tests | No `*_test.go`, no test configuration present. | `studies/agent-harness-study/sources/temporal/:1` (directory empty) |
| Build / packaging config | No `go.mod`, `go.sum`, `Makefile`, `Dockerfile`, or `goreleaser.yml` present. | `studies/agent-harness-study/sources/temporal/:1` (directory empty) |
| Source manifest pointer | URL `https://github.com/temporalio/temporal` declared but not fetched into the source directory. | `sources/temporal.ultraplan-source.yml:2` |
| Dimension scope | This dimension (22.01) is listed as applicable (line 57 of the descriptor), confirming the study intent. | `sources/temporal.ultraplan-source.yml:57` |
| Project descriptor framing | Descriptor labels temporal as "Gold standard for workflow durability and replay" — stated design goal, not implementation evidence. | `sources/temporal.ultraplan-source.yml:3` |

## Answers to Dimension Questions

1. **Are modules cleanly separated?** — No clear evidence found. Search boundary: the entire `studies/agent-harness-study/sources/temporal/` directory, which contains zero files. Without a `go.mod` and module tree, no claim about the project's well-known multi-module layout (`service/`, `common/`, `components/`, `client/`) can be cited as evidence under the isolation rule.
2. **Do dependencies flow in one direction?** — No clear evidence found. No import statements, `go.mod` dependency block, or module graph artifacts are available to inspect.
3. **Can modules be used independently?** — No clear evidence found. No package metadata (`go.mod`), no `internal/` package convention enforcement, and no optional build tags exist to demonstrate optional-component compilation or sub-module distribution.
4. **Are public APIs distinguished from internal ones?** — No clear evidence found. There is no `pkg/` vs `internal/` tree, no documented exported-symbol surface, and no Go convention enforcement (e.g., `internal/...` package path rule) available to inspect.

## Architectural Decisions

No clear evidence found. The directory contains no files from which architectural decisions about package or module boundaries could be derived. The descriptor's "Gold standard for workflow durability and replay" framing (`sources/temporal.ultraplan-source.yml:3`) is a stated design goal, not implementation evidence and cannot be cited as an architectural decision under rule #2 (every code mention must include a file path inside the selected source).

## Notable Patterns

No clear evidence found.

## Tradeoffs

No clear evidence found. Without a manifest or module tree, no tradeoffs (e.g., single-module vs. multi-module Go workspace, `pkg/` vs `internal/` discipline, runtime vs. client SDK sub-packages) can be evaluated. The descriptor's positioning as a reference baseline (`sources/temporal.ultraplan-source.yml:3`) is editorial and cannot serve as evidence.

## Failure Modes / Edge Cases

- **Source not materialised.** The study workflow assumes the source has been cloned/copied into `studies/agent-harness-study/sources/temporal/`. In this run it has not. Any downstream dimension that depends on this source will hit the same gap and must either (a) request materialisation of the source or (b) record "no clear evidence found".
- **Cross-source isolation blocks workaround.** Hard rule #1 forbids reading sibling sources (e.g., `langfuse/`, `openhands/`) to compensate, so the analysis must terminate at the empty-directory boundary.
- **Descriptor-stated reputation is not evidence.** Temporal is widely described as a reference for workflow durability and replay boundaries; without on-disk code, this reputation cannot be cited and the analysis must score from absence, not from external knowledge.

## Future Considerations

- Materialise `temporal` into `studies/agent-harness-study/sources/temporal/` (e.g., `git clone https://github.com/temporalio/temporal`) before running any dimension that requires code inspection.
- Once materialised, re-run this dimension to evaluate the actual Go module split. The temporal monorepo is publicly known to include components such as `service/history`, `service/matching`, `service/worker`, `service/frontend`, `common/`, `components/` (e.g., `components/callbacks`, `components/scheduler`), `client/`, and an `internal/` package convention — but none of these can be cited as evidence until the source tree is on disk.
- Consider a study-level pre-flight check that fails fast when a source directory is empty, instead of producing N "no evidence" reports.
- Consider promoting temporal from "reference descriptor" (empty dir) to "fully populated source" given how many dimensions list it as applicable (60+ entries in `sources/temporal.ultraplan-source.yml:4-64`) — its absence disproportionately degrades the study.

## Questions / Gaps

- Why is the `temporal` source directory empty while sibling `langfuse/` and `openhands/` are populated? Is there a fetch step missing from the study bootstrap, or a per-source allowlist that excludes it?
- Is there an out-of-band mechanism (git submodule, archive download, monorepo path) the study expects the analyst to use? If so, it must be documented in the prompt because rule #1 forbids reaching outside the source directory.
- Should future prompts allow a "source unavailable — abort" exit code instead of forcing a low-score, no-evidence report?
- The descriptor applies 60+ dimensions to temporal (`sources/temporal.ultraplan-source.yml:4-64`). If the empty state is permanent, those dimensions will all yield identical low-quality reports. Is that the intended behaviour?

---

Generated by `dimensions/22.01-package-module-boundaries.md` against `temporal`.
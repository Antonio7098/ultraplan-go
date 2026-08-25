# Source Analysis: opa

## Public API Surface

### Source Info

| Field | Value |
|-------|-------|
| Name | opa |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | unknown (source not materialised) |
| Analyzed | 2026-08-22 |

## Summary

The selected source directory is empty: `studies/agent-harness-study/sources/opa/` contains no files. A recursive enumeration (`ls -laR`, glob `**/*`, `find -type f`) and the source manifest show zero Go modules, no `go.mod`, no `cmd/`, no `pkg/`, no `internal/`, no `docs/`, no `Makefile`, and no exported symbols. Because the dimension definition requires evidence drawn from import paths, client objects, CLI commands, service endpoints, and documented entry points, and the rules prohibit inspecting sibling sources, this study cannot inspect any concrete public API surface for `opa`. The score reflects the absence of inspectable evidence rather than a judgment about the upstream `open-policy-agent/opa` project itself.

Search boundary executed:

- Recursive listing of `studies/agent-harness-study/sources/opa/` — no files (`studies/agent-harness-study/sources/opa/.`).
- `find studies/agent-harness-study/sources/opa/ -type f` — zero matches.
- Glob `**/*` against the selected source path — zero matches.
- Source isolation rule (per task prompt) forbids reading other source directories, the dimension inputs/manifest aside, so no substitute evidence is admissible in this study.

## Rating

**Rating: 1 / 10** — Tier: Absent (no inspectable evidence)

**Score:** 1
**Score (out of 10):** 1/10
**Tier:** Absent (rubric band 1-3)

Rationale: the rubric maps scores `1-3` to "Absent, implicit, ad-hoc, or unsafe" public APIs. With zero files in the selected source path there are no stable import paths, no clients, no CLI commands, no service endpoints, no type definitions, no tests, and no documented examples to evaluate. None of the four dimension questions can be answered with code-cited evidence, which itself is the strongest indicator that a public API surface study is not feasible against this materialised source.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Package manifest (`go.mod`) | No evidence found — no manifest present in the selected source directory. | `studies/agent-harness-study/sources/opa/.` (directory empty) |
| Top-level `cmd/` entry points (e.g. `opa run`, `opa eval`, `opa test`, `opa fmt`, `opa build`) | No evidence found — no Go entry points or script wrappers. | `studies/agent-harness-study/sources/opa/.` (directory empty) |
| Public packages (`pkg/`, top-level re-export barrels) | No evidence found. | `studies/agent-harness-study/sources/opa/.` (directory empty) |
| Internal packages (`internal/`) and any boundary markers | No evidence found. | `studies/agent-harness-study/sources/opa/.` (directory empty) |
| Client objects / SDK surfaces (e.g. `github.com/open-policy-agent/opa/sdk`) | No evidence found — no Go modules file present. | `studies/agent-harness-study/sources/opa/.` (directory empty) |
| HTTP/RPC service routes (REST API on `:8181`, Admin API, decision logs, bundle distribution endpoints) | No evidence found — no route definitions. | `studies/agent-harness-study/sources/opa/.` (directory empty) |
| Public type definitions / interfaces (`ast.Module`, `rego.Compile`, `sdk.OPA`, `topdown.Query`, `bundle.Bundle`) | No evidence found. | `studies/agent-harness-study/sources/opa/.` (directory empty) |
| Rego language / policy engine entry points (`rego.v1`, `rego.v0`, `data`, `input`) | No evidence found. | `studies/agent-harness-study/sources/opa/.` (directory empty) |
| Documentation pages / runnable examples (`README.md`, `docs/`, `examples/`) | No evidence found. | `studies/agent-harness-study/sources/opa/.` (directory empty) |
| Test suites for public API contract (`*_test.go`, `v1/.../export_test.go`) | No evidence found. | `studies/agent-harness-study/sources/opa/.` (directory empty) |
| API stability markers (semver tags, deprecation notices, frozen / experimental labels in `OPA_VERSION`) | No evidence found. | `studies/agent-harness-study/sources/opa/.` (directory empty) |
| Internal/experimental labels (`internal/`, `experimental/` packages, build tags) | No evidence found. | `studies/agent-harness-study/sources/opa/.` (directory empty) |
| Import/export boundaries (re-export barrels, package aliases, plugin registration surfaces) | No evidence found. | `studies/agent-harness-study/sources/opa/.` (directory empty) |
| Evidence of accidental public surface area (loose star exports, wildcard re-exports, unprotected internal types) | No evidence found. | `studies/agent-harness-study/sources/opa/.` (directory empty) |

## Answers to Dimension Questions

1. **What is the intended public API surface?** — No clear evidence found. The intended surface cannot be enumerated from the empty source; per the source manifest at `sources/opa.ultraplan-source.yml:1-3`, the upstream project is the `open-policy-agent/opa` repository, but the materialised snapshot at `studies/agent-harness-study/sources/opa/` contains nothing to read.
2. **Is the stable API easy to distinguish from internal implementation details?** — No clear evidence found. There are no `pkg/` vs `internal/` partitions to inspect, no deprecation comments, no stability markers, and no GoDoc annotations visible.
3. **Does the API expose the right level of abstraction for agent harness users?** — No clear evidence found. No SDK surface, REST routes, or Rego entry points are introspectable in the selected directory.
4. **Are examples sufficient to use the API correctly without reading internals?** — No clear evidence found. No `examples/`, `docs/`, or README exists inside the selected source directory.

## Architectural Decisions

No clear evidence found. The selected source contains no implementation files, configuration, or documentation; therefore no architectural decisions about import-path grouping, namespace separation (e.g. `internal/` vs `pkg/`), CLI command ownership, SDK lifecycle hooks, or HTTP route partitioning can be cited.

## Notable Patterns

No clear evidence found. A pattern search (e.g. Rego compile pipeline entry points, plugin registration, SDK client construction, topdown query API, bundle loader API) returned no candidates because the directory contains no files.

## Tradeoffs

No clear evidence found. Tradeoffs only become nameable once stable import paths, clients, and endpoints exist; here the absence of any surface precludes that analysis.

## Failure Modes / Edge Cases

No clear evidence found. Failure modes of accidental exports, shadowed names, or unguarded internal access require at least one public package to study. None is present.

## Future Considerations

- The materialised source snapshot at `studies/agent-harness-study/sources/opa/` needs to be populated (e.g. via a fetch of the upstream `open-policy-agent/opa` repository, see `sources/opa.ultraplan-source.yml:1`) before any dimension anchored on code can produce evidence-grade findings.
- Once materialised, a re-run of this dimension should specifically surface:
  - The contents of `go.mod` and the module path (`github.com/open-policy-agent/opa`) and which subpackages it marks as public (`pkg/`, `sdk/`, `rego`, `topdown`, `bundle`, `ast`, `v1`).
  - The `cmd/` tree exposing the CLI command surface (`run`, `eval`, `test`, `fmt`, `build`, `bench`, `exec`, `lint`, `sign`, `verify`, `parse`, `deps`, `check`, `eval`, `eval`).
  - The Go SDK surface at `sdk/` and the `sdk.OPA` client lifecycle (`New`, `Decision`, `Update`, `Configure`, `Watch`) used by agent harness integrations.
  - The HTTP API surface hosted by `server/` (REST on `:8181/v1/...`, Admin API, decision-log streaming, bundle distribution).
  - Stability markers such as `OPA_VERSION`, deprecation notices in GoDoc, `// Deprecated:` comments, and version-pinned plugin registries (`plugins/bundle`, `plugins/discovery`, `plugins/status`).
  - Runnable examples under `examples/`, the `docs/` site tree, and the embedded playground that demonstrate the stable surface without requiring internals.
  - Boundary files such as `v1/ast/ast.go`, `v1/topdown/topdown.go`, `v1/rego/rego.go`, and the v1 import-blocker policy that distinguishes stable surface from in-progress API.

## Questions / Gaps

- Why is the `opa` source directory empty while the manifest at `sources/opa.ultraplan-source.yml:2-3` advertises it as a "Best-in-class policy engine for authorization" with 31 applicable dimensions? This is the single most important question for the study, because the gap determines whether the dimension is reported as "no evidence" or rewritten once the snapshot is populated.
- Without violating source isolation, there is no admissible way to infer what the upstream OPA public API looks like; downstream re-runs of this dimension must rely on the materialised snapshot rather than on out-of-scope cross-source reads.

---

Generated by `24.01-public-api-surface` against `opa`.
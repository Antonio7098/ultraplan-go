# Source Analysis: temporal

## Public API Surface

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Unknown — source not present locally |
| Analyzed | 2026-08-22 |

## Summary

The selected source directory `studies/agent-harness-study/sources/temporal` is empty. A directory listing via `ls -la` and a recursive `find` for files both return no entries (verified 2026-08-22). The companion manifest `sources/temporal.ultraplan-source.yml:1-64` declares the source name `temporal`, the upstream URL `https://github.com/temporalio/temporal`, and lists Dimension `24.01` as applicable, but no source tree, no cloned checkout, and no vendored snippet is present under the source path. The applicable-dimensions list in the manifest is `studies/agent-harness-study/sources/temporal.ultraplan-source.yml:61`.

Cross-source filesystem access is forbidden by the study's hard rules, so this analysis is restricted to the selected source directory only. Because no files exist, no evidence of any kind (public packages, clients, CLI commands, HTTP/RPC routes, documentation, examples, or tests) can be cited.

## Rating

**1 / 10 — Absent.**

Rationale: For Public API Surface, a rating of 1–3 means the surface is "Absent, implicit, ad-hoc, or unsafe." With no source tree present, no part of the API surface can be observed — not stable import paths, not client objects, not CLI commands, not service endpoints, not public/internal markers, not example coverage. The score is 1 rather than 0 only because the manifest exists and confirms intent to analyze `temporal`.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Selected source directory contents | Empty directory; `ls -la` and recursive `find` return zero entries | `studies/agent-harness-study/sources/temporal/`: (no entries) |
| Source manifest declaring target | Manifest exists and lists this source, URL, description, and applicable dimensions; provides metadata only, no code surface | `studies/agent-harness-study/sources/temporal.ultraplan-source.yml:1-64` |
| Upstream URL declared by manifest | `https://github.com/temporalio/temporal` is referenced as the upstream source | `studies/agent-harness-study/sources/temporal.ultraplan-source.yml:2` |
| Description declared by manifest | "Gold standard for workflow durability and replay" — design intent, not implementation evidence | `studies/agent-harness-study/sources/temporal.ultraplan-source.yml:3` |
| Dimension 24.01 applicability | This dimension is listed as applicable for the `temporal` source | `studies/agent-harness-study/sources/temporal.ultraplan-source.yml:61` |

No evidence found for: stable import paths, client objects, CLI command groups, HTTP/RPC routes, documented entry points, public/internal separation labels, example coverage, generated-code markers, accidental exports, or any implementation symbol. Search boundary: the entire `studies/agent-harness-study/sources/temporal/` directory tree.

## Answers to Dimension Questions

1. **What is the intended public API surface?** — **No evidence found.** The selected source directory contains no files. The only artifact is the manifest at `studies/agent-harness-study/sources/temporal.ultraplan-source.yml:1-64`, which names the upstream repository (`https://github.com/temporalio/temporal`) and a thematic tagline (`studies/agent-harness-study/sources/temporal.ultraplan-source.yml:2-3`) but provides no symbol, import path, client object, command, or endpoint. Per the source-isolation rule, the upstream Temporal repository on GitHub was not inspected and is not cited here.
2. **Is the stable API easy to distinguish from internal implementation details?** — **No evidence found.** No files exist to expose an internal/external boundary, deprecation markers, `internal/` directories, `public/` packages, or stability labels.
3. **Does the API expose the right level of abstraction for agent harness users?** — **No evidence found.** No abstraction layer, client SDK, or operator surface is observable in the selected source directory.
4. **Are examples sufficient to use the API correctly without reading internals?** — **No evidence found.** No example, README, doc, or runnable sample is present under `studies/agent-harness-study/sources/temporal/`.

## Architectural Decisions

No clear evidence found.

The selected source directory is empty (`studies/agent-harness-study/sources/temporal/` contains no files). No architectural decisions, package boundaries, or import graphs are observable in the selected scope. Search boundary: every path under `studies/agent-harness-study/sources/temporal/`.

## Notable Patterns

No clear evidence found.

No source files exist under `studies/agent-harness-study/sources/temporal/`, so no patterns (versioned client objects, command-group registries, stable surface directories, `index.ts` re-exports, generated-code fences, etc.) can be observed. Search boundary: the full selected source tree.

## Tradeoffs

No clear evidence found.

Tradeoffs cannot be observed because the selected source tree contains no implementation. The manifest's declared description "Gold standard for workflow durability and replay" (`studies/agent-harness-study/sources/temporal.ultraplan-source.yml:3`) is a stated design goal, not an observed tradeoff. Per the evidence rules, manifest text alone does not constitute evidence of an implemented API surface.

## Failure Modes / Edge Cases

No clear evidence found.

Without source files under `studies/agent-harness-study/sources/temporal/`, no failure modes (breaking-change handling, deprecation paths, accidental exports, internal leaks, mis-versioned clients) are observable. Search boundary: the entire selected source directory.

The only observable, source-isolation–compliant issue is a **source-availability failure mode for the study pipeline itself**: the selected source directory is empty, which means every dimension in `temporal.ultraplan-source.yml:4-64` will return "no evidence" until the upstream tree (`https://github.com/temporalio/temporal`, per `studies/agent-harness-study/sources/temporal.ultraplan-source.yml:2`) is fetched into `studies/agent-harness-study/sources/temporal/` using a method that respects any license and attribution requirements.

## Future Considerations

No clear evidence found.

No API surface exists in the selected source directory to derive future considerations from. Any speculation about future direction would exceed the citation rules.

Concrete remediation that would unblock this dimension (and every other applicable dimension in `studies/agent-harness-study/sources/temporal.ultraplan-source.yml:4-64`):

- Fetch the upstream repository declared at `studies/agent-harness-study/sources/temporal.ultraplan-source.yml:2` into `studies/agent-harness-study/sources/temporal/`, then re-run Dimension `24.01` against the populated tree.
- If a vendored subset is intended instead of a full clone, document the subset boundary in `studies/agent-harness-study/sources/temporal.ultraplan-source.yml` so the source-isolation boundary is unambiguous for every dimension.

## Questions / Gaps

- **Q1 — Source fetch status.** Why is `studies/agent-harness-study/sources/temporal/` empty when the sibling manifests exist (e.g., `sources/agent-framework.ultraplan-source.yml`, `sources/langfuse.ultraplan-source.yml`) and other sources have populated trees? Was the Temporal source intentionally excluded, or did a fetch step fail silently?
- **Q2 — Scope of "source."** Per `studies/agent-harness-study/sources/temporal.ultraplan-source.yml:2`, the upstream is `https://github.com/temporalio/temporal` (the Go-based Temporal server). Should this dimension's "public API" instead cover an SDK (e.g., the Go SDK or Python SDK) and, if so, does the manifest need updating?
- **Q3 — Conflicting directory.** A second empty directory `studies/ultraplan-daemon-events-study/sources/temporal/` exists in the workspace. Is the agent-harness-study analysis meant to consult that tree? Under the source-isolation rule, no — but flagging it for the orchestrator.
- **Q4 — Evidence floor.** Without any files in the selected source directory, the dimension's required evidence (stable import paths, client objects, CLI commands, service endpoints, documented entry points, internal/external markers, accidental exports) cannot be collected. Re-running this dimension after the source is populated is required for a rating above 1.

---

Generated by `24.01-public-api-surface` against `temporal`.

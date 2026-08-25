# Source Analysis: agent-framework

## Public API Surface

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Unknown — source directory is empty; manifest references `https://github.com/microsoft/agent-framework` (Microsoft's strategic enterprise agent framework, expected to be multi-language .NET + Python) |
| Analyzed | 2026-08-22 |

## Summary

The selected source directory `studies/agent-harness-study/sources/agent-framework` contains no files. Searched the directory recursively for files, subdirectories, hidden files, symlinks, and any contents — only the directory itself exists. The sibling manifest `studies/agent-harness-study/sources/agent-framework.ultraplan-source.yml` exists at line 1-119 and references `https://github.com/microsoft/agent-framework`, but the manifest is metadata describing this study's plan, not part of the source itself and therefore off-limits for API-surface evidence under the isolation rule. No source code, configuration, package manifests, public API definitions, examples, or documentation files are present to inspect. Consequently, no claims about the public API surface of agent-framework can be substantiated from local evidence.

Search boundary: `find studies/agent-harness-study/sources/agent-framework -type f` returned zero results; `find … -type d` returned only the source root itself; `ls -la` confirms a single empty directory entry (`.` and `..` only, no `README`, no `pyproject.toml`, no `.csproj`, no `.sln`, no `package.json`, no `Cargo.toml`, no `go.mod`, no `Gemfile`, no source tree, no `docs/`, no `examples/`, no `LICENSE`). No `__init__.py`, no `src/`, no `dotnet/`, no `python/`, no `samples/` directory exists.

## Rating

**Score: 1 / 10 — Absent.**

Rationale (per the dimension rubric): the public API surface is absent from the inspection boundary because the source material itself is absent. A score of 1 is warranted under the rubric band "Absent, implicit, ad-hoc, or unsafe." Without any local artifacts to inspect, the dimension cannot be evaluated for naming consistency, lifecycle ownership, abstraction boundaries, documentation, or discoverability. A higher score is not defensible: there is no public API to grade, only an empty source directory.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Source presence | `find studies/agent-harness-study/sources/agent-framework -type f` returned zero results; directory listing contains only `.` and `..` | `studies/agent-harness-study/sources/agent-framework/:1` (directory entry) |
| Manifest reference (metadata only, not source) | The source manifest names the upstream URL `https://github.com/microsoft/agent-framework` and lists applicable dimensions; this file is the study's planning metadata, not source code | `sources/agent-framework.ultraplan-source.yml:2` |
| Stable import paths | No clear evidence found — no `__init__.py`, `pyproject.toml`, `Cargo.toml`, `package.json`, `.csproj`, or `.sln` exists to define import boundaries | n/a (no file present) |
| Public packages, modules, clients, command groups, HTTP/RPC routes | No clear evidence found — no source tree, no entry points, no client objects, no CLI definitions exist in the selected source directory | n/a (no file present) |
| Documentation and example coverage | No clear evidence found — no `README`, no `docs/`, no `examples/`, no `samples/` directory exists in the selected source directory | n/a (no file present) |
| API stability markers or internal/experimental labels | No clear evidence found — no API definitions, decorators, attribute markers, or annotation files exist | n/a (no file present) |
| Import/export boundaries | No clear evidence found — no language-specific module or package manifests exist | n/a (no file present) |
| Evidence of accidental public surface area | No clear evidence found — no exports, re-exports, or symbol lists exist to assess accidental exposure | n/a (no file present) |

## Answers to Dimension Questions

1. **What is the intended public API surface?**
   No clear evidence found. The selected source directory is empty; there are no stable import paths, client objects, CLI commands, service endpoints, or documented entry points present locally to identify the intended public API surface.

2. **Is the stable API easy to distinguish from internal implementation details?**
   No clear evidence found. With no source files present, no separation between stable public API and internals can be observed (e.g., no `__all__` lists in Python, no `public/` versus `internal/` directory layout, no `InternalsVisibleTo` or `[EditorBrowsable(Never)]` attributes in .NET, no `pub` versus private module distinction in Rust, no `internal` versus exported symbol distinction in TypeScript/Go).

3. **Does the API expose the right level of abstraction for agent harness users?**
   No clear evidence found. No abstraction layer, base classes, agent builders, tool registries, or runtime entry points exist locally to evaluate abstraction choices for harness authors.

4. **Are examples sufficient to use the API correctly without reading internals?**
   No clear evidence found. No example files, tutorials, snippets, or `examples/` directory are present in the selected source. Whether the upstream repository ships examples cannot be verified from this study.

## Architectural Decisions

No clear evidence found. No source files, configuration, manifests, or documentation are present in the selected source directory to identify architectural decisions about API grouping, naming, lifecycle ownership, version policy, or abstraction layering.

## Notable Patterns

No clear evidence found. No patterns (factory, builder, fluent-API, module facade, capability provider, etc.) can be observed because no source code is present.

## Tradeoffs

No clear evidence found. Without source material, no tradeoff discussion (e.g., breadth vs. stability, ergonomics vs. flexibility, public surface area vs. maintenance burden) is grounded in evidence.

## Failure Modes / Edge Cases

No clear evidence found. No API definitions, validation logic, error envelopes, or deprecation markers exist locally to study failure modes. The only observable failure mode is at the study-input layer: an empty source directory prevents evidence-based analysis of the dimension at all.

## Future Considerations

If the source directory is populated (e.g., via `git clone https://github.com/microsoft/agent-framework` into `studies/agent-harness-study/sources/agent-framework/`), the analysis should be re-run. Specifically, re-inspect:

- Multi-package layout: Microsoft agent-framework is expected to ship both .NET and Python distributions; verify whether the public API is duplicated across languages or split per language
- Whether `agent-framework` exposes a stable `Agent`, `AgentThread`, `Tool`, and `ChatClientProtocol` entry point surface versus implementation-internal types
- Whether Python `agent-framework` mirrors `agent-framework-core` and `agent-framework-azure-ai` packages with explicit re-exports
- Whether .NET ships `Microsoft.Agents.AI`, `Microsoft.Agents.AI.Hosting`, and `Microsoft.Agents.AI.OpenAI` as documented NuGet packages
- Documentation index under `docs/` or `python/packages/core/agent_framework/`
- Sample coverage under `samples/` or `python/samples/`

## Questions / Gaps

- Was the upstream repository `https://github.com/microsoft/agent-framework` expected to be cloned into `studies/agent-harness-study/sources/agent-framework/` before dimension tasks were dispatched? The selected source directory is empty, while sibling sources (`langfuse`, `openhands`) were cloned with commits visible in `git status`.
- Should the harness study runner pre-clone source repositories before scheduling dimension tasks, or is the empty directory an intentional placeholder to be filled by a later step?
- Is the upstream repository even publicly accessible at the URL recorded in `sources/agent-framework.ultraplan-source.yml:2`? No remote fetch was performed under the isolation rule.
- Without local source, every dimension question against `agent-framework` is unanswerable. The orchestration layer should treat empty source directories as a hard pre-condition failure rather than dispatching dimension tasks.

---

Generated by `24.01-public-api-surface` against `agent-framework`.

# Source Analysis: agent-framework

## Interface Contract Design

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Unknown — source directory is empty; manifest references `https://github.com/microsoft/agent-framework` (Microsoft's strategic enterprise agent framework, expected to be multi-language .NET + Python) |
| Analyzed | 2026-08-22 |

## Summary

The selected source directory `studies/agent-harness-study/sources/agent-framework` contains no files. Searched the directory recursively for files, subdirectories, hidden files, symlinks, and any contents — only the directory itself exists. The sibling manifest `studies/agent-harness-study/sources/agent-framework.ultraplan-source.yml:1-119` exists and references `https://github.com/microsoft/agent-framework`, but the manifest is metadata describing this study's plan, not part of the source itself and therefore off-limits for interface-contract evidence under the isolation rule. No source code, configuration, package manifests, public API definitions, examples, or documentation files are present to inspect. Consequently, no claims about the interface contract design of agent-framework can be substantiated from local evidence.

Search boundary: `find studies/agent-harness-study/sources/agent-framework -type f` returned zero results; `find … -type d` returned only the source root itself; `ls -la` confirms a single empty directory entry (`.` and `..` only, no `README`, no `pyproject.toml`, no `.csproj`, no `.sln`, no `package.json`, no `Cargo.toml`, no `go.mod`, no `Gemfile`, no source tree, no `docs/`, no `examples/`, no `LICENSE`). No `__init__.py`, no `src/`, no `dotnet/`, no `python/`, no `samples/` directory exists. The dimension's central objects of study — interfaces, protocols, abstract base classes, trait objects, schemas, and service contracts — are all absent from the inspection boundary.

## Rating

**Score: 1 / 10 — Absent.**

Rationale (per the dimension rubric): interface contracts are absent from the inspection boundary because the source material itself is absent. A score of 1 is warranted under the rubric band "Absent, implicit, ad-hoc, or unsafe." Without any local artifacts to inspect, the dimension cannot be evaluated for interface size, dependency direction, error contracts, context propagation, cancellation semantics, lifecycle methods, substitutability, or compile-time/schema-time/runtime contract validation. A higher score is not defensible: there are no contracts to grade, only an empty source directory.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Source presence | `find studies/agent-harness-study/sources/agent-framework -type f` returned zero results; directory listing contains only `.` and `..` | `studies/agent-harness-study/sources/agent-framework/:1` (directory entry) |
| Manifest reference (metadata only, not source) | The source manifest names the upstream URL `https://github.com/microsoft/agent-framework` and lists applicable dimensions; this file is the study's planning metadata, not source code | `sources/agent-framework.ultraplan-source.yml:2` |
| Interface / protocol / abstract base class definitions | No clear evidence found — no `.cs`, `.py`, `.ts`, `.rs`, `.go`, `.rb`, or `.java` files exist; no `interface`, `protocol`, `ABC`, `trait`, or `Schema` symbols are present | n/a (no file present) |
| Adapter implementations | No clear evidence found — no adapter files, no provider implementations, no plugin entry points exist | n/a (no file present) |
| Contract tests / conformance suites | No clear evidence found — no `tests/`, `__tests__`, conformance fixtures, or golden files exist | n/a (no file present) |
| Error, cancellation, streaming, and lifecycle semantics | No clear evidence found — no exception classes, error enums, cancellation tokens, or lifecycle hooks exist | n/a (no file present) |
| Validation logic (compile-time, schema-time, runtime) | No clear evidence found — no type validators, JSON-Schema definitions, pydantic models, or schema registries exist | n/a (no file present) |
| Documentation tied to contract design | No clear evidence found — no `README`, no `docs/`, no `examples/`, no ADRs exist in the selected source directory | n/a (no file present) |
| Consumer-side ownership markers | No clear evidence found — no `__all__` exports, no `InternalsVisibleTo`, no `[EditorBrowsable(Never)]`, no `pub(crate)` boundaries exist | n/a (no file present) |

## Answers to Dimension Questions

1. **Are interfaces small, coherent, and owned by the consumer side?**
   No clear evidence found. The selected source directory is empty; there are no interfaces, protocols, abstract base classes, trait objects, or schemas present locally to evaluate size, coherence, or ownership direction. Whether agent-framework places interface ownership on the consumer (e.g., a Python `Protocol` declared where it is consumed, or a .NET interface declared in the assembly that consumes it) cannot be verified from this study.

2. **Do contracts specify behavior, not just method signatures?**
   No clear evidence found. With no source files present, no behavioral contracts, docstrings, invariants, pre/post-conditions, type-level guarantees, or `.editorconfig`-style enforcement rules can be observed. Whether agent-framework encodes semantic guarantees (e.g., "an `AgentRunResponse` is immutable after emission", "a `ChatClient` call is retryable on HTTP 429", "a tool invocation always returns within `timeout_ms`") cannot be assessed.

3. **Can providers, tools, stores, and runtimes be replaced safely?**
   No clear evidence found. No substitutability evidence — no provider registries, dependency-inversion boundaries, adapter indirection layers, swappable backends, or feature flags — exists locally. Whether two independent implementations (e.g., an Azure AI provider versus an OpenAI provider) can satisfy the same contract without relying on undocumented behavior is unverifiable from this study.

4. **Are compatibility failures caught early by tests or validation?**
   No clear evidence found. No test files, conformance suites, contract tests, golden file comparisons, schema validation harnesses, or CI configuration exist in the selected source to demonstrate that compatibility failures are caught early. Whether the upstream repository ships a conformance suite is unknown from local evidence.

## Architectural Decisions

No clear evidence found. No source files, configuration, manifests, or documentation are present in the selected source directory to identify architectural decisions about interface segregation, dependency inversion, error envelope shape, cancellation propagation, lifecycle ownership, or schema versioning.

## Notable Patterns

No clear evidence found. No patterns (consumer-defined interfaces, port-and-adapter, hexagonal architecture, capability providers, schema-first design, etc.) can be observed because no source code is present.

## Tradeoffs

No clear evidence found. Without source material, no tradeoff discussion (e.g., narrow contracts versus developer ergonomics, structural typing versus nominal typing, schema-strictness versus flexibility, runtime versus compile-time validation) is grounded in evidence.

## Failure Modes / Edge Cases

No clear evidence found. No interface definitions, validation logic, error envelopes, or deprecation markers exist locally to study failure modes. The only observable failure mode is at the study-input layer: an empty source directory prevents evidence-based analysis of the dimension at all.

## Future Considerations

If the source directory is populated (e.g., via `git clone https://github.com/microsoft/agent-framework` into `studies/agent-harness-study/sources/agent-framework/`), the analysis should be re-run. Specifically, re-inspect:

- The .NET surface (`Microsoft.Agents.AI.*`) for explicit interface declarations versus public class leakage; check whether `IChatClient`, `IAgentThread`, `ITool`, `IHostedAgent`, or analogous types are declared as interfaces or as concrete classes that cannot be substituted without inheritance.
- The Python surface (`agent_framework`, `agent_framework_core`, `agent_framework_azure_ai`) for `Protocol` and `ABC` usage, `runtime_checkable` markers, and `__all__` lists that define the stable contract boundary.
- Whether `py.typed` is shipped so static type checkers can enforce the contract at compile time.
- Whether a conformance suite (e.g., `tests/conformance/`) runs every provider adapter against the same scenario matrix.
- Whether the schema for tool/function definitions, message envelopes, or thread state is checked at schema-time (e.g., JSON Schema, pydantic models, OpenAPI spec) before runtime invocation.
- Whether cancellation flows uniformly across the contract (e.g., `CancellationToken` in .NET, `asyncio.CancelledError` in Python) and whether partial-failure semantics are part of the contract.
- Whether the framework ships explicit error enums or exception hierarchies versus relying on string messages.
- Whether the `samples/` or `python/samples/` directories show consumers declaring their own narrow interfaces for test doubles, indicating consumer-side ownership.

## Questions / Gaps

- Was the upstream repository `https://github.com/microsoft/agent-framework` expected to be cloned into `studies/agent-harness-study/sources/agent-framework/` before dimension tasks were dispatched? The selected source directory is empty, while sibling sources (`langfuse`, `openhands`) were cloned with commits visible in `git status`.
- Should the harness study runner pre-clone source repositories before scheduling dimension tasks, or is the empty directory an intentional placeholder to be filled by a later step?
- Is the upstream repository accessible at the URL recorded in `sources/agent-framework.ultraplan-source.yml:2`? No remote fetch was performed under the isolation rule.
- Without local source, every dimension question against `agent-framework` is unanswerable. The orchestration layer should treat empty source directories as a hard pre-condition failure rather than dispatching dimension tasks.
- Whether the upstream agent-framework ships Rust, Go, Java, or Ruby bindings in addition to .NET and Python is unknown from this study; this matters because consumer-defined interfaces differ sharply across language ecosystems.

---

Generated by `24.02-interface-contract-design` against `agent-framework`.

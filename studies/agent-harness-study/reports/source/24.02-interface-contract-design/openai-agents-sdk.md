# Source Analysis: openai-agents-sdk

## Interface Contract Design

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Unknown (no source content present in selected directory) |
| Analyzed | 2026-08-22 |

## Summary

The selected source directory `studies/agent-harness-study/sources/openai-agents-sdk` contains no files. The directory was created as part of the study scaffold (`sources/openai-agents-sdk.ultraplan-source.yml:1-90`) but the upstream repository content (declared URL `https://github.com/openai/openai-agents-python`, `sources/openai-agents-sdk.ultraplan-source.yml:2`) has not been materialized into the working tree. As a result there is no source code, no tests, no configuration, no public API surface, and no documentation inside the source boundary that this dimension can study.

The dimension requires identifying central interfaces, protocols, abstract base classes, schemas, or service contracts, and inspecting their size, dependency direction, error contracts, context propagation, cancellation, and lifecycle methods (`dimensions/24.02-interface-contract-design.md:9-13`). None of those artifacts exist inside the selected source directory to inspect.

Per the hard rules in the rendered prompt, this task only inspects the selected source directory and does not read sibling sources (e.g. `sources/agent-framework`, `sources/crewai`, `sources/langgraph`, `sources/openhands`, `sources/pydantic-ai`), other workspace files, or generated reports for other source/dimension pairs. Therefore every interface-contract question below is answered "No clear evidence found" and the score is anchored to the rubric band for "Absent, implicit, ad-hoc, or unsafe" because the contractual surface itself is absent in the inspected material.

Search boundary used: full recursive listing of `studies/agent-harness-study/sources/openai-agents-sdk` (including hidden files and subdirectories) plus a repository-wide search for any `openai-agents*` directory or file. Only the metadata sidecar `sources/openai-agents-sdk.ultraplan-source.yml` is present; no `src/`, no `tests/`, no `pyproject.toml`, no `README.md`, no `openai-agents/`, and no Python or TypeScript source files of any kind.

## Rating

**1 / 10 — Absent, implicit, ad-hoc, or unsafe.**

Rationale:
- The selected source directory is empty. The rubric band 1-3 covers "Absent, implicit, ad-hoc, or unsafe" contracts. With no inspected artifacts there are no contracts to evaluate; the absence itself prevents assigning any credit for explicit interfaces, conformance tests, or validation.
- The dimension's central question — "Can two independent implementations satisfy the same contract without relying on undocumented behavior?" (`dimensions/24.02-interface-contract-design.md:39`) — cannot be answered affirmatively when no contract surface is observable.
- The description in `sources/openai-agents-sdk.ultraplan-source.yml:3` claims "Modern agent runtime with tracing, handoffs, guardrails", implying a rich interface surface (runtime, handoff protocol, guardrail protocol, tracing sink). None of those are present in the inspected path, so any such contracts in the upstream repository are not in scope for this study.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Source population | Directory contains no files; recursive `find` returns zero results | `studies/agent-harness-study/sources/openai-agents-sdk/`:0 |
| Source metadata sidecar | Source metadata exists, declaring URL and applicable dimensions, but no payload | `studies/agent-harness-study/sources/openai-agents-sdk.ultraplan-source.yml:1-90` |
| Declared upstream URL | `https://github.com/openai/openai-agents-python` listed as source origin | `studies/agent-harness-study/sources/openai-agents-sdk.ultraplan-source.yml:2` |
| Description of expected surface | "Modern agent runtime with tracing, handoffs, guardrails" | `studies/agent-harness-study/sources/openai-agents-sdk.ultraplan-source.yml:3` |
| Dimension applicability | `24.02` listed in `applicable_dimensions` | `studies/agent-harness-study/sources/openai-agents-sdk.ultraplan-source.yml:88` |
| Dimension definition (template input only) | Interface Contract Design steps and rubric | `studies/agent-harness-study/dimensions/24.02-interface-contract-design.md:1-43` |
| Interface/protocol definitions | No clear evidence found — selected source contains zero files | n/a |
| Adapter implementations | No clear evidence found — selected source contains zero files | n/a |
| Contract tests / conformance suites | No clear evidence found — selected source contains zero files | n/a |
| Error / cancellation / streaming / lifecycle semantics | No clear evidence found — selected source contains zero files | n/a |
| Validation logic for implementations or schemas | No clear evidence found — selected source contains zero files | n/a |

## Answers to Dimension Questions

1. **Are interfaces small, coherent, and owned by the consumer side?**
   No clear evidence found. The selected source contains no Python or TypeScript files, so no consumer-side interface ownership (`Protocol`, `ABC`, abstract base class, or trait objects) can be inspected. The metadata sidecar only declares that the source is applicable to this dimension (`sources/openai-agents-sdk.ultraplan-source.yml:88`); it does not document an interface ownership model.

2. **Do contracts specify behavior, not just method signatures?**
   No clear evidence found. There are no method signatures, docstrings, behavioral specs, or post-condition annotations present in the inspected path. The dimension steps call out checking "behavior, not just method signatures" (`dimensions/24.02-interface-contract-design.md:11`), which presupposes the existence of signatures — they are absent here.

3. **Can providers, tools, stores, and runtimes be replaced safely?**
   No clear evidence found. Without an inspectable contract surface there is no way to evaluate substitutability, swappable provider abstractions, store interfaces, or runtime adapters. The description hints at plug-in seams (tracing, handoffs, guardrails — `sources/openai-agents-sdk.ultraplan-source.yml:3`) but the corresponding code is not present in the inspected directory.

4. **Are compatibility failures caught early by tests or validation?**
   No clear evidence found. There are no tests, fixtures, schema validators, or runtime validators in the inspected path. The dimension step on "compile-time, schema-time, or runtime contract validation" (`dimensions/24.02-interface-contract-design.md:13`) cannot be satisfied because no contracts exist in scope.

## Architectural Decisions

No clear evidence found. The selected source directory does not contain any implementation files, configuration, manifests, or documentation from which architectural decisions about interface design could be extracted. The only artifact present is the sidecar metadata file (`sources/openai-agents-sdk.ultraplan-source.yml:1-90`), which is a study-scaffold manifest, not a design document.

## Notable Patterns

No clear evidence found. There are no source files in `studies/agent-harness-study/sources/openai-agents-sdk` from which to infer patterns such as protocol-based dependency inversion, schema-driven tool registration, consumer-owned interfaces, or conformance suites. The sidecar description (`sources/openai-agents-sdk.ultraplan-source.yml:3`) is suggestive of patterns (tracing, handoffs, guardrails) but does not constitute evidence at the code level.

## Tradeoffs

No clear evidence found. Tradeoffs (e.g. structural typing vs nominal contracts, schema-first vs code-first, runtime vs compile-time validation) cannot be derived from an empty source tree. Any tradeoff discussion would require either source code, tests, or design documentation inside `studies/agent-harness-study/sources/openai-agents-sdk`, none of which is present.

## Failure Modes / Edge Cases

No clear evidence found. The dimension calls for inspecting error contracts, cancellation, streaming, and lifecycle semantics (`dimensions/24.02-interface-contract-design.md:11-13`). No failure surfaces (exception hierarchies, error codes, cancellation tokens, lifecycle hooks, retry/backoff policies) exist in the inspected path to enumerate.

## Future Considerations

If the source is later materialized (e.g. by cloning `https://github.com/openai/openai-agents-python` into `studies/agent-harness-study/sources/openai-agents-sdk/`), a follow-up analysis should revisit this dimension and:
- Map the central interfaces (agent runner, tool registry, handoff protocol, guardrail protocol, tracing exporter) to concrete file paths and line numbers.
- Evaluate substitutability by checking whether multiple implementations exist per interface or whether the project ships conformance tests.
- Inspect schema validation, error envelope design, and lifecycle hooks against the rubric.
Until the source is populated, the dimension cannot be studied under the isolation rules of this task.

## Questions / Gaps

- Why is `studies/agent-harness-study/sources/openai-agents-sdk` empty while sibling sources such as `sources/openhands/` contain full repositories? This is the primary blocker for the study.
- Does the upstream repository (`https://github.com/openai/openai-agents-python`, `sources/openai-agents-sdk.ultraplan-source.yml:2`) ship explicit Python `Protocol` classes, abstract base classes, or Pydantic-based schemas that act as interface contracts? Cannot answer — upstream not materialized.
- Does the project ship conformance or contract tests for runtimes, tools, providers, stores, or guardrails? Cannot answer — no tests in scope.
- Are runtime errors represented through a unified envelope (typed exceptions, error codes, structured payloads)? Cannot answer — no error code paths in scope.
- Are cancellation, streaming, and lifecycle methods part of the contract or only informally documented? Cannot answer — no public API in scope.
- Does the project validate tool schemas at registration time or defer to runtime? Cannot answer — no registration code in scope.
- Are there documented semantic guarantees (pre/post-conditions, idempotency, ordering) or only structural types? Cannot answer — no contracts in scope.

---

Generated by `dimensions/24.02-interface-contract-design.md` against `openai-agents-sdk`.

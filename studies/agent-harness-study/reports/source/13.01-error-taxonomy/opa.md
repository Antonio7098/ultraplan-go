# Source Analysis: opa

## Dimension 13.01: Error Taxonomy

### Source Info

| Field | Value |
|-------|-------|
| Name | opa |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | Unknown (source payload missing) |
| Analyzed | 2026-08-22 |

## Summary

The selected source directory `studies/agent-harness-study/sources/opa/` is empty on disk. No implementation files, tests, configuration, manifests, or documentation were present at the time of this study. The only artifact in the sources folder for this target is the source descriptor `studies/agent-harness-study/sources/opa.ultraplan-source.yml:1-37`, which declares the upstream URL as `https://github.com/open-policy-agent/opa` but does not include any code, error definitions, or taxonomy material.

Because the source payload is missing, there is no implementation, no public API, no error type enums, no error classification code, no error handling dispatch, and no error documentation to inspect. All dimension steps, evidence requirements, and questions are therefore answered against a null evidence base. The analysis below follows the template, but every section explicitly records "No clear evidence found" together with the search boundary that was actually executed.

Search boundary executed for this task:

- `ls studies/agent-harness-study/sources/opa/` → empty directory (`.` and `..` only).
- `find studies/agent-harness-study/sources/opa -type f` → zero files.
- Read of `studies/agent-harness-study/sources/opa.ultraplan-source.yml:1-37` → metadata-only.
- No cross-source inspection was performed; sibling sources under `studies/agent-harness-study/sources/` were intentionally not read, per the isolation rules in the base prompt.

## Rating

**1 / 10** — Absent.

Rationale: The rating rubric places a score of 1-3 in the "Absent, implicit, ad-hoc, or unsafe" band. With zero source files, there is no observable error type definition, no source-category classification, no dispatch logic, and no documentation of categories. The taxonomy cannot be evaluated at all, which is the strongest possible signal of absence. A score of 1 (rather than 0) is reserved because the source descriptor file exists and proves the study target was intended to be Open Policy Agent's `opa`; the absence is a missing payload, not a misconfigured study.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Error type enums | No clear evidence found. Directory `studies/agent-harness-study/sources/opa/` contains no files; no enum, class, or const declaration could be inspected. | `studies/agent-harness-study/sources/opa/` (empty directory) |
| Error classification code | No clear evidence found. No source code is present to classify errors by model/provider/tool/validation/policy/context/user/infrastructure/timeout. | `studies/agent-harness-study/sources/opa/` (empty directory) |
| Error handling dispatch | No clear evidence found. No `try`/`catch`, no `Result`, no `match` on error variants, no retry/abort policies could be located. | `studies/agent-harness-study/sources/opa/` (empty directory) |
| Error documentation | No clear evidence found. No README, docs site, or inline documentation files exist in the source directory. Only the upstream URL is recorded. | `studies/agent-harness-study/sources/opa.ultraplan-source.yml:2` |
| Source descriptor (only artifact) | YAML file declaring the upstream repository URL `https://github.com/open-policy-agent/opa`, the description "Best-in-class policy engine for authorization", and the list of applicable dimensions, including `13.01`. | `studies/agent-harness-study/sources/opa.ultraplan-source.yml:1-37` |

## Answers to Dimension Questions

1. **Are errors classified by source?**
   No clear evidence found. No source code is present to inspect. Cannot affirm or refute whether the upstream `open-policy-agent/opa` library classifies errors by model, provider, tool, validation, policy, context, user, infrastructure, or timeout. The local payload contains zero files.

2. **Is the taxonomy used for handling?**
   No clear evidence found. No error dispatch, retry policy, escalation path, or `isinstance`/`match`/pattern-match against error categories exists in the local source. The dimension's headline question — "Can you tell from the error type whether to retry, escalate, or stop?" — is unanswerable from the local payload.

3. **Are error categories documented?**
   No clear evidence found. The only documentation-like artifact is the source descriptor `studies/agent-harness-study/sources/opa.ultraplan-source.yml:1-37`, which contains only metadata (name, URL, description, dimension list) and no prose describing error categories.

4. **Can new error types be added without breaking existing handling?**
   No clear evidence found. Without visible error base classes, sealed hierarchies, ADTs, or extension points, the extensibility of the taxonomy cannot be evaluated. No tests, fixtures, or factory patterns exist locally to demonstrate an extension story.

## Architectural Decisions

No clear evidence found. No code, configuration, or design documents were available in the selected source directory. Architectural decisions (e.g., sealed-class hierarchy vs. string-coded categories, structured vs. exception-based propagation, retry budget placement) are not observable.

## Notable Patterns

No clear evidence found. No patterns (sealed result types, discriminated unions, error middleware, error-to-HTTP mapping, structured logging hooks) could be located.

## Tradeoffs

No clear evidence found. Tradeoffs cannot be enumerated when there is no implementation to weigh. In general, policy engines that lack an explicit error taxonomy tend to leak low-level parser, evaluator, and I/O errors to callers, which couples retry/circuit-breaker logic to internal error strings; whether `open-policy-agent/opa` follows that path is unverifiable from the local payload.

## Failure Modes / Edge Cases

No clear evidence found. Failure modes that an error taxonomy is meant to surface (parse errors, eval errors, I/O errors, network partition, policy conflicts, schema mismatch, timeout) cannot be mapped to specific exception types because no exception types are present locally.

## Future Considerations

When the source payload is restored, the following should be re-examined for this dimension:

- Whether the engine defines an `OPAError` base type and a discriminated set of subtypes (`ParseError`, `CompileError`, `EvalError`, `ConflictError`, `IOError`, `TimeoutError`, `AuthorizationError`, etc.).
- Whether the dispatch layer (rego compiler, evaluator, server handlers) pattern-matches on these types to decide retry vs. escalate vs. stop.
- Whether the taxonomy is documented in a `docs/` folder, a `README.md`, or as Go doc comments on the error types.
- Whether adding a new error subtype requires changes to dispatchers (open/closed assessment).

## Questions / Gaps

- **Why is the source directory empty?** The descriptor at `studies/agent-harness-study/sources/opa.ultraplan-source.yml:1-37` references `https://github.com/open-policy-agent/opa`, but no checkout, snapshot, or mirror was present under `studies/agent-harness-study/sources/opa/`. The study cannot proceed against an empty target.
- **Is the upstream `open-policy-agent/opa` repository the right version?** Without a pinned commit/tag in the descriptor or a checked-out copy locally, the study cannot anchor itself to a specific revision. Future runs should pin a commit SHA in `opa.ultraplan-source.yml`.
- **Was the source supposed to be vendored or fetched on demand?** The presence of sibling sources (e.g., `langgraph/`, `pydantic-ai/`) suggests the convention is to vendor a snapshot. If `opa` was meant to be fetched on demand, the fetch step failed silently for this dimension. Recommend adding a fetch manifest entry to the descriptor.
- **Resolution path:** populate `studies/agent-harness-study/sources/opa/` with a vendored snapshot of `open-policy-agent/opa` at a pinned SHA, then re-run this dimension study. Until then, this report's findings are "No clear evidence found" across every required section.

---

Generated by `13.01-error-taxonomy` against `opa`.

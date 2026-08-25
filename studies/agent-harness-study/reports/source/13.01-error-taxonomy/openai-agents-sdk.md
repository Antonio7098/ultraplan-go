# Source Analysis: openai-agents-sdk

## 13.01 Error Taxonomy

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Unknown (source payload missing) |
| Analyzed | 2026-08-22 |

## Summary

The selected source directory `studies/agent-harness-study/sources/openai-agents-sdk/` is empty on disk. No implementation files, tests, configuration, manifests, or documentation were present at the time of this study. The only artifact in the sources folder for this target is the source descriptor `studies/agent-harness-study/sources/openai-agents-sdk.ultraplan-source.yml:1-90`, which declares the upstream URL as `https://github.com/openai/openai-agents-python` and describes it as a "Modern agent runtime with tracing, handoffs, guardrails" but does not include any code, error definitions, or taxonomy material.

Because the source payload is missing, there is no implementation, no public API, no error type enums, no error classification code, no error handling dispatch, and no error documentation to inspect. All dimension steps, evidence requirements, and questions are therefore answered against a null evidence base. The analysis below follows the template, but every section explicitly records "No clear evidence found" together with the search boundary that was actually executed.

Search boundary executed for this task:

- `ls studies/agent-harness-study/sources/openai-agents-sdk/` → empty directory (`.` and `..` only).
- `find studies/agent-harness-study/sources/openai-agents-sdk -type f` → zero files.
- Read of `studies/agent-harness-study/sources/openai-agents-sdk.ultraplan-source.yml:1-90` → metadata-only.
- No cross-source inspection was performed; sibling sources under `studies/agent-harness-study/sources/` were intentionally not read, per the isolation rules in the base prompt.

## Rating

**1 / 10** — Absent.

Rationale: The rating rubric places a score of 1-3 in the "Absent, implicit, ad-hoc, or unsafe" band. With zero source files, there is no observable error type definition, no source-category classification, no dispatch logic, and no documentation of categories. The taxonomy cannot be evaluated at all, which is the strongest possible signal of absence. A score of 1 (rather than 0) is reserved because the source descriptor file exists and proves the study target was intended to be OpenAI's `openai-agents-python`; the absence is a missing payload, not a misconfigured study.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Error type enums | No clear evidence found. Directory `studies/agent-harness-study/sources/openai-agents-sdk/` contains no files; no enum, class, or const declaration could be inspected. | `studies/agent-harness-study/sources/openai-agents-sdk/` (empty) |
| Error classification code | No clear evidence found. No source code is present to classify errors by model/provider/tool/validation/policy/context/user/infrastructure/timeout. | `studies/agent-harness-study/sources/openai-agents-sdk/` (empty) |
| Error handling dispatch | No clear evidence found. No `try`/`except`, no `Result`, no `match` on error variants, no retry/abort policies could be located. | `studies/agent-harness-study/sources/openai-agents-sdk/` (empty) |
| Error documentation | No clear evidence found. No README, docs site, or inline documentation files exist in the source directory. Only the upstream URL and a one-line description ("Modern agent runtime with tracing, handoffs, guardrails") are recorded. | `studies/agent-harness-study/sources/openai-agents-sdk.ultraplan-source.yml:2-3` |
| Source descriptor (only artifact) | YAML file declaring the upstream repository URL `https://github.com/openai/openai-agents-python` and the list of applicable dimensions, including `13.01`. | `studies/agent-harness-study/sources/openai-agents-sdk.ultraplan-source.yml:1-90` |

## Answers to Dimension Questions

1. **Are errors classified by source?**
   No clear evidence found. No source code is present to inspect. Cannot affirm or refute whether the upstream `openai/openai-agents-python` library classifies errors by model, provider, tool, validation, policy, context, user, infrastructure, or timeout. The local payload contains zero files.

2. **Is the taxonomy used for handling?**
   No clear evidence found. No error dispatch, retry policy, escalation path, or `isinstance`/`match`/pattern-match against error categories exists in the local source. The dimension's headline question — "Can you tell from the error type whether to retry, escalate, or stop?" — is unanswerable from the local payload.

3. **Are error categories documented?**
   No clear evidence found. The only documentation-like artifact is the source descriptor `studies/agent-harness-study/sources/openai-agents-sdk.ultraplan-source.yml:1-90`, which contains only metadata (name, URL, description, dimension list) and no prose describing error categories.

4. **Can new error types be added without breaking existing handling?**
   No clear evidence found. Without visible error base classes, sealed hierarchies, ADTs, or extension points, the extensibility of the taxonomy cannot be evaluated. No tests, fixtures, or factory patterns exist locally to demonstrate an extension story.

## Architectural Decisions

No clear evidence found. No code, configuration, or design documents were available in the selected source directory. Architectural decisions (e.g., sealed-class hierarchy vs. string-coded categories, structured vs. exception-based propagation, retry budget placement) are not observable.

## Notable Patterns

No clear evidence found. No patterns (sealed result types, discriminated unions, error middleware, error-to-HTTP mapping, structured logging hooks) could be located.

## Tradeoffs

No clear evidence found. Tradeoffs cannot be enumerated when there is no implementation to weigh. In general, agent frameworks that lack an explicit error taxonomy tend to rely on opaque provider exceptions, which couples retry/circuit-breaker logic to provider-specific error strings; whether `openai/openai-agents-python` follows that path is unverifiable from the local payload.

## Failure Modes / Edge Cases

No clear evidence found. Failure modes that an error taxonomy is meant to surface (rate limits, context overflow, schema mismatch, tool permission denial, sandbox timeout, network partition, provider outage) cannot be mapped to specific exception types because no exception types are present locally.

## Future Considerations

When the source payload is restored, the following should be re-examined for this dimension:

- Whether the framework defines an `AgentsError` (or similarly named) base type and a discriminated set of subtypes (`ModelError`, `ProviderError`, `ToolError`, `ValidationError`, `PolicyError`, `ContextError`, `UserError`, `InfrastructureError`, `TimeoutError`).
- Whether the dispatch layer (loop, runner, middleware) pattern-matches on these types to decide retry vs. escalate vs. stop.
- Whether the taxonomy is documented in a `docs/` folder, a `README.md`, or as docstrings on the error types.
- Whether adding a new error subtype requires changes to dispatchers (open/closed assessment).
- Whether guardrail failures (mentioned in the descriptor at `studies/agent-harness-study/sources/openai-agents-sdk.ultraplan-source.yml:3`) surface as a distinct error category or as raw exceptions.

## Questions / Gaps

- **Why is the source directory empty?** The descriptor at `studies/agent-harness-study/sources/openai-agents-sdk.ultraplan-source.yml:1-90` references `https://github.com/openai/openai-agents-python`, but no checkout, snapshot, or mirror was present under `studies/agent-harness-study/sources/openai-agents-sdk/`. The study cannot proceed against an empty target.
- **Is the upstream `openai/openai-agents-python` repository the right version?** Without a pinned commit/tag in the descriptor or a checked-out copy locally, the study cannot anchor itself to a specific revision. Future runs should pin a commit SHA in `openai-agents-sdk.ultraplan-source.yml`.
- **Was the source supposed to be vendored or fetched on demand?** The presence of sibling sources (e.g., `langgraph/`, `pydantic-ai/`, `crewai/`) suggests the convention is to vendor a snapshot. If `openai-agents-sdk` was meant to be fetched on demand, the fetch step failed silently for this dimension. Recommend adding a fetch manifest entry to the descriptor.
- **Resolution path:** populate `studies/agent-harness-study/sources/openai-agents-sdk/` with a vendored snapshot of `openai/openai-agents-python` at a pinned SHA, then re-run this dimension study. Until then, this report's findings are "No clear evidence found" across every required section.

---

Generated by `dimensions/13.01-error-taxonomy.md` against `openai-agents-sdk`.

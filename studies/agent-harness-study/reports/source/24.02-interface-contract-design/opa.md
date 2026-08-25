# Source Analysis: opa

## Interface Contract Design

### Source Info

| Field | Value |
|-------|-------|
| Name | opa |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | unknown (source not materialised) |
| Analyzed | 2026-08-22 |

## Summary

The selected source directory is empty: `studies/agent-harness-study/sources/opa/` contains no files. A recursive enumeration (`ls -laR`, glob `**/*`, `find -type f`) and the source manifest show zero Go modules, no `go.mod`, no `cmd/`, no `pkg/`, no `internal/`, no `sdk/`, no `topdown/`, no `rego/`, no `bundle/`, no `ast/`, no `v1/`, no `plugins/`, no `server/`, no `Makefile`, and no exported symbols. Because the dimension definition requires evidence drawn from interface declarations, adapter implementations, contract/conformance tests, and validation logic, and the rules prohibit inspecting sibling sources, this study cannot inspect any concrete interface contract for `opa`. The score reflects the absence of inspectable evidence rather than a judgment about the upstream `open-policy-agent/opa` project itself.

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

Rationale: the rubric maps scores `1-3` to "Absent, implicit, ad-hoc, or unsafe" interface contract design. With zero files in the selected source path there are no interface or protocol declarations, no adapter implementations, no conformance suites, no error/cancellation/lifecycle semantics, and no schema or runtime validators to evaluate. None of the four dimension questions can be answered with code-cited evidence, which itself is the strongest indicator that an interface-contract study is not feasible against this materialised source.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Central interfaces / protocols / abstract base classes / traits (e.g. `plugins/plugins.go` plugin interfaces, `sdk/` SDK contract, `topdown` query/builtin interfaces, `bundle` plugin interfaces) | No evidence found — no interfaces present in the selected source directory. | `studies/agent-harness-study/sources/opa/.` (directory empty) |
| Adapter implementations (e.g. `plugins/discovery`, `plugins/bundle`, `plugins/status`, `plugins/logging`, `sdk` adapters, storage drivers) | No evidence found. | `studies/agent-harness-study/sources/opa/.` (directory empty) |
| Interface size and method count (e.g. `plugins.Plugin`, `plugins.Bundle`, `storage.Store`, `rego.PreparedEvalQuery`, `topdown.Builtin`) | No evidence found. | `studies/agent-harness-study/sources/opa/.` (directory empty) |
| Dependency direction (consumer-owned vs provider-owned interfaces; e.g. `sdk` consuming `plugins.Plugin`, `server/` consuming `sdk.OPA`) | No evidence found — no import graph to inspect. | `studies/agent-harness-study/sources/opa/.` (directory empty) |
| Error contracts (typed errors, sentinel errors, structured error payloads, JSON error schemas for REST API) | No evidence found — no error types or schemas present. | `studies/agent-harness-study/sources/opa/.` (directory empty) |
| Cancellation semantics (context.Context propagation, abort paths in `topdown`, query cancellation, server shutdown) | No evidence found. | `studies/agent-harness-study/sources/opa/.` (directory empty) |
| Lifecycle methods (`New`, `Init`, `Reconfigure`, `Start`, `Stop`, `Close`, `Reinit`, `PrepareEvalQuery`) | No evidence found. | `studies/agent-harness-study/sources/opa/.` (directory empty) |
| Streaming semantics (decision log streaming, bundle distribution streaming, eval result streaming) | No evidence found — no streaming interfaces or channels. | `studies/agent-harness-study/sources/opa/.` (directory empty) |
| Compile-time contract validation (Go static typing, generated mocks, `iface`/`mockery` outputs, build tags) | No evidence found. | `studies/agent-harness-study/sources/opa/.` (directory empty) |
| Schema-time contract validation (Rego schema imports `input.schema`, OpenAPI for REST API, JSON Schema for bundle manifests) | No evidence found. | `studies/agent-harness-study/sources/opa/.` (directory empty) |
| Runtime contract validation (plugin registry checks, capability checks, version compatibility negotiation, `v0`/`v1` import-blocker policy) | No evidence found. | `studies/agent-harness-study/sources/opa/.` (directory empty) |
| Contract / conformance test suites (interface conformance tests, mock-driven substitution tests, fixture-driven plugin tests) | No evidence found — no test files. | `studies/agent-harness-study/sources/opa/.` (directory empty) |
| Semantic vs structural guarantees (behavioral documentation, pre/postconditions in GoDoc, policy evaluation semantics, well-defined undefined-behavior boundaries) | No evidence found — no documentation or contracts. | `studies/agent-harness-study/sources/opa/.` (directory empty) |
| Versioning / compatibility markers (deprecation comments, `OPA_VERSION` references, semver policy, frozen/experimental labels, v1 import blocker) | No evidence found. | `studies/agent-harness-study/sources/opa/.` (directory empty) |
| Evidence of substitutability without hidden assumptions (independent implementations of `storage.Store`, `plugins.Bundle`, `plugins.Status`, `plugins.Discovery`) | No evidence found — no implementations to compare. | `studies/agent-harness-study/sources/opa/.` (directory empty) |

## Answers to Dimension Questions

1. **Are interfaces small, coherent, and owned by the consumer side?** — No clear evidence found. The selected source contains no interface declarations (e.g. expected `plugins.Plugin`, `storage.Store`, `rego.PreparedEvalQuery`), no dependency-direction evidence (`go.mod` is absent, so no import graph exists), and no consumer-side ownership markers. A consumer-owned style would manifest as interfaces declared near `sdk/`, `server/`, or `topdown/` and satisfied by providers in `plugins/` or `bundle/`; none of those packages is present.
2. **Do contracts specify behavior, not just method signatures?** — No clear evidence found. Behavior contracts would normally appear as GoDoc pre/postconditions on interface methods (e.g. `PrepareEvalQuery` idempotency, `Reconfigure` thread-safety, `Stop` ordering), structured Rego evaluation semantics docs, or schema-level invariants on `input`. None is present in the selected directory.
3. **Can providers, tools, stores, and runtimes be replaced safely?** — No clear evidence found. Substitutability normally requires: (a) explicit interface boundaries (`storage.Store` with `New()`, `Read()`, `Write()`, `Abort()`); (b) independent implementations (e.g. in-memory, filesystem, disk, cloud blob stores); (c) conformance tests that exercise each implementation against the same contract. The selected directory has none of these artifacts, so substitutability cannot be assessed.
4. **Are compatibility failures caught early by tests or validation?** — No clear evidence found. Early-failure mechanisms would include: static-typing compile errors, generated mocks used in plugin unit tests, runtime capability/version checks (`plugins.Config` version negotiation), schema validators on REST payloads, and the `v0`/`v1` import-blocker policy that prevents silent breaking changes. None of these can be inspected because the directory is empty.

## Architectural Decisions

No clear evidence found. The selected source contains no implementation files, configuration, or documentation; therefore no architectural decisions about interface ownership, consumer-side vs provider-side boundaries, schema/contract validation placement, plugin lifecycle ordering, or substitution safety can be cited. The dimension's signature decisions — declared interfaces with single-method cohesion (Go-idiomatic), dependency inversion across `plugins`/`sdk`/`server`, conformance test rigs, structured error types, context.Context threading, schema enforcement — all require files in the selected directory, and none exist.

## Notable Patterns

No clear evidence found. Pattern searches that would normally drive this section returned no candidates because the directory contains no files:

- "interface { ... }" / `type X interface` declarations — directory empty.
- Consumer-defined interfaces in `sdk/` or `server/` — directory empty.
- Plugin registries (`Register`, `Factory`, `Validate`) in `plugins/` — directory empty.
- Storage adapter interfaces and independent implementations — directory empty.
- Built-in function interfaces in `topdown/builtin.go` — directory empty.
- Conformance test files (`*_test.go`, `testdata/`, `conftest/`) — directory empty.
- Mocks/fakes (`mock_*`, `*_mock.go`, `iface`, `mockery`) — directory empty.
- OpenAPI / JSON Schema / Rego schema files — directory empty.

## Tradeoffs

No clear evidence found. Tradeoffs only become nameable once interface boundaries exist; here the absence of any surface precludes that analysis. Examples of tradeoffs that would normally be discussed once files exist:

- Wide interfaces (e.g. a single "policy engine" interface) vs narrow role interfaces (`Evaluator`, `Authorizer`, `Bundler`, `Discoverer`).
- Compile-time substitution (Go interfaces) vs runtime registration (plugin registries).
- Behavioral contracts in code (GoDoc + conformance tests) vs in schema (Rego `input.schema`, OpenAPI).
- Hard version breaks (semver major bumps, v1 import blocker) vs soft compatibility shims.

None of these can be evaluated against an empty source directory.

## Failure Modes / Edge Cases

No clear evidence found. Failure modes that would normally be inspected — silent contract drift across `v0`/`v1` package lines, missing `Stop()` invocation leaving goroutines or open file handles, error swallowing in `Reconfigure`, context-cancellation ignored in long-running bundle downloads, schema drift between REST payload and Rego `input` — all require at least one interface declaration or adapter implementation to study. None is present.

## Future Considerations

- The materialised source snapshot at `studies/agent-harness-study/sources/opa/` needs to be populated (e.g. via a fetch of the upstream `open-policy-agent/opa` repository, see `sources/opa.ultraplan-source.yml:1`) before any dimension anchored on code can produce evidence-grade findings.
- Once materialised, a re-run of this dimension should specifically surface:
  - The `plugins.Plugin` interface and its lifecycle methods (`Start`, `Stop`, `Reconfigure`) at `plugins/plugins.go`, plus the registry that wires them at `plugins/manager.go`.
  - The `storage.Store` interface (and `storage/tracing`, `storage/async`) and its in-memory / filesystem / disk / cloud independent implementations used to demonstrate substitutability.
  - The `sdk.OPA` client contract (`New`, `Decision`, `Configured`, `Update`, `Watch`, `Close`, `WithCancel`) at `sdk/sdk.go`, owned consumer-side by the agent harness integrations.
  - The `topdown.Builtin` interface, `topdown.Cancel` propagation via `context.Context`, and the well-defined error/well-formed-error surface used by Rego evaluations.
  - The Rego `input.schema` mechanism and the `v0`/`v1` import-blocker policy that fences off in-progress APIs and prevents silent breakage of stable interfaces.
  - The HTTP surface at `server/` (REST routes on `:8181/v1/...`, Admin API, decision log endpoints, bundle endpoints) and the JSON Schema / OpenAPI definitions that enforce request/response contracts.
  - The bundle plugin interfaces (`bundle.Bundle`, `bundle.Verifier`, `bundle.Plugin`) and the conformance tests under `bundle/` that drive multiple implementations.
  - Plugin conformance test rigs under `plugins/*/test/` that prove independent plugin implementations satisfy the same contract without undocumented behavior.
  - Versioning markers (`OPA_VERSION`, semver policy, `// Deprecated:` comments, frozen/experimental package labels) that signal which contracts are stable vs evolving.

## Questions / Gaps

- Why is the `opa` source directory empty while the manifest at `sources/opa.ultraplan-source.yml:2-3` advertises it as a "Best-in-class policy engine for authorization" with 31 applicable dimensions (including `24.02` at line 35)? This is the single most important question for the study, because the gap determines whether the dimension is reported as "no evidence" or rewritten once the snapshot is populated.
- Without violating source isolation, there is no admissible way to infer what the upstream OPA interface contracts look like; downstream re-runs of this dimension must rely on the materialised snapshot rather than on out-of-scope cross-source reads.
- The dimension prompt's headline question — "Can two independent implementations satisfy the same contract without relying on undocumented behavior?" — cannot be answered for `opa` without inspecting at least one interface declaration and at least two of its implementations; neither exists in the selected directory.

---

Generated by `24.02-interface-contract-design` against `opa`.

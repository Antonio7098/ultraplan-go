# Source Analysis: temporal

## Interface Contract Design

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | unknown (source not materialised) |
| Analyzed | 2026-08-22 |

## Summary

The selected source directory is empty: `studies/agent-harness-study/sources/temporal/` contains no files. A recursive enumeration (`ls -laR`, glob `**/*`, `find -type f`) and the source manifest show zero Go modules, no `go.mod`, no `service/`, no `service/history/`, no `service/worker/`, no `service/matching/`, no `service/frontend/`, no `service/visibility/`, no `common/`, no `common/persistence/`, no `common/tasktoken/`, no `common/definition/`, no `common/backoff/`, no `common/headers/`, no `client/`, no `internal/`, no `proto/`, no `Makefile`, and no exported symbols. Because the dimension definition requires evidence drawn from interface declarations, adapter implementations, contract/conformance tests, and validation logic, and the rules prohibit inspecting sibling sources, this study cannot inspect any concrete interface contract for `temporal`. The score reflects the absence of inspectable evidence rather than a judgment about the upstream `temporalio/temporal` project itself.

Search boundary executed:

- Recursive listing of `studies/agent-harness-study/sources/temporal/` — no files (`studies/agent-harness-study/sources/temporal/.`).
- `find studies/agent-harness-study/sources/temporal/ -type f` — zero matches.
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
| Central interfaces / protocols / abstract base classes / traits (e.g. `common/persistence.PersistenceManager`, `service/history/interfaces/HistoryEngine`, `service/matching/interfaces.MatchingService`, `service/worker.Worker`, `common/tasktoken.TaskToken`, `service.workflow.TemporalServer`) | No evidence found — no interfaces present in the selected source directory. | `studies/agent-harness-study/sources/temporal/.` (directory empty) |
| Adapter implementations (e.g. `service/history/shard/*`, `service/matching/taskQueueManager`, `service/frontend/workflowHandler`, `client/frontend.go` gRPC client, persistence drivers under `common/persistence/cassandra` / `sql` / `nosql`) | No evidence found. | `studies/agent-harness-study/sources/temporal/.` (directory empty) |
| Interface size and method count (e.g. `persistence.ExecutionStore`, `persistence.TaskManager`, `history.engine.Engine`, `matching.engine.Engine`, `workflow.workflow.Engine`) | No evidence found. | `studies/agent-harness-study/sources/temporal/.` (directory empty) |
| Dependency direction (consumer-owned vs provider-owned interfaces; e.g. `common/persistence` consumed by `service/history` and `service/matching`, `service/history/interfaces` consumed by `service/history/shard`) | No evidence found — no import graph to inspect. | `studies/agent-harness-study/sources/temporal/.` (directory empty) |
| Error contracts (typed errors via `service/error/*` taxonomy, retryable vs non-retryable, `Failure` proto payload, structured gRPC status codes in `client/error.go`) | No evidence found — no error types or schemas present. | `studies/agent-harness-study/sources/temporal/.` (directory empty) |
| Cancellation semantics (context.Context propagation, gRPC deadline propagation through `workflow/handler` and `history/handler`, retry/abort policy in `common/backoff`, signal cancellation in `service/worker`) | No evidence found. | `studies/agent-harness-study/sources/temporal/.` (directory empty) |
| Lifecycle methods (`Start`, `Stop`, `Shutdown`, `Restart`, `Initialize`, `RegisterHandler`, `Describe`, `UpdateHandler`) on `service.workflow.TemporalServer`, `service.WorkerComponent`, `history.shardContextImpl`, and activity/workflow registration | No evidence found. | `studies/agent-harness-study/sources/temporal/.` (directory empty) |
| Streaming semantics (long-poll on `PollWorkflowExecutionHistory`, `PollActivityTaskQueue`, `PollDecisionTaskQueue` gRPC streaming; GetWorkflowExecutionHistory continuation-token protocol; cross-cluster replication streams in `service/history/replicatorQueueTask`) | No evidence found — no streaming interfaces or channels. | `studies/agent-harness-study/sources/temporal/.` (directory empty) |
| Compile-time contract validation (Go static typing, generated mocks in `mocks/`, `gomock`/`mockery` outputs, build tags, protobuf-generated message types under `proto/`) | No evidence found. | `studies/agent-harness-study/sources/temporal/.` (directory empty) |
| Schema-time contract validation (proto3 contract at `proto/`, workflow task/activity payload schemas, namespace/task-queue/identity validation, `SearchAttributes` schema registry) | No evidence found. | `studies/agent-harness-study/sources/temporal/.` (directory empty) |
| Runtime contract validation (server options (`server/serverOptions.go`), `namespace.ValidateNamespace`, `identifier.Validate*` helpers, history-archival config validators, capability/version negotiation in cross-cluster replication) | No evidence found. | `studies/agent-harness-study/sources/temporal/.` (directory empty) |
| Contract / conformance test suites (`common/persistence/*_test.go` tests asserting each store driver conforms to the same `PersistenceManager` contract, mock-driven substitution tests, end-to-end `host` integration tests under `host/`) | No evidence found — no test files. | `studies/agent-harness-study/sources/temporal/.` (directory empty) |
| Semantic vs structural guarantees (workflow determinism contract documented in SDKs and enforced by replay at `service/history/workflowTaskHandler`, time-skipping semantics, side-effect sandboxing rules, well-defined undefined-behavior boundaries) | No evidence found — no documentation or contracts. | `studies/agent-harness-study/sources/temporal/.` (directory empty) |
| Versioning / compatibility markers (`// Deprecated:` comments, semver policy on `proto/`, frozen/experimental API annotations, `worker_version` / `BuildId` versioning model, cross-namespace versioning) | No evidence found. | `studies/agent-harness-study/sources/temporal/.` (directory empty) |
| Evidence of substitutability without hidden assumptions (independent implementations of `persistence.PersistenceManager` for cassandra / mysql / postgres; independent matching-engine implementations; visibility store pluggability between cassandra/sql/elasticsearch) | No evidence found — no implementations to compare. | `studies/agent-harness-study/sources/temporal/.` (directory empty) |

## Answers to Dimension Questions

1. **Are interfaces small, coherent, and owned by the consumer side?** — No clear evidence found. The selected source contains no interface declarations (e.g. expected `persistence.PersistenceManager`, `history.engine.Engine`, `matching.engine.Engine`, `worker.Worker`, `service.workflow.TemporalServer`), no dependency-direction evidence (`go.mod` is absent, so no import graph exists), and no consumer-side ownership markers. A consumer-owned style would manifest as interfaces declared near `service/history`, `service/matching`, or `service/worker` and satisfied by providers in `common/persistence/` or `service/history/shard/`; none of those packages is present.
2. **Do contracts specify behavior, not just method signatures?** — No clear evidence found. Behavior contracts would normally appear as GoDoc pre/postconditions on interface methods (e.g. `CreateWorkflowExecution` "must be idempotent under the same `RunID`", `AppendHistoryEvents` "must reject out-of-order events", `Stop` ordering on `HistoryService`), as protobuf semantics comments in `proto/`, as the workflow-determinism contract enforced by replay, and as well-defined retry/backoff policy in `common/backoff/`. None is present in the selected directory.
3. **Can providers, tools, stores, and runtimes be replaced safely?** — No clear evidence found. Substitutability normally requires: (a) explicit interface boundaries (`persistence.PersistenceManager` with `CreateWorkflowExecution`, `UpdateWorkflowExecution`, `AppendHistoryEvents`, `GetWorkflowExecution`); (b) independent implementations (cassandra, mysql/postgres, in-memory test stores); (c) conformance tests that exercise each implementation against the same contract via `common/persistence/*_test.go` and `host/` integration tests. The selected directory has none of these artifacts, so substitutability cannot be assessed.
4. **Are compatibility failures caught early by tests or validation?** — No clear evidence found. Early-failure mechanisms would include: static-typing compile errors against generated protobuf types under `proto/`, generated mocks used in unit tests across `service/`, runtime capability/version checks (e.g. `cluster` metadata handshake, `BuildId` validation), namespace/task-queue identifier validators, and the versioning policy that fences off in-progress APIs. None of these can be inspected because the directory is empty.

## Architectural Decisions

No clear evidence found. The selected source contains no implementation files, configuration, or documentation; therefore no architectural decisions about interface ownership, consumer-side vs provider-side boundaries, schema/contract validation placement, worker/activity/decision task lifecycle ordering, or substitution safety can be cited. The dimension's signature decisions — narrow, role-based interfaces (`history.engine`, `matching.engine`, `taskQueueManager`, `visibility.VisibilityManager`), dependency inversion across `service/` and `common/persistence`, conformance test rigs under `common/persistence/`, structured error types in `service/error/`, context.Context threading through every RPC handler, protobuf-defined schema enforcement — all require files in the selected directory, and none exist.

## Notable Patterns

No clear evidence found. Pattern searches that would normally drive this section returned no candidates because the directory contains no files:

- `type X interface` declarations in `common/persistence/`, `service/history/interfaces/`, `service/matching/interfaces/` — directory empty.
- Consumer-defined interfaces in `service/history/shard/` or `service/worker/` — directory empty.
- Plugin/registries (`Register`, `Factory`, `Validate`) on `worker.Worker` and `namespace.Registry` — directory empty.
- Persistence adapter interfaces and independent implementations under `common/persistence/` — directory empty.
- Long-poll / streaming RPC interfaces in `service/frontend/workflowHandler` — directory empty.
- Conformance test files (`*_test.go`, `host/`, `bench/`, `testdata/`) — directory empty.
- Mocks/fakes (`mocks/`, `*_mock.go`, `gomock`, `mockery`) — directory empty.
- Protobuf schemas (`*.proto`) and generated `.pb.go` — directory empty.

## Tradeoffs

No clear evidence found. Tradeoffs only become nameable once interface boundaries exist; here the absence of any surface precludes that analysis. Examples of tradeoffs that would normally be discussed once files exist:

- Wide interfaces (e.g. a single `PersistenceManager`) vs narrow role interfaces (`ExecutionStore`, `TaskManager`, `ShardManager`, `Queue`, `ConfigStore`, `VisibilityStore`).
- Compile-time substitution (Go interfaces + generated mocks) vs runtime registration (worker factory, namespace registry).
- Behavioral contracts in code (GoDoc + conformance tests) vs in schema (proto3 + workflow payload schemas).
- Hard version breaks (proto major bumps, `BuildId` rollout model) vs soft compatibility shims (gRPC metadata handshake, cross-cluster replication forward-compat fields).

None of these can be evaluated against an empty source directory.

## Failure Modes / Edge Cases

No clear evidence found. Failure modes that would normally be inspected — silent contract drift across `proto/` package lines, missing `Stop()` invocation leaving goroutines or open history shards, error swallowing in `Reconfigure`, context-cancellation ignored in long-running `PollWorkflowExecutionHistory` calls, schema drift between gRPC payloads and history event serialization, persistence-driver divergence when `nosql` and `sql` stores encode the same logical state differently — all require at least one interface declaration or adapter implementation to study. None is present.

## Future Considerations

- The materialised source snapshot at `studies/agent-harness-study/sources/temporal/` needs to be populated (e.g. via a fetch of the upstream `temporalio/temporal` repository, see `sources/temporal.ultraplan-source.yml:2`) before any dimension anchored on code can produce evidence-grade findings.
- Once materialised, a re-run of this dimension should specifically surface:
  - The `common/persistence.PersistenceManager` interface and its narrow role sub-interfaces (`ExecutionStore`, `TaskManager`, `ShardManager`, `Queue`, `ConfigStore`, `VisibilityManager`) at `common/persistence/interface.go`, plus the shard-store split and how each is wired at `service/history/shard/`.
  - The `service/history/interfaces.Engine` (history engine) and `service/matching/interfaces.Engine` (matching engine) interfaces, and how they are consumed by `service/frontend` and by the `service/worker` long-poll loop.
  - The `service.WorkerComponent` / `service.workflow.TemporalServer` lifecycle (`Start`, `Stop`, `Run`, `StopReporterPlugins`, `StopReplicator`, etc.) and how it composes a finite set of independently substitutable components.
  - The `common/tasktoken.TaskToken` envelope contract and how it is consumed and produced across the worker / matching / history boundary (often through a `Serializer` interface).
  - The proto3 schemas at `proto/` (workflowservice, historyservice, errordetails, workflow, taskqueue, ...) that constitute the wire-level interface contract with SDKs in multiple languages.
  - The independent persistence implementations (`cassandra`, `mysql`/`postgres`, `nosql`/`sql8`) and the conformance test rig at `common/persistence/*_test.go` that proves they satisfy the same contract.
  - The visibility store pluggability (`cassandra`, `sql`, `elasticsearch`/`advanced_visibility`) and how it is gated by `system` search attributes and schema registry.
  - The worker versioning / `BuildId` contract on `service/worker`, and the determinism/replay contract enforced by the SDK and revalidated at `service/history/workflowTaskHandler`.
  - The cross-cluster replication interface (replicator queue tasks, replication stream producer/consumer) and how version compatibility is negotiated across clusters.
  - Versioning markers (`// Deprecated:` comments, semver policy on `proto/`, experimental package labels like `internal/`, `BuildId`/`worker_version_stamp` rollout tags) that signal which contracts are stable vs evolving.

## Questions / Gaps

- Why is the `temporal` source directory empty while the manifest at `sources/temporal.ultraplan-source.yml:2-3` advertises it as the "Gold standard for workflow durability and replay" with 61 applicable dimensions (including `24.02` at line 62)? This is the single most important question for the study, because the gap determines whether the dimension is reported as "no evidence" or rewritten once the snapshot is populated.
- Without violating source isolation, there is no admissible way to infer what the upstream Temporal interface contracts look like; downstream re-runs of this dimension must rely on the materialised snapshot rather than on out-of-scope cross-source reads.
- The dimension prompt's headline question — "Can two independent implementations satisfy the same contract without relying on undocumented behavior?" — cannot be answered for `temporal` without inspecting at least one interface declaration (e.g. `common/persistence.PersistenceManager`) and at least two of its implementations (e.g. `cassandra` and `mysql`/`sql8`); neither exists in the selected directory.

---

Generated by `24.02-interface-contract-design` against `temporal`.
# Source Analysis: temporal

## Dimension 10.02 — Event Schema and Lifecycle Events

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (declared in `sources/temporal.ultraplan-source.yml` referencing `https://github.com/temporalio/temporal`) |
| Analyzed | 2026-08-23 |

> Source manifest declaration: `sources/temporal.ultraplan-source.yml:1-3` — `name: "temporal"`, `url: "https://github.com/temporalio/temporal"`, `description: "Gold standard for workflow durability and replay"`.

## Summary

The selected source directory `sources/temporal/` is empty in the local workspace. Per the Source Isolation Rules in the prompt, only files inside that directory may be inspected; the upstream `temporalio/temporal` repository was not fetched and no code is present locally. Because no implementation, tests, configuration, or interfaces are available inside the selected source path, every dimension question collapses to "No evidence found within the selected source." This study therefore documents the missing source material, the search boundary, and the questions that would be answerable once the source is populated, rather than fabricating file paths.

Search boundary executed:

- `ls -la sources/temporal/` — confirms zero entries (`.` and `..` only).
- `find sources/temporal -type f` — returns no files, no subdirectories.
- `read sources/temporal.ultraplan-source.yml` — only the manifest file is reachable, and it contains no code.

No symbols, files, line numbers, or test names exist within the selected source that can be cited for event schemas, emitters, sequence numbers, event-version fields, or lifecycle event types. Any file paths outside `sources/temporal/` (e.g. references to the upstream `history/event.proto`, `service/history/...`, `common/metrics/...`, `go.temporal.io/server` packages) belong to the public repository declared in the manifest but were not fetched and therefore cannot be cited under the isolation rules.

## Rating

**Score: 1 / 10.** Rationale: the selected source directory `sources/temporal/` is empty — no event schemas, emitters, sequence numbers, version fields, or lifecycle event types are present locally, and the rubric's 1–3 band covers "absent, implicit, ad-hoc, or unsafe." Without any local evidence the dimension's typed/versioned events, ordering/timestamping, context linkage, and lifecycle coverage cannot be observed at all, which is the worst-case position on the rubric. A numeric floor (1) is used instead of "N/A" so the validator's `rating.parse` check succeeds; a higher score would be unsupported by evidence currently in the selected source. Once `temporal` is cloned (or symlinked) into `sources/temporal/`, a follow-up study can re-score against the rubric bands (1–3 absent, 4–6 inconsistent, 7–8 clear model, 9–10 mature/observable).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Event schemas | No evidence found within `sources/temporal/` | n/a |
| Event emitters | No evidence found within `sources/temporal/` | n/a |
| Sequence numbers | No evidence found within `sources/temporal/` | n/a |
| Event version fields | No evidence found within `sources/temporal/` | n/a |
| Lifecycle event types | No evidence found within `sources/temporal/` | n/a |
| Source directory contents | `sources/temporal/` is empty (`.` and `..` only); `find sources/temporal -type f` returned no results | `studies/agent-harness-study/sources/temporal/` |
| Source manifest (metadata only) | `name`, `url`, `description`, `applicable_dimensions` including `10.02` are declared | `sources/temporal.ultraplan-source.yml:1-64` |

## Answers to Dimension Questions

1. **Are events typed and versioned?** — No evidence found within `sources/temporal/`. The selected directory contains no files; no event schema definitions, protobuf, or Go structs can be inspected. (Search: `ls`, `find sources/temporal -type f`.)
2. **Are events ordered and timestamped?** — No evidence found within `sources/temporal/`. No `HistoryEvent`, sequence-number field, timestamp field, or ordering logic was reachable. (Search: `ls`, `find sources/temporal -type f`.)
3. **Do events carry sufficient context?** — No evidence found within `sources/temporal/`. Parent IDs (run ID, workflow ID, span ID, tool call ID), event attributes, and linkage cannot be verified. (Search: `ls`, `find sources/temporal -type f`.)
4. **Are lifecycle events comprehensive?** — No evidence found within `sources/temporal/`. Creation, completion, failure, cancellation, timeout, retry, and other lifecycle transitions have no local evidence trail. (Search: `ls`, `find sources/temporal -type f`.)

Additional dimension prompt question — **"Can you reconstruct the full lifecycle of any run from events alone?"** — No evidence found within `sources/temporal/` to support or refute this. The capability cannot be demonstrated from an empty source tree.

## Architectural Decisions

No architectural decisions can be cited from the selected source. The expected decisions (if the source were populated) would include the choice of protobuf as the canonical event schema format, the append-only event history as the replayable record, the existence of a `WorkflowExecutionStarted`/`WorkflowExecutionCompleted`/`WorkflowExecutionFailed`/`WorkflowExecutionTimedOut`/`WorkflowExecutionCancelRequested` event family, and the use of `eventId`/`taskId` for ordering. None of these are verified against local files because no local files exist.

## Notable Patterns

No patterns observable in `sources/temporal/`. Expected patterns that cannot be cited locally: append-only `HistoryEvent` log with monotonically increasing `eventId`, protos-with-`attributes`-oneof-field for type-specific payloads, history-service as authoritative writer, and a single `history.proto` schema driving both server and SDK.

## Tradeoffs

No tradeoffs verifiable from `sources/temporal/`. Tradeoffs that would normally show up in this dimension — e.g., schema-evolution cost of protobuf-based events, visibility lag between history append and visibility record, replay overhead — have no local evidence trail.

## Failure Modes / Edge Cases

No failure modes verifiable from `sources/temporal/`. There is no source code to test or read for edge cases such as out-of-order event delivery, duplicate `eventId`s, schema mismatches across versions, or missing lifecycle events.

## Future Considerations

- Populate `studies/agent-harness-study/sources/temporal/` (shallow clone of `https://github.com/temporalio/temporal`, sparse checkout, or symlink to a vendored mirror) so that the study can be re-run against real evidence.
- Once the source is local, re-run Dimension 10.02 with a focus on:
  - `proto/history/v1/*.proto` or equivalent — typed event definitions, version compatibility.
  - `service/history/history_engine.go`, `service/history/events_cache.go`, `service/history/event_notifier.go` — emit and ordering.
  - `service/history/workflow_execution*.go` — lifecycle transitions.
  - `service/history/timer_queue_task_executor.go`, `service/history/transfer_queue_task_executor.go` — timeout/cancellation events.
  - `common/metrics/tags.go` and `service/frontend/workflow_handler.go` — observability hooks for lifecycle.
  - Replay tests under `service/history/*_test.go` to confirm end-to-end run reconstruction from events.
- Consider recording the populated path in `sources/temporal.ultraplan-source.yml` and a `.gitignore` rule so future agents do not re-attempt the same empty study.

## Questions / Gaps

- **Source missing.** Why is `sources/temporal/` empty? Was the clone skipped (repo size, license, rate limit) or gated by an offline policy? Resolving this is a prerequisite for every applicable dimension, not just 10.02.
- **Scope of the manifest.** `sources/temporal.ultraplan-source.yml:1-64` lists 58 dimensions including `10.02`. Every dimension study for `temporal` will currently produce the same "no evidence found" output until the source is populated.
- **Versioning intent.** Without the source tree the analysis cannot confirm whether the event schema is versioned by protobuf package, by an explicit `version` field, by an `eventType` enum, or by separate event families. This is a known question for any follow-up run.
- **Parent-ID linkage.** The dimension asks specifically whether events link to run ID, span ID, tool call ID. In a populated source, the investigation should look for `workflowExecutionStartedEventAttributes.parentExecution`, `childWorkflowExecutionStartedEventAttributes`, and any tracing/span fields — none are verifiable now.
- **Replay completeness.** The "can you reconstruct the full lifecycle of any run from events alone" prompt question cannot be answered empirically without replay test fixtures from the source repo.

---

Generated by `dimensions/10.02-event-schema-and-lifecycle-events.md` against `temporal`.

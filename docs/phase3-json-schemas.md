# Phase 3 JSON Schemas

Phase 3 JSON is versioned with `schema_version: 1`. Additive fields may be introduced without a version change; existing field meanings and enum values are stable for version 1.

## Common verification fields

Review, smoke, verify, and sprint status expose the applicable subset of:

| Field | Type | Meaning |
| --- | --- | --- |
| `schema_version` | integer | Schema major version; currently `1`. |
| `project`, `sprint` | string | Resolved governed identity. |
| `execution_status` | string | Lifecycle fact such as `ready`, `running`, `completed`, `failed`, or `cancelled`. |
| `verdict` | string | Evidence verdict; distinct from execution status. |
| `stale` | boolean | Whether current inputs differ from recorded evidence. |
| `assessment` | string | Deterministic combined verification assessment. |
| `artifact` | string | Contained workspace-relative Markdown artifact path. |
| `diagnostics` | array | Bounded, redacted operator-safe diagnostics. |
| `next_action` | string | Required recovery or continuation action. |

## Conformance Review

The existing `review` JSON fields and `sprint.review` operation remain unchanged; Conformance Review is the human label. Review additionally exposes the review fingerprint, coverage summaries, finding counts, runtime/model facts where known, and resumable attempt state. `pass`, `pass_with_findings`, `fail`, and `blocked` are verdicts, not process statuses.

## Read-only QA

CLI QA uses one envelope: `schema_version`, `operation: "sprint.qa"`, `status`, `result`, and optional `error`. HTTP wraps the same typed result in the standard API success/error envelope. QA fields are additive to sprint status.

The CLI envelope is a stable JSON surface. Consumers must accept additive result fields while preserving the meanings of `schema_version`, `operation`, `status`, `result`, and `error`. QA error codes use the closed `qa.*` category names below. The result includes durable run and operational attempt IDs when the command accepted runtime-backed work.

The QA result contains project/sprint identity; `phase`; `fresh` and reasons; semantic attempt, durable run, operational attempt, and fencing correlation; run lifecycle and terminal result; governed-input, implementation, review, map, and policy fingerprints; independent `conformance_review_status`, `conformance_review_verdict`, and `conformance_review_fresh`; changed/covered counts; effective limits; bounded shard summaries; outcome totals; blocker; cancellation; and next action. Shard and theory resources embed the same summary plus one focused record. Synthesis adds validated challenger inputs, deduplication groups, contradictions, interactions, blockers, outcome totals, parent-linked follow-up shards, and its next action. Challenger records reference current theories and cannot alter their outcomes.

Closed phase values are `missing`, `mapped`, `queued`, `running`, `synthesizing`, `completed`, `blocked`, `cancelled`, `interrupted`, `stale`, and `invalid`. Closed theory outcomes are `confirmed`, `refuted`, `invalid`, `inconclusive`, `blocked`, `cross_shard`, and `not_applicable`. Phase, freshness, run lifecycle, cancellation, terminal result, and Conformance Review verdict are distinct; `completed` is not a pass verdict.

Stable QA errors use `qa.unknown_schema`, `qa.invalid_state`, `qa.stale_input`, `qa.permission_denied`, `qa.budget_exhausted`, `qa.conflict`, `qa.persistence_failure`, and `qa.runtime_unavailable`. Trusted CLI and durable-operation results retain a safe cause and recovery action. HTTP query responses keep those codes but use bounded public messages: invalid governed state is `422`, ownership conflict is `409`, and persistence or runtime unavailability is `503`.

Detailed filesystem state is strict schema v1 in the local `qa-v1` ID namespace:

```text
projects/<project>/sprints/<sprint>/verification/state.json
projects/<project>/sprints/<sprint>/verification/attempts/<qa-v1-attempt-id>/map.json
projects/<project>/sprints/<sprint>/verification/attempts/<qa-v1-attempt-id>/shards/<qa-v1-shard-id>.json
projects/<project>/sprints/<sprint>/verification/attempts/<qa-v1-attempt-id>/synthesis.json
```

`state.json` contains the current phase, freshness fingerprints, attempt and artifact pointers/digests, bounded counts, run/cancellation/blocker facts, next action, and update time. Maps freeze budgets, source traces, target/Git identity, coverage, shard definitions, and policy/catalog fingerprints. Shards retain attempts, safe evidence summaries, approved-check summaries, and complete falsifiable theories. `flow-state.json` contains only the bounded `QAFlowSummary` pointer projection.

| Artifact | Required identity and lifecycle fields | Referenced or bounded content |
| --- | --- | --- |
| `state.json` | `schema_version`, project, sprint, phase, freshness, current attempt, run correlation, update time | map and synthesis paths/digests, shard counts, outcome counts, blocker, cancellation, next action |
| `map.json` | `schema_version`, `qa-v1` map ID, semantic attempt ID, project, sprint, input and policy fingerprints | effective limits and sources, target/Git identity, coverage ownership, immutable shard definitions, governed artifact references |
| shard record | `schema_version`, `qa-v1` shard ID, attempt ID, kind, phase | assigned and context paths, approved checks, bounded attempts, usage and cost when known, evidence summaries, theories, blocker |
| `synthesis.json` | `schema_version`, `qa-v1` synthesis ID, map ID | theory IDs, challenge records, deduplication, contradictions, interactions, blockers, outcome counts, parent-linked follow-up shards, next action |

Verification IDs are deterministic only inside schema v1 and the selected project and sprint. A reader must not compare them as global content IDs. Unknown `qa-v1` record fields, invalid scoped IDs, pointer escape, digest mismatch, or a schema major other than 1 makes the detailed state unreadable until explicit recovery or a compatible migration is available.

No QA state migration ships in this release. Future migrations must be ordered pure transforms with fixtures for the prior and next forms. They must preserve the current attempt, last readable records, review fingerprint, run correlation, and pointer digests or stop without replacing the prior state. Operators should keep the existing files intact, use a compatible UltraPlan binary, and run `qa recover` only after that binary recognizes the stored major version.

`flow-state.json` remains schema version 2 and now permits the optional bounded `qa` member. A pre-Sprint-36 binary uses strict unknown-field decoding, so it rejects a QA-published flow state as malformed. It does not delete or rewrite the file. Mixed-version processes must not share a workspace after QA publication. Upgrade every process that can touch the workspace before publishing QA state. This accepted compatibility limit is specific to the single-host deployment model and is frozen by compatibility tests.

Unknown major versions fail closed. Version 1 readers reject unknown fields, trailing JSON, unsafe paths or modes, invalid IDs, and digest mismatches. IDs are deterministic only in their documented schema/project/sprint scope and make no global content-identity claim. There is no automatic downgrade or inference from older records; a future migration must be explicit, fixture-tested, and preserve the last readable state.

## Smoke

Smoke additionally exposes `review_verdict`, `review_fingerprint`, diagnostic override facts, selected scope and rationale, prerequisite state, safe argv, run/evidence identity, counts, issue references, and cleanup outcome. Raw harness stdout, stderr, and provider payloads are never embedded.

## Verify

Verify projects the shared ordered transition through review and optionally smoke. It does not create a third assessment artifact. A diagnostic override or narrow run cannot improve canonical freshness, verdict, or assessment.

## Status

Status is read-only. It reports current artifact/state facts, freshness, reconciliation needs, overall assessment, diagnostics, and next action without launching a reviewer or harness.

## Compatibility and safety

- Unknown schema major versions must be rejected.
- Missing required identity or lifecycle fields must not be interpreted as pass.
- Secrets and raw provider/harness payloads are excluded.
- Cancellation and unavailable prerequisites remain distinct from pass.
- Consumers must tolerate additive fields within schema version 1.

## QA v2 evidence records

Evidence-producing attempts write private QA state schema version 2 while
retaining explicit reads of state version 1. Existing `qa-v1` attempt, map,
shard, theory, challenge, and synthesis IDs keep their meaning. New scoped
`qa-v2` IDs cover plan, evidence, patch, adjudication, root-cause group, issue,
and assessment records; none is a global content identity.

Under `verification/attempts/<attempt-id>/`, v2 adds `plans/`, `evidence/`,
`patches/`, `adjudication.json`, `issues.json`, and `assessment.json`.
`qa.md` remains the canonical bounded report. State references these records
by contained relative path and SHA-256 digest. Flow state keeps only phase,
freshness, assessment, counts, cancellation, next action, and state/report
pointers. Unknown majors, unknown fields, invalid modes, path escapes,
symlinks, ownership mismatches, or digest mismatches fail closed.

Public CLI and app JSON stays at schema version 1. Additive fields include QA
suite, assessment, evidence and rejected-evidence counts, issue and regression
candidate counts, canonical report, current failure, and the new finite limits.
Focused evidence and issue resources expose bounded summaries, never raw
stdout, stderr, environment values, model payloads, or arbitrary patch bodies.

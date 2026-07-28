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

## Review

Review additionally exposes the review fingerprint, coverage summaries, finding counts, runtime/model facts where known, and resumable attempt state. `pass`, `pass_with_findings`, `fail`, and `blocked` are verdicts, not process statuses.

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

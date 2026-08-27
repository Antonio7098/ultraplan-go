# Recovery Runbook

Runtime success is not product success. Treat a run as complete only when required artifacts exist and validation passes.

## First Checks

Run:

```bash
ultraplan health
ultraplan study <study> status
ultraplan study <study> validate
```

Use `--json` for automation:

```bash
ultraplan health --json
ultraplan study <study> status --json
ultraplan study <study> validate --json
```

## Validation Failures

If validation fails:

1. Read the failed check, observed value, path, and guidance.
2. Inspect the named artifact.
3. Repair the source artifact or rerun the affected task.
4. Run validation again.

Do not treat a runtime-completed task as complete when the expected report is missing or invalid.

## Missing Artifacts

Common missing artifacts include per-source reports, final reports, `summary.csv`, and `.ultraplan/run-state.json`.

- Missing per-source report: rerun `study <study> run <dimension> <source>` or resume with `run-loop`.
- Missing final report: rerun `study <study> synthesize <dimension>` after source reports validate.
- Missing summary: run `study <study> summary`.
- Missing run state: start `study <study> run-loop` to create durable state, or use `run-all` for a non-resumable batch.

Planning artifacts use a separate chain under `projects/<project>/sprints/<sprint>/`. Missing planning artifacts should be repaired stage by stage:

- Missing `requirements.md`: run `sprint <project> <sprint> prompt requirements` to inspect roadmap/docs context, then `sprint <project> <sprint> flow --to requirements`.
- Missing or invalid `code-context.md`: validate requirements and the configured implementation target, inspect `prompt code-context`, then run `flow --to code-context`. A successful rerun atomically replaces only `code-context.md`; runtime failure, cancellation, validation failure, or promotion/state failure preserves the last valid artifact.
- Missing `sprint-index.md`: run `sprint <project> <sprint> prompt sprint-index` to inspect context, then `sprint <project> <sprint> flow --to sprint-index`.
- Missing `technical-handbook.md`: validate `sprint-index` first, then run `flow --to technical-handbook`.
- Missing `reasoning.md`: validate area reasoning inputs if selected, then run `flow --to reasoning`.
- Missing `plan.md`: validate `reasoning`, then run `flow --to plan`.
- Missing `flow-state.json`: run `sprint <project> <sprint> status` to refresh artifact state.

If a downstream stage reports `invalid_path`, `containment`, `file_kind`, `missing_source`, `invalid_range`, `changed_during_read`, `invalid_encoding`, or `budget_exceeded`, repair or regenerate the reference-only context pack and rerun the affected stage. UltraPlan does not silently omit or truncate selected evidence. The shared prefix is rebuilt from current files on each top-level operation; browser reconnect/restart recovers readiness and the latest durable outcome from sprint state rather than retaining prompt bytes in the web process.

If stage skills are missing or stale, preview and rematerialise them:

```bash
ultraplan skills materialise all --dry-run
ultraplan skills materialise all
```

Customized skill files are preserved unless overwrite is confirmed. Use
`--force` only when the embedded versions should replace those customizations.
After an agent writes or repairs a planning artifact, run `sprint status
--json`; status persists the derived stage state while preserving review and
smoke evidence. Do not hand-edit state JSON to mark a stage complete.

The governed sprint chain continues through execute, review, and smoke using the shared `sprint verify` transition. Verification does not authorize issue management, remediation, or agent-driven Git mutation. Configured stage publication runs only after canonical validation and persistence.

Final integration uses `sprint merge`. Inspect `.merge-state.json` and `sprint merge status` before recovery. A `conflicts` or `failed` state with the recorded `MERGE_HEAD` still active can resume with `sprint merge continue --yes`. Use `sprint merge abort --yes` only when discarding the active merge is intended. UltraPlan will not continue if the recorded source commit, target branch, merge head, or allowed conflict paths drifted.

## Verify Recovery

- Interrupted review: run `sprint <project> <sprint> status` to inspect completed coverage and retained sessions, then rerun `review`, `verify`, or `flow`. Compatible attempts resume by default.
- Intentional fresh review: use `review --restart`, `verify --restart-review`, or `flow --restart-review`. Restart cannot be combined with focused review.
- Changed review inputs or model: the saved attempt is incompatible and the next review starts fresh automatically.
- Expired running attempt: `sprint status` derives an attempt that has lacked a terminal update for more than 24 hours as timed out without mutating state; the next explicit review/smoke operation owns the durable transition.
- Review failure: resolve findings and rerun review. Use `--force-review --override-reason <text> --yes` only for diagnostic smoke; it cannot promote review or the overall assessment.
- Smoke interruption or timeout: confirm no harness process remains, inspect external run evidence, then rerun `verify --to smoke --yes` or the explicit `smoke --yes` action.
- Fresh canonical review with stale smoke: rerun the required containing smoke suite; a narrow diagnostic selection does not replace containing-suite evidence.

## Smoke Recovery

- `smoke review_gate`: regenerate a missing, malformed, or stale review. Use `--force-review` only for a current fail/blocked diagnostic run.
- `smoke protocol` or `containment`: repair the cataloged protocol-v1 manifest, executable, cwd, or evidence roots; never infer commands from README prose.
- `smoke timeout`, `cancellation`, or `cleanup`: inspect external evidence, confirm owned descendants are gone, and retry with a bounded timeout. The previous valid `smoke.md` remains current until validation marks it stale.
- `smoke evidence`: restore immutable run/issue evidence or rerun the sufficient suite. Do not copy raw evidence into the sprint.
- `reconciliation required`: `smoke.md` committed but flow state did not. Validate both files and rerun smoke to reconcile; automatic recovery is deferred.
- stale or missing evidence: `sprint status` and `validate smoke` must be treated as non-passing until a new evidence-backed run is committed.

## Stale Running Tasks

`study status` shows active, retrying, waiting, failed, cancelled, and recent tasks from persisted run state. If tasks appear stuck:

1. Check whether an UltraPlan process is still running.
2. Check lock diagnostics in `study status`.
3. Confirm runtime/provider state outside UltraPlan if a task is still active.
4. Resume with `study <study> run-loop` only after deciding the previous process is gone or safe to abandon. Continuing shared study progress is the default. Use `--reset` only when you intentionally want to archive and rebuild progress.

## Browser Operation Recovery

The browser's operation handle, event replay, confirmation, and subscriber are
bounded, ephemeral views. Product-owned workspace/run state is authoritative.

- Refresh, navigation, tab close, SSE loss, and a slow subscriber cancel only
  observation. They do not prove completion and do not cancel product work.
- On `recovery_required`, reconnect exhaustion, operation expiry, or a missing
  terminal event, follow the durable refresh link or run the equivalent CLI
  status/validation command before retrying.
- Explicit cancellation is idempotent. A late cancel cannot replace an already
  authoritative completion/failure result.
- During shutdown, new mutations receive `server_draining`; wait for the server
  to stop, inspect durable status, then restart and retry preparation.
- A `.cleanup-uncertain.json` marker means bounded shutdown could not prove
  cleanup. Restart reconciliation runs through the owning sprint/study module;
  process absence and artifact presence are evidence only, not success.
- A stale product mutation lock must be assessed through normal CLI status and
  lock guidance. Never delete it merely because the browser operation expired.

## Locks And `--force-unlock`

`run-loop` uses a per-study lock to refuse concurrent runs. Use:

```bash
ultraplan study <study> status
```

to inspect lock path, PID, command, and acquisition time.

Use `--force-unlock` only when an operator has confirmed the existing lock is stale:

```bash
ultraplan study <study> run-loop --force-unlock
```

Forcing an active lock can corrupt operator expectations, duplicate runtime work, and race report writes.

## Cancellation

On interrupt or context cancellation, the runtime boundary is asked to cancel and run state is preserved where possible. Recovery path:

1. Run `study status`.
2. Inspect cancelled or active tasks.
3. Run `study validate`.
4. Resume with `study run-loop` or rerun specific failed tasks.

## Retry And Fallback Metadata

Status output can include retry time, policy decisions, final attempt count, fallback decisions, repair metadata, cleanup metadata, usage, cost, and omitted raw payload notes. Use it to decide whether to wait for `retry_after`, fix runtime/provider config, or rerun after a provider issue clears.

Unknown usage or cost means the runtime did not provide safe metadata; it is not a validation failure by itself.

## Partial Completion

`run-all`, `run-loop`, and `code` can return partial completion when some work succeeded and some work failed or remained unresolved. Treat partial completion as release-blocking for production evidence unless the unresolved scope is explicitly documented.

## Failed Planning Stages

For a failed planning stage:

1. Run `ultraplan project <project> validate`.
2. Run `ultraplan sprint <project> <sprint> status`.
3. Validate the earliest incomplete stage with `ultraplan sprint <project> <sprint> validate <stage>`.
4. Use `prompt <stage>` to inspect the runtime input before rerunning flow.
5. Rerun `flow --to <stage>` only after the upstream artifact validates.

When using a stage skill, it performs the same checks and must ask before
filling prerequisite gaps. An explicit proposal-only or deep-dive discussion
does not advance flow state until the governed artifact is written and
validated.

Common causes are project-index references that do not resolve, sprint-index entries outside the project catalog, missing selected evidence, reasoning that does not include decisions/risks/evidence, or a plan that does not trace tasks to `reasoning.md`.

## Atomic Write Failures

UltraPlan writes durable state and generated artifacts loudly. If a write fails:

1. Preserve stderr and the failing path.
2. Check disk, permissions, parent directories, and workspace path safety.
3. Avoid manually editing run-state files unless directed by a focused remediation.
4. Re-run validation after filesystem issues are fixed.

## Read-only QA recovery

Start with `ultraplan sprint <project> <sprint> qa status --json` and inspect its phase, freshness reasons, map and implementation fingerprints, durable run correlation, blocker, cancellation, terminal result, and next action. Do not infer QA success from a run event or infer Conformance Review success from QA.

- `missing`: use `qa --dry-run`; recovery is a runtime-free no-op.
- `queued`, `running`, or `synthesizing` after process loss: run `qa recover`. It records `interrupted`, reconciles the bounded flow summary, and directs runtime work to `qa resume`.
- `stale`: inputs, implementation, Conformance Review evidence, policy, check catalog, limits, or selected shard no longer match. Run a new dry-run and start; never reuse stale outcomes as current.
- `invalid` or `unknown_schema`: stop. Keep the private state unchanged, use a compatible binary or documented migration, and do not hand-edit version or digest fields.
- `cancelled` or wall-clock exhaustion: completed promoted shards remain inspectable. Resume only if status says the semantic attempt is current.
- `cleanup_uncertain` or persistence failure: inspect the durable run and filesystem health, restore reliable local persistence, then recover. Do not claim completion.

Detailed records live under `projects/<project>/sprints/<sprint>/verification/` with mode 0700 directories and 0600 files. `state.json` points to digest-checked records beneath `attempts/<qa-v1-attempt-id>/`. A missing reference, digest mismatch, unsafe mode, symlink, path escape, malformed/trailing JSON, or unknown major version fails closed. Recovery can republish a previously validated terminal state to fix its `flow-state.json` summary; it never rebuilds missing theory evidence or adopts a worker.

Every resume accepts and claims a new durable run and fencing generation. A stale writer cannot publish. Cancellation cleanup uses a separate bounded context so the current owner can record truthful terminal state even after the work context is cancelled. If owner identity is lost, wait for run-control reconciliation and resume; never reuse a process or provider session.

Retention keeps at most eight semantic attempts by default, including the current protected attempt, and the detailed-state hard budget is 128 MiB. Normal start/recovery prunes only validated attempt directories in deterministic order. If quota is exhausted, preserve the current attempt, remove unrelated external files first, and use normal recovery; do not delete `state.json` or a referenced current directory manually.

A gated real-runtime QA run that cannot meet runtime, current evidence, local-filesystem, browser, or Git prerequisites is `blocked`. Record the missing prerequisite; fake-runtime tests do not satisfy the dogfood gate.

## Unsafe Data Handling

Do not paste provider tokens, full environment dumps, full prompts, full generated report bodies, or raw unsafe runtime payloads into issue reports or release evidence. Use redacted command summaries and artifact paths.
## Run-control recovery

Start with `ultraplan run diagnostics`, then inspect the affected run with
`ultraplan run show <run-id>` and replay retained evidence using
`ultraplan run follow <run-id> --after <sequence>`. A replay gap means earlier
events were compacted; refresh the snapshot and resume from its
`oldest_retained_sequence`. Never infer operational success from sprint/study
artifacts, a process ID, a lock, or a provider session.

Cancellation is a durable request, not proof that cleanup completed. States
such as `stalled`, `owner_unreachable`, `interrupted`, `cleanup_uncertain`, and
`persistence_degraded` intentionally preserve uncertainty. Re-run diagnostics,
confirm the exact process identity, and let reconciliation record a conservative
terminal outcome. Do not kill a PID based only on the number shown in support
evidence.

The private `.ultraplan/run-control.log` is bounded to 1 MiB and contains only
allowlisted structured correlation and decision fields. Prefer
`ultraplan run diagnostics --support-export <file>` to collect its newest safe
records with health, snapshots, omission facts, config source classes, and
reconciliation evidence. Do not substitute the log for the SQLite journal.

If a process exits after acceptance but before its first owner claim, the run
has no process identity to probe. After 45 seconds, reconciliation records an
`interrupted` terminal with `owner_never_claimed_after_grace`; it does not
invent an attempt or adopt any work that might remain outside run control.

For quota pressure, stop starting new work, free space outside the active
database, and rerun diagnostics. UltraPlan begins compaction at 80 percent,
rejects starts at the soft threshold, and reserves 16 MiB for heartbeat,
cancellation, recovery, and terminal writes. It never deletes an active
snapshot to regain space.

Schema migrations create private timestamped backups next to
`.ultraplan/run-control.db` (maximum three retained backups and 512 MiB per
backup). Restoration is an offline procedure: stop every UltraPlan process for
the workspace, keep the backup unchanged, restore the matching UltraPlan
binary, and use the tested restore path before reopening the workspace. Never
copy only the database while a WAL writer is active. An unsupported newer
schema or failed integrity check is a stop condition, not a fallback to an
empty history.

Run control is supported only for same-host processes on a local filesystem
with reliable SQLite locking/WAL semantics. Move the workspace to such a
filesystem before recovery if those guarantees are unavailable.

## QA evidence recovery

Start with `qa status`. `qa recover` is runtime-free: it can mark abandoned
active state interrupted, reconcile the bounded flow pointer, or expose stale
inputs, but it never adopts a worker, replays a missing command, repairs a
record, or treats a v1 record as current v2 evidence. Resume only when the
semantic attempt and retained identities remain current.

Protected-target drift, unapproved copy changes, permission denial, timeout,
cancellation, malformed or stale evidence, stale writer fencing, and uncertain
descendant or workspace cleanup are non-pass outcomes. Keep immutable attempt
records for diagnosis. A failed new canonical publication restores the last
complete `qa.md`, state pointer, and flow projection while the operation error
remains visible. Restore platform capability or current review and containing
smoke prerequisites before starting again; do not apply retained patches or
edit private verification JSON by hand.

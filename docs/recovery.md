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
- Missing `sprint-index.md`: run `sprint <project> <sprint> prompt sprint-index` to inspect context, then `sprint <project> <sprint> flow --to sprint-index`.
- Missing `technical-handbook.md`: validate `sprint-index` first, then run `flow --to technical-handbook`.
- Missing `reasoning.md`: validate area reasoning inputs if selected, then run `flow --to reasoning`.
- Missing `plan.md`: validate `reasoning`, then run `flow --to plan`.
- Missing `flow-state.json`: run `sprint <project> <sprint> status` to refresh artifact state.

The governed sprint chain continues through execute, review, and smoke using the shared `sprint verify` transition. Verification does not authorize issue management, remediation, or Git mutation.

## Verify Recovery

- Interrupted review: run `sprint <project> <sprint> status` to inspect completed coverage and retained sessions, then rerun `review`, `verify`, or `flow`. Compatible attempts resume by default.
- Intentional fresh review: use `review --restart`, `verify --restart-review`, or `flow --restart-review`. Restart cannot be combined with focused review.
- Changed review inputs or model: the saved attempt is incompatible and the next review starts fresh automatically.
- Expired running attempt: `sprint status` marks an attempt that has lacked a terminal update for more than 24 hours as timed out while retaining usable review checkpoints.
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

Common causes are project-index references that do not resolve, sprint-index entries outside the project catalog, missing selected evidence, reasoning that does not include decisions/risks/evidence, or a plan that does not trace tasks to `reasoning.md`.

## Atomic Write Failures

UltraPlan writes durable state and generated artifacts loudly. If a write fails:

1. Preserve stderr and the failing path.
2. Check disk, permissions, parent directories, and workspace path safety.
3. Avoid manually editing run-state files unless directed by a focused remediation.
4. Re-run validation after filesystem issues are fixed.

## Unsafe Data Handling

Do not paste provider tokens, full environment dumps, full prompts, full generated report bodies, or raw unsafe runtime payloads into issue reports or release evidence. Use redacted command summaries and artifact paths.

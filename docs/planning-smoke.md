# Gated Planning And Deep Smoke

The normal release gates are offline and do not require OpenCode, provider credentials, network access, or real subprocess smoke fixtures. This smoke is optional and gated for machines that have a real runtime environment and a prepared planning project.

## Prerequisites

- Built `ultraplan` binary or `go run ./cmd/ultraplan`.
- Initialized UltraPlan workspace with `projects/`.
- Valid `ultraplan.yml`.
- A project containing `docs/`, `roadmap.md`, and `project-index.md`.
- A sprint containing at least `requirements.md`.
- OpenCode executable available through `agentwrap.executable` if running non-dry-run flow.
- Provider/model configured through OpenCode/provider-native mechanisms if running non-dry-run flow.
- Required network access for the provider if running non-dry-run flow.
- No provider tokens or sensitive environment values captured in logs or evidence.

## Offline Planning Checks

These checks do not invoke runtime execution:

```bash
ultraplan project <project> status
ultraplan project <project> validate
ultraplan sprint <project> <sprint> status
ultraplan sprint <project> <sprint> prompt sprint-index
ultraplan sprint <project> <sprint> prompt technical-handbook
ultraplan sprint <project> <sprint> prompt area-reasoning
ultraplan sprint <project> <sprint> prompt reasoning
ultraplan sprint <project> <sprint> prompt plan
ultraplan sprint <project> <sprint> flow --to plan --dry-run
ultraplan sprint <project> <sprint> flow --to execute --dry-run
```

## Runtime Planning Smoke

Run only when the runtime prerequisites are available:

```bash
ultraplan sprint <project> <sprint> flow --to requirements
ultraplan sprint <project> <sprint> validate requirements
ultraplan sprint <project> <sprint> flow --to sprint-index
ultraplan sprint <project> <sprint> validate sprint-index
ultraplan sprint <project> <sprint> flow --to technical-handbook
ultraplan sprint <project> <sprint> validate technical-handbook
ultraplan sprint <project> <sprint> flow --to reasoning
ultraplan sprint <project> <sprint> validate reasoning
ultraplan sprint <project> <sprint> flow --to plan
ultraplan sprint <project> <sprint> validate plan
ultraplan sprint <project> <sprint> validate execute
```

Use `area-reasoning` only when the selected sprint-index includes reasoning templates that require area artifacts.

## Review-Gated Deep Smoke

The project catalog must contain one `Smoke Harnesses` row with an absolute harness root and a manifest contained by that root. The manifest is strict protocol v1 and supplies direct executable/argv forms, discovery/run commands, evidence roots, capabilities, and environment names.

```bash
ultraplan sprint <project> <sprint> review
ultraplan sprint <project> <sprint> smoke --dry-run
ultraplan sprint <project> <sprint> smoke --yes
ultraplan sprint <project> <sprint> validate smoke
ultraplan sprint <project> <sprint> status --json
```

A current `pass` or `pass_with_findings` review runs normally. A current `fail` or `blocked` review requires an explicitly confirmed `--force-review` diagnostic run; missing, malformed, or stale review evidence cannot be overridden. Use only one of `--level`, `--suite`, or `--test`. A narrow diagnostic test does not replace required containing-suite evidence unless discovery declares complete equivalence.

Cancellation terminates the owned process group, waits for cleanup, and escalates within the configured grace period. Timeout, cancellation, malformed output, missing evidence, path escape, hash mismatch, or uncertain cleanup never creates a passing summary and does not replace the last valid `smoke.md`.

Raw JSON, stdout/stderr, per-test artifacts, and issues remain in the manifest-declared harness `runs/` and `issues/` roots. The sprint owns only `smoke.md` and smoke flow state.

The real harness lane is explicit:

```bash
ULTRAPLAN_REAL_SMOKE=1 go test ./internal/sprint -run TestRealSmokeHarness -v
```

Without the gate, harness, current review, runtime, credentials, or network, record the exact skipped/blocked prerequisite; never report a pass.

## Expected Artifacts

- `projects/<project>/sprints/<sprint>/sprint-index.md`
- `projects/<project>/sprints/<sprint>/technical-handbook.md`
- optional files under `projects/<project>/sprints/<sprint>/reasoning/`
- `projects/<project>/sprints/<sprint>/reasoning.md`
- `projects/<project>/sprints/<sprint>/plan.md`
- `projects/<project>/sprints/<sprint>/flow-state.json`

Deep smoke may write only the current sprint `smoke.md`, smoke flow state, and manifest-declared external run/issue evidence. It never mutates product source/tests, governed planning inputs, harness maintenance files, or Git state.

## Skip Path

If prerequisites are unavailable, record a skip in release evidence:

```text
Gated planning runtime smoke: skipped
Reason: missing <OpenCode executable | provider credentials | network | configured workspace | prepared planning project>
Risk: real runtime planning generation was not exercised on this machine; offline tests, build gates, prompt previews, and dry-run checks still passed.
```

Do not dump full environment variables, provider tokens, full prompts, generated artifact bodies, or raw runtime payloads.

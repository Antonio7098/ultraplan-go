# Gated Planning Smoke

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
```

## Runtime Planning Smoke

Run only when the runtime prerequisites are available:

```bash
ultraplan sprint <project> <sprint> flow --to sprint-index
ultraplan sprint <project> <sprint> validate sprint-index
ultraplan sprint <project> <sprint> flow --to technical-handbook
ultraplan sprint <project> <sprint> validate technical-handbook
ultraplan sprint <project> <sprint> flow --to reasoning
ultraplan sprint <project> <sprint> validate reasoning
ultraplan sprint <project> <sprint> flow --to plan
ultraplan sprint <project> <sprint> validate plan
```

Use `area-reasoning` only when the selected sprint-index includes reasoning templates that require area artifacts.

## Expected Artifacts

- `projects/<project>/sprints/<sprint>/sprint-index.md`
- `projects/<project>/sprints/<sprint>/technical-handbook.md`
- optional files under `projects/<project>/sprints/<sprint>/reasoning/`
- `projects/<project>/sprints/<sprint>/reasoning.md`
- `projects/<project>/sprints/<sprint>/plan.md`
- `projects/<project>/sprints/<sprint>/flow-state.json`

No implementation, smoke, review, issue, or Git mutation artifacts are expected from this release.

## Skip Path

If prerequisites are unavailable, record a skip in release evidence:

```text
Gated planning runtime smoke: skipped
Reason: missing <OpenCode executable | provider credentials | network | configured workspace | prepared planning project>
Risk: real runtime planning generation was not exercised on this machine; offline tests, build gates, prompt previews, and dry-run checks still passed.
```

Do not dump full environment variables, provider tokens, full prompts, generated artifact bodies, or raw runtime payloads.

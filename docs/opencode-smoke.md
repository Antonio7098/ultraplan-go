# Gated Study OpenCode Smoke

The normal release gates are offline and do not require OpenCode, provider credentials, network access, or real subprocess smoke fixtures. This smoke is optional and gated for machines that have a real runtime environment and a prepared study. For planning flow smoke, use [planning-smoke.md](planning-smoke.md).

## Prerequisites

- Built `ultraplan` binary or `go run ./cmd/ultraplan`.
- Initialized UltraPlan workspace.
- Valid `ultraplan.yml`.
- OpenCode executable available through `agentwrap.executable`.
- Provider/model configured through OpenCode/provider-native mechanisms.
- Required network access for the provider.
- A small study with one dimension and one source.
- No provider tokens or sensitive environment values captured in logs or evidence.

## Commands

From the repository or release workspace:

```bash
ultraplan health
ultraplan study <study> prompt analysis <dimension> <source> --output smoke/analysis-prompt.txt
ultraplan study <study> run <dimension> <source>
ultraplan study <study> validate
ultraplan study <study> synthesize <dimension>
ultraplan study <study> summary
ultraplan study <study> status --json
```

For resumability smoke:

```bash
ultraplan study <study> run-loop --dimension <dimension> --source <source> --parallel 1
```

## Expected Artifacts

- per-source report under the study reports tree.
- final report for the selected dimension.
- `studies/<study>/summary.csv`.
- `studies/<study>/.ultraplan/run-state.json` when `run-loop` is used.
- validation passes after the run.

## Cleanup

Remove temporary smoke-only studies, prompt previews, or generated reports only after evidence has been recorded. Do not remove release binaries or checksum files during smoke cleanup.

## Skip Path

If prerequisites are unavailable, record a skip in `dist/smoke-evidence.md`:

```text
Gated OpenCode smoke: skipped
Reason: missing <OpenCode executable | provider credentials | network | configured workspace | prepared study>
Risk: real runtime integration was not exercised on this machine; offline fake-first tests and build gates still passed.
```

Do not dump full environment variables, provider tokens, full prompts, full generated reports, or raw runtime payloads.

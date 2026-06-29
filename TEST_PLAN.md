# UltraPlan Go - Test Plan & Manual Test Report

## Overview

This document records the current automated and manual verification state for the `ultraplan-go` CLI. Manual results are backed by the transcript in [`evidence/manual-cli-2026-05-30.md`](evidence/manual-cli-2026-05-30.md).

- **Project:** `ultraplan-go`
- **Build command:** `go build -o bin/ultraplan ./cmd/ultraplan`
- **Binary tested:** `bin/ultraplan`
- **Go version:** `go1.26.3-X:nodwarf5 linux/amd64`
- **Test date:** 2026-05-30
- **Manual workspace root:** `/tmp/ultraplan-manual-20260530`

## 1. Automated Test Suite Results

`go test ./...` passes.

| Package | Test functions | Status |
|---|---:|---|
| `internal/app` | 13 | PASS |
| `internal/platform/config` | 3 | PASS |
| `internal/platform/logging` | 1 | PASS |
| `internal/study` | 8 | PASS |
| `internal/workspace` | 3 | PASS |

Total: **28** test functions across 5 test packages.

`go vet ./...` is clean.

## 2. Manual Feature Tests

Evidence: [`evidence/manual-cli-2026-05-30.md`](evidence/manual-cli-2026-05-30.md).

### 2.1 CLI Dispatch

| Test | Input | Expected | Actual | Status |
|---|---|---|---|---|
| No args | `ultraplan` | Show help | Help displayed, exit 0 | PASS |
| `--help` flag | `ultraplan --help` | Show help | Help displayed, exit 0 | PASS |
| `-h` flag | `ultraplan -h` | Show help | Help displayed, exit 0 | PASS |
| Unknown command | `ultraplan unknown` | Error + exit 2 | `unknown command "unknown"`, exit 2 | PASS |
| Version command | `ultraplan version` | Print build metadata | Version/Commit/BuildDate/GoVersion printed, exit 0 | PASS |

### 2.2 Workspace Init

| Test | Input | Expected | Actual | Status |
|---|---|---|---|---|
| Dry run | `init-workspace --path <dir> --dry-run` | Show would-create operations | `studies/` and `ultraplan.yml` listed with `would create` prefix | PASS |
| Create | `init-workspace --path <dir>` | Create scaffold | `studies/` and `ultraplan.yml` created with `created` prefix | PASS |
| Scaffold files | `find <workspace>` | Required scaffold exists | `ultraplan.yml` and `studies/` present; prompts/templates absent until installed | PASS |
| Idempotency | `init-workspace --path <dir>` second call | No-op | `No changes needed.`, exit 0 | PASS |

Scaffold structure verified:

```text
ultraplan.yml
studies/
```

### 2.2.1 Defaults Install

| Test | Input | Expected | Actual | Status |
|---|---|---|---|---|
| Dry run | `defaults install --path <dir> --dry-run` | Show prompt/template files that would be created | Built-in prompt/template files listed with `would create` prefix | PASS |
| Install | `defaults install --path <dir>` | Materialize editable defaults | `prompts/` and `templates/` created | PASS |
| Idempotency | `defaults install --path <dir>` second call | No-op | `No changes needed.`, exit 0 | PASS |
| Customized file | Existing prompt differs from default | List customized file and ask before overwrite | Customized file preserved unless confirmation is `yes` or `--force` is used | PASS |

Installed defaults include:

```text
prompts/base.md
prompts/synthesize.md
prompts/create-sprint-index.md
prompts/create-technical-handbook.md
prompts/create-area-reasoning.md
prompts/create-sprint-reasoning.md
prompts/plan-sprint.md
templates/repo-analysis.md
templates/report.md
templates/sprint-index.md
templates/technical-handbook.md
templates/sprint-reasoning.md
templates/sprint-plan.md
```

### 2.3 Workspace Discovery

| Test | Input | Expected | Actual | Status |
|---|---|---|---|---|
| Explicit `--workspace` | `--workspace <ws1>` | Use given path | Config and health resolve to `<ws1>` | PASS |
| Env var | `ULTRAPLAN_WORKSPACE=<ws1>` | Use env path | Config resolves to `<ws1>` | PASS |
| Flag overrides env | `--workspace <ws1>` with `ULTRAPLAN_WORKSPACE=<ws2>` | Use flag path | Config resolves to `<ws1>` | PASS |
| Parent walk | Run `health` from nested child dir | Walk up to `ultraplan.yml` | Workspace resolves to parent workspace | PASS |
| No workspace | `ultraplan health` outside workspace | Error exit 4 | `workspace not found`, exit 4 | PASS |

### 2.4 Config Show

| Test | Input | Expected | Actual | Status |
|---|---|---|---|---|
| Text output | `config show` | Print effective fields | Includes scalar fields and `agentwrap.required_health` | PASS |
| JSON output | `config show --json` | JSON with config and sources | Full JSON includes `agentwrap.required_health` and `sources` map | PASS |
| Redaction, text | `ULTRAPLAN_MODEL_DEFAULT=secret_key_abc config show` | Secret value redacted | `models.default: [REDACTED]` | PASS |
| Redaction, JSON | `ULTRAPLAN_MODEL_DEFAULT=secret_key_abc config show --json` | Secret value redacted | `"default": "[REDACTED]"` | PASS |
| Missing `config` subcommand | `config` | Error exit 2 | `config requires a subcommand`, exit 2 | PASS |
| Unknown config subcommand | `config blah` | Error exit 2 | `config: unknown subcommand "blah"`, exit 2 | PASS |

### 2.5 Environment Overrides

| Test | Input | Expected | Actual | Status |
|---|---|---|---|---|
| Override single field | `ULTRAPLAN_MODEL_DEFAULT=claude-sonnet` | Value overridden | `models.default: claude-sonnet` | PASS |
| Override multiple fields | `ULTRAPLAN_LOG_LEVEL=debug ULTRAPLAN_DEFAULT_PARALLEL=5` | Both overridden | `logging.level: debug`, `execution.default_parallel: 5` | PASS |
| Health env count, none | `health` with no `ULTRAPLAN_` vars | Count 0 | `0 ULTRAPLAN_ override(s) present` | PASS |
| Health env count, one | `ULTRAPLAN_MODEL_DEFAULT=claude-sonnet health` | Count 1 | `1 ULTRAPLAN_ override(s) present` | PASS |

### 2.6 Health Checks

| Test | Input | Expected | Actual | Status |
|---|---|---|---|---|
| Valid workspace | `health` on well-formed workspace | Checks pass, runtime skipped | 5 ok + 1 skipped, exit 0 | PASS |
| Missing workspace | `health` outside workspace | Workspace error | `workspace not found`, exit 4 | PASS |
| Invalid workspace | `health` on workspace missing `ultraplan.yml` | Workspace discovery failure | `missing ultraplan.yml`, exit 4 | PASS |
| Bad config | `health` on workspace with `version: 2` | Config validation failure | `config.validation: fail`, exit 3 | PASS |
| JSON output | `health --json` | JSON with check details | Per-check JSON returned, exit 0 | PASS |

Health checks executed:

1. `workspace.discovery`
2. `workspace.structure`
3. `config.validation`
4. `filesystem.read`
5. `environment.overrides`
6. `runtime.opencode` - skipped with message `out of scope for this sprint`

### 2.7 Study Listing & Detail

| Test | Input | Expected | Actual | Status |
|---|---|---|---|---|
| Empty studies | `study list` on fresh workspace | `(none)` | `(none)` displayed | PASS |
| Populated studies | `study list` with two studies and one hidden dir | Sorted visible list | `arch-analysis`, `runtime-review`; hidden dir omitted | PASS |
| Study detail | `study arch-analysis list` | Sources + dimensions | Sources and dimensions listed, sorted | PASS |
| Study prefix ref | `study arch list` | Resolve unique prefix | Matches `arch-analysis` | PASS |
| Missing ref | `study nonexistent list` | Error + available list | `not found; available: arch-analysis, runtime-review`, exit 5 | PASS |
| Ambiguous ref | `study arch list` with `archaeology` also present | Error + candidates | `ambiguous; matches: arch-analysis, archaeology`, exit 5 | PASS |

### 2.8 Source & Dimension Discovery

| Test | Input | Expected | Actual | Status |
|---|---|---|---|---|
| Source dirs | `study arch-analysis list` with `sources/docs` and `sources/src` | Shows directory sources | `docs directory`, `src directory` | PASS |
| Dimension files | Markdown dimensions in `dimensions/` | Shows normalized number, slug, filename | `01 api 01 api.md`; `02 workspace-validation 2 workspace validation.md` | PASS |
| Hidden studies ignored | `.hidden/` under `studies/` | Not listed | Hidden study omitted from manual `study list` | PASS |

### 2.9 Config Validation

| Test | Input | Expected | Actual | Status |
|---|---|---|---|---|
| Invalid version | `version: 2` | Config validation error | `version: expected schema version 1`, exit 3 | PASS |
| Negative retries | `default_retries: -1` | Validation error | Covered by unit tests | PASS |
| Invalid log level | `level: trace` | Validation error | Covered by unit tests | PASS |
| Zero parallel | `default_parallel: 0` | Validation error | Covered by unit tests | PASS |
| Empty model | `models.default: ""` | Validation error | Covered by unit tests | PASS |
| Invalid runtime | `runtime.default: anthropic` | Validation error | Covered by unit tests | PASS |

## 3. Issues Found

### Issue 1: [LOW] Text config output can drift from config struct

- **File:** `internal/app/config_commands.go`
- **Severity:** Low
- **Evidence:** Text output is manually maintained separately from structured JSON output.
- **Description:** The text output is manually maintained separately from the config struct and JSON representation.
- **Mitigation:** A regression test now asserts `agentwrap.required_health` is present in text output. Broader field parity coverage can still be added if the config surface grows.

### Issue 2: [INFO] Runtime health check always skipped

- **File:** `internal/app/health_commands.go`
- **Severity:** Info
- **Evidence:** Manual `health` output reports `runtime.opencode: skipped - out of scope for this sprint`.
- **Description:** This appears intentional for the current implementation stage, but it means `health` does not yet verify runtime availability.
- **Suggested fix:** Track as a runtime integration roadmap item.

### Issue 3: [LOW] `execution.default_variant` accepts any non-empty value

- **File:** `internal/platform/config/config.go`
- **Severity:** Low
- **Description:** `DefaultVariant` is validated only for non-empty. Current README validation docs match that behavior, but if variants become constrained this should be tightened.
- **Suggested fix:** Add enum validation once supported variants are finalized and documented.

## 4. Coverage Gaps

| Area | Current state | Risk |
|---|---|---|
| Binary-level integration tests | Manual only | Regressions in CLI wiring, stdout/stderr, or exit codes may not be caught automatically. |
| Text `config show` coverage | Partial regression coverage | Human-readable output can still drift as new fields are added. |
| Config parser error paths | Partial unit coverage | Malformed or unsupported YAML-like input needs more explicit assertions. |
| Workspace parent walk edge cases | Basic behavior covered | Multiple ancestor workspaces and precedence edge cases could use targeted tests. |
| `internal/codeextract/` | Doc-only placeholder | No testable implementation yet. |
| `internal/platform/filesystem/` | Doc-only placeholder | No testable implementation yet. |
| `internal/platform/runtime/` | Doc-only placeholder | No testable implementation yet. |

## 5. Recommendations

1. Add binary-level integration tests for the high-value manual scenarios in this report.
2. Add broader field parity coverage for text `config show` if the config surface grows.
3. Add `execution.default_variant` enum validation when the allowed values are finalized.
4. Promote the runtime health check from skipped to active when runtime integration lands.

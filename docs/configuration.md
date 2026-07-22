# Configuration

UltraPlan loads configuration from built-in defaults, workspace `ultraplan.yml`, supported `ULTRAPLAN_` environment variables, and command flags where implemented.

## Precedence

Effective config is resolved in this order:

1. Built-in defaults.
2. Workspace `ultraplan.yml`.
3. Environment variables.
4. Command-specific flags.

`config show` reports the effective configuration. `config show --json` includes source metadata for fields after redaction.

## Workspace Config

Default `ultraplan.yml`:

```yaml
version: 1
runtime:
  default: opencode
models:
  default: provider/model
  primary: provider/model
  backup: provider/model
execution:
  default_variant: high
  default_parallel: 3
  default_timeout: 30m
  default_retries: 3
smoke:
  discovery_timeout: 30s
  run_timeout: 30m
  stdout_limit: 4194304
  stderr_limit: 1048576
  cleanup_grace: 5s
  environment:
    - PATH
    - HOME
    - TMPDIR
    - LANG
    - LC_ALL
logging:
  format: text
  level: info
agentwrap:
  executable: opencode
  required_health:
    - runtime_available
    - structured_output
    - workdir
```

Additional supported `agentwrap` fields:

```yaml
agentwrap:
  extra_args:
    - "--some-runtime-arg"
  env:
    - "KEY=value"
  stderr_limit: 16384
  required_capabilities:
    - structured_events
    - cancellation
  sandbox: workspace_write
  permission_mode: restricted
  permission_default: ask
  permission_unsupported_behavior: best_effort
```

Unsupported fields are rejected with `unknown config field`.

## Environment Overrides

Supported environment overrides:

```text
ULTRAPLAN_WORKSPACE
ULTRAPLAN_RUNTIME_DEFAULT
ULTRAPLAN_MODEL_DEFAULT
ULTRAPLAN_MODEL_PRIMARY
ULTRAPLAN_MODEL_BACKUP
ULTRAPLAN_DEFAULT_VARIANT
ULTRAPLAN_DEFAULT_PARALLEL
ULTRAPLAN_DEFAULT_TIMEOUT
ULTRAPLAN_DEFAULT_RETRIES
ULTRAPLAN_SMOKE_DISCOVERY_TIMEOUT
ULTRAPLAN_SMOKE_RUN_TIMEOUT
ULTRAPLAN_SMOKE_STDOUT_LIMIT
ULTRAPLAN_SMOKE_STDERR_LIMIT
ULTRAPLAN_SMOKE_CLEANUP_GRACE
ULTRAPLAN_LOG_FORMAT
ULTRAPLAN_LOG_LEVEL
ULTRAPLAN_AGENTWRAP_EXECUTABLE
ULTRAPLAN_AGENTWRAP_STDERR_LIMIT
ULTRAPLAN_AGENTWRAP_SANDBOX
ULTRAPLAN_AGENTWRAP_PERMISSION_MODE
```

`ULTRAPLAN_WORKSPACE` participates in workspace discovery. The other variables override matching config fields when non-empty.

## Command Flags

Implemented config-related command flags include:

- `--workspace <path>` for workspace selection.
- `--json` on JSON-capable commands, which affects output format but does not change workspace config fields.
- `--parallel <n>` on `study run-all` and `study run-loop`, which overrides configured default parallelism for that command.

## Validation

Validation rejects:

- config schema versions other than `1`.
- non-integer `version`, parallelism, retries, or stderr limit.
- runtime names other than `opencode`.
- empty required model/runtime/variant/executable fields.
- non-positive parallelism, timeout, or stderr limit.
- negative retries.
- invalid Go duration syntax such as an empty or non-positive `execution.default_timeout`.
- smoke discovery timeouts above 5 minutes, run timeouts above 24 hours, cleanup grace above 30 seconds, stdout above 64 MiB, stderr above 16 MiB, or invalid environment names.
- logging formats other than `text` or `json`.
- logging levels other than `debug`, `info`, `warn`, or `error`.
- unsupported health checks or capabilities.
- unsupported permission defaults or unsupported-behavior values.

Run-state files also carry schema versions. Commands such as `study status`, `study validate`, and `study run-loop` reject unsupported run-state schema versions instead of silently migrating them.

## Runtime And Model Mapping

UltraPlan delegates runtime execution through agentwrap and the OpenCode adapter:

- `agentwrap.executable` maps to `opencode.WithExecutable`.
- `agentwrap.extra_args` maps to OpenCode extra args.
- `agentwrap.env` maps to OpenCode environment additions.
- `models.primary` maps to the primary agentwrap provider/model request.
- `models.default` is used when primary cannot be split into provider/model.
- `models.backup` configures an agentwrap fallback target when it differs from primary.
- `execution.default_timeout` maps to the runtime request timeout.
- `execution.default_retries` configures retry policy attempts.
- `agentwrap.required_health` maps to required runtime health checks.
- `agentwrap.required_capabilities` maps to required runtime capabilities.
- `agentwrap.sandbox`, `agentwrap.permission_mode`, `agentwrap.permission_default`, and `agentwrap.permission_unsupported_behavior` map to agentwrap sandbox and permission policy fields.

UltraPlan does not own OpenCode provider credentials or provider billing. Configure those through OpenCode/provider-native mechanisms.

## Smoke Configuration

Smoke configuration is resolved after manifest defaults and before command/TUI overrides. UltraPlan passes only named environment variables; values never appear in config output, JSON, Markdown, or TUI diagnostics. The built-in platform set is `PATH`, `HOME`, `TMPDIR`, `LANG`, and `LC_ALL`. Add a manifest-declared name to `smoke.environment` only when the harness genuinely needs it. The real-harness test lane is opt-in with `ULTRAPLAN_REAL_SMOKE=1`; normal tests never launch it.

## Redaction

UltraPlan redacts sensitive-looking values in config, logs, diagnostics, status output, and health output. Do not place provider tokens in `ultraplan.yml`; prefer runtime-native environment or credential stores. Release evidence must not include provider tokens, full sensitive environment dumps, full prompts, full report bodies, or raw unsafe runtime payloads.

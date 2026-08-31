# Configuration

UltraPlan loads configuration from built-in defaults, workspace `ultraplan.yml`, supported `ULTRAPLAN_` environment variables, and command flags where implemented.

## Precedence

Effective config is resolved in this order:

1. Built-in defaults.
2. Workspace `ultraplan.yml`.
3. Environment variables.
4. Command-specific flags.

`config show` reports the effective configuration. `config show --json` includes source metadata for fields after redaction.

## Reasoning Prompt And Template Precedence

Reasoning Markdown defaults use a separate file precedence:

1. Project override under `projects/<project>/`.
2. Workspace override.
3. Embedded built-in default.

The supported project override paths are:

```text
projects/<project>/prompts/create-area-reasoning.md
projects/<project>/prompts/create-sprint-reasoning.md
projects/<project>/templates/sprint-reasoning.md
```

Overrides replace the complete Markdown file; UltraPlan does not merge sections.
An existing unreadable, empty, or non-file project override fails closed instead
of silently falling back. `ultraplan project <project> status` shows the
effective source for each reasoning default.

Materialised stage skills do not introduce another prompt precedence layer.
Their embedded canonical prompt is a portable baseline; at execution time the
skill requests the effective CLI prompt, so project and workspace overrides
above still win. Skill customization under `.agents/skills` is independent of
prompt customization and is preserved by `skills materialise` unless overwrite
is confirmed.

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
qa:
  model: provider/model
  variant: high
  mapper_model: provider/model
  mapper_variant: medium
  investigator_model: provider/model
  investigator_variant: high
  challenger_model: provider/model
  challenger_variant: high
  arbiter_model: provider/model
  arbiter_variant: high
  reconciler_model: provider/model
  reconciler_variant: high
  evaluator_model: provider/model
  evaluator_variant: high
  repair_model: provider/model
  repair_variant: high
  arbiter_max_theories: 24
  repair_assignment_mode: per_issue
  issues_per_repair_agent: 1
  repair_execution_mode: sequential
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
git:
  stage_completion: off
  remote: origin
  push_timeout: 2m
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

QA role models are independent. The role keys are `mapper`, `investigator`,
`challenger`, `arbiter`, `reconciler`, `evaluator`, and `repair`, each with a
`_model` and `_variant` field. An empty role model or variant inherits
`qa.model` or `qa.variant`; those fields retain their existing fallback chain.
The semantic mapper, investigators, per-group arbiters, issue reconciler,
failed-evidence evaluator, and repair proposal workers are model-backed.
Theory grouping before arbitration, synthesis, adjudication, approved checks,
and repair integration and verification remain product-owned deterministic
work.

The semantic mapper is required for an evidence-producing QA run. Runtime
failure or invalid mapper output stops the run. UltraPlan does not silently use
the deterministic dry-run partition as the investigation map.

`qa.arbiter_max_theories` bounds each arbiter call from 1 through 64. UltraPlan
groups theories deterministically by shared frozen context blocks, code paths,
expectation references, risk tags, and shard ownership. It sends the union of a
group's code context once, then runs an issue reconciliation agent over the
compact arbiter results. Reconciliation cannot promote an issue without
deterministically admitted failing evidence.

`qa.repair_assignment_mode` selects the repair scheduler path. `per_issue` is
the compatibility default and requires `issues_per_repair_agent: 1`. `grouped`
accepts 1 through 16. Adjudication records root-cause grouping suggestions and
creates `ceil(repair-eligible issues / issues_per_repair_agent)` worker queues.
These queues are repair assignments. A repair run handles one issue. A repair
campaign is the durable coordinator that executes all current assignments. It
creates one reusable model session per non-empty queue. With
`qa.repair_execution_mode: sequential`, proposal and integration work retain
the compatibility order. With `parallel`, queue agents generate issue-scoped
proposals concurrently in private copied workspaces, bounded by the QA
concurrency limit. Production integration and verification remain ordered and
single-writer. Each issue is refreshed before integration; a proposal with a
stale file preimage is discarded and regenerated against the current target.
No Git merge is required, and a worker never receives a multi-issue patch
packet.
Before integration, UltraPlan runs each distinct frozen external check in the
private proposal workspace. A non-pass rejects the proposal without modifying
the canonical target. Product-owned checks and containing smoke still run only
after ordered integration.
Intermediate items may record `verified_pending_campaign` and defer containing
smoke. The final item must pass the full ladder before the campaign completes.
Campaign admission requires a qualifying manual repair proof and explicit
confirmation.

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
ULTRAPLAN_GIT_STAGE_COMPLETION
ULTRAPLAN_GIT_REMOTE
ULTRAPLAN_GIT_PUSH_TIMEOUT
ULTRAPLAN_LOG_FORMAT
ULTRAPLAN_LOG_LEVEL
ULTRAPLAN_AGENTWRAP_EXECUTABLE
ULTRAPLAN_AGENTWRAP_STDERR_LIMIT
ULTRAPLAN_AGENTWRAP_SANDBOX
ULTRAPLAN_AGENTWRAP_PERMISSION_MODE
```

`ULTRAPLAN_WORKSPACE` participates in workspace discovery. The other variables override matching config fields when non-empty.

## Per-Study Execution Order

Workspace runtime defaults remain in `ultraplan.yml`. A study can independently prioritize dimensions with `studies/<study>/study.json`:

```json
{
  "version": 1,
  "dimension_order": ["04", "02-runtime"]
}
```

References use normal dimension resolution. Unknown, ambiguous, and duplicate dimensions are invalid. Listed dimensions run as ordered priority tiers, followed by all unlisted dimensions. Missing `study.json` and an empty list preserve natural execution behavior.

## Command Flags

Implemented config-related command flags include:

- `--workspace <path>` for workspace selection.
- `--json` on JSON-capable commands, which affects output format but does not change workspace config fields.
- `--parallel <n>` on `study run-all` and `study run-loop`, which overrides configured default parallelism for that command.
- `--listen <numeric-loopback:port>` and `--open-browser` on `serve`.

## Local Web Policy

`serve` resolves the normal workspace and effective runtime/smoke configuration
before listening. Its browser security and resource caps are immutable built-in
policy in this compatibility version; no workspace YAML or `ULTRAPLAN_`
environment field can weaken them. The policy is validated as a coherent set
before the listener opens.

The fixed defaults are: 5s header, 15s read, 30s write, 60s idle, and 10s
shutdown timeouts; 32 in-flight requests; 8 KiB request targets; 64 KiB command
bodies; 128-byte identifiers; 8 active operations; 128 preparations retained
for two minutes; 256 events and 256 KiB of events per operation; 16 KiB encoded
events; 256 KiB terminal results; 8 subscribers per operation and 32 concurrent
streams; 32 queued events per subscriber; 10-minute terminal retention;
15-second heartbeat; and 30-minute stream lifetime.

Adding configurable overrides is a compatibility change: it requires named
fields and environment variables, documented valid ranges, precedence/source
reporting, and fail-closed tests. See `web-compatibility-baseline.md`.

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
- Git stage-completion modes other than `off`, `commit`, or `commit-and-push`, empty or whitespace-containing remote names, and push timeouts above 30 minutes.
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

## Git stage publication

`git.stage_completion` controls publication after a study task or sprint stage has produced valid canonical output and persisted its state. `commit` creates a local commit. `commit-and-push` also pushes the current branch, using its upstream when present or `git.remote` when no upstream exists. `off` preserves the working tree without Git mutation.

UltraPlan commits only paths owned by the completed stage. It leaves unrelated staged and unstaged changes alone. Execute is different because its recorded Git worktree belongs to one sprint; UltraPlan commits the complete worktree change set there. Agents remain prohibited from running `git add`, `git commit`, or `git push` themselves.

A push failure does not reopen a completed product stage. The command returns an error and leaves the local commit in place. Rerunning the stage pushes that commit without creating a duplicate. Git commands never prompt for credentials, and `git.push_timeout` bounds each push attempt.

## Smoke Configuration

Smoke configuration is resolved after manifest defaults and before command/TUI overrides. UltraPlan passes only named environment variables; values never appear in config output, JSON, Markdown, or TUI diagnostics. The built-in platform set is limited to `TMPDIR`, `LANG`, and `LC_ALL`; `PATH` and `HOME` are not forwarded by default. Add a manifest-declared name to `smoke.environment` only when the harness genuinely needs it. The real-harness test lane is opt-in with `ULTRAPLAN_REAL_SMOKE=1`; normal tests never launch it.

## Redaction

UltraPlan redacts sensitive-looking values in config, logs, diagnostics, status output, and health output. Do not place provider tokens in `ultraplan.yml`; prefer runtime-native environment or credential stores. Release evidence must not include provider tokens, full sensitive environment dumps, full prompts, full report bodies, or raw unsafe runtime payloads.
## Run-control retention and quota

The following settings participate in the normal default/YAML/environment/CLI
effective-configuration reporting and are safe to expose through redacted
diagnostics:

| Setting | Default | Constraint |
| --- | ---: | --- |
| `run_control.full_history` | `168h` | at least `1h` |
| `run_control.tombstone_history` | `720h` | at least `24h` |
| `run_control.workspace_quota_bytes` | `536870912` | at least 64 MiB |

Environment names use the existing UltraPlan convention:
`ULTRAPLAN_RUN_CONTROL_FULL_HISTORY`,
`ULTRAPLAN_RUN_CONTROL_TOMBSTONE_HISTORY`, and
`ULTRAPLAN_RUN_CONTROL_WORKSPACE_QUOTA_BYTES`.

The 16 MiB reserved headroom, 496 MiB default soft threshold, lease/heartbeat
timings, 16 KiB event limit, 4,096-event and 16 MiB per-run limits, replay page
size, and polling intervals are fixed safety invariants rather than settings.
Invalid combinations fail configuration loading before the repository accepts
new work.

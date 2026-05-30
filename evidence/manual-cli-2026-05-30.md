# Manual CLI Evidence - 2026-05-30

Repository: /home/antonioborgerees/coding/ultraplan-go
Binary: /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan
Go: go version go1.26.3-X:nodwarf5 linux/amd64
Manual workspace root: /tmp/ultraplan-manual-20260530

```sh
$ env -u ULTRAPLAN_WORKSPACE /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan
ultraplan

Usage:
  ultraplan [--workspace <path>] [command]

Commands:
  init-workspace   Initialize an UltraPlan workspace.
  config           Inspect effective configuration.
  health           Check workspace, config, filesystem, and environment basics.
  study            Inspect studies, sources, and dimensions.
  version          Print build metadata.

Flags:
  --workspace <path>   Use a workspace path.
  -h, --help          Show help.
[exit 0]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan --help
ultraplan

Usage:
  ultraplan [--workspace <path>] [command]

Commands:
  init-workspace   Initialize an UltraPlan workspace.
  config           Inspect effective configuration.
  health           Check workspace, config, filesystem, and environment basics.
  study            Inspect studies, sources, and dimensions.
  version          Print build metadata.

Flags:
  --workspace <path>   Use a workspace path.
  -h, --help          Show help.
[exit 0]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan -h
ultraplan

Usage:
  ultraplan [--workspace <path>] [command]

Commands:
  init-workspace   Initialize an UltraPlan workspace.
  config           Inspect effective configuration.
  health           Check workspace, config, filesystem, and environment basics.
  study            Inspect studies, sources, and dimensions.
  version          Print build metadata.

Flags:
  --workspace <path>   Use a workspace path.
  -h, --help          Show help.
[exit 0]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan unknown
unknown command "unknown"

Run 'ultraplan --help' to see available commands.
[exit 2]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan version
Version: 0.0.0-local
Commit: local
BuildDate: local
GoVersion: go1.26.3-X:nodwarf5
[exit 0]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan init-workspace --path /tmp/ultraplan-manual-20260530/ws1 --dry-run
Workspace: /tmp/ultraplan-manual-20260530/ws1
would create dir prompts
would create dir templates
would create dir studies
would create file ultraplan.yml
would create file prompts/base.md
would create file prompts/synthesize.md
would create file templates/repo-analysis.md
would create file templates/report.md
[exit 0]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan init-workspace --path /tmp/ultraplan-manual-20260530/ws1
Workspace: /tmp/ultraplan-manual-20260530/ws1
created dir prompts
created dir templates
created dir studies
created file ultraplan.yml
created file prompts/base.md
created file prompts/synthesize.md
created file templates/repo-analysis.md
created file templates/report.md
[exit 0]
```

```sh
$ find /tmp/ultraplan-manual-20260530/ws1 -maxdepth 3 -type f -o -type d
/tmp/ultraplan-manual-20260530/ws1
/tmp/ultraplan-manual-20260530/ws1/ultraplan.yml
/tmp/ultraplan-manual-20260530/ws1/studies
/tmp/ultraplan-manual-20260530/ws1/templates
/tmp/ultraplan-manual-20260530/ws1/templates/report.md
/tmp/ultraplan-manual-20260530/ws1/templates/repo-analysis.md
/tmp/ultraplan-manual-20260530/ws1/prompts
/tmp/ultraplan-manual-20260530/ws1/prompts/synthesize.md
/tmp/ultraplan-manual-20260530/ws1/prompts/base.md
[exit 0]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan init-workspace --path /tmp/ultraplan-manual-20260530/ws1
Workspace: /tmp/ultraplan-manual-20260530/ws1
No changes needed.
[exit 0]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan init-workspace --path /tmp/ultraplan-manual-20260530/empty-ws
Workspace: /tmp/ultraplan-manual-20260530/empty-ws
created dir prompts
created dir templates
created dir studies
created file ultraplan.yml
created file prompts/base.md
created file prompts/synthesize.md
created file templates/repo-analysis.md
created file templates/report.md
[exit 0]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan --workspace /tmp/ultraplan-manual-20260530/empty-ws study list
Workspace: /tmp/ultraplan-manual-20260530/empty-ws
Studies:
  (none)
[exit 0]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan --workspace /tmp/ultraplan-manual-20260530/ws1 config show
Workspace: /tmp/ultraplan-manual-20260530/ws1
version: 1
runtime.default: opencode
models.default: provider/model
models.primary: provider/model
models.backup: provider/model
execution.default_variant: high
execution.default_parallel: 3
execution.default_timeout: 30m
execution.default_retries: 3
logging.format: text
logging.level: info
agentwrap.executable: opencode
agentwrap.required_health: runtime_available, structured_output, workdir
[exit 0]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan --workspace /tmp/ultraplan-manual-20260530/ws1 config show --json
{
  "command": "config show",
  "workspace": "/tmp/ultraplan-manual-20260530/ws1",
  "status": "ok",
  "result": {
    "version": 1,
    "runtime": {
      "default": "opencode"
    },
    "models": {
      "default": "provider/model",
      "primary": "provider/model",
      "backup": "provider/model"
    },
    "execution": {
      "default_variant": "high",
      "default_parallel": 3,
      "default_timeout": "30m",
      "default_retries": 3
    },
    "logging": {
      "format": "text",
      "level": "info"
    },
    "agentwrap": {
      "executable": "opencode",
      "required_health": [
        "runtime_available",
        "structured_output",
        "workdir"
      ]
    },
    "sources": {
      "agentwrap.executable": "workspace",
      "agentwrap.required_health": "default",
      "execution.default_parallel": "workspace",
      "execution.default_retries": "workspace",
      "execution.default_timeout": "workspace",
      "execution.default_variant": "workspace",
      "logging.format": "workspace",
      "logging.level": "workspace",
      "models.backup": "workspace",
      "models.default": "workspace",
      "models.primary": "workspace",
      "runtime.default": "workspace",
      "version": "workspace"
    }
  }
}
[exit 0]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE ULTRAPLAN_WORKSPACE=/tmp/ultraplan-manual-20260530/ws1 /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan config show
Workspace: /tmp/ultraplan-manual-20260530/ws1
version: 1
runtime.default: opencode
models.default: provider/model
models.primary: provider/model
models.backup: provider/model
execution.default_variant: high
execution.default_parallel: 3
execution.default_timeout: 30m
execution.default_retries: 3
logging.format: text
logging.level: info
agentwrap.executable: opencode
agentwrap.required_health: runtime_available, structured_output, workdir
[exit 0]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE ULTRAPLAN_WORKSPACE=/tmp/ultraplan-manual-20260530/ws2 /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan --workspace /tmp/ultraplan-manual-20260530/ws1 config show
Workspace: /tmp/ultraplan-manual-20260530/ws1
version: 1
runtime.default: opencode
models.default: provider/model
models.primary: provider/model
models.backup: provider/model
execution.default_variant: high
execution.default_parallel: 3
execution.default_timeout: 30m
execution.default_retries: 3
logging.format: text
logging.level: info
agentwrap.executable: opencode
agentwrap.required_health: runtime_available, structured_output, workdir
[exit 0]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan --workspace /tmp/ultraplan-manual-20260530/ws1 config
config requires a subcommand

Run 'ultraplan config show --help' for usage.
[exit 2]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan --workspace /tmp/ultraplan-manual-20260530/ws1 config blah
config: unknown subcommand "blah"
[exit 2]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE ULTRAPLAN_MODEL_DEFAULT=secret_key_abc /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan --workspace /tmp/ultraplan-manual-20260530/ws1 config show
Workspace: /tmp/ultraplan-manual-20260530/ws1
version: 1
runtime.default: opencode
models.default: [REDACTED]
models.primary: provider/model
models.backup: provider/model
execution.default_variant: high
execution.default_parallel: 3
execution.default_timeout: 30m
execution.default_retries: 3
logging.format: text
logging.level: info
agentwrap.executable: opencode
agentwrap.required_health: runtime_available, structured_output, workdir
[exit 0]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE ULTRAPLAN_MODEL_DEFAULT=secret_key_abc /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan --workspace /tmp/ultraplan-manual-20260530/ws1 config show --json
{
  "command": "config show",
  "workspace": "/tmp/ultraplan-manual-20260530/ws1",
  "status": "ok",
  "result": {
    "version": 1,
    "runtime": {
      "default": "opencode"
    },
    "models": {
      "default": "[REDACTED]",
      "primary": "provider/model",
      "backup": "provider/model"
    },
    "execution": {
      "default_variant": "high",
      "default_parallel": 3,
      "default_timeout": "30m",
      "default_retries": 3
    },
    "logging": {
      "format": "text",
      "level": "info"
    },
    "agentwrap": {
      "executable": "opencode",
      "required_health": [
        "runtime_available",
        "structured_output",
        "workdir"
      ]
    },
    "sources": {
      "agentwrap.executable": "workspace",
      "agentwrap.required_health": "default",
      "execution.default_parallel": "workspace",
      "execution.default_retries": "workspace",
      "execution.default_timeout": "workspace",
      "execution.default_variant": "workspace",
      "logging.format": "workspace",
      "logging.level": "workspace",
      "models.backup": "workspace",
      "models.default": "env",
      "models.primary": "workspace",
      "runtime.default": "workspace",
      "version": "workspace"
    }
  }
}
[exit 0]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE ULTRAPLAN_MODEL_DEFAULT=claude-sonnet /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan --workspace /tmp/ultraplan-manual-20260530/ws1 config show
Workspace: /tmp/ultraplan-manual-20260530/ws1
version: 1
runtime.default: opencode
models.default: claude-sonnet
models.primary: provider/model
models.backup: provider/model
execution.default_variant: high
execution.default_parallel: 3
execution.default_timeout: 30m
execution.default_retries: 3
logging.format: text
logging.level: info
agentwrap.executable: opencode
agentwrap.required_health: runtime_available, structured_output, workdir
[exit 0]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE ULTRAPLAN_LOG_LEVEL=debug ULTRAPLAN_DEFAULT_PARALLEL=5 /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan --workspace /tmp/ultraplan-manual-20260530/ws1 config show
Workspace: /tmp/ultraplan-manual-20260530/ws1
version: 1
runtime.default: opencode
models.default: provider/model
models.primary: provider/model
models.backup: provider/model
execution.default_variant: high
execution.default_parallel: 5
execution.default_timeout: 30m
execution.default_retries: 3
logging.format: text
logging.level: debug
agentwrap.executable: opencode
agentwrap.required_health: runtime_available, structured_output, workdir
[exit 0]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE ULTRAPLAN_MODEL_DEFAULT=claude-sonnet /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan --workspace /tmp/ultraplan-manual-20260530/ws1 health
Workspace: /tmp/ultraplan-manual-20260530/ws1
workspace.discovery: ok - /tmp/ultraplan-manual-20260530/ws1
workspace.structure: ok
config.validation: ok
filesystem.read: ok - ultraplan.yml
environment.overrides: ok - 1 ULTRAPLAN_ override(s) present
runtime.opencode: skipped - out of scope for this sprint
[exit 0]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan --workspace /tmp/ultraplan-manual-20260530/ws1 health
Workspace: /tmp/ultraplan-manual-20260530/ws1
workspace.discovery: ok - /tmp/ultraplan-manual-20260530/ws1
workspace.structure: ok
config.validation: ok
filesystem.read: ok - ultraplan.yml
environment.overrides: ok - 0 ULTRAPLAN_ override(s) present
runtime.opencode: skipped - out of scope for this sprint
[exit 0]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan --workspace /tmp/ultraplan-manual-20260530/ws1 health --json
{
  "command": "health",
  "workspace": "/tmp/ultraplan-manual-20260530/ws1",
  "status": "ok",
  "result": {
    "checks": [
      {
        "name": "workspace.discovery",
        "status": "ok",
        "message": "/tmp/ultraplan-manual-20260530/ws1"
      },
      {
        "name": "workspace.structure",
        "status": "ok"
      },
      {
        "name": "config.validation",
        "status": "ok"
      },
      {
        "name": "filesystem.read",
        "status": "ok",
        "message": "ultraplan.yml"
      },
      {
        "name": "environment.overrides",
        "status": "ok",
        "message": "0 ULTRAPLAN_ override(s) present"
      },
      {
        "name": "runtime.opencode",
        "status": "skipped",
        "message": "out of scope for this sprint"
      }
    ]
  }
}
[exit 0]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan health
workspace.discover: workspace not found: initialize one with 'ultraplan init-workspace'
[exit 4]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan --workspace /tmp/ultraplan-manual-20260530/bad-ws health
Workspace: /tmp/ultraplan-manual-20260530/bad-ws
workspace.discovery: ok - /tmp/ultraplan-manual-20260530/bad-ws
workspace.structure: fail - missing required file: prompts/base.md
config.validation: ok
filesystem.read: ok - ultraplan.yml
environment.overrides: ok - 0 ULTRAPLAN_ override(s) present
runtime.opencode: skipped - out of scope for this sprint
workspace.validate: missing required file: prompts/base.md
[exit 5]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan --workspace /tmp/ultraplan-manual-20260530/bad-config health
Workspace: /tmp/ultraplan-manual-20260530/bad-config
workspace.discovery: ok - /tmp/ultraplan-manual-20260530/bad-config
workspace.structure: ok
config.validation: fail - config.load: version: expected schema version 1
filesystem.read: ok - ultraplan.yml
environment.overrides: ok - 0 ULTRAPLAN_ override(s) present
runtime.opencode: skipped - out of scope for this sprint
config.load: version: expected schema version 1
[exit 3]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan --workspace /tmp/ultraplan-manual-20260530/ws1 study list
Workspace: /tmp/ultraplan-manual-20260530/ws1
Studies:
  arch-analysis
  runtime-review
[exit 0]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan --workspace /tmp/ultraplan-manual-20260530/ws1 study arch-analysis list
Study: arch-analysis
Sources:
  docs directory
  src directory
Dimensions:
  01 api 01 api.md
  02 workspace-validation 2 workspace validation.md
[exit 0]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan --workspace /tmp/ultraplan-manual-20260530/ws1 study arch list
Study: arch-analysis
Sources:
  docs directory
  src directory
Dimensions:
  01 api 01 api.md
  02 workspace-validation 2 workspace validation.md
[exit 0]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan --workspace /tmp/ultraplan-manual-20260530/ws1 study nonexistent list
study.resolve: study reference "nonexistent" not found; available: arch-analysis, runtime-review
[exit 5]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan --workspace /tmp/ultraplan-manual-20260530/ambiguous-ws study arch list
study.resolve: ambiguous study reference "arch"; matches: arch-analysis, archaeology
[exit 5]
```

```sh
$ env -u ULTRAPLAN_WORKSPACE /home/antonioborgerees/coding/ultraplan-go/bin/ultraplan health
Workspace: /tmp/ultraplan-manual-20260530/ws1
workspace.discovery: ok - /tmp/ultraplan-manual-20260530/ws1
workspace.structure: ok
config.validation: ok
filesystem.read: ok - ultraplan.yml
environment.overrides: ok - 0 ULTRAPLAN_ override(s) present
runtime.opencode: skipped - out of scope for this sprint
[exit 0]
```


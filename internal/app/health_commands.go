package app

import (
	"context"
	"fmt"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
	runtimepkg "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

type healthCheck struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
	Guidance string `json:"guidance,omitempty"`
}

type healthResult struct {
	SchemaVersion int           `json:"schema_version"`
	Runtime       string        `json:"runtime,omitempty"`
	Workspace     string        `json:"workspace,omitempty"`
	Config        string        `json:"config,omitempty"`
	Checks        []healthCheck `json:"checks"`
}

var runtimeHealthChecks = runRuntimeHealth

func runHealth(deps dependencies, args []string) error {
	jsonOut := false
	for _, arg := range args {
		switch arg {
		case "--help", "-h":
			_, err := deps.stdout.Write([]byte(healthHelp()))
			return err
		case "--json":
			jsonOut = true
		default:
			return classified(ExitUsage, "health: unknown argument %q", arg)
		}
	}
	var checks []healthCheck
	root, err := discoverWorkspace(deps)
	if err != nil {
		checks = append(checks, healthCheck{ID: "workspace.discovery", Name: "workspace.discovery", Status: "fail", Message: config.RedactValue("workspace.error", err.Error()), Guidance: "run from an UltraPlan workspace or pass --workspace"})
		if jsonOut {
			_ = writeJSON(deps.stdout, "health", "", "fail", healthResult{SchemaVersion: 1, Checks: checks})
		}
		return err
	}
	checks = append(checks, healthCheck{ID: "workspace.discovery", Name: "workspace.discovery", Status: "ok", Message: workspace.Rel(root.Path, root.Path)})
	validation := workspace.Validate(root.Path)
	if validation.Valid {
		checks = append(checks, healthCheck{ID: "workspace.structure", Name: "workspace.structure", Status: "ok"})
	} else {
		checks = append(checks, healthCheck{ID: "workspace.structure", Name: "workspace.structure", Status: "fail", Message: config.RedactValue("workspace.structure", validation.Issues[0]), Guidance: "run 'ultraplan init-workspace' or restore required workspace files"})
	}
	effective, cfgErr := loadEffectiveConfig(root, deps, config.CLIOverrides{JSON: jsonOut})
	if cfgErr == nil {
		checks = append(checks, healthCheck{ID: "config.validation", Name: "config.validation", Status: "ok"})
	} else {
		checks = append(checks, healthCheck{ID: "config.validation", Name: "config.validation", Status: "fail", Message: config.RedactValue("config.error", cfgErr.Error()), Guidance: "fix ultraplan.yml or CLI overrides"})
	}
	checks = append(checks, healthCheck{ID: "filesystem.read", Name: "filesystem.read", Status: "ok", Message: workspace.MarkerFile})
	checks = append(checks, healthCheck{ID: "environment.overrides", Name: "environment.overrides", Status: "ok", Message: envSummary(deps)})
	runtimeFailed := false
	if cfgErr == nil {
		runtimeChecks, err := runtimeHealthChecks(deps.ctx, root.Path, effective.Config)
		checks = append(checks, sanitizeHealthChecks(runtimeChecks)...)
		if err != nil {
			runtimeFailed = true
		}
	}
	result := healthResult{SchemaVersion: 1, Workspace: root.Path, Checks: checks}
	if cfgErr == nil {
		result.Runtime = effective.Config.Runtime.Default
		result.Config = "valid"
	} else {
		result.Config = "invalid"
	}
	status := "ok"
	if !validation.Valid || cfgErr != nil || runtimeFailed {
		status = "fail"
	}
	if jsonOut {
		if err := writeJSON(deps.stdout, "health", root.Path, status, result); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(deps.stdout, "Workspace: %s\n", root.Path)
		for _, check := range checks {
			if check.Message == "" {
				fmt.Fprintf(deps.stdout, "%s: %s\n", check.Name, check.Status)
			} else {
				fmt.Fprintf(deps.stdout, "%s: %s - %s\n", check.Name, check.Status, check.Message)
			}
		}
	}
	if cfgErr != nil {
		return cfgErr
	}
	if runtimeFailed {
		return classified(ExitRuntime, "runtime.health: one or more runtime checks failed")
	}
	if !validation.Valid {
		return classified(ExitValidation, "workspace.validate: %s", validation.Issues[0])
	}
	return nil
}

func runRuntimeHealth(ctx context.Context, workDir string, c config.Config) ([]healthCheck, error) {
	adapter, err := runtimepkg.NewOpenCode(c)
	if err != nil {
		return []healthCheck{{ID: "runtime.opencode", Name: "runtime.opencode", Status: "fail", Message: config.RedactValue("runtime.error", err.Error()), Guidance: "check runtime executable and agentwrap configuration"}}, err
	}
	req, err := runtimepkg.RequestFromConfig(c, workDir)
	if err != nil {
		return []healthCheck{{ID: "runtime.opencode", Name: "runtime.opencode", Status: "fail", Message: config.RedactValue("runtime.error", err.Error()), Guidance: "check runtime configuration"}}, err
	}
	report, err := adapter.Health(ctx, runtimepkg.HealthRequest{
		WorkDir:        workDir,
		Provider:       req.Provider,
		Model:          req.Model,
		Checks:         req.RequireHealth,
		RequiredChecks: req.RequireHealth,
		Capabilities:   req.RequireCaps,
	})
	checks := make([]healthCheck, 0, len(report.Checks)+len(report.Capabilities))
	for _, check := range report.Checks {
		id := "runtime." + check.Name
		checks = append(checks, healthCheck{ID: id, Name: id, Status: check.Status, Message: config.RedactValue(id, check.Message), Guidance: runtimeGuidance(check.Status)})
	}
	for _, cap := range report.Capabilities {
		status := "ok"
		if !cap.Supported {
			status = "fail"
		}
		id := "runtime.capability." + cap.Name
		checks = append(checks, healthCheck{ID: id, Name: id, Status: status, Message: config.RedactValue(id, cap.Message), Guidance: runtimeGuidance(status)})
	}
	if err != nil {
		if len(checks) == 0 {
			checks = append(checks, healthCheck{ID: "runtime.opencode", Name: "runtime.opencode", Status: "fail", Message: config.RedactValue("runtime.error", err.Error()), Guidance: "check runtime executable and provider configuration"})
		}
		return checks, err
	}
	return checks, nil
}

func runtimeGuidance(status string) string {
	if status == "ok" {
		return ""
	}
	return "check runtime executable, provider configuration, and required capabilities"
}

func sanitizeHealthChecks(checks []healthCheck) []healthCheck {
	out := append([]healthCheck(nil), checks...)
	for i := range out {
		if out[i].ID == "" {
			out[i].ID = out[i].Name
		}
		if out[i].Name == "" {
			out[i].Name = out[i].ID
		}
		out[i].Message = config.RedactValue(out[i].ID, out[i].Message)
		out[i].Guidance = config.RedactValue(out[i].ID+".guidance", out[i].Guidance)
		if out[i].Guidance == "" {
			out[i].Guidance = runtimeGuidance(out[i].Status)
		}
	}
	return out
}

func envSummary(deps dependencies) string {
	env := envLookup(deps.env)
	count := 0
	keys := []string{"ULTRAPLAN_WORKSPACE"}
	for _, override := range config.EnvOverrides() {
		keys = append(keys, override.Key)
	}
	for _, key := range keys {
		if env(key) != "" {
			count++
		}
	}
	return fmt.Sprintf("%d ULTRAPLAN_ override(s) present", count)
}

func healthHelp() string {
	return `ultraplan health

Usage:
  ultraplan health [--json]

Flags:
  --json      Print JSON output.
  -h, --help  Show help.
`
}

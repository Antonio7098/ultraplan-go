package app

import (
	"fmt"

	"ultraplan-go/internal/platform/config"
	"ultraplan-go/internal/workspace"
)

type healthCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type healthResult struct {
	Checks []healthCheck `json:"checks"`
}

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
		checks = append(checks, healthCheck{Name: "workspace.discovery", Status: "fail", Message: err.Error()})
		if jsonOut {
			_ = writeJSON(deps.stdout, "health", "", "fail", healthResult{Checks: checks})
		}
		return err
	}
	checks = append(checks, healthCheck{Name: "workspace.discovery", Status: "ok", Message: root.Path})
	validation := workspace.Validate(root.Path)
	if validation.Valid {
		checks = append(checks, healthCheck{Name: "workspace.structure", Status: "ok"})
	} else {
		checks = append(checks, healthCheck{Name: "workspace.structure", Status: "fail", Message: validation.Issues[0]})
	}
	effective, cfgErr := loadEffectiveConfig(root, deps, config.CLIOverrides{JSON: jsonOut})
	if cfgErr == nil {
		_ = effective
		checks = append(checks, healthCheck{Name: "config.validation", Status: "ok"})
	} else {
		checks = append(checks, healthCheck{Name: "config.validation", Status: "fail", Message: cfgErr.Error()})
	}
	checks = append(checks, healthCheck{Name: "filesystem.read", Status: "ok", Message: workspace.MarkerFile})
	checks = append(checks, healthCheck{Name: "environment.overrides", Status: "ok", Message: envSummary(deps)})
	checks = append(checks, healthCheck{Name: "runtime.opencode", Status: "skipped", Message: "out of scope for this sprint"})
	result := healthResult{Checks: checks}
	status := "ok"
	if !validation.Valid || cfgErr != nil {
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
	if !validation.Valid {
		return classified(ExitValidation, "workspace.validate: %s", validation.Issues[0])
	}
	return nil
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

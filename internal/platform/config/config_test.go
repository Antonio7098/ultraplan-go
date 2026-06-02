package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPrecedenceAndValidation(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ultraplan.yml"), []byte(`version: 1
runtime:
  default: opencode
models:
  default: workspace/default
  primary: workspace/primary
  backup: workspace/backup
execution:
  default_variant: medium
  default_parallel: 2
  default_timeout: 10m
  default_retries: 1
logging:
  format: text
  level: info
agentwrap:
  executable: opencode
  required_health:
    - runtime_available
`), 0o644); err != nil {
		t.Fatal(err)
	}
	logFormat := "json"
	effective, err := Load(LoadOptions{
		WorkspaceRoot: root,
		Env: func(key string) string {
			if key == "ULTRAPLAN_MODEL_PRIMARY" {
				return "env/primary"
			}
			return ""
		},
		CLI: CLIOverrides{LogFormat: &logFormat},
	})
	if err != nil {
		t.Fatal(err)
	}
	if effective.Config.Models.Default != "workspace/default" {
		t.Fatalf("workspace value not loaded: %+v", effective.Config.Models)
	}
	if effective.Config.Models.Primary != "env/primary" {
		t.Fatalf("env did not win: %+v", effective.Config.Models)
	}
	if effective.Config.Logging.Format != "json" {
		t.Fatalf("cli did not win: %+v", effective.Config.Logging)
	}
}

func TestRedactSensitiveValues(t *testing.T) {
	e := Effective{Config: Defaults()}
	e.Config.Models.Default = "secret/model-token"
	e.Config.Agentwrap.Env = []string{"OPENAI_API_KEY=secret"}
	redacted := Redact(e)
	if redacted.Models.Default != "[REDACTED]" {
		t.Fatalf("secret was not redacted: %q", redacted.Models.Default)
	}
	if redacted.Agentwrap.Env[0] != "[REDACTED]" {
		t.Fatalf("env secret was not redacted: %q", redacted.Agentwrap.Env[0])
	}
}

func TestLoadAgentwrapListThenScalarFields(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ultraplan.yml"), []byte(`version: 1
agentwrap:
  executable: opencode
  required_health:
    - runtime_available
    - structured_output
    - workdir
  sandbox: workspace_write
  permission_mode: restricted
  permission_default: allow
`), 0o644); err != nil {
		t.Fatal(err)
	}
	effective, err := Load(LoadOptions{WorkspaceRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if got := effective.Config.Agentwrap.RequiredHealth; len(got) != 3 || got[0] != "runtime_available" || got[2] != "workdir" {
		t.Fatalf("RequiredHealth = %+v", got)
	}
	if effective.Config.Agentwrap.Sandbox != "workspace_write" {
		t.Fatalf("Sandbox = %q", effective.Config.Agentwrap.Sandbox)
	}
	if effective.Config.Agentwrap.PermissionMode != "restricted" {
		t.Fatalf("PermissionMode = %q", effective.Config.Agentwrap.PermissionMode)
	}
	if effective.Config.Agentwrap.PermissionDefault != "allow" {
		t.Fatalf("PermissionDefault = %q", effective.Config.Agentwrap.PermissionDefault)
	}
}

func TestValidateRejectsBadConfig(t *testing.T) {
	c := Defaults()
	c.Execution.DefaultTimeout = "nope"
	if err := Validate(c); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidateRejectsRuntimeMappingValues(t *testing.T) {
	for name, mutate := range map[string]func(*Config){
		"health":     func(c *Config) { c.Agentwrap.RequiredHealth = []string{"bad"} },
		"cap":        func(c *Config) { c.Agentwrap.RequiredCapabilities = []string{"bad"} },
		"stderr":     func(c *Config) { c.Agentwrap.StderrLimit = 0 },
		"permission": func(c *Config) { c.Agentwrap.PermissionDefault = "sometimes" },
	} {
		t.Run(name, func(t *testing.T) {
			c := Defaults()
			mutate(&c)
			if err := Validate(c); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

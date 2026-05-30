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
	redacted := Redact(e)
	if redacted.Models.Default != "[REDACTED]" {
		t.Fatalf("secret was not redacted: %q", redacted.Models.Default)
	}
}

func TestValidateRejectsBadConfig(t *testing.T) {
	c := Defaults()
	c.Execution.DefaultTimeout = "nope"
	if err := Validate(c); err == nil {
		t.Fatal("expected validation error")
	}
}

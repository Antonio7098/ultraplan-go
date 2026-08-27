package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
planning:
  requirements_model: openai/gpt-5.5
  requirements_variant: high
  code_context_model: workspace/context
  code_context_variant: high
  sprint_index_model: openai/gpt-5.5
  sprint_index_variant: high
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
			if key == "ULTRAPLAN_CODE_CONTEXT_MODEL" {
				return "env/context"
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
	if effective.Config.Planning.RequirementsModel != "openai/gpt-5.5" || effective.Config.Planning.RequirementsVariant != "high" {
		t.Fatalf("requirements planning config not loaded: %+v", effective.Config.Planning)
	}
	if effective.Config.Planning.SprintIndexModel != "openai/gpt-5.5" || effective.Config.Planning.SprintIndexVariant != "high" {
		t.Fatalf("planning config not loaded: %+v", effective.Config.Planning)
	}
	if effective.Config.Planning.CodeContextModel != "env/context" || effective.Config.Planning.CodeContextVariant != "high" || effective.Sources["planning.code_context_model"] != "env" || effective.Sources["planning.code_context_variant"] != "workspace" {
		t.Fatalf("code-context planning config/source not resolved: config=%+v sources=%+v", effective.Config.Planning, effective.Sources)
	}
}

func TestRedactSensitiveValues(t *testing.T) {
	e := Effective{Config: Defaults()}
	e.Config.Models.Default = "secret/model-token"
	e.Config.Agentwrap.Env = []string{"OPENAI_API_KEY=secret"}
	e.Config.Planning.CodeContextModel = "secret/context-token"
	redacted := Redact(e)
	if redacted.Models.Default != "[REDACTED]" {
		t.Fatalf("secret was not redacted: %q", redacted.Models.Default)
	}
	if redacted.Agentwrap.Env[0] != "[REDACTED]" {
		t.Fatalf("env secret was not redacted: %q", redacted.Agentwrap.Env[0])
	}
	if redacted.Planning.CodeContextModel != "[REDACTED]" {
		t.Fatalf("code-context model was not redacted: %q", redacted.Planning.CodeContextModel)
	}
	if redacted.Git != e.Config.Git {
		t.Fatalf("git config missing from redacted projection: got %+v want %+v", redacted.Git, e.Config.Git)
	}
	if got := RedactValue("lock.command", "ultraplan study demo run-loop --api-key=secret-value"); got != "[REDACTED]" {
		t.Fatalf("dash-form api key was not redacted: %q", got)
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

func TestSmokeConfigBoundsAndEnvironment(t *testing.T) {
	c := Defaults()
	c.Smoke.RunTimeout = "25h"
	if err := Validate(c); err == nil {
		t.Fatal("expected bounded run timeout error")
	}
	c = Defaults()
	c.Smoke.Environment = []string{"PATH", "bad-name"}
	if err := Validate(c); err == nil {
		t.Fatal("expected environment-name error")
	}
}

func TestGitPublicationConfigDefaultsPrecedenceAndBounds(t *testing.T) {
	defaults := Defaults().Git
	if defaults.StageCompletion != "off" || defaults.Remote != "origin" || defaults.PushTimeout != "2m" {
		t.Fatalf("git defaults = %+v", defaults)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ultraplan.yml"), []byte(`version: 1
git:
  stage_completion: commit
  remote: upstream
  push_timeout: 3m
`), 0o600); err != nil {
		t.Fatal(err)
	}
	effective, err := Load(LoadOptions{WorkspaceRoot: root, Env: func(key string) string {
		if key == "ULTRAPLAN_GIT_STAGE_COMPLETION" {
			return "commit-and-push"
		}
		return ""
	}})
	if err != nil {
		t.Fatal(err)
	}
	if effective.Config.Git.StageCompletion != "commit-and-push" || effective.Config.Git.Remote != "upstream" || effective.Config.Git.PushTimeout != "3m" {
		t.Fatalf("git effective config = %+v", effective.Config.Git)
	}
	if effective.Sources["git.stage_completion"] != "env" || effective.Sources["git.remote"] != "workspace" {
		t.Fatalf("git config sources = %+v", effective.Sources)
	}

	for name, mutate := range map[string]func(*Config){
		"mode":    func(c *Config) { c.Git.StageCompletion = "always" },
		"remote":  func(c *Config) { c.Git.Remote = "bad remote" },
		"timeout": func(c *Config) { c.Git.PushTimeout = "31m" },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := Defaults()
			mutate(&cfg)
			if err := Validate(cfg); err == nil {
				t.Fatal("expected git config validation error")
			}
		})
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

func TestRunControlConfigDefaultsPrecedenceAndBounds(t *testing.T) {
	defaults := Defaults().RunControl
	if defaults.FullHistory != "168h" || defaults.TombstoneHistory != "720h" || defaults.WorkspaceQuota != 512<<20 {
		t.Fatalf("run-control defaults = %+v", defaults)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ultraplan.yml"), []byte(`version: 1
run_control:
  full_history: 48h
  tombstone_history: 240h
  workspace_quota_bytes: 268435456
`), 0o600); err != nil {
		t.Fatal(err)
	}
	effective, err := Load(LoadOptions{WorkspaceRoot: root, Env: func(key string) string {
		if key == "ULTRAPLAN_RUN_CONTROL_FULL_HISTORY" {
			return "72h"
		}
		return ""
	}})
	if err != nil {
		t.Fatal(err)
	}
	if effective.Config.RunControl.FullHistory != "72h" || effective.Config.RunControl.TombstoneHistory != "240h" || effective.Config.RunControl.WorkspaceQuota != 256<<20 {
		t.Fatalf("run-control effective config = %+v", effective.Config.RunControl)
	}
	if effective.Sources["run_control.full_history"] != "env" || effective.Sources["run_control.tombstone_history"] != "workspace" {
		t.Fatalf("run-control sources = %+v", effective.Sources)
	}

	for name, mutate := range map[string]func(*Config){
		"short full history": func(c *Config) { c.RunControl.FullHistory = (time.Minute * 59).String() },
		"short tombstone":    func(c *Config) { c.RunControl.TombstoneHistory = "23h" },
		"reversed retention": func(c *Config) { c.RunControl.FullHistory, c.RunControl.TombstoneHistory = "48h", "24h" },
		"small quota":        func(c *Config) { c.RunControl.WorkspaceQuota = (64 << 20) - 1 },
	} {
		t.Run(name, func(t *testing.T) {
			config := Defaults()
			mutate(&config)
			if err := Validate(config); err == nil {
				t.Fatal("expected run-control validation error")
			}
		})
	}
}

func TestQAConfigFieldsHaveEffectiveSourcesAndLowerOnlyBounds(t *testing.T) {
	effective, err := Load(LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(QAConfigFields()) != 52 {
		t.Fatalf("QA field count = %d", len(QAConfigFields()))
	}
	for _, field := range QAConfigFields() {
		if effective.Sources[field] != "default" {
			t.Fatalf("source for %s = %q", field, effective.Sources[field])
		}
	}

	for name, mutate := range map[string]func(*QA){
		"changed paths":           func(q *QA) { q.ChangedPaths = 513 },
		"primary shards":          func(q *QA) { q.PrimaryShards = 33 },
		"boundary shards":         func(q *QA) { q.BoundaryShards = 9 },
		"follow-up shards":        func(q *QA) { q.FollowUpShards = 5 },
		"total shards":            func(q *QA) { q.TotalShards = 45 },
		"pending entries":         func(q *QA) { q.PendingEntries = 45 },
		"changed paths per shard": func(q *QA) { q.ChangedPathsPerShard = 65 },
		"context paths per shard": func(q *QA) { q.ContextPathsPerShard = 129 },
		"context expansions":      func(q *QA) { q.ContextExpansions = 5 },
		"paths per expansion":     func(q *QA) { q.PathsPerExpansion = 33 },
		"behavioral concerns":     func(q *QA) { q.BehavioralConcernsPerShard = 25 },
		"theories":                func(q *QA) { q.TheoriesPerShard = 25 },
		"iterations":              func(q *QA) { q.IterationsPerAttempt = 9 },
		"commands":                func(q *QA) { q.CommandsPerAttempt = 17 },
		"repairs":                 func(q *QA) { q.OutputRepairAttempts = 3 },
		"concurrency":             func(q *QA) { q.ConcurrentInvestigators = 9 },
		"command timeout":         func(q *QA) { q.CommandTimeout = "11m" },
		"shard timeout":           func(q *QA) { q.ShardTimeout = "31m" },
		"run timeout":             func(q *QA) { q.RunTimeout = "91m" },
		"cleanup timeout":         func(q *QA) { q.CleanupTimeout = "31s" },
		"command output":          func(q *QA) { q.CommandOutputBytes = (512 << 10) + 1 },
		"shard output":            func(q *QA) { q.ShardOutputBytes = (2 << 20) + 1 },
		"tree files":              func(q *QA) { q.TreeFiles = 400_001 },
		"tree bytes":              func(q *QA) { q.TreeBytes = (4 << 30) + 1 },
		"file bytes":              func(q *QA) { q.FileBytes = (64 << 20) + 1 },
		"generated checks":        func(q *QA) { q.GeneratedChecks = 129 },
		"generated patch":         func(q *QA) { q.GeneratedPatchBytes = (4 << 20) + 1 },
		"evidence records":        func(q *QA) { q.EvidenceRecords = 513 },
		"issues":                  func(q *QA) { q.Issues = 201 },
		"prompt":                  func(q *QA) { q.PromptBytes = (1 << 20) + 1 },
		"progress":                func(q *QA) { q.RecentProgress = 201 },
		"retention":               func(q *QA) { q.RetainedAttempts = 9 },
		"state":                   func(q *QA) { q.StateBytes = (128 << 20) + 1 },
	} {
		t.Run(name, func(t *testing.T) {
			config := Defaults()
			mutate(&config.QA)
			if err := Validate(config); err == nil {
				t.Fatal("expected QA maximum validation error")
			}
		})
	}
	config := Defaults()
	config.QA.CommandsPerAttempt = 0
	if err := Validate(config); err == nil {
		t.Fatal("zero QA limit accepted")
	}
	config = Defaults()
	config.QA.CommandsPerAttempt = -1
	if err := Validate(config); err == nil {
		t.Fatal("negative QA limit accepted")
	}
}

func TestRepairBudgetConfigUsesWorkspaceThenEnvironmentAndRejectsRaises(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ultraplan.yml"), []byte("version: 1\nqa:\n  repair:\n    max_files_per_run: 12\n    wall_time: 30m\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	effective, err := Load(LoadOptions{WorkspaceRoot: root, Env: func(key string) string {
		if key == "ULTRAPLAN_QA_REPAIR_MAX_FILES_PER_RUN" {
			return "10"
		}
		return ""
	}})
	if err != nil {
		t.Fatal(err)
	}
	if effective.Config.QA.Repair.MaxFilesPerRun != 10 || effective.Sources["qa.repair.max_files_per_run"] != "env" || effective.Config.QA.Repair.WallTime != "30m" || effective.Sources["qa.repair.wall_time"] != "workspace" {
		t.Fatalf("repair config/source = %+v %+v", effective.Config.QA.Repair, effective.Sources)
	}
	_, err = Load(LoadOptions{Env: func(key string) string {
		if key == "ULTRAPLAN_QA_REPAIR_MAX_CYCLES" {
			return "6"
		}
		return ""
	}})
	if err == nil || !strings.Contains(err.Error(), "qa.repair.max_cycles") {
		t.Fatalf("raised repair limit error = %v", err)
	}
}

func TestQAConfigRejectsMalformedEnvironmentValuesWithoutClaimingEnvSource(t *testing.T) {
	effective, err := Load(LoadOptions{Env: func(key string) string {
		if key == "ULTRAPLAN_QA_PRIMARY_SHARDS" {
			return "not-a-number"
		}
		return ""
	}})
	if err == nil || !strings.Contains(err.Error(), "ULTRAPLAN_QA_PRIMARY_SHARDS") || !strings.Contains(err.Error(), "must be an integer") {
		t.Fatalf("malformed QA environment value error = %v", err)
	}
	if effective.Sources["qa.primary_shards"] != "default" {
		t.Fatalf("failed override claimed source %q", effective.Sources["qa.primary_shards"])
	}
}

func TestQAConfigUnknownFieldIsReportedBeforeValueParsing(t *testing.T) {
	config := Defaults()
	err := setField(&config, "qa.concurrent_investigator", "high")
	if err == nil || !strings.Contains(err.Error(), "unknown config field") || strings.Contains(err.Error(), "must be an integer") {
		t.Fatalf("unknown QA field error = %v", err)
	}
}

func TestQAConfigWorkspaceAndEnvironmentPrecedence(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ultraplan.yml"), []byte("version: 1\nqa:\n  model: workspace/qa\n  concurrent_investigators: 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	effective, err := Load(LoadOptions{WorkspaceRoot: root, Env: func(key string) string {
		if key == "ULTRAPLAN_QA_CONCURRENT_INVESTIGATORS" {
			return "1"
		}
		return ""
	}})
	if err != nil {
		t.Fatal(err)
	}
	if effective.Config.QA.Model != "workspace/qa" || effective.Sources["qa.model"] != "workspace" {
		t.Fatalf("QA model/source = %q/%q", effective.Config.QA.Model, effective.Sources["qa.model"])
	}
	if effective.Config.QA.ConcurrentInvestigators != 1 || effective.Sources["qa.concurrent_investigators"] != "env" {
		t.Fatalf("QA concurrency/source = %d/%q", effective.Config.QA.ConcurrentInvestigators, effective.Sources["qa.concurrent_investigators"])
	}
}

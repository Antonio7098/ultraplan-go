package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunHelp(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "no args", args: nil},
		{name: "long help", args: []string{"--help"}},
		{name: "short help", args: []string{"-h"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, status := runForTest(tc.args)

			if status != ExitOK {
				t.Fatalf("status = %d, want %d", status, ExitOK)
			}
			if stderr != "" {
				t.Fatalf("stderr = %q, want empty", stderr)
			}
			assertContains(t, stdout, "ultraplan")
			assertContains(t, stdout, "Usage:")
			assertContains(t, stdout, "init-workspace")
			assertContains(t, stdout, "config")
			assertContains(t, stdout, "health")
			assertContains(t, stdout, "version")

			for _, deferred := range []string{
				"runtime",
				"summary",
				"validation",
				"code",
				"target",
				"sprint",
			} {
				assertNotContains(t, stdout, deferred)
			}
		})
	}
}

func TestRunVersion(t *testing.T) {
	stdout, stderr, status := runForTest([]string{"version"})

	if status != ExitOK {
		t.Fatalf("status = %d, want %d", status, ExitOK)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	for _, field := range []string{
		"Version: 1.2.3-test",
		"Commit: abc123",
		"BuildDate: 2026-05-30",
		"GoVersion: go-test",
	} {
		assertContains(t, stdout, field)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	stdout, stderr, status := runForTest([]string{"definitely-unknown"})

	if status != ExitUsage {
		t.Fatalf("status = %d, want %d", status, ExitUsage)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	assertContains(t, stderr, `unknown command "definitely-unknown"`)
	assertContains(t, stderr, "ultraplan --help")
}

func TestClassifiedErrorPreservesCauseAndCode(t *testing.T) {
	cause := errors.New("original failure")
	err := classified(ExitWorkspace, "workspace.discover: %w", cause)

	if !errors.Is(err, cause) {
		t.Fatalf("classified error did not preserve cause")
	}
	var classifiedErr classedError
	if !errors.As(err, &classifiedErr) {
		t.Fatalf("classified error type not found")
	}
	if classifiedErr.class != ExitWorkspace {
		t.Fatalf("class = %d, want %d", classifiedErr.class, ExitWorkspace)
	}
	if classifiedErr.Code() != "validation.workspace" {
		t.Fatalf("code = %q, want validation.workspace", classifiedErr.Code())
	}
}

func TestInitWorkspaceDryRunAndCreate(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, status := runForTest([]string{"init-workspace", "--path", dir, "--dry-run"})
	if status != ExitOK {
		t.Fatalf("dry-run status = %d, stderr = %q", status, stderr)
	}
	assertContains(t, stdout, "would create file ultraplan.yml")
	if _, err := os.Stat(filepath.Join(dir, "ultraplan.yml")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote config, stat err = %v", err)
	}

	stdout, stderr, status = runForTest([]string{"init-workspace", "--path", dir})
	if status != ExitOK {
		t.Fatalf("create status = %d, stderr = %q", status, stderr)
	}
	assertContains(t, stdout, "created file ultraplan.yml")
	for _, rel := range []string{"ultraplan.yml", "prompts/base.md", "prompts/synthesize.md", "templates/repo-analysis.md", "templates/report.md", "studies"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected %s: %v", rel, err)
		}
	}
}

func TestConfigShowJSONRedactsAndUsesWorkspace(t *testing.T) {
	dir := initializedWorkspace(t)
	stdout, stderr, status := runForTest([]string{"--workspace", dir, "config", "show", "--json"})
	if status != ExitOK {
		t.Fatalf("status = %d, stderr = %q", status, stderr)
	}
	assertNotContains(t, stdout, "secret")
	var payload struct {
		Command string `json:"command"`
		Status  string `json:"status"`
		Result  struct {
			Version int `json:"version"`
			Logging struct {
				Format string `json:"format"`
			} `json:"logging"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, stdout)
	}
	if payload.Command != "config show" || payload.Status != "ok" || payload.Result.Version != 1 || payload.Result.Logging.Format != "text" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestHealthValidAndInvalidWorkspace(t *testing.T) {
	dir := initializedWorkspace(t)
	stdout, stderr, status := runForTest([]string{"--workspace", dir, "health"})
	if status != ExitOK {
		t.Fatalf("status = %d, stderr = %q", status, stderr)
	}
	assertContains(t, stdout, "workspace.discovery: ok")
	assertContains(t, stdout, "runtime.opencode: skipped")

	if err := os.Remove(filepath.Join(dir, "templates", "report.md")); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, status = runForTest([]string{"--workspace", dir, "health", "--json"})
	if status != ExitValidation {
		t.Fatalf("status = %d, want %d, stdout = %q stderr = %q", status, ExitValidation, stdout, stderr)
	}
	assertContains(t, stdout, `"status": "fail"`)
	assertContains(t, stderr, "missing required file: templates/report.md")
}

func TestHealthEnvironmentSummaryCountsAllKnownOverrides(t *testing.T) {
	dir := initializedWorkspace(t)
	stdout, stderr, status := runForTestWithEnv([]string{"--workspace", dir, "health"}, map[string]string{
		"ULTRAPLAN_WORKSPACE":            dir,
		"ULTRAPLAN_RUNTIME_DEFAULT":      "opencode",
		"ULTRAPLAN_MODEL_DEFAULT":        "provider/default",
		"ULTRAPLAN_MODEL_PRIMARY":        "provider/primary",
		"ULTRAPLAN_MODEL_BACKUP":         "provider/backup",
		"ULTRAPLAN_DEFAULT_VARIANT":      "high",
		"ULTRAPLAN_DEFAULT_PARALLEL":     "4",
		"ULTRAPLAN_DEFAULT_TIMEOUT":      "45m",
		"ULTRAPLAN_DEFAULT_RETRIES":      "2",
		"ULTRAPLAN_LOG_FORMAT":           "json",
		"ULTRAPLAN_LOG_LEVEL":            "debug",
		"ULTRAPLAN_AGENTWRAP_EXECUTABLE": "opencode",
		"ULTRAPLAN_UNKNOWN_NOT_COUNTED":  "ignored",
	})
	if status != ExitOK {
		t.Fatalf("status = %d, stderr = %q", status, stderr)
	}
	assertContains(t, stdout, "environment.overrides: ok - 12 ULTRAPLAN_ override(s) present")
}

func initializedWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	_, stderr, status := runForTest([]string{"init-workspace", "--path", dir})
	if status != ExitOK {
		t.Fatalf("init status = %d, stderr = %q", status, stderr)
	}
	return dir
}

func runForTest(args []string) (string, string, int) {
	return runForTestWithEnv(args, nil)
}

func runForTestWithEnv(args []string, env map[string]string) (string, string, int) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	status := Run(Config{
		Args:   args,
		Stdout: &stdout,
		Stderr: &stderr,
		Version: Version{
			Version:   "1.2.3-test",
			Commit:    "abc123",
			BuildDate: "2026-05-30",
			GoVersion: "go-test",
		},
		Env: env,
	})

	return stdout.String(), stderr.String(), status
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected %q to contain %q", got, want)
	}
}

func assertNotContains(t *testing.T, got, unwanted string) {
	t.Helper()
	if strings.Contains(got, unwanted) {
		t.Fatalf("expected %q not to contain %q", got, unwanted)
	}
}

package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ultraplan-go/internal/platform/config"
)

func TestHealthValidAndInvalidWorkspace(t *testing.T) {
	restore := stubRuntimeHealth(t, []healthCheck{{Name: "runtime.runtime_available", Status: "ok", Message: "fake runtime ready"}}, nil)
	defer restore()
	dir := initializedWorkspace(t)
	stdout, stderr, status := runForTest([]string{"--workspace", dir, "health"})
	if status != ExitOK {
		t.Fatalf("status = %d, stderr = %q", status, stderr)
	}
	assertContains(t, stdout, "workspace.discovery: ok")
	assertContains(t, stdout, "runtime.runtime_available: ok - fake runtime ready")

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
	restore := stubRuntimeHealth(t, []healthCheck{{Name: "runtime.runtime_available", Status: "ok"}}, nil)
	defer restore()
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

func TestHealthRuntimeFailureUsesRuntimeExit(t *testing.T) {
	restore := stubRuntimeHealth(t, []healthCheck{{Name: "runtime.runtime_available", Status: "fail", Message: "missing executable"}}, errors.New("missing executable"))
	defer restore()
	dir := initializedWorkspace(t)

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "health", "--json"})
	if status != ExitRuntime {
		t.Fatalf("status = %d, want %d, stdout = %q stderr = %q", status, ExitRuntime, stdout, stderr)
	}
	assertContains(t, stdout, `"status": "fail"`)
	assertContains(t, stdout, `"name": "runtime.runtime_available"`)
	assertContains(t, stderr, "runtime.health")
}

func TestHealthJSONEnvelopeAndRedaction(t *testing.T) {
	restore := stubRuntimeHealth(t, []healthCheck{{ID: "runtime.runtime_available", Name: "runtime.runtime_available", Status: "fail", Message: "Bearer sk-test-secret"}}, errors.New("Bearer sk-test-secret"))
	defer restore()
	dir := initializedWorkspace(t)

	stdout, _, status := runForTest([]string{"--workspace", dir, "health", "--json"})
	if status != ExitRuntime {
		t.Fatalf("status = %d stdout = %q", status, stdout)
	}
	var payload struct {
		SchemaVersion int    `json:"schema_version"`
		Command       string `json:"command"`
		Status        string `json:"status"`
		Result        struct {
			SchemaVersion int           `json:"schema_version"`
			Checks        []healthCheck `json:"checks"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, stdout)
	}
	if payload.SchemaVersion != 1 || payload.Command != "health" || payload.Status != "fail" || payload.Result.SchemaVersion != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	assertContains(t, stdout, `"id": "runtime.runtime_available"`)
	assertNotContains(t, stdout, "sk-test-secret")
	assertNotContains(t, stdout, "\x1b[")
}

func stubRuntimeHealth(t *testing.T, checks []healthCheck, err error) func() {
	t.Helper()
	orig := runtimeHealthChecks
	runtimeHealthChecks = func(context.Context, string, config.Config) ([]healthCheck, error) {
		return checks, err
	}
	return func() { runtimeHealthChecks = orig }
}

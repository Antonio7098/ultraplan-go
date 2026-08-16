package sprint

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pprocess "github.com/Antonio7098/ultraplan-go/internal/platform/process"
)

type smokeRecordingRunner struct {
	discovery smokeDiscovery
	run       smokeRunResponse
	malformed bool
	calls     [][]string
}

func (r *smokeRecordingRunner) Run(_ context.Context, req pprocess.Request) (pprocess.Result, error) {
	r.calls = append(r.calls, append([]string{req.Executable}, req.Args...))
	result := pprocess.Result{ExitCode: 0, CleanupComplete: true, StartedAt: time.Now(), FinishedAt: time.Now()}
	if r.malformed {
		result.Stdout = "not-json"
		return result, nil
	}
	var value any = r.run
	for _, arg := range req.Args {
		if arg == "discover" {
			value = r.discovery
		}
	}
	data, _ := json.Marshal(value)
	result.Stdout = string(data)
	return result, nil
}

func TestSmokeSelectionAndVerdicts(t *testing.T) {
	d := smokeDiscovery{SprintMappings: []smokeSprintMapping{{Sprint: "27", Suites: []string{"suite-b", "suite-a"}, Complete: true, Rationale: "complete"}}, Suites: []smokeSuite{{ID: "suite-a"}, {ID: "suite-b"}}, Tests: []smokeTest{{ID: "test-a", Suite: "suite-a"}}}
	selection, err := selectSmoke(d, "27", SmokeRequest{})
	if err != nil || strings.Join(selection.IDs, ",") != "suite-a,suite-b" || selection.Kind != "suite" {
		t.Fatalf("selection=%+v err=%v", selection, err)
	}
	d.SprintMappings[0].NotApplicable = true
	selection, _ = selectSmoke(d, "27", SmokeRequest{})
	if selection.Verdict != SmokeNotApplicable {
		t.Fatalf("selection=%+v", selection)
	}
	d.SprintMappings[0].NotApplicable = false
	d.Tests[0].EquivalentComplete = true
	selection, _ = selectSmoke(d, "27", SmokeRequest{Test: "test-a"})
	if !selection.DiagnosticOnly {
		t.Fatalf("test selection must be diagnostic: %+v", selection)
	}
	if got := synthesizeSmokeVerdict(SmokeCounts{Total: 1, Passed: 1}, nil); got != SmokePass {
		t.Fatalf("verdict=%s", got)
	}
	if got := synthesizeSmokeVerdict(SmokeCounts{Total: 1, Passed: 1}, []SmokeIssue{{Status: "open"}}); got != SmokePassWithOpenIssues {
		t.Fatalf("verdict=%s", got)
	}
	if got := synthesizeSmokeVerdict(SmokeCounts{Total: 1, Failed: 1}, nil); got != SmokeFailVerdict {
		t.Fatalf("verdict=%s", got)
	}
}

func TestDefaultSmokeEnvironmentPreservesInterpreterPath(t *testing.T) {
	settings := DefaultSmokeSettings()
	values := map[string]string{
		"PATH":              "/opt/node/bin:/usr/bin",
		"HOME":              "/home/smoke",
		"TMPDIR":            "/tmp",
		"UNDECLARED_SECRET": "must-not-pass",
	}
	env := smokeEnvironment(settings, smokeManifest{}, func(name string) string { return values[name] })
	if !contains(env, "PATH=/opt/node/bin:/usr/bin") {
		t.Fatalf("environment=%v, want PATH for contained script interpreters", env)
	}
	if !contains(env, "HOME=/home/smoke") {
		t.Fatalf("environment=%v, want HOME for bounded tool caches and configuration", env)
	}
	for _, value := range env {
		if strings.HasPrefix(value, "UNDECLARED_SECRET=") {
			t.Fatalf("environment leaked a non-allowlisted value: %v", env)
		}
	}
}

func TestSmokeExplicitScopeMustCoverCompleteMapping(t *testing.T) {
	d := smokeDiscovery{
		Levels:         []smokeLevel{{ID: "all", Suites: []string{"suite-a", "suite-b"}}, {ID: "partial", Suites: []string{"suite-a"}}},
		Suites:         []smokeSuite{{ID: "suite-a"}, {ID: "suite-b"}},
		SprintMappings: []smokeSprintMapping{{Sprint: "27", Suites: []string{"suite-a", "suite-b"}, Complete: true}},
	}
	selection, err := selectSmoke(d, "27", SmokeRequest{Suite: "suite-a"})
	if err != nil || !selection.DiagnosticOnly {
		t.Fatalf("single suite must remain diagnostic: selection=%+v err=%v", selection, err)
	}
	selection, err = selectSmoke(d, "27", SmokeRequest{Level: "partial"})
	if err != nil || !selection.DiagnosticOnly {
		t.Fatalf("partial level must remain diagnostic: selection=%+v err=%v", selection, err)
	}
	selection, err = selectSmoke(d, "27", SmokeRequest{Level: "all"})
	if err != nil || selection.DiagnosticOnly {
		t.Fatalf("containing level should be sufficient: selection=%+v err=%v", selection, err)
	}
}

func TestSmokeDiscoveryRejectsBrokenRelationships(t *testing.T) {
	m := smokeManifest{ProtocolVersion: "1.0", Harness: smokeHarnessIdentity{ID: "fake"}}
	d := smokeDiscovery{SchemaVersion: 1, ProtocolVersion: "1.0", HarnessID: "fake", EvidenceSchema: 1, Suites: []smokeSuite{{ID: "suite", Prerequisites: []string{"missing"}}}}
	if err := validateSmokeDiscovery(d, m); err == nil {
		t.Fatal("expected unknown prerequisite rejection")
	}
	d.Suites[0].Prerequisites = nil
	d.Tests = []smokeTest{{ID: "test", Suite: "missing"}}
	if err := validateSmokeDiscovery(d, m); err == nil {
		t.Fatal("expected unknown test suite rejection")
	}
}

func TestSmokeDiscoveryAllowsIdentityReuseAcrossKinds(t *testing.T) {
	m := smokeManifest{ProtocolVersion: "1.0", Harness: smokeHarnessIdentity{ID: "fake"}}
	d := smokeDiscovery{
		SchemaVersion:   1,
		ProtocolVersion: "1.0",
		HarnessID:       "fake",
		EvidenceSchema:  1,
		Levels:          []smokeLevel{{ID: "runtime", Suites: []string{"runtime"}}},
		Suites:          []smokeSuite{{ID: "runtime", Tests: []string{"runtime"}}},
		Tests:           []smokeTest{{ID: "runtime", Suite: "runtime"}},
	}
	if err := validateSmokeDiscovery(d, m); err != nil {
		t.Fatalf("cross-kind identity reuse should be valid: %v", err)
	}
	d.Suites = append(d.Suites, smokeSuite{ID: "runtime"})
	if err := validateSmokeDiscovery(d, m); err == nil {
		t.Fatal("same-kind duplicate suite identity should be rejected")
	}
}

func TestSmokeRunCommitsValidatedArtifactAndPreservesItOnMalformedRun(t *testing.T) {
	root, sp := reviewFixture(t)
	harness := t.TempDir()
	writeFileContent(t, harness, "#!/bin/sh\n", "runner")
	if err := os.Chmod(filepath.Join(harness, "runner"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(harness, "runs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(harness, "issues"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schemaVersion":1,"protocolVersion":"1.0","harness":{"id":"fake","version":"1"},"executable":"runner","cwd":".","commands":{"discover":["discover"],"run":["run"]},"evidence":{"runs":"runs","issues":"issues"},"capabilities":["discovery","run","evidence-v1","scope-mapping"],"environment":[]}`
	writeFileContent(t, harness, manifest, "manifest.json")
	projectIndexPath := filepath.Join(root, "projects", "proj", "project-index.md")
	data, _ := os.ReadFile(projectIndexPath)
	data = append(data, []byte("\n## Smoke Harnesses\n\n| Harness | Path | Manifest | Evidence | Useful For | Status |\n|---|---|---|---|---|---|\n| fake | "+harness+" | "+filepath.Join(harness, "manifest.json")+" | runs/ and issues/ | tests | current |\n")...)
	if err := os.WriteFile(projectIndexPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	reviewer := &reviewRuntime{}
	service := NewService(root).WithRuntime(reviewer).WithStageRuntime(map[PlanningStage]StageRuntime{StageReview: {Model: "test/model"}})
	if _, err := service.Review(context.Background(), "proj", "01", ReviewRequest{}); err != nil {
		t.Fatal(err)
	}
	runID := "run-1"
	runJSON := filepath.Join(harness, "runs", runID+".json")
	summary := filepath.Join(harness, "runs", runID+"-summary.md")
	if err := os.WriteFile(runJSON, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(summary, []byte("# summary\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &smokeRecordingRunner{
		discovery: smokeDiscovery{SchemaVersion: 1, ProtocolVersion: "1.0", HarnessID: "fake", EvidenceSchema: 1, Suites: []smokeSuite{{ID: "sprint"}}, SprintMappings: []smokeSprintMapping{{Sprint: sp.Slug, Suites: []string{"sprint"}, Complete: true, Rationale: "dedicated"}}},
		run:       smokeRunResponse{SchemaVersion: 1, ProtocolVersion: "1.0", HarnessID: "fake", RunID: runID, ScopeKind: "suite", Scope: []string{"sprint"}, Counts: SmokeCountsWire{Total: 1, Passed: 1}, DurationMs: 10, Evidence: []smokeEvidenceWire{{Kind: "run", Path: "runs/" + runID + ".json"}, {Kind: "summary", Path: "runs/" + runID + "-summary.md"}}},
	}
	service = NewService(root).WithProcessRunner(runner).WithSmokeSettings(DefaultSmokeSettings()).WithStageRuntime(map[PlanningStage]StageRuntime{StageReview: {Model: "test/model"}})
	result, err := service.RunSmoke(context.Background(), "proj", "01", SmokeRequest{})
	if err != nil || result.Verdict != SmokePass || len(runner.calls) != 2 {
		t.Fatalf("result=%+v calls=%v err=%v", result, runner.calls, err)
	}
	artifact := filepath.Join(sp.Path, "smoke.md")
	prior, err := os.ReadFile(artifact)
	if err != nil || len(ValidateSmokeContent(string(prior))) != 0 {
		t.Fatalf("artifact err=%v content=%s", err, prior)
	}
	status, statusErr := service.VerificationStatus("proj", "01")
	if statusErr != nil || !status.Smoke.Fresh {
		t.Fatalf("fresh smoke status=%+v err=%v", status, statusErr)
	}
	originalRun, _ := os.ReadFile(runJSON)
	if err := os.WriteFile(runJSON, []byte("externally edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, statusErr = service.VerificationStatus("proj", "01")
	if statusErr != nil || status.Smoke.Fresh {
		t.Fatalf("external evidence edit did not stale smoke: %+v err=%v", status, statusErr)
	}
	if err := os.WriteFile(runJSON, originalRun, 0o644); err != nil {
		t.Fatal(err)
	}
	runner.malformed = true
	if _, err := service.RunSmoke(context.Background(), "proj", "01", SmokeRequest{}); err == nil {
		t.Fatal("expected malformed discovery failure")
	}
	after, _ := os.ReadFile(artifact)
	if string(after) != string(prior) {
		t.Fatal("malformed run replaced valid smoke.md")
	}
	state, stateErr := LoadFlowState(root, sp)
	if stateErr != nil || state.Smoke.LastComplete == nil || state.Smoke.LastComplete.Verdict != SmokePass || state.Smoke.LastAttempt == nil || state.Smoke.LastAttempt.Status != AttemptFailed {
		t.Fatalf("failed attempt did not preserve last complete smoke: state=%+v err=%v", state.Smoke, stateErr)
	}
}

func TestSmokeManifestRejectsUnsupportedAndUnsafeValues(t *testing.T) {
	m := smokeManifest{SchemaVersion: 1, ProtocolVersion: "2.0", Harness: smokeHarnessIdentity{ID: "h", Version: "1"}, Executable: "run", CWD: ".", Commands: smokeCommands{Discover: []string{"d"}, Run: []string{"r"}}, Evidence: smokeEvidenceRoots{Runs: "runs", Issues: "issues"}, Capabilities: []string{"discovery", "run", "evidence-v1", "scope-mapping"}}
	if err := validateSmokeManifest(m); err == nil {
		t.Fatal("expected unsupported protocol")
	}
	m.ProtocolVersion = "1.0"
	m.Environment = []string{"TOKEN", "TOKEN"}
	if err := validateSmokeManifest(m); err == nil {
		t.Fatal("expected duplicate environment rejection")
	}
	m.Environment = nil
	for _, timeout := range []string{"invalid", "0s", "-1s", "25h"} {
		m.Defaults.Timeout = timeout
		if err := validateSmokeManifest(m); err == nil {
			t.Fatalf("expected timeout %q rejection", timeout)
		}
	}
	m.Defaults.Timeout = "2m"
	if err := validateSmokeManifest(m); err != nil {
		t.Fatalf("valid timeout rejected: %v", err)
	}
	argv := safeArgv("/opt/harness", []string{"run", "--authorization", "Bearer top-secret", "--credential=value", "--scope", "suite"})
	if strings.Contains(argv, "top-secret") || strings.Contains(argv, "value") || strings.Contains(argv, "suite") || !strings.Contains(argv, "--authorization") {
		t.Fatalf("unsafe stable argv: %s", argv)
	}
}

func TestRealSmokeHarness(t *testing.T) {
	if os.Getenv("ULTRAPLAN_REAL_SMOKE") != "1" {
		t.Skip("blocked: set ULTRAPLAN_REAL_SMOKE=1 to opt into the cataloged external harness")
	}
	workspaceRoot := os.Getenv("ULTRAPLAN_REAL_SMOKE_WORKSPACE")
	if workspaceRoot == "" {
		workspaceRoot = "/home/antonioborgerees/coding/ultraplan-go-workspace"
	}
	projectRef := os.Getenv("ULTRAPLAN_REAL_SMOKE_PROJECT")
	if projectRef == "" {
		projectRef = "ultraplan-go"
	}
	sprintRef := os.Getenv("ULTRAPLAN_REAL_SMOKE_SPRINT")
	if sprintRef == "" {
		sprintRef = "27-deep-smoke"
	}
	result, err := NewService(workspaceRoot).RunSmoke(context.Background(), projectRef, sprintRef, SmokeRequest{})
	if err != nil {
		t.Skipf("blocked real harness prerequisite/gate: %v", err)
	}
	if result.Verdict != SmokePass && result.Verdict != SmokePassWithOpenIssues && result.Verdict != SmokeFailVerdict {
		t.Fatalf("real harness returned non-evidence verdict: %+v", result)
	}
	t.Logf("protocol=%s run=%s verdict=%s evidence=%d", result.Protocol, result.RunID, result.Verdict, len(result.Evidence))
}

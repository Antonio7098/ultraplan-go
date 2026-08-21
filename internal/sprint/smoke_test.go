package sprint

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pprocess "github.com/Antonio7098/ultraplan-go/internal/platform/process"
	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

type smokeAuthorRuntime struct {
	requests    []pruntime.Request
	permissions pruntime.PermissionSummary
}

func (r *smokeAuthorRuntime) StartRun(_ context.Context, req pruntime.Request) (pruntime.Result, error) {
	r.requests = append(r.requests, req)
	permissions := r.permissions
	if permissions.Mode == "" {
		permissions.Mode = "restricted"
	}
	return pruntime.Result{RunID: "author-1", Permissions: permissions}, nil
}

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
	d := smokeDiscovery{SprintMappings: []smokeSprintMapping{{Sprint: "27", Suites: []string{"suite-b", "suite-a"}, Complete: true, Rationale: "complete", RequiredCoverage: []string{"coverage-a"}}}, Suites: []smokeSuite{{ID: "suite-a", Tests: []string{"test-a"}}, {ID: "suite-b", Tests: []string{"test-b"}}}, Tests: []smokeTest{{ID: "test-a", Suite: "suite-a", Coverage: []string{"coverage-a"}}, {ID: "test-b", Suite: "suite-b", Coverage: []string{"coverage-a"}}}}
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

func TestRenderSmokeIncludesDetailedIssueFinding(t *testing.T) {
	content := RenderSmoke(SmokeResult{
		Status: SmokeCompleted, Verdict: SmokeFailVerdict,
		Issues: []SmokeIssue{{
			ID: "runtime-live-capacity", Status: "open", Path: "/harness/issues/runtime-live-capacity.md",
			TestID: "live-capacity", Severity: "high", Title: "capacity was not enforced",
			Summary: "the ninth start returned 202", Theory: "active operations may be counted after admission",
			Evidence: "nine operation_started events were recorded", Action: "inspect the active-count transition",
		}},
	})
	for _, want := range []string{
		"### `live-capacity` — capacity was not enforced",
		"Observed: the ninth start returned 202",
		"Working theory: active operations may be counted after admission",
		"Supporting evidence: nine operation_started events were recorded",
		"Next investigation: inspect the active-count transition",
		"`runtime-live-capacity` (high, test `live-capacity`)",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("rendered smoke missing %q:\n%s", want, content)
		}
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

func TestSmokeAuthorProtectedWriteAttribution(t *testing.T) {
	root := "/workspace/product"
	cases := []struct {
		name   string
		events []pruntime.Event
		want   bool
	}{
		{
			name: "write tool under protected root",
			events: []pruntime.Event{{Kind: "tool", Payload: map[string]any{
				"part": map[string]any{"tool": "write", "state": map[string]any{"input": map[string]any{"filePath": root + "/internal/web/server.go"}}},
			}}},
			want: true,
		},
		{
			name: "read tool under protected root",
			events: []pruntime.Event{{Kind: "tool", Payload: map[string]any{
				"part": map[string]any{"tool": "read", "state": map[string]any{"input": map[string]any{"filePath": root + "/internal/web/server.go"}}},
			}}},
		},
		{
			name: "write tool in harness",
			events: []pruntime.Event{{Kind: "tool", Payload: map[string]any{
				"part": map[string]any{"tool": "write", "state": map[string]any{"input": map[string]any{"filePath": "/workspace/harness/src/test.ts"}}},
			}}},
		},
		{
			name: "shell edit command under protected root",
			events: []pruntime.Event{{Kind: "tool", Payload: map[string]any{
				"part": map[string]any{"tool": "bash", "state": map[string]any{"input": map[string]any{"command": "sed -i s/old/new/ " + root + "/go.mod"}}},
			}}},
			want: true,
		},
		{
			name: "non-tool event cannot attribute",
			events: []pruntime.Event{{Kind: "message", Payload: map[string]any{
				"text": "edited " + root + "/go.mod",
			}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := smokeAuthorAttributedProtectedWrite(tc.events, root); got != tc.want {
				t.Fatalf("attributed=%t want=%t", got, tc.want)
			}
		})
	}
}

func TestSmokeConcurrentChangeDiagnosticIsBounded(t *testing.T) {
	paths := make([]string, 25)
	for i := range paths {
		paths[i] = fmt.Sprintf("path-%02d", i)
	}
	got := smokeConcurrentChangeDiagnostic("product target", paths)
	for _, want := range []string{"concurrent_target_change", "path-00", "(+5 more)"} {
		if !strings.Contains(got, want) {
			t.Fatalf("diagnostic %q missing %q", got, want)
		}
	}
}

func TestSmokeExplicitScopeMustCoverCompleteMapping(t *testing.T) {
	d := smokeDiscovery{
		Levels:         []smokeLevel{{ID: "all", Suites: []string{"suite-a", "suite-b"}}, {ID: "partial", Suites: []string{"suite-a"}}},
		Suites:         []smokeSuite{{ID: "suite-a", Tests: []string{"test-a"}}, {ID: "suite-b", Tests: []string{"test-b"}}},
		Tests:          []smokeTest{{ID: "test-a", Suite: "suite-a", Coverage: []string{"coverage"}}, {ID: "test-b", Suite: "suite-b", Coverage: []string{"coverage"}}},
		SprintMappings: []smokeSprintMapping{{Sprint: "27", Suites: []string{"suite-a", "suite-b"}, Complete: true, RequiredCoverage: []string{"coverage"}}},
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

func TestPopulateSmokeCoverageRequirementsUsesGovernedAcceptanceOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "requirements.md")
	content := "# Requirements\n\n## Acceptance Criteria\n\n- [ ] First governed behavior.\n- [x] Second `governed` behavior.\n\n## Constraints\n\n- not coverage\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	mapping := &SmokeCoverageMapping{RequiredCoverage: []string{"AC-01", "AC-02", "external-boundary"}, Tests: []SmokeCoverageTest{{ID: "probe-b", Coverage: []string{"AC-01"}}, {ID: "probe-a", Coverage: []string{"AC-01", "external-boundary"}}}}
	populateSmokeCoverageRequirements(mapping, path)
	if len(mapping.Requirements) != 3 || mapping.Requirements[0].Description != "First governed behavior." || mapping.Requirements[1].Description != "Second `governed` behavior." || mapping.Requirements[2].Description != "" || strings.Join(mapping.Requirements[0].MappedTests, ",") != "probe-a,probe-b" {
		t.Fatalf("unexpected governed coverage descriptions: %+v", mapping.Requirements)
	}
}

func TestLegacyMappingsDoNotBlockAuthoredSprintButCannotPass(t *testing.T) {
	m := smokeManifest{ProtocolVersion: "1.0", Harness: smokeHarnessIdentity{ID: "fake"}}
	d := smokeDiscovery{
		SchemaVersion:   1,
		ProtocolVersion: "1.0",
		HarnessID:       "fake",
		EvidenceSchema:  1,
		Suites:          []smokeSuite{{ID: "legacy"}, {ID: "current", Tests: []string{"live"}}},
		Tests:           []smokeTest{{ID: "live", Suite: "current", Coverage: []string{"real-boundary"}}},
		SprintMappings: []smokeSprintMapping{
			{Sprint: "old", Suites: []string{"legacy"}, Complete: true},
			{Sprint: "new", Suites: []string{"current"}, Complete: true, RequiredCoverage: []string{"real-boundary"}},
		},
	}
	if err := validateSmokeDiscovery(d, m); err != nil {
		t.Fatalf("unrelated legacy mapping blocked authored discovery: %v", err)
	}
	selection, err := selectSmoke(d, "old", SmokeRequest{})
	if err != nil || selection.Verdict != SmokeBlockedVerdict {
		t.Fatalf("legacy mapping must not establish complete smoke: selection=%+v err=%v", selection, err)
	}
	selection, err = selectSmoke(d, "new", SmokeRequest{})
	if err != nil || selection.Verdict != "" || strings.Join(selection.IDs, ",") != "current" {
		t.Fatalf("authored mapping was not selectable: selection=%+v err=%v", selection, err)
	}
}

func TestSmokeAuthoringPathAllowlist(t *testing.T) {
	allowed := []string{"src/tests", "src/protocol.ts"}
	for _, path := range []string{"src/tests/sprint-31.ts", "src/protocol.ts"} {
		if !smokeAuthorPathAllowed(path, allowed) {
			t.Fatalf("expected authoring path %q to be allowed", path)
		}
	}
	for _, path := range []string{"src/cli.ts", "src/tests-escape/file.ts", "runs/run.json"} {
		if smokeAuthorPathAllowed(path, allowed) {
			t.Fatalf("expected authoring path %q to be rejected", path)
		}
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
	manifest := `{"schemaVersion":1,"protocolVersion":"1.0","harness":{"id":"fake","version":"1"},"executable":"runner","cwd":".","commands":{"discover":["discover"],"run":["run"]},"evidence":{"runs":"runs","issues":"issues"},"authoring":{"paths":["suite"]},"capabilities":["discovery","run","evidence-v1","scope-mapping","authoring-v1"],"environment":[]}`
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
		discovery: smokeDiscovery{SchemaVersion: 1, ProtocolVersion: "1.0", HarnessID: "fake", EvidenceSchema: 1, Suites: []smokeSuite{{ID: "sprint", Tests: []string{"live"}}}, Tests: []smokeTest{{ID: "live", Suite: "sprint", Coverage: []string{"real-boundary"}}}, SprintMappings: []smokeSprintMapping{{Sprint: sp.Slug, Suites: []string{"sprint"}, Complete: true, RequiredCoverage: []string{"real-boundary"}, Rationale: "dedicated"}}},
		run:       smokeRunResponse{SchemaVersion: 1, ProtocolVersion: "1.0", HarnessID: "fake", RunID: runID, ScopeKind: "suite", Scope: []string{"sprint"}, Counts: SmokeCountsWire{Total: 1, Passed: 1}, Tests: []SmokeTestResult{{ID: "live", Status: "passed"}}, DurationMs: 10, Evidence: []smokeEvidenceWire{{Kind: "run", Path: "runs/" + runID + ".json"}, {Kind: "summary", Path: "runs/" + runID + "-summary.md"}}},
	}
	author := &smokeAuthorRuntime{}
	service = NewService(root).WithRuntime(author).WithProcessRunner(runner).WithSmokeSettings(DefaultSmokeSettings()).WithStageRuntime(map[PlanningStage]StageRuntime{StageReview: {Model: "test/model"}, StageSmoke: {Model: "test/author"}})
	result, err := service.RunSmoke(context.Background(), "proj", "01", SmokeRequest{})
	if err != nil || result.Verdict != SmokePass || len(runner.calls) != 2 {
		t.Fatalf("result=%+v calls=%v err=%v", result, runner.calls, err)
	}
	if result.CoverageMapping == nil || !result.CoverageMapping.Complete || result.CoverageMapping.Rationale != "dedicated" || len(result.CoverageMapping.RequiredCoverage) != 1 || len(result.CoverageMapping.Tests) != 1 || result.CoverageMapping.Tests[0].ID != "live" {
		t.Fatalf("coverage mapping was not retained in result: %+v", result.CoverageMapping)
	}
	if len(author.requests) != 1 || author.requests[0].WorkDir != harness || author.requests[0].Model != "author" || !containsString(author.requests[0].RequireCaps, "permissions") || author.requests[0].Policy.UnsupportedBehavior != "" || !strings.Contains(author.requests[0].Prompt, "required deep-smoke coverage ID") || strings.Count(author.requests[0].Prompt, sharedPromptStageBoundary) != 1 {
		t.Fatalf("smoke author request was not sprint-specific and model-routed: %+v", author.requests)
	}
	boundary := strings.Index(author.requests[0].Prompt, sharedPromptStageBoundary) + len(sharedPromptStageBoundary)
	if strings.Contains(author.requests[0].Prompt[:boundary], "UltraPlan Smoke Authoring Manifest") {
		t.Fatal("smoke-specific manifest entered the shared prefix")
	}
	for _, required := range []string{
		"writable-path list above is exhaustive, not illustrative",
		"Use the existing-coverage fast path",
		"run only the harness discovery command",
		"inspect the implementation of every selected test",
		"Coverage IDs and a complete mapping are routing metadata, not evidence",
		"package, integration, fake-runtime",
		"broad test-name regex is insufficient",
		"local regression command cannot convert that criterion to passed",
		"criterion-to-probe audit",
		"union of declared coverage IDs alone must never justify",
		"inspect the run adapter statically",
		"every failed or errored result must be",
		"associated with an open issue returned in the protocol response",
		"observed summary, falsifiable",
		"An issue file written to disk",
		"a constant empty issues array is",
		`{"id":"...","status":"open","path":"issues/<id>.md","test_id":"<failed-test-id>"`,
		"Use the exact snake_case keys shown",
		"Do not substitute camelCase aliases",
		"testId, observedSummary, falsifiableTheory, supportingEvidence",
		"Every displayed value must be a non-empty string",
		"evidence is one string, not an array",
		"failure-to-issue wiring and the criterion-to-probe audit are complete",
		"make no changes and return success promptly",
		"Do not add opportunistic tests",
		"resume it narrowly",
		"Repair only concrete",
		"Prefer finishing the current suite over",
		"Do not execute the harness run command, browsers, product builds",
		"independent discovery validation and execution after authoring returns",
		"inspect the existing harness tests, suites",
		"work left by an earlier unfinished",
		"Only an existing file already inside a",
		"writable path may be adopted directly",
		"Do not duplicate equivalent coverage",
		"Treat existing files as candidates, not as authoritative",
		"Do not weaken assertions merely to make it pass",
		"Every pre-existing path outside the writable-path list is strictly read-only",
		"Never edit, rename",
		"do not adopt or import the out-of-scope file itself",
		"Do not create scratch, debug, probe, backup, generated, or temporary files",
		"Before every write, resolve the destination",
		"inspect every path changed during this authoring session",
		"Never try",
		"leave it",
		"metadata-for-metadata unchanged",
		`"src/tests/probe.ts" is allowed`,
		`"src/test-probe.ts" is forbidden`,
		"will reject the whole authoring run if even one path outside the list changes",
	} {
		if !strings.Contains(author.requests[0].Prompt, required) {
			t.Fatalf("smoke author prompt omitted path-boundary instruction %q", required)
		}
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
	flowState, stateErr := LoadFlowState(root, sp)
	if stateErr != nil || flowState.Smoke == nil || flowState.Smoke.CoverageMapping == nil || flowState.Smoke.CoverageMapping.Tests[0].ID != "live" {
		t.Fatalf("coverage mapping was not persisted: state=%+v err=%v", flowState.Smoke, stateErr)
	}
	originalRun, _ := os.ReadFile(runJSON)
	if err := os.WriteFile(runJSON, []byte("externally edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, statusErr = service.VerificationStatus("proj", "01")
	if statusErr != nil || !status.Smoke.Fresh {
		t.Fatalf("external evidence edit unexpectedly staled smoke while snapshot freshness is disabled: %+v err=%v", status, statusErr)
	}
	if err := os.WriteFile(runJSON, originalRun, 0o644); err != nil {
		t.Fatal(err)
	}
	author.permissions.UnsupportedCount = 1
	runnerCalls := len(runner.calls)
	if _, err := service.RunSmoke(context.Background(), "proj", "01", SmokeRequest{}); err == nil || !strings.Contains(err.Error(), "permission enforcement was unsupported") {
		t.Fatalf("unsupported smoke author permissions err=%v", err)
	}
	if len(runner.calls) != runnerCalls {
		t.Fatalf("smoke proceeded after unsupported author permissions: calls=%v", runner.calls)
	}
	author.permissions.UnsupportedCount = 0
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
	m := smokeManifest{SchemaVersion: 1, ProtocolVersion: "2.0", Harness: smokeHarnessIdentity{ID: "h", Version: "1"}, Executable: "run", CWD: ".", Commands: smokeCommands{Discover: []string{"d"}, Run: []string{"r"}}, Evidence: smokeEvidenceRoots{Runs: "runs", Issues: "issues"}, Authoring: smokeAuthoring{Paths: []string{"src/tests"}}, Capabilities: []string{"discovery", "run", "evidence-v1", "scope-mapping", "authoring-v1"}}
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
	m.Authoring.Paths = []string{"src", "src/tests"}
	if err := validateSmokeManifest(m); err == nil {
		t.Fatal("expected overlapping authoring paths to be rejected")
	}
	m.Authoring.Paths = []string{"."}
	if err := validateSmokeManifest(m); err == nil {
		t.Fatal("expected harness-root authoring to be rejected")
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

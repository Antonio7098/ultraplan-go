package sprint

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	pprocess "github.com/Antonio7098/ultraplan-go/internal/platform/process"
	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

func TestQALegacySessionAuthorsIntoManagedWorkspace(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("ULTRAPLAN_QA_RUNTIME_DIR", t.TempDir())
	root, target := t.TempDir(), t.TempDir()
	writeFileContent(t, target, "package test\n", "calc.go")
	qaMap, shard, spec := authoredTestFixture(t, target)
	if err := os.MkdirAll(filepath.Join(root, "projects", qaMap.Project, "sprints", qaMap.Sprint), 0755); err != nil {
		t.Fatal(err)
	}
	workspace, err := prepareQAInvestigatorWorkspace(context.Background(), root, target, qaMap, shard)
	if err != nil {
		t.Fatal(err)
	}
	legacy := qaLegacyInvestigatorWorkspacePath(root, qaMap.SemanticAttemptID, shard.ID)
	runtime := &qaFeedbackRuntime{t: t, legacyDir: legacy}
	service := NewService(root).WithRuntime(runtime).WithQAMapFence(func(QAMap) error { return nil })
	initial := pruntime.Request{Provider: "openai", Model: "qa", Metadata: map[string]string{"shard": shard.ID}}
	original := QAInvestigatorAttempt{ID: "original", Provider: initial.Provider, Model: initial.Model, SessionID: "original-" + shard.ID, WorkspaceID: hashOpaque(legacy)}
	request := QAArbiterEvidenceRequest{ID: "qa-v2-request-aaaaaaaaaaaaaaaaaaaaaaaa", OriginShardID: shard.ID}
	_, files, _, err := service.continueQAInvestigatorForEvidence(context.Background(), qaMap, shard, initial, original, request, spec, nil, 1)
	if err != nil || len(files) != 1 || runtime.authorCalls != 1 {
		t.Fatalf("legacy continuation failed: files=%d calls=%d err=%s", len(files), runtime.authorCalls, qaUnderlyingDiagnostic(err))
	}
	if _, err := os.Lstat(legacy); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("compatibility link was retained after the runtime stopped")
	}
	if content, err := os.ReadFile(filepath.Join(workspace, spec.ApprovedTestPaths[0])); err != nil || string(content) != files[0].Content {
		t.Fatal("legacy session did not write the managed copy")
	}
}

func TestQALegacyWorkspaceAliasRejectsOccupiedAndRedirectedPaths(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	workspace := t.TempDir()
	legacy := qaLegacyInvestigatorWorkspacePath(t.TempDir(), "attempt", "shard")
	if err := os.MkdirAll(legacy, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := qaLegacyWorkspaceAlias(legacy, workspace); err == nil {
		t.Fatal("occupied directory replaced")
	}
	if err := os.Remove(legacy); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := qaLegacyWorkspaceAlias(legacy, workspace); err == nil {
		t.Fatal("foreign link replaced")
	}
	if err := os.Remove(legacy); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Dir(legacy)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Dir(legacy)); err != nil {
		t.Fatal(err)
	}
	if _, err := qaLegacyWorkspaceAlias(legacy, workspace); err == nil {
		t.Fatal("redirected parent accepted")
	}
}

func TestQARecoveryIncludesLegacyPreparationAndRelocationFailures(t *testing.T) {
	requests := []QAArbiterEvidenceRequest{
		{ID: "snapshot", OriginShardID: "shard", Attempts: 1, ReasonCode: "evidence_authoring_inconclusive", NextAction: "cannot snapshot the private investigator workspace"},
		{ID: "budget", OriginShardID: "shard", Attempts: 1, ReasonCode: "evidence_round_budget_exhausted"},
		{ID: "session", OriginShardID: "session-shard", Attempts: 1, ReasonCode: "original_session_unavailable"},
		{ID: "other", OriginShardID: "other-shard", ReasonCode: "original_session_unavailable"},
	}
	shard := QAShard{ID: "session-shard", Attempts: []QAInvestigatorAttempt{{SessionID: "retained", WorkspaceID: "legacy"}, {ID: "original/evidence/session/1", SessionID: "retained", WorkspaceID: "managed", FailureKind: "original_session_unavailable"}}}
	selected := grantQAInfrastructureRecovery(requests, nil, time.Unix(10, 0), shard)
	if len(selected) != 3 || selected["other"] {
		t.Fatalf("wrong selection: %v", selected)
	}
	grantQAInfrastructureRecovery(requests, nil, time.Unix(20, 0), shard)
	if requests[2].Attempts != 1 || requests[2].RecoveryAllowance != 1 || requests[2].RecoveryGrantedAt.Unix() != 10 {
		t.Fatal("relocation recovery reset accounting")
	}
}

func TestQAAdmissionFailureExplainsCapacityShortfall(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ULTRAPLAN_QA_RUNTIME_DIR", root)
	free, err := qaStorageAvailable(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = reserveQAResources(ctx, free+(1<<30), 0)
	var failure *qaExecutionError
	if !errors.As(err, &failure) || !strings.Contains(failure.Failure.Diagnostic, "Insufficient QA disk:") || !strings.Contains(failure.Failure.Diagnostic, "requested") {
		t.Fatalf("missing capacity diagnostic: %v", err)
	}
	message := qaCapacityDiagnostic("memory", 1900<<20, 0, 1536<<20, 1024<<20)
	if !strings.Contains(message, "free at least 660 MiB") {
		t.Fatal(message)
	}
}

type qaRetryRunner struct {
	calls      int
	alwaysFail bool
}

func (r *qaRetryRunner) Run(_ context.Context, _ pprocess.Request) (pprocess.Result, error) {
	r.calls++
	if r.calls == 1 || r.alwaysFail {
		return pprocess.Result{ExitCode: 1, Stderr: "compile: no space left on device", CleanupAttempted: true, CleanupComplete: true}, nil
	}
	return pprocess.Result{ExitCode: 1, Stdout: "=== RUN TestAdd\n--- FAIL: TestAdd\ngot 1 want 2", CleanupAttempted: true, CleanupComplete: true}, nil
}

func qaReadyTest(t *testing.T) (string, QAMap, QAShard, QAArbiterEvidenceRequest, QATestPublication) {
	t.Helper()
	target := t.TempDir()
	writeFileContent(t, target, "package calc\n", "calc.go")
	qaMap, shard, spec := authoredTestFixture(t, target)
	request := QAArbiterEvidenceRequest{ID: "qa-v2-request-aaaaaaaaaaaaaaaaaaaaaaaa", OriginShardID: shard.ID, TheoryIDs: spec.TheoryIDs, Attempts: 2, EvidenceRound: 2, Status: "ready"}
	spec.EvidenceRequestID = request.ID
	var err error
	spec, err = FreezeQAReproductionSpec(qaMap.Project, qaMap.Sprint, spec, qaMap.Budgets, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := BuildQATestBundle(qaMap.Project, qaMap.Sprint, spec, []QATestFile{{Path: "calc_test.go", Content: "package calc\n"}}, "", qaMap.Budgets)
	if err != nil {
		t.Fatal(err)
	}
	request.TestBundleID = bundle.ID
	return target, qaMap, shard, request, QATestPublication{Spec: spec, Bundle: bundle}
}

func TestQAInfrastructureRetryReusesBundleBeyondAuthoringBudget(t *testing.T) {
	target, qaMap, shard, request, test := qaReadyTest(t)
	runner := &qaRetryRunner{}
	service := NewService(t.TempDir()).WithProcessRunner(runner)
	requests := []QAArbiterEvidenceRequest{request}
	checkpoints := 0
	_, tests, progressed, err := service.strengthenQARequestedEvidence(context.Background(), qaMap, target, []QAShard{shard}, requests, map[string]bool{request.ID: true}, []QATestPublication{test}, func(_ []QAShard, publications []QATestPublication, current []QAArbiterEvidenceRequest) error {
		checkpoints++
		if len(publications) != 1 || publications[0].Bundle.ID != test.Bundle.ID {
			t.Fatal("retry replaced immutable bundle")
		}
		if current[0].Attempts != 2 {
			t.Fatal("execution consumed authoring allowance")
		}
		return nil
	})
	if err != nil || !progressed || runner.calls != 2 || len(tests) != 1 || len(tests[0].Runs) != 2 || checkpoints < 4 {
		t.Fatalf("retry: calls=%d tests=%+v err=%v", runner.calls, tests, err)
	}
	if requests[0].Status != "evidence_recorded" || tests[0].Runs[0].ReasonCode != "disk_space_exhausted" || tests[0].Runs[1].Outcome != QAEvidenceFail {
		t.Fatalf("incorrect result: %+v", requests[0])
	}
	_, more, _, err := service.strengthenQARequestedEvidence(context.Background(), qaMap, target, []QAShard{shard}, requests, map[string]bool{request.ID: true}, tests, func([]QAShard, []QATestPublication, []QAArbiterEvidenceRequest) error { return nil })
	if err != nil || len(more) != 0 || runner.calls != 2 {
		t.Fatal("resume reran accepted evidence")
	}
}

func TestQAExecutionReservationSurvivesProcessInterruption(t *testing.T) {
	target, qaMap, shard, request, test := qaReadyTest(t)
	runner := &qaRetryRunner{alwaysFail: true}
	service := NewService(t.TempDir()).WithProcessRunner(runner)
	var saved []byte
	interrupted := errors.New("process ended after durable reservation")
	_, _, _, err := service.strengthenQARequestedEvidence(context.Background(), qaMap, target, []QAShard{shard}, []QAArbiterEvidenceRequest{request}, map[string]bool{request.ID: true}, []QATestPublication{test}, func(_ []QAShard, _ []QATestPublication, r []QAArbiterEvidenceRequest) error {
		saved, _ = json.Marshal(r)
		return interrupted
	})
	if !errors.Is(err, interrupted) || runner.calls != 0 {
		t.Fatal("command started without its durable reservation")
	}
	var requests []QAArbiterEvidenceRequest
	if err := json.Unmarshal(saved, &requests); err != nil {
		t.Fatal(err)
	}
	if len(requests[0].ExecutionAttempts) != 1 {
		t.Fatal("reservation missing")
	}
	_, tests, _, err := service.strengthenQARequestedEvidence(context.Background(), qaMap, target, []QAShard{shard}, requests, map[string]bool{request.ID: true}, []QATestPublication{test}, func([]QAShard, []QATestPublication, []QAArbiterEvidenceRequest) error { return nil })
	if err != nil || runner.calls != 2 || requests[0].ReasonCode != "infrastructure_retry_budget_exhausted" {
		t.Fatalf("reservation reset: %+v %v", requests, err)
	}
	_, _, _, err = service.strengthenQARequestedEvidence(context.Background(), qaMap, target, []QAShard{shard}, requests, map[string]bool{request.ID: true}, tests, func([]QAShard, []QATestPublication, []QAArbiterEvidenceRequest) error { return nil })
	if err != nil || runner.calls != 2 {
		t.Fatal("resume reset exhausted execution budget")
	}
}

func TestQARecoveryAllowanceIsBoundedAndSelective(t *testing.T) {
	_, _, _, request, test := qaReadyTest(t)
	request.ReasonCode = "investigator_workspace_unavailable"
	request.TestBundleID = ""
	requests := []QAArbiterEvidenceRequest{request, {ID: "fixture", OriginShardID: request.OriginShardID, ReasonCode: "test_bundle_invalid"}, {ID: "waiting", OriginShardID: request.OriginShardID, ReasonCode: "evidence_round_budget_exhausted"}, {ID: "done", Status: "evidence_recorded"}}
	active := grantQAInfrastructureRecovery(requests, nil, time.Unix(10, 0))
	if len(active) != 2 || !active[request.ID] || !active["waiting"] {
		t.Fatalf("wrong retry selection: %v", active)
	}
	grantQAInfrastructureRecovery(requests, []QATestPublication{test}, time.Unix(20, 0))
	if requests[0].RecoveryAllowance != 1 || requests[0].RecoveryGrantedAt.Unix() != 10 || requests[0].Attempts != 2 {
		t.Fatal("recovery rewrote old accounting or replenished allowance")
	}
}

func TestQARequestReportingKeepsBrokenChainsVisible(t *testing.T) {
	requests := []QAArbiterEvidenceRequest{{ID: "old", SupersededBy: "new"}, {ID: "new"}, {ID: "missing", SupersededBy: "absent"}, {ID: "cycle1", SupersededBy: "cycle2"}, {ID: "cycle2", SupersededBy: "cycle1"}, {ID: "answered", Status: "evidence_recorded"}}
	active, history := qaRequestBlockers(requests)
	if len(active) != 4 || len(history) != 1 || history[0].Scope != "old" {
		t.Fatalf("active=%+v history=%+v", active, history)
	}
}

func TestQAFailureDiagnosticsSeparateInfrastructureFromFixtures(t *testing.T) {
	for _, tc := range []struct {
		phase, text, code string
		err               error
		retry             bool
	}{
		{"workspace", "", "disk_space_exhausted", &os.PathError{Op: "copy", Path: "snapshot", Err: syscall.ENOSPC}, true},
		{"compile", "Get module: connection reset by peer", "dependency_network_failure", nil, true},
		{"assertion", "connection reset by peer", "dependency_network_failure", nil, false},
		{"fixture", "template component/findings not found", "fixture_failure", nil, false},
		{"compile", "build failed: undefined: missing", "compile_failure", nil, false},
	} {
		d := qaFailureDiagnostic(tc.phase, tc.err, tc.text)
		if d.Code != tc.code || d.Retryable != tc.retry {
			t.Fatalf("%s: %+v", tc.text, d)
		}
	}
}

func TestQAResourceReservationCoordinatesAndReleases(t *testing.T) {
	if root := os.Getenv("ULTRAPLAN_TEST_RESERVATION_CHILD"); root != "" {
		release, ok, err := tryReserveQAResources(root, 100<<20, 0)
		if release != nil {
			release()
		}
		if err != nil || ok {
			t.Fatalf("child ignored parent reservation: %t %v", ok, err)
		}
		return
	}
	root := t.TempDir()
	free, err := qaStorageAvailable(root)
	if err != nil {
		t.Skip(err)
	}
	release, ok, err := tryReserveQAResources(root, free-(300<<20), 0)
	if err != nil || !ok {
		t.Fatalf("reserve: %t %v", ok, err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(executable, "-test.run=^TestQAResourceReservationCoordinatesAndReleases$")
	child.Env = append(os.Environ(), "ULTRAPLAN_TEST_RESERVATION_CHILD="+root)
	if output, err := child.CombinedOutput(); err != nil {
		t.Fatalf("cross-process reservation: %s %v", output, err)
	}
	_, ok, err = tryReserveQAResources(root, 100<<20, 0)
	if err != nil || ok {
		t.Fatal("concurrent admission ignored reservation")
	}
	release()
	release, ok, err = tryReserveQAResources(root, 100<<20, 0)
	if err != nil || !ok {
		t.Fatal("released capacity unavailable")
	}
	release()
	data, _ := json.Marshal(qaResourceLease{PID: 2147483647, Bytes: free})
	if err := os.WriteFile(filepath.Join(root, "reservation-dead.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	release, ok, err = tryReserveQAResources(root, 1, 0)
	if err != nil || !ok {
		t.Fatal("dead worker retained capacity")
	}
	release()
}

func TestQARedactionPreservesStructuredAssertionEvidence(t *testing.T) {
	target, qaMap, _, _, test := qaReadyTest(t)
	runner := &qaRetryRunner{calls: 1}
	run, err := RunQAReproduction(context.Background(), QAReproductionRequest{Project: qaMap.Project, Sprint: qaMap.Sprint, TargetRoot: target, WorkspaceParent: t.TempDir(), Spec: test.Spec, Bundle: test.Bundle, Budgets: qaMap.Budgets, ExpectedTargetID: test.Spec.ImplementationFingerprint, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Result.TestEvents) != 2 || run.Result.MatchedMarker != test.Spec.PredictedFailure.OutputMatcher {
		t.Fatal("structured assertion evidence missing")
	}
	run.Result.TestEvents[0].Test = "invented"
	if err := ValidateQAReproductionRun(run, test.Spec, test.Bundle); err == nil || !strings.Contains(err.Error(), "events") {
		t.Fatal("forged events admitted")
	}
}

func TestQADependencyCacheReusedAcrossPackagesAndReadOnlyInTests(t *testing.T) {
	if !pprocess.IsolationCapabilityFacts().NativeProtectedRootDeny {
		t.Skip("native isolation unavailable")
	}
	target := t.TempDir()
	writeFileContent(t, target, "module example.test/cache\n\ngo 1.22\n", "go.mod")
	writeFileContent(t, target, "package calc\n", "calc.go")
	writeFileContent(t, target, "package sub\n", "sub", "calc.go")
	qaMap, _, spec := authoredTestFixture(t, target)
	spec.EvidenceRequestID = "qa-v2-request-aaaaaaaaaaaaaaaaaaaaaaaa"
	spec.Command.Args = []string{"test", ".", "-v", "-run", "^TestAdd$", "-count=1"}
	var err error
	spec, err = FreezeQAReproductionSpec(qaMap.Project, qaMap.Sprint, spec, qaMap.Budgets, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	content := "package calc\nimport (\"os\"; \"path/filepath\"; \"testing\")\nfunc TestAdd(t *testing.T) { cache:=os.Getenv(\"GOMODCACHE\"); if cache == \"\" { t.Fatal(\"cache missing\") }; if _,err:=os.ReadFile(filepath.Join(cache,\".ready\")); err != nil { t.Fatal(err) }; if err:=os.WriteFile(filepath.Join(cache,\"poison\"),[]byte(\"bad\"),0600); err == nil { t.Fatal(\"shared cache was writable\") } }\n"
	bundle, err := BuildQATestBundle(qaMap.Project, qaMap.Sprint, spec, []QATestFile{{Path: "calc_test.go", Content: content}}, "", qaMap.Budgets)
	if err != nil {
		t.Fatal(err)
	}
	req := QAReproductionRequest{Project: qaMap.Project, Sprint: qaMap.Sprint, TargetRoot: target, WorkspaceParent: t.TempDir(), Spec: spec, Bundle: bundle, Budgets: qaMap.Budgets, ExpectedTargetID: spec.ImplementationFingerprint}
	first, version, err := prepareQADependencyCache(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = removeQAReproductionRuntime(first) })
	before, err := os.Stat(filepath.Join(first, ".ready"))
	if err != nil {
		t.Fatal(err)
	}
	other := req
	other.Spec.Command.WorkingDirectory = "sub"
	second, secondVersion, err := prepareQADependencyCache(context.Background(), other)
	if err != nil || first != second || version != secondVersion {
		t.Fatalf("package used a cold cache: %s %s %v", first, second, err)
	}
	after, _ := os.Stat(filepath.Join(second, ".ready"))
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("ready cache was prepared again")
	}
	run, err := RunQAReproduction(context.Background(), req)
	if err != nil || run.Outcome != QAEvidencePass {
		t.Fatalf("cache isolation: %+v %v", run, err)
	}
}

func TestQALegacyMapsKeepTheirEvidenceAccounting(t *testing.T) {
	legacy := QAMap{Budgets: DefaultQABudgets()}
	if qaEvidenceRequestLimit(legacy) != legacy.Budgets.EvidenceRoundsPerShard {
		t.Fatal("legacy map silently acquired a larger request budget")
	}
	modern := legacy
	modern.EvidenceAccountingVersion = 2
	if qaEvidenceRequestLimit(modern) != modern.Budgets.TheoriesPerShard {
		t.Fatal("new requests still limited by shard size")
	}
}

func TestQARetainedAcceptedCheckKeepsItsIdentity(t *testing.T) {
	plan := QAEvidencePlan{ID: "original", ImplementationFingerprint: "frozen", CheckID: "check"}
	record := QAEvidenceRecord{ID: "accepted", PlanID: plan.ID, Outcome: QAEvidencePass}
	retained := []qaRetainedCheck{{plan: plan, record: record}}
	found, ok := retainedQACheckForPlan(retained, plan)
	if !ok {
		t.Fatal("accepted check not reusable")
	}
	cloned, err := cloneQAEvidenceForPlan(Sprint{}, found, plan)
	if err != nil || cloned.ID != record.ID {
		t.Fatal("unchanged accepted evidence was replaced")
	}
	plan.ImplementationFingerprint = "changed"
	if _, ok := retainedQACheckForPlan(retained, plan); ok {
		t.Fatal("stale check reused")
	}
}

func TestQAWorkspaceDiagnosticRetainsSanitizedUnderlyingCause(t *testing.T) {
	underlying := &os.PathError{Op: "copy", Path: "snapshot", Err: syscall.ENOSPC}
	wrapped := NewQAError(QAErrorPermissionDenied, "restore", "workspace unavailable token=private-value", underlying)
	failure := qaFailureDiagnostic("workspace", wrapped, "")
	if failure.Code != "disk_space_exhausted" || !strings.Contains(failure.Diagnostic, "copy snapshot") || strings.Contains(failure.Diagnostic, "private-value") {
		t.Fatalf("underlying cause lost or leaked: %+v", failure)
	}
}

func TestQARuntimeStorageRejectsProtectedLocationBeforeWriting(t *testing.T) {
	target := t.TempDir()
	runtime := filepath.Join(target, "qa-cache")
	t.Setenv("ULTRAPLAN_QA_RUNTIME_DIR", runtime)
	if err := validateQARuntimeLocation([]string{target}); err == nil {
		t.Fatal("runtime inside target admitted")
	}
	if _, err := os.Stat(runtime); !os.IsNotExist(err) {
		t.Fatal("validation wrote inside target")
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(target, alias); err != nil {
		t.Skip(err)
	}
	t.Setenv("ULTRAPLAN_QA_RUNTIME_DIR", filepath.Join(alias, "qa-cache"))
	if err := validateQARuntimeLocation([]string{target}); err == nil {
		t.Fatal("symlink concealed protected runtime location")
	}
}

func TestQAArbiterCannotGrantItselfRecoveryAllowance(t *testing.T) {
	_, qaMap, shard, request, _ := qaReadyTest(t)
	qaMap.EvidenceAccountingVersion = 2
	request.Gap, request.RequestedEvidence, request.RequiredObservation, request.ControlRequirement, request.Priority = "gap", "test", "observe", "control", "high"
	now := time.Now()
	request.RecoveryAllowance, request.RecoveryGrantedAt, request.RecoveryReason = 100, &now, "model override"
	request.PreparationAttempts, request.InfrastructureRetries = 99, 99
	request.Failure = &QAFailureDiagnostic{Code: "made-up", Retryable: true}
	group := qaTheoryGroupPlan{ID: "group", Theories: shard.Theories}
	validated, err := validateQAArbiterEvidenceRequests(qaMap, group, []QAArbiterEvidenceRequest{request}, map[string]QATheoryOutcome{shard.Theories[0].ID: QATheoryInconclusive})
	if err != nil {
		t.Fatal(err)
	}
	got := validated[0]
	if got.RecoveryAllowance != 0 || got.RecoveryGrantedAt != nil || got.RecoveryReason != "" || got.PreparationAttempts != 0 || got.InfrastructureRetries != 0 || got.Failure != nil || got.Attempts != 0 {
		t.Fatal("model changed product-owned accounting")
	}
}

package sprint

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pprocess "github.com/Antonio7098/ultraplan-go/internal/platform/process"
)

type authoredTestRunner struct{}

func (authoredTestRunner) Run(_ context.Context, req pprocess.Request) (pprocess.Result, error) {
	content, err := os.ReadFile(filepath.Join(req.Dir, "calc.go"))
	if err != nil {
		return pprocess.Result{}, err
	}
	if strings.Contains(string(content), "return a + b") {
		return pprocess.Result{ExitCode: 0, Stdout: "ok example.test/calc", CleanupComplete: true}, nil
	}
	return pprocess.Result{ExitCode: 1, Stdout: "--- FAIL: TestAdd\n    calc_test.go:3: got 1 want 2", CleanupComplete: true}, nil
}

type uncleanAuthoredTestRunner struct{}

func (uncleanAuthoredTestRunner) Run(_ context.Context, _ pprocess.Request) (pprocess.Result, error) {
	return pprocess.Result{ExitCode: 1, Stdout: "--- FAIL: TestAdd\n    calc_test.go:3: got 1 want 2", CleanupComplete: false}, nil
}

func authoredTestFixture(t *testing.T, target string) (QAMap, QAShard, QAReproductionSpec) {
	t.Helper()
	const project, sprintSlug = "alpha", "01-authored-tests"
	fingerprint, err := targetIdentity(target)
	if err != nil {
		t.Fatal(err)
	}
	identity := QASemanticIdentity{GovernedInputFingerprint: strings.Repeat("1", 64), ImplementationFingerprint: fingerprint, ReviewFingerprint: strings.Repeat("2", 64), PolicyFingerprint: strings.Repeat("3", 64)}
	attemptID, err := NewQASemanticAttemptID(project, sprintSlug, identity)
	if err != nil {
		t.Fatal(err)
	}
	mapID, err := NewQAMapID(project, sprintSlug, attemptID, identity)
	if err != nil {
		t.Fatal(err)
	}
	shardID, err := NewQAShardID(project, sprintSlug, mapID, QAShardIdentity{Kind: QAShardPrimary, ChangedPaths: []string{"calc.go"}})
	if err != nil {
		t.Fatal(err)
	}
	theoryID, err := NewQATheoryID(project, sprintSlug, shardID, QATheoryIdentity{Claim: "Add returns the sum", Basis: "behavior", VerificationSurface: "calc.go"})
	if err != nil {
		t.Fatal(err)
	}
	budgets := DefaultQABudgets()
	qaMap := QAMap{Project: project, Sprint: sprintSlug, ID: mapID, SemanticAttemptID: attemptID, ImplementationFingerprint: fingerprint, Budgets: budgets}
	shard := QAShard{ID: shardID, Theories: []QATheory{{ID: theoryID, ShardID: shardID, Outcome: QATheoryInconclusive}}}
	spec, err := FreezeQAReproductionSpec(project, sprintSlug, QAReproductionSpec{
		AttemptID: attemptID, ShardID: shardID, TheoryIDs: []string{theoryID}, Claim: "Add returns the wrong value",
		ExpectedBehavior: "Add(1, 1) returns 2", InconclusiveConditions: []string{"unrelated failure"}, ApprovedTestPaths: []string{"calc_test.go"},
		PredictedFailure:          QAFailureSignature{TestName: "TestAdd", Assertion: "got 1 want 2", ExitClass: "nonzero", OutputMatcher: "got 1 want 2"},
		Command:                   QACheckDescriptor{ID: "go-test-add", Executable: "go", Args: []string{"test", ".", "-run", "^TestAdd$", "-count=1"}, Timeout: time.Minute, OutputLimit: 64 << 10},
		ImplementationFingerprint: fingerprint,
	}, budgets, time.Unix(100, 0))
	if err != nil {
		t.Fatal(err)
	}
	return qaMap, shard, spec
}

func TestReproductionClassificationRejectsUnrelatedFailures(t *testing.T) {
	signature := QAFailureSignature{TestName: "TestClaim", Assertion: "want true", ExitClass: "nonzero", OutputMatcher: "want true"}
	base := QACommandResult{ExitCode: 1, CleanupComplete: true, Stdout: "--- FAIL: TestClaim\nwant true"}
	if outcome, _ := ClassifyQAReproductionResult(base, signature); outcome != QAEvidenceFail {
		t.Fatalf("matching failure = %q", outcome)
	}
	cases := map[string]QACommandResult{
		"compile":   {ExitCode: 1, CleanupComplete: true, Stderr: "build failed: undefined: missing"},
		"panic":     {ExitCode: 1, CleanupComplete: true, Stderr: "panic: unrelated"},
		"timeout":   {ExitCode: 1, CleanupComplete: true, TimedOut: true},
		"truncated": {ExitCode: 1, CleanupComplete: true, Truncated: true, Stdout: base.Stdout},
		"mismatch":  {ExitCode: 1, CleanupComplete: true, Stdout: "--- FAIL: TestClaim\nwant false"},
	}
	for name, result := range cases {
		t.Run(name, func(t *testing.T) {
			if outcome, _ := ClassifyQAReproductionResult(result, signature); outcome != QAEvidenceInconclusive {
				t.Fatalf("unrelated failure = %q", outcome)
			}
		})
	}
}

func TestReproductionClassificationUsesExecutableMatcherNotProseAssertion(t *testing.T) {
	signature := QAFailureSignature{TestName: "TestClaim", Assertion: "a long human-readable observation", ExitClass: "nonzero", OutputMatcher: "ULTRAPLAN_QA_PREDICTED_FAILURE:TestClaim"}
	result := QACommandResult{ExitCode: 1, CleanupComplete: true, Stdout: "--- FAIL: TestClaim\nULTRAPLAN_QA_PREDICTED_FAILURE:TestClaim: got invalid behavior"}
	outcome, reason := ClassifyQAReproductionResult(result, signature)
	if outcome != QAEvidenceFail || reason != "predicted_failure_reproduced" {
		t.Fatalf("classification = %s %q", outcome, reason)
	}
}

func TestAuthoredBundleIsAllowlistedAndImmutable(t *testing.T) {
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "calc.go"), []byte("package calc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	qaMap, _, spec := authoredTestFixture(t, target)
	bundle, err := BuildQATestBundle(qaMap.Project, qaMap.Sprint, spec, []QATestFile{{Path: "calc_test.go", Content: "package calc\n"}}, "", qaMap.Budgets)
	if err != nil {
		t.Fatal(err)
	}
	changed := bundle
	changed.Files = append([]QATestFile(nil), bundle.Files...)
	changed.Files[0].Content += "// changed\n"
	if err := ValidateQATestBundle(changed, spec, qaMap.Budgets); err == nil {
		t.Fatal("changed bundle content retained the original digest")
	}
	if _, err := BuildQATestBundle(qaMap.Project, qaMap.Sprint, spec, []QATestFile{{Path: "calc.go", Content: "package calc\n"}}, "", qaMap.Budgets); err == nil {
		t.Fatal("product source escaped the authored-test allowlist")
	}
	derived, err := DeriveQATestBundle(qaMap.Project, qaMap.Sprint, spec, bundle, []QATestFile{{Path: "calc_test.go", Content: "package calc\n// repository convention\n"}}, qaMap.Budgets)
	if err != nil || derived.DerivedFrom != bundle.ID || derived.ID == bundle.ID {
		t.Fatalf("derived bundle = %+v, %v", derived, err)
	}
	if _, err := DeriveQATestBundle(qaMap.Project, qaMap.Sprint, spec, bundle, bundle.Files, qaMap.Budgets); err == nil {
		t.Fatal("unchanged content created a derived bundle")
	}
}

func TestArbiterEvidenceRequestsStayWithinOriginShard(t *testing.T) {
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "calc.go"), []byte("package calc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	qaMap, shardA, _ := authoredTestFixture(t, target)
	shardBID, err := NewQAShardID(qaMap.Project, qaMap.Sprint, qaMap.ID, QAShardIdentity{Kind: QAShardBoundary, ChangedPaths: []string{"other.go"}})
	if err != nil {
		t.Fatal(err)
	}
	theoryBID, err := NewQATheoryID(qaMap.Project, qaMap.Sprint, shardBID, QATheoryIdentity{Claim: "other", Basis: "behavior", VerificationSurface: "other.go"})
	if err != nil {
		t.Fatal(err)
	}
	group := qaTheoryGroupPlan{ID: "group", Theories: []QATheory{shardA.Theories[0], {ID: theoryBID, ShardID: shardBID, Outcome: QATheoryInconclusive}}}
	outcomes := map[string]QATheoryOutcome{shardA.Theories[0].ID: QATheoryInconclusive, theoryBID: QATheoryInconclusive}
	request := func(shardID string, theoryIDs ...string) QAArbiterEvidenceRequest {
		return QAArbiterEvidenceRequest{TheoryIDs: theoryIDs, OriginShardID: shardID, Gap: "missing discriminating behavior", RequestedEvidence: "a focused test", RequiredObservation: "expected assertion", ControlRequirement: "run one control", Priority: "high"}
	}
	if _, err := validateQAArbiterEvidenceRequests(qaMap, group, []QAArbiterEvidenceRequest{request(shardA.ID, shardA.Theories[0].ID, theoryBID)}, outcomes); err == nil {
		t.Fatal("cross-shard request was accepted")
	}
	requests, err := validateQAArbiterEvidenceRequests(qaMap, group, []QAArbiterEvidenceRequest{request(shardA.ID, shardA.Theories[0].ID), request(shardBID, theoryBID)}, outcomes)
	if err != nil || len(requests) != 2 {
		t.Fatalf("per-origin requests = %+v, %v", requests, err)
	}
	invalidPriority := request(shardA.ID, shardA.Theories[0].ID)
	invalidPriority.Priority = "1"
	if _, err := validateQAArbiterEvidenceRequests(qaMap, group, []QAArbiterEvidenceRequest{invalidPriority}, outcomes); err == nil || !strings.Contains(err.Error(), "priority") {
		t.Fatalf("expected invalid priority rejection, got %v", err)
	}
}

func TestInvestigatorEvidencePromptRequiresExactFailureSignature(t *testing.T) {
	source, err := os.ReadFile("qa_evidence_continuation.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, required := range []string{"predicted_failure.test_name", "assertion", "exact single-line output_matcher", "passing control"} {
		if !strings.Contains(text, required) {
			t.Fatalf("investigator continuation prompt does not require %q", required)
		}
	}
}

func TestRequestedReproductionSpecUsesDeterministicOutputMarker(t *testing.T) {
	target := t.TempDir()
	if err := os.Mkdir(filepath.Join(target, "internal"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "internal", "calc.go"), []byte("package calc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	qaMap, shard, _ := authoredTestFixture(t, target)
	shard.ChangedPaths = []string{"internal/calc.go"}
	request := QAArbiterEvidenceRequest{
		ID:                  "qa-v1-request-abc123",
		TheoryIDs:           []string{shard.Theories[0].ID},
		OriginShardID:       shard.ID,
		Gap:                 "missing behavior",
		RequestedEvidence:   "a focused test",
		RequiredObservation: "error includes punctuation: got %q; want a bounded value",
		ControlRequirement:  "a passing control",
		Priority:            "high",
	}
	spec, err := buildQARequestedReproductionSpec(qaMap, shard, request, target, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	want := "ULTRAPLAN_QA_PREDICTED_FAILURE:" + spec.PredictedFailure.TestName
	if spec.PredictedFailure.OutputMatcher != want {
		t.Fatalf("output matcher = %q, want %q", spec.PredictedFailure.OutputMatcher, want)
	}
	if spec.PredictedFailure.Assertion != request.RequiredObservation {
		t.Fatalf("assertion = %q, want arbiter observation %q", spec.PredictedFailure.Assertion, request.RequiredObservation)
	}
}

func TestEvidencePlanFreezeTimeIsStableAcrossResume(t *testing.T) {
	started := time.Unix(123, 456).UTC()
	shard := QAShard{Attempts: []QAInvestigatorAttempt{{StartedAt: started}}}
	if got := qaEvidencePlanFrozenAt(shard); !got.Equal(started) {
		t.Fatalf("freeze time = %v, want %v", got, started)
	}
	if first, second := qaEvidencePlanFrozenAt(QAShard{}), qaEvidencePlanFrozenAt(QAShard{}); !first.Equal(second) || first.IsZero() {
		t.Fatalf("fallback freeze times = %v and %v", first, second)
	}
}

func TestResumeAppliesLatestRetainedAuthoredTestOutcome(t *testing.T) {
	shards := []QAShard{{Theories: []QATheory{{ID: "theory", Outcome: QATheoryInconclusive}}}}
	tests := []QATestPublication{{Spec: QAReproductionSpec{TheoryIDs: []string{"theory"}}, Runs: []QAReproductionRun{
		{Outcome: QAEvidenceInconclusive},
		{Outcome: QAEvidenceFail},
	}}}
	got := applyRetainedQAReproductionOutcomes(shards, tests)
	if got[0].Theories[0].Outcome != QATheoryConfirmed || !strings.Contains(got[0].Theories[0].OutcomeReason, "matched") {
		t.Fatalf("retained outcome = %+v", got[0].Theories[0])
	}
}

func TestLegacySignatureMismatchRemainsLoadableAfterMatcherUpgrade(t *testing.T) {
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "calc.go"), []byte("package calc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	qaMap, _, spec := authoredTestFixture(t, target)
	spec.PredictedFailure = QAFailureSignature{TestName: "TestAdd", Assertion: "descriptive assertion", ExitClass: "nonzero", OutputMatcher: "MARKER"}
	spec, err := FreezeQAReproductionSpec(qaMap.Project, qaMap.Sprint, spec, qaMap.Budgets, spec.FrozenAt)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := BuildQATestBundle(qaMap.Project, qaMap.Sprint, spec, []QATestFile{{Path: "calc_test.go", Content: "package calc\n"}}, "", qaMap.Budgets)
	if err != nil {
		t.Fatal(err)
	}
	runID, err := NewQAV2ID("run", qaMap.Project, qaMap.Sprint, spec.ID, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	run := QAReproductionRun{SchemaVersion: QAEvidenceSchemaVersion, ID: runID, SpecID: spec.ID, TestBundleID: bundle.ID, TargetIdentity: qaMap.ImplementationFingerprint, Result: QACommandResult{ExitCode: 1, Stdout: "--- FAIL: TestAdd\nMARKER", CleanupComplete: true}, Signature: spec.PredictedFailure, Outcome: QAEvidenceInconclusive, ReasonCode: "failure_signature_mismatch", Cleanup: QACleanupFacts{Attempted: true, DescendantsTerminated: true, WorkspaceRemoved: true, Complete: true}, CompletedAt: time.Unix(200, 0)}
	if err := ValidateQAReproductionRun(run, spec, bundle); err != nil {
		t.Fatal(err)
	}
}

func TestRetainedInvestigatorIdentityMustMatchExactly(t *testing.T) {
	original := QAInvestigatorAttempt{SessionID: "session", Provider: "provider", Model: "model", Variant: "high", RuntimeStoreRef: "/runtime", WorkspaceID: "workspace"}
	if err := validateRetainedRuntimeIdentity(original, "provider", "model", "high", "/runtime", "workspace", "session"); err != nil {
		t.Fatal(err)
	}
	cases := map[string][]string{
		"session loss": {"provider", "model", "high", "/runtime", "workspace", ""},
		"provider":     {"replacement", "model", "high", "/runtime", "workspace", "session"},
		"model":        {"provider", "replacement", "high", "/runtime", "workspace", "session"},
		"variant":      {"provider", "model", "low", "/runtime", "workspace", "session"},
		"runtime":      {"provider", "model", "high", "/replacement", "workspace", "session"},
		"workspace":    {"provider", "model", "high", "/runtime", "replacement", "session"},
	}
	for name, values := range cases {
		t.Run(name, func(t *testing.T) {
			if err := validateRetainedRuntimeIdentity(original, values[0], values[1], values[2], values[3], values[4], values[5]); err == nil {
				t.Fatal("changed investigator identity was accepted")
			}
		})
	}
}

func TestOnlyAffectedTheoryGroupsRequireRearbitration(t *testing.T) {
	groups := []qaTheoryGroupPlan{{Theories: []QATheory{{ID: "theory-a"}}}, {Theories: []QATheory{{ID: "theory-b"}}}}
	if !qaTheoryGroupAffected(groups[0], map[string]bool{"theory-a": true}) {
		t.Fatal("affected group was retained")
	}
	if qaTheoryGroupAffected(groups[1], map[string]bool{"theory-a": true}) {
		t.Fatal("unaffected group was selected for re-arbitration")
	}
}

func TestTestsPerIssueBudgetRemovesRepairEligibility(t *testing.T) {
	candidates := map[string]QAIssueCandidate{"issue": {RepairEligible: true, RegressionCandidate: true}}
	enforceQAAuthoredTestIssueBudget(candidates, map[string]map[string]bool{"issue": {"test-a": true, "test-b": true}}, 1)
	if candidates["issue"].RepairEligible || candidates["issue"].RegressionCandidate {
		t.Fatal("over-budget authored tests remained repair eligible")
	}
}

func TestIssueCoverageCanBindOneTestToSeveralTheories(t *testing.T) {
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "calc.go"), []byte("package calc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	qaMap, shard, spec := authoredTestFixture(t, target)
	first := shard.Theories[0].ID
	second, err := NewQATheoryID(qaMap.Project, qaMap.Sprint, shard.ID, QATheoryIdentity{Claim: "shared cause", Basis: "behavior", VerificationSurface: "calc.go"})
	if err != nil {
		t.Fatal(err)
	}
	spec.TheoryIDs = append(spec.TheoryIDs, second)
	spec, err = FreezeQAReproductionSpec(qaMap.Project, qaMap.Sprint, spec, qaMap.Budgets, spec.FrozenAt)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := BuildQATestBundle(qaMap.Project, qaMap.Sprint, spec, []QATestFile{{Path: "calc_test.go", Content: "package calc\n"}}, "", qaMap.Budgets)
	if err != nil {
		t.Fatal(err)
	}
	issueID, err := NewQAV2ID("issue", qaMap.Project, qaMap.Sprint, qaMap.SemanticAttemptID, "shared")
	if err != nil {
		t.Fatal(err)
	}
	coverage := QAIssueEvidenceCoverage{SchemaVersion: QAEvidenceSchemaVersion, IssueID: issueID, TheoryIDs: append([]string(nil), spec.TheoryIDs...), TestBundleIDs: []string{bundle.ID}, PrimaryReproducers: []string{bundle.ID}, Coverage: map[string][]string{first: {bundle.ID}, second: {bundle.ID}}}
	if err := ValidateQAIssueEvidenceCoverage(coverage); err != nil {
		t.Fatal(err)
	}
	evidenceID, err := NewQAV2ID("evidence", qaMap.Project, qaMap.Sprint, spec.ID, "failure")
	if err != nil {
		t.Fatal(err)
	}
	built := buildQAIssueEvidenceCoverage(
		QAAdjudication{Issues: []QAIssue{{ID: issueID, EvidenceIDs: []string{evidenceID}}}},
		[]QAArbiterIssue{{Title: "different reconciler title", Location: "different/location.go", TheoryIDs: []string{first}}},
		[]QATestPublication{{Spec: spec, Bundle: bundle, Runs: []QAReproductionRun{{Outcome: QAEvidenceFail}}}},
		map[string]string{bundle.ID: evidenceID},
	)
	if len(built) != 1 || !containsQAString(built[0].TheoryIDs, first) || !containsQAString(built[0].TheoryIDs, second) {
		t.Fatalf("authored evidence identity did not preserve issue coverage across reconciliation: %+v", built)
	}
	delete(coverage.Coverage, second)
	if err := ValidateQAIssueEvidenceCoverage(coverage); err == nil {
		t.Fatal("confirmed theory without test coverage was accepted")
	}
}

func TestDuplicateEvidenceRequestRequiresNewEvidence(t *testing.T) {
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "calc.go"), []byte("package calc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	qaMap, shard, _ := authoredTestFixture(t, target)
	request := QAArbiterEvidenceRequest{TheoryIDs: []string{shard.Theories[0].ID}, OriginShardID: shard.ID, Gap: "gap", RequestedEvidence: "test", RequiredObservation: "observation", ControlRequirement: "control", Priority: "high"}
	outcomes := map[string]QATheoryOutcome{shard.Theories[0].ID: QATheoryInconclusive}
	if _, err := validateQAArbiterEvidenceRequests(qaMap, qaTheoryGroupPlan{ID: "group", Theories: shard.Theories}, []QAArbiterEvidenceRequest{request, request}, outcomes); err == nil {
		t.Fatal("duplicate normalized evidence request was accepted")
	}
	bundle := QATestBundle{ID: "qa-v2-test-aaaaaaaaaaaaaaaaaaaaaaaa", Files: []QATestFile{{Path: "calc_test.go"}}}
	run := QAReproductionRun{Outcome: QAEvidenceFail, ReasonCode: "predicted_failure_reproduced", Result: QACommandResult{StdoutDigest: strings.Repeat("a", 64)}}
	if !applyQAReproductionToTheories(&shard, request, bundle, run) {
		t.Fatal("first evidence was not recorded")
	}
	if applyQAReproductionToTheories(&shard, request, bundle, run) || shard.Theories[0].OutcomeReason != "repeated_evidence_request_without_new_evidence" {
		t.Fatal("repeated request advanced without new evidence")
	}
}

func TestImmutableAuthoredTestFailsBeforeAndPassesAfterRepair(t *testing.T) {
	target := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(target, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.test/calc\n\ngo 1.22\n")
	write("calc.go", "package calc\nfunc Add(a, b int) int { return a }\n")
	qaMap, _, spec := authoredTestFixture(t, target)
	bundle, err := BuildQATestBundle(qaMap.Project, qaMap.Sprint, spec, []QATestFile{{Path: "calc_test.go", Content: "package calc\nimport \"testing\"\nfunc TestAdd(t *testing.T) { if got := Add(1, 1); got != 2 { t.Fatalf(\"got %d want 2\", got) } }\n"}}, "", qaMap.Budgets)
	if err != nil {
		t.Fatal(err)
	}
	run, err := RunQAReproduction(context.Background(), QAReproductionRequest{Project: qaMap.Project, Sprint: qaMap.Sprint, TargetRoot: target, WorkspaceParent: t.TempDir(), ProtectedRoots: []string{target}, Spec: spec, Bundle: bundle, Budgets: qaMap.Budgets, Runner: authoredTestRunner{}, ExpectedTargetID: spec.ImplementationFingerprint})
	if err != nil || run.Outcome != QAEvidenceFail || !run.Cleanup.Complete {
		t.Fatalf("broken run = %+v, %v", run, err)
	}
	write("calc.go", "package calc\nfunc Add(a, b int) int { return a + b }\n")
	current, err := targetIdentity(target)
	if err != nil {
		t.Fatal(err)
	}
	run, err = RunQAReproduction(context.Background(), QAReproductionRequest{Project: qaMap.Project, Sprint: qaMap.Sprint, TargetRoot: target, WorkspaceParent: t.TempDir(), ProtectedRoots: []string{target}, Spec: spec, Bundle: bundle, Budgets: qaMap.Budgets, Runner: authoredTestRunner{}, ExpectedTargetID: current, AllowDifferentImplementation: true})
	if err != nil || run.Outcome != QAEvidencePass {
		t.Fatalf("repaired run = %+v, %v", run, err)
	}
}

func TestIncompleteReproductionCleanupIsRetainedAsInconclusive(t *testing.T) {
	target := t.TempDir()
	for name, content := range map[string]string{
		"go.mod":  "module example.test/calc\n\ngo 1.22\n",
		"calc.go": "package calc\nfunc Add(a, b int) int { return a }\n",
	} {
		if err := os.WriteFile(filepath.Join(target, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	qaMap, _, spec := authoredTestFixture(t, target)
	bundle, err := BuildQATestBundle(qaMap.Project, qaMap.Sprint, spec, []QATestFile{{Path: "calc_test.go", Content: "package calc\n"}}, "", qaMap.Budgets)
	if err != nil {
		t.Fatal(err)
	}
	run, err := RunQAReproduction(context.Background(), QAReproductionRequest{Project: qaMap.Project, Sprint: qaMap.Sprint, TargetRoot: target, WorkspaceParent: t.TempDir(), ProtectedRoots: []string{target}, Spec: spec, Bundle: bundle, Budgets: qaMap.Budgets, Runner: uncleanAuthoredTestRunner{}, ExpectedTargetID: spec.ImplementationFingerprint})
	if err != nil {
		t.Fatal(err)
	}
	if run.Outcome != QAEvidenceInconclusive || run.ReasonCode != "cleanup_uncertain" || run.Cleanup.Complete || run.Cleanup.DescendantsTerminated {
		t.Fatalf("cleanup result = %+v", run)
	}
}

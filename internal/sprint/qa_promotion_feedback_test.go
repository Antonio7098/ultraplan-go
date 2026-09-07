package sprint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

type qaFeedbackRuntime struct {
	t              *testing.T
	authorCalls    int
	arbiterSawTest bool
	unavailable    bool
}

func (r *qaFeedbackRuntime) StartRun(ctx context.Context, req pruntime.Request) (pruntime.Result, error) {
	r.t.Helper()
	result := pruntime.Result{Status: "completed", SessionID: "original-" + req.Metadata["shard"], RuntimeStorePath: req.RuntimeStorePath, Permissions: pruntime.PermissionSummary{Mode: "restricted", Default: "deny"}}
	switch req.Metadata["operation"] {
	case "qa-investigate-evidence-continuation":
		r.authorCalls++
		if r.unavailable {
			return result, errors.New("retained session unavailable")
		}
		if req.SessionAction != "continue" || req.SessionID != result.SessionID {
			r.t.Fatalf("did not continue the original investigator: %+v", req)
		}
		var packet struct {
			Spec QAReproductionSpec `json:"reproduction_spec"`
		}
		if err := json.Unmarshal([]byte(req.Prompt[strings.Index(req.Prompt, "\n\n{")+2:]), &packet); err != nil {
			r.t.Fatal(err)
		}
		file := filepath.Join(req.WorkDir, packet.Spec.ApprovedTestPaths[0])
		content := fmt.Sprintf("package test\nimport \"testing\"\nfunc %s(t *testing.T) { if Probe(0) != 0 { t.Fatal(\"control failed\") }; if got := Probe(1); got != 2 { t.Fatalf(\"%s: got %%d want 2\", got) } }\n", packet.Spec.PredictedFailure.TestName, packet.Spec.PredictedFailure.OutputMatcher)
		if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
			r.t.Fatal(err)
		}
		result.TerminalOutput = "Test authored."
	case "qa-arbitrate":
		var packet struct {
			Theories []QATheory `json:"theories"`
		}
		if err := json.Unmarshal([]byte(req.Prompt[strings.Index(req.Prompt, "Frozen arbiter group pack:\n")+len("Frozen arbiter group pack:\n"):]), &packet); err != nil {
			r.t.Fatal(err)
		}
		output := qaArbiterOutput{SchemaVersion: QASchemaVersion}
		for _, theory := range packet.Theories {
			for _, evidence := range theory.Evidence {
				if strings.Contains(evidence.Summary, "Probe(0)") && strings.Contains(evidence.Summary, "got 1 want 2") {
					r.arbiterSawTest = true
				}
			}
			output.Overrides = append(output.Overrides, QAArbiterOverride{TheoryIDs: []string{theory.ID}, Action: QAArbiterConfirm, Outcome: QATheoryConfirmed, Reason: "source shows the wrong return value", ReasonRefs: []string{theory.ID}, Confidence: .9})
			output.Issues = append(output.Issues, QAArbiterIssue{TheoryIDs: []string{theory.ID}, Claim: theory.Claim, Title: "Probe returns the wrong value", IssueClass: "behavior", Severity: "high", Location: "internal/app/usecases.go", Reason: "the positive input is mishandled", EvidenceRefs: []string{theory.ID}})
		}
		data, _ := json.Marshal(output)
		result.TerminalOutput, result.SessionID = string(data), "arbiter-original"
	case "qa-reconcile-issues":
		start := strings.Index(req.Prompt, "Frozen provisional issue packet:\n") + len("Frozen provisional issue packet:\n")
		var packet struct {
			Issues []QAArbiterIssue `json:"provisional_issues"`
		}
		if err := json.Unmarshal([]byte(req.Prompt[start:]), &packet); err != nil {
			r.t.Fatal(err)
		}
		data, _ := json.Marshal(qaIssueReconcilerOutput{SchemaVersion: QASchemaVersion, Issues: packet.Issues})
		result.TerminalOutput = string(data)
	default:
		base, err := (&qaInvestigatorRuntime{}).StartRun(ctx, req)
		if err != nil {
			return base, err
		}
		var output qaInvestigatorOutput
		if err := json.Unmarshal([]byte(base.TerminalOutput), &output); err != nil {
			r.t.Fatal(err)
		}
		output.Theories[0].Claim = "Probe(1) returns 1 instead of 2"
		output.Theories[0].VerificationSurface = "internal/app/usecases.go:Probe"
		output.Theories[0].ConfirmationCondition = "Probe(1) returns 1"
		output.Theories[0].RefutationCondition = "Probe(1) returns 2"
		output.Theories[0].Outcome = QATheoryConfirmed
		data, _ := json.Marshal(output)
		result.TerminalOutput = string(data)
	}
	return result, nil
}

// Exercise the actual batch, arbiter, continuation, isolated Go execution,
// persistence, re-arbitration and adjudication boundaries with a fake model.
func TestQAPromotionFeedbackReturnsStaticFindingToOriginalInvestigator(t *testing.T) {
	root, sp, target, qaMap, flow, state, token := qaRunFixture(t)
	writeFileContent(t, target, "module example.test/feedback\n\ngo 1.22\n", "go.mod")
	writeFileContent(t, target, "package test\nfunc Probe(n int) int { return n }\n", "internal", "app", "usecases.go")
	identity, err := targetIdentity(target)
	if err != nil {
		t.Fatal(err)
	}
	qaMap.ImplementationFingerprint = identity
	state.Freshness.ImplementationFingerprint = identity
	foundation, err := BuildQAFoundation(ReviewManifest{Project: qaMap.Project, Sprint: qaMap.Sprint}, qaMap.GovernedInputFingerprint, identity, qaMap.ReviewFingerprint, nil, qaMap.Budgets, "Current review")
	if err != nil {
		t.Fatal(err)
	}
	qaMap.Foundation = &foundation
	var selected QAShard
	for _, shard := range qaMap.Shards {
		if shard.Kind == QAShardPrimary && containsQAString(shard.ChangedPaths, "internal/app/usecases.go") {
			selected = shard
			break
		}
	}
	if selected.ID == "" {
		t.Fatal("missing app shard")
	}
	runtime := &qaFeedbackRuntime{t: t}
	settings := QASettings{Runtime: StageRuntime{Model: "openai/qa", Variant: "high"}, Budgets: qaMap.Budgets}
	service := NewService(root).WithRuntime(runtime).WithQASettings(settings).WithQAMapFence(func(QAMap) error { return nil })
	store := NewQAStore(root, sp).WithWriterFence(func(QAWriterToken) error { return nil })
	t.Cleanup(func() { cleanupQAInvestigatorWorkspaces(root, qaMap.SemanticAttemptID) })
	if err := store.Publish(QAPublication{Map: &qaMap, Shards: qaMap.Shards, State: state, Flow: flow}, token); err != nil {
		t.Fatal(err)
	}
	shards, _, err := service.runQAShardBatch(context.Background(), store, flow, qaMap, target, []QAShard{selected}, state, QARunRequest{WriterToken: token, EvidenceProducing: true})
	if err != nil {
		t.Fatal(err)
	}
	workspace := qaInvestigatorWorkspacePath(root, qaMap.SemanticAttemptID, selected.ID)
	if _, err := os.Stat(workspace); err != nil {
		t.Fatalf("batch removed evidence workspace: %v", err)
	}
	arbitration, err := service.arbitrateQA(context.Background(), qaMap, shards, target)
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureQAPromotionRequests(qaMap, &arbitration, shards, nil); err != nil {
		t.Fatal(err)
	}
	if len(arbitration.EvidenceRequests) != 1 {
		t.Fatalf("missing promotion request: %+v", arbitration)
	}
	request := arbitration.EvidenceRequests[0]
	checkpointCount := 0
	checkpoint := func(current []QAShard, tests []QATestPublication, requests []QAArbiterEvidenceRequest) error {
		checkpointCount++
		bundle := QAEvidencePublication{Budgets: qaMap.Budgets, InvestigatorTests: tests, EvidenceRequests: requests}
		return store.Publish(QAPublication{Shards: current, State: state, Flow: flow, Evidence: &bundle}, token)
	}
	shards, tests, progressed, err := service.strengthenQARequestedEvidence(context.Background(), qaMap, target, shards, arbitration.EvidenceRequests, map[string]bool{request.ID: true}, nil, checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	if !progressed || len(tests) != 1 || !qaUsableReproduction(qaMap, tests[0]) || tests[0].Runs[0].Outcome != QAEvidenceFail {
		t.Fatalf("evidence was not produced: progressed=%t tests=%+v requests=%+v", progressed, tests, arbitration.EvidenceRequests)
	}
	if checkpointCount < 2 {
		t.Fatal("request not checkpointed before and after execution")
	}
	loaded, err := store.LoadArbiterEvidenceRequest(qaMap.SemanticAttemptID, request.ID)
	if err != nil || loaded.Attempts != 1 || loaded.Status != "evidence_recorded" {
		t.Fatalf("request checkpoint: %+v %v", loaded, err)
	}
	returned, err := service.arbitrateQAAffected(context.Background(), qaMap, shards, target, &arbitration, map[string]bool{shards[0].Theories[0].ID: true})
	if err != nil {
		t.Fatal(err)
	}
	if !runtime.arbiterSawTest {
		t.Fatal("arbiter did not receive the assertion, control and execution result")
	}
	if err := ensureQAPromotionRequests(qaMap, &returned, shards, tests); err != nil {
		t.Fatal(err)
	}
	if len(returned.EvidenceRequests) != 0 {
		t.Fatal("satisfied finding was sent for evidence again")
	}
	// Use the real publication/adjudication path to prove the finding promotes.
	status := VerificationStatus{Review: VerificationStage{Fresh: true, ExecutionStatus: string(ReviewCompleted), Verdict: string(ReviewPass)}}
	publication, _, err := service.buildQAEvidencePublicationAdmitted(context.Background(), sp, qaMap, target, nil, returned.Issues, tests, []QAArbiterEvidenceRequest{loaded}, nil, status, identity)
	if err != nil {
		t.Fatal(err)
	}
	if len(publication.Adjudication.Issues) != 1 || len(publication.Adjudication.Unpromoted) != 0 {
		t.Fatalf("adjudication: %+v", publication.Adjudication)
	}
	dismissed, _, err := service.buildQAEvidencePublicationAdmitted(context.Background(), sp, qaMap, target, nil, nil, tests, nil, nil, status, identity)
	if err != nil || len(dismissed.Adjudication.Issues) != 0 {
		t.Fatalf("retained failure resurrected dismissed theory: %v", err)
	}
	_, again, progressed, err := service.strengthenQARequestedEvidence(context.Background(), qaMap, target, shards, []QAArbiterEvidenceRequest{loaded}, map[string]bool{request.ID: true}, tests, checkpoint)
	if err != nil || progressed || len(again) != 0 || runtime.authorCalls != 1 {
		t.Fatalf("resume duplicated satisfied work: %v", err)
	}
	// A new, stronger request must reach the same original session. Failure to
	// resume that session is an operational blocker, not a refutation.
	stronger := request
	stronger.ID = "qa-v2-request-bbbbbbbbbbbbbbbbbbbbbbbb"
	stronger.Gap = "prove the caller also reaches this branch"
	runtime.unavailable = true
	requests := []QAArbiterEvidenceRequest{loaded, stronger}
	_, more, progressed, err := service.strengthenQARequestedEvidence(context.Background(), qaMap, target, shards, requests, map[string]bool{stronger.ID: true}, tests, checkpoint)
	if err != nil || progressed || len(more) != 0 || requests[1].ReasonCode != "original_session_unavailable" || runtime.authorCalls != 2 {
		t.Fatalf("session failure was not retained: %+v %v", requests, err)
	}
	requests, err = store.ListArbiterEvidenceRequests(qaMap.SemanticAttemptID)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, err = service.strengthenQARequestedEvidence(context.Background(), qaMap, target, shards, requests, map[string]bool{stronger.ID: true}, tests, checkpoint)
	if err != nil || runtime.authorCalls != 2 {
		t.Fatal("resume reset the consumed shard budget")
	}
	for _, retained := range requests {
		if retained.ID == stronger.ID && retained.ReasonCode != "evidence_round_budget_exhausted" {
			t.Fatalf("missing budget blocker: %+v", retained)
		}
	}
	// Completed run cleanup is recoverable without creating a new session.
	if facts := cleanupQAInvestigatorWorkspaces(root, qaMap.SemanticAttemptID); !facts.Complete {
		t.Fatal("cleanup failed")
	}
	if err := restoreQAInvestigatorEvidenceWorkspace(context.Background(), root, target, qaMap, shards[0], tests); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(workspace, tests[0].Bundle.Files[0].Path))
	if err != nil || string(content) != tests[0].Bundle.Files[0].Content {
		t.Fatal("resume did not restore the immutable test")
	}
}

func TestQAPromotionCoverageRequestsAllElevenUncoveredCandidates(t *testing.T) {
	target := t.TempDir()
	writeFileContent(t, target, "package calc\n", "calc.go")
	qaMap, shard, spec := authoredTestFixture(t, target)
	bundle, err := BuildQATestBundle(qaMap.Project, qaMap.Sprint, spec, []QATestFile{{Path: "calc_test.go", Content: "package calc\n"}}, "", qaMap.Budgets)
	if err != nil {
		t.Fatal(err)
	}
	run := QAReproductionRun{SchemaVersion: QAEvidenceSchemaVersion, ID: "qa-v2-run-aaaaaaaaaaaaaaaaaaaaaaaa", SpecID: spec.ID, TestBundleID: bundle.ID, TargetIdentity: qaMap.ImplementationFingerprint, Signature: spec.PredictedFailure, Outcome: QAEvidenceFail, ReasonCode: "predicted_failure_reproduced", CompletedAt: time.Unix(101, 0), Result: QACommandResult{ExitCode: 1, CleanupComplete: true, Stdout: "--- FAIL: TestAdd\ngot 1 want 2"}, Cleanup: QACleanupFacts{Attempted: true, Complete: true, DescendantsTerminated: true, WorkspaceRemoved: true}}
	tests := []QATestPublication{{Spec: spec, Bundle: bundle, Runs: []QAReproductionRun{run}}}
	group := QAArbiterGroup{ID: "qa-v1-arbiter-group-aaaaaaaaaaaaaaaaaaaaaaaa"}
	for i := 0; i < 12; i++ {
		theory := shard.Theories[0]
		if i != 0 {
			theory.ID, _ = NewQATheoryID(qaMap.Project, qaMap.Sprint, shard.ID, QATheoryIdentity{Claim: fmt.Sprintf("claim %d", i), Basis: "static", VerificationSurface: "calc.go"})
			shard.Theories = append(shard.Theories, theory)
		}
		group.Issues = append(group.Issues, QAArbiterIssue{TheoryIDs: []string{theory.ID}})
	}
	arbitration := QAArbitration{Groups: []QAArbiterGroup{group}}
	if err := ensureQAPromotionRequests(qaMap, &arbitration, []QAShard{shard}, tests); err != nil {
		t.Fatal(err)
	}
	if len(arbitration.EvidenceRequests) != 11 {
		t.Fatalf("requests=%d want 11", len(arbitration.EvidenceRequests))
	}
	if err := ensureQAPromotionRequests(qaMap, &arbitration, []QAShard{shard}, tests); err != nil {
		t.Fatal(err)
	}
	if len(arbitration.EvidenceRequests) != 11 {
		t.Fatal("coverage check duplicated requests")
	}
	// A merged candidate must still request the uncovered member.
	arbitration = QAArbitration{Groups: []QAArbiterGroup{{ID: group.ID, Issues: []QAArbiterIssue{{TheoryIDs: []string{shard.Theories[0].ID, shard.Theories[1].ID}}}}}}
	if err := ensureQAPromotionRequests(qaMap, &arbitration, []QAShard{shard}, tests); err != nil {
		t.Fatal(err)
	}
	if len(arbitration.EvidenceRequests) != 1 || arbitration.EvidenceRequests[0].TheoryIDs[0] != shard.Theories[1].ID {
		t.Fatal("merged finding lost uncovered theory")
	}
}

func TestQAEvidenceFeedbackPersistsBlockersAndResumeBudgets(t *testing.T) {
	target := t.TempDir()
	writeFileContent(t, target, "package calc\n", "calc.go")
	qaMap, shard, _ := authoredTestFixture(t, target)
	shard.Theories[0].Outcome = QATheoryConfirmed
	request := QAArbiterEvidenceRequest{ID: "qa-v2-request-aaaaaaaaaaaaaaaaaaaaaaaa", OriginShardID: shard.ID, TheoryIDs: []string{shard.Theories[0].ID}}
	service := NewService(t.TempDir())
	checkpoint := func(_ []QAShard, _ []QATestPublication, _ []QAArbiterEvidenceRequest) error { return nil }
	requests := []QAArbiterEvidenceRequest{request}
	_, _, progressed, err := service.strengthenQARequestedEvidence(context.Background(), qaMap, target, []QAShard{shard}, requests, map[string]bool{request.ID: true}, nil, checkpoint)
	if err != nil || progressed || requests[0].ReasonCode != "original_session_unavailable" {
		t.Fatalf("missing session: %+v %v", requests, err)
	}
	if shard.Theories[0].Outcome != QATheoryConfirmed {
		t.Fatal("operational blocker erased static finding")
	}
	shard.Attempts = []QAInvestigatorAttempt{{ID: "original", Number: 1}}
	requests = []QAArbiterEvidenceRequest{request}
	requests[0].EvidenceRound = qaMap.Budgets.EvidenceRoundsPerShard
	requests[0].Attempts = 1
	_, _, progressed, err = service.strengthenQARequestedEvidence(context.Background(), qaMap, target, []QAShard{shard}, requests, map[string]bool{request.ID: true}, nil, checkpoint)
	if err != nil || progressed || requests[0].ReasonCode != "evidence_round_budget_exhausted" {
		t.Fatalf("resume reset budget: %+v %v", requests, err)
	}
	final := finalizeQAArbiterEvidenceRequests(requests, nil, []QAShard{shard})
	if final[0].ReasonCode != requests[0].ReasonCode {
		t.Fatal("finalization erased stopping reason")
	}
}

func TestQAEvidenceResultCannotSatisfyDifferentRequest(t *testing.T) {
	request := QAArbiterEvidenceRequest{ID: "qa-v2-request-aaaaaaaaaaaaaaaaaaaaaaaa", OriginShardID: "shard", TheoryIDs: []string{"theory"}, Status: "pending"}
	test := QATestPublication{Spec: QAReproductionSpec{EvidenceRequestID: "qa-v2-request-bbbbbbbbbbbbbbbbbbbbbbbb", ShardID: "shard", TheoryIDs: []string{"theory"}}, Runs: []QAReproductionRun{{Outcome: QAEvidenceFail}}}
	if qaTestAnswersRequest(test, request) {
		t.Fatal("same theory closed a different request")
	}
	final := finalizeQAArbiterEvidenceRequests([]QAArbiterEvidenceRequest{request}, []QATestPublication{test}, nil)
	if final[0].Status != "pending" {
		t.Fatal("older reproduction satisfied a stronger request")
	}
	test.Spec.EvidenceRequestID = ""
	if qaTestAnswersRequest(test, request) {
		t.Fatal("unbound legacy test closed a new request")
	}
}

func TestQAEvidenceAttemptReservationSurvivesInterruption(t *testing.T) {
	target := t.TempDir()
	writeFileContent(t, target, "package calc\n", "calc.go")
	qaMap, shard, _ := authoredTestFixture(t, target)
	qaMap.Budgets.EvidenceRoundsPerShard = 1
	shard.Attempts = []QAInvestigatorAttempt{{ID: "original", Number: 1}}
	request := QAArbiterEvidenceRequest{ID: "qa-v2-request-aaaaaaaaaaaaaaaaaaaaaaaa", OriginShardID: shard.ID, TheoryIDs: []string{shard.Theories[0].ID}}
	service := NewService(t.TempDir())
	interrupted := errors.New("process interrupted after checkpoint")
	var saved []byte
	_, _, _, err := service.strengthenQARequestedEvidence(context.Background(), qaMap, target, []QAShard{shard}, []QAArbiterEvidenceRequest{request}, map[string]bool{request.ID: true}, nil, func(_ []QAShard, _ []QATestPublication, requests []QAArbiterEvidenceRequest) error {
		saved, _ = json.Marshal(requests)
		return interrupted
	})
	if !errors.Is(err, interrupted) {
		t.Fatalf("checkpoint failure=%v", err)
	}
	var resumed []QAArbiterEvidenceRequest
	if err := json.Unmarshal(saved, &resumed); err != nil {
		t.Fatal(err)
	}
	if resumed[0].Attempts != 1 || resumed[0].Status != "running" {
		t.Fatal("attempt was not reserved before execution")
	}
	_, _, progressed, err := service.strengthenQARequestedEvidence(context.Background(), qaMap, target, []QAShard{shard}, resumed, map[string]bool{request.ID: true}, nil, func([]QAShard, []QATestPublication, []QAArbiterEvidenceRequest) error { return nil })
	if err != nil || progressed || resumed[0].ReasonCode != "evidence_round_budget_exhausted" {
		t.Fatal("interruption reset budget")
	}
}

func TestRequestedReproductionCannotRefuteWithSkippedOrAbsentTest(t *testing.T) {
	spec := QAReproductionSpec{EvidenceRequestID: "qa-v2-request-aaaaaaaaaaaaaaaaaaaaaaaa", Command: QACheckDescriptor{Executable: "go"}, PredictedFailure: QAFailureSignature{TestName: "TestClaim", Assertion: "expected result", OutputMatcher: "failure", ExitClass: "nonzero"}}
	for _, output := range []string{"ok example.test [no tests to run]", "--- SKIP: TestClaim (0.00s)\nPASS", "--- PASS: TestOther (0.00s)\nPASS", ""} {
		outcome, _ := classifyQARequestedReproduction(QACommandResult{ExitCode: 0, CleanupComplete: true, Stdout: output}, spec)
		if outcome != QAEvidenceInconclusive {
			t.Fatalf("nonexecuted test refuted claim: %q", output)
		}
	}
	outcome, _ := classifyQARequestedReproduction(QACommandResult{ExitCode: 0, CleanupComplete: true, Stdout: "--- PASS: TestClaim (0.00s)\nPASS"}, spec)
	if outcome != QAEvidencePass {
		t.Fatal("executed passing test was not accepted")
	}
}

func TestQAUnverifiableEvidenceRequiresReasonAndPrerequisite(t *testing.T) {
	for _, output := range []string{`{"status":"unverifiable"}`, `{"status":"unverifiable","reason":"cannot reach service"}`, `{"status":"refuted","reason":"cannot reach service","required_prerequisite":"service fixture"}`} {
		if qaUnverifiableEvidenceReason(output) != "" {
			t.Fatalf("accepted incomplete refusal: %s", output)
		}
	}
	reason := qaUnverifiableEvidenceReason(`{"status":"unverifiable","reason":"the production API has no local fixture","required_prerequisite":"a contained API simulator"}`)
	if !strings.Contains(reason, "no local fixture") || !strings.Contains(reason, "contained API simulator") {
		t.Fatalf("lost unverifiable explanation: %s", reason)
	}
}

func TestQAEvidenceReplacementRequiresItsOwnSuccessfulObservation(t *testing.T) {
	target := t.TempDir()
	writeFileContent(t, target, "package calc\n", "calc.go")
	qaMap, shard, spec := authoredTestFixture(t, target)
	old := QAArbiterEvidenceRequest{ID: "qa-v2-request-aaaaaaaaaaaaaaaaaaaaaaaa", OriginShardID: shard.ID, TheoryIDs: spec.TheoryIDs, Status: "inconclusive", ReasonCode: "failure_signature_mismatch"}
	replacement := old
	replacement.ID, replacement.Status, replacement.ReasonCode = "qa-v2-request-bbbbbbbbbbbbbbbbbbbbbbbb", "pending", ""
	history := []QAArbiterEvidenceRequest{old}
	linkQAReplacementRequests(history, []QAArbiterEvidenceRequest{replacement})
	if history[0].SupersededBy != replacement.ID {
		t.Fatal("replacement lineage missing")
	}
	history = append(history, replacement)
	final := finalizeQAArbiterEvidenceRequests(history, nil, []QAShard{shard})
	if final[0].Status != "inconclusive" || final[1].Status != "pending" {
		t.Fatal("unanswered replacement cleared a blocker")
	}
	spec.EvidenceRequestID = replacement.ID
	spec, err := FreezeQAReproductionSpec(qaMap.Project, qaMap.Sprint, spec, qaMap.Budgets, spec.FrozenAt)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := BuildQATestBundle(qaMap.Project, qaMap.Sprint, spec, []QATestFile{{Path: "calc_test.go", Content: "package calc\n"}}, "", qaMap.Budgets)
	if err != nil {
		t.Fatal(err)
	}
	run := QAReproductionRun{SchemaVersion: QAEvidenceSchemaVersion, ID: "qa-v2-run-aaaaaaaaaaaaaaaaaaaaaaaa", SpecID: spec.ID, TestBundleID: bundle.ID, TargetIdentity: qaMap.ImplementationFingerprint, Signature: spec.PredictedFailure, Outcome: QAEvidenceFail, ReasonCode: "predicted_failure_reproduced", CompletedAt: time.Unix(101, 0), Result: QACommandResult{ExitCode: 1, CleanupComplete: true, Stdout: "--- FAIL: TestAdd\ngot 1 want 2"}, Cleanup: QACleanupFacts{Attempted: true, Complete: true, DescendantsTerminated: true, WorkspaceRemoved: true}}
	final = finalizeQAArbiterEvidenceRequests(history, []QATestPublication{{Spec: spec, Bundle: bundle, Runs: []QAReproductionRun{run}}}, []QAShard{shard})
	if final[0].Status != "superseded" || final[1].Status != "evidence_recorded" {
		t.Fatalf("answered replacement left stale blocker: %+v", final)
	}
	// Repeating an older request cannot create a cycle through its replacement.
	linkQAReplacementRequests(history, []QAArbiterEvidenceRequest{old})
	if history[1].SupersededBy != "" {
		t.Fatal("request replacement cycle created")
	}
}

func TestQAPromotionFeedbackIncludesInconclusiveTheoriesWithoutRequests(t *testing.T) {
	target := t.TempDir()
	writeFileContent(t, target, "package calc\n", "calc.go")
	qaMap, shard, _ := authoredTestFixture(t, target)
	arbitration := QAArbitration{Groups: []QAArbiterGroup{{ID: "qa-v1-arbiter-group-aaaaaaaaaaaaaaaaaaaaaaaa", TheoryIDs: []string{shard.Theories[0].ID}}}}
	if err := ensureQAPromotionRequests(qaMap, &arbitration, []QAShard{shard}, nil); err != nil {
		t.Fatal(err)
	}
	if len(arbitration.EvidenceRequests) != 1 {
		t.Fatal("inconclusive theory escaped evidence collection")
	}
}

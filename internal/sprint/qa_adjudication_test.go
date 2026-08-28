package sprint

import (
	"strings"
	"testing"
	"time"
)

func TestQAAdjudicationPromotesOnlyCurrentContainedRepeatableEvidence(t *testing.T) {
	budgets := DefaultQABudgets()
	attemptID, _ := NewQASemanticAttemptID("alpha", "37-evidence", QASemanticIdentity{ChangedPaths: []string{"a.go"}})
	shardID, _ := NewQAShardID("alpha", "37-evidence", attemptID, QAShardIdentity{Kind: QAShardPrimary, ChangedPaths: []string{"a.go"}, BehavioralConcerns: []string{"behavior"}, ExpectationRefs: []string{"REQ-1"}})
	plan, err := FreezeQAEvidencePlan("alpha", "37-evidence", QAEvidencePlan{AttemptID: attemptID, ShardID: shardID, ExpectationRefs: []string{"REQ-1"}, Kind: QACheckBehavioral, ConfirmationCondition: "command fails", RefutationCondition: "command passes", InconclusiveCondition: "command cannot run", ApprovedPaths: []string{"a.go"}, Executable: "go", Args: []string{"test", "./..."}, Timeout: time.Minute, OutputLimit: 1024, CleanupRequired: true, GovernedInputFingerprint: testQAFingerprint, ImplementationFingerprint: strings.Repeat("b", 64), MapFingerprint: strings.Repeat("c", 64)}, budgets, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	evidenceID, _ := NewQAV2ID("evidence", "alpha", "37-evidence", plan.ID, "failure")
	record := QAEvidenceRecord{SchemaVersion: 2, ID: evidenceID, PlanID: plan.ID, AttemptID: attemptID, ShardID: shardID, WorkspaceID: "opaque", WorkspaceIdentity: strings.Repeat("d", 64), TargetIdentityBefore: strings.Repeat("e", 64), TargetIdentityAfter: strings.Repeat("e", 64), GovernedInputFingerprint: plan.GovernedInputFingerprint, ImplementationFingerprint: plan.ImplementationFingerprint, MapFingerprint: plan.MapFingerprint, Commands: []QACommandResult{{Executable: "go", ArgsDigest: strings.Repeat("f", 64), ExitCode: 1}}, Outcome: QAEvidenceFail, ReasonCode: "assertion_failed", Repeatable: true, Contained: true, Cleanup: QACleanupFacts{Attempted: true, DescendantsTerminated: true, WorkspaceRemoved: true, Complete: true}, CompletedAt: time.Unix(2, 0)}
	result, err := AdjudicateQA(QAAdjudicationRequest{Project: "alpha", Sprint: "37-evidence", AttemptID: attemptID, MapFingerprint: plan.MapFingerprint, Plans: []QAEvidencePlan{plan}, Evidence: []QAEvidenceRecord{record}, Candidates: []QAIssueCandidate{{Claim: "request fails", Title: "Valid request fails", IssueClass: "behavior", Severity: "high", Location: "internal/a.go", EvidenceIDs: []string{record.ID}, RepairEligible: true, RegressionCandidate: true}}, Budgets: budgets, Now: time.Unix(3, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.AcceptedIDs) != 1 || len(result.Issues) != 1 || !result.Issues[0].RegressionCandidate {
		t.Fatalf("adjudication = %+v", result)
	}
	record.Cleanup.Complete = false
	blocked, err := AdjudicateQA(QAAdjudicationRequest{Project: "alpha", Sprint: "37-evidence", AttemptID: attemptID, MapFingerprint: plan.MapFingerprint, Plans: []QAEvidencePlan{plan}, Evidence: []QAEvidenceRecord{record}, Candidates: []QAIssueCandidate{{Claim: "request fails", Title: "Valid request fails", IssueClass: "behavior", Location: "internal/a.go", EvidenceIDs: []string{record.ID}}}, Budgets: budgets, Now: time.Unix(3, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if len(blocked.Issues) != 0 || len(blocked.Rejected) != 1 {
		t.Fatalf("unsafe evidence was promoted: %+v", blocked)
	}
}

func TestQAFailedShardRequiresThreeFreshCompleteEvaluators(t *testing.T) {
	digest := strings.Repeat("a", 64)
	results := []QAModelObservation{
		{CallID: "1", SessionID: "s1", EvidenceDigest: digest, Outcome: QAEvidencePass, Valid: true},
		{CallID: "2", SessionID: "s2", EvidenceDigest: digest, Outcome: QAEvidencePass, Valid: true},
		{CallID: "3", SessionID: "s3", EvidenceDigest: digest, Outcome: QAEvidenceFail, Valid: true},
	}
	if got, err := AdjudicateFailedShard(digest, results); err != nil || got != QAEvidencePass {
		t.Fatalf("majority = %s, %v", got, err)
	}
	results[2].Valid = false
	if got, err := AdjudicateFailedShard(digest, results); err == nil || got != QAEvidenceBlocked {
		t.Fatalf("incomplete evaluator = %s, %v", got, err)
	}
}

func TestQAAdjudicationAdmitsDeterministicallySufficientFactFailure(t *testing.T) {
	budgets := DefaultQABudgets()
	attemptID, _ := NewQASemanticAttemptID("alpha", "37-evidence", QASemanticIdentity{ChangedPaths: []string{"a.go"}})
	shardID, _ := NewQAShardID("alpha", "37-evidence", attemptID, QAShardIdentity{Kind: QAShardPrimary, ChangedPaths: []string{"a.go"}, BehavioralConcerns: []string{"behavior"}, ExpectationRefs: []string{"REQ-1"}})
	plan, err := FreezeQAEvidencePlan("alpha", "37-evidence", QAEvidencePlan{AttemptID: attemptID, ShardID: shardID, ExpectationRefs: []string{"REQ-1"}, Kind: QACheckFact, ConfirmationCondition: "command passes", RefutationCondition: "command fails", InconclusiveCondition: "command cannot run", ApprovedPaths: []string{"a.go"}, Executable: "go", Args: []string{"test", "./..."}, Timeout: time.Minute, OutputLimit: 1024, CleanupRequired: true, GovernedInputFingerprint: testQAFingerprint, ImplementationFingerprint: strings.Repeat("b", 64), MapFingerprint: strings.Repeat("c", 64)}, budgets, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	evidenceID, _ := NewQAV2ID("evidence", "alpha", "37-evidence", plan.ID, "deterministic-failure")
	record := QAEvidenceRecord{SchemaVersion: 2, ID: evidenceID, PlanID: plan.ID, AttemptID: attemptID, ShardID: shardID, WorkspaceID: "opaque", WorkspaceIdentity: strings.Repeat("d", 64), TargetIdentityBefore: strings.Repeat("e", 64), TargetIdentityAfter: strings.Repeat("e", 64), GovernedInputFingerprint: plan.GovernedInputFingerprint, ImplementationFingerprint: plan.ImplementationFingerprint, MapFingerprint: plan.MapFingerprint, Commands: []QACommandResult{{Executable: "go", ArgsDigest: strings.Repeat("f", 64), ExitCode: 1}}, Outcome: QAEvidenceFail, ReasonCode: "command_failed", Contained: true, Cleanup: QACleanupFacts{Attempted: true, DescendantsTerminated: true, WorkspaceRemoved: true, Complete: true}, CompletedAt: time.Unix(2, 0)}
	result, err := AdjudicateQA(QAAdjudicationRequest{Project: "alpha", Sprint: "37-evidence", AttemptID: attemptID, MapFingerprint: plan.MapFingerprint, Plans: []QAEvidencePlan{plan}, Evidence: []QAEvidenceRecord{record}, Candidates: []QAIssueCandidate{{Claim: "approved check fails", Title: "Approved check fails", IssueClass: "behavior", EvidenceIDs: []string{record.ID}}}, Budgets: budgets, Now: time.Unix(3, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.AcceptedIDs) != 1 || len(result.Issues) != 1 || len(result.Rejected) != 0 {
		t.Fatalf("deterministically sufficient failure was not promoted: %+v", result)
	}
}

func TestQAAdjudicationDeduplicatesCandidatesWithOneRootCause(t *testing.T) {
	budgets := DefaultQABudgets()
	attemptID, _ := NewQASemanticAttemptID("alpha", "38-repair", QASemanticIdentity{ChangedPaths: []string{"a.go"}})
	shardID, _ := NewQAShardID("alpha", "38-repair", attemptID, QAShardIdentity{Kind: QAShardPrimary, ChangedPaths: []string{"a.go"}, BehavioralConcerns: []string{"formatting"}, ExpectationRefs: []string{"REQ-1"}})
	plan, err := FreezeQAEvidencePlan("alpha", "38-repair", QAEvidencePlan{AttemptID: attemptID, ShardID: shardID, ExpectationRefs: []string{"REQ-1"}, Kind: QACheckFact, ConfirmationCondition: "gofmt passes", RefutationCondition: "gofmt fails", InconclusiveCondition: "gofmt cannot run", ApprovedPaths: []string{"a.go"}, Executable: "gofmt", Args: []string{"-l", "a.go"}, Timeout: time.Minute, OutputLimit: 1024, CleanupRequired: true, GovernedInputFingerprint: testQAFingerprint, ImplementationFingerprint: strings.Repeat("b", 64), MapFingerprint: strings.Repeat("c", 64)}, budgets, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	records := make([]QAEvidenceRecord, 2)
	for i := range records {
		evidenceID, _ := NewQAV2ID("evidence", "alpha", "38-repair", plan.ID, i)
		records[i] = QAEvidenceRecord{SchemaVersion: 2, ID: evidenceID, PlanID: plan.ID, AttemptID: attemptID, ShardID: shardID, WorkspaceID: "opaque", WorkspaceIdentity: strings.Repeat("d", 64), TargetIdentityBefore: strings.Repeat("e", 64), TargetIdentityAfter: strings.Repeat("e", 64), GovernedInputFingerprint: plan.GovernedInputFingerprint, ImplementationFingerprint: plan.ImplementationFingerprint, MapFingerprint: plan.MapFingerprint, Commands: []QACommandResult{{Executable: "gofmt", ArgsDigest: strings.Repeat("f", 64), ExitCode: 1}}, Outcome: QAEvidenceFail, ReasonCode: "command_failed", Contained: true, Cleanup: QACleanupFacts{Attempted: true, DescendantsTerminated: true, WorkspaceRemoved: true, Complete: true}, CompletedAt: time.Unix(int64(2+i), 0)}
	}
	candidates := []QAIssueCandidate{
		{Claim: "approved gofmt check fails", Title: "Formatting check failed (shard B)", IssueClass: "source_integrity", Severity: "low", Location: "a.go", EvidenceIDs: []string{records[1].ID}, RepairEligible: true},
		{Claim: "approved gofmt check fails", Title: "Formatting check failed (shard A)", IssueClass: "source_integrity", Severity: "high", Location: "a.go", EvidenceIDs: []string{records[0].ID}, RegressionCandidate: true},
	}
	result, err := AdjudicateQA(QAAdjudicationRequest{Project: "alpha", Sprint: "38-repair", AttemptID: attemptID, MapFingerprint: plan.MapFingerprint, Plans: []QAEvidencePlan{plan}, Evidence: records, Candidates: candidates, Budgets: budgets, Now: time.Unix(4, 0), RepairAssignmentMode: "grouped", IssuesPerRepairAgent: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Groups) != 1 || len(result.Issues) != 1 || len(result.RepairAssignments) != 1 {
		t.Fatalf("duplicate root cause was not collapsed: %+v", result)
	}
	issue := result.Issues[0]
	if len(issue.EvidenceIDs) != 2 || issue.Title != "Formatting check failed (shard A)" || issue.Severity != "high" || !issue.RepairEligible || !issue.RegressionCandidate {
		t.Fatalf("deduplicated issue did not preserve aggregate facts: %+v", issue)
	}
}

func TestPlanRepairAssignmentsPreservesIssueScopedRuns(t *testing.T) {
	issues := []QAIssue{
		{ID: "issue-a", RootCauseGroupID: "root-a", RepairEligible: true},
		{ID: "issue-b", RootCauseGroupID: "root-a", RepairEligible: true},
		{ID: "issue-c", RootCauseGroupID: "root-b", RepairEligible: true},
	}
	adjudication := QAAdjudication{Issues: issues, RepairGroups: []QARepairIssueGroup{
		{ID: "group-a", IssueIDs: []string{"issue-a", "issue-b"}, Reason: "same root cause"},
		{ID: "group-b", IssueIDs: []string{"issue-c"}, Reason: "same root cause"},
	}}
	grouped, err := PlanRepairAssignments(adjudication, "grouped", 2)
	if err != nil || len(grouped) != 2 || len(grouped[0].Issues) != 2 || len(grouped[1].Issues) != 1 {
		t.Fatalf("grouped assignments = %+v, %v", grouped, err)
	}
	perIssue, err := PlanRepairAssignments(adjudication, "per_issue", 1)
	if err != nil || len(perIssue) != 3 {
		t.Fatalf("per-issue assignments = %+v, %v", perIssue, err)
	}
	for _, assignment := range perIssue {
		if len(assignment.Issues) != 1 {
			t.Fatalf("assignment combined issue runs: %+v", assignment)
		}
	}
}

func TestQAAdmissionFailsClosed(t *testing.T) {
	valid := QAAdmission{ReviewCurrent: true, ReviewVerdict: string(ReviewPassWithFindings), SmokeCurrent: true, SmokeVerdict: string(SmokePass), ContainingSmoke: true, ReadOnlyProofs: []string{"map", "cancellation", "resume"}, MapComplete: true, IsolationProven: true, WritableConcurrency: 1}
	if err := ValidateQAAdmission(valid); err != nil {
		t.Fatal(err)
	}
	valid.IsolationProven = false
	if err := ValidateQAAdmission(valid); err == nil {
		t.Fatal("unproven isolation admitted")
	}
}

func TestDeriveQAAssessmentNextActionMatchesPromotedIssues(t *testing.T) {
	review := VerificationStage{Fresh: true, ExecutionStatus: string(ReviewCompleted), Verdict: string(ReviewPassWithFindings)}
	evidence := []QAEvidenceRecord{{ID: "evidence"}}
	adjudication := QAAdjudication{AcceptedIDs: []string{"evidence"}}

	assessment, next := DeriveQAAssessment(review, evidence, adjudication, nil, nil)
	if assessment != AssessmentPassWithFindings || next != "Review the current Conformance Review findings." {
		t.Fatalf("review-only findings assessment=%q next=%q", assessment, next)
	}

	adjudication.Issues = []QAIssue{{ID: "issue"}}
	assessment, next = DeriveQAAssessment(review, evidence, adjudication, nil, nil)
	if assessment != AssessmentPassWithFindings || next != "Review the promoted issues before governed repair." {
		t.Fatalf("promoted issue assessment=%q next=%q", assessment, next)
	}
}

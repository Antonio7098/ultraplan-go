package sprint

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestQAReplayIdentityAllowsPolicyMigrationOnly(t *testing.T) {
	retained := QAMap{
		Project: "alpha", Sprint: "39-performance-stage",
		GovernedInputFingerprint: strings.Repeat("a", 64), ImplementationFingerprint: strings.Repeat("b", 64),
		ReviewFingerprint: strings.Repeat("c", 64), CheckCatalogFingerprint: strings.Repeat("d", 64),
		PolicyFingerprint: strings.Repeat("e", 64), Target: QATargetIdentity{Fingerprint: strings.Repeat("f", 64)},
	}
	current := retained
	current.PolicyFingerprint = strings.Repeat("1", 64)
	if err := validateQAReplayIdentity(retained, current); err != nil {
		t.Fatalf("policy-only migration was rejected: %v", err)
	}
	current.ImplementationFingerprint = strings.Repeat("2", 64)
	if err := validateQAReplayIdentity(retained, current); err == nil {
		t.Fatal("implementation drift was accepted")
	}
}

func TestQAReplayRetainsEveryArbiterCandidate(t *testing.T) {
	attemptID, _ := NewQASemanticAttemptID("alpha", "39-performance-stage", QASemanticIdentity{ChangedPaths: []string{"a.go"}})
	issues := make([]QAArbiterIssue, 12)
	for i := range issues {
		theoryID, _ := NewQATheoryID("alpha", "39-performance-stage", attemptID, QATheoryIdentity{Claim: fmt.Sprintf("claim %d", i), Basis: "source", VerificationSurface: "a.go"})
		issues[i] = QAArbiterIssue{ID: fmt.Sprintf("qa-v1-arbiter-issue-%024x", i+1), TheoryIDs: []string{theoryID}, Claim: fmt.Sprintf("claim %d", i), Title: fmt.Sprintf("Candidate %d", i), IssueClass: "behavior", Severity: "medium", Location: "a.go"}
	}
	candidates := buildQARetainedCandidates(issues, nil, nil, nil)
	if len(candidates) != 12 {
		t.Fatalf("retained candidates = %d, want 12", len(candidates))
	}
	result, err := AdjudicateQA(QAAdjudicationRequest{Project: "alpha", Sprint: "39-performance-stage", AttemptID: attemptID, MapFingerprint: strings.Repeat("a", 64), Candidates: candidates, Budgets: DefaultQABudgets(), Now: time.Unix(1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Issues) != 0 || len(result.Unpromoted) != 12 {
		t.Fatalf("replay coverage = %d promoted, %d unpromoted", len(result.Issues), len(result.Unpromoted))
	}
}

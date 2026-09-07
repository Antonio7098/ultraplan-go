package sprint

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestQARenderReportIsDeterministicAndEscapesHostileText(t *testing.T) {
	attemptID, _ := NewQASemanticAttemptID("alpha", "37-evidence", QASemanticIdentity{ChangedPaths: []string{"a.go"}})
	assessmentID, _ := NewQAV2ID("assessment", "alpha", "37-evidence", attemptID, "pass")
	assessment := QAAssessmentRecord{SchemaVersion: 2, ID: assessmentID, AttemptID: attemptID, ReviewVerdict: ReviewPass, ReviewFingerprint: testQAFingerprint, Assessment: AssessmentPassWithFindings, EvidenceTotal: 1, IssueTotal: 1, NextAction: "Inspect `issue`\nthen stop", CompletedAt: time.Unix(2, 0)}
	adjudicationID, _ := NewQAV2ID("adjudication", "alpha", "37-evidence", attemptID, "one")
	issueID, _ := NewQAV2ID("issue", "alpha", "37-evidence", attemptID, "one")
	adjudication := QAAdjudication{SchemaVersion: 2, ID: adjudicationID, AttemptID: attemptID, MapFingerprint: strings.Repeat("b", 64), AcceptedIDs: []string{"evidence"}, Issues: []QAIssue{{ID: issueID, Title: "hostile <script>", Severity: "medium", Location: "a.go", EvidenceIDs: []string{"evidence"}}}, CompletedAt: time.Unix(2, 0)}
	evidence := []QAEvidenceRecord{{ID: "evidence", Outcome: QAEvidenceFail, ReasonCode: "assertion", Contained: true, Cleanup: QACleanupFacts{Complete: true}}}
	first, err := RenderQAReport("alpha", "37-evidence", testQAFingerprint, evidence, adjudication, assessment)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := RenderQAReport("alpha", "37-evidence", testQAFingerprint, evidence, adjudication, assessment)
	if !bytes.Equal(first, second) || !bytes.HasPrefix(first, []byte("# QA\n")) || bytes.Contains(first, []byte("`issue`")) {
		t.Fatalf("report is unstable or unsafe:\n%s", first)
	}
}

func TestQAReportCountsPromotedAndUnpromotedCandidates(t *testing.T) {
	attemptID, _ := NewQASemanticAttemptID("alpha", "39-coverage", QASemanticIdentity{ChangedPaths: []string{"a.go"}})
	assessmentID, _ := NewQAV2ID("assessment", "alpha", "39-coverage", attemptID, "blocked")
	adjudicationID, _ := NewQAV2ID("adjudication", "alpha", "39-coverage", attemptID, "coverage")
	issueID, _ := NewQAV2ID("issue", "alpha", "39-coverage", attemptID, "promoted")
	assessment := QAAssessmentRecord{SchemaVersion: 2, ID: assessmentID, AttemptID: attemptID, ReviewVerdict: ReviewPass, ReviewFingerprint: testQAFingerprint, Assessment: AssessmentBlocked, CandidateTotal: 2, UnpromotedTotal: 1, IssueTotal: 1, NextAction: "Collect promotion evidence.", CompletedAt: time.Unix(2, 0)}
	adjudication := QAAdjudication{SchemaVersion: 2, ID: adjudicationID, AttemptID: attemptID, MapFingerprint: strings.Repeat("b", 64), Issues: []QAIssue{{ID: issueID, Title: "Promoted", Severity: "high", Location: "a.go", EvidenceIDs: []string{"evidence"}}}, Unpromoted: []QAUnpromotedIssue{{CandidateID: "candidate", Title: "Unpromoted", IssueClass: "behavior", Severity: "medium", Location: "b.go", ReasonCode: "promotion_evidence_missing", Detail: "no accepted failing evidence was linked to this candidate"}}, CompletedAt: time.Unix(2, 0)}
	report, err := RenderQAReport("alpha", "39-coverage", testQAFingerprint, nil, adjudication, assessment)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## Issue candidates", "Total: `2`", "Promoted: `1`", "Unpromoted: `1`", "reason `promotion_evidence_missing`"} {
		if !bytes.Contains(report, []byte(want)) {
			t.Fatalf("report does not contain %q:\n%s", want, report)
		}
	}
}

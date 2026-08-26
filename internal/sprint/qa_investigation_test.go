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

func TestQAInvestigationWritableCopyPreservesTargetAndCleans(t *testing.T) {
	target := t.TempDir()
	targetFile := filepath.Join(target, "source.txt")
	if err := os.WriteFile(targetFile, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	budgets := DefaultQABudgets()
	budgets.CommandTimeout = 5 * time.Second
	budgets.ShardTimeout = 5 * time.Second
	limits := pprocess.IsolationLimits{MaxFiles: budgets.TreeFiles, MaxBytes: budgets.TreeBytes, MaxFileSize: budgets.FileBytes, Timeout: budgets.ShardTimeout}
	targetIdentity, err := pprocess.IdentifyTree(context.Background(), target, limits)
	if err != nil {
		t.Fatal(err)
	}
	attemptID, _ := NewQASemanticAttemptID("alpha", "37-evidence", QASemanticIdentity{ChangedPaths: []string{"source.txt"}})
	shardID, _ := NewQAShardID("alpha", "37-evidence", attemptID, QAShardIdentity{Kind: QAShardPrimary, ChangedPaths: []string{"source.txt"}, BehavioralConcerns: []string{"copy"}, ExpectationRefs: []string{"REQ-1"}})
	plan, err := FreezeQAEvidencePlan("alpha", "37-evidence", QAEvidencePlan{
		AttemptID:                 attemptID,
		ShardID:                   shardID,
		ExpectationRefs:           []string{"REQ-1"},
		Kind:                      QACheckBehavioral,
		ConfirmationCondition:     "copy command succeeds",
		RefutationCondition:       "copy command fails",
		InconclusiveCondition:     "copy command cannot complete",
		ApprovedPaths:             []string{"probe.txt"},
		Executable:                "touch",
		Args:                      []string{"probe.txt"},
		EnvironmentNames:          []string{"PATH"},
		Timeout:                   5 * time.Second,
		OutputLimit:               1024,
		CleanupRequired:           true,
		GovernedInputFingerprint:  testQAFingerprint,
		ImplementationFingerprint: strings.Repeat("b", 64),
		MapFingerprint:            strings.Repeat("c", 64),
	}, budgets, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	workspaceParent := t.TempDir()
	record, err := RunQAInvestigation(context.Background(), QAInvestigationRequest{
		Project:          "alpha",
		Sprint:           "37-evidence",
		TargetRoot:       target,
		WorkspaceParent:  workspaceParent,
		ProtectedRoots:   []string{target},
		Plan:             plan,
		Budgets:          budgets,
		Environment:      map[string]string{"PATH": os.Getenv("PATH")},
		ExpectedTargetID: targetIdentity.Digest,
		Now:              func() time.Time { return time.Unix(2, 0) },
	})
	if err != nil {
		if typed, ok := AsQAError(err); ok && typed.Category == QAErrorAdmissionBlocked {
			t.Skip("native protected-root isolation is unavailable")
		}
		t.Fatal(err)
	}
	if err := ValidateQAEvidence(record, plan, budgets); err != nil {
		t.Fatalf("invalid retained evidence: %v", err)
	}
	if record.Outcome != QAEvidencePass || !record.Cleanup.Complete || !record.Contained {
		t.Fatalf("evidence = %+v", record)
	}
	scopeViolation := record
	scopeViolation.ChangedPaths = append(scopeViolation.ChangedPaths, "outside.txt")
	scopeViolation.Outcome = QAEvidenceBlocked
	scopeViolation.ReasonCode = "path_not_approved"
	if err := ValidateQAEvidence(scopeViolation, plan, budgets); err != nil {
		t.Fatalf("blocked scope violation was not retainable: %v", err)
	}
	scopeViolation.Outcome = QAEvidencePass
	if err := ValidateQAEvidence(scopeViolation, plan, budgets); err == nil {
		t.Fatal("unapproved path was accepted without a blocked scope-violation outcome")
	}
	data, readErr := os.ReadFile(targetFile)
	if readErr != nil || string(data) != "original\n" {
		t.Fatalf("target changed: %q, %v", data, readErr)
	}
	entries, readErr := os.ReadDir(workspaceParent)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("isolated workspace remains: entries=%v err=%v", entries, readErr)
	}
}

func TestQAInvestigationRejectsOriginalTargetLeak(t *testing.T) {
	target := t.TempDir()
	budgets := DefaultQABudgets()
	limits := pprocess.IsolationLimits{MaxFiles: budgets.TreeFiles, MaxBytes: budgets.TreeBytes, MaxFileSize: budgets.FileBytes, Timeout: budgets.ShardTimeout}
	targetIdentity, identityErr := pprocess.IdentifyTree(context.Background(), target, limits)
	if identityErr != nil {
		t.Fatal(identityErr)
	}
	attemptID, _ := NewQASemanticAttemptID("alpha", "37-evidence", QASemanticIdentity{ChangedPaths: []string{"a.go"}})
	shardID, _ := NewQAShardID("alpha", "37-evidence", attemptID, QAShardIdentity{Kind: QAShardPrimary, ChangedPaths: []string{"a.go"}, BehavioralConcerns: []string{"path"}, ExpectationRefs: []string{"REQ-1"}})
	plan, err := FreezeQAEvidencePlan("alpha", "37-evidence", QAEvidencePlan{AttemptID: attemptID, ShardID: shardID, ExpectationRefs: []string{"REQ-1"}, Kind: QACheckNegative, ConfirmationCondition: "path absent", RefutationCondition: "path present", InconclusiveCondition: "request unavailable", ApprovedPaths: []string{"a.go"}, Executable: "true", Args: []string{target}, Timeout: time.Second, OutputLimit: 128, CleanupRequired: true, GovernedInputFingerprint: testQAFingerprint, ImplementationFingerprint: strings.Repeat("b", 64), MapFingerprint: strings.Repeat("c", 64)}, budgets, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	_, err = RunQAInvestigation(context.Background(), QAInvestigationRequest{Project: "alpha", Sprint: "37-evidence", TargetRoot: target, WorkspaceParent: t.TempDir(), ProtectedRoots: []string{target}, Plan: plan, Budgets: budgets, ExpectedTargetID: targetIdentity.Digest})
	if typed, ok := AsQAError(err); !ok || typed.Category != QAErrorPermissionDenied {
		t.Fatalf("target leak error = %v", err)
	}
}

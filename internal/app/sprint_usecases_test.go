package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/sprint"
)

func canonicalQAFixture(t *testing.T) QAResult {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "testdata", "qa-canonical-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result QAResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestCanonicalQAFixtureFreezesSharedProjectionFields(t *testing.T) {
	result := canonicalQAFixture(t)
	if result.SchemaVersion != 1 || result.MapFingerprint != "qa-v1-map-bbbbbbbbbbbbbbbbbbbbbbbb" || result.ChangedPaths != 2 || result.CoveredPaths != 2 || result.Shards[0].Theories[0].Outcome != "refuted" || result.NextAction != "Inspect retained outcomes." {
		t.Fatalf("canonical QA fixture = %+v", result)
	}
}

func TestSprintSummaryReportsUnreadableQAStateAsInvalid(t *testing.T) {
	root := initializedWorkspace(t)
	writeCommandSprintProject(t, root, "alpha", "01-test")
	base := filepath.Join(root, "projects", "alpha", "sprints", "01-test")
	writeFixtureFileContent(t, base, `{broken`, "verification", "state.json")
	summaries, err := (dashboardUseCases{root: root, readOnly: true}).SprintSummaries(context.Background())
	if err != nil || len(summaries) != 1 {
		t.Fatalf("summaries=%+v err=%v", summaries, err)
	}
	qa := summaries[0].QA
	if qa.Phase != "invalid" || qa.Blocker == nil || qa.Blocker.Category != "qa.invalid_state" || qa.NextAction == "" {
		t.Fatalf("QA summary masked unreadable state: %+v", qa)
	}
}

func TestExecuteTerminalComplete(t *testing.T) {
	tests := []struct {
		name    string
		summary ExecuteSummary
		want    bool
	}{
		{name: "all complete", summary: ExecuteSummary{Available: true, Total: 3, Complete: 3}, want: true},
		{name: "accepted deferred work", summary: ExecuteSummary{Available: true, Total: 3, Complete: 2, Deferred: 1}, want: true},
		{name: "pending", summary: ExecuteSummary{Available: true, Total: 3, Complete: 2, Pending: 1}},
		{name: "failed", summary: ExecuteSummary{Available: true, Total: 3, Complete: 2, Failed: 1}},
		{name: "empty", summary: ExecuteSummary{Available: true}},
		{name: "unavailable", summary: ExecuteSummary{Total: 1, Complete: 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := executeTerminalComplete(test.summary); got != test.want {
				t.Fatalf("executeTerminalComplete() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestQAProjectionIsAdapterIndependentBoundedAndSanitized(t *testing.T) {
	qaMap := sprint.QAMap{SchemaVersion: 1, Project: "alpha", Sprint: "36-read-only-qa", CheckCatalogFingerprint: "catalog", Budgets: sprint.DefaultQABudgets(), EffectiveSources: []sprint.QAEffectiveSource{{Field: "runtime.model", Source: "workspace"}}, Target: sprint.QATargetIdentity{Fingerprint: "target", GitHead: "head", Categories: map[string]string{"worktree": "dirty"}}, Coverage: sprint.QACoverage{ChangedPaths: []string{"internal/app/a.go"}, PrimaryOwners: map[string]string{"internal/app/a.go": "shard-00"}, BoundaryOverlaps: map[string][]string{"internal/app/a.go": {"shard-01"}}}, InputRefs: []sprint.QAArtifactRef{{Path: "requirements.md", Digest: "requirements-digest"}}}
	for i := 0; i < 45; i++ {
		shard := sprint.QAShard{ID: fmt.Sprintf("shard-%02d", i), Kind: sprint.QAShardPrimary, Title: "branch\x1b[31m", Phase: sprint.QAPhaseCompleted}
		for j := 0; j < 25; j++ {
			shard.Theories = append(shard.Theories, sprint.QATheory{ID: fmt.Sprintf("theory-%02d-%02d", i, j), Claim: "claim\x00", Outcome: sprint.QATheoryRefuted})
		}
		qaMap.Shards = append(qaMap.Shards, shard)
	}
	result := qaMapProjection(qaMap)
	if len(result.Shards) != sprint.MaximumQABudgets().TotalShards || len(result.Shards[0].Theories) != 24 {
		t.Fatalf("projection bounds = shards:%d theories:%d", len(result.Shards), len(result.Shards[0].Theories))
	}
	if strings.ContainsAny(result.Shards[0].Title+result.Shards[0].Theories[0].Claim, "\x00\x1b") {
		t.Fatalf("projection retained terminal control content: %+v", result.Shards[0])
	}
	if result.Limits.TotalShards != qaMap.Budgets.TotalShards || result.Limits.RunTimeout != qaMap.Budgets.RunTimeout.String() {
		t.Fatalf("effective QA limits not projected: %+v", result.Limits)
	}
	if result.CheckCatalogFingerprint != "catalog" || len(result.EffectiveSources) != 1 || result.Target.GitHead != "head" || result.Coverage.PrimaryOwners["internal/app/a.go"] != "shard-00" || len(result.InputRefs) != 1 {
		t.Fatalf("map observability not projected: %+v", result)
	}
}

func TestQAShardProjectionRetainsSafeInvestigationObservability(t *testing.T) {
	started := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)
	completed := started.Add(1250 * time.Millisecond)
	attempt := sprint.QAInvestigatorAttempt{
		ID: "op/shard/1", Number: 1, StartedAt: started, CompletedAt: &completed,
		ImplementationBefore: "before", ImplementationAfter: "after", StopReason: "accepted\x1b[31m",
		ContextRequests: []sprint.QAContextRequest{{ID: "context-1", Paths: []string{"internal/app"}, Reason: "trace\x00 boundary", Approved: true}},
		Commands:        []sprint.QACommandSummary{{CheckID: "go-test-app", DescriptorFingerprint: "descriptor", ExitCode: 0, Duration: 80 * time.Millisecond, StdoutDigest: "stdout", OutputBytes: 42, Truncated: true}},
		Evidence:        []sprint.QAEvidenceSummary{{Kind: "check", Summary: "passed\x1b[32m", Paths: []string{"internal/app"}, CheckID: "go-test-app", OutputDigest: "digest"}},
	}
	shard := sprint.QAShard{
		ID: "qa-v1-shard-aaaaaaaaaaaaaaaaaaaaaaaa", AttemptID: "qa-v1-attempt-bbbbbbbbbbbbbbbbbbbbbbbb", Kind: sprint.QAShardBoundary,
		Title: "boundary", Phase: sprint.QAPhaseCompleted, ChangedPaths: []string{"internal/app/a.go"}, ContextPaths: []string{"internal/app/b.go"},
		OverlapPaths: []string{"internal/app/shared.go"}, BoundaryReason: "shared behavior", BehavioralConcerns: []string{"cancellation"},
		ExpectationRefs: []string{"AC-1"}, RiskTags: []string{"concurrency"}, ApprovedChecks: []sprint.QAApprovedCheckRef{{ID: "go-test-app", Fingerprint: "check-fingerprint"}},
		ParentTheoryIDs: []string{"qa-v1-theory-parent"}, Attempts: []sprint.QAInvestigatorAttempt{attempt},
		Theories: []sprint.QATheory{{ID: "qa-v1-theory-cccccccccccccccccccccccc", ShardID: "qa-v1-shard-aaaaaaaaaaaaaaaaaaaaaaaa", Claim: "claim", Basis: "basis", VerificationSurface: "surface", SeverityIfConfirmed: "high", ConfirmationCondition: "fails", RefutationCondition: "passes", InconclusiveCondition: "unavailable", SafeEvidenceStrategy: "approved check", ImplementationFingerprint: "before", AttemptHistory: []sprint.QAInvestigatorAttempt{attempt}, Evidence: attempt.Evidence, Outcome: sprint.QATheoryRefuted, OutcomeReason: "passed"}},
	}
	result := qaShardProjection(shard)
	if result.AttemptID != shard.AttemptID || result.BoundaryReason != "shared behavior" || len(result.ApprovedChecks) != 1 || len(result.Attempts) != 1 || len(result.Theories) != 1 {
		t.Fatalf("shard observability missing: %+v", result)
	}
	if result.Attempts[0].Duration != "1.25s" || len(result.Attempts[0].ContextRequests) != 1 || len(result.Attempts[0].Commands) != 1 || len(result.Attempts[0].Evidence) != 1 {
		t.Fatalf("attempt observability missing: %+v", result.Attempts[0])
	}
	if len(result.Theories[0].AttemptHistory) != 1 || len(result.Theories[0].Evidence) != 1 {
		t.Fatalf("theory observability missing: %+v", result.Theories[0])
	}
	joined := result.Attempts[0].StopReason + result.Attempts[0].ContextRequests[0].Reason + result.Attempts[0].Evidence[0].Summary
	if strings.ContainsAny(joined, "\x00\x1b") {
		t.Fatalf("investigation projection retained terminal controls: %q", joined)
	}
}

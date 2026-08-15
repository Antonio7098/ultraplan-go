package sprint

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

type gatedReviewRuntime struct {
	started chan struct{}
	release chan struct{}
}

func (g *gatedReviewRuntime) StartRun(_ context.Context, req pruntime.Request) (pruntime.Result, error) {
	select {
	case g.started <- struct{}{}:
	default:
	}
	<-g.release
	data, _ := json.Marshal(ReviewCoverageResult{SchemaVersion: 1, CoverageID: req.Metadata["coverage"], Applicability: "direct", Summary: "checked"})
	return pruntime.Result{Events: []pruntime.Event{{Payload: map[string]any{"content": string(data)}}}, Permissions: pruntime.PermissionSummary{Mode: "restricted"}}, nil
}

func TestDeriveAssessmentPrecedence(t *testing.T) {
	currentReview := VerificationStage{ExecutionStatus: string(ReviewCompleted), Verdict: string(ReviewPass), Fresh: true}
	currentSmoke := VerificationStage{ExecutionStatus: string(SmokeCompleted), Verdict: string(SmokePass), Fresh: true}
	tests := []struct {
		name          string
		review, smoke VerificationStage
		malformed     bool
		want          OverallAssessment
	}{
		{"malformed", currentReview, currentSmoke, true, AssessmentBlocked},
		{"review fail", VerificationStage{ExecutionStatus: string(ReviewCompleted), Verdict: string(ReviewFail), Fresh: true}, currentSmoke, false, AssessmentFail},
		{"review blocked", VerificationStage{ExecutionStatus: string(ReviewCompleted), Verdict: string(ReviewVerdictBlocked), Fresh: true}, currentSmoke, false, AssessmentBlocked},
		{"review stale", VerificationStage{ExecutionStatus: string(ReviewCompleted), Verdict: string(ReviewPass)}, currentSmoke, false, AssessmentIncomplete},
		{"attempt failed", VerificationStage{ExecutionStatus: string(ReviewFailed), Verdict: string(ReviewPass), Fresh: true}, currentSmoke, false, AssessmentIncomplete},
		{"smoke stale", currentReview, VerificationStage{ExecutionStatus: string(SmokeCompleted), Verdict: string(SmokePass)}, false, AssessmentIncomplete},
		{"smoke fail", currentReview, VerificationStage{ExecutionStatus: string(SmokeCompleted), Verdict: string(SmokeFailVerdict), Fresh: true}, false, AssessmentFail},
		{"smoke blocked", currentReview, VerificationStage{ExecutionStatus: string(SmokeCompleted), Verdict: string(SmokeBlockedVerdict), Fresh: true}, false, AssessmentBlocked},
		{"not applicable", currentReview, VerificationStage{ExecutionStatus: string(SmokeCompleted), Verdict: string(SmokeNotApplicable), Fresh: true}, false, AssessmentNotApplicable},
		{"open issues", currentReview, VerificationStage{ExecutionStatus: string(SmokeCompleted), Verdict: string(SmokePassWithOpenIssues), Fresh: true}, false, AssessmentPassWithFindings},
		{"review findings", VerificationStage{ExecutionStatus: string(ReviewCompleted), Verdict: string(ReviewPassWithFindings), Fresh: true}, currentSmoke, false, AssessmentPassWithFindings},
		{"pass", currentReview, currentSmoke, false, AssessmentPass},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, next := deriveAssessment(test.review, test.smoke, test.malformed)
			if got != test.want || next == "" {
				t.Fatalf("assessment=%s next=%q want=%s", got, next, test.want)
			}
		})
	}
}

func TestFlowStateMigratesExactlyOnePredecessor(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	state := NewFlowState(sp, completeStates(sp), now)
	state.SchemaVersion = PreviousFlowStateSchemaVersion
	state.Review = &ReviewStageState{Status: ReviewCompleted, Verdict: ReviewPass, Path: ArtifactRelPath(sp, StageReview), LastRunAt: &now, Fingerprint: "legacy", Completed: 1, Total: 1}
	path, _ := FlowStatePath(root, sp)
	writeJSON(t, path, state)
	loaded, err := LoadFlowState(root, sp)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SchemaVersion != FlowStateSchemaVersion || loaded.Review == nil || !loaded.Review.Stale || loaded.Review.LastComplete == nil {
		t.Fatalf("migration=%+v", loaded)
	}
	data, _ := os.ReadFile(path)
	var persisted map[string]any
	if err := json.Unmarshal(data, &persisted); err != nil || int(persisted["schemaVersion"].(float64)) != PreviousFlowStateSchemaVersion {
		t.Fatalf("read unexpectedly persisted migration=%s err=%v", data, err)
	}
	state.SchemaVersion = 0
	writeJSON(t, path, state)
	if _, err := LoadFlowState(root, sp); !errors.Is(err, ErrFlowStateMalformed) {
		t.Fatalf("zero version error=%v", err)
	}
	state.SchemaVersion = 99
	writeJSON(t, path, state)
	if _, err := LoadFlowState(root, sp); !errors.Is(err, ErrFlowStateUnsupported) {
		t.Fatalf("unknown version error=%v", err)
	}
}

func TestVerificationStatusDerivesExpiredReviewAttemptWithoutWriting(t *testing.T) {
	root, sp := reviewFixture(t)
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	started := now.Add(-25 * time.Hour)
	state := NewFlowState(sp, completeStates(sp), started)
	state.Review = &ReviewStageState{
		Status:        ReviewRunning,
		Path:          ArtifactRelPath(sp, StageReview),
		LastRunAt:     &started,
		ActiveAttempt: &VerificationAttempt{ID: "review-stale", Status: AttemptRunning, StartedAt: started},
	}
	if err := SaveFlowState(root, sp, state); err != nil {
		t.Fatal(err)
	}
	service := NewService(root)
	service.now = func() time.Time { return now }
	status, err := service.VerificationStatus("proj", "01")
	if err != nil {
		t.Fatal(err)
	}
	if status.Review.ExecutionStatus != string(ReviewFailed) || status.Review.ActiveAttempt != nil || status.Review.LastAttempt == nil || status.Review.LastAttempt.Status != AttemptTimedOut {
		t.Fatalf("status did not reconcile expired attempt: %+v", status.Review)
	}
	persisted, err := LoadFlowState(root, sp)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Review.ActiveAttempt == nil || persisted.Review.ActiveAttempt.Status != AttemptRunning || persisted.Review.LastAttempt != nil {
		t.Fatalf("read-only status mutated durable attempt: %+v", persisted.Review)
	}
}

func TestVerificationStatusImmediatelyRecoversDeadAttemptOwner(t *testing.T) {
	root, sp := reviewFixture(t)
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	started := now.Add(-time.Minute)
	state := NewFlowState(sp, completeStates(sp), started)
	state.Review = &ReviewStageState{
		Status:        ReviewRunning,
		Path:          ArtifactRelPath(sp, StageReview),
		LastRunAt:     &started,
		ActiveAttempt: &VerificationAttempt{ID: "review-dead-owner", Status: AttemptRunning, StartedAt: started, HeartbeatAt: started, OwnerPID: 99999999},
	}
	if err := SaveFlowState(root, sp, state); err != nil {
		t.Fatal(err)
	}
	service := NewService(root)
	service.now = func() time.Time { return now }
	status, err := service.VerificationStatus("proj", "01")
	if err != nil {
		t.Fatal(err)
	}
	if status.Review.ActiveAttempt != nil || status.Review.LastAttempt == nil || status.Review.LastAttempt.Status != AttemptTimedOut {
		t.Fatalf("dead owner was not recovered: %+v", status.Review)
	}
}

func TestReviewFreshnessArtifactEditAndFocusedMerge(t *testing.T) {
	root, sp := reviewFixture(t)
	runtime := &reviewRuntime{}
	service := NewService(root).WithRuntime(runtime).WithStageRuntime(map[PlanningStage]StageRuntime{StageReview: {Model: "test/model"}})
	first, err := service.Review(context.Background(), "proj", "01", ReviewRequest{})
	if err != nil {
		t.Fatal(err)
	}
	status, err := service.VerificationStatus("proj", "01")
	if err != nil || !status.Review.Fresh {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	initialCalls := runtime.calls
	focus := first.Coverage[0].CoverageID
	second, err := service.Review(context.Background(), "proj", "01", ReviewRequest{Focus: []string{focus}})
	if err != nil || len(second.Coverage) != len(first.Coverage) || runtime.calls != initialCalls+1 {
		t.Fatalf("focused=%+v calls=%d err=%v", second, runtime.calls, err)
	}
	artifact := filepath.Join(sp.Path, "review.md")
	data, _ := os.ReadFile(artifact)
	if err := os.WriteFile(artifact, append(data, []byte("\nexternal edit\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err = service.VerificationStatus("proj", "01")
	if err != nil || status.Review.Fresh || status.Assessment != AssessmentIncomplete {
		t.Fatalf("edited status=%+v err=%v", status, err)
	}
}

func TestDiagnosticOverrideCannotPromoteCanonicalSmoke(t *testing.T) {
	root, sp := reviewFixture(t)
	now := time.Now().UTC()
	state := NewFlowState(sp, completeStates(sp), now)
	state.Review = &ReviewStageState{Status: ReviewCompleted, Verdict: ReviewFail, Path: ArtifactRelPath(sp, StageReview), LastRunAt: &now, Fingerprint: "f", ArtifactDigest: "d", LastComplete: &ReviewCompletion{Verdict: ReviewFail, Artifact: ArtifactRelPath(sp, StageReview), ArtifactDigest: "d", InputFingerprint: "f", CompletedAt: now}}
	state.Smoke = &SmokeStageState{Status: SmokeCompleted, Verdict: SmokeFailVerdict, Path: ArtifactRelPath(sp, StageSmoke), LastRunAt: &now, InputFingerprint: "old", ArtifactDigest: "old", LastComplete: &SmokeCompletion{Verdict: SmokeFailVerdict, Artifact: ArtifactRelPath(sp, StageSmoke), ArtifactDigest: "old", InputFingerprint: "old", CompletedAt: now}}
	if err := SaveFlowState(root, sp, state); err != nil {
		t.Fatal(err)
	}
	assessment, _ := deriveAssessment(VerificationStage{ExecutionStatus: string(ReviewCompleted), Verdict: string(ReviewFail), Fresh: true}, VerificationStage{ExecutionStatus: string(SmokeCompleted), Verdict: string(SmokePass), Fresh: true, Override: &DiagnosticOverride{Requested: true, Confirmed: true}}, false)
	if assessment != AssessmentFail {
		t.Fatalf("override laundered review failure: %s", assessment)
	}
}

func TestVerificationMutationConflictIsTyped(t *testing.T) {
	root, _ := reviewFixture(t)
	runtime := &gatedReviewRuntime{started: make(chan struct{}, 1), release: make(chan struct{})}
	service := NewService(root).WithRuntime(runtime).WithStageRuntime(map[PlanningStage]StageRuntime{StageReview: {Model: "test/model"}})
	done := make(chan error, 1)
	go func() {
		_, err := service.Review(context.Background(), "proj", "01", ReviewRequest{Concurrency: 1})
		done <- err
	}()
	<-runtime.started
	if _, err := service.Review(context.Background(), "proj", "01-alpha", ReviewRequest{}); !errors.Is(err, ErrVerificationConflict) {
		t.Fatalf("conflict error=%v", err)
	}
	close(runtime.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

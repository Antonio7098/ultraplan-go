package sprint

import "testing"

func TestVerificationPhaseIsIndependentFromPlanningOrder(t *testing.T) {
	wantPlanning := []PlanningStage{
		StageRequirements,
		StageCodeContext,
		StageSprintIndex,
		StageTechnicalHandbook,
		StageAreaReasoning,
		StageReasoning,
		StagePlan,
	}
	got := PlanningStages()
	if len(got) != len(wantPlanning) {
		t.Fatalf("PlanningStages() = %v", got)
	}
	for i := range wantPlanning {
		if got[i] != wantPlanning[i] {
			t.Fatalf("PlanningStages()[%d] = %q, want %q", i, got[i], wantPlanning[i])
		}
	}
	for _, stage := range got {
		if stage == PlanningStage(VerificationPhaseQA) || stage == PlanningStage(VerificationPhaseRepair) || stage == PlanningStage(VerificationPhaseConformanceReview) {
			t.Fatalf("verification phase %q entered planning order", stage)
		}
	}
}

func TestVerificationPhaseCompatibility(t *testing.T) {
	for _, alias := range []string{"review", "conformance-review"} {
		phase, err := ParseVerificationPhase(alias)
		if err != nil || phase != VerificationPhaseConformanceReview {
			t.Fatalf("ParseVerificationPhase(%q) = %q, %v", alias, phase, err)
		}
		stage, ok := phase.CompatibilityStage()
		if !ok || stage != StageReview {
			t.Fatalf("CompatibilityStage(%q) = %q, %v", phase, stage, ok)
		}
	}
	for _, phase := range []VerificationPhase{VerificationPhaseQA, VerificationPhaseRepair} {
		if _, ok := phase.CompatibilityStage(); ok {
			t.Fatalf("%q unexpectedly has a planning compatibility stage", phase)
		}
	}
	if _, err := ParseVerificationPhase("smoke"); err == nil {
		t.Fatal("expected unsupported phase error")
	}
}

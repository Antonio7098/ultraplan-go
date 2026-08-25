package sprint

import "fmt"

// VerificationPhase identifies verification work independently from the
// authored planning sequence. Conformance review keeps the existing review
// implementation and public compatibility contracts.
type VerificationPhase string

const (
	VerificationPhaseConformanceReview VerificationPhase = "conformance-review"
	VerificationPhaseQA                VerificationPhase = "qa"
	VerificationPhaseRepair            VerificationPhase = "repair"
)

func VerificationPhases() []VerificationPhase {
	return []VerificationPhase{
		VerificationPhaseConformanceReview,
		VerificationPhaseQA,
		VerificationPhaseRepair,
	}
}

func ParseVerificationPhase(value string) (VerificationPhase, error) {
	switch value {
	case "review", string(VerificationPhaseConformanceReview):
		return VerificationPhaseConformanceReview, nil
	case string(VerificationPhaseQA):
		return VerificationPhaseQA, nil
	case string(VerificationPhaseRepair):
		return VerificationPhaseRepair, nil
	default:
		return "", fmt.Errorf("unsupported verification phase %q", value)
	}
}

// CompatibilityStage maps only shipped legacy stage capabilities. QA and
// repair have no PlanningStage identity and therefore cannot enter planning
// order through this adapter.
func (p VerificationPhase) CompatibilityStage() (PlanningStage, bool) {
	if p == VerificationPhaseConformanceReview {
		return StageReview, true
	}
	return "", false
}

func verificationPhaseForStage(stage PlanningStage) (VerificationPhase, bool) {
	if stage == StageReview {
		return VerificationPhaseConformanceReview, true
	}
	return "", false
}

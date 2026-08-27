package sprint

import "strings"

// StageInputContract is the canonical artifact order for one stage. It is a
// routing contract, not a freshness fingerprint: it never invalidates or reruns
// an existing artifact.
type StageInputContract struct {
	Stage     PlanningStage `json:"stage"`
	Required  []string      `json:"required"`
	Optional  []string      `json:"optional,omitempty"`
	Forbidden []string      `json:"forbidden,omitempty"`
}

func InputContract(stage PlanningStage) StageInputContract {
	shared := []string{"requirements", "code-context", "resolved-source-evidence"}
	contract := StageInputContract{Stage: stage}
	switch stage {
	case StageRequirements:
		contract.Required = []string{"project-index", "roadmap", "project-docs"}
		contract.Optional = []string{"prior-sprint-reviews"}
	case StageCodeContext:
		contract.Required = []string{"requirements", "implementation-repository"}
	case StageSprintIndex:
		contract.Required = append(append([]string{}, shared...), "project-index", "roadmap", "project-docs")
	case StageTechnicalHandbook:
		contract.Required = append(append([]string{}, shared...), "sprint-index", "selected-evidence")
	case StageAreaReasoning:
		contract.Required = append(append([]string{}, shared...), "project-docs", "sprint-index", "technical-handbook", "selected-context-files", "selected-area-template")
		contract.Forbidden = []string{"sibling-area-templates"}
	case StageReasoning:
		contract.Required = append(append([]string{}, shared...), "project-index", "roadmap", "project-docs", "sprint-index", "technical-handbook", "selected-context-files", "area-reasoning")
	case StagePlan:
		contract.Required = append(append([]string{}, shared...), "project-index", "roadmap", "project-docs", "sprint-index", "technical-handbook", "area-reasoning", "reasoning", "sprint-plan-template")
	case StageExecute:
		contract.Required = append(append([]string{}, shared...), "project-index", "roadmap", "project-docs", "sprint-index", "technical-handbook", "area-reasoning", "reasoning", "plan", "ordered-plan-task-queue", "current-plan-task")
		contract.Forbidden = []string{"full-details-for-non-current-tasks"}
	case StageReview:
		contract.Required = append(append([]string{}, shared...), "coverage-source", "governed-review-inputs", "changed-target-files")
		contract.Forbidden = []string{"sibling-coverage-sources"}
	case StageSmoke:
		contract.Required = append(append([]string{}, shared...), "sprint-index", "technical-handbook", "area-reasoning", "reasoning", "plan", "execution-evidence", "review-outcome", "execution-handoff", "smoke-harness")
	case StageMerge:
		contract.Required = []string{"workspace-record", "source-commit", "target-commit", "merge-base", "changed-paths", "review-outcome", "smoke-outcome"}
		contract.Forbidden = []string{"unrecorded-target-branch", "unapproved-conflict-paths"}
	}
	return contract
}

func (c StageInputContract) requiredMetadata() string { return strings.Join(c.Required, ",") }

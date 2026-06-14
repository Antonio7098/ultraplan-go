package sprint

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const FlowStateSchemaVersion = 1

var (
	ErrFlowStateMissing     = errors.New("flow state missing")
	ErrFlowStateMalformed   = errors.New("flow state malformed")
	ErrFlowStateUnsupported = errors.New("flow state unsupported")
)

type Sprint struct {
	Project string
	Slug    string
	Path    string
}

type PlanningStage string

const (
	StageRequirements      PlanningStage = "requirements"
	StageSprintIndex       PlanningStage = "sprint-index"
	StageTechnicalHandbook PlanningStage = "technical-handbook"
	StageAreaReasoning     PlanningStage = "area-reasoning"
	StageReasoning         PlanningStage = "reasoning"
	StagePlan              PlanningStage = "plan"
)

type StageStatus string

const (
	StatusMissing  StageStatus = "missing"
	StatusReady    StageStatus = "ready"
	StatusComplete StageStatus = "complete"
	StatusFailed   StageStatus = "failed"
	StatusSkipped  StageStatus = "skipped"
)

type StageState struct {
	Stage     PlanningStage `json:"stage"`
	Status    StageStatus   `json:"status"`
	Path      string        `json:"path"`
	LastRunAt *time.Time    `json:"lastRunAt,omitempty"`
	Error     string        `json:"error,omitempty"`
}

type FlowState struct {
	SchemaVersion int          `json:"schemaVersion"`
	Project       string       `json:"project"`
	Sprint        string       `json:"sprint"`
	UpdatedAt     time.Time    `json:"updatedAt"`
	Stages        []StageState `json:"stages"`
}

type StatusSummary struct {
	Project       string
	Sprint        string
	SprintRoot    string
	FlowStatePath string
	Stages        []StageState
}

type ValidationFinding struct {
	Section    string
	EntryName  string
	Path       string
	Problem    string
	Cause      string
	Suggestion string
}

type ValidationResult struct {
	Project  string
	Sprint   string
	Artifact string
	Findings []ValidationFinding
}

func (r ValidationResult) Valid() bool { return len(r.Findings) == 0 }

func PlanningStages() []PlanningStage {
	return []PlanningStage{
		StageRequirements,
		StageSprintIndex,
		StageTechnicalHandbook,
		StageAreaReasoning,
		StageReasoning,
		StagePlan,
	}
}

func StageStatuses() []StageStatus {
	return []StageStatus{StatusMissing, StatusReady, StatusComplete, StatusFailed, StatusSkipped}
}

func ValidStage(stage PlanningStage) bool {
	for _, allowed := range PlanningStages() {
		if stage == allowed {
			return true
		}
	}
	return false
}

func ValidStatus(status StageStatus) bool {
	for _, allowed := range StageStatuses() {
		if status == allowed {
			return true
		}
	}
	return false
}

type RefError struct {
	Ref        string
	Candidates []string
	Ambiguous  bool
}

func (e RefError) Error() string {
	if e.Ambiguous {
		return fmt.Sprintf("ambiguous sprint reference %q; matches: %s", e.Ref, strings.Join(e.Candidates, ", "))
	}
	if len(e.Candidates) == 0 {
		return fmt.Sprintf("sprint reference %q not found", e.Ref)
	}
	return fmt.Sprintf("sprint reference %q not found; available: %s", e.Ref, strings.Join(e.Candidates, ", "))
}

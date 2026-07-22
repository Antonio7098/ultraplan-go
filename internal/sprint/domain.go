package sprint

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const FlowStateSchemaVersion = 1
const ExecuteRunStateSchemaVersion = 1

var (
	ErrFlowStateMissing     = errors.New("flow state missing")
	ErrFlowStateMalformed   = errors.New("flow state malformed")
	ErrFlowStateUnsupported = errors.New("flow state unsupported")

	ErrExecuteRunStateMissing     = errors.New("execute run state missing")
	ErrExecuteRunStateMalformed   = errors.New("execute run state malformed")
	ErrExecuteRunStateUnsupported = errors.New("execute run state unsupported")
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

type ExecuteTaskStatus string

const (
	ExecuteTaskPending   ExecuteTaskStatus = "pending"
	ExecuteTaskRunning   ExecuteTaskStatus = "running"
	ExecuteTaskComplete  ExecuteTaskStatus = "complete"
	ExecuteTaskFailed    ExecuteTaskStatus = "failed"
	ExecuteTaskCancelled ExecuteTaskStatus = "cancelled"
)

type ExecuteTaskIdentity struct {
	Name         string   `json:"name"`
	PlanLine     int      `json:"planLine"`
	Decisions    []string `json:"decisions,omitempty"`
	Requirements []string `json:"requirements,omitempty"`
	Evidence     []string `json:"evidence,omitempty"`
}

type ExecuteDiagnostic struct {
	Code    string    `json:"code"`
	Message string    `json:"message"`
	At      time.Time `json:"at"`
}

type ExecuteEvidence struct {
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
	Path    string `json:"path,omitempty"`
}

type ExecuteRuntimeSummary struct {
	RunID             string `json:"runId,omitempty"`
	SessionID         string `json:"sessionId,omitempty"`
	Model             string `json:"model,omitempty"`
	ModelSource       string `json:"modelSource,omitempty"`
	PermissionSummary string `json:"permissionSummary,omitempty"`
	ValidationSummary string `json:"validationSummary,omitempty"`
	UsageSummary      string `json:"usageSummary,omitempty"`
	OmissionReason    string `json:"omissionReason,omitempty"`
}

type ExecuteTaskRecord struct {
	ID          string                 `json:"id"`
	Identity    ExecuteTaskIdentity    `json:"identity"`
	Status      ExecuteTaskStatus      `json:"status"`
	Attempts    int                    `json:"attempts"`
	CreatedAt   time.Time              `json:"createdAt"`
	UpdatedAt   time.Time              `json:"updatedAt"`
	StartedAt   *time.Time             `json:"startedAt,omitempty"`
	CompletedAt *time.Time             `json:"completedAt,omitempty"`
	Diagnostics []ExecuteDiagnostic    `json:"diagnostics,omitempty"`
	Evidence    []ExecuteEvidence      `json:"evidence,omitempty"`
	Runtime     *ExecuteRuntimeSummary `json:"runtime,omitempty"`
}

type ExecuteTargetRef struct {
	Path   string `json:"path"`
	Source string `json:"source"`
}

type ExecuteRunState struct {
	SchemaVersion   int                 `json:"schemaVersion"`
	Project         string              `json:"project"`
	Sprint          string              `json:"sprint"`
	Target          ExecuteTargetRef    `json:"target"`
	PlanPath        string              `json:"planPath"`
	PlanFingerprint string              `json:"planFingerprint"`
	CreatedAt       time.Time           `json:"createdAt"`
	UpdatedAt       time.Time           `json:"updatedAt"`
	Tasks           []ExecuteTaskRecord `json:"tasks"`
	Metadata        map[string]string   `json:"metadata,omitempty"`
}

type FlowState struct {
	SchemaVersion int               `json:"schemaVersion"`
	Project       string            `json:"project"`
	Sprint        string            `json:"sprint"`
	UpdatedAt     time.Time         `json:"updatedAt"`
	Stages        []StageState      `json:"stages"`
	Review        *ReviewStageState `json:"review,omitempty"`
	Smoke         *SmokeStageState  `json:"smoke,omitempty"`
}

type StatusSummary struct {
	Project       string            `json:"project"`
	Sprint        string            `json:"sprint"`
	SprintRoot    string            `json:"sprint_root"`
	FlowStatePath string            `json:"flow_state_path"`
	Stages        []StageState      `json:"stages"`
	ExecuteState  *ExecuteRunState  `json:"execute_state,omitempty"`
	ExecutePath   string            `json:"execute_path"`
	RunStatePath  string            `json:"run_state_path"`
	Review        *ReviewStageState `json:"review,omitempty"`
	ReviewPath    string            `json:"review_path"`
	Smoke         *SmokeStageState  `json:"smoke,omitempty"`
	SmokePath     string            `json:"smoke_path"`
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

func ExecuteTaskStatuses() []ExecuteTaskStatus {
	return []ExecuteTaskStatus{ExecuteTaskPending, ExecuteTaskRunning, ExecuteTaskComplete, ExecuteTaskFailed, ExecuteTaskCancelled}
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

func ValidExecuteTaskStatus(status ExecuteTaskStatus) bool {
	for _, allowed := range ExecuteTaskStatuses() {
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

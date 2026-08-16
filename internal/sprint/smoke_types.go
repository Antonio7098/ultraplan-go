package sprint

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const StageSmoke PlanningStage = "smoke"

const SmokeProtocolMajor = 1

type SmokeExecutionStatus string
type SmokeVerdict string
type SmokePhase string

const (
	SmokeReady     SmokeExecutionStatus = "ready"
	SmokeRunning   SmokeExecutionStatus = "running"
	SmokeCompleted SmokeExecutionStatus = "completed"
	SmokeFailed    SmokeExecutionStatus = "failed"
	SmokeCancelled SmokeExecutionStatus = "cancelled"

	SmokePass               SmokeVerdict = "pass"
	SmokePassWithOpenIssues SmokeVerdict = "pass_with_open_issues"
	SmokeFailVerdict        SmokeVerdict = "fail"
	SmokeBlockedVerdict     SmokeVerdict = "blocked"
	SmokeNotApplicable      SmokeVerdict = "not_applicable"

	SmokePhasePreflight          SmokePhase = "preflight"
	SmokePhaseAuthoring          SmokePhase = "authoring"
	SmokePhaseDiscovery          SmokePhase = "discovery"
	SmokePhaseSelection          SmokePhase = "selection"
	SmokePhaseRunning            SmokePhase = "running"
	SmokePhaseValidatingEvidence SmokePhase = "validating_evidence"
	SmokePhaseWritingArtifact    SmokePhase = "writing_artifact"
	SmokePhaseCompleted          SmokePhase = "completed"
	SmokePhaseCancelled          SmokePhase = "cancelled"
	SmokePhaseFailed             SmokePhase = "failed"
)

type SmokeSettings struct {
	DiscoveryTimeout, RunTimeout, CleanupGrace time.Duration
	StdoutLimit, StderrLimit                   int
	Environment                                []string
	Sources                                    map[string]string
	Getenv                                     func(string) string
}

func DefaultSmokeSettings() SmokeSettings {
	return SmokeSettings{DiscoveryTimeout: 30 * time.Second, RunTimeout: 30 * time.Minute, CleanupGrace: 5 * time.Second, StdoutLimit: 4 << 20, StderrLimit: 1 << 20, Environment: []string{"PATH", "HOME", "TMPDIR", "LANG", "LC_ALL"}, Sources: map[string]string{}, Getenv: os.Getenv}
}

type SmokeRequest struct {
	Level, Suite, Test string
	Timeout            time.Duration
	ForceReview        bool
	OverrideConfirmed  bool
	OverrideRationale  string
	DryRun             bool
	NonInteractive     bool
	Progress           func(SmokeProgress)
}

type SmokeProgress struct {
	Phase         SmokePhase `json:"phase"`
	Suite         string     `json:"suite,omitempty"`
	Test          string     `json:"test,omitempty"`
	Message       string     `json:"message,omitempty"`
	Completed     int        `json:"completed,omitempty"`
	Total         int        `json:"total,omitempty"`
	DroppedEvents int        `json:"dropped_events,omitempty"`
}

type SmokeCounts struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
	Errors  int `json:"errors"`
}

type SmokeEvidence struct {
	Kind       string    `json:"kind"`
	Path       string    `json:"path"`
	SHA256     string    `json:"sha256"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modified_at"`
}

type SmokeIssue struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Path   string `json:"path"`
}

type SmokeTestResult struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type SmokeResult struct {
	Project            string               `json:"project"`
	Sprint             string               `json:"sprint"`
	Harness            string               `json:"harness"`
	Protocol           string               `json:"protocol"`
	Artifact           string               `json:"artifact"`
	Status             SmokeExecutionStatus `json:"execution_status"`
	Verdict            SmokeVerdict         `json:"verdict,omitempty"`
	Ready              bool                 `json:"ready"`
	Stale              bool                 `json:"stale"`
	ReviewOverride     bool                 `json:"review_override"`
	OverrideRationale  string               `json:"override_rationale,omitempty"`
	ReviewVerdict      ReviewVerdict        `json:"review_verdict"`
	ReviewFingerprint  string               `json:"review_fingerprint"`
	ScopeKind          string               `json:"scope_kind,omitempty"`
	Scope              string               `json:"scope,omitempty"`
	ScopeRationale     string               `json:"scope_rationale,omitempty"`
	DurationClass      string               `json:"duration_class,omitempty"`
	CostClass          string               `json:"cost_class,omitempty"`
	EvidenceRoots      []string             `json:"evidence_roots,omitempty"`
	Prerequisites      []string             `json:"prerequisites,omitempty"`
	Diagnostics        []string             `json:"diagnostics,omitempty"`
	SafeArgv           string               `json:"safe_argv,omitempty"`
	RunID              string               `json:"run_id,omitempty"`
	AuthorRunID        string               `json:"author_run_id,omitempty"`
	AuthorModel        string               `json:"author_model,omitempty"`
	AuthorChangedPaths []string             `json:"author_changed_paths,omitempty"`
	Runtime            string               `json:"runtime,omitempty"`
	Model              string               `json:"model,omitempty"`
	Duration           time.Duration        `json:"duration_ns,omitempty"`
	EffectiveTimeout   time.Duration        `json:"effective_timeout_ns,omitempty"`
	TimeoutSource      string               `json:"timeout_source,omitempty"`
	Counts             SmokeCounts          `json:"counts"`
	Evidence           []SmokeEvidence      `json:"evidence,omitempty"`
	Issues             []SmokeIssue         `json:"issues,omitempty"`
	Tests              []SmokeTestResult    `json:"tests,omitempty"`
	NextAction         string               `json:"next_action,omitempty"`
	DryRun             bool                 `json:"dry_run"`
	DiagnosticOnly     bool                 `json:"diagnostic_only"`
	Reconciliation     bool                 `json:"reconciliation_required"`
}

type SmokeStageState struct {
	Status             SmokeExecutionStatus `json:"status"`
	Verdict            SmokeVerdict         `json:"verdict,omitempty"`
	Path               string               `json:"path"`
	LastRunAt          *time.Time           `json:"lastRunAt,omitempty"`
	ReviewFingerprint  string               `json:"reviewFingerprint,omitempty"`
	SmokeFingerprint   string               `json:"smokeFingerprint,omitempty"`
	RunID              string               `json:"runId,omitempty"`
	AuthorRunID        string               `json:"authorRunId,omitempty"`
	AuthorModel        string               `json:"authorModel,omitempty"`
	AuthorChangedPaths []string             `json:"authorChangedPaths,omitempty"`
	EvidenceID         string               `json:"evidenceId,omitempty"`
	ReviewOverride     bool                 `json:"reviewOverride"`
	Stale              bool                 `json:"stale"`
	Reconciliation     bool                 `json:"reconciliationRequired"`
	Diagnostics        []string             `json:"diagnostics,omitempty"`
	ArtifactDigest     string               `json:"artifactDigest,omitempty"`
	InputFingerprint   string               `json:"inputFingerprint,omitempty"`
	Issues             []SmokeIssue         `json:"issues,omitempty"`
	Evidence           []EvidenceReference  `json:"evidence,omitempty"`
	Override           *DiagnosticOverride  `json:"override,omitempty"`
	ActiveAttempt      *VerificationAttempt `json:"activeAttempt,omitempty"`
	LastAttempt        *VerificationAttempt `json:"lastAttempt,omitempty"`
	LastComplete       *SmokeCompletion     `json:"lastComplete,omitempty"`
}

type SmokeCompletion struct {
	Verdict            SmokeVerdict        `json:"verdict"`
	Artifact           string              `json:"artifact"`
	ArtifactDigest     string              `json:"artifactDigest"`
	InputFingerprint   string              `json:"inputFingerprint"`
	CompletedAt        time.Time           `json:"completedAt"`
	RunID              string              `json:"runId,omitempty"`
	AuthorRunID        string              `json:"authorRunId,omitempty"`
	AuthorModel        string              `json:"authorModel,omitempty"`
	AuthorChangedPaths []string            `json:"authorChangedPaths,omitempty"`
	EvidenceID         string              `json:"evidenceId,omitempty"`
	Evidence           []EvidenceReference `json:"evidence,omitempty"`
	Issues             []SmokeIssue        `json:"issues,omitempty"`
	Override           *DiagnosticOverride `json:"override,omitempty"`
}

type SmokeError struct {
	Code, Category, Message, Guidance string
	Err                               error
}

func (e *SmokeError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return fmt.Sprintf("smoke %s: %s: %v", e.Category, e.Message, e.Err)
	}
	return fmt.Sprintf("smoke %s: %s", e.Category, e.Message)
}
func (e *SmokeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func smokeError(code, category, message, guidance string, err error) error {
	return &SmokeError{Code: code, Category: category, Message: message, Guidance: guidance, Err: err}
}

func AsSmokeError(err error) (*SmokeError, bool) {
	var target *SmokeError
	ok := errors.As(err, &target)
	return target, ok
}

func validSmokeVerdict(v SmokeVerdict) bool {
	switch v {
	case SmokePass, SmokePassWithOpenIssues, SmokeFailVerdict, SmokeBlockedVerdict, SmokeNotApplicable:
		return true
	}
	return false
}

func validateSmokeStageState(root string, sp Sprint, state SmokeStageState, path string) error {
	switch state.Status {
	case SmokeReady, SmokeRunning, SmokeCompleted, SmokeFailed, SmokeCancelled:
	default:
		return fmt.Errorf("%w: %s: smoke has unsupported status %q", ErrFlowStateMalformed, path, state.Status)
	}
	if state.Verdict != "" && !validSmokeVerdict(state.Verdict) {
		return fmt.Errorf("%w: %s: smoke has unsupported verdict %q", ErrFlowStateMalformed, path, state.Verdict)
	}
	if state.Path != ArtifactRelPath(sp, StageSmoke) {
		return fmt.Errorf("%w: %s: smoke path mismatch", ErrFlowStateMalformed, path)
	}
	if _, err := resolveSprintContained(root, sp, state.Path); err != nil {
		return fmt.Errorf("%w: %s: unsafe smoke path: %v", ErrFlowStateMalformed, path, err)
	}
	for _, d := range state.Diagnostics {
		if len(d) > 240 {
			return fmt.Errorf("%w: %s: smoke diagnostic too long", ErrFlowStateMalformed, path)
		}
	}
	if err := validateAttempt(state.ActiveAttempt, true); err != nil {
		return fmt.Errorf("%w: %s: smoke active attempt: %v", ErrFlowStateMalformed, path, err)
	}
	if err := validateAttempt(state.LastAttempt, false); err != nil {
		return fmt.Errorf("%w: %s: smoke last attempt: %v", ErrFlowStateMalformed, path, err)
	}
	if state.Override != nil {
		if !state.Override.Requested || !state.Override.Confirmed || strings.TrimSpace(state.Override.Rationale) == "" || len(state.Override.Rationale) > 240 || strings.ContainsAny(state.Override.Rationale, "\x00\r\n") {
			return fmt.Errorf("%w: %s: invalid diagnostic override", ErrFlowStateMalformed, path)
		}
	}
	for _, issue := range state.Issues {
		if issue.ID == "" || (issue.Status != "open" && issue.Status != "resolved") {
			return fmt.Errorf("%w: %s: invalid smoke issue", ErrFlowStateMalformed, path)
		}
	}
	if state.LastComplete != nil {
		if state.LastComplete.Artifact != ArtifactRelPath(sp, StageSmoke) || state.LastComplete.CompletedAt.IsZero() || state.LastComplete.InputFingerprint == "" || state.LastComplete.ArtifactDigest == "" {
			return fmt.Errorf("%w: %s: invalid smoke lastComplete", ErrFlowStateMalformed, path)
		}
	}
	return nil
}

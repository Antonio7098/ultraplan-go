package study

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	RunStateSchemaVersion = 1
	RunStateDirName       = ".ultraplan"
	RunStateFileName      = "run-state.json"
)

type Study struct {
	Name string
	Path string
}

type ReportKind string

const (
	ReportKindSource ReportKind = "source"
	ReportKindFinal  ReportKind = "final"
)

type SourceKind string

const (
	SourceKindDirectory SourceKind = "directory"
	SourceKindMarkdown  SourceKind = "markdown"
)

type Source struct {
	Name                 string
	Kind                 SourceKind
	Path                 string
	ApplicableDimensions []string
	Frontmatter          map[string]any
}

type Dimension struct {
	Number               string
	Slug                 string
	File                 string
	Path                 string
	DisableCodeCitations bool
}

type PromptKind string

const (
	PromptKindDirectoryAnalysis PromptKind = "directory_analysis"
	PromptKindMarkdownAnalysis  PromptKind = "markdown_analysis"
	PromptKindSynthesis         PromptKind = "synthesis"
)

type PromptRequest struct {
	WorkspaceRoot string
	Study         Study
	Dimension     Dimension
	Source        Source
}

type PromptManifest struct {
	Kind               PromptKind          `json:"kind"`
	Study              string              `json:"study"`
	Dimension          string              `json:"dimension"`
	Source             string              `json:"source,omitempty"`
	SourceKind         SourceKind          `json:"source_kind,omitempty"`
	Templates          []string            `json:"templates"`
	DimensionPath      string              `json:"dimension_path"`
	InputDocumentPath  string              `json:"input_document_path,omitempty"`
	InputReportPaths   []string            `json:"input_report_paths,omitempty"`
	SourceReports      []SourceReportInput `json:"source_reports,omitempty"`
	ExpectedOutputPath string              `json:"expected_output_path"`
}

type SourceReportInput struct {
	Source     string     `json:"source"`
	SourceKind SourceKind `json:"source_kind"`
	Path       string     `json:"path"`
}

type PromptResult struct {
	Text     string
	Manifest PromptManifest
}

type ValidationStatus string

const (
	ValidationStatusPassed  ValidationStatus = "passed"
	ValidationStatusFailed  ValidationStatus = "failed"
	ValidationStatusSkipped ValidationStatus = "skipped"
)

type ValidationSeverity string

const (
	ValidationSeverityInfo  ValidationSeverity = "info"
	ValidationSeverityWarn  ValidationSeverity = "warn"
	ValidationSeverityError ValidationSeverity = "error"
)

type ValidationCheck struct {
	Name       string             `json:"name"`
	Status     ValidationStatus   `json:"status"`
	Severity   ValidationSeverity `json:"severity"`
	Path       string             `json:"path,omitempty"`
	Expected   string             `json:"expected,omitempty"`
	Observed   string             `json:"observed,omitempty"`
	SourceKind SourceKind         `json:"source_kind,omitempty"`
	Guidance   string             `json:"guidance,omitempty"`
	Err        error              `json:"-"`
}

type ValidationResult struct {
	Kind   ReportKind        `json:"kind"`
	Path   string            `json:"path"`
	Status ValidationStatus  `json:"status"`
	Checks []ValidationCheck `json:"checks"`
	Err    error             `json:"-"`
}

type RatingState string

const (
	RatingStateValid     RatingState = "valid"
	RatingStateMissing   RatingState = "missing"
	RatingStateInvalid   RatingState = "invalid"
	RatingStateAmbiguous RatingState = "ambiguous"
)

type RatingResult struct {
	State  RatingState `json:"state"`
	Score  int         `json:"score,omitempty"`
	Raw    string      `json:"raw,omitempty"`
	Reason string      `json:"reason,omitempty"`
}

type TaskKind string

const (
	TaskKindAnalysis  TaskKind = "analysis"
	TaskKindSynthesis TaskKind = "synthesis"
)

type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusRunning    TaskStatus = "running"
	TaskStatusValidating TaskStatus = "validating"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
	TaskStatusCancelled  TaskStatus = "cancelled"
	TaskStatusSkipped    TaskStatus = "skipped"
	TaskStatusWaiting    TaskStatus = "waiting"
	TaskStatusRetrying   TaskStatus = "retrying"
)

type RunState struct {
	SchemaVersion int           `json:"schema_version"`
	RunID         string        `json:"run_id"`
	Study         string        `json:"study"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
	Filters       RunFilters    `json:"filters"`
	Config        ConfigSummary `json:"config_summary"`
	Tasks         []TaskState   `json:"tasks"`
	Complete      bool          `json:"complete"`
}

type RunFilters struct {
	Dimensions []string `json:"dimensions,omitempty"`
	Sources    []string `json:"sources,omitempty"`
}

type ConfigSummary struct {
	Runtime          string `json:"runtime,omitempty"`
	Model            string `json:"model,omitempty"`
	Variant          string `json:"variant,omitempty"`
	DefaultParallel  int    `json:"default_parallel,omitempty"`
	DefaultTimeout   string `json:"default_timeout,omitempty"`
	DefaultRetries   int    `json:"default_retries,omitempty"`
	WorkspaceVersion string `json:"workspace_version,omitempty"`
}

type TaskState struct {
	ID           string                `json:"id"`
	Kind         TaskKind              `json:"kind"`
	Status       TaskStatus            `json:"status"`
	Study        string                `json:"study"`
	Dimension    string                `json:"dimension,omitempty"`
	DimensionRef string                `json:"dimension_ref,omitempty"`
	Source       string                `json:"source,omitempty"`
	SourceKind   SourceKind            `json:"source_kind,omitempty"`
	OutputPath   string                `json:"output_path"`
	Attempts     int                   `json:"attempts"`
	CreatedAt    time.Time             `json:"created_at"`
	UpdatedAt    time.Time             `json:"updated_at"`
	StartedAt    *time.Time            `json:"started_at,omitempty"`
	CompletedAt  *time.Time            `json:"completed_at,omitempty"`
	RetryAfter   *time.Time            `json:"retry_after,omitempty"`
	LastError    *TaskError            `json:"last_error,omitempty"`
	Validation   *ValidationSummary    `json:"validation,omitempty"`
	Agent        AgentMetadata         `json:"agent,omitempty"`
	Dependencies []SynthesisDependency `json:"dependencies,omitempty"`
}

type TaskError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}

type ValidationSummary struct {
	Status       ValidationStatus `json:"status"`
	CheckedAt    time.Time        `json:"checked_at"`
	Path         string           `json:"path"`
	PassedChecks int              `json:"passed_checks"`
	FailedChecks int              `json:"failed_checks"`
	Message      string           `json:"message,omitempty"`
}

type AgentMetadata struct {
	Runtime string `json:"runtime,omitempty"`
	RunID   string `json:"run_id,omitempty"`
	Model   string `json:"model,omitempty"`
}

type SynthesisDependency struct {
	TaskID     string     `json:"task_id"`
	Source     string     `json:"source"`
	SourceKind SourceKind `json:"source_kind"`
	ReportPath string     `json:"report_path"`
}

type StatusSummary struct {
	Total       int
	Pending     int
	Running     int
	Validating  int
	Completed   int
	Failed      int
	Cancelled   int
	Skipped     int
	Waiting     int
	Retrying    int
	Active      int
	RetryCount  int
	NextRetryAt *time.Time
	Complete    bool
	StatePath   string
	RunID       string
}

type ExecutionStatus string

const (
	ExecutionStatusCompleted        ExecutionStatus = "completed"
	ExecutionStatusSkipped          ExecutionStatus = "skipped"
	ExecutionStatusRuntimeFailed    ExecutionStatus = "runtime_failed"
	ExecutionStatusValidationFailed ExecutionStatus = "validation_failed"
	ExecutionStatusPreflightBlocked ExecutionStatus = "preflight_blocked"
	ExecutionStatusCancelled        ExecutionStatus = "cancelled"
)

type ExecutionRequest struct {
	StudyRef     string
	DimensionRef string
	SourceRef    string
}

type ExecutionResult struct {
	Status           ExecutionStatus
	TaskKind         TaskKind
	Study            Study
	Dimension        Dimension
	Source           Source
	OutputPath       string
	SkippedReason    string
	RuntimeRunID     string
	RuntimeStatus    string
	RuntimeError     string
	RuntimeErr       error
	RuntimeCategory  string
	Warnings         []string
	Validation       ValidationResult
	PreflightResults []ValidationResult
	Blockers         []string
}

type SynthesisRequest struct {
	StudyRef     string
	DimensionRef string
}

func (d Dimension) Ref() string {
	if d.Slug == "" {
		return d.Number
	}
	return d.Number + "-" + d.Slug
}

var dimensionFilePattern = regexp.MustCompile(`^([0-9]+)(?:[-_ ]+(.+))?\.md$`)

func dimensionFromFile(path string) (Dimension, bool) {
	file := filepath.Base(path)
	matches := dimensionFilePattern.FindStringSubmatch(file)
	if matches == nil {
		return Dimension{}, false
	}
	number, ok := normalizeDimensionNumber(matches[1])
	if !ok {
		return Dimension{}, false
	}
	slug := normalizeSlug(matches[2])
	return Dimension{
		Number: number,
		Slug:   slug,
		File:   file,
		Path:   path,
	}, true
}

func normalizeDimensionNumber(raw string) (string, bool) {
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return "", false
	}
	return fmt.Sprintf("%02d", n), true
}

func normalizeDimensionRef(ref string) string {
	if number, ok := normalizeDimensionNumber(ref); ok {
		return number
	}
	return strings.TrimSpace(ref)
}

func normalizeSlug(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimSuffix(raw, ".md")
	raw = strings.Trim(raw, "-_ ")
	raw = strings.ToLower(raw)
	var b strings.Builder
	lastDash := false
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

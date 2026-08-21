package study

import "time"

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
	SchemaVersion            int           `json:"schema_version"`
	RunID                    string        `json:"run_id"`
	Study                    string        `json:"study"`
	CreatedAt                time.Time     `json:"created_at"`
	UpdatedAt                time.Time     `json:"updated_at"`
	Filters                  RunFilters    `json:"filters"`
	Config                   ConfigSummary `json:"config_summary"`
	ApplicabilityFingerprint string        `json:"applicability_fingerprint,omitempty"`
	Tasks                    []TaskState   `json:"tasks"`
	Complete                 bool          `json:"complete"`
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
	Session      *TaskSession          `json:"session,omitempty"`
	Dependencies []SynthesisDependency `json:"dependencies,omitempty"`
}

// TaskSession is the durable checkpoint needed to continue interrupted study
// work without attaching a different task or a changed prompt to the session.
type TaskSession struct {
	SessionID        string    `json:"session_id"`
	Provider         string    `json:"provider,omitempty"`
	Model            string    `json:"model,omitempty"`
	WorkDir          string    `json:"work_dir"`
	InputFingerprint string    `json:"input_fingerprint"`
	UpdatedAt        time.Time `json:"updated_at"`
	ContinueFailures int       `json:"continue_failures,omitempty"`
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
	Lock        *LockInfo
	Tasks       []TaskState
}

// RetrySummary aggregates how often study tasks needed retries and whether
// those retries continued the same agent session or started a fresh one.
type RetrySummary struct {
	RetriedTasks int        `json:"retried_tasks"`
	TotalRetries int        `json:"total_retries"`
	SameSession  int        `json:"same_session"`
	FreshSession int        `json:"fresh_session"`
	Waiting      int        `json:"waiting"`
	NextRetryAt  *time.Time `json:"next_retry_at,omitempty"`
}

// ParallelismThrottle summarizes whether memory pressure reduced the run-loop
// parallelism below the requested level.
type ParallelismThrottle struct {
	Decreased            bool      `json:"decreased"`
	Events               int       `json:"events,omitempty"`
	RequestedParallelism int       `json:"requested_parallelism,omitempty"`
	EffectiveParallelism int       `json:"effective_parallelism,omitempty"`
	MemoryAvailableBytes uint64    `json:"memory_available_bytes,omitempty"`
	LastAt               time.Time `json:"last_at,omitempty"`
}

type LockInfo struct {
	Path       string    `json:"path"`
	Study      string    `json:"study"`
	PID        int       `json:"pid"`
	Command    string    `json:"command"`
	AcquiredAt time.Time `json:"acquired_at"`
}

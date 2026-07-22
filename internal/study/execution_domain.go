package study

import runtimepkg "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"

type TaskKind string

const (
	TaskKindAnalysis  TaskKind = "analysis"
	TaskKindSynthesis TaskKind = "synthesis"
)

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
	OnEvent      func(runtimepkg.Event)
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
	Agent            AgentMetadata
	Warnings         []string
	Validation       ValidationResult
	PreflightResults []ValidationResult
	Blockers         []string
}

type SynthesisRequest struct {
	StudyRef     string
	DimensionRef string
	SourceRefs   []string
	OnEvent      func(runtimepkg.Event)
}

type RunAllRequest struct {
	StudyRef      string
	DimensionRefs []string
	SourceRefs    []string
	Parallelism   int
	Progress      func(RunAllProgress)
}

type RunAllProgress struct {
	TaskKind     TaskKind
	DimensionRef string
	SourceRef    string
	Event        runtimepkg.Event
}

type RunAllStatus string

const (
	RunAllStatusCompleted        RunAllStatus = "completed"
	RunAllStatusPartial          RunAllStatus = "partial"
	RunAllStatusValidationFailed RunAllStatus = "validation_failed"
	RunAllStatusRuntimeFailed    RunAllStatus = "runtime_failed"
	RunAllStatusCancelled        RunAllStatus = "cancelled"
)

type RunAllCounts struct {
	Completed int
	Failed    int
	Skipped   int
	Pending   int
}

type RunAllWarning struct {
	Path    string
	Message string
}

type RunAllResult struct {
	Status        RunAllStatus
	Study         Study
	Parallelism   int
	Analysis      []ExecutionResult
	Synthesis     []ExecutionResult
	Counts        RunAllCounts
	Warnings      []RunAllWarning
	SummaryPath   string
	SummaryResult SummaryResult
}

type RunLoopRequest struct {
	StudyRef      string
	DimensionRefs []string
	SourceRefs    []string
	Parallelism   int
	Config        ConfigSummary
	Command       []string
	ForceUnlock   bool
	Continue      bool
	Reset         bool
	Progress      func(RunLoopProgress)
}

type RunLoopResult struct {
	Status      RunAllStatus
	Study       Study
	Parallelism int
	State       RunState
	StatePath   string
	LockPath    string
	Counts      RunAllCounts
	ScopeCounts RunAllCounts
	Warnings    []RunAllWarning
}

type RunLoopProgress struct {
	Event        RunLoopProgressEvent
	Task         TaskState
	Counts       StatusSummary
	ScopeCounts  StatusSummary
	RuntimeEvent *runtimepkg.Event
}

type RunLoopProgressEvent string

const (
	RunLoopProgressStarted   RunLoopProgressEvent = "started"
	RunLoopProgressCompleted RunLoopProgressEvent = "completed"
	RunLoopProgressFailed    RunLoopProgressEvent = "failed"
	RunLoopProgressWaiting   RunLoopProgressEvent = "waiting"
	RunLoopProgressCancelled RunLoopProgressEvent = "cancelled"
	RunLoopProgressRuntime   RunLoopProgressEvent = "runtime"
)

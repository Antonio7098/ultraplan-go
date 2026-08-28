package sprint

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	QASchemaVersion         = 1
	QAStateSchemaVersion    = 2
	QAEvidenceSchemaVersion = 2
	QAIDScope               = "qa-v1"
	QAEvidenceIDScope       = "qa-v2"
	QARepairSchemaVersion   = 1
	QARepairIDScope         = "repair-v1"
)

type QAPhaseStatus string

const (
	QAPhaseMissing      QAPhaseStatus = "missing"
	QAPhaseMapped       QAPhaseStatus = "mapped"
	QAPhaseQueued       QAPhaseStatus = "queued"
	QAPhaseRunning      QAPhaseStatus = "running"
	QAPhaseSynthesizing QAPhaseStatus = "synthesizing"
	QAPhaseCompleted    QAPhaseStatus = "completed"
	QAPhaseBlocked      QAPhaseStatus = "blocked"
	QAPhaseCancelled    QAPhaseStatus = "cancelled"
	QAPhaseInterrupted  QAPhaseStatus = "interrupted"
	QAPhaseStale        QAPhaseStatus = "stale"
	QAPhaseInvalid      QAPhaseStatus = "invalid"
)

func QAPhaseStatuses() []QAPhaseStatus {
	return []QAPhaseStatus{QAPhaseMissing, QAPhaseMapped, QAPhaseQueued, QAPhaseRunning, QAPhaseSynthesizing, QAPhaseCompleted, QAPhaseBlocked, QAPhaseCancelled, QAPhaseInterrupted, QAPhaseStale, QAPhaseInvalid}
}

type QATheoryOutcome string

const (
	QATheoryConfirmed     QATheoryOutcome = "confirmed"
	QATheoryRefuted       QATheoryOutcome = "refuted"
	QATheoryInvalid       QATheoryOutcome = "invalid"
	QATheoryInconclusive  QATheoryOutcome = "inconclusive"
	QATheoryBlocked       QATheoryOutcome = "blocked"
	QATheoryCrossShard    QATheoryOutcome = "cross_shard"
	QATheoryNotApplicable QATheoryOutcome = "not_applicable"
)

func QATheoryOutcomes() []QATheoryOutcome {
	return []QATheoryOutcome{QATheoryConfirmed, QATheoryRefuted, QATheoryInvalid, QATheoryInconclusive, QATheoryBlocked, QATheoryCrossShard, QATheoryNotApplicable}
}

type QAShardKind string

const (
	QAShardPrimary  QAShardKind = "primary"
	QAShardBoundary QAShardKind = "boundary"
	QAShardFollowUp QAShardKind = "follow_up"
)

type QARunLifecycle string

const (
	QARunUnaccepted QARunLifecycle = "unaccepted"
	QARunAccepted   QARunLifecycle = "accepted"
	QARunClaimed    QARunLifecycle = "claimed"
	QARunActive     QARunLifecycle = "active"
	QARunTerminal   QARunLifecycle = "terminal"
)

type QATerminalResult string

const (
	QATerminalCompleted        QATerminalResult = "completed"
	QATerminalBlocked          QATerminalResult = "blocked"
	QATerminalCancelled        QATerminalResult = "cancelled"
	QATerminalInterrupted      QATerminalResult = "interrupted"
	QATerminalCleanupUncertain QATerminalResult = "cleanup_uncertain"
)

type QABudgets struct {
	ChangedPaths               int           `json:"changed_paths"`
	PrimaryShards              int           `json:"primary_shards"`
	BoundaryShards             int           `json:"boundary_shards"`
	FollowUpShards             int           `json:"follow_up_shards"`
	TotalShards                int           `json:"total_shards"`
	PendingEntries             int           `json:"pending_entries"`
	ChangedPathsPerShard       int           `json:"changed_paths_per_shard"`
	ContextPathsPerShard       int           `json:"context_paths_per_shard"`
	ContextExpansions          int           `json:"context_expansions"`
	PathsPerExpansion          int           `json:"paths_per_expansion"`
	BehavioralConcernsPerShard int           `json:"behavioral_concerns_per_shard"`
	TheoriesPerShard           int           `json:"theories_per_shard"`
	IterationsPerAttempt       int           `json:"iterations_per_attempt"`
	CommandsPerAttempt         int           `json:"commands_per_attempt"`
	OutputRepairAttempts       int           `json:"output_repair_attempts"`
	ConcurrentInvestigators    int           `json:"concurrent_investigators"`
	CommandTimeout             time.Duration `json:"command_timeout"`
	ShardTimeout               time.Duration `json:"shard_timeout"`
	RunTimeout                 time.Duration `json:"run_timeout"`
	CleanupTimeout             time.Duration `json:"cleanup_timeout"`
	CommandOutputBytes         int           `json:"command_output_bytes"`
	ShardOutputBytes           int           `json:"shard_output_bytes"`
	PromptBytes                int           `json:"prompt_bytes"`
	RecentProgress             int           `json:"recent_progress"`
	RetainedAttempts           int           `json:"retained_attempts"`
	StateBytes                 int           `json:"state_bytes"`
	TreeFiles                  int           `json:"tree_files"`
	TreeBytes                  int64         `json:"tree_bytes"`
	FileBytes                  int64         `json:"file_bytes"`
	GeneratedChecks            int           `json:"generated_checks"`
	GeneratedPatchBytes        int           `json:"generated_patch_bytes"`
	EvidenceRecords            int           `json:"evidence_records"`
	Issues                     int           `json:"issues"`
	AnalyzerCalls              int           `json:"analyzer_calls"`
	EvaluatorCalls             int           `json:"evaluator_calls"`
}

func DefaultQABudgets() QABudgets {
	return QABudgets{
		ChangedPaths: 512, PrimaryShards: 32, BoundaryShards: 8, FollowUpShards: 4,
		TotalShards: 44, PendingEntries: 44, ChangedPathsPerShard: 12,
		ContextPathsPerShard: 64, ContextExpansions: 2, PathsPerExpansion: 16,
		BehavioralConcernsPerShard: 12, TheoriesPerShard: 12,
		IterationsPerAttempt: 4, CommandsPerAttempt: 8, OutputRepairAttempts: 1,
		ConcurrentInvestigators: 3, CommandTimeout: 5 * time.Minute,
		ShardTimeout: 20 * time.Minute, RunTimeout: 60 * time.Minute,
		CleanupTimeout: 30 * time.Second, CommandOutputBytes: 256 << 10,
		ShardOutputBytes: 1 << 20, PromptBytes: 512 << 10, RecentProgress: 100,
		RetainedAttempts: 8, StateBytes: 128 << 20,
		TreeFiles: 200_000, TreeBytes: 2 << 30, FileBytes: 32 << 20,
		GeneratedChecks: 88, GeneratedPatchBytes: 2 << 20, EvidenceRecords: 256,
		Issues: 200, AnalyzerCalls: 3, EvaluatorCalls: 3,
	}
}

func MaximumQABudgets() QABudgets {
	return QABudgets{
		ChangedPaths: 512, PrimaryShards: 32, BoundaryShards: 8, FollowUpShards: 4,
		TotalShards: 44, PendingEntries: 44, ChangedPathsPerShard: 64,
		ContextPathsPerShard: 128, ContextExpansions: 4, PathsPerExpansion: 32,
		BehavioralConcernsPerShard: 24, TheoriesPerShard: 24,
		IterationsPerAttempt: 8, CommandsPerAttempt: 16, OutputRepairAttempts: 2,
		ConcurrentInvestigators: 8, CommandTimeout: 10 * time.Minute,
		ShardTimeout: 30 * time.Minute, RunTimeout: 90 * time.Minute,
		CleanupTimeout: 30 * time.Second, CommandOutputBytes: 512 << 10,
		ShardOutputBytes: 2 << 20, PromptBytes: 1 << 20, RecentProgress: 200,
		RetainedAttempts: 8, StateBytes: 128 << 20,
		TreeFiles: 400_000, TreeBytes: 4 << 30, FileBytes: 64 << 20,
		GeneratedChecks: 128, GeneratedPatchBytes: 4 << 20, EvidenceRecords: 512,
		Issues: 200, AnalyzerCalls: 3, EvaluatorCalls: 3,
	}
}

type QAEffectiveSource struct {
	Field  string `json:"field"`
	Source string `json:"source"`
}

type QASettings struct {
	Runtime StageRuntime        `json:"runtime"`
	Budgets QABudgets           `json:"budgets"`
	Sources []QAEffectiveSource `json:"sources"`
}

type QAFreshness struct {
	Current                   bool     `json:"current"`
	Reasons                   []string `json:"reasons,omitempty"`
	GovernedInputFingerprint  string   `json:"governed_input_fingerprint"`
	ImplementationFingerprint string   `json:"implementation_fingerprint"`
	ReviewFingerprint         string   `json:"review_fingerprint"`
	PolicyFingerprint         string   `json:"policy_fingerprint"`
}

type QABlocker struct {
	Category   QAErrorCategory `json:"category"`
	Scope      string          `json:"scope"`
	Summary    string          `json:"summary"`
	NextAction string          `json:"next_action"`
}

type QACancellation struct {
	Requested bool       `json:"requested"`
	Scope     string     `json:"scope,omitempty"`
	ShardID   string     `json:"shard_id,omitempty"`
	Reason    string     `json:"reason,omitempty"`
	At        *time.Time `json:"at,omitempty"`
}

type QAWriterToken struct {
	RunID                string `json:"run_id"`
	OperationalAttemptID string `json:"operational_attempt_id"`
	FencingGeneration    uint64 `json:"fencing_generation"`
}

func (token QAWriterToken) Validate() error {
	if strings.TrimSpace(token.RunID) == "" || strings.TrimSpace(token.OperationalAttemptID) == "" || token.FencingGeneration == 0 {
		return fmt.Errorf("QA writer token requires run ID, operational attempt ID, and fencing generation")
	}
	if strings.ContainsAny(token.RunID+token.OperationalAttemptID, "\x00\r\n") {
		return fmt.Errorf("QA writer token contains unsafe identity bytes")
	}
	return nil
}

type QARunCorrelation struct {
	Lifecycle            QARunLifecycle   `json:"lifecycle"`
	RunID                string           `json:"run_id,omitempty"`
	OperationalAttemptID string           `json:"operational_attempt_id,omitempty"`
	FencingGeneration    uint64           `json:"fencing_generation,omitempty"`
	TerminalResult       QATerminalResult `json:"terminal_result,omitempty"`
}

type QAArtifactRef struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

type QAState struct {
	SchemaVersion        int                     `json:"schema_version"`
	Project              string                  `json:"project"`
	Sprint               string                  `json:"sprint"`
	Phase                QAPhaseStatus           `json:"phase"`
	Freshness            QAFreshness             `json:"freshness"`
	CurrentAttemptID     string                  `json:"current_attempt_id,omitempty"`
	Map                  *QAArtifactRef          `json:"map,omitempty"`
	Synthesis            *QAArtifactRef          `json:"synthesis,omitempty"`
	Adjudication         *QAArtifactRef          `json:"adjudication,omitempty"`
	Issues               *QAArtifactRef          `json:"issues,omitempty"`
	Assessment           *QAArtifactRef          `json:"assessment,omitempty"`
	CanonicalReport      *QAArtifactRef          `json:"canonical_report,omitempty"`
	EvidenceCount        int                     `json:"evidence_count,omitempty"`
	RejectedCount        int                     `json:"rejected_count,omitempty"`
	IssueCount           int                     `json:"issue_count,omitempty"`
	RegressionCandidates int                     `json:"regression_candidates,omitempty"`
	CanonicalAssessment  OverallAssessment       `json:"canonical_assessment,omitempty"`
	CurrentFailure       *QABlocker              `json:"current_failure,omitempty"`
	CompletedShards      int                     `json:"completed_shards"`
	TotalShards          int                     `json:"total_shards"`
	OutcomeCounts        map[QATheoryOutcome]int `json:"outcome_counts,omitempty"`
	Blocker              *QABlocker              `json:"blocker,omitempty"`
	Cancellation         QACancellation          `json:"cancellation"`
	Run                  QARunCorrelation        `json:"run"`
	NextAction           string                  `json:"next_action"`
	UpdatedAt            time.Time               `json:"updated_at"`
}

// QAFlowSummary is the only QA detail stored in flow-state.json. It is a
// bounded pointer projection, not a theory or attempt database.
type QAFlowSummary struct {
	Phase            QAPhaseStatus     `json:"phase"`
	Fresh            bool              `json:"fresh"`
	CompletedShards  int               `json:"completed_shards"`
	TotalShards      int               `json:"total_shards"`
	Confirmed        int               `json:"confirmed"`
	Blocked          int               `json:"blocked"`
	Cancellation     QACancellation    `json:"cancellation"`
	StatePath        string            `json:"state_path"`
	StateDigest      string            `json:"state_digest"`
	CurrentAttemptID string            `json:"current_attempt_id,omitempty"`
	NextAction       string            `json:"next_action"`
	Assessment       OverallAssessment `json:"assessment,omitempty"`
	EvidenceCount    int               `json:"evidence_count,omitempty"`
	RejectedCount    int               `json:"rejected_count,omitempty"`
	IssueCount       int               `json:"issue_count,omitempty"`
	ReportPath       string            `json:"report_path,omitempty"`
	ReportDigest     string            `json:"report_digest,omitempty"`
}

type QATargetIdentity struct {
	Fingerprint       string            `json:"fingerprint"`
	Scope             string            `json:"scope,omitempty"`
	GitHead           string            `json:"git_head,omitempty"`
	GitIndex          string            `json:"git_index,omitempty"`
	GitWorktree       string            `json:"git_worktree,omitempty"`
	WorkspaceBranch   string            `json:"workspace_branch,omitempty"`
	WorkspaceBaseline string            `json:"workspace_baseline,omitempty"`
	BaselineRelation  string            `json:"baseline_relation,omitempty"`
	CommitsSinceBase  int               `json:"commits_since_baseline,omitempty"`
	Categories        map[string]string `json:"categories,omitempty"`
}

type RepairMode string

const (
	RepairModeManual    RepairMode = "manual"
	RepairModeAutomatic RepairMode = "automatic"
)

type RepairPhase string

const (
	RepairPhasePrepared      RepairPhase = "prepared"
	RepairPhaseConfirmed     RepairPhase = "confirmed"
	RepairPhaseProposing     RepairPhase = "proposing"
	RepairPhaseApplying      RepairPhase = "applying"
	RepairPhaseReverifying   RepairPhase = "reverifying"
	RepairPhaseCleaning      RepairPhase = "cleaning"
	RepairPhaseTerminalizing RepairPhase = "terminalizing"
	RepairPhaseTerminal      RepairPhase = "terminal"
	RepairPhaseInterrupted   RepairPhase = "interrupted"
	RepairPhaseStale         RepairPhase = "stale"
)

type RepairOutcome string

const (
	RepairOutcomeVerified             RepairOutcome = "verified"
	RepairOutcomeVerifiedWithFindings RepairOutcome = "verified_with_findings"
	RepairOutcomeFailed               RepairOutcome = "failed"
	RepairOutcomeBlocked              RepairOutcome = "blocked"
	RepairOutcomeEscalated            RepairOutcome = "escalated"
	RepairOutcomeStalled              RepairOutcome = "stalled"
)

type RepairGateKind string

const (
	RepairGateExactReproducer RepairGateKind = "exact_reproducer"
	RepairGatePrimaryShards   RepairGateKind = "primary_shards"
	RepairGateLinkedTheories  RepairGateKind = "linked_theories"
	RepairGateFollowUpShards  RepairGateKind = "follow_up_shards"
	RepairGateContainingQA    RepairGateKind = "containing_qa"
	RepairGateContainingSmoke RepairGateKind = "containing_smoke"
)

type RepairGateStatus string

const (
	RepairGatePending RepairGateStatus = "pending"
	RepairGateRunning RepairGateStatus = "running"
	RepairGatePassed  RepairGateStatus = "passed"
	RepairGateFailed  RepairGateStatus = "failed"
	RepairGateBlocked RepairGateStatus = "blocked"
	RepairGateSkipped RepairGateStatus = "skipped"
)

type RepairStopReason string

const (
	RepairStopNone                RepairStopReason = ""
	RepairStopVerified            RepairStopReason = "verified"
	RepairStopRequiredCheckFailed RepairStopReason = "required_check_failed"
	RepairStopPrerequisite        RepairStopReason = "prerequisite_unavailable"
	RepairStopCancellation        RepairStopReason = "cancelled"
	RepairStopCleanupUncertain    RepairStopReason = "cleanup_uncertain"
	RepairStopScopeGrowth         RepairStopReason = "scope_growth"
	RepairStopSeverityGrowth      RepairStopReason = "severity_growth"
	RepairStopTargetDrift         RepairStopReason = "target_drift"
	RepairStopGovernedDrift       RepairStopReason = "governed_input_drift"
	RepairStopDesignDecision      RepairStopReason = "design_decision"
	RepairStopContradiction       RepairStopReason = "contradictory_expectations"
	RepairStopUnsupportedChange   RepairStopReason = "unsupported_change"
	RepairStopUncertainEvidence   RepairStopReason = "uncertain_evidence"
	RepairStopUnknownSchema       RepairStopReason = "unknown_schema"
	RepairStopStagnation          RepairStopReason = "stagnation"
	RepairStopRepeatedPatch       RepairStopReason = "repeated_patch"
	RepairStopRepeatedTarget      RepairStopReason = "repeated_target"
	RepairStopCycleLimit          RepairStopReason = "cycle_limit"
	RepairStopReopeningLimit      RepairStopReason = "reopening_limit"
)

type RepairBudgets struct {
	MaxCycles         int           `json:"max_cycles"`
	MaxMutationCycles int           `json:"max_mutation_cycles"`
	MaxReopenings     int           `json:"max_reopenings"`
	StagnationLimit   int           `json:"stagnation_limit"`
	MaxFilesPerCycle  int           `json:"max_files_per_cycle"`
	MaxFilesPerRun    int           `json:"max_files_per_run"`
	MaxBytesPerCycle  int64         `json:"max_bytes_per_cycle"`
	MaxBytesPerRun    int64         `json:"max_bytes_per_run"`
	MaxPatchBytes     int           `json:"max_patch_bytes"`
	WallTime          time.Duration `json:"wall_time"`
	RuntimeAttempts   int           `json:"runtime_attempts"`
	ModelTurns        int           `json:"model_turns"`
	CommandCount      int           `json:"command_count"`
	CommandTimeout    time.Duration `json:"command_timeout"`
	OutputBytes       int           `json:"output_bytes"`
	RetainedCycles    int           `json:"retained_cycles"`
	CleanupTimeout    time.Duration `json:"cleanup_timeout"`
}

func DefaultRepairBudgets() RepairBudgets {
	return RepairBudgets{
		MaxCycles: 3, MaxMutationCycles: 3, MaxReopenings: 1, StagnationLimit: 1,
		MaxFilesPerCycle: 8, MaxFilesPerRun: 16, MaxBytesPerCycle: 256 << 10,
		MaxBytesPerRun: 512 << 10, MaxPatchBytes: 512 << 10, WallTime: 45 * time.Minute,
		RuntimeAttempts: 3, ModelTurns: 12, CommandCount: 32, CommandTimeout: 10 * time.Minute,
		OutputBytes: 1 << 20, RetainedCycles: 8, CleanupTimeout: 30 * time.Second,
	}
}

func MaximumRepairBudgets() RepairBudgets {
	return RepairBudgets{
		MaxCycles: 5, MaxMutationCycles: 5, MaxReopenings: 2, StagnationLimit: 2,
		MaxFilesPerCycle: 16, MaxFilesPerRun: 32, MaxBytesPerCycle: 1 << 20,
		MaxBytesPerRun: 2 << 20, MaxPatchBytes: 1 << 20, WallTime: 90 * time.Minute,
		RuntimeAttempts: 5, ModelTurns: 20, CommandCount: 64, CommandTimeout: 15 * time.Minute,
		OutputBytes: 2 << 20, RetainedCycles: 12, CleanupTimeout: 60 * time.Second,
	}
}

type RepairConsumed struct {
	Cycles          int   `json:"cycles"`
	MutationCycles  int   `json:"mutation_cycles"`
	Reopenings      int   `json:"reopenings"`
	StagnantCycles  int   `json:"stagnant_cycles"`
	ChangedFiles    int   `json:"changed_files"`
	ChangedBytes    int64 `json:"changed_bytes"`
	RuntimeAttempts int   `json:"runtime_attempts"`
	ModelTurns      int   `json:"model_turns"`
	Commands        int   `json:"commands"`
	OutputBytes     int   `json:"output_bytes"`
}

type RepairCheckDescriptor struct {
	ID               string         `json:"id"`
	Gate             RepairGateKind `json:"gate"`
	Executable       string         `json:"executable"`
	Args             []string       `json:"args,omitempty"`
	Workdir          string         `json:"workdir,omitempty"`
	EnvironmentNames []string       `json:"environment_names,omitempty"`
	Timeout          time.Duration  `json:"timeout"`
	OutputLimit      int            `json:"output_limit"`
	Expected         string         `json:"expected"`
	SourcePlanID     string         `json:"source_plan_id,omitempty"`
}

type RepairIssuePacket struct {
	SchemaVersion             int                     `json:"schema_version"`
	Project                   string                  `json:"project"`
	Sprint                    string                  `json:"sprint"`
	QAAttemptID               string                  `json:"qa_attempt_id"`
	RepairRunID               string                  `json:"repair_run_id"`
	Issue                     QAIssue                 `json:"issue"`
	RootCauseGroup            QARootCauseGroup        `json:"root_cause_group"`
	AdjudicationID            string                  `json:"adjudication_id"`
	EvidenceIDs               []string                `json:"evidence_ids"`
	PlanIDs                   []string                `json:"plan_ids"`
	MapID                     string                  `json:"map_id"`
	ShardIDs                  []string                `json:"shard_ids"`
	TheoryIDs                 []string                `json:"theory_ids,omitempty"`
	ExpectationRefs           []string                `json:"expectation_refs"`
	ExactReproducer           RepairCheckDescriptor   `json:"exact_reproducer"`
	Checks                    []RepairCheckDescriptor `json:"checks"`
	AllowedPaths              []string                `json:"allowed_paths"`
	ForbiddenPaths            []string                `json:"forbidden_paths"`
	AcceptanceCriteria        []string                `json:"acceptance_criteria"`
	Mode                      RepairMode              `json:"mode"`
	Budgets                   RepairBudgets           `json:"budgets"`
	BudgetSources             []QAEffectiveSource     `json:"budget_sources"`
	Target                    QATargetIdentity        `json:"target"`
	GovernedInputFingerprint  string                  `json:"governed_input_fingerprint"`
	ImplementationFingerprint string                  `json:"implementation_fingerprint"`
	ReviewFingerprint         string                  `json:"review_fingerprint"`
	SmokeFingerprint          string                  `json:"smoke_fingerprint"`
	PolicyFingerprint         string                  `json:"policy_fingerprint"`
	IsolationFingerprint      string                  `json:"isolation_fingerprint"`
	PreparedAt                time.Time               `json:"prepared_at"`
	PacketDigest              string                  `json:"packet_digest"`
}

type RepairConfirmation struct {
	SchemaVersion            int              `json:"schema_version"`
	Project                  string           `json:"project"`
	Sprint                   string           `json:"sprint"`
	QAAttemptID              string           `json:"qa_attempt_id"`
	RepairRunID              string           `json:"repair_run_id"`
	PacketDigest             string           `json:"packet_digest"`
	Target                   QATargetIdentity `json:"target"`
	Mode                     RepairMode       `json:"mode"`
	AutomaticOptIn           bool             `json:"automatic_opt_in"`
	Budgets                  RepairBudgets    `json:"budgets"`
	GovernedInputFingerprint string           `json:"governed_input_fingerprint"`
	PolicyFingerprint        string           `json:"policy_fingerprint"`
	OperationRunID           string           `json:"operation_run_id"`
	OperationalAttemptID     string           `json:"operational_attempt_id"`
	FencingGeneration        uint64           `json:"fencing_generation"`
	Confirmer                string           `json:"confirmer"`
	ConfirmedAt              time.Time        `json:"confirmed_at"`
	ConfirmationDigest       string           `json:"confirmation_digest"`
}

type RepairFreshness struct {
	Current bool     `json:"current"`
	Reasons []string `json:"reasons,omitempty"`
}

type RepairState struct {
	SchemaVersion int                       `json:"schema_version"`
	Project       string                    `json:"project"`
	Sprint        string                    `json:"sprint"`
	QAAttemptID   string                    `json:"qa_attempt_id"`
	RepairRunID   string                    `json:"repair_run_id"`
	Mode          RepairMode                `json:"mode"`
	Phase         RepairPhase               `json:"phase"`
	Freshness     RepairFreshness           `json:"freshness"`
	Run           QARunCorrelation          `json:"run"`
	Packet        *QAArtifactRef            `json:"packet,omitempty"`
	Confirmation  *QAArtifactRef            `json:"confirmation,omitempty"`
	CurrentCycle  int                       `json:"current_cycle"`
	EarliestCycle int                       `json:"earliest_retained_cycle"`
	Consumed      RepairConsumed            `json:"consumed"`
	Runtime       *RepairRuntimeObservation `json:"runtime,omitempty"`
	Deadline      time.Time                 `json:"deadline"`
	Outcome       RepairOutcome             `json:"outcome,omitempty"`
	StopReason    RepairStopReason          `json:"stop_reason,omitempty"`
	Blocker       *QABlocker                `json:"blocker,omitempty"`
	Result        *QAArtifactRef            `json:"result,omitempty"`
	NextAction    string                    `json:"next_action"`
	UpdatedAt     time.Time                 `json:"updated_at"`
}

type RepairRuntimeObservation struct {
	Provider          string         `json:"provider"`
	Model             string         `json:"model"`
	Variant           string         `json:"variant,omitempty"`
	SessionID         string         `json:"session_id,omitempty"`
	Usage             QAUsageSummary `json:"usage"`
	EstimatedCost     *QACostSummary `json:"estimated_cost,omitempty"`
	StartedAt         time.Time      `json:"started_at"`
	CompletedAt       time.Time      `json:"completed_at"`
	Duration          time.Duration  `json:"duration"`
	DurationMS        int64          `json:"duration_ms"`
	RuntimeEvents     int64          `json:"runtime_events"`
	RetainedEvents    int            `json:"retained_events"`
	ObservedToolCalls int            `json:"observed_tool_calls"`
}

type RepairScopeRecord struct {
	SchemaVersion int              `json:"schema_version"`
	RepairRunID   string           `json:"repair_run_id"`
	Cycle         int              `json:"cycle"`
	Before        QATargetIdentity `json:"before"`
	After         QATargetIdentity `json:"after"`
	IntendedPaths []string         `json:"intended_paths"`
	ActualPaths   []string         `json:"actual_paths"`
	ChangedBytes  int64            `json:"changed_bytes"`
	Enforced      bool             `json:"enforced"`
	Reason        string           `json:"reason,omitempty"`
}

type RepairGateResult struct {
	Gate        RepairGateKind   `json:"gate"`
	Status      RepairGateStatus `json:"status"`
	Reason      string           `json:"reason,omitempty"`
	NextAction  string           `json:"next_action,omitempty"`
	ExitCode    int              `json:"exit_code,omitempty"`
	Duration    time.Duration    `json:"duration,omitempty"`
	DurationMS  int64            `json:"duration_ms,omitempty"`
	OutputHash  string           `json:"output_digest,omitempty"`
	OutputBytes int              `json:"output_bytes,omitempty"`
	Diagnostic  string           `json:"diagnostic,omitempty"`
}

type RepairReverification struct {
	SchemaVersion         int                `json:"schema_version"`
	RepairRunID           string             `json:"repair_run_id"`
	Cycle                 int                `json:"cycle"`
	Gates                 []RepairGateResult `json:"gates"`
	IssueIDsBefore        []string           `json:"issue_ids_before"`
	IssueIDsAfter         []string           `json:"issue_ids_after"`
	HighestSeverityBefore string             `json:"highest_severity_before,omitempty"`
	HighestSeverityAfter  string             `json:"highest_severity_after,omitempty"`
	ScopeGrew             bool               `json:"scope_grew"`
	SeverityGrew          bool               `json:"severity_grew"`
	Contradiction         bool               `json:"contradiction"`
	CompletedAt           time.Time          `json:"completed_at"`
}

type RepairCleanup struct {
	SchemaVersion         int           `json:"schema_version"`
	RepairRunID           string        `json:"repair_run_id"`
	Cycle                 int           `json:"cycle"`
	ProcessTreeTerminated bool          `json:"process_tree_terminated"`
	WorkspaceRemoved      bool          `json:"workspace_removed"`
	CompensationKnown     bool          `json:"compensation_known"`
	TargetCurrent         bool          `json:"target_current"`
	LeaseReleased         bool          `json:"lease_released"`
	Complete              bool          `json:"complete"`
	Duration              time.Duration `json:"duration"`
	DurationMS            int64         `json:"duration_ms"`
	Diagnostic            string        `json:"diagnostic,omitempty"`
}

type RepairProgressFact struct {
	ExactFailureRemoved bool   `json:"exact_failure_removed"`
	IssueCountBefore    int    `json:"issue_count_before"`
	IssueCountAfter     int    `json:"issue_count_after"`
	SeverityBefore      string `json:"severity_before,omitempty"`
	SeverityAfter       string `json:"severity_after,omitempty"`
}

type RepairCycle struct {
	SchemaVersion  int                `json:"schema_version"`
	RepairRunID    string             `json:"repair_run_id"`
	Number         int                `json:"number"`
	Proposal       *QAArtifactRef     `json:"proposal,omitempty"`
	Scope          *QAArtifactRef     `json:"scope,omitempty"`
	Reverification *QAArtifactRef     `json:"reverification,omitempty"`
	Cleanup        *QAArtifactRef     `json:"cleanup,omitempty"`
	Progress       RepairProgressFact `json:"progress"`
	StopReason     RepairStopReason   `json:"stop_reason,omitempty"`
	StartedAt      time.Time          `json:"started_at"`
	CompletedAt    *time.Time         `json:"completed_at,omitempty"`
}

type RepairApplyOperation struct {
	Path            string `json:"path"`
	PreimageDigest  string `json:"preimage_digest,omitempty"`
	PreimagePath    string `json:"preimage_path,omitempty"`
	PostimageDigest string `json:"postimage_digest,omitempty"`
	StagedPath      string `json:"staged_path,omitempty"`
	Applied         bool   `json:"applied"`
	Restored        bool   `json:"restored"`
}

type RepairApplyJournal struct {
	SchemaVersion int                    `json:"schema_version"`
	RepairRunID   string                 `json:"repair_run_id"`
	Cycle         int                    `json:"cycle"`
	State         string                 `json:"state"`
	Operations    []RepairApplyOperation `json:"operations"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

type RepairResult struct {
	SchemaVersion     int                       `json:"schema_version"`
	Project           string                    `json:"project"`
	Sprint            string                    `json:"sprint"`
	QAAttemptID       string                    `json:"qa_attempt_id"`
	RepairRunID       string                    `json:"repair_run_id"`
	Mode              RepairMode                `json:"mode"`
	Outcome           RepairOutcome             `json:"outcome"`
	Reason            string                    `json:"reason"`
	StopReason        RepairStopReason          `json:"stop_reason"`
	Consumed          RepairConsumed            `json:"consumed"`
	Runtime           *RepairRuntimeObservation `json:"runtime,omitempty"`
	Target            QATargetIdentity          `json:"target"`
	CleanupComplete   bool                      `json:"cleanup_complete"`
	ProductionApplied bool                      `json:"production_applied"`
	CompleteLadder    bool                      `json:"complete_ladder"`
	UnresolvedIssues  []string                  `json:"unresolved_issues,omitempty"`
	Evidence          []QAArtifactRef           `json:"evidence"`
	NextAction        string                    `json:"next_action"`
	CompletedAt       time.Time                 `json:"completed_at"`
}

type ManualRepairProof struct {
	SchemaVersion             int              `json:"schema_version"`
	Project                   string           `json:"project"`
	Sprint                    string           `json:"sprint"`
	RepairRunID               string           `json:"repair_run_id"`
	PacketDigest              string           `json:"packet_digest"`
	ResultDigest              string           `json:"result_digest"`
	Outcome                   RepairOutcome    `json:"outcome"`
	Target                    QATargetIdentity `json:"target"`
	ProtocolFingerprint       string           `json:"protocol_fingerprint"`
	ImplementationFingerprint string           `json:"implementation_fingerprint"`
	PolicyFingerprint         string           `json:"policy_fingerprint"`
	IsolationFingerprint      string           `json:"isolation_fingerprint"`
	GovernedInputFingerprint  string           `json:"governed_input_fingerprint"`
	RuntimeFingerprint        string           `json:"runtime_fingerprint"`
	CleanupComplete           bool             `json:"cleanup_complete"`
	ProductionApplied         bool             `json:"production_applied"`
	CompleteLadder            bool             `json:"complete_ladder"`
	PublishedAt               time.Time        `json:"published_at"`
}

type RepairFlowSummary struct {
	Phase        RepairPhase      `json:"phase"`
	Fresh        bool             `json:"fresh"`
	Mode         RepairMode       `json:"mode"`
	RepairRunID  string           `json:"repair_run_id"`
	QAAttemptID  string           `json:"qa_attempt_id"`
	CurrentCycle int              `json:"current_cycle"`
	Outcome      RepairOutcome    `json:"outcome,omitempty"`
	StopReason   RepairStopReason `json:"stop_reason,omitempty"`
	StatePath    string           `json:"state_path"`
	StateDigest  string           `json:"state_digest"`
	NextAction   string           `json:"next_action"`
}

type QAApprovedCheckRef struct {
	ID          string `json:"id"`
	Fingerprint string `json:"fingerprint"`
}

type QACoverage struct {
	ChangedPaths     []string            `json:"changed_paths"`
	PrimaryOwners    map[string]string   `json:"primary_owners"`
	BoundaryOverlaps map[string][]string `json:"boundary_overlaps,omitempty"`
	BlockedPaths     []string            `json:"blocked_paths,omitempty"`
}

type QAShard struct {
	SchemaVersion      int                     `json:"schema_version"`
	ID                 string                  `json:"id"`
	AttemptID          string                  `json:"attempt_id"`
	Kind               QAShardKind             `json:"kind"`
	Title              string                  `json:"title"`
	ChangedPaths       []string                `json:"changed_paths,omitempty"`
	ContextPaths       []string                `json:"context_paths,omitempty"`
	OverlapPaths       []string                `json:"overlap_paths,omitempty"`
	BoundaryReason     string                  `json:"boundary_reason,omitempty"`
	BehavioralConcerns []string                `json:"behavioral_concerns"`
	ExpectationRefs    []string                `json:"expectation_refs"`
	RiskTags           []string                `json:"risk_tags,omitempty"`
	ApprovedChecks     []QAApprovedCheckRef    `json:"approved_checks,omitempty"`
	ParentTheoryIDs    []string                `json:"parent_theory_ids,omitempty"`
	Phase              QAPhaseStatus           `json:"phase"`
	Blocker            *QABlocker              `json:"blocker,omitempty"`
	Attempts           []QAInvestigatorAttempt `json:"attempts,omitempty"`
	Theories           []QATheory              `json:"theories,omitempty"`
}

type QAMap struct {
	SchemaVersion             int                 `json:"schema_version"`
	ID                        string              `json:"id"`
	Project                   string              `json:"project"`
	Sprint                    string              `json:"sprint"`
	SemanticAttemptID         string              `json:"semantic_attempt_id"`
	GovernedInputFingerprint  string              `json:"governed_input_fingerprint"`
	ImplementationFingerprint string              `json:"implementation_fingerprint"`
	ReviewFingerprint         string              `json:"review_fingerprint"`
	PolicyFingerprint         string              `json:"policy_fingerprint"`
	CheckCatalogFingerprint   string              `json:"check_catalog_fingerprint"`
	Budgets                   QABudgets           `json:"budgets"`
	EffectiveSources          []QAEffectiveSource `json:"effective_sources"`
	Target                    QATargetIdentity    `json:"target"`
	Coverage                  QACoverage          `json:"coverage"`
	Shards                    []QAShard           `json:"shards"`
	InputRefs                 []QAArtifactRef     `json:"input_refs"`
}

type QAEvidenceSummary struct {
	Kind         string   `json:"kind"`
	Summary      string   `json:"summary"`
	Paths        []string `json:"paths,omitempty"`
	CheckID      string   `json:"check_id,omitempty"`
	OutputDigest string   `json:"output_digest,omitempty"`
}

type QAContextRequest struct {
	ID           string   `json:"id"`
	Paths        []string `json:"paths"`
	Reason       string   `json:"reason"`
	Approved     bool     `json:"approved"`
	DeniedReason string   `json:"denied_reason,omitempty"`
}

type QACommandSummary struct {
	CheckID               string        `json:"check_id"`
	DescriptorFingerprint string        `json:"descriptor_fingerprint"`
	ExitCode              int           `json:"exit_code"`
	Duration              time.Duration `json:"duration"`
	StdoutDigest          string        `json:"stdout_digest,omitempty"`
	StderrDigest          string        `json:"stderr_digest,omitempty"`
	OutputBytes           int           `json:"output_bytes"`
	Truncated             bool          `json:"truncated"`
}

type QAInvestigatorAttempt struct {
	ID                   string              `json:"id"`
	Number               int                 `json:"number"`
	StartedAt            time.Time           `json:"started_at"`
	CompletedAt          *time.Time          `json:"completed_at,omitempty"`
	ImplementationBefore string              `json:"implementation_before"`
	ImplementationAfter  string              `json:"implementation_after,omitempty"`
	ContextRequests      []QAContextRequest  `json:"context_requests,omitempty"`
	Commands             []QACommandSummary  `json:"commands,omitempty"`
	Evidence             []QAEvidenceSummary `json:"evidence,omitempty"`
	Usage                QAUsageSummary      `json:"usage"`
	RuntimeEvents        int64               `json:"runtime_events"`
	RetainedEvents       int                 `json:"retained_events"`
	ObservedToolCalls    int                 `json:"observed_tool_calls"`
	EstimatedCost        *QACostSummary      `json:"estimated_cost,omitempty"`
	FailureKind          string              `json:"failure_kind,omitempty"`
	Retryable            bool                `json:"retryable,omitempty"`
	StopReason           string              `json:"stop_reason,omitempty"`
	OutputDiagnostic     *QAOutputDiagnostic `json:"output_diagnostic,omitempty"`
	Repair               *QARepairDiagnostic `json:"repair,omitempty"`
}

// QAOutputDiagnostic retains bounded facts about rejected investigator output
// without persisting the model's response.
type QAOutputDiagnostic struct {
	Kind        string `json:"kind"`
	Detail      string `json:"detail,omitempty"`
	Source      string `json:"source,omitempty"`
	OutputBytes int    `json:"output_bytes,omitempty"`
	EventCount  int    `json:"event_count,omitempty"`
	Status      string `json:"status,omitempty"`
	Session     bool   `json:"session"`
	UsageKnown  bool   `json:"usage_known"`
}

type QARepairDiagnostic struct {
	Attempted              bool   `json:"attempted"`
	MaxAttempts            int    `json:"max_attempts"`
	AttemptCount           int    `json:"attempt_count"`
	Exhausted              bool   `json:"exhausted"`
	ExhaustedReason        string `json:"exhausted_reason,omitempty"`
	PermissionDenied       bool   `json:"permission_denied"`
	UnsupportedSameSession bool   `json:"unsupported_same_session"`
}

type QAUsageSummary struct {
	InputTokensKnown      bool  `json:"input_tokens_known"`
	InputTokens           int64 `json:"input_tokens,omitempty"`
	OutputTokensKnown     bool  `json:"output_tokens_known"`
	OutputTokens          int64 `json:"output_tokens,omitempty"`
	TotalTokensKnown      bool  `json:"total_tokens_known"`
	TotalTokens           int64 `json:"total_tokens,omitempty"`
	ReasoningTokensKnown  bool  `json:"reasoning_tokens_known"`
	ReasoningTokens       int64 `json:"reasoning_tokens,omitempty"`
	CacheReadTokensKnown  bool  `json:"cache_read_tokens_known"`
	CacheReadTokens       int64 `json:"cache_read_tokens,omitempty"`
	CacheWriteTokensKnown bool  `json:"cache_write_tokens_known"`
	CacheWriteTokens      int64 `json:"cache_write_tokens,omitempty"`
	TurnsKnown            bool  `json:"turns_known"`
	Turns                 int64 `json:"turns,omitempty"`
}

type QACostSummary struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
	Estimate bool    `json:"estimate"`
	Source   string  `json:"source"`
}

type QATheory struct {
	SchemaVersion             int                     `json:"schema_version"`
	ID                        string                  `json:"id"`
	ShardID                   string                  `json:"shard_id"`
	Claim                     string                  `json:"claim"`
	Basis                     string                  `json:"basis"`
	VerificationSurface       string                  `json:"verification_surface"`
	ExpectationRefs           []string                `json:"expectation_refs"`
	SeverityIfConfirmed       string                  `json:"severity_if_confirmed"`
	ConfirmationCondition     string                  `json:"confirmation_condition"`
	RefutationCondition       string                  `json:"refutation_condition"`
	InconclusiveCondition     string                  `json:"inconclusive_condition"`
	SafeEvidenceStrategy      string                  `json:"safe_evidence_strategy"`
	ImplementationFingerprint string                  `json:"implementation_fingerprint"`
	AttemptHistory            []QAInvestigatorAttempt `json:"attempt_history"`
	Evidence                  []QAEvidenceSummary     `json:"evidence,omitempty"`
	Outcome                   QATheoryOutcome         `json:"outcome"`
	OutcomeReason             string                  `json:"outcome_reason"`
}

// QAChallenge is a bounded, validated challenger proposal. It is an explicit
// synthesis input, not an adjudication or a change to any theory outcome.
type QAChallenge struct {
	SchemaVersion        int      `json:"schema_version"`
	ID                   string   `json:"id"`
	MapID                string   `json:"map_id"`
	TheoryIDs            []string `json:"theory_ids"`
	Claim                string   `json:"claim"`
	Basis                string   `json:"basis"`
	SafeEvidenceStrategy string   `json:"safe_evidence_strategy"`
	EvidenceRefs         []string `json:"evidence_refs,omitempty"`
}

type QASynthesis struct {
	SchemaVersion  int                     `json:"schema_version"`
	ID             string                  `json:"id"`
	AttemptID      string                  `json:"attempt_id"`
	MapID          string                  `json:"map_id"`
	TheoryIDs      []string                `json:"theory_ids"`
	Challenges     []QAChallenge           `json:"challenges,omitempty"`
	Deduplicated   map[string][]string     `json:"deduplicated,omitempty"`
	Contradictions [][]string              `json:"contradictions,omitempty"`
	Interactions   []string                `json:"interactions,omitempty"`
	FollowUpShards []QAShard               `json:"follow_up_shards,omitempty"`
	OutcomeCounts  map[QATheoryOutcome]int `json:"outcome_counts"`
	Blockers       []QABlocker             `json:"blockers,omitempty"`
	NextAction     string                  `json:"next_action"`
}

type QASemanticIdentity struct {
	GovernedInputFingerprint  string   `json:"governed_input_fingerprint"`
	ImplementationFingerprint string   `json:"implementation_fingerprint"`
	ReviewFingerprint         string   `json:"review_fingerprint"`
	PolicyFingerprint         string   `json:"policy_fingerprint"`
	ChangedPaths              []string `json:"changed_paths"`
}

type QASynthesisIdentity struct {
	MapID             string   `json:"map_id"`
	TheoryIDs         []string `json:"theory_ids"`
	ChallengeIDs      []string `json:"challenge_ids,omitempty"`
	FollowUpIDs       []string `json:"follow_up_ids,omitempty"`
	PolicyFingerprint string   `json:"policy_fingerprint"`
}

type QAShardIdentity struct {
	Kind               QAShardKind `json:"kind"`
	ChangedPaths       []string    `json:"changed_paths,omitempty"`
	ContextPaths       []string    `json:"context_paths,omitempty"`
	BehavioralConcerns []string    `json:"behavioral_concerns"`
	ExpectationRefs    []string    `json:"expectation_refs"`
	ParentTheoryIDs    []string    `json:"parent_theory_ids,omitempty"`
}

type QATheoryIdentity struct {
	Claim               string   `json:"claim"`
	Basis               string   `json:"basis"`
	VerificationSurface string   `json:"verification_surface"`
	ExpectationRefs     []string `json:"expectation_refs"`
}

type QAChallengeIdentity struct {
	TheoryIDs            []string `json:"theory_ids"`
	Claim                string   `json:"claim"`
	Basis                string   `json:"basis"`
	SafeEvidenceStrategy string   `json:"safe_evidence_strategy"`
	EvidenceRefs         []string `json:"evidence_refs,omitempty"`
}

func NewQASemanticAttemptID(project, sprint string, identity QASemanticIdentity) (string, error) {
	return newQAID("attempt", project, sprint, "", identity)
}

func NewQAMapID(project, sprint, semanticAttemptID string, identity QASemanticIdentity) (string, error) {
	return newQAID("map", project, sprint, semanticAttemptID, identity)
}

func NewQAShardID(project, sprint, mapID string, identity QAShardIdentity) (string, error) {
	return newQAID("shard", project, sprint, mapID, identity)
}

func NewQATheoryID(project, sprint, shardID string, identity QATheoryIdentity) (string, error) {
	return newQAID("theory", project, sprint, shardID, identity)
}

func NewQAChallengeID(project, sprint, mapID string, identity QAChallengeIdentity) (string, error) {
	return newQAID("challenge", project, sprint, mapID, identity)
}

func NewQASynthesisID(project, sprint, attemptID string, identity QASynthesisIdentity) (string, error) {
	return newQAID("synthesis", project, sprint, attemptID, identity)
}

func newQAID(kind, project, sprint, parent string, content any) (string, error) {
	if !safeQAName(project) || !safeQAName(sprint) || strings.TrimSpace(kind) == "" {
		return "", fmt.Errorf("invalid QA identity scope")
	}
	data, err := canonicalQAJSON(content)
	if err != nil {
		return "", fmt.Errorf("canonicalize QA identity: %w", err)
	}
	digest := sha256.Sum256(bytes.Join([][]byte{[]byte(QAIDScope), []byte(kind), []byte(project), []byte(sprint), []byte(parent), data}, []byte{0}))
	return fmt.Sprintf("%s-%s-%s", QAIDScope, kind, hex.EncodeToString(digest[:12])), nil
}

func canonicalQAJSON(value any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

var qaIDPattern = regexp.MustCompile(`^qa-v1-(attempt|map|shard|theory|challenge|synthesis)-[0-9a-f]{24}$`)

func validQAID(value string) bool { return qaIDPattern.MatchString(value) }

func safeQAName(value string) bool {
	if value == "" || len(value) > 128 || strings.Contains(value, "..") {
		return false
	}
	for _, r := range value {
		if !(r == '-' || r == '_' || r == '.' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func validFingerprint(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func ValidateQASettings(settings QASettings) error {
	if strings.TrimSpace(settings.Runtime.Model) == "" {
		return fmt.Errorf("qa model is required")
	}
	if err := validateQABudgets(settings.Budgets); err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, source := range settings.Sources {
		if strings.TrimSpace(source.Field) == "" || strings.TrimSpace(source.Source) == "" {
			return fmt.Errorf("QA effective source requires field and source")
		}
		if _, duplicate := seen[source.Field]; duplicate {
			return fmt.Errorf("duplicate QA effective source %q", source.Field)
		}
		seen[source.Field] = struct{}{}
	}
	return nil
}

func validateQABudgets(got QABudgets) error {
	max := MaximumQABudgets()
	ints := []struct {
		name     string
		got, max int
	}{
		{"changed_paths", got.ChangedPaths, max.ChangedPaths}, {"primary_shards", got.PrimaryShards, max.PrimaryShards}, {"boundary_shards", got.BoundaryShards, max.BoundaryShards},
		{"follow_up_shards", got.FollowUpShards, max.FollowUpShards}, {"total_shards", got.TotalShards, max.TotalShards}, {"pending_entries", got.PendingEntries, max.PendingEntries},
		{"changed_paths_per_shard", got.ChangedPathsPerShard, max.ChangedPathsPerShard}, {"context_paths_per_shard", got.ContextPathsPerShard, max.ContextPathsPerShard},
		{"context_expansions", got.ContextExpansions, max.ContextExpansions}, {"paths_per_expansion", got.PathsPerExpansion, max.PathsPerExpansion},
		{"behavioral_concerns_per_shard", got.BehavioralConcernsPerShard, max.BehavioralConcernsPerShard}, {"theories_per_shard", got.TheoriesPerShard, max.TheoriesPerShard},
		{"iterations_per_attempt", got.IterationsPerAttempt, max.IterationsPerAttempt}, {"commands_per_attempt", got.CommandsPerAttempt, max.CommandsPerAttempt},
		{"output_repair_attempts", got.OutputRepairAttempts, max.OutputRepairAttempts}, {"concurrent_investigators", got.ConcurrentInvestigators, max.ConcurrentInvestigators},
		{"command_output_bytes", got.CommandOutputBytes, max.CommandOutputBytes}, {"shard_output_bytes", got.ShardOutputBytes, max.ShardOutputBytes},
		{"prompt_bytes", got.PromptBytes, max.PromptBytes}, {"recent_progress", got.RecentProgress, max.RecentProgress},
		{"retained_attempts", got.RetainedAttempts, max.RetainedAttempts}, {"state_bytes", got.StateBytes, max.StateBytes},
		{"tree_files", got.TreeFiles, max.TreeFiles}, {"generated_checks", got.GeneratedChecks, max.GeneratedChecks},
		{"generated_patch_bytes", got.GeneratedPatchBytes, max.GeneratedPatchBytes}, {"evidence_records", got.EvidenceRecords, max.EvidenceRecords},
		{"issues", got.Issues, max.Issues}, {"analyzer_calls", got.AnalyzerCalls, max.AnalyzerCalls}, {"evaluator_calls", got.EvaluatorCalls, max.EvaluatorCalls},
	}
	for _, limit := range ints {
		if limit.got <= 0 || limit.got > limit.max {
			return fmt.Errorf("qa budget %s must be between 1 and %d", limit.name, limit.max)
		}
	}
	durations := []struct {
		name     string
		got, max time.Duration
	}{{"command_timeout", got.CommandTimeout, max.CommandTimeout}, {"shard_timeout", got.ShardTimeout, max.ShardTimeout}, {"run_timeout", got.RunTimeout, max.RunTimeout}, {"cleanup_timeout", got.CleanupTimeout, max.CleanupTimeout}}
	for _, limit := range durations {
		if limit.got <= 0 || limit.got > limit.max {
			return fmt.Errorf("qa budget %s must be positive and no greater than %s", limit.name, limit.max)
		}
	}
	for _, limit := range []struct {
		name     string
		got, max int64
	}{{"tree_bytes", got.TreeBytes, max.TreeBytes}, {"file_bytes", got.FileBytes, max.FileBytes}} {
		if limit.got <= 0 || limit.got > limit.max {
			return fmt.Errorf("qa budget %s must be between 1 and %d", limit.name, limit.max)
		}
	}
	if got.AnalyzerCalls != 3 || got.EvaluatorCalls != 3 {
		return fmt.Errorf("qa semantic analyzer and evaluator counts must be exactly 3")
	}
	return nil
}

func ValidateQATheory(theory QATheory) error {
	if theory.SchemaVersion != QASchemaVersion || !validQAID(theory.ID) || !validQAID(theory.ShardID) {
		return fmt.Errorf("invalid QA theory schema or identity")
	}
	required := map[string]string{"claim": theory.Claim, "basis": theory.Basis, "verification_surface": theory.VerificationSurface, "severity_if_confirmed": theory.SeverityIfConfirmed, "confirmation_condition": theory.ConfirmationCondition, "refutation_condition": theory.RefutationCondition, "inconclusive_condition": theory.InconclusiveCondition, "safe_evidence_strategy": theory.SafeEvidenceStrategy, "outcome_reason": theory.OutcomeReason}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("QA theory %s is required", name)
		}
	}
	if len(theory.ExpectationRefs) == 0 || len(theory.AttemptHistory) == 0 {
		return fmt.Errorf("QA theory expectation references and attempt history are required")
	}
	if !validFingerprint(theory.ImplementationFingerprint) {
		return fmt.Errorf("invalid QA theory implementation fingerprint")
	}
	if !containsQATheoryOutcome(theory.Outcome) {
		return fmt.Errorf("unsupported QA theory outcome %q", theory.Outcome)
	}
	return nil
}

func ValidateQAMap(m QAMap) error {
	if m.SchemaVersion != QASchemaVersion || !validQAID(m.ID) || !validQAID(m.SemanticAttemptID) || !safeQAName(m.Project) || !safeQAName(m.Sprint) {
		return fmt.Errorf("invalid QA map schema or identity")
	}
	for name, value := range map[string]string{"governed input": m.GovernedInputFingerprint, "implementation": m.ImplementationFingerprint, "review": m.ReviewFingerprint, "policy": m.PolicyFingerprint, "check catalog": m.CheckCatalogFingerprint, "target": m.Target.Fingerprint} {
		if !validFingerprint(value) {
			return fmt.Errorf("invalid %s fingerprint", name)
		}
	}
	if err := validateQABudgets(m.Budgets); err != nil {
		return err
	}
	owners := map[string]int{}
	ownerIDs := map[string]string{}
	for _, shard := range m.Shards {
		if err := ValidateQAShard(shard); err != nil {
			return err
		}
		if shard.Kind == QAShardPrimary {
			for _, path := range shard.ChangedPaths {
				owners[path]++
				ownerIDs[path] = shard.ID
			}
		}
	}
	for _, path := range m.Coverage.ChangedPaths {
		if owners[path] != 1 || m.Coverage.PrimaryOwners[path] != ownerIDs[path] {
			return fmt.Errorf("changed path %q must have exactly one primary owner", path)
		}
	}
	return nil
}

func ValidateQAShard(shard QAShard) error {
	if shard.SchemaVersion != QASchemaVersion || !validQAID(shard.ID) || !validQAID(shard.AttemptID) || strings.TrimSpace(shard.Title) == "" {
		return fmt.Errorf("invalid QA shard schema or identity")
	}
	switch shard.Kind {
	case QAShardPrimary:
		if len(shard.ChangedPaths) == 0 {
			return fmt.Errorf("primary QA shard requires changed paths")
		}
	case QAShardBoundary:
		if len(shard.OverlapPaths) == 0 || strings.TrimSpace(shard.BoundaryReason) == "" {
			return fmt.Errorf("boundary QA shard requires overlap paths and reason")
		}
	case QAShardFollowUp:
		if len(shard.ParentTheoryIDs) == 0 {
			return fmt.Errorf("follow-up QA shard requires parent theories")
		}
	default:
		return fmt.Errorf("unsupported QA shard kind %q", shard.Kind)
	}
	if !containsQAPhase(shard.Phase) || len(shard.BehavioralConcerns) == 0 || len(shard.ExpectationRefs) == 0 {
		return fmt.Errorf("QA shard phase, concerns, and expectations are required")
	}
	return nil
}

func ValidateQAState(state QAState) error {
	if state.SchemaVersion != QASchemaVersion && state.SchemaVersion != QAStateSchemaVersion || !safeQAName(state.Project) || !safeQAName(state.Sprint) || !containsQAPhase(state.Phase) {
		return fmt.Errorf("invalid QA state schema, scope, or phase")
	}
	if state.CurrentAttemptID != "" && !validQAID(state.CurrentAttemptID) {
		return fmt.Errorf("invalid current QA attempt ID")
	}
	if state.CompletedShards < 0 || state.TotalShards < 0 || state.CompletedShards > state.TotalShards {
		return fmt.Errorf("invalid QA shard counts")
	}
	if state.EvidenceCount < 0 || state.RejectedCount < 0 || state.IssueCount < 0 || state.RegressionCandidates < 0 {
		return fmt.Errorf("invalid QA evidence counts")
	}
	if strings.TrimSpace(state.NextAction) == "" || state.UpdatedAt.IsZero() {
		return fmt.Errorf("QA next action and update time are required")
	}
	return nil
}

func ValidateQASynthesis(synthesis QASynthesis, budgets QABudgets) error {
	if synthesis.SchemaVersion != QASchemaVersion || !validQAID(synthesis.ID) || !validQAID(synthesis.AttemptID) || !validQAID(synthesis.MapID) {
		return fmt.Errorf("invalid QA synthesis schema or identity")
	}
	theoryIDs := make(map[string]struct{}, len(synthesis.TheoryIDs))
	for _, theoryID := range synthesis.TheoryIDs {
		if !validQAID(theoryID) {
			return fmt.Errorf("invalid QA synthesis theory reference")
		}
		theoryIDs[theoryID] = struct{}{}
	}
	if len(synthesis.FollowUpShards) > budgets.FollowUpShards {
		return fmt.Errorf("QA synthesis exceeds follow-up shard budget")
	}
	if len(synthesis.Challenges) > budgets.FollowUpShards {
		return fmt.Errorf("QA synthesis exceeds challenger record budget")
	}
	for _, challenge := range synthesis.Challenges {
		if err := ValidateQAChallenge(challenge, budgets); err != nil || challenge.MapID != synthesis.MapID {
			return fmt.Errorf("invalid QA synthesis challenge")
		}
		for _, theoryID := range challenge.TheoryIDs {
			if _, ok := theoryIDs[theoryID]; !ok {
				return fmt.Errorf("QA synthesis challenge references an unknown theory")
			}
		}
	}
	for _, shard := range synthesis.FollowUpShards {
		if err := ValidateQAShard(shard); err != nil || shard.Kind != QAShardFollowUp {
			return fmt.Errorf("invalid QA synthesis follow-up shard")
		}
	}
	if strings.TrimSpace(synthesis.NextAction) == "" {
		return fmt.Errorf("QA synthesis next action is required")
	}
	return nil
}

func ValidateQAChallenge(challenge QAChallenge, budgets QABudgets) error {
	if challenge.SchemaVersion != QASchemaVersion || !validQAID(challenge.ID) || !validQAID(challenge.MapID) {
		return fmt.Errorf("invalid QA challenge schema or identity")
	}
	if len(challenge.TheoryIDs) == 0 || len(challenge.TheoryIDs) > budgets.TheoriesPerShard {
		return fmt.Errorf("QA challenge theory references exceed bounds")
	}
	for _, theoryID := range challenge.TheoryIDs {
		if !validQAID(theoryID) {
			return fmt.Errorf("invalid QA challenge theory reference")
		}
	}
	if strings.TrimSpace(challenge.Claim) == "" || strings.TrimSpace(challenge.Basis) == "" || strings.TrimSpace(challenge.SafeEvidenceStrategy) == "" {
		return fmt.Errorf("QA challenge claim, basis, and safe evidence strategy are required")
	}
	return nil
}

func containsQAPhase(value QAPhaseStatus) bool {
	for _, allowed := range QAPhaseStatuses() {
		if value == allowed {
			return true
		}
	}
	return false
}

func containsQATheoryOutcome(value QATheoryOutcome) bool {
	for _, allowed := range QATheoryOutcomes() {
		if value == allowed {
			return true
		}
	}
	return false
}

func normalizeQAStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.ReplaceAll(strings.TrimSpace(value), "\r\n", "\n")
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

type QAErrorCategory string

const (
	QAErrorUnknownSchema      QAErrorCategory = "unknown_schema"
	QAErrorInvalidState       QAErrorCategory = "invalid_state"
	QAErrorStaleInput         QAErrorCategory = "stale_input"
	QAErrorPermissionDenied   QAErrorCategory = "permission_denied"
	QAErrorBudgetExhausted    QAErrorCategory = "budget_exhausted"
	QAErrorConflict           QAErrorCategory = "conflict"
	QAErrorPersistenceFailure QAErrorCategory = "persistence_failure"
	QAErrorRuntimeUnavailable QAErrorCategory = "runtime_unavailable"
	QAErrorAdmissionBlocked   QAErrorCategory = "admission_blocked"
	QAErrorAssertionFailure   QAErrorCategory = "assertion_failure"
	QAErrorCleanupUncertain   QAErrorCategory = "cleanup_uncertain"
	QAErrorMalformedEvidence  QAErrorCategory = "malformed_evidence"
)

type QAError struct {
	Category  QAErrorCategory `json:"category"`
	Operation string          `json:"operation"`
	Detail    string          `json:"detail"`
	Recovery  string          `json:"recovery"`
	Cause     error           `json:"-"`
}

func (e *QAError) Error() string {
	if e == nil {
		return ""
	}
	if e.Detail != "" {
		return fmt.Sprintf("qa %s: %s", e.Operation, e.Detail)
	}
	return fmt.Sprintf("qa %s: %s", e.Operation, e.Category)
}

func (e *QAError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func NewQAError(category QAErrorCategory, operation, detail string, cause error) *QAError {
	return &QAError{Category: category, Operation: operation, Detail: detail, Recovery: qaRecovery(category), Cause: cause}
}

func AsQAError(err error) (*QAError, bool) {
	var target *QAError
	ok := errors.As(err, &target)
	return target, ok
}

func qaRecovery(category QAErrorCategory) string {
	switch category {
	case QAErrorUnknownSchema:
		return "Use a compatible UltraPlan binary or an explicit supported migration."
	case QAErrorInvalidState:
		return "Inspect the detailed state and run explicit QA recovery."
	case QAErrorStaleInput:
		return "Rebuild the QA map from current governed inputs."
	case QAErrorPermissionDenied:
		return "Use a runtime that enforces the required read-only permission policy."
	case QAErrorBudgetExhausted:
		return "Reduce the governed scope or lower work per attempt; do not raise product maxima."
	case QAErrorConflict:
		return "Wait for the current owner or cancel it through run control."
	case QAErrorPersistenceFailure:
		return "Restore reliable workspace persistence, then run explicit QA recovery."
	case QAErrorRuntimeUnavailable:
		return "Restore the configured runtime and resume the current semantic attempt."
	case QAErrorAdmissionBlocked:
		return "Restore the current review, containing smoke, mapping, and isolation prerequisites before retrying."
	case QAErrorAssertionFailure:
		return "Inspect the admitted evidence and adjudicated issue before any governed repair."
	case QAErrorCleanupUncertain:
		return "Inspect and remove the isolated workspace, then start a new QA attempt."
	case QAErrorMalformedEvidence:
		return "Discard the invalid attempt and rerun the evidence plan from current inputs."
	default:
		return "Inspect the QA status and next action."
	}
}

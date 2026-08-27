package app

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/project"
	"github.com/Antonio7098/ultraplan-go/internal/sprint"
)

type SprintSummary struct {
	Project           string
	Slug              string
	Status            string
	FlowStatePath     string
	ExecutePath       string
	RunStatePath      string
	ReviewPath        string
	SmokePath         string
	Stages            []StageSummary
	Execute           ExecuteSummary
	Review            ReviewSummary
	Smoke             SmokeSummary
	Merge             MergeSummary
	QA                QAResult
	Repair            RepairStatusResult
	Findings          []DisplayFinding
	Artifacts         []DisplayArtifact
	RefreshMayWrite   bool
	RefreshActionNote string
	Assessment        string
	NextAction        string
}

type ReviewSummary struct {
	Available                bool
	Status, Verdict          string
	Error                    string
	Stale                    bool
	Completed, Total         int
	Pending, Running, Failed int
	Artifact, Digest         string
	FreshnessReasons         []string
	Reviewers                []ReviewItemSummary
	StartedAt                *time.Time
}

type ReviewItemSummary struct {
	ID, Name, Kind, Path, Status, Summary string
}

type SmokeSummary struct {
	Available                    bool
	Status, Verdict, RunID       string
	Error                        string
	Stale, Reconciliation        bool
	Artifact, Digest, NextAction string
	FreshnessReasons             []string
	Issues                       []sprint.SmokeIssue
	Override                     *sprint.DiagnosticOverride
	CoverageMapping              *sprint.SmokeCoverageMapping
}

type MergeSummary struct {
	Available bool
	Status    string
	Commit    string
	Error     string
}

// QAUseCases is the adapter-independent QA boundary. Its DTOs contain bounded
// product facts and never expose verification persistence records directly.
type QAUseCases interface {
	QAQueries
	RunQA(context.Context, QARequest, func(OperationEvent)) (QAResult, error)
	ResumeQA(context.Context, QARequest, func(OperationEvent)) (QAResult, error)
	CancelQA(context.Context, QARequest) (QACancelResult, error)
	RecoverQA(context.Context, QARequest) (QAResult, error)
}

// RepairUseCases is additive to the existing QA boundary. Repair records are
// projected into bounded product facts; proposal patches and raw runtime data
// never cross this interface.
type RepairUseCases interface {
	RepairStatus(context.Context, RepairRequest) (RepairStatusResult, error)
}

type RepairRequest struct {
	Project     string
	Sprint      string
	RepairRunID string
}

type RepairStatusResult struct {
	SchemaVersion      int                        `json:"schema_version"`
	Project            string                     `json:"project"`
	Sprint             string                     `json:"sprint"`
	Phase              string                     `json:"phase"`
	Fresh              bool                       `json:"fresh"`
	FreshnessReasons   []string                   `json:"freshness_reasons,omitempty"`
	RepairRunID        string                     `json:"repair_run_id,omitempty"`
	QAAttemptID        string                     `json:"qa_attempt_id,omitempty"`
	OperationRunID     string                     `json:"operation_run_id,omitempty"`
	OperationalAttempt string                     `json:"operational_attempt_id,omitempty"`
	FencingGeneration  uint64                     `json:"fencing_generation,omitempty"`
	RunLifecycle       string                     `json:"run_lifecycle,omitempty"`
	Mode               string                     `json:"mode,omitempty"`
	Packet             *RepairPacketSummary       `json:"packet,omitempty"`
	Confirmation       *RepairConfirmSummary      `json:"confirmation,omitempty"`
	CurrentCycle       int                        `json:"current_cycle,omitempty"`
	EarliestCycle      int                        `json:"earliest_cycle,omitempty"`
	Outcome            string                     `json:"outcome,omitempty"`
	StopReason         string                     `json:"stop_reason,omitempty"`
	CleanupComplete    bool                       `json:"cleanup_complete"`
	ProductionApplied  bool                       `json:"production_applied"`
	CompleteLadder     bool                       `json:"complete_ladder"`
	UnresolvedIssues   []string                   `json:"unresolved_issues,omitempty"`
	Deadline           time.Time                  `json:"deadline,omitempty"`
	UpdatedAt          time.Time                  `json:"updated_at,omitempty"`
	NextAction         string                     `json:"next_action"`
	Reason             string                     `json:"reason,omitempty"`
	Blocker            *QABlockerSummary          `json:"blocker,omitempty"`
	EffectiveSources   []sprint.QAEffectiveSource `json:"effective_sources,omitempty"`
}

type RepairPacketSummary struct {
	Digest             string                  `json:"digest"`
	IssueID            string                  `json:"issue_id"`
	IssueTitle         string                  `json:"issue_title"`
	Target             QATargetIdentitySummary `json:"target"`
	AllowedPaths       []string                `json:"allowed_paths"`
	ForbiddenPaths     []string                `json:"forbidden_paths"`
	AcceptanceCriteria []string                `json:"acceptance_criteria"`
	CheckCount         int                     `json:"check_count"`
	Budgets            RepairBudgetSummary     `json:"budgets"`
}

type RepairBudgetSummary struct {
	MaxCycles         int                        `json:"max_cycles"`
	MaxMutationCycles int                        `json:"max_mutation_cycles"`
	MaxFiles          int                        `json:"max_files"`
	MaxBytes          int64                      `json:"max_bytes"`
	WallTime          string                     `json:"wall_time"`
	CommandTimeout    string                     `json:"command_timeout"`
	Sources           []sprint.QAEffectiveSource `json:"sources"`
}

type RepairConfirmSummary struct {
	Digest            string    `json:"digest"`
	PacketDigest      string    `json:"packet_digest"`
	Confirmer         string    `json:"confirmer"`
	OperationRunID    string    `json:"operation_run_id"`
	FencingGeneration uint64    `json:"fencing_generation"`
	ConfirmedAt       time.Time `json:"confirmed_at"`
}

type QAQueries interface {
	QAMap(context.Context, QARequest) (QAResult, error)
	QAStatus(context.Context, QARequest) (QAResult, error)
	QAShard(context.Context, QARequest) (QAShardResult, error)
	QATheory(context.Context, QARequest) (QATheoryResult, error)
	QASynthesis(context.Context, QARequest) (QASynthesisResult, error)
}

// QAEvidenceQueries is additive so older local adapters that implement the
// Sprint 36 QA query set remain source-compatible.
type QAEvidenceQueries interface {
	QAEvidence(context.Context, QARequest) (QAEvidenceResult, error)
	QAAdjudication(context.Context, QARequest) (QAAdjudicationResult, error)
	QAIssues(context.Context, QARequest) (QAIssuePage, error)
	QAIssue(context.Context, QARequest) (QAIssueSummary, error)
	QAAssessment(context.Context, QARequest) (QAAssessmentResult, error)
	QASmokeSuite(context.Context, QARequest) (QASmokeSuiteResult, error)
}

type QARequest struct {
	Project  string
	Sprint   string
	Shard    string
	Theory   string
	RunID    string
	Suite    string
	Evidence string
	Issue    string
	Cursor   string
	Limit    int
}

type QAResult struct {
	SchemaVersion                int                        `json:"schema_version"`
	Project                      string                     `json:"project"`
	Sprint                       string                     `json:"sprint"`
	Phase                        string                     `json:"phase"`
	Fresh                        bool                       `json:"fresh"`
	FreshnessReasons             []string                   `json:"freshness_reasons,omitempty"`
	AttemptID                    string                     `json:"attempt_id,omitempty"`
	RunID                        string                     `json:"run_id,omitempty"`
	OperationalAttemptID         string                     `json:"operational_attempt_id,omitempty"`
	FencingGeneration            uint64                     `json:"fencing_generation,omitempty"`
	RunLifecycle                 string                     `json:"run_lifecycle,omitempty"`
	TerminalResult               string                     `json:"terminal_result,omitempty"`
	GovernedInputFingerprint     string                     `json:"governed_input_fingerprint,omitempty"`
	ImplementationFingerprint    string                     `json:"implementation_fingerprint,omitempty"`
	ReviewFingerprint            string                     `json:"review_fingerprint,omitempty"`
	ConformanceReviewStatus      string                     `json:"conformance_review_status,omitempty"`
	ConformanceReviewVerdict     string                     `json:"conformance_review_verdict,omitempty"`
	ConformanceReviewFresh       bool                       `json:"conformance_review_fresh"`
	ConformanceReviewUnavailable bool                       `json:"conformance_review_unavailable,omitempty"`
	ConformanceReviewDiagnostic  string                     `json:"conformance_review_diagnostic,omitempty"`
	MapFingerprint               string                     `json:"map_fingerprint,omitempty"`
	PolicyFingerprint            string                     `json:"policy_fingerprint,omitempty"`
	CheckCatalogFingerprint      string                     `json:"check_catalog_fingerprint,omitempty"`
	UpdatedAt                    time.Time                  `json:"updated_at,omitempty"`
	MapRecord                    *QAArtifactRefSummary      `json:"map_record,omitempty"`
	SynthesisRecord              *QAArtifactRefSummary      `json:"synthesis_record,omitempty"`
	EffectiveSources             []QAEffectiveSourceSummary `json:"effective_sources,omitempty"`
	Target                       QATargetIdentitySummary    `json:"target"`
	Coverage                     QACoverageSummary          `json:"coverage"`
	InputRefs                    []QAArtifactRefSummary     `json:"input_refs,omitempty"`
	Limits                       QALimitsSummary            `json:"limits"`
	ChangedPaths                 int                        `json:"changed_paths"`
	CoveredPaths                 int                        `json:"covered_paths"`
	CompletedShards              int                        `json:"completed_shards"`
	TotalShards                  int                        `json:"total_shards"`
	OutcomeTotals                map[string]int             `json:"outcome_totals,omitempty"`
	Shards                       []QAShardSummary           `json:"shards"`
	Blocker                      *QABlockerSummary          `json:"blocker,omitempty"`
	Cancellation                 QACancellationSummary      `json:"cancellation"`
	NextAction                   string                     `json:"next_action"`
	Suite                        string                     `json:"suite,omitempty"`
	Assessment                   string                     `json:"assessment,omitempty"`
	EvidenceCount                int                        `json:"evidence_count,omitempty"`
	RejectedEvidenceCount        int                        `json:"rejected_evidence_count,omitempty"`
	IssueCount                   int                        `json:"issue_count,omitempty"`
	RegressionCandidateCount     int                        `json:"regression_candidate_count,omitempty"`
	CanonicalReport              *QAArtifactRefSummary      `json:"canonical_report,omitempty"`
	CurrentFailure               *QABlockerSummary          `json:"current_failure,omitempty"`
}

type QAArtifactRefSummary struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

type QAEffectiveSourceSummary struct {
	Field  string `json:"field"`
	Source string `json:"source"`
}

type QATargetIdentitySummary struct {
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

type QACoverageSummary struct {
	ChangedPaths     []string            `json:"changed_paths,omitempty"`
	PrimaryOwners    map[string]string   `json:"primary_owners,omitempty"`
	BoundaryOverlaps map[string][]string `json:"boundary_overlaps,omitempty"`
	BlockedPaths     []string            `json:"blocked_paths,omitempty"`
}

type QALimitsSummary struct {
	ChangedPaths               int    `json:"changed_paths"`
	TotalShards                int    `json:"total_shards"`
	PendingEntries             int    `json:"pending_entries"`
	ChangedPathsPerShard       int    `json:"changed_paths_per_shard"`
	ContextPathsPerShard       int    `json:"context_paths_per_shard"`
	ContextExpansions          int    `json:"context_expansions"`
	PathsPerExpansion          int    `json:"paths_per_expansion"`
	BehavioralConcernsPerShard int    `json:"behavioral_concerns_per_shard"`
	TheoriesPerShard           int    `json:"theories_per_shard"`
	IterationsPerAttempt       int    `json:"iterations_per_attempt"`
	CommandsPerAttempt         int    `json:"commands_per_attempt"`
	OutputRepairAttempts       int    `json:"output_repair_attempts"`
	ConcurrentInvestigators    int    `json:"concurrent_investigators"`
	CommandTimeout             string `json:"command_timeout"`
	ShardTimeout               string `json:"shard_timeout"`
	RunTimeout                 string `json:"run_timeout"`
	CleanupTimeout             string `json:"cleanup_timeout"`
	CommandOutputBytes         int    `json:"command_output_bytes"`
	ShardOutputBytes           int    `json:"shard_output_bytes"`
	PromptBytes                int    `json:"prompt_bytes"`
	FollowUpShards             int    `json:"follow_up_shards"`
	TreeFiles                  int    `json:"tree_files,omitempty"`
	TreeBytes                  int64  `json:"tree_bytes,omitempty"`
	FileBytes                  int64  `json:"file_bytes,omitempty"`
	GeneratedChecks            int    `json:"generated_checks,omitempty"`
	GeneratedPatchBytes        int    `json:"generated_patch_bytes,omitempty"`
	EvidenceRecords            int    `json:"evidence_records,omitempty"`
	Issues                     int    `json:"issues,omitempty"`
	AnalyzerCalls              int    `json:"analyzer_calls,omitempty"`
	EvaluatorCalls             int    `json:"evaluator_calls,omitempty"`
}

type QAShardSummary struct {
	ID                 string                         `json:"id"`
	AttemptID          string                         `json:"attempt_id,omitempty"`
	Kind               string                         `json:"kind"`
	Title              string                         `json:"title"`
	Phase              string                         `json:"phase"`
	ChangedPaths       []string                       `json:"changed_paths,omitempty"`
	ContextPaths       []string                       `json:"context_paths,omitempty"`
	OverlapPaths       []string                       `json:"overlap_paths,omitempty"`
	BoundaryReason     string                         `json:"boundary_reason,omitempty"`
	BehavioralConcerns []string                       `json:"behavioral_concerns,omitempty"`
	ExpectationRefs    []string                       `json:"expectation_refs,omitempty"`
	RiskTags           []string                       `json:"risk_tags,omitempty"`
	ApprovedChecks     []QAApprovedCheckSummary       `json:"approved_checks,omitempty"`
	ParentTheoryIDs    []string                       `json:"parent_theory_ids,omitempty"`
	Attempts           []QAInvestigatorAttemptSummary `json:"attempts,omitempty"`
	TheoryCount        int                            `json:"theory_count"`
	Theories           []QATheorySummary              `json:"theories,omitempty"`
	Blocker            *QABlockerSummary              `json:"blocker,omitempty"`
}

type QAApprovedCheckSummary struct {
	ID          string `json:"id"`
	Fingerprint string `json:"fingerprint"`
}

type QAEvidenceSummary struct {
	Kind         string   `json:"kind"`
	Summary      string   `json:"summary"`
	Paths        []string `json:"paths,omitempty"`
	CheckID      string   `json:"check_id,omitempty"`
	OutputDigest string   `json:"output_digest,omitempty"`
}

type QAContextRequestSummary struct {
	ID           string   `json:"id"`
	Paths        []string `json:"paths"`
	Reason       string   `json:"reason"`
	Approved     bool     `json:"approved"`
	DeniedReason string   `json:"denied_reason,omitempty"`
}

type QACommandSummary struct {
	CheckID               string `json:"check_id"`
	DescriptorFingerprint string `json:"descriptor_fingerprint"`
	ExitCode              int    `json:"exit_code"`
	Duration              string `json:"duration"`
	StdoutDigest          string `json:"stdout_digest,omitempty"`
	StderrDigest          string `json:"stderr_digest,omitempty"`
	OutputBytes           int    `json:"output_bytes"`
	Truncated             bool   `json:"truncated"`
}

type QAInvestigatorAttemptSummary struct {
	ID                   string                     `json:"id"`
	Number               int                        `json:"number"`
	StartedAt            time.Time                  `json:"started_at"`
	CompletedAt          *time.Time                 `json:"completed_at,omitempty"`
	Duration             string                     `json:"duration,omitempty"`
	ImplementationBefore string                     `json:"implementation_before"`
	ImplementationAfter  string                     `json:"implementation_after,omitempty"`
	ContextRequests      []QAContextRequestSummary  `json:"context_requests,omitempty"`
	Commands             []QACommandSummary         `json:"commands,omitempty"`
	Evidence             []QAEvidenceSummary        `json:"evidence,omitempty"`
	Usage                sprint.QAUsageSummary      `json:"usage"`
	EstimatedCost        *sprint.QACostSummary      `json:"estimated_cost,omitempty"`
	FailureKind          string                     `json:"failure_kind,omitempty"`
	Retryable            bool                       `json:"retryable,omitempty"`
	StopReason           string                     `json:"stop_reason,omitempty"`
	OutputDiagnostic     *sprint.QAOutputDiagnostic `json:"output_diagnostic,omitempty"`
	Repair               *sprint.QARepairDiagnostic `json:"repair,omitempty"`
}

type QABlockerSummary struct {
	Category   string `json:"category"`
	Scope      string `json:"scope"`
	Summary    string `json:"summary"`
	NextAction string `json:"next_action"`
}

type QACancellationSummary struct {
	Requested bool       `json:"requested"`
	Scope     string     `json:"scope,omitempty"`
	ShardID   string     `json:"shard_id,omitempty"`
	Reason    string     `json:"reason,omitempty"`
	At        *time.Time `json:"at,omitempty"`
}

type QAShardResult struct {
	QAResult
	Shard    QAShardSummary    `json:"shard"`
	Theories []QATheorySummary `json:"theories"`
}

type QATheorySummary struct {
	ID                        string                         `json:"id"`
	ShardID                   string                         `json:"shard_id"`
	Claim                     string                         `json:"claim"`
	Basis                     string                         `json:"basis"`
	VerificationSurface       string                         `json:"verification_surface"`
	ExpectationRefs           []string                       `json:"expectation_refs"`
	SeverityIfConfirmed       string                         `json:"severity_if_confirmed"`
	ConfirmationCondition     string                         `json:"confirmation_condition"`
	RefutationCondition       string                         `json:"refutation_condition"`
	InconclusiveCondition     string                         `json:"inconclusive_condition"`
	SafeEvidenceStrategy      string                         `json:"safe_evidence_strategy"`
	ImplementationFingerprint string                         `json:"implementation_fingerprint"`
	AttemptHistory            []QAInvestigatorAttemptSummary `json:"attempt_history,omitempty"`
	Evidence                  []QAEvidenceSummary            `json:"evidence,omitempty"`
	Outcome                   string                         `json:"outcome"`
	OutcomeReason             string                         `json:"outcome_reason"`
}

type QATheoryResult struct {
	QAResult
	Theory QATheorySummary `json:"theory"`
}

type QASynthesisResult struct {
	QAResult
	ID             string               `json:"id,omitempty"`
	MapID          string               `json:"map_id,omitempty"`
	NextAction     string               `json:"synthesis_next_action,omitempty"`
	TheoryIDs      []string             `json:"theory_ids"`
	Challenges     []QAChallengeSummary `json:"challenges,omitempty"`
	OutcomeTotals  map[string]int       `json:"synthesis_outcome_totals,omitempty"`
	Deduplicated   map[string][]string  `json:"deduplicated,omitempty"`
	Contradictions [][]string           `json:"contradictions,omitempty"`
	Interactions   []string             `json:"interactions,omitempty"`
	Blockers       []QABlockerSummary   `json:"blockers,omitempty"`
	FollowUpShards []QAShardSummary     `json:"follow_up_shards"`
}

type QAChallengeSummary struct {
	ID                   string   `json:"id"`
	TheoryIDs            []string `json:"theory_ids"`
	Claim                string   `json:"claim"`
	Basis                string   `json:"basis"`
	SafeEvidenceStrategy string   `json:"safe_evidence_strategy"`
	EvidenceRefs         []string `json:"evidence_refs,omitempty"`
}

type QACancelResult struct {
	Run       RunSnapshot `json:"run"`
	Requested bool        `json:"requested"`
	QA        QAResult    `json:"qa"`
}

type QAEvidenceResult struct {
	ID              string `json:"id"`
	PlanID          string `json:"plan_id"`
	AttemptID       string `json:"attempt_id"`
	ShardID         string `json:"shard_id"`
	Outcome         string `json:"outcome"`
	ReasonCode      string `json:"reason_code"`
	Repeatable      bool   `json:"repeatable"`
	Contained       bool   `json:"contained"`
	CleanupComplete bool   `json:"cleanup_complete"`
	CommandCount    int    `json:"command_count"`
	AnalyzerCount   int    `json:"analyzer_count"`
}

type QAAdjudicationResult struct {
	ID             string              `json:"id"`
	AttemptID      string              `json:"attempt_id"`
	AcceptedCount  int                 `json:"accepted_count"`
	Rejected       []QARejectedSummary `json:"rejected,omitempty"`
	IssueCount     int                 `json:"issue_count"`
	EvaluatorCount int                 `json:"evaluator_count"`
}

type QARejectedSummary struct {
	EvidenceID string `json:"evidence_id"`
	Code       string `json:"code"`
	Detail     string `json:"detail"`
}

type QAIssueSummary struct {
	ID                  string   `json:"id"`
	Title               string   `json:"title"`
	IssueClass          string   `json:"issue_class"`
	Severity            string   `json:"severity"`
	Location            string   `json:"location"`
	EvidenceIDs         []string `json:"evidence_ids"`
	PromotionReason     string   `json:"promotion_reason"`
	RepairEligible      bool     `json:"repair_eligible"`
	RegressionCandidate bool     `json:"regression_candidate"`
}

type QAIssuePage struct {
	Items        []QAIssueSummary `json:"items"`
	NextCursor   string           `json:"next_cursor,omitempty"`
	OmittedCount int              `json:"omitted_count"`
}

type QAAssessmentResult struct {
	Assessment    string             `json:"assessment"`
	ReviewVerdict string             `json:"review_verdict"`
	SmokeVerdict  string             `json:"smoke_verdict,omitempty"`
	EvidenceTotal int                `json:"evidence_total"`
	RejectedTotal int                `json:"rejected_total"`
	IssueTotal    int                `json:"issue_total"`
	Blockers      []QABlockerSummary `json:"blockers,omitempty"`
	NextAction    string             `json:"next_action"`
}

type QASmokeSuiteResult struct {
	ExecutionStatus string `json:"execution_status"`
	Verdict         string `json:"verdict,omitempty"`
	Fresh           bool   `json:"fresh"`
	RunID           string `json:"run_id,omitempty"`
	NextAction      string `json:"next_action"`
}

type StageSummary struct {
	Name              string
	Status            string
	Path              string
	Error             string
	ArtifactAvailable bool
	ArtifactValid     bool
	LatestOutcome     string
	NextAction        string
	PromptContract    *sprint.StageInputContract
	// ConfiguredModel is the stage-specific ultraplan.yml planning model that
	// applies when the operator leaves the model input empty. It stays empty
	// when only the workspace default governs the stage.
	ConfiguredModel string
	// RunModel is the model recorded by the most recent completed run of this
	// stage in sprint runtime metrics.
	RunModel string
}

type ExecuteSummary struct {
	Available bool
	Total     int
	Pending   int
	Running   int
	Complete  int
	Deferred  int
	Failed    int
	Cancelled int
	Message   string
}

func (u dashboardUseCases) SprintSummaries(ctx context.Context) ([]SprintSummary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	projects, err := project.DiscoverProjects(u.root)
	if err != nil {
		return nil, mapProjectError("project.list", err)
	}
	service := u.sprintService()
	var out []SprintSummary
	for _, p := range projects {
		sprints, err := sprint.DiscoverSprints(u.root, p)
		if err != nil {
			return nil, mapSprintError("sprint.list", err)
		}
		for _, sp := range sprints {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			status, err := service.Status(p.Name, sp.Slug)
			if err != nil {
				qaGuidance := "Inspect sprint status and the detailed QA state before running QA recovery."
				out = append(out, SprintSummary{
					Project:           p.Name,
					Slug:              sp.Slug,
					Status:            "status unavailable",
					RefreshMayWrite:   !u.readOnly,
					RefreshActionNote: "refresh recomputes deterministic flow-state.json status when existing state is valid",
					Findings:          []DisplayFinding{{Severity: "error", Section: "sprint.status", Problem: displaySafe(err.Error()), Suggestion: "Inspect or regenerate sprint flow-state.json outside the read-only TUI."}},
					Artifacts: []DisplayArtifact{
						{Label: "requirements", Path: sprint.ArtifactRelPath(sp, sprint.StageRequirements), Kind: "markdown"},
						{Label: "code-context", Path: sprint.ArtifactRelPath(sp, sprint.StageCodeContext), Kind: "markdown"},
						{Label: "sprint-index", Path: sprint.ArtifactRelPath(sp, sprint.StageSprintIndex), Kind: "markdown"},
						{Label: "technical-handbook", Path: sprint.ArtifactRelPath(sp, sprint.StageTechnicalHandbook), Kind: "markdown"},
						{Label: "reasoning", Path: sprint.ArtifactRelPath(sp, sprint.StageReasoning), Kind: "markdown"},
						{Label: "plan", Path: sprint.ArtifactRelPath(sp, sprint.StagePlan), Kind: "markdown"},
						{Label: "execute", Path: sprint.ArtifactRelPath(sp, sprint.StageExecute), Kind: "markdown"},
						{Label: "review", Path: sprint.ArtifactRelPath(sp, sprint.StageReview), Kind: "markdown"},
						{Label: "smoke", Path: sprint.ArtifactRelPath(sp, sprint.StageSmoke), Kind: "markdown"},
						{Label: "merge", Path: sprint.ArtifactRelPath(sp, sprint.StageMerge), Kind: "markdown"},
						{Label: "flow-state", Path: sprint.FlowStateRelPath(sp), Kind: "json"},
						{Label: "run-state", Path: sprint.ExecuteRunStateRelPath(sp), Kind: "json"},
					},
					Execute: ExecuteSummary{Message: "execute status unavailable because sprint status failed"},
					QA:      QAResult{SchemaVersion: 1, Project: p.Name, Sprint: sp.Slug, Phase: string(sprint.QAPhaseInvalid), FreshnessReasons: []string{"sprint.status_unavailable"}, Blocker: &QABlockerSummary{Category: "qa.invalid_state", Scope: "state", Summary: "QA status is unavailable because sprint status could not be loaded.", NextAction: qaGuidance}, NextAction: qaGuidance},
				})
				continue
			}
			qaSummary := QAResult{SchemaVersion: 1, Project: p.Name, Sprint: sp.Slug, Phase: string(sprint.QAPhaseMissing), NextAction: "Run a QA dry run to inspect the current map."}
			if qaSnapshot, qaErr := service.QAStatus(p.Name, sp.Slug); qaErr == nil {
				qaSummary = qaSnapshotProjection(qaSnapshot)
			} else {
				mapped := mapQAUseCaseError(qaErr)
				code := "qa.invalid_state"
				message := "The QA state cannot be read."
				guidance := "Inspect the detailed QA state and run explicit QA recovery."
				if typed, ok := AsQAUseCaseError(mapped); ok {
					code, message, guidance = typed.Code, typed.Message, typed.Guidance
				}
				qaSummary.Phase = string(sprint.QAPhaseInvalid)
				qaSummary.FreshnessReasons = []string{code}
				qaSummary.Blocker = &QABlockerSummary{Category: code, Scope: "state", Summary: message, NextAction: guidance}
				qaSummary.NextAction = guidance
			}
			qaSummary.ConformanceReviewStatus = string(status.Verification.Review.ExecutionStatus)
			qaSummary.ConformanceReviewVerdict = string(status.Verification.Review.Verdict)
			qaSummary.ConformanceReviewFresh = status.Verification.Review.Fresh
			repairSummary := RepairStatusResult{SchemaVersion: 1, Project: p.Name, Sprint: sp.Slug, Phase: string(sprint.RepairPhaseStale), NextAction: "Prepare one current repair-eligible QA issue."}
			if repairSnapshot, repairErr := service.RepairStatus(p.Name, sp.Slug); repairErr == nil {
				repairSummary = repairSnapshotProjection(repairSnapshot)
			}
			review := summarizeReview(status.Review)
			manifest, _, manifestErr := service.PrepareReview(p.Name, sp.Slug, sprint.ReviewRequest{})
			if manifestErr == nil {
				review.Reviewers = summarizeReviewers(status.Review, manifest.Coverage)
			} else {
				review.Reviewers = summarizeReviewers(status.Review, nil)
			}
			for _, reviewer := range review.Reviewers {
				switch reviewer.Status {
				case "pending":
					review.Pending++
				case "running":
					review.Running++
				case "failed", "cancelled", "timed_out", "blocked":
					review.Failed++
				}
			}
			summary := SprintSummary{
				Project:           p.Name,
				Slug:              sp.Slug,
				Status:            "available",
				FlowStatePath:     status.FlowStatePath,
				ExecutePath:       status.ExecutePath,
				RunStatePath:      status.RunStatePath,
				ReviewPath:        status.ReviewPath,
				SmokePath:         status.SmokePath,
				RefreshMayWrite:   !u.readOnly,
				RefreshActionNote: "refresh derives verification freshness and assessment from current evidence without caching them as authoritative state",
				Execute:           summarizeExecute(status.ExecuteState),
				Review:            review,
				Smoke:             summarizeSmoke(status.Smoke),
				Merge:             summarizeMerge(status.Merge),
				QA:                qaSummary,
				Repair:            repairSummary,
				Assessment:        string(status.Verification.Assessment),
				NextAction:        status.Verification.NextAction,
			}
			if status.HistoricalExecutionStatus != "" {
				summary.Status = status.HistoricalExecutionStatus
				summary.Execute = summarizeHistoricalExecute(status.HistoricalExecutionStatus)
			} else if executeTerminalComplete(summary.Execute) {
				summary.Status = "complete"
			}
			summary.Review.Artifact, summary.Review.Digest, summary.Review.FreshnessReasons = status.Verification.Review.Artifact, status.Verification.Review.ArtifactDigest, append([]string(nil), status.Verification.Review.FreshnessReasons...)
			summary.Review.Stale = !status.Verification.Review.Fresh
			summary.Smoke.Artifact, summary.Smoke.Digest, summary.Smoke.NextAction = status.Verification.Smoke.Artifact, status.Verification.Smoke.ArtifactDigest, status.Verification.Smoke.NextAction
			summary.Smoke.FreshnessReasons, summary.Smoke.Issues, summary.Smoke.Override = append([]string(nil), status.Verification.Smoke.FreshnessReasons...), append([]sprint.SmokeIssue(nil), status.Verification.Smoke.Issues...), status.Verification.Smoke.Override
			summary.Smoke.Stale = !status.Verification.Smoke.Fresh
			for _, stage := range status.Stages {
				latestOutcome := stage.LatestOutcome
				if latestOutcome == "" {
					latestOutcome = string(stage.Status)
				}
				stageSummary := StageSummary{Name: string(stage.Stage), Status: string(stage.Status), Path: stage.Path, Error: displaySafe(stage.Error), LatestOutcome: latestOutcome}
				if stage.Stage == sprint.StageCodeContext {
					if info, statErr := os.Lstat(filepath.Join(u.root, filepath.FromSlash(stage.Path))); statErr == nil && info.Mode().IsRegular() {
						stageSummary.ArtifactAvailable = true
					}
				}
				summary.Stages = append(summary.Stages, stageSummary)
			}
			summary.Artifacts = append(summary.Artifacts,
				DisplayArtifact{Label: "requirements", Path: sprint.ArtifactRelPath(sp, sprint.StageRequirements), Kind: "markdown"},
				DisplayArtifact{Label: "code-context", Path: sprint.ArtifactRelPath(sp, sprint.StageCodeContext), Kind: "markdown"},
				DisplayArtifact{Label: "sprint-index", Path: sprint.ArtifactRelPath(sp, sprint.StageSprintIndex), Kind: "markdown"},
				DisplayArtifact{Label: "technical-handbook", Path: sprint.ArtifactRelPath(sp, sprint.StageTechnicalHandbook), Kind: "markdown"},
				DisplayArtifact{Label: "reasoning", Path: sprint.ArtifactRelPath(sp, sprint.StageReasoning), Kind: "markdown"},
				DisplayArtifact{Label: "plan", Path: sprint.ArtifactRelPath(sp, sprint.StagePlan), Kind: "markdown"},
				DisplayArtifact{Label: "execute", Path: sprint.ArtifactRelPath(sp, sprint.StageExecute), Kind: "markdown"},
				DisplayArtifact{Label: "review", Path: sprint.ArtifactRelPath(sp, sprint.StageReview), Kind: "markdown"},
				DisplayArtifact{Label: "smoke", Path: sprint.ArtifactRelPath(sp, sprint.StageSmoke), Kind: "markdown"},
				DisplayArtifact{Label: "merge", Path: sprint.ArtifactRelPath(sp, sprint.StageMerge), Kind: "markdown"},
				DisplayArtifact{Label: "flow-state", Path: sprint.FlowStateRelPath(sp), Kind: "json"},
				DisplayArtifact{Label: "run-state", Path: sprint.ExecuteRunStateRelPath(sp), Kind: "json"},
			)
			for _, stage := range []sprint.PlanningStage{sprint.StageRequirements, sprint.StageCodeContext, sprint.StageSprintIndex, sprint.StageTechnicalHandbook, sprint.StageReasoning, sprint.StagePlan, sprint.StageExecute, sprint.StageReview, sprint.StageSmoke, sprint.StageMerge} {
				result, err := validateSprintStage(service, p.Name, sp.Slug, stage)
				if err != nil {
					continue
				}
				for _, finding := range result.Findings {
					summary.Findings = append(summary.Findings, sprintFinding(finding))
				}
				if stage == sprint.StageCodeContext {
					for i := range summary.Stages {
						if summary.Stages[i].Name == string(stage) {
							summary.Stages[i].ArtifactValid = result.Valid()
							switch {
							case summary.Stages[i].Status == string(sprint.StatusFailed) && summary.Stages[i].ArtifactAvailable && summary.Stages[i].ArtifactValid:
								summary.Stages[i].NextAction = "A prior valid artifact is preserved; inspect the failure and explicitly rerun code-context."
							case summary.Stages[i].Status == string(sprint.StatusFailed):
								summary.Stages[i].NextAction = "Inspect the failure and explicitly rerun code-context."
							case summary.Stages[i].Status == string(sprint.StatusComplete):
								summary.Stages[i].NextAction = "Continue to sprint-index."
							default:
								summary.Stages[i].NextAction = "Run code-context after requirements validate."
							}
						}
					}
				}
			}
			sortSprintArtifacts(summary.Artifacts)
			out = append(out, summary)
		}
	}
	return out, nil
}

func sortSprintArtifacts(items []DisplayArtifact) {
	order := map[string]int{"requirements": 0, "code-context": 1, "sprint-index": 2, "technical-handbook": 3, "reasoning": 4, "plan": 5, "execute": 6, "review": 7, "smoke": 8, "merge": 9, "flow-state": 10, "run-state": 11}
	sort.SliceStable(items, func(i, j int) bool {
		left, leftOK := order[items[i].Label]
		right, rightOK := order[items[j].Label]
		if leftOK && rightOK {
			return left < right
		}
		if leftOK != rightOK {
			return leftOK
		}
		return items[i].Path < items[j].Path
	})
}

func validateSprintStage(service sprint.Service, projectRef, sprintRef string, stage sprint.PlanningStage) (sprint.ValidationResult, error) {
	switch stage {
	case sprint.StageRequirements:
		return service.ValidateRequirements(projectRef, sprintRef)
	case sprint.StageCodeContext:
		return service.ValidateCodeContext(projectRef, sprintRef)
	case sprint.StageSprintIndex:
		return service.ValidateSprintIndex(projectRef, sprintRef)
	case sprint.StageTechnicalHandbook:
		return service.ValidateTechnicalHandbook(projectRef, sprintRef)
	case sprint.StageAreaReasoning:
		return service.ValidateAreaReasoning(projectRef, sprintRef)
	case sprint.StageReasoning:
		return service.ValidateReasoning(projectRef, sprintRef)
	case sprint.StagePlan:
		return service.ValidatePlan(projectRef, sprintRef)
	case sprint.StageExecute:
		return service.ValidateExecute(projectRef, sprintRef)
	case sprint.StageReview:
		return service.ValidateReview(projectRef, sprintRef)
	case sprint.StageSmoke:
		return service.ValidateSmoke(projectRef, sprintRef)
	case sprint.StageMerge:
		return service.ValidateMerge(projectRef, sprintRef)
	default:
		return sprint.ValidationResult{}, fmt.Errorf("unsupported validation stage %q", stage)
	}
}

func summarizeSmoke(state *sprint.SmokeStageState) SmokeSummary {
	if state == nil {
		return SmokeSummary{}
	}
	state.CoverageMapping.EnsureMatrix()
	return SmokeSummary{Available: true, Status: string(state.Status), Verdict: string(state.Verdict), RunID: state.RunID, Error: displayReasons(state.Diagnostics), Stale: state.Stale, Reconciliation: state.Reconciliation, CoverageMapping: state.CoverageMapping}
}

func summarizeMerge(state *sprint.MergeState) MergeSummary {
	if state == nil {
		return MergeSummary{}
	}
	return MergeSummary{Available: true, Status: string(state.Status), Commit: state.MergeCommit, Error: state.Diagnostic}
}

func summarizeReview(state *sprint.ReviewStageState) ReviewSummary {
	if state == nil {
		return ReviewSummary{}
	}
	reasons := make([]string, 0, len(state.Diagnostics))
	for _, diagnostic := range state.Diagnostics {
		reason := diagnostic.Message
		if diagnostic.Code != "" {
			reason = diagnostic.Code + ": " + reason
		}
		reasons = append(reasons, reason)
	}
	summary := ReviewSummary{Available: true, Status: string(state.Status), Verdict: string(state.Verdict), Error: displayReasons(reasons), Stale: state.Stale, Completed: state.Completed, Total: state.Total}
	if state.ActiveAttempt != nil {
		started := state.ActiveAttempt.StartedAt
		summary.StartedAt = &started
	}
	return summary
}

func (u dashboardUseCases) QAMap(ctx context.Context, req QARequest) (QAResult, error) {
	if err := ctx.Err(); err != nil {
		return QAResult{}, err
	}
	result, err := u.sprintService().QAMap(req.Project, req.Sprint)
	if err != nil {
		return QAResult{}, mapQAUseCaseError(err)
	}
	return u.withQAConformanceReview(req, qaMapProjection(result.Map)), nil
}

func (u dashboardUseCases) QAStatus(ctx context.Context, req QARequest) (QAResult, error) {
	if err := ctx.Err(); err != nil {
		return QAResult{}, err
	}
	snapshot, err := u.sprintService().QAStatus(req.Project, req.Sprint)
	if err != nil {
		return QAResult{}, mapQAUseCaseError(err)
	}
	return u.withQAConformanceReview(req, qaSnapshotProjection(snapshot)), nil
}

func (u dashboardUseCases) RepairStatus(ctx context.Context, req RepairRequest) (RepairStatusResult, error) {
	if err := ctx.Err(); err != nil {
		return RepairStatusResult{}, err
	}
	snapshot, err := u.sprintService().RepairStatus(req.Project, req.Sprint)
	if err != nil {
		return RepairStatusResult{}, mapQAUseCaseError(err)
	}
	if req.RepairRunID != "" && snapshot.State.RepairRunID != req.RepairRunID {
		return RepairStatusResult{}, fmt.Errorf("repair run %q is not current", req.RepairRunID)
	}
	return repairSnapshotProjection(snapshot), nil
}

func repairSnapshotProjection(snapshot sprint.RepairSnapshot) RepairStatusResult {
	state := snapshot.State
	out := RepairStatusResult{
		SchemaVersion:      1,
		Project:            state.Project,
		Sprint:             state.Sprint,
		Phase:              string(state.Phase),
		Fresh:              state.Freshness.Current,
		FreshnessReasons:   append([]string(nil), state.Freshness.Reasons...),
		RepairRunID:        state.RepairRunID,
		QAAttemptID:        state.QAAttemptID,
		OperationRunID:     state.Run.RunID,
		OperationalAttempt: state.Run.OperationalAttemptID,
		FencingGeneration:  state.Run.FencingGeneration,
		RunLifecycle:       string(state.Run.Lifecycle),
		Mode:               string(state.Mode),
		CurrentCycle:       state.CurrentCycle,
		EarliestCycle:      state.EarliestCycle,
		Outcome:            string(state.Outcome),
		StopReason:         string(state.StopReason),
		Deadline:           state.Deadline,
		UpdatedAt:          state.UpdatedAt,
		NextAction:         displaySafe(state.NextAction),
		Blocker:            qaBlockerProjection(state.Blocker),
	}
	if packet := snapshot.Packet; packet != nil {
		out.Packet = &RepairPacketSummary{
			Digest:             packet.PacketDigest,
			IssueID:            packet.Issue.ID,
			IssueTitle:         displaySafe(packet.Issue.Title),
			Target:             qaTargetProjection(packet.Target),
			AllowedPaths:       append([]string(nil), packet.AllowedPaths...),
			ForbiddenPaths:     append([]string(nil), packet.ForbiddenPaths...),
			AcceptanceCriteria: append([]string(nil), packet.AcceptanceCriteria...),
			CheckCount:         len(packet.Checks),
			Budgets: RepairBudgetSummary{
				MaxCycles: packet.Budgets.MaxCycles, MaxMutationCycles: packet.Budgets.MaxMutationCycles,
				MaxFiles: packet.Budgets.MaxFilesPerRun, MaxBytes: packet.Budgets.MaxBytesPerRun,
				WallTime: packet.Budgets.WallTime.String(), CommandTimeout: packet.Budgets.CommandTimeout.String(), Sources: append([]sprint.QAEffectiveSource(nil), packet.BudgetSources...),
			},
		}
	}
	if confirmation := snapshot.Confirmation; confirmation != nil {
		out.Confirmation = &RepairConfirmSummary{Digest: confirmation.ConfirmationDigest, PacketDigest: confirmation.PacketDigest, Confirmer: displaySafe(confirmation.Confirmer), OperationRunID: confirmation.OperationRunID, FencingGeneration: confirmation.FencingGeneration, ConfirmedAt: confirmation.ConfirmedAt}
	}
	if result := snapshot.Result; result != nil {
		out.Outcome = string(result.Outcome)
		out.StopReason = string(result.StopReason)
		out.CleanupComplete = result.CleanupComplete
		out.ProductionApplied = result.ProductionApplied
		out.CompleteLadder = result.CompleteLadder
		out.UnresolvedIssues = append([]string(nil), result.UnresolvedIssues...)
		out.NextAction = displaySafe(result.NextAction)
		out.Reason = displaySafe(result.Reason)
	}
	return out
}

func (u dashboardUseCases) QAShard(ctx context.Context, req QARequest) (QAShardResult, error) {
	status, err := u.QAStatus(ctx, req)
	if err != nil {
		return QAShardResult{}, err
	}
	shard, err := u.sprintService().QAShard(req.Project, req.Sprint, req.Shard)
	if err != nil {
		return QAShardResult{}, mapQAUseCaseError(err)
	}
	result := QAShardResult{QAResult: status, Shard: qaShardProjection(shard)}
	for _, theory := range shard.Theories {
		result.Theories = append(result.Theories, qaTheoryProjection(theory))
	}
	if len(result.Theories) > 24 {
		result.Theories = result.Theories[:24]
	}
	return result, nil
}

func (u dashboardUseCases) QATheory(ctx context.Context, req QARequest) (QATheoryResult, error) {
	status, err := u.QAStatus(ctx, req)
	if err != nil {
		return QATheoryResult{}, err
	}
	theory, err := u.sprintService().QATheory(req.Project, req.Sprint, req.Theory)
	if err != nil {
		return QATheoryResult{}, mapQAUseCaseError(err)
	}
	return QATheoryResult{QAResult: status, Theory: qaTheoryProjection(theory)}, nil
}

func (u dashboardUseCases) QASynthesis(ctx context.Context, req QARequest) (QASynthesisResult, error) {
	status, err := u.QAStatus(ctx, req)
	if err != nil {
		return QASynthesisResult{}, err
	}
	snapshot, err := u.sprintService().QAStatus(req.Project, req.Sprint)
	if err != nil {
		return QASynthesisResult{}, mapQAUseCaseError(err)
	}
	result := QASynthesisResult{QAResult: status}
	if snapshot.Synthesis == nil {
		return result, nil
	}
	result.ID, result.MapID, result.NextAction = snapshot.Synthesis.ID, snapshot.Synthesis.MapID, snapshot.Synthesis.NextAction
	result.TheoryIDs = append([]string(nil), snapshot.Synthesis.TheoryIDs...)
	for _, challenge := range snapshot.Synthesis.Challenges {
		result.Challenges = append(result.Challenges, QAChallengeSummary{ID: challenge.ID, TheoryIDs: append([]string(nil), challenge.TheoryIDs...), Claim: displaySafe(challenge.Claim), Basis: displaySafe(challenge.Basis), SafeEvidenceStrategy: displaySafe(challenge.SafeEvidenceStrategy), EvidenceRefs: append([]string(nil), challenge.EvidenceRefs...)})
	}
	result.OutcomeTotals = qaOutcomeProjection(snapshot.Synthesis.OutcomeCounts)
	result.Deduplicated = make(map[string][]string, len(snapshot.Synthesis.Deduplicated))
	for key, ids := range snapshot.Synthesis.Deduplicated {
		result.Deduplicated[key] = append([]string(nil), ids...)
	}
	for _, group := range snapshot.Synthesis.Contradictions {
		result.Contradictions = append(result.Contradictions, append([]string(nil), group...))
	}
	result.Interactions = append([]string(nil), snapshot.Synthesis.Interactions...)
	for i := range snapshot.Synthesis.Blockers {
		if blocker := qaBlockerProjection(&snapshot.Synthesis.Blockers[i]); blocker != nil {
			result.Blockers = append(result.Blockers, *blocker)
		}
	}
	for _, shard := range snapshot.Synthesis.FollowUpShards {
		result.FollowUpShards = append(result.FollowUpShards, qaShardProjection(shard))
	}
	return result, nil
}

func (u dashboardUseCases) QAEvidence(ctx context.Context, req QARequest) (QAEvidenceResult, error) {
	if err := ctx.Err(); err != nil {
		return QAEvidenceResult{}, err
	}
	if req.Evidence == "" {
		return QAEvidenceResult{}, fmt.Errorf("evidence ID is required")
	}
	if err := validateFocusedQARequest(req, "evidence"); err != nil {
		return QAEvidenceResult{}, err
	}
	record, err := u.sprintService().QAEvidence(req.Project, req.Sprint, req.Evidence)
	if err != nil {
		return QAEvidenceResult{}, mapQAUseCaseError(err)
	}
	return QAEvidenceResult{ID: record.ID, PlanID: record.PlanID, AttemptID: record.AttemptID, ShardID: record.ShardID, Outcome: string(record.Outcome), ReasonCode: displaySafe(record.ReasonCode), Repeatable: record.Repeatable, Contained: record.Contained, CleanupComplete: record.Cleanup.Complete, CommandCount: len(record.Commands), AnalyzerCount: len(record.Analyzers)}, nil
}

func (u dashboardUseCases) QAAdjudication(ctx context.Context, req QARequest) (QAAdjudicationResult, error) {
	if err := ctx.Err(); err != nil {
		return QAAdjudicationResult{}, err
	}
	if err := validateFocusedQARequest(req); err != nil {
		return QAAdjudicationResult{}, err
	}
	value, err := u.sprintService().QAAdjudication(req.Project, req.Sprint)
	if err != nil {
		return QAAdjudicationResult{}, mapQAUseCaseError(err)
	}
	result := QAAdjudicationResult{ID: value.ID, AttemptID: value.AttemptID, AcceptedCount: len(value.AcceptedIDs), IssueCount: len(value.Issues), EvaluatorCount: len(value.Evaluators)}
	for _, rejected := range value.Rejected {
		result.Rejected = append(result.Rejected, QARejectedSummary{EvidenceID: rejected.EvidenceID, Code: displaySafe(rejected.Code), Detail: displaySafe(rejected.Detail)})
	}
	return result, nil
}

func (u dashboardUseCases) QAIssues(ctx context.Context, req QARequest) (QAIssuePage, error) {
	if err := ctx.Err(); err != nil {
		return QAIssuePage{}, err
	}
	if err := validateFocusedQARequest(req, "cursor", "limit"); err != nil {
		return QAIssuePage{}, err
	}
	limit := req.Limit
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 200 {
		return QAIssuePage{}, fmt.Errorf("issue page limit must be between 1 and 200")
	}
	value, err := u.sprintService().QAAdjudication(req.Project, req.Sprint)
	if err != nil {
		return QAIssuePage{}, mapQAUseCaseError(err)
	}
	start := 0
	if req.Cursor != "" {
		decoded, decodeErr := base64.RawURLEncoding.DecodeString(req.Cursor)
		if decodeErr != nil {
			return QAIssuePage{}, fmt.Errorf("invalid issue cursor")
		}
		parts := strings.SplitN(string(decoded), "\x00", 2)
		if len(parts) != 2 || parts[0] != value.AttemptID {
			return QAIssuePage{}, fmt.Errorf("issue cursor is stale")
		}
		found := false
		for i, issue := range value.Issues {
			if issue.ID == parts[1] {
				start, found = i+1, true
				break
			}
		}
		if !found {
			return QAIssuePage{}, fmt.Errorf("issue cursor is stale")
		}
	}
	end := start + limit
	if end > len(value.Issues) {
		end = len(value.Issues)
	}
	page := QAIssuePage{OmittedCount: len(value.Issues) - end}
	for _, issue := range value.Issues[start:end] {
		page.Items = append(page.Items, qaIssueProjection(issue))
	}
	if end < len(value.Issues) && end > start {
		page.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(value.AttemptID + "\x00" + value.Issues[end-1].ID))
	}
	return page, nil
}

func (u dashboardUseCases) QAIssue(ctx context.Context, req QARequest) (QAIssueSummary, error) {
	if req.Issue == "" {
		return QAIssueSummary{}, fmt.Errorf("issue ID is required")
	}
	if err := validateFocusedQARequest(req, "issue"); err != nil {
		return QAIssueSummary{}, err
	}
	page, err := u.QAIssues(ctx, QARequest{Project: req.Project, Sprint: req.Sprint, Limit: 200})
	if err != nil {
		return QAIssueSummary{}, err
	}
	for _, issue := range page.Items {
		if issue.ID == req.Issue {
			return issue, nil
		}
	}
	return QAIssueSummary{}, fmt.Errorf("issue is not owned by the current QA attempt")
}

func (u dashboardUseCases) QAAssessment(ctx context.Context, req QARequest) (QAAssessmentResult, error) {
	if err := ctx.Err(); err != nil {
		return QAAssessmentResult{}, err
	}
	if err := validateFocusedQARequest(req); err != nil {
		return QAAssessmentResult{}, err
	}
	value, err := u.sprintService().QAAssessment(req.Project, req.Sprint)
	if err != nil {
		return QAAssessmentResult{}, mapQAUseCaseError(err)
	}
	result := QAAssessmentResult{Assessment: string(value.Assessment), ReviewVerdict: string(value.ReviewVerdict), SmokeVerdict: string(value.SmokeVerdict), EvidenceTotal: value.EvidenceTotal, RejectedTotal: value.RejectedTotal, IssueTotal: value.IssueTotal, NextAction: displaySafe(value.NextAction)}
	for i := range value.Blockers {
		if blocker := qaBlockerProjection(&value.Blockers[i]); blocker != nil {
			result.Blockers = append(result.Blockers, *blocker)
		}
	}
	return result, nil
}

func (u dashboardUseCases) QASmokeSuite(ctx context.Context, req QARequest) (QASmokeSuiteResult, error) {
	if err := ctx.Err(); err != nil {
		return QASmokeSuiteResult{}, err
	}
	if err := validateFocusedQARequest(req); err != nil {
		return QASmokeSuiteResult{}, err
	}
	status, err := u.sprintService().VerificationStatus(req.Project, req.Sprint)
	if err != nil {
		return QASmokeSuiteResult{}, err
	}
	return QASmokeSuiteResult{ExecutionStatus: status.Smoke.ExecutionStatus, Verdict: status.Smoke.Verdict, Fresh: status.Smoke.Fresh, RunID: status.Smoke.RunID, NextAction: displaySafe(status.Smoke.NextAction)}, nil
}

func validateFocusedQARequest(req QARequest, allowed ...string) error {
	permit := make(map[string]bool, len(allowed))
	for _, field := range allowed {
		permit[field] = true
	}
	present := map[string]bool{
		"shard": req.Shard != "", "theory": req.Theory != "", "run_id": req.RunID != "", "suite": req.Suite != "",
		"evidence": req.Evidence != "", "issue": req.Issue != "", "cursor": req.Cursor != "", "limit": req.Limit != 0,
	}
	for field, set := range present {
		if set && !permit[field] {
			return fmt.Errorf("focused QA query does not accept %s", field)
		}
	}
	return nil
}

func qaIssueProjection(issue sprint.QAIssue) QAIssueSummary {
	return QAIssueSummary{ID: issue.ID, Title: displaySafe(issue.Title), IssueClass: displaySafe(issue.IssueClass), Severity: displaySafe(issue.Severity), Location: displaySafe(issue.Location), EvidenceIDs: append([]string(nil), issue.EvidenceIDs...), PromotionReason: displaySafe(issue.PromotionReason), RepairEligible: issue.RepairEligible, RegressionCandidate: issue.RegressionCandidate}
}

func (u dashboardUseCases) RunQA(ctx context.Context, req QARequest, emit func(OperationEvent)) (QAResult, error) {
	return u.runQA(ctx, req, false, emit)
}

func (u dashboardUseCases) ResumeQA(ctx context.Context, req QARequest, emit func(OperationEvent)) (QAResult, error) {
	return u.runQA(ctx, req, true, emit)
}

func (u dashboardUseCases) runQA(ctx context.Context, req QARequest, resume bool, emit func(OperationEvent)) (QAResult, error) {
	if u.runner == nil {
		return QAResult{}, ErrWebUnavailable
	}
	kind := OperationQAStart
	if resume {
		kind = OperationQAResume
	}
	_, runErr := u.runner(ctx, OperationRequest{Kind: kind, Project: req.Project, Sprint: req.Sprint, Task: req.Shard, Suite: req.Suite}, emit)
	status, statusErr := u.QAStatus(context.WithoutCancel(ctx), req)
	return status, errors.Join(runErr, statusErr)
}

func (u dashboardUseCases) CancelQA(ctx context.Context, req QARequest) (QACancelResult, error) {
	if u.runs == nil {
		return QACancelResult{}, ErrWebUnavailable
	}
	runID := RunID(req.RunID)
	if err := runID.Validate(); err != nil {
		return QACancelResult{}, err
	}
	current, err := u.runs.Run(ctx, runID)
	if err != nil {
		return QACancelResult{}, err
	}
	if current.Target.Project != req.Project || current.Target.Sprint != req.Sprint || (current.Target.Operation != string(OperationQAStart) && current.Target.Operation != string(OperationQAResume)) {
		return QACancelResult{}, fmt.Errorf("QA run does not belong to the selected sprint")
	}
	run, requested, err := u.runs.CancelRun(ctx, runID, "QA cancellation requested")
	if err != nil {
		return QACancelResult{}, err
	}
	status, statusErr := u.QAStatus(ctx, req)
	return QACancelResult{Run: run, Requested: requested, QA: status}, statusErr
}

func (u dashboardUseCases) RecoverQA(ctx context.Context, req QARequest) (QAResult, error) {
	snapshot, err := u.sprintService().RecoverQA(ctx, req.Project, req.Sprint)
	if err != nil {
		return QAResult{}, mapQAUseCaseError(err)
	}
	return u.withQAConformanceReview(req, qaSnapshotProjection(snapshot)), nil
}

func (u dashboardUseCases) withQAConformanceReview(req QARequest, result QAResult) QAResult {
	status, err := u.sprintService().WithoutStatusWrites().Status(req.Project, req.Sprint)
	if err != nil {
		result.ConformanceReviewUnavailable = true
		result.ConformanceReviewDiagnostic = "Conformance Review status is unavailable. Inspect sprint status before relying on QA freshness."
		return result
	}
	result.ConformanceReviewStatus = string(status.Verification.Review.ExecutionStatus)
	result.ConformanceReviewVerdict = string(status.Verification.Review.Verdict)
	result.ConformanceReviewFresh = status.Verification.Review.Fresh
	return result
}

func qaMapProjection(qaMap sprint.QAMap) QAResult {
	result := QAResult{SchemaVersion: 1, Project: qaMap.Project, Sprint: qaMap.Sprint, Phase: string(sprint.QAPhaseMapped), Fresh: true, AttemptID: qaMap.SemanticAttemptID, GovernedInputFingerprint: qaMap.GovernedInputFingerprint, ImplementationFingerprint: qaMap.ImplementationFingerprint, ReviewFingerprint: qaMap.ReviewFingerprint, MapFingerprint: qaMap.ID, PolicyFingerprint: qaMap.PolicyFingerprint, CheckCatalogFingerprint: qaMap.CheckCatalogFingerprint, EffectiveSources: qaEffectiveSourcesProjection(qaMap.EffectiveSources), Target: qaTargetProjection(qaMap.Target), Coverage: qaCoverageProjection(qaMap.Coverage), InputRefs: qaArtifactRefsProjection(qaMap.InputRefs), Limits: qaLimitsProjection(qaMap.Budgets), ChangedPaths: len(qaMap.Coverage.ChangedPaths), CoveredPaths: len(qaMap.Coverage.PrimaryOwners), TotalShards: len(qaMap.Shards), NextAction: "Start QA evidence production from this current deterministic map."}
	for _, shard := range qaMap.Shards {
		result.Shards = append(result.Shards, qaShardProjection(shard))
	}
	if max := sprint.MaximumQABudgets().TotalShards; len(result.Shards) > max {
		result.Shards = result.Shards[:max]
	}
	return result
}

func qaSnapshotProjection(snapshot sprint.QASnapshot) QAResult {
	state := snapshot.State
	result := QAResult{SchemaVersion: 1, Project: state.Project, Sprint: state.Sprint, Phase: string(state.Phase), Fresh: state.Freshness.Current, FreshnessReasons: qaDisplayStrings(state.Freshness.Reasons), AttemptID: state.CurrentAttemptID, RunID: state.Run.RunID, OperationalAttemptID: state.Run.OperationalAttemptID, FencingGeneration: state.Run.FencingGeneration, RunLifecycle: string(state.Run.Lifecycle), TerminalResult: string(state.Run.TerminalResult), GovernedInputFingerprint: state.Freshness.GovernedInputFingerprint, ImplementationFingerprint: state.Freshness.ImplementationFingerprint, ReviewFingerprint: state.Freshness.ReviewFingerprint, PolicyFingerprint: state.Freshness.PolicyFingerprint, UpdatedAt: state.UpdatedAt, MapRecord: qaArtifactRefProjection(state.Map), SynthesisRecord: qaArtifactRefProjection(state.Synthesis), CompletedShards: state.CompletedShards, TotalShards: state.TotalShards, OutcomeTotals: qaOutcomeProjection(state.OutcomeCounts), Blocker: qaBlockerProjection(state.Blocker), Cancellation: QACancellationSummary{Requested: state.Cancellation.Requested, Scope: displaySafe(state.Cancellation.Scope), ShardID: state.Cancellation.ShardID, Reason: displaySafe(state.Cancellation.Reason), At: state.Cancellation.At}, NextAction: displaySafe(state.NextAction), Assessment: string(state.CanonicalAssessment), EvidenceCount: state.EvidenceCount, RejectedEvidenceCount: state.RejectedCount, IssueCount: state.IssueCount, RegressionCandidateCount: state.RegressionCandidates, CanonicalReport: qaArtifactRefProjection(state.CanonicalReport), CurrentFailure: qaBlockerProjection(state.CurrentFailure)}
	if snapshot.Map != nil {
		result.MapFingerprint = snapshot.Map.ID
		result.CheckCatalogFingerprint = snapshot.Map.CheckCatalogFingerprint
		result.EffectiveSources = qaEffectiveSourcesProjection(snapshot.Map.EffectiveSources)
		result.Target = qaTargetProjection(snapshot.Map.Target)
		result.Coverage = qaCoverageProjection(snapshot.Map.Coverage)
		result.InputRefs = qaArtifactRefsProjection(snapshot.Map.InputRefs)
		result.Limits = qaLimitsProjection(snapshot.Map.Budgets)
		result.ChangedPaths = len(snapshot.Map.Coverage.ChangedPaths)
		result.CoveredPaths = len(snapshot.Map.Coverage.PrimaryOwners)
	}
	for _, shard := range snapshot.Shards {
		result.Shards = append(result.Shards, qaShardProjection(shard))
	}
	return result
}

func qaArtifactRefProjection(ref *sprint.QAArtifactRef) *QAArtifactRefSummary {
	if ref == nil {
		return nil
	}
	return &QAArtifactRefSummary{Path: displaySafe(ref.Path), Digest: ref.Digest}
}

func qaArtifactRefsProjection(refs []sprint.QAArtifactRef) []QAArtifactRefSummary {
	result := make([]QAArtifactRefSummary, 0, len(refs))
	for i := range refs {
		if ref := qaArtifactRefProjection(&refs[i]); ref != nil {
			result = append(result, *ref)
		}
	}
	return result
}

func qaEffectiveSourcesProjection(sources []sprint.QAEffectiveSource) []QAEffectiveSourceSummary {
	result := make([]QAEffectiveSourceSummary, 0, len(sources))
	for _, source := range sources {
		result = append(result, QAEffectiveSourceSummary{Field: displaySafe(source.Field), Source: displaySafe(source.Source)})
	}
	return result
}

func qaTargetProjection(target sprint.QATargetIdentity) QATargetIdentitySummary {
	result := QATargetIdentitySummary{Fingerprint: target.Fingerprint, Scope: target.Scope, GitHead: target.GitHead, GitIndex: target.GitIndex, GitWorktree: target.GitWorktree, WorkspaceBranch: target.WorkspaceBranch, WorkspaceBaseline: target.WorkspaceBaseline, BaselineRelation: target.BaselineRelation, CommitsSinceBase: target.CommitsSinceBase, Categories: make(map[string]string, len(target.Categories))}
	for key, value := range target.Categories {
		result.Categories[displaySafe(key)] = displaySafe(value)
	}
	return result
}

func qaCoverageProjection(coverage sprint.QACoverage) QACoverageSummary {
	result := QACoverageSummary{ChangedPaths: qaDisplayStrings(coverage.ChangedPaths), PrimaryOwners: make(map[string]string, len(coverage.PrimaryOwners)), BoundaryOverlaps: make(map[string][]string, len(coverage.BoundaryOverlaps)), BlockedPaths: qaDisplayStrings(coverage.BlockedPaths)}
	for path, owner := range coverage.PrimaryOwners {
		result.PrimaryOwners[displaySafe(path)] = owner
	}
	for path, owners := range coverage.BoundaryOverlaps {
		result.BoundaryOverlaps[displaySafe(path)] = append([]string(nil), owners...)
	}
	return result
}

func qaLimitsProjection(b sprint.QABudgets) QALimitsSummary {
	return QALimitsSummary{
		ChangedPaths: b.ChangedPaths, TotalShards: b.TotalShards, PendingEntries: b.PendingEntries,
		ChangedPathsPerShard: b.ChangedPathsPerShard, ContextPathsPerShard: b.ContextPathsPerShard,
		ContextExpansions: b.ContextExpansions, PathsPerExpansion: b.PathsPerExpansion,
		BehavioralConcernsPerShard: b.BehavioralConcernsPerShard, TheoriesPerShard: b.TheoriesPerShard,
		IterationsPerAttempt: b.IterationsPerAttempt, CommandsPerAttempt: b.CommandsPerAttempt,
		OutputRepairAttempts: b.OutputRepairAttempts, ConcurrentInvestigators: b.ConcurrentInvestigators,
		CommandTimeout: b.CommandTimeout.String(), ShardTimeout: b.ShardTimeout.String(),
		RunTimeout: b.RunTimeout.String(), CleanupTimeout: b.CleanupTimeout.String(),
		CommandOutputBytes: b.CommandOutputBytes, ShardOutputBytes: b.ShardOutputBytes,
		PromptBytes: b.PromptBytes, FollowUpShards: b.FollowUpShards,
		TreeFiles: b.TreeFiles, TreeBytes: b.TreeBytes, FileBytes: b.FileBytes,
		GeneratedChecks: b.GeneratedChecks, GeneratedPatchBytes: b.GeneratedPatchBytes,
		EvidenceRecords: b.EvidenceRecords, Issues: b.Issues, AnalyzerCalls: b.AnalyzerCalls, EvaluatorCalls: b.EvaluatorCalls,
	}
}

func qaShardProjection(shard sprint.QAShard) QAShardSummary {
	result := QAShardSummary{ID: shard.ID, AttemptID: shard.AttemptID, Kind: string(shard.Kind), Title: displaySafe(shard.Title), Phase: string(shard.Phase), ChangedPaths: qaDisplayStrings(shard.ChangedPaths), ContextPaths: qaDisplayStrings(shard.ContextPaths), OverlapPaths: qaDisplayStrings(shard.OverlapPaths), BoundaryReason: displaySafe(shard.BoundaryReason), BehavioralConcerns: qaDisplayStrings(shard.BehavioralConcerns), ExpectationRefs: qaDisplayStrings(shard.ExpectationRefs), RiskTags: qaDisplayStrings(shard.RiskTags), ParentTheoryIDs: append([]string(nil), shard.ParentTheoryIDs...), TheoryCount: len(shard.Theories), Blocker: qaBlockerProjection(shard.Blocker)}
	for _, check := range shard.ApprovedChecks {
		result.ApprovedChecks = append(result.ApprovedChecks, QAApprovedCheckSummary{ID: check.ID, Fingerprint: check.Fingerprint})
	}
	for _, attempt := range shard.Attempts {
		result.Attempts = append(result.Attempts, qaAttemptProjection(attempt))
	}
	for _, theory := range shard.Theories {
		result.Theories = append(result.Theories, qaTheoryProjection(theory))
		if len(result.Theories) == 24 {
			break
		}
	}
	return result
}

func qaTheoryProjection(theory sprint.QATheory) QATheorySummary {
	result := QATheorySummary{ID: theory.ID, ShardID: theory.ShardID, Claim: displaySafe(theory.Claim), Basis: displaySafe(theory.Basis), VerificationSurface: displaySafe(theory.VerificationSurface), ExpectationRefs: qaDisplayStrings(theory.ExpectationRefs), SeverityIfConfirmed: displaySafe(theory.SeverityIfConfirmed), ConfirmationCondition: displaySafe(theory.ConfirmationCondition), RefutationCondition: displaySafe(theory.RefutationCondition), InconclusiveCondition: displaySafe(theory.InconclusiveCondition), SafeEvidenceStrategy: displaySafe(theory.SafeEvidenceStrategy), ImplementationFingerprint: theory.ImplementationFingerprint, Outcome: string(theory.Outcome), OutcomeReason: displaySafe(theory.OutcomeReason)}
	for _, attempt := range theory.AttemptHistory {
		result.AttemptHistory = append(result.AttemptHistory, qaAttemptProjection(attempt))
	}
	for _, evidence := range theory.Evidence {
		result.Evidence = append(result.Evidence, qaEvidenceProjection(evidence))
	}
	return result
}

func qaAttemptProjection(attempt sprint.QAInvestigatorAttempt) QAInvestigatorAttemptSummary {
	result := QAInvestigatorAttemptSummary{ID: attempt.ID, Number: attempt.Number, StartedAt: attempt.StartedAt, CompletedAt: attempt.CompletedAt, ImplementationBefore: attempt.ImplementationBefore, ImplementationAfter: attempt.ImplementationAfter, Usage: attempt.Usage, EstimatedCost: attempt.EstimatedCost, FailureKind: displaySafe(attempt.FailureKind), Retryable: attempt.Retryable, StopReason: displaySafe(attempt.StopReason), OutputDiagnostic: attempt.OutputDiagnostic, Repair: attempt.Repair}
	if attempt.CompletedAt != nil && !attempt.StartedAt.IsZero() {
		result.Duration = attempt.CompletedAt.Sub(attempt.StartedAt).Round(time.Millisecond).String()
	}
	for _, request := range attempt.ContextRequests {
		result.ContextRequests = append(result.ContextRequests, QAContextRequestSummary{ID: request.ID, Paths: qaDisplayStrings(request.Paths), Reason: displaySafe(request.Reason), Approved: request.Approved, DeniedReason: displaySafe(request.DeniedReason)})
	}
	for _, command := range attempt.Commands {
		result.Commands = append(result.Commands, QACommandSummary{CheckID: command.CheckID, DescriptorFingerprint: command.DescriptorFingerprint, ExitCode: command.ExitCode, Duration: command.Duration.Round(time.Millisecond).String(), StdoutDigest: command.StdoutDigest, StderrDigest: command.StderrDigest, OutputBytes: command.OutputBytes, Truncated: command.Truncated})
	}
	for _, evidence := range attempt.Evidence {
		result.Evidence = append(result.Evidence, qaEvidenceProjection(evidence))
	}
	return result
}

func qaEvidenceProjection(evidence sprint.QAEvidenceSummary) QAEvidenceSummary {
	return QAEvidenceSummary{Kind: displaySafe(evidence.Kind), Summary: displaySafe(evidence.Summary), Paths: qaDisplayStrings(evidence.Paths), CheckID: evidence.CheckID, OutputDigest: evidence.OutputDigest}
}

func qaDisplayStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, displaySafe(value))
	}
	return result
}

func qaBlockerProjection(blocker *sprint.QABlocker) *QABlockerSummary {
	if blocker == nil {
		return nil
	}
	return &QABlockerSummary{Category: string(blocker.Category), Scope: displaySafe(blocker.Scope), Summary: displaySafe(blocker.Summary), NextAction: displaySafe(blocker.NextAction)}
}

func qaOutcomeProjection(values map[sprint.QATheoryOutcome]int) map[string]int {
	result := make(map[string]int, len(values))
	for outcome, count := range values {
		result[string(outcome)] = count
	}
	return result
}

func summarizeReviewers(state *sprint.ReviewStageState, coverage []sprint.ReviewInput) []ReviewItemSummary {
	items := make([]ReviewItemSummary, 0, len(coverage))
	positions := make(map[string]int, len(coverage))
	for _, input := range coverage {
		if input.ID == "" {
			continue
		}
		positions[input.ID] = len(items)
		items = append(items, ReviewItemSummary{ID: input.ID, Name: input.Name, Kind: input.Kind, Path: input.Path, Status: "pending"})
	}
	ensure := func(id string) int {
		if index, ok := positions[id]; ok {
			return index
		}
		positions[id] = len(items)
		items = append(items, ReviewItemSummary{ID: id, Name: id, Status: "pending"})
		return len(items) - 1
	}
	if state == nil {
		return items
	}
	if state.LastComplete != nil {
		for _, result := range state.LastComplete.Coverage {
			index := ensure(result.CoverageID)
			items[index].Status = "completed"
			items[index].Summary = displaySafe(result.Summary)
		}
	}
	if state.Resume != nil {
		for _, checkpoint := range state.Resume.Coverage {
			index := ensure(checkpoint.CoverageID)
			items[index].Status = string(checkpoint.Status)
			if checkpoint.Result != nil {
				items[index].Summary = displaySafe(checkpoint.Result.Summary)
			}
		}
	}
	return items
}

func summarizeExecute(state *sprint.ExecuteRunState) ExecuteSummary {
	if state == nil {
		return ExecuteSummary{Message: "execute run-state unavailable"}
	}
	summary := ExecuteSummary{Available: true, Total: len(state.Tasks)}
	for _, task := range state.Tasks {
		switch task.Status {
		case sprint.ExecuteTaskPending:
			summary.Pending++
		case sprint.ExecuteTaskRunning:
			summary.Running++
		case sprint.ExecuteTaskComplete:
			summary.Complete++
		case sprint.ExecuteTaskDeferred:
			summary.Deferred++
		case sprint.ExecuteTaskFailed:
			summary.Failed++
			if len(task.Diagnostics) > 0 {
				diagnostic := task.Diagnostics[len(task.Diagnostics)-1]
				summary.Message = displaySafe(task.ID + ": " + diagnostic.Message)
			}
		case sprint.ExecuteTaskCancelled:
			summary.Cancelled++
		}
	}
	return summary
}

func displayReasons(values []string) string {
	reasons := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(displaySafe(value)); value != "" {
			reasons = append(reasons, value)
		}
	}
	return strings.Join(reasons, "; ")
}

func summarizeHistoricalExecute(status string) ExecuteSummary {
	summary := ExecuteSummary{Available: true, Total: 1, Message: "historical terminal execution evidence"}
	switch status {
	case "complete":
		summary.Complete = 1
	case "failed":
		summary.Failed = 1
	case "cancelled":
		summary.Cancelled = 1
	}
	return summary
}

// executeTerminalComplete keeps the delivery status independent from the
// downstream review/smoke assessment. Migrated task-addressable run state is
// complete when every planned task reached an accepted terminal outcome.
func executeTerminalComplete(summary ExecuteSummary) bool {
	return summary.Available && summary.Total > 0 && summary.Complete+summary.Deferred == summary.Total
}

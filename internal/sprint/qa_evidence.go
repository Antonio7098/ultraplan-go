package sprint

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type QACheckKind string

const (
	QACheckFact        QACheckKind = "fact"
	QACheckNegative    QACheckKind = "negative"
	QACheckBehavioral  QACheckKind = "behavioral"
	QACheckSemantic    QACheckKind = "semantic"
	QACheckAdversarial QACheckKind = "adversarial"
)

type QAEvidenceOutcome string

const (
	QAEvidencePass    QAEvidenceOutcome = "pass"
	QAEvidenceFail    QAEvidenceOutcome = "fail"
	QAEvidenceBlocked QAEvidenceOutcome = "blocked"
)

type QACleanupFacts struct {
	Attempted             bool   `json:"attempted"`
	DescendantsTerminated bool   `json:"descendants_terminated"`
	WorkspaceRemoved      bool   `json:"workspace_removed"`
	Complete              bool   `json:"complete"`
	Diagnostic            string `json:"diagnostic,omitempty"`
}

type QAEvidencePlan struct {
	SchemaVersion             int           `json:"schema_version"`
	ID                        string        `json:"id"`
	AttemptID                 string        `json:"attempt_id"`
	ShardID                   string        `json:"shard_id"`
	TheoryIDs                 []string      `json:"theory_ids"`
	ExpectationRefs           []string      `json:"expectation_refs"`
	Kind                      QACheckKind   `json:"kind"`
	ConfirmationCondition     string        `json:"confirmation_condition"`
	RefutationCondition       string        `json:"refutation_condition"`
	InconclusiveCondition     string        `json:"inconclusive_condition"`
	ApprovedPaths             []string      `json:"approved_paths"`
	CheckID                   string        `json:"check_id,omitempty"`
	Executable                string        `json:"executable,omitempty"`
	Args                      []string      `json:"args,omitempty"`
	EnvironmentNames          []string      `json:"environment_names,omitempty"`
	Timeout                   time.Duration `json:"timeout"`
	OutputLimit               int           `json:"output_limit"`
	RequireEmptyStdout        bool          `json:"require_empty_stdout,omitempty"`
	AnalyzerCalls             int           `json:"analyzer_calls"`
	CleanupRequired           bool          `json:"cleanup_required"`
	GovernedInputFingerprint  string        `json:"governed_input_fingerprint"`
	ImplementationFingerprint string        `json:"implementation_fingerprint"`
	MapFingerprint            string        `json:"map_fingerprint"`
	FrozenAt                  time.Time     `json:"frozen_at"`
}

type QACommandResult struct {
	Executable       string        `json:"executable"`
	ArgsDigest       string        `json:"args_digest"`
	ExitCode         int           `json:"exit_code"`
	Duration         time.Duration `json:"duration"`
	StdoutDigest     string        `json:"stdout_digest,omitempty"`
	StderrDigest     string        `json:"stderr_digest,omitempty"`
	OutputBytes      int           `json:"output_bytes"`
	Truncated        bool          `json:"truncated"`
	TimedOut         bool          `json:"timed_out"`
	Cancelled        bool          `json:"cancelled"`
	CleanupAttempted bool          `json:"cleanup_attempted"`
	CleanupComplete  bool          `json:"cleanup_complete"`
}

type QAModelObservation struct {
	CallID         string            `json:"call_id"`
	SessionID      string            `json:"session_id"`
	EvidenceDigest string            `json:"evidence_digest"`
	Outcome        QAEvidenceOutcome `json:"outcome"`
	Valid          bool              `json:"valid"`
	ReasonCode     string            `json:"reason_code"`
}

type QAEvidenceRecord struct {
	SchemaVersion             int                  `json:"schema_version"`
	ID                        string               `json:"id"`
	PlanID                    string               `json:"plan_id"`
	AttemptID                 string               `json:"attempt_id"`
	ShardID                   string               `json:"shard_id"`
	WorkspaceID               string               `json:"workspace_id"`
	WorkspaceIdentity         string               `json:"workspace_identity"`
	TargetIdentityBefore      string               `json:"target_identity_before"`
	TargetIdentityAfter       string               `json:"target_identity_after"`
	GovernedInputFingerprint  string               `json:"governed_input_fingerprint"`
	ImplementationFingerprint string               `json:"implementation_fingerprint"`
	MapFingerprint            string               `json:"map_fingerprint"`
	Patch                     *QAArtifactRef       `json:"patch,omitempty"`
	Commands                  []QACommandResult    `json:"commands,omitempty"`
	ChangedPaths              []string             `json:"changed_paths,omitempty"`
	Analyzers                 []QAModelObservation `json:"analyzers,omitempty"`
	Outcome                   QAEvidenceOutcome    `json:"outcome"`
	ReasonCode                string               `json:"reason_code"`
	Repeatable                bool                 `json:"repeatable"`
	Contained                 bool                 `json:"contained"`
	Cleanup                   QACleanupFacts       `json:"cleanup"`
	ExternalEvidence          []EvidenceReference  `json:"external_evidence,omitempty"`
	CompletedAt               time.Time            `json:"completed_at"`
}

type QARejectedEvidence struct {
	EvidenceID string `json:"evidence_id"`
	Code       string `json:"code"`
	Detail     string `json:"detail"`
}

type QARootCauseGroup struct {
	ID          string   `json:"id"`
	Claim       string   `json:"claim"`
	IssueClass  string   `json:"issue_class"`
	Location    string   `json:"location"`
	EvidenceIDs []string `json:"evidence_ids"`
}

type QAIssue struct {
	ID                  string   `json:"id"`
	RootCauseGroupID    string   `json:"root_cause_group_id"`
	Title               string   `json:"title"`
	IssueClass          string   `json:"issue_class"`
	Severity            string   `json:"severity"`
	Location            string   `json:"location"`
	EvidenceIDs         []string `json:"evidence_ids"`
	PromotionReason     string   `json:"promotion_reason"`
	RepairEligible      bool     `json:"repair_eligible"`
	RegressionCandidate bool     `json:"regression_candidate"`
}

type QAAdjudication struct {
	SchemaVersion  int                  `json:"schema_version"`
	ID             string               `json:"id"`
	AttemptID      string               `json:"attempt_id"`
	MapFingerprint string               `json:"map_fingerprint"`
	AcceptedIDs    []string             `json:"accepted_evidence_ids"`
	Rejected       []QARejectedEvidence `json:"rejected_evidence"`
	Groups         []QARootCauseGroup   `json:"root_cause_groups"`
	Issues         []QAIssue            `json:"issues"`
	Evaluators     []QAModelObservation `json:"evaluators,omitempty"`
	CompletedAt    time.Time            `json:"completed_at"`
}

type QAAssessmentRecord struct {
	SchemaVersion     int               `json:"schema_version"`
	ID                string            `json:"id"`
	AttemptID         string            `json:"attempt_id"`
	ReviewVerdict     ReviewVerdict     `json:"review_verdict"`
	ReviewFingerprint string            `json:"review_fingerprint"`
	SmokeVerdict      SmokeVerdict      `json:"smoke_verdict,omitempty"`
	SmokeRunID        string            `json:"smoke_run_id,omitempty"`
	Assessment        OverallAssessment `json:"assessment"`
	EvidenceTotal     int               `json:"evidence_total"`
	RejectedTotal     int               `json:"rejected_total"`
	IssueTotal        int               `json:"issue_total"`
	Blockers          []QABlocker       `json:"blockers,omitempty"`
	NextAction        string            `json:"next_action"`
	CompletedAt       time.Time         `json:"completed_at"`
}

type QAAdmission struct {
	ReviewCurrent       bool     `json:"review_current"`
	ReviewVerdict       string   `json:"review_verdict"`
	SmokeCurrent        bool     `json:"smoke_current"`
	SmokeVerdict        string   `json:"smoke_verdict"`
	ContainingSmoke     bool     `json:"containing_smoke"`
	ReadOnlyProofs      []string `json:"read_only_proofs"`
	MapComplete         bool     `json:"map_complete"`
	IsolationProven     bool     `json:"isolation_proven"`
	WritableConcurrency int      `json:"writable_concurrency"`
	Diagnostics         []string `json:"diagnostics,omitempty"`
}

func ValidateQAAdmission(admission QAAdmission) error {
	if !admission.ReviewCurrent || admission.ReviewVerdict != string(ReviewPass) && admission.ReviewVerdict != string(ReviewPassWithFindings) {
		return NewQAError(QAErrorAdmissionBlocked, "admission", "a current acceptable Conformance Review is required", nil)
	}
	if !admission.SmokeCurrent || !admission.ContainingSmoke || admission.SmokeVerdict != string(SmokePass) && admission.SmokeVerdict != string(SmokePassWithOpenIssues) {
		return NewQAError(QAErrorAdmissionBlocked, "admission", "current containing smoke evidence is required", nil)
	}
	if !admission.MapComplete || len(admission.ReadOnlyProofs) == 0 {
		return NewQAError(QAErrorAdmissionBlocked, "admission", "deterministic mapping and read-only QA proof are required", nil)
	}
	if !admission.IsolationProven {
		return NewQAError(QAErrorAdmissionBlocked, "admission", "writable isolation capabilities are not proven", nil)
	}
	if admission.WritableConcurrency != 1 {
		return NewQAError(QAErrorAdmissionBlocked, "admission", "writable QA concurrency must be exactly one", nil)
	}
	return nil
}

func ValidateQAEvidencePlan(plan QAEvidencePlan, budgets QABudgets) error {
	if plan.SchemaVersion != QAEvidenceSchemaVersion || !validQAV2ID(plan.ID, "plan") || !validQAIDKind(plan.AttemptID, "attempt") || !validQAIDKind(plan.ShardID, "shard") {
		return fmt.Errorf("invalid QA evidence plan schema or identity")
	}
	if !validQACheckKind(plan.Kind) || len(plan.ExpectationRefs) == 0 || len(plan.ApprovedPaths) == 0 {
		return fmt.Errorf("QA evidence plan requires kind, expectations, and approved paths")
	}
	for _, value := range []string{plan.ConfirmationCondition, plan.RefutationCondition, plan.InconclusiveCondition} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("QA evidence plan conditions are required")
		}
	}
	for _, path := range plan.ApprovedPaths {
		if err := validateQAPath(path); err != nil {
			return err
		}
	}
	if plan.CheckID != "" && !safeQAName(plan.CheckID) {
		return fmt.Errorf("QA evidence plan check ID is invalid")
	}
	if plan.Executable != "" {
		switch strings.ToLower(filepath.Base(plan.Executable)) {
		case "sh", "bash", "zsh", "fish", "cmd", "powershell", "pwsh", "git":
			return fmt.Errorf("QA evidence plan executable is prohibited")
		}
	}
	if plan.Timeout <= 0 || plan.Timeout > budgets.CommandTimeout || plan.OutputLimit <= 0 || plan.OutputLimit > budgets.CommandOutputBytes || !plan.CleanupRequired || plan.FrozenAt.IsZero() {
		return fmt.Errorf("QA evidence plan has invalid execution bounds")
	}
	expectedCalls := 0
	if plan.Kind == QACheckSemantic || plan.Kind == QACheckAdversarial {
		expectedCalls = 3
	}
	if plan.AnalyzerCalls != expectedCalls {
		return fmt.Errorf("QA evidence plan has invalid analyzer count")
	}
	for _, fingerprint := range []string{plan.GovernedInputFingerprint, plan.ImplementationFingerprint, plan.MapFingerprint} {
		if !validFingerprint(fingerprint) {
			return fmt.Errorf("QA evidence plan fingerprint is invalid")
		}
	}
	return nil
}

func ValidateQAEvidence(record QAEvidenceRecord, plan QAEvidencePlan, budgets QABudgets) error {
	if record.SchemaVersion != QAEvidenceSchemaVersion || !validQAV2ID(record.ID, "evidence") || record.PlanID != plan.ID || record.AttemptID != plan.AttemptID || record.ShardID != plan.ShardID {
		return fmt.Errorf("invalid QA evidence schema or ownership")
	}
	if record.GovernedInputFingerprint != plan.GovernedInputFingerprint || record.ImplementationFingerprint != plan.ImplementationFingerprint || record.MapFingerprint != plan.MapFingerprint {
		return fmt.Errorf("QA evidence is stale or mismatched")
	}
	if record.WorkspaceID == "" || !validFingerprint(record.WorkspaceIdentity) || !validFingerprint(record.TargetIdentityBefore) || record.TargetIdentityBefore != record.TargetIdentityAfter {
		return fmt.Errorf("QA evidence identity is invalid or target drifted")
	}
	if len(record.Commands) > budgets.CommandsPerAttempt || len(record.Analyzers) != plan.AnalyzerCalls || record.CompletedAt.IsZero() {
		return fmt.Errorf("QA evidence exceeds plan bounds")
	}
	unapprovedPath := ""
	for _, path := range record.ChangedPaths {
		if err := validateQAPath(path); err != nil {
			return fmt.Errorf("QA evidence contains an invalid changed path %q", path)
		}
		if unapprovedPath == "" && !qaPathApproved(path, plan.ApprovedPaths) {
			unapprovedPath = path
		}
	}
	if record.Patch != nil && (filepath.IsAbs(record.Patch.Path) || !validFingerprint(record.Patch.Digest)) {
		return fmt.Errorf("QA evidence patch reference is invalid")
	}
	if !record.Contained || !record.Cleanup.Complete || !record.Cleanup.DescendantsTerminated || !record.Cleanup.WorkspaceRemoved {
		return fmt.Errorf("QA evidence containment or cleanup is incomplete")
	}
	if !validEvidenceOutcome(record.Outcome) || strings.TrimSpace(record.ReasonCode) == "" {
		return fmt.Errorf("QA evidence outcome is invalid")
	}
	if unapprovedPath != "" && (record.Outcome != QAEvidenceBlocked || record.ReasonCode != "path_not_approved") {
		return fmt.Errorf("QA evidence changed unapproved path %q without a blocked scope-violation outcome", unapprovedPath)
	}
	seenSessions := map[string]struct{}{}
	for _, observation := range record.Analyzers {
		if !observation.Valid || observation.EvidenceDigest == "" || observation.CallID == "" || observation.SessionID == "" {
			return fmt.Errorf("QA analyzer result is incomplete")
		}
		if _, duplicate := seenSessions[observation.SessionID]; duplicate {
			return fmt.Errorf("QA analyzers must use fresh sessions")
		}
		seenSessions[observation.SessionID] = struct{}{}
	}
	return nil
}

func qaPathApproved(path string, approved []string) bool {
	for _, candidate := range approved {
		if path == candidate || strings.HasPrefix(path, strings.TrimSuffix(candidate, "/")+"/") {
			return true
		}
	}
	return false
}

func NewQAV2ID(kind, project, sprint, parent string, value any) (string, error) {
	if !safeQAName(project) || !safeQAName(sprint) || !validQAV2Kind(kind) {
		return "", fmt.Errorf("invalid QA v2 identity scope")
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return "", err
	}
	digest := sha256.Sum256(bytes.Join([][]byte{[]byte(QAEvidenceIDScope), []byte(kind), []byte(project), []byte(sprint), []byte(parent), bytes.TrimSpace(buf.Bytes())}, []byte{0}))
	return fmt.Sprintf("%s-%s-%s", QAEvidenceIDScope, kind, hex.EncodeToString(digest[:12])), nil
}

var qaV2IDPattern = regexp.MustCompile(`^qa-v2-(plan|evidence|patch|adjudication|issue|assessment|group)-[0-9a-f]{24}$`)

func validQAV2Kind(kind string) bool {
	switch kind {
	case "plan", "evidence", "patch", "adjudication", "issue", "assessment", "group":
		return true
	default:
		return false
	}
}

func validQAV2ID(value, kind string) bool {
	return qaV2IDPattern.MatchString(value) && strings.HasPrefix(value, QAEvidenceIDScope+"-"+kind+"-")
}

func validQACheckKind(kind QACheckKind) bool {
	switch kind {
	case QACheckFact, QACheckNegative, QACheckBehavioral, QACheckSemantic, QACheckAdversarial:
		return true
	default:
		return false
	}
}

func validEvidenceOutcome(outcome QAEvidenceOutcome) bool {
	return outcome == QAEvidencePass || outcome == QAEvidenceFail || outcome == QAEvidenceBlocked
}

func normalizeIssueLocation(value string) string {
	value = filepath.ToSlash(filepath.Clean(strings.TrimSpace(value)))
	if value == "." || strings.HasPrefix(value, "../") || filepath.IsAbs(value) {
		return "unknown"
	}
	return value
}

func sortedEvidenceIDs(records []QAEvidenceRecord) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.ID)
	}
	sort.Strings(ids)
	return ids
}

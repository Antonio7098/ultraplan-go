package sprint

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	pprocess "github.com/Antonio7098/ultraplan-go/internal/platform/process"
	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

var repairIDPattern = regexp.MustCompile(`^repair-v1-(run|check)-[0-9a-f]{24}$`)

const (
	repairProposalPromptID      = "ultraplan.bounded-repair.proposal"
	repairProposalPromptVersion = "1"
	repairProposalPromptBody    = `# Bounded repair proposal

Work only in the isolated copy provided as your working directory. Repair the one frozen issue in the packet. Modify only allowed_paths. Do not change tests, generated evidence, configuration, Git data, governed inputs, or acceptance criteria. Do not run Git, a shell, formatters, or cleanup commands. Do not claim success. Product code will derive the patch, enforce actual scope, apply it to production, and run every frozen check.

Frozen packet:
`
)

type RepairPathClass string

const (
	RepairPathProduction        RepairPathClass = "production"
	RepairPathGovernedInput     RepairPathClass = "governed_input"
	RepairPathVerification      RepairPathClass = "verification_evidence"
	RepairPathWorkspaceState    RepairPathClass = "workspace_state"
	RepairPathRepositoryControl RepairPathClass = "repository_control"
	RepairPathConfiguration     RepairPathClass = "configuration"
	RepairPathTestAsset         RepairPathClass = "test_or_acceptance_asset"
	RepairPathGeneratedEvidence RepairPathClass = "generated_evidence"
	RepairPathNonProduction     RepairPathClass = "non_production_data"
	RepairPathUnsafe            RepairPathClass = "unsafe"
)

type RepairPrepareRequest struct {
	IssueID       string
	Mode          RepairMode
	Budgets       RepairBudgets
	BudgetSources []QAEffectiveSource
	WriterToken   QAWriterToken
}

type RepairPrepareResult struct {
	Packet RepairIssuePacket `json:"packet"`
	State  RepairState       `json:"state"`
}

type RepairSnapshot struct {
	State        RepairState           `json:"state"`
	Packet       *RepairIssuePacket    `json:"packet,omitempty"`
	Confirmation *RepairConfirmation   `json:"confirmation,omitempty"`
	Result       *RepairResult         `json:"result,omitempty"`
	Cycles       []RepairCycleSnapshot `json:"cycles,omitempty"`
}

type RepairCycleSnapshot struct {
	Cycle          RepairCycle           `json:"cycle"`
	Scope          *RepairScopeRecord    `json:"scope,omitempty"`
	Reverification *RepairReverification `json:"reverification,omitempty"`
	Cleanup        *RepairCleanup        `json:"cleanup,omitempty"`
}

type RepairConfirmRequest struct {
	RepairRunID    string
	Confirmer      string
	AutomaticOptIn bool
	WriterToken    QAWriterToken
}

type RepairRunRequest struct {
	RepairRunID string
	WriterToken QAWriterToken
	Progress    func(RepairProgress)
}

type RepairRecoverRequest struct {
	RepairRunID string
	WriterToken QAWriterToken
}

// ResumeRepair continues only by reconciling the last durable boundary. It
// never grants a second proposal or replays a production apply. A repair that
// stopped before a provable continuation becomes a terminal blocked or
// escalated result through the same recovery path.
func (s Service) ResumeRepair(ctx context.Context, projectRef, sprintRef string, req RepairRunRequest) (RepairResult, error) {
	return s.RecoverRepair(ctx, projectRef, sprintRef, RepairRecoverRequest{RepairRunID: req.RepairRunID, WriterToken: req.WriterToken})
}

type RepairProgress struct {
	Phase   RepairPhase    `json:"phase"`
	Cycle   int            `json:"cycle"`
	Gate    RepairGateKind `json:"gate,omitempty"`
	Message string         `json:"message"`
}

// RequireAutomaticRepairProof performs the runtime-free admission check before
// a durable prepare operation is accepted or deduplicated.
func (s Service) RequireAutomaticRepairProof(projectRef, sprintRef string) error {
	sp, err := s.resolveMutationSprint(projectRef, sprintRef)
	if err != nil {
		return err
	}
	proof, err := NewQAStore(s.root, sp).LoadManualRepairProof()
	if err != nil || proof.Outcome != RepairOutcomeVerified && proof.Outcome != RepairOutcomeVerifiedWithFindings || !proof.CleanupComplete || !proof.ProductionApplied || !proof.CompleteLadder {
		return NewQAError(QAErrorAdmissionBlocked, "prepare automatic repair", "automatic repair is unavailable until qualifying manual proof exists", err)
	}
	return nil
}

func (s Service) RepairStatus(projectRef, sprintRef string) (RepairSnapshot, error) {
	sp, err := s.resolveMutationSprint(projectRef, sprintRef)
	if err != nil {
		return RepairSnapshot{}, err
	}
	store := NewQAStore(s.root, sp)
	state, err := store.LoadRepairState()
	if errors.Is(err, fs.ErrNotExist) {
		return RepairSnapshot{State: RepairState{SchemaVersion: QARepairSchemaVersion, Project: sp.Project, Sprint: sp.Slug, Phase: RepairPhaseStale, Freshness: RepairFreshness{Current: false, Reasons: []string{"no repair packet has been prepared"}}, NextAction: "Prepare one current repair-eligible QA issue."}}, nil
	}
	if err != nil {
		return RepairSnapshot{}, NewQAError(QAErrorPersistenceFailure, "load repair status", "repair state is unavailable", err)
	}
	out := RepairSnapshot{State: state}
	if state.Packet != nil {
		packet, loadErr := store.LoadRepairPacket(state.QAAttemptID, state.RepairRunID)
		if loadErr != nil {
			return RepairSnapshot{}, NewQAError(QAErrorPersistenceFailure, "load repair packet", "repair packet is unavailable", loadErr)
		}
		out.Packet = &packet
		if state.Confirmation != nil {
			confirmation, confirmationErr := store.LoadRepairConfirmation(state.QAAttemptID, state.RepairRunID, packet)
			if confirmationErr != nil {
				return RepairSnapshot{}, NewQAError(QAErrorPersistenceFailure, "load repair confirmation", "repair confirmation is unavailable", confirmationErr)
			}
			out.Confirmation = &confirmation
		}
	}
	if state.Result != nil {
		result, loadErr := store.LoadRepairResult(state.QAAttemptID, state.RepairRunID)
		if loadErr != nil {
			return RepairSnapshot{}, NewQAError(QAErrorPersistenceFailure, "load repair result", "repair result is unavailable", loadErr)
		}
		out.Result = &result
	}
	for cycleNumber := state.EarliestCycle; cycleNumber > 0 && cycleNumber <= state.Consumed.Cycles; cycleNumber++ {
		cycle, loadErr := store.LoadRepairCycle(state.QAAttemptID, state.RepairRunID, cycleNumber)
		if loadErr != nil {
			return RepairSnapshot{}, NewQAError(QAErrorPersistenceFailure, "load repair cycle", "repair cycle is unavailable", loadErr)
		}
		item := RepairCycleSnapshot{Cycle: cycle}
		if cycle.Scope != nil {
			var value RepairScopeRecord
			if loadErr := store.loadRepairCycleRecord(state.QAAttemptID, state.RepairRunID, cycleNumber, "scope.json", &value); loadErr != nil {
				return RepairSnapshot{}, loadErr
			}
			if loadErr := ValidateRepairScope(value); loadErr != nil || value.RepairRunID != state.RepairRunID || value.Cycle != cycleNumber {
				return RepairSnapshot{}, NewQAError(QAErrorInvalidState, "load repair scope", "repair scope is invalid or stored under the wrong identity", loadErr)
			}
			item.Scope = &value
		}
		if cycle.Reverification != nil {
			var value RepairReverification
			if loadErr := store.loadRepairCycleRecord(state.QAAttemptID, state.RepairRunID, cycleNumber, "reverification.json", &value); loadErr != nil {
				return RepairSnapshot{}, loadErr
			}
			if loadErr := ValidateRepairReverification(value); loadErr != nil || value.RepairRunID != state.RepairRunID || value.Cycle != cycleNumber {
				return RepairSnapshot{}, NewQAError(QAErrorInvalidState, "load repair reverification", "repair reverification is invalid or stored under the wrong identity", loadErr)
			}
			item.Reverification = &value
		}
		if cycle.Cleanup != nil {
			var value RepairCleanup
			if loadErr := store.loadRepairCycleRecord(state.QAAttemptID, state.RepairRunID, cycleNumber, "cleanup.json", &value); loadErr != nil {
				return RepairSnapshot{}, loadErr
			}
			if loadErr := ValidateRepairCleanup(value); loadErr != nil || value.RepairRunID != state.RepairRunID || value.Cycle != cycleNumber {
				return RepairSnapshot{}, NewQAError(QAErrorInvalidState, "load repair cleanup", "repair cleanup is invalid or stored under the wrong identity", loadErr)
			}
			item.Cleanup = &value
		}
		out.Cycles = append(out.Cycles, item)
	}
	return out, nil
}

func (s Service) PrepareRepair(ctx context.Context, projectRef, sprintRef string, req RepairPrepareRequest) (RepairPrepareResult, error) {
	if strings.TrimSpace(req.IssueID) == "" || !validRepairMode(req.Mode) {
		return RepairPrepareResult{}, NewQAError(QAErrorInvalidState, "prepare repair", "one current issue and repair mode are required", nil)
	}
	if err := req.WriterToken.Validate(); err != nil {
		return RepairPrepareResult{}, NewQAError(QAErrorConflict, "prepare repair", err.Error(), err)
	}
	if req.Budgets == (RepairBudgets{}) {
		req.Budgets = DefaultRepairBudgets()
	}
	// Sprint 38 deliberately supports one bounded cycle in both modes. Automatic
	// changes admission and confirmation, not the mutation engine or authority.
	req.Budgets.MaxCycles = 1
	req.Budgets.MaxMutationCycles = 1
	if err := ValidateLowerRepairBudgets(req.Budgets, DefaultRepairBudgets()); err != nil {
		return RepairPrepareResult{}, NewQAError(QAErrorInvalidState, "prepare repair", err.Error(), err)
	}
	lockedCtx, release, err := s.acquireMutationContext(ctx, projectRef, sprintRef)
	if err != nil {
		return RepairPrepareResult{}, err
	}
	defer release()
	if err := lockedCtx.Err(); err != nil {
		return RepairPrepareResult{}, err
	}
	sp, _, _, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return RepairPrepareResult{}, err
	}
	flow, err := LoadFlowState(s.root, sp)
	if err != nil {
		return RepairPrepareResult{}, NewQAError(QAErrorStaleInput, "prepare repair", "flow state is unavailable", err)
	}
	if req.Mode == RepairModeAutomatic {
		proof, proofErr := NewQAStore(s.root, sp).LoadManualRepairProof()
		if proofErr != nil || proof.Outcome != RepairOutcomeVerified && proof.Outcome != RepairOutcomeVerifiedWithFindings || !proof.CleanupComplete || !proof.ProductionApplied || !proof.CompleteLadder {
			return RepairPrepareResult{}, NewQAError(QAErrorAdmissionBlocked, "prepare automatic repair", "automatic repair is unavailable until qualifying manual proof exists", proofErr)
		}
	}
	if err := validateRepairFlowAdmission(flow); err != nil {
		return RepairPrepareResult{}, err
	}
	fence := s.qaWriterFence
	if fence == nil {
		expected := req.WriterToken
		fence = func(got QAWriterToken) error {
			if got != expected {
				return errors.New("writer token does not own this repair preparation")
			}
			return nil
		}
	}
	store := NewQAStore(s.root, sp).WithWriterFence(fence)
	priorRepair, priorRepairErr := store.LoadRepairState()
	if barrierErr := validateRepairPreparationBarrier(priorRepair, priorRepairErr); barrierErr != nil {
		return RepairPrepareResult{}, barrierErr
	}
	qaState, err := store.LoadState()
	if err != nil {
		return RepairPrepareResult{}, NewQAError(QAErrorAdmissionBlocked, "prepare repair", "current evidence-producing QA state is required", err)
	}
	if !qaState.Freshness.Current || qaState.Phase != QAPhaseCompleted || qaState.CurrentAttemptID == "" || qaState.CanonicalAssessment != AssessmentPassWithFindings && qaState.CanonicalAssessment != AssessmentPass {
		return RepairPrepareResult{}, NewQAError(QAErrorAdmissionBlocked, "prepare repair", "QA must be current, complete, and acceptable", nil)
	}
	if priorRepairErr == nil && priorRepair.Phase == RepairPhasePrepared && priorRepair.Freshness.Current && priorRepair.Mode == req.Mode && priorRepair.QAAttemptID == qaState.CurrentAttemptID {
		packet, loadErr := store.LoadRepairPacket(priorRepair.QAAttemptID, priorRepair.RepairRunID)
		if loadErr == nil && packet.Issue.ID == req.IssueID && packet.Budgets == req.Budgets {
			return RepairPrepareResult{Packet: packet, State: priorRepair}, nil
		}
	}
	qaMap, err := store.LoadMap(qaState.CurrentAttemptID)
	if err != nil {
		return RepairPrepareResult{}, err
	}
	adjudication, err := store.LoadAdjudication(qaState.CurrentAttemptID, qaMap.Budgets)
	if err != nil {
		return RepairPrepareResult{}, err
	}
	assessment, err := store.LoadAssessment(qaState.CurrentAttemptID)
	if err != nil || assessment.Assessment != qaState.CanonicalAssessment {
		return RepairPrepareResult{}, NewQAError(QAErrorAdmissionBlocked, "prepare repair", "QA assessment is missing or inconsistent", err)
	}
	issue, group, err := selectRepairIssue(adjudication, req.IssueID)
	if err != nil {
		return RepairPrepareResult{}, err
	}
	evidence, plans, err := loadRepairEvidence(store, qaMap, adjudication, issue)
	if err != nil {
		return RepairPrepareResult{}, err
	}
	manifest, _, manifestErr := s.PrepareReview(projectRef, sprintRef, ReviewRequest{})
	if manifestErr != nil || strings.TrimSpace(manifest.Target) == "" {
		return RepairPrepareResult{}, NewQAError(QAErrorStaleInput, "prepare repair", "recorded implementation worktree is unavailable", nil)
	}
	currentIdentity, err := targetIdentity(manifest.Target)
	if err != nil || currentIdentity != qaMap.ImplementationFingerprint || currentIdentity != qaMap.Target.Fingerprint {
		return RepairPrepareResult{}, NewQAError(QAErrorStaleInput, "prepare repair", "implementation target changed after QA", err)
	}
	capabilities := pprocess.IsolationCapabilityFacts()
	if !capabilities.NativeProtectedRootDeny || !capabilities.ProcessGroup || !capabilities.DescendantCleanup || !capabilities.WorkspaceRemoval {
		return RepairPrepareResult{}, NewQAError(QAErrorAdmissionBlocked, "prepare repair", "host isolation cannot prove target denial and cleanup", nil)
	}
	isolationFingerprint, err := repairDigest(capabilities)
	if err != nil {
		return RepairPrepareResult{}, err
	}
	policyFingerprint, err := repairDigest(struct {
		QA     string
		Repair RepairBudgets
	}{qaMap.PolicyFingerprint, req.Budgets})
	if err != nil {
		return RepairPrepareResult{}, err
	}
	now := s.now().UTC()
	runID, err := NewRepairRunID(sp.Project, sp.Slug, qaState.CurrentAttemptID, issue.ID, req.Mode, now)
	if err != nil {
		return RepairPrepareResult{}, err
	}
	checks, exact, err := freezeRepairChecks(runID, plans, evidence, qaMap, flow, req.Budgets)
	if err != nil {
		return RepairPrepareResult{}, err
	}
	allowed, err := repairAllowedPaths(issue, evidence)
	if err != nil {
		return RepairPrepareResult{}, err
	}
	planIDs := make([]string, 0, len(plans))
	for _, plan := range plans {
		planIDs = append(planIDs, plan.ID)
	}
	packet, err := FinalizeRepairPacket(RepairIssuePacket{
		Project: sp.Project, Sprint: sp.Slug, QAAttemptID: qaState.CurrentAttemptID, RepairRunID: runID,
		Issue: issue, RootCauseGroup: group, AdjudicationID: adjudication.ID, EvidenceIDs: issue.EvidenceIDs,
		PlanIDs: planIDs, MapID: qaMap.ID, ShardIDs: repairShardIDs(plans), TheoryIDs: repairTheoryIDs(plans),
		ExpectationRefs: repairExpectationRefs(plans), ExactReproducer: exact, Checks: checks, AllowedPaths: allowed,
		ForbiddenPaths: repairForbiddenPaths(), AcceptanceCriteria: repairAcceptanceCriteria(plans), Mode: req.Mode, Budgets: req.Budgets, BudgetSources: append([]QAEffectiveSource(nil), req.BudgetSources...),
		Target: qaMap.Target, GovernedInputFingerprint: qaMap.GovernedInputFingerprint, ImplementationFingerprint: qaMap.ImplementationFingerprint,
		ReviewFingerprint: flow.Review.Fingerprint, SmokeFingerprint: repairSmokeFingerprint(flow.Smoke), PolicyFingerprint: policyFingerprint,
		IsolationFingerprint: isolationFingerprint, PreparedAt: now,
	})
	if err != nil {
		return RepairPrepareResult{}, NewQAError(QAErrorAdmissionBlocked, "prepare repair", err.Error(), err)
	}
	state := RepairState{SchemaVersion: QARepairSchemaVersion, Project: sp.Project, Sprint: sp.Slug, QAAttemptID: qaState.CurrentAttemptID, RepairRunID: runID, Mode: req.Mode, Phase: RepairPhasePrepared, Freshness: RepairFreshness{Current: true}, Run: QARunCorrelation{Lifecycle: QARunAccepted, RunID: req.WriterToken.RunID, OperationalAttemptID: req.WriterToken.OperationalAttemptID, FencingGeneration: req.WriterToken.FencingGeneration}, Deadline: now.Add(req.Budgets.WallTime), NextAction: "Review the frozen issue packet and publish explicit confirmation.", UpdatedAt: now}
	if err := store.PublishRepairPacket(packet, state, flow, req.WriterToken); err != nil {
		return RepairPrepareResult{}, err
	}
	loaded, err := store.LoadRepairState()
	if err != nil {
		return RepairPrepareResult{}, err
	}
	return RepairPrepareResult{Packet: packet, State: loaded}, nil
}

func (s Service) ConfirmRepair(ctx context.Context, projectRef, sprintRef string, req RepairConfirmRequest) (RepairSnapshot, error) {
	if !validRepairID(req.RepairRunID, "run") || strings.TrimSpace(req.Confirmer) == "" {
		return RepairSnapshot{}, NewQAError(QAErrorInvalidState, "confirm repair", "repair run and confirmer are required", nil)
	}
	if err := req.WriterToken.Validate(); err != nil {
		return RepairSnapshot{}, NewQAError(QAErrorConflict, "confirm repair", err.Error(), err)
	}
	lockedCtx, release, err := s.acquireMutationContext(ctx, projectRef, sprintRef)
	if err != nil {
		return RepairSnapshot{}, err
	}
	defer release()
	if err := lockedCtx.Err(); err != nil {
		return RepairSnapshot{}, err
	}
	sp, _, _, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return RepairSnapshot{}, err
	}
	flow, err := LoadFlowState(s.root, sp)
	if err != nil {
		return RepairSnapshot{}, err
	}
	if err := validateRepairFlowAdmission(flow); err != nil {
		return RepairSnapshot{}, err
	}
	fence := s.repairWriterFence(req.WriterToken)
	store := NewQAStore(s.root, sp).WithWriterFence(fence)
	state, err := store.LoadRepairState()
	if err != nil {
		return RepairSnapshot{}, err
	}
	if state.RepairRunID != req.RepairRunID || state.Phase != RepairPhasePrepared || state.Confirmation != nil || !state.Freshness.Current {
		return RepairSnapshot{}, NewQAError(QAErrorConflict, "confirm repair", "repair packet is not current and unconfirmed", nil)
	}
	packet, err := store.LoadRepairPacket(state.QAAttemptID, state.RepairRunID)
	if err != nil {
		return RepairSnapshot{}, err
	}
	if packet.Mode == RepairModeAutomatic {
		if !req.AutomaticOptIn {
			return RepairSnapshot{}, NewQAError(QAErrorPermissionDenied, "confirm repair", "automatic repair requires a separate explicit opt-in", nil)
		}
		if err := s.validateAutomaticRepairProof(store, packet); err != nil {
			return RepairSnapshot{}, err
		}
	} else if req.AutomaticOptIn {
		return RepairSnapshot{}, NewQAError(QAErrorInvalidState, "confirm repair", "manual packet cannot accept automatic opt-in", nil)
	}
	manifest, _, manifestErr := s.PrepareReview(projectRef, sprintRef, ReviewRequest{})
	if manifestErr != nil {
		return RepairSnapshot{}, NewQAError(QAErrorStaleInput, "confirm repair", "cannot resolve current target", manifestErr)
	}
	identity, identityErr := targetIdentity(manifest.Target)
	if identityErr != nil || identity != packet.Target.Fingerprint {
		return RepairSnapshot{}, NewQAError(QAErrorStaleInput, "confirm repair", "target changed after packet review", identityErr)
	}
	now := s.now().UTC()
	confirmation, err := FinalizeRepairConfirmation(RepairConfirmation{Project: sp.Project, Sprint: sp.Slug, QAAttemptID: packet.QAAttemptID, RepairRunID: packet.RepairRunID, PacketDigest: packet.PacketDigest, Target: packet.Target, Mode: packet.Mode, AutomaticOptIn: req.AutomaticOptIn, Budgets: packet.Budgets, GovernedInputFingerprint: packet.GovernedInputFingerprint, PolicyFingerprint: packet.PolicyFingerprint, OperationRunID: req.WriterToken.RunID, OperationalAttemptID: req.WriterToken.OperationalAttemptID, FencingGeneration: req.WriterToken.FencingGeneration, Confirmer: strings.TrimSpace(req.Confirmer), ConfirmedAt: now}, packet)
	if err != nil {
		return RepairSnapshot{}, err
	}
	state.UpdatedAt = now
	state.NextAction = "Dispatch the confirmed repair through the shared durable runner."
	if err := store.PublishRepairConfirmation(confirmation, state, flow, req.WriterToken); err != nil {
		return RepairSnapshot{}, err
	}
	return s.RepairStatus(projectRef, sprintRef)
}

func (s Service) RunRepair(ctx context.Context, projectRef, sprintRef string, req RepairRunRequest) (result RepairResult, retErr error) {
	if !validRepairID(req.RepairRunID, "run") {
		return RepairResult{}, NewQAError(QAErrorInvalidState, "run repair", "repair run identity is required", nil)
	}
	if err := req.WriterToken.Validate(); err != nil {
		return RepairResult{}, NewQAError(QAErrorConflict, "run repair", err.Error(), err)
	}
	runtime := s.repairRuntime
	if runtime == nil {
		runtime = s.runtime
	}
	if runtime == nil {
		return RepairResult{}, NewQAError(QAErrorRuntimeUnavailable, "run repair", "a repair runtime is required", nil)
	}
	lockedCtx, release, err := s.acquireMutationContext(ctx, projectRef, sprintRef)
	if err != nil {
		return RepairResult{}, err
	}
	released := false
	defer func() {
		if !released {
			release()
		}
	}()
	sp, _, _, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return RepairResult{}, err
	}
	flow, err := LoadFlowState(s.root, sp)
	if err != nil {
		return RepairResult{}, err
	}
	fence := s.repairWriterFence(req.WriterToken)
	store := NewQAStore(s.root, sp).WithWriterFence(fence)
	state, err := store.LoadRepairState()
	if err != nil {
		return RepairResult{}, err
	}
	if state.RepairRunID != req.RepairRunID || state.Phase != RepairPhaseConfirmed || state.Confirmation == nil || state.Run.RunID != req.WriterToken.RunID || state.Run.OperationalAttemptID != req.WriterToken.OperationalAttemptID || state.Run.FencingGeneration != req.WriterToken.FencingGeneration {
		return RepairResult{}, NewQAError(QAErrorConflict, "run repair", "confirmed repair is not owned by this durable operation", nil)
	}
	packet, err := store.LoadRepairPacket(state.QAAttemptID, state.RepairRunID)
	if err != nil {
		return RepairResult{}, err
	}
	if _, err := store.LoadRepairConfirmation(state.QAAttemptID, state.RepairRunID, packet); err != nil {
		return RepairResult{}, err
	}
	manifest, _, manifestErr := s.PrepareReview(projectRef, sprintRef, ReviewRequest{})
	if manifestErr != nil {
		return RepairResult{}, NewQAError(QAErrorStaleInput, "run repair", "cannot resolve current target", manifestErr)
	}
	beforeIdentity, err := repairTargetIdentity(manifest.Target)
	if err != nil || beforeIdentity.Fingerprint != packet.Target.Fingerprint {
		return RepairResult{}, NewQAError(QAErrorStaleInput, "run repair", "target changed before proposal", err)
	}
	if !state.Deadline.IsZero() && !s.now().UTC().Before(state.Deadline) {
		return RepairResult{}, NewQAError(QAErrorBudgetExhausted, "run repair", "repair wall-time deadline expired before dispatch", nil)
	}
	cycleNumber := state.CurrentCycle + 1
	state.CurrentCycle = cycleNumber
	if state.EarliestCycle == 0 {
		state.EarliestCycle = cycleNumber
	}
	state.Phase = RepairPhaseProposing
	state.Run.Lifecycle = QARunActive
	state.NextAction = "Wait for one bounded isolated proposal."
	state.UpdatedAt = s.now().UTC()
	if err := store.publishRepairState(state, flow, req.WriterToken); err != nil {
		return RepairResult{}, err
	}
	emitRepair(req.Progress, RepairProgress{Phase: RepairPhaseProposing, Cycle: cycleNumber, Message: "Preparing isolated proposal"})
	parent, err := os.MkdirTemp("", "ultraplan-repair-parent-")
	if err != nil {
		return RepairResult{}, err
	}
	parentRemoved := false
	defer func() {
		if !parentRemoved {
			retErr = errors.Join(retErr, os.RemoveAll(parent))
		}
	}()
	limits := pprocess.IsolationLimits{MaxFiles: MaximumQABudgets().TreeFiles, MaxBytes: MaximumQABudgets().TreeBytes, MaxFileSize: MaximumQABudgets().FileBytes, Timeout: packet.Budgets.WallTime}
	workspace, err := pprocess.CreateIsolation(lockedCtx, pprocess.IsolationRequest{SourceRoot: manifest.Target, ParentDir: parent, Prefix: packet.RepairRunID, ProtectedRoots: []string{s.root, manifest.Target}, Limits: limits})
	if err != nil {
		return s.finishBlockedRepair(store, state, flow, req.WriterToken, release, &released, RepairStopPrerequisite, "cannot create a protected isolated repair workspace", err)
	}
	request, err := s.repairProposalRequest(packet, workspace.Path)
	if err != nil {
		return RepairResult{}, errors.Join(err, cleanupError(workspace.Cleanup()))
	}
	start := s.now().UTC()
	runtimeResult, runErr := runtime.StartRun(lockedCtx, request)
	runtimeCompleted := s.now().UTC()
	runtimeDuration := runtimeCompleted.Sub(start)
	state.Consumed.RuntimeAttempts++
	state.Consumed.ModelTurns += int(runtimeResult.Usage.Turns)
	runtimeEvents := runtimeResult.EventStats.Total
	if runtimeEvents == 0 {
		runtimeEvents = int64(len(runtimeResult.Events))
	}
	state.Runtime = &RepairRuntimeObservation{
		Provider:          request.Provider,
		Model:             request.Model,
		Variant:           request.Metadata["variant"],
		SessionID:         runtimeResult.SessionID,
		Usage:             qaUsageSummary(runtimeResult.Usage),
		StartedAt:         start,
		CompletedAt:       runtimeCompleted,
		Duration:          runtimeDuration,
		DurationMS:        runtimeDuration.Milliseconds(),
		RuntimeEvents:     runtimeEvents,
		RetainedEvents:    len(runtimeResult.Events),
		ObservedToolCalls: qaObservedToolCalls(runtimeResult.Events),
	}
	if runtimeResult.EstimatedCost != nil && runtimeResult.EstimatedCost.Source != "unpriced" && (runtimeResult.EstimatedCost.Source != "" || runtimeResult.EstimatedCost.Amount != 0) {
		state.Runtime.EstimatedCost = &QACostSummary{Amount: runtimeResult.EstimatedCost.Amount, Currency: runtimeResult.EstimatedCost.Currency, Estimate: runtimeResult.EstimatedCost.Estimate, Source: runtimeResult.EstimatedCost.Source}
	}
	if runErr != nil || lockedCtx.Err() != nil {
		cleanup := workspace.Cleanup()
		parentErr := os.Remove(parent)
		parentRemoved = parentErr == nil || errors.Is(parentErr, fs.ErrNotExist)
		stop := RepairStopPrerequisite
		if lockedCtx.Err() != nil {
			stop = RepairStopCancellation
		}
		return s.finishBlockedRepair(store, state, flow, req.WriterToken, release, &released, stop, "repair runtime did not produce a usable proposal", errors.Join(runErr, lockedCtx.Err(), cleanupError(cleanup), parentErr))
	}
	changedPaths, err := pprocess.CompareTrees(context.WithoutCancel(lockedCtx), manifest.Target, workspace.Path, limits)
	if err != nil || len(changedPaths) == 0 {
		cleanup := workspace.Cleanup()
		return s.finishBlockedRepair(store, state, flow, req.WriterToken, release, &released, RepairStopUncertainEvidence, "isolated proposal has no complete bounded diff", errors.Join(err, cleanupError(cleanup)))
	}
	proposal, replacements, preimages, changedBytes, err := deriveRepairProposal(manifest.Target, workspace.Path, changedPaths, packet)
	if err != nil {
		cleanup := workspace.Cleanup()
		return s.finishEscalatedRepair(store, state, flow, req.WriterToken, release, &released, RepairStopUnsupportedChange, "isolated proposal violated confirmed scope", errors.Join(err, cleanupError(cleanup)))
	}
	if err := store.PublishRepairProposal(state, flow, cycleNumber, proposal, req.WriterToken); err != nil {
		return RepairResult{}, errors.Join(err, cleanupError(workspace.Cleanup()))
	}
	state, err = store.LoadRepairState()
	if err != nil {
		return RepairResult{}, errors.Join(err, cleanupError(workspace.Cleanup()))
	}
	state.Phase = RepairPhaseApplying
	state.NextAction = "Apply the retained proposal through the product-owned boundary."
	state.UpdatedAt = s.now().UTC()
	if err := store.publishRepairState(state, flow, req.WriterToken); err != nil {
		return RepairResult{}, errors.Join(err, cleanupError(workspace.Cleanup()))
	}
	emitRepair(req.Progress, RepairProgress{Phase: RepairPhaseApplying, Cycle: cycleNumber, Message: "Applying confirmed production scope"})
	currentBefore, err := repairTargetIdentity(manifest.Target)
	if err != nil || currentBefore.Fingerprint != beforeIdentity.Fingerprint {
		return s.finishEscalatedRepair(store, state, flow, req.WriterToken, release, &released, RepairStopTargetDrift, "target changed before apply", errors.Join(err, cleanupError(workspace.Cleanup())))
	}
	journal, err := store.StageRepairApplyJournal(state, flow, cycleNumber, manifest.Target, replacements, preimages, req.WriterToken)
	if err != nil {
		return s.finishEscalatedRepair(store, state, flow, req.WriterToken, release, &released, RepairStopUnsupportedChange, "cannot durably stage production preimages", errors.Join(err, cleanupError(workspace.Cleanup())))
	}
	operations, appliedBytes, err := applyRepairFilesJournaled(manifest.Target, replacements, preimages, packet.Budgets.MaxFilesPerCycle, packet.Budgets.MaxBytesPerCycle, func(current []RepairApplyOperation) error {
		journal.State = "applying"
		journal.Operations = mergeRepairApplyOperations(current, journal.Operations)
		journal.UpdatedAt = s.now().UTC()
		return store.PublishRepairApplyJournal(state, flow, journal, req.WriterToken)
	})
	if err != nil {
		journal.Operations = mergeRepairApplyOperations(operations, journal.Operations)
		journal.State = "uncertain"
		if repairApplyCompensated(journal.Operations) {
			journal.State = "compensated"
		}
		journal.UpdatedAt = s.now().UTC()
		_ = store.PublishRepairApplyJournal(state, flow, journal, req.WriterToken)
		return s.finishEscalatedRepair(store, state, flow, req.WriterToken, release, &released, RepairStopUnsupportedChange, "product-owned apply failed or compensation is uncertain", errors.Join(err, cleanupError(workspace.Cleanup())))
	}
	journal.State = "applied"
	journal.Operations = mergeRepairApplyOperations(operations, journal.Operations)
	journal.UpdatedAt = s.now().UTC()
	if err := store.PublishRepairApplyJournal(state, flow, journal, req.WriterToken); err != nil {
		return RepairResult{}, errors.Join(err, cleanupError(workspace.Cleanup()))
	}
	state.Consumed.MutationCycles++
	state.Consumed.ChangedFiles += len(operations)
	state.Consumed.ChangedBytes += appliedBytes
	afterApply, err := repairTargetIdentity(manifest.Target)
	if err != nil {
		return s.finishEscalatedRepair(store, state, flow, req.WriterToken, release, &released, RepairStopTargetDrift, "cannot identify target after apply", errors.Join(err, cleanupError(workspace.Cleanup())))
	}
	scope := &RepairScopeRecord{SchemaVersion: QARepairSchemaVersion, RepairRunID: packet.RepairRunID, Cycle: cycleNumber, Before: beforeIdentity, After: afterApply, IntendedPaths: append([]string(nil), changedPaths...), ActualPaths: append([]string(nil), changedPaths...), ChangedBytes: changedBytes, Enforced: sameRepairPaths(changedPaths, mapKeys(replacements)) && repairPathsAllowed(changedPaths, packet.AllowedPaths)}
	if !scope.Enforced {
		return s.finishEscalatedRepair(store, state, flow, req.WriterToken, release, &released, RepairStopScopeGrowth, "actual production scope differs from the retained proposal", cleanupError(workspace.Cleanup()))
	}
	state.Phase = RepairPhaseReverifying
	state.NextAction = "Run the fixed progressive reverification ladder."
	state.UpdatedAt = s.now().UTC()
	if err := store.publishRepairState(state, flow, req.WriterToken); err != nil {
		return RepairResult{}, errors.Join(err, cleanupError(workspace.Cleanup()))
	}
	emitRepair(req.Progress, RepairProgress{Phase: RepairPhaseReverifying, Cycle: cycleNumber, Message: "Running progressive reverification"})
	reverification, exactRemoved, requiredPassed := s.runRepairReverification(lockedCtx, packet, manifest.Target, flow, cycleNumber, req.Progress)
	for _, gate := range reverification.Gates {
		if gate.Status != RepairGateSkipped {
			state.Consumed.Commands++
			state.Consumed.OutputBytes += gate.OutputBytes
		}
	}
	state.Phase = RepairPhaseCleaning
	state.NextAction = "Prove workspace, process, target, and lease cleanup."
	state.UpdatedAt = s.now().UTC()
	if err := store.publishRepairState(state, flow, req.WriterToken); err != nil {
		return RepairResult{}, errors.Join(err, cleanupError(workspace.Cleanup()))
	}
	cleanupStarted := s.now().UTC()
	cleanupResult := workspace.Cleanup()
	parentErr := os.Remove(parent)
	parentRemoved = parentErr == nil || errors.Is(parentErr, fs.ErrNotExist)
	finalTarget, targetErr := repairTargetIdentity(manifest.Target)
	cleanupDuration := s.now().UTC().Sub(cleanupStarted)
	cleanup := &RepairCleanup{SchemaVersion: QARepairSchemaVersion, RepairRunID: packet.RepairRunID, Cycle: cycleNumber, ProcessTreeTerminated: cleanupResult.Complete, WorkspaceRemoved: cleanupResult.Complete && parentRemoved, CompensationKnown: true, TargetCurrent: targetErr == nil && finalTarget.Fingerprint == afterApply.Fingerprint, LeaseReleased: false, Duration: cleanupDuration, DurationMS: cleanupDuration.Milliseconds(), Diagnostic: joinRepairDiagnostics(cleanupResult.Error, errorString(parentErr), errorString(targetErr))}
	cleanup.Complete = cleanup.ProcessTreeTerminated && cleanup.WorkspaceRemoved && cleanup.CompensationKnown && cleanup.TargetCurrent
	progress := RepairProgressFact{ExactFailureRemoved: exactRemoved, IssueCountBefore: 1, IssueCountAfter: boolInt(!exactRemoved), SeverityBefore: packet.Issue.Severity, SeverityAfter: chooseSeverity(exactRemoved, "", packet.Issue.Severity)}
	cycle := RepairCycle{SchemaVersion: QARepairSchemaVersion, RepairRunID: packet.RepairRunID, Number: cycleNumber, Progress: progress, StartedAt: start}
	completed := s.now().UTC()
	cycle.CompletedAt = &completed
	if !RepairMadeProgress(progress) {
		cycle.StopReason = RepairStopStagnation
		state.Consumed.StagnantCycles++
	}
	state.Consumed.Cycles = maxInt(state.Consumed.Cycles, cycleNumber)
	state.Phase = RepairPhaseTerminalizing
	state.NextAction = "Release the owned mutation lease before terminal publication."
	state.UpdatedAt = s.now().UTC()
	if err := store.publishRepairState(state, flow, req.WriterToken); err != nil {
		return RepairResult{}, err
	}
	release()
	released = true
	cleanup.LeaseReleased = true
	cleanup.Complete = cleanup.Complete && cleanup.LeaseReleased
	journal.State = "applied"
	journal.UpdatedAt = completed
	if err := store.PublishRepairCycle(RepairCyclePublication{Cycle: cycle, Proposal: proposal, Scope: scope, Reverification: &reverification, Cleanup: cleanup, Journal: &journal}, state, flow, req.WriterToken); err != nil {
		return RepairResult{}, err
	}
	state, err = store.LoadRepairState()
	if err != nil {
		return RepairResult{}, err
	}
	facts := RepairOutcomeFacts{Mode: packet.Mode, ExactIssueRemoved: exactRemoved, AllRequiredPassed: requiredPassed, OnlyNonBlocking: flow.Review.Verdict == ReviewPassWithFindings || flow.Smoke.Verdict == SmokePassWithOpenIssues, CleanupComplete: cleanup.Complete, TargetCurrent: cleanup.TargetCurrent, IssueStillReproduces: !exactRemoved, RequiredCheckFailed: !requiredPassed, UnsafeOrUncertain: !cleanup.Complete, Stagnated: !RepairMadeProgress(progress), StopReason: cycle.StopReason}
	outcome, outcomeErr := DeriveRepairOutcome(facts)
	if outcomeErr != nil {
		return RepairResult{}, outcomeErr
	}
	stop := cycle.StopReason
	if outcome == RepairOutcomeVerified || outcome == RepairOutcomeVerifiedWithFindings {
		stop = RepairStopVerified
	} else if outcome == RepairOutcomeFailed {
		stop = RepairStopRequiredCheckFailed
	} else if outcome == RepairOutcomeEscalated && stop == RepairStopNone {
		stop = RepairStopCleanupUncertain
	}
	evidenceRefs, evidenceErr := repairResultEvidence(store, state, cycleNumber)
	if evidenceErr != nil {
		return RepairResult{}, evidenceErr
	}
	result = RepairResult{SchemaVersion: QARepairSchemaVersion, Project: sp.Project, Sprint: sp.Slug, QAAttemptID: packet.QAAttemptID, RepairRunID: packet.RepairRunID, Mode: packet.Mode, Outcome: outcome, Reason: repairOutcomeReason(outcome), StopReason: stop, Consumed: state.Consumed, Runtime: state.Runtime, Target: finalTarget, CleanupComplete: cleanup.Complete, ProductionApplied: state.Consumed.MutationCycles > 0, CompleteLadder: requiredPassed, UnresolvedIssues: unresolvedRepairIssues(packet, exactRemoved), Evidence: evidenceRefs, NextAction: repairOutcomeNextAction(outcome), CompletedAt: s.now().UTC()}
	state.UpdatedAt = result.CompletedAt
	if err := store.PublishRepairResult(result, state, flow, req.WriterToken); err != nil {
		return RepairResult{}, err
	}
	if packet.Mode == RepairModeManual && (result.Outcome == RepairOutcomeVerified || result.Outcome == RepairOutcomeVerifiedWithFindings) {
		terminal, loadErr := store.LoadRepairState()
		if loadErr != nil || terminal.Result == nil {
			return RepairResult{}, errors.Join(loadErr, errors.New("terminal repair result pointer is unavailable"))
		}
		protocolFingerprint, protocolErr := repairDigest(struct {
			Schema         int
			Gates          []RepairGateKind
			Maximum        RepairBudgets
			PromptID       string
			PromptVersion  string
			PromptChecksum string
		}{QARepairSchemaVersion, RepairGateOrder(), MaximumRepairBudgets(), repairProposalPromptID, repairProposalPromptVersion, hashBytes([]byte(repairProposalPromptBody))})
		runtimeFingerprint, runtimeErr := repairDigest(struct {
			Provider string
			Model    string
			Variant  string
		}{request.Provider, request.Model, request.Metadata["variant"]})
		if protocolErr != nil || runtimeErr != nil {
			return RepairResult{}, errors.Join(protocolErr, runtimeErr)
		}
		proof := ManualRepairProof{SchemaVersion: QARepairSchemaVersion, Project: sp.Project, Sprint: sp.Slug, RepairRunID: packet.RepairRunID, PacketDigest: packet.PacketDigest, ResultDigest: terminal.Result.Digest, Outcome: result.Outcome, Target: result.Target, ProtocolFingerprint: protocolFingerprint, ImplementationFingerprint: result.Target.Fingerprint, PolicyFingerprint: packet.PolicyFingerprint, IsolationFingerprint: packet.IsolationFingerprint, GovernedInputFingerprint: packet.GovernedInputFingerprint, RuntimeFingerprint: runtimeFingerprint, CleanupComplete: result.CleanupComplete, ProductionApplied: result.ProductionApplied, CompleteLadder: result.CompleteLadder, PublishedAt: s.now().UTC()}
		if err := store.PublishManualRepairProof(proof, packet, result, protocolFingerprint, runtimeFingerprint, req.WriterToken); err != nil {
			return RepairResult{}, err
		}
	}
	return result, nil
}

// RecoverRepair reconciles an interrupted manual apply without granting new
// proposal authority. It either proves no production write occurred or uses
// retained private preimages to compensate exact postimages, then publishes a
// terminal semantic result under the recovery operation's writer fence.
func (s Service) RecoverRepair(ctx context.Context, projectRef, sprintRef string, req RepairRecoverRequest) (RepairResult, error) {
	if !validRepairID(req.RepairRunID, "run") {
		return RepairResult{}, NewQAError(QAErrorInvalidState, "recover repair", "repair run identity is required", nil)
	}
	if err := req.WriterToken.Validate(); err != nil {
		return RepairResult{}, NewQAError(QAErrorConflict, "recover repair", err.Error(), err)
	}
	lockedCtx, release, err := s.acquireMutationContext(ctx, projectRef, sprintRef)
	if err != nil {
		return RepairResult{}, err
	}
	released := false
	defer func() {
		if !released {
			release()
		}
	}()
	sp, _, _, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return RepairResult{}, err
	}
	flow, err := LoadFlowState(s.root, sp)
	if err != nil {
		return RepairResult{}, err
	}
	store := NewQAStore(s.root, sp).WithWriterFence(s.repairWriterFence(req.WriterToken))
	state, err := store.LoadRepairState()
	if err != nil {
		return RepairResult{}, err
	}
	if state.RepairRunID != req.RepairRunID {
		return RepairResult{}, NewQAError(QAErrorConflict, "recover repair", "selected repair is not current", nil)
	}
	if state.Phase == RepairPhaseTerminal {
		return store.LoadRepairResult(state.QAAttemptID, state.RepairRunID)
	}
	if err := validateRepairRecoverablePhase(state.Phase); err != nil {
		return RepairResult{}, err
	}
	packet, err := store.LoadRepairPacket(state.QAAttemptID, state.RepairRunID)
	if err != nil {
		return RepairResult{}, err
	}
	manifest, _, manifestErr := s.PrepareReview(projectRef, sprintRef, ReviewRequest{})
	if manifestErr != nil {
		return RepairResult{}, NewQAError(QAErrorStaleInput, "recover repair", "cannot resolve the production target for reconciliation", manifestErr)
	}
	state.Run = QARunCorrelation{Lifecycle: QARunClaimed, RunID: req.WriterToken.RunID, OperationalAttemptID: req.WriterToken.OperationalAttemptID, FencingGeneration: req.WriterToken.FencingGeneration}
	state.Phase = RepairPhaseInterrupted
	state.NextAction = "Reconcile retained apply preimages before terminal publication."
	state.UpdatedAt = s.now().UTC()
	if err := store.publishRepairState(state, flow, req.WriterToken); err != nil {
		return RepairResult{}, err
	}
	productionApplied := false
	cleanupComplete := true
	stop := RepairStopCancellation
	reason := "interrupted repair had no retained production apply"
	if state.CurrentCycle > 0 {
		journal, journalErr := store.LoadRepairApplyJournal(state.QAAttemptID, state.RepairRunID, state.CurrentCycle)
		if journalErr == nil {
			productionApplied, cleanupComplete = reconcileRepairJournal(store, manifest.Target, &journal)
			journal.UpdatedAt = s.now().UTC()
			if publishErr := store.PublishRepairApplyJournal(state, flow, journal, req.WriterToken); publishErr != nil {
				return RepairResult{}, publishErr
			}
			reason = "interrupted production apply was reconciled from digest-bound private preimages"
			if !cleanupComplete {
				stop = RepairStopCleanupUncertain
				reason = "interrupted production apply could not be compensated with certainty"
			}
		} else if !errors.Is(journalErr, fs.ErrNotExist) {
			return RepairResult{}, journalErr
		}
	}
	if productionApplied {
		state.Consumed.MutationCycles = maxInt(state.Consumed.MutationCycles, 1)
	}
	currentTarget := packet.Target
	observedTarget, targetErr := repairTargetIdentity(manifest.Target)
	if targetErr == nil {
		currentTarget = observedTarget
	}
	if targetErr != nil || currentTarget.Fingerprint != packet.Target.Fingerprint {
		cleanupComplete = false
		stop = RepairStopTargetDrift
		reason = "recovery could not prove the original target identity"
	}
	outcome := RepairOutcomeBlocked
	if productionApplied || !cleanupComplete {
		outcome = RepairOutcomeEscalated
	}
	state.Phase = RepairPhaseTerminalizing
	state.StopReason = stop
	state.NextAction = "Release recovery ownership before terminal publication."
	state.UpdatedAt = s.now().UTC()
	if err := store.publishRepairState(state, flow, req.WriterToken); err != nil {
		return RepairResult{}, err
	}
	release()
	released = true
	evidenceRefs, evidenceErr := repairResultEvidence(store, state, state.CurrentCycle)
	if evidenceErr != nil {
		return RepairResult{}, evidenceErr
	}
	result := RepairResult{SchemaVersion: QARepairSchemaVersion, Project: sp.Project, Sprint: sp.Slug, QAAttemptID: state.QAAttemptID, RepairRunID: state.RepairRunID, Mode: state.Mode, Outcome: outcome, Reason: reason, StopReason: stop, Consumed: state.Consumed, Runtime: state.Runtime, Target: currentTarget, CleanupComplete: cleanupComplete, ProductionApplied: productionApplied, CompleteLadder: false, UnresolvedIssues: []string{packet.Issue.ID}, Evidence: evidenceRefs, NextAction: repairOutcomeNextAction(outcome), CompletedAt: s.now().UTC()}
	state.UpdatedAt = result.CompletedAt
	if err := store.PublishRepairResult(result, state, flow, req.WriterToken); err != nil {
		return RepairResult{}, err
	}
	return result, lockedCtx.Err()
}

func reconcileRepairJournal(store QAStore, target string, journal *RepairApplyJournal) (productionApplied, complete bool) {
	complete = true
	journal.State = "compensated"
	for i := range journal.Operations {
		operation := &journal.Operations[i]
		targetPath := filepath.Join(target, filepath.FromSlash(operation.Path))
		if err := ensureRepairRegularPath(target, targetPath); err != nil {
			complete = false
			journal.State = "uncertain"
			continue
		}
		current, readErr := os.ReadFile(targetPath)
		if readErr != nil {
			complete = false
			journal.State = "uncertain"
			continue
		}
		switch digest := hashBytes(current); digest {
		case operation.PreimageDigest:
			continue
		case operation.PostimageDigest:
			productionApplied = true
			preimage, loadErr := store.loadRepairPreimage(*operation)
			if loadErr != nil || privateAtomicWrite(targetPath, preimage, "repair-recover-compensate", QAStateHooks{}) != nil {
				complete = false
				journal.State = "uncertain"
				continue
			}
			operation.Restored = true
		default:
			complete = false
			journal.State = "uncertain"
		}
	}
	return productionApplied, complete
}

func validateRepairPreparationBarrier(prior RepairState, loadErr error) error {
	if loadErr == nil && prior.Phase == RepairPhaseTerminalizing {
		return NewQAError(QAErrorConflict, "prepare repair", "a repair is terminalizing and still blocks new mutation", nil)
	}
	if loadErr != nil && !errors.Is(loadErr, fs.ErrNotExist) {
		return loadErr
	}
	return nil
}

func validateRepairRecoverablePhase(phase RepairPhase) error {
	switch phase {
	case RepairPhaseProposing, RepairPhaseApplying, RepairPhaseReverifying, RepairPhaseCleaning, RepairPhaseInterrupted:
		return nil
	default:
		return NewQAError(QAErrorInvalidState, "recover repair", "repair is not at a recoverable interrupted boundary", nil)
	}
}

func RepairGateOrder() []RepairGateKind {
	return []RepairGateKind{
		RepairGateExactReproducer,
		RepairGatePrimaryShards,
		RepairGateLinkedTheories,
		RepairGateFollowUpShards,
		RepairGateContainingQA,
		RepairGateContainingSmoke,
	}
}

func NewRepairRunID(project, sprint, attemptID, issueID string, mode RepairMode, preparedAt time.Time) (string, error) {
	if !safeQAName(project) || !safeQAName(sprint) || !validQAIDKind(attemptID, "attempt") || !validQAV2ID(issueID, "issue") || !validRepairMode(mode) || preparedAt.IsZero() {
		return "", fmt.Errorf("invalid repair run identity inputs")
	}
	data, err := canonicalQAJSON(struct {
		Project    string
		Sprint     string
		AttemptID  string
		IssueID    string
		Mode       RepairMode
		PreparedAt time.Time
	}{project, sprint, attemptID, issueID, mode, preparedAt.UTC()})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(append([]byte(QARepairIDScope+"\x00run\x00"), data...))
	return QARepairIDScope + "-run-" + hex.EncodeToString(digest[:12]), nil
}

func NewRepairCheckID(runID string, gate RepairGateKind, source string) (string, error) {
	if !validRepairID(runID, "run") || !validRepairGate(gate) || strings.TrimSpace(source) == "" {
		return "", fmt.Errorf("invalid repair check identity inputs")
	}
	data, err := canonicalQAJSON(struct {
		RunID  string
		Gate   RepairGateKind
		Source string
	}{runID, gate, strings.TrimSpace(source)})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(append([]byte(QARepairIDScope+"\x00check\x00"), data...))
	return QARepairIDScope + "-check-" + hex.EncodeToString(digest[:12]), nil
}

func FinalizeRepairPacket(packet RepairIssuePacket) (RepairIssuePacket, error) {
	packet.SchemaVersion = QARepairSchemaVersion
	packet.PreparedAt = packet.PreparedAt.UTC()
	packet.EvidenceIDs = normalizeQAStrings(packet.EvidenceIDs)
	packet.PlanIDs = normalizeQAStrings(packet.PlanIDs)
	packet.ShardIDs = normalizeQAStrings(packet.ShardIDs)
	packet.TheoryIDs = normalizeQAStrings(packet.TheoryIDs)
	packet.ExpectationRefs = normalizeQAStrings(packet.ExpectationRefs)
	packet.AcceptanceCriteria = normalizeQAStrings(packet.AcceptanceCriteria)
	var err error
	packet.AllowedPaths, err = NormalizeRepairPaths(packet.AllowedPaths)
	if err != nil {
		return RepairIssuePacket{}, err
	}
	packet.ForbiddenPaths, err = NormalizeRepairPaths(packet.ForbiddenPaths)
	if err != nil {
		return RepairIssuePacket{}, err
	}
	packet.PacketDigest = ""
	digest, err := repairDigest(packet)
	if err != nil {
		return RepairIssuePacket{}, err
	}
	packet.PacketDigest = digest
	if err := ValidateRepairPacket(packet); err != nil {
		return RepairIssuePacket{}, err
	}
	return packet, nil
}

func ValidateRepairPacket(packet RepairIssuePacket) error {
	if packet.SchemaVersion != QARepairSchemaVersion || !safeQAName(packet.Project) || !safeQAName(packet.Sprint) || !validQAIDKind(packet.QAAttemptID, "attempt") || !validRepairID(packet.RepairRunID, "run") {
		return fmt.Errorf("invalid repair packet schema or scope")
	}
	if !validQAV2ID(packet.Issue.ID, "issue") || !packet.Issue.RepairEligible || packet.Issue.RootCauseGroupID != packet.RootCauseGroup.ID || !validQAV2ID(packet.RootCauseGroup.ID, "group") {
		return fmt.Errorf("repair packet requires one eligible issue and its root-cause group")
	}
	if packet.AdjudicationID == "" || !validQAV2ID(packet.AdjudicationID, "adjudication") || packet.MapID == "" || !validQAIDKind(packet.MapID, "map") {
		return fmt.Errorf("repair packet authority records are incomplete")
	}
	if len(packet.EvidenceIDs) == 0 || len(packet.PlanIDs) == 0 || len(packet.ShardIDs) == 0 || len(packet.ExpectationRefs) == 0 || len(packet.AcceptanceCriteria) == 0 {
		return fmt.Errorf("repair packet lacks evidence, checks, or acceptance authority")
	}
	if len(packet.AllowedPaths) == 0 {
		return fmt.Errorf("repair packet requires a finite production path set")
	}
	for _, path := range packet.AllowedPaths {
		if ClassifyRepairPath(path) != RepairPathProduction {
			return fmt.Errorf("repair path %q is not mutable production", path)
		}
	}
	for _, path := range packet.ForbiddenPaths {
		if repairPathSetContains(packet.AllowedPaths, path) {
			return fmt.Errorf("repair path %q is both allowed and forbidden", path)
		}
	}
	if !validRepairMode(packet.Mode) || packet.Mode == RepairModeAutomatic && packet.Budgets.MaxCycles < 1 {
		return fmt.Errorf("invalid repair mode")
	}
	if err := ValidateRepairBudgets(packet.Budgets); err != nil {
		return err
	}
	if err := ValidateRepairCheck(packet.ExactReproducer, packet.Budgets); err != nil || packet.ExactReproducer.Gate != RepairGateExactReproducer {
		return fmt.Errorf("invalid exact reproducer: %w", err)
	}
	if err := validateRepairCheckSequence(packet.Checks, packet.Budgets); err != nil {
		return err
	}
	if !validRepairTarget(packet.Target) || packet.PreparedAt.IsZero() {
		return fmt.Errorf("repair packet target or timestamp is invalid")
	}
	for _, fingerprint := range []string{packet.GovernedInputFingerprint, packet.ImplementationFingerprint, packet.ReviewFingerprint, packet.SmokeFingerprint, packet.PolicyFingerprint, packet.IsolationFingerprint, packet.PacketDigest} {
		if !validFingerprint(fingerprint) {
			return fmt.Errorf("repair packet fingerprint is invalid")
		}
	}
	copyPacket := packet
	copyPacket.PacketDigest = ""
	digest, err := repairDigest(copyPacket)
	if err != nil || digest != packet.PacketDigest {
		return fmt.Errorf("repair packet digest mismatch")
	}
	return nil
}

func ValidateRepairCheck(check RepairCheckDescriptor, budgets RepairBudgets) error {
	if !validRepairID(check.ID, "check") || !validRepairGate(check.Gate) || strings.TrimSpace(check.Executable) == "" || strings.TrimSpace(check.Expected) == "" {
		return fmt.Errorf("repair check is incomplete")
	}
	switch strings.ToLower(filepath.Base(check.Executable)) {
	case "sh", "bash", "zsh", "fish", "cmd", "powershell", "pwsh", "git":
		return fmt.Errorf("repair check executable is prohibited")
	}
	if check.Workdir != "" {
		if _, err := normalizeRepairPath(check.Workdir); err != nil {
			return fmt.Errorf("repair check workdir is unsafe")
		}
	}
	for _, value := range append(append([]string{}, check.Args...), check.EnvironmentNames...) {
		if strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("repair check contains unsafe bytes")
		}
	}
	if check.Timeout <= 0 || check.Timeout > budgets.CommandTimeout || check.OutputLimit <= 0 || check.OutputLimit > budgets.OutputBytes {
		return fmt.Errorf("repair check exceeds frozen bounds")
	}
	return nil
}

func ValidateRepairBudgets(got RepairBudgets) error {
	max := MaximumRepairBudgets()
	positive := []struct {
		name string
		got  int
		max  int
	}{
		{"max_cycles", got.MaxCycles, max.MaxCycles}, {"max_mutation_cycles", got.MaxMutationCycles, max.MaxMutationCycles},
		{"max_reopenings", got.MaxReopenings, max.MaxReopenings}, {"stagnation_limit", got.StagnationLimit, max.StagnationLimit},
		{"max_files_per_cycle", got.MaxFilesPerCycle, max.MaxFilesPerCycle}, {"max_files_per_run", got.MaxFilesPerRun, max.MaxFilesPerRun},
		{"max_patch_bytes", got.MaxPatchBytes, max.MaxPatchBytes}, {"runtime_attempts", got.RuntimeAttempts, max.RuntimeAttempts},
		{"model_turns", got.ModelTurns, max.ModelTurns}, {"command_count", got.CommandCount, max.CommandCount},
		{"output_bytes", got.OutputBytes, max.OutputBytes}, {"retained_cycles", got.RetainedCycles, max.RetainedCycles},
	}
	for _, field := range positive {
		if field.got <= 0 || field.got > field.max {
			return fmt.Errorf("repair budget %s must be between 1 and %d", field.name, field.max)
		}
	}
	if got.MaxMutationCycles > got.MaxCycles || got.MaxFilesPerCycle > got.MaxFilesPerRun {
		return fmt.Errorf("repair per-cycle budgets cannot exceed run budgets")
	}
	if got.MaxBytesPerCycle <= 0 || got.MaxBytesPerCycle > max.MaxBytesPerCycle || got.MaxBytesPerRun <= 0 || got.MaxBytesPerRun > max.MaxBytesPerRun || got.MaxBytesPerCycle > got.MaxBytesPerRun {
		return fmt.Errorf("repair byte budgets are invalid")
	}
	if got.WallTime <= 0 || got.WallTime > max.WallTime || got.CommandTimeout <= 0 || got.CommandTimeout > max.CommandTimeout || got.CleanupTimeout <= 0 || got.CleanupTimeout > max.CleanupTimeout {
		return fmt.Errorf("repair duration budgets are invalid")
	}
	return nil
}

func ValidateLowerRepairBudgets(requested, ceiling RepairBudgets) error {
	if err := ValidateRepairBudgets(requested); err != nil {
		return err
	}
	if err := ValidateRepairBudgets(ceiling); err != nil {
		return fmt.Errorf("invalid repair budget ceiling: %w", err)
	}
	requestedJSON, _ := json.Marshal(requested)
	ceilingJSON, _ := json.Marshal(ceiling)
	var req, max map[string]any
	_ = json.Unmarshal(requestedJSON, &req)
	_ = json.Unmarshal(ceilingJSON, &max)
	for field, value := range req {
		if number, ok := value.(float64); ok {
			if ceilingNumber, exists := max[field].(float64); !exists || number > ceilingNumber {
				return fmt.Errorf("repair budget %s may only be lowered", field)
			}
		}
	}
	return nil
}

func FinalizeRepairConfirmation(value RepairConfirmation, packet RepairIssuePacket) (RepairConfirmation, error) {
	value.SchemaVersion = QARepairSchemaVersion
	value.ConfirmedAt = value.ConfirmedAt.UTC()
	value.ConfirmationDigest = ""
	digest, err := repairDigest(value)
	if err != nil {
		return RepairConfirmation{}, err
	}
	value.ConfirmationDigest = digest
	if err := ValidateRepairConfirmation(value, packet); err != nil {
		return RepairConfirmation{}, err
	}
	return value, nil
}

func ValidateRepairConfirmation(value RepairConfirmation, packet RepairIssuePacket) error {
	if err := ValidateRepairPacket(packet); err != nil {
		return err
	}
	if value.SchemaVersion != QARepairSchemaVersion || value.Project != packet.Project || value.Sprint != packet.Sprint || value.QAAttemptID != packet.QAAttemptID || value.RepairRunID != packet.RepairRunID || value.PacketDigest != packet.PacketDigest {
		return fmt.Errorf("repair confirmation scope does not match packet")
	}
	if value.Mode != packet.Mode || value.AutomaticOptIn != (value.Mode == RepairModeAutomatic) || value.Budgets != packet.Budgets || value.Target.Fingerprint != packet.Target.Fingerprint || value.GovernedInputFingerprint != packet.GovernedInputFingerprint || value.PolicyFingerprint != packet.PolicyFingerprint {
		return fmt.Errorf("repair confirmation authority changed after packet review")
	}
	if strings.TrimSpace(value.OperationRunID) == "" || strings.TrimSpace(value.OperationalAttemptID) == "" || value.FencingGeneration == 0 || strings.TrimSpace(value.Confirmer) == "" || value.ConfirmedAt.IsZero() || !validFingerprint(value.ConfirmationDigest) {
		return fmt.Errorf("repair confirmation acceptance facts are incomplete")
	}
	copyValue := value
	copyValue.ConfirmationDigest = ""
	digest, err := repairDigest(copyValue)
	if err != nil || digest != value.ConfirmationDigest {
		return fmt.Errorf("repair confirmation digest mismatch")
	}
	return nil
}

func NormalizeRepairPaths(values []string) ([]string, error) {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		normalized, err := normalizeRepairPath(value)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out, nil
}

func ClassifyRepairPath(value string) RepairPathClass {
	path, err := normalizeRepairPath(value)
	if err != nil {
		return RepairPathUnsafe
	}
	lower := strings.ToLower(path)
	base := strings.ToLower(filepath.Base(path))
	components := strings.Split(lower, "/")
	for _, component := range components {
		switch component {
		case ".git", ".hg", ".svn":
			return RepairPathRepositoryControl
		case "verification", "evidence", "snapshots", "baselines", "testdata", "golden", "goldens":
			return RepairPathGeneratedEvidence
		case ".ultra", ".ultraplan":
			return RepairPathWorkspaceState
		case "test", "tests", "acceptance", "fixtures":
			return RepairPathTestAsset
		}
	}
	switch base {
	case "readme", "readme.md", "readme.txt", "license", "license.md", "changelog.md":
		return RepairPathNonProduction
	case "requirements.md", "code-context.md", "sprint-index.md", "technical-handbook.md", "reasoning.md", "plan.md", "execute.md", "review.md", "smoke.md", "qa.md":
		return RepairPathGovernedInput
	case "flow-state.json", ".workspace.json", ".run-state.json", "repair-state.json", "manual-repair-proof.json":
		return RepairPathWorkspaceState
	case ".gitignore", ".gitattributes", ".gitmodules":
		return RepairPathRepositoryControl
	case "ultraplan.yml", "ultraplan.yaml", ".env", "go.mod", "go.sum":
		return RepairPathConfiguration
	}
	if strings.HasSuffix(lower, "_test.go") || strings.HasSuffix(lower, ".snap") || strings.HasSuffix(lower, ".golden") || strings.Contains(lower, "snapshot") || strings.Contains(lower, "baseline") {
		return RepairPathTestAsset
	}
	if strings.HasPrefix(lower, "docs/") || strings.HasPrefix(lower, "projects/") || strings.HasPrefix(lower, "system/") || strings.HasPrefix(lower, "prompts/") || strings.HasPrefix(lower, "templates/") {
		return RepairPathNonProduction
	}
	return RepairPathProduction
}

func RepairMadeProgress(fact RepairProgressFact) bool {
	if fact.ExactFailureRemoved || fact.IssueCountAfter < fact.IssueCountBefore {
		return true
	}
	return repairSeverityRank(fact.SeverityAfter) < repairSeverityRank(fact.SeverityBefore)
}

type RepairOutcomeFacts struct {
	Mode                 RepairMode
	ExactIssueRemoved    bool
	AllRequiredPassed    bool
	OnlyNonBlocking      bool
	CleanupComplete      bool
	TargetCurrent        bool
	IssueStillReproduces bool
	RequiredCheckFailed  bool
	PrerequisiteMissing  bool
	Cancelled            bool
	UnsafeOrUncertain    bool
	Stagnated            bool
	StopReason           RepairStopReason
}

func DeriveRepairOutcome(facts RepairOutcomeFacts) (RepairOutcome, error) {
	if !validRepairMode(facts.Mode) {
		return "", fmt.Errorf("invalid repair mode")
	}
	if facts.UnsafeOrUncertain || facts.StopReason == RepairStopCleanupUncertain || facts.StopReason == RepairStopScopeGrowth || facts.StopReason == RepairStopSeverityGrowth || facts.StopReason == RepairStopTargetDrift || facts.StopReason == RepairStopGovernedDrift || facts.StopReason == RepairStopDesignDecision || facts.StopReason == RepairStopContradiction || facts.StopReason == RepairStopUnsupportedChange || facts.StopReason == RepairStopUncertainEvidence || facts.StopReason == RepairStopUnknownSchema {
		return RepairOutcomeEscalated, nil
	}
	if facts.PrerequisiteMissing || facts.Cancelled || facts.StopReason == RepairStopPrerequisite || facts.StopReason == RepairStopCancellation {
		return RepairOutcomeBlocked, nil
	}
	if facts.ExactIssueRemoved && facts.AllRequiredPassed && facts.CleanupComplete && facts.TargetCurrent {
		if facts.OnlyNonBlocking {
			return RepairOutcomeVerifiedWithFindings, nil
		}
		return RepairOutcomeVerified, nil
	}
	if facts.Mode == RepairModeAutomatic && (facts.Stagnated || facts.StopReason == RepairStopStagnation || facts.StopReason == RepairStopRepeatedPatch || facts.StopReason == RepairStopRepeatedTarget || facts.StopReason == RepairStopCycleLimit || facts.StopReason == RepairStopReopeningLimit) {
		return RepairOutcomeStalled, nil
	}
	if facts.IssueStillReproduces || facts.RequiredCheckFailed || facts.StopReason == RepairStopRequiredCheckFailed {
		return RepairOutcomeFailed, nil
	}
	return RepairOutcomeBlocked, nil
}

func QualifyManualRepairProof(proof ManualRepairProof, packet RepairIssuePacket, result RepairResult, protocol, runtime string) error {
	if proof.SchemaVersion != QARepairSchemaVersion || proof.Project != packet.Project || proof.Sprint != packet.Sprint || proof.RepairRunID != packet.RepairRunID || result.RepairRunID != packet.RepairRunID || result.Mode != RepairModeManual {
		return fmt.Errorf("manual repair proof scope is invalid")
	}
	if proof.PacketDigest != packet.PacketDigest || proof.Outcome != result.Outcome || proof.Outcome != RepairOutcomeVerified && proof.Outcome != RepairOutcomeVerifiedWithFindings {
		return fmt.Errorf("manual repair proof does not name a qualifying result")
	}
	if !proof.CleanupComplete || !proof.ProductionApplied || !proof.CompleteLadder || proof.PublishedAt.IsZero() {
		return fmt.Errorf("manual repair proof lacks production, ladder, or cleanup evidence")
	}
	if proof.Target.Fingerprint != result.Target.Fingerprint || !result.CleanupComplete || !result.ProductionApplied || !result.CompleteLadder || proof.ImplementationFingerprint != result.Target.Fingerprint || proof.PolicyFingerprint != packet.PolicyFingerprint || proof.IsolationFingerprint != packet.IsolationFingerprint || proof.GovernedInputFingerprint != packet.GovernedInputFingerprint {
		return fmt.Errorf("manual repair proof fingerprint mismatch")
	}
	if proof.ProtocolFingerprint != protocol || proof.RuntimeFingerprint != runtime || !validFingerprint(proof.ResultDigest) {
		return fmt.Errorf("manual repair proof protocol or runtime mismatch")
	}
	return nil
}

func validateRepairFlowAdmission(flow FlowState) error {
	if flow.Review == nil || flow.Review.Stale || flow.Review.Status != ReviewCompleted || flow.Review.Verdict != ReviewPass && flow.Review.Verdict != ReviewPassWithFindings || !validFingerprint(flow.Review.Fingerprint) {
		return NewQAError(QAErrorAdmissionBlocked, "prepare repair", "a current acceptable Conformance Review is required", nil)
	}
	if flow.Smoke == nil || flow.Smoke.Stale || flow.Smoke.Status != SmokeCompleted || flow.Smoke.Verdict != SmokePass && flow.Smoke.Verdict != SmokePassWithOpenIssues || !validFingerprint(repairSmokeFingerprint(flow.Smoke)) {
		return NewQAError(QAErrorAdmissionBlocked, "prepare repair", "a current passing containing smoke result is required", nil)
	}
	if flow.QA == nil || !flow.QA.Fresh || flow.QA.Assessment != AssessmentPassWithFindings && flow.QA.Assessment != AssessmentPass {
		return NewQAError(QAErrorAdmissionBlocked, "prepare repair", "a current acceptable evidence-producing QA attempt is required", nil)
	}
	return nil
}

func selectRepairIssue(adjudication QAAdjudication, issueID string) (QAIssue, QARootCauseGroup, error) {
	var issue QAIssue
	for _, candidate := range adjudication.Issues {
		if candidate.ID == issueID {
			issue = candidate
			break
		}
	}
	if issue.ID == "" || !issue.RepairEligible {
		return QAIssue{}, QARootCauseGroup{}, NewQAError(QAErrorAdmissionBlocked, "prepare repair", "issue is not current and repair eligible", nil)
	}
	var group QARootCauseGroup
	for _, candidate := range adjudication.Groups {
		if candidate.ID == issue.RootCauseGroupID {
			group = candidate
			break
		}
	}
	if group.ID == "" {
		return QAIssue{}, QARootCauseGroup{}, NewQAError(QAErrorInvalidState, "prepare repair", "issue root-cause group is unavailable", nil)
	}
	return issue, group, nil
}

func loadRepairEvidence(store QAStore, qaMap QAMap, adjudication QAAdjudication, issue QAIssue) ([]QAEvidenceRecord, []QAEvidencePlan, error) {
	accepted := make(map[string]bool, len(adjudication.AcceptedIDs))
	for _, id := range adjudication.AcceptedIDs {
		accepted[id] = true
	}
	records := make([]QAEvidenceRecord, 0, len(issue.EvidenceIDs))
	plans := make([]QAEvidencePlan, 0, len(issue.EvidenceIDs))
	seenPlans := map[string]bool{}
	for _, id := range normalizeQAStrings(issue.EvidenceIDs) {
		if !accepted[id] {
			return nil, nil, NewQAError(QAErrorAdmissionBlocked, "prepare repair", "issue references rejected evidence", nil)
		}
		record, err := store.LoadEvidence(qaMap.SemanticAttemptID, id)
		if err != nil {
			return nil, nil, err
		}
		if record.Outcome != QAEvidenceFail || !record.Contained || !record.Cleanup.Complete || record.TargetIdentityBefore != qaMap.ImplementationFingerprint || record.TargetIdentityAfter != qaMap.ImplementationFingerprint || !record.Repeatable && len(record.Commands) == 0 {
			return nil, nil, NewQAError(QAErrorAdmissionBlocked, "prepare repair", "issue evidence is not current, failing, contained, and reproducible", nil)
		}
		plan, err := store.LoadEvidencePlan(qaMap.SemanticAttemptID, record.PlanID, qaMap.Budgets)
		if err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(plan.Executable) == "" {
			return nil, nil, NewQAError(QAErrorAdmissionBlocked, "prepare repair", "issue has no exact executable reproducer", nil)
		}
		records = append(records, record)
		if !seenPlans[plan.ID] {
			plans = append(plans, plan)
			seenPlans[plan.ID] = true
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	sort.Slice(plans, func(i, j int) bool { return plans[i].ID < plans[j].ID })
	return records, plans, nil
}

func freezeRepairChecks(runID string, plans []QAEvidencePlan, evidence []QAEvidenceRecord, qaMap QAMap, flow FlowState, budgets RepairBudgets) ([]RepairCheckDescriptor, RepairCheckDescriptor, error) {
	if len(plans) == 0 || len(evidence) == 0 {
		return nil, RepairCheckDescriptor{}, NewQAError(QAErrorAdmissionBlocked, "prepare repair", "no frozen reproducer plans are available", nil)
	}
	plansByID := make(map[string]QAEvidencePlan, len(plans))
	for _, plan := range plans {
		plansByID[plan.ID] = plan
	}
	var exactPlan QAEvidencePlan
	for _, record := range evidence {
		if record.Outcome == QAEvidenceFail {
			exactPlan = plansByID[record.PlanID]
			break
		}
	}
	if exactPlan.ID == "" {
		return nil, RepairCheckDescriptor{}, NewQAError(QAErrorAdmissionBlocked, "prepare repair", "no exact failing reproducer is available", nil)
	}
	makePlanCheck := func(gate RepairGateKind, source string, plan QAEvidencePlan, expected string) (RepairCheckDescriptor, error) {
		id, err := NewRepairCheckID(runID, gate, source)
		if err != nil {
			return RepairCheckDescriptor{}, err
		}
		timeout := plan.Timeout
		if timeout > budgets.CommandTimeout {
			timeout = budgets.CommandTimeout
		}
		limit := plan.OutputLimit
		if limit > budgets.OutputBytes {
			limit = budgets.OutputBytes
		}
		return RepairCheckDescriptor{ID: id, Gate: gate, Executable: plan.Executable, Args: append([]string(nil), plan.Args...), EnvironmentNames: normalizeQAStrings(plan.EnvironmentNames), Timeout: timeout, OutputLimit: limit, Expected: expected, SourcePlanID: plan.ID}, nil
	}
	exact, err := makePlanCheck(RepairGateExactReproducer, exactPlan.ID, exactPlan, exactPlan.RefutationCondition)
	if err != nil {
		return nil, RepairCheckDescriptor{}, err
	}
	checks := []RepairCheckDescriptor{exact}
	for _, gate := range []RepairGateKind{RepairGatePrimaryShards, RepairGateLinkedTheories, RepairGateFollowUpShards, RepairGateContainingQA} {
		check, makeErr := makePlanCheck(gate, exactPlan.ID+"/"+string(gate), exactPlan, "all frozen "+string(gate)+" conditions pass")
		if makeErr != nil {
			return nil, RepairCheckDescriptor{}, makeErr
		}
		checks = append(checks, check)
	}
	for _, internal := range []struct {
		gate   RepairGateKind
		source string
		arg    string
		want   string
	}{
		{RepairGateContainingSmoke, repairSmokeFingerprint(flow.Smoke), "containing-smoke", "current containing smoke passes on the repaired target"},
	} {
		id, idErr := NewRepairCheckID(runID, internal.gate, internal.source)
		if idErr != nil {
			return nil, RepairCheckDescriptor{}, idErr
		}
		checks = append(checks, RepairCheckDescriptor{ID: id, Gate: internal.gate, Executable: "@product", Args: []string{internal.arg, internal.source, qaMap.SemanticAttemptID}, Timeout: budgets.CommandTimeout, OutputLimit: budgets.OutputBytes, Expected: internal.want})
	}
	if err := validateRepairCheckSequence(checks, budgets); err != nil {
		return nil, RepairCheckDescriptor{}, err
	}
	return checks, exact, nil
}

func repairAllowedPaths(issue QAIssue, evidence []QAEvidenceRecord) ([]string, error) {
	values := []string{repairIssuePath(issue.Location)}
	for _, record := range evidence {
		for _, path := range record.ChangedPaths {
			if ClassifyRepairPath(path) == RepairPathProduction {
				values = append(values, path)
			}
		}
	}
	paths, err := NormalizeRepairPaths(values)
	if err != nil || len(paths) == 0 {
		return nil, NewQAError(QAErrorAdmissionBlocked, "prepare repair", "issue has no unambiguous production path", err)
	}
	for _, path := range paths {
		if ClassifyRepairPath(path) != RepairPathProduction {
			return nil, NewQAError(QAErrorAdmissionBlocked, "prepare repair", "issue scope includes a protected path", nil)
		}
	}
	return paths, nil
}

func repairIssuePath(location string) string {
	value := strings.TrimSpace(location)
	if index := strings.LastIndex(value, ":"); index > 0 {
		if suffix := value[index+1:]; suffix != "" && strings.Trim(suffix, "0123456789") == "" {
			value = value[:index]
		}
	}
	return value
}

func repairForbiddenPaths() []string {
	return []string{".gitignore", ".gitattributes", ".gitmodules", "go.mod", "go.sum", "ultraplan.yml"}
}

func repairShardIDs(plans []QAEvidencePlan) []string {
	values := make([]string, 0, len(plans))
	for _, plan := range plans {
		values = append(values, plan.ShardID)
	}
	return normalizeQAStrings(values)
}

func repairTheoryIDs(plans []QAEvidencePlan) []string {
	var values []string
	for _, plan := range plans {
		values = append(values, plan.TheoryIDs...)
	}
	return normalizeQAStrings(values)
}

func repairExpectationRefs(plans []QAEvidencePlan) []string {
	var values []string
	for _, plan := range plans {
		values = append(values, plan.ExpectationRefs...)
	}
	return normalizeQAStrings(values)
}

func repairAcceptanceCriteria(plans []QAEvidencePlan) []string {
	var values []string
	for _, plan := range plans {
		values = append(values, plan.RefutationCondition)
	}
	return normalizeQAStrings(values)
}

func repairSmokeFingerprint(smoke *SmokeStageState) string {
	if smoke == nil {
		return ""
	}
	for _, value := range []string{smoke.SmokeFingerprint, smoke.InputFingerprint, smoke.ArtifactDigest} {
		if validFingerprint(value) {
			return value
		}
	}
	return ""
}

func (s Service) repairWriterFence(expected QAWriterToken) func(QAWriterToken) error {
	if s.qaWriterFence != nil {
		return s.qaWriterFence
	}
	return func(got QAWriterToken) error {
		if got != expected {
			return errors.New("writer token does not own this repair operation")
		}
		return nil
	}
}

func (s Service) validateAutomaticRepairProof(store QAStore, packet RepairIssuePacket) error {
	proof, err := store.LoadManualRepairProof()
	if err != nil {
		return NewQAError(QAErrorAdmissionBlocked, "confirm automatic repair", "a current qualifying manual repair proof is required", err)
	}
	if proof.Outcome != RepairOutcomeVerified && proof.Outcome != RepairOutcomeVerifiedWithFindings || !proof.CleanupComplete || !proof.ProductionApplied || !proof.CompleteLadder {
		return NewQAError(QAErrorAdmissionBlocked, "confirm automatic repair", "manual repair proof is not qualifying", nil)
	}
	if proof.Target.Fingerprint != packet.Target.Fingerprint || proof.ImplementationFingerprint != packet.ImplementationFingerprint || proof.PolicyFingerprint != packet.PolicyFingerprint || proof.IsolationFingerprint != packet.IsolationFingerprint || proof.GovernedInputFingerprint != packet.GovernedInputFingerprint {
		return NewQAError(QAErrorStaleInput, "confirm automatic repair", "manual repair proof fingerprint does not match the current packet", nil)
	}
	return nil
}

func (s Service) repairProposalRequest(packet RepairIssuePacket, workspace string) (pruntime.Request, error) {
	data, err := canonicalQAJSON(packet)
	if err != nil {
		return pruntime.Request{}, err
	}
	prompt := repairProposalPromptBody + string(data) + "\n"
	request := s.runtimeRequest(prompt, map[string]string{"project": packet.Project, "sprint": packet.Sprint, "stage": string(VerificationPhaseRepair), "repair_run": packet.RepairRunID, "cycle": "1"})
	request.Metadata["prompt_id"] = repairProposalPromptID
	request.Metadata["prompt_version"] = repairProposalPromptVersion
	request.Metadata["prompt_owner_kind"] = "sprint"
	request.Metadata["prompt_owner_id"] = packet.Project + "/" + packet.Sprint
	request.Metadata["prompt_purpose"] = "bounded_repair_proposal"
	request.Metadata["prompt_checksum"] = hashBytes([]byte(repairProposalPromptBody))
	settings, settingsErr := s.effectiveQASettings()
	if settingsErr != nil {
		return pruntime.Request{}, settingsErr
	}
	runtimeSettings := settings.Runtime
	if override, ok := s.verificationRuntime[VerificationPhaseRepair]; ok && strings.TrimSpace(override.Model) != "" {
		runtimeSettings = override
	}
	request.Provider, request.Model = splitProviderModel(runtimeSettings.Model)
	request.Metadata["variant"] = runtimeSettings.Variant
	request.Metadata["reasoning_effort"] = runtimeSettings.Variant
	request.WorkDir = filepath.Clean(workspace)
	request.Timeout = packet.Budgets.WallTime
	request.Sandbox = "workspace_write"
	request.Permissions = "restricted"
	request.RequireCaps = appendUnique(request.RequireCaps, "permissions")
	request.Policy = pruntime.PermissionPolicy{Default: "deny", Tools: map[string]string{"read": "allow", "list": "allow", "search": "allow", "glob": "allow", "write": "allow", "edit": "allow", "patch": "allow", "bash": "deny", "shell": "deny"}}
	for _, rel := range packet.AllowedPaths {
		path := filepath.Join(workspace, filepath.FromSlash(rel))
		if !inside(workspace, path) {
			return pruntime.Request{}, NewQAError(QAErrorPermissionDenied, "prepare repair runtime", "allowed path escapes isolated workspace", nil)
		}
		request.Policy.PathRules = append(request.Policy.PathRules, pruntime.PermissionPathRule{Path: path, Action: "allow"})
	}
	sort.Slice(request.Policy.PathRules, func(i, j int) bool { return request.Policy.PathRules[i].Path < request.Policy.PathRules[j].Path })
	return request, nil
}

func deriveRepairProposal(target, isolated string, changedPaths []string, packet RepairIssuePacket) ([]byte, map[string][]byte, map[string]string, int64, error) {
	paths, err := NormalizeRepairPaths(changedPaths)
	if err != nil || len(paths) == 0 || len(paths) > packet.Budgets.MaxFilesPerCycle {
		return nil, nil, nil, 0, fmt.Errorf("proposal path set is invalid or over budget")
	}
	if !repairPathsAllowed(paths, packet.AllowedPaths) {
		return nil, nil, nil, 0, fmt.Errorf("proposal changed a path outside confirmed scope")
	}
	var patch strings.Builder
	replacements := make(map[string][]byte, len(paths))
	preimages := make(map[string]string, len(paths))
	var changedBytes int64
	for _, rel := range paths {
		if ClassifyRepairPath(rel) != RepairPathProduction {
			return nil, nil, nil, 0, fmt.Errorf("proposal changed protected path %q", rel)
		}
		beforePath := filepath.Join(target, filepath.FromSlash(rel))
		afterPath := filepath.Join(isolated, filepath.FromSlash(rel))
		if err := ensureRepairRegularPath(target, beforePath); err != nil {
			return nil, nil, nil, 0, err
		}
		if err := ensureRepairRegularPath(isolated, afterPath); err != nil {
			return nil, nil, nil, 0, err
		}
		before, beforeErr := os.ReadFile(beforePath)
		after, afterErr := os.ReadFile(afterPath)
		if beforeErr != nil || afterErr != nil {
			return nil, nil, nil, 0, errors.Join(beforeErr, afterErr)
		}
		if bytes.IndexByte(before, 0) >= 0 || bytes.IndexByte(after, 0) >= 0 {
			return nil, nil, nil, 0, fmt.Errorf("binary proposal is prohibited")
		}
		changedBytes += int64(len(before) + len(after))
		if changedBytes > packet.Budgets.MaxBytesPerCycle {
			return nil, nil, nil, 0, fmt.Errorf("proposal changed bytes exceed confirmed limit")
		}
		preimages[rel] = hashBytes(before)
		replacements[rel] = append([]byte(nil), after...)
		writeWholeFilePatch(&patch, rel, before, after)
	}
	proposal := []byte(patch.String())
	if len(proposal) == 0 || len(proposal) > packet.Budgets.MaxPatchBytes {
		return nil, nil, nil, 0, fmt.Errorf("derived proposal patch exceeds confirmed limit")
	}
	return proposal, replacements, preimages, changedBytes, nil
}

func writeWholeFilePatch(out *strings.Builder, path string, before, after []byte) {
	beforeLines := splitRepairLines(before)
	afterLines := splitRepairLines(after)
	fmt.Fprintf(out, "--- a/%s\n+++ b/%s\n@@ -1,%d +1,%d @@\n", path, path, len(beforeLines), len(afterLines))
	for _, line := range beforeLines {
		out.WriteByte('-')
		out.WriteString(line)
		out.WriteByte('\n')
	}
	for _, line := range afterLines {
		out.WriteByte('+')
		out.WriteString(line)
		out.WriteByte('\n')
	}
}

func splitRepairLines(data []byte) []string {
	value := strings.ReplaceAll(string(data), "\r\n", "\n")
	value = strings.TrimSuffix(value, "\n")
	if value == "" {
		return nil
	}
	return strings.Split(value, "\n")
}

func (s Service) runRepairReverification(ctx context.Context, packet RepairIssuePacket, target string, flow FlowState, cycle int, progress func(RepairProgress)) (RepairReverification, bool, bool) {
	byGate := make(map[RepairGateKind]RepairCheckDescriptor, len(packet.Checks))
	for _, check := range packet.Checks {
		if _, exists := byGate[check.Gate]; !exists {
			byGate[check.Gate] = check
		}
	}
	results := make([]RepairGateResult, 0, len(RepairGateOrder()))
	stopped := false
	exactRemoved := false
	for _, gate := range RepairGateOrder() {
		if stopped {
			results = append(results, RepairGateResult{Gate: gate, Status: RepairGateSkipped, Reason: "a narrower required gate did not pass", NextAction: "Resolve the first non-pass and start a newly confirmed repair."})
			continue
		}
		check, ok := byGate[gate]
		if !ok {
			results = append(results, RepairGateResult{Gate: gate, Status: RepairGateBlocked, Reason: "frozen gate descriptor is unavailable", NextAction: "Produce a current QA packet with the complete ladder."})
			stopped = true
			continue
		}
		emitRepair(progress, RepairProgress{Phase: RepairPhaseReverifying, Cycle: cycle, Gate: gate, Message: "Running " + string(gate)})
		started := s.now().UTC()
		result := RepairGateResult{Gate: gate, Status: RepairGatePassed}
		if check.Executable == "@product" {
			switch gate {
			case RepairGateContainingSmoke:
				if flow.Smoke == nil || flow.Smoke.Stale || repairSmokeFingerprint(flow.Smoke) != packet.SmokeFingerprint {
					result.Status = RepairGateBlocked
					result.Reason = "containing smoke selection authority became stale"
					result.NextAction = "Prepare a new repair packet from current smoke authority."
				} else {
					smoke, smokeErr := s.runSmoke(ctx, packet.Project, packet.Sprint, SmokeRequest{ForceReview: true, OverrideConfirmed: true, OverrideRationale: "bounded repair reverification after the single conformance review", NonInteractive: true, RepairVerification: true})
					result.ExitCode = smoke.Counts.Failed + smoke.Counts.Errors
					result.OutputHash = repairSmokeResultFingerprint(smoke)
					if smokeErr != nil || smoke.Status != SmokeCompleted || smoke.Verdict != SmokePass && smoke.Verdict != SmokePassWithOpenIssues {
						result.Status = RepairGateFailed
						result.Reason = "repaired-target containing smoke did not pass"
						result.NextAction = "Inspect the retained smoke diagnostic and adjudicate the remaining failure."
						result.Diagnostic = boundRepairText(errorString(smokeErr), 512)
					} else {
						result.Reason = "repaired-target containing smoke passed"
					}
				}
			default:
				result.Status = RepairGateBlocked
				result.Reason = "unknown product-owned repair verifier"
				result.NextAction = "Prepare a new packet with an executable bounded verifier."
			}
		} else {
			identityBefore, identityErr := targetIdentity(target)
			if identityErr != nil {
				result.Status, result.Reason, result.NextAction = RepairGateBlocked, "target identity is unavailable", "Restore a stable target and recover the repair."
			} else {
				workdir := target
				if check.Workdir != "" {
					workdir = filepath.Join(target, filepath.FromSlash(check.Workdir))
				}
				environment := make(map[string]string)
				for _, name := range check.EnvironmentNames {
					if value, exists := os.LookupEnv(name); exists {
						environment[name] = value
					}
				}
				processResult, runErr := s.processRunner.Run(ctx, pprocess.Request{Executable: check.Executable, Args: append([]string(nil), check.Args...), Dir: workdir, Env: pprocess.SortedEnvironment(environment), Timeout: check.Timeout, StdoutLimit: check.OutputLimit, StderrLimit: check.OutputLimit, CleanupGrace: packet.Budgets.CleanupTimeout})
				result.ExitCode = processResult.ExitCode
				result.OutputBytes = len(processResult.Stdout) + len(processResult.Stderr)
				result.OutputHash = hashBytes([]byte(processResult.Stdout + "\x00" + processResult.Stderr))
				if runErr != nil || processResult.ExitCode != 0 {
					result.Status, result.Reason, result.NextAction = RepairGateFailed, "frozen check did not pass", "Inspect retained check evidence and adjudicate the remaining failure."
					if runErr != nil {
						result.Diagnostic = boundRepairText(runErr.Error(), 512)
					}
				}
				if processResult.Cancelled || processResult.TimedOut || processResult.StdoutTruncated || processResult.StderrTruncated || !processResult.CleanupComplete {
					result.Status, result.Reason, result.NextAction = RepairGateBlocked, "check execution or cleanup is incomplete", "Recover cleanup and rerun from a proven boundary."
				}
				identityAfter, afterErr := targetIdentity(target)
				if afterErr != nil || identityAfter != identityBefore {
					result.Status, result.Reason, result.NextAction = RepairGateBlocked, "target changed during reverification", "Adjudicate target drift before another mutation."
				}
			}
		}
		result.Duration = s.now().UTC().Sub(started)
		result.DurationMS = result.Duration.Milliseconds()
		results = append(results, result)
		if result.Status != RepairGatePassed {
			stopped = true
		} else if gate == RepairGateExactReproducer {
			exactRemoved = true
		}
	}
	return RepairReverification{SchemaVersion: QARepairSchemaVersion, RepairRunID: packet.RepairRunID, Cycle: cycle, Gates: results, IssueIDsBefore: []string{packet.Issue.ID}, IssueIDsAfter: unresolvedRepairIssues(packet, exactRemoved), HighestSeverityBefore: packet.Issue.Severity, HighestSeverityAfter: chooseSeverity(exactRemoved, "", packet.Issue.Severity), CompletedAt: s.now().UTC()}, exactRemoved, !stopped
}

func repairSmokeResultFingerprint(result SmokeResult) string {
	digest, err := repairDigest(struct {
		Status    SmokeExecutionStatus
		Verdict   SmokeVerdict
		RunID     string
		ScopeKind string
		Scope     string
		Counts    SmokeCounts
		Evidence  []SmokeEvidence
	}{result.Status, result.Verdict, result.RunID, result.ScopeKind, result.Scope, result.Counts, result.Evidence})
	if err != nil {
		return ""
	}
	return digest
}

func (s Service) finishBlockedRepair(store QAStore, state RepairState, flow FlowState, token QAWriterToken, release func(), released *bool, stop RepairStopReason, reason string, cause error) (RepairResult, error) {
	return s.finishRepairWithoutApply(store, state, flow, token, release, released, RepairOutcomeBlocked, stop, reason, cause)
}

func (s Service) finishEscalatedRepair(store QAStore, state RepairState, flow FlowState, token QAWriterToken, release func(), released *bool, stop RepairStopReason, reason string, cause error) (RepairResult, error) {
	return s.finishRepairWithoutApply(store, state, flow, token, release, released, RepairOutcomeEscalated, stop, reason, cause)
}

func (s Service) finishRepairWithoutApply(store QAStore, state RepairState, flow FlowState, token QAWriterToken, release func(), released *bool, outcome RepairOutcome, stop RepairStopReason, reason string, cause error) (RepairResult, error) {
	state.Phase = RepairPhaseTerminalizing
	state.StopReason = stop
	state.NextAction = "Release the owned mutation lease before terminal publication."
	state.UpdatedAt = s.now().UTC()
	if err := store.publishRepairState(state, flow, token); err != nil {
		return RepairResult{}, errors.Join(cause, err)
	}
	release()
	*released = true
	packet, packetErr := store.LoadRepairPacket(state.QAAttemptID, state.RepairRunID)
	if packetErr != nil {
		return RepairResult{}, errors.Join(cause, packetErr)
	}
	detail := strings.TrimSpace(reason)
	if cause != nil {
		detail += ": " + boundRepairText(cause.Error(), 512)
	}
	evidenceRefs, evidenceErr := repairResultEvidence(store, state, state.CurrentCycle)
	if evidenceErr != nil {
		return RepairResult{}, errors.Join(cause, evidenceErr)
	}
	result := RepairResult{SchemaVersion: QARepairSchemaVersion, Project: state.Project, Sprint: state.Sprint, QAAttemptID: state.QAAttemptID, RepairRunID: state.RepairRunID, Mode: state.Mode, Outcome: outcome, Reason: detail, StopReason: stop, Consumed: state.Consumed, Runtime: state.Runtime, Target: packet.Target, CleanupComplete: false, UnresolvedIssues: []string{packet.Issue.ID}, Evidence: evidenceRefs, NextAction: repairOutcomeNextAction(outcome), CompletedAt: s.now().UTC()}
	state.UpdatedAt = result.CompletedAt
	if err := store.PublishRepairResult(result, state, flow, token); err != nil {
		return RepairResult{}, errors.Join(cause, err)
	}
	return result, nil
}

func repairTargetIdentity(target string) (QATargetIdentity, error) {
	fingerprint, err := targetIdentity(target)
	if err != nil {
		return QATargetIdentity{}, err
	}
	head, index, worktree := qaGitIdentity(target)
	return QATargetIdentity{Fingerprint: fingerprint, GitHead: head, GitIndex: index, GitWorktree: worktree}, nil
}

func cleanupError(result pprocess.CleanupResult) error {
	if result.Complete {
		return nil
	}
	return errors.New(result.Error)
}

func joinRepairDiagnostics(values ...string) string {
	clean := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			clean = append(clean, value)
		}
	}
	return strings.Join(clean, "; ")
}

func sameRepairPaths(a, b []string) bool {
	left, leftErr := NormalizeRepairPaths(a)
	right, rightErr := NormalizeRepairPaths(b)
	if leftErr != nil || rightErr != nil || len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func repairPathsAllowed(paths, allowed []string) bool {
	normalized, err := NormalizeRepairPaths(paths)
	if err != nil {
		return false
	}
	allowedSet, err := NormalizeRepairPaths(allowed)
	if err != nil {
		return false
	}
	for _, path := range normalized {
		if !repairPathSetContains(allowedSet, path) || ClassifyRepairPath(path) != RepairPathProduction {
			return false
		}
	}
	return true
}

func mapKeys(values map[string][]byte) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func emitRepair(progress func(RepairProgress), value RepairProgress) {
	if progress != nil {
		value.Message = boundRepairText(value.Message, 256)
		progress(value)
	}
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func chooseSeverity(condition bool, yes, no string) string {
	if condition {
		return yes
	}
	return no
}

func unresolvedRepairIssues(packet RepairIssuePacket, exactRemoved bool) []string {
	if exactRemoved {
		return nil
	}
	return []string{packet.Issue.ID}
}

func repairOutcomeReason(outcome RepairOutcome) string {
	switch outcome {
	case RepairOutcomeVerified:
		return "the exact issue and every frozen progressive gate passed with proven cleanup"
	case RepairOutcomeVerifiedWithFindings:
		return "the exact issue and required gates passed; current non-blocking findings remain"
	case RepairOutcomeFailed:
		return "deterministic reverification shows the issue or a required check still fails"
	case RepairOutcomeBlocked:
		return "repair could not reach a complete semantic conclusion"
	case RepairOutcomeEscalated:
		return "repair encountered unsafe or uncertain authority that requires adjudication"
	case RepairOutcomeStalled:
		return "automatic repair reached a persisted bound without enough progress"
	default:
		return "repair ended without a recognized outcome"
	}
}

func repairOutcomeNextAction(outcome RepairOutcome) string {
	switch outcome {
	case RepairOutcomeVerified, RepairOutcomeVerifiedWithFindings:
		return "Inspect the retained result and repaired-target smoke evidence."
	case RepairOutcomeFailed:
		return "Adjudicate the remaining failure before preparing another packet."
	case RepairOutcomeBlocked:
		return "Restore the named prerequisite and resume only from the latest proven boundary."
	case RepairOutcomeEscalated:
		return "Inspect scope, drift, cleanup, and apply evidence before any further mutation."
	case RepairOutcomeStalled:
		return "Review consumed limits and progress facts; automatic mutation cannot continue."
	default:
		return "Inspect repair state."
	}
}

func repairResultEvidence(store QAStore, state RepairState, cycle int) ([]QAArtifactRef, error) {
	values := make([]QAArtifactRef, 0, 9)
	for _, ref := range []*QAArtifactRef{state.Packet, state.Confirmation} {
		if ref != nil {
			values = append(values, *ref)
		}
	}
	if cycle > 0 {
		base := QARepairCycleRelPath(store.sprint, state.QAAttemptID, state.RepairRunID, cycle)
		for _, name := range []string{"proposal.patch", "scope.json", "apply-journal.json", "reverification.json", "cleanup.json", "cycle.json"} {
			rel := filepath.ToSlash(filepath.Join(base, name))
			path, err := store.resolve(rel)
			if err != nil {
				return nil, err
			}
			info, err := os.Lstat(path)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
				return nil, NewQAError(QAErrorInvalidState, "build repair result evidence", "repair evidence is not a regular file", nil)
			}
			digest, err := hashFile(path)
			if err != nil {
				return nil, err
			}
			values = append(values, QAArtifactRef{Path: rel, Digest: digest})
		}
	}
	return values, nil
}

func boundRepairText(value string, limit int) string {
	lower := strings.ToLower(value)
	for _, marker := range []string{"bearer ", "token=", "password=", "api_key", "apikey", "sk-", "ghp_", "github_pat_", "xoxb-", "/home/", "/tmp/"} {
		if strings.Contains(lower, marker) {
			return "[redacted repair diagnostic]"
		}
	}
	value = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\x00' || r < 0x20 && r != '\t' {
			return ' '
		}
		return r
	}, value)
	value = strings.TrimSpace(value)
	if len(value) > limit {
		return value[:limit]
	}
	return value
}

func validateRepairCheckSequence(checks []RepairCheckDescriptor, budgets RepairBudgets) error {
	if len(checks) == 0 || len(checks) > budgets.CommandCount {
		return fmt.Errorf("repair packet check collection is empty or over budget")
	}
	order := RepairGateOrder()
	last := -1
	seen := make([]bool, len(order))
	for _, check := range checks {
		if err := ValidateRepairCheck(check, budgets); err != nil {
			return err
		}
		index := -1
		for i, gate := range order {
			if gate == check.Gate {
				index = i
				break
			}
		}
		if index < last {
			return fmt.Errorf("repair checks are not in fixed gate order")
		}
		last = index
		seen[index] = true
	}
	for i, present := range seen {
		if !present {
			return fmt.Errorf("repair checks omit required gate %s", order[i])
		}
	}
	if checks[len(checks)-1].Gate != RepairGateContainingSmoke {
		return fmt.Errorf("repair checks must cover exact reproducer through containing smoke")
	}
	return nil
}

func normalizeRepairPath(value string) (string, error) {
	value = filepath.ToSlash(strings.TrimSpace(value))
	if value == "" || filepath.IsAbs(filepath.FromSlash(value)) || strings.ContainsAny(value, "\x00\r\n") {
		return "", fmt.Errorf("unsafe repair path %q", value)
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("unsafe repair path %q", value)
	}
	return clean, nil
}

func repairPathSetContains(values []string, path string) bool {
	index := sort.SearchStrings(values, path)
	return index < len(values) && values[index] == path
}

func validRepairID(value, kind string) bool {
	return repairIDPattern.MatchString(value) && strings.HasPrefix(value, QARepairIDScope+"-"+kind+"-")
}

func validRepairMode(value RepairMode) bool {
	return value == RepairModeManual || value == RepairModeAutomatic
}

func validRepairGate(value RepairGateKind) bool {
	for _, gate := range RepairGateOrder() {
		if value == gate {
			return true
		}
	}
	return false
}

func validRepairPhase(value RepairPhase) bool {
	switch value {
	case RepairPhasePrepared, RepairPhaseConfirmed, RepairPhaseProposing, RepairPhaseApplying, RepairPhaseReverifying, RepairPhaseCleaning, RepairPhaseTerminalizing, RepairPhaseTerminal, RepairPhaseInterrupted, RepairPhaseStale:
		return true
	default:
		return false
	}
}

func validRepairOutcome(value RepairOutcome) bool {
	switch value {
	case RepairOutcomeVerified, RepairOutcomeVerifiedWithFindings, RepairOutcomeFailed, RepairOutcomeBlocked, RepairOutcomeEscalated, RepairOutcomeStalled:
		return true
	default:
		return false
	}
}

func validRepairTarget(value QATargetIdentity) bool {
	return validFingerprint(value.Fingerprint) && strings.TrimSpace(value.GitWorktree) != ""
}

func repairDigest(value any) (string, error) {
	data, err := canonicalQAJSON(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func repairSeverityRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	case "":
		return 0
	default:
		return 5
	}
}

// applyRepairFiles is the only production byte mutation helper. Callers must
// hold both the verification mutation lease and the durable writer fence.
func applyRepairFiles(root string, replacements map[string][]byte, expected map[string]string, maxFiles int, maxBytes int64) ([]RepairApplyOperation, int64, error) {
	return applyRepairFilesJournaled(root, replacements, expected, maxFiles, maxBytes, nil)
}

func applyRepairFilesJournaled(root string, replacements map[string][]byte, expected map[string]string, maxFiles int, maxBytes int64, progress func([]RepairApplyOperation) error) ([]RepairApplyOperation, int64, error) {
	if len(replacements) == 0 || len(replacements) > maxFiles {
		return nil, 0, fmt.Errorf("repair replacement count is outside the confirmed limit")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, 0, err
	}
	paths := make([]string, 0, len(replacements))
	for path := range replacements {
		normalized, pathErr := normalizeRepairPath(path)
		if pathErr != nil || ClassifyRepairPath(normalized) != RepairPathProduction {
			return nil, 0, fmt.Errorf("repair replacement path is not mutable production: %q", path)
		}
		paths = append(paths, normalized)
	}
	sort.Strings(paths)
	operations := make([]RepairApplyOperation, 0, len(paths))
	preimages := make(map[string][]byte, len(paths))
	var changedBytes int64
	for _, rel := range paths {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := ensureRepairRegularPath(root, path); err != nil {
			return nil, 0, err
		}
		before, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, 0, fmt.Errorf("read repair preimage %s: %w", rel, readErr)
		}
		preDigest := hashBytes(before)
		if wanted := expected[rel]; !validFingerprint(wanted) || wanted != preDigest {
			return nil, 0, fmt.Errorf("repair preimage changed for %s", rel)
		}
		post := replacements[rel]
		if bytes.IndexByte(post, 0) >= 0 {
			return nil, 0, fmt.Errorf("binary repair content is prohibited")
		}
		changedBytes += int64(len(before) + len(post))
		if changedBytes > maxBytes {
			return nil, 0, fmt.Errorf("repair changed bytes exceed the confirmed limit")
		}
		preimages[rel] = before
		operations = append(operations, RepairApplyOperation{Path: rel, PreimageDigest: preDigest, PostimageDigest: hashBytes(post)})
	}
	for i := range operations {
		op := &operations[i]
		path := filepath.Join(root, filepath.FromSlash(op.Path))
		if err := privateAtomicWrite(path, replacements[op.Path], "repair-apply", QAStateHooks{}); err != nil {
			for j := i - 1; j >= 0; j-- {
				prior := &operations[j]
				if restoreErr := privateAtomicWrite(filepath.Join(root, filepath.FromSlash(prior.Path)), preimages[prior.Path], "repair-compensate", QAStateHooks{}); restoreErr == nil {
					prior.Restored = true
				} else {
					err = errors.Join(err, fmt.Errorf("restore %s: %w", prior.Path, restoreErr))
				}
			}
			return operations, changedBytes, err
		}
		op.Applied = true
		if progress != nil {
			if progressErr := progress(append([]RepairApplyOperation(nil), operations...)); progressErr != nil {
				for j := i; j >= 0; j-- {
					prior := &operations[j]
					if !prior.Applied {
						continue
					}
					if restoreErr := privateAtomicWrite(filepath.Join(root, filepath.FromSlash(prior.Path)), preimages[prior.Path], "repair-compensate", QAStateHooks{}); restoreErr == nil {
						prior.Restored = true
					} else {
						progressErr = errors.Join(progressErr, fmt.Errorf("restore %s: %w", prior.Path, restoreErr))
					}
				}
				return operations, changedBytes, progressErr
			}
		}
	}
	return operations, changedBytes, nil
}

func mergeRepairApplyOperations(current, staged []RepairApplyOperation) []RepairApplyOperation {
	preimagePaths := make(map[string]string, len(staged))
	for _, operation := range staged {
		preimagePaths[operation.Path] = operation.PreimagePath
	}
	out := append([]RepairApplyOperation(nil), current...)
	for i := range out {
		out[i].PreimagePath = preimagePaths[out[i].Path]
	}
	return out
}

func repairApplyCompensated(operations []RepairApplyOperation) bool {
	for _, operation := range operations {
		if operation.Applied && !operation.Restored {
			return false
		}
	}
	return true
}

func ensureRepairRegularPath(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("repair path escapes target")
	}
	current := root
	for _, component := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, statErr := os.Lstat(current)
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() && current == path {
			return fmt.Errorf("repair path is linked or not regular: %s", rel)
		}
		if current != path && !info.IsDir() {
			return fmt.Errorf("repair path parent is not a directory: %s", rel)
		}
		if info.Mode().IsRegular() {
			links, linkErr := linkCountForRepair(info)
			if linkErr != nil {
				return fmt.Errorf("repair link count is unavailable for %s: %w", rel, linkErr)
			}
			if links > 1 {
				return fmt.Errorf("repair hard-linked file is prohibited: %s", rel)
			}
		}
	}
	return nil
}

func linkCountForRepair(info fs.FileInfo) (uint64, error) {
	value := reflect.ValueOf(info.Sys())
	if value.IsValid() && value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if value.IsValid() && value.Kind() == reflect.Struct {
		field := value.FieldByName("Nlink")
		if field.IsValid() && field.CanUint() {
			return field.Uint(), nil
		}
	}
	return 0, errors.New("platform file metadata does not expose a link count")
}

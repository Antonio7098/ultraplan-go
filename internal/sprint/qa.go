package sprint

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	pprocess "github.com/Antonio7098/ultraplan-go/internal/platform/process"
	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

type QARunRequest struct {
	Resume            bool
	FocusShard        string
	Suite             string
	EvidenceProducing bool
	WriterToken       QAWriterToken
	Progress          func(QAProgress)
}

type QAProgress struct {
	Phase      QAPhaseStatus `json:"phase"`
	ShardID    string        `json:"shard_id,omitempty"`
	ShardKind  QAShardKind   `json:"shard_kind,omitempty"`
	ShardPhase QAPhaseStatus `json:"shard_phase,omitempty"`
	Event      string        `json:"event"`
	Completed  int           `json:"completed"`
	Total      int           `json:"total"`
	Message    string        `json:"message"`
}

type QARunResult struct {
	Project   string       `json:"project"`
	Sprint    string       `json:"sprint"`
	State     QAState      `json:"state"`
	Map       QAMap        `json:"map"`
	Shards    []QAShard    `json:"shards"`
	Synthesis QASynthesis  `json:"synthesis"`
	Smoke     *SmokeResult `json:"smoke,omitempty"`
}

type QASnapshot struct {
	State     QAState      `json:"state"`
	Map       *QAMap       `json:"map,omitempty"`
	Shards    []QAShard    `json:"shards,omitempty"`
	Synthesis *QASynthesis `json:"synthesis,omitempty"`
}

// QAStatus reads the authoritative QA pointer and its referenced records. It
// never constructs a runtime or repairs state as a side effect.
func (s Service) QAStatus(projectRef, sprintRef string) (QASnapshot, error) {
	sp, err := s.resolveMutationSprint(projectRef, sprintRef)
	if err != nil {
		return QASnapshot{}, err
	}
	store := NewQAStore(s.root, sp)
	state, err := store.LoadState()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return QASnapshot{State: QAState{SchemaVersion: QASchemaVersion, Project: sp.Project, Sprint: sp.Slug, Phase: QAPhaseMissing, Freshness: QAFreshness{Current: false, Reasons: []string{"no QA attempt has been persisted"}}, NextAction: "Run qa --dry-run to inspect the current deterministic map."}}, nil
		}
		return QASnapshot{}, err
	}
	snapshot := QASnapshot{State: state}
	if state.Map != nil {
		qaMap, loadErr := store.LoadMap(state.CurrentAttemptID)
		if loadErr != nil {
			return QASnapshot{}, loadErr
		}
		snapshot.Map = &qaMap
		for _, planned := range qaMap.Shards {
			loaded, shardErr := store.LoadShard(state.CurrentAttemptID, planned.ID)
			if shardErr != nil {
				return QASnapshot{}, shardErr
			}
			snapshot.Shards = append(snapshot.Shards, loaded)
		}
	}
	if state.Synthesis != nil {
		budgets := MaximumQABudgets()
		if snapshot.Map != nil {
			budgets = snapshot.Map.Budgets
		}
		synthesis, loadErr := store.LoadSynthesis(state.CurrentAttemptID, budgets)
		if loadErr != nil {
			return QASnapshot{}, loadErr
		}
		snapshot.Synthesis = &synthesis
		for _, follow := range synthesis.FollowUpShards {
			loaded, shardErr := store.LoadShard(state.CurrentAttemptID, follow.ID)
			if shardErr == nil {
				snapshot.Shards = append(snapshot.Shards, loaded)
			}
		}
	}
	sort.Slice(snapshot.Shards, func(i, j int) bool { return snapshot.Shards[i].ID < snapshot.Shards[j].ID })
	return snapshot, nil
}

func (s Service) QAShard(projectRef, sprintRef, shardID string) (QAShard, error) {
	snapshot, err := s.QAStatus(projectRef, sprintRef)
	if err != nil {
		return QAShard{}, err
	}
	for _, shard := range snapshot.Shards {
		if shard.ID == shardID {
			return shard, nil
		}
	}
	return QAShard{}, NewQAError(QAErrorInvalidState, "read shard", "shard is not owned by the current QA attempt", nil)
}

func (s Service) QATheory(projectRef, sprintRef, theoryID string) (QATheory, error) {
	snapshot, err := s.QAStatus(projectRef, sprintRef)
	if err != nil {
		return QATheory{}, err
	}
	for _, shard := range snapshot.Shards {
		for _, theory := range shard.Theories {
			if theory.ID == theoryID {
				return theory, nil
			}
		}
	}
	return QATheory{}, NewQAError(QAErrorInvalidState, "read theory", "theory is not owned by the current QA attempt", nil)
}

func (s Service) QAEvidence(projectRef, sprintRef, evidenceID string) (QAEvidenceRecord, error) {
	sp, _, _, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return QAEvidenceRecord{}, err
	}
	state, err := NewQAStore(s.root, sp).LoadState()
	if err != nil {
		return QAEvidenceRecord{}, err
	}
	return NewQAStore(s.root, sp).LoadEvidence(state.CurrentAttemptID, evidenceID)
}

func (s Service) QAAdjudication(projectRef, sprintRef string) (QAAdjudication, error) {
	sp, _, _, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return QAAdjudication{}, err
	}
	store := NewQAStore(s.root, sp)
	state, err := store.LoadState()
	if err != nil {
		return QAAdjudication{}, err
	}
	return store.LoadAdjudication(state.CurrentAttemptID, MaximumQABudgets())
}

func (s Service) QAAssessment(projectRef, sprintRef string) (QAAssessmentRecord, error) {
	sp, _, _, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return QAAssessmentRecord{}, err
	}
	store := NewQAStore(s.root, sp)
	state, err := store.LoadState()
	if err != nil {
		return QAAssessmentRecord{}, err
	}
	return store.LoadAssessment(state.CurrentAttemptID)
}

// RecoverQA reconciles an abandoned or stale QA pointer without creating a
// runtime or adopting any prior worker or session.
func (s Service) RecoverQA(ctx context.Context, projectRef, sprintRef string) (QASnapshot, error) {
	lockedCtx, release, err := s.acquireMutationContext(ctx, projectRef, sprintRef)
	if err != nil {
		return QASnapshot{}, err
	}
	defer release()
	_ = lockedCtx
	sp, err := s.resolveMutationSprint(projectRef, sprintRef)
	if err != nil {
		return QASnapshot{}, err
	}
	store := NewQAStore(s.root, sp)
	state, err := store.LoadState()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return s.QAStatus(projectRef, sprintRef)
		}
		return QASnapshot{}, err
	}
	flow, err := LoadFlowState(s.root, sp)
	if err != nil {
		return QASnapshot{}, err
	}
	now := s.now().UTC()
	changed := false
	statePath, pathErr := store.StatePath()
	if pathErr != nil {
		return QASnapshot{}, pathErr
	}
	stateDigest, digestErr := hashFile(statePath)
	if digestErr != nil {
		return QASnapshot{}, NewQAError(QAErrorPersistenceFailure, "recover", "cannot fingerprint QA state", digestErr)
	}
	expectedSummary := qaFlowSummary(state, stateDigest, sp)
	if flow.QA == nil || *flow.QA != *expectedSummary {
		changed = true
	}
	changed = reconcileInterruptedQAState(&state) || changed
	if current, mapErr := s.QAMap(projectRef, sprintRef); mapErr != nil || current.Map.SemanticAttemptID != state.CurrentAttemptID {
		changed = true
		state.Phase = QAPhaseStale
		state.Freshness.Current = false
		state.Freshness.Reasons = []string{"governed QA inputs no longer match the retained semantic attempt"}
		state.NextAction = "Run qa --dry-run, then start a new QA attempt from the current map."
	}
	if !changed {
		return s.QAStatus(projectRef, sprintRef)
	}
	state.UpdatedAt = now
	if err := store.SaveRecoveredState(state, flow); err != nil {
		return QASnapshot{}, err
	}
	settings, settingsErr := s.effectiveQASettings()
	if settingsErr == nil {
		if err := store.PruneAttempts(state.CurrentAttemptID, settings.Budgets.RetainedAttempts); err != nil {
			return QASnapshot{}, err
		}
	}
	return s.QAStatus(projectRef, sprintRef)
}

func reconcileInterruptedQAState(state *QAState) bool {
	switch state.Phase {
	case QAPhaseMapped:
		state.Phase = QAPhaseInterrupted
		state.NextAction = "Run qa resume to claim and execute the mapped shards."
		return true
	case QAPhaseQueued, QAPhaseRunning, QAPhaseSynthesizing:
		state.Phase = QAPhaseInterrupted
		state.Run.Lifecycle = QARunTerminal
		state.Run.TerminalResult = QATerminalInterrupted
		state.Blocker = &QABlocker{Category: QAErrorConflict, Scope: "attempt", Summary: "the prior QA owner stopped before recording a terminal result", NextAction: "Run qa resume to continue current valid shards with a new owner."}
		state.NextAction = state.Blocker.NextAction
		return true
	default:
		return false
	}
}

type qaInvestigatorOutput struct {
	SchemaVersion int                    `json:"schema_version"`
	Theories      []qaInvestigatorTheory `json:"theories"`
	Evidence      []QAEvidenceSummary    `json:"evidence"`
	Context       []QAContextRequest     `json:"context_requests"`
	Checks        []QAApprovedCheckRef   `json:"check_requests"`
}

type qaInvestigatorTheory struct {
	Claim                 string          `json:"claim"`
	Basis                 string          `json:"basis"`
	VerificationSurface   string          `json:"verification_surface"`
	ExpectationRefs       []string        `json:"expectation_refs"`
	SeverityIfConfirmed   string          `json:"severity_if_confirmed"`
	ConfirmationCondition string          `json:"confirmation_condition"`
	RefutationCondition   string          `json:"refutation_condition"`
	InconclusiveCondition string          `json:"inconclusive_condition"`
	SafeEvidenceStrategy  string          `json:"safe_evidence_strategy"`
	Outcome               QATheoryOutcome `json:"outcome"`
	OutcomeReason         string          `json:"outcome_reason"`
}

type qaShardResult struct {
	shard QAShard
	err   error
}

// RunQA owns one bounded read-only investigation attempt. Mapping stays pure;
// this method starts persistence and runtimes only after a valid writer token
// and the sprint mutation lease have both been acquired.
func (s Service) RunQA(ctx context.Context, projectRef, sprintRef string, req QARunRequest) (QARunResult, error) {
	if req.Suite != "" {
		if req.Suite != "smoke" {
			return QARunResult{}, NewQAError(QAErrorInvalidState, "run", "unsupported QA suite", nil)
		}
		if req.Resume || req.FocusShard != "" {
			return QARunResult{}, NewQAError(QAErrorInvalidState, "run", "the smoke QA suite cannot resume or focus a shard", nil)
		}
		if err := req.WriterToken.Validate(); err != nil {
			return QARunResult{}, NewQAError(QAErrorConflict, "run", err.Error(), err)
		}
		smoke, smokeErr := s.RunSmoke(ctx, projectRef, sprintRef, SmokeRequest{Progress: func(progress SmokeProgress) {
			emitQA(req.Progress, QAProgress{Phase: QAPhaseRunning, Event: "smoke_" + string(progress.Phase), Message: progress.Message})
		}})
		phase, terminal := QAPhaseCompleted, QATerminalCompleted
		next := smoke.NextAction
		if smokeErr != nil || smoke.Status != SmokeCompleted || smoke.Verdict == SmokeFailVerdict || smoke.Verdict == SmokeBlockedVerdict {
			phase, terminal = QAPhaseBlocked, QATerminalBlocked
		}
		state := QAState{SchemaVersion: QASchemaVersion, Project: projectRef, Sprint: sprintRef, Phase: phase, Freshness: QAFreshness{Current: smokeErr == nil && !smoke.Stale}, Run: qaRunCorrelation(req.WriterToken, QARunTerminal), NextAction: next, UpdatedAt: s.now().UTC()}
		state.Run.TerminalResult = terminal
		return QARunResult{Project: projectRef, Sprint: sprintRef, State: state, Smoke: &smoke}, smokeErr
	}
	if s.runtime == nil {
		return QARunResult{}, NewQAError(QAErrorRuntimeUnavailable, "run", "a QA runtime is required", nil)
	}
	if err := req.WriterToken.Validate(); err != nil {
		return QARunResult{}, NewQAError(QAErrorConflict, "run", err.Error(), err)
	}
	settings, err := s.effectiveQASettings()
	if err != nil {
		return QARunResult{}, NewQAError(QAErrorInvalidState, "run", "effective QA settings are invalid", err)
	}
	req.Progress = boundedQAProgress(req.Progress, settings.Budgets.RecentProgress)
	lockedCtx, release, err := s.acquireMutationContext(ctx, projectRef, sprintRef)
	if err != nil {
		return QARunResult{}, err
	}
	defer release()
	sp, err := s.resolveMutationSprint(projectRef, sprintRef)
	if err != nil {
		return QARunResult{}, err
	}
	fence := s.qaWriterFence
	if fence == nil {
		expected := req.WriterToken
		fence = func(got QAWriterToken) error {
			if got != expected {
				return errors.New("writer token does not own this QA invocation")
			}
			return nil
		}
	}
	store := NewQAStore(s.root, sp).WithWriterFence(fence)
	if err := store.PruneAttempts("", settings.Budgets.RetainedAttempts); err != nil {
		return QARunResult{}, err
	}
	if used, sizeErr := store.VerificationBytes(); sizeErr != nil {
		return QARunResult{}, sizeErr
	} else if used > int64(settings.Budgets.StateBytes) {
		return QARunResult{}, NewQAError(QAErrorBudgetExhausted, "run", "retained QA state exceeds the configured state budget", nil)
	}
	mapResult, err := s.QAMap(projectRef, sprintRef)
	if err != nil {
		return QARunResult{}, err
	}
	manifest, findings, err := s.PrepareReview(projectRef, sprintRef, ReviewRequest{})
	if err != nil || len(findings) > 0 {
		return QARunResult{}, NewQAError(QAErrorStaleInput, "run", "cannot resolve the current governed target", err)
	}
	mapResult.Map, err = s.refineQAMapSemantically(lockedCtx, mapResult.Map, manifest.Target)
	if err != nil {
		return QARunResult{}, err
	}
	if err := s.prepareQAShardPacks(&mapResult.Map); err != nil {
		return QARunResult{}, err
	}
	flow, err := LoadFlowState(s.root, sp)
	if err != nil {
		return QARunResult{}, NewQAError(QAErrorInvalidState, "run", "flow state is unavailable", err)
	}
	// Evidence-producing runs have prerequisites that are knowable before any
	// investigator is started. Reject them here so stale review/smoke state does
	// not consume model time and tokens only to fail during publication.
	if req.EvidenceProducing {
		if _, _, err := s.validateQAEvidenceAdmission(sp, mapResult.Map, manifest.Target); err != nil {
			return QARunResult{}, err
		}
	}
	state, shards, err := s.prepareQAAttempt(store, flow, mapResult.Map, req, settings)
	if err != nil {
		return QARunResult{}, err
	}
	emitQA(req.Progress, QAProgress{Phase: QAPhaseQueued, Event: "shards_queued", Completed: state.CompletedShards, Total: state.TotalShards, Message: "QA shards queued"})

	runCtx, cancel := context.WithTimeout(lockedCtx, settings.Budgets.RunTimeout)
	defer cancel()
	state.Phase = QAPhaseRunning
	state.Run.Lifecycle = QARunActive
	state.NextAction = "Wait for the bounded read-only investigators."
	state.UpdatedAt = s.now().UTC()
	if err := store.Publish(QAPublication{State: state, Flow: flow}, req.WriterToken); err != nil {
		return QARunResult{}, err
	}
	emitQA(req.Progress, QAProgress{Phase: QAPhaseRunning, Event: "investigation_started", Completed: state.CompletedShards, Total: state.TotalShards, Message: "QA investigation started"})
	shards, state, runErr := s.runQAShardBatch(runCtx, store, flow, mapResult.Map, manifest.Target, shards, state, req)
	if runErr != nil {
		state = terminalQAState(state, runErr, s.now().UTC())
		if publishErr := store.Publish(QAPublication{State: state, Flow: flow}, req.WriterToken); publishErr != nil {
			return QARunResult{}, errors.Join(runErr, publishErr)
		}
		return QARunResult{Project: sp.Project, Sprint: sp.Slug, State: state, Map: mapResult.Map, Shards: shards}, runErr
	}

	state.Phase = QAPhaseSynthesizing
	state.NextAction = "Synthesize the retained shard outcomes."
	state.UpdatedAt = s.now().UTC()
	if err := store.Publish(QAPublication{State: state, Flow: flow}, req.WriterToken); err != nil {
		return s.publishTerminalQAFailure(store, flow, mapResult.Map, shards, state, req.WriterToken, err)
	}
	emitQA(req.Progress, QAProgress{Phase: QAPhaseSynthesizing, Event: "synthesis_started", Completed: state.CompletedShards, Total: state.TotalShards, Message: "QA synthesis started"})
	var synthesis QASynthesis
	var decisionShards []QAShard
	for {
		arbitration, arbitrationErr := s.arbitrateQA(runCtx, mapResult.Map, shards, manifest.Target)
		if arbitrationErr != nil {
			return s.publishTerminalQAFailure(store, flow, mapResult.Map, shards, state, req.WriterToken, arbitrationErr)
		}
		decisionShards = applyQAArbitration(shards, arbitration)
		synthesis, err = SynthesizeQA(mapResult.Map, decisionShards)
		if err != nil {
			return s.publishTerminalQAFailure(store, flow, mapResult.Map, shards, state, req.WriterToken, err)
		}
		synthesis.Arbitration = &arbitration
		if err := ValidateQASynthesis(synthesis, mapResult.Map.Budgets); err != nil {
			return s.publishTerminalQAFailureWithSynthesis(store, flow, mapResult.Map, shards, synthesis, state, req.WriterToken, err)
		}
		follow := pendingQASynthesisFollowUps(synthesis, shards, mapResult.Map.Budgets.FollowUpShards)
		if len(follow) == 0 {
			break
		}
		state.TotalShards += len(follow)
		shards = append(shards, follow...)
		shards, state, runErr = s.runQAShardBatch(runCtx, store, flow, mapResult.Map, manifest.Target, shards, state, req)
		if runErr != nil {
			state = terminalQAState(state, runErr, s.now().UTC())
			if publishErr := store.Publish(QAPublication{State: state, Flow: flow}, req.WriterToken); publishErr != nil {
				return QARunResult{}, errors.Join(runErr, publishErr)
			}
			return QARunResult{Project: sp.Project, Sprint: sp.Slug, State: state, Map: mapResult.Map, Shards: shards}, runErr
		}
	}
	if err := finalizeQASynthesisFollowUps(&synthesis, mapResult.Map, shards); err != nil {
		return s.publishTerminalQAFailureWithSynthesis(store, flow, mapResult.Map, shards, synthesis, state, req.WriterToken, err)
	}
	state.OutcomeCounts = cloneQAOutcomeCounts(synthesis.OutcomeCounts)
	state.Phase = QAPhaseCompleted
	state.Run.Lifecycle = QARunTerminal
	state.Run.TerminalResult = QATerminalCompleted
	state.NextAction = synthesis.NextAction
	state.UpdatedAt = s.now().UTC()
	var evidencePublication *QAEvidencePublication
	if req.EvidenceProducing {
		if countCompletedQAShards(shards) == 0 {
			// There is no evidence work to perform. Preserve the synthesis result and
			// its actionable shard blockers instead of letting target materialization
			// replace them with a secondary isolation error.
			state.Phase = QAPhaseBlocked
			state.Run.TerminalResult = QATerminalBlocked
			state.NextAction = synthesis.NextAction
			state.Blocker = &QABlocker{Category: QAErrorInvalidState, Scope: "synthesis", Summary: "no QA shard produced evidence-ready theories", NextAction: synthesis.NextAction}
		} else {
			var reconciledIssues []QAArbiterIssue
			if synthesis.Arbitration != nil {
				reconciledIssues = synthesis.Arbitration.Issues
			}
			bundle, assessment, evidenceErr := s.buildQAEvidencePublication(runCtx, sp, mapResult.Map, manifest.Target, decisionShards, reconciledIssues, req.Progress)
			if evidenceErr != nil {
				return s.publishTerminalQAFailureWithSynthesis(store, flow, mapResult.Map, shards, synthesis, state, req.WriterToken, evidenceErr)
			}
			evidencePublication = &bundle
			state.NextAction = assessment.NextAction
			state.CanonicalAssessment = assessment.Assessment
			if assessment.Assessment == AssessmentFail || assessment.Assessment == AssessmentBlocked || assessment.Assessment == AssessmentIncomplete {
				state.Phase = QAPhaseBlocked
				state.Run.TerminalResult = QATerminalBlocked
			}
		}
	}
	if err := store.Publish(QAPublication{Shards: shards, Synthesis: &synthesis, State: state, Flow: flow, Evidence: evidencePublication}, req.WriterToken); err != nil {
		return QARunResult{}, err
	}
	loaded, err := store.LoadState()
	if err != nil {
		return QARunResult{}, err
	}
	emitQA(req.Progress, QAProgress{Phase: loaded.Phase, Event: "investigation_complete", Completed: loaded.CompletedShards, Total: loaded.TotalShards, Message: "QA investigation complete"})
	return QARunResult{Project: sp.Project, Sprint: sp.Slug, State: loaded, Map: mapResult.Map, Shards: shards, Synthesis: synthesis}, nil
}

func (s Service) buildQAEvidencePublication(ctx context.Context, sp Sprint, qaMap QAMap, target string, shards []QAShard, reconciledIssues []QAArbiterIssue, progress func(QAProgress)) (QAEvidencePublication, QAAssessmentRecord, error) {
	// Recheck admission after investigation because review, smoke, or the target
	// may have changed while the model work was in flight.
	status, implementationBefore, err := s.validateQAEvidenceAdmission(sp, qaMap, target)
	if err != nil {
		return QAEvidencePublication{}, QAAssessmentRecord{}, err
	}
	return s.buildQAEvidencePublicationAdmitted(ctx, sp, qaMap, target, shards, reconciledIssues, progress, status, implementationBefore)
}

func (s Service) validateQAEvidenceAdmission(sp Sprint, qaMap QAMap, target string) (VerificationStatus, string, error) {
	implementationBefore, err := targetIdentity(target)
	if err != nil || implementationBefore != qaMap.ImplementationFingerprint {
		return VerificationStatus{}, "", NewQAError(QAErrorStaleInput, "admission", "implementation no longer matches the frozen QA map", err)
	}
	status, err := s.VerificationStatus(sp.Project, sp.Slug)
	if err != nil {
		return VerificationStatus{}, "", NewQAError(QAErrorAdmissionBlocked, "admission", "verification prerequisites are unavailable", err)
	}
	capabilities := pprocess.IsolationCapabilityFacts()
	admission := QAAdmission{
		ReviewCurrent: status.Review.Fresh, ReviewVerdict: status.Review.Verdict,
		SmokeCurrent: status.Smoke.Fresh, SmokeVerdict: status.Smoke.Verdict, ContainingSmoke: status.Smoke.Fresh,
		ReadOnlyProofs: []string{"deterministic_map", "bounded_investigation", "synthesis"}, MapComplete: len(qaMap.Coverage.BlockedPaths) == 0 && len(qaMap.Coverage.PrimaryOwners) == len(qaMap.Coverage.ChangedPaths),
		IsolationProven:     capabilities.NativeProtectedRootDeny && capabilities.DescendantCleanup && capabilities.WorkspaceRemoval,
		WritableConcurrency: 1,
	}
	if err := ValidateQAAdmission(admission); err != nil {
		return VerificationStatus{}, "", err
	}
	return status, implementationBefore, nil
}

func (s Service) buildQAEvidencePublicationAdmitted(ctx context.Context, sp Sprint, qaMap QAMap, target string, shards []QAShard, reconciledIssues []QAArbiterIssue, progress func(QAProgress), status VerificationStatus, implementationBefore string) (QAEvidencePublication, QAAssessmentRecord, error) {
	limits := pprocess.IsolationLimits{MaxFiles: qaMap.Budgets.TreeFiles, MaxBytes: qaMap.Budgets.TreeBytes, MaxFileSize: qaMap.Budgets.FileBytes, Timeout: qaMap.Budgets.ShardTimeout}
	targetTreeIdentity, err := pprocess.IdentifyTree(ctx, target, limits)
	if err != nil {
		return QAEvidencePublication{}, QAAssessmentRecord{}, NewQAError(QAErrorPermissionDenied, "admission", "cannot freeze the protected target identity", err)
	}
	mapFingerprint, err := fingerprintQAValue(qaMap)
	if err != nil {
		return QAEvidencePublication{}, QAAssessmentRecord{}, err
	}
	workspaceParent, err := os.MkdirTemp("", "ultraplan-qa-evidence-")
	if err != nil {
		return QAEvidencePublication{}, QAAssessmentRecord{}, NewQAError(QAErrorPermissionDenied, "admission", "cannot create the private QA workspace parent", err)
	}
	defer os.RemoveAll(workspaceParent)
	plans := make([]QAEvidencePlan, 0, len(shards)*2)
	records := make([]QAEvidenceRecord, 0, len(shards)*2)
	candidateByKey := make(map[string]QAIssueCandidate)
	issueByTheory := make(map[string]QAArbiterIssue)
	for _, issue := range reconciledIssues {
		for _, theoryID := range issue.TheoryIDs {
			issueByTheory[theoryID] = issue
		}
	}
	evaluators := make([]QAModelObservation, 0)
	approvedPathsByShard := make(map[string][]string, len(shards))
	descriptorsByShard := make(map[string][]QACheckDescriptor, len(shards))
	totalEvidence := 0
	for _, shard := range shards {
		if shard.Phase != QAPhaseCompleted {
			continue
		}
		approvedPaths := normalizeQAStrings(append(append([]string(nil), shard.ChangedPaths...), shard.ContextPaths...))
		if len(approvedPaths) == 0 {
			continue
		}
		descriptors, checkErr := ApprovedQAChecks(target, approvedPaths, qaMap.Budgets)
		if checkErr != nil {
			return QAEvidencePublication{}, QAAssessmentRecord{}, checkErr
		}
		if len(descriptors) == 0 {
			checkID := "no-applicable-check"
			if qaHasTextEvidencePaths(approvedPaths) {
				checkID = "text-integrity"
			}
			descriptors = []QACheckDescriptor{{ID: checkID, Timeout: qaMap.Budgets.CommandTimeout, OutputLimit: qaMap.Budgets.CommandOutputBytes}}
		}
		approvedPathsByShard[shard.ID] = approvedPaths
		descriptorsByShard[shard.ID] = descriptors
		totalEvidence += len(descriptors)
	}
	completedEvidence := 0
	executedChecks := make(map[string]QAEvidenceRecord)
	for _, shard := range shards {
		if shard.Phase != QAPhaseCompleted {
			continue
		}
		approvedPaths := approvedPathsByShard[shard.ID]
		if len(approvedPaths) == 0 {
			continue
		}
		emitQA(progress, QAProgress{Phase: QAPhaseRunning, Event: "evidence_started", ShardID: shard.ID, Completed: completedEvidence, Total: totalEvidence, Message: "Running isolated evidence check"})
		descriptors := descriptorsByShard[shard.ID]
		for _, descriptor := range descriptors {
			confirmed := make([]QATheory, 0, len(shard.Theories))
			confirmedIDs := make([]string, 0, len(shard.Theories))
			for _, theory := range shard.Theories {
				if theory.Outcome == QATheoryConfirmed && qaTheoryUsesCheck(theory, descriptor.ID) {
					confirmed = append(confirmed, theory)
					confirmedIDs = append(confirmedIDs, theory.ID)
				}
			}
			executable := ""
			if descriptor.Executable != "" {
				var lookupErr error
				executable, lookupErr = exec.LookPath(descriptor.Executable)
				if lookupErr != nil {
					return QAEvidencePublication{}, QAAssessmentRecord{}, NewQAError(QAErrorAdmissionBlocked, "plan evidence", "an approved check executable is unavailable", lookupErr)
				}
			}
			plan, planErr := FreezeQAEvidencePlan(sp.Project, sp.Slug, QAEvidencePlan{
				AttemptID: qaMap.SemanticAttemptID, ShardID: shard.ID, TheoryIDs: confirmedIDs, ExpectationRefs: shard.ExpectationRefs,
				Kind: QACheckFact, ConfirmationCondition: "approved check exits successfully and satisfies its output policy", RefutationCondition: "approved check exits unsuccessfully or violates its output policy", InconclusiveCondition: "approved check is unavailable or incomplete",
				ApprovedPaths: approvedPaths, CheckID: descriptor.ID, Executable: executable, Args: append([]string(nil), descriptor.Args...), Timeout: descriptor.Timeout, OutputLimit: descriptor.OutputLimit, RequireEmptyStdout: descriptor.RequireEmptyOut,
				CleanupRequired: true, GovernedInputFingerprint: qaMap.GovernedInputFingerprint, ImplementationFingerprint: qaMap.ImplementationFingerprint, MapFingerprint: mapFingerprint,
			}, qaMap.Budgets, s.now().UTC())
			if planErr != nil {
				return QAEvidencePublication{}, QAAssessmentRecord{}, planErr
			}
			checkKey, _ := fingerprintQAValue(struct {
				Descriptor QACheckDescriptor
				Paths      []string
				Target     string
			}{descriptor, approvedPaths, targetTreeIdentity.Digest})
			record, reused := executedChecks[checkKey]
			if reused {
				record, planErr = cloneQAEvidenceForPlan(sp, record, plan)
				if planErr != nil {
					return QAEvidencePublication{}, QAAssessmentRecord{}, planErr
				}
			} else {
				var runErr error
				record, runErr = RunQAInvestigation(ctx, QAInvestigationRequest{Project: sp.Project, Sprint: sp.Slug, TargetRoot: target, WorkspaceParent: workspaceParent, ProtectedRoots: []string{s.root, target}, Plan: plan, Budgets: qaMap.Budgets, ExpectedTargetID: targetTreeIdentity.Digest, Now: s.now})
				if runErr != nil {
					return QAEvidencePublication{}, QAAssessmentRecord{}, runErr
				}
				executedChecks[checkKey] = record
			}
			plans, records = append(plans, plan), append(records, record)
			completedEvidence++
			emitQA(progress, QAProgress{Phase: QAPhaseRunning, Event: "evidence_completed", ShardID: shard.ID, Completed: completedEvidence, Total: totalEvidence, Message: "Isolated evidence check complete"})
			if record.Outcome != QAEvidenceFail {
				continue
			}
			if len(confirmed) == 0 {
				key := shard.ID + "\x00" + descriptor.ID
				candidateByKey[key] = QAIssueCandidate{Title: "Approved QA check failed", Claim: "approved check " + descriptor.ID + " failed in the isolated copy", IssueClass: "behavior", Severity: "medium", Location: approvedPaths[0], EvidenceIDs: []string{record.ID}, RepairEligible: true, RegressionCandidate: true}
				continue
			}
			for _, theory := range confirmed {
				key := theory.ID
				candidate := candidateByKey[key]
				candidate.Claim, candidate.Title, candidate.Location = theory.Claim, theory.Claim, theory.VerificationSurface
				candidate.IssueClass, candidate.Severity = "behavior", theory.SeverityIfConfirmed
				if issue, ok := issueByTheory[theory.ID]; ok {
					key = issue.ID
					candidate = candidateByKey[key]
					candidate.Claim, candidate.Title, candidate.Location = issue.Claim, issue.Title, issue.Location
					candidate.IssueClass, candidate.Severity = issue.IssueClass, issue.Severity
				}
				candidate.RepairEligible, candidate.RegressionCandidate = true, true
				candidate.EvidenceIDs = normalizeQAStrings(append(candidate.EvidenceIDs, record.ID))
				candidateByKey[key] = candidate
			}
		}
	}
	candidateKeys := make([]string, 0, len(candidateByKey))
	for key := range candidateByKey {
		candidateKeys = append(candidateKeys, key)
	}
	sort.Strings(candidateKeys)
	if len(candidateKeys) > qaMap.Budgets.Issues {
		candidateKeys = candidateKeys[:qaMap.Budgets.Issues]
	}
	candidates := make([]QAIssueCandidate, 0, len(candidateKeys))
	for _, key := range candidateKeys {
		candidates = append(candidates, candidateByKey[key])
	}
	settings, err := s.effectiveQASettings()
	if err != nil {
		return QAEvidencePublication{}, QAAssessmentRecord{}, err
	}
	adjudication, err := AdjudicateQA(QAAdjudicationRequest{Project: sp.Project, Sprint: sp.Slug, AttemptID: qaMap.SemanticAttemptID, MapFingerprint: mapFingerprint, Plans: plans, Evidence: records, Candidates: candidates, Evaluators: evaluators, Budgets: qaMap.Budgets, Now: s.now().UTC(), RepairAssignmentMode: settings.RepairAssignmentMode, IssuesPerRepairAgent: settings.IssuesPerRepairAgent})
	if err != nil {
		return QAEvidencePublication{}, QAAssessmentRecord{}, err
	}
	assessmentValue, nextAction := DeriveQAAssessment(status.Review, records, adjudication, &status.Smoke, nil)
	assessmentID, err := NewQAV2ID("assessment", sp.Project, sp.Slug, qaMap.SemanticAttemptID, struct {
		Assessment OverallAssessment
		Evidence   []string
		Issues     int
	}{assessmentValue, adjudication.AcceptedIDs, len(adjudication.Issues)})
	if err != nil {
		return QAEvidencePublication{}, QAAssessmentRecord{}, err
	}
	assessment := QAAssessmentRecord{SchemaVersion: QAEvidenceSchemaVersion, ID: assessmentID, AttemptID: qaMap.SemanticAttemptID, ReviewVerdict: ReviewVerdict(status.Review.Verdict), ReviewFingerprint: status.Review.InputFingerprint, SmokeVerdict: SmokeVerdict(status.Smoke.Verdict), SmokeRunID: status.Smoke.RunID, Assessment: assessmentValue, EvidenceTotal: len(records), RejectedTotal: len(adjudication.Rejected), IssueTotal: len(adjudication.Issues), NextAction: nextAction, CompletedAt: s.now().UTC()}
	report, err := RenderQAReport(sp.Project, sp.Slug, qaMap.GovernedInputFingerprint, records, adjudication, assessment)
	if err != nil {
		return QAEvidencePublication{}, QAAssessmentRecord{}, err
	}
	implementationAfter, err := targetIdentity(target)
	if err != nil || implementationAfter != implementationBefore {
		return QAEvidencePublication{}, QAAssessmentRecord{}, NewQAError(QAErrorStaleInput, "publish evidence", "implementation changed during evidence production", err)
	}
	return QAEvidencePublication{Plans: plans, Records: records, Adjudication: &adjudication, Assessment: &assessment, Report: []byte(report), Budgets: qaMap.Budgets}, assessment, nil
}

func cloneQAEvidenceForPlan(sp Sprint, source QAEvidenceRecord, plan QAEvidencePlan) (QAEvidenceRecord, error) {
	record := source
	record.ID = ""
	record.PlanID, record.AttemptID, record.ShardID = plan.ID, plan.AttemptID, plan.ShardID
	record.GovernedInputFingerprint, record.ImplementationFingerprint, record.MapFingerprint = plan.GovernedInputFingerprint, plan.ImplementationFingerprint, plan.MapFingerprint
	id, err := NewQAV2ID("evidence", sp.Project, sp.Slug, plan.ID, struct {
		Source   string
		Commands []QACommandResult
		Outcome  QAEvidenceOutcome
	}{source.ID, record.Commands, record.Outcome})
	if err != nil {
		return QAEvidenceRecord{}, err
	}
	record.ID = id
	return record, nil
}

func qaHasTextEvidencePaths(paths []string) bool {
	if len(paths) == 0 {
		return false
	}
	for _, path := range paths {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".md", ".txt", ".json", ".yaml", ".yml", ".toml", ".html", ".css", ".js":
		default:
			return false
		}
	}
	return true
}

func qaTheoryUsesCheck(theory QATheory, checkID string) bool {
	if strings.TrimSpace(checkID) == "" {
		return false
	}
	for _, evidence := range theory.Evidence {
		if evidence.CheckID == checkID {
			return true
		}
	}
	return false
}

func (s Service) publishTerminalQAFailure(store QAStore, flow FlowState, qaMap QAMap, shards []QAShard, state QAState, token QAWriterToken, runErr error) (QARunResult, error) {
	return s.publishTerminalQAFailureRecord(store, flow, qaMap, shards, nil, state, token, runErr)
}

func (s Service) publishTerminalQAFailureWithSynthesis(store QAStore, flow FlowState, qaMap QAMap, shards []QAShard, synthesis QASynthesis, state QAState, token QAWriterToken, runErr error) (QARunResult, error) {
	return s.publishTerminalQAFailureRecord(store, flow, qaMap, shards, &synthesis, state, token, runErr)
}

func (s Service) publishTerminalQAFailureRecord(store QAStore, flow FlowState, qaMap QAMap, shards []QAShard, synthesis *QASynthesis, state QAState, token QAWriterToken, runErr error) (QARunResult, error) {
	state = terminalQAState(state, runErr, s.now().UTC())
	if publishErr := store.Publish(QAPublication{Synthesis: synthesis, State: state, Flow: flow}, token); publishErr != nil {
		return QARunResult{Project: qaMap.Project, Sprint: qaMap.Sprint, State: state, Map: qaMap, Shards: shards}, errors.Join(runErr, publishErr)
	}
	result := QARunResult{Project: qaMap.Project, Sprint: qaMap.Sprint, State: state, Map: qaMap, Shards: shards}
	if synthesis != nil {
		result.Synthesis = *synthesis
	}
	return result, runErr
}

func hydrateQASynthesisFollowUps(synthesis *QASynthesis, shards []QAShard) error {
	byID := make(map[string]QAShard, len(shards))
	for _, shard := range shards {
		byID[shard.ID] = shard
	}
	for i := range synthesis.FollowUpShards {
		current, ok := byID[synthesis.FollowUpShards[i].ID]
		if !ok || (current.Phase != QAPhaseCompleted && current.Phase != QAPhaseBlocked) {
			return NewQAError(QAErrorInvalidState, "synthesize", "a proposed follow-up shard did not reach a retained terminal state", nil)
		}
		synthesis.FollowUpShards[i] = current
	}
	return nil
}

func pendingQASynthesisFollowUps(synthesis QASynthesis, shards []QAShard, limit int) []QAShard {
	retained := make(map[string]struct{}, len(shards))
	followUpCount := 0
	for _, shard := range shards {
		retained[shard.ID] = struct{}{}
		if shard.Kind == QAShardFollowUp {
			followUpCount++
		}
	}
	remaining := limit - followUpCount
	if remaining <= 0 {
		return nil
	}
	pending := make([]QAShard, 0, remaining)
	for _, follow := range synthesis.FollowUpShards {
		if _, ok := retained[follow.ID]; ok {
			continue
		}
		pending = append(pending, follow)
		if len(pending) == remaining {
			break
		}
	}
	return pending
}

func finalizeQASynthesisFollowUps(synthesis *QASynthesis, qaMap QAMap, shards []QAShard) error {
	followUps := make([]QAShard, 0, qaMap.Budgets.FollowUpShards)
	for _, shard := range shards {
		if shard.Kind != QAShardFollowUp {
			continue
		}
		if shard.Phase != QAPhaseCompleted && shard.Phase != QAPhaseBlocked {
			return NewQAError(QAErrorInvalidState, "synthesize", "a retained follow-up shard did not reach a terminal state", nil)
		}
		followUps = append(followUps, shard)
	}
	if len(followUps) > qaMap.Budgets.FollowUpShards {
		return NewQAError(QAErrorBudgetExhausted, "synthesize", "retained follow-up shards exceed the configured budget", nil)
	}
	sort.Slice(followUps, func(i, j int) bool { return followUps[i].ID < followUps[j].ID })
	synthesis.FollowUpShards = followUps
	followIDs := make([]string, 0, len(followUps))
	for _, follow := range followUps {
		followIDs = append(followIDs, follow.ID)
	}
	challengeIDs := make([]string, 0, len(synthesis.Challenges))
	for _, challenge := range synthesis.Challenges {
		challengeIDs = append(challengeIDs, challenge.ID)
	}
	id, err := NewQASynthesisID(qaMap.Project, qaMap.Sprint, qaMap.SemanticAttemptID, QASynthesisIdentity{MapID: qaMap.ID, TheoryIDs: synthesis.TheoryIDs, ChallengeIDs: challengeIDs, FollowUpIDs: followIDs, PolicyFingerprint: qaMap.PolicyFingerprint})
	if err != nil {
		return err
	}
	synthesis.ID = id
	synthesis.NextAction = "Inspect the retained theory outcomes."
	if len(synthesis.Blockers) > 0 {
		synthesis.NextAction = "Inspect the retained shard blockers and resume only after their stated prerequisites are restored."
	}
	if err := ValidateQASynthesis(*synthesis, qaMap.Budgets); err != nil {
		return NewQAError(QAErrorInvalidState, "synthesize", err.Error(), err)
	}
	return nil
}

func (s Service) prepareQAAttempt(store QAStore, flow FlowState, qaMap QAMap, req QARunRequest, settings QASettings) (QAState, []QAShard, error) {
	now := s.now().UTC()
	if req.Resume {
		prior, err := store.LoadState()
		if err == nil && prior.CurrentAttemptID == qaMap.SemanticAttemptID && prior.Map != nil {
			shards := append([]QAShard(nil), qaMap.Shards...)
			for i := range shards {
				loaded, loadErr := store.LoadShard(qaMap.SemanticAttemptID, shards[i].ID)
				if loadErr == nil && (loaded.Phase == QAPhaseCompleted || loaded.Phase == QAPhaseBlocked) {
					shards[i] = loaded
				}
			}
			prior.Run = qaRunCorrelation(req.WriterToken, QARunClaimed)
			prior.Phase = QAPhaseQueued
			prior.Blocker = nil
			prior.Cancellation = QACancellation{}
			prior.CompletedShards = countTerminalQAShards(shards)
			prior.TotalShards = len(shards)
			prior.NextAction = "Resume incomplete shards from the current semantic attempt."
			prior.UpdatedAt = now
			if err := store.Publish(QAPublication{State: prior, Flow: flow}, req.WriterToken); err != nil {
				return QAState{}, nil, err
			}
			return prior, shards, nil
		}
	}
	if prior, err := store.LoadState(); err == nil && prior.CurrentAttemptID == qaMap.SemanticAttemptID {
		return QAState{}, nil, NewQAError(QAErrorConflict, "run", "the current semantic attempt already exists; use qa resume", nil)
	}
	state := QAState{SchemaVersion: QASchemaVersion, Project: qaMap.Project, Sprint: qaMap.Sprint, Phase: QAPhaseMapped,
		Freshness:        QAFreshness{Current: true, GovernedInputFingerprint: qaMap.GovernedInputFingerprint, ImplementationFingerprint: qaMap.ImplementationFingerprint, ReviewFingerprint: qaMap.ReviewFingerprint, PolicyFingerprint: qaMap.PolicyFingerprint},
		CurrentAttemptID: qaMap.SemanticAttemptID, CompletedShards: countTerminalQAShards(qaMap.Shards), TotalShards: len(qaMap.Shards), Run: qaRunCorrelation(req.WriterToken, QARunClaimed), NextAction: "Run the mapped read-only QA shards.", UpdatedAt: now}
	shards := append([]QAShard(nil), qaMap.Shards...)
	if err := store.Publish(QAPublication{Map: &qaMap, Shards: shards, State: state, Flow: flow}, req.WriterToken); err != nil {
		return QAState{}, nil, err
	}
	loaded, err := store.LoadState()
	if err != nil {
		return QAState{}, nil, err
	}
	if err := store.PruneAttempts(qaMap.SemanticAttemptID, settings.Budgets.RetainedAttempts); err != nil {
		return QAState{}, nil, err
	}
	return loaded, shards, nil
}

func (s Service) runQAShardBatch(ctx context.Context, store QAStore, flow FlowState, qaMap QAMap, target string, shards []QAShard, state QAState, req QARunRequest) ([]QAShard, QAState, error) {
	batchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	abortResults := make(chan struct{})
	indices := make([]int, 0, len(shards))
	for i := range shards {
		if shards[i].Phase == QAPhaseCompleted || shards[i].Phase == QAPhaseBlocked {
			continue
		}
		if req.FocusShard != "" && shards[i].ID != req.FocusShard {
			continue
		}
		indices = append(indices, i)
	}
	if req.FocusShard != "" && len(indices) == 0 {
		return shards, state, NewQAError(QAErrorInvalidState, "run", "focused shard is absent or already terminal", nil)
	}
	workers := qaMap.Budgets.ConcurrentInvestigators
	if workers > len(indices) {
		workers = len(indices)
	}
	if workers == 0 {
		return shards, state, nil
	}
	jobs := make(chan int, workers)
	results := make(chan qaShardResult, workers)
	var group sync.WaitGroup
	for i := 0; i < workers; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for index := range jobs {
				shard, err := s.runOneQAShardSafely(batchCtx, qaMap, shards[index], target, req.WriterToken)
				select {
				case results <- qaShardResult{shard: shard, err: err}:
				case <-abortResults:
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, index := range indices {
			select {
			case jobs <- index:
			case <-batchCtx.Done():
				return
			}
		}
	}()
	go func() {
		group.Wait()
		close(results)
	}()
	byID := make(map[string]int, len(shards))
	for i := range shards {
		byID[shards[i].ID] = i
	}
	var publishErr error
	for result := range results {
		if publishErr != nil {
			continue
		}
		index := byID[result.shard.ID]
		if result.err != nil {
			switch {
			case errors.Is(result.err, context.Canceled):
				result.shard.Phase = QAPhaseCancelled
			case errors.Is(result.err, context.DeadlineExceeded):
				result.shard.Phase = QAPhaseInterrupted
			default:
				result.shard.Phase = QAPhaseBlocked
			}
			result.shard.Blocker = qaBlocker(result.err, result.shard.ID)
		}
		shards[index] = result.shard
		state.CompletedShards = countTerminalQAShards(shards)
		state.UpdatedAt = s.now().UTC()
		if err := store.Publish(QAPublication{Shards: []QAShard{result.shard}, State: state, Flow: flow}, req.WriterToken); err != nil {
			publishErr = err
			close(abortResults)
			cancel()
			continue
		}
		emitQA(req.Progress, QAProgress{Phase: state.Phase, ShardID: result.shard.ID, ShardKind: result.shard.Kind, ShardPhase: result.shard.Phase, Event: "shard_terminal", Completed: state.CompletedShards, Total: state.TotalShards, Message: "QA shard reached a terminal state"})
	}
	if publishErr != nil {
		return shards, state, publishErr
	}
	if err := ctx.Err(); err != nil {
		return shards, state, err
	}
	return shards, state, nil
}

func (s Service) runOneQAShardSafely(ctx context.Context, qaMap QAMap, shard QAShard, target string, token QAWriterToken) (result QAShard, err error) {
	result = shard
	defer func() {
		if recovered := recover(); recovered != nil {
			err = NewQAError(QAErrorRuntimeUnavailable, "investigate shard", "investigator runtime panicked", fmt.Errorf("panic: %v", recovered))
		}
	}()
	return s.runOneQAShard(ctx, qaMap, shard, target, token)
}

func (s Service) runOneQAShard(ctx context.Context, qaMap QAMap, shard QAShard, target string, token QAWriterToken) (QAShard, error) {
	if err := s.validateCurrentQAMap(qaMap); err != nil {
		return shard, err
	}
	request, err := s.QAInvestigatorRequest(qaMap, shard, target)
	if err != nil {
		return shard, err
	}
	request.Metadata["operation"] = "qa-investigate"
	request.Metadata["task"] = shard.ID
	request.Metadata["operational_attempt"] = token.OperationalAttemptID
	before, identityErr := targetIdentity(target)
	if identityErr != nil || before != qaMap.ImplementationFingerprint {
		return shard, NewQAError(QAErrorStaleInput, "investigate shard", "implementation identity no longer matches the QA map", identityErr)
	}
	started := s.now().UTC()
	result, output, repairDiagnostic, runErr, decodeErr := s.runQAInvestigatorRuntime(ctx, qaMap, request)
	completed := s.now().UTC()
	after, afterErr := targetIdentity(target)
	runtimeEvents := result.EventStats.Total
	if runtimeEvents == 0 {
		runtimeEvents = int64(len(result.Events))
	}
	attempt := QAInvestigatorAttempt{ID: fmt.Sprintf("%s/%s/1", token.OperationalAttemptID, shard.ID), Number: 1, StartedAt: started, CompletedAt: &completed, ImplementationBefore: before, ImplementationAfter: after, Usage: qaUsageSummary(result.Usage), RuntimeEvents: runtimeEvents, RetainedEvents: len(result.Events), ObservedToolCalls: qaObservedToolCalls(result.Events), ContextMetrics: qaAttemptContextMetrics(request, result.Events, completed.Sub(started))}
	attempt.Repair = repairDiagnostic
	if result.EstimatedCost != nil && result.EstimatedCost.Source != "unpriced" && (result.EstimatedCost.Source != "" || result.EstimatedCost.Amount != 0) {
		attempt.EstimatedCost = &QACostSummary{Amount: result.EstimatedCost.Amount, Currency: result.EstimatedCost.Currency, Estimate: result.EstimatedCost.Estimate, Source: result.EstimatedCost.Source}
	}
	if afterErr != nil || after != before {
		attempt.StopReason = "implementation identity drift"
		shard.Attempts = append(shard.Attempts, attempt)
		return shard, NewQAError(QAErrorPermissionDenied, "investigate shard", "implementation identity changed during read-only investigation", afterErr)
	}
	if currentErr := s.validateCurrentQAMap(qaMap); currentErr != nil {
		attempt.StopReason = "governed input drift"
		shard.Attempts = append(shard.Attempts, attempt)
		return shard, currentErr
	}
	if result.Permissions.Mode != "restricted" || result.Permissions.Default != "deny" || result.Permissions.UnsupportedCount != 0 {
		attempt.StopReason = "permission enforcement unavailable"
		shard.Attempts = append(shard.Attempts, attempt)
		return shard, NewQAError(QAErrorPermissionDenied, "investigate shard", "runtime did not enforce restricted default-deny permissions", nil)
	}
	if runErr != nil {
		attempt.FailureKind, attempt.Retryable = classifyQARuntimeFailure(result, runErr)
		attempt.StopReason = "runtime policy exhausted"
		shard.Attempts = append(shard.Attempts, attempt)
		if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
			return shard, runErr
		}
		return shard, NewQAError(QAErrorRuntimeUnavailable, "investigate shard", "runtime resilience policy exhausted", runErr)
	}
	if decodeErr != nil {
		_, diagnostic, _ := decodeQAInvestigatorOutput(result, qaMap.Budgets.ShardOutputBytes)
		attempt.FailureKind, attempt.OutputDiagnostic = "invalid_output", &diagnostic
		shard.Attempts = append(shard.Attempts, attempt)
		if repairDiagnostic == nil {
			attempt.StopReason = "investigator output rejected"
			shard.Attempts[len(shard.Attempts)-1] = attempt
			return shard, NewQAError(QAErrorInvalidState, "investigate shard", "runtime returned invalid investigator output", decodeErr)
		}
		attempt.StopReason = "investigator output repair exhausted"
		shard.Attempts[len(shard.Attempts)-1] = attempt
		detail := fmt.Sprintf("investigator output repair exhausted after %d repair attempts", repairDiagnostic.AttemptCount)
		return shard, NewQAError(QAErrorBudgetExhausted, "investigate shard", detail, decodeErr)
	}
	if result.Usage.TurnsKnown && result.Usage.Turns > int64(qaMap.Budgets.IterationsPerAttempt) {
		attempt.StopReason = "investigator iteration limit exceeded"
		shard.Attempts = append(shard.Attempts, attempt)
		return shard, NewQAError(QAErrorBudgetExhausted, "investigate shard", fmt.Sprintf("investigator used %d turns; limit is %d", result.Usage.Turns, qaMap.Budgets.IterationsPerAttempt), nil)
	}
	attempt.StopReason = "terminal investigator output accepted"
	if !withinQAInvestigatorBudgets(output, qaMap.Budgets) {
		return shard, NewQAError(QAErrorBudgetExhausted, "investigate shard", "investigator output exceeds map-owned limits", nil)
	}
	attempt.ContextRequests = append([]QAContextRequest(nil), output.Context...)
	attempt.Evidence = append([]QAEvidenceSummary(nil), output.Evidence...)
	for i := range attempt.ContextRequests {
		contextRequest := &attempt.ContextRequests[i]
		if contextRequest.Approved {
			return shard, NewQAError(QAErrorPermissionDenied, "investigate shard", "runtime cannot self-approve context expansion", nil)
		}
		if len(contextRequest.Paths) > qaMap.Budgets.PathsPerExpansion {
			return shard, NewQAError(QAErrorBudgetExhausted, "investigate shard", "context request exceeds path limit", nil)
		}
		for _, path := range contextRequest.Paths {
			if err := validateQAPath(path); err != nil {
				return shard, NewQAError(QAErrorPermissionDenied, "investigate shard", err.Error(), err)
			}
		}
		approved, reason := approveQAContextPaths(target, contextRequest.Paths)
		contextRequest.Approved = approved
		contextRequest.DeniedReason = reason
	}
	approvedContext := make([]string, 0)
	for _, contextRequest := range attempt.ContextRequests {
		if contextRequest.Approved {
			approvedContext = append(approvedContext, contextRequest.Paths...)
		}
	}
	approvedContext = normalizeQAStrings(approvedContext)
	if len(approvedContext) > 0 && result.SessionID != "" {
		continuedResult, continuedOutput, continuationBytes, continueErr := s.continueQAInvestigator(ctx, qaMap, shard, target, request, result.SessionID, approvedContext)
		attempt.ContextMetrics.Expansions++
		attempt.ContextMetrics.ContinuationBytes += continuationBytes
		if continueErr != nil {
			attempt.StopReason = "approved context continuation failed"
			shard.Attempts = append(shard.Attempts, attempt)
			return shard, continueErr
		}
		output = continuedOutput
		attempt.Usage = mergeQAUsage(attempt.Usage, qaUsageSummary(continuedResult.Usage))
		attempt.RuntimeEvents += continuedResult.EventStats.Total
		attempt.RetainedEvents += len(continuedResult.Events)
		attempt.ObservedToolCalls += qaObservedToolCalls(continuedResult.Events)
	}
	checks, err := ApprovedQAChecks(target, qaMap.Coverage.ChangedPaths, qaMap.Budgets)
	if err != nil {
		return shard, err
	}
	checkByID := map[string]QACheckDescriptor{}
	for _, descriptor := range checks {
		checkByID[descriptor.ID] = descriptor
	}
	for _, requested := range output.Checks {
		descriptor, ok := checkByID[requested.ID]
		if !ok {
			return shard, NewQAError(QAErrorPermissionDenied, "investigate shard", "runtime requested a check outside the map-owned catalog", nil)
		}
		summary, checkErr := s.RunApprovedQACheck(ctx, qaMap, descriptor, requested)
		attempt.Commands = append(attempt.Commands, summary)
		if checkErr != nil {
			return shard, checkErr
		}
	}
	theories := make([]QATheory, 0, len(output.Theories))
	for _, draft := range output.Theories {
		identity := QATheoryIdentity{Claim: draft.Claim, Basis: draft.Basis, VerificationSurface: draft.VerificationSurface, ExpectationRefs: normalizeQAStrings(draft.ExpectationRefs)}
		id, idErr := NewQATheoryID(qaMap.Project, qaMap.Sprint, shard.ID, identity)
		if idErr != nil {
			return shard, idErr
		}
		theory := QATheory{SchemaVersion: QASchemaVersion, ID: id, ShardID: shard.ID, Claim: draft.Claim, Basis: draft.Basis, VerificationSurface: draft.VerificationSurface, ExpectationRefs: identity.ExpectationRefs, SeverityIfConfirmed: draft.SeverityIfConfirmed, ConfirmationCondition: draft.ConfirmationCondition, RefutationCondition: draft.RefutationCondition, InconclusiveCondition: draft.InconclusiveCondition, SafeEvidenceStrategy: draft.SafeEvidenceStrategy, ImplementationFingerprint: attempt.ImplementationBefore, AttemptHistory: append(append([]QAInvestigatorAttempt(nil), shard.Attempts...), attempt), Evidence: append([]QAEvidenceSummary(nil), output.Evidence...), Outcome: draft.Outcome, OutcomeReason: draft.OutcomeReason}
		if err := ValidateQATheory(theory); err != nil {
			return shard, NewQAError(QAErrorInvalidState, "investigate shard", err.Error(), err)
		}
		theories = append(theories, theory)
	}
	if len(theories) == 0 {
		shard.Attempts = append(shard.Attempts, attempt)
		return shard, NewQAError(QAErrorInvalidState, "investigate shard", "investigator returned no falsifiable theories", nil)
	}
	if err := s.validateCurrentQAMap(qaMap); err != nil {
		return shard, err
	}
	sort.Slice(theories, func(i, j int) bool { return theories[i].ID < theories[j].ID })
	shard.Attempts = append(shard.Attempts, attempt)
	shard.Theories = theories
	shard.Phase = QAPhaseCompleted
	shard.Blocker = nil
	return shard, nil
}

func withinQAInvestigatorBudgets(output qaInvestigatorOutput, budgets QABudgets) bool {
	return len(output.Theories) <= budgets.TheoriesPerShard && len(output.Context) <= budgets.ContextExpansions && len(output.Checks) <= budgets.CommandsPerAttempt
}

func (s Service) continueQAInvestigator(ctx context.Context, qaMap QAMap, shard QAShard, target string, initial pruntime.Request, sessionID string, paths []string) (pruntime.Result, qaInvestigatorOutput, int, error) {
	blocks := make([]QAContextBlock, 0, len(paths))
	for _, rel := range paths {
		data, err := os.ReadFile(filepath.Join(target, filepath.FromSlash(rel)))
		if err != nil {
			return pruntime.Result{}, qaInvestigatorOutput{}, 0, NewQAError(QAErrorStaleInput, "continue investigator", "approved context path is no longer readable", err)
		}
		blocks = append(blocks, newQAContextBlock("source", rel, string(data), "approved in-session context delta", shard.ExpectationRefs, []string{rel}))
	}
	delta, err := canonicalQAJSON(struct {
		SchemaVersion int              `json:"schema_version"`
		ShardID       string           `json:"shard_id"`
		Blocks        []QAContextBlock `json:"blocks"`
	}{QASchemaVersion, shard.ID, blocks})
	if err != nil {
		return pruntime.Result{}, qaInvestigatorOutput{}, 0, err
	}
	prompt := "Approved context delta follows. Continue the same investigation without repeating repository discovery. Return the complete five-field investigator JSON object, replacing any earlier provisional theories. Cite the new block IDs where used.\n\n" + string(delta) + "\n"
	if len(prompt) > qaMap.Budgets.PromptBytes {
		return pruntime.Result{}, qaInvestigatorOutput{}, len(prompt), NewQAError(QAErrorBudgetExhausted, "continue investigator", "context delta exceeds prompt budget", nil)
	}
	req := initial
	req.Prompt, req.SessionID, req.SessionAction, req.Cache = prompt, sessionID, "continue", pruntime.CacheDirective{}
	req.Metadata["operation"] = "qa-investigate-context-continuation"
	req.Policy = qaNoToolPolicy()
	result, output, _, runErr, decodeErr := s.runQAInvestigatorRuntime(ctx, qaMap, req)
	if runErr != nil {
		return result, qaInvestigatorOutput{}, len(prompt), NewQAError(QAErrorRuntimeUnavailable, "continue investigator", "same-session context continuation failed", runErr)
	}
	if result.SessionID != sessionID || result.Permissions.Mode != "restricted" || result.Permissions.Default != "deny" || result.Permissions.UnsupportedCount != 0 {
		return result, qaInvestigatorOutput{}, len(prompt), NewQAError(QAErrorPermissionDenied, "continue investigator", "runtime did not preserve the restricted investigator session", nil)
	}
	if decodeErr != nil {
		return result, qaInvestigatorOutput{}, len(prompt), NewQAError(QAErrorInvalidState, "continue investigator", "context continuation returned invalid output", decodeErr)
	}
	return result, output, len(prompt), nil
}

func (s Service) runQAInvestigatorRuntime(ctx context.Context, qaMap QAMap, request pruntime.Request) (pruntime.Result, qaInvestigatorOutput, *QARepairDiagnostic, error, error) {
	run := func(req pruntime.Request) (pruntime.Result, error) {
		capture := &qaOutputCapture{}
		previousOnEvent := req.OnEvent
		req.Validation = nil
		req.OnEvent = func(event pruntime.Event) {
			capture.observe(event.Payload)
			if previousOnEvent != nil {
				previousOnEvent(event)
			}
		}
		result, err := s.startQARuntime(ctx, qaMap, req)
		if result.TerminalOutput == "" {
			result.TerminalOutput = capture.load()
		}
		return result, err
	}

	result, runErr := run(request)
	output, diagnostic, decodeErr := validateQAInvestigatorRuntimeOutput(result, qaMap.Budgets)
	if runErr != nil || decodeErr == nil || !qaRuntimePermissionsRestricted(result) || result.SessionID == "" {
		return result, output, nil, runErr, decodeErr
	}

	repair := &QARepairDiagnostic{MaxAttempts: qaMap.Budgets.OutputRepairAttempts}
	lastResult := result
	lastErr := decodeErr
	for attempt := 1; attempt <= qaMap.Budgets.OutputRepairAttempts && ctx.Err() == nil; attempt++ {
		repair.Attempted = true
		repair.AttemptCount = attempt
		repairRequest := request
		repairRequest.Metadata = cloneMetadata(request.Metadata, map[string]string{
			"operation": request.Metadata["operation"] + "-output-repair",
			"repair_of": lastResult.RunID,
		})
		repairRequest.Prompt = qaInvestigatorOutputRepairPrompt(diagnostic, lastErr)
		repairRequest.Cache = pruntime.CacheDirective{}
		repairRequest.Policy = qaNoToolPolicy()
		if attempt == 1 && lastResult.SessionID != "" {
			repairRequest.SessionID = lastResult.SessionID
			repairRequest.SessionAction = "continue"
		} else {
			repairRequest.SessionID = ""
			repairRequest.SessionAction = "fresh"
		}
		repaired, repairErr := run(repairRequest)
		result = mergeQAInvestigatorRuntimeResult(result, repaired)
		lastResult = repaired
		if repairErr != nil {
			runErr = repairErr
			lastErr = repairErr
			continue
		}
		if !qaRuntimePermissionsRestricted(repaired) {
			repair.PermissionDenied = true
			return result, qaInvestigatorOutput{}, repair, nil, errors.New("investigator output repair did not preserve restricted permissions")
		}
		output, diagnostic, decodeErr = validateQAInvestigatorRuntimeOutput(repaired, qaMap.Budgets)
		lastErr = decodeErr
		if decodeErr == nil {
			return result, output, repair, nil, nil
		}
	}
	repair.Exhausted = true
	repair.ExhaustedReason = "strict investigator output remained invalid"
	if ctx.Err() != nil {
		runErr = ctx.Err()
	}
	return result, qaInvestigatorOutput{}, repair, runErr, lastErr
}

func validateQAInvestigatorRuntimeOutput(result pruntime.Result, budgets QABudgets) (qaInvestigatorOutput, QAOutputDiagnostic, error) {
	output, diagnostic, err := decodeQAInvestigatorOutput(result, budgets.ShardOutputBytes)
	if err == nil && len(output.Theories) == 0 {
		err = errors.New("investigator output contains no falsifiable theory")
	}
	if err == nil && !withinQAInvestigatorBudgets(output, budgets) {
		err = errors.New("investigator output exceeds map-owned limits")
	}
	return output, diagnostic, err
}

func qaRuntimePermissionsRestricted(result pruntime.Result) bool {
	return result.Permissions.Mode == "restricted" && result.Permissions.Default == "deny" && result.Permissions.UnsupportedCount == 0
}

func qaInvestigatorOutputRepairPrompt(diagnostic QAOutputDiagnostic, err error) string {
	return "Return only one corrected strict QA JSON object. Do not perform more tool calls. Unknown fields and text outside the object are rejected. Required top-level fields are schema_version, theories, evidence, context_requests, and check_requests. schema_version must be 1 and the other fields must be arrays. theories must contain at least one falsifiable theory. If context is insufficient, return an inconclusive theory plus a bounded context request; an empty theories array is rejected. Rejection: " + qaValidationDiagnostic(diagnostic, err)
}

func mergeQAInvestigatorRuntimeResult(first, next pruntime.Result) pruntime.Result {
	merged := next
	merged.Events = append(append([]pruntime.Event(nil), first.Events...), next.Events...)
	merged.EventStats.Total = first.EventStats.Total + next.EventStats.Total
	merged.EventStats.Retained = first.EventStats.Retained + next.EventStats.Retained
	merged.EventStats.Dropped = first.EventStats.Dropped + next.EventStats.Dropped
	merged.Attempts = append(append([]pruntime.AttemptSummary(nil), first.Attempts...), next.Attempts...)
	merged.Warnings = append(append([]string(nil), first.Warnings...), next.Warnings...)
	merged.SessionIDs = append(append([]string(nil), first.SessionIDs...), next.SessionIDs...)
	merged.Usage = mergeQARuntimeUsage(first.Usage, next.Usage)
	merged.StartedAt = first.StartedAt
	if merged.EstimatedCost != nil && first.EstimatedCost != nil && merged.EstimatedCost.Currency == first.EstimatedCost.Currency {
		cost := *merged.EstimatedCost
		cost.Amount += first.EstimatedCost.Amount
		cost.Estimate = cost.Estimate || first.EstimatedCost.Estimate
		if cost.Source != first.EstimatedCost.Source {
			cost.Source = "mixed"
		}
		merged.EstimatedCost = &cost
	} else if first.EstimatedCost != nil || merged.EstimatedCost != nil {
		merged.EstimatedCost = nil
	}
	return merged
}

func mergeQARuntimeUsage(first, next pruntime.Usage) pruntime.Usage {
	return pruntime.Usage{
		InputTokensKnown: first.InputTokensKnown && next.InputTokensKnown, InputTokens: first.InputTokens + next.InputTokens,
		OutputTokensKnown: first.OutputTokensKnown && next.OutputTokensKnown, OutputTokens: first.OutputTokens + next.OutputTokens,
		TotalTokensKnown: first.TotalTokensKnown && next.TotalTokensKnown, TotalTokens: first.TotalTokens + next.TotalTokens,
		ReasoningTokensKnown: first.ReasoningTokensKnown && next.ReasoningTokensKnown, ReasoningTokens: first.ReasoningTokens + next.ReasoningTokens,
		CacheReadTokensKnown: first.CacheReadTokensKnown && next.CacheReadTokensKnown, CacheReadTokens: first.CacheReadTokens + next.CacheReadTokens,
		CacheWriteTokensKnown: first.CacheWriteTokensKnown && next.CacheWriteTokensKnown, CacheWriteTokens: first.CacheWriteTokens + next.CacheWriteTokens,
		TurnsKnown: first.TurnsKnown && next.TurnsKnown, Turns: first.Turns + next.Turns,
	}
}

func mergeQAUsage(left, right QAUsageSummary) QAUsageSummary {
	left.InputTokensKnown = left.InputTokensKnown && right.InputTokensKnown
	left.OutputTokensKnown = left.OutputTokensKnown && right.OutputTokensKnown
	left.TotalTokensKnown = left.TotalTokensKnown && right.TotalTokensKnown
	left.ReasoningTokensKnown = left.ReasoningTokensKnown && right.ReasoningTokensKnown
	left.CacheReadTokensKnown = left.CacheReadTokensKnown && right.CacheReadTokensKnown
	left.CacheWriteTokensKnown = left.CacheWriteTokensKnown && right.CacheWriteTokensKnown
	left.TurnsKnown = left.TurnsKnown && right.TurnsKnown
	left.InputTokens += right.InputTokens
	left.OutputTokens += right.OutputTokens
	left.TotalTokens += right.TotalTokens
	left.ReasoningTokens += right.ReasoningTokens
	left.CacheReadTokens += right.CacheReadTokens
	left.CacheWriteTokens += right.CacheWriteTokens
	left.Turns += right.Turns
	return left
}

func approveQAContextPaths(target string, paths []string) (bool, string) {
	for _, rel := range paths {
		full := filepath.Join(target, filepath.FromSlash(rel))
		resolved, err := filepath.EvalSymlinks(full)
		if err != nil {
			return false, "requested context path is unavailable"
		}
		if !inside(target, resolved) {
			return false, "requested context path escapes the QA target"
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.Mode().IsRegular() {
			return false, "requested context path is not a regular file"
		}
	}
	return true, ""
}

func qaSafeDiagnostic(value string) string {
	value = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\x00' {
			return ' '
		}
		return r
	}, value)
	if len(value) > 512 {
		value = value[:512]
	}
	return value
}

func (s Service) validateCurrentQAMap(expected QAMap) error {
	if s.qaMapFence != nil {
		if err := s.qaMapFence(expected); err != nil {
			return NewQAError(QAErrorStaleInput, "investigate shard", "governed QA inputs changed during investigation", err)
		}
		return nil
	}
	current, err := s.QAMap(expected.Project, expected.Sprint)
	if err != nil {
		return NewQAError(QAErrorStaleInput, "investigate shard", "cannot revalidate governed QA inputs", err)
	}
	if current.Map.ID != expected.ID {
		return NewQAError(QAErrorStaleInput, "investigate shard", "governed QA inputs changed during investigation", nil)
	}
	return nil
}

func decodeQAInvestigatorOutput(result pruntime.Result, limit int) (qaInvestigatorOutput, QAOutputDiagnostic, error) {
	content := result.TerminalOutput
	source := "terminal_output"
	if content == "" {
		source = "event"
		for i := len(result.Events) - 1; i >= 0; i-- {
			if value, ok := result.Events[i].Payload["content"].(string); ok && value != "" {
				content = value
				break
			}
		}
	}
	diagnostic := QAOutputDiagnostic{Source: source, OutputBytes: len(content), EventCount: len(result.Events), Status: result.Status, Session: result.SessionID != "", UsageKnown: result.Usage.InputTokensKnown || result.Usage.OutputTokensKnown || result.Usage.TotalTokensKnown || result.Usage.TurnsKnown}
	if len(content) == 0 || len(content) > limit {
		diagnostic.Kind = "empty"
		detail := "terminal output is empty"
		if len(content) > limit {
			diagnostic.Kind, detail = "too_large", "terminal output exceeds the shard output limit"
		}
		diagnostic.Detail = detail
		return qaInvestigatorOutput{}, diagnostic, NewQAError(QAErrorBudgetExhausted, "decode investigator", detail, nil)
	}
	decoder := json.NewDecoder(bytes.NewBufferString(content))
	decoder.DisallowUnknownFields()
	var output qaInvestigatorOutput
	if err := decoder.Decode(&output); err != nil {
		diagnostic.Kind, diagnostic.Detail = qaJSONFailureKind(err), qaSafeDiagnostic(err.Error())
		return qaInvestigatorOutput{}, diagnostic, NewQAError(QAErrorInvalidState, "decode investigator", "terminal output is not one strict QA JSON object", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		diagnostic.Kind, diagnostic.Detail = "trailing_json", "terminal output has trailing JSON"
		return qaInvestigatorOutput{}, diagnostic, NewQAError(QAErrorInvalidState, "decode investigator", "terminal output has trailing JSON", err)
	}
	if output.SchemaVersion != QASchemaVersion {
		diagnostic.Kind, diagnostic.Detail = "schema_version", fmt.Sprintf("unsupported investigator schema version %d", output.SchemaVersion)
		return qaInvestigatorOutput{}, diagnostic, NewQAError(QAErrorUnknownSchema, "decode investigator", diagnostic.Detail, nil)
	}
	if output.Theories == nil || output.Evidence == nil || output.Context == nil || output.Checks == nil {
		diagnostic.Kind, diagnostic.Detail = "missing_field", "all five top-level fields are required and array fields cannot be null"
		return qaInvestigatorOutput{}, diagnostic, NewQAError(QAErrorInvalidState, "decode investigator", diagnostic.Detail, nil)
	}
	return output, QAOutputDiagnostic{}, nil
}

func qaJSONFailureKind(err error) string {
	message := err.Error()
	switch {
	case strings.Contains(message, "unknown field"):
		return "unknown_field"
	case strings.Contains(message, "cannot unmarshal"):
		return "type_mismatch"
	default:
		return "syntax"
	}
}

func terminalQAState(state QAState, err error, now time.Time) QAState {
	state.Run.Lifecycle = QARunTerminal
	state.UpdatedAt = now
	state.Blocker = qaBlocker(err, "attempt")
	state.NextAction = state.Blocker.NextAction
	switch {
	case errors.Is(err, context.Canceled):
		state.Phase = QAPhaseCancelled
		state.Run.TerminalResult = QATerminalCancelled
		state.Cancellation = QACancellation{Requested: true, Scope: "attempt", Reason: "context cancelled", At: &now}
	case errors.Is(err, context.DeadlineExceeded):
		state.Phase = QAPhaseInterrupted
		state.Run.TerminalResult = QATerminalInterrupted
	default:
		state.Phase = QAPhaseBlocked
		state.Run.TerminalResult = QATerminalBlocked
	}
	return state
}

func qaRunCorrelation(token QAWriterToken, lifecycle QARunLifecycle) QARunCorrelation {
	return QARunCorrelation{Lifecycle: lifecycle, RunID: token.RunID, OperationalAttemptID: token.OperationalAttemptID, FencingGeneration: token.FencingGeneration}
}

func qaBlocker(err error, scope string) *QABlocker {
	if errors.Is(err, context.Canceled) {
		return &QABlocker{Category: QAErrorConflict, Scope: scope, Summary: "read-only QA was cancelled", NextAction: "Run qa resume to continue incomplete current shards with a new durable owner."}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &QABlocker{Category: QAErrorBudgetExhausted, Scope: scope, Summary: "read-only QA exhausted its wall-clock limit", NextAction: "Inspect retained shard outcomes, then run qa resume if the current inputs are unchanged."}
	}
	category := QAErrorRuntimeUnavailable
	next := qaRecovery(category)
	summary := err.Error()
	if typed, ok := AsQAError(err); ok {
		category = typed.Category
		next = typed.Recovery
		summary = typed.Detail
	}
	return &QABlocker{Category: category, Scope: scope, Summary: summary, NextAction: next}
}

func countTerminalQAShards(shards []QAShard) int {
	total := 0
	for _, shard := range shards {
		if shard.Phase == QAPhaseCompleted || shard.Phase == QAPhaseBlocked {
			total++
		}
	}
	return total
}

func countCompletedQAShards(shards []QAShard) int {
	total := 0
	for _, shard := range shards {
		if shard.Phase == QAPhaseCompleted {
			total++
		}
	}
	return total
}

func cloneQAOutcomeCounts(input map[QATheoryOutcome]int) map[QATheoryOutcome]int {
	result := make(map[QATheoryOutcome]int, len(input))
	for outcome, count := range input {
		result[outcome] = count
	}
	return result
}

func qaObservedToolCalls(events []pruntime.Event) int {
	count := 0
	for _, event := range events {
		kind := strings.ToLower(strings.TrimSpace(event.Kind))
		typeName := strings.ToLower(strings.TrimSpace(event.Type))
		if strings.Contains(kind, "tool") || strings.Contains(typeName, "tool_use") || strings.Contains(typeName, "tool.call") {
			count++
		}
	}
	return count
}

func qaAttemptContextMetrics(req pruntime.Request, events []pruntime.Event, duration time.Duration) QAContextMetrics {
	metric := QAContextMetrics{PromptBytes: len(req.Prompt), PrefixBytes: req.Cache.BreakpointBytes, PrefixDigest: req.Cache.PrefixDigest, CacheCohort: req.Cache.Key, Duration: duration, ToolCallsByKind: map[string]int{}}
	reads := map[string]int{}
	for _, event := range events {
		kind := strings.ToLower(strings.TrimSpace(event.Kind + " " + event.Type))
		if !strings.Contains(kind, "tool") {
			continue
		}
		name := "unknown"
		for _, key := range []string{"tool", "tool_name", "name"} {
			if value, ok := event.Payload[key].(string); ok && strings.TrimSpace(value) != "" {
				name = strings.ToLower(strings.TrimSpace(value))
				break
			}
		}
		metric.ToolCallsByKind[name]++
		if strings.Contains(name, "search") || strings.Contains(name, "grep") {
			metric.SearchCalls++
		}
		if strings.Contains(name, "read") {
			path := qaEventString(event.Payload, "path", "file", "file_path", "filePath")
			if path != "" {
				reads[path]++
			}
			metric.ReadBytes += int64(qaEventTextBytes(event.Payload))
		}
	}
	for _, count := range reads {
		metric.DistinctFilesRead++
		if count > 1 {
			metric.RepeatedReads += count - 1
		}
	}
	if len(metric.ToolCallsByKind) == 0 {
		metric.ToolCallsByKind = nil
	}
	return metric
}

func qaEventString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key].(string); ok {
			return strings.TrimSpace(value)
		}
		if nested, ok := payload["input"].(map[string]any); ok {
			if value, ok := nested[key].(string); ok {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func qaEventTextBytes(payload map[string]any) int {
	total := 0
	for _, key := range []string{"content", "output", "text"} {
		if value, ok := payload[key].(string); ok {
			total += len(value)
		}
	}
	return total
}

func emitQA(progress func(QAProgress), event QAProgress) {
	if progress != nil {
		progress(event)
	}
}

func boundedQAProgress(progress func(QAProgress), limit int) func(QAProgress) {
	if progress == nil || limit <= 0 {
		return progress
	}
	var mu sync.Mutex
	emitted := 0
	return func(event QAProgress) {
		mu.Lock()
		defer mu.Unlock()
		if emitted >= limit {
			return
		}
		emitted++
		progress(event)
	}
}

func qaUsageSummary(usage pruntime.Usage) QAUsageSummary {
	return QAUsageSummary{
		InputTokensKnown: usage.InputTokensKnown, InputTokens: usage.InputTokens,
		OutputTokensKnown: usage.OutputTokensKnown, OutputTokens: usage.OutputTokens,
		TotalTokensKnown: usage.TotalTokensKnown, TotalTokens: usage.TotalTokens,
		ReasoningTokensKnown: usage.ReasoningTokensKnown, ReasoningTokens: usage.ReasoningTokens,
		CacheReadTokensKnown: usage.CacheReadTokensKnown, CacheReadTokens: usage.CacheReadTokens,
		CacheWriteTokensKnown: usage.CacheWriteTokensKnown, CacheWriteTokens: usage.CacheWriteTokens,
		TurnsKnown: usage.TurnsKnown, Turns: usage.Turns,
	}
}

func classifyQARuntimeFailure(result pruntime.Result, err error) (string, bool) {
	if errors.Is(err, context.Canceled) {
		return "cancelled", false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline_exceeded", false
	}
	if result.Error == nil || strings.TrimSpace(result.Error.Category) == "" {
		return "runtime_error", true
	}
	kind := strings.ToLower(strings.TrimSpace(result.Error.Category))
	switch kind {
	case "validation", "invalid_input", "permission_denied", "configuration", "unsupported":
		return kind, false
	default:
		return kind, true
	}
}

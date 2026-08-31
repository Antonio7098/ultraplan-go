package sprint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	pprocess "github.com/Antonio7098/ultraplan-go/internal/platform/process"
)

const repairCampaignSchemaVersion = 1

type RepairCampaignRequest struct {
	Confirmer   string
	Budgets     RepairBudgets
	Sources     []QAEffectiveSource
	WriterToken QAWriterToken
	Progress    func(RepairCampaignProgress)
}

type RepairCampaignProgress struct {
	CampaignID string `json:"campaign_id"`
	Worker     int    `json:"worker"`
	Issue      int    `json:"issue"`
	Completed  int    `json:"completed"`
	Total      int    `json:"total"`
	Message    string `json:"message"`
}

type RepairCampaignIssue struct {
	OriginalIssueID string                    `json:"original_issue_id"`
	CurrentIssueID  string                    `json:"current_issue_id,omitempty"`
	Title           string                    `json:"title"`
	IssueClass      string                    `json:"issue_class"`
	Location        string                    `json:"location"`
	RootCauseClaim  string                    `json:"root_cause_claim"`
	RepairRunID     string                    `json:"repair_run_id,omitempty"`
	Outcome         RepairOutcome             `json:"outcome,omitempty"`
	Status          string                    `json:"status"`
	Reason          string                    `json:"reason,omitempty"`
	ProposalRuntime *RepairRuntimeObservation `json:"proposal_runtime,omitempty"`
}

type RepairCampaignWorker struct {
	Number    int                   `json:"number"`
	SessionID string                `json:"session_id,omitempty"`
	Issues    []RepairCampaignIssue `json:"issues"`
}

type RepairCampaignState struct {
	SchemaVersion int                    `json:"schema_version"`
	ID            string                 `json:"id"`
	Project       string                 `json:"project"`
	Sprint        string                 `json:"sprint"`
	Mode          string                 `json:"assignment_mode"`
	PerAgent      int                    `json:"issues_per_agent"`
	ExecutionMode string                 `json:"execution_mode,omitempty"`
	Status        string                 `json:"status"`
	Workers       []RepairCampaignWorker `json:"workers"`
	Completed     int                    `json:"completed"`
	Total         int                    `json:"total"`
	StartedAt     time.Time              `json:"started_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	CompletedAt   *time.Time             `json:"completed_at,omitempty"`
}

func QARepairCampaignRelPath(s Sprint) string {
	return strings.TrimSuffix(QARepairStateRelPath(s), "repair-state.json") + "repair-campaign.json"
}

func (s Service) RunRepairCampaign(ctx context.Context, projectRef, sprintRef string, req RepairCampaignRequest) (RepairCampaignState, error) {
	if strings.TrimSpace(req.Confirmer) == "" {
		return RepairCampaignState{}, NewQAError(QAErrorInvalidState, "run repair campaign", "a confirmer is required", nil)
	}
	if err := req.WriterToken.Validate(); err != nil {
		return RepairCampaignState{}, err
	}
	if req.Budgets == (RepairBudgets{}) {
		req.Budgets = DefaultRepairBudgets()
	}
	settings, err := s.effectiveQASettings()
	if err != nil {
		return RepairCampaignState{}, err
	}
	sp, _, _, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return RepairCampaignState{}, err
	}
	store := NewQAStore(s.root, sp).WithWriterFence(s.repairWriterFence(req.WriterToken))
	adjudication, err := currentRepairAdjudication(store)
	if err != nil {
		return RepairCampaignState{}, err
	}
	assignments, err := PlanRepairAssignments(adjudication, settings.RepairAssignmentMode, settings.IssuesPerRepairAgent)
	if err != nil {
		return RepairCampaignState{}, err
	}
	if len(assignments) == 0 {
		return RepairCampaignState{}, NewQAError(QAErrorAdmissionBlocked, "run repair campaign", "there are no current repair-eligible issues", nil)
	}
	state, err := newRepairCampaignState(sp, adjudication, assignments, settings, s.now().UTC())
	if err != nil {
		return RepairCampaignState{}, err
	}
	if err := store.publishRepairCampaign(state, req.WriterToken); err != nil {
		return RepairCampaignState{}, err
	}
	workerParent, err := os.MkdirTemp("", "ultraplan-repair-campaign-")
	if err != nil {
		return finishRepairCampaignFailure(store, state, req.WriterToken, err)
	}
	defer os.RemoveAll(workerParent)
	if settings.RepairExecutionMode == "parallel" {
		return s.runParallelRepairCampaign(ctx, projectRef, sprintRef, req, settings, store, state, workerParent)
	}

	authorized := false
	for wi := range state.Workers {
		workerRoot := filepath.Join(workerParent, fmt.Sprintf("worker-%d", state.Workers[wi].Number))
		if err := os.Mkdir(workerRoot, 0o700); err != nil {
			return finishRepairCampaignFailure(store, state, req.WriterToken, err)
		}
		for ii := range state.Workers[wi].Issues {
			if err := ctx.Err(); err != nil {
				state.Status = "cancelled"
				state.UpdatedAt = s.now().UTC()
				_ = store.publishRepairCampaign(state, req.WriterToken)
				return state, err
			}
			item := &state.Workers[wi].Issues[ii]
			current, loadErr := currentRepairAdjudication(store)
			if loadErr != nil {
				return finishRepairCampaignFailure(store, state, req.WriterToken, loadErr)
			}
			issue := matchCampaignIssue(current, *item)
			if issue == nil {
				item.Status, item.Reason = "resolved", "no matching issue remains in the fresh adjudication"
				state.Completed++
				state.UpdatedAt = s.now().UTC()
				if err := store.publishRepairCampaign(state, req.WriterToken); err != nil {
					return state, err
				}
				continue
			}
			if ii > 0 && strings.TrimSpace(state.Workers[wi].SessionID) == "" {
				return finishRepairCampaignFailure(store, state, req.WriterToken, NewQAError(QAErrorRuntimeUnavailable, "continue repair campaign worker", "the runtime did not retain the worker session required for a multi-issue queue", nil))
			}
			item.CurrentIssueID, item.Status = issue.ID, "preparing"
			state.UpdatedAt = s.now().UTC()
			if err := store.publishRepairCampaign(state, req.WriterToken); err != nil {
				return state, err
			}
			emitRepairCampaign(req.Progress, state, wi, ii, "Freezing one issue packet")
			prepared, runErr := s.PrepareRepair(ctx, projectRef, sprintRef, RepairPrepareRequest{IssueID: issue.ID, Mode: RepairModeAutomatic, Budgets: req.Budgets, BudgetSources: req.Sources, WriterToken: req.WriterToken, campaignAuthorized: authorized})
			if runErr != nil {
				return finishRepairCampaignFailure(store, state, req.WriterToken, runErr)
			}
			item.RepairRunID, item.Status = prepared.Packet.RepairRunID, "prepared"
			state.UpdatedAt = s.now().UTC()
			if err := store.publishRepairCampaign(state, req.WriterToken); err != nil {
				return state, err
			}
			_, runErr = s.ConfirmRepair(ctx, projectRef, sprintRef, RepairConfirmRequest{RepairRunID: prepared.Packet.RepairRunID, Confirmer: req.Confirmer, AutomaticOptIn: true, WriterToken: req.WriterToken, campaignAuthorized: authorized})
			if runErr != nil {
				return finishRepairCampaignFailure(store, state, req.WriterToken, runErr)
			}
			item.Status = "confirmed"
			state.UpdatedAt = s.now().UTC()
			if err := store.publishRepairCampaign(state, req.WriterToken); err != nil {
				return state, err
			}
			authorized = true
			emitRepairCampaign(req.Progress, state, wi, ii, "Running isolated issue repair")
			intermediate := state.Completed+1 < state.Total
			result, runErr := s.RunRepair(ctx, projectRef, sprintRef, RepairRunRequest{RepairRunID: prepared.Packet.RepairRunID, WriterToken: req.WriterToken, SessionID: state.Workers[wi].SessionID, WorkerNumber: state.Workers[wi].Number, WorkerQueueSize: len(state.Workers[wi].Issues), WorkerRoot: workerRoot, campaignAuthorized: true, campaignIntermediate: intermediate})
			item.Outcome = result.Outcome
			if result.Runtime != nil {
				state.Workers[wi].SessionID = result.Runtime.SessionID
			}
			acceptable := result.Outcome == RepairOutcomeVerified || result.Outcome == RepairOutcomeVerifiedWithFindings || intermediate && result.Outcome == RepairOutcomeCampaignPending
			if runErr != nil || !acceptable {
				item.Status, item.Reason = "failed", strings.TrimSpace(result.Reason)
				if runErr == nil {
					runErr = fmt.Errorf("repair %s ended with %s", result.RepairRunID, result.Outcome)
				}
				return finishRepairCampaignFailure(store, state, req.WriterToken, runErr)
			}
			item.Status = "verified"
			state.Completed++
			state.UpdatedAt = s.now().UTC()
			if err := store.publishRepairCampaign(state, req.WriterToken); err != nil {
				return state, err
			}
		}
	}
	now := s.now().UTC()
	state.Status, state.UpdatedAt, state.CompletedAt = "completed", now, &now
	if err := store.publishRepairCampaign(state, req.WriterToken); err != nil {
		return state, err
	}
	return state, nil
}

type campaignPreparedProposal struct {
	Proposal     []byte
	Replacements map[string][]byte
	Preimages    map[string]string
	ChangedPaths []string
	ChangedBytes int64
	Runtime      RepairRuntimeObservation
}

func (s Service) runParallelRepairCampaign(ctx context.Context, projectRef, sprintRef string, req RepairCampaignRequest, settings QASettings, store QAStore, state RepairCampaignState, workerParent string) (RepairCampaignState, error) {
	// Parallel proposal work still requires the same automatic-repair proof as
	// the sequential path. This check happens before any model is dispatched.
	if err := s.RequireAutomaticRepairProof(projectRef, sprintRef); err != nil {
		return finishRepairCampaignFailure(store, state, req.WriterToken, err)
	}
	packets := make([][]*RepairIssuePacket, len(state.Workers))
	proposals := make([][]*campaignPreparedProposal, len(state.Workers))
	for wi := range state.Workers {
		packets[wi] = make([]*RepairIssuePacket, len(state.Workers[wi].Issues))
		proposals[wi] = make([]*campaignPreparedProposal, len(state.Workers[wi].Issues))
		workerRoot := filepath.Join(workerParent, fmt.Sprintf("worker-%d", state.Workers[wi].Number))
		if err := os.Mkdir(workerRoot, 0o700); err != nil {
			return finishRepairCampaignFailure(store, state, req.WriterToken, err)
		}
		for ii := range state.Workers[wi].Issues {
			if err := ctx.Err(); err != nil {
				state.Status, state.UpdatedAt = "cancelled", s.now().UTC()
				_ = store.publishRepairCampaign(state, req.WriterToken)
				return state, err
			}
			item := &state.Workers[wi].Issues[ii]
			current, loadErr := currentRepairAdjudication(store)
			if loadErr != nil {
				return finishRepairCampaignFailure(store, state, req.WriterToken, loadErr)
			}
			issue := matchCampaignIssue(current, *item)
			if issue == nil {
				item.Status, item.Reason = "resolved", "no matching issue remains in the fresh adjudication"
				state.Completed++
				continue
			}
			item.CurrentIssueID, item.Status = issue.ID, "preparing"
			emitRepairCampaign(req.Progress, state, wi, ii, "Freezing one issue packet for parallel proposal work")
			prepared, prepareErr := s.PrepareRepair(ctx, projectRef, sprintRef, RepairPrepareRequest{IssueID: issue.ID, Mode: RepairModeAutomatic, Budgets: req.Budgets, BudgetSources: req.Sources, WriterToken: req.WriterToken, campaignAuthorized: true})
			if prepareErr != nil {
				return finishRepairCampaignFailure(store, state, req.WriterToken, prepareErr)
			}
			packet := prepared.Packet
			packets[wi][ii] = &packet
			item.RepairRunID, item.Status = packet.RepairRunID, "prepared"
		}
	}
	for wi := range state.Workers {
		for ii := range state.Workers[wi].Issues {
			if packets[wi][ii] != nil {
				state.Workers[wi].Issues[ii].Status = "proposing"
			}
		}
	}
	state.UpdatedAt = s.now().UTC()
	if err := store.publishRepairCampaign(state, req.WriterToken); err != nil {
		return state, err
	}

	parallelism := settings.Budgets.ConcurrentInvestigators
	if parallelism < 1 {
		parallelism = 1
	}
	if parallelism > len(state.Workers) {
		parallelism = len(state.Workers)
	}
	sem := make(chan struct{}, parallelism)
	errs := make([]error, len(state.Workers))
	var wg sync.WaitGroup
	for wi := range state.Workers {
		wi := wi
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			workerRoot := filepath.Join(workerParent, fmt.Sprintf("worker-%d", state.Workers[wi].Number))
			sessionID := ""
			for ii, packet := range packets[wi] {
				if packet == nil {
					continue
				}
				proposal, proposalErr := s.generateCampaignProposal(ctx, *packet, workerRoot, sessionID, state.Workers[wi].Number, len(state.Workers[wi].Issues))
				if proposalErr != nil {
					errs[wi] = proposalErr
					return
				}
				proposals[wi][ii] = proposal
				sessionID = proposal.Runtime.SessionID
			}
		}()
	}
	wg.Wait()
	for wi, proposalErr := range errs {
		if proposalErr != nil {
			for ii := range state.Workers[wi].Issues {
				if proposals[wi][ii] == nil && packets[wi][ii] != nil {
					state.Workers[wi].Issues[ii].Status, state.Workers[wi].Issues[ii].Reason = "failed", safeError(proposalErr)
					break
				}
			}
			return finishRepairCampaignFailure(store, state, req.WriterToken, proposalErr)
		}
		for ii, proposal := range proposals[wi] {
			if proposal != nil {
				state.Workers[wi].Issues[ii].Status = "proposed"
				observation := proposal.Runtime
				state.Workers[wi].Issues[ii].ProposalRuntime = &observation
				state.Workers[wi].SessionID = proposal.Runtime.SessionID
			}
		}
	}
	state.UpdatedAt = s.now().UTC()
	if err := store.publishRepairCampaign(state, req.WriterToken); err != nil {
		return state, err
	}

	// Integration remains ordered and single-writer. A proposal whose file
	// preimages became stale is regenerated by RunRepair against the fresh
	// packet instead of being merged blindly.
	for wi := range state.Workers {
		workerRoot := filepath.Join(workerParent, fmt.Sprintf("worker-%d", state.Workers[wi].Number))
		for ii, preparedProposal := range proposals[wi] {
			if preparedProposal == nil {
				continue
			}
			item := &state.Workers[wi].Issues[ii]
			current, loadErr := currentRepairAdjudication(store)
			if loadErr != nil {
				return finishRepairCampaignFailure(store, state, req.WriterToken, loadErr)
			}
			issue := matchCampaignIssue(current, *item)
			if issue == nil {
				item.Status, item.Reason = "resolved", "no matching issue remains at the integration boundary"
				state.Completed++
				continue
			}
			prepared, prepareErr := s.PrepareRepair(ctx, projectRef, sprintRef, RepairPrepareRequest{IssueID: issue.ID, Mode: RepairModeAutomatic, Budgets: req.Budgets, BudgetSources: req.Sources, WriterToken: req.WriterToken, campaignAuthorized: true})
			if prepareErr != nil {
				return finishRepairCampaignFailure(store, state, req.WriterToken, prepareErr)
			}
			item.CurrentIssueID, item.RepairRunID, item.Status = issue.ID, prepared.Packet.RepairRunID, "prepared"
			if _, confirmErr := s.ConfirmRepair(ctx, projectRef, sprintRef, RepairConfirmRequest{RepairRunID: prepared.Packet.RepairRunID, Confirmer: req.Confirmer, AutomaticOptIn: true, WriterToken: req.WriterToken, campaignAuthorized: true}); confirmErr != nil {
				return finishRepairCampaignFailure(store, state, req.WriterToken, confirmErr)
			}
			item.Status = "confirmed"
			intermediate := state.Completed+1 < state.Total
			emitRepairCampaign(req.Progress, state, wi, ii, "Integrating isolated repair proposal")
			result, runErr := s.RunRepair(ctx, projectRef, sprintRef, RepairRunRequest{RepairRunID: prepared.Packet.RepairRunID, WriterToken: req.WriterToken, SessionID: state.Workers[wi].SessionID, WorkerNumber: state.Workers[wi].Number, WorkerQueueSize: len(state.Workers[wi].Issues), WorkerRoot: workerRoot, campaignAuthorized: true, campaignIntermediate: intermediate, preparedProposal: preparedProposal})
			item.Outcome = result.Outcome
			if result.Runtime != nil {
				state.Workers[wi].SessionID = result.Runtime.SessionID
			}
			acceptable := result.Outcome == RepairOutcomeVerified || result.Outcome == RepairOutcomeVerifiedWithFindings || intermediate && result.Outcome == RepairOutcomeCampaignPending
			if runErr != nil || !acceptable {
				item.Status, item.Reason = "failed", strings.TrimSpace(result.Reason)
				if runErr == nil {
					runErr = fmt.Errorf("repair %s ended with %s", result.RepairRunID, result.Outcome)
				}
				return finishRepairCampaignFailure(store, state, req.WriterToken, runErr)
			}
			item.Status = "verified"
			state.Completed++
			state.UpdatedAt = s.now().UTC()
			if err := store.publishRepairCampaign(state, req.WriterToken); err != nil {
				return state, err
			}
		}
	}
	now := s.now().UTC()
	state.Status, state.UpdatedAt, state.CompletedAt = "completed", now, &now
	if err := store.publishRepairCampaign(state, req.WriterToken); err != nil {
		return state, err
	}
	return state, nil
}

func (s Service) generateCampaignProposal(ctx context.Context, packet RepairIssuePacket, workerRoot, sessionID string, workerNumber, queueSize int) (*campaignPreparedProposal, error) {
	runtime := s.repairRuntime
	if runtime == nil {
		runtime = s.runtime
	}
	if runtime == nil {
		return nil, NewQAError(QAErrorRuntimeUnavailable, "parallel repair proposal", "a repair runtime is required", nil)
	}
	manifest, _, err := s.PrepareReview(packet.Project, packet.Sprint, ReviewRequest{})
	if err != nil {
		return nil, err
	}
	identity, err := repairTargetIdentity(manifest.Target)
	if err != nil || identity.Fingerprint != packet.Target.Fingerprint {
		return nil, NewQAError(QAErrorStaleInput, "parallel repair proposal", "target changed before isolated proposal", err)
	}
	limits := pprocess.IsolationLimits{MaxFiles: MaximumQABudgets().TreeFiles, MaxBytes: MaximumQABudgets().TreeBytes, MaxFileSize: MaximumQABudgets().FileBytes, Timeout: packet.Budgets.WallTime}
	workspace, err := pprocess.CreateIsolation(ctx, pprocess.IsolationRequest{SourceRoot: manifest.Target, ParentDir: workerRoot, Prefix: packet.RepairRunID, Destination: filepath.Join(workerRoot, "workspace"), ProtectedRoots: []string{s.root, manifest.Target}, Limits: limits})
	if err != nil {
		return nil, err
	}
	request, err := s.repairProposalRequest(packet, workspace.Path)
	if err != nil {
		return nil, errors.Join(err, cleanupError(workspace.Cleanup()))
	}
	if strings.TrimSpace(sessionID) != "" {
		request.SessionID, request.SessionAction = strings.TrimSpace(sessionID), "continue"
		request.Metadata["repair_worker_session"] = "continue"
	} else {
		request.Metadata["repair_worker_session"] = "fresh"
	}
	request.Metadata["repair_worker"], request.Metadata["repair_worker_queue_size"] = fmt.Sprint(workerNumber), fmt.Sprint(queueSize)
	started := s.now().UTC()
	runtimeResult, runErr := runtime.StartRun(ctx, request)
	if sp, sprintErr := s.resolveMutationSprint(packet.Project, packet.Sprint); sprintErr == nil {
		if metricErr := s.recordRuntimeMetric(sp, PlanningStage(VerificationPhaseRepair), request, runtimeResult); metricErr != nil {
			runtimeResult.Warnings = append(runtimeResult.Warnings, "runtime metrics were not persisted: "+safeError(metricErr))
			runErr = errors.Join(runErr, fmt.Errorf("persist required repair runtime metrics: %w", metricErr))
		}
	} else {
		runtimeResult.Warnings = append(runtimeResult.Warnings, "runtime metrics were not persisted: "+safeError(sprintErr))
		runErr = errors.Join(runErr, fmt.Errorf("resolve required repair runtime metrics ledger: %w", sprintErr))
	}
	completed := s.now().UTC()
	if runErr != nil || ctx.Err() != nil {
		return nil, errors.Join(runErr, ctx.Err(), cleanupError(workspace.Cleanup()))
	}
	changedPaths, err := pprocess.CompareTrees(context.WithoutCancel(ctx), manifest.Target, workspace.Path, limits)
	if err != nil || len(changedPaths) == 0 {
		return nil, errors.Join(fmt.Errorf("parallel isolated proposal has no complete bounded diff"), err, cleanupError(workspace.Cleanup()))
	}
	proposal, replacements, preimages, changedBytes, err := deriveRepairProposal(manifest.Target, workspace.Path, changedPaths, packet)
	if err != nil {
		return nil, errors.Join(err, cleanupError(workspace.Cleanup()))
	}
	checks, checksPassed := s.runRepairProvisionalChecks(ctx, packet, workspace.Path)
	cleanup := workspace.Cleanup()
	if !checksPassed || !cleanup.Complete {
		if !checksPassed {
			err = NewQAError(QAErrorInvalidState, "parallel repair proposal", "isolated proposal did not pass its frozen provisional checks", nil)
		}
		return nil, errors.Join(err, cleanupError(cleanup))
	}
	runtimeEvents := runtimeResult.EventStats.Total
	if runtimeEvents == 0 {
		runtimeEvents = int64(len(runtimeResult.Events))
	}
	observation := RepairRuntimeObservation{Provider: request.Provider, Model: request.Model, Variant: request.Metadata["variant"], SessionID: runtimeResult.SessionID, Usage: qaUsageSummary(runtimeResult.Usage), StartedAt: started, CompletedAt: completed, Duration: completed.Sub(started), DurationMS: completed.Sub(started).Milliseconds(), RuntimeEvents: runtimeEvents, RetainedEvents: len(runtimeResult.Events), ObservedToolCalls: qaObservedToolCalls(runtimeResult.Events), ProvisionalChecks: checks, ProvisionalPassed: checksPassed}
	if runtimeResult.EstimatedCost != nil && runtimeResult.EstimatedCost.Source != "unpriced" && (runtimeResult.EstimatedCost.Source != "" || runtimeResult.EstimatedCost.Amount != 0) {
		observation.EstimatedCost = &QACostSummary{Amount: runtimeResult.EstimatedCost.Amount, Currency: runtimeResult.EstimatedCost.Currency, Estimate: runtimeResult.EstimatedCost.Estimate, Source: runtimeResult.EstimatedCost.Source}
	}
	return &campaignPreparedProposal{Proposal: proposal, Replacements: replacements, Preimages: preimages, ChangedPaths: changedPaths, ChangedBytes: changedBytes, Runtime: observation}, nil
}

func campaignPreparedProposalCurrent(target string, packet RepairIssuePacket, prepared *campaignPreparedProposal) bool {
	if prepared == nil || len(prepared.Proposal) == 0 || len(prepared.Proposal) > packet.Budgets.MaxPatchBytes || len(prepared.ChangedPaths) == 0 || len(prepared.ChangedPaths) > packet.Budgets.MaxFilesPerCycle || prepared.ChangedBytes > packet.Budgets.MaxBytesPerCycle || !sameRepairPaths(prepared.ChangedPaths, mapKeys(prepared.Replacements)) || !repairPathsAllowed(prepared.ChangedPaths, packet.AllowedPaths) {
		return false
	}
	for _, rel := range prepared.ChangedPaths {
		path := filepath.Join(target, filepath.FromSlash(rel))
		if ensureRepairRegularPath(target, path) != nil {
			return false
		}
		current, err := os.ReadFile(path)
		if err != nil || hashBytes(current) != prepared.Preimages[rel] {
			return false
		}
	}
	return true
}

func cloneRepairReplacements(input map[string][]byte) map[string][]byte {
	result := make(map[string][]byte, len(input))
	for path, content := range input {
		result[path] = append([]byte(nil), content...)
	}
	return result
}

func cloneRepairPreimages(input map[string]string) map[string]string {
	result := make(map[string]string, len(input))
	for path, digest := range input {
		result[path] = digest
	}
	return result
}

func currentRepairAdjudication(store QAStore) (QAAdjudication, error) {
	state, err := store.LoadState()
	if err != nil || !state.Freshness.Current || state.Phase != QAPhaseCompleted {
		return QAAdjudication{}, NewQAError(QAErrorStaleInput, "load repair campaign queue", "current completed QA is required", err)
	}
	qaMap, err := store.LoadMap(state.CurrentAttemptID)
	if err != nil {
		return QAAdjudication{}, err
	}
	return store.LoadAdjudication(state.CurrentAttemptID, qaMap.Budgets)
}

func newRepairCampaignState(sp Sprint, adjudication QAAdjudication, assignments []QARepairAssignment, settings QASettings, now time.Time) (RepairCampaignState, error) {
	issueByID := make(map[string]QAIssue, len(adjudication.Issues))
	groupByID := make(map[string]QARootCauseGroup, len(adjudication.Groups))
	for _, issue := range adjudication.Issues {
		issueByID[issue.ID] = issue
	}
	for _, group := range adjudication.Groups {
		groupByID[group.ID] = group
	}
	workers := make([]RepairCampaignWorker, 0, len(assignments))
	total := 0
	for _, assignment := range assignments {
		worker := RepairCampaignWorker{Number: assignment.Agent}
		for _, id := range assignment.Issues {
			issue := issueByID[id]
			group := groupByID[issue.RootCauseGroupID]
			worker.Issues = append(worker.Issues, RepairCampaignIssue{OriginalIssueID: id, Title: issue.Title, IssueClass: issue.IssueClass, Location: issue.Location, RootCauseClaim: group.Claim, Status: "queued"})
			total++
		}
		workers = append(workers, worker)
	}
	executionMode := settings.RepairExecutionMode
	if executionMode == "" {
		executionMode = "sequential"
	}
	digest := sha256.Sum256([]byte(strings.Join([]string{sp.Project, sp.Slug, adjudication.ID, settings.RepairAssignmentMode, fmt.Sprint(settings.IssuesPerRepairAgent), executionMode, now.UTC().Format(time.RFC3339Nano)}, "\x00")))
	return RepairCampaignState{SchemaVersion: repairCampaignSchemaVersion, ID: "repair-campaign-v1-" + hex.EncodeToString(digest[:12]), Project: sp.Project, Sprint: sp.Slug, Mode: settings.RepairAssignmentMode, PerAgent: settings.IssuesPerRepairAgent, ExecutionMode: executionMode, Status: "running", Workers: workers, Total: total, StartedAt: now, UpdatedAt: now}, nil
}

func matchCampaignIssue(adjudication QAAdjudication, queued RepairCampaignIssue) *QAIssue {
	groups := make(map[string]QARootCauseGroup, len(adjudication.Groups))
	for _, group := range adjudication.Groups {
		groups[group.ID] = group
	}
	issues := append([]QAIssue(nil), adjudication.Issues...)
	sort.Slice(issues, func(i, j int) bool { return issues[i].ID < issues[j].ID })
	for i := range issues {
		group := groups[issues[i].RootCauseGroupID]
		if issues[i].RepairEligible && strings.EqualFold(issues[i].IssueClass, queued.IssueClass) && normalizeIssueLocation(issues[i].Location) == normalizeIssueLocation(queued.Location) && strings.EqualFold(strings.TrimSpace(group.Claim), strings.TrimSpace(queued.RootCauseClaim)) {
			return &issues[i]
		}
	}
	return nil
}

func (store QAStore) publishRepairCampaign(state RepairCampaignState, token QAWriterToken) error {
	if err := ValidateRepairCampaignState(state); err != nil {
		return NewQAError(QAErrorInvalidState, "publish repair campaign", err.Error(), err)
	}
	path, err := store.resolve(QARepairCampaignRelPath(store.sprint))
	if err != nil {
		return err
	}
	_, err = store.writeRepairRecord(token, "repair-campaign", path, state, false)
	return err
}

func (store QAStore) LoadRepairCampaign() (RepairCampaignState, error) {
	path, err := store.resolve(QARepairCampaignRelPath(store.sprint))
	if err != nil {
		return RepairCampaignState{}, err
	}
	var state RepairCampaignState
	if err := store.readStrictVersion(path, "repair-campaign", repairCampaignSchemaVersion, &state); err != nil {
		return RepairCampaignState{}, err
	}
	if state.Project != store.sprint.Project || state.Sprint != store.sprint.Slug || ValidateRepairCampaignState(state) != nil {
		return RepairCampaignState{}, NewQAError(QAErrorInvalidState, "load repair campaign", "repair campaign state is invalid", nil)
	}
	return state, nil
}

func ValidateRepairCampaignState(state RepairCampaignState) error {
	if state.SchemaVersion != repairCampaignSchemaVersion || !strings.HasPrefix(state.ID, "repair-campaign-v1-") || !safeQAName(state.Project) || !safeQAName(state.Sprint) || state.Mode != "per_issue" && state.Mode != "grouped" || state.ExecutionMode != "" && state.ExecutionMode != "sequential" && state.ExecutionMode != "parallel" || state.PerAgent < 1 || state.PerAgent > 16 || len(state.Workers) == 0 || state.Total < 1 || state.Completed < 0 || state.Completed > state.Total || state.StartedAt.IsZero() || state.UpdatedAt.IsZero() {
		return fmt.Errorf("invalid repair campaign identity or totals")
	}
	if state.Status != "running" && state.Status != "completed" && state.Status != "failed" && state.Status != "cancelled" {
		return fmt.Errorf("invalid repair campaign status")
	}
	count := 0
	for index, worker := range state.Workers {
		if worker.Number != index+1 || len(worker.SessionID) > 512 || len(worker.Issues) == 0 || len(worker.Issues) > state.PerAgent {
			return fmt.Errorf("invalid repair campaign worker")
		}
		for _, issue := range worker.Issues {
			if strings.TrimSpace(issue.OriginalIssueID) == "" || strings.TrimSpace(issue.Title) == "" || strings.TrimSpace(issue.IssueClass) == "" || strings.TrimSpace(issue.RootCauseClaim) == "" || issue.Status != "queued" && issue.Status != "preparing" && issue.Status != "prepared" && issue.Status != "proposing" && issue.Status != "proposed" && issue.Status != "confirmed" && issue.Status != "verified" && issue.Status != "resolved" && issue.Status != "failed" {
				return fmt.Errorf("invalid repair campaign issue")
			}
			count++
		}
	}
	if count != state.Total || state.Status == "completed" && state.Completed != state.Total || state.Status == "completed" && state.CompletedAt == nil {
		return fmt.Errorf("repair campaign totals are inconsistent")
	}
	return nil
}

func finishRepairCampaignFailure(store QAStore, state RepairCampaignState, token QAWriterToken, cause error) (RepairCampaignState, error) {
	state.Status, state.UpdatedAt = "failed", time.Now().UTC()
	_ = store.publishRepairCampaign(state, token)
	return state, cause
}

func emitRepairCampaign(progress func(RepairCampaignProgress), state RepairCampaignState, worker, issue int, message string) {
	if progress != nil {
		progress(RepairCampaignProgress{CampaignID: state.ID, Worker: worker + 1, Issue: issue + 1, Completed: state.Completed, Total: state.Total, Message: message})
	}
}

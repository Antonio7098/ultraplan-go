package sprint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
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
	OriginalIssueID string        `json:"original_issue_id"`
	CurrentIssueID  string        `json:"current_issue_id,omitempty"`
	Title           string        `json:"title"`
	IssueClass      string        `json:"issue_class"`
	Location        string        `json:"location"`
	RootCauseClaim  string        `json:"root_cause_claim"`
	RepairRunID     string        `json:"repair_run_id,omitempty"`
	Outcome         RepairOutcome `json:"outcome,omitempty"`
	Status          string        `json:"status"`
	Reason          string        `json:"reason,omitempty"`
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

	authorized := false
	for wi := range state.Workers {
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
			emitRepairCampaign(req.Progress, state, wi, ii, "Freezing one issue packet")
			prepared, runErr := s.PrepareRepair(ctx, projectRef, sprintRef, RepairPrepareRequest{IssueID: issue.ID, Mode: RepairModeAutomatic, Budgets: req.Budgets, BudgetSources: req.Sources, WriterToken: req.WriterToken, campaignAuthorized: authorized})
			if runErr != nil {
				return finishRepairCampaignFailure(store, state, req.WriterToken, runErr)
			}
			item.RepairRunID, item.Status = prepared.Packet.RepairRunID, "confirmed"
			_, runErr = s.ConfirmRepair(ctx, projectRef, sprintRef, RepairConfirmRequest{RepairRunID: prepared.Packet.RepairRunID, Confirmer: req.Confirmer, AutomaticOptIn: true, WriterToken: req.WriterToken, campaignAuthorized: authorized})
			if runErr != nil {
				return finishRepairCampaignFailure(store, state, req.WriterToken, runErr)
			}
			authorized = true
			emitRepairCampaign(req.Progress, state, wi, ii, "Running isolated issue repair")
			result, runErr := s.RunRepair(ctx, projectRef, sprintRef, RepairRunRequest{RepairRunID: prepared.Packet.RepairRunID, WriterToken: req.WriterToken, SessionID: state.Workers[wi].SessionID, WorkerNumber: state.Workers[wi].Number, WorkerQueueSize: len(state.Workers[wi].Issues)})
			item.Outcome = result.Outcome
			if result.Runtime != nil {
				state.Workers[wi].SessionID = result.Runtime.SessionID
			}
			if runErr != nil || result.Outcome != RepairOutcomeVerified && result.Outcome != RepairOutcomeVerifiedWithFindings {
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
			emitRepairCampaign(req.Progress, state, wi, ii, "Refreshing QA before the next issue")
			runErr = s.refreshRepairCampaignAuthority(ctx, projectRef, sprintRef, req.WriterToken)
			if runErr != nil {
				return finishRepairCampaignFailure(store, state, req.WriterToken, runErr)
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

func (s Service) refreshRepairCampaignAuthority(ctx context.Context, projectRef, sprintRef string, token QAWriterToken) error {
	review, err := s.Review(ctx, projectRef, sprintRef, ReviewRequest{})
	if err != nil || review.Status != ReviewCompleted || review.Verdict != ReviewPass && review.Verdict != ReviewPassWithFindings {
		return NewQAError(QAErrorAdmissionBlocked, "refresh repair campaign", "post-repair Conformance Review did not pass", err)
	}
	smoke, err := s.RunSmoke(ctx, projectRef, sprintRef, SmokeRequest{NonInteractive: true})
	if err != nil || smoke.Status != SmokeCompleted || smoke.Verdict != SmokePass && smoke.Verdict != SmokePassWithOpenIssues {
		return NewQAError(QAErrorAdmissionBlocked, "refresh repair campaign", "post-repair containing smoke did not pass", err)
	}
	qa, err := s.RunQA(ctx, projectRef, sprintRef, QARunRequest{EvidenceProducing: true, WriterToken: token})
	if err != nil || qa.State.Phase != QAPhaseCompleted {
		return NewQAError(QAErrorAdmissionBlocked, "refresh repair campaign", "post-repair evidence-producing QA did not complete", err)
	}
	return nil
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
	digest := sha256.Sum256([]byte(strings.Join([]string{sp.Project, sp.Slug, adjudication.ID, settings.RepairAssignmentMode, fmt.Sprint(settings.IssuesPerRepairAgent), now.UTC().Format(time.RFC3339Nano)}, "\x00")))
	return RepairCampaignState{SchemaVersion: repairCampaignSchemaVersion, ID: "repair-campaign-v1-" + hex.EncodeToString(digest[:12]), Project: sp.Project, Sprint: sp.Slug, Mode: settings.RepairAssignmentMode, PerAgent: settings.IssuesPerRepairAgent, Status: "running", Workers: workers, Total: total, StartedAt: now, UpdatedAt: now}, nil
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
	if state.SchemaVersion != repairCampaignSchemaVersion || !strings.HasPrefix(state.ID, "repair-campaign-v1-") || !safeQAName(state.Project) || !safeQAName(state.Sprint) || state.Mode != "per_issue" && state.Mode != "grouped" || state.PerAgent < 1 || state.PerAgent > 16 || len(state.Workers) == 0 || state.Total < 1 || state.Completed < 0 || state.Completed > state.Total || state.StartedAt.IsZero() || state.UpdatedAt.IsZero() {
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
			if strings.TrimSpace(issue.OriginalIssueID) == "" || strings.TrimSpace(issue.Title) == "" || strings.TrimSpace(issue.IssueClass) == "" || strings.TrimSpace(issue.RootCauseClaim) == "" || issue.Status != "queued" && issue.Status != "preparing" && issue.Status != "confirmed" && issue.Status != "verified" && issue.Status != "resolved" && issue.Status != "failed" {
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

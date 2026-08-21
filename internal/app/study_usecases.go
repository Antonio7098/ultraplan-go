package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/study"
	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

type StudySummary struct {
	Name        string
	Sources     []string
	Dimensions  []string
	Status      string
	RunID       string
	StatePath   string
	Total       int
	Completed   int
	Failed      int
	RunActive   bool
	RunStatus   string
	ActiveTasks int
	Pending     int
	Cancelled   int
	Retries     study.RetrySummary
	Tasks       []RunTaskSummary
	Findings    []DisplayFinding
	Artifacts   []DisplayArtifact
}

type RunTaskSummary struct {
	ID, Kind, Dimension, Source, Status, Provider, Model, Duration                string
	Attempts, RuntimeAttempts                                                     int
	Retries                                                                       int
	SessionReuse                                                                  string
	ErrorCode, Error                                                              string
	Turns                                                                         int64
	TurnsKnown                                                                    bool
	Tokens                                                                        int64
	TokensKnown                                                                   bool
	InputTokens, OutputTokens, ReasoningTokens, CacheReadTokens, CacheWriteTokens int64
	Events                                                                        int64
	Cost                                                                          string
	DurationMS                                                                    int64
}

func (u dashboardUseCases) StudySummaries(ctx context.Context) ([]StudySummary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	service := study.NewService(u.root)
	studies, err := service.ListStudies()
	if err != nil {
		return nil, mapStudyError(err)
	}
	out := make([]StudySummary, 0, len(studies))
	for _, st := range studies {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		listing, err := service.ListStudy(st.Name)
		if err != nil {
			return nil, mapStudyError(err)
		}
		summary := StudySummary{Name: st.Name, Status: "run-state missing"}
		for _, source := range listing.Sources {
			summary.Sources = append(summary.Sources, source.Name)
		}
		for _, dim := range listing.Dimensions {
			summary.Dimensions = append(summary.Dimensions, dim.Number+"-"+dim.Slug)
		}
		// Planned runs are derived from the same applicability-aware task graph as
		// run-loop, even before a durable run state exists.
		if planned, planErr := study.NewRunState(study.NewRunStateRequest{
			WorkspaceRoot: u.root, Study: listing.Study, Sources: listing.Sources,
			Dimensions: listing.Dimensions, RunID: "tui-preview", Now: time.Now().UTC(),
		}); planErr == nil {
			summary.Total = len(planned.Tasks)
		}
		if state, err := study.LoadRunState(listing.Study); err == nil {
			now := time.Now().UTC()
			study.ReconcileRunState(&state, u.root, listing.Study, listing.Sources, listing.Dimensions, now)
			status := study.SummarizeRunState(state, study.RunStatePath(listing.Study))
			summary.Status = "complete=false"
			if status.Complete {
				summary.Status = "complete=true"
			}
			summary.RunID = status.RunID
			summary.StatePath = workspace.Rel(u.root, status.StatePath)
			summary.Total = status.Total
			summary.Completed = status.Completed
			summary.Failed = status.Failed
			summary.ActiveTasks = status.Active
			summary.Pending = status.Pending + status.Waiting + status.Retrying
			summary.Cancelled = status.Cancelled
			summary.Retries = study.SummarizeRetries(state)
			for _, task := range state.Tasks {
				summary.Tasks = append(summary.Tasks, runTaskSummary(task, now))
			}
		} else if !errors.Is(err, study.ErrRunStateMissing) {
			summary.Status = displaySafe(err.Error())
		}
		if active, _, lockErr := study.RunLoopActive(listing.Study); lockErr == nil && active {
			summary.RunActive = true
			summary.RunStatus = "active"
		}
		validation := study.ValidateStudyArtifacts(listing)
		for _, check := range validation.Checks {
			if check.Status != study.ValidationStatusPassed && check.Status != study.ValidationStatusInapplicable {
				check.Path = workspace.Rel(u.root, check.Path)
				summary.Findings = append(summary.Findings, studyFinding(check))
			}
		}
		summary.Artifacts = append(summary.Artifacts, DisplayArtifact{Label: "run-state", Path: workspace.Rel(u.root, study.RunStatePath(listing.Study)), Kind: "json"})
		for _, dim := range listing.Dimensions {
			summary.Artifacts = append(summary.Artifacts, DisplayArtifact{Label: "dimension", Path: workspace.Rel(u.root, dim.Path), Kind: "markdown"})
		}
		for _, src := range listing.Sources {
			if src.Kind == study.SourceKindMarkdown {
				summary.Artifacts = append(summary.Artifacts, DisplayArtifact{Label: "source", Path: workspace.Rel(u.root, src.Path), Kind: "markdown"})
			}
		}
		if len(listing.Dimensions) > 0 {
			summary.Artifacts = append(summary.Artifacts, DisplayArtifact{Label: "final-report", Path: workspace.Rel(u.root, filepath.Join(listing.Study.Path, "reports", "final")), Kind: "markdown"})
		}
		sortArtifacts(summary.Artifacts)
		out = append(out, summary)
	}
	return out, nil
}

func runTaskSummary(task study.TaskState, now time.Time) RunTaskSummary {
	r := RunTaskSummary{ID: task.ID, Kind: string(task.Kind), Dimension: task.DimensionRef, Source: task.Source, Status: string(task.Status), Provider: task.Agent.Provider, Model: task.Agent.Model, Attempts: task.Attempts, RuntimeAttempts: len(task.Agent.Attempts), Turns: task.Agent.Usage.Turns, TurnsKnown: task.Agent.Usage.TurnsKnown, Tokens: task.Agent.Usage.TotalTokens, TokensKnown: task.Agent.Usage.TotalTokensKnown, InputTokens: task.Agent.Usage.InputTokens, OutputTokens: task.Agent.Usage.OutputTokens, ReasoningTokens: task.Agent.Usage.ReasoningTokens, CacheReadTokens: task.Agent.Usage.CacheReadTokens, CacheWriteTokens: task.Agent.Usage.CacheWriteTokens, Cost: "n/a"}
	if retries := task.Attempts - 1; retries > 0 {
		r.Retries = retries
		if study.TaskSessionContinued(task) {
			r.SessionReuse = "same"
		} else {
			r.SessionReuse = "fresh"
		}
	}
	if task.LastError != nil {
		r.ErrorCode = task.LastError.Code
		r.Error = task.LastError.Message
	}
	if task.Agent.DurationMS > 0 {
		r.DurationMS = task.Agent.DurationMS
		r.Duration = (time.Duration(task.Agent.DurationMS) * time.Millisecond).Round(time.Second).String()
	} else if task.StartedAt != nil {
		end := now
		if task.CompletedAt != nil {
			end = *task.CompletedAt
		}
		d := end.Sub(*task.StartedAt)
		if d < 0 {
			d = 0
		}
		r.Duration = d.Round(time.Second).String()
		r.DurationMS = d.Milliseconds()
	} else {
		r.Duration = "n/a"
	}
	if task.Agent.Events != nil {
		r.Events = task.Agent.Events.Total
	}
	if task.Agent.Cost != nil {
		currency := task.Agent.Cost.Currency
		if currency == "" {
			currency = "cost"
		}
		r.Cost = fmt.Sprintf("%.4g %s", task.Agent.Cost.Amount, currency)
	}
	return r
}

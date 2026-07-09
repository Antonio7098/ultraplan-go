package app

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/study"
	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

type StudySummary struct {
	Name       string
	Sources    []string
	Dimensions []string
	Status     string
	RunID      string
	StatePath  string
	Total      int
	Completed  int
	Failed     int
	Findings   []DisplayFinding
	Artifacts  []DisplayArtifact
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
		if state, err := study.LoadRunState(listing.Study); err == nil {
			study.ReconcileRunState(&state, u.root, listing.Study, listing.Sources, listing.Dimensions, time.Now().UTC())
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
		} else if !errors.Is(err, study.ErrRunStateMissing) {
			summary.Status = displaySafe(err.Error())
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

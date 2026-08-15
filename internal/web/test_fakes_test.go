package web

import (
	"context"

	"github.com/Antonio7098/ultraplan-go/internal/app"
)

type fakeQueries struct {
	dashboard   app.WebDashboardResult
	projects    app.WebProjectsResult
	project     app.WebProjectResult
	sprint      app.WebSprintResult
	studies     app.WebStudiesResult
	study       app.WebStudyResult
	validation  app.WebValidationResult
	artifact    app.WebArtifactPreview
	health      app.WebHealthResult
	err         error
	healthCalls int
}

func sampleQueries() *fakeQueries {
	artifact := app.WebArtifactLink{Ref: "artifact_ref", Label: "plan", DisplayPath: "projects/alpha/sprints/30-web/plan.md", MediaType: "text/markdown"}
	finding := app.DisplayFinding{Severity: "warn", Section: "plan", Problem: "Review this item", Suggestion: "Inspect the plan."}
	sprint := app.WebSprintResult{
		Ref: "sprint_ref", Project: "alpha", Slug: "30-web", Status: "available",
		Assessment: "pass", NextAction: "none",
		Stages:   []app.StageSummary{{Name: "plan", Status: "complete"}},
		Execute:  app.ExecuteSummary{Available: true, Total: 1, Complete: 1},
		Review:   app.ReviewSummary{Available: true, Status: "complete", Verdict: "pass"},
		Smoke:    app.SmokeSummary{Available: true, Status: "complete", Verdict: "pass"},
		Findings: []app.DisplayFinding{finding}, Artifacts: []app.WebArtifactLink{artifact},
	}
	project := app.WebProjectResult{
		Ref: "project_ref", Name: "alpha", Docs: []string{"docs/PRD.md"},
		Findings: []app.DisplayFinding{finding}, Artifacts: []app.WebArtifactLink{artifact},
		Sprints:      []app.WebSprintResult{sprint},
		SprintCounts: app.CollectionInfo{ReturnedCount: 1, TotalCount: 1},
	}
	study := app.WebStudyResult{
		Ref: "study_ref", Name: "research", Sources: []string{"source"}, Dimensions: []string{"01-structure"},
		Status: "complete=true", Total: 1, Completed: 1, Findings: []app.DisplayFinding{}, Artifacts: []app.WebArtifactLink{artifact},
	}
	return &fakeQueries{
		dashboard: app.WebDashboardResult{
			Ref: "workspace_ref", Workspace: "workspace",
			Projects: app.WebProjectsResult{Items: []app.WebProjectResult{project}, CollectionInfo: app.CollectionInfo{ReturnedCount: 1, TotalCount: 1}},
			Sprints:  []app.WebSprintResult{sprint}, SprintCounts: app.CollectionInfo{ReturnedCount: 1, TotalCount: 1},
			Studies: app.WebStudiesResult{Items: []app.WebStudyResult{study}, CollectionInfo: app.CollectionInfo{ReturnedCount: 1, TotalCount: 1}},
		},
		projects: app.WebProjectsResult{Items: []app.WebProjectResult{project}, CollectionInfo: app.CollectionInfo{ReturnedCount: 1, TotalCount: 1}},
		project:  project, sprint: sprint,
		studies:    app.WebStudiesResult{Items: []app.WebStudyResult{study}, CollectionInfo: app.CollectionInfo{ReturnedCount: 1, TotalCount: 1}},
		study:      study,
		validation: app.WebValidationResult{Scope: "project", Ref: "project_ref", Findings: []app.DisplayFinding{finding}, CollectionInfo: app.CollectionInfo{ReturnedCount: 1, TotalCount: 1}},
		artifact:   app.WebArtifactPreview{Ref: "artifact_ref", DisplayPath: artifact.DisplayPath, MediaType: "text/markdown", Content: "# Plan\n", SizeBytes: 7, ReturnedBytes: 7},
		health:     app.WebHealthResult{Status: "ok", Server: true, Workspace: true},
	}
}

func (f *fakeQueries) Dashboard(context.Context) (app.WebDashboardResult, error) {
	return f.dashboard, f.err
}
func (f *fakeQueries) Projects(context.Context) (app.WebProjectsResult, error) {
	return f.projects, f.err
}
func (f *fakeQueries) Project(context.Context, string) (app.WebProjectResult, error) {
	return f.project, f.err
}
func (f *fakeQueries) Sprint(context.Context, string, string) (app.WebSprintResult, error) {
	return f.sprint, f.err
}
func (f *fakeQueries) Studies(context.Context) (app.WebStudiesResult, error) { return f.studies, f.err }
func (f *fakeQueries) Study(context.Context, string) (app.WebStudyResult, error) {
	return f.study, f.err
}
func (f *fakeQueries) Validations(context.Context, string, string) (app.WebValidationResult, error) {
	return f.validation, f.err
}
func (f *fakeQueries) Artifact(context.Context, string) (app.WebArtifactPreview, error) {
	return f.artifact, f.err
}
func (f *fakeQueries) Health(context.Context) (app.WebHealthResult, error) {
	f.healthCalls++
	return f.health, f.err
}

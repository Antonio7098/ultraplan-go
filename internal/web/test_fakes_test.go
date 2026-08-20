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
	requirementsArtifact := app.WebArtifactLink{Ref: "requirements_ref", Label: "requirements", DisplayPath: "projects/alpha/sprints/30-web/requirements.md", MediaType: "text/markdown"}
	contextArtifact := app.WebArtifactLink{Ref: "context_ref", Label: "code-context", DisplayPath: "projects/alpha/sprints/30-web/code-context.md", MediaType: "text/markdown"}
	indexArtifact := app.WebArtifactLink{Ref: "index_ref", Label: "sprint-index", DisplayPath: "projects/alpha/sprints/30-web/sprint-index.md", MediaType: "text/markdown"}
	finding := app.DisplayFinding{Severity: "warn", Section: "plan", Problem: "Review this item", Suggestion: "Inspect the plan."}
	sprint := app.WebSprintResult{
		Ref: "sprint_ref", Project: "alpha", Slug: "30-web", Status: "available",
		Overview: "Make sprint delivery easier to understand.", Assessment: "pass", NextAction: "Continue to review.",
		Stages:          []app.StageSummary{{Name: "plan", Status: "complete"}},
		RunStages:       []app.StageSummary{{Name: "requirements", Status: "complete", Path: "projects/alpha/sprints/30-web/requirements.md"}, {Name: "code-context", Status: "failed", Error: "provider failed", Path: "projects/alpha/sprints/30-web/code-context.md", ArtifactAvailable: true, ArtifactValid: true, LatestOutcome: "failed", NextAction: "A prior valid artifact is preserved; inspect the failure and explicitly rerun code-context."}, {Name: "sprint-index", Status: "waiting"}, {Name: "plan", Status: "complete"}, {Name: "execute", Status: "complete"}, {Name: "review", Status: "running"}, {Name: "smoke", Status: "waiting"}},
		CompletedStages: 3, TotalStages: 5, CurrentStage: "review",
		Execute: app.ExecuteSummary{Available: true, Total: 1, Complete: 1},
		Review: app.ReviewSummary{Available: true, Status: "running", Verdict: "", Completed: 1, Total: 3, Pending: 1, Running: 1, Reviewers: []app.ReviewItemSummary{
			{ID: "contract-security", Name: "Security contract", Kind: "contract", Path: "contracts/security.md", Status: "completed", Summary: "Security requirements checked."},
			{ID: "contract-api", Name: "API contract", Kind: "contract", Path: "contracts/api.md", Status: "running"},
			{ID: "handbook", Name: "Technical handbook", Kind: "handbook", Path: "technical-handbook.md", Status: "pending"},
		}},
		Smoke:    app.SmokeSummary{Available: true, Status: "complete", Verdict: "pass"},
		Findings: []app.DisplayFinding{finding}, Artifacts: []app.WebArtifactLink{requirementsArtifact, contextArtifact, indexArtifact, artifact},
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

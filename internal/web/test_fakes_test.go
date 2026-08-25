package web

import (
	"context"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/app"
	sprintpkg "github.com/Antonio7098/ultraplan-go/internal/sprint"
)

type fakeQueries struct {
	dashboard    app.WebDashboardResult
	projects     app.WebProjectsResult
	project      app.WebProjectResult
	sprint       app.WebSprintResult
	studies      app.WebStudiesResult
	study        app.WebStudyResult
	validation   app.WebValidationResult
	artifact     app.WebArtifactPreview
	dimensions   app.WebDimensionsResult
	reports      app.WebStudyReportsResult
	repoScores   app.WebStudyReposResult
	health       app.WebHealthResult
	prompt       app.WebPromptBundleResult
	models       app.WebModelsResult
	modelsErr    error
	err          error
	createErr    error
	sprintUsage  app.SprintMetricsSummary

	healthCalls    int
	promptCalls    int
	createdProject string
	createdSprint  string
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
		Smoke: app.SmokeSummary{Available: true, Status: "complete", Verdict: "pass", CoverageMapping: &sprintpkg.SmokeCoverageMapping{Sprint: "30-web", Suites: []string{"sprint-30"}, Complete: false, Rationale: "provider probe missing", RequiredCoverage: []string{"AC-01", "AC-02"}, Requirements: []sprintpkg.SmokeCoverageRequirement{
			{ID: "AC-01", Description: "The browser boundary is exercised.", MappedTests: []string{"browser-boundary"}},
			{ID: "AC-02", Description: "The provider boundary is exercised."},
		}, Tests: []sprintpkg.SmokeCoverageTest{{ID: "browser-boundary", Suite: "sprint-30", Coverage: []string{"AC-01"}}}}},
		Findings: []app.DisplayFinding{finding}, Artifacts: []app.WebArtifactLink{requirementsArtifact, contextArtifact, indexArtifact, artifact},
	}
	sprint.Smoke.CoverageMapping.EnsureMatrix()
	for index := range sprint.RunStages {
		contract := sprintpkg.InputContract(sprintpkg.PlanningStage(sprint.RunStages[index].Name))
		sprint.RunStages[index].PromptContract = &contract
	}
	contract := sprintpkg.InputContract(sprintpkg.StagePlan)
	explanation := sprintpkg.ExplainPrompt("stable\n<<< ULTRAPLAN STAGE-SPECIFIC INSTRUCTIONS BEGIN >>>\nstage")
	explanation.InputContract = &contract
	project := app.WebProjectResult{
		Ref: "project_ref", Name: "alpha", Docs: []string{"docs/PRD.md"},
		Findings: []app.DisplayFinding{finding}, Artifacts: []app.WebArtifactLink{artifact},
		Sprints:      []app.WebSprintResult{sprint},
		SprintCounts: app.CollectionInfo{ReturnedCount: 1, TotalCount: 1},
		Roadmap: []app.WebRoadmapPhase{{
			Title: "Product Phase 4 Wave — Local Web",
			Sprints: []app.WebRoadmapSprint{
				{Number: 29, Title: "Phase 3 Documentation And Release", Slug: "29-phase-3-documentation-hardening-release", Status: "delivered", Goal: "Stabilize review and smoke as supported workflows.", GateItems: []string{"Release gate passes"}, Exists: true, Assessment: "pass", CompletedStages: 10, TotalStages: 10},
				{Number: 30, Title: "Local Web Foundation", Slug: "30-web", Status: "active", Goal: "Serve a loopback-only read-only browser dashboard.", Exists: true, Assessment: "incomplete", CompletedStages: 3, TotalStages: 5, CurrentStage: "review"},
				{Number: 31, Title: "Guarded Web Operations And SSE Progress", Slug: "31-web-operations", Status: "planned", Goal: "Stream truthful live progress to the browser."},
			},
		}},
	}
	study := app.WebStudyResult{
		Ref: "study_ref", Name: "research", Sources: []string{"source"}, Dimensions: []string{"01-structure"},
		Status: "complete=true", Total: 1, Completed: 1, Findings: []app.DisplayFinding{}, Artifacts: []app.WebArtifactLink{artifact},
	}
	study.Retries.RetriedTasks = 1
	study.Retries.TotalRetries = 3
	study.Retries.SameSession = 1
	study.Parallelism = &app.ParallelismSummary{Decreased: true, Events: 2, RequestedParallelism: 4, EffectiveParallelism: 2}
	study.RetriedTasks = []app.WebStudyTaskRetry{{ID: "analysis:01-structure:repo", Kind: "analysis", Status: "completed", Retries: 3, SessionReuse: "same"}}
	study.Tasks = func() []app.RunTaskSummary {
		retryAt := time.Date(2026, 8, 21, 12, 30, 0, 0, time.UTC)
		return []app.RunTaskSummary{{ID: "analysis:01-structure:repo", Kind: "analysis", Dimension: "01", Source: "repo", Status: "failed", Duration: "4m32s", Attempts: 4, Retries: 3, SessionReuse: "same", ErrorCode: "runtime.failed", Error: "provider exited before the report was committed (exit 1)", Runtime: "codex", RetryAfter: &retryAt, SessionID: "sess_study_01", Provider: "openai", Model: "gpt-5.2", Turns: 12, TurnsKnown: true, Tokens: 45678, TokensKnown: true, Cost: "0.42 USD"}}
	}()
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
		dimensions: app.WebDimensionsResult{Items: []app.WebDimension{{Study: "research", Number: "01", Slug: "contract", Title: "Contract", DisplayPath: "studies/research/dimensions/01-contract.md", Content: "# Contract\n\nCheck the contract boundary.", Bytes: 35}}, CollectionInfo: app.CollectionInfo{ReturnedCount: 1, TotalCount: 1}},
		reports: app.WebStudyReportsResult{Dimensions: []app.WebStudyDimensionReports{{
			Number: "01", Slug: "contract",
			Final:   &app.WebStudyReportFile{Source: "01-contract", Ref: "final_ref", DisplayPath: "studies/research/reports/final/01-contract.md", Bytes: 12},
			Sources: []app.WebStudyReportFile{{Source: "repo", Ref: "source_ref", DisplayPath: "studies/research/reports/source/01-contract/repo.md", Bytes: 8}},
		}}, CollectionInfo: app.CollectionInfo{ReturnedCount: 2, TotalCount: 2}},
		health: app.WebHealthResult{Status: "ok", Server: true, Workspace: true},
		prompt: app.WebPromptBundleResult{Stage: sprintpkg.StagePlan, Available: true, Scope: "Deterministic stage preview", InputContract: contract, Explanation: &explanation},
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
func (f *fakeQueries) PromptBundle(context.Context, string, string, string) (app.WebPromptBundleResult, error) {
	f.promptCalls++
	return f.prompt, f.err
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
func (f *fakeQueries) StudyDimensions(context.Context, string) (app.WebDimensionsResult, error) {
	return f.dimensions, f.err
}
func (f *fakeQueries) StudyReports(context.Context, string) (app.WebStudyReportsResult, error) {
	return f.reports, f.err
}
func (f *fakeQueries) StudyRepos(context.Context, string) (app.WebStudyReposResult, error) {
	return f.repoScores, f.err
}
func (f *fakeQueries) Models(context.Context) (app.WebModelsResult, error) {
	return f.models, f.modelsErr
}

func (f *fakeQueries) CreateSprintWorkspace(_ context.Context, project, slug string) error {
	f.createdProject, f.createdSprint = project, slug
	return f.createErr
}

func (f *fakeQueries) Health(context.Context) (app.WebHealthResult, error) {
	f.healthCalls++
	return f.health, f.err
}

func (f *fakeQueries) SprintRuntimeUsage(_ context.Context, project, slug string) (app.SprintMetricsSummary, error) {
	if f.err != nil {
		return app.SprintMetricsSummary{}, f.err
	}
	return f.sprintUsage, nil
}

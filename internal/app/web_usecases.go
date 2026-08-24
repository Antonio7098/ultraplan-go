package app

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
	"github.com/Antonio7098/ultraplan-go/internal/project"
	"github.com/Antonio7098/ultraplan-go/internal/runcontrol"
	"github.com/Antonio7098/ultraplan-go/internal/sprint"
	"github.com/Antonio7098/ultraplan-go/internal/study"
	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

const (
	WebCollectionLimit  = 200
	WebPreviewByteLimit = 256 * 1024
)

var (
	ErrWebNotFound    = errors.New("web resource not found")
	ErrWebUnavailable = errors.New("web query unavailable")
)

type WebQueries interface {
	Dashboard(context.Context) (WebDashboardResult, error)
	Projects(context.Context) (WebProjectsResult, error)
	Project(context.Context, string) (WebProjectResult, error)
	Sprint(context.Context, string, string) (WebSprintResult, error)
	Studies(context.Context) (WebStudiesResult, error)
	Study(context.Context, string) (WebStudyResult, error)
	Validations(context.Context, string, string) (WebValidationResult, error)
	Artifact(context.Context, string) (WebArtifactPreview, error)
	Health(context.Context) (WebHealthResult, error)
}

// WebPromptQueries is an additive read-only capability used by prompt
// observability clients without widening the compatibility-critical WebQueries
// interface implemented by existing embedders.
type WebPromptQueries interface {
	PromptBundle(context.Context, string, string, string) (WebPromptBundleResult, error)
}

// WebResourceQueries is an additive read-only capability used by resource
// monitoring surfaces.
type WebResourceQueries interface {
	StudyResources(context.Context, string) (study.RunLoopResourceHistory, error)
}

// WebSprintUsageQueries is an additive read-only capability exposing sprint
// runtime token/cost metrics without widening the compatibility-critical
// WebQueries interface implemented by existing embedders.
type WebSprintUsageQueries interface {
	SprintRuntimeUsage(ctx context.Context, project, slug string) (SprintMetricsSummary, error)
}

// WebSprintWorkspaceMutation is an additive capability for surfaces that may
// materialize a roadmap sprint workspace directory without running any flow
// stage.
type WebSprintWorkspaceMutation interface {
	CreateSprintWorkspace(ctx context.Context, project, slug string) error
}

// SprintMetricsSummary projects sprint runtime metrics for usage surfaces.
type SprintMetricsSummary struct {
	RecentRuns []SprintMetricRow
	UpdatedAt  time.Time
}

type SprintMetricRow struct {
	Stage           string
	Task            string
	Model           string
	Status          string
	InputKnown      bool
	Input           int64
	OutputKnown     bool
	Output          int64
	ReasoningKnown  bool
	Reasoning       int64
	CacheReadKnown  bool
	CacheRead       int64
	CacheWriteKnown bool
	CacheWrite      int64
	TotalKnown      bool
	Total           int64
	TurnsKnown      bool
	Turns           int64
	CostKnown       bool
	CostAmount      float64
	CostSource      string
}

// WebModelQueries is an additive read-only capability that exposes the models
// available to the configured runtime for model selection surfaces.
type WebModelQueries interface {
	Models(context.Context) (WebModelsResult, error)
}

// RuntimeModelLister enumerates models through the configured runtime adapter.
type RuntimeModelLister interface {
	ListModels(ctx context.Context, provider string) ([]runtime.Model, error)
}

// WebModel describes one model selectable for runtime-backed operations.
type WebModel struct {
	Provider string
	ID       string
}

// WebModelsResult is the bounded model listing shown by selection surfaces.
type WebModelsResult struct {
	Models  []WebModel
	Default string
}

type WebUseCases interface {
	WebQueries
	WebPromptQueries
	WebResourceQueries
	WebSprintUsageQueries
	WebModelQueries
	WebOperations
	RunUseCases
}

type WebUseCaseOptions struct {
	StageRuntime      map[sprint.PlanningStage]sprint.StageRuntime
	ReviewConcurrency int
	SmokeSettings     sprint.SmokeSettings
	Runner            func(context.Context, OperationRequest, func(OperationEvent)) (OperationResult, error)
	RunControl        RunUseCases
	DurableOperations DurableOperationManager
	// DefaultModel is the workspace-configured model shown as the selection
	// default; it stays unchanged when empty.
	DefaultModel string
	// Models optionally supplies the runtime-backed model listing.
	Models RuntimeModelLister
}

type CollectionInfo struct {
	ReturnedCount int
	TotalCount    int
	Truncated     bool
}

type WebArtifactLink struct {
	Ref         string
	Label       string
	DisplayPath string
	MediaType   string
}

type WebProjectResult struct {
	Ref          string
	Name         string
	Docs         []string
	Findings     []DisplayFinding
	Artifacts    []WebArtifactLink
	Sprints      []WebSprintResult
	Roadmap      []WebRoadmapPhase
	SprintCounts CollectionInfo
}

// WebRoadmapSprint is one roadmap entry joined with live sprint state when the
// sprint workspace has been materialized.
type WebRoadmapSprint struct {
	Number          int
	Title           string
	Slug            string
	Status          string
	Goal            string
	GateItems       []string
	DependsOn       []int
	Exists          bool
	Assessment      string
	CompletedStages int
	TotalStages     int
	CurrentStage    string
}

type WebRoadmapPhase struct {
	Title   string
	Sprints []WebRoadmapSprint
}

type WebSprintResult struct {
	Ref               string
	Project           string
	Slug              string
	Status            string
	Assessment        string
	NextAction        string
	Stages            []StageSummary
	Execute           ExecuteSummary
	Review            ReviewSummary
	Smoke             SmokeSummary
	Findings          []DisplayFinding
	Artifacts         []WebArtifactLink
	Overview          string
	RunStages         []StageSummary
	CompletedStages   int
	TotalStages       int
	CurrentStage      string
	AttentionFindings []DisplayFinding
}

// WebPromptBundleResult is a content-free, read-only projection of the prompt
// that would be sent for one sprint stage. The full prompt is deliberately not
// returned by the web surface: its ordered blocks, sizes, digests, and stable
// prefix boundary provide observability without duplicating governed artifact
// and source contents into the browser.
type WebPromptBundleResult struct {
	Stage             sprint.PlanningStage
	Available         bool
	Scope             string
	UnavailableReason string
	InputContract     sprint.StageInputContract
	Explanation       *sprint.PromptExplanation
}

// ParallelismSummary is the web-facing view of run-loop parallelism throttling.
type ParallelismSummary struct {
	Decreased            bool
	Events               int
	RequestedParallelism int
	EffectiveParallelism int
}

type WebStudyResult struct {
	Ref          string
	Name         string
	Sources      []string
	Dimensions   []string
	Status       string
	RunID        string
	Total        int
	Completed    int
	Failed       int
	RunActive    bool
	ActiveTasks  int
	Pending      int
	Cancelled    int
	Retries      study.RetrySummary
	Parallelism  *ParallelismSummary
	RetriedTasks []WebStudyTaskRetry
	Tasks        []RunTaskSummary
	Findings     []DisplayFinding
	Artifacts    []WebArtifactLink
}

// WebStudyTaskRetry describes one task that needed retries and whether those
// retries continued the same agent session.
type WebStudyTaskRetry struct {
	ID           string
	Kind         string
	Status       string
	Retries      int
	SessionReuse string
}

type WebProjectsResult struct {
	Items []WebProjectResult
	CollectionInfo
}

type WebStudiesResult struct {
	Items []WebStudyResult
	CollectionInfo
}

type WebDashboardResult struct {
	Ref          string
	Workspace    string
	Projects     WebProjectsResult
	Sprints      []WebSprintResult
	Studies      WebStudiesResult
	SprintCounts CollectionInfo
}

type WebValidationResult struct {
	Scope    string
	Ref      string
	Findings []DisplayFinding
	CollectionInfo
}

type WebArtifactPreview struct {
	Ref           string
	DisplayPath   string
	MediaType     string
	Content       string
	SizeBytes     int64
	ReturnedBytes int
	Truncated     bool
	JSONValid     bool
}

type WebHealthResult struct {
	Status    string
	Server    bool
	Workspace bool
}

type webRefTarget struct {
	kind   string
	values []string
}

type webUseCases struct {
	root         string
	dashboard    dashboardUseCases
	secret       [32]byte
	mu           sync.RWMutex
	refs         map[string]webRefTarget
	runs         RunUseCases
	durable      DurableOperationManager
	defaultModel string
	models       RuntimeModelLister
}

// Models enumerates the models available to the configured runtime. The
// listing is bounded and read-only; unavailable runtimes surface a typed
// error instead of blocking dashboards that do not use the capability.
func (u *webUseCases) Models(ctx context.Context) (WebModelsResult, error) {
	if err := ctx.Err(); err != nil {
		return WebModelsResult{}, err
	}
	if u.models == nil {
		return WebModelsResult{}, ErrWebUnavailable
	}
	items, err := u.models.ListModels(ctx, "")
	if err != nil {
		return WebModelsResult{}, fmt.Errorf("%w: model listing", ErrWebUnavailable)
	}
	if len(items) > runtime.MaxModelListing {
		items = items[:runtime.MaxModelListing]
	}
	models := make([]WebModel, 0, len(items))
	for _, item := range items {
		models = append(models, WebModel{Provider: item.Provider, ID: item.ID})
	}
	return WebModelsResult{Models: nonNil(models), Default: u.defaultModel}, nil
}

func (u *webUseCases) StudyResources(ctx context.Context, name string) (study.RunLoopResourceHistory, error) {
	if err := ctx.Err(); err != nil {
		return study.RunLoopResourceHistory{}, err
	}
	listing, err := study.NewService(u.root).ListStudy(name)
	if err != nil {
		return study.RunLoopResourceHistory{}, fmt.Errorf("%w: study resources", ErrWebNotFound)
	}
	return study.LoadRunLoopResourceHistory(listing.Study, 240)
}

func NewWebUseCases(root string, opts WebUseCaseOptions) WebUseCases {
	u := &webUseCases{
		root: root,
		dashboard: dashboardUseCases{
			root:              root,
			stageRuntime:      opts.StageRuntime,
			reviewConcurrency: opts.ReviewConcurrency,
			smokeSettings:     opts.SmokeSettings,
			readOnly:          true,
			runner:            opts.Runner,
		},
		refs:         make(map[string]webRefTarget),
		runs:         opts.RunControl,
		durable:      opts.DurableOperations,
		defaultModel: strings.TrimSpace(opts.DefaultModel),
		models:       opts.Models,
	}
	if _, err := rand.Read(u.secret[:]); err != nil {
		u.secret = sha256.Sum256([]byte(filepath.Clean(root)))
	}
	return u
}

func (u *webUseCases) AcceptOperation(ctx context.Context, confirmation Confirmation, digest string) (AcceptedOperation, error) {
	if u.durable == nil {
		return AcceptedOperation{}, ErrWebUnavailable
	}
	return u.durable.AcceptOperation(ctx, confirmation, digest)
}

func (u *webUseCases) FinishOperation(ctx context.Context, runID string, state OperationState, runErr error) error {
	if u.durable == nil {
		return ErrWebUnavailable
	}
	return u.durable.FinishOperation(ctx, runID, state, runErr)
}

func (u *webUseCases) RecordOperationEvent(ctx context.Context, runID string, event OperationEvent) (bool, error) {
	if u.durable == nil {
		return false, ErrWebUnavailable
	}
	return u.durable.RecordOperationEvent(ctx, runID, event)
}

func (u *webUseCases) Runs(ctx context.Context, query runcontrol.Query) (runcontrol.Page, error) {
	if u.runs == nil {
		return runcontrol.Page{}, ErrWebUnavailable
	}
	return u.runs.Runs(ctx, query)
}

func (u *webUseCases) Run(ctx context.Context, id runcontrol.RunID) (runcontrol.Snapshot, error) {
	if u.runs == nil {
		return runcontrol.Snapshot{}, ErrWebUnavailable
	}
	return u.runs.Run(ctx, id)
}

func (u *webUseCases) RunEvents(ctx context.Context, id runcontrol.RunID, after uint64, limit int) ([]runcontrol.Event, error) {
	if u.runs == nil {
		return nil, ErrWebUnavailable
	}
	return u.runs.RunEvents(ctx, id, after, limit)
}

func (u *webUseCases) CancelRun(ctx context.Context, id runcontrol.RunID, reason string) (runcontrol.Snapshot, bool, error) {
	if u.runs == nil {
		return runcontrol.Snapshot{}, false, ErrWebUnavailable
	}
	return u.runs.CancelRun(ctx, id, reason)
}

func (u *webUseCases) RunHealth(ctx context.Context) (runcontrol.Health, error) {
	if u.runs == nil {
		return runcontrol.Health{}, ErrWebUnavailable
	}
	return u.runs.RunHealth(ctx)
}

func (u *webUseCases) Validate(ctx context.Context, req ValidationRequest) (ValidationOperationResult, error) {
	return u.dashboard.Validate(ctx, req)
}

func (u *webUseCases) PrepareOperation(ctx context.Context, req OperationRequest) (Confirmation, error) {
	return u.dashboard.PrepareOperation(ctx, req)
}

func (u *webUseCases) RunOperation(ctx context.Context, req OperationRequest, emit func(OperationEvent)) (OperationResult, error) {
	return u.dashboard.RunOperation(ctx, req, emit)
}

func (u *webUseCases) RecordOperationCleanupUncertain(ctx context.Context, record OperationCleanupUncertain) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if record.Request.Project != "" && record.Request.Sprint != "" {
		return sprint.NewService(u.root).RecordCleanupUncertain(ctx, record.Request.Project, record.Request.Sprint, sprint.CleanupUncertainRecord{
			OperationID: record.OperationID,
			Kind:        string(record.Request.Kind),
			Reason:      record.Reason,
			RecordedAt:  record.RecordedAt,
		})
	}
	if record.Request.Study != "" {
		return study.NewService(u.root).RecordCleanupUncertain(ctx, record.Request.Study, study.CleanupUncertainRecord{
			OperationID: record.OperationID,
			Kind:        string(record.Request.Kind),
			Reason:      record.Reason,
			RecordedAt:  record.RecordedAt,
		})
	}
	return fmt.Errorf("durable cleanup uncertainty is unavailable for operation %q", record.Request.Kind)
}

func (u *webUseCases) ReconcileOperations(ctx context.Context) error {
	summaries, err := u.dashboard.SprintSummaries(ctx)
	if err != nil {
		return err
	}
	service := sprint.NewService(u.root)
	for _, summary := range summaries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := service.ReconcileInterruptedMutation(ctx, summary.Project, summary.Slug); err != nil {
			return err
		}
	}
	studies, err := study.NewService(u.root).ListStudies()
	if err != nil {
		return err
	}
	studyService := study.NewService(u.root)
	for _, item := range studies {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := studyService.ReconcileInterruptedRun(ctx, item.Name); err != nil {
			return err
		}
	}
	return nil
}

func (u *webUseCases) Dashboard(ctx context.Context) (WebDashboardResult, error) {
	if err := ctx.Err(); err != nil {
		return WebDashboardResult{}, err
	}
	projects, err := u.Projects(ctx)
	if err != nil {
		return WebDashboardResult{}, err
	}
	studies, err := u.Studies(ctx)
	if err != nil {
		return WebDashboardResult{}, err
	}
	summaries, err := u.dashboard.SprintSummaries(ctx)
	if err != nil {
		return WebDashboardResult{}, err
	}
	total := len(summaries)
	summaries = bounded(summaries)
	sprints := make([]WebSprintResult, 0, len(summaries))
	for _, item := range summaries {
		sprints = append(sprints, u.webSprint(item))
	}
	return WebDashboardResult{
		Ref:          u.issue("workspace"),
		Workspace:    filepath.Base(filepath.Clean(u.root)),
		Projects:     projects,
		Sprints:      sprints,
		Studies:      studies,
		SprintCounts: collectionInfo(len(sprints), total),
	}, nil
}

func (u *webUseCases) Projects(ctx context.Context) (WebProjectsResult, error) {
	items, err := u.dashboard.ProjectSummaries(ctx)
	if err != nil {
		return WebProjectsResult{}, err
	}
	total := len(items)
	items = bounded(items)
	out := make([]WebProjectResult, 0, len(items))
	for _, item := range items {
		out = append(out, u.webProject(item, nil))
	}
	return WebProjectsResult{Items: out, CollectionInfo: collectionInfo(len(out), total)}, nil
}

func (u *webUseCases) Project(ctx context.Context, name string) (WebProjectResult, error) {
	items, err := u.dashboard.ProjectSummaries(ctx)
	if err != nil {
		return WebProjectResult{}, err
	}
	var selected *ProjectSummary
	for i := range items {
		if items[i].Name == name {
			selected = &items[i]
			break
		}
	}
	if selected == nil {
		return WebProjectResult{}, ErrWebNotFound
	}
	allSprints, err := u.dashboard.SprintSummaries(ctx)
	if err != nil {
		return WebProjectResult{}, err
	}
	var projectSprints []SprintSummary
	for _, item := range allSprints {
		if item.Project == name {
			projectSprints = append(projectSprints, item)
		}
	}
	total := len(projectSprints)
	liveBySlug := make(map[string]SprintSummary, len(projectSprints))
	for _, item := range projectSprints {
		liveBySlug[item.Slug] = item
	}
	projectSprints = bounded(projectSprints)
	out := make([]WebSprintResult, 0, len(projectSprints))
	for _, item := range projectSprints {
		out = append(out, u.webSprint(item))
	}
	result := u.webProject(*selected, out)
	result.Roadmap = u.projectRoadmap(name, liveBySlug)
	result.SprintCounts = collectionInfo(len(out), total)
	return result, nil
}

// projectRoadmap parses the project's governed roadmap and joins each entry
// with live sprint state when the sprint workspace exists.
func (u *webUseCases) projectRoadmap(projectName string, live map[string]SprintSummary) []WebRoadmapPhase {
	content, err := os.ReadFile(filepath.Join(u.root, "projects", projectName, "roadmap.md"))
	if err != nil {
		return nil
	}
	parsed, _ := project.ParseRoadmap(string(content))
	phases := make([]WebRoadmapPhase, 0, len(parsed.Phases))
	for _, phase := range parsed.Phases {
		sprints := make([]WebRoadmapSprint, 0, len(phase.Sprints))
		for _, item := range phase.Sprints {
			entry := WebRoadmapSprint{
				Number:    item.Number,
				Title:     item.Title,
				Slug:      item.Slug,
				Status:    string(item.Status),
				Goal:      item.Goal,
				GateItems: append([]string(nil), item.GateItems...),
				DependsOn: append([]int(nil), item.DependsOn...),
			}
			if state, ok := live[item.Slug]; ok {
				entry.Exists = true
				entry.Assessment = state.Assessment
				entry.TotalStages = len(state.Stages)
				for _, stage := range state.Stages {
					if stage.Status == "complete" || stage.Status == "completed" || stage.Status == "skipped" {
						entry.CompletedStages++
						continue
					}
					if entry.CurrentStage == "" {
						entry.CurrentStage = stage.Name
					}
				}
			}
			sprints = append(sprints, entry)
		}
		phases = append(phases, WebRoadmapPhase{Title: phase.Title, Sprints: sprints})
	}
	for i, j := 0, len(phases)-1; i < j; i, j = i+1, j-1 {
		phases[i], phases[j] = phases[j], phases[i]
	}
	for i := range phases {
		sprints := phases[i].Sprints
		for a, b := 0, len(sprints)-1; a < b; a, b = a+1, b-1 {
			sprints[a], sprints[b] = sprints[b], sprints[a]
		}
		phases[i].Sprints = sprints
	}
	return phases
}

func (u *webUseCases) SprintRuntimeUsage(ctx context.Context, projectRef, slug string) (SprintMetricsSummary, error) {
	if err := ctx.Err(); err != nil {
		return SprintMetricsSummary{}, err
	}
	metrics, err := sprint.NewService(u.root).RuntimeMetrics(projectRef, slug)
	if err != nil {
		return SprintMetricsSummary{}, fmt.Errorf("%w: sprint runtime metrics", ErrWebNotFound)
	}
	summary := SprintMetricsSummary{UpdatedAt: metrics.UpdatedAt}
	const maxRecentRuns = 12
	start := 0
	if len(metrics.Runs) > maxRecentRuns {
		start = len(metrics.Runs) - maxRecentRuns
	}
	for _, run := range metrics.Runs[start:] {
		row := SprintMetricRow{
			Stage: string(run.Stage), Task: run.Task, Model: run.Model, Status: run.Status,
			InputKnown: run.InputTokens.Known, Input: run.InputTokens.Value,
			OutputKnown: run.OutputTokens.Known, Output: run.OutputTokens.Value,
			ReasoningKnown: run.ReasoningTokens.Known, Reasoning: run.ReasoningTokens.Value,
			CacheReadKnown: run.CacheReadTokens.Known, CacheRead: run.CacheReadTokens.Value,
			CacheWriteKnown: run.CacheWriteTokens.Known, CacheWrite: run.CacheWriteTokens.Value,
			TotalKnown: run.TotalTokens.Known, Total: run.TotalTokens.Value,
			TurnsKnown: run.Turns.Known, Turns: run.Turns.Value,
			CostKnown: run.CostCurrency != "" || run.CostAmount != 0, CostAmount: run.CostAmount, CostSource: run.CostSource,
		}
		summary.RecentRuns = append(summary.RecentRuns, row)
	}
	return summary, nil
}

// CreateSprintWorkspace materializes the sprint directory for a roadmap slug
// without running any flow stage.
func (u *webUseCases) CreateSprintWorkspace(ctx context.Context, project, slug string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := u.dashboard.sprintService().CreateWorkspace(project, slug)
	if err != nil {
		return mapSprintError("sprint.create", err)
	}
	return nil
}

func (u *webUseCases) Sprint(ctx context.Context, project, slug string) (WebSprintResult, error) {
	items, err := u.dashboard.SprintSummaries(ctx)
	if err != nil {
		return WebSprintResult{}, err
	}
	for _, item := range items {
		if item.Project == project && item.Slug == slug {
			result := u.webSprint(item)
			result.AttentionFindings = append([]DisplayFinding(nil), result.Findings...)
			if len(result.AttentionFindings) > 3 {
				result.AttentionFindings = result.AttentionFindings[:3]
			}
			result.Overview = u.sprintOverview(project, slug)
			result.RunStages = sprintRunStages(item)
			result.TotalStages = len(result.RunStages)
			for _, stage := range result.RunStages {
				if stage.Status == "complete" || stage.Status == "completed" || stage.Status == "skipped" {
					result.CompletedStages++
					continue
				}
				if result.CurrentStage == "" {
					result.CurrentStage = stage.Name
				}
			}
			return result, nil
		}
	}
	return WebSprintResult{}, ErrWebNotFound
}

func (u *webUseCases) PromptBundle(ctx context.Context, project, slug, stageName string) (WebPromptBundleResult, error) {
	if err := ctx.Err(); err != nil {
		return WebPromptBundleResult{}, err
	}
	stage := sprint.PlanningStage(stageName)
	result := WebPromptBundleResult{
		Stage:         stage,
		Scope:         "Deterministic stage preview",
		InputContract: sprint.InputContract(stage),
	}
	service := u.dashboard.sprintService()
	var (
		preview sprint.PromptPreview
		err     error
	)
	switch stage {
	case sprint.StageRequirements:
		preview, err = service.PromptRequirements(project, slug)
	case sprint.StageCodeContext:
		preview, err = service.PromptCodeContext(project, slug)
	case sprint.StageSprintIndex:
		preview, err = service.PromptSprintIndex(project, slug)
	case sprint.StageTechnicalHandbook:
		preview, err = service.PromptTechnicalHandbook(project, slug)
	case sprint.StageAreaReasoning:
		result.Scope = "First selected area prompt"
		preview, err = service.PromptAreaReasoning(project, slug)
	case sprint.StageReasoning:
		preview, err = service.PromptReasoning(project, slug)
	case sprint.StagePlan:
		preview, err = service.PromptPlan(project, slug)
	case sprint.StageExecute:
		result.Scope = "One session for the ordered task queue"
		preview, err = service.PromptExecute(project, slug, sprint.ExecuteRequest{})
	case sprint.StageReview:
		result.Scope = "Review manifest preview; reviewer suffixes vary"
		preview, err = service.PromptReview(project, slug, sprint.ReviewRequest{})
	case sprint.StageSmoke:
		result.Scope = "Prepared after smoke harness discovery"
		result.UnavailableReason = "The smoke prompt is assembled only after the governed harness and acceptance coverage are discovered."
		return result, nil
	default:
		return WebPromptBundleResult{}, ErrWebNotFound
	}
	if err := ctx.Err(); err != nil {
		return WebPromptBundleResult{}, err
	}
	if err != nil {
		result.UnavailableReason = displaySafe(err.Error())
		return result, nil
	}
	explanation := preview.Explanation
	if explanation == nil {
		value := sprint.ExplainPrompt(preview.Prompt)
		explanation = &value
	}
	contract := result.InputContract
	explanation.InputContract = &contract
	result.Available = true
	result.Explanation = explanation
	return result, nil
}

func (u *webUseCases) sprintOverview(project, slug string) string {
	path := filepath.Join(u.root, "projects", project, "sprints", slug, "requirements.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	inGoal := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			if inGoal {
				break
			}
			inGoal = strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(trimmed, "## ")), "Sprint Goal")
			continue
		}
		if inGoal && trimmed != "" {
			return strings.Trim(strings.TrimSpace(trimmed), "`*")
		}
	}
	return ""
}

func sprintRunStages(item SprintSummary) []StageSummary {
	stages := append([]StageSummary(nil), item.Stages...)
	executeStatus := "waiting"
	if item.Execute.Available {
		switch {
		case item.Execute.Running > 0:
			executeStatus = "running"
		case item.Execute.Failed > 0:
			executeStatus = "failed"
		case item.Execute.Total > 0 && item.Execute.Complete+item.Execute.Deferred == item.Execute.Total:
			executeStatus = "complete"
		default:
			executeStatus = "ready"
		}
	}
	stages = append(stages, StageSummary{Name: "execute", Status: executeStatus, Error: item.Execute.Message})
	reviewStatus := "waiting"
	if item.Review.Available {
		reviewStatus = item.Review.Status
		if reviewStatus == "" {
			reviewStatus = "ready"
		}
	}
	stages = append(stages, StageSummary{Name: "review", Status: reviewStatus, Error: item.Review.Error})
	smokeStatus := "waiting"
	if item.Smoke.Available {
		smokeStatus = item.Smoke.Status
		if smokeStatus == "" {
			smokeStatus = "ready"
		}
	}
	stages = append(stages, StageSummary{Name: "smoke", Status: smokeStatus, Error: item.Smoke.Error})
	for index := range stages {
		contract := sprint.InputContract(sprint.PlanningStage(stages[index].Name))
		stages[index].PromptContract = &contract
	}
	return stages
}

func (u *webUseCases) Studies(ctx context.Context) (WebStudiesResult, error) {
	items, err := u.dashboard.StudySummaries(ctx)
	if err != nil {
		return WebStudiesResult{}, err
	}
	total := len(items)
	items = bounded(items)
	out := make([]WebStudyResult, 0, len(items))
	for _, item := range items {
		out = append(out, u.webStudy(item))
	}
	return WebStudiesResult{Items: out, CollectionInfo: collectionInfo(len(out), total)}, nil
}

func (u *webUseCases) Study(ctx context.Context, name string) (WebStudyResult, error) {
	items, err := u.dashboard.StudySummaries(ctx)
	if err != nil {
		return WebStudyResult{}, err
	}
	for _, item := range items {
		if item.Name == name {
			result := u.webStudy(item)
			if listing, listErr := study.NewService(u.root).ListStudy(name); listErr == nil {
				if history, histErr := study.LoadRunLoopResourceHistory(listing.Study, 240); histErr == nil {
					throttle := study.SummarizeParallelismThrottle(history)
					result.Parallelism = &ParallelismSummary{Decreased: throttle.Decreased, Events: throttle.Events, RequestedParallelism: throttle.RequestedParallelism, EffectiveParallelism: throttle.EffectiveParallelism}
				}
			}
			return result, nil
		}
	}
	return WebStudyResult{}, ErrWebNotFound
}

func (u *webUseCases) Validations(ctx context.Context, scope, ref string) (WebValidationResult, error) {
	target, ok := u.resolve(ref, scope)
	if !ok {
		return WebValidationResult{}, ErrWebNotFound
	}
	var findings []DisplayFinding
	switch scope {
	case "workspace":
		if err := ctx.Err(); err != nil {
			return WebValidationResult{}, err
		}
		result := workspace.Validate(u.root)
		for _, issue := range result.Issues {
			findings = append(findings, DisplayFinding{Severity: "error", Section: "workspace", Problem: displaySafe(issue)})
		}
	case "project":
		result, err := u.Project(ctx, target.values[0])
		if err != nil {
			return WebValidationResult{}, err
		}
		findings = append(findings, result.Findings...)
	case "sprint":
		result, err := u.Sprint(ctx, target.values[0], target.values[1])
		if err != nil {
			return WebValidationResult{}, err
		}
		findings = append(findings, result.Findings...)
	case "study":
		result, err := u.Study(ctx, target.values[0])
		if err != nil {
			return WebValidationResult{}, err
		}
		findings = append(findings, result.Findings...)
	default:
		return WebValidationResult{}, ErrWebNotFound
	}
	total := len(findings)
	findings = bounded(findings)
	if findings == nil {
		findings = []DisplayFinding{}
	}
	return WebValidationResult{Scope: scope, Ref: ref, Findings: findings, CollectionInfo: collectionInfo(len(findings), total)}, nil
}

func (u *webUseCases) Artifact(ctx context.Context, ref string) (WebArtifactPreview, error) {
	target, ok := u.resolve(ref, "artifact")
	if !ok || len(target.values) != 1 {
		return WebArtifactPreview{}, ErrWebNotFound
	}
	rel := target.values[0]
	if err := ctx.Err(); err != nil {
		return WebArtifactPreview{}, err
	}
	if !supportedPreviewPath(rel) {
		return WebArtifactPreview{}, ErrWebNotFound
	}
	path, err := u.containedArtifactPath(rel)
	if err != nil {
		return WebArtifactPreview{}, ErrWebNotFound
	}
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return WebArtifactPreview{}, ErrWebNotFound
		}
		return WebArtifactPreview{}, fmt.Errorf("%w: artifact read", ErrWebUnavailable)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return WebArtifactPreview{}, ErrWebNotFound
	}
	data, err := io.ReadAll(io.LimitReader(file, WebPreviewByteLimit+1))
	if err != nil {
		return WebArtifactPreview{}, fmt.Errorf("%w: artifact read", ErrWebUnavailable)
	}
	truncated := len(data) > WebPreviewByteLimit
	if truncated {
		data = data[:WebPreviewByteLimit]
	}
	media := mediaTypeForPath(rel)
	return WebArtifactPreview{
		Ref:           ref,
		DisplayPath:   filepath.ToSlash(rel),
		MediaType:     media,
		Content:       string(data),
		SizeBytes:     info.Size(),
		ReturnedBytes: len(data),
		Truncated:     truncated,
		JSONValid:     media != "application/json" || json.Valid(data),
	}, nil
}

func (u *webUseCases) Health(ctx context.Context) (WebHealthResult, error) {
	if err := ctx.Err(); err != nil {
		return WebHealthResult{}, err
	}
	info, err := os.Stat(filepath.Join(u.root, workspace.MarkerFile))
	available := err == nil && !info.IsDir()
	status := "ok"
	if !available {
		status = "unavailable"
	}
	return WebHealthResult{Status: status, Server: true, Workspace: available}, nil
}

func (u *webUseCases) webProject(item ProjectSummary, sprints []WebSprintResult) WebProjectResult {
	docs := append([]string(nil), item.MarkdownDocs...)
	if docs == nil {
		docs = []string{}
	}
	findings := bounded(append([]DisplayFinding(nil), item.Findings...))
	if findings == nil {
		findings = []DisplayFinding{}
	}
	return WebProjectResult{
		Ref:       u.issue("project", item.Name),
		Name:      item.Name,
		Docs:      docs,
		Findings:  findings,
		Artifacts: u.webArtifacts(item.Artifacts),
		Sprints:   nonNil(sprints),
	}
}

func (u *webUseCases) webSprint(item SprintSummary) WebSprintResult {
	stages := append([]StageSummary(nil), item.Stages...)
	if stages == nil {
		stages = []StageSummary{}
	}
	findings := bounded(append([]DisplayFinding(nil), item.Findings...))
	if findings == nil {
		findings = []DisplayFinding{}
	}
	return WebSprintResult{
		Ref:        u.issue("sprint", item.Project, item.Slug),
		Project:    item.Project,
		Slug:       item.Slug,
		Status:     item.Status,
		Assessment: item.Assessment,
		NextAction: displaySafe(item.NextAction),
		Stages:     stages,
		Execute:    item.Execute,
		Review:     item.Review,
		Smoke:      item.Smoke,
		Findings:   findings,
		Artifacts:  u.webArtifacts(item.Artifacts),
	}
}

func (u *webUseCases) webStudy(item StudySummary) WebStudyResult {
	findings := bounded(append([]DisplayFinding(nil), item.Findings...))
	if findings == nil {
		findings = []DisplayFinding{}
	}
	retried := make([]WebStudyTaskRetry, 0, len(item.Tasks))
	for _, task := range item.Tasks {
		if task.Retries == 0 && task.Status != "retrying" {
			continue
		}
		retries := task.Retries
		if retries == 0 {
			retries = 1
		}
		retried = append(retried, WebStudyTaskRetry{ID: task.ID, Kind: task.Kind, Status: task.Status, Retries: retries, SessionReuse: task.SessionReuse})
	}
	if retried == nil {
		retried = []WebStudyTaskRetry{}
	}
	return WebStudyResult{
		Ref:          u.issue("study", item.Name),
		Name:         item.Name,
		Sources:      nonNil(append([]string(nil), item.Sources...)),
		Dimensions:   nonNil(append([]string(nil), item.Dimensions...)),
		Status:       item.Status,
		RunID:        displaySafe(item.RunID),
		Total:        item.Total,
		Completed:    item.Completed,
		Failed:       item.Failed,
		RunActive:    item.RunActive,
		ActiveTasks:  item.ActiveTasks,
		Pending:      item.Pending,
		Cancelled:    item.Cancelled,
		Retries:      item.Retries,
		RetriedTasks: retried,
		Tasks:        append([]RunTaskSummary(nil), item.Tasks...),
		Findings:     findings,
		Artifacts:    u.webArtifacts(item.Artifacts),
	}
}

func (u *webUseCases) webArtifacts(items []DisplayArtifact) []WebArtifactLink {
	out := make([]WebArtifactLink, 0, len(items))
	for _, item := range items {
		rel := filepath.ToSlash(filepath.Clean(filepath.FromSlash(item.Path)))
		if filepath.IsAbs(rel) || !supportedPreviewPath(rel) {
			continue
		}
		out = append(out, WebArtifactLink{
			Ref:         u.issue("artifact", rel),
			Label:       item.Label,
			DisplayPath: rel,
			MediaType:   mediaTypeForPath(rel),
		})
		if len(out) == WebCollectionLimit {
			break
		}
	}
	if out == nil {
		return []WebArtifactLink{}
	}
	return out
}

func (u *webUseCases) issue(kind string, values ...string) string {
	mac := hmac.New(sha256.New, u.secret[:])
	_, _ = mac.Write([]byte(kind))
	for _, value := range values {
		_, _ = mac.Write([]byte{0})
		_, _ = mac.Write([]byte(value))
	}
	ref := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	target := webRefTarget{kind: kind, values: append([]string(nil), values...)}
	u.mu.Lock()
	u.refs[ref] = target
	u.mu.Unlock()
	return ref
}

func (u *webUseCases) resolve(ref, kind string) (webRefTarget, bool) {
	u.mu.RLock()
	target, ok := u.refs[ref]
	u.mu.RUnlock()
	return target, ok && target.kind == kind
}

func (u *webUseCases) containedArtifactPath(rel string) (string, error) {
	path, err := workspace.ResolveInside(u.root, rel)
	if err != nil {
		return "", err
	}
	root, err := filepath.EvalSymlinks(u.root)
	if err != nil {
		return "", err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	inside, err := filepath.Rel(root, path)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return "", errors.New("artifact escapes workspace")
	}
	return path, nil
}

func mediaTypeForPath(path string) string {
	if strings.EqualFold(filepath.Ext(path), ".json") {
		return "application/json"
	}
	return "text/markdown"
}

func bounded[T any](items []T) []T {
	if len(items) > WebCollectionLimit {
		items = items[:WebCollectionLimit]
	}
	return items
}

func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

func collectionInfo(returned, total int) CollectionInfo {
	return CollectionInfo{ReturnedCount: returned, TotalCount: total, Truncated: returned < total}
}

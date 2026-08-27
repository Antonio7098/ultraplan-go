package sprint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/platform/gitpublish"
	pprocess "github.com/Antonio7098/ultraplan-go/internal/platform/process"
	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
	"github.com/Antonio7098/ultraplan-go/internal/project"
	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

type Service struct {
	root                string
	store               FSStore
	now                 func() time.Time
	runtime             Runtime
	repairRuntime       Runtime
	runtimeConfig       pruntime.Request
	runtimeProgress     func(RuntimeProgress)
	stageRuntime        map[PlanningStage]StageRuntime
	verificationRuntime map[VerificationPhase]StageRuntime
	qaSettings          QASettings
	qaSettingsErr       error
	qaWriterFence       func(QAWriterToken) error
	qaMapFence          func(QAMap) error
	reviewConcurrency   int
	processRunner       pprocess.Runner
	smokeSettings       SmokeSettings
	mutations           *sync.Map
	metricsMu           *sync.Mutex
	statusWrites        bool
	codeContextTarget   func(string) (ExecuteTargetRef, []ValidationFinding)
	publisher           gitpublish.Publisher
}

func (s Service) WithReviewConcurrency(n int) Service { s.reviewConcurrency = n; return s }

// WithClock supplies a deterministic clock for state-transition tests.
func (s Service) WithClock(now func() time.Time) Service {
	if now != nil {
		s.now = now
	}
	return s
}

func (s Service) WithProcessRunner(runner pprocess.Runner) Service {
	s.processRunner = runner
	return s
}

func (s Service) WithPublisher(publisher gitpublish.Publisher) Service {
	s.publisher = publisher
	return s
}

func (s Service) WithSmokeSettings(settings SmokeSettings) Service {
	s.smokeSettings = settings
	return s
}

type StageRuntime struct {
	Model   string
	Variant string
}

func NewService(root string) Service {
	return Service{root: root, store: NewFSStore(root), now: func() time.Time { return time.Now().UTC() }, processRunner: pprocess.DirectRunner{}, smokeSettings: DefaultSmokeSettings(), mutations: &sync.Map{}, metricsMu: &sync.Mutex{}, statusWrites: true}
}

// WithoutStatusWrites derives status from current artifacts without creating a
// missing flow-state file. It is used by strictly read-only presentation
// surfaces; existing CLI/TUI status behavior remains unchanged.
func (s Service) WithoutStatusWrites() Service {
	s.statusWrites = false
	return s
}

var ErrVerificationConflict = errors.New("verification mutation already in progress")

func (s Service) acquireMutation(projectRef, sprintRef string) (func(), error) {
	sp, err := s.resolveMutationSprint(projectRef, sprintRef)
	if err != nil {
		return nil, err
	}
	key := filepath.Clean(sp.Path)
	if s.mutations == nil {
		s.mutations = &sync.Map{}
	}
	if _, loaded := s.mutations.LoadOrStore(key, struct{}{}); loaded {
		return nil, fmt.Errorf("%w for %s/%s; wait for the active attempt or cancel it", ErrVerificationConflict, sp.Project, sp.Slug)
	}
	fileLock, lockErr := acquireVerificationFileLock(s.root, sp, s.now().UTC())
	if lockErr != nil {
		s.mutations.Delete(key)
		return nil, lockErr
	}
	return func() {
		_ = fileLock.release()
		s.mutations.Delete(key)
	}, nil
}

func (s Service) WithRuntime(rt Runtime, reqs ...pruntime.Request) Service {
	s.runtime = rt
	if len(reqs) > 0 {
		s.runtimeConfig = reqs[0]
	}
	return s
}

func (s Service) WithRepairRuntime(rt Runtime) Service {
	s.repairRuntime = rt
	return s
}

type RuntimeProgress struct {
	Stage      PlanningStage
	Task       string
	CoverageID string
	Event      pruntime.Event
}

// WithRuntimeProgress observes bounded, sanitized runtime events for every
// runtime-backed sprint operation created by this service.
func (s Service) WithRuntimeProgress(progress func(RuntimeProgress)) Service {
	s.runtimeProgress = progress
	return s
}

func (s Service) WithStageRuntime(overrides map[PlanningStage]StageRuntime) Service {
	s.stageRuntime = map[PlanningStage]StageRuntime{}
	if s.verificationRuntime == nil {
		s.verificationRuntime = map[VerificationPhase]StageRuntime{}
	}
	for stage, override := range overrides {
		if phase, ok := verificationPhaseForStage(stage); ok {
			s.verificationRuntime[phase] = override
			continue
		}
		s.stageRuntime[stage] = override
	}
	return s
}

func (s Service) WithVerificationRuntime(overrides map[VerificationPhase]StageRuntime) Service {
	s.verificationRuntime = make(map[VerificationPhase]StageRuntime, len(overrides))
	for phase, override := range overrides {
		s.verificationRuntime[phase] = override
	}
	return s
}

// WithQASettings freezes the validated effective QA policy on the service
// value. It does not construct a runtime, acquire ownership, or write state.
func (s Service) WithQASettings(settings QASettings) Service {
	s.qaSettings = settings
	s.qaSettingsErr = ValidateQASettings(settings)
	return s
}

// WithQAWriterFence installs the durable run-ownership check used before each
// QA publication. Callers that own run control should compare all token fields
// against the currently claimed operation.
func (s Service) WithQAWriterFence(fence func(QAWriterToken) error) Service {
	s.qaWriterFence = fence
	return s
}

// WithQAMapFence installs the governed-input check used immediately before
// investigator work and publication. Production callers normally use the
// service's deterministic map rebuild; tests and alternate stores can supply
// the same boundary without mutating Service internals.
func (s Service) WithQAMapFence(fence func(QAMap) error) Service {
	s.qaMapFence = fence
	return s
}

func (s Service) effectiveQASettings() (QASettings, error) {
	if s.qaSettingsErr != nil {
		return QASettings{}, s.qaSettingsErr
	}
	if strings.TrimSpace(s.qaSettings.Runtime.Model) == "" {
		return QASettings{}, fmt.Errorf("QA settings are not configured")
	}
	return s.qaSettings, nil
}

// withStageOverrides returns a service copy whose stage runtime map is merged
// with request-scoped stage model/variant overrides. Configured defaults stay
// unchanged for stages without an entry.
func (s Service) withStageOverrides(overrides map[PlanningStage]StageRuntime) Service {
	if len(overrides) == 0 {
		return s
	}
	merged := make(map[PlanningStage]StageRuntime, len(s.stageRuntime)+len(overrides))
	for stage, override := range s.stageRuntime {
		merged[stage] = override
	}
	for stage, override := range overrides {
		if override.Model == "" && override.Variant == "" {
			continue
		}
		current := merged[stage]
		if override.Model != "" {
			current.Model = override.Model
		}
		if override.Variant != "" {
			current.Variant = override.Variant
		}
		merged[stage] = current
	}
	s.stageRuntime = merged
	return s
}

func (s Service) runtimeForStage(stage PlanningStage) (StageRuntime, bool) {
	if phase, ok := verificationPhaseForStage(stage); ok {
		runtime, found := s.verificationRuntime[phase]
		return runtime, found
	}
	runtime, found := s.stageRuntime[stage]
	return runtime, found
}

func (s Service) Status(projectRef, sprintRef string) (StatusSummary, error) {
	projects, err := project.DiscoverProjects(s.root)
	if err != nil {
		return StatusSummary{}, err
	}
	p, err := project.ResolveProject(projects, projectRef)
	if err != nil {
		return StatusSummary{}, err
	}
	sprints, err := DiscoverSprints(s.root, p)
	if err != nil {
		return StatusSummary{}, err
	}
	sp, err := ResolveSprint(sprints, sprintRef)
	if err != nil {
		return StatusSummary{}, err
	}
	if !inside(p.Path, sp.Path) {
		return StatusSummary{}, fmt.Errorf("sprint path mismatch for %q", sp.Slug)
	}
	legacyCodeContextState := preCodeContextFlowState(s.root, sp)
	state, err := LoadFlowState(s.root, sp)
	stateLoaded := err == nil
	if err != nil && !errors.Is(err, ErrFlowStateMissing) {
		return StatusSummary{}, err
	}
	snap, err := s.store.ReadArtifacts(sp)
	if err != nil {
		return StatusSummary{}, err
	}
	var prior []StageState
	if stateLoaded {
		prior = state.Stages
	}
	stages := DeriveStages(sp, snap, prior)
	refreshed := NewFlowState(sp, stages, s.now())
	if stateLoaded {
		refreshed.Review = state.Review
		refreshed.Smoke = state.Smoke
		refreshed.QA = state.QA
		if refreshed.Review != nil && refreshed.Review.Fingerprint != "" {
			manifest, reviewFindings, reviewErr := s.PrepareReview(projectRef, sprintRef, ReviewRequest{})
			refreshed.Review.Stale = reviewErr != nil || len(reviewFindings) > 0 || (strictCompletedReviewSnapshotFreshness && manifest.Fingerprint != refreshed.Review.Fingerprint)
			if !refreshed.Review.Stale {
				content, readErr := s.store.ReadArtifact(sp, StageReview)
				validationManifest := manifest
				if !strictCompletedReviewSnapshotFreshness {
					validationManifest.Fingerprint = refreshed.Review.Fingerprint
				}
				refreshed.Review.Stale = readErr != nil || len(ValidateReviewContent(content, validationManifest)) > 0 || refreshed.Review.ArtifactDigest == "" || hashBytes([]byte(content)) != refreshed.Review.ArtifactDigest
			}
		}
		if refreshed.Smoke != nil {
			smokePath, pathErr := ArtifactPath(s.root, sp, StageSmoke)
			data, readErr := os.ReadFile(smokePath)
			invalid := pathErr != nil || readErr != nil || len(ValidateSmokeContent(string(data))) > 0
			fingerprintMismatch := readErr == nil && refreshed.Smoke.SmokeFingerprint != "" && refreshed.Smoke.SmokeFingerprint != hashBytes(data)
			reviewMismatch := refreshed.Review == nil || refreshed.Review.Stale || refreshed.Smoke.ReviewFingerprint != refreshed.Review.Fingerprint
			refreshed.Smoke.Stale = invalid || fingerprintMismatch || reviewMismatch
			refreshed.Smoke.Reconciliation = fingerprintMismatch || (readErr == nil && refreshed.Smoke.SmokeFingerprint == "")
		}
	}
	if s.statusWrites && !legacyCodeContextState {
		if err := SaveFlowState(s.root, sp, refreshed); err != nil {
			return StatusSummary{}, err
		}
	}
	flowPath, err := FlowStatePath(s.root, sp)
	if err != nil {
		return StatusSummary{}, err
	}
	var executeState *ExecuteRunState
	var historicalExecutionStatus string
	loadedExecute, executeErr := LoadExecuteRunState(s.root, sp)
	if executeErr != nil && !errors.Is(executeErr, ErrExecuteRunStateMissing) {
		if legacyStatus, ok := LegacyTerminalExecuteStatus(s.root, sp); ok {
			historicalExecutionStatus = legacyStatus
		} else {
			return StatusSummary{}, executeErr
		}
	}
	if executeErr == nil {
		executeState = &loadedExecute
	}
	runStatePath, err := ExecuteRunStatePath(s.root, sp)
	if err != nil {
		return StatusSummary{}, err
	}
	verification := VerificationStatus{Project: sp.Project, Sprint: sp.Slug}
	if historicalExecutionStatus == "" {
		var verificationErr error
		verification, verificationErr = s.VerificationStatus(projectRef, sprintRef)
		if verificationErr != nil && !errors.Is(verificationErr, ErrFlowStateMissing) {
			return StatusSummary{}, verificationErr
		}
	} else {
		verification.Assessment = AssessmentNotApplicable
		verification.NextAction = "Historical terminal execution evidence is preserved; modern review and smoke evidence is not available."
	}
	var mergeState *MergeState
	if loadedMerge, mergeErr := s.LoadMergeState(projectRef, sprintRef); mergeErr == nil {
		mergeState = &loadedMerge
	} else if !errors.Is(mergeErr, os.ErrNotExist) {
		return StatusSummary{}, mergeErr
	}
	return StatusSummary{
		Project:                   sp.Project,
		Sprint:                    sp.Slug,
		SprintRoot:                workspace.Rel(s.root, sp.Path),
		FlowStatePath:             workspace.Rel(s.root, flowPath),
		Stages:                    stages,
		ExecuteState:              executeState,
		HistoricalExecutionStatus: historicalExecutionStatus,
		ExecutePath:               ArtifactRelPath(sp, StageExecute),
		RunStatePath:              workspace.Rel(s.root, runStatePath),
		Review:                    refreshed.Review,
		ReviewPath:                ArtifactRelPath(sp, StageReview),
		Smoke:                     refreshed.Smoke,
		SmokePath:                 ArtifactRelPath(sp, StageSmoke),
		Merge:                     mergeState,
		MergePath:                 mergeArtifactRelPath(sp),
		QA:                        refreshed.QA,
		Verification:              verification,
	}, nil
}

func (s Service) ValidateSprintIndex(projectRef, sprintRef string) (ValidationResult, error) {
	sp, inputs, catalog, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return ValidationResult{}, err
	}
	_, findings := ValidateSprintIndexContent(inputs.SprintIndex, catalog)
	return ValidationResult{
		Project:  sp.Project,
		Sprint:   sp.Slug,
		Artifact: workspace.Rel(s.root, mustArtifactPath(s.root, sp, StageSprintIndex)),
		Findings: findings,
	}, nil
}

func (s Service) ValidateRequirements(projectRef, sprintRef string) (ValidationResult, error) {
	sp, _, _, err := s.resolveSprintForRequirements(projectRef, sprintRef, false)
	if err != nil {
		return ValidationResult{}, err
	}
	path := mustArtifactPath(s.root, sp, StageRequirements)
	data, err := s.store.ReadArtifact(sp, StageRequirements)
	var findings []ValidationFinding
	if err != nil {
		findings = append(findings, finding("requirements.md", "", workspace.Rel(s.root, path), "missing requirements", err.Error(), "Generate requirements.md before validation."))
	} else {
		findings = append(findings, ValidateRequirementsContent(data)...)
	}
	return ValidationResult{
		Project:  sp.Project,
		Sprint:   sp.Slug,
		Artifact: workspace.Rel(s.root, path),
		Findings: findings,
	}, nil
}

func (s Service) ValidateTechnicalHandbook(projectRef, sprintRef string) (ValidationResult, error) {
	sp, inputs, catalog, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return ValidationResult{}, err
	}
	manifest, findings := BuildHandbookManifest(s.root, sp, inputs, catalog)
	path := mustArtifactPath(s.root, sp, StageTechnicalHandbook)
	data, err := s.store.ReadArtifact(sp, StageTechnicalHandbook)
	if err != nil {
		findings = append(findings, finding("technical-handbook.md", "", workspace.Rel(s.root, path), "missing technical handbook", err.Error(), "Generate technical-handbook.md before validation."))
	} else {
		findings = append(findings, ValidateTechnicalHandbookContent(data, manifest)...)
	}
	sortSprintFindings(findings)
	return ValidationResult{
		Project:  sp.Project,
		Sprint:   sp.Slug,
		Artifact: workspace.Rel(s.root, path),
		Findings: findings,
	}, nil
}

func (s Service) PromptSprintIndex(projectRef, sprintRef string) (PromptPreview, error) {
	sp, inputs, catalog, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return PromptPreview{}, err
	}
	return s.composeSharedPrompt(context.Background(), sp, inputs, RenderSprintIndexPrompt(s.root, sp, catalog, inputs.Docs))
}

func (s Service) PromptTechnicalHandbook(projectRef, sprintRef string) (PromptPreview, error) {
	sp, inputs, catalog, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return PromptPreview{}, err
	}
	manifest, findings := BuildHandbookManifest(s.root, sp, inputs, catalog)
	if len(findings) > 0 {
		return PromptPreview{}, fmt.Errorf("selected evidence validation failed")
	}
	return s.composeSharedPrompt(context.Background(), sp, inputs, RenderTechnicalHandbookPrompt(s.root, manifest))
}

func (s Service) ValidateAreaReasoning(projectRef, sprintRef string) (ValidationResult, error) {
	sp, inputs, catalog, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return ValidationResult{}, err
	}
	manifest, findings := BuildReasoningManifest(s.root, sp, inputs, catalog)
	if len(findings) == 0 {
		for _, entry := range manifest.ReasoningTemplates {
			path, err := workspace.ResolveInside(s.root, normalizeWorkspacePath(entry.OutputPath))
			if err != nil {
				findings = append(findings, finding("area-reasoning", entry.Name, entry.OutputPath, "unsafe area reasoning path", err.Error(), "Use a workspace-contained selected output path."))
				continue
			}
			data, err := s.store.ReadFile(path)
			if err != nil {
				findings = append(findings, finding("area-reasoning", entry.Name, entry.OutputPath, "missing area reasoning", err.Error(), "Generate the selected area reasoning artifact."))
				continue
			}
			findings = append(findings, ValidateAreaReasoningContent(data, entry, manifest)...)
		}
	}
	sortSprintFindings(findings)
	return ValidationResult{Project: sp.Project, Sprint: sp.Slug, Artifact: ArtifactRelPath(sp, StageAreaReasoning), Findings: findings}, nil
}

func (s Service) ValidateReasoning(projectRef, sprintRef string) (ValidationResult, error) {
	sp, inputs, catalog, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return ValidationResult{}, err
	}
	manifest, findings := BuildReasoningManifest(s.root, sp, inputs, catalog)
	if len(findings) == 0 {
		for _, entry := range manifest.ReasoningTemplates {
			path, err := workspace.ResolveInside(s.root, normalizeWorkspacePath(entry.OutputPath))
			if err != nil {
				findings = append(findings, finding("Area-Specific Reasoning Inputs", entry.Name, entry.OutputPath, "unsafe area reasoning path", err.Error(), "Use a workspace-contained selected output path."))
				continue
			}
			data, err := s.store.ReadFile(path)
			if err != nil {
				findings = append(findings, finding("Area-Specific Reasoning Inputs", entry.Name, entry.OutputPath, "missing selected area reasoning", err.Error(), "Generate and validate selected area reasoning before final reasoning."))
				continue
			}
			findings = append(findings, ValidateAreaReasoningContent(data, entry, manifest)...)
		}
	}
	path := mustArtifactPath(s.root, sp, StageReasoning)
	if len(findings) == 0 {
		data, err := s.store.ReadArtifact(sp, StageReasoning)
		if err != nil {
			findings = append(findings, finding("reasoning.md", "", workspace.Rel(s.root, path), "missing final reasoning", err.Error(), "Generate reasoning.md before validation."))
		} else {
			findings = append(findings, ValidateFinalReasoningContent(data, manifest)...)
		}
	}
	sortSprintFindings(findings)
	return ValidationResult{Project: sp.Project, Sprint: sp.Slug, Artifact: workspace.Rel(s.root, path), Findings: findings}, nil
}

func (s Service) ValidatePlan(projectRef, sprintRef string) (ValidationResult, error) {
	sp, inputs, catalog, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return ValidationResult{}, err
	}
	manifest, findings := s.planManifest(sp, inputs, catalog)
	path := mustArtifactPath(s.root, sp, StagePlan)
	if len(findings) == 0 {
		data, err := s.store.ReadArtifact(sp, StagePlan)
		if err != nil {
			findings = append(findings, finding("plan.md", "", workspace.Rel(s.root, path), "missing plan", err.Error(), "Generate plan.md before validation."))
		} else {
			findings = append(findings, ValidatePlanContent(data, manifest)...)
		}
	}
	sortSprintFindings(findings)
	return ValidationResult{Project: sp.Project, Sprint: sp.Slug, Artifact: workspace.Rel(s.root, path), Findings: findings}, nil
}

func (s Service) ValidateExecute(projectRef, sprintRef string) (ValidationResult, error) {
	sp, inputs, catalog, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return ValidationResult{}, err
	}
	manifest, findings := s.planManifest(sp, inputs, catalog)
	path := mustArtifactPath(s.root, sp, StagePlan)
	if len(findings) == 0 {
		if _, targetFindings := s.resolveSprintTarget(sp, inputs.ProjectIndex, false); len(targetFindings) > 0 {
			findings = append(findings, targetFindings...)
		}
	}
	if len(findings) == 0 {
		data, err := s.store.ReadArtifact(sp, StagePlan)
		if err != nil {
			findings = append(findings, finding("plan.md", "", workspace.Rel(s.root, path), "missing plan", err.Error(), "Generate and validate plan.md before execute."))
		} else {
			_, findings = ExtractExecutePlanTasks(data, manifest)
		}
	}
	sortSprintFindings(findings)
	return ValidationResult{Project: sp.Project, Sprint: sp.Slug, Artifact: workspace.Rel(s.root, path), Findings: findings}, nil
}

func (s Service) PromptAreaReasoning(projectRef, sprintRef string) (PromptPreview, error) {
	sp, inputs, catalog, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return PromptPreview{}, err
	}
	manifest, findings := BuildReasoningManifest(s.root, sp, inputs, catalog)
	if len(findings) > 0 {
		return PromptPreview{}, fmt.Errorf("selected reasoning template validation failed")
	}
	if len(manifest.ReasoningTemplates) == 0 {
		return PromptPreview{Project: sp.Project, Sprint: sp.Slug, Prompt: "No selected reasoning templates; area-reasoning is skipped.\n"}, nil
	}
	entry := manifest.ReasoningTemplates[0]
	prompt, err := RenderAreaReasoningPrompt(s.root, manifestForAreaEntry(manifest, entry), entry)
	if err != nil {
		return PromptPreview{}, err
	}
	return s.composeSharedPrompt(context.Background(), sp, inputs, prompt)
}

func (s Service) PromptReasoning(projectRef, sprintRef string) (PromptPreview, error) {
	sp, inputs, catalog, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return PromptPreview{}, err
	}
	manifest, findings := BuildReasoningManifest(s.root, sp, inputs, catalog)
	if len(findings) > 0 {
		return PromptPreview{}, fmt.Errorf("selected reasoning template validation failed")
	}
	prompt, err := RenderFinalReasoningPrompt(s.root, manifest)
	if err != nil {
		return PromptPreview{}, err
	}
	return s.composeSharedPrompt(context.Background(), sp, inputs, prompt)
}

func (s Service) PromptPlan(projectRef, sprintRef string) (PromptPreview, error) {
	sp, inputs, catalog, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return PromptPreview{}, err
	}
	manifest, findings := s.planManifest(sp, inputs, catalog)
	if len(findings) > 0 {
		return PromptPreview{}, fmt.Errorf("plan prerequisites failed validation")
	}
	prompt := RenderPlanPrompt(s.root, manifest)
	return s.composeSharedPrompt(context.Background(), sp, inputs, prompt)
}

func (s Service) PromptRequirements(projectRef, sprintRef string) (PromptPreview, error) {
	sp, catalog, docs, err := s.resolveSprintForRequirements(projectRef, sprintRef, false)
	if err != nil {
		return PromptPreview{}, err
	}
	return RenderRequirementsPrompt(s.root, sp, catalog, docs), nil
}

func (s Service) FlowRequirements(ctx context.Context, projectRef, sprintRef string, req FlowRequest) (FlowResult, error) {
	if err := validateFlowTarget(req.To); err != nil {
		return FlowResult{}, err
	}
	sp, catalog, docs, err := s.resolveSprintForRequirements(projectRef, sprintRef, !req.DryRun)
	if err != nil {
		return FlowResult{}, err
	}
	now := s.now().UTC()
	prompt := RenderRequirementsPrompt(s.root, sp, catalog, docs)
	if req.DryRun {
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: true, Message: prompt.Prompt}, nil
	}
	if s.runtime == nil {
		err := fmt.Errorf("runtime is required for requirements flow")
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Stages: stages}, err
	}
	runtimeReq := s.runtimeRequest(prompt.Prompt, map[string]string{"project": sp.Project, "sprint": sp.Slug, "stage": string(StageRequirements)})
	runtimeReq.Validation = s.requirementsValidationSpec(sp)
	runtimeResult, err := s.startPlanningStageRun(ctx, sp, StageRequirements, runtimeReq)
	if err != nil {
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
	}
	content, err := s.store.ReadArtifact(sp, StageRequirements)
	if err != nil {
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
	}
	findings := ValidateRequirementsContent(content)
	if len(findings) > 0 {
		runtimeResult, findings, err = s.repairGeneratedArtifact(ctx, runtimeReq, runtimeResult, ArtifactRelPath(sp, StageRequirements), findings, func() []ValidationFinding {
			data, readErr := s.store.ReadArtifact(sp, StageRequirements)
			if readErr != nil {
				return []ValidationFinding{finding("requirements.md", "", ArtifactRelPath(sp, StageRequirements), "missing requirements", readErr.Error(), "Generate requirements.md.")}
			}
			return ValidateRequirementsContent(data)
		})
		if err != nil {
			stages := flowFailedStages(sp, req.To, err, now)
			_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
			return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
		}
	}
	if len(findings) > 0 {
		err := fmt.Errorf("generated requirements.md failed validation")
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages, Findings: findings}, err
	}
	stages := flowRequirementsSuccessStages(sp, now)
	if err := SaveFlowState(s.root, sp, NewFlowState(sp, stages, now)); err != nil {
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
	}
	_ = s.cleanupPlanningStageSessions(ctx, sp, StageRequirements, runtimeResult)
	return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages, Message: "requirements complete"}, nil
}

func (s Service) FlowSprintIndex(ctx context.Context, projectRef, sprintRef string, req FlowRequest) (FlowResult, error) {
	if err := validateFlowTarget(req.To); err != nil {
		return FlowResult{}, err
	}
	sp, inputs, catalog, err := s.resolveSprintInputsForFlow(projectRef, sprintRef, !req.DryRun)
	if err != nil {
		return FlowResult{}, err
	}
	now := s.now().UTC()
	if findings, prerequisiteErr := s.codeContextPrerequisite(sp); prerequisiteErr != nil {
		stages := s.flowFailedStages(sp, req.To, prerequisiteErr, now)
		if !req.DryRun {
			_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		}
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: req.DryRun, Stages: stages, Findings: findings}, prerequisiteErr
	}
	if stringsTrim(inputs.Requirements) == "" || containsPlaceholder(inputs.Requirements) {
		err := fmt.Errorf("requirements.md is empty or contains placeholder content")
		stages := flowFailedStages(sp, req.To, err, now)
		if !req.DryRun {
			_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		}
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: req.DryRun, Stages: stages}, err
	}
	prompt, promptErr := s.composeSharedRuntimePrompt(ctx, sp, inputs, RenderSprintIndexPrompt(s.root, sp, catalog, inputs.Docs))
	if promptErr != nil {
		stages := s.flowFailedStages(sp, req.To, promptErr, now)
		if !req.DryRun {
			_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		}
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: req.DryRun, Stages: stages}, promptErr
	}
	if req.DryRun {
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: true, Message: prompt.Prompt}, nil
	}
	if s.runtime == nil {
		err := fmt.Errorf("runtime is required for sprint-index flow")
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Stages: stages}, err
	}
	runtimeReq := s.runtimeRequest(prompt.Prompt, map[string]string{"project": sp.Project, "sprint": sp.Slug, "stage": string(StageSprintIndex)})
	runtimeReq.Validation = s.sprintIndexValidationSpec(sp, catalog)
	runtimeResult, err := s.startPlanningStageRun(ctx, sp, StageSprintIndex, runtimeReq)
	if err != nil {
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
	}
	inputs, err = s.store.ReadPlanningInputs(sp)
	if err != nil {
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
	}
	index, findings := ValidateSprintIndexContent(inputs.SprintIndex, catalog)
	if len(findings) > 0 {
		runtimeResult, findings, err = s.repairGeneratedArtifact(ctx, runtimeReq, runtimeResult, ArtifactRelPath(sp, StageSprintIndex), findings, func() []ValidationFinding {
			updated, readErr := s.store.ReadPlanningInputs(sp)
			if readErr != nil {
				return []ValidationFinding{finding("sprint-index.md", "", ArtifactRelPath(sp, StageSprintIndex), "missing sprint index", readErr.Error(), "Generate sprint-index.md.")}
			}
			var updatedIndex SprintIndex
			updatedIndex, findings = ValidateSprintIndexContent(updated.SprintIndex, catalog)
			index = updatedIndex
			return findings
		})
		if err != nil {
			stages := flowFailedStages(sp, req.To, err, now)
			_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
			return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
		}
	}
	if len(findings) > 0 {
		err := fmt.Errorf("generated sprint-index.md failed validation")
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages, Findings: findings}, err
	}
	stages := flowSprintIndexSuccessStages(sp, index.NoTemplates, now)
	if err := SaveFlowState(s.root, sp, NewFlowState(sp, stages, now)); err != nil {
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
	}
	_ = s.cleanupPlanningStageSessions(ctx, sp, StageSprintIndex, runtimeResult)
	return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages, Message: "sprint-index complete"}, nil
}

func (s Service) FlowPlan(ctx context.Context, projectRef, sprintRef string, req FlowRequest) (FlowResult, error) {
	if err := validateFlowTarget(req.To); err != nil {
		return FlowResult{}, err
	}
	if req.To != StagePlan {
		return FlowResult{}, fmt.Errorf("unsupported plan flow target %q", req.To)
	}
	sp, inputs, catalog, err := s.resolveSprintInputsForFlow(projectRef, sprintRef, !req.DryRun)
	if err != nil {
		return FlowResult{}, err
	}
	now := s.now().UTC()
	manifest, findings := s.planManifest(sp, inputs, catalog)
	sortSprintFindings(findings)
	if len(findings) > 0 {
		err := fmt.Errorf("plan prerequisites failed validation")
		stages := s.flowFailedStages(sp, req.To, err, now)
		if !req.DryRun {
			_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		}
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: req.DryRun, Stages: stages, Findings: findings}, err
	}
	planPrompt := RenderPlanPrompt(s.root, manifest)
	prompt, promptErr := s.composeSharedRuntimePrompt(ctx, sp, inputs, planPrompt)
	if promptErr != nil {
		stages := s.flowFailedStages(sp, req.To, promptErr, now)
		if !req.DryRun {
			_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		}
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: req.DryRun, Stages: stages}, promptErr
	}
	if req.DryRun {
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: true, Message: prompt.Prompt}, nil
	}
	if s.runtime == nil {
		err := fmt.Errorf("runtime is required for plan flow")
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Stages: stages}, err
	}
	runtimeReq := s.runtimeRequest(prompt.Prompt, map[string]string{"project": sp.Project, "sprint": sp.Slug, "stage": string(StagePlan)})
	runtimeReq.Validation = s.planValidationSpec(sp, manifest)
	runtimeResult, err := s.startPlanningStageRun(ctx, sp, StagePlan, runtimeReq)
	if err != nil {
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
	}
	data, err := s.store.ReadArtifact(sp, StagePlan)
	if err != nil {
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
	}
	findings = ValidatePlanContent(data, manifest)
	if len(findings) > 0 {
		runtimeResult, findings, err = s.repairGeneratedArtifact(ctx, runtimeReq, runtimeResult, ArtifactRelPath(sp, StagePlan), findings, func() []ValidationFinding {
			updated, readErr := s.store.ReadArtifact(sp, StagePlan)
			if readErr != nil {
				return []ValidationFinding{finding("plan.md", "", ArtifactRelPath(sp, StagePlan), "missing plan", readErr.Error(), "Generate plan.md.")}
			}
			return ValidatePlanContent(updated, manifest)
		})
		if err != nil {
			stages := flowFailedStages(sp, req.To, err, now)
			_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
			return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
		}
	}
	if len(findings) > 0 {
		err := fmt.Errorf("generated plan.md failed validation")
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages, Findings: findings}, err
	}
	stages := flowPlanSuccessStages(sp, len(manifest.ReasoningTemplates) == 0, now)
	if err := SaveFlowState(s.root, sp, NewFlowState(sp, stages, now)); err != nil {
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
	}
	_ = s.cleanupPlanningStageSessions(ctx, sp, StagePlan, runtimeResult)
	return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages, Message: "plan complete"}, nil
}

func (s Service) FlowTechnicalHandbook(ctx context.Context, projectRef, sprintRef string, req FlowRequest) (FlowResult, error) {
	if err := validateFlowTarget(req.To); err != nil {
		return FlowResult{}, err
	}
	if req.To != StageTechnicalHandbook {
		return FlowResult{}, fmt.Errorf("unsupported technical-handbook flow target %q", req.To)
	}
	sp, inputs, catalog, err := s.resolveSprintInputsForFlow(projectRef, sprintRef, !req.DryRun)
	if err != nil {
		return FlowResult{}, err
	}
	now := s.now().UTC()
	if stringsTrim(inputs.Requirements) == "" || containsPlaceholder(inputs.Requirements) {
		err := fmt.Errorf("requirements.md is empty or contains placeholder content")
		stages := flowFailedStages(sp, req.To, err, now)
		if !req.DryRun {
			_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		}
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: req.DryRun, Stages: stages}, err
	}
	index, _ := ValidateSprintIndexContent(inputs.SprintIndex, catalog)
	manifest, findings := BuildHandbookManifest(s.root, sp, inputs, catalog)
	sortSprintFindings(findings)
	if len(findings) > 0 {
		err := fmt.Errorf("selected evidence validation failed")
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: req.DryRun, Stages: stages, Findings: findings}, err
	}
	prompt, promptErr := s.composeSharedRuntimePrompt(ctx, sp, inputs, RenderTechnicalHandbookPrompt(s.root, manifest))
	if promptErr != nil {
		stages := s.flowFailedStages(sp, req.To, promptErr, now)
		if !req.DryRun {
			_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		}
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: req.DryRun, Stages: stages}, promptErr
	}
	if req.DryRun {
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: true, Message: prompt.Prompt}, nil
	}
	if s.runtime == nil {
		err := fmt.Errorf("runtime is required for technical-handbook flow")
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Stages: stages}, err
	}
	runtimeReq := s.runtimeRequest(prompt.Prompt, map[string]string{"project": sp.Project, "sprint": sp.Slug, "stage": string(StageTechnicalHandbook)})
	runtimeReq.Validation = s.technicalHandbookValidationSpec(sp, manifest)
	runtimeResult, err := s.startPlanningStageRun(ctx, sp, StageTechnicalHandbook, runtimeReq)
	if err != nil {
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
	}
	data, err := s.store.ReadArtifact(sp, StageTechnicalHandbook)
	if err != nil {
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
	}
	findings = ValidateTechnicalHandbookContent(data, manifest)
	if len(findings) > 0 {
		runtimeResult, findings, err = s.repairGeneratedArtifact(ctx, runtimeReq, runtimeResult, ArtifactRelPath(sp, StageTechnicalHandbook), findings, func() []ValidationFinding {
			updated, readErr := s.store.ReadArtifact(sp, StageTechnicalHandbook)
			if readErr != nil {
				return []ValidationFinding{finding("technical-handbook.md", "", ArtifactRelPath(sp, StageTechnicalHandbook), "missing technical handbook", readErr.Error(), "Generate technical-handbook.md.")}
			}
			return ValidateTechnicalHandbookContent(updated, manifest)
		})
		if err != nil {
			stages := flowFailedStages(sp, req.To, err, now)
			_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
			return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
		}
	}
	if len(findings) > 0 {
		err := fmt.Errorf("generated technical-handbook.md failed validation")
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages, Findings: findings}, err
	}
	stages := flowTechnicalHandbookSuccessStages(sp, index.NoTemplates, now)
	if err := SaveFlowState(s.root, sp, NewFlowState(sp, stages, now)); err != nil {
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
	}
	_ = s.cleanupPlanningStageSessions(ctx, sp, StageTechnicalHandbook, runtimeResult)
	return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages, Message: "technical-handbook complete"}, nil
}

func (s Service) FlowReasoning(ctx context.Context, projectRef, sprintRef string, req FlowRequest) (FlowResult, error) {
	if err := validateFlowTarget(req.To); err != nil {
		return FlowResult{}, err
	}
	if req.To != StageAreaReasoning && req.To != StageReasoning {
		return FlowResult{}, fmt.Errorf("unsupported reasoning flow target %q", req.To)
	}
	sp, inputs, catalog, err := s.resolveSprintInputsForFlow(projectRef, sprintRef, !req.DryRun)
	if err != nil {
		return FlowResult{}, err
	}
	now := s.now().UTC()
	if stringsTrim(inputs.Requirements) == "" || containsPlaceholder(inputs.Requirements) {
		err := fmt.Errorf("requirements.md is empty or contains placeholder content")
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: req.DryRun, Stages: stages}, err
	}
	handbookManifest, handbookFindings := BuildHandbookManifest(s.root, sp, inputs, catalog)
	if len(handbookFindings) == 0 {
		data, err := s.store.ReadArtifact(sp, StageTechnicalHandbook)
		if err != nil {
			handbookFindings = append(handbookFindings, finding("technical-handbook.md", "", ArtifactRelPath(sp, StageTechnicalHandbook), "missing technical handbook", err.Error(), "Generate technical-handbook.md before reasoning."))
		} else {
			handbookFindings = append(handbookFindings, ValidateTechnicalHandbookContent(data, handbookManifest)...)
		}
	}
	manifest, findings := BuildReasoningManifest(s.root, sp, inputs, catalog)
	findings = append(findings, handbookFindings...)
	sortSprintFindings(findings)
	if len(findings) > 0 {
		err := fmt.Errorf("reasoning prerequisites failed validation")
		stages := flowFailedStages(sp, req.To, err, now)
		if !req.DryRun {
			_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		}
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: req.DryRun, Stages: stages, Findings: findings}, err
	}
	if req.To == StageAreaReasoning {
		return s.flowAreaReasoning(ctx, sp, inputs, req, manifest, now)
	}
	return s.flowFinalReasoning(ctx, sp, inputs, req, manifest, now)
}

func (s Service) resolveSprintInputs(projectRef, sprintRef string) (Sprint, PlanningInputs, project.ProjectIndex, error) {
	return s.resolveSprintInputsWithCreate(projectRef, sprintRef, false)
}

func (s Service) resolveSprintInputsForFlow(projectRef, sprintRef string, createMissing bool) (Sprint, PlanningInputs, project.ProjectIndex, error) {
	return s.resolveSprintInputsWithCreate(projectRef, sprintRef, createMissing)
}

func (s Service) resolveSprintInputsWithCreate(projectRef, sprintRef string, createMissing bool) (Sprint, PlanningInputs, project.ProjectIndex, error) {
	projects, err := project.DiscoverProjects(s.root)
	if err != nil {
		return Sprint{}, PlanningInputs{}, project.ProjectIndex{}, err
	}
	p, err := project.ResolveProject(projects, projectRef)
	if err != nil {
		return Sprint{}, PlanningInputs{}, project.ProjectIndex{}, err
	}
	sprints, err := DiscoverSprints(s.root, p)
	if err != nil {
		return Sprint{}, PlanningInputs{}, project.ProjectIndex{}, err
	}
	sp, err := ResolveSprint(sprints, sprintRef)
	if err != nil {
		var refErr RefError
		if !createMissing || !errors.As(err, &refErr) || refErr.Ambiguous {
			return Sprint{}, PlanningInputs{}, project.ProjectIndex{}, err
		}
		sp, err = s.createSprintSkeleton(p, sprintRef)
		if err != nil {
			return Sprint{}, PlanningInputs{}, project.ProjectIndex{}, err
		}
	}
	if !inside(p.Path, sp.Path) {
		return Sprint{}, PlanningInputs{}, project.ProjectIndex{}, fmt.Errorf("sprint path mismatch for %q", sp.Slug)
	}
	inputs, err := s.store.ReadPlanningInputs(sp)
	if err != nil {
		return Sprint{}, PlanningInputs{}, project.ProjectIndex{}, err
	}
	catalog, parseFindings := project.ParseProjectIndex(inputs.ProjectIndex)
	if len(parseFindings) > 0 {
		return Sprint{}, PlanningInputs{}, project.ProjectIndex{}, fmt.Errorf("project-index.md has malformed catalog rows")
	}
	return sp, inputs, catalog, nil
}

func (s Service) resolveSprintForRequirements(projectRef, sprintRef string, createMissing bool) (Sprint, project.ProjectIndex, []string, error) {
	projects, err := project.DiscoverProjects(s.root)
	if err != nil {
		return Sprint{}, project.ProjectIndex{}, nil, err
	}
	p, err := project.ResolveProject(projects, projectRef)
	if err != nil {
		return Sprint{}, project.ProjectIndex{}, nil, err
	}
	sprints, err := DiscoverSprints(s.root, p)
	if err != nil {
		return Sprint{}, project.ProjectIndex{}, nil, err
	}
	sp, err := ResolveSprint(sprints, sprintRef)
	if err != nil {
		var refErr RefError
		if !createMissing || !errors.As(err, &refErr) || refErr.Ambiguous {
			return Sprint{}, project.ProjectIndex{}, nil, err
		}
		sp, err = s.createSprintSkeleton(p, sprintRef)
		if err != nil {
			return Sprint{}, project.ProjectIndex{}, nil, err
		}
	}
	if !inside(p.Path, sp.Path) {
		return Sprint{}, project.ProjectIndex{}, nil, fmt.Errorf("sprint path mismatch for %q", sp.Slug)
	}
	data, err := os.ReadFile(filepath.Join(p.Path, "project-index.md"))
	if err != nil {
		return Sprint{}, project.ProjectIndex{}, nil, err
	}
	catalog, parseFindings := project.ParseProjectIndex(string(data))
	if len(parseFindings) > 0 {
		return Sprint{}, project.ProjectIndex{}, nil, fmt.Errorf("project-index.md has malformed catalog rows")
	}
	files, err := project.NewFSStore(s.root).ReadProjectFiles(p)
	if err != nil {
		return Sprint{}, project.ProjectIndex{}, nil, err
	}
	return sp, catalog, files.MarkdownDocs, nil
}

// CreateWorkspace materializes the sprint directory for a project and slug
// without running any flow stage. An existing sprint is returned unchanged.
func (s Service) CreateWorkspace(projectRef, sprintRef string) (Sprint, error) {
	projects, err := project.DiscoverProjects(s.root)
	if err != nil {
		return Sprint{}, err
	}
	p, err := project.ResolveProject(projects, projectRef)
	if err != nil {
		return Sprint{}, err
	}
	sprints, err := DiscoverSprints(s.root, p)
	if err != nil {
		return Sprint{}, err
	}
	sp, err := ResolveSprint(sprints, sprintRef)
	if err != nil {
		var refErr RefError
		if !errors.As(err, &refErr) || refErr.Ambiguous {
			return Sprint{}, err
		}
		return s.createSprintSkeleton(p, sprintRef)
	}
	return sp, nil
}

func (s Service) createSprintSkeleton(p project.Project, sprintRef string) (Sprint, error) {
	slug := strings.TrimSpace(sprintRef)
	if !project.IsSafeName(slug) {
		return Sprint{}, fmt.Errorf("invalid sprint reference %q: use a single safe path segment", sprintRef)
	}
	sprintsDir, err := workspace.ResolveInside(s.root, filepath.ToSlash(filepath.Join("projects", p.Name, "sprints")))
	if err != nil {
		return Sprint{}, err
	}
	path := filepath.Join(sprintsDir, slug)
	sp := Sprint{Project: p.Name, Slug: slug, Path: path}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return Sprint{}, fmt.Errorf("create sprint %s: %w", ArtifactRelPath(sp, ""), err)
	}
	return sp, nil
}

func (s Service) planManifest(sp Sprint, inputs PlanningInputs, catalog project.ProjectIndex) (PlanManifest, []ValidationFinding) {
	var findings []ValidationFinding
	handbookManifest, handbookFindings := BuildHandbookManifest(s.root, sp, inputs, catalog)
	if len(handbookFindings) == 0 {
		if data, err := s.store.ReadArtifact(sp, StageTechnicalHandbook); err != nil {
			handbookFindings = append(handbookFindings, finding("technical-handbook.md", "", ArtifactRelPath(sp, StageTechnicalHandbook), "missing technical handbook", err.Error(), "Generate technical-handbook.md before plan."))
		} else {
			handbookFindings = append(handbookFindings, ValidateTechnicalHandbookContent(data, handbookManifest)...)
		}
	}
	findings = append(findings, handbookFindings...)
	reasoningManifest, reasoningFindings := BuildReasoningManifest(s.root, sp, inputs, catalog)
	findings = append(findings, reasoningFindings...)
	for _, entry := range reasoningManifest.ReasoningTemplates {
		path, pathErr := workspace.ResolveInside(s.root, normalizeWorkspacePath(entry.OutputPath))
		if pathErr != nil {
			findings = append(findings, finding("Area-Specific Reasoning Inputs", entry.Name, entry.OutputPath, "unsafe area reasoning path", pathErr.Error(), "Use a workspace-contained selected output path."))
			continue
		}
		data, readErr := s.store.ReadFile(path)
		if readErr != nil {
			findings = append(findings, finding("Area-Specific Reasoning Inputs", entry.Name, entry.OutputPath, "missing selected area reasoning", readErr.Error(), "Generate and validate selected area reasoning before plan."))
			continue
		}
		findings = append(findings, ValidateAreaReasoningContent(data, entry, reasoningManifest)...)
	}
	var reasoning string
	if len(findings) == 0 {
		data, err := s.store.ReadArtifact(sp, StageReasoning)
		if err != nil {
			findings = append(findings, finding("reasoning.md", "", ArtifactRelPath(sp, StageReasoning), "missing final reasoning", err.Error(), "Generate reasoning.md before plan."))
		} else {
			reasoning = data
			findings = append(findings, ValidateFinalReasoningContent(data, reasoningManifest)...)
		}
	}
	manifest, planFindings := BuildPlanManifest(s.root, sp, inputs, inputs.SprintIndex, reasoning)
	manifest.ReasoningTemplates = reasoningManifest.ReasoningTemplates
	findings = append(findings, planFindings...)
	sortSprintFindings(findings)
	return manifest, findings
}

func (s Service) runtimeRequest(prompt string, metadata map[string]string) pruntime.Request {
	req := s.runtimeConfig
	req.Prompt = prompt
	req.WorkDir = s.root
	req.Metadata = cloneMetadata(req.Metadata, metadata)
	stage := strings.TrimSpace(metadata["stage"])
	promptKind := stage
	if stage == string(VerificationPhaseQA) {
		promptKind += ".investigator"
		if role := strings.TrimSpace(metadata["role"]); role != "" {
			promptKind = stage + "." + role
		}
	}
	project := strings.TrimSpace(metadata["project"])
	sprint := strings.TrimSpace(metadata["sprint"])
	storeOwner := strings.Join([]string{"sprint", project, sprint, stage, strings.TrimSpace(metadata["task"]), strings.TrimSpace(metadata["coverage"]), strings.TrimSpace(metadata["area"])}, ":")
	req.RuntimeStoreOwner = storeOwner
	req.RuntimeStorePath = pruntime.ScopedRuntimeStorePath(filepath.Join(s.root, "projects", project, "sprints", sprint), storeOwner)
	sum := sha256.Sum256([]byte(prompt))
	checksum := hex.EncodeToString(sum[:])
	req.PromptRef = pruntime.PromptReference{
		ID:        "sprint." + promptKind,
		Version:   "1",
		OwnerKind: "sprint",
		OwnerID:   project + "/" + sprint,
		Purpose:   promptKind,
		Checksum:  checksum,
	}
	if req.TraceID == "" {
		trace := sha256.Sum256([]byte(project + "\x00" + sprint + "\x00" + stage + "\x00" + checksum + "\x00" + s.now().UTC().Format(time.RFC3339Nano)))
		req.TraceID = hex.EncodeToString(trace[:16])
	}
	req.Metadata["trace_id"] = req.TraceID
	req.Metadata["prompt_id"] = req.PromptRef.ID
	req.Metadata["prompt_version"] = req.PromptRef.Version
	req.Metadata["prompt_checksum"] = req.PromptRef.Checksum
	explanation := explainComposedPrompt(prompt)
	if explanation.CacheCandidate {
		req.Cache = pruntime.CacheDirective{Key: explanation.CacheKey, BreakpointBytes: explanation.CacheBreakpoint, PrefixDigest: explanation.SharedPrefixDigest, Mode: "stable-prefix"}
		req.Metadata["prompt_prefix_bytes"] = fmt.Sprintf("%d", explanation.SharedPrefixBytes)
		req.Metadata["prompt_suffix_bytes"] = fmt.Sprintf("%d", explanation.StageSuffixBytes)
		req.Metadata["prompt_prefix_sha256"] = explanation.SharedPrefixDigest
		req.Metadata["prompt_cache_transport"] = explanation.CacheTransport
	}
	if stage := PlanningStage(metadata["stage"]); stage != "" {
		contract := InputContract(stage)
		req.Metadata["prompt_required_inputs"] = contract.requiredMetadata()
		if len(contract.Optional) > 0 {
			req.Metadata["prompt_optional_inputs"] = strings.Join(contract.Optional, ",")
		}
		if override, ok := s.runtimeForStage(stage); ok {
			if override.Model != "" {
				req.Provider, req.Model = splitProviderModel(override.Model)
			}
			if override.Variant != "" {
				req.Metadata["variant"] = override.Variant
				req.Metadata["reasoning_effort"] = override.Variant
			}
		}
	}
	if s.runtimeProgress != nil {
		configured := req.OnEvent
		stage := PlanningStage(metadata["stage"])
		task := metadata["task"]
		coverageID := metadata["coverage"]
		req.OnEvent = func(event pruntime.Event) {
			if configured != nil {
				configured(event)
			}
			s.runtimeProgress(RuntimeProgress{Stage: stage, Task: task, CoverageID: coverageID, Event: event})
		}
	}
	return req
}

func (s Service) repairGeneratedArtifact(ctx context.Context, req pruntime.Request, previous pruntime.Result, path string, findings []ValidationFinding, validate func() []ValidationFinding) (pruntime.Result, []ValidationFinding, error) {
	if len(findings) == 0 || s.runtime == nil || previous.SessionID == "" {
		return previous, findings, nil
	}
	repairReq := req
	repairReq.Prompt = buildGeneratedArtifactRepairPromptFromFindings(path, findings)
	repairReq.SessionID = previous.SessionID
	repairReq.SessionAction = "continue"
	repairReq.Metadata = cloneMetadata(repairReq.Metadata, map[string]string{"repair": "validation", "repair_artifact": path})
	sp := Sprint{Project: repairReq.Metadata["project"], Slug: repairReq.Metadata["sprint"], Path: filepath.Join(s.root, "projects", repairReq.Metadata["project"], "sprints", repairReq.Metadata["sprint"])}
	repairResult, err := s.startSprintRuntime(ctx, sp, PlanningStage(repairReq.Metadata["stage"]), repairReq)
	if err != nil {
		return repairResult, findings, err
	}
	return repairResult, validate(), nil
}

func (s Service) flowFailedStages(sp Sprint, target PlanningStage, err error, now time.Time) []StageState {
	snap, snapErr := s.store.ReadArtifacts(sp)
	if snapErr != nil {
		return flowFailedStages(sp, target, err, now)
	}
	stages := DeriveStages(sp, snap, nil)
	for i := range stages {
		if stages[i].Stage == target {
			stages[i].Status = StatusFailed
			stages[i].LastRunAt = &now
			stages[i].Error = safeError(err)
			break
		}
	}
	return stages
}

func splitProviderModel(value string) (string, string) {
	for i, r := range value {
		if r == '/' {
			return value[:i], value[i+1:]
		}
	}
	return "", value
}

func cloneMetadata(base, overlay map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
}

func (s Service) flowAreaReasoning(ctx context.Context, sp Sprint, inputs PlanningInputs, req FlowRequest, manifest ReasoningManifest, now time.Time) (FlowResult, error) {
	if len(manifest.ReasoningTemplates) == 0 {
		stages := flowAreaReasoningSuccessStages(sp, true, now)
		if req.DryRun {
			return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: true, Message: "Area reasoning skipped: no selected reasoning templates.\n"}, nil
		}
		if err := SaveFlowState(s.root, sp, NewFlowState(sp, stages, now)); err != nil {
			return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Stages: stages}, err
		}
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Stages: stages, Message: "area-reasoning skipped"}, nil
	}
	firstManifest := manifestForAreaEntry(manifest, manifest.ReasoningTemplates[0])
	prompt, promptErr := RenderAreaReasoningPrompt(s.root, firstManifest, manifest.ReasoningTemplates[0])
	if promptErr != nil {
		stages := flowFailedStages(sp, req.To, promptErr, now)
		if !req.DryRun {
			_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		}
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: req.DryRun, Stages: stages}, promptErr
	}
	prompt, promptErr = s.composeSharedRuntimePrompt(ctx, sp, inputs, prompt)
	if promptErr != nil {
		stages := s.flowFailedStages(sp, req.To, promptErr, now)
		if !req.DryRun {
			_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		}
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: req.DryRun, Stages: stages}, promptErr
	}
	if req.DryRun {
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: true, Message: prompt.Prompt}, nil
	}
	if s.runtime == nil {
		err := fmt.Errorf("runtime is required for area-reasoning flow")
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Stages: stages}, err
	}
	sharedPrefix, promptErr := s.prepareSharedPromptContext(ctx, sp, inputs, true)
	if promptErr != nil {
		stages := flowFailedStages(sp, req.To, promptErr, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Stages: stages}, promptErr
	}
	var runtimeResult pruntime.Result
	for _, entry := range manifest.ReasoningTemplates {
		if len(s.areaReasoningEntryFindings(manifest, entry)) == 0 {
			continue
		}
		entryManifest := manifestForAreaEntry(manifest, entry)
		entryPreview, renderErr := RenderAreaReasoningPrompt(s.root, entryManifest, entry)
		if renderErr != nil {
			stages := flowFailedStages(sp, req.To, renderErr, now)
			_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
			return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, renderErr
		}
		composed, composeErr := composeStagePromptChecked(sharedPrefix, entryPreview.Prompt)
		if composeErr != nil {
			stages := flowFailedStages(sp, req.To, composeErr, now)
			_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
			return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, composeErr
		}
		runtimeReq := s.runtimeRequest(composed, map[string]string{"project": sp.Project, "sprint": sp.Slug, "stage": string(StageAreaReasoning), "area": entry.Name, "output_path": entry.OutputPath})
		runtimeReq.Validation = s.areaReasoningEntryValidationSpec(manifest, entry)
		result, runErr := s.startPlanningStageRun(ctx, sp, StageAreaReasoning, runtimeReq)
		runtimeResult = result
		if runErr != nil {
			// OpenCode can exit after committing the requested artifact but before
			// emitting its final event. This entry was invalid before the run, so a
			// valid artifact here is durable proof that the requested work finished.
			// Keep the session checkpoint for normal stage cleanup below.
			if len(s.areaReasoningEntryFindings(manifest, entry)) == 0 {
				continue
			}
			stages := flowFailedStages(sp, req.To, runErr, now)
			_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
			return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, runErr
		}
		entryFindings := s.areaReasoningEntryFindings(manifest, entry)
		if len(entryFindings) > 0 {
			result, entryFindings, runErr = s.repairGeneratedArtifact(ctx, runtimeReq, result, entry.OutputPath, entryFindings, func() []ValidationFinding {
				return s.areaReasoningEntryFindings(manifest, entry)
			})
			runtimeResult = result
			if runErr != nil {
				stages := flowFailedStages(sp, req.To, runErr, now)
				_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
				return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, runErr
			}
			if len(entryFindings) > 0 {
				err := fmt.Errorf("generated area reasoning %q failed validation", entry.Name)
				stages := flowFailedStages(sp, req.To, err, now)
				_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
				return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages, Findings: entryFindings}, err
			}
		}
	}
	var findings []ValidationFinding
	for _, entry := range manifest.ReasoningTemplates {
		findings = append(findings, s.areaReasoningEntryFindings(manifest, entry)...)
	}
	sortSprintFindings(findings)
	if len(findings) > 0 {
		err := fmt.Errorf("generated area reasoning failed validation")
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages, Findings: findings}, err
	}
	stages := flowAreaReasoningSuccessStages(sp, false, now)
	if err := SaveFlowState(s.root, sp, NewFlowState(sp, stages, now)); err != nil {
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
	}
	_ = s.cleanupPlanningStageSessions(ctx, sp, StageAreaReasoning, runtimeResult)
	return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages, Message: "area-reasoning complete"}, nil
}

func manifestForAreaEntry(manifest ReasoningManifest, entry ReasoningTemplateEntry) ReasoningManifest {
	manifest.ReasoningTemplates = []ReasoningTemplateEntry{entry}
	return manifest
}

func (s Service) areaReasoningEntryFindings(manifest ReasoningManifest, entry ReasoningTemplateEntry) []ValidationFinding {
	path, pathErr := workspace.ResolveInside(s.root, normalizeWorkspacePath(entry.OutputPath))
	if pathErr != nil {
		return []ValidationFinding{finding("area-reasoning", entry.Name, entry.OutputPath, "unsafe area reasoning path", pathErr.Error(), "Use a workspace-contained selected output path.")}
	}
	data, readErr := s.store.ReadFile(path)
	if readErr != nil {
		return []ValidationFinding{finding("area-reasoning", entry.Name, entry.OutputPath, "missing area reasoning", readErr.Error(), "Generate the selected area reasoning artifact.")}
	}
	return ValidateAreaReasoningContent(data, entry, manifest)
}

func (s Service) flowFinalReasoning(ctx context.Context, sp Sprint, inputs PlanningInputs, req FlowRequest, manifest ReasoningManifest, now time.Time) (FlowResult, error) {
	var findings []ValidationFinding
	for _, entry := range manifest.ReasoningTemplates {
		path, pathErr := workspace.ResolveInside(s.root, normalizeWorkspacePath(entry.OutputPath))
		if pathErr != nil {
			findings = append(findings, finding("Area-Specific Reasoning Inputs", entry.Name, entry.OutputPath, "unsafe area reasoning path", pathErr.Error(), "Use a workspace-contained selected output path."))
			continue
		}
		data, readErr := s.store.ReadFile(path)
		if readErr != nil {
			findings = append(findings, finding("Area-Specific Reasoning Inputs", entry.Name, entry.OutputPath, "missing selected area reasoning", readErr.Error(), "Generate and validate selected area reasoning before final reasoning."))
			continue
		}
		findings = append(findings, ValidateAreaReasoningContent(data, entry, manifest)...)
	}
	sortSprintFindings(findings)
	if len(findings) > 0 {
		err := fmt.Errorf("selected area reasoning failed validation")
		stages := flowFailedStages(sp, req.To, err, now)
		if !req.DryRun {
			_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		}
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: req.DryRun, Stages: stages, Findings: findings}, err
	}
	prompt, promptErr := RenderFinalReasoningPrompt(s.root, manifest)
	if promptErr != nil {
		stages := flowFailedStages(sp, req.To, promptErr, now)
		if !req.DryRun {
			_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		}
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: req.DryRun, Stages: stages}, promptErr
	}
	prompt, promptErr = s.composeSharedRuntimePrompt(ctx, sp, inputs, prompt)
	if promptErr != nil {
		stages := s.flowFailedStages(sp, req.To, promptErr, now)
		if !req.DryRun {
			_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		}
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: req.DryRun, Stages: stages}, promptErr
	}
	if req.DryRun {
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: true, Message: prompt.Prompt}, nil
	}
	if s.runtime == nil {
		err := fmt.Errorf("runtime is required for reasoning flow")
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Stages: stages}, err
	}
	runtimeReq := s.runtimeRequest(prompt.Prompt, map[string]string{"project": sp.Project, "sprint": sp.Slug, "stage": string(StageReasoning)})
	runtimeReq.Validation = s.finalReasoningValidationSpec(sp, manifest)
	runtimeResult, err := s.startPlanningStageRun(ctx, sp, StageReasoning, runtimeReq)
	if err != nil {
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
	}
	data, err := s.store.ReadArtifact(sp, StageReasoning)
	if err != nil {
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
	}
	findings = ValidateFinalReasoningContent(data, manifest)
	if len(findings) > 0 {
		runtimeResult, findings, err = s.repairGeneratedArtifact(ctx, runtimeReq, runtimeResult, ArtifactRelPath(sp, StageReasoning), findings, func() []ValidationFinding {
			updated, readErr := s.store.ReadArtifact(sp, StageReasoning)
			if readErr != nil {
				return []ValidationFinding{finding("reasoning.md", "", ArtifactRelPath(sp, StageReasoning), "missing final reasoning", readErr.Error(), "Generate reasoning.md.")}
			}
			return ValidateFinalReasoningContent(updated, manifest)
		})
		if err != nil {
			stages := flowFailedStages(sp, req.To, err, now)
			_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
			return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
		}
	}
	if len(findings) > 0 {
		err := fmt.Errorf("generated reasoning.md failed validation")
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages, Findings: findings}, err
	}
	stages := flowReasoningSuccessStages(sp, len(manifest.ReasoningTemplates) == 0, now)
	if err := SaveFlowState(s.root, sp, NewFlowState(sp, stages, now)); err != nil {
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
	}
	_ = s.cleanupPlanningStageSessions(ctx, sp, StageReasoning, runtimeResult)
	return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages, Message: "reasoning complete"}, nil
}

func mustArtifactPath(root string, sp Sprint, stage PlanningStage) string {
	path, _ := ArtifactPath(root, sp, stage)
	return path
}

func stringsTrim(value string) string {
	return strings.TrimSpace(value)
}

func DeriveStages(sp Sprint, snap ArtifactSnapshot, prior []StageState) []StageState {
	failed := map[PlanningStage]StageState{}
	previous := map[PlanningStage]StageState{}
	for _, state := range prior {
		previous[state.Stage] = state
		if state.Status == StatusFailed {
			failed[state.Stage] = state
		}
	}
	var out []StageState
	blocked := false
	readyAssigned := false
	for _, stage := range PlanningStages() {
		status := StatusMissing
		switch stage {
		case StageRequirements:
			if snap.Files[stage] {
				if priorState, ok := previous[stage]; !ok || priorState.Status != StatusFailed {
					status = StatusComplete
				}
			}
		case StageCodeContext:
			if snap.Files[stage] {
				if priorState, ok := previous[stage]; ok && priorState.Status == StatusComplete {
					status = StatusComplete
				}
			} else if priorState, ok := previous[stage]; ok && priorState.Status == StatusSkipped {
				status = StatusSkipped
			}
		case StageAreaReasoning:
			if len(snap.AreaReasoningFiles) > 0 {
				status = StatusComplete
			} else if snap.NoReasoningSelected {
				status = StatusSkipped
			}
		default:
			if snap.Files[stage] {
				status = StatusComplete
			}
		}
		if status == StatusMissing {
			if priorFailed, ok := failed[stage]; ok {
				priorFailed.Path = ArtifactRelPath(sp, stage)
				out = append(out, priorFailed)
				blocked = true
				continue
			}
		}
		if status == StatusMissing && !blocked && !readyAssigned {
			status = StatusReady
			readyAssigned = true
		}
		if status == StatusMissing || status == StatusReady || status == StatusFailed {
			blocked = true
		}
		derived := StageState{Stage: stage, Status: status, Path: ArtifactRelPath(sp, stage)}
		if priorState, ok := previous[stage]; ok && priorState.Status == status {
			derived.LastRunAt = priorState.LastRunAt
		}
		out = append(out, derived)
	}
	return out
}

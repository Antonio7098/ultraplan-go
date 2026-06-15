package sprint

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	pruntime "ultraplan-go/internal/platform/runtime"
	"ultraplan-go/internal/project"
	"ultraplan-go/internal/workspace"
)

type Service struct {
	root          string
	store         FSStore
	now           func() time.Time
	runtime       Runtime
	runtimeConfig pruntime.Request
}

func NewService(root string) Service {
	return Service{root: root, store: NewFSStore(root), now: func() time.Time { return time.Now().UTC() }}
}

func (s Service) WithRuntime(rt Runtime, reqs ...pruntime.Request) Service {
	s.runtime = rt
	if len(reqs) > 0 {
		s.runtimeConfig = reqs[0]
	}
	return s
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
	if err := SaveFlowState(s.root, sp, refreshed); err != nil {
		return StatusSummary{}, err
	}
	flowPath, err := FlowStatePath(s.root, sp)
	if err != nil {
		return StatusSummary{}, err
	}
	return StatusSummary{
		Project:       sp.Project,
		Sprint:        sp.Slug,
		SprintRoot:    workspace.Rel(s.root, sp.Path),
		FlowStatePath: workspace.Rel(s.root, flowPath),
		Stages:        stages,
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
	return RenderSprintIndexPrompt(s.root, sp, catalog, inputs.Docs), nil
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
	return RenderTechnicalHandbookPrompt(s.root, manifest), nil
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
	return RenderAreaReasoningPrompt(s.root, manifest, manifest.ReasoningTemplates[0]), nil
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
	return RenderFinalReasoningPrompt(s.root, manifest), nil
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
	return RenderPlanPrompt(s.root, manifest), nil
}

func (s Service) FlowSprintIndex(ctx context.Context, projectRef, sprintRef string, req FlowRequest) (FlowResult, error) {
	if err := validateFlowTarget(req.To); err != nil {
		return FlowResult{}, err
	}
	sp, inputs, catalog, err := s.resolveSprintInputs(projectRef, sprintRef)
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
	prompt := RenderSprintIndexPrompt(s.root, sp, catalog, inputs.Docs)
	if req.DryRun {
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: true, Message: prompt.Prompt}, nil
	}
	if s.runtime == nil {
		err := fmt.Errorf("runtime is required for sprint-index flow")
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Stages: stages}, err
	}
	runtimeResult, err := s.runtime.StartRun(ctx, s.runtimeRequest(prompt.Prompt, map[string]string{"project": sp.Project, "sprint": sp.Slug, "stage": string(StageSprintIndex)}))
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
		err := fmt.Errorf("generated sprint-index.md failed validation")
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages, Findings: findings}, err
	}
	stages := flowSprintIndexSuccessStages(sp, index.NoTemplates, now)
	if err := SaveFlowState(s.root, sp, NewFlowState(sp, stages, now)); err != nil {
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
	}
	return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages, Message: "sprint-index complete"}, nil
}

func (s Service) FlowPlan(ctx context.Context, projectRef, sprintRef string, req FlowRequest) (FlowResult, error) {
	if err := validateFlowTarget(req.To); err != nil {
		return FlowResult{}, err
	}
	if req.To != StagePlan {
		return FlowResult{}, fmt.Errorf("unsupported plan flow target %q", req.To)
	}
	sp, inputs, catalog, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return FlowResult{}, err
	}
	now := s.now().UTC()
	manifest, findings := s.planManifest(sp, inputs, catalog)
	sortSprintFindings(findings)
	if len(findings) > 0 {
		err := fmt.Errorf("plan prerequisites failed validation")
		stages := flowFailedStages(sp, req.To, err, now)
		if !req.DryRun {
			_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		}
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: req.DryRun, Stages: stages, Findings: findings}, err
	}
	prompt := RenderPlanPrompt(s.root, manifest)
	if req.DryRun {
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: true, Message: prompt.Prompt}, nil
	}
	if s.runtime == nil {
		err := fmt.Errorf("runtime is required for plan flow")
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Stages: stages}, err
	}
	runtimeResult, err := s.runtime.StartRun(ctx, s.runtimeRequest(prompt.Prompt, map[string]string{"project": sp.Project, "sprint": sp.Slug, "stage": string(StagePlan)}))
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
		err := fmt.Errorf("generated plan.md failed validation")
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages, Findings: findings}, err
	}
	stages := flowPlanSuccessStages(sp, len(manifest.ReasoningTemplates) == 0, now)
	if err := SaveFlowState(s.root, sp, NewFlowState(sp, stages, now)); err != nil {
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
	}
	return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages, Message: "plan complete"}, nil
}

func (s Service) FlowTechnicalHandbook(ctx context.Context, projectRef, sprintRef string, req FlowRequest) (FlowResult, error) {
	if err := validateFlowTarget(req.To); err != nil {
		return FlowResult{}, err
	}
	if req.To != StageTechnicalHandbook {
		return FlowResult{}, fmt.Errorf("unsupported technical-handbook flow target %q", req.To)
	}
	sp, inputs, catalog, err := s.resolveSprintInputs(projectRef, sprintRef)
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
	prompt := RenderTechnicalHandbookPrompt(s.root, manifest)
	if req.DryRun {
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: true, Message: prompt.Prompt}, nil
	}
	if s.runtime == nil {
		err := fmt.Errorf("runtime is required for technical-handbook flow")
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Stages: stages}, err
	}
	runtimeResult, err := s.runtime.StartRun(ctx, s.runtimeRequest(prompt.Prompt, map[string]string{"project": sp.Project, "sprint": sp.Slug, "stage": string(StageTechnicalHandbook)}))
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
		err := fmt.Errorf("generated technical-handbook.md failed validation")
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages, Findings: findings}, err
	}
	stages := flowTechnicalHandbookSuccessStages(sp, index.NoTemplates, now)
	if err := SaveFlowState(s.root, sp, NewFlowState(sp, stages, now)); err != nil {
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
	}
	return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages, Message: "technical-handbook complete"}, nil
}

func (s Service) FlowReasoning(ctx context.Context, projectRef, sprintRef string, req FlowRequest) (FlowResult, error) {
	if err := validateFlowTarget(req.To); err != nil {
		return FlowResult{}, err
	}
	if req.To != StageAreaReasoning && req.To != StageReasoning {
		return FlowResult{}, fmt.Errorf("unsupported reasoning flow target %q", req.To)
	}
	sp, inputs, catalog, err := s.resolveSprintInputs(projectRef, sprintRef)
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
		return s.flowAreaReasoning(ctx, sp, req, manifest, now)
	}
	return s.flowFinalReasoning(ctx, sp, req, manifest, now)
}

func (s Service) resolveSprintInputs(projectRef, sprintRef string) (Sprint, PlanningInputs, project.ProjectIndex, error) {
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
		return Sprint{}, PlanningInputs{}, project.ProjectIndex{}, err
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
	return req
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

func (s Service) flowAreaReasoning(ctx context.Context, sp Sprint, req FlowRequest, manifest ReasoningManifest, now time.Time) (FlowResult, error) {
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
	prompt := RenderAreaReasoningPrompt(s.root, manifest, manifest.ReasoningTemplates[0])
	if req.DryRun {
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: true, Message: prompt.Prompt}, nil
	}
	if s.runtime == nil {
		err := fmt.Errorf("runtime is required for area-reasoning flow")
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Stages: stages}, err
	}
	runtimeResult, err := s.runtime.StartRun(ctx, s.runtimeRequest(prompt.Prompt, map[string]string{"project": sp.Project, "sprint": sp.Slug, "stage": string(StageAreaReasoning)}))
	if err != nil {
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
	}
	var findings []ValidationFinding
	for _, entry := range manifest.ReasoningTemplates {
		path, pathErr := workspace.ResolveInside(s.root, normalizeWorkspacePath(entry.OutputPath))
		if pathErr != nil {
			findings = append(findings, finding("area-reasoning", entry.Name, entry.OutputPath, "unsafe area reasoning path", pathErr.Error(), "Use a workspace-contained selected output path."))
			continue
		}
		data, readErr := s.store.ReadFile(path)
		if readErr != nil {
			findings = append(findings, finding("area-reasoning", entry.Name, entry.OutputPath, "missing area reasoning", readErr.Error(), "Generate the selected area reasoning artifact."))
			continue
		}
		findings = append(findings, ValidateAreaReasoningContent(data, entry, manifest)...)
	}
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
	return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages, Message: "area-reasoning complete"}, nil
}

func (s Service) flowFinalReasoning(ctx context.Context, sp Sprint, req FlowRequest, manifest ReasoningManifest, now time.Time) (FlowResult, error) {
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
	prompt := RenderFinalReasoningPrompt(s.root, manifest)
	if req.DryRun {
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: true, Message: prompt.Prompt}, nil
	}
	if s.runtime == nil {
		err := fmt.Errorf("runtime is required for reasoning flow")
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Stages: stages}, err
	}
	runtimeResult, err := s.runtime.StartRun(ctx, s.runtimeRequest(prompt.Prompt, map[string]string{"project": sp.Project, "sprint": sp.Slug, "stage": string(StageReasoning)}))
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
		err := fmt.Errorf("generated reasoning.md failed validation")
		stages := flowFailedStages(sp, req.To, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages, Findings: findings}, err
	}
	stages := flowReasoningSuccessStages(sp, len(manifest.ReasoningTemplates) == 0, now)
	if err := SaveFlowState(s.root, sp, NewFlowState(sp, stages, now)); err != nil {
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
	}
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
	for _, state := range prior {
		if state.Status == StatusFailed {
			failed[state.Stage] = state
		}
	}
	var out []StageState
	blocked := false
	readyAssigned := false
	for _, stage := range PlanningStages() {
		if priorFailed, ok := failed[stage]; ok {
			priorFailed.Path = ArtifactRelPath(sp, stage)
			out = append(out, priorFailed)
			blocked = true
			continue
		}
		status := StatusMissing
		switch stage {
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
		if status == StatusMissing && !blocked && !readyAssigned {
			status = StatusReady
			readyAssigned = true
		}
		if status == StatusMissing || status == StatusReady || status == StatusFailed {
			blocked = true
		}
		out = append(out, StageState{Stage: stage, Status: status, Path: ArtifactRelPath(sp, stage)})
	}
	return out
}

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
	root    string
	store   FSStore
	now     func() time.Time
	runtime Runtime
}

func NewService(root string) Service {
	return Service{root: root, store: NewFSStore(root), now: func() time.Time { return time.Now().UTC() }}
}

func (s Service) WithRuntime(rt Runtime) Service {
	s.runtime = rt
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

func (s Service) PromptSprintIndex(projectRef, sprintRef string) (PromptPreview, error) {
	sp, inputs, catalog, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return PromptPreview{}, err
	}
	return RenderSprintIndexPrompt(s.root, sp, catalog, inputs.Docs), nil
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
		stages := flowFailedStages(sp, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: req.DryRun, Stages: stages}, err
	}
	prompt := RenderSprintIndexPrompt(s.root, sp, catalog, inputs.Docs)
	if req.DryRun {
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: true, Message: prompt.Prompt}, nil
	}
	if s.runtime == nil {
		err := fmt.Errorf("runtime is required for sprint-index flow")
		stages := flowFailedStages(sp, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Stages: stages}, err
	}
	runtimeResult, err := s.runtime.StartRun(ctx, pruntime.Request{
		Prompt:   prompt.Prompt,
		WorkDir:  s.root,
		Metadata: map[string]string{"project": sp.Project, "sprint": sp.Slug, "stage": string(StageSprintIndex)},
	})
	if err != nil {
		stages := flowFailedStages(sp, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
	}
	inputs, err = s.store.ReadPlanningInputs(sp)
	if err != nil {
		stages := flowFailedStages(sp, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
	}
	index, findings := ValidateSprintIndexContent(inputs.SprintIndex, catalog)
	if len(findings) > 0 {
		err := fmt.Errorf("generated sprint-index.md failed validation")
		stages := flowFailedStages(sp, err, now)
		_ = SaveFlowState(s.root, sp, NewFlowState(sp, stages, now))
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages, Findings: findings}, err
	}
	stages := flowSuccessStages(sp, index.NoTemplates, now)
	if err := SaveFlowState(s.root, sp, NewFlowState(sp, stages, now)); err != nil {
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages}, err
	}
	return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: runtimeResult, Stages: stages, Message: "sprint-index complete"}, nil
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

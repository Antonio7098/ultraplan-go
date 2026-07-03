package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
	runtimepkg "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
	"github.com/Antonio7098/ultraplan-go/internal/project"
	"github.com/Antonio7098/ultraplan-go/internal/sprint"
	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

var sprintRuntimeFactory = func(c config.Config) (sprint.Runtime, error) {
	return runtimepkg.NewOpenCode(c)
}

func runSprint(deps dependencies, args []string) error {
	if len(args) == 0 {
		return classified(ExitUsage, "sprint requires a subcommand\n\nRun 'ultraplan sprint --help' for usage.")
	}
	if args[0] == "--help" || args[0] == "-h" {
		_, err := deps.stdout.Write([]byte(sprintHelp()))
		return err
	}
	if len(args) >= 4 && (args[3] == "--help" || args[3] == "-h") && args[2] == "status" {
		_, err := deps.stdout.Write([]byte(sprintStatusHelp()))
		return err
	}
	if len(args) >= 4 && (args[3] == "--help" || args[3] == "-h") {
		switch args[2] {
		case "validate":
			_, err := deps.stdout.Write([]byte(sprintValidateHelp()))
			return err
		case "prompt":
			_, err := deps.stdout.Write([]byte(sprintPromptHelp()))
			return err
		case "flow":
			_, err := deps.stdout.Write([]byte(sprintFlowHelp()))
			return err
		}
	}
	if len(args) < 3 {
		if len(args) == 2 {
			return classified(ExitUsage, "sprint: expected '<project> <sprint> status'")
		}
		return classified(ExitUsage, "sprint: expected '<project> <sprint> <status|validate|prompt|flow>'")
	}
	root, err := discoverWorkspace(deps)
	if err != nil {
		return err
	}
	service := sprint.NewService(root.Path)
	switch args[2] {
	case "status":
		if len(args) != 3 {
			return classified(ExitUsage, "sprint: expected '<project> <sprint> status'")
		}
		status, err := service.Status(args[0], args[1])
		if err != nil {
			return mapSprintError("sprint.status", err)
		}
		renderSprintStatus(deps, status)
		return nil
	case "validate":
		if len(args) != 4 {
			return classified(ExitUsage, "sprint.validate: expected 'validate <sprint-index|technical-handbook|area-reasoning|reasoning|plan>'")
		}
		var result sprint.ValidationResult
		var err error
		switch sprint.PlanningStage(args[3]) {
		case sprint.StageSprintIndex:
			result, err = service.ValidateSprintIndex(args[0], args[1])
		case sprint.StageTechnicalHandbook:
			result, err = service.ValidateTechnicalHandbook(args[0], args[1])
		case sprint.StageAreaReasoning:
			result, err = service.ValidateAreaReasoning(args[0], args[1])
		case sprint.StageReasoning:
			result, err = service.ValidateReasoning(args[0], args[1])
		case sprint.StagePlan:
			result, err = service.ValidatePlan(args[0], args[1])
		default:
			return classified(ExitUsage, "sprint.validate: unsupported stage %q", args[3])
		}
		if err != nil {
			return mapSprintError("sprint.validate", err)
		}
		renderSprintValidation(deps, result)
		if !result.Valid() {
			return classified(ExitValidation, "sprint.validate: %s validation failed", args[3])
		}
		return nil
	case "prompt":
		if len(args) != 4 {
			return classified(ExitUsage, "sprint.prompt: expected 'prompt <sprint-index|technical-handbook|area-reasoning|reasoning|plan>'")
		}
		var preview sprint.PromptPreview
		var err error
		switch sprint.PlanningStage(args[3]) {
		case sprint.StageSprintIndex:
			preview, err = service.PromptSprintIndex(args[0], args[1])
		case sprint.StageTechnicalHandbook:
			preview, err = service.PromptTechnicalHandbook(args[0], args[1])
		case sprint.StageAreaReasoning:
			preview, err = service.PromptAreaReasoning(args[0], args[1])
		case sprint.StageReasoning:
			preview, err = service.PromptReasoning(args[0], args[1])
		case sprint.StagePlan:
			preview, err = service.PromptPlan(args[0], args[1])
		default:
			return classified(ExitUsage, "sprint.prompt: unsupported stage %q", args[3])
		}
		if err != nil {
			return mapSprintError("sprint.prompt", err)
		}
		fmt.Fprint(deps.stdout, preview.Prompt)
		return nil
	case "flow":
		req, err := parseSprintFlowArgs(args[3:])
		if err != nil {
			return classified(ExitUsage, "sprint.flow: %w", err)
		}
		flowService := service
		if !req.DryRun {
			flowService, err = sprintRuntimeService(deps, root)
			if err != nil {
				return err
			}
		}
		var result sprint.FlowResult
		result, err = runSprintFlow(deps.ctx, flowService, args[0], args[1], req)
		if result.DryRun && err == nil {
			renderSprintFlow(deps, result)
			return nil
		}
		if err != nil {
			if len(result.Findings) > 0 {
				renderSprintFlow(deps, result)
				return classified(ExitValidation, "sprint.flow: %w", err)
			}
			if strings.Contains(err.Error(), "runtime") {
				return classified(ExitRuntime, "sprint.flow: %w", err)
			}
			return mapSprintError("sprint.flow", err)
		}
		renderSprintFlow(deps, result)
		return nil
	default:
		return classified(ExitUsage, "sprint: unsupported command %q", args[2])
	}
}

func runSprintFlow(ctx context.Context, service sprint.Service, projectRef, sprintRef string, req sprint.FlowRequest) (sprint.FlowResult, error) {
	stages := []sprint.PlanningStage{sprint.StageSprintIndex}
	switch req.To {
	case sprint.StageSprintIndex:
	case sprint.StageTechnicalHandbook:
		stages = append(stages, sprint.StageTechnicalHandbook)
	case sprint.StageAreaReasoning:
		stages = append(stages, sprint.StageTechnicalHandbook, sprint.StageAreaReasoning)
	case sprint.StageReasoning:
		stages = append(stages, sprint.StageTechnicalHandbook, sprint.StageAreaReasoning, sprint.StageReasoning)
	case sprint.StagePlan:
		stages = append(stages, sprint.StageTechnicalHandbook, sprint.StageAreaReasoning, sprint.StageReasoning, sprint.StagePlan)
	default:
		return sprint.FlowResult{}, fmt.Errorf("unsupported flow target %q", req.To)
	}
	if req.DryRun {
		stages = []sprint.PlanningStage{req.To}
	}
	var result sprint.FlowResult
	for _, stage := range stages {
		stageReq := sprint.FlowRequest{To: stage, DryRun: req.DryRun}
		var err error
		switch stage {
		case sprint.StageSprintIndex:
			result, err = service.FlowSprintIndex(ctx, projectRef, sprintRef, stageReq)
		case sprint.StageTechnicalHandbook:
			result, err = service.FlowTechnicalHandbook(ctx, projectRef, sprintRef, stageReq)
		case sprint.StageAreaReasoning, sprint.StageReasoning:
			result, err = service.FlowReasoning(ctx, projectRef, sprintRef, stageReq)
		case sprint.StagePlan:
			result, err = service.FlowPlan(ctx, projectRef, sprintRef, stageReq)
		default:
			err = fmt.Errorf("unsupported flow target %q", stage)
		}
		if err != nil {
			return result, err
		}
	}
	result.To = req.To
	return result, nil
}

func sprintRuntimeService(deps dependencies, root workspace.Root) (sprint.Service, error) {
	effective, err := loadEffectiveConfig(root, deps, config.CLIOverrides{})
	if err != nil {
		return sprint.Service{}, err
	}
	req, err := runtimepkg.RequestFromConfig(effective.Config, root.Path)
	if err != nil {
		return sprint.Service{}, classified(ExitConfig, "runtime.config: %w", err)
	}
	rt, err := sprintRuntimeFactory(effective.Config)
	if err != nil {
		return sprint.Service{}, classified(ExitRuntime, "runtime.init: %w", err)
	}
	return sprint.NewService(root.Path).WithRuntime(rt, req).WithStageRuntime(planningStageRuntime(effective.Config)), nil
}

func planningStageRuntime(c config.Config) map[sprint.PlanningStage]sprint.StageRuntime {
	return map[sprint.PlanningStage]sprint.StageRuntime{
		sprint.StageSprintIndex: {
			Model:   c.Planning.SprintIndexModel,
			Variant: c.Planning.SprintIndexVariant,
		},
		sprint.StageTechnicalHandbook: {
			Model:   c.Planning.TechnicalHandbookModel,
			Variant: c.Planning.TechnicalHandbookVariant,
		},
		sprint.StageAreaReasoning: {
			Model:   c.Planning.AreaReasoningModel,
			Variant: c.Planning.AreaReasoningVariant,
		},
		sprint.StageReasoning: {
			Model:   c.Planning.ReasoningModel,
			Variant: c.Planning.ReasoningVariant,
		},
		sprint.StagePlan: {
			Model:   c.Planning.PlanModel,
			Variant: c.Planning.PlanVariant,
		},
	}
}

func mapSprintError(prefix string, err error) error {
	var projectRef project.RefError
	var sprintRef sprint.RefError
	switch {
	case errors.Is(err, sprint.ErrFlowStateMalformed), errors.Is(err, sprint.ErrFlowStateUnsupported):
		return classified(ExitValidation, "%s: %w", prefix, err)
	case strings.Contains(err.Error(), "validation failed"):
		return classified(ExitValidation, "%s: %w", prefix, err)
	case errors.As(err, &projectRef), errors.As(err, &sprintRef):
		return classified(ExitValidation, "%s: %w", prefix, err)
	default:
		return classified(ExitWorkspace, "%s: %w", prefix, err)
	}
}

func parseSprintFlowArgs(args []string) (sprint.FlowRequest, error) {
	req := sprint.FlowRequest{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--to":
			if i+1 >= len(args) {
				return req, fmt.Errorf("--to requires a stage")
			}
			req.To = sprint.PlanningStage(args[i+1])
			i++
		case "--dry-run":
			req.DryRun = true
		default:
			return req, fmt.Errorf("unsupported argument %q", args[i])
		}
	}
	if req.To == "" {
		return req, fmt.Errorf("--to sprint-index, --to technical-handbook, --to area-reasoning, --to reasoning, or --to plan is required")
	}
	if req.To != sprint.StageSprintIndex && req.To != sprint.StageTechnicalHandbook && req.To != sprint.StageAreaReasoning && req.To != sprint.StageReasoning && req.To != sprint.StagePlan {
		return req, fmt.Errorf("unsupported flow target %q", req.To)
	}
	return req, nil
}

func renderSprintStatus(deps dependencies, status sprint.StatusSummary) {
	fmt.Fprintf(deps.stdout, "Project: %s\n", status.Project)
	fmt.Fprintf(deps.stdout, "Sprint: %s\n", status.Sprint)
	fmt.Fprintf(deps.stdout, "Sprint root: %s\n", status.SprintRoot)
	fmt.Fprintf(deps.stdout, "Flow state: %s\n", status.FlowStatePath)
	fmt.Fprintln(deps.stdout, "Stages:")
	for _, stage := range status.Stages {
		fmt.Fprintf(deps.stdout, "  %s: %s (%s)", stage.Stage, stage.Status, stage.Path)
		if stage.Error != "" {
			fmt.Fprintf(deps.stdout, " error=%q", stage.Error)
		}
		fmt.Fprintln(deps.stdout)
	}
}

func renderSprintValidation(deps dependencies, result sprint.ValidationResult) {
	fmt.Fprintf(deps.stdout, "Project: %s\n", result.Project)
	fmt.Fprintf(deps.stdout, "Sprint: %s\n", result.Sprint)
	fmt.Fprintf(deps.stdout, "Artifact: %s\n", result.Artifact)
	if result.Valid() {
		fmt.Fprintln(deps.stdout, "Validation: ok")
		return
	}
	fmt.Fprintln(deps.stdout, "Validation: failed")
	for _, finding := range result.Findings {
		fmt.Fprintf(deps.stdout, "- %s", finding.Section)
		if finding.EntryName != "" {
			fmt.Fprintf(deps.stdout, " %q", finding.EntryName)
		}
		if finding.Path != "" {
			fmt.Fprintf(deps.stdout, " (%s)", finding.Path)
		}
		fmt.Fprintf(deps.stdout, ": %s", finding.Problem)
		if finding.Cause != "" {
			fmt.Fprintf(deps.stdout, "; %s", finding.Cause)
		}
		if finding.Suggestion != "" {
			fmt.Fprintf(deps.stdout, "; fix: %s", finding.Suggestion)
		}
		fmt.Fprintln(deps.stdout)
	}
}

func renderSprintFlow(deps dependencies, result sprint.FlowResult) {
	fmt.Fprintf(deps.stdout, "Project: %s\n", result.Project)
	fmt.Fprintf(deps.stdout, "Sprint: %s\n", result.Sprint)
	fmt.Fprintf(deps.stdout, "Flow target: %s\n", result.To)
	if result.DryRun {
		fmt.Fprintln(deps.stdout, "Dry run: true")
		fmt.Fprintln(deps.stdout, result.Message)
		return
	}
	if result.Message != "" {
		fmt.Fprintf(deps.stdout, "Result: %s\n", result.Message)
	}
	if len(result.Findings) > 0 {
		fmt.Fprintln(deps.stdout, "Validation findings:")
		for _, finding := range result.Findings {
			fmt.Fprintf(deps.stdout, "- %s", finding.Section)
			if finding.EntryName != "" {
				fmt.Fprintf(deps.stdout, " %q", finding.EntryName)
			}
			if finding.Path != "" {
				fmt.Fprintf(deps.stdout, " (%s)", finding.Path)
			}
			fmt.Fprintf(deps.stdout, ": %s", finding.Problem)
			if finding.Cause != "" {
				fmt.Fprintf(deps.stdout, "; %s", finding.Cause)
			}
			if finding.Suggestion != "" {
				fmt.Fprintf(deps.stdout, "; fix: %s", finding.Suggestion)
			}
			fmt.Fprintln(deps.stdout)
		}
	}
	if len(result.Stages) > 0 {
		fmt.Fprintln(deps.stdout, "Stages:")
		for _, stage := range result.Stages {
			fmt.Fprintf(deps.stdout, "  %s: %s\n", stage.Stage, stage.Status)
		}
	}
}

func sprintHelp() string {
	return `ultraplan sprint

Usage:
  ultraplan sprint <project> <sprint> status
  ultraplan sprint <project> <sprint> validate sprint-index
  ultraplan sprint <project> <sprint> validate technical-handbook
  ultraplan sprint <project> <sprint> validate area-reasoning
  ultraplan sprint <project> <sprint> validate reasoning
  ultraplan sprint <project> <sprint> validate plan
  ultraplan sprint <project> <sprint> prompt sprint-index
  ultraplan sprint <project> <sprint> prompt technical-handbook
  ultraplan sprint <project> <sprint> prompt area-reasoning
  ultraplan sprint <project> <sprint> prompt reasoning
  ultraplan sprint <project> <sprint> prompt plan
  ultraplan sprint <project> <sprint> flow --to sprint-index [--dry-run]
  ultraplan sprint <project> <sprint> flow --to technical-handbook [--dry-run]
  ultraplan sprint <project> <sprint> flow --to area-reasoning [--dry-run]
  ultraplan sprint <project> <sprint> flow --to reasoning [--dry-run]
  ultraplan sprint <project> <sprint> flow --to plan [--dry-run]

Commands:
  <project> <sprint> status  Inspect planning artifacts and refresh flow-state.json.
  <project> <sprint> validate <stage>  Validate sprint-index.md, technical-handbook.md, area reasoning, reasoning.md, or plan.md.
  <project> <sprint> prompt <stage>    Print a runtime-free stage prompt preview.
  <project> <sprint> flow --to <stage> Run or preview sprint planning flow through plan.

Scope:
  Supports planning stages through plan.md only. It does not execute implementation, smoke, review, issue, Git, prompt-generation, or runtime workflows.
`
}

func sprintStatusHelp() string {
	return `ultraplan sprint <project> <sprint> status

Usage:
  ultraplan sprint <project> <sprint> status

Shows deterministic planning-stage status for requirements.md, sprint-index.md, technical-handbook.md, reasoning/*.md, reasoning.md, and plan.md. Missing or valid flow state is refreshed; invalid flow-state.json fails without repair.
`
}

func sprintValidateHelp() string {
	return `ultraplan sprint <project> <sprint> validate

Usage:
  ultraplan sprint <project> <sprint> validate sprint-index
  ultraplan sprint <project> <sprint> validate technical-handbook
  ultraplan sprint <project> <sprint> validate area-reasoning
  ultraplan sprint <project> <sprint> validate reasoning
  ultraplan sprint <project> <sprint> validate plan

Validates sprint-index.md selected context, technical-handbook.md selected evidence distillation, area reasoning, final reasoning.md, or plan.md. Validation failures exit with code 5.
`
}

func sprintPromptHelp() string {
	return `ultraplan sprint <project> <sprint> prompt

Usage:
  ultraplan sprint <project> <sprint> prompt sprint-index
  ultraplan sprint <project> <sprint> prompt technical-handbook
  ultraplan sprint <project> <sprint> prompt area-reasoning
  ultraplan sprint <project> <sprint> prompt reasoning
  ultraplan sprint <project> <sprint> prompt plan

Prints a deterministic runtime-free prompt preview. It does not invoke the runtime and does not write artifacts.
`
}

func sprintFlowHelp() string {
	return `ultraplan sprint <project> <sprint> flow

Usage:
  ultraplan sprint <project> <sprint> flow --to sprint-index [--dry-run]
  ultraplan sprint <project> <sprint> flow --to technical-handbook [--dry-run]
  ultraplan sprint <project> <sprint> flow --to area-reasoning [--dry-run]
  ultraplan sprint <project> <sprint> flow --to reasoning [--dry-run]
  ultraplan sprint <project> <sprint> flow --to plan [--dry-run]

Dry-run prints planned prompt inputs without mutation. Non-dry-run validates prerequisites, invokes the generic runtime boundary, validates the generated artifact, and updates flow-state.json after all gates pass.
`
}

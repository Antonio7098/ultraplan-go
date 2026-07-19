package app

import (
	"context"
	"encoding/json"
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
		return classified(ExitUsage, "sprint: expected '<project> <sprint> <status|validate|prompt|flow|execute|review>'")
	}
	root, err := discoverWorkspace(deps)
	if err != nil {
		return err
	}
	effective, err := loadEffectiveConfig(root, deps, config.CLIOverrides{})
	if err != nil {
		return err
	}
	service := sprint.NewService(root.Path).WithStageRuntime(planningStageRuntime(effective.Config)).WithReviewConcurrency(effective.Config.Execution.DefaultParallel)
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
			return classified(ExitUsage, "sprint.validate: expected 'validate <requirements|sprint-index|technical-handbook|area-reasoning|reasoning|plan>'")
		}
		var result sprint.ValidationResult
		var err error
		switch sprint.PlanningStage(args[3]) {
		case sprint.StageRequirements:
			result, err = service.ValidateRequirements(args[0], args[1])
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
		case sprint.StageExecute:
			result, err = service.ValidateExecute(args[0], args[1])
		case sprint.StageReview:
			result, err = service.ValidateReview(args[0], args[1])
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
			return classified(ExitUsage, "sprint.prompt: expected 'prompt <requirements|sprint-index|technical-handbook|area-reasoning|reasoning|plan>'")
		}
		var preview sprint.PromptPreview
		var err error
		switch sprint.PlanningStage(args[3]) {
		case sprint.StageRequirements:
			preview, err = service.PromptRequirements(args[0], args[1])
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
		case sprint.StageExecute:
			preview, err = service.PromptExecute(args[0], args[1], sprint.ExecuteRequest{})
		case sprint.StageReview:
			preview, err = service.PromptReview(args[0], args[1], sprint.ReviewRequest{})
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
	case "execute":
		req, err := parseSprintExecuteArgs(args[3:])
		if err != nil {
			return classified(ExitUsage, "sprint.execute: %w", err)
		}
		execService := service
		if !req.DryRun {
			execService, err = sprintRuntimeService(deps, root)
			if err != nil {
				return err
			}
		}
		result, err := execService.Execute(deps.ctx, args[0], args[1], req)
		renderSprintExecute(deps, result)
		if err != nil {
			if len(result.Findings) > 0 {
				return classified(ExitValidation, "sprint.execute: %w", err)
			}
			if strings.Contains(err.Error(), "failed tasks") {
				return classified(ExitPartial, "sprint.execute: %w", err)
			}
			if strings.Contains(err.Error(), "runtime") {
				return classified(ExitRuntime, "sprint.execute: %w", err)
			}
			return mapSprintError("sprint.execute", err)
		}
		return nil
	case "review":
		req, jsonOut, err := parseSprintReviewArgs(args[3:])
		if err != nil {
			return classified(ExitUsage, "sprint.review: %w", err)
		}
		reviewService := service
		if !req.DryRun {
			reviewService, err = sprintRuntimeService(deps, root)
			if err != nil {
				return err
			}
		}
		result, runErr := reviewService.Review(deps.ctx, args[0], args[1], req)
		if jsonOut {
			_ = json.NewEncoder(deps.stdout).Encode(map[string]any{"schema_version": 1, "operation": "sprint.review", "status": result.Status, "result": result})
		} else {
			renderSprintReview(deps, result)
		}
		if runErr != nil {
			if result.Verdict == sprint.ReviewFail {
				return classified(ExitValidation, "sprint.review: %w", runErr)
			}
			if result.Status == sprint.ReviewBlocked {
				return classified(ExitValidation, "sprint.review: %w", runErr)
			}
			if strings.Contains(runErr.Error(), "runtime") {
				return classified(ExitRuntime, "sprint.review: %w", runErr)
			}
			return mapSprintError("sprint.review", runErr)
		}
		return nil
	default:
		return classified(ExitUsage, "sprint: unsupported command %q", args[2])
	}
}

func runSprintFlow(ctx context.Context, service sprint.Service, projectRef, sprintRef string, req sprint.FlowRequest) (sprint.FlowResult, error) {
	stages := []sprint.PlanningStage{sprint.StageRequirements}
	switch req.To {
	case sprint.StageRequirements:
	case sprint.StageSprintIndex:
		stages = append(stages, sprint.StageSprintIndex)
	case sprint.StageTechnicalHandbook:
		stages = append(stages, sprint.StageSprintIndex, sprint.StageTechnicalHandbook)
	case sprint.StageAreaReasoning:
		stages = append(stages, sprint.StageSprintIndex, sprint.StageTechnicalHandbook, sprint.StageAreaReasoning)
	case sprint.StageReasoning:
		stages = append(stages, sprint.StageSprintIndex, sprint.StageTechnicalHandbook, sprint.StageAreaReasoning, sprint.StageReasoning)
	case sprint.StagePlan:
		stages = append(stages, sprint.StageSprintIndex, sprint.StageTechnicalHandbook, sprint.StageAreaReasoning, sprint.StageReasoning, sprint.StagePlan)
	case sprint.StageExecute:
		stages = append(stages, sprint.StageSprintIndex, sprint.StageTechnicalHandbook, sprint.StageAreaReasoning, sprint.StageReasoning, sprint.StagePlan, sprint.StageExecute)
	case sprint.StageReview:
		stages = append(stages, sprint.StageSprintIndex, sprint.StageTechnicalHandbook, sprint.StageAreaReasoning, sprint.StageReasoning, sprint.StagePlan, sprint.StageExecute, sprint.StageReview)
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
		if !req.DryRun {
			valid, validateErr := sprintStageAlreadyValid(service, projectRef, sprintRef, stage)
			if validateErr != nil {
				return sprint.FlowResult{}, validateErr
			}
			if valid {
				result = sprint.FlowResult{Project: projectRef, Sprint: sprintRef, To: stage, Message: string(stage) + " already complete"}
				continue
			}
		}
		switch stage {
		case sprint.StageRequirements:
			result, err = service.FlowRequirements(ctx, projectRef, sprintRef, stageReq)
		case sprint.StageSprintIndex:
			result, err = service.FlowSprintIndex(ctx, projectRef, sprintRef, stageReq)
		case sprint.StageTechnicalHandbook:
			result, err = service.FlowTechnicalHandbook(ctx, projectRef, sprintRef, stageReq)
		case sprint.StageAreaReasoning, sprint.StageReasoning:
			result, err = service.FlowReasoning(ctx, projectRef, sprintRef, stageReq)
		case sprint.StagePlan:
			result, err = service.FlowPlan(ctx, projectRef, sprintRef, stageReq)
		case sprint.StageExecute:
			exec, execErr := service.Execute(ctx, projectRef, sprintRef, sprint.ExecuteRequest{DryRun: req.DryRun, Resume: true})
			result = sprint.FlowResult{Project: exec.Project, Sprint: exec.Sprint, To: sprint.StageExecute, DryRun: exec.DryRun, Message: firstNonEmpty(exec.Prompt, exec.Message), Findings: exec.Findings}
			err = execErr
		case sprint.StageReview:
			review, reviewErr := service.Review(ctx, projectRef, sprintRef, sprint.ReviewRequest{DryRun: req.DryRun})
			result = sprint.FlowResult{Project: review.Project, Sprint: review.Sprint, To: sprint.StageReview, DryRun: review.DryRun, Message: firstNonEmpty(review.Prompt, review.Message)}
			err = reviewErr
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

func sprintStageAlreadyValid(service sprint.Service, projectRef, sprintRef string, stage sprint.PlanningStage) (bool, error) {
	var result sprint.ValidationResult
	var err error
	switch stage {
	case sprint.StageRequirements:
		result, err = service.ValidateRequirements(projectRef, sprintRef)
	case sprint.StageSprintIndex:
		result, err = service.ValidateSprintIndex(projectRef, sprintRef)
	case sprint.StageTechnicalHandbook:
		result, err = service.ValidateTechnicalHandbook(projectRef, sprintRef)
	case sprint.StageAreaReasoning:
		result, err = service.ValidateAreaReasoning(projectRef, sprintRef)
	case sprint.StageReasoning:
		result, err = service.ValidateReasoning(projectRef, sprintRef)
	case sprint.StagePlan:
		result, err = service.ValidatePlan(projectRef, sprintRef)
	case sprint.StageExecute:
		return false, nil
	case sprint.StageReview:
		result, err = service.ValidateReview(projectRef, sprintRef)
	default:
		return false, fmt.Errorf("unsupported flow target %q", stage)
	}
	if err != nil {
		return false, nil
	}
	return result.Valid(), nil
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
	return sprint.NewService(root.Path).WithRuntime(rt, req).WithStageRuntime(planningStageRuntime(effective.Config)).WithReviewConcurrency(effective.Config.Execution.DefaultParallel), nil
}

func planningStageRuntime(c config.Config) map[sprint.PlanningStage]sprint.StageRuntime {
	return map[sprint.PlanningStage]sprint.StageRuntime{
		sprint.StageRequirements: {
			Model:   c.Planning.RequirementsModel,
			Variant: c.Planning.RequirementsVariant,
		},
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
		sprint.StageExecute: {
			Model:   c.Planning.ExecuteModel,
			Variant: c.Planning.ExecuteVariant,
		},
		sprint.StageReview: {Model: c.Planning.ReviewModel, Variant: c.Planning.ReviewVariant},
	}
}

func mapSprintError(prefix string, err error) error {
	var projectRef project.RefError
	var sprintRef sprint.RefError
	switch {
	case errors.Is(err, sprint.ErrFlowStateMalformed), errors.Is(err, sprint.ErrFlowStateUnsupported):
		return classified(ExitValidation, "%s: %w", prefix, err)
	case errors.Is(err, sprint.ErrExecuteRunStateMissing), errors.Is(err, sprint.ErrExecuteRunStateMalformed), errors.Is(err, sprint.ErrExecuteRunStateUnsupported):
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
		return req, fmt.Errorf("--to requirements, --to sprint-index, --to technical-handbook, --to area-reasoning, --to reasoning, --to plan, --to execute, or --to review is required")
	}
	if req.To != sprint.StageRequirements && req.To != sprint.StageSprintIndex && req.To != sprint.StageTechnicalHandbook && req.To != sprint.StageAreaReasoning && req.To != sprint.StageReasoning && req.To != sprint.StagePlan && req.To != sprint.StageExecute && req.To != sprint.StageReview {
		return req, fmt.Errorf("unsupported flow target %q", req.To)
	}
	return req, nil
}

func parseSprintReviewArgs(args []string) (sprint.ReviewRequest, bool, error) {
	req := sprint.ReviewRequest{}
	jsonOut := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dry-run", "--prompt":
			req.DryRun = true
		case "--json":
			jsonOut = true
		case "--model":
			if i+1 >= len(args) {
				return req, jsonOut, fmt.Errorf("--model requires a provider/model value")
			}
			i++
			req.ModelOverride = args[i]
		case "--parallel":
			if i+1 >= len(args) {
				return req, jsonOut, fmt.Errorf("--parallel requires a number")
			}
			i++
			var n int
			if _, err := fmt.Sscanf(args[i], "%d", &n); err != nil || n < 1 {
				return req, jsonOut, fmt.Errorf("--parallel must be positive")
			}
			req.Concurrency = n
		default:
			return req, jsonOut, fmt.Errorf("unsupported argument %q", args[i])
		}
	}
	return req, jsonOut, nil
}

func parseSprintExecuteArgs(args []string) (sprint.ExecuteRequest, error) {
	req := sprint.ExecuteRequest{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--task":
			if i+1 >= len(args) {
				return req, fmt.Errorf("--task requires an id")
			}
			req.TaskID = args[i+1]
			i++
		case "--dry-run", "--prompt":
			req.DryRun = true
		case "--resume":
			req.Resume = true
		case "--model":
			if i+1 >= len(args) {
				return req, fmt.Errorf("--model requires a provider/model value")
			}
			req.ModelOverride = args[i+1]
			i++
		default:
			return req, fmt.Errorf("unsupported argument %q", args[i])
		}
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
	fmt.Fprintln(deps.stdout, "Execute:")
	fmt.Fprintf(deps.stdout, "  summary: %s\n", status.ExecutePath)
	fmt.Fprintf(deps.stdout, "  run state: %s\n", status.RunStatePath)
	if status.ExecuteState == nil {
		fmt.Fprintln(deps.stdout, "  status: not started")
	} else {
		counts := map[sprint.ExecuteTaskStatus]int{}
		for _, task := range status.ExecuteState.Tasks {
			counts[task.Status]++
		}
		for _, state := range sprint.ExecuteTaskStatuses() {
			fmt.Fprintf(deps.stdout, "  %s: %d\n", state, counts[state])
		}
	}
	fmt.Fprintln(deps.stdout, "Review:")
	fmt.Fprintf(deps.stdout, "  artifact: %s\n", status.ReviewPath)
	if status.Review == nil {
		fmt.Fprintln(deps.stdout, "  status: not started")
	} else {
		fmt.Fprintf(deps.stdout, "  status: %s\n  verdict: %s\n  stale: %t\n  progress: %d/%d\n", status.Review.Status, status.Review.Verdict, status.Review.Stale, status.Review.Completed, status.Review.Total)
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

func renderSprintExecute(deps dependencies, result sprint.ExecuteResult) {
	fmt.Fprintf(deps.stdout, "Project: %s\n", result.Project)
	fmt.Fprintf(deps.stdout, "Sprint: %s\n", result.Sprint)
	if result.DryRun {
		fmt.Fprintln(deps.stdout, "Dry run: true")
		fmt.Fprintln(deps.stdout, result.Prompt)
		return
	}
	if result.Message != "" {
		fmt.Fprintf(deps.stdout, "Result: %s\n", result.Message)
	}
	if result.RunStatePath != "" {
		fmt.Fprintf(deps.stdout, "Run state: %s\n", result.RunStatePath)
	}
	if result.SummaryPath != "" {
		fmt.Fprintf(deps.stdout, "Summary: %s\n", result.SummaryPath)
	}
	for _, task := range result.Tasks {
		fmt.Fprintf(deps.stdout, "- %s %s attempts=%d\n", task.ID, task.Status, task.Attempts)
	}
	if len(result.Findings) > 0 {
		fmt.Fprintln(deps.stdout, "Validation findings:")
		for _, finding := range result.Findings {
			fmt.Fprintf(deps.stdout, "- %s: %s", finding.Section, finding.Problem)
			if finding.Cause != "" {
				fmt.Fprintf(deps.stdout, "; %s", finding.Cause)
			}
			fmt.Fprintln(deps.stdout)
		}
	}
}

func renderSprintReview(deps dependencies, result sprint.ReviewResult) {
	fmt.Fprintf(deps.stdout, "Project: %s\nSprint: %s\nReview status: %s\nVerdict: %s\nFingerprint: %s\n", result.Project, result.Sprint, result.Status, result.Verdict, result.Fingerprint)
	if result.DryRun {
		fmt.Fprintln(deps.stdout, "Dry run: true")
		fmt.Fprintln(deps.stdout, result.Prompt)
		return
	}
	if result.Artifact != "" {
		fmt.Fprintf(deps.stdout, "Artifact: %s\n", result.Artifact)
	}
	for _, f := range result.Findings {
		fmt.Fprintf(deps.stdout, "- [%s] %s: %s\n", f.Severity, f.Title, f.Detail)
	}
	for _, d := range result.Diagnostics {
		fmt.Fprintf(deps.stdout, "- diagnostic %s: %s\n", d.Code, d.Message)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func sprintHelp() string {
	return `ultraplan sprint

Usage:
  ultraplan sprint <project> <sprint> status
  ultraplan sprint <project> <sprint> validate requirements
  ultraplan sprint <project> <sprint> validate sprint-index
  ultraplan sprint <project> <sprint> validate technical-handbook
  ultraplan sprint <project> <sprint> validate area-reasoning
  ultraplan sprint <project> <sprint> validate reasoning
  ultraplan sprint <project> <sprint> validate plan
  ultraplan sprint <project> <sprint> validate execute
  ultraplan sprint <project> <sprint> validate review
  ultraplan sprint <project> <sprint> prompt requirements
  ultraplan sprint <project> <sprint> prompt sprint-index
  ultraplan sprint <project> <sprint> prompt technical-handbook
  ultraplan sprint <project> <sprint> prompt area-reasoning
  ultraplan sprint <project> <sprint> prompt reasoning
  ultraplan sprint <project> <sprint> prompt plan
  ultraplan sprint <project> <sprint> prompt execute
  ultraplan sprint <project> <sprint> prompt review
  ultraplan sprint <project> <sprint> flow --to requirements [--dry-run]
  ultraplan sprint <project> <sprint> flow --to sprint-index [--dry-run]
  ultraplan sprint <project> <sprint> flow --to technical-handbook [--dry-run]
  ultraplan sprint <project> <sprint> flow --to area-reasoning [--dry-run]
  ultraplan sprint <project> <sprint> flow --to reasoning [--dry-run]
  ultraplan sprint <project> <sprint> flow --to plan [--dry-run]
  ultraplan sprint <project> <sprint> flow --to execute [--dry-run]
  ultraplan sprint <project> <sprint> flow --to review [--dry-run]
  ultraplan sprint <project> <sprint> execute [--task <id>] [--dry-run] [--resume] [--model <provider/model>]
  ultraplan sprint <project> <sprint> review [--dry-run] [--model <provider/model>] [--parallel <n>] [--json]
  execute <project> <sprint> is available as the sprint execute action above.

Commands:
  <project> <sprint> status  Inspect planning artifacts and refresh flow-state.json.
  <project> <sprint> validate <stage>  Validate requirements.md, sprint-index.md, technical-handbook.md, area reasoning, reasoning.md, plan.md, or execute readiness.
  <project> <sprint> prompt <stage>    Print a runtime-free stage prompt preview.
  <project> <sprint> flow --to <stage> Run or preview sprint planning and execute flow.
  <project> <sprint> execute           Execute validated plan tasks through the generic runtime boundary.
  <project> <sprint> review            Run bounded read-only reviewers and atomically write review.md.

Scope:
  Supports governed planning, controlled execute, and automated review. It does not run smoke, issue tracking, Git mutation, hosted/browser, or cross-sprint scheduling workflows.
`
}

func sprintStatusHelp() string {
	return `ultraplan sprint <project> <sprint> status

Usage:
  ultraplan sprint <project> <sprint> status

Shows deterministic planning-stage status for requirements.md, sprint-index.md, technical-handbook.md, reasoning/*.md, reasoning.md, plan.md, and execute run state when present. Missing or valid flow state is refreshed; invalid state fails without repair.
`
}

func sprintValidateHelp() string {
	return `ultraplan sprint <project> <sprint> validate

Usage:
  ultraplan sprint <project> <sprint> validate requirements
  ultraplan sprint <project> <sprint> validate sprint-index
  ultraplan sprint <project> <sprint> validate technical-handbook
  ultraplan sprint <project> <sprint> validate area-reasoning
  ultraplan sprint <project> <sprint> validate reasoning
  ultraplan sprint <project> <sprint> validate plan
  ultraplan sprint <project> <sprint> validate execute
  ultraplan sprint <project> <sprint> validate review

Validates requirements.md, sprint-index.md selected context, technical-handbook.md selected evidence distillation, area reasoning, final reasoning.md, plan.md, or execute readiness. Validation failures exit with code 5.
`
}

func sprintPromptHelp() string {
	return `ultraplan sprint <project> <sprint> prompt

Usage:
  ultraplan sprint <project> <sprint> prompt requirements
  ultraplan sprint <project> <sprint> prompt sprint-index
  ultraplan sprint <project> <sprint> prompt technical-handbook
  ultraplan sprint <project> <sprint> prompt area-reasoning
  ultraplan sprint <project> <sprint> prompt reasoning
  ultraplan sprint <project> <sprint> prompt plan
  ultraplan sprint <project> <sprint> prompt execute
  ultraplan sprint <project> <sprint> prompt review

Prints a deterministic runtime-free prompt preview. Execute prompts are rendered from validated plan tasks and target safety policy. It does not invoke the runtime and does not write artifacts.
`
}

func sprintFlowHelp() string {
	return `ultraplan sprint <project> <sprint> flow

Usage:
  ultraplan sprint <project> <sprint> flow --to requirements [--dry-run]
  ultraplan sprint <project> <sprint> flow --to sprint-index [--dry-run]
  ultraplan sprint <project> <sprint> flow --to technical-handbook [--dry-run]
  ultraplan sprint <project> <sprint> flow --to area-reasoning [--dry-run]
  ultraplan sprint <project> <sprint> flow --to reasoning [--dry-run]
  ultraplan sprint <project> <sprint> flow --to plan [--dry-run]
  ultraplan sprint <project> <sprint> flow --to execute [--dry-run]
  ultraplan sprint <project> <sprint> flow --to review [--dry-run]

Dry-run prints planned prompt inputs without mutation. Non-dry-run validates prerequisites, invokes the generic runtime boundary, validates the generated artifact or execute evidence, and updates durable state after all gates pass.
`
}

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
	runtimepkg "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
	"github.com/Antonio7098/ultraplan-go/internal/project"
	"github.com/Antonio7098/ultraplan-go/internal/sprint"
	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

type SprintRuntimeFactory func(config.Config) (sprint.Runtime, error)

func defaultSprintRuntimeFactory(c config.Config) (sprint.Runtime, error) {
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
		case "metrics":
			_, err := deps.stdout.Write([]byte(sprintMetricsHelp()))
			return err
		case "prompt":
			_, err := deps.stdout.Write([]byte(sprintPromptHelp()))
			return err
		case "flow":
			_, err := deps.stdout.Write([]byte(sprintFlowHelp()))
			return err
		case "smoke":
			_, err := deps.stdout.Write([]byte(sprintSmokeHelp()))
			return err
		case "verify":
			_, err := deps.stdout.Write([]byte(sprintVerifyHelp()))
			return err
		case "execute":
			_, err := deps.stdout.Write([]byte(sprintExecuteHelp()))
			return err
		case "review":
			_, err := deps.stdout.Write([]byte(sprintReviewHelp()))
			return err
		}
	}
	if len(args) < 3 {
		if len(args) == 2 {
			return classified(ExitUsage, "sprint: expected '<project> <sprint> status'")
		}
		return classified(ExitUsage, "sprint: expected '<project> <sprint> <status|metrics|validate|prompt|flow|execute|review|smoke|verify>'")
	}
	root, err := discoverWorkspace(deps)
	if err != nil {
		return err
	}
	effective, err := loadEffectiveConfig(root, deps, config.CLIOverrides{})
	if err != nil {
		return err
	}
	service := sprint.NewService(root.Path).WithStageRuntime(planningStageRuntime(effective.Config)).WithReviewConcurrency(effective.Config.Execution.DefaultParallel).WithSmokeSettings(smokeSettings(effective, envLookup(deps.env)))
	switch args[2] {
	case "status":
		jsonOut := len(args) == 4 && args[3] == "--json"
		if len(args) != 3 && !jsonOut {
			return classified(ExitUsage, "sprint: expected '<project> <sprint> status'")
		}
		status, err := service.Status(args[0], args[1])
		if err != nil {
			return mapSprintError("sprint.status", err)
		}
		statusLabel := "ok"
		smokeReadiness, smokeErr := service.SmokeStatus(args[0], args[1])
		if smokeErr != nil {
			smokeReadiness.Ready = false
			if smokeFailure, ok := sprint.AsSmokeError(smokeErr); ok && (smokeFailure.Category == "catalog" || smokeFailure.Category == "review_gate") {
				statusLabel = "partial"
			} else {
				mapped := mapSmokeError(smokeErr)
				if jsonOut {
					_ = json.NewEncoder(deps.stdout).Encode(map[string]any{"schema_version": 1, "operation": "sprint.status", "status": "failed", "result": status, "smoke_readiness": smokeReadiness, "error": stableCommandError(mapped)})
				}
				return mapped
			}
		}
		if jsonOut {
			return json.NewEncoder(deps.stdout).Encode(map[string]any{"schema_version": 1, "operation": "sprint.status", "status": statusLabel, "result": status, "smoke_readiness": smokeReadiness})
		}
		renderSprintStatus(deps, status)
		fmt.Fprintf(deps.stdout, "  readiness: %t\n", smokeReadiness.Ready)
		for _, diagnostic := range smokeReadiness.Diagnostics {
			fmt.Fprintf(deps.stdout, "  diagnostic: %s\n", diagnostic)
		}
		return nil
	case "metrics":
		jsonOut := len(args) == 4 && args[3] == "--json"
		if len(args) != 3 && !jsonOut {
			return classified(ExitUsage, "sprint.metrics: expected 'metrics [--json]'")
		}
		metrics, err := service.RuntimeMetrics(args[0], args[1])
		if err != nil {
			return mapSprintError("sprint.metrics", err)
		}
		if jsonOut {
			return json.NewEncoder(deps.stdout).Encode(metrics)
		}
		fmt.Fprintf(deps.stdout, "Sprint runtime metrics: %s/%s\nRuns: %d\n", metrics.Project, metrics.Sprint, len(metrics.Runs))
		for _, run := range metrics.Runs {
			fmt.Fprintf(deps.stdout, "- stage=%s operation=%s task=%s coverage=%s provider=%s model=%s prompt=%d prefix=%d suffix=%d input=%s output=%s reasoning=%s cache_read=%s cache_write=%s\n", run.Stage, run.Operation, run.Task, run.Coverage, run.Provider, run.Model, run.PromptBytes, run.SharedPrefixBytes, run.StageSuffixBytes, formatRuntimeTokenMetric(run.InputTokens), formatRuntimeTokenMetric(run.OutputTokens), formatRuntimeTokenMetric(run.ReasoningTokens), formatRuntimeTokenMetric(run.CacheReadTokens), formatRuntimeTokenMetric(run.CacheWriteTokens))
		}
		return nil
	case "validate":
		if len(args) != 4 {
			return classified(ExitUsage, "sprint.validate: expected 'validate <requirements|code-context|sprint-index|technical-handbook|area-reasoning|reasoning|plan>'")
		}
		var result sprint.ValidationResult
		var err error
		switch sprint.PlanningStage(args[3]) {
		case sprint.StageRequirements:
			result, err = service.ValidateRequirements(args[0], args[1])
		case sprint.StageCodeContext:
			result, err = service.ValidateCodeContext(args[0], args[1])
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
		case sprint.StageSmoke:
			result, err = service.ValidateSmoke(args[0], args[1])
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
		explain := len(args) == 5 && args[4] == "--explain"
		if len(args) != 4 && !explain {
			return classified(ExitUsage, "sprint.prompt: expected 'prompt <requirements|code-context|sprint-index|technical-handbook|area-reasoning|reasoning|plan>'")
		}
		var preview sprint.PromptPreview
		var err error
		switch sprint.PlanningStage(args[3]) {
		case sprint.StageRequirements:
			preview, err = service.PromptRequirements(args[0], args[1])
		case sprint.StageCodeContext:
			preview, err = service.PromptCodeContext(args[0], args[1])
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
		if explain {
			if preview.Explanation == nil {
				explanation := sprint.ExplainPrompt(preview.Prompt)
				preview.Explanation = &explanation
			}
			contract := sprint.InputContract(sprint.PlanningStage(args[3]))
			preview.Explanation.InputContract = &contract
			return json.NewEncoder(deps.stdout).Encode(preview.Explanation)
		}
		fmt.Fprint(deps.stdout, preview.Prompt)
		return nil
	case "flow":
		flowArgs, jsonOut := stripFlag(args[3:], "--json")
		req, err := parseSprintFlowArgs(flowArgs)
		if err != nil {
			return classified(ExitUsage, "sprint.flow: %w", err)
		}
		flowService := service
		runCtx := deps.ctx
		var durable *durableCLICommand
		if !req.DryRun {
			if req.To == sprint.StageSmoke && !req.Smoke.NonInteractive {
				return classified(ExitUsage, "sprint.flow: --yes is required for smoke execution")
			}
			req.Progress = renderSprintFlowProgress(deps)
			durable, err = beginDurableCLICommand(deps, OperationRequest{Kind: OperationFlow, Project: args[0], Sprint: args[1], Stage: string(req.To)})
			if err != nil {
				return err
			}
			runCtx = durable.Context()
			flowService, err = sprintRuntimeService(deps, root)
			if err != nil {
				return finishDurableCLICommand(durable, err)
			}
		}
		var result sprint.FlowResult
		result, err = runSprintFlow(runCtx, flowService, args[0], args[1], req)
		err = finishDurableCLICommand(durable, err)
		if result.DryRun && err == nil {
			if jsonOut {
				return json.NewEncoder(deps.stdout).Encode(map[string]any{"schema_version": 1, "operation": "sprint.flow", "status": "ready", "result": result})
			}
			renderSprintFlow(deps, result)
			return nil
		}
		if err != nil {
			if jsonOut {
				_ = json.NewEncoder(deps.stdout).Encode(map[string]any{"schema_version": 1, "operation": "sprint.flow", "status": "failed", "result": result})
				if len(result.Findings) > 0 {
					return classified(ExitValidation, "sprint.flow: %w", err)
				}
				return mapSprintError("sprint.flow", err)
			}
			if len(result.Findings) > 0 {
				renderSprintFlow(deps, result)
				return classified(ExitValidation, "sprint.flow: %w", err)
			}
			if strings.Contains(err.Error(), "runtime") {
				return classified(ExitRuntime, "sprint.flow: %w", err)
			}
			return mapSprintError("sprint.flow", err)
		}
		if jsonOut {
			verification, _ := flowService.VerificationStatus(args[0], args[1])
			return json.NewEncoder(deps.stdout).Encode(map[string]any{"schema_version": 1, "operation": "sprint.flow", "status": "complete", "result": result, "verification": verification})
		}
		renderSprintFlow(deps, result)
		return nil
	case "verify":
		req, jsonOut, err := parseSprintVerifyArgs(args[3:])
		if err != nil {
			return classified(ExitUsage, "sprint.verify: %w", err)
		}
		if req.To == sprint.StageSmoke && !req.DryRun && !req.Smoke.NonInteractive {
			return classified(ExitUsage, "sprint.verify: --yes is required for smoke execution")
		}
		verifyService := service
		runCtx := deps.ctx
		var durable *durableCLICommand
		if !req.DryRun {
			durable, err = beginDurableCLICommand(deps, OperationRequest{Kind: OperationVerifyStart, Project: args[0], Sprint: args[1], Stage: string(req.To)})
			if err != nil {
				return err
			}
			runCtx = durable.Context()
			verifyService, err = sprintRuntimeService(deps, root)
			if err != nil {
				return finishDurableCLICommand(durable, err)
			}
			req.Progress = renderSprintFlowProgress(deps)
		}
		result, runErr := verifyService.Verify(runCtx, args[0], args[1], req)
		runErr = finishDurableCLICommand(durable, runErr)
		var mappedRunErr error
		if runErr != nil {
			if _, ok := sprint.AsSmokeError(runErr); ok {
				mappedRunErr = mapSmokeError(runErr)
			} else {
				mappedRunErr = mapSprintError("sprint.verify", runErr)
			}
		}
		if jsonOut {
			payload := map[string]any{"schema_version": 1, "operation": "sprint.verify", "status": result.Verification.Assessment, "result": result}
			if mappedRunErr != nil {
				payload["error"] = stableCommandError(mappedRunErr)
			}
			_ = json.NewEncoder(deps.stdout).Encode(payload)
		} else {
			renderSprintVerification(deps, result.Verification)
		}
		if mappedRunErr != nil {
			return mappedRunErr
		}
		if result.Verification.Assessment == sprint.AssessmentFail || result.Verification.Assessment == sprint.AssessmentBlocked {
			return classified(ExitValidation, "sprint.verify: assessment %s", result.Verification.Assessment)
		}
		return nil
	case "execute":
		req, err := parseSprintExecuteArgs(args[3:])
		if err != nil {
			return classified(ExitUsage, "sprint.execute: %w", err)
		}
		execService := service
		runCtx := deps.ctx
		var durable *durableCLICommand
		if !req.DryRun && req.DeferReason == "" {
			durable, err = beginDurableCLICommand(deps, OperationRequest{Kind: OperationExecuteStart, Project: args[0], Sprint: args[1], Stage: "execute", Task: req.TaskID})
			if err != nil {
				return err
			}
			runCtx = durable.Context()
			execService, err = sprintRuntimeService(deps, root)
			if err != nil {
				return finishDurableCLICommand(durable, err)
			}
		}
		var result sprint.ExecuteResult
		if req.DeferReason != "" {
			result, err = execService.DeferExecuteTask(deps.ctx, args[0], args[1], req.TaskID, req.DeferReason)
		} else {
			result, err = execService.Execute(runCtx, args[0], args[1], req)
		}
		err = finishDurableCLICommand(durable, err)
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
		runCtx := deps.ctx
		var durable *durableCLICommand
		if !req.DryRun {
			req.Progress = func(progress sprint.ReviewProgress) {
				fmt.Fprintf(deps.stderr, "[sprint] review coverage %d/%d", progress.Completed, progress.Total)
				if progress.CoverageID != "" {
					fmt.Fprintf(deps.stderr, " %s", progress.CoverageID)
				}
				fmt.Fprintf(deps.stderr, ": %s\n", progress.Message)
			}
			durable, err = beginDurableCLICommand(deps, OperationRequest{Kind: OperationReviewStart, Project: args[0], Sprint: args[1], Stage: "review", RestartReview: req.Restart})
			if err != nil {
				return err
			}
			runCtx = durable.Context()
			reviewService, err = sprintRuntimeService(deps, root)
			if err != nil {
				return finishDurableCLICommand(durable, err)
			}
		}
		result, runErr := reviewService.Review(runCtx, args[0], args[1], req)
		runErr = finishDurableCLICommand(durable, runErr)
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
	case "smoke":
		req, jsonOut, err := parseSprintSmokeArgs(args[3:])
		if err != nil {
			return classified(ExitUsage, "sprint.smoke: %w", err)
		}
		if !req.DryRun && !req.NonInteractive {
			return classified(ExitUsage, "sprint.smoke: --yes is required for non-interactive external harness execution")
		}
		if !req.DryRun {
			req.Progress = func(progress sprint.SmokeProgress) {
				fmt.Fprintf(deps.stderr, "[smoke] %-20s", progress.Phase)
				if progress.Test != "" {
					fmt.Fprintf(deps.stderr, " test=%s", progress.Test)
				} else if progress.Suite != "" {
					fmt.Fprintf(deps.stderr, " suite=%s", progress.Suite)
				}
				if progress.Total > 0 {
					fmt.Fprintf(deps.stderr, " %d/%d", progress.Completed, progress.Total)
				}
				fmt.Fprintf(deps.stderr, " | %s\n", config.RedactValue("smoke.progress", progress.Message))
			}
		}
		smokeService := service
		runCtx := deps.ctx
		var durable *durableCLICommand
		if !req.DryRun {
			durable, err = beginDurableCLICommand(deps, OperationRequest{Kind: OperationSmokeStart, Project: args[0], Sprint: args[1], Stage: "smoke", Level: string(req.Level), Suite: req.Suite, Test: req.Test})
			if err != nil {
				return err
			}
			runCtx = durable.Context()
			smokeService, err = sprintRuntimeService(deps, root)
			if err != nil {
				return finishDurableCLICommand(durable, err)
			}
		}
		result, runErr := smokeService.RunSmoke(runCtx, args[0], args[1], req)
		runErr = finishDurableCLICommand(durable, runErr)
		if jsonOut {
			_ = json.NewEncoder(deps.stdout).Encode(map[string]any{"schema_version": 1, "operation": "sprint.smoke", "status": result.Status, "result": result})
		} else {
			renderSprintSmoke(deps, result)
		}
		if runErr != nil {
			return mapSmokeError(runErr)
		}
		if result.Verdict == sprint.SmokeFailVerdict || result.Verdict == sprint.SmokeBlockedVerdict {
			return classified(ExitValidation, "sprint.smoke: verdict %s; %s", result.Verdict, result.NextAction)
		}
		return nil
	default:
		return classified(ExitUsage, "sprint: unsupported command %q", args[2])
	}
}

func stripFlag(args []string, flag string) ([]string, bool) {
	out := make([]string, 0, len(args))
	found := false
	for _, arg := range args {
		if arg == flag {
			found = true
			continue
		}
		out = append(out, arg)
	}
	return out, found
}

func runSprintFlow(ctx context.Context, service sprint.Service, projectRef, sprintRef string, req sprint.FlowRequest) (sprint.FlowResult, error) {
	return service.Flow(ctx, projectRef, sprintRef, req)
}

func sprintRuntimeService(deps dependencies, root workspace.Root, observers ...func(sprint.RuntimeProgress)) (sprint.Service, error) {
	effective, err := loadEffectiveConfig(root, deps, config.CLIOverrides{})
	if err != nil {
		return sprint.Service{}, err
	}
	req, err := runtimepkg.RequestFromConfig(effective.Config, root.Path)
	if err != nil {
		return sprint.Service{}, classified(ExitConfig, "runtime.config: %w", err)
	}
	rt, err := deps.sprintRuntimeFactory(effective.Config)
	if err != nil {
		return sprint.Service{}, classified(ExitRuntime, "runtime.init: %w", err)
	}
	controlled, err := controlledRuntimeFor(deps, root.Path, effective.Config, rt)
	if err != nil {
		return sprint.Service{}, classified(ExitRuntime, "run-control.init: %w", err)
	}
	progress := renderSprintRuntimeProgress(deps)
	if len(observers) > 0 {
		progress = observers[0]
	}
	return sprint.NewService(root.Path).WithRuntime(controlled, req).WithRuntimeProgress(progress).WithStageRuntime(planningStageRuntime(effective.Config)).WithReviewConcurrency(effective.Config.Execution.DefaultParallel).WithSmokeSettings(smokeSettings(effective, envLookup(deps.env))), nil
}

func renderSprintFlowProgress(deps dependencies) func(sprint.FlowProgress) {
	return func(progress sprint.FlowProgress) {
		fmt.Fprintf(deps.stderr, "[sprint] %-18s %-8s %s\n", progress.Stage, progress.State, config.RedactValue("sprint.progress", progress.Message))
	}
}

func renderSprintRuntimeProgress(deps dependencies) func(sprint.RuntimeProgress) {
	var mu sync.Mutex
	return func(progress sprint.RuntimeProgress) {
		if !runtimeEventIsProgress(progress.Event) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		fmt.Fprintf(deps.stderr, "[runtime] %-18s", progress.Stage)
		if progress.Task != "" {
			fmt.Fprintf(deps.stderr, " task=%s", progress.Task)
		}
		if progress.CoverageID != "" {
			fmt.Fprintf(deps.stderr, " coverage=%s", progress.CoverageID)
		}
		fmt.Fprintf(deps.stderr, " | %s\n", runtimeProgressSummary(progress.Event))
	}
}

func smokeSettings(e config.Effective, lookups ...func(string) string) sprint.SmokeSettings {
	discovery, _ := time.ParseDuration(e.Config.Smoke.DiscoveryTimeout)
	run, _ := time.ParseDuration(e.Config.Smoke.RunTimeout)
	cleanup, _ := time.ParseDuration(e.Config.Smoke.CleanupGrace)
	getenv := os.Getenv
	if len(lookups) > 0 && lookups[0] != nil {
		getenv = lookups[0]
	}
	return sprint.SmokeSettings{DiscoveryTimeout: discovery, RunTimeout: run, CleanupGrace: cleanup, StdoutLimit: e.Config.Smoke.StdoutLimit, StderrLimit: e.Config.Smoke.StderrLimit, Environment: append([]string(nil), e.Config.Smoke.Environment...), Sources: e.Sources, Getenv: getenv}
}

func parseSprintSmokeArgs(args []string) (sprint.SmokeRequest, bool, error) {
	var req sprint.SmokeRequest
	jsonOut := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--level", "--suite", "--test", "--timeout", "--override-reason":
			if i+1 >= len(args) {
				return req, jsonOut, fmt.Errorf("%s requires a value", args[i])
			}
			key, value := args[i], args[i+1]
			i++
			switch key {
			case "--level":
				req.Level = value
			case "--suite":
				req.Suite = value
			case "--test":
				req.Test = value
			case "--timeout":
				d, err := time.ParseDuration(value)
				if err != nil || d <= 0 || d > 24*time.Hour {
					return req, jsonOut, fmt.Errorf("--timeout must be positive and no more than 24h")
				}
				req.Timeout = d
			case "--override-reason":
				req.OverrideRationale = value
			}
		case "--force-review":
			req.ForceReview = true
		case "--dry-run", "--preview":
			req.DryRun = true
		case "--yes", "--non-interactive":
			req.NonInteractive, req.OverrideConfirmed = true, true
		case "--json":
			jsonOut = true
		default:
			return req, jsonOut, fmt.Errorf("unsupported argument %q", args[i])
		}
	}
	selected := 0
	for _, value := range []string{req.Level, req.Suite, req.Test} {
		if value != "" {
			selected++
		}
	}
	if selected > 1 {
		return req, jsonOut, fmt.Errorf("choose only one of --level, --suite, or --test")
	}
	if req.ForceReview && strings.TrimSpace(req.OverrideRationale) == "" {
		return req, jsonOut, fmt.Errorf("--force-review requires --override-reason")
	}
	return req, jsonOut, nil
}

func mapSmokeError(err error) error {
	se, ok := sprint.AsSmokeError(err)
	if !ok {
		return mapSprintError("sprint.smoke", err)
	}
	switch se.Category {
	case "cancellation":
		return classifiedCause(ExitCancel, se, "%s", se.Code)
	case "process", "timeout", "cleanup":
		return classifiedCause(ExitRuntime, se, "%s", se.Code)
	default:
		return classifiedCause(ExitValidation, se, "%s", se.Code)
	}
}

func planningStageRuntime(c config.Config) map[sprint.PlanningStage]sprint.StageRuntime {
	codeContextModel := c.Planning.CodeContextModel
	if strings.TrimSpace(codeContextModel) == "" {
		codeContextModel = c.Models.Primary
		if strings.TrimSpace(codeContextModel) == "" {
			codeContextModel = c.Models.Default
		}
	}
	codeContextVariant := c.Planning.CodeContextVariant
	if strings.TrimSpace(codeContextVariant) == "" {
		codeContextVariant = c.Execution.DefaultVariant
	}
	return map[sprint.PlanningStage]sprint.StageRuntime{
		sprint.StageRequirements: {
			Model:   c.Planning.RequirementsModel,
			Variant: c.Planning.RequirementsVariant,
		},
		sprint.StageCodeContext: {
			Model:   codeContextModel,
			Variant: codeContextVariant,
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
		sprint.StageSmoke:  {Model: c.Planning.SmokeModel, Variant: c.Planning.SmokeVariant},
	}
}

func mapSprintError(prefix string, err error) error {
	var projectRef project.RefError
	var sprintRef sprint.RefError
	switch {
	case errors.Is(err, context.Canceled):
		return classified(ExitCancel, "%s: %w", prefix, err)
	case errors.Is(err, context.DeadlineExceeded):
		return classified(ExitRuntime, "%s: %w", prefix, err)
	case errors.Is(err, sprint.ErrVerificationConflict):
		return classified(ExitPartial, "%s: %w", prefix, err)
	case errors.Is(err, sprint.ErrFlowStateMalformed), errors.Is(err, sprint.ErrFlowStateUnsupported):
		return classified(ExitValidation, "%s: %w", prefix, err)
	case errors.Is(err, sprint.ErrExecuteRunStateMissing), errors.Is(err, sprint.ErrExecuteRunStateMalformed), errors.Is(err, sprint.ErrExecuteRunStateUnsupported):
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
		case "--model":
			if i+1 >= len(args) {
				return req, fmt.Errorf("--model requires a provider/model value")
			}
			i++
			req.ModelOverride = args[i]
		case "--variant":
			if i+1 >= len(args) {
				return req, fmt.Errorf("--variant requires a value")
			}
			i++
			req.VariantOverride = args[i]
		case "--restart-review":
			req.Review.Restart = true
		case "--yes", "--non-interactive":
			req.Smoke.NonInteractive, req.Smoke.OverrideConfirmed = true, true
		case "--force-review":
			req.Smoke.ForceReview = true
		case "--override-reason":
			if i+1 >= len(args) {
				return req, fmt.Errorf("--override-reason requires a value")
			}
			i++
			req.Smoke.OverrideRationale = args[i]
		default:
			return req, fmt.Errorf("unsupported argument %q", args[i])
		}
	}
	if req.To == "" {
		return req, fmt.Errorf("--to requirements, --to code-context, --to sprint-index, --to technical-handbook, --to area-reasoning, --to reasoning, --to plan, --to execute, --to review, or --to smoke is required")
	}
	if req.To != sprint.StageRequirements && req.To != sprint.StageCodeContext && req.To != sprint.StageSprintIndex && req.To != sprint.StageTechnicalHandbook && req.To != sprint.StageAreaReasoning && req.To != sprint.StageReasoning && req.To != sprint.StagePlan && req.To != sprint.StageExecute && req.To != sprint.StageReview && req.To != sprint.StageSmoke {
		return req, fmt.Errorf("unsupported flow target %q", req.To)
	}
	if req.Smoke.ForceReview && strings.TrimSpace(req.Smoke.OverrideRationale) == "" {
		return req, fmt.Errorf("--force-review requires --override-reason")
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
		case "--restart":
			req.Restart = true
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
		case "--focus":
			if i+1 >= len(args) {
				return req, jsonOut, fmt.Errorf("--focus requires a coverage id")
			}
			i++
			req.Focus = append(req.Focus, args[i])
		default:
			return req, jsonOut, fmt.Errorf("unsupported argument %q", args[i])
		}
	}
	if req.Restart && len(req.Focus) > 0 {
		return req, jsonOut, fmt.Errorf("--restart cannot be combined with --focus")
	}
	return req, jsonOut, nil
}

func parseSprintVerifyArgs(args []string) (sprint.VerifyRequest, bool, error) {
	req := sprint.VerifyRequest{To: sprint.StageSmoke}
	jsonOut := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--to":
			if i+1 >= len(args) {
				return req, jsonOut, fmt.Errorf("--to requires review or smoke")
			}
			i++
			req.To = sprint.PlanningStage(args[i])
		case "--focus-review":
			if i+1 >= len(args) {
				return req, jsonOut, fmt.Errorf("--focus-review requires a coverage id")
			}
			i++
			req.Review.Focus = append(req.Review.Focus, args[i])
		case "--restart-review":
			req.Review.Restart = true
		case "--level", "--suite", "--test", "--timeout", "--override-reason":
			if i+1 >= len(args) {
				return req, jsonOut, fmt.Errorf("%s requires a value", args[i])
			}
			key, value := args[i], args[i+1]
			i++
			switch key {
			case "--level":
				req.Smoke.Level = value
			case "--suite":
				req.Smoke.Suite = value
			case "--test":
				req.Smoke.Test = value
			case "--override-reason":
				req.Smoke.OverrideRationale = value
			case "--timeout":
				d, parseErr := time.ParseDuration(value)
				if parseErr != nil || d <= 0 || d > 24*time.Hour {
					return req, jsonOut, fmt.Errorf("--timeout must be positive and no more than 24h")
				}
				req.Smoke.Timeout = d
			}
		case "--force-review":
			req.Smoke.ForceReview = true
		case "--yes", "--non-interactive":
			req.Smoke.NonInteractive, req.Smoke.OverrideConfirmed = true, true
		case "--dry-run", "--preview":
			req.DryRun, req.Review.DryRun, req.Smoke.DryRun = true, true, true
		case "--json":
			jsonOut = true
		default:
			return req, jsonOut, fmt.Errorf("unsupported argument %q", args[i])
		}
	}
	if req.To != sprint.StageReview && req.To != sprint.StageSmoke {
		return req, jsonOut, fmt.Errorf("--to must be review or smoke")
	}
	if req.Review.Restart && len(req.Review.Focus) > 0 {
		return req, jsonOut, fmt.Errorf("--restart-review cannot be combined with --focus-review")
	}
	selected := 0
	for _, value := range []string{req.Smoke.Level, req.Smoke.Suite, req.Smoke.Test} {
		if value != "" {
			selected++
		}
	}
	if selected > 1 {
		return req, jsonOut, fmt.Errorf("choose only one of --level, --suite, or --test")
	}
	if req.Smoke.ForceReview && strings.TrimSpace(req.Smoke.OverrideRationale) == "" {
		return req, jsonOut, fmt.Errorf("--force-review requires --override-reason")
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
		case "--defer":
			if i+1 >= len(args) || args[i+1] != "--reason" {
				return req, fmt.Errorf("--defer requires --reason <text>")
			}
			if i+2 >= len(args) {
				return req, fmt.Errorf("--reason requires text")
			}
			req.DeferReason = args[i+2]
			i += 2
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
	if req.DeferReason != "" && req.TaskID == "" {
		return req, fmt.Errorf("--defer requires --task <id>")
	}
	if req.DeferReason != "" && (req.DryRun || req.Resume || req.ModelOverride != "") {
		return req, fmt.Errorf("--defer cannot be combined with --dry-run, --resume, or --model")
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
		if stage.LatestOutcome != "" {
			fmt.Fprintf(deps.stdout, " latest=%s", stage.LatestOutcome)
		}
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
	fmt.Fprintln(deps.stdout, "Smoke:")
	fmt.Fprintf(deps.stdout, "  artifact: %s\n", status.SmokePath)
	if status.Smoke == nil {
		fmt.Fprintln(deps.stdout, "  status: not started")
	} else {
		fmt.Fprintf(deps.stdout, "  status: %s\n  verdict: %s\n  stale: %t\n  run: %s\n  reconciliation required: %t\n", status.Smoke.Status, status.Smoke.Verdict, status.Smoke.Stale, status.Smoke.RunID, status.Smoke.Reconciliation)
	}
	renderSprintVerification(deps, status.Verification)
}

func renderSprintVerification(deps dependencies, status sprint.VerificationStatus) {
	fmt.Fprintln(deps.stdout, "Verification:")
	for _, stage := range []sprint.VerificationStage{status.Review, status.Smoke} {
		fmt.Fprintf(deps.stdout, "  %s: execution=%s verdict=%s fresh=%t artifact=%s", stage.Stage, stage.ExecutionStatus, stage.Verdict, stage.Fresh, stage.Artifact)
		if stage.RunID != "" {
			fmt.Fprintf(deps.stdout, " run=%s", stage.RunID)
		}
		fmt.Fprintln(deps.stdout)
		for _, reason := range stage.FreshnessReasons {
			fmt.Fprintf(deps.stdout, "    stale reason: %s\n", reason)
		}
		for _, issue := range stage.Issues {
			fmt.Fprintf(deps.stdout, "    issue: %s status=%s path=%s\n", issue.ID, issue.Status, issue.Path)
		}
		if stage.Override != nil && stage.Override.Requested {
			fmt.Fprintf(deps.stdout, "    diagnostic override: confirmed=%t rationale=%s\n", stage.Override.Confirmed, stage.Override.Rationale)
		}
	}
	fmt.Fprintf(deps.stdout, "  overall assessment: %s\n", status.Assessment)
	fmt.Fprintf(deps.stdout, "  next action: %s\n", status.NextAction)
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

func renderSprintSmoke(deps dependencies, result sprint.SmokeResult) {
	fmt.Fprintf(deps.stdout, "Project: %s\nSprint: %s\nSmoke status: %s\nVerdict: %s\n", result.Project, result.Sprint, result.Status, result.Verdict)
	fmt.Fprintf(deps.stdout, "Review gate: %s fingerprint=%s override=%t\n", result.ReviewVerdict, result.ReviewFingerprint, result.ReviewOverride)
	fmt.Fprintf(deps.stdout, "Harness: %s protocol=%s\nScope: %s %s\nRationale: %s\n", result.Harness, result.Protocol, result.ScopeKind, result.Scope, result.ScopeRationale)
	if result.AuthorRunID != "" {
		fmt.Fprintf(deps.stdout, "Smoke author: %s model=%s changed=%d\n", result.AuthorRunID, result.AuthorModel, len(result.AuthorChangedPaths))
	}
	fmt.Fprintf(deps.stdout, "Duration/cost class: %s/%s\nEvidence roots: %s\n", result.DurationClass, result.CostClass, strings.Join(result.EvidenceRoots, ", "))
	fmt.Fprintf(deps.stdout, "Effective timeout: %s (source=%s)\n", result.EffectiveTimeout, result.TimeoutSource)
	if result.SafeArgv != "" {
		fmt.Fprintf(deps.stdout, "Safe argv: %s\n", result.SafeArgv)
	}
	if result.RunID != "" {
		fmt.Fprintf(deps.stdout, "Run: %s counts=%d/%d passed failed=%d errors=%d duration=%s\n", result.RunID, result.Counts.Passed, result.Counts.Total, result.Counts.Failed, result.Counts.Errors, result.Duration)
	}
	if result.Artifact != "" {
		fmt.Fprintf(deps.stdout, "Artifact: %s\n", result.Artifact)
	}
	for _, evidence := range result.Evidence {
		fmt.Fprintf(deps.stdout, "Evidence: %s sha256=%s\n", evidence.Path, evidence.SHA256)
	}
	for _, issue := range result.Issues {
		fmt.Fprintf(deps.stdout, "Issue: %s %s %s\n", issue.Status, issue.ID, issue.Path)
	}
	for _, diagnostic := range result.Diagnostics {
		fmt.Fprintf(deps.stdout, "Diagnostic: %s\n", diagnostic)
	}
	if result.NextAction != "" {
		fmt.Fprintf(deps.stdout, "Next action: %s\n", result.NextAction)
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

func stableCommandError(err error) map[string]string {
	code := "internal.error"
	var classed classedError
	if errors.As(err, &classed) {
		code = classed.Code()
	}
	return map[string]string{"code": code, "message": displaySafe(err.Error()), "recovery": "Inspect stderr and sprint status, repair the reported cause, then retry."}
}

func sprintHelp() string {
	return `ultraplan sprint

Usage:
  ultraplan sprint <project> <sprint> status
  ultraplan sprint <project> <sprint> metrics [--json]
  ultraplan sprint <project> <sprint> validate requirements
  ultraplan sprint <project> <sprint> validate code-context
  ultraplan sprint <project> <sprint> validate sprint-index
  ultraplan sprint <project> <sprint> validate technical-handbook
  ultraplan sprint <project> <sprint> validate area-reasoning
  ultraplan sprint <project> <sprint> validate reasoning
  ultraplan sprint <project> <sprint> validate plan
  ultraplan sprint <project> <sprint> validate execute
  ultraplan sprint <project> <sprint> validate review
  ultraplan sprint <project> <sprint> validate smoke
  ultraplan sprint <project> <sprint> prompt requirements
  ultraplan sprint <project> <sprint> prompt code-context
  ultraplan sprint <project> <sprint> prompt sprint-index
  ultraplan sprint <project> <sprint> prompt technical-handbook
  ultraplan sprint <project> <sprint> prompt area-reasoning
  ultraplan sprint <project> <sprint> prompt reasoning
  ultraplan sprint <project> <sprint> prompt plan
  ultraplan sprint <project> <sprint> prompt execute
  ultraplan sprint <project> <sprint> prompt review
  ultraplan sprint <project> <sprint> prompt <stage> --explain
  ultraplan sprint <project> <sprint> flow --to requirements [--dry-run]
  ultraplan sprint <project> <sprint> flow --to code-context [--dry-run] [--model <provider/model>] [--variant <name>]
  ultraplan sprint <project> <sprint> flow --to sprint-index [--dry-run]
  ultraplan sprint <project> <sprint> flow --to technical-handbook [--dry-run]
  ultraplan sprint <project> <sprint> flow --to area-reasoning [--dry-run]
  ultraplan sprint <project> <sprint> flow --to reasoning [--dry-run]
  ultraplan sprint <project> <sprint> flow --to plan [--dry-run]
  ultraplan sprint <project> <sprint> flow --to execute [--dry-run]
  ultraplan sprint <project> <sprint> flow --to review [--restart-review] [--dry-run]
  ultraplan sprint <project> <sprint> flow --to smoke [--restart-review] [--dry-run]
  ultraplan sprint <project> <sprint> execute [--task <id>] [--dry-run] [--resume] [--model <provider/model>]
  ultraplan sprint <project> <sprint> execute --task <id> --defer --reason <text>
  ultraplan sprint <project> <sprint> review [--restart] [--dry-run] [--model <provider/model>] [--parallel <n>] [--json]
  ultraplan sprint <project> <sprint> smoke [--level <id>|--suite <id>|--test <id>] [--timeout <duration>] [--force-review --override-reason <text>] [--dry-run] [--yes] [--json]
  ultraplan sprint <project> <sprint> verify [--to review|smoke] [--focus-review <id>] [--restart-review] [--level <id>|--suite <id>|--test <id>] [--yes] [--json]
  execute <project> <sprint> is available as the sprint execute action above.

Commands:
  <project> <sprint> status  Inspect planning artifacts and refresh flow-state.json.
  <project> <sprint> metrics Inspect persisted prompt/cache/token measurements without raw runtime payloads.
  <project> <sprint> validate <stage>  Validate requirements.md, sprint-index.md, technical-handbook.md, area reasoning, reasoning.md, plan.md, or execute readiness.
  <project> <sprint> prompt <stage>    Print a runtime-free stage prompt preview.
  <project> <sprint> flow --to <stage> Run or preview sprint planning and execute flow.
  <project> <sprint> execute           Execute validated plan tasks through the generic runtime boundary.
  <project> <sprint> review            Run bounded read-only reviewers and atomically write review.md.
  <project> <sprint> smoke             Run the cataloged external harness and atomically write smoke.md.
  <project> <sprint> verify            Run the shared execute-evidence -> review -> smoke transition.

Scope:
  Supports governed planning, controlled execute, automated review, and review-gated smoke. It does not run issue tracking, Git mutation, hosted/browser, or cross-sprint scheduling workflows.
`
}

func sprintStatusHelp() string {
	return `ultraplan sprint <project> <sprint> status

Usage:
  ultraplan sprint <project> <sprint> status

Shows deterministic planning-stage status for requirements.md, code-context.md, sprint-index.md, technical-handbook.md, reasoning/*.md, reasoning.md, plan.md, and execute run state when present. Missing or valid flow state is refreshed; invalid state fails without repair.
`
}

func sprintExecuteHelp() string {
	return `ultraplan sprint <project> <sprint> execute

Usage:
  ultraplan sprint <project> <sprint> execute [--task <id>] [--dry-run] [--resume] [--model <provider/model>]
  ultraplan sprint <project> <sprint> execute --task <id> --defer --reason <text>

Executes validated plan tasks in one reusable agent session. The first turn receives the ordered queue and shared sprint context; later tasks are compact continuation turns with a durable status/evidence checkpoint between them. --resume reuses the latest compatible model/target session when available. Dry-run prints the first frozen execution prompt without invoking the runtime. Deferral requires an explicit task ID and rationale, records both durably, and leaves the plan checkbox visibly unchecked.
`
}

func sprintReviewHelp() string {
	return `ultraplan sprint <project> <sprint> review

Usage:
  ultraplan sprint <project> <sprint> review [--focus <coverage-id>] [--restart] [--dry-run] [--model <provider/model>] [--parallel <n>] [--json]

Runs bounded read-only reviewers. Compatible interrupted attempts resume validated coverage and retained OpenCode sessions by default. --restart discards the resumable attempt and starts every reviewer in a fresh session. A focused rerun requires complete same-fingerprint retained coverage and promotes only a fully validated canonical review.
`
}

func sprintValidateHelp() string {
	return `ultraplan sprint <project> <sprint> validate

Usage:
  ultraplan sprint <project> <sprint> validate requirements
  ultraplan sprint <project> <sprint> validate code-context
  ultraplan sprint <project> <sprint> validate sprint-index
  ultraplan sprint <project> <sprint> validate technical-handbook
  ultraplan sprint <project> <sprint> validate area-reasoning
  ultraplan sprint <project> <sprint> validate reasoning
  ultraplan sprint <project> <sprint> validate plan
  ultraplan sprint <project> <sprint> validate execute
  ultraplan sprint <project> <sprint> validate review
  ultraplan sprint <project> <sprint> validate smoke

Validates requirements.md, code-context.md structural evidence, sprint-index.md selected context, technical-handbook.md selected evidence distillation, area reasoning, final reasoning.md, plan.md, or execute readiness. Validation failures exit with code 5.
`
}

func sprintSmokeHelp() string {
	return `ultraplan sprint <project> <sprint> smoke

Usage:
  ultraplan sprint <project> <sprint> smoke [--level <id>|--suite <id>|--test <id>] [--timeout <duration>] [--force-review --override-reason <text>] [--dry-run] [--yes] [--json]

Discovers the protocol-v1 harness from project-index.md, requires a current review, selects the narrowest sufficient scope, and executes a direct bounded process. Raw evidence stays in the harness; a validated smoke.md and flow state are written only for authoritative results.

Flags:
  --level <id>       Select a discovered level.
  --suite <id>       Select a discovered suite.
  --test <id>        Run a diagnostic test; it is authoritative only when discovery proves complete equivalence.
  --timeout <value>  Positive bounded run timeout, maximum 24h.
  --force-review     Permit an explicitly diagnostic run after a current fail/blocked review.
  --override-reason  Required non-empty actor-neutral rationale for --force-review.
  --dry-run          Discover and preview without launching the run command or writing artifacts.
  --yes              Mark non-interactive confirmation explicit.
  --json             Emit stable JSON without native harness streams.
`
}

func sprintVerifyHelp() string {
	return `ultraplan sprint <project> <sprint> verify

Usage:
  ultraplan sprint <project> <sprint> verify [--to review|smoke] [--focus-review <coverage-id>] [--restart-review] [--level <id>|--suite <id>|--test <id>] [--timeout <duration>] [--force-review --override-reason <text>] [--dry-run] [--yes] [--json]

Requires complete execute evidence, obtains a current review, then applies the review gate before smoke. Focused review results promote only with complete same-fingerprint retained coverage. Test/level smoke is diagnostic unless the harness proves containing coverage. A review override requires --yes and a rationale, remains diagnostic, and cannot improve the overall assessment.
`
}

func sprintMetricsHelp() string {
	return `ultraplan sprint <project> <sprint> metrics

Usage:
  ultraplan sprint <project> <sprint> metrics [--json]

Prints bounded, content-free measurements persisted for sprint runtime calls: prompt and stable-prefix bytes, provider-reported tokens, cache reads/writes, model identity, and run status. Unknown provider metrics are printed as n/a. It does not invoke the runtime or read raw runtime payloads.
`
}

func sprintPromptHelp() string {
	return `ultraplan sprint <project> <sprint> prompt

Usage:
  ultraplan sprint <project> <sprint> prompt requirements
  ultraplan sprint <project> <sprint> prompt code-context
  ultraplan sprint <project> <sprint> prompt sprint-index
  ultraplan sprint <project> <sprint> prompt technical-handbook
  ultraplan sprint <project> <sprint> prompt area-reasoning
  ultraplan sprint <project> <sprint> prompt reasoning
  ultraplan sprint <project> <sprint> prompt plan
  ultraplan sprint <project> <sprint> prompt execute
  ultraplan sprint <project> <sprint> prompt review
  ultraplan sprint <project> <sprint> prompt <stage> --explain

Prints a deterministic runtime-free prompt preview. Execute prompts are rendered from validated plan tasks and target safety policy. --explain emits the ordered prompt-block contract, sizes, digests, cache metadata, and required inputs as JSON. It does not invoke the runtime and does not write artifacts.
`
}

func formatRuntimeTokenMetric(metric sprint.RuntimeTokenMetric) string {
	if !metric.Known {
		return "n/a"
	}
	return strconv.FormatInt(metric.Value, 10)
}

func sprintFlowHelp() string {
	return `ultraplan sprint <project> <sprint> flow

Usage:
  ultraplan sprint <project> <sprint> flow --to requirements [--dry-run]
  ultraplan sprint <project> <sprint> flow --to code-context [--dry-run] [--model <provider/model>] [--variant <name>]
  ultraplan sprint <project> <sprint> flow --to sprint-index [--dry-run]
  ultraplan sprint <project> <sprint> flow --to technical-handbook [--dry-run]
  ultraplan sprint <project> <sprint> flow --to area-reasoning [--dry-run]
  ultraplan sprint <project> <sprint> flow --to reasoning [--dry-run]
  ultraplan sprint <project> <sprint> flow --to plan [--dry-run]
  ultraplan sprint <project> <sprint> flow --to execute [--dry-run]
  ultraplan sprint <project> <sprint> flow --to review [--restart-review] [--dry-run]
  ultraplan sprint <project> <sprint> flow --to smoke [--restart-review] [--dry-run] [--yes]

Dry-run prints planned inputs without mutation. Non-dry-run validates prerequisites and uses the same sprint-owned review-to-smoke transition as verify. Smoke requires --yes; a diagnostic review override additionally requires --force-review and --override-reason.
`
}

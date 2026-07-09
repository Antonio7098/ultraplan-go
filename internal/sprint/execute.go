package sprint

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

const StageExecute PlanningStage = "execute"

type ExecuteRequest struct {
	TaskID        string
	DryRun        bool
	Resume        bool
	ModelOverride string
}

type ExecuteResult struct {
	Project      string
	Sprint       string
	DryRun       bool
	Prompt       string
	RunStatePath string
	SummaryPath  string
	Tasks        []ExecuteTaskRecord
	Findings     []ValidationFinding
	Runtime      []pruntime.Result
	Message      string
}

func (s Service) PromptExecute(projectRef, sprintRef string, req ExecuteRequest) (PromptPreview, error) {
	sp, tasks, target, selection, findings, err := s.prepareExecute(projectRef, sprintRef, req)
	if err != nil {
		return PromptPreview{}, err
	}
	if len(findings) > 0 {
		return PromptPreview{}, fmt.Errorf("execute prerequisites failed validation")
	}
	task := tasks[0]
	if req.TaskID != "" {
		for _, candidate := range tasks {
			if candidate.ID == req.TaskID {
				task = candidate
				break
			}
		}
	}
	return PromptPreview{Project: sp.Project, Sprint: sp.Slug, Prompt: RenderExecutePrompt(sp, task, target, selection)}, nil
}

func (s Service) Execute(ctx context.Context, projectRef, sprintRef string, req ExecuteRequest) (ExecuteResult, error) {
	sp, tasks, target, selection, findings, err := s.prepareExecute(projectRef, sprintRef, req)
	if err != nil {
		return ExecuteResult{}, err
	}
	result := ExecuteResult{Project: sp.Project, Sprint: sp.Slug, DryRun: req.DryRun, Findings: findings}
	if len(findings) > 0 {
		return result, fmt.Errorf("execute prerequisites failed validation")
	}
	if req.DryRun {
		promptTask := tasks[0]
		if req.TaskID != "" {
			for _, task := range tasks {
				if task.ID == req.TaskID {
					promptTask = task
					break
				}
			}
		}
		result.Prompt = RenderExecutePrompt(sp, promptTask, target, selection)
		result.Message = "execute dry run"
		return result, nil
	}
	if s.runtime == nil {
		return result, fmt.Errorf("runtime is required for execute")
	}
	now := s.now().UTC()
	records := ExecuteTasksToRecords(tasks, func() time.Time { return now })
	state := NewExecuteRunState(sp, target, ArtifactRelPath(sp, StagePlan), PlanFingerprint(mustReadPlan(s, sp)), records, now)
	if existing, loadErr := LoadExecuteRunState(s.root, sp); loadErr == nil && req.Resume {
		state = reconcileExecuteState(existing, records, now)
	}
	if err := SaveExecuteRunState(s.root, sp, state); err != nil {
		return result, err
	}
	for i := range state.Tasks {
		task := &state.Tasks[i]
		if req.TaskID != "" && task.ID != req.TaskID {
			continue
		}
		if task.Status == ExecuteTaskComplete {
			continue
		}
		if task.Status == ExecuteTaskRunning {
			task.Status = ExecuteTaskFailed
			task.CompletedAt = ptrTime(now)
			task.Diagnostics = append(task.Diagnostics, ExecuteDiagnostic{Code: "stale-running", Message: "recovered stale running task before resume", At: now})
		}
		if task.Status != ExecuteTaskPending && task.Status != ExecuteTaskFailed {
			continue
		}
		start := s.now().UTC()
		task.Status = ExecuteTaskRunning
		task.Attempts++
		task.StartedAt = &start
		task.UpdatedAt = start
		_ = SaveExecuteRunState(s.root, sp, state)
		planTask := taskByID(tasks, task.ID)
		runtimeReq := s.runtimeRequest(RenderExecutePrompt(sp, planTask, target, selection), map[string]string{"project": sp.Project, "sprint": sp.Slug, "stage": string(StageExecute), "task": task.ID, "model_source": selection.Source})
		runtimeReq.WorkDir = target.Path
		runtimeReq.Policy.PathRules = append(runtimeReq.Policy.PathRules, pruntime.PermissionPathRule{Path: target.Path, Action: "allow"})
		run, runErr := s.runtime.StartRun(ctx, runtimeReq)
		result.Runtime = append(result.Runtime, run)
		finish := s.now().UTC()
		task.Runtime = runtimeSummary(run, selection)
		task.UpdatedAt = finish
		task.CompletedAt = &finish
		switch {
		case ctx.Err() != nil:
			task.Status = ExecuteTaskCancelled
			task.Diagnostics = append(task.Diagnostics, ExecuteDiagnostic{Code: "cancelled", Message: ctx.Err().Error(), At: finish})
		case runErr != nil:
			task.Status = ExecuteTaskFailed
			task.Diagnostics = append(task.Diagnostics, ExecuteDiagnostic{Code: "runtime-failed", Message: safeError(runErr), At: finish})
		case len(run.Artifacts) > 0:
			task.Status = ExecuteTaskComplete
			for _, artifact := range run.Artifacts {
				task.Evidence = append(task.Evidence, ExecuteEvidence{Kind: artifact.Kind, Summary: firstNonEmptyString(artifact.Description, artifact.ID), Path: safeArtifactPath(artifact.URI)})
			}
		case hasDiagnosticOnlyCompletion(run):
			task.Status = ExecuteTaskComplete
			task.Diagnostics = append(task.Diagnostics, ExecuteDiagnostic{Code: "diagnostic-only-completion", Message: "runtime reported safe diagnostic-only completion", At: finish})
		default:
			task.Status = ExecuteTaskFailed
			task.Diagnostics = append(task.Diagnostics, ExecuteDiagnostic{Code: "missing-evidence", Message: "runtime succeeded without expected evidence", At: finish})
		}
		if err := SaveExecuteRunState(s.root, sp, state); err != nil {
			return result, err
		}
		if req.TaskID != "" {
			break
		}
	}
	if err := WriteExecuteSummary(s.root, sp, state); err != nil {
		return result, err
	}
	statePath, _ := ExecuteRunStatePath(s.root, sp)
	result.RunStatePath = workspace.Rel(s.root, statePath)
	result.SummaryPath = ArtifactRelPath(sp, StageExecute)
	result.Tasks = state.Tasks
	result.Message = executeResultMessage(state.Tasks)
	if hasFailedExecuteTask(state.Tasks) {
		return result, fmt.Errorf("execute completed with failed tasks")
	}
	return result, nil
}

func (s Service) prepareExecute(projectRef, sprintRef string, req ExecuteRequest) (Sprint, []ExecutePlanTask, ExecuteTargetRef, ExecuteModelSelection, []ValidationFinding, error) {
	sp, inputs, catalog, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return Sprint{}, nil, ExecuteTargetRef{}, ExecuteModelSelection{}, nil, err
	}
	manifest, findings := s.planManifest(sp, inputs, catalog)
	var target ExecuteTargetRef
	if len(findings) == 0 {
		var targetFindings []ValidationFinding
		target, targetFindings = ResolveExecuteTarget(inputs.ProjectIndex)
		findings = append(findings, targetFindings...)
	}
	var tasks []ExecutePlanTask
	if len(findings) == 0 {
		data, readErr := s.store.ReadArtifact(sp, StagePlan)
		if readErr != nil {
			findings = append(findings, finding("plan.md", "", ArtifactRelPath(sp, StagePlan), "missing plan", readErr.Error(), "Generate and validate plan.md before execute."))
		} else {
			tasks, findings = ExtractExecutePlanTasks(data, manifest)
		}
	}
	if req.TaskID != "" && len(findings) == 0 && taskByID(tasks, req.TaskID).ID == "" {
		findings = append(findings, finding("Tasks", req.TaskID, ArtifactRelPath(sp, StagePlan), "unknown execute task", "selected task id does not exist in plan.md", "Use a task id from validate execute or run without --task."))
	}
	selection := s.executeModelSelection(req.ModelOverride)
	if selection.Model == "" {
		findings = append(findings, finding("Configuration", "execute model", "", "missing execute model", "no execute model configured", "Set planning.execute_model, planning.plan_model, models.primary, or models.default."))
	}
	sortSprintFindings(findings)
	return sp, tasks, target, selection, findings, nil
}

func RenderExecutePrompt(sp Sprint, task ExecutePlanTask, target ExecuteTargetRef, selection ExecuteModelSelection) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Execute Sprint Task\n\nProject: `%s`\nSprint: `%s`\nTask ID: `%s`\nTask: %s\n", sp.Project, sp.Slug, task.ID, task.Name)
	fmt.Fprintf(&b, "\nApproved target: `%s`\nModel source: `%s`\n", target.Path, selection.Source)
	fmt.Fprintln(&b, "\nTraceability:")
	for _, d := range task.Decisions {
		fmt.Fprintf(&b, "- %s\n", d)
	}
	for _, r := range task.Requirements {
		fmt.Fprintf(&b, "- %s\n", r)
	}
	fmt.Fprintln(&b, "\nImplementation steps:")
	for _, step := range task.Steps {
		fmt.Fprintf(&b, "- %s\n", step)
	}
	fmt.Fprintln(&b, "\nExpected evidence:")
	for _, evidence := range task.Evidence {
		fmt.Fprintf(&b, "- %s\n", evidence)
	}
	fmt.Fprintln(&b, "\nSafety constraints:")
	for _, line := range ExecuteSafetyInstructions(target) {
		fmt.Fprintf(&b, "- %s\n", line)
	}
	fmt.Fprintln(&b, "\nComplete only after producing verifiable evidence or an explicit safe diagnostic explaining why evidence cannot be machine-validated.")
	return b.String()
}

func WriteExecuteSummary(root string, sp Sprint, state ExecuteRunState) error {
	path, err := resolveSprintContained(root, sp, ArtifactRelPath(sp, StageExecute))
	if err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Execute Summary\n\nPlan: `%s`\nRun state: `%s`\n\n", state.PlanPath, ExecuteRunStateRelPath(sp))
	counts := map[ExecuteTaskStatus]int{}
	for _, task := range state.Tasks {
		counts[task.Status]++
	}
	fmt.Fprintln(&b, "## Task Counts")
	fmt.Fprintln(&b)
	for _, status := range ExecuteTaskStatuses() {
		fmt.Fprintf(&b, "- %s: %d\n", status, counts[status])
	}
	fmt.Fprintln(&b, "\n## Tasks")
	fmt.Fprintln(&b)
	for _, task := range state.Tasks {
		fmt.Fprintf(&b, "- `%s` %s: %s (attempts: %d)\n", task.ID, task.Status, task.Identity.Name, task.Attempts)
		for _, evidence := range task.Evidence {
			fmt.Fprintf(&b, "  - evidence: %s %s\n", evidence.Kind, evidence.Summary)
		}
		for _, diagnostic := range task.Diagnostics {
			fmt.Fprintf(&b, "  - diagnostic: %s %s\n", diagnostic.Code, diagnostic.Message)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func ArtifactExecuteRelPath(s Sprint) string { return ArtifactRelPath(s, StageExecute) }

func taskByID(tasks []ExecutePlanTask, id string) ExecutePlanTask {
	for _, task := range tasks {
		if task.ID == id {
			return task
		}
	}
	return ExecutePlanTask{}
}

func reconcileExecuteState(existing ExecuteRunState, planned []ExecuteTaskRecord, now time.Time) ExecuteRunState {
	byID := map[string]ExecuteTaskRecord{}
	for _, task := range existing.Tasks {
		if task.Status == ExecuteTaskRunning {
			task.Status = ExecuteTaskFailed
			task.CompletedAt = &now
			task.Diagnostics = append(task.Diagnostics, ExecuteDiagnostic{Code: "stale-running", Message: "recovered stale running task on resume", At: now})
		}
		byID[task.ID] = task
	}
	for i, task := range planned {
		if old, ok := byID[task.ID]; ok {
			planned[i] = old
		}
	}
	existing.Tasks = planned
	existing.UpdatedAt = now
	return existing
}

func runtimeSummary(run pruntime.Result, selection ExecuteModelSelection) *ExecuteRuntimeSummary {
	return &ExecuteRuntimeSummary{RunID: run.RunID, SessionID: run.SessionID, Model: selection.Model, ModelSource: selection.Source, PermissionSummary: run.Permissions.Mode, ValidationSummary: fmt.Sprintf("configured=%t passed=%t failures=%d", run.Validation.Configured, run.Validation.Passed, run.Validation.Failures), OmissionReason: "raw runtime payloads omitted"}
}

func hasDiagnosticOnlyCompletion(run pruntime.Result) bool {
	for _, warning := range run.Warnings {
		if strings.Contains(strings.ToLower(warning), "diagnostic-only") {
			return true
		}
	}
	return false
}

func safeArtifactPath(uri string) string {
	if safeRelPath(uri) {
		return uri
	}
	return ""
}

func ptrTime(t time.Time) *time.Time { return &t }

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "runtime artifact"
}

func mustReadPlan(s Service, sp Sprint) string {
	data, _ := s.store.ReadArtifact(sp, StagePlan)
	return data
}

func executeResultMessage(tasks []ExecuteTaskRecord) string {
	if hasFailedExecuteTask(tasks) {
		return "execute failed"
	}
	return "execute complete"
}

func (s Service) executeModelSelection(override string) ExecuteModelSelection {
	if strings.TrimSpace(override) != "" {
		return ExecuteModelSelection{Model: override, Source: "command override"}
	}
	if rt, ok := s.stageRuntime[StageExecute]; ok && strings.TrimSpace(rt.Model) != "" {
		return ExecuteModelSelection{Model: rt.Model, Source: "planning.execute_model"}
	}
	if rt, ok := s.stageRuntime[StagePlan]; ok && strings.TrimSpace(rt.Model) != "" {
		return ExecuteModelSelection{Model: rt.Model, Source: "planning.plan_model"}
	}
	if model := joinProviderModel(s.runtimeConfig.Provider, s.runtimeConfig.Model); model != "" {
		return ExecuteModelSelection{Model: model, Source: "runtime.config"}
	}
	return ExecuteModelSelection{Model: "provider/model", Source: "default"}
}

func joinProviderModel(provider, model string) string {
	if strings.TrimSpace(model) == "" {
		return ""
	}
	if strings.TrimSpace(provider) == "" {
		return model
	}
	return provider + "/" + model
}

func hasFailedExecuteTask(tasks []ExecuteTaskRecord) bool {
	for _, task := range tasks {
		if task.Status == ExecuteTaskFailed || task.Status == ExecuteTaskCancelled {
			return true
		}
	}
	return false
}

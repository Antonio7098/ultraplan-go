package sprint

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
	"github.com/Antonio7098/ultraplan-go/internal/project"
	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

const StageExecute PlanningStage = "execute"

type ExecuteRequest struct {
	TaskID        string
	DryRun        bool
	Resume        bool
	ModelOverride string
	DeferReason   string
}

func (s Service) DeferExecuteTask(ctx context.Context, projectRef, sprintRef, taskID, reason string) (ExecuteResult, error) {
	reason = strings.TrimSpace(reason)
	if taskID == "" || reason == "" {
		return ExecuteResult{}, fmt.Errorf("task id and deferral reason are required")
	}
	if len(reason) > 1000 || strings.ContainsAny(reason, "\x00\r\n") {
		return ExecuteResult{}, fmt.Errorf("deferral reason must be a single line of at most 1000 characters")
	}
	ctx, release, err := s.acquireMutationContext(ctx, projectRef, sprintRef)
	if err != nil {
		return ExecuteResult{}, err
	}
	defer release()
	_ = ctx
	sp, err := s.resolveMutationSprint(projectRef, sprintRef)
	if err != nil {
		return ExecuteResult{}, err
	}
	state, err := LoadExecuteRunState(s.root, sp)
	if err != nil {
		return ExecuteResult{}, err
	}
	now := s.now().UTC()
	found := false
	for i := range state.Tasks {
		task := &state.Tasks[i]
		if task.ID != taskID {
			continue
		}
		found = true
		if task.Status == ExecuteTaskComplete || task.Status == ExecuteTaskDeferred {
			return ExecuteResult{}, fmt.Errorf("completed task %q cannot be deferred", taskID)
		}
		task.Status = ExecuteTaskDeferred
		task.UpdatedAt = now
		task.CompletedAt = &now
		task.Diagnostics = append(task.Diagnostics, ExecuteDiagnostic{Code: "deferred", Message: safeExecuteText("execute.defer_reason", reason), At: now})
		break
	}
	if !found {
		return ExecuteResult{}, fmt.Errorf("unknown execute task %q", taskID)
	}
	if err := SaveExecuteRunState(s.root, sp, state); err != nil {
		return ExecuteResult{}, err
	}
	if err := WriteExecuteSummary(s.root, sp, state); err != nil {
		return ExecuteResult{}, err
	}
	statePath, _ := ExecuteRunStatePath(s.root, sp)
	return ExecuteResult{Project: sp.Project, Sprint: sp.Slug, RunStatePath: workspace.Rel(s.root, statePath), SummaryPath: ArtifactRelPath(sp, StageExecute), Tasks: state.Tasks, Message: "execute task deferred"}, nil
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
	inputs, err := s.store.ReadPlanningInputs(sp)
	if err != nil {
		return PromptPreview{}, err
	}
	prefix, err := s.prepareSharedPromptContext(context.Background(), sp, inputs, false)
	if err != nil {
		return PromptPreview{}, err
	}
	queue := filterExecuteQueue(tasks, req.TaskID)
	prompt, err := composeStagePromptChecked(prefix, s.renderExecuteSessionPrompt(sp, task, queue, target, executeSelectionForTask(selection, task)))
	if err != nil {
		return PromptPreview{}, err
	}
	explanation := explainComposedPrompt(prompt)
	return PromptPreview{Project: sp.Project, Sprint: sp.Slug, Prompt: prompt, Explanation: &explanation}, nil
}

func (s Service) Execute(ctx context.Context, projectRef, sprintRef string, req ExecuteRequest) (ExecuteResult, error) {
	if !req.DryRun {
		lockedCtx, release, lockErr := s.acquireMutationContext(ctx, projectRef, sprintRef)
		if lockErr != nil {
			return ExecuteResult{}, lockErr
		}
		defer release()
		ctx = lockedCtx
	}
	sp, tasks, target, selection, findings, err := s.prepareExecute(projectRef, sprintRef, req)
	if err != nil {
		return ExecuteResult{}, err
	}
	result := ExecuteResult{Project: sp.Project, Sprint: sp.Slug, DryRun: req.DryRun, Findings: findings}
	if len(findings) > 0 {
		return result, fmt.Errorf("execute prerequisites failed validation")
	}
	inputs, err := s.store.ReadPlanningInputs(sp)
	if err != nil {
		return result, err
	}
	sharedPrefix, err := s.prepareSharedPromptContext(ctx, sp, inputs, true)
	if err != nil {
		return result, err
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
		result.Prompt, err = composeStagePromptChecked(sharedPrefix, s.renderExecuteSessionPrompt(sp, promptTask, filterExecuteQueue(tasks, req.TaskID), target, executeSelectionForTask(selection, promptTask)))
		if err != nil {
			return result, err
		}
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
	executionQueue := executeQueueFromState(state.Tasks, tasks, req.TaskID)
	batchSessionID := ""
	batchSessionModel := selection.Model
	if req.Resume && filepath.Clean(state.Target.Path) == filepath.Clean(target.Path) {
		if len(executionQueue) > 0 {
			if first := taskByID(tasks, executionQueue[0].ID); first.ID != "" {
				batchSessionModel = executeSelectionForTask(selection, first).Model
			}
		}
		batchSessionID = reusableExecuteSession(state.Tasks, req.TaskID, batchSessionModel)
	}
	executionTurn := 0
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
		if task.Status != ExecuteTaskPending && task.Status != ExecuteTaskFailed && task.Status != ExecuteTaskCancelled {
			continue
		}
		start := s.now().UTC()
		task.Status = ExecuteTaskRunning
		task.Attempts++
		task.StartedAt = &start
		task.UpdatedAt = start
		if err := SaveExecuteRunState(s.root, sp, state); err != nil {
			return result, fmt.Errorf("persist running execute task %q: %w", task.ID, err)
		}
		planTask := taskByID(tasks, task.ID)
		taskSelection := executeSelectionForTask(selection, planTask)
		executionTurn++
		continueSession := batchSessionID != "" && batchSessionModel == taskSelection.Model
		stagePrompt := ""
		promptPrefix := sharedPrefix
		if continueSession {
			stagePrompt = RenderExecuteContinuationPrompt(sp, planTask)
			promptPrefix = ""
		} else {
			stagePrompt = s.renderExecuteSessionPrompt(sp, planTask, executionQueue, target, taskSelection)
		}
		composedPrompt, composeErr := composeStagePromptChecked(promptPrefix, stagePrompt)
		if composeErr != nil {
			return result, composeErr
		}
		sessionMode := "initial"
		if continueSession {
			sessionMode = "continue"
		} else if executionTurn > 1 {
			sessionMode = "fresh-fallback"
		}
		runtimeReq := s.runtimeRequest(composedPrompt, map[string]string{
			"project": sp.Project, "sprint": sp.Slug, "stage": string(StageExecute), "task": task.ID, "model_source": taskSelection.Source,
			"execution_session_mode": sessionMode, "execution_turn": fmt.Sprintf("%d", executionTurn), "execution_queue_size": fmt.Sprintf("%d", len(executionQueue)),
		})
		runtimeReq.WorkDir = target.Path
		if continueSession {
			runtimeReq.SessionID = batchSessionID
			runtimeReq.SessionAction = "continue"
		}
		previousOnEvent := runtimeReq.OnEvent
		var sessionMu sync.Mutex
		var sessionSaveErr error
		runtimeReq.OnEvent = func(event pruntime.Event) {
			if previousOnEvent != nil {
				previousOnEvent(event)
			}
			sessionMu.Lock()
			defer sessionMu.Unlock()
			if event.SessionID == "" || (task.Runtime != nil && task.Runtime.SessionID == event.SessionID) {
				return
			}
			task.Runtime = &ExecuteRuntimeSummary{SessionID: event.SessionID, Model: taskSelection.Model, ModelSource: taskSelection.Source, OmissionReason: "raw runtime payloads omitted"}
			task.UpdatedAt = s.now().UTC()
			state.UpdatedAt = task.UpdatedAt
			if err := SaveExecuteRunState(s.root, sp, state); err != nil && sessionSaveErr == nil {
				sessionSaveErr = err
			}
		}
		run, runErr := s.startSprintRuntime(ctx, sp, StageExecute, runtimeReq)
		sessionMu.Lock()
		checkpointErr := sessionSaveErr
		sessionMu.Unlock()
		result.Runtime = append(result.Runtime, run)
		finish := s.now().UTC()
		task.Runtime = mergeRuntimeSummary(task.Runtime, runtimeSummary(run, taskSelection))
		if runErr == nil {
			batchSessionID = run.SessionID
			batchSessionModel = taskSelection.Model
			if batchSessionID == "" && task.Runtime != nil {
				batchSessionID = task.Runtime.SessionID
			}
		}
		task.UpdatedAt = finish
		task.CompletedAt = &finish
		deferReason, deferErr := s.agentDeferredTaskReason(sp, task.ID)
		switch {
		case checkpointErr != nil:
			task.Status = ExecuteTaskFailed
			task.Diagnostics = append(task.Diagnostics, ExecuteDiagnostic{Code: "state-save-failed", Message: safeExecuteText("execute.state_save_error", checkpointErr.Error()), At: finish})
		case deferErr != nil:
			task.Status = ExecuteTaskFailed
			task.Diagnostics = append(task.Diagnostics, ExecuteDiagnostic{Code: "invalid-deferral", Message: safeExecuteText("execute.defer_error", deferErr.Error()), At: finish})
		case deferReason != "":
			task.Status = ExecuteTaskDeferred
			task.Diagnostics = append(task.Diagnostics, ExecuteDiagnostic{Code: "deferred", Message: safeExecuteText("execute.defer_reason", deferReason), At: finish})
		case ctx.Err() != nil:
			task.Status = ExecuteTaskCancelled
			task.Diagnostics = append(task.Diagnostics, ExecuteDiagnostic{Code: "cancelled", Message: safeExecuteText("execute.cancelled", ctx.Err().Error()), At: finish})
		case runErr != nil:
			task.Status = ExecuteTaskFailed
			task.Diagnostics = append(task.Diagnostics, ExecuteDiagnostic{Code: "runtime-failed", Message: safeExecuteText("execute.runtime_error", safeError(runErr)), At: finish})
		case len(run.Artifacts) > 0:
			task.Status = ExecuteTaskComplete
			for _, artifact := range run.Artifacts {
				task.Evidence = append(task.Evidence, ExecuteEvidence{Kind: artifact.Kind, Summary: safeExecuteText("execute.evidence", firstNonEmptyString(artifact.Description, artifact.ID)), Path: safeArtifactPath(artifact.URI)})
			}
		case hasDiagnosticOnlyCompletion(run):
			task.Status = ExecuteTaskComplete
			task.Diagnostics = append(task.Diagnostics, ExecuteDiagnostic{Code: "diagnostic-only-completion", Message: "runtime reported safe diagnostic-only completion", At: finish})
		default:
			task.Status = ExecuteTaskFailed
			task.Diagnostics = append(task.Diagnostics, ExecuteDiagnostic{Code: "missing-evidence", Message: "runtime succeeded without expected evidence", At: finish})
		}
		if err := SaveExecuteRunState(s.root, sp, state); err != nil {
			return result, fmt.Errorf("persist terminal execute task %q: %w", task.ID, err)
		}
		if checkpointErr != nil {
			return result, fmt.Errorf("persist runtime session for execute task %q: %w", task.ID, checkpointErr)
		}
		if task.Status == ExecuteTaskFailed || task.Status == ExecuteTaskCancelled {
			break
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
	for _, run := range result.Runtime {
		_ = s.deleteCompletedSessions(ctx, run)
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
		target, targetFindings = s.resolveSprintTarget(sp, inputs.ProjectIndex, false)
		findings = append(findings, targetFindings...)
	}
	var tasks []ExecutePlanTask
	if len(findings) == 0 {
		data, readErr := s.store.ReadArtifact(sp, StagePlan)
		if readErr != nil {
			findings = append(findings, finding("plan.md", "", ArtifactRelPath(sp, StagePlan), "missing plan", readErr.Error(), "Generate and validate plan.md before execute."))
		} else {
			tasks, findings = extractExecutePlanTasks(data, manifest, req.Resume)
			if req.Resume && len(findings) == 0 {
				findings = append(findings, s.validateResolvedResumeTasks(sp, tasks, manifest.OutputPath)...)
			}
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

func (s Service) validateResolvedResumeTasks(sp Sprint, tasks []ExecutePlanTask, planPath string) []ValidationFinding {
	var resolved []ExecutePlanTask
	for _, task := range tasks {
		if task.Checked || task.Deferred {
			resolved = append(resolved, task)
		}
	}
	if len(resolved) == 0 {
		return nil
	}
	state, err := LoadExecuteRunState(s.root, sp)
	if err != nil {
		return []ValidationFinding{finding("Tasks", "", planPath, "checked tasks lack execution state", "one or more top-level tasks are checked but no valid resumable execution state exists", "Restore the matching .run-state.json or leave tasks unchecked for a new execution.")}
	}
	byID := make(map[string]ExecuteTaskStatus, len(state.Tasks))
	for _, task := range state.Tasks {
		byID[task.ID] = task.Status
	}
	var findings []ValidationFinding
	for _, task := range resolved {
		expected := ExecuteTaskComplete
		problem := "checked task is not complete in execution state"
		detail := "top-level task is checked without a matching complete run-state record"
		suggestion := "Restore the task checkbox to unchecked or reconcile the execution state before resuming."
		if task.Deferred {
			expected = ExecuteTaskDeferred
			problem = "plan deferral is not recorded in execution state"
			detail = "top-level task uses [/] without a matching deferred run-state record"
			suggestion = "Resume the owning execute attempt so it can record the deferral, or restore the task marker to unchecked."
		}
		if byID[task.ID] != expected {
			findings = append(findings, finding("Tasks", task.Name, planPath, problem, detail, suggestion))
		}
	}
	return findings
}

func (s Service) agentDeferredTaskReason(sp Sprint, taskID string) (string, error) {
	data, err := s.store.ReadArtifact(sp, StagePlan)
	if err != nil {
		return "", err
	}
	inputs, err := s.store.ReadPlanningInputs(sp)
	if err != nil {
		return "", err
	}
	catalog, parseFindings := project.ParseProjectIndex(inputs.ProjectIndex)
	if len(parseFindings) > 0 {
		return "", fmt.Errorf("project-index.md has malformed catalog rows")
	}
	manifest, findings := s.planManifest(sp, inputs, catalog)
	if len(findings) > 0 {
		return "", fmt.Errorf("plan manifest is invalid")
	}
	tasks, findings := extractExecutePlanTasks(data, manifest, true)
	if len(findings) > 0 {
		return "", fmt.Errorf("plan changed to an invalid task state")
	}
	for _, task := range tasks {
		if task.ID == taskID && task.Deferred {
			return task.DeferReason, nil
		}
	}
	return "", nil
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
	fmt.Fprintln(&b, "If remaining work is explicitly accepted for later follow-up, change this task's top-level plan marker to `[/]` and append `— Deferred: <concrete reason>` on the same line. Do not use `[/]` without a reason and do not mark deferred work complete.")
	return b.String()
}

func (s Service) renderExecuteSessionPrompt(sp Sprint, task ExecutePlanTask, queue []ExecutePlanTask, target ExecuteTargetRef, selection ExecuteModelSelection) string {
	prompt := RenderExecutePrompt(sp, task, target, selection) + renderExecuteQueue(task.ID, queue)
	inputs := directProjectDefinitionInputsFromWorkspace(s.root, sp)
	inputs = append(inputs,
		directSprintArtifactInput(s.root, sp, StageSprintIndex),
		directSprintArtifactInput(s.root, sp, StageTechnicalHandbook),
	)
	inputs = append(inputs, directReasoningDirectoryInputs(s.root, sp)...)
	inputs = append(inputs,
		directSprintArtifactInput(s.root, sp, StageReasoning),
		directSprintArtifactInput(s.root, sp, StagePlan),
	)
	return appendDirectInputPacket(prompt, inputs, sharedPromptSuffixReserve)
}

func RenderExecuteContinuationPrompt(sp Sprint, task ExecutePlanTask) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Continue Sprint Execution\n\nProject: `%s`\nSprint: `%s`\nTask ID: `%s`\nTask: %s\n", sp.Project, sp.Slug, task.ID, task.Name)
	fmt.Fprintln(&b, "\nContinue in the same execution session. The original approved target, safety constraints, shared sprint context, handbook examples, and ordered queue remain in force. Re-read the current repository state, execute only this next task, and retain context for the following checkpoint.")
	fmt.Fprintln(&b, "\nTraceability:")
	for _, decision := range task.Decisions {
		fmt.Fprintf(&b, "- %s\n", decision)
	}
	for _, requirement := range task.Requirements {
		fmt.Fprintf(&b, "- %s\n", requirement)
	}
	fmt.Fprintln(&b, "\nImplementation steps:")
	for _, step := range task.Steps {
		fmt.Fprintf(&b, "- %s\n", step)
	}
	fmt.Fprintln(&b, "\nExpected evidence:")
	for _, evidence := range task.Evidence {
		fmt.Fprintf(&b, "- %s\n", evidence)
	}
	fmt.Fprintln(&b, "\nComplete only after producing verifiable evidence or an explicit safe diagnostic explaining why evidence cannot be machine-validated.")
	fmt.Fprintln(&b, "If remaining work is explicitly accepted for later follow-up, change this task's top-level plan marker to `[/]` and append `— Deferred: <concrete reason>` on the same line.")
	return b.String()
}

func renderExecuteQueue(currentTaskID string, queue []ExecutePlanTask) string {
	if len(queue) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## Ordered Sprint Execution Queue\n\n")
	b.WriteString("This agent session owns the complete ordered queue below. Execute only the current task during this turn; UltraPlan will checkpoint its evidence before advancing the same session.\n\n")
	for index, task := range queue {
		marker := "queued"
		if task.ID == currentTaskID {
			marker = "current"
		}
		fmt.Fprintf(&b, "%d. [%s] `%s` — %s\n", index+1, marker, task.ID, task.Name)
	}
	return b.String()
}

func filterExecuteQueue(tasks []ExecutePlanTask, taskID string) []ExecutePlanTask {
	if taskID == "" {
		return append([]ExecutePlanTask(nil), tasks...)
	}
	selected := taskByID(tasks, taskID)
	if selected.ID == "" {
		return nil
	}
	return []ExecutePlanTask{selected}
}

func executeQueueFromState(records []ExecuteTaskRecord, tasks []ExecutePlanTask, taskID string) []ExecutePlanTask {
	queue := make([]ExecutePlanTask, 0, len(records))
	for _, record := range records {
		if taskID != "" && record.ID != taskID {
			continue
		}
		if record.Status == ExecuteTaskComplete || record.Status == ExecuteTaskDeferred {
			continue
		}
		if task := taskByID(tasks, record.ID); task.ID != "" {
			queue = append(queue, task)
		}
	}
	return queue
}

func reusableExecuteSession(records []ExecuteTaskRecord, taskID, model string) string {
	sessionID := ""
	for _, record := range records {
		if taskID != "" && record.ID != taskID {
			continue
		}
		if record.Runtime != nil && record.Runtime.Model == model && record.Runtime.SessionID != "" {
			sessionID = record.Runtime.SessionID
		}
	}
	return sessionID
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
	return &ExecuteRuntimeSummary{RunID: run.RunID, SessionID: run.SessionID, Model: selection.Model, ModelSource: selection.Source, PermissionSummary: run.Permissions.Mode, ValidationSummary: fmt.Sprintf("configured=%t passed=%t failures=%d", run.Validation.Configured, run.Validation.Passed, run.Validation.Failures), UsageSummary: formatRuntimeUsage(run.Usage), OmissionReason: "raw runtime payloads omitted"}
}

func formatRuntimeUsage(usage pruntime.Usage) string {
	var parts []string
	appendKnown := func(name string, known bool, value int64) {
		if known {
			parts = append(parts, fmt.Sprintf("%s=%d", name, value))
		}
	}
	appendKnown("input", usage.InputTokensKnown, usage.InputTokens)
	appendKnown("output", usage.OutputTokensKnown, usage.OutputTokens)
	appendKnown("reasoning", usage.ReasoningTokensKnown, usage.ReasoningTokens)
	appendKnown("cache_read", usage.CacheReadTokensKnown, usage.CacheReadTokens)
	appendKnown("cache_write", usage.CacheWriteTokensKnown, usage.CacheWriteTokens)
	appendKnown("total", usage.TotalTokensKnown, usage.TotalTokens)
	appendKnown("turns", usage.TurnsKnown, usage.Turns)
	return strings.Join(parts, " ")
}

func mergeRuntimeSummary(previous, current *ExecuteRuntimeSummary) *ExecuteRuntimeSummary {
	if current == nil {
		return previous
	}
	if current.SessionID == "" && previous != nil {
		current.SessionID = previous.SessionID
	}
	return current
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
	return ExecuteModelSelection{Model: "unresolved", Source: "unresolved"}
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

// executeSelectionForTask resolves one task's effective model selection. An
// inline plan annotation wins over the batch selection; defaults stay
// unchanged when no annotation is present.
func executeSelectionForTask(selection ExecuteModelSelection, task ExecutePlanTask) ExecuteModelSelection {
	if model := strings.TrimSpace(task.Model); model != "" {
		return ExecuteModelSelection{Model: model, Source: "plan.md task annotation"}
	}
	return selection
}

func safeExecuteText(key, value string) string {
	return config.RedactValue(key, safeError(fmt.Errorf("%s", value)))
}

func hasFailedExecuteTask(tasks []ExecuteTaskRecord) bool {
	for _, task := range tasks {
		if task.Status == ExecuteTaskFailed || task.Status == ExecuteTaskCancelled {
			return true
		}
	}
	return false
}

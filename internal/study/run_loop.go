package study

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	runtimepkg "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

// RunLoop resumes or creates durable study execution for the selected study.
//
// The workflow is intentionally resumable rather than exactly-once: safety comes
// from one per-study mutator lock, atomic transition persistence, completed
// artifact revalidation before trust, and attempt/history preservation across
// process restarts. If a process exits mid-task, the next run reconciles active
// states before scheduling more work.
func (s Service) RunLoop(ctx context.Context, req RunLoopRequest) (out RunLoopResult, err error) {
	if req.Parallelism < 1 {
		return RunLoopResult{}, fmt.Errorf("parallelism must be at least 1")
	}
	listing, err := s.ListStudy(req.StudyRef)
	if err != nil {
		return RunLoopResult{}, err
	}
	lock, err := AcquireRunLoopLock(listing.Study, req.Command, req.ForceUnlock, time.Now().UTC())
	if err != nil {
		return RunLoopResult{}, err
	}
	defer func() {
		if releaseErr := lock.Release(); err == nil && releaseErr != nil {
			err = releaseErr
		}
	}()

	scopeDimensions, err := resolveDimensions(listing.Dimensions, req.DimensionRefs)
	if err != nil {
		return RunLoopResult{}, err
	}
	scopeSources, err := resolveSources(listing.Sources, req.SourceRefs)
	if err != nil {
		return RunLoopResult{}, err
	}

	state, err := loadOrCreateRunLoopState(req, listing.Study, listing.Sources, listing.Dimensions, s.workspaceRoot)
	if err != nil {
		return RunLoopResult{}, err
	}
	ReconcileRunState(&state, s.workspaceRoot, listing.Study, listing.Sources, listing.Dimensions, time.Now().UTC())
	ResumeValidateRunState(&state, listing.Study, listing.Sources, listing.Dimensions, time.Now().UTC())
	if err := SaveRunState(listing.Study, state); err != nil {
		return RunLoopResult{}, err
	}
	if err := SyncRunHistory(listing.Study, state); err != nil {
		return RunLoopResult{}, err
	}

	result := RunLoopResult{
		Study:       listing.Study,
		Parallelism: req.Parallelism,
		StatePath:   RunStatePath(listing.Study),
		LockPath:    RunLoopLockPath(listing.Study),
	}
	taskIndex := map[string]int{}
	for i, task := range state.Tasks {
		taskIndex[task.ID] = i
	}
	scope := runLoopScope(listing.Study, listing.Sources, scopeSources, scopeDimensions, len(req.SourceRefs) > 0)

	var mu sync.Mutex
	emit := func(event RunLoopProgressEvent, task TaskState) {
		if req.Progress == nil {
			return
		}
		req.Progress(RunLoopProgress{Event: event, Task: task, Counts: SummarizeRunState(state, result.StatePath), ScopeCounts: SummarizeRunState(filterRunState(state, scope), result.StatePath)})
	}
	emitTask := func(event RunLoopProgressEvent, id string) {
		if req.Progress == nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		idx, ok := taskIndex[id]
		if !ok {
			return
		}
		req.Progress(RunLoopProgress{Event: event, Task: state.Tasks[idx], Counts: SummarizeRunState(state, result.StatePath), ScopeCounts: SummarizeRunState(filterRunState(state, scope), result.StatePath)})
	}
	emitRuntime := func(id string, event runtimeEvent) {
		if req.Progress == nil || !interestingRuntimeEvent(event.Kind) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		idx, ok := taskIndex[id]
		if !ok {
			return
		}
		req.Progress(RunLoopProgress{Event: RunLoopProgressRuntime, Task: state.Tasks[idx], Counts: SummarizeRunState(state, result.StatePath), ScopeCounts: SummarizeRunState(filterRunState(state, scope), result.StatePath), RuntimeEvent: &event})
	}
	save := func() error {
		mu.Lock()
		stateCopy := cloneRunState(state)
		mu.Unlock()
		return SaveRunState(listing.Study, stateCopy)
	}
	update := func(id string, fn func(*TaskState)) error {
		mu.Lock()
		idx, ok := taskIndex[id]
		if !ok {
			mu.Unlock()
			return fmt.Errorf("task %q not found", id)
		}
		fn(&state.Tasks[idx])
		state.UpdatedAt = time.Now().UTC()
		stateCopy := cloneRunState(state)
		mu.Unlock()
		return SaveRunState(listing.Study, stateCopy)
	}
	taskSnapshot := func(id string) (TaskState, error) {
		mu.Lock()
		defer mu.Unlock()
		idx, ok := taskIndex[id]
		if !ok {
			return TaskState{}, fmt.Errorf("task %q not found", id)
		}
		return state.Tasks[idx], nil
	}
	var historyMu sync.Mutex
	recordHistory := func(id string) error {
		mu.Lock()
		idx, ok := taskIndex[id]
		if !ok {
			mu.Unlock()
			return fmt.Errorf("task %q not found", id)
		}
		stateCopy := cloneRunState(state)
		task := state.Tasks[idx]
		mu.Unlock()
		historyMu.Lock()
		defer historyMu.Unlock()
		if err := AppendRunHistory(listing.Study, stateCopy, task); err != nil {
			return err
		}
		return WriteRunHistorySummary(listing.Study, stateCopy)
	}
	for _, task := range state.Tasks {
		if task.Status == TaskStatusRunning || task.Status == TaskStatusValidating || task.Status == TaskStatusRetrying {
			emit(RunLoopProgressStarted, task)
		}
	}

	var firstErr error
	var errMu sync.Mutex
	recordErr := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		defer errMu.Unlock()
		if firstErr == nil {
			firstErr = err
		}
	}
	attempted := map[string]bool{}
	runTask := func(id string) {
		if ctx.Err() != nil {
			recordErr(markTaskCancelled(update, id, ctx.Err()))
			recordErr(recordHistory(id))
			emitTask(RunLoopProgressCancelled, id)
			return
		}
		task, err := taskSnapshot(id)
		if err != nil {
			recordErr(err)
			return
		}
		recordErr(update(id, func(t *TaskState) {
			now := time.Now().UTC()
			t.Status = TaskStatusRunning
			t.Attempts++
			t.StartedAt = &now
			t.UpdatedAt = now
			t.LastError = nil
			t.RetryAfter = nil
		}))
		emitTask(RunLoopProgressStarted, id)
		var res ExecutionResult
		switch task.Kind {
		case TaskKindAnalysis:
			res, err = s.RunAnalysis(ctx, ExecutionRequest{StudyRef: listing.Study.Name, DimensionRef: task.DimensionRef, SourceRef: task.Source, OnEvent: func(event runtimeEvent) {
				emitRuntime(id, event)
			}})
			if err != nil {
				res = ExecutionResult{Status: ExecutionStatusRuntimeFailed, TaskKind: TaskKindAnalysis, Study: listing.Study, OutputPath: task.OutputPath, RuntimeError: safeError(err), RuntimeErr: err}
			}
		case TaskKindSynthesis:
			res, err = s.Synthesize(ctx, SynthesisRequest{StudyRef: listing.Study.Name, DimensionRef: task.DimensionRef, SourceRefs: selectedSourceNames(listing.Sources), OnEvent: func(event runtimeEvent) {
				emitRuntime(id, event)
			}})
			if err != nil {
				res = ExecutionResult{Status: ExecutionStatusRuntimeFailed, TaskKind: TaskKindSynthesis, Study: listing.Study, OutputPath: task.OutputPath, RuntimeError: safeError(err), RuntimeErr: err}
			}
		default:
			recordErr(fmt.Errorf("unsupported task kind %q", task.Kind))
			return
		}
		recordErr(applyExecutionResult(update, id, res))
		recordErr(recordHistory(id))
		emitTask(progressEventForExecution(res), id)
	}
	for ctx.Err() == nil {
		mu.Lock()
		now := time.Now().UTC()
		ids := runnableTaskIDs(state, scope, attempted, req.Parallelism, now)
		nextRetry := nextRetryAfter(state, scope, now)
		mu.Unlock()
		if len(ids) == 0 {
			if nextRetry == nil {
				break
			}
			waitUntilRetry(ctx, *nextRetry, func() {
				emitRetryWait(state, scope, result.StatePath, req.Progress)
			})
			continue
		}
		for _, id := range ids {
			attempted[id] = true
		}
		var wg sync.WaitGroup
		for _, id := range ids {
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				runTask(id)
			}(id)
		}
		wg.Wait()
		if firstErr != nil {
			return result, firstErr
		}
	}

	state.Complete = allTasksComplete(state)
	if err := save(); err != nil {
		return result, err
	}
	if err := SyncRunHistory(listing.Study, state); err != nil {
		return result, err
	}
	result.State = state
	result.Counts = runLoopCounts(state)
	result.ScopeCounts = runLoopCounts(filterRunState(state, scope))
	result.Status = runLoopStatus(filterRunState(state, scope), result.ScopeCounts, ctx.Err() != nil)
	return result, nil
}

func waitUntilRetry(ctx context.Context, retryAt time.Time, emit func()) {
	for {
		if emit != nil {
			emit()
		}
		wait := time.Until(retryAt)
		if wait <= 0 {
			return
		}
		if wait > time.Minute {
			wait = time.Minute
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func emitRetryWait(state RunState, scope map[string]bool, statePath string, progress func(RunLoopProgress)) {
	if progress == nil {
		return
	}
	for _, task := range state.Tasks {
		if !scope[task.ID] {
			continue
		}
		if task.Status == TaskStatusRetrying && task.RetryAfter != nil && task.RetryAfter.After(time.Now().UTC()) {
			progress(RunLoopProgress{Event: RunLoopProgressWaiting, Task: task, Counts: SummarizeRunState(state, statePath), ScopeCounts: SummarizeRunState(filterRunState(state, scope), statePath)})
			return
		}
	}
}

func loadOrCreateRunLoopState(req RunLoopRequest, study Study, sources []Source, dimensions []Dimension, workspaceRoot string) (RunState, error) {
	if !req.Reset {
		state, err := LoadRunState(study)
		if err == nil {
			return state, nil
		}
		if !errors.Is(err, ErrRunStateMissing) {
			return RunState{}, err
		}
	}
	if err := archiveRunStateIfExists(study); err != nil {
		return RunState{}, err
	}
	return NewRunState(NewRunStateRequest{
		WorkspaceRoot: workspaceRoot,
		Study:         study,
		Sources:       sources,
		Dimensions:    dimensions,
		Filters:       RunFilters{},
		Config:        req.Config,
	})
}

func archiveRunStateIfExists(study Study) error {
	path := RunStatePath(study)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	archiveDir := filepath.Join(study.Path, RunStateDirName, "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return err
	}
	name := fmt.Sprintf("run-state-%s.json", time.Now().UTC().Format("20060102T150405Z"))
	return os.Rename(path, filepath.Join(archiveDir, name))
}

func progressEventForExecution(res ExecutionResult) RunLoopProgressEvent {
	switch res.Status {
	case ExecutionStatusCompleted, ExecutionStatusSkipped:
		return RunLoopProgressCompleted
	case ExecutionStatusCancelled:
		return RunLoopProgressCancelled
	default:
		if executionShouldRetry(res) {
			return RunLoopProgressWaiting
		}
		return RunLoopProgressFailed
	}
}

type runtimeEvent = runtimepkg.Event

func interestingRuntimeEvent(kind string) bool {
	switch kind {
	case "rate_limit", "retry", "fallback", "lifecycle", "warning", "fatal_error":
		return true
	default:
		return false
	}
}

func runnableTaskIDs(state RunState, scope map[string]bool, attempted map[string]bool, limit int, now time.Time) []string {
	if limit < 1 {
		limit = 1
	}
	var ids []string
	byID := map[string]TaskState{}
	for _, task := range state.Tasks {
		byID[task.ID] = task
	}
	for _, task := range state.Tasks {
		if len(ids) >= limit {
			return ids
		}
		if !scope[task.ID] {
			continue
		}
		if taskAttemptBlocked(task, attempted) || task.Kind != TaskKindSynthesis || !taskRunnable(task, now) {
			continue
		}
		if dependenciesCompleteFrom(byID, task) {
			ids = append(ids, task.ID)
		}
	}
	for _, task := range state.Tasks {
		if len(ids) >= limit {
			return ids
		}
		if !scope[task.ID] {
			continue
		}
		if taskAttemptBlocked(task, attempted) || task.Kind != TaskKindAnalysis || !taskRunnable(task, now) {
			continue
		}
		ids = append(ids, task.ID)
	}
	return ids
}

func runLoopScope(study Study, allSources []Source, scopeSources []Source, dimensions []Dimension, sourceFiltered bool) map[string]bool {
	scope := map[string]bool{}
	for _, dimension := range dimensions {
		applicable := GetApplicableSources(scopeSources, dimension)
		if len(applicable) == 0 {
			continue
		}
		for _, source := range applicable {
			scope[analysisTaskID(study, dimension, source)] = true
		}
		allApplicable := GetApplicableSources(allSources, dimension)
		if !sourceFiltered || len(applicable) == len(allApplicable) {
			scope[synthesisTaskID(study, dimension)] = true
		}
	}
	return scope
}

func filterRunState(state RunState, scope map[string]bool) RunState {
	if len(scope) == 0 {
		out := state
		out.Tasks = nil
		out.Complete = true
		return out
	}
	out := state
	out.Tasks = make([]TaskState, 0, len(state.Tasks))
	for _, task := range state.Tasks {
		if scope[task.ID] {
			out.Tasks = append(out.Tasks, task)
		}
	}
	out.Complete = allTasksComplete(out)
	return out
}

func taskAttemptBlocked(task TaskState, attempted map[string]bool) bool {
	return attempted[task.ID] && task.Status != TaskStatusRetrying
}

func taskRunnable(task TaskState, now time.Time) bool {
	switch task.Status {
	case TaskStatusPending, TaskStatusFailed, TaskStatusCancelled, TaskStatusWaiting:
		if task.RetryAfter != nil && task.RetryAfter.After(now) {
			return false
		}
		return true
	case TaskStatusRetrying:
		return task.RetryAfter == nil || !task.RetryAfter.After(now)
	default:
		return false
	}
}

func nextRetryAfter(state RunState, scope map[string]bool, now time.Time) *time.Time {
	var next *time.Time
	for _, task := range state.Tasks {
		if !scope[task.ID] {
			continue
		}
		if task.Status != TaskStatusRetrying || task.RetryAfter == nil || !task.RetryAfter.After(now) {
			continue
		}
		retry := *task.RetryAfter
		if next == nil || retry.Before(*next) {
			next = &retry
		}
	}
	return next
}

func dependenciesComplete(state RunState, task TaskState) bool {
	byID := map[string]TaskState{}
	for _, item := range state.Tasks {
		byID[item.ID] = item
	}
	return dependenciesCompleteFrom(byID, task)
}

func dependenciesCompleteFrom(byID map[string]TaskState, task TaskState) bool {
	for _, dep := range task.Dependencies {
		if byID[dep.TaskID].Status != TaskStatusCompleted {
			return false
		}
	}
	return true
}

func readySynthesisTaskIDs(state RunState, attempted map[string]bool, now time.Time) []string {
	byID := map[string]TaskState{}
	for _, task := range state.Tasks {
		byID[task.ID] = task
	}
	var ids []string
	for _, task := range state.Tasks {
		if attempted[task.ID] || task.Kind != TaskKindSynthesis || !taskRunnable(task, now) {
			continue
		}
		if dependenciesCompleteFrom(byID, task) {
			ids = append(ids, task.ID)
		}
	}
	return ids
}

func applyExecutionResult(update func(string, func(*TaskState)) error, id string, res ExecutionResult) error {
	return update(id, func(t *TaskState) {
		now := time.Now().UTC()
		t.UpdatedAt = now
		t.CompletedAt = &now
		t.Agent = res.Agent
		if t.Agent.RunID == "" {
			t.Agent.RunID = res.RuntimeRunID
		}
		if t.Agent.Status == "" {
			t.Agent.Status = res.RuntimeStatus
		}
		if retryAfter := retryAfterFromAgent(t.Agent); retryAfter != nil {
			t.RetryAfter = retryAfter
		}
		if res.Validation.Path != "" {
			summary := validationSummary(res.Validation, now)
			t.Validation = &summary
		}
		switch res.Status {
		case ExecutionStatusCompleted, ExecutionStatusSkipped:
			t.Status = TaskStatusCompleted
			t.RetryAfter = nil
		case ExecutionStatusCancelled:
			t.Status = TaskStatusCancelled
			t.LastError = &TaskError{Code: "runtime.cancelled", Message: safeExecutionMessage(res)}
		case ExecutionStatusValidationFailed, ExecutionStatusPreflightBlocked:
			t.Status = TaskStatusFailed
			t.LastError = &TaskError{Code: "validation.failed", Message: safeExecutionMessage(res), Path: res.OutputPath}
		default:
			if executionShouldRetry(res) {
				t.Status = TaskStatusRetrying
				if t.RetryAfter == nil {
					retry := now.Add(defaultRuntimeRetryDelay(res))
					t.RetryAfter = &retry
				}
			} else {
				t.Status = TaskStatusFailed
			}
			t.LastError = &TaskError{Code: "runtime.failed", Message: safeExecutionMessage(res)}
		}
	})
}

func executionShouldRetry(res ExecutionResult) bool {
	switch res.RuntimeCategory {
	case "rate_limit", "timeout", "provider_unavailable", "runtime_unavailable":
		return true
	default:
		return false
	}
}

func defaultRuntimeRetryDelay(res ExecutionResult) time.Duration {
	if res.RuntimeCategory == "rate_limit" {
		return 10 * time.Minute
	}
	return 2 * time.Minute
}

func markTaskCancelled(update func(string, func(*TaskState)) error, id string, err error) error {
	return update(id, func(t *TaskState) {
		now := time.Now().UTC()
		t.Status = TaskStatusCancelled
		t.UpdatedAt = now
		t.CompletedAt = &now
		t.LastError = &TaskError{Code: "workflow.cancelled", Message: err.Error()}
	})
}

func safeExecutionMessage(res ExecutionResult) string {
	if res.RuntimeError != "" {
		return res.RuntimeError
	}
	if len(res.Blockers) > 0 {
		return "blocked by invalid or missing reports"
	}
	if res.SkippedReason != "" {
		return res.SkippedReason
	}
	return string(res.Status)
}

func allTasksComplete(state RunState) bool {
	for _, task := range state.Tasks {
		if task.Status != TaskStatusCompleted && task.Status != TaskStatusSkipped {
			return false
		}
	}
	return true
}

func runLoopCounts(state RunState) RunAllCounts {
	var counts RunAllCounts
	for _, task := range state.Tasks {
		switch task.Status {
		case TaskStatusCompleted:
			counts.Completed++
		case TaskStatusSkipped, TaskStatusWaiting:
			counts.Skipped++
		case TaskStatusPending, TaskStatusRetrying, TaskStatusRunning, TaskStatusValidating:
			counts.Pending++
		default:
			counts.Failed++
		}
	}
	return counts
}

func runLoopStatus(state RunState, counts RunAllCounts, cancelled bool) RunAllStatus {
	if cancelled || hasCancelledTask(state) {
		return RunAllStatusCancelled
	}
	if counts.Failed == 0 && counts.Pending == 0 && counts.Skipped == 0 {
		return RunAllStatusCompleted
	}
	if counts.Completed > 0 {
		return RunAllStatusPartial
	}
	return RunAllStatusValidationFailed
}

func hasCancelledTask(state RunState) bool {
	for _, task := range state.Tasks {
		if task.Status == TaskStatusCancelled {
			return true
		}
	}
	return false
}

func cloneRunState(state RunState) RunState {
	out := state
	out.Filters.Dimensions = append([]string(nil), state.Filters.Dimensions...)
	out.Filters.Sources = append([]string(nil), state.Filters.Sources...)
	out.Tasks = append([]TaskState(nil), state.Tasks...)
	for i := range out.Tasks {
		out.Tasks[i].Dependencies = append([]SynthesisDependency(nil), state.Tasks[i].Dependencies...)
	}
	return out
}

func LockInfoForStatus(study Study) (*LockInfo, error) {
	info, err := ReadRunLoopLock(study)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return &info, nil
}

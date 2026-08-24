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

	diagnostics := newRunLoopDiagnostics(listing.Study, "")
	diagnostics.sample("state.load.start", "", 0, nil)
	loadStarted := time.Now()
	state, err := loadOrCreateRunLoopState(req, listing.Study, listing.Sources, listing.Dimensions, s.workspaceRoot)
	diagnostics.sample("state.load.end", "", time.Since(loadStarted), err)
	if err != nil {
		return RunLoopResult{}, err
	}
	diagnostics.runID = state.RunID
	diagnostics.configureParallelism(req.Parallelism)
	startupCleanup := runtimepkg.CleanupRuntimeStores(listing.Study.Path, 72*time.Hour, 2*1024*1024*1024, false)
	diagnostics.storage("runtime_store.recovered", startupCleanup, readDiskPressure(listing.Study.Path))
	stopDiagnostics := diagnostics.start(ctx)
	defer stopDiagnostics()
	history, err := LoadRunHistory(listing.Study)
	if err != nil {
		return RunLoopResult{}, err
	}
	now := time.Now().UTC()
	ReconcileRunState(&state, s.workspaceRoot, listing.Study, listing.Sources, listing.Dimensions, now)
	ResumeValidateRunState(&state, listing.Study, listing.Sources, listing.Dimensions, now)
	if !req.Reset {
		RestoreCompletedRunHistory(&state, listing.Study, listing.Sources, listing.Dimensions, history, now)
	}
	initialSaveStarted := time.Now()
	err = SaveRunState(listing.Study, state)
	diagnostics.sample("state.save.end", "", time.Since(initialSaveStarted), err)
	if err != nil {
		return RunLoopResult{}, err
	}
	if err := SyncRunHistory(listing.Study, state); err != nil {
		return RunLoopResult{}, err
	}
	historyKeys, err := readRunHistoryKeys(RunHistoryPath(listing.Study))
	if err != nil {
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
	effectiveParallelism := req.Parallelism
	var memoryAvailable uint64
	var schedulingMessage string
	progressFor := func(event RunLoopProgressEvent, task TaskState, runtimeEvent *runtimeEvent) RunLoopProgress {
		return RunLoopProgress{
			Event:                event,
			Task:                 task,
			Counts:               SummarizeRunStateCounts(state, result.StatePath),
			ScopeCounts:          summarizeRunStateCountsForScope(state, scope, result.StatePath),
			RuntimeEvent:         runtimeEvent,
			RequestedParallelism: req.Parallelism,
			EffectiveParallelism: effectiveParallelism,
			MemoryAvailableBytes: memoryAvailable,
			Message:              schedulingMessage,
		}
	}
	emit := func(event RunLoopProgressEvent, task TaskState) {
		if req.Progress == nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		req.Progress(progressFor(event, task, nil))
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
		req.Progress(progressFor(event, state.Tasks[idx], nil))
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
		req.Progress(progressFor(RunLoopProgressRuntime, state.Tasks[idx], &event))
	}
	var saveMu sync.Mutex
	var stateVersion uint64
	var persistedVersion uint64
	var lastPersisted time.Time
	persist := func(taskID string, force bool) error {
		mu.Lock()
		requestedVersion := stateVersion
		mu.Unlock()
		saveMu.Lock()
		defer saveMu.Unlock()
		if !force && persistedVersion >= requestedVersion {
			return nil
		}
		if !force {
			if wait := 250*time.Millisecond - time.Since(lastPersisted); wait > 0 {
				timer := time.NewTimer(wait)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
			}
		}
		mu.Lock()
		stateCopy := cloneRunState(state)
		savedVersion := stateVersion
		mu.Unlock()
		started := time.Now()
		err := SaveRunState(listing.Study, stateCopy)
		diagnostics.sample("state.save.end", taskID, time.Since(started), err)
		if err == nil {
			persistedVersion = savedVersion
			lastPersisted = time.Now()
		}
		return err
	}
	save := func() error { return persist("", true) }
	update := func(id string, fn func(*TaskState)) error {
		mu.Lock()
		idx, ok := taskIndex[id]
		if !ok {
			mu.Unlock()
			return fmt.Errorf("task %q not found", id)
		}
		fn(&state.Tasks[idx])
		state.UpdatedAt = time.Now().UTC()
		stateVersion++
		mu.Unlock()
		return persist(id, false)
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
		return appendRunHistoryWithKeys(listing.Study, stateCopy, task, historyKeys)
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
	loadFirstErr := func() error {
		errMu.Lock()
		defer errMu.Unlock()
		return firstErr
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
		mu.Lock()
		dependencyState := cloneRunState(state)
		mu.Unlock()
		if !dependenciesComplete(dependencyState, task) {
			if dependenciesTerminal(dependencyState, task) {
				recordErr(markSynthesisDependenciesFailed(update, id))
				recordErr(recordHistory(id))
				emitTask(RunLoopProgressFailed, id)
				return
			}
			recordErr(update(id, func(t *TaskState) {
				t.Status = TaskStatusWaiting
				t.UpdatedAt = time.Now().UTC()
			}))
			emitTask(RunLoopProgressWaiting, id)
			return
		}
		recordErr(update(id, func(t *TaskState) {
			now := time.Now().UTC()
			t.Status = TaskStatusRunning
			t.Attempts++
			t.StartedAt = &now
			t.CompletedAt = nil
			t.UpdatedAt = now
			t.LastError = nil
			t.RetryAfter = nil
			t.Agent = AgentMetadata{}
		}))
		diagnostics.sample("runtime.start", id, 0, nil)
		runtimeStarted := time.Now()
		var res ExecutionResult
		checkpointSession := func(session TaskSession) {
			recordErr(update(id, func(t *TaskState) {
				copy := session
				t.Session = &copy
				t.UpdatedAt = time.Now().UTC()
			}))
		}
		switch task.Kind {
		case TaskKindAnalysis:
			res, err = s.RunAnalysis(ctx, ExecutionRequest{StudyRef: listing.Study.Name, DimensionRef: task.DimensionRef, SourceRef: task.Source, Model: req.Model, ResumeSession: task.Session, OnSession: checkpointSession, OnEvent: func(event runtimeEvent) {
				emitRuntime(id, event)
			}})
			if err != nil {
				res = ExecutionResult{Status: ExecutionStatusRuntimeFailed, TaskKind: TaskKindAnalysis, Study: listing.Study, OutputPath: task.OutputPath, RuntimeError: safeError(err), RuntimeErr: err}
			}
		case TaskKindSynthesis:
			res, err = s.Synthesize(ctx, SynthesisRequest{StudyRef: listing.Study.Name, DimensionRef: task.DimensionRef, SourceRefs: selectedSourceNames(listing.Sources), Model: req.Model, ResumeSession: task.Session, OnSession: checkpointSession, OnEvent: func(event runtimeEvent) {
				emitRuntime(id, event)
			}})
			if err != nil {
				res = ExecutionResult{Status: ExecutionStatusRuntimeFailed, TaskKind: TaskKindSynthesis, Study: listing.Study, OutputPath: task.OutputPath, RuntimeError: safeError(err), RuntimeErr: err}
			}
		default:
			recordErr(fmt.Errorf("unsupported task kind %q", task.Kind))
			return
		}
		diagnostics.sample("runtime.end", id, time.Since(runtimeStarted), err)
		recordErr(applyExecutionResult(update, id, res))
		recordErr(recordHistory(id))
		emitTask(progressEventForExecution(res), id)
	}
	done := make(chan struct{}, req.Parallelism)
	active := 0
	stopScheduling := false
	for active > 0 || (!stopScheduling && ctx.Err() == nil) {
		if loadFirstErr() != nil {
			stopScheduling = true
		}
		pressure := readMemoryPressure()
		mu.Lock()
		previousParallelism := effectiveParallelism
		if pressure.Stretched && effectiveParallelism > 1 {
			effectiveParallelism--
		} else if pressure.Recovered && effectiveParallelism < req.Parallelism {
			effectiveParallelism++
		}
		memoryAvailable = pressure.AvailableBytes
		if effectiveParallelism != previousParallelism {
			if effectiveParallelism < previousParallelism {
				schedulingMessage = fmt.Sprintf("memory pressure reduced parallelism from %d to %d", previousParallelism, effectiveParallelism)
				diagnostics.scheduling("parallelism.throttled", req.Parallelism, effectiveParallelism, pressure.AvailableBytes)
			} else {
				schedulingMessage = fmt.Sprintf("memory recovered; restored parallelism from %d to %d", previousParallelism, effectiveParallelism)
				diagnostics.scheduling("parallelism.restored", req.Parallelism, effectiveParallelism, pressure.AvailableBytes)
			}
		}
		parallelismChanged := effectiveParallelism != previousParallelism
		changeEvent := RunLoopProgressRestored
		if effectiveParallelism < previousParallelism {
			changeEvent = RunLoopProgressThrottled
		}
		mu.Unlock()
		if parallelismChanged {
			emit(changeEvent, TaskState{})
		}
		disk := readDiskPressure(listing.Study.Path)
		if disk.Pressured {
			cleanup := runtimepkg.CleanupRuntimeStores(listing.Study.Path, 72*time.Hour, 2*1024*1024*1024, disk.Critical)
			diagnostics.storage("disk.admission_paused", cleanup, disk)
			disk = readDiskPressure(listing.Study.Path)
			if disk.Pressured {
				mu.Lock()
				schedulingMessage = fmt.Sprintf("disk pressure paused new workers; %.1f%% used, %d MiB available", disk.UsedPercent, disk.AvailableBytes/(1024*1024))
				mu.Unlock()
				if active > 0 {
					<-done
					active--
					continue
				}
				timer := time.NewTimer(30 * time.Second)
				select {
				case <-ctx.Done():
					timer.Stop()
				case <-timer.C:
				}
				continue
			}
		}
		available := effectiveParallelism - active
		var ids []string
		var nextRetry *time.Time
		mu.Lock()
		now := time.Now().UTC()
		if !stopScheduling && available > 0 {
			ids = runnableTaskIDs(state, scope, attempted, available, now, listing.DimensionOrder)
			nextRetry = nextRetryAfter(state, scope, now)
		}
		mu.Unlock()
		for _, id := range ids {
			attempted[id] = true
			active++
			// Emit Started at claim time so progress ordering matches the
			// scheduler's deterministic tier order rather than racing worker
			// goroutines.
			emitTask(RunLoopProgressStarted, id)
			go func(id string) {
				runTask(id)
				done <- struct{}{}
			}(id)
		}
		if active > 0 {
			// Each task owns its OpenCode database, so a completed slot can be
			// refilled without waiting for sibling cleanup.
			<-done
			active--
			continue
		}
		if stopScheduling || (len(ids) == 0 && nextRetry == nil) {
			break
		}
		waitUntilRetry(ctx, *nextRetry, func() {
			mu.Lock()
			defer mu.Unlock()
			emitRetryWait(state, scope, result.StatePath, req.Progress)
		})
	}
	if err := loadFirstErr(); err != nil {
		return result, err
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
			progress(RunLoopProgress{Event: RunLoopProgressWaiting, Task: task, Counts: SummarizeRunStateCounts(state, statePath), ScopeCounts: summarizeRunStateCountsForScope(state, scope, statePath)})
			return
		}
	}
}

func summarizeRunStateCountsForScope(state RunState, scope map[string]bool, statePath string) StatusSummary {
	if len(scope) == 0 {
		return StatusSummary{Complete: true, StatePath: statePath, RunID: state.RunID}
	}
	summary := StatusSummary{Complete: true, StatePath: statePath, RunID: state.RunID}
	for _, task := range state.Tasks {
		if !scope[task.ID] {
			continue
		}
		summary.Total++
		countTaskStatus(&summary, task)
		if task.Status != TaskStatusCompleted && task.Status != TaskStatusSkipped {
			summary.Complete = false
		}
	}
	return summary
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
	case "", "message", "session", "native_extension":
		return false
	default:
		return true
	}
}

func runnableTaskIDs(state RunState, scope map[string]bool, attempted map[string]bool, limit int, now time.Time, dimensionOrder []Dimension) []string {
	if limit < 1 {
		limit = 1
	}
	var ids []string
	byID := map[string]TaskState{}
	for _, task := range state.Tasks {
		byID[task.ID] = task
	}
	ranks := dimensionPriorityRanks(dimensionOrder)
	remainingRank := len(dimensionOrder)
	// Dimension order remains a priority, but not a barrier: lower-priority
	// dimensions backfill any workers the earlier tiers cannot occupy.
	for rank := 0; rank <= remainingRank; rank++ {
		for _, kind := range []TaskKind{TaskKindSynthesis, TaskKindAnalysis} {
			for _, task := range state.Tasks {
				if len(ids) >= limit {
					return ids
				}
				if !scope[task.ID] || taskAttemptBlocked(task, attempted) || task.Kind != kind || !taskRunnable(task, now) {
					continue
				}
				if dimensionTaskRank(task, ranks, remainingRank) != rank {
					continue
				}
				if kind == TaskKindSynthesis && !dependenciesCompleteFrom(byID, task) && !dependenciesTerminalFrom(byID, task) {
					continue
				}
				ids = append(ids, task.ID)
			}
		}
	}
	return ids
}

func dimensionPriorityRanks(order []Dimension) map[string]int {
	ranks := make(map[string]int, len(order))
	for i, dimension := range order {
		ranks[dimension.Ref()] = i
	}
	return ranks
}

func dimensionTaskRank(task TaskState, ranks map[string]int, remainingRank int) int {
	if rank, ok := ranks[task.DimensionRef]; ok {
		return rank
	}
	return remainingRank
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

func dependenciesTerminal(state RunState, task TaskState) bool {
	byID := map[string]TaskState{}
	for _, item := range state.Tasks {
		byID[item.ID] = item
	}
	return dependenciesTerminalFrom(byID, task)
}

func dependenciesTerminalFrom(byID map[string]TaskState, task TaskState) bool {
	for _, dep := range task.Dependencies {
		depTask, ok := byID[dep.TaskID]
		if !ok {
			return true
		}
		switch depTask.Status {
		case TaskStatusCompleted, TaskStatusFailed, TaskStatusCancelled, TaskStatusSkipped:
			continue
		default:
			return false
		}
	}
	return true
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
		if t.Agent.Cleanup.Attempted && !t.Agent.Cleanup.Completed {
			t.Session = nil
		}
		switch res.Status {
		case ExecutionStatusCompleted, ExecutionStatusSkipped:
			t.Status = TaskStatusCompleted
			t.RetryAfter = nil
			t.Session = nil
		case ExecutionStatusCancelled:
			t.Status = TaskStatusCancelled
			t.LastError = executionTaskError("runtime.cancelled", res)
		case ExecutionStatusValidationFailed, ExecutionStatusPreflightBlocked:
			t.Status = TaskStatusFailed
			t.LastError = executionTaskError("validation.failed", res)
			t.LastError.Path = res.OutputPath
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
			t.LastError = executionTaskError("runtime.failed", res)
		}
	})
}

func executionTaskError(code string, res ExecutionResult) *TaskError {
	detail := compactDiagnostic(res.RuntimeDetail)
	if detail == "" {
		for _, attempt := range res.Agent.Attempts {
			if attempt.ErrorDetail != "" {
				detail = compactDiagnostic(attempt.ErrorDetail)
				break
			}
		}
	}
	return &TaskError{Code: code, Message: safeExecutionMessage(res), Detail: detail}
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

func markSynthesisDependenciesFailed(update func(string, func(*TaskState)) error, id string) error {
	return update(id, func(t *TaskState) {
		now := time.Now().UTC()
		t.Status = TaskStatusFailed
		t.UpdatedAt = now
		t.CompletedAt = &now
		t.LastError = &TaskError{Code: "synthesis.dependencies_failed", Message: "synthesis dependencies failed or were cancelled"}
	})
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
		base := compactDiagnostic(res.RuntimeError)
		// Enrich with provider/model and attempt timeline when available
		// so fallback from primary (e.g. ox-alpha) to backup is immediately
		// visible instead of showing a generic "runtime.failed".
		if res.Agent.Provider != "" || res.Agent.Model != "" || len(res.Agent.Attempts) > 0 {
			detail := base
			if res.RuntimeCategory != "" && res.RuntimeCategory != "runtime_exit" {
				detail = res.RuntimeCategory + ": " + base
			}
			if len(res.Agent.Attempts) > 0 {
				timeline := formatAttemptTimeline(res.Agent.Attempts, res.Agent.Policy.Decisions)
				if timeline != "" {
					detail += " [" + timeline + "]"
				}
			} else if res.Agent.Provider != "" {
				detail += " [" + res.Agent.Provider + "/" + res.Agent.Model + "]"
			}
			return compactDiagnostic(detail)
		}
		return base
	}
	if len(res.Blockers) > 0 {
		return "blocked by invalid or missing reports"
	}
	if res.SkippedReason != "" {
		return compactDiagnostic(res.SkippedReason)
	}
	return string(res.Status)
}

func formatAttemptTimeline(attempts []AttemptMetadata, decisions []PolicyDecisionMetadata) string {
	if len(attempts) == 0 {
		return ""
	}
	var parts []string
	for _, a := range attempts {
		label := a.Provider + "/" + a.Model
		if label == "/" {
			label = a.Status
		}
		if a.ErrorCategory != "" && a.ErrorCategory != "none" {
			label += ":" + a.ErrorCategory
		}
		if a.ErrorDetail != "" {
			label += " (" + compactDiagnostic(a.ErrorDetail) + ")"
		}
		parts = append(parts, label)
	}
	// annotate fallbacks from policy decisions
	for _, d := range decisions {
		if d.Kind == "fallback" && d.Detail != "" {
			parts = append(parts, "fallback:"+d.Detail)
			break
		}
	}
	joined := ""
	for i, p := range parts {
		if i == 0 {
			joined = p
		} else {
			joined += " → " + p
		}
	}
	return joined
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
		if state.Tasks[i].LastError != nil {
			lastError := *state.Tasks[i].LastError
			out.Tasks[i].LastError = &lastError
		}
		if state.Tasks[i].Validation != nil {
			validation := *state.Tasks[i].Validation
			out.Tasks[i].Validation = &validation
		}
		out.Tasks[i].Agent = cloneAgentMetadata(state.Tasks[i].Agent)
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

package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
	runtimepkg "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
	"github.com/Antonio7098/ultraplan-go/internal/runcontrol"
)

const runControlLease = runcontrol.OwnerLeaseDuration

// runControlState owns one repository handle per workspace for the lifetime of
// the process. dependencies is copied throughout command dispatch, so keeping
// this state behind a pointer prevents accidental duplicate connection pools.
type runControlState struct {
	mu       sync.Mutex
	repos    map[string]*runcontrol.SQLiteRepository
	loggers  map[string]*runcontrol.LocalFileLogger
	policies map[string]runcontrol.RetentionPolicy
	owner    runcontrol.Owner
	initErr  error
}

func newRunControlState() *runControlState {
	owner, err := currentRunOwner()
	return &runControlState{repos: make(map[string]*runcontrol.SQLiteRepository), loggers: make(map[string]*runcontrol.LocalFileLogger), policies: make(map[string]runcontrol.RetentionPolicy), owner: owner, initErr: err}
}

func (s *runControlState) repository(ctx context.Context, workspaceRoot string, policies ...runcontrol.RetentionPolicy) (*runcontrol.SQLiteRepository, error) {
	if s == nil {
		return nil, errors.New("run-control process state is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.initErr != nil {
		return nil, s.initErr
	}
	if repository := s.repos[workspaceRoot]; repository != nil {
		if len(policies) > 0 && s.policies[workspaceRoot] != policies[0] {
			return nil, errors.New("run-control retention policy changed during the process lifetime")
		}
		return repository, nil
	}
	var retention runcontrol.RetentionPolicy
	if len(policies) > 0 {
		retention = policies[0]
	}
	repository, err := runcontrol.OpenSQLite(ctx, workspaceRoot, runcontrol.SQLiteOptions{Retention: retention})
	if err != nil {
		return nil, err
	}
	logger, err := runcontrol.OpenLocalFileLogger(workspaceRoot)
	if err != nil {
		_ = repository.Close()
		return nil, fmt.Errorf("open run-control diagnostic log: %w", err)
	}
	repository.SetLogger(logger)
	if _, err := repository.Reconcile(ctx, runcontrol.NativeProcessProbe{}, runcontrol.ReconcileOptions{}); err != nil {
		_ = repository.Close()
		_ = logger.Close()
		return nil, fmt.Errorf("startup run reconciliation failed: %w", err)
	}
	s.repos[workspaceRoot] = repository
	s.loggers[workspaceRoot] = logger
	s.policies[workspaceRoot] = retention
	return repository, nil
}

func (s *runControlState) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for root, repository := range s.repos {
		_ = repository.Close()
		if logger := s.loggers[root]; logger != nil {
			_ = logger.Close()
			delete(s.loggers, root)
		}
		delete(s.repos, root)
	}
}

func currentRunOwner() (runcontrol.Owner, error) {
	return runcontrol.NewProcessOwner()
}

type controlledRuntime struct {
	base interface {
		StartRun(context.Context, runtimepkg.Request) (runtimepkg.Result, error)
	}
	repository runcontrol.Repository
	owner      runcontrol.Owner
}

func controlledRuntimeFor(deps dependencies, workspaceRoot string, effectiveConfig config.Config, base interface {
	StartRun(context.Context, runtimepkg.Request) (runtimepkg.Result, error)
}) (controlledRuntime, error) {
	if deps.runControl == nil {
		return controlledRuntime{}, errors.New("run-control process state is unavailable")
	}
	repository, err := deps.runControl.repository(deps.ctx, workspaceRoot, runControlRetentionPolicy(effectiveConfig))
	if err != nil {
		return controlledRuntime{}, fmt.Errorf("open durable run control: %w", err)
	}
	return controlledRuntime{base: base, repository: repository, owner: deps.runControl.owner}, nil
}

func runControlRetentionPolicy(c config.Config) runcontrol.RetentionPolicy {
	full, _ := time.ParseDuration(c.RunControl.FullHistory)
	tombstone, _ := time.ParseDuration(c.RunControl.TombstoneHistory)
	return runcontrol.RetentionPolicy{FullHistory: full, TombstoneHistory: tombstone, HardQuotaBytes: c.RunControl.WorkspaceQuota}
}

func (r controlledRuntime) StartRun(ctx context.Context, req runtimepkg.Request) (runtimepkg.Result, error) {
	target := targetFromRuntimeRequest(req)
	correlation := runcontrol.Correlation{ProductTaskID: boundedSafe(req.TraceID)}
	snapshot, err := r.repository.Accept(ctx, runcontrol.Acceptance{
		Target:        target,
		Correlation:   correlation,
		ProductStatus: "accepted",
	})
	if err != nil {
		return runtimepkg.Result{}, fmt.Errorf("durable run acceptance failed: %w", err)
	}
	attempt, _, err := r.repository.Claim(ctx, runcontrol.Claim{
		RunID: snapshot.RunID, Owner: r.owner, Lease: runControlLease, Correlation: correlation,
	})
	if err != nil {
		return runtimepkg.Result{}, fmt.Errorf("durable run ownership failed: %w", err)
	}
	fence := runcontrol.Fence{
		RunID: snapshot.RunID, AttemptID: attempt.ID, OwnerID: r.owner.ID,
		FencingGeneration: attempt.FencingGeneration,
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	req.Metadata = cloneRuntimeMetadata(req.Metadata)
	req.Metadata["run_control_run_id"] = string(snapshot.RunID)
	configuredOnEvent := req.OnEvent
	var eventMu sync.Mutex
	var persistenceErr error
	var progressKey string
	var progressCommittedAt time.Time
	var progressOmitted uint64
	var progressOmittedFirst time.Time
	var progressOmittedLast time.Time
	setPersistenceErr := func(value error) {
		eventMu.Lock()
		defer eventMu.Unlock()
		if persistenceErr == nil {
			persistenceErr = value
			cancel()
		}
	}
	req.OnEvent = func(event runtimepkg.Event) {
		eventMu.Lock()
		defer eventMu.Unlock()
		if persistenceErr != nil {
			return
		}
		draft := runtimeEventDraft(req, event)
		eventAt := event.Time.UTC()
		if eventAt.IsZero() {
			eventAt = time.Now().UTC()
		}
		key := string(draft.Type) + "\x00" + draft.Stage + "\x00" + draft.Task + "\x00" + draft.Payload["type"] + "\x00" + draft.Payload["kind"]
		elapsed := eventAt.Sub(progressCommittedAt)
		if draft.Type == runcontrol.EventProgress && key == progressKey && elapsed >= 0 && elapsed < runcontrol.ProgressCoalesceWindow {
			if progressOmitted == 0 {
				progressOmittedFirst = eventAt
			}
			progressOmitted++
			progressOmittedLast = eventAt
			return
		}
		if progressOmitted > 0 {
			if draft.Omission == nil {
				draft.Omission = &runcontrol.Omission{Reason: "equivalent progress coalesced"}
			}
			draft.Omission.Count += progressOmitted
			draft.Omission.FirstAt = &progressOmittedFirst
			draft.Omission.LastAt = &progressOmittedLast
			progressOmitted = 0
		}
		if _, _, appendErr := appendRunEventWithRetry(runCtx, r.repository, fence, draft); appendErr != nil {
			persistenceErr = fmt.Errorf("persist runtime event: %w", appendErr)
			cancel()
			return
		}
		if draft.Type == runcontrol.EventProgress {
			progressKey = key
			progressCommittedAt = eventAt
		}
		if configuredOnEvent != nil {
			configuredOnEvent(event)
		}
	}

	controlDone := make(chan struct{})
	go func() {
		defer close(controlDone)
		ticker := time.NewTicker(runcontrol.OwnerTickInterval)
		defer ticker.Stop()
		lastHeartbeat := time.Now()
		lastReconcile := time.Now()
		for {
			select {
			case <-runCtx.Done():
				return
			case now := <-ticker.C:
				snapshot, err := r.repository.Snapshot(runCtx, fence.RunID)
				if err != nil {
					if runCtx.Err() != nil {
						return
					}
					setPersistenceErr(fmt.Errorf("poll durable run control: %w", err))
					return
				}
				if snapshot.Cancellation.State == runcontrol.CancellationRequested {
					if _, _, err := r.repository.AcknowledgeCancellation(runCtx, fence); err != nil {
						setPersistenceErr(fmt.Errorf("acknowledge durable cancellation: %w", err))
						return
					}
					cancel()
					return
				}
				if now.Sub(lastHeartbeat) >= runcontrol.HeartbeatInterval {
					if _, err := r.repository.Heartbeat(runCtx, fence, runControlLease); err != nil {
						if runCtx.Err() != nil {
							return
						}
						setPersistenceErr(fmt.Errorf("persist owner heartbeat: %w", err))
						return
					}
					lastHeartbeat = now
				}
				if now.Sub(lastReconcile) >= runcontrol.ReconciliationInterval {
					if _, err := r.repository.Reconcile(runCtx, runcontrol.NativeProcessProbe{}, runcontrol.ReconcileOptions{}); err != nil {
						if runCtx.Err() != nil {
							return
						}
						setPersistenceErr(fmt.Errorf("reconcile durable runs: %w", err))
						return
					}
					lastReconcile = now
				}
			}
		}
	}()
	result, runErr := r.base.StartRun(runCtx, req)
	eventMu.Lock()
	if persistenceErr == nil && progressOmitted > 0 {
		_, _, appendErr := appendRunEventWithRetry(runCtx, r.repository, fence, runcontrol.EventDraft{
			Type: runcontrol.EventOmission,
			Omission: &runcontrol.Omission{
				Reason: "equivalent progress coalesced", Count: progressOmitted,
				FirstAt: &progressOmittedFirst, LastAt: &progressOmittedLast,
			},
		})
		if appendErr != nil {
			persistenceErr = fmt.Errorf("persist progress omission: %w", appendErr)
		}
	}
	eventMu.Unlock()
	cancel()
	<-controlDone
	eventMu.Lock()
	persistErr := persistenceErr
	eventMu.Unlock()
	if persistErr != nil {
		terminalCtx, terminalCancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, _, terminalErr := proposeRunTerminalWithRetry(terminalCtx, r.repository, fence, runcontrol.TerminalProposal{
			Outcome: runcontrol.TerminalPersistenceLost, Reason: "durable event persistence failed", ProposedBy: r.owner.ID,
		})
		terminalCancel()
		if terminalErr != nil {
			return result, errors.Join(persistErr, terminalErr)
		}
		return result, persistErr
	}

	outcome, reason := terminalOutcome(result, runErr, ctx)
	terminalCtx, terminalCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer terminalCancel()
	if _, _, err := proposeRunTerminalWithRetry(terminalCtx, r.repository, fence, runcontrol.TerminalProposal{
		Outcome: outcome, Reason: reason, ProposedBy: r.owner.ID,
	}); err != nil {
		return result, errors.Join(runErr, fmt.Errorf("persist terminal run outcome: %w", err))
	}
	return result, runErr
}

func appendRunEventWithRetry(ctx context.Context, repository runcontrol.Repository, fence runcontrol.Fence, draft runcontrol.EventDraft) (runcontrol.Event, runcontrol.Snapshot, error) {
	deadline := time.Now().Add(5 * time.Second)
	for {
		event, snapshot, err := repository.Append(ctx, fence, draft)
		if err == nil || !retryableRunControlError(err) || !time.Now().Before(deadline) {
			return event, snapshot, err
		}
		if err := waitRunControlRetry(ctx, deadline); err != nil {
			return runcontrol.Event{}, runcontrol.Snapshot{}, err
		}
	}
}

func proposeRunTerminalWithRetry(ctx context.Context, repository runcontrol.Repository, fence runcontrol.Fence, proposal runcontrol.TerminalProposal) (runcontrol.Snapshot, bool, error) {
	for {
		snapshot, won, err := repository.ProposeTerminal(ctx, fence, proposal)
		if err == nil || !retryableRunControlError(err) {
			return snapshot, won, err
		}
		if err := waitRunControlRetry(ctx, time.Now().Add(250*time.Millisecond)); err != nil {
			return runcontrol.Snapshot{}, false, err
		}
	}
}

func retryableRunControlError(err error) bool {
	return errors.Is(err, runcontrol.ErrUnavailable) || errors.Is(err, runcontrol.ErrBusy)
}

func waitRunControlRetry(ctx context.Context, deadline time.Time) error {
	wait := time.Until(deadline)
	if wait > 100*time.Millisecond {
		wait = 100 * time.Millisecond
	}
	if wait <= 0 {
		return context.DeadlineExceeded
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func targetFromRuntimeRequest(req runtimepkg.Request) runcontrol.Target {
	target := runcontrol.Target{
		Kind:      boundedSafe(req.PromptRef.OwnerKind),
		Operation: boundedSafe(req.PromptRef.Purpose),
		Project:   boundedSafe(req.Metadata["project"]),
		Sprint:    boundedSafe(req.Metadata["sprint"]),
		Study:     boundedSafe(req.Metadata["study"]),
		Stage:     boundedSafe(req.Metadata["stage"]),
		Task:      boundedSafe(firstSafeValue(req.Metadata["task"], req.Metadata["task.kind"], req.Metadata["coverage"])),
	}
	if target.Kind == "" {
		if target.Study != "" {
			target.Kind = "study"
		} else if target.Sprint != "" {
			target.Kind = "sprint"
		} else {
			target.Kind = "runtime"
		}
	}
	if target.Operation == "" {
		target.Operation = boundedSafe(firstSafeValue(target.Stage, req.Metadata["task.kind"], req.PromptRef.ID, "runtime"))
	}
	return target
}

func runtimeEventDraft(req runtimepkg.Request, event runtimepkg.Event) runcontrol.EventDraft {
	eventType := runcontrol.EventProgress
	switch strings.ToLower(strings.TrimSpace(event.Type)) {
	case "warning", "warn", "error":
		eventType = runcontrol.EventWarning
	case "artifact", "file":
		eventType = runcontrol.EventArtifact
	case "finding":
		eventType = runcontrol.EventFinding
	case "message", "text", "output":
		eventType = runcontrol.EventMessage
	}
	payload := map[string]string{
		"runtime_event_id": boundedSafe(event.ID),
		"runtime_run_id":   boundedSafe(event.RunID),
		"session_id":       boundedSafe(event.SessionID),
		"type":             boundedSafe(event.Type),
		"kind":             boundedSafe(event.Kind),
	}
	omission := (*runcontrol.Omission)(nil)
	if event.RawPresent || event.RawOmitted || len(event.Payload) > 0 {
		reason := firstSafeValue(event.RawOmissionReason, "runtime payload omitted by safe persistence policy")
		omission = &runcontrol.Omission{Reason: boundedSafe(reason), Count: 1}
	}
	return runcontrol.EventDraft{
		Type: eventType, Stage: boundedSafe(req.Metadata["stage"]), Task: boundedSafe(firstSafeValue(req.Metadata["task"], req.Metadata["task.kind"])),
		Payload: payload, Omission: omission,
	}
}

func terminalOutcome(result runtimepkg.Result, runErr error, parent context.Context) (runcontrol.TerminalOutcome, string) {
	status := strings.ToLower(strings.TrimSpace(result.Status))
	switch {
	case errors.Is(runErr, context.DeadlineExceeded), errors.Is(parent.Err(), context.DeadlineExceeded), result.Error != nil && result.Error.Category == "timeout":
		return runcontrol.TerminalTimedOut, "runtime deadline exceeded"
	case errors.Is(runErr, context.Canceled), errors.Is(parent.Err(), context.Canceled), status == "cancelled", result.Error != nil && result.Error.Category == "cancellation":
		return runcontrol.TerminalCancelled, "runtime cancelled"
	case runErr != nil, status == "failed", status == "error":
		return runcontrol.TerminalFailed, "runtime failed"
	default:
		return runcontrol.TerminalSucceeded, "runtime completed"
	}
}

func cloneRuntimeMetadata(input map[string]string) map[string]string {
	out := make(map[string]string, len(input)+1)
	for key, value := range input {
		out[key] = value
	}
	return out
}

func boundedSafe(value string) string {
	value = strings.Map(func(r rune) rune {
		if r == 0 || r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	if len(value) > runcontrol.MaxTargetFieldBytes {
		value = value[:runcontrol.MaxTargetFieldBytes]
	}
	return value
}

func firstSafeValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

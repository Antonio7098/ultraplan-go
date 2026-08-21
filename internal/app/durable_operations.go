package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/runcontrol"
)

type durableOperationManager struct {
	repository runcontrol.Repository
	owner      runcontrol.Owner
	mu         sync.Mutex
	owned      map[string]*ownedDurableOperation
}

type ownedDurableOperation struct {
	fence        runcontrol.Fence
	cancel       context.CancelFunc
	stop         chan struct{}
	done         chan struct{}
	eventMu      sync.Mutex
	progressKey  string
	progressAt   time.Time
	omitted      uint64
	omittedFirst time.Time
	omittedLast  time.Time
}

// durableCLICommand is the synchronous CLI counterpart to the web/TUI
// acceptance boundary. It keeps the manager alive for the whole command and
// gives product code the cancellation-aware context owned by the durable run.
type durableCLICommand struct {
	manager  *durableOperationManager
	accepted AcceptedOperation
}

func beginDurableCLICommand(deps dependencies, request OperationRequest) (*durableCLICommand, error) {
	repository, _, err := runRepository(deps)
	if err != nil {
		return nil, err
	}
	manager := newDurableOperationManager(repository, deps.runControl.owner)
	accepted, err := manager.AcceptOperation(deps.ctx, Confirmation{Request: request}, "")
	if err != nil {
		return nil, classifiedCause(ExitRuntime, err, "run-control.accept")
	}
	return &durableCLICommand{manager: manager, accepted: accepted}, nil
}

func (c *durableCLICommand) Context() context.Context { return c.accepted.Context }

func (c *durableCLICommand) Finish(runErr error) error {
	state := OperationComplete
	if c.accepted.Context.Err() != nil {
		state = OperationCancelled
	} else if runErr != nil {
		state = OperationFailed
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := c.manager.FinishOperation(ctx, c.accepted.RunID, state, runErr); err != nil {
		return classifiedCause(ExitRuntime, err, "run-control.finish")
	}
	return nil
}

func finishDurableCLICommand(command *durableCLICommand, runErr error) error {
	if command == nil {
		return runErr
	}
	return errors.Join(runErr, command.Finish(runErr))
}

func newDurableOperationManager(repository runcontrol.Repository, owner runcontrol.Owner) *durableOperationManager {
	return &durableOperationManager{repository: repository, owner: owner, owned: make(map[string]*ownedDurableOperation)}
}

func (m *durableOperationManager) AcceptOperation(ctx context.Context, confirmation Confirmation, digest string) (AcceptedOperation, error) {
	req := confirmation.Request
	target := runcontrol.Target{
		Kind: "operation", Operation: string(req.Kind), Project: req.Project, Sprint: req.Sprint,
		Study: req.Study, Stage: req.Stage, Task: req.Task,
	}
	snapshot, err := m.repository.Accept(ctx, runcontrol.Acceptance{
		Target: target, ProductStatus: "prepared", OperationAlias: digest, ConfirmationDigest: digest,
	})
	if err != nil && digest != "" && errors.Is(err, runcontrol.ErrConflict) {
		existing, resolveErr := m.repository.ResolveOperationAlias(ctx, digest)
		if resolveErr != nil {
			return AcceptedOperation{}, errors.Join(err, resolveErr)
		}
		return AcceptedOperation{RunID: string(existing.RunID), Context: ctx, Existing: true, Lifecycle: string(existing.Lifecycle)}, nil
	}
	if err != nil {
		return AcceptedOperation{}, fmt.Errorf("persist confirmed operation acceptance: %w", err)
	}
	attempt, _, err := m.repository.Claim(ctx, runcontrol.Claim{RunID: snapshot.RunID, Owner: m.owner, Lease: runcontrol.OwnerLeaseDuration})
	if err != nil {
		return AcceptedOperation{}, fmt.Errorf("persist confirmed operation owner claim: %w", err)
	}
	fence := runcontrol.Fence{RunID: snapshot.RunID, AttemptID: attempt.ID, OwnerID: m.owner.ID, FencingGeneration: attempt.FencingGeneration}
	if _, _, err := appendRunEventWithRetry(ctx, m.repository, fence, runcontrol.EventDraft{
		Type: runcontrol.EventLifecycle, Lifecycle: runcontrol.LifecycleRunning,
		Payload: map[string]string{"lifecycle": string(runcontrol.LifecycleRunning)},
	}); err != nil {
		terminalCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, _, _ = proposeRunTerminalWithRetry(terminalCtx, m.repository, fence, runcontrol.TerminalProposal{Outcome: runcontrol.TerminalPersistenceLost, Reason: "operation start persistence failed", ProposedBy: m.owner.ID})
		cancel()
		return AcceptedOperation{}, fmt.Errorf("persist confirmed operation start: %w", err)
	}
	operationCtx, cancel := context.WithCancel(ctx)
	owned := &ownedDurableOperation{fence: fence, cancel: cancel, stop: make(chan struct{}), done: make(chan struct{})}
	m.mu.Lock()
	m.owned[string(snapshot.RunID)] = owned
	m.mu.Unlock()
	go m.controlOperation(operationCtx, owned)
	return AcceptedOperation{RunID: string(snapshot.RunID), Context: operationCtx, Lifecycle: string(runcontrol.LifecycleRunning)}, nil
}

func (m *durableOperationManager) RecordOperationEvent(ctx context.Context, runID string, event OperationEvent) (bool, error) {
	m.mu.Lock()
	owned := m.owned[runID]
	m.mu.Unlock()
	if owned == nil {
		return false, runcontrol.ErrNotFound
	}
	owned.eventMu.Lock()
	defer owned.eventMu.Unlock()
	eventType := runcontrol.EventProgress
	if event.State == OperationFailed {
		eventType = runcontrol.EventWarning
	}
	key := string(eventType) + "\x00" + event.Stage + "\x00" + event.Task + "\x00" + event.EventType + "\x00" + event.EventKind + "\x00" + event.Tool + "\x00" + event.Reason + "\x00" + event.Detail
	now := time.Now().UTC()
	elapsed := now.Sub(owned.progressAt)
	if eventType == runcontrol.EventProgress && key == owned.progressKey && elapsed >= 0 && elapsed < runcontrol.ProgressCoalesceWindow {
		if owned.omitted == 0 {
			owned.omittedFirst = now
		}
		owned.omitted++
		owned.omittedLast = now
		return false, nil
	}
	payload := map[string]string{
		"state": string(event.State), "type": event.EventType, "kind": event.EventKind,
		"tool": event.Tool, "action": event.Action, "reason": event.Reason, "detail": event.Detail,
		"count": fmt.Sprintf("%d/%d", event.Completed, event.Total),
	}
	var omission *runcontrol.Omission
	if event.Message != "" {
		omission = &runcontrol.Omission{Reason: "presentation message omitted", Count: 1}
	}
	if owned.omitted > 0 {
		if omission == nil {
			omission = &runcontrol.Omission{Reason: "equivalent progress coalesced"}
		}
		omission.Count += owned.omitted
		omission.FirstAt, omission.LastAt = &owned.omittedFirst, &owned.omittedLast
		owned.omitted = 0
	}
	_, _, err := appendRunEventWithRetry(ctx, m.repository, owned.fence, runcontrol.EventDraft{
		Type: eventType, Stage: event.Stage, Task: event.Task, Payload: payload, Omission: omission,
	})
	if err != nil {
		owned.cancel()
		return false, err
	}
	if eventType == runcontrol.EventProgress {
		owned.progressKey, owned.progressAt = key, now
	}
	return true, nil
}

func (m *durableOperationManager) controlOperation(ctx context.Context, owned *ownedDurableOperation) {
	defer close(owned.done)
	ticker := time.NewTicker(runcontrol.OwnerTickInterval)
	defer ticker.Stop()
	lastHeartbeat, lastReconcile := time.Now(), time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-owned.stop:
			return
		case now := <-ticker.C:
			snapshot, err := m.repository.Snapshot(ctx, owned.fence.RunID)
			if err != nil {
				owned.cancel()
				return
			}
			if snapshot.Cancellation.State == runcontrol.CancellationRequested {
				if _, _, err := m.repository.AcknowledgeCancellation(ctx, owned.fence); err != nil {
					owned.cancel()
					return
				}
				owned.cancel()
				return
			}
			if now.Sub(lastHeartbeat) >= runcontrol.HeartbeatInterval {
				if _, err := m.repository.Heartbeat(ctx, owned.fence, runcontrol.OwnerLeaseDuration); err != nil {
					owned.cancel()
					return
				}
				lastHeartbeat = now
			}
			if now.Sub(lastReconcile) >= runcontrol.ReconciliationInterval {
				if _, err := m.repository.Reconcile(ctx, runcontrol.NativeProcessProbe{}, runcontrol.ReconcileOptions{}); err != nil {
					owned.cancel()
					return
				}
				lastReconcile = now
			}
		}
	}
}

func (m *durableOperationManager) FinishOperation(ctx context.Context, runID string, state OperationState, runErr error) error {
	m.mu.Lock()
	owned := m.owned[runID]
	delete(m.owned, runID)
	m.mu.Unlock()
	if owned == nil {
		return nil
	}
	owned.eventMu.Lock()
	if owned.omitted > 0 {
		_, _, appendErr := appendRunEventWithRetry(ctx, m.repository, owned.fence, runcontrol.EventDraft{Type: runcontrol.EventOmission, Omission: &runcontrol.Omission{Reason: "equivalent progress coalesced", Count: owned.omitted, FirstAt: &owned.omittedFirst, LastAt: &owned.omittedLast}})
		if appendErr != nil && runErr == nil {
			runErr = appendErr
		}
		owned.omitted = 0
	}
	owned.eventMu.Unlock()
	close(owned.stop)
	<-owned.done
	owned.cancel()
	outcome := runcontrol.TerminalSucceeded
	reason := "operation completed"
	switch {
	case errors.Is(runErr, context.DeadlineExceeded):
		outcome, reason = runcontrol.TerminalTimedOut, "operation deadline exceeded"
	case errors.Is(runErr, context.Canceled), state == OperationCancelled:
		outcome, reason = runcontrol.TerminalCancelled, "operation cancelled"
	case runErr != nil, state == OperationFailed:
		outcome, reason = runcontrol.TerminalFailed, "operation failed"
	case state == OperationPartial:
		outcome, reason = runcontrol.TerminalInterrupted, "operation interrupted"
	}
	_, _, err := proposeRunTerminalWithRetry(ctx, m.repository, owned.fence, runcontrol.TerminalProposal{Outcome: outcome, Reason: reason, ProposedBy: m.owner.ID})
	return err
}

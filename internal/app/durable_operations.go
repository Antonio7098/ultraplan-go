package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/runcontrol"
	"github.com/Antonio7098/ultraplan-go/internal/sprint"
)

type durableOperationManager struct {
	repository runcontrol.Repository
	owner      runcontrol.Owner
	mu         sync.Mutex
	owned      map[string]*ownedDurableOperation
}

type ownedDurableOperation struct {
	kind         OperationKind
	fence        runcontrol.Fence
	ctx          context.Context
	cancel       context.CancelFunc
	stop         chan struct{}
	done         chan struct{}
	dispatched   bool
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

type durableOperationContextKey struct{}

type durableOperationOwnership struct {
	repository runcontrol.Repository
	fence      runcontrol.Fence
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
	accepted, err = manager.DispatchOperation(deps.ctx, accepted.RunID)
	if err != nil {
		finishCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = manager.FinishOperation(finishCtx, accepted.RunID, OperationFailed, err)
		return nil, classifiedCause(ExitRuntime, err, "run-control.dispatch")
	}
	return &durableCLICommand{manager: manager, accepted: accepted}, nil
}

func (c *durableCLICommand) Context() context.Context { return c.accepted.Context }

func (c *durableCLICommand) QAWriterToken() (sprint.QAWriterToken, func(sprint.QAWriterToken) error, error) {
	if c == nil {
		return sprint.QAWriterToken{}, nil, errors.New("durable QA ownership is unavailable")
	}
	return qaOwnershipFromContext(c.accepted.Context)
}

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
	operationCtx, cancel := context.WithCancel(runcontrol.WithParentRun(ctx, snapshot.RunID))
	operationCtx = context.WithValue(operationCtx, durableOperationContextKey{}, durableOperationOwnership{repository: m.repository, fence: fence})
	owned := &ownedDurableOperation{kind: req.Kind, fence: fence, ctx: operationCtx, cancel: cancel}
	m.mu.Lock()
	m.owned[string(snapshot.RunID)] = owned
	m.mu.Unlock()
	return AcceptedOperation{RunID: string(snapshot.RunID), Context: operationCtx, Lifecycle: "claimed"}, nil
}

// DispatchOperation is the explicit hand-off from immutable confirmation to
// ownership control. AcceptOperation deliberately creates no goroutine, so a
// caller can publish confirmation under the claimed writer fence first.
func (m *durableOperationManager) DispatchOperation(ctx context.Context, runID string) (AcceptedOperation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	owned := m.owned[runID]
	if owned == nil {
		return AcceptedOperation{}, runcontrol.ErrNotFound
	}
	if owned.dispatched {
		return AcceptedOperation{RunID: runID, Context: owned.ctx, Existing: true, Lifecycle: string(runcontrol.LifecycleRunning)}, nil
	}
	owned.stop = make(chan struct{})
	owned.done = make(chan struct{})
	if _, _, err := appendRunEventWithRetry(ctx, m.repository, owned.fence, runcontrol.EventDraft{
		Type: runcontrol.EventLifecycle, Lifecycle: runcontrol.LifecycleRunning,
		Payload: map[string]string{"lifecycle": string(runcontrol.LifecycleRunning), "transition": "dispatch"},
	}); err != nil {
		terminalCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, _, _ = proposeRunTerminalWithRetry(terminalCtx, m.repository, owned.fence, runcontrol.TerminalProposal{Outcome: runcontrol.TerminalPersistenceLost, Reason: "operation dispatch persistence failed", ProposedBy: m.owner.ID})
		cancel()
		return AcceptedOperation{}, fmt.Errorf("persist confirmed operation dispatch: %w", err)
	}
	owned.dispatched = true
	go m.controlOperation(owned.ctx, owned)
	return AcceptedOperation{RunID: runID, Context: owned.ctx, Lifecycle: string(runcontrol.LifecycleRunning)}, nil
}

func qaOwnershipFromContext(ctx context.Context) (sprint.QAWriterToken, func(sprint.QAWriterToken) error, error) {
	ownership, ok := ctx.Value(durableOperationContextKey{}).(durableOperationOwnership)
	if !ok || ownership.repository == nil {
		return sprint.QAWriterToken{}, nil, errors.New("durable QA ownership is missing from the operation context")
	}
	token := sprint.QAWriterToken{RunID: string(ownership.fence.RunID), OperationalAttemptID: string(ownership.fence.AttemptID), FencingGeneration: ownership.fence.FencingGeneration}
	fence := func(got sprint.QAWriterToken) error {
		if got != token {
			return runcontrol.ErrStaleFence
		}
		fenceCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		_, err := ownership.repository.Heartbeat(fenceCtx, ownership.fence, runControlLease)
		return err
	}
	return token, fence, nil
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
	key := string(eventType) + "\x00" + event.Stage + "\x00" + event.Task + "\x00" + event.PhaseState + "\x00" + event.SafeSummary + "\x00" + event.EventType + "\x00" + event.EventKind + "\x00" + event.Tool + "\x00" + event.Reason + "\x00" + event.Detail
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
		"state": string(event.State), "type": event.EventType,
		"count": fmt.Sprintf("%d/%d", event.Completed, event.Total),
	}
	for key, value := range map[string]string{
		"phase_state": event.PhaseState, "summary": event.SafeSummary,
		"tool_call_id": event.ToolCallID, "tool_status": event.ToolStatus,
		"tool_arguments": event.ToolArguments, "tool_result": event.ToolResult, "tool_error": event.ToolError,
		"provider": event.Provider, "model": event.Model, "harness": event.Harness,
		"code": event.Code, "severity": event.Severity, "project": event.Project, "sprint": event.Sprint,
		"repair_run_id": event.RepairRunID, "operation_run_id": event.OperationRunID, "operational_attempt_id": event.OperationalAttemptID,
	} {
		if value != "" {
			payload[key] = value
		}
	}
	if event.FencingGeneration != 0 {
		payload["fencing_generation"] = fmt.Sprintf("%d", event.FencingGeneration)
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
		Type: eventType, Scope: runcontrol.EventScopeOperation, Stage: event.Stage, Task: event.Task,
		Kind: event.EventKind, Tool: event.Tool, Action: event.Action, Reason: event.Reason,
		Detail: event.Detail, Payload: payload, Omission: omission,
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
	lastHeartbeat := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-owned.stop:
			return
		case now := <-ticker.C:
			var snapshot runcontrol.Snapshot
			err := retryRunControlOperation(ctx, func(callCtx context.Context) error {
				var snapshotErr error
				snapshot, snapshotErr = m.repository.Snapshot(callCtx, owned.fence.RunID)
				return snapshotErr
			})
			if err != nil {
				owned.cancel()
				return
			}
			if snapshot.Cancellation.State == runcontrol.CancellationRequested {
				err := retryRunControlOperation(ctx, func(callCtx context.Context) error {
					_, _, acknowledgeErr := m.repository.AcknowledgeCancellation(callCtx, owned.fence)
					return acknowledgeErr
				})
				if err != nil {
					owned.cancel()
					return
				}
				owned.cancel()
				return
			}
			if now.Sub(lastHeartbeat) >= runcontrol.HeartbeatInterval {
				err := retryRunControlOperation(ctx, func(callCtx context.Context) error {
					_, heartbeatErr := m.repository.Heartbeat(callCtx, owned.fence, runcontrol.OwnerLeaseDuration)
					return heartbeatErr
				})
				if err != nil {
					owned.cancel()
					return
				}
				lastHeartbeat = now
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
	if owned.dispatched {
		close(owned.stop)
		<-owned.done
	}
	owned.cancel()
	outcome := runcontrol.TerminalSucceeded
	reason := "operation completed"
	if !owned.dispatched && owned.kind != OperationRepairPrepare {
		outcome, reason = runcontrol.TerminalPersistenceLost, "accepted operation was not confirmed and dispatched"
	} else {
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
	}
	_, _, err := proposeRunTerminalWithRetry(ctx, m.repository, owned.fence, runcontrol.TerminalProposal{Outcome: outcome, Reason: reason, ProposedBy: m.owner.ID})
	return err
}

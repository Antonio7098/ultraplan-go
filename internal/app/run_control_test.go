package app

import (
	"context"
	"errors"
	"sync"
	"testing"

	runtimepkg "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
	"github.com/Antonio7098/ultraplan-go/internal/runcontrol"
)

type controlledRuntimeSpy struct {
	started int
	deleted []string
	request runtimepkg.Request
	result  runtimepkg.Result
	err     error
	events  int
}

type appendFailureRepository struct {
	runcontrol.Repository
	err error
}

type transientAppendRepository struct {
	runcontrol.Repository
	mu        sync.Mutex
	remaining int
	calls     int
}

type batchFailureRepository struct {
	runcontrol.Repository
	calls  int
	drafts int
	err    error
}

func (r *batchFailureRepository) AppendBatch(_ context.Context, _ runcontrol.Fence, drafts []runcontrol.EventDraft) ([]runcontrol.Event, runcontrol.Snapshot, error) {
	r.calls++
	r.drafts += len(drafts)
	return nil, runcontrol.Snapshot{}, r.err
}

func (r *transientAppendRepository) Append(ctx context.Context, fence runcontrol.Fence, draft runcontrol.EventDraft) (runcontrol.Event, runcontrol.Snapshot, error) {
	r.mu.Lock()
	r.calls++
	if r.remaining > 0 {
		r.remaining--
		r.mu.Unlock()
		return runcontrol.Event{}, runcontrol.Snapshot{}, &runcontrol.Error{Code: runcontrol.CodeBusy, Operation: "append_begin", Message: "writer busy", Retryable: true}
	}
	r.mu.Unlock()
	return r.Repository.Append(ctx, fence, draft)
}

func (r appendFailureRepository) Append(context.Context, runcontrol.Fence, runcontrol.EventDraft) (runcontrol.Event, runcontrol.Snapshot, error) {
	return runcontrol.Event{}, runcontrol.Snapshot{}, r.err
}

func (s *controlledRuntimeSpy) DeleteSession(_ context.Context, sessionID string) error {
	s.deleted = append(s.deleted, sessionID)
	return nil
}

func TestControlledRuntimeForwardsSessionDeletion(t *testing.T) {
	spy := &controlledRuntimeSpy{}
	controlled := controlledRuntime{base: spy}
	if err := controlled.DeleteSession(context.Background(), "session-complete"); err != nil {
		t.Fatal(err)
	}
	if len(spy.deleted) != 1 || spy.deleted[0] != "session-complete" {
		t.Fatalf("deleted sessions=%v", spy.deleted)
	}
}

func (s *controlledRuntimeSpy) StartRun(_ context.Context, request runtimepkg.Request) (runtimepkg.Result, error) {
	s.started++
	s.request = request
	eventCount := s.events
	if eventCount == 0 {
		eventCount = 1
	}
	for i := 0; request.OnEvent != nil && i < eventCount; i++ {
		request.OnEvent(runtimepkg.Event{
			ID: "runtime-event-1", RunID: "runtime-run-1", SessionID: "session-1",
			Type: "message", Kind: "assistant", Payload: map[string]any{"secret": "must-not-persist"},
			RawPresent: true,
		})
	}
	return s.result, s.err
}

func TestControlledRuntimeBatchesEventsBeforeCallbackDelivery(t *testing.T) {
	ctx := context.Background()
	repository, err := runcontrol.OpenSQLite(ctx, t.TempDir(), runcontrol.SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	owner, err := currentRunOwner()
	if err != nil {
		t.Fatal(err)
	}
	spy := &controlledRuntimeSpy{result: runtimepkg.Result{Status: "completed"}, events: 10}
	controlled := controlledRuntime{base: spy, repository: repository, owner: owner}
	delivered := 0
	_, err = controlled.StartRun(ctx, runtimepkg.Request{OnEvent: func(runtimepkg.Event) {
		delivered++
		runID := runcontrol.RunID(spy.request.Metadata["run_control_run_id"])
		events, eventErr := repository.Events(ctx, runID, 0, 20)
		if eventErr != nil {
			t.Errorf("read committed batch: %v", eventErr)
		} else if len(events) != 10 {
			t.Errorf("committed events during callback = %d, want 10", len(events))
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	if delivered != 10 {
		t.Fatalf("delivered callbacks = %d, want 10", delivered)
	}
	health, err := repository.Health(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if health.Metrics.Append.Count != 1 {
		t.Fatalf("append transactions = %d, want 1", health.Metrics.Append.Count)
	}
}

func TestControlledRuntimeBatchFailureCancelsAndRecordsCause(t *testing.T) {
	ctx := context.Background()
	repository, err := runcontrol.OpenSQLite(ctx, t.TempDir(), runcontrol.SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	owner, err := currentRunOwner()
	if err != nil {
		t.Fatal(err)
	}
	persistenceErr := &runcontrol.Error{Code: runcontrol.CodeQuota, Operation: "append_quota", Message: "quota reached"}
	failing := &batchFailureRepository{Repository: repository, err: persistenceErr}
	spy := &controlledRuntimeSpy{result: runtimepkg.Result{Status: "completed"}, events: 10}
	controlled := controlledRuntime{base: spy, repository: failing, owner: owner}
	delivered := 0
	_, gotErr := controlled.StartRun(ctx, runtimepkg.Request{OnEvent: func(runtimepkg.Event) { delivered++ }})
	if !errors.Is(gotErr, runcontrol.ErrQuota) {
		t.Fatalf("error = %v, want quota classification", gotErr)
	}
	if failing.calls != 1 || failing.drafts != 10 || delivered != 0 {
		t.Fatalf("batch calls=%d drafts=%d delivered=%d", failing.calls, failing.drafts, delivered)
	}
	runID := runcontrol.RunID(spy.request.Metadata["run_control_run_id"])
	events, err := repository.Events(ctx, runID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	terminal := events[len(events)-1]
	if terminal.Payload["persistence_code"] != string(runcontrol.CodeQuota) || terminal.Payload["persistence_operation"] != "append_quota" {
		t.Fatalf("terminal event = %+v", terminal)
	}
}

func TestControlledRuntimeAcceptsClaimsAndCommitsBeforeDelivery(t *testing.T) {
	parentRunID, err := (runcontrol.RandomIDSource{}).NewRunID()
	if err != nil {
		t.Fatal(err)
	}
	ctx := runcontrol.WithParentRun(context.Background(), parentRunID)
	repository, err := runcontrol.OpenSQLite(ctx, t.TempDir(), runcontrol.SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	owner, err := currentRunOwner()
	if err != nil {
		t.Fatal(err)
	}
	spy := &controlledRuntimeSpy{result: runtimepkg.Result{Status: "completed", RunID: "runtime-run-1"}}
	controlled := controlledRuntime{base: spy, repository: repository, owner: owner}
	delivered := 0
	result, err := controlled.StartRun(ctx, runtimepkg.Request{
		Prompt:    "never persisted",
		PromptRef: runtimepkg.PromptReference{OwnerKind: "sprint", Purpose: "execute"},
		Metadata:  map[string]string{"project": "ultraplan-go", "sprint": "35", "stage": "execute"},
		OnEvent: func(runtimepkg.Event) {
			delivered++
			runID := runcontrol.RunID(spy.request.Metadata["run_control_run_id"])
			events, eventErr := repository.Events(ctx, runID, 0, 10)
			if eventErr != nil {
				t.Errorf("read committed event during delivery: %v", eventErr)
			} else if len(events) != 1 {
				t.Errorf("committed events during delivery = %d, want 1", len(events))
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "completed" || spy.started != 1 || delivered != 1 {
		t.Fatalf("result=%+v started=%d delivered=%d", result, spy.started, delivered)
	}
	runID := runcontrol.RunID(spy.request.Metadata["run_control_run_id"])
	if err := runID.Validate(); err != nil {
		t.Fatalf("runtime did not receive durable run ID: %v", err)
	}
	snapshot, err := repository.Snapshot(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Lifecycle != runcontrol.LifecycleSucceeded || snapshot.Target.Project != "ultraplan-go" || snapshot.Target.Sprint != "35" {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	if snapshot.Correlation.ProductRunID != string(parentRunID) {
		t.Fatalf("parent correlation = %q, want %q", snapshot.Correlation.ProductRunID, parentRunID)
	}
	events, err := repository.Events(ctx, runID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Payload["secret"] != "" || events[0].Payload["scope"] != "runtime" || events[0].Omission == nil || events[1].Type != runcontrol.EventTerminal {
		t.Fatalf("sanitized terminal journal = %+v", events)
	}
}

func TestControlledRuntimeDoesNotStartWhenAcceptancePersistenceFails(t *testing.T) {
	repository, err := runcontrol.OpenSQLite(context.Background(), t.TempDir(), runcontrol.SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}
	owner, err := currentRunOwner()
	if err != nil {
		t.Fatal(err)
	}
	spy := &controlledRuntimeSpy{}
	controlled := controlledRuntime{base: spy, repository: repository, owner: owner}
	_, err = controlled.StartRun(context.Background(), runtimepkg.Request{})
	if err == nil || spy.started != 0 {
		t.Fatalf("err=%v started=%d, want persistence error and no child start", err, spy.started)
	}
}

func TestControlledRuntimePersistsFailureWithoutLeakingRuntimeError(t *testing.T) {
	ctx := context.Background()
	repository, err := runcontrol.OpenSQLite(ctx, t.TempDir(), runcontrol.SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	owner, err := currentRunOwner()
	if err != nil {
		t.Fatal(err)
	}
	runtimeErr := errors.New("provider secret must not persist")
	spy := &controlledRuntimeSpy{result: runtimepkg.Result{Status: "failed"}, err: runtimeErr}
	controlled := controlledRuntime{base: spy, repository: repository, owner: owner}
	_, gotErr := controlled.StartRun(ctx, runtimepkg.Request{})
	if !errors.Is(gotErr, runtimeErr) {
		t.Fatalf("error = %v, want runtime error", gotErr)
	}
	runID := runcontrol.RunID(spy.request.Metadata["run_control_run_id"])
	snapshot, err := repository.Snapshot(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Lifecycle != runcontrol.LifecycleFailed || snapshot.Terminal == nil || snapshot.Terminal.Reason != "runtime failed" {
		t.Fatalf("failure snapshot = %+v", snapshot)
	}
}

func TestControlledRuntimeRecordsPersistenceFailureClassification(t *testing.T) {
	ctx := context.Background()
	repository, err := runcontrol.OpenSQLite(ctx, t.TempDir(), runcontrol.SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	owner, err := currentRunOwner()
	if err != nil {
		t.Fatal(err)
	}
	persistenceErr := &runcontrol.Error{Code: runcontrol.CodeQuota, Operation: "append_quota", Message: "quota reached"}
	spy := &controlledRuntimeSpy{result: runtimepkg.Result{Status: "completed"}}
	controlled := controlledRuntime{base: spy, repository: appendFailureRepository{Repository: repository, err: persistenceErr}, owner: owner}
	_, gotErr := controlled.StartRun(ctx, runtimepkg.Request{})
	if !errors.Is(gotErr, runcontrol.ErrQuota) {
		t.Fatalf("error = %v, want quota classification", gotErr)
	}
	runID := runcontrol.RunID(spy.request.Metadata["run_control_run_id"])
	events, err := repository.Events(ctx, runID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	terminal := events[len(events)-1]
	if terminal.Type != runcontrol.EventTerminal || terminal.Payload["persistence_code"] != string(runcontrol.CodeQuota) || terminal.Payload["persistence_operation"] != "append_quota" {
		t.Fatalf("terminal event = %+v", terminal)
	}
}

func TestControlledRuntimeWaitsForTransientWriterContention(t *testing.T) {
	ctx := context.Background()
	repository, err := runcontrol.OpenSQLite(ctx, t.TempDir(), runcontrol.SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	owner, err := currentRunOwner()
	if err != nil {
		t.Fatal(err)
	}
	contended := &transientAppendRepository{Repository: repository, remaining: 3}
	spy := &controlledRuntimeSpy{result: runtimepkg.Result{Status: "completed"}}
	controlled := controlledRuntime{base: spy, repository: contended, owner: owner}
	result, gotErr := controlled.StartRun(ctx, runtimepkg.Request{})
	if gotErr != nil || result.Status != "completed" {
		t.Fatalf("result = %+v, error = %v", result, gotErr)
	}
	contended.mu.Lock()
	calls := contended.calls
	contended.mu.Unlock()
	if calls != 4 {
		t.Fatalf("append calls = %d, want 4", calls)
	}
	runID := runcontrol.RunID(spy.request.Metadata["run_control_run_id"])
	snapshot, err := repository.Snapshot(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Terminal == nil || snapshot.Terminal.Outcome != runcontrol.TerminalSucceeded {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

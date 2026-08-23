package app

import (
	"context"
	"errors"
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
	if request.OnEvent != nil {
		request.OnEvent(runtimepkg.Event{
			ID: "runtime-event-1", RunID: "runtime-run-1", SessionID: "session-1",
			Type: "message", Kind: "assistant", Payload: map[string]any{"secret": "must-not-persist"},
			RawPresent: true,
		})
	}
	return s.result, s.err
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

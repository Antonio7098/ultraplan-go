package runcontrol

import (
	"errors"
	"testing"
	"time"
)

func TestSnapshotValidationEnforcesLifecycleAndTerminalInvariants(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)
	runID := mustRunID(t)
	base := Snapshot{
		RunID:     runID,
		Target:    Target{Kind: "sprint", Operation: "sprint.plan", Project: "ultraplan-go", Sprint: "35-durable-run-observability"},
		Lifecycle: LifecycleAccepted, Liveness: LivenessUnknown, RecordState: RecordFull,
		AcceptedAt: now, UpdatedAt: now, OldestRetainedSequence: 1, HistoryComplete: true,
		Cancellation: Cancellation{State: CancellationNone},
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid active snapshot: %v", err)
	}

	terminalWithoutWinner := base
	terminalWithoutWinner.Lifecycle = LifecycleSucceeded
	terminalWithoutWinner.Liveness = LivenessTerminal
	if err := terminalWithoutWinner.Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("terminal snapshot without winner error = %v, want invalid argument", err)
	}

	finished := now.Add(time.Second)
	validTerminal := base
	validTerminal.Lifecycle = LifecycleSucceeded
	validTerminal.Liveness = LivenessTerminal
	validTerminal.UpdatedAt = finished
	validTerminal.FinishedAt = &finished
	validTerminal.Terminal = &Terminal{Outcome: TerminalSucceeded, WonAt: finished}
	if err := validTerminal.Validate(); err != nil {
		t.Fatalf("valid terminal snapshot: %v", err)
	}

	mismatched := validTerminal
	mismatched.Terminal = &Terminal{Outcome: TerminalFailed, WonAt: finished}
	if err := mismatched.Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("mismatched terminal error = %v, want invalid argument", err)
	}

	badSequence := base
	badSequence.LastSequence = 3
	badSequence.OldestRetainedSequence = 5
	if err := badSequence.Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("bad replay bounds error = %v, want invalid argument", err)
	}
}

func TestTargetValidationRejectsUnsafeOrUnboundedValues(t *testing.T) {
	t.Parallel()
	for _, target := range []Target{
		{},
		{Kind: "sprint", Operation: ""},
		{Kind: "sprint\nforged", Operation: "plan"},
	} {
		if err := target.Validate(); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("Target%+v.Validate() error = %v, want invalid argument", target, err)
		}
	}
}

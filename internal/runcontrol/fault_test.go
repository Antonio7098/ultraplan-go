package runcontrol

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestSQLiteFullIsTypedAndNeverReturnsUncommittedAppendSuccess(t *testing.T) {
	ctx := context.Background()
	repository, fence := openClaimedRepository(t)
	var pageCount int
	if err := repository.db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pageCount); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.db.ExecContext(ctx, fmt.Sprintf("PRAGMA max_page_count=%d", pageCount+1)); err != nil {
		t.Fatal(err)
	}
	payload := map[string]string{"message": strings.Repeat("bounded-capacity-evidence-", 500)}
	committed := uint64(0)
	var fullErr error
	for index := 0; index < 100; index++ {
		event, _, err := repository.Append(ctx, fence, EventDraft{Type: EventMessage, Payload: payload})
		if err != nil {
			fullErr = err
			break
		}
		committed = event.Sequence
	}
	if !errors.Is(fullErr, ErrQuota) {
		t.Fatalf("append error=%v, want typed quota/full after sequence %d", fullErr, committed)
	}
	snapshot, err := repository.Snapshot(ctx, fence.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.LastSequence != committed {
		t.Fatalf("snapshot sequence=%d, last committed=%d", snapshot.LastSequence, committed)
	}
	events, err := repository.Events(ctx, fence.RunID, 0, 512)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(len(events)) != committed {
		t.Fatalf("retained events=%d, committed=%d", len(events), committed)
	}
}

func TestClosedRepositoryFailsEveryMutationWithoutStaleSuccess(t *testing.T) {
	ctx := context.Background()
	repository, fence := openClaimedRepository(t)
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		name string
		call func() error
	}{
		{"accept", func() error {
			_, err := repository.Accept(ctx, Acceptance{Target: Target{Kind: "fault", Operation: "accept"}})
			return err
		}},
		{"append", func() error { _, _, err := repository.Append(ctx, fence, EventDraft{Type: EventMessage}); return err }},
		{"heartbeat", func() error { _, err := repository.Heartbeat(ctx, fence, OwnerLeaseDuration); return err }},
		{"cancel", func() error {
			_, _, err := repository.RequestCancellation(ctx, fence.RunID, "user_requested")
			return err
		}},
		{"cancel-ack", func() error { _, _, err := repository.AcknowledgeCancellation(ctx, fence); return err }},
		{"terminal", func() error {
			_, _, err := repository.ProposeTerminal(ctx, fence, TerminalProposal{Outcome: TerminalFailed, Reason: "fault", ProposedBy: "test"})
			return err
		}},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.call(); err == nil || (!errors.Is(err, ErrUnavailable) && !errors.Is(err, ErrBusy)) {
				t.Fatalf("mutation error=%v, want typed repository failure", err)
			}
		})
	}
}

func TestSQLiteReadOnlyPermissionLossRejectsEveryActiveMutation(t *testing.T) {
	ctx := context.Background()
	repository := openTestRepository(t, t.TempDir())
	defer repository.Close()
	repository.db.SetMaxOpenConns(1)
	repository.db.SetMaxIdleConns(1)
	runID, fence := acceptedClaimedRun(t, repository)
	before, err := repository.Snapshot(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.db.ExecContext(ctx, `PRAGMA query_only = ON`); err != nil {
		t.Fatal(err)
	}

	checks := []struct {
		name string
		call func() error
	}{
		{"append", func() error { _, _, err := repository.Append(ctx, fence, EventDraft{Type: EventMessage}); return err }},
		{"heartbeat", func() error { _, err := repository.Heartbeat(ctx, fence, time.Minute); return err }},
		{"cancel", func() error { _, _, err := repository.RequestCancellation(ctx, runID, "user_requested"); return err }},
		{"acknowledge", func() error { _, _, err := repository.AcknowledgeCancellation(ctx, fence); return err }},
		{"terminal", func() error {
			_, _, err := repository.ProposeTerminal(ctx, fence, TerminalProposal{Outcome: TerminalSucceeded, Reason: "must not commit", ProposedBy: "test"})
			return err
		}},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.call(); !errors.Is(err, ErrPermission) {
				t.Fatalf("error=%v, want typed permission loss", err)
			}
		})
	}
	after, err := repository.Snapshot(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if after.LastSequence != before.LastSequence || after.Lifecycle != before.Lifecycle || after.Cancellation.State != before.Cancellation.State || after.Terminal != nil {
		t.Fatalf("read-only failures reported stale success: before=%+v after=%+v", before, after)
	}
}

package runcontrol

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPerRunEventLimitAdvancesReplayBoundaryAndPreservesNewestCommit(t *testing.T) {
	ctx := context.Background()
	repository, fence := openClaimedRepository(t)
	now := formatTime(time.Now().UTC())
	if _, err := repository.db.ExecContext(ctx, `
WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM seq WHERE n < 4096)
INSERT INTO events (run_id, sequence, committed_at, event_type, payload_json)
SELECT ?, n, ?, 'progress', '{}' FROM seq`, string(fence.RunID), now); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.db.ExecContext(ctx, `UPDATE runs SET last_sequence = 4096 WHERE run_id = ?`, string(fence.RunID)); err != nil {
		t.Fatal(err)
	}
	event, snapshot, err := repository.Append(ctx, fence, EventDraft{Type: EventProgress, Payload: map[string]string{"status": "newest"}})
	if err != nil {
		t.Fatal(err)
	}
	if event.Sequence != 4097 || snapshot.LastSequence != 4097 || snapshot.OldestRetainedSequence != 2 || snapshot.HistoryComplete || snapshot.RecordState != RecordCompacted {
		t.Fatalf("event=%+v snapshot=%+v", event, snapshot)
	}
	retained, err := repository.Events(ctx, fence.RunID, 4096, 10)
	if err != nil || len(retained) != 1 || retained[0].Payload["status"] != "newest" {
		t.Fatalf("newest retained=%+v err=%v", retained, err)
	}
}

func TestTerminalRetentionCompactsTombstonesAndExpiresInOrder(t *testing.T) {
	ctx := context.Background()
	clock := &mutableClock{at: time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)}
	repository, err := OpenSQLite(ctx, t.TempDir(), SQLiteOptions{Clock: clock, Retention: RetentionPolicy{FullHistory: time.Hour, TombstoneHistory: 24 * time.Hour, HardQuotaBytes: 64 << 20}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	snapshot, err := repository.Accept(ctx, Acceptance{Target: Target{Kind: "sprint", Operation: "execute"}})
	if err != nil {
		t.Fatal(err)
	}
	owner := Owner{ID: "retention-owner", Process: ProcessIdentity{PID: 1}}
	attempt, _, err := repository.Claim(ctx, Claim{RunID: snapshot.RunID, Owner: owner, Lease: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	fence := Fence{RunID: snapshot.RunID, AttemptID: attempt.ID, OwnerID: owner.ID, FencingGeneration: attempt.FencingGeneration}
	if _, _, err := repository.Append(ctx, fence, EventDraft{Type: EventProgress, Payload: map[string]string{"status": "working"}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repository.Append(ctx, fence, EventDraft{Type: EventWarning, Payload: map[string]string{"code": "safe_warning"}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repository.ProposeTerminal(ctx, fence, TerminalProposal{Outcome: TerminalSucceeded, Reason: "completed"}); err != nil {
		t.Fatal(err)
	}

	clock.at = clock.at.Add(31 * time.Minute)
	report, err := repository.Compact(ctx, 10)
	if err != nil || report.CompactedRuns != 1 || report.DeletedEvents != 1 {
		t.Fatalf("compact report=%+v err=%v", report, err)
	}
	compacted, err := repository.Snapshot(ctx, snapshot.RunID)
	if err != nil || compacted.RecordState != RecordCompacted || compacted.HistoryComplete {
		t.Fatalf("compacted=%+v err=%v", compacted, err)
	}
	events, err := repository.Events(ctx, snapshot.RunID, 0, 10)
	if err != nil || len(events) != 2 || events[0].Type != EventWarning || events[1].Type != EventTerminal {
		t.Fatalf("preserved events=%+v err=%v", events, err)
	}

	clock.at = clock.at.Add(30 * time.Minute)
	report, err = repository.Compact(ctx, 10)
	if err != nil || report.TombstonedRuns != 1 {
		t.Fatalf("tombstone report=%+v err=%v", report, err)
	}
	tombstone, err := repository.Snapshot(ctx, snapshot.RunID)
	if err != nil || tombstone.RecordState != RecordTombstone || tombstone.Terminal == nil {
		t.Fatalf("tombstone=%+v err=%v", tombstone, err)
	}

	clock.at = clock.at.Add(24*time.Hour + time.Minute)
	report, err = repository.Compact(ctx, 10)
	if err != nil || report.DeletedRuns != 1 {
		t.Fatalf("expiry report=%+v err=%v", report, err)
	}
	if _, err := repository.Snapshot(ctx, snapshot.RunID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired snapshot err=%v", err)
	}
}

func TestSoftQuotaRejectsAcceptanceAndHealthReportsReservedHeadroom(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repository, err := OpenSQLite(ctx, root, SQLiteOptions{Retention: RetentionPolicy{FullHistory: time.Hour, TombstoneHistory: 24 * time.Hour, HardQuotaBytes: 64 << 20}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	quotaFixture := filepath.Join(root, ".ultraplan", "run-control.db.quota-fixture")
	file, err := os.OpenFile(quotaFixture, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(49 << 20); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Accept(ctx, Acceptance{Target: Target{Kind: "test", Operation: "quota"}}); !errors.Is(err, ErrQuota) {
		t.Fatalf("quota acceptance error=%v", err)
	}
	health, err := repository.Health(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if health.Status != HealthDegraded || health.SoftQuotaBytes != 48<<20 || health.HardQuotaBytes != 64<<20 || health.ReservedHeadroomBytes != 16<<20 {
		t.Fatalf("quota health=%+v", health)
	}
}

package runcontrol

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSQLiteRepositoryCreatesPrivateDurableSchema(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repository := openTestRepository(t, root)
	defer repository.Close()

	directoryInfo, err := os.Stat(filepath.Join(root, ".ultraplan"))
	if err != nil {
		t.Fatalf("stat run-control directory: %v", err)
	}
	if got := directoryInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("run-control directory mode = %04o, want 0700", got)
	}
	databaseInfo, err := os.Stat(filepath.Join(root, DatabaseRelativePath))
	if err != nil {
		t.Fatalf("stat database: %v", err)
	}
	if got := databaseInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("database mode = %04o, want 0600", got)
	}

	health, err := repository.Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if health.Status != HealthOK || !strings.EqualFold(health.JournalMode, "wal") || health.Synchronous != "2" || !health.ForeignKeys || health.BusyTimeout != 5*time.Second {
		t.Fatalf("unexpected durable SQLite policy: %+v", health)
	}

	wantTables := []string{"app_schema", "attempts", "events", "operation_aliases", "reconciliation_log", "runs"}
	rows, err := repository.db.Query(`SELECT name FROM sqlite_schema WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatalf("query schema tables: %v", err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		tables = append(tables, name)
	}
	if fmt.Sprint(tables) != fmt.Sprint(wantTables) {
		t.Fatalf("schema tables = %v, want %v", tables, wantTables)
	}
	wantIndexes := []string{
		"idx_attempts_owner_lease", "idx_attempts_run_fence", "idx_events_replay", "idx_events_retention",
		"idx_operation_aliases_run", "idx_reconciliation_run_time", "idx_runs_active_updated",
		"idx_runs_retention", "idx_runs_target_updated",
	}
	indexRows, err := repository.db.Query(`SELECT name FROM sqlite_schema WHERE type = 'index' AND name LIKE 'idx_%' ORDER BY name`)
	if err != nil {
		t.Fatalf("query schema indexes: %v", err)
	}
	defer indexRows.Close()
	var indexes []string
	for indexRows.Next() {
		var name string
		if err := indexRows.Scan(&name); err != nil {
			t.Fatalf("scan index name: %v", err)
		}
		indexes = append(indexes, name)
	}
	if fmt.Sprint(indexes) != fmt.Sprint(wantIndexes) {
		t.Fatalf("schema indexes = %v, want %v", indexes, wantIndexes)
	}

	var foreignKeyViolations int
	err = repository.db.QueryRow(`SELECT COUNT(*) FROM pragma_foreign_key_check`).Scan(&foreignKeyViolations)
	if err != nil || foreignKeyViolations != 0 {
		t.Fatalf("foreign key check = %d, %v", foreignKeyViolations, err)
	}
	if _, err := repository.db.Exec(`INSERT INTO events (run_id, sequence, committed_at, event_type) VALUES ('run_aaaaaaaaaaaaaaaaaaaaaaaaaa', 1, '2026-08-21T00:00:00Z', 'message')`); err == nil {
		t.Fatal("foreign key policy permitted an event for a missing run")
	}
}

type capturedLog struct {
	message string
	fields  []LogField
}

type captureRunLogger struct {
	mu   sync.Mutex
	logs []capturedLog
}

func (l *captureRunLogger) Log(_ context.Context, _ LogLevel, message string, fields ...LogField) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logs = append(l.logs, capturedLog{message: message, fields: append([]LogField(nil), fields...)})
}

func TestStructuredRunLogsUseSafeBoundedCorrelationFields(t *testing.T) {
	logger := &captureRunLogger{}
	repository, err := OpenSQLite(context.Background(), t.TempDir(), SQLiteOptions{Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	run, err := repository.Accept(context.Background(), Acceptance{Target: Target{Kind: "sprint", Operation: "execute", Project: "alpha"}})
	if err != nil {
		t.Fatal(err)
	}
	owner := Owner{ID: "safe-owner", Process: ProcessIdentity{PID: 1}}
	attempt, _, err := repository.Claim(context.Background(), Claim{RunID: run.RunID, Owner: owner, Lease: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	fence := Fence{RunID: run.RunID, AttemptID: attempt.ID, OwnerID: owner.ID, FencingGeneration: attempt.FencingGeneration}
	if _, _, err := repository.Append(context.Background(), fence, EventDraft{Type: EventProgress, Payload: map[string]string{"message": "credential=must-not-log"}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repository.ProposeTerminal(context.Background(), fence, TerminalProposal{Outcome: TerminalSucceeded, Reason: "safe terminal", ProposedBy: owner.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Accept(context.Background(), Acceptance{}); err == nil {
		t.Fatal("invalid acceptance unexpectedly succeeded")
	}
	health, err := repository.Health(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if health.Metrics.Acceptance.Count != 2 || health.Metrics.Acceptance.Failures != 1 || health.Metrics.Append.Count != 1 || health.Metrics.Terminal.Count != 1 {
		t.Fatalf("local metrics = %+v", health.Metrics)
	}
	logger.mu.Lock()
	defer logger.mu.Unlock()
	encoded := fmt.Sprint(logger.logs)
	for _, want := range []string{"run_id", "attempt_id", "fencing_generation", "sequence", "event_type", "terminal_outcome", "terminal_winner"} {
		if !strings.Contains(encoded, want) {
			t.Fatalf("structured logs missing %q: %s", want, encoded)
		}
	}
	if strings.Contains(encoded, "must-not-log") {
		t.Fatalf("structured logs leaked event payload: %s", encoded)
	}
}

func TestAppendBatchCommitsOrderedEventsInOneTransaction(t *testing.T) {
	ctx := context.Background()
	repository := openTestRepository(t, t.TempDir())
	defer repository.Close()
	run, err := repository.Accept(ctx, Acceptance{Target: Target{Kind: "study", Operation: "analysis"}})
	if err != nil {
		t.Fatal(err)
	}
	owner := Owner{ID: "batch-owner", Process: ProcessIdentity{PID: 1}}
	attempt, _, err := repository.Claim(ctx, Claim{RunID: run.RunID, Owner: owner, Lease: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	fence := Fence{RunID: run.RunID, AttemptID: attempt.ID, OwnerID: owner.ID, FencingGeneration: attempt.FencingGeneration}
	events, snapshot, err := repository.AppendBatch(ctx, fence, []EventDraft{
		{Type: EventMessage, Payload: map[string]string{"message": "one"}},
		{Type: EventProgress, Payload: map[string]string{"message": "two"}},
		{Type: EventArtifact, Payload: map[string]string{"message": "three"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[0].Sequence+2 != events[2].Sequence || snapshot.LastSequence != events[2].Sequence {
		t.Fatalf("events = %+v snapshot sequence = %d", events, snapshot.LastSequence)
	}
	stored, err := repository.Events(ctx, run.RunID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 3 || stored[0].Payload["message"] != "one" || stored[2].Payload["message"] != "three" {
		t.Fatalf("stored batch = %+v", stored)
	}
	health, err := repository.Health(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if health.Metrics.Append.Count != 1 {
		t.Fatalf("append transaction count = %d, want 1", health.Metrics.Append.Count)
	}
}

func TestAppendBatchRejectsWholeBatchBeforeWriting(t *testing.T) {
	ctx := context.Background()
	repository := openTestRepository(t, t.TempDir())
	defer repository.Close()
	run, err := repository.Accept(ctx, Acceptance{Target: Target{Kind: "runtime", Operation: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	owner := Owner{ID: "batch-owner", Process: ProcessIdentity{PID: 1}}
	attempt, _, err := repository.Claim(ctx, Claim{RunID: run.RunID, Owner: owner, Lease: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	fence := Fence{RunID: run.RunID, AttemptID: attempt.ID, OwnerID: owner.ID, FencingGeneration: attempt.FencingGeneration}
	_, _, err = repository.AppendBatch(ctx, fence, []EventDraft{
		{Type: EventLifecycle, Lifecycle: LifecycleCancelling},
		{Type: EventLifecycle, Lifecycle: LifecycleRunning},
	})
	if err == nil {
		t.Fatal("invalid batch unexpectedly committed")
	}
	stored, readErr := repository.Events(ctx, run.RunID, 0, 10)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(stored) != 0 {
		t.Fatalf("partial batch committed: %+v", stored)
	}
}

func TestSQLiteRepositoryPersistsCommittedStateAcrossReopen(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repository := openTestRepository(t, root)
	runID, fence := acceptedClaimedRun(t, repository)
	if _, _, err := repository.Append(context.Background(), fence, EventDraft{Type: EventMessage, Payload: map[string]string{"message": "committed"}}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := repository.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened := openTestRepository(t, root)
	defer reopened.Close()
	snapshot, err := reopened.Snapshot(context.Background(), runID)
	if err != nil {
		t.Fatalf("Snapshot after reopen: %v", err)
	}
	if snapshot.LastSequence != 1 || snapshot.Lifecycle != LifecycleRunning {
		t.Fatalf("snapshot after reopen = %+v", snapshot)
	}
	var integrity string
	if err := reopened.db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil {
		t.Fatalf("integrity_check: %v", err)
	}
	if integrity != "ok" {
		t.Fatalf("integrity_check = %q, want ok", integrity)
	}
	if _, err := reopened.db.Exec(`UPDATE events SET payload_json = '{}' WHERE run_id = ? AND sequence = 1`, string(runID)); err == nil {
		t.Fatal("immutable event journal allowed an in-place update")
	}
	events, err := reopened.Events(context.Background(), runID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Payload["message"] != "committed" {
		t.Fatalf("persisted events = %+v", events)
	}
}

func TestSQLiteRepositoryRejectsSymlinkDatabaseBoundary(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.Mkdir(filepath.Join(root, ".ultraplan"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, DatabaseRelativePath)); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenSQLite(context.Background(), root, SQLiteOptions{}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("OpenSQLite error = %v, want invalid argument", err)
	}
}

func TestSQLiteRepositoryLifecycleEventAndTerminalContract(t *testing.T) {
	t.Parallel()
	repository := openTestRepository(t, t.TempDir())
	defer repository.Close()

	runID := mustRunID(t)
	snapshot, err := repository.Accept(context.Background(), Acceptance{
		RunID:          runID,
		Target:         Target{Kind: "sprint", Operation: "sprint.execute", Project: "ultraplan-go", Sprint: "35-durable-run-observability"},
		ProductStatus:  "planned",
		OperationAlias: "op_legacy-visible",
	})
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if snapshot.Lifecycle != LifecycleAccepted || snapshot.LastSequence != 0 {
		t.Fatalf("accepted snapshot = %+v", snapshot)
	}
	if _, err := repository.Accept(context.Background(), Acceptance{RunID: runID, Target: snapshot.Target}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate Accept error = %v, want conflict", err)
	}

	attemptID := mustAttemptID(t)
	attempt, claimed, err := repository.Claim(context.Background(), Claim{
		RunID: runID, AttemptID: attemptID,
		Owner: Owner{ID: "owner-alpha", Process: ProcessIdentity{HostDigest: "host-digest", BootID: "boot-id", PID: 1234, BirthToken: "birth-token"}},
		Lease: 15 * time.Second,
	})
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if attempt.FencingGeneration != 1 || claimed.Lifecycle != LifecycleRunning || claimed.CurrentAttemptID != attemptID {
		t.Fatalf("claim attempt=%+v snapshot=%+v", attempt, claimed)
	}
	fence := Fence{RunID: runID, AttemptID: attemptID, OwnerID: "owner-alpha", FencingGeneration: 1}
	event, progressed, err := repository.Append(context.Background(), fence, EventDraft{
		Type: EventProgress, Stage: "execute", Task: "task-1", Payload: map[string]string{"message": "repository ready"},
		Omission: &Omission{Reason: "equivalent_progress", Count: 2},
	})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if event.Sequence != 1 || progressed.LastSequence != 1 || progressed.OmissionTotal != 2 {
		t.Fatalf("event=%+v snapshot=%+v", event, progressed)
	}

	winner, won, err := repository.ProposeTerminal(context.Background(), fence, TerminalProposal{Outcome: TerminalSucceeded, Reason: "product completed", ProposedBy: "owner"})
	if err != nil {
		t.Fatalf("ProposeTerminal: %v", err)
	}
	if !won || winner.Lifecycle != LifecycleSucceeded || winner.Terminal == nil || winner.Terminal.Outcome != TerminalSucceeded || winner.LastSequence != 2 {
		t.Fatalf("terminal winner won=%v snapshot=%+v", won, winner)
	}
	loser, won, err := repository.ProposeTerminal(context.Background(), fence, TerminalProposal{Outcome: TerminalFailed, Reason: "late failure", ProposedBy: "late"})
	if err != nil {
		t.Fatalf("second ProposeTerminal: %v", err)
	}
	if won || loser.Terminal == nil || loser.Terminal.Outcome != TerminalSucceeded || loser.LastSequence != 2 {
		t.Fatalf("terminal result changed: won=%v snapshot=%+v", won, loser)
	}
	if _, _, err := repository.Append(context.Background(), fence, EventDraft{Type: EventMessage, Payload: map[string]string{"message": "late"}}); !errors.Is(err, ErrTerminal) {
		t.Fatalf("append after terminal error = %v, want terminal", err)
	}
	events, err := repository.Events(context.Background(), runID, 0, 10)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(events) != 2 || events[0].Sequence != 1 || events[1].Sequence != 2 || events[1].Type != EventTerminal {
		t.Fatalf("events = %+v", events)
	}
}

func TestSQLiteRepositoryConcurrentWritersAllocateMonotonicSequence(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	first := openTestRepository(t, root)
	defer first.Close()
	second := openTestRepository(t, root)
	defer second.Close()
	runID, fence := acceptedClaimedRun(t, first)

	const writes = 40
	sequences := make(chan uint64, writes)
	errorsCh := make(chan error, writes)
	var wait sync.WaitGroup
	for i := 0; i < writes; i++ {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			repository := first
			if i%2 == 1 {
				repository = second
			}
			event, _, err := repository.Append(context.Background(), fence, EventDraft{Type: EventMessage, Payload: map[string]string{"index": fmt.Sprint(i)}})
			if err != nil {
				errorsCh <- err
				return
			}
			sequences <- event.Sequence
		}(i)
	}
	wait.Wait()
	close(errorsCh)
	close(sequences)
	for err := range errorsCh {
		t.Errorf("concurrent Append: %v", err)
	}
	if t.Failed() {
		return
	}
	got := make([]int, 0, writes)
	for sequence := range sequences {
		got = append(got, int(sequence))
	}
	sort.Ints(got)
	for i, sequence := range got {
		if want := i + 1; sequence != want {
			t.Fatalf("sorted sequence[%d] = %d, want %d; all=%v", i, sequence, want, got)
		}
	}
	snapshot, err := second.Snapshot(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.LastSequence != writes {
		t.Fatalf("last sequence = %d, want %d", snapshot.LastSequence, writes)
	}
}

func TestSQLiteRepositoryTerminalCompareAndSetHasOneWinner(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	first := openTestRepository(t, root)
	defer first.Close()
	second := openTestRepository(t, root)
	defer second.Close()
	runID, fence := acceptedClaimedRun(t, first)

	type result struct {
		snapshot Snapshot
		won      bool
		err      error
	}
	results := make(chan result, 2)
	go func() {
		snapshot, won, err := first.ProposeTerminal(context.Background(), fence, TerminalProposal{Outcome: TerminalSucceeded, ProposedBy: "first"})
		results <- result{snapshot: snapshot, won: won, err: err}
	}()
	go func() {
		snapshot, won, err := second.ProposeTerminal(context.Background(), fence, TerminalProposal{Outcome: TerminalFailed, ProposedBy: "second"})
		results <- result{snapshot: snapshot, won: won, err: err}
	}()
	winners := 0
	var outcome TerminalOutcome
	for i := 0; i < 2; i++ {
		result := <-results
		if result.err != nil {
			t.Fatalf("ProposeTerminal: %v", result.err)
		}
		if result.won {
			winners++
		}
		if result.snapshot.Terminal == nil {
			t.Fatalf("terminal snapshot missing: %+v", result.snapshot)
		}
		if outcome == "" {
			outcome = result.snapshot.Terminal.Outcome
		} else if result.snapshot.Terminal.Outcome != outcome {
			t.Fatalf("observers disagree on immutable winner: %s vs %s", outcome, result.snapshot.Terminal.Outcome)
		}
	}
	if winners != 1 {
		t.Fatalf("terminal winners = %d, want 1", winners)
	}
	events, err := first.Events(context.Background(), runID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != EventTerminal {
		t.Fatalf("terminal events = %+v, want exactly one", events)
	}
}

func TestSQLiteRepositoryImmediateTransactionsRespectContextAndLocking(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	first := openTestRepository(t, root)
	defer first.Close()
	second := openTestRepository(t, root)
	defer second.Close()
	_, fence := acceptedClaimedRun(t, first)

	lock, err := first.db.BeginTx(context.Background(), &sql.TxOptions{})
	if err != nil {
		t.Fatalf("begin lock transaction: %v", err)
	}
	if _, err := lock.Exec(`UPDATE runs SET updated_at = updated_at WHERE run_id = ?`, string(fence.RunID)); err != nil {
		t.Fatalf("acquire write lock: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, _, err := second.Append(ctx, fence, EventDraft{Type: EventMessage}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked writer error = %v, want deadline exceeded", err)
	}
	if err := lock.Rollback(); err != nil {
		t.Fatalf("rollback lock transaction: %v", err)
	}
	if _, _, err := second.Append(context.Background(), fence, EventDraft{Type: EventMessage}); err != nil {
		t.Fatalf("append after releasing lock: %v", err)
	}
}

func openTestRepository(t *testing.T, root string) *SQLiteRepository {
	t.Helper()
	repository, err := OpenSQLite(context.Background(), root, SQLiteOptions{Clock: fixedClock{at: time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)}})
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	return repository
}

func acceptedClaimedRun(t *testing.T, repository *SQLiteRepository) (RunID, Fence) {
	t.Helper()
	runID := mustRunID(t)
	if _, err := repository.Accept(context.Background(), Acceptance{RunID: runID, Target: Target{Kind: "study", Operation: "study.run", Study: "example"}}); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	attemptID := mustAttemptID(t)
	attempt, _, err := repository.Claim(context.Background(), Claim{RunID: runID, AttemptID: attemptID, Owner: Owner{ID: "owner-test"}, Lease: 15 * time.Second})
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	return runID, Fence{RunID: runID, AttemptID: attemptID, OwnerID: "owner-test", FencingGeneration: attempt.FencingGeneration}
}

func mustRunID(t *testing.T) RunID {
	t.Helper()
	id, err := (RandomIDSource{}).NewRunID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustAttemptID(t *testing.T) AttemptID {
	t.Helper()
	id, err := (RandomIDSource{}).NewAttemptID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

type fixedClock struct{ at time.Time }

func (c fixedClock) Now() time.Time { return c.at }

package runcontrol

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const (
	DatabaseRelativePath = ".ultraplan/run-control.db"
	BusyTimeout          = 5 * time.Second
	defaultMaxOpenConns  = 4
	maxOpenConns         = 16
	defaultEventPage     = 200
	maxEventPage         = 512
)

// SQLiteOptions controls process-local seams and connection bounds. Durable
// safety pragmas are fixed product invariants and cannot be weakened here.
type SQLiteOptions struct {
	Clock        Clock
	IDs          IDSource
	Logger       Logger
	Notifier     Notifier
	MaxOpenConns int
	Retention    RetentionPolicy
}

// SQLiteRepository is a direct multi-process repository over one workspace
// database. Every mutation uses a short immediate transaction.
type SQLiteRepository struct {
	db            *sql.DB
	path          string
	clock         Clock
	clockInjected bool
	ids           IDSource
	loggerMu      sync.RWMutex
	logger        Logger
	notifier      Notifier
	retention     RetentionPolicy
	softQuota     int64
	metrics       repositoryMetrics
}

var _ Repository = (*SQLiteRepository)(nil)

// OpenSQLite opens or creates the private workspace run-control database.
func OpenSQLite(ctx context.Context, workspaceRoot string, opts SQLiteOptions) (*SQLiteRepository, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	root, err := validateWorkspaceRoot(workspaceRoot)
	if err != nil {
		return nil, err
	}
	databasePath, err := preparePrivateDatabase(root)
	if err != nil {
		return nil, err
	}

	values := url.Values{}
	values.Set("_busy_timeout", strconv.FormatInt(BusyTimeout.Milliseconds(), 10))
	values.Set("_foreign_keys", "on")
	values.Set("_journal_mode", "WAL")
	values.Set("_synchronous", "FULL")
	values.Set("_txlock", "immediate")
	values.Set("_defensive", "1")
	dsn := (&url.URL{Scheme: "file", Path: filepath.ToSlash(databasePath), RawQuery: values.Encode()}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, classifyStoreError("open", "open run-control database failed", err)
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = db.Close()
		}
	}()

	connections := opts.MaxOpenConns
	if connections == 0 {
		connections = defaultMaxOpenConns
	}
	if connections < 1 || connections > maxOpenConns {
		return nil, invalidField("max_open_connections", fmt.Sprintf("must be between 1 and %d", maxOpenConns))
	}
	db.SetMaxOpenConns(connections)
	db.SetMaxIdleConns(connections)
	if err := db.PingContext(ctx); err != nil {
		return nil, classifyStoreError("ping", "run-control database is unavailable", err)
	}
	if err := verifyPragmas(ctx, db); err != nil {
		return nil, err
	}
	clock := opts.Clock
	clockInjected := clock != nil
	if clock == nil {
		clock = WallClock{}
	}
	if err := migrateSchema(ctx, db, databasePath, clock); err != nil {
		return nil, err
	}
	if err := enforcePrivateMode(databasePath, 0o600); err != nil {
		return nil, err
	}

	ids := opts.IDs
	if ids == nil {
		ids = RandomIDSource{}
	}
	retention, err := normalizeRetentionPolicy(opts.Retention)
	if err != nil {
		return nil, err
	}
	repository := &SQLiteRepository{
		db: db, path: databasePath, clock: clock, clockInjected: clockInjected, ids: ids,
		logger: opts.Logger, notifier: opts.Notifier,
		retention: retention, softQuota: retention.HardQuotaBytes - ReservedHeadroomBytes,
	}
	closeOnError = false
	return repository, nil
}

func validateWorkspaceRoot(workspaceRoot string) (string, error) {
	if strings.TrimSpace(workspaceRoot) == "" {
		return "", invalidField("workspace_root", "is required")
	}
	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", wrapError(CodeInvalidArgument, "resolve_workspace", "workspace root is invalid", false, err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", classifyStoreError("stat_workspace", "workspace root cannot be inspected", err)
	}
	if !info.IsDir() {
		return "", invalidField("workspace_root", "must be a directory")
	}
	return filepath.Clean(root), nil
}

func preparePrivateDatabase(root string) (string, error) {
	directory := filepath.Join(root, ".ultraplan")
	info, err := os.Lstat(directory)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err := os.Mkdir(directory, 0o700); err != nil {
			return "", classifyStoreError("create_directory", "create private run-control directory failed", err)
		}
	case err != nil:
		return "", classifyStoreError("inspect_directory", "inspect run-control directory failed", err)
	case info.Mode()&os.ModeSymlink != 0 || !info.IsDir():
		return "", invalidField("run_control_directory", "must be a real directory, not a symlink")
	}
	if err := enforcePrivateMode(directory, 0o700); err != nil {
		return "", err
	}

	path := filepath.Join(directory, "run-control.db")
	info, err = os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		file, createErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if createErr != nil {
			return "", classifyStoreError("create_database", "create private run-control database failed", createErr)
		}
		if closeErr := file.Close(); closeErr != nil {
			return "", classifyStoreError("close_database", "close new run-control database failed", closeErr)
		}
	case err != nil:
		return "", classifyStoreError("inspect_database", "inspect run-control database failed", err)
	case info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular():
		return "", invalidField("run_control_database", "must be a regular file, not a symlink")
	}
	if err := enforcePrivateMode(path, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func enforcePrivateMode(path string, want fs.FileMode) error {
	if err := os.Chmod(path, want); err != nil {
		return classifyStoreError("set_permissions", "set private run-control permissions failed", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return classifyStoreError("verify_permissions", "verify run-control permissions failed", err)
	}
	if got := info.Mode().Perm(); got != want {
		return wrapError(CodePermission, "verify_permissions", fmt.Sprintf("private permissions are %04o; require %04o", got, want), false, nil)
	}
	return nil
}

func verifyPragmas(ctx context.Context, db *sql.DB) error {
	checks := []struct {
		name string
		want string
	}{
		{name: "journal_mode", want: "wal"},
		{name: "synchronous", want: "2"},
		{name: "foreign_keys", want: "1"},
		{name: "busy_timeout", want: "5000"},
	}
	for _, check := range checks {
		var got string
		if err := db.QueryRowContext(ctx, "PRAGMA "+check.name).Scan(&got); err != nil {
			return classifyStoreError("verify_pragma", "verify required SQLite durability policy failed", err)
		}
		if !strings.EqualFold(got, check.want) {
			return wrapError(CodeInvariant, "verify_pragma", fmt.Sprintf("SQLite %s=%s; require %s", check.name, got, check.want), false, nil)
		}
	}
	return nil
}

const initialSchema = `
CREATE TABLE IF NOT EXISTS app_schema (
    component TEXT PRIMARY KEY,
    version INTEGER NOT NULL CHECK (version > 0),
    migrated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS runs (
    run_id TEXT PRIMARY KEY,
    target_kind TEXT NOT NULL,
    operation_kind TEXT NOT NULL,
    project_id TEXT NOT NULL DEFAULT '',
    sprint_id TEXT NOT NULL DEFAULT '',
    study_id TEXT NOT NULL DEFAULT '',
    stage_id TEXT NOT NULL DEFAULT '',
    task_id TEXT NOT NULL DEFAULT '',
    lifecycle TEXT NOT NULL CHECK (lifecycle IN ('accepted','queued','running','cancelling','succeeded','failed','cancelled','timed_out','interrupted','cleanup_uncertain','persistence_degraded')),
    liveness TEXT NOT NULL CHECK (liveness IN ('unknown','live','stalled','owner_unreachable','interrupted','cleanup_uncertain','terminal')),
    record_state TEXT NOT NULL CHECK (record_state IN ('full','compacted','tombstone')),
    accepted_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    started_at TEXT,
    finished_at TEXT,
    current_attempt_id TEXT,
    last_sequence INTEGER NOT NULL DEFAULT 0 CHECK (last_sequence >= 0),
    oldest_retained_sequence INTEGER NOT NULL DEFAULT 1 CHECK (oldest_retained_sequence >= 1),
    history_complete INTEGER NOT NULL DEFAULT 1 CHECK (history_complete IN (0,1)),
    omission_total INTEGER NOT NULL DEFAULT 0 CHECK (omission_total >= 0),
    correlation_json TEXT NOT NULL DEFAULT '{}',
    product_status TEXT NOT NULL DEFAULT '',
    confirmation_digest TEXT NOT NULL DEFAULT '',
    cancellation_state TEXT NOT NULL DEFAULT 'none' CHECK (cancellation_state IN ('none','requested','acknowledged','uncertain')),
    cancellation_reason TEXT NOT NULL DEFAULT '',
    cancellation_requested_at TEXT,
    cancellation_acknowledged_at TEXT,
    terminal_outcome TEXT CHECK (terminal_outcome IS NULL OR terminal_outcome IN ('succeeded','failed','cancelled','timed_out','interrupted','cleanup_uncertain','persistence_degraded')),
    terminal_reason TEXT NOT NULL DEFAULT '',
    terminal_at TEXT,
    terminal_proposed_by TEXT NOT NULL DEFAULT '',
    CHECK ((terminal_outcome IS NULL AND finished_at IS NULL AND lifecycle IN ('accepted','queued','running','cancelling'))
        OR (terminal_outcome IS NOT NULL AND finished_at IS NOT NULL AND terminal_outcome = lifecycle)),
    FOREIGN KEY (current_attempt_id) REFERENCES attempts(attempt_id)
);

CREATE TABLE IF NOT EXISTS attempts (
    attempt_id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL REFERENCES runs(run_id) ON DELETE CASCADE,
    ordinal INTEGER NOT NULL CHECK (ordinal > 0),
    owner_id TEXT NOT NULL,
    fencing_generation INTEGER NOT NULL CHECK (fencing_generation > 0),
    host_digest TEXT NOT NULL DEFAULT '',
    boot_id TEXT NOT NULL DEFAULT '',
    pid INTEGER NOT NULL DEFAULT 0 CHECK (pid >= 0),
    process_birth_token TEXT NOT NULL DEFAULT '',
    claimed_at TEXT NOT NULL,
    heartbeat_at TEXT NOT NULL,
    lease_expires_at TEXT NOT NULL,
    correlation_json TEXT NOT NULL DEFAULT '{}',
    outcome TEXT CHECK (outcome IS NULL OR outcome IN ('succeeded','failed','cancelled','timed_out','interrupted','cleanup_uncertain','persistence_degraded')),
    UNIQUE (run_id, ordinal),
    UNIQUE (run_id, fencing_generation)
);

CREATE TABLE IF NOT EXISTS events (
    run_id TEXT NOT NULL REFERENCES runs(run_id) ON DELETE CASCADE,
    sequence INTEGER NOT NULL CHECK (sequence > 0),
    committed_at TEXT NOT NULL,
    event_type TEXT NOT NULL CHECK (event_type IN ('accepted','claimed','lifecycle','progress','message','warning','finding','artifact','cancellation','recovery','terminal','omission')),
    attempt_id TEXT,
    stage_id TEXT NOT NULL DEFAULT '',
    task_id TEXT NOT NULL DEFAULT '',
    payload_json TEXT NOT NULL DEFAULT '{}',
    omission_json TEXT,
    PRIMARY KEY (run_id, sequence),
    FOREIGN KEY (attempt_id) REFERENCES attempts(attempt_id)
);

CREATE TABLE IF NOT EXISTS operation_aliases (
    alias_id TEXT PRIMARY KEY,
    run_id TEXT REFERENCES runs(run_id) ON DELETE CASCADE,
    alias_kind TEXT NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT,
    recovery_code TEXT NOT NULL DEFAULT '',
    recovery_guidance TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS reconciliation_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id TEXT NOT NULL REFERENCES runs(run_id) ON DELETE CASCADE,
    attempt_id TEXT,
    observed_at TEXT NOT NULL,
    action TEXT NOT NULL CHECK (length(action) BETWEEN 1 AND 128),
    decision TEXT NOT NULL CHECK (length(decision) BETWEEN 1 AND 128),
    evidence_json TEXT NOT NULL DEFAULT '{}' CHECK (length(evidence_json) <= 16384),
    FOREIGN KEY (attempt_id) REFERENCES attempts(attempt_id)
);

CREATE TRIGGER IF NOT EXISTS trg_events_immutable
BEFORE UPDATE ON events
BEGIN
    SELECT RAISE(ABORT, 'run-control events are immutable');
END;

CREATE INDEX IF NOT EXISTS idx_runs_active_updated ON runs(lifecycle, updated_at DESC, run_id DESC);
CREATE INDEX IF NOT EXISTS idx_runs_target_updated ON runs(target_kind, project_id, sprint_id, study_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_runs_retention ON runs(record_state, finished_at, updated_at);
CREATE INDEX IF NOT EXISTS idx_attempts_owner_lease ON attempts(owner_id, lease_expires_at, run_id);
CREATE INDEX IF NOT EXISTS idx_attempts_run_fence ON attempts(run_id, fencing_generation DESC);
CREATE INDEX IF NOT EXISTS idx_events_replay ON events(run_id, sequence);
CREATE INDEX IF NOT EXISTS idx_events_retention ON events(committed_at, run_id, sequence);
CREATE INDEX IF NOT EXISTS idx_operation_aliases_run ON operation_aliases(run_id, alias_id);
CREATE INDEX IF NOT EXISTS idx_reconciliation_run_time ON reconciliation_log(run_id, observed_at DESC, id DESC);
`

func createInitialSchema(ctx context.Context, db *sql.DB, migratedAt time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return classifyStoreError("begin_schema", "begin run-control schema transaction failed", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, initialSchema); err != nil {
		return classifyStoreError("create_schema", "create run-control schema failed", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO app_schema (component, version, migrated_at) VALUES ('run_control', 1, ?)`, formatTime(migratedAt)); err != nil {
		return classifyStoreError("record_schema", "record run-control schema version failed", err)
	}
	if _, err := tx.ExecContext(ctx, `PRAGMA user_version = 1`); err != nil {
		return classifyStoreError("record_user_version", "record SQLite user version failed", err)
	}
	if err := tx.Commit(); err != nil {
		return classifyStoreError("commit_schema", "commit run-control schema failed", err)
	}
	return nil
}

func (r *SQLiteRepository) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	if err := r.db.Close(); err != nil {
		return classifyStoreError("close", "close run-control database failed", err)
	}
	return nil
}

func (r *SQLiteRepository) Accept(ctx context.Context, input Acceptance) (snapshotResult Snapshot, resultErr error) {
	metricStarted := time.Now()
	defer func() { r.metrics.acceptance.observe(metricStarted, resultErr) }()
	if ctx == nil {
		ctx = context.Background()
	}
	if usage, err := r.storageBytes(); err != nil {
		return Snapshot{}, err
	} else if usage >= r.retention.HardQuotaBytes*80/100 {
		if _, compactErr := r.Compact(ctx, 64); compactErr != nil {
			return Snapshot{}, compactErr
		}
	}
	if usage, err := r.storageBytes(); err != nil {
		return Snapshot{}, err
	} else if usage >= r.softQuota {
		return Snapshot{}, wrapError(CodeQuota, "accept_quota", "run-control soft quota prevents new acceptance", true, nil)
	}
	if err := input.Target.Validate(); err != nil {
		return Snapshot{}, err
	}
	if err := input.Correlation.Validate(); err != nil {
		return Snapshot{}, err
	}
	if len(input.ProductStatus) > MaxSafeValueBytes || len(input.ConfirmationDigest) > MaxSafeValueBytes {
		return Snapshot{}, invalidField("acceptance", "contains an unbounded status or confirmation digest")
	}
	runID := input.RunID
	var err error
	if runID == "" {
		runID, err = r.ids.NewRunID()
		if err != nil {
			return Snapshot{}, err
		}
	}
	if err := runID.Validate(); err != nil {
		return Snapshot{}, err
	}
	correlation, err := marshalBounded(input.Correlation, MaxSafeValueBytes*8)
	if err != nil {
		return Snapshot{}, err
	}
	now := r.now()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Snapshot{}, ctxErr
		}
		return Snapshot{}, classifyStoreError("accept_begin", "begin durable acceptance failed", err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
INSERT INTO runs (
    run_id, target_kind, operation_kind, project_id, sprint_id, study_id, stage_id, task_id,
    lifecycle, liveness, record_state, accepted_at, updated_at, correlation_json,
    product_status, confirmation_digest, cancellation_state
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(runID), input.Target.Kind, input.Target.Operation, input.Target.Project, input.Target.Sprint,
		input.Target.Study, input.Target.Stage, input.Target.Task, string(LifecycleAccepted), string(LivenessUnknown),
		string(RecordFull), formatTime(now), formatTime(now), correlation, input.ProductStatus,
		input.ConfirmationDigest, string(CancellationNone))
	if err != nil {
		if isConstraint(err) {
			return Snapshot{}, runError(CodeConflict, "accept", runID, "run identity or acceptance key already exists", false, err)
		}
		return Snapshot{}, classifyStoreError("accept", "persist durable acceptance failed", err)
	}
	if input.OperationAlias != "" {
		if len(input.OperationAlias) > MaxSafeValueBytes || strings.ContainsAny(input.OperationAlias, "\x00\r\n") {
			return Snapshot{}, invalidField("operation_alias", "must be a bounded opaque value")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO operation_aliases (alias_id, run_id, alias_kind, created_at) VALUES (?, ?, 'operation', ?)`, input.OperationAlias, string(runID), formatTime(now)); err != nil {
			if isConstraint(err) {
				return Snapshot{}, runError(CodeConflict, "accept_alias", runID, "operation alias already exists", false, err)
			}
			return Snapshot{}, classifyStoreError("accept_alias", "persist operation alias failed", err)
		}
	}
	snapshot, err := loadSnapshot(ctx, tx, runID)
	if err != nil {
		return Snapshot{}, err
	}
	if err := snapshot.Validate(); err != nil {
		return Snapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return Snapshot{}, classifyStoreError("accept_commit", "commit durable acceptance failed", err)
	}
	r.log(ctx, LogInfo, "run accepted",
		LogField{Key: "run_id", Value: string(runID)}, LogField{Key: "target_kind", Value: input.Target.Kind},
		LogField{Key: "operation", Value: input.Target.Operation}, LogField{Key: "lifecycle", Value: string(snapshot.Lifecycle)})
	return snapshot, nil
}

func (r *SQLiteRepository) ResolveOperationAlias(ctx context.Context, alias string) (Snapshot, error) {
	if strings.TrimSpace(alias) == "" || len(alias) > MaxSafeValueBytes || strings.ContainsAny(alias, "\x00\r\n") {
		return Snapshot{}, invalidField("operation_alias", "must be a bounded opaque value")
	}
	var runID RunID
	if err := r.db.QueryRowContext(ctx, `SELECT run_id FROM operation_aliases WHERE alias_id = ?`, alias).Scan(&runID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Snapshot{}, wrapError(CodeNotFound, "resolve_alias", "operation alias is not retained", false, err)
		}
		return Snapshot{}, classifyStoreError("resolve_alias", "resolve operation alias failed", err)
	}
	return r.Snapshot(ctx, runID)
}

func (r *SQLiteRepository) Claim(ctx context.Context, input Claim) (Attempt, Snapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := input.RunID.Validate(); err != nil {
		return Attempt{}, Snapshot{}, err
	}
	if err := input.Owner.Validate(); err != nil {
		return Attempt{}, Snapshot{}, err
	}
	if err := input.Correlation.Validate(); err != nil {
		return Attempt{}, Snapshot{}, err
	}
	if input.Lease <= 0 {
		return Attempt{}, Snapshot{}, invalidField("lease", "must be positive")
	}
	attemptID := input.AttemptID
	var err error
	if attemptID == "" {
		attemptID, err = r.ids.NewAttemptID()
		if err != nil {
			return Attempt{}, Snapshot{}, err
		}
	}
	if err := attemptID.Validate(); err != nil {
		return Attempt{}, Snapshot{}, err
	}
	correlation, err := marshalBounded(input.Correlation, MaxSafeValueBytes*8)
	if err != nil {
		return Attempt{}, Snapshot{}, err
	}
	now := r.now()
	leaseAt := now.Add(input.Lease)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Attempt{}, Snapshot{}, ctxErr
		}
		return Attempt{}, Snapshot{}, classifyStoreError("claim_begin", "begin owner claim failed", err)
	}
	defer tx.Rollback()
	var lifecycle string
	var currentAttempt sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT lifecycle, current_attempt_id FROM runs WHERE run_id = ?`, string(input.RunID)).Scan(&lifecycle, &currentAttempt); err != nil {
		return Attempt{}, Snapshot{}, mapRunLookupError("claim", input.RunID, err)
	}
	state := Lifecycle(lifecycle)
	if state.IsTerminal() {
		return Attempt{}, Snapshot{}, runError(CodeTerminal, "claim", input.RunID, "terminal run cannot be claimed", false, nil)
	}
	if currentAttempt.Valid && currentAttempt.String != "" {
		return Attempt{}, Snapshot{}, runError(CodeConflict, "claim", input.RunID, "run already has a current owner attempt", false, nil)
	}
	nextLifecycle := LifecycleRunning
	if state == LifecycleCancelling {
		nextLifecycle = LifecycleCancelling
	}
	var ordinal, generation uint64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(ordinal), 0) + 1, COALESCE(MAX(fencing_generation), 0) + 1 FROM attempts WHERE run_id = ?`, string(input.RunID)).Scan(&ordinal, &generation); err != nil {
		return Attempt{}, Snapshot{}, classifyStoreError("claim_allocate", "allocate owner fence failed", err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO attempts (
    attempt_id, run_id, ordinal, owner_id, fencing_generation, host_digest, boot_id, pid,
    process_birth_token, claimed_at, heartbeat_at, lease_expires_at, correlation_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(attemptID), string(input.RunID), ordinal, input.Owner.ID, generation,
		input.Owner.Process.HostDigest, input.Owner.Process.BootID, input.Owner.Process.PID,
		input.Owner.Process.BirthToken, formatTime(now), formatTime(now), formatTime(leaseAt), correlation)
	if err != nil {
		if isConstraint(err) {
			return Attempt{}, Snapshot{}, runError(CodeConflict, "claim", input.RunID, "attempt identity or fence already exists", false, err)
		}
		return Attempt{}, Snapshot{}, classifyStoreError("claim", "persist owner claim failed", err)
	}
	result, err := tx.ExecContext(ctx, `
UPDATE runs
SET current_attempt_id = ?, lifecycle = ?, liveness = ?, started_at = COALESCE(started_at, ?), updated_at = ?
WHERE run_id = ? AND current_attempt_id IS NULL AND terminal_outcome IS NULL`,
		string(attemptID), string(nextLifecycle), string(LivenessLive), formatTime(now), formatTime(now), string(input.RunID))
	if err != nil {
		return Attempt{}, Snapshot{}, classifyStoreError("claim_snapshot", "update run owner snapshot failed", err)
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return Attempt{}, Snapshot{}, runError(CodeConflict, "claim", input.RunID, "run owner changed while claiming", true, err)
	}
	snapshot, err := loadSnapshot(ctx, tx, input.RunID)
	if err != nil {
		return Attempt{}, Snapshot{}, err
	}
	if err := snapshot.Validate(); err != nil {
		return Attempt{}, Snapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return Attempt{}, Snapshot{}, classifyStoreError("claim_commit", "commit owner claim failed", err)
	}
	attempt := Attempt{
		ID: attemptID, RunID: input.RunID, Ordinal: ordinal, Owner: input.Owner,
		FencingGeneration: generation, ClaimedAt: now, HeartbeatAt: now,
		LeaseExpiresAt: leaseAt, Correlation: input.Correlation,
	}
	r.log(ctx, LogInfo, "run owner claimed",
		LogField{Key: "run_id", Value: string(input.RunID)}, LogField{Key: "attempt_id", Value: string(attemptID)},
		LogField{Key: "owner_id", Value: input.Owner.ID}, LogField{Key: "fencing_generation", Value: strconv.FormatUint(generation, 10)},
		LogField{Key: "lifecycle", Value: string(snapshot.Lifecycle)})
	return attempt, snapshot, nil
}

func (r *SQLiteRepository) Snapshot(ctx context.Context, runID RunID) (Snapshot, error) {
	if err := runID.Validate(); err != nil {
		return Snapshot{}, err
	}
	snapshot, err := loadSnapshot(ctx, r.db, runID)
	if err != nil {
		return Snapshot{}, err
	}
	if err := snapshot.Validate(); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func (r *SQLiteRepository) Append(ctx context.Context, fence Fence, draft EventDraft) (eventResult Event, snapshotResult Snapshot, resultErr error) {
	metricStarted := time.Now()
	defer func() { r.metrics.append.observe(metricStarted, resultErr) }()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := fence.Validate(); err != nil {
		return Event{}, Snapshot{}, err
	}
	draft = sanitizeEventDraft(NormalizeEventDraft(draft))
	if usage, err := r.storageBytes(); err != nil {
		return Event{}, Snapshot{}, err
	} else if usage >= r.retention.HardQuotaBytes && !reservedEventType(draft.Type) {
		return Event{}, Snapshot{}, runError(CodeQuota, "append_quota", fence.RunID, "hard quota permits only reserved lifecycle recovery writes", true, nil)
	}
	if err := validateEventDraft(draft); err != nil {
		return Event{}, Snapshot{}, err
	}
	payload, err := marshalBounded(draft.Payload, MaxSafeValueBytes*8)
	if err != nil {
		return Event{}, Snapshot{}, err
	}
	var omissionJSON any
	if draft.Omission != nil {
		encoded, err := marshalBounded(draft.Omission, MaxSafeValueBytes*2)
		if err != nil {
			return Event{}, Snapshot{}, err
		}
		omissionJSON = encoded
	}
	now := r.now()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Event{}, Snapshot{}, ctxErr
		}
		return Event{}, Snapshot{}, classifyStoreError("append_begin", "begin event append failed", err)
	}
	defer tx.Rollback()
	if err := verifyFence(ctx, tx, fence); err != nil {
		return Event{}, Snapshot{}, err
	}
	var lastSequence uint64
	var lifecycle string
	if err := tx.QueryRowContext(ctx, `SELECT last_sequence, lifecycle FROM runs WHERE run_id = ?`, string(fence.RunID)).Scan(&lastSequence, &lifecycle); err != nil {
		return Event{}, Snapshot{}, mapRunLookupError("append", fence.RunID, err)
	}
	current := Lifecycle(lifecycle)
	if current.IsTerminal() {
		return Event{}, Snapshot{}, runError(CodeTerminal, "append", fence.RunID, "terminal run journal is immutable", false, nil)
	}
	nextLifecycle := current
	if draft.Lifecycle != "" {
		if !validActiveTransition(current, draft.Lifecycle) {
			return Event{}, Snapshot{}, runError(CodeInvariant, "append", fence.RunID, fmt.Sprintf("invalid lifecycle transition %s -> %s", current, draft.Lifecycle), false, nil)
		}
		nextLifecycle = draft.Lifecycle
	}
	sequence := lastSequence + 1
	if _, err := tx.ExecContext(ctx, `
INSERT INTO events (run_id, sequence, committed_at, event_type, attempt_id, stage_id, task_id, payload_json, omission_json)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, string(fence.RunID), sequence, formatTime(now), string(draft.Type),
		string(fence.AttemptID), draft.Stage, draft.Task, payload, omissionJSON); err != nil {
		return Event{}, Snapshot{}, classifyStoreError("append_event", "persist ordered event failed", err)
	}
	omissionCount := uint64(0)
	if draft.Omission != nil {
		omissionCount = draft.Omission.Count
	}
	result, err := tx.ExecContext(ctx, `
UPDATE runs SET last_sequence = ?, lifecycle = ?, omission_total = omission_total + ?, updated_at = ?
WHERE run_id = ? AND last_sequence = ? AND current_attempt_id = ? AND terminal_outcome IS NULL`,
		sequence, string(nextLifecycle), omissionCount, formatTime(now), string(fence.RunID), lastSequence, string(fence.AttemptID))
	if err != nil {
		return Event{}, Snapshot{}, classifyStoreError("append_snapshot", "update durable event snapshot failed", err)
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return Event{}, Snapshot{}, runError(CodeStaleFence, "append", fence.RunID, "event writer lost its authoritative fence", false, err)
	}
	if err := compactRunJournal(ctx, tx, fence.RunID, sequence); err != nil {
		return Event{}, Snapshot{}, err
	}
	snapshot, err := loadSnapshot(ctx, tx, fence.RunID)
	if err != nil {
		return Event{}, Snapshot{}, err
	}
	if err := snapshot.Validate(); err != nil {
		return Event{}, Snapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return Event{}, Snapshot{}, classifyStoreError("append_commit", "commit ordered event failed", err)
	}
	event := Event{
		RunID: fence.RunID, Sequence: sequence, CommittedAt: now, Type: draft.Type,
		AttemptID: fence.AttemptID, Stage: draft.Stage, Task: draft.Task,
		Payload: clonePayload(draft.Payload), Omission: cloneOmission(draft.Omission),
	}
	r.notify(fence.RunID, sequence)
	r.log(ctx, LogDebug, "run event committed",
		LogField{Key: "run_id", Value: string(fence.RunID)}, LogField{Key: "attempt_id", Value: string(fence.AttemptID)},
		LogField{Key: "sequence", Value: strconv.FormatUint(sequence, 10)}, LogField{Key: "event_type", Value: string(draft.Type)},
		LogField{Key: "lifecycle", Value: string(snapshot.Lifecycle)})
	return event, snapshot, nil
}

func reservedEventType(eventType EventType) bool {
	switch eventType {
	case EventLifecycle, EventWarning, EventCancellation, EventRecovery, EventTerminal, EventOmission:
		return true
	default:
		return false
	}
}

func (r *SQLiteRepository) ProposeTerminal(ctx context.Context, fence Fence, proposal TerminalProposal) (snapshotResult Snapshot, wonResult bool, resultErr error) {
	metricStarted := time.Now()
	defer func() { r.metrics.terminal.observe(metricStarted, resultErr) }()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := fence.Validate(); err != nil {
		return Snapshot{}, false, err
	}
	if !proposal.Outcome.IsValid() {
		return Snapshot{}, false, invalidField("terminal.outcome", "is unknown")
	}
	if len(proposal.Reason) > MaxSafeValueBytes || len(proposal.ProposedBy) > MaxSafeValueBytes {
		return Snapshot{}, false, invalidField("terminal", "contains an unbounded reason or proposer")
	}
	now := r.now()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Snapshot{}, false, ctxErr
		}
		return Snapshot{}, false, classifyStoreError("terminal_begin", "begin terminal proposal failed", err)
	}
	defer tx.Rollback()
	if err := verifyFence(ctx, tx, fence); err != nil {
		return Snapshot{}, false, err
	}
	current, err := loadSnapshot(ctx, tx, fence.RunID)
	if err != nil {
		return Snapshot{}, false, err
	}
	if current.Terminal != nil {
		if err := tx.Commit(); err != nil {
			return Snapshot{}, false, classifyStoreError("terminal_read_commit", "finish terminal winner read failed", err)
		}
		return current, false, nil
	}
	sequence := current.LastSequence + 1
	payload, err := marshalBounded(map[string]string{"outcome": string(proposal.Outcome), "reason": proposal.Reason}, MaxSafeValueBytes*2)
	if err != nil {
		return Snapshot{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO events (run_id, sequence, committed_at, event_type, attempt_id, payload_json)
VALUES (?, ?, ?, 'terminal', ?, ?)`, string(fence.RunID), sequence, formatTime(now), string(fence.AttemptID), payload); err != nil {
		return Snapshot{}, false, classifyStoreError("terminal_event", "persist terminal event failed", err)
	}
	result, err := tx.ExecContext(ctx, `
UPDATE runs
SET lifecycle = ?, liveness = ?, finished_at = ?, updated_at = ?, last_sequence = ?,
    terminal_outcome = ?, terminal_reason = ?, terminal_at = ?, terminal_proposed_by = ?
WHERE run_id = ? AND current_attempt_id = ? AND terminal_outcome IS NULL`,
		string(proposal.Outcome.Lifecycle()), string(LivenessTerminal), formatTime(now), formatTime(now), sequence,
		string(proposal.Outcome), proposal.Reason, formatTime(now), proposal.ProposedBy,
		string(fence.RunID), string(fence.AttemptID))
	if err != nil {
		return Snapshot{}, false, classifyStoreError("terminal_cas", "persist terminal winner failed", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return Snapshot{}, false, classifyStoreError("terminal_cas", "inspect terminal winner failed", err)
	}
	if changed != 1 {
		return Snapshot{}, false, runError(CodeStaleFence, "terminal_cas", fence.RunID, "terminal proposer lost its authoritative fence", false, nil)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE attempts SET outcome = ? WHERE attempt_id = ? AND run_id = ? AND owner_id = ? AND fencing_generation = ?`,
		string(proposal.Outcome), string(fence.AttemptID), string(fence.RunID), fence.OwnerID, fence.FencingGeneration); err != nil {
		return Snapshot{}, false, classifyStoreError("terminal_attempt", "persist attempt outcome failed", err)
	}
	winner, err := loadSnapshot(ctx, tx, fence.RunID)
	if err != nil {
		return Snapshot{}, false, err
	}
	if err := winner.Validate(); err != nil {
		return Snapshot{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return Snapshot{}, false, classifyStoreError("terminal_commit", "commit terminal winner failed", err)
	}
	r.notify(fence.RunID, sequence)
	r.log(ctx, LogInfo, "run terminal committed",
		LogField{Key: "run_id", Value: string(fence.RunID)}, LogField{Key: "attempt_id", Value: string(fence.AttemptID)},
		LogField{Key: "sequence", Value: strconv.FormatUint(sequence, 10)}, LogField{Key: "terminal_outcome", Value: string(proposal.Outcome)},
		LogField{Key: "terminal_winner", Value: "true"})
	return winner, true, nil
}

func (r *SQLiteRepository) Events(ctx context.Context, runID RunID, after uint64, limit int) ([]Event, error) {
	if err := runID.Validate(); err != nil {
		return nil, err
	}
	if limit == 0 {
		limit = defaultEventPage
	}
	if limit < 1 || limit > maxEventPage {
		return nil, invalidField("event_limit", fmt.Sprintf("must be between 1 and %d", maxEventPage))
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT sequence, committed_at, event_type, attempt_id, stage_id, task_id, payload_json, omission_json
FROM events WHERE run_id = ? AND sequence > ? ORDER BY sequence ASC LIMIT ?`, string(runID), after, limit)
	if err != nil {
		return nil, classifyStoreError("events", "read retained events failed", err)
	}
	defer rows.Close()
	events := make([]Event, 0, limit)
	for rows.Next() {
		var sequence uint64
		var committed, eventType, stage, task, payload string
		var attempt, omission sql.NullString
		if err := rows.Scan(&sequence, &committed, &eventType, &attempt, &stage, &task, &payload, &omission); err != nil {
			return nil, classifyStoreError("events_scan", "decode retained event failed", err)
		}
		committedAt, err := parseTime(committed)
		if err != nil {
			return nil, err
		}
		event := Event{RunID: runID, Sequence: sequence, CommittedAt: committedAt, Type: EventType(eventType), AttemptID: AttemptID(attempt.String), Stage: stage, Task: task}
		if err := json.Unmarshal([]byte(payload), &event.Payload); err != nil {
			return nil, wrapError(CodeCorrupt, "events_decode", "stored event payload is malformed", false, err)
		}
		if omission.Valid {
			event.Omission = &Omission{}
			if err := json.Unmarshal([]byte(omission.String), event.Omission); err != nil {
				return nil, wrapError(CodeCorrupt, "events_decode", "stored omission metadata is malformed", false, err)
			}
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, classifyStoreError("events_rows", "read retained events failed", err)
	}
	return events, nil
}

func (r *SQLiteRepository) Health(ctx context.Context) (Health, error) {
	health := Health{Status: HealthOK, DatabasePath: DatabaseRelativePath, BusyTimeout: BusyTimeout, SoftQuotaBytes: r.softQuota, HardQuotaBytes: r.retention.HardQuotaBytes, ReservedHeadroomBytes: ReservedHeadroomBytes}
	health.Metrics = r.metrics.snapshot()
	if err := r.db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&health.JournalMode); err != nil {
		return Health{}, classifyStoreError("health", "read journal mode failed", err)
	}
	var synchronous, foreignKeys int
	if err := r.db.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&synchronous); err != nil {
		return Health{}, classifyStoreError("health", "read synchronous mode failed", err)
	}
	health.Synchronous = strconv.Itoa(synchronous)
	if err := r.db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		return Health{}, classifyStoreError("health", "read foreign-key policy failed", err)
	}
	health.ForeignKeys = foreignKeys == 1
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM runs WHERE lifecycle IN ('accepted','queued','running','cancelling')`).Scan(&health.ActiveRuns); err != nil {
		return Health{}, classifyStoreError("health", "read active-run projection failed", err)
	}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM runs WHERE terminal_outcome IS NULL AND liveness = 'stalled'`).Scan(&health.StalledRuns); err != nil {
		return Health{}, classifyStoreError("health", "read stalled-run projection failed", err)
	}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM runs WHERE cancellation_state = 'uncertain'`).Scan(&health.CancellationUncertain); err != nil {
		return Health{}, classifyStoreError("health", "read cancellation-uncertainty projection failed", err)
	}
	acceptedPredicate, acceptedArgs := r.expiredTimestampPredicate("runs.accepted_at", ReconciliationGrace)
	leasePredicate, leaseArgs := r.expiredLeasePredicate(ReconciliationGrace)
	backlogArgs := append(acceptedArgs, leaseArgs...)
	backlogWhere := `runs.terminal_outcome IS NULL AND ((runs.current_attempt_id IS NULL AND ` + acceptedPredicate + `)
OR (runs.current_attempt_id IS NOT NULL AND ` + leasePredicate + `))`
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM runs LEFT JOIN attempts ON attempts.attempt_id = runs.current_attempt_id
WHERE `+backlogWhere, backlogArgs...).Scan(&health.ReconciliationBacklog); err != nil {
		return Health{}, classifyStoreError("health", "read reconciliation backlog failed", err)
	}
	var oldestLease sql.NullString
	if err := r.db.QueryRowContext(ctx, `SELECT MIN(CASE WHEN runs.current_attempt_id IS NULL THEN runs.accepted_at ELSE attempts.lease_expires_at END)
FROM runs LEFT JOIN attempts ON attempts.attempt_id = runs.current_attempt_id WHERE `+backlogWhere, backlogArgs...).Scan(&oldestLease); err != nil {
		return Health{}, classifyStoreError("health", "read reconciliation backlog age failed", err)
	}
	if oldestLease.Valid {
		oldest, err := parseTime(oldestLease.String)
		if err != nil {
			return Health{}, err
		}
		health.OldestBacklogAge = max(0, r.now().Sub(oldest))
	}
	storageBytes, err := r.storageBytes()
	if err != nil {
		return Health{}, err
	}
	health.StorageBytes = storageBytes
	if health.StorageBytes >= health.HardQuotaBytes {
		health.Status = HealthFailed
		health.Diagnostics = append(health.Diagnostics, "hard quota reached; active owners must stop")
	} else if health.StorageBytes >= health.SoftQuotaBytes {
		health.Status = HealthDegraded
		health.Diagnostics = append(health.Diagnostics, "soft quota reached; new run acceptance is paused")
	}
	return health, nil
}

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func loadSnapshot(ctx context.Context, q queryRower, runID RunID) (Snapshot, error) {
	var snapshot Snapshot
	var lifecycle, liveness, recordState, accepted, updated string
	var started, finished, currentAttempt sql.NullString
	var correlationJSON string
	var historyComplete int
	var cancellationState, cancellationReason string
	var cancellationRequested, cancellationAcknowledged sql.NullString
	var terminalOutcome, terminalAt sql.NullString
	var terminalReason, terminalProposedBy string
	err := q.QueryRowContext(ctx, `
SELECT run_id, target_kind, operation_kind, project_id, sprint_id, study_id, stage_id, task_id,
       lifecycle, liveness, record_state, accepted_at, updated_at, started_at, finished_at,
       current_attempt_id, last_sequence, oldest_retained_sequence, history_complete, omission_total,
       correlation_json, product_status, cancellation_state, cancellation_reason,
       cancellation_requested_at, cancellation_acknowledged_at,
       terminal_outcome, terminal_reason, terminal_at, terminal_proposed_by
FROM runs WHERE run_id = ?`, string(runID)).Scan(
		&snapshot.RunID, &snapshot.Target.Kind, &snapshot.Target.Operation, &snapshot.Target.Project,
		&snapshot.Target.Sprint, &snapshot.Target.Study, &snapshot.Target.Stage, &snapshot.Target.Task,
		&lifecycle, &liveness, &recordState, &accepted, &updated, &started, &finished,
		&currentAttempt, &snapshot.LastSequence, &snapshot.OldestRetainedSequence, &historyComplete,
		&snapshot.OmissionTotal, &correlationJSON, &snapshot.ProductStatus, &cancellationState,
		&cancellationReason, &cancellationRequested, &cancellationAcknowledged,
		&terminalOutcome, &terminalReason, &terminalAt, &terminalProposedBy)
	if err != nil {
		return Snapshot{}, mapRunLookupError("snapshot", runID, err)
	}
	snapshot.Lifecycle = Lifecycle(lifecycle)
	snapshot.Liveness = Liveness(liveness)
	snapshot.RecordState = RecordState(recordState)
	snapshot.CurrentAttemptID = AttemptID(currentAttempt.String)
	snapshot.HistoryComplete = historyComplete == 1
	if snapshot.AcceptedAt, err = parseTime(accepted); err != nil {
		return Snapshot{}, err
	}
	if snapshot.UpdatedAt, err = parseTime(updated); err != nil {
		return Snapshot{}, err
	}
	if snapshot.StartedAt, err = parseOptionalTime(started); err != nil {
		return Snapshot{}, err
	}
	if snapshot.FinishedAt, err = parseOptionalTime(finished); err != nil {
		return Snapshot{}, err
	}
	if err := json.Unmarshal([]byte(correlationJSON), &snapshot.Correlation); err != nil {
		return Snapshot{}, wrapError(CodeCorrupt, "snapshot_decode", "stored correlation is malformed", false, err)
	}
	snapshot.Cancellation = Cancellation{State: CancellationState(cancellationState), Reason: cancellationReason}
	if snapshot.Cancellation.RequestedAt, err = parseOptionalTime(cancellationRequested); err != nil {
		return Snapshot{}, err
	}
	if snapshot.Cancellation.AcknowledgedAt, err = parseOptionalTime(cancellationAcknowledged); err != nil {
		return Snapshot{}, err
	}
	if terminalOutcome.Valid {
		wonAt, err := parseTime(terminalAt.String)
		if err != nil {
			return Snapshot{}, err
		}
		snapshot.Terminal = &Terminal{Outcome: TerminalOutcome(terminalOutcome.String), Reason: terminalReason, WonAt: wonAt, ProposedBy: terminalProposedBy}
	}
	return snapshot, nil
}

func verifyFence(ctx context.Context, q queryRower, fence Fence) error {
	var ownerID string
	var generation uint64
	var currentAttempt sql.NullString
	err := q.QueryRowContext(ctx, `
SELECT attempts.owner_id, attempts.fencing_generation, runs.current_attempt_id
FROM attempts JOIN runs ON runs.run_id = attempts.run_id
WHERE attempts.run_id = ? AND attempts.attempt_id = ?`, string(fence.RunID), string(fence.AttemptID)).Scan(&ownerID, &generation, &currentAttempt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return runError(CodeStaleFence, "verify_fence", fence.RunID, "owner attempt is not authoritative", false, err)
		}
		return classifyStoreError("verify_fence", "verify owner fence failed", err)
	}
	if ownerID != fence.OwnerID || generation != fence.FencingGeneration || !currentAttempt.Valid || currentAttempt.String != string(fence.AttemptID) {
		return runError(CodeStaleFence, "verify_fence", fence.RunID, "owner attempt is not authoritative", false, nil)
	}
	return nil
}

func validateEventDraft(draft EventDraft) error {
	if !draft.Type.IsValid() {
		return invalidField("event.type", "is unknown")
	}
	if !draft.Scope.IsValid() {
		return invalidField("event.scope", "is unknown")
	}
	if draft.Lifecycle != "" && (!draft.Lifecycle.IsValid() || draft.Lifecycle.IsTerminal()) {
		return invalidField("event.lifecycle", "must be an active lifecycle; terminal state uses terminal arbitration")
	}
	for name, value := range map[string]string{"stage": draft.Stage, "task": draft.Task} {
		if len(value) > MaxTargetFieldBytes || strings.ContainsAny(value, "\x00\r\n") {
			return invalidField("event."+name, "must be a bounded safe value")
		}
	}
	for key, value := range draft.Payload {
		if key == "" || len(key) > 128 || strings.ContainsAny(key, "\x00\r\n") || len(value) > MaxSafeValueBytes || strings.ContainsRune(value, '\x00') {
			return invalidField("event.payload", "must contain bounded safe string fields")
		}
	}
	if draft.Omission != nil && (draft.Omission.Count == 0 || len(draft.Omission.Reason) > MaxSafeValueBytes) {
		return invalidField("event.omission", "requires a bounded reason and positive count")
	}
	return nil
}

func validActiveTransition(from, to Lifecycle) bool {
	if from == to {
		return true
	}
	switch from {
	case LifecycleAccepted:
		return to == LifecycleQueued || to == LifecycleRunning || to == LifecycleCancelling
	case LifecycleQueued:
		return to == LifecycleRunning || to == LifecycleCancelling
	case LifecycleRunning:
		return to == LifecycleCancelling
	case LifecycleCancelling:
		return false
	default:
		return false
	}
}

func formatTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, wrapError(CodeCorrupt, "decode_time", "stored timestamp is malformed", false, err)
	}
	return parsed.UTC(), nil
}

func parseOptionalTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid || value.String == "" {
		return nil, nil
	}
	parsed, err := parseTime(value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func marshalBounded(value any, limit int) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", wrapError(CodeInvalidArgument, "encode", "run-control value cannot be encoded", false, err)
	}
	if len(encoded) > limit {
		return "", invalidField("encoded_value", fmt.Sprintf("exceeds %d bytes", limit))
	}
	return string(encoded), nil
}

func mapRunLookupError(operation string, runID RunID, err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return runError(CodeNotFound, operation, runID, "run is not retained", false, err)
	}
	return classifyStoreError(operation, "read run-control record failed", err)
}

func classifyStoreError(operation, message string, err error) error {
	type sqliteCoder interface{ Code() int }
	var coded sqliteCoder
	if errors.As(err, &coded) {
		switch coded.Code() & 0xff {
		case 5, 6: // SQLITE_BUSY, SQLITE_LOCKED
			return wrapError(CodeBusy, operation, message, true, err)
		case 3, 8: // SQLITE_PERM, SQLITE_READONLY
			return wrapError(CodePermission, operation, message, false, err)
		case 11, 26: // SQLITE_CORRUPT, SQLITE_NOTADB
			return wrapError(CodeCorrupt, operation, message, false, err)
		case 13: // SQLITE_FULL
			return wrapError(CodeQuota, operation, message, true, err)
		}
	}
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, fs.ErrPermission):
		return wrapError(CodePermission, operation, message, false, err)
	default:
		return wrapError(CodeUnavailable, operation, message, true, err)
	}
}

func isConstraint(err error) bool {
	// SQLite exposes constraint codes through driver-specific errors. Keeping
	// this narrow check local prevents callers from parsing messages.
	type coder interface{ Code() int }
	var coded coder
	if errors.As(err, &coded) {
		return coded.Code()&0xff == 19 // SQLITE_CONSTRAINT and extended variants.
	}
	return false
}

func (r *SQLiteRepository) now() time.Time { return r.clock.Now().UTC() }

func (r *SQLiteRepository) SetLogger(logger Logger) {
	r.loggerMu.Lock()
	defer r.loggerMu.Unlock()
	r.logger = logger
}

// expiredLeasePredicate uses SQLite's connection-local clock in production so
// competing processes compare leases against one time authority. Injected
// clocks retain deterministic test control.
func (r *SQLiteRepository) expiredLeasePredicate(grace time.Duration) (string, []any) {
	return r.expiredTimestampPredicate("attempts.lease_expires_at", grace)
}

func (r *SQLiteRepository) expiredTimestampPredicate(column string, grace time.Duration) (string, []any) {
	if r.clockInjected {
		return column + " <= ?", []any{formatTime(r.now().Add(-grace))}
	}
	return "julianday(" + column + ") <= julianday('now', ?)", []any{fmt.Sprintf("-%f seconds", grace.Seconds())}
}

func (r *SQLiteRepository) notify(runID RunID, sequence uint64) {
	if r.notifier != nil {
		r.notifier.Notify(runID, sequence)
	}
}

func (r *SQLiteRepository) log(ctx context.Context, level LogLevel, message string, fields ...LogField) {
	r.loggerMu.RLock()
	logger := r.logger
	r.loggerMu.RUnlock()
	if logger != nil {
		logger.Log(ctx, level, message, fields...)
	}
}

func clonePayload(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func cloneOmission(input *Omission) *Omission {
	if input == nil {
		return nil
	}
	copy := *input
	return &copy
}

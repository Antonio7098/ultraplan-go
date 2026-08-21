package runcontrol

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMigrationCreatesVersionRecordsBackupAndIntegrity(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	directory := filepath.Join(root, ".ultraplan")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, DatabaseRelativePath)
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`CREATE TABLE legacy_marker (value TEXT); INSERT INTO legacy_marker VALUES ('before-migration')`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	repository := openTestRepository(t, root)
	defer repository.Close()
	var userVersion, appVersion int
	if err := repository.db.QueryRow(`PRAGMA user_version`).Scan(&userVersion); err != nil {
		t.Fatal(err)
	}
	if err := repository.db.QueryRow(`SELECT version FROM app_schema WHERE component='run_control'`).Scan(&appVersion); err != nil {
		t.Fatal(err)
	}
	if userVersion != CurrentSchemaVersion || appVersion != CurrentSchemaVersion {
		t.Fatalf("schema versions user=%d app=%d", userVersion, appVersion)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	backups := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "run-control.db.bak.") {
			backups++
			info, err := entry.Info()
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o600 {
				t.Fatalf("backup mode = %04o", info.Mode().Perm())
			}
		}
	}
	if backups != 1 {
		t.Fatalf("migration backups = %d, want 1", backups)
	}
}

func TestMigrationRejectsNewerSchemaAndConcurrentLock(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repository := openTestRepository(t, root)
	if _, err := repository.db.Exec(`PRAGMA user_version = 99`); err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenSQLite(context.Background(), root, SQLiteOptions{}); !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("newer schema error = %v, want unsupported schema", err)
	}

	root = t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".ultraplan"), 0o700); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(root, DatabaseRelativePath) + ".migrate.lock"
	if err := os.WriteFile(lockPath, []byte("owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenSQLite(context.Background(), root, SQLiteOptions{}); !errors.Is(err, ErrBusy) {
		t.Fatalf("migration lock error = %v, want busy", err)
	}
}

func TestMigrationReclaimsExactlyProvenStaleLock(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".ultraplan"), 0o700); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(root, DatabaseRelativePath) + ".migrate.lock"
	stale := ProcessIdentity{HostDigest: localHostDigest(), BootID: "stale-boot", PID: 1 << 30, BirthToken: "stale-birth"}
	encoded, err := json.Marshal(stale)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := OpenSQLite(context.Background(), root, SQLiteOptions{})
	if err != nil {
		t.Fatalf("reclaim stale migration lock: %v", err)
	}
	defer repository.Close()
	if _, err := os.Stat(lockPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale lock remains: %v", err)
	}
}

func TestBackupRestoreFixture(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repository := openTestRepository(t, root)
	runID, _ := acceptedClaimedRun(t, repository)
	if err := checkpointWAL(context.Background(), repository.db); err != nil {
		t.Fatal(err)
	}
	backup, err := createMigrationBackup(repository.path, time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Accept(context.Background(), Acceptance{RunID: mustRunID(t), Target: Target{Kind: "study", Operation: "study.run", Study: "later"}}); err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}
	if err := RestoreBackup(context.Background(), root, filepath.Base(backup)); err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}
	restored := openTestRepository(t, root)
	defer restored.Close()
	if _, err := restored.Snapshot(context.Background(), runID); err != nil {
		t.Fatalf("restored original run: %v", err)
	}
	var count int
	if err := restored.db.QueryRow(`SELECT COUNT(*) FROM runs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("restored run count = %d, want 1", count)
	}
}

func TestMigrationRejectsCorruptDatabaseWithoutReplacingEvidence(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	directory := filepath.Join(root, ".ultraplan")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, DatabaseRelativePath)
	original := []byte("not a sqlite database; retain this evidence")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenSQLite(context.Background(), root, SQLiteOptions{}); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("corrupt open error = %v, want corrupt", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatalf("corrupt evidence was rewritten: %q", after)
	}
}

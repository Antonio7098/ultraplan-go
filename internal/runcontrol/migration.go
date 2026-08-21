package runcontrol

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	CurrentSchemaVersion       = 1
	maxMigrationBackups        = 3
	maxBackupBytes       int64 = 512 << 20
)

func migrateSchema(ctx context.Context, db *sql.DB, databasePath string, clock Clock) error {
	version, err := schemaVersion(ctx, db)
	if err != nil {
		return err
	}
	if version > CurrentSchemaVersion {
		return wrapError(CodeUnsupportedSchema, "schema_version", fmt.Sprintf("database schema %d is newer than supported schema %d", version, CurrentSchemaVersion), false, nil)
	}
	// Opening an already-current database is the common multi-process path. It
	// must not contend on the migration lock: that lock protects schema writes,
	// not ordinary repository startup.
	if version == CurrentSchemaVersion {
		if err := verifySchemaRecord(ctx, db); err != nil {
			return err
		}
		return integrityCheck(ctx, db)
	}

	release, err := acquireMigrationLock(databasePath)
	if err != nil {
		return err
	}
	defer release()

	// Another process may have completed the migration between the optimistic
	// read above and lock acquisition.
	version, err = schemaVersion(ctx, db)
	if err != nil {
		return err
	}
	if version > CurrentSchemaVersion {
		return wrapError(CodeUnsupportedSchema, "schema_version", fmt.Sprintf("database schema %d is newer than supported schema %d", version, CurrentSchemaVersion), false, nil)
	}
	if version == 0 {
		hasSchema, err := hasApplicationSchema(ctx, db)
		if err != nil {
			return err
		}
		if hasSchema {
			if err := checkpointWAL(ctx, db); err != nil {
				return err
			}
			if _, err := createMigrationBackup(databasePath, clock.Now()); err != nil {
				return err
			}
		}
		if err := createInitialSchema(ctx, db, clock.Now()); err != nil {
			return err
		}
	}
	if err := verifySchemaRecord(ctx, db); err != nil {
		return err
	}
	if err := integrityCheck(ctx, db); err != nil {
		return err
	}
	return pruneMigrationBackups(databasePath)
}

func schemaVersion(ctx context.Context, db *sql.DB) (int, error) {
	var version int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return 0, classifyStoreError("schema_version", "read run-control schema version failed", err)
	}
	return version, nil
}

func acquireMigrationLock(databasePath string) (func(), error) {
	path := databasePath + ".migrate.lock"
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			removed, inspectErr := removeStaleMigrationLock(path)
			if inspectErr != nil {
				return nil, inspectErr
			}
			if removed {
				file, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			}
			if !removed || err != nil {
				return nil, wrapError(CodeBusy, "migration_lock", "another local UltraPlan process owns the schema migration lock", true, err)
			}
		} else {
			return nil, classifyStoreError("migration_lock", "acquire schema migration lock failed", err)
		}
	}
	identity, probeErr := probeNativeProcess(context.Background(), os.Getpid())
	if probeErr != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, wrapError(CodeUnavailable, "migration_lock", "exact migration owner identity is unavailable", true, probeErr)
	}
	encoded, err := json.Marshal(identity)
	if err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, classifyStoreError("migration_lock", "encode schema migration owner failed", err)
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, classifyStoreError("migration_lock", "record schema migration lock failed", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, classifyStoreError("migration_lock", "flush schema migration lock failed", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return nil, classifyStoreError("migration_lock", "close schema migration lock failed", err)
	}
	return func() { _ = os.Remove(path) }, nil
}

func removeStaleMigrationLock(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, classifyStoreError("migration_lock", "inspect schema migration owner failed", err)
	}
	var expected ProcessIdentity
	if err := json.Unmarshal(data, &expected); err != nil || expected.Validate() != nil {
		// A legacy or malformed record cannot authorize lock removal.
		return false, nil
	}
	observed, err := probeNativeProcess(context.Background(), expected.PID)
	switch {
	case errors.Is(err, ErrProcessNotFound):
		// Exact absence proves this owner is gone.
	case err != nil:
		return false, nil
	case observed == expected:
		return false, nil
	default:
		// Exact birth mismatch proves PID reuse.
	}
	if err := os.Remove(path); err != nil {
		return false, classifyStoreError("migration_lock", "remove stale schema migration lock failed", err)
	}
	return true, nil
}

func hasApplicationSchema(ctx context.Context, db *sql.DB) (bool, error) {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_schema WHERE type IN ('table','index','trigger') AND name NOT LIKE 'sqlite_%'`).Scan(&count); err != nil {
		return false, classifyStoreError("inspect_schema", "inspect pre-migration schema failed", err)
	}
	return count > 0, nil
}

func checkpointWAL(ctx context.Context, db *sql.DB) error {
	var busy, logFrames, checkpointed int
	if err := db.QueryRowContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &logFrames, &checkpointed); err != nil {
		return classifyStoreError("migration_checkpoint", "checkpoint WAL before migration failed", err)
	}
	if busy != 0 {
		return wrapError(CodeBusy, "migration_checkpoint", "WAL checkpoint is busy; stop other UltraPlan processes before migration", true, nil)
	}
	return nil
}

func createMigrationBackup(databasePath string, now time.Time) (string, error) {
	info, err := os.Stat(databasePath)
	if err != nil {
		return "", classifyStoreError("backup_stat", "inspect database before backup failed", err)
	}
	if info.Size() > maxBackupBytes {
		return "", wrapError(CodeUnavailable, "backup_size", "database exceeds the bounded migration backup limit", false, nil)
	}
	stamp := now.UTC().Format("20060102T150405.000000000Z")
	backupPath := databasePath + ".bak." + stamp
	if err := copyPrivateFile(databasePath, backupPath, maxBackupBytes); err != nil {
		return "", err
	}
	return backupPath, nil
}

func copyPrivateFile(source, destination string, limit int64) error {
	in, err := os.Open(source)
	if err != nil {
		return classifyStoreError("backup_open", "open backup source failed", err)
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return classifyStoreError("backup_create", "create private database backup failed", err)
	}
	ok := false
	defer func() {
		_ = out.Close()
		if !ok {
			_ = os.Remove(destination)
		}
	}()
	written, err := io.Copy(out, io.LimitReader(in, limit+1))
	if err != nil {
		return classifyStoreError("backup_copy", "copy database backup failed", err)
	}
	if written > limit {
		return wrapError(CodeUnavailable, "backup_size", "database exceeds the bounded migration backup limit", false, nil)
	}
	if err := out.Sync(); err != nil {
		return classifyStoreError("backup_sync", "flush database backup failed", err)
	}
	if err := out.Close(); err != nil {
		return classifyStoreError("backup_close", "close database backup failed", err)
	}
	ok = true
	return nil
}

func verifySchemaRecord(ctx context.Context, db *sql.DB) error {
	var version int
	if err := db.QueryRowContext(ctx, `SELECT version FROM app_schema WHERE component = 'run_control'`).Scan(&version); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return wrapError(CodeCorrupt, "schema_record", "run-control application schema record is missing", false, err)
		}
		return classifyStoreError("schema_record", "read application schema record failed", err)
	}
	if version != CurrentSchemaVersion {
		return wrapError(CodeUnsupportedSchema, "schema_record", fmt.Sprintf("application schema %d does not match supported schema %d", version, CurrentSchemaVersion), false, nil)
	}
	return nil
}

func integrityCheck(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA integrity_check`)
	if err != nil {
		return classifyStoreError("integrity_check", "run-control database integrity check failed", err)
	}
	defer rows.Close()
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return classifyStoreError("integrity_check", "decode database integrity result failed", err)
		}
		if result != "ok" {
			return wrapError(CodeCorrupt, "integrity_check", "run-control database failed integrity validation", false, nil)
		}
	}
	return rows.Err()
}

func pruneMigrationBackups(databasePath string) error {
	directory := filepath.Dir(databasePath)
	prefix := filepath.Base(databasePath) + ".bak."
	entries, err := os.ReadDir(directory)
	if err != nil {
		return classifyStoreError("backup_list", "list migration backups failed", err)
	}
	var backups []string
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		backups = append(backups, entry.Name())
	}
	sort.Strings(backups)
	for len(backups) > maxMigrationBackups {
		path := filepath.Join(directory, backups[0])
		if err := os.Remove(path); err != nil {
			return classifyStoreError("backup_prune", "remove expired migration backup failed", err)
		}
		backups = backups[1:]
	}
	return nil
}

// RestoreBackup replaces the stopped workspace database with one bounded,
// private, integrity-checked backup. Callers must stop all UltraPlan processes
// and restore the matching binary before invoking this operation.
func RestoreBackup(ctx context.Context, workspaceRoot, backupName string) error {
	root, err := validateWorkspaceRoot(workspaceRoot)
	if err != nil {
		return err
	}
	databasePath := filepath.Join(root, DatabaseRelativePath)
	directory := filepath.Dir(databasePath)
	if filepath.Base(backupName) != backupName || !strings.HasPrefix(backupName, filepath.Base(databasePath)+".bak.") {
		return invalidField("backup_name", "must name a run-control migration backup in the workspace")
	}
	backupPath := filepath.Join(directory, backupName)
	info, err := os.Lstat(backupPath)
	if err != nil {
		return classifyStoreError("restore_backup", "inspect restore backup failed", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > maxBackupBytes {
		return invalidField("backup_name", "must reference a bounded regular backup file")
	}
	if err := validateBackupIntegrity(ctx, backupPath); err != nil {
		return err
	}
	temp, err := os.CreateTemp(directory, ".run-control.restore.*.tmp")
	if err != nil {
		return classifyStoreError("restore_create", "create temporary restore database failed", err)
	}
	tempPath := temp.Name()
	cleanup := true
	defer func() {
		_ = temp.Close()
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return classifyStoreError("restore_permissions", "set restore database permissions failed", err)
	}
	in, err := os.Open(backupPath)
	if err != nil {
		return classifyStoreError("restore_open", "open restore backup failed", err)
	}
	defer in.Close()
	written, err := io.Copy(temp, io.LimitReader(in, maxBackupBytes+1))
	if err != nil || written > maxBackupBytes {
		return wrapError(CodeUnavailable, "restore_copy", "copy bounded restore database failed", false, err)
	}
	if err := temp.Sync(); err != nil {
		return classifyStoreError("restore_sync", "flush restored database failed", err)
	}
	if err := temp.Close(); err != nil {
		return classifyStoreError("restore_close", "close restored database failed", err)
	}
	if err := os.Rename(tempPath, databasePath); err != nil {
		return classifyStoreError("restore_rename", "replace database from backup failed", err)
	}
	cleanup = false
	return enforcePrivateMode(databasePath, 0o600)
}

func validateBackupIntegrity(ctx context.Context, path string) error {
	values := url.Values{"mode": []string{"ro"}, "_query_only": []string{"1"}}
	dsn := (&url.URL{Scheme: "file", Path: filepath.ToSlash(path), RawQuery: values.Encode()}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return classifyStoreError("restore_validate", "open backup for validation failed", err)
	}
	defer db.Close()
	return integrityCheck(ctx, db)
}

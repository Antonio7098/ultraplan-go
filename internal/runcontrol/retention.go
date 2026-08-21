package runcontrol

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func normalizeRetentionPolicy(policy RetentionPolicy) (RetentionPolicy, error) {
	if policy.FullHistory == 0 {
		policy.FullHistory = DefaultFullHistory
	}
	if policy.TombstoneHistory == 0 {
		policy.TombstoneHistory = DefaultTombstoneHistory
	}
	if policy.HardQuotaBytes == 0 {
		policy.HardQuotaBytes = DefaultHardQuotaBytes
	}
	if policy.FullHistory < time.Hour {
		return RetentionPolicy{}, invalidField("retention.full_history", "must be at least 1 hour")
	}
	if policy.TombstoneHistory < 24*time.Hour || policy.TombstoneHistory < policy.FullHistory {
		return RetentionPolicy{}, invalidField("retention.tombstone_history", "must be at least 24 hours and no shorter than full history")
	}
	if policy.HardQuotaBytes < 64<<20 || policy.HardQuotaBytes <= ReservedHeadroomBytes {
		return RetentionPolicy{}, invalidField("retention.hard_quota", "must be at least 64 MiB with 16 MiB reserved headroom")
	}
	return policy, nil
}

func (r *SQLiteRepository) storageBytes() (int64, error) {
	directory := filepath.Dir(r.path)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return 0, classifyStoreError("quota_read", "inspect run-control storage usage failed", err)
	}
	prefix := filepath.Base(r.path)
	var total int64
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return 0, classifyStoreError("quota_read", "inspect run-control storage file failed", err)
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
	}
	return total, nil
}

func compactRunJournal(ctx context.Context, tx *sql.Tx, runID RunID, preserveSequence uint64) error {
	for batch := 0; batch < 32; batch++ {
		var count int64
		var bytes int64
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(length(payload_json)+COALESCE(length(omission_json),0)+length(stage_id)+length(task_id)+96),0) FROM events WHERE run_id = ?`, string(runID)).Scan(&count, &bytes); err != nil {
			return classifyStoreError("event_limit", "inspect per-run event retention failed", err)
		}
		if count <= MaxRetainedEventsPerRun && bytes <= MaxRetainedBytesPerRun {
			return nil
		}
		remove := int64(256)
		if excess := count - MaxRetainedEventsPerRun; excess > 0 && excess < remove {
			remove = excess
		}
		if remove < 1 {
			remove = 1
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM events WHERE run_id = ? AND sequence IN (
SELECT sequence FROM events WHERE run_id = ? AND sequence <> ? AND event_type IN ('progress','message','omission') ORDER BY sequence ASC LIMIT ?
)`, string(runID), string(runID), preserveSequence, remove)
		if err != nil {
			return classifyStoreError("event_limit", "compact bounded per-run event history failed", err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return classifyStoreError("event_limit", "inspect bounded event compaction failed", err)
		}
		if changed == 0 {
			return runError(CodeQuota, "event_limit", runID, "required durable event history reached its bounded capacity", false, nil)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE runs SET record_state = 'compacted', history_complete = 0,
oldest_retained_sequence = COALESCE((SELECT MIN(sequence) FROM events WHERE run_id = ?), last_sequence + 1)
WHERE run_id = ?`, string(runID), string(runID)); err != nil {
			return classifyStoreError("event_limit", "advance durable replay boundary failed", err)
		}
	}
	return runError(CodeQuota, "event_limit", runID, "bounded event compaction could not restore capacity", false, nil)
}

func (r *SQLiteRepository) Compact(ctx context.Context, limit int) (CompactionReport, error) {
	if limit == 0 {
		limit = 64
	}
	if limit < 1 || limit > 256 {
		return CompactionReport{}, invalidField("compaction.limit", "must be between 1 and 256")
	}
	now := r.now()
	usage, err := r.storageBytes()
	if err != nil {
		return CompactionReport{}, err
	}
	pressure := usage >= r.retention.HardQuotaBytes*80/100
	report := CompactionReport{}
	compactCutoff := now.Add(-r.retention.FullHistory / 2)
	if pressure {
		compactCutoff = now
	}
	ids, err := r.retentionCandidates(ctx, `terminal_outcome IS NOT NULL AND record_state = 'full' AND finished_at <= ?`, compactCutoff, limit)
	if err != nil {
		return report, err
	}
	for _, id := range ids {
		deleted, err := r.compactTerminalRun(ctx, id, RecordCompacted)
		if err != nil {
			return report, err
		}
		if deleted > 0 {
			report.CompactedRuns++
			report.DeletedEvents += deleted
		}
	}
	tombstoneCutoff := now.Add(-r.retention.FullHistory)
	ids, err = r.retentionCandidates(ctx, `terminal_outcome IS NOT NULL AND record_state IN ('full','compacted') AND finished_at <= ?`, tombstoneCutoff, limit)
	if err != nil {
		return report, err
	}
	for _, id := range ids {
		deleted, err := r.compactTerminalRun(ctx, id, RecordTombstone)
		if err != nil {
			return report, err
		}
		report.TombstonedRuns++
		report.DeletedEvents += deleted
	}
	expiredCutoff := now.Add(-r.retention.FullHistory - r.retention.TombstoneHistory)
	result, err := r.db.ExecContext(ctx, `DELETE FROM runs WHERE run_id IN (
SELECT run_id FROM runs WHERE record_state = 'tombstone' AND terminal_outcome IS NOT NULL AND finished_at <= ? ORDER BY finished_at ASC LIMIT ?
)`, formatTime(expiredCutoff), limit)
	if err != nil {
		return report, classifyStoreError("retention_delete", "remove expired run tombstones failed", err)
	}
	deleted, _ := result.RowsAffected()
	report.DeletedRuns = int(deleted)
	_, _ = r.db.ExecContext(ctx, `PRAGMA wal_checkpoint(PASSIVE)`)
	_, _ = r.db.ExecContext(ctx, `PRAGMA incremental_vacuum(64)`)
	r.log(ctx, LogInfo, "run retention completed",
		LogField{Key: "compacted_runs", Value: fmt.Sprint(report.CompactedRuns)},
		LogField{Key: "tombstoned_runs", Value: fmt.Sprint(report.TombstonedRuns)},
		LogField{Key: "deleted_runs", Value: fmt.Sprint(report.DeletedRuns)},
		LogField{Key: "deleted_events", Value: fmt.Sprint(report.DeletedEvents)})
	r.metrics.compactions.Add(1)
	return report, nil
}

func (r *SQLiteRepository) retentionCandidates(ctx context.Context, condition string, cutoff time.Time, limit int) ([]RunID, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT run_id FROM runs WHERE `+condition+` ORDER BY finished_at ASC, run_id ASC LIMIT ?`, formatTime(cutoff), limit)
	if err != nil {
		return nil, classifyStoreError("retention_scan", "scan terminal run retention failed", err)
	}
	defer rows.Close()
	var ids []RunID
	for rows.Next() {
		var id RunID
		if err := rows.Scan(&id); err != nil {
			return nil, classifyStoreError("retention_scan", "decode terminal retention candidate failed", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *SQLiteRepository) compactTerminalRun(ctx context.Context, runID RunID, state RecordState) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, classifyStoreError("retention_begin", "begin terminal run compaction failed", err)
	}
	defer tx.Rollback()
	removable := "'progress','message','omission'"
	if state == RecordTombstone {
		removable = "'accepted','claimed','lifecycle','progress','message','recovery','omission'"
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM events WHERE run_id = ? AND event_type IN (`+removable+`)`, string(runID))
	if err != nil {
		return 0, classifyStoreError("retention_events", "compact terminal run events failed", err)
	}
	deleted, _ := result.RowsAffected()
	result, err = tx.ExecContext(ctx, `UPDATE runs SET record_state = ?, history_complete = 0,
oldest_retained_sequence = COALESCE((SELECT MIN(sequence) FROM events WHERE run_id = ?), last_sequence + 1), updated_at = ?
WHERE run_id = ? AND terminal_outcome IS NOT NULL`, string(state), string(runID), formatTime(r.now()), string(runID))
	if err != nil {
		return 0, classifyStoreError("retention_snapshot", "update terminal retention snapshot failed", err)
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return 0, runError(CodeConflict, "retention_snapshot", runID, "terminal run changed during compaction", true, err)
	}
	if err := tx.Commit(); err != nil {
		return 0, classifyStoreError("retention_commit", "commit terminal run compaction failed", err)
	}
	return deleted, nil
}

package study

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const cleanupUncertainFileName = "cleanup-uncertain.json"

var ErrCleanupUncertain = errors.New("study cleanup remains uncertain")

type CleanupUncertainRecord struct {
	SchemaVersion int       `json:"schemaVersion"`
	OperationID   string    `json:"operationId"`
	Kind          string    `json:"kind"`
	Reason        string    `json:"reason"`
	OwnerPID      int       `json:"ownerPid"`
	RecordedAt    time.Time `json:"recordedAt"`
}

// RecordCleanupUncertain writes a separate study-owned recovery marker. It
// does not acquire the run-loop lock because the original owner may still
// hold that lock when the server's shutdown deadline expires.
func (s Service) RecordCleanupUncertain(ctx context.Context, studyRef string, record CleanupUncertainRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	st, err := s.resolveCleanupStudy(studyRef)
	if err != nil {
		return err
	}
	record.SchemaVersion = 1
	record.OperationID = strings.TrimSpace(record.OperationID)
	record.Kind = strings.TrimSpace(record.Kind)
	record.Reason = strings.TrimSpace(record.Reason)
	record.OwnerPID = os.Getpid()
	if record.RecordedAt.IsZero() {
		record.RecordedAt = time.Now().UTC()
	} else {
		record.RecordedAt = record.RecordedAt.UTC()
	}
	if record.OperationID == "" || record.Kind == "" || record.Reason != "server_shutdown" || strings.ContainsAny(record.OperationID+record.Kind, "\x00\r\n") {
		return fmt.Errorf("invalid cleanup uncertainty record")
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := cleanupUncertainPath(st)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create cleanup uncertainty directory %s: %w", filepath.Dir(path), err)
	}
	return atomicWriteFile(path, data, "."+cleanupUncertainFileName+".*.tmp")
}

// ReconcileInterruptedRun converts active task states left without a live
// run-loop owner into explicit recovery-required cancellation evidence.
func (s Service) ReconcileInterruptedRun(ctx context.Context, studyRef string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	st, err := s.resolveCleanupStudy(studyRef)
	if err != nil {
		return false, err
	}
	lock, err := AcquireRunLoopLock(st, []string{"ultraplan", "serve", "reconcile"}, false, time.Now().UTC())
	if err != nil {
		if errors.Is(err, ErrStudyLocked) {
			return false, nil
		}
		return false, err
	}
	changed, reconcileErr := reconcileInterruptedRunLocked(st, time.Now().UTC())
	releaseErr := lock.Release()
	if reconcileErr != nil {
		return false, reconcileErr
	}
	if releaseErr != nil {
		return false, releaseErr
	}
	return changed, nil
}

func (s Service) resolveCleanupStudy(ref string) (Study, error) {
	studies, err := DiscoverStudies(s.workspaceRoot)
	if err != nil {
		return Study{}, err
	}
	return ResolveStudy(studies, ref)
}

func reconcileInterruptedRunLocked(st Study, now time.Time) (bool, error) {
	_, uncertaintyErr := loadCleanupUncertain(st)
	hasUncertainty := uncertaintyErr == nil
	if uncertaintyErr != nil && !errors.Is(uncertaintyErr, fs.ErrNotExist) {
		return false, uncertaintyErr
	}
	state, err := LoadRunState(st)
	if err != nil {
		if errors.Is(err, ErrRunStateMissing) && !hasUncertainty {
			return false, nil
		}
		if errors.Is(err, ErrRunStateMissing) {
			return false, fmt.Errorf("%w: %s", ErrCleanupUncertain, cleanupUncertainPath(st))
		}
		return false, err
	}
	changed := false
	for i := range state.Tasks {
		task := &state.Tasks[i]
		switch task.Status {
		case TaskStatusRunning, TaskStatusValidating, TaskStatusWaiting, TaskStatusRetrying:
		default:
			continue
		}
		task.Status = TaskStatusCancelled
		task.RetryAfter = nil
		task.UpdatedAt = now
		task.CompletedAt = &now
		task.LastError = &TaskError{Code: "workflow.interrupted", Message: "task belonged to a stopped process; inspect durable study state before resuming"}
		changed = true
	}
	if changed {
		state.UpdatedAt = now
		state.Complete = false
		if err := SaveRunState(st, state); err != nil {
			return false, err
		}
		if err := SyncRunHistory(st, state); err != nil {
			return false, err
		}
	}
	if hasUncertainty && changed {
		if err := removeCleanupUncertain(st); err != nil {
			return false, err
		}
	}
	if hasUncertainty && !changed {
		return false, fmt.Errorf("%w: %s", ErrCleanupUncertain, cleanupUncertainPath(st))
	}
	return changed, nil
}

func cleanupUncertainPath(st Study) string {
	return filepath.Join(st.Path, RunStateDirName, cleanupUncertainFileName)
}

func loadCleanupUncertain(st Study) (CleanupUncertainRecord, error) {
	path := cleanupUncertainPath(st)
	data, err := os.ReadFile(path)
	if err != nil {
		return CleanupUncertainRecord{}, err
	}
	var record CleanupUncertainRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return CleanupUncertainRecord{}, fmt.Errorf("read cleanup uncertainty %s: %w", path, err)
	}
	if record.SchemaVersion != 1 || record.OperationID == "" || record.Kind == "" || record.Reason != "server_shutdown" || record.OwnerPID < 1 || record.RecordedAt.IsZero() {
		return CleanupUncertainRecord{}, fmt.Errorf("invalid cleanup uncertainty record %s", path)
	}
	return record, nil
}

func removeCleanupUncertain(st Study) error {
	err := os.Remove(cleanupUncertainPath(st))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

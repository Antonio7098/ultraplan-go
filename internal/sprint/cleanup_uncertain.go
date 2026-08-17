package sprint

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

const cleanupUncertainFileName = ".cleanup-uncertain.json"

var ErrCleanupUncertain = errors.New("sprint cleanup remains uncertain")

type CleanupUncertainRecord struct {
	SchemaVersion int       `json:"schemaVersion"`
	OperationID   string    `json:"operationId"`
	Kind          string    `json:"kind"`
	Reason        string    `json:"reason"`
	OwnerPID      int       `json:"ownerPid"`
	RecordedAt    time.Time `json:"recordedAt"`
}

// RecordCleanupUncertain writes a separate product-owned recovery marker. It
// deliberately does not rewrite canonical run state or acquire the mutation
// lease: at deadline exhaustion the original owner may still hold that lease.
func (s Service) RecordCleanupUncertain(ctx context.Context, projectRef, sprintRef string, record CleanupUncertainRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	sp, err := s.resolveMutationSprint(projectRef, sprintRef)
	if err != nil {
		return err
	}
	record.SchemaVersion = 1
	record.OperationID = strings.TrimSpace(record.OperationID)
	record.Kind = strings.TrimSpace(record.Kind)
	record.Reason = strings.TrimSpace(record.Reason)
	record.OwnerPID = os.Getpid()
	if record.RecordedAt.IsZero() {
		record.RecordedAt = s.now().UTC()
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
	return atomicWriteFile(filepath.Join(sp.Path, cleanupUncertainFileName), data)
}

func loadCleanupUncertain(sp Sprint) (CleanupUncertainRecord, error) {
	path := filepath.Join(sp.Path, cleanupUncertainFileName)
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

func removeCleanupUncertain(sp Sprint) error {
	err := os.Remove(filepath.Join(sp.Path, cleanupUncertainFileName))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

package runcontrol

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	LocalLogRelativePath = ".ultraplan/run-control.log"
	MaxLocalLogBytes     = int64(1 << 20)
	maxLocalLogLineBytes = 4 << 10
)

type LocalLogRecord struct {
	At      time.Time         `json:"at"`
	Level   LogLevel          `json:"level"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// LocalFileLogger is a bounded, private, append-only diagnostic sink. Each
// process writes one JSON record with one O_APPEND write; reaching the bound
// drops later diagnostic records rather than consuming reserved DB headroom.
type LocalFileLogger struct {
	mu   sync.Mutex
	file *os.File
}

func OpenLocalFileLogger(workspaceRoot string) (*LocalFileLogger, error) {
	root, err := validateWorkspaceRoot(workspaceRoot)
	if err != nil {
		return nil, err
	}
	directory := filepath.Join(root, ".ultraplan")
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, classifyStoreError("open_local_log", "inspect private run-control directory failed", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, invalidField("run_control_directory", "must be a real directory, not a symlink")
	}
	path := filepath.Join(root, filepath.FromSlash(LocalLogRelativePath))
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, invalidField("run_control_log", "must be a regular file, not a symlink")
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, classifyStoreError("open_local_log", "inspect private run-control log failed", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, classifyStoreError("open_local_log", "open private run-control log failed", err)
	}
	if err := enforcePrivateMode(path, 0o600); err != nil {
		_ = file.Close()
		return nil, err
	}
	return &LocalFileLogger{file: file}, nil
}

func (l *LocalFileLogger) Log(_ context.Context, level LogLevel, message string, fields ...LogField) {
	if l == nil || l.file == nil {
		return
	}
	record := LocalLogRecord{At: time.Now().UTC(), Level: level, Message: safeEventValue(message, 256), Fields: make(map[string]string)}
	for _, field := range fields {
		key := safeEventValue(field.Key, 64)
		if key == "" || sensitiveEventField(key) {
			continue
		}
		value := field.Value
		if unsafeEventValue(value) || len(value) > MaxSafeValueBytes {
			value = "[omitted]"
		}
		record.Fields[key] = safeEventValue(value, MaxSafeValueBytes)
	}
	encoded, err := json.Marshal(record)
	if err != nil || len(encoded)+1 > maxLocalLogLineBytes {
		return
	}
	encoded = append(encoded, '\n')
	l.mu.Lock()
	defer l.mu.Unlock()
	info, err := l.file.Stat()
	if err != nil || info.Size()+int64(len(encoded)) > MaxLocalLogBytes {
		return
	}
	_, _ = l.file.Write(encoded)
}

func (l *LocalFileLogger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

func ReadLocalLogs(workspaceRoot string, limit int) ([]LocalLogRecord, error) {
	if limit < 1 || limit > 500 {
		return nil, invalidField("local_log.limit", "must be between 1 and 500")
	}
	root, err := validateWorkspaceRoot(workspaceRoot)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(root, filepath.FromSlash(LocalLogRelativePath))
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return []LocalLogRecord{}, nil
	}
	if err != nil {
		return nil, classifyStoreError("read_local_log", "inspect private run-control log failed", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > MaxLocalLogBytes {
		return nil, wrapError(CodeCorrupt, "read_local_log", "private run-control log violates its bounded regular-file contract", false, nil)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, classifyStoreError("read_local_log", "open private run-control log failed", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(io.LimitReader(file, MaxLocalLogBytes+1))
	scanner.Buffer(make([]byte, 1024), maxLocalLogLineBytes)
	records := make([]LocalLogRecord, 0, limit)
	for scanner.Scan() {
		var record LocalLogRecord
		if json.Unmarshal(scanner.Bytes(), &record) != nil || record.At.IsZero() || record.Message == "" {
			continue
		}
		if len(records) == limit {
			copy(records, records[1:])
			records = records[:limit-1]
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, classifyStoreError("read_local_log", "read private run-control log failed", err)
	}
	return records, nil
}

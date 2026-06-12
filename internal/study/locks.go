package study

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrStudyLocked = errors.New("study run-loop locked")

func RunLoopLockPath(study Study) string {
	return filepath.Join(study.Path, RunStateDirName, "run-loop.lock")
}

type studyLock struct {
	path string
	info LockInfo
}

func AcquireRunLoopLock(study Study, command []string, force bool, now time.Time) (*studyLock, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	path := RunLoopLockPath(study)
	if force {
		if err := ForceUnlockRunLoop(study); err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create lock directory %s: %w", filepath.Dir(path), err)
	}
	info := LockInfo{
		Path:       path,
		Study:      study.Name,
		PID:        os.Getpid(),
		Command:    sanitizeCommand(command),
		AcquiredAt: now.UTC(),
	}
	content, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal lock %s: %w", path, err)
	}
	content = append(content, '\n')
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			existing, readErr := ReadRunLoopLock(study)
			if readErr != nil {
				return nil, fmt.Errorf("%w: %s exists and could not be read: %v", ErrStudyLocked, path, readErr)
			}
			return nil, fmt.Errorf("%w: %s held by pid %d since %s command %q", ErrStudyLocked, path, existing.PID, existing.AcquiredAt.Format(time.RFC3339), existing.Command)
		}
		return nil, fmt.Errorf("create lock %s: %w", path, err)
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("write lock %s: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("flush lock %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("close lock %s: %w", path, err)
	}
	return &studyLock{path: path, info: info}, nil
}

func (l *studyLock) Release() error {
	if l == nil {
		return nil
	}
	info, err := readLockPath(l.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if info.PID != l.info.PID || info.Study != l.info.Study || !info.AcquiredAt.Equal(l.info.AcquiredAt) {
		return fmt.Errorf("lock %s ownership changed; refusing release", l.path)
	}
	if err := os.Remove(l.path); err != nil {
		return fmt.Errorf("release lock %s: %w", l.path, err)
	}
	return nil
}

func ReadRunLoopLock(study Study) (LockInfo, error) {
	return readLockPath(RunLoopLockPath(study))
}

func ForceUnlockRunLoop(study Study) error {
	path := RunLoopLockPath(study)
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("force unlock %s: %w", path, err)
	}
	return nil
}

func readLockPath(path string) (LockInfo, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return LockInfo{}, err
	}
	var info LockInfo
	if err := json.Unmarshal(content, &info); err != nil {
		return LockInfo{}, fmt.Errorf("parse lock %s: %w", path, err)
	}
	if info.Path == "" {
		info.Path = path
	}
	return info, nil
}

func sanitizeCommand(args []string) string {
	if len(args) == 0 {
		return "ultraplan study run-loop"
	}
	safe := make([]string, 0, len(args))
	for _, arg := range args {
		lower := strings.ToLower(arg)
		if strings.Contains(lower, "token") || strings.Contains(lower, "key") || strings.Contains(lower, "secret") {
			safe = append(safe, "[redacted]")
			continue
		}
		if len(arg) > 120 {
			safe = append(safe, arg[:120]+"...")
			continue
		}
		safe = append(safe, arg)
	}
	return strings.Join(safe, " ")
}

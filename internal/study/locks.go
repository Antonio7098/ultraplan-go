package study

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var ErrStudyLocked = errors.New("study run-loop locked")

var processAlive = func(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

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
			if !processAlive(existing.PID) {
				if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
					return nil, fmt.Errorf("%w: stale lock %s held by dead pid %d could not be removed: %v", ErrStudyLocked, path, existing.PID, err)
				}
				file, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
				if err == nil {
					goto writeLock
				}
				if errors.Is(err, fs.ErrExist) {
					replaced, readErr := ReadRunLoopLock(study)
					if readErr != nil {
						return nil, fmt.Errorf("%w: %s exists and could not be read after stale cleanup: %v", ErrStudyLocked, path, readErr)
					}
					return nil, fmt.Errorf("%w: %s held by pid %d since %s command %q", ErrStudyLocked, path, replaced.PID, replaced.AcquiredAt.Format(time.RFC3339), replaced.Command)
				}
				return nil, fmt.Errorf("create lock %s after stale cleanup: %w", path, err)
			}
			return nil, fmt.Errorf("%w: %s held by pid %d since %s command %q", ErrStudyLocked, path, existing.PID, existing.AcquiredAt.Format(time.RFC3339), existing.Command)
		}
		return nil, fmt.Errorf("create lock %s: %w", path, err)
	}
writeLock:
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

// RunLoopActive reports whether the study lock belongs to a live process.
// A missing, malformed, or stale lock is not presented as an active run.
func RunLoopActive(study Study) (bool, LockInfo, error) {
	info, err := ReadRunLoopLock(study)
	if err != nil {
		return false, LockInfo{}, err
	}
	return processAlive(info.PID), info, nil
}

// CancelRunLoop asks the live lock owner to cancel gracefully. The owner
// persists terminal state and releases the lock through its normal context path.
func CancelRunLoop(study Study) (LockInfo, error) {
	active, info, err := RunLoopActive(study)
	if err != nil {
		return LockInfo{}, fmt.Errorf("inspect run-loop lock: %w", err)
	}
	if !active {
		return info, fmt.Errorf("study run-loop is not active")
	}
	if info.Study != "" && info.Study != study.Name {
		return info, fmt.Errorf("run-loop lock belongs to study %q", info.Study)
	}
	if info.PID == os.Getpid() {
		return info, fmt.Errorf("cannot signal the current process; use its operation context")
	}
	if err := syscall.Kill(info.PID, syscall.SIGINT); err != nil {
		return info, fmt.Errorf("request run-loop cancellation from pid %d: %w", info.PID, err)
	}
	return info, nil
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

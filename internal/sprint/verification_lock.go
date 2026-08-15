package sprint

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type verificationLockInfo struct {
	Project    string    `json:"project"`
	Sprint     string    `json:"sprint"`
	PID        int       `json:"pid"`
	AcquiredAt time.Time `json:"acquiredAt"`
}

type verificationFileLock struct {
	path string
	info verificationLockInfo
}

func acquireVerificationFileLock(root string, sprint Sprint, now time.Time) (*verificationFileLock, error) {
	path := filepath.Join(root, ".ultraplan", "locks", "sprint", sprint.Project+"--"+sprint.Slug+".lock")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create verification lock directory: %w", err)
	}
	info := verificationLockInfo{Project: sprint.Project, Sprint: sprint.Slug, PID: os.Getpid(), AcquiredAt: now.UTC()}
	for attempt := 0; attempt < 2; attempt++ {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			data, marshalErr := json.Marshal(info)
			if marshalErr == nil {
				_, marshalErr = file.Write(append(data, '\n'))
			}
			closeErr := file.Close()
			if marshalErr != nil || closeErr != nil {
				_ = os.Remove(path)
				return nil, errors.Join(marshalErr, closeErr)
			}
			return &verificationFileLock{path: path, info: info}, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("create verification lock: %w", err)
		}
		existing, readErr := readVerificationFileLock(path)
		if readErr != nil {
			return nil, fmt.Errorf("%w: unreadable lock %s: %v", ErrVerificationConflict, path, readErr)
		}
		if verificationProcessAlive(existing.PID) {
			return nil, fmt.Errorf("%w for %s/%s; pid %d has owned the attempt since %s", ErrVerificationConflict, sprint.Project, sprint.Slug, existing.PID, existing.AcquiredAt.Format(time.RFC3339))
		}
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
			return nil, fmt.Errorf("remove stale verification lock: %w", removeErr)
		}
	}
	return nil, fmt.Errorf("%w for %s/%s", ErrVerificationConflict, sprint.Project, sprint.Slug)
}

func readVerificationFileLock(path string) (verificationLockInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return verificationLockInfo{}, err
	}
	var info verificationLockInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return verificationLockInfo{}, err
	}
	if info.PID <= 0 || info.Project == "" || info.Sprint == "" || info.AcquiredAt.IsZero() {
		return verificationLockInfo{}, fmt.Errorf("invalid lock identity")
	}
	return info, nil
}

func (lock *verificationFileLock) release() error {
	if lock == nil {
		return nil
	}
	current, err := readVerificationFileLock(lock.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if current.PID != lock.info.PID || current.Project != lock.info.Project || current.Sprint != lock.info.Sprint || !current.AcquiredAt.Equal(lock.info.AcquiredAt) {
		return fmt.Errorf("verification lock ownership changed; refusing release")
	}
	return os.Remove(lock.path)
}

func verificationProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

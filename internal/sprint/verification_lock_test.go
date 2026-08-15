package sprint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestVerificationFileLockRejectsLiveOwnerAndReplacesDeadOwner(t *testing.T) {
	root, sp := reviewFixture(t)
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	lock, err := acquireVerificationFileLock(root, sp, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireVerificationFileLock(root, sp, now.Add(time.Second)); err == nil {
		t.Fatal("second live owner acquired verification lock")
	}
	if err := lock.release(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root, ".ultraplan", "locks", "sprint", sp.Project+"--"+sp.Slug+".lock")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(verificationLockInfo{Project: sp.Project, Sprint: sp.Slug, PID: 99999999, AcquiredAt: now})
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	replacement, err := acquireVerificationFileLock(root, sp, now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("replace stale owner: %v", err)
	}
	if err := replacement.release(); err != nil {
		t.Fatal(err)
	}
}

package study

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestRunLoopLockConflictForceUnlockAndRelease(t *testing.T) {
	_, st := testStudyRoot(t)
	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	lock, err := AcquireRunLoopLock(st, []string{"ultraplan", "study", "sample", "run-loop", "--api-key=secret"}, false, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireRunLoopLock(st, []string{"second"}, false, now); !errors.Is(err, ErrStudyLocked) {
		t.Fatalf("second acquire error = %v, want ErrStudyLocked", err)
	}
	info, err := ReadRunLoopLock(st)
	if err != nil {
		t.Fatal(err)
	}
	if info.Study != st.Name || info.PID == 0 || info.Command == "" {
		t.Fatalf("lock info = %#v", info)
	}
	if err := ForceUnlockRunLoop(st); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(RunLoopLockPath(st)); !os.IsNotExist(err) {
		t.Fatalf("lock still exists after force unlock: %v", err)
	}
	replacement, err := AcquireRunLoopLock(st, []string{"replacement"}, false, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := replacement.Release(); err != nil {
		t.Fatal(err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("stale release should be harmless after missing lock: %v", err)
	}
}

func TestRunLoopLockStaleDeadPIDIsReplaced(t *testing.T) {
	_, st := testStudyRoot(t)
	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	restore := stubProcessAlive(false)
	defer restore()

	first, err := AcquireRunLoopLock(st, []string{"first"}, false, now)
	if err != nil {
		t.Fatal(err)
	}
	if first.info.Command != "first" {
		t.Fatalf("first lock = %#v", first.info)
	}
	second, err := AcquireRunLoopLock(st, []string{"second"}, false, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("stale lock should be replaced: %v", err)
	}
	info, err := ReadRunLoopLock(st)
	if err != nil {
		t.Fatal(err)
	}
	if info.Command != "second" {
		t.Fatalf("lock was not replaced: %#v", info)
	}
	if err := second.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestRunLoopLockLivePIDStillBlocks(t *testing.T) {
	_, st := testStudyRoot(t)
	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	restore := stubProcessAlive(true)
	defer restore()

	lock, err := AcquireRunLoopLock(st, []string{"first"}, false, now)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	if _, err := AcquireRunLoopLock(st, []string{"second"}, false, now.Add(time.Minute)); !errors.Is(err, ErrStudyLocked) {
		t.Fatalf("live lock acquire error = %v, want ErrStudyLocked", err)
	}
}

func stubProcessAlive(alive bool) func() {
	orig := processAlive
	processAlive = func(int) bool { return alive }
	return func() { processAlive = orig }
}

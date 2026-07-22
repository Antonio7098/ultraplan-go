package process

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestDirectRunnerExactEnvironmentCwdAndCapture(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	dir := t.TempDir()
	result, err := (DirectRunner{}).Run(context.Background(), Request{
		Executable: "/bin/sh", Args: []string{"-c", `printf '%s|%s' "$ONLY_VALUE" "$PWD"; printf err >&2`},
		Dir: dir, Env: []string{"ONLY_VALUE=allowed"}, Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "allowed|"+dir || result.Stderr != "err" {
		t.Fatalf("result = %+v", result)
	}
}

func TestDirectRunnerCancellationCleansOwnedDescendant(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("process-group assertion is Unix-only")
	}
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "child.pid")
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(60 * time.Millisecond); cancel() }()
	result, err := (DirectRunner{}).Run(ctx, Request{Executable: "/bin/sh", Args: []string{"-c", "sleep 30 & echo $! > child.pid; wait"}, Dir: dir, Env: []string{"PATH=/usr/bin:/bin"}, Timeout: time.Second, CleanupGrace: 50 * time.Millisecond})
	if err == nil || !result.Cancelled || !result.CleanupComplete {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	data, readErr := os.ReadFile(pidPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		killErr := syscall.Kill(pid, 0)
		if killErr != nil {
			return
		}
		if runtime.GOOS == "linux" {
			if stat, readErr := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat")); readErr == nil {
				fields := strings.Fields(string(stat))
				if len(fields) > 2 && fields[2] == "Z" {
					return
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("descendant process %d survived cancellation", pid)
}

func TestDirectRunnerSlowProgressDoesNotBlockDrain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	result, err := (DirectRunner{}).Run(context.Background(), Request{Executable: "/bin/sh", Args: []string{"-c", "i=0; while [ $i -lt 5000 ]; do printf x; i=$((i+1)); done"}, Dir: os.TempDir(), Env: []string{"PATH=/usr/bin:/bin"}, Timeout: 2 * time.Second, StdoutLimit: 128, Progress: func(Event) { time.Sleep(time.Millisecond) }})
	if err != nil {
		t.Fatal(err)
	}
	if !result.StdoutTruncated || len(result.Stdout) != 128 {
		t.Fatalf("result=%+v", result)
	}
}

func TestDirectRunnerBoundsAndTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	result, err := (DirectRunner{}).Run(context.Background(), Request{
		Executable: "/bin/sh", Args: []string{"-c", "printf 123456789; sleep 5"}, Dir: filepath.Clean(os.TempDir()),
		Env: []string{"PATH=/usr/bin:/bin"}, Timeout: 50 * time.Millisecond, StdoutLimit: 4, CleanupGrace: 50 * time.Millisecond,
	})
	if err == nil || !result.TimedOut || !result.CleanupComplete {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if result.Stdout != "1234" || !result.StdoutTruncated || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

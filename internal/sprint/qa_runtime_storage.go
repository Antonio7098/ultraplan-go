package sprint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Runtime storage is deliberately outside both the implementation and governed
// workspace. XDG_RUNTIME_DIR and os.TempDir commonly refer to small tmpfs mounts.
func qaRuntimeRoot() string {
	if path := os.Getenv("ULTRAPLAN_QA_RUNTIME_DIR"); path != "" {
		return filepath.Clean(path)
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base = filepath.Join("/var/tmp", fmt.Sprintf("ultraplan-cache-%d", os.Getuid()))
	}
	return filepath.Join(base, "ultraplan", "qa-runtime")
}

func validateQARuntimeLocation(protected []string) error {
	if !filepath.IsAbs(qaRuntimeRoot()) {
		return fmt.Errorf("ULTRAPLAN_QA_RUNTIME_DIR must be absolute")
	}
	runtimeRoot, err := filepath.Abs(qaRuntimeRoot())
	if err != nil {
		return err
	}
	// Resolve the existing ancestor even before the configured directory exists.
	resolve := func(path string) string {
		var suffix []string
		for {
			if resolved, err := filepath.EvalSymlinks(path); err == nil {
				for i := len(suffix) - 1; i >= 0; i-- {
					resolved = filepath.Join(resolved, suffix[i])
				}
				return resolved
			}
			parent := filepath.Dir(path)
			if parent == path {
				return path
			}
			suffix = append(suffix, filepath.Base(path))
			path = parent
		}
	}
	runtimeRoot = resolve(runtimeRoot)
	for _, path := range protected {
		if path == "" {
			continue
		}
		path, err = filepath.Abs(path)
		if err != nil {
			return err
		}
		path = resolve(path)
		if inside(path, runtimeRoot) || inside(runtimeRoot, path) {
			return fmt.Errorf("QA runtime directory must be outside the implementation and governed workspace")
		}
	}
	return nil
}

func qaRuntimeTemp(prefix string) (string, error) {
	root := qaRuntimeRoot()
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("ULTRAPLAN_QA_RUNTIME_DIR must be absolute")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	cleanupQAOrphanScratch(root)
	path, err := os.MkdirTemp(root, prefix)
	if err != nil {
		return "", err
	}
	owner, _ := json.Marshal(qaResourceLease{PID: os.Getpid()})
	if err := os.WriteFile(filepath.Join(path, ".qa-runtime-owner.json"), owner, 0600); err != nil {
		_ = os.RemoveAll(path)
		return "", err
	}
	return path, nil
}

type qaResourceLease struct {
	PID    int   `json:"pid"`
	Bytes  int64 `json:"bytes"`
	Memory int64 `json:"memory"`
}

// Reservations are coordinated across processes, and a dead owner cannot keep
// capacity reserved. Actual retained workspaces are accounted by filesystem free
// space. Reserving peak additional bytes is conservative while a worker grows.
func reserveQAResources(ctx context.Context, bytes, memory int64) (func(), error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if value := os.Getenv("ULTRAPLAN_QA_WORKER_MEMORY_MB"); value != "" {
		mb, err := strconv.ParseInt(value, 10, 64)
		if err != nil || mb <= 0 || mb > 1<<20 {
			return nil, fmt.Errorf("ULTRAPLAN_QA_WORKER_MEMORY_MB must be a positive integer no greater than 1048576")
		}
	}
	root := qaRuntimeRoot()
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("QA runtime directory must be absolute")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	for {
		release, available, err := tryReserveQAResources(root, bytes, memory)
		if err != nil {
			return nil, err
		}
		if available {
			return release, nil
		}
		select {
		case <-ctx.Done():
			return nil, &qaExecutionError{Failure: &QAFailureDiagnostic{Phase: "admission", Code: "resource_capacity_unavailable", Retryable: true, Diagnostic: "QA waited for disk and memory capacity; restore capacity before retrying."}, Err: ctx.Err()}
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func tryReserveQAResources(root string, bytes, memory int64) (func(), bool, error) {
	unlock, locked, err := qaStorageTryLock(filepath.Join(root, "admission.lock"))
	if err == nil && !locked {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer unlock()
	free, err := qaStorageAvailable(root)
	if err != nil {
		return nil, false, err
	}
	entries, err := filepath.Glob(filepath.Join(root, "reservation-*.json"))
	if err != nil {
		return nil, false, err
	}
	var reserved, reservedMemory int64
	for _, path := range entries {
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, false, err
		}
		var lease qaResourceLease
		if err := json.Unmarshal(data, &lease); err != nil {
			return nil, false, err
		}
		if !qaProcessAlive(lease.PID) {
			_ = os.Remove(path)
			continue
		}
		reserved += lease.Bytes
		reservedMemory += lease.Memory
	}
	const diskReserve = 256 << 20
	if free-reserved < bytes+diskReserve {
		return nil, false, nil
	}
	if memory > 0 {
		if available, ok := qaHostAvailableMemory(); ok && available-reservedMemory < memory+qaHostMemoryReserve {
			return nil, false, nil
		}
	}
	file, err := os.CreateTemp(root, ".reservation-pending-*")
	if err != nil {
		return nil, false, err
	}
	path := file.Name()
	err = json.NewEncoder(file).Encode(qaResourceLease{PID: os.Getpid(), Bytes: bytes, Memory: memory})
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		return nil, false, err
	}
	published := filepath.Join(root, "reservation-"+filepath.Base(path)+".json")
	if err := os.Rename(path, published); err != nil {
		_ = os.Remove(path)
		return nil, false, err
	}
	return func() { _ = os.Remove(published) }, true, nil
}

func qaStorageLockContext(ctx context.Context, path string) (func(), error) {
	for {
		unlock, locked, err := qaStorageTryLock(path)
		if err != nil {
			return nil, err
		}
		if locked {
			return unlock, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func cleanupQAOrphanScratch(root string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		ownedPrefix := false
		for _, prefix := range []string{"ultraplan-qa-dependencies-", "ultraplan-qa-authored-test-", "ultraplan-qa-rerun-", "ultraplan-qa-evidence-", "ultraplan-qa-authoring-snapshot-"} {
			ownedPrefix = ownedPrefix || strings.HasPrefix(entry.Name(), prefix)
		}
		if !entry.IsDir() || !ownedPrefix {
			continue
		}
		path := filepath.Join(root, entry.Name())
		data, err := os.ReadFile(filepath.Join(path, ".qa-runtime-owner.json"))
		if err != nil {
			continue
		}
		var owner qaResourceLease
		if json.Unmarshal(data, &owner) != nil || owner.PID <= 0 || qaProcessAlive(owner.PID) {
			continue
		}
		_ = removeQAReproductionRuntime(path)
	}
}

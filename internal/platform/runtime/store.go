package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

const RuntimeStoreDirName = "runtime/opencode"

type RuntimeStoreState string

const (
	RuntimeStoreActive         RuntimeStoreState = "active"
	RuntimeStoreRetained       RuntimeStoreState = "retained"
	RuntimeStoreCleanupPending RuntimeStoreState = "cleanup_pending"
)

type RuntimeStoreRecord struct {
	SchemaVersion int               `json:"schema_version"`
	Owner         string            `json:"owner"`
	DatabasePath  string            `json:"database_path"`
	State         RuntimeStoreState `json:"state"`
	PID           int               `json:"pid,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	LastError     string            `json:"last_error,omitempty"`
}

type RuntimeStoreInfo struct {
	RuntimeStoreRecord
	Bytes uint64 `json:"bytes"`
}

type RuntimeStoreCleanup struct {
	Removed []RuntimeStoreInfo `json:"removed,omitempty"`
	Failed  []string           `json:"failed,omitempty"`
}

func ScopedRuntimeStorePath(scopeRoot, owner string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(owner)))
	return filepath.Join(scopeRoot, ".ultraplan", RuntimeStoreDirName, hex.EncodeToString(sum[:16]), "opencode.db")
}

func prepareRuntimeStore(path, owner string) error {
	dir, err := validatedRuntimeStoreDir(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create runtime store: %w", err)
	}
	now := time.Now().UTC()
	record := RuntimeStoreRecord{SchemaVersion: 1, Owner: owner, DatabasePath: path, State: RuntimeStoreActive, PID: os.Getpid(), CreatedAt: now, UpdatedAt: now}
	if previous, err := loadRuntimeStoreRecord(dir); err == nil {
		record.CreatedAt = previous.CreatedAt
	}
	return writeRuntimeStoreRecord(dir, record)
}

func retainRuntimeStore(path, owner string, runErr error) {
	if strings.TrimSpace(path) == "" {
		return
	}
	dir, err := validatedRuntimeStoreDir(path)
	if err != nil {
		return
	}
	record, loadErr := loadRuntimeStoreRecord(dir)
	if loadErr != nil {
		now := time.Now().UTC()
		record = RuntimeStoreRecord{SchemaVersion: 1, Owner: owner, DatabasePath: path, CreatedAt: now}
	}
	record.State = RuntimeStoreRetained
	record.PID = 0
	record.UpdatedAt = time.Now().UTC()
	if runErr != nil {
		record.LastError = runErr.Error()
	}
	_ = writeRuntimeStoreRecord(dir, record)
}

func markRuntimeStoreCleanupPending(path string, cleanupErr error) {
	dir, err := validatedRuntimeStoreDir(path)
	if err != nil {
		return
	}
	record, err := loadRuntimeStoreRecord(dir)
	if err != nil {
		return
	}
	record.State = RuntimeStoreCleanupPending
	record.PID = 0
	record.UpdatedAt = time.Now().UTC()
	if cleanupErr != nil {
		record.LastError = cleanupErr.Error()
	}
	_ = writeRuntimeStoreRecord(dir, record)
}

func removeRuntimeStore(path string) error {
	dir, err := validatedRuntimeStoreDir(path)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove runtime store %s: %w", dir, err)
	}
	return nil
}

func validatedRuntimeStoreDir(path string) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(path))
	if !filepath.IsAbs(clean) || filepath.Base(clean) != "opencode.db" {
		return "", fmt.Errorf("invalid runtime store path %q", path)
	}
	dir := filepath.Dir(clean)
	marker := string(filepath.Separator) + filepath.FromSlash(".ultraplan/"+RuntimeStoreDirName) + string(filepath.Separator)
	if !strings.Contains(dir+string(filepath.Separator), marker) {
		return "", fmt.Errorf("runtime store is outside managed storage: %q", path)
	}
	return dir, nil
}

func loadRuntimeStoreRecord(dir string) (RuntimeStoreRecord, error) {
	data, err := os.ReadFile(filepath.Join(dir, "store.json"))
	if err != nil {
		return RuntimeStoreRecord{}, err
	}
	var record RuntimeStoreRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return RuntimeStoreRecord{}, err
	}
	if record.SchemaVersion != 1 {
		return RuntimeStoreRecord{}, errors.New("unsupported runtime store record")
	}
	return record, nil
}

func writeRuntimeStoreRecord(dir string, record RuntimeStoreRecord) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(dir, ".store.*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Chmod(0o600)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(dir, "store.json"))
}

func InspectRuntimeStores(scopeRoot string) ([]RuntimeStoreInfo, error) {
	root := filepath.Join(scopeRoot, ".ultraplan", RuntimeStoreDirName)
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	stores := make([]RuntimeStoreInfo, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		record, err := loadRuntimeStoreRecord(dir)
		if err != nil {
			continue
		}
		stores = append(stores, RuntimeStoreInfo{RuntimeStoreRecord: record, Bytes: directoryBytes(dir)})
	}
	sort.Slice(stores, func(i, j int) bool { return stores[i].UpdatedAt.Before(stores[j].UpdatedAt) })
	return stores, nil
}

// CleanupRuntimeStores retries interrupted cleanup, expires abandoned stores,
// and optionally sacrifices retained sessions under critical disk pressure.
func CleanupRuntimeStores(scopeRoot string, maxAge time.Duration, maxBytes uint64, aggressive bool) RuntimeStoreCleanup {
	stores, err := InspectRuntimeStores(scopeRoot)
	if err != nil {
		return RuntimeStoreCleanup{Failed: []string{err.Error()}}
	}
	now := time.Now().UTC()
	var total uint64
	for _, store := range stores {
		total += store.Bytes
	}
	result := RuntimeStoreCleanup{}
	for _, store := range stores {
		staleActive := store.State == RuntimeStoreActive && !processAlive(store.PID) && now.Sub(store.UpdatedAt) > 30*time.Minute
		expired := store.State == RuntimeStoreRetained && maxAge > 0 && now.Sub(store.UpdatedAt) > maxAge
		overQuota := maxBytes > 0 && total > maxBytes && store.State != RuntimeStoreActive
		remove := store.State == RuntimeStoreCleanupPending || staleActive || expired || overQuota || (aggressive && store.State != RuntimeStoreActive)
		if !remove {
			continue
		}
		if err := removeRuntimeStore(store.DatabasePath); err != nil {
			result.Failed = append(result.Failed, err.Error())
			continue
		}
		result.Removed = append(result.Removed, store)
		if store.Bytes <= total {
			total -= store.Bytes
		}
	}
	return result
}

func processAlive(pid int) bool {
	if pid < 1 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

func directoryBytes(root string) uint64 {
	var total uint64
	_ = filepath.WalkDir(root, func(_ string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if info, statErr := entry.Info(); statErr == nil && info.Size() > 0 {
			total += uint64(info.Size())
		}
		return nil
	})
	return total
}

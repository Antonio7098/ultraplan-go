package runtime

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRuntimeStoreLifecycle(t *testing.T) {
	root := t.TempDir()
	path := ScopedRuntimeStorePath(root, "study/demo/analysis/01/repo")
	if err := prepareRuntimeStore(path, "study/demo/analysis/01/repo"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, 128), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+"-wal", make([]byte, 64), 0o600); err != nil {
		t.Fatal(err)
	}
	stores, err := InspectRuntimeStores(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(stores) != 1 || stores[0].Owner != "study/demo/analysis/01/repo" || stores[0].Bytes < 192 {
		t.Fatalf("stores = %+v", stores)
	}
	retainRuntimeStore(path, "study/demo/analysis/01/repo", nil)
	stores, err = InspectRuntimeStores(root)
	if err != nil || len(stores) != 1 || stores[0].State != RuntimeStoreRetained {
		t.Fatalf("retained stores = %+v, err = %v", stores, err)
	}
	if err := removeRuntimeStore(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("store directory still exists: %v", err)
	}
}

func TestCleanupRuntimeStoresRetriesPendingAndRemovesExpiredStores(t *testing.T) {
	root := t.TempDir()
	for _, owner := range []string{"pending", "expired"} {
		path := ScopedRuntimeStorePath(root, owner)
		if err := prepareRuntimeStore(path, owner); err != nil {
			t.Fatal(err)
		}
		retainRuntimeStore(path, owner, nil)
		if owner == "pending" {
			markRuntimeStoreCleanupPending(path, os.ErrPermission)
		}
		record, err := loadRuntimeStoreRecord(filepath.Dir(path))
		if err != nil {
			t.Fatal(err)
		}
		record.UpdatedAt = time.Now().Add(-48 * time.Hour)
		if err := writeRuntimeStoreRecord(filepath.Dir(path), record); err != nil {
			t.Fatal(err)
		}
	}
	summary := CleanupRuntimeStores(root, 24*time.Hour, 0, false)
	if len(summary.Failed) != 0 || len(summary.Removed) != 2 {
		t.Fatalf("cleanup = %+v", summary)
	}
	stores, err := InspectRuntimeStores(root)
	if err != nil || len(stores) != 0 {
		t.Fatalf("stores = %+v, err = %v", stores, err)
	}
}

func TestRemoveRuntimeStoreRejectsPathsOutsideManagedRoot(t *testing.T) {
	if err := removeRuntimeStore(filepath.Join(t.TempDir(), "opencode.db")); err == nil {
		t.Fatal("expected unmanaged path to be rejected")
	}
}

func TestCleanupRuntimeStoresPreservesAnInterruptedStoreForResume(t *testing.T) {
	root := t.TempDir()
	path := ScopedRuntimeStorePath(root, "interrupted")
	if err := prepareRuntimeStore(path, "interrupted"); err != nil {
		t.Fatal(err)
	}
	record, err := loadRuntimeStoreRecord(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	record.PID = 999999999
	record.UpdatedAt = time.Now().Add(-time.Hour)
	if err := writeRuntimeStoreRecord(filepath.Dir(path), record); err != nil {
		t.Fatal(err)
	}
	summary := CleanupRuntimeStores(root, 72*time.Hour, 0, false)
	if len(summary.Removed) != 0 || len(summary.Failed) != 0 {
		t.Fatalf("cleanup = %+v", summary)
	}
	stores, err := InspectRuntimeStores(root)
	if err != nil || len(stores) != 1 || stores[0].State != RuntimeStoreRetained {
		t.Fatalf("stores = %+v, err = %v", stores, err)
	}
}

func TestAggressiveCleanupSacrificesRetainedButNotLiveStores(t *testing.T) {
	root := t.TempDir()
	retained := ScopedRuntimeStorePath(root, "retained")
	live := ScopedRuntimeStorePath(root, "live")
	for owner, path := range map[string]string{"retained": retained, "live": live} {
		if err := prepareRuntimeStore(path, owner); err != nil {
			t.Fatal(err)
		}
	}
	retainRuntimeStore(retained, "retained", nil)
	summary := CleanupRuntimeStores(root, 72*time.Hour, 0, true)
	if len(summary.Removed) != 1 || summary.Removed[0].Owner != "retained" {
		t.Fatalf("cleanup = %+v", summary)
	}
	stores, err := InspectRuntimeStores(root)
	if err != nil || len(stores) != 1 || stores[0].Owner != "live" || stores[0].State != RuntimeStoreActive {
		t.Fatalf("stores = %+v, err = %v", stores, err)
	}
}

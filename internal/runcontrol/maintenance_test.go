package runcontrol

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSQLiteDefaultsToOneProcessLocalConnection(t *testing.T) {
	repository, err := OpenSQLite(context.Background(), t.TempDir(), SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	if got := repository.db.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("max open connections = %d, want 1", got)
	}
}

func TestMaintainSkipsWhenAnotherProcessOwnsTheLock(t *testing.T) {
	root := t.TempDir()
	repository, err := OpenSQLite(context.Background(), root, SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	lock, acquired, err := tryMaintenanceLock(filepath.Join(root, ".ultraplan", maintenanceLockName))
	if err != nil || !acquired {
		t.Fatalf("acquire maintenance lock: acquired=%v err=%v", acquired, err)
	}
	defer lock.release()
	performed, err := repository.Maintain(context.Background(), NativeProcessProbe{})
	if err != nil {
		t.Fatal(err)
	}
	if performed {
		t.Fatal("maintenance ran while its cross-process lock was held")
	}
}

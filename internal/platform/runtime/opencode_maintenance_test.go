package runtime

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPruneLogDirectoryExpiresAndCapsInactiveLogs(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC()
	writeLog := func(name string, size int, age time.Duration) string {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
			t.Fatal(err)
		}
		updated := now.Add(-age)
		if err := os.Chtimes(path, updated, updated); err != nil {
			t.Fatal(err)
		}
		return path
	}
	expired := writeLog("expired.log", 8, 72*time.Hour)
	oldest := writeLog("oldest.log", 16, time.Hour)
	recent := writeLog("recent.log", 16, time.Minute)
	if err := pruneLogDirectory(root, now, 48*time.Hour, 20); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{expired, oldest} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("log was not removed: %s (%v)", path, err)
		}
	}
	if _, err := os.Stat(recent); err != nil {
		t.Fatalf("recent active log was removed: %v", err)
	}
}

func TestEnvValueUsesLastOverride(t *testing.T) {
	if got := envValue([]string{"XDG_DATA_HOME=/first", "OTHER=1", "XDG_DATA_HOME=/second"}, "XDG_DATA_HOME"); got != "/second" {
		t.Fatalf("value = %q", got)
	}
}

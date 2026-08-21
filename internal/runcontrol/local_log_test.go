package runcontrol

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalFileLoggerIsPrivateBoundedAndRedactsUnsafeFields(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repository, err := OpenSQLite(ctx, root, SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	logger, err := OpenLocalFileLogger(root)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()
	repository.SetLogger(logger)
	snapshot, err := repository.Accept(ctx, Acceptance{Target: Target{Kind: "log-test", Operation: "safe"}})
	if err != nil {
		t.Fatal(err)
	}
	logger.Log(ctx, LogWarn, "bounded diagnostic",
		LogField{Key: "run_id", Value: string(snapshot.RunID)},
		LogField{Key: "prompt", Value: "must never appear"},
		LogField{Key: "diagnostic", Value: "/absolute/private/path"})
	path := filepath.Join(root, filepath.FromSlash(LocalLogRelativePath))
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 || info.Size() <= 0 || info.Size() > MaxLocalLogBytes {
		t.Fatalf("log mode=%04o size=%d", info.Mode().Perm(), info.Size())
	}
	records, err := ReadLocalLogs(root, 100)
	if err != nil || len(records) < 2 {
		t.Fatalf("records=%+v err=%v", records, err)
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "must never appear") || strings.Contains(string(encoded), "/absolute/private/path") || strings.Contains(string(encoded), "prompt") {
		t.Fatalf("unsafe local log content: %s", encoded)
	}
	if !strings.Contains(string(encoded), string(snapshot.RunID)) || !strings.Contains(string(encoded), `"diagnostic":"[omitted]"`) {
		t.Fatalf("safe correlation or omission missing: %s", encoded)
	}
}

func TestReadLocalLogsReturnsOnlyNewestBoundedRecords(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repository, err := OpenSQLite(ctx, root, SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	logger, err := OpenLocalFileLogger(root)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()
	for index := 0; index < 30; index++ {
		logger.Log(ctx, LogInfo, "bounded record", LogField{Key: "sequence", Value: string(rune('a' + index%26))})
	}
	records, err := ReadLocalLogs(root, 7)
	if err != nil || len(records) != 7 {
		t.Fatalf("newest records=%d err=%v", len(records), err)
	}
}

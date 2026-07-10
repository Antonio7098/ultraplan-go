package study

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRunLoopDiagnosticsWritesMemorySample(t *testing.T) {
	study := Study{Name: "sample", Path: t.TempDir()}
	statePath := RunStatePath(study)
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, []byte("state"), 0o644); err != nil {
		t.Fatal(err)
	}
	diagnostics := newRunLoopDiagnostics(study, "run-1")
	diagnostics.sample("state.save.end", "task-1", 0, nil)

	file, err := os.Open(diagnostics.path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		t.Fatalf("missing diagnostic sample: %v", scanner.Err())
	}
	var sample runLoopMemorySample
	if err := json.Unmarshal(scanner.Bytes(), &sample); err != nil {
		t.Fatal(err)
	}
	if sample.RunID != "run-1" || sample.Phase != "state.save.end" || sample.TaskID != "task-1" {
		t.Fatalf("unexpected sample: %+v", sample)
	}
	if sample.StateBytes != 5 || sample.HeapAllocBytes == 0 || sample.Goroutines == 0 {
		t.Fatalf("missing memory facts: %+v", sample)
	}
}

func TestSnapshotFilesSkipsInternalStateAndGitData(t *testing.T) {
	root := t.TempDir()
	visible := filepath.Join(root, "source.go")
	internal := filepath.Join(root, RunStateDirName, RunStateFileName)
	gitObject := filepath.Join(root, ".git", "objects", "object")
	for path, content := range map[string]string{visible: "visible", internal: "state", gitObject: "object"} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := snapshotFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := snapshot[visible]; !ok {
		t.Fatalf("visible file missing from snapshot: %+v", snapshot)
	}
	if _, ok := snapshot[internal]; ok {
		t.Fatalf("internal state included in snapshot: %+v", snapshot)
	}
	if _, ok := snapshot[gitObject]; ok {
		t.Fatalf("git object included in snapshot: %+v", snapshot)
	}
}

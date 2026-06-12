package study

import (
	"context"
	"os"
	"testing"
)

func TestRunLoopCreatesDurableStateRunsTasksAndReleasesLock(t *testing.T) {
	root, st := executionFixture(t)
	rt := &runAllRuntime{write: validSourceReport}
	service := NewService(root, WithRuntime(rt, runtimeRequest()))

	result, err := service.RunLoop(context.Background(), RunLoopRequest{
		StudyRef:      "demo",
		DimensionRefs: []string{"01"},
		SourceRefs:    []string{"repo", "doc.md", "other.md"},
		Parallelism:   2,
		Config:        ConfigSummary{Runtime: "opencode", Model: "test/model"},
		Command:       []string{"ultraplan", "study", "demo", "run-loop"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != RunAllStatusCompleted {
		t.Fatalf("Status = %q counts = %+v", result.Status, result.Counts)
	}
	if result.Counts.Completed != 3 || result.Counts.Failed != 0 || result.Counts.Pending != 0 {
		t.Fatalf("Counts = %+v", result.Counts)
	}
	if rt.calls != 3 {
		t.Fatalf("runtime calls = %d, want 3", rt.calls)
	}
	loaded, err := LoadRunState(st)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Complete || loaded.Config.Model != "test/model" || len(loaded.Tasks) != 3 {
		t.Fatalf("loaded state = %#v", loaded)
	}
	for _, task := range loaded.Tasks {
		if task.Status != TaskStatusCompleted || task.Attempts != 1 {
			t.Fatalf("task = %#v", task)
		}
	}
	if _, err := os.Stat(RunLoopLockPath(st)); !os.IsNotExist(err) {
		t.Fatalf("lock should be released, stat err = %v", err)
	}
}

func TestRunLoopResumesAndRevalidatesCompletedStateBeforeScheduling(t *testing.T) {
	root, st := executionFixture(t)
	rt := &runAllRuntime{write: validSourceReport}
	service := NewService(root, WithRuntime(rt, runtimeRequest()))
	if _, err := service.RunLoop(context.Background(), RunLoopRequest{StudyRef: "demo", DimensionRefs: []string{"01"}, SourceRefs: []string{"repo"}, Parallelism: 1}); err != nil {
		t.Fatal(err)
	}
	os.Remove(SourceReportPath(st, Source{Name: "repo", Kind: SourceKindDirectory}, Dimension{Number: "01", Slug: "structure"}))

	rt = &runAllRuntime{write: validSourceReport}
	service = NewService(root, WithRuntime(rt, runtimeRequest()))
	result, err := service.RunLoop(context.Background(), RunLoopRequest{StudyRef: "demo", DimensionRefs: []string{"01"}, SourceRefs: []string{"repo"}, Parallelism: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != RunAllStatusCompleted {
		t.Fatalf("Status = %q counts = %+v", result.Status, result.Counts)
	}
	if rt.calls == 0 {
		t.Fatal("expected missing completed report to be rerun after resume validation")
	}
}

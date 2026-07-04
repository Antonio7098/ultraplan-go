package study

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	runtimepkg "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
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
	if result.ScopeCounts.Completed != 3 || result.ScopeCounts.Failed != 0 || result.ScopeCounts.Pending != 0 {
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

func TestRunLoopProgressUsesCountOnlySummaries(t *testing.T) {
	root, _ := executionFixture(t)
	rt := &runAllRuntime{write: validSourceReport}
	service := NewService(root, WithRuntime(rt, runtimeRequest()))
	events := 0

	_, err := service.RunLoop(context.Background(), RunLoopRequest{
		StudyRef:      "demo",
		DimensionRefs: []string{"01"},
		SourceRefs:    []string{"repo"},
		Parallelism:   1,
		Progress: func(progress RunLoopProgress) {
			events++
			if len(progress.Counts.Tasks) != 0 {
				t.Fatalf("progress Counts.Tasks length = %d, want 0", len(progress.Counts.Tasks))
			}
			if len(progress.ScopeCounts.Tasks) != 0 {
				t.Fatalf("progress ScopeCounts.Tasks length = %d, want 0", len(progress.ScopeCounts.Tasks))
			}
			if progress.Counts.Total == 0 || progress.ScopeCounts.Total == 0 {
				t.Fatalf("progress counts missing totals: counts=%+v scope=%+v", progress.Counts, progress.ScopeCounts)
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if events == 0 {
		t.Fatal("expected progress events")
	}
}

func TestRunLoopContinueResumesAndRevalidatesCompletedStateBeforeScheduling(t *testing.T) {
	root, st := executionFixture(t)
	rt := &runAllRuntime{write: validSourceReport}
	service := NewService(root, WithRuntime(rt, runtimeRequest()))
	if _, err := service.RunLoop(context.Background(), RunLoopRequest{StudyRef: "demo", DimensionRefs: []string{"01"}, SourceRefs: []string{"repo"}, Parallelism: 1}); err != nil {
		t.Fatal(err)
	}
	os.Remove(SourceReportPath(st, Source{Name: "repo", Kind: SourceKindDirectory}, Dimension{Number: "01", Slug: "structure"}))

	rt = &runAllRuntime{write: validSourceReport}
	service = NewService(root, WithRuntime(rt, runtimeRequest()))
	result, err := service.RunLoop(context.Background(), RunLoopRequest{StudyRef: "demo", DimensionRefs: []string{"01"}, SourceRefs: []string{"repo"}, Parallelism: 1, Continue: true})
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

func TestRunLoopDefaultResumesExistingStateAndResetStartsFresh(t *testing.T) {
	root, st := executionFixture(t)
	rt := &runAllRuntime{write: validSourceReport}
	service := NewService(root, WithRuntime(rt, runtimeRequest()))
	if _, err := service.RunLoop(context.Background(), RunLoopRequest{StudyRef: "demo", DimensionRefs: []string{"01"}, SourceRefs: []string{"repo"}, Parallelism: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRunState(st); err != nil {
		t.Fatal(err)
	}

	rt = &runAllRuntime{write: validSourceReport}
	service = NewService(root, WithRuntime(rt, runtimeRequest()))
	result, err := service.RunLoop(context.Background(), RunLoopRequest{StudyRef: "demo", DimensionRefs: []string{"01"}, SourceRefs: []string{"doc.md"}, Parallelism: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != RunAllStatusCompleted {
		t.Fatalf("Status = %q counts = %+v", result.Status, result.Counts)
	}
	resumed, err := LoadRunState(st)
	if err != nil {
		t.Fatal(err)
	}
	if len(resumed.Tasks) != 3 || resumed.Tasks[0].Source != "doc.md" || resumed.Tasks[1].Source != "repo" {
		t.Fatalf("tasks = %#v, want shared full-study state", resumed.Tasks)
	}

	rt = &runAllRuntime{write: validSourceReport}
	service = NewService(root, WithRuntime(rt, runtimeRequest()))
	result, err = service.RunLoop(context.Background(), RunLoopRequest{StudyRef: "demo", DimensionRefs: []string{"01"}, SourceRefs: []string{"doc.md"}, Parallelism: 1, Reset: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != RunAllStatusCompleted {
		t.Fatalf("reset Status = %q counts = %+v", result.Status, result.Counts)
	}
	second, err := LoadRunState(st)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Tasks) != 3 || second.Tasks[0].Source != "doc.md" || second.Tasks[1].Source != "repo" {
		t.Fatalf("tasks = %#v, want fresh doc.md-only state", second.Tasks)
	}
	entries, err := os.ReadDir(filepath.Join(st.Path, RunStateDirName, "archive"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected archived prior run-state")
	}
}

func TestRunLoopSynthesizesDimensionAsSoonAsItsAnalysisCompletes(t *testing.T) {
	root, _ := executionFixture(t)
	writeReport(t, filepath.Join(root, "studies", "demo", "dimensions", "02-runtime.md"), "# Runtime\n")
	rt := &orderedRuntime{}
	service := NewService(root, WithRuntime(rt, runtimeRequest()))

	result, err := service.RunLoop(context.Background(), RunLoopRequest{StudyRef: "demo", DimensionRefs: []string{"01", "02"}, Parallelism: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != RunAllStatusCompleted {
		t.Fatalf("Status = %q counts = %+v", result.Status, result.Counts)
	}
	want := []string{
		"analysis:01-structure",
		"analysis:01-structure",
		"synthesis:01-structure",
		"analysis:02-runtime",
		"analysis:02-runtime",
		"synthesis:02-runtime",
	}
	if len(rt.order) != len(want) {
		t.Fatalf("order len = %d order = %#v", len(rt.order), rt.order)
	}
	for i := range want {
		if rt.order[i] != want[i] {
			t.Fatalf("order[%d] = %q, want %q; full order %#v", i, rt.order[i], want[i], rt.order)
		}
	}
}

func TestRunLoopCancellationDoesNotCancelUnscheduledTasks(t *testing.T) {
	root, st := executionFixture(t)
	rt := &runAllRuntime{write: validSourceReport}
	service := NewService(root, WithRuntime(rt, runtimeRequest()))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := service.RunLoop(ctx, RunLoopRequest{StudyRef: "demo", DimensionRefs: []string{"01"}, SourceRefs: []string{"repo", "doc.md"}, Parallelism: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != RunAllStatusCancelled {
		t.Fatalf("Status = %q", result.Status)
	}
	loaded, err := LoadRunState(st)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range loaded.Tasks {
		if task.Status != TaskStatusPending {
			t.Fatalf("task %s status = %s, want pending", task.ID, task.Status)
		}
	}
	records, err := LoadRunHistory(st)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("history records = %d, want 0", len(records))
	}
}

func TestRunLoopMarksSynthesisFailedWhenDependenciesTerminal(t *testing.T) {
	root, st := executionFixture(t)
	service := NewService(root, WithRuntime(failingRuntime{}, runtimeRequest()))

	result, err := service.RunLoop(context.Background(), RunLoopRequest{StudyRef: "demo", DimensionRefs: []string{"01"}, Parallelism: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != RunAllStatusValidationFailed {
		t.Fatalf("Status = %q counts = %+v", result.Status, result.Counts)
	}

	loaded, err := LoadRunState(st)
	if err != nil {
		t.Fatal(err)
	}
	var synthesis *TaskState
	for i := range loaded.Tasks {
		if loaded.Tasks[i].Kind == TaskKindSynthesis {
			synthesis = &loaded.Tasks[i]
			break
		}
	}
	if synthesis == nil {
		t.Fatal("synthesis task missing")
	}
	if synthesis.Status != TaskStatusFailed {
		t.Fatalf("synthesis status = %q, want failed", synthesis.Status)
	}
	if synthesis.LastError == nil || synthesis.LastError.Code != "synthesis.dependencies_failed" {
		t.Fatalf("synthesis last error = %#v", synthesis.LastError)
	}
}

type orderedRuntime struct {
	mu    sync.Mutex
	order []string
}

type failingRuntime struct{}

func (failingRuntime) StartRun(context.Context, runtimepkg.Request) (runtimepkg.Result, error) {
	return runtimepkg.Result{RunID: "failed-run", Status: "failed"}, errors.New("runtime unavailable")
}

func (r *orderedRuntime) StartRun(ctx context.Context, req runtimepkg.Request) (runtimepkg.Result, error) {
	if ctx == nil {
		panic("nil context")
	}
	kind := req.Metadata["task.kind"]
	dimension := req.Metadata["dimension.ref"]
	r.mu.Lock()
	r.order = append(r.order, kind+":"+dimension)
	r.mu.Unlock()
	content := validSourceReport
	if kind == string(TaskKindSynthesis) {
		content = validFinalReport
	}
	if req.Validation != nil && len(req.Validation.Expectations) > 0 {
		path := req.Validation.Expectations[0].Path
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			panic(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			panic(err)
		}
	}
	return runtimepkg.Result{RunID: "ordered-run", Status: "completed"}, nil
}

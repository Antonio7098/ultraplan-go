package study

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	runtimepkg "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

type runAllRuntime struct {
	mu        sync.Mutex
	calls     int
	active    int
	maxActive int
	write     string
	block     chan struct{}
}

func (r *runAllRuntime) StartRun(ctx context.Context, req runtimepkg.Request) (runtimepkg.Result, error) {
	r.mu.Lock()
	r.calls++
	r.active++
	if r.active > r.maxActive {
		r.maxActive = r.active
	}
	r.mu.Unlock()
	if r.block != nil {
		select {
		case <-r.block:
		case <-ctx.Done():
		}
	}
	if r.write != "" && req.Validation != nil && len(req.Validation.Expectations) > 0 {
		path := req.Validation.Expectations[0].Path
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			panic(err)
		}
		content := r.write
		if req.Metadata["task.kind"] == string(TaskKindSynthesis) {
			content = validFinalReport
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			panic(err)
		}
	}
	r.mu.Lock()
	r.active--
	r.mu.Unlock()
	return runtimepkg.Result{RunID: "run", Status: "completed"}, nil
}

func TestRunAllFiltersMatrixSynthesisSummary(t *testing.T) {
	root, st := executionFixture(t)
	rt := &runAllRuntime{write: validSourceReport}
	service := NewService(root, WithRuntime(rt, runtimeRequest()))

	result, err := service.RunAll(context.Background(), RunAllRequest{StudyRef: "demo", DimensionRefs: []string{"01"}, SourceRefs: []string{"repo", "doc.md", "other.md"}, Parallelism: 2})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != RunAllStatusCompleted {
		t.Fatalf("Status = %q result = %+v", result.Status, result)
	}
	if len(result.Analysis) != 2 {
		t.Fatalf("analysis count = %d", len(result.Analysis))
	}
	if rt.calls != 3 {
		t.Fatalf("runtime calls = %d, want 3 (2 analysis + synthesis)", rt.calls)
	}
	if result.Counts.Completed != 3 || result.Counts.Failed != 0 || result.Counts.Skipped != 0 {
		t.Fatalf("Counts = %+v", result.Counts)
	}
	if _, err := os.Stat(SummaryPath(st)); err != nil {
		t.Fatal(err)
	}
}

func TestRunAllPreflightFailuresStartNoRuntime(t *testing.T) {
	root, _ := executionFixture(t)
	rt := &runAllRuntime{write: validSourceReport}
	service := NewService(root, WithRuntime(rt, runtimeRequest()))

	if _, err := service.RunAll(context.Background(), RunAllRequest{StudyRef: "demo", DimensionRefs: []string{"missing"}, Parallelism: 1}); err == nil {
		t.Fatal("expected missing dimension error")
	}
	if rt.calls != 0 {
		t.Fatalf("runtime calls = %d", rt.calls)
	}
	if _, err := service.RunAll(context.Background(), RunAllRequest{StudyRef: "demo", Parallelism: 0}); err == nil {
		t.Fatal("expected invalid parallelism error")
	}
	if rt.calls != 0 {
		t.Fatalf("runtime calls after invalid parallelism = %d", rt.calls)
	}
}

func TestRunAllEmptyApplicableMatrixSkipsSynthesisAndRuntime(t *testing.T) {
	root, st := executionFixture(t)
	rt := &runAllRuntime{write: validSourceReport}
	service := NewService(root, WithRuntime(rt, runtimeRequest()))

	result, err := service.RunAll(context.Background(), RunAllRequest{StudyRef: "demo", DimensionRefs: []string{"01"}, SourceRefs: []string{"other.md"}, Parallelism: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != RunAllStatusCompleted {
		t.Fatalf("Status = %q result = %+v", result.Status, result)
	}
	if len(result.Analysis) != 0 || len(result.Synthesis) != 0 {
		t.Fatalf("analysis = %d synthesis = %d", len(result.Analysis), len(result.Synthesis))
	}
	if result.Counts != (RunAllCounts{}) {
		t.Fatalf("Counts = %+v", result.Counts)
	}
	if rt.calls != 0 {
		t.Fatalf("runtime calls = %d", rt.calls)
	}
	content, err := os.ReadFile(SummaryPath(st))
	if err != nil {
		t.Fatal(err)
	}
	want := "source,01-structure,total\nother.md,N/A,0\n"
	if string(content) != want {
		t.Fatalf("summary:\n%s\nwant:\n%s", content, want)
	}
}

func TestRunAllBoundsParallelismAndReportsCancellation(t *testing.T) {
	root, _ := executionFixture(t)
	block := make(chan struct{})
	rt := &runAllRuntime{write: validSourceReport, block: block}
	service := NewService(root, WithRuntime(rt, runtimeRequest()))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan RunAllResult, 1)
	go func() {
		result, _ := service.RunAll(ctx, RunAllRequest{StudyRef: "demo", Parallelism: 1})
		done <- result
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	close(block)
	result := <-done
	if rt.maxActive > 1 {
		t.Fatalf("maxActive = %d", rt.maxActive)
	}
	if result.Status != RunAllStatusCancelled {
		t.Fatalf("Status = %q", result.Status)
	}
}

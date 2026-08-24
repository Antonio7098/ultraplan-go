package study

import (
	"context"
	"testing"

	"github.com/Antonio7098/ultraplan-go/internal/platform/gitpublish"
)

type recordingPublisher struct {
	requests []gitpublish.Request
}

func (p *recordingPublisher) Publish(_ context.Context, req gitpublish.Request) (gitpublish.Result, error) {
	p.requests = append(p.requests, req)
	return gitpublish.Result{Repository: req.Root, Branch: "main", Commit: "abc", Committed: true}, nil
}

func TestPublishExecutionIncludesReportAndRunStatePaths(t *testing.T) {
	publisher := &recordingPublisher{}
	service := NewService(t.TempDir(), WithPublisher(publisher))
	result := ExecutionResult{
		Status: ExecutionStatusCompleted, TaskKind: TaskKindAnalysis,
		Study:     Study{Name: "demo", Path: "/workspace/studies/demo"},
		Dimension: Dimension{Number: "02", Slug: "runtime"}, Source: Source{Name: "source"},
		OutputPath: "/workspace/studies/demo/reports/source/02-runtime.md",
	}

	result, err := service.publishExecution(context.Background(), result, "/workspace/studies/demo/.ultraplan/run-state.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Publications) != 1 || len(publisher.requests) != 1 {
		t.Fatalf("result=%#v requests=%#v", result, publisher.requests)
	}
	request := publisher.requests[0]
	if len(request.Paths) != 2 || request.Paths[0] != result.OutputPath || request.Identity != "study/demo/analysis/02-runtime/source" {
		t.Fatalf("request = %#v", request)
	}
}

func TestPublishExecutionIgnoresFailedStage(t *testing.T) {
	publisher := &recordingPublisher{}
	service := NewService(t.TempDir(), WithPublisher(publisher))
	result := ExecutionResult{Status: ExecutionStatusValidationFailed, Study: Study{Name: "demo", Path: "/workspace/studies/demo"}}
	if _, err := service.publishExecution(context.Background(), result); err != nil {
		t.Fatal(err)
	}
	if len(publisher.requests) != 0 {
		t.Fatalf("failed stage published: %#v", publisher.requests)
	}
}

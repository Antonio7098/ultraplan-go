package sprint

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Antonio7098/ultraplan-go/internal/platform/gitpublish"
)

type recordingPublisher struct {
	requests []gitpublish.Request
	results  []gitpublish.Result
	err      error
}

func (p *recordingPublisher) Publish(_ context.Context, req gitpublish.Request) (gitpublish.Result, error) {
	p.requests = append(p.requests, req)
	result := gitpublish.Result{Repository: req.Root, Branch: "main", Commit: "abc", Committed: true}
	if len(p.results) >= len(p.requests) {
		result = p.results[len(p.requests)-1]
	}
	return result, p.err
}

func TestPublishPlanningStageUsesOwnedWorkspacePaths(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	publisher := &recordingPublisher{}
	service := NewService(root).WithPublisher(publisher)

	results, err := service.publishPlanningStage(context.Background(), "proj", "01", StageRequirements)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || len(publisher.requests) != 1 {
		t.Fatalf("results=%#v requests=%#v", results, publisher.requests)
	}
	req := publisher.requests[0]
	if req.Root != root || req.All {
		t.Fatalf("request = %#v", req)
	}
	want := map[string]bool{
		filepath.Join(sp.Path, "requirements.md"): true,
		filepath.Join(sp.Path, "flow-state.json"): true,
	}
	for _, path := range req.Paths {
		delete(want, path)
	}
	if len(want) != 0 || len(req.Paths) != 2 {
		t.Fatalf("paths = %#v, missing = %#v", req.Paths, want)
	}
}

func TestPublishExecuteUsesTargetBeforeWorkspace(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	publisher := &recordingPublisher{}
	service := NewService(root).WithPublisher(publisher)
	target := ExecuteTargetRef{Path: filepath.Join(t.TempDir(), "target")}

	results, err := service.publishExecuteStage(context.Background(), sp, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || len(publisher.requests) != 2 {
		t.Fatalf("results=%#v requests=%#v", results, publisher.requests)
	}
	if publisher.requests[0].Root != target.Path || !publisher.requests[0].All {
		t.Fatalf("target request = %#v", publisher.requests[0])
	}
	if publisher.requests[1].Root != root || publisher.requests[1].All {
		t.Fatalf("workspace request = %#v", publisher.requests[1])
	}
}

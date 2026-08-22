package app

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

var testTUIRunner TUIRunner
var testSprintRuntimeFactory SprintRuntimeFactory

func TestTUICommandHelpAndRunner(t *testing.T) {
	stdout, stderr, status := runForTest([]string{"tui", "--help"})
	if status != ExitOK {
		t.Fatalf("status = %d stderr = %q", status, stderr)
	}
	assertContains(t, stdout, "operational terminal dashboard")
	assertContains(t, stdout, "Every sprint status, validation, prompt, flow, execute, and review")
	assertContains(t, stdout, "flow-state.json")

	dir := initializedWorkspace(t)
	called := false
	testTUIRunner = func(ctx context.Context, opts TUIRunOptions) error {
		called = true
		if opts.UseCases == nil {
			t.Fatalf("missing use cases")
		}
		_, _ = fmt.Fprint(opts.Stdout, "tui started\n")
		return nil
	}
	defer func() { testTUIRunner = nil }()

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "tui"})
	if status != ExitOK {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	if !called {
		t.Fatalf("runner not called")
	}
	assertContains(t, stdout, "tui started")
}

func TestTUICommandInvalidWorkspace(t *testing.T) {
	_, stderr, status := runForTest([]string{"--workspace", "/definitely/missing/ultraplan", "tui"})
	if status != ExitWorkspace {
		t.Fatalf("status = %d stderr = %q", status, stderr)
	}
	assertContains(t, stderr, "workspace.discover")
}

func TestTUISprintFlowRunsAndStreamsProgress(t *testing.T) {
	dir := initializedWorkspace(t)
	writeCommandSprintProject(t, dir, "proj", "01-alpha")
	writeFixtureFileContent(t, dir, commandProjectIndex(t), "projects", "proj", "project-index.md")
	fake := &sprintCommandRuntime{}
	restoreRuntime := stubSprintRuntimeFactory(fake)
	defer restoreRuntime()

	var events []OperationEvent
	testTUIRunner = func(ctx context.Context, opts TUIRunOptions) error {
		req := OperationRequest{Kind: OperationFlow, Project: "proj", Sprint: "01", Stage: "requirements"}
		confirmation, err := opts.UseCases.PrepareOperation(ctx, req)
		if err != nil {
			return err
		}
		if !confirmation.Runtime || !confirmation.Mutates {
			t.Fatalf("flow confirmation = %+v", confirmation)
		}
		result, err := opts.UseCases.RunOperation(ctx, confirmation.Request, func(event OperationEvent) {
			events = append(events, event)
		})
		if err != nil {
			return err
		}
		if result.State != OperationComplete || !strings.Contains(result.Message, "requirements complete") {
			t.Fatalf("flow result = %+v", result)
		}
		return nil
	}
	defer func() { testTUIRunner = nil }()

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "tui"})
	if status != ExitOK {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	joined := ""
	for _, event := range events {
		joined += event.Stage + " " + event.Message + "\n"
	}
	for _, want := range []string{"operation started", "checking: checking prerequisites", "running: starting runtime-backed stage", "lifecycle.transition state=running", "complete: requirements complete", "operation complete"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("TUI flow progress missing %q:\n%s", want, joined)
		}
	}
	if fake.calls != 1 {
		t.Fatalf("runtime calls=%d, want 1", fake.calls)
	}
}

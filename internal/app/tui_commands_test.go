package app

import (
	"context"
	"fmt"
	"testing"
)

func TestTUICommandHelpAndRunner(t *testing.T) {
	stdout, stderr, status := runForTest([]string{"tui", "--help"})
	if status != ExitOK {
		t.Fatalf("status = %d stderr = %q", status, stderr)
	}
	assertContains(t, stdout, "read-only terminal dashboard")
	assertContains(t, stdout, "flow-state.json")

	dir := initializedWorkspace(t)
	called := false
	SetTUIRunner(func(ctx context.Context, opts TUIRunOptions) error {
		called = true
		if opts.UseCases == nil {
			t.Fatalf("missing use cases")
		}
		_, _ = fmt.Fprint(opts.Stdout, "tui started\n")
		return nil
	})
	defer SetTUIRunner(nil)

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

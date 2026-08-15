package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestServeHelpIsLazyAndDocumentsContract(t *testing.T) {
	called := false
	var stdout, stderr bytes.Buffer
	status := Run(Config{
		Args: []string{"serve", "--help"}, Stdout: &stdout, Stderr: &stderr,
		WebRunner: func(context.Context, ServeRunOptions) error { called = true; return nil },
	})
	if status != ExitOK || called {
		t.Fatalf("status=%d called=%v stderr=%q", status, called, stderr.String())
	}
	for _, want := range []string{"--listen", "127.0.0.1:8080", "--open-browser", "--workspace", "shuts", "read-only"} {
		assertContains(t, stdout.String(), want)
	}
}

func TestServePreflightAndRunnerOptions(t *testing.T) {
	root := initializedWorkspace(t)
	var got ServeRunOptions
	var stdout, stderr bytes.Buffer
	status := Run(Config{
		Args:   []string{"serve", "--workspace", root, "--listen", "[::1]:9090", "--open-browser"},
		Stdout: &stdout, Stderr: &stderr,
		WebRunner: func(_ context.Context, opts ServeRunOptions) error {
			got = opts
			return nil
		},
	})
	if status != ExitOK {
		t.Fatalf("status=%d stderr=%q", status, stderr.String())
	}
	if got.Listen != "[::1]:9090" || !got.OpenBrowser || got.UseCases == nil || got.Stdout != &stdout || got.Diagnostics != &stderr {
		t.Fatalf("runner options = %+v", got)
	}
	health, err := got.UseCases.Health(context.Background())
	if err != nil || !health.Workspace {
		t.Fatalf("health=%+v err=%v", health, err)
	}
}

func TestServeListenValidationRunsBeforeWorkspaceAndRunner(t *testing.T) {
	for _, value := range []string{"localhost:8080", "0.0.0.0:8080", "192.0.2.1:8080", "127.0.0.1", "127.0.0.1:0", "[::1]", "[fe80::1]:8080", " 127.0.0.1:8080"} {
		t.Run(value, func(t *testing.T) {
			called := false
			var stderr bytes.Buffer
			status := Run(Config{
				Args: []string{"serve", "--listen", value}, Stderr: &stderr,
				WebRunner: func(context.Context, ServeRunOptions) error { called = true; return nil },
			})
			if status != ExitUsage || called {
				t.Fatalf("status=%d called=%v stderr=%q", status, called, stderr.String())
			}
			assertContains(t, stderr.String(), "serve.listen")
		})
	}
	for _, value := range []string{"127.0.0.1:1", "127.42.0.8:65535", "[::1]:8080"} {
		if err := ValidateLoopbackListen(value); err != nil {
			t.Errorf("%q rejected: %v", value, err)
		}
	}
}

func TestServeWorkspaceDiscoveryAndStartupFailure(t *testing.T) {
	root := initializedWorkspace(t)
	nested := filepath.Join(root, "projects", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("listener failed")
	var stderr bytes.Buffer
	status := Run(Config{
		Args: []string{"serve"}, WorkDir: nested, Stderr: &stderr,
		WebRunner: func(_ context.Context, opts ServeRunOptions) error {
			if opts.UseCases == nil {
				t.Fatal("missing queries")
			}
			return sentinel
		},
	})
	if status != ExitError {
		t.Fatalf("status=%d stderr=%q", status, stderr.String())
	}
	assertContains(t, stderr.String(), "serve.start")
	assertContains(t, stderr.String(), "listener failed")
}

func TestServeCancellationIsClean(t *testing.T) {
	root := initializedWorkspace(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stderr bytes.Buffer
	status := Run(Config{
		Args: []string{"--workspace", root, "serve"}, Context: ctx, Stderr: &stderr,
		WebRunner: func(ctx context.Context, _ ServeRunOptions) error { return ctx.Err() },
	})
	if status != ExitOK || stderr.Len() != 0 {
		t.Fatalf("status=%d stderr=%q", status, stderr.String())
	}
}

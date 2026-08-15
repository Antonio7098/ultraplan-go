package web

import (
	"bytes"
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/app"
)

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *syncBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(data)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func (b *syncBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Len()
}

func TestServerLifecycleCanonicalURLLauncherWarningAndShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	listening := make(chan struct{})
	listen := func(network, _ string) (net.Listener, error) {
		ln, err := net.Listen(network, "127.0.0.1:0")
		if err == nil {
			close(listening)
		}
		return ln, err
	}
	var stdout syncBuffer
	var diagnostics bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{
			Listen: "127.0.0.1:8080", Queries: sampleQueries(), Stdout: &stdout, Diagnostics: &diagnostics,
			ListenFunc: listen, OpenBrowser: true,
			LaunchBrowser: func(context.Context, string) error { return context.DeadlineExceeded },
		})
	}()
	select {
	case <-listening:
	case <-time.After(2 * time.Second):
		t.Fatal("listener was not acquired")
	}
	deadline := time.Now().Add(2 * time.Second)
	for stdout.Len() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !strings.HasPrefix(stdout.String(), "Dashboard: http://127.0.0.1:") || !strings.HasSuffix(stdout.String(), "/\n") {
		t.Fatalf("stdout=%q", stdout.String())
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not shut down")
	}
	for _, want := range []string{"event=server_started", "warning=launch_failed", "event=server_stopped"} {
		if !strings.Contains(diagnostics.String(), want) {
			t.Errorf("diagnostics missing %q: %s", want, diagnostics.String())
		}
	}
}

func TestServerListenFailureAndPolicyRecheck(t *testing.T) {
	err := Run(context.Background(), Options{
		Listen: "0.0.0.0:8080", Queries: sampleQueries(),
	})
	if err == nil || !strings.Contains(err.Error(), "validate listen address") {
		t.Fatalf("non-loopback err=%v", err)
	}
	err = Run(context.Background(), Options{
		Listen: "127.0.0.1:8080", Queries: sampleQueries(),
		ListenFunc: func(string, string) (net.Listener, error) { return nil, net.ErrClosed },
	})
	if err == nil || !strings.Contains(err.Error(), "listen on configured loopback") {
		t.Fatalf("listen err=%v", err)
	}
}

func TestHealthUnavailableIsTruthfulAndCheap(t *testing.T) {
	queries := sampleQueries()
	queries.health = app.WebHealthResult{Status: "unavailable", Server: true, Workspace: false}
	h := testHandler(t, queries, nil)
	res := request(h, "GET", "/api/v1/health", nil)
	if res.Code != 503 || !strings.Contains(res.Body.String(), `"status":"unavailable"`) || queries.healthCalls != 1 {
		t.Fatalf("status=%d calls=%d body=%s", res.Code, queries.healthCalls, res.Body.String())
	}
}

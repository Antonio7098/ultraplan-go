package web

import (
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestPackagedBinaryServesEmbeddedAssetsOutsideSourceTree(t *testing.T) {
	if testing.Short() {
		t.Skip("outside-tree binary packaging check")
	}
	temp := t.TempDir()
	binary := filepath.Join(temp, "ultraplan")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/ultraplan")
	build.Dir = filepath.Join("..", "..")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build packaged binary: %v\n%s", err, output)
	}
	workspace := filepath.Join(temp, "workspace")
	initialize := exec.Command(binary, "init-workspace", "--path", workspace)
	initialize.Dir = temp
	if output, err := initialize.CombinedOutput(); err != nil {
		t.Fatalf("initialize packaged workspace: %v\n%s", err, output)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	authority := listener.Addr().String()
	_ = listener.Close()

	var stdout, stderr syncBuffer
	command := exec.Command(binary, "--workspace", workspace, "serve", "--listen", authority)
	command.Dir = temp
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if command.ProcessState == nil || !command.ProcessState.Exited() {
			_ = command.Process.Kill()
			_, _ = command.Process.Wait()
		}
	})

	baseURL := "http://" + authority
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(5 * time.Second)
	for {
		response, requestErr := client.Get(baseURL + "/api/v1/health")
		if requestErr == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("packaged server did not become ready: %v stdout=%s stderr=%s", requestErr, stdout.String(), stderr.String())
		}
		time.Sleep(20 * time.Millisecond)
	}

	paths := []string{"/", "/projects", "/studies", "/api/v1/dashboard", "/api/v1/projects", "/api/v1/studies", "/api/v1/health", "/static/app.css", "/static/app.js", "/static/css/tokens.css", "/static/css/base.css", "/static/css/primitives.css", "/static/css/components.css", "/static/css/layouts.css", "/static/css/utilities.css", "/static/js/app.js", "/static/js/operations.js", "/static/js/sse.js"}
	for _, path := range paths {
		response, err := client.Get(baseURL + path)
		if err != nil {
			t.Fatalf("request %s: %v", path, err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Errorf("%s status=%d", path, response.StatusCode)
		}
	}
	if strings.Contains(stdout.String()+stderr.String(), filepath.Join("internal", "web", "static")) {
		t.Fatalf("runtime output exposed source-tree assets: stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if err := command.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("packaged server shutdown: %v stderr=%s", err, stderr.String())
	}
}

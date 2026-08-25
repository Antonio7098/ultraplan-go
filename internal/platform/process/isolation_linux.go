//go:build linux

package process

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

var linuxIsolationProbe struct {
	sync.Once
	path string
	ok   bool
}

func isolationCapabilities() IsolationCapabilities {
	path, ok := bubblewrapCapability()
	_ = path
	return IsolationCapabilities{
		PrivateWorkspace:        true,
		ContainedCopy:           true,
		ProcessGroup:            true,
		DescendantCleanup:       true,
		WorkspaceRemoval:        true,
		NativeProtectedRootDeny: ok,
	}
}

func bubblewrapCapability() (string, bool) {
	linuxIsolationProbe.Do(func() {
		path, err := exec.LookPath("bwrap")
		if err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, path, "--ro-bind", "/", "/", "--unshare-pid", "--die-with-parent", "--", "/bin/true")
		if err := cmd.Run(); err == nil {
			linuxIsolationProbe.path, linuxIsolationProbe.ok = path, true
		}
	})
	return linuxIsolationProbe.path, linuxIsolationProbe.ok
}

func nativeIsolationRequest(workspace, dir string, req Request) (Request, error) {
	path, ok := bubblewrapCapability()
	if !ok {
		return Request{}, fmt.Errorf("native protected-root isolation is unavailable")
	}
	args := []string{"--ro-bind", "/", "/", "--bind", workspace, workspace, "--unshare-pid", "--die-with-parent", "--chdir", dir, "--", req.Executable}
	args = append(args, req.Args...)
	req.Executable = path
	req.Args = args
	req.Dir = workspace
	return req, nil
}

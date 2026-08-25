//go:build !linux && !darwin

package process

import "os"

func linkCount(os.FileInfo) uint64 { return 1 }

func isolationCapabilities() IsolationCapabilities {
	return IsolationCapabilities{PrivateWorkspace: true, ContainedCopy: true, WorkspaceRemoval: true}
}

func nativeIsolationRequest(_, _ string, req Request) (Request, error) { return req, nil }

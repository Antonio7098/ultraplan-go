//go:build darwin

package process

func isolationCapabilities() IsolationCapabilities {
	return IsolationCapabilities{PrivateWorkspace: true, ContainedCopy: true, ProcessGroup: true, DescendantCleanup: true, WorkspaceRemoval: true}
}

func nativeIsolationRequest(_, _ string, req Request) (Request, error) { return req, nil }

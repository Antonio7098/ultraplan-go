//go:build !linux && !darwin

package runcontrol

// Platforms without flock keep process-local maintenance behavior. SQLite
// still serializes the writes, and the application retry policy handles busy
// results from another process.
type maintenanceLock struct{}

func tryMaintenanceLock(string) (*maintenanceLock, bool, error) {
	return &maintenanceLock{}, true, nil
}

func (*maintenanceLock) release() {}

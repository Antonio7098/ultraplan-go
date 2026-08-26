package runcontrol

import (
	"context"
	"path/filepath"
)

const maintenanceLockName = "run-control-maintenance.lock"

// Maintain lets at most one process reconcile and compact a workspace at a
// time. A busy lock means another process owns this pass, so callers can skip
// it without treating healthy coordination as a persistence failure.
func (r *SQLiteRepository) Maintain(ctx context.Context, probe ProcessProbe) (bool, error) {
	lock, acquired, err := tryMaintenanceLock(filepath.Join(filepath.Dir(r.path), maintenanceLockName))
	if err != nil || !acquired {
		return acquired, err
	}
	defer lock.release()
	if _, err := r.Reconcile(ctx, probe, ReconcileOptions{}); err != nil {
		return true, err
	}
	if _, err := r.Compact(ctx, 64); err != nil {
		return true, err
	}
	return true, nil
}

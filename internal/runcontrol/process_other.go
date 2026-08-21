//go:build !linux && !darwin

package runcontrol

import (
	"context"
	"fmt"
)

func probeNativeProcess(ctx context.Context, pid int) (ProcessIdentity, error) {
	if err := validateProbePID(pid); err != nil {
		return ProcessIdentity{}, err
	}
	if err := ctx.Err(); err != nil {
		return ProcessIdentity{}, err
	}
	return ProcessIdentity{HostDigest: localHostDigest(), PID: pid}, fmt.Errorf("%w: platform process-birth probe unavailable", ErrProcessIdentityUnavailable)
}

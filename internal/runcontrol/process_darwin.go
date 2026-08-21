//go:build darwin

package runcontrol

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

func probeNativeProcess(ctx context.Context, pid int) (ProcessIdentity, error) {
	if err := validateProbePID(pid); err != nil {
		return ProcessIdentity{}, err
	}
	if err := ctx.Err(); err != nil {
		return ProcessIdentity{}, err
	}
	process, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		if errors.Is(err, unix.ESRCH) {
			return ProcessIdentity{}, ErrProcessNotFound
		}
		return ProcessIdentity{HostDigest: localHostDigest(), PID: pid}, fmt.Errorf("%w: read darwin process metadata: %v", ErrProcessIdentityUnavailable, err)
	}
	if process == nil || process.Proc.P_pid != int32(pid) || process.Proc.P_starttime.Sec <= 0 {
		return ProcessIdentity{HostDigest: localHostDigest(), PID: pid}, fmt.Errorf("%w: darwin process start time unavailable", ErrProcessIdentityUnavailable)
	}
	boot, err := unix.SysctlTimeval("kern.boottime")
	if err != nil || boot == nil || boot.Sec <= 0 {
		return ProcessIdentity{HostDigest: localHostDigest(), PID: pid}, fmt.Errorf("%w: darwin boot identity unavailable", ErrProcessIdentityUnavailable)
	}
	return ProcessIdentity{
		HostDigest: localHostDigest(), PID: pid,
		BootID:     fmt.Sprintf("%d.%06d", boot.Sec, boot.Usec),
		BirthToken: fmt.Sprintf("%d.%06d", process.Proc.P_starttime.Sec, process.Proc.P_starttime.Usec),
	}, nil
}

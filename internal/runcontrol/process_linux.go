//go:build linux

package runcontrol

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func probeNativeProcess(ctx context.Context, pid int) (ProcessIdentity, error) {
	if err := validateProbePID(pid); err != nil {
		return ProcessIdentity{}, err
	}
	if err := ctx.Err(); err != nil {
		return ProcessIdentity{}, err
	}
	stat, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if errors.Is(err, fs.ErrNotExist) {
		return ProcessIdentity{}, ErrProcessNotFound
	}
	if err != nil {
		return ProcessIdentity{}, fmt.Errorf("%w: read process stat: %v", ErrProcessIdentityUnavailable, err)
	}
	closeParen := strings.LastIndexByte(string(stat), ')')
	if closeParen < 0 {
		return ProcessIdentity{}, fmt.Errorf("%w: malformed process stat", ErrProcessIdentityUnavailable)
	}
	fields := strings.Fields(string(stat)[closeParen+1:])
	if len(fields) <= 19 || fields[19] == "" {
		return ProcessIdentity{}, fmt.Errorf("%w: process start token missing", ErrProcessIdentityUnavailable)
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil || strings.TrimSpace(string(boot)) == "" {
		return ProcessIdentity{}, fmt.Errorf("%w: boot identity missing", ErrProcessIdentityUnavailable)
	}
	return ProcessIdentity{
		HostDigest: localHostDigest(), BootID: strings.TrimSpace(string(boot)), PID: pid, BirthToken: fields[19],
	}, nil
}

//go:build linux || darwin

package process

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

func configureOwnedProcess(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

func stopAndWait(cmd *exec.Cmd, waited <-chan error, grace time.Duration) (error, bool) {
	if cmd.Process == nil {
		return nil, true
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		return <-waited, false
	}
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case err := <-waited:
		// The group leader may exit before one of its descendants observes TERM.
		// A final group kill is harmless for already-exited members and closes that race.
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		return err, true
	case <-timer.C:
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		return <-waited, true
	}
}

func processSignal(state *os.ProcessState) string {
	if status, ok := state.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return status.Signal().String()
	}
	return ""
}

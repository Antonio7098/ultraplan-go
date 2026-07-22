//go:build !linux && !darwin

package process

import (
	"os"
	"os/exec"
	"time"
)

func configureOwnedProcess(*exec.Cmd) {}

func stopAndWait(cmd *exec.Cmd, waited <-chan error, grace time.Duration) (error, bool) {
	if cmd.Process == nil {
		return nil, true
	}
	_ = cmd.Process.Kill()
	select {
	case err := <-waited:
		return err, true
	case <-time.After(grace):
		return nil, false
	}
}

func processSignal(*os.ProcessState) string { return "" }

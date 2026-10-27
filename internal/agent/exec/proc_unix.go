//go:build !windows

package exec

import (
	osexec "os/exec"
	"syscall"
)

// configureProcess runs the script in its own process group so a timeout
// kills everything it spawned, not just the interpreter.
func configureProcess(cmd *osexec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

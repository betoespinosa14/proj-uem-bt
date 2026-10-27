package exec

import (
	osexec "os/exec"
	"syscall"
)

func configureProcess(cmd *osexec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

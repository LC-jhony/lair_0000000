//go:build !windows

package project

import (
	"os/exec"
	"syscall"
)

// setProcessGroup isolates `laravel new` and its Composer/PHP children so a
// cancellation reaches the whole tree, not just the top-level process.
func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

//go:build !windows

package service

import (
	"os/exec"
	"syscall"
)

// hideWindow puts the daemon in its own process group, so a terminal's
// Ctrl-C reaches only sonicpid (which then shuts the server down cleanly)
// and killTree can reach everything the daemon started.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	cmd.Process.Kill()
}

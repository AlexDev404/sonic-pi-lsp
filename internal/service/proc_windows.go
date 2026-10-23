//go:build windows

package service

import (
	"os/exec"
	"strconv"
	"syscall"
)

const createNoWindow = 0x08000000

// hideWindow stops the daemon (and the processes it starts) opening
// console windows.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow | syscall.CREATE_NEW_PROCESS_GROUP}
}

func killTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	kill.Run()
	cmd.Process.Kill()
}

// Package svcmgr installs and controls the background service with the
// platform's service manager: a systemd user unit on Linux and a Windows
// service on Windows. When neither is available the service is started as
// an ordinary background process.
package svcmgr

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Name is the service's name with the service manager.
const Name = "sonicpid"

// ErrNoManager means the platform's service manager cannot be used.
var ErrNoManager = errors.New("no service manager available")

// DaemonExe finds the sonicpid executable: next to the running executable
// first, then on PATH.
func DaemonExe() (string, error) {
	name := "sonicpid"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if exe, err := os.Executable(); err == nil {
		if r, err := filepath.EvalSymlinks(exe); err == nil {
			exe = r
		}
		if filepath.Base(exe) == name {
			return exe, nil
		}
		p := filepath.Join(filepath.Dir(exe), name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return exec.LookPath(name)
}

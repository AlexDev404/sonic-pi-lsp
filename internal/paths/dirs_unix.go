//go:build !windows

package paths

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
)

// StateDirs lists where a running service publishes its state file, in the
// order the CLI looks.
func StateDirs() []string {
	if v := os.Getenv("SONICPI_STATE_DIR"); v != "" {
		return []string{v}
	}
	if v := os.Getenv("XDG_RUNTIME_DIR"); v != "" {
		return []string{filepath.Join(v, "sonic-pi-cli")}
	}
	return []string{filepath.Join(os.TempDir(), fmt.Sprintf("sonic-pi-cli-%d", os.Getuid()))}
}

// StateDir is where this process publishes its state.
func StateDir() string { return StateDirs()[0] }

func userName() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	if v := os.Getenv("USER"); v != "" {
		return v
	}
	return fmt.Sprint(os.Getuid())
}

func writable(dir string) bool {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return false
	}
	f, err := os.CreateTemp(dir, ".probe")
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(f.Name())
	return true
}

// LogDir is where the service writes the session log: /var/log/sonic-pi/<user>
// (the .deb creates /var/log/sonic-pi for this), falling back to
// $XDG_STATE_HOME/sonic-pi-cli/log when /var/log is not writable.
func LogDir() string {
	if v := os.Getenv("SONICPI_LOG_DIR"); v != "" {
		return v
	}
	sys := filepath.Join("/var/log/sonic-pi", userName())
	if writable(sys) {
		return sys
	}
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		state = filepath.Join(UserDir(), ".local", "state")
	}
	return filepath.Join(state, "sonic-pi-cli", "log")
}

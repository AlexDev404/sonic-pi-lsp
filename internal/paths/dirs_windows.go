//go:build windows

package paths

import (
	"os"
	"path/filepath"
)

// StateDirs lists where a running service publishes its state file, in the
// order the CLI looks: the per-user directory (a service started on demand
// by the CLI) and then ProgramData (the Windows service).
func StateDirs() []string {
	if v := os.Getenv("SONICPI_STATE_DIR"); v != "" {
		return []string{v}
	}
	var dirs []string
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		dirs = append(dirs, filepath.Join(v, "sonic-pi-cli"))
	}
	pd := os.Getenv("ProgramData")
	if pd == "" {
		pd = `C:\ProgramData`
	}
	return append(dirs, filepath.Join(pd, "sonic-pi-cli"))
}

// StateDir is where this process publishes its state.
func StateDir() string { return StateDirs()[0] }

// LogDir is where the service writes the session log: %TEMP%\sonic-pi.
func LogDir() string {
	if v := os.Getenv("SONICPI_LOG_DIR"); v != "" {
		return v
	}
	return filepath.Join(os.TempDir(), "sonic-pi")
}

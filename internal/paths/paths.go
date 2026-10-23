// Package paths finds the files the CLI and the service share with the
// desktop client: the ~/.sonic-pi tree, the bundled Sonic Pi server, the
// linter, the service's state file and the log directory.
package paths

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

// UserDir mirrors SonicPi::Paths.user_dir in the Ruby server, so the CLI,
// the service and the desktop client all agree on where ~/.sonic-pi lives.
func UserDir() string {
	if v := os.Getenv("SONIC_PI_HOME"); v != "" {
		return filepath.Clean(v)
	}
	if runtime.GOOS == "windows" {
		if v := os.Getenv("USERPROFILE"); v != "" {
			return filepath.Clean(v)
		}
		drive, path := os.Getenv("HOMEDRIVE"), os.Getenv("HOMEPATH")
		if drive != "" && path != "" {
			return filepath.Clean(drive + path)
		}
	}
	if v := os.Getenv("HOME"); v != "" {
		return filepath.Clean(v)
	}
	h, _ := os.UserHomeDir()
	return h
}

// SonicPiHome is ~/.sonic-pi.
func SonicPiHome() string { return filepath.Join(UserDir(), ".sonic-pi") }

// ConfigDir is ~/.sonic-pi/config, the desktop client's configuration.
func ConfigDir() string { return filepath.Join(SonicPiHome(), "config") }

// GUISettingsPath is the desktop client's preferences file.
func GUISettingsPath() string { return filepath.Join(ConfigDir(), "gui-settings.ini") }

// AudioSettingsPath is scsynth's configuration, read by the boot daemon.
func AudioSettingsPath() string { return filepath.Join(ConfigDir(), "audio-settings.toml") }

// InitPath is the code run when the server boots.
func InitPath() string { return filepath.Join(ConfigDir(), "init.rb") }

// ColourThemePath is the desktop client's theme overrides.
func ColourThemePath() string { return filepath.Join(ConfigDir(), "colour-theme.properties") }

// ShortcutsPath is the desktop client's keyboard shortcuts.
func ShortcutsPath() string { return filepath.Join(ConfigDir(), "keyboard-shortcuts.ini") }

// TauSettingsPath is Tau's configuration, read by the boot daemon.
func TauSettingsPath() string { return filepath.Join(ConfigDir(), "tau-settings.toml") }

// ServerLogDir is where the Ruby server, scsynth and Tau write their logs.
func ServerLogDir() string { return filepath.Join(SonicPiHome(), "log") }

// StoreDir holds the desktop client's ten buffers.
func StoreDir() string { return filepath.Join(SonicPiHome(), "store", "default") }

// RecordingsDir is where `sonicpi record` puts recordings it names itself.
func RecordingsDir() string { return filepath.Join(SonicPiHome(), "recordings") }

func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return filepath.Dir(exe)
}

func isRoot(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "app", "server", "ruby", "bin", "daemon.rb"))
	return err == nil
}

// walkUp calls fn on dir and each of its parents until fn returns true.
func walkUp(dir string, fn func(string) bool) bool {
	for dir != "" {
		if fn(dir) {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
	return false
}

// ErrNoRoot is returned when no Sonic Pi server can be found.
var ErrNoRoot = errors.New("cannot find the Sonic Pi server: set SONICPI_ROOT to a Sonic Pi checkout or install")

// Root finds the bundled Sonic Pi tree (the one holding app/server). It
// looks at $SONICPI_ROOT, next to the executable (installed layout), and
// finally walks up to a checkout's external/sonic-pi (development layout).
func Root() (string, error) {
	if v := os.Getenv("SONICPI_ROOT"); v != "" {
		if isRoot(v) {
			return filepath.Abs(v)
		}
		return "", errors.New("SONICPI_ROOT does not point at a Sonic Pi tree: " + v)
	}
	var found string
	for _, start := range []string{exeDir(), cwd()} {
		if start == "" {
			continue
		}
		if walkUp(start, func(d string) bool {
			for _, c := range []string{filepath.Join(d, "sonic-pi"), filepath.Join(d, "external", "sonic-pi")} {
				if isRoot(c) {
					found = c
					return true
				}
			}
			return false
		}) {
			return found, nil
		}
	}
	return "", ErrNoRoot
}

func cwd() string {
	d, _ := os.Getwd()
	return d
}

// LintDir finds the linter (the directory holding splint.rb).
func LintDir() (string, error) {
	if v := os.Getenv("SONICPI_LINT_DIR"); v != "" {
		return v, nil
	}
	var found string
	for _, start := range []string{exeDir(), cwd()} {
		if start == "" {
			continue
		}
		if walkUp(start, func(d string) bool {
			c := filepath.Join(d, "lint")
			if _, err := os.Stat(filepath.Join(c, "splint.rb")); err == nil {
				found = c
				return true
			}
			return false
		}) {
			return found, nil
		}
	}
	return "", errors.New("cannot find the linter (lint/splint.rb): set SONICPI_LINT_DIR")
}

// Ruby returns the Ruby interpreter to run Sonic Pi with. Like the server
// itself, it prefers the Ruby bundled under app/server/native/ruby.
func Ruby(root string) string {
	if v := os.Getenv("SONICPI_RUBY"); v != "" {
		return v
	}
	name := "ruby"
	if runtime.GOOS == "windows" {
		name = "ruby.exe"
	}
	if root != "" {
		p := filepath.Join(root, "app", "server", "native", "ruby", "bin", name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return name
}

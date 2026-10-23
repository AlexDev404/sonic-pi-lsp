// Package paths finds the files the CLI and the service share with the
// desktop client: the ~/.sonic-pi tree, the bundled Sonic Pi server, the
// linter, the service's state file and the log directory.
package paths

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
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

// Layout names the desktop's configuration files, which Sonic Pi 5 renamed
// with a v5- prefix so they do not clash with a 4.x install's.
type Layout struct {
	Major     int
	GUI       string
	Audio     string
	Theme     string
	Shortcuts string
}

// LayoutFor returns the file names for a Sonic Pi major version.
func LayoutFor(major int) Layout {
	if major >= 5 {
		return Layout{major, "v5-gui-settings.ini", "v5-audio-settings.toml", "v5-colour-theme.properties", "v5-keyboard-shortcuts.ini"}
	}
	return Layout{major, "gui-settings.ini", "audio-settings.toml", "colour-theme.properties", "keyboard-shortcuts.ini"}
}

var (
	layoutOnce sync.Once
	layout     Layout
)

// CurrentLayout is the layout of the bundled Sonic Pi (Sonic Pi 5's when
// none can be found).
func CurrentLayout() Layout {
	layoutOnce.Do(func() {
		major := 5
		if root, err := Root(); err == nil {
			major = Major(root)
		}
		layout = LayoutFor(major)
	})
	return layout
}

// Version reads the version of the Sonic Pi tree at root: its VERSION file
// (Sonic Pi 5), or the version runtime.rb sets (4.x).
func Version(root string) string {
	if b, err := os.ReadFile(filepath.Join(root, "VERSION")); err == nil {
		return strings.TrimSpace(string(b))
	}
	b, err := os.ReadFile(filepath.Join(root, "app", "server", "ruby", "lib", "sonicpi", "runtime.rb"))
	if err != nil {
		return ""
	}
	m := regexp.MustCompile(`(?m)^\s*@version = Version\.new\(([^)]*)\)`).FindStringSubmatch(string(b))
	if m == nil {
		return ""
	}
	var parts []string
	for _, p := range strings.Split(m[1], ",") {
		if p = strings.Trim(strings.TrimSpace(p), `"'`); p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ".")
}

// Major is the major version of the Sonic Pi tree at root (5 if unknown).
func Major(root string) int {
	v := Version(root)
	if i := strings.IndexAny(v, ".-"); i > 0 {
		v = v[:i]
	}
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return n
	}
	return 5
}

// GUISettingsPath is the desktop client's preferences file.
func GUISettingsPath() string { return filepath.Join(ConfigDir(), CurrentLayout().GUI) }

// AudioSettingsPath is the audio engine's configuration, read by the boot daemon.
func AudioSettingsPath() string { return filepath.Join(ConfigDir(), CurrentLayout().Audio) }

// InitPath is the code run when the server boots.
func InitPath() string { return filepath.Join(ConfigDir(), "init.rb") }

// ColourThemePath is the desktop client's theme overrides.
func ColourThemePath() string { return filepath.Join(ConfigDir(), CurrentLayout().Theme) }

// ShortcutsPath is the desktop client's keyboard shortcuts.
func ShortcutsPath() string { return filepath.Join(ConfigDir(), CurrentLayout().Shortcuts) }

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

// RubyPathFile, next to the Sonic Pi tree, names the Ruby a package's
// native extensions were built for (Linux packages use the system Ruby).
const RubyPathFile = "ruby.path"

// Ruby returns the Ruby interpreter to run Sonic Pi with: $SONICPI_RUBY,
// the Ruby bundled under app/server/native/ruby (as on Windows), the one
// named in ruby.path, or ruby on PATH.
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
		if b, err := os.ReadFile(filepath.Join(filepath.Dir(root), RubyPathFile)); err == nil {
			if p := strings.TrimSpace(string(b)); p != "" {
				if _, err := os.Stat(p); err == nil {
					return p
				}
			}
		}
	}
	return name
}

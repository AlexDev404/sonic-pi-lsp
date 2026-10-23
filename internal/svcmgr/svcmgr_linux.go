//go:build linux

package svcmgr

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const unitName = Name + ".service"

// systemPaths are where a package installs the user unit for every user.
var systemPaths = []string{"/usr/lib/systemd/user/" + unitName, "/lib/systemd/user/" + unitName, "/etc/systemd/user/" + unitName}

func userUnitPath() string {
	cfg := os.Getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		home, _ := os.UserHomeDir()
		cfg = filepath.Join(home, ".config")
	}
	return filepath.Join(cfg, "systemd", "user", unitName)
}

func systemctl(args ...string) (string, error) {
	cmd := exec.Command("systemctl", append([]string{"--user"}, args...)...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

// usable reports whether the user's systemd instance answers.
func usable() bool {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}
	_, err := systemctl("show-environment")
	return err == nil
}

// Describe names how the service is managed here.
func Describe() string { return "systemd user unit " + unitName }

// Installed reports whether a unit file exists for this user.
func Installed() bool {
	for _, p := range append(systemPaths, userUnitPath()) {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

// Unit is the unit file `sonicpi service install` writes.
func Unit(exe string) string {
	return fmt.Sprintf(`[Unit]
Description=Sonic Pi CLI service (sonicpid)
Documentation=https://github.com/AlexDev404/sonic-pi-lsp
After=pipewire.service pipewire-pulse.service wireplumber.service sound.target
Wants=pipewire.service

[Service]
Type=simple
ExecStart=%s run --mode service
Restart=on-failure
RestartSec=5
TimeoutStopSec=30

[Install]
WantedBy=default.target
`, exe)
}

// Install writes a user unit (unless a package already installed one),
// then enables and starts it.
func Install(exe string) error {
	if !usable() {
		return fmt.Errorf("%w: systemd's user instance is not reachable (is this a systemd session?)", ErrNoManager)
	}
	packaged := false
	for _, p := range systemPaths {
		if _, err := os.Stat(p); err == nil {
			packaged = true
		}
	}
	if !packaged {
		p := userUnitPath()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(Unit(exe)), 0o644); err != nil {
			return err
		}
		if out, err := systemctl("daemon-reload"); err != nil {
			return fmt.Errorf("systemctl daemon-reload: %s", out)
		}
	}
	if out, err := systemctl("enable", "--now", unitName); err != nil {
		return fmt.Errorf("systemctl enable: %s", out)
	}
	return nil
}

// Uninstall stops and disables the unit, removing it if this user owns it.
func Uninstall() error {
	if usable() {
		systemctl("disable", "--now", unitName)
	}
	p := userUnitPath()
	if _, err := os.Stat(p); err == nil {
		if err := os.Remove(p); err != nil {
			return err
		}
		if usable() {
			systemctl("daemon-reload")
		}
	}
	return nil
}

func run(verb string) error {
	if !Installed() || !usable() {
		return ErrNoManager
	}
	if out, err := systemctl(verb, unitName); err != nil {
		return fmt.Errorf("systemctl %s: %s", verb, out)
	}
	return nil
}

// Start starts the installed service.
func Start() error { return run("start") }

// Stop stops the installed service.
func Stop() error { return run("stop") }

// Restart restarts the installed service.
func Restart() error { return run("restart") }

// Status describes the service manager's view of the service.
func Status() string {
	if !Installed() {
		return "not installed"
	}
	if !usable() {
		return "installed, but systemd's user instance is not reachable"
	}
	active, _ := systemctl("is-active", unitName)
	enabled, _ := systemctl("is-enabled", unitName)
	return fmt.Sprintf("%s, %s", active, enabled)
}

// Spawn starts sonicpid as a detached background process.
func Spawn(exe string, logPath string) error {
	out, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		out = nil
	}
	cmd := exec.Command(exe, "run", "--mode", "user")
	cmd.Stdout, cmd.Stderr = out, out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

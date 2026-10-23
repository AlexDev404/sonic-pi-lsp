//go:build windows

package svcmgr

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// Describe names how the service is managed here.
func Describe() string { return "Windows service " + Name }

func open() (*mgr.Mgr, *mgr.Service, error) {
	m, err := mgr.Connect()
	if err != nil {
		return nil, nil, err
	}
	s, err := m.OpenService(Name)
	if err != nil {
		m.Disconnect()
		return nil, nil, err
	}
	return m, s, nil
}

// Installed reports whether the Windows service exists.
func Installed() bool {
	m, err := mgr.Connect()
	if err != nil {
		// Without the right to connect, look for the service's registry key.
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services\`+Name, registry.QUERY_VALUE)
		if err != nil {
			return false
		}
		k.Close()
		return true
	}
	defer m.Disconnect()
	s, err := m.OpenService(Name)
	if err != nil {
		return false
	}
	s.Close()
	return true
}

// InstallOptions say whose desktop configuration the service uses. The
// service runs as LocalSystem, so it is pointed at the installing user's
// ~/.sonic-pi and %TEMP%.
type InstallOptions struct {
	UserHome string // SONIC_PI_HOME
	LogDir   string // SONICPI_LOG_DIR
}

// Install creates the service (start at boot, restart on failure) and starts it.
func Install(exe string) error {
	home, _ := os.UserHomeDir()
	return InstallWith(exe, InstallOptions{UserHome: home, LogDir: filepath.Join(os.TempDir(), "sonic-pi")})
}

// InstallWith is Install with explicit options (used by the installer).
func InstallWith(exe string, o InstallOptions) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connecting to the service manager (run as administrator): %w", err)
	}
	defer m.Disconnect()
	if s, err := m.OpenService(Name); err == nil {
		s.Close()
		return fmt.Errorf("the %s service is already installed", Name)
	}
	s, err := m.CreateService(Name, exe, mgr.Config{
		DisplayName:  "Sonic Pi CLI Service",
		Description:  "Runs the Sonic Pi server in the background for the sonicpi command-line client.",
		StartType:    mgr.StartAutomatic,
		ErrorControl: mgr.ErrorNormal,
	}, "run", "--mode", "service")
	if err != nil {
		return err
	}
	defer s.Close()
	s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 15 * time.Second},
		{Type: mgr.ServiceRestart, Delay: time.Minute},
	}, 24*60*60)

	pd := os.Getenv("ProgramData")
	if pd == "" {
		pd = `C:\ProgramData`
	}
	env := []string{"SONICPI_STATE_DIR=" + filepath.Join(pd, "sonic-pi-cli")}
	if o.UserHome != "" {
		env = append(env, "SONIC_PI_HOME="+o.UserHome)
	}
	if o.LogDir != "" {
		env = append(env, "SONICPI_LOG_DIR="+o.LogDir)
	}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services\`+Name, registry.SET_VALUE)
	if err == nil {
		k.SetStringsValue("Environment", env)
		k.Close()
	}
	return s.Start()
}

// Uninstall stops and deletes the service.
func Uninstall() error {
	m, s, err := open()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	defer s.Close()
	stop(s)
	return s.Delete()
}

func stop(s *mgr.Service) error {
	st, err := s.Control(svc.Stop)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(30 * time.Second)
	for st.State != svc.Stopped && time.Now().Before(deadline) {
		time.Sleep(300 * time.Millisecond)
		if st, err = s.Query(); err != nil {
			return err
		}
	}
	return nil
}

// Start starts the service.
func Start() error {
	if !Installed() {
		return ErrNoManager
	}
	m, s, err := open()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	defer s.Close()
	return s.Start()
}

// Stop stops the service.
func Stop() error {
	if !Installed() {
		return ErrNoManager
	}
	m, s, err := open()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	defer s.Close()
	return stop(s)
}

// Restart restarts the service.
func Restart() error {
	if err := Stop(); err != nil && err != ErrNoManager {
		return err
	}
	return Start()
}

// Status describes the service manager's view of the service.
func Status() string {
	if !Installed() {
		return "not installed"
	}
	m, s, err := open()
	if err != nil {
		return "installed"
	}
	defer m.Disconnect()
	defer s.Close()
	st, err := s.Query()
	if err != nil {
		return "installed"
	}
	names := map[svc.State]string{svc.Stopped: "stopped", svc.StartPending: "starting", svc.StopPending: "stopping", svc.Running: "running"}
	cfg, _ := s.Config()
	start := "manual"
	if cfg.StartType == mgr.StartAutomatic {
		start = "starts at boot"
	}
	return strings.TrimSpace(names[st.State] + ", " + start)
}

// Spawn starts sonicpid as a detached background process.
func Spawn(exe string, logPath string) error {
	out, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		out = nil
	}
	cmd := exec.Command(exe, "run", "--mode", "user")
	cmd.Stdout, cmd.Stderr = out, out
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP | 0x08000000,
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

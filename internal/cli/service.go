package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/AlexDev404/sonic-pi-lsp/internal/control"
	"github.com/AlexDev404/sonic-pi-lsp/internal/paths"
	"github.com/AlexDev404/sonic-pi-lsp/internal/svcmgr"
	"github.com/AlexDev404/sonic-pi-lsp/internal/version"
)

func init() {
	register(&command{
		name:    "service",
		aliases: []string{"daemon"},
		summary: "manage the background service (start, stop, install, ...)",
		usage:   "sonicpi service [status|start|stop|restart|install|uninstall|run]",
		run:     cmdService,
	})
	register(&command{
		name:    "status",
		summary: "show the service and server state, ports and devices",
		usage:   "sonicpi status [--json]",
		run:     cmdStatus,
	})
	register(&command{
		name:    "reboot",
		summary: "reboot the Sonic Pi server (picks up audio settings and init.rb)",
		usage:   "sonicpi reboot",
		run:     cmdReboot,
	})
	register(&command{
		name:    "version",
		summary: "show the CLI and Sonic Pi versions",
		usage:   "sonicpi version [--check]",
		run:     cmdVersion,
	})
	register(&command{
		name:    "doctor",
		summary: "check the installation and explain what is missing",
		usage:   "sonicpi doctor",
		run:     cmdDoctor,
	})
}

func cmdService(g *Globals, args []string) error {
	fs := newFlags(g, "service", "sonicpi service [status|start|stop|restart|install|uninstall|run]")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	sub := "status"
	if len(pos) > 0 {
		sub = pos[0]
	}
	switch sub {
	case "status":
		return cmdStatus(g, nil)
	case "start":
		if _, err := control.Dial(); err == nil {
			fmt.Fprintln(os.Stderr, "The service is already running.")
			return nil
		}
		g.NoAutostart = false
		if _, err := ensureService(g); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "The service is running. Sonic Pi is booting; `sonicpi status` shows when it is ready.")
		return nil
	case "stop":
		return stopService(g)
	case "restart":
		if err := stopService(g); err != nil {
			return err
		}
		g.NoAutostart = false
		_, err := ensureService(g)
		return err
	case "install":
		exe, err := svcmgr.DaemonExe()
		if err != nil {
			return err
		}
		if c, err := control.Dial(); err == nil && c.State.Mode == "user" {
			c.Call("shutdown", nil, nil)
			waitGone(10 * time.Second)
		}
		if err := svcmgr.Install(exe); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Installed and started the %s.\n", svcmgr.Describe())
		return nil
	case "uninstall":
		if err := svcmgr.Uninstall(); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Removed the %s.\n", svcmgr.Describe())
		return nil
	case "run":
		exe, err := svcmgr.DaemonExe()
		if err != nil {
			return err
		}
		cmd := exec.Command(exe, append([]string{"run", "--echo"}, pos[1:]...)...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		return cmd.Run()
	}
	return usageError("unknown service command %q", sub)
}

func stopService(g *Globals) error {
	c, err := control.Dial()
	if err != nil {
		fmt.Fprintln(os.Stderr, "The service is not running.")
		return nil
	}
	if c.State.Mode == "service" && svcmgr.Installed() {
		if err := svcmgr.Stop(); err == nil {
			waitGone(30 * time.Second)
			fmt.Fprintln(os.Stderr, "Stopped the service.")
			return nil
		}
	}
	if err := c.Call("shutdown", nil, nil); err != nil {
		return err
	}
	if !waitGone(30 * time.Second) {
		return errors.New("the service did not stop")
	}
	fmt.Fprintln(os.Stderr, "Stopped the service.")
	return nil
}

func waitGone(d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if _, err := control.Find(); err != nil {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func cmdStatus(g *Globals, args []string) error {
	fs := newFlags(g, "status", "sonicpi status [--json]")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	c, err := control.Dial()
	if err != nil {
		if g.JSON {
			fmt.Println(`{"state":"not running"}`)
			return errSilent
		}
		fmt.Println("Service:   not running")
		fmt.Println("Manager:  ", svcmgr.Describe()+":", svcmgr.Status())
		fmt.Println("\n`sonicpi FILE` starts it on demand; `sonicpi service install` starts it at login/boot.")
		return errSilent
	}
	var st control.Status
	if err := c.Call("status", nil, &st); err != nil {
		return err
	}
	if g.JSON {
		b, _ := json.MarshalIndent(st, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	state := st.State
	if st.State == control.StateRunning {
		state += fmt.Sprintf(" (up %s)", time.Since(st.Booted).Round(time.Second))
	}
	fmt.Printf("Server:    %s\n", state)
	if st.Error != "" {
		fmt.Printf("Error:     %s\n", strings.ReplaceAll(st.Error, "\n", "\n           "))
	}
	fmt.Printf("Service:   sonicpid %s, pid %d, %s mode (%s)\n", st.Version, st.PID, st.Mode, svcmgr.Status())
	if st.SonicPiVersion != "" {
		v := "Sonic Pi " + st.SonicPiVersion
		if st.LatestVersion != "" && st.LatestVersion != st.SonicPiVersion {
			v += " (latest: " + st.LatestVersion + ")"
		}
		fmt.Printf("Version:   %s on %s\n", v, st.Platform)
	}
	fmt.Printf("Root:      %s\n", st.Root)
	fmt.Printf("Logs:      %s\n           %s\n", st.LogDir, st.ServerLogDir)
	if st.Ports.GUISend != 0 {
		fmt.Printf("Ports:     server %d, gui %d, OSC cues %d, scsynth %d, Tau %d, daemon %d\n",
			st.Ports.GUISend, st.Ports.GUIListen, st.Ports.OSCCues, st.Ports.Scsynth, st.Ports.Tau, st.Ports.Daemon)
	}
	if st.Scsynth != "" {
		fmt.Printf("Audio:     %s\n", strings.ReplaceAll(strings.TrimSpace(st.Scsynth), "\n", "\n           "))
	}
	if st.MidiIn != "" || st.MidiOut != "" {
		fmt.Printf("MIDI in:   %s\nMIDI out:  %s\n", st.MidiIn, st.MidiOut)
	}
	if st.LinkBPM != "" {
		fmt.Printf("Link:      %s BPM, %s peers\n", st.LinkBPM, st.LinkPeers)
	}
	fmt.Printf("Runs:      %d since boot %d\n", st.Runs, st.Boots)
	return nil
}

func cmdReboot(g *Globals, args []string) error {
	fs := newFlags(g, "reboot", "sonicpi reboot")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	c, err := ensureService(g)
	if err != nil {
		return err
	}
	if err := c.Call("restart", nil, nil); err != nil {
		return err
	}
	time.Sleep(time.Second)
	if _, err := connect(g); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Sonic Pi has rebooted.")
	return nil
}

func cmdVersion(g *Globals, args []string) error {
	fs := newFlags(g, "version", "sonicpi version [--check]")
	check := fs.Bool("check", false, "ask the server to check for a newer Sonic Pi now")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	out := map[string]string{"cli": version.Version, "os": runtime.GOOS + "/" + runtime.GOARCH}
	if root, err := paths.Root(); err == nil {
		if b, err := os.ReadFile(filepath.Join(root, "app", "server", "ruby", "lib", "sonicpi", "runtime.rb")); err == nil {
			out["bundled"] = parseRubyVersion(string(b))
		}
	}
	if c, err := control.Dial(); err == nil {
		if *check {
			spider(c, "/check-for-updates-now")
		}
		var e struct {
			Data []string `json:"data"`
		}
		if err := c.Call("send", control.SendArgs{Target: control.ToSpider, Address: "/version", Expect: "version", Timeout: 20}, &e); err == nil && len(e.Data) >= 3 {
			out["server"] = e.Data[0]
			out["latest"] = e.Data[2]
		}
	}
	if g.JSON {
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	fmt.Printf("sonicpi %s (%s)\n", out["cli"], out["os"])
	if v := out["bundled"]; v != "" {
		fmt.Printf("Sonic Pi %s bundled\n", v)
	}
	if v := out["server"]; v != "" {
		fmt.Printf("Sonic Pi %s running", v)
		if l := out["latest"]; l != "" && l != v {
			fmt.Printf(" (latest release: %s)", l)
		}
		fmt.Println()
	}
	return nil
}

func parseRubyVersion(src string) string {
	// runtime.rb sets the server's version: @version = Version.new(4, 6, 0)
	for _, l := range strings.Split(src, "\n") {
		l = strings.TrimSpace(l)
		if i := strings.Index(l, "Version.new("); i >= 0 && strings.HasPrefix(l, "@version =") {
			args := strings.TrimSuffix(strings.TrimSpace(l[i+len("Version.new("):]), ")")
			parts := strings.Split(args, ",")
			var nums []string
			for _, p := range parts {
				p = strings.Trim(strings.TrimSpace(p), `"'`)
				if p != "" {
					nums = append(nums, p)
				}
			}
			if len(nums) >= 3 {
				v := strings.Join(nums[:3], ".")
				if len(nums) > 3 {
					v += "-" + nums[3]
				}
				return v
			}
		}
	}
	return ""
}

func cmdDoctor(g *Globals, args []string) error {
	fs := newFlags(g, "doctor", "sonicpi doctor")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	ok := true
	check := func(name string, good bool, detail string) {
		mark := "ok  "
		if !good {
			mark, ok = "FAIL", false
		}
		fmt.Printf("[%s] %-22s %s\n", mark, name, detail)
	}
	warn := func(name, detail string) { fmt.Printf("[warn] %-20s %s\n", name, detail) }

	root, err := paths.Root()
	check("Sonic Pi server", err == nil, orErr(root, err))
	ruby := paths.Ruby(root)
	out, err := exec.Command(ruby, "-e", "print RUBY_VERSION").Output()
	check("Ruby", err == nil, orErr(ruby+" "+string(out), err))
	if root != "" {
		tau := filepath.Join(root, "app", "server", "beam", "tau", "_build", "prod", "rel", "tau")
		_, err := os.Stat(tau)
		check("Tau (MIDI, Link, OSC)", err == nil, orErr(tau, err))
		syn := filepath.Join(root, "etc", "synthdefs", "compiled")
		_, err = os.Stat(syn)
		check("Synthdefs", err == nil, orErr(syn, err))
	}
	scsynth := "scsynth"
	if runtime.GOOS == "windows" && root != "" {
		scsynth = filepath.Join(root, "app", "server", "native", "scsynth.exe")
	}
	p, err := exec.LookPath(scsynth)
	check("scsynth", err == nil, orErr(p, err))
	if runtime.GOOS == "linux" {
		// The boot daemon starts scsynth through PipeWire's JACK layer.
		p, err := exec.LookPath("pw-jack")
		check("PipeWire JACK (pw-jack)", err == nil, orErr(p, err))
		if os.Getenv("XDG_RUNTIME_DIR") == "" {
			warn("Audio session", "XDG_RUNTIME_DIR is unset; scsynth may not reach PipeWire")
		}
	}
	lint, err := paths.LintDir()
	check("Linter", err == nil, orErr(lint, err))
	exe, err := svcmgr.DaemonExe()
	check("sonicpid", err == nil, orErr(exe, err))
	fmt.Printf("[info] %-20s %s: %s\n", "Service manager", svcmgr.Describe(), svcmgr.Status())
	fmt.Printf("[info] %-20s %s\n", "Config", paths.ConfigDir())
	fmt.Printf("[info] %-20s %s\n", "Log", logDir())
	if c, err := control.Dial(); err == nil {
		var st control.Status
		c.Call("status", nil, &st)
		check("Server", st.State == control.StateRunning, st.State+" "+st.Error)
	} else {
		fmt.Printf("[info] %-20s %s\n", "Service", "not running (starts on demand)")
	}
	if !ok {
		return errSilent
	}
	return nil
}

func orErr(s string, err error) string {
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(s)
}

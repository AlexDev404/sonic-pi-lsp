// Package service is the background service (sonicpid). It boots Sonic
// Pi's own boot daemon and then plays the part the desktop GUI plays: it
// keeps the daemon alive, listens on the GUI port for everything the server
// reports, applies the desktop's preferences, and logs. The CLI drives it
// over the control socket.
package service

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AlexDev404/sonic-pi-lsp/internal/control"
	"github.com/AlexDev404/sonic-pi-lsp/internal/events"
	"github.com/AlexDev404/sonic-pi-lsp/internal/logfile"
	"github.com/AlexDev404/sonic-pi-lsp/internal/osc"
	"github.com/AlexDev404/sonic-pi-lsp/internal/paths"
	"github.com/AlexDev404/sonic-pi-lsp/internal/prefs"
)

// BootTimeout bounds how long the server may take to answer its first ping.
var BootTimeout = 180 * time.Second

// Supervisor owns one Sonic Pi server.
type Supervisor struct {
	Root    string
	Ruby    string
	Log     *logfile.Writer
	Version string
	Mode    string

	major     int // the Sonic Pi major version being run
	mu        sync.Mutex
	state     string
	bootErr   string
	ports     control.Ports
	token     int32
	cmd       *exec.Cmd
	gui       *osc.Server
	spider    *osc.Client
	daemon    *osc.Client
	tau       *osc.Client
	cues      *osc.Client
	engine    *osc.TCPClient
	booted    time.Time
	boots     int
	started   time.Time
	info      map[string]string
	runs      int
	stateCh   chan struct{} // closed and replaced whenever state changes
	subs      map[chan *events.Event]struct{}
	stopping  bool
	sessionCh chan struct{} // closed when the current server session ends
}

// New makes a supervisor for the Sonic Pi tree at root.
func New(root string, log *logfile.Writer, version, mode string) *Supervisor {
	return &Supervisor{
		Root:    root,
		Ruby:    paths.Ruby(root),
		Log:     log,
		Version: version,
		Mode:    mode,
		major:   paths.Major(root),
		state:   control.StateStopped,
		started: time.Now(),
		info:    map[string]string{},
		stateCh: make(chan struct{}),
		subs:    map[chan *events.Event]struct{}{},
	}
}

func (s *Supervisor) setState(st, errText string) {
	s.mu.Lock()
	s.state, s.bootErr = st, errText
	close(s.stateCh)
	s.stateCh = make(chan struct{})
	s.mu.Unlock()
}

// emit logs an event and hands it to every subscriber.
func (s *Supervisor) emit(e *events.Event) {
	if s.Log != nil {
		s.Log.Write(e)
	}
	s.mu.Lock()
	for ch := range s.subs {
		select {
		case ch <- e:
		default: // a slow subscriber loses events rather than stalling the server
		}
	}
	s.mu.Unlock()
}

func (s *Supervisor) notice(format string, a ...any) { s.emit(events.Service(format, a...)) }

// Subscribe returns a channel of every event from now on.
func (s *Supervisor) Subscribe() (chan *events.Event, func()) {
	ch := make(chan *events.Event, 4096)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}
}

// Status reports the server's state.
func (s *Supervisor) Status() control.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := control.Status{
		State:        s.state,
		Error:        s.bootErr,
		PID:          os.Getpid(),
		Mode:         s.Mode,
		Version:      s.Version,
		Root:         s.Root,
		ServerLogDir: paths.ServerLogDir(),
		Started:      s.started,
		Booted:       s.booted,
		Boots:        s.boots,
		Ports:        s.ports,
		Runs:         s.runs,
	}
	if s.Log != nil {
		st.LogDir = s.Log.Dir()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		st.DaemonPID = s.cmd.Process.Pid
	}
	st.SonicPiVersion = s.info["version"]
	st.LatestVersion = s.info["latest_version"]
	st.Platform = s.info["platform"]
	st.Scsynth = s.info["scsynth"]
	st.MidiIn = s.info["midi/in-ports"]
	st.MidiOut = s.info["midi/out-ports"]
	st.LinkBPM = s.info["/link-bpm"]
	st.LinkPeers = s.info["/link-num-peers"]
	return st
}

// WaitBooted blocks until the server is running or has failed.
func (s *Supervisor) WaitBooted(ctx context.Context) control.Status {
	for {
		s.mu.Lock()
		st, ch := s.state, s.stateCh
		s.mu.Unlock()
		if st == control.StateRunning || st == control.StateFailed {
			return s.Status()
		}
		select {
		case <-ctx.Done():
			return s.Status()
		case <-ch:
		}
	}
}

// Run keeps a server running until ctx ends, rebooting it when it dies.
func (s *Supervisor) Run(ctx context.Context) {
	backoff := 2 * time.Second
	for {
		start := time.Now()
		err := s.session(ctx)
		if ctx.Err() != nil {
			s.setState(control.StateStopped, "")
			return
		}
		msg := "the server stopped"
		if err != nil {
			msg = err.Error()
		}
		s.setState(control.StateFailed, msg)
		s.notice("Sonic Pi server stopped: %s", msg)
		if time.Since(start) > 2*time.Minute {
			backoff = 2 * time.Second
		}
		s.notice("Rebooting in %s", backoff)
		select {
		case <-ctx.Done():
			s.setState(control.StateStopped, "")
			return
		case <-time.After(backoff):
		}
		if backoff < time.Minute {
			backoff *= 2
		}
	}
}

// Restart ends the current server session; Run boots a fresh one.
func (s *Supervisor) Restart() {
	s.mu.Lock()
	ch := s.sessionCh
	s.mu.Unlock()
	if ch != nil {
		select {
		case <-ch:
		default:
			close(ch)
		}
	}
}

// session boots one server and blocks until it exits.
func (s *Supervisor) session(ctx context.Context) error {
	st, err := prefs.Load(s.Root)
	if err != nil {
		return fmt.Errorf("reading preferences: %w", err)
	}
	s.mu.Lock()
	s.boots++
	s.info = map[string]string{}
	s.sessionCh = make(chan struct{})
	sessionCh := s.sessionCh
	s.mu.Unlock()
	s.setState(control.StateBooting, "")

	daemonPath := filepath.Join(s.Root, "app", "server", "ruby", "bin", "daemon.rb")
	args := append([]string{daemonPath}, daemonArgs(st, s.major)...)
	cmd := exec.Command(s.Ruby, args...)
	cmd.Dir = filepath.Dir(daemonPath)
	cmd.Env = os.Environ()
	hideWindow(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	s.notice("Booting Sonic Pi from %s (ruby: %s)", s.Root, s.Ruby)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting the boot daemon: %w", err)
	}
	s.mu.Lock()
	s.cmd = cmd
	s.mu.Unlock()
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			if t := strings.TrimSpace(sc.Text()); t != "" {
				s.notice("daemon: %s", t)
			}
		}
	}()

	// The daemon's first meaningful line is either its ports and token, or
	// a scsynth boot error followed by scsynth's own output.
	lines := bufio.NewReader(stdout)
	type portResult struct {
		n    []int
		fail string
	}
	result := make(chan portResult, 1)
	go func() {
		for {
			l, err := lines.ReadString('\n')
			if strings.Contains(l, "Boot Error") {
				rest, _ := io.ReadAll(io.LimitReader(lines, 64<<10))
				result <- portResult{fail: l + string(rest)}
				return
			}
			if n := parsePorts(l); n != nil {
				result <- portResult{n: n}
				return
			}
			if t := strings.TrimSpace(l); t != "" {
				s.notice("daemon: %s", t)
			}
			if err != nil {
				result <- portResult{fail: "the boot daemon closed its output without reporting its ports"}
				return
			}
		}
	}()
	var n []int
	select {
	case r := <-result:
		if r.fail != "" {
			killTree(cmd)
			return errors.New(strings.TrimSpace(r.fail))
		}
		n = r.n
	case err := <-exited:
		return fmt.Errorf("the boot daemon exited before it reported its ports: %v (see %s)", err, paths.ServerLogDir())
	case <-time.After(BootTimeout):
		killTree(cmd)
		return errors.New("timed out waiting for the boot daemon to report its ports")
	case <-ctx.Done():
		killTree(cmd)
		return ctx.Err()
	}
	// Sonic Pi 5: daemon gui-listen gui-send engine osc-cues token.
	// Sonic Pi 4: daemon gui-listen gui-send scsynth osc-cues tau tau-phx token.
	ports := control.Ports{Daemon: n[0], GUIListen: n[1], GUISend: n[2], Scsynth: n[3], OSCCues: n[4]}
	if len(n) == 8 {
		ports.Tau, ports.TauPhx = n[5], n[6]
	}
	token := int32(n[len(n)-1])

	// Everything else the daemon prints goes to the log.
	go func() {
		sc := bufio.NewScanner(lines)
		for sc.Scan() {
			if t := strings.TrimSpace(sc.Text()); t != "" {
				s.notice("daemon: %s", t)
			}
		}
	}()

	gui, err := osc.Listen(ports.GUIListen)
	if err != nil {
		killTree(cmd)
		return fmt.Errorf("listening on the GUI port %d: %w", ports.GUIListen, err)
	}
	defer gui.Close()
	spider, _ := osc.Dial("127.0.0.1", ports.GUISend)
	daemon, _ := osc.Dial("127.0.0.1", ports.Daemon)
	tau, _ := osc.Dial("127.0.0.1", ports.Tau)
	cues, _ := osc.Dial("127.0.0.1", ports.OSCCues)
	defer func() {
		for _, c := range []*osc.Client{spider, daemon, tau, cues} {
			if c != nil {
				c.Close()
			}
		}
	}()
	var engine *osc.TCPClient
	if s.major >= 5 {
		engine = osc.NewTCPClient("127.0.0.1", ports.Scsynth)
		defer engine.Close()
	}
	s.mu.Lock()
	s.ports, s.token, s.gui, s.spider, s.daemon, s.tau, s.cues, s.engine = ports, token, gui, spider, daemon, tau, cues, engine
	s.mu.Unlock()

	acked := make(chan struct{})
	var ackOnce sync.Once
	serverGone := make(chan string, 1)
	go gui.Serve(func(m *osc.Message) {
		if m.Address == "/ack" {
			ackOnce.Do(func() { close(acked) })
			return
		}
		if m.Address == "/exited" || m.Address == "/exited-with-boot-error" {
			why := "the Ruby server exited"
			if m.Address == "/exited-with-boot-error" {
				why = "the Ruby server failed to boot: " + strings.TrimSpace(m.Str(0))
			}
			select {
			case serverGone <- why:
			default:
			}
		}
		if !s.record(m) {
			return // a device list or Link state that has not changed
		}
		if e := events.FromOSC(m, time.Now()); e != nil {
			s.emit(e)
		}
	})

	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go s.every(sessionCtx, 2*time.Second, func() { daemon.Send("/daemon/keep-alive", token) })

	// Ping until the server answers, as the GUI does.
	pingCtx, stopPing := context.WithCancel(sessionCtx)
	go s.every(pingCtx, 500*time.Millisecond, func() { spider.Send("/ping", token, "sonicpid") })
	select {
	case <-acked:
	case why := <-serverGone:
		stopPing()
		s.shutdownDaemon(cmd, daemon, token, exited)
		return fmt.Errorf("%s (see %s)", why, paths.ServerLogDir())
	case err := <-exited:
		stopPing()
		return fmt.Errorf("the boot daemon exited while the server was booting: %v (see %s)", err, paths.ServerLogDir())
	case <-time.After(BootTimeout):
		stopPing()
		s.shutdownDaemon(cmd, daemon, token, exited)
		return fmt.Errorf("the server did not answer within %s (see %s)", BootTimeout, paths.ServerLogDir())
	case <-ctx.Done():
		stopPing()
		s.shutdownDaemon(cmd, daemon, token, exited)
		return ctx.Err()
	}
	stopPing()

	s.mu.Lock()
	s.booted = time.Now()
	s.mu.Unlock()
	s.ApplyPrefs(st, true)
	spider.Send("/version", token)
	go s.every(sessionCtx, time.Second, func() { spider.Send("/gui-heartbeat", token) })
	s.setState(control.StateRunning, "")
	s.notice("Sonic Pi server is running (ports: gui %d, server %d, cues %d)", ports.GUIListen, ports.GUISend, ports.OSCCues)

	select {
	case why := <-serverGone:
		s.shutdownDaemon(cmd, daemon, token, exited)
		return errors.New(why)
	case err := <-exited:
		return fmt.Errorf("the boot daemon exited: %v", err)
	case <-sessionCh:
		s.notice("Restarting the Sonic Pi server")
		s.shutdownDaemon(cmd, daemon, token, exited)
		return errors.New("restart requested")
	case <-ctx.Done():
		s.setState(control.StateStopping, "")
		s.notice("Shutting down the Sonic Pi server")
		s.shutdownDaemon(cmd, daemon, token, exited)
		return ctx.Err()
	}
}

func (s *Supervisor) shutdownDaemon(cmd *exec.Cmd, daemon *osc.Client, token int32, exited chan error) {
	if daemon != nil {
		daemon.Send("/daemon/exit", token)
	}
	select {
	case <-exited:
	case <-time.After(15 * time.Second):
		killTree(cmd)
		<-exited
	}
}

func (s *Supervisor) every(ctx context.Context, d time.Duration, fn func()) {
	fn()
	t := time.NewTicker(d)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fn()
		}
	}
}

// record keeps the latest of the facts `sonicpi status` reports. It
// returns false for a MIDI port list or Link state that has not changed,
// which the server resends often and which is not worth logging again.
func (s *Supervisor) record(m *osc.Message) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	keep := func(key, v string) bool {
		old, seen := s.info[key]
		s.info[key] = v
		return !seen || old != v
	}
	switch m.Address {
	case "/midi/in-ports", "midi/in-ports":
		return keep("midi/in-ports", m.Str(0))
	case "/midi/out-ports", "midi/out-ports":
		return keep("midi/out-ports", m.Str(0))
	case "/link-bpm", "/link-num-peers":
		return keep(m.Address, m.Str(0))
	}
	switch m.Address {
	case "/version":
		s.info["version"] = m.Str(0)
		s.info["latest_version"] = m.Str(2)
		s.info["platform"] = m.Str(7)
	case "/scsynth/info", "/supersonic/info":
		s.info["scsynth"] = m.Str(0)
	}
	return true
}

// parsePorts reads the daemon's port line: its ports and then a token,
// five ports for Sonic Pi 5 and seven for 4.x.
func parsePorts(l string) []int {
	fields := strings.Fields(l)
	if len(fields) != 6 && len(fields) != 8 {
		return nil
	}
	n := make([]int, len(fields))
	for i, f := range fields {
		v, err := strconv.ParseInt(f, 10, 64)
		if err != nil {
			return nil
		}
		n[i] = int(v)
	}
	return n
}

// ErrNotBooted is returned for commands that need a running server.
var ErrNotBooted = errors.New("the Sonic Pi server is not running yet")

// Send sends an OSC message to one of the server's processes.
func (s *Supervisor) Send(target, address string, args ...any) error {
	s.mu.Lock()
	state, token := s.state, s.token
	var c *osc.Client
	switch target {
	case control.ToSpider:
		c = s.spider
		args = append([]any{token}, args...)
	case control.ToDaemon:
		c = s.daemon
		args = append([]any{token}, args...)
	case control.ToTau:
		c = s.tau
	case control.ToCues:
		c = s.cues
	case control.ToEngine:
		e := s.engine
		s.mu.Unlock()
		if state != control.StateRunning || e == nil {
			return ErrNotBooted
		}
		return e.Send(address, args...)
	default:
		s.mu.Unlock()
		return fmt.Errorf("unknown target %q", target)
	}
	s.mu.Unlock()
	if state != control.StateRunning || c == nil {
		return ErrNotBooted
	}
	return c.Send(address, args...)
}

// RunCode evaluates code, as the desktop's Run button does.
func (s *Supervisor) RunCode(code string) error {
	if err := s.Send(control.ToSpider, "/run-code", code); err != nil {
		return err
	}
	s.mu.Lock()
	s.runs++
	s.mu.Unlock()
	return nil
}

func boolMsg(on bool, yes, no string) string {
	if on {
		return yes
	}
	return no
}

// ApplyPrefs sends the server the preferences that take effect live, as the
// desktop does when it boots and when its preferences change.
func (s *Supervisor) ApplyPrefs(st *prefs.Store, silent bool) {
	quiet := int32(0)
	if silent {
		quiet = 1
	}
	if s.major >= 5 {
		vol := float32(st.Int("output-volume")) / 100
		s.Send(control.ToSpider, "/mixer-output-volume", vol, quiet)
		s.Send(control.ToSpider, "/mixer-drive", float32(DriveAmount(drivePct(st))), quiet)
		s.Send(control.ToSpider, boolMsg(st.Bool("gamepad-enable"), "/gamepad-start", "/gamepad-stop"), quiet)
	} else {
		// 4.x maps its 0-100 slider onto an amp of 0-2.
		vol := float32(st.Int("system-vol")) / 100 * 2
		s.Send(control.ToSpider, "/mixer-amp", vol, quiet)
	}
	s.Send(control.ToSpider, boolMsg(st.Bool("midi-enable"), "/midi-start", "/midi-stop"), quiet)
	s.Send(control.ToSpider, boolMsg(st.Bool("osc-enabled"), "/cue-port-start", "/cue-port-stop"))
	s.Send(control.ToSpider, boolMsg(st.Bool("osc-public"), "/cue-port-external", "/cue-port-internal"))
	s.Send(control.ToSpider, boolMsg(st.Bool("mixer-invert-stereo"), "/mixer-invert-stereo", "/mixer-standard-stereo"))
	s.Send(control.ToSpider, boolMsg(st.Bool("mixer-force-mono"), "/mixer-mono-mode", "/mixer-stereo-mode"))
	s.Send(control.ToSpider, boolMsg(st.Bool("rp/check-updates"), "/enable-update-checking", "/disable-update-checking"))
}

// drivePct reads the drive setting. Like the desktop, a settings file from
// 4.x (with system-vol but no drive-pct) carries its volume over as drive.
func drivePct(st *prefs.Store) int {
	d, _ := prefs.Lookup("drive-pct")
	if _, set := st.Get(d); !set {
		v, _ := prefs.Lookup("system-vol")
		if _, old := st.Get(v); old {
			return int(float64(st.Int("system-vol")) * 2.4)
		}
	}
	return st.Int("drive-pct")
}

// DriveAmount converts a drive percentage (100 = unity) into the 0-1
// amount set_drive! takes: 0 is a quarter, 0.5 unity and 1 four times.
func DriveAmount(pct int) float64 {
	gain := float64(pct) / 100
	if gain < 0.25 {
		gain = 0.25
	}
	return math.Log(gain/0.25) / math.Log(16)
}

// daemonArgs are the boot daemon's flags: audio inputs off unless enabled,
// and from Sonic Pi 5 the desktop's saved audio device choices.
func daemonArgs(st *prefs.Store, major int) []string {
	var args []string
	if !st.Bool("enable-scsynth-inputs") {
		args = append(args, "--no-scsynth-inputs")
	}
	if major < 5 {
		return args
	}
	get := func(key string) string {
		s, _ := prefs.Lookup(key)
		v, _ := st.Get(s)
		return strings.TrimSpace(v)
	}
	// The desktop's sentinels mean "no particular device".
	if v := get("audio-output-device"); v != "" && v != "__system__" {
		args = append(args, "--audio-output", v)
	}
	if v := get("audio-input-device"); v != "" && v != "__none__" && v != "__disabled__" {
		args = append(args, "--audio-input", v)
	}
	if n := st.Int("audio-sample-rate"); n > 0 {
		args = append(args, "--audio-sample-rate", strconv.Itoa(n))
	}
	if n := st.Int("audio-buffer-size"); n > 0 {
		args = append(args, "--audio-buffer-size", strconv.Itoa(n))
	}
	if v := get("audio-driver"); v != "" {
		args = append(args, "--audio-driver", v)
	}
	return args
}

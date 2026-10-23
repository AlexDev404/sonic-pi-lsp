package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/AlexDev404/sonic-pi-lsp/internal/control"
	"github.com/AlexDev404/sonic-pi-lsp/internal/events"
	"github.com/AlexDev404/sonic-pi-lsp/internal/paths"
	"github.com/AlexDev404/sonic-pi-lsp/internal/prefs"
)

func init() {
	register(&command{
		name:    "run",
		aliases: []string{"play"},
		summary: "play a file (or - for stdin) and show its log",
		usage:   "sonicpi run [flags] FILE|-",
		run:     cmdRun,
	})
	register(&command{
		name:    "eval",
		summary: "play code given on the command line",
		usage:   "sonicpi eval [flags] CODE...",
		run:     cmdEval,
	})
	register(&command{
		name:    "stop",
		summary: "stop everything that is playing",
		usage:   "sonicpi stop",
		run: func(g *Globals, args []string) error {
			fs := newFlags(g, "stop", "sonicpi stop")
			if _, err := parse(fs, args); err != nil {
				return err
			}
			c, err := connect(g)
			if err != nil {
				return err
			}
			return spider(c, "/stop-all-jobs")
		},
	})
}

// runFlags are shared by run, eval and repl.
type runFlags struct {
	detach      bool
	lint        bool
	timeout     time.Duration
	linger      time.Duration
	record      string
	noStop      bool
	timestamps  bool
	synths      boolFlag
	cues        boolFlag
	checkArgs   boolFlag
	external    boolFlag
	timing      boolFlag
	midiChannel int
}

// register adds every flag of run, eval and buffer run.
func (r *runFlags) register(fs *flag.FlagSet) {
	fs.BoolVar(&r.detach, "detach", false, "send the code and return at once; it keeps playing")
	fs.BoolVar(&r.detach, "d", false, "shorthand for --detach")
	fs.BoolVar(&r.lint, "lint", false, "lint the file first and refuse to play it if there are errors")
	fs.DurationVar(&r.timeout, "timeout", 0, "stop everything after this long (e.g. 30s, 2m)")
	fs.DurationVar(&r.linger, "linger", 0, "keep listening this long after all runs complete (default 2s with --record)")
	fs.StringVar(&r.record, "record", "", "record the output to this `FILE` (.wav)")
	fs.BoolVar(&r.noStop, "no-stop", false, "on Ctrl-C, leave the music playing instead of stopping it")
	r.registerSession(fs)
}

// registerSession adds the flags that shape the log and the preferences
// code runs with, which the REPL takes too.
func (r *runFlags) registerSession(fs *flag.FlagSet) {
	fs.BoolVar(&r.timestamps, "timestamps", false, "prefix log lines with the time")
	r.synths.register(fs, "synths", "log synth and sample triggers (default: the log-synths setting)")
	r.cues.register(fs, "cues", "log cues and incoming OSC/MIDI (default: the log-cues setting)")
	r.checkArgs.register(fs, "check-args", "check synth and FX opts (default: the check-args setting)")
	r.external.register(fs, "external-synths", "allow external synths (default: the enable-external-synths setting)")
	r.timing.register(fs, "timing-guarantees", "drop late synths (default: the synth-trigger-timing-guarantees setting)")
	fs.IntVar(&r.midiChannel, "midi-channel", -1, "default MIDI channel `N`, 0 for all (default: the midi-default-channel setting)")
}

func (r *runFlags) overrides() map[string]string {
	o := map[string]string{}
	set := func(key string, b boolFlag) {
		if b.set {
			o[key] = fmt.Sprint(b.val)
		}
	}
	set("log-synths", r.synths)
	set("log-cues", r.cues)
	set("check-args", r.checkArgs)
	set("enable-external-synths", r.external)
	set("synth-trigger-timing-guarantees", r.timing)
	if r.midiChannel >= 0 {
		o["midi-default-channel"] = fmt.Sprint(r.midiChannel)
	}
	return o
}

// preamble builds the lines the desktop puts before code, from the shared
// preferences and these flags.
func (r *runFlags) preamble() (string, *prefs.Store, error) {
	root, _ := paths.Root()
	st, err := prefs.Load(root)
	if err != nil {
		return "", nil, err
	}
	st.Overrides = r.overrides()
	return st.RunPreamble(), st, nil
}

func (r *runFlags) filter(st *prefs.Store) events.Filter {
	f := events.AllFilter
	f.Synths = r.synths.or(st.Bool("log-synths"))
	f.Cues = r.cues.or(st.Bool("log-cues"))
	return f
}

func cmdRun(g *Globals, args []string) error {
	fs := newFlags(g, "run", "sonicpi run [flags] FILE|-")
	var r runFlags
	r.register(fs)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageError("run takes one file (or - for stdin)")
	}
	src := pos[0]
	read := func() (string, error) {
		if src == "-" {
			b, err := io.ReadAll(os.Stdin)
			return string(b), err
		}
		b, err := os.ReadFile(src)
		return string(b), err
	}
	code, err := read()
	if err != nil {
		return err
	}
	if g.Watch && src == "-" {
		return usageError("--watch needs a file, not stdin")
	}
	if r.lint && src != "-" {
		if err := lintBeforeRun(g, src); err != nil {
			return err
		}
	}
	name := filepath.Base(src)
	if src == "-" {
		name = "stdin"
	}
	return play(g, &r, name, code, func() (string, bool) {
		if src == "-" {
			return "", false
		}
		c, err := read()
		return c, err == nil
	}, src)
}

func cmdEval(g *Globals, args []string) error {
	fs := newFlags(g, "eval", "sonicpi eval [flags] CODE...")
	var r runFlags
	r.register(fs)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return usageError("eval needs code, e.g. sonicpi eval 'play 60'")
	}
	code := strings.Join(pos, " ")
	if r.lint {
		if err := lintCodeBeforeRun(g, "eval.rb", code); err != nil {
			return err
		}
	}
	return play(g, &r, "eval", code, nil, "")
}

// play runs code and, unless detached, shows what it does until all runs
// complete. With --watch it re-reads the source and plays it again each
// time the file changes.
func play(g *Globals, r *runFlags, name, code string, reread func() (string, bool), path string) error {
	pre, st, err := r.preamble()
	if err != nil {
		return err
	}
	c, err := connect(g)
	if err != nil {
		return err
	}
	if r.detach {
		if err := c.Call("run", control.RunArgs{Code: pre + code}, nil); err != nil {
			return err
		}
		if !g.Quiet && !g.JSON {
			fmt.Fprintf(os.Stderr, "Playing %s in the background. `sonicpi stop` stops it; `sonicpi logs -f` follows its log.\n", name)
		}
		return nil
	}

	s := newSession(g, c, r.filter(st), name, path)
	s.timestamps = r.timestamps
	if err := s.open(); err != nil {
		return err
	}
	defer s.close()

	if r.record != "" {
		if err := spider(c, "/start-recording"); err != nil {
			return err
		}
		if r.linger == 0 {
			r.linger = 2 * time.Second
		}
	}
	if err := c.Call("run", control.RunArgs{Code: pre + code}, nil); err != nil {
		return err
	}
	if !g.Quiet && !g.JSON {
		how := "Ctrl-C stops it"
		if g.Watch {
			how = "re-runs when the file changes; Ctrl-C stops it"
		}
		fmt.Fprintf(os.Stderr, "%s Playing %s (%s)\n", s.paint("\x1b[38;5;205m", "▶"), name, how)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)

	var timeout <-chan time.Time
	if r.timeout > 0 {
		timeout = time.After(r.timeout)
	}
	var watch <-chan time.Time
	var lastMod time.Time
	if g.Watch && path != "" {
		t := time.NewTicker(300 * time.Millisecond)
		defer t.Stop()
		watch = t.C
		if fi, err := os.Stat(path); err == nil {
			lastMod = fi.ModTime()
		}
	}
	var linger <-chan time.Time
	interrupted := false

loop:
	for {
		select {
		case e, ok := <-s.events:
			if !ok {
				return fmt.Errorf("lost the connection to the service")
			}
			s.show(e)
			if e.Kind == events.KindRunsComplete && !g.Watch {
				if r.linger > 0 {
					linger = time.After(r.linger)
					continue
				}
				break loop
			}
			if e.Kind == events.KindExited {
				break loop
			}
		case <-linger:
			break loop
		case <-watch:
			fi, err := os.Stat(path)
			if err != nil || !fi.ModTime().After(lastMod) {
				continue
			}
			lastMod = fi.ModTime()
			time.Sleep(50 * time.Millisecond) // let the editor finish writing
			code, ok := reread()
			if !ok {
				continue
			}
			pre, _, _ = r.preamble()
			if !g.Quiet && !g.JSON {
				fmt.Fprintf(os.Stderr, "%s %s changed, playing it again\n", s.paint("\x1b[38;5;205m", "↻"), name)
			}
			if err := c.Call("run", control.RunArgs{Code: pre + code}, nil); err != nil {
				fmt.Fprintln(os.Stderr, "sonicpi:", err)
			}
		case <-timeout:
			if !r.noStop {
				spider(c, "/stop-all-jobs")
			}
			break loop
		case <-sig:
			interrupted = true
			if !r.noStop {
				spider(c, "/stop-all-jobs")
				if !g.Quiet && !g.JSON {
					fmt.Fprintln(os.Stderr, "\nStopped.")
				}
			}
			break loop
		}
	}

	if r.record != "" {
		if err := saveRecording(c, r.record); err != nil {
			return err
		}
		if !g.Quiet && !g.JSON {
			fmt.Fprintln(os.Stderr, "Recorded to", r.record)
		}
	}
	if s.errors > 0 {
		return errSilent
	}
	if interrupted {
		return &exitError{code: 130}
	}
	return nil
}

// saveRecording stops a recording and saves it to path.
func saveRecording(c *control.Client, path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := spider(c, "/stop-recording"); err != nil {
		return err
	}
	before, _ := os.Stat(abs)
	if err := spider(c, "/save-recording", control.Str(abs)); err != nil {
		return err
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if fi, err := os.Stat(abs); err == nil && fi.Size() > 0 && (before == nil || fi.ModTime().After(before.ModTime())) {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("the server did not write %s (see `sonicpi logs --source spider`)", abs)
}

// session streams events from the service and prints them.
type session struct {
	g          *Globals
	c          *control.Client
	filter     events.Filter
	name       string
	path       string
	timestamps bool
	events     chan *events.Event
	stop       chan struct{}
	errors     int
	source     []string
}

func newSession(g *Globals, c *control.Client, f events.Filter, name, path string) *session {
	return &session{g: g, c: c, filter: f, name: name, path: path, events: make(chan *events.Event, 1024), stop: make(chan struct{})}
}

// open subscribes and waits until the service has confirmed it, so no
// event of a run started afterwards is missed.
func (s *session) open() error {
	ready := make(chan error, 1)
	go func() {
		first := true
		err := s.c.Stream("subscribe", nil, nil, func(e *events.Event) {
			if first {
				first = false
				ready <- nil
				return
			}
			s.events <- e
		}, s.stop)
		if first {
			ready <- err
		}
		close(s.events)
	}()
	select {
	case err := <-ready:
		return err
	case <-time.After(10 * time.Second):
		return fmt.Errorf("the service did not answer")
	}
}

func (s *session) close() {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
}

func (s *session) paint(code, text string) string {
	if !s.g.Color() {
		return text
	}
	return code + text + "\x1b[0m"
}

func (s *session) show(e *events.Event) {
	if e.Kind == events.KindError || e.Kind == events.KindSyntaxError {
		s.errors++
	}
	if s.g.JSON {
		b, _ := json.Marshal(e)
		fmt.Println(string(b))
		return
	}
	shown := s.filter.Apply(e)
	if shown == nil {
		return
	}
	if s.g.Quiet && shown.Kind != events.KindError && shown.Kind != events.KindSyntaxError {
		return
	}
	f := events.Formatter{Color: s.g.Color(), Timestamps: s.timestamps, Source: s.name}
	fmt.Print(f.Format(shown))
	if (e.Kind == events.KindError || e.Kind == events.KindSyntaxError) && e.Line > 0 {
		s.showSource(e.Line)
	}
}

// showSource prints the lines around an error, as the desktop highlights
// the line in the editor.
func (s *session) showSource(line int) {
	if s.source == nil && s.path != "" && s.path != "-" {
		if b, err := os.ReadFile(s.path); err == nil {
			s.source = strings.Split(string(b), "\n")
		}
	}
	if line > len(s.source) {
		return
	}
	for i := line - 2; i <= line; i++ {
		if i < 1 {
			continue
		}
		mark := "  "
		text := fmt.Sprintf("%5d | %s", i, s.source[i-1])
		if i == line {
			mark = "> "
			text = s.paint("\x1b[1m", text)
		}
		fmt.Println("   " + mark + text)
	}
}

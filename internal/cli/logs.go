package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/AlexDev404/sonic-pi-lsp/internal/control"
	"github.com/AlexDev404/sonic-pi-lsp/internal/events"
	"github.com/AlexDev404/sonic-pi-lsp/internal/logfile"
	"github.com/AlexDev404/sonic-pi-lsp/internal/paths"
	"github.com/AlexDev404/sonic-pi-lsp/internal/prefs"
)

func init() {
	register(&command{
		name:    "logs",
		aliases: []string{"log"},
		summary: "show the log (synths, cues, puts, errors); -f to follow",
		usage:   "sonicpi [--watch] logs [flags]",
		run:     cmdLogs,
	})
}

// sources are the raw logs the server's own processes write in ~/.sonic-pi/log.
var sources = map[string]string{
	"spider":  "spider.log",
	"daemon":  "daemon.log",
	"scsynth": "scsynth.log",
	"tau":     "tau.log",
	"tau-out": "tau_stdouterr.log",
	"jackd":   "jackd.log",
	"debug":   "debug.log",
	"gui":     "gui.log",
}

func cmdLogs(g *Globals, args []string) error {
	fs := newFlags(g, "logs", "sonicpi [--watch] logs [flags]")
	follow := fs.Bool("f", false, "follow the log (same as --watch)")
	fs.BoolVar(follow, "follow", false, "follow the log (same as --watch)")
	n := fs.Int("n", 100, "show this many past events (-1 for all, 0 for none)")
	fs.IntVar(n, "lines", 100, "same as -n")
	var synths, cues, puts, info, warnings boolFlag
	synths.register(fs, "synths", "show synth and sample triggers (default: the log-synths setting)")
	cues.register(fs, "cues", "show cues, syncs and incoming OSC/MIDI (default: the log-cues setting)")
	puts.register(fs, "puts", "show puts/print output (default on)")
	info.register(fs, "info", "show server and service notices (default on)")
	warnings.register(fs, "warnings", "show warnings (default on)")
	errorsOnly := fs.Bool("errors", false, "show only errors")
	run := fs.Int("run", 0, "show only this run")
	thread := fs.String("thread", "", "show only this thread (live_loop name)")
	timestamps := fs.Bool("timestamps", false, "prefix events with the time")
	fs.BoolVar(timestamps, "t", false, "shorthand for --timestamps")
	since := fs.Duration("since", 0, "show only events newer than this (e.g. 10m)")
	source := fs.String("source", "", "show a raw server log instead: "+strings.Join(sourceNames(), ", "))
	showPath := fs.Bool("path", false, "print the log directory and exit")
	clear := fs.Bool("clear", false, "start a fresh session log (the old one is kept as .1)")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	if *follow {
		g.Watch = true
	}

	dir := logDir()
	if *showPath {
		fmt.Println(dir)
		return nil
	}
	if *clear {
		return clearLog(dir)
	}
	if *source != "" {
		file, ok := sources[*source]
		if !ok {
			return usageError("unknown --source %q (one of %s)", *source, strings.Join(sourceNames(), ", "))
		}
		return tailText(filepath.Join(paths.ServerLogDir(), file), *n, g.Watch)
	}

	root, _ := paths.Root()
	st, err := prefs.Load(root)
	if err != nil {
		return err
	}
	f := events.Filter{
		Synths:   synths.or(st.Bool("log-synths")),
		Cues:     cues.or(st.Bool("log-cues")),
		Puts:     puts.or(true),
		Info:     info.or(true),
		Warnings: warnings.or(true),
		Errors:   true,
		Run:      *run,
		Thread:   *thread,
	}
	if *errorsOnly {
		f = events.Filter{Errors: true, Run: *run}
	}
	fm := events.Formatter{Color: g.Color(), Timestamps: *timestamps}
	var cutoff time.Time
	if *since > 0 {
		cutoff = time.Now().Add(-*since)
	}
	show := func(e *events.Event) {
		if !cutoff.IsZero() && e.Time.Before(cutoff) {
			return
		}
		e = f.Apply(e)
		if e == nil {
			return
		}
		if g.JSON {
			b, _ := json.Marshal(e)
			fmt.Println(string(b))
			return
		}
		fmt.Print(fm.Format(e))
	}

	if *n != 0 {
		past, err := logfile.Tail(dir, -1)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if os.IsNotExist(err) && !g.Watch {
			fmt.Fprintf(os.Stderr, "No log yet in %s. Play something with `sonicpi FILE`.\n", dir)
			return nil
		}
		// Count -n in shown events, not raw ones.
		var shown []*events.Event
		for _, e := range past {
			if (cutoff.IsZero() || !e.Time.Before(cutoff)) && f.Apply(e) != nil {
				shown = append(shown, e)
			}
		}
		if *n > 0 && len(shown) > *n {
			shown = shown[len(shown)-*n:]
		}
		for _, e := range shown {
			show(e)
		}
	}
	if !g.Watch {
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return logfile.Follow(ctx, dir, show)
}

func sourceNames() []string {
	var names []string
	for k := range sources {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// logDir is the running service's log directory, or where it would be.
func logDir() string {
	if s, err := control.Find(); err == nil && s.LogDir != "" {
		return s.LogDir
	}
	return paths.LogDir()
}

func clearLog(dir string) error {
	for _, name := range []string{logfile.EventsName, logfile.TextName} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if err := os.Rename(p, p+".1"); err != nil {
			return err
		}
	}
	fmt.Fprintln(os.Stderr, "Started a fresh log in", dir)
	return nil
}

// tailText prints the last n lines of a text file, following it if asked.
func tailText(path string, n int, follow bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if n != 0 {
		var lines []string
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			lines = append(lines, sc.Text())
			if n > 0 && len(lines) > n {
				lines = lines[1:]
			}
		}
		for _, l := range lines {
			fmt.Println(l)
		}
	} else {
		f.Seek(0, io.SeekEnd)
	}
	if !follow {
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	buf := make([]byte, 32<<10)
	for {
		k, err := f.Read(buf)
		if k > 0 {
			os.Stdout.Write(buf[:k])
		}
		if err == io.EOF || k == 0 {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(200 * time.Millisecond):
			}
			if fi, err := os.Stat(path); err == nil {
				if pos, _ := f.Seek(0, io.SeekCurrent); fi.Size() < pos {
					f.Seek(0, io.SeekStart) // truncated
				}
			}
		} else if err != nil {
			return err
		}
	}
}

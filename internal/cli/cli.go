// Package cli implements the sonicpi command.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/AlexDev404/sonic-pi-lsp/internal/control"
	"github.com/AlexDev404/sonic-pi-lsp/internal/paths"
	"github.com/AlexDev404/sonic-pi-lsp/internal/svcmgr"
)

// Globals are the flags every command accepts.
type Globals struct {
	Watch       bool
	NoColor     bool
	JSON        bool
	NoAutostart bool
	Quiet       bool
}

func (g *Globals) register(fs *flag.FlagSet) {
	fs.BoolVar(&g.Watch, "watch", g.Watch, "keep watching: follow the log, or re-run a file when it changes")
	fs.BoolVar(&g.Watch, "w", g.Watch, "shorthand for --watch")
	fs.BoolVar(&g.NoColor, "no-color", g.NoColor, "disable colour (also NO_COLOR=1)")
	fs.BoolVar(&g.JSON, "json", g.JSON, "machine-readable JSON output")
	fs.BoolVar(&g.NoAutostart, "no-autostart", g.NoAutostart, "don't start the service if it isn't running")
	fs.BoolVar(&g.Quiet, "quiet", g.Quiet, "print less")
	fs.BoolVar(&g.Quiet, "q", g.Quiet, "shorthand for --quiet")
}

// Color reports whether to use ANSI colour on stdout.
func (g *Globals) Color() bool {
	if g.NoColor || g.JSON || os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("FORCE_COLOR") != "" || os.Getenv("CLICOLOR_FORCE") == "1" {
		return true
	}
	return isTerminal(os.Stdout)
}

type command struct {
	name    string
	aliases []string
	summary string
	usage   string
	run     func(g *Globals, args []string) error
}

var commands []*command

func register(c *command) { commands = append(commands, c) }

func find(name string) *command {
	for _, c := range commands {
		if c.name == name {
			return c
		}
		for _, a := range c.aliases {
			if a == name {
				return c
			}
		}
	}
	return nil
}

// Exit codes.
const (
	ExitOK    = 0
	ExitError = 1 // the command failed, or the code it ran raised an error
	ExitUsage = 2
)

// exitError carries a specific exit code.
type exitError struct {
	code int
	msg  string
}

func (e *exitError) Error() string { return e.msg }

func exitCode(code int, format string, a ...any) error {
	return &exitError{code: code, msg: fmt.Sprintf(format, a...)}
}

// errSilent fails a command whose output already said why.
var errSilent = &exitError{code: ExitError}

func usageError(format string, a ...any) error {
	return exitCode(ExitUsage, format, a...)
}

// Main runs the CLI and returns its exit code.
func Main(args []string) int {
	enableVT()
	g := &Globals{}
	// Global flags may come before the command: sonicpi --watch logs
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") || a == "-" {
			rest = args[i:]
			break
		}
		switch strings.TrimLeft(a, "-") {
		case "watch", "w":
			g.Watch = true
		case "no-color":
			g.NoColor = true
		case "json":
			g.JSON = true
		case "no-autostart":
			g.NoAutostart = true
		case "quiet", "q":
			g.Quiet = true
		case "help", "h":
			printHelp(os.Stdout)
			return ExitOK
		case "version", "v":
			rest = append([]string{"version"}, args[i+1:]...)
			i = len(args)
		default:
			// Not a global flag: let `run` handle it (sonicpi --detach song.rb).
			rest = append([]string{"run"}, args[i:]...)
			i = len(args)
		}
	}
	if len(rest) == 0 {
		printHelp(os.Stderr)
		return ExitUsage
	}
	name, cargs := rest[0], rest[1:]
	c := find(name)
	if c == nil {
		// sonicpi FILE is sonicpi run FILE.
		if name == "-" || strings.HasSuffix(name, ".rb") || isFile(name) {
			c, cargs = find("run"), rest
		} else {
			fmt.Fprintf(os.Stderr, "sonicpi: unknown command or file %q\n\n", name)
			printHelp(os.Stderr)
			return ExitUsage
		}
	}
	err := c.run(g, cargs)
	if err == nil {
		return ExitOK
	}
	var ee *exitError
	if errors.As(err, &ee) {
		if ee.msg != "" {
			fmt.Fprintln(os.Stderr, "sonicpi:", ee.msg)
		}
		if ee.code == ExitUsage && ee.msg != "" {
			fmt.Fprintf(os.Stderr, "\nUsage: %s\n", c.usage)
		}
		return ee.code
	}
	if errors.Is(err, flag.ErrHelp) {
		return ExitOK
	}
	fmt.Fprintln(os.Stderr, "sonicpi:", err)
	return ExitError
}

func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

func printHelp(w io.Writer) {
	fmt.Fprint(w, `sonicpi - Sonic Pi from the command line

Usage:
  sonicpi FILE                 play FILE (same as: sonicpi run FILE)
  sonicpi <command> [args]

Commands:
`)
	cs := append([]*command(nil), commands...)
	sort.Slice(cs, func(i, j int) bool { return cs[i].name < cs[j].name })
	for _, c := range cs {
		fmt.Fprintf(w, "  %-12s %s\n", c.name, c.summary)
	}
	fmt.Fprint(w, `
Global flags (before or after the command):
  -w, --watch        keep watching (follow logs, re-run a file on save)
      --json         JSON output
      --no-color     no ANSI colour
      --no-autostart don't start the background service on demand
  -q, --quiet        print less

Run "sonicpi <command> --help" for a command's flags.
`)
}

// newFlags makes a flag set for a command, with the global flags included.
func newFlags(g *Globals, c string, usage string) *flag.FlagSet {
	fs := flag.NewFlagSet(c, flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s\n\nFlags:\n", usage)
		fs.PrintDefaults()
	}
	g.register(fs)
	return fs
}

// parse parses flags anywhere among the arguments (until "--") and returns
// the positional ones.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var tail []string
	for i, a := range args {
		if a == "--" {
			tail = args[i+1:]
			args = args[:i]
			break
		}
	}
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil, err
			}
			return nil, usageError("%v", err)
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return append(pos, tail...), nil
		}
		pos = append(pos, rest[0])
		args = rest[1:]
	}
}

// boolFlag registers --name and --no-name for one setting.
type boolFlag struct {
	set bool
	val bool
}

func (b *boolFlag) register(fs *flag.FlagSet, name, usage string) {
	fs.Func(name, usage, func(s string) error {
		b.set, b.val = true, s == "" || s == "true"
		return nil
	})
	f := fs.Lookup(name)
	f.DefValue = ""
	fs.Func("no-"+name, "opposite of --"+name, func(string) error {
		b.set, b.val = true, false
		return nil
	})
	markBool(fs, name)
	markBool(fs, "no-"+name)
}

// markBool lets a Func flag be given without a value.
func markBool(fs *flag.FlagSet, name string) {
	f := fs.Lookup(name)
	f.Value = boolValue{f.Value}
}

type boolValue struct{ flag.Value }

func (boolValue) IsBoolFlag() bool { return true }

func (b *boolFlag) or(def bool) bool {
	if b.set {
		return b.val
	}
	return def
}

// connect returns a client for the running service, starting it when
// allowed, and waits for the Sonic Pi server to be ready.
func connect(g *Globals) (*control.Client, error) {
	c, err := ensureService(g)
	if err != nil {
		return nil, err
	}
	var st control.Status
	if err := c.Call("status", nil, &st); err != nil {
		return nil, err
	}
	if st.State != control.StateRunning {
		if !g.Quiet && !g.JSON {
			fmt.Fprintln(os.Stderr, "Waiting for Sonic Pi to boot...")
		}
		if err := c.Call("status", control.WaitArgs{Wait: true, Timeout: 180}, &st); err != nil {
			return nil, err
		}
	}
	switch st.State {
	case control.StateRunning:
		return c, nil
	case control.StateFailed:
		return nil, fmt.Errorf("the Sonic Pi server failed to boot:\n%s\n\nSee `sonicpi logs --source daemon` and `sonicpi doctor`", indentText(st.Error))
	}
	return nil, fmt.Errorf("the Sonic Pi server is %s", st.State)
}

// ensureService returns a client for the service, starting it if needed.
func ensureService(g *Globals) (*control.Client, error) {
	if c, err := control.Dial(); err == nil {
		return c, nil
	}
	if g.NoAutostart || os.Getenv("SONICPI_NO_AUTOSTART") != "" {
		return nil, fmt.Errorf("%w (start it with `sonicpi service start`)", control.ErrNotRunning)
	}
	if err := startService(g); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if c, err := control.Dial(); err == nil {
			return c, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil, fmt.Errorf("the service did not start; see %s", filepath.Join(paths.LogDir(), "sonicpid.out"))
}

func startService(g *Globals) error {
	if svcmgr.Installed() {
		if err := svcmgr.Start(); err == nil {
			return nil
		}
	}
	exe, err := svcmgr.DaemonExe()
	if err != nil {
		return fmt.Errorf("cannot find sonicpid: %w", err)
	}
	logDir := paths.LogDir()
	os.MkdirAll(logDir, 0o755)
	if !g.Quiet && !g.JSON {
		fmt.Fprintln(os.Stderr, "Starting the Sonic Pi service...")
	}
	return svcmgr.Spawn(exe, filepath.Join(logDir, "sonicpid.out"))
}

// spider sends a message to the Sonic Pi server's API.
func spider(c *control.Client, address string, args ...control.Arg) error {
	return c.Call("send", control.SendArgs{Target: control.ToSpider, Address: address, Args: args}, nil)
}

func indentText(s string) string {
	return "  " + strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", "\n  ")
}

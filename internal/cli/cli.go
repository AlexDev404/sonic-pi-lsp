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
	"strconv"
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
	fs.Usage = func() { printUsage(os.Stderr, fs, usage) }
	g.register(fs)
	return fs
}

// globalFlags are the flags every command takes, listed apart in help.
var globalFlags = map[string]bool{"watch": true, "w": true, "no-color": true, "json": true, "no-autostart": true, "quiet": true, "q": true}

// printUsage prints a command's flags GNU style: a shorthand beside its
// long flag (-d, --detach), --x and --no-x on one line, and the global
// flags at the end.
func printUsage(w io.Writer, fs *flag.FlagSet, usage string) {
	fmt.Fprintf(w, "Usage: %s\n", usage)
	short := map[string]string{} // long name -> shorthand
	skip := map[string]bool{}
	fs.VisitAll(func(f *flag.Flag) {
		if long, ok := strings.CutPrefix(f.Usage, "shorthand for --"); ok && len(f.Name) == 1 {
			short[long] = f.Name
			skip[f.Name] = true
		}
		if strings.HasPrefix(f.Usage, "opposite of --") {
			skip[f.Name] = true
		}
	})
	type row struct{ left, usage string }
	var own, global []row
	fs.VisitAll(func(f *flag.Flag) {
		if skip[f.Name] {
			return
		}
		left := "    "
		if s, ok := short[f.Name]; ok {
			left = "-" + s + ", "
		}
		if len(f.Name) == 1 {
			left += "-" + f.Name
		} else {
			left += "--" + f.Name
		}
		if _, isSwitch := f.Value.(*switchValue); isSwitch {
			left += ", --no-" + f.Name
		}
		typ, text := flag.UnquoteUsage(f)
		if typ != "" {
			left += " " + strings.ToUpper(typ)
		}
		switch f.DefValue {
		case "", "0", "false", "-1", "0s":
		default:
			text += " (default " + f.DefValue + ")"
		}
		r := row{left, text}
		if globalFlags[f.Name] {
			global = append(global, r)
		} else {
			own = append(own, r)
		}
	})
	width := 0
	for _, r := range append(own, global...) {
		width = max(width, len(r.left))
	}
	show := func(title string, rows []row) {
		if len(rows) == 0 {
			return
		}
		fmt.Fprintf(w, "\n%s:\n", title)
		for _, r := range rows {
			fmt.Fprintf(w, "  %-*s  %s\n", width, r.left, r.usage)
		}
	}
	show("Flags", own)
	show("Global flags", global)
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
	fs.Var(&switchValue{b: b, on: true}, name, usage)
	fs.Var(&switchValue{b: b, on: false}, "no-"+name, "opposite of --"+name)
}

// switchValue is one side of a --name / --no-name pair. Its methods must
// work on the zero value, which the flag package makes to print help.
type switchValue struct {
	b  *boolFlag
	on bool
}

func (v *switchValue) String() string   { return "" }
func (v *switchValue) IsBoolFlag() bool { return true }

func (v *switchValue) Set(s string) error {
	val, err := strconv.ParseBool(s)
	if err != nil {
		return fmt.Errorf("takes true or false, got %q", s)
	}
	v.b.set, v.b.val = true, val == v.on
	return nil
}

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

package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/AlexDev404/sonic-pi-lsp/internal/paths"
)

func init() {
	register(&command{
		name:    "lint",
		summary: "check files for errors against Sonic Pi's own synths, FX, samples and API",
		usage:   "sonicpi lint [flags] FILE|DIR...   (sonicpi lint --help for all flags)",
		run:     cmdLint,
	})
	register(&command{
		name:    "fmt",
		aliases: []string{"format"},
		summary: "auto-align code with the desktop's beautifier",
		usage:   "sonicpi fmt [-w|--check|--diff] FILE...",
		run:     cmdFmt,
	})
}

// rubyTool runs the bundled Ruby tool (lint/splint.rb) with args and
// returns its exit code.
func rubyTool(g *Globals, args []string) (int, error) {
	return rubyToolIn(g, args, os.Stdin)
}

func rubyToolIn(g *Globals, args []string, stdin io.Reader) (int, error) {
	root, err := paths.Root()
	if err != nil {
		return 0, err
	}
	dir, err := paths.LintDir()
	if err != nil {
		return 0, err
	}
	full := []string{filepath.Join(dir, "splint.rb"), "--sonic-pi-root", root}
	if g.Color() {
		full = append(full, "--color")
	}
	full = append(full, args...)
	cmd := exec.Command(paths.Ruby(root), full...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, os.Stdout, os.Stderr
	err = cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	if err != nil {
		return 0, fmt.Errorf("running the linter with %s: %w", paths.Ruby(root), err)
	}
	return 0, nil
}

// passGlobals turns global flags into the Ruby tool's own.
func passGlobals(g *Globals, args []string) []string {
	if g.JSON {
		args = append([]string{"--format", "json"}, args...)
	}
	if g.Quiet {
		args = append([]string{"--quiet"}, args...)
	}
	return args
}

// stripGlobals takes out of args the shared flags that cmd uses (the Go
// side handles them) and leaves everything else for the Ruby tool. So
// fmt's -w stays --write: fmt does not take --watch.
func stripGlobals(g *Globals, cmd string, args []string) []string {
	uses := globalsFor(cmd)
	var out []string
	for i, a := range args {
		if a == "--" {
			return append(out, args[i:]...)
		}
		name := strings.TrimLeft(a, "-")
		switch name {
		case "w":
			name = gWatch
		case "q":
			name = gQuiet
		}
		if !strings.HasPrefix(a, "-") || !slices.Contains(uses, name) {
			out = append(out, a)
			continue
		}
		switch name {
		case gJSON:
			g.JSON = true
		case gNoColor:
			g.NoColor = true
		case gQuiet:
			g.Quiet = true
		case gWatch:
			g.Watch = true
		case gNoAutostart:
			g.NoAutostart = true
		}
	}
	return out
}

func cmdLint(g *Globals, args []string) error {
	args = stripGlobals(g, "lint", args)
	args = passGlobals(g, args)
	if g.Watch {
		args = append([]string{"--watch"}, args...)
	}
	code, err := rubyTool(g, append([]string{"lint"}, args...))
	if err != nil {
		return err
	}
	if code != 0 {
		return &exitError{code: code}
	}
	return nil
}

func cmdFmt(g *Globals, args []string) error {
	args = stripGlobals(g, "fmt", args)
	code, err := rubyTool(g, append([]string{"fmt"}, args...))
	if err != nil {
		return err
	}
	if code != 0 {
		return &exitError{code: code}
	}
	return nil
}

// lintCodeBeforeRun lints code that is not in a file (eval) the same way.
func lintCodeBeforeRun(g *Globals, name, code string) error {
	code2, err := rubyToolIn(g, []string{"lint", "--use-defaults", "--fail-on", "error", "--quiet", "--stdin", "--stdin-filename", name}, strings.NewReader(code))
	if err != nil {
		return err
	}
	if code2 != 0 {
		return exitCode(code2, "not playing it: the linter found errors")
	}
	return nil
}

// lintBeforeRun lints file and fails if it has errors (warnings pass).
func lintBeforeRun(g *Globals, file string) error {
	code, err := rubyTool(g, []string{"lint", "--use-defaults", "--fail-on", "error", "--quiet", file})
	if err != nil {
		return err
	}
	if code != 0 {
		return exitCode(code, "not playing %s: the linter found errors", file)
	}
	return nil
}

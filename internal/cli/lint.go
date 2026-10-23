package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
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

// stripGlobals removes the global flags the Go side already handled.
func stripGlobals(g *Globals, args []string) []string {
	var out []string
	for i, a := range args {
		if a == "--" {
			return append(out, args[i:]...)
		}
		switch a {
		case "--json", "-json":
			g.JSON = true
		case "--no-color", "-no-color":
			g.NoColor = true
		case "--quiet", "-quiet", "-q":
			g.Quiet = true
		case "--watch", "-watch", "-w":
			g.Watch = true
		default:
			out = append(out, a)
		}
	}
	return out
}

func cmdLint(g *Globals, args []string) error {
	args = stripGlobals(g, args)
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
	args = stripGlobals(g, args)
	code, err := rubyTool(g, append([]string{"fmt"}, args...))
	if err != nil {
		return err
	}
	if code != 0 {
		return &exitError{code: code}
	}
	return nil
}

// lintBeforeRun lints file and fails if it has errors (warnings pass).
func lintBeforeRun(g *Globals, file string) error {
	code, err := rubyTool(g, []string{"lint", "--fail-on", "error", "--quiet", file})
	if err != nil {
		return err
	}
	if code != 0 {
		return exitCode(code, "not playing %s: the linter found errors", file)
	}
	return nil
}

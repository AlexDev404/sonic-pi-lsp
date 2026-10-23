package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/AlexDev404/sonic-pi-lsp/internal/winpath"
)

func init() {
	register(&command{
		name:    "setup",
		summary: "installer helpers: add sonicpi to the Windows system PATH, or remove it",
		usage:   "sonicpi setup path add|remove [DIR]",
		run:     cmdSetup,
	})
}

func cmdSetup(g *Globals, args []string) error {
	fs := newFlags(g, "setup", "sonicpi setup path add|remove [DIR]")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 2 || pos[0] != "path" || (pos[1] != "add" && pos[1] != "remove") {
		return usageError("setup takes: path add [DIR] or path remove [DIR]")
	}
	dir := ""
	if len(pos) > 2 {
		dir = pos[2]
	} else {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		dir = filepath.Dir(exe)
	}
	if pos[1] == "add" {
		err = winpath.Add(dir)
	} else {
		err = winpath.Remove(dir)
	}
	if err != nil {
		return err
	}
	if !g.Quiet {
		fmt.Fprintf(os.Stderr, "System PATH: %s %s\n", map[string]string{"add": "added", "remove": "removed"}[pos[1]], dir)
	}
	return nil
}

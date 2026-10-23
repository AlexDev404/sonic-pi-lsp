package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/AlexDev404/sonic-pi-lsp/internal/control"
	"github.com/AlexDev404/sonic-pi-lsp/internal/paths"
)

func init() {
	register(&command{
		name:    "buffer",
		aliases: []string{"buffers", "buf"},
		summary: "the desktop's ten buffers: list, show, run, edit, load a file into one",
		usage:   "sonicpi buffer [list|show N|run N|edit N|load N FILE|save N FILE]   (the flags are for buffer run)",
		run:     cmdBuffer,
	})
}

var bufferNames = []string{"zero", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine"}

func bufferID(n int) string { return "workspace_" + bufferNames[n] }

func bufferPath(n int) string { return filepath.Join(paths.StoreDir(), bufferID(n)+".spi") }

func bufferArg(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 || n > 9 {
		return 0, usageError("buffers are numbered 0 to 9, got %q", s)
	}
	return n, nil
}

func readBuffer(n int) (string, error) {
	b, err := os.ReadFile(bufferPath(n))
	if os.IsNotExist(err) {
		return "", nil
	}
	return string(b), err
}

func cmdBuffer(g *Globals, args []string) error {
	fs := newFlags(g, "buffer", "sonicpi buffer [list|show N|run N|edit N|load N FILE|save N FILE]\n\nThe flags below apply to buffer run, which plays like sonicpi run.")
	var r runFlags
	r.register(fs)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	sub := "list"
	if len(pos) > 0 {
		sub, pos = pos[0], pos[1:]
	}
	if sub == "list" || sub == "ls" {
		for i := range bufferNames {
			code, _ := readBuffer(i)
			first := ""
			for _, l := range strings.Split(code, "\n") {
				if t := strings.TrimSpace(l); t != "" {
					first = t
					break
				}
			}
			if len(first) > 60 {
				first = first[:57] + "..."
			}
			lines := 0
			if code != "" {
				lines = strings.Count(strings.TrimRight(code, "\n"), "\n") + 1
			}
			fmt.Printf("%d  %4d lines  %s\n", i, lines, first)
		}
		if !g.Quiet {
			fmt.Printf("\n%s\n", paths.StoreDir())
		}
		return nil
	}
	if len(pos) == 0 {
		return usageError("buffer %s needs a buffer number", sub)
	}
	n, err := bufferArg(pos[0])
	if err != nil {
		return err
	}
	switch sub {
	case "show", "cat":
		code, err := readBuffer(n)
		if err != nil {
			return err
		}
		fmt.Print(code)
		if code != "" && !strings.HasSuffix(code, "\n") {
			fmt.Println()
		}
		return nil
	case "path":
		fmt.Println(bufferPath(n))
		return nil
	case "run", "play":
		code, err := readBuffer(n)
		if err != nil {
			return err
		}
		if r.lint {
			if err := lintBeforeRun(g, bufferPath(n)); err != nil {
				return err
			}
		}
		return play(g, &r, fmt.Sprintf("buffer %d", n), code, func() (string, bool) {
			c, err := readBuffer(n)
			return c, err == nil
		}, bufferPath(n))
	case "save", "export":
		if len(pos) != 2 {
			return usageError("buffer save takes a buffer number and a file")
		}
		code, err := readBuffer(n)
		if err != nil {
			return err
		}
		if pos[1] == "-" {
			_, err := io.WriteString(os.Stdout, code)
			return err
		}
		return os.WriteFile(pos[1], []byte(code), 0o644)
	case "load", "import":
		if len(pos) != 2 {
			return usageError("buffer load takes a buffer number and a file")
		}
		var b []byte
		if pos[1] == "-" {
			b, err = io.ReadAll(os.Stdin)
		} else {
			b, err = os.ReadFile(pos[1])
		}
		if err != nil {
			return err
		}
		return writeBuffer(n, string(b))
	case "edit":
		before, _ := os.Stat(bufferPath(n))
		if before == nil {
			os.MkdirAll(paths.StoreDir(), 0o755)
			os.WriteFile(bufferPath(n), nil, 0o644)
		}
		if err := edit(bufferPath(n)); err != nil {
			return err
		}
		code, err := readBuffer(n)
		if err != nil {
			return err
		}
		// Save through the server too, so it records the change in the
		// buffers' git history as it does for the desktop.
		if c, err := control.Dial(); err == nil {
			spider(c, "/save-buffer", control.Str(bufferID(n)), control.Str(code))
		}
		return nil
	}
	return usageError("unknown buffer command %q", sub)
}

// writeBuffer saves a buffer through the server when it is running (which
// keeps the buffers' git history), or directly otherwise.
func writeBuffer(n int, code string) error {
	if c, err := control.Dial(); err == nil {
		if err := spider(c, "/save-buffer", control.Str(bufferID(n)), control.Str(code)); err == nil {
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				if cur, _ := readBuffer(n); cur == code {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			fmt.Fprintln(os.Stderr, "Saved. The desktop app shows it the next time it starts (it keeps open buffers in memory).")
			return nil
		}
	}
	if err := os.MkdirAll(paths.StoreDir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(bufferPath(n), []byte(code), 0o644)
}

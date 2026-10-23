package cli

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/AlexDev404/sonic-pi-lsp/internal/control"
)

func init() {
	register(&command{
		name:    "repl",
		summary: "type code and hear it at once (like Sonic Pi's repl.rb)",
		usage:   "sonicpi repl [FILE]",
		run:     cmdRepl,
	})
}

var (
	openers    = regexp.MustCompile(`(^|[^\w.])(do|begin|case|if|unless|while|until|def|class|module)\b`)
	trailingIf = regexp.MustCompile(`\S.*\b(if|unless|while|until)\b`)
	closers    = regexp.MustCompile(`(^|[^\w.])end\b`)
)

// depth estimates how many blocks a line opens, so a `live_loop ... do`
// line keeps reading until its `end`.
func depth(line string) int {
	l := line
	if i := strings.Index(l, "#"); i >= 0 {
		l = l[:i]
	}
	n := 0
	for _, m := range openers.FindAllStringSubmatch(l, -1) {
		if m[2] == "if" || m[2] == "unless" || m[2] == "while" || m[2] == "until" {
			// a modifier (`play 60 if x`) opens nothing
			if trailingIf.MatchString(l) && !strings.HasPrefix(strings.TrimSpace(l), m[2]) && !strings.Contains(l, "= "+m[2]) {
				continue
			}
		}
		n++
	}
	return n - len(closers.FindAllString(l, -1))
}

func cmdRepl(g *Globals, args []string) error {
	fs := newFlags(g, "repl", "sonicpi repl [FILE]")
	var r runFlags
	r.register(fs)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	pre, st, err := r.preamble()
	if err != nil {
		return err
	}
	c, err := connect(g)
	if err != nil {
		return err
	}
	s := newSession(g, c, r.filter(st), "repl", "")
	if err := s.open(); err != nil {
		return err
	}
	defer s.close()

	var mu sync.Mutex
	logOn := true
	go func() {
		for e := range s.events {
			mu.Lock()
			if logOn {
				fmt.Print("\r\x1b[K")
				s.show(e)
				fmt.Print("sonicpi> ")
			}
			mu.Unlock()
		}
	}()
	run := func(code string) {
		if err := c.Call("run", control.RunArgs{Code: pre + code}, nil); err != nil {
			fmt.Fprintln(os.Stderr, "sonicpi:", err)
		}
	}
	if len(pos) == 1 {
		b, err := os.ReadFile(pos[0])
		if err != nil {
			return err
		}
		run(string(b))
	}
	fmt.Println(`Sonic Pi REPL. Type code and press enter to play it; blocks run when closed.
.help for commands, .stop (or .) to stop, .quit to leave.`)
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 1<<20)
	var buf []string
	level := 0
	prompt := func() {
		mu.Lock()
		if level > 0 {
			fmt.Print("    ...> ")
		} else {
			fmt.Print("sonicpi> ")
		}
		mu.Unlock()
	}
	prompt()
	for in.Scan() {
		line := in.Text()
		t := strings.TrimSpace(line)
		if level == 0 && strings.HasPrefix(t, ".") || t == "," {
			f := strings.Fields(t)
			switch f[0] {
			case ".", ".stop":
				spider(c, "/stop-all-jobs")
			case ".q", ".quit", ".exit":
				return nil
			case ".l", ".log":
				mu.Lock()
				logOn = !logOn
				mu.Unlock()
				fmt.Println("log", map[bool]string{true: "on", false: "off"}[logOn])
			case ".a", ".amp":
				if len(f) == 2 {
					if v, err := strconv.ParseFloat(f[1], 64); err == nil {
						spider(c, "/mixer-amp", control.Float(v), control.Int(1))
					}
				}
			case ".vol", ".volume":
				if len(f) == 2 {
					cmdVolume(g, f[1:])
				}
			case ".run", ".load":
				if len(f) == 2 {
					if b, err := os.ReadFile(f[1]); err == nil {
						run(string(b))
					} else {
						fmt.Fprintln(os.Stderr, err)
					}
				}
			case ",":
				fmt.Println("Multi-line mode: end with a line holding just a comma.")
				var lines []string
				for in.Scan() {
					if strings.TrimSpace(in.Text()) == "," {
						break
					}
					lines = append(lines, in.Text())
				}
				run(strings.Join(lines, "\n"))
			case ".help", ".h", ".?":
				fmt.Println(`  .stop, .        stop everything
  .log, .l        toggle the log
  .amp X, .a X    set the mixer amp (0-2) for this session
  .vol N          set the saved volume (0-100)
  .run FILE       play a file
  ,               multi-line mode (end with a lone comma)
  .quit, .q       leave`)
			default:
				fmt.Println("unknown command; .help lists them")
			}
			prompt()
			continue
		}
		buf = append(buf, line)
		level += depth(line)
		if level <= 0 {
			level = 0
			code := strings.Join(buf, "\n")
			buf = nil
			if strings.TrimSpace(code) != "" {
				run(code)
			}
		}
		prompt()
	}
	return nil
}

package cli

import (
	"strings"
	"testing"
)

func TestParseInterspersed(t *testing.T) {
	g := &Globals{}
	fs := newFlags(g, "x", "x")
	n := fs.Int("n", 0, "")
	var synths boolFlag
	synths.register(fs, "synths", "")
	pos, err := parse(fs, []string{"a", "-n", "3", "--no-synths", "b", "--watch", "--", "-c"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(pos, " ") != "a b -c" || *n != 3 || !g.Watch || !synths.set || synths.val {
		t.Fatalf("pos=%v n=%d watch=%v synths=%+v", pos, *n, g.Watch, synths)
	}
	var cues boolFlag
	fs2 := newFlags(g, "y", "y")
	cues.register(fs2, "cues", "")
	if _, err := parse(fs2, []string{"--cues"}); err != nil || !cues.or(false) {
		t.Fatalf("--cues: %+v %v", cues, err)
	}
	if cues2 := (boolFlag{}); cues2.or(true) != true {
		t.Fatal("default ignored")
	}
}

func TestGlobalFlagsBeforeCommand(t *testing.T) {
	// sonicpi --watch logs --path prints the log directory and exits.
	if code := Main([]string{"--watch", "logs", "--path"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if code := Main([]string{"no-such-command"}); code != ExitUsage {
		t.Fatalf("exit %d", code)
	}
}

func TestReplDepth(t *testing.T) {
	cases := map[string]int{
		"live_loop :a do":           1,
		"end":                       -1,
		"play 60 if one_in(2)":      0,
		"if x then":                 1,
		"8.times do |i| play i end": 0,
		"x = [1, 2].map { |v| v }":  0,
		"# do":                      0,
	}
	for line, want := range cases {
		if got := depth(line); got != want {
			t.Errorf("depth(%q) = %d, want %d", line, got, want)
		}
	}
}

func TestFmtKeepsShortWrite(t *testing.T) {
	g := &Globals{}
	got := stripGlobals(g, []string{"-w", "song.rb", "--json"}, "-w")
	if strings.Join(got, " ") != "-w song.rb" || g.Watch || !g.JSON {
		t.Fatalf("got %v, watch=%v json=%v", got, g.Watch, g.JSON)
	}
	// lint keeps -w as --watch.
	g = &Globals{}
	if got := stripGlobals(g, []string{"-w", "song.rb"}); strings.Join(got, " ") != "song.rb" || !g.Watch {
		t.Fatalf("lint: got %v, watch=%v", got, g.Watch)
	}
}

// TestHelpForEveryCommand prints each command's --help, which used to
// panic for --x/--no-x switches.
func TestHelpForEveryCommand(t *testing.T) {
	for _, c := range commands {
		if c.name == "lint" || c.name == "fmt" {
			continue // their help comes from the Ruby tool
		}
		g := &Globals{}
		fs := newFlags(g, c.name, c.usage)
		var r runFlags
		r.register(fs)
		var b strings.Builder
		printUsage(&b, fs, c.usage)
		out := b.String()
		for _, bad := range []string{"panic", "\n  -check-args", "zero cli."} {
			if strings.Contains(out, bad) {
				t.Fatalf("%s help contains %q:\n%s", c.name, bad, out)
			}
		}
		if !strings.Contains(out, "--synths, --no-synths") || !strings.Contains(out, "-d, --detach") {
			t.Fatalf("%s help:\n%s", c.name, out)
		}
	}
	// The REPL takes the log and preference flags, not the playback ones.
	g := &Globals{}
	fs := newFlags(g, "repl", "sonicpi repl [FILE]")
	var r runFlags
	r.registerSession(fs)
	var b strings.Builder
	printUsage(&b, fs, "sonicpi repl [FILE]")
	if out := b.String(); strings.Contains(out, "--detach") || strings.Contains(out, "--record") || !strings.Contains(out, "--synths, --no-synths") {
		t.Fatalf("repl help:\n%s", out)
	}
}

func TestSwitchValues(t *testing.T) {
	g := &Globals{}
	fs := newFlags(g, "x", "x")
	var b boolFlag
	b.register(fs, "synths", "")
	if _, err := parse(fs, []string{"--synths=false"}); err != nil || !b.set || b.val {
		t.Fatalf("--synths=false: %+v %v", b, err)
	}
	if _, err := parse(fs, []string{"--no-synths=false"}); err != nil || !b.val {
		t.Fatalf("--no-synths=false: %+v %v", b, err)
	}
}

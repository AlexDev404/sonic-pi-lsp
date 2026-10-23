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

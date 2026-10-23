package events

import (
	"strings"
	"testing"
	"time"

	"github.com/AlexDev404/sonic-pi-lsp/internal/osc"
)

func TestMultiMessage(t *testing.T) {
	m := osc.NewMessage("/log/multi_message", int32(3), ":live_loop_drums", "1.5", int32(2),
		int32(StyleSynth), "synth :beep, {note: 60.0}", int32(StyleUser), "hello")
	e := FromOSC(m, time.Now())
	if e.Kind != KindLog || e.Run != 3 || e.Thread != ":live_loop_drums" || len(e.Lines) != 2 {
		t.Fatalf("got %+v", e)
	}
	out := Formatter{}.Format(e)
	want := "{run: 3, time: 1.5, thread: :live_loop_drums}\n ├─ synth :beep, {note: 60.0}\n └─ hello\n"
	if out != want {
		t.Fatalf("format:\n%q\nwant\n%q", out, want)
	}
}

func TestFilter(t *testing.T) {
	e := &Event{Kind: KindLog, Run: 1, Lines: []Line{{StyleSynth, "synth"}, {StyleUser, "puts"}, {StyleCue, "cue :x"}}}
	f := AllFilter
	f.Synths, f.Cues = false, false
	got := f.Apply(e)
	if got == nil || len(got.Lines) != 1 || got.Lines[0].Text != "puts" {
		t.Fatalf("got %+v", got)
	}
	if len(e.Lines) != 3 {
		t.Fatal("Apply changed its input")
	}
	f.Puts = false
	if f.Apply(e) != nil {
		t.Fatal("an event with no shown lines should be dropped")
	}
	if (Filter{Errors: true}).Apply(&Event{Kind: KindCue}) != nil {
		t.Fatal("cue shown with cues off")
	}
	if (Filter{Errors: true, Run: 2}).Apply(&Event{Kind: KindError, Run: 1}) != nil {
		t.Fatal("other run shown")
	}
}

func TestErrorsAreUnescaped(t *testing.T) {
	e := FromOSC(osc.NewMessage("/error", int32(1), "undefined method `bar&#39; for &lt;x&gt;", "trace", int32(4)), time.Now())
	if e.Text != "undefined method `bar' for <x>" || e.Line != 4 {
		t.Fatalf("got %+v", e)
	}
	if !strings.Contains(Formatter{Source: "song.rb"}.Format(e), "[song.rb, line 4]") {
		t.Fatal("no location")
	}
}

func TestPlainIsTaggedPerLine(t *testing.T) {
	e := &Event{Time: time.Unix(0, 0), Kind: KindLog, Run: 1, Beat: "0", Lines: []Line{{0, "a"}, {0, "b"}}}
	for _, l := range strings.Split(strings.TrimSpace(Plain(e)), "\n") {
		if !strings.Contains(l, " [log] ") {
			t.Fatalf("untagged line %q", l)
		}
	}
}

package prefs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexDev404/sonic-pi-lsp/internal/paths"
)

const desktopINI = `[General]
first_time=0
pos=@Point(100 100)

[prefs]
log-synths=true
system-vol=80
scope\show-scopes=true
language=
`

func TestINIPreservesAndEdits(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gui-settings.ini")
	os.WriteFile(p, []byte(desktopINI), 0o644)
	ini, err := LoadINI(p)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := ini.Get("prefs/scope/show-scopes"); !ok || v != "true" {
		t.Fatalf("scope key: %q %v", v, ok)
	}
	if v, _ := ini.Get("pos"); v != "@Point(100 100)" {
		t.Fatalf("pos: %q", v)
	}
	ini.Set("prefs/log-synths", "false")
	ini.Set("prefs/log-cues", "true")
	ini.Set("prefs/language", "en, GB")
	ini.Set("other/thing", "x")
	if !ini.Unset("prefs/system-vol") {
		t.Fatal("unset failed")
	}
	if err := ini.Save(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	got := string(b)
	for _, want := range []string{"pos=@Point(100 100)\n", "log-synths=false\n", "log-cues=true\n", `language="en, GB"` + "\n", "[other]\nthing=x\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "system-vol") {
		t.Errorf("system-vol not removed:\n%s", got)
	}
	// log-cues is added inside [prefs], before the blank line that ends it.
	if strings.Index(got, "log-cues") > strings.Index(got, "[other]") {
		t.Errorf("log-cues landed outside [prefs]:\n%s", got)
	}
	ini2, _ := LoadINI(p)
	if v, _ := ini2.Get("prefs/language"); v != "en, GB" {
		t.Fatalf("quoted round trip: %q", v)
	}
}

func TestTOMLUncommentsInPlace(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "audio-settings.toml")
	os.WriteFile(p, []byte("# Options\n# sound_card_name = \"\"\n# num_inputs = 16\n"), 0o644)
	tm, _ := LoadTOML(p, "")
	tm.Set("sound_card_name", EncodeTOMLValue("USB Audio", "string"))
	tm.Set("block_size", "128")
	tm.Save()
	b, _ := os.ReadFile(p)
	want := "# Options\nsound_card_name = \"USB Audio\"\n# num_inputs = 16\nblock_size = 128\n"
	if string(b) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", b, want)
	}
	tm, _ = LoadTOML(p, "")
	if v, _ := tm.Get("sound_card_name"); v != "USB Audio" {
		t.Fatalf("get: %q", v)
	}
	tm.Unset("sound_card_name")
	if _, ok := tm.Get("sound_card_name"); ok {
		t.Fatal("still active")
	}
}

func TestPreambleMatchesDesktop(t *testing.T) {
	t.Setenv("SONIC_PI_HOME", t.TempDir())
	st, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	got := st.RunPreamble()
	// Desktop defaults: log synths on, log cues off, arg checks on.
	for _, want := range []string{`use_midi_defaults channel: "*"`, "use_arg_checks true", "use_cue_logging false"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "use_debug false") {
		t.Errorf("use_debug false with log-synths on: %q", got)
	}
	for _, l := range strings.Split(strings.TrimSpace(got), "\n") {
		if !strings.Contains(l, "#__nosave__") {
			t.Errorf("line without #__nosave__: %q", l)
		}
	}
	st.Overrides = map[string]string{"log-synths": "false"}
	if !strings.Contains(st.RunPreamble(), "use_debug false") {
		t.Error("override ignored")
	}
}

func TestValidate(t *testing.T) {
	s, _ := Lookup("system-vol")
	if _, err := s.Validate("101"); err == nil {
		t.Error("101 accepted")
	}
	b, _ := Lookup("log-synths")
	if v, err := b.Validate("off"); err != nil || v != "false" {
		t.Errorf("off: %q %v", v, err)
	}
	if _, err := Lookup("nope"); err == nil {
		t.Error("unknown key accepted")
	}
	if s, err := Lookup("gui.prefs/whatever"); err != nil || s.Name != "prefs/whatever" {
		t.Errorf("raw key: %+v %v", s, err)
	}
}

func TestLayoutFollowsVersion(t *testing.T) {
	if l := paths.LayoutFor(5); l.GUI != "v5-gui-settings.ini" || l.Audio != "v5-audio-settings.toml" {
		t.Fatalf("5: %+v", l)
	}
	if l := paths.LayoutFor(4); l.GUI != "gui-settings.ini" || l.Audio != "audio-settings.toml" {
		t.Fatalf("4: %+v", l)
	}
	vol, _ := Lookup("system-vol")
	out, _ := Lookup("output-volume")
	if vol.For(5) || !vol.For(4) || !out.For(5) || out.For(4) {
		t.Fatal("version gating")
	}
}

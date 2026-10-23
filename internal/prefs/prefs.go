// Package prefs reads and writes the desktop client's configuration in
// ~/.sonic-pi/config, so the CLI and the desktop share one set of settings.
package prefs

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/AlexDev404/sonic-pi-lsp/internal/paths"
)

// File names a configuration file in ~/.sonic-pi/config.
type File string

const (
	GUI   File = "gui"   // gui-settings.ini
	Audio File = "audio" // audio-settings.toml
	Tau   File = "tau"   // tau-settings.toml
)

// Effect says when a change to a setting takes hold.
type Effect int

const (
	// NextRun: read each time code is run.
	NextRun Effect = iota
	// Live: the service applies it to the running server at once.
	Live
	// Restart: the boot daemon reads it, so the server must restart.
	Restart
	// DesktopOnly: only the desktop client uses it.
	DesktopOnly
)

func (e Effect) String() string {
	return [...]string{"next run", "live", "restart", "desktop only"}[e]
}

// Setting describes one known option.
type Setting struct {
	Key     string // what the CLI calls it: "log-synths", "audio.sound_card_name"
	File    File
	Name    string // the key inside the file
	Kind    string // bool, int, float, string
	Default string
	Min     *float64
	Max     *float64
	Effect  Effect
	Doc     string
	// Major limits a setting to one Sonic Pi major version: 4 for 4.x only,
	// 5 for 5 and later, 0 for every version.
	Major int
}

// For reports whether the setting applies to a Sonic Pi major version.
func (s Setting) For(major int) bool {
	switch s.Major {
	case 0:
		return true
	case 4:
		return major < 5
	}
	return major >= s.Major
}

func only(major int, s Setting) Setting {
	s.Major = major
	return s
}

func f(v float64) *float64 { return &v }

func gui(name, kind, def string, eff Effect, doc string) Setting {
	return Setting{Key: strings.TrimPrefix(name, "prefs/"), File: GUI, Name: name, Kind: kind, Default: def, Effect: eff, Doc: doc}
}

func audio(name, kind string, doc string) Setting {
	return Setting{Key: "audio." + name, File: Audio, Name: name, Kind: kind, Effect: Restart, Doc: doc}
}

// Known lists every option the CLI understands, with the desktop's defaults.
var Known = func() []Setting {
	vol := gui("prefs/system-vol", "int", "80", Live, "Main volume, 0-100 (the desktop's volume slider)")
	vol.Min, vol.Max = f(0), f(100)
	out := gui("prefs/output-volume", "int", "100", Live, "Volume: the fader after the limiter, 0-100")
	out.Min, out.Max = f(0), f(100)
	drive := gui("prefs/drive-pct", "int", "192", Live, "Drive: gain before the limiter, in percent (100 is unity, 25-400)")
	drive.Min, drive.Max = f(25), f(400)
	ch := gui("prefs/midi-default-channel", "int", "0", NextRun, "Default MIDI out channel: 0 for all (*), or 1-16")
	ch.Min, ch.Max = f(0), f(16)
	rate := gui("prefs/audio-sample-rate", "int", "0", Restart, "Sample rate, 0 for the device's own")
	buf := gui("prefs/audio-buffer-size", "int", "0", Restart, "Audio buffer size in frames, 0 for the device's own")
	return []Setting{
		only(4, vol),
		only(5, out),
		only(5, drive),
		gui("prefs/log-synths", "bool", "true", NextRun, "Log each synth and sample triggered"),
		gui("prefs/log-cues", "bool", "false", NextRun, "Log cues (cue, sync, live_loop and incoming OSC/MIDI)"),
		gui("prefs/check-args", "bool", "true", NextRun, "Safe mode: check synth and FX opts before they reach scsynth"),
		gui("prefs/enable-external-synths", "bool", "false", NextRun, "Allow synths and FX on external scsynth buses"),
		gui("prefs/synth-trigger-timing-guarantees", "bool", "false", NextRun, "Drop synths that would be triggered late instead of playing them late"),
		ch,
		gui("prefs/midi-enable", "bool", "true", Live, "Enable MIDI input and output"),
		gui("prefs/osc-enabled", "bool", "true", Live, "Open the OSC cue port"),
		gui("prefs/osc-public", "bool", "false", Live, "Let other machines send OSC to the cue port"),
		gui("prefs/mixer-force-mono", "bool", "false", Live, "Mix the output down to mono"),
		gui("prefs/mixer-invert-stereo", "bool", "false", Live, "Swap the left and right channels"),
		gui("prefs/enable-scsynth-inputs", "bool", "false", Restart, "Enable audio inputs (sound_in, live_audio)"),
		only(5, gui("prefs/audio-output-device", "string", "", Restart, "Output device name (empty for the system default; see `sonicpi audio devices`)")),
		only(5, gui("prefs/audio-input-device", "string", "", Restart, "Input device name (empty for none)")),
		only(5, gui("prefs/audio-driver", "string", "", Restart, "Audio driver (ALSA, JACK, Windows Audio, ASIO...; empty for the default)")),
		only(5, rate),
		only(5, buf),
		only(5, gui("prefs/gamepad-enable", "bool", "true", Live, "Enable gamepad input")),
		gui("prefs/rp/check-updates", "bool", "true", Live, "Check for new Sonic Pi releases"),
		gui("prefs/clear-output-on-run", "bool", "true", DesktopOnly, "Clear the log when code is run"),
		gui("prefs/auto-indent-on-run", "bool", "true", DesktopOnly, "Auto-align code when it is run (see `sonicpi fmt`)"),
		gui("prefs/log-auto-scroll", "bool", "true", DesktopOnly, "Scroll the log as it fills"),
		gui("prefs/language", "string", "", DesktopOnly, "Interface language"),
		gui("prefs/theme", "string", "", DesktopOnly, "Colour theme"),
		gui("prefs/show-line-numbers", "bool", "true", DesktopOnly, "Show line numbers"),
		gui("prefs/show-autocompletion", "bool", "true", DesktopOnly, "Show autocompletion"),
		gui("prefs/show-context", "bool", "true", DesktopOnly, "Show the context pane"),
		gui("prefs/show-log", "bool", "true", DesktopOnly, "Show the log"),
		gui("prefs/show_cues", "bool", "true", DesktopOnly, "Show the cue pane"),
		gui("prefs/show_metro", "bool", "true", DesktopOnly, "Show the metronome"),
		gui("prefs/show-buttons", "bool", "true", DesktopOnly, "Show the toolbar"),
		gui("prefs/show-tabs", "bool", "true", DesktopOnly, "Show the buffer tabs"),
		gui("prefs/show-titles", "bool", "true", DesktopOnly, "Show pane titles"),
		gui("prefs/shortcut-mode", "int", "1", DesktopOnly, "Keyboard shortcut mode"),
		gui("prefs/gui_transparency", "int", "0", DesktopOnly, "Window transparency"),
		gui("prefs/hide-menubar-in-fullscreen", "bool", "false", DesktopOnly, "Hide the menu bar in full screen"),
		gui("prefs/scope/show-scopes", "bool", "true", DesktopOnly, "Show scopes"),
		gui("prefs/scope/show-labels", "bool", "false", DesktopOnly, "Show scope labels"),

		audio("sound_card_name", "string", "Sound card to use for input and output"),
		audio("sound_card_sample_rate", "int", "Sample rate"),
		audio("sound_card_buffer_size", "int", "Hardware buffer size"),
		audio("num_inputs", "int", "Number of input channels"),
		audio("num_outputs", "int", "Number of output channels"),
		audio("input_sound_card_name", "string", "Input sound card (overrides sound_card_name)"),
		audio("output_sound_card_name", "string", "Output sound card (overrides sound_card_name)"),
		audio("block_size", "int", "scsynth block size"),
		audio("enable_inputs", "bool", "Enable inputs (overrides enable-scsynth-inputs)"),
		audio("enable_outputs", "bool", "Enable outputs"),
		audio("num_control_bus_channels", "int", "Control bus channels"),
		audio("num_audio_bus_channels", "int", "Audio bus channels"),
		audio("num_sample_buffers", "int", "Sample buffers"),
		audio("max_num_nodes", "int", "Maximum nodes"),
		audio("max_num_synthdefs", "int", "Maximum synthdefs"),
		audio("real_time_memory_size", "int", "Real-time memory, in KB"),
		audio("num_wire_buffers", "int", "Wire buffers"),
		audio("num_random_seeds", "int", "Random seeds"),
		audio("linux_pipewire_buffsize", "int", "PipeWire quantum (Linux)"),
		audio("linux_pipewire_samplerate", "int", "PipeWire sample rate (Linux)"),
		audio("scsynth_opts", "string", "Extra scsynth command-line options"),
		audio("scsynth_opts_override", "string", "Replace every scsynth option with these"),

		{Key: "tau.phx_port", File: Tau, Name: "phx_port", Kind: "int", Effect: Restart, Doc: "Port for Tau's web server"},
		{Key: "tau.env", File: Tau, Name: "env", Kind: "string", Effect: Restart, Doc: "Tau environment: prod or dev"},
	}
}()

// Lookup resolves a CLI key. Besides the known keys it accepts any raw key
// as gui.<QSettings key>, audio.<key> or tau.<key>.
func Lookup(key string) (Setting, error) {
	for _, s := range Known {
		if s.Key == key || (s.File == GUI && s.Name == key) {
			return s, nil
		}
	}
	switch {
	case strings.HasPrefix(key, "gui."):
		return Setting{Key: key, File: GUI, Name: strings.TrimPrefix(key, "gui."), Effect: DesktopOnly}, nil
	case strings.HasPrefix(key, "audio."):
		return Setting{Key: key, File: Audio, Name: strings.TrimPrefix(key, "audio."), Effect: Restart}, nil
	case strings.HasPrefix(key, "tau."):
		return Setting{Key: key, File: Tau, Name: strings.TrimPrefix(key, "tau."), Effect: Restart}, nil
	}
	return Setting{}, fmt.Errorf("unknown setting %q (see `sonicpi config list --all`)", key)
}

// Validate normalises and checks a value for s.
func (s Setting) Validate(v string) (string, error) {
	switch s.Kind {
	case "bool":
		switch strings.ToLower(v) {
		case "true", "on", "yes", "1":
			return "true", nil
		case "false", "off", "no", "0":
			return "false", nil
		}
		return "", fmt.Errorf("%s takes true or false, got %q", s.Key, v)
	case "int", "float":
		n, err := strconv.ParseFloat(v, 64)
		if err != nil || (s.Kind == "int" && n != float64(int64(n))) {
			return "", fmt.Errorf("%s takes a number, got %q", s.Key, v)
		}
		if (s.Min != nil && n < *s.Min) || (s.Max != nil && n > *s.Max) {
			return "", fmt.Errorf("%s must be between %g and %g", s.Key, *s.Min, *s.Max)
		}
	}
	return v, nil
}

// Store holds the loaded configuration files.
type Store struct {
	// Overrides replace stored values for this process only (command-line
	// flags such as `sonicpi run --no-log-synths`), keyed by Setting.Key.
	Overrides map[string]string

	gui   *INI
	audio *TOML
	tau   *TOML
	root  string
}

// Load reads the configuration. root is the Sonic Pi tree, used to seed
// audio-settings.toml from the desktop's commented template; it may be "".
func Load(root string) (*Store, error) {
	g, err := LoadINI(paths.GUISettingsPath())
	if err != nil {
		return nil, err
	}
	tmpl := ""
	if root != "" {
		tmpl = filepath.Join(root, "app", "config", "user-examples", paths.CurrentLayout().Audio)
	}
	a, err := LoadTOML(paths.AudioSettingsPath(), tmpl)
	if err != nil {
		return nil, err
	}
	t, err := LoadTOML(paths.TauSettingsPath(), "")
	if err != nil {
		return nil, err
	}
	return &Store{gui: g, audio: a, tau: t, root: root}, nil
}

func (st *Store) toml(f File) *TOML {
	if f == Tau {
		return st.tau
	}
	return st.audio
}

// Get returns a setting's value, and whether it is set (rather than default).
func (st *Store) Get(s Setting) (string, bool) {
	if v, ok := st.Overrides[s.Key]; ok {
		return v, true
	}
	if s.File == GUI {
		if v, ok := st.gui.Get(s.Name); ok {
			return v, true
		}
		return s.Default, false
	}
	return st.toml(s.File).Get(s.Name)
}

// Set stores a value after validating it.
func (st *Store) Set(s Setting, v string) error {
	v, err := s.Validate(v)
	if err != nil {
		return err
	}
	if s.File == GUI {
		st.gui.Set(s.Name, v)
		return st.gui.Save()
	}
	t := st.toml(s.File)
	t.Set(s.Name, EncodeTOMLValue(v, s.Kind))
	return t.Save()
}

// Unset returns a setting to its default.
func (st *Store) Unset(s Setting) error {
	if s.File == GUI {
		if st.gui.Unset(s.Name) {
			return st.gui.Save()
		}
		return nil
	}
	t := st.toml(s.File)
	if t.Unset(s.Name) {
		return t.Save()
	}
	return nil
}

// Bool reads a boolean setting by key, falling back to its default.
func (st *Store) Bool(key string) bool {
	s, err := Lookup(key)
	if err != nil {
		return false
	}
	v, _ := st.Get(s)
	b, _ := strconv.ParseBool(strings.ToLower(v))
	return b
}

// Int reads an integer setting by key, falling back to its default.
func (st *Store) Int(key string) int {
	s, err := Lookup(key)
	if err != nil {
		return 0
	}
	v, _ := st.Get(s)
	n, _ := strconv.ParseFloat(v, 64)
	return int(n)
}

// Unknown lists keys present in the files that Known does not describe.
func (st *Store) Unknown() []string {
	var out []string
	known := map[string]bool{}
	for _, s := range Known {
		known[string(s.File)+":"+s.Name] = true
	}
	for _, k := range st.gui.Keys() {
		if !known["gui:"+k] {
			out = append(out, "gui."+k)
		}
	}
	for _, k := range st.audio.Active() {
		if !known["audio:"+k] {
			out = append(out, "audio."+k)
		}
	}
	for _, k := range st.tau.Active() {
		if !known["tau:"+k] {
			out = append(out, "tau."+k)
		}
	}
	sort.Strings(out)
	return out
}

// PathOf returns the file a setting lives in.
func PathOf(f File) string {
	switch f {
	case Audio:
		return paths.AudioSettingsPath()
	case Tau:
		return paths.TauSettingsPath()
	}
	return paths.GUISettingsPath()
}

// RunPreamble returns the lines the desktop client puts before code it runs,
// built from the same preferences. Each line carries the #__nosave__ marker,
// which the server strips when it reports line numbers.
func (st *Store) RunPreamble() string {
	const tag = " #__nosave__ set by sonicpi CLI user preferences.\n"
	var b strings.Builder
	// The desktop prepends each line, so the last one it adds comes first.
	ch := st.Int("midi-default-channel")
	chs := "*"
	if ch > 0 {
		chs = strconv.Itoa(ch)
	}
	b.WriteString(`use_midi_defaults channel: "` + chs + `"` + tag)
	if st.Bool("synth-trigger-timing-guarantees") {
		b.WriteString("use_timing_guarantees true" + tag)
	}
	if st.Bool("enable-external-synths") {
		b.WriteString("use_external_synths true" + tag)
	}
	if st.Bool("check-args") {
		b.WriteString("use_arg_checks true" + tag)
	}
	if !st.Bool("log-cues") {
		b.WriteString("use_cue_logging false" + tag)
	}
	if !st.Bool("log-synths") {
		b.WriteString("use_debug false" + tag)
	}
	return b.String()
}

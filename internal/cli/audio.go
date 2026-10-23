package cli

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/AlexDev404/sonic-pi-lsp/internal/control"
	"github.com/AlexDev404/sonic-pi-lsp/internal/paths"
	"github.com/AlexDev404/sonic-pi-lsp/internal/prefs"
)

func init() {
	register(&command{
		name:    "volume",
		aliases: []string{"vol"},
		summary: "show or set the main volume (0-100), shared with the desktop",
		usage:   "sonicpi volume [0-100]",
		run:     cmdVolume,
	})
	register(&command{
		name:    "mixer",
		summary: "mono/stereo, invert stereo, and the main high/low pass filters",
		usage:   "sonicpi mixer [--mono|--stereo] [--invert|--no-invert] [--hpf HZ|--no-hpf] [--lpf HZ|--no-lpf]",
		run:     cmdMixer,
	})
	register(&command{
		name:    "record",
		aliases: []string{"rec"},
		summary: "record the output to a .wav file",
		usage:   "sonicpi record FILE.wav [--duration 30s] | record start|stop|save FILE|discard",
		run:     cmdRecord,
	})
}

func loadPrefs() (*prefs.Store, error) {
	root, _ := paths.Root()
	return prefs.Load(root)
}

func setPref(st *prefs.Store, key, value string) error {
	s, err := prefs.Lookup(key)
	if err != nil {
		return err
	}
	return st.Set(s, value)
}

// volumeKey is the desktop's volume setting: Sonic Pi 5's output fader, or
// 4.x's single volume slider.
func volumeKey() string {
	if paths.CurrentLayout().Major >= 5 {
		return "output-volume"
	}
	return "system-vol"
}

func cmdVolume(g *Globals, args []string) error {
	fs := newFlags(g, "volume", "sonicpi volume [0-100]")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	st, err := loadPrefs()
	if err != nil {
		return err
	}
	key := volumeKey()
	if len(pos) == 0 {
		fmt.Println(st.Int(key))
		return nil
	}
	v, err := strconv.Atoi(pos[0])
	if err != nil || v < 0 || v > 100 {
		return usageError("volume takes a number from 0 to 100")
	}
	if err := setPref(st, key, strconv.Itoa(v)); err != nil {
		return err
	}
	if c, err := control.Dial(); err == nil {
		return c.Call("apply-prefs", nil, nil)
	}
	return nil
}

func cmdMixer(g *Globals, args []string) error {
	fs := newFlags(g, "mixer", "sonicpi mixer [flags]")
	mono := fs.Bool("mono", false, "mix down to mono (saved, like the desktop's setting)")
	stereo := fs.Bool("stereo", false, "stereo output (saved)")
	var invert boolFlag
	invert.register(fs, "invert", "swap left and right (saved)")
	hpf := fs.Float64("hpf", -1, "main high pass filter cutoff, as a MIDI note (0-118)")
	noHPF := fs.Bool("no-hpf", false, "turn the main high pass filter off")
	lpf := fs.Float64("lpf", -1, "main low pass filter cutoff, as a MIDI note (0-130)")
	noLPF := fs.Bool("no-lpf", false, "turn the main low pass filter off")
	var limiter boolFlag
	limiter.register(fs, "limiter", "the final limiter (on by default)")
	slide := fs.Float64("slide", 0, "slide filter changes over this many beats")
	reset := fs.Bool("reset", false, "reset the filters and limiter to their defaults")
	amp := fs.Float64("amp", -1, "set the mixer amp directly (0-2) without saving it (Sonic Pi 4)")
	drive := fs.Int("drive", -1, "drive: gain before the limiter in percent, 100 is unity (saved; Sonic Pi 5)")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	if *mono && *stereo {
		return usageError("--mono and --stereo together")
	}
	st, err := loadPrefs()
	if err != nil {
		return err
	}
	if *mono || *stereo {
		if err := setPref(st, "mixer-force-mono", strconv.FormatBool(*mono)); err != nil {
			return err
		}
	}
	if invert.set {
		if err := setPref(st, "mixer-invert-stereo", strconv.FormatBool(invert.val)); err != nil {
			return err
		}
	}
	if *drive >= 0 {
		if err := setPref(st, "drive-pct", strconv.Itoa(*drive)); err != nil {
			return err
		}
	}
	var ctl []string
	add := func(on bool, opt string) {
		if on {
			ctl = append(ctl, opt)
		}
	}
	add(*hpf >= 0, fmt.Sprintf("hpf: %g, hpf_bypass: 0", *hpf))
	add(*noHPF, "hpf: 0")
	add(*lpf >= 0, fmt.Sprintf("lpf: %g, lpf_bypass: 0", *lpf))
	add(*noLPF, "lpf: 135.5")
	add(limiter.set, fmt.Sprintf("limiter_bypass: %d", map[bool]int{true: 0, false: 1}[limiter.val]))
	add(len(ctl) > 0 && *slide > 0, fmt.Sprintf("hpf_slide: %g, lpf_slide: %g", *slide, *slide))
	nothing := !*mono && !*stereo && !invert.set && len(ctl) == 0 && !*reset && *amp < 0 && *drive < 0
	if nothing {
		fmt.Printf("mono:   %v\ninvert: %v\nvolume: %d\n", st.Bool("mixer-force-mono"), st.Bool("mixer-invert-stereo"), st.Int(volumeKey()))
		if paths.CurrentLayout().Major >= 5 {
			fmt.Printf("drive:  %d%%\n", st.Int("drive-pct"))
		}
		return nil
	}
	c, err := connect(g)
	if err != nil {
		return err
	}
	if *mono || *stereo || invert.set || *drive >= 0 {
		if err := c.Call("apply-prefs", nil, nil); err != nil {
			return err
		}
	}
	// The server's own /mixer-hpf-* endpoints are not wired up, so the
	// filters go through the language, as set_mixer_control! in a buffer would.
	code := ""
	if *reset {
		code += "reset_mixer!\n"
	}
	if len(ctl) > 0 {
		code += "set_mixer_control! " + strings.Join(ctl, ", ") + "\n"
	}
	if code != "" {
		if err := c.Call("run", control.RunArgs{Code: "use_debug false #__nosave__\n" + code}, nil); err != nil {
			return err
		}
	}
	if *amp >= 0 {
		return spider(c, "/mixer-amp", control.Float(*amp), control.Int(0))
	}
	return nil
}

func cmdRecord(g *Globals, args []string) error {
	fs := newFlags(g, "record", "sonicpi record FILE.wav [--duration D] | record start|stop|save FILE|discard")
	duration := fs.Duration("duration", 0, "stop after this long (default: until Ctrl-C)")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return usageError("record needs a file name or start, stop, save or discard")
	}
	c, err := connect(g)
	if err != nil {
		return err
	}
	switch pos[0] {
	case "start":
		return spider(c, "/start-recording")
	case "stop":
		return spider(c, "/stop-recording")
	case "discard", "delete":
		return spider(c, "/delete-recording")
	case "save":
		if len(pos) != 2 {
			return usageError("record save takes a file name")
		}
		abs, err := filepath.Abs(pos[1])
		if err != nil {
			return err
		}
		return spider(c, "/save-recording", control.Str(abs))
	}
	file := pos[0]
	if filepath.Ext(file) == "" {
		file += ".wav"
	}
	if err := spider(c, "/start-recording"); err != nil {
		return err
	}
	if !g.Quiet {
		how := "Ctrl-C to stop"
		if *duration > 0 {
			how = "for " + duration.String()
		}
		fmt.Fprintf(os.Stderr, "● Recording (%s)...\n", how)
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)
	var done <-chan time.Time
	if *duration > 0 {
		done = time.After(*duration)
	}
	start := time.Now()
	select {
	case <-sig:
	case <-done:
	}
	took := time.Since(start).Round(time.Second)
	if err := saveRecording(c, file); err != nil {
		return err
	}
	if !g.Quiet {
		fmt.Fprintf(os.Stderr, "Saved %s of audio to %s\n", took, file)
	}
	return nil
}

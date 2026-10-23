package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/AlexDev404/sonic-pi-lsp/internal/control"
)

func init() {
	register(&command{
		name:    "midi",
		summary: "enable, disable or reset MIDI, and list MIDI ports",
		usage:   "sonicpi midi [ports|enable|disable|reset|channel N]",
		run:     cmdMidi,
	})
	register(&command{
		name:    "osc",
		summary: "the OSC cue port: enable, disable, public/private, send",
		usage:   "sonicpi osc [status|enable|disable|public|private|send /path ARGS...]",
		run:     cmdOSC,
	})
	register(&command{
		name:    "link",
		summary: "Ableton Link: enable, disable, set the tempo",
		usage:   "sonicpi link [status|enable|disable|bpm N|reset]",
		run:     cmdLink,
	})
	register(&command{
		name:    "timewarp",
		summary: "shift when all sound is scheduled, in seconds (the desktop's timewarp)",
		usage:   "sonicpi timewarp SECONDS",
		run:     cmdTimewarp,
	})
}

func status(c *control.Client) (control.Status, error) {
	var st control.Status
	err := c.Call("status", nil, &st)
	return st, err
}

// setLive saves a live preference and applies it to the running server.
func setLive(g *Globals, key, value string) error {
	st, err := loadPrefs()
	if err != nil {
		return err
	}
	if err := setPref(st, key, value); err != nil {
		return err
	}
	c, err := connect(g)
	if err != nil {
		return err
	}
	return c.Call("apply-prefs", nil, nil)
}

func cmdMidi(g *Globals, args []string) error {
	fs := newFlags(g, "midi", "sonicpi midi [ports|enable|disable|reset|channel N]")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	sub := "ports"
	if len(pos) > 0 {
		sub = pos[0]
	}
	switch sub {
	case "enable", "on", "start":
		return setLive(g, "midi-enable", "true")
	case "disable", "off", "stop":
		return setLive(g, "midi-enable", "false")
	case "reset":
		c, err := connect(g)
		if err != nil {
			return err
		}
		return spider(c, "/midi-reset", control.Int(0))
	case "channel":
		if len(pos) != 2 {
			st, err := loadPrefs()
			if err != nil {
				return err
			}
			fmt.Println(st.Int("midi-default-channel"))
			return nil
		}
		n, err := strconv.Atoi(pos[1])
		if err != nil || n < 0 || n > 16 {
			return usageError("the channel is 0 (all) or 1-16")
		}
		st, err := loadPrefs()
		if err != nil {
			return err
		}
		return setPref(st, "midi-default-channel", pos[1])
	case "ports", "list":
		c, err := connect(g)
		if err != nil {
			return err
		}
		st, err := status(c)
		if err != nil {
			return err
		}
		if g.JSON {
			fmt.Printf("{\"in\": %q, \"out\": %q}\n", st.MidiIn, st.MidiOut)
			return nil
		}
		show := func(label, ports string) {
			fmt.Println(label + ":")
			ports = strings.TrimSpace(ports)
			if ports == "" || ports == "[]" {
				fmt.Println("  (none)")
				return
			}
			for _, p := range strings.Split(strings.Trim(ports, "[]"), ",") {
				fmt.Println("  " + strings.Trim(strings.TrimSpace(p), `"`))
			}
		}
		show("MIDI in", st.MidiIn)
		show("MIDI out", st.MidiOut)
		return nil
	}
	return usageError("unknown midi command %q", sub)
}

func cmdOSC(g *Globals, args []string) error {
	fs := newFlags(g, "osc", "sonicpi osc [status|enable|disable|public|private|send /path ARGS...]")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	sub := "status"
	if len(pos) > 0 {
		sub = pos[0]
	}
	switch sub {
	case "status":
		st, err := loadPrefs()
		if err != nil {
			return err
		}
		port := ""
		if c, err := control.Dial(); err == nil {
			if s, err := status(c); err == nil && s.Ports.OSCCues != 0 {
				port = fmt.Sprint(s.Ports.OSCCues)
			}
		}
		fmt.Printf("enabled: %v\npublic:  %v\n", st.Bool("osc-enabled"), st.Bool("osc-public"))
		if port != "" {
			fmt.Printf("port:    %s\n", port)
		}
		return nil
	case "enable", "on", "start":
		return setLive(g, "osc-enabled", "true")
	case "disable", "off", "stop":
		return setLive(g, "osc-enabled", "false")
	case "public", "external":
		return setLive(g, "osc-public", "true")
	case "private", "internal":
		return setLive(g, "osc-public", "false")
	case "send":
		if len(pos) < 2 || !strings.HasPrefix(pos[1], "/") {
			return usageError("osc send needs a path such as /trigger/drums")
		}
		var oargs []control.Arg
		for _, a := range pos[2:] {
			v, err := control.ParseArg(a)
			if err != nil {
				return usageError("%v", err)
			}
			oargs = append(oargs, v)
		}
		c, err := connect(g)
		if err != nil {
			return err
		}
		return c.Call("send", control.SendArgs{Target: control.ToCues, Address: pos[1], Args: oargs}, nil)
	}
	return usageError("unknown osc command %q", sub)
}

func cmdLink(g *Globals, args []string) error {
	fs := newFlags(g, "link", "sonicpi link [status|enable|disable|bpm N|reset]")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	sub := "status"
	if len(pos) > 0 {
		sub = pos[0]
	}
	c, err := connect(g)
	if err != nil {
		return err
	}
	st, err := status(c)
	if err != nil {
		return err
	}
	// Sonic Pi 5 runs Link in its SuperSonic engine; 4.x in Tau.
	v5 := st.Ports.Tau == 0
	send := func(target, addr string, a ...control.Arg) error {
		return c.Call("send", control.SendArgs{Target: target, Address: addr, Args: a}, nil)
	}
	tau := func(addr string, a ...control.Arg) error { return send(control.ToTau, addr, a...) }
	switch sub {
	case "status":
		if st.LinkBPM == "" {
			fmt.Println("No Link activity seen yet. `sonicpi link enable`, or use_bpm :link in code.")
			return nil
		}
		fmt.Printf("bpm:   %s\npeers: %s\n", st.LinkBPM, st.LinkPeers)
		return nil
	case "enable", "on":
		if v5 {
			return send(control.ToEngine, "/clock/visibility", control.Int(2)) // network wide
		}
		return tau("/link-enable")
	case "disable", "off":
		if v5 {
			return send(control.ToEngine, "/clock/visibility", control.Int(0))
		}
		return tau("/link-disable")
	case "reset":
		if v5 {
			return usageError("link reset is a Sonic Pi 4 command; use link disable and link enable")
		}
		return tau("/link-reset")
	case "bpm", "tempo":
		if len(pos) != 2 {
			return usageError("link bpm takes a tempo")
		}
		bpm, err := strconv.ParseFloat(pos[1], 64)
		if err != nil || bpm < 20 || bpm > 999 {
			return usageError("the tempo must be a number from 20 to 999")
		}
		if v5 {
			return send(control.ToEngine, "/clock/tempo/set", control.Float(bpm))
		}
		return tau("/link-set-tempo", control.Float(bpm))
	}
	return usageError("unknown link command %q", sub)
}

func cmdTimewarp(g *Globals, args []string) error {
	fs := newFlags(g, "timewarp", "sonicpi timewarp SECONDS")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageError("timewarp takes a number of seconds (negative to play earlier)")
	}
	t, err := strconv.ParseFloat(pos[0], 64)
	if err != nil {
		return usageError("not a number: %q", pos[0])
	}
	c, err := connect(g)
	if err != nil {
		return err
	}
	if err := spider(c, "/set-global-timewarp", control.Float(t)); err != nil {
		return err
	}
	if !g.Quiet {
		fmt.Fprintf(os.Stderr, "Global timewarp set to %gs.\n", t)
	}
	return nil
}

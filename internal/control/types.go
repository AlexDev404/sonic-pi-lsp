package control

import (
	"fmt"
	"strconv"
	"time"
)

// Server states.
const (
	StateBooting  = "booting"
	StateRunning  = "running"
	StateFailed   = "failed"
	StateStopped  = "stopped"
	StateStopping = "stopping"
)

// Ports are the ones the boot daemon allocated.
type Ports struct {
	Daemon    int `json:"daemon"`
	GUIListen int `json:"gui_listen"`
	GUISend   int `json:"gui_send"`
	Scsynth   int `json:"scsynth"`
	OSCCues   int `json:"osc_cues"`
	Tau       int `json:"tau"`
	TauPhx    int `json:"tau_phx"`
}

// Status answers the "status" command.
type Status struct {
	State          string    `json:"state"`
	Error          string    `json:"error,omitempty"`
	PID            int       `json:"pid"`
	DaemonPID      int       `json:"daemon_pid,omitempty"`
	Mode           string    `json:"mode"`
	Version        string    `json:"version"`
	Root           string    `json:"root"`
	LogDir         string    `json:"log_dir"`
	ServerLogDir   string    `json:"server_log_dir"`
	Started        time.Time `json:"started"`
	Booted         time.Time `json:"booted,omitempty"`
	Boots          int       `json:"boots"`
	Ports          Ports     `json:"ports"`
	SonicPiVersion string    `json:"sonic_pi_version,omitempty"`
	LatestVersion  string    `json:"latest_version,omitempty"`
	Platform       string    `json:"platform,omitempty"`
	Scsynth        string    `json:"scsynth,omitempty"`
	MidiIn         string    `json:"midi_in,omitempty"`
	MidiOut        string    `json:"midi_out,omitempty"`
	LinkBPM        string    `json:"link_bpm,omitempty"`
	LinkPeers      string    `json:"link_peers,omitempty"`
	Runs           int       `json:"runs"`
}

// WaitArgs asks "status" to block until the server has booted or failed.
type WaitArgs struct {
	Wait    bool    `json:"wait"`
	Timeout float64 `json:"timeout"` // seconds
}

// RunArgs runs code. To see what it does, subscribe first.
type RunArgs struct {
	Code string `json:"code"`
}

// Arg is one typed OSC argument.
type Arg struct {
	I *int32   `json:"i,omitempty"`
	F *float32 `json:"f,omitempty"`
	D *float64 `json:"d,omitempty"`
	S *string  `json:"s,omitempty"`
}

// Int makes an int32 argument.
func Int(v int) Arg { i := int32(v); return Arg{I: &i} }

// Float makes a float32 argument.
func Float(v float64) Arg { f := float32(v); return Arg{F: &f} }

// Str makes a string argument.
func Str(v string) Arg { return Arg{S: &v} }

// Value returns the argument as an OSC value.
func (a Arg) Value() any {
	switch {
	case a.I != nil:
		return *a.I
	case a.F != nil:
		return *a.F
	case a.D != nil:
		return *a.D
	case a.S != nil:
		return *a.S
	}
	return nil
}

// ParseArg reads a command-line OSC argument: integers become int32,
// other numbers float32, and anything else a string. A "s:" prefix forces
// a string, "i:" an int and "f:" a float.
func ParseArg(s string) (Arg, error) {
	switch {
	case len(s) > 2 && s[:2] == "s:":
		return Str(s[2:]), nil
	case len(s) > 2 && s[:2] == "i:":
		n, err := strconv.ParseInt(s[2:], 10, 32)
		if err != nil {
			return Arg{}, fmt.Errorf("not an int: %q", s)
		}
		return Int(int(n)), nil
	case len(s) > 2 && s[:2] == "f:":
		n, err := strconv.ParseFloat(s[2:], 32)
		if err != nil {
			return Arg{}, fmt.Errorf("not a float: %q", s)
		}
		return Float(n), nil
	}
	if n, err := strconv.ParseInt(s, 10, 32); err == nil {
		return Int(int(n)), nil
	}
	if n, err := strconv.ParseFloat(s, 32); err == nil {
		return Float(n), nil
	}
	return Str(s), nil
}

// Targets for SendArgs.
const (
	ToSpider = "spider" // the Ruby server's API; the token is added as the first argument
	ToTau    = "tau"    // Tau's API (Link, MIDI, OSC out)
	ToDaemon = "daemon" // the boot daemon; the token is added as the first argument
	ToCues   = "cues"   // the OSC cue port, as an external device would send
	ToEngine = "engine" // Sonic Pi 5's SuperSonic engine (Link, clock), over TCP
)

// SendArgs sends one OSC message.
type SendArgs struct {
	Target  string `json:"target"`
	Address string `json:"address"`
	Args    []Arg  `json:"args,omitempty"`
	// Expect makes the call wait for the server to answer with an event at
	// this address, which is returned as the result.
	Expect  string  `json:"expect,omitempty"`
	Timeout float64 `json:"timeout,omitempty"`
}

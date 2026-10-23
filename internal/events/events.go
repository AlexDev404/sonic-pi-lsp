// Package events turns what the Sonic Pi server sends its GUI into
// structured events, and formats them the way the desktop log does.
package events

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/AlexDev404/sonic-pi-lsp/internal/osc"
)

// Kinds of event.
const (
	KindLog          = "log"           // /log/multi_message: what a run did at one moment
	KindInfo         = "info"          // /log/info: server notices
	KindCue          = "cue"           // /incoming/osc: the cue pane
	KindError        = "error"         // /error: a runtime error
	KindSyntaxError  = "syntax_error"  // /syntax_error
	KindRunsComplete = "runs_complete" // /runs/all-completed
	KindScsynth      = "scsynth"       // /scsynth/info
	KindVersion      = "version"       // /version
	KindMidiPorts    = "midi_ports"    // /midi/in-ports, /midi/out-ports
	KindLink         = "link"          // /link-bpm, /link-num-peers
	KindBuffer       = "buffer"        // /buffer/replace and friends
	KindExited       = "exited"        // /exited, /exited-with-boot-error
	KindService      = "service"       // the CLI service's own notices
)

// Line styles inside a log event, as set by the server's __delayed_* calls.
const (
	StyleSynth      = 0 // synth and sample triggers, FX, control
	StyleUser       = 1 // puts / print
	StyleWarning    = 2
	StyleSerious    = 3
	StyleCue        = 4 // cue
	StyleSynced     = 5 // synced
	StyleSyncing    = 6 // sync (waiting)
	StyleBootNotice = 1 // /log/info style 1
)

// Line is one line of a log event.
type Line struct {
	Style int    `json:"style"`
	Text  string `json:"text"`
}

// Event is one thing the server reported.
type Event struct {
	Time      time.Time `json:"t"`
	Kind      string    `json:"kind"`
	Run       int       `json:"run,omitempty"`
	Thread    string    `json:"thread,omitempty"`
	Beat      string    `json:"beat,omitempty"` // the run's local time
	Lines     []Line    `json:"lines,omitempty"`
	Style     int       `json:"style,omitempty"`
	Text      string    `json:"text,omitempty"`
	Line      int       `json:"line,omitempty"`
	Trace     string    `json:"trace,omitempty"`
	ErrorLine string    `json:"error_line,omitempty"`
	Address   string    `json:"address,omitempty"`
	Args      string    `json:"args,omitempty"`
	ID        int       `json:"id,omitempty"`
	Data      []string  `json:"data,omitempty"`
}

// FromOSC converts a message sent to the GUI port. It returns nil for
// messages that carry nothing worth logging (such as /ack).
func FromOSC(m *osc.Message, now time.Time) *Event {
	e := &Event{Time: now.UTC()}
	switch m.Address {
	case "/log/multi_message":
		e.Kind = KindLog
		e.Run, _ = m.Int(0)
		e.Thread = m.Str(1)
		if e.Thread == `""` {
			e.Thread = ""
		}
		e.Beat = m.Str(2)
		for i := 4; i+1 < len(m.Args); i += 2 {
			style, _ := m.Int(i)
			e.Lines = append(e.Lines, Line{Style: style, Text: m.Str(i + 1)})
		}
	case "/log/info":
		e.Kind = KindInfo
		e.Style, _ = m.Int(0)
		e.Text = m.Str(1)
	case "/incoming/osc":
		e.Kind = KindCue
		e.Beat = m.Str(0)
		e.ID, _ = m.Int(1)
		e.Address = m.Str(2)
		e.Args = m.Str(3)
	case "/error":
		e.Kind = KindError
		e.Run, _ = m.Int(0)
		e.Text = html.UnescapeString(m.Str(1))
		e.Trace = html.UnescapeString(m.Str(2))
		e.Line, _ = m.Int(3)
	case "/syntax_error":
		e.Kind = KindSyntaxError
		e.Run, _ = m.Int(0)
		e.Text = html.UnescapeString(m.Str(1))
		e.ErrorLine = m.Str(2)
		e.Line, _ = m.Int(3)
	case "/runs/all-completed":
		e.Kind = KindRunsComplete
	case "/scsynth/info":
		e.Kind = KindScsynth
		e.Text = m.Str(0)
	case "/version":
		e.Kind = KindVersion
		for i := range m.Args {
			e.Data = append(e.Data, m.Str(i))
		}
	case "/midi/in-ports", "/midi/out-ports", "midi/in-ports", "midi/out-ports":
		e.Kind = KindMidiPorts
		e.Address = strings.TrimPrefix(m.Address, "/")
		e.Text = m.Str(0)
	case "/link-bpm", "/link-num-peers":
		e.Kind = KindLink
		e.Address = m.Address
		e.Text = m.Str(0)
	case "/buffer/replace", "/buffer/replace-idx", "/buffer/replace-lines", "/buffer/run-idx":
		e.Kind = KindBuffer
		e.Address = m.Address
		for i := range m.Args {
			e.Data = append(e.Data, m.Str(i))
		}
	case "/exited":
		e.Kind = KindExited
		e.Text = "Sonic Pi server has exited"
	case "/exited-with-boot-error":
		e.Kind = KindExited
		e.Text = m.Str(0)
	default:
		return nil
	}
	return e
}

// Service makes one of the service's own notices.
func Service(format string, a ...any) *Event {
	return &Event{Time: time.Now().UTC(), Kind: KindService, Text: fmt.Sprintf(format, a...)}
}

// Marshal encodes an event as one JSON line.
func (e *Event) Marshal() []byte {
	b, _ := json.Marshal(e)
	return append(b, '\n')
}

// Parse decodes one JSON line.
func Parse(b []byte) (*Event, error) {
	var e Event
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

// Filter chooses which events and lines to show.
type Filter struct {
	Synths   bool // synth and sample triggers (style 0 lines)
	Cues     bool // cue lines and the cue pane (/incoming/osc)
	Puts     bool // puts / print output
	Info     bool // server notices and the service's own
	Warnings bool
	Errors   bool
	Run      int // only this run, when non-zero
	Thread   string
}

// AllFilter shows everything.
var AllFilter = Filter{Synths: true, Cues: true, Puts: true, Info: true, Warnings: true, Errors: true}

func (f Filter) line(l Line) bool {
	switch l.Style {
	case StyleSynth:
		return f.Synths
	case StyleUser:
		return f.Puts
	case StyleWarning, StyleSerious:
		return f.Warnings
	case StyleCue, StyleSynced, StyleSyncing:
		return f.Cues
	}
	return true
}

// Apply returns the event as the filter would show it (with hidden lines
// removed), or nil when nothing of it is shown.
func (f Filter) Apply(e *Event) *Event {
	if f.Run != 0 && e.Run != 0 && e.Run != f.Run {
		return nil
	}
	if f.Thread != "" && e.Kind == KindLog && strings.Trim(e.Thread, `":`) != strings.Trim(f.Thread, `":`) {
		return nil
	}
	switch e.Kind {
	case KindLog:
		var keep []Line
		for _, l := range e.Lines {
			if f.line(l) {
				keep = append(keep, l)
			}
		}
		if len(keep) == 0 {
			return nil
		}
		c := *e
		c.Lines = keep
		return &c
	case KindCue:
		if !f.Cues {
			return nil
		}
	case KindError, KindSyntaxError, KindExited:
		if !f.Errors {
			return nil
		}
	case KindInfo, KindService, KindScsynth, KindVersion, KindMidiPorts, KindLink:
		if !f.Info {
			return nil
		}
	case KindRunsComplete, KindBuffer:
		return nil
	}
	return e
}

package events

import (
	"fmt"
	"strings"
	"time"
)

// Formatter renders events as text, in the desktop log's layout.
type Formatter struct {
	Color      bool
	Timestamps bool
	// Source names the code being run, for error messages ("song.rb").
	Source string
}

const (
	reset   = "\x1b[0m"
	bold    = "\x1b[1m"
	dim     = "\x1b[2m"
	red     = "\x1b[31m"
	green   = "\x1b[32m"
	yellow  = "\x1b[33m"
	blue    = "\x1b[34m"
	magenta = "\x1b[35m"
	cyan    = "\x1b[36m"
	pink    = "\x1b[38;5;205m"
	orange  = "\x1b[38;5;208m"
)

func (f Formatter) paint(code, s string) string {
	if !f.Color || code == "" {
		return s
	}
	return code + s + reset
}

func styleColor(style int) string {
	switch style {
	case StyleUser:
		return cyan
	case StyleWarning:
		return yellow
	case StyleSerious:
		return red
	case StyleCue:
		return pink
	case StyleSynced, StyleSyncing:
		return magenta
	}
	return ""
}

func (f Formatter) stamp(e *Event) string {
	if !f.Timestamps || e.Time.IsZero() {
		return ""
	}
	return f.paint(dim, e.Time.Local().Format("15:04:05.000")+" ")
}

// Format renders one event, ending with a newline. It returns "" for
// events with no text form.
func (f Formatter) Format(e *Event) string {
	var b strings.Builder
	ts := f.stamp(e)
	switch e.Kind {
	case KindLog:
		head := fmt.Sprintf("{run: %d, time: %s", e.Run, e.Beat)
		if e.Thread != "" {
			head += ", thread: " + strings.Trim(e.Thread, `"`)
		}
		head += "}"
		b.WriteString(ts + f.paint(bold, head) + "\n")
		for i, l := range e.Lines {
			branch := " ├─ "
			if i == len(e.Lines)-1 {
				branch = " └─ "
			}
			b.WriteString(f.paint(dim, branch) + f.paint(styleColor(l.Style), l.Text) + "\n")
		}
	case KindInfo:
		c := green
		if e.Style == StyleBootNotice {
			c = blue
		}
		b.WriteString(ts + f.paint(c, "=> "+strings.TrimRight(e.Text, "\n")) + "\n")
	case KindService:
		b.WriteString(ts + f.paint(dim, "-- "+e.Text) + "\n")
	case KindCue:
		b.WriteString(ts + f.paint(pink, e.Address) + " " + e.Args + "\n")
	case KindError:
		loc := f.where(e.Line)
		b.WriteString(ts + f.paint(red+bold, fmt.Sprintf("Runtime Error: [%s] run %d", loc, e.Run)) + "\n")
		b.WriteString(f.paint(red, indent(e.Text)) + "\n")
		if strings.TrimSpace(e.Trace) != "" {
			b.WriteString(f.paint(dim, indent(e.Trace)) + "\n")
		}
	case KindSyntaxError:
		b.WriteString(ts + f.paint(orange+bold, fmt.Sprintf("Syntax Error: [%s] run %d", f.where(e.Line), e.Run)) + "\n")
		b.WriteString(f.paint(orange, indent(e.Text)) + "\n")
		if strings.TrimSpace(e.ErrorLine) != "" {
			b.WriteString(f.paint(dim, indent(e.ErrorLine)) + "\n")
		}
	case KindScsynth:
		b.WriteString(ts + f.paint(blue, indent(strings.TrimSpace(e.Text))) + "\n")
	case KindVersion:
		if len(e.Data) > 0 {
			b.WriteString(ts + f.paint(dim, "-- Sonic Pi "+e.Data[0]) + "\n")
		}
	case KindMidiPorts:
		b.WriteString(ts + f.paint(dim, "-- "+e.Address+": "+e.Text) + "\n")
	case KindLink:
		b.WriteString(ts + f.paint(dim, "-- "+strings.TrimPrefix(e.Address, "/")+": "+e.Text) + "\n")
	case KindExited:
		b.WriteString(ts + f.paint(red, "-- "+e.Text) + "\n")
	}
	return b.String()
}

func (f Formatter) where(line int) string {
	src := f.Source
	if src == "" {
		src = "code"
	}
	if line > 0 {
		return fmt.Sprintf("%s, line %d", src, line)
	}
	return src
}

func indent(s string) string {
	s = strings.TrimRight(s, "\n")
	return "   " + strings.ReplaceAll(s, "\n", "\n   ")
}

// Plain renders an event without colour for the human-readable log file.
func Plain(e *Event) string {
	out := Formatter{}.Format(e)
	if out == "" {
		return ""
	}
	ts := e.Time.Format(time.RFC3339Nano)
	var b strings.Builder
	for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		b.WriteString(ts + " [" + e.Kind + "] " + l + "\n")
	}
	return b.String()
}

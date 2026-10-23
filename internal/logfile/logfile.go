// Package logfile writes the service's session log and reads it back for
// `sonicpi logs`. The log directory holds two files, rotated together:
//
//	events.jsonl  one JSON event per line, which `sonicpi logs` filters and formats
//	sonicpi.log   the same events as plain text, for people and log tools
package logfile

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/AlexDev404/sonic-pi-lsp/internal/events"
)

const (
	EventsName = "events.jsonl"
	TextName   = "sonicpi.log"
	// MaxSize is the size at which the files are rotated.
	MaxSize = 16 << 20
	// Keep is how many rotated generations are kept.
	Keep = 3
)

// Writer appends events to the log directory.
type Writer struct {
	dir    string
	mu     sync.Mutex
	events *os.File
	text   *os.File
	size   int64
}

// Open opens (creating) the log files in dir.
func Open(dir string) (*Writer, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	w := &Writer{dir: dir}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

// Dir is the log directory.
func (w *Writer) Dir() string { return w.dir }

func (w *Writer) open() error {
	var err error
	w.events, err = os.OpenFile(filepath.Join(w.dir, EventsName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	w.text, err = os.OpenFile(filepath.Join(w.dir, TextName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		w.events.Close()
		return err
	}
	if fi, err := w.events.Stat(); err == nil {
		w.size = fi.Size()
	}
	return nil
}

func (w *Writer) rotate() error {
	w.events.Close()
	w.text.Close()
	for _, name := range []string{EventsName, TextName} {
		base := filepath.Join(w.dir, name)
		os.Remove(fmt.Sprintf("%s.%d", base, Keep))
		for i := Keep - 1; i >= 1; i-- {
			os.Rename(fmt.Sprintf("%s.%d", base, i), fmt.Sprintf("%s.%d", base, i+1))
		}
		os.Rename(base, base+".1")
	}
	return w.open()
}

// Write appends one event to both files.
func (w *Writer) Write(e *events.Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.size > MaxSize {
		if err := w.rotate(); err != nil {
			return err
		}
	}
	b := e.Marshal()
	n, err := w.events.Write(b)
	w.size += int64(n)
	if err != nil {
		return err
	}
	if t := events.Plain(e); t != "" {
		_, err = w.text.WriteString(t)
	}
	return err
}

// Close closes the files.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.events.Close()
	return w.text.Close()
}

// Tail returns the last n events in dir (all of them when n < 0), reading
// into rotated files when the current one holds fewer.
func Tail(dir string, n int) ([]*events.Event, error) {
	base := filepath.Join(dir, EventsName)
	files := []string{base}
	for i := 1; i <= Keep; i++ {
		files = append(files, fmt.Sprintf("%s.%d", base, i))
	}
	var out []*events.Event
	for _, f := range files {
		evs, err := readAll(f)
		if err != nil {
			if os.IsNotExist(err) {
				if f == base {
					return nil, err
				}
				break
			}
			return nil, err
		}
		out = append(evs, out...)
		if n >= 0 && len(out) >= n {
			break
		}
	}
	if n >= 0 && len(out) > n {
		out = out[len(out)-n:]
	}
	return out, nil
}

func readAll(path string) ([]*events.Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []*events.Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		if e, err := events.Parse(sc.Bytes()); err == nil {
			out = append(out, e)
		}
	}
	return out, sc.Err()
}

// Follow sends each event appended to dir's log from now on, until ctx ends.
// It survives rotation and the file not existing yet.
func Follow(ctx context.Context, dir string, fn func(*events.Event)) error {
	path := filepath.Join(dir, EventsName)
	var f *os.File
	var pos int64
	var pending []byte
	open := func(fromEnd bool) {
		var err error
		f, err = os.Open(path)
		if err != nil {
			f = nil
			return
		}
		pos = 0
		if fromEnd {
			pos, _ = f.Seek(0, io.SeekEnd)
		}
		pending = nil
	}
	open(true)
	defer func() {
		if f != nil {
			f.Close()
		}
	}()
	buf := make([]byte, 64<<10)
	tick := time.NewTicker(150 * time.Millisecond)
	defer tick.Stop()
	for {
		if f == nil {
			open(false)
		}
		if f != nil {
			for {
				n, err := f.Read(buf)
				if n > 0 {
					pos += int64(n)
					pending = append(pending, buf[:n]...)
					for {
						i := bytes.IndexByte(pending, '\n')
						if i < 0 {
							break
						}
						if e, err := events.Parse(pending[:i]); err == nil {
							fn(e)
						}
						pending = pending[i+1:]
					}
				}
				if err != nil || n == 0 {
					break
				}
			}
			// Rotated or truncated: start over on the new file.
			if fi, err := os.Stat(path); err != nil || fi.Size() < pos || !sameFile(f, fi) {
				f.Close()
				f = nil
				continue
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
	}
}

func sameFile(f *os.File, fi os.FileInfo) bool {
	cur, err := f.Stat()
	if err != nil {
		return false
	}
	return os.SameFile(cur, fi)
}

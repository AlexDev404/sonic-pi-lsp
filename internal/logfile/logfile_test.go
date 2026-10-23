package logfile

import (
	"context"
	"testing"
	"time"

	"github.com/AlexDev404/sonic-pi-lsp/internal/events"
)

func TestTailAndFollow(t *testing.T) {
	dir := t.TempDir()
	w, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	for i := 0; i < 5; i++ {
		w.Write(events.Service("event %d", i))
	}
	evs, err := Tail(dir, 2)
	if err != nil || len(evs) != 2 || evs[1].Text != "event 4" {
		t.Fatalf("tail: %v %v", evs, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got := make(chan string, 10)
	go Follow(ctx, dir, func(e *events.Event) { got <- e.Text })
	time.Sleep(300 * time.Millisecond)
	w.Write(events.Service("new"))
	// Rotation mid-follow: the follower moves to the new file.
	w.mu.Lock()
	w.rotate()
	w.mu.Unlock()
	w.Write(events.Service("after rotation"))
	for _, want := range []string{"new", "after rotation"} {
		select {
		case g := <-got:
			if g != want {
				t.Fatalf("got %q, want %q", g, want)
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %q", want)
		}
	}
	all, _ := Tail(dir, -1)
	if len(all) != 7 {
		t.Fatalf("tail across rotation: %d events", len(all))
	}
}

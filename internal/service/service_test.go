package service

import (
	"context"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlexDev404/sonic-pi-lsp/internal/control"
	"github.com/AlexDev404/sonic-pi-lsp/internal/events"
	"github.com/AlexDev404/sonic-pi-lsp/internal/logfile"
	"github.com/AlexDev404/sonic-pi-lsp/internal/prefs"
)

func TestParsePorts(t *testing.T) {
	if n := parsePorts("1 2 3 4 5 -77\n"); len(n) != 6 || n[5] != -77 {
		t.Fatalf("Sonic Pi 5 line: %v", n)
	}
	if n := parsePorts("1 2 3 4 5 6 7 8"); len(n) != 8 {
		t.Fatalf("Sonic Pi 4 line: %v", n)
	}
	for _, bad := range []string{"", "1 2 3", "a b c d e f", "Welcome to the daemon"} {
		if parsePorts(bad) != nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestDriveAmount(t *testing.T) {
	for pct, want := range map[int]float64{25: 0, 100: 0.5, 400: 1} {
		if got := DriveAmount(pct); math.Abs(got-want) > 1e-9 {
			t.Errorf("DriveAmount(%d) = %v, want %v", pct, got, want)
		}
	}
}

func TestDaemonArgs(t *testing.T) {
	t.Setenv("SONIC_PI_HOME", t.TempDir())
	st, err := prefs.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(daemonArgs(st, 5), " "); got != "--no-scsynth-inputs" {
		t.Fatalf("defaults: %q", got)
	}
	st.Overrides = map[string]string{
		"enable-scsynth-inputs": "true",
		"audio-output-device":   "USB Audio",
		"audio-input-device":    "__none__",
		"audio-sample-rate":     "48000",
		"audio-driver":          "JACK",
	}
	want := "--audio-output USB Audio --audio-sample-rate 48000 --audio-driver JACK"
	if got := strings.Join(daemonArgs(st, 5), " "); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := daemonArgs(st, 4); len(got) != 0 {
		t.Fatalf("Sonic Pi 4 takes no audio flags: %v", got)
	}
}

// TestService runs the whole service against a fake Sonic Pi server.
func TestService(t *testing.T) {
	ruby, err := exec.LookPath("ruby")
	if err != nil {
		t.Skip("needs ruby")
	}
	root, _ := filepath.Abs("testdata/fake")
	dir := t.TempDir()
	t.Setenv("SONICPI_ROOT", root)
	t.Setenv("SONICPI_RUBY", ruby)
	t.Setenv("SONIC_PI_HOME", filepath.Join(dir, "home"))
	t.Setenv("SONICPI_LOG_DIR", filepath.Join(dir, "log"))
	t.Setenv("SONICPI_STATE_DIR", filepath.Join(dir, "state"))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, Options{Version: "test", Mode: "user"}) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			t.Error("service did not stop")
		}
	}()

	var c *control.Client
	for i := 0; i < 100 && c == nil; i++ {
		c, _ = control.Dial()
		time.Sleep(50 * time.Millisecond)
	}
	if c == nil {
		t.Fatal("service did not publish its state")
	}
	var st control.Status
	if err := c.Call("status", control.WaitArgs{Wait: true, Timeout: 20}, &st); err != nil {
		t.Fatal(err)
	}
	if st.State != control.StateRunning {
		t.Fatalf("state %s: %s", st.State, st.Error)
	}
	if st.Ports.Tau != 0 || st.Ports.GUISend == 0 {
		t.Fatalf("ports: %+v", st.Ports)
	}

	// Subscribe, run code, and see its events arrive in order.
	got := make(chan *events.Event, 100)
	stop := make(chan struct{})
	defer close(stop)
	go c.Stream("subscribe", nil, nil, func(e *events.Event) { got <- e }, stop)
	<-got // "subscribed"
	if err := c.Call("run", control.RunArgs{Code: "use_debug false #__nosave__\nboom"}, nil); err != nil {
		t.Fatal(err)
	}
	var kinds []string
	timeout := time.After(10 * time.Second)
	for len(kinds) == 0 || kinds[len(kinds)-1] != events.KindRunsComplete {
		select {
		case e := <-got:
			if e.Kind == events.KindVersion {
				continue // the service's own /version at boot
			}
			kinds = append(kinds, e.Kind)
			if e.Kind == events.KindLog && e.Lines[0].Text != "ran: boom" {
				t.Errorf("log line %q", e.Lines[0].Text)
			}
			if e.Kind == events.KindError && e.Line != 2 {
				t.Errorf("error line %d", e.Line)
			}
		case <-timeout:
			t.Fatalf("no run completion; got %v", kinds)
		}
	}
	if strings.Join(kinds, ",") != "info,log,error,runs_complete" {
		t.Fatalf("events %v", kinds)
	}

	// The same events are in the session log.
	time.Sleep(100 * time.Millisecond)
	evs, err := logfile.Tail(filepath.Join(dir, "log"), -1)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range evs {
		if e.Kind == events.KindError && strings.Contains(e.Text, "boom") {
			found = true
		}
	}
	if !found {
		t.Fatal("error not in the session log")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "log", logfile.TextName)); !strings.Contains(string(b), "[log]") {
		t.Fatalf("text log:\n%s", b)
	}

	// Asking for an answer: /version comes back as an event.
	var v events.Event
	if err := c.Call("send", control.SendArgs{Target: control.ToSpider, Address: "/version", Expect: events.KindVersion, Timeout: 5}, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Data) == 0 || v.Data[0] != "v5.0" {
		t.Fatalf("version %+v", v)
	}
}

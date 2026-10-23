package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/AlexDev404/sonic-pi-lsp/internal/control"
	"github.com/AlexDev404/sonic-pi-lsp/internal/events"
	"github.com/AlexDev404/sonic-pi-lsp/internal/logfile"
	"github.com/AlexDev404/sonic-pi-lsp/internal/paths"
)

// Options configure one run of the service.
type Options struct {
	Version string
	Mode    string // "service" or "user"
	// Echo prints events to stdout as well (foreground runs).
	Echo bool
}

// ErrAlreadyRunning is returned when another service answers.
var ErrAlreadyRunning = errors.New("the Sonic Pi service is already running")

// Run runs the service until ctx ends or a client asks it to shut down.
func Run(ctx context.Context, o Options) error {
	if s, err := control.Find(); err == nil {
		return fmt.Errorf("%w (pid %d, state file %s)", ErrAlreadyRunning, s.PID, s.Path())
	}
	root, err := paths.Root()
	if err != nil {
		return err
	}
	logDir := paths.LogDir()
	lw, err := logfile.Open(logDir)
	if err != nil {
		return fmt.Errorf("opening the log in %s: %w", logDir, err)
	}
	defer lw.Close()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sup := New(root, lw, o.Version, o.Mode)
	srv := &Server{Sup: sup, Token: control.NewToken(), Shutdown: cancel}
	port, err := srv.Listen()
	if err != nil {
		return err
	}
	st := &control.State{
		PID:         os.Getpid(),
		ControlPort: port,
		Token:       srv.Token,
		LogDir:      logDir,
		Root:        root,
		Mode:        o.Mode,
		Version:     o.Version,
		Started:     time.Now(),
	}
	stateDir := paths.StateDir()
	if err := control.WriteState(stateDir, st); err != nil {
		return fmt.Errorf("writing the state file in %s: %w", stateDir, err)
	}
	defer control.RemoveState(stateDir, st.PID)

	if o.Echo {
		ch, unsub := sup.Subscribe()
		defer unsub()
		f := events.Formatter{Color: false, Timestamps: true}
		go func() {
			for e := range ch {
				fmt.Print(f.Format(e))
			}
		}()
	}

	sup.notice("sonicpid %s started (pid %d, mode %s, logs in %s)", o.Version, st.PID, o.Mode, logDir)
	// The control socket stays open until the server has shut down, so
	// `sonicpi service stop` can wait for the whole shutdown.
	serveCtx, stopServing := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		srv.Serve(serveCtx)
	}()
	sup.Run(ctx)
	stopServing()
	wg.Wait()
	sup.notice("sonicpid stopped")
	return nil
}

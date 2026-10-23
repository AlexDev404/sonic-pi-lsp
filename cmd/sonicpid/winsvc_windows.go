//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"

	"github.com/AlexDev404/sonic-pi-lsp/internal/service"
	"github.com/AlexDev404/sonic-pi-lsp/internal/svcmgr"
	"github.com/AlexDev404/sonic-pi-lsp/internal/version"
)

func runningAsWindowsService() bool {
	ok, err := svc.IsWindowsService()
	return err == nil && ok
}

type handler struct{}

func (handler) Execute(args []string, r <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- service.Run(ctx, service.Options{Version: version.Version, Mode: "service"})
	}()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case err := <-done:
			if err != nil {
				logEvent(err)
				return true, 1
			}
			return false, 0
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				status <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending, WaitHint: 30000}
				cancel()
				select {
				case <-done:
				case <-time.After(30 * time.Second):
				}
				return false, 0
			}
		}
	}
}

func logEvent(err error) {
	if l, e := eventlog.Open(svcmgr.Name); e == nil {
		l.Error(1, err.Error())
		l.Close()
	}
	fmt.Fprintln(os.Stderr, "sonicpid:", err)
}

func runWindowsService() {
	if err := svc.Run(svcmgr.Name, handler{}); err != nil {
		logEvent(err)
		os.Exit(1)
	}
}

// Command sonicpid is the background service behind the sonicpi CLI. It
// boots and supervises the Sonic Pi server and answers the CLI.
//
//	sonicpid run [--mode user|service] [--echo]   run in the foreground
//	sonicpid version
//
// Under the Windows service manager it runs as the "sonicpid" service; on
// Linux the sonicpid.service systemd user unit runs `sonicpid run`.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/AlexDev404/sonic-pi-lsp/internal/service"
	"github.com/AlexDev404/sonic-pi-lsp/internal/version"
)

func usage() {
	fmt.Fprintf(os.Stderr, `sonicpid %s - the Sonic Pi service behind the sonicpi CLI

Usage:
  sonicpid run [--mode user|service] [--echo]
  sonicpid version

Most people never run this directly: "sonicpi" starts it when needed, and
"sonicpi service install" registers it with the system's service manager.
`, version.Version)
}

func main() {
	if runningAsWindowsService() {
		runWindowsService()
		return
	}
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "run":
		fs := flag.NewFlagSet("run", flag.ExitOnError)
		mode := fs.String("mode", "user", "how this service was started: user (on demand) or service (by the service manager)")
		echo := fs.Bool("echo", false, "print events to stdout too")
		fs.Parse(os.Args[2:])
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := service.Run(ctx, service.Options{Version: version.Version, Mode: *mode, Echo: *echo}); err != nil {
			fmt.Fprintln(os.Stderr, "sonicpid:", err)
			os.Exit(1)
		}
	case "version", "--version", "-v":
		fmt.Println(version.Version)
	case "help", "-h", "--help":
		usage()
	default:
		usage()
		os.Exit(2)
	}
}

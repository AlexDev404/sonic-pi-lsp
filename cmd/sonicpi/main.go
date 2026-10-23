// Command sonicpi is a command-line client for Sonic Pi.
//
//	sonicpi song.rb            play a file and show its log
//	sonicpi lint song.rb       check it for errors
//	sonicpi --watch logs       follow the log
//
// Run "sonicpi" with no arguments for the full list of commands.
package main

import (
	"os"

	"github.com/AlexDev404/sonic-pi-lsp/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}

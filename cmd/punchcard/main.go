// Command punchcard times work against Jira tickets: one binary, a CLI
// and (later) a TUI over one core.
package main

import (
	"os"
	"time"

	"github.com/j-dsmith/punchcard/internal/cli"
	"golang.org/x/term"
)

// run is the whole program; main_test.go re-runs it as `punchcard` in
// testscript scripts.
func run() int {
	return cli.Run(os.Args[1:], cli.Env{
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Getenv:  os.Getenv,
		Now:     time.Now,
		Version: cli.BuildVersion(),
		TTY:     term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())),
	})
}

func main() { os.Exit(run()) }

// Command punchcard times work against Jira tickets: one binary, a CLI
// and a TUI over one core.
package main

import (
	"os"
	"os/exec"
	"time"

	"github.com/j-dsmith/punchcard/internal/cli"
	"github.com/j-dsmith/punchcard/internal/tui"
	"github.com/j-dsmith/punchcard/internal/worklog"
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
		TUI:     openTUI,
	})
}

// openTUI is bare `punchcard` on a terminal. The CLI hands over its
// environment (store, clock, ids already defaulted); the TUI package
// never imports the CLI.
func openTUI(e cli.Env) int {
	return tui.Run(tui.Options{
		Store: e.Store, Now: e.Now, NewID: e.NewID,
		Sender: worklog.Sender{Store: e.Store, Jira: e.Jira}, Jira: e.Jira,
		Setup: setupCommand,
		Stdin: e.Stdin, Stdout: e.Stdout, Stderr: e.Stderr,
	})
}

// setupCommand is `punchcard setup` as a child process, for S in the
// TUI: tea.ExecProcess gives it the terminal and redraws after.
func setupCommand() *exec.Cmd {
	exe, err := os.Executable()
	if err != nil {
		exe = os.Args[0]
	}
	return exec.Command(exe, "setup")
}

func main() { os.Exit(run()) }

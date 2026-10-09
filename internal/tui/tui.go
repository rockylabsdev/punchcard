// Package tui is Punchcard's full-screen face: a Bubble Tea v2 program
// over the same store the CLI uses. The model never writes: every key
// asks the store for a transition and shows the World it hands back.
package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/tracker"
	"github.com/j-dsmith/punchcard/internal/worklog"
)

// Options is what Run needs from the binary.
type Options struct {
	Store Store
	Now   func() time.Time // read once, to seed the clock before the first tick
	NewID func() string    // entry ids for starts
	// Tracking is the tracker in effect when the TUI opens.
	Tracking
	// Retrack reads the tracker in effect again, after S: setup may
	// have chosen another. Nil: it never changes while the TUI is open.
	Retrack func() Tracking
	// Setup builds the `punchcard setup` command that S suspends into
	// (tea.ExecProcess hands it the terminal). Nil: S says to run it
	// from the shell.
	Setup  func() *exec.Cmd
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Tracking is the tracker in effect and everything that follows from
// it. With none, a stop is local, nothing is looked up and the Jira
// conditions never show.
type Tracking struct {
	// Sender stops an entry and sends its worklog (worklog.Sender in the
	// binary). Nil: a stop is recorded and its worklog left pending.
	Sender Sender
	// Tracker opens the client for summary lookups (add, and confirming
	// an unverified ticket on start). Nil: the tracker is not configured.
	Tracker func() (tracker.Client, error)
	// NoTracker: none is configured. Nothing is looked up, a ticket
	// added is never unverified, and nothing is said about Jira.
	NoTracker bool
	// Grammar is the key grammar in effect (the zero value: the slug).
	Grammar tracker.Grammar
}

// Sender is the worklog seam, the only code that POSTs. Stop applies the
// stop, then sends that entry's worklog once; Sync sends everything owed
// (key "" is all). Both can block for worklog.PostBudget per POST, so
// the model only ever calls them inside a tea.Cmd.
type Sender interface {
	Stop(ctx context.Context, c core.Stop) (worklog.StopResult, error)
	Sync(ctx context.Context, key string) (worklog.Report, error)
}

// Run opens the TUI and blocks until the user quits. Timers keep running
// after it returns: they are on disk. It returns the exit code.
func Run(o Options) int {
	var opts []tea.ProgramOption
	if o.Stdin != nil {
		opts = append(opts, tea.WithInput(o.Stdin))
	}
	if o.Stdout != nil {
		opts = append(opts, tea.WithOutput(o.Stdout))
	}
	p := tea.NewProgram(New(o), opts...)
	if _, err := p.Run(); err != nil {
		if errors.Is(err, tea.ErrInterrupted) {
			return 130 // SIGINT from outside; timers are untouched
		}
		if o.Stderr != nil {
			fmt.Fprintf(o.Stderr, "punchcard: %v\n", err)
		}
		return 1
	}
	return 0
}

// Package tui is Punchcard's full-screen face: a Bubble Tea v2 program
// over the same store the CLI uses. The model never writes: every key
// asks the store for a transition and shows the World it hands back.
package tui

import (
	"errors"
	"fmt"
	"io"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Options is what Run needs from the binary.
type Options struct {
	Store  Store
	Now    func() time.Time // read once, to seed the clock before the first tick
	NewID  func() string    // entry ids for starts
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
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
	p := tea.NewProgram(New(o.Store, o.Now(), o.NewID), opts...)
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

package tui

import (
	"errors"
	"os/exec"
	"strconv"

	tea "charm.land/bubbletea/v2"
)

// setupDoneMsg: the setup process S suspended into has exited.
type setupDoneMsg struct{ err error }

// runSetup is S: suspend the TUI, run `punchcard setup` on the same
// terminal, then redraw. Setup is the one place credentials change.
func (m Model) runSetup() (Model, tea.Cmd) {
	if m.setup == nil {
		return m.notify("setup is not available here; run punchcard setup in a shell"), nil
	}
	return m, m.execProcess(m.setup(), func(err error) tea.Msg { return setupDoneMsg{err: err} })
}

// trackingMsg is the tracker in effect, read again after setup.
type trackingMsg Tracking

// setupDone re-reads what setup may have changed: the store (a fresh
// setup clears authFailedAt), the tracker in effect (Retrack) and
// whether Jira is configured (the probe, which follows the tracker).
func (m Model) setupDone(msg setupDoneMsg) (Model, tea.Cmd) {
	var exit *exec.ExitError
	switch {
	case errors.As(msg.err, &exit):
		m = m.notify("setup exited with status " + strconv.Itoa(exit.ExitCode()))
	case msg.err != nil:
		m = m.notify("setup: " + msg.err.Error())
	}
	m, reload := m.enqueue(intent{kind: intentReload})
	if m.retrack == nil {
		return m, tea.Batch(reload, m.probe())
	}
	retrack := m.retrack
	return m, tea.Batch(reload, func() tea.Msg { return trackingMsg(retrack()) })
}

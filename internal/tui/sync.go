package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/worklog"
)

// syncMsg is r's sync coming back.
type syncMsg struct {
	rep worklog.Report
	err error
}

// sync is r: send every pending and failed worklog. It runs outside the
// store queue (a POST can take worklog.PostBudget and must never hold up
// a start or a pause); the Sender takes sync.lock and records each
// outcome through the store itself. One at a time: r while a sync is
// out does nothing.
func (m Model) sync() (Model, tea.Cmd) {
	if m.syncing {
		return m, nil
	}
	owed, _ := core.Owed(m.shown().State)
	switch {
	case m.noTracker:
		return m.notify("no tracker · nothing is sent · S to set one up"), nil
	case owed == 0:
		return m.notify("nothing to send: Jira has everything"), nil
	case m.sender == nil:
		return m.notify("Jira is not set up · " + worklogs(owed) + " wait here"), nil
	}
	m.syncing = true
	s := m.sender
	return m, func() tea.Msg {
		rep, err := s.Sync(context.Background(), "")
		return syncMsg{rep: rep, err: err}
	}
}

// synced reports the sync, then re-reads the store through the queue
// (the newest World, never the sync's own, which a start may have
// overtaken) and asks again whether Jira is set up.
func (m Model) synced(s syncMsg) (Model, tea.Cmd) {
	m.syncing = false
	sent := len(s.rep.Sent)
	switch {
	case s.err == nil:
		m = m.happened(m.now, core.OriginHand, "sent "+worklogs(sent)+" to Jira")
	case worklog.Why(s.err) == worklog.ProblemNotConfigured:
		m = m.notify("Jira is not set up · " + worklogs(len(s.rep.Remaining)) + " wait here")
	default:
		text := sendProblem(s.err) + " · " + worklogs(len(s.rep.Remaining)) + " still owed · nothing is lost"
		if sent > 0 {
			text = "sent " + worklogs(sent) + " · " + text
		}
		m = m.notify(text)
	}
	m, cmd := m.enqueue(intent{kind: intentReload})
	return m, tea.Batch(cmd, m.probe())
}

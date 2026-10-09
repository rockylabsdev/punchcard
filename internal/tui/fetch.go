package tui

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"
	"github.com/j-dsmith/punchcard/internal/config"
	"github.com/j-dsmith/punchcard/internal/jira"
)

// fetchMsg is what Jira said of a key, the way `start` asks (slice 04's
// seam: Options.Jira, then Summary within jira.SummaryBudget).
type fetchMsg struct {
	key     string
	forAdd  bool // the add dialog asked; else confirm-on-contact
	tracked bool // the key was on the list when asked
	summary string
	found   bool
	// notFound: a 404. On a key new to the list nothing changes; on a
	// tracked one it simply stays unverified.
	notFound   bool
	authFailed bool   // a 401: recorded, and the change goes ahead unverified
	notice     string // a broken config, treated as not configured
}

// fetch asks Jira for key's summary in a tea.Cmd; it never blocks Update.
// Unconfigured, unreachable or any other answer leaves the key
// unverified.
func (m Model) fetch(key string, tracked, forAdd bool) tea.Cmd {
	open := m.jira
	return func() tea.Msg {
		msg := fetchMsg{key: key, forAdd: forAdd, tracked: tracked}
		if open == nil {
			return msg
		}
		c, err := open()
		if err != nil {
			if !errors.Is(err, config.ErrNotConfigured) {
				msg.notice = err.Error() + "; treating Jira as not configured"
			}
			return msg
		}
		ctx, cancel := context.WithTimeout(context.Background(), jira.SummaryBudget)
		defer cancel()
		s, err := c.Summary(ctx, key)
		switch {
		case err == nil:
			msg.summary, msg.found = s, true
		case errors.Is(err, jira.ErrNotFound):
			msg.notFound = true
		case errors.Is(err, jira.ErrUnauthorized):
			msg.authFailed = true
		}
		return msg
	}
}

// fetched acts on Jira's answer: a 401 is recorded whatever asked; a
// confirm-on-contact lands as core.Verify; an add's answer goes to the
// add dialog, if it is still waiting on that key.
func (m Model) fetched(f fetchMsg) (Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd
	if f.notice != "" {
		m = m.notify(f.notice)
	}
	if f.authFailed {
		m, cmd = m.enqueue(intent{kind: intentAuthFailure})
		cmds = append(cmds, cmd)
	}
	switch {
	case !f.forAdd:
		if f.found {
			m, cmd = m.enqueue(intent{kind: intentVerify, key: f.key, summary: f.summary})
			cmds = append(cmds, cmd)
		}
	default:
		if (f.found || f.notFound) && !m.world.State.AuthFailedAt.IsZero() {
			// Jira answered with these credentials: a recorded 401 no
			// longer holds (a confirm-on-contact clears it via Verify).
			m, cmd = m.enqueue(intent{kind: intentClearAuth})
			cmds = append(cmds, cmd)
		}
		if d, ok := m.dlg.(addDialog); ok && d.asking == f.key {
			m, cmd = d.answered(m, f)
			cmds = append(cmds, cmd)
		}
	}
	return m, tea.Batch(cmds...)
}

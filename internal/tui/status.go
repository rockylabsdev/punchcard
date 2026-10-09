package tui

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/j-dsmith/punchcard/internal/config"
	"github.com/j-dsmith/punchcard/internal/core"
)

// The status line is one slot, filled in this order:
//
//  1. a toast (a refusal, a notice, an outcome) for toastFor, or
//     "syncing…" while r's sync is out: the only things that ever cover
//     the banner, and they go by themselves;
//  2. the banner: the worst condition that holds now (see banner). It
//     cannot be dismissed; it clears only when the fact clears;
//  3. the last event, with its time and who caused it.

// toastFor is how long a toast covers the status line.
const toastFor = 5 * time.Second

// toast is a transient notice, timed by the model's clock (ticks).
type toast struct {
	text string
	at   time.Time
}

// event is the last thing that happened, for the status line.
type event struct {
	at     time.Time
	origin core.Origin
	text   string
}

// notify shows text as a toast from now.
func (m Model) notify(text string) Model {
	m.toast = toast{text: text, at: m.now}
	return m
}

// happened records the last event (and toasts it, so it shows even
// under a banner).
func (m Model) happened(at time.Time, origin core.Origin, text string) Model {
	m.last = event{at: at, origin: origin, text: text}
	return m.notify(text)
}

func (m Model) toasting() bool {
	return m.toast.text != "" && m.now.Sub(m.toast.at) < toastFor
}

// banner is the worst condition that holds now; ok is false when
// nothing is wrong. The switch order is the priority, worst first:
//
//	token rejected (authFailedAt) > Keychain error > not connected > failed worklogs
//
// A Keychain error is a kind of not connected (the probe wraps it as
// ErrNotConfigured), so it sits just above that case to name the cause.
//
// A new condition is one case at its rank. Facts about Jira's
// configuration arrive through the probe as m.link.err; "not connected"
// matches config.ErrNotConfigured, so a condition whose error wraps it
// must sit above that case to be seen.
//
// Every condition is about reaching Jira, so with no tracker there is
// no banner: the gate is the tracker, never the state file's fields.
func (m Model) banner() (text string, st lipgloss.Style, ok bool) {
	if m.noTracker {
		return "", lipgloss.Style{}, false
	}
	s := m.shown().State
	owed, _ := core.Owed(s)
	failed := 0
	for _, u := range core.Unsent(s) {
		if u.Worklog.State == core.WorklogFailed {
			failed++
		}
	}
	switch kc := m.keychainError(); {
	case !s.AuthFailedAt.IsZero():
		return gFailed + " Jira rejected the token at " + m.when(s.AuthFailedAt.Time) +
			" · " + worklogs(owed) + " waiting · S to fix", sFail, true
	case kc != nil:
		return "Keychain error: " + kc.Err.Error() + " · worklogs wait here, nothing is lost", sWarn, true
	case m.notConnected():
		return "Not connected to Jira · S to set up · worklogs wait here, nothing is lost", sWarn, true
	case failed > 0:
		return gFailed + " " + worklogs(failed) + " failed to reach Jira · r to retry · nothing is lost", sFail, true
	}
	return "", lipgloss.Style{}, false
}

// keychainError is the probe's Keychain failure, if that is why Jira
// is not connected.
func (m Model) keychainError() *config.KeychainError {
	var kc *config.KeychainError
	if m.link.known && errors.As(m.link.err, &kc) {
		return kc
	}
	return nil
}

// notConnected: Jira is not set up (or its config cannot be read). It
// is known only once the probe answered, so a configured Jira never
// flashes the banner at startup.
func (m Model) notConnected() bool {
	return m.link.known && errors.Is(m.link.err, config.ErrNotConfigured)
}

// worklogs is "1 worklog", "2 worklogs".
func worklogs(n int) string { return strconv.Itoa(n) + plural(n, " worklog", " worklogs") }

// statusLine is the one slot: toast or syncing, else the banner, else
// the last event.
func (m Model) statusLine(w int) string {
	switch {
	case m.syncing:
		return fit(" "+sSync.Render(gPending+" syncing…"), w)
	case m.toasting():
		return fit(" "+sBold.Render(m.toast.text), w)
	}
	if text, st, ok := m.banner(); ok {
		return fit(" "+st.Render(text), w)
	}
	if m.last.text == "" {
		return ""
	}
	return fit(" "+sDim.Render(m.last.at.In(m.now.Location()).Format("15:04"))+" "+
		originTag(m.last.origin)+" "+m.last.text, w)
}

// originTag names who caused an event: you, prompt or agent.
func originTag(o core.Origin) string {
	switch o {
	case core.OriginHand:
		return sDim.Render("you")
	}
	return sBold.Render(string(o))
}

// link is what the TUI knows about reaching Jira: whether Options.Tracker
// opens a client. It never calls Jira to find out.
type link struct {
	known bool
	err   error // nil: configured
}

// linkMsg is the probe's answer.
type linkMsg struct{ err error }

// probe opens the Jira client (config and token, no network) in a
// tea.Cmd, since reading the token may wait on the Keychain. Any error
// counts as not configured, as it does for worklog.Sender; the cause
// stays inside it for errors.As. With no tracker there is nothing to
// connect to, so nothing is opened and nothing is wrong.
func (m Model) probe() tea.Cmd {
	open := m.jira
	if m.noTracker {
		return func() tea.Msg { return linkMsg{} }
	}
	return func() tea.Msg {
		if open == nil {
			return linkMsg{err: config.ErrNotConfigured}
		}
		_, err := open()
		if err != nil && !errors.Is(err, config.ErrNotConfigured) {
			err = fmt.Errorf("%w: %w", config.ErrNotConfigured, err)
		}
		return linkMsg{err: err}
	}
}

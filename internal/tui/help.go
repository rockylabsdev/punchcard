package tui

import tea "charm.land/bubbletea/v2"

// helpDialog is ?: every key and every glyph, laid over the whole body
// (it is the one dialog wider than [2]). Any key closes it and does
// nothing else.
type helpDialog struct{}

func (helpDialog) title() string       { return "Help" }
func (helpDialog) bindings() []binding { return []binding{{"any key", "close"}} }
func (d helpDialog) tick(Model) dialog { return d }
func (helpDialog) key(m Model, _ tea.KeyPressMsg) (Model, tea.Cmd) {
	m.dlg = nil
	return m, nil
}

func (helpDialog) body(m Model, w int) []string {
	key := func(k, what string) string { return "  " + fit(sBold.Render(k), 16) + what }
	lines := []string{
		sBold.Render(" Keys"),
		key("space / enter", "start · pause · resume the selected ticket"),
		key("p", "pause whatever is running, from any row"),
		key("x", "stop the selected entry: review, then send to Jira"),
		key("a / d", "add a ticket / remove the selected one"),
		"  " + fit(sSyncKey.Render("r"), 16) + "sync: send pending and failed worklogs",
		key("S", "setup: Jira credentials, the Claude Code plugin"),
		key("j k ↑ ↓ g G", "move"),
		key("q", "quit; timers keep running without the TUI"),
		"",
		sBold.Render(" Glyphs"),
		"  " + m.pal.running.Render(gRunning) + " running   " + sPaused.Render(gPaused) + " paused   " +
			sRecov.Render(gRecovered) + " recovered: check the duration at stop",
		"  " + sDim.Render(gIdle) + " idle   " + sDim.Render(gUnverified) + " not confirmed by Jira yet",
		"  " + sSync.Render(gPending) + " waiting to be sent   " + sFail.Render(gFailed) + " failed   " +
			sDim.Render(gSynced) + " in Jira",
	}
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, wrap(l, w)...)
	}
	return out
}

// openHelp is ?.
func (m Model) openHelp() Model {
	m.dlg = helpDialog{}
	return m
}

// PROTOTYPE — throwaway. Variant A2 "Panes, refined": A after the first round
// of feedback (A won for having no layout shift; B's simplicity liked; C's big
// clock too loud).
//   - header is B's full-width reverse strip, not A's quiet title line
//   - ticket list widened to ~62%; per-row sync counts collapse to one marker
//   - worklog pane sized to its content; detail pane gets the rest, summary wrapped
//   - nothing changes size when the selection moves
package main

import (
	"strings"

	"charm.land/lipgloss/v2"
)

func (m model) stripHeader(w int) string {
	g, label, t, st := m.clockState()
	today := "today " + hm(m.st.today(m.now)) + " "
	if t == nil {
		return sDim.Reverse(true).Render(fit(" "+g+" "+label, w-len(today)) + today)
	}
	l := " " + g + " " + t.key + "  "
	r := "  " + clock(t.current().elapsed(m.now)) + "  " + sDim.Render("") + today
	return st.Reverse(true).Render(fit(l+t.summary, w-lipgloss.Width(r)) + r)
}

// one marker for a row's debt to Jira: failed beats pending; synced is quiet
func syncMarker(t *ticket) string {
	p, _, f := t.counts()
	switch {
	case f > 0:
		return sFail.Render(gFailed)
	case p > 0:
		return sSync.Render(gPending)
	}
	return " "
}

func (m model) rowA2(t *ticket, w int, selected bool) string {
	cur := " "
	if selected {
		cur = gCursor
	}
	total := t.total(m.now)
	tot := sDim.Render(hm(total))
	if total == 0 {
		tot = sFaint.Render("—")
	}
	key, sum := t.key, summaryText(t)
	switch t.state() {
	case stRunning:
		key, sum, tot = sRunning.Render(t.key), sRunning.Render(t.summary), sRunning.Render(clock(total))
	case stPaused:
		key = sPaused.Render(t.key)
		if t.recovered() {
			key, tot = sRecov.Render(t.key), sRecov.Render(hm(total)+"?")
		}
	}
	if c := m.changed(t); c != "" {
		sum = c + sDim.Render(" · ") + sum
	}
	sumW := w - 2 - 1 - 9 - 8 - 2
	row := cur + stateGlyph(t) + " " + fit(key, 9) + fit(sum, sumW) + rfit(tot, 8) + " " + syncMarker(t)
	if selected {
		return sSel.Render(fit(row, w))
	}
	return fit(row, w)
}

func (m model) viewA2(w, h int) string {
	top := []string{m.stripHeader(w)}
	if b := m.banner(w); b != "" {
		top = append(top, b)
	}
	bodyH := h - len(top) - 2
	lw := max(44, w*62/100)
	rw := w - lw

	ts := m.st.sorted()
	var rows []string
	for i, x := range ts {
		rows = append(rows, m.rowA2(x, lw-2, i == m.sel))
	}
	if len(ts) == 0 {
		rows = append(rows, "", sDim.Render("  No tickets yet."), sDim.Render("  Press ")+sKey.Render("a")+sDim.Render(" to add one by key."))
	}
	if len(rows) > bodyH-2 {
		start := max(0, min(m.sel-(bodyH-2)/2, len(rows)-(bodyH-2)))
		rows = rows[start:]
	}
	leftPane := pane("[1] Tickets", strings.Join(rows, "\n"), lw, bodyH, m.mode == modeList)

	var right string
	switch m.mode {
	case modeStop:
		right = pane("Stop "+m.stopT.key, lipgloss.NewStyle().Width(rw-2).Render(m.stopForm(rw-2)), rw, bodyH, true)
	case modeAdd:
		right = pane("Add ticket", m.addForm(rw-3), rw, bodyH, true)
	default:
		var q []string
		synced := 0
		for _, wl := range m.st.worklogs() {
			if wl.e.sync == syncSynced {
				synced++
				continue
			}
			q = append(q, syncGlyphShort(wl.e)+" "+fit(wl.t.key, 9)+fit(hm(wl.e.sendDur), 8)+sDim.Render(wl.e.stoppedAt.Format("15:04")))
		}
		if len(q) == 0 {
			q = append(q, sDim.Render(gSynced+" nothing owed"))
		}
		if synced > 0 {
			q = append(q, sDim.Render(gSynced+" "+itoa(synced)+" synced today"))
		}
		qH := min(len(q)+2, bodyH/2)
		detailH := bodyH - qH
		title, detail := "[2] Entries", ""
		if sel := m.selected(); sel != nil {
			title = "[2] " + sel.key
			wrapped := lipgloss.NewStyle().Width(rw - 2).Render(summaryText(sel))
			lines := []string{sDim.Render(wrapped), ""}
			lines = append(lines, m.entryLines(sel, rw-2)...)
			detail = strings.Join(lines, "\n")
		}
		right = lipgloss.JoinVertical(lipgloss.Left,
			pane(title, detail, rw, detailH, false),
			pane("[3] Owed to Jira", strings.Join(q, "\n"), rw, qH, false))
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, right)
	return strings.Join(append(top, body, m.lastEvent(w), m.keybar(w)), "\n")
}

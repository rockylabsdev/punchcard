// PROTOTYPE — throwaway. Variant A "Panes": lazygit-style.
// Left: the ticket list. Right: the selected ticket's entries over the worklog
// queue. Stop and add take over the right column as a pane. Top line is the
// one-glance clock; banner directly beneath it.
package main

import (
	"strings"

	"charm.land/lipgloss/v2"
)

func (m model) viewA(w, h int) string {
	var top []string
	// title line
	g, label, t, st := m.clockState()
	left := sBold.Render(" punchcard ")
	right := sDim.Render("today " + hm(m.st.today(m.now)) + " ")
	mid := st.Render(g + " " + label)
	if t != nil {
		el := t.current().elapsed(m.now)
		mid = st.Render(g+" "+t.key+" "+clock(el)) + sDim.Render("  "+label)
	}
	top = append(top, fit(left+mid, w-lipgloss.Width(right))+right)
	if b := m.banner(w); b != "" {
		top = append(top, b)
	}
	bodyH := h - len(top) - 2
	lw := max(40, w*56/100)
	rw := w - lw

	// left: tickets
	ts := m.st.sorted()
	var rows []string
	for i, x := range ts {
		rows = append(rows, m.ticketRow(x, lw-2, i == m.sel, 8))
	}
	if len(ts) == 0 {
		rows = append(rows, "", sDim.Render("  No tickets yet."), sDim.Render("  Press ")+sKey.Render("a")+sDim.Render(" to add one by key."))
	}
	leftPane := pane("[1] Tickets", strings.Join(rows, "\n"), lw, bodyH, m.mode == modeList)

	// right column
	var right2 string
	switch m.mode {
	case modeStop:
		right2 = pane("Stop "+m.stopT.key, m.stopForm(rw-4), rw, bodyH, true)
	case modeAdd:
		right2 = pane("Add ticket", m.addForm(rw-4), rw, bodyH, true)
	default:
		detailH := bodyH * 6 / 10
		qH := bodyH - detailH
		title := "[2] Entries"
		detail := ""
		if sel := m.selected(); sel != nil {
			title = "[2] " + sel.key
			lines := []string{sDim.Render(summaryText(sel)), ""}
			lines = append(lines, m.entryLines(sel, rw-2)...)
			detail = strings.Join(lines, "\n")
		}
		var q []string
		synced := 0
		for _, wl := range m.st.worklogs() {
			switch wl.e.sync {
			case syncSynced:
				synced++
			default:
				q = append(q, syncGlyphShort(wl.e)+" "+fit(wl.t.key, 9)+fit(hm(wl.e.sendDur), 8)+sDim.Render(wl.e.stoppedAt.Format("15:04")))
			}
		}
		if len(q) == 0 {
			q = append(q, sDim.Render(gSynced+" nothing owed to Jira"))
		}
		if synced > 0 {
			q = append(q, sDim.Render(gSynced+" "+itoa(synced)+" synced today"))
		}
		right2 = lipgloss.JoinVertical(lipgloss.Left,
			pane(title, detail, rw, detailH, false),
			pane("[3] Worklogs", strings.Join(q, "\n"), rw, qH, false))
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, right2)
	return strings.Join(append(top, body, m.lastEvent(w), m.keybar(w)), "\n")
}

func syncGlyphShort(e *entry) string {
	switch e.sync {
	case syncPending:
		return sSync.Render(gPending)
	case syncFailed:
		return sFail.Render(gFailed)
	}
	return sDim.Render(gSynced)
}

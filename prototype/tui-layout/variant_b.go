// PROTOTYPE — throwaway. Variant B "Ledger": one dense full-width table.
// A reverse-video strip at the top says whether you're on the clock. The
// selected row expands inline to show its entries. Stop and add are centred
// modals over the table. The banner sits at the bottom, above the keys.
package main

import (
	"strings"

	"charm.land/lipgloss/v2"
)

func (m model) viewB(w, h int) string {
	g, label, t, st := m.clockState()
	var strip string
	if t != nil {
		el := clock(t.current().elapsed(m.now))
		l := " " + g + " " + label + "  " + t.key + "  "
		r := "  " + el + " "
		strip = st.Reverse(true).Render(fit(l+t.summary, w-len(r)) + r)
	} else {
		strip = sDim.Reverse(true).Render(fit(" "+g+" "+label, w))
	}
	lines := []string{strip}
	keyW := 9
	hdr := "  " + " " + fit("KEY", keyW) + " " + fit("SUMMARY", w-2-1-keyW-1-9-7) + rfit("TODAY", 8) + "  JIRA"
	lines = append(lines, sDim.Render(fit(hdr, w)))

	ts := m.st.sorted()
	foot := []string{}
	if b := m.banner(w); b != "" {
		foot = append(foot, b)
	}
	foot = append(foot, m.lastEvent(w), m.keybar(w))
	avail := h - len(lines) - len(foot)
	var rows []string
	for i, x := range ts {
		rows = append(rows, m.ticketRow(x, w, i == m.sel, keyW))
		if i == m.sel && m.mode == modeList {
			el := m.entryLines(x, w-6)
			// keep the expansion modest so the list stays a list
			maxE := max(0, min(len(el), avail-len(ts)))
			for j, l := range el[:maxE] {
				pre := "   ├ "
				if j == maxE-1 {
					pre = "   └ "
				}
				rows = append(rows, sFaint.Render(pre)+l)
			}
		}
	}
	if len(ts) == 0 {
		rows = append(rows, "", sDim.Render("   Nothing tracked. ")+sKey.Render("a")+sDim.Render(" adds a ticket by key; recents are offered."))
	}
	// scroll so the selection stays visible
	if len(rows) > avail {
		start := max(0, min(m.sel-avail/2, len(rows)-avail))
		rows = rows[start : start+avail]
	}
	for len(rows) < avail {
		rows = append(rows, "")
	}
	lines = append(lines, rows...)
	lines = append(lines, foot...)
	base := strings.Join(lines, "\n")

	switch m.mode {
	case modeStop:
		box := modal("Stop "+m.stopT.key+" and send to Jira", m.stopForm(50), 54)
		return overlay(base, box, w, h)
	case modeAdd:
		box := modal("Add a ticket", m.addForm(40), 44)
		return overlay(base, box, w, h)
	}
	return base
}

func modal(title, body string, w int) string {
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cFg).Padding(0, 1).Width(w).
		Render(sBold.Render(title) + "\n\n" + body)
}

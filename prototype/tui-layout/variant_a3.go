// PROTOTYPE — throwaway. Variant A3: A2 after round two.
//   - no header strip: recents order puts the running ticket on row 1 anyway,
//     so the strip only repeated it. Today's total moves into the pane title.
//   - the banner keeps its rules (one slot, never dismissible, worst condition)
//     but loses the background block. F7 cycles where it sits:
//     top     a quiet line above the panes
//     bottom  a quiet line above the keys, in place of the last-event line
//     border  set into the Tickets pane's bottom border
package main

import (
	"strings"

	"charm.land/lipgloss/v2"
)

var bannerPlaces = []string{"top", "bottom", "border"}

// quietBanner: the same worst-condition text, coloured glyph, plain words.
func (m model) quietBanner() string {
	p, f := m.st.owed()
	switch {
	case m.scenario == scAuthFailed:
		return sFail.Render(gFailed) + " Jira rejected your token at " + m.now.Add(-38*60e9).Format("15:04") + sDim.Render(" · "+itoa(p+f)+" worklogs waiting · ") + sKey.Render("S") + sDim.Render(" to fix")
	case m.scenario == scNotConfigured:
		return sPaused.Render("!") + " Not connected to Jira" + sDim.Render(" — worklogs will wait · ") + sKey.Render("S") + sDim.Render(" to set up")
	case f > 0:
		return sFail.Render(gFailed) + " " + itoa(f) + " worklog failed to reach Jira" + sDim.Render(" · ") + sSync.Bold(true).Render("r") + sDim.Render(" to retry · nothing is lost")
	}
	return ""
}

// paneFoot is pane() with text set into the bottom border.
func paneFoot(title, foot, body string, w, h int, focused bool) string {
	p := pane(title, body, w, h, focused)
	if foot == "" {
		return p
	}
	bc := sFaint
	if focused {
		bc = lipgloss.NewStyle().Foreground(cFg)
	}
	lines := strings.Split(p, "\n")
	iw := w - 2
	foot = " " + fit(foot, min(lipgloss.Width(foot), iw-3)) + " "
	lines[len(lines)-1] = bc.Render("╰─") + foot + bc.Render(strings.Repeat("─", max(0, iw-1-lipgloss.Width(foot)))+"╯")
	return strings.Join(lines, "\n")
}

func (m model) viewA3(w, h int) string {
	place := bannerPlaces[m.bannerPlace]
	ban := m.quietBanner()
	var top []string
	if ban != "" && place == "top" {
		top = append(top, " "+fit(ban, w-1))
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
	title := "[1] Tickets " + sDim.Render("· today "+hm(m.st.today(m.now)))
	foot := ""
	if place == "border" {
		foot = ban
	}
	leftPane := paneFoot(title, foot, strings.Join(rows, "\n"), lw, bodyH, m.mode == modeList)

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
		t2, detail := "[2] Entries", ""
		if sel := m.selected(); sel != nil {
			t2 = "[2] " + sel.key
			wrapped := lipgloss.NewStyle().Width(rw - 2).Render(summaryText(sel))
			lines := []string{sDim.Render(wrapped), ""}
			lines = append(lines, m.entryLines(sel, rw-2)...)
			detail = strings.Join(lines, "\n")
		}
		right = lipgloss.JoinVertical(lipgloss.Left,
			pane(t2, detail, rw, detailH, false),
			pane("[3] Owed to Jira", strings.Join(q, "\n"), rw, qH, false))
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, right)
	status := m.lastEvent(w)
	if ban != "" && place == "bottom" && !(m.toast != "" && m.now.Sub(m.toastAt) < 5e9) && !m.syncing {
		status = " " + fit(ban, w-1)
	}
	return strings.Join(append(top, body, status, m.keybar(w)), "\n")
}

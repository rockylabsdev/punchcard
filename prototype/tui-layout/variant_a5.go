// PROTOTYPE — throwaway. Variant A5: A4 after round four ("what are these
// numbers?"). One number per timer, everything labelled.
//
//	[2] the headline is the work stop will send — the same number as the row
//	    while it runs; its stretches sit under it with pauses as their own
//	    lines, so the sum is visible. "in Jira 0m" became a WORKLOGS TODAY list.
//	[3] rows lead with the outcome in words, under a dim column header.
package main

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// outcome: the same words in the detail pane and the Jira strip
func outcome(e *entry) string {
	switch e.sync {
	case syncFailed:
		return sFail.Render(gFailed+" failed") + sDim.Render(" — "+strings.Trim(shortErr(e.syncErr), "()")+" · ") + sSync.Bold(true).Render("r")
	case syncPending:
		return sSync.Render(gPending+" not sent yet") + sDim.Render(" · ") + sSync.Bold(true).Render("r")
	case syncSynced:
		return sDim.Render(gSynced + " logged " + e.sentAt.Format("15:04"))
	}
	return ""
}

func spanLine(a, b string, d time.Duration, open bool, label string) string {
	return fit(a+sDim.Render("–")+b, 11) + " " + rfit(stretch(d, open), 7) + " " + label
}

func (m model) detailA5(t *ticket, w int) string {
	lines := []string{lipgloss.NewStyle().Width(w).Render(summaryText(t)), ""}
	head := func(l string, st lipgloss.Style, num string) {
		lines = append(lines, st.Render(l)+rfit(st.Render(num), w-lipgloss.Width(l)))
	}
	c := t.current()
	switch {
	case c == nil:
		lines = append(lines, sDim.Render(gIdle+" NOT RUNNING"), sDim.Render("  ")+sKey.Render("space")+sDim.Render(" starts it"))
	default:
		el := c.elapsed(m.now)
		switch {
		case c.open():
			head(gRunning+" RUNNING", sRunning, clock(el))
		case c.recovered:
			head(gRecovered+" RECOVERED", sRecov, hm(el))
			lines = append(lines, wrapIndent(sRecov, "capped at "+c.cappedAt.Format("15:04")+", your last presence — check it at stop", w))
		default:
			head(gPaused+" PAUSED", sPaused, hm(el))
		}
		lines = append(lines, wrapIndent(sDim, "stop sends this as one worklog — x", w), "")
		if c.recovered {
			lines = append(lines, gapLine(c.cappedAt.Format("15:04"), "now", sRecov, gRecovered+" away "+hm(m.now.Sub(c.cappedAt)), w)...)
		}
		for i := len(c.segs) - 1; i >= 0; i-- {
			s := c.segs[i]
			if s.end.IsZero() {
				lines = append(lines, " "+spanLine(s.start.Format("15:04"), sRunning.Render("now"), m.now.Sub(s.start), true, sRunning.Render("counted")))
			} else {
				lines = append(lines, " "+spanLine(s.start.Format("15:04"), s.end.Format("15:04"), s.end.Sub(s.start), false, "counted"))
			}
			if i > 0 {
				prev := c.segs[i-1]
				lines = append(lines, gapLine(prev.end.Format("15:04"), s.start.Format("15:04"), sDim, gPaused+" paused "+hm(s.start.Sub(prev.end)), w)...)
			}
		}
	}

	lines = append(lines, "", sDim.Render("WORKLOGS TODAY"))
	n := 0
	for i := len(t.entries) - 1; i >= 0; i-- {
		e := t.entries[i]
		if !e.stopped || !(sameDay(e.stoppedAt, m.now) || e.sync != syncSynced) {
			continue
		}
		lines = append(lines, "  "+rfit(hm(e.sendDur), 6)+"  "+outcome(e))
		n++
	}
	if n == 0 {
		lines = append(lines, sFaint.Render("  nothing yet"))
	}
	return strings.Join(lines, "\n")
}

func (m model) jiraLedgerA5(w, maxRows int) (title string, rows []string) {
	title, _ = m.jiraLedger(w, maxRows) // same title
	const stW = 30
	type item struct {
		rank int
		line string
	}
	var items []item
	for _, wl := range m.st.worklogs() {
		e := wl.e
		rank := map[syncState]int{syncFailed: 0, syncPending: 1, syncSynced: 2}[e.sync]
		if e.sync == syncSynced && !sameDay(e.sentAt, m.now) {
			continue
		}
		key := wl.t.key
		if e.sync == syncSynced {
			key = sDim.Render(key)
		}
		items = append(items, item{rank, " " + fit(outcome(e), stW) + fit(key, 10) + rfit(hm(e.sendDur), 7) + sDim.Render("   "+e.stoppedAt.Format("15:04"))})
	}
	for r := 0; r <= 2; r++ {
		for _, it := range items {
			if it.rank == r {
				rows = append(rows, it.line)
			}
		}
	}
	if len(rows) == 0 {
		return title, []string{sFaint.Render(" nothing stopped today — x on a row stops it and sends it here")}
	}
	if len(rows) > maxRows-1 {
		more := len(rows) - (maxRows - 1) + 1
		rows = append(rows[:maxRows-2], sDim.Render(" +"+itoa(more)+" more"))
	}
	hdr := sFaint.Render(" " + fit("STATUS", stW) + fit("TICKET", 10) + rfit("TIME", 7) + "   STOPPED")
	return title, append([]string{hdr}, rows...)
}

func (m model) viewA5(w, h int) string {
	jt, jrows := m.jiraLedgerA5(w-2, max(2, min(6, h/4)))
	jiraH := len(jrows) + 2
	bodyH := h - jiraH - 2
	lw := max(44, w*62/100)
	rw := w - lw

	ts := m.st.sorted()
	var rows []string
	for i, x := range ts {
		rows = append(rows, m.rowA4(x, lw-2, i == m.sel))
	}
	if len(ts) == 0 {
		rows = append(rows, "", sDim.Render("  No tickets yet."), sDim.Render("  Press ")+sKey.Render("a")+sDim.Render(" to add one by key."))
	}
	if len(rows) > bodyH-2 {
		start := max(0, min(m.sel-(bodyH-2)/2, len(rows)-(bodyH-2)))
		rows = rows[start:]
	}
	leftPane := pane("[1] Tickets "+sDim.Render("· today "+hm(m.allToday())), strings.Join(rows, "\n"), lw, bodyH, m.mode == modeList)

	var right string
	switch m.mode {
	case modeStop:
		right = pane("Stop "+m.stopT.key, lipgloss.NewStyle().Width(rw-2).Render(m.stopForm(rw-2)), rw, bodyH, true)
	case modeAdd:
		right = pane("Add ticket", m.addForm(rw-3), rw, bodyH, true)
	default:
		title, detail := "[2] Ticket", ""
		if sel := m.selected(); sel != nil {
			title = "[2] " + sel.key
			detail = m.detailA5(sel, rw-2)
		}
		right = pane(title, detail, rw, bodyH, false)
	}
	top := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, right)
	jira := pane(jt, strings.Join(jrows, "\n"), w, jiraH, false)

	status := m.lastEvent(w)
	if ban := m.quietBanner(); ban != "" && !(m.toast != "" && m.now.Sub(m.toastAt) < 5*time.Second) && !m.syncing {
		status = " " + fit(ban, w-1)
	}
	return strings.Join([]string{top, jira, status, m.keybar(w)}, "\n")
}

// wrapIndent wraps text to w with a two-space hanging indent.
func wrapIndent(st lipgloss.Style, text string, w int) string {
	wrapped := lipgloss.NewStyle().Width(w - 2).Render(text)
	ls := strings.Split(wrapped, "\n")
	for i, l := range ls {
		ls[i] = "  " + st.Render(strings.TrimRight(l, " "))
	}
	return strings.Join(ls, "\n")
}

// gapLine: time that is not counted (a pause, or time away after a cap). Its
// length lives in the label, out of the counted column, so it can't be summed
// by eye. "not counted" drops to its own line when the pane is narrow.
func gapLine(a, b string, st lipgloss.Style, label string, w int) []string {
	head := " " + st.Render(fit(a+"–"+b, 11)+" "+label)
	if lipgloss.Width(head)+len(", not counted") <= w {
		return []string{head + st.Render(", not counted")}
	}
	return []string{head, st.Render(strings.Repeat(" ", 13) + "not counted")}
}

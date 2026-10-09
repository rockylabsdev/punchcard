// PROTOTYPE — throwaway. Variant A4: A3 after round three. Each pane answers
// one question, in one unit of time, each a level deeper than the last.
//
//	[1] Tickets  what am I tracking, where did today go?  one number per row: time today
//	[2] Ticket   what happened on this one?   totals, then a timeline of stretches with fates
//	[3] Jira     what's my standing with Jira?   full-width ledger: owed, then sent today
//
// The banner sits on the status line (bottom) and only says *that* something
// is wrong; the Jira ledger says *what*.
package main

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

func midnight(now time.Time) time.Time {
	y, mo, d := now.Date()
	return time.Date(y, mo, d, 0, 0, 0, 0, now.Location())
}

// overlap of [a,b) with today
func todayPart(a, b, now time.Time) time.Duration {
	if b.IsZero() {
		b = now
	}
	if m0 := midnight(now); a.Before(m0) {
		a = m0
	}
	if b.Before(a) {
		return 0
	}
	return b.Sub(a)
}

func (m model) ticketToday(t *ticket) time.Duration {
	var d time.Duration
	for _, e := range t.entries {
		for _, s := range e.segs {
			d += todayPart(s.start, s.end, m.now)
		}
	}
	return d
}

func (m model) allToday() time.Duration {
	var d time.Duration
	for _, t := range m.st.tickets {
		d += m.ticketToday(t)
	}
	return d
}

func sameDay(a, b time.Time) bool { return !a.IsZero() && a.After(midnight(b)) }

// inJira / owed for one ticket (nil = all tickets): logged today, owed any day
func (m model) jiraTotals(only *ticket) (logged, owed time.Duration) {
	for _, t := range m.st.tickets {
		if only != nil && t != only {
			continue
		}
		for _, e := range t.entries {
			if !e.stopped {
				continue
			}
			switch e.sync {
			case syncSynced:
				if sameDay(e.sentAt, m.now) {
					logged += e.sendDur
				}
			case syncPending, syncFailed:
				owed += e.sendDur
			}
		}
	}
	return
}

func (m model) lastSync() time.Time {
	var last time.Time
	for _, t := range m.st.tickets {
		for _, e := range t.entries {
			if e.sentAt.After(last) {
				last = e.sentAt
			}
		}
	}
	return last
}

// a stretch's length: the open one ticks like the row's clock, closed ones are minutes
func stretch(d time.Duration, open bool) string {
	if open {
		return clock(d)
	}
	return hm(d)
}

func (m model) rowA4(t *ticket, w int, selected bool) string {
	cur := " "
	if selected {
		cur = gCursor
	}
	d := m.ticketToday(t)
	num := sDim.Render(hm(d))
	if d == 0 {
		num = sFaint.Render("—")
	}
	key, sum := t.key, summaryText(t)
	switch t.state() {
	case stRunning:
		key, sum, num = sRunning.Render(t.key), sRunning.Render(t.summary), sRunning.Render(clock(d))
	case stPaused:
		key = sPaused.Render(t.key)
		if t.recovered() {
			key = sRecov.Render(t.key)
		}
	}
	if c := m.changed(t); c != "" {
		sum = c + sDim.Render(" · ") + sum
	}
	sumW := w - 2 - 1 - 9 - 8 - 2
	row := cur + stateGlyph(t) + " " + fit(key, 9) + fit(sum, sumW) + rfit(num, 8) + " " + syncMarker(t)
	if selected {
		return sSel.Render(fit(row, w))
	}
	return fit(row, w)
}

type stretchLine struct {
	start, end time.Time
	fate       string
}

// detailA4: totals on top, then today's stretches newest first, each with its fate.
func (m model) detailA4(t *ticket, w int) string {
	lines := []string{lipgloss.NewStyle().Width(w).Render(summaryText(t)), ""}

	c := t.current()
	switch t.state() {
	case stRunning:
		lines = append(lines, sRunning.Render(gRunning+" running "+clock(c.elapsed(m.now))))
	case stPaused:
		if c.recovered {
			lines = append(lines, sRecov.Render(gRecovered+" recovered · "+hm(c.elapsed(m.now))))
		} else {
			lines = append(lines, sPaused.Render(gPaused+" paused at "+hm(c.elapsed(m.now))))
		}
	default:
		lines = append(lines, sDim.Render(gIdle+" not running — space starts it"))
	}
	logged, owed := m.jiraTotals(t)
	tot := "today " + hm(m.ticketToday(t)) + " · in Jira " + hm(logged)
	if owed > 0 {
		tot += " · owed " + hm(owed)
	}
	lines = append(lines, sDim.Render(tot))

	var today, earlier []stretchLine
	for _, e := range t.entries {
		for i := len(e.segs) - 1; i >= 0; i-- {
			s := e.segs[i]
			fate := ""
			if i == len(e.segs)-1 { // the entry's latest stretch carries its fate
				switch {
				case !e.stopped && e.open():
					fate = sRunning.Render(gRunning+" running · ") + sFail.Render("x") + sDim.Render(" stops & sends")
				case !e.stopped && e.recovered:
					fate = sRecov.Render(gRecovered + " capped " + e.cappedAt.Format("15:04"))
				case !e.stopped:
					fate = sPaused.Render(gPaused + " paused")
				case e.sync == syncSynced:
					fate = sDim.Render(gSynced + " in Jira " + hm(e.sendDur))
				case e.sync == syncPending:
					fate = sSync.Render(gPending + " waiting " + hm(e.sendDur))
				case e.sync == syncFailed:
					fate = sFail.Render(gFailed+" failed") + sDim.Render(" "+shortErr(e.syncErr))
				}
			}
			sl := stretchLine{s.start, s.end, fate}
			if s.end.IsZero() || s.end.After(midnight(m.now)) {
				today = append(today, sl)
			} else {
				earlier = append(earlier, sl)
			}
		}
	}
	sortStretches(today)
	sortStretches(earlier)
	render := func(xs []stretchLine) {
		for _, x := range xs {
			end, until := sRunning.Render("now  "), m.now
			if !x.end.IsZero() {
				end, until = x.end.Format("15:04"), x.end
			}
			l := x.start.Format("15:04") + sDim.Render("–") + end + " " + rfit(stretch(until.Sub(x.start), x.end.IsZero()), 7)
			if x.fate == "" {
				lines = append(lines, l)
			} else if lipgloss.Width(l)+2+lipgloss.Width(x.fate) <= w {
				lines = append(lines, l+"  "+x.fate)
			} else { // narrow: the fate drops to its own line
				lines = append(lines, l, "  "+x.fate)
			}
		}
	}
	lines = append(lines, "", sDim.Render("TODAY"))
	if len(today) == 0 {
		lines = append(lines, sFaint.Render("nothing yet"))
	}
	render(today)
	if len(earlier) > 0 {
		lines = append(lines, "", sDim.Render("EARLIER"))
		render(earlier)
	}
	return strings.Join(lines, "\n")
}

func shortErr(s string) string {
	s = strings.TrimPrefix(s, "Jira ")
	if i := strings.Index(s, " "); i > 0 {
		return "(" + s[:i] + ")"
	}
	return s
}

func sortStretches(xs []stretchLine) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j].start.After(xs[j-1].start); j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}

// jiraLedger: owed first (failed, then pending), then sent today.
func (m model) jiraLedger(w, maxRows int) (title string, rows []string) {
	logged, owed := m.jiraTotals(nil)
	title = "[3] Jira " + sDim.Render("· logged today "+hm(logged)+" · owed "+hm(owed))
	switch {
	case m.scenario == scNotConfigured:
		title += sDim.Render(" · ") + sPaused.Render("not connected")
	case m.scenario == scAuthFailed:
		title += sDim.Render(" · ") + sFail.Render("token rejected")
	default:
		if ls := m.lastSync(); !ls.IsZero() {
			title += sDim.Render(" · last sync " + ls.Format("15:04"))
		}
	}
	type item struct {
		rank int
		line string
	}
	var items []item
	for _, wl := range m.st.worklogs() {
		e := wl.e
		head := fit(wl.t.key, 9) + rfit(hm(e.sendDur), 7) + sDim.Render("  stopped "+e.stoppedAt.Format("15:04")+"  ")
		switch e.sync {
		case syncFailed:
			items = append(items, item{0, sFail.Render(gFailed) + " " + head + sFail.Render(e.syncErr) + sDim.Render(" — ") + sSync.Bold(true).Render("r") + sDim.Render(" retries")})
		case syncPending:
			why := "waiting"
			if m.scenario == scNotConfigured {
				why = "waiting for setup — S"
			}
			items = append(items, item{1, sSync.Render(gPending) + " " + head + sSync.Render(why)})
		case syncSynced:
			if sameDay(e.sentAt, m.now) {
				items = append(items, item{2, sDim.Render(gSynced+" ") + sDim.Render(fit(wl.t.key, 9)+rfit(hm(e.sendDur), 7)+"  stopped "+e.stoppedAt.Format("15:04")+"  sent "+e.sentAt.Format("15:04"))})
			}
		}
	}
	for r := 0; r <= 2; r++ {
		for _, it := range items {
			if it.rank == r {
				rows = append(rows, " "+it.line)
			}
		}
	}
	if len(rows) == 0 {
		rows = append(rows, sFaint.Render(" nothing stopped today — x on a row stops it and sends it here"))
	}
	if len(rows) > maxRows {
		more := len(rows) - maxRows + 1
		rows = append(rows[:maxRows-1], sDim.Render(fmt.Sprintf(" +%d more", more)))
	}
	return title, rows
}

func (m model) viewA4(w, h int) string {
	jt, jrows := m.jiraLedger(w-2, max(1, min(5, h/5)))
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
			detail = m.detailA4(sel, rw-2)
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

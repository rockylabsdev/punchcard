package tui

import (
	"time"

	"github.com/j-dsmith/punchcard/internal/core"
)

// ticketState is what a row's symbol says.
type ticketState int

const (
	stateIdle ticketState = iota
	stateRunning
	statePaused
	stateRecovered // paused by recovery: check the duration at stop
)

// stateOf reads a ticket's open (unstopped) entry.
func stateOf(t core.TrackedTicket) ticketState {
	for _, e := range t.Entries {
		switch {
		case e.Running():
			return stateRunning
		case e.Paused() && e.Recovered:
			return stateRecovered
		case e.Paused():
			return statePaused
		}
	}
	return stateIdle
}

// ticketsPane is [1] Tickets: what am I tracking and where did today go.
// The title carries the day's total.
func (m Model) ticketsPane(w, h int) []string {
	rows := m.rows()
	var total time.Duration
	keyW := 9
	for _, t := range rows {
		total += core.TimeToday(t, m.now)
		keyW = max(keyW, len(t.Key)+1)
	}
	title := "[1] Tickets " + sDim.Render("· today "+hm(total))

	var body []string
	for i, t := range rows {
		body = append(body, m.ticketRow(t, w-2, keyW, i == m.sel))
	}
	if len(rows) == 0 {
		body = []string{"",
			sDim.Render("  No tickets yet."),
			sDim.Render("  Press ") + sBold.Render("a") + sDim.Render(" to add one by key."),
		}
	}
	// Scroll so the cursor stays in view, centred where it can be.
	if visible := h - 2; visible > 0 && len(body) > visible {
		first := max(0, min(m.sel-visible/2, len(body)-visible))
		body = body[first:]
	}
	return pane(title, body, w, h, true)
}

// ticketRow: cursor, state symbol, key, summary, one number (time today:
// h:mm:ss while running, else 1h 12m, or — for none) and a sync marker.
func (m Model) ticketRow(t core.TrackedTicket, w, keyW int, selected bool) string {
	cur := " "
	if selected {
		cur = gCursor
	}
	d := core.TimeToday(t, m.now)
	num := sDim.Render(hm(d))
	if d == 0 {
		num = sFaint.Render("—")
	}
	glyph, key, sum := sDim.Render(gIdle), t.Key, summaryText(t)
	switch stateOf(t) {
	case stateRunning:
		glyph, key, sum, num = sRunning.Render(gRunning), sRunning.Render(t.Key), sRunning.Render(t.Summary), sRunning.Render(clock(d))
	case statePaused:
		glyph, key = sPaused.Render(gPaused), sPaused.Render(t.Key)
	case stateRecovered:
		glyph, key = sRecov.Render(gRecovered), sRecov.Render(t.Key)
	}
	if tg := m.tagged(t.Key); tg != "" {
		sum = sChanged.Render(tg) + sDim.Render(" · ") + sum
	}
	const numW = 8
	sumW := w - 2 - 1 - keyW - numW - 2
	row := cur + glyph + " " + fit(key, keyW) + fit(sum, sumW) + rfit(num, numW) + " " + syncMarker(t)
	if selected {
		return sSel.Render(fit(row, w))
	}
	return fit(row, w)
}

// summaryText is honest about a ticket Jira has not confirmed.
func summaryText(t core.TrackedTicket) string {
	if t.Unverified {
		return sDim.Render(gUnverified + " unverified — Jira not reached yet")
	}
	return t.Summary
}

// syncMarker: ✗ if any worklog failed, ○ if any waits to be sent.
func syncMarker(t core.TrackedTicket) string {
	pending := false
	for _, e := range t.Entries {
		if e.Worklog == nil {
			continue
		}
		switch e.Worklog.State {
		case core.WorklogFailed:
			return sFail.Render(gFailed)
		case core.WorklogPending:
			pending = true
		}
	}
	if pending {
		return sSync.Render(gPending)
	}
	return " "
}

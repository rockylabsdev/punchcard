package tui

import (
	"time"

	"github.com/j-dsmith/punchcard/internal/core"
)

// stateOf is t's timer state as shown: the core's, over the shown World,
// where a stale timer already appears recovered (so never stale).
func (m Model) stateOf(t core.TrackedTicket) core.TimerState {
	return core.StateOf(m.shown(), t, m.now)
}

// recoveredPause: t's open entry was paused by recovery, so the row
// says check the duration at stop.
func recoveredPause(t core.TrackedTicket) bool {
	e, ok := openEntry(t)
	return ok && e.Paused() && e.Recovered
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
		if m.noTracker {
			// Said once, here, and gone with the first ticket: no
			// tracker is a fine way to work, not a fault to nag about.
			body = append(body, "", sFaint.Render("  no tracker · ")+sBold.Render("S")+sFaint.Render(" to set one up"))
		}
	}
	// Scroll so the cursor stays in view, centred where it can be.
	if visible := h - 2; visible > 0 && len(body) > visible {
		first := max(0, min(m.sel-visible/2, len(body)-visible))
		body = body[first:]
	}
	return m.pane(title, body, w, h, true)
}

// ticketRow: cursor, state symbol, key, summary, one number (time today:
// h:mm:ss while running, else 1h 12m, or — for none) and a sync marker.
func (m Model) ticketRow(t core.TrackedTicket, w, keyW int, selected bool) string {
	cur := " "
	if selected {
		cur = m.pal.brand.Render(gCursor)
	}
	d := core.TimeToday(t, m.now)
	num := sDim.Render(hm(d))
	if d == 0 {
		num = sFaint.Render("—")
	}
	glyph, key, sum := sDim.Render(gIdle), t.Key, summaryText(t)
	switch st := m.stateOf(t); {
	case st == core.TimerRunning:
		glyph, key, sum, num = m.pal.running.Render(gRunning), m.pal.running.Render(t.Key), m.pal.running.Render(t.Summary), m.pal.running.Render(clock(d))
	case recoveredPause(t):
		glyph, key = sRecov.Render(gRecovered), sRecov.Render(t.Key)
	case st == core.TimerPaused:
		glyph, key = sPaused.Render(gPaused), sPaused.Render(t.Key)
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

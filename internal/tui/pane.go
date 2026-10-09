package tui

import (
	"regexp"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/j-dsmith/punchcard/internal/core"
)

// ticketPane is [2] Ticket: what happened on the selected ticket. The
// full summary, a headline that is the work stop would send, the open
// entry's stretches newest first (counted, and the not-counted pauses
// and time away that explain the number), then WORKLOGS TODAY. An open
// dialog is laid over this pane by frame(); it never resizes it.
func (m Model) ticketPane(w, h int) []string {
	t, ok := m.selected()
	if !ok {
		return pane("[2] Ticket", nil, w, h, false)
	}
	return pane("[2] "+t.Key, m.ticketDetail(t, w-2), w, h, false)
}

func (m Model) ticketDetail(t core.TrackedTicket, w int) []string {
	if w <= 0 {
		return nil
	}
	lines := append(wrap(summaryText(t), w), "")
	head := func(label string, st lipgloss.Style, num string) {
		lines = append(lines, st.Render(label)+rfit(st.Render(num), w-lipgloss.Width(label)))
	}
	e, open := openEntry(t)
	if !open {
		lines = append(lines, sDim.Render(gIdle+" NOT RUNNING"), sDim.Render("  ")+sBold.Render("space")+sDim.Render(" starts it"))
	} else {
		el := core.Elapsed(e, m.now)
		switch stateOf(t) {
		case stateRunning:
			head(gRunning+" RUNNING", sRunning, clock(el))
		case stateRecovered:
			head(gRecovered+" RECOVERED", sRecov, hm(el))
		default:
			head(gPaused+" PAUSED", sPaused, hm(el))
		}
		if e.Recovered && e.Running() {
			lines = append(lines, indent(sRecov, "recovered earlier — check the duration at stop", w)...)
		}
		lines = append(lines, indent(sDim, "stop sends this as one worklog — x", w)...)
		lines = append(lines, "")
		lines = append(lines, m.stretches(e, w)...)
	}
	lines = append(lines, "", sDim.Render("WORKLOGS TODAY"))
	n := 0
	for i := len(t.Entries) - 1; i >= 0; i-- {
		wl := t.Entries[i].Worklog
		if wl == nil || (wl.State == core.WorklogSynced && !sameDay(wl.SyncedAt.Time, m.now)) {
			continue // owed from any day, sent only from today
		}
		d := time.Duration(wl.TimeSpentSeconds) * time.Second
		lines = append(lines, "  "+rfit(hm(d), 6)+"  "+outcome(*wl, m.now))
		n++
	}
	if n == 0 {
		lines = append(lines, sFaint.Render("  nothing yet"))
	}
	return lines
}

// stretches are the open entry's segments newest first, with the gaps
// between them. Only counted stretches put a number in the number
// column; a gap carries its length in its label, so it cannot be summed
// by eye.
func (m Model) stretches(e core.TimeEntry, w int) []string {
	var lines []string
	hhmm := func(ts core.Timestamp) string { return ts.In(m.now.Location()).Format("15:04") }
	n := len(e.Segments)
	if e.Recovered && e.Paused() {
		end := e.Segments[n-1].EndedAt
		lines = append(lines, gapLine(hhmm(end), "now", sRecov, gRecovered+" away "+hm(m.now.Sub(end.Time)), w)...)
	}
	for i := n - 1; i >= 0; i-- {
		s := e.Segments[i]
		if s.Open() {
			d := m.now.Sub(s.StartedAt.Time)
			lines = append(lines, " "+fit(hhmm(s.StartedAt)+"–"+sRunning.Render("now"), 11)+" "+
				rfit(sRunning.Render(clock(d)), 7)+" "+sRunning.Render("counted"))
		} else {
			d := s.EndedAt.Sub(s.StartedAt.Time)
			lines = append(lines, " "+fit(hhmm(s.StartedAt)+"–"+hhmm(s.EndedAt), 11)+" "+rfit(hm(d), 7)+" counted")
		}
		if i > 0 {
			prev := e.Segments[i-1].EndedAt
			lines = append(lines, gapLine(hhmm(prev), hhmm(s.StartedAt), sDim, gPaused+" paused "+hm(s.StartedAt.Sub(prev.Time)), w)...)
		}
	}
	return lines
}

// gapLine is time that is not counted: a pause, or time away after
// recovery capped a timer. ", not counted" drops to its own line when
// the pane is narrow.
func gapLine(from, to string, st lipgloss.Style, label string, w int) []string {
	head := " " + fit(from+"–"+to, 11) + " " + label
	if lipgloss.Width(head)+len(", not counted") <= w {
		return []string{st.Render(head + ", not counted")}
	}
	return []string{st.Render(head), st.Render(strings.Repeat(" ", 13) + "not counted")}
}

// indent wraps text to w under a two-space indent.
func indent(st lipgloss.Style, text string, w int) []string {
	var out []string
	for _, l := range wrap(text, max(1, w-2)) {
		out = append(out, "  "+st.Render(l))
	}
	return out
}

// openEntry is t's running or paused entry, if any.
func openEntry(t core.TrackedTicket) (core.TimeEntry, bool) {
	for _, e := range t.Entries {
		if !e.Stopped() && len(e.Segments) > 0 {
			return e, true
		}
	}
	return core.TimeEntry{}, false
}

// outcome is a worklog's standing with Jira, leading with the outcome.
// The Jira strip uses the same words (slice 9 appends "· r" once r
// syncs).
func outcome(wl core.Worklog, now time.Time) string {
	switch wl.State {
	case core.WorklogFailed:
		return sFail.Render(gFailed+" failed") + sDim.Render(" — "+shortReason(wl.Message))
	case core.WorklogPending:
		return sSync.Render(gPending + " not sent yet")
	}
	return sDim.Render(gSynced + " logged " + wl.SyncedAt.In(now.Location()).Format("15:04"))
}

var statusPattern = regexp.MustCompile(`^Jira answered (\d{3})`)

// shortReason is a failure message cut to what fits a row: the HTTP
// status when Jira answered, else the message itself.
func shortReason(msg string) string {
	if m := statusPattern.FindStringSubmatch(msg); m != nil {
		return m[1]
	}
	return msg
}

func sameDay(a, b time.Time) bool {
	a = a.In(b.Location())
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

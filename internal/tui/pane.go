package tui

import (
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/tracker"
)

// ticketPane is [2] Ticket: what happened on the selected ticket. The
// full summary, a headline that is the work stop would send, the open
// entry's stretches newest first (segments bare, and the pauses and time
// away that explain the number marked by their reason), then WORKLOGS
// TODAY. An open
// dialog is laid over this pane by frame(); it never resizes it.
func (m Model) ticketPane(w, h int) []string {
	t, ok := m.selected()
	if !ok {
		return m.pane("[2] Ticket", nil, w, h, false)
	}
	return m.pane("[2] "+t.Key, m.ticketDetail(t, w-2), w, h, false)
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
		switch {
		case m.stateOf(t) == core.TimerRunning:
			head(gRunning+" RUNNING", m.pal.running, clock(el))
		case recoveredPause(t):
			head(gRecovered+" RECOVERED", sRecov, hm(el))
		default:
			head(gPaused+" PAUSED", sPaused, hm(el))
		}
		if e.Recovered && e.Running() {
			lines = append(lines, indent(sRecov, "recovered earlier — check the duration at stop", w)...)
		}
		lines = append(lines, indent(sDim, "stop "+m.sends()+" this as one worklog — x", w)...)
		if out := leftOut(e); out != "" {
			lines = append(lines, indent(sDim, out, w)...)
		}
		lines = append(lines, "")
		lines = append(lines, m.stretches(e, w)...)
	}
	lines = append(lines, "", sDim.Render("WORKLOGS TODAY"))
	n := 0
	for i := len(t.Entries) - 1; i >= 0; i-- {
		wl := t.Entries[i].Worklog
		if wl == nil || !m.inDayLog(*wl) {
			continue
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

// leftOut says once what the stretch list leaves out of the worklog:
// the pauses between segments, the time away (cut by the user, or after
// a recovered entry's last segment), or both. A single segment with
// nothing away leaves nothing out.
func leftOut(e core.TimeEntry) string {
	paused, away := false, e.Recovered && e.Paused()
	for i := 1; i < len(e.Segments); i++ {
		if e.Segments[i-1].Cut {
			away = true
		} else {
			paused = true
		}
	}
	switch {
	case paused && away:
		return "pauses and time away are left out"
	case paused:
		return "pauses are left out"
	case away:
		return "time away is left out"
	}
	return ""
}

// stretches are the open entry's segments newest first, with the gaps
// between them. A segment is bare: its length in the number column. A
// gap is marked by its reason and carries its length in its label, so
// the number column sums by eye to the headline.
func (m Model) stretches(e core.TimeEntry, w int) []string {
	var lines []string
	hhmm := func(ts core.Timestamp) string { return ts.In(m.now.Location()).Format("15:04") }
	n := len(e.Segments)
	if e.Recovered && e.Paused() {
		end := e.Segments[n-1].EndedAt
		lines = append(lines, gapLine(hhmm(end), "now", sRecov, gRecovered+" away "+hm(m.now.Sub(end.Time)), w))
	}
	for i := n - 1; i >= 0; i-- {
		s := e.Segments[i]
		if s.Open() {
			d := m.now.Sub(s.StartedAt.Time)
			lines = append(lines, " "+fit(hhmm(s.StartedAt)+"–"+m.pal.running.Render("now"), 11)+" "+
				rfit(m.pal.running.Render(clock(d)), 7))
		} else {
			d := s.EndedAt.Sub(s.StartedAt.Time)
			lines = append(lines, " "+fit(hhmm(s.StartedAt)+"–"+hhmm(s.EndedAt), 11)+" "+rfit(stretchHM(d), 7))
		}
		if i > 0 {
			prev := e.Segments[i-1]
			why := " paused "
			if prev.Cut {
				why = " away " // the user cut it out: pause or start at a time
			}
			lines = append(lines, gapLine(hhmm(prev.EndedAt), hhmm(s.StartedAt), sDim, gPaused+why+hm(s.StartedAt.Sub(prev.EndedAt.Time)), w))
		}
	}
	return lines
}

// gapLine is time left out of the worklog: a pause, or time away after
// recovery capped a timer. It is always one line, cut with … when the
// pane is narrow.
func gapLine(from, to string, st lipgloss.Style, label string, w int) string {
	return st.Render(ansi.Truncate(" "+fit(from+"–"+to, 11)+" "+label, w, "…"))
}

// stretchHM is a closed segment's length: hm, except that under a
// minute is <1m, not 0m.
func stretchHM(d time.Duration) string {
	if d < time.Minute {
		return "<1m"
	}
	return hm(d)
}

// indent wraps text to w under a two-space indent.
func indent(st lipgloss.Style, text string, w int) []string {
	var out []string
	for _, l := range wrap(text, max(1, w-2)) {
		out = append(out, "  "+st.Render(l))
	}
	return out
}

// sends is what a stop does with the worklog: sends it to the tracker,
// or with none, logs it.
func (m Model) sends() string {
	if m.noTracker {
		return "logs"
	}
	return "sends"
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

// outcome is a worklog's standing, leading with the outcome, in the
// words [2] and the day's log share. An owed one ends "· r": r sends
// it. A local one is logged and has no moment the tracker took it.
func outcome(wl core.Worklog, now time.Time) string {
	switch wl.State {
	case core.WorklogFailed:
		return sFail.Render(gFailed+" failed") + sDim.Render(" — "+shortReason(wl)) + retry
	case core.WorklogPending:
		return sSync.Render(gPending+" not sent yet") + retry
	case core.WorklogLocal:
		return sDim.Render(gSynced + " logged")
	}
	return sDim.Render(gSynced + " logged " + wl.SyncedAt.In(now.Location()).Format("15:04"))
}

// retry is the r key after an owed worklog; the key is sync blue.
var retry = sDim.Render(" · ") + sSyncKey.Render("r")

// shortReason is a failed worklog's reason cut to what fits a row: the
// refusal's status when the tracker answered, "unreachable" when it did
// not, else the message itself cut short, so the row's "· r" always
// shows.
func shortReason(wl core.Worklog) string {
	if wl.RefusedStatus != 0 {
		return strconv.Itoa(wl.RefusedStatus)
	}
	if strings.HasPrefix(wl.Message, tracker.ErrUnreachable.Error()) {
		return "unreachable"
	}
	return ansi.Truncate(wl.Message, 14, "…")
}

func sameDay(a, b time.Time) bool {
	a = a.In(b.Location())
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

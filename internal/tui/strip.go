package tui

import (
	"cmp"
	"slices"
	"strconv"
	"time"

	"github.com/j-dsmith/punchcard/internal/core"
)

// dayLog is [3] Today: the day's log. The title says logged today, and
// with a tracker owed · last sync (or why nothing can sync); the rows
// lead with the outcome, worst first: with a tracker, owed worklogs
// from any day, then the ones logged today. Without one, only those
// logged today: nothing is owed to anyone. At most maxRows rows, the
// last of them "+N more" when they do not fit; the pane's height
// follows.
func (m Model) dayLog(w, maxRows int) []string {
	title := m.dayLogTitle()
	items := m.stripItems()
	var body []string
	if len(items) == 0 {
		body = []string{sFaint.Render(" nothing logged today — x on a row stops it and logs it here")}
	} else {
		body = append(body, sDim.Render(" "+fit("STATUS", stripStatusW)+fit("TICKET", stripKeyW)+rfit("TIME", 7)+"   STOPPED"))
		more := ""
		if maxRows = max(1, maxRows); len(items) > maxRows {
			more = sDim.Render(" +" + strconv.Itoa(len(items)-(maxRows-1)) + " more")
			items = items[:maxRows-1]
		}
		body = append(body, m.stripRows(items)...)
		if more != "" {
			body = append(body, more)
		}
	}
	return m.pane(title, body, w, len(body)+2, false)
}

const (
	stripStatusW = 30 // the outcome column
	stripKeyW    = 10
)

// stripItem is one worklog on the strip.
type stripItem struct {
	key     string
	wl      core.Worklog
	stopped time.Time // the end of the entry's last segment
}

// rank orders the strip: failed, then waiting, then sent.
func (s stripItem) rank() int {
	switch s.wl.State {
	case core.WorklogFailed:
		return 0
	case core.WorklogPending:
		return 1
	}
	return 2
}

// stripItems are the worklogs the day's log shows, worst first and
// newest first within each outcome.
func (m Model) stripItems() []stripItem {
	var items []stripItem
	for _, t := range m.shown().State.TrackedTickets {
		for _, e := range t.Entries {
			wl := e.Worklog
			if wl == nil || !m.inDayLog(*wl) {
				continue
			}
			var stopped time.Time
			if n := len(e.Segments); n > 0 {
				stopped = e.Segments[n-1].EndedAt.Time
			}
			items = append(items, stripItem{key: t.Key, wl: *wl, stopped: stopped})
		}
	}
	slices.SortStableFunc(items, func(a, b stripItem) int {
		if c := cmp.Compare(a.rank(), b.rank()); c != 0 {
			return c
		}
		return b.stopped.Compare(a.stopped)
	})
	return items
}

func (m Model) stripRows(items []stripItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		key := it.key
		if it.wl.Logged() {
			key = sDim.Render(key)
		}
		d := time.Duration(it.wl.TimeSpentSeconds) * time.Second
		out = append(out, " "+fit(outcome(it.wl, m.now), stripStatusW)+fit(key, stripKeyW)+rfit(hm(d), 7)+
			sDim.Render("   "+m.when(it.stopped)))
	}
	return out
}

// when is a moment on the strip: 15:04 today, else with its date.
func (m Model) when(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	if sameDay(t, m.now) {
		return t.In(m.now.Location()).Format("15:04")
	}
	return t.In(m.now.Location()).Format("Jan 2 15:04")
}

// inDayLog: logged today, or (with a tracker) owed from any day.
func (m Model) inDayLog(wl core.Worklog) bool {
	if wl.Logged() {
		return loggedToday(wl, m.now)
	}
	return !m.noTracker
}

// loggedToday: wl was logged today. A synced worklog counts on the day
// the tracker took it, as it always has; a local one has no such
// moment, so it counts on its own day, the day it started (CONTEXT.md).
func loggedToday(wl core.Worklog, now time.Time) bool {
	switch wl.State {
	case core.WorklogSynced:
		return sameDay(wl.SyncedAt.Time, now)
	case core.WorklogLocal:
		return sameDay(wl.StartedAt.Time, now)
	}
	return false
}

// dayLogTitle: logged today, and with a tracker owed · last sync, where
// the last part says instead why nothing can sync.
func (m Model) dayLogTitle() string {
	st := m.shown().State
	var logged time.Duration
	var last time.Time
	for _, t := range st.TrackedTickets {
		for _, e := range t.Entries {
			wl := e.Worklog
			if wl == nil || !wl.Logged() {
				continue
			}
			if loggedToday(*wl, m.now) {
				logged += time.Duration(wl.TimeSpentSeconds) * time.Second
			}
			if wl.SyncedAt.After(last) {
				last = wl.SyncedAt.Time
			}
		}
	}
	title := "[3] Today " + sDim.Render("· logged "+hm(logged))
	if m.noTracker {
		return title
	}
	_, owed := core.Owed(st)
	title += sDim.Render(" · owed " + hm(owed))
	switch {
	case !st.AuthFailedAt.IsZero():
		title += sDim.Render(" · ") + sFail.Render("token rejected")
	case m.keychainError() != nil:
		title += sDim.Render(" · ") + sWarn.Render("Keychain error")
	case m.notConnected():
		title += sDim.Render(" · ") + sWarn.Render("not connected")
	case !last.IsZero():
		title += sDim.Render(" · last sync " + m.when(last))
	}
	return title
}

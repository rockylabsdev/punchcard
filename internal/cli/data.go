package cli

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/j-dsmith/punchcard/internal/config"
	"github.com/j-dsmith/punchcard/internal/core"
)

// The `data` member of each command's envelope. Words follow CONTEXT.md:
// running|paused|stale for a timer, pending|synced|failed|local for a worklog.

// Timer is one open (running or paused) entry.
type Timer struct {
	Key        string          `json:"key"`
	Summary    string          `json:"summary,omitempty"`
	Unverified bool            `json:"unverified"`
	EntryID    string          `json:"entryId"`
	State      core.TimerState `json:"state"` // running|paused|stale
	ByHand     bool            `json:"byHand"`
	Origin     core.Origin     `json:"origin"`
	Recovered  bool            `json:"recovered"`
	// ElapsedSeconds is the measured time; for a stale timer it is
	// capped at lastSeen, the figure the next change will record.
	ElapsedSeconds int `json:"elapsedSeconds"`
	// UncappedSeconds and LastSeen are set only when stale.
	UncappedSeconds int            `json:"uncappedSeconds,omitempty"`
	LastSeen        core.Timestamp `json:"lastSeen,omitzero"`
}

// Owed counts the worklogs still to reach the tracker (never a local one).
type Owed struct {
	Count   int `json:"count"`
	Seconds int `json:"seconds"`
}

// StatusData is `data` for status, and for start, pause, resume and
// touch (the world after the change).
type StatusData struct {
	Running *Timer  `json:"running"`
	Paused  []Timer `json:"paused"`
	Unsent  Owed    `json:"unsent"`
	// TodaySeconds is the day's total over every ticket, removed ones
	// included: a stopped entry's worklog duration on its day, open time
	// up to now, a stale timer capped (core.TodayTotal).
	TodaySeconds int            `json:"todaySeconds"`
	AuthFailedAt core.Timestamp `json:"authFailedAt"` // null unless Jira last said 401
}

// StatusReport is `data` for status alone: the snapshot plus the
// tracker in effect and, with Jira, whether it is configured, where the
// token comes from, and what broke. Only status looks this up (it reads
// the token; touch never does).
type StatusReport struct {
	StatusData
	Tracker config.Tracker `json:"tracker"` // none|jira
	// Here is --cwd's key: a *Here, which is null when the directory
	// names none. Absent without --cwd (a nil interface), since there is
	// no directory to answer for.
	Here any `json:"here,omitempty"`
	// Configured and the rest are present only with Jira.
	Configured  *bool         `json:"configured,omitempty"`
	TokenSource config.Source `json:"tokenSource,omitempty"` // env|keychain|file
	// StrayTokenFile is the token file's path when the Keychain won.
	StrayTokenFile string `json:"strayTokenFile,omitempty"`
	// KeychainError is why the Keychain could not be read (with no
	// file to fall back on); configured is then false.
	KeychainError string `json:"keychainError,omitempty"`
	// NotConfigured says why configured is false, in words.
	NotConfigured string `json:"notConfigured,omitempty"`
}

// Ticket is one row of `ls` (and of `add`).
type Ticket struct {
	Key        string         `json:"key"`
	Summary    string         `json:"summary,omitempty"`
	Unverified bool           `json:"unverified"`
	AddedAt    core.Timestamp `json:"addedAt"`
	// RemovedAt is set only for a removed ticket, which only ls --all lists.
	RemovedAt core.Timestamp `json:"removedAt,omitzero"`
	// State is the open entry's (running|paused|stale); absent when idle.
	State core.TimerState `json:"state,omitempty"`
	// ElapsedSeconds is the open entry's elapsed (capped if stale), else 0.
	ElapsedSeconds int `json:"elapsedSeconds"`
	TodaySeconds   int `json:"todaySeconds"`
	Unsynced       int `json:"unsynced"`
}

// ListData is `data` for ls and add.
type ListData struct {
	Tickets []Ticket `json:"tickets"`
}

// StopData is `data` for stop: the worklog it recorded.
type StopData struct {
	Key     string       `json:"key"`
	EntryID string       `json:"entryId"`
	Worklog core.Worklog `json:"worklog"`
}

// Preview is the exact worklog a stop would send.
type Preview struct {
	Key       string       `json:"key"`
	EntryID   string       `json:"entryId"`
	Worklog   core.Worklog `json:"worklog"`
	Recovered bool         `json:"recovered"`
}

// PreviewData is `data` with outcome needs_confirmation.
type PreviewData struct {
	Preview Preview `json:"preview"`
}

// RemoveData is `data` for rm. KeptWorklogs counts the ticket's logged
// worklogs, which stay in the log.
type RemoveData struct {
	Key          string `json:"key"`
	KeptWorklogs int    `json:"keptWorklogs"`
}

// UnsentWorklog is one worklog still owed to the tracker.
type UnsentWorklog struct {
	Key     string       `json:"key"`
	EntryID string       `json:"entryId"`
	Worklog core.Worklog `json:"worklog"`
}

// SyncData is `data` for sync: what was sent and what remains.
type SyncData struct {
	Sent      []UnsentWorklog `json:"sent"`
	Remaining []UnsentWorklog `json:"remaining"`
}

// CandidatesData is `data` when a target is ambiguous (exit 4).
type CandidatesData struct {
	Candidates []string `json:"candidates"`
}

// VersionData is `data` for `version --json`.
type VersionData struct {
	Version       string `json:"version"`
	Contract      int    `json:"contract"`
	SchemaVersion int    `json:"schemaVersion"`
	Go            string `json:"go"`
	Platform      string `json:"platform"`
}

// timerOf describes an open entry at now; a stale running timer is
// reported capped, as recovery would record it.
func timerOf(w core.World, t core.TrackedTicket, e core.TimeEntry, now time.Time) Timer {
	tm := Timer{
		Key: t.Key, Summary: t.Summary, Unverified: t.Unverified, EntryID: e.ID,
		ByHand: e.ByHand, Origin: e.Origin, Recovered: e.Recovered,
		ElapsedSeconds: seconds(core.Elapsed(e, now)),
		State:          core.EntryState(w, e, now),
	}
	if tm.State == core.TimerStale {
		r, _ := core.Recover(w, now)
		capped, _ := r.State.Entry(t.Key, e.ID)
		tm.UncappedSeconds = tm.ElapsedSeconds
		tm.ElapsedSeconds = seconds(core.Elapsed(capped, now))
		tm.LastSeen = core.EffectiveLastSeen(w.Heartbeat, e)
	}
	return tm
}

// snapshot is StatusData for w at now. Paused timers are in ls order.
func snapshot(w core.World, now time.Time) StatusData {
	d := StatusData{Paused: []Timer{}, AuthFailedAt: w.State.AuthFailedAt}
	for _, t := range core.ListOrder(w.State, now) {
		for _, e := range t.Entries {
			switch {
			case e.Running():
				tm := timerOf(w, t, e, now)
				d.Running = &tm
			case e.Paused():
				d.Paused = append(d.Paused, timerOf(w, t, e, now))
			}
		}
	}
	n, total := core.Owed(w.State)
	d.Unsent = Owed{Count: n, Seconds: seconds(total)}
	d.TodaySeconds = seconds(core.TodayTotal(w, now))
	return d
}

// ticketRow is one ls row for t.
func ticketRow(w core.World, t core.TrackedTicket, now time.Time) Ticket {
	r := Ticket{Key: t.Key, Summary: t.Summary, Unverified: t.Unverified, AddedAt: t.AddedAt, RemovedAt: t.RemovedAt,
		TodaySeconds: seconds(core.TimeToday(t, now))}
	for _, e := range t.Entries {
		switch {
		case e.Stopped():
			if !e.Worklog.Logged() {
				r.Unsynced++
			}
		default:
			tm := timerOf(w, t, e, now)
			r.State, r.ElapsedSeconds = tm.State, tm.ElapsedSeconds
		}
	}
	return r
}

// listData is the list's rows, only those keys if only is set; all
// includes removed tickets.
func listData(w core.World, now time.Time, only []string, all bool) ListData {
	d := ListData{Tickets: []Ticket{}}
	order := core.ListOrder
	if all {
		order = core.AllOrder
	}
	for _, t := range order(w.State, now) {
		if only == nil || slices.Contains(only, t.Key) {
			d.Tickets = append(d.Tickets, ticketRow(w, t, now))
		}
	}
	return d
}

func seconds(d time.Duration) int { return int(d / time.Second) }

// Human rendering.

// timerLine is the status line for one timer.
func (o output) timerLine(tm Timer, loc *time.Location, now time.Time) string {
	return o.timerLineAt(tm, loc, now, core.Timestamp{})
}

// timerLineAt is timerLine for a change made at a named time at: the
// line says when (running since 10:45, paused at 10:00).
func (o output) timerLineAt(tm Timer, loc *time.Location, now time.Time, at core.Timestamp) string {
	when := func(prep string) string {
		if at.IsZero() {
			return ""
		}
		return " " + prep + " " + core.Clock(at.Time, loc)
	}
	var b strings.Builder
	switch tm.State {
	case core.TimerRunning:
		fmt.Fprintf(&b, "%s %s running%s  %s", o.paint(green, "▶"), tm.Key, when("since"), clock(secs(tm.ElapsedSeconds)))
		if tm.ByHand {
			b.WriteString("  by hand")
		}
	case core.TimerPaused:
		fmt.Fprintf(&b, "%s %s paused%s  %s", o.paint(yellow, "⏸"), tm.Key, when("at"), clock(secs(tm.ElapsedSeconds)))
	case core.TimerStale:
		fmt.Fprintf(&b, "%s %s stale  %s (capped; %s uncapped)\n", o.paint(red, "!"), tm.Key,
			clock(secs(tm.ElapsedSeconds)), clock(secs(tm.UncappedSeconds)))
		fmt.Fprintf(&b, "  last seen %s (%s ago); the next action will recover it",
			core.Clock(tm.LastSeen.Time, loc), age(now.Sub(tm.LastSeen.Time)))
	}
	if tm.Recovered {
		b.WriteString("  (recovered: check the duration at stop)")
	}
	if tm.Summary != "" && tm.State != core.TimerStale {
		b.WriteString("  " + tm.Summary)
	}
	b.WriteString("\n")
	return b.String()
}

// describe is one event as a human line. w is the world after the
// change (it says whether an added ticket was verified).
func (o output) describe(e core.Event, w core.World, loc *time.Location) string {
	switch e.Kind {
	case core.EventAdded:
		for _, t := range w.State.TrackedTickets {
			if t.Key == e.Key && !t.Unverified {
				return fmt.Sprintf("+ %s added\n", e.Key)
			}
		}
		return fmt.Sprintf("+ %s added (not verified with Jira yet)\n", e.Key)
	case core.EventRestored:
		return fmt.Sprintf("+ %s restored\n", e.Key)
	case core.EventPaused:
		if !e.At.IsZero() {
			s := fmt.Sprintf("%s %s paused at %s\n", o.paint(yellow, "⏸"), e.Key, core.Clock(e.At.Time, loc))
			if e.Message != "" {
				s += "  " + e.Message + "\n"
			}
			return s
		}
		return fmt.Sprintf("%s %s paused\n", o.paint(yellow, "⏸"), e.Key)
	case core.EventByHand:
		return fmt.Sprintf("%s set by hand: agents can't move it\n", e.Key)
	case core.EventStarted:
		if !e.At.IsZero() {
			return fmt.Sprintf("%s %s started at %s\n", o.paint(green, "▶"), e.Key, core.Clock(e.At.Time, loc))
		}
		return fmt.Sprintf("%s %s started\n", o.paint(green, "▶"), e.Key)
	case core.EventRecovered:
		return fmt.Sprintf("%s %s recovered: no presence since %s, so it was capped there; fix the duration at stop\n",
			o.paint(red, "!"), e.Key, core.Clock(e.At.Time, loc))
	default:
		return fmt.Sprintf("%s %s\n", e.Kind, e.Key)
	}
}

func secs(n int) time.Duration { return time.Duration(n) * time.Second }

// clock formats a duration as h:mm:ss.
func clock(d time.Duration) string {
	d = d.Truncate(time.Second)
	return fmt.Sprintf("%d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}

// short formats a duration as h:mm, for status --short.
func short(d time.Duration) string {
	return fmt.Sprintf("%d:%02d", int(d.Hours()), int(d.Minutes())%60)
}

// worklogDuration formats a worklog's time as 1h30m.
func worklogDuration(sec int) string {
	d := secs(sec)
	h, m := int(d.Hours()), int(d.Minutes())%60
	switch {
	case h == 0:
		return fmt.Sprintf("%dm", m)
	case m == 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dh%dm", h, m)
	}
}

// age formats how long ago, coarsely: 3h15m, 12m, 40s.
func age(d time.Duration) string {
	switch {
	case d >= time.Hour:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}

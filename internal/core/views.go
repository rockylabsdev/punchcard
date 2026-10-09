package core

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Elapsed is the measured time of an entry: the sum of its segments, with
// an open segment counted up to now. It is never truncated.
func Elapsed(e TimeEntry, now time.Time) time.Duration {
	var d time.Duration
	for _, s := range e.Segments {
		end := now
		if !s.Open() {
			end = s.EndedAt.Time
		}
		if end.After(s.StartedAt.Time) {
			d += end.Sub(s.StartedAt.Time)
		}
	}
	return d
}

// TimeToday is the part of t's segments (every entry) inside the local
// calendar day containing now, in now's location. A segment spanning
// midnight is split there; an open segment counts up to now.
func TimeToday(t TrackedTicket, now time.Time) time.Duration {
	y, m, d := now.Date()
	dayStart := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	dayEnd := dayStart.AddDate(0, 0, 1)
	var total time.Duration
	for _, e := range t.Entries {
		for _, s := range e.Segments {
			from, to := s.StartedAt.Time, now
			if !s.Open() && s.EndedAt.Before(now) {
				to = s.EndedAt.Time
			}
			if from.Before(dayStart) {
				from = dayStart
			}
			if to.After(dayEnd) {
				to = dayEnd
			}
			if to.After(from) {
				total += to.Sub(from)
			}
		}
	}
	return total
}

// LastActivity is the latest moment anything happened on t: added, a
// segment started or ended, or now while it runs.
func LastActivity(t TrackedTicket, now time.Time) time.Time {
	last := t.AddedAt.Time
	for _, e := range t.Entries {
		for _, s := range e.Segments {
			end := s.EndedAt.Time
			if s.Open() {
				end = now
			}
			for _, c := range []time.Time{s.StartedAt.Time, end} {
				if c.After(last) {
					last = c
				}
			}
		}
	}
	return last
}

// ListOrder returns the tracked tickets most recently active first (ties
// by key). This is also the recents order; it returns a new slice.
func ListOrder(s State, now time.Time) []TrackedTicket {
	out := slices.Clone(s.TrackedTickets)
	slices.SortStableFunc(out, func(a, b TrackedTicket) int {
		if c := LastActivity(b, now).Compare(LastActivity(a, now)); c != 0 {
			return c
		}
		return strings.Compare(a.Key, b.Key)
	})
	return out
}

// Preview is the exact worklog a stop would produce, and where.
type Preview struct {
	Key       string
	EntryID   string
	Worklog   Worklog
	Recovered bool // the entry is (or would first be) recovered
}

// PreviewStop is what Apply(w, stop, now) would record, without recording
// it: recovery is applied first, so a stale timer previews its capped
// elapsed. It refuses exactly as Stop would.
func PreviewStop(w World, stop Stop, now time.Time) (Preview, error) {
	r, _ := Recover(w, now)
	a := &applier{w: r, now: At(now)}
	t, e, err := a.stopTarget(stop.Key)
	if err != nil {
		return Preview{}, err
	}
	if stop.EntryID != "" && e.ID != stop.EntryID {
		return Preview{}, fmt.Errorf("%w: entry %s is no longer open", ErrNoTimer, stop.EntryID)
	}
	wl, err := worklogFor(*e, stop, a.now)
	if err != nil {
		return Preview{}, err
	}
	return Preview{Key: t.Key, EntryID: e.ID, Worklog: wl, Recovered: e.Recovered}, nil
}

// UnsentWorklog names a worklog that still has to reach the tracker.
type UnsentWorklog struct {
	Key     string
	EntryID string
	Worklog Worklog
}

// Unsent lists every pending or failed worklog in list order. A synced
// worklog is never offered: it must never be POSTed again. Nor is a
// local one: there is no tracker to send it to.
func Unsent(s State) []UnsentWorklog {
	var out []UnsentWorklog
	for _, t := range s.TrackedTickets {
		for _, e := range t.Entries {
			if e.Worklog != nil && !e.Worklog.Logged() {
				out = append(out, UnsentWorklog{Key: t.Key, EntryID: e.ID, Worklog: *e.Worklog})
			}
		}
	}
	return out
}

// Owed is how many worklogs are unsent and how much time they carry.
func Owed(s State) (count int, total time.Duration) {
	for _, u := range Unsent(s) {
		count++
		total += time.Duration(u.Worklog.TimeSpentSeconds) * time.Second
	}
	return count, total
}

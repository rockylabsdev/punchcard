package core

import "time"

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

// UnsentWorklog names a worklog that still has to reach Jira.
type UnsentWorklog struct {
	Key     string
	EntryID string
	Worklog Worklog
}

// Unsent lists every pending or failed worklog in list order. A synced
// worklog is never offered: it must never be POSTed again.
func Unsent(s State) []UnsentWorklog {
	var out []UnsentWorklog
	for _, t := range s.TrackedTickets {
		for _, e := range t.Entries {
			if e.Worklog != nil && e.Worklog.State != WorklogSynced {
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

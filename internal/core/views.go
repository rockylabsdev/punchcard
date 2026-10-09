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

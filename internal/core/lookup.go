package core

import "time"

// Entry finds t's entry with this id.
func (t TrackedTicket) Entry(id string) (TimeEntry, bool) {
	for _, e := range t.Entries {
		if e.ID == id {
			return e, true
		}
	}
	return TimeEntry{}, false
}

// Ticket finds the tracked ticket with this key.
func (s State) Ticket(key string) (TrackedTicket, bool) {
	for _, t := range s.TrackedTickets {
		if t.Key == key {
			return t, true
		}
	}
	return TrackedTicket{}, false
}

// Entry finds the entry with this id on the ticket with this key.
func (s State) Entry(key, id string) (TimeEntry, bool) {
	if t, ok := s.Ticket(key); ok {
		return t.Entry(id)
	}
	return TimeEntry{}, false
}

// LastPaused is the most recently paused entry and its ticket: the one
// a bare resume reopens. ok is false when nothing is paused.
func LastPaused(s State) (t TrackedTicket, e TimeEntry, ok bool) {
	i, j := lastPaused(s)
	if i < 0 {
		return TrackedTicket{}, TimeEntry{}, false
	}
	return s.TrackedTickets[i], s.TrackedTickets[i].Entries[j], true
}

// lastPaused is LastPaused as indices into s, (-1, -1) for none.
func lastPaused(s State) (ti, ei int) {
	ti, ei = -1, -1
	for i, t := range s.TrackedTickets {
		for j, e := range t.Entries {
			if e.Paused() && (ti < 0 || lastEnd(e).After(lastEnd(s.TrackedTickets[ti].Entries[ei]).Time)) {
				ti, ei = i, j
			}
		}
	}
	return ti, ei
}

// TimerState is what a ticket's timer is doing, in CONTEXT.md's words.
// The strings are the CLI contract's `state`.
type TimerState string

const (
	// TimerIdle: nothing running or paused (no open entry).
	TimerIdle TimerState = ""
	// TimerRunning: the open entry is accruing.
	TimerRunning TimerState = "running"
	// TimerPaused: the open entry is suspended.
	TimerPaused TimerState = "paused"
	// TimerStale: running with no presence for StaleAfter; the next
	// change recovers it.
	TimerStale TimerState = "stale"
)

// EntryState is e's state in w at now. A stopped entry is idle.
func EntryState(w World, e TimeEntry, now time.Time) TimerState {
	switch {
	case e.Paused():
		return TimerPaused
	case e.Running() && Stale(w, now):
		return TimerStale
	case e.Running():
		return TimerRunning
	default:
		return TimerIdle
	}
}

// StateOf is the state of t's open entry in w at now; idle with none.
func StateOf(w World, t TrackedTicket, now time.Time) TimerState {
	for _, e := range t.Entries {
		if s := EntryState(w, e, now); s != TimerIdle {
			return s
		}
	}
	return TimerIdle
}

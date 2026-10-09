package core

import "time"

const (
	// StaleAfter: a running timer with no presence for longer than this
	// is stale, and the next change recovers it.
	StaleAfter = 2 * time.Hour
	// PresenceThrottle: lastSeen is rewritten at most this often.
	PresenceThrottle = 30 * time.Second
)

// RecordPresence returns hb with lastSeen moved to now, unless the last
// write was under PresenceThrottle ago. A clock that went backwards is
// always rewritten. The store compares the result with hb to decide
// whether heartbeat.json needs writing.
func RecordPresence(hb Heartbeat, now time.Time) Heartbeat {
	last := hb.LastSeen.Time
	if !hb.LastSeen.IsZero() && !now.Before(last) && now.Sub(last) < PresenceThrottle {
		return hb
	}
	hb.SchemaVersion = SchemaVersion
	hb.LastSeen = At(now)
	return hb
}

// Stale reports whether the running timer has seen no presence for longer
// than StaleAfter. With no lastSeen ever recorded, the open segment's
// start stands in for it. Reading staleness never changes anything.
func Stale(w World, now time.Time) bool {
	_, e, ok := Running(w.State)
	if !ok {
		return false
	}
	return now.Sub(EffectiveLastSeen(w.Heartbeat, e).Time) > StaleAfter
}

// EffectiveLastSeen is the presence a running entry e is judged by:
// lastSeen, or with none ever recorded, e's open segment's start.
func EffectiveLastSeen(hb Heartbeat, e TimeEntry) Timestamp {
	if hb.LastSeen.IsZero() {
		return e.Segments[len(e.Segments)-1].StartedAt
	}
	return hb.LastSeen
}

// Recover caps a stale running timer at lastSeen (clamped to its open
// segment's start), flags the entry recovered and reports it. A timer
// that is not stale is returned unchanged with no events. Apply runs this
// first on every change; it is exported for views that need the capped
// figures (status of a stale timer).
func Recover(w World, now time.Time) (World, []Event) {
	a := &applier{w: w.clone(), now: At(now)}
	a.recover()
	return a.w, a.events
}

func (a *applier) recover() {
	if !Stale(a.w, a.now.Time) {
		return
	}
	t, e := a.running()
	open := &e.Segments[len(e.Segments)-1]
	end := EffectiveLastSeen(a.w.Heartbeat, *e)
	if end.Before(open.StartedAt.Time) {
		end = open.StartedAt
	}
	open.EndedAt = end
	e.Recovered = true
	a.events = append(a.events, Event{Kind: EventRecovered, Key: t.Key, At: end})
}

// Running returns the running ticket and entry; ok is false when nothing
// runs. There is at most one.
func Running(s State) (t TrackedTicket, e TimeEntry, ok bool) {
	for _, t := range s.TrackedTickets {
		for _, e := range t.Entries {
			if e.Running() {
				return t, e, true
			}
		}
	}
	return TrackedTicket{}, TimeEntry{}, false
}

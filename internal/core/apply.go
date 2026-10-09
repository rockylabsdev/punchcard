package core

import (
	"fmt"
	"time"
)

// Command is one requested transition. The concrete types below are the
// whole set.
type Command interface{ command() }

// Start starts the timer on Key. If Key is not tracked it is added with
// Summary and Unverified. If Key has an open (unstopped) entry it is
// resumed; otherwise a new entry with NewEntryID is created.
type Start struct {
	Key        string
	Summary    string
	Unverified bool
	Origin     Origin
	NewEntryID string
}

func (Start) command() {}

// EventKind names a side effect nobody asked for.
type EventKind string

const (
	EventAdded  EventKind = "added"  // start added an untracked ticket
	EventPaused EventKind = "paused" // another ticket's start paused this one
	EventHeld   EventKind = "held"   // a start by hand held the running timer
)

// Event is a side effect of a command, reported so nothing happens behind
// the user's back. Fields beyond Kind and Key are set per kind.
type Event struct {
	Kind EventKind `json:"kind"`
	Key  string    `json:"key"`
}

// Apply is the one transition: it takes the world as read fresh, a
// command and the current time, and returns the next world and the side
// effects. It never mutates w.
func Apply(w World, cmd Command, now time.Time) (World, []Event, error) {
	a := &applier{w: w.clone(), now: At(now)}
	var err error
	switch c := cmd.(type) {
	case Start:
		err = a.start(c)
	default:
		err = fmt.Errorf("%w: unknown command %T", ErrUsage, cmd)
	}
	if err != nil {
		return w, nil, err
	}
	return a.w, a.events, nil
}

// applier carries one transition's working copy and its events.
type applier struct {
	w      World
	now    Timestamp
	events []Event
}

func (a *applier) emit(kind EventKind, key string) {
	a.events = append(a.events, Event{Kind: kind, Key: key})
}

func (a *applier) start(c Start) error {
	if !ValidKey(c.Key) {
		return fmt.Errorf("%w: %q is not a ticket key", ErrUsage, c.Key)
	}
	if t, e := a.running(); e != nil {
		if t.Key == c.Key {
			if c.Origin == OriginHand && !e.Held {
				e.Held, e.Origin = true, OriginHand
				a.emit(EventHeld, t.Key)
			}
			return nil
		}
	}
	t := a.ticket(c.Key)
	e := openEntry(t)
	if e == nil && c.NewEntryID == "" {
		return fmt.Errorf("%w: a new entry needs an id", ErrUsage)
	}
	if t == nil {
		a.w.State.TrackedTickets = append(a.w.State.TrackedTickets, TrackedTicket{
			Key: c.Key, Summary: c.Summary, Unverified: c.Unverified, AddedAt: a.now, Entries: []TimeEntry{},
		})
		a.emit(EventAdded, c.Key)
	}
	a.pauseRunning(true)
	t = a.ticket(c.Key)
	if e = openEntry(t); e == nil {
		t.Entries = append(t.Entries, TimeEntry{ID: c.NewEntryID})
		e = &t.Entries[len(t.Entries)-1]
	}
	e.Segments = append(e.Segments, Segment{StartedAt: a.now})
	e.Held, e.Origin = c.Origin == OriginHand, c.Origin
	return nil
}

// pauseRunning closes the open segment, if any, at now.
func (a *applier) pauseRunning(announce bool) {
	t, e := a.running()
	if e == nil {
		return
	}
	e.Segments[len(e.Segments)-1].EndedAt = a.now
	if announce {
		a.emit(EventPaused, t.Key)
	}
}

// ticket returns a pointer into the working state, or nil.
func (a *applier) ticket(key string) *TrackedTicket {
	for i := range a.w.State.TrackedTickets {
		if a.w.State.TrackedTickets[i].Key == key {
			return &a.w.State.TrackedTickets[i]
		}
	}
	return nil
}

// running returns pointers to the running ticket and entry, or nils.
func (a *applier) running() (*TrackedTicket, *TimeEntry) {
	for i := range a.w.State.TrackedTickets {
		t := &a.w.State.TrackedTickets[i]
		for j := range t.Entries {
			if t.Entries[j].Running() {
				return t, &t.Entries[j]
			}
		}
	}
	return nil, nil
}

// openEntry returns the ticket's unstopped entry (running or paused).
func openEntry(t *TrackedTicket) *TimeEntry {
	if t == nil {
		return nil
	}
	for j := range t.Entries {
		if t.Entries[j].Worklog == nil {
			return &t.Entries[j]
		}
	}
	return nil
}

func (w World) clone() World {
	out := w
	out.State.TrackedTickets = make([]TrackedTicket, len(w.State.TrackedTickets))
	for i, t := range w.State.TrackedTickets {
		t.Entries = append([]TimeEntry{}, t.Entries...)
		for j, e := range t.Entries {
			e.Segments = append([]Segment(nil), e.Segments...)
			if e.Worklog != nil {
				wl := *e.Worklog
				e.Worklog = &wl
			}
			t.Entries[j] = e
		}
		out.State.TrackedTickets[i] = t
	}
	return out
}

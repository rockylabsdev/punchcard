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

// Pause closes the running segment. The entry stays open.
type Pause struct {
	Origin Origin
}

// Resume with a Key is Start with that Key. With no Key it resumes the
// most recently paused entry, and is a no-op while a timer runs.
type Resume struct {
	Key        string
	Origin     Origin
	NewEntryID string
}

// Stop ends an entry and produces its pending Worklog. With no Key it
// targets the running entry, else the single paused one. Duration zero
// means "use the measured elapsed"; either way it is truncated to the
// minute and refused under one minute. Stop does not gate on origin: the
// CLI owns needs_confirmation for agents.
type Stop struct {
	Key      string
	Duration time.Duration
	Comment  string
	Origin   Origin
}

func (Start) command()  {}
func (Pause) command()  {}
func (Resume) command() {}
func (Stop) command()   {}

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
	case Pause:
		err = a.pause(c)
	case Resume:
		err = a.resume(c)
	case Stop:
		err = a.stop(c)
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
	if err := a.pauseRunning(c.Origin, true); err != nil {
		return err
	}
	t = a.ticket(c.Key)
	if e = openEntry(t); e == nil {
		t.Entries = append(t.Entries, TimeEntry{ID: c.NewEntryID})
		e = &t.Entries[len(t.Entries)-1]
	}
	a.open(e, c.Origin)
	return nil
}

func (a *applier) pause(c Pause) error {
	if _, e := a.running(); e == nil {
		return fmt.Errorf("%w: nothing is running", ErrNoTimer)
	}
	return a.pauseRunning(c.Origin, false)
}

func (a *applier) resume(c Resume) error {
	if c.Key != "" {
		return a.start(Start{Key: c.Key, Origin: c.Origin, NewEntryID: c.NewEntryID})
	}
	if _, e := a.running(); e != nil {
		return nil
	}
	var latest *TimeEntry
	for i := range a.w.State.TrackedTickets {
		t := &a.w.State.TrackedTickets[i]
		for j := range t.Entries {
			e := &t.Entries[j]
			if e.Paused() && (latest == nil || lastEnd(*e).After(lastEnd(*latest).Time)) {
				latest = e
			}
		}
	}
	if latest == nil {
		return fmt.Errorf("%w: nothing is paused", ErrNoTimer)
	}
	a.open(latest, c.Origin)
	return nil
}

func (a *applier) stop(c Stop) error {
	_, e, err := a.stopTarget(c.Key)
	if err != nil {
		return err
	}
	wl, err := worklogFor(*e, c.Duration, c.Comment, a.now)
	if err != nil {
		return err
	}
	if e.Running() {
		e.Segments[len(e.Segments)-1].EndedAt = a.now
	}
	e.Worklog = &wl
	e.Recovered = false
	return nil
}

// stopTarget finds the entry a stop acts on (see Stop).
func (a *applier) stopTarget(key string) (*TrackedTicket, *TimeEntry, error) {
	if key != "" {
		t := a.ticket(key)
		if t == nil {
			return nil, nil, fmt.Errorf("%w: %s", ErrNotTracked, key)
		}
		e := openEntry(t)
		if e == nil {
			return nil, nil, fmt.Errorf("%w: %s has nothing running or paused", ErrNoTimer, key)
		}
		return t, e, nil
	}
	if t, e := a.running(); e != nil {
		return t, e, nil
	}
	var keys []string
	var t *TrackedTicket
	var e *TimeEntry
	for i := range a.w.State.TrackedTickets {
		if pe := openEntry(&a.w.State.TrackedTickets[i]); pe != nil {
			t, e = &a.w.State.TrackedTickets[i], pe
			keys = append(keys, t.Key)
		}
	}
	switch len(keys) {
	case 0:
		return nil, nil, fmt.Errorf("%w: nothing is running or paused", ErrNoTimer)
	case 1:
		return t, e, nil
	default:
		return nil, nil, &AmbiguousError{Keys: keys}
	}
}

// worklogFor is the pending worklog stopping e at now would produce.
func worklogFor(e TimeEntry, d time.Duration, comment string, now Timestamp) (Worklog, error) {
	if d == 0 {
		d = Elapsed(e, now.Time)
	}
	d = d.Truncate(time.Minute)
	if d < time.Minute {
		return Worklog{}, fmt.Errorf("%w: %s", ErrTooShort, d)
	}
	return Worklog{
		State:            WorklogPending,
		StartedAt:        e.Segments[0].StartedAt,
		TimeSpentSeconds: int(d / time.Second),
		Comment:          comment,
	}, nil
}

// open starts a new segment on e; held and origin describe this stretch.
func (a *applier) open(e *TimeEntry, origin Origin) {
	e.Segments = append(e.Segments, Segment{StartedAt: a.now})
	e.Held, e.Origin = origin == OriginHand, origin
}

// pauseRunning closes the open segment, if any, at now. Only an act by
// hand may close a held timer.
func (a *applier) pauseRunning(origin Origin, announce bool) error {
	t, e := a.running()
	if e == nil {
		return nil
	}
	if e.Held && origin != OriginHand {
		return fmt.Errorf("%w: %s was set by hand", ErrHeld, t.Key)
	}
	e.Segments[len(e.Segments)-1].EndedAt = a.now
	if announce {
		a.emit(EventPaused, t.Key)
	}
	return nil
}

func lastEnd(e TimeEntry) Timestamp { return e.Segments[len(e.Segments)-1].EndedAt }

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

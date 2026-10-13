package core

import (
	"fmt"
	"time"
)

// Apply is the one transition: it takes the world as read fresh, a
// command and the current time, and returns the next world and the side
// effects. It never mutates w.
//
// Order inside one transition: recover a stale timer first (capped at
// the old lastSeen), then apply the command, then record presence if
// the origin is presence. A refused command returns w unchanged, with no
// events: recovery is deferred to the next change, which loses nothing
// because lastSeen did not move.
func Apply(w World, cmd Command, now time.Time) (World, []Event, error) {
	a := &applier{w: w.clone(), now: At(now)}
	a.recover()
	recovered := len(a.events) > 0
	var err error
	var origin Origin
	switch c := cmd.(type) {
	case Start:
		origin, err = c.Origin, a.start(c)
	case Pause:
		origin = c.Origin
		if _, e := a.running(); e != nil || !recovered {
			err = a.pause(c)
		} // else: recovery already paused it, which is what was asked.
	case Resume:
		origin = c.Origin
		err = a.resume(c)
	case Stop:
		origin, err = c.Origin, a.stop(c)
	case Add:
		origin, err = c.Origin, a.add(c)
	case Remove:
		origin, err = c.Origin, a.remove(c)
	case Touch:
		var vouched bool
		vouched, err = a.touch(c)
		if vouched {
			origin = c.Origin
		}
	case MarkSynced:
		err = a.markSynced(c)
	case MarkFailed:
		err = a.markFailed(c)
	case MarkLocal:
		err = a.markLocal(c)
	case RecordAuthFailure:
		a.w.State.AuthFailedAt = a.now
	case ClearAuthFailure:
		a.w.State.AuthFailedAt = Timestamp{}
	case Verify:
		err = a.verify(c)
	default:
		err = fmt.Errorf("%w: unknown command %T", ErrUsage, cmd)
	}
	if err != nil {
		return w, nil, err
	}
	if origin.IsPresence() {
		a.w.Heartbeat = RecordPresence(a.w.Heartbeat, now)
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
	if !c.Grammar.Valid(c.Key) {
		return fmt.Errorf("%w: %q is not a ticket key", ErrUsage, c.Key)
	}
	if t, e := a.running(); e != nil {
		if t.Key == c.Key {
			a.track(NewTicket{Key: c.Key, Summary: c.Summary, Unverified: c.Unverified}) // confirms only
			if c.Origin == OriginHand && !e.ByHand {
				e.ByHand, e.Origin = true, OriginHand
				a.emit(EventByHand, t.Key)
			}
			return nil
		}
	}
	if err := a.guard(c.Origin); err != nil {
		return err
	}
	_, err := a.switchTo(NewTicket{Key: c.Key, Summary: c.Summary, Unverified: c.Unverified}, c.Origin, c.NewEntryID)
	return err
}

// switchTo closes any running segment (announced) and opens one on n's
// key, tracking n first if needed. It returns the entry it opened.
func (a *applier) switchTo(n NewTicket, origin Origin, newEntryID string) (*TimeEntry, error) {
	if openEntry(a.ticket(n.Key)) == nil && newEntryID == "" {
		return nil, fmt.Errorf("%w: a new entry needs an id", ErrUsage)
	}
	if err := a.pauseRunning(origin, true); err != nil {
		return nil, err
	}
	if a.track(n) {
		a.emit(EventAdded, n.Key)
	}
	t := a.ticket(n.Key)
	e := openEntry(t)
	if e == nil {
		t.Entries = append(t.Entries, TimeEntry{ID: newEntryID})
		e = &t.Entries[len(t.Entries)-1]
	}
	a.open(e, origin)
	return e, nil
}

func (a *applier) pause(c Pause) error {
	if _, e := a.running(); e == nil {
		return fmt.Errorf("%w: nothing is running", ErrNoTimer)
	}
	if err := a.guard(c.Origin); err != nil {
		return err
	}
	return a.pauseRunning(c.Origin, false)
}

func (a *applier) resume(c Resume) error {
	if c.Key != "" {
		return a.start(Start{Key: c.Key, Origin: c.Origin, NewEntryID: c.NewEntryID, Grammar: c.Grammar})
	}
	if _, e := a.running(); e != nil {
		return nil
	}
	i, j := lastPaused(a.w.State)
	if i < 0 {
		return fmt.Errorf("%w: nothing is paused", ErrNoTimer)
	}
	a.open(&a.w.State.TrackedTickets[i].Entries[j], c.Origin)
	return nil
}

func (a *applier) stop(c Stop) error {
	_, e, err := a.stopTarget(c.Key)
	if err != nil {
		return err
	}
	if c.EntryID != "" && e.ID != c.EntryID {
		return fmt.Errorf("%w: entry %s is no longer open", ErrNoTimer, c.EntryID)
	}
	wl, err := worklogFor(*e, c, a.now)
	if err != nil {
		return err
	}
	if e.Running() {
		e.Segments[len(e.Segments)-1].EndedAt = a.now
	}
	wl.Recovered = e.Recovered
	e.Worklog = &wl
	e.Origin = c.Origin
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

// worklogFor is the worklog stopping e with c at now would produce.
func worklogFor(e TimeEntry, c Stop, now Timestamp) (Worklog, error) {
	d := c.Duration
	if d == 0 {
		d = Elapsed(e, now.Time)
	}
	d = d.Truncate(time.Minute)
	if d < time.Minute {
		return Worklog{}, fmt.Errorf("%w: %s", ErrTooShort, d)
	}
	state := WorklogPending
	if c.Local {
		state = WorklogLocal
	}
	return Worklog{
		State:            state,
		StartedAt:        e.Segments[0].StartedAt,
		TimeSpentSeconds: int(d / time.Second),
		Comment:          c.Comment,
	}, nil
}

// open starts a new segment on e; ByHand and origin describe this stretch.
func (a *applier) open(e *TimeEntry, origin Origin) {
	e.Segments = append(e.Segments, Segment{StartedAt: a.now})
	e.ByHand, e.Origin = origin == OriginHand, origin
}

// guard refuses a start or pause of a timer set by hand unless it is by
// hand too. Focus is the user's and skips it (ADR 0010).
func (a *applier) guard(origin Origin) error {
	if t, e := a.running(); e != nil && e.ByHand && origin != OriginHand {
		return fmt.Errorf("%s %w", t.Key, ErrByHand)
	}
	return nil
}

// pauseRunning closes the open segment, if any, at now.
func (a *applier) pauseRunning(origin Origin, announce bool) error {
	t, e := a.running()
	if e == nil {
		return nil
	}
	e.Segments[len(e.Segments)-1].EndedAt = a.now
	e.Origin = origin
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

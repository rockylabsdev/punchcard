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
		if !c.At.IsZero() {
			err = a.pauseAt(c)
		} else if _, e := a.running(); e != nil || !recovered {
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
	named := !c.At.IsZero()
	when := a.now
	if named {
		when = At(c.At)
		if err := a.notFuture(when); err != nil {
			return err
		}
	}
	if t, e := a.running(); e != nil {
		if t.Key == c.Key {
			if named {
				return fmt.Errorf("%w: %s is already running; to cut time out of it, pause and resume at a time", ErrUsage, c.Key)
			}
			a.track(NewTicket{Key: c.Key, Summary: c.Summary, Unverified: c.Unverified}) // confirms only
			if c.Origin == OriginHand && !e.ByHand {
				e.ByHand, e.Origin = true, OriginHand
				a.emit(EventByHand, t.Key)
			}
			return nil
		}
	}
	if err := a.guardByHand(c.Origin); err != nil {
		return err
	}
	if named {
		var skip *Segment
		if t, e := a.running(); e != nil {
			skip = &e.Segments[len(e.Segments)-1]
			if when.Before(skip.StartedAt.Time) {
				return fmt.Errorf("%w: %s is before %s started at %s", ErrUsage, a.hhmm(when), t.Key, a.hhmm(skip.StartedAt))
			}
		}
		if err := a.overlaps(when, a.now, skip); err != nil {
			return err
		}
	}
	_, err := a.switchTo(NewTicket{Key: c.Key, Summary: c.Summary, Unverified: c.Unverified}, c.Origin, c.NewEntryID, when, named)
	if err == nil && named {
		a.events = append(a.events, Event{Kind: EventStarted, Key: c.Key, At: when})
	}
	return err
}

// switchTo closes any running segment (announced) and opens one on n's
// key, tracking n first if needed, both at when. named says when is a
// time the user named rather than now. It returns the entry it opened.
func (a *applier) switchTo(n NewTicket, origin Origin, newEntryID string, when Timestamp, named bool) (*TimeEntry, error) {
	if openEntry(a.ticket(n.Key)) == nil && newEntryID == "" {
		return nil, fmt.Errorf("%w: a new entry needs an id", ErrUsage)
	}
	if t, e := a.running(); e != nil {
		ev := Event{Kind: EventPaused, Key: t.Key}
		if named {
			ev.At = when
			ev.Message = a.closeAt(e, when, false)
		} else {
			e.Segments[len(e.Segments)-1].EndedAt = when
		}
		e.Origin = origin
		a.events = append(a.events, ev)
	}
	if ev := a.track(n); ev != "" {
		a.emit(ev, n.Key)
	}
	t := a.ticket(n.Key)
	e := openEntry(t)
	if e == nil {
		t.Entries = append(t.Entries, TimeEntry{ID: newEntryID})
		e = &t.Entries[len(t.Entries)-1]
	}
	a.openAt(e, origin, when)
	return e, nil
}

// pauseAt is a Pause at a named time: it closes the running segment
// then, or moves a paused entry's pause there (see Pause).
func (a *applier) pauseAt(c Pause) error {
	when := At(c.At)
	if err := a.notFuture(when); err != nil {
		return err
	}
	t, e := a.running()
	if e != nil && c.Key != "" && t.Key != c.Key {
		t, e = nil, nil
	}
	switch {
	case e != nil:
		if err := a.guardByHand(c.Origin); err != nil {
			return err
		}
	case c.Key != "":
		t = a.listed(c.Key)
		e = openEntry(t)
	default:
		if i, j := lastPaused(a.w.State); i >= 0 {
			t = &a.w.State.TrackedTickets[i]
			e = &t.Entries[j]
		}
	}
	if e == nil || len(e.Segments) == 0 {
		return fmt.Errorf("%w: nothing is running or paused", ErrNoTimer)
	}
	if e.ByHand && c.Origin != OriginHand {
		return fmt.Errorf("%s %w", t.Key, ErrByHand)
	}
	last := len(e.Segments) - 1
	s := e.Segments[last]
	if when.Before(s.StartedAt.Time) {
		since := "the entry started"
		if last > 0 {
			since = "it resumed"
		}
		return fmt.Errorf("%w: %s is before %s at %s", ErrUsage, a.hhmm(when), since, a.hhmm(s.StartedAt))
	}
	if !s.Open() && when.After(s.EndedAt.Time) {
		if err := a.overlaps(s.EndedAt, when, &e.Segments[last]); err != nil {
			return err
		}
	}
	msg := a.closeAt(e, when, true)
	e.Origin = c.Origin
	a.events = append(a.events, Event{Kind: EventPaused, Key: t.Key, At: when, Message: msg})
	return nil
}

// closeAt closes e's last segment at when, a time the user named, marked
// cut if cut. A segment that would be left under a minute is dropped,
// or with no other segment closed at its own start; the returned note
// says so ("" otherwise).
func (a *applier) closeAt(e *TimeEntry, when Timestamp, cut bool) string {
	i := len(e.Segments) - 1
	s := &e.Segments[i]
	d := when.Sub(s.StartedAt.Time)
	if d <= 0 || d >= time.Minute {
		s.EndedAt, s.Cut = when, cut
		return ""
	}
	note := fmt.Sprintf("the %s from %s was under a minute, so it was dropped", d.Round(time.Second), a.hhmm(s.StartedAt))
	if i > 0 {
		e.Segments = e.Segments[:i]
		e.Segments[i-1].Cut = e.Segments[i-1].Cut || cut
		return note
	}
	s.EndedAt, s.Cut = s.StartedAt, cut
	return note
}

// notFuture refuses a named time after now.
func (a *applier) notFuture(when Timestamp) error {
	if when.After(a.now.Time) {
		return fmt.Errorf("%w: %s is in the future", ErrUsage, a.hhmm(when))
	}
	return nil
}

// overlaps refuses measuring [from, to) when any segment but skip
// already has time in it: the user's time is one stream.
func (a *applier) overlaps(from, to Timestamp, skip *Segment) error {
	for i := range a.w.State.TrackedTickets {
		t := &a.w.State.TrackedTickets[i]
		for j := range t.Entries {
			for k := range t.Entries[j].Segments {
				s := &t.Entries[j].Segments[k]
				end := s.EndedAt
				if s.Open() {
					end = a.now
				}
				if s == skip || !s.StartedAt.Before(to.Time) || !end.After(from.Time) {
					continue
				}
				return fmt.Errorf("%w: that overlaps %s's time from %s to %s", ErrUsage,
					t.Key, a.hhmm(s.StartedAt), a.hhmm(end))
			}
		}
	}
	return nil
}

// hhmm is a time as the user named it, in now's zone.
func (a *applier) hhmm(ts Timestamp) string { return ts.In(a.now.Location()).Format("15:04") }

func (a *applier) pause(c Pause) error {
	if _, e := a.running(); e == nil {
		return fmt.Errorf("%w: nothing is running", ErrNoTimer)
	}
	if err := a.guardByHand(c.Origin); err != nil {
		return err
	}
	_, e := a.running()
	e.Segments[len(e.Segments)-1].EndedAt = a.now
	e.Origin = c.Origin
	return nil
}

func (a *applier) resume(c Resume) error {
	if c.Key != "" {
		return a.start(Start{Key: c.Key, At: c.At, Origin: c.Origin, NewEntryID: c.NewEntryID, Grammar: c.Grammar})
	}
	named := !c.At.IsZero()
	if t, e := a.running(); e != nil {
		if named {
			return fmt.Errorf("%w: %s is running; resume at a time reopens a paused entry", ErrUsage, t.Key)
		}
		return nil
	}
	i, j := lastPaused(a.w.State)
	if i < 0 {
		return fmt.Errorf("%w: nothing is paused", ErrNoTimer)
	}
	t, e := &a.w.State.TrackedTickets[i], &a.w.State.TrackedTickets[i].Entries[j]
	if !named {
		a.openAt(e, c.Origin, a.now)
		return nil
	}
	when := At(c.At)
	if err := a.notFuture(when); err != nil {
		return err
	}
	if end := lastEnd(*e); when.Before(end.Time) {
		return fmt.Errorf("%w: %s is before %s", ErrUsage, a.hhmm(when), a.hhmm(end))
	}
	if err := a.overlaps(when, a.now, nil); err != nil {
		return err
	}
	a.openAt(e, c.Origin, when)
	a.events = append(a.events, Event{Kind: EventStarted, Key: t.Key, At: when})
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
		t := a.listed(key)
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

// openAt starts a new segment on e at when; ByHand and origin describe
// this stretch.
func (a *applier) openAt(e *TimeEntry, origin Origin, when Timestamp) {
	e.Segments = append(e.Segments, Segment{StartedAt: when})
	e.ByHand, e.Origin = origin == OriginHand, origin
}

// guardByHand refuses a start or pause of a timer set by hand unless it
// is by hand too. Focus is the user's and skips it (ADR 0010).
func (a *applier) guardByHand(origin Origin) error {
	if t, e := a.running(); e != nil && e.ByHand && origin != OriginHand {
		return fmt.Errorf("%s %w", t.Key, ErrByHand)
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

// listed is ticket, but nil for a removed ticket: one off the list.
func (a *applier) listed(key string) *TrackedTicket {
	if t := a.ticket(key); t != nil && t.Listed() {
		return t
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

package core

import (
	"fmt"
	"slices"
)

func (a *applier) add(c Add) error {
	for _, n := range c.Tickets {
		if !c.Grammar.Valid(n.Key) {
			return fmt.Errorf("%w: %q is not a ticket key", ErrUsage, n.Key)
		}
	}
	for _, n := range c.Tickets {
		a.track(n)
	}
	return nil
}

// track adds n to the list if it is not there; it reports whether it did.
// If the tracked ticket is unverified and n is confirmed, it confirms
// it; a confirmed ticket is left alone.
func (a *applier) track(n NewTicket) bool {
	if t := a.ticket(n.Key); t != nil {
		if t.Unverified && !n.Unverified {
			t.Summary, t.Unverified = n.Summary, false
		}
		return false
	}
	a.w.State.TrackedTickets = append(a.w.State.TrackedTickets, TrackedTicket{
		Key: n.Key, Summary: n.Summary, Unverified: n.Unverified, AddedAt: a.now, Entries: []TimeEntry{},
	})
	return true
}

func (a *applier) verify(c Verify) error {
	t := a.ticket(c.Key)
	if t == nil {
		return fmt.Errorf("%w: %s", ErrNotTracked, c.Key)
	}
	t.Summary, t.Unverified = c.Summary, false
	a.w.State.AuthFailedAt = Timestamp{} // Jira answered
	return nil
}

func (a *applier) remove(c Remove) error {
	t := a.ticket(c.Key)
	if t == nil {
		return fmt.Errorf("%w: %s", ErrNotTracked, c.Key)
	}
	if !c.Force && owes(*t) {
		return fmt.Errorf("%w: %s has a running or paused entry or a worklog not yet sent", ErrUnlogged, c.Key)
	}
	a.w.State.TrackedTickets = slices.DeleteFunc(a.w.State.TrackedTickets, func(t TrackedTicket) bool { return t.Key == c.Key })
	return nil
}

// owes reports whether removing t would throw away time not yet logged.
func owes(t TrackedTicket) bool {
	for _, e := range t.Entries {
		if e.Worklog == nil || !e.Worklog.Logged() {
			return true
		}
	}
	return false
}

// touch applies focus and reports whether the call vouches for presence:
// a presence origin always does, whether or not it moves the timer.
// Focus moves only an un-held timer, never starts one from idle, and
// moves nothing unless it differs from the last focus, which every keyed
// presence touch records (ADR 0009).
func (a *applier) touch(c Touch) (bool, error) {
	if !c.Origin.IsPresence() {
		return false, nil
	}
	if c.Key == "" {
		return true, nil
	}
	if !c.Grammar.Valid(c.Key) {
		return false, fmt.Errorf("%w: %q is not a ticket key", ErrUsage, c.Key)
	}
	changed := c.Key != a.w.State.LastFocus
	a.w.State.LastFocus = c.Key
	rt, re := a.running()
	switch {
	case !changed:
		return true, nil // the same focus again: a switch away from it stands
	case re == nil:
		return true, nil // focus never starts a timer from idle
	case rt.Key == c.Key:
		return true, nil
	case re.Held:
		return true, nil // the prompt is presence; only hand moves a held timer
	}
	e, err := a.switchTo(NewTicket{Key: c.Key, Summary: c.Summary, Unverified: c.Unverified}, c.Origin, c.NewEntryID)
	if err != nil {
		return false, err
	}
	e.Held = false // focus never holds
	a.emit(EventStarted, c.Key)
	return true, nil
}

// stoppedEntry finds the entry a sync outcome is about.
func (a *applier) stoppedEntry(key, id string) (*TimeEntry, error) {
	t := a.ticket(key)
	if t == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotTracked, key)
	}
	for j := range t.Entries {
		e := &t.Entries[j]
		if e.ID != id {
			continue
		}
		if e.Worklog == nil {
			return nil, fmt.Errorf("%w: entry %s on %s is not stopped", ErrNoTimer, id, key)
		}
		if e.Worklog.State == WorklogSynced {
			return nil, fmt.Errorf("%w: %s worklog %s", ErrAlreadySynced, key, e.Worklog.WorklogID)
		}
		if e.Worklog.State == WorklogLocal {
			return nil, fmt.Errorf("%w: entry %s on %s", ErrLocal, id, key)
		}
		return e, nil
	}
	return nil, fmt.Errorf("%w: no entry %s on %s", ErrNoTimer, id, key)
}

func (a *applier) markSynced(c MarkSynced) error {
	e, err := a.stoppedEntry(c.Key, c.EntryID)
	if err != nil {
		return err
	}
	wl := e.Worklog
	wl.State, wl.WorklogID, wl.SyncedAt = WorklogSynced, c.WorklogID, a.now
	wl.clearFailure()
	a.ticket(c.Key).Unverified = false // an accepted worklog proves the key
	a.w.State.AuthFailedAt = Timestamp{}
	a.emit(EventSynced, c.Key)
	return nil
}

func (a *applier) markFailed(c MarkFailed) error {
	e, err := a.stoppedEntry(c.Key, c.EntryID)
	if err != nil {
		return err
	}
	wl := e.Worklog
	wl.State, wl.Message, wl.RefusedStatus, wl.FailedAt = WorklogFailed, c.Message, c.Status, a.now
	a.events = append(a.events, Event{Kind: EventSyncFailed, Key: c.Key, Message: c.Message})
	return nil
}

func (a *applier) markLocal(c MarkLocal) error {
	e, err := a.stoppedEntry(c.Key, c.EntryID)
	if err != nil {
		return err
	}
	wl := e.Worklog
	wl.State = WorklogLocal
	wl.clearFailure()
	return nil
}

// clearFailure forgets a failed send: the worklog is logged now.
func (wl *Worklog) clearFailure() {
	wl.Message, wl.RefusedStatus, wl.FailedAt = "", 0, Timestamp{}
}

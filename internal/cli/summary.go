package cli

import (
	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/tracker"
)

// ticketNotFound: the tracker answered 404 for a ticket new to the list.
// It is exit 5 and nothing changes. A 404 is missing or invisible; the
// copy in failure() says so.
type ticketNotFound struct{ key string }

func (e ticketNotFound) Error() string { return e.key + ": " + tracker.ErrNotFound.Error() }

// key is a key as typed, normalised by the grammar in effect (Jira
// upper-cases it). Whether it is valid is left to the caller or core.
func (a *app) key(typed string) string { return a.env.Grammar().Normalize(typed) }

// newTicket is what start, resume KEY and add record for a key. With no
// tracker there is nothing to ask: the ticket is added as typed, with no
// summary, and is never unverified. With one, only a ticket new to the
// list (or still unverified on it) fetches its summary, within
// tracker.SummaryBudget:
//   - found: a confirmed NewTicket, which also confirms an unverified one;
//   - 404 on a new ticket: ticketNotFound (exit 5, nothing changes);
//   - unreachable, unconfigured, 401, any other answer, or a 404 on a
//     ticket already on the list: unverified, and the change goes ahead.
//
// A confirmed ticket on the list never fetches. A bad key is left for
// core to refuse as usage, without a network call.
func (a *app) newTicket(key string) (core.NewTicket, error) {
	if !a.env.HasTracker() {
		return core.NewTicket{Key: key}, nil
	}
	n := core.NewTicket{Key: key, Unverified: true}
	if !a.env.Grammar().Valid(key) {
		return n, nil
	}
	w, _, err := a.read()
	if err != nil {
		return n, err
	}
	tracked := false
	for _, t := range w.State.TrackedTickets {
		if t.Key == key {
			if !t.Unverified {
				return n, nil
			}
			tracked = true
		}
	}
	l := tracker.LookUp(a.env.OpenTracker, key)
	if l.Notice != "" {
		a.out.notices([]string{l.Notice})
	}
	switch {
	case l.Outcome == tracker.LookupFound:
		// The tracker answered: a recorded 401 no longer holds.
		if !w.State.AuthFailedAt.IsZero() {
			if _, aerr := a.apply(core.ClearAuthFailure{}); aerr != nil {
				return n, aerr
			}
		}
		return core.NewTicket{Key: key, Summary: l.Summary}, nil
	case l.Outcome == tracker.LookupNotFound && !tracked:
		return n, ticketNotFound{key}
	case l.Outcome == tracker.LookupAuthFailed:
		// A 401 is a fact about the tracker, persisted for status and
		// the banner.
		if _, aerr := a.apply(core.RecordAuthFailure{}); aerr != nil {
			return n, aerr
		}
	}
	return n, nil
}

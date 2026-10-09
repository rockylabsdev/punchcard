package cli

import (
	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/jira"
)

// ticketNotFound: Jira answered 404 for a ticket new to the list. It is
// exit 5 and nothing changes. A 404 is missing or invisible; the copy
// in failure() says so.
type ticketNotFound struct{ key string }

func (e ticketNotFound) Error() string { return e.key + ": " + jira.ErrNotFound.Error() }

// newTicket is what start, resume KEY and add record for a key. Only a
// ticket new to the list (or still unverified on it) fetches its summary,
// within jira.SummaryBudget:
//   - found: a confirmed NewTicket, which also confirms an unverified one;
//   - 404 on a new ticket: ticketNotFound (exit 5, nothing changes);
//   - unreachable, unconfigured, 401, any other answer, or a 404 on a
//     ticket already on the list: unverified, and the change goes ahead.
//
// A confirmed ticket on the list never fetches. A bad key is left for
// core to refuse as usage, without a network call.
func (a *app) newTicket(key string) (core.NewTicket, error) {
	n := core.NewTicket{Key: key, Unverified: true}
	if !core.ValidKey(key) {
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
	l := jira.LookUp(a.env.Jira, key)
	if l.Notice != "" {
		a.out.notices([]string{l.Notice})
	}
	switch {
	case l.Outcome == jira.LookupFound:
		// Jira answered: a recorded 401 no longer holds.
		if !w.State.AuthFailedAt.IsZero() {
			if _, aerr := a.apply(core.ClearAuthFailure{}); aerr != nil {
				return n, aerr
			}
		}
		return core.NewTicket{Key: key, Summary: l.Summary}, nil
	case l.Outcome == jira.LookupNotFound && !tracked:
		return n, ticketNotFound{key}
	case l.Outcome == jira.LookupAuthFailed:
		// A 401 is a fact about Jira, persisted for status and the banner.
		if _, aerr := a.apply(core.RecordAuthFailure{}); aerr != nil {
			return n, aerr
		}
	}
	return n, nil
}

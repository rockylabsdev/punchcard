package tracker

import (
	"context"
	"errors"
)

// LookupOutcome is what a summary fetch found, as the CLI and the TUI
// act on it.
type LookupOutcome int

const (
	// LookupUnknown: the tracker is unconfigured, unreachable or
	// answered something else. The key stays unverified.
	LookupUnknown LookupOutcome = iota
	// LookupFound: the tracker named the ticket; Summary is set.
	LookupFound
	// LookupNotFound: a 404. A key new to the list is refused; a
	// tracked one simply stays unverified.
	LookupNotFound
	// LookupAuthFailed: a 401, to be recorded as authFailedAt. The
	// change still goes ahead, unverified.
	LookupAuthFailed
)

// Lookup is one summary fetch's answer.
type Lookup struct {
	Outcome LookupOutcome
	Summary string
	// Notice is set when opening the tracker failed for a reason other
	// than being unconfigured (a broken config): it is treated as unconfigured
	// and the user is told.
	Notice string
}

// LookUp asks the tracker for key's summary within SummaryBudget,
// through open (nil, or ErrNoTracker: no tracker at all). It never returns
// an error: every failure is an outcome. Callers with no tracker should
// not ask: there is nothing to verify.
func LookUp(open func() (Client, error), key string) Lookup {
	if open == nil {
		return Lookup{}
	}
	c, err := open()
	if err != nil {
		if errors.Is(err, ErrNotConfigured) || errors.Is(err, ErrNoTracker) {
			return Lookup{}
		}
		return Lookup{Notice: err.Error() + "; treating the tracker as not configured"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), SummaryBudget)
	defer cancel()
	s, err := c.Summary(ctx, key)
	switch {
	case err == nil:
		return Lookup{Outcome: LookupFound, Summary: s}
	case errors.Is(err, ErrNotFound):
		return Lookup{Outcome: LookupNotFound}
	case errors.Is(err, ErrUnauthorized):
		return Lookup{Outcome: LookupAuthFailed}
	default:
		return Lookup{}
	}
}

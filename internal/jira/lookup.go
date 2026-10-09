package jira

import (
	"context"
	"errors"

	"github.com/j-dsmith/punchcard/internal/config"
)

// LookupOutcome is what a summary fetch found, as the CLI and the TUI
// act on it.
type LookupOutcome int

const (
	// LookupUnknown: Jira is unconfigured, unreachable or answered
	// something else. The key stays unverified.
	LookupUnknown LookupOutcome = iota
	// LookupFound: Jira named the ticket; Summary is set.
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
	// Notice is set when opening Jira failed for a reason other than
	// being unconfigured (a broken config): it is treated as unconfigured
	// and the user is told.
	Notice string
}

// LookUp asks Jira for key's summary within SummaryBudget, through open
// (nil: no Jira at all). It never returns an error: every failure is an
// outcome.
func LookUp(open func() (Client, error), key string) Lookup {
	if open == nil {
		return Lookup{}
	}
	c, err := open()
	if err != nil {
		if errors.Is(err, config.ErrNotConfigured) {
			return Lookup{}
		}
		return Lookup{Notice: err.Error() + "; treating Jira as not configured"}
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

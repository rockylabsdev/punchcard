// Package tracker is the seam between Punchcard and the optional remote a
// worklog is sent to (ADR 0008). It owns the client interface, its value
// types, a tracker-neutral error set and the key grammar. Jira is one
// implementation (package jira); every caller matches these errors and
// never an implementation's.
package tracker

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// SummaryBudget is how long a summary fetch may take before the caller
// gives up and leaves the ticket unverified.
const SummaryBudget = 2 * time.Second

// Client is the whole tracker surface Punchcard uses.
type Client interface {
	// Summary verifies a key and fetches its ticket's summary. A key the
	// tracker does not know (or will not show) is ErrNotFound.
	Summary(ctx context.Context, key string) (string, error)
	// PostWorklog submits one worklog to key and returns the tracker's
	// id for it.
	PostWorklog(ctx context.Context, key string, w Worklog) (string, error)
	// TestConnection checks the site, then the credentials, and names
	// the account.
	TestConnection(ctx context.Context) (Account, error)
}

// Worklog is what PostWorklog tells the tracker. An empty Comment is
// omitted.
type Worklog struct {
	Started          time.Time
	TimeSpentSeconds int
	Comment          string
}

// Account is who the credentials authenticate as.
type Account struct {
	AccountID   string
	DisplayName string
	Email       string
}

// Outcomes every caller distinguishes. Match with errors.Is (and
// *Refusal with errors.As). The words name no tracker: an
// implementation that wants its own name in them wraps them (as
// jira.APIError does a Refusal).
var (
	// ErrNoTracker: none is configured. It is what opening a client
	// answers with `tracker: none`, and is not ErrNotConfigured: there is
	// nothing to do, not something to set up.
	ErrNoTracker = errors.New("no tracker")
	// ErrNotConfigured: a tracker is chosen but cannot be opened yet (no
	// config, a field or the token missing). Being unconfigured is
	// treated like being offline. config.ErrNotConfigured matches it.
	ErrNotConfigured = errors.New("the tracker is not set up")
	// ErrNotFound: the tracker does not know the key, or this account
	// cannot see it.
	ErrNotFound = errors.New("not found or not visible to this account")
	// ErrUnauthorized: the credentials were rejected.
	ErrUnauthorized = errors.New("credentials rejected by the tracker")
	// ErrUnreachable: no answer at all (network, DNS, timeout).
	ErrUnreachable = errors.New("could not reach the tracker")
	// ErrBadSite: the site is not one this tracker can talk to.
	ErrBadSite = errors.New("not a site this tracker can reach")
)

// Refusal is any other answer the tracker gave: an HTTP-like status and
// what it said. A 404 matches ErrNotFound and a 401 ErrUnauthorized.
type Refusal struct {
	Status  int
	Message string
}

func (r *Refusal) Error() string {
	if r.Message == "" {
		return fmt.Sprintf("the tracker answered %d", r.Status)
	}
	return fmt.Sprintf("the tracker answered %d: %s", r.Status, r.Message)
}

func (r *Refusal) Is(target error) bool {
	switch target {
	case ErrNotFound:
		return r.Status == 404
	case ErrUnauthorized:
		return r.Status == 401
	}
	return false
}

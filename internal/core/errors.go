package core

import (
	"errors"
	"regexp"
	"strings"
)

// Refusals. Apply wraps these with detail; callers match with errors.Is
// and map them to exit codes (ADR 0004).
var (
	// ErrUsage: the command itself is malformed (bad key, missing id).
	ErrUsage = errors.New("usage")
	// ErrNoTimer: nothing running or paused to act on.
	ErrNoTimer = errors.New("no timer")
	// ErrHeld: the running timer was set by hand and only another act by
	// hand may move or pause it.
	ErrHeld = errors.New("timer is held")
	// ErrTooShort: a worklog under one minute is never sent.
	ErrTooShort = errors.New("under one minute")
	// ErrNotTracked: the key is not on the list.
	ErrNotTracked = errors.New("ticket not tracked")
	// ErrUnlogged: removing the ticket would throw away time not yet in Jira.
	ErrUnlogged = errors.New("ticket has unlogged time")
	// ErrAlreadySynced: the idempotency guard. A synced worklog is never
	// POSTed or marked again.
	ErrAlreadySynced = errors.New("worklog already synced")
)

// AmbiguousError: stop with no key found several paused entries and
// nothing running. It matches ErrNoTimer.
type AmbiguousError struct{ Keys []string }

func (e *AmbiguousError) Error() string {
	return "several tickets are paused, name one: " + strings.Join(e.Keys, ", ")
}

func (e *AmbiguousError) Is(target error) bool { return target == ErrNoTimer }

var keyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]+-[0-9]+$`)

// ValidKey reports whether key looks like a Jira ticket key (ABC-123).
func ValidKey(key string) bool { return keyPattern.MatchString(key) }

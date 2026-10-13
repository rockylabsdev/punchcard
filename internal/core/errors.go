package core

import (
	"errors"
	"strings"
)

// Refusals. Apply wraps these with detail; callers match with errors.Is
// and map them to exit codes (ADR 0004).
var (
	// ErrUsage: the command itself is malformed (bad key, missing id).
	ErrUsage = errors.New("usage")
	// ErrNoTimer: nothing running or paused to act on.
	ErrNoTimer = errors.New("no timer")
	// ErrByHand: the running timer was set by hand, so an agent may not
	// move or pause it; only an act by hand or the user's focus may.
	ErrByHand = errors.New("was set by hand")
	// ErrTooShort: a worklog under one minute is never sent.
	ErrTooShort = errors.New("under one minute")
	// ErrNotTracked: the key is not on the list.
	ErrNotTracked = errors.New("ticket not tracked")
	// ErrUnlogged: removing the ticket would throw away time not yet
	// logged: an open entry, or a worklog the tracker has not been sent.
	ErrUnlogged = errors.New("ticket has unlogged time")
	// ErrAlreadySynced: the idempotency guard. A synced worklog is never
	// POSTed or marked again.
	ErrAlreadySynced = errors.New("worklog already synced")
	// ErrLocal: a local worklog was recorded with no tracker. It is final
	// and is never sent, so it is never marked synced or failed.
	ErrLocal = errors.New("worklog is local")
)

// AmbiguousError: stop with no key found several paused entries and
// nothing running. It matches ErrNoTimer.
type AmbiguousError struct{ Keys []string }

func (e *AmbiguousError) Error() string {
	return "several tickets are paused, name one: " + strings.Join(e.Keys, ", ")
}

func (e *AmbiguousError) Is(target error) bool { return target == ErrNoTimer }

// Reason is a refusal in the user's words: err's message without the
// sentinel it wraps ("usage: 10:45 is before 10:00" reads "10:45 is
// before 10:00"). Any other error is its message.
func Reason(err error) string {
	msg := err.Error()
	for _, s := range []error{ErrUsage, ErrNoTimer} {
		if errors.Is(err, s) {
			if r, ok := strings.CutPrefix(msg, s.Error()+": "); ok {
				return r
			}
		}
	}
	return msg
}

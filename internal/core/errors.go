package core

import (
	"errors"
	"regexp"
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
)

var keyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]+-[0-9]+$`)

// ValidKey reports whether key looks like a Jira ticket key (ABC-123).
func ValidKey(key string) bool { return keyPattern.MatchString(key) }

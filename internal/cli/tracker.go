package cli

import (
	"github.com/j-dsmith/punchcard/internal/config"
	"github.com/j-dsmith/punchcard/internal/jira"
	"github.com/j-dsmith/punchcard/internal/tracker"
)

// The tracker in effect, resolved once per invocation: which one config
// names, its key grammar, and the "open a client" seam every caller
// takes. This file is the one place the CLI names an implementation.

// chosenTracker is config's tracker. A config file punchcard cannot use
// counts as Jira: someone set it up, and opening the client reports the
// problem where Jira is needed, as before.
func chosenTracker(getenv func(string) string) config.Tracker {
	t, err := config.Chosen(getenv)
	if err != nil {
		return config.TrackerJira
	}
	return t
}

// Grammar is the key grammar of e's tracker: Jira's with Jira, else
// the slug.
func (e Env) Grammar() tracker.Grammar {
	if e.Tracker == config.TrackerJira {
		return jira.Grammar
	}
	return tracker.Slug
}

// HasTracker reports whether a tracker is configured (it may still lack
// credentials).
func (e Env) HasTracker() bool { return e.Tracker != config.TrackerNone }

// Retracked is e with the tracker read from config again, and the seam
// rebuilt for it. The TUI's S runs setup as a child process, which may
// choose another tracker while the TUI stays open.
func (e Env) Retracked() Env {
	e.Tracker = chosenTracker(e.Getenv)
	e.OpenTracker = e.openTracker(nil)
	return e
}

// openTracker is the seam for e's tracker: with none it answers
// tracker.ErrNoTracker and open is never called; with Jira it is open,
// or jira.Open (the config file plus the token lookup) when open is nil.
func (e Env) openTracker(open func() (tracker.Client, error)) func() (tracker.Client, error) {
	if !e.HasTracker() {
		return func() (tracker.Client, error) { return nil, tracker.ErrNoTracker }
	}
	if open != nil {
		return open
	}
	getenv, kc := e.Getenv, e.Keychain
	return func() (tracker.Client, error) { return jira.Open(getenv, kc) }
}

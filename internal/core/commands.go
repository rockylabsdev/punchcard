package core

import (
	"time"

	"github.com/j-dsmith/punchcard/internal/tracker"
)

// Command is one requested transition. The concrete types in this file
// are the whole set; Apply refuses anything else.
type Command interface{ command() }

// Start starts the timer on Key, closing any other running segment first
// (with an EventPaused). If Key is not tracked it is added with Summary
// and Unverified (EventAdded); if it was removed it is restored
// (EventRestored). If Key has an open (unstopped) entry that
// entry gets a new segment; otherwise a new entry with NewEntryID is
// created. Start on the running ticket is a no-op, except that by hand
// it sets the timer by hand (EventByHand). Another origin may not move
// or pause a timer set by hand (ErrByHand); only focus skips this (Touch).
//
// At, when set, is the time the start happened instead of now: the
// segment opens then, and the ticket it pauses is paused then too
// (EventPaused and EventStarted carry At). It may not be in the future,
// before the running ticket's open segment started, or before other
// time already measured ended (ErrUsage), and it refuses the running
// ticket's own key.
type Start struct {
	Key        string
	At         time.Time
	Summary    string
	Unverified bool
	Origin     Origin
	NewEntryID string // used only when a new entry is created
	// Grammar is the key grammar in effect (the tracker's); the zero
	// value is the slug, for no tracker.
	Grammar tracker.Grammar
}

// Pause closes the running segment. The entry stays open.
//
// At, when set, closes it at that time instead of now and marks it Cut:
// the user was away from then. With nothing running it moves the pause
// of Key's paused entry, or with no Key the most recently paused one. At
// may not be in the future, before the segment started, or over other
// time already measured (ErrUsage). A cut that would leave a segment
// under a minute drops it, and its EventPaused says so in Message. Key
// is read only with At.
type Pause struct {
	Key    string
	At     time.Time
	Origin Origin
}

// Resume with a Key is Start with that Key. With no Key it resumes the
// most recently paused entry, and is a no-op while a timer runs.
//
// At, when set, opens the segment at that time instead of now. It may
// not be in the future, before the pause, or before other time already
// measured ended; with no Key it refuses while a timer runs (ErrUsage).
type Resume struct {
	Key        string
	At         time.Time
	Origin     Origin
	NewEntryID string
	// Grammar is the key grammar in effect (the tracker's); the zero
	// value is the slug, for no tracker.
	Grammar tracker.Grammar
}

// Stop ends an entry and produces its Worklog: pending, or local when
// Local says no tracker is in effect. With no Key it
// targets the running entry, else the single paused one (several paused:
// *AmbiguousError). Duration zero means "use the measured elapsed";
// either way it is truncated to the minute and refused under one minute
// (ErrTooShort). Stop clears Recovered. It does not gate on origin: the
// CLI owns needs_confirmation for agents. EntryID, when set, pins the
// stop to the entry the caller previewed: if the target is another entry
// (that one was stopped meanwhile), Stop refuses with ErrNoTimer.
type Stop struct {
	Key      string
	EntryID  string
	Duration time.Duration
	Comment  string
	Origin   Origin
	// Local: no tracker is in effect, so the worklog is local (final)
	// rather than pending. Core never reads config; the caller decides.
	Local bool
}

// NewTicket is a ticket to put on the list, with what the tracker said
// of it.
type NewTicket struct {
	Key        string
	Summary    string
	Unverified bool
}

// Add puts tickets on the list. Keys already on it are left alone; a
// removed ticket is restored with its history (EventRestored).
type Add struct {
	Tickets []NewTicket
	Origin  Origin
	// Grammar is the key grammar in effect (the tracker's); the zero
	// value is the slug, for no tracker.
	Grammar tracker.Grammar
}

// Remove takes a ticket off the list: it sets RemovedAt and keeps its
// logged worklogs, which stay in the log. It refuses (ErrUnlogged) while
// the ticket has a running or paused entry or a worklog not yet synced,
// unless Force, which discards that unlogged time; a logged worklog is
// never discarded. A ticket already removed is ErrNotTracked.
type Remove struct {
	Key    string
	Force  bool
	Origin Origin
}

// Touch is the mod's presence call. With no Key it is presence only.
// With a Key it is also focus: a timer on another ticket moves to Key
// (EventPaused, EventStarted), but only when Key differs from
// State.LastFocus, which it then records (ADR 0009). It moves a timer
// set by hand too (ADR 0010). A prompt or hand Touch is always presence,
// whatever it moves. Focus never starts a timer when nothing runs. An
// agent's Touch does nothing.
type Touch struct {
	Key        string
	Summary    string // used only if Key is not tracked yet
	Unverified bool
	Origin     Origin
	NewEntryID string
	// Grammar is the key grammar in effect (the tracker's); the zero
	// value is the slug, for no tracker.
	Grammar tracker.Grammar
}

// MarkSynced records that the tracker accepted an entry's worklog. It refuses a
// worklog already synced (ErrAlreadySynced) or local (ErrLocal) and
// clears AuthFailedAt. Since the tracker accepted the key, it also clears
// the ticket's Unverified; the summary is left as it is, for a Verify.
type MarkSynced struct {
	Key       string
	EntryID   string
	WorklogID string
}

// MarkFailed records a failed POST; the worklog stays retryable. It
// never downgrades a synced worklog (ErrAlreadySynced) and never marks a
// local one (ErrLocal).
type MarkFailed struct {
	Key     string
	EntryID string
	Message string
	// Status is the tracker's refusal status, when it refused (0: no
	// answer at all).
	Status int
}

// MarkLocal records that a pending or failed worklog is logged with no
// tracker: the user removed the tracker and chose not to owe it. It
// clears the failure and keeps the time and comment. It refuses a synced
// worklog (ErrAlreadySynced) and one already local (ErrLocal). Only the
// setup wizard's removal offer applies it; it has no CLI surface.
type MarkLocal struct {
	Key     string
	EntryID string
}

// RecordAuthFailure records a Jira 401 at now.
type RecordAuthFailure struct{}

// ClearAuthFailure records that an authenticated Jira call succeeded:
// it clears AuthFailedAt. MarkSynced and Verify clear it too; a fresh
// setup applies this.
type ClearAuthFailure struct{}

// Verify records what Jira said of a ticket on the list: its summary, and
// that it exists (clearing Unverified), and, since Jira answered, clears
// AuthFailedAt. A key not on the list (untracked or removed) is
// ErrNotTracked.
// Start, Add and Touch carrying a confirmed NewTicket (Unverified false)
// confirm a tracked unverified ticket the same way, in one transition;
// MarkSynced confirms it too, but leaves the summary for a Verify.
type Verify struct {
	Key     string
	Summary string
}

func (Start) command()             {}
func (Pause) command()             {}
func (Resume) command()            {}
func (Stop) command()              {}
func (Add) command()               {}
func (Remove) command()            {}
func (Touch) command()             {}
func (MarkSynced) command()        {}
func (MarkFailed) command()        {}
func (MarkLocal) command()         {}
func (RecordAuthFailure) command() {}
func (ClearAuthFailure) command()  {}
func (Verify) command()            {}

// EventKind names a side effect nobody asked for.
type EventKind string

const (
	EventAdded EventKind = "added" // start or focus added an untracked ticket
	// EventRestored: start, focus or add put a removed ticket back on the
	// list, its history intact.
	EventRestored EventKind = "restored"
	// EventPaused: another ticket's start paused this one, or a pause at
	// a named time (At) closed it.
	EventPaused EventKind = "paused"
	EventByHand EventKind = "byHand" // a start by hand made the running timer one an agent can't move
	// EventStarted: focus moved the timer to this ticket, or a start or
	// resume at a named time (At) opened it.
	EventStarted EventKind = "started"
	// EventRecovered: a stale timer was capped; At is where it was capped.
	EventRecovered  EventKind = "recovered"
	EventSynced     EventKind = "synced"
	EventSyncFailed EventKind = "syncFailed" // Message says why
)

// Event is a side effect of a command, reported so nothing happens behind
// the user's back. Fields beyond Kind and Key are set per kind.
type Event struct {
	Kind    EventKind `json:"kind"`
	Key     string    `json:"key"`
	At      Timestamp `json:"at,omitzero"`
	Message string    `json:"message,omitempty"`
}

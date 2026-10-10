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
// and Unverified (EventAdded). If Key has an open (unstopped) entry that
// entry gets a new segment; otherwise a new entry with NewEntryID is
// created. Start on the running ticket is a no-op, except that by hand
// it holds an un-held timer (EventHeld). Only OriginHand may move a held
// timer (ErrHeld).
type Start struct {
	Key        string
	Summary    string
	Unverified bool
	Origin     Origin
	NewEntryID string // used only when a new entry is created
	// Grammar is the key grammar in effect (the tracker's); the zero
	// value is the slug, for no tracker.
	Grammar tracker.Grammar
}

// Pause closes the running segment. The entry stays open.
type Pause struct {
	Origin Origin
}

// Resume with a Key is Start with that Key. With no Key it resumes the
// most recently paused entry, and is a no-op while a timer runs.
type Resume struct {
	Key        string
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

// Add puts tickets on the list. Keys already tracked are left alone.
type Add struct {
	Tickets []NewTicket
	Origin  Origin
	// Grammar is the key grammar in effect (the tracker's); the zero
	// value is the slug, for no tracker.
	Grammar tracker.Grammar
}

// Remove takes a ticket off the list. It refuses (ErrUnlogged) while the
// ticket has a running or paused entry or a worklog not yet synced,
// unless Force.
type Remove struct {
	Key    string
	Force  bool
	Origin Origin
}

// Touch is the mod's presence call. With no Key it is presence only.
// With a Key it is also focus: an un-held timer on another ticket moves
// to Key (EventPaused, EventStarted), but only when Key differs from
// State.LastFocus, which it then records (ADR 0009); a held timer is
// never moved. A prompt or hand Touch is always presence, whatever it
// moves. Focus never starts a timer when nothing runs. An agent's Touch
// does nothing.
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
// clears AuthFailedAt.
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

// Verify records what Jira said of a tracked ticket: its summary, and
// that it exists (clearing Unverified), and, since Jira answered, clears
// AuthFailedAt. An untracked key is ErrNotTracked.
// Start, Add and Touch carrying a confirmed NewTicket (Unverified false)
// confirm a tracked unverified ticket the same way, in one transition.
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
	EventAdded   EventKind = "added"   // start or focus added an untracked ticket
	EventPaused  EventKind = "paused"  // another ticket's start paused this one
	EventHeld    EventKind = "held"    // a start by hand held the running timer
	EventStarted EventKind = "started" // focus moved the timer to this ticket
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

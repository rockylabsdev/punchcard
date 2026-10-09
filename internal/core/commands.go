package core

import "time"

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
}

// Stop ends an entry and produces its pending Worklog. With no Key it
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
}

// NewTicket is a ticket to put on the list, with what Jira said of it.
type NewTicket struct {
	Key        string
	Summary    string
	Unverified bool
}

// Add puts tickets on the list. Keys already tracked are left alone.
type Add struct {
	Tickets []NewTicket
	Origin  Origin
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
// With a Key it is focus: the running ticket is vouched for; an un-held
// timer on another ticket moves to Key (EventPaused, EventStarted); a
// held timer is neither moved nor vouched for. Focus never starts a
// timer when nothing runs. An agent's Touch does nothing.
type Touch struct {
	Key        string
	Summary    string // used only if Key is not tracked yet
	Unverified bool
	Origin     Origin
	NewEntryID string
}

// MarkSynced records that Jira accepted an entry's worklog. It refuses a
// worklog already synced (ErrAlreadySynced) and clears AuthFailedAt.
type MarkSynced struct {
	Key       string
	EntryID   string
	WorklogID string
}

// MarkFailed records a failed POST; the worklog stays retryable. It
// never downgrades a synced worklog (ErrAlreadySynced).
type MarkFailed struct {
	Key     string
	EntryID string
	Message string
}

// RecordAuthFailure records a Jira 401 at now.
type RecordAuthFailure struct{}

// Verify records what Jira said of a tracked ticket: its summary, and
// that it exists (clearing Unverified). An untracked key is ErrNotTracked.
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
func (RecordAuthFailure) command() {}
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

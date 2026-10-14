package core

import (
	"fmt"
	"time"
)

// SchemaVersion is the version of state.json and heartbeat.json this
// build writes. A file with a higher version is refused, never reset.
const SchemaVersion = 1

// Origin is who caused a change. Only OriginHand sets a timer by hand;
// hand and prompt are presence, agent is not.
type Origin string

const (
	OriginHand   Origin = "hand"
	OriginPrompt Origin = "prompt"
	OriginAgent  Origin = "agent"
)

// IsPresence reports whether a change with this origin is evidence the
// user is at the machine.
func (o Origin) IsPresence() bool { return o == OriginHand || o == OriginPrompt }

// State is the whole of state.json.
type State struct {
	SchemaVersion int `json:"schemaVersion"`
	// AuthFailedAt is set by a Jira 401 and cleared by a successful call.
	AuthFailedAt Timestamp `json:"authFailedAt,omitzero"`
	// LastFocus is the key of the last presence Touch that carried one;
	// focus moves a timer only when it changes (ADR 0009). Empty, as in
	// a file written before it existed, differs from every key.
	LastFocus      string          `json:"lastFocus,omitempty"`
	TrackedTickets []TrackedTicket `json:"trackedTickets"`
}

// NewState is the state of a first run: no tickets, current schema.
func NewState() State {
	return State{SchemaVersion: SchemaVersion, TrackedTickets: []TrackedTicket{}}
}

// TrackedTicket is a ticket the user has tracked, whether or not it has
// time against it. Key is its identity. It is on the list unless it was
// removed; a removed ticket keeps its entries, so its logged worklogs
// stay in the log.
type TrackedTicket struct {
	Key     string `json:"key"`
	Summary string `json:"summary,omitempty"`
	// Unverified: added while Jira could not be reached; no summary yet.
	Unverified bool      `json:"unverified"`
	AddedAt    Timestamp `json:"addedAt"`
	// RemovedAt is when rm took the ticket off the list; zero (as in a
	// file written before it existed) while it is on the list.
	RemovedAt Timestamp   `json:"removedAt,omitzero"`
	Entries   []TimeEntry `json:"entries"`
}

// Listed reports whether t is on the list: not removed.
func (t TrackedTicket) Listed() bool { return t.RemovedAt.IsZero() }

// TimeEntry is one start→stop period. Pausing closes a segment but not
// the entry; one entry produces exactly one Worklog.
type TimeEntry struct {
	// ID is a UUID and the idempotency key for the worklog POST.
	ID       string    `json:"id"`
	Segments []Segment `json:"segments"`
	// ByHand describes the entry's current (or last) running stretch; only
	// OriginHand sets it. Origin is who made the entry's last change:
	// opened its stretch, paused it or stopped it (recovery keeps it).
	ByHand bool   `json:"byHand"`
	Origin Origin `json:"origin"`
	// Recovered is set by recovery and cleared only by stop.
	Recovered bool `json:"recovered"`
	// Worklog is nil until the entry is stopped.
	Worklog *Worklog `json:"worklog,omitempty"`
}

// Running reports whether the entry is accruing: its final segment is open.
func (e TimeEntry) Running() bool {
	return e.Worklog == nil && len(e.Segments) > 0 && e.Segments[len(e.Segments)-1].Open()
}

// Paused reports whether the entry is suspended: not stopped, final
// segment closed.
func (e TimeEntry) Paused() bool {
	return e.Worklog == nil && len(e.Segments) > 0 && !e.Segments[len(e.Segments)-1].Open()
}

// Stopped reports whether the entry has produced its worklog.
func (e TimeEntry) Stopped() bool { return e.Worklog != nil }

// Segment is one uninterrupted stretch of accrual. A zero EndedAt means
// the segment is open (running); it is omitted from JSON.
type Segment struct {
	StartedAt Timestamp `json:"startedAt"`
	EndedAt   Timestamp `json:"endedAt,omitzero"`
}

// Open reports whether the segment is still running.
func (s Segment) Open() bool { return s.EndedAt.IsZero() }

// WorklogState is the sync state of a Worklog.
type WorklogState string

const (
	WorklogPending WorklogState = "pending"
	WorklogSynced  WorklogState = "synced"
	WorklogFailed  WorklogState = "failed"
	// WorklogLocal: recorded with no tracker in effect. It is final, as a
	// synced worklog is: nothing is owed and it is never sent.
	WorklogLocal WorklogState = "local"
)

// Worklog is the record of a stopped entry: what the tracker is (or will
// be) told, or, with no tracker, the local record of the time.
// It is a flat tagged shape: WorklogID and SyncedAt only when synced,
// Message, RefusedStatus and FailedAt only when failed.
type Worklog struct {
	State WorklogState `json:"state"`
	// StartedAt is the entry's first segment start: the worklog's day.
	StartedAt        Timestamp `json:"startedAt"`
	TimeSpentSeconds int       `json:"timeSpentSeconds"`
	Comment          string    `json:"comment,omitempty"`
	WorklogID        string    `json:"worklogId,omitempty"`
	SyncedAt         Timestamp `json:"syncedAt,omitzero"`
	Message          string    `json:"message,omitempty"`
	// RefusedStatus is the tracker's status when it answered with a
	// refusal; 0 when it could not be reached (or for any other failure).
	RefusedStatus int       `json:"refusedStatus,omitempty"`
	FailedAt      Timestamp `json:"failedAt,omitzero"`
	// Recovered: the entry was recovered when it stopped, so its time was
	// a correction rather than a measurement. The entry's own flag clears
	// at stop; this keeps the fact for log. Never sent to the tracker.
	Recovered bool `json:"recovered,omitempty"`
}

// Logged reports whether the worklog is done with: synced to the tracker,
// or local because there is none. A logged worklog is never owed or sent.
func (w Worklog) Logged() bool { return w.State == WorklogSynced || w.State == WorklogLocal }

// Heartbeat is the whole of heartbeat.json: the last moment of presence.
type Heartbeat struct {
	SchemaVersion int       `json:"schemaVersion"`
	LastSeen      Timestamp `json:"lastSeen,omitzero"`
}

// World is everything Apply reads: both files, read fresh under the lock.
type World struct {
	State     State
	Heartbeat Heartbeat
}

// TimeLayout is the Jira formatter used for every timestamp on disk:
// milliseconds and a numeric offset without a colon.
const TimeLayout = "2006-01-02T15:04:05.000-0700"

// Timestamp is a time.Time that marshals with TimeLayout. The zero value
// means "absent" and is omitted from JSON via omitzero.
type Timestamp struct{ time.Time }

// At wraps t, truncated to the millisecond so it survives a round-trip.
func At(t time.Time) Timestamp { return Timestamp{t.Truncate(time.Millisecond)} }

func (t Timestamp) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	return []byte(`"` + t.Format(TimeLayout) + `"`), nil
}

func (t *Timestamp) UnmarshalJSON(b []byte) error {
	s := string(b)
	if s == "null" {
		*t = Timestamp{}
		return nil
	}
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return fmt.Errorf("timestamp: not a string: %s", s)
	}
	s = s[1 : len(s)-1]
	for _, layout := range []string{TimeLayout, time.RFC3339Nano} {
		if p, err := time.Parse(layout, s); err == nil {
			*t = Timestamp{p}
			return nil
		}
	}
	return fmt.Errorf("timestamp: %q is not %s", s, TimeLayout)
}

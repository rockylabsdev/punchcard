package core_test

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/j-dsmith/punchcard/internal/core"
)

// zone is a fixed UTC-5 offset so tests never depend on the machine's TZ.
var zone = time.FixedZone("UTC-5", -5*60*60)

// at returns 2026-08-09 at the given clock time ("15:04" or "15:04:05") in zone.
func at(clock string) core.Timestamp { return day(9, clock) }

// day returns 2026-08-<d> at the given clock time in zone.
func day(d int, clock string) core.Timestamp {
	t, err := time.ParseInLocation("15:04:05", clock, zone)
	if err != nil {
		t, err = time.ParseInLocation("15:04", clock, zone)
		if err != nil {
			panic(err)
		}
	}
	return core.Timestamp{Time: time.Date(2026, 8, d, t.Hour(), t.Minute(), t.Second(), 0, zone)}
}

// seg is a segment on 2026-08-09; end "" means open.
func seg(start, end string) core.Segment {
	s := core.Segment{StartedAt: at(start)}
	if end != "" {
		s.EndedAt = at(end)
	}
	return s
}

func ticket(key string, entries ...core.TimeEntry) core.TrackedTicket {
	if entries == nil {
		entries = []core.TimeEntry{}
	}
	return core.TrackedTicket{Key: key, AddedAt: at("08:00"), Entries: entries}
}

func entry(id string, origin core.Origin, segs ...core.Segment) core.TimeEntry {
	return core.TimeEntry{ID: id, Segments: segs, Held: origin == core.OriginHand, Origin: origin}
}

func recovered(e core.TimeEntry) core.TimeEntry { e.Recovered = true; return e }

func withWorklog(e core.TimeEntry, wl core.Worklog) core.TimeEntry { e.Worklog = &wl; return e }

func state(tickets ...core.TrackedTicket) core.State {
	s := core.NewState()
	s.TrackedTickets = append(s.TrackedTickets, tickets...)
	return s
}

// world builds a World whose lastSeen is the given clock time ("" = none).
func world(lastSeen string, tickets ...core.TrackedTicket) core.World {
	w := core.World{State: state(tickets...), Heartbeat: core.Heartbeat{SchemaVersion: 1}}
	if lastSeen != "" {
		w.Heartbeat.LastSeen = at(lastSeen)
	}
	return w
}

func diff(t *testing.T, label string, want, got any) {
	t.Helper()
	if d := cmp.Diff(want, got); d != "" {
		t.Errorf("%s mismatch (-want +got):\n%s", label, d)
	}
}

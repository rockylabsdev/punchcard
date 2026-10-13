package core_test

import (
	"errors"
	"testing"

	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/tracker"
)

// strict is a tracker's grammar, Jira's shape: ABC-123.
var strict = tracker.NewGrammar(`^[A-Z][A-Z0-9]+-[0-9]+$`, true, "PUN-123")

const (
	hand   = core.OriginHand
	prompt = core.OriginPrompt
	agent  = core.OriginAgent
)

func TestStartOnEmptyStateTracksTicketAndOpensSegment(t *testing.T) {
	w := core.World{State: core.NewState()}
	got, events, err := core.Apply(w, core.Start{Key: "PUNCH-12", Summary: "Wire up", Origin: hand, NewEntryID: "E1"}, at("09:00").Time)
	if err != nil {
		t.Fatal(err)
	}
	want := core.State{
		SchemaVersion: 1,
		TrackedTickets: []core.TrackedTicket{{
			Key: "PUNCH-12", Summary: "Wire up", AddedAt: at("09:00"),
			Entries: []core.TimeEntry{{
				ID: "E1", Segments: []core.Segment{seg("09:00", "")},
				ByHand: true, Origin: hand,
			}},
		}},
	}
	diff(t, "state", want, got.State)
	diff(t, "events", []core.Event{{Kind: core.EventAdded, Key: "PUNCH-12"}}, events)
}

type applyCase struct {
	name   string
	w      core.World
	cmd    core.Command
	now    string // clock time on 2026-08-09
	want   core.State
	events []core.Event
	err    error // checked with errors.Is; state is not compared on error
}

func runApply(t *testing.T, cases []applyCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.w.State
			got, events, err := core.Apply(tc.w, tc.cmd, at(tc.now).Time)
			if tc.err != nil {
				if !errors.Is(err, tc.err) {
					t.Fatalf("err = %v, want %v", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			diff(t, "state", tc.want, got.State)
			diff(t, "events", tc.events, events)
			diff(t, "input state (Apply must not mutate it)", before, tc.w.State)
		})
	}
}

func TestApplyStart(t *testing.T) {
	runApply(t, []applyCase{
		{
			name: "start B while A runs pauses A and says so",
			w:    world("09:30", ticket("AA-1", entry("EA", hand, seg("09:00", ""))), ticket("BB-2")),
			cmd:  core.Start{Key: "BB-2", Origin: hand, NewEntryID: "EB"},
			now:  "09:30",
			want: state(
				ticket("AA-1", entry("EA", hand, seg("09:00", "09:30"))),
				ticket("BB-2", entry("EB", hand, seg("09:30", ""))),
			),
			events: []core.Event{{Kind: core.EventPaused, Key: "AA-1"}},
		},
		{
			name: "start A while A runs is a no-op for an agent",
			w:    world("09:30", ticket("AA-1", entry("EA", agent, seg("09:00", "")))),
			cmd:  core.Start{Key: "AA-1", Origin: agent, NewEntryID: "X"},
			now:  "09:30",
			want: state(ticket("AA-1", entry("EA", agent, seg("09:00", "")))),
		},
		{
			name:   "start A by hand while A runs sets it by hand",
			w:      world("09:30", ticket("AA-1", entry("EA", agent, seg("09:00", "")))),
			cmd:    core.Start{Key: "AA-1", Origin: hand, NewEntryID: "X"},
			now:    "09:30",
			want:   state(ticket("AA-1", entry("EA", hand, seg("09:00", "")))),
			events: []core.Event{{Kind: core.EventByHand, Key: "AA-1"}},
		},
		{
			name: "start on a paused ticket continues the same entry",
			w:    world("10:00", ticket("AA-1", entry("EA", hand, seg("09:00", "09:30")))),
			cmd:  core.Start{Key: "AA-1", Origin: agent, NewEntryID: "X"},
			now:  "10:00",
			want: state(ticket("AA-1", entry("EA", agent, seg("09:00", "09:30"), seg("10:00", "")))),
		},
		{
			name: "start on a ticket whose entries are stopped opens a new entry",
			w: world("10:00", ticket("AA-1",
				withWorklog(entry("EA", hand, seg("09:00", "09:30")), core.Worklog{State: core.WorklogPending, StartedAt: at("09:00"), TimeSpentSeconds: 1800}))),
			cmd: core.Start{Key: "AA-1", Origin: hand, NewEntryID: "EB"},
			now: "10:00",
			want: state(ticket("AA-1",
				withWorklog(entry("EA", hand, seg("09:00", "09:30")), core.Worklog{State: core.WorklogPending, StartedAt: at("09:00"), TimeSpentSeconds: 1800}),
				entry("EB", hand, seg("10:00", "")))),
		},
		{
			name: "a new entry needs an id",
			w:    world("10:00", ticket("AA-1")),
			cmd:  core.Start{Key: "AA-1", Origin: hand},
			now:  "10:00",
			err:  core.ErrUsage,
		},
		{
			name: "a malformed key is refused",
			w:    world("10:00"),
			cmd:  core.Start{Key: "abc", Origin: hand, NewEntryID: "E", Grammar: strict},
			now:  "10:00",
			err:  core.ErrUsage,
		},
		{
			name: "with no tracker a key is a slug, kept as typed",
			w:    world("10:00"),
			cmd:  core.Start{Key: "payroll", Origin: hand, NewEntryID: "E"},
			now:  "10:00",
			want: state(core.TrackedTicket{Key: "payroll", AddedAt: at("10:00"),
				Entries: []core.TimeEntry{entry("E", hand, seg("10:00", ""))}}),
			events: []core.Event{{Kind: core.EventAdded, Key: "payroll"}},
		},
		{
			name: "a slug has no spaces",
			w:    world("10:00"),
			cmd:  core.Start{Key: "pay roll", Origin: hand, NewEntryID: "E"},
			now:  "10:00",
			err:  core.ErrUsage,
		},
		{
			name: "resume KEY keeps the grammar",
			w:    world("10:00"),
			cmd:  core.Resume{Key: "payroll", Origin: hand, NewEntryID: "E", Grammar: strict},
			now:  "10:00",
			err:  core.ErrUsage,
		},
	})
}

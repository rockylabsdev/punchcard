package core_test

import (
	"testing"

	"github.com/j-dsmith/punchcard/internal/core"
)

func TestApplyAddRemove(t *testing.T) {
	stopped := func(state core.WorklogState) core.TimeEntry {
		return withWorklog(entry("EA", hand, seg("09:00", "09:30")), core.Worklog{State: state, StartedAt: at("09:00"), TimeSpentSeconds: 1800})
	}
	added := func(key, summary string, unverified bool) core.TrackedTicket {
		t := ticket(key)
		t.Summary, t.Unverified, t.AddedAt = summary, unverified, at("10:00")
		return t
	}
	runApply(t, []applyCase{
		{
			name: "add tracks tickets with no time against them",
			w:    world("10:00", ticket("AA-1")),
			cmd:  core.Add{Tickets: []core.NewTicket{{Key: "BB-2", Summary: "Fix it"}, {Key: "CC-3", Unverified: true}, {Key: "AA-1", Summary: "ignored"}}},
			now:  "10:00",
			want: state(ticket("AA-1"), added("BB-2", "Fix it", false), added("CC-3", "", true)),
		},
		{
			name: "add refuses a malformed key and adds nothing",
			w:    world("10:00"),
			cmd:  core.Add{Tickets: []core.NewTicket{{Key: "BB-2"}, {Key: "nope"}}, Grammar: strict},
			now:  "10:00",
			err:  core.ErrUsage,
		},
		{
			name: "remove drops a ticket with nothing owed",
			w:    world("10:00", ticket("AA-1", stopped(core.WorklogSynced)), ticket("BB-2")),
			cmd:  core.Remove{Key: "AA-1"},
			now:  "10:00",
			want: state(ticket("BB-2")),
		},
		{
			name: "remove refuses a running entry",
			w:    world("10:00", ticket("AA-1", entry("EA", hand, seg("09:00", "")))),
			cmd:  core.Remove{Key: "AA-1"}, now: "10:00", err: core.ErrUnlogged,
		},
		{
			name: "remove refuses a paused entry",
			w:    world("10:00", ticket("AA-1", entry("EA", hand, seg("09:00", "09:10")))),
			cmd:  core.Remove{Key: "AA-1"}, now: "10:00", err: core.ErrUnlogged,
		},
		{
			name: "remove refuses an unsent worklog",
			w:    world("10:00", ticket("AA-1", stopped(core.WorklogFailed))),
			cmd:  core.Remove{Key: "AA-1"}, now: "10:00", err: core.ErrUnlogged,
		},
		{
			name: "remove --force overrides",
			w:    world("10:00", ticket("AA-1", stopped(core.WorklogPending))),
			cmd:  core.Remove{Key: "AA-1", Force: true},
			now:  "10:00",
			want: state(),
		},
		{
			name: "remove of an untracked key is refused",
			w:    world("10:00"),
			cmd:  core.Remove{Key: "AA-1"}, now: "10:00", err: core.ErrNotTracked,
		},
	})
}

func TestApplyTouch(t *testing.T) {
	unheldA := func() core.World {
		return world("09:00", ticket("AA-1", entry("EA", agent, seg("09:00", ""))), ticket("BB-2"))
	}
	heldA := func() core.World {
		return world("09:00", ticket("AA-1", entry("EA", hand, seg("09:00", ""))), ticket("BB-2"))
	}
	cases := []struct {
		name     string
		w        core.World
		cmd      core.Touch
		want     core.State
		events   []core.Event
		lastSeen core.Timestamp
	}{
		{
			name:     "touch with no key is presence only",
			w:        heldA(),
			cmd:      core.Touch{Origin: prompt},
			want:     heldA().State,
			lastSeen: at("09:30"),
		},
		{
			name:     "touch of the running ticket vouches for it",
			w:        unheldA(),
			cmd:      core.Touch{Key: "AA-1", Origin: prompt},
			want:     unheldA().State,
			lastSeen: at("09:30"),
		},
		{
			name: "focus moves an un-held timer",
			w:    unheldA(),
			cmd:  core.Touch{Key: "BB-2", Origin: prompt, NewEntryID: "EB"},
			want: state(
				ticket("AA-1", entry("EA", prompt, seg("09:00", "09:30"))),
				ticket("BB-2", entry("EB", prompt, seg("09:30", "")))),
			events:   []core.Event{{Kind: core.EventPaused, Key: "AA-1"}, {Kind: core.EventStarted, Key: "BB-2"}},
			lastSeen: at("09:30"),
		},
		{
			// The user's own prompt is presence in any session (CONTEXT.md),
			// even one on another ticket; only the move is refused.
			name:     "focus never moves a held timer, but the prompt is still presence",
			w:        heldA(),
			cmd:      core.Touch{Key: "BB-2", Origin: prompt, NewEntryID: "EB"},
			want:     heldA().State,
			lastSeen: at("09:30"),
		},
		{
			name:     "an agent's touch on another ticket while held is not presence",
			w:        heldA(),
			cmd:      core.Touch{Key: "BB-2", Origin: agent, NewEntryID: "EB"},
			want:     heldA().State,
			lastSeen: at("09:00"),
		},
		{
			name:     "focus does not start a timer when nothing runs",
			w:        world("09:00", ticket("AA-1", entry("EA", hand, seg("08:00", "08:30")))),
			cmd:      core.Touch{Key: "AA-1", Origin: prompt, NewEntryID: "X"},
			want:     state(ticket("AA-1", entry("EA", hand, seg("08:00", "08:30")))),
			lastSeen: at("09:30"),
		},
		{
			name:     "an agent's touch is not presence",
			w:        unheldA(),
			cmd:      core.Touch{Key: "BB-2", Origin: agent, NewEntryID: "EB"},
			want:     unheldA().State,
			lastSeen: at("09:00"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, events, err := core.Apply(tc.w, tc.cmd, at("09:30").Time)
			if err != nil {
				t.Fatal(err)
			}
			diff(t, "state", tc.want, got.State)
			diff(t, "events", tc.events, events)
			diff(t, "lastSeen", tc.lastSeen, got.Heartbeat.LastSeen)
		})
	}
}

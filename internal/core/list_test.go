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
			name: "remove hides a ticket and keeps its synced worklog",
			w:    world("10:00", ticket("AA-1", stopped(core.WorklogSynced)), ticket("BB-2")),
			cmd:  core.Remove{Key: "AA-1"},
			now:  "10:00",
			want: state(removed(ticket("AA-1", stopped(core.WorklogSynced)), "10:00"), ticket("BB-2")),
		},
		{
			name: "remove hides a ticket and keeps its local worklog",
			w:    world("10:00", ticket("AA-1", stopped(core.WorklogLocal)), ticket("BB-2")),
			cmd:  core.Remove{Key: "AA-1"},
			now:  "10:00",
			want: state(removed(ticket("AA-1", stopped(core.WorklogLocal)), "10:00"), ticket("BB-2")),
		},
		{
			name: "remove hides a ticket with no time against it",
			w:    world("10:00", ticket("AA-1")),
			cmd:  core.Remove{Key: "AA-1"},
			now:  "10:00",
			want: state(removed(ticket("AA-1"), "10:00")),
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
			name: "remove --force discards the unsent worklog",
			w:    world("10:00", ticket("AA-1", stopped(core.WorklogPending))),
			cmd:  core.Remove{Key: "AA-1", Force: true},
			now:  "10:00",
			want: state(removed(ticket("AA-1"), "10:00")),
		},
		{
			name: "remove --force discards the open entry and keeps the logged worklog",
			w: world("10:00", ticket("AA-1",
				withWorklog(entry("E1", hand, seg("08:00", "08:30")), core.Worklog{State: core.WorklogLocal, StartedAt: at("08:00"), TimeSpentSeconds: 1800}),
				entry("E2", hand, seg("09:00", "09:10")))),
			cmd: core.Remove{Key: "AA-1", Force: true},
			now: "10:00",
			want: state(removed(ticket("AA-1",
				withWorklog(entry("E1", hand, seg("08:00", "08:30")), core.Worklog{State: core.WorklogLocal, StartedAt: at("08:00"), TimeSpentSeconds: 1800})), "10:00")),
		},
		{
			name: "remove of an untracked key is refused",
			w:    world("10:00"),
			cmd:  core.Remove{Key: "AA-1"}, now: "10:00", err: core.ErrNotTracked,
		},
		{
			name: "remove of a removed ticket is refused",
			w:    world("10:00", removed(ticket("AA-1"), "09:00")),
			cmd:  core.Remove{Key: "AA-1"}, now: "10:00", err: core.ErrNotTracked,
		},
		{
			name:   "add restores a removed ticket, its addedAt and history intact",
			w:      world("10:00", removed(ticket("AA-1", stopped(core.WorklogLocal)), "09:45"), ticket("BB-2")),
			cmd:    core.Add{Tickets: []core.NewTicket{{Key: "AA-1"}, {Key: "BB-2"}}},
			now:    "10:00",
			want:   state(ticket("AA-1", stopped(core.WorklogLocal)), ticket("BB-2")),
			events: []core.Event{{Kind: core.EventRestored, Key: "AA-1"}},
		},
		{
			name:   "add confirms an unverified removed ticket as it restores it",
			w:      world("10:00", removed(unverified(ticket("AA-1")), "09:45")),
			cmd:    core.Add{Tickets: []core.NewTicket{{Key: "AA-1", Summary: "Fix it"}}},
			now:    "10:00",
			want:   state(summarized(ticket("AA-1"), "Fix it")),
			events: []core.Event{{Kind: core.EventRestored, Key: "AA-1"}},
		},
		{
			name:   "start restores a removed ticket and opens a new entry",
			w:      world("10:00", removed(ticket("AA-1", stopped(core.WorklogSynced)), "09:45")),
			cmd:    core.Start{Key: "AA-1", Origin: hand, NewEntryID: "E2"},
			now:    "10:00",
			want:   state(ticket("AA-1", stopped(core.WorklogSynced), entry("E2", hand, seg("10:00", "")))),
			events: []core.Event{{Kind: core.EventRestored, Key: "AA-1"}},
		},
		{
			name:   "resume KEY restores a removed ticket",
			w:      world("10:00", removed(ticket("AA-1"), "09:45")),
			cmd:    core.Resume{Key: "AA-1", Origin: hand, NewEntryID: "E1"},
			now:    "10:00",
			want:   state(ticket("AA-1", entry("E1", hand, seg("10:00", "")))),
			events: []core.Event{{Kind: core.EventRestored, Key: "AA-1"}},
		},
		{
			name: "focus on a removed ticket restores it as a start does",
			w:    world("10:00", ticket("BB-2", entry("EB", agent, seg("09:00", ""))), removed(ticket("AA-1"), "09:45")),
			cmd:  core.Touch{Key: "AA-1", Origin: prompt, NewEntryID: "E1"},
			now:  "10:00",
			want: focused(state(ticket("BB-2", entry("EB", prompt, seg("09:00", "10:00"))),
				ticket("AA-1", entry("E1", prompt, seg("10:00", "")))), "AA-1"),
			events: []core.Event{{Kind: core.EventPaused, Key: "BB-2"}, {Kind: core.EventRestored, Key: "AA-1"}, {Kind: core.EventStarted, Key: "AA-1"}},
		},
		{
			name: "stop names a removed ticket as not tracked",
			w:    world("10:00", removed(ticket("AA-1"), "09:45")),
			cmd:  core.Stop{Key: "AA-1"}, now: "10:00", err: core.ErrNotTracked,
		},
		{
			name: "verify of a removed ticket is refused",
			w:    world("10:00", removed(unverified(ticket("AA-1")), "09:45")),
			cmd:  core.Verify{Key: "AA-1", Summary: "Fix it"}, now: "10:00", err: core.ErrNotTracked,
		},
	})
}

func TestApplyTouch(t *testing.T) {
	notByHandA := func() core.World {
		return world("09:00", ticket("AA-1", entry("EA", agent, seg("09:00", ""))), ticket("BB-2"))
	}
	byHandA := func() core.World {
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
			w:        byHandA(),
			cmd:      core.Touch{Origin: prompt},
			want:     byHandA().State,
			lastSeen: at("09:30"),
		},
		{
			name:     "touch of the running ticket vouches for it",
			w:        notByHandA(),
			cmd:      core.Touch{Key: "AA-1", Origin: prompt},
			want:     focused(notByHandA().State, "AA-1"),
			lastSeen: at("09:30"),
		},
		{
			name: "focus moves a timer not set by hand",
			w:    notByHandA(),
			cmd:  core.Touch{Key: "BB-2", Origin: prompt, NewEntryID: "EB"},
			want: focused(state(
				ticket("AA-1", entry("EA", prompt, seg("09:00", "09:30"))),
				ticket("BB-2", entry("EB", prompt, seg("09:30", "")))), "BB-2"),
			events:   []core.Event{{Kind: core.EventPaused, Key: "AA-1"}, {Kind: core.EventStarted, Key: "BB-2"}},
			lastSeen: at("09:30"),
		},
		{
			// ADR 0010: the user's focus moves a timer they set by hand; the
			// new stretch is not by hand, and the paused one keeps its mark.
			name: "focus moves a timer set by hand",
			w:    byHandA(),
			cmd:  core.Touch{Key: "BB-2", Origin: prompt, NewEntryID: "EB"},
			want: focused(state(
				ticket("AA-1", withOrigin(entry("EA", hand, seg("09:00", "09:30")), prompt)),
				ticket("BB-2", entry("EB", prompt, seg("09:30", "")))), "BB-2"),
			events:   []core.Event{{Kind: core.EventPaused, Key: "AA-1"}, {Kind: core.EventStarted, Key: "BB-2"}},
			lastSeen: at("09:30"),
		},
		{
			name:     "an unchanged focus leaves a timer set by hand",
			w:        lastFocus(byHandA(), "BB-2"),
			cmd:      core.Touch{Key: "BB-2", Origin: prompt, NewEntryID: "EB"},
			want:     focused(byHandA().State, "BB-2"),
			lastSeen: at("09:30"),
		},
		{
			name:     "focus on the running ticket set by hand moves nothing",
			w:        byHandA(),
			cmd:      core.Touch{Key: "AA-1", Origin: prompt, NewEntryID: "X"},
			want:     focused(byHandA().State, "AA-1"),
			lastSeen: at("09:30"),
		},
		{
			name:     "an agent's touch on another ticket, with the timer set by hand, is not presence",
			w:        byHandA(),
			cmd:      core.Touch{Key: "BB-2", Origin: agent, NewEntryID: "EB"},
			want:     byHandA().State,
			lastSeen: at("09:00"),
		},
		{
			name:     "focus does not start a timer when nothing runs",
			w:        world("09:00", ticket("AA-1", entry("EA", hand, seg("08:00", "08:30")))),
			cmd:      core.Touch{Key: "AA-1", Origin: prompt, NewEntryID: "X"},
			want:     focused(state(ticket("AA-1", entry("EA", hand, seg("08:00", "08:30")))), "AA-1"),
			lastSeen: at("09:30"),
		},
		{
			name:     "an agent's touch is not presence",
			w:        notByHandA(),
			cmd:      core.Touch{Key: "BB-2", Origin: agent, NewEntryID: "EB"},
			want:     notByHandA().State,
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

func removed(t core.TrackedTicket, clock string) core.TrackedTicket {
	t.RemovedAt = at(clock)
	return t
}

func unverified(t core.TrackedTicket) core.TrackedTicket { t.Unverified = true; return t }

func summarized(t core.TrackedTicket, summary string) core.TrackedTicket {
	t.Summary = summary
	return t
}

// #56: the list is the tickets not removed; a removed ticket's worklogs
// are still counted wherever the range, not the list, decides.
func TestListOrderLeavesOutRemovedTickets(t *testing.T) {
	s := state(ticket("AA-1"), removed(ticket("BB-2"), "09:00"), ticket("CC-3"))
	keys := func(ts []core.TrackedTicket) []string {
		var out []string
		for _, t := range ts {
			out = append(out, t.Key)
		}
		return out
	}
	diff(t, "list", []string{"AA-1", "CC-3"}, keys(core.ListOrder(s, at("10:00").Time)))
	diff(t, "all", []string{"AA-1", "BB-2", "CC-3"}, keys(core.AllOrder(s, at("10:00").Time)))
}

func TestKeptWorklogsCountsOnlyLogged(t *testing.T) {
	wl := func(id string, st core.WorklogState) core.TimeEntry {
		return withWorklog(entry(id, hand, seg("08:00", "08:30")), core.Worklog{State: st, StartedAt: at("08:00"), TimeSpentSeconds: 1800})
	}
	tk := ticket("AA-1", wl("E1", core.WorklogSynced), wl("E2", core.WorklogLocal), wl("E3", core.WorklogFailed), entry("E4", hand, seg("09:00", "")))
	if n := core.KeptWorklogs(tk); n != 2 {
		t.Errorf("KeptWorklogs = %d, want 2", n)
	}
}

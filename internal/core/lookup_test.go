package core_test

import (
	"testing"

	"github.com/j-dsmith/punchcard/internal/core"
)

func TestFindEntryByKeyAndID(t *testing.T) {
	s := state(
		ticket("AA-1", entry("E1", hand, seg("08:00", "08:30")), entry("E2", hand, seg("09:00", ""))),
		ticket("BB-2", entry("E3", agent, seg("07:00", "07:10"))),
	)
	if e, ok := s.Entry("AA-1", "E2"); !ok || e.ID != "E2" || !e.Running() {
		t.Errorf("Entry(AA-1, E2) = %s, %v", e.ID, ok)
	}
	if _, ok := s.Entry("BB-2", "E2"); ok {
		t.Error("Entry found E2 under the wrong key")
	}
	if _, ok := s.Entry("CC-3", "E1"); ok {
		t.Error("Entry found an entry on an untracked key")
	}
	if e, ok := s.TrackedTickets[1].Entry("E3"); !ok || e.ID != "E3" {
		t.Errorf("TrackedTicket.Entry(E3) = %s, %v", e.ID, ok)
	}
	if _, ok := s.TrackedTickets[1].Entry("E1"); ok {
		t.Error("TrackedTicket.Entry found another ticket's entry")
	}
}

func TestLastPausedIsTheMostRecentlyPausedEntry(t *testing.T) {
	s := state(
		ticket("AA-1", entry("EA", hand, seg("08:00", "08:30"))),
		ticket("BB-2", entry("EB", hand, seg("09:00", "09:45"))),
		ticket("CC-3", entry("EC", hand, seg("10:00", ""))),
		ticket("DD-4", withWorklog(entry("ED", hand, seg("11:00", "11:30")), core.Worklog{State: core.WorklogPending})),
	)
	tk, e, ok := core.LastPaused(s)
	if !ok || tk.Key != "BB-2" || e.ID != "EB" {
		t.Errorf("LastPaused = %s/%s/%v, want BB-2/EB/true", tk.Key, e.ID, ok)
	}
	if _, _, ok := core.LastPaused(state(ticket("CC-3", entry("EC", hand, seg("10:00", ""))))); ok {
		t.Error("LastPaused found a paused entry where only one runs")
	}
}

func TestStateOf(t *testing.T) {
	running := ticket("AA-1", entry("EA", hand, seg("09:00", "")))
	paused := ticket("BB-2", entry("EB", hand, seg("08:00", "08:30")))
	stopped := ticket("CC-3", withWorklog(entry("EC", hand, seg("07:00", "07:30")), core.Worklog{State: core.WorklogPending}))
	idle := ticket("DD-4")
	cases := []struct {
		name     string
		lastSeen string
		t        core.TrackedTicket
		now      string
		want     core.TimerState
	}{
		{"running with presence", "09:30", running, "10:00", core.TimerRunning},
		{"running with no presence for over two hours is stale", "09:30", running, "11:31", core.TimerStale},
		{"paused", "09:30", paused, "10:00", core.TimerPaused},
		{"paused never goes stale", "08:00", paused, "23:00", core.TimerPaused},
		{"stopped is idle", "09:30", stopped, "10:00", core.TimerIdle},
		{"no entries is idle", "09:30", idle, "10:00", core.TimerIdle},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := world(tc.lastSeen, tc.t)
			if got := core.StateOf(w, tc.t, at(tc.now).Time); got != tc.want {
				t.Errorf("StateOf = %q, want %q", got, tc.want)
			}
		})
	}
	// The JSON words are the contract's.
	for s, want := range map[core.TimerState]string{core.TimerRunning: "running", core.TimerPaused: "paused", core.TimerStale: "stale", core.TimerIdle: ""} {
		if string(s) != want {
			t.Errorf("TimerState %q, want %q", s, want)
		}
	}
}

func TestEffectiveLastSeen(t *testing.T) {
	e := entry("EA", hand, seg("08:00", "08:30"), seg("09:00", ""))
	if got := core.EffectiveLastSeen(core.Heartbeat{LastSeen: at("09:40")}, e); !got.Equal(at("09:40").Time) {
		t.Errorf("with lastSeen = %v", got)
	}
	if got := core.EffectiveLastSeen(core.Heartbeat{}, e); !got.Equal(at("09:00").Time) {
		t.Errorf("without lastSeen = %v, want the open segment's start 09:00", got)
	}
}

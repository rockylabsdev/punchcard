package core_test

import (
	"testing"

	"github.com/j-dsmith/punchcard/internal/core"
)

func TestStale(t *testing.T) {
	running := ticket("AA-1", entry("EA", hand, seg("09:00", "")))
	paused := ticket("AA-1", entry("EA", hand, seg("09:00", "09:10")))
	cases := []struct {
		name string
		w    core.World
		now  string
		want bool
	}{
		{"exactly 2 h with no presence is not yet stale", world("10:00", running), "12:00", false},
		{"past 2 h with no presence is stale", world("10:00", running), "12:00:01", true},
		{"a paused timer is never stale", world("10:00", paused), "15:00", false},
		{"nothing running is never stale", world("10:00"), "15:00", false},
		{"no presence ever recorded is stale past 2 h", world("", running), "11:00:01", true},
		{"no presence ever, under 2 h of running, is not stale", world("", running), "10:59", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := core.Stale(tc.w, at(tc.now).Time); got != tc.want {
				t.Errorf("Stale = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestApplyRecovery(t *testing.T) {
	staleA := func() core.World {
		return world("10:00", ticket("AA-1", entry("EA", agent, seg("09:00", ""))), ticket("BB-2"))
	}
	runApply(t, []applyCase{
		{
			name: "recovery caps the open segment at lastSeen and leaves it paused",
			w:    staleA(),
			cmd:  core.Start{Key: "BB-2", Origin: hand, NewEntryID: "EB"},
			now:  "13:00",
			want: state(
				ticket("AA-1", recovered(entry("EA", agent, seg("09:00", "10:00")))),
				ticket("BB-2", entry("EB", hand, seg("13:00", "")))),
			events: []core.Event{{Kind: core.EventRecovered, Key: "AA-1", At: at("10:00")}},
		},
		{
			name: "recovery clamps to the segment start when lastSeen is earlier",
			w:    world("08:00", ticket("AA-1", entry("EA", agent, seg("09:00", ""))), ticket("BB-2")),
			cmd:  core.Start{Key: "BB-2", Origin: hand, NewEntryID: "EB"},
			now:  "12:00",
			want: state(
				ticket("AA-1", recovered(entry("EA", agent, seg("09:00", "09:00")))),
				ticket("BB-2", entry("EB", hand, seg("12:00", "")))),
			events: []core.Event{{Kind: core.EventRecovered, Key: "AA-1", At: at("09:00")}},
		},
		{
			name: "an agent's change recovers too: origin never decides recovery",
			w:    staleA(),
			cmd:  core.Start{Key: "BB-2", Origin: agent, NewEntryID: "EB"},
			now:  "13:00",
			want: state(
				ticket("AA-1", recovered(entry("EA", agent, seg("09:00", "10:00")))),
				ticket("BB-2", entry("EB", agent, seg("13:00", "")))),
			events: []core.Event{{Kind: core.EventRecovered, Key: "AA-1", At: at("10:00")}},
		},
		{
			name: "pausing a stale timer recovers it and is satisfied",
			w:    staleA(),
			cmd:  core.Pause{Origin: hand},
			now:  "13:00",
			want: state(
				ticket("AA-1", recovered(entry("EA", agent, seg("09:00", "10:00")))),
				ticket("BB-2")),
			events: []core.Event{{Kind: core.EventRecovered, Key: "AA-1", At: at("10:00")}},
		},
		{
			name: "resuming a recovered entry keeps the flag",
			w:    world("13:00", ticket("AA-1", recovered(entry("EA", agent, seg("09:00", "10:00"))))),
			cmd:  core.Resume{Origin: hand},
			now:  "13:00",
			want: state(ticket("AA-1", recovered(entry("EA", hand, seg("09:00", "10:00"), seg("13:00", ""))))),
		},
		{
			name: "a timer within the threshold is not recovered",
			w:    world("10:00", ticket("AA-1", entry("EA", agent, seg("09:00", ""))), ticket("BB-2")),
			cmd:  core.Start{Key: "BB-2", Origin: hand, NewEntryID: "EB"},
			now:  "11:59",
			want: state(
				ticket("AA-1", withOrigin(entry("EA", agent, seg("09:00", "11:59")), hand)),
				ticket("BB-2", entry("EB", hand, seg("11:59", "")))),
			events: []core.Event{{Kind: core.EventPaused, Key: "AA-1"}},
		},
	})
}

func TestRecoveryHappensBeforePresenceIsRecorded(t *testing.T) {
	w := world("10:00", ticket("AA-1", entry("EA", agent, seg("09:00", ""))))
	got, _, err := core.Apply(w, core.Start{Key: "AA-1", Origin: hand}, at("13:00").Time)
	if err != nil {
		t.Fatal(err)
	}
	// Capped at the old lastSeen, not at the presence this change records.
	diff(t, "state", state(ticket("AA-1", recovered(entry("EA", hand, seg("09:00", "10:00"), seg("13:00", ""))))), got.State)
	diff(t, "lastSeen", at("13:00"), got.Heartbeat.LastSeen)
}

func TestRecoverIsAPureViewOfTheSameRule(t *testing.T) {
	w := world("10:00", ticket("AA-1", entry("EA", agent, seg("09:00", ""))))
	got, events := core.Recover(w, at("13:00").Time)
	diff(t, "state", state(ticket("AA-1", recovered(entry("EA", agent, seg("09:00", "10:00"))))), got.State)
	diff(t, "events", []core.Event{{Kind: core.EventRecovered, Key: "AA-1", At: at("10:00")}}, events)
	diff(t, "heartbeat untouched", w.Heartbeat, got.Heartbeat)

	same, none := core.Recover(world("12:30", ticket("AA-1", entry("EA", agent, seg("09:00", "")))), at("13:00").Time)
	diff(t, "fresh timer untouched", state(ticket("AA-1", entry("EA", agent, seg("09:00", "")))), same.State)
	diff(t, "no events", []core.Event(nil), none)
}

func TestPresence(t *testing.T) {
	cases := []struct {
		name     string
		lastSeen string
		origin   core.Origin
		now      string
		want     string
	}{
		{"hand is presence", "09:00", hand, "09:10", "09:10"},
		{"prompt is presence", "09:00", prompt, "09:10", "09:10"},
		{"agent is not presence", "09:00", agent, "09:10", "09:00"},
		{"presence within 30 s is not rewritten", "09:00:00", hand, "09:00:29", "09:00:00"},
		{"presence at 30 s is rewritten", "09:00:00", hand, "09:00:30", "09:00:30"},
		{"first presence ever is recorded", "", hand, "09:00", "09:00"},
		{"a clock that went backwards is rewritten", "09:00", hand, "08:00", "08:00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := world(tc.lastSeen, ticket("AA-1", entry("EA", agent, seg("07:00", ""))))
			got, _, err := core.Apply(w, core.Start{Key: "AA-1", Origin: tc.origin}, at(tc.now).Time)
			if err != nil {
				t.Fatal(err)
			}
			diff(t, "lastSeen", at(tc.want), got.Heartbeat.LastSeen)
		})
	}
}

func TestRecordPresence(t *testing.T) {
	hb := core.Heartbeat{SchemaVersion: 1, LastSeen: at("09:00")}
	diff(t, "throttled", hb, core.RecordPresence(hb, at("09:00:10").Time))
	diff(t, "written", core.Heartbeat{SchemaVersion: 1, LastSeen: at("09:01")}, core.RecordPresence(hb, at("09:01").Time))
	diff(t, "first write sets the schema", core.Heartbeat{SchemaVersion: 1, LastSeen: at("09:01")}, core.RecordPresence(core.Heartbeat{}, at("09:01").Time))
}

func TestRefusedChangeRecordsNothing(t *testing.T) {
	w := world("10:00", ticket("AA-1", entry("EA", hand, seg("09:00", "09:30"))))
	got, events, err := core.Apply(w, core.Pause{Origin: hand}, at("10:30").Time)
	if err == nil {
		t.Fatal("want an error")
	}
	diff(t, "world", w, got)
	diff(t, "events", []core.Event(nil), events)
}

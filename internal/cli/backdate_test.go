package cli_test

import (
	"encoding/json"
	"testing"
	"time"
)

func TestARefusedAtSaysWhyWithoutTheSentinel(t *testing.T) {
	m := newMachine(t)
	m.now = time.Date(2026, 8, 9, 8, 58, 0, 0, zone)
	m.ok("start", "AA-1")
	m.now = time.Date(2026, 8, 9, 10, 50, 0, 0, zone)
	const why = "08:30 is before the entry started at 08:58"

	if r := m.run("pause", "--at", "08:30"); r.stderr != "punchcard: "+why+"\n" {
		t.Errorf("stderr %q, want %q", r.stderr, "punchcard: "+why+"\n")
	}
	r := m.run("pause", "--at", "08:30", "--json")
	var env struct {
		Error struct{ Code, Message string }
	}
	if err := json.Unmarshal([]byte(r.stdout), &env); err != nil {
		t.Fatalf("%v: %q", err, r.stdout)
	}
	if env.Error.Code != "usage" || env.Error.Message != why {
		t.Errorf("error %+v, want code usage and message %q", env.Error, why)
	}
}

func TestNothingToPauseSaysWhyWithoutTheSentinel(t *testing.T) {
	m := newMachine(t)
	m.now = time.Date(2026, 8, 9, 9, 0, 0, 0, zone)
	const why = "nothing is running"

	if r := m.run("pause"); r.stderr != "punchcard: "+why+"\n" {
		t.Errorf("stderr %q, want %q", r.stderr, "punchcard: "+why+"\n")
	}
	r := m.run("pause", "--json")
	var env struct {
		Error struct{ Code, Message string }
	}
	if err := json.Unmarshal([]byte(r.stdout), &env); err != nil {
		t.Fatalf("%v: %q", err, r.stdout)
	}
	if env.Error.Code != "no_timer" || env.Error.Message != why {
		t.Errorf("error %+v, want code no_timer and message %q", env.Error, why)
	}
}

func TestAMeetingCutOutWithTwoCommands(t *testing.T) {
	m := newMachine(t)
	m.now = time.Date(2026, 8, 9, 8, 58, 0, 0, zone)
	m.ok("start", "AA-1")
	m.now = time.Date(2026, 8, 9, 10, 50, 0, 0, zone)

	wantLines(t, m.ok("pause", "--at", "10:00"), "⏸ AA-1 paused at 10:00  1:02:00")
	wantLines(t, m.ok("resume", "--at", "-5m"), "▶ AA-1 running since 10:45  1:07:00")
}

func TestACutThatLeavesAStubSaysSo(t *testing.T) {
	m := newMachine(t)
	m.now = time.Date(2026, 8, 9, 9, 59, 30, 0, zone)
	m.ok("start", "AA-1")
	m.now = time.Date(2026, 8, 9, 10, 50, 0, 0, zone)
	wantLines(t, m.ok("pause", "--at", "10:00"), "paused at 10:00", "the 30s from 09:59 was under a minute, so it was dropped")
}

func TestBackdatedCommandsReportAtInEvents(t *testing.T) {
	m := newMachine(t)
	m.now = time.Date(2026, 8, 9, 8, 58, 0, 0, zone)
	m.ok("start", "AA-1")
	m.now = time.Date(2026, 8, 9, 10, 50, 0, 0, zone)

	env := decode(t, m.ok("pause", "--at", "10:00", "--json"))
	wantEvent(t, env, "paused", "AA-1", "2026-08-09T10:00:00.000-0500")
	env = decode(t, m.ok("resume", "--at", "10:45", "--json"))
	wantEvent(t, env, "started", "AA-1", "2026-08-09T10:45:00.000-0500")
	env = decode(t, m.ok("start", "AA-2", "--at", "10:48", "--json"))
	wantEvent(t, env, "paused", "AA-1", "2026-08-09T10:48:00.000-0500")
	wantEvent(t, env, "started", "AA-2", "2026-08-09T10:48:00.000-0500")

	env = decode(t, m.ok("pause", "--json"))
	for _, e := range env["events"].([]any) {
		if _, ok := e.(map[string]any)["at"]; ok {
			t.Errorf("a pause now carries no at: %v", e)
		}
	}
}

func wantEvent(t *testing.T, env map[string]any, kind, key, at string) {
	t.Helper()
	evs, _ := env["events"].([]any)
	for _, e := range evs {
		ev := e.(map[string]any)
		if ev["kind"] == kind && ev["key"] == key && ev["at"] == at {
			return
		}
	}
	t.Errorf("no %s event for %s at %s in %v", kind, key, at, evs)
}

func TestBackdatedRefusalsAreUsageAndChangeNothing(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"a time in the future", []string{"pause", "--at", "+5m"}},
		{"not a time", []string{"pause", "--at", "ten"}},
		{"a bare number", []string{"pause", "--at", "10"}},
		{"before the entry started", []string{"pause", "--at", "08:30"}},
		{"resume while running", []string{"resume", "--at", "10:00"}},
		{"start before the running ticket started", []string{"start", "AA-2", "--at", "08:30"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newMachine(t)
			m.now = time.Date(2026, 8, 9, 8, 58, 0, 0, zone)
			m.ok("start", "AA-1")
			m.now = time.Date(2026, 8, 9, 10, 50, 0, 0, zone)
			before := m.file("state.json")
			r := m.run(c.args...)
			if r.code != 64 || r.stderr == "" {
				t.Errorf("exit %d stderr %q, want 64 with a message", r.code, r.stderr)
			}
			if m.file("state.json") != before {
				t.Error("a refused --at changed the state")
			}
		})
	}
}

func TestTheMeetingCutIsWhatStopLogs(t *testing.T) {
	m := newMachine(t)
	m.now = time.Date(2026, 8, 9, 8, 58, 0, 0, zone)
	m.ok("start", "AA-1")
	m.now = time.Date(2026, 8, 9, 10, 50, 0, 0, zone)
	m.ok("pause", "--at", "10:00")
	m.ok("resume", "--at", "10:45")
	m.now = time.Date(2026, 8, 9, 11, 0, 0, 0, zone)

	ls := decode(t, m.ok("ls", "--json"))
	row := ls["data"].(map[string]any)["tickets"].([]any)[0].(map[string]any)
	if got := row["elapsedSeconds"]; got != float64(77*60) {
		t.Errorf("ls elapsedSeconds %v, want %d", got, 77*60)
	}
	if r := m.run("stop", "--yes"); r.code != 7 { // stopped; owed to Jira, which this machine lacks
		t.Fatalf("stop: exit %d %q", r.code, r.stderr)
	}
	log := decode(t, m.ok("log", "--json"))
	rows := log["data"].(map[string]any)["rows"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["minutes"] != float64(77) {
		t.Errorf("log rows %v, want one of 77 minutes", rows)
	}
}

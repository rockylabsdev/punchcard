package core_test

import (
	"strings"
	"testing"
	"time"

	"github.com/j-dsmith/punchcard/internal/core"
)

// cut is a segment the user closed at a named time (pause --at).
func cut(start, end string) core.Segment { s := seg(start, end); s.Cut = true; return s }

// pausedBy is e after o paused it: its stretch's ByHand stays as it was.
func pausedBy(o core.Origin, e core.TimeEntry) core.TimeEntry { e.Origin = o; return e }

func TestPauseAt(t *testing.T) {
	running := func() core.World {
		return world("10:50", ticket("AA-1", entry("EA", hand, seg("08:58", ""))))
	}
	runApply(t, []applyCase{
		{
			name:   "pause --at closes the open segment at that time, marked cut",
			w:      running(),
			cmd:    core.Pause{Origin: hand, At: at("10:00").Time},
			now:    "10:50",
			want:   state(ticket("AA-1", entry("EA", hand, cut("08:58", "10:00")))),
			events: []core.Event{{Kind: core.EventPaused, Key: "AA-1", At: at("10:00")}},
		},
		{
			name: "pause --at in the future is refused",
			w:    running(), cmd: core.Pause{Origin: hand, At: at("10:51").Time}, now: "10:50", err: core.ErrUsage,
		},
		{
			name: "pause --at before the open segment started is refused",
			w:    running(), cmd: core.Pause{Origin: hand, At: at("08:57").Time}, now: "10:50", err: core.ErrUsage,
		},
		{
			name: "pause --at is refused to an agent on a timer set by hand",
			w:    running(), cmd: core.Pause{Origin: agent, At: at("10:00").Time}, now: "10:50", err: core.ErrByHand,
		},
		{
			name: "a cut that leaves a stub under a minute drops the stub",
			w: world("10:50", ticket("AA-1", entry("EA", hand,
				seg("08:00", "08:30"), seg("09:59:30", "")))),
			cmd:  core.Pause{Origin: hand, At: at("10:00").Time},
			now:  "10:50",
			want: state(ticket("AA-1", entry("EA", hand, cut("08:00", "08:30")))),
			events: []core.Event{{Kind: core.EventPaused, Key: "AA-1", At: at("10:00"),
				Message: "the 30s from 09:59 was under a minute, so it was dropped"}},
		},
		{
			name: "a cut at the segment's own start drops it like any stub",
			w: world("10:50", ticket("AA-1", entry("EA", hand,
				cut("08:58", "10:00"), seg("10:45", "")))),
			cmd:  core.Pause{Origin: hand, At: at("10:45").Time},
			now:  "10:50",
			want: state(ticket("AA-1", entry("EA", hand, cut("08:58", "10:00")))),
			events: []core.Event{{Kind: core.EventPaused, Key: "AA-1", At: at("10:45"),
				Message: "the 0s from 10:45 was under a minute, so it was dropped"}},
		},
		{
			name: "a stub that is the entry's only segment is closed at its own start",
			w:    world("10:50", ticket("AA-1", entry("EA", hand, seg("09:59:30", "")))),
			cmd:  core.Pause{Origin: hand, At: at("10:00").Time},
			now:  "10:50",
			want: state(ticket("AA-1", entry("EA", hand, cut("09:59:30", "09:59:30")))),
			events: []core.Event{{Kind: core.EventPaused, Key: "AA-1", At: at("10:00"),
				Message: "the 30s from 09:59 was under a minute, so it was dropped"}},
		},
		{
			name:   "pause --at on a paused entry moves its pause",
			w:      world("10:50", ticket("AA-1", entry("EA", hand, seg("08:58", "10:30")))),
			cmd:    core.Pause{Origin: hand, At: at("10:00").Time},
			now:    "10:50",
			want:   state(ticket("AA-1", entry("EA", hand, cut("08:58", "10:00")))),
			events: []core.Event{{Kind: core.EventPaused, Key: "AA-1", At: at("10:00")}},
		},
		{
			name: "pause --at KEY moves that paused entry's pause, not the last paused",
			w: world("10:50",
				ticket("AA-1", entry("EA", hand, seg("08:00", "09:00"))),
				ticket("BB-2", entry("EB", hand, seg("09:00", "10:30")))),
			cmd: core.Pause{Key: "AA-1", Origin: hand, At: at("08:40").Time},
			now: "10:50",
			want: state(
				ticket("AA-1", entry("EA", hand, cut("08:00", "08:40"))),
				ticket("BB-2", entry("EB", hand, seg("09:00", "10:30")))),
			events: []core.Event{{Kind: core.EventPaused, Key: "AA-1", At: at("08:40")}},
		},
		{
			name: "moving a pause later over another ticket's time is refused",
			w: world("10:50",
				ticket("AA-1", entry("EA", hand, seg("08:00", "09:00"))),
				ticket("BB-2", entry("EB", hand, seg("09:00", "10:30")))),
			cmd: core.Pause{Key: "AA-1", Origin: hand, At: at("09:10").Time},
			now: "10:50",
			err: core.ErrUsage,
		},
		{
			name: "pause --at on a stale timer recovers it, then moves the pause",
			w:    world("09:30", ticket("AA-1", entry("EA", hand, seg("08:58", "")))),
			cmd:  core.Pause{Origin: hand, At: at("09:10").Time},
			now:  "12:00",
			want: state(ticket("AA-1", recovered(entry("EA", hand, cut("08:58", "09:10"))))),
			events: []core.Event{
				{Kind: core.EventRecovered, Key: "AA-1", At: at("09:30")},
				{Kind: core.EventPaused, Key: "AA-1", At: at("09:10")},
			},
		},
		{
			name: "pause --at with nothing open is refused",
			w:    world("10:50", ticket("AA-1")), cmd: core.Pause{Origin: hand, At: at("10:00").Time}, now: "10:50", err: core.ErrNoTimer,
		},
	})
}

func TestResumeAt(t *testing.T) {
	paused := func() core.World {
		return world("10:50", ticket("AA-1", entry("EA", hand, cut("08:58", "10:00"))))
	}
	runApply(t, []applyCase{
		{
			name:   "resume --at opens the new segment at that time",
			w:      paused(),
			cmd:    core.Resume{Origin: hand, At: at("10:45").Time},
			now:    "10:50",
			want:   state(ticket("AA-1", entry("EA", hand, cut("08:58", "10:00"), seg("10:45", "")))),
			events: []core.Event{{Kind: core.EventStarted, Key: "AA-1", At: at("10:45")}},
		},
		{
			name: "resume --at before the pause is refused: the gap cannot be negative",
			w:    paused(), cmd: core.Resume{Origin: hand, At: at("09:59").Time}, now: "10:50", err: core.ErrUsage,
		},
		{
			name: "resume --at in the future is refused",
			w:    paused(), cmd: core.Resume{Origin: hand, At: at("10:51").Time}, now: "10:50", err: core.ErrUsage,
		},
		{
			name: "resume --at over another ticket's time since is refused",
			w: world("10:50",
				ticket("AA-1", entry("EA", hand, seg("08:00", "09:00"))),
				ticket("BB-2", entry("EB", hand, seg("09:00", "10:00")))),
			cmd: core.Resume{Key: "AA-1", Origin: hand, At: at("09:30").Time},
			now: "10:50",
			err: core.ErrUsage,
		},
		{
			name: "resume --at while a timer runs is refused",
			w:    world("10:50", ticket("AA-1", entry("EA", hand, seg("08:58", "")))),
			cmd:  core.Resume{Origin: hand, At: at("10:00").Time},
			now:  "10:50",
			err:  core.ErrUsage,
		},
	})
}

func TestStartAt(t *testing.T) {
	runApply(t, []applyCase{
		{
			name:   "start --at starts a new entry at that time",
			w:      world("10:50", ticket("AA-1")),
			cmd:    core.Start{Key: "AA-1", Origin: hand, NewEntryID: "EA", At: at("09:00").Time},
			now:    "10:50",
			want:   state(ticket("AA-1", entry("EA", hand, seg("09:00", "")))),
			events: []core.Event{{Kind: core.EventStarted, Key: "AA-1", At: at("09:00")}},
		},
		{
			name: "start --at pauses the running ticket at that time too",
			w:    world("10:50", ticket("AA-1", entry("EA", agent, seg("08:30", ""))), ticket("BB-2")),
			cmd:  core.Start{Key: "BB-2", Origin: hand, NewEntryID: "EB", At: at("09:00").Time},
			now:  "10:50",
			want: state(
				ticket("AA-1", pausedBy(hand, entry("EA", agent, seg("08:30", "09:00")))),
				ticket("BB-2", entry("EB", hand, seg("09:00", "")))),
			events: []core.Event{
				{Kind: core.EventPaused, Key: "AA-1", At: at("09:00")},
				{Kind: core.EventStarted, Key: "BB-2", At: at("09:00")},
			},
		},
		{
			name: "start --at before the running ticket's open segment started is refused",
			w:    world("10:50", ticket("AA-1", entry("EA", agent, seg("09:30", ""))), ticket("BB-2")),
			cmd:  core.Start{Key: "BB-2", Origin: hand, NewEntryID: "EB", At: at("09:00").Time},
			now:  "10:50",
			err:  core.ErrUsage,
		},
		{
			name: "start --at before the ticket's last stopped entry ended is refused",
			w: world("10:50", ticket("AA-1", withWorklog(entry("EA", hand, seg("08:00", "09:30")),
				core.Worklog{State: core.WorklogPending, StartedAt: at("08:00"), TimeSpentSeconds: 5400}))),
			cmd: core.Start{Key: "AA-1", Origin: hand, NewEntryID: "EB", At: at("09:00").Time},
			now: "10:50",
			err: core.ErrUsage,
		},
		{
			name: "start --at in the future is refused",
			w:    world("10:50", ticket("AA-1")),
			cmd:  core.Start{Key: "AA-1", Origin: hand, NewEntryID: "EA", At: at("11:00").Time},
			now:  "10:50",
			err:  core.ErrUsage,
		},
		{
			name: "start --at on the running ticket is refused",
			w:    world("10:50", ticket("AA-1", entry("EA", hand, seg("09:30", "")))),
			cmd:  core.Start{Key: "AA-1", Origin: hand, NewEntryID: "X", At: at("10:00").Time},
			now:  "10:50",
			err:  core.ErrUsage,
		},
		{
			name: "start --at is refused to an agent over a timer set by hand",
			w:    world("10:50", ticket("AA-1", entry("EA", hand, seg("08:30", ""))), ticket("BB-2")),
			cmd:  core.Start{Key: "BB-2", Origin: agent, NewEntryID: "EB", At: at("09:00").Time},
			now:  "10:50",
			err:  core.ErrByHand,
		},
		{
			name: "the pause start --at causes drops a stub under a minute",
			w: world("10:50", ticket("AA-1", entry("EA", agent, seg("08:00", "08:30"), seg("08:59:20", ""))),
				ticket("BB-2")),
			cmd: core.Start{Key: "BB-2", Origin: hand, NewEntryID: "EB", At: at("09:00").Time},
			now: "10:50",
			want: state(
				ticket("AA-1", pausedBy(hand, entry("EA", agent, seg("08:00", "08:30")))),
				ticket("BB-2", entry("EB", hand, seg("09:00", "")))),
			events: []core.Event{
				{Kind: core.EventPaused, Key: "AA-1", At: at("09:00"), Message: "the 40s from 08:59 was under a minute, so it was dropped"},
				{Kind: core.EventStarted, Key: "BB-2", At: at("09:00")},
			},
		},
	})
}

func TestBackdatedRefusalsSayWhy(t *testing.T) {
	w := world("10:50", ticket("AA-1", entry("EA", hand, seg("08:58", ""))))
	_, _, err := core.Apply(w, core.Pause{Origin: hand, At: at("08:30").Time}, at("10:50").Time)
	if err == nil || !strings.Contains(err.Error(), "before the entry started at 08:58") {
		t.Errorf("err = %v", err)
	}
	p := world("10:50", ticket("AA-1", entry("EA", hand, cut("08:58", "10:00"))))
	_, _, err = core.Apply(p, core.Resume{Origin: hand, At: at("09:45").Time}, at("10:50").Time)
	if err == nil || !strings.Contains(err.Error(), "09:45 is before 10:00") {
		t.Errorf("err = %v", err)
	}
}

func TestAMeetingCutOutOfARunningEntry(t *testing.T) {
	w := world("10:50", ticket("AA-1", entry("EA", hand, seg("08:58", ""))))
	w, _, err := core.Apply(w, core.Pause{Origin: hand, At: at("10:00").Time}, at("10:50").Time)
	if err != nil {
		t.Fatal(err)
	}
	w, _, err = core.Apply(w, core.Resume{Origin: hand, At: at("10:45").Time}, at("10:50").Time)
	if err != nil {
		t.Fatal(err)
	}
	_, e, _ := core.Running(w.State)
	if got, want := core.Elapsed(e, at("10:50").Time), 67*time.Minute; got != want {
		t.Errorf("elapsed %s, want %s", got, want)
	}
}

func TestStartAtLastNightSplitsAtMidnightForToday(t *testing.T) {
	now := day(10, "00:10").Time
	when, err := core.ParseAt("23:50", now)
	if err != nil {
		t.Fatal(err)
	}
	w, _, err := core.Apply(world(""), core.Start{Key: "AA-1", Origin: hand, NewEntryID: "EA", At: when}, now)
	if err != nil {
		t.Fatal(err)
	}
	tk, _, _ := core.Running(w.State)
	if got := core.TimeToday(tk, now); got != 10*time.Minute {
		t.Errorf("time today %s, want 10m", got)
	}
}

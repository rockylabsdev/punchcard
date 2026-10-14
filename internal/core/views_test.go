package core_test

import (
	"errors"
	"testing"
	"time"

	"github.com/j-dsmith/punchcard/internal/core"
)

func TestTimeToday(t *testing.T) {
	span := func(startDay int, start string, endDay int, end string) core.Segment {
		s := core.Segment{StartedAt: day(startDay, start)}
		if end != "" {
			s.EndedAt = day(endDay, end)
		}
		return s
	}
	cases := []struct {
		name string
		segs []core.Segment
		now  core.Timestamp
		want time.Duration
	}{
		{"a segment inside today counts whole", []core.Segment{span(9, "09:00", 9, "10:30")}, day(9, "12:00"), 90 * time.Minute},
		{"a segment over midnight counts only after midnight", []core.Segment{span(8, "23:00", 9, "01:15")}, day(9, "12:00"), 75 * time.Minute},
		{"a running segment over midnight counts from midnight to now", []core.Segment{span(8, "22:00", 0, "")}, day(9, "00:40"), 40 * time.Minute},
		{"yesterday is never recounted", []core.Segment{span(8, "09:00", 8, "17:00")}, day(9, "12:00"), 0},
		{"today never carries over to tomorrow", []core.Segment{span(9, "23:30", 10, "00:10")}, day(9, "23:59"), 29 * time.Minute},
		{"segments across entries sum", []core.Segment{span(9, "09:00", 9, "09:10"), span(9, "11:00", 0, "")}, day(9, "11:05"), 15 * time.Minute},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tk := ticket("AA-1", core.TimeEntry{ID: "E", Segments: tc.segs})
			if got := core.TimeToday(tk, tc.now.Time); got != tc.want {
				t.Errorf("TimeToday = %v, want %v", got, tc.want)
			}
		})
	}
}

// A stopped entry counts what was logged, not what was measured: the
// day's total agrees with the worklogs.
func TestTimeTodayCountsAStoppedEntrysWorklog(t *testing.T) {
	stopped := func(minutes int, segs ...core.Segment) core.TimeEntry {
		return core.TimeEntry{ID: "E", Segments: segs,
			Worklog: &core.Worklog{State: core.WorklogLocal, StartedAt: segs[0].StartedAt, TimeSpentSeconds: minutes * 60}}
	}
	cases := []struct {
		name    string
		entries []core.TimeEntry
		now     core.Timestamp
		want    time.Duration
	}{
		{"a stop that logged more than was measured counts the worklog",
			[]core.TimeEntry{stopped(90, core.Segment{StartedAt: day(9, "22:24"), EndedAt: day(9, "22:27")})},
			day(9, "22:30"), 90 * time.Minute},
		{"a stop that logged less than was measured counts the worklog",
			[]core.TimeEntry{stopped(30, core.Segment{StartedAt: day(9, "09:00"), EndedAt: day(9, "11:00")})},
			day(9, "12:00"), 30 * time.Minute},
		{"a stopped entry over midnight counts whole on its worklog's day",
			[]core.TimeEntry{stopped(120, core.Segment{StartedAt: day(8, "23:00"), EndedAt: day(9, "01:00")})},
			day(8, "23:59"), 2 * time.Hour},
		{"and nothing the day after",
			[]core.TimeEntry{stopped(120, core.Segment{StartedAt: day(8, "23:00"), EndedAt: day(9, "01:00")})},
			day(9, "12:00"), 0},
		{"a stopped entry and an open one sum",
			[]core.TimeEntry{
				stopped(90, core.Segment{StartedAt: day(9, "09:00"), EndedAt: day(9, "09:03")}),
				{ID: "E2", Segments: []core.Segment{{StartedAt: day(9, "10:00")}}},
			},
			day(9, "10:15"), 105 * time.Minute},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := core.TimeToday(ticket("AA-1", tc.entries...), tc.now.Time); got != tc.want {
				t.Errorf("TimeToday = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTodayTotal(t *testing.T) {
	removed := func(tk core.TrackedTicket) core.TrackedTicket { tk.RemovedAt = at("11:00"); return tk }
	overnight := core.Segment{StartedAt: day(8, "23:00"), EndedAt: day(9, "01:00")}
	cases := []struct {
		name string
		w    core.World
		now  string
		want time.Duration
	}{
		{"every ticket's time today sums",
			world("", ticket("AA-1", entry("E1", core.OriginHand, seg("09:00", "10:00"))),
				ticket("AA-2", entry("E2", core.OriginHand, seg("10:30", "11:00")))),
			"12:00", 90 * time.Minute},
		{"an open segment counts up to now",
			world("11:59", ticket("AA-1", entry("E1", core.OriginHand, seg("09:00", "10:00"))),
				ticket("AA-2", entry("E2", core.OriginHand, seg("11:00", "")))),
			"12:00", 2 * time.Hour},
		{"a segment over midnight counts from midnight",
			world("", ticket("AA-1", entry("E1", core.OriginHand, overnight))),
			"12:00", time.Hour},
		{"a stale timer counts capped at lastSeen",
			world("09:40", ticket("AA-1", entry("E1", core.OriginHand, seg("09:00", "")))),
			"12:00", 40 * time.Minute},
		{"a removed ticket's time today still counts: the total answers for the day",
			world("", ticket("AA-1", entry("E1", core.OriginHand, seg("09:00", "10:00"))),
				removed(ticket("AA-2", entry("E2", core.OriginHand, seg("10:00", "10:30"))))),
			"12:00", 90 * time.Minute},
		{"nothing tracked is zero", world(""), "12:00", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := core.TodayTotal(tc.w, at(tc.now).Time); got != tc.want {
				t.Errorf("TodayTotal = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestElapsed(t *testing.T) {
	e := entry("EA", hand, seg("09:00", "09:20"), seg("09:40", ""))
	if got, want := core.Elapsed(e, at("10:00").Time), 40*time.Minute; got != want {
		t.Errorf("Elapsed = %v, want %v", got, want)
	}
}

func TestRunning(t *testing.T) {
	s := state(ticket("AA-1", entry("EA", hand, seg("08:00", "08:30"))), ticket("BB-2", entry("EB", agent, seg("09:00", ""))))
	tk, e, ok := core.Running(s)
	if !ok || tk.Key != "BB-2" || e.ID != "EB" {
		t.Errorf("Running = %s/%s/%v, want BB-2/EB/true", tk.Key, e.ID, ok)
	}
	if _, _, ok := core.Running(state(ticket("AA-1"))); ok {
		t.Error("Running reported a timer on an idle state")
	}
}

func TestListOrderIsMostRecentActivityFirst(t *testing.T) {
	added := func(k, clock string) core.TrackedTicket { tk := ticket(k); tk.AddedAt = at(clock); return tk }
	s := state(
		ticket("AA-1", entry("EA", hand, seg("09:00", "09:30"))), // last activity 09:30
		added("BB-2", "10:00"),                              // added 10:00, never worked
		ticket("CC-3", entry("EC", hand, seg("08:00", ""))), // running: activity is now
		added("DD-4", "07:00"),
	)
	var got []string
	for _, tk := range core.ListOrder(s, at("11:00").Time) {
		got = append(got, tk.Key)
	}
	diff(t, "order", []string{"CC-3", "BB-2", "AA-1", "DD-4"}, got)
}

func TestPreviewStop(t *testing.T) {
	w := world("09:30", ticket("AA-1", entry("EA", hand, seg("09:00", ""))))
	p, err := core.PreviewStop(w, core.Stop{Origin: agent, Comment: "done"}, at("09:45:30").Time)
	if err != nil {
		t.Fatal(err)
	}
	diff(t, "preview", core.Preview{Key: "AA-1", EntryID: "EA", Worklog: pending("09:00", 2700, "done")}, p)

	// A stale timer previews its capped elapsed: what stop would really send.
	stale := world("09:20", ticket("AA-1", entry("EA", agent, seg("09:00", ""))))
	p, err = core.PreviewStop(stale, core.Stop{Origin: agent}, at("13:00").Time)
	if err != nil {
		t.Fatal(err)
	}
	diff(t, "stale preview", 1200, p.Worklog.TimeSpentSeconds)
	diff(t, "preview flags recovery", true, p.Recovered)

	_, err = core.PreviewStop(world("09:00", ticket("AA-1", entry("EA", hand, seg("09:00", "")))), core.Stop{}, at("09:00:30").Time)
	if !errors.Is(err, core.ErrTooShort) {
		t.Errorf("err = %v, want ErrTooShort", err)
	}
}

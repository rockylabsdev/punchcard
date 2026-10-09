package core_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/j-dsmith/punchcard/internal/core"
)

func pending(start string, secs int, comment string) core.Worklog {
	return core.Worklog{State: core.WorklogPending, StartedAt: at(start), TimeSpentSeconds: secs, Comment: comment}
}

func TestApplyStop(t *testing.T) {
	runApply(t, []applyCase{
		{
			name: "stop truncates measured elapsed to the minute",
			w:    world("09:30", ticket("AA-1", entry("EA", hand, seg("09:00", "")))),
			cmd:  core.Stop{Origin: hand},
			now:  "09:30:45",
			want: state(ticket("AA-1", withWorklog(entry("EA", hand, seg("09:00", "09:30:45")), pending("09:00", 1800, "")))),
		},
		{
			name: "measured elapsed is the sum of segments across pauses",
			w:    world("09:50", ticket("AA-1", entry("EA", hand, seg("09:00", "09:20"), seg("09:40", "")))),
			cmd:  core.Stop{Origin: hand, Comment: "Fixed the formatter"},
			now:  "09:50:59",
			want: state(ticket("AA-1", withWorklog(entry("EA", hand, seg("09:00", "09:20"), seg("09:40", "09:50:59")), pending("09:00", 1800, "Fixed the formatter")))),
		},
		{
			name: "a user duration may exceed measured elapsed",
			w:    world("09:30", ticket("AA-1", entry("EA", hand, seg("09:00", "")))),
			cmd:  core.Stop{Origin: hand, Duration: 2 * time.Hour},
			now:  "09:30",
			want: state(ticket("AA-1", withWorklog(entry("EA", hand, seg("09:00", "09:30")), pending("09:00", 7200, "")))),
		},
		{
			name: "a user duration is truncated to the minute too",
			w:    world("09:30", ticket("AA-1", entry("EA", hand, seg("09:00", "")))),
			cmd:  core.Stop{Origin: hand, Duration: 90*time.Minute + 30*time.Second},
			now:  "09:30",
			want: state(ticket("AA-1", withWorklog(entry("EA", hand, seg("09:00", "09:30")), pending("09:00", 5400, "")))),
		},
		{
			name: "a stop pinned to the entry it previewed stops that entry",
			w:    world("09:30", ticket("AA-1", entry("EA", hand, seg("09:00", "")))),
			cmd:  core.Stop{Key: "AA-1", EntryID: "EA", Origin: hand},
			now:  "09:30",
			want: state(ticket("AA-1", withWorklog(entry("EA", hand, seg("09:00", "09:30")), pending("09:00", 1800, "")))),
		},
		{
			name: "a stop pinned to an entry that is no longer open is refused",
			w: world("09:30", ticket("AA-1",
				withWorklog(entry("EA", hand, seg("08:00", "08:30")), pending("08:00", 1800, "")),
				entry("EB", hand, seg("09:00", "")))),
			cmd: core.Stop{Key: "AA-1", EntryID: "EA", Origin: hand},
			now: "09:30",
			err: core.ErrNoTimer,
		},
		{
			name: "under one minute measured is refused",
			w:    world("09:00", ticket("AA-1", entry("EA", hand, seg("09:00", "")))),
			cmd:  core.Stop{Origin: hand},
			now:  "09:00:59",
			err:  core.ErrTooShort,
		},
		{
			name: "under one minute supplied is refused",
			w:    world("09:30", ticket("AA-1", entry("EA", hand, seg("09:00", "")))),
			cmd:  core.Stop{Origin: hand, Duration: 59 * time.Second},
			now:  "09:30",
			err:  core.ErrTooShort,
		},
		{
			name: "recovered clears only at stop",
			w:    world("09:30", ticket("AA-1", recovered(entry("EA", hand, seg("09:00", "09:10"), seg("09:20", ""))))),
			cmd:  core.Stop{Origin: hand, Duration: 40 * time.Minute},
			now:  "09:30",
			want: state(ticket("AA-1", withWorklog(entry("EA", hand, seg("09:00", "09:10"), seg("09:20", "09:30")), pending("09:00", 2400, "")))),
		},
		{
			name: "with nothing running, stop takes the single paused entry",
			w:    world("09:30", ticket("AA-1", entry("EA", hand, seg("09:00", "09:20"))), ticket("BB-2")),
			cmd:  core.Stop{Origin: hand},
			now:  "09:30",
			want: state(ticket("AA-1", withWorklog(entry("EA", hand, seg("09:00", "09:20")), pending("09:00", 1200, ""))), ticket("BB-2")),
		},
		{
			name: "stop KEY targets that ticket and leaves the running one alone",
			w: world("09:30",
				ticket("AA-1", entry("EA", hand, seg("09:00", "09:20"))),
				ticket("BB-2", entry("EB", hand, seg("09:20", "")))),
			cmd: core.Stop{Key: "AA-1", Origin: hand},
			now: "09:30",
			want: state(
				ticket("AA-1", withWorklog(entry("EA", hand, seg("09:00", "09:20")), pending("09:00", 1200, ""))),
				ticket("BB-2", entry("EB", hand, seg("09:20", "")))),
		},
		{
			name: "stop with nothing open is refused",
			w:    world("09:30", ticket("AA-1")),
			cmd:  core.Stop{Origin: hand},
			now:  "09:30",
			err:  core.ErrNoTimer,
		},
		{
			name: "stop KEY on an untracked ticket is refused",
			w:    world("09:30"),
			cmd:  core.Stop{Key: "AA-1", Origin: hand},
			now:  "09:30",
			err:  core.ErrNotTracked,
		},
	})
}

func TestStopWithSeveralPausedNamesThem(t *testing.T) {
	w := world("09:30",
		ticket("AA-1", entry("EA", hand, seg("09:00", "09:10"))),
		ticket("BB-2", entry("EB", hand, seg("09:10", "09:20"))))
	_, _, err := core.Apply(w, core.Stop{Origin: hand}, at("09:30").Time)
	if !errors.Is(err, core.ErrNoTimer) {
		t.Fatalf("err = %v, want ErrNoTimer", err)
	}
	var amb *core.AmbiguousError
	if !errors.As(err, &amb) {
		t.Fatalf("err = %v, want *AmbiguousError", err)
	}
	diff(t, "keys", []string{"AA-1", "BB-2"}, amb.Keys)
	if !strings.Contains(err.Error(), "AA-1") {
		t.Errorf("message %q does not name the paused tickets", err)
	}
}

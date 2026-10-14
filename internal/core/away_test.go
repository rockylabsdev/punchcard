package core_test

import (
	"strings"
	"testing"

	"github.com/j-dsmith/punchcard/internal/core"
)

// away is the Away dialog's change by hand; to "" leaves To empty.
func away(key, from, to string) core.Away {
	c := core.Away{Key: key, From: at(from).Time, Origin: hand}
	if to != "" {
		c.To = at(to).Time
	}
	return c
}

func TestAwayRemovesAWindow(t *testing.T) {
	running := func() core.World {
		return world("10:50", ticket("AA-1", entry("EA", hand, seg("08:58", ""))))
	}
	runApply(t, []applyCase{
		{
			name: "a window inside the running stretch splits it, and the entry keeps running",
			w:    running(),
			cmd:  away("AA-1", "10:00", "10:45"),
			now:  "10:50",
			want: state(ticket("AA-1", entry("EA", hand, seg("08:58", "10:00"), seg("10:45", "")))),
			events: []core.Event{
				{Kind: core.EventPaused, Key: "AA-1", At: at("10:00")},
				{Kind: core.EventStarted, Key: "AA-1", At: at("10:45")},
			},
		},
		{
			name:   "a window inside an earlier stretch splits it, and a paused entry stays paused",
			w:      world("10:50", ticket("AA-1", entry("EA", hand, seg("08:00", "10:00")))),
			cmd:    away("AA-1", "08:30", "09:00"),
			now:    "10:50",
			want:   state(ticket("AA-1", entry("EA", hand, seg("08:00", "08:30"), seg("09:00", "10:00")))),
			events: []core.Event{{Kind: core.EventPaused, Key: "AA-1", At: at("08:30")}},
		},
		{
			name:   "a window over a gap trims the stretches either side: resumed 21:06, paused 21:12, away 20:43–21:07",
			w:      world("21:30", ticket("AA-1", entry("EA", hand, seg("20:00", "20:50"), seg("21:06", "21:12")))),
			cmd:    away("AA-1", "20:43", "21:07"),
			now:    "21:30",
			want:   state(ticket("AA-1", entry("EA", hand, seg("20:00", "20:43"), seg("21:07", "21:12")))),
			events: []core.Event{{Kind: core.EventPaused, Key: "AA-1", At: at("20:43")}},
		},
		{
			name:   "a window over an edge trims that stretch only",
			w:      world("10:50", ticket("AA-1", entry("EA", hand, seg("08:00", "09:00"), seg("09:30", "10:30")))),
			cmd:    away("AA-1", "09:15", "09:45"),
			now:    "10:50",
			want:   state(ticket("AA-1", entry("EA", hand, seg("08:00", "09:00"), seg("09:45", "10:30")))),
			events: []core.Event{{Kind: core.EventPaused, Key: "AA-1", At: at("09:15")}},
		},
		{
			name: "a stretch wholly inside the window disappears",
			w: world("10:50", ticket("AA-1", entry("EA", hand,
				seg("08:00", "09:00"), seg("09:10", "09:20"), seg("09:30", "10:00")))),
			cmd:    away("AA-1", "09:05", "09:25"),
			now:    "10:50",
			want:   state(ticket("AA-1", entry("EA", hand, seg("08:00", "09:00"), seg("09:30", "10:00")))),
			events: []core.Event{{Kind: core.EventPaused, Key: "AA-1", At: at("09:05")}},
		},
		{
			name: "a window over earlier stretches leaves a running entry running",
			w:    world("10:50", ticket("AA-1", entry("EA", hand, seg("08:00", "09:00"), seg("09:30", "")))),
			cmd:  away("AA-1", "08:30", "09:45"),
			now:  "10:50",
			want: state(ticket("AA-1", entry("EA", hand, seg("08:00", "08:30"), seg("09:45", "")))),
			events: []core.Event{
				{Kind: core.EventPaused, Key: "AA-1", At: at("08:30")},
				{Kind: core.EventStarted, Key: "AA-1", At: at("09:45")},
			},
		},
		{
			name: "it never touches another ticket's time inside the window",
			w: world("10:50",
				ticket("AA-1", entry("EA", hand, seg("08:00", "09:00"), seg("10:00", "10:30"))),
				ticket("BB-2", entry("EB", hand, seg("09:00", "10:00")))),
			cmd: away("AA-1", "08:30", "10:15"),
			now: "10:50",
			want: state(
				ticket("AA-1", entry("EA", hand, seg("08:00", "08:30"), seg("10:15", "10:30"))),
				ticket("BB-2", entry("EB", hand, seg("09:00", "10:00")))),
			events: []core.Event{{Kind: core.EventPaused, Key: "AA-1", At: at("08:30")}},
		},
		{
			name:   "To empty on a running entry is away until now: it ends paused at From",
			w:      running(),
			cmd:    away("AA-1", "10:30", ""),
			now:    "10:50",
			want:   state(ticket("AA-1", entry("EA", hand, seg("08:58", "10:30")))),
			events: []core.Event{{Kind: core.EventPaused, Key: "AA-1", At: at("10:30")}},
		},
		{
			name:   "To empty on a paused entry removes From to the pause",
			w:      world("10:50", ticket("AA-1", entry("EA", hand, seg("08:00", "09:00"), seg("09:30", "10:30")))),
			cmd:    away("AA-1", "10:00", ""),
			now:    "10:50",
			want:   state(ticket("AA-1", entry("EA", hand, seg("08:00", "09:00"), seg("09:30", "10:00")))),
			events: []core.Event{{Kind: core.EventPaused, Key: "AA-1", At: at("10:00")}},
		},
		{
			name: "a paused entry whose To is after the pause resumes at To: the late lunch",
			w:    world("13:20", ticket("AA-1", entry("EA", hand, seg("09:00", "12:30")))),
			cmd:  away("AA-1", "12:00", "13:00"),
			now:  "13:20",
			want: state(ticket("AA-1", entry("EA", hand, seg("09:00", "12:00"), seg("13:00", "")))),
			events: []core.Event{
				{Kind: core.EventPaused, Key: "AA-1", At: at("12:00")},
				{Kind: core.EventStarted, Key: "AA-1", At: at("13:00")},
			},
		},
		{
			name:   "From at the pause with To after it only resumes at To",
			w:      world("10:50", ticket("AA-1", entry("EA", hand, seg("08:58", "10:30")))),
			cmd:    away("AA-1", "10:30", "10:40"),
			now:    "10:50",
			want:   state(ticket("AA-1", entry("EA", hand, seg("08:58", "10:30"), seg("10:40", "")))),
			events: []core.Event{{Kind: core.EventStarted, Key: "AA-1", At: at("10:40")}},
		},
		{
			name: "it resumes Key's entry, not the most recently paused",
			w: world("10:50",
				ticket("AA-1", entry("EA", hand, seg("08:00", "09:00"))),
				ticket("BB-2", entry("EB", hand, seg("09:00", "10:00")))),
			cmd: away("AA-1", "09:00", "10:30"),
			now: "10:50",
			want: state(
				ticket("AA-1", entry("EA", hand, seg("08:00", "09:00"), seg("10:30", ""))),
				ticket("BB-2", entry("EB", hand, seg("09:00", "10:00")))),
			events: []core.Event{{Kind: core.EventStarted, Key: "AA-1", At: at("10:30")}},
		},
		{
			name: "a window with nothing counted changes nothing",
			w:    world("10:50", ticket("AA-1", entry("EA", hand, seg("08:00", "09:00"), seg("10:00", "10:30")))),
			cmd:  away("AA-1", "09:10", "09:50"),
			now:  "10:50",
			want: state(ticket("AA-1", entry("EA", hand, seg("08:00", "09:00"), seg("10:00", "10:30")))),
		},
		{
			name: "From at the pause and To empty changes nothing",
			w:    world("10:50", ticket("AA-1", entry("EA", hand, seg("08:58", "10:30")))),
			cmd:  away("AA-1", "10:30", ""),
			now:  "10:50",
			want: state(ticket("AA-1", entry("EA", hand, seg("08:58", "10:30")))),
		},
		{
			name: "a trimmed stretch left under a minute is dropped, and the pause says so",
			w:    world("10:50", ticket("AA-1", entry("EA", hand, seg("08:00", "09:00"), seg("09:30", "10:00")))),
			cmd:  core.Away{Key: "AA-1", From: at("08:30").Time, To: at("09:59:20").Time, Origin: hand},
			now:  "10:50",
			want: state(ticket("AA-1", entry("EA", hand, seg("08:00", "08:30")))),
			events: []core.Event{{Kind: core.EventPaused, Key: "AA-1", At: at("08:30"),
				Message: "the 40s from 09:59 was under a minute, so it was dropped"}},
		},
		{
			name: "a stretch under a minute outside the window is left alone",
			w: world("10:50", ticket("AA-1", entry("EA", hand,
				seg("08:00:00", "08:00:30"), seg("09:00", "10:00")))),
			cmd: away("AA-1", "09:30", "09:40"),
			now: "10:50",
			want: state(ticket("AA-1", entry("EA", hand,
				seg("08:00:00", "08:00:30"), seg("09:00", "09:30"), seg("09:40", "10:00")))),
			events: []core.Event{{Kind: core.EventPaused, Key: "AA-1", At: at("09:30")}},
		},
	})
}

func TestAwayRefusals(t *testing.T) {
	running := func() core.World {
		return world("10:50", ticket("AA-1", entry("EA", hand, seg("08:58", ""))))
	}
	runApply(t, []applyCase{
		{name: "From after To", w: running(), cmd: away("AA-1", "10:45", "10:00"), now: "10:50", err: core.ErrUsage},
		{name: "From equal to To", w: running(), cmd: away("AA-1", "10:00", "10:00"), now: "10:50", err: core.ErrUsage},
		{name: "From in the future", w: running(), cmd: away("AA-1", "10:55", ""), now: "10:50", err: core.ErrUsage},
		{name: "To in the future", w: running(), cmd: away("AA-1", "10:00", "10:55"), now: "10:50", err: core.ErrUsage},
		{name: "From before the entry started", w: running(), cmd: away("AA-1", "08:30", "09:30"), now: "10:50", err: core.ErrUsage},
		{name: "no From", w: running(), cmd: core.Away{Key: "AA-1", To: at("10:00").Time, Origin: hand}, now: "10:50", err: core.ErrUsage},
		{
			name: "resuming at To while another ticket runs",
			w: world("10:50",
				ticket("AA-1", entry("EA", hand, seg("08:00", "09:00"))),
				ticket("BB-2", entry("EB", hand, seg("09:00", "")))),
			cmd: away("AA-1", "08:30", "10:30"),
			now: "10:50",
			err: core.ErrUsage,
		},
		{
			name: "resuming at To over another ticket's time after To",
			w: world("10:50",
				ticket("AA-1", entry("EA", hand, seg("08:00", "09:00"))),
				ticket("BB-2", entry("EB", hand, seg("09:00", "10:00")))),
			cmd: away("AA-1", "08:30", "09:30"),
			now: "10:50",
			err: core.ErrUsage,
		},
		{name: "nothing open", w: world("10:50", ticket("AA-1")), cmd: away("AA-1", "10:00", ""), now: "10:50", err: core.ErrNoTimer},
		{
			name: "an agent on a timer set by hand",
			w:    running(),
			cmd:  core.Away{Key: "AA-1", From: at("10:00").Time, Origin: agent},
			now:  "10:50",
			err:  core.ErrByHand,
		},
	})
}

func TestAwayRefusalsSayWhy(t *testing.T) {
	w := world("10:50", ticket("AA-1", entry("EA", hand, seg("08:58", ""))))
	for _, tc := range []struct {
		cmd  core.Away
		want string
	}{
		{away("AA-1", "10:45", "10:00"), "10:45 is not before 10:00"},
		{away("AA-1", "10:55", ""), "10:55 is in the future"},
		{away("AA-1", "08:00", "09:00"), "08:00 is before the entry started at 08:58"},
	} {
		_, _, err := core.Apply(w, tc.cmd, at("10:50").Time)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s–%s: err = %v, want %q", tc.cmd.From.Format("15:04"), tc.cmd.To.Format("15:04"), err, tc.want)
		}
	}
}

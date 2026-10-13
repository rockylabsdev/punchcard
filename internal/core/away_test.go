package core_test

import (
	"testing"

	"github.com/j-dsmith/punchcard/internal/core"
)

func TestAway(t *testing.T) {
	running := func() core.World {
		return world("10:50", ticket("AA-1", entry("EA", hand, seg("08:58", ""))))
	}
	runApply(t, []applyCase{
		{
			name: "away cuts From to To out of the running entry in one change",
			w:    running(),
			cmd:  core.Away{Key: "AA-1", From: at("10:00").Time, To: at("10:45").Time, Origin: hand},
			now:  "10:50",
			want: state(ticket("AA-1", entry("EA", hand, cut("08:58", "10:00"), seg("10:45", "")))),
			events: []core.Event{
				{Kind: core.EventPaused, Key: "AA-1", At: at("10:00")},
				{Kind: core.EventStarted, Key: "AA-1", At: at("10:45")},
			},
		},
		{
			name:   "away with no To is a backdated pause",
			w:      running(),
			cmd:    core.Away{Key: "AA-1", From: at("10:30").Time, Origin: hand},
			now:    "10:50",
			want:   state(ticket("AA-1", entry("EA", hand, cut("08:58", "10:30")))),
			events: []core.Event{{Kind: core.EventPaused, Key: "AA-1", At: at("10:30")}},
		},
		{
			name:   "away with no From resumes the paused entry at To, the pause left where it is",
			w:      world("10:50", ticket("AA-1", entry("EA", hand, seg("08:58", "10:30")))),
			cmd:    core.Away{Key: "AA-1", To: at("10:40").Time, Origin: hand},
			now:    "10:50",
			want:   state(ticket("AA-1", entry("EA", hand, seg("08:58", "10:30"), seg("10:40", "")))),
			events: []core.Event{{Kind: core.EventStarted, Key: "AA-1", At: at("10:40")}},
		},
		{
			name: "away resumes Key's entry, not the most recently paused",
			w: world("10:50",
				ticket("AA-1", entry("EA", hand, seg("08:00", "09:00"))),
				ticket("BB-2", entry("EB", hand, seg("09:00", "10:00")))),
			cmd: core.Away{Key: "AA-1", To: at("10:30").Time, Origin: hand},
			now: "10:50",
			want: state(
				ticket("AA-1", entry("EA", hand, seg("08:00", "09:00"), seg("10:30", ""))),
				ticket("BB-2", entry("EB", hand, seg("09:00", "10:00")))),
			events: []core.Event{{Kind: core.EventStarted, Key: "AA-1", At: at("10:30")}},
		},
		{
			name: "away's To is a resume: refused while another ticket runs",
			w: world("10:50",
				ticket("AA-1", entry("EA", hand, seg("08:00", "09:00"))),
				ticket("BB-2", entry("EB", hand, seg("09:00", "")))),
			cmd: core.Away{Key: "AA-1", To: at("10:30").Time, Origin: hand},
			now: "10:50",
			err: core.ErrUsage,
		},
		{
			name: "a refused To refuses the whole cut: the pause at From is not kept",
			w:    running(),
			cmd:  core.Away{Key: "AA-1", From: at("10:45").Time, To: at("10:00").Time, Origin: hand},
			now:  "10:50",
			err:  core.ErrUsage,
		},
		{
			name: "away on a ticket with nothing open is refused",
			w:    world("10:50", ticket("AA-1")),
			cmd:  core.Away{Key: "AA-1", To: at("10:00").Time, Origin: hand},
			now:  "10:50",
			err:  core.ErrNoTimer,
		},
	})
}

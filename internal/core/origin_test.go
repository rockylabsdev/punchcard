package core_test

import (
	"testing"

	"github.com/j-dsmith/punchcard/internal/core"
)

// The entry's origin names who made its last change, so a TUI that
// reads the file afterwards can say "paused by agent".
func TestApplyRecordsWhoChangedTheEntry(t *testing.T) {
	runApply(t, []applyCase{
		{
			name: "pause records who paused",
			w:    world("09:30", ticket("AA-1", entry("EA", prompt, seg("09:00", "")))),
			cmd:  core.Pause{Origin: agent},
			now:  "09:30",
			want: state(ticket("AA-1", entry("EA", agent, seg("09:00", "09:30")))),
		},
		{
			name: "a hand pause of a prompt timer does not make it held",
			w:    world("09:30", ticket("AA-1", entry("EA", prompt, seg("09:00", "")))),
			cmd:  core.Pause{Origin: hand},
			now:  "09:30",
			want: state(ticket("AA-1", withOrigin(entry("EA", prompt, seg("09:00", "09:30")), hand))),
		},
		{
			name: "starting another ticket records who paused this one",
			w:    world("09:30", ticket("AA-1", entry("EA", prompt, seg("09:00", ""))), ticket("BB-2")),
			cmd:  core.Start{Key: "BB-2", Origin: agent, NewEntryID: "EB"},
			now:  "09:30",
			want: state(
				ticket("AA-1", entry("EA", agent, seg("09:00", "09:30"))),
				ticket("BB-2", entry("EB", agent, seg("09:30", "")))),
			events: []core.Event{{Kind: core.EventPaused, Key: "AA-1"}},
		},
		{
			name: "stop records who stopped",
			w:    world("09:30", ticket("AA-1", entry("EA", prompt, seg("09:00", "")))),
			cmd:  core.Stop{Origin: hand},
			now:  "09:30",
			want: state(ticket("AA-1", withWorklog(withOrigin(entry("EA", prompt, seg("09:00", "09:30")), hand), pending("09:00", 1800, "")))),
		},
	})
}

func withOrigin(e core.TimeEntry, o core.Origin) core.TimeEntry { e.Origin = o; return e }

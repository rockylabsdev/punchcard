package core_test

import (
	"testing"

	"github.com/j-dsmith/punchcard/internal/core"
)

func TestApplyPauseResume(t *testing.T) {
	runApply(t, []applyCase{
		{
			name: "pause closes the open segment",
			w:    world("09:30", ticket("AA-1", entry("EA", hand, seg("09:00", "")))),
			cmd:  core.Pause{Origin: hand},
			now:  "09:30",
			want: state(ticket("AA-1", entry("EA", hand, seg("09:00", "09:30")))),
		},
		{
			name: "pause with nothing running is refused",
			w:    world("09:30", ticket("AA-1", entry("EA", hand, seg("09:00", "09:10")))),
			cmd:  core.Pause{Origin: hand},
			now:  "09:30",
			err:  core.ErrNoTimer,
		},
		{
			name: "resume continues the same entry with a new segment",
			w:    world("10:00", ticket("AA-1", entry("EA", hand, seg("09:00", "09:30")))),
			cmd:  core.Resume{Origin: hand},
			now:  "10:00",
			want: state(ticket("AA-1", entry("EA", hand, seg("09:00", "09:30"), seg("10:00", "")))),
		},
		{
			name: "resume with no key picks the most recently paused entry",
			w: world("10:00",
				ticket("AA-1", entry("EA", hand, seg("09:00", "09:40"))),
				ticket("BB-2", entry("EB", hand, seg("09:40", "09:50")))),
			cmd: core.Resume{Origin: agent},
			now: "10:00",
			want: state(
				ticket("AA-1", entry("EA", hand, seg("09:00", "09:40"))),
				ticket("BB-2", entry("EB", agent, seg("09:40", "09:50"), seg("10:00", "")))),
		},
		{
			name: "resume KEY is start KEY",
			w: world("10:00",
				ticket("AA-1", entry("EA", hand, seg("09:00", "09:40"))),
				ticket("BB-2", entry("EB", hand, seg("09:40", ""))),
			),
			cmd: core.Resume{Key: "AA-1", Origin: hand},
			now: "10:00",
			want: state(
				ticket("AA-1", entry("EA", hand, seg("09:00", "09:40"), seg("10:00", ""))),
				ticket("BB-2", entry("EB", hand, seg("09:40", "10:00")))),
			events: []core.Event{{Kind: core.EventPaused, Key: "BB-2"}},
		},
		{
			name: "resume with nothing paused is refused",
			w:    world("10:00", ticket("AA-1")),
			cmd:  core.Resume{Origin: hand},
			now:  "10:00",
			err:  core.ErrNoTimer,
		},
		{
			name: "resume with no key while running is a no-op",
			w:    world("10:00", ticket("AA-1", entry("EA", hand, seg("09:00", "")))),
			cmd:  core.Resume{Origin: hand},
			now:  "10:00",
			want: state(ticket("AA-1", entry("EA", hand, seg("09:00", "")))),
		},
	})
}

func TestApplyByHand(t *testing.T) {
	byHandA := func() core.World {
		return world("09:30", ticket("AA-1", entry("EA", hand, seg("09:00", ""))), ticket("BB-2"))
	}
	runApply(t, []applyCase{
		{name: "an agent cannot pause a timer set by hand", w: byHandA(), cmd: core.Pause{Origin: agent}, now: "09:30", err: core.ErrByHand},
		{name: "a prompt's pause cannot pause a timer set by hand", w: byHandA(), cmd: core.Pause{Origin: prompt}, now: "09:30", err: core.ErrByHand},
		{name: "an agent cannot move a timer set by hand", w: byHandA(), cmd: core.Start{Key: "BB-2", Origin: agent, NewEntryID: "EB"}, now: "09:30", err: core.ErrByHand},
		{name: "a prompt's start cannot move a timer set by hand", w: byHandA(), cmd: core.Start{Key: "BB-2", Origin: prompt, NewEntryID: "EB"}, now: "09:30", err: core.ErrByHand},
		{
			name: "another act by hand moves a timer set by hand",
			w:    byHandA(),
			cmd:  core.Start{Key: "BB-2", Origin: hand, NewEntryID: "EB"},
			now:  "09:30",
			want: state(
				ticket("AA-1", entry("EA", hand, seg("09:00", "09:30"))),
				ticket("BB-2", entry("EB", hand, seg("09:30", "")))),
			events: []core.Event{{Kind: core.EventPaused, Key: "AA-1"}},
		},
		{
			name: "an agent moves a timer not set by hand",
			w:    world("09:30", ticket("AA-1", entry("EA", agent, seg("09:00", ""))), ticket("BB-2")),
			cmd:  core.Start{Key: "BB-2", Origin: agent, NewEntryID: "EB"},
			now:  "09:30",
			want: state(
				ticket("AA-1", entry("EA", agent, seg("09:00", "09:30"))),
				ticket("BB-2", entry("EB", agent, seg("09:30", "")))),
			events: []core.Event{{Kind: core.EventPaused, Key: "AA-1"}},
		},
		{name: "an agent cannot resume another ticket over a timer set by hand", w: byHandA(), cmd: core.Resume{Key: "BB-2", Origin: agent, NewEntryID: "EB"}, now: "09:30", err: core.ErrByHand},
		{
			name: "an agent resuming a paused entry once set by hand leaves it not by hand",
			w:    world("10:00", ticket("AA-1", entry("EA", hand, seg("09:00", "09:30")))),
			cmd:  core.Resume{Origin: agent},
			now:  "10:00",
			want: state(ticket("AA-1", entry("EA", agent, seg("09:00", "09:30"), seg("10:00", "")))),
		},
	})
}

package core_test

import (
	"testing"

	"github.com/j-dsmith/punchcard/internal/core"
)

func TestApplyVerify(t *testing.T) {
	unverified := func(tk core.TrackedTicket) core.TrackedTicket { tk.Unverified = true; return tk }
	summarised := func(tk core.TrackedTicket, s string) core.TrackedTicket { tk.Summary = s; return tk }
	running := entry("EA", hand, seg("09:00", ""))

	runApply(t, []applyCase{
		{
			name: "verify confirms an unverified ticket with its summary",
			w:    world("09:00", unverified(ticket("AA-1"))),
			cmd:  core.Verify{Key: "AA-1", Summary: "Fix the thing"},
			now:  "09:05",
			want: state(summarised(ticket("AA-1"), "Fix the thing")),
		},
		{
			name: "verify refreshes a confirmed ticket's summary",
			w:    world("09:00", summarised(ticket("AA-1"), "Old")),
			cmd:  core.Verify{Key: "AA-1", Summary: "New"},
			now:  "09:05",
			want: state(summarised(ticket("AA-1"), "New")),
		},
		{
			name: "verify an untracked ticket is refused",
			w:    world("09:00"),
			cmd:  core.Verify{Key: "AA-1", Summary: "x"},
			now:  "09:05", err: core.ErrNotTracked,
		},
		{
			name: "start with a confirmed summary confirms an unverified ticket already on the list",
			w:    world("09:00", unverified(ticket("AA-1"))),
			cmd:  core.Start{Key: "AA-1", Summary: "Fix the thing", Origin: hand, NewEntryID: "EA"},
			now:  "09:00",
			want: state(summarised(ticket("AA-1", running), "Fix the thing")),
		},
		{
			name: "add with a confirmed summary confirms an unverified ticket already on the list",
			w:    world("09:00", unverified(ticket("AA-1"))),
			cmd:  core.Add{Tickets: []core.NewTicket{{Key: "AA-1", Summary: "Fix the thing"}}, Origin: hand},
			now:  "09:00",
			want: state(summarised(ticket("AA-1"), "Fix the thing")),
		},
		{
			name: "start on the running unverified ticket confirms it",
			w:    world("09:00", unverified(ticket("AA-1", running))),
			cmd:  core.Start{Key: "AA-1", Summary: "Fix the thing", Origin: hand, NewEntryID: "EB"},
			now:  "09:05",
			want: state(summarised(ticket("AA-1", running), "Fix the thing")),
		},
		{
			name: "an unverified start leaves a tracked ticket's summary alone",
			w:    world("09:00", summarised(ticket("AA-1"), "Kept")),
			cmd:  core.Start{Key: "AA-1", Unverified: true, Origin: hand, NewEntryID: "EA"},
			now:  "09:00",
			want: state(summarised(ticket("AA-1", running), "Kept")),
		},
	})
}

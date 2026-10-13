package tui

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/j-dsmith/punchcard/internal/core"
)

// meetingCut is PUN-1 from 08:58 with 10:00–10:45 cut out by the CLI.
func meetingCut(hs *harness) {
	hs.setup("08:58:00", add("PUN-1"), start("PUN-1", "E1"))
	hs.setup("10:50:00",
		core.Pause{Origin: core.OriginHand, At: at("10:00:00")},
		core.Start{Key: "PUN-1", Origin: core.OriginHand, At: at("10:45:00")})
}

func TestTicketPaneShowsACutAsTimeAway(t *testing.T) {
	hs := newHarness(t)
	meetingCut(hs)
	hs.open("11:00:00", 120, 32)

	lines := paneText(t, hs.frame(), 120)
	wantLines(t, lines,
		"time away is left out",
		"10:45–now   0:15:00",
		"10:00–10:45 ‖ away 45m",
		"08:58–10:00      1h 02m",
	)
	noCounted(t, hs.frame())
	if containsLine(lines, "pauses are left out") || containsLine(lines, "pauses and time away are left out") {
		t.Errorf("a cut is time away, not a pause:\n%s", hs.frame())
	}
}

func TestTicketPaneSaysPausesAndACutAreLeftOut(t *testing.T) {
	hs := newHarness(t)
	meetingCut(hs)
	hs.setup("10:55:00", pause)
	hs.setup("11:00:00", start("PUN-1", "E1"))
	hs.open("11:05:00", 120, 32)

	wantLines(t, paneText(t, hs.frame(), 120),
		"pauses and time away are left out",
		"10:55–11:00 ‖ paused 5m",
		"10:00–10:45 ‖ away 45m",
	)
}

func TestAwayDialogCutsAMeetingOutOfARunningEntry(t *testing.T) {
	hs := newHarness(t)
	hs.setup("08:58:00", add("PUN-1"), start("PUN-1", "E1"))
	hs.open("10:50:00", 120, 32)

	hs.key("-")
	frameHas(t, hs, "Away from PUN-1", "running since 08:58", "From", "To")
	hs.typeText("10:00")
	hs.key("tab")
	hs.typeText("10:45")
	frameHas(t, hs, "→ cuts 45m · entry becomes 1:07")
	hs.st.commands()
	hs.key("enter")

	want := []core.Command{
		core.Pause{Key: "PUN-1", Origin: core.OriginHand, At: at("10:00:00")},
		core.Start{Key: "PUN-1", Summary: "Summary of PUN-1", Origin: core.OriginHand, At: at("10:45:00"), NewEntryID: "ID-1"},
	}
	if d := cmp.Diff(want, hs.st.commands(), anyGrammar); d != "" {
		t.Fatalf("commands (-want +got)\n%s", d)
	}
	frameLacks(t, hs, "Away from PUN-1")
	wantLines(t, paneText(t, hs.frame(), 120), "10:00–10:45 ‖ away 45m")
}

func TestAwayDialogWithNoToIsABackdatedPause(t *testing.T) {
	hs := newHarness(t)
	hs.setup("08:58:00", add("PUN-1"), start("PUN-1", "E1"))
	hs.open("10:50:00", 120, 32)

	hs.key("-")
	hs.typeText("-20m")
	frameHas(t, hs, "→ cuts 20m · entry becomes 1:32")
	hs.st.commands()
	hs.key("enter")
	want := []core.Command{core.Pause{Key: "PUN-1", Origin: core.OriginHand, At: at("10:30:00")}}
	if d := cmp.Diff(want, hs.st.commands(), anyGrammar); d != "" {
		t.Fatalf("commands (-want +got)\n%s", d)
	}
	if hs.running() != "" {
		t.Errorf("still running")
	}
}

func TestAwayDialogOnAPausedEntryMovesThePause(t *testing.T) {
	hs := newHarness(t)
	hs.setup("08:58:00", add("PUN-1"), start("PUN-1", "E1"))
	hs.setup("10:30:00", pause)
	hs.open("10:50:00", 120, 32)

	hs.key("-")
	frameHas(t, hs, "paused at 10:30", "10:30")
	for range len("10:30") {
		hs.key("backspace")
	}
	hs.typeText("10:00")
	frameHas(t, hs, "→ cuts 30m · entry becomes 1:02")
}

func TestAwayDialogSaysWhyItCannotCut(t *testing.T) {
	hs := newHarness(t)
	hs.setup("08:58:00", add("PUN-1"), start("PUN-1", "E1"))
	hs.open("10:50:00", 120, 32)

	hs.key("-")
	hs.typeText("08:30")
	frameHas(t, hs, "✗ 08:30 is before the entry started at 08:58")
	hs.key("backspace", "backspace", "backspace", "backspace", "backspace")
	hs.typeText("10:45")
	hs.key("tab")
	hs.typeText("10:00")
	frameHas(t, hs, "✗ 10:00 is before 10:45")
	hs.st.commands()
	hs.key("enter")
	for _, c := range hs.st.commands() {
		if _, ok := c.(core.Touch); !ok {
			t.Errorf("an invalid cut applied %#v", c)
		}
	}
	frameHas(t, hs, "Away from PUN-1")
}

func TestAwayKeyNeedsAnOpenEntry(t *testing.T) {
	hs := newHarness(t)
	hs.setup("08:00:00", add("PUN-1"))
	hs.open("09:00:00", 120, 32)
	hs.key("-")
	frameLacks(t, hs, "Away from")
}

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

func TestTicketPaneShowsACutAsAPause(t *testing.T) {
	hs := newHarness(t)
	meetingCut(hs)
	hs.open("11:00:00", 120, 32)

	lines := paneText(t, hs.frame(), 120)
	wantLines(t, lines,
		"pauses are left out",
		"10:45–now   0:15:00",
		"10:00–10:45 ‖ paused 45m",
		"08:58–10:00      1h 02m",
	)
	noCounted(t, hs.frame())
	if containsLine(lines, "time away is left out") || containsLine(lines, "pauses and time away are left out") {
		t.Errorf("a gap the user made is a pause, not time away:\n%s", hs.frame())
	}
}

func TestTicketPaneSaysPausesForACutAndAPlainPause(t *testing.T) {
	hs := newHarness(t)
	meetingCut(hs)
	hs.setup("10:55:00", pause)
	hs.setup("11:00:00", start("PUN-1", "E1"))
	hs.open("11:05:00", 120, 32)

	wantLines(t, paneText(t, hs.frame(), 120),
		"pauses are left out",
		"10:55–11:00 ‖ paused 5m",
		"10:00–10:45 ‖ paused 45m",
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
		core.Away{Key: "PUN-1", From: at("10:00:00"), To: at("10:45:00"), Origin: core.OriginHand},
	}
	if d := cmp.Diff(want, hs.st.commands(), anyGrammar); d != "" {
		t.Fatalf("commands (-want +got)\n%s", d)
	}
	frameLacks(t, hs, "Away from PUN-1", "cut time away")
	frameHas(t, hs, "paused PUN-1 10:00–10:45")
	wantLines(t, paneText(t, hs.frame(), 120), "10:00–10:45 ‖ paused 45m")
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
	want := []core.Command{core.Away{Key: "PUN-1", From: at("10:30:00"), Origin: core.OriginHand}}
	if d := cmp.Diff(want, hs.st.commands(), anyGrammar); d != "" {
		t.Fatalf("commands (-want +got)\n%s", d)
	}
	if hs.running() != "" {
		t.Errorf("still running")
	}
	frameHas(t, hs, "paused PUN-1 at 10:30")
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
	hs.key("enter")
	frameHas(t, hs, "paused PUN-1 at 10:00")
}

func TestAwayDialogToIsAResumeNotASwitch(t *testing.T) {
	hs := newHarness(t)
	hs.setup("08:58:00", add("PUN-1"), add("PUN-2"), start("PUN-1", "E1"))
	hs.setup("10:00:00", start("PUN-2", "E2"))
	hs.open("10:50:00", 120, 32)

	hs.key("down", "-") // PUN-1, paused at 10:00 while PUN-2 runs
	frameHas(t, hs, "Away from PUN-1", "paused at 10:00")
	hs.key("tab")
	hs.typeText("10:30")
	frameHas(t, hs, "✗ PUN-2 is running; resume at a time reopens")
	hs.st.commands()
	hs.key("enter")
	for _, c := range hs.st.commands() {
		if _, ok := c.(core.Touch); !ok {
			t.Errorf("a refused cut applied %#v", c)
		}
	}
	if hs.running() != "PUN-2" {
		t.Errorf("running %q, want PUN-2", hs.running())
	}
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
	frameHas(t, hs, "✗ 10:45 is not before 10:00")
	hs.st.commands()
	hs.key("enter")
	for _, c := range hs.st.commands() {
		if _, ok := c.(core.Touch); !ok {
			t.Errorf("an invalid cut applied %#v", c)
		}
	}
	frameHas(t, hs, "Away from PUN-1")
}

func TestAwayDialogRemovesAWindowOverAGap(t *testing.T) {
	hs := newHarness(t)
	hs.setup("20:00:00", add("PUN-1"), start("PUN-1", "E1"))
	hs.setup("20:50:00", pause)
	hs.setup("21:06:00", start("PUN-1", "E1"))
	hs.setup("21:12:00", pause)
	hs.open("21:30:00", 120, 32)

	hs.key("-")
	frameHas(t, hs, "paused at 21:12")
	for range len("21:12") {
		hs.key("backspace")
	}
	hs.typeText("20:43")
	hs.key("tab")
	hs.typeText("21:07")
	frameHas(t, hs, "→ cuts 8m · entry becomes 0:48")
	hs.st.commands()
	hs.key("enter")

	want := []core.Command{
		core.Away{Key: "PUN-1", From: at("20:43:00"), To: at("21:07:00"), Origin: core.OriginHand},
	}
	if d := cmp.Diff(want, hs.st.commands(), anyGrammar); d != "" {
		t.Fatalf("commands (-want +got)\n%s", d)
	}
	frameHas(t, hs, "paused PUN-1 20:43–21:07")
	wantLines(t, paneText(t, hs.frame(), 120),
		"21:07–21:12      5m",
		"20:43–21:07 ‖ paused 24m",
		"20:00–20:43      43m",
	)
}

func TestAwayDialogLateLunchCutsAndResumes(t *testing.T) {
	hs := newHarness(t)
	hs.setup("11:00:00", add("PUN-1"), start("PUN-1", "E1"))
	hs.setup("12:30:00", pause)
	hs.open("13:20:00", 120, 32)

	hs.key("-")
	for range len("12:30") {
		hs.key("backspace")
	}
	hs.typeText("12:00")
	hs.key("tab")
	hs.typeText("13:00")
	frameHas(t, hs, "→ cuts 30m · entry becomes 1:20")
	hs.key("enter")
	if hs.running() != "PUN-1" {
		t.Errorf("running %q, want PUN-1 from 13:00", hs.running())
	}
	frameHas(t, hs, "paused PUN-1 12:00–13:00")
}

func TestAwayDialogFromThePauseOnlyResumes(t *testing.T) {
	hs := newHarness(t)
	hs.setup("09:00:00", add("PUN-1"), start("PUN-1", "E1"))
	hs.setup("10:30:00", pause)
	hs.open("10:50:00", 120, 32)

	hs.key("-", "tab")
	hs.typeText("10:40")
	frameHas(t, hs, "→ adds 10m · entry becomes 1:40")
	hs.key("enter")
	if hs.running() != "PUN-1" {
		t.Errorf("running %q, want PUN-1 from 10:40", hs.running())
	}
}

func TestAwayDialogWithNothingCountedJustCloses(t *testing.T) {
	hs := newHarness(t)
	hs.setup("08:00:00", add("PUN-1"), start("PUN-1", "E1"))
	hs.setup("09:00:00", pause)
	hs.setup("10:00:00", start("PUN-1", "E1"))
	hs.setup("10:30:00", pause)
	hs.open("10:50:00", 120, 32)

	hs.key("-")
	frameHas(t, hs, "nothing counted between 10:30 and 10:50")
	for range len("10:30") {
		hs.key("backspace")
	}
	hs.typeText("09:10")
	hs.key("tab")
	hs.typeText("09:50")
	frameHas(t, hs, "nothing counted between 09:10 and 09:50")
	hs.st.commands()
	hs.key("enter")
	for _, c := range hs.st.commands() {
		if _, ok := c.(core.Touch); !ok {
			t.Errorf("nothing to cut applied %#v", c)
		}
	}
	frameLacks(t, hs, "Away from PUN-1", "paused PUN-1")
}

func TestAwayKeyNeedsAnOpenEntry(t *testing.T) {
	hs := newHarness(t)
	hs.setup("08:00:00", add("PUN-1"))
	hs.open("09:00:00", 120, 32)
	hs.key("-")
	frameLacks(t, hs, "Away from")
}

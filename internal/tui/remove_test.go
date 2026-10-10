package tui

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/j-dsmith/punchcard/internal/core"
)

func TestRemoveConfirmsWithY(t *testing.T) {
	hs := newHarness(t)
	hs.setup("07:00:00", add("AA-1", "BB-2"))
	hs.open("09:00:00", 120, 32)

	hs.key("d")
	frameHas(t, hs, "Remove AA-1?", "y removes it")
	frameLacks(t, hs, "would be lost")
	hs.key("y")

	want := []core.Command{core.Remove{Key: "AA-1", Origin: core.OriginHand}}
	if d := cmp.Diff(want, changes(hs.st.commands()), anyGrammar); d != "" {
		t.Errorf("changes (-want +got)\n%s", d)
	}
	frameLacks(t, hs, "Remove AA-1?", "AA-1 Summary")
	frameHas(t, hs, "removed AA-1")
}

func TestRemoveAnyOtherKeyKeepsIt(t *testing.T) {
	hs := newHarness(t)
	hs.setup("07:00:00", add("AA-1"))
	hs.open("09:00:00", 120, 32)

	hs.key("d", "n")

	if got := changes(hs.st.commands()); len(got) != 0 {
		t.Errorf("changed %v", got)
	}
	frameLacks(t, hs, "Remove AA-1?")
	frameHas(t, hs, "AA-1 Summary")
}

// A local worklog is logged: removing its ticket loses nothing.
func TestRemoveDoesNotWarnAboutLocalTime(t *testing.T) {
	hs := newHarness(t)
	hs.setup("07:00:00", add("AA-1"), start("AA-1", "A"))
	hs.setup("07:20:00", core.Stop{Origin: core.OriginHand, Local: true})
	hs.open("09:00:00", 120, 32)

	hs.key("d")
	frameHas(t, hs, "Remove AA-1?", "y removes it")
	frameLacks(t, hs, "would be lost")
	hs.key("y")

	want := []core.Command{core.Remove{Key: "AA-1", Origin: core.OriginHand}}
	if d := cmp.Diff(want, changes(hs.st.commands()), anyGrammar); d != "" {
		t.Errorf("changes (-want +got)\n%s", d)
	}
}

func TestRemoveWarnsWhenUnsentTimeWouldBeLost(t *testing.T) {
	hs := newHarness(t)
	hs.setup("07:00:00", add("AA-1"), start("AA-1", "A"))
	hs.setup("07:20:00", core.Stop{Origin: core.OriginHand}) // 20m owed
	hs.setup("08:00:00", start("AA-1", "B"))
	hs.setup("08:15:00", pause) // 15m open
	hs.open("09:00:00", 120, 32)

	hs.key("d")
	frameHas(t, hs, "Remove AA-1?", "its unsent 35m would be lost")
	hs.key("y")

	want := []core.Command{core.Remove{Key: "AA-1", Force: true, Origin: core.OriginHand}}
	if d := cmp.Diff(want, changes(hs.st.commands()), anyGrammar); d != "" {
		t.Errorf("changes (-want +got)\n%s", d)
	}
}

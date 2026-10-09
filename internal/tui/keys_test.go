package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"
	"github.com/j-dsmith/punchcard/internal/core"
)

func TestSpaceStartsTheSelectedRowThroughTheStore(t *testing.T) {
	hs := newHarness(t)
	hs.setup("08:00:00", add("AA-1"))
	hs.open("09:00:00", 80, 24)

	hs.key("space")

	want := []core.Command{core.Start{Key: "AA-1", Summary: "Summary of AA-1", Origin: core.OriginHand, NewEntryID: "ID-1"}}
	if diff := cmp.Diff(want, hs.st.commands(), anyGrammar); diff != "" {
		t.Errorf("commands (-want +got):\n%s", diff)
	}
	if got := hs.running(); got != "AA-1" {
		t.Errorf("running = %q, want AA-1", got)
	}
}

func TestKeys(t *testing.T) {
	hand := core.OriginHand
	// AA-1 BB-2 CC-3 are added at 07:00; recents order puts the most
	// recently active first, ties by key.
	tests := []struct {
		name        string
		setup       []core.Command // at 08:00
		keys        []string
		wantCmds    []core.Command
		wantRunning string
		wantSel     string // the ticket under the cursor afterwards
	}{
		{
			name:        "space on another row pauses the running one and starts it",
			setup:       []core.Command{start("AA-1", "E-A")},
			keys:        []string{"j", "space"},
			wantCmds:    []core.Command{core.Touch{Origin: hand}, core.Start{Key: "BB-2", Summary: "Summary of BB-2", Origin: hand, NewEntryID: "ID-1"}},
			wantRunning: "BB-2",
			wantSel:     "BB-2", // the cursor follows it to the top
		},
		{
			name:     "space on the running row pauses it",
			setup:    []core.Command{start("AA-1", "E-A")},
			keys:     []string{"space"},
			wantCmds: []core.Command{core.Pause{Origin: hand}},
			wantSel:  "AA-1",
		},
		{
			name:        "enter on a paused row resumes it",
			setup:       []core.Command{start("AA-1", "E-A"), pause},
			keys:        []string{"enter"},
			wantCmds:    []core.Command{core.Start{Key: "AA-1", Summary: "Summary of AA-1", Origin: hand, NewEntryID: "ID-1"}},
			wantRunning: "AA-1",
			wantSel:     "AA-1",
		},
		{
			name:     "p pauses whatever runs from any row",
			setup:    []core.Command{start("AA-1", "E-A")},
			keys:     []string{"G", "p"},
			wantCmds: []core.Command{core.Touch{Origin: hand}, core.Pause{Origin: hand}},
			wantSel:  "CC-3",
		},
		{
			name:     "movement is presence, sent once per throttle window",
			keys:     []string{"j", "j", "k", "down", "up", "g", "G"},
			wantCmds: []core.Command{core.Touch{Origin: hand}},
			wantSel:  "CC-3",
		},
		{
			name:     "the cursor stops at both ends",
			keys:     []string{"k", "up", "G", "j", "down"},
			wantCmds: []core.Command{core.Touch{Origin: hand}},
			wantSel:  "CC-3",
		},
		{
			name:        "q asks the store for nothing and leaves the timer running",
			setup:       []core.Command{start("AA-1", "E-A")},
			keys:        []string{"q"},
			wantRunning: "AA-1",
			wantSel:     "AA-1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hs := newHarness(t)
			hs.setup("07:00:00", add("AA-1", "BB-2", "CC-3"))
			hs.setup("08:00:00", tt.setup...)
			hs.open("09:00:00", 80, 24)

			hs.key(tt.keys...)

			if diff := cmp.Diff(tt.wantCmds, hs.st.commands(), anyGrammar); diff != "" {
				t.Errorf("commands (-want +got):\n%s", diff)
			}
			if got := hs.running(); got != tt.wantRunning {
				t.Errorf("running = %q, want %q", got, tt.wantRunning)
			}
			if got, _ := hs.m.selected(); got.Key != tt.wantSel {
				t.Errorf("cursor on %q, want %q", got.Key, tt.wantSel)
			}
		})
	}
}

func TestQuitReturnsTeaQuit(t *testing.T) {
	hs := newHarness(t)
	hs.open("09:00:00", 80, 24)
	_, cmd := hs.m.Update(keyPress("q"))
	if cmd == nil {
		t.Fatal("q returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("q's command is not tea.Quit")
	}
}

// Two presses before the store answers: the second is resolved against
// the first one's result, so space-space starts then pauses.
func TestKeysWaitForTheStoreInOrder(t *testing.T) {
	hs := newHarness(t)
	hs.setup("08:00:00", add("AA-1"))
	hs.open("09:00:00", 80, 24)

	m1, first := hs.m.Update(keyPress("space"))
	m2, second := m1.Update(keyPress("space"))
	if second != nil {
		t.Fatal("second press went to the store while the first was in flight")
	}
	hs.m = m2.(Model)
	hs.run(first)

	want := []core.Command{
		core.Start{Key: "AA-1", Summary: "Summary of AA-1", Origin: core.OriginHand, NewEntryID: "ID-1"},
		core.Pause{Origin: core.OriginHand},
	}
	if diff := cmp.Diff(want, hs.st.commands(), anyGrammar); diff != "" {
		t.Errorf("commands (-want +got):\n%s", diff)
	}
	if got := hs.running(); got != "" {
		t.Errorf("running = %q, want nothing", got)
	}
}

// A refusal leaves the store alone and says why in the status line.
func TestPauseWithNothingRunningSaysSo(t *testing.T) {
	hs := newHarness(t)
	hs.setup("08:00:00", add("AA-1"))
	hs.open("09:00:00", 80, 24)

	hs.key("p")

	if statusText(hs) != "nothing is running" {
		t.Errorf("status = %q", statusText(hs))
	}
}

// A busy lock on a presence touch is skipped silently; on a change it
// is reported and the last good World stays on screen.
func TestBusyLock(t *testing.T) {
	hs := newHarness(t)
	hs.setup("08:00:00", add("AA-1", "BB-2"))
	hs.open("09:00:00", 80, 24)
	hs.st.busy = true

	hs.key("j")
	if strings.Contains(statusText(hs), "another punchcard") {
		t.Errorf("status after a busy touch = %q, want empty", statusText(hs))
	}
	hs.key("space")
	if !strings.Contains(statusText(hs), "another punchcard") {
		t.Errorf("a busy start said nothing")
	}
	if got, _ := hs.m.selected(); got.Key != "BB-2" {
		t.Errorf("cursor on %q, want BB-2", got.Key)
	}
}

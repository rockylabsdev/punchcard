package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/tracker"
)

func frameHas(t *testing.T, hs *harness, want ...string) {
	t.Helper()
	f := squash(hs.frame())
	for _, w := range want {
		if !strings.Contains(f, squash(w)) {
			t.Errorf("frame has no %q:\n%s", w, hs.frame())
		}
	}
}

func frameLacks(t *testing.T, hs *harness, unwanted ...string) {
	t.Helper()
	f := squash(hs.frame())
	for _, w := range unwanted {
		if strings.Contains(f, squash(w)) {
			t.Errorf("frame still has %q:\n%s", w, hs.frame())
		}
	}
}

// running opens the TUI on PUN-1 running since 08:00, at 09:12:34.
func running(t *testing.T) *harness {
	hs := newHarness(t)
	hs.setup("08:00:00", add("PUN-1"), start("PUN-1", "E1"))
	hs.open("09:12:00", 120, 32)
	hs.tick("09:12:34")
	return hs
}

func TestStopDialogSendsTheMeasuredDuration(t *testing.T) {
	hs := running(t)

	hs.key("x")
	frameHas(t, hs, "Stop PUN-1", "Duration", "Comment", "→ sends 1h 12m to Jira as one worklog")
	if d := cmp.Diff([]core.Command{core.Touch{Origin: core.OriginHand}}, hs.st.commands(), anyGrammar); d != "" {
		t.Fatalf("opening the dialog is only presence (-want +got)\n%s", d)
	}
	hs.key("enter")

	want := []core.Stop{{Key: "PUN-1", EntryID: "E1", Duration: 72 * time.Minute, Origin: core.OriginHand}}
	if d := cmp.Diff(want, hs.sender.calls()); d != "" {
		t.Errorf("stops (-want +got)\n%s", d)
	}
	frameLacks(t, hs, "Stop PUN-1")
	frameHas(t, hs, "stopped PUN-1 · logged 1h 12m", "✓ logged 09:12")
}

func TestStopDialogEscLeavesTheTimerAlone(t *testing.T) {
	hs := running(t)
	hs.key("x", "esc")

	if got := hs.sender.calls(); len(got) != 0 {
		t.Errorf("esc sent %v", got)
	}
	if got := hs.running(); got != "PUN-1" {
		t.Errorf("running = %q, want PUN-1", got)
	}
	frameLacks(t, hs, "Stop PUN-1")
	frameHas(t, hs, "▶ RUNNING")
}

func TestStopDialogEditsDurationAndComment(t *testing.T) {
	hs := running(t)
	hs.key("x")
	for range len("1h 12m") {
		hs.key("backspace")
	}
	hs.typeText("45m")
	frameHas(t, hs, "→ sends 45m to Jira")
	hs.key("tab")
	hs.typeText("did it")
	hs.key("enter")

	want := []core.Stop{{Key: "PUN-1", EntryID: "E1", Duration: 45 * time.Minute, Comment: "did it", Origin: core.OriginHand}}
	if d := cmp.Diff(want, hs.sender.calls()); d != "" {
		t.Errorf("stops (-want +got)\n%s", d)
	}
}

func TestStopDialogRefusesWhatItCannotRead(t *testing.T) {
	hs := running(t)
	hs.key("x")
	for range len("1h 12m") {
		hs.key("backspace")
	}
	hs.typeText("soon")
	frameHas(t, hs, "✗ can't read that duration")
	hs.key("enter")

	if got := hs.sender.calls(); len(got) != 0 {
		t.Errorf("sent %v", got)
	}
	frameHas(t, hs, "Stop PUN-1", "try 1h 20m, 80m or 1:20")
}

func TestStopDialogAsksARecoveredEntryForADuration(t *testing.T) {
	hs := newHarness(t)
	hs.setup("06:00:00", add("PUN-1"), start("PUN-1", "E1"))
	hs.setup("06:50:00", core.Touch{Origin: core.OriginHand})
	hs.open("09:12:00", 120, 32)

	hs.key("x")
	frameHas(t, hs, "! recovered", "how long did you work?")
	frameLacks(t, hs, "→ sends")
	hs.key("enter")
	if got := hs.sender.calls(); len(got) != 0 {
		t.Fatalf("sent %v without a duration", got)
	}
	hs.typeText("40m")
	hs.key("enter")

	want := []core.Stop{{Key: "PUN-1", EntryID: "E1", Duration: 40 * time.Minute, Origin: core.OriginHand}}
	if d := cmp.Diff(want, hs.sender.calls()); d != "" {
		t.Errorf("stops (-want +got)\n%s", d)
	}
}

func TestStopThatCannotSendStillStops(t *testing.T) {
	hs := running(t)
	hs.sender.syncErr = tracker.ErrUnreachable
	hs.key("x", "enter")

	if got := hs.running(); got != "" {
		t.Errorf("still running %q", got)
	}
	frameHas(t, hs, "stopped PUN-1 · 1h 12m not sent", "nothing is lost", "○ not sent yet")
}

func TestStopWithNothingOpen(t *testing.T) {
	hs := newHarness(t)
	hs.setup("08:00:00", add("PUN-1"))
	hs.open("09:00:00", 120, 32)
	hs.key("x")

	frameLacks(t, hs, "Stop PUN-1")
	frameHas(t, hs, "nothing to stop on PUN-1 — space starts it")
}

// A dialog is laid over [2]: everything outside it stays put, and at a
// width too narrow for [2] it covers the body without resizing it.
func TestDialogsNeverMoveTheFrame(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {120, 32}, {60, 16}} {
		for _, k := range []string{"x", "a", "d"} {
			hs := newHarness(t)
			runningDay(hs)
			hs.open("09:00:00", sz[0], sz[1])
			before := strings.Split(hs.frame(), "\n")
			hs.key(k)
			after := strings.Split(hs.frame(), "\n")
			if len(after) != len(before) {
				t.Fatalf("%dx%d %s: %d lines, was %d", sz[0], sz[1], k, len(after), len(before))
			}
			x := ticketsWidth(sz[0])
			if sz[0]-x < minDialogWidth {
				x = 0
			}
			bodyH := len(before) - 5         // above the Jira strip (3), status and keys
			for i := range len(before) - 2 { // the status and key bar change
				b, a := []rune(before[i]), []rune(after[i])
				if i < bodyH {
					b, a = b[:x], a[:x]
				}
				if string(b) != string(a) {
					t.Errorf("%dx%d %s: line %d moved:\n%s\n%s", sz[0], sz[1], k, i, string(b), string(a))
				}
			}
		}
	}
}

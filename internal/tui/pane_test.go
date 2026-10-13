package tui

import (
	"strings"
	"testing"

	"github.com/j-dsmith/punchcard/internal/core"
)

// paneText is [2]'s inner text, one string per line, trailing blanks
// trimmed: the columns right of the Tickets pane, borders cut off.
func paneText(t *testing.T, frame string, w int) []string {
	t.Helper()
	lw := ticketsWidth(w)
	var out []string
	for _, l := range strings.Split(frame, "\n") {
		r := []rune(l)
		if len(r) < w {
			continue
		}
		out = append(out, strings.TrimRight(string(r[lw+1:w-1]), " "))
	}
	return out
}

func containsLine(lines []string, want string) bool {
	for _, l := range lines {
		if squash(l) == squash(want) {
			return true
		}
	}
	return false
}

// squash collapses runs of spaces: alignment is the goldens' job.
func squash(s string) string { return strings.Join(strings.Fields(s), " ") }

func wantLines(t *testing.T, lines []string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !containsLine(lines, w) {
			t.Errorf("[2] has no line %q:\n%s", w, strings.Join(lines, "\n"))
		}
	}
}

func TestTicketPaneSpellsOutAPausedEntry(t *testing.T) {
	hs := newHarness(t)
	hs.setup("08:14:00", add("PUN-1"), start("PUN-1", "E1"))
	hs.setup("08:15:00", pause)
	hs.setup("08:20:00", start("PUN-1", "E1"))
	hs.setup("08:41:30", pause)
	hs.open("09:00:00", 120, 32)

	wantLines(t, paneText(t, hs.frame(), 120),
		"Summary of PUN-1",
		"‖ PAUSED                                   22m",
		"stop sends this as one worklog — x",
		"pauses are left out",
		"08:20–08:41     21m",
		"08:15–08:20 ‖ paused 5m",
		"08:14–08:15      1m",
		"WORKLOGS TODAY",
		"nothing yet",
	)
}

func TestTicketPaneRunningHeadlineTicks(t *testing.T) {
	hs := newHarness(t)
	hs.setup("08:00:00", add("PUN-1"), start("PUN-1", "E1"))
	hs.open("09:00:00", 120, 32)
	hs.tick("09:12:34")

	wantLines(t, paneText(t, hs.frame(), 120),
		"▶ RUNNING                              1:12:34",
		"08:00–now   1:12:34",
	)
	noCounted(t, hs.frame())
	if containsLine(paneText(t, hs.frame(), 120), "pauses are left out") {
		t.Errorf("a single running segment says what is left out")
	}
}

func TestTicketPaneRecoveredEntryShowsTimeAway(t *testing.T) {
	hs := newHarness(t)
	hs.setup("06:00:00", add("PUN-1"), start("PUN-1", "E1"))
	hs.setup("06:50:00", core.Touch{Origin: core.OriginHand})
	hs.open("09:12:00", 120, 32) // no presence since 06:50: stale

	lines := paneText(t, hs.frame(), 120)
	wantLines(t, lines,
		"! RECOVERED                                50m",
		"time away is left out",
		"06:50–now   ! away 2h 22m",
		"06:00–06:50     50m",
	)
	noCounted(t, hs.frame())
	if !strings.Contains(hs.frame(), "!") {
		t.Errorf("no recovered marker")
	}
}

// noCounted fails if a frame still labels stretches: only gaps are
// marked, by their reason.
func noCounted(t *testing.T, frame string) {
	t.Helper()
	if strings.Contains(frame, "counted") {
		t.Errorf("frame labels a stretch counted:\n%s", frame)
	}
}

func TestTicketPaneSubMinuteSegmentIsUnderAMinute(t *testing.T) {
	hs := newHarness(t)
	hs.setup("08:14:00", add("PUN-1"), start("PUN-1", "E1"))
	hs.setup("08:14:40", pause)
	hs.setup("08:20:00", start("PUN-1", "E1"))
	hs.open("09:00:00", 120, 32)

	lines := paneText(t, hs.frame(), 120)
	wantLines(t, lines, "08:14–08:14     <1m", "08:20–now   0:40:00")
	if containsLine(lines, "08:14–08:14      0m") {
		t.Errorf("a sub-minute segment reads 0m")
	}
}

func TestTicketPaneSaysPausesAndTimeAwayAreLeftOut(t *testing.T) {
	hs := newHarness(t)
	hs.setup("06:00:00", add("PUN-1"), start("PUN-1", "E1"))
	hs.setup("06:20:00", pause)
	hs.setup("06:30:00", start("PUN-1", "E1"))
	hs.setup("06:50:00", core.Touch{Origin: core.OriginHand})
	hs.open("09:12:00", 120, 32)

	wantLines(t, paneText(t, hs.frame(), 120),
		"stop sends this as one worklog — x",
		"pauses and time away are left out",
		"06:50–now   ! away 2h 22m",
		"06:20–06:30 ‖ paused 10m",
	)
}

func TestTicketPaneWithNoTrackerLogs(t *testing.T) {
	hs := newHarness(t)
	hs.noTracker = true
	hs.setup("08:00:00", add("payroll"), start("payroll", "E1"))
	hs.open("09:00:00", 120, 32)

	wantLines(t, paneText(t, hs.frame(), 120), "stop logs this as one worklog — x")
}

// At 80 columns a gap still takes one line.
func TestTicketPaneGapsTakeOneLineWhenNarrow(t *testing.T) {
	hs := newHarness(t)
	hs.setup("06:00:00", add("PUN-1"), start("PUN-1", "E1"))
	hs.setup("06:20:00", pause)
	hs.setup("06:30:00", start("PUN-1", "E1"))
	hs.setup("06:50:00", core.Touch{Origin: core.OriginHand})
	hs.open("09:12:00", 80, 24)

	lines := paneText(t, hs.frame(), 80)
	for i, l := range lines {
		if strings.Contains(l, "away") || strings.Contains(l, "paused") {
			if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "             ") && strings.TrimSpace(lines[i+1]) != "" {
				t.Errorf("gap %q spills onto %q", l, lines[i+1])
			}
		}
	}
	wantLines(t, lines, "06:50–now   ! away 2h 22m", "06:20–06:30 ‖ paused 10m")
	noCounted(t, hs.frame())
}

func TestTicketPaneIdleTicket(t *testing.T) {
	hs := newHarness(t)
	hs.setup("08:00:00", add("PUN-1"))
	hs.open("09:00:00", 120, 32)

	wantLines(t, paneText(t, hs.frame(), 120), "· NOT RUNNING", "space starts it", "nothing yet")
}

// A local worklog has no moment a tracker took it: it is logged on its
// own day, the day it started.
func TestALocalWorklogIsLoggedToday(t *testing.T) {
	hs := newHarness(t)
	hs.noTracker = true
	hs.setup("07:00:00", add("payroll"), start("payroll", "A"))
	hs.setup("07:30:00", core.Stop{Origin: core.OriginHand, Local: true})
	hs.open("09:00:00", 120, 32)

	wantLines(t, paneText(t, hs.frame(), 120), "30m ✓ logged")
}

func TestWorklogsTodayUseTheOutcomeWords(t *testing.T) {
	hs := newHarness(t)
	hs.setup("07:00:00", add("PUN-1"), start("PUN-1", "A"))
	hs.setup("07:30:00", core.Stop{Origin: core.OriginHand})
	hs.setup("07:31:00", core.MarkSynced{Key: "PUN-1", EntryID: "A", WorklogID: "W9"})
	hs.setup("08:00:00", start("PUN-1", "B"))
	hs.setup("08:20:00", core.Stop{Origin: core.OriginHand})
	hs.setup("08:20:01", core.MarkFailed{Key: "PUN-1", EntryID: "B", Message: "Service Unavailable: down for maintenance", Status: 503})
	hs.setup("08:30:00", start("PUN-1", "C"))
	hs.setup("08:45:00", core.Stop{Origin: core.OriginHand})
	hs.open("09:00:00", 120, 32)

	wantLines(t, paneText(t, hs.frame(), 120),
		"15m  ○ not sent yet · r",
		"20m  ✗ failed — 503 · r",
		"30m  ✓ logged 07:31",
	)
}

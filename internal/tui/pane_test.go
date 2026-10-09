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
		"08:20–08:41     21m counted",
		"08:15–08:20 ‖ paused 5m, not counted",
		"08:14–08:15      1m counted",
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
		"08:00–now   1:12:34 counted",
	)
}

func TestTicketPaneRecoveredEntryShowsTimeAway(t *testing.T) {
	hs := newHarness(t)
	hs.setup("06:00:00", add("PUN-1"), start("PUN-1", "E1"))
	hs.setup("06:50:00", core.Touch{Origin: core.OriginHand})
	hs.open("09:12:00", 120, 32) // no presence since 06:50: stale

	lines := paneText(t, hs.frame(), 120)
	wantLines(t, lines,
		"! RECOVERED                                50m",
		"06:50–now   ! away 2h 22m, not counted",
		"06:00–06:50     50m counted",
	)
	if !strings.Contains(hs.frame(), "!") {
		t.Errorf("no recovered marker")
	}
}

func TestTicketPaneIdleTicket(t *testing.T) {
	hs := newHarness(t)
	hs.setup("08:00:00", add("PUN-1"))
	hs.open("09:00:00", 120, 32)

	wantLines(t, paneText(t, hs.frame(), 120), "· NOT RUNNING", "space starts it", "nothing yet")
}

func TestWorklogsTodayUseTheOutcomeWords(t *testing.T) {
	hs := newHarness(t)
	hs.setup("07:00:00", add("PUN-1"), start("PUN-1", "A"))
	hs.setup("07:30:00", core.Stop{Origin: core.OriginHand})
	hs.setup("07:31:00", core.MarkSynced{Key: "PUN-1", EntryID: "A", WorklogID: "W9"})
	hs.setup("08:00:00", start("PUN-1", "B"))
	hs.setup("08:20:00", core.Stop{Origin: core.OriginHand})
	hs.setup("08:20:01", core.MarkFailed{Key: "PUN-1", EntryID: "B", Message: "Jira answered 503: down"})
	hs.setup("08:30:00", start("PUN-1", "C"))
	hs.setup("08:45:00", core.Stop{Origin: core.OriginHand})
	hs.open("09:00:00", 120, 32)

	wantLines(t, paneText(t, hs.frame(), 120),
		"15m  ○ not sent yet · r",
		"20m  ✗ failed — 503 · r",
		"30m  ✓ logged 07:31",
	)
}

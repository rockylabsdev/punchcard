package tui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/tracker"
)

// row is key's row in [1] Tickets, squashed.
func row(t *testing.T, hs *harness, key string) string {
	t.Helper()
	re := regexp.MustCompile(`^│[ ›]\S ` + regexp.QuoteMeta(key) + ` `)
	for _, l := range strings.Split(hs.frame(), "\n") {
		if re.MatchString(l) {
			return squash(l)
		}
	}
	t.Fatalf("no row for %s:\n%s", key, hs.frame())
	return ""
}

// elsewhere is AA-1 started from a prompt at 08:00 (so not held) with
// BB-2 and CC-3 idle, then before, open at 09:00 with Jira configured.
func elsewhere(t *testing.T, before ...core.Command) *harness {
	hs := newHarness(t)
	hs.setup("08:00:00", add("AA-1", "BB-2", "CC-3"),
		core.Start{Key: "AA-1", Origin: core.OriginPrompt, NewEntryID: "EA"})
	hs.setup("08:30:00", before...)
	hs.jira = &tracker.Fake{}
	hs.open("09:00:00", 120, 32)
	return hs
}

// A change another process makes between two ticks shows on its row,
// naming what and who, and the status line logs it with its time.
func TestChangeElsewhereIsTaggedOnItsRow(t *testing.T) {
	cases := []struct {
		name   string
		before []core.Command // before the TUI opens
		change []core.Command
		rows   map[string]string // key → the tag its row shows
		status string
	}{
		{"an agent pauses", nil,
			[]core.Command{core.Pause{Origin: core.OriginAgent}},
			map[string]string{"AA-1": "paused by agent"},
			"09:00 agent paused AA-1"},
		{"a prompt starts another ticket, pausing the first", nil,
			[]core.Command{core.Touch{Key: "BB-2", Origin: core.OriginPrompt, NewEntryID: "EB"}},
			map[string]string{"AA-1": "paused by prompt", "BB-2": "started by prompt"},
			"09:00 prompt started BB-2 · paused AA-1"},
		{"a stop by hand in another terminal", nil,
			[]core.Command{core.Stop{Origin: core.OriginHand}},
			map[string]string{"AA-1": "stopped by hand"},
			"09:00 you stopped AA-1"},
		{"an agent resumes",
			[]core.Command{core.Pause{Origin: core.OriginHand}},
			[]core.Command{core.Resume{Origin: core.OriginAgent}},
			map[string]string{"AA-1": "resumed by agent"},
			"09:00 agent resumed AA-1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hs := elsewhere(t, c.before...)
			hs.tick("09:00:01")
			hs.setup("09:00:05", c.change...)
			hs.tick("09:00:06")
			for key, tag := range c.rows {
				if r := row(t, hs, key); !strings.Contains(r, tag) {
					t.Errorf("%s row %q has no %q", key, r, tag)
				}
			}
			hs.tick("09:00:11") // the toast has gone; the last event shows
			if got := statusText(hs); got != c.status {
				t.Errorf("status line %q, want %q", got, c.status)
			}
			for key, tag := range c.rows {
				if r := row(t, hs, key); !strings.Contains(r, tag) {
					t.Errorf("at 5 s %s row %q lost %q", key, r, tag)
				}
			}
			hs.tick("09:00:12")
			frameLacks(t, hs, " by ")
		})
	}
}

// What the TUI does itself is not "elsewhere": its own start shows as
// the user's, with no tag, even after the next tick re-reads the file.
func TestOwnChangesAreNotTagged(t *testing.T) {
	hs := elsewhere(t)
	hs.key("j", "space") // start BB-2
	if hs.running() != "BB-2" {
		t.Fatalf("running %q", hs.running())
	}
	hs.tick("09:00:01")
	hs.tick("09:00:02")
	frameLacks(t, hs, "by hand", "by agent", "by prompt")
}

// An untouched open TUI is not presence: ticks, even ones that find a
// change elsewhere, never ask the store to write anything, and an
// unchanged file is never re-read.
func TestUntouchedTUIWritesNothing(t *testing.T) {
	hs := elsewhere(t)
	hs.st.reads() // the startup read
	for _, s := range []string{"09:00:01", "09:00:02", "09:00:30", "09:05:00"} {
		hs.tick(s)
	}
	if got := hs.st.reads(); got != 0 {
		t.Errorf("%d reads with nothing changed", got)
	}
	hs.setup("09:05:30", core.Pause{Origin: core.OriginAgent})
	for _, s := range []string{"09:05:31", "09:05:32", "09:10:00"} {
		hs.tick(s)
	}
	if got := hs.st.commands(); len(got) != 0 {
		t.Errorf("the TUI asked the store for %v", got)
	}
	if got := hs.st.reads(); got != 1 {
		t.Errorf("%d reads for one change, want 1", got)
	}
}

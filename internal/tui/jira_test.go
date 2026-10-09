package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/j-dsmith/punchcard/internal/config"
	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/tracker"
)

// stripText is [3]'s lines, title first, borders cut off and spaces
// squashed.
func stripText(frame string) []string {
	var out []string
	in := false
	for _, l := range strings.Split(frame, "\n") {
		switch {
		case strings.HasPrefix(l, "╭─[3] Today"):
			in = true
			out = append(out, squash(strings.Trim(l, "╭─╮")))
		case in && strings.HasPrefix(l, "╰"):
			return out
		case in:
			out = append(out, squash(strings.Trim(l, "│")))
		}
	}
	return out
}

// statusText is the status line, squashed.
func statusText(hs *harness) string {
	lines := strings.Split(hs.frame(), "\n")
	return squash(lines[len(lines)-2])
}

// owing is AA-1 with a stopped 30m worklog that failed with a 503 and
// BB-2 with a pending 15m one.
func owing(t *testing.T) *harness {
	hs := newHarness(t)
	hs.setup("07:00:00", add("AA-1", "BB-2"), start("AA-1", "E-A"))
	hs.setup("07:30:00", core.Stop{Origin: core.OriginHand})
	hs.setup("07:30:01", core.MarkFailed{Key: "AA-1", EntryID: "E-A", Message: "Jira answered 503 Service Unavailable", Status: 503})
	hs.setup("08:00:00", start("BB-2", "E-B"))
	hs.setup("08:15:00", core.Stop{Origin: core.OriginHand})
	return hs
}

// The status line carries the worst condition, worst first, and the
// strip's title says why nothing can sync.
func TestBannerIsTheWorstCondition(t *testing.T) {
	cases := []struct {
		name       string
		configured bool
		rejected   bool
		owe        bool
		banner     string
		title      string
	}{
		{"token rejected beats not connected", false, true, true,
			"✗ Jira rejected the token at 08:20 · 2 worklogs waiting · S to fix", "· token rejected"},
		{"token rejected", true, true, true,
			"✗ Jira rejected the token at 08:20 · 2 worklogs waiting · S to fix", "· token rejected"},
		{"not connected beats a failure", false, false, true,
			"Not connected to Jira · S to set up · worklogs wait here, nothing is lost", "· not connected"},
		{"not connected, nothing owed", false, false, false,
			"Not connected to Jira · S to set up · worklogs wait here, nothing is lost", "· not connected"},
		{"a failure", true, false, true,
			"✗ 1 worklog failed to reach Jira · r to retry · nothing is lost", "· owed 45m"},
		{"all well", true, false, false, "", "· owed 0m"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hs := newHarness(t)
			hs.setup("07:00:00", add("AA-1", "BB-2"))
			if c.owe {
				hs = owing(t)
			}
			if c.rejected {
				hs.setup("08:20:00", core.RecordAuthFailure{})
			}
			if c.configured {
				hs.jira = &tracker.Fake{}
			}
			hs.open("09:12:00", 120, 32)
			hs.tick("09:12:34")
			if got := statusText(hs); got != c.banner {
				t.Errorf("status line = %q, want %q", got, c.banner)
			}
			if title := stripText(hs.frame())[0]; !strings.Contains(title, c.title) {
				t.Errorf("strip title = %q, want it to have %q", title, c.title)
			}
		})
	}
}

// r sends everything owed through the Sender in a tea.Cmd; "syncing…"
// holds the status line until it answers.
func TestRSyncsEverythingOwed(t *testing.T) {
	hs := owing(t)
	hs.jira = &tracker.Fake{}
	hs.open("09:12:00", 120, 32)
	hs.tick("09:12:34")

	next, cmd := hs.m.Update(keyPress("r"))
	hs.m = next.(Model)
	if got := statusText(hs); got != "○ syncing…" {
		t.Errorf("status line while syncing = %q", got)
	}
	if hs.sender.syncs != 0 {
		t.Fatalf("Update called the sender itself")
	}
	// A second r while the first is out starts nothing.
	next, again := hs.m.Update(keyPress("r"))
	hs.m = next.(Model)
	hs.run(cmd)
	hs.run(again)

	if hs.sender.syncs != 1 {
		t.Errorf("syncs = %d, want 1", hs.sender.syncs)
	}
	if got := len(hs.jira.Worklogs()); got != 2 {
		t.Errorf("Jira got %d worklogs, want 2", got)
	}
	frameHas(t, hs, "✓ logged 09:12 AA-1", "✓ logged 09:12 BB-2", "last sync 09:12")
	frameLacks(t, hs, "not sent yet", "failed —")
	if got := statusText(hs); got != "sent 2 worklogs to Jira" {
		t.Errorf("toast = %q", got)
	}
	hs.tick("09:12:40")
	if got := statusText(hs); got != "09:12 you sent 2 worklogs to Jira" {
		t.Errorf("after the toast = %q", got)
	}
}

// The hand check: a bad site fails the sync (✗ row and banner); once
// the site is fixed, r clears both.
func TestRClearsAFailureOnceJiraAnswers(t *testing.T) {
	hs := owing(t)
	hs.jira = &tracker.Fake{Err: tracker.ErrUnreachable}
	hs.open("09:12:00", 120, 32)
	hs.key("r")
	if got := statusText(hs); got != "Jira not reachable · 2 worklogs still owed · nothing is lost" {
		t.Errorf("toast = %q", got)
	}
	hs.tick("09:12:40")
	if got := statusText(hs); got != "✗ 1 worklog failed to reach Jira · r to retry · nothing is lost" {
		t.Errorf("banner = %q", got)
	}
	frameHas(t, hs, "✗ failed — unreachable · r AA-1")

	hs.jira.Err = nil
	hs.key("r")
	hs.tick("09:12:50")
	frameLacks(t, hs, "failed —", "not sent yet", "failed to reach Jira")
	if got := statusText(hs); got != "09:12 you sent 2 worklogs to Jira" {
		t.Errorf("status line = %q", got)
	}
}

func TestROutcomes(t *testing.T) {
	cases := []struct {
		name  string
		owe   bool
		jira  *tracker.Fake
		toast string
	}{
		{"nothing owed", false, &tracker.Fake{}, "nothing to send: Jira has everything"},
		{"not configured", true, nil, "Jira is not set up · 2 worklogs wait here"},
		{"token rejected", true, &tracker.Fake{Err: tracker.ErrUnauthorized}, "Jira rejected the token · 2 worklogs still owed · nothing is lost"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hs := newHarness(t)
			hs.setup("07:00:00", add("AA-1"))
			if c.owe {
				hs = owing(t)
			}
			hs.jira = c.jira
			hs.open("09:12:00", 120, 32)
			hs.key("r")
			if got := statusText(hs); got != c.toast {
				t.Errorf("toast = %q, want %q", got, c.toast)
			}
		})
	}
}

// keyBarText is the bottom line, squashed.
func keyBarText(hs *harness) string {
	lines := strings.Split(hs.frame(), "\n")
	return squash(lines[len(lines)-1])
}

// The key bar shows only the keys that apply now.
func TestKeyBarShowsWhatApplies(t *testing.T) {
	cases := []struct {
		name  string
		build func(t *testing.T) *harness
		keys  []string
		want  string
	}{
		{"idle row, something owed", owing, nil,
			"space start a add d remove r sync 2 S setup ? help q quit"},
		{"running row, nothing owed", func(t *testing.T) *harness {
			hs := newHarness(t)
			hs.setup("08:00:00", add("AA-1"), start("AA-1", "E"))
			return hs
		}, nil, "space pause x stop… a add d remove S setup ? help q quit"},
		{"another row runs", func(t *testing.T) *harness {
			hs := owing(t)
			hs.setup("08:30:00", start("AA-1", "E2"))
			return hs
		}, []string{"j"}, "space start p pause a add d remove r sync 2 S setup ? help q quit"},
		{"empty list", func(t *testing.T) *harness { return newHarness(t) }, nil,
			"a add S setup ? help q quit"},
		{"help open", owing, []string{"?"}, "any key close"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hs := c.build(t)
			hs.jira = &tracker.Fake{}
			hs.open("09:12:00", 120, 32)
			hs.key(c.keys...)
			if got := keyBarText(hs); got != c.want {
				t.Errorf("key bar = %q, want %q", got, c.want)
			}
		})
	}
}

// ? lays the keys and glyphs over the body; any key closes it and does
// nothing else.
func TestHelp(t *testing.T) {
	hs := owing(t)
	hs.jira = &tracker.Fake{}
	hs.open("09:12:00", 120, 32)
	hs.key("?")
	frameHas(t, hs, "Help", "r sync: send pending and failed worklogs", "○ waiting to be sent", "✗ failed")
	hs.key("space")
	frameLacks(t, hs, "r sync: send pending")
	if got := hs.running(); got != "" {
		t.Errorf("the key that closed help started %s", got)
	}
}

// With no tracker the Jira conditions never show: the gate is the
// tracker, not the state file, so even an authFailedAt left behind (or
// a failed worklog from before the tracker went) leaves the status line
// to the last event, and the pane to what was logged.
func TestNoTrackerNeverShowsTheJiraBanner(t *testing.T) {
	hs := owing(t)
	hs.setup("08:20:00", core.RecordAuthFailure{})
	hs.noTracker = true
	hs.open("09:12:00", 120, 32)
	hs.tick("09:12:34")
	if got := statusText(hs); got != "" {
		t.Errorf("status line = %q, want it empty", got)
	}
	if got := stripText(hs.frame()); strings.Join(got, "\n") != "[3] Today · logged 0m\nnothing logged today — x on a row stops it and logs it here" {
		t.Errorf("strip = %q", got)
	}
	frameLacks(t, hs, "Jira", "sync")
}

// A config that cannot be read is not connected too, as worklog.Sender
// treats it.
func TestABrokenConfigIsNotConnected(t *testing.T) {
	hs := owing(t)
	hs.openErr = errors.New("config.json: invalid character")
	hs.open("09:12:00", 120, 32)
	if got := statusText(hs); got != "Not connected to Jira · S to set up · worklogs wait here, nothing is lost" {
		t.Errorf("status line = %q", got)
	}
}

// A Keychain that cannot be read is not connected, and the banner names
// the Keychain; a rejected token still outranks it.
func TestABrokenKeychainNamesTheKeychain(t *testing.T) {
	kc := &config.KeychainError{Err: config.ErrKeychainTimeout}
	hs := owing(t)
	hs.openErr = kc
	hs.open("09:12:00", 120, 32)
	if got := statusText(hs); got != "Keychain error: timed out reading the token · worklogs wait here, nothing is lost" {
		t.Errorf("status line = %q", got)
	}

	hs = owing(t)
	hs.setup("08:20:00", core.RecordAuthFailure{})
	hs.openErr = kc
	hs.open("09:12:00", 120, 32)
	if got := statusText(hs); !strings.HasPrefix(got, "✗ Jira rejected the token") {
		t.Errorf("status line = %q", got)
	}
}

func TestJiraStripRowsLeadWithTheOutcome(t *testing.T) {
	hs := newHarness(t)
	hs.setup("07:00:00", add("PUN-120", "OPS-77", "PUN-9"))
	hs.setup("07:05:00", start("PUN-120", "E-120"))
	hs.setup("07:45:00", core.Stop{Origin: core.OriginHand})
	hs.setup("07:45:01", core.MarkFailed{Key: "PUN-120", EntryID: "E-120", Message: "Jira answered 503 Service Unavailable", Status: 503})
	hs.setup("07:46:00", start("PUN-9", "E-9"))
	hs.setup("08:16:00", core.Stop{Origin: core.OriginHand})
	hs.setup("08:17:00", core.MarkSynced{Key: "PUN-9", EntryID: "E-9", WorklogID: "W9"})
	hs.setup("08:20:00", start("OPS-77", "E-77"))
	hs.setup("08:35:00", core.Stop{Origin: core.OriginHand})
	hs.jira = &tracker.Fake{}
	hs.open("09:12:00", 120, 32)
	hs.tick("09:12:34")

	got := stripText(hs.frame())
	want := []string{
		"[3] Today · logged 30m · owed 55m · last sync 08:17",
		"STATUS TICKET TIME STOPPED",
		"✗ failed — 503 · r PUN-120 40m 07:45",
		"○ not sent yet · r OPS-77 15m 08:35",
		"✓ logged 08:17 PUN-9 30m 08:16",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("strip:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

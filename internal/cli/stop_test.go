package cli_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/j-dsmith/punchcard/internal/tracker"
)

const (
	nineThirty = `"2026-08-09T09:30:00.000-0500"`
	ten        = `"2026-08-09T10:00:00.000-0500"`
	stopHint   = "run `punchcard sync` later; never stop again"
)

// stopData is stop's data for AA-1's first entry with this worklog.
func stopData(worklog string) string {
	return fmt.Sprintf(`{"key": "AA-1", "entryId": "ID-1", "worklog": %s}`, worklog)
}

func owed(command string, data, events, code string) string {
	return fmt.Sprintf(`{"v": 1, "ok": true, "outcome": "sync_pending", "command": %q, "data": %s, "events": %s,
		"error": {"code": %q, "message": "*", "hint": %q}}`, command, data, events, code, stopHint)
}

func preview(secs int, recovered bool) string {
	return fmt.Sprintf(`{"preview": {"key": "AA-1", "entryId": "ID-1", "recovered": %t,
		"worklog": {"state": "pending", "startedAt": %s, "timeSpentSeconds": %d}}}`, recovered, nine, secs)
}

const syncedAA1 = `{"state": "synced", "startedAt": ` + nine + `, "timeSpentSeconds": 1800, "worklogId": "W1", "syncedAt": ` + nineThirty + `}`

func TestStopEnvelopes(t *testing.T) {
	startAA1 := func(h *harness) { h.do("09:00", "start", "AA-1") }
	asAgent := func(h *harness) { h.do("09:00", "start", "AA-1"); h.env["CLAUDECODE"] = "1" }
	recoveredAA1 := func(h *harness) { h.do("09:00", "start", "AA-1"); h.do("09:45", "touch") }
	cases := []struct {
		name  string
		jira  *tracker.Fake
		setup func(h *harness)
		at    string
		args  []string
		code  int
		want  string
		posts int
	}{
		{"stop sends the worklog", &tracker.Fake{}, startAA1, "09:30",
			[]string{"stop"}, 0, ok("stop", stopData(syncedAA1), `[{"kind": "synced", "key": "AA-1"}]`), 1},
		{"unreachable: stopped, worklog failed, exit 7", &tracker.Fake{Err: tracker.ErrUnreachable}, startAA1, "09:30",
			[]string{"stop"}, 7,
			owed("stop", stopData(`{"state": "failed", "startedAt": `+nine+`, "timeSpentSeconds": 1800, "message": "*", "failedAt": `+nineThirty+`}`),
				`[{"kind": "syncFailed", "key": "AA-1", "message": "*"}]`, "unreachable"), 1},
		{"a 401: stopped, worklog pending, exit 7", &tracker.Fake{Err: tracker.ErrUnauthorized}, startAA1, "09:30",
			[]string{"stop"}, 7,
			owed("stop", stopData(`{"state": "pending", "startedAt": `+nine+`, "timeSpentSeconds": 1800}`), noEvents, "auth_failed"), 1},
		{"Jira refuses: worklog failed, exit 7", &tracker.Fake{Err: &tracker.Refusal{Status: 503}}, startAA1, "09:30",
			[]string{"stop"}, 7,
			owed("stop", stopData(`{"state": "failed", "startedAt": `+nine+`, "timeSpentSeconds": 1800, "message": "the tracker answered 503", "refusedStatus": 503, "failedAt": `+nineThirty+`}`),
				`[{"kind": "syncFailed", "key": "AA-1", "message": "the tracker answered 503"}]`, "sync_failed"), 1},
		{"unconfigured: stopped, exit 7 not_configured", nil, startAA1, "09:30",
			[]string{"stop"}, 7,
			owed("stop", stopData(`{"state": "pending", "startedAt": `+nine+`, "timeSpentSeconds": 1800}`), noEvents, "not_configured"), 0},
		{"--no-sync never calls Jira", &tracker.Fake{}, startAA1, "09:30",
			[]string{"stop", "--no-sync"}, 0,
			ok("stop", stopData(`{"state": "pending", "startedAt": `+nine+`, "timeSpentSeconds": 1800}`), noEvents), 0},
		{"an agent's stop is a proposal", &tracker.Fake{}, asAgent, "09:30",
			[]string{"stop", "-m", "x"}, 6,
			refused("stop", "needs_confirmation", "needs_confirmation", "show the user this worklog; only `punchcard stop --yes` sends it",
				`{"preview": {"key": "AA-1", "entryId": "ID-1", "recovered": false,
				  "worklog": {"state": "pending", "startedAt": `+nine+`, "timeSpentSeconds": 1800, "comment": "x"}}}`), 0},
		{"an agent's stop --yes sends", &tracker.Fake{}, asAgent, "09:30",
			[]string{"stop", "--yes"}, 0, ok("stop", stopData(syncedAA1), `[{"kind": "synced", "key": "AA-1"}]`), 1},
		{"--dry-run previews and changes nothing", &tracker.Fake{}, startAA1, "09:30",
			[]string{"stop", "--dry-run", "--duration", "1h"}, 0, ok("stop", preview(3600, false), noEvents), 0},
		{"--dry-run previews for an agent too", &tracker.Fake{}, asAgent, "09:30",
			[]string{"stop", "--dry-run"}, 0, ok("stop", preview(1800, false), noEvents), 0},
		{"--dry-run previews a recovered entry", &tracker.Fake{}, recoveredAA1, "13:00",
			[]string{"stop", "--dry-run"}, 0, ok("stop", preview(2700, true), noEvents), 0},
		{"a recovered entry off a TTY needs --duration, even with --yes", &tracker.Fake{}, recoveredAA1, "13:00",
			[]string{"stop", "--yes"}, 6,
			refused("stop", "needs_confirmation", "needs_confirmation", "rerun with --duration, e.g. --duration 1h30m", preview(2700, true)), 0},
		{"under a minute exits 64", &tracker.Fake{}, startAA1, "09:00",
			[]string{"stop"}, 64, refused("stop", "usage", "usage", "pass --duration", "null"), 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			h.jira = knowing(c.jira)
			if c.setup != nil {
				c.setup(h)
			}
			h.now = at(c.at)
			r := h.run(append(c.args, "--json")...)
			if r.code != c.code {
				t.Errorf("exit %d, want %d\nstderr %q", r.code, c.code, r.stderr)
			}
			if d := cmp.Diff(decode(t, c.want), decode(t, r.stdout), anyMessage); d != "" {
				t.Errorf("envelope (-want +got):\n%s", d)
			}
			if c.jira != nil && posts(c.jira) != c.posts {
				t.Errorf("calls %v, want %d POST(s)", c.jira.Calls(), c.posts)
			}
		})
	}
}

// knowing gives f the test tickets, so a start against it verifies
// rather than 404s. Nil stays nil (unconfigured).
func knowing(f *tracker.Fake) *tracker.Fake {
	if f != nil && f.Summaries == nil {
		f.Summaries = map[string]string{"AA-1": "Fix the thing", "AA-2": "Other thing"}
	}
	return f
}

// posts counts the worklog POSTs tried, accepted or not.
func posts(f *tracker.Fake) int {
	n := 0
	for _, c := range f.Calls() {
		if strings.HasPrefix(c, "worklog ") {
			n++
		}
	}
	return n
}

func TestAnAgentProposalAndADryRunChangeNothing(t *testing.T) {
	for _, args := range [][]string{{"stop", "--json"}, {"stop", "--dry-run", "--json"}} {
		h := newHarness(t)
		h.jira = knowing(&tracker.Fake{})
		h.do("09:00", "start", "AA-1")
		h.env["AI_AGENT"] = "x"
		h.now = at("09:30")
		h.run(args...)
		if r := h.run("status", "--json"); !strings.Contains(r.stdout, `"state":"running"`) {
			t.Errorf("%v changed the timer: %s", args, r.stdout)
		}
	}
}

func TestStatusShowsTheAuthFailureAfterA401(t *testing.T) {
	h := newHarness(t)
	h.jira = knowing(&tracker.Fake{Err: tracker.ErrUnauthorized})
	h.do("09:00", "start", "AA-1")
	h.do("09:30", "stop")
	r := h.run("status", "--json")
	if !strings.Contains(r.stdout, `"authFailedAt":`+nineThirty) {
		t.Errorf("status = %s", r.stdout)
	}
	h.jira.Err = nil
	h.do("10:00", "sync")
	if r := h.run("status", "--json"); !strings.Contains(r.stdout, `"authFailedAt":null`) {
		t.Errorf("a successful sync must clear it: %s", r.stdout)
	}
}

func TestSyncEnvelopes(t *testing.T) {
	twoOwed := func(h *harness) {
		h.do("08:00", "start", "AA-1")
		h.do("08:30", "stop", "--no-sync")
		h.do("09:00", "start", "AA-2")
		h.do("09:30", "stop", "--no-sync")
	}
	pending := func(key, id, start string) string {
		return fmt.Sprintf(`{"key": %q, "entryId": %q, "worklog": {"state": "pending", "startedAt": %s, "timeSpentSeconds": 1800}}`, key, id, start)
	}
	synced := func(key, id, start, w string) string {
		return fmt.Sprintf(`{"key": %q, "entryId": %q, "worklog": {"state": "synced", "startedAt": %s, "timeSpentSeconds": 1800, "worklogId": %q, "syncedAt": %s}}`,
			key, id, start, w, ten)
	}
	eight := `"2026-08-09T08:00:00.000-0500"`
	cases := []struct {
		name  string
		jira  *tracker.Fake
		setup func(h *harness)
		args  []string
		code  int
		want  string
	}{
		{"all sent", &tracker.Fake{}, twoOwed, []string{"sync"}, 0,
			ok("sync", `{"sent": [`+synced("AA-1", "ID-1", eight, "W1")+`, `+synced("AA-2", "ID-2", nine, "W2")+`], "remaining": []}`,
				`[{"kind": "synced", "key": "AA-1"}, {"kind": "synced", "key": "AA-2"}]`)},
		{"sync KEY sends only that ticket's", &tracker.Fake{}, twoOwed, []string{"sync", "AA-2"}, 0,
			ok("sync", `{"sent": [`+synced("AA-2", "ID-2", nine, "W1")+`], "remaining": []}`, `[{"kind": "synced", "key": "AA-2"}]`)},
		{"some remain: exit 7", &tracker.Fake{PostErrs: map[string]error{"AA-1": &tracker.Refusal{Status: 503}}}, twoOwed, []string{"sync"}, 7,
			fmt.Sprintf(`{"v": 1, "ok": true, "outcome": "sync_pending", "command": "sync",
				"data": {"sent": [%s], "remaining": [{"key": "AA-1", "entryId": "ID-1", "worklog": {"state": "failed", "startedAt": %s, "timeSpentSeconds": 1800, "message": "the tracker answered 503", "refusedStatus": 503, "failedAt": %s}}]},
				"events": [{"kind": "syncFailed", "key": "AA-1", "message": "the tracker answered 503"}, {"kind": "synced", "key": "AA-2"}],
				"error": {"code": "sync_failed", "message": "*", "hint": "run `+"`punchcard sync`"+` again later"}}`,
				synced("AA-2", "ID-2", nine, "W1"), eight, ten)},
		{"a 401 exits 8 and leaves them pending", &tracker.Fake{Err: tracker.ErrUnauthorized}, twoOwed, []string{"sync"}, 8,
			refused("sync", "auth_failed", "auth_failed", "check the email and API token with `punchcard setup`; a scoped token is refused too",
				`{"sent": [], "remaining": [`+pending("AA-1", "ID-1", eight)+`, `+pending("AA-2", "ID-2", nine)+`]}`)},
		{"unconfigured exits 3", nil, twoOwed, []string{"sync"}, 3,
			refused("sync", "not_configured", "not_configured", "run `punchcard setup`",
				`{"sent": [], "remaining": [`+pending("AA-1", "ID-1", eight)+`, `+pending("AA-2", "ID-2", nine)+`]}`)},
		{"nothing owed is 0 even unconfigured", nil, nil, []string{"sync"}, 0, ok("sync", `{"sent": [], "remaining": []}`, noEvents)},
		{"retry is sync", &tracker.Fake{}, twoOwed, []string{"retry", "AA-1"}, 0,
			ok("retry", `{"sent": [`+synced("AA-1", "ID-1", eight, "W1")+`], "remaining": []}`, `[{"kind": "synced", "key": "AA-1"}]`)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			h.jira = knowing(c.jira)
			if c.setup != nil {
				c.setup(h)
			}
			h.now = at("10:00")
			r := h.run(append(c.args, "--json")...)
			if r.code != c.code {
				t.Errorf("exit %d, want %d\nstderr %q", r.code, c.code, r.stderr)
			}
			if d := cmp.Diff(decode(t, c.want), decode(t, r.stdout), anyMessage); d != "" {
				t.Errorf("envelope (-want +got):\n%s", d)
			}
		})
	}
}

func TestSyncWhileAnotherSendsIsBusy(t *testing.T) {
	h := newHarness(t)
	h.jira = knowing(&tracker.Fake{})
	h.do("09:00", "start", "AA-1")
	h.do("09:30", "stop", "--no-sync")
	release, err := h.st.LockSync()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	r := h.run("sync", "--json")
	if r.code != 75 || posts(h.jira) != 0 {
		t.Errorf("exit %d calls %v, want 75 and no POST", r.code, h.jira.Calls())
	}
}

func TestStopPromptsOnATTY(t *testing.T) {
	cases := []struct {
		name, stdin string
		args        []string
		at          string
		recovered   bool
		code        int
		wantSecs    int
		wantComment string
		wantPrompt  string
	}{
		{"enter accepts the measured time", "\n\n", []string{"stop"}, "09:30", false, 0, 1800, "", "Duration [30m]"},
		{"a typed duration and comment", "1h\nfixed it\n", []string{"stop"}, "09:30", false, 0, 3600, "fixed it", "Duration [30m]"},
		{"flags prefill the prompt", "\n\n", []string{"stop", "--duration", "45m", "-m", "pre"}, "09:30", false, 0, 2700, "pre", "Duration [45m]"},
		{"a bad duration asks again", "1:30\n90m\n\n", []string{"stop"}, "09:30", false, 0, 5400, "", "use 1h30m"},
		{"under a minute has no default", "\n5\n\n", []string{"stop"}, "09:00", false, 0, 300, "", "Duration: "},
		{"a recovered entry is prompted, not refused", "\n\n", []string{"stop"}, "13:00", true, 0, 2700, "", "recovered"},
		{"--yes skips the prompt", "", []string{"stop", "--yes"}, "09:30", false, 0, 1800, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			h.jira = knowing(&tracker.Fake{})
			h.do("09:00", "start", "AA-1")
			if c.recovered {
				h.do("09:45", "touch")
			}
			h.now = at(c.at)
			r := h.runTTY(c.stdin, nil, c.args...)
			if r.code != c.code {
				t.Fatalf("exit %d, want %d\nstdout %q\nstderr %q", r.code, c.code, r.stdout, r.stderr)
			}
			wl := h.jira.Worklogs()
			if len(wl) != 1 || wl[0].Worklog.TimeSpentSeconds != c.wantSecs || wl[0].Worklog.Comment != c.wantComment {
				t.Errorf("posted %+v, want %d s %q", wl, c.wantSecs, c.wantComment)
			}
			if c.wantPrompt != "" && !strings.Contains(r.stderr, c.wantPrompt) {
				t.Errorf("stderr %q lacks %q", r.stderr, c.wantPrompt)
			}
			if c.wantPrompt == "" && strings.Contains(r.stderr, "Duration") {
				t.Errorf("--yes prompted: %q", r.stderr)
			}
			if !strings.Contains(r.stdout, "AA-1 stopped") || !strings.Contains(r.stdout, "sent to Jira") {
				t.Errorf("stdout %q", r.stdout)
			}
		})
	}
}

func TestStopPromptCancelledAtEOFChangesNothing(t *testing.T) {
	h := newHarness(t)
	h.jira = knowing(&tracker.Fake{})
	h.do("09:00", "start", "AA-1")
	h.now = at("09:30")
	r := h.runTTY("", nil, "stop")
	if r.code != 0 || posts(h.jira) != 0 || !strings.Contains(r.stderr, "not stopped") {
		t.Errorf("exit %d calls %v stderr %q", r.code, h.jira.Calls(), r.stderr)
	}
	if s := h.run("status", "--json"); !strings.Contains(s.stdout, `"state":"running"`) {
		t.Errorf("cancel changed the timer: %s", s.stdout)
	}
}

func TestStopUnsentHumanOutput(t *testing.T) {
	h := newHarness(t)
	h.jira = knowing(&tracker.Fake{Err: tracker.ErrUnreachable})
	h.do("09:00", "start", "AA-1")
	h.now = at("09:30")
	r := h.run("stop")
	if r.code != 7 {
		t.Fatalf("exit %d", r.code)
	}
	wantLines(t, r.stdout, "AA-1 stopped: 30m")
	wantLines(t, r.stderr, "could not reach the tracker", "punchcard sync")
}

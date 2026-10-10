package cli_test

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/j-dsmith/punchcard/internal/config"
)

// noTracker is a harness with no tracker in effect. Its Jira fake is
// there only to record that nothing ever asks it.
func noTracker(t *testing.T) *harness {
	h := newHarness(t)
	h.tracker = config.TrackerNone
	h.jira = knows()
	return h
}

func (h *harness) neverAsked() {
	h.t.Helper()
	if calls := h.jira.Calls(); len(calls) != 0 {
		h.t.Errorf("the tracker was asked with none in effect: %v", calls)
	}
}

const localPayroll = `{"state": "local", "startedAt": ` + nine + `, "timeSpentSeconds": 1800}`

// With no tracker a stop is final: the worklog is local, exit 0, and
// nothing is owed.
func TestWithoutATrackerStopLogsLocally(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"stop", []string{"stop", "--yes", "--duration", "30"},
			ok("stop", `{"key": "payroll", "entryId": "ID-1", "worklog": `+localPayroll+`}`, noEvents)},
		{"stop --no-sync", []string{"stop", "--no-sync", "--duration", "30"},
			ok("stop", `{"key": "payroll", "entryId": "ID-1", "worklog": `+localPayroll+`}`, noEvents)},
		{"stop --dry-run", []string{"stop", "--dry-run", "--duration", "30"},
			ok("stop", `{"preview": {"key": "payroll", "entryId": "ID-1", "recovered": false, "worklog": `+localPayroll+`}}`, noEvents)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := noTracker(t)
			h.do("09:00", "start", "payroll")
			h.now = at("10:00")
			r := h.run(append(c.args, "--json")...)
			if r.code != 0 {
				t.Errorf("exit %d, want 0\nstderr %q", r.code, r.stderr)
			}
			if d := cmp.Diff(decode(t, c.want), decode(t, r.stdout), anyMessage); d != "" {
				t.Errorf("envelope (-want +got):\n%s", d)
			}
			h.neverAsked()
		})
	}
}

// With no tracker, nothing punchcard prints names Jira or asks for a
// sync: a stop logs.
func TestWithoutATrackerNothingSaysJira(t *testing.T) {
	h := noTracker(t)
	var all strings.Builder
	say := func(want string, args ...string) {
		t.Helper()
		r := h.run(args...)
		all.WriteString(r.stdout + r.stderr)
		if !strings.Contains(r.stdout+r.stderr, want) {
			t.Errorf("%v says %q, want %q in it", args, r.stdout+r.stderr, want)
		}
	}
	h.now = at("09:00")
	say("payroll running", "start", "payroll")
	h.now = at("09:30")
	say("payroll: would log 30m", "stop", "--dry-run")
	say("refused while it has unlogged time", "rm", "--help")
	say("stop it first, or pass --force", "rm", "payroll")
	h.env["CLAUDECODE"] = "1"
	say("only `punchcard stop --yes` logs it", "stop")
	delete(h.env, "CLAUDECODE")
	say("payroll stopped: 30m, logged", "stop", "--yes")
	say("Nothing running.", "status")
	say("punchcard", "--help")
	say("Stop an entry and log its worklog", "stop", "--help")
	say("tracker", "sync", "--help")
	say("no tracker", "sync")
	for _, word := range []string{"Jira", "sync it", "unsent time", "sends it"} {
		if strings.Contains(all.String(), word) {
			t.Errorf("with no tracker the output says %q:\n%s", word, all.String())
		}
	}
}

// A local worklog is logged time: counted today, owed to no one, and
// no reason to keep a ticket.
func TestWithoutATrackerNothingIsOwed(t *testing.T) {
	h := noTracker(t)
	h.do("09:00", "start", "payroll")
	h.do("09:30", "stop", "--yes")
	h.now = at("10:00")

	if r := h.run("status", "--json"); r.code != 0 {
		t.Errorf("status: exit %d", r.code)
	} else if d := cmp.Diff(decode(t, `{"count": 0, "seconds": 0}`), decode(t, r.stdout)["data"].(map[string]any)["unsent"]); d != "" {
		t.Errorf("status unsent (-want +got):\n%s", d)
	}

	var ls struct {
		Data struct {
			Tickets []map[string]any `json:"tickets"`
		} `json:"data"`
	}
	decodeInto(t, h.run("ls", "--json").stdout, &ls)
	if s := ls.Data.Tickets[0]["todaySeconds"]; s != float64(1800) {
		t.Errorf("ls todaySeconds = %v, want 1800", s)
	}
	if u := ls.Data.Tickets[0]["unsynced"]; u != float64(0) {
		t.Errorf("ls unsynced = %v, want 0", u)
	}

	r := h.run("sync", "--json")
	if r.code != 3 {
		t.Errorf("sync: exit %d, want 3", r.code)
	}
	if d := cmp.Diff(decode(t, refused("sync", "not_configured", "not_configured",
		"worklogs are logged here; `punchcard setup` connects one", `{"sent": [], "remaining": []}`)),
		decode(t, r.stdout), anyMessage); d != "" {
		t.Errorf("sync envelope (-want +got):\n%s", d)
	}
	if r := h.run("sync"); !strings.Contains(r.stderr, "no tracker") || strings.Contains(r.stderr, "Jira") {
		t.Errorf("sync says %q, want no tracker and never Jira", r.stderr)
	}

	if r := h.run("rm", "payroll"); r.code != 0 || r.stderr != "" {
		t.Errorf("rm: exit %d stderr %q, want 0 and no warning", r.code, r.stderr)
	}
	h.neverAsked()
}

// With no tracker the summary is what the user typed: add --summary
// records it, and ls shows it.
func TestWithoutATrackerAddTakesATypedSummary(t *testing.T) {
	h := noTracker(t)
	r := h.run("add", "payroll", "--summary", "  Monthly payroll run ", "--json")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	var add struct {
		Data struct {
			Tickets []map[string]any `json:"tickets"`
		} `json:"data"`
	}
	decodeInto(t, r.stdout, &add)
	if got := add.Data.Tickets[0]["summary"]; got != "Monthly payroll run" {
		t.Errorf("summary %v, want Monthly payroll run", got)
	}
	if r := h.run("ls"); !strings.Contains(r.stdout, "Monthly payroll run") {
		t.Errorf("ls does not show the summary:\n%s", r.stdout)
	}
	h.neverAsked()
}

// --summary names one ticket, and with a tracker the summary is the
// tracker's: both are usage, and nothing is added.
func TestAddSummaryUsage(t *testing.T) {
	cases := []struct {
		name string
		h    func(*testing.T) *harness
		args []string
	}{
		{"two keys", noTracker, []string{"add", "payroll", "invoices", "--summary", "x"}},
		{"with Jira", newHarness, []string{"add", "AA-1", "--summary", "x"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := c.h(t)
			r := h.run(append(c.args, "--json")...)
			if r.code != 64 {
				t.Fatalf("exit %d, want 64\n%s%s", r.code, r.stdout, r.stderr)
			}
			if r := h.run("ls"); strings.Contains(r.stdout, "added") || !strings.Contains(r.stdout, "No tickets yet") {
				t.Errorf("a refused add changed the list:\n%s", r.stdout)
			}
		})
	}
}

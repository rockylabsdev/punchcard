package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/j-dsmith/punchcard/internal/tracker"
)

// knows is a Jira that has these tickets (key, summary, key, summary...).
func knows(kv ...string) *tracker.Fake {
	f := &tracker.Fake{Summaries: map[string]string{}}
	for i := 0; i < len(kv); i += 2 {
		f.Summaries[kv[i]] = kv[i+1]
	}
	return f
}

const verifiedAA1 = `{"key": "AA-1", "summary": "Fix the thing", "unverified": false, "entryId": "ID-1", "state": "running", "byHand": true, "origin": "hand", "recovered": false, "elapsedSeconds": 0}`

func TestStartFetchesTheSummaryOfANewTicket(t *testing.T) {
	cases := []struct {
		name      string
		jira      *tracker.Fake
		setup     func(h *harness)
		args      []string
		code      int
		want      string
		wantCalls []string
	}{
		{"a new ticket gets its real summary", knows("AA-1", "Fix the thing"), nil,
			[]string{"start", "AA-1"}, 0,
			ok("start", status(verifiedAA1), `[{"kind": "added", "key": "AA-1"}]`),
			[]string{"summary AA-1"}},
		{"a typo exits 5 and changes nothing", knows(), nil,
			[]string{"start", "AA-1"}, 5,
			refused("start", "ticket_not_found", "ticket_not_found", notFoundHint, "null"),
			[]string{"summary AA-1"}},
		{"unreachable starts anyway, unverified", &tracker.Fake{Err: tracker.ErrUnreachable}, nil,
			[]string{"start", "AA-1"}, 0,
			ok("start", status(running("AA-1", "ID-1", true, "hand", 0)), `[{"kind": "added", "key": "AA-1"}]`),
			[]string{"summary AA-1"}},
		{"a ticket already on the list does not fetch", knows("AA-1", "Fix the thing"),
			func(h *harness) { h.do("08:00", "start", "AA-1"); h.do("08:30", "pause") },
			[]string{"start", "AA-1"}, 0,
			ok("start", status(`{"key": "AA-1", "summary": "Fix the thing", "unverified": false, "entryId": "ID-1", "state": "running", "byHand": true, "origin": "hand", "recovered": false, "elapsedSeconds": 1800}`), noEvents),
			[]string{"summary AA-1"}}, // the setup's start only
		{"an unverified ticket is confirmed by a later start", &tracker.Fake{Err: tracker.ErrUnreachable},
			func(h *harness) {
				h.do("09:00", "add", "AA-1")
				h.jira.Err = nil
				h.jira.Summaries = map[string]string{"AA-1": "Fix the thing"}
			},
			[]string{"start", "AA-1"}, 0,
			ok("start", status(verifiedAA1), noEvents),
			[]string{"summary AA-1", "summary AA-1"}},
		{"an unverified ticket that now 404s still starts, unverified", &tracker.Fake{Err: tracker.ErrUnreachable},
			func(h *harness) { h.do("09:00", "add", "AA-1"); h.jira.Err = nil },
			[]string{"start", "AA-1"}, 0,
			ok("start", status(running("AA-1", "ID-1", true, "hand", 0)), noEvents),
			[]string{"summary AA-1", "summary AA-1"}},
		{"resume KEY fetches like start", knows(), nil,
			[]string{"resume", "AA-1"}, 5,
			refused("resume", "ticket_not_found", "ticket_not_found", notFoundHint, "null"),
			[]string{"summary AA-1"}},
		{"add with one typo adds nothing", knows("AA-1", "Fix the thing"), nil,
			[]string{"add", "AA-1", "AA-2"}, 5,
			refused("add", "ticket_not_found", "ticket_not_found", notFoundHint, "null"),
			[]string{"summary AA-1", "summary AA-2"}},
		{"add records real summaries", knows("AA-1", "Fix the thing"), nil,
			[]string{"add", "AA-1"}, 0,
			ok("add", `{"tickets": [{"key": "AA-1", "summary": "Fix the thing", "unverified": false, "addedAt": `+nine+`, "elapsedSeconds": 0, "todaySeconds": 0, "unsynced": 0}]}`, noEvents),
			[]string{"summary AA-1"}},
		{"a bad key never reaches Jira", knows(), nil,
			[]string{"start", "x"}, 64, refused("start", "usage", "usage", "", "null"), nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			h.jira = c.jira
			if c.setup != nil {
				c.setup(h)
			}
			h.now = at("09:00")
			r := h.run(append(c.args, "--json")...)
			if r.code != c.code {
				t.Errorf("exit %d, want %d\nstderr %q", r.code, c.code, r.stderr)
			}
			if d := cmp.Diff(decode(t, c.want), decode(t, r.stdout), anyMessage); d != "" {
				t.Errorf("envelope (-want +got):\n%s", d)
			}
			if d := cmp.Diff(c.wantCalls, c.jira.Calls()); d != "" {
				t.Errorf("Jira calls (-want +got):\n%s", d)
			}
		})
	}
}

const notFoundHint = "check the key; if it is right, the ticket may exist but be hidden from this account. Nothing changed."

func TestATypoLeavesTheListUnchanged(t *testing.T) {
	h := newHarness(t)
	h.jira = knows("AA-1", "Fix the thing")
	h.do("08:00", "add", "AA-1")
	before := h.run("ls", "--json").stdout
	if r := h.run("start", "AA-9"); r.code != 5 {
		t.Fatalf("exit %d, want 5", r.code)
	}
	if after := h.run("ls", "--json").stdout; after != before {
		t.Errorf("ls changed:\nbefore %s\nafter  %s", before, after)
	}
}

func TestNotFoundCopyAdmitsTheTicketMayBeInvisible(t *testing.T) {
	h := newHarness(t)
	h.jira = knows()
	r := h.run("start", "AA-9")
	if r.code != 5 || !strings.Contains(r.stderr, "AA-9") || !strings.Contains(r.stderr, "see") {
		t.Errorf("exit %d, stderr %q", r.code, r.stderr)
	}
	if strings.Contains(r.stderr, "does not exist") {
		t.Errorf("copy asserts the ticket is missing: %q", r.stderr)
	}
}

func TestStartShowsTheRealSummary(t *testing.T) {
	h := newHarness(t)
	h.jira = knows("AA-1", "Fix the thing")
	r := h.run("start", "AA-1")
	if r.code != 0 || !strings.Contains(r.stdout, "+ AA-1 added\n") || !strings.Contains(r.stdout, "Fix the thing") {
		t.Errorf("exit %d, stdout %q", r.code, r.stdout)
	}
}

func TestA401LeavesItUnverifiedAndRecordsTheAuthFailure(t *testing.T) {
	h := newHarness(t)
	h.jira = &tracker.Fake{Err: tracker.ErrUnauthorized}
	r := h.run("start", "AA-1", "--json")
	got := decode(t, r.stdout)
	data := got["data"].(map[string]any)
	if r.code != 0 || data["running"].(map[string]any)["unverified"] != true || data["authFailedAt"] != nine[1:len(nine)-1] {
		t.Errorf("exit %d, data %v", r.code, data)
	}
}

func TestReadsNeverCallJira(t *testing.T) {
	h := newHarness(t)
	h.jira = &tracker.Fake{Err: tracker.ErrUnreachable}
	h.do("09:00", "start", "AA-1") // unverified
	h.jira = knows("AA-1", "Fix the thing")
	for _, args := range [][]string{{"status"}, {"status", "--short"}, {"ls"}, {}, {"pause"}, {"resume"}, {"touch", "AA-1", "--origin", "prompt"}, {"touch", "AA-2", "--origin", "prompt"}, {"log"}, {"log", "--csv"}, {"log", "--json"}} {
		h.run(args...)
	}
	if calls := h.jira.Calls(); len(calls) != 0 {
		t.Errorf("Jira was called: %v", calls)
	}
}

// Unconfigured is offline: with no config file at all, start leaves the
// ticket unverified; a config file and PUNCHCARD_TOKEN are what the
// default lookup needs (proven against a server in the jira package).
func TestUnconfiguredStartsUnverified(t *testing.T) {
	h := newHarness(t)
	h.env["XDG_CONFIG_HOME"] = t.TempDir()
	h.env["PUNCHCARD_TOKEN"] = "tok"
	r := h.run("start", "AA-1", "--json")
	if r.code != 0 || !strings.Contains(r.stdout, `"unverified":true`) {
		t.Errorf("exit %d, stdout %s", r.code, r.stdout)
	}
	if r.stderr != "" {
		t.Errorf("unconfigured is quiet: %q", r.stderr)
	}
}

// A config file punchcard cannot use is said on stderr, then treated as
// unconfigured.
func TestABrokenConfigIsReportedAndStartsUnverified(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "punchcard"), 0o700)
	writeFile(t, filepath.Join(dir, "punchcard"), "config.json", `{"schemaVersion": 9, "siteUrl": "https://x", "email": "e"}`)
	h.env["XDG_CONFIG_HOME"] = dir
	r := h.run("start", "AA-1")
	if r.code != 0 || !strings.Contains(r.stderr, "newer") {
		t.Errorf("exit %d, stderr %q", r.code, r.stderr)
	}
}

// #42: a ticket added while Jira was unreachable is confirmed, with its
// summary, once Jira accepts a worklog for it.
func TestSyncConfirmsAnUnverifiedTicket(t *testing.T) {
	h := newHarness(t)
	h.jira = &tracker.Fake{Err: tracker.ErrUnreachable}
	h.do("09:00", "start", "NEW-1")
	h.do("09:30", "stop") // exit 7: owed
	h.jira = knows("NEW-1", "Fix the thing")
	h.now = at("10:00")
	if r := h.run("sync"); r.code != 0 {
		t.Fatalf("sync: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	r := h.run("ls", "--json")
	tickets := decode(t, r.stdout)["data"].(map[string]any)["tickets"].([]any)
	tk := tickets[0].(map[string]any)
	if tk["key"] != "NEW-1" || tk["unverified"] != false || tk["summary"] != "Fix the thing" {
		t.Errorf("ls --json ticket = %v", tk)
	}
}

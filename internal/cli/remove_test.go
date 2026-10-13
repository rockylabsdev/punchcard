package cli_test

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// #56: rm hides a ticket from the list; its logged worklogs stay in the
// log, and start or add restores it with its history.
func TestRmKeepsLoggedWorklogs(t *testing.T) {
	h := noTracker(t)
	h.do("08:00", "start", "payroll")
	h.do("08:30", "stop", "--yes")
	h.do("08:30", "start", "payroll")
	h.do("09:00", "stop", "--yes")
	h.do("09:00", "add", "infra")

	r := h.run("rm", "payroll")
	if r.code != 0 || r.stdout != "- payroll removed (its 2 worklogs stay in the log)\n" {
		t.Fatalf("rm: exit %d stdout %q", r.code, r.stdout)
	}
	if r := h.run("rm", "infra"); r.stdout != "- infra removed\n" {
		t.Errorf("rm with nothing logged: %q", r.stdout)
	}

	if r := h.run("log"); strings.Count(r.stdout, "payroll") < 2 {
		t.Errorf("log lost the removed ticket's worklogs:\n%s", r.stdout)
	}
	if r := h.run("ls"); strings.Contains(r.stdout, "payroll") || strings.Contains(r.stdout, "infra") {
		t.Errorf("ls lists removed tickets:\n%s", r.stdout)
	}
	r = h.run("ls", "--all")
	wantLines(t, r.stdout, "payroll", "removed", "infra")

	r = h.run("start", "payroll")
	if r.code != 0 || !strings.HasPrefix(r.stdout, "+ payroll restored\n") {
		t.Errorf("start of a removed ticket: exit %d stdout %q", r.code, r.stdout)
	}
	if r := h.run("add", "infra"); r.stdout != "+ infra restored\n" {
		t.Errorf("add of a removed ticket: %q", r.stdout)
	}
	if r := h.run("ls"); !strings.Contains(r.stdout, "payroll") || !strings.Contains(r.stdout, "infra") {
		t.Errorf("restored tickets are not on the list:\n%s", r.stdout)
	}
	h.neverAsked()
}

func TestRmJSONCountsKeptWorklogs(t *testing.T) {
	h := noTracker(t)
	h.do("08:00", "start", "payroll")
	h.do("08:30", "stop", "--yes")
	r := h.run("rm", "payroll", "--json")
	if d := cmp.Diff(decode(t, ok("rm", `{"key": "payroll", "keptWorklogs": 1}`, noEvents)), decode(t, r.stdout), anyMessage); d != "" {
		t.Errorf("envelope (-want +got):\n%s", d)
	}
	r = h.run("ls", "--all", "--json")
	rows := decode(t, r.stdout)["data"].(map[string]any)["tickets"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["removedAt"] == nil {
		t.Errorf("ls --all: %s", r.stdout)
	}
	r = h.run("start", "payroll", "--json")
	if !strings.Contains(r.stdout, `"kind":"restored"`) && !strings.Contains(r.stdout, `"kind": "restored"`) {
		t.Errorf("start reports no restored event: %s", r.stdout)
	}
}

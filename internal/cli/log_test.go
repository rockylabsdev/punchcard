package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/golden"
	"github.com/google/go-cmp/cmp"
	"github.com/j-dsmith/punchcard/internal/config"
	"github.com/j-dsmith/punchcard/internal/store"
	"github.com/j-dsmith/punchcard/internal/tracker"
)

// on is a wall-clock time on 2026-08-<d>, in UTC-5. August 2026 starts
// on a Saturday: Monday is the 3rd, Wednesday the 5th, Sunday the 9th.
func on(d int, hhmm string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", fmt.Sprintf("2026-08-%02d %s", d, hhmm), zone)
	if err != nil {
		panic(err)
	}
	return t
}

// doOn runs a setup step on another day of the test month.
func (h *harness) doOn(d int, hhmm string, args ...string) {
	h.t.Helper()
	h.now = on(d, hhmm)
	if r := h.run(args...); r.code != 0 && r.code != 7 {
		h.t.Fatalf("setup %v: exit %d\n%s%s", args, r.code, r.stdout, r.stderr)
	}
}

func TestLogOfAnEmptyRangeIsOK(t *testing.T) {
	h := newHarness(t)
	r := h.run("log", "--json")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	want := map[string]any{"rows": []any{}, "open": nil, "range": map[string]any{"from": "2026-08-03", "to": "2026-08-09"}}
	if d := cmp.Diff(want, decode(t, r.stdout)["data"]); d != "" {
		t.Errorf("data (-want +got):\n%s", d)
	}
}

// awkward is a comment no table or CSV cell can hold unescaped.
const awkward = "Split | \"quoted\", C:\\| and\nnewline"

// week scripts Saturday 1 August through Wednesday the 5th against a fake
// Jira: a worklog on Saturday (last week), a synced one and one spanning
// midnight on Monday, a recovered one with a corrected duration and a
// pending one on Tuesday, a failed one on Wednesday, then a paused entry.
// The clock ends at Wednesday 11:30.
func week(t *testing.T) *harness {
	h := newHarness(t)
	h.jira = &tracker.Fake{Summaries: map[string]string{
		"AA-1": "Fix the thing", "AA-2": "Other thing", "AA-3": "Pipes | in a summary"}}
	h.doOn(1, "10:00", "start", "AA-1")
	h.doOn(1, "11:00", "stop", "-m", "last week")
	h.doOn(3, "09:00", "start", "AA-1")
	h.doOn(3, "10:30", "stop", "-m", "Fixed the formatter")
	h.doOn(3, "23:30", "start", "AA-2")
	h.doOn(4, "00:45", "stop", "-m", awkward)
	h.doOn(4, "09:00", "start", "AA-1")
	h.doOn(4, "09:45", "touch")
	h.doOn(4, "13:00", "stop", "--duration", "2h", "-m", "Recovered morning")
	h.doOn(4, "14:00", "start", "AA-3")
	h.doOn(4, "14:30", "stop", "--no-sync")
	h.jira.PostErrs = map[string]error{"AA-2": &tracker.Refusal{Status: 503}}
	h.doOn(5, "09:00", "start", "AA-2")
	h.doOn(5, "10:00", "stop", "-m", "Tried")
	h.jira.PostErrs = nil
	h.doOn(5, "10:15", "start", "AA-1")
	h.doOn(5, "11:00", "pause")
	h.now = on(5, "11:30")
	return h
}

func TestLogJSONOfAWeek(t *testing.T) {
	h := week(t)
	r := h.run("log", "--json")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	row := func(date, started, key, summary string, minutes int, comment, state, remote, entry string, recovered bool) map[string]any {
		return map[string]any{"date": date, "started": started, "key": key, "summary": summary, "minutes": float64(minutes),
			"comment": comment, "state": state, "remoteId": remote, "entryId": entry, "recovered": recovered}
	}
	want := map[string]any{
		"rows": []any{
			row("2026-08-03", "2026-08-03T09:00:00.000-0500", "AA-1", "Fix the thing", 90, "Fixed the formatter", "synced", "W2", "ID-2", false),
			row("2026-08-03", "2026-08-03T23:30:00.000-0500", "AA-2", "Other thing", 75, awkward, "synced", "W3", "ID-3", false),
			row("2026-08-04", "2026-08-04T09:00:00.000-0500", "AA-1", "Fix the thing", 120, "Recovered morning", "synced", "W4", "ID-4", true),
			row("2026-08-04", "2026-08-04T14:00:00.000-0500", "AA-3", "Pipes | in a summary", 30, "", "pending", "", "ID-6", false),
			row("2026-08-05", "2026-08-05T09:00:00.000-0500", "AA-2", "Other thing", 60, "Tried", "failed", "", "ID-7", false),
		},
		"open": map[string]any{"key": "AA-1", "summary": "Fix the thing", "unverified": false, "entryId": "ID-8",
			"state": "paused", "held": true, "origin": "hand", "recovered": false, "elapsedSeconds": 2700.0},
		"range": map[string]any{"from": "2026-08-03", "to": "2026-08-05"},
	}
	if d := cmp.Diff(want, decode(t, r.stdout)["data"]); d != "" {
		t.Errorf("data (-want +got):\n%s", d)
	}
}

// TestLogGoldens pins the Markdown and CSV of the scripted week and of an
// empty week. The goldens were written by hand; refresh them with -update
// only for a deliberate change, and read the diff.
func TestLogGoldens(t *testing.T) {
	for _, c := range []struct {
		name  string
		empty bool
		args  []string
	}{
		{"md", false, []string{"log", "--md"}},
		{"csv", false, []string{"log", "--csv"}},
		{"empty_md", true, []string{"log", "--md"}},
		{"empty_csv", true, []string{"log", "--csv"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			if !c.empty {
				h = week(t)
			}
			r := h.run(c.args...)
			if r.code != 0 || r.stderr != "" {
				t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
			}
			golden.RequireEqual(t, r.stdout)
		})
	}
}

func TestLogWithNoFormatFlagIsTheMarkdown(t *testing.T) {
	h := week(t)
	if plain, md := h.run("log").stdout, h.run("log", "--md").stdout; plain != md {
		t.Errorf("plain log differs from --md:\n%s\n---\n%s", plain, md)
	}
}

// TestLogRanges runs the scripted week (rows on the 1st, 3rd, 4th and
// 5th) and checks which days each range covers.
func TestLogRanges(t *testing.T) {
	cases := []struct {
		name     string
		now      time.Time
		args     []string
		from, to string
		days     []string
	}{
		{"--week on a Wednesday is Monday through Wednesday", on(5, "11:30"), []string{"--week"},
			"2026-08-03", "2026-08-05", []string{"2026-08-03", "2026-08-03", "2026-08-04", "2026-08-04", "2026-08-05"}},
		{"the default is the week", on(5, "11:30"), nil,
			"2026-08-03", "2026-08-05", []string{"2026-08-03", "2026-08-03", "2026-08-04", "2026-08-04", "2026-08-05"}},
		{"on a Monday the week is one day", on(3, "23:59"), nil,
			"2026-08-03", "2026-08-03", []string{"2026-08-03", "2026-08-03"}},
		{"--month on the 9th is the 1st through the 9th", on(9, "12:00"), []string{"--month"},
			"2026-08-01", "2026-08-09", []string{"2026-08-01", "2026-08-03", "2026-08-03", "2026-08-04", "2026-08-04", "2026-08-05"}},
		{"--from and --to are inclusive at both ends", on(9, "12:00"), []string{"--from", "2026-08-01", "--to", "2026-08-03"},
			"2026-08-01", "2026-08-03", []string{"2026-08-01", "2026-08-03", "2026-08-03"}},
		{"--from alone runs through today", on(5, "11:30"), []string{"--from", "2026-08-04"},
			"2026-08-04", "2026-08-05", []string{"2026-08-04", "2026-08-04", "2026-08-05"}},
		{"--to alone starts on its own week's Monday", on(5, "11:30"), []string{"--to", "2026-08-04"},
			"2026-08-03", "2026-08-04", []string{"2026-08-03", "2026-08-03", "2026-08-04", "2026-08-04"}},
		{"--to alone in a past week is that week through --to", on(5, "11:30"), []string{"--to", "2026-08-01"},
			"2026-07-27", "2026-08-01", []string{"2026-08-01"}},
		{"--from alone in the future is that one day", on(5, "11:30"), []string{"--from", "2026-08-07"},
			"2026-08-07", "2026-08-07", nil},
		{"one day", on(9, "12:00"), []string{"--from", "2026-08-02", "--to", "2026-08-02"},
			"2026-08-02", "2026-08-02", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := week(t)
			h.now = c.now
			r := h.run(append([]string{"log", "--json"}, c.args...)...)
			if r.code != 0 {
				t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
			}
			data := decode(t, r.stdout)["data"].(map[string]any)
			var days []string
			for _, row := range data["rows"].([]any) {
				days = append(days, row.(map[string]any)["date"].(string))
			}
			if d := cmp.Diff(c.days, days); d != "" {
				t.Errorf("days (-want +got):\n%s", d)
			}
			if d := cmp.Diff(map[string]any{"from": c.from, "to": c.to}, data["range"]); d != "" {
				t.Errorf("range (-want +got):\n%s", d)
			}
		})
	}
}

func TestLogUsage(t *testing.T) {
	for _, args := range [][]string{
		{"--from", "2026-08-05", "--to", "2026-08-03"},
		{"--from", "8/3/2026"},
		{"--to", "2026-08-32"},
		{"--week", "--month"},
		{"--month", "--from", "2026-08-01"},
		{"--csv", "--md"},
		{"--csv", "--json"},
		{"KEY"},
	} {
		t.Run(fmt.Sprint(args), func(t *testing.T) {
			h := newHarness(t)
			h.now = on(5, "11:30")
			r := h.run(append([]string{"log", "--json"}, args...)...)
			if r.code != 64 {
				t.Fatalf("exit %d, want 64\n%s%s", r.code, r.stdout, r.stderr)
			}
			if m := decode(t, r.stdout); m["outcome"] != "usage" || m["command"] != "log" {
				t.Errorf("envelope %s", r.stdout)
			}
		})
	}
}

// log is a read: over a stale running timer (which any change would
// recover) it writes nothing, never moves lastSeen and asks no tracker.
func TestLogWritesNothingAndCallsNoTracker(t *testing.T) {
	h := week(t)
	h.doOn(5, "11:30", "start", "AA-2")
	h.now = on(5, "15:00")
	files := func() string {
		var s string
		for _, name := range []string{store.StateFile, store.HeartbeatFile} {
			b, err := os.ReadFile(filepath.Join(h.dir, name))
			if err != nil {
				t.Fatal(err)
			}
			s += string(b)
		}
		return s
	}
	before, calls := files(), len(h.jira.Calls())
	for _, args := range [][]string{{"log"}, {"log", "--csv"}, {"log", "--json"}, {"log", "--month"}} {
		if r := h.run(args...); r.code != 0 {
			t.Fatalf("%v: exit %d\n%s", args, r.code, r.stderr)
		}
	}
	if files() != before {
		t.Error("log wrote to disk")
	}
	if got := h.jira.Calls(); len(got) != calls {
		t.Errorf("log called the tracker: %v", got[calls:])
	}
	if r := h.run("log"); !strings.HasSuffix(r.stdout, "\nRunning: AA-2 0:00, not logged\n") {
		t.Errorf("a stale timer is still the running entry, capped at lastSeen:\n%s", r.stdout)
	}
}

// Without a tracker a row is local: no debt in the heading or the cell.
func TestLogWithoutATrackerIsLocal(t *testing.T) {
	h := newHarness(t)
	h.tracker = config.TrackerNone
	h.doOn(3, "09:00", "start", "payroll")
	h.doOn(3, "09:30", "stop", "-m", "March run")
	h.now = on(3, "10:00")
	wantLines(t, h.run("log").stdout, "## 2026-08-03 Mon · 0h 30m\n", "| payroll |  | 0:30 | March run |\n", "**Total: 0h 30m**\n")
	wantLines(t, h.run("log", "--csv").stdout, "2026-08-03,09:00,payroll,,30,March run,local,,ID-1,false\r\n")
}

// An agent with PUNCHCARD_JSON=1 that asks for CSV by name gets CSV.
func TestLogFormatFlagBeatsTheJSONEnvironment(t *testing.T) {
	h := newHarness(t)
	h.env["PUNCHCARD_JSON"] = "1"
	if r := h.run("log", "--csv"); r.code != 0 || !strings.HasPrefix(r.stdout, "date,started,") {
		t.Errorf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
}

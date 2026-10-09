package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// fromConfig makes the harness resolve the tracker the way the binary
// does, from the config file, with configJSON as that file ("" for
// none at all).
func (h *harness) fromConfig(configJSON string) {
	h.t.Helper()
	h.tracker = ""
	dir := filepath.Join(h.t.TempDir(), "punchcard")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		h.t.Fatal(err)
	}
	if configJSON != "" {
		writeFile(h.t, dir, "config.json", configJSON)
	}
	h.env["XDG_CONFIG_HOME"] = filepath.Dir(dir)
}

// status --json names the tracker in effect: no file is none, a file
// with site and email and no tracker field is Jira, the field wins.
func TestStatusNamesTheTracker(t *testing.T) {
	cases := []struct {
		name, config string
		want         string
	}{
		{"no config file", "", "none"},
		{"site and email, no tracker field", `{"schemaVersion": 1, "siteUrl": "https://example.atlassian.net", "email": "user@example.com"}`, "jira"},
		{"tracker none", `{"schemaVersion": 1, "tracker": "none"}`, "none"},
		{"tracker none beside old Jira fields", `{"schemaVersion": 1, "tracker": "none", "siteUrl": "https://example.atlassian.net", "email": "user@example.com"}`, "none"},
		{"tracker jira", `{"schemaVersion": 1, "tracker": "jira", "siteUrl": "https://example.atlassian.net", "email": "user@example.com"}`, "jira"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			h.fromConfig(c.config)
			r := h.run("status", "--json")
			if r.code != 0 {
				t.Fatalf("exit %d: %s", r.code, r.stderr)
			}
			data := decode(t, r.stdout)["data"].(map[string]any)
			if data["tracker"] != c.want {
				t.Errorf("tracker = %v, want %q", data["tracker"], c.want)
			}
			_, hasConfigured := data["configured"]
			if hasConfigured != (c.want == "jira") {
				t.Errorf("configured present = %t with tracker %s", hasConfigured, c.want)
			}
		})
	}
}

// With no HOME at all there is no config file either: no tracker.
func TestNoConfigDirIsNoTracker(t *testing.T) {
	h := newHarness(t)
	h.tracker = ""
	data := decode(t, h.run("status", "--json").stdout)["data"].(map[string]any)
	if data["tracker"] != "none" {
		t.Errorf("tracker = %v", data["tracker"])
	}
}

// Without a tracker a key is a slug, kept as typed, never looked up and
// never unverified.
func TestWithoutATrackerAKeyIsASlug(t *testing.T) {
	h := newHarness(t)
	h.fromConfig("")
	h.jira = knows() // must never be asked
	for _, step := range [][]string{{"start", "payroll"}, {"pause"}, {"resume"}, {"start", "INV-2041"}, {"start", "v1.2_rc-3"}} {
		if r := h.run(step...); r.code != 0 {
			t.Fatalf("%v: exit %d %s", step, r.code, r.stderr)
		}
	}
	var ls struct {
		Data struct {
			Tickets []map[string]any `json:"tickets"`
		} `json:"data"`
	}
	decodeInto(t, h.run("ls", "--json").stdout, &ls)
	var got []string
	for _, tk := range ls.Data.Tickets {
		if tk["unverified"] != false {
			t.Errorf("%v is unverified", tk["key"])
		}
		if _, ok := tk["summary"]; ok {
			t.Errorf("%v has a summary", tk["key"])
		}
		got = append(got, tk["key"].(string))
	}
	sort.Strings(got)
	if d := cmp.Diff([]string{"INV-2041", "payroll", "v1.2_rc-3"}, got); d != "" {
		t.Errorf("keys (-want +got):\n%s", d)
	}
	for _, bad := range []string{"pay roll", "a/b", "-payroll", ".x"} {
		if r := h.run("start", bad); r.code != 64 {
			t.Errorf("start %q: exit %d, want 64", bad, r.code)
		}
	}
	if calls := h.jira.Calls(); len(calls) != 0 {
		t.Errorf("the tracker was asked without one configured: %v", calls)
	}
}

// With Jira a key is upper-cased on entry, then looked up.
func TestWithJiraAKeyIsUpperCasedOnEntry(t *testing.T) {
	h := newHarness(t)
	h.jira = knows("ABC-123", "Fix the thing")
	if r := h.run("start", "abc-123"); r.code != 0 {
		t.Fatalf("exit %d %s", r.code, r.stderr)
	}
	if d := cmp.Diff([]string{"summary ABC-123"}, h.jira.Calls()); d != "" {
		t.Errorf("calls (-want +got):\n%s", d)
	}
	r := h.run("status", "--json")
	data := decode(t, r.stdout)["data"].(map[string]any)
	if run := data["running"].(map[string]any); run["key"] != "ABC-123" {
		t.Errorf("running %v", run["key"])
	}
	if data["tracker"] != "jira" {
		t.Errorf("tracker = %v", data["tracker"])
	}
	if r := h.run("start", "payroll"); r.code != 64 {
		t.Errorf("start payroll with Jira: exit %d, want 64", r.code)
	}
}

func decodeInto(t *testing.T, s string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(s), v); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, s)
	}
}

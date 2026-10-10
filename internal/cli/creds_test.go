package cli_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/j-dsmith/punchcard/internal/tracker"
)

// configure writes a config file for user@example.com (and a token file
// when token is not ""), and points the harness at it.
func (h *harness) configure(token string) string {
	h.t.Helper()
	dir := filepath.Join(h.t.TempDir(), "punchcard")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		h.t.Fatal(err)
	}
	writeFile(h.t, dir, "config.json", `{"schemaVersion": 1, "siteUrl": "https://example.atlassian.net", "email": "user@example.com"}`)
	if token != "" {
		writeFile(h.t, dir, "token", token+"\n")
	}
	h.env["XDG_CONFIG_HOME"] = filepath.Dir(dir)
	return filepath.Join(dir, "token")
}

var errLocked = errors.New("security find-generic-password: user interaction is not allowed")

// status reports where the token came from, a stray file, and a broken
// Keychain as not configured with its cause. It never calls Jira.
func TestStatusReportsTheTokenSource(t *testing.T) {
	cases := []struct {
		name     string
		setup    func(h *harness)
		json     []string // substrings of status --json
		human    []string // substrings of status
		notJSON  []string
		notHuman []string
	}{
		{"unconfigured", nil,
			[]string{`"configured":false`}, []string{"Not connected to Jira: neither"},
			[]string{"tokenSource", "keychainError"}, nil},
		{"keychain", func(h *harness) { h.configure(""); h.kc.token = "k" },
			[]string{`"configured":true`, `"tokenSource":"keychain"`}, []string{"Jira token: Keychain"},
			[]string{"strayTokenFile", "keychainError"}, []string{"stray"}},
		{"keychain and a stray file", func(h *harness) { h.configure("f"); h.kc.token = "k" },
			[]string{`"tokenSource":"keychain"`, `"strayTokenFile":"`}, []string{"Jira token: Keychain", "stray token file"},
			nil, nil},
		{"file", func(h *harness) { h.configure("f") },
			[]string{`"configured":true`, `"tokenSource":"file"`}, []string{"Jira token: file"},
			[]string{"strayTokenFile"}, nil},
		{"env", func(h *harness) { h.configure("f"); h.kc.token = "k"; h.env["PUNCHCARD_TOKEN"] = "e" },
			[]string{`"tokenSource":"env"`}, []string{"Jira token: PUNCHCARD_TOKEN"},
			nil, nil},
		{"a broken keychain and no file", func(h *harness) { h.configure(""); h.kc.err = errLocked },
			[]string{`"configured":false`, `"keychainError":"macOS Keychain: security find-generic-password: user interaction is not allowed"`, `"authFailedAt":null`},
			[]string{"Not connected to Jira", "macOS Keychain", "user interaction is not allowed"},
			[]string{"tokenSource"}, nil},
		{"a broken keychain falls back to the file", func(h *harness) { h.configure("f"); h.kc.err = errLocked },
			[]string{`"configured":true`, `"tokenSource":"file"`}, nil,
			[]string{"keychainError"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			h.jira = &tracker.Fake{}
			if c.setup != nil {
				c.setup(h)
			}
			r := h.run("status", "--json")
			if r.code != 0 {
				t.Fatalf("exit %d: %s", r.code, r.stdout)
			}
			out := strings.ReplaceAll(r.stdout, " ", "")
			for _, s := range c.json {
				if !strings.Contains(out, strings.ReplaceAll(s, " ", "")) {
					t.Errorf("status --json lacks %s:\n%s", s, r.stdout)
				}
			}
			for _, s := range c.notJSON {
				if strings.Contains(out, s) {
					t.Errorf("status --json has %s:\n%s", s, r.stdout)
				}
			}
			hr := h.run("status")
			wantLines(t, hr.stdout, c.human...)
			for _, s := range c.notHuman {
				if strings.Contains(hr.stdout, s) {
					t.Errorf("status has %q:\n%s", s, hr.stdout)
				}
			}
			if calls := h.jira.Calls(); len(calls) != 0 {
				t.Errorf("status called Jira: %v", calls)
			}
		})
	}
}

// A broken Keychain with no file is not configured, naming the Keychain:
// sync exits 3, stop keeps the worklog pending (exit 7 not_configured),
// start goes ahead unverified with a notice. It is never an auth failure.
func TestABrokenKeychainIsNotConfiguredNotAnAuthFailure(t *testing.T) {
	h := newHarness(t)
	h.configure("")
	h.kc.err = errLocked
	h.do("09:00", "start", "AA-1")

	h.now = at("09:30")
	r := h.run("stop", "--json")
	if r.code != 7 || !strings.Contains(r.stdout, `"code":"not_configured"`) || !strings.Contains(r.stdout, "macOS Keychain") {
		t.Errorf("stop: exit %d\n%s", r.code, r.stdout)
	}
	r = h.run("sync", "--json")
	if r.code != 3 || !strings.Contains(r.stdout, `"code":"not_configured"`) || !strings.Contains(r.stdout, "macOS Keychain") {
		t.Errorf("sync: exit %d\n%s", r.code, r.stdout)
	}
	r = h.run("sync")
	if r.code != 3 || !strings.Contains(r.stderr, "macOS Keychain") {
		t.Errorf("sync (human): exit %d\n%s", r.code, r.stderr)
	}
	r = h.run("start", "AA-2")
	if r.code != 0 || !strings.Contains(r.stderr, "macOS Keychain") {
		t.Errorf("start: exit %d, stderr %q", r.code, r.stderr)
	}
	if s := h.run("status", "--json"); !strings.Contains(s.stdout, `"authFailedAt":null`) {
		t.Errorf("a Keychain error recorded an auth failure:\n%s", s.stdout)
	}
}

// Only Jira-calling commands (and status, which reports the source) read
// the token; the presence path and the other reads never do.
func TestThePresencePathNeverReadsTheToken(t *testing.T) {
	h := newHarness(t)
	h.configure("")
	h.kc.token = "k"
	h.jira = knows("AA-1", "One", "AA-2", "Two") // never the network
	h.do("09:00", "add", "AA-1", "AA-2")
	h.do("09:00", "start", "AA-1")
	h.kc.reads = 0
	for _, args := range [][]string{
		{"touch", "--origin", "prompt"}, {"touch", "AA-2", "--origin", "prompt"}, {"touch", "AA-1", "--origin", "prompt"},
		{"status", "--short"}, {"ls"}, {"pause"}, {"resume"}, {"version"}, {"stop", "--no-sync"},
	} {
		h.run(args...)
	}
	if h.kc.reads != 0 {
		t.Errorf("the token was read %d times", h.kc.reads)
	}
}

// authFailedAt: a 401 sets it; an unreachable Jira or a broken Keychain
// leaves it; any successful authenticated call (a summary fetch, a sync)
// clears it.
func TestAuthFailedAtSetAndClear(t *testing.T) {
	failed := func(h *harness) bool {
		h.t.Helper()
		return !strings.Contains(h.run("status", "--json").stdout, `"authFailedAt":null`)
	}
	h := newHarness(t)
	h.jira = &tracker.Fake{Err: tracker.ErrUnauthorized}
	h.do("09:00", "start", "AA-1")
	if !failed(h) {
		t.Fatal("a 401 did not set authFailedAt")
	}
	h.jira = &tracker.Fake{Err: tracker.ErrUnreachable}
	h.do("09:05", "start", "AA-2")
	if !failed(h) {
		t.Error("an unreachable Jira cleared authFailedAt")
	}
	h.jira = knows("AA-3", "Three")
	h.do("09:10", "start", "AA-3")
	if failed(h) {
		t.Error("a successful summary fetch left authFailedAt set")
	}

	h.jira = &tracker.Fake{Err: tracker.ErrUnauthorized}
	h.do("09:15", "stop") // 401: pending, authFailedAt set
	if !failed(h) {
		t.Fatal("a 401 on stop did not set authFailedAt")
	}
	h.jira = &tracker.Fake{}
	if r := h.run("sync"); r.code != 0 {
		t.Fatalf("sync: exit %d %s", r.code, r.stderr)
	}
	if failed(h) {
		t.Error("a successful sync left authFailedAt set")
	}
}

// An unconfigured stop names the setup hint once, not twice.
func TestUnconfiguredStopNamesSetupOnce(t *testing.T) {
	for _, args := range [][]string{{"stop"}, {"stop", "--json"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			h := newHarness(t)
			h.env["XDG_CONFIG_HOME"] = t.TempDir() // no config.json
			h.do("09:00", "start", "AA-1")
			h.now = at("09:30")
			r := h.run(args...)
			if r.code != 7 {
				t.Fatalf("exit %d, want 7\n%s%s", r.code, r.stdout, r.stderr)
			}
			if n := strings.Count(r.stdout+r.stderr, "punchcard setup"); n != 1 {
				t.Errorf("the setup hint appears %d times:\n%s%s", n, r.stdout, r.stderr)
			}
		})
	}
}

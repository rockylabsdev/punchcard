package cli_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/j-dsmith/punchcard/internal/cli"
	"github.com/j-dsmith/punchcard/internal/config"
	"github.com/j-dsmith/punchcard/internal/store"
	"github.com/j-dsmith/punchcard/internal/tracker"
	"golang.org/x/sys/unix"
)

// harness runs punchcard in-process against an injected store, a fixed
// clock and predictable entry ids. Each run is one invocation.
type harness struct {
	t   *testing.T
	st  *store.Store
	dir string
	now time.Time
	env map[string]string
	ids int
	// tracker is the tracker every run has: Jira unless a test says
	// otherwise ("" resolves it from the config file, as the binary does).
	tracker config.Tracker
	// jira, when set, is the Jira every run talks to. Nil means the
	// default lookup, which finds no config here: unconfigured.
	jira *tracker.Fake
	// kc is the Keychain seam every run gets (never the real one): no
	// item unless a test says otherwise.
	kc *fakeKeychain
	// setups counts runs of the wizard (runTTY injects a fake that
	// exits setupCode).
	setups, setupCode int
}

// fakeKeychain answers the Keychain seam and counts the reads.
type fakeKeychain struct {
	token string
	err   error
	reads int
}

func (k *fakeKeychain) read(string) (string, error) {
	k.reads++
	if k.token == "" && k.err == nil {
		return "", config.ErrNoKeychainItem
	}
	return k.token, k.err
}

func (h *harness) jiraFn() func() (tracker.Client, error) {
	if h.jira == nil {
		return nil
	}
	return func() (tracker.Client, error) { return h.jira, nil }
}

func newHarness(t *testing.T) *harness {
	h := &harness{t: t, dir: t.TempDir(), now: at("09:00"), env: map[string]string{}, kc: &fakeKeychain{}, tracker: config.TrackerJira}
	h.st = store.New(h.dir, func() time.Time { return h.now })
	h.st.LockWait = 50 * time.Millisecond
	return h
}

// at is a wall-clock time on the test day, in UTC-5.
func at(hhmm string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", "2026-08-09 "+hhmm, zone)
	if err != nil {
		panic(err)
	}
	return t
}

func (h *harness) run(args ...string) result {
	h.t.Helper()
	var out, errb bytes.Buffer
	code := cli.Run(args, cli.Env{
		Stdin:       bytes.NewReader(nil),
		Stdout:      &out,
		Stderr:      &errb,
		Getenv:      func(k string) string { return h.env[k] },
		Now:         func() time.Time { return h.now },
		Version:     "1.2.3",
		Store:       h.st,
		Tracker:     h.tracker,
		OpenTracker: h.jiraFn(),
		Keychain:    h.kc.read,
		NewID: func() string {
			h.ids++
			return fmt.Sprintf("ID-%d", h.ids)
		},
	})
	return result{code, out.String(), errb.String()}
}

// do runs a setup step at a wall-clock time; it must exit 0 or 7.
func (h *harness) do(hhmm string, args ...string) {
	h.t.Helper()
	h.now = at(hhmm)
	if r := h.run(args...); r.code != 0 && r.code != 7 {
		h.t.Fatalf("setup %v: exit %d\n%s%s", args, r.code, r.stdout, r.stderr)
	}
}

// holdLock takes state.lock as another punchcard would, until the test ends.
func (h *harness) holdLock() {
	f, err := os.OpenFile(filepath.Join(h.dir, store.LockFile), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		h.t.Fatal(err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { f.Close() })
}

const (
	nine     = `"2026-08-09T09:00:00.000-0500"`
	idle     = `{"running": null, "paused": [], "unsent": {"count": 0, "seconds": 0}, "authFailedAt": null}`
	noEvents = `[]`
)

func ok(command, data, events string) string {
	return fmt.Sprintf(`{"v": 1, "ok": true, "outcome": "ok", "command": %q, "data": %s, "events": %s, "error": null}`,
		command, data, events)
}

func refused(command, outcome, code, hint, data string) string {
	h := ""
	if hint != "" {
		h = fmt.Sprintf(`, "hint": %q`, hint)
	}
	return fmt.Sprintf(`{"v": 1, "ok": false, "outcome": %q, "command": %q, "data": %s, "events": [], "error": {"code": %q, "message": "*"%s}}`,
		outcome, command, data, code, h)
}

func running(key, id string, byHand bool, origin string, elapsed int) string {
	return fmt.Sprintf(`{"key": %q, "unverified": true, "entryId": %q, "state": "running", "byHand": %t, "origin": %q, "recovered": false, "elapsedSeconds": %d}`,
		key, id, byHand, origin, elapsed)
}

func paused(key, id string, byHand bool, origin string, elapsed int) string {
	return fmt.Sprintf(`{"key": %q, "unverified": true, "entryId": %q, "state": "paused", "byHand": %t, "origin": %q, "recovered": false, "elapsedSeconds": %d}`,
		key, id, byHand, origin, elapsed)
}

func status(run string, paused ...string) string {
	return fmt.Sprintf(`{"running": %s, "paused": [%s], "unsent": {"count": 0, "seconds": 0}, "authFailedAt": null}`,
		run, strings.Join(paused, ","))
}

// unlinked is status data as status itself reports it in the harness:
// the snapshot plus the Jira link, unconfigured (no HOME).
func unlinked(data string) string {
	data = strings.TrimSpace(data)
	return strings.TrimSuffix(data, "}") + `, "tracker": "jira", "configured": false, "notConfigured": "not connected to Jira: neither XDG_CONFIG_HOME nor HOME is set"}`
}

func TestEnvelopes(t *testing.T) {
	cases := []struct {
		name  string
		setup func(h *harness)
		at    string
		args  []string
		code  int
		want  string
	}{
		// start
		{"start adds an untracked ticket and sets it by hand", nil, "09:00",
			[]string{"start", "AA-1"}, 0,
			ok("start", status(running("AA-1", "ID-1", true, "hand", 0)), `[{"kind": "added", "key": "AA-1"}]`)},
		{"start B pauses A", func(h *harness) { h.do("09:00", "start", "AA-1") }, "09:10",
			[]string{"start", "AA-2"}, 0,
			ok("start", status(running("AA-2", "ID-2", true, "hand", 0), paused("AA-1", "ID-1", true, "hand", 600)),
				`[{"kind": "paused", "key": "AA-1"}, {"kind": "added", "key": "AA-2"}]`)},
		{"start A by hand while an agent's A runs is a no-op that sets it by hand", func(h *harness) {
			h.env["CLAUDECODE"] = "1"
			h.do("09:00", "start", "AA-1")
			delete(h.env, "CLAUDECODE")
		}, "09:05",
			[]string{"start", "AA-1"}, 0,
			ok("start", status(running("AA-1", "ID-1", true, "hand", 300)), `[{"kind": "byHand", "key": "AA-1"}]`)},
		{"start A by an agent while A runs is a no-op that does not set it by hand", func(h *harness) {
			h.env["AI_AGENT"] = "x"
			h.do("09:00", "start", "AA-1")
		}, "09:05",
			[]string{"start", "AA-1"}, 0,
			ok("start", status(running("AA-1", "ID-1", false, "agent", 300)), noEvents)},
		{"an agent cannot move a timer set by hand", func(h *harness) {
			h.do("09:00", "start", "AA-1")
			h.env["CLAUDECODE"] = "1"
		}, "09:05",
			[]string{"start", "AA-2"}, 4,
			refused("start", "no_timer", "by_hand", "the user set it by hand; only they can move or pause it", "null")},
		{"--origin hand overrides the agent environment", func(h *harness) {
			h.do("09:00", "start", "AA-1")
			h.env["CLAUDECODE"] = "1"
		}, "09:05",
			[]string{"start", "AA-2", "--origin", "hand"}, 0,
			ok("start", status(running("AA-2", "ID-2", true, "hand", 0), paused("AA-1", "ID-1", true, "hand", 300)),
				`[{"kind": "paused", "key": "AA-1"}, {"kind": "added", "key": "AA-2"}]`)},
		{"--origin must be a known origin", nil, "09:00",
			[]string{"start", "AA-1", "--origin", "robot"}, 64,
			refused("start", "usage", "usage", "see punchcard --help", "null")},
		{"start without a key off a TTY is usage", nil, "09:00",
			[]string{"start"}, 64,
			refused("start", "usage", "usage", "see punchcard --help", "null")},
		{"start with a bad key is usage", nil, "09:00",
			[]string{"start", "nope"}, 64,
			refused("start", "usage", "usage", "", "null")},
		{"start waits for the lock, then busy", func(h *harness) { h.holdLock() }, "09:00",
			[]string{"start", "AA-1"}, 75,
			refused("start", "busy", "busy", "try again", "null")},

		// pause, resume
		{"pause", func(h *harness) { h.do("09:00", "start", "AA-1") }, "09:20",
			[]string{"pause"}, 0,
			ok("pause", status("null", paused("AA-1", "ID-1", true, "hand", 1200)), noEvents)},
		{"pause with nothing running", nil, "09:00",
			[]string{"pause"}, 4,
			refused("pause", "no_timer", "no_timer", "", "null")},
		{"resume the most recently paused", func(h *harness) {
			h.do("09:00", "start", "AA-1")
			h.do("09:20", "pause")
		}, "10:00",
			[]string{"resume"}, 0,
			ok("resume", status(running("AA-1", "ID-1", true, "hand", 1200)), noEvents)},
		{"resume with nothing paused", nil, "09:00",
			[]string{"resume"}, 4,
			refused("resume", "no_timer", "no_timer", "", "null")},
		{"resume KEY is start KEY", nil, "09:00",
			[]string{"resume", "AA-7"}, 0,
			ok("resume", status(running("AA-7", "ID-1", true, "hand", 0)), `[{"kind": "added", "key": "AA-7"}]`)},

		// stop
		{"stop unconfigured is a success with debt", func(h *harness) { h.do("09:00", "start", "AA-1") }, "09:30",
			[]string{"stop", "-m", "did it"}, 7,
			`{"v": 1, "ok": true, "outcome": "sync_pending", "command": "stop",
			  "data": {"key": "AA-1", "entryId": "ID-1", "worklog": {"state": "pending", "startedAt": ` + nine + `, "timeSpentSeconds": 1800, "comment": "did it"}},
			  "events": [], "error": {"code": "not_configured", "message": "*", "hint": "run ` + "`punchcard sync`" + ` later; never stop again"}}`},
		{"stop --no-sync leaves it pending and exits 0", func(h *harness) { h.do("09:00", "start", "AA-1") }, "09:30",
			[]string{"stop", "--no-sync", "--duration", "1h30m"}, 0,
			ok("stop", `{"key": "AA-1", "entryId": "ID-1", "worklog": {"state": "pending", "startedAt": `+nine+`, "timeSpentSeconds": 5400}}`, noEvents)},
		{"stop takes bare minutes", func(h *harness) { h.do("09:00", "start", "AA-1") }, "09:30",
			[]string{"stop", "AA-1", "--no-sync", "--duration", "45"}, 0,
			ok("stop", `{"key": "AA-1", "entryId": "ID-1", "worklog": {"state": "pending", "startedAt": `+nine+`, "timeSpentSeconds": 2700}}`, noEvents)},
		{"stop refuses h:mm", func(h *harness) { h.do("09:00", "start", "AA-1") }, "09:30",
			[]string{"stop", "--duration", "1:30"}, 64,
			refused("stop", "usage", "usage", "see punchcard --help", "null")},
		{"stop refuses over 24h", func(h *harness) { h.do("09:00", "start", "AA-1") }, "09:30",
			[]string{"stop", "--duration", "25h"}, 64,
			refused("stop", "usage", "usage", "see punchcard --help", "null")},
		{"stop under a minute", func(h *harness) { h.do("09:00", "start", "AA-1") }, "09:00",
			[]string{"stop"}, 64,
			refused("stop", "usage", "usage", "pass --duration", "null")},
		{"stop with several paused lists them", func(h *harness) {
			h.do("09:00", "start", "AA-1")
			h.do("09:10", "start", "AA-2")
			h.do("09:20", "pause")
		}, "09:30",
			[]string{"stop"}, 4,
			refused("stop", "no_timer", "no_timer", "name one: punchcard stop KEY", `{"candidates": ["AA-1", "AA-2"]}`)},
		{"stop with nothing open", nil, "09:00",
			[]string{"stop"}, 4,
			refused("stop", "no_timer", "no_timer", "", "null")},
		{"stop on a recovered entry needs a duration", func(h *harness) {
			h.do("09:00", "start", "AA-1")
			h.do("09:45", "touch")
		}, "13:00",
			[]string{"stop"}, 6,
			refused("stop", "needs_confirmation", "needs_confirmation", "rerun with --duration, e.g. --duration 1h30m",
				`{"preview": {"key": "AA-1", "entryId": "ID-1", "recovered": true,
				  "worklog": {"state": "pending", "startedAt": `+nine+`, "timeSpentSeconds": 2700}}}`)},
		{"stop on a recovered entry with a duration", func(h *harness) {
			h.do("09:00", "start", "AA-1")
			h.do("09:45", "touch")
		}, "13:00",
			[]string{"stop", "--duration", "2h", "--no-sync"}, 0,
			ok("stop", `{"key": "AA-1", "entryId": "ID-1", "worklog": {"state": "pending", "startedAt": `+nine+`, "timeSpentSeconds": 7200, "recovered": true}}`,
				`[{"kind": "recovered", "key": "AA-1", "at": "2026-08-09T09:45:00.000-0500"}]`)},

		// status
		{"status idle", nil, "09:00", []string{"status"}, 0, ok("status", unlinked(idle), noEvents)},
		{"status running and paused", func(h *harness) {
			h.do("09:00", "start", "AA-1")
			h.do("09:10", "start", "AA-2")
		}, "09:15",
			[]string{"status"}, 0,
			ok("status", unlinked(status(running("AA-2", "ID-2", true, "hand", 300), paused("AA-1", "ID-1", true, "hand", 600))), noEvents)},
		{"status on a stale timer reports it capped", func(h *harness) {
			h.do("09:00", "start", "AA-1")
			h.do("09:45", "touch")
		}, "13:00",
			[]string{"status"}, 0,
			ok("status", unlinked(`{"running": {"key": "AA-1", "unverified": true, "entryId": "ID-1", "state": "stale", "byHand": true, "origin": "hand",
				"recovered": false, "elapsedSeconds": 2700, "uncappedSeconds": 14400, "lastSeen": "2026-08-09T09:45:00.000-0500"},
				"paused": [], "unsent": {"count": 0, "seconds": 0}, "authFailedAt": null}`), noEvents)},
		{"status counts unsent worklogs", func(h *harness) {
			h.do("09:00", "start", "AA-1")
			h.do("09:30", "stop")
		}, "10:00",
			[]string{"status"}, 0,
			ok("status", unlinked(`{"running": null, "paused": [], "unsent": {"count": 1, "seconds": 1800}, "authFailedAt": null}`), noEvents)},
		{"status --short is the same envelope", nil, "09:00", []string{"status", "--short"}, 0, ok("status", idle, noEvents)},
		{"status refuses a newer schema", func(h *harness) {
			os.WriteFile(filepath.Join(h.dir, "state.json"), []byte(`{"schemaVersion": 9}`), 0o600)
		}, "09:00",
			[]string{"status"}, 65,
			refused("status", "state_unreadable", "state_unreadable", "upgrade punchcard", "null")},
		{"bare punchcard off a TTY is status", nil, "09:00", []string{}, 0, ok("status", unlinked(idle), noEvents)},

		// ls, add, rm
		{"ls is most recently active first", func(h *harness) {
			h.do("08:00", "add", "AA-3")
			h.do("09:00", "start", "AA-1")
			h.do("09:10", "start", "AA-2")
			h.do("09:40", "stop", "AA-2")
		}, "10:00",
			[]string{"ls"}, 0,
			ok("ls", `{"tickets": [
				{"key": "AA-2", "unverified": true, "addedAt": "2026-08-09T09:10:00.000-0500", "elapsedSeconds": 0, "todaySeconds": 1800, "unsynced": 1},
				{"key": "AA-1", "unverified": true, "addedAt": `+nine+`, "state": "paused", "elapsedSeconds": 600, "todaySeconds": 600, "unsynced": 0},
				{"key": "AA-3", "unverified": true, "addedAt": "2026-08-09T08:00:00.000-0500", "elapsedSeconds": 0, "todaySeconds": 0, "unsynced": 0}]}`, noEvents)},
		{"ls empty", nil, "09:00", []string{"ls"}, 0, ok("ls", `{"tickets": []}`, noEvents)},
		{"add", func(h *harness) { h.do("08:00", "add", "AA-1") }, "09:00",
			[]string{"add", "AA-1", "AA-2"}, 0,
			ok("add", `{"tickets": [
				{"key": "AA-2", "unverified": true, "addedAt": `+nine+`, "elapsedSeconds": 0, "todaySeconds": 0, "unsynced": 0},
				{"key": "AA-1", "unverified": true, "addedAt": "2026-08-09T08:00:00.000-0500", "elapsedSeconds": 0, "todaySeconds": 0, "unsynced": 0}]}`, noEvents)},
		{"add a bad key", nil, "09:00", []string{"add", "AA-1", "x"}, 64, refused("add", "usage", "usage", "", "null")},
		{"add needs a key", nil, "09:00", []string{"add"}, 64, refused("add", "usage", "usage", "see punchcard --help", "null")},
		{"rm", func(h *harness) { h.do("08:00", "add", "AA-1") }, "09:00",
			[]string{"rm", "AA-1"}, 0, ok("rm", `{"key": "AA-1"}`, noEvents)},
		{"rm refuses unlogged time with a hint", func(h *harness) { h.do("09:00", "start", "AA-1") }, "09:30",
			[]string{"rm", "AA-1"}, 4,
			refused("rm", "no_timer", "no_timer", "stop and sync it first, or pass --force to drop the unsent time", "null")},
		{"rm --force", func(h *harness) { h.do("09:00", "start", "AA-1") }, "09:30",
			[]string{"rm", "--force", "AA-1"}, 0, ok("rm", `{"key": "AA-1"}`, noEvents)},
		{"rm an untracked ticket", nil, "09:00", []string{"rm", "AA-1"}, 4, refused("rm", "no_timer", "no_timer", "", "null")},

		// sync, setup, touch
		{"sync with nothing owed", nil, "09:00", []string{"sync"}, 0, ok("sync", `{"sent": [], "remaining": []}`, noEvents)},
		{"sync unconfigured exits 3 and lists what is owed", func(h *harness) {
			h.do("09:00", "start", "AA-1")
			h.do("09:30", "stop")
		}, "10:00",
			[]string{"retry"}, 3,
			refused("retry", "not_configured", "not_configured", "run `punchcard setup`",
				`{"sent": [], "remaining": [{"key": "AA-1", "entryId": "ID-1", "worklog": {"state": "pending", "startedAt": `+nine+`, "timeSpentSeconds": 1800}}]}`)},
		{"setup needs a terminal", nil, "09:00", []string{"setup"}, 64, refused("setup", "usage", "usage", "see punchcard --help", "null")},
		{"touch from a prompt moves a timer not set by hand", func(h *harness) {
			h.env["CLAUDECODE"] = "1"
			h.do("09:00", "start", "AA-1")
		}, "09:10",
			[]string{"touch", "AA-2", "--origin", "prompt"}, 0,
			ok("touch", status(running("AA-2", "ID-2", false, "prompt", 0), paused("AA-1", "ID-1", false, "prompt", 600)),
				`[{"kind": "paused", "key": "AA-1"}, {"kind": "added", "key": "AA-2"}, {"kind": "started", "key": "AA-2"}]`)},
		{"touch from a prompt moves a timer set by hand", func(h *harness) { h.do("09:00", "start", "AA-1") }, "09:10",
			[]string{"touch", "AA-2", "--origin", "prompt"}, 0,
			ok("touch", status(running("AA-2", "ID-2", false, "prompt", 0), paused("AA-1", "ID-1", true, "prompt", 600)),
				`[{"kind": "paused", "key": "AA-1"}, {"kind": "added", "key": "AA-2"}, {"kind": "started", "key": "AA-2"}]`)},
		{"touch skips when the lock is busy", func(h *harness) { h.holdLock() }, "09:00",
			[]string{"touch"}, 0, ok("touch", "null", noEvents)},

		// usage
		{"unknown command", nil, "09:00", []string{"frobnicate"}, 64, refused("", "usage", "usage", "see punchcard --help", "null")},
		{"unknown flag", nil, "09:00", []string{"pause", "--bogus"}, 64, refused("pause", "usage", "usage", "see punchcard --help", "null")},
		{"extra arguments", nil, "09:00", []string{"pause", "AA-1"}, 64, refused("pause", "usage", "usage", "see punchcard --help", "null")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			if c.setup != nil {
				c.setup(h)
			}
			h.now = at(c.at)
			r := h.run(append(c.args, "--json")...)
			if r.code != c.code {
				t.Errorf("exit %d, want %d\nstderr %q", r.code, c.code, r.stderr)
			}
			if r.stderr != "" {
				t.Errorf("--json wrote to stderr: %q", r.stderr)
			}
			got := decode(t, r.stdout)
			want := decode(t, c.want)
			if d := cmp.Diff(want, got, anyMessage); d != "" {
				t.Errorf("envelope (-want +got):\n%s", d)
			}
		})
	}
}

// anyMessage: error messages are prose; a "*" in the want matches any
// non-empty message.
var anyMessage = cmp.FilterPath(func(p cmp.Path) bool {
	return p.Last().String() == `["message"]`
}, cmp.Comparer(func(a, b any) bool {
	sa, _ := a.(string)
	sb, _ := b.(string)
	if sa == "*" {
		return sb != ""
	}
	if sb == "*" {
		return sa != ""
	}
	return sa == sb
}))

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

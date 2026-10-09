package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/j-dsmith/punchcard/internal/cli"
)

var zone = time.FixedZone("UTC-5", -5*60*60)

// machine is one user's data dir and clock; each call to run is a
// separate punchcard invocation, as from another terminal.
type machine struct {
	t    *testing.T
	env  map[string]string
	now  time.Time
	data string
}

func newMachine(t *testing.T) *machine {
	data := t.TempDir()
	return &machine{
		t:    t,
		env:  map[string]string{"XDG_DATA_HOME": data, "HOME": t.TempDir()},
		now:  time.Date(2026, 8, 9, 9, 0, 0, 0, zone),
		data: filepath.Join(data, "punchcard"),
	}
}

func (m *machine) run(args ...string) result {
	m.t.Helper()
	var out, errb bytes.Buffer
	code := cli.Run(args, cli.Env{
		Stdout:  &out,
		Stderr:  &errb,
		Getenv:  func(k string) string { return m.env[k] },
		Now:     func() time.Time { return m.now },
		Version: "1.2.3",
	})
	return result{code, out.String(), errb.String()}
}

// ok runs a command that must exit 0 and returns its stdout.
func (m *machine) ok(args ...string) string {
	m.t.Helper()
	r := m.run(args...)
	if r.code != 0 {
		m.t.Fatalf("%v: exit %d\nstdout %q\nstderr %q", args, r.code, r.stdout, r.stderr)
	}
	return r.stdout
}

func (m *machine) file(name string) string {
	m.t.Helper()
	b, err := os.ReadFile(filepath.Join(m.data, name))
	if err != nil {
		m.t.Fatal(err)
	}
	return string(b)
}

func wantLines(t *testing.T, got string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("output lacks %q:\n%s", w, got)
		}
	}
}

func TestStartInOneTerminalShowsInAnother(t *testing.T) {
	m := newMachine(t)
	wantLines(t, m.ok("start", "AA-1"), "AA-1", "added")
	m.now = m.now.Add(12*time.Minute + 3*time.Second)
	wantLines(t, m.ok("status"), "▶ AA-1", "running", "0:12:03")
	if s := m.file("state.json"); !strings.Contains(s, `"unverified": true`) {
		t.Errorf("a key added without Jira is unverified:\n%s", s)
	}
}

func TestStartAnotherTicketPausesTheFirst(t *testing.T) {
	m := newMachine(t)
	m.ok("start", "AA-1")
	m.now = m.now.Add(20 * time.Minute)
	wantLines(t, m.ok("start", "AA-2"), "AA-1 paused", "▶ AA-2")
	m.now = m.now.Add(5 * time.Minute)
	wantLines(t, m.ok("status"), "▶ AA-2", "0:05:00", "⏸ AA-1", "0:20:00")
}

func TestPauseAndResume(t *testing.T) {
	m := newMachine(t)
	m.ok("start", "AA-1")
	m.now = m.now.Add(10 * time.Minute)
	wantLines(t, m.ok("pause"), "⏸ AA-1", "0:10:00")

	r := m.run("pause")
	if r.code != 4 || r.stderr == "" {
		t.Errorf("pause with nothing running: exit %d stderr %q, want 4", r.code, r.stderr)
	}

	m.now = m.now.Add(time.Hour)
	wantLines(t, m.ok("resume"), "▶ AA-1", "0:10:00")
	m.now = m.now.Add(time.Minute)
	wantLines(t, m.ok("status"), "▶ AA-1", "0:11:00")
}

func TestResumeKeyIsStartKey(t *testing.T) {
	m := newMachine(t)
	wantLines(t, m.ok("resume", "AA-7"), "▶ AA-7")
}

func TestStatusIdle(t *testing.T) {
	m := newMachine(t)
	wantLines(t, m.ok("status"), "Nothing running")
	if _, err := os.Stat(m.data); !os.IsNotExist(err) {
		t.Error("status created the data dir; reads never write")
	}
}

func TestStatusOnAStaleTimerReportsItAndWritesNothing(t *testing.T) {
	m := newMachine(t)
	m.ok("start", "AA-1") // 09:00
	heartbeat := `{"schemaVersion": 1, "lastSeen": "2026-08-09T09:45:00.000-0500"}` + "\n"
	if err := os.WriteFile(filepath.Join(m.data, "heartbeat.json"), []byte(heartbeat), 0o600); err != nil {
		t.Fatal(err)
	}
	state := m.file("state.json")

	m.now = time.Date(2026, 8, 9, 13, 0, 0, 0, zone)
	wantLines(t, m.ok("status"),
		"AA-1", "stale", "0:45:00", "4:00:00", "09:45", "3h15m ago", "next action will recover it")
	if m.file("state.json") != state || m.file("heartbeat.json") != heartbeat {
		t.Error("status wrote to disk")
	}

	wantLines(t, m.ok("pause"), "AA-1 recovered", "09:45")
	wantLines(t, m.ok("status"), "⏸ AA-1", "0:45:00", "recovered")
}

func TestRefusals(t *testing.T) {
	cases := []struct {
		name  string
		setup func(m *machine)
		args  []string
		code  int
	}{
		{"bad key", nil, []string{"start", "nope"}, 64},
		{"start without a key", nil, []string{"start"}, 64},
		{"extra arguments", nil, []string{"pause", "AA-1"}, 64},
		{"resume with nothing paused", nil, []string{"resume"}, 4},
		{"agent cannot pause a held timer", func(m *machine) {
			m.ok("start", "AA-1")
			m.env["CLAUDECODE"] = "1"
		}, []string{"pause"}, 4},
		{"newer schema", func(m *machine) {
			os.MkdirAll(m.data, 0o700)
			os.WriteFile(filepath.Join(m.data, "state.json"), []byte(`{"schemaVersion": 9}`), 0o600)
		}, []string{"start", "AA-1"}, 65},
		{"newer schema on status", func(m *machine) {
			os.MkdirAll(m.data, 0o700)
			os.WriteFile(filepath.Join(m.data, "state.json"), []byte(`{"schemaVersion": 9}`), 0o600)
		}, []string{"status"}, 65},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newMachine(t)
			if c.setup != nil {
				c.setup(m)
			}
			r := m.run(c.args...)
			if r.code != c.code {
				t.Errorf("exit %d, want %d (stderr %q)", r.code, c.code, r.stderr)
			}
			if r.stderr == "" {
				t.Error("no message on stderr")
			}
		})
	}
}

func TestACorruptStateFileIsReportedOnStderr(t *testing.T) {
	m := newMachine(t)
	os.MkdirAll(m.data, 0o700)
	os.WriteFile(filepath.Join(m.data, "state.json"), []byte("{garbage"), 0o600)
	r := m.run("start", "AA-1")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	wantLines(t, r.stderr, filepath.Join(m.data, "state.json.corrupt-"))
}

package cli_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/j-dsmith/punchcard/internal/cli"
	"github.com/j-dsmith/punchcard/internal/config"
	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/jira"
	"github.com/j-dsmith/punchcard/internal/store"
	"golang.org/x/sys/unix"
)

// fakeClaude is `claude` on PATH for doctor: its version and its
// plugin list. Run is never called.
type fakeClaude struct {
	path, list string
	// budgets is how long each command was given, by its args.
	budgets map[string]time.Duration
}

func (c *fakeClaude) Path() string { return c.path }

func (c *fakeClaude) Run(context.Context, io.Writer, ...string) error {
	panic("doctor never runs a claude command that changes anything")
}

func (c *fakeClaude) Output(ctx context.Context, args ...string) ([]byte, error) {
	if d, ok := ctx.Deadline(); ok {
		if c.budgets == nil {
			c.budgets = map[string]time.Duration{}
		}
		c.budgets[strings.Join(args, " ")] = time.Until(d)
	}
	switch strings.Join(args, " ") {
	case "--version":
		return []byte("2.1.291 (Claude Code)\n"), nil
	case "plugin list --json":
		return []byte(c.list), nil
	}
	return nil, fmt.Errorf("unexpected claude %v", args)
}

// doctorRig is a machine for doctor: a temp HOME holding the XDG and
// Claude Code defaults, a clock, the fakes. Nothing reaches the network.
type doctorRig struct {
	t      *testing.T
	home   string
	env    map[string]string
	now    time.Time
	st     *store.Store
	kc     *fakeKeychain
	claude *fakeClaude
}

const (
	doctorSite  = "https://acme.atlassian.net"
	doctorEmail = "me@acme.test"
)

// newDoctorRig is the happy machine: Jira with its token in the
// Keychain, PUN-142 running by hand since 08:00, the plugin at this
// build's version and the ask rule in place.
func newDoctorRig(t *testing.T) *doctorRig {
	home := t.TempDir()
	r := &doctorRig{
		t: t, home: home, now: at("08:00"),
		env: map[string]string{
			"HOME": home, "TERM": "xterm-ghostty", "TERM_PROGRAM": "ghostty", "COLORTERM": "truecolor",
		},
		kc:     &fakeKeychain{token: "tok"},
		claude: &fakeClaude{path: "/opt/homebrew/bin/claude", list: `[{"id":"punchcard@punchcard","version":"1.2.3"}]`},
	}
	r.st = store.Open(r.getenv, func() time.Time { return r.now })
	if err := config.Save(r.getenv, config.Config{Tracker: config.TrackerJira, SiteURL: doctorSite, Email: doctorEmail}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.st.Apply(core.Start{Key: "PUN-142", Origin: core.OriginHand, NewEntryID: "E1", Grammar: jira.Grammar}); err != nil {
		t.Fatal(err)
	}
	r.settings(`{"permissions":{"ask":["Bash(punchcard stop *--yes*)"]}}`)
	r.now = at("09:12")
	return r
}

func (r *doctorRig) getenv(k string) string { return r.env[k] }

func (r *doctorRig) settings(body string) {
	r.t.Helper()
	r.write(filepath.Join(r.home, ".claude", "settings.json"), body)
}

func (r *doctorRig) write(path, body string) {
	r.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		r.t.Fatal(err)
	}
}

func (r *doctorRig) run(args ...string) result {
	r.t.Helper()
	var out, errb bytes.Buffer
	env := cli.Env{
		Stdin: bytes.NewReader(nil), Stdout: &out, Stderr: &errb,
		Getenv: r.getenv, Now: func() time.Time { return r.now }, Version: "1.2.3",
		Store: r.st, Keychain: r.kc.read,
		Machine: cli.Machine{
			Executable: func() (string, error) { return filepath.Join(r.home, "bin", "punchcard"), nil },
			OSVersion:  func() string { return "15.1" },
			TermSize:   func() (int, int, bool) { return 120, 32, true },
		},
	}
	if r.claude != nil {
		env.Claude = r.claude
	}
	code := cli.Run(append([]string{"doctor"}, args...), env)
	return result{code, out.String(), errb.String()}
}

// line is the human output's line for label.
func line(t *testing.T, out, label string) string {
	t.Helper()
	for l := range strings.Lines(out) {
		if strings.HasPrefix(l, fmt.Sprintf("%-12s", label)) {
			return strings.TrimSpace(strings.TrimPrefix(l, fmt.Sprintf("%-12s", label)))
		}
	}
	t.Fatalf("no %q line in:\n%s", label, out)
	return ""
}

func TestDoctorPrintsOnePasteableReport(t *testing.T) {
	r := newDoctorRig(t)
	got := r.run()
	want := fmt.Sprintf(`punchcard 1.2.3 (contract v1, %s %s/%s)
binary      ~/bin/punchcard
data        ~/.local/share/punchcard  state.json ok (schema 1, 1 ticket)  heartbeat 08:00
config      ~/.config/punchcard/config.json  tracker jira  site <redacted>  email <redacted>
token       Keychain
timer       ▶ PUN-142 1:12 by hand
unsent      0
claude      2.1.291 at /opt/homebrew/bin/claude
plugin      punchcard@punchcard 1.2.3  ✓ matches binary
ask rule    present in ~/.claude/settings.json
terminal    TERM=xterm-ghostty  TERM_PROGRAM=ghostty  COLORTERM=truecolor  NO_COLOR unset  120x32
os          %s 15.1 %s
`, runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.GOOS, runtime.GOARCH)
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	if d := cmp.Diff(want, got.stdout); d != "" {
		t.Errorf("doctor (-want +got):\n%s", d)
	}
}

func TestDoctorClaudeAndPlugin(t *testing.T) {
	cases := []struct {
		name               string
		claude             *fakeClaude
		wantClaude, wantPl string
	}{
		{"no claude", &fakeClaude{}, "not found on PATH", "claude not found"},
		{"plugin at this version", nil, "2.1.291 at /opt/homebrew/bin/claude", "punchcard@punchcard 1.2.3  ✓ matches binary"},
		{"plugin at another release",
			&fakeClaude{path: "/opt/homebrew/bin/claude", list: `[{"id":"punchcard@punchcard","version":"1.2.0"}]`},
			"2.1.291 at /opt/homebrew/bin/claude", "punchcard@punchcard 1.2.0  ≠ 1.2.3 → run punchcard setup"},
		{"plugin not installed", &fakeClaude{path: "/opt/homebrew/bin/claude", list: `[]`},
			"2.1.291 at /opt/homebrew/bin/claude", "not installed → run punchcard setup"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newDoctorRig(t)
			if c.claude != nil {
				r.claude = c.claude
			}
			got := r.run()
			if got.code != 0 {
				t.Fatalf("exit %d", got.code)
			}
			if l := line(t, got.stdout, "claude"); l != c.wantClaude {
				t.Errorf("claude line %q, want %q", l, c.wantClaude)
			}
			if l := line(t, got.stdout, "plugin"); l != c.wantPl {
				t.Errorf("plugin line %q, want %q", l, c.wantPl)
			}
		})
	}
}

func TestDoctorBoundsEveryClaudeCommandByItsBudget(t *testing.T) {
	r := newDoctorRig(t)
	if got := r.run(); got.code != 0 {
		t.Fatalf("exit %d", got.code)
	}
	for _, args := range []string{"--version", "plugin list --json"} {
		d, ok := r.claude.budgets[args]
		if !ok || d > 10*time.Second {
			t.Errorf("claude %s ran with budget %v (bounded %v), want at most 10s", args, d, ok)
		}
	}
}

func TestDoctorAskRule(t *testing.T) {
	for name, c := range map[string]struct{ settings, want string }{
		"absent":      {`{"model":"opus"}`, "absent → punchcard setup adds it"},
		"no file":     {"", "absent → punchcard setup adds it"},
		"unparseable": {`{"permissions": {`, "~/.claude/settings.json unparseable"},
	} {
		t.Run(name, func(t *testing.T) {
			r := newDoctorRig(t)
			p := filepath.Join(r.home, ".claude", "settings.json")
			if c.settings == "" {
				os.Remove(p)
			} else {
				r.settings(c.settings)
			}
			if l := line(t, r.run().stdout, "ask rule"); l != c.want {
				t.Errorf("ask rule line %q, want %q", l, c.want)
			}
		})
	}
}

func TestDoctorTrackerAndToken(t *testing.T) {
	t.Run("no tracker", func(t *testing.T) {
		r := newDoctorRig(t)
		os.Remove(filepath.Join(r.home, ".config", "punchcard", "config.json"))
		out := r.run().stdout
		if l := line(t, out, "config"); l != "~/.config/punchcard/config.json  none yet  tracker none" {
			t.Errorf("config line %q", l)
		}
		if l := line(t, out, "token"); l != "none (no tracker)" {
			t.Errorf("token line %q", l)
		}
		if r.kc.reads != 0 {
			t.Errorf("Keychain read %d times with no tracker", r.kc.reads)
		}
	})
	sources := []struct {
		name    string
		prepare func(r *doctorRig)
		want    string
	}{
		{"Keychain", func(*doctorRig) {}, "Keychain"},
		{"PUNCHCARD_TOKEN", func(r *doctorRig) { r.env["PUNCHCARD_TOKEN"] = "tok-env" }, "PUNCHCARD_TOKEN"},
		{"file", func(r *doctorRig) {
			r.kc.token = ""
			if err := config.WriteTokenFile(r.getenv, "tok-file"); err != nil {
				t.Fatal(err)
			}
		}, "file"},
		{"none", func(r *doctorRig) { r.kc.token = "" }, "none"},
	}
	for _, s := range sources {
		t.Run("Jira, token from "+s.name, func(t *testing.T) {
			r := newDoctorRig(t)
			s.prepare(r)
			got := r.run()
			if l := line(t, got.stdout, "token"); l != s.want {
				t.Errorf("token line %q, want %q", l, s.want)
			}
			for _, secret := range []string{"tok-env", "tok-file"} {
				if strings.Contains(got.stdout, secret) {
					t.Errorf("output carries the token %q", secret)
				}
			}
		})
	}
}

func TestDoctorRedacts(t *testing.T) {
	r := newDoctorRig(t)
	got := r.run()
	for _, secret := range []string{doctorSite, "acme", doctorEmail, r.home} {
		if strings.Contains(got.stdout, secret) {
			t.Errorf("redacted output carries %q:\n%s", secret, got.stdout)
		}
	}
	plain := r.run("--no-redact").stdout
	if l := line(t, plain, "config"); l != filepath.Join(r.home, ".config/punchcard/config.json")+"  tracker jira  site "+doctorSite+"  email "+doctorEmail {
		t.Errorf("--no-redact config line %q", l)
	}
	if l := line(t, plain, "binary"); l != filepath.Join(r.home, "bin/punchcard") {
		t.Errorf("--no-redact binary line %q", l)
	}
}

func TestDoctorJSON(t *testing.T) {
	r := newDoctorRig(t)
	got := r.run("--json")
	if got.code != 0 {
		t.Fatalf("exit %d", got.code)
	}
	env := decode(t, got.stdout)
	if env["ok"] != true || env["command"] != "doctor" {
		t.Fatalf("envelope %v", env)
	}
	d := env["data"].(map[string]any)
	want := map[string]any{
		"binary": "~/bin/punchcard",
		"data": map[string]any{"dir": "~/.local/share/punchcard", "state": "ok", "schemaVersion": 1.0, "tickets": 1.0,
			"heartbeat": "2026-08-09T08:00:00.000-0500", "locked": false},
		"config": map[string]any{"path": "~/.config/punchcard/config.json", "exists": true, "tracker": "jira",
			"site": "<redacted>", "email": "<redacted>"},
		"token":   "keychain",
		"unsent":  map[string]any{"count": 0.0, "seconds": 0.0},
		"claude":  map[string]any{"found": true, "version": "2.1.291", "path": "/opt/homebrew/bin/claude"},
		"plugin":  map[string]any{"installed": true, "version": "1.2.3", "matches": true, "skewed": false},
		"askRule": map[string]any{"path": "~/.claude/settings.json", "state": "present"},
		"terminal": map[string]any{"TERM": "xterm-ghostty", "TERM_PROGRAM": "ghostty", "COLORTERM": "truecolor",
			"NO_COLOR": false, "width": 120.0, "height": 32.0},
		"os": map[string]any{"goos": runtime.GOOS, "version": "15.1", "arch": runtime.GOARCH},
	}
	for k, w := range want {
		if diff := cmp.Diff(w, d[k]); diff != "" {
			t.Errorf("data.%s (-want +got):\n%s", k, diff)
		}
	}
	if tm, _ := d["timer"].(map[string]any); tm["key"] != "PUN-142" || tm["byHand"] != true {
		t.Errorf("data.timer = %v", d["timer"])
	}
	if pc, _ := d["punchcard"].(map[string]any); pc["version"] != "1.2.3" || pc["contract"] != 1.0 {
		t.Errorf("data.punchcard = %v", d["punchcard"])
	}
}

// doctor never fixes state.json: an unreadable file stays where it is,
// and doctor still exits 0.
func TestDoctorReportsStateProblemsWithoutFixingThem(t *testing.T) {
	statePath := func(r *doctorRig) string { return filepath.Join(r.home, ".local/share/punchcard/state.json") }
	t.Run("unreadable", func(t *testing.T) {
		r := newDoctorRig(t)
		r.write(statePath(r), "{not json")
		got := r.run()
		if got.code != 0 {
			t.Fatalf("exit %d", got.code)
		}
		if l := line(t, got.stdout, "data"); !strings.Contains(l, "state.json unreadable: ") ||
			!strings.Contains(l, "the next change will move it aside") || strings.Contains(l, r.home) {
			t.Errorf("data line %q", l)
		}
		if b, _ := os.ReadFile(statePath(r)); string(b) != "{not json" {
			t.Errorf("state.json changed: %q", b)
		}
		if got := r.run("--json"); got.code != 0 || decode(t, got.stdout)["ok"] != true {
			t.Errorf("--json: exit %d %s", got.code, got.stdout)
		}
	})
	t.Run("newer", func(t *testing.T) {
		r := newDoctorRig(t)
		r.write(statePath(r), `{"schemaVersion": 99}`)
		got := r.run()
		if l := line(t, got.stdout, "data"); got.code != 0 || !strings.Contains(l, "state.json written by a newer punchcard") {
			t.Errorf("exit %d data line %q", got.code, l)
		}
	})
	t.Run("locked", func(t *testing.T) {
		r := newDoctorRig(t)
		f, err := os.OpenFile(filepath.Join(r.home, ".local/share/punchcard", store.LockFile), os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
			t.Fatal(err)
		}
		if l := line(t, r.run().stdout, "data"); !strings.Contains(l, "state is locked by another punchcard") {
			t.Errorf("data line %q", l)
		}
	})
	t.Run("a heartbeat from another day shows its date", func(t *testing.T) {
		r := newDoctorRig(t)
		r.now = at("09:12").Add(24 * time.Hour)
		if l := line(t, r.run().stdout, "data"); !strings.HasSuffix(l, "heartbeat 2026-08-09 08:00") {
			t.Errorf("data line %q", l)
		}
	})
	t.Run("no data directory", func(t *testing.T) {
		r := newDoctorRig(t)
		r.env = map[string]string{}
		r.st = store.Open(r.getenv, func() time.Time { return r.now })
		got := r.run()
		if l := line(t, got.stdout, "data"); got.code != 0 || !strings.HasPrefix(l, "no data directory") {
			t.Errorf("exit %d data line %q", got.code, l)
		}
	})
}

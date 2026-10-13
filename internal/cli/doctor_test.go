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
)

// fakeClaude is `claude` on PATH for doctor: its version and its
// plugin list. Run is never called.
type fakeClaude struct {
	path, list string
}

func (c *fakeClaude) Path() string { return c.path }

func (c *fakeClaude) Run(context.Context, io.Writer, ...string) error {
	panic("doctor never runs a claude command that changes anything")
}

func (c *fakeClaude) Output(_ context.Context, args ...string) ([]byte, error) {
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

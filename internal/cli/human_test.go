package cli_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/j-dsmith/punchcard/internal/cli"
)

// runTTY is h.run on a terminal, with stdin and optional hooks.
func (h *harness) runTTY(stdin string, tweak func(*cli.Env), args ...string) result {
	h.t.Helper()
	var out, errb bytes.Buffer
	env := cli.Env{
		Stdin: strings.NewReader(stdin), Stdout: &out, Stderr: &errb,
		Getenv: func(k string) string { return h.env[k] }, Now: func() time.Time { return h.now },
		Version: "1.2.3", Store: h.st, TTY: true, Jira: h.jiraFn(),
	}
	if tweak != nil {
		tweak(&env)
	}
	code := cli.Run(args, env)
	return result{code, out.String(), errb.String()}
}

func TestStatusShort(t *testing.T) {
	cases := []struct {
		name  string
		setup func(h *harness)
		at    string
		want  string
	}{
		{"idle prints nothing", nil, "09:00", ""},
		{"running", func(h *harness) { h.do("09:00", "start", "AA-1") }, "10:12", "▶ AA-1 1:12\n"},
		{"paused", func(h *harness) {
			h.do("09:00", "start", "AA-1")
			h.do("09:40", "pause")
		}, "10:00", "⏸ AA-1 0:40\n"},
		{"stale", func(h *harness) { h.do("09:00", "start", "AA-1") }, "13:00", "! AA-1 stale\n"},
		{"running with unsent", func(h *harness) {
			h.do("08:00", "start", "AA-2")
			h.do("08:30", "stop")
			h.do("09:00", "start", "AA-1")
		}, "10:12", "▶ AA-1 1:12 · 1 unsent\n"},
		{"idle with unsent shows only the count", func(h *harness) {
			h.do("08:00", "start", "AA-2")
			h.do("08:30", "stop")
		}, "09:00", "1 unsent\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			if c.setup != nil {
				c.setup(h)
			}
			h.now = at(c.at)
			r := h.run("status", "--short")
			if r.code != 0 || r.stdout != c.want || r.stderr != "" {
				t.Errorf("got exit %d stdout %q stderr %q, want 0 %q", r.code, r.stdout, r.stderr, c.want)
			}
		})
	}
}

func TestStatusShortAlwaysExitsZero(t *testing.T) {
	h := newHarness(t)
	writeFile(t, h.dir, "state.json", `{"schemaVersion": 9}`)
	r := h.run("status", "--short")
	if r.code != 0 || r.stdout != "" || r.stderr != "" {
		t.Errorf("exit %d stdout %q stderr %q, want a silent 0", r.code, r.stdout, r.stderr)
	}
}

func TestEveryHumanChangeWarnsWhileWorklogsAreUnsent(t *testing.T) {
	h := newHarness(t)
	h.do("08:00", "start", "AA-2")
	h.now = at("08:30")
	r := h.run("stop")
	if r.code != 7 || !strings.Contains(r.stderr, "not sent") || !strings.Contains(r.stderr, "punchcard sync") {
		t.Errorf("stop: exit %d stderr %q, want 7 and a pending note", r.code, r.stderr)
	}
	for _, args := range [][]string{{"start", "AA-1"}, {"pause"}, {"resume"}, {"add", "AA-3"}, {"rm", "AA-3"}} {
		r := h.run(args...)
		if r.code != 0 || !strings.Contains(r.stderr, "1 unsent worklog(s), 0:30:00; run `punchcard sync`") {
			t.Errorf("%v: exit %d stderr %q, want the unsent warning", args, r.code, r.stderr)
		}
	}
	if r := h.run("status"); r.stderr != "" {
		t.Errorf("a read is not a change; stderr %q", r.stderr)
	}
	if r := h.run("pause", "--json"); r.code != 0 || r.stderr != "" {
		t.Errorf("--json keeps stderr clean: %q", r.stderr)
	}
}

func TestRefusalsGoToStderrWithAHint(t *testing.T) {
	h := newHarness(t)
	h.do("09:00", "start", "AA-1")
	r := h.run("rm", "AA-1")
	if r.code != 4 || r.stdout != "" {
		t.Fatalf("exit %d stdout %q", r.code, r.stdout)
	}
	wantLines(t, r.stderr, "punchcard: ", "AA-1", "--force")

	h.do("09:10", "start", "AA-2")
	h.do("09:20", "pause")
	r = h.run("stop")
	if r.code != 4 {
		t.Fatalf("exit %d", r.code)
	}
	wantLines(t, r.stderr, "AA-1, AA-2", "punchcard stop KEY")
}

func TestColourOnlyOnATTYWithoutNoColor(t *testing.T) {
	h := newHarness(t)
	h.do("09:00", "start", "AA-1")
	if r := h.run("status"); strings.Contains(r.stdout, "\x1b[") {
		t.Errorf("colour off a TTY: %q", r.stdout)
	}
	if r := h.runTTY("", nil, "status"); !strings.Contains(r.stdout, "\x1b[32m▶\x1b[0m") {
		t.Errorf("no colour on a TTY: %q", r.stdout)
	}
	h.env["NO_COLOR"] = "1"
	if r := h.runTTY("", nil, "status"); strings.Contains(r.stdout, "\x1b[") {
		t.Errorf("colour with NO_COLOR set: %q", r.stdout)
	}
}

func TestStartWithNoKeyPicksFromRecentsOnATTY(t *testing.T) {
	h := newHarness(t)
	h.env["NO_COLOR"] = "1"
	h.do("08:00", "add", "AA-3")
	h.do("09:00", "start", "AA-1")
	h.do("09:10", "pause")
	h.now = at("09:30")

	var offered []string
	r := h.runTTY("", func(e *cli.Env) {
		e.Pick = func(cs []cli.Choice) (string, error) {
			for _, c := range cs {
				offered = append(offered, c.Key)
			}
			return "AA-3", nil
		}
	}, "start")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	if strings.Join(offered, " ") != "AA-1 AA-3" {
		t.Errorf("offered %v, want the ls order", offered)
	}
	wantLines(t, r.stdout, "▶ AA-3 running")

	r = h.runTTY("2\n", nil, "start") // the default line picker: AA-3 is now first
	if r.code != 0 {
		t.Fatalf("line picker: exit %d: %s", r.code, r.stderr)
	}
	wantLines(t, r.stderr, "1) AA-3", "2) AA-1")
	wantLines(t, r.stdout, "▶ AA-1 running")

	if r := h.runTTY("\n", nil, "start"); r.code != 0 || !strings.Contains(r.stderr, "Nothing started") {
		t.Errorf("cancel: exit %d stderr %q", r.code, r.stderr)
	}
	if r := h.runTTY("", nil, "start", "--json"); r.code != 64 {
		t.Errorf("--json never prompts: exit %d", r.code)
	}
}

func TestBarePunchcardOnATTYHandsOffToTheTUI(t *testing.T) {
	h := newHarness(t)
	called := false
	r := h.runTTY("", func(e *cli.Env) {
		e.TUI = func(cli.Env) int { called = true; return 0 }
	})
	if !called || r.code != 0 {
		t.Errorf("TUI not run (exit %d)", r.code)
	}
	if r := h.runTTY("", nil); !strings.Contains(r.stdout, "Nothing running") {
		t.Errorf("no TUI wired: want status, got %q", r.stdout)
	}
}

func TestSetupOnATTYIsNotBuiltYet(t *testing.T) {
	h := newHarness(t)
	if r := h.runTTY("", nil, "setup"); r.code != 1 || !strings.Contains(r.stderr, "not built yet") {
		t.Errorf("exit %d stderr %q", r.code, r.stderr)
	}
}

func TestTouchTakesTheKeyFromTheGitBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "feature/AA-9-thing"}, {"commit", "-q", "--allow-empty", "-m", "x"}} {
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		cmd.Env = []string{"HOME=" + t.TempDir(), "GIT_CONFIG_NOSYSTEM=1", "PATH=/usr/bin:/bin:/usr/local/bin:/opt/homebrew/bin"}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	h := newHarness(t)
	h.env["CLAUDECODE"] = "1"
	h.do("09:00", "start", "AA-1")
	h.now = at("09:10")
	r := h.run("touch", "--cwd", repo, "--origin", "prompt")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	wantLines(t, r.stdout, "▶ AA-9 started")

	h.now = at("09:20")
	if r := h.run("touch", "--cwd", t.TempDir(), "--origin", "prompt"); r.code != 0 || r.stdout != "" {
		t.Errorf("not a repo is presence only: exit %d stdout %q", r.code, r.stdout)
	}
}

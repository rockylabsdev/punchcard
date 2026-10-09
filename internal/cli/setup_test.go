package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/j-dsmith/punchcard/internal/cli"
	"github.com/j-dsmith/punchcard/internal/config"
)

func TestSetupNeedsATerminal(t *testing.T) {
	h := newHarness(t)
	r := h.run("setup")
	if r.code != 64 || !strings.Contains(r.stderr, "setup is interactive; run it in a terminal") {
		t.Errorf("exit %d stderr %q", r.code, r.stderr)
	}
	if r := h.runTTY("", nil, "setup", "--json"); r.code != 64 || h.setups != 0 {
		t.Errorf("--json: exit %d, wizard runs %d", r.code, h.setups)
	}
}

func TestSetupOnATerminalRunsTheWizard(t *testing.T) {
	h := newHarness(t)
	h.setupCode = 65
	if r := h.runTTY("", nil, "setup"); r.code != 65 || h.setups != 1 {
		t.Errorf("exit %d, wizard runs %d", r.code, h.setups)
	}
}

// A true first run (no config, no state) runs the wizard, then the TUI
// whatever the wizard did; anything already there skips it.
func TestFirstRun(t *testing.T) {
	cases := []struct {
		name   string
		before func(h *harness)
		wizard bool
	}{
		{"no config, no state", nil, true},
		{"state but no config", func(h *harness) { h.do("08:00", "add", "AA-1") }, false},
		{"config but no state", func(h *harness) {
			if err := config.Save(func(k string) string { return h.env[k] }, config.Config{SiteURL: "https://a.atlassian.net", Email: "e@a.test"}); err != nil {
				t.Fatal(err)
			}
		}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			h.env["XDG_CONFIG_HOME"] = filepath.Join(t.TempDir(), "config")
			if c.before != nil {
				c.before(h)
			}
			h.setupCode = 0
			var order []string
			r := h.runTTY("", func(e *cli.Env) {
				e.Setup = func(cli.Env) int { order = append(order, "setup"); return 1 } // even a failed wizard lands in the TUI
				e.TUI = func(cli.Env) int { order = append(order, "tui"); return 0 }
			})
			want := "tui"
			if c.wizard {
				want = "setup tui"
			}
			if got := strings.Join(order, " "); got != want || r.code != 0 {
				t.Errorf("ran %q exit %d, want %q", got, r.code, want)
			}
		})
	}
}

// With no HOME and no XDG dirs there is nowhere to write a config: no
// wizard, straight to the TUI.
func TestNoConfigDirIsNoFirstRun(t *testing.T) {
	h := newHarness(t)
	ran := false
	h.runTTY("", func(e *cli.Env) {
		e.Setup = func(cli.Env) int { ran = true; return 0 }
		e.TUI = func(cli.Env) int { return 0 }
	})
	if ran {
		t.Errorf("wizard ran with no config dir")
	}
	if _, err := os.Stat(filepath.Join(h.dir, "state.json")); err == nil {
		t.Errorf("state.json created")
	}
}

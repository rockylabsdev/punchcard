package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/j-dsmith/punchcard/internal/cli"
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

// A blank machine (no config, no state) is never forced into setup: it
// runs the command it was given, and bare punchcard opens the TUI.
func TestABlankMachineIsNeverForcedIntoSetup(t *testing.T) {
	blank := func(t *testing.T) (*harness, *[]string, func(*cli.Env)) {
		h := newHarness(t)
		h.env["XDG_CONFIG_HOME"] = filepath.Join(t.TempDir(), "config")
		var order []string
		return h, &order, func(e *cli.Env) {
			e.Setup = func(cli.Env) int { order = append(order, "setup"); return 0 }
			e.TUI = func(cli.Env) int { order = append(order, "tui"); return 0 }
		}
	}
	t.Run("bare punchcard on a terminal opens the TUI", func(t *testing.T) {
		h, order, stub := blank(t)
		r := h.runTTY("", stub)
		if got := strings.Join(*order, " "); got != "tui" || r.code != 0 {
			t.Errorf("ran %q exit %d, want the TUI alone", got, r.code)
		}
		if _, err := os.Stat(filepath.Join(h.dir, "state.json")); err == nil {
			t.Errorf("state.json created")
		}
	})
	t.Run("status prints the idle status", func(t *testing.T) {
		h, order, stub := blank(t)
		r := h.runTTY("", stub, "status")
		if r.code != 0 || !strings.Contains(r.stdout, "Nothing running") {
			t.Errorf("exit %d stdout %q", r.code, r.stdout)
		}
		if len(*order) > 0 {
			t.Errorf("ran %v", *order)
		}
	})
}

// With no HOME and no XDG dirs there is nowhere to write a config:
// still no wizard, straight to the TUI.
func TestNoConfigDirOpensTheTUI(t *testing.T) {
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

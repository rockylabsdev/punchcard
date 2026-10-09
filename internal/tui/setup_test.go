package tui

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"
	"github.com/j-dsmith/punchcard/internal/config"
	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/jira"
)

// suspend stubs ExecProcess: "running setup" is done(), then the
// callback fires with err, as Bubble Tea does once the process exits.
func (hs *harness) suspend(done func(), err error) *[]string {
	var ran []string
	hs.m.setup = func() *exec.Cmd { return exec.Command("punchcard", "setup") }
	hs.m.execProcess = func(c *exec.Cmd, fn tea.ExecCallback) tea.Cmd {
		ran = append(ran, strings.Join(c.Args, " "))
		return func() tea.Msg {
			done()
			return fn(err)
		}
	}
	return &ran
}

func TestSSuspendsIntoSetupAndRereads(t *testing.T) {
	hs := owing(t)
	hs.setup("08:20:00", core.RecordAuthFailure{})
	hs.open("09:12:00", 120, 32) // not configured, and a 401 on record
	if got := statusText(hs); !strings.HasPrefix(got, "✗ Jira rejected the token") || !strings.HasSuffix(got, "S to fix") {
		t.Fatalf("status line before = %q", got)
	}
	ran := hs.suspend(func() {
		// What a fresh setup does: config written, authFailedAt cleared.
		hs.jira = &jira.Fake{}
		hs.st.Apply(core.ClearAuthFailure{})
	}, nil)
	opened := false
	hs.m.jira = func() (jira.Client, error) {
		if !opened {
			return nil, config.ErrNotConfigured
		}
		return hs.jira, nil
	}
	hs.run(hs.m.probe())
	opened = true

	hs.key("S")

	if d := cmp.Diff([]string{"punchcard setup"}, *ran); d != "" {
		t.Errorf("ran (-want +got)\n%s", d)
	}
	// Only the failed worklog's banner is left: the token and the
	// connection are fixed.
	if got := statusText(hs); got != "✗ 1 worklog failed to reach Jira · r to retry · nothing is lost" {
		t.Errorf("status line after setup = %q", got)
	}
	if strings.Contains(hs.frame(), "token rejected") || strings.Contains(hs.frame(), "not connected") {
		t.Errorf("[3] still says Jira is not usable:\n%s", hs.frame())
	}
}

func TestSetupThatFailsSaysSo(t *testing.T) {
	hs := owing(t)
	hs.open("09:12:00", 120, 32)
	hs.suspend(func() {}, &exec.ExitError{})
	hs.key("S")
	if got := statusText(hs); !strings.HasPrefix(got, "setup exited with status") {
		t.Errorf("status line = %q", got)
	}
}

func TestSWithoutSetupTellsYouWhere(t *testing.T) {
	hs := owing(t)
	hs.open("09:12:00", 120, 32)
	hs.key("S")
	if got := statusText(hs); got != "setup is not available here; run punchcard setup in a shell" {
		t.Errorf("status line = %q", got)
	}
}

func TestKeyBarOffersSetup(t *testing.T) {
	hs := owing(t)
	hs.open("09:12:00", 120, 32)
	frameHas(t, hs, "S setup  ? help  q quit")
}

func TestNotConnectedBannerMentionsS(t *testing.T) {
	hs := owing(t)
	hs.unconfigured = true
	hs.open("09:12:00", 120, 32)
	if got := statusText(hs); got != "Not connected to Jira · S to set up · worklogs wait here, nothing is lost" {
		t.Errorf("status line = %q", got)
	}
}

// The add dialog's lookup is an authenticated answer: a recorded 401
// no longer holds.
func TestAddAnsweredByJiraClearsTheAuthFailure(t *testing.T) {
	hs := listed(newHarness(t))
	hs.setup("08:20:00", core.RecordAuthFailure{})
	hs.jira = &jira.Fake{Summaries: map[string]string{"PUN-7": "Fix it"}}
	hs.open("09:00:00", 120, 32)
	hs.key("a")
	hs.typeText("PUN-7")
	hs.key("enter")
	w, _, _ := hs.st.Read()
	if !w.State.AuthFailedAt.IsZero() {
		t.Errorf("authFailedAt still set after Jira answered")
	}
}

func TestStripNamesTheKeychain(t *testing.T) {
	hs := owing(t)
	hs.openErr = &config.KeychainError{Err: errors.New("security: boom")}
	hs.open("09:12:00", 120, 32)
	frameHas(t, hs, "· Keychain error")
}

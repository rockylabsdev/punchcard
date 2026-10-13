//go:build unix

package setup

import (
	"image/color"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
)

// screen is `punchcard setup` on a pseudo-terminal with a dark
// background, rendered by a terminal emulator that also answers the
// queries Huh and lipgloss send.
type screen struct {
	t    *testing.T
	ptmx io.Writer
	mu   sync.Mutex // the emulator's own lock leaves String unguarded
	emu  *vt.Emulator
}

func (s *screen) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.emu.Write(p)
}

func (s *screen) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.emu.String()
}

const ptyCols, ptyRows = 100, 70

func newScreen(t *testing.T, r *rig) (*screen, <-chan int) {
	t.Helper()
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	t.Cleanup(func() { ptmx.Close(); tty.Close() })
	// Huh draws nothing on a 0×0 terminal.
	if err := pty.Setsize(ptmx, &pty.Winsize{Cols: ptyCols, Rows: ptyRows}); err != nil {
		t.Fatal(err)
	}
	s := &screen{t: t, ptmx: ptmx, emu: vt.NewEmulator(ptyCols, ptyRows)}
	s.emu.SetDefaultBackgroundColor(color.Black)
	go io.Copy(s, ptmx)     //nolint:errcheck // ends when the pty closes
	go io.Copy(ptmx, s.emu) //nolint:errcheck // the emulator's replies to queries
	ui := NewTerminal(tty, tty, []string{"TERM=xterm-256color"})
	done := make(chan int, 1)
	go func() {
		ui.Begin()
		d := Deps{
			UI: ui, Getenv: r.getenv, Connect: r.jira.connect, Claude: r.claude,
			Keychain: r.kc, Store: r.store, Version: "v0.1.0",
			Now: func() time.Time { return r.now },
		}
		code := Run(d)
		ui.End()
		done <- code
	}()
	return s, done
}

func (s *screen) lines() []string {
	return strings.Split(s.String(), "\n")
}

// waitFor waits until a line of the screen starts with prefix and
// returns that line's index.
func (s *screen) waitFor(prefix string) int {
	s.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		for i, l := range s.lines() {
			if strings.HasPrefix(l, prefix) {
				return i
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	s.t.Fatalf("no line starting %q; screen:\n%s", prefix, s.String())
	return -1
}

func (s *screen) send(keys string) {
	s.t.Helper()
	if _, err := io.WriteString(s.ptmx, keys); err != nil {
		s.t.Fatal(err)
	}
}

// closed checks the screen above the open form, whose title line
// starts with next: it holds the closed step's summary, title and
// answer, and no line of that step's form.
func (s *screen) closed(next, title, answer string) {
	s.t.Helper()
	at := s.waitFor(next)
	above := s.lines()[:at]
	text := strings.Join(above, "\n")
	if !strings.Contains(text, "◇  "+title+"\n│  "+answer) {
		s.t.Errorf("no summary %q / %q above %q; screen:\n%s", title, answer, next, s.String())
	}
	for _, l := range above {
		if strings.HasPrefix(l, "┃") {
			s.t.Errorf("a closed form's line %q is left above %q; screen:\n%s", l, next, s.String())
			return
		}
	}
}

// Each closed step, a select, a text input, a secret input and a
// Yes/No question, leaves only its summary on screen (#54).
func TestTerminalStepLeavesOnlyItsSummary(t *testing.T) {
	r := newRig(t)
	s, done := newScreen(t, r)

	s.waitFor("┃ Tracker")
	s.send("\x1b[B\r") // down to Jira
	s.closed("┃ Jira site", "Tracker", "Jira: stopped time becomes Jira worklogs")

	s.send("acme.atlassian.net\r")
	s.closed("┃ Email", "Jira site", site)

	s.send(email + "\r")
	s.closed("┃ API token", "Email", email)

	s.send(tok + "\r")
	s.closed("┃ Save these credentials?", "API token", "••••••••")

	s.send("\r") // Yes is the default
	s.closed("┃ Add the ask rule", "Save these credentials?", "Yes")

	s.send("\r")
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("exit %d", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("setup did not finish; screen:\n%s", s.String())
	}
	s.waitFor("│  Done.")
	text := s.String()
	for _, l := range s.lines() {
		if strings.HasPrefix(l, "┃") {
			t.Errorf("a closed form's line %q is left; screen:\n%s", l, text)
			break
		}
	}
}

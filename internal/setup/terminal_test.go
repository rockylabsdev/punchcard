package setup

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
)

// The answer under a closed step is in the terminal's own foreground,
// never ANSI 8, which many dark themes draw near the background.
func TestAnsweredAnswerIsNotANSI8(t *testing.T) {
	var buf bytes.Buffer
	ui := &Terminal{out: &colorprofile.Writer{Forward: &buf, Profile: colorprofile.ANSI256}}
	ui.answered("Tracker", "Jira: stopped time becomes Jira worklogs")
	got := buf.String()
	if !strings.Contains(got, "Jira: stopped time becomes Jira worklogs") {
		t.Fatalf("no answer in %q", got)
	}
	for _, l := range strings.Split(got, "\n") {
		if !strings.Contains(l, "Jira: stopped") {
			continue
		}
		answer := l[strings.Index(l, "│")+len("│"):]
		if strings.Contains(answer, "\x1b[90m") || strings.Contains(answer, "38;5;8m") {
			t.Errorf("answer drawn in ANSI 8: %q", l)
		}
	}
}

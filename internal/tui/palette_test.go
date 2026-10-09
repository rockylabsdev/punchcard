package tui

import (
	"bytes"
	"regexp"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/jira"
)

// run is a stretch of text and the SGR parameters it is drawn with.
type run struct {
	params []string
	text   string
}

var sgr = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// runs renders the model's frame as a 16-colour terminal would receive
// it and splits it into styled stretches.
func runs(t *testing.T, hs *harness) []run {
	t.Helper()
	var buf bytes.Buffer
	w := &colorprofile.Writer{Forward: &buf, Profile: colorprofile.ANSI}
	if _, err := w.WriteString(hs.m.View().Content); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	var out []run
	var params []string
	last := 0
	for _, loc := range sgr.FindAllStringSubmatchIndex(s, -1) {
		if text := s[last:loc[0]]; text != "" {
			out = append(out, run{params, text})
		}
		params = nil
		if p := s[loc[2]:loc[3]]; p != "" && p != "0" {
			params = strings.Split(p, ";")
		}
		last = loc[1]
	}
	return out
}

// palette is a day with every state on screen, in a 16-colour terminal.
func palette(t *testing.T) *harness {
	hs := newHarness(t)
	hs.setup("04:00:00", add("REC-1"), start("REC-1", "E-R"))
	hs.setup("04:30:00", core.Touch{Origin: core.OriginHand})
	day(hs) // its first start finds REC-1 stale: recovered
	hs.setup("08:20:00", add("SYN-1"), start("SYN-1", "E-S"))
	hs.setup("08:40:00", core.Stop{Origin: core.OriginHand})
	hs.setup("08:41:00", core.MarkSynced{Key: "SYN-1", EntryID: "E-S", WorklogID: "W1"})
	hs.setup("08:58:00", start("PUN-142", "E-142b"))
	hs.jira = &jira.Fake{}
	hs.open("09:00:00", 160, 40)
	hs.send(tea.ColorProfileMsg{Profile: colorprofile.ANSI})
	hs.tick("09:12:34")
	return hs
}

// Each state's symbol has its ANSI slot: the colours are the terminal's
// own 16, so they follow its theme. Running is the brand colour, which
// falls back to bright green.
func TestEachStateHasItsANSISlot(t *testing.T) {
	hs := palette(t)
	rs := runs(t, hs)
	cases := []struct {
		state, glyph string
		want         []string // exact SGR parameters
	}{
		{"running", gRunning, []string{"1", "92"}},
		{"paused", gPaused, []string{"33"}},
		{"recovered", gRecovered, []string{"1", "35"}},
		{"failed", gFailed, []string{"1", "31"}},
		{"waiting to sync", gPending, []string{"34"}},
		{"synced", gSynced, []string{"90"}},
		{"idle", gIdle, []string{"90"}},
		{"unverified", gUnverified + " unverified", []string{"90"}},
	}
	for _, c := range cases {
		found := false
		for _, r := range rs {
			if strings.HasPrefix(r.text, c.glyph) {
				found = true
				if !slices.Equal(sorted(r.params), c.want) {
					t.Errorf("%s %q is drawn with %v, want %v", c.state, r.text, r.params, c.want)
				}
			}
		}
		if !found {
			t.Errorf("no %s glyph %s on screen:\n%s", c.state, c.glyph, hs.frame())
		}
	}
}

// Blue is reserved for sync: the waiting glyph and the r key, nothing
// else, in every state of the status line.
func TestBlueIsOnlyForSync(t *testing.T) {
	hs := palette(t)
	check := func(when string) {
		t.Helper()
		blue := 0
		for _, r := range runs(t, hs) {
			if !slices.Contains(r.params, "34") {
				continue
			}
			blue++
			txt := strings.TrimSpace(r.text)
			if txt != "r" && !strings.HasPrefix(txt, gPending) {
				t.Errorf("%s: %q is blue, but blue is only for sync", when, r.text)
			}
		}
		if blue == 0 {
			t.Errorf("%s: nothing is blue; expected ○ rows and the r key", when)
		}
	}
	check("at rest")
	next, _ := hs.m.Update(keyPress("r"))
	hs.m = next.(Model)
	check("syncing")
}

// Greys come from the 256 cube; in a 16-colour terminal they fall back
// to bright black, never to white or black.
func TestGreysFallBackToBrightBlack(t *testing.T) {
	hs := palette(t)
	for _, r := range runs(t, hs) {
		for _, p := range r.params {
			if p == "37" || p == "30" || p == "40" || p == "47" {
				t.Errorf("%q is drawn with %v: a grey fell back to white or black", r.text, r.params)
			}
		}
	}
}

func sorted(s []string) []string {
	s = slices.Clone(s)
	slices.Sort(s)
	return s
}

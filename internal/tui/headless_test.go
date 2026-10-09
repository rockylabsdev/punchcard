package tui

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// TestHeadlessKeyRun drives the model with thousands of keystrokes,
// ticks and resizes through Update, rendering after each, and checks
// that nothing panics and every frame is exactly the window's size.
func TestHeadlessKeyRun(t *testing.T) {
	// steps per scenario: 1400 x 11 scenarios keeps the total near the
	// 16000 of 2000 x 8 that the package could afford under -race.
	const steps = 1400
	keys := []string{"space", "enter", "p", "j", "k", "up", "down", "g", "G", "x", "?", "r",
		"a", "d", "y", "n", "esc", "tab", "shift+tab", "backspace", "1", "h", "m", "P", "U", "N", "-", "4", ":"}
	sizes := [][2]int{{80, 24}, {120, 32}, {60, 16}, {44, 10}, {30, 6}, {10, 3}, {1, 1}}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			t.Parallel()
			hs := newHarness(t)
			sc.build(hs)
			hs.configure()
			hs.open("09:00:00", 80, 24)
			hs.play(sc.keys...)
			rng := rand.New(rand.NewPCG(1, 2))
			w, h := 80, 24
			for i := range steps {
				var msg tea.Msg
				switch r := rng.IntN(100); {
				case r < 5:
					s := sizes[rng.IntN(len(sizes))]
					w, h = s[0], s[1]
					msg = tea.WindowSizeMsg{Width: w, Height: h}
				case r < 25:
					hs.now = hs.now.Add(time.Duration(rng.IntN(120)) * time.Second)
					msg = tickMsg(hs.now)
				default:
					msg = keyPress(keys[rng.IntN(len(keys))])
				}
				hs.send(msg)
				checkFrame(t, i, msg, hs.m.View().Content, w, h)
			}
		})
	}
}

func checkFrame(t *testing.T, step int, msg tea.Msg, frame string, w, h int) {
	t.Helper()
	lines := strings.Split(frame, "\n")
	if len(lines) != h {
		t.Fatalf("step %d (%v): frame has %d lines, want %d:\n%s", step, msg, len(lines), h, frame)
	}
	for n, l := range lines {
		if got := ansi.StringWidth(l); got != w {
			t.Fatalf("step %d (%v): line %d is %d cells, want %d: %q", step, msg, n, got, w, fmt.Sprint(l))
		}
	}
}

package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Semantic colours are the 16 ANSI slots, so they follow the terminal's
// theme; only greys come from the 256 cube (they fall back to bright
// black). Blue is reserved for sync. Every state also has a symbol, so
// the frame reads with no colour at all.
//
// The one colour that is Punchcard's own is the brand, a chartreuse: the
// running state, the focused pane and the cursor. It has a darker shade
// for light backgrounds, and on 16 colours it falls back to bright green.
var (
	BrandDark  = lipgloss.Color("#C8FF00")
	BrandLight = lipgloss.Color("#5F8700")

	cRed     = lipgloss.Color("1")
	cGreen   = lipgloss.Color("2")
	cYellow  = lipgloss.Color("3")
	cBlue    = lipgloss.Color("4")
	cMagenta = lipgloss.Color("5")
	cDim     = lipgloss.Color("244")
	cFaint   = lipgloss.Color("238")
	cSelBg   = lipgloss.Color("236")
)

// greyFallback rewrites the 256-cube greys for a 16-colour terminal,
// whose nearest match would be white (244) or black (238, 236): a grey
// foreground becomes bright black, a grey background (the selection)
// the terminal's own, leaving the cursor › to mark the row.
func greyFallback(frame string) string {
	return sgrPattern.ReplaceAllStringFunc(frame, func(seq string) string {
		params := strings.Split(seq[2:len(seq)-1], ";")
		out := make([]string, 0, len(params))
		for i := 0; i < len(params); i++ {
			if i+2 < len(params) && (params[i] == "38" || params[i] == "48") && params[i+1] == "5" {
				if n, err := strconv.Atoi(params[i+2]); err == nil && n >= 232 {
					if params[i] == "38" {
						out = append(out, "90")
					} else {
						out = append(out, "49")
					}
					i += 2
					continue
				}
			}
			out = append(out, params[i])
		}
		return "\x1b[" + strings.Join(out, ";") + "m"
	})
}

var sgrPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// brandStyles holds the styles that depend on the terminal's background:
// everything drawn in the brand colour.
type brandStyles struct {
	brand   lipgloss.Style // the cursor, the focused pane's title
	running lipgloss.Style // the running state
	focus   lipgloss.Style // the focused pane's border
}

// newPalette picks the brand shade for a dark or a light background.
func newPalette(dark bool) brandStyles {
	c := lipgloss.LightDark(dark)(BrandLight, BrandDark)
	return brandStyles{
		brand:   lipgloss.NewStyle().Foreground(c).Bold(true),
		running: lipgloss.NewStyle().Foreground(c).Bold(true),
		focus:   lipgloss.NewStyle().Foreground(c),
	}
}

var (
	sDim     = lipgloss.NewStyle().Foreground(cDim)
	sFaint   = lipgloss.NewStyle().Foreground(cFaint)
	sBold    = lipgloss.NewStyle().Bold(true)
	sPaused  = lipgloss.NewStyle().Foreground(cYellow)
	sRecov   = lipgloss.NewStyle().Foreground(cMagenta).Bold(true)
	sSync    = lipgloss.NewStyle().Foreground(cBlue)
	sSyncKey = lipgloss.NewStyle().Foreground(cBlue).Bold(true)
	sFail    = lipgloss.NewStyle().Foreground(cRed).Bold(true)
	sWarn    = lipgloss.NewStyle().Foreground(cYellow) // not connected: worklogs wait
	sSel     = lipgloss.NewStyle().Background(cSelBg)
	sChanged = lipgloss.NewStyle().Italic(true) // a row changed elsewhere; blue stays sync's
)

// Symbols are single-cell, no emoji, none whose width varies.
const (
	gRunning    = "▶"
	gPaused     = "‖"
	gRecovered  = "!"
	gIdle       = "·"
	gUnverified = "?"
	gPending    = "○"
	gFailed     = "✗"
	gSynced     = "✓"
	gCursor     = "›"
)

// clock is a running timer: h:mm:ss.
func clock(d time.Duration) string {
	d = d.Truncate(time.Second)
	return fmt.Sprintf("%d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}

// hm is a still amount: 45m, 2h, 1h 12m.
func hm(d time.Duration) string {
	h, m := int(d.Hours()), int(d.Minutes())%60
	switch {
	case h == 0:
		return fmt.Sprintf("%dm", m)
	case m == 0:
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %02dm", h, m)
}

// fit pads or truncates a styled string to exactly w cells.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, "…")
	return s + strings.Repeat(" ", max(0, w-lipgloss.Width(s)))
}

// rfit is fit aligned right.
func rfit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, "…")
	return strings.Repeat(" ", max(0, w-lipgloss.Width(s))) + s
}

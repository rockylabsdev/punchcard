package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Semantic colours are the 16 ANSI slots, so they follow the terminal's
// theme; only greys come from the 256 cube (they fall back to bright
// black). Blue is reserved for sync. Every state also has a symbol, so
// the frame reads with no colour at all.
var (
	cRed     = lipgloss.Color("1")
	cGreen   = lipgloss.Color("2")
	cYellow  = lipgloss.Color("3")
	cBlue    = lipgloss.Color("4")
	cMagenta = lipgloss.Color("5")
	cFg      = lipgloss.Color("7")
	cDim     = lipgloss.Color("244")
	cFaint   = lipgloss.Color("238")
	cSelBg   = lipgloss.Color("236")
)

var (
	sDim     = lipgloss.NewStyle().Foreground(cDim)
	sFaint   = lipgloss.NewStyle().Foreground(cFaint)
	sBold    = lipgloss.NewStyle().Bold(true)
	sFocus   = lipgloss.NewStyle().Foreground(cFg)
	sRunning = lipgloss.NewStyle().Foreground(cGreen).Bold(true)
	sPaused  = lipgloss.NewStyle().Foreground(cYellow)
	sRecov   = lipgloss.NewStyle().Foreground(cMagenta).Bold(true)
	sSync    = lipgloss.NewStyle().Foreground(cBlue)
	sFail    = lipgloss.NewStyle().Foreground(cRed).Bold(true)
	sSel     = lipgloss.NewStyle().Background(cSelBg)
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

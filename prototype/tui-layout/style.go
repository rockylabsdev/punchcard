// PROTOTYPE — throwaway. Palette and glyphs under test.
//
// Proposal being tested: semantic colours are the 16 ANSI slots, so they follow
// the user's terminal theme; only greys come from the 256 cube (they downsample
// to bright-black). Every state also has a glyph and/or attribute, so the
// no-colour mode must still read.
//
//	green   (2) on the clock: running row, header        ▶  bold
//	yellow  (3) paused                                    ‖
//	magenta (5) recovered: check the duration             !
//	blue    (4) RESERVED for sync: pending ○, synced ✓, the r key
//	red     (1) failure: a failed worklog ✗, the banner   reverse in no-colour
package main

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

var (
	cGreen   = lipgloss.Color("2")
	cYellow  = lipgloss.Color("3")
	cBlue    = lipgloss.Color("4")
	cMagenta = lipgloss.Color("5")
	cRed     = lipgloss.Color("1")
	cDim     = lipgloss.Color("244") // → bright-black in 16 colours
	cFaint   = lipgloss.Color("238")
	cSelBg   = lipgloss.Color("236")
	cFg      = lipgloss.Color("7")
)

var (
	sDim     = lipgloss.NewStyle().Foreground(cDim)
	sFaint   = lipgloss.NewStyle().Foreground(cFaint)
	sBold    = lipgloss.NewStyle().Bold(true)
	sRunning = lipgloss.NewStyle().Foreground(cGreen).Bold(true)
	sPaused  = lipgloss.NewStyle().Foreground(cYellow)
	sRecov   = lipgloss.NewStyle().Foreground(cMagenta).Bold(true)
	sSync    = lipgloss.NewStyle().Foreground(cBlue)
	sFail    = lipgloss.NewStyle().Foreground(cRed).Bold(true)
	sKey     = lipgloss.NewStyle().Bold(true)
	sSel     = lipgloss.NewStyle().Background(cSelBg)
	sChanged = lipgloss.NewStyle().Foreground(cBlue).Italic(true)

	sBannerErr  = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(cRed).Bold(true).Reverse(false)
	sBannerWarn = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(cYellow).Bold(true)
)

// glyphs: single-cell, no emoji, no East-Asian-ambiguous width
const (
	gRunning    = "▶"
	gPaused     = "‖"
	gIdle       = "·"
	gRecovered  = "!"
	gPending    = "○"
	gSynced     = "✓"
	gFailed     = "✗"
	gUnverified = "?"
	gCursor     = "›"
)

func stateGlyph(t *ticket) string {
	switch t.state() {
	case stRunning:
		return sRunning.Render(gRunning)
	case stPaused:
		if t.recovered() {
			return sRecov.Render(gRecovered)
		}
		return sPaused.Render(gPaused)
	}
	return sDim.Render(gIdle)
}

func syncGlyphs(t *ticket) string {
	p, s, f := t.counts()
	out := ""
	if f > 0 {
		out += sFail.Render(gFailed) + itoa(f) + " "
	}
	if p > 0 {
		out += sSync.Render(gPending) + itoa(p) + " "
	}
	if s > 0 {
		out += sDim.Render(gSynced+itoa(s)) + " "
	}
	return strings.TrimSpace(out)
}

func syncGlyph(e *entry) string {
	switch e.sync {
	case syncPending:
		return sSync.Render(gPending + " pending")
	case syncSynced:
		return sDim.Render(gSynced + " synced")
	case syncFailed:
		return sFail.Render(gFailed + " failed")
	}
	return ""
}

func itoa(n int) string { return strconv.Itoa(n) }

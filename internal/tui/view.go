package tui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/j-dsmith/punchcard/internal/core"
)

// frame is the settled lazygit-style layout, top to bottom:
//
//	[1] Tickets (about 62%) | [2] Ticket      body, all the height left
//	[3] Jira, full width, sized to its rows
//	status line
//	key bar
//
// Each region is one method returning its own lines at the size it is
// given; a later slice fills a region in without touching this layout.
func (m Model) frame() string {
	w, h := m.w, m.h
	jira := m.jiraStrip(w, max(1, min(5, h/4)))
	status := m.statusLine(w)
	keys := m.keyBar(w)

	bodyH := max(0, h-len(jira)-2)
	lw := ticketsWidth(w)
	body := joinColumns(m.ticketsPane(lw, bodyH), m.ticketPane(w-lw, bodyH))

	lines := append(body, jira...)
	lines = append(lines, status, keys)
	base := fitFrame(lines, w, h)
	if m.dlg == nil || bodyH == 0 || w == 0 {
		return base
	}
	// A dialog is laid over [2] at exactly its size; a window too narrow
	// for [2] gets it over the whole body instead. Nothing underneath
	// moves.
	x, dw := lw, w-lw
	if _, wide := m.dlg.(helpDialog); wide || dw < minDialogWidth {
		x, dw = 0, w
	}
	top := strings.Join(pane(m.dlg.title(), m.dlg.body(m, dw-2), dw, bodyH, true), "\n")
	return overlay(base, top, x, 0, w, h)
}

// minDialogWidth is the narrowest [2] a dialog is laid over.
const minDialogWidth = 30

// overlay composites top over base at (x, y) with Lip Gloss's
// compositor, keeping the frame exactly w×h.
func overlay(base, top string, x, y, w, h int) string {
	out := lipgloss.NewCompositor(
		lipgloss.NewLayer(base),
		lipgloss.NewLayer(top).X(x).Y(y).Z(1),
	).Render()
	return fitFrame(strings.Split(out, "\n"), w, h)
}

// ticketsWidth is [1]'s share: about 62%, never under 44 columns while
// the window has them.
func ticketsWidth(w int) int {
	if lw := w * 62 / 100; lw >= 44 {
		return lw
	}
	return min(w, 44)
}

// fitFrame makes the frame exactly w×h: every line padded or cut to w
// cells, and lines added or dropped to h. A frame is never taller than
// the window, whatever a region returned.
func fitFrame(lines []string, w, h int) string {
	out := make([]string, h)
	for i := range out {
		l := ""
		if i < len(lines) {
			l = lines[i]
		}
		out[i] = fit(l, w)
	}
	return strings.Join(out, "\n")
}

// joinColumns sets two equal-height blocks side by side.
func joinColumns(left, right []string) []string {
	out := make([]string, max(len(left), len(right)))
	for i := range out {
		if i < len(left) {
			out[i] = left[i]
		}
		if i < len(right) {
			out[i] += right[i]
		}
	}
	return out
}

// pane is a rounded box w×h with its title set into the top border.
// The focused pane's border is the foreground colour, others faint.
func pane(title string, body []string, w, h int, focused bool) []string {
	if h <= 0 {
		return nil
	}
	out := make([]string, 0, h)
	if w < 2 {
		for range h {
			out = append(out, strings.Repeat(" ", max(0, w)))
		}
		return out
	}
	border, tstyle := sFaint, sDim
	if focused {
		border, tstyle = sFocus, sBold
	}
	iw := w - 2
	title = ansi.Truncate(title, max(0, iw-1), "…")
	out = append(out, border.Render("╭─")+tstyle.Render(title)+
		border.Render(strings.Repeat("─", max(0, iw-1-lipgloss.Width(title)))+"╮"))
	for i := range h - 2 {
		l := ""
		if i < len(body) {
			l = body[i]
		}
		out = append(out, border.Render("│")+fit(l, iw)+border.Render("│"))
	}
	if h > 1 {
		out = append(out, border.Render("╰"+strings.Repeat("─", iw)+"╯"))
	}
	return out
}

// binding is one entry on the key bar.
type binding struct{ key, label string }

// bindings are the keys that apply now.
func (m Model) bindings() []binding {
	if m.dlg != nil {
		return m.dlg.bindings()
	}
	var bs []binding
	rows := m.rows()
	sel, ok := m.selected()
	if ok {
		label := "start"
		switch stateOf(sel) {
		case stateRunning:
			label = "pause"
		case statePaused, stateRecovered:
			label = "resume"
		}
		bs = append(bs, binding{"space", label})
		if stateOf(sel) != stateIdle {
			bs = append(bs, binding{"x", "stop…"})
		}
	}
	// p pauses from any row; on the running row space already says so.
	if len(rows) > 0 && stateOf(rows[0]) == stateRunning && (!ok || sel.Key != rows[0].Key) {
		bs = append(bs, binding{"p", "pause"})
	}
	bs = append(bs, binding{"a", "add"})
	if ok {
		bs = append(bs, binding{"d", "remove"})
	}
	// Movement is in help: the bar keeps room for what changes things.
	if n, _ := core.Owed(m.shown().State); n > 0 {
		bs = append(bs, binding{"r", "sync " + strconv.Itoa(n)})
	}
	return append(bs, binding{"S", "setup"}, binding{"?", "help"}, binding{"q", "quit"})
}

// keyBar shows the keys that apply now. r is sync blue, like every
// other mention of sync.
func (m Model) keyBar(w int) string {
	parts := make([]string, 0, 8)
	for _, b := range m.bindings() {
		k := sBold.Render(b.key)
		if b.key == "r" {
			k = sSyncKey.Render(b.key)
		}
		parts = append(parts, k+" "+sDim.Render(b.label))
	}
	return fit(" "+strings.Join(parts, "  "), w)
}

// wrap breaks s into lines of at most w cells.
func wrap(s string, w int) []string {
	if w <= 0 {
		return nil
	}
	return strings.Split(ansi.Wrap(s, w, ""), "\n")
}

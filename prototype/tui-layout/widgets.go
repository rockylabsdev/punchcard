// PROTOTYPE — throwaway. Pieces the variants share: rows, panes, dialogs.
package main

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// fit pads or truncates a styled string to exactly w cells.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, "…")
	if pad := w - lipgloss.Width(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

func rfit(s string, w int) string {
	s = ansi.Truncate(s, w, "…")
	if pad := w - lipgloss.Width(s); pad > 0 {
		s = strings.Repeat(" ", pad) + s
	}
	return s
}

// pane draws a lazygit-style box with the title set into the top border.
func pane(title, body string, w, h int, focused bool) string {
	bc := sFaint
	tc := sDim
	if focused {
		bc = lipgloss.NewStyle().Foreground(cFg)
		tc = sBold
	}
	iw := w - 2
	top := bc.Render("╭─") + tc.Render(title) + bc.Render(strings.Repeat("─", max(0, iw-1-lipgloss.Width(title)))+"╮")
	lines := strings.Split(body, "\n")
	out := []string{top}
	for i := 0; i < h-2; i++ {
		l := ""
		if i < len(lines) {
			l = lines[i]
		}
		out = append(out, bc.Render("│")+fit(l, iw)+bc.Render("│"))
	}
	out = append(out, bc.Render("╰"+strings.Repeat("─", iw)+"╯"))
	return strings.Join(out, "\n")
}

// summary text, honest about unverified keys
func summaryText(t *ticket) string {
	if t.unverified {
		return sDim.Render(gUnverified + " unverified — Jira not reached yet")
	}
	return t.summary
}

// ticketRow is the one-line row used by A and B (C is denser).
func (m model) ticketRow(t *ticket, w int, selected bool, keyW int) string {
	cur := " "
	if selected {
		cur = gCursor
	}
	total := t.total(m.now)
	tot := sDim.Render(hm(total))
	if total == 0 {
		tot = sFaint.Render("—")
	}
	key := t.key
	sum := summaryText(t)
	switch t.state() {
	case stRunning:
		key = sRunning.Render(t.key)
		sum = sRunning.Render(t.summary)
		tot = sRunning.Render(clock(total))
	case stPaused:
		key = sPaused.Render(t.key)
		if t.recovered() {
			key = sRecov.Render(t.key)
			tot = sRecov.Render(hm(total) + "?")
		}
	}
	if c := m.changed(t); c != "" {
		sum = c + sDim.Render(" · ") + sum
	}
	sync := syncGlyphs(t)
	syncW := 6
	sumW := w - 2 - 1 - keyW - 1 - 9 - syncW
	row := cur + stateGlyph(t) + " " + fit(key, keyW) + " " + fit(sum, sumW) + rfit(tot, 8) + " " + fit(" "+sync, syncW)
	if selected {
		return sSel.Render(fit(row, w))
	}
	return fit(row, w)
}

// entryLines describes a ticket's entries: the open one's segments and each worklog.
func (m model) entryLines(t *ticket, w int) []string {
	var out []string
	if c := t.current(); c != nil {
		state := sPaused.Render(gPaused + " paused")
		if c.open() {
			state = sRunning.Render(gRunning + " running")
		}
		out = append(out, state+sDim.Render(fmt.Sprintf(" · %s this entry · %d segment(s)", clock(c.elapsed(m.now)), len(c.segs))))
		for _, s := range c.segs {
			end, until := sRunning.Render("now  "), m.now
			if !s.end.IsZero() {
				end, until = s.end.Format("15:04"), s.end
			}
			out = append(out, sDim.Render("  "+s.start.Format("15:04")+"–")+end+sDim.Render("  "+hm(until.Sub(s.start))))
		}
		if c.recovered {
			out = append(out, sRecov.Render(gRecovered+" recovered: capped at "+c.cappedAt.Format("15:04")+","))
			out = append(out, sRecov.Render("  your last presence. Check it at stop."))
		}
	}
	any := false
	for _, e := range t.entries {
		if !e.stopped {
			continue
		}
		if !any {
			out = append(out, "", sDim.Render("worklogs"))
			any = true
		}
		l := "  " + e.stoppedAt.Format("15:04") + "  " + fit(hm(e.sendDur), 7) + " " + syncGlyph(e)
		if e.comment != "" {
			l += sDim.Render("  “" + e.comment + "”")
		}
		out = append(out, l)
		if e.sync == syncFailed {
			out = append(out, sFail.Render("    "+e.syncErr))
		}
	}
	if len(out) == 0 {
		out = append(out, sDim.Render("no time yet — space starts it"))
	}
	return out
}

// stopForm is the stop dialog's body; the variants frame it differently.
func (m model) stopForm(w int) string {
	t := m.stopT
	c := t.current()
	measured := c.elapsed(m.now)
	field := func(label string, in string, focused bool) string {
		l := sDim.Render(label)
		if focused {
			l = sBold.Render(label)
		}
		box := lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, true, false).BorderForeground(cDim)
		if focused {
			box = box.BorderForeground(cFg)
		}
		return l + "\n" + box.Width(min(w, 44)).Render(in)
	}
	lines := []string{
		sBold.Render(t.key) + " " + sDim.Render(ansi.Truncate(t.summary, max(0, w-len(t.key)-1), "…")),
		sDim.Render(fmt.Sprintf("measured %s across %d segment(s)", clock(measured), len(c.segs))),
	}
	if c.recovered {
		lines = append(lines, sRecov.Render(gRecovered+" recovered — capped at "+c.cappedAt.Format("15:04")+". Is this right?"))
	}
	lines = append(lines, "", field("Duration", m.durIn.View(), m.stopFoc == 0), field("Comment", m.comIn.View(), m.stopFoc == 1), "")
	if d, ok := parseDur(m.durIn.Value()); ok && d >= time.Minute {
		lines = append(lines, "→ sends "+sSync.Bold(true).Render(hm(d))+" to Jira as one worklog")
	} else {
		lines = append(lines, sFail.Render(gFailed+" can't read that duration"))
	}
	if m.stopErr != "" {
		lines = append(lines, sFail.Render(m.stopErr))
	}
	return strings.Join(lines, "\n")
}

func (m model) addForm(w int) string {
	lines := []string{sBold.Render("Ticket key"), "› " + m.addIn.View()}
	if m.addErr != "" {
		lines = append(lines, sFail.Render(ansi.Truncate(m.addErr, w, "…")))
	} else {
		lines = append(lines, sDim.Render("try XXX-1 (not found) or OFF-1 (offline)"))
	}
	lines = append(lines, "", sDim.Render("recent"))
	for i, r := range m.st.recents {
		cur := "  "
		row := r.key + " " + sDim.Render(r.summary)
		if i == m.addSel {
			cur = gCursor + " "
			row = sSel.Render(fit(r.key+" "+r.summary, w-2))
		}
		lines = append(lines, ansi.Truncate(cur+row, w, "…"))
	}
	return strings.Join(lines, "\n")
}

// head is the one-glance answer to "am I on the clock?"
func (m model) clockState() (glyph, label string, t *ticket, st lipgloss.Style) {
	if r := m.st.running(); r != nil {
		return gRunning, "ON THE CLOCK", r, sRunning
	}
	for _, t := range m.st.sorted() {
		if t.state() == stPaused {
			if t.recovered() {
				return gRecovered, "RECOVERED", t, sRecov
			}
			return gPaused, "PAUSED", t, sPaused
		}
	}
	return gIdle, "off the clock", nil, sDim
}

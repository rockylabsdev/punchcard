// PROTOTYPE — throwaway. Variant C "Clock": the running entry in big digits
// owns the top of the screen; tickets are a compact list underneath with the
// worklog tray beside it. Stop and add slide up as a drawer over the lower half.
package main

import (
	"strings"

	"charm.land/lipgloss/v2"
)

var font = map[rune][5]string{
	'0': {"111", "101", "101", "101", "111"},
	'1': {"010", "110", "010", "010", "111"},
	'2': {"111", "001", "111", "100", "111"},
	'3': {"111", "001", "111", "001", "111"},
	'4': {"101", "101", "111", "001", "001"},
	'5': {"111", "100", "111", "001", "111"},
	'6': {"111", "100", "111", "101", "111"},
	'7': {"111", "001", "001", "001", "001"},
	'8': {"111", "101", "111", "101", "111"},
	'9': {"111", "101", "111", "001", "111"},
	':': {"0", "1", "0", "1", "0"},
	'-': {"000", "000", "111", "000", "000"},
}

// bigText renders with half blocks: 5 pixel rows → 3 text rows, 2 cells per pixel.
func bigText(s string) string {
	var out [3]strings.Builder
	for i, r := range s {
		g, ok := font[r]
		if !ok {
			continue
		}
		if i > 0 {
			for k := range out {
				out[k].WriteString(" ")
			}
		}
		for line := 0; line < 3; line++ {
			top := g[line*2]
			bot := strings.Repeat("0", len(top))
			if line*2+1 < 5 {
				bot = g[line*2+1]
			}
			for x := range top {
				a, b := top[x] == '1', bot[x] == '1'
				c := " "
				switch {
				case a && b:
					c = "█"
				case a:
					c = "▀"
				case b:
					c = "▄"
				}
				out[line].WriteString(c + c)
			}
		}
	}
	return out[0].String() + "\n" + out[1].String() + "\n" + out[2].String()
}

func (m model) viewC(w, h int) string {
	var lines []string
	if b := m.banner(w); b != "" {
		lines = append(lines, b)
	}
	g, label, t, st := m.clockState()
	digits := "-:--:--"
	caption := st.Render(g+" "+label) + sDim.Render("   today "+hm(m.st.today(m.now)))
	sub := sDim.Render("space starts the selected ticket")
	if t != nil {
		digits = clock(t.current().elapsed(m.now))
		sub = st.Render(t.key) + " " + t.summary
	}
	if m.mode == modeStop || m.mode == modeAdd {
		// the drawer needs the room: the clock collapses to one line
		lines = append(lines, fit(" "+st.Render(g+" "+digits)+"  "+sub, w))
	} else {
		big := st.Render(bigText(digits))
		head := lipgloss.JoinVertical(lipgloss.Center, "", big, "", caption, sub)
		lines = append(lines, lipgloss.PlaceHorizontal(w, lipgloss.Center, head))
	}
	if m.mode == modeList {
		lines = append(lines, sFaint.Render(strings.Repeat("─", w)))
	}
	used := lipgloss.Height(strings.Join(lines, "\n"))
	lowH := h - used - 2

	var low string
	switch m.mode {
	case modeStop:
		low = drawer("Stop "+m.stopT.key, m.stopForm(w-6), w, lowH)
	case modeAdd:
		low = drawer("Add a ticket", m.addForm(w-6), w, lowH)
	default:
		trayW := 24
		listW := w - trayW - 1
		ts := m.st.sorted()
		var rows []string
		for i, x := range ts {
			cur := " "
			if i == m.sel {
				cur = gCursor
			}
			key := x.key
			tot := sDim.Render(hm(x.total(m.now)))
			if x.total(m.now) == 0 {
				tot = sFaint.Render("—")
			}
			switch x.state() {
			case stRunning:
				key = sRunning.Render(x.key)
				tot = sRunning.Render(hm(x.total(m.now)))
			case stPaused:
				key = sPaused.Render(x.key)
				if x.recovered() {
					key = sRecov.Render(x.key)
				}
			}
			sum := summaryText(x)
			if c := m.changed(x); c != "" {
				sum = c
			}
			row := cur + stateGlyph(x) + " " + fit(key, 9) + fit(sum, listW-2-10-8) + rfit(tot, 7)
			if i == m.sel {
				row = sSel.Render(fit(row, listW))
			}
			rows = append(rows, fit(row, listW))
		}
		if len(ts) == 0 {
			rows = append(rows, sDim.Render("  a — add your first ticket"))
		}
		if len(rows) > lowH {
			start := max(0, min(m.sel-lowH/2, len(rows)-lowH))
			rows = rows[start : start+lowH]
		}
		var tray []string
		tray = append(tray, sDim.Render("owed to Jira"))
		synced := 0
		for _, wl := range m.st.worklogs() {
			if wl.e.sync == syncSynced {
				synced++
				continue
			}
			tray = append(tray, syncGlyphShort(wl.e)+" "+fit(wl.t.key, 9)+hm(wl.e.sendDur))
		}
		if len(tray) == 1 {
			tray = append(tray, sDim.Render(gSynced+" nothing"))
		}
		if synced > 0 {
			tray = append(tray, "", sDim.Render(gSynced+" "+itoa(synced)+" synced"))
		}
		sep := sFaint.Render(strings.Repeat("│\n", max(0, lowH-1)) + "│")
		low = lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Height(lowH).Render(strings.Join(rows, "\n")),
			sep,
			lipgloss.NewStyle().PaddingLeft(1).Render(strings.Join(tray, "\n")))
	}
	lines = append(lines, low, m.lastEvent(w), m.keybar(w))
	return strings.Join(lines, "\n")
}

func drawer(title, body string, w, h int) string {
	return lipgloss.NewStyle().Border(lipgloss.ThickBorder(), true, false, false, false).BorderForeground(cFg).
		Width(w).Height(h).Padding(0, 2).Render(sBold.Render(title) + "\n" + body)
}

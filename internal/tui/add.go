package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/j-dsmith/punchcard/internal/core"
)

// maxRecents is how many recent tickets the add dialog offers.
const maxRecents = 8

// addDialog is a: type a key, or pick one of the recents (the list's
// own order, filtered by what is typed) with ↑↓. A key new to the list,
// or still unverified on it, is looked up in the tracker first, as
// `start` does; a 404 on a new key keeps the dialog open and changes
// nothing. With no tracker nothing is looked up, so tab moves to a
// summary field: the summary is what the user types (ADR 0008).
type addDialog struct {
	in          textinput.Model
	summary     textinput.Model
	withSummary bool   // no tracker: the summary field shows
	onSummary   bool   // the summary field has the keys
	pick        int    // -1: the typed key; else an index into recents
	err         string // the last refusal, shown under the input
	asking      string // the key the tracker is being asked about; keys wait meanwhile
}

func (m Model) openAdd() (Model, tea.Cmd) {
	d := addDialog{in: newInput(m.grammar.Example()), summary: newInput("optional"), withSummary: m.noTracker, pick: -1}
	d.in.CharLimit = 32
	cmd := d.in.Focus()
	m.dlg = d
	return m, cmd
}

// recents are the tracked tickets in list order whose key or summary
// contains what is typed.
func (d addDialog) recents(m Model) []core.TrackedTicket {
	q := strings.ToUpper(strings.TrimSpace(d.in.Value()))
	var out []core.TrackedTicket
	for _, t := range m.rows() {
		if len(out) == maxRecents {
			break
		}
		if q == "" || strings.Contains(strings.ToUpper(t.Key), q) || strings.Contains(strings.ToUpper(t.Summary), q) {
			out = append(out, t)
		}
	}
	return out
}

func (d addDialog) title() string { return "Add ticket" }

func (d addDialog) bindings() []binding {
	if d.withSummary {
		return []binding{{"enter", "add"}, {"tab", "field"}, {"↑↓", "recents"}, {"esc", "cancel"}}
	}
	return []binding{{"enter", "add"}, {"↑↓", "recents"}, {"esc", "cancel"}}
}

func (d addDialog) tick(Model) dialog { return d }

func (d addDialog) key(m Model, k tea.KeyPressMsg) (Model, tea.Cmd) {
	if d.asking != "" && k.String() != "esc" {
		return m, nil // Jira has the floor
	}
	switch k.String() {
	case "esc":
		m.dlg = nil
		return m, nil
	case "down":
		d.pick = min(len(d.recents(m))-1, d.pick+1)
	case "up":
		d.pick = max(-1, d.pick-1)
	case "enter":
		return d.submit(m)
	case "tab", "shift+tab":
		if !d.withSummary {
			break
		}
		d.onSummary = !d.onSummary
		if d.onSummary {
			d.in.Blur()
			d.summary.Focus()
		} else {
			d.summary.Blur()
			d.in.Focus()
		}
	default:
		var cmd tea.Cmd
		if d.onSummary {
			d.summary, cmd = d.summary.Update(k)
			m.dlg = d
			return m, cmd
		}
		d.in, cmd = d.in.Update(k)
		d.pick, d.err = -1, ""
		m.dlg = d
		return m, cmd
	}
	m.dlg = d
	return m, nil
}

// submit adds the picked or typed key: straight to the list if it is
// there and confirmed, else after asking Jira.
func (d addDialog) submit(m Model) (Model, tea.Cmd) {
	key := m.grammar.Normalize(d.in.Value())
	if rs := d.recents(m); d.pick >= 0 && d.pick < len(rs) {
		key = rs[d.pick].Key
	}
	switch {
	case key == "":
		return m, nil
	case !m.grammar.Valid(key):
		d.err = key + " is not a ticket key (like " + m.grammar.Example() + ")"
		m.dlg = d
		return m, nil
	}
	t, tracked := m.tracked(key)
	if tracked && !t.Unverified {
		m.dlg = nil
		m = m.notify(key + " is already on your list")
		for i, r := range m.rows() {
			if r.Key == key {
				m.sel, m.selKey = i, key
			}
		}
		return m, nil
	}
	if m.noTracker {
		// Nothing to ask: added as typed, with the summary typed, never
		// unverified.
		return d.answered(m, fetchMsg{key: key, forAdd: true, tracked: tracked, found: true,
			summary: strings.TrimSpace(d.summary.Value())})
	}
	d.asking, d.err = key, ""
	m.dlg = d
	return m, m.fetch(key, tracked, true)
}

// answered takes Jira's word on the key being added.
func (d addDialog) answered(m Model, f fetchMsg) (Model, tea.Cmd) {
	d.asking = ""
	if f.notFound && !f.tracked {
		d.err = f.key + " not found — or not visible to this Jira account"
		m.dlg = d
		return m, nil
	}
	m.dlg = nil
	m.pendingSel = f.key
	say := "added " + f.key
	switch {
	case f.tracked:
		say = f.key + " is already on your list"
	case !f.found:
		say += " (not verified with Jira yet)"
	}
	return m.enqueue(intent{kind: intentAdd, say: say,
		ticket: core.NewTicket{Key: f.key, Summary: f.summary, Unverified: !f.found}})
}

func (d addDialog) body(m Model, w int) []string {
	var lines []string
	if d.withSummary {
		lines = append(lines, field("Ticket key", d.in, !d.onSummary, w)...)
		lines = append(lines, field("Summary (optional)", d.summary, d.onSummary, w)...)
	} else {
		lines = append(lines, sBold.Render("Ticket key"))
		in := d.in
		in.SetWidth(max(1, w-3))
		lines = append(lines, m.pal.brand.Render(gCursor)+" "+in.View())
	}
	switch {
	case d.asking != "":
		lines = append(lines, wrap(sSync.Render("asking Jira about "+d.asking+"…"), w)...)
	case d.err != "":
		lines = append(lines, wrap(sFail.Render(d.err), w)...)
	case d.withSummary:
		lines = append(lines, wrap(sDim.Render("enter adds it · tab moves to the summary · ↓ picks a recent one"), w)...)
	default:
		lines = append(lines, wrap(sDim.Render("enter adds it · ↓ picks a recent one"), w)...)
	}
	rs := d.recents(m)
	if len(rs) == 0 {
		return lines
	}
	lines = append(lines, "", sDim.Render("recent"))
	for i, t := range rs {
		if i == d.pick {
			lines = append(lines, sSel.Render(fit(m.pal.brand.Render(gCursor)+" "+t.Key+" "+t.Summary, w)))
			continue
		}
		lines = append(lines, fit("  "+t.Key+" "+sDim.Render(t.Summary), w))
	}
	return lines
}

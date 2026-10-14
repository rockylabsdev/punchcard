package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/j-dsmith/punchcard/internal/core"
)

// awayDialog is -: "I wasn't working from From to To". Enter is one
// core.Away, which removes that window from the selected open entry
// wherever it falls; To empty means until now, so the entry ends paused
// at From. A paused entry whose To is after its pause resumes at To. On a
// paused entry From starts at the pause. Both take core.ParseAt's forms.
// The last line previews the cut, or says why core would refuse it: the
// preview is the same Apply.
type awayDialog struct {
	ticket, entryID string
	summary         string
	since           string // "running since 08:58" or "paused at 10:30"
	// pausedAt is a paused entry's pause, and prefill From's first value
	// (its HH:MM): From left as it was means the pause itself, to the
	// second.
	pausedAt core.Timestamp
	prefill  string
	from, to textinput.Model
	onTo     bool
}

func (m Model) openAway() Model {
	t, ok := m.selected()
	if !ok {
		return m
	}
	e, open := openEntry(t)
	if !open {
		m = m.notify("nothing to cut on " + t.Key + " — space starts it")
		return m
	}
	hhmm := func(ts core.Timestamp) string { return core.Clock(ts.Time, m.now.Location()) }
	last := e.Segments[len(e.Segments)-1]
	d := awayDialog{ticket: t.Key, entryID: e.ID, summary: t.Summary,
		from: newInput("10:00 or -45m"), to: newInput("now")}
	if last.Open() {
		d.since = "running since " + hhmm(last.StartedAt)
	} else {
		d.since = "paused at " + hhmm(last.EndedAt)
		d.pausedAt, d.prefill = last.EndedAt, hhmm(last.EndedAt)
		d.from.SetValue(d.prefill)
		d.from.CursorEnd()
	}
	d.from.Focus()
	m.dlg = d
	return m
}

func (d awayDialog) title() string     { return "Away from " + d.ticket }
func (d awayDialog) tick(Model) dialog { return d }

func (d awayDialog) bindings() []binding {
	return []binding{{"enter", "cut"}, {"tab", "field"}, {"esc", "cancel"}}
}

// cut is the one change the fields ask for, a core.Away: the window
// From–To, To zero for now. From left at the prefilled pause is the
// pause to the second. An error says what cannot be read.
func (d awayDialog) cut(m Model) (core.Away, error) {
	c := core.Away{Key: d.ticket, Origin: core.OriginHand, From: d.pausedAt.Time}
	if d.prefill == "" || strings.TrimSpace(d.from.Value()) != d.prefill {
		from, err := core.ParseAt(d.from.Value(), m.now)
		if err != nil {
			return core.Away{}, fmt.Errorf("from? try 10:00 or -45m")
		}
		c.From = from
	}
	if strings.TrimSpace(d.to.Value()) != "" {
		to, err := core.ParseAt(d.to.Value(), m.now)
		if err != nil {
			return core.Away{}, fmt.Errorf("to? try 10:45 or -5m, or leave it empty for now")
		}
		c.To = to
	}
	return c, nil
}

// awayPreview is what a cut would do to the entry.
type awayPreview struct {
	c core.Away
	// changes says the cut changes anything: counted time in the
	// window, or a resume at To.
	changes bool
	// removed is the counted time taken out; after the entry's elapsed
	// once cut, and added the time a resume at To counts from then.
	removed, after, added time.Duration
}

// preview applies the cut to what is shown, as the store would.
func (d awayDialog) preview(m Model) (awayPreview, error) {
	c, err := d.cut(m)
	if err != nil {
		return awayPreview{}, err
	}
	w := m.shown()
	e, _ := entryByID(w, d.ticket, d.entryID)
	before := core.Elapsed(e, m.now)
	next, events, err := core.Apply(w, c, m.now)
	if err != nil {
		return awayPreview{}, err
	}
	p := awayPreview{c: c, after: before}
	for _, ev := range events {
		if ev.Key == d.ticket && (ev.Kind == core.EventPaused || ev.Kind == core.EventStarted) {
			p.changes = true
		}
	}
	if !p.changes {
		return p, nil
	}
	ne, _ := entryByID(next, d.ticket, d.entryID)
	p.after = core.Elapsed(ne, m.now)
	if len(e.Segments) > 0 && !e.Running() && ne.Running() {
		p.added = m.now.Sub(c.To)
	}
	p.removed = before + p.added - p.after
	return p, nil
}

func entryByID(w core.World, key, id string) (core.TimeEntry, bool) {
	for _, t := range w.State.TrackedTickets {
		if t.Key != key {
			continue
		}
		for _, e := range t.Entries {
			if e.ID == id {
				return e, true
			}
		}
	}
	return core.TimeEntry{}, false
}

func (d awayDialog) key(m Model, k tea.KeyPressMsg) (Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.dlg = nil
		return m, nil
	case "tab", "shift+tab", "up", "down":
		d.onTo = !d.onTo
		if d.onTo {
			d.from.Blur()
			d.to.Focus()
		} else {
			d.to.Blur()
			d.from.Focus()
		}
		m.dlg = d
		return m, nil
	case "enter":
		p, err := d.preview(m)
		if err != nil {
			m.dlg = d // the preview line already says why
			return m, nil
		}
		m.dlg = nil
		if !p.changes {
			return m, nil // nothing counted in the window: just close
		}
		return m.enqueue(intent{kind: intentCommand, cmd: p.c, say: pausedSay(p.c, m.now.Location())})
	}
	var cmd tea.Cmd
	if d.onTo {
		d.to, cmd = d.to.Update(k)
	} else {
		d.from, cmd = d.from.Update(k)
	}
	m.dlg = d
	return m, cmd
}

func (d awayDialog) body(m Model, w int) []string {
	lines := []string{fit(sBold.Render(d.ticket)+" "+sDim.Render(d.summary), w)}
	lines = append(lines, wrap(sDim.Render(d.since), w)...)
	lines = append(lines, "")
	lines = append(lines, field("From", d.from, !d.onTo, w)...)
	lines = append(lines, field("To", d.to, d.onTo, w)...)
	lines = append(lines, "")
	if strings.TrimSpace(d.from.Value()) == "" {
		return append(lines, wrap(sDim.Render("when did you leave? 10:00 or -45m"), w)...)
	}
	p, err := d.preview(m)
	if err != nil {
		return append(lines, wrap(sFail.Render(gFailed+" "+core.Reason(err)), w)...)
	}
	loc := m.now.Location()
	if !p.changes {
		to := m.now
		if !p.c.To.IsZero() {
			to = p.c.To
		}
		return append(lines, wrap(sDim.Render("nothing counted between "+core.Clock(p.c.From, loc)+" and "+core.Clock(to, loc)), w)...)
	}
	verb, diff := "cuts", p.removed
	if diff == 0 { // only a resume at To
		verb, diff = "adds", p.added
	}
	becomes := fmt.Sprintf("%d:%02d", int(p.after.Hours()), int(p.after.Minutes())%60)
	return append(lines, wrap("→ "+verb+" "+sPaused.Bold(true).Render(core.FormatDuration(diff))+" · entry becomes "+becomes, w)...)
}

// pausedSay is what the Away dialog did, in the pane's words: a gap the
// user made is a pause, however it was made.
func pausedSay(c core.Away, loc *time.Location) string {
	if c.To.IsZero() {
		return "paused " + c.Key + " at " + core.Clock(c.From, loc)
	}
	return "paused " + c.Key + " " + core.Clock(c.From, loc) + "–" + core.Clock(c.To, loc)
}

package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/j-dsmith/punchcard/internal/core"
)

// awayDialog is -: cut time the user was away out of the selected open
// entry. From is when they left (a pause at that time), To when they came
// back (a resume at that time, refused while another ticket runs); To
// empty leaves the entry paused from From. Enter is one core.Away. On a paused entry From starts at the pause, so editing it moves
// the pause. Both take core.ParseAt's forms. The last line previews the
// cut, or says why core would refuse it: the preview is the same Apply.
type awayDialog struct {
	ticket, entryID string
	summary         string
	since           string // "running since 08:58" or "paused at 10:30"
	// pausedAt is a paused entry's pause, and prefill From's first value
	// (its HH:MM): From left as it was means the pause itself.
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
	hhmm := func(ts core.Timestamp) string { return ts.In(m.now.Location()).Format("15:04") }
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

// cut is the one change the fields ask for, a core.Away: a pause at
// From (none when From is left at the pause), then, if To is set, a
// resume at To. A zero Away asks for nothing. An error says what cannot
// be read.
func (d awayDialog) cut(m Model) (core.Away, error) {
	c := core.Away{Key: d.ticket, Origin: core.OriginHand}
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

// asks says c changes anything.
func asks(c core.Away) bool { return !c.From.IsZero() || !c.To.IsZero() }

// preview applies the cut to what is shown, as the store would, and
// returns it with the entry's elapsed before and after.
func (d awayDialog) preview(m Model) (c core.Away, before, after time.Duration, err error) {
	if c, err = d.cut(m); err != nil {
		return c, 0, 0, err
	}
	w := m.shown()
	before = entryElapsed(w, d.ticket, d.entryID, m.now)
	if !asks(c) {
		return c, before, before, nil
	}
	if w, _, err = core.Apply(w, c, m.now); err != nil {
		return c, 0, 0, err
	}
	return c, before, entryElapsed(w, d.ticket, d.entryID, m.now), nil
}

func entryElapsed(w core.World, key, id string, now time.Time) time.Duration {
	for _, t := range w.State.TrackedTickets {
		if t.Key != key {
			continue
		}
		for _, e := range t.Entries {
			if e.ID == id {
				return core.Elapsed(e, now)
			}
		}
	}
	return 0
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
		c, _, _, err := d.preview(m)
		if err != nil {
			m.dlg = d // the preview line already says why
			return m, nil
		}
		m.dlg = nil
		if !asks(c) {
			return m, nil // a pause left where it was: just close
		}
		return m.enqueue(intent{kind: intentCommand, cmd: c, say: "cut time away from " + d.ticket})
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
	c, before, after, err := d.preview(m)
	if err != nil {
		msg := strings.TrimPrefix(err.Error(), core.ErrUsage.Error()+": ")
		return append(lines, wrap(sFail.Render(gFailed+" "+msg), w)...)
	}
	if !asks(c) {
		return append(lines, wrap(sDim.Render("change From to move the pause, or set To to resume"), w)...)
	}
	verb, diff := "cuts", before-after
	if diff < 0 {
		verb, diff = "adds", -diff
	}
	becomes := fmt.Sprintf("%d:%02d", int(after.Hours()), int(after.Minutes())%60)
	return append(lines, wrap("→ "+verb+" "+sPaused.Bold(true).Render(hm(diff))+" · entry becomes "+becomes, w)...)
}
